package distintabozza

// Le prove L1 della lettura delle forme di R2 costruite dal server (direttiva 35, blocco C-3, R2 passo 6b; EM §9.2): crea
// (le quattro partenze, la sorgente e l'impronta della partenza step, la ripartenza), cambia_sorgente, riallinea_proposta
// (le scelte) e converti_da; bozza_id e versione_attesa forma per forma; i motivi obbligatori (MOTIVO_OBBLIGATORIO);
// l'impronta della richiesta.

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func formaProva(forma string, corpoForma map[string]any) map[string]any {
	m := map[string]any{"versione_attesa": 0, "prodotto_id": "00000000-0000-4000-8000-0000000b0009", "vocabolario": 2,
		"chiave_salvataggio": "00000000-0000-4000-8000-0000000c0001", forma: corpoForma}
	if forma == FormaCambiaSorgente || forma == FormaRiallineaProposta {
		m["bozza_id"] = "00000000-0000-4000-8000-0000000b0001"
		m["versione_attesa"] = 3
	} else {
		m["bozza_id"] = nil
	}
	return m
}

func sorgenteProva() map[string]any {
	return map[string]any{"allegato_id": "00000000-0000-4000-8000-0000000a0001", "gruppo": "bom"}
}

func TestV1C3LeFormeDelServerSiLeggono(t *testing.T) {
	imp := strings.Repeat("a", 64)
	// crea, partenza step, con la ripartenza
	r, err := LeggiRichiestaR2(corpoR2(t, formaProva(FormaCrea, map[string]any{"partenza": "step", "sorgente": sorgenteProva(),
		"impronta_contenuto": imp, "motivo": " codice " + eAccento + " diverso ",
		"abbandona": map[string]any{"bozza_id": "00000000-0000-4000-8000-0000000b0002", "versione_attesa": 4, "motivo": "si riparte"}})))
	if err != nil {
		t.Fatal(err)
	}
	if r.Forma != FormaCrea || r.Crea == nil || r.Crea.Partenza != PartenzaStep || r.Crea.Sorgente == nil || r.Crea.Sorgente.Gruppo != "bom" ||
		r.Crea.ImprontaContenuto != imp || r.Crea.Motivo != "codice é diverso" || r.Crea.Abbandona == nil ||
		r.Crea.Abbandona.VersioneAttesa != 4 || r.Comune.BozzaID != uuid.Nil || r.Comune.VersioneAttesa != 0 ||
		!reSha256.MatchString(r.Comune.ImprontaRichiesta) {
		t.Fatalf("crea step: %+v %+v", r, r.Crea)
	}
	// le altre partenze, senza sorgente
	for _, p := range []string{PartenzaVuota, PartenzaRegistrato, PartenzaConferma} {
		r, err := LeggiRichiestaR2(corpoR2(t, formaProva(FormaCrea, map[string]any{"partenza": p})))
		if err != nil || r.Crea.Partenza != p || r.Crea.Sorgente != nil {
			t.Fatalf("crea %s: %v %+v", p, err, r.Crea)
		}
	}
	// cambia_sorgente con le scelte fra due copie
	r, err = LeggiRichiestaR2(corpoR2(t, formaProva(FormaCambiaSorgente, map[string]any{"sorgente": sorgenteProva(), "impronta_contenuto": imp,
		"motivo": "revisione CAD", "scelte": []any{map[string]any{"chiave_bozza": "k-7", "scelta": "mantieni"}}})))
	if err != nil || r.CambiaSorgente == nil || r.CambiaSorgente.Motivo != "revisione CAD" || len(r.CambiaSorgente.Scelte) != 1 ||
		r.Comune.BozzaID == uuid.Nil || r.Comune.VersioneAttesa != 3 {
		t.Fatalf("cambia_sorgente: %v %+v", err, r)
	}
	// riallinea_proposta con le quattro scelte
	r, err = LeggiRichiestaR2(corpoR2(t, formaProva(FormaRiallineaProposta, map[string]any{"impronta_contenuto": imp, "motivo": "proposta di oggi",
		"scelte": []any{map[string]any{"chiave_bozza": "k-1", "scelta": "togli"}, map[string]any{"chiave_bozza": "k-2", "scelta": "tieni_senza_casella"},
			map[string]any{"chiave_bozza": "k-3", "scelta": "recepisci"}, map[string]any{"chiave_bozza": "k-4", "scelta": "mantieni"},
			map[string]any{"decisione_id": "ni-1", "scelta": "togli"}}})))
	if err != nil || r.Riallinea == nil || len(r.Riallinea.Scelte) != 5 || r.Riallinea.Scelte[4].DecisioneID != "ni-1" {
		t.Fatalf("riallinea_proposta: %v %+v", err, r)
	}
	// converti_da e la richiesta del servizio
	r, err = LeggiRichiestaR2(corpoR2(t, formaProva(FormaConvertiDa, map[string]any{"bozza_id": "00000000-0000-4000-8000-0000000b0003",
		"versione_attesa": 2, "motivo": "si lavora nel formato nuovo"})))
	if err != nil || r.Converti == nil {
		t.Fatalf("converti_da: %v %+v", err, r)
	}
	c := r.Conversione3Da(idDi("rfq"), idDi("utente"))
	if c.BozzaID.String() != "00000000-0000-4000-8000-0000000b0003" || c.VersioneAttesa != 2 || c.Motivo != "si lavora nel formato nuovo" ||
		c.ChiaveSalvataggio != r.Comune.ChiaveSalvataggio || c.ImprontaRichiesta != r.Comune.ImprontaRichiesta || c.ThreadID != idDi("rfq") {
		t.Fatalf("Conversione3Da: %+v", c)
	}
}

func TestV1C3LeFormeDelServerIRifiuti(t *testing.T) {
	imp := strings.Repeat("a", 64)
	casi := []struct {
		nome   string
		forma  string
		corpo  map[string]any
		cambia func(m map[string]any)
		codice string
		campo  string
		motivo string
	}{
		{"crea con bozza_id", FormaCrea, map[string]any{"partenza": "vuota"}, func(m map[string]any) { m["bozza_id"] = "00000000-0000-4000-8000-0000000b0001" },
			CodiceCorpoNonValido, "bozza_id", ""},
		{"crea con versione 1", FormaCrea, map[string]any{"partenza": "vuota"}, func(m map[string]any) { m["versione_attesa"] = 1 },
			CodiceCorpoNonValido, "versione_attesa", ""},
		{"crea con azioni", FormaCrea, map[string]any{"partenza": "vuota"}, func(m map[string]any) {
			m["azioni"] = []any{map[string]any{"azione": "nodo_tolto"}}
		}, CodiceCorpoNonValido, "richiesta", ""},
		{"partenza sconosciuta", FormaCrea, map[string]any{"partenza": "proposta"}, nil, CodiceCorpoNonValido, "crea.partenza", ""},
		{"step senza sorgente", FormaCrea, map[string]any{"partenza": "step", "impronta_contenuto": imp}, nil, CodiceCorpoNonValido, "crea.sorgente", ""},
		{"step senza impronta", FormaCrea, map[string]any{"partenza": "step", "sorgente": sorgenteProva()}, nil, CodiceCorpoNonValido,
			"crea.impronta_contenuto", ""},
		{"step con il gruppo senza", FormaCrea, map[string]any{"partenza": "step", "impronta_contenuto": imp,
			"sorgente": map[string]any{"allegato_id": "00000000-0000-4000-8000-0000000a0001", "gruppo": "senza"}}, nil, CodiceCorpoNonValido,
			"crea.sorgente.gruppo", MotivoGruppoNonImportabile},
		{"step con lo sha in maiuscolo", FormaCrea, map[string]any{"partenza": "step", "impronta_contenuto": imp,
			"sorgente": map[string]any{"allegato_id": "00000000-0000-4000-8000-0000000a0001", "gruppo": "bom", "sha256": strings.Repeat("A", 64)}}, nil,
			CodiceCorpoNonValido, "crea.sorgente.sha256", ""},
		{"registrato con la sorgente", FormaCrea, map[string]any{"partenza": "registrato", "sorgente": sorgenteProva()}, nil, CodiceCorpoNonValido,
			"crea.sorgente", ""},
		{"ripartenza senza versione", FormaCrea, map[string]any{"partenza": "vuota", "abbandona": map[string]any{
			"bozza_id": "00000000-0000-4000-8000-0000000b0002", "motivo": "x"}}, nil, CodiceCorpoNonValido, "crea.abbandona.versione_attesa", ""},
		{"campo sconosciuto in crea", FormaCrea, map[string]any{"partenza": "vuota", "importazione": imp}, nil, CodiceCorpoNonValido, "crea", ""},
		{"cambia_sorgente senza bozza", FormaCambiaSorgente, map[string]any{"sorgente": sorgenteProva(), "impronta_contenuto": imp, "motivo": "x"},
			func(m map[string]any) { delete(m, "bozza_id") }, CodiceCorpoNonValido, "bozza_id", ""},
		{"cambia_sorgente con versione 0", FormaCambiaSorgente, map[string]any{"sorgente": sorgenteProva(), "impronta_contenuto": imp, "motivo": "x"},
			func(m map[string]any) { m["versione_attesa"] = 0 }, CodiceCorpoNonValido, "versione_attesa", ""},
		{"cambia_sorgente con togli", FormaCambiaSorgente, map[string]any{"sorgente": sorgenteProva(), "impronta_contenuto": imp, "motivo": "x",
			"scelte": []any{map[string]any{"chiave_bozza": "k-1", "scelta": "togli"}}}, nil, CodiceCorpoNonValido, "cambia_sorgente.scelte[0].scelta", ""},
		{"riallinea senza impronta", FormaRiallineaProposta, map[string]any{"motivo": "x"}, nil, CodiceCorpoNonValido,
			"riallinea_proposta.impronta_contenuto", ""},
		{"scelta sconosciuta", FormaRiallineaProposta, map[string]any{"impronta_contenuto": imp, "motivo": "x",
			"scelte": []any{map[string]any{"chiave_bozza": "k-1", "scelta": "lascia"}}}, nil, CodiceCorpoNonValido, "riallinea_proposta.scelte[0].scelta", ""},
		{"scelta con nodo e decisione", FormaRiallineaProposta, map[string]any{"impronta_contenuto": imp, "motivo": "x",
			"scelte": []any{map[string]any{"chiave_bozza": "k-1", "decisione_id": "ni-1", "scelta": "togli"}}}, nil, CodiceCorpoNonValido,
			"riallinea_proposta.scelte[0]", ""},
		{"decisione tenuta", FormaRiallineaProposta, map[string]any{"impronta_contenuto": imp, "motivo": "x",
			"scelte": []any{map[string]any{"decisione_id": "ni-1", "scelta": "tieni_senza_casella"}}}, nil, CodiceCorpoNonValido,
			"riallinea_proposta.scelte[0].decisione_id", ""},
		{"due scelte per lo stesso nodo", FormaRiallineaProposta, map[string]any{"impronta_contenuto": imp, "motivo": "x",
			"scelte": []any{map[string]any{"chiave_bozza": "k-1", "scelta": "togli"}, map[string]any{"chiave_bozza": "k-1", "scelta": "recepisci"}}}, nil,
			CodiceCorpoNonValido, "riallinea_proposta.scelte[1]", ""},
		{"converti senza bozza", FormaConvertiDa, map[string]any{"versione_attesa": 2, "motivo": "x"}, nil, CodiceCorpoNonValido, "converti_da.bozza_id", ""},
		{"converti con la versione di primo livello", FormaConvertiDa, map[string]any{"bozza_id": "00000000-0000-4000-8000-0000000b0003",
			"versione_attesa": 2, "motivo": "x"}, func(m map[string]any) { m["versione_attesa"] = 2 }, CodiceCorpoNonValido, "versione_attesa", ""},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			m := formaProva(c.forma, c.corpo)
			if c.cambia != nil {
				c.cambia(m)
			}
			_, err := LeggiRichiestaR2(corpoR2(t, m))
			e := errore3(t, err)
			if e.Codice != c.codice || e.Campo != c.campo || e.Motivo != c.motivo {
				t.Fatalf("errore %+v (atteso %s su %s, motivo %q)", e, c.codice, c.campo, c.motivo)
			}
		})
	}
}

// Messaggio 46 [U] (21/10), corsia MOTIVO: le forme costruite dal server non vogliono piu' il motivo nel corpo (la
// ripartenza, il cambio della fonte, il riallineamento, la conversione); un motivo di soli spazi e' come assente. Dove lo
// schema vuole un motivo lo scrive il server (la traccia del gesto: prove L4).
func TestV1M46LeFormeDelServerSenzaMotivo(t *testing.T) {
	imp := strings.Repeat("a", 64)
	casi := []struct {
		nome  string
		forma string
		corpo map[string]any
	}{
		{"ripartenza senza motivo", FormaCrea, map[string]any{"partenza": "vuota", "abbandona": map[string]any{
			"bozza_id": "00000000-0000-4000-8000-0000000b0002", "versione_attesa": 1}}},
		{"cambia_sorgente con il motivo di soli spazi", FormaCambiaSorgente, map[string]any{"sorgente": sorgenteProva(), "impronta_contenuto": imp, "motivo": "  "}},
		{"riallinea senza motivo", FormaRiallineaProposta, map[string]any{"impronta_contenuto": imp}},
		{"converti senza motivo", FormaConvertiDa, map[string]any{"bozza_id": "00000000-0000-4000-8000-0000000b0003", "versione_attesa": 2}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			r, err := LeggiRichiestaR2(corpoR2(t, formaProva(c.forma, c.corpo)))
			if err != nil {
				t.Fatalf("rifiutata: %v", err)
			}
			switch {
			case r.Crea != nil && (r.Crea.Abbandona == nil || r.Crea.Abbandona.Motivo != ""):
				t.Fatalf("ripartenza: %+v", r.Crea.Abbandona)
			case r.CambiaSorgente != nil && r.CambiaSorgente.Motivo != "":
				t.Fatalf("cambia_sorgente: %q", r.CambiaSorgente.Motivo)
			case r.Riallinea != nil && r.Riallinea.Motivo != "":
				t.Fatalf("riallinea: %q", r.Riallinea.Motivo)
			case r.Converti != nil && r.Converti.Motivo != "":
				t.Fatalf("converti: %q", r.Converti.Motivo)
			}
		})
	}
}

// L'impronta della richiesta di una forma del server e' quella della forma canonica: gli spazi non la cambiano, un campo
// della forma si'.
func TestV1C3LImprontaDelleFormeDelServer(t *testing.T) {
	m := formaProva(FormaCrea, map[string]any{"partenza": "vuota"})
	a, err := LeggiRichiestaR2(corpoR2(t, m))
	if err != nil {
		t.Fatal(err)
	}
	b, err := LeggiRichiestaR2([]byte("  " + string(corpoR2(t, m)) + "\n"))
	if err != nil || b.Comune.ImprontaRichiesta != a.Comune.ImprontaRichiesta {
		t.Fatalf("gli spazi cambiano l'impronta: %v", err)
	}
	m["crea"] = map[string]any{"partenza": "registrato"}
	c, err := LeggiRichiestaR2(corpoR2(t, m))
	if err != nil || c.Comune.ImprontaRichiesta == a.Comune.ImprontaRichiesta {
		t.Fatalf("due corpi con la stessa impronta: %v", err)
	}
}
