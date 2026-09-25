// L1 — la catena di risposta (piano C): la storia citata va in Corpo.Storia con le sue regole, e
// quello che classificazione.TagliaCatena non taglia resta nei Blocchi.
package lettura

import (
	"reflect"
	"strings"
	"testing"
)

const (
	rispostaIT = `Grazie, confermiamo l'ordine del PZ-001.

________________________________
Da: Fornitore Esempio <ordini@fornitore.example>
Inviato: lunedì 21 settembre 2026 10:15
A: Ufficio Acquisti <acquisti@acme.example>
Oggetto: RFQ PZ-001

ATTENZIONE: questa e-mail proviene da un mittente esterno all'organizzazione. Non aprire gli allegati se non conosci il mittente.

Buongiorno, inviamo l'offerta per il PZ-001.
Saluti`

	inoltroSenzaCommento = `________________________________
Da: Fornitore Esempio <ordini@fornitore.example>
Inviato: lunedì 21 settembre 2026 10:15
A: Ufficio Acquisti <acquisti@acme.example>
Oggetto: I: RFQ PZ-001

Buongiorno, inviamo l'offerta per il PZ-001.`
)

func TestCatenaLaRispostaCitataVaInStoria(t *testing.T) {
	c := verifica(t, rispostaIT, "")
	soloBlocchi(t, c.Blocchi, BloccoTesto)
	if got := testoDi(c.Blocchi[0]); got != "Grazie, confermiamo l'ordine del PZ-001." {
		t.Errorf("parte fresca: %q", got)
	}
	// La storia comincia dalla riga di underscore (la sposta lì TagliaCatena), poi l'intestazione,
	// poi il banner del messaggio citato: chiuso anche lì, perché le regole girano su ogni sezione.
	var banner *Blocco
	for i := range c.Storia {
		if c.Storia[i].Tipo == BloccoRumore {
			banner = &c.Storia[i]
		}
	}
	if banner == nil || !reflect.DeepEqual(banner.Motivi, []Motivo{MotivoBannerEsterno}) {
		t.Fatalf("il banner citato non è chiuso: %s", tipi(c.Storia))
	}
	if c.Storia[0].Tipo != BloccoSeparatore {
		t.Errorf("la storia dovrebbe aprirsi con il separatore: %s", tipi(c.Storia))
	}
	if !strings.Contains(c.Storia[len(c.Storia)-1].Grezzo, "inviamo l'offerta") {
		t.Errorf("il testo citato manca dalla storia")
	}
}

func TestCatenaInoltroSenzaCommentoNonHaStoria(t *testing.T) {
	c := verifica(t, inoltroSenzaCommento, "")
	if len(c.Storia) != 0 {
		t.Fatalf("un inoltro senza commento ha perso il testo nella storia: %s", tipi(c.Storia))
	}
	var tutto strings.Builder
	for _, b := range c.Blocchi {
		tutto.WriteString(b.Grezzo)
	}
	if !strings.Contains(tutto.String(), "inviamo l'offerta") {
		t.Errorf("il testo inoltrato manca dai blocchi")
	}
}

func TestCatenaUnCicloDiLavorazioneNonEUnaStoria(t *testing.T) {
	const corpo = "Riepilogo lavorazione del PZ-001:\nDa: tornitura\nA: rettifica\n\nGrazie"
	c := verifica(t, corpo, "")
	soloBlocchi(t, c.Blocchi, BloccoTesto)
	if len(c.Storia) != 0 || !strings.Contains(testoDi(c.Blocchi[0]), "Da: tornitura\nA: rettifica") {
		t.Errorf("il ciclo di lavorazione è stato spostato: %+v", c)
	}
}
