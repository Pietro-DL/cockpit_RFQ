package distintabozza

// Il vassoio «Da ricollocare» (EM §3.7; MAT §4.1; PM23 T-16): le preassociazioni tolte da un gesto strutturale e non ancora
// ricollocate o lasciate. Non sta nello snapshot: si ricava dal giornale con una regola sola, la stessa per il server (R1)
// e per la pagina (distinta_unica.mjs, vassoioDa), provata sui casi condivisi transport/web/e2e/distinta_unica_casi.json:
//   - per ogni allegato (direttiva 35, C-2: la regola e' la stessa, la chiave e' l'occorrenza e non piu' la proposta di
//     documento) conta l'ultima azione efficace fra file_svincolato, file_preassociato e file_lasciato: senza quelle
//     annullate con annulla, e con quelle ripetute con rifai (una ripetizione rende di nuovo efficace l'evento, in quel
//     punto del giornale);
//   - l'allegato e' nel vassoio se quell'azione e' un file_svincolato con un motivo diverso da utente (dopo.motivo, PE-04).

import (
	"encoding/json"
	"sort"
)

// AzioniDelVassoio sono le azioni del giornale che la regola del vassoio guarda (VassoioDa): le altre non la cambiano.
// La lettura delle bozze di tutti i prodotti (bozze_rfq.go) legge dal database solo questi eventi.
var AzioniDelVassoio = []string{AzFileSvincolato, AzFilePreassociato, AzFileLasciato, AzAnnulla, AzRifai}

// EventoGiornale e' un evento del giornale come lo leggono le regole del vassoio e della pila d'annullamento.
type EventoGiornale struct {
	N           int
	Azione      string
	ChiaveBozza string
	Prima, Dopo json.RawMessage
}

// VoceVassoio e' un file nel vassoio, {allegato_id, motivo, chiave_bozza}: l'occorrenza, il motivo dello svincolo e il
// nodo da cui e' uscita.
type VoceVassoio struct {
	AllegatoID  string `json:"allegato_id"`
	Motivo      string `json:"motivo"`
	ChiaveBozza string `json:"chiave_bozza"`
}

// GiornaleDa: gli eventi letti nella forma delle regole del giornale.
func GiornaleDa(ee []EventoLetto) []EventoGiornale {
	out := make([]EventoGiornale, 0, len(ee))
	for _, e := range ee {
		out = append(out, EventoGiornale{N: e.N, Azione: e.Azione, ChiaveBozza: e.ChiaveBozza, Prima: e.Prima, Dopo: e.Dopo})
	}
	return out
}

func numeroEvento(dopo json.RawMessage) (int, bool) {
	var x struct {
		N *int `json:"n"`
	}
	if json.Unmarshal(dopo, &x) != nil || x.N == nil {
		return 0, false
	}
	return *x.N, true
}

// VassoioDa ricava il vassoio dal giornale, con la regola in testa al file. Le voci in ordine di allegato.
func VassoioDa(eventi []EventoGiornale) []VoceVassoio {
	perN := map[int]EventoGiornale{}
	var efficaci []int // gli eventi dei file in ordine di efficacia
	for _, e := range eventi {
		switch e.Azione {
		case AzFileSvincolato, AzFilePreassociato, AzFileLasciato:
			perN[e.N] = e
			efficaci = append(efficaci, e.N)
		case AzAnnulla, AzRifai:
			n, ok := numeroEvento(e.Dopo)
			if !ok {
				continue
			}
			if _, file := perN[n]; !file {
				continue
			}
			if e.Azione == AzRifai {
				efficaci = append(efficaci, n)
				continue
			}
			for i := len(efficaci) - 1; i >= 0; i-- {
				if efficaci[i] == n {
					efficaci = append(efficaci[:i], efficaci[i+1:]...)
					break
				}
			}
		}
	}
	ultima := map[string]EventoGiornale{}
	for _, n := range efficaci {
		e := perN[n]
		var x struct {
			AllegatoID string `json:"allegato_id"`
		}
		src := e.Dopo
		if e.Azione == AzFileSvincolato {
			src = e.Prima
		}
		if json.Unmarshal(src, &x) != nil || x.AllegatoID == "" {
			continue
		}
		ultima[x.AllegatoID] = e
	}
	var out []VoceVassoio
	for id, e := range ultima {
		if e.Azione != AzFileSvincolato {
			continue
		}
		var d struct {
			Motivo string `json:"motivo"`
		}
		_ = json.Unmarshal(e.Dopo, &d)
		if !SvincoloNelVassoio(d.Motivo) {
			continue
		}
		var p struct {
			ChiaveBozza string `json:"chiave_bozza"`
		}
		_ = json.Unmarshal(e.Prima, &p)
		chiave := p.ChiaveBozza
		if chiave == "" {
			chiave = e.ChiaveBozza
		}
		out = append(out, VoceVassoio{AllegatoID: id, Motivo: d.Motivo, ChiaveBozza: chiave})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AllegatoID < out[j].AllegatoID })
	if out == nil {
		out = []VoceVassoio{}
	}
	return out
}
