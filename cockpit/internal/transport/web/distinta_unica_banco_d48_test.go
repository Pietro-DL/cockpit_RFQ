package web

// L1 del banco della Distinta unica: l'emulazione della PB del motore intatta (D-48-01 A [T], preparata in attesa della
// risposta; distintabozza.pbDalMotore) nel R4 del banco. La PB del motore sul telaio (k-7, la voce #7 propone ACM-1100_A.pdf)
// con il file dal nome «da partner» e' applicabile; torna al confronto del codice letto (codice_diverso) con la rev del nodo
// cambiata, con la proposta del file anche sulla radice (ambigua), con la proposta spostata sulla radice e per una PB
// manuale. Dati sintetici (ACME, nome di file inventato).

import "testing"

func TestBancoDistintaUnicaPBDelMotoreD48(t *testing.T) {
	proponiSu := func(chiavi ...string) func(reg m) {
		return func(reg m) {
			for _, x := range reg["proposta"].(m)["nodi"].([]any) {
				v := x.(m)
				v["associazioni_proposte"] = []any{}
				for _, k := range chiavi {
					if bdStr(v["origine"].(m)["chiave"]) == k {
						v["associazioni_proposte"] = []any{m{"allegato_id": bdA2, "motivo": "nome", "livello": "forte"}}
					}
				}
			}
		}
	}
	esito := func(prov string, cambiaReg func(reg m), cambiaNodo func(n m)) (string, string) {
		t.Helper()
		b := nuovoBancoDU(t)
		reg := b.registro(bdA1, bdShaStep, b.versioneProposta)
		if cambiaReg != nil {
			cambiaReg(reg)
		}
		imp := bdImpronta(reg)
		b.registri[imp] = reg
		snap := bdSnapshotCanonico(b.statoDiPartenza(reg, imp))
		pb := m{"allegato_id": bdA2, "chiave_bozza": "k-7", "provenienza": prov}
		if prov == "motore" {
			pb["importazione"] = imp
		}
		snap["documenti_preassociati"] = []any{pb}
		b.bozze[bdP1] = &bozzaBanco{ID: "00000000-0000-4000-8000-0000000e4801", Stato: "aperta", Registro: imp, Versione: 1, Snapshot: bdSnapshotCanonico(snap)}
		f := b.fileDi(bdA2)
		f.Nome, f.Codice, f.Rev = "BOM_9120001_1010_20260101_E.pdf", "BOM_9120001_1010_20260101", "E"
		corpo := bdSnapshotCanonico(snap)
		if cambiaNodo != nil {
			for _, x := range corpo["nodi"].([]any) {
				if n := x.(m); n["chiave_bozza"] == "k-7" {
					cambiaNodo(n)
				}
			}
		}
		r := b.riepilogo(bdP1, corpo, 1, true)
		for _, x := range bdElenco(r["documenti_preassociati"]) {
			if e := x.(m); bdStr(e["allegato_id"]) == bdA2 {
				return bdStr(e["esito"]), bdStr(e["motivo"])
			}
		}
		t.Fatalf("la PB di ACM-1100_A.pdf non e' nel riepilogo del banco: %v", r["documenti_preassociati"])
		return "", ""
	}
	if e, mo := esito("motore", nil, nil); e != "applicabile" {
		t.Fatalf("PB del motore intatta: %s %s", e, mo)
	}
	for nome, c := range map[string]struct {
		prov string
		reg  func(m)
		nodo func(m)
	}{
		"rev del nodo cambiata":          {"motore", nil, func(n m) { n["identita"].(m)["rev"] = "B" }},
		"proposta ambigua":               {"motore", proponiSu("#1", "#7"), nil},
		"proposta spostata sulla radice": {"motore", proponiSu("#1"), nil},
		"PB manuale":                     {"manuale", nil, nil},
	} {
		if e, mo := esito(c.prov, c.reg, c.nodo); e != "in_conflitto" || mo != "codice_diverso" {
			t.Fatalf("%s: %s %s", nome, e, mo)
		}
	}
}
