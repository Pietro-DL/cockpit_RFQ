package web

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// TestScenarioMGRigaInbox: lo scenario «MG» (classificazione.TestScenarioMG, docs/domande/scenario_MG_28-09.md
// fuori da git), la riga dell'allegato nel pannello dell'Inbox. Prova da fare del giro 4.
//
// Nel caso vero il PDF d'assieme con tre righe nell'elenco particolari mostrava nella colonna il nome del file
// (qui «ACME-030P7120102.pdf») e accanto «score 85» con la regola del cartiglio, che era la lettura di UN
// FIGLIO: la colonna (il Riepilogo, che con le fonti discordi tiene la lettura del nome) e lo score (della
// dimensione, cioe' della lettura vincente) parlano di due letture diverse, e la riga non dice che discordano
// (web/templates/frammenti.html, "allegato_riga").
func TestScenarioMGRigaInbox(t *testing.T) {
	m := classificazione.Compila("ACME", regole.Regole{FamiglieCodice: []regole.FamigliaCodice{{
		Regex:        `(?i)(?:^|[^0-9A-Za-z])(?P<codice>P?712[0-9]{4})(?:-(?P<rev>rev\d{2}))?(?:[^0-9A-Za-z]|$)`,
		Descrizione:  "codici ACME: 712 + 4 cifre, eventuale P davanti, revisione -revNN nei file",
		Esempio:      "7120001-rev01",
		RevNelCodice: true,
	}}})
	testo := "Pos. Part Number Descrizione Q.ty\n1 7120121 MONTANTE M 1\n2 7120120 SQUADRA N 1\n3 7120135 SPINA R 1\n" +
		"Rev Mod. N. Type Modification object / Descrizione modifica\n00 ACME-P003 A Prima emissione\n" +
		"Family: Part Nr: Descrizione: Description:\n-- 7120102 TELAIO CENTRALE\n"
	bd := []float64{560, 400, 830, 590}
	tp := worker.TestoPDF{Versione: 1, Estraibile: true, Pagine: 1, PagineLette: 1, FormatoPagina1: []float64{842, 595},
		Caratteri: len(testo), OCR: worker.OCRPDF{Stato: worker.OCRNonNecessario},
		Frammenti: []worker.FrammentoPDF{{Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Testo: testo, Riquadro: bd}},
		Cartiglio: []worker.CampoCartiglio{{Etichetta: worker.CampoCodice, Letta: "Part Number", Valore: "7120121", Pagina: 1,
			Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Riquadro: bd}}}
	fatti, _ := json.Marshal(map[string]any{"cartiglio": true, "termini_trovati": []string{"SCALA"},
		"fonti": map[string]string{"tipo": "termini_pdf"}, "testo_pdf": tp})
	nome := "ACME-030P7120102.pdf"
	v := classificazione.Valuta(classificazione.IngressoFile{Da: classificazione.DaAnalisi, NomeFile: nome, Direzione: "entrata",
		Motore: m, Esito: &classificazione.Esito{Tipo: "disegno_2d", Fonte: "cartiglio"}, Fatti: fatti})
	dett, r := classificazione.ConValutazione(json.RawMessage(`{}`), v, time.Date(2026, 9, 28, 17, 54, 0, 0, time.UTC))
	p := &db.DocumentoProposta{TipoProposto: db.TipoDocumento(r.Tipo), Codice: txtT(r.Codice), Rev: txtT(r.Rev),
		Confidenza: int16(r.Confidenza), Fonte: db.FonteProposta(r.Fonte), Dettagli: dett, Stato: db.StatoPropostaAperta}
	a := AllegatoUI{Allegato: db.Allegato{NomeFile: nome, Natura: db.NaturaAllegatoFile, Stato: db.StatoAllegatoAnalizzato}, Proposta: p}
	var buf bytes.Buffer
	if err := serverTest(t).pagine["inbox.html"].ExecuteTemplate(&buf, "allegato_riga", rigaAllegato{A: a}); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	proposta := regexp.MustCompile(`(?s)<td>\s*<span class="chip [^"]*"[^>]*>[^<]*</span>(.*?)</td>`).FindStringSubmatch(html)
	if proposta == nil {
		t.Fatalf("la cella della proposta non si trova:\n%s", html)
	}
	cella := strings.Join(strings.Fields(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(proposta[1], " ")), " ")
	t.Logf("oggi la cella della proposta dice: %q (colonna %q score %d; dimensione %q score %d %s %s)",
		cella, r.Codice, r.Confidenza, v.Codice.Valore, v.Codice.Score, v.Codice.Regola, v.Codice.Stato)

	// Invariante: la colonna e' la lettura del nome (A5.14.3, U7: con le fonti discordi non vince nessuno).
	t.Run("con le fonti discordi la colonna e' il nome del file", func(t *testing.T) {
		if v.Codice.Stato == classificazione.StatoDiscorde && !strings.Contains(cella, "ACME-030P7120102") {
			t.Errorf("la cella non mostra la lettura del nome: %q", cella)
		}
	})

	t.Run("da fare: la riga mostra lo score della lettura in colonna e dice che le fonti discordano", func(t *testing.T) {
		coerente := !strings.Contains(cella, "score 85") || strings.Contains(cella, v.Codice.Valore)
		detto := strings.Contains(cella, "discord")
		if coerente && detto {
			t.Logf("soddisfatta: togliere il salto e lasciare l'asserzione")
			return
		}
		t.Skipf("da fare giro 4: accanto al codice in colonna lo score e la regola di QUELLA lettura, e il chip «fonti discordi» "+
			"(frammenti.html, allegato_riga: {{.Codice.String}} con {{score $v.Codice}} · {{$v.Codice.Regola}}) (oggi: %q)", cella)
	})
}
