package web

// L1 — R5 formato 3 (motore_a_bozza_v1_r5.go): la traduzione dei rifiuti in HTTP (422 BLOCCHI con i blocchi, 409
// ANTEPRIMA_SUPERATA con il riepilogo, 500 CONTROLLO_FINALE e 503 CALCOLO_OCCUPATO non registrati con la chiave, gli altri
// codici come le rotte della bozza, un errore qualunque «da verificare» con la chiave).

import (
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
)

func TestV1D39R5LaTraduzioneDeiRifiuti(t *testing.T) {
	chiave := uuid.New()
	blocchi := []distintabozza.Rilievo{{Codice: distintabozza.BloccoStepSorgenteNonConfermata, Motivo: distintabozza.SorgenteMotivoSenzaSorgente}}
	rp := &distintabozza.RiepilogoR4{FirmaAnteprima: "x"}
	for _, c := range []struct {
		nome                                string
		err                                 error
		stato                               int
		esito                               string
		codice                              string
		conChiave, conBlocchi, conRiepilogo bool
	}{
		{"blocchi", &distintabozza.RifiutoR5{Err: &distintabozza.Errore{Codice: distintabozza.CodiceBlocchi}, Blocchi: blocchi},
			http.StatusUnprocessableEntity, "rifiutato", distintabozza.CodiceBlocchi, false, true, false},
		{"anteprima superata", &distintabozza.RifiutoR5{Err: &distintabozza.Errore{Codice: distintabozza.CodiceAnteprimaSuperata, Motivo: distintabozza.AnteprimaRiepilogoBom}, Riepilogo: rp},
			http.StatusConflict, "conflitto", distintabozza.CodiceAnteprimaSuperata, false, false, true},
		{"riprova", &distintabozza.RifiutoR5{Err: &distintabozza.Errore{Codice: distintabozza.CodiceAnteprimaSuperata, Motivo: distintabozza.MotivoRiprova}},
			http.StatusConflict, "conflitto", distintabozza.CodiceAnteprimaSuperata, false, false, false},
		{"controllo finale", &distintabozza.RifiutoR5{Err: &distintabozza.Errore{Codice: distintabozza.CodiceControlloFinale, Motivo: "P4X01"}, NonRegistrato: true},
			http.StatusInternalServerError, "non_registrato", distintabozza.CodiceControlloFinale, true, false, false},
		{"calcolo occupato", &distintabozza.RifiutoR5{Err: &distintabozza.Errore{Codice: distintabozza.CodiceCalcoloOccupato}, NonRegistrato: true},
			http.StatusServiceUnavailable, "non_registrato", distintabozza.CodiceCalcoloOccupato, true, false, false},
		{"calcolo scaduto", &distintabozza.RifiutoR5{Err: &distintabozza.Errore{Codice: distintabozza.CodiceCalcoloScaduto}, NonRegistrato: true},
			http.StatusGatewayTimeout, "non_registrato", distintabozza.CodiceCalcoloScaduto, true, false, false},
		{"errore del calcolo", &distintabozza.RifiutoR5{Err: &distintabozza.Errore{Codice: distintabozza.CodiceErroreCalcolo}, NonRegistrato: true},
			http.StatusInternalServerError, "non_registrato", distintabozza.CodiceErroreCalcolo, true, false, false},
		{"bozza versione", &distintabozza.Errore{Codice: distintabozza.CodiceBozzaVersione, Motivo: distintabozza.MotivoBozzaChiusa},
			http.StatusConflict, "conflitto", distintabozza.CodiceBozzaVersione, false, false, false},
		{"nessuna modifica", &distintabozza.Errore{Codice: distintabozza.CodiceNessunaModifica},
			http.StatusUnprocessableEntity, "rifiutato", distintabozza.CodiceNessunaModifica, false, false, false},
		{"errore qualunque", errors.New("guasto"), http.StatusInternalServerError, "da_verificare", codiceErroreV1, true, false, false},
	} {
		stato, corpo := rispostaErroreR5(c.err, &chiave)
		if stato != c.stato || corpo.Esito != c.esito || corpo.Codice != c.codice || (corpo.ChiaveIdempotenza != nil) != c.conChiave ||
			(len(corpo.Blocchi) > 0) != c.conBlocchi || (corpo.Riepilogo != nil) != c.conRiepilogo {
			t.Errorf("%s: %d %+v", c.nome, stato, corpo)
		}
	}
	// i codici di R5 fuori dai cataloghi del ponte non sono nell'elenco unico (T-05 confronta solo 409 e 422)
	for _, c := range CodiciServerR5Altri {
		for _, x := range CodiciServerBozzaV1 {
			if x.Codice == c.Codice {
				t.Errorf("%s nell'elenco unico", c.Codice)
			}
		}
	}
}
