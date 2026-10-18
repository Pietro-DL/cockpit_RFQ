//go:build integrazione

// L4 — V1, ondata B, corsia persistenza: le rotte della bozza della Distinta unica (motore_a_bozza_v1.go) con il banco
// web vero (sessioni, CSRF, database di prova):
//   - T-04 (TestNessunaGetScriveDistintaV1): le GET di R1 (anche ?eventi=1, ?importazione=1, ?confronto=1 ed
//     ?effetto_cambio_tipo=, PE-09), R6, R7 e del guscio della pagina (/thread/{id}/distinta-unica, R-8 dell'interfaccia)
//     non cambiano una riga e non accodano lavoro; chi non e' admin riceve 403;
//   - R2 forma salvataggio via HTTP: 200 con la versione nuova, lo snapshot di R1 senza l'escape HTML, 409 «conflitto» su
//     una versione superata; T-12 (meta' L4 di TestV1NessunaBozzaFormato2Nuova): un corpo formato 2 e' 422 e nessuna
//     bozza o versione formato 2 nasce;
//   - R3: 409 con la versione superata, 200 con quella corrente;
//   - R11: gli stati di F8 in attesa diventano eseguito, risolto_da riallinea;
//   - T-15: nessun percorso (C:\, \\) nelle risposte JSON.
// Le rotte si montano qui accanto a Registra finche' registraMotoreA non le monta (richiesta nel resoconto della corsia).
// Dati inventati: ACME, codici 712xxxx.

package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// scenaBozzaV1: una RFQ ACME con il finito 7120001 (e un sottoassieme 7120010) e la sua bozza formato 3 alla versione 1
// (la sola radice, nessun registro), con l'evento di apertura del server.
type scenaBozzaV1 struct {
	thread, prodotto, sotto, bozza, admin uuid.UUID
	snapshot                              map[string]any
}

// bancoBozzaV1: il banco web con le rotte della bozza e il guscio della pagina montati (se registraMotoreA non li monta
// gia'), il flag del motore A acceso, e la scena.
func bancoBozzaV1(t *testing.T) (*bancoWeb, *scenaBozzaV1) {
	t.Helper()
	b := preparaBancoWeb(t)
	mux := http.NewServeMux()
	b.ws.Registra(mux)
	id := mwUUID(1).String()
	if _, pat := mux.Handler(httptest.NewRequest(http.MethodPost, "/thread/"+id+"/motore-a/bozza", nil)); pat != "POST /thread/{id}/motore-a/bozza" {
		b.ws.registraBozzaMotoreA(mux)
	}
	if _, pat := mux.Handler(httptest.NewRequest(http.MethodGet, "/thread/"+id+percorsoDistintaUnica[len("/thread/{id}"):], nil)); pat != "GET "+percorsoDistintaUnica {
		b.ws.registraPaginaDistintaUnica(mux)
	}
	srv := httptest.NewServer(ProtezioneCSRF(mux))
	t.Cleanup(srv.Close)
	b.srv = srv
	mwAccendi(t, b, motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoIndiceAssente})

	s := &scenaBozzaV1{thread: uuid.New(), prodotto: uuid.New(), sotto: uuid.New()}
	s.admin = uuidSQL(t, b, `SELECT utente_id FROM utente WHERE sigla = 'AD'`)
	cliente, conv, msg := uuid.New(), uuid.New(), uuid.New()
	for _, x := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO cliente (cliente_id, cartella_nas, ragione_sociale) VALUES ($1, 'ACMEV1', 'ACME S.p.A.')`, []any{cliente}},
		{`INSERT INTO thread_offerta (thread_id, cliente_id, canale, data_inizio, oggetto) VALUES ($1, $2, 'outlook', now(), 'RFQ 7120001')`, []any{s.thread, cliente}},
		{`INSERT INTO conversazione (conversazione_id, canale, chiave_esterna, thread_id, primo_messaggio_il) VALUES ($1, 'outlook', 'conv-v1@acme.example', $2, now())`,
			[]any{conv, s.thread}},
		{`INSERT INTO messaggio (messaggio_id, canale, chiave_esterna, conversazione_id, thread_id, direzione, data_evento, oggetto)
			VALUES ($1, 'outlook', 'msg-v1@acme.example', $2, $3, 'entrata', now(), 'RFQ 7120001')`, []any{msg, conv, s.thread}},
		{`INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il)
			VALUES (gen_random_uuid(), $1, 1, '7120001.stp', 'stp', repeat('b', 64), now())`, []any{msg}},
		{`INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES ($1, $3, '7120001', 'finito', 'manuale', $4),
			($2, $3, '7120010', 'sottoassieme', 'manuale', $4)`, []any{s.prodotto, s.sotto, s.thread, s.admin}},
	} {
		if _, err := b.pool.Exec(b.ctx, x.sql, x.args...); err != nil {
			t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	clienteE := distintabozza.EmittenteCliente(cliente)
	s.snapshot = map[string]any{"formato": 3, "prodotto_id": s.prodotto.String(),
		"versione_bom_base": map[string]any{"conferma_id": nil, "numero": 0, "contatore": 0}, "firma_base": strings.Repeat("1", 64),
		"nodi": []any{map[string]any{"chiave_bozza": "k-root", "componente_id": s.prodotto.String(), "op": "riusa", "tipo": "finito", "origine": "registrato",
			"identita": map[string]any{"emittente": clienteE, "emittente_presunto": true, "codice": "7120001", "modo": "registrato"}}},
		"archi": []any{}, "decisioni": []any{}}
	raw, _ := json.Marshal(s.snapshot)
	snap, err := distintabozza.LeggiSnapshot3(raw)
	if err != nil {
		t.Fatalf("snapshot della scena: %v", err)
	}
	can, err := snap.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := b.pool.Begin(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(b.ctx) //nolint:errcheck
	q := db.New(tx)
	bz, err := q.InsertDistintaBozza(b.ctx, db.InsertDistintaBozzaParams{ThreadID: s.thread, ProdottoID: s.prodotto, Formato: distintabozza.FormatoSnapshot3,
		FirmaBase: strings.Repeat("1", 64), UtenteID: s.admin})
	if err != nil {
		t.Fatal(err)
	}
	s.bozza = bz.BozzaID
	if _, err := q.InsertDistintaBozzaVersione(b.ctx, db.InsertDistintaBozzaVersioneParams{BozzaID: bz.BozzaID, Versione: 1, Snapshot: can,
		Impronta: jsoncanonico.Impronta(can), FirmaBase: strings.Repeat("1", 64), SalvataDa: s.admin,
		ChiaveSalvataggio: uuid.NullUUID{UUID: uuid.New(), Valid: true}, ImprontaRichiesta: pgtype.Text{String: strings.Repeat("4", 64), Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertDistintaBozzaEvento(b.ctx, db.InsertDistintaBozzaEventoParams{BozzaID: bz.BozzaID, N: 1, Versione: 1,
		Vocabolario: int16(distintabozza.VocabolarioFormato3), Azione: distintabozza.AzBozzaAperta, Origine: "server", UtenteID: s.admin}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(b.ctx); err != nil {
		t.Fatal(err)
	}
	return b, s
}

// controllaSenzaPercorsi: T-15 su una risposta JSON.
func controllaSenzaPercorsi(t *testing.T, cosa, corpo string) {
	t.Helper()
	if bzHaPercorsi(corpo) {
		t.Errorf("%s: la risposta contiene un percorso: %s", cosa, corpo)
	}
}

func TestNessunaGetScriveDistintaV1(t *testing.T) {
	b, s := bancoBozzaV1(t)
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	op := operatore(b)
	base := "/thread/" + s.thread.String()
	get := []struct {
		percorso string
		json     bool
	}{
		{base + "/motore-a/bozza?prodotto=" + s.prodotto.String(), true},
		{base + "/motore-a/bozza?prodotto=" + s.prodotto.String() + "&eventi=1&importazione=1", true},
		{base + "/motore-a/bozza?prodotto=" + s.prodotto.String() + "&confronto=1", true},
		{base + "/motore-a/bozza?prodotto=" + s.prodotto.String() + "&effetto_cambio_tipo=" + s.sotto.String() + "&tipo_nuovo=sciolto", true},
		{base + "/motore-a/intento/" + uuid.New().String(), true},
		{base + "/motore-a/conferme?prodotto=" + s.prodotto.String(), true},
		{base + "/distinta-unica", false},
	}
	prima := fotoDelDatabase(t, b)
	for _, g := range get {
		resp, corpo := ad.fai(http.MethodGet, g.percorso, nil, false)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("admin, GET %s: %d %s", g.percorso, resp.StatusCode, corpo)
			continue
		}
		if g.json {
			controllaSenzaPercorsi(t, g.percorso, corpo)
			if resp.Header.Get("Cache-Control") != "no-store" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
				t.Errorf("GET %s: intestazioni %v", g.percorso, resp.Header)
			}
		}
		if resp2, _ := op.fai(http.MethodGet, g.percorso, nil, false); resp2.StatusCode != http.StatusForbidden {
			t.Errorf("operatore, GET %s: %d, atteso 403", g.percorso, resp2.StatusCode)
		}
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("le GET della Distinta unica hanno scritto in: %s", d)
	}
	// le risposte dicono quello che devono: R1 con n_eventi e il confronto non disponibile, la variante dell'effetto con
	// la firma, R6 non registrato, R7 con gli elenchi vuoti
	_, corpo := ad.fai(http.MethodGet, get[2].percorso, nil, false)
	var r1 struct {
		Stato     string `json:"stato"`
		Formato   int    `json:"formato"`
		NEventi   *int   `json:"n_eventi"`
		Confronto *struct {
			Errore string `json:"errore"`
		} `json:"confronto"`
	}
	if err := json.Unmarshal([]byte(corpo), &r1); err != nil || r1.Stato != "aperta" || r1.Formato != 3 || r1.NEventi == nil || *r1.NEventi != 1 ||
		r1.Confronto == nil || r1.Confronto.Errore == "" {
		t.Errorf("R1 con il confronto: %s", corpo)
	}
	_, corpo = ad.fai(http.MethodGet, get[3].percorso, nil, false)
	var eff struct {
		ComponenteID uuid.UUID `json:"componente_id"`
		TipoNuovo    string    `json:"tipo_nuovo"`
		FirmaEffetto string    `json:"firma_effetto"`
	}
	if err := json.Unmarshal([]byte(corpo), &eff); err != nil || eff.ComponenteID != s.sotto || eff.TipoNuovo != "sciolto" || len(eff.FirmaEffetto) < 24 {
		t.Errorf("effetto del cambio di tipo: %s", corpo)
	}
	if _, corpo = ad.fai(http.MethodGet, get[4].percorso, nil, false); !strings.Contains(corpo, `"stato":"non_registrato"`) {
		t.Errorf("R6: %s", corpo)
	}
	if _, corpo = ad.fai(http.MethodGet, get[5].percorso, nil, false); !strings.Contains(corpo, `"conferme":[]`) {
		t.Errorf("R7: %s", corpo)
	}
	// i rifiuti in lettura: un prodotto che non e' della RFQ (422 FUORI_RFQ), una RFQ che non c'e' (404)
	if resp, corpo := ad.fai(http.MethodGet, base+"/motore-a/bozza?prodotto="+uuid.New().String(), nil, false); resp.StatusCode != http.StatusUnprocessableEntity ||
		!strings.Contains(corpo, `"codice":"FUORI_RFQ"`) {
		t.Errorf("R1 fuori dalla RFQ: %d %s", resp.StatusCode, corpo)
	}
	if resp, _ := ad.fai(http.MethodGet, "/thread/"+uuid.New().String()+"/motore-a/bozza?prodotto="+s.prodotto.String(), nil, false); resp.StatusCode != http.StatusNotFound {
		t.Errorf("R1 di una RFQ che non c'e': %d", resp.StatusCode)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("le letture hanno scritto in: %s", d)
	}
}

func TestBozzaV1SalvaAbbandonaERiallineaViaHTTP(t *testing.T) {
	b, s := bancoBozzaV1(t)
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	base := "/thread/" + s.thread.String() + "/motore-a"
	nodo := map[string]any{"chiave_bozza": "k-u1", "op": "crea", "tipo": "commerciale", "origine": "utente", "descrizione": "Vite & dado <M8>",
		"identita": s.snapshot["nodi"].([]any)[0].(map[string]any)["identita"]}
	identita := map[string]any{}
	for k, v := range nodo["identita"].(map[string]any) {
		identita[k] = v
	}
	identita["codice"], identita["modo"] = "7120099", "manuale"
	nodo["identita"] = identita
	arco := map[string]any{"padre": "k-root", "figlio": "k-u1", "qta": 2, "origine": "utente"}
	nuovo := map[string]any{}
	for k, v := range s.snapshot {
		nuovo[k] = v
	}
	nuovo["nodi"] = append(append([]any{}, s.snapshot["nodi"].([]any)...), nodo)
	nuovo["archi"] = []any{arco}
	corpo := func(versione int, chiave uuid.UUID, snap map[string]any, azioni []any) string {
		b, _ := json.Marshal(map[string]any{"bozza_id": s.bozza, "versione_attesa": versione, "prodotto_id": s.prodotto, "vocabolario": 2,
			"chiave_salvataggio": chiave, "snapshot": snap, "azioni": azioni})
		return string(b)
	}
	azioni := []any{
		map[string]any{"azione": "nodo_aggiunto", "chiave_bozza": "k-u1", "dopo": nodo, "origine": "utente"},
		map[string]any{"azione": "arco_aggiunto", "chiave_bozza": "k-u1", "dopo": arco, "origine": "utente"},
	}
	resp, testo := ad.chiediJSON(http.MethodPost, base+"/bozza", corpo(1, uuid.New(), nuovo, azioni))
	controllaSenzaPercorsi(t, "R2", testo)
	if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"versione":2`) || !strings.Contains(testo, `"nuova_versione":true`) {
		t.Fatalf("R2: %d %s", resp.StatusCode, testo)
	}
	// lo snapshot di R1 e' quello salvato, senza l'escape HTML di encoding/json
	_, testo = ad.fai(http.MethodGet, base+"/bozza?prodotto="+s.prodotto.String(), nil, false)
	if !strings.Contains(testo, "Vite & dado <M8>") || strings.Contains(testo, `\u0026`) {
		t.Errorf("R1 dopo il salvataggio: %s", testo)
	}
	// una versione superata: 409 conflitto con lo stato della bozza
	resp, testo = ad.chiediJSON(http.MethodPost, base+"/bozza", corpo(1, uuid.New(), nuovo, nil))
	if resp.StatusCode != http.StatusConflict || !strings.Contains(testo, `"esito":"conflitto"`) || !strings.Contains(testo, `"codice":"BOZZA_VERSIONE"`) ||
		!strings.Contains(testo, `"versione":2`) {
		t.Errorf("R2 su una versione superata: %d %s", resp.StatusCode, testo)
	}
	// T-12, meta' L4: un corpo formato 2 (vocabolario 1, snapshot formato 2) e' 422 e non nasce niente del formato 2
	f2 := map[string]any{"bozza_id": s.bozza, "versione_attesa": 2, "prodotto_id": s.prodotto, "vocabolario": 1, "chiave_salvataggio": uuid.New(),
		"snapshot": map[string]any{"formato": 2, "prodotto_id": s.prodotto.String(), "versione_bom_base": map[string]any{"conferma_id": nil, "numero": 0, "contatore": 0},
			"firma_base": strings.Repeat("1", 64), "nodi": []any{map[string]any{"chiave_bozza": "r", "componente_id": s.prodotto.String(), "op": "riusa",
				"codice": "7120001", "tipo": "finito", "origine": "registrato"}}, "archi": []any{}}, "azioni": []any{}}
	raw, _ := json.Marshal(f2)
	formato2 := `SELECT (SELECT count(*) FROM distinta_bozza WHERE formato = 2) + (SELECT count(*) FROM distinta_bozza_versione v JOIN distinta_bozza x USING (bozza_id) WHERE x.formato = 2)`
	n2 := contaSQL(t, b, formato2)
	resp, testo = ad.chiediJSON(http.MethodPost, base+"/bozza", string(raw))
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(testo, `"campo":"vocabolario"`) {
		t.Errorf("R2 con un corpo formato 2: %d %s", resp.StatusCode, testo)
	}
	if contaSQL(t, b, formato2) != n2 || n2 != 0 {
		t.Errorf("righe formato 2 dopo R2: %d → %d", n2, contaSQL(t, b, formato2))
	}
	// R3: la versione superata e' 409, quella corrente chiude la bozza
	resp, testo = ad.chiediJSON(http.MethodPost, base+"/bozza/"+s.bozza.String()+"/abbandona", `{"versione_attesa":1,"motivo":"si ricomincia"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("R3 su una versione superata: %d %s", resp.StatusCode, testo)
	}
	resp, testo = ad.chiediJSON(http.MethodPost, base+"/bozza/"+s.bozza.String()+"/abbandona", `{"versione_attesa":2,"motivo":"si ricomincia"}`)
	controllaSenzaPercorsi(t, "R3", testo)
	if resp.StatusCode != http.StatusOK || contaSQL(t, b, `SELECT count(*) FROM distinta_bozza WHERE bozza_id = $1 AND stato = 'abbandonata'`, s.bozza) != 1 {
		t.Errorf("R3: %d %s", resp.StatusCode, testo)
	}
	// R11: gli stati di F8 non eseguiti della RFQ, creati prima, diventano eseguito con risolto_da riallinea
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO f8_stato (thread_id, chiave_intento) VALUES ($1, $2)`, s.thread, uuid.New()); err != nil {
		t.Fatal(err)
	}
	resp, testo = ad.chiediJSON(http.MethodPost, base+"/riallinea", `{}`)
	controllaSenzaPercorsi(t, "R11", testo)
	if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"f8_stato":"eseguito"`) || !strings.Contains(testo, `"aggiornati":1`) {
		t.Fatalf("R11: %d %s", resp.StatusCode, testo)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM f8_stato WHERE thread_id = $1 AND stato = 'eseguito' AND risolto_da = 'riallinea' AND tentativi = 1`, s.thread); n != 1 {
		t.Errorf("f8_stato dopo R11: %d", n)
	}
	// chi non e' admin non salva
	op := operatore(b)
	if resp, _ := op.chiediJSON(http.MethodPost, base+"/riallinea", `{}`); resp.StatusCode != http.StatusForbidden {
		t.Errorf("R11 dell'operatore: %d", resp.StatusCode)
	}
}
