package fascicolo

// L1 — Smistamento P4: «quasi uguale». Prova 79 dell'addendum A5.8.1 (TestQuasiUgualeEVicini), anticipata
// con la F2 perche' la guardia serve al componente scritto nell'editor (U4, prova 107).

import (
	"strings"
	"testing"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/db"
)

// Prova 79: i positivi della P4 (lettera finale, separatore e suffisso, separatori, zeri in testa, suffisso
// decorativo del cliente) e i negativi che la stessa P4 fissa: 7120001/7120010 e 7120011/7120012 sono pezzi
// diversi della stessa numerazione, non vicini. Uguale non e' «quasi uguale». Simmetrica.
//
// Scostamento dalla bozza A5.8.1, DA DECIDERE (agente principale o architetto): il positivo «cifre scambiate»
// non c'e'. Lo scambio delle ultime due cifre e' proprio il negativo 7120001/7120010; uno scambio in mezzo
// (7102001/7120001, la «trasposizione di cifre» del «cosa previene» di P4) renderebbe vicini anche codici
// progressivi come 7120012/7120102. La scelta di oggi e' fissata qui sotto come negativo esplicito: se la
// decisione sara' di reintrodurre lo scambio (tranne le ultime due cifre), questo caso diventa un positivo e
// i due negativi della P4 restano.
func TestQuasiUgualeEVicini(t *testing.T) {
	r, err := regole.ValidaRegole([]byte(`{"suffissi_decorativi": ["_PRT"]}`))
	if err != nil {
		t.Fatal(err)
	}
	m := classificazione.Compila("ACME", r)
	casi := []struct {
		a, b   string
		motore *classificazione.Motore
		vicini bool
		motivo string
	}{
		{"7120001A", "7120001", nil, true, VicinoLetteraFinale},
		{"7120001ab", "7120001", nil, true, VicinoLetteraFinale},
		{"7120012_F2", "7120012", nil, true, VicinoSuffisso},
		{"7120001_1", "7120001", nil, true, VicinoSuffisso},
		{"7120001-A", "7120001", nil, true, VicinoSuffisso},
		{"712-0001", "7120001", nil, true, VicinoSeparatori},
		{"PC.PROVA", "PC-PROVA", nil, true, VicinoSeparatori},
		{"007120001", "7120001", nil, true, VicinoZeri},
		{"7120001_PRT", "7120001", m, true, VicinoSuffissoCliente},
		// negativi
		{"7120001", "7120010", nil, false, ""},
		{"7120011", "7120012", nil, false, ""},
		{"7120001", "7120002", nil, false, ""},
		{"7102001", "7120001", nil, false, ""},  // cifre adiacenti scambiate in mezzo: fuori, per la decisione in testa
		{"7120001", "71200011", nil, false, ""}, // una cifra in piu' e' un altro numero
		{"7120001A", "7120001B", nil, false, ""},
		{"7120001ABC", "7120001", nil, false, ""}, // tre lettere non sono una lettera finale
		{"7120001", "7120001", nil, false, ""},    // uguale
		{"7120001", "7120001", m, false, ""},
		{"a1b", "A1B_C", nil, false, ""}, // troppo corto perche' un prefisso conti
		{"", "7120001", nil, false, ""},
	}
	for _, c := range casi {
		for _, verso := range [][2]string{{c.a, c.b}, {c.b, c.a}} {
			ok, motivo := QuasiUguale(verso[0], verso[1], c.motore)
			if ok != c.vicini || motivo != c.motivo {
				t.Errorf("QuasiUguale(%q, %q) = %v %q, atteso %v %q", verso[0], verso[1], ok, motivo, c.vicini, c.motivo)
			}
		}
	}
}

// Vicini elenca i codici quasi uguali una volta sola, nell'ordine dato, e non elenca l'uguale.
func TestViciniDiUnCodiceScritto(t *testing.T) {
	v := Vicini("7120001A", []string{"7120010", "7120001", "7120001", "7120001A", "007120001A"}, nil)
	if len(v) != 2 || v[0].Codice != "7120001" || v[0].Motivo != VicinoLetteraFinale || v[1].Codice != "007120001A" || v[1].Motivo != VicinoZeri {
		t.Errorf("vicini di 7120001A: %+v", v)
	}
	if v := Vicini("7120099", []string{"7120001", "7120010", "7120011"}, nil); len(v) != 0 {
		t.Errorf("7120099 non ha vicini: %+v", v)
	}
}

// Prova 107 (parte pura): che cosa la RFQ sa del codice che l'operatore scrive. Uno che c'e' (anche
// archiviato, senza distinguere le maiuscole) e' quel componente; uno della richiesta si dice; uno quasi
// uguale porta i vicini; uno non ammesso ha il suo errore; uno nuovo e lontano passa.
func TestValutaCodiceNuovo(t *testing.T) {
	prodotto := componente("7120001", db.TipoComponenteFinito, false)
	archiviato := componente("7120011", db.TipoComponenteSciolto, true)
	comp := []db.Componente{prodotto, archiviato}
	richiesta := []string{"7120002"}

	if e := ValutaCodiceNuovo(" 7120011 ", comp, richiesta, nil); e.Esistente == nil || e.Esistente.ComponenteID != archiviato.ComponenteID || e.Codice != "7120011" {
		t.Errorf("7120011 c'e' gia' (archiviato): %+v", e)
	}
	if e := ValutaCodiceNuovo("7120002", comp, richiesta, nil); !e.Richiesta || e.Esistente != nil {
		t.Errorf("7120002 e' un codice della richiesta: %+v", e)
	}
	if e := ValutaCodiceNuovo("7120001A", comp, richiesta, nil); len(e.Vicini) != 1 || e.Vicini[0].Codice != "7120001" || e.Esistente != nil {
		t.Errorf("7120001A e' quasi uguale al prodotto: %+v", e)
	}
	if e := ValutaCodiceNuovo("7120002_F2", comp, richiesta, nil); len(e.Vicini) != 1 || e.Vicini[0].Codice != "7120002" {
		t.Errorf("7120002_F2 e' quasi uguale al codice della richiesta: %+v", e)
	}
	if e := ValutaCodiceNuovo("7120010", comp, richiesta, nil); len(e.Vicini) != 0 || e.Esistente != nil || e.Richiesta || e.Errore != "" {
		t.Errorf("7120010 e' un pezzo nuovo, lontano da tutti: %+v", e)
	}
	for _, sbagliato := range []string{"", "   ", "71 20001", strings.Repeat("9", classificazione.MaxCodice+1)} {
		if e := ValutaCodiceNuovo(sbagliato, comp, richiesta, nil); e.Errore == "" || e.Esistente != nil {
			t.Errorf("%q: atteso un errore, %+v", sbagliato, e)
		}
	}
}
