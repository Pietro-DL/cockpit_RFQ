// L1 — direttiva 39 §3, corsia motore: i quattro casi della struttura di uno STEP per un prodotto (struttura_step.go) e lo
// stato dell'analisi del file. Registri sintetici: una radice e zero o più figli. Tutto è inventato.
package propostabom_test

import (
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/valutazione"
)

// registroConFigli: un registro con la voce della radice e n figli.
func registroConFigli(n int) *importazione.Contenuto {
	c := &importazione.Contenuto{Gruppo: importazione.Gruppo{Chiave: "g", Radice: "nodo:r"}, ImprontaContenuto: "imp"}
	c.Proposta.Nodi = append(c.Proposta.Nodi, importazione.Voce{Voce: "v0", Origine: importazione.OrigineVoce{Rif: "nodo:r"}})
	for i := 0; i < n; i++ {
		c.Proposta.Nodi = append(c.Proposta.Nodi, importazione.Voce{Voce: "v" + string(rune('1'+i)), Origine: importazione.OrigineVoce{Rif: "nodo:f" + string(rune('1'+i))}})
	}
	return c
}

func TestV1D39IQuattroCasiDellaStrutturaDelloSTEP(t *testing.T) {
	casi := []struct {
		nome      string
		analisi   string
		gruppi    []propostabom.GruppoImportato
		struttura string
		partenza  string
		presa     bool
		gruppo    string
		figli     int
	}{
		{"con figli", propostabom.AnalisiPronta, []propostabom.GruppoImportato{{Chiave: "cand:a:r", Registro: registroConFigli(3)}},
			propostabom.StrutturaConFigli, propostabom.PartenzaStep, false, "cand:a:r", 3},
		{"sola radice: il vero gruppo, nessun mono_pezzo", propostabom.AnalisiPronta, []propostabom.GruppoImportato{{Chiave: "cand:a:r", Registro: registroConFigli(0)}},
			propostabom.StrutturaSolaRadice, propostabom.PartenzaStep, false, "cand:a:r", 0},
		{"nessuna struttura importabile", propostabom.AnalisiPronta, []propostabom.GruppoImportato{{Chiave: "cand:a:r"}},
			propostabom.StrutturaNessuna, propostabom.PartenzaSoloProdotto, false, "", 0},
		{"nessun gruppo per il prodotto", propostabom.AnalisiPronta, nil,
			propostabom.StrutturaNessuna, propostabom.PartenzaSoloProdotto, false, "", 0},
		{"analisi in corso: mai una prova geometrica", propostabom.AnalisiInCorso, nil,
			propostabom.StrutturaNonDeterminabile, propostabom.PartenzaSoloProdotto, true, "", 0},
		{"analisi in corso con fatti parziali importabili", propostabom.AnalisiInCorso, []propostabom.GruppoImportato{{Chiave: "cand:a:r", Registro: registroConFigli(2)}},
			propostabom.StrutturaNonDeterminabile, propostabom.PartenzaSoloProdotto, true, "", 0},
		{"analisi fallita", propostabom.AnalisiErrore, nil,
			propostabom.StrutturaNonDeterminabile, propostabom.PartenzaSoloProdotto, true, "", 0},
		{"fuori dal calcolo", propostabom.AnalisiNonValutato, nil,
			propostabom.StrutturaNonDeterminabile, propostabom.PartenzaSoloProdotto, true, "", 0},
		{"più gruppi, uno in vista", propostabom.AnalisiPronta, []propostabom.GruppoImportato{{Chiave: "cand:a:x", Registro: registroConFigli(4)},
			{Chiave: "cand:a:r", InVista: true, Registro: registroConFigli(0)}},
			propostabom.StrutturaSolaRadice, propostabom.PartenzaStep, false, "cand:a:r", 0},
		{"più gruppi, nessuno in vista: la scelta è dell'anteprima", propostabom.AnalisiPronta, []propostabom.GruppoImportato{{Chiave: "cand:a:x", Registro: registroConFigli(0)},
			{Chiave: "cand:a:y", Registro: registroConFigli(2)}},
			propostabom.StrutturaConFigli, propostabom.PartenzaStep, false, "", 2},
	}
	for _, c := range casi {
		s := propostabom.ClassificaStruttura(c.analisi, c.gruppi)
		if s.StatoAnalisi != c.analisi || s.Struttura != c.struttura || s.Partenza != c.partenza || s.PresaDAttoRichiesta != c.presa ||
			s.Gruppo != c.gruppo || s.FigliEstratti != c.figli {
			t.Errorf("%s: %+v", c.nome, s)
		}
		if c.struttura != propostabom.StrutturaConFigli && s.Testo == "" {
			t.Errorf("%s: il caso senza struttura deve dire perché", c.nome)
		}
		if c.gruppo != "" && s.ImprontaContenuto != "imp" {
			t.Errorf("%s: l'impronta del gruppo scelto manca: %+v", c.nome, s)
		}
	}
}

func TestV1D39LoStatoDellAnalisiDelFile(t *testing.T) {
	a := uuid.MustParse("00000000-0000-4000-8000-0000000d3901")
	for disp, atteso := range map[ancoraggio.Disponibilita]string{
		ancoraggio.DisponibilitaPendente:    propostabom.AnalisiInCorso,
		ancoraggio.DisponibilitaMancante:    propostabom.AnalisiInCorso,
		ancoraggio.DisponibilitaErrore:      propostabom.AnalisiErrore,
		ancoraggio.DisponibilitaIlleggibile: propostabom.AnalisiErrore,
		ancoraggio.DisponibilitaDisponibile: propostabom.AnalisiPronta,
		ancoraggio.DisponibilitaParziale:    propostabom.AnalisiPronta,
	} {
		f := valutazione.FileInterpretato{AllegatoID: a, Disponibilita: disp}
		f.Documento.Fonte.RiferimentoFatti.Tipo = "analisi_file"
		et := &valutazione.EsitoThread{File: []valutazione.FileInterpretato{f}}
		if got := propostabom.StatoAnalisiDelFile(et, a); got != atteso {
			t.Errorf("%s: %s, atteso %s", disp, got, atteso)
		}
	}
	// un file senza i fatti di un'analisi (il motore ne legge solo il nome, disponibilità parziale): non valutato, mai
	// «pronta» (l'assenza di struttura non sarebbe una prova geometrica)
	senza := &valutazione.EsitoThread{File: []valutazione.FileInterpretato{{AllegatoID: a, Disponibilita: ancoraggio.DisponibilitaParziale}}}
	senza.File[0].Documento.Fonte.RiferimentoFatti.Tipo = "nessuno"
	if got := propostabom.StatoAnalisiDelFile(senza, a); got != propostabom.AnalisiNonValutato {
		t.Errorf("senza i fatti: %s", got)
	}
	if got := propostabom.StatoAnalisiDelFile(&valutazione.EsitoThread{}, a); got != propostabom.AnalisiNonValutato {
		t.Errorf("file fuori dal calcolo: %s", got)
	}
	if got := propostabom.StatoAnalisiDelFile(nil, a); got != propostabom.AnalisiNonValutato {
		t.Errorf("senza esito: %s", got)
	}
}
