package web

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/inbox/aggancio"
	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// L1 — Smistamento M1 sulla schermata della posta (A5.16.4): gli score non sono percentuali, le card non
// hanno radio né default, a pari merito nessuna card è marcata.

// rePercentuale: un numero seguito da «%». I nomi di file non ci sono in questi dati: qualunque «%» dopo
// un numero è uno score scritto come una probabilità.
var rePercentuale = regexp.MustCompile(`\d+\s*%`)

// 247 — nessuno score della posta si mostra come percentuale: pannello con le card, form «Aggancia a…»,
// form «Nuova RFQ» con l'avviso, lista dell'Inbox con il chip del triage. Niente radio e niente
// «checked» nelle card; la prima card ha solo il bordo («primo»).
func TestNessunoScoreDellaPostaSiMostraComePercentuale(t *testing.T) {
	s := serverTest(t)
	md, td, _ := datiSintetici()
	tid := uuid.New()
	l := letturaSintetica(tid)
	md.Thread = nil
	md.Allegati = nil // le proposte dei FILE e la loro resa sono della fase F3, non di questa prova
	md.Riga.TriageEsito, md.Riga.TriageConfidenza = txtT("aggancia"), pgtype.Int2{Int16: 98, Valid: true}
	md.Riga.ThreadProposto = uuid.NullUUID{UUID: tid, Valid: true}
	md.Candidati = vistaCandidati(md.M.MessaggioID, l, modoPannello)
	td.Candidati = l
	td.Riga = md.Riga
	aggancia := *td
	aggancia.Azione = "aggancia"
	righe := []db.VInbox{
		{MessaggioID: uuid.New(), Direzione: db.DirezioneEntrata, DataEvento: time.Now(), Oggetto: txtT("una"), ControparteTipo: "cliente",
			TriageEsito: txtT("aggancia"), TriageConfidenza: pgtype.Int2{Int16: 86, Valid: true}, ThreadProposto: uuid.NullUUID{UUID: tid, Valid: true}},
		{MessaggioID: uuid.New(), Direzione: db.DirezioneEntrata, DataEvento: time.Now(), Oggetto: txtT("due"), ControparteTipo: "cliente",
			TriageEsito: txtT("nuova_rfq"), TriageConfidenza: pgtype.Int2{Int16: 60, Valid: true}},
		{MessaggioID: uuid.New(), Direzione: db.DirezioneEntrata, DataEvento: time.Now(), Oggetto: txtT("tre"), ControparteTipo: "cliente",
			TriageEsito: txtT("aggancia"), TriageConfidenza: pgtype.Int2{Int16: 72, Valid: true}},
	}
	casi := []struct {
		nome, frammento string
		dati            any
		attesi          []string
	}{
		{"pannello", "messaggio_pannello", md, []string{"molto forte · score 98", "Aggancia a questa RFQ", "RFQ: molto forte", `class="candidato carta  primo"`}},
		{"aggancia a…", "triage_form", &aggancia, []string{"molto forte · score 98", "Aggancia a questa RFQ"}},
		{"nuova RFQ", "triage_form", td, []string{"molto forte · score 98"}},
		{"lista", "inbox_lista", inboxDati{Filtro: "tutti", Righe: righe}, []string{"RFQ: forte", "nuova RFQ? score 60", "RFQ: medio-forte · da scegliere"}},
	}
	for _, c := range casi {
		var buf bytes.Buffer
		if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, c.frammento, vista{Dati: c.dati, Frammento: true}); err != nil {
			t.Fatalf("%s: %v", c.nome, err)
		}
		html := buf.String()
		if m := rePercentuale.FindString(html); m != "" {
			t.Errorf("%s: uno score scritto come percentuale: %q", c.nome, m)
		}
		for _, a := range c.attesi {
			if !strings.Contains(html, a) {
				t.Errorf("%s: manca %q", c.nome, a)
			}
		}
		if strings.Contains(html, `type="radio"`) {
			t.Errorf("%s: c'è ancora un radio per scegliere la RFQ", c.nome)
		}
		// le card non nascono scelte (il form che le contiene ha altre caselle dopo: si guarda fino alla
		// fine del gruppo delle card)
		if i := strings.Index(html, `class="candidati-aggancio"`); i >= 0 {
			carte := html[i:]
			if j := strings.Index(carte, "</fieldset>"); j >= 0 {
				carte = carte[:j]
			}
			if strings.Contains(carte, "checked") || strings.Contains(carte, "selected") {
				t.Errorf("%s: una card nasce scelta", c.nome)
			}
		}
	}
	// l'avviso del form «Nuova RFQ» non ha bottoni che mandino il form della RFQ nuova con una RFQ dentro
	var buf bytes.Buffer
	if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, "triage_form", vista{Dati: td, Frammento: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `name="thread_id"`) {
		t.Error("il form «Nuova RFQ» ha un bottone con thread_id: manderebbe una RFQ nuova con un aggancio dentro")
	}
	// e il controllo: con la resa di prima (esito e confidenza con «%») la prova se ne accorge
	if rePercentuale.FindString("aggancia 95%") == "" {
		t.Fatal("la regex non riconosce la resa di prima: la prova non prova niente")
	}
}

// 240 sulla schermata (P37) — a pari merito nessuna card è marcata come prima, e la riga lo dice.
func TestAPariMeritoNessunaCardEPrima(t *testing.T) {
	s := serverTest(t)
	t1, t2 := uuid.New(), uuid.New()
	l := aggancio.Lettura{Candidati: classificazione.RaggruppaEOrdina([]classificazione.Candidato{
		classificazione.NuovoCandidato(t1.String(), classificazione.TipoR3CodiceBuyer, "codice 7120001, stesso buyer", false),
		classificazione.NuovoCandidato(t2.String(), classificazione.TipoR3CodiceBuyer, "codice 7120001, stesso buyer", false),
	}), RFQ: map[string]aggancio.RFQ{t1.String(): {Cliente: "Acme"}, t2.String(): {Cliente: "Acme"}}}
	l.PariMerito = classificazione.PariMerito(l.Candidati)
	v := vistaCandidati(uuid.New(), l, modoPannello)
	var buf bytes.Buffer
	if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, "candidati_aggancio", v); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "nessuna RFQ è proposta") || strings.Contains(html, " primo\"") {
		t.Errorf("a pari merito: %s", html)
	}
	if strings.Count(html, "Aggancia a questa RFQ") != 2 {
		t.Errorf("ogni card ha il suo bottone: %d", strings.Count(html, "Aggancia a questa RFQ"))
	}
	// il controllo: senza pari merito la prima card ha il bordo
	l.Candidati = l.Candidati[:1]
	l.PariMerito = false
	buf.Reset()
	if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, "candidati_aggancio", vistaCandidati(uuid.New(), l, modoPannello)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), " primo\"") {
		t.Error("con una sola RFQ la card ha il bordo della prima")
	}
}

// Il chip del triage: il livello per «aggancia», il punteggio per «nuova_rfq», nessuna percentuale; le
// confidenze scritte con le regole di prima si leggono verso il basso (A5.10). «risposta a una nostra
// richiesta» solo per la posta di un fornitore che risponde a una richiesta: a pari merito fra due RFQ
// il chip dice «da scegliere» anche per un fornitore. Il colore della riga va d'accordo col chip: verde
// quando il triage propone qualcosa, giallo quando la RFQ è da scegliere.
func TestIlChipDelTriageDiceIlLivello(t *testing.T) {
	proposto := uuid.NullUUID{UUID: uuid.New(), Valid: true}
	casi := []struct {
		esito    string
		conf     int16
		proposto uuid.NullUUID
		cp       string
		pari     bool
		atteso   string
		colore   string
	}{
		{"aggancia", 98, proposto, "cliente", false, "RFQ: molto forte", "verde"},
		{"aggancia", 86, proposto, "cliente", false, "RFQ: forte", "verde"},
		{"aggancia", 95, proposto, "cliente", false, "RFQ: debole", "verde"}, // R1 di prima: il solo ConversationID
		{"aggancia", 72, uuid.NullUUID{}, "cliente", true, "RFQ: medio-forte · da scegliere", "giallo"},
		{"aggancia", 95, uuid.NullUUID{}, "fornitore", false, "risposta a una nostra richiesta", "verde"},
		{"aggancia", 72, uuid.NullUUID{}, "fornitore", true, "RFQ: medio-forte · da scegliere", "giallo"},
		{"aggancia", 86, uuid.NullUUID{}, "sconosciuto", true, "RFQ: forte · da scegliere", "giallo"},
		{"aggancia", 86, proposto, "fornitore", false, "RFQ: forte", "verde"},
		{"nuova_rfq", 60, uuid.NullUUID{}, "cliente", false, "nuova RFQ? score 60", "giallo"},
		{"ignora", 0, uuid.NullUUID{}, "cliente", false, "ignora", "grigio"},
	}
	for _, c := range casi {
		conf := pgtype.Int2{Int16: c.conf, Valid: true}
		got := testoTriage(txtT(c.esito), conf, c.proposto, c.cp, c.pari)
		if got != c.atteso || strings.Contains(got, "%") {
			t.Errorf("%s %d %s pari=%v: %q, atteso %q", c.esito, c.conf, c.cp, c.pari, got, c.atteso)
		}
		if col := coloreDelTriage(txtT(c.esito), conf, c.proposto, c.cp, c.pari); col != c.colore {
			t.Errorf("%s %d %s pari=%v: colore %q, atteso %q", c.esito, c.conf, c.cp, c.pari, col, c.colore)
		}
	}
	// il pari merito si legge dai motivi della riga, con la frase che scrive il triage
	motivi := func(m ...string) *json.RawMessage {
		b, _ := json.Marshal(m)
		r := json.RawMessage(b)
		return &r
	}
	riga := db.VInbox{TriageEsito: txtT("aggancia"), TriageConfidenza: pgtype.Int2{Int16: 72, Valid: true}, ControparteTipo: "fornitore",
		TriageMotivi: motivi("mittente censito come fornitore", classificazione.FrasePariMerito)}
	if got := chipTriage(riga); got != "RFQ: medio-forte · da scegliere" {
		t.Errorf("fornitore a pari merito, dalla riga: %q", got)
	}
	if col := coloreTriage(riga); col != "giallo" {
		t.Errorf("fornitore a pari merito, colore della riga: %q", col)
	}
	riga.TriageMotivi = motivi("mittente censito come fornitore", "risponde alla richiesta del 12/09")
	if got := chipTriage(riga); got != "risposta a una nostra richiesta" {
		t.Errorf("fornitore che risponde a una richiesta, dalla riga: %q", got)
	}
	if col := coloreTriage(riga); col != "verde" {
		t.Errorf("fornitore che risponde a una richiesta, colore della riga: %q", col)
	}
	if pariMeritoNeiMotivi(nil) || pariMeritoNeiMotivi(motivi()) {
		t.Error("senza motivi non c'è pari merito")
	}
}

// 247, P37 — Invio in un campo del form «Aggancia a…» non sceglie nessuna RFQ. Il bottone di default di
// un form (il primo submit) è quello che l'invio implicito usa: se fosse il bottone di una card, Invio
// nella ricerca o nel cognome aggancerebbe alla prima RFQ, anche a pari merito. Il primo submit del
// form deve essere disabilitato e senza thread_id. Il controllo: il primo bottone di card che segue
// porta davvero thread_id (se la guardia sparisse, sarebbe lui il default).
func TestInvioNelFormAgganciaNonScegliePerTe(t *testing.T) {
	s := serverTest(t)
	_, td, _ := datiSintetici()
	tid := uuid.New()
	td.Candidati = letturaSintetica(tid)
	td.Azione = "aggancia"
	var buf bytes.Buffer
	if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, "triage_form", vista{Dati: td, Frammento: true}); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	i := strings.Index(html, `/aggancia"`)
	if i < 0 {
		t.Fatalf("manca il form «Aggancia a…»: %.300s", html)
	}
	form := html[i:]
	if j := strings.Index(form, "</form>"); j >= 0 {
		form = form[:j]
	}
	submit := regexp.MustCompile(`<button[^>]*type="submit"[^>]*>`).FindAllString(form, -1)
	if len(submit) < 2 {
		t.Fatalf("nel form ci vogliono il bottone inerte e quelli delle card: %v", submit)
	}
	if primo := submit[0]; !strings.Contains(primo, " disabled") || strings.Contains(primo, "thread_id") {
		t.Errorf("il bottone di default del form non è inerte: Invio aggancerebbe con %s", primo)
	}
	if !strings.Contains(submit[1], `name="thread_id"`) {
		t.Errorf("il controllo: dopo il bottone inerte viene quello di una card, con thread_id: %s", submit[1])
	}
}
