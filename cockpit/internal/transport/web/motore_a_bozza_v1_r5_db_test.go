//go:build integrazione

// L4 — R5 formato 3 via HTTP (motore_a_bozza_v1_r5.go; EM §9.5; direttiva 39 §1, §2, §8 [U + T-39]), corsia persistenza,
// con il banco web vero (sessioni, CSRF, database di prova): la bozza senza STEP dà 422 BLOCCHI con
// STEP_SORGENTE_NON_CONFERMATA e nessuna riga cambia (anche la coda del NAS); con la sorgente confermata da S8 (sola radice)
// la conferma passa (200), R5 promuove lo STEP, F8 gira dopo il COMMIT e lo scrive in f8_stato; il reinvio e' riproposto
// senza scritture; R6 trova l'intento conferma_bom_v3; i rifiuti di forma (senza bozza_id, un parametro), un altro metodo,
// un operatore. Dati inventati: ACME, codici 712xxxx.

package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
)

func TestV1D39R5ViaHTTP(t *testing.T) {
	b, s := bancoBozzaV1(t)
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	op := operatore(b)
	base := "/thread/" + s.thread.String() + "/motore-a/bom/"
	// la bozza della scena ha una base finta: si riparte dalla partenza vuota (abbandonando quella)
	c, err := distintabozza.CreaFormato3(b.ctx, b.pool, distintabozza.Creazione3{ThreadID: s.thread, UtenteID: s.admin,
		Comune: distintabozza.ComuneR2{ProdottoID: s.prodotto, ChiaveSalvataggio: uuid.New(), ImprontaRichiesta: strings.Repeat("a", 64)},
		Crea: distintabozza.CreaR2{Partenza: distintabozza.PartenzaVuota,
			Abbandona: &distintabozza.AbbandonaR2{BozzaID: s.bozza, VersioneAttesa: 1, Motivo: "ripartenza della prova"}}})
	if err != nil {
		t.Fatalf("crea vuota: %v", err)
	}
	corpoBozza := func(versione int) map[string]any {
		var raw json.RawMessage
		if err := b.pool.QueryRow(b.ctx, `SELECT snapshot FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = $2`, c.BozzaID, versione).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		m := map[string]any{}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		m["bozza_id"], m["bozza_versione"] = c.BozzaID, versione
		return m
	}
	riepilogo := func(versione int, motore bool) distintabozza.RiepilogoR4 {
		raw, _ := json.Marshal(corpoBozza(versione))
		percorso := base + "riepilogo"
		if motore {
			percorso += "?avvisi_motore=1"
		}
		resp, testo := ad.chiediJSON(http.MethodPost, percorso, string(raw))
		var r distintabozza.RiepilogoR4
		if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &r) != nil {
			t.Fatalf("R4: %d %s", resp.StatusCode, testo)
		}
		return r
	}
	conferma := func(versione int, firma string, chiave uuid.UUID, cambia func(m map[string]any)) string {
		m := corpoBozza(versione)
		m["chiave_idempotenza"] = chiave
		corpo := map[string]any{"firma_anteprima": firma, "bozza": m}
		if cambia != nil {
			cambia(corpo)
		}
		raw, _ := json.Marshal(corpo)
		return string(raw)
	}
	// la bozza senza STEP: 422 BLOCCHI con il gate, nessuna riga cambia
	r := riepilogo(1, false)
	if len(r.Blocchi) != 1 || r.Blocchi[0].Codice != distintabozza.BloccoStepSorgenteNonConfermata {
		t.Fatalf("R4 senza STEP: %+v", r.Blocchi)
	}
	prima := fotoDelDatabase(t, b)
	resp, testo := ad.chiediJSON(http.MethodPost, base+"conferma", conferma(1, r.FirmaAnteprima, uuid.New(), nil))
	var rif struct {
		Esito   string                  `json:"esito"`
		Codice  string                  `json:"codice"`
		Blocchi []distintabozza.Rilievo `json:"blocchi"`
	}
	if resp.StatusCode != http.StatusUnprocessableEntity || json.Unmarshal([]byte(testo), &rif) != nil || rif.Codice != distintabozza.CodiceBlocchi ||
		rif.Esito != "rifiutato" || len(rif.Blocchi) != 1 || rif.Blocchi[0].Codice != distintabozza.BloccoStepSorgenteNonConfermata ||
		rif.Blocchi[0].Motivo != distintabozza.SorgenteMotivoSenzaSorgente {
		t.Fatalf("R5 senza STEP: %d %s", resp.StatusCode, testo)
	}
	controllaSenzaPercorsi(t, "R5 422", testo)
	// i rifiuti di forma, il metodo, il ruolo
	for _, x := range []struct {
		nome, percorso, corpo, frammento string
		stato                            int
	}{
		{"senza bozza_id", base + "conferma", conferma(1, r.FirmaAnteprima, uuid.New(), func(m map[string]any) { delete(m["bozza"].(map[string]any), "bozza_id") }),
			`"motivo":"bozza_obbligatoria"`, http.StatusUnprocessableEntity},
		{"un parametro", base + "conferma?x=1", conferma(1, r.FirmaAnteprima, uuid.New(), nil), `"campo":"x"`, http.StatusUnprocessableEntity},
		{"campo sconosciuto", base + "conferma", conferma(1, r.FirmaAnteprima, uuid.New(), func(m map[string]any) { m["autorizza"] = true }),
			`"codice":"CORPO_NON_VALIDO"`, http.StatusUnprocessableEntity},
		{"RFQ che non c'e'", "/thread/" + uuid.New().String() + "/motore-a/bom/conferma", conferma(1, r.FirmaAnteprima, uuid.New(), nil),
			`"codice":"RFQ_INESISTENTE"`, http.StatusNotFound},
	} {
		resp, testo := ad.chiediJSON(http.MethodPost, x.percorso, x.corpo)
		if resp.StatusCode != x.stato || !strings.Contains(testo, x.frammento) {
			t.Errorf("%s: %d %s", x.nome, resp.StatusCode, testo)
		}
	}
	if resp, _ := ad.fai(http.MethodGet, base+"conferma", nil, false); resp.StatusCode != http.StatusMethodNotAllowed || resp.Header.Get("Allow") != "POST" {
		t.Errorf("GET: %d %q", resp.StatusCode, resp.Header.Get("Allow"))
	}
	if resp, _ := op.chiediJSON(http.MethodPost, base+"conferma", conferma(1, r.FirmaAnteprima, uuid.New(), nil)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("operatore: %d", resp.StatusCode)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Fatalf("R5 rifiutata ha scritto in: %s", d)
	}
	// S8 conferma lo STEP del prodotto come sorgente; S8 passo 3 riconcilia la bozza (solo_prodotto, versione 2)
	allegato := uuidSQL(t, b, `SELECT a.allegato_id FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE m.thread_id = '`+s.thread.String()+`'`)
	sha := strings.Repeat("b", 64)
	var doc uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, $2, 'cad_3d', '7120001', '7120001.stp', 'stp', $3, '3D/7120001.stp', $4) RETURNING documento_id`, s.thread, s.prodotto, sha, s.admin).Scan(&doc); err != nil {
		t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, documento_id, chiave_idempotenza, autore_id)
		VALUES ($1, $2, 'prodotto', 'conferma_documento', $3, $4, $5, gen_random_uuid(), $6)`, s.thread, s.prodotto, allegato, sha, doc, s.admin); err != nil {
		t.Fatal(err)
	}
	if _, err := distintabozza.SoloProdottoSuBozzaFormato3(b.ctx, b.pool, distintabozza.SoloProdottoSuBozza3{ThreadID: s.thread, UtenteID: s.admin,
		Comune:   distintabozza.ComuneR2{ProdottoID: s.prodotto, BozzaID: c.BozzaID, VersioneAttesa: 1, ChiaveSalvataggio: uuid.New(), ImprontaRichiesta: strings.Repeat("a", 64)},
		Sorgente: distintabozza.SorgenteDichiarata{AllegatoID: allegato, Gruppo: distintabozza.GruppoSoloProdotto}, Motivo: "STEP senza struttura importabile",
		PresaDAttoAnalisi: "pronta"}); err != nil {
		t.Fatalf("S8 passo 3: %v", err)
	}
	if n := uuidSQL(t, b, `SELECT COALESCE(step_strutturale_id, '00000000-0000-0000-0000-000000000000') FROM componente WHERE componente_id = '`+s.prodotto.String()+`'`); n != uuid.Nil {
		t.Fatal("S8 non scrive step_strutturale_id")
	}
	// il motore non pronto: R4 con il motore non lo calcola, R5 503 CALCOLO_OCCUPATO non registrato, niente scritto
	rm := riepilogo(2, true)
	if len(rm.Blocchi) != 0 || rm.Confermabile || rm.AvvisiMotoreCalcolati || rm.AvvisiMotoreErrore == nil ||
		rm.AvvisiMotoreErrore.Codice != distintabozza.MotoreErroreOccupato {
		t.Fatalf("R4 con il motore non pronto: %+v %+v", rm.Blocchi, rm.AvvisiMotoreErrore)
	}
	primaM := fotoDelDatabase(t, b)
	resp, testo = ad.chiediJSON(http.MethodPost, base+"conferma", conferma(2, rm.FirmaAnteprima, uuid.New(), nil))
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(testo, `"codice":"CALCOLO_OCCUPATO"`) || !strings.Contains(testo, `"esito":"non_registrato"`) {
		t.Fatalf("R5 con il motore non pronto: %d %s", resp.StatusCode, testo)
	}
	if d := differenze(primaM, fotoDelDatabase(t, b)); d != "" {
		t.Fatalf("R5 senza il motore ha scritto in: %s", d)
	}
	// le regole pronte: il riepilogo con il motore e' confermabile, quello leggero no e ha un'altra firma; R5 con la firma del
	// leggero risponde 409 con il riepilogo nuovo (con il motore)
	accendiMotoreV1(t, b, s.thread)
	rl := riepilogo(2, false)
	r = riepilogo(2, true)
	if len(r.Blocchi) != 0 || !r.Confermabile || !r.AvvisiMotoreCalcolati || rl.Confermabile || rl.FirmaAnteprima == r.FirmaAnteprima {
		t.Fatalf("R4 con la sorgente: %+v %v / leggero %v", r.Blocchi, r.Confermabile, rl.Confermabile)
	}
	resp, testo = ad.chiediJSON(http.MethodPost, base+"conferma", conferma(2, rl.FirmaAnteprima, uuid.New(), nil))
	var sup struct {
		Codice    string                     `json:"codice"`
		Riepilogo *distintabozza.RiepilogoR4 `json:"riepilogo"`
	}
	if resp.StatusCode != http.StatusConflict || json.Unmarshal([]byte(testo), &sup) != nil || sup.Codice != distintabozza.CodiceAnteprimaSuperata ||
		sup.Riepilogo == nil || sup.Riepilogo.FirmaAnteprima != r.FirmaAnteprima {
		t.Fatalf("R5 con la firma del riepilogo leggero: %d %s", resp.StatusCode, testo)
	}
	if d := differenze(primaM, fotoDelDatabase(t, b)); d != "" {
		t.Fatalf("R5 superata ha scritto in: %s", d)
	}
	chiave := uuid.New()
	corpo := conferma(2, r.FirmaAnteprima, chiave, nil)
	resp, testo = ad.chiediJSON(http.MethodPost, base+"conferma", corpo)
	var out distintabozza.ConfermaR5
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "no-store" || json.Unmarshal([]byte(testo), &out) != nil ||
		out.Esito != "confermata" || out.NumeroConferma != 1 || !out.Fonte.Promossa || out.Riproposto || out.Formato != 3 {
		t.Fatalf("R5: %d %s", resp.StatusCode, testo)
	}
	controllaSenzaPercorsi(t, "R5 200", testo)
	t.Logf("F8 dopo il COMMIT di R5: %s", out.F8Stato)
	if out.F8Stato != "eseguito" && out.F8Stato != "errore" {
		t.Fatalf("F8 dopo il COMMIT: %q", out.F8Stato)
	}
	if n := uuidSQL(t, b, `SELECT step_strutturale_id FROM componente WHERE componente_id = '`+s.prodotto.String()+`'`); n != doc {
		t.Fatal("R5 promuove lo STEP")
	}
	var f8 string
	if err := b.pool.QueryRow(b.ctx, `SELECT stato FROM f8_stato WHERE conferma_id = $1`, out.ConfermaID).Scan(&f8); err != nil || f8 != out.F8Stato {
		t.Fatalf("f8_stato: %q %v", f8, err)
	}
	// il reinvio: riproposto, nessuna scrittura
	dopo := fotoDelDatabase(t, b)
	resp, testo = ad.chiediJSON(http.MethodPost, base+"conferma", corpo)
	var out2 distintabozza.ConfermaR5
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &out2) != nil || !out2.Riproposto || out2.ConfermaID != out.ConfermaID ||
		out2.F8Stato != out.F8Stato {
		t.Fatalf("reinvio: %d %s", resp.StatusCode, testo)
	}
	if d := differenze(dopo, fotoDelDatabase(t, b)); d != "" {
		t.Fatalf("il reinvio ha scritto in: %s", d)
	}
	// R6: l'intento conferma_bom_v3
	resp, testo = ad.chiediJSON(http.MethodGet, "/thread/"+s.thread.String()+"/motore-a/intento/"+chiave.String(), "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"stato":"registrato"`) || !strings.Contains(testo, `"gesto":"conferma_bom_v3"`) {
		t.Fatalf("R6: %d %s", resp.StatusCode, testo)
	}
	// una chiave nuova sulla bozza chiusa: 409 BOZZA_VERSIONE
	resp, testo = ad.chiediJSON(http.MethodPost, base+"conferma", conferma(2, r.FirmaAnteprima, uuid.New(), nil))
	if resp.StatusCode != http.StatusConflict || !strings.Contains(testo, `"codice":"BOZZA_VERSIONE"`) || !strings.Contains(testo, `"esito":"conflitto"`) {
		t.Fatalf("bozza chiusa: %d %s", resp.StatusCode, testo)
	}
}
