package agente

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/domain"
	"promatec/cockpit/internal/platform/db"
)

// Servizio è ciò che collega l'agente al database: costruisce il contesto da un messaggio già
// ingerito, evita di ripagare la stessa domanda, e scrive il risultato in `analisi_messaggio`.
//
// Non scrive nient'altro. In particolare non tocca `messaggio.thread_id`, `proposta_triage.stato`,
// `identificativo_thread`, `documento` né la coda dei job verso Outlook: la proposta dell'agente
// vive in una tabella sua e diventa un'azione solo quando un operatore la sceglie.
type Servizio struct {
	Modello Modello
	// Attivo: spento se non lo si accende in configurazione. Il testo delle mail dei clienti esce
	// verso un servizio esterno, ed è una cosa che si mette per iscritto prima, non dopo.
	Attivo bool
	// Caselle sono gli indirizzi delle caselle su cui l'analisi è permessa. Vuoto = nessuna: un
	// elenco vuoto che significasse «tutte» sarebbe il modo più silenzioso di accendere tutto.
	Caselle map[string]bool
}

// ErrSpento: l'agente non è acceso, o non lo è per questa casella. Non è un errore da registrare
// come fallimento: è la configurazione che dice di no.
var ErrSpento = errors.New("analisi semantica non attiva")

// Consentito dice se si può analizzare un messaggio arrivato in queste caselle.
func (s *Servizio) Consentito(indirizzi []string) bool {
	if s == nil || !s.Attivo || s.Modello == nil {
		return false
	}
	for _, i := range indirizzi {
		if s.Caselle[strings.ToLower(strings.TrimSpace(i))] {
			return true
		}
	}
	return false
}

// Contesto costruisce, dai FATTI già in database, tutto ciò che viene mandato al modello. È l'unico
// punto in cui si decide che cosa esce: il prompt legge questo e nient'altro.
//
// Il server preseleziona: non passa la casella, passa il messaggio e le poche richieste che il motore
// deterministico ha già indicato come candidate. Mandare tutta la corrispondenza a un servizio
// esterno per farsi dire quale sia la richiesta giusta sarebbe caro, lento e sbagliato — la domanda
// «quali richieste somigliano a questa» ha già una risposta deterministica.
func (s *Servizio) Contesto(ctx context.Context, q *db.Queries, messaggioID uuid.UUID) (Contesto, error) {
	var c Contesto
	m, err := q.GetMessaggio(ctx, messaggioID)
	if err != nil {
		return c, err
	}
	c.Oggetto = m.Oggetto.String
	c.Corpo = m.CorpoTesto.String
	c.Mittente = m.MittenteIndirizzo.String

	riga, err := q.GetInboxRiga(ctx, messaggioID)
	if err == nil && riga.Cliente.Valid {
		c.Cliente = riga.Cliente.String
	}
	allegati, err := q.ListAllegatiMessaggio(ctx, messaggioID)
	if err != nil {
		return c, err
	}
	for _, a := range allegati {
		if a.Natura == db.NaturaAllegatoFile || a.Natura == db.NaturaAllegatoElementoOutlook {
			c.NomiAllegati = append(c.NomiAllegati, a.NomeFile)
		}
	}
	cand, err := q.ListCandidatiAggancio(ctx, messaggioID)
	if err != nil {
		return c, err
	}
	for _, k := range cand {
		c.Candidati = append(c.Candidati, RichiestaNota{
			ThreadID: k.ThreadID.String(), Oggetto: k.Oggetto.String,
			Perche: string(k.Regola) + " — " + k.Evidenza})
	}
	codici, err := q.ListCandidatiCodice(ctx, messaggioID)
	if err != nil {
		return c, err
	}
	for _, k := range codici {
		c.CodiciNoti = append(c.CodiciNoti, k.Codice)
	}
	return c, nil
}

// Analizza esegue il giro e lo registra. Restituisce l'analisi scritta, o ErrSpento.
//
// Idempotente per costruzione: se esiste già un'analisi con lo stesso input, lo stesso prompt e lo
// stesso modello, la restituisce senza chiedere niente. Rianalizzare è una scelta — si cambia il
// prompt, o si cambia il modello — non un effetto collaterale di un job ripetuto.
func (s *Servizio) Analizza(ctx context.Context, q *db.Queries, messaggioID uuid.UUID) (db.AnalisiMessaggio, error) {
	var vuota db.AnalisiMessaggio
	if s == nil || !s.Attivo || s.Modello == nil {
		return vuota, ErrSpento
	}
	c, err := s.Contesto(ctx, q, messaggioID)
	if err != nil {
		return vuota, err
	}
	impronta := c.Impronta()
	chiave := db.GetAnalisiPerInputParams{
		MessaggioID: messaggioID, InputHash: impronta, Prompt: VersionePrompt, Modello: s.Modello.Nome()}
	if gia, err := q.GetAnalisiPerInput(ctx, chiave); err == nil {
		return gia, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return vuota, err
	}
	riga, err := q.ApriAnalisi(ctx, db.ApriAnalisiParams{
		MessaggioID: messaggioID, InputHash: impronta, Versione: VersioneAnalizzatore,
		Prompt: VersionePrompt, Modello: s.Modello.Nome()})
	if errors.Is(err, pgx.ErrNoRows) {
		// qualcun altro l'ha aperta nel frattempo: si rilegge la sua invece di farne una seconda
		return q.GetAnalisiPerInput(ctx, chiave)
	}
	if err != nil {
		return vuota, err
	}

	inizio := time.Now()
	e := Analizza(ctx, s.Modello, c)
	durata := int32(time.Since(inizio).Milliseconds())

	risultato, _ := json.Marshal(e.Proposta)
	scarti, _ := json.Marshal(nonNil(e.Scarti))
	var grezzo *json.RawMessage
	if json.Valid(e.Grezzo) {
		g := json.RawMessage(e.Grezzo)
		grezzo = &g
	} else if len(e.Grezzo) > 0 {
		// non è JSON: si conserva come stringa JSON, perché è proprio il caso in cui serve rileggerlo
		if b, err := json.Marshal(string(e.Grezzo)); err == nil {
			g := json.RawMessage(b)
			grezzo = &g
		}
	}
	stato := db.StatoAnalisiCompletata
	var messaggioErrore string
	if e.Errore != nil {
		stato = db.StatoAnalisiErrore
		if errors.Is(e.Errore, ErrSchema) {
			stato = db.StatoAnalisiRifiutata
		}
		messaggioErrore = e.Errore.Error()
		risultato = []byte("{}")
	}
	return q.ChiudiAnalisi(ctx, db.ChiudiAnalisiParams{
		AnalisiID: riga.AnalisiID, Stato: stato, Risultato: risultato, Grezzo: grezzo, Scartato: scarti,
		Errore:  txt(messaggioErrore),
		TokenIn: int4(e.TokenIn), TokenOut: int4(e.TokenOut), DurataMs: int4(int(durata)),
	})
}

// LeggiProposta rilegge l'ultima analisi completata di un messaggio, già verificata. È ciò che la
// schermata mostra: non il grezzo, che resta in database per capire che cosa è stato scartato.
func LeggiProposta(ctx context.Context, q *db.Queries, messaggioID uuid.UUID) (Proposta, []Scarto, bool) {
	riga, err := q.UltimaAnalisiCompletata(ctx, messaggioID)
	if err != nil {
		return Proposta{}, nil, false
	}
	var p Proposta
	if err := json.Unmarshal(riga.Risultato, &p); err != nil {
		return Proposta{}, nil, false
	}
	var scarti []Scarto
	_ = json.Unmarshal(riga.Scartato, &scarti)
	return p, scarti, true
}

// PropostaLeggibile traduce la proposta in righe da mostrare, con la provenienza. Sta qui e non nel
// template perché la stessa frase serve anche al replay del corpus.
func PropostaLeggibile(p Proposta) []string {
	var out []string
	out = append(out, "intento proposto: "+p.Intento)
	if p.Riferimento != "" {
		out = append(out, "riferimento della richiesta: "+p.Riferimento)
	}
	for _, k := range p.Codici {
		out = append(out, fmt.Sprintf("codice %s (%s)", k.Codice, k.Ruolo))
	}
	out = append(out, p.CosaManca...)
	return out
}

func nonNil(s []Scarto) []Scarto {
	if s == nil {
		return []Scarto{}
	}
	return s
}

// txt e int4 sono i soliti adattatori verso i tipi nullable di pgx.
func txt(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

func int4(n int) pgtype.Int4 { return pgtype.Int4{Int32: int32(n), Valid: n != 0} }

// Conferma le dipendenze che il pacchetto usa dal dominio senza importarlo a vuoto: i ruoli dei
// codici sono gli stessi, e se qualcuno ne aggiunge uno di là questo controllo lo fa notare qui.
var _ = func() bool {
	for _, r := range []string{domain.RuoloRiferimento, domain.RuoloProdotto, domain.RuoloParte, domain.RuoloIgnoto} {
		if !ruoliValidi[r] {
			panic("agente: il ruolo " + r + " esiste nel dominio ma l'agente non lo accetta")
		}
	}
	return true
}()
