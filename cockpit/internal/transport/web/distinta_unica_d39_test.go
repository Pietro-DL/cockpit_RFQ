package web

// Direttiva 39 (corsia interfaccia): la pagina, il ponte e il banco finto della Distinta unica con S8 unico gesto della
// sorgente, la revoca, le partenze senza STEP (vuota) e dal solo prodotto, la riconciliazione della bozza senza STEP e
// i suggerimenti dopo l'apertura. Le prove statiche dicono che i pezzi ci sono e che quelli superati non ci sono più; le
// prove del banco dicono che le risposte finte hanno le forme delle rotte vere (core/rfq/sorgente, distintabozza,
// motore_a), così che le DU-B nel browser provino la pagina contro il contratto. Non provano il server.

import (
	"fmt"
	"strings"
	"testing"
)

func TestDistintaUnicaDirettiva39(t *testing.T) {
	src := duFile(t, duPagina)
	for _, v := range []string{
		// S8 unico gesto, partenza dal solo prodotto con la presa d'atto, la revoca (senza motivo dal messaggio 46 [U])
		`P.confermaSorgente(c.allegato_id, corpo)`, `partenza: "solo_prodotto", stato_analisi: ss.stato_analisi, presa_d_atto: true`,
		`"Conferma come sorgente e parti dal solo prodotto…"`, `P.revocaSorgente(f.allegato_id, corpo)`, `const corpo = { prodotto_id: id, sorgente_errata: true };`, `"Revoca come sorgente…"`,
		`corpo.bozza = { bozza_id: S.bozza.bozza_id, versione_attesa: S.bozza.versione }`, `codice_atteso: cod || undefined`,
		// la partenza vuota e il blocco della sorgente
		`crea: { partenza: "vuota" }`, `"Parti dal solo prodotto…"`, `"STEP_SORGENTE_NON_CONFERMATA"`,
		// «Usa nuovo STEP» e «Aggiorna proposta» (due comandi nel backend)
		`"Usa nuovo STEP per "`, `"Aggiorna proposta…"`,
		// la riconciliazione e i suggerimenti
		`e.dopo && e.dopo.abbinamenti_proposti`, `azione: "nodo_modificato", chiave_bozza: n.chiave_bozza, prima: { caselle: null, identita: clona(n.identita) }`,
		`p && p.preassociazioni_proposte`, `"Tieni separati"`,
		// i codici nuovi
		`"RICONCILIAZIONE_NON_DISPONIBILE"`, `r.codice === "FONTE_NON_CONFERMATA"`, `GIA_DOCUMENTO:`,
		// la sorgente confermata da S8, distinta dal documento legacy e dal legame storico
		`fo.sorgente_s8`, `fo.legame_storico`, `legato_come_sorgente:`,
	} {
		if !strings.Contains(src, v) {
			t.Errorf("la pagina non ha %s (direttiva 39)", v)
		}
	}
	for _, v := range []string{"apriAutorizza", `atto("autorizza"`, `atto("revoca"`, "P.anteprimaAutorizzazione", "Autorizza lo STEP strutturale", "Revoca l'autorizzazione…",
		"prima revoca l'autorizzazione", "Cambia la sorgente di ", "Aggiorna alla proposta di oggi…"} {
		if strings.Contains(src, v) {
			t.Errorf("la pagina ha ancora %q (superato dalla direttiva 39)", v)
		}
	}
	// il corpo di S8: codice_atteso è il codice del prodotto, niente componente_atteso (il server lo vuole uguale al prodotto)
	if s8 := duFunzione(t, src, "async function apriConfermaSorgente("); strings.Contains(s8, "componente_atteso") || strings.Contains(s8, "f.proposta.codice") {
		t.Error("il corpo di S8 porta ancora il codice o il componente della proposta del file")
	}
	ponte := duFile(t, duPonte)
	for _, v := range []string{`const STATI_ANALISI_S8 = ["pronta", "in_analisi", "errore", "non_valutato"];`, `if (c.presa_d_atto !== true) return no(`,
		`scriviConChiave("revocaSorgente", "sorgente:" + c.prodotto_id.toLowerCase(), allegato, c, "Revoca della sorgente")`} {
		if !strings.Contains(ponte, v) {
			t.Errorf("il ponte non ha %s (direttiva 39)", v)
		}
	}
	if strings.Contains(ponte, "step-strutturale") {
		t.Error("il ponte chiama ancora le rotte dell'autorizzazione (direttiva 39 §2)")
	}
}

// Il banco con la direttiva 39: le forme che la pagina legge, contro quelle delle rotte vere.
func TestBancoDistintaUnicaDirettiva39(t *testing.T) {
	b := nuovoBancoDU(t)
	base := "/thread/" + bdThread
	k := func(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-0000000e9%03d", n) }
	// /dati.fonte: S8 è il gesto, nessuna sorgente confermata, struttura_step dello STEP del prodotto
	d := b.bdChiedi(t, "GET", base+"/motore-a/dati", nil, 200)
	fo := d["fonte"].(m)
	bdHaCampi(t, "/dati.fonte", fo, "gesto", "sorgente_confermata", "struttura_step", "conferma_ammessa")
	if fo["sorgente_confermata"] != false || fo["conferma_ammessa"] != true || fo["struttura_step"].(m)["struttura"] != "con_struttura" {
		t.Errorf("/dati.fonte all'inizio: %v", fo)
	}
	// S6 per il secondo prodotto: nessuna struttura importabile, la partenza dal solo prodotto
	s6 := b.bdChiedi(t, "GET", base+"/sorgente/step/"+bdA1+"/anteprima?prodotto="+bdP2, nil, 200)
	if ss := s6["struttura_step"].(m); s6["stato"] != "pronta" || ss["struttura"] != "nessuna_struttura" || ss["partenza"] != "solo_prodotto" {
		t.Errorf("S6 per il secondo prodotto: %v", s6)
	}
	// R2 crea vuota (direttiva 39 §1): la sola radice, nessuna sorgente
	r2 := b.bdChiedi(t, "POST", base+"/motore-a/bozza", m{"bozza_id": nil, "versione_attesa": 0, "prodotto_id": bdP1, "vocabolario": 2, "chiave_salvataggio": k(1), "crea": m{"partenza": "vuota"}}, 200)
	snap := r2["snapshot"].(m)
	if snap["sorgente"] != nil || len(snap["nodi"].([]any)) != 1 || r2["importazione"] != nil {
		t.Errorf("R2 vuota: %v", r2)
	}
	bozza := bdStr(r2["bozza_id"])
	// un nodo manuale con lo stesso codice e rev della voce #7 (ACM-1100 A), uno con la rev diversa (ACM-1120 B), un arco
	nodi := append(snap["nodi"].([]any), m{"chiave_bozza": "k-m1", "op": "crea", "tipo": "sottoassieme", "origine": "manuale", "identita": m{"emittente": "cliente:" + bdCliente, "codice": "ACM-1100", "rev": "A", "modo": "manuale"}},
		m{"chiave_bozza": "k-m2", "op": "crea", "tipo": "sciolto", "origine": "manuale", "identita": m{"emittente": "cliente:" + bdCliente, "codice": "ACM-1120", "rev": "B", "modo": "manuale"}})
	snap2 := m{}
	for kk, v := range snap {
		snap2[kk] = v
	}
	snap2["nodi"] = nodi
	snap2["archi"] = []any{m{"padre": "k-root", "figlio": "k-m1", "qta": 3, "origine": "manuale"}}
	b.bdChiedi(t, "POST", base+"/motore-a/bozza", m{"bozza_id": bozza, "versione_attesa": 1, "prodotto_id": bdP1, "vocabolario": 2, "chiave_salvataggio": k(2), "snapshot": snap2,
		"azioni": []any{m{"azione": "nodo_aggiunto", "chiave_bozza": "k-m1"}}}, 200)
	// S9 senza S8: nessuna riga del motore, i file in attesa con il motivo (D-AR11 B)
	s9 := b.bdChiedi(t, "GET", base+"/motore-a/dati?sorgente=1", nil, 200)
	p1 := s9["prodotti"].([]any)[0].(m)
	for _, x := range p1["righe"].([]any) {
		if x.(m)["origine"] == "motore" {
			t.Errorf("S9 senza S8 ha una riga del motore: %v", x)
		}
	}
	if att := p1["in_attesa_sorgente"].([]any); len(att) == 0 || att[0].(m)["motivo"] != "sorgente_non_confermata" {
		t.Errorf("S9 senza S8: in_attesa_sorgente %v", att)
	}
	// R4 con il gate acceso: il blocco della sorgente
	b.bdChiedi(t, "POST", "/__banco/interruttore", m{"nome": "gate-sorgente", "attivo": true}, 204)
	r1 := b.bdChiedi(t, "GET", base+"/motore-a/bozza?prodotto="+bdP1, nil, 200)
	corpo := m{}
	for kk, v := range r1["snapshot"].(m) {
		corpo[kk] = v
	}
	corpo["bozza_id"], corpo["bozza_versione"] = bozza, r1["versione"]
	r4 := b.bdChiedi(t, "POST", base+"/motore-a/bom/riepilogo", corpo, 200)
	trovato := false
	for _, x := range r4["blocchi"].([]any) {
		if x.(m)["codice"] == "STEP_SORGENTE_NON_CONFERMATA" && x.(m)["motivo"] == "senza_sorgente" {
			trovato = true
		}
	}
	if !trovato || r4["confermabile"] != false {
		t.Errorf("R4 senza sorgente con il gate: %v", r4["blocchi"])
	}
	// S8 sulla bozza manuale senza dichiararla: 409 BOZZA_VERSIONE con il conflitto
	reg := b.registro(bdA1, bdShaStep, b.versioneProposta)
	s8 := m{"chiave_idempotenza": k(3), "prodotto_id": bdP1, "gruppo": reg["gruppo"].(m)["chiave"], "impronta_contenuto": reg["impronta_contenuto"], "codice_atteso": "ACM-1000", "motivo": "arriva lo STEP"}
	if e := b.bdChiedi(t, "POST", base+"/sorgente/step/"+bdA1+"/conferma-sorgente", s8, 409); e["codice"] != "BOZZA_VERSIONE" || e["conflitto"] == nil {
		t.Errorf("S8 sulla bozza manuale senza bozza: %v", e)
	}
	// con la bozza: la stessa bozza riconciliata, un abbinamento certo, uno proposto, nessun doppione
	s8["chiave_idempotenza"] = k(4)
	s8["bozza"] = m{"bozza_id": bozza, "versione_attesa": 2}
	es := b.bdChiedi(t, "POST", base+"/sorgente/step/"+bdA1+"/conferma-sorgente", s8, 200)
	if es["esito"] != "completata" || es["bozza"].(m)["bozza_id"] != bozza {
		t.Errorf("S8 riconcilia: %v", es)
	}
	if p := es["passi"].([]any); p[0].(m)["esito"] != "non_serve" {
		t.Errorf("S8: il passo dell'associazione non è «non_serve»: %v", p)
	}
	r1 = b.bdChiedi(t, "GET", base+"/motore-a/bozza?prodotto="+bdP1+"&eventi=1", nil, 200)
	codici := map[string]int{}
	for _, x := range r1["snapshot"].(m)["nodi"].([]any) {
		if id, _ := x.(m)["identita"].(m); id != nil {
			codici[bdStr(id["codice"])]++
		}
	}
	if codici["ACM-1100"] != 1 || codici["ACM-1120"] != 1 {
		t.Errorf("riconciliazione: doppioni %v", codici)
	}
	ev := r1["eventi"].([]any)
	ult := ev[len(ev)-1].(m)
	ab, _ := ult["dopo"].(m)["abbinamenti_proposti"].([]any)
	if ult["azione"] != "sorgente_cambiata" || ult["dopo"].(m)["algoritmo"] != "cambio-fonte-2" || len(ab) != 1 || ab[0].(m)["chiave_bozza"] != "k-m2" || ab[0].(m)["motivo"] != "incompatibile" {
		t.Errorf("evento della riconciliazione: %v", ult)
	}
	// la fonte: S8 valida, nessuna autorizzazione (la working non cambia); S9 con lo STEP legato come sorgente e i
	// suggerimenti dopo l'apertura
	fo = b.bdChiedi(t, "GET", base+"/motore-a/dati", nil, 200)["fonte"].(m)
	if fo["sorgente_confermata"] != true || fo["autorizzata"] != false || fo["sorgente_s8"].(m)["valida"] != true {
		t.Errorf("/dati.fonte dopo S8: %v", fo)
	}
	p1 = b.bdChiedi(t, "GET", base+"/motore-a/dati?sorgente=1", nil, 200)["prodotti"].([]any)[0].(m)
	legato := false
	for _, x := range p1["righe"].([]any) {
		if x.(m)["stato"] == "legato_come_sorgente" && x.(m)["allegato_id"] == bdA1 {
			legato = true
		}
	}
	if !legato || len(p1["preassociazioni_proposte"].([]any)) == 0 || p1["bozza"].(m)["sorgente"] != bdA1 {
		t.Errorf("S9 dopo S8: legato %v, suggerimenti %v, bozza %v", legato, p1["preassociazioni_proposte"], p1["bozza"])
	}
	// la revoca senza motivo (messaggio 46 [U]: il motivo lo scrive il server), poi la sorgente non vale più; una seconda
	// revoca non ha niente da revocare
	rv := b.bdChiedi(t, "POST", base+"/sorgente/step/"+bdA1+"/revoca-sorgente", m{"chiave_idempotenza": k(6), "prodotto_id": bdP1}, 200)
	bdHaCampi(t, "revoca", rv, "esito", "riproposto", "chiave_idempotenza", "prodotto_id", "allegato_id", "documento_id", "testo")
	if !strings.HasPrefix(bdStr(rv["motivo"]), "gesto di PS da «Revoca come sorgente»: ") {
		t.Errorf("il motivo della revoca senza il motivo dell'utente: %v", rv["motivo"])
	}
	if e := b.bdChiedi(t, "POST", base+"/sorgente/step/"+bdA1+"/revoca-sorgente", m{"chiave_idempotenza": k(7), "prodotto_id": bdP1, "motivo": "di nuovo"}, 422); e["motivo"] != "nessuna_sorgente_da_revocare" {
		t.Errorf("seconda revoca: %v", e)
	}
	if r6 := b.bdChiedi(t, "GET", base+"/motore-a/intento/"+k(6), nil, 200); r6["stato"] != "registrato" || r6["gesto"] != "revoca_sorgente" {
		t.Errorf("R6 della revoca: %v", r6)
	}
	fo = b.bdChiedi(t, "GET", base+"/motore-a/dati", nil, 200)["fonte"].(m)
	if fo["sorgente_confermata"] != false || fo["sorgente_s8"].(m)["motivo"] != "revocata" {
		t.Errorf("/dati.fonte dopo la revoca: %v", fo)
	}
	// solo_prodotto per il secondo prodotto: presa d'atto obbligatoria, stato dell'analisi confrontato, motivo
	sp := m{"chiave_idempotenza": k(8), "prodotto_id": bdP2, "partenza": "solo_prodotto", "stato_analisi": "pronta", "motivo": "STEP senza struttura"}
	if e := b.bdChiedi(t, "POST", base+"/sorgente/step/"+bdA10+"/conferma-sorgente", sp, 422); e["motivo"] != "presa_d_atto_mancante" {
		t.Errorf("solo_prodotto senza presa d'atto: %v", e)
	}
	sp["chiave_idempotenza"], sp["presa_d_atto"], sp["stato_analisi"] = k(9), true, "in_analisi"
	if e := b.bdChiedi(t, "POST", base+"/sorgente/step/"+bdA10+"/conferma-sorgente", sp, 409); e["anteprima"] != "stato_analisi" {
		t.Errorf("solo_prodotto con lo stato cambiato: %v", e)
	}
	sp["chiave_idempotenza"], sp["stato_analisi"] = k(10), "pronta"
	es = b.bdChiedi(t, "POST", base+"/sorgente/step/"+bdA10+"/conferma-sorgente", sp, 200)
	r1 = b.bdChiedi(t, "GET", base+"/motore-a/bozza?prodotto="+bdP2, nil, 200)
	if s := r1["snapshot"].(m)["sorgente"].(m); es["esito"] != "completata" || s["gruppo"] != "solo_prodotto" || s["allegato_id"] != bdA10 || r1["importazione"] != nil {
		t.Errorf("solo_prodotto: %v %v", es, r1["snapshot"])
	}
	// vuota rifiutata con una sorgente S8 valida (R39-19)
	b.bdChiedi(t, "POST", base+"/motore-a/bozza/"+bdStr(r1["bozza_id"])+"/abbandona", m{"versione_attesa": r1["versione"]}, 200)
	if e := b.bdChiedi(t, "POST", base+"/motore-a/bozza", m{"bozza_id": nil, "versione_attesa": 0, "prodotto_id": bdP2, "vocabolario": 2, "chiave_salvataggio": k(11), "crea": m{"partenza": "vuota"}}, 422); e["motivo"] != "partenza_non_ammessa" {
		t.Errorf("vuota con la sorgente S8: %v", e)
	}
}

// I rilievi del revisore del 20/10 sulla pagina: il riuso di un componente registrato (CREA_SU_ESISTENTE e la voce con
// decisione_esistente, decisione riusa_esistente scritta con R2) e le differenze di archi e nodi con lo STEP dopo la
// riconciliazione («Recepisci» con un gesto normale, «Mantieni» senza scrittura). S7 è provata da
// TestPonteDistintaUnicaRotteNelServer. Il browser (DU-B62, DU-B67) prova i gesti; qui i pezzi ci sono.
func TestDistintaUnicaRilieviDel20Ottobre(t *testing.T) {
	src := duFile(t, duPagina)
	riuso := duFunzione(t, src, "function azioniRiuso(")
	if !strings.Contains(riuso, `tipo: "riusa_esistente", chiave_bozza: k, componente_id: String(cid).toLowerCase()`) || !strings.Contains(riuso, `azione: "decisione_aggiunta"`) {
		t.Error("«Riusa ‹C›» non scrive la decisione riusa_esistente {chiave_bozza, componente_id} con R2")
	}
	for _, v := range []string{`gestiD1(b), gestoRiuso(b)`, `riusoInBlocco(blocchi)`, `"Riusa i componenti con lo stesso codice ("`, `const riuso = riusoNelPannello(g, v);`,
		`v.voce.decisione_esistente`, `"Togli il riuso"`} {
		if !strings.Contains(src, v) {
			t.Errorf("la pagina non ha %s (riuso, rilievo del 20/10)", v)
		}
	}
	diff := duFunzione(t, src, "function differenzeRiconciliazione(")
	for _, v := range []string{`u.e.dopo.differenze`, `reg.proposta && reg.proposta.archi`, `daArco("cambiato"`, `daArco("tolto"`, `daArco("aggiunto"`, `tipo_proposto`} {
		if !strings.Contains(diff, v) {
			t.Errorf("le differenze con lo STEP non hanno %s (rilievo del 20/10)", v)
		}
	}
	rec := duFunzione(t, src, "function azioniRecepisci(")
	for _, v := range []string{`azione: "qta_modificata"`, `azione: "arco_aggiunto"`, `origine: "step", step: d.step`, `azione: "arco_tolto"`, `tipo: "togli_arco"`} {
		if !strings.Contains(rec, v) {
			t.Errorf("«Recepisci» non ha %s", v)
		}
	}
	if tieni := duFunzione(t, src, "function tieniDifferenza("); strings.Contains(tieni, "fai(") || strings.Contains(tieni, "P.") {
		t.Error("«Mantieni» scrive: deve lasciare la bozza com'è")
	}
	for _, v := range []string{`riquadroAbbinamenti(), riquadroDifferenze(), strumentiOrg(g)`, `riquadroAbbinamenti(), riquadroDifferenze(), vassoio()`, `" Differenze con lo STEP da vedere nella Revisione: "`} {
		if !strings.Contains(src, v) {
			t.Errorf("la pagina non mostra le differenze: manca %s", v)
		}
	}
	for _, v := range []string{"apriAssocia(", "function apriAssocia(", "P.associaStep", `btn("Associa lo STEP a "`, `"Associa lo STEP a " + p.codice`} {
		if strings.Contains(src, v) {
			t.Errorf("la pagina ha ancora %q (S7, che il server non registra)", v)
		}
	}
}
