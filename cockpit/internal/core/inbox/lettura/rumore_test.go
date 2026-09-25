// L1 — il rumore (piano B): ogni regola con il suo gemello negativo, perché l'errore che costa è
// chiudere una frase vera.
package lettura

import (
	"reflect"
	"strings"
	"testing"
)

const meetupJoin = "https://teams.microsoft.com/l/meetup-join/19%3ameeting_ZmFrZS1yaXVuaW9uZS0wMDAw%40thread.v2/0?context=%7b%22Tid%22%3a%2200000000-0000-0000-0000-000000000000%22%7d"

const (
	bannerEN = `CAUTION: This email originated from outside of the organization. Do not click links or open attachments unless you recognize the sender and know the content is safe.

Buongiorno,
vi mandiamo il disegno del PZ-001 rev 2.

Cordiali saluti
Fornitore Esempio`

	bannerIT = `Buongiorno,
confermiamo la consegna del PZ-002.

Saluti
Fornitore Esempio

ATTENZIONE: questa e-mail proviene da un mittente esterno all’organizzazione.
Non aprire gli allegati se non conosci il mittente.`

	primoContattoEN = `You don't often get email from ordini@fornitore.example. Learn why this is important<https://aka.ms/LearnAboutSenderIdentification>

Buongiorno, alleghiamo l'offerta per il PZ-001.`

	primoContattoIT1 = `Non si ricevono spesso messaggi di posta elettronica da ordini@fornitore.example. Informazioni sul perché è importante<https://aka.ms/LearnAboutSenderIdentification>

Buongiorno, alleghiamo l'offerta per il PZ-001.`

	primoContattoIT2 = `Alcune persone che hanno ricevuto questo messaggio non ricevono spesso messaggi di posta elettronica da ordini@fornitore.example. Scopri perché è importante<https://aka.ms/LearnAboutSenderIdentification>

Buongiorno, alleghiamo l'offerta per il PZ-001.`

	primoContattoIT3 = `Non ricevi spesso e-mail da ordini@fornitore.example. Scopri perché è importante

Buongiorno, alleghiamo l'offerta per il PZ-001.`

	teamsENClassico = `Buongiorno, vi aspettiamo alla riunione sul PZ-001.

________________________________________________________________________________
Microsoft Teams meeting
Join on your computer, mobile app or room device
Click here to join the meeting<` + meetupJoin + `>
Meeting ID: 123 456 789 012
Passcode: AbC123
Download Teams<https://www.microsoft.com/en-us/microsoft-teams/download-app> | Join on the web<https://www.microsoft.com/microsoft-teams/join-a-meeting>
Learn More<https://aka.ms/JoinTeamsMeeting> | Meeting options<https://teams.microsoft.com/meetingOptions/?organizerId=00000000-0000-0000-0000-000000000000>
________________________________________________________________________________`

	teamsENNuovo = `Ciao, ecco l'invito per il PZ-003.

________________________________________________________________________________
Microsoft Teams Need help?<https://aka.ms/JoinTeamsMeeting?omkt=en-US>
Join the meeting now<` + meetupJoin + `>
Meeting ID: 987 654 321 098
Passcode: xY9zZ9
________________________________
Dial in by phone
+39 00 0000 0000,,000000000# Italy, Roma
Find a local number<https://dialin.teams.microsoft.com/00000000-0000-0000-0000-000000000000?id=000000000>
Phone conference ID: 000 000 000#
For organizers: Meeting options<https://teams.microsoft.com/meetingOptions/?organizerId=0> | Reset dial-in PIN<https://dialin.teams.microsoft.com/usp/pstnconferencing>
________________________________________________________________________________`

	// L'invito italiano con il link dentro l'involucro Safe Links e senza «aka.ms/JoinTeamsMeeting»:
	// vale solo se il link si srotola.
	teamsIT = `Buongiorno, vi invito alla revisione del disegno PZ-004.

________________________________________________________________________________
Riunione di Microsoft Teams
Partecipa dal computer, dall'app per dispositivi mobili o dal dispositivo della stanza
Fai clic qui per partecipare alla riunione<https://eur01.safelinks.protection.outlook.com/?url=https%3A%2F%2Fteams.microsoft.com%2Fl%2Fmeetup-join%2F19%253ameeting_ZmFrZQ%2540thread.v2%2F0&data=05%7C02%7C%7C00000000000000000000000000000000%7C0%7C0%7C000000000000000000%7CUnknown%7C0%7C0&sdata=0000&reserved=0>
ID riunione: 111 222 333 444
Passcode: Zz0000
Opzioni riunione<https://teams.microsoft.com/meetingOptions/?organizerId=0>
________________________________________________________________________________`

	clausolaIT = `Confermiamo la consegna del PZ-001 per venerdì.

Cordiali saluti
Fornitore Esempio S.r.l.

Questo messaggio e i suoi allegati sono riservati e destinati esclusivamente al destinatario indicato. Se lo avete ricevuto per errore vi preghiamo di cancellarlo e di avvisare il mittente. Qualsiasi uso non autorizzato è vietato (Regolamento UE 2016/679).`

	clausolaEN = `Please find attached the drawing for PZ-001.

Best regards
Fornitore Esempio

CONFIDENTIALITY NOTICE: This e-mail and any attachments are confidential and intended solely for the use of the individual or entity to whom they are addressed. If you have received this email in error please notify the sender and delete it. Any dissemination or copying is strictly prohibited.`

	clausolaBilingue = `Allego la revisione 3 del PZ-005.

Saluti
ACME

Le informazioni contenute in questo messaggio sono riservate e destinate esclusivamente al destinatario. Se avete ricevuto questa e-mail per errore, siete pregati di distruggerla e di darne notizia al mittente.

This message is confidential and intended only for the named recipient. If you are not the intended recipient please notify the sender and delete this message; any distribution or copying is prohibited.`

	ambienteConGlifo = `Grazie, a presto.

P
Please consider the environment before printing this email.`
)

// soloBlocco controlla che bb sia fatto dei tipi attesi, in ordine.
func soloBlocchi(t *testing.T, bb []Blocco, attesi ...TipoBlocco) {
	t.Helper()
	var got []TipoBlocco
	for _, b := range bb {
		got = append(got, b.Tipo)
	}
	if !reflect.DeepEqual(got, attesi) {
		t.Fatalf("blocchi %v, attesi %v", got, attesi)
	}
}

func motivi(b Blocco) []Motivo { return b.Motivi }

// ---------------------------------------------------------------- N1

func TestN1BannerInCimaSiChiude(t *testing.T) {
	c := verifica(t, bannerEN, "")
	soloBlocchi(t, c.Blocchi, BloccoRumore, BloccoTesto)
	if b := c.Blocchi[0]; !reflect.DeepEqual(b.Motivi, []Motivo{MotivoBannerEsterno}) || b.Etichetta != "avviso: posta esterna" {
		t.Errorf("banner: %+v", b)
	}
	if !strings.Contains(c.Blocchi[0].Grezzo, "originated from outside") {
		t.Errorf("il testo grezzo del banner non è dentro il blocco")
	}
	if c.NRumore != 1 || c.SoloRumore {
		t.Errorf("NRumore=%d SoloRumore=%v", c.NRumore, c.SoloRumore)
	}
}

func TestN1BannerItalianoInFondoSiChiude(t *testing.T) {
	c := verifica(t, bannerIT, "")
	soloBlocchi(t, c.Blocchi, BloccoTesto, BloccoRumore)
	if !reflect.DeepEqual(motivi(c.Blocchi[1]), []Motivo{MotivoBannerEsterno}) {
		t.Errorf("motivi: %v", c.Blocchi[1].Motivi)
	}
}

func TestN1FraseSimileAMetaResta(t *testing.T) {
	const corpo = `Buongiorno,

vi scrivo per il PZ-001.

Come sapete questa e-mail proviene da un mittente esterno all'organizzazione del cliente finale, quindi la gira Fornitore Esempio.

Il disegno è allegato.

Saluti`
	c := verifica(t, corpo, "")
	soloBlocchi(t, c.Blocchi, BloccoTesto)
}

func TestN1FornitoreEsternoNonEUnBanner(t *testing.T) {
	const corpo = "This part originated from outside supplier ACME.\n\nPlease quote PZ-001."
	c := verifica(t, corpo, "")
	soloBlocchi(t, c.Blocchi, BloccoTesto)
}

func TestN1EtichettaSolaInCima(t *testing.T) {
	c := verifica(t, "[EXT]\nBuongiorno, vi mandiamo l'offerta per il PZ-001.", "")
	soloBlocchi(t, c.Blocchi, BloccoRumore, BloccoTesto)
	if got := testoDi(c.Blocchi[1]); got != "Buongiorno, vi mandiamo l'offerta per il PZ-001." {
		t.Errorf("testo: %q", got)
	}
	// La stessa parola a metà non è un'etichetta.
	c = verifica(t, "Buongiorno,\n[EXT]\nciao", "")
	soloBlocchi(t, c.Blocchi, BloccoTesto)
}

// ---------------------------------------------------------------- N2

func TestN2PrimoContattoSiChiude(t *testing.T) {
	for nome, corpo := range map[string]string{
		"EN": primoContattoEN, "IT si ricevono": primoContattoIT1,
		"IT alcune persone": primoContattoIT2, "IT ricevi senza link": primoContattoIT3,
	} {
		t.Run(nome, func(t *testing.T) {
			c := verifica(t, corpo, "")
			soloBlocchi(t, c.Blocchi, BloccoRumore, BloccoTesto)
			if b := c.Blocchi[0]; !reflect.DeepEqual(b.Motivi, []Motivo{MotivoPrimoContatto}) || b.Etichetta != "avviso Microsoft 365: primo contatto" {
				t.Errorf("primo contatto: %+v", b)
			}
		})
	}
}

func TestN2FraseSimileResta(t *testing.T) {
	c := verifica(t, "Non riceviamo spesso disegni da voi, ma questo del PZ-001 è chiaro.\nordini@fornitore.example", "")
	soloBlocchi(t, c.Blocchi, BloccoTesto)
}

func TestN1EN2AdiacentiSiUniscono(t *testing.T) {
	corpo := strings.Replace(primoContattoEN, "\n\nBuongiorno", "\n\nCAUTION: This email originated from outside of the organization.\n\nBuongiorno", 1)
	c := verifica(t, corpo, "")
	soloBlocchi(t, c.Blocchi, BloccoRumore, BloccoTesto)
	b := c.Blocchi[0]
	if !reflect.DeepEqual(b.Motivi, []Motivo{MotivoPrimoContatto, MotivoBannerEsterno}) {
		t.Errorf("motivi: %v", b.Motivi)
	}
	if b.Etichetta != "testo automatico: primo contatto, posta esterna" {
		t.Errorf("etichetta: %q", b.Etichetta)
	}
	if c.NRumore != 2 {
		t.Errorf("NRumore: %d", c.NRumore)
	}
}

func TestSoloRumore(t *testing.T) {
	corpo := "You don't often get email from ordini@fornitore.example. Learn why this is important<https://aka.ms/LearnAboutSenderIdentification>\n\nCAUTION: This email originated from outside of the organization."
	c := verifica(t, corpo, "")
	soloBlocchi(t, c.Blocchi, BloccoRumore)
	if !c.SoloRumore {
		t.Error("SoloRumore falso con solo testo automatico")
	}
	if c = verifica(t, bannerEN, ""); c.SoloRumore {
		t.Error("SoloRumore vero con del testo")
	}
}

// ---------------------------------------------------------------- N3

func TestN3OutlookMobile(t *testing.T) {
	for _, riga := range []string{
		"Scarica Outlook per iOS<https://aka.ms/o0ukef>",
		"Get Outlook for Android<https://aka.ms/AAb9ysg>",
		"Inviato da Outlook per iOS",
		"Inviato da iPhone",
		"Sent from my iPad",
	} {
		c := verifica(t, "Ok, confermo il PZ-001.\n\n"+riga, "")
		soloBlocchi(t, c.Blocchi, BloccoTesto, BloccoRumore)
		if !reflect.DeepEqual(c.Blocchi[1].Motivi, []Motivo{MotivoOutlookMobile}) {
			t.Errorf("%q: motivi %v", riga, c.Blocchi[1].Motivi)
		}
	}
	c := verifica(t, "Scarica Outlook per iOS e poi mandaci il disegno PZ-001", "")
	soloBlocchi(t, c.Blocchi, BloccoTesto)
}

// ---------------------------------------------------------------- N4

func TestN4Separatore(t *testing.T) {
	c := verifica(t, "Ecco il disegno del PZ-001.\n\n________________________________\n\nFornitore Esempio - Ufficio Tecnico", "")
	soloBlocchi(t, c.Blocchi, BloccoTesto, BloccoSeparatore, BloccoTesto)
	c = verifica(t, "Ecco il disegno del PZ-001.\n\n___\n\nFornitore Esempio", "")
	soloBlocchi(t, c.Blocchi, BloccoTesto)
}

// ---------------------------------------------------------------- N5

func TestN5RiunioneTeams(t *testing.T) {
	for nome, caso := range map[string]struct{ corpo, id string }{
		"EN classico": {teamsENClassico, "123 456 789 012"},
		"EN nuovo":    {teamsENNuovo, "987 654 321 098"},
		"IT":          {teamsIT, "111 222 333 444"},
	} {
		t.Run(nome, func(t *testing.T) {
			c := verifica(t, caso.corpo, "")
			// Tutto l'invito in un blocco: le righe di underscore sopra, in mezzo e sotto comprese.
			soloBlocchi(t, c.Blocchi, BloccoTesto, BloccoRumore)
			b := c.Blocchi[1]
			if !reflect.DeepEqual(b.Motivi, []Motivo{MotivoRiunioneTeams}) {
				t.Errorf("motivi: %v", b.Motivi)
			}
			if want := "riunione Microsoft Teams · ID " + caso.id; b.Etichetta != want {
				t.Errorf("etichetta %q, attesa %q", b.Etichetta, want)
			}
		})
	}
}

func TestN5ProsaCheCitaTeamsResta(t *testing.T) {
	const corpo = `Microsoft Teams
Ci sentiamo domani su Teams per il PZ-001, l'invito lo mando io.
Meeting ID: da definire`
	c := verifica(t, corpo, "")
	soloBlocchi(t, c.Blocchi, BloccoTesto)
}

// ---------------------------------------------------------------- N6

func TestN6ClausolaInFondo(t *testing.T) {
	for nome, corpo := range map[string]string{"IT": clausolaIT, "EN": clausolaEN, "bilingue": clausolaBilingue} {
		t.Run(nome, func(t *testing.T) {
			c := verifica(t, corpo, "")
			soloBlocchi(t, c.Blocchi, BloccoTesto, BloccoRumore)
			if b := c.Blocchi[1]; !reflect.DeepEqual(b.Motivi, []Motivo{MotivoRiservatezza}) || b.Etichetta != "clausola di riservatezza" {
				t.Errorf("clausola: %+v", b)
			}
			if strings.Contains(testoDi(c.Blocchi[0]), "riservat") || strings.Contains(testoDi(c.Blocchi[0]), "confidential") {
				t.Errorf("un paragrafo della clausola è rimasto nel testo:\n%s", testoDi(c.Blocchi[0]))
			}
		})
	}
}

func TestN6ConTitoloBastaUnaVoce(t *testing.T) {
	corpo := "Allego l'offerta per il PZ-001.\n\nDisclaimer:\n\n" +
		"The information in this message is confidential. The views expressed are those of the author and do not necessarily represent the company position."
	c := verifica(t, corpo, "")
	soloBlocchi(t, c.Blocchi, BloccoTesto, BloccoRumore)
	if !strings.HasPrefix(c.Blocchi[1].Grezzo, "Disclaimer:") {
		t.Errorf("il titolo non è entrato nel blocco: %q", c.Blocchi[1].Grezzo)
	}
}

func TestN6FraseRiservataAMetaResta(t *testing.T) {
	const corpo = `Buongiorno,
il prezzo del PZ-001 è riservato a voi e non va diffuso ad altri destinatari.

Grazie`
	c := verifica(t, corpo, "")
	soloBlocchi(t, c.Blocchi, BloccoTesto)
}

func TestN6ClausolaSeguitaDaTestoDiLavoroResta(t *testing.T) {
	c := verifica(t, clausolaIT+"\n\nConfermate la consegna?", "")
	soloBlocchi(t, c.Blocchi, BloccoTesto)
}

// ---------------------------------------------------------------- N7

func TestN7AmbienteConIlGlifo(t *testing.T) {
	c := verifica(t, ambienteConGlifo, "")
	soloBlocchi(t, c.Blocchi, BloccoTesto, BloccoRumore)
	b := c.Blocchi[1]
	if !reflect.DeepEqual(b.Motivi, []Motivo{MotivoAmbiente}) || !strings.HasPrefix(b.Grezzo, "P\n") {
		t.Errorf("ambiente: %+v", b)
	}
	c = verifica(t, "Pensate all’ambiente prima di stampare questa mail.", "")
	soloBlocchi(t, c.Blocchi, BloccoRumore)
}

func TestN7EClausolaSiUniscono(t *testing.T) {
	c := verifica(t, clausolaIT+"\n\nP\nPlease consider the environment before printing this email.", "")
	soloBlocchi(t, c.Blocchi, BloccoTesto, BloccoRumore)
	if want := []Motivo{MotivoRiservatezza, MotivoAmbiente}; !reflect.DeepEqual(c.Blocchi[1].Motivi, want) {
		t.Errorf("motivi %v, attesi %v", c.Blocchi[1].Motivi, want)
	}
}
