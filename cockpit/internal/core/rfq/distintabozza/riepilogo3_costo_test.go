package distintabozza

// L1 — la misura del costo di R4 formato 3 con 5000 nodi (EM §9.4, «Uso dalla pagina»: il costo si misura con 5000 nodi
// [T, A4-Q11 A]; direttiva 39, blocco «R4 completo» della corsia persistenza, punto 8), sulla parte pura: la lettura del
// corpo (LeggiRichiestaR4: snapshot stretto e validato), la forma canonica, calcolaR4 con la firma. Due casi: 5000 nodi
// nuovi (crea) e 5000 nodi registrati con la working uguale. Le durate vanno nel log (go test -v); la prova fallisce solo
// oltre un tetto largo (30 s per il calcolo), per non dipendere dalla macchina. La misura con il database e' la L4
// TestV1D39R4Costo5000NodiDB. Dati sintetici.

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// albero5000: 50 sottoassiemi sotto la radice e 99 particolari sotto ognuno (98 sotto l'ultimo): con la radice 5000 nodi,
// il limite della bozza (MaxNodi3), e 4999 archi.
func albero5000(f *fixR4, registrati bool) {
	for i := 0; i < 50; i++ {
		ks := fmt.Sprintf("k-s%02d", i)
		cs := fmt.Sprintf("ACM-S%04d", i)
		if registrati {
			id := f.componente(ks, cs, "sottoassieme")
			f.arcoW(f.prodotto, id, 1)
			f.nodoReg(ks, id, "sottoassieme")
		} else {
			f.nodoMan(ks, cs, "sottoassieme")
		}
		f.arco("k-root", ks, 1)
		for j := 0; j < 99 && !(i == 49 && j == 98); j++ {
			kp := fmt.Sprintf("k-p%02d-%02d", i, j)
			cp := fmt.Sprintf("ACM-P%02d%02d", i, j)
			if registrati {
				id := f.componente(kp, cp, "sciolto")
				f.arcoW(idDi("r4-"+ks), id, int32(j%5+1))
				f.nodoReg(kp, id, "sciolto")
			} else {
				f.nodoMan(kp, cp, "sciolto")
			}
			f.arco(ks, kp, j%5+1)
		}
	}
}

func TestR4Costo5000Nodi(t *testing.T) {
	if testing.Short() {
		t.Skip("misura del costo: senza -short")
	}
	for _, caso := range []struct {
		nome       string
		registrati bool
	}{{"nodi nuovi", false}, {"nodi registrati", true}} {
		f := nuovaFixR4(t)
		albero5000(f, caso.registrati)
		f.sorgenteValida(shaProva('e'))
		corpo := map[string]any{"formato": 3, "prodotto_id": f.prodotto, "versione_bom_base": map[string]any{"conferma_id": nil, "numero": 0, "contatore": 0},
			"firma_base": firmaProva, "nodi": f.nodi, "archi": f.archi, "decisioni": []any{}, "documenti_preassociati": []any{}, "sorgente": f.sorgente}
		raw, err := json.Marshal(corpo)
		if err != nil {
			t.Fatal(err)
		}
		t0 := time.Now()
		if _, err := LeggiRichiestaR4(raw); err != nil {
			t.Fatalf("%s: il corpo: %v", caso.nome, err)
		}
		lettura := time.Since(t0)
		in := f.ingressi()
		t1 := time.Now()
		r, err := calcolaR4(in)
		calcolo := time.Since(t1)
		if err != nil {
			t.Fatalf("%s: %v", caso.nome, err)
		}
		if len(r.Identita) != 5000 || len(r.Archi) != 4999 || len(r.Blocchi) != 0 {
			t.Fatalf("%s: identita %d archi %d blocchi %v", caso.nome, len(r.Identita), len(r.Archi), codici(r.Blocchi))
		}
		t.Logf("R4 con 5000 nodi (%s): corpo di %d byte letto in %v, calcolo e firma in %v", caso.nome, len(raw), lettura, calcolo)
		if calcolo > 30*time.Second {
			t.Fatalf("%s: il calcolo di R4 con 5000 nodi dura %v (tetto 30 s)", caso.nome, calcolo)
		}
	}
}
