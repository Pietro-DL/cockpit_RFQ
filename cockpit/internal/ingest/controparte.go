package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/aggancio"
	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/domain"
)

// LA CONTROPARTE NELL'INGEST (blocco 7A, D33)
//
// Il resolver e' in `domain` e non sa niente del database; qui c'e' la rubrica vera, la scrittura
// del fatto sul messaggio, l'interpretazione riusabile (triage + candidati) e i due ricalcoli:
// quello mirato dopo un «Censisci» e quello di tutti i messaggi entrati prima della 0014.

// rubricaDB risponde alle quattro domande del resolver con l'anagrafica in database.
type rubricaDB struct{ q *db.Queries }

func (r rubricaDB) ContattiFornitore(ctx context.Context, email string) ([]domain.Voce, error) {
	righe, err := r.q.ListFornitoriPerContatto(ctx, email)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Voce, 0, len(righe))
	for _, f := range righe {
		out = append(out, domain.Voce{ID: f.FornitoreID, Nome: f.RagioneSociale})
	}
	return out, nil
}

func (r rubricaDB) BuyerCliente(ctx context.Context, email string) (domain.Voce, bool, error) {
	b, err := r.q.GetBuyerPerEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Voce{}, false, nil
	}
	if err != nil {
		return domain.Voce{}, false, err
	}
	nome := ""
	if c, err := r.q.GetCliente(ctx, b.ClienteID); err == nil {
		nome = c.CartellaNas
	}
	return domain.Voce{ID: b.ClienteID, Nome: nome}, true, nil
}

func (r rubricaDB) FornitorePerDominio(ctx context.Context, dominio string) (domain.Voce, bool, error) {
	f, err := r.q.GetFornitorePerDominio(ctx, dominio)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Voce{}, false, nil
	}
	if err != nil {
		return domain.Voce{}, false, err
	}
	return domain.Voce{ID: f.FornitoreID, Nome: f.RagioneSociale}, true, nil
}

func (r rubricaDB) ClientePerDominio(ctx context.Context, dominio string) (domain.Voce, bool, error) {
	c, err := r.q.GetClientePerDominio(ctx, dominio)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Voce{}, false, nil
	}
	if err != nil {
		return domain.Voce{}, false, err
	}
	return domain.Voce{ID: c.ClienteID, Nome: c.CartellaNas}, true, nil
}

// indirizziDi sono i soli indirizzi dei destinatari, nell'ordine in cui stanno nel messaggio.
func indirizziDi(d []api.Destinatario) []string {
	out := make([]string, 0, len(d))
	for _, x := range d {
		out = append(out, x.Indirizzo)
	}
	return out
}

// indirizziDaJSON legge i destinatari salvati sul messaggio (`messaggio.destinatari`).
func indirizziDaJSON(raw json.RawMessage) []string {
	var d []api.Destinatario
	if len(raw) == 0 || json.Unmarshal(raw, &d) != nil {
		return nil
	}
	return indirizziDi(d)
}

// risolviControparte applica il resolver con la rubrica vera e l'elenco delle nostre caselle.
func risolviControparte(ctx context.Context, q *db.Queries, nostri Nostri, mittente string, destinatari []string) (domain.Controparte, error) {
	in := domain.IngressoControparte{Mittente: mittente, Destinatari: destinatari}
	if len(nostri) > 0 {
		in.Nostro = nostri.Nostro
	}
	return domain.RisolviControparte(ctx, in, rubricaDB{q})
}

// clienteDallaControparte e' il cliente per il triage, le regole e lo staging automatico: SOLO in
// entrata, e SOLO se la controparte e' un cliente. Un fornitore non ha un cliente; un ambiguo
// nemmeno, finche' una persona non decide. Il buyer si prende quando il riconoscimento e' passato
// dall'indirizzo esatto, come prima della 0014.
func clienteDallaControparte(ctx context.Context, q *db.Queries, c domain.Controparte, dir db.Direzione) (*db.Buyer, uuid.NullUUID, error) {
	if dir != db.DirezioneEntrata || c.Tipo != domain.ControparteCliente {
		return nil, uuid.NullUUID{}, nil
	}
	clienteID := uuid.NullUUID{UUID: c.ClienteID, Valid: true}
	if c.Via != domain.ViaContatto {
		return nil, clienteID, nil
	}
	b, err := q.GetBuyerPerEmail(ctx, c.Indirizzo)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, clienteID, nil
	}
	if err != nil {
		return nil, clienteID, err
	}
	return &b, clienteID, nil
}

// parametriControparte traduce la risposta del resolver nella riga da scrivere.
func parametriControparte(id uuid.UUID, c domain.Controparte) db.SetControparteMessaggioParams {
	p := db.SetControparteMessaggioParams{MessaggioID: id, ControparteTipo: db.TipoControparte(c.Tipo)}
	if c.Via != "" {
		p.Via = db.NullViaControparte{ViaControparte: db.ViaControparte(c.Via), Valid: true}
	}
	switch c.Tipo {
	case domain.ControparteCliente:
		p.ControparteClienteID = uuid.NullUUID{UUID: c.ClienteID, Valid: true}
	case domain.ControparteFornitore:
		p.ControparteFornitoreID = uuid.NullUUID{UUID: c.FornitoreID, Valid: true}
	}
	return p
}

// interpretazione e' cio' che serve per proporre un esito su un messaggio orfano: lo stesso
// insieme sia al primo ingresso sia al ricalcolo. Se fossero due elenchi in due posti, il giorno
// in cui uno impara a leggere un campo in piu' l'altro no.
type interpretazione struct {
	MessaggioID     uuid.UUID
	ConversazioneID uuid.UUID
	ClienteID       uuid.NullUUID
	BuyerID         uuid.NullUUID
	Controparte     domain.Controparte
	Oggetto, Corpo  string
	NomiAllegati    []string
	Direzione       db.Direzione
	Interno         bool
	InReplyTo       string
	Riferimenti     []string
	DataEvento      time.Time
	Motore          *domain.Motore
	// Blocco 7B: il mittente (indirizzi automatici) e il fornitore, per i candidati verso una richiesta.
	Mittente    string
	FornitoreID uuid.UUID
}

// interpreta calcola i candidati di aggancio, il triage e i candidati di codice, e scrive la
// proposta. Non decide niente: UpsertTriage non tocca una proposta gia' accettata o rifiutata.
func (s *Servizio) interpreta(ctx context.Context, q *db.Queries, in interpretazione) (domain.EsitoTriage, error) {
	it := domain.IngressoTriage{
		Oggetto: in.Oggetto, Corpo: in.Corpo, NomiAllegati: in.NomiAllegati, Direzione: string(in.Direzione),
		Interno: in.Interno, ClienteNoto: in.ClienteID.Valid, BuyerNoto: in.BuyerID.Valid,
		Controparte: in.Controparte.Tipo, Motore: in.Motore, Mittente: in.Mittente,
	}
	e := in.Motore.Estrai(it.Testi()...)
	codiciFamiglia := domain.SoloCodici(domain.DiFamiglia(e.Codici))
	cand, err := aggancio.CalcolaESalva(ctx, q, aggancio.Ingresso{
		MessaggioID: in.MessaggioID, ConversazioneID: in.ConversazioneID,
		ClienteID: in.ClienteID, BuyerID: in.BuyerID,
		InReplyTo: in.InReplyTo, Riferimenti: in.Riferimenti,
		Oggetto: domain.OggettoPulito(in.Oggetto), DataEvento: in.DataEvento,
		Codici: codiciFamiglia, Riferimento: e.Riferimento,
		FinestraGG: in.Motore.Finestra(),
	})
	if err != nil {
		return domain.EsitoTriage{}, fmt.Errorf("candidati di aggancio: %w", err)
	}
	it.Candidati = cand
	// Blocco 7B: la posta di un fornitore si aggancia alla RICHIESTA che gli abbiamo mandato (R0, R1,
	// R3f verso richiesta_fornitore); una nostra mail a un fornitore che cita una RFQ aperta e' la
	// richiesta mandata a mano (RF_oggetto). Sono candidati, e li vede l'operatore.
	fornitore := in.Controparte.Tipo == domain.ControparteFornitore && in.FornitoreID != uuid.Nil
	if fornitore && in.Direzione == db.DirezioneEntrata {
		cr, err := aggancio.CalcolaRichieste(ctx, q, aggancio.IngressoRichieste{
			MessaggioID: in.MessaggioID, FornitoreID: in.FornitoreID, ConversazioneID: in.ConversazioneID,
			InReplyTo: in.InReplyTo, Riferimenti: in.Riferimenti, Codici: codiciFamiglia,
		})
		if err != nil {
			return domain.EsitoTriage{}, fmt.Errorf("candidati richiesta: %w", err)
		}
		if err := aggancio.SalvaCandidatiRichiesta(ctx, q, in.MessaggioID, cr); err != nil {
			return domain.EsitoTriage{}, fmt.Errorf("candidati richiesta: %w", err)
		}
		it.CandidatiRichiesta = cr
	}
	if fornitore && in.Direzione == db.DirezioneUscita {
		rm, err := aggancio.RichiesteManuali(ctx, q, domain.SoloCodici(e.Codici))
		if err != nil {
			return domain.EsitoTriage{}, err
		}
		it.RichiesteManuali = rm
	}
	tr := domain.Triage(it)
	if err := aggancio.SalvaCandidatiCodice(ctx, q, in.MessaggioID, tr.Estrazione); err != nil {
		return tr, fmt.Errorf("candidati di codice: %w", err)
	}
	// Il motivo della controparte sta in testa ai motivi quando ha deciso l'esito: e' la frase che
	// l'operatore legge per capire perche' una «richiesta d'offerta» non e' diventata una RFQ.
	if m := in.Controparte.Motivo; m != "" && (in.Controparte.Tipo == domain.ControparteFornitore || in.Controparte.Tipo == domain.ControparteAmbiguo) {
		tr.Motivi = append([]string{m}, tr.Motivi...)
	}
	motivi, _ := json.Marshal(tr.Motivi)
	if tr.Codici == nil {
		tr.Codici = []string{}
	}
	var scad *time.Time
	if d, ok := domain.RilevaScadenza(in.Corpo, in.DataEvento); ok {
		scad = &d
	}
	var proposto, richiestaProposta, fornitoreProposto uuid.NullUUID
	if tr.Candidato != nil {
		if tid, err := uuid.Parse(tr.Candidato.ThreadID); err == nil {
			proposto = uuid.NullUUID{UUID: tid, Valid: true}
		}
	}
	if tr.CandidatoRichiesta != nil {
		if rid, err := uuid.Parse(tr.CandidatoRichiesta.RichiestaID); err == nil {
			richiestaProposta = uuid.NullUUID{UUID: rid, Valid: true}
		}
	}
	if tr.Intento == domain.IntentoRfqFornitore && proposto.Valid {
		fornitoreProposto = uuid.NullUUID{UUID: in.FornitoreID, Valid: true}
	}
	var intento db.NullIntentoMessaggio
	if tr.Intento != "" {
		intento = db.NullIntentoMessaggio{IntentoMessaggio: db.IntentoMessaggio(tr.Intento), Valid: true}
	}
	if _, err := q.UpsertTriage(ctx, db.UpsertTriageParams{
		MessaggioID: in.MessaggioID, Esito: db.EsitoTriage(tr.Esito), ClienteProposto: in.ClienteID, BuyerProposto: in.BuyerID,
		ThreadProposto: proposto, Intento: intento, RichiestaProposta: richiestaProposta, FornitoreProposto: fornitoreProposto,
		Identificativi: tr.Codici, ScadenzaProposta: scad, Confidenza: int16(tr.Confidenza), Motivi: motivi, Fonte: db.FonteTriageDeterministico,
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return tr, fmt.Errorf("triage: %w", err)
	}
	return tr, nil
}

// daInterpretare dice se un messaggio orfano va passato al triage: la posta in entrata e quella
// interna, come sempre, e dal 7B anche una nostra mail a un fornitore (la richiesta mandata a mano).
func daInterpretare(dir db.Direzione, interno bool, c domain.Controparte) bool {
	return dir == db.DirezioneEntrata || interno || (dir == db.DirezioneUscita && c.Tipo == domain.ControparteFornitore)
}

// motorePer sceglie il motore: quello del cliente, oppure — per un fornitore — l'unione delle famiglie
// dei clienti che hanno richieste aperte a lui (IB8). Nil per tutti gli altri.
func motorePer(ctx context.Context, q *db.Queries, motori *Motori, clienteID uuid.NullUUID, c domain.Controparte) *domain.Motore {
	if clienteID.Valid {
		return motori.Per(ctx, q, clienteID.UUID)
	}
	if c.Tipo == domain.ControparteFornitore && c.FornitoreID != uuid.Nil {
		return motori.PerFornitore(ctx, q, c.FornitoreID)
	}
	return nil
}

// EsitoRitriage e' il resoconto del ricalcolo mirato: quanti messaggi si sono guardati, quante
// controparti e quante proposte sono cambiate. Va nel log e nell'avviso di chi ha premuto il
// pulsante.
type EsitoRitriage struct {
	Guardati            int
	ContropartiCambiate int
	ProposteCambiate    int
	Dettagli            []string
}

func (e EsitoRitriage) String() string {
	return fmt.Sprintf("%d messaggi ricalcolati, %d controparti cambiate, %d proposte cambiate", e.Guardati, e.ContropartiCambiate, e.ProposteCambiate)
}

// Ritriage ricalcola controparte, candidati e proposta dei messaggi NON DECISI che parlano con
// questo indirizzo o con questo dominio (7A.3). Le decisioni prese non si toccano: la query non li
// restituisce nemmeno. Una transazione per messaggio: un messaggio che non si riesce a ricalcolare
// non ferma gli altri, e finisce nei dettagli.
func (s *Servizio) Ritriage(ctx context.Context, indirizzo, dominio string) (EsitoRitriage, error) {
	var out EsitoRitriage
	q := db.New(s.Pool)
	indirizzo = strings.ToLower(strings.TrimSpace(indirizzo))
	dominio = strings.ToLower(strings.TrimSpace(dominio))
	if indirizzo == "" && dominio == "" {
		return out, errors.New("ritriage senza indirizzo né dominio: non so quali messaggi guardare")
	}
	return s.ritriageChiavi(ctx, q, []string{indirizzo}, []string{dominio})
}

// RitriageMolti e' il Ritriage dopo una scrittura che tocca TANTI indirizzi in una volta: l'import
// del seme dei fornitori, il seme dei clienti (7B.5). Senza, i messaggi gia' arrivati resterebbero
// «sconosciuti» finche' qualcuno non li riguarda uno per uno: censire Polver e non vedere la sua
// posta passare in «Fornitori» e' il modo piu' rapido per non fidarsi piu' della schermata.
//
// Le decisioni prese restano intoccate, come sempre: e' la stessa strada di un censimento singolo,
// solo ripetuta. Un elenco vuoto non e' un errore — vuol dire che non c'era niente da scrivere.
func (s *Servizio) RitriageMolti(ctx context.Context, indirizzi, domini []string) (EsitoRitriage, error) {
	return s.ritriageChiavi(ctx, db.New(s.Pool), indirizzi, domini)
}

// ritriageChiavi raccoglie i messaggi di tutte le chiavi, li sfoltisce (lo stesso messaggio puo'
// rispondere a due domini diversi) e li ricalcola una volta sola.
func (s *Servizio) ritriageChiavi(ctx context.Context, q *db.Queries, indirizzi, domini []string) (EsitoRitriage, error) {
	var out EsitoRitriage
	chiavi := make([][2]string, 0, len(indirizzi)+len(domini))
	visto := map[[2]string]bool{}
	aggiungi := func(indirizzo, dominio string) {
		k := [2]string{strings.ToLower(strings.TrimSpace(indirizzo)), strings.ToLower(strings.TrimSpace(dominio))}
		if (k[0] == "" && k[1] == "") || visto[k] {
			return
		}
		visto[k] = true
		chiavi = append(chiavi, k)
	}
	for _, i := range indirizzi {
		aggiungi(i, "")
	}
	for _, d := range domini {
		aggiungi("", d)
	}
	if len(chiavi) == 0 {
		return out, nil
	}
	nostri, err := CaricaNostri(ctx, q)
	if err != nil {
		return out, err
	}
	var messaggi []db.Messaggio
	visti := map[uuid.UUID]bool{}
	for _, k := range chiavi {
		lotto, err := q.ListMessaggiDaRitriage(ctx, db.ListMessaggiDaRitriageParams{Indirizzo: k[0], Dominio: k[1]})
		if err != nil {
			return out, err
		}
		for _, m := range lotto {
			if !visti[m.MessaggioID] {
				visti[m.MessaggioID] = true
				messaggi = append(messaggi, m)
			}
		}
	}
	motori := NuoviMotori()
	for _, m := range messaggi {
		cambioC, cambioP, err := s.ritriageUno(ctx, nostri, motori, m)
		if err != nil {
			out.Dettagli = append(out.Dettagli, fmt.Sprintf("%s: %v", m.ChiaveEsterna, err))
			if s.Log != nil {
				s.Log.Warn("ritriage non riuscito", "messaggio", m.MessaggioID, "err", err)
			}
			continue
		}
		out.Guardati++
		if cambioC != "" {
			out.ContropartiCambiate++
			out.Dettagli = append(out.Dettagli, cambioC)
		}
		if cambioP != "" {
			out.ProposteCambiate++
			out.Dettagli = append(out.Dettagli, cambioP)
		}
	}
	if s.Log != nil {
		s.Log.Info("ritriage mirato", "chiavi", len(chiavi), "esito", out.String())
	}
	return out, nil
}

func (s *Servizio) ritriageUno(ctx context.Context, nostri Nostri, motori *Motori, m db.Messaggio) (cambioControparte, cambioProposta string, err error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	prima, err := q.GetTriageMessaggio(ctx, m.MessaggioID)
	esitoPrima, intentoPrima := "", ""
	if err == nil {
		esitoPrima = string(prima.Esito)
		if prima.Intento.Valid {
			intentoPrima = string(prima.Intento.IntentoMessaggio)
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", err
	}
	c, err := risolviControparte(ctx, q, nostri, m.MittenteIndirizzo.String, indirizziDaJSON(m.Destinatari))
	if err != nil {
		return "", "", err
	}
	if _, err := q.SetControparteMessaggio(ctx, parametriControparte(m.MessaggioID, c)); err != nil {
		return "", "", fmt.Errorf("controparte: %w", err)
	}
	if string(m.ControparteTipo) != c.Tipo {
		cambioControparte = fmt.Sprintf("%s: controparte %s → %s (%s)", m.ChiaveEsterna, m.ControparteTipo, c.Tipo, c.Motivo)
	}
	buyer, clienteID, err := clienteDallaControparte(ctx, q, c, m.Direzione)
	if err != nil {
		return "", "", err
	}
	buyerID := m.BuyerID
	if buyer != nil && !buyerID.Valid {
		buyerID = uuid.NullUUID{UUID: buyer.BuyerID, Valid: true}
		if err := q.SetBuyerMessaggio(ctx, db.SetBuyerMessaggioParams{MessaggioID: m.MessaggioID, BuyerID: buyerID}); err != nil {
			return "", "", err
		}
	}
	if !m.ThreadID.Valid && daInterpretare(m.Direzione, m.Interno, c) {
		motore := motorePer(ctx, q, motori, clienteID, c)
		var nomi []string
		if allegati, err := q.ListAllegatiMessaggio(ctx, m.MessaggioID); err == nil {
			for _, a := range allegati {
				if !a.ContenitoreID.Valid {
					nomi = append(nomi, a.NomeFile)
				}
			}
		}
		var inReplyTo string
		var riferimenti []string
		if mo, err := q.GetMessaggioOutlook(ctx, m.MessaggioID); err == nil {
			inReplyTo, riferimenti = mo.InReplyTo.String, mo.Riferimenti
		}
		// i candidati si ricalcolano da zero: un upsert lascerebbe in piedi quelli di prima
		if _, err := q.EliminaCandidatiAggancio(ctx, m.MessaggioID); err != nil {
			return "", "", err
		}
		if _, err := q.EliminaCandidatiCodice(ctx, m.MessaggioID); err != nil {
			return "", "", err
		}
		tr, err := s.interpreta(ctx, q, interpretazione{
			MessaggioID: m.MessaggioID, ConversazioneID: m.ConversazioneID,
			ClienteID: clienteID, BuyerID: buyerID, Controparte: c,
			Oggetto: m.Oggetto.String, Corpo: m.CorpoTesto.String, NomiAllegati: nomi,
			Direzione: m.Direzione, Interno: m.Interno, InReplyTo: inReplyTo, Riferimenti: riferimenti,
			DataEvento: m.DataEvento, Motore: motore, Mittente: m.MittenteIndirizzo.String, FornitoreID: c.FornitoreID,
		})
		if err != nil {
			return "", "", err
		}
		if tr.Esito != esitoPrima || tr.Intento != intentoPrima {
			cambioProposta = fmt.Sprintf("%s: proposta %s/%s → %s/%s", m.ChiaveEsterna, primo(esitoPrima, "nessuna"), primo(intentoPrima, "-"), tr.Esito, primo(tr.Intento, "-"))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return cambioControparte, cambioProposta, nil
}

func primo(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// RicalcolaControparti risolve la controparte dei messaggi che non ce l'hanno ancora: quelli
// entrati prima della 0014 (CP8). Non tocca proposte ne' candidati — quelli si ricalcolano per
// indirizzo con «Censisci» — e scrive nel log il conteggio per tipo prima e dopo. A lotti, senza
// transazione lunga: un server che riparte a meta' riprende da dove era.
func RicalcolaControparti(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) (int, error) {
	q := db.New(pool)
	prima, err := q.ContaControparti(ctx)
	if err != nil {
		return 0, err
	}
	nostri, err := CaricaNostri(ctx, q)
	if err != nil {
		return 0, err
	}
	fatti := 0
	for {
		lotto, err := q.ListMessaggiSenzaControparte(ctx, 500)
		if err != nil {
			return fatti, err
		}
		if len(lotto) == 0 {
			break
		}
		scritti := 0
		for _, m := range lotto {
			c, err := risolviControparte(ctx, q, nostri, m.MittenteIndirizzo.String, indirizziDaJSON(m.Destinatari))
			if err != nil {
				return fatti, err
			}
			n, err := q.SetControparteMessaggio(ctx, parametriControparte(m.MessaggioID, c))
			if err != nil {
				return fatti, fmt.Errorf("controparte di %s: %w", m.ChiaveEsterna, err)
			}
			scritti += int(n)
		}
		fatti += scritti
		if scritti == 0 {
			// nessuna riga aggiornata: qualcosa impedisce la scrittura, e ripetere il lotto
			// all'infinito non e' il modo di scoprirlo
			return fatti, fmt.Errorf("ricalcolo delle controparti: %d messaggi senza controparte, nessuno aggiornato", len(lotto))
		}
	}
	if fatti > 0 && log != nil {
		dopo, _ := q.ContaControparti(ctx)
		log.Info("controparti ricalcolate per i messaggi di prima della 0014", "messaggi", fatti,
			"prima", contiPerTipo(prima), "dopo", contiPerTipo(dopo))
	}
	return fatti, nil
}

func contiPerTipo(righe []db.ContaContropartiRow) string {
	var parti []string
	for _, r := range righe {
		parti = append(parti, fmt.Sprintf("%s=%d", r.Tipo, r.N))
	}
	return strings.Join(parti, " ")
}

// IndirizzoDaCensire e' l'indirizzo che un «Censisci» dal pannello del messaggio deve censire: il
// mittente in entrata, il primo destinatario esterno in uscita — lo stesso che il resolver ha
// guardato per decidere la controparte. Il dominio e' la parte dopo la chiocciola, vuoto se non
// c'e' (un mittente Exchange senza chiocciola non ha un dominio da censire).
func IndirizzoDaCensire(ctx context.Context, q *db.Queries, m db.Messaggio) (indirizzo, dominio string, err error) {
	nostri, err := CaricaNostri(ctx, q)
	if err != nil {
		return "", "", err
	}
	c, err := risolviControparte(ctx, q, nostri, m.MittenteIndirizzo.String, indirizziDaJSON(m.Destinatari))
	if err != nil {
		return "", "", err
	}
	indirizzo = c.Indirizzo
	if i := strings.LastIndex(indirizzo, "@"); i > 0 {
		dominio = indirizzo[i+1:]
	}
	return indirizzo, dominio, nil
}
