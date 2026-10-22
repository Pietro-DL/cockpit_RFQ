package web

// Il giro G39 della V1 (decisioni dell'utente del 20/10 sera), corsia interfaccia, G39-02 [U]: lo STEP del prodotto
// finito è un candidato sorgente, riconosciuto con il Motore A, proposto nella pagina prima di S8 e registrato solo con
// S8; mai PB né AR ordinaria. Dopo la revisione G39 (rilievi bloccanti 1 e 2) la pagina legge la forma del server della
// corsia motore, fonte.proposte_sorgente[] (motore_a/proposta_sorgente.go), senza ripieghi propri, e il ponte conosce il
// 422 STEP_CANDIDATO_DEL_FINITO di R2 (distintabozza/stepcandidati3.go). Le prove qui:
//   - TestBancoDistintaUnicaG39FormaDelServer: il banco finto dà la proposta e il rifiuto con i nomi del server (le
//     etichette JSON di motore_a.PropostaSorgente e di propostabom.EvidenzaCandidatura, il codice di R2, il testo): la
//     pagina che il browser prova (DU-B68) legge la forma vera, non una inventata;
//   - TestBancoDistintaUnicaG39StepProposto: il banco, rotta per rotta (S9, /dati, R2, S8);
//   - TestDistintaUnicaG39StepProposto: nella pagina non resta la forma inventata né il ripiego.
// Il comportamento della pagina lo provano DU-B68 (banco) e TestDistintaUnicaServerVeroNelBrowser (server vero).

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// etichetteJSON: i nomi JSON dei campi di un tipo struct (senza «-»).
func etichetteJSON(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumField(); i++ {
		n := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if n != "" && n != "-" {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func chiaviDelBanco(o m) []string {
	var out []string
	for k := range o {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// La forma della proposta e del rifiuto nel banco è quella del server (fallisce sul banco di 858f799, che dava
// prodotti[].step_proposto con le evidenze come oggetto e il 422 CORPO_NON_VALIDO con un motivo inventato).
func TestBancoDistintaUnicaG39FormaDelServer(t *testing.T) {
	b := nuovoBancoDU(t)
	b.stepProposto = true
	b.s8[bdP2] = m{"allegato_id": bdA1, "sha256": bdShaStep, "documento_id": "00000000-0000-4000-8000-0000000f0d02", "valida": true}
	pp := b.proposteSorgenteBanco(bdP1)
	if len(pp) != 1 {
		t.Fatalf("proposte del banco: %v", pp)
	}
	x := pp[0].(m)
	// il banco riempie tutti i campi del server, compreso sorgente_di (qui A1 è la sorgente S8 valida del secondo prodotto)
	if got, want := chiaviDelBanco(x), etichetteJSON(reflect.TypeOf(motorea_web.PropostaSorgente{})); !reflect.DeepEqual(got, want) {
		t.Errorf("campi della proposta del banco %v, del server %v", got, want)
	}
	server := map[string]bool{}
	for _, k := range etichetteJSON(reflect.TypeOf(propostabom.EvidenzaCandidatura{})) {
		server[k] = true
	}
	for _, e := range x["evidenze"].([]any) {
		for k := range e.(m) {
			if !server[k] {
				t.Errorf("l'evidenza del banco ha %q, che il server non ha (%v)", k, server)
			}
		}
	}
	// la proposta del banco si rilegge nel tipo del server senza campi persi
	raw, _ := json.Marshal(x)
	var ps motorea_web.PropostaSorgente
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ps); err != nil || ps.SorgenteDi == nil || len(ps.Evidenze) != 3 || ps.Evidenze[1].MarcatoreLetto != "Z" {
		t.Errorf("la proposta del banco non è un motore_a.PropostaSorgente: %v (%+v)", err, ps)
	}
	if TestoPropostaSorgenteBanco != motorea_web.TestoPropostaSorgente || ps.Testo != fmt.Sprintf(motorea_web.TestoPropostaSorgente, "ACM-1000") {
		t.Errorf("testo della proposta: banco %q, server %q", TestoPropostaSorgenteBanco, motorea_web.TestoPropostaSorgente)
	}
	if CodiceStepCandidatoDelFinito != distintabozza.CodiceStepCandidatoDelFinito {
		t.Errorf("codice del rifiuto di R2: banco %q, server %q", CodiceStepCandidatoDelFinito, distintabozza.CodiceStepCandidatoDelFinito)
	}
}

func TestDistintaUnicaG39StepProposto(t *testing.T) {
	src := duFile(t, duPagina)
	// la forma inventata dalla corsia e il ripiego non ci sono più (rilievo bloccante 2 della revisione G39)
	for _, v := range []string{"step_proposto", "il server non ha ancora mandato la proposta", `"data-dal-server"`, "ETICHETTE_EVIDENZA"} {
		if strings.Contains(src, v) {
			t.Errorf("la pagina ha ancora %q (forma inventata o ripiego)", v)
		}
	}
	ps := duFunzione(t, src, "function proposteSorgente(")
	for _, v := range []string{"fo.proposte_sorgente", "x.sorgente_di", "x.evidenze", "x.testo", "x.codice_prodotto", "sorgenteValida(id)"} {
		if !strings.Contains(ps, v) {
			t.Errorf("proposteSorgente non legge %s", v)
		}
	}
	if strings.Contains(ps, "candidatePer(") || strings.Contains(ps, "compatibile") {
		t.Error("proposteSorgente calcola ancora una proposta della pagina dalle candidate")
	}
	ev := duFunzione(t, src, "function evidenzeProposta(")
	for _, v := range []string{"e.codice_letto", "e.marcatore_letto", "e.esito_marcatore", "e.selettore"} {
		if !strings.Contains(ev, v) {
			t.Errorf("le evidenze non mostrano %s", v)
		}
	}
	// la chiave interna del nodo di una struttura candidata non si mostra (trovato sul server vero)
	if !strings.Contains(ev, `e.codice_letto.indexOf(":") < 0`) {
		t.Error("le evidenze mostrano la chiave interna del nodo della struttura candidata")
	}
	if fin := duFunzione(t, src, "function stepSorgenteDelFinito("); !strings.Contains(fin, "p.fonte.proposte_sorgente") {
		t.Error("lo STEP proposto come sorgente resta fra i file da preassociare")
	}
	if !strings.Contains(src, "TESTI_R2_MOTIVO[r.motivo] || TESTI_R2_CODICE[r.codice] || r.testo") || !strings.Contains(src, "STEP_CANDIDATO_DEL_FINITO: ") {
		t.Error("il 422 STEP_CANDIDATO_DEL_FINITO di R2 non ha il testo della pagina")
	}
}

// N4 con il nome di chi ha salvato (difetto trovato dalla prova d1 il 20/10): R1 ?ingressi=1 dà aggiornata_da come
// utente_id, e la pagina mostrava l'identificativo. Ora lo risolve fra le persone che conosce, o dice «un altro utente».
func TestDistintaUnicaN4SenzaIdentificativo(t *testing.T) {
	src := duFile(t, duPagina)
	if strings.Contains(src, `(ogg(da) ? da.nome || da.utente_id : da)`) {
		t.Error("N4 mostra ancora aggiornata_da così com'è (l'utente_id)")
	}
	chi := duFunzione(t, src, "function chiHaSalvato(")
	for _, v := range []string{`"un altro utente"`, `S.utente`, `fo.confermata_da`, `c.confermato_da_nome`} {
		if !strings.Contains(chi, v) {
			t.Errorf("chiHaSalvato non ha %s", v)
		}
	}
	if !strings.Contains(src, `w.n4 = { utente: chiHaSalvato(pn.bozza.aggiornata_da)`) {
		t.Error("N4 non usa chiHaSalvato")
	}
	b := nuovoBancoDU(t)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bozze[bdP1] = &bozzaBanco{ID: "bz", Stato: "aperta", Versione: 3}
	b.altrove[bdP1] = 3
	parti, _ := b.partiIngressi(bdP1)
	if da, ok := parti["bozza"].(m)["aggiornata_da"].(string); !ok || da == "" {
		t.Errorf("il banco dà aggiornata_da come il server (un utente_id): %v", parti["bozza"])
	}
}

// Il banco con il giro G39: S9 e /dati fonte.proposte_sorgente[] e il 422 STEP_CANDIDATO_DEL_FINITO di R2.
func TestBancoDistintaUnicaG39StepProposto(t *testing.T) {
	b := nuovoBancoDU(t)
	base := "/thread/" + bdThread
	k := func(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-0000000e8%03d", n) }
	prodotto1 := func() m {
		t.Helper()
		for _, x := range b.bdChiedi(t, "GET", base+"/motore-a/dati?sorgente=1", nil, 200)["prodotti"].([]any) {
			if x.(m)["prodotto_id"] == bdP1 {
				return x.(m)
			}
		}
		t.Fatal("S9 senza il primo prodotto")
		return nil
	}
	proposte := func() []any {
		t.Helper()
		fo, _ := prodotto1()["fonte"].(m)
		pp, ok := fo["proposte_sorgente"].([]any)
		if !ok {
			t.Fatalf("S9 fonte senza proposte_sorgente (sempre un elenco): %v", fo)
		}
		return pp
	}
	// spento: un elenco vuoto
	if pp := proposte(); len(pp) != 0 {
		t.Errorf("S9 dà proposte con l'interruttore spento: %v", pp)
	}
	b.bdChiedi(t, "POST", "/__banco/interruttore", m{"nome": "step-proposto", "attivo": true}, 204)
	pp := proposte()
	if len(pp) != 1 || pp[0].(m)["allegato_id"] != bdA1 {
		t.Fatalf("S9 con l'interruttore acceso: %v", pp)
	}
	bdHaCampi(t, "proposte_sorgente[0]", pp[0].(m), "allegato_id", "nome_file", "codice_prodotto", "evidenze", "testo")
	if _, c := pp[0].(m)["sorgente_di"]; c {
		t.Errorf("sorgente_di senza una sorgente S8 dell'altro prodotto: %v", pp[0])
	}
	// /dati del prodotto: la stessa proposta nella fonte
	d := b.bdChiedi(t, "GET", base+"/motore-a/dati?prodotto=prodotto:ACM-1000", nil, 200)
	if fo, _ := d["fonte"].(m); fo == nil || len(fo["proposte_sorgente"].([]any)) != 1 {
		t.Errorf("/dati fonte senza la proposta: %v", d["fonte"])
	}
	// R2 crea vuota (la bozza manuale), poi la preassociazione dello STEP proposto: 422 STEP_CANDIDATO_DEL_FINITO
	r2 := b.bdChiedi(t, "POST", base+"/motore-a/bozza", m{"bozza_id": nil, "versione_attesa": 0, "prodotto_id": bdP1, "vocabolario": 2, "chiave_salvataggio": k(1), "crea": m{"partenza": "vuota"}}, 200)
	snap := m{}
	for kk, v := range r2["snapshot"].(m) {
		snap[kk] = v
	}
	snap["documenti_preassociati"] = []any{m{"allegato_id": bdA1, "chiave_bozza": "k-root"}}
	e := b.bdChiedi(t, "POST", base+"/motore-a/bozza", m{"bozza_id": r2["bozza_id"], "versione_attesa": 1, "prodotto_id": bdP1, "vocabolario": 2, "chiave_salvataggio": k(2), "snapshot": snap,
		"azioni": []any{m{"azione": "file_preassociato", "chiave_bozza": "k-root", "dopo": m{"allegato_id": bdA1, "chiave_bozza": "k-root"}}}}, 422)
	if e["codice"] != CodiceStepCandidatoDelFinito {
		t.Errorf("R2 con la PB dello STEP proposto: %v", e)
	}
	// dopo S8 lo STEP è la sorgente: niente più proposta
	reg := b.registro(bdA1, bdShaStep, b.versioneProposta)
	s8 := m{"chiave_idempotenza": k(3), "prodotto_id": bdP1, "gruppo": reg["gruppo"].(m)["chiave"], "impronta_contenuto": reg["impronta_contenuto"], "codice_atteso": "ACM-1000",
		"motivo": "lo STEP proposto", "bozza": m{"bozza_id": r2["bozza_id"], "versione_attesa": 1}}
	b.bdChiedi(t, "POST", base+"/sorgente/step/"+bdA1+"/conferma-sorgente", s8, 200)
	if pp := proposte(); len(pp) != 0 {
		t.Errorf("S9 dà ancora proposte dopo S8: %v", pp)
	}
}

// G39-03 [U] nella pagina (rilievo bloccante 1 della revisione G39): lo spostamento della sorgente, la revoca con la causa
// e la bozza, i rimedi dei rifiuti e il segno della correzione. Il comportamento lo prova DU-B69
// (TestDistintaUnicaG39NelBrowser); qui i pezzi che il browser non attraversa (la riassociazione di un documento, R5 e R7).
func TestDistintaUnicaG39SpostaLaSorgente(t *testing.T) {
	src := duFile(t, duPagina)
	cand := duFunzione(t, src, "function azioniCandidatura(")
	if !strings.Contains(cand, `"Sposta la sorgente su " + cod + "…"`) || !strings.Contains(cand, "apriConfermaSorgente(c, { spostaDa: altro })") || strings.Contains(cand, "è già la sorgente di") {
		t.Error("la candidatura con sorgente_di non offre «Sposta la sorgente su ‹P›…» (o la spegne ancora)")
	}
	s8 := duFunzione(t, src, "async function apriConfermaSorgente(")
	for _, v := range []string{"if (sposta) corpo.sposta_da = sposta;", "corpo.correzione_sorgente = true", `id: "f-correzione"`, `"f-sposta"`} {
		if !strings.Contains(s8, v) {
			t.Errorf("il dialogo di S8 non ha %s", v)
		}
	}
	es := duFunzione(t, src, "async function esitoS8(")
	for _, v := range []string{`"sorgente_spostabile", "bom_confermata", "riassociazione"`, "r.dati.altro_prodotto", `"Apri la revisione di " + codA`, `vaiPasso("revisione")`,
		`riassocia: r.motivo === "riassociazione"`, "d.spostamento.testo"} {
		if !strings.Contains(es, v) {
			t.Errorf("l'esito di S8 non ha %s", v)
		}
	}
	rv := duFunzione(t, src, "async function apriRevocaSorgente(")
	for _, v := range []string{"sorgente_errata: true", "corpo.bozza = { bozza_id: S.bozza.bozza_id, versione_attesa: S.bozza.versione }", "la revoca annulla la bozza"} {
		if !strings.Contains(rv, v) {
			t.Errorf("la revoca non ha %s", v)
		}
	}
	if strings.Contains(rv, "La revoca non cambia la BOM confermata, la bozza") {
		t.Error("il dialogo della revoca dice ancora che la bozza non cambia (contrario a G39-03 [U])")
	}
	if !strings.Contains(duFunzione(t, src, "async function esitoRevocaSorgente("), `r.codice === "BOZZA_VERSIONE"`) {
		t.Error("la revoca non tratta il 409 BOZZA_VERSIONE della bozza dichiarata")
	}
	for _, v := range []string{`id: "du-wz-correzione-sorgente"`, `id: "du-wz-esito-correzione"`, `id: "du-conferma-correzione"`, "e.correzione_sorgente", "fo.correzione_sorgente", "ultima.correzione_sorgente"} {
		if !strings.Contains(src, v) {
			t.Errorf("il segno della correzione della sorgente non ha %s", v)
		}
	}
	ponte := duFile(t, duPonte)
	for _, v := range []string{`revocaSorgente: ["CHIAVE_RIUSATA", "BOZZA_VERSIONE"]`, "c.sposta_da", "c.sorgente_errata", "c.correzione_sorgente"} {
		if !strings.Contains(ponte, v) {
			t.Errorf("il ponte non ha %s", v)
		}
	}
}

// Il banco con G39-03: S8 con sposta_da e i suoi rifiuti (forma di core/rfq/sorgente/sposta.go), la revoca con la causa e la
// bozza (revoca.go).
func TestBancoDistintaUnicaG39Sposta(t *testing.T) {
	b := nuovoBancoDU(t)
	base := "/thread/" + bdThread
	k := func(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-0000000e9%03d", n) }
	b.bdChiedi(t, "POST", "/__banco/sorgente-s8", m{"prodotto": bdP2, "allegato": bdA1, "bozza": true}, 204)
	reg := b.registro(bdA1, bdShaStep, b.versioneProposta)
	s8 := func(n int, extra m) m {
		c := m{"chiave_idempotenza": k(n), "prodotto_id": bdP1, "gruppo": reg["gruppo"].(m)["chiave"], "impronta_contenuto": reg["impronta_contenuto"], "codice_atteso": "ACM-1000"}
		for kk, v := range extra {
			c[kk] = v
		}
		return c
	}
	url := base + "/sorgente/step/" + bdA1 + "/conferma-sorgente"
	// senza il gesto: sorgente_spostabile con altro_prodotto
	if e := b.bdChiedi(t, "POST", url, s8(1, nil), 422); e["codice"] != "STEP_SORGENTE_DI_ALTRO_PRODOTTO" || e["motivo"] != "sorgente_spostabile" || e["altro_prodotto"] != bdP2 {
		t.Errorf("S8 senza sposta_da: %v", e)
	}
	// la BOM dell'altro confermata con lui: bom_confermata, niente spostamento
	b.bomConfermataAltro = true
	if e := b.bdChiedi(t, "POST", url, s8(2, m{"sposta_da": bdP2, "motivo": "m"}), 422); e["motivo"] != "bom_confermata" || e["altro_prodotto"] != bdP2 {
		t.Errorf("S8 con la BOM dell'altro confermata: %v", e)
	}
	b.bomConfermataAltro = false
	if b.s8[bdP2]["valida"] != true || b.bozze[bdP2].Stato != "aperta" {
		t.Fatal("i rifiuti hanno scritto")
	}
	// lo spostamento: la revoca «sorgente errata» e la bozza annullata di là, la sorgente qui
	r := b.bdChiedi(t, "POST", url, s8(3, m{"sposta_da": bdP2, "motivo": "lo STEP è del primo prodotto"}), 200)
	sp, _ := r["spostamento"].(m)
	if sp == nil || sp["caso"] != "sorgente_s8" || sp["da_prodotto_id"] != bdP2 || sp["bozza_abbandonata"] != b.bozze[bdP2].ID || !strings.HasPrefix(bdStr(sp["motivo"]), "sorgente_errata: ") {
		t.Errorf("spostamento: %v", sp)
	}
	if b.s8[bdP2]["valida"] != false || b.s8[bdP2]["causa"] != "sorgente_errata" || b.bozze[bdP2].Stato != "abbandonata" || b.s8Valida(bdP1) != bdA1 {
		t.Errorf("dopo lo spostamento: %v %v %s", b.s8[bdP2], b.s8[bdP1], b.bozze[bdP2].Stato)
	}
	// la revoca con la bozza dichiarata a una versione vecchia: 409 BOZZA_VERSIONE, niente scritto
	bz := b.bozze[bdP1]
	rev := base + "/sorgente/step/" + bdA1 + "/revoca-sorgente"
	if e := b.bdChiedi(t, "POST", rev, m{"chiave_idempotenza": k(4), "prodotto_id": bdP1, "motivo": "m", "sorgente_errata": true, "bozza": m{"bozza_id": bz.ID, "versione_attesa": bz.Versione + 1}}, 409); e["codice"] != "BOZZA_VERSIONE" {
		t.Errorf("revoca con la bozza vecchia: %v", e)
	}
	if b.s8Valida(bdP1) != bdA1 || bz.Stato != "aperta" {
		t.Fatal("la revoca rifiutata ha scritto")
	}
	// la revoca giusta: la bozza nata dalla sorgente annullata (la BOM non è confermata con lui)
	e := b.bdChiedi(t, "POST", rev, m{"chiave_idempotenza": k(5), "prodotto_id": bdP1, "motivo": "m", "sorgente_errata": true, "bozza": m{"bozza_id": bz.ID, "versione_attesa": bz.Versione}}, 200)
	if e["bozza_abbandonata"] != bz.ID || bz.Stato != "abbandonata" || e["motivo"] != "sorgente_errata: m" {
		t.Errorf("revoca: %v (bozza %s)", e, bz.Stato)
	}
	// lo STEP ora è un documento del primo prodotto senza S8 valida: S8 sul secondo vuole il gesto «Riassocia a ‹P›»
	// (sposta_da; messaggio 46 [U]: non un motivo scritto)
	b.s8[bdP1]["spostato_a"] = nil
	ss := m{"chiave_idempotenza": k(6), "prodotto_id": bdP2, "partenza": "solo_prodotto", "stato_analisi": "letto", "presa_d_atto": true}
	if e := b.bdChiedi(t, "POST", url, ss, 422); e["codice"] != "STEP_SORGENTE_DI_ALTRO_PRODOTTO" || e["motivo"] != "riassociazione" || e["altro_prodotto"] != bdP1 {
		t.Errorf("S8 di un documento dell'altro prodotto senza «Riassocia»: %v", e)
	}
}

// bdCopiaJSON: una copia profonda di un oggetto del banco (per i confronti fra versioni dello snapshot).
func bdCopiaJSON(t *testing.T, o m) m {
	t.Helper()
	raw, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	var c m
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// G39-01 [U] nel banco (parte 2 della corsia interfaccia, dopo le corsie server): la provenienza della PB con il registro
// dichiarato la decide come il server (distintabozza/provenienza3.go): sul nodo proposto «motore», su un altro nodo
// «corretta», un registro che non propone il file 422 proposta_non_corrispondente; R1 con ?suggerimenti_motore=1 dà
// suggerimenti_motore e step_candidati[] nella forma del server. Sul banco di 649f08b fallisce: ogni registro dichiarato
// era «motore» e R1 ignorava ?suggerimenti_motore=1.
func TestBancoDistintaUnicaG39ProvenienzaDelRegistro(t *testing.T) {
	b := nuovoBancoDU(t)
	reg := b.registro(bdA1, bdShaStep, b.versioneProposta)
	imp := bdImpronta(reg)
	b.registri[imp] = reg
	prima := bdCopiaJSON(t, b.statoDiPartenza(reg, imp))
	for _, c := range []struct{ a, k, atteso string }{{bdA2, "k-7", "motore"}, {bdA2, "k-11", "corretta"}, {bdA3, "k-11", "motore"}, {bdA3, "k-12", "corretta"}, {bdA4, "k-7", ""}} {
		if got := b.provenienzaDelRegistro(imp, c.a, c.k, prima); got != c.atteso {
			t.Errorf("provenienza di %s su %s con il registro: %q, attesa %q", c.a, c.k, got, c.atteso)
		}
	}
	nuovo := bdCopiaJSON(t, prima)
	nuovo["documenti_preassociati"] = []any{m{"allegato_id": bdA2, "chiave_bozza": "k-7", "importazione": imp}, m{"allegato_id": bdA3, "chiave_bozza": "k-12", "importazione": imp}}
	if codice, motivo, campo, testo := b.regolePreassociazioni(prima, nuovo, nil); codice != "" {
		t.Fatalf("R2 rifiuta il registro dichiarato che propone i file: %s %s %s %s", codice, motivo, campo, testo)
	}
	bdProvenienzeDecise(prima, nuovo, func(i, a, k string) string { return b.provenienzaDelRegistro(i, a, k, nuovo) })
	got := map[string]string{}
	for _, x := range bdElenco(nuovo["documenti_preassociati"]) {
		d := x.(m)
		got[bdStr(d["allegato_id"])] = bdStr(d["provenienza"]) + "|" + bdStr(d["importazione"])
	}
	if got[bdA2] != "motore|"+imp || got[bdA3] != "corretta|"+imp {
		t.Errorf("provenienze decise: %v", got)
	}
	senza := bdCopiaJSON(t, prima)
	senza["documenti_preassociati"] = []any{m{"allegato_id": bdA4, "chiave_bozza": "k-7", "importazione": imp}}
	if codice, motivo, _, _ := b.regolePreassociazioni(prima, senza, nil); codice != "CORPO_NON_VALIDO" || motivo != "proposta_non_corrispondente" {
		t.Errorf("il registro che non propone il file: %s %s", codice, motivo)
	}
	b.modi["r2-superato"] = "422"
	if codice, motivo, _, testo := b.regolePreassociazioni(prima, nuovo, nil); codice != "CORPO_NON_VALIDO" || motivo != "proposta_non_corrispondente" || !strings.Contains(testo, "si rileggono i suggerimenti (R1)") {
		t.Errorf("la modalità r2-superato: %s %s %s", codice, motivo, testo)
	}
	// R1 con ?suggerimenti_motore=1, anche senza bozza (nessuna_bozza): le chiavi del server (distintabozza.LetturaR1)
	base := "/thread/" + bdThread + "/motore-a/bozza?prodotto=" + bdP1
	if r1 := b.bdChiedi(t, "GET", base, nil, 200); r1["step_candidati"] != nil || r1["suggerimenti_motore"] != nil {
		t.Errorf("R1 senza ?suggerimenti_motore=1 dà il calcolo: %v", r1)
	}
	r1 := b.bdChiedi(t, "GET", base+"&suggerimenti_motore=1", nil, 200)
	if sm, _ := r1["suggerimenti_motore"].(m); sm["calcolati"] != true || len(bdElenco(r1["step_candidati"])) != 0 {
		t.Errorf("R1 con il calcolo, senza candidati: %v", r1)
	}
	b.bdChiedi(t, "POST", "/__banco/interruttore", m{"nome": "step-candidati", "attivo": true}, 204)
	l := bdElenco(b.bdChiedi(t, "GET", base+"&suggerimenti_motore=1", nil, 200)["step_candidati"])
	if len(l) != 1 || l[0].(m)["allegato_id"] != bdA1 || l[0].(m)["dal_motore"] != true {
		t.Fatalf("step_candidati con l'interruttore: %v", l)
	}
	if got, want := chiaviDelBanco(l[0].(m)), etichetteJSON(reflect.TypeOf(distintabozza.StepCandidatoR1{})); !reflect.DeepEqual(got, want) {
		t.Errorf("campi di step_candidati[]: banco %v, server %v", got, want)
	}
	if got, want := etichetteJSON(reflect.TypeOf(distintabozza.EsitoMotoreR1{})), []string{"calcolati", "errore"}; !reflect.DeepEqual(got, want) {
		t.Errorf("campi di suggerimenti_motore del server: %v (il banco dà %v)", got, want)
	}
	if !b.stepCandidatoDelFinito(b.fileDi(bdA1)) {
		t.Error("con lo STEP candidato non univoco R2 del banco non rifiuta la sua preassociazione (il server sì)")
	}
}

// G39-01 e G39-02 nella pagina, parte 2 (rilievi della revisione 2 della corsia interfaccia): i pezzi che il browser
// attraversa li prova DU-B70 (TestDistintaUnicaG39ProvenienzaNelBrowser); qui il resto.
func TestDistintaUnicaG39Parte2(t *testing.T) {
	src := duFile(t, duPagina)
	// ogni gesto che preassocia dichiara il registro del motore del file, se il motore lo propone con una sola destinazione
	if fai := duFunzione(t, src, "function fai("); !strings.Contains(fai, "conRegistriDelMotore(azioni)") {
		t.Error("fai non dichiara il registro del motore delle PB nuove (l'Elenco salvava «manuale» una proposta del registro)")
	}
	rf := duFunzione(t, src, "function registroDelFile(")
	for _, v := range []string{"p.preassociazioni_proposte", "S.bozza.preassociazioni_proposte", "registroBozza()", "rif.size === 1"} {
		if !strings.Contains(rf, v) {
			t.Errorf("registroDelFile non ha %s", v)
		}
	}
	if sg := duFunzione(t, src, "function suggerimentiDopoApertura("); !strings.Contains(sg, `"Su un altro nodo…"`) || !strings.Contains(sg, "suggerito: k") {
		t.Error("il suggerimento dopo l'apertura non si sposta su un altro nodo («corretta», G39-01 [U])")
	}
	if !strings.Contains(src, "proposta_non_corrispondente: ") || !strings.Contains(duFunzione(t, src, "async function rileggiBozzaSalvata("), "await rileggiSuggerimenti()") {
		t.Error("il 422 proposta_non_corrispondente non rilegge i suggerimenti")
	}
	if !strings.Contains(duFunzione(t, src, "async function rileggiSuggerimenti("), "suggerimenti_motore: true") {
		t.Error("rileggiSuggerimenti non chiede R1 con ?suggerimenti_motore=1")
	}
	av := duFunzione(t, src, "function avvisoStepCandidati(")
	for _, v := range []string{"suggerimenti_motore: true", "j.step_candidati", `id: "f-step-proposto"`, `id: "f-senza-proposto"`, "fin.__aggiorna()"} {
		if !strings.Contains(av, v) {
			t.Errorf("l'avviso degli STEP candidati non ha %s", v)
		}
	}
	// lo stato della conferma: il segno della correzione non spegne più «mono_pezzo proposto in bozza» (else spostato)
	sc := duFunzione(t, src, "function statoConferma(")
	if i, j := strings.Index(sc, `pill("info", "finito mono_pezzo")`), strings.Index(sc, `pill("bozza", "mono_pezzo proposto in bozza")`); i < 0 || j < 0 ||
		!strings.Contains(sc[i:j], "else if") || strings.Contains(sc[i:j], "correzione_sorgente") {
		t.Error("statoConferma: «mono_pezzo proposto in bozza» non è più l'else di «finito mono_pezzo»")
	}
	if cd := duFunzione(t, src, "function sezioneCondivisione("); !strings.Contains(cd, "x.conferma_valida_prima && strutturali") || !strings.Contains(cd, `e.tipo === "documento"`) {
		t.Error("la condivisione segna «la sua ultima conferma può non valere più» anche per i soli effetti sui documenti")
	}
	if es := duFunzione(t, src, "async function esitoS8("); !strings.Contains(es, `r.motivo === "sostituito_in_altro_prodotto"`) {
		t.Error("S8: GIA_DOCUMENTO sostituito_in_altro_prodotto senza il suo testo")
	}
	if ponte := duFile(t, duPonte); !strings.Contains(ponte, `if (o.suggerimenti_motore) q += "&suggerimenti_motore=1";`) {
		t.Error("il ponte non chiede R1 con ?suggerimenti_motore=1")
	}
}
