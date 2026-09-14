// Package ingest scrive il FATTO (messaggio, messaggio_outlook, allegato), esegue l'aggancio automatico
// deterministico e produce le prime INTERPRETAZIONI (proposta_triage, riferimento_portale).
// Non scrive mai sul NAS e non prende decisioni: ogni proposta è revocabile dall'operatore.
//
// Il lotto è UNA transazione con un savepoint per elemento (piano §2.4, D15). Prima era una
// transazione per elemento e il primo errore interrompeva il lotto: bastava un messaggio che il
// database rifiutava per fermare il sync e far ripartire la scansione dallo stesso punto, all'infinito.
// Ora l'elemento rotto viene annullato fino al suo savepoint e registrato in ingest_scarto, gli altri
// entrano, e il cursore avanza nella stessa transazione. Così «200 ⇔ tutto durevole» è una proprietà
// del codice, non una convenzione: se il commit fallisce la risposta è 5xx e in database non è rimasto
// nulla di parziale, quindi il worker può ripetere il lotto identico senza duplicare niente.
package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/domain"
)

// MaxIdentificativo: oltre questa lunghezza un Message-ID non è più un identificativo utilizzabile.
// Troncarlo sarebbe peggio che scartarlo: due messaggi diversi diventerebbero lo stesso (N34).
const MaxIdentificativo = 1000

type Servizio struct {
	Pool *pgxpool.Pool
	Log  *slog.Logger
}

// Tentativo identifica il tentativo di esecuzione del job che sta consegnando il lotto. Ogni scrittura
// che arriva da un worker deve esibirlo: senza, un tentativo scaduto potrebbe ancora scrivere messaggi
// e far avanzare il cursore del tentativo che gli è subentrato (precisazione P5).
type Tentativo struct {
	JobID      int64
	LeaseToken uuid.UUID
	WorkerID   string
}

// Lotto è ciò che il worker consegna in una sola chiamata: gli elementi convertiti, quelli che non è
// riuscito a leggere, il cursore raggiunto e il tentativo che li sta consegnando.
type Lotto struct {
	Casella   db.Casella
	Tentativo *Tentativo // nil = chiamata interna (replay da admin): nessun job da validare
	Messaggi  []api.MessaggioIn
	Saltati   []api.ElementoSaltato
	Cursore   *api.CursoreLotto
}

var (
	// ErrTentativoNonValido: il job non è più in corso con questo token. Chi chiama risponde 409.
	ErrTentativoNonValido = errors.New("tentativo non più valido")
	// ErrCasellaNonCensita: errore di configurazione, non di dato. Non produce scarti (I23).
	ErrCasellaNonCensita = errors.New("casella non censita")
)

// forzaErrore è il gancio di prova della voce 0.5: se COCKPIT_INGEST_FORZA_ERRORE contiene una
// sottostringa, ogni messaggio il cui Message-ID la contiene fallisce come se il DB l'avesse
// rifiutato. Serve ai test del poison pill (§8 del piano di test) per provocare un errore su un
// elemento preciso senza costruire dati corrotti a mano. Fuori dai test la variabile non esiste e la
// funzione costa un confronto con la stringa vuota.
func forzaErrore(messageID string) error {
	spia := os.Getenv("COCKPIT_INGEST_FORZA_ERRORE")
	if spia == "" || !strings.Contains(messageID, spia) {
		return nil
	}
	return fmt.Errorf("errore forzato da COCKPIT_INGEST_FORZA_ERRORE=%q (solo test)", spia)
}

// RisolviCasella traduce il casella_id del lotto in una casella viva. Va chiamata PRIMA di aprire la
// transazione: una casella sconosciuta o disattivata è un errore di configurazione che riguarda tutto
// il lotto, non un dato da scartare elemento per elemento.
func RisolviCasella(ctx context.Context, q *db.Queries, casellaID *uuid.UUID, predefinita string) (db.Casella, error) {
	var c db.Casella
	var err error
	switch {
	case casellaID != nil && *casellaID != uuid.Nil:
		c, err = q.GetCasella(ctx, *casellaID)
	case predefinita != "":
		c, err = q.GetCasellaPerIndirizzo(ctx, db.GetCasellaPerIndirizzoParams{Canale: db.CanaleOutlook, Indirizzo: predefinita})
	default:
		return c, fmt.Errorf("%w: né casella_id nel lotto né [outlook].casella_default", ErrCasellaNonCensita)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return c, fmt.Errorf("%w: %v", ErrCasellaNonCensita, casellaOId(casellaID, predefinita))
	}
	if err != nil {
		return c, err
	}
	if !c.Attiva {
		return c, fmt.Errorf("%w: %s è disattivata", ErrCasellaNonCensita, c.Indirizzo)
	}
	return c, nil
}

func casellaOId(id *uuid.UUID, predefinita string) string {
	if id != nil {
		return id.String()
	}
	return predefinita
}

// Ingerisci elabora un lotto in una sola transazione, con un savepoint per elemento.
func (s *Servizio) Ingerisci(ctx context.Context, l Lotto) (api.IngestRisposta, error) {
	out := api.IngestRisposta{Esiti: []api.EsitoMessaggio{}}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)

	// Il tentativo si verifica DENTRO la transazione e con la riga del job bloccata: così non può
	// scadere a metà scrittura e due tentativi diversi non possono scrivere lo stesso lotto insieme.
	if l.Tentativo != nil {
		_, err := q.BloccaTentativo(ctx, db.BloccaTentativoParams{
			JobID:      l.Tentativo.JobID,
			LeaseToken: uuid.NullUUID{UUID: l.Tentativo.LeaseToken, Valid: true},
			WorkerID:   pgtype.Text{String: l.Tentativo.WorkerID, Valid: true},
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return out, ErrTentativoNonValido
		}
		if err != nil {
			return out, err
		}
	}

	for i := range l.Messaggi {
		m := &l.Messaggi[i]
		sp, err := tx.Begin(ctx) // SAVEPOINT
		if err != nil {
			return out, err
		}
		esito, errEl := s.uno(ctx, db.New(sp), l.Casella, m)
		if errEl == nil {
			errEl = forzaErrore(m.MessageID)
		}
		if errEl != nil {
			// ROLLBACK TO SAVEPOINT: l'elemento sparisce, il lotto continua, la transazione resta viva
			_ = sp.Rollback(ctx)
			if err := s.scarta(ctx, q, l.Casella, m, errEl); err != nil {
				return out, fmt.Errorf("scarto di %s: %w", m.MessageID, err)
			}
			s.Log.Warn("elemento scartato", "message_id", m.MessageID, "entry_id", m.EntryID, "err", errEl)
			out.Falliti++
			out.Esiti = append(out.Esiti, api.EsitoMessaggio{MessageID: m.MessageID, Aggancio: "nessuno", Errore: errEl.Error()})
			continue
		}
		if err := sp.Commit(ctx); err != nil { // RELEASE SAVEPOINT
			return out, err
		}
		if esito.Inserito {
			out.Inseriti++
		} else {
			out.Aggiornati++
		}
		// un elemento che era in scarto ed è entrato non è più in scarto
		if _, err := q.EliminaScartoPerElemento(ctx, db.EliminaScartoPerElementoParams{CasellaID: l.Casella.CasellaID, EntryID: m.EntryID}); err != nil {
			return out, err
		}
		out.Esiti = append(out.Esiti, esito)
	}

	// elementi che il worker non è riuscito nemmeno a leggere: si registrano per poterli rileggere
	for _, sal := range l.Saltati {
		if err := s.scartaLettura(ctx, q, l.Casella, sal); err != nil {
			return out, err
		}
		out.Falliti++
	}

	// Il cursore avanza NELLA STESSA TRANSAZIONE del lotto: se il commit non riesce, il cursore non si
	// muove e il lotto viene ripetuto per intero. GREATEST impedisce che due tentativi lo facciano
	// arretrare (Q22).
	if l.Cursore != nil && l.Cursore.Cartella != "" {
		if err := q.UpsertSyncCursore(ctx, db.UpsertSyncCursoreParams{
			Cartella: l.Cursore.Cartella, UltimoReceived: &l.Cursore.UltimoReceived,
			NMessaggi: int32(out.Inseriti + out.Aggiornati),
		}); err != nil {
			return out, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return out, err
	}
	return out, nil
}

// scarta registra un elemento che il database ha rifiutato. Il payload completo resta in DB: il replay
// non deve ripassare da Outlook, che nel frattempo potrebbe non avere più l'elemento.
func (s *Servizio) scarta(ctx context.Context, q *db.Queries, c db.Casella, m *api.MessaggioIn, errEl error) error {
	payload, err := json.Marshal(m)
	if err != nil {
		return err
	}
	entry := m.EntryID
	if entry == "" {
		entry = "noentry:" + m.MessageID
	}
	ric := m.DataEvento
	_, err = q.UpsertIngestScarto(ctx, db.UpsertIngestScartoParams{
		CasellaID: c.CasellaID, EntryID: entry, Cartella: txtN(m.Cartella, 200), MessageID: txt(m.MessageID),
		RicevutoIl: &ric, Oggetto: txtN(m.Oggetto, 500), Origine: "ingest", Payload: payload,
		Errore: errEl.Error(),
	})
	return err
}

func (s *Servizio) scartaLettura(ctx context.Context, q *db.Queries, c db.Casella, sal api.ElementoSaltato) error {
	payload, err := json.Marshal(sal)
	if err != nil {
		return err
	}
	if sal.EntryID == "" {
		return errors.New("elemento saltato senza entry_id: non sarebbe rileggibile")
	}
	_, err = q.UpsertIngestScarto(ctx, db.UpsertIngestScartoParams{
		CasellaID: c.CasellaID, EntryID: sal.EntryID, Cartella: txtN(sal.Cartella, 200), MessageID: txt(sal.MessageID),
		RicevutoIl: sal.RicevutoIl, Oggetto: txtN(sal.Oggetto, 500), Origine: "lettura", Payload: payload,
		Errore: sal.Errore,
	})
	return err
}

func txt(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func txtN(s string, max int) pgtype.Text {
	if len([]rune(s)) > max {
		s = string([]rune(s)[:max])
	}
	return txt(s)
}

// naturaAllegato traduce il campo del contratto nell'enum. Un valore fuori enum non viene ricondotto
// al più vicino: sarebbe un dato inventato dal server. L'elemento va in scarto e si vede (N7).
func naturaAllegato(v string) (db.NaturaAllegato, error) {
	n := db.NaturaAllegato(strings.TrimSpace(v))
	if v == "" {
		return db.NaturaAllegatoFile, nil
	}
	if !n.Valid() {
		return "", fmt.Errorf("natura allegato non valida: %q", v)
	}
	return n, nil
}

func (s *Servizio) uno(ctx context.Context, q *db.Queries, casella db.Casella, m *api.MessaggioIn) (api.EsitoMessaggio, error) {
	esito := api.EsitoMessaggio{MessageID: m.MessageID, Aggancio: "nessuno"}
	if m.MessageID == "" {
		return esito, errors.New("message_id vuoto")
	}
	if len(m.MessageID) > MaxIdentificativo {
		return esito, fmt.Errorf("message_id di %d caratteri: oltre %d non è un identificativo utilizzabile", len(m.MessageID), MaxIdentificativo)
	}
	dir := db.Direzione(m.Direzione)
	if !dir.Valid() {
		return esito, fmt.Errorf("direzione non valida: %q", m.Direzione)
	}

	chiaveConv := m.ConversationID
	if chiaveConv == "" {
		chiaveConv = "msg:" + m.MessageID
	}
	if len(chiaveConv) > MaxIdentificativo {
		return esito, fmt.Errorf("conversation_id di %d caratteri: oltre %d", len(chiaveConv), MaxIdentificativo)
	}
	conv, err := q.UpsertConversazione(ctx, db.UpsertConversazioneParams{Canale: db.CanaleOutlook, ChiaveEsterna: chiaveConv, PrimoMessaggioIl: m.DataEvento})
	if err != nil {
		return esito, fmt.Errorf("conversazione: %w", err)
	}

	// anagrafica: buyer per indirizzo, cliente per dominio (solo in entrata)
	var buyer *db.Buyer
	var clienteID uuid.NullUUID
	indirizzo := strings.ToLower(strings.TrimSpace(m.MittenteIndirizzo))
	if dir == db.DirezioneEntrata && indirizzo != "" {
		if b, err := q.GetBuyerPerEmail(ctx, indirizzo); err == nil {
			buyer = &b
			clienteID = uuid.NullUUID{UUID: b.ClienteID, Valid: true}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return esito, err
		}
		if !clienteID.Valid {
			if i := strings.LastIndex(indirizzo, "@"); i > 0 {
				if c, err := q.GetClientePerDominio(ctx, indirizzo[i+1:]); err == nil {
					clienteID = uuid.NullUUID{UUID: c.ClienteID, Valid: true}
				} else if !errors.Is(err, pgx.ErrNoRows) {
					return esito, err
				}
			}
		}
	}

	var parent uuid.NullUUID
	if m.ParentMessageID != "" {
		if p, err := q.GetMessaggioPerChiave(ctx, db.GetMessaggioPerChiaveParams{Canale: db.CanaleOutlook, ChiaveEsterna: m.ParentMessageID}); err == nil {
			parent = uuid.NullUUID{UUID: p.MessaggioID, Valid: true}
		}
	}
	dest, _ := json.Marshal(m.Destinatari)
	if m.Destinatari == nil {
		dest = []byte("[]")
	}
	var buyerID uuid.NullUUID
	if buyer != nil {
		buyerID = uuid.NullUUID{UUID: buyer.BuyerID, Valid: true}
	}
	var imp pgtype.Int2
	if m.Importanza >= 0 && m.Importanza <= 2 {
		imp = pgtype.Int2{Int16: int16(m.Importanza), Valid: true}
	}
	row, err := q.UpsertMessaggio(ctx, db.UpsertMessaggioParams{
		Canale: db.CanaleOutlook, ChiaveEsterna: m.MessageID, ConversazioneID: conv.ConversazioneID,
		ParentMessaggioID: parent, Direzione: dir, DataEvento: m.DataEvento,
		MittenteNome: txtN(m.MittenteNome, 150), MittenteIndirizzo: txtN(indirizzo, 200), BuyerID: buyerID,
		Destinatari: dest, Oggetto: txtN(m.Oggetto, 500), CorpoTesto: txt(m.CorpoTesto), CorpoHtml: txt(m.CorpoHTML),
		Importanza: imp,
	})
	if err != nil {
		return esito, fmt.Errorf("messaggio: %w", err)
	}
	esito.MessaggioID = row.MessaggioID
	esito.Inserito = row.Inserito

	var flag pgtype.Int2
	if m.FlagStato > 0 {
		flag = pgtype.Int2{Int16: int16(m.FlagStato), Valid: true}
	}
	if err := q.UpsertMessaggioOutlook(ctx, db.UpsertMessaggioOutlookParams{
		MessaggioID: row.MessaggioID, EntryID: m.EntryID, StoreID: m.StoreID, ConversationID: txtN(m.ConversationID, 255),
		ConversationIndex: txtN(m.ConversationIndex, 600), InReplyTo: txtN(m.InReplyTo, 255), Riferimenti: m.Riferimenti,
		Cartella: txtN(m.Cartella, 200), Categorie: m.Categorie, NonLetto: m.NonLetto, FlagStato: flag,
	}); err != nil {
		return esito, fmt.Errorf("messaggio_outlook: %w", err)
	}

	// FATTO: allegati. Nessun download automatico: sul disco vanno solo i file che l'operatore chiede
	// (SPEC: staging su richiesta). Qui si registra l'allegato e la prima proposta dal solo nome file.
	var nomiAllegati []string
	for _, a := range m.Allegati {
		nat, err := naturaAllegato(a.Natura)
		if err != nil {
			return esito, fmt.Errorf("allegato %d: %w", a.Indice, err)
		}
		ext := strings.ToLower(strings.TrimPrefix(a.Estensione, "."))
		if ext == "" {
			if i := strings.LastIndex(a.NomeFile, "."); i >= 0 {
				ext = strings.ToLower(a.NomeFile[i+1:])
			}
		}
		var by pgtype.Int8
		if a.Bytes > 0 {
			by = pgtype.Int8{Int64: a.Bytes, Valid: true}
		}
		al, err := q.UpsertAllegato(ctx, db.UpsertAllegatoParams{
			MessaggioID: row.MessaggioID, Indice: int16(a.Indice), NomeFile: txtN(a.NomeFile, 300).String,
			Estensione: txtN(ext, 10), ContentType: txtN(a.ContentType, 120), Natura: nat, Origine: db.OrigineAllegatoOutlook,
			Bytes: by, RicevutoIl: m.DataEvento,
		})
		if err != nil {
			return esito, fmt.Errorf("allegato %d: %w", a.Indice, err)
		}
		if nat == db.NaturaAllegatoFile || nat == db.NaturaAllegatoElementoOutlook {
			nomiAllegati = append(nomiAllegati, a.NomeFile)
			pr := domain.PropostaDaNome(a.NomeFile, a.Bytes, m.Direzione)
			dett, _ := json.Marshal(map[string]any{"estensione": ext, "bytes": a.Bytes, "pre_spunta": pr.PreSpunta})
			if err := q.InsertPropostaSeAssente(ctx, db.InsertPropostaSeAssenteParams{
				AllegatoID: al.AllegatoID, ThreadID: row.ThreadID, TipoProposto: db.TipoDocumento(pr.Tipo), Codice: txtN(pr.Codice, 60),
				Rev: txtN(pr.Rev, 10), Confidenza: int16(pr.Confidenza), Fonte: db.FonteProposta(pr.Fonte), Dettagli: dett,
			}); err != nil {
				return esito, fmt.Errorf("proposta allegato %d: %w", a.Indice, err)
			}
		}
	}

	// aggancio automatico, solo per messaggi nuovi ancora orfani
	codici := domain.EstraiCodici(append([]string{m.Oggetto, m.CorpoTesto}, senzaEstensione(nomiAllegati)...)...)
	threadID := row.ThreadID
	if row.Inserito && !row.ThreadID.Valid {
		if tid, ok, err := s.threadPerConversazione(ctx, q, conv); err != nil {
			return esito, err
		} else if ok {
			threadID = uuid.NullUUID{UUID: tid, Valid: true}
			esito.Aggancio = string(db.AggancioAutoConversazione)
		} else if clienteID.Valid {
			for _, c := range codici {
				tid, err := q.ThreadPerCodiceCliente(ctx, db.ThreadPerCodiceClienteParams{ClienteID: clienteID.UUID, Upper: c})
				if err == nil {
					threadID = uuid.NullUUID{UUID: tid, Valid: true}
					esito.Aggancio = string(db.AggancioAutoIdentificativo)
					break
				}
				if !errors.Is(err, pgx.ErrNoRows) {
					return esito, err
				}
			}
		}
		if threadID.Valid {
			if err := q.AgganciaMessaggio(ctx, db.AgganciaMessaggioParams{MessaggioID: row.MessaggioID, ThreadID: threadID, Aggancio: db.Aggancio(esito.Aggancio)}); err != nil {
				return esito, err
			}
			// ogni decisione di aggancio lascia una traccia: qui l'autore è il sistema (utente NULL)
			if err := q.InsertAgganciaLog(ctx, db.InsertAgganciaLogParams{
				MessaggioID: row.MessaggioID, ThreadID: threadID, Azione: "aggancia", Motivo: txt(esito.Aggancio),
			}); err != nil {
				return esito, fmt.Errorf("log aggancio: %w", err)
			}
			if !conv.ThreadID.Valid {
				_ = q.CollegaConversazione(ctx, db.CollegaConversazioneParams{ConversazioneID: conv.ConversazioneID, ThreadID: threadID, CollegataDa: db.Aggancio(esito.Aggancio)})
			}
		}
	} else if row.ThreadID.Valid {
		esito.Aggancio = string(row.Aggancio)
	}
	if threadID.Valid {
		t := threadID.UUID
		esito.ThreadID = &t
	}

	// INTERPRETAZIONE: riferimenti portale e triage deterministico (solo alla prima vista del messaggio)
	if row.Inserito {
		for _, r := range domain.RilevaPortale(m.CorpoTesto) {
			cod := r.Codici
			if len(cod) == 0 {
				cod = []string{""}
			}
			for _, c := range cod {
				if _, err := q.UpsertRiferimentoPortale(ctx, db.UpsertRiferimentoPortaleParams{
					MessaggioID: row.MessaggioID, ThreadID: threadID, Codice: txtN(c, 60), Url: txtN(r.URL, 500), TestoCitato: txtN(r.TestoCitato, 1000).String,
				}); err != nil {
					return esito, fmt.Errorf("riferimento_portale: %w", err)
				}
			}
		}
		if !threadID.Valid && dir == db.DirezioneEntrata {
			tr := domain.Triage(domain.IngressoTriage{
				Oggetto: m.Oggetto, Corpo: m.CorpoTesto, NomiAllegati: nomiAllegati, Direzione: m.Direzione,
				ClienteNoto: clienteID.Valid, BuyerNoto: buyer != nil, ConversazioneNota: conv.ThreadID.Valid,
			})
			motivi, _ := json.Marshal(tr.Motivi)
			if tr.Codici == nil {
				tr.Codici = []string{}
			}
			var scad *time.Time
			if d, ok := domain.RilevaScadenza(m.CorpoTesto, m.DataEvento); ok {
				scad = &d
			}
			if _, err := q.UpsertTriage(ctx, db.UpsertTriageParams{
				MessaggioID: row.MessaggioID, Esito: db.EsitoTriage(tr.Esito), ClienteProposto: clienteID, BuyerProposto: buyerID,
				Identificativi: tr.Codici, ScadenzaProposta: scad, Confidenza: int16(tr.Confidenza), Motivi: motivi, Fonte: db.FonteTriageDeterministico,
			}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return esito, fmt.Errorf("triage: %w", err)
			}
		}
	}
	return esito, nil
}

func (s *Servizio) threadPerConversazione(ctx context.Context, q *db.Queries, conv db.Conversazione) (uuid.UUID, bool, error) {
	if conv.ThreadID.Valid {
		return conv.ThreadID.UUID, true, nil
	}
	tid, err := q.ThreadDellaConversazione(ctx, conv.ConversazioneID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return tid.UUID, tid.Valid, nil
}

func senzaEstensione(nomi []string) []string {
	out := make([]string, 0, len(nomi))
	for _, n := range nomi {
		if i := strings.LastIndex(n, "."); i > 0 {
			n = n[:i]
		}
		out = append(out, n)
	}
	return out
}
