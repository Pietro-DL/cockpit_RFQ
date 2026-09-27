package web

// L1 — Smistamento F2 (addendum A5, R1): via i gesti che fanno di un'evidenza un componente o un documento.
// La pagina della RFQ (la futura Comunicazioni) e il pannello dell'Inbox non decidono niente; nessuna
// domanda sul nuovo STEP strutturale ha una risposta gia' scelta; il triage non spunta da solo un codice
// visto solo nel nome di un allegato.

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// gestiCheDecidono sono i segni di un gesto che decide su un file o su un codice: la conferma o lo scarto
// di una proposta, l'aggiunta di un codice, l'accettazione o lo scarto di un nodo, il ripristino.
var gestiCheDecidono = []string{"/proposta/", "/codice/aggiungi", "/nodo/", "/accetta", ">Scarta<", "Conferma…", "Conferma → NAS",
	"/ripristina", "+ Prodotto", "+ Assieme", "+ Particolare", "Accetta la proposta"}

// Prova 93: la pagina della RFQ e il pannello dell'Inbox non hanno gesti che decidono. Gli allegati ci
// sono, con una proposta aperta e il file in staging (prima avevano «Conferma… → NAS» e «Scarta»); il
// pannello dei codici c'e', con una riga per situazione (prima: «+», «Accetta la proposta», «Ripristina»).
// Restano Scarica, Anteprima, Apri in Inbox.
func TestLaComunicazioniNonHaGestiCheDecidono(t *testing.T) {
	s := serverTest(t)
	md, _, _ := datiSintetici()
	thd, _ := pannelloSintetico(0)
	if !thd.Messaggi[0].Allegati[0].Confermabile() || !md.Allegati[0].Confermabile() {
		t.Fatal("la scena deve avere un allegato confermabile: e' quello che prima portava «Conferma… → NAS»")
	}
	var buf bytes.Buffer
	if err := s.pagine["thread.html"].ExecuteTemplate(&buf, "thread_corpo", vista{Dati: thd, Frammento: true}); err != nil {
		t.Fatal(err)
	}
	pagina := buf.String()
	buf.Reset()
	if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, "messaggio_pannello", vista{Dati: md, Frammento: true}); err != nil {
		t.Fatal(err)
	}
	pannello := buf.String()
	for nome, html := range map[string]string{"pagina della RFQ": pagina, "pannello dell'Inbox": pannello} {
		for _, vietato := range gestiCheDecidono {
			if strings.Contains(html, vietato) {
				t.Errorf("%s: c'e' ancora %q", nome, vietato)
			}
		}
		for _, atteso := range []string{"1234567A_4.pdf", "Scarica selezionati", "Anteprima"} {
			if !strings.Contains(html, atteso) {
				t.Errorf("%s: manca %q (la tabella degli allegati resta)", nome, atteso)
			}
		}
	}
	for _, atteso := range []string{`id="codice-77720517"`, `id="codice-77760000"`, "Apri in Inbox"} {
		if !strings.Contains(pagina, atteso) {
			t.Errorf("pagina della RFQ: manca %q", atteso)
		}
	}

	// il frammento sa ancora disegnare i due gesti: e' la vista che li spegne (Gesti), non il template rotto
	buf.Reset()
	v := allegatiVista{Allegati: md.Allegati, MessaggioID: md.M.MessaggioID, Agganciato: true, NScaricabili: 1, Gesti: true}
	if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, "allegati_tabella", v); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Conferma → NAS") || !strings.Contains(buf.String(), ">Scarta<") {
		t.Error("con Gesti il frammento offre Conferma e Scarta: il campo della vista e' quello che li toglie")
	}
}

// Prova 95 (la parte `nuovo_riferimento`, P26): nessun modulo di sostituzione ha la risposta «il nuovo
// diventa lo STEP strutturale» gia' scelta, in nessun template; e il server, senza risposta, rifiuta di
// sostituire lo STEP strutturale.
func TestNessunaSceltaStrutturaleNasceSpuntata(t *testing.T) {
	file, err := filepath.Glob(filepath.Join("..", "..", "..", "web", "templates", "*.html"))
	if err != nil || len(file) == 0 {
		t.Fatalf("template non trovati: %v", err)
	}
	campo := regexp.MustCompile(`<input[^>]*name="nuovo_riferimento[^"]*"[^>]*>`)
	trovati := 0
	for _, f := range file {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range campo.FindAllString(string(b), -1) {
			trovati++
			if strings.Contains(m, "checked") {
				t.Errorf("%s: %s nasce spuntato", filepath.Base(f), m)
			}
		}
	}
	if trovati != 10 { // cinque moduli, «sì» e «no»
		t.Errorf("%d caselle nuovo_riferimento nei template, attese 10: un modulo e' cambiato, va guardato", trovati)
	}

	// il server: senza risposta la sostituzione dello STEP strutturale si rifiuta; con «no» o «sì» passa
	step := uuid.New()
	c := db.Componente{Codice: "7120001", StepStrutturaleID: uuid.NullUUID{UUID: step, Valid: true}}
	sc, err := leggiScelta("sostituisci:"+step.String(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := verificaSostituzione("7120001A_2.stp", c, sc); err == nil || !strings.Contains(err.Error(), "si dice se il nuovo diventa il riferimento") {
		t.Errorf("senza risposta: %v", err)
	}
	for _, risposta := range []string{"0", "1"} {
		sc, _ := leggiScelta("sostituisci:"+step.String(), risposta, "")
		if err := verificaSostituzione("7120001A_2.stp", c, sc); err != nil {
			t.Errorf("con la risposta %q: %v", risposta, err)
		}
	}
}
