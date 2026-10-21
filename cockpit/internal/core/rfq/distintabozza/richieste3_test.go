package distintabozza

// Le prove L1 della lettura dei corpi di R2 e R3 nel formato 3 (V1, ondata B; EM §9.2, §9.3): una e una sola forma, il
// vocabolario 2, la chiave del salvataggio UUID v4, il corpo del formato 2 rifiutato (T-12, meta' L1), l'impronta della
// richiesta sulla forma canonica. Le forme costruite dal server (direttiva 35, C-3) sono in richieste3_forme_test.go.

import (
	"encoding/json"
	"strings"
	"testing"
)

func corpoR2(t *testing.T, m map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func salvataggioProva(id idProva3, imp string) map[string]any {
	return map[string]any{"bozza_id": "00000000-0000-4000-8000-0000000b0001", "versione_attesa": 1, "prodotto_id": id.prodotto.String(),
		"vocabolario": 2, "chiave_salvataggio": "00000000-0000-4000-8000-0000000c0001", "snapshot": snapshotProva3(id, imp), "azioni": []any{}}
}

func TestV1LeggiRichiestaR2LaFormaSalvataggio(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	m := salvataggioProva(id, imp)
	r, err := LeggiRichiestaR2(corpoR2(t, m))
	if err != nil {
		t.Fatal(err)
	}
	if r.Forma != FormaSalvataggio || r.Salvataggio.VersioneAttesa != 1 || r.Salvataggio.ProdottoID != id.prodotto ||
		!reSha256.MatchString(r.Salvataggio.ImprontaRichiesta) {
		t.Fatalf("richiesta letta: %+v", r)
	}
	// l'impronta e' della forma canonica: lo stesso corpo con le chiavi in un altro ordine e gli spazi da' la stessa
	b := []byte(" {\"vocabolario\": 2, " + strings.TrimPrefix(string(corpoR2(t, m)), "{"))
	var x map[string]any
	if json.Unmarshal(b, &x) == nil {
		// il corpo con la chiave «vocabolario» ripetuta e' rifiutato dalla lettura stretta
		if _, err := LeggiRichiestaR2(b); err == nil {
			t.Error("chiave ripetuta accettata")
		}
	}
	r2, err := LeggiRichiestaR2([]byte("\n" + string(corpoR2(t, m)) + "  "))
	if err != nil {
		t.Fatal(err)
	}
	if r2.Salvataggio.ImprontaRichiesta != r.Salvataggio.ImprontaRichiesta {
		t.Error("l'impronta cambia con gli spazi")
	}
	// un campo diverso cambia l'impronta
	m["versione_attesa"] = 2
	r3, err := LeggiRichiestaR2(corpoR2(t, m))
	if err != nil {
		t.Fatal(err)
	}
	if r3.Salvataggio.ImprontaRichiesta == r.Salvataggio.ImprontaRichiesta {
		t.Error("due corpi diversi con la stessa impronta")
	}
}

func TestV1LeggiRichiestaR2IRifiuti(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	casi := []struct {
		nome   string
		cambia func(m map[string]any)
		campo  string
		motivo string
	}{
		{"nessuna forma", func(m map[string]any) { delete(m, "snapshot") }, "richiesta", ""},
		{"due forme", func(m map[string]any) { m["crea"] = map[string]any{"importazione": imp} }, "richiesta", ""},
		{"vocabolario 1 (il corpo del formato 2, T-12)", func(m map[string]any) { m["vocabolario"] = 1 }, "vocabolario", ""},
		{"senza vocabolario", func(m map[string]any) { delete(m, "vocabolario") }, "vocabolario", ""},
		{"chiave non v4", func(m map[string]any) { m["chiave_salvataggio"] = "00000000-0000-1000-8000-0000000c0001" }, "chiave_salvataggio", ""},
		{"senza prodotto", func(m map[string]any) { delete(m, "prodotto_id") }, "prodotto_id", ""},
		{"senza versione attesa", func(m map[string]any) { delete(m, "versione_attesa") }, "versione_attesa", ""},
		{"senza bozza", func(m map[string]any) { delete(m, "bozza_id") }, "bozza_id", ""},
		{"campo sconosciuto", func(m map[string]any) { m["formato"] = 3 }, "richiesta", ""},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			m := salvataggioProva(id, imp)
			c.cambia(m)
			_, err := LeggiRichiestaR2(corpoR2(t, m))
			e := errore3(t, err)
			if e.Codice != CodiceCorpoNonValido || (c.campo != "richiesta" && !strings.HasPrefix(e.Campo, c.campo)) || e.Motivo != c.motivo {
				t.Fatalf("errore %+v (campo %s, motivo %q)", e, c.campo, c.motivo)
			}
		})
	}
	// il corpo del formato 2 per intero (snapshot formato 2, vocabolario 1): 422 sul vocabolario, prima di ogni scrittura
	f2 := map[string]any{"bozza_id": "00000000-0000-4000-8000-0000000b0001", "versione_attesa": 1, "prodotto_id": id.prodotto.String(),
		"vocabolario": 1, "chiave_salvataggio": "00000000-0000-4000-8000-0000000c0001",
		"snapshot": map[string]any{"formato": 2, "prodotto_id": id.prodotto.String(), "nodi": []any{}, "archi": []any{}}, "azioni": []any{}}
	_, err := LeggiRichiestaR2(corpoR2(t, f2))
	if e := errore3(t, err); e.Codice != CodiceCorpoNonValido || e.Campo != "vocabolario" {
		t.Fatalf("corpo formato 2: %+v", e)
	}
}

func TestV1LeggiRichiestaR3(t *testing.T) {
	a, err := LeggiRichiestaR3([]byte(`{"versione_attesa":3,"motivo":"si rifa' da capo"}`))
	if err != nil || a.VersioneAttesa != 3 || a.Motivo != "si rifa' da capo" {
		t.Fatalf("%v %+v", err, a)
	}
	for _, b := range []string{`{}`, `{"versione_attesa":0}`, `{"versione_attesa":1,"altro":true}`, `[]`} {
		if _, err := LeggiRichiestaR3([]byte(b)); err == nil {
			t.Errorf("%s accettato", b)
		}
	}
}
