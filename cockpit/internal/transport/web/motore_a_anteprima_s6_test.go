// L1 — direttiva 39, corsia motore: S6 e lo stato dell'analisi dello STEP.
//   - lo stato dell'analisi che la pagina mostra (propostabom.StatoAnalisiDelFile, in /dati.fonte.struttura_step e in S6)
//     è lo stesso calcolo di statoAnalisiS8, quello che la rotta di S8 confronta con lo stato dichiarato (409
//     ANTEPRIMA_SUPERATA, anteprima stato_analisi): stessi valori, stessa regola per ogni disponibilità dei fatti;
//   - i codici di S6 e il loro stato HTTP sono quelli della mappa comune delle rotte della bozza (statoDelCodiceV1), salvo
//     NON_STEP, che è di S6 e di S8.
//
// Tutto è inventato.
package web

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	sorgentev1 "promatec/cockpit/internal/core/rfq/sorgente"
	"promatec/cockpit/internal/core/valutazione"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

func TestV1D39LoStatoDellAnalisiEQuelloDiS8(t *testing.T) {
	for _, c := range [][2]string{{propostabom.AnalisiPronta, sorgentev1.AnalisiPronta}, {propostabom.AnalisiInCorso, sorgentev1.AnalisiInCorso},
		{propostabom.AnalisiErrore, sorgentev1.AnalisiErrore}, {propostabom.AnalisiNonValutato, sorgentev1.AnalisiNonValutato}} {
		if c[0] != c[1] {
			t.Errorf("i valori dello stato dell'analisi differiscono: motore %q, S8 %q", c[0], c[1])
		}
	}
	a := uuid.MustParse("00000000-0000-4000-8000-0000000d3906")
	for _, disp := range []ancoraggio.Disponibilita{ancoraggio.DisponibilitaDisponibile, ancoraggio.DisponibilitaMancante, ancoraggio.DisponibilitaPendente,
		ancoraggio.DisponibilitaParziale, ancoraggio.DisponibilitaIlleggibile, ancoraggio.DisponibilitaErrore, ""} {
		for _, tipo := range []string{"analisi_file", "nessuno", ""} {
			f := valutazione.FileInterpretato{AllegatoID: a, Disponibilita: disp}
			f.Documento.Fonte.RiferimentoFatti.Tipo = tipo
			et := &valutazione.EsitoThread{File: []valutazione.FileInterpretato{f}}
			ris := &motorea_web.Risultato{EsitoThread: et}
			if m, s8 := propostabom.StatoAnalisiDelFile(et, a), statoAnalisiS8(ris, a); m != s8 {
				t.Errorf("disponibilità %q, fatti %q: motore %q, S8 %q", disp, tipo, m, s8)
			}
		}
	}
	vuoto := &valutazione.EsitoThread{}
	if m, s8 := propostabom.StatoAnalisiDelFile(vuoto, a), statoAnalisiS8(&motorea_web.Risultato{EsitoThread: vuoto}, a); m != s8 {
		t.Errorf("file fuori dal calcolo: motore %q, S8 %q", m, s8)
	}
}

func TestV1D39ICodiciDiS6(t *testing.T) {
	for _, c := range CodiciServerS6 {
		if c.Codice == codiceNonStepS6 {
			if c.Stato != 422 {
				t.Errorf("NON_STEP: %d", c.Stato)
			}
			continue
		}
		if c.Codice == codiceCalcoloOccupato || c.Codice == codiceNonConfigurato || c.Codice == codiceCalcoloScaduto {
			continue
		}
		if stato, ok := statoDelCodiceV1[c.Codice]; !ok || stato != c.Stato {
			t.Errorf("%s: S6 %d, mappa comune %d (%v)", c.Codice, c.Stato, stato, ok)
		}
	}
}

// TestV1D39LEffettoDiS6NellaFormaDiR2: effetto_sulla_bozza della rotta (EffettoS6) prende il posto di quello di
// propostabom.Anteprima nel JSON, con l'algoritmo e le caselle riscritte nella forma di R2 (allegato_prima valorizzato per
// cambio-fonte-1); senza la riconciliazione (bozza con struttura) resta cambio-fonte-1.
func TestV1D39LEffettoDiS6NellaFormaDiR2(t *testing.T) {
	prima, dopo := uuid.MustParse("00000000-0000-4000-8000-0000000d3961"), uuid.MustParse("00000000-0000-4000-8000-0000000d3962")
	eff := propostabom.EffettoCambioFonte{NodiCheEntrano: []string{"v-2"}, CaselleRiscritte: []propostabom.CasellaRiscritta{{ChiaveBozza: "k-1", Rif: "#4",
		Prima: prima, Dopo: dopo}}}
	e, err := effettoS6Da(letturaS6{}, preparazioneS6{}, eff, nil)
	if err != nil || e == nil || e.Algoritmo != distintabozza.AlgoritmoCambioFonte || len(e.CaselleRiscritte) != 1 || e.CaselleRiscritte[0].Prima == nil ||
		*e.CaselleRiscritte[0].Prima != prima || e.Abbinati != nil || e.AbbinamentiProposti != nil {
		t.Fatalf("l'effetto con struttura: %+v %v", e, err)
	}
	raw, err := json.Marshal(RispostaS6{Anteprima: propostabom.Anteprima{EffettoSullaBozza: &eff}, EffettoSullaBozza: e})
	if err != nil {
		t.Fatal(err)
	}
	var x struct {
		Effetto map[string]json.RawMessage `json:"effetto_sulla_bozza"`
	}
	if err := json.Unmarshal(raw, &x); err != nil || string(x.Effetto["algoritmo"]) != `"cambio-fonte-1"` ||
		!strings.Contains(string(x.Effetto["caselle_riscritte"]), `"allegato_prima":"`+prima.String()+`"`) {
		t.Errorf("il JSON dell'effetto: %s %v", raw, err)
	}
	if e, err := effettoS6(letturaS6{}, nil); e != nil || err != nil {
		t.Errorf("senza bozza: %+v %v", e, err)
	}
}
