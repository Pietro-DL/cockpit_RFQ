package grammatica

import (
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// L1 — le capacità riservate in A1 (A1a-CAP; v3 §2; C-10; R20 c; D-12; R48 A, P-16): i tre esiti della
// validazione. Un valore ignoto è un errore; un elemento noto ma riservato dà capacita.non_supportata
// (avviso) e il resto della grammatica resta valido; il selettore «storia» è attivo come gli altri, senza
// diagnosi.
//
// I clienti di questi test sono inventati: la grammatica è quella ACME di valida_test.go, e ogni caso ne
// aggiunge o ne cambia un pezzo solo.

// TestA1aCAPVersioneCapacita fissa la versione dell'elenco dei riservati: cambiarla vuol dire riscrivere
// questa prova con il titolo «Riscritta per …» (par.3.4.1).
func TestA1aCAPVersioneCapacita(t *testing.T) {
	if VersioneCapacita != "capacita-a1-1" {
		t.Fatalf("VersioneCapacita = %q", VersioneCapacita)
	}
}

// TestA1aCAPRiservatoConDiagnosi: ogni capacità riservata dà l'avviso con il suo percorso, nessun errore;
// una forma che ha solo selettori riservati non è attiva, quindi non chiede un esempio positivo.
func TestA1aCAPRiservatoConDiagnosi(t *testing.T) {
	casi := []struct {
		nome     string
		raw      []byte
		percorso string
	}{
		{"forma su elenco_pdf.codice", variante(t,
			`{"id": "cartiglio", "selettori": ["cartiglio.codice"]`,
			`{"id": "elenco", "selettori": ["elenco_pdf.codice"], "stato": "attiva", "completa": true, "parti": [{"tipo": "base", "min": 1, "max": 1}]},
        {"id": "cartiglio", "selettori": ["cartiglio.codice"]`),
			"famiglie_codice[acme-prefisso].forme[elenco].selettori[0]"},
		{"forma su voce_archivio", variante(t,
			`{"id": "cartiglio", "selettori": ["cartiglio.codice"]`,
			`{"id": "voce", "selettori": ["voce_archivio"], "stato": "attiva", "completa": true, "parti": [{"tipo": "base", "min": 1, "max": 1}]},
        {"id": "cartiglio", "selettori": ["cartiglio.codice"]`),
			"famiglie_codice[acme-prefisso].forme[voce].selettori[0]"},
		{"forma sul titolo del cartiglio", variante(t,
			`{"id": "cartiglio", "selettori": ["cartiglio.codice"]`,
			`{"id": "titolo", "selettori": ["cartiglio.titolo"], "stato": "attiva", "completa": true, "parti": [{"tipo": "base", "min": 1, "max": 1}]},
        {"id": "cartiglio", "selettori": ["cartiglio.codice"]`),
			"famiglie_codice[acme-prefisso].forme[titolo].selettori[0]"},
		{"forma sul nome di un nodo STEP", variante(t,
			`{"id": "cartiglio", "selettori": ["cartiglio.codice"]`,
			`{"id": "nodo", "selettori": ["nodo_step.nome"], "stato": "attiva", "completa": true, "parti": [{"tipo": "base", "min": 1, "max": 1}]},
        {"id": "cartiglio", "selettori": ["cartiglio.codice"]`),
			"famiglie_codice[acme-prefisso].forme[nodo].selettori[0]"},
		{"selettore generico", variante(t,
			`{"id": "cartiglio", "selettori": ["cartiglio.codice"]`,
			`{"id": "tutto", "selettori": ["cartiglio.*"], "stato": "attiva", "completa": true, "parti": [{"tipo": "base", "min": 1, "max": 1}]},
        {"id": "cartiglio", "selettori": ["cartiglio.codice"]`),
			"famiglie_codice[acme-prefisso].forme[tutto].selettori[0]"},
		{"decorazione formato", variante(t,
			`{"id": "stato", "tipo": "stato_pdm", "letterali": ["IN_WORK"], "selettori": ["nome_file"]}`,
			`{"id": "stato", "tipo": "stato_pdm", "letterali": ["IN_WORK"], "selettori": ["nome_file"]},
        {"id": "foglio-a", "tipo": "formato", "letterali": ["A3"], "selettori": ["nome_file"]}`),
			"famiglie_codice[acme-documento].decorazioni[foglio-a]"},
		{"riferimenti RFQ", variante(t,
			`"famiglie_codice": [`,
			`"riferimenti_rfq": [{"id": "rdo", "stato": "riservata", "segmenti": [{"nome": "n", "pattern": "[0-9]{4}", "identitario": true}], "selettori": ["oggetto"]}],
  "famiglie_codice": [`),
			"riferimenti_rfq"},
		{"qualificatori testuali", variante(t,
			`"famiglie_codice": [`,
			`"qualificatori_testuali": [{"id": "ricambio", "stato": "riservata", "ambito": "riga", "testi": ["come ricambio"], "selettori": ["corpo"], "valore": {"destinazione": "ricambio"}}],
  "famiglie_codice": [`),
			"qualificatori_testuali"},
		{"proiezione non in coda", variante(t,
			`"segmenti_mancanti": ["T"]`, `"segmenti_mancanti": ["b"]`),
			"famiglie_codice[acme-punti].forme[parziale].segmenti_mancanti"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			ds := diagnosiDi(t, c.raw, limitiACME())
			if e := errori(ds); len(e) > 0 {
				t.Fatalf("un riservato non è un errore:%s", elenca(e))
			}
			d := richiedi(t, ds, CodiceCapacitaNonSupportata, c.percorso)
			if d.Gravita != evidenze.GravitaAvviso || d.Natura != evidenze.NaturaCapacita {
				t.Fatalf("riservato: %s %s, atteso avviso di capacità", d.Gravita, d.Natura)
			}
		})
	}
}

// TestA1aCAPProiezioneNonInCodaNonAttiva: la forma parziale riservata non è attiva: il suo esempio non è più
// obbligatorio, e senza quell'esempio la grammatica resta senza errori.
func TestA1aCAPProiezioneNonInCodaNonAttiva(t *testing.T) {
	raw := variante(t, `"segmenti_mancanti": ["T"]`, `"segmenti_mancanti": ["b"]`,
		`"atteso": {"letture": [{"forma": "parziale", "mancanti": ["T"]}]}`, `"atteso": {"nessuna": true}`)
	if e := errori(diagnosiDi(t, raw, limitiACME())); len(e) > 0 {
		t.Fatalf("errori inattesi:%s", elenca(e))
	}
}

// TestA1aCAPErroreNonRiservato: un valore che il vocabolario non conosce non è una capacità riservata: è un
// errore, e la grammatica non si attiva.
func TestA1aCAPErroreNonRiservato(t *testing.T) {
	ds := diagnosiDi(t, variante(t, `"tipo": "stato_pdm"`, `"tipo": "timbro"`), limitiACME())
	d := richiedi(t, ds, CodiceEnumIgnoto, "famiglie_codice[acme-documento].decorazioni[stato].tipo")
	if d.Gravita != evidenze.GravitaErrore {
		t.Fatalf("enum ignoto con gravità %s", d.Gravita)
	}
	for _, x := range ds {
		if x.Codice == CodiceCapacitaNonSupportata {
			t.Fatalf("un valore ignoto non è una capacità riservata:%s", elenca(ds))
		}
	}
	// Un contesto fuori elenco con «.*» non è un generico: lo rifiuta la foglia.
	ds = diagnosiDi(t, variante(t, `"selettori": ["cartiglio.codice"]`, `"selettori": ["figlio_step.*"]`), limitiACME())
	richiedi(t, ds, evidenze.CodiceSelettoreNonAmmesso, "famiglie_codice[acme-prefisso].forme[cartiglio].selettori[0]")
}

// TestA1aCAPStoriaAttiva: una forma, un affisso e un esempio sul selettore «storia» sono attivi, senza
// nessuna diagnosi (R48 A, P-16). La grammatica ACME ne ha già: la forma della mail vale su corpo e storia.
func TestA1aCAPStoriaAttiva(t *testing.T) {
	raw := variante(t, `{"id": "e4", "origine": "sintetico", "selettore": "corpo"`, `{"id": "e4", "origine": "sintetico", "selettore": "storia"`)
	ds := diagnosiDi(t, raw, limitiACME())
	for _, d := range ds {
		if d.Codice != CodiceRiserva {
			t.Fatalf("«storia» deve essere attivo, senza diagnosi:%s", elenca(ds))
		}
	}
	if !selettoreGenerico("storia.*") || selettoreGenerico("storia") || selettoreGenerico("figlio_step.*") {
		t.Fatal("selettoreGenerico sbagliato")
	}
}
