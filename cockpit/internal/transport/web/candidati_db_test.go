//go:build integrazione

// L4 — checkpoint 3R §2 e §4: la decisione dell'operatore non trascina, e il form non conferma da solo.
//
// Due difetti visti sulla posta vera, tutti e due invisibili finché il corpus è piccolo:
//
//   - T21: agganciare UN messaggio agganciava tutti gli altri orfani con lo stesso Outlook
//     ConversationID, ne accettava il triage e ne assegnava proposte e riferimenti. In una
//     conversazione lunga c'è quasi sempre una mail che parla d'altro, e finiva dentro la RFQ senza
//     che nessuno l'avesse guardata;
//   - T23: il form «Nuova RFQ» aveva una casella di testo PRECOMPILATA con tutto ciò che l'estrattore
//     generico aveva visto, e al submit ogni numero diventava un identificativo `manuale` con
//     confidenza 100. Nessuno li aveva digitati, e il database registrava che qualcuno l'aveva fatto.
package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// scena3R è un cliente con le sue regole, una conversazione con tre messaggi orfani e un operatore.
type scena3R struct {
	pool    *pgxpool.Pool
	q       *db.Queries
	utente  db.Utente
	cliente db.Cliente
	conv    uuid.UUID
	msg     []uuid.UUID // in ordine di arrivo
}

const regole3R = `{
  "famiglie_codice": [{"regex": "\\b\\d{7}[A-Z]\\b", "descrizione": "7 cifre + lettera", "esempio": "1234567A"}],
  "riferimento_rfq": {"regex": "\\bRDO\\s*\\d{9}\\b", "descrizione": "RDO", "esempio": "RDO 400012345"}
}`

func prepara3R(t *testing.T) (scena3R, context.Context) {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()
	s := scena3R{pool: p, q: db.New(p)}

	if err := p.QueryRow(ctx, `INSERT INTO utente (sigla, nome, ufficio, ruolo) VALUES ('FP','Operatore','commerciale','operatore')
		RETURNING utente_id, sigla, nome, ufficio, ruolo, password_hash, attivo, creato_il`).
		Scan(&s.utente.UtenteID, &s.utente.Sigla, &s.utente.Nome, &s.utente.Ufficio, &s.utente.Ruolo,
			&s.utente.PasswordHash, &s.utente.Attivo, &s.utente.CreatoIl); err != nil {
		t.Fatalf("utente: %v", err)
	}
	c, err := s.q.InsertCliente(ctx, db.InsertClienteParams{
		CartellaNas: "ACME", RagioneSociale: "Acme S.p.A.", Regole: json.RawMessage(regole3R)})
	if err != nil {
		t.Fatalf("cliente: %v", err)
	}
	s.cliente = c
	if err := s.q.InsertDominioCliente(ctx, db.InsertDominioClienteParams{Lower: "acme.example", ClienteID: c.ClienteID}); err != nil {
		t.Fatalf("dominio: %v", err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook','CONV-3R', now()) RETURNING conversazione_id`).Scan(&s.conv); err != nil {
		t.Fatalf("conversazione: %v", err)
	}
	oggetti := []string{
		"RICHIESTA D'OFFERTA RDO 400012345",
		"R: RICHIESTA D'OFFERTA RDO 400012345",
		"R: RICHIESTA D'OFFERTA RDO 400012345 — ferie di agosto", // la mail che parla d'altro
	}
	for i, og := range oggetti {
		var id uuid.UUID
		if err := p.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento,
			mittente_indirizzo, oggetto, corpo_testo) VALUES ('outlook',$1,$2,'entrata',$3,'buyer@acme.example',$4,$5)
			RETURNING messaggio_id`, "<3r-"+string(rune('a'+i))+"@acme.example>", s.conv,
			time.Now().Add(time.Duration(i)*time.Hour), og, "codice 1234567A, CAP 98765").Scan(&id); err != nil {
			t.Fatalf("messaggio %d: %v", i, err)
		}
		// ogni messaggio ha la sua proposta di triage, come dopo un ingest vero
		if _, err := s.q.UpsertTriage(ctx, db.UpsertTriageParams{
			MessaggioID: id, Esito: db.EsitoTriageNuovaRfq, ClienteProposto: uuid.NullUUID{UUID: c.ClienteID, Valid: true},
			Identificativi: []string{}, Confidenza: 60, Motivi: json.RawMessage(`["prova"]`), Fonte: db.FonteTriageDeterministico,
		}); err != nil {
			t.Fatalf("triage %d: %v", i, err)
		}
		s.msg = append(s.msg, id)
	}
	return s, ctx
}

// T21 — la decisione dell'operatore vale per IL messaggio su cui l'ha presa.
//
// Riscritta per lo Smistamento (M1, A5.16.5): prima fissava che gli altri orfani della conversazione
// ricevessero un candidato R1 fisso a 95 e la proposta «aggancia» a 95. Adesso ricevono il candidato con il
// livello che l'indice e il cliente danno: in questa scena nessun messaggio ha il ConversationIndex, quindi
// è il solo ConversationID, debole (40), e la loro proposta resta quella di prima («nuova_rfq» a 60).
// Restano orfani, con il triage non accettato, la traccia `candidato` e la conversazione collegata. La
// variante con l'indice che discende è TestGliOrfaniDellaConversazioneHannoIlLivelloGiusto (257).
func TestT21LAggancioNonTrascinaLaConversazione(t *testing.T) {
	s, ctx := prepara3R(t)
	srv := &Server{Pool: s.pool, Log: testutil.LogSilenzioso()}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	m, _, err := messaggioDaDecidere(ctx, q, s.msg[0])
	if err != nil {
		t.Fatal(err)
	}
	th, err := q.InsertThread(ctx, db.InsertThreadParams{
		ClienteID: s.cliente.ClienteID, Canale: m.Canale, DataInizio: m.DataEvento,
		Oggetto: ptxt("RFQ 1234567A"), CartellaRelativa: ptxt(`ACME\WIP\x`), Priorita: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.agganciaMessaggioAThread(ctx, q, &s.utente, m, th.ThreadID, uuid.NullUUID{}, classificazione.GestoAggancia); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// il messaggio deciso è agganciato
	var tid uuid.NullUUID
	if err := s.pool.QueryRow(ctx, `SELECT thread_id FROM messaggio WHERE messaggio_id = $1`, s.msg[0]).Scan(&tid); err != nil {
		t.Fatal(err)
	}
	if !tid.Valid || tid.UUID != th.ThreadID {
		t.Fatalf("il messaggio deciso non è agganciato: %+v", tid)
	}

	// gli ALTRI no
	for i, id := range s.msg[1:] {
		var altro, proposto uuid.NullUUID
		var stato, esito string
		var conf int16
		if err := s.pool.QueryRow(ctx, `SELECT m.thread_id, t.stato::text, t.esito::text, t.confidenza, t.thread_proposto
			FROM messaggio m JOIN proposta_triage t USING (messaggio_id) WHERE m.messaggio_id = $1`, id).
			Scan(&altro, &stato, &esito, &conf, &proposto); err != nil {
			t.Fatal(err)
		}
		if altro.Valid {
			t.Errorf("messaggio %d: AGGANCIATO dalla decisione presa su un altro (thread %s)", i+1, altro.UUID)
		}
		if stato != "proposta" {
			t.Errorf("messaggio %d: il triage è stato ACCETTATO senza che nessuno decidesse (stato %q)", i+1, stato)
		}
		if esito != "nuova_rfq" || conf != 60 || proposto.Valid {
			t.Errorf("messaggio %d: proposta %s a %d verso %v, attesa quella di prima (nuova_rfq a 60): il solo ConversationID non la cambia", i+1, esito, conf, proposto)
		}
		cand, err := s.q.ListCandidatiAggancio(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if len(cand) != 1 || cand[0].ThreadID != th.ThreadID || cand[0].Regola != db.RegolaAggancioR1Conversazione {
			t.Errorf("messaggio %d: candidato atteso R1 verso %s, trovato %+v", i+1, th.ThreadID, cand)
		} else if cand[0].Punteggio != 40 || classificazione.TipoDaRiga(string(cand[0].Regola), int(cand[0].Punteggio)) != classificazione.TipoR1Solo {
			t.Errorf("messaggio %d: R1 a %d, atteso 40 (solo ConversationID)", i+1, cand[0].Punteggio)
		}
		// e la traccia dice che è una proposta, non una decisione presa per procura, con il livello
		var azione, motivo string
		if err := s.pool.QueryRow(ctx, `SELECT azione, COALESCE(motivo, '') FROM messaggio_aggancio_log WHERE messaggio_id = $1
			ORDER BY eseguito_il DESC LIMIT 1`, id).Scan(&azione, &motivo); err != nil {
			t.Fatalf("messaggio %d: manca la traccia: %v", i+1, err)
		}
		if azione != "candidato" || !strings.Contains(motivo, "debole · score 40") {
			t.Errorf("messaggio %d: azione registrata %q (%q), attesa «candidato» con il livello", i+1, azione, motivo)
		}
	}

	// la conversazione SÌ: collegarla è la decisione dell'operatore, ed è ciò che regge R1
	var convThread uuid.NullUUID
	if err := s.pool.QueryRow(ctx, `SELECT thread_id FROM conversazione WHERE conversazione_id = $1`, s.conv).Scan(&convThread); err != nil {
		t.Fatal(err)
	}
	if !convThread.Valid || convThread.UUID != th.ThreadID {
		t.Error("la conversazione deve restare collegata: è l'evidenza su cui si regge R1")
	}
}

// 257 — gli orfani della conversazione hanno il livello giusto. Il messaggio deciso ha il suo
// ConversationIndex; il primo orfano ne discende ed è dello stesso cliente (forte, 86: la proposta diventa
// «aggancia»), il secondo no (debole, 40: la proposta resta com'era). Tutti e due restano orfani, con la
// traccia `candidato` e il livello. Le righe già scritte seguono K25: una riga delle regole di prima si
// sostituisce, una variante più forte scritta dall'ingest non si abbassa.
func TestGliOrfaniDellaConversazioneHannoIlLivelloGiusto(t *testing.T) {
	s, ctx := prepara3R(t)
	srv := &Server{Pool: s.pool, Log: testutil.LogSilenzioso()}
	const indice = "01DB2C3D4E5F60718293A4B5C6D7E8F901A2B3C4D5E6"
	for id, idx := range map[uuid.UUID]string{s.msg[0]: indice, s.msg[1]: indice + "0000A1B2C3"} {
		if _, err := s.pool.Exec(ctx, `INSERT INTO messaggio_outlook (messaggio_id, conversation_index) VALUES ($1, $2)`, id, idx); err != nil {
			t.Fatal(err)
		}
	}
	th, err := s.q.InsertThread(ctx, db.InsertThreadParams{ClienteID: s.cliente.ClienteID, Canale: db.CanaleOutlook, DataInizio: time.Now(),
		Oggetto: ptxt("RFQ 1234567A"), CartellaRelativa: ptxt(`ACME\WIP\x`), Priorita: 1})
	if err != nil {
		t.Fatal(err)
	}
	// le righe già scritte: al primo orfano un R1 delle regole di prima (95), al secondo un R1 forte che
	// l'ingest avrebbe potuto scrivere (86)
	for id, punti := range map[uuid.UUID]int{s.msg[1]: 95, s.msg[2]: 86} {
		if _, err := s.pool.Exec(ctx, `INSERT INTO candidato_aggancio (messaggio_id, thread_id, regola, punteggio, evidenza, thread_stato)
			VALUES ($1, $2, 'R1_conversazione', $3, 'riga scritta prima', 'APERTA')`, id, th.ThreadID, punti); err != nil {
			t.Fatal(err)
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	m, _, err := messaggioDaDecidere(ctx, q, s.msg[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.agganciaMessaggioAThread(ctx, q, &s.utente, m, th.ThreadID, uuid.NullUUID{}, classificazione.GestoAggancia); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	riga := func(id uuid.UUID) (punti int16, esito string, conf int16, proposto uuid.NullUUID, orfano bool, motivo string) {
		t.Helper()
		var tid uuid.NullUUID
		if err := s.pool.QueryRow(ctx, `SELECT k.punteggio, p.esito::text, p.confidenza, p.thread_proposto, m.thread_id,
			(SELECT motivo FROM messaggio_aggancio_log l WHERE l.messaggio_id = m.messaggio_id AND l.azione = 'candidato' ORDER BY eseguito_il DESC LIMIT 1)
			FROM messaggio m JOIN proposta_triage p USING (messaggio_id)
			JOIN candidato_aggancio k ON k.messaggio_id = m.messaggio_id AND k.thread_id = $2 AND k.regola = 'R1_conversazione'
			WHERE m.messaggio_id = $1`, id, th.ThreadID).Scan(&punti, &esito, &conf, &proposto, &tid, &motivo); err != nil {
			t.Fatal(err)
		}
		return punti, esito, conf, proposto, !tid.Valid, motivo
	}
	// il primo orfano discende ed è dello stesso cliente: forte, e la proposta si alza
	punti, esito, conf, proposto, orfano, motivo := riga(s.msg[1])
	if punti != 86 || esito != "aggancia" || conf != 86 || !proposto.Valid || proposto.UUID != th.ThreadID || !orfano {
		t.Errorf("orfano con l'indice che discende: R1 %d, proposta %s %d verso %v, orfano %v", punti, esito, conf, proposto, orfano)
	}
	if !strings.Contains(motivo, "forte · score 86") || !strings.Contains(motivo, "proposto") {
		t.Errorf("la traccia del primo orfano: %q", motivo)
	}
	// il secondo no: la riga forte già scritta resta (K25), ma la proposta non la tocca il giro, che ha
	// trovato solo il ConversationID
	punti, esito, conf, proposto, orfano, motivo = riga(s.msg[2])
	if punti != 86 {
		t.Errorf("il giro degli orfani ha abbassato una riga più forte: %d", punti)
	}
	if esito != "nuova_rfq" || conf != 60 || proposto.Valid || !orfano {
		t.Errorf("orfano senza indice: proposta %s %d verso %v, orfano %v — attesa quella di prima", esito, conf, proposto, orfano)
	}
	if !strings.Contains(motivo, "debole · score 40") {
		t.Errorf("la traccia del secondo orfano: %q", motivo)
	}
	// nessuna decisione presa per procura
	var accettati int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM proposta_triage WHERE messaggio_id = ANY($1) AND stato <> 'proposta'`, s.msg[1:]).Scan(&accettati); err != nil || accettati != 0 {
		t.Errorf("proposte decise fra gli orfani: %d (%v)", accettati, err)
	}
}

// T23 — nel form «Nuova RFQ» diventano identificativi SOLO i codici spuntati, e con l'origine della
// proposta; il riferimento del cliente va nel suo campo e non fra i codici.
func TestT23SoloISpuntatiDiventanoIdentificativi(t *testing.T) {
	s, ctx := prepara3R(t)
	srv := serverConSessione(t, s)

	// i candidati che l'ingest avrebbe scritto
	id := s.msg[0]
	candidati := []db.InsertCandidatoCodiceParams{
		{MessaggioID: id, Codice: "RDO 400012345", Ruolo: db.RuoloCodiceRiferimentoRfq, Origine: db.OrigineCodiceRiferimento, Punteggio: 90, Evidenza: "oggetto"},
		{MessaggioID: id, Codice: "1234567A", Ruolo: db.RuoloCodiceProdotto, Origine: db.OrigineCodiceFamiglia, Famiglia: "7 cifre + lettera", Punteggio: 80, Evidenza: "corpo"},
		{MessaggioID: id, Codice: "98765", Ruolo: db.RuoloCodiceNonClassificato, Origine: db.OrigineCodiceGenerico, Punteggio: 30, Evidenza: "corpo"},
	}
	for _, c := range candidati {
		if err := s.q.InsertCandidatoCodice(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	form := url.Values{
		"cliente_id":          {s.cliente.ClienteID.String()},
		"buyer_id":            {""},
		"oggetto":             {"RFQ 1234567A"},
		"riferimento_cliente": {"RDO 400012345"},
		"codice":              {"1234567A"}, // spuntato solo questo: il CAP resta fuori
		"identificativi":      {"XYZ-9999"}, // digitato a mano
		"priorita":            {"1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/messaggio/"+id.String()+"/rfq", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id.String())
	req = req.WithContext(context.WithValue(ctx, ctxUtente, &s.utente))
	w := httptest.NewRecorder()
	srv.nuovaRFQ(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("nuovaRFQ ha risposto %d: %s", w.Code, w.Body.String())
	}

	var threadID uuid.UUID
	var riferimento *string
	if err := s.pool.QueryRow(ctx, `SELECT t.thread_id, t.riferimento_cliente FROM thread_offerta t
		JOIN messaggio m ON m.thread_id = t.thread_id WHERE m.messaggio_id = $1`, id).Scan(&threadID, &riferimento); err != nil {
		t.Fatalf("la RFQ non è stata creata: %v", err)
	}
	if riferimento == nil || *riferimento != "RDO 400012345" {
		t.Errorf("il riferimento del cliente non è nel suo campo: %v", riferimento)
	}

	righe, err := s.q.ListIdentificativi(ctx, threadID)
	if err != nil {
		t.Fatal(err)
	}
	trovati := map[string]db.IdentificativoThread{}
	for _, r := range righe {
		trovati[r.Codice] = r
	}
	if len(trovati) != 2 {
		t.Fatalf("identificativi creati: %v (attesi 2: quello spuntato e quello digitato)", trovati)
	}
	if _, esiste := trovati["98765"]; esiste {
		t.Error("il CAP è diventato un identificativo della RFQ senza che nessuno lo spuntasse")
	}
	if _, esiste := trovati["400012345"]; esiste {
		t.Error("il numero della RDO è diventato un codice prodotto")
	}
	if r, ok := trovati["1234567A"]; !ok {
		t.Error("il codice spuntato non è entrato")
	} else {
		if r.Origine != db.OrigineIdentificativoPropostaFamiglia {
			t.Errorf("origine %q: un codice spuntato fra le proposte non è «manuale», manuale è quello digitato", r.Origine)
		}
		if !r.Confidenza.Valid || r.Confidenza.Int16 != 80 {
			t.Errorf("confidenza %+v, atteso 80 (il punteggio della proposta)", r.Confidenza)
		}
	}
	if r, ok := trovati["XYZ-9999"]; !ok {
		t.Error("il codice digitato a mano non è entrato")
	} else if r.Origine != db.OrigineIdentificativoManuale {
		t.Errorf("origine %q per un codice digitato: atteso «manuale»", r.Origine)
	}
}

// Spuntare un codice che non era fra i candidati non lo fa entrare: la spunta sceglie fra ciò che il
// sistema ha proposto, non è un secondo campo di testo travestito.
func TestUnCodiceInventatoNonEntraDallaSpunta(t *testing.T) {
	s, ctx := prepara3R(t)
	srv := serverConSessione(t, s)
	id := s.msg[0]
	if err := s.q.InsertCandidatoCodice(ctx, db.InsertCandidatoCodiceParams{
		MessaggioID: id, Codice: "1234567A", Ruolo: db.RuoloCodiceProdotto, Origine: db.OrigineCodiceFamiglia,
		Punteggio: 80, Evidenza: "corpo"}); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"cliente_id": {s.cliente.ClienteID.String()}, "oggetto": {"RFQ"}, "priorita": {"1"},
		"codice": {"1234567A", "MAI-PROPOSTO-1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/messaggio/"+id.String()+"/rfq", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", id.String())
	req = req.WithContext(context.WithValue(ctx, ctxUtente, &s.utente))
	w := httptest.NewRecorder()
	srv.nuovaRFQ(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("risposta %d: %s", w.Code, w.Body.String())
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM identificativo_thread WHERE codice = 'MAI-PROPOSTO-1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("un codice mai proposto è entrato perché qualcuno ha spedito il suo nome nel form")
	}
}

// serverConSessione e' il Server dei test con i template caricati (l'handler risponde con un
// frammento HTML) e il pool del database di prova.
func serverConSessione(t *testing.T, s scena3R) *Server {
	t.Helper()
	srv := serverTest(t)
	srv.Pool = s.pool
	srv.Log = testutil.LogSilenzioso()
	return srv
}
