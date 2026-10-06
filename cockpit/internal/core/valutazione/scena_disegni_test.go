// L1 — gli aiuti delle prove dei 2D (B5, fase 2: la validità, il gruppo e il primario, la revisione del documento): la
// famiglia ACME con la revisione anche negli id dello STEP, i fatti dei PDF (con il testo, scansionati, illeggibili, non
// letti) e delle immagini, i documenti disegno_2d confermati, costruiti in Go come li darebbe il caricatore.
package valutazione_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
)

// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un dato
// dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il giorno in cui
// un cliente cambia convenzione. Il cliente è ACME (acme.example); i codici sono di fantasia (712xxxx con il marcatore
// A o B e la revisione di una cifra: «7120200A_2.tif» nel nome del file, «7120200A2» nel cartiglio, «7120200A3» negli
// id dello STEP con la revisione), gli UUID sono 00000000-0000-4000-8000-0000000000nn. Le prove citano i requisiti (R83,
// R84, R104, T-B0-31, T-E1-08, T-E1R-05…09, PO-08…PO-12, PO-22, PO-38), mai i casi degli attesi.

// famigliaCatenaRev: la famiglia acme-catena con la revisione facoltativa anche negli id delle radici e dei nodi dello
// STEP ([base][A][rev]): uno STEP con la radice e un figlio di revisioni diverse (E1R §8.4, PO-38). È un meccanismo
// (un nodo che porta la sua revisione nell'id), mai il significato di un cliente.
func famigliaCatenaRev() grammatica.FamigliaCodice {
	f := famigliaCatena()
	for i := range f.Forme {
		if f.Forme[i].ID == "step" {
			f.Forme[i].Parti = append(f.Forme[i].Parti, grammatica.Parte{Tipo: grammatica.TipoParteRevisione, Rif: "rev-c", Min: 0, Max: 1})
		}
	}
	f.Revisioni[0].Selettori = append(f.Revisioni[0].Selettori, "radice_step.id", "nodo_step.id")
	f.Esempi = append(f.Esempi, grammatica.EsempioCodice{ID: "e-step-rev", Origine: grammatica.OrigineSintetico, Selettore: "nodo_step.id", Testo: "7120200A3",
		Atteso: grammatica.AttesoEsempio{Letture: []grammatica.LetturaAttesa{{Forma: "step", Base: "7120200", Marcatore: "A", Revisione: "3"}}}})
	return f
}

// ---- i fatti ----

// payloadPDF: i fatti di un PDF con il testo v2 del worker e i campi del cartiglio dati (etichetta → valore, in ordine);
// senza campi e con estraibile falso è una scansione senza testo nativo e senza OCR.
func payloadPDF(t *testing.T, estraibile bool, campi ...[2]string) string {
	t.Helper()
	cs := []map[string]any{}
	for _, c := range campi {
		cs = append(cs, map[string]any{"etichetta": c[0], "letta": c[0], "valore": c[1], "pagina": 1, "zona": "basso_destra",
			"fonte": "nativo", "riquadro": []float64{800, 700, 900, 711}, "confidenza": nil})
	}
	ocr := "non_necessario"
	if !estraibile {
		ocr = "non_disponibile"
	}
	raw, err := json.Marshal(map[string]any{"testo_pdf": map[string]any{
		"versione": 2, "estraibile": estraibile, "pagine": 1, "pagine_lette": 1, "caratteri": 10, "troncato": false, "formato_pagina1": []float64{1191, 842},
		"frammenti": []map[string]any{}, "cartiglio": cs, "metadati": map[string]any{"titolo": "", "soggetto": "", "parole_chiave": "", "creatore": "", "produttore": ""},
		"ocr": map[string]any{"stato": ocr, "motivo": "", "motore": "", "tentativi": []string{}},
		"limiti": map[string]any{"pagine_max": 11, "frammenti_max": 200, "frammento_max": 512, "caratteri_max": 16384, "campi_max": 24,
			"ocr_soglia_pagina": 20, "ocr_soglia_cartiglio": 8, "ocr_pagine_max": 2, "ocr_tempo_max_s": 60, "ocr_dpi": 300},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// fattiPDFCampi: i fatti di un PDF con il testo e i campi del cartiglio dati.
func fattiPDFCampi(t *testing.T, sha string, campi ...[2]string) fotorfq.Fatti {
	t.Helper()
	return fatti(t, sha, payloadPDF(t, true, campi...), nil)
}

// fattiPDFErrore: i fatti di un PDF che il worker non ha aperto (errore_pdf).
func fattiPDFErrore(t *testing.T, sha string) fotorfq.Fatti {
	t.Helper()
	return fatti(t, sha, `{"errore_pdf": "file danneggiato"}`, nil)
}

// fattiSoloEsito: i fatti con il solo esito del worker, senza testo né errore (un PDF analizzato da un worker di prima,
// un'immagine: il worker non la decodifica, LD-01).
func fattiSoloEsito(t *testing.T, sha string) fotorfq.Fatti {
	t.Helper()
	return fatti(t, sha, `{"esito": {"tipo": "altro"}}`, nil)
}

// ---- i 2D ----

// documento2D: un documento disegno_2d confermato sul componente, con l'allegato da cui viene (uuid.Nil: nessuno).
func documento2D(id, comp, allegato uuid.UUID, nome, est, sha string) fotorfq.DocumentoConfermato {
	c := comp
	d := fotorfq.DocumentoConfermato{ID: id, ThreadID: threadACME, ComponenteID: &c, Tipo: "disegno_2d", NomeFile: nome, Estensione: est, Sha256: sha,
		StatoNas: "scritto", ConfermatoDa: operatore, ConfermatoIl: dataACME.Add(time.Hour)}
	if allegato != uuid.Nil {
		d.Allegati = []uuid.UUID{allegato}
	}
	return d
}

// conFile2D: un allegato del thread con i suoi fatti (nil: nessuno) e, se c'è, il documento disegno_2d confermato sul
// componente; senza documento, una proposta aperta di tipo disegno_2d, con «assegna» sul componente se assegna non è
// uuid.Nil.
func conFile2D(th *fotorfq.Thread, a fotorfq.Allegato, f *fotorfq.Fatti, doc *fotorfq.DocumentoConfermato, assegna uuid.UUID) {
	conAllegato(th, a, f)
	if doc != nil {
		th.Documenti = append(th.Documenti, *doc)
		return
	}
	p := fotorfq.PropostaAttuale{ID: uid(0x900 + len(th.Proposte)), AllegatoID: a.ID, Tipo: "disegno_2d", Fonte: "estensione", Stato: "aperta"}
	if assegna != uuid.Nil {
		c := assegna
		p.ComponenteID = &c
	}
	th.Proposte = append(th.Proposte, p)
}

// ---- le letture dell'esito ----

// gruppoDi: il gruppo dei 2D del componente; un componente senza 2D fa fallire la prova.
func gruppoDi(t *testing.T, v valutazione.ValutazioneProdotti, comp uuid.UUID) valutazione.GruppoDisegni2D {
	t.Helper()
	for _, d := range v.Disegni {
		if d.ComponenteID == comp {
			return d.Gruppo
		}
	}
	t.Fatalf("nessun 2D per il componente %s fra %d", comp, len(v.Disegni))
	return valutazione.GruppoDisegni2D{}
}

// tuttiDi: il primario e gli alternativi del gruppo, nell'ordine.
func tuttiDi(g valutazione.GruppoDisegni2D) []valutazione.Disegno2D {
	if g.Primario == nil {
		return nil
	}
	return append([]valutazione.Disegno2D{*g.Primario}, g.Alternativi...)
}

// disegnoDi: il 2D del gruppo con quello sha256.
func disegnoDi(t *testing.T, g valutazione.GruppoDisegni2D, sha string) valutazione.Disegno2D {
	t.Helper()
	for _, d := range tuttiDi(g) {
		if d.Sha256 == sha {
			return d
		}
	}
	t.Fatalf("nessun 2D con lo sha %s nel gruppo", sha)
	return valutazione.Disegno2D{}
}

// evidenzaDi: l'evidenza di revisione del 2D con quella fonte.
func evidenzaDi(t *testing.T, d valutazione.Disegno2D, fonte string) valutazione.EvidenzaRevisione {
	t.Helper()
	for _, e := range d.EvidenzeRevisione {
		if e.Fonte == fonte {
			return e
		}
	}
	t.Fatalf("nessuna evidenza %s fra %+v", fonte, d.EvidenzeRevisione)
	return valutazione.EvidenzaRevisione{}
}

// val: il testo di un puntatore, «<nil>» se nil, per i confronti.
func val(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}
