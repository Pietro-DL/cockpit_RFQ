package web

// distinta_unica_r45_test.go: L1 della corsia interfaccia per il wizard R4/R5 formato 3 e le AR nella pagina (direttiva 39
// §1, §4, §5, §8): il banco finto in pari con le forme del server (distintabozza.RiepilogoR4 e ConfermaR5, i campi letti con
// la riflessione), i suoi esiti (gate della sorgente, gruppi D-PB-04, sostituzione con lo svincolo, AR scritte solo da R5,
// 503 e 409 di R5), e i pezzi della pagina e del ponte che li leggono.

import (
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/distintabozza"
)

// campiJSON: i nomi JSON dei campi di un tipo (con i campi delle strutture incorporate) e quelli senza omitempty.
func campiJSON(tp reflect.Type) (tutti, sempre map[string]bool) {
	tutti, sempre = map[string]bool{}, map[string]bool{}
	for i := 0; i < tp.NumField(); i++ {
		f := tp.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct {
			t2, s2 := campiJSON(f.Type)
			for k := range t2 {
				tutti[k] = true
			}
			for k := range s2 {
				sempre[k] = true
			}
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" || tag == "" {
			continue
		}
		parti := strings.Split(tag, ",")
		tutti[parti[0]] = true
		if !strings.Contains(tag, "omitempty") {
			sempre[parti[0]] = true
		}
	}
	return tutti, sempre
}

// inPari: o ha tutti i campi sempre presenti del tipo, e nessun campo fuori dal tipo salvo quelli dichiarati del banco.
func inPari(t *testing.T, dove string, o m, tp reflect.Type, soloBanco ...string) {
	t.Helper()
	tutti, sempre := campiJSON(tp)
	for k := range sempre {
		if _, ok := o[k]; !ok {
			t.Errorf("%s: manca il campo %q del server (%s)", dove, k, tp.Name())
		}
	}
	extra := []string{}
	for k := range o {
		if !tutti[k] && !contieneTesto(soloBanco, k) {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Errorf("%s: campi che il server non dà (%s): %v", dove, tp.Name(), extra)
	}
}

func contieneTesto(l []string, x string) bool {
	for _, y := range l {
		if y == x {
			return true
		}
	}
	return false
}

// bancoConBozza: il banco con la sorgente confermata da S8 e la bozza aperta dallo STEP (partenza step); il corpo di R4.
func bancoConBozza(t *testing.T) (*bancoDU, string) {
	b := nuovoBancoDU(t)
	b.fonte["confermata"] = true
	b.s8[bdP1] = m{"allegato_id": bdA1, "sha256": bdShaStep, "documento_id": "00000000-0000-4000-8000-0000000f0d01", "valida": true}
	base := "/thread/" + bdThread
	s6 := b.bdChiedi(t, "GET", base+"/sorgente/step/"+bdA1+"/anteprima?prodotto="+bdP1, nil, 200)
	g := s6["gruppi"].([]any)[0].(m)
	b.bdChiedi(t, "POST", base+"/motore-a/bozza", m{"bozza_id": nil, "versione_attesa": 0, "prodotto_id": bdP1, "vocabolario": 2, "chiave_salvataggio": "00000000-0000-4000-8000-0000000e4001",
		"crea": m{"partenza": "step", "sorgente": m{"allegato_id": bdA1, "sha256": bdShaStep, "gruppo": g["gruppo"]}, "impronta_contenuto": g["impronta_contenuto"]}}, 200)
	return b, base
}

func (b *bancoDU) corpoR4(t *testing.T) m {
	t.Helper()
	r1 := b.bdChiedi(t, "GET", "/thread/"+bdThread+"/motore-a/bozza?prodotto="+bdP1, nil, 200)
	c := m{}
	for k, v := range r1["snapshot"].(m) {
		c[k] = v
	}
	c["bozza_id"], c["bozza_versione"] = r1["bozza_id"], r1["versione"]
	return c
}

func codiciDi(l []any) []string {
	out := []string{}
	for _, x := range l {
		out = append(out, bdStr(x.(m)["codice"]))
	}
	return out
}

func TestBancoDistintaUnicaR45InPari(t *testing.T) {
	b, base := bancoConBozza(t)
	bz := b.bozze[bdP1]
	// le PB per le forme dei gruppi: una applicabile, una legacy, una su un file registrato su ACM-1400 (gia_associata_altrove)
	b.mu.Lock()
	k7, k41 := "", ""
	for _, x := range bdElenco(bz.Snapshot["nodi"]) {
		n := x.(m)
		id, _ := n["identita"].(m)
		switch bdStr(id["codice"]) {
		case "ACM-1100":
			k7 = bdStr(n["chiave_bozza"])
		case "ACM-1410":
			k41 = bdStr(n["chiave_bozza"])
		case "":
			id["codice"] = "ACM-1130"
		}
	}
	bz.Snapshot["documenti_preassociati"] = []any{m{"allegato_id": bdA2, "chiave_bozza": k7, "provenienza": "motore", "importazione": bz.Registro},
		m{"allegato_id": bdA11, "chiave_bozza": k41}}
	b.mu.Unlock()
	r4 := b.bdChiedi(t, "POST", base+"/motore-a/bom/riepilogo?avvisi_motore=1", b.corpoR4(t), 200)
	inPari(t, "R4", r4, reflect.TypeOf(distintabozza.RiepilogoR4{}), bdCampiR4SoloBanco...)
	// ingressi nella forma di R1 ?ingressi=1 (blocco «R4 completo» della persistenza)
	inPari(t, "R4 ingressi", r4["ingressi"].(m), reflect.TypeOf(distintabozza.Ingressi{}))
	inPari(t, "R4 ingressi.parti", r4["ingressi"].(m)["parti"].(m), reflect.TypeOf(distintabozza.PartiIngressi{}))
	inPari(t, "R4 identita[]", r4["identita"].([]any)[0].(m), reflect.TypeOf(distintabozza.IdentitaR4{}))
	inPari(t, "R4 effetti_documentali", r4["effetti_documentali"].(m), reflect.TypeOf(distintabozza.EffettiDocumentaliR4{}))
	inPari(t, "R4 sorgente", r4["sorgente"].(m), reflect.TypeOf(distintabozza.SorgenteR4{}))
	inPari(t, "R4 sorgente_legata", r4["sorgente_legata"].(m), reflect.TypeOf(distintabozza.SorgenteLegataR4{}))
	inPari(t, "R4 effetto_sulla_fonte", r4["effetto_sulla_fonte"].(m), reflect.TypeOf(distintabozza.EffettoFonteR4{}))
	for _, x := range r4["documenti_preassociati"].([]any) {
		inPari(t, "R4 documenti_preassociati[]", x.(m), reflect.TypeOf(distintabozza.PreassociazioneR4{}))
	}
	for _, x := range append(r4["blocchi"].([]any), r4["avvisi"].([]any)...) {
		inPari(t, "R4 rilievo "+bdStr(x.(m)["codice"]), x.(m), reflect.TypeOf(distintabozza.Rilievo{}))
	}
	for _, x := range r4["esclusi"].([]any) {
		e := x.(m)
		if e["effetti"] != nil {
			inPari(t, "R4 esclusi[].effetti", e["effetti"].(m), reflect.TypeOf(distintabozza.EffettiEsclusoR4{}))
			delete(e, "effetti")
		}
		inPari(t, "R4 esclusi[]", e, reflect.TypeOf(distintabozza.EsclusoR4{}))
	}
	// i gruppi D-PB-04 e la PB su un file registrato altrove nello stesso prodotto (Prova 7)
	ed := r4["effetti_documentali"].(m)
	if len(ed["motore_intatta"].([]any)) != 1 || len(ed["manuale"].([]any)) != 1 {
		t.Errorf("gruppi D-PB-04: %v", ed)
	}
	var pb11 m
	for _, x := range r4["documenti_preassociati"].([]any) {
		if x.(m)["allegato_id"] == bdA11 {
			pb11 = x.(m)
		}
	}
	if pb11 == nil || pb11["esito"] != associazioni.EsitoInConflitto || pb11["motivo"] != associazioni.MotivoGiaAssociataAltrove || pb11["variante"] != associazioni.VarianteStessoProdotto ||
		pb11["ar_componente_id"] != bdC40 {
		t.Errorf("PB su un file registrato su ACM-1400: %v", pb11)
	}
	if r4["confermabile"] != false || !contieneTesto(codiciDi(r4["blocchi"].([]any)), "PREASSOCIAZIONE_NON_APPLICABILE") {
		t.Errorf("R4 con la PB in conflitto: confermabile %v, blocchi %v", r4["confermabile"], codiciDi(r4["blocchi"].([]any)))
	}
	// lo svincolo della AR di prima nella stessa bozza: sostituzione e uno spostamento di AR
	b.mu.Lock()
	bz.Snapshot["decisioni"] = append(bdElenco(bz.Snapshot["decisioni"]), m{"id": "sv-prova", "tipo": "scollega_documento", "allegato_id": bdA11, "componente_id": bdC40})
	b.mu.Unlock()
	r4 = b.bdChiedi(t, "POST", base+"/motore-a/bom/riepilogo?avvisi_motore=1", b.corpoR4(t), 200)
	sp := r4["effetti_documentali"].(m)["spostamenti_ar"].([]any)
	if len(sp) != 1 || sp[0].(m)["tipo"] != "sostituzione" || sp[0].(m)["da_componente_id"] != bdC40 || r4["confermabile"] != true {
		t.Fatalf("sostituzione con lo svincolo: spostamenti %v, confermabile %v, blocchi %v", sp, r4["confermabile"], codiciDi(r4["blocchi"].([]any)))
	}
	if l := r4["sorgente_legata"].(m); l["confermata"] != true || l["allegato_id"] != bdA1 {
		t.Errorf("riga «legato come sorgente»: %v", l)
	}
	// le forme del blocco «R4 completo» e «R5 completo» (interruttore r4-completo): l'avviso del registro con il valore, il
	// blocco RICODIFICA_FILE_NAS con il motivo, l'avviso e il conflitto del motore, le esclusioni legacy ed ereditata
	b.bdChiedi(t, "POST", "/__banco/interruttore", m{"nome": "r4-completo", "attivo": true}, 204)
	rc := b.bdChiedi(t, "POST", base+"/motore-a/bom/riepilogo?avvisi_motore=1", b.corpoR4(t), 200)
	for _, x := range append(rc["blocchi"].([]any), rc["avvisi"].([]any)...) {
		inPari(t, "R4 completo rilievo "+bdStr(x.(m)["codice"]), x.(m), reflect.TypeOf(distintabozza.Rilievo{}))
	}
	for _, x := range rc["avvisi_motore"].([]any) {
		inPari(t, "R4 avvisi_motore[]", x.(m), reflect.TypeOf(distintabozza.AvvisoMotoreR4{}))
	}
	origini := []string{}
	for _, x := range rc["esclusi"].([]any) {
		e := x.(m)
		origini = append(origini, bdStr(e["origine"]))
		delete(e, "effetti")
		inPari(t, "R4 completo esclusi[]", e, reflect.TypeOf(distintabozza.EsclusoR4{}))
	}
	if !contieneTesto(codiciDi(rc["blocchi"].([]any)), "RICODIFICA_FILE_NAS") || len(rc["avvisi_motore"].([]any)) != 1 || len(rc["conflitti_motore"].([]any)) != 1 ||
		!contieneTesto(origini, "legacy") || !contieneTesto(origini, "ereditata") || rc["confermabile"] != false || rc["firma_anteprima"] == r4["firma_anteprima"] {
		t.Errorf("R4 completo: blocchi %v, avvisi motore %v, conflitti %v, origini %v, confermabile %v", codiciDi(rc["blocchi"].([]any)), rc["avvisi_motore"], rc["conflitti_motore"], origini, rc["confermabile"])
	}
	b.bdChiedi(t, "POST", "/__banco/interruttore", m{"nome": "r4-completo", "attivo": false}, 204)
	// il riepilogo leggero (senza ?avvisi_motore=1) non è confermabile e ha un'altra firma (PA-C01); il motore occupato idem
	leggero := b.bdChiedi(t, "POST", base+"/motore-a/bom/riepilogo", b.corpoR4(t), 200)
	if leggero["confermabile"] != false || len(leggero["blocchi"].([]any)) != 0 || leggero["firma_anteprima"] == r4["firma_anteprima"] {
		t.Errorf("R4 leggero: confermabile %v, blocchi %v", leggero["confermabile"], codiciDi(leggero["blocchi"].([]any)))
	}
	b.bdChiedi(t, "POST", "/__banco/modo", m{"rotta": "motore", "esito": "ko"}, http.StatusNoContent)
	if occ := b.bdChiedi(t, "POST", base+"/motore-a/bom/riepilogo?avvisi_motore=1", b.corpoR4(t), 200); occ["confermabile"] != false || occ["avvisi_motore_errore"] == nil {
		t.Errorf("R4 con il motore occupato: confermabile %v, errore %v", occ["confermabile"], occ["avvisi_motore_errore"])
	}
	// R5: 503 e 409 provocati (niente scritto), poi la conferma con le AR scritte solo qui
	conferma := func(chiave string, atteso int) m {
		c := b.corpoR4(t)
		c["chiave_idempotenza"] = chiave
		return b.bdChiedi(t, "POST", base+"/motore-a/bom/conferma", m{"firma_anteprima": r4["firma_anteprima"], "bozza": c}, atteso)
	}
	b.bdChiedi(t, "POST", "/__banco/modo", m{"rotta": "R5", "esito": "503"}, http.StatusNoContent)
	if r := conferma("00000000-0000-4000-8000-0000000e4002", 503); r["esito"] != "non_registrato" || r["codice"] != "CALCOLO_OCCUPATO" || r["chiave_idempotenza"] == nil {
		t.Errorf("R5 503: %v", r)
	}
	b.bdChiedi(t, "POST", "/__banco/modo", m{"rotta": "R5", "esito": "409_movimento"}, http.StatusNoContent)
	if r := conferma("00000000-0000-4000-8000-0000000e4003", 409); r["codice"] != "ANTEPRIMA_SUPERATA" || r["motivo"] != "ingressi_in_movimento" {
		t.Errorf("R5 409 ingressi_in_movimento: %v", r)
	}
	// il motore nella connessione S di R5 (blocco «R4 completo»): scaduto 504, guasto 500 ERRORE_CALCOLO, occupato 503; tutti
	// non registrati, con la chiave
	b.bdChiedi(t, "POST", "/__banco/modo", m{"rotta": "R5", "esito": "504"}, http.StatusNoContent)
	if r := conferma("00000000-0000-4000-8000-0000000e4005", 504); r["esito"] != "non_registrato" || r["codice"] != distintabozza.CodiceCalcoloScaduto {
		t.Errorf("R5 504: %v", r)
	}
	b.bdChiedi(t, "POST", "/__banco/modo", m{"rotta": "R5", "esito": "500_calcolo"}, http.StatusNoContent)
	if r := conferma("00000000-0000-4000-8000-0000000e4006", 500); r["esito"] != "non_registrato" || r["codice"] != distintabozza.CodiceErroreCalcolo {
		t.Errorf("R5 500 ERRORE_CALCOLO: %v", r)
	}
	b.bdChiedi(t, "POST", "/__banco/modo", m{"rotta": "motore", "esito": "ko"}, http.StatusNoContent)
	if r := conferma("00000000-0000-4000-8000-0000000e4007", 503); r["esito"] != "non_registrato" || r["codice"] != "CALCOLO_OCCUPATO" {
		t.Errorf("R5 con il motore occupato nella connessione S: %v", r)
	}
	// la firma del riepilogo leggero non vale per R5: 409 con il riepilogo con il motore
	{
		c := b.corpoR4(t)
		c["chiave_idempotenza"] = "00000000-0000-4000-8000-0000000e4008"
		if r := b.bdChiedi(t, "POST", base+"/motore-a/bom/conferma", m{"firma_anteprima": leggero["firma_anteprima"], "bozza": c}, 409); r["codice"] != "ANTEPRIMA_SUPERATA" {
			t.Errorf("R5 con la firma del riepilogo leggero: %v", r)
		}
	}
	if f := b.fileDi(bdA2); f.Componente != "" {
		t.Fatalf("una AR prima di R5: %v", f.Componente)
	}
	r5 := conferma("00000000-0000-4000-8000-0000000e4004", 200)
	inPari(t, "R5", r5, reflect.TypeOf(distintabozza.ConfermaR5{}))
	inPari(t, "R5 fonte", r5["fonte"].(m), reflect.TypeOf(distintabozza.FonteR5{}))
	if bdNum(r5["associazioni_applicate"]) != 2 || r5["fonte"].(m)["promossa"] != true || r5["f8_stato"] != "eseguito" {
		t.Errorf("R5: associazioni %v, fonte %v, f8 %v", r5["associazioni_applicate"], r5["fonte"], r5["f8_stato"])
	}
	if b.conti["R5"] != 1 {
		t.Errorf("i rifiuti di R5 hanno scritto: %d conferme", b.conti["R5"])
	}
	if f := b.fileDi(bdA2); f.Componente == "" || f.Componente != bdStr(r5["mappa"].(m)[k7]) {
		t.Errorf("la AR di R5 non è sul componente del nodo: %v (mappa %v)", f.Componente, r5["mappa"].(m)[k7])
	}
	if f := b.fileDi(bdA11); f.Componente != bdStr(r5["mappa"].(m)[k41]) {
		t.Errorf("la sostituzione non ha portato la AR sul nodo nuovo: %v", f.Componente)
	}
}

// TestBancoDistintaUnicaRiconoscimento: R39-15 nel banco come nel server (provenienza3.go, riconcilia3.go): la PB legacy
// riconosciuta con file_riconosciuto resta legacy e diventa verificata (gruppo legacy_verificate, non più in conflitto per
// legacy_da_verificare); il riconoscimento di una PB non legacy è 422 riconoscimento_non_applicabile; una verificata
// dichiarata dalla pagina senza riconoscimento è 422 provenienza_dal_server; spostata, la PB torna manuale.
func TestBancoDistintaUnicaRiconoscimento(t *testing.T) {
	b, base := bancoConBozza(t)
	k7, k41 := "", ""
	for _, x := range bdElenco(b.bozze[bdP1].Snapshot["nodi"]) {
		n := x.(m)
		switch bdStr(n["identita"].(m)["codice"]) {
		case "ACM-1100":
			k7 = bdStr(n["chiave_bozza"])
		case "ACM-1410":
			k41 = bdStr(n["chiave_bozza"])
		}
	}
	b.bdChiedi(t, "POST", "/__banco/provenienza", m{"allegato_id": bdA2, "provenienza": "legacy", "chiave_bozza": k7}, http.StatusNoContent)
	n := 0
	salva := func(cambia func(s m), azioni []any, atteso int) m {
		n++
		r1 := b.bdChiedi(t, "GET", base+"/motore-a/bozza?prodotto="+bdP1, nil, 200)
		s := r1["snapshot"].(m)
		if cambia != nil {
			cambia(s)
		}
		return b.bdChiedi(t, "POST", base+"/motore-a/bozza", m{"bozza_id": r1["bozza_id"], "versione_attesa": r1["versione"], "prodotto_id": bdP1, "vocabolario": 2,
			"chiave_salvataggio": fmt.Sprintf("00000000-0000-4000-8000-0000000e5%03d", n), "snapshot": s, "azioni": azioni}, atteso)
	}
	riconosci := func(aid, k string) []any {
		return []any{m{"azione": "file_riconosciuto", "chiave_bozza": k, "prima": m{"allegato_id": aid, "chiave_bozza": k}, "dopo": m{"allegato_id": aid, "chiave_bozza": k, "motivo": "verificata a mano"}, "origine": "utente"}}
	}
	pbDi := func(aid string) m {
		for _, x := range bdElenco(b.bozze[bdP1].Snapshot["documenti_preassociati"]) {
			if bdStr(x.(m)["allegato_id"]) == aid {
				return x.(m)
			}
		}
		return nil
	}
	// R4 prima: legacy da verificare, in conflitto, con il rimedio del riconoscimento
	r4 := b.bdChiedi(t, "POST", base+"/motore-a/bom/riepilogo?avvisi_motore=1", b.corpoR4(t), 200)
	if l := r4["effetti_documentali"].(m)["legacy_da_verificare"].([]any); len(l) != 1 {
		t.Fatalf("PB legacy prima del riconoscimento: %v", r4["effetti_documentali"])
	}
	// una verificata dichiarata senza il riconoscimento
	tocca := []any{m{"azione": "nodo_modificato", "chiave_bozza": "k-root", "prima": m{"descrizione": nil}, "dopo": m{"descrizione": "rivista"}, "origine": "utente"}}
	if r := salva(func(s m) {
		s["documenti_preassociati"].([]any)[0].(m)["verificata"] = true
		s["nodi"].([]any)[0].(m)["descrizione"] = "rivista"
	}, tocca, 422); r["motivo"] != "provenienza_dal_server" {
		t.Errorf("verificata senza riconoscimento: %v", r)
	}
	// il riconoscimento: la PB resta legacy, diventa verificata; lo snapshot strutturale non cambia
	salva(nil, riconosci(bdA2, k7), 200)
	if pb := pbDi(bdA2); pb == nil || pb["provenienza"] != "legacy" || pb["verificata"] != true || pb["chiave_bozza"] != k7 {
		t.Fatalf("PB dopo il riconoscimento: %v", pb)
	}
	r4 = b.bdChiedi(t, "POST", base+"/motore-a/bom/riepilogo?avvisi_motore=1", b.corpoR4(t), 200)
	ed := r4["effetti_documentali"].(m)
	if len(ed["legacy_verificate"].([]any)) != 1 || len(ed["legacy_da_verificare"].([]any)) != 0 {
		t.Errorf("gruppi dopo il riconoscimento: %v", ed)
	}
	for _, x := range r4["documenti_preassociati"].([]any) {
		if d := x.(m); d["allegato_id"] == bdA2 && (d["motivo"] == "legacy_da_verificare" || d["gruppo"] != "legacy_verificata" || d["verificata"] != true) {
			t.Errorf("PB riconosciuta nel riepilogo: %v", d)
		}
	}
	// un salvataggio dopo che rimanda la verificata letta va bene; il riconoscimento di nuovo di una PB non legacy no
	salva(func(s m) { s["nodi"].([]any)[0].(m)["descrizione"] = "rivista" }, tocca, 200)
	if pb := pbDi(bdA2); pb["verificata"] != true {
		t.Errorf("la verifica è caduta con un salvataggio che non tocca la PB: %v", pb)
	}
	b.bdChiedi(t, "POST", "/__banco/provenienza", m{"allegato_id": bdA2, "provenienza": "manuale"}, http.StatusNoContent)
	if r := salva(nil, riconosci(bdA2, k7), 422); r["motivo"] != "riconoscimento_non_applicabile" {
		t.Errorf("riconoscimento di una PB manuale: %v", r)
	}
	// spostata su un altro nodo (la pagina la rimette come {allegato_id, chiave_bozza}), una PB legacy verificata torna
	// manuale e non più verificata
	b.bdChiedi(t, "POST", "/__banco/provenienza", m{"allegato_id": bdA2, "provenienza": "legacy"}, http.StatusNoContent)
	salva(nil, riconosci(bdA2, k7), 200)
	salva(func(s m) { s["documenti_preassociati"].([]any)[0] = m{"allegato_id": bdA2, "chiave_bozza": k41} },
		[]any{m{"azione": "file_svincolato", "chiave_bozza": k7, "prima": m{"allegato_id": bdA2, "chiave_bozza": k7}, "dopo": m{"allegato_id": bdA2, "motivo": "utente"}, "origine": "utente"},
			m{"azione": "file_preassociato", "chiave_bozza": k41, "prima": nil, "dopo": m{"allegato_id": bdA2, "chiave_bozza": k41}, "origine": "utente"}}, 200)
	if pb := pbDi(bdA2); pb["provenienza"] != "manuale" || pb["verificata"] != nil {
		t.Errorf("PB legacy verificata spostata: %v", pb)
	}
}

func TestBancoDistintaUnicaR45Gate(t *testing.T) {
	b := nuovoBancoDU(t)
	base := "/thread/" + bdThread
	// la bozza senza STEP (partenza vuota, direttiva 39 §1): R4 completo con il blocco firmato, R5 422 BLOCCHI anche con una
	// firma qualunque (il gate prima della firma), niente scritto
	b.bdChiedi(t, "POST", base+"/motore-a/bozza", m{"bozza_id": nil, "versione_attesa": 0, "prodotto_id": bdP1, "vocabolario": 2, "chiave_salvataggio": "00000000-0000-4000-8000-0000000e4101",
		"crea": m{"partenza": "vuota"}}, 200)
	r4 := b.bdChiedi(t, "POST", base+"/motore-a/bom/riepilogo", b.corpoR4(t), 200)
	var gate m
	for _, x := range r4["blocchi"].([]any) {
		if x.(m)["codice"] == distintabozza.BloccoStepSorgenteNonConfermata {
			gate = x.(m)
		}
	}
	if gate == nil || gate["motivo"] != distintabozza.SorgenteMotivoSenzaSorgente || r4["confermabile"] != false || r4["sorgente_legata"] != nil {
		t.Fatalf("R4 senza STEP: gate %v, confermabile %v", gate, r4["confermabile"])
	}
	c := b.corpoR4(t)
	c["chiave_idempotenza"] = "00000000-0000-4000-8000-0000000e4102"
	r := b.bdChiedi(t, "POST", base+"/motore-a/bom/conferma", m{"firma_anteprima": strings.Repeat("0", 64), "bozza": c}, 422)
	if r["codice"] != "BLOCCHI" || !contieneTesto(codiciDi(r["blocchi"].([]any)), distintabozza.BloccoStepSorgenteNonConfermata) {
		t.Errorf("R5 senza sorgente: %v", r)
	}
	if b.conti["R5"] != 0 || len(b.conferme[bdP1]) != 0 {
		t.Errorf("R5 senza sorgente ha scritto: %d conferme", len(b.conferme[bdP1]))
	}
}

// la pagina e il ponte leggono le forme di R4 e R5 formato 3 e le AR dalle righe di S9
func TestDistintaUnicaR45Pagina(t *testing.T) {
	src := duFile(t, duPagina)
	ponte := duFile(t, duPonte)
	dn := duFunzione(t, src, "function documentiNodo(")
	if strings.Contains(dn, "proposta.componente_id") || !strings.Contains(dn, "associazioniRegistrate()") {
		t.Error("documentiNodo legge ancora le AR dalla copia di S1 (DIR35-DP [U]: le AR dalle righe di S9)")
	}
	ar := duFunzione(t, src, "function associazioniRegistrate(")
	for _, v := range []string{`"gia_registrato"`, `"da_scollegare"`, "destinazione.componente_id", `"documento_confermato"`} {
		if !strings.Contains(ar, v) {
			t.Errorf("associazioniRegistrate non legge %s dalle righe di S9", v)
		}
	}
	// senza bozza aperta (dopo «Conferma BOM») la cartella del prodotto mostra anche le AR di S9 (prova sul server vero)
	if !strings.Contains(duFunzione(t, src, "function contenuto("), `x.stato === "gia_registrato"`) {
		t.Error("la cartella del prodotto senza bozza non mostra le righe gia_registrato di S9")
	}
	if strings.Contains(duFunzione(t, src, "function sezioneDocumenti("), "proposta.componente_id") {
		t.Error("i file da smistare escludono le AR dalla copia di S1")
	}
	wz := duFunzione(t, src, "function sezioneDocumentiWizard(")
	for _, v := range []string{"GRUPPI_PB", "proposte_rimosse", "spostamenti_ar", "ar_invariate", "du-wz-cumulativa"} {
		if !strings.Contains(wz, v) {
			t.Errorf("la sezione dei documenti del wizard non ha %s (D-PB-04 A)", v)
		}
	}
	for _, g := range []string{distintabozza.GruppoMotoreIntatta, distintabozza.GruppoCorretta, distintabozza.GruppoManuale, distintabozza.GruppoLegacyDaVerificare} {
		if !strings.Contains(src, `["`+g+`", `) {
			t.Errorf("GRUPPI_PB non ha il gruppo %s del server", g)
		}
	}
	if !strings.Contains(duFunzione(t, src, "function sezioneSorgenteWizard("), "sorgente_legata") || !strings.Contains(src, "function sezioneCondivisione(") {
		t.Error("il wizard non mostra la riga «legato come sorgente» o condivisione[]")
	}
	no := duFunzione(t, src, "function perchéNoConfermaWizard(")
	if !strings.Contains(no, `"STEP_SORGENTE_NON_CONFERMATA"`) || !strings.Contains(no, "conferma uno STEP dalla Sorgente") {
		t.Error("la conferma non si spegne con il rimedio del gate della sorgente")
	}
	ew := duFunzione(t, src, "function esitoWizard(")
	for _, v := range []string{"R5_NON_APPLICABILE", "ingressi_in_movimento", "snapshot_diverso", "du-wz-riprova-occupato", "du-wz-esito-vai-sorgente", "du-wz-rileggi-blocchi"} {
		if !strings.Contains(ew, v) {
			t.Errorf("l'esito di R5 non tratta %s", v)
		}
	}
	if ec := duFunzione(t, src, "async function esitoConferma("); !strings.Contains(ec, `["CALCOLO_OCCUPATO", "CALCOLO_SCADUTO", "ERRORE_CALCOLO"].includes(r.codice)`) {
		t.Error("il 503, il 504 e il 500 ERRORE_CALCOLO di R5 (non registrati) non hanno l'esito da ritentare")
	}
	if !strings.Contains(ew, "du-wz-rileggi-calcolo") || !strings.Contains(src, "ERRORE_CALCOLO: \"") || !strings.Contains(src, "CALCOLO_SCADUTO: \"Il calcolo del motore") {
		t.Error("l'esito di R5 non dice il 504 e il 500 del motore")
	}
	// R5 completo (persistenza): i campi nuovi della risposta 200
	for _, v := range []string{"identita_ricodificate", "correzioni_ricodifica", "identita_ripristinate", "tipi_cambiati", "assenze_applicate"} {
		if !strings.Contains(ew, "d."+v) {
			t.Errorf("l'esito 200 di R5 non legge %s", v)
		}
	}
	// R4 completo (persistenza): il motore nella firma (niente presa d'atto: senza avvisi del motore la conferma è spenta), i
	// conflitti del motore, gli avvisi con il valore, i motivi dei blocchi, gli effetti degli esclusi e le altre origini
	if strings.Contains(src, "presaMotore") || strings.Contains(src, `id: "du-wz-presa",`) {
		t.Error("resta la presa d'atto degli avvisi del motore: dal blocco «R4 completo» il riepilogo senza motore non si conferma")
	}
	if !strings.Contains(no, "w.riepilogo.avvisi_motore_calcolati !== true") {
		t.Error("la conferma non si spegne senza gli avvisi del motore")
	}
	fg := duFunzione(t, src, "function faseGerarchie(")
	for _, v := range []string{"conflitti_motore", "du-wz-conflitti-motore", "a.valore", "data-du-valore", `"data-motivo": b.motivo`, "a.differenze", "tabellaAltreEsclusioni(r)"} {
		if !strings.Contains(fg, v) {
			t.Errorf("la fase 2 del wizard non ha %s", v)
		}
	}
	te := duFunzione(t, src, "function testoEffettiEscluso(")
	for _, v := range []string{"righe_da_chiudere", "archi_da_chiudere", "figli", "fabbisogni_tolti_dopo_conferma"} {
		if !strings.Contains(te, v) {
			t.Errorf("gli effetti degli esclusi non leggono %s", v)
		}
	}
	for _, o := range []string{"assenza", "escludi", "legacy", "ereditata"} {
		if !strings.Contains(src, o+": \"") {
			t.Errorf("le altre esclusioni non hanno l'origine %s", o)
		}
	}
	// gli ingressi di R4 nella forma di R1 ?ingressi=1 (le notifiche leggono le parti)
	if !strings.Contains(duFunzione(t, src, "function partiIngressiR4("), "x.parti") || strings.Contains(src, "w.ingressi = ogg(w.riepilogo.ingressi) ? w.riepilogo.ingressi") {
		t.Error("le notifiche del wizard non leggono le parti degli ingressi di R4")
	}
	// R39-15: il riconoscimento della PB legacy, fuori dalla pila d'annullamento
	if !strings.Contains(src, `["legacy_verificate", `) {
		t.Error("GRUPPI_PB non ha le legacy verificate (legacy_verificate)")
	}
	ric := duFunzione(t, src, "function apriRiconosci(")
	for _, v := range []string{`azione: "` + distintabozza.AzFileRiconosciuto + `"`, "S.pila.pop()", "salvaOra()"} {
		if !strings.Contains(ric, v) {
			t.Errorf("«Riconosci…» non ha %s", v)
		}
	}
	if !strings.Contains(duFunzione(t, src, "function rimediPB("), `d.motivo === "legacy_da_verificare"`) {
		t.Error("la PB legacy da verificare non ha il rimedio «Riconosci…»")
	}
	if !strings.Contains(duFunzione(t, src, "function pilaDalGiornale("), `e.azione === "file_riconosciuto"`) {
		t.Error("la pila ricostruita dal giornale contiene il riconoscimento (che non si annulla)")
	}
	if !strings.Contains(duFunzione(t, ponte, "async function scriviConChiave("), `nome === "confermaBom" && r.stato === 500 && r.json && r.json.esito === "non_registrato" && codice === "ERRORE_CALCOLO"`) {
		t.Error("il ponte tratta il 500 ERRORE_CALCOLO di R5 come il controllo finale")
	}
	cor := duFunzione(t, src, "function apriCorreggi(")
	if !strings.Contains(cor, "fileDiscordanti(") || !strings.Contains(cor, "apriDiscordanza(") {
		t.Error("«Correggi codice» non apre il dialogo di D-ASS-01")
	}
	ad := duFunzione(t, src, "function apriDiscordanza(")
	for _, v := range []string{"Annulla", "Scollega i file", "Mantieni i collegamenti", `tipo: "scollega_documento"`, `azione: "file_svincolato"`} {
		if !strings.Contains(ad, v) {
			t.Errorf("il dialogo di D-ASS-01 non ha %s", v)
		}
	}
	sc := duFunzione(t, ponte, "async function scriviConChiave(")
	if !strings.Contains(sc, `nome === "confermaBom" && r.json && r.json.esito === "non_registrato"`) {
		t.Error("il ponte non tiene la chiave di R5 dopo il 503 non_registrato")
	}
}
