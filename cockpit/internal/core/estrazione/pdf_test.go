// L1 — l'adattatore PDF (A1b-15; piano A, 5.4.5, «Mappatura PDF»): oltre ai golden di F-PDF-1…11
// (golden_test.go), le proprietà della mappatura che un golden da solo non dice: lo stato del testo segue la
// regola di testoDelPDF (replicata, non importata) su tutti gli ingressi; un PDF senza testo, non letto,
// illeggibile o non analizzato non è mai un PDF «senza codici»; i campi del cartiglio stanno con il disegno solo
// per EntitaID, e un frammento non si lega a niente; il cartiglio di una sottoversione di prima è un indizio; i
// riquadri diventano decimi interi.
//
// Tutti i dati sono sintetici (ACME, codici di fantasia, UUID 00000000-0000-4000-8000-0000000000nn): il
// repository è pubblico.
package estrazione

import (
	"slices"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione"
)

// testoPDFACME: un payload con il testo v2 dato per frammenti e campi già scritti in JSON.
func testoPDFACME(versione, estraibile, frammenti, cartiglio, ocr string) string {
	return `{"esito":{"tipo_proposto":"disegno_2d","codice":"ACME0000000","rev":"9"},"testo_pdf":{"versione":` + versione +
		`,"estraibile":` + estraibile + `,"pagine":1,"pagine_lette":1,"caratteri":10,"troncato":false,"formato_pagina1":[1191.0,842.0],` +
		`"frammenti":` + frammenti + `,"cartiglio":` + cartiglio + `,"metadati":{"titolo":"","soggetto":"","parole_chiave":"","creatore":"","produttore":""},` +
		`"ocr":` + ocr + `,"limiti":{"pagine_max":11,"frammenti_max":200,"frammento_max":512,"caratteri_max":16384,"campi_max":24,` +
		`"ocr_soglia_pagina":20,"ocr_soglia_cartiglio":8,"ocr_pagine_max":2,"ocr_tempo_max_s":60,"ocr_dpi":300}}}`
}

const ocrNonNecessario = `{"stato":"non_necessario","motivo":"","motore":"","tentativi":[]}`

func campoACME(etichetta, letta, valore string) string {
	return `{"etichetta":"` + etichetta + `","letta":"` + letta + `","valore":"` + valore +
		`","pagina":1,"zona":"basso_destra","fonte":"nativo","riquadro":[800.0,700.0,900.0,711.0],"confidenza":null}`
}

func frammentoACME(testo string) string {
	return `{"pagina":1,"zona":"basso_destra","fonte":"nativo","testo":"` + testo + `","riquadro":[800.0,690.0,900.0,720.0],"confidenza":null}`
}

// pdfACME: il documento di un PDF sintetico con il payload dato (nil = senza fatti).
func pdfACME(t *testing.T, payload *string) evidenze.DocumentoEvidenze {
	t.Helper()
	a := allegatoACME("ACME7000100.pdf", ptr("pdf"))
	a.Sha256 = ptr(shaACME)
	var d evidenze.DocumentoEvidenze
	var err error
	if payload == nil {
		d, err = DaAllegato(a, nil, nil)
	} else {
		d, err = DaAllegato(a, fattiACME(t, shaACME, *payload), nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestLoStatoDelTestoPDFSegueLaRegolaDiOggi: su ogni ingresso PDF dei golden, lo stato che l'adattatore
// dichiara è quello che la classificazione di oggi ricava dagli stessi fatti (StatoDelTestoPDF): la regola di
// testoDelPDF è replicata, e una deriva fra le due copie fa fallire la prova (5.4.5; 5.10 n.11).
func TestLoStatoDelTestoPDFSegueLaRegolaDiOggi(t *testing.T) {
	codiceDelloStato := map[string]string{
		classificazione.TestoAssente:     CodicePDFSenzaTesto,
		classificazione.TestoNonLetto:    CodicePDFNonLetto,
		classificazione.TestoIlleggibile: CodicePDFIlleggibile,
	}
	visti := map[string]bool{}
	for _, c := range casiGolden {
		if c.tipo != "pdf" {
			continue
		}
		t.Run(c.nome, func(t *testing.T) {
			in := leggiIngressoAllegato(t, c)
			d, err := DaAllegato(in.Allegato, in.Fatti, in.Contenitore)
			if err != nil {
				t.Fatal(err)
			}
			stato := classificazione.StatoDelTestoPDF(in.Fatti.Payload)
			visti[stato] = true
			codici := codiciDi(d)
			for s, codice := range codiceDelloStato {
				if slices.Contains(codici, codice) != (s == stato) {
					t.Errorf("stato di oggi %q, diagnostiche %v: %s solo per lo stato %q", stato, codici, codice, s)
				}
			}
			testo, ok := capacitaDi(d, capTesto)
			if !ok || (stato == classificazione.TestoLetto) != (testo.Stato != statoNonDisponibile) {
				t.Errorf("stato di oggi %q, capacità del testo %+v", stato, testo)
			}
			if stato == classificazione.TestoIlleggibile && d.Qualita.Stato != statoErrore {
				t.Errorf("un PDF illeggibile ha lo stato della fonte %q", d.Qualita.Stato)
			}
		})
	}
	for _, s := range []string{classificazione.TestoLetto, classificazione.TestoAssente, classificazione.TestoNonLetto, classificazione.TestoIlleggibile} {
		if !visti[s] {
			t.Errorf("nessun ingresso dei golden ha lo stato %q: la prova non copre la regola", s)
		}
	}
}

// TestUnPDFSenzaTestoNonEUnPDFSenzaCodici (A-C09 nella parte dell'adattatore): senza fatti, senza testo
// nativo, non letto o illeggibile, il documento ha la capacità del testo non disponibile con il motivo, la sua
// diagnostica e la fonte parziale (in errore, se il PDF non si apre); elenco_pdf non è mai disponibile. Il codice dell'esito del
// worker non entra mai.
func TestUnPDFSenzaTestoNonEUnPDFSenzaCodici(t *testing.T) {
	senzaTesto := testoPDFACME("2", "false", "[]", "[]", `{"stato":"non_disponibile","motivo":"","motore":"","tentativi":[]}`)
	nonLetto := `{"esito":{"tipo_proposto":"disegno_2d","codice":"ACME0000000"}}`
	illeggibile := `{"esito":{"tipo_proposto":"disegno_2d"},"errore_pdf":"file cifrato"}`
	for _, c := range []struct {
		nome    string
		payload *string
		codice  string
		motivo  string
		stato   string
	}{
		{"senza fatti", nil, CodicePDFNonAnalizzato, "non è stato analizzato", statoParziale},
		{"senza testo nativo", &senzaTesto, CodicePDFSenzaTesto, "OCR «non_disponibile»", statoParziale},
		{"senza testo_pdf", &nonLetto, CodicePDFNonLetto, motivoTestoNonLetto, statoParziale},
		{"con errore_pdf", &illeggibile, CodicePDFIlleggibile, "file cifrato", statoErrore},
	} {
		t.Run(c.nome, func(t *testing.T) {
			d := pdfACME(t, c.payload)
			if d.Qualita.Stato != c.stato {
				t.Errorf("stato della fonte %q, atteso %q", d.Qualita.Stato, c.stato)
			}
			testo, ok := capacitaDi(d, capTesto)
			if !ok || testo.Stato != statoNonDisponibile || !strings.Contains(testo.Motivo, c.motivo) {
				t.Errorf("capacità del testo %+v, attesa non disponibile con un motivo che dice %q", testo, c.motivo)
			}
			if el, ok := capacitaDi(d, capElencoPDF); !ok || el.Stato != statoNonDisponibile {
				t.Errorf("capacità elenco_pdf %+v, attesa non disponibile", el)
			}
			if _, ok := capacitaDi(d, capContenuto); ok {
				t.Error("un PDF dichiara la capacità del solo nome: per un PDF parla la capacità del testo")
			}
			if !slices.Contains(codiciDi(d), c.codice) {
				t.Errorf("diagnostiche %v, attesa %s", codiciDi(d), c.codice)
			}
			for _, u := range d.Unita {
				if u.ID != idUnitaNome {
					t.Errorf("unità %s in un PDF senza testo letto", u.ID)
				}
			}
		})
	}
}

// TestIlCartiglioStaConIlDisegnoEIlFrammentoDaSolo (A-C02 nella parte dell'adattatore): i campi del cartiglio
// sono legati all'entità «disegno» della pagina 1 con EntitaID; ogni frammento ha un'entità «unita_isolata» sua,
// quindi la «Rev» scritta in un frammento non diventa un attributo del disegno. Un'etichetta che la foglia non
// conosce va in cartiglio.sconosciuto, con l'etichetta letta conservata; un valore vuoto non dà unità, ma conta
// nel numero dell'ID.
func TestIlCartiglioStaConIlDisegnoEIlFrammentoDaSolo(t *testing.T) {
	p := testoPDFACME("2", "true",
		"["+frammentoACME(`Rev 03`)+","+frammentoACME(`ACME7000100`)+"]",
		"["+campoACME("codice", "CODICE", "")+","+campoACME("codice", "PART N°", "ACME7000100")+","+
			campoACME("revisione", "REV", "02")+","+campoACME("finitura", "FINITURA", "ANODIZZATO")+"]",
		ocrNonNecessario)
	d := pdfACME(t, &p)

	attese := map[string][2]string{ // ID → selettore, entità
		"u:pdf:cartiglio:codice:2":    {"cartiglio.codice", idEntitaPagina1},
		"u:pdf:cartiglio:revisione:1": {"cartiglio.revisione", idEntitaPagina1},
		"u:pdf:cartiglio:finitura:1":  {"cartiglio.sconosciuto", idEntitaPagina1},
		"u:pdf:frammento:1":           {"testo_pdf", "e:pdf:frammento:1"},
		"u:pdf:frammento:2":           {"testo_pdf", "e:pdf:frammento:2"},
	}
	for id, a := range attese {
		u, ok := unitaDi(d, id)
		if !ok {
			t.Errorf("manca l'unità %s", id)
			continue
		}
		if u.Selettore.String() != a[0] || u.EntitaID != a[1] {
			t.Errorf("%s: selettore %s ed entità %s, attesi %s e %s", id, u.Selettore, u.EntitaID, a[0], a[1])
		}
	}
	if _, ok := unitaDi(d, "u:pdf:cartiglio:codice:1"); ok {
		t.Error("un campo con il valore vuoto dà un'unità")
	}
	if u, _ := unitaDi(d, "u:pdf:cartiglio:finitura:1"); u.CampoOriginale.Etichetta != "FINITURA" {
		t.Errorf("l'etichetta letta di un campo sconosciuto non è conservata: %+v", u.CampoOriginale)
	}
	if len(d.Unita) != len(attese)+1 {
		t.Errorf("%d unità, attese %d (più il nome)", len(d.Unita), len(attese))
	}
	for _, e := range d.Entita {
		if e.ID == idEntitaPagina1 && e.Tipo != tipoEntitaDisegno || e.ID != idEntitaPagina1 && e.Tipo != tipoEntitaIsolata {
			t.Errorf("entità %s di tipo %s", e.ID, e.Tipo)
		}
	}
	if d.Legami != nil {
		t.Errorf("legami %v: la mappatura PDF non ne produce, l'appartenenza sta in EntitaID (legami-1)", d.Legami)
	}
}

// TestIlCartiglioDiPrimaEUnIndizio: con un testo della sottoversione 1 i campi del cartiglio diventano unità
// testo_pdf isolate, nessuna entità «disegno», la mappatura della fonte «sconosciuta», la capacità del cartiglio
// non disponibile e pdf.testo_di_prima con gli ID di quei campi (5.4.5; giro 4, fase 4.6r).
func TestIlCartiglioDiPrimaEUnIndizio(t *testing.T) {
	p := testoPDFACME("1", "true", "[]", "["+campoACME("codice", "Part Number", "ACME7000111")+"]", ocrNonNecessario)
	d := pdfACME(t, &p)
	u, ok := unitaDi(d, "u:pdf:cartiglio:codice:1")
	if !ok || u.Selettore.String() != "testo_pdf" || u.EntitaID != "e:pdf:cartiglio:codice:1" || u.CampoOriginale.Etichetta != "Part Number" {
		t.Errorf("unità del campo di prima %+v", u)
	}
	for _, e := range d.Entita {
		if e.Tipo == tipoEntitaDisegno {
			t.Errorf("un testo di prima ha l'entità disegno %s", e.ID)
		}
	}
	if d.Qualita.Mappatura != mappaturaSconosciuta || d.Qualita.Stato != statoParziale || d.Fonte.RiferimentoFatti.SottoversionePDF != 1 {
		t.Errorf("qualità %+v, sottoversione %d", d.Qualita, d.Fonte.RiferimentoFatti.SottoversionePDF)
	}
	if c, _ := capacitaDi(d, capCartiglio); c.Stato != statoNonDisponibile {
		t.Errorf("capacità del cartiglio %+v", c)
	}
	var trovata bool
	for _, x := range d.Qualita.Diagnostiche {
		if x.Codice == CodicePDFTestoDiPrima {
			trovata = slices.Equal(x.Rif, []string{"u:pdf:cartiglio:codice:1"})
		}
	}
	if !trovata {
		t.Errorf("pdf.testo_di_prima con il riferimento al campo attesa: %+v", d.Qualita.Diagnostiche)
	}
}

// TestIRiquadriDiventanoDecimiInteri: math.Round(x*10), il mezzo lontano da zero; un riquadro che non ha
// quattro numeri non si scrive (5.4.5, «Niente numeri decimali nel documento»).
func TestIRiquadriDiventanoDecimiInteri(t *testing.T) {
	if r := riquadroInDecimi([]float64{210.25, 120.04, 180.06, 0.05}); r == nil || *r != [4]int{2103, 1200, 1801, 1} {
		t.Errorf("riquadro %v, atteso [2103 1200 1801 1]", r)
	}
	for _, r := range [][]float64{nil, {1, 2, 3}, {1, 2, 3, 4, 5}} {
		if riquadroInDecimi(r) != nil {
			t.Errorf("il riquadro %v dà dei decimi", r)
		}
	}
}
