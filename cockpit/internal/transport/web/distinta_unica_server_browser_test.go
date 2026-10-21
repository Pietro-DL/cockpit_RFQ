//go:build integrazione && browser

// Browser contro il server vero — direttiva 39 §8 («e2e browser sui percorsi Sorgente → bozza manuale/STEP → R4/R5 →
// revisione → AR»), corsia interfaccia: la pagina della Distinta unica (distinta_unica.mjs e il ponte) servita dal banco web
// VERO (preparaBancoWeb: sessioni, CSRF, database di prova cockpit_test*, rotte vere di R1-R7, S1, S6, S8, S9) con il calcolo
// vero del Motore A (le regole ACME della scena B87 di motore_a_db_test.go), in un browser vero (Playwright su Edge):
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test COCKPIT_BROWSER_CANALE=msedge go test -tags "integrazione browser" -count=1 -run TestDistintaUnicaServerVeroNelBrowser ./internal/transport/web/
//
// La scena (duServerVero) è una RFQ ACME con i codici della grammatica ACME (motore_a_db_test.go, mwRegoleACME), perché il
// calcolo vero legga le identità dei nodi dello STEP: il prodotto finito 9120001 senza BOM registrata; lo STEP 9120001.stp
// confermato dalle pagine di prima come 3D del prodotto ma mai legato con S8 (un legame storico, D-AR04 (iii)); un secondo
// STEP 9120001_r2.stp per la revisione; il PDF 9120020A_1.pdf del particolare, in proposta aperta. I file sono veri nello
// staging (il PDF è una tavola vera); i fatti degli STEP sono quelli dell'analizzatore corrente, scritti dalla scena.
//
// Lo script e2e/distinta_unica_server.py fa, come l'ingegnere e solo con i gesti della pagina:
//   - fase 1: «Parti dal solo prodotto…» (R2 crea vuota), il PDF associato a mano al prodotto (una PB manuale), R4 completo
//     con il blocco STEP_SORGENTE_NON_CONFERMATA e la conferma spenta con il rimedio della Sorgente;
//   - fra le fasi, qui: R5 chiamata direttamente dal test con la bozza salvata e la firma di R4: 422 BLOCCHI e nessuna riga
//     cambiata (il gate non dipende dalla pagina, direttiva 39 §1);
//   - fase 2: «Conferma questo STEP come sorgente di 9120001…» (S8) sulla bozza senza STEP, che il server riconcilia nella
//     stessa bozza; la PB spostata sul particolare; R4 confermabile; «Conferma BOM» (R5); la AR scritta da R5 letta dalle
//     righe di S9 («già registrato»);
//   - fase 3: la revisione, «Usa nuovo STEP…» (S8) con lo STEP rev 2: qui il test vede la working ancora sul primo STEP, che
//     non è sostituito prima di R5 (direttiva 39 §2);
//   - fase 4: R4 con la promozione e la AR che resta; nel sottotest «R5 della revisione», R5: lo STEP nuovo nel componente,
//     il primo sostituito, la storia intatta (SKIP con il motivo, mai PASS, se il riepilogo del server blocca la conferma).
// Dopo ogni fase il database dice che cosa è successo davvero.
//
// Clienti, codici e nomi dei file sono inventati (ACME, 912xxxx, acme.example): il repository è pubblico.

package web

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// duServerVero: la scena del commento di testa.
type duServerVero struct {
	*rfqFascicolo
	b                               *bancoWeb
	prodotto                        uuid.UUID
	step, allStep, allStep2, allPDF uuid.UUID
	shaStep2                        string
	ad                              *browser
	// prodotto2: il secondo prodotto finito della scena G39 (TestDistintaUnicaG39ServerVeroNelBrowser); zero nelle altre
	prodotto2 uuid.UUID
}

func (b *bancoWeb) duServerVero(t *testing.T, chiave string) *duServerVero {
	t.Helper()
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	r := b.rfqFascicolo(cliente, chiave)
	s := &duServerVero{rfqFascicolo: r, b: b}
	r.fase("FATTIBILITA")
	s.prodotto = r.componenteTipo("9120001", "finito")
	s.step, s.allStep = r.documentoDa("9120001.stp", "stp", db.TipoDocumentoCad3d, "9120001", s.prodotto, db.StatoNasScritto)
	r.analisiCorrente(r.shaDoc(s.step), fattiDi("#1", []string{"#1=9120001A", "#2=9120010A", "#3=9120020A"}, []string{"#1>#2*2", "#2>#3*4"}))
	// il PDF del particolare: una tavola vera nello staging (le miniature della pagina la leggono), con la sua proposta
	s.allPDF, _ = r.allegatoExt("9120020A_1.pdf", "pdf")
	contenuto := pdfTavola("9120020")
	var percorso string
	if err := b.pool.QueryRow(b.ctx, `SELECT path_staging FROM allegato WHERE allegato_id = $1`, s.allPDF).Scan(&percorso); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(percorso, contenuto, 0o644); err != nil {
		t.Fatal(err)
	}
	r.esegui(`UPDATE allegato SET sha256 = $2, bytes = $3 WHERE allegato_id = $1`, s.allPDF, shaDi(contenuto), len(contenuto))
	r.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1, $2, 'disegno_2d', '9120020A', 85, 'nome_file')`,
		s.allPDF, r.thread)
	b.ws.Staging = filepath.Dir(percorso)
	mwAccendi(t, b, mwRegoleACME(t, cliente, "Acme S.p.A.", 20*time.Second))
	s.ad = b.browser("10.0.0.8")
	s.ad.login("AD", "prova-ad")
	return s
}

// arrivaStepRevisione: lo STEP rev 2 del prodotto arriva dopo la prima conferma (la stessa struttura, il particolare ×3
// sotto l'assieme), letto dall'analizzatore corrente, con la sua proposta di 3D. Arriva dopo, non con gli altri file: uno
// STEP del finito presente già alla nascita della bozza diventa una PB del motore sulla radice e, con R5, una AR, che poi
// impedisce di confermarlo come sorgente (resoconto della corsia interfaccia, rischi).
func (s *duServerVero) arrivaStepRevisione() {
	s.allStep2, s.shaStep2 = s.allegatoExt("9120001_r2.stp", "stp")
	s.analisiCorrente(s.shaStep2, fattiDi("#1", []string{"#1=9120001A", "#2=9120010A", "#3=9120020A"}, []string{"#1>#2*2", "#2>#3*3"}))
	s.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1, $2, 'cad_3d', '9120001', 80, 'nome_file')`,
		s.allStep2, s.thread)
}

// lancia: lo script del browser per una fase (1-4).
func (s *duServerVero) lancia(t *testing.T, fase string) string {
	t.Helper()
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
		return ""
	}
	script := filepath.Join("e2e", "distinta_unica_server.py")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("lo script del browser non c'è: %v", err)
	}
	nomi := map[string]string{}
	righe, err := s.b.pool.Query(s.b.ctx, `SELECT a.nome_file, a.allegato_id::text FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id
		WHERE m.thread_id = $1`, s.thread)
	if err != nil {
		t.Fatal(err)
	}
	for righe.Next() {
		var k, v string
		if err := righe.Scan(&k, &v); err != nil {
			righe.Close()
			t.Fatal(err)
		}
		nomi[k] = v
	}
	righe.Close()
	d := map[string]any{"thread": s.thread, "prodotto": s.prodotto, "allegati": nomi}
	if s.prodotto2 != uuid.Nil {
		d["prodotto2"] = s.prodotto2
	}
	dati, _ := json.Marshal(d)
	arg := []string{"-I", script, "--url", s.b.srv.URL, "--ip", "10.0.0.5:51000", "--sigla", "AD", "--password", "prova-ad", "--dati", string(dati), "--fase", fase}
	if foto := os.Getenv("COCKPIT_FOTO_DIR"); foto != "" {
		arg = append(arg, "--foto", foto)
	}
	if canale := os.Getenv("COCKPIT_BROWSER_CANALE"); canale != "" {
		arg = append(arg, "--canale", canale)
	}
	cmd := exec.Command(python, arg...)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8")
	uscita, err := cmd.CombinedOutput()
	testo := strings.TrimRight(string(uscita), "\r\n")
	if err != nil {
		if strings.Contains(testo, "ModuleNotFoundError") || strings.Contains(testo, "Executable doesn't exist") {
			t.Skipf("Playwright o il browser non sono installati su questa macchina:\n%s", testo)
		}
		t.Fatalf("la fase %s nel browser è fallita (%v):\n%s", fase, err, testo)
	}
	t.Logf("fase %s nel browser:\n%s", fase, testo)
	return testo
}

func TestDistintaUnicaServerVeroNelBrowser(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.duServerVero(t, "DUSV1")
	conta := func(sql string, arg ...any) int { t.Helper(); return contaSQL(t, b, sql, arg...) }
	vietate := func() string {
		return strings.Join([]string{
			itoa(conta(`SELECT count(*) FROM componente WHERE step_strutturale_id IS NOT NULL AND thread_id = $1`, s.thread)),
			itoa(conta(`SELECT count(*) FROM documento WHERE sostituito_da IS NOT NULL AND thread_id = $1`, s.thread)),
			itoa(conta(`SELECT count(*) FROM componente_relazione WHERE thread_id = $1`, s.thread)),
			itoa(conta(`SELECT count(*) FROM conferma_strutturale WHERE thread_id = $1`, s.thread)),
			itoa(conta(`SELECT count(*) FROM file_rfq_associazione`)), itoa(conta(`SELECT count(*) FROM associazione_evento`)),
			itoa(conta(`SELECT count(*) FROM fonte_step_evento`))}, "|")
	}
	v0 := vietate()

	// fase 1: bozza senza STEP, PB manuale, R4 con il gate
	s.lancia(t, "1")
	if v := vietate(); v != v0 {
		t.Fatalf("la fase 1 ha scritto working, conferme, AR o eventi della sorgente: %s -> %s", v0, v)
	}
	var bozza uuid.UUID
	var versione int
	if err := b.pool.QueryRow(b.ctx, `SELECT b.bozza_id, max(v.versione) FROM distinta_bozza b JOIN distinta_bozza_versione v ON v.bozza_id = b.bozza_id
		WHERE b.thread_id = $1 AND b.prodotto_id = $2 AND b.stato = 'aperta' GROUP BY b.bozza_id`, s.thread, s.prodotto).Scan(&bozza, &versione); err != nil {
		t.Fatalf("la bozza aperta dopo la fase 1: %v", err)
	}
	if n := conta(`SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1 AND fonte_allegato_id IS NOT NULL`, bozza); n != 0 {
		t.Errorf("la bozza vuota ha una sorgente: %d versioni", n)
	}
	// R5 chiamata direttamente (senza la pagina) con la firma di R4: 422 BLOCCHI con il gate, niente scritto
	var raw json.RawMessage
	if err := b.pool.QueryRow(b.ctx, `SELECT snapshot FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = $2`, bozza, versione).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	corpo := map[string]any{}
	if err := json.Unmarshal(raw, &corpo); err != nil {
		t.Fatal(err)
	}
	corpo["bozza_id"], corpo["bozza_versione"] = bozza, versione
	c4, _ := json.Marshal(corpo)
	base := "/thread/" + s.thread.String() + "/motore-a/bom/"
	resp, testo := s.ad.chiediJSON(http.MethodPost, base+"riepilogo", string(c4))
	var r4 struct {
		Firma   string `json:"firma_anteprima"`
		Blocchi []struct {
			Codice string `json:"codice"`
		} `json:"blocchi"`
	}
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &r4) != nil || r4.Firma == "" {
		t.Fatalf("R4 diretta: %d %.400s", resp.StatusCode, testo)
	}
	corpo["chiave_idempotenza"] = uuid.New()
	c5, _ := json.Marshal(map[string]any{"firma_anteprima": r4.Firma, "bozza": corpo})
	prima := fotoDelDatabase(t, b)
	resp, testo = s.ad.chiediJSON(http.MethodPost, base+"conferma", string(c5))
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(testo, `"codice":"BLOCCHI"`) || !strings.Contains(testo, `"STEP_SORGENTE_NON_CONFERMATA"`) {
		t.Fatalf("R5 diretta senza sorgente: %d %.400s", resp.StatusCode, testo)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Fatalf("R5 rifiutata ha scritto in: %s", d)
	}

	// fase 2: S8, riconciliazione, revisione, R4/R5, AR
	s.lancia(t, "2")
	if n := conta(`SELECT count(*) FROM distinta_bozza WHERE thread_id = $1 AND prodotto_id = $2`, s.thread, s.prodotto); n != 1 {
		t.Errorf("bozze del prodotto dopo S8: %d, attesa una sola (S8 riconcilia la stessa bozza)", n)
	}
	if n := conta(`SELECT count(*) FROM fonte_step_evento WHERE thread_id = $1 AND prodotto_id = $2 AND azione = 'conferma_documento'`, s.thread, s.prodotto); n != 1 {
		t.Errorf("conferme S8 della sorgente: %d", n)
	}
	if n := conta(`SELECT count(*) FROM conferma_strutturale WHERE thread_id = $1 AND prodotto_id = $2`, s.thread, s.prodotto); n != 1 {
		t.Errorf("conferme registrate: %d, attesa una", n)
	}
	if id := uuidSQL(t, b, `SELECT COALESCE(step_strutturale_id, '00000000-0000-0000-0000-000000000000') FROM componente WHERE componente_id = $1`, s.prodotto); id != s.step {
		t.Errorf("R5 non ha promosso lo STEP del prodotto: %v (atteso il documento %v)", id, s.step)
	}
	if n := conta(`SELECT count(*) FROM file_rfq_associazione fa JOIN componente c ON c.componente_id = fa.componente_id
		WHERE fa.allegato_id = $1 AND c.codice = '9120020A' AND fa.origine = 'conferma_bom' AND fa.provenienza_pb = 'motore' AND fa.stato = 'attiva'`, s.allPDF); n != 1 {
		// G39-01 [U]: la PB spostata sul nodo che il motore propone è la sua proposta accettata (la pagina dichiara il
		// registro, il server decide «motore»; prima di G39-01 era «manuale») [T-39 della corsia interfaccia, da ratificare]
		t.Errorf("la AR di 9120020A_1.pdf sul particolare 9120020, scritta da R5 con la provenienza motore: %d", n)
	}
	if n := conta(`SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1`, s.allStep); n != 0 {
		t.Errorf("lo STEP sorgente ha una AR: %d (D-AR02 A: la sorgente sta fuori dalle AR)", n)
	}
	ar1 := uuidSQL(t, b, `SELECT associazione_id FROM file_rfq_associazione WHERE allegato_id = $1 AND stato = 'attiva'`, s.allPDF)

	// fase 3: arriva lo STEP rev 2; la revisione con «Usa nuovo STEP…» (S8): la working resta sul primo STEP, che non è
	// sostituito prima di R5 (direttiva 39 §2)
	s.arrivaStepRevisione()
	s.lancia(t, "3")
	var docB uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT documento_id FROM documento WHERE thread_id = $1 AND componente_id = $2 AND sha256 = $3`, s.thread, s.prodotto, s.shaStep2).Scan(&docB); err != nil {
		t.Fatalf("il documento dello STEP nuovo dopo S8: %v", err)
	}
	if id := uuidSQL(t, b, `SELECT step_strutturale_id FROM componente WHERE componente_id = $1`, s.prodotto); id != s.step {
		t.Errorf("S8 ha cambiato lo STEP della working: %v", id)
	}
	if n := conta(`SELECT count(*) FROM documento WHERE documento_id = $1 AND sostituito_da IS NOT NULL`, s.step); n != 0 {
		t.Error("il primo STEP è sostituito prima di R5")
	}
	if n := conta(`SELECT count(*) FROM distinta_bozza WHERE thread_id = $1 AND prodotto_id = $2 AND stato = 'aperta'`, s.thread, s.prodotto); n != 1 {
		t.Errorf("bozze aperte dopo S8 della revisione: %d", n)
	}
	if n := conta(`SELECT count(*) FROM conferma_strutturale WHERE thread_id = $1 AND prodotto_id = $2`, s.thread, s.prodotto); n != 1 {
		t.Errorf("S8 ha registrato una conferma: %d", n)
	}

	// fase 4: R4 della revisione (la sorgente nuova confermata, la promozione prevista); poi, nel sottotest «R5 della
	// revisione», R5: lo STEP nuovo promosso, il primo sostituito, la AR intatta, la storia conservata. Se il riepilogo del
	// server blocca la conferma, la fase lo dice: qui si controlla che niente sia stato scritto, e il sottotest è SKIP con il
	// motivo, mai PASS (rilievo del revisore del 20/10: prima il test usciva PASS con R5 della revisione NON ESEGUITA)
	out := s.lancia(t, "4")
	eseguita := !strings.Contains(out, "R5 DELLA REVISIONE NON ESEGUITA")
	if !eseguita {
		if n := conta(`SELECT count(*) FROM conferma_strutturale WHERE thread_id = $1 AND prodotto_id = $2`, s.thread, s.prodotto); n != 1 {
			t.Errorf("conferme dopo una revisione non confermata: %d", n)
		}
		if id := uuidSQL(t, b, `SELECT step_strutturale_id FROM componente WHERE componente_id = $1`, s.prodotto); id != s.step {
			t.Errorf("lo STEP della working è cambiato senza R5: %v", id)
		}
	}
	t.Run("R5 della revisione", func(t *testing.T) {
		if !eseguita {
			t.Skip("NON ESEGUITA: il riepilogo del server blocca la conferma della revisione (i blocchi sono nell'uscita della fase 4)")
		}
		duR5RevisioneNelDatabase(t, s, docB, ar1)
	})
}

// duR5RevisioneNelDatabase: dopo R5 della revisione, che cosa dice il database (direttiva 39 §2, §8).
func duR5RevisioneNelDatabase(t *testing.T, s *duServerVero, docB, ar1 uuid.UUID) {
	t.Helper()
	b := s.b
	conta := func(sql string, arg ...any) int { t.Helper(); return contaSQL(t, b, sql, arg...) }
	if n := conta(`SELECT count(*) FROM conferma_strutturale WHERE thread_id = $1 AND prodotto_id = $2`, s.thread, s.prodotto); n != 2 {
		t.Errorf("conferme registrate dopo la revisione: %d, attese 2", n)
	}
	if id := uuidSQL(t, b, `SELECT step_strutturale_id FROM componente WHERE componente_id = $1`, s.prodotto); id != docB {
		t.Errorf("R5 della revisione non ha promosso lo STEP nuovo: %v (atteso %v)", id, docB)
	}
	if id := uuidSQL(t, b, `SELECT COALESCE(sostituito_da, '00000000-0000-0000-0000-000000000000') FROM documento WHERE documento_id = $1`, s.step); id != docB {
		t.Errorf("il primo STEP non è sostituito dal nuovo: %v", id)
	}
	if n := conta(`SELECT count(*) FROM file_rfq_associazione WHERE associazione_id = $1 AND stato = 'attiva'`, ar1); n != 1 {
		t.Error("la AR della prima conferma non è più attiva dopo la revisione")
	}
	if n := conta(`SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = ANY($1)`, []uuid.UUID{s.allStep, s.allStep2}); n != 0 {
		t.Errorf("gli STEP sorgente hanno AR: %d", n)
	}
	if n := conta(`SELECT count(*) FROM componente_relazione cr JOIN componente f ON f.componente_id = cr.figlio_id
		WHERE cr.thread_id = $1 AND f.codice = '9120020A' AND cr.qta = 3`, s.thread); n != 1 {
		t.Errorf("la quantità dello STEP nuovo (×3) nella working: %d archi", n)
	}
}
