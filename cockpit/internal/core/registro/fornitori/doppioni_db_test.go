//go:build integrazione

package fornitori

import (
	"strings"
	"testing"
)

// Un foglio compilato a mano ripete le cose: lo stesso dominio sotto due fornitori dello stesso
// gruppo, la stessa email due volte. L'anteprima lo deve dire, e «Applica» deve scrivere quello che
// l'anteprima ha promesso invece di fermarsi sulla chiave primaria e annullare tutto l'import.
func TestIDoppioniDelFileNonFermanoLImport(t *testing.T) {
	b := prepara(t)
	s := Seme{Fornitori: []FornitoreSeme{
		{RagioneSociale: "Torneria Esempio", Tipo: "processi",
			Domini:   []string{"gruppo.example", "gruppo.example"},
			Contatti: []ContattoSeme{{Email: "ufficio@gruppo.example"}, {Email: "ufficio@gruppo.example"}}},
		{RagioneSociale: "Fresature Esempio", Tipo: "processi",
			Domini: []string{"gruppo.example"}},
	}}

	a, err := Calcola(b.ctx, b.q, s)
	if err != nil {
		t.Fatal(err)
	}
	conta := func(rr []Riga, cosa string) int {
		n := 0
		for _, r := range rr {
			if strings.Contains(r.Cosa, cosa) {
				n++
			}
		}
		return n
	}
	if n := conta(a.DaAggiungere, "dominio gruppo.example"); n != 1 {
		t.Errorf("il dominio ripetuto è «da aggiungere» %d volte: %+v", n, a.DaAggiungere)
	}
	if n := conta(a.DaAggiungere, "contatto ufficio@gruppo.example"); n != 1 {
		t.Errorf("l'email ripetuta è «da aggiungere» %d volte: %+v", n, a.DaAggiungere)
	}
	if n := conta(a.NonRisolti, "dominio gruppo.example"); n != 1 {
		t.Errorf("il dominio anche del secondo fornitore non è fra i non risolti: %+v", a.NonRisolti)
	}

	if _, err := Applica(b.ctx, b.pool, s); err != nil {
		t.Fatalf("Applica si ferma sui doppioni del file: %v", err)
	}
	f, err := b.q.GetFornitorePerDominio(b.ctx, "gruppo.example")
	if err != nil || f.RagioneSociale != "Torneria Esempio" {
		t.Errorf("il dominio è andato a %q (err %v): doveva restare al primo fornitore che lo dichiara", f.RagioneSociale, err)
	}
	if n := contaRighe(t, b, `SELECT count(*) FROM contatto_fornitore WHERE email = 'ufficio@gruppo.example'`); n != 1 {
		t.Errorf("%d contatti con la stessa email", n)
	}
}

func contaRighe(t *testing.T, b *banco, sql string) int {
	t.Helper()
	var n int
	if err := b.pool.QueryRow(b.ctx, sql).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
