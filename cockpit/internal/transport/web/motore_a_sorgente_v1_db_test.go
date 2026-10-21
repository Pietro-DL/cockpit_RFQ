//go:build integrazione

// L4 — direttiva 39 §2, §3: S8 e la revoca attraverso le rotte (motore_a_sorgente_v1.go), con il banco web vero (sessione,
// CSRF, flag del motore A, database di prova). Il banco non carica le regole del motore: S8 con un corpo valido risponde
// 503 NON_CONFIGURATO senza scrivere; il percorso completo passa dal calcolo sostituito (preparaS8Prova) con la partenza
// solo_prodotto, che non ha registro. Dati inventati: ACME, codici 712xxxx.

package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	sorgentev1 "promatec/cockpit/internal/core/rfq/sorgente"
)

func TestV1S8ConfermaERevocaViaHTTP(t *testing.T) {
	b, s := bancoBozzaV1(t)
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	op := b.browser("10.0.0.9")
	op.login("FP", "prova-fp")
	esegui := func(sql string, args ...any) {
		t.Helper()
		if _, err := b.pool.Exec(b.ctx, sql, args...); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	conta := func(sql string, args ...any) int {
		t.Helper()
		var n int
		if err := b.pool.QueryRow(b.ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
		return n
	}
	// lo STEP della scena scaricato, con la sua proposta di 3D; la cartella della RFQ; un finito nuovo senza bozza
	step := uuidSQL(t, b, `SELECT a.allegato_id FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE m.thread_id = $1 AND a.estensione = 'stp'`, s.thread)
	percorso := filepath.Join(t.TempDir(), "7120001.stp")
	if err := os.WriteFile(percorso, []byte("STEP sintetico"), 0o644); err != nil {
		t.Fatal(err)
	}
	esegui(`UPDATE allegato SET path_staging = $2, natura = 'file', origine = 'outlook', stato = 'analizzato', bytes = 14 WHERE allegato_id = $1`, step, percorso)
	esegui(`UPDATE thread_offerta SET cartella_relativa = 'ACMEV1\WIP\RFQ 7120001' WHERE thread_id = $1`, s.thread)
	esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1, $2, 'cad_3d', '7120004', 80, 'nome_file')`,
		step, s.thread)
	nuovo := uuid.New()
	esegui(`INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES ($1, $2, '7120004', 'finito', 'manuale', $3)`,
		nuovo, s.thread, s.admin)
	url := "/thread/" + s.thread.String() + "/sorgente/step/" + step.String() + "/conferma-sorgente"
	corpo := func(chiave, prodotto uuid.UUID, extra map[string]any) string {
		c := map[string]any{"chiave_idempotenza": chiave, "prodotto_id": prodotto, "partenza": "solo_prodotto", "stato_analisi": "in_analisi",
			"presa_d_atto": true, "motivo": "lo STEP e' ancora in analisi"}
		for k, v := range extra {
			c[k] = v
		}
		raw, _ := json.Marshal(c)
		return string(raw)
	}
	vietate := func() string {
		return strings.Join([]string{
			itoa(conta(`SELECT count(*) FROM componente WHERE step_strutturale_id IS NOT NULL`)),
			itoa(conta(`SELECT count(*) FROM documento WHERE sostituito_da IS NOT NULL`)),
			itoa(conta(`SELECT count(*) FROM componente_relazione`)), itoa(conta(`SELECT count(*) FROM conferma_strutturale`)),
			itoa(conta(`SELECT count(*) FROM file_rfq_associazione`)), itoa(conta(`SELECT count(*) FROM associazione_evento`))}, "|")
	}

	prima := fotoDelDatabase(t, b)
	// l'operatore non e' admin; un altro metodo e' 405; il prodotto con la bozza manuale senza la bozza nel corpo e' 409
	// BOZZA_VERSIONE (la riconciliazione vuole la bozza dichiarata, direttiva 39 §3); un campo fuori forma e' 422
	if resp, _ := op.chiediJSON(http.MethodPost, url, corpo(uuid.New(), nuovo, nil)); resp.StatusCode == http.StatusOK {
		t.Fatal("S8 dall'operatore: 200")
	}
	if resp, _ := ad.chiediJSON(http.MethodGet, url, ""); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", resp.StatusCode)
	}
	resp, testo := ad.chiediJSON(http.MethodPost, url, corpo(uuid.New(), s.prodotto, nil))
	if resp.StatusCode != http.StatusConflict || !strings.Contains(testo, `"codice":"BOZZA_VERSIONE"`) || !strings.Contains(testo, `"motivo":"altra_bozza_aperta"`) {
		t.Fatalf("bozza manuale: %d %s", resp.StatusCode, testo)
	}
	resp, testo = ad.chiediJSON(http.MethodPost, url, corpo(uuid.New(), nuovo, map[string]any{"autorizza": true}))
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(testo, `"codice":"CORPO_NON_VALIDO"`) {
		t.Fatalf("campo autorizza: %d %s", resp.StatusCode, testo)
	}
	// il calcolo del motore senza le regole: 503, niente scritto
	resp, testo = ad.chiediJSON(http.MethodPost, url, corpo(uuid.New(), nuovo, nil))
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(testo, `"codice":"NON_CONFIGURATO"`) {
		t.Fatalf("senza le regole del motore: %d %s", resp.StatusCode, testo)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Fatalf("i rifiuti hanno scritto in: %s", d)
	}

	// il calcolo sostituito: lo STEP in analisi, nessuna struttura importabile
	preparaS8Prova = func(context.Context, uuid.UUID, uuid.UUID, sorgentev1.RichiestaS8) (sorgentev1.Preparazione, error) {
		return sorgentev1.Preparazione{StatoAnalisi: sorgentev1.AnalisiInCorso}, nil
	}
	t.Cleanup(func() { preparaS8Prova = nil })
	v0 := vietate()
	chiave := uuid.New()
	resp, testo = ad.chiediJSON(http.MethodPost, url, corpo(chiave, nuovo, nil))
	if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"esito":"completata"`) || !strings.Contains(testo, `"riproposto":false`) {
		t.Fatalf("S8: %d %s", resp.StatusCode, testo)
	}
	controllaSenzaPercorsi(t, "S8", testo)
	var es sorgentev1.EsitoS8
	if err := json.Unmarshal([]byte(testo), &es); err != nil || es.Bozza == nil || es.Bozza.Importazione != nil || len(es.Passi) != 3 {
		t.Fatalf("risposta di S8: %+v %v", es, err)
	}
	if v := vietate(); v != v0 {
		t.Fatalf("S8 ha toccato STEP strutturale, sostituzioni, working o AR: %s -> %s", v0, v)
	}
	if n := conta(`SELECT count(*) FROM documento WHERE componente_id = $1 AND tipo = 'cad_3d'`, nuovo); n != 1 {
		t.Fatalf("documento dello STEP: %d", n)
	}
	if n := conta(`SELECT count(*) FROM distinta_importazione`); n != 0 {
		t.Fatalf("registro d'importazione con solo_prodotto: %d", n)
	}
	// il reinvio: 200 riproposto, niente scritto
	dopo := fotoDelDatabase(t, b)
	resp, testo = ad.chiediJSON(http.MethodPost, url, corpo(chiave, nuovo, nil))
	if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"riproposto":true`) {
		t.Fatalf("reinvio: %d %s", resp.StatusCode, testo)
	}
	resp, testo = ad.chiediJSON(http.MethodPost, url, corpo(chiave, nuovo, map[string]any{"motivo": "un altro corpo"}))
	if resp.StatusCode != http.StatusConflict || !strings.Contains(testo, `"codice":"CHIAVE_RIUSATA"`) {
		t.Fatalf("stessa chiave, altro corpo: %d %s", resp.StatusCode, testo)
	}
	if d := differenze(dopo, fotoDelDatabase(t, b)); d != "" {
		t.Fatalf("il reinvio ha scritto in: %s", d)
	}
	// R6 ritrova l'intento
	resp, testo = ad.chiediJSON(http.MethodGet, "/thread/"+s.thread.String()+"/motore-a/intento/"+chiave.String(), "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"conferma_sorgente"`) {
		t.Fatalf("R6: %d %s", resp.StatusCode, testo)
	}

	// la revoca: 200, il reinvio riproposto, una seconda revoca senza niente da revocare 422
	urlRevoca := "/thread/" + s.thread.String() + "/sorgente/step/" + step.String() + "/revoca-sorgente"
	k := uuid.New()
	rv := `{"chiave_idempotenza":"` + k.String() + `","prodotto_id":"` + nuovo.String() + `","motivo":"STEP scelto per errore"}`
	v1 := vietate()
	resp, testo = ad.chiediJSON(http.MethodPost, urlRevoca, rv)
	// G39-03 [U]: prima di R5 la bozza nata dalla sorgente revocata si annulla
	if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"esito":"revocata"`) ||
		!strings.Contains(testo, `"bozza_abbandonata":"`+es.Bozza.BozzaID.String()+`"`) {
		t.Fatalf("revoca: %d %s", resp.StatusCode, testo)
	}
	if v := vietate(); v != v1 {
		t.Fatalf("la revoca ha toccato la working: %s -> %s", v1, v)
	}
	if resp, testo = ad.chiediJSON(http.MethodPost, urlRevoca, rv); resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"riproposto":true`) {
		t.Fatalf("reinvio della revoca: %d %s", resp.StatusCode, testo)
	}
	rv2 := `{"chiave_idempotenza":"` + uuid.New().String() + `","prodotto_id":"` + nuovo.String() + `","motivo":"di nuovo"}`
	if resp, testo = ad.chiediJSON(http.MethodPost, urlRevoca, rv2); resp.StatusCode != http.StatusUnprocessableEntity ||
		!strings.Contains(testo, `"motivo":"nessuna_sorgente_da_revocare"`) {
		t.Fatalf("seconda revoca: %d %s", resp.StatusCode, testo)
	}

	// G39-03 [U]: «Sposta la sorgente su questo prodotto» (sposta_da), attraverso la rotta. Lo STEP torna sorgente del finito
	// nuovo; un terzo finito lo chiede: senza il gesto 422 con altro_prodotto, niente scritto; con il gesto 200 con spostamento
	if resp, testo = ad.chiediJSON(http.MethodPost, url, corpo(uuid.New(), nuovo, nil)); resp.StatusCode != http.StatusOK {
		t.Fatalf("S8 di nuovo: %d %s", resp.StatusCode, testo)
	}
	terzo := uuid.New()
	esegui(`INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES ($1, $2, '7120005', 'finito', 'manuale', $3)`,
		terzo, s.thread, s.admin)
	f0 := fotoDelDatabase(t, b)
	resp, testo = ad.chiediJSON(http.MethodPost, url, corpo(uuid.New(), terzo, nil))
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(testo, `"codice":"STEP_SORGENTE_DI_ALTRO_PRODOTTO"`) ||
		!strings.Contains(testo, `"motivo":"sorgente_spostabile"`) || !strings.Contains(testo, `"altro_prodotto":"`+nuovo.String()+`"`) {
		t.Fatalf("senza il gesto: %d %s", resp.StatusCode, testo)
	}
	if d := differenze(f0, fotoDelDatabase(t, b)); d != "" {
		t.Fatalf("il rifiuto ha scritto in: %s", d)
	}
	v2 := vietate()
	resp, testo = ad.chiediJSON(http.MethodPost, url, corpo(uuid.New(), terzo, map[string]any{"sposta_da": nuovo}))
	if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"esito":"completata"`) || !strings.Contains(testo, `"da_prodotto_id":"`+nuovo.String()+`"`) ||
		!strings.Contains(testo, `"caso":"sorgente_s8"`) {
		t.Fatalf("lo spostamento: %d %s", resp.StatusCode, testo)
	}
	controllaSenzaPercorsi(t, "spostamento", testo)
	if v := vietate(); v != v2 {
		t.Fatalf("lo spostamento ha toccato STEP strutturale, sostituzioni, working o AR: %s -> %s", v2, v)
	}
	if n := conta(`SELECT count(*) FROM documento d JOIN documento_provenienza p ON p.documento_id = d.documento_id WHERE p.allegato_id = $1 AND d.componente_id = $2
		AND d.codice = '7120005'`, step, terzo); n != 1 {
		t.Fatal("il documento dello STEP non e' passato al terzo finito")
	}
}
