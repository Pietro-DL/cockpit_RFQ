// Package caricatore è l'unico pacchetto del motore A che tocca il database (piano A, A1c, 3.3.6 e 6.4.2; contratto
// di A1c, §3). In core/README.md dichiara «DB: sì, sola lettura»:
//   - apre UNA transazione REPEATABLE READ READ ONLY, e si ferma se il database non la dichiara in sola lettura e
//     repeatable read (ErrNonInSolaLettura), prima di qualunque lettura di dominio;
//   - legge con le query sqlc su db.New(tx), mai sul pool, tutto sulla stessa connessione: l'elenco delle query è
//     chiuso (contratto §3.4: 29), e SQL scritto a mano c'è solo per i due SHOW e per SELECT now();
//   - chiude la transazione con ROLLBACK e solo dopo converte le righe nei tipi puri di fotorfq e le ordina. Il
//     calcolo si fa dopo, fuori, con valutazione.Calcola chiamata da chi chiama: qui dentro nessuna interpretazione
//     e nessuna proposta.
//
// I modelli sono censimento.Leggi (registro/censimento/leggi.go), che legge e poi lascia calcolare fuori dalla
// transazione, e anteprimaDellaRfq (rfq/fascicolo/riapertura.go), con il controllo che ferma se la transazione non
// è in sola lettura (transazioneInSolaLettura). Non calibrazione.Misura, che apre solo READ ONLY, cioè READ
// COMMITTED. A differenza di anteprimaDellaRfq, qui non si calcola a transazione aperta.
//
// Non si leggono mai: la tabella dei suggerimenti dell'agente, le credenziali dei worker, le sessioni,
// cliente.regole, utente.password_hash, i job di analisi falliti (LD-08), il registro degli agganci (T-06). Le
// query che prendono lucchetti (Blocca*, ListDocumentiComponente, WorkingBloccataNelDatabase) e i lettori del
// Fascicolo legacy non si usano.
package caricatore

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/migrazioni"
)

// Iniziatore: ciò che serve per aprire la transazione. Lo soddisfa *pgxpool.Pool: quello in sola lettura del
// banco, quello del server web.
type Iniziatore interface {
	BeginTx(ctx context.Context, o pgx.TxOptions) (pgx.Tx, error)
}

// Richiesta: che cosa fotografare. Con Tutti, l'elenco delle RFQ si legge nella stessa transazione (non sul pool,
// come fa il comando U5). Messaggi sono i messaggi senza RFQ dei casi del banco (R34). Sorgente è la destinazione
// senza password (migrazioni.Destinazione), per il rapporto: resta fuori dall'impronta.
type Richiesta struct {
	Thread   []uuid.UUID
	Tutti    bool
	Messaggi []uuid.UUID
	Sorgente string
}

// ErrNonInSolaLettura: SHOW transaction_read_only non dice «on», o l'isolamento non è repeatable read.
var ErrNonInSolaLettura = errors.New("transazione non in sola lettura: nessuna fotografia")

// Il solo SQL scritto a mano: i due SHOW del controllo della transazione e l'ora della fotografia.
const (
	sqlSolaLettura = "SHOW transaction_read_only"
	sqlIsolamento  = "SHOW transaction_isolation"
	sqlAdesso      = "SELECT now()"
)

// Le risposte che il database deve dare dentro la transazione del caricatore.
const (
	solaLetturaAttesa = "on"
	isolamentoAtteso  = "repeatable read"
)

// apriInSolaLettura apre la transazione del caricatore e la controlla: REPEATABLE READ READ ONLY, poi
// SHOW transaction_read_only = on e SHOW transaction_isolation = repeatable read. Se il database non conferma,
// la transazione si chiude e torna ErrNonInSolaLettura, prima di qualunque lettura di dominio. Il controllo che
// ferma è quello di transazioneInSolaLettura (rfq/fascicolo/riapertura.go); censimento.Leggi ha la stessa
// transazione ma registra soltanto la risposta. Non esportata: la prova della scrittura rifiutata la chiama dallo
// stesso pacchetto.
func apriInSolaLettura(ctx context.Context, p Iniziatore) (pgx.Tx, error) {
	tx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("caricatore: la transazione non si apre: %w", err)
	}
	var ro, iso string
	if err := tx.QueryRow(ctx, sqlSolaLettura).Scan(&ro); err != nil {
		_ = tx.Rollback(ctx)
		return nil, fmt.Errorf("caricatore: %s: %w", sqlSolaLettura, err)
	}
	if err := tx.QueryRow(ctx, sqlIsolamento).Scan(&iso); err != nil {
		_ = tx.Rollback(ctx)
		return nil, fmt.Errorf("caricatore: %s: %w", sqlIsolamento, err)
	}
	if ro != solaLetturaAttesa || iso != isolamentoAtteso {
		_ = tx.Rollback(ctx)
		return nil, fmt.Errorf("%w (transaction_read_only = %q, transaction_isolation = %q)", ErrNonInSolaLettura, ro, iso)
	}
	return tx, nil
}

// Carica legge, chiude la transazione e restituisce la fotografia ordinata (6.4.2, «Sequenza di Carica»):
//  1. apriInSolaLettura; il ROLLBACK è l'ultimo testo SQL;
//  2. migrazioni.Applicate sulla transazione → SchemaDB; SELECT now() → PresaIl, UTC al millisecondo;
//  3. db.New(tx): GetAnalizzatoreCorrente → la terna; senza riga, nessun fatto e la diagnosi
//     fotografia.nessun_analizzatore;
//  4. il perimetro: Richiesta.Thread, e con Tutti ListRfqPerLaRiapertura sulla stessa transazione;
//  5. per ogni thread le sue letture, nella stessa transazione (leggiThread);
//  6. i clienti dei thread (ListClientiDellaFotografia, mai cliente.regole);
//  7. i messaggi fuori RFQ, con i loro allegati e i fatti per sha256;
//  8. le sigle degli utenti (ListSigleUtenti, mai password_hash);
//  9. ROLLBACK esplicito; poi, a transazione chiusa, la conversione nei tipi puri, ImprontaPayload, Ordina.
//
// Un thread o un messaggio chiesti che non ci sono sono un errore: la fotografia non li salta in silenzio.
func Carica(ctx context.Context, p Iniziatore, r Richiesta) (fotorfq.Fotografia, error) {
	tx, err := apriInSolaLettura(ctx, p)
	if err != nil {
		return fotorfq.Fotografia{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // dopo il ROLLBACK esplicito non manda niente
	l, err := leggi(ctx, tx, r)
	if err != nil {
		return fotorfq.Fotografia{}, err
	}
	if err := tx.Rollback(ctx); err != nil {
		return fotorfq.Fotografia{}, fmt.Errorf("caricatore: chiusura della transazione: %w", err)
	}
	return componi(l, r.Sorgente)
}

// letture: le righe come le ha date il database, prima della conversione.
type letture struct {
	schema       int
	presaIl      time.Time
	analizzatore *db.AnalizzatoreCorrente
	thread       []letturaThread
	clienti      []db.ListClientiDellaFotografiaRow
	fuori        []letturaFuori
	utenti       []db.ListSigleUtentiRow
}

type letturaThread struct {
	thread         db.ThreadOfferta
	messaggi       []db.Messaggio
	allegati       []db.Allegato
	fatti          []db.ListFattiDelThreadRow
	proposte       []db.DocumentoProposta
	documenti      []db.Documento
	provenienze    []db.ListProvenienzeThreadRow
	identificativi []db.IdentificativoThread
	componenti     []db.Componente
	relazioni      []db.ComponenteRelazione
	righeNodi      []db.ListComponenteProposteThreadRow
	righeArchi     []db.ListRelazioneProposteThreadRow
	rimozioni      []db.RimozioneProposta
	stepProdotto   []db.VStepProdotto
	fascicolo      []db.VFascicolo
	fabbisogni     []db.ListFabbisognoEffettivoRow
	deroghe        []db.DerogaFabbisogno
	triage         []db.ListTriageThreadRow
	candidati      []db.ListCandidatiCodiceThreadRow
	pendenti       []db.ListLavoroPendenteRfqRow
	versione       *db.BomVersione
	congelata      *db.BomVersione
}

type letturaFuori struct {
	messaggio db.Messaggio
	allegati  []db.Allegato
	fatti     []db.ListFattiPerShaRow
}

// leggi: i passi 2-8 di Carica, tutti sulla transazione.
func leggi(ctx context.Context, tx pgx.Tx, r Richiesta) (letture, error) {
	var l letture
	applicate, err := migrazioni.Applicate(ctx, tx)
	if err != nil {
		return l, fmt.Errorf("caricatore: schema: %w", err)
	}
	l.schema = migrazioni.UltimaApplicata(applicate)
	if err := tx.QueryRow(ctx, sqlAdesso).Scan(&l.presaIl); err != nil {
		return l, fmt.Errorf("caricatore: %s: %w", sqlAdesso, err)
	}

	q := db.New(tx)
	an, err := q.GetAnalizzatoreCorrente(ctx)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return l, fmt.Errorf("caricatore: analizzatore corrente: %w", err)
	default:
		l.analizzatore = &an
	}

	perimetro := unici(r.Thread)
	if r.Tutti {
		rfq, err := q.ListRfqPerLaRiapertura(ctx, uuid.NullUUID{})
		if err != nil {
			return l, fmt.Errorf("caricatore: elenco delle RFQ: %w", err)
		}
		for _, x := range rfq {
			perimetro = append(perimetro, x.ThreadID)
		}
		perimetro = unici(perimetro)
	}
	clienti := []uuid.UUID{}
	for _, id := range perimetro {
		lt, err := leggiThread(ctx, q, id, l.analizzatore)
		if err != nil {
			return l, err
		}
		l.thread = append(l.thread, lt)
		clienti = append(clienti, lt.thread.ClienteID)
	}
	if l.clienti, err = q.ListClientiDellaFotografia(ctx, unici(clienti)); err != nil {
		return l, fmt.Errorf("caricatore: clienti: %w", err)
	}

	for _, id := range unici(r.Messaggi) {
		lf, err := leggiFuori(ctx, q, id, l.analizzatore)
		if err != nil {
			return l, err
		}
		l.fuori = append(l.fuori, lf)
	}

	if l.utenti, err = q.ListSigleUtenti(ctx); err != nil {
		return l, fmt.Errorf("caricatore: utenti: %w", err)
	}
	return l, nil
}

// leggiThread: le letture di una RFQ (6.4.2, passo 5; contratto §3.2, §3.4). I fatti si leggono solo alla terna
// corrente, e senza terna non si leggono.
func leggiThread(ctx context.Context, q *db.Queries, id uuid.UUID, an *db.AnalizzatoreCorrente) (letturaThread, error) {
	var lt letturaThread
	var err error
	sbaglio := func(cosa string, err error) error {
		return fmt.Errorf("caricatore: RFQ %s, %s: %w", id, cosa, err)
	}
	nullo := uuid.NullUUID{UUID: id, Valid: true}

	lt.thread, err = q.GetThread(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return lt, fmt.Errorf("caricatore: la RFQ %s non esiste", id)
	}
	if err != nil {
		return lt, sbaglio("thread", err)
	}
	if lt.messaggi, err = q.ListMessaggiThread(ctx, nullo); err != nil {
		return lt, sbaglio("messaggi", err)
	}
	if lt.allegati, err = q.ListAllegatiThread(ctx, nullo); err != nil {
		return lt, sbaglio("allegati", err)
	}
	if an != nil {
		if lt.fatti, err = q.ListFattiDelThread(ctx, db.ListFattiDelThreadParams{
			VersioneAnalizzatore: an.VersioneAnalizzatore, HashConfigurazione: an.HashConfigurazione, ThreadID: id,
		}); err != nil {
			return lt, sbaglio("fatti", err)
		}
	}
	if lt.proposte, err = q.ListProposteDocumentoThread(ctx, nullo); err != nil {
		return lt, sbaglio("proposte di documento", err)
	}
	if lt.documenti, err = q.ListDocumentiThread(ctx, id); err != nil {
		return lt, sbaglio("documenti", err)
	}
	if lt.provenienze, err = q.ListProvenienzeThread(ctx, id); err != nil {
		return lt, sbaglio("provenienze", err)
	}
	if lt.identificativi, err = q.ListIdentificativi(ctx, id); err != nil {
		return lt, sbaglio("identificativi", err)
	}
	if lt.componenti, err = q.ListComponentiThread(ctx, id); err != nil {
		return lt, sbaglio("componenti", err)
	}
	if lt.relazioni, err = q.ListRelazioniDellaRfq(ctx, id); err != nil {
		return lt, sbaglio("relazioni", err)
	}
	if lt.righeNodi, err = q.ListComponenteProposteThread(ctx, id); err != nil {
		return lt, sbaglio("righe di componente_proposta", err)
	}
	if lt.righeArchi, err = q.ListRelazioneProposteThread(ctx, id); err != nil {
		return lt, sbaglio("righe di relazione_proposta", err)
	}
	if lt.rimozioni, err = q.ListRimozioniAperte(ctx, id); err != nil {
		return lt, sbaglio("rimozioni aperte", err)
	}
	if lt.stepProdotto, err = q.ListStepProdotto(ctx, id); err != nil {
		return lt, sbaglio("v_step_prodotto", err)
	}
	if lt.fascicolo, err = q.ListFascicolo(ctx, id); err != nil {
		return lt, sbaglio("v_fascicolo", err)
	}
	if lt.fabbisogni, err = q.ListFabbisognoEffettivo(ctx, uuid.NullUUID{UUID: lt.thread.ClienteID, Valid: true}); err != nil {
		return lt, sbaglio("fabbisogni effettivi", err)
	}
	if lt.deroghe, err = q.ListDerogheThread(ctx, id); err != nil {
		return lt, sbaglio("deroghe", err)
	}
	if lt.triage, err = q.ListTriageThread(ctx, id); err != nil {
		return lt, sbaglio("triage", err)
	}
	if lt.candidati, err = q.ListCandidatiCodiceThread(ctx, id); err != nil {
		return lt, sbaglio("candidati di codice", err)
	}
	if lt.pendenti, err = q.ListLavoroPendenteRfq(ctx, nullo); err != nil {
		return lt, sbaglio("lavoro pendente", err)
	}
	if lt.versione, err = versioneSeCe(q.GetUltimaVersione(ctx, id)); err != nil {
		return lt, sbaglio("ultima versione della BOM", err)
	}
	if lt.congelata, err = versioneSeCe(q.GetUltimaCongelata(ctx, id)); err != nil {
		return lt, sbaglio("ultima versione congelata", err)
	}
	return lt, nil
}

// versioneSeCe: una versione della BOM se c'è; nessuna riga vuol dire nessuna versione, non un errore.
func versioneSeCe(v db.BomVersione, err error) (*db.BomVersione, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// leggiFuori: un messaggio senza RFQ dei casi del banco, con gli allegati e i fatti dei loro contenuti alla terna
// corrente.
func leggiFuori(ctx context.Context, q *db.Queries, id uuid.UUID, an *db.AnalizzatoreCorrente) (letturaFuori, error) {
	var lf letturaFuori
	var err error
	lf.messaggio, err = q.GetMessaggio(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return lf, fmt.Errorf("caricatore: il messaggio %s non esiste", id)
	}
	if err != nil {
		return lf, fmt.Errorf("caricatore: messaggio %s: %w", id, err)
	}
	if lf.allegati, err = q.ListAllegatiMessaggio(ctx, id); err != nil {
		return lf, fmt.Errorf("caricatore: messaggio %s, allegati: %w", id, err)
	}
	if an == nil {
		return lf, nil
	}
	sha := []string{}
	for _, a := range lf.allegati {
		if a.Sha256.Valid {
			sha = append(sha, a.Sha256.String)
		}
	}
	slices.Sort(sha)
	sha = slices.Compact(sha)
	elenco := make([]interface{}, 0, len(sha))
	for _, s := range sha {
		elenco = append(elenco, s)
	}
	if lf.fatti, err = q.ListFattiPerSha(ctx, db.ListFattiPerShaParams{
		Sha256: elenco, VersioneAnalizzatore: an.VersioneAnalizzatore, HashConfigurazione: an.HashConfigurazione,
	}); err != nil {
		return lf, fmt.Errorf("caricatore: messaggio %s, fatti: %w", id, err)
	}
	return lf, nil
}

// unici: gli ID senza ripetizioni, in ordine di byte.
func unici(ids []uuid.UUID) []uuid.UUID {
	out := append([]uuid.UUID{}, ids...)
	slices.SortFunc(out, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	return slices.Compact(out)
}
