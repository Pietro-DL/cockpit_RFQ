// L1 — blocco 6: il taglio della catena di risposta.
//
// Il corpus è sintetico ma le forme sono quelle vere: l'intestazione che Outlook italiano mette sotto
// una riga di underscore, il «-----Original Message-----» dei client inglesi, la frase «Il giorno …
// ha scritto:» di Gmail, il «>» dei client che marcano il testo citato.
//
// ATTENZIONE, per onestà: questo NON è il corpus reale. Il corpus reale sta fuori dal repository, e
// la verifica su quello è la condizione A del gate dell'agente AI, che resta aperta.
package classificazione

import (
	"strings"
	"testing"

	"promatec/cockpit/internal/core/registro/regole"
)

// ---------------------------------------------------------------- 1. mail nuova, senza storia

func TestUnaMailNuovaNonSiTocca(t *testing.T) {
	const corpo = `Buongiorno,
inviamo richiesta d'offerta per il codice 1234567A rev 4.
Consegna richiesta entro il 30/10/2026.

Cordiali saluti
Mario Rossi`

	utile, storia := TagliaCatena(corpo)
	if utile != strings.TrimSpace(corpo) {
		t.Errorf("una mail senza storia è stata tagliata:\n--- rimasto ---\n%s", utile)
	}
	if storia != "" {
		t.Errorf("storia inventata dal nulla:\n%s", storia)
	}
}

// ---------------------------------------------------------------- 2. una risposta italiana

func TestUnaRispostaItalianaSiFermaAllIntestazione(t *testing.T) {
	const corpo = `Confermiamo il codice 1234567A rev 5.

________________________________
Da: Mario Rossi <mario.rossi@acme.example>
Inviato: giovedì 17 settembre 2026 09:12
A: Ufficio Tecnico
Oggetto: RICHIESTA D'OFFERTA 7781234

Buongiorno, allego il disegno 7781234 per quotazione.`

	utile, storia := TagliaCatena(corpo)
	if utile != "Confermiamo il codice 1234567A rev 5." {
		t.Errorf("il taglio non si è fermato dove doveva:\n--- rimasto ---\n%s", utile)
	}
	if !strings.Contains(storia, "7781234") {
		t.Errorf("la storia citata non è stata conservata:\n%s", storia)
	}
	if strings.Contains(utile, "_____") {
		t.Errorf("la riga di separazione è rimasta nel corpo utile:\n%s", utile)
	}
}

// ---------------------------------------------------------------- 3. una risposta inglese

func TestUnaRispostaIngleseSiFerma(t *testing.T) {
	casi := map[string]string{
		"separatore esplicito": `Please quote part 1234567A.

-----Original Message-----
From: John Smith <john@buyer.example>
Sent: Thursday, September 17, 2026 9:12 AM
To: Sales
Subject: RFQ 998877

Dear Sir, please quote drawing 998877.`,
		"apertura di citazione": `Please quote part 1234567A.

On Thu, Sep 17, 2026 at 9:12 AM John Smith <john@buyer.example> wrote:
> Dear Sir, please quote drawing 998877.
> Thank you.`,
	}
	for nome, corpo := range casi {
		t.Run(nome, func(t *testing.T) {
			utile, storia := TagliaCatena(corpo)
			if utile != "Please quote part 1234567A." {
				t.Errorf("taglio sbagliato:\n--- rimasto ---\n%s", utile)
			}
			if !strings.Contains(storia, "998877") {
				t.Errorf("la storia non è stata conservata:\n%s", storia)
			}
		})
	}
}

// ---------------------------------------------------------------- 4. quattro risposte concatenate

func TestQuattroRisposteConcatenateSiTaglianoAllaPrima(t *testing.T) {
	const corpo = `Ultima risposta: il codice buono è 1234567A.

Da: Mario Rossi
Inviato: giovedì 17 settembre 2026 09:12
Oggetto: RE: RE: RE: offerta

terza risposta, codice 5551111

-----Messaggio originale-----
Da: Luigi Bianchi
Oggetto: RE: RE: offerta

seconda risposta, codice 4442222

Il giorno mer 16 set 2026 alle ore 14:32 Anna Verdi <anna@acme.example> ha scritto:
> prima richiesta, codice 3333333`

	utile, storia := TagliaCatena(corpo)
	if utile != "Ultima risposta: il codice buono è 1234567A." {
		t.Fatalf("non si è fermato alla PRIMA catena:\n--- rimasto ---\n%s", utile)
	}
	for _, vecchio := range []string{"5551111", "4442222", "3333333"} {
		if strings.Contains(utile, vecchio) {
			t.Errorf("il codice %s della storia è finito nel corpo utile", vecchio)
		}
		if !strings.Contains(storia, vecchio) {
			t.Errorf("il codice %s è stato perso: la storia serve per l'aggancio", vecchio)
		}
	}
}

// ---------------------------------------------------------------- 5. testo legittimo con «Da:»

// È l'avvertimento del checkpoint: «Da:» e «From:» compaiono anche nel testo normale, e una regex
// che taglia qualunque riga che li contiene butta via il messaggio di chi scrive così — in silenzio,
// perché il testo tagliato non lo vede nessuno.
func TestUnTestoLegittimoConDaNonVieneTagliato(t *testing.T) {
	casi := map[string]string{
		"Da: in mezzo a una frase": `Buongiorno,
la quota va presa Da: spigolo esterno, come da disegno 1234567A.
From: the desk of Mario Rossi — questa è solo la firma.
Grazie`,
		"elenco con due punti": `Riepilogo lavorazione del 1234567A:
Da: tornitura
A: rettifica
Nota: la rettifica va fatta dopo il trattamento.`,
		// Una sola intestazione non basta, nemmeno quando porta un indirizzo vero: chi scrive
		// «rispondere a questo indirizzo» non sta citando nessun messaggio precedente.
		"una sola intestazione con l'indirizzo": `Per il preventivo rispondere a questo indirizzo.
From: ordini@acme.example
Grazie.`,
		// «On» e «Il giorno» NON stanno sulla prima riga di proposito: se ci stessero, il taglio
		// lascerebbe un corpo vuoto e la guardia sul vuoto rimetterebbe tutto a posto — la prova
		// passerebbe per il motivo sbagliato, senza dire niente sul riconoscimento.
		"On all'inizio di una frase": `Please check the tolerance.
On the drawing you sent the tolerance is wrong.
Please confirm item 1234567A.`,
		"Il giorno all'inizio di una frase": `Vi segnaliamo un problema di consegna.
Il giorno del ritiro la merce non era pronta.
Il codice è 1234567A.`,
	}
	for nome, corpo := range casi {
		t.Run(nome, func(t *testing.T) {
			utile, storia := TagliaCatena(corpo)
			if utile != strings.TrimSpace(corpo) || storia != "" {
				t.Errorf("testo legittimo tagliato: sarebbe sparito senza che nessuno se ne accorga\n"+
					"--- rimasto ---\n%s\n--- buttato nella storia ---\n%s", utile, storia)
			}
		})
	}
}

// Il caso «elenco con due punti» merita una prova a sé: lì DUE intestazioni di ruolo diverso («Da:»
// e «A:») stanno una dietro l'altra, ed è esattamente la forma che fa scattare il taglio. Non scatta
// perché la prima riga dell'elenco non è un'intestazione ma la frase che lo introduce, e perché il
// blocco si chiude su una riga di prosa.
func TestDueRigheChePaionoIntestazioniMaSonoUnElenco(t *testing.T) {
	const corpo = `Ciclo:
Da: tornitura
A: rettifica`

	if _, storia := TagliaCatena(corpo); storia != "" {
		t.Errorf("un elenco di due righe è stato preso per un'intestazione citata:\n%s", storia)
	}
}

// ---------------------------------------------------------------- 6 e 7. i codici

// 6. Codici presenti SOLO nella storia citata: non devono finire nel corpo utile. È da lì che
// arrivano quasi tutti i falsi multi-codice.
func TestICodiciDellaSoloStoriaRestanoNellaStoria(t *testing.T) {
	const corpo = `Ricevuto, grazie.

Da: Mario Rossi
Inviato: giovedì 17 settembre 2026 09:12
Oggetto: RE: offerta

Vi confermiamo i codici 1234567A, 7781234 e 9990001.`

	utile, storia := TagliaCatena(corpo)
	if utile != "Ricevuto, grazie." {
		t.Fatalf("taglio sbagliato:\n%s", utile)
	}
	for _, c := range []string{"1234567A", "7781234", "9990001"} {
		if strings.Contains(utile, c) {
			t.Errorf("%s è nel corpo utile e stava solo nella storia", c)
		}
		if !strings.Contains(storia, c) {
			t.Errorf("%s è stato perso del tutto", c)
		}
	}
}

// 7. Codice NUOVO nel testo corrente e codici vecchi nella storia: deve emergere principalmente il
// nuovo. «Principalmente» e non «soltanto»: i vecchi restano visibili fra gli altri numeri trovati,
// perché sono l'evidenza migliore per agganciare il messaggio alla richiesta giusta.
func TestIlCodiceNuovoPrevaleSuQuelliDellaStoria(t *testing.T) {
	const schema = `{"famiglie_codice":[
	  {"regex":"\\b(?P<codice>\\d{7})(?P<rev>[A-Z])\\b","descrizione":"ACME sette cifre","esempio":"1234567A","rev_nel_codice":true}]}`
	r, err := regole.ValidaRegole([]byte(schema))
	if err != nil {
		t.Fatal(err)
	}
	in := IngressoTriage{
		Oggetto: "RE: offerta",
		Corpo: `Aggiungiamo alla richiesta il codice 8889999B.

Da: Mario Rossi
Inviato: giovedì 17 settembre 2026 09:12
Oggetto: offerta

Vi chiediamo quotazione per 1234567A e 7781234C.`,
		Direzione: "entrata", ClienteNoto: true, Motore: Compila("ACME", r),
	}

	e := in.Motore.Estrai(in.Testi()...)
	proponibili := SoloCodici(e.Proponibili())
	if len(proponibili) != 1 || proponibili[0] != "8889999" {
		t.Fatalf("proponibili = %v, atteso il solo codice nuovo 8889999: i codici della storia si "+
			"riproporrebbero a ogni risposta, e la richiesta sembrerebbe di tre pezzi", proponibili)
	}
	// i vecchi non spariscono: si vedono, e per entrare serve un clic
	tutti := SoloCodici(e.Codici)
	for _, vecchio := range []string{"1234567", "7781234"} {
		if !contieneStringaTest(tutti, vecchio) {
			t.Errorf("%s è sparito del tutto: era l'evidenza per agganciare la risposta alla sua RFQ (%v)", vecchio, tutti)
		}
	}
	for _, c := range e.Codici {
		if c.Codice == "1234567" && c.Dove != DoveStoria {
			t.Errorf("il codice della storia non è etichettato come tale: Dove = %q", c.Dove)
		}
	}
}

func contieneStringaTest(elenco []string, s string) bool {
	for _, x := range elenco {
		if x == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- il triage non conta la storia

// «Richiesta d'offerta» sta nel primo messaggio di ogni catena: contarlo vorrebbe dire dare
// trentacinque punti di «sembra una richiesta nuova» a ogni «ricevuto, grazie».
func TestLeParoleDellaStoriaNonFannoPuntiDiRichiestaNuova(t *testing.T) {
	in := IngressoTriage{
		Oggetto: "RE: conferma",
		Corpo: `Ricevuto, grazie.

Da: Mario Rossi <mario.rossi@acme.example>
Inviato: giovedì 17 settembre 2026 09:12
Oggetto: richiesta d'offerta

Vi inviamo richiesta d'offerta per i pezzi allegati.`,
		Direzione: "entrata", ClienteNoto: true,
	}

	esito := Triage(in)
	for _, m := range esito.Motivi {
		if strings.Contains(m, "testo contiene") {
			t.Errorf("il triage ha contato una parola che sta solo nella storia citata: %q", m)
		}
	}
}

// Un riferimento trovato nella storia dice a quale richiesta si risponde, non che questa sia una
// richiesta nuova.
func TestIlRiferimentoDellaStoriaNonFaPuntiDiRichiestaNuova(t *testing.T) {
	const schema = `{"riferimento_rfq":{"regex":"\\bRDO\\s?\\d{6}\\b","descrizione":"RDO a sei cifre","esempio":"RDO 400012"}}`
	r, err := regole.ValidaRegole([]byte(schema))
	if err != nil {
		t.Fatal(err)
	}
	in := IngressoTriage{
		Oggetto: "RE: conferma",
		Corpo: `Ricevuto, grazie.

Da: Mario Rossi <mario.rossi@acme.example>
Inviato: giovedì 17 settembre 2026 09:12
Oggetto: RDO 400012

Vi inviamo la RDO 400012.`,
		Direzione: "entrata", ClienteNoto: true, Motore: Compila("ACME", r),
	}

	esito := Triage(in)
	// il riferimento si TROVA lo stesso: serve all'aggancio
	if esito.Riferimento == "" {
		t.Fatal("il riferimento della storia è sparito: era l'evidenza per agganciare la risposta alla sua RFQ")
	}
	if esito.Estrazione.RiferimentoDove != DoveStoria {
		t.Errorf("il riferimento non è etichettato come venuto dalla storia: %q", esito.Estrazione.RiferimentoDove)
	}
	for _, m := range esito.Motivi {
		if strings.Contains(m, "riferimento ") {
			t.Errorf("il triage ha contato un riferimento che sta solo nella storia citata: %q", m)
		}
	}
}

// ---------------------------------------------------------------- casi al bordo

// Un inoltro senza commento non deve diventare un messaggio vuoto: lì la storia È la richiesta.
func TestUnInoltroSenzaCommentoTieneTutto(t *testing.T) {
	const corpo = `-----Messaggio originale-----
Da: Mario Rossi
Oggetto: RICHIESTA D'OFFERTA

Buongiorno, allego il disegno 1234567A per quotazione.`

	utile, storia := TagliaCatena(corpo)
	if !strings.Contains(utile, "1234567A") {
		t.Errorf("l'inoltro è stato svuotato: l'interpretazione non vedrebbe più niente\n%s", utile)
	}
	if storia != "" {
		t.Errorf("il corpo è stato diviso quando non c'era niente di nuovo da separare:\n%s", storia)
	}
}

// Il corpo vuoto non deve far esplodere niente.
func TestCorpoVuoto(t *testing.T) {
	if u, s := TagliaCatena(""); u != "" || s != "" {
		t.Errorf("corpo vuoto → (%q, %q)", u, s)
	}
	if u, s := TagliaCatena("   \n\n  "); u != "" || s != "" {
		t.Errorf("corpo di soli spazi → (%q, %q)", u, s)
	}
}

// Il taglio non deve dipendere dai fine riga di Windows.
func TestIFineRigaDiWindowsNonCambianoIlTaglio(t *testing.T) {
	// Il corpo utile e' di DUE righe di proposito: con una sola, il \r finale se lo porterebbe via
	// TrimSpace e la prova non direbbe niente. I fine riga che contano sono quelli in mezzo.
	const unix = "Confermiamo 1234567A.\nSecondo la vostra richiesta.\n\nDa: Mario Rossi\nOggetto: RE: offerta\n\nvecchio 7781234"
	win := strings.ReplaceAll(unix, "\n", "\r\n")
	u1, _ := TagliaCatena(unix)
	u2, _ := TagliaCatena(win)
	if u1 != u2 {
		t.Errorf("stesso testo, tagli diversi:\n  unix: %q\n  win:  %q", u1, u2)
	}
	if strings.Contains(u2, "\r") {
		t.Errorf("il corpo utile si porta dentro i fine riga di Windows: %q", u2)
	}
}

// Gli spazi unificatori dei client di posta non devono nascondere un separatore.
//
// Il posto in cui fanno danno e' DENTRO la frase, non dopo i due punti: «ha scritto:» non e'
// «ha scritto:» per nessuna regex, perche' \s non comprende lo spazio unificatore, e Outlook e Gmail
// ne mettono uno li' con una certa regolarita'.
func TestLoSpazioUnificatoreNonNascondeLApertura(t *testing.T) {
	corpo := "Confermiamo 1234567A.\n\nIl giorno mer 16 set 2026 alle ore 14:32 Anna Verdi ha scritto:\nvecchio 7781234"
	utile, storia := TagliaCatena(corpo)
	if utile != "Confermiamo 1234567A." {
		t.Errorf("l'apertura di citazione con lo spazio unificatore non è stata riconosciuta:\n%s", utile)
	}
	if !strings.Contains(storia, "7781234") {
		t.Errorf("storia:\n%s", storia)
	}
}

// Due aperture che la regex di prima non vedeva (revisione del 25/09): il tedesco con i due punti DOPO
// il nome («Am … schrieb Max Muster <max@…>:», Gmail, Apple Mail, Thunderbird) e Thunderbird italiano,
// che comincia con la data invece che con «Il giorno».
func TestLeAperturaTedescaEThunderbirdItalianoSiRiconoscono(t *testing.T) {
	casi := map[string]string{
		"tedesco con nome e indirizzo": "Bitte um Angebot für 1234567A.\n\nAm 17.09.2026 um 09:12 schrieb Max Muster <max.muster@acme.example>:\n> Anfrage für Zeichnung 7781234.",
		"tedesco di Gmail":             "Bitte um Angebot für 1234567A.\n\nAm Do., 17. Sept. 2026 um 09:12 Uhr schrieb Max Muster <max.muster@acme.example>:\n\nAnfrage für Zeichnung 7781234.",
		"tedesco senza indirizzo":      "Bitte um Angebot für 1234567A.\n\nAm 17.09.26 um 09:12 schrieb Max Muster:\nAnfrage für Zeichnung 7781234.",
		"Thunderbird italiano":         "Confermiamo il codice 1234567A.\n\nIl 12/09/26 10:00, Mario Rossi ha scritto:\n> Vi chiediamo quotazione per 7781234.",
		"Thunderbird, riga spezzata":   "Confermiamo il codice 1234567A.\n\nIl 12/09/2026 10:00, Mario Rossi <mario.rossi@acme.example> ha\nscritto:\nVi chiediamo quotazione per 7781234.",
	}
	for nome, corpo := range casi {
		t.Run(nome, func(t *testing.T) {
			utile, storia := TagliaCatena(corpo)
			if strings.Contains(utile, "7781234") || !strings.Contains(utile, "1234567A") {
				t.Errorf("taglio sbagliato:\n--- rimasto ---\n%s", utile)
			}
			if !strings.Contains(storia, "7781234") {
				t.Errorf("la storia non è stata conservata:\n%s", storia)
			}
		})
	}
	// e la prosa che comincia allo stesso modo non si taglia
	prosa := map[string]string{
		"Am … schrieb in una frase":      "Vielen Dank.\nAm Montag schrieb uns der Kunde, dass die Teile fehlen.\nBitte prüfen: 1234567A.",
		"Il con una data, senza scritto": "Buongiorno.\nIl 12/09/2026 abbiamo spedito i pezzi.\nIl codice è 1234567A.",
		"Il con data e scritto a metà":   "Buongiorno.\nIl 12/09/2026 il cliente ha scritto: la quota va rivista, codice 1234567A.\nGrazie.",
		"Am con data e schrieb a metà":   "Hallo.\nAm 17.09.2026 schrieb der Kunde: die Maße stimmen nicht, Teil 1234567A.\nDanke.",
	}
	for nome, corpo := range prosa {
		t.Run(nome, func(t *testing.T) {
			if utile, storia := TagliaCatena(corpo); utile != strings.TrimSpace(corpo) || storia != "" {
				t.Errorf("testo legittimo tagliato:\n--- rimasto ---\n%s\n--- storia ---\n%s", utile, storia)
			}
		})
	}
}

// CorpoUtilePerInterpretazione è il nome con cui il checkpoint chiama la cosa: deve esistere e dire
// quello che dice TagliaCatena.
func TestCorpoUtileEeLaPrimaMetaDelTaglio(t *testing.T) {
	const corpo = "Nuovo: 1234567A.\n\nDa: Mario\nOggetto: RE: x\n\nvecchio 7781234"
	u, _ := TagliaCatena(corpo)
	if CorpoUtilePerInterpretazione(corpo) != u {
		t.Error("le due funzioni non dicono la stessa cosa")
	}
}
