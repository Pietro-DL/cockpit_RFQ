// La RFQ vista dal browser: la lista di lavoro e quello che si apre da una richiesta.
//
// I gestori veri stanno nei file per tema che c'erano gia' — thread.go (il fascicolo e la riprova
// delle copie), richieste.go (la richiesta a un fornitore), allegati.go (conferma e scarto di una
// proposta). Qui ci sono le due liste e il montaggio delle rotte dell'area.

package web

import (
	"net/http"

	"promatec/cockpit/internal/platform/db"
)

// registraRFQ monta le rotte della RFQ: il fascicolo di un thread, le richieste ai fornitori, le
// proposte di allegato da confermare o scartare, e le due liste di lavoro.
func (s *Server) registraRFQ(mux *http.ServeMux) {
	mux.HandleFunc("POST /thread/{id}/richiesta", s.autenticato(s.nuovaRichiestaFornitore))
	mux.HandleFunc("POST /thread/{id}/richiesta/{rid}/annulla", s.autenticato(s.annullaRichiestaFornitore))
	mux.HandleFunc("GET /thread/{id}", s.autenticato(s.thread))
	mux.HandleFunc("POST /thread/{id}/riprova-copie", s.autenticato(s.riprovaCopie))
	mux.HandleFunc("POST /proposta/{id}/conferma", s.autenticato(s.conferma))
	mux.HandleFunc("POST /proposta/{id}/scarta", s.autenticato(s.scarta))
	mux.HandleFunc("GET /cruscotto", s.autenticato(s.cruscotto))
	mux.HandleFunc("GET /richieste", s.autenticato(s.richieste))
}

// cruscotto non esiste piu' come tabella globale (checkpoint 3R §1): era la stessa lista di
// «Richieste» con le stesse colonne e un ordinamento diverso. Il cruscotto e' quello DI UNA
// richiesta, e si apre da li'. La rotta resta e reindirizza, perche' chi aveva salvato il
// segnalibro deve trovare qualcosa, non un 404.
func (s *Server) cruscotto(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/richieste", http.StatusSeeOther)
}

// richieste è la lista di lavoro: le RFQ aperte, quelle dei clienti con il peso più alto in cima.
//
// L'ordinamento è `cliente.peso` e poi la scadenza, e si ferma lì. Il PUNTEGGIO di priorità
// dell'addendum 2 — quello con i pesi 40/25/20/15 e le soglie — non esiste ancora, e le sue
// regole vanno rese esplicite prima di essere codificate: inventarne una versione provvisoria qui
// vorrebbe dire che l'ordine di lavoro di tutti dipende da una formula che nessuno ha approvato.
func (s *Server) richieste(w http.ResponseWriter, r *http.Request) {
	righe, err := db.New(s.Pool).ListRichieste(r.Context(), 200)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.rendi(w, r, "richieste.html", "richieste_tabella", "Richieste", righe)
}
