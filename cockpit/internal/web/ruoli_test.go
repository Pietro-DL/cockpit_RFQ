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

// W15 (L1) — la rail porta la sezione *Admin* solo a chi può aprirla.
//
// L'elenco delle voci non è scritto qui: si legge da `navPer`, la stessa funzione che costruisce
// la rail. Così una voce aggiunta domani entra automaticamente nel test — un elenco copiato a mano
// resterebbe fermo, e la voce nuova sarebbe l'unica scoperta proprio perché è nuova.
func TestW15LaBarraSiCostruiscePerRuolo(t *testing.T) {
	s := serverTest(t)
	rendi := func(ruolo db.RuoloUtente) string {
		t.Helper()
		var buf bytes.Buffer
		v := vista{Utente: &db.Utente{Sigla: "XX", Nome: "Nome Cognome", Ruolo: ruolo}, Titolo: "Inbox",
			Dati: "", Admin: ruolo == db.RuoloUtenteAdmin, Stato: &statoUI{}}
		if err := s.pagine["vietato.html"].ExecuteTemplate(&buf, "layout", v); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	operatore, admin := rendi(db.RuoloUtenteOperatore), rendi(db.RuoloUtenteAdmin)

	// tutto ciò che `navPer` promette a un ruolo, la pagina di quel ruolo lo mostra davvero
	for ruolo, html := range map[db.RuoloUtente]string{db.RuoloUtenteOperatore: operatore, db.RuoloUtenteAdmin: admin} {
		for _, sez := range navPer(&db.Utente{Ruolo: ruolo}) {
			for _, voce := range sez.Voci {
				if !strings.Contains(html, `href="`+voce.Href+`"`) {
					t.Errorf("%s: la rail non porta %s (%s)", ruolo, voce.Href, voce.Etichetta)
				}
			}
		}
	}
	// e niente della sezione Admin compare all'operatore
	for _, sez := range navPer(&db.Utente{Ruolo: db.RuoloUtenteAdmin}) {
		if sez.Nome != "Admin" {
			continue
		}
		if !strings.Contains(admin, ">Admin<") {
			t.Error("la rail dell'admin non mostra l'intestazione della sezione")
		}
		if strings.Contains(operatore, ">Admin<") {
			t.Error("la rail dell'operatore mostra l'intestazione della sezione Admin")
		}
		for _, voce := range sez.Voci {
			if strings.Contains(operatore, `href="`+voce.Href+`"`) {
				t.Errorf("la rail dell'operatore porta %s", voce.Href)
			}
		}
	}
	// le tre operative ci sono per tutti: sono il lavoro, non l'amministrazione
	for _, voce := range []string{"/inbox", "/richieste", "/cruscotto"} {
		for nome, html := range map[string]string{"operatore": operatore, "admin": admin} {
			if !strings.Contains(html, `href="`+voce+`"`) {
				t.Errorf("%s: manca %s", nome, voce)
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
