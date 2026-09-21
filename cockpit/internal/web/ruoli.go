package web

import (
	"net/http"

	"promatec/cockpit/internal/platform/db"
)

// I ruoli — chi può fare che cosa (voce 6.9)
//
// I quattro ruoli esistono nello schema dalla 0001 (`ruolo_utente`): admin, operatore, tecnico,
// consultazione. Fino a qui però nessuno di loro voleva dire niente: le sette rotte /admin/* erano
// soltanto `autenticato`, e l'unico confronto con `admin` stava dentro la generazione del pacchetto.
// Un operatore che scriveva /admin/job nella barra degli indirizzi entrava, riaccodava e annullava.
//
// Da qui in avanti la decisione si prende in UN posto solo — `rango` — e si applica da UNA funzione
// sola, `soloRuolo`. Vale anche per la barra di navigazione: la testata chiede a `almeno` le stesse
// cose che chiede il wrapper, così la voce che si vede e la porta che si apre non possono
// allontanarsi. Nascondere una voce di menu non è un controllo: è un suggerimento.
//
// # L'ordine, e perché è un ordine e non ancora una matrice
//
// consultazione < operatore < tecnico < admin. È fedele a come i ruoli sono stati definiti: il
// tecnico è l'operatore più le azioni della fattibilità e dell'albero (che oggi non esistono ancora,
// quindi oggi tecnico e operatore possono esattamente le stesse cose), e l'admin è l'operatore più
// le schermate tecniche.
//
// La MATRICE ruolo × azione — quale singola azione appartiene a chi — è rimandata di proposito: con
// due utenti configurati e metà delle schermate ancora da scrivere, una tabella scritta oggi
// sarebbe una tabella da riscrivere. Il giorno in cui serve, il posto dove scriverla è questo file,
// e il segno che l'ordine lineare non basta più sarà un ruolo che può qualcosa che il ruolo sopra
// di lui non può.
func rango(r db.RuoloUtente) int {
	switch r {
	case db.RuoloUtenteAdmin:
		return 4
	case db.RuoloUtenteTecnico:
		return 3
	case db.RuoloUtenteOperatore:
		return 2
	case db.RuoloUtenteConsultazione:
		return 1
	}
	// Un ruolo che non riconosciamo non è «come un operatore»: è zero. In database non ci può
	// arrivare (è un enum) e in configurazione l'avvio si ferma prima, ma se un domani un valore
	// nuovo comparisse senza passare di qui, deve chiudere le porte, non aprirle.
	return 0
}

// almeno è il predicato: questo utente arriva almeno a questo ruolo?
func almeno(u *db.Utente, minimo db.RuoloUtente) bool {
	return u != nil && rango(u.Ruolo) >= rango(minimo)
}

// soloRuolo è il wrapper: sotto il ruolo minimo, 403. Va messo DENTRO `autenticato`, che è ciò che
// carica l'utente nel contesto — `soloAdmin` lo fa nell'ordine giusto una volta per tutte.
func (s *Server) soloRuolo(minimo db.RuoloUtente, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !almeno(utenteDa(r.Context()), minimo) {
			s.nega(w, r, "Questa schermata è dell'amministratore.")
			return
		}
		h(w, r)
	}
}

// soloAdmin è l'unica forma in uso oggi: autenticazione, poi ruolo. L'ordine conta — il controllo
// del ruolo legge l'utente che l'autenticazione ha messo nel contesto.
func (s *Server) soloAdmin(h http.HandlerFunc) http.HandlerFunc {
	return s.autenticato(s.soloRuolo(db.RuoloUtenteAdmin, h))
}

// metodoCheScrive dice se questa richiesta cambia qualcosa. GET, HEAD e OPTIONS non cambiano niente
// (è la stessa distinzione su cui si regge la protezione CSRF della voce 2.5).
func metodoCheScrive(metodo string) bool {
	switch metodo {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// nega è l'unico posto da cui esce un 403 di autorizzazione, e risponde in due forme perché arriva
// da due strade diverse: un clic dentro la pagina (HTMX, che si aspetta un frammento da innestare —
// del testo grezzo finirebbe in mezzo alla schermata) e un indirizzo scritto a mano nella barra,
// che merita una pagina intera con la testata e la via d'uscita.
//
// Finisce anche nel log: un accesso negato è un'informazione, sia quando è un tentativo sia — molto
// più spesso — quando è un collega che non capisce perché non vede una voce di menu.
func (s *Server) nega(w http.ResponseWriter, r *http.Request, motivo string) {
	u := utenteDa(r.Context())
	sigla, ruolo := "?", db.RuoloUtente("?")
	if u != nil {
		sigla, ruolo = u.Sigla, u.Ruolo
	}
	s.Log.Warn("accesso negato", "utente", sigla, "ruolo", ruolo, "metodo", r.Method, "percorso", r.URL.Path)
	v := vista{Utente: u, Titolo: "Non autorizzato", Dati: motivo, Frammento: r.Header.Get("HX-Request") == "true"}
	nome := "vietato"
	if !v.Frammento {
		nome = "layout"
		if u != nil {
			v.Admin = almeno(u, db.RuoloUtenteAdmin)
			v.Stato = s.stato(r.Context(), sessioneDa(r.Context()))
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	if err := s.pagine["vietato.html"].ExecuteTemplate(w, nome, v); err != nil {
		s.Log.Error("template", "pagina", "vietato.html", "err", err)
	}
}
