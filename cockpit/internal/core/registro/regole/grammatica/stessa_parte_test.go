package grammatica

import (
	"encoding/json"
	"strings"
	"testing"
)

// L1 — il campo stessa_parte_di di una forma (KN-01, stessa_parte.go): la relazione della stessa parte fra due letture
// di un nome di file. Valida accetta un elenco di famiglie della stessa grammatica, di un altro namespace, su una forma
// che legge solo nome_file; ogni altro uso è un errore con il suo percorso. Senza il campo, canonico e hash di prima.

// conRelazione: la grammatica ACME con acme-documento in un namespace a sé e la sua forma del nome che dichiara la
// relazione verso le famiglie date.
func conRelazione(t *testing.T, namespace string, cambia func(*FormaCodice)) []string {
	t.Helper()
	ds := mutata(t, func(g *Grammatica) {
		for i := range g.Famiglie {
			if g.Famiglie[i].ID != "acme-documento" {
				continue
			}
			g.Famiglie[i].Namespace = namespace
			cambia(&g.Famiglie[i].Forme[0])
		}
	})
	var out []string
	for _, d := range errori(ds) {
		out = append(out, d.Codice+" "+d.Percorso)
	}
	return out
}

func TestStessaParteDiValida(t *testing.T) {
	const p = "famiglie_codice[acme-documento].forme[documento].stessa_parte_di"
	if got := conRelazione(t, "acme-doc", func(f *FormaCodice) { f.StessaParteDi = []string{"acme-prefisso", "acme-etichetta"} }); len(got) != 0 {
		t.Fatalf("una relazione verso due famiglie di un altro namespace è valida: %v", got)
	}
	for _, c := range []struct {
		nome      string
		namespace string
		cambia    func(*FormaCodice)
		atteso    string
	}{
		{"famiglia assente", "acme-doc", func(f *FormaCodice) { f.StessaParteDi = []string{"acme-nessuna"} }, CodiceRiferimentoPendente + " " + p + "[0]"},
		{"la famiglia della forma", "acme-doc", func(f *FormaCodice) { f.StessaParteDi = []string{"acme-documento"} }, CodiceEnumIgnoto + " " + p + "[0]"},
		{"lo stesso namespace", "acme", func(f *FormaCodice) { f.StessaParteDi = []string{"acme-prefisso"} }, CodiceEnumIgnoto + " " + p + "[0]"},
		{"un ID vuoto", "acme-doc", func(f *FormaCodice) { f.StessaParteDi = []string{" "} }, CodiceCampoObbligatorio + " " + p + "[0]"},
		{"una ripetizione", "acme-doc", func(f *FormaCodice) { f.StessaParteDi = []string{"acme-prefisso", "acme-prefisso"} }, CodiceInsiemeConDuplicati + " " + p + "[1]"},
		{"fuori dal nome del file", "acme-doc", func(f *FormaCodice) {
			f.Selettori = []string{"nome_file", "corpo"}
			f.StessaParteDi = []string{"acme-prefisso"}
		}, CodiceEnumIgnoto + " " + p},
	} {
		got := conRelazione(t, c.namespace, c.cambia)
		trovato := false
		for _, g := range got {
			trovato = trovato || g == c.atteso
		}
		if !trovato {
			t.Errorf("%s: atteso %q, errori %v", c.nome, c.atteso, got)
		}
	}
}

// TestStessaParteDiNelCanonico: il campo entra nel canonico come insieme (in ordine), e senza il campo il canonico non
// ha la chiave: le grammatiche di prima hanno lo stesso hash.
func TestStessaParteDiNelCanonico(t *testing.T) {
	prima, err := NuovoSnapshot([]byte(grammaticaACME), limitiACME())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(prima.Canonico), "stessa_parte_di") {
		t.Fatalf("senza il campo il canonico non ha la chiave")
	}
	g, err := Decodifica([]byte(grammaticaACME))
	if err != nil {
		t.Fatal(err)
	}
	for i := range g.Famiglie {
		if g.Famiglie[i].ID == "acme-documento" {
			g.Famiglie[i].Namespace = "acme-doc"
			g.Famiglie[i].Forme[0].StessaParteDi = []string{"acme-prefisso", "acme-etichetta"}
		}
	}
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NuovoSnapshot(raw, limitiACME())
	if err != nil {
		t.Fatalf("snapshot con la relazione: %v", err)
	}
	if !strings.Contains(string(s.Canonico), `"stessa_parte_di":["acme-etichetta","acme-prefisso"]`) || s.Hash == prima.Hash {
		t.Errorf("il canonico con la relazione in ordine: %s", s.Canonico)
	}
}
