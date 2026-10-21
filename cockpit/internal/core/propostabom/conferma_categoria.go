package propostabom

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/valutazione"
)

// L'adattatore della conferma della categoria (V1, emendamento U2 §6.4.2, «Il motore la legge dall'ultima conferma
// valida»; addendum ad A1c su LD-19): dal registro delle decisioni dell'ultima conferma valida di un prodotto
// (conferma_strutturale.decisioni) le ConfermaCategoria che la valutazione aspetta da A1c (A1C §2.6). È il primo
// produttore di un ingresso già definito: non cambia nessuna regola. La sola classe della grammatica non esenta dal 2D:
// qui entrano solo le decisioni di una persona con esito «conferma»; il rifiuto non produce niente.
//
// Le regole [T]:
//   - solo una conferma valida (la validità la decide confermabom sullo stato della RFQ letto nella stessa fotografia):
//     con una conferma non più valida nessuna ConfermaCategoria (base [T] di U2-Q18: le minuterie tornano a chiedere il
//     2D finché una nuova conferma non le riporta);
//   - le voci del registro con tipo «conferma_categoria», esito «conferma» e una categoria del vocabolario chiuso
//     (fabbricato | commerciale | minuteria); le altre il motore le scarterebbe (conferma_non_ammessa), qui non entrano;
//   - il componente: soggetto.componente_id, oppure soggetto.chiave_bozza risolta con la mappa della conferma; una voce
//     che non si risolve non entra;
//   - l'autore e il momento individuali: «autore» (un UUID, o {utente_id}) e «quando» della voce, che R5 prende
//     dall'evento che ha introdotto la decisione o, per le riportate (da_conferma), dal registro della conferma che
//     l'ha introdotta (U2-P02, CAT-07). Una voce senza autore o senza momento prende quelli della conferma, e lo si
//     dichiara nel resoconto come limite: l'autore individuale lo scrive R5.

// I valori della voce del registro letti qui.
const (
	tipoConfermaCategoria  = "conferma_categoria"
	esitoConfermaCategoria = "conferma"
)

// categorieAmmesse: il vocabolario chiuso di conferma_categoria (U2 §6.4.2), lo stesso della valutazione.
var categorieAmmesse = map[string]bool{
	valutazione.CategoriaFabbricato: true, valutazione.CategoriaCommerciale: true, valutazione.CategoriaMinuteria: true,
}

// voceCategoria: i campi di una voce del registro delle decisioni che l'adattatore legge.
type voceCategoria struct {
	Tipo     string `json:"tipo"`
	Soggetto struct {
		ChiaveBozza  string `json:"chiave_bozza"`
		ComponenteID string `json:"componente_id"`
	} `json:"soggetto"`
	Categoria string          `json:"categoria"`
	Esito     string          `json:"esito"`
	Autore    json.RawMessage `json:"autore"`
	Quando    string          `json:"quando"`
}

// ConfermaDelProdotto: l'ultima conferma di un prodotto come la legge l'adattatore: se è valida, chi e quando, il
// registro delle decisioni e la mappa chiave_bozza → componente.
type ConfermaDelProdotto struct {
	Valida       bool
	ConfermataDa uuid.UUID
	ConfermataIl time.Time
	Decisioni    []byte
	Mappa        []byte
}

// ConfermeCategoria: le ConfermaCategoria delle ultime conferme valide dei prodotti di un thread, in ordine di
// componente, categoria, momento. Un registro che non si legge non dà niente (mai un'esenzione per errore).
func ConfermeCategoria(conferme []ConfermaDelProdotto) []valutazione.ConfermaCategoria {
	var out []valutazione.ConfermaCategoria
	for _, c := range conferme {
		if !c.Valida || len(c.Decisioni) == 0 {
			continue
		}
		var voci []voceCategoria
		if err := json.Unmarshal(c.Decisioni, &voci); err != nil {
			continue
		}
		mappa := map[string]string{}
		if len(c.Mappa) > 0 {
			_ = json.Unmarshal(c.Mappa, &mappa)
		}
		for _, v := range voci {
			if v.Tipo != tipoConfermaCategoria || v.Esito != esitoConfermaCategoria || !categorieAmmesse[v.Categoria] {
				continue
			}
			comp := v.Soggetto.ComponenteID
			if comp == "" && v.Soggetto.ChiaveBozza != "" {
				comp = mappa[v.Soggetto.ChiaveBozza]
			}
			id, err := uuid.Parse(comp)
			if err != nil {
				continue
			}
			k := valutazione.ConfermaCategoria{ComponenteID: id, Categoria: v.Categoria, Da: c.ConfermataDa, Il: c.ConfermataIl}
			if da, ok := autoreDellaVoce(v.Autore); ok {
				k.Da = da
			}
			if il, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(v.Quando)); err == nil {
				k.Il = il
			}
			out = append(out, k)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ComponenteID != out[j].ComponenteID {
			return out[i].ComponenteID.String() < out[j].ComponenteID.String()
		}
		if out[i].Categoria != out[j].Categoria {
			return out[i].Categoria < out[j].Categoria
		}
		return out[i].Il.Before(out[j].Il)
	})
	return out
}

// autoreDellaVoce: l'autore di una voce, un UUID oppure {utente_id}.
func autoreDellaVoce(b json.RawMessage) (uuid.UUID, bool) {
	if len(b) == 0 {
		return uuid.Nil, false
	}
	var s string
	if json.Unmarshal(b, &s) == nil {
		id, err := uuid.Parse(s)
		return id, err == nil
	}
	var o struct {
		UtenteID string `json:"utente_id"`
	}
	if json.Unmarshal(b, &o) == nil {
		id, err := uuid.Parse(o.UtenteID)
		return id, err == nil
	}
	return uuid.Nil, false
}
