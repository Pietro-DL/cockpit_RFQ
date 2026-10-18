package motore_a

import (
	"testing"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/valutazione"
)

// A-09 (V1: emendamento di V1 §6.1; piano della 0023 T-09, la parità della chiave): i gruppi della pagina
// (Indice.Gruppi) sono quelli del core (propostabom.GruppiDelProdotto), con la chiave, il tipo, l'allegato e la radice,
// nello stesso ordine, per ogni prodotto delle scene del web: la scena ACME; con le candidate in un altro ordine; con la
// BOM di lavoro che non è la prima struttura; senza BOM di lavoro e senza confermati senza STEP. La chiave della pagina
// e quella del registro d'importazione sono la stessa.
func TestIndiceGruppiComeIlCore(t *testing.T) {
	scene := map[string]func(et *valutazione.EsitoThread){
		"la scena ACME": func(*valutazione.EsitoThread) {},
		"le candidate in un altro ordine": func(et *valutazione.EsitoThread) {
			sc := et.ProdottiValutati[0].StruttureCandidate
			sc[0], sc[2] = sc[2], sc[0]
		},
		"la BOM di lavoro non è la prima struttura": func(et *valutazione.EsitoThread) {
			st := et.Ancoraggi.Strutture
			for i := range st {
				if st[i].Stato == ancoraggio.StatoBOMDiLavoroProposta && i+1 < len(st) {
					st[i], st[len(st)-1] = st[len(st)-1], st[i]
					break
				}
			}
		},
		"senza BOM di lavoro e senza confermati": func(et *valutazione.EsitoThread) {
			et.ProdottiValutati[0].Nodi, et.ProdottiValutati[0].ConfermatiSenzaSTEP = nil, nil
		},
	}
	for nome, cambia := range scene {
		t.Run(nome, func(t *testing.T) {
			f, e, c := scenaIndiceACME()
			et := &e.Thread[0]
			cambia(et)
			x := nuovoIndice3(&f.Thread[0], et, &c)
			if len(et.ProdottiValutati) == 0 {
				t.Fatal("la scena non ha prodotti")
			}
			for _, pv := range et.ProdottiValutati {
				web, core := x.Gruppi(pv.Rif), propostabom.GruppiDelProdotto(pv, et.Ancoraggi.Strutture)
				if len(web) != len(core) {
					t.Fatalf("%s: la pagina ha %d gruppi, il core %d", pv.Rif, len(web), len(core))
				}
				for i := range web {
					w, k := web[i], core[i]
					if w.Chiave != k.Chiave || w.Tipo != k.Tipo || w.Radice != k.Radice || (w.AllegatoID == nil) != (k.AllegatoID == nil) ||
						(w.AllegatoID != nil && *w.AllegatoID != *k.AllegatoID) {
						t.Errorf("%s, gruppo %d: pagina %q %q %q %v, core %q %q %q %v", pv.Rif, i, w.Chiave, w.Tipo, w.Radice, w.AllegatoID,
							k.Chiave, k.Tipo, k.Radice, k.AllegatoID)
					}
					if w.Tipo == GruppoCandidata && (w.Struttura == nil || k.Sha256 != w.Struttura.Sha256) {
						t.Errorf("%s, gruppo %q: lo sha256 del core %q non è quello della candidata", pv.Rif, w.Chiave, k.Sha256)
					}
					if st := x.bomDi(pv.Rif); w.Tipo == GruppoBOM && (st == nil) != (k.AllegatoID == nil) || w.Tipo == GruppoBOM && st != nil && k.Sha256 != st.Sha256 {
						t.Errorf("%s: il gruppo bom del core %+v non ha la struttura della BOM di lavoro della pagina %+v", pv.Rif, k, st)
					}
				}
			}
		})
	}
	if GruppoBOM != propostabom.GruppoBOM || GruppoCandidata != propostabom.GruppoCandidata || GruppoSenzaSTEP != propostabom.GruppoSenzaSTEP {
		t.Error("le costanti dei gruppi della pagina non sono quelle del core")
	}
}
