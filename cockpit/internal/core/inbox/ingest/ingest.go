// Package ingest scrive il FATTO (messaggio, messaggio_outlook, allegato) e produce le prime
// INTERPRETAZIONI: candidati di aggancio, candidati di codice, riferimenti al portale, proposta di triage.
//
// Non aggancia niente. Dal checkpoint 3R `messaggio.thread_id` non viene scritto qui in nessun caso:
// l'ingest propone e basta, e la decisione è un bottone premuto da un operatore (D9). Non scrive mai
// sul NAS.
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
	"bytes"
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

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// MaxIdentificativo: oltre questa lunghezza un Message-ID non è più un identificativo utilizzabile.
// Troncarlo sarebbe peggio che scartarlo: due messaggi diversi diventerebbero lo stesso (N34).
const MaxIdentificativo = 1000

type Servizio struct {
	Pool *pgxpool.Pool
	Log  *slog.Logger

	// PrimaDelCommit, se valorizzato, viene chiamato con la transazione del lotto ancora aperta,
	// dopo gli elementi e dopo il cursore, subito prima del COMMIT. In produzione è nil.
	//
	// Non è una comodità: è l'unico modo di dimostrare due proprietà che il piano richiede e che
	// dall'esterno sarebbero invisibili. (1) §8.1 punto 3: una seconda connessione, mentre il gancio
	// è dentro, non deve vedere NIENTE — né messaggi, né scarti, né cursore. (2) I16: se il commit
	// non riesce, la risposta è 5xx e in database non resta nulla di parziale; il gancio può abortire
	// la transazione con un errore SQL vero, così il COMMIT fallisce davvero e non per finta.
	PrimaDelCommit func(context.Context, pgx.Tx) error

	// StagingAutomatico e StagingMaxByte: D30 (checkpoint 3R §5). Quando è acceso, gli allegati di un
	// mittente RICONOSCIUTO, sotto la soglia, scendono nello staging del server appena il messaggio
	// entra — senza che nessuno prema «Scarica» — così l'analisi può dire che cosa sono.
	//
	// Il motivo è che il tipo di un PDF si sa solo aprendolo, e finché non lo si apre ogni allegato si
	// presenta come «PDF · da determinare»: per sapere se vale la pena scaricarlo bisognava
	// scaricarlo. Resta spento se non lo si accende nel file di configurazione, e non tocca il NAS:
	// lo staging è una cartella del server.
	StagingAutomatico bool
	StagingMaxByte    int64

	// StagingBootstrap: se lo staging automatico vale anche alla PRIMA sincronizzazione di una
	// (casella, cartella), quella che porta dentro settimane di posta in un colpo solo. Assente =
	// false, ed e' il default giusto: un bootstrap e' l'unico momento in cui «gli allegati recenti»
	// sono migliaia, e accenderlo di fatto e' l'ordine di scaricare l'archivio.
	//
	// Non esiste la voce corrispondente per lo STORICO. «Carica precedenti» serve a rendere
	// consultabile la posta vecchia; scaricarne gli allegati, scompattarli e analizzarli non e' una
	// preferenza — e' il difetto che questo blocco toglie, e una voce di configurazione sarebbe il
	// modo di rimetterlo.
	StagingBootstrap bool
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
	Messaggi  []worker.MessaggioIn
	Saltati   []worker.ElementoSaltato
	Cursore   *worker.CursoreLotto
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

// ---------------------------------------------------------------- di chi è questo indirizzo

// Nostri è l'insieme dei domini delle caselle censite. Da qui il SERVER decide la direzione di un
// messaggio e se è traffico interno (voce 2.1), invece di fidarsi di ciò che il worker deduce dal
// proprio profilo Outlook.
//
// Il worker sa due cose, e sono due cose fragili: «questa cartella è la Posta inviata del profilo» e
// «questo indirizzo è fra i miei». Su una casella condivisa la prima è ambigua (la Posta inviata di
// chi?) e la seconda dipende da come il profilo è configurato su QUEL PC. La direzione sbagliata non
// è un dettaglio: cambia la lettura dell'intero messaggio — niente triage, nessun cliente
// riconosciuto, proposte diverse sugli allegati — e il difetto viaggia fino al fascicolo.
// Le caselle censite, invece, sono un elenco dichiarato e uguale per tutti i worker.
type Nostri map[string]bool

// CaricaNostri legge i domini delle caselle censite. Una volta per lotto: sono poche righe e non
// cambiano durante un sync.
func CaricaNostri(ctx context.Context, q *db.Queries) (Nostri, error) {
	domini, err := q.DominiNostri(ctx)
	if err != nil {
		return nil, err
	}
	n := make(Nostri, len(domini))
	for _, d := range domini {
		if d = strings.TrimSpace(strings.ToLower(d)); d != "" {
			n[d] = true
		}
	}
	return n, nil
}

// Nostro dice se l'indirizzo appartiene a un dominio che è nostro.
// Motori è la cache per lotto dei motori di riconoscimento (voce 6.11).
//
// Le regole di un cliente sono regex, e compilarle costa. Un lotto di duecento messaggi che
// vengono quasi tutti dagli stessi tre o quattro clienti compilerebbe le stesse espressioni
// duecento volte: la cache vive quanto il lotto, quindi una regola cambiata in Anagrafica vale dal
// lotto successivo — che è dopo pochi secondi — senza che nessuno debba invalidare niente.
type Motori struct {
	per          map[uuid.UUID]*classificazione.Motore
	perFornitore map[uuid.UUID]*classificazione.Motore
}

func NuoviMotori() *Motori {
	return &Motori{per: map[uuid.UUID]*classificazione.Motore{}, perFornitore: map[uuid.UUID]*classificazione.Motore{}}
}

// PerFornitore e' il motore per la posta di un fornitore (7B, IB8): l'unione delle famiglie di
// codice dei clienti che hanno richieste aperte a lui. Niente riferimento RFQ, niente frasi
// portale: sono cose dei clienti. Senza richieste aperte il motore e' vuoto e — poiche' nel ramo
// fornitore contano solo i codici di famiglia — non si estrae nessun codice: un materiale o una
// norma citati da un fornitore non diventano mai «codici trovati».
func (m *Motori) PerFornitore(ctx context.Context, q *db.Queries, id uuid.UUID) *classificazione.Motore {
	if mo, ok := m.perFornitore[id]; ok {
		return mo
	}
	var raccolte regole.Regole
	if clienti, err := q.ClientiConRichiesteAlFornitore(ctx, id); err == nil {
		for _, c := range clienti {
			r, _ := regole.LeggiRegole(c.Regole)
			for _, f := range r.FamiglieCodice {
				if f.Descrizione != "" {
					f.Descrizione = c.CartellaNas + ": " + f.Descrizione
				}
				raccolte.FamiglieCodice = append(raccolte.FamiglieCodice, f)
			}
		}
	}
	mo := classificazione.Compila("fornitore", raccolte)
	m.perFornitore[id] = mo
	return mo
}

// Per restituisce il motore del cliente. Un cliente senza regole dà un motore vuoto e non nil:
// un motore vuoto fa comunque funzionare l'estrattore generico, ed è il caso normale finché
// l'anagrafica non è compilata.
func (m *Motori) Per(ctx context.Context, q *db.Queries, id uuid.UUID) *classificazione.Motore {
	if mo, ok := m.per[id]; ok {
		return mo
	}
	var mo *classificazione.Motore
	if c, err := q.GetCliente(ctx, id); err == nil {
		lette, _ := regole.LeggiRegole(c.Regole)
		mo = classificazione.Compila(c.RagioneSociale, lette)
	}
	m.per[id] = mo
	return mo
}

func (n Nostri) Nostro(indirizzo string) bool {
	i := strings.LastIndex(indirizzo, "@")
	if i < 0 || i == len(indirizzo)-1 {
		return false
	}
	return n[strings.ToLower(indirizzo[i+1:])]
}

// DirezioneEInterno decide le due cose che il piano tiene separate (D10).
//
//   - DIREZIONE: «uscita» se il messaggio è partito da un nostro indirizzo, «entrata» altrimenti. È
//     una proprietà del MESSAGGIO, non della copia: vale uguale in tutte le caselle in cui arriva, e
//     non dipende da quale casella ha sincronizzato per prima. La consolidazione nell'upsert («uscita
//     vince») serve solo a non perdere il fatto se una copia dovesse risultare diversa.
//   - INTERNO: mittente e TUTTI i destinatari sono nostri. È il collega che gira una mail al collega:
//     tecnicamente parte da noi, ma non è traffico con il cliente. Senza un flag separato finirebbe
//     confuso con un'offerta inviata, e un terzo valore nell'enum `direzione` avrebbe costretto a
//     rispondere insieme a due domande che restano distinte.
//
// `dichiarata` è ciò che dice il worker e resta la riserva: quando l'elenco delle caselle non aiuta
// (indirizzo vuoto, o un indirizzo Exchange in forma di DN senza chiocciola) si usa quella, che è
// esattamente ciò che si faceva prima di questa voce.
func (n Nostri) DirezioneEInterno(mittente string, destinatari []worker.Destinatario, dichiarata db.Direzione) (db.Direzione, bool) {
	mittente = strings.ToLower(strings.TrimSpace(mittente))
	if len(n) == 0 || !strings.Contains(mittente, "@") {
		return dichiarata, false
	}
	if !n.Nostro(mittente) {
		return db.DirezioneEntrata, false
	}
	interno := len(destinatari) > 0
	for _, d := range destinatari {
		if !n.Nostro(strings.ToLower(strings.TrimSpace(d.Indirizzo))) {
			interno = false
			break
		}
	}
	return db.DirezioneUscita, interno
}

// Ingerisci elabora un lotto in una sola transazione, con un savepoint per elemento.
func (s *Servizio) Ingerisci(ctx context.Context, l Lotto) (worker.IngestRisposta, error) {
	out := worker.IngestRisposta{Esiti: []worker.EsitoMessaggio{}}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)

	// I domini nostri si leggono una volta per lotto: servono a ogni elemento per decidere direzione
	// e `interno` (voce 2.1), e sono le stesse poche righe per tutto il sync.
	nostri, err := CaricaNostri(ctx, q)
	if err != nil {
		return out, err
	}
	motori := NuoviMotori()

	// Il tentativo si verifica DENTRO la transazione e con la riga del job bloccata: così non può
	// scadere a metà scrittura e due tentativi diversi non possono scrivere lo stesso lotto insieme.
	var job *db.Job
	if l.Tentativo != nil {
		j, err := q.BloccaTentativo(ctx, db.BloccaTentativoParams{
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
		job = &j
	}

	// Il modo del sync si legge dal JOB, non dal lotto. La differenza conta: il payload lo ha scritto
	// il server all'accodamento, e la riga e' bloccata qui sopra. Un campo nella richiesta del worker
	// sarebbe invece una dichiarazione di chi esegue — e basterebbe un worker vecchio, o un worker
	// riscritto male, per far passare un «Carica precedenti» come aggiornamento e riportare dentro il
	// difetto insieme a mille download.
	scendono, perche := s.scendonoDaSoli(job)
	if !scendono && s.StagingAutomatico && s.Log != nil && len(l.Messaggi) > 0 {
		s.Log.Info("staging automatico sospeso per questo lotto", "motivo", perche, "messaggi", len(l.Messaggi))
	}

	for i := range l.Messaggi {
		m := &l.Messaggi[i]
		sp, err := tx.Begin(ctx) // SAVEPOINT
		if err != nil {
			return out, err
		}
		esito, errEl := s.uno(ctx, db.New(sp), l.Casella, nostri, motori, m, scendono)
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
			out.Esiti = append(out.Esiti, worker.EsitoMessaggio{MessageID: m.MessageID, Aggancio: "nessuno", Errore: errEl.Error()})
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
		switch {
		case worker.NelFuturo(l.Cursore.UltimoReceived, time.Now()):
			// Il cursore lo calcola il worker sulle stesse date dei messaggi: se quelle sono nel futuro
			// lo è anche lui, e scriverlo significherebbe aprire la prossima finestra dopo l'orologio e
			// non leggere più niente finché il futuro non è passato (16/09/2026). Fermo dov'è, la
			// finestra viene riletta: costa una rilettura, che è l'errore che si corregge da solo.
			s.Log.Warn("cursore nel futuro: non avanza", "casella", l.Casella.Indirizzo, "cartella", l.Cursore.Cartella,
				"ultimo_received", l.Cursore.UltimoReceived.UTC().Format(time.RFC3339), "tolleranza", worker.TolleranzaFuturo)
		default:
			// Il cursore è di QUESTA casella: la cartella da sola non basta più. Due caselle con una
			// «Posta in arrivo» scrivevano sulla stessa riga e si facevano avanzare il cursore a
			// vicenda, e un cursore avanzato di troppo è una finestra che l'altra casella non rilegge
			// mai (2.1).
			if err := q.UpsertSyncCursore(ctx, db.UpsertSyncCursoreParams{
				CasellaID: l.Casella.CasellaID, Cartella: l.Cursore.Cartella, UltimoReceived: &l.Cursore.UltimoReceived,
				NMessaggi: int32(out.Inseriti + out.Aggiornati),
			}); err != nil {
				return out, err
			}
		}
	}

	if s.PrimaDelCommit != nil {
		if err := s.PrimaDelCommit(ctx, tx); err != nil {
			return worker.IngestRisposta{Esiti: []worker.EsitoMessaggio{}}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		// Il commit non è riuscito: niente di questo lotto è durevole, nemmeno gli elementi che erano
		// andati a buon fine. La risposta deve essere un errore (5xx) e mai un 200 parziale, perché il
		// worker decide se ripetere il lotto proprio da lì. Si azzerano anche i conteggi: riportare
		// «inseriti 2» dopo un commit fallito sarebbe una bugia sul contenuto del database (I16).
		return worker.IngestRisposta{Esiti: []worker.EsitoMessaggio{}}, fmt.Errorf("commit del lotto: %w", err)
	}
	return out, nil
}

// scarta registra un elemento che il database ha rifiutato. Il payload completo resta in DB: il replay
// non deve ripassare da Outlook, che nel frattempo potrebbe non avere più l'elemento.
func (s *Servizio) scarta(ctx context.Context, q *db.Queries, c db.Casella, m *worker.MessaggioIn, errEl error) error {
	payload, err := payloadScarto(m)
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

func (s *Servizio) scartaLettura(ctx context.Context, q *db.Queries, c db.Casella, sal worker.ElementoSaltato) error {
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

// payloadScarto serializza l'elemento per la colonna jsonb dello scarto.
//
// Non è un json.Marshal e basta. Un elemento finisce in scarto proprio perché ha qualcosa che il
// database rifiuta, e il caso più frequente — un byte NUL nel corpo, che Outlook produce con certi
// messaggi malformati — sarebbe rifiutato una seconda volta qui: PostgreSQL non accetta \u0000
// nemmeno dentro jsonb (SQLSTATE 22P05). Il risultato sarebbe il peggiore possibile: l'errore dello
// scarto abortisce la transazione del lotto, quindi un solo messaggio rotto farebbe di nuovo fallire
// tutti gli altri — cioè esattamente il poison pill che questa fase deve eliminare.
//
// I byte NUL vengono quindi sostituiti con U+FFFD, il carattere che significa «qui c'era qualcosa di
// non rappresentabile». Il payload resta fedele in tutto il resto e il replay funziona.
func payloadScarto(m *worker.MessaggioIn) ([]byte, error) {
	grezzo, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	if !bytes.Contains(grezzo, []byte{0}) && !bytes.Contains(grezzo, []byte(`\u0000`)) {
		return grezzo, nil
	}
	var albero any
	if err := json.Unmarshal(grezzo, &albero); err != nil {
		return nil, err
	}
	return json.Marshal(senzaNul(albero))
}

// senzaNul ripulisce ricorsivamente le stringhe di un albero JSON decodificato. Si lavora sull'albero
// e non sul testo serializzato perché in JSON \u0000 è indistinguibile, a colpo d'occhio, dalla
// sequenza letterale «\u0000» che un utente può aver scritto nel corpo: sostituirla nel testo
// corromperebbe un messaggio legittimo.
func senzaNul(v any) any {
	switch t := v.(type) {
	case string:
		return strings.ReplaceAll(t, "\x00", "�")
	case []any:
		for i := range t {
			t[i] = senzaNul(t[i])
		}
		return t
	case map[string]any:
		for k, el := range t {
			t[k] = senzaNul(el)
		}
		return t
	}
	return v
}

// RicevutoIn è il ReceivedTime dell'elemento nella casella: il valore su cui avanza il cursore.
//
// Chiude W2. Il cursore avanzava su `data_evento`, che per la Posta inviata è SentOn, mentre il
// filtro della scansione usa ReceivedTime: due grandezze diverse spacciate per una. Per la Posta in
// arrivo coincidono e il difetto non si vede mai; per la Posta inviata no, e una mail scritta lunedì
// e inviata giovedì può spingere il cursore oltre elementi che nessuno ha ancora letto — persi senza
// che niente lo segnali.
//
// Un worker che non manda ancora `ricevuto_il` continua a funzionare com'era: si usa `data_evento`.
// Preferire un valore assente a uno sbagliato non è prudenza generica: fra i due, quello che rompe
// l'ordinamento del cursore è il secondo.
func RicevutoIn(m *worker.MessaggioIn) time.Time {
	if m.RicevutoIl != nil && !m.RicevutoIl.IsZero() {
		return *m.RicevutoIl
	}
	return m.DataEvento
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

// sogliaStaging è la dimensione massima per lo staging automatico (D30), con il valore predefinito
// quando la configurazione non lo dice.
func (s *Servizio) sogliaStaging() int64 {
	if s.StagingMaxByte > 0 {
		return s.StagingMaxByte
	}
	return classificazione.SogliaStagingAutomatico
}

// scendonoDaSoli dice se gli allegati di questo lotto possono finire in staging senza che nessuno
// abbia premuto «Scarica», e perche' no quando la risposta e' no (la frase finisce nel log).
//
// `j` e' il job che sta consegnando il lotto: nil quando a chiamare e' l'amministratore che riprova
// uno scarto, cioe' un gesto di una persona su un elemento solo.
func (s *Servizio) scendonoDaSoli(j *db.Job) (bool, string) {
	if !s.StagingAutomatico {
		return false, "staging automatico spento"
	}
	if j == nil || j.Tipo != db.TipoJobSyncOutlook {
		return true, "richiesta puntuale"
	}
	var p worker.PayloadSyncOutlook
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		// Un payload illeggibile non e' un permesso: qui si decide se aprire Outlook e scaricare file,
		// e «non ho capito di che sync si tratta» deve valere no.
		return false, "payload del job illeggibile"
	}
	switch p.ModoEffettivo() {
	case worker.ModoStorico:
		return false, "storico: «Carica precedenti» rende consultabile la posta vecchia, non ne scarica gli allegati"
	case worker.ModoBootstrap:
		if !s.StagingBootstrap {
			return false, "bootstrap: prima sincronizzazione della casella (staging.bootstrap = false)"
		}
		return true, worker.ModoBootstrap
	default:
		return true, worker.ModoAggiornamento
	}
}

func (s *Servizio) uno(ctx context.Context, q *db.Queries, casella db.Casella, nostri Nostri, motori *Motori, m *worker.MessaggioIn, scendonoDaSoli bool) (worker.EsitoMessaggio, error) {
	esito := worker.EsitoMessaggio{MessageID: m.MessageID, Aggancio: "nessuno"}
	if m.MessageID == "" {
		return esito, errors.New("message_id vuoto")
	}
	if len(m.MessageID) > MaxIdentificativo {
		return esito, fmt.Errorf("message_id di %d caratteri: oltre %d non è un identificativo utilizzabile", len(m.MessageID), MaxIdentificativo)
	}
	// La direzione dichiarata dal worker viene comunque validata prima di essere sostituita: un
	// valore fuori enum è un difetto del worker, e lasciarlo passare in silenzio perché tanto il
	// server ricalcola vorrebbe dire nasconderlo per sempre (N7). Poi decide il server (voce 2.1).
	dichiarata := db.Direzione(m.Direzione)
	if !dichiarata.Valid() {
		return esito, fmt.Errorf("direzione non valida: %q", m.Direzione)
	}
	// Un messaggio che dichiara di essere arrivato nel futuro non entra: finisce in scarto con il
	// payload intero, e il cursore resta dov'è (vedi Ingerisci). Scartarlo non è pignoleria sul dato:
	// è `ricevuto_il` che fa avanzare il cursore, quindi accettarlo vorrebbe dire spostare la
	// finestra di lettura oltre l'orologio e smettere di leggere la posta. Quando l'ora torna
	// plausibile — worker aggiornato, o orologio sistemato — lo stesso elemento rientra dal replay o
	// dal sync successivo, e lo scarto sparisce da solo.
	if ric := RicevutoIn(m); worker.NelFuturo(ric, time.Now()) {
		return esito, fmt.Errorf("ricevuto_il nel futuro: %s, oltre la tolleranza di %s sull'ora del server. "+
			"O l'orologio del PC del worker è avanti, o le date di Outlook stanno arrivando come ora locale "+
			"etichettata UTC (correzione del 16/09/2026 in outlook_com._utc)",
			ric.UTC().Format(time.RFC3339), worker.TolleranzaFuturo)
	}
	dir, interno := nostri.DirezioneEInterno(m.MittenteIndirizzo, m.Destinatari, dichiarata)

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

	// Blocco 7A (D33): chi c'e' dall'altra parte lo dice il resolver, con la precedenza contatto >
	// dominio > sconosciuto e `ambiguo` se doppia. Il cliente per il triage e per le regole viene da
	// li': un mittente riconosciuto come FORNITORE non ha un cliente, e non puo' averlo.
	indirizzo := strings.ToLower(strings.TrimSpace(m.MittenteIndirizzo))
	controparte, err := risolviControparte(ctx, q, nostri, m.MittenteIndirizzo, indirizziDi(m.Destinatari))
	if err != nil {
		return esito, err
	}
	buyer, clienteID, err := clienteDallaControparte(ctx, q, controparte, dir)
	if err != nil {
		return esito, err
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
		Importanza: imp, Interno: interno,
	})
	if err != nil {
		return esito, fmt.Errorf("messaggio: %w", err)
	}
	esito.MessaggioID = row.MessaggioID
	esito.Inserito = row.Inserito
	if _, err := q.SetControparteMessaggio(ctx, parametriControparte(row.MessaggioID, controparte)); err != nil {
		return esito, fmt.Errorf("controparte: %w", err)
	}
	// Blocco 7B: i marcatori scritti dal Cockpit sulla bozza. CockpitRichiestaFornitore lega la nostra
	// mail alla richiesta e la aggancia alla RFQ senza euristiche; CockpitBozza chiude la bozza.
	if agganciato, err := s.applicaMarcatori(ctx, q, &row, m, dir); err != nil {
		return esito, err
	} else if agganciato {
		esito.Aggancio = string(row.Aggancio)
	}

	var flag pgtype.Int2
	if m.FlagStato > 0 {
		flag = pgtype.Int2{Int16: int16(m.FlagStato), Valid: true}
	}
	// Ciò che è del MESSAGGIO (la catena di conversazione) e ciò che è della COPIA (dove sta, com'è
	// stato letto) vanno in due tabelle diverse dalla 0004. Prima stavano insieme, e la seconda
	// casella sovrascriveva l'EntryID della prima: da quel momento «Apri in Outlook» apriva
	// l'elemento sbagliato, o non lo trovava, e nessuno collegava le due cose.
	if err := q.UpsertMessaggioOutlook(ctx, db.UpsertMessaggioOutlookParams{
		MessaggioID: row.MessaggioID, ConversationID: txtN(m.ConversationID, 255),
		ConversationIndex: txtN(m.ConversationIndex, 600), InReplyTo: txtN(m.InReplyTo, 255), Riferimenti: m.Riferimenti,
	}); err != nil {
		return esito, fmt.Errorf("messaggio_outlook: %w", err)
	}
	if m.EntryID == "" {
		return esito, errors.New("entry_id vuoto: senza non si ritrova l'elemento nella casella")
	}
	if err := q.UpsertPresenza(ctx, db.UpsertPresenzaParams{
		MessaggioID: row.MessaggioID, CasellaID: casella.CasellaID, EntryID: m.EntryID,
		Cartella: txtN(m.Cartella, 200), RicevutoIl: RicevutoIn(m), NonLetto: m.NonLetto,
		FlagStato: flag, Categorie: m.Categorie,
	}); err != nil {
		return esito, fmt.Errorf("presenza in %s: %w", casella.Indirizzo, err)
	}

	// FATTO: allegati. Nessun download automatico: sul disco vanno solo i file che l'operatore chiede
	// (SPEC: staging su richiesta). Qui si registra l'allegato e la prima proposta dal solo nome file.
	// Due allegati con lo stesso indice nello stesso messaggio non sono un doppione innocuo: l'upsert
	// di `allegato` ha come chiave (messaggio_id, contenitore_id, indice), quindi il secondo
	// sovrascriverebbe il primo e un allegato sparirebbe in silenzio. Meglio uno scarto visibile che
	// un messaggio acquisito senza uno dei suoi file (voce 1.11).
	visti := make(map[int]string, len(m.Allegati))
	for _, a := range m.Allegati {
		if gia, dup := visti[a.Indice]; dup {
			return esito, fmt.Errorf("due allegati con lo stesso indice %d (%q e %q): il secondo sovrascriverebbe il primo", a.Indice, gia, a.NomeFile)
		}
		visti[a.Indice] = a.NomeFile
	}

	var nomiAllegati []string
	var daStaggiare []db.Allegato // D30: allegati che scendono da soli, se lo staging automatico è acceso
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
			pr := classificazione.PropostaDaNome(a.NomeFile, a.Bytes, string(dir))
			if nat == db.NaturaAllegatoFile && pr.PreSpunta && a.Bytes > 0 && a.Bytes <= s.sogliaStaging() {
				daStaggiare = append(daStaggiare, al)
			}
			dettagli := map[string]any{"estensione": ext, "bytes": a.Bytes, "pre_spunta": pr.PreSpunta}
			if len(pr.CodiciNelNome) > 0 {
				dettagli["codici_nel_nome"] = pr.CodiciNelNome
			}
			dett, _ := json.Marshal(dettagli)
			// Niente troncamento a 60 e a 10 (7C.1, P0): il dominio garantisce che un codice stia in
			// MaxCodice e una revisione in MaxRev, e un valore che non ci sta non e' un codice — non
			// si accorcia in silenzio, si lascia fuori (PropostaDaNome lo mette in CodiciNelNome).
			if err := q.InsertPropostaSeAssente(ctx, db.InsertPropostaSeAssenteParams{
				AllegatoID: al.AllegatoID, ThreadID: row.ThreadID, TipoProposto: db.TipoDocumento(pr.Tipo), Codice: txt(pr.Codice),
				Rev: txt(pr.Rev), Confidenza: int16(pr.Confidenza), Fonte: db.FonteProposta(pr.Fonte), Dettagli: dett,
			}); err != nil {
				return esito, fmt.Errorf("proposta allegato %d: %w", a.Indice, err)
			}
		}
	}

	// Le regole del cliente riconosciuto (voce 6.11): le famiglie di codice dicono che cosa è un
	// codice DI QUESTO cliente, mentre l'estrattore generico dice solo che cosa ha la forma di un
	// codice. Cliente sconosciuto o senza regole → motore nil, e vale il solo generico. Per un
	// fornitore, le famiglie dei clienti che gli hanno mandato richieste (7B).
	motore := motorePer(ctx, q, motori, clienteID, controparte)

	// NESSUN AGGANCIO AUTOMATICO (checkpoint 3R §2). Qui prima c'era un blocco che, per un messaggio
	// nuovo e orfano, scriveva `messaggio.thread_id` se il ConversationID coincideva con quello di una
	// conversazione già agganciata, oppure se un codice qualunque coincideva con un identificativo di
	// una RFQ dello stesso cliente. Erano due decisioni prese da una coincidenza.
	//
	// «Rispondi» su una mail vecchia per parlare d'altro conserva il ConversationID; l'estrattore
	// generico chiamava «codice» qualunque numero con tre cifre. Il messaggio finiva nella RFQ
	// sbagliata, e ci restava, perché un aggancio non si annulla da solo. Ora quelle stesse
	// informazioni diventano CANDIDATI, e chi decide è l'operatore (D9).
	threadID := row.ThreadID
	if row.ThreadID.Valid {
		esito.Aggancio = string(row.Aggancio)
		t := threadID.UUID
		esito.ThreadID = &t
	}

	// D30: staging automatico. Solo alla prima vista, solo da un mittente RICONOSCIUTO, solo sotto la
	// soglia, e solo per ciò che varrebbe comunque la pena scaricare. «Riconosciuto» è la condizione
	// che tiene: senza, la prima newsletter con un PDF allegato farebbe partire un download.
	//
	// E solo se QUESTO LOTTO può farlo (`scendonoDaSoli`). Le quattro condizioni qui sotto guardano il
	// messaggio; nessuna di loro guarda il motivo per cui il messaggio sta entrando adesso. Finché
	// mancava, «Carica precedenti» — che serve a rendere consultabile la posta vecchia — scaricava
	// tutti gli zip di due giorni di archivio, li scompattava e accodava l'analisi di ogni disegno che
	// c'era dentro: un clic per leggere il passato diventava ore di lavoro e qualche giga di disco.
	//
	// Un errore qui non deve far cadere l'ingest dell'elemento: il messaggio è un FATTO ed è già
	// scritto, mentre lo staging è una comodità. Viene registrato e basta.
	if scendonoDaSoli && row.Inserito && clienteID.Valid && (dir == db.DirezioneEntrata || interno) {
		for _, a := range daStaggiare {
			esitoStage, _, err := jobs.AccodaStage(ctx, q, jobs.FileStaging{}, a,
				db.Messaggio{MessaggioID: row.MessaggioID, ChiaveEsterna: m.MessageID},
				jobs.Copia{CasellaID: casella.CasellaID, EntryID: m.EntryID}, 3)
			if err != nil && s.Log != nil {
				s.Log.Warn("staging automatico non riuscito", "allegato", a.NomeFile, "errore", err)
			}
			if err == nil && s.Log != nil {
				s.Log.Debug("staging automatico", "allegato", a.NomeFile, "esito", esitoStage)
			}
		}
	}

	// INTERPRETAZIONE: riferimenti portale e triage deterministico (solo alla prima vista del messaggio)
	if row.Inserito {
		for _, r := range classificazione.RilevaPortale(m.CorpoTesto, motore.FrasiPortale()...) {
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
		// Il triage vale per ciò che ARRIVA a noi: la posta in entrata e la posta interna. Una mail
		// interna è in uscita per definizione (parte da un nostro indirizzo), ma «te la giro» è uno
		// dei modi in cui una richiesta arriva davvero sul tavolo: escluderla perché il mittente è un
		// collega significherebbe non proporre niente proprio sui messaggi che qualcuno ha inoltrato
		// apposta perché qualcun altro li guardasse.
		if !threadID.Valid && daInterpretare(dir, interno, controparte) {
			if _, err := s.interpreta(ctx, q, interpretazione{
				MessaggioID: row.MessaggioID, ConversazioneID: conv.ConversazioneID,
				ClienteID: clienteID, BuyerID: buyerID, Controparte: controparte,
				Oggetto: m.Oggetto, Corpo: m.CorpoTesto, NomiAllegati: nomiAllegati,
				Direzione: dir, Interno: interno, InReplyTo: m.InReplyTo, Riferimenti: m.Riferimenti,
				DataEvento: m.DataEvento, Motore: motore, Mittente: indirizzo, FornitoreID: controparte.FornitoreID,
			}); err != nil {
				return esito, err
			}
		}
	}
	return esito, nil
}
