//go:build integrazione

// L4 — §8.1 del piano di test: il test di accettazione del poison pill.
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/ingest/
//
// Prima della fase 1 un solo messaggio che il database rifiutava fermava il sync: l'ingest usciva al
// primo errore, rispondeva 422 sull'intero lotto e il cursore non avanzava, quindi alla scansione
// successiva si ripresentava lo stesso messaggio e il ciclo ricominciava identico. Quel messaggio è
// la «pillola avvelenata»: non fa danno da solo, blocca tutti gli altri.
//
// La garanzia da dimostrare è una sola e si scrive in una riga: **200 ⇔ tutto durevole**. Se la
// risposta è 200, tutto ciò che il lotto ha prodotto — messaggi, scarti e cursore — è in database; se
// non è 200, in database non è rimasto nulla di quel lotto. I test qui sotto attaccano quella riga da
// tutti i lati previsti dal piano: I1, I13, I14, I16, I20, I22, I23, e i tre casi della precisazione
// P5 (risposta persa, tentativo scaduto, due tentativi concorrenti).
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/testutil"
)

const cartellaPP = "Inbox"

// ppBase è l'istante del primo elemento dei lotti di questi test.
var ppBase = time.Date(2026, 9, 10, 8, 0, 0, 0, time.UTC)

// preparaPP ricrea lo schema, censisce la casella Commerciale e restituisce tutto il necessario.
// §8.1 punto 1: schema pulito, una casella, nessun messaggio.
func preparaPP(t *testing.T) (*pgxpool.Pool, *Servizio, db.Casella, context.Context) {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()
	c, err := db.New(p).UpsertCasella(ctx, db.UpsertCasellaParams{
		Canale: db.CanaleOutlook, Indirizzo: "commerciale@azienda.it", Nome: "Commerciale", Condivisa: true,
	})
	if err != nil {
		t.Fatalf("casella Commerciale: %v", err)
	}
	if n := testutil.Conta(t, p, "messaggio"); n != 0 {
		t.Fatalf("lo schema non è pulito: %d messaggi", n)
	}
	return p, &Servizio{Pool: p, Log: testutil.LogSilenzioso()}, c, ctx
}

// tre costruisce il lotto di §8.1 punto 2: tre elementi con ricevuto_il crescente. Il secondo è quello
// che la variante rende avvelenato; guasta lo riceve e lo rompe a modo suo.
func tre(guasta func(*api.MessaggioIn)) []api.MessaggioIn {
	el := func(n int, quando time.Time) api.MessaggioIn {
		return api.MessaggioIn{
			MessageID: fmt.Sprintf("<pp-%d@acme.example>", n), EntryID: fmt.Sprintf("ENTRY-PP-%d", n),
			StoreID: "STORE-PP", ConversationID: fmt.Sprintf("CONV-PP-%d", n), Cartella: cartellaPP,
			Direzione: "entrata", DataEvento: quando, MittenteNome: "Mario Rossi",
			MittenteIndirizzo: "mario.rossi@acme.example", Oggetto: fmt.Sprintf("RFQ di prova %d", n),
			CorpoTesto: "Richiesta d'offerta.", Riferimenti: []string{}, Categorie: []string{},
			Allegati: []api.AllegatoIn{{Indice: 1, NomeFile: fmt.Sprintf("disegno-%d.pdf", n), Estensione: "pdf", Natura: "file", Bytes: 1000}},
		}
	}
	l := []api.MessaggioIn{el(1, ppBase), el(2, ppBase.Add(time.Hour)), el(3, ppBase.Add(2*time.Hour))}
	if guasta != nil {
		guasta(&l[1])
	}
	return l
}

// cursoreFinale è il ricevuto_il del terzo elemento: dove deve arrivare il cursore dopo un lotto
// andato a buon fine, anche se il secondo elemento è finito in scarto.
var cursoreFinale = ppBase.Add(2 * time.Hour)

func cursore(t *testing.T, p *pgxpool.Pool) *time.Time {
	t.Helper()
	var v *time.Time
	err := p.QueryRow(context.Background(), `SELECT ultimo_received FROM sync_cursore WHERE cartella = $1`, cartellaPP).Scan(&v)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		t.Fatalf("cursore: %v", err)
	}
	return v
}

type rigaScarto struct {
	Origine   string
	EntryID   string
	MessageID string
	Errore    string
	Tentativi int
	Payload   []byte
	Ricevuto  *time.Time
}

func scarti(t *testing.T, p *pgxpool.Pool) []rigaScarto {
	t.Helper()
	righe, err := p.Query(context.Background(),
		`SELECT origine, entry_id, coalesce(message_id,''), errore, tentativi, payload, ricevuto_il
		   FROM ingest_scarto ORDER BY entry_id`)
	if err != nil {
		t.Fatalf("scarti: %v", err)
	}
	defer righe.Close()
	var out []rigaScarto
	for righe.Next() {
		var r rigaScarto
		if err := righe.Scan(&r.Origine, &r.EntryID, &r.MessageID, &r.Errore, &r.Tentativi, &r.Payload, &r.Ricevuto); err != nil {
			t.Fatalf("scan scarto: %v", err)
		}
		out = append(out, r)
	}
	return out
}

// ---------------------------------------------------------------------------------------------
// I1 — il caso principale di §8.1: tre elementi, il secondo avvelenato.
// ---------------------------------------------------------------------------------------------

func TestPoisonPillIlLottoNonSiFermaEIlCursoreAvanza(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)

	// §8.1 punto 3: una seconda connessione, aperta fuori dalla transazione del lotto, non deve vedere
	// NIENTE finché il commit non è avvenuto. È la parte che distingue «una transazione per lotto» da
	// «una transazione per elemento»: con quest'ultima il primo elemento sarebbe già visibile qui.
	spia := testutil.Pool(t)
	var vistoDaFuori struct{ messaggi, scarti, cursori int }
	s.PrimaDelCommit = func(ctx context.Context, _ pgx.Tx) error {
		_ = spia.QueryRow(ctx, `SELECT count(*) FROM messaggio`).Scan(&vistoDaFuori.messaggi)
		_ = spia.QueryRow(ctx, `SELECT count(*) FROM ingest_scarto`).Scan(&vistoDaFuori.scarti)
		_ = spia.QueryRow(ctx, `SELECT count(*) FROM sync_cursore`).Scan(&vistoDaFuori.cursori)
		return nil
	}

	lotto := Lotto{
		Casella:  casella,
		Messaggi: tre(func(m *api.MessaggioIn) { m.Direzione = "x" }),
		Cursore:  &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale},
	}
	r, err := s.Ingerisci(ctx, lotto)
	if err != nil {
		t.Fatalf("il lotto è fallito per colpa di un solo elemento: %v", err)
	}
	s.PrimaDelCommit = nil

	if vistoDaFuori.messaggi != 0 || vistoDaFuori.scarti != 0 || vistoDaFuori.cursori != 0 {
		t.Errorf("prima del commit la seconda connessione vedeva già messaggi=%d scarti=%d cursori=%d: il lotto non è una sola transazione",
			vistoDaFuori.messaggi, vistoDaFuori.scarti, vistoDaFuori.cursori)
	}

	// §8.1 punto 4
	if r.Inseriti != 2 || r.Falliti != 1 || r.Aggiornati != 0 || len(r.Esiti) != 3 {
		t.Fatalf("esito del lotto: inseriti=%d aggiornati=%d falliti=%d esiti=%d (attesi 2/0/1/3)",
			r.Inseriti, r.Aggiornati, r.Falliti, len(r.Esiti))
	}
	// l'ordine degli esiti segue l'ordine del lotto: il secondo è quello scartato
	if r.Esiti[1].Errore == "" || !strings.Contains(r.Esiti[1].Errore, "direzione") {
		t.Errorf("l'esito del secondo elemento non spiega il motivo: %q", r.Esiti[1].Errore)
	}
	if r.Esiti[0].Errore != "" || r.Esiti[2].Errore != "" {
		t.Errorf("il primo o il terzo elemento risultano in errore: %q / %q", r.Esiti[0].Errore, r.Esiti[2].Errore)
	}
	if n := testutil.Conta(t, p, "messaggio"); n != 2 {
		t.Errorf("messaggi acquisiti = %d, attesi 2", n)
	}

	sc := scarti(t, p)
	if len(sc) != 1 {
		t.Fatalf("scarti = %d, atteso 1: %+v", len(sc), sc)
	}
	if sc[0].Origine != "ingest" {
		t.Errorf("origine dello scarto = %q, attesa \"ingest\" (il payload è in database, si riprova senza Outlook)", sc[0].Origine)
	}
	if sc[0].EntryID != "ENTRY-PP-2" || sc[0].MessageID != "<pp-2@acme.example>" || sc[0].Tentativi != 1 {
		t.Errorf("riferimenti dello scarto: entry=%q message=%q tentativi=%d", sc[0].EntryID, sc[0].MessageID, sc[0].Tentativi)
	}
	if sc[0].Ricevuto == nil || !sc[0].Ricevuto.Equal(ppBase.Add(time.Hour)) {
		t.Errorf("ricevuto_il dello scarto = %v, atteso %v", sc[0].Ricevuto, ppBase.Add(time.Hour))
	}
	// il payload deve bastare a rifare l'ingest senza Outlook: deve essere l'elemento intero
	var rimesso api.MessaggioIn
	if err := json.Unmarshal(sc[0].Payload, &rimesso); err != nil {
		t.Fatalf("payload dello scarto illeggibile: %v", err)
	}
	if rimesso.MessageID != "<pp-2@acme.example>" || rimesso.Direzione != "x" || len(rimesso.Allegati) != 1 {
		t.Errorf("il payload non è l'elemento intero: %+v", rimesso)
	}

	// il cursore è quello del TERZO elemento, non del secondo: l'elemento scartato è durevole quanto
	// un messaggio acquisito, quindi il cursore può superarlo senza perdere niente (N32).
	cur := cursore(t, p)
	if cur == nil || !cur.Equal(cursoreFinale) {
		t.Fatalf("cursore = %v, atteso %v", cur, cursoreFinale)
	}

	// §8.1 punto 5: lo stesso lotto di nuovo. Nessuna riga nuova, il conteggio dei tentativi dello
	// scarto sale, il cursore non si muove.
	r2, err := s.Ingerisci(ctx, lotto)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Aggiornati != 2 || r2.Inseriti != 0 || r2.Falliti != 1 {
		t.Errorf("secondo passaggio: inseriti=%d aggiornati=%d falliti=%d (attesi 0/2/1)", r2.Inseriti, r2.Aggiornati, r2.Falliti)
	}
	if sc2 := scarti(t, p); len(sc2) != 1 || sc2[0].Tentativi != 2 {
		t.Errorf("lo scarto non conta i tentativi: %+v", sc2)
	}
	if n := testutil.Conta(t, p, "messaggio"); n != 2 {
		t.Errorf("il secondo passaggio ha creato messaggi: %d", n)
	}
	if cur := cursore(t, p); cur == nil || !cur.Equal(cursoreFinale) {
		t.Errorf("il cursore si è mosso al secondo passaggio: %v", cur)
	}
}

// ---------------------------------------------------------------------------------------------
// I14 e varianti — l'elemento è rifiutato per motivi diversi, ma il lotto si comporta sempre allo
// stesso modo. La variante con il byte NUL è l'unica in cui a rifiutare è PostgreSQL: è quella che
// dimostra davvero il ROLLBACK TO SAVEPOINT, perché un errore del server abortisce la transazione
// intera se non c'è un savepoint da cui ripartire.
// ---------------------------------------------------------------------------------------------

func TestPoisonPillVarianti(t *testing.T) {
	casi := []struct {
		nome     string
		id       string // ID del piano di test
		guasta   func(*api.MessaggioIn)
		motivo   string // sottostringa attesa nell'errore
		daServer bool   // true = a rifiutare è PostgreSQL, non un controllo del server
	}{
		{"direzione fuori enum", "I1", func(m *api.MessaggioIn) { m.Direzione = "x" }, "direzione", false},
		{"natura allegato fuori enum", "I1", func(m *api.MessaggioIn) { m.Allegati[0].Natura = "boh" }, "natura", false},
		{"due allegati con lo stesso indice", "I14", func(m *api.MessaggioIn) {
			m.Allegati = append(m.Allegati, api.AllegatoIn{Indice: 1, NomeFile: "secondo.pdf", Estensione: "pdf", Natura: "file", Bytes: 2000})
		}, "stesso indice", false},
		{"message-id di 1200 caratteri", "I13", func(m *api.MessaggioIn) {
			m.MessageID = "<" + strings.Repeat("a", 1200) + "@acme.example>"
		}, "identificativo utilizzabile", false},
		{"byte nullo nel corpo", "I14", func(m *api.MessaggioIn) {
			m.CorpoTesto = "Richiesta\x00d'offerta."
		}, "", true},
	}

	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			p, s, casella, ctx := preparaPP(t)
			r, err := s.Ingerisci(ctx, Lotto{
				Casella: casella, Messaggi: tre(c.guasta),
				Cursore: &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale},
			})
			if err != nil {
				t.Fatalf("%s: il lotto intero è fallito: %v", c.id, err)
			}
			if r.Inseriti != 2 || r.Falliti != 1 {
				t.Fatalf("%s: inseriti=%d falliti=%d (attesi 2/1)", c.id, r.Inseriti, r.Falliti)
			}
			if c.motivo != "" && !strings.Contains(r.Esiti[1].Errore, c.motivo) {
				t.Errorf("%s: l'errore non dice perché: %q (atteso contenesse %q)", c.id, r.Esiti[1].Errore, c.motivo)
			}
			if c.daServer && r.Esiti[1].Errore == "" {
				t.Errorf("%s: nessun errore riportato per l'elemento rifiutato da PostgreSQL", c.id)
			}
			if n := testutil.Conta(t, p, "messaggio"); n != 2 {
				t.Errorf("%s: messaggi = %d, attesi 2", c.id, n)
			}
			sc := scarti(t, p)
			if len(sc) != 1 || sc[0].Origine != "ingest" {
				t.Fatalf("%s: scarti = %+v", c.id, sc)
			}
			// il payload deve essere rileggibile: se il motivo dello scarto è un byte che PostgreSQL
			// non accetta, quel byte non deve impedire di registrare lo scarto stesso.
			var rimesso api.MessaggioIn
			if err := json.Unmarshal(sc[0].Payload, &rimesso); err != nil {
				t.Errorf("%s: payload dello scarto illeggibile: %v", c.id, err)
			}
			if cur := cursore(t, p); cur == nil || !cur.Equal(cursoreFinale) {
				t.Errorf("%s: cursore = %v, atteso %v", c.id, cur, cursoreFinale)
			}
		})
	}
}

// ---------------------------------------------------------------------------------------------
// I2 — replay dal payload, senza Outlook (§8.1 punto 6).
// ---------------------------------------------------------------------------------------------

func TestReplayDalPayloadSenzaOutlook(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)
	if _, err := s.Ingerisci(ctx, Lotto{
		Casella: casella, Messaggi: tre(func(m *api.MessaggioIn) { m.Direzione = "x" }),
		Cursore: &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale},
	}); err != nil {
		t.Fatal(err)
	}
	var scartoID int64
	if err := p.QueryRow(ctx, `SELECT scarto_id FROM ingest_scarto`).Scan(&scartoID); err != nil {
		t.Fatal(err)
	}

	// riprovare senza correggere niente non deve far finta di niente: il messaggio è ancora rotto
	msg, err := s.Riprova(ctx, scartoID)
	if err != nil {
		t.Fatalf("riprova: %v", err)
	}
	if !strings.Contains(msg, "ancora in errore") {
		t.Errorf("riprova su un payload ancora rotto ha risposto %q", msg)
	}
	if n := testutil.Conta(t, p, "messaggio"); n != 2 {
		t.Errorf("la riprova fallita ha scritto qualcosa: %d messaggi", n)
	}

	// corretto il payload (è ciò che fa l'amministratore, oppure una correzione del worker), il
	// messaggio entra senza che Outlook venga interpellato: il payload basta da solo.
	if _, err := p.Exec(ctx, `UPDATE ingest_scarto SET payload = jsonb_set(payload, '{direzione}', '"entrata"') WHERE scarto_id = $1`, scartoID); err != nil {
		t.Fatal(err)
	}
	msg, err = s.Riprova(ctx, scartoID)
	if err != nil {
		t.Fatalf("riprova dopo correzione: %v", err)
	}
	if !strings.Contains(msg, "acquisito") {
		t.Errorf("riprova corretta ha risposto %q", msg)
	}
	if n := testutil.Conta(t, p, "messaggio"); n != 3 {
		t.Errorf("messaggi dopo il replay = %d, attesi 3", n)
	}
	if n := testutil.Conta(t, p, "ingest_scarto"); n != 0 {
		t.Errorf("lo scarto non è stato eliminato: %d righe", n)
	}
	// il replay non accoda nulla al worker Outlook: è il senso di origine='ingest'
	var nJob int
	_ = p.QueryRow(ctx, `SELECT count(*) FROM job WHERE worker_tipo = 'outlook'`).Scan(&nJob)
	if nJob != 0 {
		t.Errorf("il replay dal payload ha accodato %d job Outlook: doveva farne a meno", nJob)
	}
}

// ---------------------------------------------------------------------------------------------
// I22 — l'altro replay: l'elemento che il worker non è riuscito nemmeno a leggere. Qui il payload non
// c'è, quindi «riprova» deve accodare una rilettura di quel solo elemento.
// ---------------------------------------------------------------------------------------------

func TestReplayDiUnElementoNonLetto(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)
	quando := ppBase.Add(30 * time.Minute)
	r, err := s.Ingerisci(ctx, Lotto{
		Casella:  casella,
		Messaggi: tre(nil),
		Saltati: []api.ElementoSaltato{{
			EntryID: "ENTRY-PP-SALTATO", Cartella: cartellaPP, MessageID: "<pp-saltato@acme.example>",
			RicevutoIl: &quando, Oggetto: "elemento non convertito", Errore: "com_error su Body",
		}},
		Cursore: &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale},
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Inseriti != 3 || r.Falliti != 1 {
		t.Fatalf("un elemento saltato conta come fallito del lotto: %+v", r)
	}
	sc := scarti(t, p)
	if len(sc) != 1 || sc[0].Origine != "lettura" {
		t.Fatalf("scarto atteso con origine \"lettura\": %+v", sc)
	}

	var scartoID int64
	if err := p.QueryRow(ctx, `SELECT scarto_id FROM ingest_scarto`).Scan(&scartoID); err != nil {
		t.Fatal(err)
	}
	msg, err := s.Riprova(ctx, scartoID)
	if err != nil {
		t.Fatalf("riprova: %v", err)
	}
	if !strings.Contains(msg, "rilettura accodata") {
		t.Errorf("riprova di uno scarto di lettura ha risposto %q", msg)
	}
	var tipo, chiave string
	var casellaJob uuid.UUID
	if err := p.QueryRow(ctx, `SELECT tipo, chiave_idempotenza, casella_id FROM job`).Scan(&tipo, &chiave, &casellaJob); err != nil {
		t.Fatalf("il job di rilettura non è stato accodato: %v", err)
	}
	if tipo != "rileggi_elemento" || casellaJob != casella.CasellaID {
		t.Errorf("job accodato: tipo=%q casella=%v", tipo, casellaJob)
	}
	if chiave != "rileggi:"+casella.CasellaID.String()+":ENTRY-PP-SALTATO" {
		t.Errorf("chiave di idempotenza della rilettura: %q", chiave)
	}
	// premere due volte non accoda due riletture
	if _, err := s.Riprova(ctx, scartoID); err != nil {
		t.Fatal(err)
	}
	if n := testutil.Conta(t, p, "job"); n != 1 {
		t.Errorf("due riprove hanno accodato %d job", n)
	}
}

// ---------------------------------------------------------------------------------------------
// I16 — il commit non riesce. È il caso in cui «200 ⇔ tutto durevole» si gioca davvero: la risposta
// deve essere un errore e in database non deve restare nulla, nemmeno degli elementi andati bene.
// ---------------------------------------------------------------------------------------------

func TestCommitFallitoNonLasciaNullaDiParziale(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)
	lotto := Lotto{
		Casella: casella, Messaggi: tre(func(m *api.MessaggioIn) { m.Direzione = "x" }),
		Cursore: &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale},
	}

	// La transazione viene abortita con un errore SQL vero (divisione per zero): da quel momento
	// PostgreSQL rifiuta qualsiasi comando fino al ROLLBACK, quindi il COMMIT fallisce per davvero.
	s.PrimaDelCommit = func(ctx context.Context, tx pgx.Tx) error {
		var n int
		_ = tx.QueryRow(ctx, `SELECT 1/0`).Scan(&n)
		return nil
	}
	r, err := s.Ingerisci(ctx, lotto)
	if err == nil {
		t.Fatalf("commit fallito ma risposta positiva: %+v", r)
	}
	if r.Inseriti != 0 || r.Aggiornati != 0 || r.Falliti != 0 || len(r.Esiti) != 0 {
		t.Errorf("dopo un commit fallito i conteggi devono essere azzerati, non riportare lavoro mai reso durevole: %+v", r)
	}
	s.PrimaDelCommit = nil

	if n := testutil.Conta(t, p, "messaggio"); n != 0 {
		t.Errorf("messaggi rimasti dopo un commit fallito: %d", n)
	}
	if n := testutil.Conta(t, p, "ingest_scarto"); n != 0 {
		t.Errorf("scarti rimasti dopo un commit fallito: %d", n)
	}
	if cur := cursore(t, p); cur != nil {
		t.Errorf("il cursore è avanzato con un commit fallito: %v", cur)
	}

	// il worker ripete il lotto identico: ora entra tutto, senza duplicati
	r2, err := s.Ingerisci(ctx, lotto)
	if err != nil {
		t.Fatalf("ripetizione del lotto: %v", err)
	}
	if r2.Inseriti != 2 || r2.Falliti != 1 {
		t.Errorf("ripetizione: inseriti=%d falliti=%d (attesi 2/1)", r2.Inseriti, r2.Falliti)
	}
	if n := testutil.Conta(t, p, "messaggio"); n != 2 {
		t.Errorf("messaggi dopo la ripetizione = %d, attesi 2", n)
	}
	if cur := cursore(t, p); cur == nil || !cur.Equal(cursoreFinale) {
		t.Errorf("cursore dopo la ripetizione = %v", cur)
	}
}

// ---------------------------------------------------------------------------------------------
// I20 — la scansione si interrompe a metà. Il cursore deve essere quello dell'ultimo lotto
// confermato: né più avanti (salterebbe messaggi mai acquisiti) né più indietro (li rifarebbe tutti).
// ---------------------------------------------------------------------------------------------

func TestScansioneInterrottaIlCursoreRestaAllUltimoLottoConfermato(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)

	lotto := func(da, quanti int, fin time.Time) Lotto {
		var ms []api.MessaggioIn
		for i := da; i < da+quanti; i++ {
			ms = append(ms, api.MessaggioIn{
				MessageID: fmt.Sprintf("<pp-scan-%03d@acme.example>", i), EntryID: fmt.Sprintf("ENTRY-SCAN-%03d", i),
				StoreID: "STORE-PP", Cartella: cartellaPP, Direzione: "entrata",
				DataEvento: ppBase.Add(time.Duration(i) * time.Minute), MittenteIndirizzo: "mario.rossi@acme.example",
				Oggetto: fmt.Sprintf("scan %d", i), Riferimenti: []string{}, Categorie: []string{},
			})
		}
		return Lotto{Casella: casella, Messaggi: ms, Cursore: &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: fin}}
	}

	fine1 := ppBase.Add(50 * time.Minute)
	if _, err := s.Ingerisci(ctx, lotto(1, 50, fine1)); err != nil {
		t.Fatalf("primo lotto: %v", err)
	}
	if cur := cursore(t, p); cur == nil || !cur.Equal(fine1) {
		t.Fatalf("cursore dopo il primo lotto = %v, atteso %v", cur, fine1)
	}

	// il secondo lotto viene interrotto prima del commit: è il worker ucciso a metà scansione
	fine2 := ppBase.Add(100 * time.Minute)
	s.PrimaDelCommit = func(ctx context.Context, tx pgx.Tx) error {
		var n int
		_ = tx.QueryRow(ctx, `SELECT 1/0`).Scan(&n)
		return nil
	}
	if _, err := s.Ingerisci(ctx, lotto(51, 50, fine2)); err == nil {
		t.Fatal("il secondo lotto doveva fallire")
	}
	s.PrimaDelCommit = nil

	if n := testutil.Conta(t, p, "messaggio"); n != 50 {
		t.Errorf("messaggi dopo l'interruzione = %d, attesi 50: il secondo lotto non deve essere in database", n)
	}
	if cur := cursore(t, p); cur == nil || !cur.Equal(fine1) {
		t.Fatalf("cursore dopo l'interruzione = %v, atteso fermo a %v", cur, fine1)
	}

	// alla ripartenza il worker rilegge da cursore − 600 s: il primo lotto torna, non deve duplicare
	if _, err := s.Ingerisci(ctx, lotto(41, 60, fine2)); err != nil {
		t.Fatalf("ripartenza: %v", err)
	}
	if n := testutil.Conta(t, p, "messaggio"); n != 100 {
		t.Errorf("messaggi dopo la ripartenza = %d, attesi 100 senza duplicati", n)
	}
	if cur := cursore(t, p); cur == nil || !cur.Equal(fine2) {
		t.Errorf("cursore dopo la ripartenza = %v, atteso %v", cur, fine2)
	}
}

// il cursore non arretra nemmeno se un lotto in ritardo porta un valore più vecchio (Q22, GREATEST)
func TestIlCursoreNonArretra(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)
	tardi := ppBase.Add(2 * time.Hour)
	presto := ppBase.Add(30 * time.Minute)
	if _, err := s.Ingerisci(ctx, Lotto{Casella: casella, Messaggi: tre(nil), Cursore: &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: tardi}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ingerisci(ctx, Lotto{Casella: casella, Cursore: &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: presto}}); err != nil {
		t.Fatal(err)
	}
	if cur := cursore(t, p); cur == nil || !cur.Equal(tardi) {
		t.Errorf("il cursore è arretrato a %v: doveva restare %v", cur, tardi)
	}
}

// ---------------------------------------------------------------------------------------------
// I23 — casella non censita o disattivata. Non è un poison pill ma un errore di configurazione: si
// vede prima di aprire la transazione, riguarda tutto il lotto e non produce scarti.
// ---------------------------------------------------------------------------------------------

func TestCasellaNonCensitaSiFermaPrimaDellaTransazione(t *testing.T) {
	p, _, casella, ctx := preparaPP(t)
	q := db.New(p)

	if _, err := RisolviCasella(ctx, q, nil, ""); err == nil || !strings.Contains(err.Error(), "non censita") {
		t.Errorf("senza casella_id e senza casella_default: %v", err)
	}
	ignota := uuid.New()
	if _, err := RisolviCasella(ctx, q, &ignota, ""); err == nil || !strings.Contains(err.Error(), "non censita") {
		t.Errorf("casella_id sconosciuto: %v", err)
	}

	// disattivata: censita ma non più in uso. Vale come non censita, e per lo stesso motivo: nessuno
	// ha autorizzato l'acquisizione da quella casella.
	if _, err := p.Exec(ctx, `UPDATE casella SET attiva = false WHERE casella_id = $1`, casella.CasellaID); err != nil {
		t.Fatal(err)
	}
	_, err := RisolviCasella(ctx, q, &casella.CasellaID, "")
	if err == nil || !strings.Contains(err.Error(), "non censita") {
		t.Errorf("casella disattivata: %v", err)
	}

	if n := testutil.Conta(t, p, "ingest_scarto"); n != 0 {
		t.Errorf("una casella non censita ha prodotto %d scarti: non deve produrne", n)
	}
	if n := testutil.Conta(t, p, "messaggio"); n != 0 {
		t.Errorf("una casella non censita ha prodotto %d messaggi", n)
	}
}

// ---------------------------------------------------------------------------------------------
// Precisazione P5 — tre casi in cui il lotto e il tentativo si incrociano.
// ---------------------------------------------------------------------------------------------

// I24 — la risposta 200 si perde per strada e il worker ripete il lotto identico. Il database non deve
// accorgersi della differenza: nessun duplicato, cursore fermo.
func TestLottoRipetutoDopoUnaRispostaPersa(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)
	lotto := Lotto{Casella: casella, Messaggi: tre(nil), Cursore: &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale}}

	r1, err := s.Ingerisci(ctx, lotto)
	if err != nil || r1.Inseriti != 3 {
		t.Fatalf("primo invio: %+v %v", r1, err)
	}
	// il worker non ha mai visto la risposta e rimanda lo stesso identico lotto
	r2, err := s.Ingerisci(ctx, lotto)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Inseriti != 0 || r2.Aggiornati != 3 {
		t.Errorf("secondo invio: inseriti=%d aggiornati=%d (attesi 0/3)", r2.Inseriti, r2.Aggiornati)
	}
	if n := testutil.Conta(t, p, "messaggio"); n != 3 {
		t.Errorf("messaggi = %d: la ripetizione ha duplicato", n)
	}
	if n := testutil.Conta(t, p, "allegato"); n != 3 {
		t.Errorf("allegati = %d, attesi 3: la ripetizione ha duplicato gli allegati", n)
	}
	if cur := cursore(t, p); cur == nil || !cur.Equal(cursoreFinale) {
		t.Errorf("cursore = %v", cur)
	}
}

// I25 — un tentativo scaduto consegna un lotto. Deve ricevere 409 e non scrivere niente: né messaggi,
// né scarti, né cursore. È la differenza fra «il worker sta lavorando» e «questo worker ha ancora il
// diritto di scrivere».
func TestLottoDiUnTentativoScadutoNonScriveNulla(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)
	q := db.New(p)

	var jobID int64
	if err := p.QueryRow(ctx, `
		INSERT INTO job (tipo, worker_tipo, payload, lease_s, durata_max_s, casella_id, stato, worker_id,
		                 lease_token, avviato_il, lease_fino_a)
		VALUES ('sync_outlook', 'outlook', '{}'::jsonb, 300, 1800, $1, 'in_corso', 'outlook@PC-PROVA',
		        gen_random_uuid(), now() - interval '10 minutes', now() - interval '1 minute')
		RETURNING job_id`, casella.CasellaID).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	j, err := q.GetJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.Ingerisci(ctx, Lotto{
		Casella:   casella,
		Tentativo: &Tentativo{JobID: jobID, LeaseToken: j.LeaseToken.UUID, WorkerID: "outlook@PC-PROVA"},
		Messaggi:  tre(nil),
		Cursore:   &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale},
	})
	if err != ErrTentativoNonValido {
		t.Fatalf("lease scaduto: errore = %v, atteso ErrTentativoNonValido (409)", err)
	}
	if n := testutil.Conta(t, p, "messaggio"); n != 0 {
		t.Errorf("un tentativo scaduto ha scritto %d messaggi", n)
	}
	if cur := cursore(t, p); cur != nil {
		t.Errorf("un tentativo scaduto ha mosso il cursore: %v", cur)
	}

	// stesso job, lease rinnovato ma durata massima superata: il lease fresco non basta
	if _, err := p.Exec(ctx, `UPDATE job SET lease_fino_a = now() + interval '5 minutes', durata_max_s = 60 WHERE job_id = $1`, jobID); err != nil {
		t.Fatal(err)
	}
	_, err = s.Ingerisci(ctx, Lotto{
		Casella:   casella,
		Tentativo: &Tentativo{JobID: jobID, LeaseToken: j.LeaseToken.UUID, WorkerID: "outlook@PC-PROVA"},
		Messaggi:  tre(nil),
	})
	if err != ErrTentativoNonValido {
		t.Fatalf("durata massima superata: errore = %v, atteso ErrTentativoNonValido", err)
	}
	if n := testutil.Conta(t, p, "messaggio"); n != 0 {
		t.Errorf("un tentativo oltre la durata massima ha scritto %d messaggi", n)
	}

	// con il tentativo valido lo stesso lotto entra: il rifiuto riguardava il tentativo, non il dato
	if _, err := p.Exec(ctx, `UPDATE job SET durata_max_s = 1800, avviato_il = now() WHERE job_id = $1`, jobID); err != nil {
		t.Fatal(err)
	}
	r, err := s.Ingerisci(ctx, Lotto{
		Casella:   casella,
		Tentativo: &Tentativo{JobID: jobID, LeaseToken: j.LeaseToken.UUID, WorkerID: "outlook@PC-PROVA"},
		Messaggi:  tre(nil),
		Cursore:   &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale},
	})
	if err != nil || r.Inseriti != 3 {
		t.Fatalf("tentativo valido: %+v %v", r, err)
	}
}

// I26 — due lotti identici consegnati insieme da due tentativi diversi. Uno solo può inserire; l'altro
// deve trovare i messaggi già presenti e aggiornarli, mai duplicarli.
func TestDueLottiIdenticiConcorrentiNonDuplicano(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)

	var pronti sync.WaitGroup
	var via sync.WaitGroup
	pronti.Add(2)
	via.Add(1)
	esiti := make([]api.IngestRisposta, 2)
	errori := make([]error, 2)
	var fine sync.WaitGroup
	fine.Add(2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			defer fine.Done()
			pronti.Done()
			via.Wait()
			esiti[i], errori[i] = s.Ingerisci(ctx, Lotto{
				Casella: casella, Messaggi: tre(nil),
				Cursore: &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale},
			})
		}(i)
	}
	pronti.Wait()
	via.Done()
	fine.Wait()

	for i, err := range errori {
		if err != nil {
			t.Fatalf("lotto %d: %v", i, err)
		}
	}
	if tot := esiti[0].Inseriti + esiti[1].Inseriti; tot != 3 {
		t.Errorf("inserimenti totali = %d (%d + %d), attesi 3: ogni messaggio va inserito una volta sola",
			tot, esiti[0].Inseriti, esiti[1].Inseriti)
	}
	if n := testutil.Conta(t, p, "messaggio"); n != 3 {
		t.Errorf("messaggi = %d, attesi 3", n)
	}
	if n := testutil.Conta(t, p, "allegato"); n != 3 {
		t.Errorf("allegati = %d, attesi 3", n)
	}
	if n := testutil.Conta(t, p, "conversazione"); n != 3 {
		t.Errorf("conversazioni = %d, attese 3", n)
	}
	if cur := cursore(t, p); cur == nil || !cur.Equal(cursoreFinale) {
		t.Errorf("cursore = %v", cur)
	}
}

// ---------------------------------------------------------------------------------------------
// Blocco 3 del 3R — un lotto NON è una copertura.
// ---------------------------------------------------------------------------------------------

// Il confine fra le due colonne, provato dove passa il lotto.
//
// `ultimo_received` è la mail più recente che abbiamo, e il lotto la fa avanzare: viaggia nella
// stessa transazione degli elementi, così o entrano insieme o non entra niente. `coperto_fino_a` è
// fin dove Outlook è stato SCANDITO, e il lotto non ne sa niente: quel fatto lo può dichiarare solo
// chi ha finito di percorrere la finestra.
//
// Dal blocco 3 la lettura è dal più recente al più vecchio, e questo trasforma la differenza in un
// pericolo: il primo lotto contiene già la mail più nuova della finestra. Se il suo cursore facesse
// avanzare anche la copertura, un worker che muore subito dopo lascerebbe la copertura in cima a una
// finestra di cui ha letto solo il primo pezzo — e tutto quello che sta sotto non verrebbe più
// chiesto a nessuno.
func TestUnLottoFaAvanzareIlCursoreMaNonLaCopertura(t *testing.T) {
	p, s, casella, ctx := preparaPP(t)

	lotto := Lotto{
		Casella:  casella,
		Messaggi: tre(nil),
		Cursore:  &api.CursoreLotto{Cartella: cartellaPP, UltimoReceived: cursoreFinale},
	}
	if _, err := s.Ingerisci(ctx, lotto); err != nil {
		t.Fatalf("lotto: %v", err)
	}

	if c := cursore(t, p); c == nil || !c.Equal(cursoreFinale) {
		t.Errorf("ultimo_received = %v, atteso %v: il lotto deve farlo avanzare", c, cursoreFinale)
	}
	var coperto *time.Time
	if err := p.QueryRow(ctx, `SELECT coperto_fino_a FROM sync_cursore WHERE cartella = $1`, cartellaPP).Scan(&coperto); err != nil {
		t.Fatalf("copertura: %v", err)
	}
	if coperto != nil {
		t.Fatalf("un lotto ha dichiarato la copertura fino a %v. In lettura dal più recente al più "+
			"vecchio il primo lotto è la cima della finestra: da qui in poi tutto ciò che sta sotto "+
			"risulta già scandito e nessuno lo rileggerà", coperto)
	}
}
