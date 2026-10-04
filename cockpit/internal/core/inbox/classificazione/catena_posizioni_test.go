// L1 — il taglio con le posizioni accanto a TagliaCatena (giro 5, A1b.2; piano A, 5.4.3): l'equivalenza con il
// legacy in tabella e nel fuzz (A1b-05, A1b-06), le posizioni sul corpo originale con tutti i fine riga (A1b-07),
// la storia non separabile al posto del ripiego (A1b-08, MAIL-INOLTRO), i livelli della storia (A1b-09, R27 b-c).
//
// I corpi sono sintetici, con le forme vere dei client di posta (l'intestazione di Outlook sotto la riga di
// underscore, «-----Original Message-----», «Il giorno … ha scritto:», il «>» del testo citato). Le persone e i
// clienti sono inventati (ACME, Ufficio Acquisti, acme.example, fornitore.example) e i codici sono di fantasia:
// il repository è pubblico, e un taglio non ha bisogno di posta vera per dimostrare dove taglia. I corpi delle
// prove legacy (catena_test.go qui, catena_test.go e lettura_test.go di core/inbox/lettura) sono riscritti in
// tabella con gli stessi nomi neutri; quelle prove restano com'erano.
package classificazione

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// casoTaglio: un corpo, con lo stato, la regola e il segno d'inoltro che il taglio deve dare.
type casoTaglio struct {
	nome    string
	corpo   string
	stato   StatoTaglio
	regola  RegolaTaglio
	inoltro bool
}

// Corpi delle prove di core/inbox/lettura, riscritti con i codici di fantasia: fanno da seme al fuzz e passano
// dall'equivalenza.
const (
	letturaRispostaIT = `Grazie, confermiamo l'ordine del ACME1111.

________________________________
Da: Fornitore Esempio <ordini@fornitore.example>
Inviato: lunedì 21 settembre 2026 10:15
A: Ufficio Acquisti <acquisti@acme.example>
Oggetto: RFQ ACME1111

ATTENZIONE: questa e-mail proviene da un mittente esterno all'organizzazione. Non aprire gli allegati se non conosci il mittente.

Buongiorno, inviamo l'offerta per il ACME1111.
Saluti`

	letturaInoltroSenzaCommento = `________________________________
Da: Fornitore Esempio <ordini@fornitore.example>
Inviato: lunedì 21 settembre 2026 10:15
A: Ufficio Acquisti <acquisti@acme.example>
Oggetto: I: RFQ ACME1111

Buongiorno, inviamo l'offerta per il ACME1111.`

	letturaUnaCellaPerRiga = "Buongiorno,\r\nvi chiediamo quotazione per:\r\n\r\nCodice\r\n\r\nDescrizione\r\n\r\nQ.TA\r\n\r\n" +
		"ACME1111\r\n\r\nFlangia\r\n\r\n100\r\n\r\nACME2222\r\n\r\n\r\n\r\n250\r\n\r\n\r\nGrazie\r\n"
)

// fMail3: la forma di F-MAIL-3 (5.7.2): un inoltro senza commento, con il separatore, le intestazioni, la
// richiesta, la firma di chi inoltra con un codice, e sotto una citazione più vecchia.
const fMail3 = `---------- Forwarded message ---------
Da: Ufficio Acquisti ACME <acquisti@acme.example>
Date: gio 17 set 2026 alle ore 09:12
Subject: Richiesta di offerta
To: <tecnico@fornitore.example>

Buongiorno, vi chiediamo un'offerta per il codice ACME1111.

Cordiali saluti
Ufficio Acquisti ACME
Tel. interno ACME9999

Il giorno mer 16 set 2026 alle ore 14:32 Ufficio Tecnico ACME <tecnico@acme.example> ha scritto:
> In allegato il disegno ACME2222.`

// fMail6: la forma di F-MAIL-6 (5.7.2): una risposta con «-----Messaggio originale-----».
const fMail6 = `Vi rimando la richiesta, manca la revisione.

-----Messaggio originale-----
Da: Ufficio Acquisti ACME <acquisti@acme.example>
Inviato: mercoledì 16 settembre 2026 14:32
Oggetto: Richiesta di offerta ACME1111

Buongiorno, vi chiediamo un'offerta per ACME1111.`

// casiDelTaglio: i corpi di catena_test.go riscritti in tabella, più quelli nuovi del giro 5.
func casiDelTaglio() []casoTaglio {
	return []casoTaglio{
		// ---- da catena_test.go
		{nome: "mail nuova", stato: StatoNessunaStoria, corpo: `Buongiorno,
inviamo richiesta d'offerta per il codice ACME1111 rev 4.
Consegna richiesta entro il 30/10/2026.

Cordiali saluti
Ufficio Acquisti ACME`},
		{nome: "risposta italiana sotto la riga di underscore", stato: StatoTagliato, regola: RegolaIntestazioneCitata, corpo: `Confermiamo il codice ACME1111 rev 5.

________________________________
Da: Ufficio Acquisti ACME <acquisti@acme.example>
Inviato: giovedì 17 settembre 2026 09:12
A: Ufficio Tecnico
Oggetto: RICHIESTA D'OFFERTA ACME2222

Buongiorno, allego il disegno ACME2222 per quotazione.`},
		{nome: "risposta inglese con il separatore", stato: StatoTagliato, regola: RegolaSeparatoreEsplicito, corpo: `Please quote part ACME1111.

-----Original Message-----
From: Buyer ACME <buyer@acme.example>
Sent: Thursday, September 17, 2026 9:12 AM
To: Sales
Subject: RFQ ACME2222

Dear Sir, please quote drawing ACME2222.`},
		{nome: "risposta inglese con l'apertura", stato: StatoTagliato, regola: RegolaAperturaCitazione, corpo: `Please quote part ACME1111.

On Thu, Sep 17, 2026 at 9:12 AM Buyer ACME <buyer@acme.example> wrote:
> Dear Sir, please quote drawing ACME2222.
> Thank you.`},
		{nome: "quattro risposte concatenate", stato: StatoTagliato, regola: RegolaIntestazioneCitata, corpo: `Ultima risposta: il codice buono è ACME1111.

Da: Ufficio Acquisti ACME
Inviato: giovedì 17 settembre 2026 09:12
Oggetto: RE: RE: RE: offerta

terza risposta, codice ACME3333

-----Messaggio originale-----
Da: Ufficio Tecnico ACME
Oggetto: RE: RE: offerta

seconda risposta, codice ACME4444

Il giorno mer 16 set 2026 alle ore 14:32 Ufficio Qualità ACME <qualita@acme.example> ha scritto:
> prima richiesta, codice ACME5555`},
		{nome: "Da: in mezzo a una frase", stato: StatoNessunaStoria, corpo: `Buongiorno,
la quota va presa Da: spigolo esterno, come da disegno ACME1111.
From: the desk of the buyer — questa è solo la firma.
Grazie`},
		{nome: "elenco con due punti", stato: StatoNessunaStoria, corpo: `Riepilogo lavorazione del ACME1111:
Da: tornitura
A: rettifica
Nota: la rettifica va fatta dopo il trattamento.`},
		{nome: "una sola intestazione con l'indirizzo", stato: StatoNessunaStoria, corpo: `Per il preventivo rispondere a questo indirizzo.
From: ordini@acme.example
Grazie.`},
		{nome: "On all'inizio di una frase", stato: StatoNessunaStoria, corpo: `Please check the tolerance.
On the drawing you sent the tolerance is wrong.
Please confirm item ACME1111.`},
		{nome: "Il giorno all'inizio di una frase", stato: StatoNessunaStoria, corpo: `Vi segnaliamo un problema di consegna.
Il giorno del ritiro la merce non era pronta.
Il codice è ACME1111.`},
		{nome: "elenco di due righe", stato: StatoNessunaStoria, corpo: "Ciclo:\nDa: tornitura\nA: rettifica"},
		{nome: "codici solo nella storia", stato: StatoTagliato, regola: RegolaIntestazioneCitata, corpo: `Ricevuto, grazie.

Da: Ufficio Acquisti ACME
Inviato: giovedì 17 settembre 2026 09:12
Oggetto: RE: offerta

Vi confermiamo i codici ACME1111, ACME2222 e ACME3333.`},
		{nome: "inoltro senza commento", stato: StatoStoriaNonSeparabile, regola: RegolaSeparatoreEsplicito, corpo: `-----Messaggio originale-----
Da: Ufficio Acquisti ACME
Oggetto: RICHIESTA D'OFFERTA

Buongiorno, allego il disegno ACME1111 per quotazione.`},
		{nome: "corpo vuoto", stato: StatoNessunaStoria, corpo: ""},
		{nome: "corpo di soli spazi", stato: StatoNessunaStoria, corpo: "   \n\n  "},
		{nome: "fine riga di Windows", stato: StatoTagliato, regola: RegolaIntestazioneCitata,
			corpo: "Confermiamo ACME1111.\r\nSecondo la vostra richiesta.\r\n\r\nDa: Ufficio Acquisti\r\nOggetto: RE: offerta\r\n\r\nvecchio ACME2222"},
		{nome: "spazio unificatore nell'apertura", stato: StatoTagliato, regola: RegolaAperturaCitazione,
			corpo: "Confermiamo ACME1111.\n\nIl giorno mer 16 set 2026 alle ore 14:32 Ufficio Tecnico ha scritto:\nvecchio ACME2222"},
		{nome: "tedesco con nome e indirizzo", stato: StatoTagliato, regola: RegolaAperturaCitazione,
			corpo: "Bitte um Angebot für ACME1111.\n\nAm 17.09.2026 um 09:12 schrieb Einkauf ACME <einkauf@acme.example>:\n> Anfrage für Zeichnung ACME2222."},
		{nome: "tedesco di Gmail", stato: StatoTagliato, regola: RegolaAperturaCitazione,
			corpo: "Bitte um Angebot für ACME1111.\n\nAm Do., 17. Sept. 2026 um 09:12 Uhr schrieb Einkauf ACME <einkauf@acme.example>:\n\nAnfrage für Zeichnung ACME2222."},
		{nome: "tedesco senza indirizzo", stato: StatoTagliato, regola: RegolaAperturaCitazione,
			corpo: "Bitte um Angebot für ACME1111.\n\nAm 17.09.26 um 09:12 schrieb Einkauf ACME:\nAnfrage für Zeichnung ACME2222."},
		{nome: "Thunderbird italiano", stato: StatoTagliato, regola: RegolaAperturaCitazione,
			corpo: "Confermiamo il codice ACME1111.\n\nIl 12/09/26 10:00, Ufficio Tecnico ha scritto:\n> Vi chiediamo quotazione per ACME2222."},
		{nome: "Thunderbird, riga spezzata", stato: StatoTagliato, regola: RegolaAperturaCitazione,
			corpo: "Confermiamo il codice ACME1111.\n\nIl 12/09/2026 10:00, Ufficio Tecnico <tecnico@acme.example> ha\nscritto:\nVi chiediamo quotazione per ACME2222."},
		{nome: "prosa: Am … schrieb in una frase", stato: StatoNessunaStoria,
			corpo: "Vielen Dank.\nAm Montag schrieb uns der Kunde, dass die Teile fehlen.\nBitte prüfen: ACME1111."},
		{nome: "prosa: Il con una data, senza scritto", stato: StatoNessunaStoria,
			corpo: "Buongiorno.\nIl 12/09/2026 abbiamo spedito i pezzi.\nIl codice è ACME1111."},
		{nome: "prosa: Il con data e scritto a metà", stato: StatoNessunaStoria,
			corpo: "Buongiorno.\nIl 12/09/2026 il cliente ha scritto: la quota va rivista, codice ACME1111.\nGrazie."},
		{nome: "prosa: Am con data e schrieb a metà", stato: StatoNessunaStoria,
			corpo: "Hallo.\nAm 17.09.2026 schrieb der Kunde: die Maße stimmen nicht, Teil ACME1111.\nDanke."},
		{nome: "corpo utile e storia", stato: StatoTagliato, regola: RegolaIntestazioneCitata,
			corpo: "Nuovo: ACME1111.\n\nDa: Ufficio\nOggetto: RE: x\n\nvecchio ACME2222"},

		// ---- nuovi (giro 5)
		{nome: "F-MAIL-3: inoltro senza commento con una citazione più vecchia", stato: StatoStoriaNonSeparabile,
			regola: RegolaSeparatoreEsplicito, inoltro: true, corpo: fMail3},
		{nome: "F-MAIL-6: Messaggio originale", stato: StatoTagliato, regola: RegolaSeparatoreEsplicito, corpo: fMail6},
		{nome: "inoltro dentro una risposta", stato: StatoTagliato, regola: RegolaSeparatoreEsplicito, inoltro: true,
			corpo: "Vi giro la richiesta qui sotto.\n\n---------- Forwarded message ---------\nFrom: Buyer ACME <buyer@acme.example>\nSubject: RFQ ACME2222\n\nPlease quote ACME2222."},
		{nome: "CR soli", stato: StatoTagliato, regola: RegolaIntestazioneCitata,
			corpo: "Riga uno.\rRiga due.\r\rDa: Ufficio Acquisti <acquisti@acme.example>\rOggetto: RE: offerta\r\rvecchio ACME2222"},
		{nome: "CR CR LF", stato: StatoTagliato, regola: RegolaIntestazioneCitata,
			corpo: "Riga uno.\r\r\nDa: Ufficio Acquisti <acquisti@acme.example>\r\nOggetto: RE: offerta\r\n\r\nvecchio ACME2222"},
		{nome: "LF CR", stato: StatoTagliato, regola: RegolaIntestazioneCitata,
			corpo: "Riga uno.\n\rDa: Ufficio Acquisti <acquisti@acme.example>\n\rOggetto: RE: offerta\n\rvecchio ACME2222"},
		{nome: "testo marcato", stato: StatoTagliato, regola: RegolaTestoMarcato,
			corpo: "Va bene così.\n\n> Vi mandiamo il disegno ACME2222.\n> Grazie."},
		{nome: "apertura francese", stato: StatoTagliato, regola: RegolaAperturaCitazione,
			corpo: "Merci pour ACME1111.\n\nLe 17 sept. 2026 à 09:12, Achats ACME <achats@acme.example> a écrit :\n> Demande pour ACME2222."},
		{nome: "separatore tedesco", stato: StatoTagliato, regola: RegolaSeparatoreEsplicito,
			corpo: "Bitte um Angebot.\n\n-----Ursprüngliche Nachricht-----\nVon: Einkauf ACME <einkauf@acme.example>\nGesendet: Donnerstag, 17. September 2026 09:12\nBetreff: Anfrage ACME2222\n\nAnfrage für ACME2222."},
		{nome: "righe vuote sopra il separatore", stato: StatoStoriaNonSeparabile, regola: RegolaSeparatoreEsplicito,
			corpo: "\r\n   \r\n-----Original Message-----\r\nFrom: Buyer ACME <buyer@acme.example>\r\nSubject: RFQ ACME2222\r\n\r\nPlease quote ACME2222."},
		{nome: "solo uno spazio unificatore sopra il separatore", stato: StatoStoriaNonSeparabile, regola: RegolaSeparatoreEsplicito,
			corpo: " \n-----Original Message-----\nFrom: Buyer ACME <buyer@acme.example>\nSubject: RFQ ACME2222\n\nPlease quote ACME2222."},
		{nome: "UTF-8 non valido nella parte corrente", stato: StatoTagliato, regola: RegolaIntestazioneCitata,
			corpo: "Conferma ACME1111 \xff\xfe.\n\nDa: Ufficio Acquisti <acquisti@acme.example>\nOggetto: RE: offerta\n\nvecchio ACME2222"},
		{nome: "U+2007 e U+202F nelle intestazioni", stato: StatoTagliato, regola: RegolaIntestazioneCitata,
			corpo: "Ricevuto.\n\nDa: Ufficio Acquisti <acquisti@acme.example>\nOggetto: RE: offerta\n\nvecchio ACME2222"},
		{nome: "emoji e accenti nella parte corrente", stato: StatoTagliato, regola: RegolaAperturaCitazione,
			corpo: "🔧 Qualità ok per ACME1111 — è confermato.\r\n\r\nIl giorno mer 16 set 2026 alle ore 14:32 Ufficio Tecnico ha scritto:\r\n> vecchio ACME2222"},
		{nome: "lettura: risposta citata", stato: StatoTagliato, regola: RegolaIntestazioneCitata, corpo: letturaRispostaIT},
		{nome: "lettura: inoltro senza commento", stato: StatoStoriaNonSeparabile, regola: RegolaIntestazioneCitata,
			corpo: letturaInoltroSenzaCommento},
		{nome: "lettura: ciclo di lavorazione", stato: StatoNessunaStoria,
			corpo: "Riepilogo lavorazione del ACME1111:\nDa: tornitura\nA: rettifica\n\nGrazie"},
		{nome: "lettura: una cella per riga", stato: StatoNessunaStoria, corpo: letturaUnaCellaPerRiga},
		{nome: "lettura: righe a TAB", stato: StatoNessunaStoria, corpo: "A\tB\nC\tD\n\nE\tF"},
	}
}

// fetta: il testo dell'intervallo iv del corpo.
func fetta(corpo string, iv [2]int) string { return corpo[iv[0]:iv[1]] }

// verificaTaglio controlla, per un corpo qualunque, l'equivalenza con TagliaCatena (5.4.3 punto 5) e gli
// invarianti delle posizioni: righe che coprono il corpo con i terminatori giusti, le stesse righe di
// normalizza più Split, Corrente e Storia che dividono il corpo, gli intervalli utili che ricostruiscono le
// stringhe di TagliaCatena, i confini sui confini di runa, i livelli che coprono la storia. Restituisce il
// taglio per i controlli del chiamante.
func verificaTaglio(t testing.TB, corpo string) Taglio {
	t.Helper()
	tg := TagliaCatenaConPosizioni(corpo)
	if again := TagliaCatenaConPosizioni(corpo); !reflect.DeepEqual(tg, again) {
		t.Fatalf("due tagli dello stesso corpo sono diversi")
	}
	utile, storia := TagliaCatena(corpo)

	// L'equivalenza (A1b-05, A1b-06).
	switch tg.Stato {
	case StatoTagliato:
		if got := strings.TrimSpace(normalizza(fetta(corpo, tg.Corrente))); got != utile {
			t.Fatalf("utile diverso da TagliaCatena:\n  posizioni: %q\n  legacy:    %q", got, utile)
		}
		if got := strings.TrimSpace(normalizza(fetta(corpo, tg.Storia))); got != storia {
			t.Fatalf("storia diversa da TagliaCatena:\n  posizioni: %q\n  legacy:    %q", got, storia)
		}
		if got := normalizza(fetta(corpo, tg.CorrenteUtile)); got != utile {
			t.Fatalf("CorrenteUtile non ricostruisce l'utile: %q contro %q", got, utile)
		}
		if got := normalizza(fetta(corpo, tg.StoriaUtile)); got != storia {
			t.Fatalf("StoriaUtile non ricostruisce la storia: %q contro %q", got, storia)
		}
		if utile == "" {
			t.Fatalf("stato tagliato con l'utile vuoto")
		}
	case StatoNessunaStoria, StatoStoriaNonSeparabile:
		if utile != strings.TrimSpace(corpo) || storia != "" {
			t.Fatalf("stato %s, ma TagliaCatena ha tagliato: (%q, %q)", tg.Stato, utile, storia)
		}
	default:
		t.Fatalf("stato sconosciuto %q", tg.Stato)
	}

	// Le righe: le stesse di normalizza più Split, sul corpo originale, senza buchi.
	normali := strings.Split(normalizza(corpo), "\n")
	righe, pos := righeDelCorpo(corpo)
	if !reflect.DeepEqual(righe, normali) || !reflect.DeepEqual(pos, tg.Righe) {
		t.Fatalf("righe diverse da normalizza più Split: %q contro %q", righe, normali)
	}
	prima := 0
	for k, r := range tg.Righe {
		if r.Inizio != prima || r.Fine < r.Inizio || r.FineTerminatore < r.Fine {
			t.Fatalf("riga %d %+v: non segue la riga prima (che finisce a %d)", k, r, prima)
		}
		term := corpo[r.Fine:r.FineTerminatore]
		ultima := k == len(tg.Righe)-1
		if (ultima && term != "") || (!ultima && term != "\r\n" && term != "\r" && term != "\n") {
			t.Fatalf("riga %d: terminatore %q", k, term)
		}
		if strings.ContainsAny(fetta(corpo, [2]int{r.Inizio, r.Fine}), "\r\n") {
			t.Fatalf("riga %d: un terminatore dentro il contenuto", k)
		}
		prima = r.FineTerminatore
	}
	if prima != len(corpo) {
		t.Fatalf("le righe coprono %d byte su %d", prima, len(corpo))
	}

	// Corrente e Storia dividono il corpo; gli utili stanno dentro.
	if tg.Corrente[0] != 0 || tg.Corrente[1] != tg.Storia[0] || tg.Storia[1] != len(corpo) {
		t.Fatalf("Corrente %v e Storia %v non dividono il corpo di %d byte", tg.Corrente, tg.Storia, len(corpo))
	}
	for _, c := range []struct{ utile, tutto [2]int }{{tg.CorrenteUtile, tg.Corrente}, {tg.StoriaUtile, tg.Storia}} {
		if c.utile[0] < c.tutto[0] || c.utile[1] > c.tutto[1] || c.utile[0] > c.utile[1] {
			t.Fatalf("intervallo utile %v fuori da %v", c.utile, c.tutto)
		}
	}
	switch {
	case tg.Stato == StatoNessunaStoria:
		if tg.RigaTaglio != -1 || tg.Regola != RegolaNessuna || tg.Inoltro || tg.Storia[0] != len(corpo) {
			t.Fatalf("nessuna storia, ma il taglio dice %+v", tg)
		}
	case tg.RigaTaglio < 0 || tg.RigaTaglio >= len(tg.Righe) || tg.Storia[0] != tg.Righe[tg.RigaTaglio].Inizio:
		t.Fatalf("la storia non comincia alla riga di taglio: %+v", tg)
	case tg.Regola == RegolaNessuna:
		t.Fatalf("un taglio senza la regola che l'ha fatto")
	case tg.Stato == StatoStoriaNonSeparabile && strings.TrimSpace(fetta(corpo, tg.Corrente)) != "":
		t.Fatalf("storia non separabile con una parte corrente: %q", fetta(corpo, tg.Corrente))
	}

	// I confini cadono su confini di runa (per un corpo UTF-8 valido; uno non valido non ha rune da rispettare).
	if utf8.ValidString(corpo) {
		for _, iv := range [][2]int{tg.Corrente, tg.Storia, tg.CorrenteUtile, tg.StoriaUtile} {
			for _, b := range iv {
				if !utf8.ValidString(corpo[:b]) {
					t.Fatalf("confine %d a metà di una runa", b)
				}
			}
		}
	}

	// I livelli coprono la storia, in ordine, entro il massimo.
	for _, massimo := range []int{8, 1} {
		livelli := LivelliDellaStoria(corpo, tg, massimo)
		if tg.Stato == StatoNessunaStoria {
			if livelli != nil {
				t.Fatalf("livelli senza storia: %+v", livelli)
			}
			continue
		}
		if len(livelli) == 0 || len(livelli) > massimo {
			t.Fatalf("%d livelli con il massimo %d", len(livelli), massimo)
		}
		if livelli[0].Righe[0] != tg.RigaTaglio || livelli[0].Regola != tg.Regola || livelli[0].Inoltro != tg.Inoltro {
			t.Fatalf("il primo livello non è il taglio: %+v contro %+v", livelli[0], tg)
		}
		for k, l := range livelli {
			if l.Righe[0] >= l.Righe[1] || l.Byte[0] != tg.Righe[l.Righe[0]].Inizio {
				t.Fatalf("livello %d incoerente: %+v", k, l)
			}
			if k > 0 && (l.Righe[0] != livelli[k-1].Righe[1] || l.Byte[0] != livelli[k-1].Byte[1]) {
				t.Fatalf("buco o sovrapposizione fra i livelli %d e %d: %+v", k-1, k, livelli)
			}
		}
		if ultimo := livelli[len(livelli)-1]; ultimo.Righe[1] != len(tg.Righe) || ultimo.Byte[1] != len(corpo) {
			t.Fatalf("l'ultimo livello non arriva in fondo: %+v", ultimo)
		}
	}
	return tg
}

// TestIlTaglioConPosizioniDiceLoStessoDiTagliaCatena (A1b-05): su tutti i corpi della tabella il taglio con le
// posizioni dice lo stesso di TagliaCatena, con lo stato, la regola e il segno d'inoltro attesi.
func TestIlTaglioConPosizioniDiceLoStessoDiTagliaCatena(t *testing.T) {
	for _, c := range casiDelTaglio() {
		t.Run(c.nome, func(t *testing.T) {
			tg := verificaTaglio(t, c.corpo)
			if tg.Stato != c.stato || tg.Regola != c.regola || tg.Inoltro != c.inoltro {
				t.Errorf("stato %s, regola %q, inoltro %v; attesi %s, %q, %v", tg.Stato, tg.Regola, tg.Inoltro, c.stato, c.regola, c.inoltro)
			}
		})
	}
}

// FuzzTagliaCatenaConPosizioni (A1b-06): la stessa equivalenza su ingressi casuali, più gli invarianti delle
// posizioni. Il seme sono i corpi di A1b-05, che comprendono quelli delle prove di lettura; a mano:
// go test -run '^$' -fuzz FuzzTagliaCatenaConPosizioni -fuzztime 120s ./internal/core/inbox/classificazione/
func FuzzTagliaCatenaConPosizioni(f *testing.F) {
	for _, c := range casiDelTaglio() {
		f.Add(c.corpo)
	}
	f.Fuzz(func(t *testing.T, corpo string) {
		verificaTaglio(t, corpo)
	})
}

// TestLePosizioniDelTaglioSonoSulCorpoOriginale (A1b-07): con CRLF, CR soli, «\r\r\n», lo spazio unificatore e
// U+202F, gli offset sono quelli contati a mano sul corpo originale, e le fette del corpo originale sono i
// pezzi che ci si aspetta, con i loro byte (nessuna normalizzazione negli intervalli).
func TestLePosizioniDelTaglioSonoSulCorpoOriginale(t *testing.T) {
	casi := []struct {
		nome                    string
		corpo                   string
		rigaTaglio              int
		corrente, utile, storia [2]int
		storiaUtile             [2]int
		testoUtile              string // corpo[CorrenteUtile], byte per byte
		nellaStoria             string // un pezzo che la storia originale deve contenere com'è
	}{
		{
			nome:       "CRLF",
			corpo:      "Riga uno.\r\nRiga due.\r\n\r\nDa: Ufficio Acquisti <acquisti@acme.example>\r\nOggetto: RE: offerta\r\n\r\nvecchio ACME2222",
			rigaTaglio: 2, corrente: [2]int{0, 22}, utile: [2]int{0, 20}, storia: [2]int{22, 110}, storiaUtile: [2]int{24, 110},
			testoUtile: "Riga uno.\r\nRiga due.", nellaStoria: "offerta\r\n\r\nvecchio",
		},
		{
			nome:       "CR soli",
			corpo:      "Riga uno.\rRiga due.\r\rDa: Ufficio Acquisti <acquisti@acme.example>\rOggetto: RE: offerta\r\rvecchio ACME2222",
			rigaTaglio: 2, corrente: [2]int{0, 20}, utile: [2]int{0, 19}, storia: [2]int{20, 104}, storiaUtile: [2]int{21, 104},
			testoUtile: "Riga uno.\rRiga due.", nellaStoria: "offerta\r\rvecchio",
		},
		{
			// «\r\r\n» sono due terminatori: la riga vuota in mezzo è della storia, come in TagliaCatena.
			nome:       "CR CR LF",
			corpo:      "Riga uno.\r\r\nDa: Ufficio Acquisti <acquisti@acme.example>\r\nOggetto: RE: offerta\r\n\r\nvecchio ACME2222",
			rigaTaglio: 1, corrente: [2]int{0, 10}, utile: [2]int{0, 9}, storia: [2]int{10, 98}, storiaUtile: [2]int{12, 98},
			testoUtile: "Riga uno.", nellaStoria: "\r\nDa: Ufficio",
		},
		{
			// Lo spazio unificatore (due byte) nell'apertura: la regola lo vede come spazio, l'offset no.
			nome:       "spazio unificatore nell'apertura",
			corpo:      "Confermo ACME1111.\n\nIl giorno mer 16 set 2026 alle ore 14:32 Ufficio Tecnico ha scritto:\nvecchio ACME2222",
			rigaTaglio: 1, corrente: [2]int{0, 19}, utile: [2]int{0, 18}, storia: [2]int{19, 106}, storiaUtile: [2]int{20, 106},
			testoUtile: "Confermo ACME1111.", nellaStoria: "ha scritto:",
		},
		{
			// U+202F (tre byte) nel separatore e uno spazio unificatore in testa alla parte corrente: l'utile
			// comincia dopo i suoi due byte.
			nome:       "U+202F nel separatore",
			corpo:      " Confermo ACME1111.\n\n-----Messaggio originale-----\nDa: Ufficio Acquisti\nOggetto: RE: offerta\n\nvecchio ACME2222",
			rigaTaglio: 1, corrente: [2]int{0, 21}, utile: [2]int{2, 20}, storia: [2]int{21, 113}, storiaUtile: [2]int{22, 113},
			testoUtile: "Confermo ACME1111.", nellaStoria: "Messaggio originale",
		},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			tg := verificaTaglio(t, c.corpo)
			if tg.Stato != StatoTagliato {
				t.Fatalf("stato %s, atteso tagliato", tg.Stato)
			}
			if len(c.corpo) != c.storia[1] {
				t.Fatalf("la prova ha contato male: il corpo è di %d byte, non %d", len(c.corpo), c.storia[1])
			}
			if tg.RigaTaglio != c.rigaTaglio || tg.Corrente != c.corrente || tg.CorrenteUtile != c.utile ||
				tg.Storia != c.storia || tg.StoriaUtile != c.storiaUtile {
				t.Errorf("posizioni: riga %d, corrente %v, utile %v, storia %v, storia utile %v; attese %d, %v, %v, %v, %v",
					tg.RigaTaglio, tg.Corrente, tg.CorrenteUtile, tg.Storia, tg.StoriaUtile,
					c.rigaTaglio, c.corrente, c.utile, c.storia, c.storiaUtile)
			}
			if got := fetta(c.corpo, tg.CorrenteUtile); got != c.testoUtile {
				t.Errorf("corpo[CorrenteUtile] = %q, atteso %q", got, c.testoUtile)
			}
			if !strings.Contains(fetta(c.corpo, tg.Storia), c.nellaStoria) {
				t.Errorf("la storia non contiene %q com'è nel corpo originale: %q", c.nellaStoria, fetta(c.corpo, tg.Storia))
			}
		})
	}

	// Le righe del primo caso, una per una: il terminatore CRLF è fuori dal contenuto e dentro la riga.
	tg := TagliaCatenaConPosizioni(casi[0].corpo)
	attese := []RigaTesto{{0, 9, 11}, {11, 20, 22}, {22, 22, 24}, {24, 68, 70}, {70, 90, 92}, {92, 92, 94}, {94, 110, 110}}
	if !reflect.DeepEqual(tg.Righe, attese) {
		t.Errorf("righe %v, attese %v", tg.Righe, attese)
	}
	// «\r\r\n»: due righe, la prima chiusa dal CR da solo, la seconda vuota chiusa da CRLF.
	if r := TagliaCatenaConPosizioni(casi[2].corpo).Righe; r[0] != (RigaTesto{0, 9, 10}) || r[1] != (RigaTesto{10, 10, 12}) {
		t.Errorf("CR CR LF: righe %v", r[:2])
	}
}

// TestSenzaCommentoLaStoriaNonDiventaCorpo (A1b-08, MAIL-INOLTRO, parte del taglio): il corpo della prova legacy
// TestUnInoltroSenzaCommentoTieneTutto, riscritto con i nomi neutri. TagliaCatena tiene tutto, come prima (la
// prova legacy resta com'è); il taglio con le posizioni dà lo stato esplicito, la parte corrente vuota e la
// storia intatta, con i confini. Nessuna promozione.
func TestSenzaCommentoLaStoriaNonDiventaCorpo(t *testing.T) {
	const corpo = `-----Messaggio originale-----
Da: Ufficio Acquisti ACME
Oggetto: RICHIESTA D'OFFERTA

Buongiorno, allego il disegno ACME1111 per quotazione.`

	if utile, storia := TagliaCatena(corpo); utile != corpo || storia != "" {
		t.Fatalf("il legacy è cambiato: (%q, %q)", utile, storia)
	}
	tg := verificaTaglio(t, corpo)
	if tg.Stato != StatoStoriaNonSeparabile {
		t.Fatalf("stato %s, atteso storia_non_separabile", tg.Stato)
	}
	if tg.CorrenteUtile[0] != tg.CorrenteUtile[1] || tg.Corrente != [2]int{0, 0} {
		t.Errorf("la parte corrente non è vuota: corrente %v, utile %v", tg.Corrente, tg.CorrenteUtile)
	}
	if tg.RigaTaglio != 0 || tg.Storia != [2]int{0, len(corpo)} || tg.StoriaUtile != [2]int{0, len(corpo)} {
		t.Errorf("la storia non è tutto il corpo: riga %d, storia %v, utile %v", tg.RigaTaglio, tg.Storia, tg.StoriaUtile)
	}
	if fetta(corpo, tg.StoriaUtile) != corpo {
		t.Errorf("la storia non è intatta")
	}
	if tg.Regola != RegolaSeparatoreEsplicito || tg.Inoltro {
		t.Errorf("regola %q, inoltro %v: «Messaggio originale» taglia ma non è un inoltro", tg.Regola, tg.Inoltro)
	}
}

// TestLaStoriaSiDivideInLivelli (A1b-09; R27 b, c; E1): l'inoltro e la citazione più vecchia sono due livelli;
// il blocco d'intestazioni sotto il separatore non apre un livello; «Messaggio originale» dà una citazione; il
// testo marcato è un blocco solo; le righe «>» subito dopo un confine stanno nel suo livello; il massimo tiene
// il resto nell'ultimo livello.
func TestLaStoriaSiDivideInLivelli(t *testing.T) {
	t.Run("F-MAIL-3: inoltro, poi citazione", func(t *testing.T) {
		tg := verificaTaglio(t, fMail3)
		livelli := LivelliDellaStoria(fMail3, tg, 8)
		if len(livelli) != 2 {
			t.Fatalf("%d livelli, attesi 2 (inoltro e citazione): %+v", len(livelli), livelli)
		}
		inoltro, citazione := livelli[0], livelli[1]
		if !inoltro.Inoltro || inoltro.Regola != RegolaSeparatoreEsplicito || citazione.Inoltro || citazione.Regola != RegolaAperturaCitazione {
			t.Errorf("tipi dei livelli: %+v", livelli)
		}
		// Il secondo livello comincia sulla riga vuota sopra «Il giorno … ha scritto:» (arretra), riga 11.
		if inoltro.Righe != [2]int{0, 11} || citazione.Righe != [2]int{11, 14} {
			t.Errorf("righe dei livelli: %v e %v, attese [0 11] e [11 14]", inoltro.Righe, citazione.Righe)
		}
		primo, secondo := fetta(fMail3, inoltro.Byte), fetta(fMail3, citazione.Byte)
		// La firma di chi inoltra resta nel livello dell'inoltro: la capacità «firma» non c'è (R27 a, 5.10 n.4).
		if !strings.Contains(primo, "ACME1111") || !strings.Contains(primo, "ACME9999") || strings.Contains(primo, "ACME2222") {
			t.Errorf("il livello dell'inoltro: %q", primo)
		}
		if !strings.Contains(secondo, "ACME2222") || strings.Contains(secondo, "ACME1111") {
			t.Errorf("il livello della citazione: %q", secondo)
		}
	})

	t.Run("Messaggio originale sotto un inoltro dà una citazione", func(t *testing.T) {
		const corpo = `-----Messaggio inoltrato-----
Da: Ufficio Acquisti ACME <acquisti@acme.example>
Inviato: giovedì 17 settembre 2026 09:12
Oggetto: I: Richiesta di offerta

Vi giro la richiesta ACME1111.

-----Messaggio originale-----
Da: Ufficio Tecnico ACME <tecnico@acme.example>
Inviato: mercoledì 16 settembre 2026 14:32
Oggetto: Richiesta di offerta

Serve il disegno ACME2222.`
		tg := verificaTaglio(t, corpo)
		livelli := LivelliDellaStoria(corpo, tg, 8)
		if len(livelli) != 2 || !livelli[0].Inoltro || livelli[1].Inoltro ||
			livelli[0].Regola != RegolaSeparatoreEsplicito || livelli[1].Regola != RegolaSeparatoreEsplicito {
			t.Fatalf("attesi un inoltro e una citazione, entrambi dal separatore: %+v", livelli)
		}
		if !strings.Contains(fetta(corpo, livelli[1].Byte), "ACME2222") || strings.Contains(fetta(corpo, livelli[1].Byte), "ACME1111") {
			t.Errorf("la citazione: %q", fetta(corpo, livelli[1].Byte))
		}
	})

	t.Run("F-MAIL-6: una risposta con Messaggio originale", func(t *testing.T) {
		tg := verificaTaglio(t, fMail6)
		livelli := LivelliDellaStoria(fMail6, tg, 8)
		if tg.Inoltro || len(livelli) != 1 || livelli[0].Inoltro || livelli[0].Regola != RegolaSeparatoreEsplicito {
			t.Fatalf("attesa una sola citazione: inoltro %v, livelli %+v", tg.Inoltro, livelli)
		}
	})

	t.Run("il testo marcato è un blocco solo", func(t *testing.T) {
		// Il separatore dentro il testo marcato non apre un secondo livello: per il testo marcato il blocco di
		// confine sono tutte le righe che cominciano con «>» (5.4.3).
		const corpo = "Va bene.\n\n> Ricevuto.\n>\n> -----Original Message-----\n> From: Buyer ACME <buyer@acme.example>\n> Subject: RFQ ACME2222\n"
		tg := verificaTaglio(t, corpo)
		if tg.Regola != RegolaTestoMarcato {
			t.Fatalf("regola %q, attesa testo_marcato", tg.Regola)
		}
		if livelli := LivelliDellaStoria(corpo, tg, 8); len(livelli) != 1 {
			t.Errorf("%d livelli, atteso 1: %+v", len(livelli), livelli)
		}
	})

	// E1 = B (04/10 sera): un blocco di righe «>» che segue subito un confine riconosciuto appartiene al livello
	// che il confine apre, e non ne crea uno in più.
	t.Run("E1: «wrote:» seguito dalle righe «>» è un livello solo", func(t *testing.T) {
		const corpo = "Va bene, procediamo.\n\nOn Thu, 1 Oct 2026 at 09:12, Buyer ACME <buyer@acme.example> wrote:\n> Vi chiediamo un'offerta per ACME2222.\n> Grazie.\n"
		tg := verificaTaglio(t, corpo)
		if tg.Regola != RegolaAperturaCitazione {
			t.Fatalf("regola %q, attesa apertura_citazione", tg.Regola)
		}
		livelli := LivelliDellaStoria(corpo, tg, 8)
		if len(livelli) != 1 || livelli[0].Regola != RegolaAperturaCitazione || livelli[0].Inoltro {
			t.Fatalf("atteso un solo livello di citazione: %+v", livelli)
		}
		if l := fetta(corpo, livelli[0].Byte); !strings.Contains(l, "wrote:") || !strings.Contains(l, "ACME2222") {
			t.Errorf("il livello deve tenere l'apertura e il testo citato: %q", l)
		}
	})

	t.Run("E1: separatore, intestazioni e righe «>» sono un livello solo", func(t *testing.T) {
		const corpo = "Ok, confermo.\r\n\r\n-----Original Message-----\r\nFrom: Buyer ACME <buyer@acme.example>\r\nSubject: RFQ\r\n\r\n> Serve il disegno ACME2222.\r\n>\r\n> Grazie.\r\n"
		tg := verificaTaglio(t, corpo)
		livelli := LivelliDellaStoria(corpo, tg, 8)
		if len(livelli) != 1 || livelli[0].Regola != RegolaSeparatoreEsplicito || livelli[0].Inoltro {
			t.Fatalf("atteso un solo livello di citazione: %+v", livelli)
		}
		if livelli[0].Byte[1] != len(corpo) {
			t.Errorf("il livello finisce a %d, atteso alla fine del corpo (%d)", livelli[0].Byte[1], len(corpo))
		}
	})

	t.Run("E1: inoltro, poi una citazione con le righe «>»: due livelli", func(t *testing.T) {
		const corpo = "---------- Forwarded message ---------\nFrom: Buyer ACME <buyer@acme.example>\nSubject: RFQ\n\nVi giro la richiesta ACME1111.\n\nIl giorno mer 16 set 2026 alle 14:32 Ufficio Tecnico ACME <tecnico@acme.example> ha scritto:\n> Serve il disegno ACME2222.\n> Grazie."
		tg := verificaTaglio(t, corpo)
		livelli := LivelliDellaStoria(corpo, tg, 8)
		if len(livelli) != 2 || !livelli[0].Inoltro || livelli[1].Inoltro || livelli[1].Regola != RegolaAperturaCitazione {
			t.Fatalf("attesi un inoltro e una citazione: %+v", livelli)
		}
		if l := fetta(corpo, livelli[1].Byte); !strings.Contains(l, "ha scritto:") || !strings.Contains(l, "ACME2222") || strings.Contains(l, "ACME1111") {
			t.Errorf("la citazione deve tenere l'apertura e il testo citato: %q", l)
		}
	})

	t.Run("E1: le righe «>» che non seguono subito il confine aprono il loro livello", func(t *testing.T) {
		const corpo = "Ok.\n\nOn Thu, 1 Oct 2026 at 09:12, Buyer ACME <buyer@acme.example> wrote:\nServe il disegno ACME2222.\n\n> Testo ancora più vecchio, ACME3333.\n> Fine.\n"
		tg := verificaTaglio(t, corpo)
		livelli := LivelliDellaStoria(corpo, tg, 8)
		if len(livelli) != 2 || livelli[0].Regola != RegolaAperturaCitazione || livelli[1].Regola != RegolaTestoMarcato {
			t.Fatalf("attesi due livelli (apertura, poi testo marcato): %+v", livelli)
		}
		if l := fetta(corpo, livelli[0].Byte); strings.Contains(l, "ACME3333") || !strings.Contains(l, "ACME2222") {
			t.Errorf("il primo livello: %q", l)
		}
	})

	t.Run("oltre il massimo il resto resta nell'ultimo livello", func(t *testing.T) {
		tg := TagliaCatenaConPosizioni(fMail3)
		livelli := LivelliDellaStoria(fMail3, tg, 1)
		if len(livelli) != 1 || livelli[0].Righe != [2]int{0, 14} || livelli[0].Byte != [2]int{0, len(fMail3)} {
			t.Errorf("con il massimo 1: %+v", livelli)
		}
		if LivelliDellaStoria(fMail3, tg, 0) != nil {
			t.Error("con il massimo 0 nessun livello")
		}
	})

	t.Run("senza storia nessun livello, e un taglio di un altro corpo non vale", func(t *testing.T) {
		const nuova = "Buongiorno,\nvi chiediamo un'offerta per ACME1111."
		if l := LivelliDellaStoria(nuova, TagliaCatenaConPosizioni(nuova), 8); l != nil {
			t.Errorf("livelli senza storia: %+v", l)
		}
		if l := LivelliDellaStoria(fMail6, TagliaCatenaConPosizioni(fMail3), 8); l != nil {
			t.Errorf("livelli con il taglio di un altro corpo: %+v", l)
		}
	})
}
