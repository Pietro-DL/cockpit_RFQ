package distintabozza

// L1 — la meta' server dei casi condivisi della pagina (PM23 T-16; PE-03, decisione [T] dell'integratore del 18/10):
// transport/web/e2e/distinta_unica_casi.json, gli stessi che la prova nel browser fa girare sulla funzione della pagina
// (DU-B33). «vassoio»: dalla stessa sequenza di eventi del giornale il server (VassoioDa, quello di R1 ?eventi=1) da' lo
// stesso vassoio; «applica»: le azioni del vocabolario 2 applicate allo snapshot con la regola del server (applica2.go)
// danno lo snapshot atteso.
//
// T-16 lato server e' quindi **in parte** (rilievo della revisione dell'ondata B): il vassoio passa dalla funzione di R1,
// le azioni «applica» solo dall'applicazione strutturale (applicaV2, modificaV2), non dalla strada di R2 (LeggiSnapshot3 e
// Riconcilia3 con le forme di EM §9.2): i casi del file a 33066de hanno snapshot parziali e forme che R2 rifiuta. Passano
// da li' dopo RIC-I1, RIC-I2 (corsia interfaccia) e la decisione su RPC PD-08 (PM23 T-16; RIC-P5).

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type casiCondivisi struct {
	Vassoio []struct {
		Nome   string `json:"nome"`
		Eventi []struct {
			N      int             `json:"n"`
			Azione string          `json:"azione"`
			Chiave string          `json:"chiave"`
			Prima  json.RawMessage `json:"prima"`
			Dopo   json.RawMessage `json:"dopo"`
		} `json:"eventi"`
		Atteso json.RawMessage `json:"atteso"`
	} `json:"vassoio"`
	Applica []struct {
		Nome     string          `json:"nome"`
		Snapshot json.RawMessage `json:"snapshot"`
		Azioni   []struct {
			Azione      string          `json:"azione"`
			ChiaveBozza *string         `json:"chiave_bozza"`
			Prima       json.RawMessage `json:"prima"`
			Dopo        json.RawMessage `json:"dopo"`
		} `json:"azioni"`
		Atteso json.RawMessage `json:"atteso"`
	} `json:"applica"`
}

func leggiCasiCondivisi(t *testing.T) casiCondivisi {
	t.Helper()
	p := filepath.Join("..", "..", "..", "transport", "web", "e2e", "distinta_unica_casi.json")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("i casi condivisi della pagina non si leggono (%s): %v", p, err)
	}
	var c casiCondivisi
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Vassoio) == 0 || len(c.Applica) == 0 {
		t.Fatalf("casi condivisi vuoti: %d vassoio, %d applica", len(c.Vassoio), len(c.Applica))
	}
	return c
}

func TestV1T16IlVassoioDeiCasiCondivisi(t *testing.T) {
	for _, c := range leggiCasiCondivisi(t).Vassoio {
		t.Run(c.Nome, func(t *testing.T) {
			var ee []EventoGiornale
			for _, e := range c.Eventi {
				ee = append(ee, EventoGiornale{N: e.N, Azione: e.Azione, ChiaveBozza: e.Chiave, Prima: e.Prima, Dopo: e.Dopo})
			}
			got := VassoioDa(ee)
			var atteso []VoceVassoio
			if err := json.Unmarshal(c.Atteso, &atteso); err != nil {
				t.Fatal(err)
			}
			if atteso == nil {
				atteso = []VoceVassoio{}
			}
			if !bytes.Equal(canonico(got), canonico(atteso)) {
				t.Fatalf("vassoio del server %s, atteso %s", canonico(got), canonico(atteso))
			}
		})
	}
}

func TestV1PE03LeAzioniDeiCasiCondivisi(t *testing.T) {
	for _, c := range leggiCasiCondivisi(t).Applica {
		t.Run(c.Nome, func(t *testing.T) {
			st, err := statoV2Da(c.Snapshot)
			if err != nil {
				t.Fatal(err)
			}
			for i, a := range c.Azioni {
				var e *Errore
				switch a.Azione {
				case AzNodoModificato, AzNodoRiusato:
					chiave := ""
					if a.ChiaveBozza != nil {
						chiave = *a.ChiaveBozza
					}
					_, _, e = st.modificaV2("azioni", a.Azione, chiave, a.Prima, a.Dopo)
				default:
					e = st.applicaV2("azioni", a.Azione, a.Prima, a.Dopo)
				}
				if e != nil {
					t.Fatalf("azione %d (%s): %v", i, a.Azione, e)
				}
			}
			got, err := st.snapshotJSON()
			if err != nil {
				t.Fatal(err)
			}
			at, err := statoV2Da(c.Atteso)
			if err != nil {
				t.Fatal(err)
			}
			want, err := at.snapshotJSON()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("snapshot del server\n%s\natteso\n%s", got, want)
			}
		})
	}
}
