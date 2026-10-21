package web

// L1 — S8 e la revoca (motore_a_sorgente_v1.go): i codici propri e il loro stato, lo stato dell'analisi dello STEP.

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	sorgentev1 "promatec/cockpit/internal/core/rfq/sorgente"
	"promatec/cockpit/internal/core/valutazione"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

func TestV1S8ICodiciDelleRotte(t *testing.T) {
	for _, c := range CodiciServerS8 {
		stato, corpo := rispostaErroreBozzaV1(&distintabozza.Errore{Codice: c.Codice, Messaggio: "x"})
		if stato != c.Stato || corpo.Codice != c.Codice {
			t.Errorf("%s: %d %+v", c.Codice, stato, corpo)
		}
		if (stato == http.StatusConflict) != (corpo.Esito == "conflitto") {
			t.Errorf("%s: esito %q con lo stato %d", c.Codice, corpo.Esito, stato)
		}
	}
}

func TestV1S8LoStatoDellAnalisi(t *testing.T) {
	a := uuid.New()
	if g := statoAnalisiS8(nil, a); g != sorgentev1.AnalisiNonValutato {
		t.Fatalf("senza calcolo: %s", g)
	}
	ris := &motorea_web.Risultato{EsitoThread: &valutazione.EsitoThread{}}
	for _, c := range []struct {
		d      ancoraggio.Disponibilita
		atteso string
	}{
		{ancoraggio.DisponibilitaPendente, sorgentev1.AnalisiInCorso}, {ancoraggio.DisponibilitaMancante, sorgentev1.AnalisiInCorso},
		{ancoraggio.DisponibilitaErrore, sorgentev1.AnalisiErrore}, {ancoraggio.DisponibilitaIlleggibile, sorgentev1.AnalisiErrore},
		{ancoraggio.DisponibilitaDisponibile, sorgentev1.AnalisiPronta}, {ancoraggio.DisponibilitaParziale, sorgentev1.AnalisiPronta},
	} {
		// aggancio della corsia motore (direttiva 39): un file letto dall'analizzatore ha il riferimento ai fatti; senza, lo
		// stato è non_valutato (propostabom.StatoAnalisiDelFile, prova in motore_a_anteprima_s6_test.go)
		f := valutazione.FileInterpretato{AllegatoID: a, Disponibilita: c.d}
		f.Documento.Fonte.RiferimentoFatti.Tipo = "analisi_file"
		ris.EsitoThread.File = []valutazione.FileInterpretato{{AllegatoID: uuid.New()}, f}
		if g := statoAnalisiS8(ris, a); g != c.atteso {
			t.Errorf("%s: %s, atteso %s", c.d, g, c.atteso)
		}
	}
	ris.EsitoThread.File = nil
	if g := statoAnalisiS8(ris, a); g != sorgentev1.AnalisiNonValutato {
		t.Fatalf("file fuori dal calcolo: %s", g)
	}
}

// Le rotte di S8 e della revoca sono rotte dedicate del ponte (la quarta parte del mondo chiuso di TestPonteLaListaChiusa):
// le aggiunge questo file, senza toccare la tabella della corsia interfaccia.
func init() {
	ponteDedicate["/thread/{id}/sorgente/step/{allegato}/conferma-sorgente"] = "S8, direttiva 39 §2 (motore_a_sorgente_v1.go; corsia persistenza)"
	ponteDedicate["/thread/{id}/sorgente/step/{allegato}/revoca-sorgente"] = "revoca della sorgente, direttiva 39 §2 (motore_a_sorgente_v1.go)"
}
