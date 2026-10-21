//go:build integrazione

// L4 — R4 formato 3 via HTTP (motore_a_bozza_v1_r4.go; EM §9.4; direttiva 39 §1, §2 [U + T-39]), corsia persistenza, con il
// banco web vero (sessioni, CSRF, database di prova): la bozza senza STEP dà 200 con il blocco firmato
// STEP_SORGENTE_NON_CONFERMATA (non un errore della lettura); ?avvisi_motore=1 con le regole del motore non pronte dà 200
// con gli avvisi del motore non calcolati (CALCOLO_OCCUPATO) e la firma del riepilogo leggero; con le regole pronte il motore
// e' calcolato e la firma cambia; gli ingressi stanno nella risposta, fuori firma; i rifiuti (parametro sconosciuto, corpo formato 2, prodotto di un'altra RFQ, RFQ che non
// c'e', un altro metodo, un operatore); la conferma della sorgente da S8 toglie il gate (radice sola: niente mono_pezzo);
// nessuna riga cambia (R4 e' una POST di sola lettura, T-04). Dati inventati: ACME, codici 712xxxx.

package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
)

func TestV1D39R4ViaHTTP(t *testing.T) {
	b, s := bancoBozzaV1(t)
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	op := operatore(b)
	percorso := "/thread/" + s.thread.String() + "/motore-a/bom/riepilogo"
	var base map[string]any
	firmaBase := ""
	corpo := func(cambia func(m map[string]any)) string {
		m := map[string]any{}
		for k, v := range s.snapshot {
			m[k] = v
		}
		if base != nil {
			m["versione_bom_base"], m["firma_base"] = base, firmaBase
		}
		m["documenti_preassociati"] = []any{}
		m["bozza_id"], m["bozza_versione"] = s.bozza, 1
		if cambia != nil {
			cambia(m)
		}
		raw, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	prima := fotoDelDatabase(t, b)
	// la base della scena e' finta: la prima lettura dice BASE_SUPERATA con la versione corrente, e la bozza si riallinea
	// (nel corpo) alla versione corrente
	resp, testo := ad.chiediJSON(http.MethodPost, percorso, corpo(nil))
	var r0 distintabozza.RiepilogoR4
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &r0) != nil || r0.Base.Stato != distintabozza.BaseSuperata {
		t.Fatalf("R4 con la base finta: %d %s", resp.StatusCode, testo)
	}
	base = map[string]any{"conferma_id": nil, "numero": 0, "contatore": r0.VersioneCorrente.Contatore}
	firmaBase = r0.VersioneCorrente.FirmaStruttura
	resp, testo = ad.chiediJSON(http.MethodPost, percorso, corpo(nil))
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "no-store" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("R4: %d %v %s", resp.StatusCode, resp.Header, testo)
	}
	controllaSenzaPercorsi(t, "R4", testo)
	var r distintabozza.RiepilogoR4
	if err := json.Unmarshal([]byte(testo), &r); err != nil {
		t.Fatalf("R4 non si legge: %v %s", err, testo)
	}
	if len(r.Blocchi) != 1 || r.Blocchi[0].Codice != distintabozza.BloccoStepSorgenteNonConfermata || r.Blocchi[0].Motivo != distintabozza.SorgenteMotivoSenzaSorgente ||
		r.Confermabile || len(r.FirmaAnteprima) != 64 || r.Formato != 3 || r.AvvisiMotoreCalcolati {
		t.Fatalf("la bozza senza STEP: %s", testo)
	}
	if r.Ingressi == nil || r.Ingressi.ImprontaIngressi == "" {
		t.Fatalf("ingressi nella risposta: %+v", r.Ingressi)
	}
	// avvisi del motore con le regole non pronte: 200, non calcolati (CALCOLO_OCCUPATO), la firma del riepilogo leggero
	resp, testo = ad.chiediJSON(http.MethodPost, percorso+"?avvisi_motore=1", corpo(nil))
	var rm distintabozza.RiepilogoR4
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &rm) != nil || rm.AvvisiMotoreCalcolati || rm.AvvisiMotoreErrore == nil ||
		rm.AvvisiMotoreErrore.Codice != distintabozza.MotoreErroreOccupato || rm.FirmaAnteprima != r.FirmaAnteprima {
		t.Fatalf("avvisi del motore non pronti: %d %s", resp.StatusCode, testo)
	}
	// con le regole pronte: il motore e' calcolato, la firma cambia (gli avvisi e i conflitti del motore sono firmati)
	accendiMotoreV1(t, b, s.thread)
	resp, testo = ad.chiediJSON(http.MethodPost, percorso+"?avvisi_motore=1", corpo(nil))
	var rc distintabozza.RiepilogoR4
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &rc) != nil || !rc.AvvisiMotoreCalcolati || rc.AvvisiMotoreErrore != nil ||
		rc.FirmaAnteprima == r.FirmaAnteprima || rc.ConflittiMotore == nil || rc.Confermabile {
		t.Fatalf("avvisi del motore calcolati: %d %s", resp.StatusCode, testo)
	}
	// i rifiuti
	for _, c := range []struct {
		nome, percorso, corpo, codice, frammento string
		stato                                    int
	}{
		{"parametro sconosciuto", percorso + "?eventi=1", corpo(nil), distintabozza.CodiceCorpoNonValido, `"campo":"eventi"`, http.StatusUnprocessableEntity},
		{"avvisi_motore non 1", percorso + "?avvisi_motore=si", corpo(nil), distintabozza.CodiceCorpoNonValido, `"campo":"avvisi_motore"`, http.StatusUnprocessableEntity},
		{"formato 2", percorso, corpo(func(m map[string]any) { m["formato"] = 2 }), distintabozza.CodiceCorpoNonValido, `"motivo":"formato_da_convertire"`, http.StatusUnprocessableEntity},
		{"prodotto di un'altra RFQ", percorso, corpo(func(m map[string]any) {
			altro := uuid.New()
			radice := map[string]any{}
			for k, v := range s.snapshot["nodi"].([]any)[0].(map[string]any) {
				radice[k] = v
			}
			radice["componente_id"] = altro
			m["prodotto_id"], m["nodi"] = altro, []any{radice}
			delete(m, "bozza_id")
			delete(m, "bozza_versione")
		}), distintabozza.CodiceFuoriRFQ, "", http.StatusUnprocessableEntity},
		{"RFQ che non c'e'", "/thread/" + uuid.New().String() + "/motore-a/bom/riepilogo", corpo(nil), distintabozza.CodiceRFQInesistente, "", http.StatusNotFound},
	} {
		resp, testo := ad.chiediJSON(http.MethodPost, c.percorso, c.corpo)
		if resp.StatusCode != c.stato || !strings.Contains(testo, `"codice":"`+c.codice+`"`) || !strings.Contains(testo, c.frammento) {
			t.Errorf("%s: %d %s", c.nome, resp.StatusCode, testo)
		}
	}
	if resp, _ := ad.fai(http.MethodGet, percorso, nil, false); resp.StatusCode != http.StatusMethodNotAllowed || resp.Header.Get("Allow") != "POST" {
		t.Errorf("GET: %d %q", resp.StatusCode, resp.Header.Get("Allow"))
	}
	if resp, _ := op.chiediJSON(http.MethodPost, percorso, corpo(nil)); resp.StatusCode != http.StatusForbidden {
		t.Errorf("operatore: %d", resp.StatusCode)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("R4 ha scritto in: %s", d)
	}
	// S8 conferma lo STEP del prodotto come sorgente (documento cad_3d e conferma_documento con la chiave del passo 2): con la
	// sola radice R4 non ha blocchi, il criterio resta «nessuno» (niente mono_pezzo dedotto)
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
	conS8 := corpo(func(m map[string]any) {
		m["sorgente"] = map[string]any{"allegato_id": allegato, "sha256": sha, "gruppo": distintabozza.GruppoSoloProdotto}
		delete(m, "bozza_id")
		delete(m, "bozza_versione")
	})
	resp, testo = ad.chiediJSON(http.MethodPost, percorso+"?avvisi_motore=1", conS8)
	var r8 distintabozza.RiepilogoR4
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &r8) != nil || len(r8.Blocchi) != 0 || !r8.Confermabile ||
		r8.CriterioPrevisto.Criterio != "nessuno" || r8.SorgenteLegata == nil || !r8.SorgenteLegata.Confermata || !r8.AvvisiMotoreCalcolati {
		t.Fatalf("con la sorgente S8: %d %s", resp.StatusCode, testo)
	}
	// il riepilogo leggero della stessa bozza non e' confermabile (PA-C01)
	resp, testo = ad.chiediJSON(http.MethodPost, percorso, conS8)
	var r8l distintabozza.RiepilogoR4
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &r8l) != nil || len(r8l.Blocchi) != 0 || r8l.Confermabile ||
		r8l.FirmaAnteprima == r8.FirmaAnteprima {
		t.Fatalf("riepilogo leggero: %d %s", resp.StatusCode, testo)
	}
}
