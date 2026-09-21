//go:build integrazione

// L4 — fase 2, voce 2.1: la stessa mail in più caselle (I3, I4, I18, I21), il cursore per casella
// (W2) e la direzione decisa dal server.
//
// Che cosa c'era prima, e perché non si vedeva. Un messaggio aveva UNA riga `messaggio_outlook` con
// entry_id, cartella e stato di lettura: il modello diceva che un messaggio sta in un posto solo.
// La stessa mail mandata a Commerciale e a Francesco è invece un solo messaggio in due posti, e la
// seconda casella sovrascriveva i dati della prima. Il danno non si vedeva al momento — l'ingest
// andava a buon fine, i conteggi tornavano — ma da lì in poi «Apri in Outlook» e «Scarica allegato»
// usavano l'EntryID dell'altra casella: un'operazione che fallisce settimane dopo, per un motivo che
// nessuno collega più al sync di oggi.
//
// Questi test sono scritti contro i FATTI osservabili (quante presenze, quale cursore, che cosa resta
// deciso) e non contro le query: se domani l'implementazione cambia, il test resta valido.
package ingest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// casellaDiProva censisce una casella in più oltre a quella di preparaPP.
func casellaDiProva(t *testing.T, ctx context.Context, p *pgxpool.Pool, indirizzo, nome string) db.Casella {
	t.Helper()
	c, err := db.New(p).UpsertCasella(ctx, db.UpsertCasellaParams{
		Canale: db.CanaleOutlook, Indirizzo: indirizzo, Nome: nome, Condivisa: true,
	})
	if err != nil {
		t.Fatalf("casella %s: %v", indirizzo, err)
	}
	return c
}

// stessaMail costruisce LO STESSO messaggio (stesso Message-ID) come lo vede una casella: l'EntryID e
// la cartella sono diversi, perché sono fatti della copia e non del messaggio.
func stessaMail(entry, cartella string, ricevuto time.Time) api.MessaggioIn {
	return api.MessaggioIn{
		MessageID: "<condivisa-1@acme.example>", EntryID: entry, StoreID: "STORE-" + entry,
		ConversationID: "CONV-CONDIVISA", Cartella: cartella, Direzione: "entrata",
		DataEvento: ppBase, RicevutoIl: &ricevuto, MittenteNome: "Mario Rossi",
		MittenteIndirizzo: "mario.rossi@acme.example", Oggetto: "RFQ 6674611A",
		CorpoTesto: "Richiesta d'offerta, vedi allegato.", CorpoHTML: "<p>Richiesta d'offerta</p>",
		Riferimenti: []string{}, Categorie: []string{},
		Destinatari: []api.Destinatario{{Indirizzo: "commerciale@azienda.it", Tipo: "a"}},
		Allegati:    []api.AllegatoIn{{Indice: 1, NomeFile: "6674611A_4.pdf", Estensione: "pdf", Natura: "file", Bytes: 2000}},
	}
}

func presenze(t *testing.T, ctx context.Context, p *pgxpool.Pool, chiave string) []db.ListPresenzeRow {
	t.Helper()
	var id uuid.UUID
	if err := p.QueryRow(ctx, `SELECT messaggio_id FROM messaggio WHERE chiave_esterna = $1`, chiave).Scan(&id); err != nil {
		t.Fatalf("messaggio %s: %v", chiave, err)
	}
	pr, err := db.New(p).ListPresenze(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return pr
}

// ---------------------------------------------------------------------------------------------
// I3 — la stessa mail in due caselle, anche consegnata insieme da due worker.
// ---------------------------------------------------------------------------------------------

func TestStessaMailInDueCaselleUnSoloMessaggioDuePresenze(t *testing.T) {
	p, s, commerciale, ctx := preparaPP(t)
	francesco := casellaDiProva(t, ctx, p, "francesco@azienda.it", "Francesco")

	lotti := []Lotto{
		{Casella: commerciale, Messaggi: []api.MessaggioIn{stessaMail("ENTRY-COMM", "Posta in arrivo", ppBase)},
			Cursore: &api.CursoreLotto{Cartella: "Posta in arrivo", UltimoReceived: ppBase}},
		{Casella: francesco, Messaggi: []api.MessaggioIn{stessaMail("ENTRY-FRA", "RFQ da leggere", ppBase.Add(time.Minute))},
			Cursore: &api.CursoreLotto{Cartella: "Posta in arrivo", UltimoReceived: ppBase.Add(time.Minute)}},
	}

	var via, fine sync.WaitGroup
	via.Add(1)
	fine.Add(2)
	esiti := make([]api.IngestRisposta, 2)
	errori := make([]error, 2)
	for i := range lotti {
		go func(i int) {
			defer fine.Done()
			via.Wait()
			esiti[i], errori[i] = s.Ingerisci(ctx, lotti[i])
		}(i)
	}
	via.Done()
	fine.Wait()

	for i, err := range errori {
		if err != nil {
			t.Fatalf("lotto %d: %v", i, err)
		}
	}
	// «inserito» vuol dire «il messaggio non c'era»: può essere vero per uno solo dei due, anche
	// quando arrivano insieme. Se fosse vero per entrambi avremmo due messaggi con lo stesso
	// Message-ID, cioè due RFQ possibili dalla stessa mail.
	if tot := esiti[0].Inseriti + esiti[1].Inseriti; tot != 1 {
		t.Errorf("inserimenti = %d (%d + %d), atteso 1", tot, esiti[0].Inseriti, esiti[1].Inseriti)
	}
	if n := testutil.Conta(t, p, "messaggio"); n != 1 {
		t.Fatalf("messaggi = %d, atteso 1: la stessa mail in due caselle è UN messaggio", n)
	}
	if n := testutil.Conta(t, p, "allegato"); n != 1 {
		t.Errorf("allegati = %d, atteso 1", n)
	}
	if n := testutil.Conta(t, p, "documento_proposta"); n != 1 {
		t.Errorf("proposte = %d, attesa 1: la proposta è dell'allegato, non della copia", n)
	}
	if n := testutil.Conta(t, p, "proposta_triage"); n != 1 {
		t.Errorf("proposte di triage = %d, attesa 1: il secondo ingest ne ha aperta un'altra", n)
	}

	pres := presenze(t, ctx, p, "<condivisa-1@acme.example>")
	if len(pres) != 2 {
		t.Fatalf("presenze = %d, attese 2: %+v", len(pres), pres)
	}
	perCasella := map[uuid.UUID]db.ListPresenzeRow{}
	for _, pr := range pres {
		perCasella[pr.CasellaID] = pr
	}
	if pr := perCasella[commerciale.CasellaID]; pr.EntryID != "ENTRY-COMM" || pr.Cartella.String != "Posta in arrivo" {
		t.Errorf("presenza in Commerciale: entry=%q cartella=%q", pr.EntryID, pr.Cartella.String)
	}
	if pr := perCasella[francesco.CasellaID]; pr.EntryID != "ENTRY-FRA" || pr.Cartella.String != "RFQ da leggere" {
		t.Errorf("presenza in Francesco: entry=%q cartella=%q", pr.EntryID, pr.Cartella.String)
	}

	// i due cursori sono separati: è il difetto che la 0004 elimina. Con la chiave per sola cartella
	// la seconda casella avrebbe scritto sulla riga della prima, e ogni avanzamento di troppo è una
	// finestra di tempo che quella casella non rilegge mai.
	var righe int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM sync_cursore WHERE cartella = 'Posta in arrivo'`).Scan(&righe); err != nil {
		t.Fatal(err)
	}
	if righe != 2 {
		t.Fatalf("cursori per «Posta in arrivo» = %d, attesi 2 (uno per casella)", righe)
	}
	q := db.New(p)
	c1, err := q.GetSyncCursore(ctx, db.GetSyncCursoreParams{CasellaID: commerciale.CasellaID, Cartella: "Posta in arrivo"})
	if err != nil {
		t.Fatal(err)
	}
	c2, err := q.GetSyncCursore(ctx, db.GetSyncCursoreParams{CasellaID: francesco.CasellaID, Cartella: "Posta in arrivo"})
	if err != nil {
		t.Fatal(err)
	}
	if c1.UltimoReceived == nil || !c1.UltimoReceived.Equal(ppBase) {
		t.Errorf("cursore di Commerciale = %v, atteso %v", c1.UltimoReceived, ppBase)
	}
	if c2.UltimoReceived == nil || !c2.UltimoReceived.Equal(ppBase.Add(time.Minute)) {
		t.Errorf("cursore di Francesco = %v, atteso %v", c2.UltimoReceived, ppBase.Add(time.Minute))
	}
}

// ---------------------------------------------------------------------------------------------
// I4 — il risync di un messaggio già noto aggiorna la presenza e non perde ciò che non gli è stato
// rimandato. Un worker che legge con meno diritti, o un elemento il cui HTML non si riesce più a
// leggere, non devono cancellare il corpo che avevamo già.
// ---------------------------------------------------------------------------------------------

func TestRisyncAggiornaLaPresenzaSenzaPerdereIlCorpo(t *testing.T) {
	p, s, commerciale, ctx := preparaPP(t)

	primo := stessaMail("ENTRY-COMM", "Posta in arrivo", ppBase)
	primo.NonLetto = true
	if _, err := s.Ingerisci(ctx, Lotto{Casella: commerciale, Messaggi: []api.MessaggioIn{primo}}); err != nil {
		t.Fatal(err)
	}

	// l'elemento viene spostato in un'altra cartella, letto, e questa volta l'HTML non arriva
	secondo := stessaMail("ENTRY-COMM-MOSSO", "RFQ archiviate", ppBase.Add(2*time.Hour))
	secondo.CorpoHTML = ""
	secondo.NonLetto = false
	secondo.Categorie = []string{"RFQ"}
	r, err := s.Ingerisci(ctx, Lotto{Casella: commerciale, Messaggi: []api.MessaggioIn{secondo}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Inseriti != 0 || r.Aggiornati != 1 {
		t.Fatalf("risync: inseriti=%d aggiornati=%d (attesi 0 e 1)", r.Inseriti, r.Aggiornati)
	}

	pres := presenze(t, ctx, p, "<condivisa-1@acme.example>")
	if len(pres) != 1 {
		t.Fatalf("presenze = %d, attesa 1: il risync non deve creare una copia nuova", len(pres))
	}
	pr := pres[0]
	if pr.EntryID != "ENTRY-COMM-MOSSO" || pr.Cartella.String != "RFQ archiviate" {
		t.Errorf("presenza non aggiornata: entry=%q cartella=%q", pr.EntryID, pr.Cartella.String)
	}
	if pr.NonLetto {
		t.Error("lo stato di lettura non è stato aggiornato")
	}
	if !pr.RicevutoIl.Equal(ppBase.Add(2 * time.Hour)) {
		t.Errorf("ricevuto_il = %v, atteso %v", pr.RicevutoIl, ppBase.Add(2*time.Hour))
	}
	if len(pr.Categorie) != 1 || pr.Categorie[0] != "RFQ" {
		t.Errorf("categorie = %v", pr.Categorie)
	}

	var html, testo string
	if err := p.QueryRow(ctx, `SELECT coalesce(corpo_html,''), coalesce(corpo_testo,'') FROM messaggio
		WHERE chiave_esterna = '<condivisa-1@acme.example>'`).Scan(&html, &testo); err != nil {
		t.Fatal(err)
	}
	if html == "" {
		t.Error("il corpo HTML è stato cancellato da un risync che non lo portava")
	}
	if testo == "" {
		t.Error("il corpo di testo è stato cancellato")
	}
}

// ---------------------------------------------------------------------------------------------
// I18 — la stessa mail in QUATTRO caselle, con la RFQ creata dopo il primo lotto. Le tre copie che
// arrivano dopo aggiornano la presenza e basta: non ricreano proposte decise, non riaprono il
// triage, non fanno nascere una seconda RFQ, non fanno ripartire un download già fatto.
// ---------------------------------------------------------------------------------------------

func TestQuattroCaselleUnaSolaElaborazione(t *testing.T) {
	p, s, prima, ctx := preparaPP(t)
	q := db.New(p)
	caselle := []db.Casella{prima}
	for i := 2; i <= 4; i++ {
		caselle = append(caselle, casellaDiProva(t, ctx, p, fmt.Sprintf("utente%d@azienda.it", i), fmt.Sprintf("Utente %d", i)))
	}

	// primo worker: il messaggio entra
	if _, err := s.Ingerisci(ctx, Lotto{Casella: caselle[0],
		Messaggi: []api.MessaggioIn{stessaMail("ENTRY-1", "Posta in arrivo", ppBase)}}); err != nil {
		t.Fatal(err)
	}

	// l'operatore crea la RFQ, accetta il triage e chiede il download dell'allegato
	var msgID, allegatoID, clienteID, threadID uuid.UUID
	if err := p.QueryRow(ctx, `SELECT messaggio_id FROM messaggio`).Scan(&msgID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `SELECT allegato_id FROM allegato`).Scan(&allegatoID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME','Acme SpA') RETURNING cliente_id`).Scan(&clienteID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa)
		VALUES ($1,'outlook', now(),'RFQ 6674611A','ACME\WIP\2026 09 10 RFQ') RETURNING thread_id`, clienteID).Scan(&threadID); err != nil {
		t.Fatal(err)
	}
	if err := q.AgganciaMessaggio(ctx, db.AgganciaMessaggioParams{
		MessaggioID: msgID, ThreadID: uuid.NullUUID{UUID: threadID, Valid: true}, Aggancio: db.AggancioOperatore}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.DecidiTriage(ctx, db.DecidiTriageParams{MessaggioID: msgID, Stato: db.StatoTriageAccettata}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `UPDATE documento_proposta SET stato = 'confermata' WHERE allegato_id = $1`, allegatoID); err != nil {
		t.Fatal(err)
	}
	// il download è già stato chiesto e concluso: il file è in staging
	if _, err := p.Exec(ctx, `INSERT INTO job (tipo, worker_tipo, payload, chiave_idempotenza, stato, lease_s, durata_max_s, chiuso_il)
		VALUES ('stage_allegato','outlook','{}'::jsonb, $1, 'fatto', 300, 900, now())`, "stage:"+allegatoID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `UPDATE allegato SET stato = 'in_staging', path_staging = 'C:\staging\x.pdf',
		sha256 = repeat('a',64) WHERE allegato_id = $1`, allegatoID); err != nil {
		t.Fatal(err)
	}

	// gli altri tre worker consegnano la loro copia
	for i, c := range caselle[1:] {
		if _, err := s.Ingerisci(ctx, Lotto{Casella: c,
			Messaggi: []api.MessaggioIn{stessaMail(fmt.Sprintf("ENTRY-%d", i+2), "Posta in arrivo", ppBase.Add(time.Duration(i)*time.Minute))}}); err != nil {
			t.Fatalf("casella %s: %v", c.Indirizzo, err)
		}
	}

	if n := len(presenze(t, ctx, p, "<condivisa-1@acme.example>")); n != 4 {
		t.Errorf("presenze = %d, attese 4", n)
	}
	if n := testutil.Conta(t, p, "messaggio"); n != 1 {
		t.Errorf("messaggi = %d, atteso 1", n)
	}
	if n := testutil.Conta(t, p, "thread_offerta"); n != 1 {
		t.Errorf("RFQ = %d, attesa 1: una copia in più non fa nascere una seconda RFQ", n)
	}
	if n := testutil.Conta(t, p, "allegato"); n != 1 {
		t.Errorf("allegati = %d, atteso 1", n)
	}

	// la decisione dell'operatore è intatta
	var statoProposta, statoTriage string
	if err := p.QueryRow(ctx, `SELECT stato::text FROM documento_proposta WHERE allegato_id = $1`, allegatoID).Scan(&statoProposta); err != nil {
		t.Fatal(err)
	}
	if statoProposta != "confermata" {
		t.Errorf("proposta sull'allegato = %q: una copia in più l'ha riaperta", statoProposta)
	}
	if err := p.QueryRow(ctx, `SELECT stato::text FROM proposta_triage WHERE messaggio_id = $1`, msgID).Scan(&statoTriage); err != nil {
		t.Fatal(err)
	}
	if statoTriage != "accettata" {
		t.Errorf("triage = %q: una copia in più l'ha riaperto", statoTriage)
	}
	var thread uuid.NullUUID
	if err := p.QueryRow(ctx, `SELECT thread_id FROM messaggio WHERE messaggio_id = $1`, msgID).Scan(&thread); err != nil {
		t.Fatal(err)
	}
	if !thread.Valid || thread.UUID != threadID {
		t.Errorf("il messaggio non è più agganciato alla RFQ: %v", thread)
	}

	// un solo download, e il file resta dov'è
	var nStage int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM job WHERE chiave_idempotenza = $1`, "stage:"+allegatoID.String()).Scan(&nStage); err != nil {
		t.Fatal(err)
	}
	if nStage != 1 {
		t.Errorf("job di download = %d, atteso 1: le copie successive ne hanno chiesto un altro", nStage)
	}
	var stato, path string
	if err := p.QueryRow(ctx, `SELECT stato::text, coalesce(path_staging,'') FROM allegato WHERE allegato_id = $1`, allegatoID).Scan(&stato, &path); err != nil {
		t.Fatal(err)
	}
	if stato != "in_staging" || path == "" {
		t.Errorf("l'allegato è tornato indietro: stato=%q path=%q", stato, path)
	}
}

// ---------------------------------------------------------------------------------------------
// I21 — un messaggio ricevuto direttamente non sparisce dall'Inbox quando, il giorno dopo, arriva
// anche come allegato di un inoltro.
//
// Prima la condizione di v_inbox era `parent_messaggio_id IS NULL` e basta: il momento in cui il
// messaggio annidato veniva collegato al suo contenitore, la RDO ricevuta davvero usciva dalla
// schermata. Nessun errore, nessuna traccia: semplicemente una richiesta che non si trovava più.
// La regola giusta è: c'è se è stato RICEVUTO da qualche parte, oppure se non è figlio di nessuno.
// ---------------------------------------------------------------------------------------------

func TestMessaggioRicevutoNonSpariceDallInboxQuandoArrivaAncheComeAllegato(t *testing.T) {
	p, s, commerciale, ctx := preparaPP(t)

	// la RDO arriva direttamente: ha una presenza
	if _, err := s.Ingerisci(ctx, Lotto{Casella: commerciale,
		Messaggi: []api.MessaggioIn{stessaMail("ENTRY-DIRETTA", "Posta in arrivo", ppBase)}}); err != nil {
		t.Fatal(err)
	}
	var diretta uuid.UUID
	if err := p.QueryRow(ctx, `SELECT messaggio_id FROM messaggio WHERE chiave_esterna = '<condivisa-1@acme.example>'`).Scan(&diretta); err != nil {
		t.Fatal(err)
	}

	// il giorno dopo arriva un inoltro che la contiene come .msg: l'ingest dell'inoltro collega il
	// figlio al contenitore (parent_messaggio_id). Il figlio è lo STESSO messaggio già ricevuto.
	inoltro := stessaMail("ENTRY-INOLTRO", "Posta in arrivo", ppBase.Add(24*time.Hour))
	inoltro.MessageID = "<inoltro-1@azienda.it>"
	inoltro.ConversationID = "CONV-INOLTRO"
	inoltro.Oggetto = "I: RFQ 6674611A"
	if _, err := s.Ingerisci(ctx, Lotto{Casella: commerciale, Messaggi: []api.MessaggioIn{inoltro}}); err != nil {
		t.Fatal(err)
	}
	var contenitore uuid.UUID
	if err := p.QueryRow(ctx, `SELECT messaggio_id FROM messaggio WHERE chiave_esterna = '<inoltro-1@azienda.it>'`).Scan(&contenitore); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `UPDATE messaggio SET parent_messaggio_id = $2 WHERE messaggio_id = $1`, diretta, contenitore); err != nil {
		t.Fatal(err)
	}

	inInbox := func(id uuid.UUID) bool {
		t.Helper()
		var n int
		if err := p.QueryRow(ctx, `SELECT count(*) FROM v_inbox WHERE messaggio_id = $1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n > 0
	}
	if !inInbox(diretta) {
		t.Error("la RDO ricevuta direttamente è sparita dall'Inbox perché è arrivata anche come allegato")
	}
	if !inInbox(contenitore) {
		t.Error("l'inoltro non compare in Inbox")
	}

	// controprova: un messaggio che esiste SOLO come figlio (mai ricevuto in una casella) resta
	// nascosto. Senza questa metà la regola sarebbe «mostra tutto», e l'Inbox si riempirebbe di
	// messaggi annidati che nessuno ha mai ricevuto.
	var soloFiglio uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, parent_messaggio_id,
		direzione, data_evento, oggetto)
		SELECT 'outlook','<annidato-1@acme.example>', conversazione_id, $1, 'entrata', now(), 'allegato .msg'
		  FROM messaggio WHERE messaggio_id = $1 RETURNING messaggio_id`, contenitore).Scan(&soloFiglio); err != nil {
		t.Fatal(err)
	}
	if inInbox(soloFiglio) {
		t.Error("un messaggio che esiste solo come allegato di un altro compare in Inbox")
	}

	// e l'Inbox mostra da quali caselle arriva
	var caselle []string
	if err := p.QueryRow(ctx, `SELECT caselle FROM v_inbox WHERE messaggio_id = $1`, diretta).Scan(&caselle); err != nil {
		t.Fatal(err)
	}
	if len(caselle) != 1 || caselle[0] != commerciale.Nome {
		t.Errorf("caselle in v_inbox = %v, attesa [%s]", caselle, commerciale.Nome)
	}
}

// ---------------------------------------------------------------------------------------------
// W2 e direzione: due cose che il worker non può decidere da solo.
// ---------------------------------------------------------------------------------------------

// Il cursore avanza su `ricevuto_il`, che è il ReceivedTime nella casella: la stessa grandezza su cui
// filtra la scansione. Su `data_evento` (SentOn per la posta inviata) il cursore poteva superare
// elementi che nessuno aveva ancora letto.
func TestIlCursoreUsaRicevutoIlNonDataEvento(t *testing.T) {
	p, s, commerciale, ctx := preparaPP(t)

	scritta := ppBase                      // quando la mail è stata scritta/inviata
	arrivata := ppBase.Add(72 * time.Hour) // quando è comparsa nella casella, tre giorni dopo
	m := stessaMail("ENTRY-INVIATA", "Posta inviata", arrivata)
	m.DataEvento = scritta
	if _, err := s.Ingerisci(ctx, Lotto{Casella: commerciale, Messaggi: []api.MessaggioIn{m}}); err != nil {
		t.Fatal(err)
	}
	pr := presenze(t, ctx, p, "<condivisa-1@acme.example>")[0]
	if !pr.RicevutoIl.Equal(arrivata) {
		t.Errorf("ricevuto_il = %v, atteso %v: la presenza deve conservare il ReceivedTime, non SentOn", pr.RicevutoIl, arrivata)
	}
	var evento time.Time
	if err := p.QueryRow(ctx, `SELECT data_evento FROM messaggio WHERE chiave_esterna = '<condivisa-1@acme.example>'`).Scan(&evento); err != nil {
		t.Fatal(err)
	}
	if !evento.Equal(scritta) {
		t.Errorf("data_evento = %v, atteso %v: «quando è successo» resta quello che era", evento, scritta)
	}

	// un worker che non manda ancora ricevuto_il non deve rompersi: si usa data_evento, com'era prima
	vecchio := stessaMail("ENTRY-VECCHIO", "Posta in arrivo", ppBase)
	vecchio.MessageID = "<senza-ricevuto@acme.example>"
	vecchio.ConversationID = "CONV-VECCHIO"
	vecchio.RicevutoIl = nil
	if _, err := s.Ingerisci(ctx, Lotto{Casella: commerciale, Messaggi: []api.MessaggioIn{vecchio}}); err != nil {
		t.Fatal(err)
	}
	if pr := presenze(t, ctx, p, "<senza-ricevuto@acme.example>")[0]; !pr.RicevutoIl.Equal(vecchio.DataEvento) {
		t.Errorf("senza ricevuto_il: %v, atteso data_evento %v", pr.RicevutoIl, vecchio.DataEvento)
	}
}

// La direzione la decide il SERVER confrontando il mittente con le caselle censite, e `interno` è un
// fatto separato (D10). Il worker continua a dichiarare la sua ipotesi, che resta la riserva: su una
// casella condivisa «la Posta inviata» e «i miei indirizzi» dipendono da come è configurato quel
// profilo Outlook, e una direzione sbagliata cambia la lettura dell'intero messaggio.
func TestLaDirezioneVieneDalleCaselleCensiteNonDalWorker(t *testing.T) {
	p, s, commerciale, ctx := preparaPP(t)
	casellaDiProva(t, ctx, p, "francesco@azienda.it", "Francesco")

	casi := []struct {
		nome, messageID, mittente string
		destinatari               []api.Destinatario
		dichiarata                string
		direzione                 string
		interno                   bool
	}{
		{"cliente che scrive a noi", "<c1@acme.example>", "mario.rossi@acme.example",
			[]api.Destinatario{{Indirizzo: "commerciale@azienda.it", Tipo: "a"}}, "entrata", "entrata", false},
		{"noi che scriviamo al cliente", "<c2@azienda.it>", "commerciale@azienda.it",
			[]api.Destinatario{{Indirizzo: "mario.rossi@acme.example", Tipo: "a"}}, "uscita", "uscita", false},
		{"collega che gira una mail a un collega", "<c3@azienda.it>", "francesco@azienda.it",
			[]api.Destinatario{{Indirizzo: "commerciale@azienda.it", Tipo: "a"}}, "entrata", "uscita", true},
		{"il worker sbaglia: dice entrata ma il mittente siamo noi", "<c4@azienda.it>", "commerciale@azienda.it",
			[]api.Destinatario{{Indirizzo: "buyer@acme.example", Tipo: "a"}}, "entrata", "uscita", false},
		{"il worker sbaglia al contrario: dice uscita ma il mittente è il cliente", "<c5@acme.example>", "buyer@acme.example",
			[]api.Destinatario{{Indirizzo: "commerciale@azienda.it", Tipo: "a"}}, "uscita", "entrata", false},
	}
	for i, c := range casi {
		m := stessaMail(fmt.Sprintf("ENTRY-DIR-%d", i), "Posta in arrivo", ppBase.Add(time.Duration(i)*time.Minute))
		m.MessageID = c.messageID
		m.ConversationID = fmt.Sprintf("CONV-DIR-%d", i)
		m.MittenteIndirizzo = c.mittente
		m.Destinatari = c.destinatari
		m.Direzione = c.dichiarata
		if _, err := s.Ingerisci(ctx, Lotto{Casella: commerciale, Messaggi: []api.MessaggioIn{m}}); err != nil {
			t.Fatalf("%s: %v", c.nome, err)
		}
		var dir string
		var interno bool
		if err := p.QueryRow(ctx, `SELECT direzione::text, interno FROM messaggio WHERE chiave_esterna = $1`, c.messageID).Scan(&dir, &interno); err != nil {
			t.Fatalf("%s: %v", c.nome, err)
		}
		if dir != c.direzione {
			t.Errorf("%s: direzione = %q, attesa %q (il worker diceva %q)", c.nome, dir, c.direzione, c.dichiarata)
		}
		if interno != c.interno {
			t.Errorf("%s: interno = %v, atteso %v", c.nome, interno, c.interno)
		}
	}

	// una direzione fuori enum resta un difetto del worker e si vede: il fatto che il server
	// ricalcoli non è un motivo per accettare un contratto violato (N7).
	rotto := stessaMail("ENTRY-DIR-ROTTO", "Posta in arrivo", ppBase)
	rotto.MessageID = "<rotto@acme.example>"
	rotto.ConversationID = "CONV-ROTTO"
	rotto.Direzione = "boh"
	r, err := s.Ingerisci(ctx, Lotto{Casella: commerciale, Messaggi: []api.MessaggioIn{rotto}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Falliti != 1 || !strings.Contains(r.Esiti[0].Errore, "direzione") {
		t.Errorf("direzione fuori enum accettata: %+v", r)
	}

	// e una mail interna riceve comunque una proposta di triage: «ti giro questa richiesta» è uno
	// dei modi in cui una RFQ arriva sul tavolo.
	var nTriage int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM proposta_triage pt JOIN messaggio m USING (messaggio_id)
		WHERE m.chiave_esterna = '<c3@azienda.it>'`).Scan(&nTriage); err != nil {
		t.Fatal(err)
	}
	if nTriage != 1 {
		t.Errorf("proposte di triage per la mail interna = %d, attesa 1", nTriage)
	}
}
