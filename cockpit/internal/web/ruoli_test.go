package web

import (
	"bytes"
	"strings"
	"testing"

	"promatec/cockpit/internal/db"
)

// L1 — l'ordine dei ruoli e la barra di navigazione costruita su quell'ordine (voce 6.9, W15).
//
// L'autorizzazione vera è L4 (W1, rbac_db_test.go): qui si prova la decisione, cioè la funzione da
// cui dipendono sia il wrapper delle rotte sia le voci di menu. Che siano la stessa funzione è il
// punto: una barra che si calcola per conto suo è una barra che prima o poi offre una porta chiusa.

func TestOrdineDeiRuoli(t *testing.T) {
	// admin > tecnico > operatore > consultazione, e un ruolo che non conosciamo sta sotto a tutti.
	if !(rango(db.RuoloUtenteAdmin) > rango(db.RuoloUtenteTecnico) &&
		rango(db.RuoloUtenteTecnico) >= rango(db.RuoloUtenteOperatore) &&
		rango(db.RuoloUtenteOperatore) > rango(db.RuoloUtenteConsultazione) &&
		rango(db.RuoloUtenteConsultazione) > rango(db.RuoloUtente("capo"))) {
		t.Fatal("l'ordine dei ruoli non è quello dichiarato")
	}

	casi := []struct {
		ruolo  db.RuoloUtente
		admin  bool
		scrive bool // arriva almeno a operatore: può premere i pulsanti
	}{
		{db.RuoloUtenteAdmin, true, true},
		{db.RuoloUtenteTecnico, false, true},
		{db.RuoloUtenteOperatore, false, true},
		{db.RuoloUtenteConsultazione, false, false},
		{db.RuoloUtente("capo"), false, false}, // in database non ci arriva: è un enum
	}
	for _, c := range casi {
		u := &db.Utente{Sigla: "XX", Ruolo: c.ruolo}
		if almeno(u, db.RuoloUtenteAdmin) != c.admin {
			t.Errorf("%s: admin=%v, atteso %v", c.ruolo, !c.admin, c.admin)
		}
		if almeno(u, db.RuoloUtenteOperatore) != c.scrive {
			t.Errorf("%s: operatore=%v, atteso %v", c.ruolo, !c.scrive, c.scrive)
		}
	}

	// Nessun utente (sessione scaduta fra il controllo e l'uso) non è «un operatore qualsiasi».
	if almeno(nil, db.RuoloUtenteConsultazione) {
		t.Error("un utente che non c'è ha superato un controllo di ruolo")
	}
}

// I metodi che scrivono sono quelli che la protezione CSRF chiama non sicuri: è la stessa
// distinzione, e deve restare la stessa o `consultazione` scriverebbe da una porta e non dall'altra.
func TestQualiMetodiScrivono(t *testing.T) {
	for _, m := range []string{"GET", "HEAD", "OPTIONS"} {
		if metodoCheScrive(m) {
			t.Errorf("%s risulta un metodo che scrive", m)
		}
	}
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if !metodoCheScrive(m) {
			t.Errorf("%s non risulta un metodo che scrive", m)
		}
	}
}

// W15 (L1) — la barra di navigazione porta le tre voci tecniche solo a chi può aprirle.
func TestW15LaBarraSiCostruiscePerRuolo(t *testing.T) {
	s := serverTest(t)
	tecniche := []string{"/admin/job", "/admin/scarti", "/admin/postazioni"}
	operative := []string{"/inbox", "/cruscotto"}
	for _, admin := range []bool{false, true} {
		var buf bytes.Buffer
		v := vista{Utente: &db.Utente{Sigla: "XX", Nome: "Nome Cognome", Ruolo: db.RuoloUtenteOperatore}, Titolo: "Inbox",
			Dati: "", Admin: admin, Stato: &statoUI{}}
		if admin {
			v.Utente.Ruolo = db.RuoloUtenteAdmin
		}
		if err := s.pagine["vietato.html"].ExecuteTemplate(&buf, "layout", v); err != nil {
			t.Fatal(err)
		}
		html := buf.String()
		for _, voce := range operative {
			if !strings.Contains(html, `href="`+voce+`"`) {
				t.Errorf("admin=%v: la barra non porta %s", admin, voce)
			}
		}
		for _, voce := range tecniche {
			if c := strings.Contains(html, `href="`+voce+`"`); c != admin {
				t.Errorf("admin=%v: %s presente=%v", admin, voce, c)
			}
		}
	}
}

// Il 403 non è testo grezzo: dentro la pagina (HTMX) è un frammento che si può innestare, fuori è
// una pagina intera con la via d'uscita. In tutti e due i casi dice il ruolo che serve.
func TestLaPaginaDelDivietoDiceIlMotivo(t *testing.T) {
	s := serverTest(t)
	for _, nome := range []string{"vietato", "layout"} {
		var buf bytes.Buffer
		v := vista{Utente: &db.Utente{Sigla: "FP", Nome: "Nome Cognome", Ruolo: db.RuoloUtenteOperatore},
			Titolo: "Non autorizzato", Dati: "Questa schermata è dell'amministratore.", Stato: &statoUI{}}
		v.Frammento = nome == "vietato"
		if err := s.pagine["vietato.html"].ExecuteTemplate(&buf, nome, v); err != nil {
			t.Fatal(err)
		}
		html := buf.String()
		for _, atteso := range []string{"admin", "operatore", "amministratore"} {
			if !strings.Contains(html, atteso) {
				t.Errorf("%s: non dice %q: %s", nome, atteso, html)
			}
		}
	}
}
