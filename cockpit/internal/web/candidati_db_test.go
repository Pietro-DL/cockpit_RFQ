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

	"promatec/cockpit/internal/domain"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/testutil"
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
  "famiglie_codice": [{"regex": "\\b\\d{7}[A-Z]\\b", "descrizione": "7 cifre + lettera", "esempio": "6674611A"}],
  "riferimento_rfq": {"regex": "\\bRDO\\s*\\d{9}\\b", "descrizione": "RDO", "esempio": "RDO 490020618"}
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
		"RICHIESTA D'OFFERTA RDO 490020618",
		"R: RICHIESTA D'OFFERTA RDO 490020618",
		"R: RICHIESTA D'OFFERTA RDO 490020618 — ferie di agosto", // la mail che parla d'altro
	}
	for i, og := range oggetti {
		var id uuid.UUID
		if err := p.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento,
			mittente_indirizzo, oggetto, corpo_testo) VALUES ('outlook',$1,$2,'entrata',$3,'buyer@acme.example',$4,$5)
			RETURNING messaggio_id`, "<3r-"+string(rune('a'+i))+"@acme.example>", s.conv,
			time.Now().Add(time.Duration(i)*time.Hour), og, "codice 6674611A, CAP 61032").Scan(&id); err != nil {
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
// Gli altri orfani della conversazione restano orfani, acquistano un candidato R1 al 95, e la loro
// proposta diventa «aggancia» invece di «nuova_rfq». Sono un clic ciascuno, ed è un clic che qualcuno
// deve dare.
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
		Oggetto: ptxt("RFQ 6674611A"), CartellaRelativa: ptxt(`ACME\WIP\x`), Priorita: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.agganciaMessaggioAThread(ctx, q, &s.utente, m, th.ThreadID, uuid.NullUUID{}); err != nil {
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
		var altro uuid.NullUUID
		var stato, esito string
		var conf int16
		if err := s.pool.QueryRow(ctx, `SELECT m.thread_id, t.stato::text, t.esito::text, t.confidenza
			FROM messaggio m JOIN proposta_triage t USING (messaggio_id) WHERE m.messaggio_id = $1`, id).
			Scan(&altro, &stato, &esito, &conf); err != nil {
			t.Fatal(err)
		}
		if altro.Valid {
			t.Errorf("messaggio %d: AGGANCIATO dalla decisione presa su un altro (thread %s)", i+1, altro.UUID)
		}
		if stato != "proposta" {
			t.Errorf("messaggio %d: il triage è stato ACCETTATO senza che nessuno decidesse (stato %q)", i+1, stato)
		}
		if esito != "aggancia" {
			t.Errorf("messaggio %d: esito %q, atteso «aggancia» (il candidato R1 è comparso)", i+1, esito)
		}
		if int(conf) != domain.PuntiRegola[domain.R1Conversazione] {
			t.Errorf("messaggio %d: confidenza %d, attesa %d", i+1, conf, domain.PuntiRegola[domain.R1Conversazione])
		}
		cand, err := s.q.ListCandidatiAggancio(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if len(cand) != 1 || cand[0].ThreadID != th.ThreadID || cand[0].Regola != db.RegolaAggancioR1Conversazione {
			t.Errorf("messaggio %d: candidato atteso R1 verso %s, trovato %+v", i+1, th.ThreadID, cand)
		}
		// e la traccia dice che è una proposta, non una decisione presa per procura
		var azione string
		if err := s.pool.QueryRow(ctx, `SELECT azione FROM messaggio_aggancio_log WHERE messaggio_id = $1
			ORDER BY eseguito_il DESC LIMIT 1`, id).Scan(&azione); err != nil {
			t.Fatalf("messaggio %d: manca la traccia: %v", i+1, err)
		}
		if azione != "candidato" {
			t.Errorf("messaggio %d: azione registrata %q, attesa «candidato»", i+1, azione)
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

// T23 — nel form «Nuova RFQ» diventano identificativi SOLO i codici spuntati, e con l'origine della
// proposta; il riferimento del cliente va nel suo campo e non fra i codici.
func TestT23SoloISpuntatiDiventanoIdentificativi(t *testing.T) {
	s, ctx := prepara3R(t)
	srv := serverConSessione(t, s)

	// i candidati che l'ingest avrebbe scritto
	id := s.msg[0]
	candidati := []db.InsertCandidatoCodiceParams{
		{MessaggioID: id, Codice: "RDO 490020618", Ruolo: db.RuoloCodiceRiferimentoRfq, Origine: db.OrigineCodiceRiferimento, Punteggio: 90, Evidenza: "oggetto"},
		{MessaggioID: id, Codice: "6674611A", Ruolo: db.RuoloCodiceProdotto, Origine: db.OrigineCodiceFamiglia, Famiglia: "7 cifre + lettera", Punteggio: 80, Evidenza: "corpo"},
		{MessaggioID: id, Codice: "61032", Ruolo: db.RuoloCodiceNonClassificato, Origine: db.OrigineCodiceGenerico, Punteggio: 30, Evidenza: "corpo"},
	}
	for _, c := range candidati {
		if err := s.q.InsertCandidatoCodice(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	form := url.Values{
		"cliente_id":          {s.cliente.ClienteID.String()},
		"buyer_id":            {""},
		"oggetto":             {"RFQ 6674611A"},
		"riferimento_cliente": {"RDO 490020618"},
		"codice":              {"6674611A"}, // spuntato solo questo: il CAP resta fuori
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
	if riferimento == nil || *riferimento != "RDO 490020618" {
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
	if _, esiste := trovati["61032"]; esiste {
		t.Error("il CAP è diventato un identificativo della RFQ senza che nessuno lo spuntasse")
	}
	if _, esiste := trovati["490020618"]; esiste {
		t.Error("il numero della RDO è diventato un codice prodotto")
	}
	if r, ok := trovati["6674611A"]; !ok {
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
		MessaggioID: id, Codice: "6674611A", Ruolo: db.RuoloCodiceProdotto, Origine: db.OrigineCodiceFamiglia,
		Punteggio: 80, Evidenza: "corpo"}); err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"cliente_id": {s.cliente.ClienteID.String()}, "oggetto": {"RFQ"}, "priorita": {"1"},
		"codice": {"6674611A", "MAI-PROPOSTO-1"},
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
