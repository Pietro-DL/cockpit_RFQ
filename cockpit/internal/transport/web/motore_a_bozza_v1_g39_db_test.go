//go:build integrazione

// L4 — G39-02 [U] (20/10 sera), corsia persistenza parte A, con il Motore A vero e le regole ACME (sintetiche): lo STEP del
// prodotto finito si riconosce con la lettura del motore. Il caso del marcatore (PM-01) con codici sintetici: il prodotto 9123456A (la
// base 9123456 con il marcatore A della famiglia ACME) e lo STEP «9123456-001.stp», il cui nome il motore legge con la sola
// base: il motore lo riconosce candidato sorgente del prodotto (namespace, base e marcatore compatibili, come l'ancoraggio),
// e non riconosce lo STEP «9123457-001.stp» (un'altra base). Via HTTP: R2 rifiuta la PB su quello STEP (422
// STEP_CANDIDATO_DEL_FINITO, con il calcolo del motore che l'adattatore fa perche' la PB e' su uno STEP) e accetta quella
// sull'altro; R1 con ?suggerimenti_motore=1 da' lo STEP fra gli step_candidati del prodotto (l'avviso della pagina) e
// rifiuta un valore che non e' 1. Nessuna lettura di documento_proposta: la scena non ne ha.

package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
)

func TestV1G39_02StepDelFinitoConIlMotoreVeroDB(t *testing.T) {
	b, s := bancoBozzaV1(t)
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	base := "/thread/" + s.thread.String() + "/motore-a"
	fin := uuid.New()
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da)
		VALUES ($1, $2, '9123456A', 'finito', 'manuale', $3)`, fin, s.thread, s.admin); err != nil {
		t.Fatal(err)
	}
	msg := uuidSQL(t, b, `SELECT messaggio_id FROM messaggio WHERE thread_id = '`+s.thread.String()+`'`)
	allegato := func(nome string, sha byte) uuid.UUID {
		id := uuid.New()
		if _, err := b.pool.Exec(b.ctx, `INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il, natura)
			VALUES ($1, $2, (SELECT COALESCE(max(indice), 0) + 1 FROM allegato WHERE messaggio_id = $2), $3, 'stp', repeat($4, 64), now(), 'file')`,
			id, msg, nome, string(sha)); err != nil {
			t.Fatal(err)
		}
		return id
	}
	stepFinito := allegato("9123456-001.stp", 'd')
	stepAltro := allegato("9123457-001.stp", 'e')
	accendiMotoreV1(t, b, s.thread)

	// il riconoscimento del motore, sul calcolo vero
	ris, err := b.ws.servizioMotoreA().Calcola(b.ctx, s.thread)
	if err != nil {
		t.Fatalf("il calcolo del motore: %v", err)
	}
	cand := distintabozza.StepCandidatiDalMotore(ris.EsitoThread, ris.Thread)
	ha := func(a, p uuid.UUID) bool {
		for _, x := range cand.Prodotti[a] {
			if x == p {
				return true
			}
		}
		return false
	}
	if !cand.DalMotore || !ha(stepFinito, fin) {
		var diag []string
		for _, f := range ris.EsitoThread.Ancoraggi.File {
			if f.AllegatoID == stepFinito {
				raw, _ := json.Marshal(f)
				diag = append(diag, string(raw))
			}
		}
		t.Fatalf("il motore non riconosce lo STEP 9123456-001.stp come candidato del prodotto 9123456A: %+v; ancoraggio %v", cand, diag)
	}
	if len(cand.Prodotti[stepAltro]) != 0 {
		t.Fatalf("lo STEP di un'altra base riconosciuto candidato: %v", cand.Prodotti[stepAltro])
	}

	// R2 via HTTP: la PB sullo STEP del finito 9123456A nella bozza dell'altro prodotto e' rifiutata
	corpo := func(a uuid.UUID) string {
		m := map[string]any{}
		for k, v := range s.snapshot {
			m[k] = v
		}
		pb := map[string]any{"allegato_id": a, "chiave_bozza": "k-root"}
		m["documenti_preassociati"] = []any{pb}
		raw, _ := json.Marshal(map[string]any{"bozza_id": s.bozza, "versione_attesa": 1, "prodotto_id": s.prodotto, "vocabolario": 2,
			"chiave_salvataggio": uuid.New(), "snapshot": m,
			"azioni": []any{map[string]any{"azione": "file_preassociato", "chiave_bozza": "k-root", "dopo": pb, "origine": "utente"}}})
		return string(raw)
	}
	prima := fotoDelDatabase(t, b)
	resp, testo := ad.chiediJSON(http.MethodPost, base+"/bozza", corpo(stepFinito))
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(testo, `"codice":"STEP_CANDIDATO_DEL_FINITO"`) ||
		!strings.Contains(testo, stepFinito.String()) {
		t.Fatalf("R2 con la PB sullo STEP del finito: %d %s", resp.StatusCode, testo)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("il rifiuto ha scritto in: %s", d)
	}
	// lo STEP di un'altra base, che il motore non riconosce, si preassocia come un documento
	if resp, testo := ad.chiediJSON(http.MethodPost, base+"/bozza", corpo(stepAltro)); resp.StatusCode != http.StatusOK {
		t.Fatalf("R2 con la PB sullo STEP di un'altra base: %d %s", resp.StatusCode, testo)
	}

	// R1 con ?suggerimenti_motore=1 per il prodotto 9123456A (nessuna bozza): lo STEP fra gli step_candidati
	resp, testo = ad.chiediJSON(http.MethodGet, base+"/bozza?prodotto="+fin.String()+"&suggerimenti_motore=1", "")
	var l distintabozza.LetturaR1
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &l) != nil || l.Stato != distintabozza.StatoNessunaBozza ||
		l.SuggerimentiMotore == nil || !l.SuggerimentiMotore.Calcolati || l.StepCandidati == nil || len(*l.StepCandidati) != 1 ||
		(*l.StepCandidati)[0].AllegatoID != stepFinito || !(*l.StepCandidati)[0].DalMotore {
		t.Fatalf("R1 con i suggerimenti del motore: %d %s", resp.StatusCode, testo)
	}
	// senza il parametro: niente del motore (R1 non ricalcola, D-18)
	resp, testo = ad.chiediJSON(http.MethodGet, base+"/bozza?prodotto="+fin.String(), "")
	if resp.StatusCode != http.StatusOK || strings.Contains(testo, "step_candidati") || strings.Contains(testo, "suggerimenti_motore") {
		t.Fatalf("R1 senza il parametro: %d %s", resp.StatusCode, testo)
	}
	resp, testo = ad.chiediJSON(http.MethodGet, base+"/bozza?prodotto="+fin.String()+"&suggerimenti_motore=si", "")
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(testo, `"campo":"suggerimenti_motore"`) {
		t.Fatalf("R1 con suggerimenti_motore=si: %d %s", resp.StatusCode, testo)
	}
}

// R1 con ?suggerimenti_motore=1 e le regole del motore non pronte: 200, suggerimenti_motore.errore CALCOLO_OCCUPATO, i
// suggerimenti del registro della versione come senza il parametro (mai un 5xx per il parametro).
func TestV1G39_01R1SuggerimentiMotoreNonPronto(t *testing.T) {
	b, s := bancoBozzaV1(t)
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	resp, testo := ad.chiediJSON(http.MethodGet, "/thread/"+s.thread.String()+"/motore-a/bozza?prodotto="+s.prodotto.String()+"&suggerimenti_motore=1", "")
	var l distintabozza.LetturaR1
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &l) != nil || l.Stato != distintabozza.StatoAperta ||
		l.SuggerimentiMotore == nil || l.SuggerimentiMotore.Calcolati || l.SuggerimentiMotore.Errore != distintabozza.MotoreErroreOccupato ||
		l.PreassociazioniProposte == nil || l.StepCandidati != nil {
		t.Fatalf("R1 con il motore non pronto: %d %s", resp.StatusCode, testo)
	}
}

// R2 con una PB nuova su uno STEP e il motore che non risponde (rilievo minore 4 della revisione G39 parte A): le regole
// ACME pronte ma con un limite di un nanosecondo, quindi il calcolo scade. R2 non ripiega sulla lettura legacy (che non
// riconoscerebbe lo STEP «9123456-001.stp» come candidato del finito 9123456A: la scena non ha documento_proposta): 504
// CALCOLO_SCADUTO, esito non_registrato, niente scritto. Sul codice di prima la PB era salvata (200). Con le regole non
// configurate resta il ripiego [T-39]: lo provano le L4 di R2 con il banco a regole assenti, che salvano le PB sugli STEP.
func TestV1G39_02R2ConLoStepEIlMotoreScadutoDB(t *testing.T) {
	b, s := bancoBozzaV1(t)
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da)
		VALUES ($1, $2, '9123456A', 'finito', 'manuale', $3)`, uuid.New(), s.thread, s.admin); err != nil {
		t.Fatal(err)
	}
	msg := uuidSQL(t, b, `SELECT messaggio_id FROM messaggio WHERE thread_id = '`+s.thread.String()+`'`)
	step := uuid.New()
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il, natura)
		VALUES ($1, $2, (SELECT COALESCE(max(indice), 0) + 1 FROM allegato WHERE messaggio_id = $2), '9123456-001.stp', 'stp', repeat('d', 64), now(), 'file')`,
		step, msg); err != nil {
		t.Fatal(err)
	}
	cliente := uuidSQL(t, b, `SELECT cliente_id FROM thread_offerta WHERE thread_id = '`+s.thread.String()+`'`)
	mwAccendi(t, b, mwRegoleACME(t, cliente, "ACME S.p.A.", time.Nanosecond))
	m := map[string]any{}
	for k, v := range s.snapshot {
		m[k] = v
	}
	pb := map[string]any{"allegato_id": step, "chiave_bozza": "k-root"}
	m["documenti_preassociati"] = []any{pb}
	raw, _ := json.Marshal(map[string]any{"bozza_id": s.bozza, "versione_attesa": 1, "prodotto_id": s.prodotto, "vocabolario": 2,
		"chiave_salvataggio": uuid.New(), "snapshot": m,
		"azioni": []any{map[string]any{"azione": "file_preassociato", "chiave_bozza": "k-root", "dopo": pb, "origine": "utente"}}})
	prima := fotoDelDatabase(t, b)
	resp, testo := ad.chiediJSON(http.MethodPost, "/thread/"+s.thread.String()+"/motore-a/bozza", string(raw))
	if resp.StatusCode != http.StatusGatewayTimeout || !strings.Contains(testo, `"codice":"CALCOLO_SCADUTO"`) || !strings.Contains(testo, `"esito":"non_registrato"`) {
		t.Fatalf("R2 con lo STEP e il motore scaduto: %d %s", resp.StatusCode, testo)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("R2 con il motore scaduto ha scritto in: %s", d)
	}
}

// La nascita da-conferma-1 con il motore (rilievo minore 5 della revisione G39 parte A): l'adattatore calcola gli STEP
// candidati del finito anche per la partenza conferma, perche' le PB iniziali della revisione li escludano con il
// riconoscimento del motore (la regola del core e' la stessa della partenza step: presaInCarico con Proposta.StepCandidati).
// Con le regole pronte e un limite di un nanosecondo il calcolo scade: 504 CALCOLO_SCADUTO, niente scritto. Sul codice di
// prima la partenza conferma non calcolava il motore (qui: 422 partenza_non_ammessa, il prodotto non ha conferme). Con le
// regole non configurate resta il ripiego [T-39] (TestBozzaV1C3CreaViaHTTP: 422 partenza_non_ammessa, come prima).
func TestV1G39_02DaConfermaCalcolaGliStepCandidatiDB(t *testing.T) {
	b, s := bancoBozzaV1(t)
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	nuovo := uuid.New()
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da)
		VALUES ($1, $2, '9123456A', 'finito', 'manuale', $3)`, nuovo, s.thread, s.admin); err != nil {
		t.Fatal(err)
	}
	cliente := uuidSQL(t, b, `SELECT cliente_id FROM thread_offerta WHERE thread_id = '`+s.thread.String()+`'`)
	mwAccendi(t, b, mwRegoleACME(t, cliente, "ACME S.p.A.", time.Nanosecond))
	raw, _ := json.Marshal(map[string]any{"bozza_id": nil, "versione_attesa": 0, "prodotto_id": nuovo, "vocabolario": 2,
		"chiave_salvataggio": uuid.New(), "crea": map[string]any{"partenza": "conferma"}})
	prima := fotoDelDatabase(t, b)
	resp, testo := ad.chiediJSON(http.MethodPost, "/thread/"+s.thread.String()+"/motore-a/bozza", string(raw))
	if resp.StatusCode != http.StatusGatewayTimeout || !strings.Contains(testo, `"codice":"CALCOLO_SCADUTO"`) || !strings.Contains(testo, `"esito":"non_registrato"`) {
		t.Fatalf("crea da conferma con il motore scaduto: %d %s", resp.StatusCode, testo)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("la crea con il motore scaduto ha scritto in: %s", d)
	}
}
