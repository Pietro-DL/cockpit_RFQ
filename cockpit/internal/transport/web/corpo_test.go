package web

import (
	"bytes"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/inbox/lettura"
)

// Il corpo di una mail com'è fatto davvero: l'avviso di posta esterna in cima, una tabella incollata
// da Excel (nel testo semplice le celle sono separate da TAB, nell'HTML c'è la <table>), un indirizzo
// avvolto da Safe Links, uno script scritto nel testo, la firma di Outlook per iOS, e sotto la
// storia citata.
const corpoDiProva = "CAUTION: This email originated from outside of the organization. Do not click links or open attachments unless you recognize the sender.\r\n" +
	"\r\n" +
	"Buongiorno,\r\n" +
	"vi chiediamo quotazione per:\r\n" +
	"\r\n" +
	"Codice\tDescrizione\tQ.tà\r\n" +
	"PZ-001\tStaffa\t100\r\n" +
	"PZ-002\tPiastra\t250\r\n" +
	"\r\n" +
	"Specifiche su https://eur01.safelinks.protection.outlook.com/?url=https%3A%2F%2Fportale.acme.example%2Frfq&data=00%7C00&sdata=00&reserved=0\r\n" +
	"<script>alert(1)</script>\r\n" +
	"\r\n" +
	"Get Outlook for iOS\r\n" +
	"\r\n" +
	"________________________________\r\n" +
	"Da: Ufficio acquisti <acquisti@acme.example>\r\n" +
	"Inviato: lunedì 15 settembre 2026 10:00\r\n" +
	"A: Commerciale <commerciale@azienda.example>\r\n" +
	"Oggetto: RFQ PZ-001\r\n" +
	"\r\n" +
	"Richiesta precedente.\r\n"

const htmlDiProva = `<html><head><style>p.MsoNormal{margin:0}</style></head><body>
<p class=MsoNormal>Buongiorno,<br>vi chiediamo quotazione per:</p>
<table class=MsoNormalTable border=0 cellspacing=0 cellpadding=0>
<tr><td><p class=MsoNormal><b>Codice</b></p></td><td><p class=MsoNormal><b>Descrizione</b></p></td><td><p class=MsoNormal><b>Q.tà</b></p></td></tr>
<tr><td><p class=MsoNormal>PZ-001</p></td><td><p class=MsoNormal>Staffa</p></td><td><p class=MsoNormal align=right>100</p></td></tr>
<tr><td><p class=MsoNormal>PZ-002</p></td><td><p class=MsoNormal>Piastra</p></td><td><p class=MsoNormal align=right>250</p></td></tr>
</table></body></html>`

func TestIlCorpoDellaMailSiLeggeInSchermata(t *testing.T) {
	s := serverTest(t)
	md, _, thd := datiSintetici()
	md.M.Oggetto = txtT("[EXT] RE: RFQ PZ-001")
	md.Corpo = lettura.Presenta(corpoDiProva, htmlDiProva)
	thd.Messaggi[0].Corpo = md.Corpo

	for _, c := range []struct {
		pagina, nome string
		dati         any
	}{
		{"inbox.html", "messaggio_pannello", md},
		{"thread.html", "thread_corpo", thd},
	} {
		var buf bytes.Buffer
		if err := s.pagine[c.pagina].ExecuteTemplate(&buf, c.nome, vista{Dati: c.dati, Frammento: true}); err != nil {
			t.Fatalf("%s: %v", c.nome, err)
		}
		out := buf.String()
		for _, atteso := range []string{
			`class="corpo-tabella"`, "<th", "PZ-002", `class="num"`, // la tabella di Excel è una tabella
			`class="corpo-rumore"`,       // l'avviso e la firma di Outlook sono chiusi, non tolti
			"messaggi precedenti citati", // la storia citata sta a parte
			"testo originale",            // e il corpo com'è resta a un clic
			"https://portale.acme.example/rfq",
			"&lt;script&gt;alert(1)&lt;/script&gt;", // il testo della mail si scrive con l'escape
		} {
			if !strings.Contains(out, atteso) {
				t.Errorf("%s: manca %q", c.nome, atteso)
			}
		}
		if strings.Contains(out, "<script>alert(1)") {
			t.Errorf("%s: uno script scritto nella mail è arrivato nella pagina", c.nome)
		}
		// il corpo va da «corpo-vista» alla chiusura del «testo originale», che è il suo ultimo pezzo
		vista := out[strings.Index(out, `class="corpo-vista"`):]
		vista = vista[:strings.Index(vista, "</pre></details></div>")]
		if strings.Contains(vista, "<a ") || strings.Contains(vista, "href=") {
			t.Errorf("%s: un indirizzo della mail è diventato un collegamento cliccabile", c.nome)
		}
	}

	// l'oggetto si legge senza l'etichetta del server di posta
	var buf bytes.Buffer
	if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, "messaggio_pannello", vista{Dati: md}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "<h2>RE: RFQ PZ-001</h2>") {
		t.Errorf("l'oggetto nel pannello tiene ancora l'etichetta [EXT]")
	}
}

func TestUnMessaggioSenzaTestoLoDice(t *testing.T) {
	s := serverTest(t)
	md, _, _ := datiSintetici()
	md.Corpo = lettura.Presenta("", "")
	var buf bytes.Buffer
	if err := s.pagine["inbox.html"].ExecuteTemplate(&buf, "messaggio_pannello", vista{Dati: md}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Nessun testo.") {
		t.Errorf("un messaggio vuoto non dice che è vuoto")
	}
}
