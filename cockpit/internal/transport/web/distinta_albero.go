package web

// L'albero proposto della Distinta, in JSON (giro 4, fasi 4.4a.1a e 4.4a.1b; domande 27 = A, 28 = A): i dati che la
// pagina della fase 4.4a.3 mostrera', il riepilogo della bozza dell'operatore e la sua conferma. Solo la conferma
// scrive.
//
//   - GET  /thread/{id}/distinta/albero            l'albero proposto (fascicolo.AlberoProposto)
//   - GET  /thread/{id}/distinta/albero/riepilogo  il riepilogo della bozza vuota: l'albero accettato cosi' com'e'
//   - POST /thread/{id}/distinta/albero/riepilogo  il riepilogo della bozza dell'operatore (il corpo: la bozza)
//   - POST /thread/{id}/distinta/albero/conferma   «Conferma l'albero» (il corpo: la firma del riepilogo e la bozza)
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
// La conferma (fase 4.4a.1b) e' un gesto che scrive: una transazione, dentro fascicolo.ConfermaAlbero sotto prepara (il
// lucchetto della RFQ, D26), che ricalcola il riepilogo e si rifiuta senza scrivere niente se la firma non e' quella che
// la persona ha visto o se il riepilogo ha dei blocchi. Dopo una conferma riuscita il flusso di F8 rifa' le
// destinazioni della RFQ (dopoIlGesto): i pezzi confermati cambiano quali file si preselezionano. Chi consulta non la
// manda (403, il middleware autenticato).
//
// Tutte le risposte hanno Cache-Control: no-store, anche quelle d'errore (fase 4.4a.1b, dalla verifica della 4.4a.1a):
// l'albero cambia con ogni decisione, e una risposta vecchia sarebbe una bozza disegnata su un albero che non c'e' piu'.

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
	// il gesto che scrive (fase 4.4a.1b), con il flusso di F8 dopo
	mux.HandleFunc("POST /thread/{id}/distinta/albero/conferma", s.autenticato(s.dopoIlGesto(s.confermaAlbero)))
}

// erroreAlbero e' la risposta a una bozza che non si legge: il rifiuto, in parole.
type erroreAlbero struct {
	Errore string `json:"errore"`
}

// esitoAlbero e' la risposta a una conferma riuscita: la frase per l'operatore.
type esitoAlbero struct {
	Testo string `json:"testo"`
}

// alberoProposto: GET /thread/{id}/distinta/albero. JSON.
func (s *Server) alberoProposto(w http.ResponseWriter, r *http.Request) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		erroreNoStore(w, "id non valido", http.StatusBadRequest)
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
		erroreNoStore(w, "id non valido", http.StatusBadRequest)
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

// confermaAlbero: POST /thread/{id}/distinta/albero/conferma, il corpo JSON {"firma": "<la firma del riepilogo>",
// "bozza": {…}} (fascicolo.CorpoConferma). Tutto o niente: 200 con la frase, 422 con il rifiuto (niente e' cambiato).
// Una RFQ che non c'e' e' anche lei un rifiuto (422, «RFQ non trovata»), come per gli altri gesti del Fascicolo: la
// conferma blocca la RFQ per prima cosa (prepara), e il rifiuto arriva prima di ogni lettura. Le GET rispondono 404.
func (s *Server) confermaAlbero(w http.ResponseWriter, r *http.Request) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		erroreNoStore(w, "id non valido", http.StatusBadRequest)
		return
	}
	// la bozza, piu' la firma e le parole del corpo
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBozzaByte+4096))
	if err != nil {
		scriviJSONNoStore(w, http.StatusRequestEntityTooLarge, erroreAlbero{Errore: "la bozza mandata è troppo grande"})
		return
	}
	b, firma, err := fascicolo.LeggiConferma(raw)
	if err != nil {
		scriviJSONNoStore(w, http.StatusUnprocessableEntity, erroreAlbero{Errore: spiegaErrore(err)})
		return
	}
	ctx := r.Context()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		s.rispostaAlberoErrore(w, thread, err)
		return
	}
	defer tx.Rollback(ctx)
	msg, err := fascicolo.ConfermaAlbero(ctx, db.New(tx), thread, utenteDa(ctx).UtenteID, s.Analizzatore, b, firma)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		var rf rifiuto
		if errors.As(err, &rf) {
			scriviJSONNoStore(w, http.StatusUnprocessableEntity, erroreAlbero{Errore: spiegaErrore(err)})
			return
		}
		s.rispostaAlberoErrore(w, thread, err)
		return
	}
	scriviJSONNoStore(w, http.StatusOK, esitoAlbero{Testo: msg})
}

// leggiRiepilogo legge il riepilogo con il pool: nessuna transazione, nessun lucchetto (e' una lettura).
func leggiRiepilogo(ctx context.Context, s *Server, thread uuid.UUID, b *fascicolo.BozzaAlbero) (fascicolo.AlberoProposto, fascicolo.RiepilogoAlbero, error) {
	return fascicolo.LeggiRiepilogo(ctx, db.New(s.Pool), thread, s.Analizzatore, b)
}

// rispostaAlberoErrore: una RFQ che non c'e' e' un 404; il resto e' un errore del server, scritto nel registro.
func (s *Server) rispostaAlberoErrore(w http.ResponseWriter, thread uuid.UUID, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		erroreNoStore(w, "RFQ non trovata", http.StatusNotFound)
		return
	}
	s.Log.Error("albero proposto: lettura o conferma non riuscita", "rfq", thread, "err", err)
	erroreNoStore(w, "l'albero non si legge o non si conferma: riprova", http.StatusInternalServerError)
}

// erroreNoStore e' http.Error per le rotte dell'albero: la risposta d'errore non va in nessuna cache, come le altre.
func erroreNoStore(w http.ResponseWriter, testo string, stato int) {
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, testo, stato)
}

// scriviJSONNoStore scrive la risposta JSON che non va in nessuna cache.
func scriviJSONNoStore(w http.ResponseWriter, stato int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(stato)
	_ = json.NewEncoder(w).Encode(v)
}
