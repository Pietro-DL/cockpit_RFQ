package bancoa

import (
	"reflect"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/dataset"
)

// L1 — la modalità «regole» («profili attivi validi», A1a-BA; R43 B, R20 b, R47 b, CP-17): la grammatica
// ACME compila con i limiti dell'indice; famiglie, forme attive e riservate, riserve, esempi verificati e
// non verificati; la copertura minima con le lacune, che non cambiano l'esito; la coerenza fra gli esempi con
// rif_caso e i casi che ripetono (stesso caso, stesso cliente, stesso selettore dopo R19, stesso testo,
// valori che non si contraddicono); senza attesi la coerenza resta non eseguita; un cliente scartato e un
// indice non valido sono differenze.
//
// I clienti di questi test sono inventati. Non è pigrizia: le grammatiche vere sono dati privati, e questo
// repository è pubblico. Il cliente è ACME; i valori di rif_caso sono inventati («caso-acme-NN»).

func manifestACME(t *testing.T) dataset.Manifest {
	t.Helper()
	m, err := dataset.Leggi([]byte(leggiTestdata(t, "manifest_acme.json")), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestVerificaRegoleACME(t *testing.T) {
	ind, contenuti := regoleACME(t, nil)
	r := VerificaRegole(manifestACME(t), ind, contenuti, attesiACME(t, nil))
	if r.ClientiAttivi != 1 || r.ClientiScartati != 0 || r.Differenze() != 0 || r.Incoerenze != 0 || !r.CoerenzaEseguita {
		t.Fatalf("rapporto: %+v", r)
	}
	if r.VersioneLimiti != "limiti-acme-1" || len(r.ImprontaIndice) != 64 {
		t.Fatalf("versione dei limiti e impronta dell'indice (R43 B): %q %q", r.VersioneLimiti, r.ImprontaIndice)
	}
	c := r.Clienti[0]
	if c.ClienteID != clienteACME.String() || c.Stato != ClienteAttivo || len(c.Hash) != 64 || c.File != "acme.v1.json" ||
		!reflect.DeepEqual(c.Profili, []string{"acme"}) {
		t.Fatalf("cliente: %+v", c)
	}
	var famiglie []string
	for _, f := range c.Famiglie {
		famiglie = append(famiglie, f.ID)
	}
	if strings.Join(famiglie, " ") != "acme-etichetta-pn acme-lavagna acme-marcatore acme-prefisso acme-punti" {
		t.Errorf("famiglie in ordine di ID: %v", famiglie)
	}
	for _, f := range c.Famiglie {
		if f.ID == "acme-lavagna" && (!reflect.DeepEqual(f.FormeRiservate, []string{"lavagna"}) || len(f.FormeAttive) != 0) {
			t.Errorf("forme della famiglia riservata: %+v", f)
		}
		if !reflect.DeepEqual(f.Ruoli, []string{"componente", "prodotto"}) {
			t.Errorf("%s: ruoli %v", f.ID, f.Ruoli)
		}
	}
	if len(c.Riserve) != 2 {
		t.Errorf("riserve: %+v", c.Riserve)
	}
	if !reflect.DeepEqual(c.Esempi, ConteggioEsempi{Totali: 17, Verificati: 16, NonVerificati: 1,
		NonVerificatiID: []string{"acme-lavagna/e-lavagna"}}) {
		t.Errorf("esempi: %+v", c.Esempi)
	}
	if len(c.Coerenza) != 6 {
		t.Errorf("esempi con rif_caso confrontati: %d, attesi 6", len(c.Coerenza))
	}
	for _, k := range c.Coerenza {
		if !k.Coerente {
			t.Errorf("esempio %s/%s incoerente con %s: %v", k.Famiglia, k.Esempio, k.Caso, k.Motivi)
		}
	}
	// La forma PN dichiara anche l'oggetto, ma nessun esempio vi sta: una lacuna, elencata e non bloccante.
	trovata := false
	for _, l := range c.Lacune {
		if l == (Lacuna{Famiglia: "acme-etichetta-pn", Regola: "pn", Selettore: "oggetto", Manca: MancaPositivo}) {
			trovata = true
		}
		if l.Famiglia == "acme-lavagna" {
			t.Errorf("lacuna di una forma riservata: %+v", l)
		}
	}
	if !trovata || r.Lacune != len(c.Lacune) {
		t.Errorf("lacune: %+v", c.Lacune)
	}
	// Il nome-sottolineato ha un positivo con «.pdf» dopo il codice (la prova di confine) e un negativo vicino.
	for _, l := range c.Lacune {
		if l.Famiglia == "acme-marcatore" && l.Regola == "nome-sottolineato" {
			t.Errorf("lacuna inattesa: %+v", l)
		}
	}
	for _, d := range c.Diagnostiche {
		if d.Gravita == "errore" {
			t.Errorf("errore in un cliente attivo: %+v", d)
		}
	}
}

// TestCoerenzaEsempiCasi: in caso di conflitto vincono gli attesi, e la coerenza lo dice esempio per esempio
// (par.4.10 n.3).
func TestCoerenzaEsempiCasi(t *testing.T) {
	ind, contenuti := regoleACME(t, nil)
	a := attesiACME(t, mutazioni{"attesi_acme.yaml": func(s string) string {
		s = strings.Replace(s, "      testo: 9123456A_2.pdf\n", "      testo: 9123456A_3.pdf\n", 1) // caso 01: testo diverso
		s = strings.Replace(s, "      base: 9.123.4567.3\n      revisione: \"01\"", "      base: 9.123.4567.4\n      revisione: \"01\"", 1)
		s = strings.Replace(s, "      contesto: figlio_step.id\n", "      contesto: radice_step.id\n", 1) // caso 03: selettore diverso
		return strings.Replace(s, "  - id: caso-acme-06\n    profilo: acme\n", "  - id: caso-acme-06\n    profilo: altro\n", 1)
	}})
	r := VerificaRegole(manifestACME(t), ind, contenuti, a)
	motivi := map[string]string{}
	for _, k := range r.Clienti[0].Coerenza {
		motivi[k.Caso] = strings.Join(k.Motivi, "; ")
	}
	for caso, parola := range map[string]string{
		"caso-acme-01": "testo diverso", "caso-acme-04": "base:", "caso-acme-03": "selettore diverso", "caso-acme-06": "profilo",
	} {
		if !strings.Contains(motivi[caso], parola) {
			t.Errorf("%s: motivi %q, atteso %q", caso, motivi[caso], parola)
		}
	}
	if motivi["caso-acme-02"] != "" || motivi["caso-acme-05"] != "" {
		t.Errorf("casi coerenti segnalati: %v", motivi)
	}
	if r.Incoerenze != 4 || r.Differenze() != 4 {
		t.Errorf("incoerenze %d, differenze %d", r.Incoerenze, r.Differenze())
	}

	// Un rif_caso che non c'è negli attesi.
	ind, contenuti = regoleACME(t, mutazioni{"regole/acme.v1.json": func(s string) string {
		return strings.Replace(s, `"rif_caso": "caso-acme-02"`, `"rif_caso": "caso-acme-99"`, 1)
	}})
	r = VerificaRegole(manifestACME(t), ind, contenuti, attesiACME(t, nil))
	if r.Incoerenze != 1 {
		t.Fatalf("rif_caso pendente: %+v", r.Clienti[0].Coerenza)
	}
}

// TestCoerenzaNonEseguitaSenzaAttesi: senza attesi il controllo non è eseguito, mai «coerente» (R44).
func TestCoerenzaNonEseguitaSenzaAttesi(t *testing.T) {
	ind, contenuti := regoleACME(t, nil)
	r := VerificaRegole(manifestACME(t), ind, contenuti, Attesi{})
	if r.CoerenzaEseguita || r.Incoerenze != 0 || len(r.Clienti[0].Coerenza) != 0 {
		t.Fatalf("coerenza senza attesi: %+v", r)
	}
	if r.ClientiAttivi != 1 {
		t.Fatal("le grammatiche si compilano anche senza attesi")
	}
}

// TestClienteScartatoEIndiceNonValido: un file che manca è un cliente scartato, con il motivo; un indice senza
// limiti non attiva niente (R43 B). Tutti e due sono differenze.
func TestClienteScartatoEIndiceNonValido(t *testing.T) {
	ind, _ := regoleACME(t, nil)
	r := VerificaRegole(manifestACME(t), ind, map[string][]byte{}, attesiACME(t, nil))
	if r.ClientiScartati != 1 || r.Differenze() != 1 || r.Clienti[0].Stato != ClienteScartato ||
		r.Clienti[0].Diagnostiche[0].Codice != motorea.CodiceRegoleFileAssente {
		t.Fatalf("cliente scartato: %+v", r)
	}

	ind.Limiti = grammatica.Limiti{}
	r = VerificaRegole(manifestACME(t), ind, map[string][]byte{}, attesiACME(t, nil))
	if len(r.IndiceNonValido) == 0 || r.Differenze() != 1 || len(r.Clienti) != 0 {
		t.Fatalf("indice senza limiti: %+v", r)
	}
	if r.IndiceNonValido[0].Codice != grammatica.CodiceCampoObbligatorio {
		t.Fatalf("codice: %+v", r.IndiceNonValido[0])
	}
}
