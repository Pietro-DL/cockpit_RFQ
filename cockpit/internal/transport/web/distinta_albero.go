package web

// L'albero proposto della Distinta, in JSON, di sola lettura (giro 4, fase 4.4a.1a; domande 27 = A, 28 = A): i dati
// che la pagina della fase 4.4a.3 mostrera' e il riepilogo della bozza dell'operatore. Niente qui scrive: la conferma
// dell'albero e' la fase 4.4a.1b.
//
//   - GET  /thread/{id}/distinta/albero            l'albero proposto (fascicolo.AlberoProposto)
//   - GET  /thread/{id}/distinta/albero/riepilogo  il riepilogo della bozza vuota: l'albero accettato cosi' com'e'
//   - POST /thread/{id}/distinta/albero/riepilogo  il riepilogo della bozza dell'operatore (il corpo: la bozza)
//
// La POST del riepilogo e' una LETTURA, dichiarata qui e nella prova 98 (TestNessunaGetScrive). Perche' una POST: la
// bozza e' un documento JSON che puo' essere grande (migliaia di legami in una RFQ con tanti pezzi), e in un indirizzo
// avrebbe il limite di lunghezza delle richieste, finirebbe nei registri e nella cronologia del browser, e potrebbe
// stare in una cache; HTTP non ha ancora un metodo di sola lettura con un corpo che il browser sappia mandare. La POST
// non chiama prepara, non prende il lucchetto della RFQ, non apre una transazione e non fa girare il flusso di F8
// (niente dopoIlGesto): legge con il pool, come le GET. Il middleware autenticato la nega a chi consulta (per quel
// ruolo ogni POST e' una scrittura): chi consulta legge l'albero e il riepilogo della bozza vuota con le GET, e la bozza
// e' di chi conferma, che e' almeno operatore.
//
// Tutte le risposte hanno Cache-Control: no-store: l'albero cambia con ogni decisione, e una risposta vecchia sarebbe
// una bozza disegnata su un albero che non c'e' piu'.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// maxBozzaByte e' il limite del JSON della bozza: come la struttura voluta dell'editor, cinquemila voci ci stanno.
const maxBozzaByte = maxStrutturaByte

func (s *Server) registraAlberoProposto(mux *http.ServeMux) {
	mux.HandleFunc("GET /thread/{id}/distinta/albero", s.autenticato(s.alberoProposto))
	mux.HandleFunc("GET /thread/{id}/distinta/albero/riepilogo", s.autenticato(s.riepilogoAlbero))
	// una LETTURA con la bozza nel corpo (vedi sopra): niente prepara, niente lucchetto, niente flusso dopo
	mux.HandleFunc("POST /thread/{id}/distinta/albero/riepilogo", s.autenticato(s.riepilogoAlbero))
}

// erroreAlbero e' la risposta a una bozza che non si legge: il rifiuto, in parole.
type erroreAlbero struct {
	Errore string `json:"errore"`
}

// alberoProposto: GET /thread/{id}/distinta/albero. JSON.
func (s *Server) alberoProposto(w http.ResponseWriter, r *http.Request) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", http.StatusBadRequest)
		return
	}
	a, err := fascicolo.LeggiAlberoProposto(r.Context(), db.New(s.Pool), thread, s.Analizzatore)
	if err != nil {
		s.rispostaAlberoErrore(w, thread, err)
		return
	}
	scriviJSONNoStore(w, http.StatusOK, a)
}

// riepilogoAlbero: GET (la bozza vuota) o POST (la bozza nel corpo) /thread/{id}/distinta/albero/riepilogo. JSON. Una
// bozza che non si legge risponde 422 con il rifiuto.
func (s *Server) riepilogoAlbero(w http.ResponseWriter, r *http.Request) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", http.StatusBadRequest)
		return
	}
	var bozza *fascicolo.BozzaAlbero
	if r.Method == http.MethodPost {
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBozzaByte))
		if err != nil {
			scriviJSONNoStore(w, http.StatusRequestEntityTooLarge, erroreAlbero{Errore: "la bozza mandata è troppo grande"})
			return
		}
		b, err := fascicolo.LeggiBozza(raw)
		if err != nil {
			scriviJSONNoStore(w, http.StatusUnprocessableEntity, erroreAlbero{Errore: spiegaErrore(err)})
			return
		}
		bozza = &b
	}
	_, rp, err := leggiRiepilogo(r.Context(), s, thread, bozza)
	if err != nil {
		var rf rifiuto
		if errors.As(err, &rf) {
			scriviJSONNoStore(w, http.StatusUnprocessableEntity, erroreAlbero{Errore: spiegaErrore(err)})
			return
		}
		s.rispostaAlberoErrore(w, thread, err)
		return
	}
	scriviJSONNoStore(w, http.StatusOK, rp)
}

// leggiRiepilogo legge il riepilogo con il pool: nessuna transazione, nessun lucchetto (e' una lettura).
func leggiRiepilogo(ctx context.Context, s *Server, thread uuid.UUID, b *fascicolo.BozzaAlbero) (fascicolo.AlberoProposto, fascicolo.RiepilogoAlbero, error) {
	return fascicolo.LeggiRiepilogo(ctx, db.New(s.Pool), thread, s.Analizzatore, b)
}

// rispostaAlberoErrore: una RFQ che non c'e' e' un 404; il resto e' un errore del server, scritto nel registro.
func (s *Server) rispostaAlberoErrore(w http.ResponseWriter, thread uuid.UUID, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "RFQ non trovata", http.StatusNotFound)
		return
	}
	s.Log.Error("albero proposto: lettura non riuscita", "rfq", thread, "err", err)
	http.Error(w, "lettura dell'albero non riuscita", http.StatusInternalServerError)
}

// scriviJSONNoStore scrive la risposta JSON che non va in nessuna cache.
func scriviJSONNoStore(w http.ResponseWriter, stato int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(stato)
	_ = json.NewEncoder(w).Encode(v)
}
