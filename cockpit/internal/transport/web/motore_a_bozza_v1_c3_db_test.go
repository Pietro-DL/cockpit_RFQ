//go:build integrazione

// L4 — direttiva 35, blocco C-3 (R2 passo 6b), corsia persistenza: le forme di R2 costruite dal server attraverso la rotta
// (motore_a_bozza_v1.go), con il banco web vero (sessione, CSRF, database di prova): converti_da (la conversione di una
// bozza formato 2 aperta, il reinvio riproposto, il motivo obbligatorio); crea senza il motore (vuota, registrato con la
// bozza gia' aperta, conferma senza conferme). Dati inventati: ACME, codici 712xxxx.

package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// bozza2V1: un finito nuovo della RFQ della scena, con una bozza formato 2 aperta (la sola radice), come l'avrebbe
// lasciata la pagina vecchia. Restituisce il prodotto e la bozza.
func bozza2V1(t *testing.T, b *bancoWeb, s *scenaBozzaV1, codice string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	prodotto := uuid.New()
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da)
		VALUES ($1, $2, $3, 'finito', 'manuale', $4)`, prodotto, s.thread, codice, s.admin); err != nil {
		t.Fatal(err)
	}
	firma := strings.Repeat("1", 64)
	snap := distintabozza.Snapshot{Formato: distintabozza.FormatoSnapshot, ProdottoID: prodotto, VersioneBomBase: &distintabozza.VersioneBomBase{},
		FirmaBase: firma, Nodi: []distintabozza.Nodo{{ChiaveBozza: "r", ComponenteID: &prodotto, Op: distintabozza.OpRiusa, Codice: codice,
			Tipo: "finito", Origine: distintabozza.OrigineRegistrato}}}
	raw, _ := json.Marshal(snap)
	letto, err := distintabozza.LeggiSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	can, err := letto.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := b.pool.Begin(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(b.ctx) //nolint:errcheck
	q := db.New(tx)
	bz, err := q.InsertDistintaBozza(b.ctx, db.InsertDistintaBozzaParams{ThreadID: s.thread, ProdottoID: prodotto, Formato: distintabozza.FormatoSnapshot,
		FirmaBase: firma, UtenteID: s.admin})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertDistintaBozzaVersione(b.ctx, db.InsertDistintaBozzaVersioneParams{BozzaID: bz.BozzaID, Versione: 1, Snapshot: can,
		Impronta: jsoncanonico.Impronta(can), FirmaBase: firma, SalvataDa: s.admin}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertDistintaBozzaEvento(b.ctx, db.InsertDistintaBozzaEventoParams{BozzaID: bz.BozzaID, N: 1, Versione: 1,
		Vocabolario: int16(distintabozza.Vocabolario), Azione: distintabozza.AzBozzaAperta, Origine: "utente", UtenteID: s.admin}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(b.ctx); err != nil {
		t.Fatal(err)
	}
	return prodotto, bz.BozzaID
}

// converti_da via HTTP: il motivo obbligatorio (422 MOTIVO_OBBLIGATORIO, niente scritto), la conversione (200 con lo
// snapshot formato 3 e le tracce), il reinvio con la stessa chiave e lo stesso corpo (200 riproposto, niente scritto), la
// stessa chiave con un altro corpo (409 CHIAVE_RIUSATA); R1 legge la bozza formato 3 nuova.
func TestBozzaV1C3ConvertiViaHTTP(t *testing.T) {
	b, s := bancoBozzaV1(t)
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	base := "/thread/" + s.thread.String() + "/motore-a"
	prodotto, b2 := bozza2V1(t, b, s, "7120002")
	chiave := uuid.New()
	corpo := func(motivo string) string {
		raw, _ := json.Marshal(map[string]any{"bozza_id": nil, "versione_attesa": 0, "prodotto_id": prodotto, "vocabolario": 2,
			"chiave_salvataggio": chiave, "converti_da": map[string]any{"bozza_id": b2, "versione_attesa": 1, "motivo": motivo}})
		return string(raw)
	}
	prima := fotoDelDatabase(t, b)
	resp, testo := ad.chiediJSON(http.MethodPost, base+"/bozza", corpo(" "))
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(testo, `"codice":"MOTIVO_OBBLIGATORIO"`) ||
		!strings.Contains(testo, `"campo":"converti_da.motivo"`) {
		t.Fatalf("senza motivo: %d %s", resp.StatusCode, testo)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("il rifiuto ha scritto in: %s", d)
	}
	resp, testo = ad.chiediJSON(http.MethodPost, base+"/bozza", corpo("si lavora nel formato nuovo"))
	controllaSenzaPercorsi(t, "R2 converti_da", testo)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("conversione: %d %s", resp.StatusCode, testo)
	}
	var esito struct {
		BozzaID    uuid.UUID       `json:"bozza_id"`
		Formato    int             `json:"formato"`
		Versione   int             `json:"versione"`
		Riproposto bool            `json:"riproposto"`
		Snapshot   json.RawMessage `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(testo), &esito); err != nil || esito.Formato != 3 || esito.Versione != 1 || esito.Riproposto ||
		!strings.Contains(string(esito.Snapshot), `"modo":"formato_2"`) || !strings.Contains(testo, `"righe_cadute":[]`) {
		t.Fatalf("risposta della conversione: %v %s", err, testo)
	}
	dopo := fotoDelDatabase(t, b)
	resp, testo = ad.chiediJSON(http.MethodPost, base+"/bozza", corpo("si lavora nel formato nuovo"))
	if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"riproposto":true`) || !strings.Contains(testo, `"bozza_id":"`+esito.BozzaID.String()+`"`) {
		t.Fatalf("reinvio: %d %s", resp.StatusCode, testo)
	}
	if d := differenze(dopo, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("il reinvio ha scritto in: %s", d)
	}
	resp, testo = ad.chiediJSON(http.MethodPost, base+"/bozza", corpo("un altro motivo"))
	if resp.StatusCode != http.StatusConflict || !strings.Contains(testo, `"codice":"CHIAVE_RIUSATA"`) || !strings.Contains(testo, `"esito":"conflitto"`) {
		t.Fatalf("chiave riusata: %d %s", resp.StatusCode, testo)
	}
	_, testo = ad.fai(http.MethodGet, base+"/bozza?prodotto="+prodotto.String(), nil, false)
	if !strings.Contains(testo, `"formato":3`) || !strings.Contains(testo, esito.BozzaID.String()) {
		t.Errorf("R1 dopo la conversione: %s", testo)
	}
}

// crea via HTTP, le partenze senza il motore: vuota su un finito senza bozza (200 con lo snapshot della versione 1 e la
// partenza), il reinvio riproposto, registrato su un prodotto con la bozza aperta (409 BOZZA_VERSIONE altra_bozza_aperta),
// conferma senza conferme (422 partenza_non_ammessa); i rifiuti non scrivono.
func TestBozzaV1C3CreaViaHTTP(t *testing.T) {
	b, s := bancoBozzaV1(t)
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	base := "/thread/" + s.thread.String() + "/motore-a"
	nuovo := uuid.New()
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da)
		VALUES ($1, $2, '7120003', 'finito', 'manuale', $3)`, nuovo, s.thread, s.admin); err != nil {
		t.Fatal(err)
	}
	corpo := func(prodotto, chiave uuid.UUID, crea map[string]any) string {
		raw, _ := json.Marshal(map[string]any{"bozza_id": nil, "versione_attesa": 0, "prodotto_id": prodotto, "vocabolario": 2,
			"chiave_salvataggio": chiave, "crea": crea})
		return string(raw)
	}
	prima := fotoDelDatabase(t, b)
	resp, testo := ad.chiediJSON(http.MethodPost, base+"/bozza", corpo(s.prodotto, uuid.New(), map[string]any{"partenza": "registrato"}))
	if resp.StatusCode != http.StatusConflict || !strings.Contains(testo, `"motivo":"altra_bozza_aperta"`) {
		t.Fatalf("registrato con la bozza aperta: %d %s", resp.StatusCode, testo)
	}
	resp, testo = ad.chiediJSON(http.MethodPost, base+"/bozza", corpo(nuovo, uuid.New(), map[string]any{"partenza": "conferma"}))
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(testo, `"motivo":"partenza_non_ammessa"`) ||
		!strings.Contains(testo, `"campo":"crea.partenza"`) {
		t.Fatalf("conferma senza conferme: %d %s", resp.StatusCode, testo)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("i rifiuti hanno scritto in: %s", d)
	}
	chiave := uuid.New()
	resp, testo = ad.chiediJSON(http.MethodPost, base+"/bozza", corpo(nuovo, chiave, map[string]any{"partenza": "vuota"}))
	controllaSenzaPercorsi(t, "R2 crea", testo)
	if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"partenza":"vuota"`) || !strings.Contains(testo, `"versione":1`) ||
		!strings.Contains(testo, `"codice":"7120003"`) || !strings.Contains(testo, `"modo":"registrato"`) {
		t.Fatalf("crea vuota: %d %s", resp.StatusCode, testo)
	}
	dopo := fotoDelDatabase(t, b)
	resp, testo = ad.chiediJSON(http.MethodPost, base+"/bozza", corpo(nuovo, chiave, map[string]any{"partenza": "vuota"}))
	if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"riproposto":true`) || !strings.Contains(testo, `"partenza":"vuota"`) {
		t.Fatalf("reinvio: %d %s", resp.StatusCode, testo)
	}
	if d := differenze(dopo, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("il reinvio ha scritto in: %s", d)
	}
	_, testo = ad.fai(http.MethodGet, base+"/bozza?prodotto="+nuovo.String()+"&eventi=1", nil, false)
	if !strings.Contains(testo, `"azione":"bozza_aperta"`) || !strings.Contains(testo, `"origine":"server"`) {
		t.Errorf("R1 dopo crea: %s", testo)
	}
}

// crea con la partenza step via HTTP, senza le regole del motore nel banco: la chiave si cerca prima del calcolo (una chiave
// gia' usata con un altro corpo e' 409 CHIAVE_RIUSATA senza calcolare); con una chiave nuova il calcolo non parte (503
// NON_CONFIGURATO, esito non_registrato) e niente e' scritto. La partenza step con il registro e' provata nella L4 del core.
func TestBozzaV1C3CreaStepViaHTTP(t *testing.T) {
	b, s := bancoBozzaV1(t)
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	base := "/thread/" + s.thread.String() + "/motore-a"
	nuovo := uuid.New()
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da)
		VALUES ($1, $2, '7120004', 'finito', 'manuale', $3)`, nuovo, s.thread, s.admin); err != nil {
		t.Fatal(err)
	}
	step := uuidSQL(t, b, `SELECT a.allegato_id FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE m.thread_id = $1 AND a.estensione = 'stp'`, s.thread)
	corpo := func(chiave uuid.UUID, crea map[string]any) string {
		raw, _ := json.Marshal(map[string]any{"bozza_id": nil, "versione_attesa": 0, "prodotto_id": nuovo, "vocabolario": 2, "chiave_salvataggio": chiave, "crea": crea})
		return string(raw)
	}
	creaStep := map[string]any{"partenza": "step", "sorgente": map[string]any{"allegato_id": step, "gruppo": "bom"}, "impronta_contenuto": strings.Repeat("a", 64)}
	chiave := uuid.New()
	if resp, testo := ad.chiediJSON(http.MethodPost, base+"/bozza", corpo(chiave, map[string]any{"partenza": "vuota"})); resp.StatusCode != http.StatusOK {
		t.Fatalf("crea vuota: %d %s", resp.StatusCode, testo)
	}
	prima := fotoDelDatabase(t, b)
	resp, testo := ad.chiediJSON(http.MethodPost, base+"/bozza", corpo(chiave, creaStep))
	if resp.StatusCode != http.StatusConflict || !strings.Contains(testo, `"codice":"CHIAVE_RIUSATA"`) {
		t.Fatalf("chiave gia' usata, prima del calcolo: %d %s", resp.StatusCode, testo)
	}
	resp, testo = ad.chiediJSON(http.MethodPost, base+"/bozza", corpo(uuid.New(), creaStep))
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(testo, `"codice":"NON_CONFIGURATO"`) || !strings.Contains(testo, `"esito":"non_registrato"`) {
		t.Fatalf("senza le regole del motore: %d %s", resp.StatusCode, testo)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("i rifiuti hanno scritto in: %s", d)
	}
}

// riallinea_proposta via HTTP, senza le regole del motore nel banco: una bozza senza sorgente STEP non calcola il motore (422
// gruppo_non_importabile dal servizio); la versione superata e' 409 BOZZA_VERSIONE; niente scritto. Il riallineamento con il
// registro e' provato nella L4 del core.
func TestBozzaV1C3RiallineaViaHTTP(t *testing.T) {
	b, s := bancoBozzaV1(t)
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	base := "/thread/" + s.thread.String() + "/motore-a"
	corpo := func(versione int) string {
		raw, _ := json.Marshal(map[string]any{"bozza_id": s.bozza, "versione_attesa": versione, "prodotto_id": s.prodotto, "vocabolario": 2,
			"chiave_salvataggio": uuid.New(), "riallinea_proposta": map[string]any{"impronta_contenuto": strings.Repeat("a", 64), "motivo": "proposta di oggi"}})
		return string(raw)
	}
	prima := fotoDelDatabase(t, b)
	resp, testo := ad.chiediJSON(http.MethodPost, base+"/bozza", corpo(1))
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(testo, `"motivo":"gruppo_non_importabile"`) {
		t.Fatalf("bozza senza sorgente STEP: %d %s", resp.StatusCode, testo)
	}
	resp, testo = ad.chiediJSON(http.MethodPost, base+"/bozza", corpo(7))
	if resp.StatusCode != http.StatusConflict || !strings.Contains(testo, `"codice":"BOZZA_VERSIONE"`) {
		t.Fatalf("versione superata: %d %s", resp.StatusCode, testo)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("i rifiuti hanno scritto in: %s", d)
	}
}

// cambia_sorgente via HTTP, senza le regole del motore nel banco: il motivo e' obbligatorio (422 MOTIVO_OBBLIGATORIO prima di
// ogni calcolo); con il corpo giusto il calcolo non parte (503 NON_CONFIGURATO); niente scritto. Il cambio della fonte con i
// registri e' provato nella L4 del core.
func TestBozzaV1C3CambiaSorgenteViaHTTP(t *testing.T) {
	b, s := bancoBozzaV1(t)
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	base := "/thread/" + s.thread.String() + "/motore-a"
	step := uuidSQL(t, b, `SELECT a.allegato_id FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE m.thread_id = $1 AND a.estensione = 'stp'`, s.thread)
	corpo := func(motivo string) string {
		raw, _ := json.Marshal(map[string]any{"bozza_id": s.bozza, "versione_attesa": 1, "prodotto_id": s.prodotto, "vocabolario": 2,
			"chiave_salvataggio": uuid.New(), "cambia_sorgente": map[string]any{"sorgente": map[string]any{"allegato_id": step, "gruppo": "bom"},
				"impronta_contenuto": strings.Repeat("a", 64), "motivo": motivo}})
		return string(raw)
	}
	prima := fotoDelDatabase(t, b)
	resp, testo := ad.chiediJSON(http.MethodPost, base+"/bozza", corpo(""))
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(testo, `"codice":"MOTIVO_OBBLIGATORIO"`) {
		t.Fatalf("senza motivo: %d %s", resp.StatusCode, testo)
	}
	resp, testo = ad.chiediJSON(http.MethodPost, base+"/bozza", corpo("revisione CAD"))
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(testo, `"codice":"NON_CONFIGURATO"`) {
		t.Fatalf("senza le regole del motore: %d %s", resp.StatusCode, testo)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("i rifiuti hanno scritto in: %s", d)
	}
}
