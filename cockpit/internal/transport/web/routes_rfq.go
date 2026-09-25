// La RFQ vista dal browser: la lista di lavoro e quello che si apre da una richiesta.
//
// I gestori veri stanno nei file per tema che c'erano gia' — thread.go (il fascicolo e la riprova
// delle copie), richieste.go (la richiesta a un fornitore), allegati.go (conferma e scarto di una
// proposta), panoramica.go (la pagina Richieste). Qui c'e' il montaggio delle rotte dell'area.

package web

import (
	"net/http"
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
	// B8.3: agganciare documenti e proposte a un componente, correggere il codice di un componente
	// (fascicolo.go). La schermata del Fascicolo che le usera' e' B8.7.
	mux.HandleFunc("POST /thread/{id}/fascicolo/assegna", s.autenticato(s.assegna))
	mux.HandleFunc("POST /thread/{id}/fascicolo/componente/{cid}/codice", s.autenticato(s.correggiCodice))
	// B8.5: le proposte di struttura dagli STEP (proposte.go).
	s.registraProposte(mux)
	// B8.6: dai codici della RFQ, un componente nuovo o il ripristino di uno archiviato (codici.go).
	s.registraCodici(mux)
	// B8.7: la schermata del Fascicolo e i gesti che le mancavano (fascicolo_rotte.go).
	s.registraFascicolo(mux)
	s.registraConferma(mux)
	mux.HandleFunc("GET /cruscotto", s.autenticato(s.cruscotto))
	// La pagina Richieste, con le card delle RFQ e le schede dei prodotti (panoramica.go).
	mux.HandleFunc("GET /richieste", s.autenticato(s.richieste))
	mux.HandleFunc("GET /richieste/{id}/prodotti", s.autenticato(s.richiestaProdotti))
}

// cruscotto non esiste piu' come tabella globale (checkpoint 3R §1): era la stessa lista di
// «Richieste» con le stesse colonne e un ordinamento diverso. Il cruscotto e' quello DI UNA
// richiesta, e si apre da li'. La rotta resta e reindirizza, perche' chi aveva salvato il
// segnalibro deve trovare qualcosa, non un 404.
func (s *Server) cruscotto(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/richieste", http.StatusSeeOther)
}
