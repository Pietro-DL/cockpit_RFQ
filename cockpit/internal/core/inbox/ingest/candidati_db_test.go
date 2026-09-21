package ingest

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/domain"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// L4 — checkpoint 3R §2: l'ingest PROPONE e non decide.
//
// Prima di questo checkpoint l'ingest scriveva `messaggio.thread_id` da solo in due casi: stesso
// ConversationID di una conversazione già agganciata, oppure un codice qualunque uguale a un
// identificativo di una RFQ dello stesso cliente. Sono due coincidenze, non due decisioni:
//
//   - «Rispondi» a una mail vecchia per parlare d'altro conserva il ConversationID. Il messaggio
//     finiva nella RFQ sbagliata e ci restava, perché un aggancio non si annulla da solo;
//   - l'estrattore generico chiamava «codice» qualunque numero con tre cifre, quindi bastava un CAP
//     o un numero d'ordine per far combaciare due richieste che non c'entravano niente.
//
// Questi test sono scritti contro il COMPORTAMENTO: non guardano se una query è sparita, guardano che
// dopo un ingest il messaggio sia ancora orfano e che il candidato esista con la sua evidenza.

const regoleCliente = `{
  "famiglie_codice": [{"regex": "\\b\\d{7}[A-Z]\\b", "descrizione": "7 cifre + lettera", "esempio": "6674611A"}],
  "riferimento_rfq": {"regex": "\\bRDO\\s*\\d{9}\\b", "descrizione": "RDO", "esempio": "RDO 490020618"},
  "finestra_aggancio_gg": 45
}`

// pulisciBanco toglie tutto cio' che questi test hanno lasciato, nell'ordine delle chiavi esterne.
//
// Si chiama PRIMA e DOPO ogni banco, e non solo dopo: i test di questo pacchetto condividono un solo
// database, e una pulizia che fallisce a meta' in silenzio fa passare la suite solo finche' qualcun
// altro azzera lo schema. Lo stesso difetto che si era gia' visto in TestIngestIdempotente.
func pulisciBanco(t *testing.T, p *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	const clienti = `(SELECT cliente_id FROM cliente WHERE cartella_nas LIKE 'PROVA3R%')`
	const thread = `(SELECT thread_id FROM thread_offerta WHERE cliente_id IN ` + clienti + `)`
	const messaggi = `(SELECT messaggio_id FROM messaggio WHERE chiave_esterna LIKE '<test-ingest-3r-%')`
	passi := []string{
		`DELETE FROM candidato_aggancio WHERE thread_id IN ` + thread,
		`DELETE FROM candidato_aggancio WHERE messaggio_id IN ` + messaggi,
		`DELETE FROM candidato_codice   WHERE messaggio_id IN ` + messaggi,
		`DELETE FROM analisi_messaggio  WHERE messaggio_id IN ` + messaggi,
		`DELETE FROM messaggio_aggancio_log WHERE thread_id IN ` + thread + ` OR messaggio_id IN ` + messaggi,
		`DELETE FROM proposta_triage WHERE cliente_proposto IN ` + clienti + ` OR thread_proposto IN ` + thread + ` OR messaggio_id IN ` + messaggi,
		`DELETE FROM riferimento_portale WHERE thread_id IN ` + thread + ` OR messaggio_id IN ` + messaggi,
		`DELETE FROM documento_proposta WHERE thread_id IN ` + thread + ` OR allegato_id IN (SELECT allegato_id FROM allegato WHERE messaggio_id IN ` + messaggi + `)`,
		`DELETE FROM allegato WHERE messaggio_id IN ` + messaggi,
		`DELETE FROM identificativo_thread WHERE thread_id IN ` + thread,
		`DELETE FROM fase_log WHERE thread_id IN ` + thread,
		`DELETE FROM componente WHERE thread_id IN ` + thread,
		`UPDATE messaggio SET thread_id = NULL, aggancio = 'nessuno' WHERE thread_id IN ` + thread,
		`UPDATE conversazione SET thread_id = NULL, collegata_da = 'nessuno' WHERE thread_id IN ` + thread,
		`DELETE FROM thread_offerta WHERE cliente_id IN ` + clienti,
		`DELETE FROM messaggio_outlook WHERE messaggio_id IN ` + messaggi,
		`DELETE FROM messaggio_casella WHERE messaggio_id IN ` + messaggi,
		`UPDATE messaggio SET buyer_id = NULL WHERE buyer_id IN (SELECT buyer_id FROM buyer WHERE cliente_id IN ` + clienti + `)`,
		`DELETE FROM messaggio WHERE chiave_esterna LIKE '<test-ingest-3r-%'`,
		`DELETE FROM conversazione WHERE chiave_esterna LIKE 'CONV-TEST-3R-%'`,
		`DELETE FROM buyer WHERE cliente_id IN ` + clienti,
		`DELETE FROM dominio_cliente WHERE cliente_id IN ` + clienti,
		`DELETE FROM cliente WHERE cartella_nas LIKE 'PROVA3R%'`,
	}
	for _, sql := range passi {
		// l'errore NON si ignora: una pulizia che fallisce in silenzio e' il modo piu' sicuro di far
		// passare la suite una volta sola
		if _, err := p.Exec(ctx, sql); err != nil {
			t.Fatalf("pulizia del banco (%s): %v", sql, err)
		}
	}
}

// bancoCandidati prepara un cliente censito con le sue regole e il suo dominio.
func bancoCandidati(t *testing.T, p *pgxpool.Pool) db.Cliente {
	t.Helper()
	ctx := context.Background()
	q := db.New(p)
	pulisciBanco(t, p)
	t.Cleanup(func() { pulisciBanco(t, p) })
	c, err := q.InsertCliente(ctx, db.InsertClienteParams{
		CartellaNas: "PROVA3R", RagioneSociale: "Cliente Di Prova 3R", Regole: json.RawMessage(regoleCliente)})
	if err != nil {
		t.Fatalf("cliente di prova: %v", err)
	}
	if err := q.InsertDominioCliente(ctx, db.InsertDominioClienteParams{Lower: "prova3r.example", ClienteID: c.ClienteID}); err != nil {
		t.Fatalf("dominio: %v", err)
	}
	return c
}

// rfqDiProva crea una richiesta aperta del cliente, con un identificativo e un riferimento.
func rfqDiProva(t *testing.T, p *pgxpool.Pool, c db.Cliente, oggetto, codice, riferimento string, inizio time.Time) db.ThreadOfferta {
	t.Helper()
	ctx := context.Background()
	q := db.New(p)
	th, err := q.InsertThread(ctx, db.InsertThreadParams{
		ClienteID: c.ClienteID, Canale: db.CanaleOutlook, DataInizio: inizio,
		Oggetto: pgText(oggetto), CartellaRelativa: pgText(`PROVA3R\WIP\x`), Priorita: 1})
	if err != nil {
		t.Fatalf("thread di prova: %v", err)
	}
	if codice != "" {
		if _, err := q.UpsertIdentificativo(ctx, db.UpsertIdentificativoParams{
			ThreadID: th.ThreadID, Codice: codice, Origine: db.OrigineIdentificativoManuale}); err != nil {
			t.Fatalf("identificativo: %v", err)
		}
	}
	if riferimento != "" {
		if err := q.SetRiferimentoCliente(ctx, db.SetRiferimentoClienteParams{
			ThreadID: th.ThreadID, Riferimento: pgText(riferimento)}); err != nil {
			t.Fatalf("riferimento: %v", err)
		}
	}
	return th
}

func candidati(t *testing.T, p *pgxpool.Pool, chiave string) []db.ListCandidatiAggancioRow {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	if err := p.QueryRow(ctx, `SELECT messaggio_id FROM messaggio WHERE chiave_esterna = $1`, chiave).Scan(&id); err != nil {
		t.Fatalf("messaggio %s: %v", chiave, err)
	}
	righe, err := db.New(p).ListCandidatiAggancio(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return righe
}

func statoMessaggio(t *testing.T, p *pgxpool.Pool, chiave string) (uuid.NullUUID, string, string, int16) {
	t.Helper()
	var tid uuid.NullUUID
	var aggancio, esito string
	var conf int16
	err := p.QueryRow(context.Background(), `
		SELECT m.thread_id, m.aggancio::text, COALESCE(t.esito::text,''), COALESCE(t.confidenza,0)
		FROM messaggio m LEFT JOIN proposta_triage t ON t.messaggio_id = m.messaggio_id AND t.fonte = 'deterministico'
		WHERE m.chiave_esterna = $1`, chiave).Scan(&tid, &aggancio, &esito, &conf)
	if err != nil {
		t.Fatalf("stato di %s: %v", chiave, err)
	}
	return tid, aggancio, esito, conf
}

func ingerisci(t *testing.T, p *pgxpool.Pool, casella db.Casella, msg ...worker.MessaggioIn) {
	t.Helper()
	s := &Servizio{Pool: p, Log: slog.Default()}
	if _, err := s.Ingerisci(context.Background(), Lotto{Casella: casella, Messaggi: msg}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
}

func pgText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

// T1 — la risposta in una conversazione già agganciata NON viene agganciata: resta orfana con un
// candidato R1 al 95, e la schermata ha di che spiegarsi.
func TestT1LaConversazioneNotaProponeENonDecide(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	casella := casellaProva(t, p)
	c := bancoCandidati(t, p)
	t0 := time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)
	th := rfqDiProva(t, p, c, "PROVA-3R supporto cofano", "6674611A", "", t0)

	// la conversazione è collegata: è la decisione che un operatore ha preso agganciando il primo
	if err := db.New(p).CollegaConversazione(ctx, db.CollegaConversazioneParams{
		ConversazioneID: conversazioneDiProva(t, p, casella, "CONV-TEST-3R-1", t0), ThreadID: uuid.NullUUID{UUID: th.ThreadID, Valid: true},
		CollegataDa: db.AggancioOperatore}); err != nil {
		t.Fatalf("collega conversazione: %v", err)
	}

	ingerisci(t, p, casella, worker.MessaggioIn{
		MessageID: "<test-ingest-3r-r1@prova3r.example>", EntryID: "ENTRY-TEST-3R-1", StoreID: "S", ConversationID: "CONV-TEST-3R-1",
		Cartella: "Inbox", Direzione: "entrata", DataEvento: t0.Add(2 * time.Hour),
		MittenteIndirizzo: "buyer@prova3r.example", MittenteNome: "Buyer Prova",
		Oggetto: "R: PROVA-3R supporto cofano", CorpoTesto: "Ecco i disegni.", Riferimenti: []string{}, Categorie: []string{},
	})

	tid, aggancio, esito, conf := statoMessaggio(t, p, "<test-ingest-3r-r1@prova3r.example>")
	if tid.Valid {
		t.Fatalf("il messaggio è stato AGGANCIATO dall'ingest (thread %s): l'aggancio è una decisione dell'operatore", tid.UUID)
	}
	if aggancio != "nessuno" {
		t.Errorf("aggancio = %q, atteso «nessuno»", aggancio)
	}
	cand := candidati(t, p, "<test-ingest-3r-r1@prova3r.example>")
	if len(cand) == 0 {
		t.Fatal("nessun candidato: togliere l'aggancio automatico senza proporre niente è peggio di prima")
	}
	trovata := false
	for _, k := range cand {
		if k.Regola == db.RegolaAggancioR1Conversazione && k.ThreadID == th.ThreadID {
			trovata = true
			if k.Punteggio != int16(domain.PuntiRegola[domain.R1Conversazione]) {
				t.Errorf("R1 punteggio %d, atteso %d", k.Punteggio, domain.PuntiRegola[domain.R1Conversazione])
			}
			if k.Evidenza == "" {
				t.Error("un candidato senza evidenza è un numero senza spiegazione")
			}
		}
	}
	if !trovata {
		t.Errorf("manca il candidato R1 verso %s: %+v", th.ThreadID, cand)
	}
	if esito != "aggancia" {
		t.Errorf("esito del triage = %q, atteso «aggancia» (%d)", esito, conf)
	}
}

// conversazioneDiProva crea (o riusa) la conversazione con quella chiave.
func conversazioneDiProva(t *testing.T, p *pgxpool.Pool, _ db.Casella, chiave string, quando time.Time) uuid.UUID {
	t.Helper()
	conv, err := db.New(p).UpsertConversazione(context.Background(), db.UpsertConversazioneParams{
		Canale: db.CanaleOutlook, ChiaveEsterna: chiave, PrimoMessaggioIl: quando})
	if err != nil {
		t.Fatalf("conversazione: %v", err)
	}
	return conv.ConversazioneID
}

// T22 (L4) — una risposta con In-Reply-To verso un messaggio agganciato non diventa MAI `nuova_rfq`,
// nemmeno quando il contenuto grida «richiesta d'offerta» e ha un allegato.
//
// È il caso dello screenshot: «R: RICHIESTA OFFERTA COD …», parola chiave più allegato, 60 punti, e
// veniva proposta come richiesta nuova mentre rispondeva a una richiesta nostra.
func TestT22RispostaConInReplyToNonDiventaNuovaRFQ(t *testing.T) {
	p := pool(t)
	casella := casellaProva(t, p)
	c := bancoCandidati(t, p)
	t0 := time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)
	th := rfqDiProva(t, p, c, "PROVA-3R primo messaggio", "", "", t0)

	// il primo messaggio, già agganciato alla RFQ
	ingerisci(t, p, casella, worker.MessaggioIn{
		MessageID: "<test-ingest-3r-primo@prova3r.example>", EntryID: "ENTRY-TEST-3R-P", StoreID: "S", ConversationID: "CONV-TEST-3R-2",
		Cartella: "Inbox", Direzione: "entrata", DataEvento: t0, MittenteIndirizzo: "buyer@prova3r.example",
		Oggetto: "PROVA-3R primo messaggio", CorpoTesto: "primo", Riferimenti: []string{}, Categorie: []string{},
	})
	if _, err := p.Exec(context.Background(),
		`UPDATE messaggio SET thread_id = $1, aggancio = 'operatore' WHERE chiave_esterna = $2`,
		th.ThreadID, "<test-ingest-3r-primo@prova3r.example>"); err != nil {
		t.Fatal(err)
	}

	// la risposta: conversazione DIVERSA apposta, così a parlare è solo l'header
	ingerisci(t, p, casella, worker.MessaggioIn{
		MessageID: "<test-ingest-3r-risposta@prova3r.example>", EntryID: "ENTRY-TEST-3R-R", StoreID: "S", ConversationID: "CONV-TEST-3R-ALTRA",
		Cartella: "Inbox", Direzione: "entrata", DataEvento: t0.Add(time.Hour), MittenteIndirizzo: "buyer@prova3r.example",
		Oggetto:     "R: RICHIESTA OFFERTA COD 6674611A",
		CorpoTesto:  "Vi giro la richiesta d'offerta. In allegato i disegni.",
		InReplyTo:   "<test-ingest-3r-primo@prova3r.example>",
		Riferimenti: []string{"<test-ingest-3r-primo@prova3r.example>"}, Categorie: []string{},
		Allegati: []worker.AllegatoIn{{Indice: 1, NomeFile: "6674611A_4.pdf", Estensione: "pdf", Natura: "file", Bytes: 120000}},
	})

	tid, _, esito, conf := statoMessaggio(t, p, "<test-ingest-3r-risposta@prova3r.example>")
	if tid.Valid {
		t.Error("nemmeno con In-Reply-To l'ingest aggancia: propone")
	}
	if esito == "nuova_rfq" {
		t.Errorf("una risposta con In-Reply-To è diventata una richiesta NUOVA (confidenza %d)", conf)
	}
	if esito != "aggancia" {
		t.Errorf("esito = %q, atteso «aggancia»", esito)
	}
	cand := candidati(t, p, "<test-ingest-3r-risposta@prova3r.example>")
	r0 := false
	for _, k := range cand {
		if k.Regola == db.RegolaAggancioR0Reply && k.ThreadID == th.ThreadID {
			r0 = true
		}
	}
	if !r0 {
		t.Errorf("manca il candidato R0 (In-Reply-To): %+v", cand)
	}
}

// T4 — un codice vale come candidato SOLO per il cliente che lo usa. Due clienti diversi usano gli
// stessi numeri e non vuol dire niente.
//
// T3 — e vale dentro la finestra: lo stesso oggetto quattro mesi dopo non aggancia niente. In questo
// mestiere lo stesso pezzo viene riquotato, e una regola senza finestra trasforma ogni riquotazione
// nel seguito di una richiesta vecchia.
func TestT3T4CodiceEOggettoValgonoSoloNellaFinestraEPerIlCliente(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	casella := casellaProva(t, p)
	c := bancoCandidati(t, p)
	adesso := time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)

	// una richiesta VECCHIA (quattro mesi) con lo stesso oggetto e lo stesso codice
	vecchia := rfqDiProva(t, p, c, "PROVA-3R riquotazione", "6674611A", "", adesso.AddDate(0, -4, 0))

	// un ALTRO cliente con lo stesso codice, di ieri
	altro, err := db.New(p).InsertCliente(ctx, db.InsertClienteParams{CartellaNas: "PROVA3RB", RagioneSociale: "Altro Cliente 3R", Regole: json.RawMessage("{}")})
	if err != nil {
		t.Fatal(err)
	}
	dellAltro := rfqDiProva(t, p, altro, "PROVA-3R di un altro", "6674611A", "", adesso.AddDate(0, 0, -1))

	ingerisci(t, p, casella, worker.MessaggioIn{
		MessageID: "<test-ingest-3r-finestra@prova3r.example>", EntryID: "ENTRY-TEST-3R-F", StoreID: "S", ConversationID: "CONV-TEST-3R-3",
		Cartella: "Inbox", Direzione: "entrata", DataEvento: adesso, MittenteIndirizzo: "buyer@prova3r.example",
		Oggetto: "PROVA-3R riquotazione", CorpoTesto: "Richiesta d'offerta per il codice 6674611A.",
		Riferimenti: []string{}, Categorie: []string{},
	})

	for _, k := range candidati(t, p, "<test-ingest-3r-finestra@prova3r.example>") {
		if k.ThreadID == vecchia.ThreadID {
			t.Errorf("T3: candidato verso una richiesta di quattro mesi fa, fuori dalla finestra di 45 giorni: %s", k.Regola)
		}
		if k.ThreadID == dellAltro.ThreadID {
			t.Errorf("T4: candidato verso la richiesta di un ALTRO cliente: %s", k.Regola)
		}
	}
	// e dentro la finestra, per il cliente giusto, il candidato c'è: se non ci fosse, il test sopra
	// passerebbe anche con le regole spente
	dentro := rfqDiProva(t, p, c, "PROVA-3R dentro finestra", "6674611A", "", adesso.AddDate(0, 0, -3))
	ingerisci(t, p, casella, worker.MessaggioIn{
		MessageID: "<test-ingest-3r-dentro@prova3r.example>", EntryID: "ENTRY-TEST-3R-D", StoreID: "S", ConversationID: "CONV-TEST-3R-4",
		Cartella: "Inbox", Direzione: "entrata", DataEvento: adesso, MittenteIndirizzo: "buyer@prova3r.example",
		Oggetto: "PROVA-3R dentro finestra", CorpoTesto: "Ancora sul codice 6674611A.",
		Riferimenti: []string{}, Categorie: []string{},
	})
	trovato := false
	for _, k := range candidati(t, p, "<test-ingest-3r-dentro@prova3r.example>") {
		if k.ThreadID == dentro.ThreadID {
			trovato = true
		}
	}
	if !trovato {
		t.Error("dentro la finestra e per il cliente giusto il candidato deve esserci: altrimenti la prova sopra non prova niente")
	}
}

// T16 — i candidati si vedono TUTTI. Un messaggio che assomiglia a tre richieste ne propone tre, e
// nessuna viene scelta dal sistema.
func TestT16ICandidatiSiVedonoTutti(t *testing.T) {
	p := pool(t)
	casella := casellaProva(t, p)
	c := bancoCandidati(t, p)
	adesso := time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)

	perCodice := rfqDiProva(t, p, c, "PROVA-3R per codice", "6674611A", "", adesso.AddDate(0, 0, -2))
	perRiferimento := rfqDiProva(t, p, c, "PROVA-3R per riferimento", "", "RDO 490020618", adesso.AddDate(0, 0, -2))
	perOggetto := rfqDiProva(t, p, c, "PROVA-3R molti candidati", "", "", adesso.AddDate(0, 0, -2))

	ingerisci(t, p, casella, worker.MessaggioIn{
		MessageID: "<test-ingest-3r-molti@prova3r.example>", EntryID: "ENTRY-TEST-3R-M", StoreID: "S", ConversationID: "CONV-TEST-3R-5",
		Cartella: "Inbox", Direzione: "entrata", DataEvento: adesso, MittenteIndirizzo: "buyer@prova3r.example",
		Oggetto: "PROVA-3R molti candidati", CorpoTesto: "RDO 490020618 per il codice 6674611A.",
		Riferimenti: []string{}, Categorie: []string{},
	})

	cand := candidati(t, p, "<test-ingest-3r-molti@prova3r.example>")
	visti := map[uuid.UUID]string{}
	for _, k := range cand {
		visti[k.ThreadID] = string(k.Regola)
	}
	for nome, id := range map[string]uuid.UUID{"codice": perCodice.ThreadID, "riferimento": perRiferimento.ThreadID, "oggetto": perOggetto.ThreadID} {
		if _, ok := visti[id]; !ok {
			t.Errorf("manca il candidato per %s: %+v", nome, cand)
		}
	}
	// sono ordinati: il più forte in cima
	for i := 1; i < len(cand); i++ {
		if cand[i-1].Punteggio < cand[i].Punteggio {
			t.Errorf("i candidati non sono ordinati per punteggio: %d prima di %d", cand[i-1].Punteggio, cand[i].Punteggio)
		}
	}
	// e nessuno è stato scelto
	if tid, _, _, _ := statoMessaggio(t, p, "<test-ingest-3r-molti@prova3r.example>"); tid.Valid {
		t.Error("con tre candidati il sistema ne ha scelto uno: è esattamente ciò che non deve fare")
	}
}

// I candidati di CODICE: il riferimento della richiesta ha un ruolo suo e non finisce fra i codici
// prodotto; i numeri generici restano visibili ma con il ruolo che dice che cosa sono.
func TestICandidatiDiCodiceHannoUnRuolo(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	casella := casellaProva(t, p)
	bancoCandidati(t, p)
	adesso := time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)

	ingerisci(t, p, casella, worker.MessaggioIn{
		MessageID: "<test-ingest-3r-ruoli@prova3r.example>", EntryID: "ENTRY-TEST-3R-U", StoreID: "S", ConversationID: "CONV-TEST-3R-6",
		Cartella: "Inbox", Direzione: "entrata", DataEvento: adesso, MittenteIndirizzo: "buyer@prova3r.example",
		Oggetto: "RDO 490020618 richiesta offerta", CorpoTesto: "Codice 6674611A. Spett.le PROMATEC, 61032 Fano, tel 0721123456.",
		Riferimenti: []string{}, Categorie: []string{},
	})

	var id uuid.UUID
	if err := p.QueryRow(ctx, `SELECT messaggio_id FROM messaggio WHERE chiave_esterna = $1`,
		"<test-ingest-3r-ruoli@prova3r.example>").Scan(&id); err != nil {
		t.Fatal(err)
	}
	righe, err := db.New(p).ListCandidatiCodice(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	ruoli := map[string]db.CandidatoCodice{}
	for _, r := range righe {
		ruoli[r.Codice] = r
	}
	if r, ok := ruoli["RDO 490020618"]; !ok || r.Ruolo != db.RuoloCodiceRiferimentoRfq {
		t.Errorf("il riferimento del cliente non è stato registrato con il suo ruolo: %+v", righe)
	}
	if r, ok := ruoli["6674611A"]; !ok || r.Ruolo != db.RuoloCodiceProdotto || r.Origine != db.OrigineCodiceFamiglia {
		t.Errorf("il codice della famiglia non è un candidato «prodotto»: %+v", righe)
	}
	// il numero della RDO non compare ANCHE come codice prodotto: la chiave primaria è (messaggio, codice)
	for _, r := range righe {
		if r.Codice == "490020618" {
			t.Errorf("il numero della RDO è finito fra i codici prodotto: %+v", r)
		}
	}
	// il triage non lo ha messo fra gli identificativi proposti
	var ident []string
	if err := p.QueryRow(ctx, `SELECT identificativi FROM proposta_triage WHERE messaggio_id = $1`, id).Scan(&ident); err != nil {
		t.Fatal(err)
	}
	for _, c := range ident {
		if c == "490020618" || c == "RDO 490020618" {
			t.Errorf("il riferimento è fra gli identificativi proposti: %v", ident)
		}
		if c == "61032" || c == "0721123456" {
			t.Errorf("un numero generico è fra gli identificativi proposti benché il cliente abbia famiglie: %v", ident)
		}
	}
}

// D30 — lo staging automatico scende solo per chi e' riconosciuto, e solo sotto la soglia.
//
// La condizione «mittente riconosciuto» e' quella che tiene: senza, la prima newsletter con un PDF
// allegato farebbe partire un download su Outlook. Lo staging e' una cartella del server, e il NAS
// non viene toccato da qui: la regola D24 riguarda il NAS e resta intatta.
func TestD30LoStagingAutomaticoScendeSoloPerIRiconosciuti(t *testing.T) {
	p := pool(t)
	ctx := context.Background()
	casella := casellaProva(t, p)
	bancoCandidati(t, p)
	adesso := time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)

	conta := func() int {
		var n int
		_ = p.QueryRow(ctx, `SELECT count(*) FROM job WHERE tipo = 'stage_allegato'
			AND payload->>'entry_id' LIKE 'ENTRY-TEST-3R-S%'`).Scan(&n)
		return n
	}
	t.Cleanup(func() {
		_, _ = p.Exec(ctx, `DELETE FROM job WHERE tipo = 'stage_allegato' AND payload->>'entry_id' LIKE 'ENTRY-TEST-3R-S%'`)
	})

	s := &Servizio{Pool: p, Log: slog.Default(), StagingAutomatico: true}
	msg := func(chiave, entry, mittente string, bytes int64) worker.MessaggioIn {
		return worker.MessaggioIn{
			MessageID: chiave, EntryID: entry, StoreID: "S", ConversationID: "CONV-TEST-3R-S",
			Cartella: "Inbox", Direzione: "entrata", DataEvento: adesso, MittenteIndirizzo: mittente,
			Oggetto: "PROVA-3R staging", CorpoTesto: "in allegato", Riferimenti: []string{}, Categorie: []string{},
			Allegati: []worker.AllegatoIn{{Indice: 1, NomeFile: "6674611A.pdf", Estensione: "pdf", Natura: "file", Bytes: bytes}},
		}
	}

	// 1. mittente NON riconosciuto: niente
	if _, err := s.Ingerisci(ctx, Lotto{Casella: casella,
		Messaggi: []worker.MessaggioIn{msg("<test-ingest-3r-s1@sconosciuto.example>", "ENTRY-TEST-3R-S1", "tizio@sconosciuto.example", 100_000)}}); err != nil {
		t.Fatal(err)
	}
	if n := conta(); n != 0 {
		t.Errorf("staging automatico per un mittente non censito: %d job", n)
	}

	// 2. cliente riconosciuto, sotto soglia: scende
	if _, err := s.Ingerisci(ctx, Lotto{Casella: casella,
		Messaggi: []worker.MessaggioIn{msg("<test-ingest-3r-s2@prova3r.example>", "ENTRY-TEST-3R-S2", "buyer@prova3r.example", 100_000)}}); err != nil {
		t.Fatal(err)
	}
	if n := conta(); n != 1 {
		t.Errorf("staging automatico per un cliente riconosciuto: %d job (atteso 1)", n)
	}

	// 3. cliente riconosciuto ma file enorme: non scende da solo
	if _, err := s.Ingerisci(ctx, Lotto{Casella: casella,
		Messaggi: []worker.MessaggioIn{msg("<test-ingest-3r-s3@prova3r.example>", "ENTRY-TEST-3R-S3", "buyer@prova3r.example", 200*1024*1024)}}); err != nil {
		t.Fatal(err)
	}
	if n := conta(); n != 1 {
		t.Errorf("un file da 200 MB e' sceso da solo: %d job (atteso 1, quello di prima)", n)
	}

	// 4. con l'interruttore spento non scende niente: e' il comportamento predefinito
	spento := &Servizio{Pool: p, Log: slog.Default()}
	if _, err := spento.Ingerisci(ctx, Lotto{Casella: casella,
		Messaggi: []worker.MessaggioIn{msg("<test-ingest-3r-s4@prova3r.example>", "ENTRY-TEST-3R-S4", "buyer@prova3r.example", 100_000)}}); err != nil {
		t.Fatal(err)
	}
	if n := conta(); n != 1 {
		t.Errorf("staging automatico con l'interruttore spento: %d job (atteso 1, quello di prima)", n)
	}
}
