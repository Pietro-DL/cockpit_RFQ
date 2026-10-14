package bancoa

import (
	"fmt"
	"strings"
	"testing"
)

// L1 — la tabella delle classi nel rapporto delle regole (decisione dell'utente del 14/10, [U]): per la famiglia che la
// dichiara, il segmento e il numero di voci, nel JSON e nella riga di testo; le altre famiglie restano come prima. Il
// cliente è ACME, con la famiglia acme-punti della grammatica di testdata e una tabella inventata sul segmento «numero».
func TestRapportoRegoleConLaTabellaDelleClassi(t *testing.T) {
	ind, contenuti := regoleACME(t, mutazioni{
		"regole/acme.v1.json": func(s string) string {
			return strings.Replace(s, `"id": "acme-punti",`, `"id": "acme-punti", "classi": {"segmento": "numero", "voci": [
				{"valore": "4567", "categoria": "commerciale", "descrizione": "Boccola inventata", "fonte": "norma ACME 94567"},
				{"valore": "1111", "categoria": "minuteria", "descrizione": "Vite inventata"}]},`, 1)
		},
		"regole/indice_acme.v1.json": func(s string) string {
			return strings.Replace(s, `"max_lunghezza_letterale": 40`, `"max_lunghezza_letterale": 40, "max_voci_classi": 8`, 1)
		},
	})
	r := VerificaRegole(manifestACME(t), ind, contenuti, attesiACME(t, nil))
	if r.ClientiAttivi != 1 || r.Differenze() != 0 {
		t.Fatalf("la grammatica con la tabella non è attiva: %+v", r.Clienti)
	}
	for _, f := range r.Clienti[0].Famiglie {
		if f.ID == "acme-punti" {
			if f.SegmentoClassi != "numero" || f.VociClassi != 2 {
				t.Errorf("acme-punti: tabella %q con %d voci, attese numero e 2", f.SegmentoClassi, f.VociClassi)
			}
		} else if f.SegmentoClassi != "" || f.VociClassi != 0 {
			t.Errorf("%s: una tabella che non c'è: %q %d", f.ID, f.SegmentoClassi, f.VociClassi)
		}
	}
	var righe []string
	testoRegole(func(f string, a ...any) { righe = append(righe, fmt.Sprintf(f, a...)) }, r)
	testo := strings.Join(righe, "\n")
	if !strings.Contains(testo, "famiglia acme-punti: ") || !strings.Contains(testo, "; tabella delle classi sul segmento numero: 2 voci") ||
		strings.Count(testo, "tabella delle classi") != 1 {
		t.Errorf("il testo del rapporto:\n%s", testo)
	}
}
