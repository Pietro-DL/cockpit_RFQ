package filerfq

import (
	"errors"
	"fmt"
	"net/http"
)

// Rifiuto e' l'esito certo di un gesto o di una lettura che non ha cambiato niente: lo stato HTTP, il codice del
// contratto (EM §10.9, §10.10), il testo per chi guarda e i campi in piu' della busta (motivo, anteprima, dipendenze,
// stato e versione correnti...). Chi traduce in HTTP scrive {esito, codice, testo, …extra}: «conflitto» per un 409,
// «rifiutato» per gli altri (la busta del C3).
type Rifiuto struct {
	Stato  int
	Codice string
	Testo  string
	Extra  map[string]any
}

func (r *Rifiuto) Error() string { return fmt.Sprintf("%d %s: %s", r.Stato, r.Codice, r.Testo) }

// ComeRifiuto: il Rifiuto dentro un errore, se c'e'.
func ComeRifiuto(err error) (*Rifiuto, bool) {
	var r *Rifiuto
	if errors.As(err, &r) {
		return r, true
	}
	return nil, false
}

func rifiuto(stato int, codice, testo string, extra map[string]any) *Rifiuto {
	return &Rifiuto{Stato: stato, Codice: codice, Testo: testo, Extra: extra}
}

func extra(kv ...any) map[string]any {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		k, _ := kv[i].(string)
		m[k] = kv[i+1]
	}
	return m
}

// I rifiuti ricorrenti, con i testi del contratto.

func fuoriRFQ(testo string) *Rifiuto {
	if testo == "" {
		testo = "Niente è cambiato: il file non è di questa RFQ."
	}
	return rifiuto(http.StatusUnprocessableEntity, "FUORI_RFQ", testo, nil)
}

func corpoNonValido(campo, testo string) *Rifiuto {
	return rifiuto(http.StatusUnprocessableEntity, "CORPO_NON_VALIDO", testo, extra("campo", campo))
}

func fileVersione(stato string, versione int32) *Rifiuto {
	return rifiuto(http.StatusConflict, "FILE_VERSIONE",
		"Lo stato del file è cambiato mentre guardavi: rileggi la Sorgente.", extra("stato", stato, "versione", versione))
}

func chiaveRiusata() *Rifiuto {
	return rifiuto(http.StatusConflict, "CHIAVE_RIUSATA",
		"Questa chiave è già stata usata per un gesto diverso: niente è cambiato.", nil)
}

func rfqUnita() *Rifiuto {
	return rifiuto(http.StatusUnprocessableEntity, "RFQ_UNITA",
		"Niente è cambiato: la RFQ è stata unita a un'altra, i suoi file si gestiscono in quella.", nil)
}

func nonGestito(natura string) *Rifiuto {
	return rifiuto(http.StatusUnprocessableEntity, "NON_GESTITO",
		"Niente è cambiato: il ciclo dei file gestisce i file e gli elementi di Outlook, non «"+natura+"».", nil)
}

func fileScartato() *Rifiuto {
	return rifiuto(http.StatusUnprocessableEntity, "FILE_SCARTATO",
		"Niente è cambiato: il file è stato scartato in questa RFQ. Per riaverlo, ricaricalo.", nil)
}

func nonConfigurato(motivo, testo string) *Rifiuto {
	return rifiuto(http.StatusServiceUnavailable, "NON_CONFIGURATO", testo, extra("motivo", motivo))
}

// conservazioneNonDisponibile: «Metti in Altri file» quando la copia non si puo' scrivere (U2 §3.5): 503
// NON_CONFIGURATO con il motivo, niente scritto (nessuna riga, nessun file).
func conservazioneNonDisponibile(motivo string) *Rifiuto {
	return nonConfigurato(motivo, TestoNonDisponibile(motivo)+": niente è cambiato.")
}

// TestoNonDisponibile: perche' «Metti in Altri file» non si puo' fare su questo server, in parole (S1 e S3).
func TestoNonDisponibile(motivo string) string {
	switch motivo {
	case "nas_scrittura":
		return "La scrittura sul NAS è spenta su questo server: la cartella ALTRO sta sul NAS"
	case "nas":
		return "Il NAS non è configurato su questo server: la cartella ALTRO sta sul NAS"
	}
	return "La cartella ALTRO degli Altri file non è ancora attiva su questo server"
}
