//go:build integrazione

// L4 — C-6b, INC-G25 (corsia file; direttiva 39 §7 e §8 [U + T-39]; piano della direttiva 35 INC-G25 [T]): i gesti delle
// pagine legacy, della vecchia pagina del Motore A e della Sorgente in concorrenza con R5 formato 3 vero
// (distintabozza.Conferma3), senza 40P01 e senza effetti parziali.
//
// La scena: una RFQ della Sorgente con il prodotto finito ACME-P1 e due componenti; il PDF D2 ha una AR legacy su
// ACME-C1, nata quando la RFQ era interamente legacy; la bozza formato 3 (partenza registrato) porta lo svincolo di D2
// scritto da «Togli» della pagina legacy (che nel formato 3 diventa R2); S8 dalla rotta vera conferma lo STEP del
// prodotto come sorgente della bozza (partenza solo_prodotto, con il calcolo del motore sostituito come nella L4 di S8);
// R4 dalla rotta da' la firma. Poi:
//   - R5 prima (TestC6bConcorrenzaR5PrimaIGestiAspettano): R5 tiene la RFQ (dopo S, prima dei lucchetti delle righe,
//     l'aggancio DopoS) e dieci gesti partono insieme; ognuno aspetta sulla riga della RFQ, nessuno su una riga che R5
//     blocca dopo; R5 conferma, poi i gesti vedono lo stato dopo la conferma;
//   - il gesto prima (TestC6bConcorrenzaGestoPrimaR5Aspetta): una transazione tiene la RFQ, il gesto e poi R5 si mettono
//     in fila (FIFO dei lucchetti di riga), il gesto passa per primo, R5 dopo rilegge e risponde senza scrivere niente di
//     parziale («Scollega e scarta», lo scarto e la revoca dello STEP, «Assegna» legacy che diventa R2, il vecchio «Scarta
//     il file» con la RFQ FOR KEY SHARE).
// Nessun 40P01: R5 traduce 40P01/55P03/40001 nel 409 «riprova» con un testo proprio, che qui fa cadere la prova; i gesti
// non rispondono 5xx; il contatore deadlocks di pg_stat_database non cresce. Solo dati sintetici.

package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	sorgentev1 "promatec/cockpit/internal/core/rfq/sorgente"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// scenaR5C6b: la scena di scenaC6b con la AR legacy di D2, la bozza formato 3 con lo svincolo e la sorgente S8.
type scenaR5C6b struct {
	*scenaC6b
	admin               uuid.UUID
	bozza               uuid.UUID
	shaStep             string
	spia                *pgx.Conn // una connessione fuori dal pool: le attese e i contatori
	custode             *pgx.Conn // un'altra, per il lucchetto della RFQ delle prove (in una transazione la spia vedrebbe sempre la stessa fotografia di pg_stat_activity)
	stalliIn            int64
	pdf3, pdf4, pdf5    uuid.UUID
	prop3, prop4, prop5 uuid.UUID
}

func nuovaScenaR5C6b(t *testing.T) *scenaR5C6b {
	t.Helper()
	s := &scenaR5C6b{scenaC6b: nuovaScenaC6b(t)}
	s.admin = uuidSQL(t, s.bancoWeb, `SELECT utente_id FROM utente WHERE sigla = 'AD'`)
	// R4 confermabile e R5 vogliono il Motore A calcolato (blocco «R4 completo» della persistenza): le regole ACME pronte
	// (aggancio della corsia interfaccia all'unione con v1-persistenza a9b7aa9)
	accendiMotoreV1(t, s.bancoWeb, s.thread)
	// tre PDF senza legami, per i gesti che non devono incrociarsi fra loro
	s.pdf3, _ = s.file(s.thread, "ACME-D3.pdf", byteCasuali("x.pdf", 611, 1_500), db.OrigineAllegatoOutlook)
	s.pdf4, _ = s.file(s.thread, "ACME-D4.pdf", byteCasuali("x.pdf", 612, 1_500), db.OrigineAllegatoOutlook)
	s.pdf5, _ = s.file(s.thread, "ACME-D5.pdf", byteCasuali("x.pdf", 613, 1_500), db.OrigineAllegatoOutlook)
	s.prop3, s.prop4, s.prop5 = s.proposta(s.pdf3), s.proposta(s.pdf4), s.proposta(s.pdf5)
	// la AR legacy di D2 (RFQ interamente legacy), poi la bozza formato 3 e lo svincolo dalla pagina legacy (R2)
	if a := s.assegna(s.prop2, &s.c); !strings.Contains(a, "assegnato al componente ACME-C1") {
		t.Fatalf("AR legacy: %q", a)
	}
	s.bozza, _ = s.creaBozza(distintabozza.PartenzaRegistrato)
	if a := s.assegna(s.prop2, nil); !strings.Contains(a, "preparato nella bozza") {
		t.Fatalf("svincolo nella bozza: %q", a)
	}
	_, v := s.snapshot()
	// S8 dalla rotta: lo STEP del prodotto come sorgente della bozza manuale (solo_prodotto sulla stessa bozza)
	s.shaStep = valoreDi(t, s.bancoWeb, `SELECT sha256 FROM allegato WHERE allegato_id = $1`, s.step)
	preparaS8Prova = func(context.Context, uuid.UUID, uuid.UUID, sorgentev1.RichiestaS8) (sorgentev1.Preparazione, error) {
		return sorgentev1.Preparazione{StatoAnalisi: sorgentev1.AnalisiInCorso}, nil
	}
	t.Cleanup(func() { preparaS8Prova = nil })
	c, _ := json.Marshal(map[string]any{"chiave_idempotenza": uuid.New(), "prodotto_id": s.p, "partenza": "solo_prodotto",
		"stato_analisi": "in_analisi", "presa_d_atto": true, "motivo": "lo STEP e' ancora in analisi",
		"bozza": map[string]any{"bozza_id": s.bozza, "versione_attesa": v}})
	resp, testo := s.ad.chiediJSON(http.MethodPost, "/thread/"+s.thread.String()+"/sorgente/step/"+s.step.String()+"/conferma-sorgente", string(c))
	if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"esito":"completata"`) {
		t.Fatalf("S8: %d %s", resp.StatusCode, testo)
	}
	// la connessione spia
	spia, err := pgx.Connect(s.ctx, testutil.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = spia.Close(context.Background()) })
	s.spia = spia
	custode, err := pgx.Connect(s.ctx, testutil.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = custode.Close(context.Background()) })
	s.custode = custode
	s.stalliIn = s.stalli()
	return s
}

// versione: la versione corrente della bozza della scena.
func (s *scenaR5C6b) versione() int {
	s.t.Helper()
	var v int
	if err := s.pool.QueryRow(s.ctx, `SELECT versione FROM distinta_bozza WHERE bozza_id = $1`, s.bozza).Scan(&v); err != nil {
		s.t.Fatal(err)
	}
	return v
}

// corpoBozza: lo snapshot salvato della versione, con bozza_id e bozza_versione (come la pagina lo manda a R4 e R5).
func (s *scenaR5C6b) corpoBozza(v int) map[string]any {
	s.t.Helper()
	var raw json.RawMessage
	if err := s.pool.QueryRow(s.ctx, `SELECT snapshot FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = $2`, s.bozza, v).Scan(&raw); err != nil {
		s.t.Fatal(err)
	}
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		s.t.Fatal(err)
	}
	m["bozza_id"], m["bozza_versione"] = s.bozza, v
	return m
}

// r4: il riepilogo della versione corrente dalla rotta; la firma e la versione.
func (s *scenaR5C6b) r4() (distintabozza.RiepilogoR4, int) {
	s.t.Helper()
	v := s.versione()
	raw, _ := json.Marshal(s.corpoBozza(v))
	resp, testo := s.ad.chiediJSON(http.MethodPost, "/thread/"+s.thread.String()+"/motore-a/bom/riepilogo?avvisi_motore=1", string(raw))
	var r distintabozza.RiepilogoR4
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &r) != nil {
		s.t.Fatalf("R4: %d %s", resp.StatusCode, testo)
	}
	return r, v
}

// richiestaR5: il corpo di R5 (la forma del ponte confermaBom) letto dal lettore vero.
func (s *scenaR5C6b) richiestaR5(firma string, v int) distintabozza.RichiestaR5 {
	s.t.Helper()
	m := s.corpoBozza(v)
	m["chiave_idempotenza"] = uuid.New()
	raw, _ := json.Marshal(map[string]any{"firma_anteprima": firma, "bozza": m})
	r, err := distintabozza.LeggiRichiestaR5(raw)
	if err != nil {
		s.t.Fatalf("corpo di R5: %v", err)
	}
	return r
}

// r5: R5 vero (il core della rotta), con l'aggancio DopoS delle prove.
func (s *scenaR5C6b) r5(r distintabozza.RichiestaR5, dopoS func()) (distintabozza.ConfermaR5, error) {
	return distintabozza.Conferma3(context.Background(), s.pool, s.thread, r, distintabozza.OpzioniR5{UtenteID: s.admin,
		ImprontaRegole: improntaRegoleV1(s.ws.MotoreA), DopoS: dopoS, Motore: s.ws.motoreR5})
}

// stalloR5: R5 ha incontrato un 40P01 (o un 55P03/40001): il 409 «riprova» con il testo della traduzione (traduciR5).
func stalloR5(err error) bool {
	var e *distintabozza.Errore
	return errors.As(err, &e) && e.Codice == distintabozza.CodiceAnteprimaSuperata && e.Motivo == distintabozza.MotivoRiprova &&
		strings.Contains(e.Messaggio, "un'altra operazione sulla stessa RFQ")
}

// inAttesa: le transazioni del database di prova ferme su un lucchetto; sullaRFQ: solo quelle sulla riga della RFQ
// (la loro istruzione legge thread_offerta: BloccaThread o BloccaThreadCondiviso).
func (s *scenaR5C6b) inAttesa(sullaRFQ bool) int {
	s.t.Helper()
	q := `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid() AND wait_event_type = 'Lock'
		AND pid <> ` + fmt.Sprint(s.custode.PgConn().PID())
	if sullaRFQ {
		q += ` AND query ILIKE '%FROM thread_offerta%'`
	}
	var n int
	if err := s.spia.QueryRow(s.ctx, q).Scan(&n); err != nil {
		s.t.Fatal(err)
	}
	return n
}

// aspettaAttese aspetta che almeno n transazioni siano ferme (sulla RFQ, se sullaRFQ), fino a entro; vero se ci sono.
func (s *scenaR5C6b) aspettaAttese(n int, sullaRFQ bool, entro time.Duration) bool {
	s.t.Helper()
	fine := time.Now().Add(entro)
	for time.Now().Before(fine) {
		if s.inAttesa(sullaRFQ) >= n {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}

// attese: chi aspetta, per il messaggio di una prova che cade.
func (s *scenaR5C6b) attese() string {
	s.t.Helper()
	rr, err := s.spia.Query(s.ctx, `SELECT coalesce(wait_event, '') || ' ' || left(regexp_replace(query, '\s+', ' ', 'g'), 90) FROM pg_stat_activity
		WHERE datname = current_database() AND pid <> pg_backend_pid() AND state <> 'idle' ORDER BY query_start`)
	if err != nil {
		s.t.Fatal(err)
	}
	defer rr.Close()
	var out []string
	for rr.Next() {
		var x string
		if err := rr.Scan(&x); err != nil {
			s.t.Fatal(err)
		}
		out = append(out, x)
	}
	return strings.Join(out, "\n")
}

// stalli: il contatore deadlocks del database di prova. I contatori delle altre connessioni arrivano in pg_stat_database
// quando si chiudono (o dopo un po' che sono inattive): prima si chiudono le connessioni del pool e si aspetta che
// escano, poi si legge dalla connessione spia.
func (s *scenaR5C6b) stalli() int64 {
	s.t.Helper()
	s.pool.Reset()
	fine := time.Now().Add(5 * time.Second)
	for time.Now().Before(fine) {
		var n int
		if err := s.spia.QueryRow(s.ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid()
			AND pid <> $1 AND backend_type = 'client backend'`, s.custode.PgConn().PID()).Scan(&n); err != nil {
			s.t.Fatal(err)
		}
		if n == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := s.spia.Exec(s.ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
		s.t.Fatal(err)
	}
	var d int64
	if err := s.spia.QueryRow(s.ctx, `SELECT deadlocks FROM pg_stat_database WHERE datname = current_database()`).Scan(&d); err != nil {
		s.t.Fatal(err)
	}
	return d
}

// controllaNessunoStallo: il contatore deadlocks non e' cresciuto dall'inizio della scena.
func (s *scenaR5C6b) controllaNessunoStallo() {
	s.t.Helper()
	if d := s.stalli(); d != s.stalliIn {
		s.t.Errorf("40P01: pg_stat_database.deadlocks da %d a %d", s.stalliIn, d)
	}
}

// improntaDominio: l'impronta delle tabelle che i gesti e R5 scrivono, letta dalla spia (fuori dal pool, che i gesti in
// fila possono occupare tutto).
func (s *scenaR5C6b) improntaDominio() string {
	s.t.Helper()
	var out []string
	for _, tab := range []string{"documento_proposta", "documento", "file_rfq", "file_rfq_associazione", "associazione_evento",
		"componente", "componente_relazione", "fonte_step_evento", "distinta_bozza", "esito_intento", "componente_proposta"} {
		var x string
		if err := s.spia.QueryRow(s.ctx, `SELECT count(*)::text || ':' || coalesce(md5(string_agg(t::text, '|' ORDER BY t::text)), '-') FROM `+tab+` t`).Scan(&x); err != nil {
			s.t.Fatalf("%s: %v", tab, err)
		}
		out = append(out, tab+"="+x)
	}
	return strings.Join(out, " ")
}

// tieniRFQ: una transazione della connessione custode che tiene la RFQ FOR UPDATE (il gesto e R5 si mettono in fila dietro).
func (s *scenaR5C6b) tieniRFQ() pgx.Tx {
	s.t.Helper()
	tx, err := s.custode.Begin(s.ctx)
	if err != nil {
		s.t.Fatal(err)
	}
	if _, err := tx.Exec(s.ctx, `SELECT 1 FROM thread_offerta WHERE thread_id = $1 FOR UPDATE`, s.thread); err != nil {
		s.t.Fatal(err)
	}
	return tx
}

// ------------------------------------------------------------------ i gesti, senza testing.T (girano in goroutine)

type esitoGestoR5 struct {
	nome  string
	stato int
	corpo string
	err   error
}

// esito: l'esito leggibile (il <div data-ma-esito> del ponte, l'avviso della pagina, o il corpo JSON).
func (e esitoGestoR5) esito() string {
	if m := reEsitoPonte.FindStringSubmatch(e.corpo); m != nil {
		return m[1] + ": " + leggibile(m[2])
	}
	if a := avvisoC6b(e.corpo); a != "" {
		return a
	}
	if len(e.corpo) > 300 {
		return e.corpo[:300]
	}
	return e.corpo
}

// stallo: il gesto ha visto un 40P01 o ha risposto 5xx.
func (e esitoGestoR5) stallo() bool {
	c := strings.ToLower(e.corpo)
	return e.err != nil || e.stato >= 500 || strings.Contains(c, "40p01") || strings.Contains(c, "deadlock") || strings.Contains(c, "55p03")
}

type gestoR5 struct {
	nome string
	fai  func() esitoGestoR5
}

func (s *scenaR5C6b) grezza(nome string, w *browser, metodo, percorso string, corpo []byte, intest map[string]string) esitoGestoR5 {
	var in io.Reader
	if corpo != nil {
		in = strings.NewReader(string(corpo))
	}
	r, err := http.NewRequest(metodo, s.srv.URL+percorso, in)
	if err != nil {
		return esitoGestoR5{nome: nome, err: err}
	}
	r.Header.Set("X-Prova-IP", w.ip)
	for k, v := range intest {
		r.Header.Set(k, v)
	}
	resp, err := w.c.Do(r)
	if err != nil {
		return esitoGestoR5{nome: nome, err: err}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return esitoGestoR5{nome: nome, stato: resp.StatusCode, corpo: string(b)}
}

// dalPonte: un gesto della vecchia pagina del Motore A (POST con HX-Current-URL sulla pagina del Motore A).
func (s *scenaR5C6b) dalPonte(nome, percorso string, form url.Values) gestoR5 {
	return gestoR5{nome: nome, fai: func() esitoGestoR5 {
		return s.grezza(nome, s.op, http.MethodPost, percorso, []byte(form.Encode()), map[string]string{
			"Content-Type": "application/x-www-form-urlencoded;charset=UTF-8", "HX-Request": "true", "HX-Current-URL": s.pontePagina(s.thread)})
	}}
}

// dalFascicolo: un gesto della pagina del Fascicolo (htmx).
func (s *scenaR5C6b) dalFascicolo(nome, percorso string, form url.Values) gestoR5 {
	return gestoR5{nome: nome, fai: func() esitoGestoR5 {
		return s.grezza(nome, s.op, http.MethodPost, percorso, []byte(form.Encode()), map[string]string{
			"Content-Type": "application/x-www-form-urlencoded", "HX-Request": "true"})
	}}
}

// dallaSorgente: un gesto JSON della Sorgente dall'amministratore, con il corpo composto adesso dal modello (S1).
func (s *scenaR5C6b) dallaSorgente(nome string, allegato uuid.UUID, gesto, motivo string, extra map[string]any) gestoR5 {
	c, _ := json.Marshal(s.corpo(s.thread, allegato, gesto, motivo, extra))
	return gestoR5{nome: nome, fai: func() esitoGestoR5 {
		return s.grezza(nome, s.ad, http.MethodPost, rottaFile(s.thread, allegato, gesto), c, map[string]string{
			"Content-Type": "application/json", "Accept": "application/json"})
	}}
}

// revocaS8: la revoca della sorgente dalla rotta (direttiva 39 §2).
func (s *scenaR5C6b) revocaS8() gestoR5 {
	c, _ := json.Marshal(map[string]any{"chiave_idempotenza": uuid.New(), "prodotto_id": s.p, "motivo": "STEP scelto per errore"})
	return gestoR5{nome: "revoca della sorgente (S8)", fai: func() esitoGestoR5 {
		return s.grezza("revoca della sorgente (S8)", s.ad, http.MethodPost,
			"/thread/"+s.thread.String()+"/sorgente/step/"+s.step.String()+"/revoca-sorgente", c, map[string]string{"Content-Type": "application/json"})
	}}
}

// ------------------------------------------------------------------ le prove

// R5 prima: R5 tiene la RFQ dopo S (DopoS) e dieci gesti partono insieme. Ognuno si ferma sulla riga della RFQ (il
// primo lucchetto, INC-G25), nessuno su una riga che R5 blocca dopo (documenti, componente, proposte): R5 prende i suoi
// lucchetti e conferma. Poi i gesti, uno alla volta, vedono la RFQ dopo la conferma: nessun 5xx, nessun 40P01, e gli
// effetti attesi (lo svincolo applicato una volta sola da R5; lo STEP promosso non si scarta; i gesti atomici della RFQ
// formato 3 senza bozza aperta rifiutano senza scrivere AR).
func TestC6bConcorrenzaR5PrimaIGestiAspettano(t *testing.T) {
	s := nuovaScenaR5C6b(t)
	rp, v := s.r4()
	if len(rp.Blocchi) != 0 || !rp.Confermabile {
		t.Fatalf("R4 della scena: %+v", rp.Blocchi)
	}
	base := s.base()
	gesti := []gestoR5{
		s.dallaSorgente("«Scollega e scarta» di D2", s.pdf2, "scarta", "doppione", map[string]any{"scollega": true}),
		s.dallaSorgente("«Scarta» dello STEP sorgente", s.step, "scarta", "doppione", map[string]any{"scollega": true}),
		s.dalPonte("«Revoca» legacy dello STEP strutturale", base+"/componente/"+s.p.String()+"/step-strutturale/revoca",
			url.Values{"sha": {s.shaStep}, "motivo": {"prova di concorrenza"}}),
		s.revocaS8(),
		s.dalPonte("«Assegna» legacy di D1 su ACME-C2", base+"/assegna", url.Values{"proposta": {s.prop1.String()}, "componente": {s.c2.String()}}),
		s.dalPonte("vecchio «Scarta il file» di D3", "/proposta/"+s.prop3.String()+"/scarta", url.Values{}),
		s.dalPonte("«Documentazione generale» di D4", base+"/file/"+s.prop4.String()+"/generale", url.Values{"tipo": {"altro"}}),
		s.dalFascicolo("decisione sulla proposta di D5", base+"/proposta/"+s.prop5.String()+"/decidi",
			url.Values{"tipo": {"disegno_2d"}, "codice": {"ACME-D5"}, "rev": {"A"}}),
		s.dalFascicolo("«Rianalizza»", base+"/rianalizza", url.Values{}),
		s.dalFascicolo("«Sposta» di ACME-C2 sotto ACME-C1", base+"/componente/"+s.c2.String()+"/sposta",
			url.Values{"da": {s.p.String()}, "a": {s.c.String()}, "qta": {"1"}}),
	}
	esiti := make([]esitoGestoR5, len(gesti))
	var wg sync.WaitGroup
	var tuttiInFila bool
	var fila, improntaPrima, improntaInFila string
	out, err := s.r5(s.richiestaR5(rp.FirmaAnteprima, v), func() {
		improntaPrima = s.improntaDominio()
		for i, g := range gesti {
			wg.Add(1)
			go func(i int, g gestoR5) {
				defer wg.Done()
				esiti[i] = g.fai()
			}(i, g)
		}
		tuttiInFila = s.aspettaAttese(len(gesti), true, 20*time.Second)
		time.Sleep(300 * time.Millisecond) // un gesto che non aspettasse la RFQ avrebbe il tempo di scrivere
		improntaInFila = s.improntaDominio()
		fila = s.attese()
	})
	wg.Wait()
	if !tuttiInFila {
		t.Errorf("i gesti non sono tutti fermi sulla riga della RFQ mentre R5 la tiene:\n%s", fila)
	}
	// nessun gesto ha scritto mentre R5 teneva la RFQ (prima dei lucchetti delle righe): ognuno era fermo al primo lucchetto
	if improntaInFila != improntaPrima {
		t.Errorf("un gesto ha scritto mentre R5 teneva la RFQ:\n%s\n%s", improntaPrima, improntaInFila)
	}
	if err != nil {
		var rf *distintabozza.RifiutoR5
		if errors.As(err, &rf) {
			t.Fatalf("R5 con i gesti in fila: %v, blocchi %+v (stallo: %v)", err, rf.Blocchi, stalloR5(err))
		}
		t.Fatalf("R5 con i gesti in fila: %v (stallo: %v)", err, stalloR5(err))
	}
	if out.Esito != "confermata" || !out.Fonte.Promossa || out.SvincoliApplicati != 1 {
		t.Fatalf("R5: esito %s, promossa %v, svincoli %d/%d", out.Esito, out.Fonte.Promossa, out.SvincoliApplicati, out.SvincoliGiaChiusi)
	}
	perNome := map[string]esitoGestoR5{}
	for _, e := range esiti {
		t.Logf("%s: %d %s", e.nome, e.stato, e.esito())
		if e.stallo() {
			t.Errorf("%s: %d %v %.400s", e.nome, e.stato, e.err, e.corpo)
		}
		perNome[e.nome] = e
	}
	// lo svincolo di D2 applicato una volta sola, da R5; «Scollega e scarta» dopo non trova piu' la AR da chiudere
	if got := s.ar(s.pdf2); !strings.HasPrefix(got, "ACME-C1:gesto_legacy:chiusa:") || strings.Count(got, ":chiusa:") != 1 {
		t.Errorf("AR di D2 dopo R5 e il gesto: %q", got)
	}
	if e := perNome["«Scollega e scarta» di D2"]; e.stato == http.StatusOK && s.statoDB(s.thread, s.pdf2) != "scartato" {
		t.Errorf("«Scollega e scarta» 200 ma D2 non e' scartato: %q", s.statoDB(s.thread, s.pdf2))
	}
	// lo STEP promosso da R5 non si scarta (D-1: documento confermato)
	if e := perNome["«Scarta» dello STEP sorgente"]; e.stato != http.StatusUnprocessableEntity || !strings.Contains(e.corpo, "DIPENDENZE_ATTIVE") {
		t.Errorf("scarto dello STEP promosso: %d %.300s", e.stato, e.corpo)
	}
	if st := s.statoDB(s.thread, s.step); st == "scartato" {
		t.Errorf("STEP sorgente scartato: %q", st)
	}
	// RFQ formato 3, bozza confermata: «Assegna» non trova una bozza aperta e non scrive AR
	if e := perNome["«Assegna» legacy di D1 su ACME-C2"]; !strings.HasPrefix(e.esito(), "no") || s.ar(s.pdf1) != "" {
		t.Errorf("«Assegna» dopo R5: %s, AR %q", e.esito(), s.ar(s.pdf1))
	}
	// i gesti sui file senza legami vanno avanti dopo R5 (il vecchio «Scarta il file» risponde con il pannello del messaggio)
	if e := perNome["vecchio «Scarta il file» di D3"]; !strings.Contains(e.esito(), "Proposta scartata") {
		t.Errorf("vecchio «Scarta il file» dopo R5: %s", e.esito())
	}
	if e := perNome["«Documentazione generale» di D4"]; !strings.HasPrefix(e.esito(), "ok") {
		t.Errorf("«Documentazione generale» dopo R5: %s", e.esito())
	}
	// Prova 11 e D-ASS-04: nessuna copia diverge dalla AR attiva, al piu' una AR attiva per file
	if n := s.divergenze(); n != 0 {
		t.Errorf("copie divergenti dalla AR attiva: %d", n)
	}
	if n := s.conteggio(`SELECT count(*) FROM (SELECT allegato_id FROM file_rfq_associazione WHERE stato = 'attiva' GROUP BY allegato_id HAVING count(*) > 1) x`); n != 0 {
		t.Errorf("file con piu' di una AR attiva: %d", n)
	}
	if n := s.conteggio(`SELECT count(*) FROM conferma_strutturale`); n != 1 {
		t.Errorf("conferme: %d", n)
	}
	s.controllaNessunoStallo()
}

// gestoPrimaDiR5: la spia tiene la RFQ; il gesto parte e si ferma sulla RFQ; R5 parte e si ferma dietro; la spia lascia:
// il gesto passa per primo (FIFO dei lucchetti di riga), R5 dopo. Ritorna l'esito del gesto e quello di R5.
func (s *scenaR5C6b) gestoPrimaDiR5(g gestoR5, r distintabozza.RichiestaR5) (esitoGestoR5, distintabozza.ConfermaR5, error) {
	s.t.Helper()
	tx := s.tieniRFQ()
	chG := make(chan esitoGestoR5, 1)
	go func() { chG <- g.fai() }()
	if !s.aspettaAttese(1, true, 10*time.Second) {
		_ = tx.Rollback(s.ctx)
		s.t.Fatalf("%s non si ferma sulla RFQ:\n%s", g.nome, s.attese())
	}
	type esitoR5 struct {
		out distintabozza.ConfermaR5
		err error
	}
	chR := make(chan esitoR5, 1)
	go func() {
		out, err := s.r5(r, nil)
		chR <- esitoR5{out, err}
	}()
	if !s.aspettaAttese(2, true, 10*time.Second) {
		_ = tx.Rollback(s.ctx)
		s.t.Fatalf("R5 non si ferma sulla RFQ dietro a %s:\n%s", g.nome, s.attese())
	}
	if err := tx.Rollback(s.ctx); err != nil {
		s.t.Fatal(err)
	}
	e := <-chG
	x := <-chR
	s.t.Logf("%s: %d %s; R5: %v", g.nome, e.stato, e.esito(), x.err)
	if e.stallo() {
		s.t.Errorf("%s: %d %v %.400s", g.nome, e.stato, e.err, e.corpo)
	}
	if stalloR5(x.err) {
		s.t.Errorf("R5 dopo %s: 40P01/55P03 (%v)", g.nome, x.err)
	}
	return e, x.out, x.err
}

// controllaR5NonHaScritto: R5 rifiutata non ha lasciato niente (la working senza STEP promosso, nessuna conferma, la
// bozza aperta alla versione v).
func (s *scenaR5C6b) controllaR5NonHaScritto(v int) {
	s.t.Helper()
	if n := s.conteggio(`SELECT count(*) FROM conferma_strutturale`); n != 0 {
		s.t.Errorf("R5 rifiutata: %d conferme", n)
	}
	if n := s.conteggio(`SELECT count(*) FROM componente WHERE step_strutturale_id IS NOT NULL`); n != 0 {
		s.t.Errorf("R5 rifiutata: STEP promosso")
	}
	if got := valoreDi(s.t, s.bancoWeb, `SELECT stato::text || ':' || versione FROM distinta_bozza WHERE bozza_id = $1`, s.bozza); got != fmt.Sprintf("aperta:%d", v) {
		s.t.Errorf("R5 rifiutata: bozza %s (attesa aperta:%d)", got, v)
	}
}

// Il gesto prima, R5 dopo: R5 aspetta la RFQ, poi rilegge lo stato lasciato dal gesto e risponde senza effetti parziali.
func TestC6bConcorrenzaGestoPrimaR5Aspetta(t *testing.T) {
	// «Scollega e scarta» di D2 prima: la AR chiusa dal gesto composto (scollega_file), il file scartato; R5 con la firma
	// di prima non conferma e non scrive; R4 di nuovo e R5: lo svincolo e' «gia' chiuso» (svincolo_gia_chiuso)
	t.Run("Scollega e scarta", func(t *testing.T) {
		s := nuovaScenaR5C6b(t)
		rp, v := s.r4()
		if len(rp.Blocchi) != 0 {
			t.Fatalf("R4: %+v", rp.Blocchi)
		}
		e, primo, err := s.gestoPrimaDiR5(s.dallaSorgente("«Scollega e scarta» di D2", s.pdf2, "scarta", "doppione", map[string]any{"scollega": true}),
			s.richiestaR5(rp.FirmaAnteprima, v))
		if e.stato != http.StatusOK {
			t.Fatalf("«Scollega e scarta»: %d %s", e.stato, e.corpo)
		}
		if got := s.ar(s.pdf2); got != "ACME-C1:gesto_legacy:chiusa:scollega_file" {
			t.Errorf("AR di D2: %q", got)
		}
		if err != nil {
			s.controllaR5NonHaScritto(v)
			rp2, v2 := s.r4()
			t.Logf("R4 dopo il gesto: blocchi %+v, confermabile %v", rp2.Blocchi, rp2.Confermabile)
			if len(rp2.Blocchi) != 0 {
				t.Fatalf("R4 dopo «Scollega e scarta»: %+v", rp2.Blocchi)
			}
			out, err := s.r5(s.richiestaR5(rp2.FirmaAnteprima, v2), nil)
			if err != nil {
				t.Fatalf("R5 dopo il nuovo R4: %v", err)
			}
			if out.SvincoliGiaChiusi != 1 || out.SvincoliApplicati != 0 {
				t.Errorf("R5: svincoli applicati %d, gia' chiusi %d", out.SvincoliApplicati, out.SvincoliGiaChiusi)
			}
		} else {
			t.Logf("R5 dopo il gesto: %s (svincoli applicati %d, gia' chiusi %d)", primo.Esito, primo.SvincoliApplicati, primo.SvincoliGiaChiusi)
			if primo.SvincoliApplicati != 0 {
				t.Errorf("R5 dopo il gesto ha applicato di nuovo lo svincolo: %d", primo.SvincoliApplicati)
			}
		}
		if got := s.ar(s.pdf2); got != "ACME-C1:gesto_legacy:chiusa:scollega_file" {
			t.Errorf("AR di D2 alla fine: %q", got)
		}
		if n := s.divergenze(); n != 0 {
			t.Errorf("copie divergenti: %d", n)
		}
		s.controllaNessunoStallo()
	})
	// lo scarto dello STEP sorgente prima: D-1 (documento confermato da S8) lo rifiuta senza scrivere; R5 conferma
	t.Run("Scarta lo STEP", func(t *testing.T) {
		s := nuovaScenaR5C6b(t)
		rp, v := s.r4()
		e, out, err := s.gestoPrimaDiR5(s.dallaSorgente("«Scarta» dello STEP sorgente", s.step, "scarta", "doppione", map[string]any{"scollega": true}),
			s.richiestaR5(rp.FirmaAnteprima, v))
		if e.stato != http.StatusUnprocessableEntity || !strings.Contains(e.corpo, "DIPENDENZE_ATTIVE") {
			t.Errorf("scarto dello STEP: %d %.300s", e.stato, e.corpo)
		}
		if err != nil || out.Esito != "confermata" || !out.Fonte.Promossa {
			t.Fatalf("R5 dopo lo scarto rifiutato: %+v %v", out.Fonte, err)
		}
		if st := s.statoDB(s.thread, s.step); st == "scartato" {
			t.Errorf("STEP scartato: %q", st)
		}
		s.controllaNessunoStallo()
	})
	// la revoca della sorgente prima: G39-03 [U] (20/10 sera; core/rfq/sorgente/revoca.go di v1-pers-sorgente) la bozza
	// nata da quella sorgente, con la BOM mai confermata con lei, si annulla con la revoca; R5 trova la bozza chiusa (409
	// BOZZA_VERSIONE bozza_chiusa) e non scrive. Prima di G39-03 la bozza restava aperta e R5 rifiutava con il gate (422
	// BLOCCHI, revocata): l'atteso cambia per la decisione dell'utente, non per il codice (unione G39, corsia interfaccia)
	t.Run("Revoca della sorgente", func(t *testing.T) {
		s := nuovaScenaR5C6b(t)
		rp, v := s.r4()
		e, _, err := s.gestoPrimaDiR5(s.revocaS8(), s.richiestaR5(rp.FirmaAnteprima, v))
		if e.stato != http.StatusOK || !strings.Contains(e.corpo, `"esito":"revocata"`) || !strings.Contains(e.corpo, `"bozza_abbandonata"`) {
			t.Fatalf("revoca: %d %s", e.stato, e.corpo)
		}
		var ee *distintabozza.Errore
		if !errors.As(err, &ee) || ee.Codice != distintabozza.CodiceBozzaVersione || ee.Motivo != distintabozza.MotivoBozzaChiusa {
			t.Fatalf("R5 dopo la revoca: %v (atteso BOZZA_VERSIONE bozza_chiusa: la revoca ha annullato la bozza)", err)
		}
		if n := s.conteggio(`SELECT count(*) FROM conferma_strutturale`); n != 0 {
			t.Errorf("R5 rifiutata: %d conferme", n)
		}
		if n := s.conteggio(`SELECT count(*) FROM componente WHERE step_strutturale_id IS NOT NULL`); n != 0 {
			t.Errorf("R5 rifiutata: STEP promosso")
		}
		if got := valoreDi(t, s.bancoWeb, `SELECT stato::text || ':' || versione FROM distinta_bozza WHERE bozza_id = $1`, s.bozza); got != fmt.Sprintf("abbandonata:%d", v) {
			t.Errorf("la bozza nata dalla sorgente revocata: %s (attesa abbandonata:%d)", got, v)
		}
		s.controllaNessunoStallo()
	})
	// «Assegna» legacy prima (RFQ formato 3 con la bozza aperta): la transazione legacy si annulla e il gesto diventa un
	// salvataggio R2; R5 era in fila sulla RFQ. Un solo vincitore: R5 conferma la versione di prima e R2 trova la bozza
	// chiusa, oppure R2 salva la versione nuova e R5 risponde BOZZA_VERSIONE; mai tutti e due, mai una AR atomica.
	t.Run("Assegna legacy che diventa R2", func(t *testing.T) {
		s := nuovaScenaR5C6b(t)
		rp, v := s.r4()
		e, out, err := s.gestoPrimaDiR5(s.dalPonte("«Assegna» legacy di D1 su ACME-C2", s.base()+"/assegna",
			url.Values{"proposta": {s.prop1.String()}, "componente": {s.c2.String()}}), s.richiestaR5(rp.FirmaAnteprima, v))
		var ev *distintabozza.Errore
		switch {
		case err == nil && out.Esito == "confermata":
			if !strings.HasPrefix(e.esito(), "no") {
				t.Errorf("R5 ha confermato ma «Assegna» dice: %s", e.esito())
			}
		case errors.As(err, &ev) && ev.Codice == distintabozza.CodiceBozzaVersione:
			if !strings.HasPrefix(e.esito(), "ok") {
				t.Errorf("R5 BOZZA_VERSIONE ma «Assegna» dice: %s", e.esito())
			}
			s.controllaR5NonHaScritto(v + 1)
		default:
			t.Errorf("esiti non coerenti: «Assegna» %s, R5 %v", e.esito(), err)
		}
		if got := s.ar(s.pdf1); got != "" {
			t.Errorf("AR atomica di D1 nella RFQ formato 3: %q", got)
		}
		s.controllaNessunoStallo()
	})
	// il vecchio «Scarta il file» (la RFQ FOR KEY SHARE, poi la proposta) prima: R5 (FOR UPDATE) aspetta; poi conferma o
	// risponde senza scrivere
	t.Run("vecchio Scarta il file", func(t *testing.T) {
		s := nuovaScenaR5C6b(t)
		rp, v := s.r4()
		e, out, err := s.gestoPrimaDiR5(s.dalPonte("vecchio «Scarta il file» di D3", "/proposta/"+s.prop3.String()+"/scarta", url.Values{}),
			s.richiestaR5(rp.FirmaAnteprima, v))
		if !strings.Contains(e.esito(), "Proposta scartata") {
			t.Errorf("vecchio «Scarta il file»: %s", e.esito())
		}
		if err != nil {
			s.controllaR5NonHaScritto(v)
		} else if out.Esito != "confermata" {
			t.Errorf("R5: %+v", out)
		}
		s.controllaNessunoStallo()
	})
}

// Lo strumento: due transazioni che si aspettano a vicenda su due righe fanno crescere di uno il contatore deadlocks
// che le prove sopra leggono (stalli), e una delle due riceve 40P01. Senza questa controprova un contatore fermo non
// direbbe niente.
func TestC6bConcorrenzaIlContatoreDegliStalliFunziona(t *testing.T) {
	s := nuovaScenaR5C6b(t)
	altra, err := pgx.Connect(s.ctx, testutil.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer altra.Close(context.Background()) //nolint:errcheck
	a, err := altra.Begin(s.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Rollback(context.Background()) //nolint:errcheck
	b, err := s.custode.Begin(s.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Rollback(context.Background()) //nolint:errcheck
	if _, err := a.Exec(s.ctx, `SELECT 1 FROM componente WHERE componente_id = $1 FOR UPDATE`, s.c); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Exec(s.ctx, `SELECT 1 FROM componente WHERE componente_id = $1 FOR UPDATE`, s.c2); err != nil {
		t.Fatal(err)
	}
	fatto := make(chan error, 1)
	go func() {
		_, err := a.Exec(s.ctx, `SELECT 1 FROM componente WHERE componente_id = $1 FOR UPDATE`, s.c2)
		fatto <- err
	}()
	if !s.aspettaAttese(1, false, 5*time.Second) {
		t.Fatalf("la prima transazione non aspetta:\n%s", s.attese())
	}
	_, errB := b.Exec(s.ctx, `SELECT 1 FROM componente WHERE componente_id = $1 FOR UPDATE`, s.c)
	errA := <-fatto
	if !strings.Contains(fmt.Sprint(errA, errB), "40P01") {
		t.Fatalf("nessun 40P01: %v / %v", errA, errB)
	}
	_ = a.Rollback(s.ctx)
	_ = b.Rollback(s.ctx)
	_ = altra.Close(s.ctx)
	if d := s.stalli(); d != s.stalliIn+1 {
		t.Fatalf("il contatore deadlocks: da %d a %d (atteso +1)", s.stalliIn, d)
	}
}
