package bancoa

import (
	"strings"
	"testing"
)

// L1 — la lettura degli attesi (A1a-BA; R9, R47 a, ATT r.7-16): la testata e i casi_contratto di un YAML
// sintetico con i soli nomi neutri delle chiavi; le altre sezioni accettate e non lette; una chiave
// sconosciuta, ripetuta o fuori posto è un errore, mai ignorata; il testo dei valori resta com'è scritto
// (una revisione «00» resta «00», D4); l'errore non riporta il testo dell'ingresso.
//
// I clienti di questi test sono inventati. Non è pigrizia: gli attesi veri contengono codici e nomi dei
// clienti, e questo repository è pubblico. Il cliente è ACME e gli ID dei casi sono «caso-acme-NN».

func TestLeggiAttesiACME(t *testing.T) {
	a := attesiACME(t, nil)
	if a.Testata.VersioneAttesi != 1 || a.Testata.Data != "2026-10-04" || a.Testata.Stato != "sintetico" {
		t.Fatalf("testata: %+v", a.Testata)
	}
	if len(a.Casi) != 10 {
		t.Fatalf("%d casi, attesi 10", len(a.Casi))
	}
	c := a.Casi[0]
	if c.ID != "caso-acme-01" || c.Profilo != "acme" || c.Livello != "banco" || c.StatoAtteso != StatoAttesoDefinito ||
		c.DipendeDa != "Q-ACME-1" || c.Decisione != "D1" || c.Contesto != "nome_file" || c.Testo != "9123456A_2.pdf" {
		t.Fatalf("primo caso: %+v", c)
	}
	var nomi []string
	for _, k := range c.Atteso {
		nomi = append(nomi, k.Chiave)
	}
	if strings.Join(nomi, " ") != "base marcatore originale_conservato revisione" {
		t.Fatalf("chiavi in ordine alfabetico: %v", nomi)
	}
	valori := map[string]ValoreAtteso{}
	for _, k := range c.Atteso {
		valori[k.Chiave] = k.Valore
	}
	if v := valori["base"]; v.Tipo != TipoIntero || v.Testo != "9123456" {
		t.Errorf("base: %+v", v)
	}
	if v := valori["revisione"]; v.Tipo != TipoStringa || v.Testo != "2" {
		t.Errorf("revisione: %+v", v)
	}
	if v := valori["originale_conservato"]; v.Tipo != TipoBooleano || v.Testo != "true" {
		t.Errorf("booleano: %+v", v)
	}
	if v := a.Casi[1].Atteso[0].Valore; a.Casi[1].Atteso[0].Chiave != "basi" || v.Tipo != TipoLista || v.String() != "[9123456, 9123457]" {
		t.Errorf("lista: %+v", v)
	}
	if k := a.Casi[2].Atteso[2]; k.Chiave != "revisione_da_questo_campo" || k.Valore.Tipo != TipoNullo || k.Valore.String() != "null" {
		t.Errorf("null: %+v", k)
	}
	if a.Casi[2].Contesto != "figlio_step.id" {
		t.Errorf("il contesto si legge com'è scritto; lo traduce TraduciContesto: %q", a.Casi[2].Contesto)
	}
	if c := a.Casi[6]; c.StatoAtteso != StatoAttesoRiservato || c.Nota == "" {
		t.Errorf("caso riservato: %+v", c)
	}
	if c := a.Casi[9]; c.Precondizioni == nil || !c.Precondizioni.EntitaCondivisaConCodice || c.Precondizioni.BaseStrutturata != "9123456" ||
		c.AllegatoID != "allegato-acme-1" || c.Testo != "01" {
		t.Errorf("precondizioni e allegato: %+v", c)
	}
}

// TestLeggiAttesiTestoComeScritto: il YAML risolverebbe «00» come un intero e «1.10» come un decimale; il
// valore atteso tiene il testo del file (D4: gli zeri della revisione contano).
func TestLeggiAttesiTestoComeScritto(t *testing.T) {
	a := attesiACME(t, mutazioni{"attesi_acme.yaml": func(s string) string {
		return strings.Replace(s, `      revisione: "01"`+"\n      forte: 0", "      revisione: 00\n      base_candidata: 1.10\n      forte: 0", 1)
	}})
	valori := map[string]ValoreAtteso{}
	for _, k := range a.Casi[3].Atteso {
		valori[k.Chiave] = k.Valore
	}
	if v := valori["revisione"]; v.Testo != "00" {
		t.Errorf("revisione: %+v", v)
	}
	if v := valori["base_candidata"]; v.Testo != "1.10" || v.Tipo != TipoStringa {
		t.Errorf("decimale: %+v", v)
	}
}

// TestLeggiAttesiStretto: una chiave nuova negli attesi non si ignora; nessun ripiego.
func TestLeggiAttesiStretto(t *testing.T) {
	buono := leggiTestdata(t, "attesi_acme.yaml")
	primoAtteso := "    atteso:\n      base: 9123456\n"
	bloccoAtteso := primoAtteso + "      marcatore: A\n      revisione: \"2\"\n      originale_conservato: true\n"
	casi := map[string]string{
		"chiave di primo livello sconosciuta": buono + "sezione_nuova: []\n",
		"chiave di primo livello ripetuta":    buono + "gate: []\n",
		"chiave sconosciuta in un caso":       strings.Replace(buono, "    livello: banco\n", "    livello: banco\n    priorita: alta\n", 1),
		"chiave ripetuta in un caso":          strings.Replace(buono, "    livello: banco\n", "    livello: banco\n    livello: banco\n", 1),
		"chiave sconosciuta nell'input":       strings.Replace(buono, "      contesto: nome_file\n", "      contesto: nome_file\n      lingua: it\n", 1),
		"chiave sconosciuta nell'atteso":      strings.Replace(buono, primoAtteso, primoAtteso+"      chiave_inventata: 1\n", 1),
		"chiave ripetuta nell'atteso":         strings.Replace(buono, primoAtteso, primoAtteso+"      base: 9123456\n", 1),
		"chiave sconosciuta in precondizioni": strings.Replace(buono, "      entita_condivisa_con_codice: true\n", "      entita_condivisa_con_codice: true\n      altro: 1\n", 1),
		"stato atteso fuori elenco":           strings.Replace(buono, "    stato_atteso: definito\n", "    stato_atteso: provvisorio\n", 1),
		"id ripetuto":                         strings.Replace(buono, "  - id: caso-acme-02\n", "  - id: caso-acme-01\n", 1),
		"id assente":                          strings.Replace(buono, "  - id: caso-acme-02\n", "  - livello: banco\n", 1),
		"profilo assente":                     strings.Replace(buono, "    profilo: acme\n", "", 1),
		"testo assente":                       strings.Replace(buono, "      testo: 9123456A_2.pdf\n", "", 1),
		"atteso assente":                      strings.Replace(buono, bloccoAtteso, "", 1),
		"atteso non mappa":                    strings.Replace(buono, bloccoAtteso, "    atteso: [1]\n", 1),
		"versione assente":                    strings.Replace(buono, "versione_attesi: 1\n", "", 1),
		"casi assenti":                        strings.Replace(buono, "casi_contratto:\n", "casi_contratto_vecchi:\n", 1),
		"due documenti":                       buono + "---\nversione_attesi: 2\n",
		"file vuoto":                          "",
	}
	for nome, raw := range casi {
		t.Run(nome, func(t *testing.T) {
			if raw == buono {
				t.Fatal("la sostituzione non ha cambiato niente: il caso non prova nulla")
			}
			_, err := LeggiAttesi([]byte(raw))
			if err == nil {
				t.Fatal("attesi accettati")
			}
			if strings.Contains(err.Error(), "9123456A_2.pdf") {
				t.Errorf("l'errore riporta il testo dell'ingresso: %v", err)
			}
		})
	}
}

// TestLeggiAttesiSezioniNonLette: le sezioni che A1a non legge si accettano qualunque cosa contengano.
func TestLeggiAttesiSezioniNonLette(t *testing.T) {
	raw := strings.Replace(leggiTestdata(t, "attesi_acme.yaml"), "gate: []\n",
		"gate:\n  - voce: qualunque\n    annidata: {a: [1, 2]}\n", 1)
	if _, err := LeggiAttesi([]byte(raw)); err != nil {
		t.Fatal(err)
	}
}
