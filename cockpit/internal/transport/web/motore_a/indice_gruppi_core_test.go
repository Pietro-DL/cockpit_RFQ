package motore_a

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/valutazione"
)

// A-09 (V1: emendamento di V1 §6.1; piano della 0023 T-09, la parità della chiave): Indice.Gruppi chiama
// propostabom.GruppiDelProdotto, e i suoi gruppi sono quelli del core, con la chiave, il tipo, l'allegato e la radice,
// nello stesso ordine, più i dati della pagina giusti: il nome del file del gruppo, la candidata k-esima del prodotto per
// la k-esima «cand», i nodi del gruppo. Per ogni prodotto delle scene del web: la scena ACME; con le candidate in un altro
// ordine; con la BOM di lavoro che non è la prima struttura; con due BOM di lavoro dello stesso prodotto (fuori da R76 A:
// vale la prima, come Indice.bomDi, che la pagina usa altrove); senza BOM di lavoro e senza confermati senza STEP. La
// chiave della pagina e quella del registro d'importazione sono la stessa.
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
		"due BOM di lavoro dello stesso prodotto: vale la prima": func(et *valutazione.EsitoThread) {
			for _, s := range et.Ancoraggi.Strutture {
				if s.Stato == ancoraggio.StatoBOMDiLavoroProposta {
					s.AllegatoID, s.Sha256, s.Radice = uuid.MustParse("00000000-0000-4000-8000-0000000a09f1"), strings.Repeat("a9", 32), "nodo:altra:#1"
					et.Ancoraggi.Strutture = append(et.Ancoraggi.Strutture, s)
					return
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
				p := x.Prodotto(pv.Rif)
				candidate := 0
				for i := range web {
					w, k := web[i], core[i]
					if w.Chiave != k.Chiave || w.Tipo != k.Tipo || w.Radice != k.Radice || (w.AllegatoID == nil) != (k.AllegatoID == nil) ||
						(w.AllegatoID != nil && *w.AllegatoID != *k.AllegatoID) {
						t.Errorf("%s, gruppo %d: pagina %q %q %q %v, core %q %q %q %v", pv.Rif, i, w.Chiave, w.Tipo, w.Radice, w.AllegatoID,
							k.Chiave, k.Tipo, k.Radice, k.AllegatoID)
					}
					if w.Tipo == GruppoCandidata {
						if w.Struttura == nil || w.Struttura != &p.StruttureCandidate[candidate] || k.Sha256 != w.Struttura.Sha256 ||
							w.Struttura.AllegatoID != *w.AllegatoID || w.Struttura.Radice != w.Radice {
							t.Errorf("%s, gruppo %q: la struttura della pagina non è la candidata %d del prodotto (sha256 del core %q)", pv.Rif, w.Chiave, candidate, k.Sha256)
						}
						candidate++
					} else if w.Struttura != nil {
						t.Errorf("%s, gruppo %q: una struttura candidata fuori da «cand»", pv.Rif, w.Chiave)
					}
					if (w.AllegatoID == nil && w.NomeFile != "") || (w.AllegatoID != nil && w.NomeFile != x.NomeFile(*w.AllegatoID)) {
						t.Errorf("%s, gruppo %q: il nome del file %q", pv.Rif, w.Chiave, w.NomeFile)
					}
					if !reflect.DeepEqual(w.Nodi, x.nodiDelGruppo(pv.Rif, w.Chiave)) {
						t.Errorf("%s, gruppo %q: i nodi della scheda non sono quelli del gruppo", pv.Rif, w.Chiave)
					}
					if st := x.bomDi(pv.Rif); w.Tipo == GruppoBOM && (st == nil) != (k.AllegatoID == nil) ||
						w.Tipo == GruppoBOM && st != nil && (k.Sha256 != st.Sha256 || *k.AllegatoID != st.AllegatoID || k.Radice != st.Radice) {
						t.Errorf("%s: il gruppo bom del core %+v non ha la struttura della BOM di lavoro della pagina %+v", pv.Rif, k, st)
					}
				}
				if candidate != len(p.StruttureCandidate) {
					t.Errorf("%s: %d schede «cand» per %d candidate", pv.Rif, candidate, len(p.StruttureCandidate))
				}
			}
		})
	}
	if GruppoBOM != propostabom.GruppoBOM || GruppoCandidata != propostabom.GruppoCandidata || GruppoSenzaSTEP != propostabom.GruppoSenzaSTEP {
		t.Error("le costanti dei gruppi della pagina non sono quelle del core")
	}
}
