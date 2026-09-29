package classificazione

import (
	"encoding/json"
	"path"
	"regexp"
	"strconv"
	"strings"

	"promatec/cockpit/internal/platform/contratti/worker"
)

// Il testo dei PDF (Smistamento F9, A5.13.8; decisioni del 27/09 «ter»). Dall'analizzatore 4 il worker riporta
// i FATTI del testo di un PDF (`dettagli.testo_pdf`): frammenti con pagina e riquadro, i campi del probabile
// cartiglio come etichetta → valore, i metadati, l'esito dell'OCR selettivo. Il worker non cerca codici, perche'
// non sa per quale RFQ analizza (A5.13.7): li cerca il server, qui, con il Motore del cliente di ciascuna RFQ.
// Lo stesso PDF puo' dire «7120010, codice di famiglia» in una RFQ e niente in un'altra.
//
// Tre livelli, e chi sta sopra non guarda sotto:
//   - i fatti del worker (worker.TestoPDF), che si leggono solo qui (testoDelPDF, non esportata): fuori dalla
//     classificazione esce soltanto lo stato (StatoDelTestoPDF), mai il fatto (per il censimento delle forme,
//     4.17a, escono i campi del cartiglio che nominano il pezzo, gia' interpretati: CampiIdentificativiDelPDF);
//   - le LETTURE normalizzate (LetturaTesto): un codice, con la fonte (testo nativo del cartiglio, testo nativo
//     altrove, metadati, OCR), la pagina, il riquadro, il campo del cartiglio in cui sta, la rev del cartiglio
//     e se ripete il nome del file. Le da' EvidenzeTestoPDF (pura, per un Motore) e, per chi viene dopo e ha i
//     fatti grezzi e il nome del file, LettureDelPDF: e' quello che consuma il flusso ancorato (F8), che non
//     legge mai il JSON del worker;
//   - la valutazione del file (evidenzeCodiceDelTesto), che delle letture prende solo quelle che hanno una
//     riga nella tabella S1 (Domanda 3 = A: nessuno score nuovo).
//
// Che cosa pesa (A5.14.3, P33):
//   - il codice di FAMIGLIA nel campo del codice del cartiglio (codice o numero di disegno, nel testo nativo in
//     basso a destra della pagina 1, il «probabile cartiglio») e il codice del titolo o del soggetto dei metadati
//     entrano nella valutazione del file, dimensione codice (pdf_testo_famiglia 85, pdf_metadati 40);
//   - gli altri codici della zona del cartiglio (l'elenco particolari di un disegno d'assieme, la tabella delle
//     revisioni, «speculare di») non sono letture del codice del disegno, e nemmeno chiavi (giro 4, fase 4.6:
//     campoDelCodice);
//   - i codici del resto delle pagine servono solo come chiavi di ricerca nell'indice dei codici della RFQ
//     (F8): una quota, una norma, un numero d'ordine stampato sul disegno non diventano mai il codice del file;
//   - quello che ha letto l'OCR e' un INDIZIO: si registra e si mostra con la sua fonte, ma non ha uno score
//     finche' la calibrazione (S2) non l'ha misurato, non alza lo score di niente e non preseleziona;
//   - il cartiglio di un testo letto con una sottoversione di prima (testoDiPrima) e' anche lui un indizio e una
//     chiave, mai il codice del file, finche' «Rianalizza» non lo rilegge (giro 4, fase 4.6r); se dice un codice
//     diverso da quello che vince, la dimensione e' discorde (fase 4.6r2).
//
// Domanda 7 = B (27/09): il titolo dei metadati uguale al nome del file, o un testo che ripete il nome del file,
// DIPENDE dal nome e non alza lo score; il cartiglio che riporta un codice diverso dal nome e' una
// discordanza (mai una correzione del nome o del codice); quello che lo conferma in modo indipendente e' una
// seconda fonte concorde.

// Lo stato del testo di un PDF: che cosa si puo' dire del suo contenuto scritto (A5.13.3, passo 2b.4).
const (
	// TestoLetto: il testo c'e' (testo_pdf con estraibile).
	TestoLetto = "letto"
	// TestoAssente: il PDF e' stato letto e non ha testo nel file (curve, scansione): «serve l'OCR», e l'OCR
	// dice se c'e' stato. E' una risposta.
	TestoAssente = "senza_testo"
	// TestoNonLetto: i fatti non portano `testo_pdf` (analizzatore prima del 4, o un worker non aggiornato che
	// ha risposto a un job v4): il testo NON e' stato letto, e il PDF va rianalizzato, non dichiarato muto. Tutti
	// e due i casi si riaccodano con «Rianalizza» (fascicolo.AccodaPdfDaRileggere).
	TestoNonLetto = "non_letto"
	// TestoIlleggibile: il PDF non si apre (`errore_pdf`).
	TestoIlleggibile = "illeggibile"
	// TestoDaAnalizzare: nessuna analisi l'ha ancora letto (scaricato da poco, analisi in coda): non si sa
	// niente, e non si dice niente. Lo usa chi legge dal database (fascicolo.TestoCorrenteDelPDF): i fatti di
	// un'analisi non lo sono mai.
	TestoDaAnalizzare = "da_analizzare"
)

// testoDelPDF legge dai fatti di un'analisi la lettura del testo e ne dice lo stato. Il testo c'e' (non nil)
// solo con TestoLetto e TestoAssente: in un PDF senza testo restano i metadati e l'esito dell'OCR. Non e'
// esportata: il fatto del worker non esce dalla classificazione (F9 → F8), escono le letture (LettureDelPDF)
// e lo stato (StatoDelTestoPDF).
func testoDelPDF(fatti json.RawMessage) (*worker.TestoPDF, string) {
	if t, ok := worker.DecodificaTestoPDF(fatti); ok {
		if t.Estraibile {
			return t, TestoLetto
		}
		return t, TestoAssente
	}
	var f struct {
		ErrorePdf string `json:"errore_pdf"`
	}
	if len(fatti) > 0 && json.Unmarshal(fatti, &f) == nil && f.ErrorePdf != "" {
		return nil, TestoIlleggibile
	}
	return nil, TestoNonLetto
}

// StatoDelTestoPDF e' soltanto lo stato del testo nei fatti di un'analisi (TestoLetto, TestoAssente,
// TestoNonLetto, TestoIlleggibile), per chi lo vuole sapere senza guardare il contenuto. Non dice se il PDF va
// riletto: un testo letto con una sottoversione di prima e' TestoLetto e va riletto lo stesso. Quello lo dice
// TestoPDFDaRileggere, che e' la condizione di fascicolo.AccodaPdfDaRileggere dalla fase 4.6.
func StatoDelTestoPDF(fatti json.RawMessage) string {
	_, stato := testoDelPDF(fatti)
	return stato
}

// TestoPDFDaRileggere dice se «Rianalizza» deve riaccodare un PDF con questi fatti dell'analizzatore corrente
// (fascicolo.AccodaPdfDaRileggere): il testo non e' stato letto (TestoNonLetto), oppure e' stato letto da un worker
// con una sottoversione del testo di prima di quella di oggi (testoDiPrima; giro 4, fase 4.6: le etichette
// bilingui, «PART N°», il campo del particolare simile, l'intestazione dell'elenco particolari che non e' un
// campo). Un testo di prima si legge lo stesso finche' nessuno chiede di rileggerlo, ma il suo cartiglio non e'
// contenuto (LetturaTesto.DaRileggere, fase 4.6r): chiavi e indizi, con la frase FraseTestoDiPrima. Un PDF che non
// si apre no: e' una risposta.
func TestoPDFDaRileggere(fatti json.RawMessage) bool {
	t, stato := testoDelPDF(fatti)
	return stato == TestoNonLetto || testoDiPrima(t)
}

// testoDiPrima dice se il testo e' di una sottoversione di prima di quella che il worker di oggi scrive
// (worker.VersioneTestoPDF): chi l'ha letto non conosceva le etichette del cartiglio della fase 4.6, e prendeva
// l'intestazione dell'elenco particolari di un disegno d'assieme per il campo del codice.
func testoDiPrima(t *worker.TestoPDF) bool {
	return t != nil && t.Versione < worker.VersioneTestoPDF
}

// FraseTestoDiPrima e' la frase di un testo letto con una sottoversione di prima (testoDiPrima): il testo c'e', ma
// quello che dice il suo cartiglio non e' contenuto finche' «Rianalizza» non lo rilegge (LetturaTesto.DaRileggere).
const FraseTestoDiPrima = "testo letto con il worker di prima: da rianalizzare"

// FraseCartiglioDiPrima e' la frase di una lettura del cartiglio di un testo di prima nella valutazione
// (RegolaCartiglioDiPrima; giro 4, fase 4.6r2): il codice che quel testo dice, che non vota ma, se non e' quello
// che vince, rende le fonti discordi (Componi).
func FraseCartiglioDiPrima(codice string) string {
	return "il testo letto con il worker di prima dice " + codice + ": da rianalizzare"
}

// FraseTestoPDF e' lo stato del testo di un PDF in parole, con quello che ha fatto l'OCR quando il file non
// ha testo: una sola frase per la valutazione, la schermata e il flusso. "" per un testo letto.
func FraseTestoPDF(stato, ocr string) string {
	switch stato {
	case TestoAssente:
		switch ocr {
		case worker.OCREseguito:
			return "il PDF non ha testo nel file (curve o scansione): letto con l'OCR, solo come indizio"
		case worker.OCRNonDisponibile:
			return "il PDF non ha testo nel file (curve o scansione): serve l'OCR, che sul worker non c'e'"
		case worker.OCRSpento:
			return "il PDF non ha testo nel file (curve o scansione): serve l'OCR, spento sul worker"
		case worker.OCRFallito, worker.OCRScaduto, worker.OCRIlleggibile:
			return "il PDF non ha testo nel file (curve o scansione): l'OCR non ha letto niente (" + ocr + ")"
		}
		return "il PDF non ha testo nel file (curve o scansione): serve l'OCR"
	case TestoNonLetto:
		return "testo del PDF non letto (analisi di prima dell'analizzatore 4, o di un worker non aggiornato): da rianalizzare"
	case TestoIlleggibile:
		return "il PDF non si apre"
	case TestoDaAnalizzare:
		return "testo del PDF non ancora letto: l'analisi non e' ancora arrivata"
	}
	return ""
}

// Da dove viene una lettura del testo di un PDF (LetturaTesto.Fonte). Sono fonti diverse con pesi diversi:
// il testo scritto nel file dove sta il cartiglio, lo stesso testo altrove, i metadati (spesso il nome del
// file), l'OCR (un indizio).
const (
	FonteTestoCartiglio = "testo_cartiglio" // testo nativo nella zona in basso a destra della pagina 1
	FonteTestoPagina    = "testo_pagina"    // testo nativo altrove (anche un campo del cartiglio fuori zona)
	FonteMetadatiPDF    = "metadati_pdf"    // titolo o soggetto
	FonteOCR            = "ocr"             // letto dall'immagine, in qualunque zona
)

// Dove sta un codice trovato nel testo di un PDF (LetturaTesto.Zona): le due zone del worker e i due campi
// dei metadati che contano.
const (
	ZonaBassoDestra = worker.ZonaBassoDestra
	ZonaPagina      = worker.ZonaPagina
	ZonaTitolo      = "titolo"
	ZonaSoggetto    = "soggetto"
)

// DoveBassoDestra e' la zona in basso a destra detta all'operatore: e' un fatto geometrico, e il cartiglio li'
// e' probabile, non certo.
const DoveBassoDestra = "pagina 1, in basso a destra (probabile cartiglio)"

// LetturaTesto e' un codice letto nel testo di un PDF, NORMALIZZATO: e' la forma in cui il testo dei PDF esce
// dalla classificazione verso chi viene dopo (la valutazione, il flusso ancorato F8). Nessuna decisione.
type LetturaTesto struct {
	CodiceTrovato           // il codice, famiglia o generico, con le regole del cliente
	Fonte         string    // FonteTestoCartiglio, FonteTestoPagina, FonteMetadatiPDF, FonteOCR
	Pagina        int       // 0 per i metadati
	Zona          string    // ZonaBassoDestra, ZonaPagina, ZonaTitolo, ZonaSoggetto
	Riquadro      []float64 // [x0, y0, x1, y1] in punti sulla pagina vista (del campo, se sta in un campo); nil per i metadati
	Etichetta     string    // il campo del cartiglio in cui sta (worker.CampoNumeroDisegno, …), "" se non sta in un campo
	RevCartiglio  string    // la revisione del cartiglio (campo «REV»), se nella stessa fonte ce n'e' una sola
	Estratto      string    // la riga (o il campo dei metadati) in cui sta, accorciata
	Intero        bool      // metadati: il campo E' il codice (tolte estensione e rev), non lo cita
	Confidenza    *float64  // solo OCR, se il motore la da'
	DipendeDaNome bool      // ripete il nome del file (Domanda 7 = B): la da' LettureDelPDF, che il nome lo sa
	// DaRileggere: sta nel cartiglio di un testo letto con una sottoversione di prima (testoDiPrima; giro 4, fase
	// 4.6r). E' una chiave di ricerca e un indizio, mai il codice del file: non vota nella valutazione e nel flusso
	// non e' contenuto, finche' «Rianalizza» non porta la sottoversione di oggi.
	DaRileggere bool
}

// Indizio dice se la lettura e' solo un indizio: viene dall'OCR, e fino alla calibrazione (S2) non ha uno
// score, non alza quello di altre letture e non preseleziona niente.
func (l LetturaTesto) Indizio() bool { return l.Fonte == FonteOCR }

// EvidenzeTesto e' quello che il testo di un PDF dice in fatto di codici, fonte per fonte, con le regole di UN
// cliente. Nessuna decisione: la valutazione ne prende il cartiglio di famiglia, i metadati e gli indizi
// dell'OCR (evidenzeCodiceDelTesto), il flusso ancorato le ancore piatte (A-P1, A-P2, A-P3) e le chiavi di
// ricerca.
type EvidenzeTesto struct {
	Estraibile bool
	// Cartiglio: i codici del campo del codice del cartiglio (codice o numero di disegno) nel testo nativo della
	// zona in basso a destra della pagina 1, di famiglia e generici (A-P1). Gli altri codici della zona no
	// (campoDelCodice). In un testo di una sottoversione di prima ci sono tutti i codici della zona, segnati
	// DaRileggere (fase 4.6r): i campi del worker di prima non dicono quale sia quello del disegno.
	Cartiglio []LetturaTesto
	// Metadati: i codici del titolo e del soggetto (A-P2).
	Metadati []LetturaTesto
	// Altrove: i codici del resto del testo nativo, che non stanno gia' in Cartiglio o nei metadati. Solo
	// chiavi di ricerca (P33, A-P3).
	Altrove []LetturaTesto
	// OCR: i codici letti dall'OCR, in qualunque zona. Indizi.
	OCR []LetturaTesto
	// Simili: i codici del campo «particolare simile» (etichettaSimile, e dalla sottoversione 2 il campo
	// worker.CampoParticolareSimile), in qualunque fonte. Non sono il codice del file, ne' una lettura del testo,
	// ne' una chiave dell'indice: sono la nota «simile a X».
	Simili []LetturaTesto
	// Speculari: i codici di «SPECULARE DI X», «SPECCHIATO DI X» (etichettaSpeculare), in qualunque fonte. Come i
	// simili: la nota «speculare di X», mai un codice.
	Speculari []LetturaTesto
}

// Il «particolare simile» (giro 4, fase 4.2; risposta 4 del 29/09; domanda 3 del giro 4, A finche' l'utente non
// risponde): nel cartiglio di certi clienti un campo dice di quale pezzo gia' fatto questo e' parente
// («PARTICOLARE SIMILE / SIMILAR PART 7120012»). Quel codice non e' il codice del file e non e' un componente:
// e' una nota, «simile a 7120012». Senza questa regola le letture lo mettevano fra i codici del cartiglio (con la
// famiglia del cliente, a 85), e con il cartiglio che conta come contenuto nel flusso (F8) una rianalisi avrebbe
// dato una discordanza su quasi ogni disegno di quel cliente. Il segnaposto del modello («Inserire codice
// particolare simile») non da' niente. Dalla sottoversione 2 del testo (fase 4.6) il worker riporta il campo
// (worker.CampoParticolareSimile, leggiCampo); qui si riconosce anche l'etichetta nel testo, nei frammenti e nei
// valori degli altri campi, per i testi di prima e per le impaginazioni in cui il worker non lega il valore.
var etichettaSimile = regexp.MustCompile(`(?i)` + etichettaSimileRE)

// etichettaSimileRE sono le scritture dell'etichetta del particolare simile, per etichettaSimile e per il
// segnaposto (segnapostoSimile), che la contiene.
const etichettaSimileRE = `(?:PARTICOLARE\s+SIMILE|PART\s*\.?\s*SIMILE|SIMILAR\s+PART)`

// segnapostoSimile e' il testo del modello nel campo vuoto, «Inserire codice particolare simile», in maiuscolo o
// in minuscolo, anche nelle forme lunghe («Inserire qui il numero di codice del particolare simile», «Insert the
// code of the similar part») e spezzato a capo dentro una cella stretta; nella forma bilingue («... particolare
// simile / similar part») anche l'etichetta gemella dopo la barra e' del segnaposto, anche a capo (e' la barra a
// legarla: un'etichetta vera sulla riga dopo non comincia con la barra). Le parole «particolare simile» che
// contiene non sono un'etichetta: prese per tale, la parola dopo diventava il valore del simile, e nel cartiglio a
// tabella dei disegni veri (la colonna del simile prima, «PART. N°» dopo) quella parola e' il codice del file
// stesso («Inserire codice particolare simile   7120001A1»). Fra «Inserire»/«Insert» e l'etichetta ci sono SOLO
// le parole della frase del modello (paroleSegnaposto), almeno una: il segnaposto vuoto di un altro campo scritto
// prima dell'etichetta vera («Inserire trattamento», «Insert treatment», a capo «PARTICOLARE SIMILE 7120012»), una
// nota («INSERIRE BOCCOLE A PRESSIONE») o la sola parola INSERT (il nome di un pezzo) non sono il segnaposto, e
// l'etichetta dopo resta un'etichetta con il suo valore. Resta il dubbio quando il segnaposto di un altro campo e'
// fatto solo di quelle parole («Inserire codice», a capo «PARTICOLARE SIMILE 7120012»): e' preso per il segnaposto
// del simile, e il valore resta nel testo, fra le letture, dove la discordanza lo mostra (come nel cartiglio a
// tabella, separaSimili). «Insert similar part code», senza parole prima dell'etichetta, non serve prenderlo: dopo
// l'etichetta viene «code», una parola senza cifre, e il testo dopo resta com'e' (separaSimili).
var segnapostoSimile = regexp.MustCompile(`(?i)\b(?:INSERIRE|INSERT)(?:\s+` + paroleSegnaposto + `){1,8}\s+` + etichettaSimileRE +
	`(?:\s*/\s*` + etichettaSimileRE + `)*`)

// paroleSegnaposto e' la lista chiusa delle parole che la frase del segnaposto mette fra «Inserire»/«Insert» e
// l'etichetta, una per una: gli articoli e le preposizioni che la legano (qui, il, lo, la, di, del, dello, della;
// here, the, of) e il nome del codice con le sue abbreviazioni (codice, cod., numero, num., nr., n., n°; code,
// number, no.). Nessun'altra parola: il nome di un altro campo (trattamento, materiale, treatment, ...) o una parola
// qualunque interrompe il segnaposto.
const paroleSegnaposto = `(?:QUI|HERE|IL|LO|LA|THE|DI|DEL|DELLO|DELLA|OF|CODICE|COD\.?|CODE|NUMERO|NUMBER|NUM\.?|NR\.?|NO\.?|N\.?[°º]?)`

// EtichettaSimile e' l'etichetta di una lettura del particolare simile (LetturaTesto.Etichetta, in
// EvidenzeTesto.Simili).
const EtichettaSimile = "particolare_simile"

// NotaSimile e' la nota di un particolare simile, per le letture e per la destinazione.
func NotaSimile(codice string) string { return "simile a " + codice }

// Lo speculare (giro 4, fase 4.6; scenario del 28/09): «SPECULARE DI 7120101», «COMPONENTE SPECCHIATO DI 7120118»
// dicono che il pezzo e' il gemello speculare di un altro (il destro del sinistro). Quel codice non e' il codice del
// file e non e' una seconda lettura: e' una nota, «speculare di 7120101». Prima finiva fra le letture (nella zona
// del cartiglio con la famiglia del cliente, a 85), e nel flusso il primo candidato del disegno di un prodotto
// diventava il suo speculare, cioe' l'altro prodotto. Le forme inglesi («MIRROR OF», «OPPOSITE HAND TO») valgono
// lo stesso. Il valore e' la prima parola dopo la frase, se ha cifre (separaValori).
var etichettaSpeculare = regexp.MustCompile(`(?i)\b(?:SPECCHIAT[OA]|SPECULARE|MIRROR(?:ED)?|OPPOSITE(?:\s+HAND)?)\s+(?:DI|A|OF|TO)\b`)

// EtichettaSpeculare e' l'etichetta di una lettura dello speculare (LetturaTesto.Etichetta, in
// EvidenzeTesto.Speculari).
const EtichettaSpeculare = "speculare"

// NotaSpeculare e' la nota di uno speculare, per le letture e per la destinazione.
func NotaSpeculare(codice string) string { return "speculare di " + codice }

// separaSimili toglie dal testo il valore di ogni campo «particolare simile»: la prima parola dopo l'etichetta
// (o dopo la catena di etichette gemelle, «PARTICOLARE SIMILE / SIMILAR PART»), sulla stessa riga o, se la riga
// finisce con l'etichetta, sulla prima riga non vuota dopo, ma solo quando l'etichetta e' da sola sulla sua riga
// e il valore da solo sulla sua (aCapoDaSolo). In un cartiglio a tabella (una riga di etichette, sotto una riga
// di valori) la parola sotto «PARTICOLARE SIMILE» puo' essere il valore di un'altra colonna, spesso il codice del
// file stesso («PART. N°   PARTICOLARE SIMILE», a capo «7120001A1», con il campo simile vuoto): prenderla
// toglierebbe dal cartiglio il codice vero e ne farebbe una nota sbagliata. Nel dubbio il valore resta nel
// testo: al peggio un simile finisce fra le letture, dove la discordanza lo mostra (il campo dedicato arriva con
// la fase 4.6). Una parola senza cifre non e' un codice e resta (e' l'etichetta di un altro campo, «PART. N°», o
// il segnaposto scritto nel campo). L'etichetta dentro il segnaposto del modello (segnapostoSimile), e la sua
// gemella dopo la barra, non e' un'etichetta: il segnaposto resta nel testo com'e' (non ha cifre), e il testo
// dopo, che nel cartiglio a tabella e' spesso il codice del file, resta com'era, sulla stessa riga o sotto; il
// segnaposto di un altro campo prima dell'etichetta vera non la spegne. Restituisce il testo senza quei valori e
// i valori tolti.
func separaSimili(testo string) (string, []string) {
	return separaValori(testo, etichettaSimile, segnapostoSimile)
}

// separaSpeculari toglie dal testo il valore di ogni «SPECULARE DI X» (etichettaSpeculare), con la regola di
// separaSimili (fase 4.6): X e' la nota «speculare di X», non un codice del testo. Non c'e' un segnaposto.
func separaSpeculari(testo string) (string, []string) {
	return separaValori(testo, etichettaSpeculare, nil)
}

// separaValori e' separaSimili per un'etichetta qualunque (e il suo segnaposto, se ce n'e' uno).
func separaValori(testo string, etichetta, segnaposto *regexp.Regexp) (string, []string) {
	if !etichetta.MatchString(testo) {
		return testo, nil
	}
	var segnaposti [][]int
	if segnaposto != nil {
		segnaposti = segnaposto.FindAllStringIndex(testo, -1)
	}
	var b strings.Builder
	var valori []string
	i := 0
	for {
		loc := etichetta.FindStringIndex(testo[i:])
		if loc == nil {
			b.WriteString(testo[i:])
			return b.String(), valori
		}
		if k := fineSegnaposto(segnaposti, i+loc[0]); k > 0 {
			// «particolare simile» dentro «Inserire codice particolare simile»: non un'etichetta, e la parola
			// dopo non e' il suo valore
			b.WriteString(testo[i:k])
			i = k
			continue
		}
		fine := i + loc[1]
		for {
			j := saltaSeparatori(testo, fine, false)
			l := etichetta.FindStringIndex(testo[j:])
			if l == nil || l[0] != 0 {
				break
			}
			fine = j + l[1]
		}
		b.WriteString(testo[i:fine])
		v0 := saltaSeparatori(testo, fine, false)
		aCapo := v0 < len(testo) && (testo[v0] == '\n' || testo[v0] == '\r')
		if aCapo {
			v0 = saltaSeparatori(testo, v0, true)
		}
		v1 := v0
		for v1 < len(testo) && !strings.ContainsRune(" \t\r\n", rune(testo[v1])) {
			v1++
		}
		parola := testo[v0:v1]
		switch {
		case aCapo && !aCapoDaSolo(testo, i+loc[0], v1):
			v1 = fine // forse il valore di un'altra colonna: resta nel testo
		case strings.ContainsAny(parola, "0123456789"):
			valori = append(valori, parola)
			b.WriteString(testo[fine:v0])
			b.WriteByte(' ')
		default:
			v1 = fine // nessun valore: il testo dopo l'etichetta resta com'e'
		}
		i = v1
	}
}

// fineSegnaposto e' dove finisce il segnaposto che contiene la posizione k (0 se k non sta in un segnaposto).
func fineSegnaposto(segnaposti [][]int, k int) int {
	for _, s := range segnaposti {
		if s[0] <= k && k < s[1] {
			return s[1]
		}
	}
	return 0
}

// aCapoDaSolo: l'etichetta che comincia in inizio e' la sola cosa sulla sua riga (prima ci sono solo spazi e
// segni) e la parola che finisce in fine e' la sola cosa sulla riga dopo: cosi' il valore sotto l'etichetta non
// e' quello di un'altra colonna di un cartiglio a tabella (separaSimili).
func aCapoDaSolo(testo string, inizio, fine int) bool {
	riga := strings.LastIndexByte(testo[:inizio], '\n') + 1
	if saltaSeparatori(testo, riga, false) != inizio {
		return false
	}
	resto := testo[fine:]
	if k := strings.IndexByte(resto, '\n'); k >= 0 {
		resto = resto[:k]
	}
	return strings.TrimSpace(resto) == ""
}

// saltaSeparatori salta, da i, gli spazi e i segni fra un'etichetta e il suo valore («/», «:», «-», «.»); con
// aCapo salta anche gli a capo.
func saltaSeparatori(testo string, i int, aCapo bool) int {
	for i < len(testo) {
		switch testo[i] {
		case ' ', '\t', '/', ':', '-', '.':
		case '\r', '\n':
			if !aCapo {
				return i
			}
		default:
			return i
		}
		i++
	}
	return i
}

// letturaNote sono i codici dei valori di una nota (il particolare simile, lo speculare) di un frammento o di un
// campo, con le regole del cliente, l'etichetta della nota e il posto della lettura l.
func letturaNote(m *Motore, valori []string, l LetturaTesto, etichetta, parole string) []LetturaTesto {
	var out []LetturaTesto
	for _, v := range valori {
		for _, c := range m.Estrai(Testo{Dove: "cartiglio, " + parole, Corpo: v}).Codici {
			x := l
			x.CodiceTrovato, x.Etichetta, x.Estratto = c, etichetta, tronca(parole+" "+v, maxEstratto)
			out = append(out, x)
		}
	}
	return out
}

// maxEstratto: quanto si tiene della riga intorno a un codice. L'evidenza porta al piu' 200 caratteri
// (A5.14.2); una riga di cartiglio ne ha poche decine.
const maxEstratto = 80

// campoDelCodice dice se un campo del cartiglio e' quello che porta il codice del disegno: il codice («PART N°»,
// «PART NR», «CODICE», …) o il numero di disegno («DISEGNO N.», «DRAWING NO.», …).
//
// Giro 4, fase 4.6 (il bloccante dello scenario del 28/09): nella zona in basso a destra della pagina 1 non c'e'
// solo il cartiglio. Sui disegni d'assieme di certi clienti c'e' anche l'ELENCO PARTICOLARI, i figli con la
// quantita', subito sopra il cartiglio, e a volte la frase «SPECULARE DI X». Prima ogni codice di famiglia della
// zona era una lettura del codice del disegno (pdf_testo_famiglia, 85): a parita' di score vinceva il primo
// nell'ordine del file, cioe' la prima riga dell'elenco (un figlio), e il taglio a MaxEvidenze toglieva il codice
// vero e la lettura del nome. Adesso nella zona del cartiglio il codice del disegno lo dice soltanto il CAMPO che
// lo porta; gli altri codici della zona non sono letture del codice del disegno ne' chiavi dell'indice (l'elenco
// particolari sara' la distinta del disegno, fase 4.12). Un disegno il cui campo del codice il worker non
// riconosce non ha un codice dal cartiglio, e resta quello del nome: meno automatismo, mai un pezzo sbagliato; le
// etichette che mancano le dice il censimento (4.17).
func campoDelCodice(etichetta string) bool {
	return etichetta == worker.CampoCodice || etichetta == worker.CampoNumeroDisegno
}

// EvidenzeTestoPDF cerca i codici nella lettura del testo di un PDF con le regole del cliente (m, anche nil:
// allora solo l'estrattore generico), fonte per fonte. Pura. Il riferimento della richiesta del cliente (RDO)
// non e' un codice (Estrai lo toglie), e un codice gia' visto in una fonte piu' forte non si ripete in una piu'
// debole del testo nativo: in basso a destra prima, poi i metadati, poi il resto. L'OCR fa una lista sua. Una
// lettura che sta in un campo del cartiglio ne porta l'etichetta e il riquadro; la rev del cartiglio passa alle
// letture della zona in basso a destra della stessa fonte quando e' una sola. Nella zona in basso a destra del
// testo nativo restano soltanto i codici del campo del codice (campoDelCodice, fase 4.6); in un testo di una
// sottoversione di prima restano tutti, segnati DaRileggere (fase 4.6r). Il particolare simile e lo speculare sono
// note, in ogni fonte. DipendeDaNome resta falso: il nome del file lo sa LettureDelPDF.
func EvidenzeTestoPDF(m *Motore, t worker.TestoPDF) EvidenzeTesto {
	e := EvidenzeTesto{Estraibile: t.Estraibile}
	var visti, vistiOCR = map[string]bool{}, map[string]bool{}
	nuove := func(v map[string]bool, in []LetturaTesto) []LetturaTesto {
		var out []LetturaTesto
		for _, c := range in {
			if k := strings.ToUpper(c.Codice); !v[k] {
				v[k] = true
				out = append(out, c)
			}
		}
		return out
	}
	revNativa, revOCR := revDelCartiglio(t, worker.FonteTestoNativo), revDelCartiglio(t, worker.FonteTestoOCR)
	vistiSimili, vistiSpeculari := map[string]bool{}, map[string]bool{}
	// note tiene da parte le letture del particolare simile e dello speculare: una nota, mai un codice
	// (separaSimili, separaSpeculari)
	note := func(l lettureDi) []LetturaTesto {
		e.Simili = append(e.Simili, nuove(vistiSimili, l.simili)...)
		e.Speculari = append(e.Speculari, nuove(vistiSpeculari, l.speculari)...)
		return l.codici
	}
	// soloDelCodice: nella zona del cartiglio il codice del disegno e' soltanto quello del suo campo (fase 4.6).
	// Non in un testo di una sottoversione di prima (fase 4.6r): il worker di prima prendeva l'intestazione
	// dell'elenco particolari per il campo del codice, con il primo figlio come valore, e non conosceva certe
	// etichette («PART N°»): i suoi campi non dicono quale codice della zona sia quello del disegno. Tenere il suo
	// campo del codice rifaceva il bug 3 della Distinta (il disegno d'assieme letto «unico» con il codice del figlio),
	// e toglierlo cancellava la lettura dei cartigli che non capiva. Li' restano tutti i codici della zona, segnati
	// da rileggere: chiavi di ricerca e indizi, mai il codice del file, finche' «Rianalizza» non porta la
	// sottoversione di oggi
	diPrima := testoDiPrima(&t)
	soloDelCodice := func(in []LetturaTesto) []LetturaTesto {
		var out []LetturaTesto
		for _, l := range in {
			switch {
			case diPrima:
				l.DaRileggere = true
				out = append(out, l)
			case campoDelCodice(l.Etichetta):
				out = append(out, l)
			}
		}
		return out
	}
	for _, f := range t.Frammenti {
		if f.Fonte == worker.FonteTestoNativo && f.Pagina == 1 && f.Zona == worker.ZonaBassoDestra {
			e.Cartiglio = append(e.Cartiglio, nuove(visti, soloDelCodice(note(leggiFrammento(m, t, f, FonteTestoCartiglio, revNativa))))...)
		}
	}
	// un campo del cartiglio il cui frammento e' rimasto fuori dai limiti porta lo stesso il suo codice
	for _, c := range t.Cartiglio {
		if c.Fonte == worker.FonteTestoNativo && c.Zona == worker.ZonaBassoDestra {
			e.Cartiglio = append(e.Cartiglio, nuove(visti, soloDelCodice(note(leggiCampo(m, c, FonteTestoCartiglio, revNativa))))...)
		}
	}
	for _, md := range []struct{ zona, testo string }{{ZonaTitolo, t.Metadati.Titolo}, {ZonaSoggetto, t.Metadati.Soggetto}} {
		if strings.TrimSpace(md.testo) == "" {
			continue
		}
		// i metadati non si deduplicano contro il cartiglio: «il titolo dice 7120010» e' un'altra lettura
		// dello stesso codice, e la valutazione la vuole (pdf_metadati)
		intero := codiceIntero(m, md.testo)
		for _, c := range m.Estrai(Testo{Dove: "metadati del PDF: " + md.zona, Corpo: md.testo}).Codici {
			e.Metadati = append(e.Metadati, LetturaTesto{CodiceTrovato: c, Fonte: FonteMetadatiPDF, Zona: md.zona,
				Estratto: estratto(md.testo, c.Codice), Intero: intero != "" && strings.EqualFold(c.Codice, intero)})
		}
	}
	for _, c := range e.Metadati {
		visti[strings.ToUpper(c.Codice)] = true
	}
	for _, f := range t.Frammenti {
		if f.Fonte == worker.FonteTestoNativo && !(f.Pagina == 1 && f.Zona == worker.ZonaBassoDestra) {
			e.Altrove = append(e.Altrove, nuove(visti, note(leggiFrammento(m, t, f, FonteTestoPagina, "")))...)
		}
	}
	for _, c := range t.Cartiglio {
		if c.Fonte == worker.FonteTestoNativo && c.Zona != worker.ZonaBassoDestra {
			e.Altrove = append(e.Altrove, nuove(visti, note(leggiCampo(m, c, FonteTestoPagina, "")))...)
		}
	}
	for _, f := range t.Frammenti {
		if f.Fonte == worker.FonteTestoOCR {
			rev := ""
			if f.Pagina == 1 && f.Zona == worker.ZonaBassoDestra {
				rev = revOCR
			}
			e.OCR = append(e.OCR, nuove(vistiOCR, note(leggiFrammento(m, t, f, FonteOCR, rev)))...)
		}
	}
	for _, c := range t.Cartiglio {
		if c.Fonte == worker.FonteTestoOCR {
			e.OCR = append(e.OCR, nuove(vistiOCR, note(leggiCampo(m, c, FonteOCR, revOCR)))...)
		}
	}
	return e
}

// lettureDi sono i codici di un frammento o di un campo e, a parte, le note che vi sono scritte (il particolare
// simile, lo speculare): le note non sono codici del testo.
type lettureDi struct {
	codici, simili, speculari []LetturaTesto
}

// leggiFrammento sono i codici di un frammento, con il suo posto; un codice che sta nel valore di un campo del
// cartiglio della stessa pagina e fonte prende l'etichetta e il riquadro del campo (campoCon). A parte, le
// letture del particolare simile (separaSimili, e il codice che il campo del particolare simile riporta) e dello
// speculare (separaSpeculari): non sono codici del frammento.
func leggiFrammento(m *Motore, t worker.TestoPDF, f worker.FrammentoPDF, fonte, rev string) lettureDi {
	dove := "testo del PDF, pagina " + strconv.Itoa(f.Pagina)
	if f.Pagina == 1 && f.Zona == worker.ZonaBassoDestra {
		dove = DoveBassoDestra
	}
	if f.Fonte == worker.FonteTestoOCR {
		dove += ", letto con l'OCR"
	}
	corpo, valoriSimili := separaSimili(f.Testo)
	corpo, valoriSpeculari := separaSpeculari(corpo)
	var out lettureDi
	posto := LetturaTesto{Fonte: fonte, Pagina: f.Pagina, Zona: f.Zona, Riquadro: f.Riquadro, Confidenza: f.Confidenza}
	for _, c := range m.Estrai(Testo{Dove: dove, Corpo: corpo}).Codici {
		l := LetturaTesto{CodiceTrovato: c, Fonte: fonte, Pagina: f.Pagina, Zona: f.Zona, Riquadro: f.Riquadro,
			RevCartiglio: rev, Estratto: estratto(f.Testo, c.Codice), Confidenza: f.Confidenza}
		campo, ok := campoCon(t, f, c.Codice)
		switch {
		case ok && campo.Etichetta == worker.CampoParticolareSimile:
			// il codice che il campo del particolare simile riporta (in un cartiglio a tabella non sta accanto
			// all'etichetta, e separaSimili non lo vede): la nota, non un codice del frammento
			valoriSimili = append(valoriSimili, c.Codice)
			continue
		case ok:
			l.Etichetta, l.Riquadro = campo.Etichetta, campo.Riquadro
		}
		out.codici = append(out.codici, l)
	}
	out.simili = letturaNote(m, valoriSimili, posto, EtichettaSimile, "particolare simile")
	out.speculari = letturaNote(m, valoriSpeculari, posto, EtichettaSpeculare, "speculare di")
	return out
}

// campoCon e' il campo del cartiglio della stessa pagina e fonte del frammento f il cui valore contiene il codice:
// prima il campo del particolare simile (il codice e' la nota), poi un campo del codice della stessa zona
// (campoDelCodice), poi un altro campo; mai la revisione, che un codice non e', ne' un campo del codice di
// un'altra zona, che non dice niente di questa.
func campoCon(t worker.TestoPDF, f worker.FrammentoPDF, codice string) (worker.CampoCartiglio, bool) {
	k := strings.ToUpper(codice)
	var delCodice, altro *worker.CampoCartiglio
	for i := range t.Cartiglio {
		c := &t.Cartiglio[i]
		if c.Fonte != f.Fonte || c.Pagina != f.Pagina || c.Etichetta == worker.CampoRevisione ||
			!strings.Contains(strings.ToUpper(c.Valore), k) {
			continue
		}
		switch {
		case c.Etichetta == worker.CampoParticolareSimile:
			if !segnapostoSimile.MatchString(c.Valore) && parolaIn(c.Valore, k) {
				return *c, true
			}
		case campoDelCodice(c.Etichetta):
			if c.Zona == f.Zona && delCodice == nil {
				delCodice = c
			}
		case altro == nil:
			altro = c
		}
	}
	switch {
	case delCodice != nil:
		return *delCodice, true
	case altro != nil:
		return *altro, true
	}
	return worker.CampoCartiglio{}, false
}

// parolaIn dice se k (in maiuscolo) sta nel valore come parola intera, fra due caratteri che non sono lettere ne'
// cifre: il codice del file 7120001 non e' il particolare simile 7120001X.
func parolaIn(valore, k string) bool {
	up := strings.ToUpper(valore)
	for i := 0; ; {
		j := strings.Index(up[i:], k)
		if j < 0 {
			return false
		}
		a, b := i+j, i+j+len(k)
		if (a == 0 || !alfanumerico(up[a-1])) && (b == len(up) || !alfanumerico(up[b])) {
			return true
		}
		i = a + 1
	}
}

func alfanumerico(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

// leggiCampo sono i codici del valore di un campo del cartiglio (non della revisione, che un codice non e'), e a
// parte quelli del particolare simile e dello speculare scritti nel valore. Il campo del particolare simile
// (sottoversione 2 del testo, fase 4.6) e' tutto una nota, con la stessa regola dei frammenti: il suo valore e'
// «simile a X», mai un codice ne' una chiave; il segnaposto del modello («Inserire codice particolare simile») non
// da' niente, nemmeno con un codice accanto, che in un cartiglio a tabella e' il valore di un'altra colonna.
func leggiCampo(m *Motore, c worker.CampoCartiglio, fonte, rev string) lettureDi {
	posto := LetturaTesto{Fonte: fonte, Pagina: c.Pagina, Zona: c.Zona, Riquadro: c.Riquadro, Confidenza: c.Confidenza}
	switch c.Etichetta {
	case worker.CampoRevisione:
		return lettureDi{}
	case worker.CampoParticolareSimile:
		if segnapostoSimile.MatchString(c.Valore) {
			return lettureDi{}
		}
		return lettureDi{simili: letturaNote(m, []string{c.Valore}, posto, EtichettaSimile, "particolare simile")}
	}
	dove := "cartiglio, " + strings.ReplaceAll(c.Etichetta, "_", " ")
	valore, valoriSimili := separaSimili(c.Valore)
	valore, valoriSpeculari := separaSpeculari(valore)
	var out lettureDi
	for _, k := range m.Estrai(Testo{Dove: dove, Corpo: valore}).Codici {
		out.codici = append(out.codici, LetturaTesto{CodiceTrovato: k, Fonte: fonte, Pagina: c.Pagina, Zona: c.Zona, Riquadro: c.Riquadro,
			Etichetta: c.Etichetta, RevCartiglio: rev, Estratto: tronca(c.Letta+" "+c.Valore, maxEstratto), Confidenza: c.Confidenza})
	}
	out.simili = letturaNote(m, valoriSimili, posto, EtichettaSimile, "particolare simile")
	out.speculari = letturaNote(m, valoriSpeculari, posto, EtichettaSpeculare, "speculare di")
	return out
}

// revDelCartiglio e' la revisione scritta nel campo «REV» del cartiglio in basso a destra della pagina 1, per
// una fonte: una sola, o "" (nessuna, o piu' d'una diversa: allora non si sceglie).
func revDelCartiglio(t worker.TestoPDF, fonte string) string {
	rev := ""
	for _, c := range t.Cartiglio {
		if c.Etichetta != worker.CampoRevisione || c.Fonte != fonte || c.Pagina != 1 || c.Zona != worker.ZonaBassoDestra {
			continue
		}
		v := strings.ToUpper(strings.TrimSpace(c.Valore))
		if v == "" || len(v) > 8 {
			continue // una rev e' corta: un valore lungo e' la tabella delle revisioni, non la rev
		}
		if rev != "" && rev != v {
			return ""
		}
		rev = v
	}
	return rev
}

// Chiavi sono i codici del testo nativo e dei metadati, senza ripetizioni e in maiuscolo, nell'ordine delle
// fonti: servono SOLO a cercare nell'indice dei codici della RFQ (P33). Un token che non e' nell'indice non
// dice niente. Quelli dell'OCR restano fuori: sono indizi, e un indizio non trova un componente da solo.
func (e EvidenzeTesto) Chiavi() []string {
	return chiaviDi(e.Cartiglio, e.Metadati, e.Altrove)
}

func chiaviDi(liste ...[]LetturaTesto) []string {
	var out []string
	visti := map[string]bool{}
	for _, l := range liste {
		for _, c := range l {
			if k := strings.ToUpper(c.Codice); !visti[k] {
				visti[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- per chi viene dopo (F9 → F8)

// LettureTestoPDF e' il testo di un PDF come lo vede chi viene dopo la classificazione (il flusso ancorato,
// F8): lo stato, la frase, l'esito dell'OCR e le letture normalizzate, gia' con la dipendenza dal nome del
// file. Nessun campo del worker passa di qui senza essere stato interpretato.
type LettureTestoPDF struct {
	Stato     string // TestoLetto, TestoAssente, TestoNonLetto, TestoIlleggibile, TestoDaAnalizzare (dal DB)
	Frase     string // FraseTestoPDF: "" per un testo letto dal worker di oggi; FraseTestoDiPrima per uno di prima
	OCR       string // lo stato dell'OCR (worker.OCR…), "" se il testo non e' stato letto
	MotivoOCR string
	Troncato  bool // il worker si e' fermato ai limiti: le letture possono non essere tutte
	// DaRileggere: il testo e' di una sottoversione di prima (testoDiPrima, fase 4.6r): le letture del cartiglio
	// sono segnate (LetturaTesto.DaRileggere), e «Rianalizza» lo riaccoda (TestoPDFDaRileggere).
	DaRileggere bool
	// Letture: prima il cartiglio, poi i metadati, poi il resto del testo nativo, poi l'OCR (EvidenzeTesto).
	Letture []LetturaTesto
	// Simili: i codici del particolare simile, in maiuscolo e senza ripetizioni: la nota «simile a X»
	// (NotaSimile), mai una lettura.
	Simili []string
	// Speculari: i codici di «SPECULARE DI X», allo stesso modo: la nota «speculare di X» (NotaSpeculare), mai una
	// lettura (fase 4.6).
	Speculari []string
}

// Chiavi sono le chiavi di ricerca nell'indice (EvidenzeTesto.Chiavi): testo nativo e metadati, non l'OCR.
func (l LettureTestoPDF) Chiavi() []string {
	var nat []LetturaTesto
	for _, x := range l.Letture {
		if !x.Indizio() {
			nat = append(nat, x)
		}
	}
	return chiaviDi(nat)
}

// LettureDelPDF normalizza i fatti di un'analisi di un PDF in letture, per le regole di un cliente (m) e per
// il nome di QUESTO allegato (P15): e' l'unica porta da cui il testo dei PDF esce verso il flusso (F8). Pura.
//
// DipendeDaNome segue la Domanda 7 = B, la stessa regola della valutazione: un codice uguale a quello del nome
// ripete il nome quando viene dai metadati (il titolo di un PDF e' spesso il nome del file da cui e' stato
// stampato), e quando nella sua fonte e nella sua zona sta soltanto dentro il nome del file ripetuto (una
// stampa che riporta «7120010_1.pdf» sotto il disegno). Scritto per conto suo nel cartiglio, lo conferma.
func LettureDelPDF(m *Motore, fatti json.RawMessage, nomeFile string) LettureTestoPDF {
	t, stato := testoDelPDF(fatti)
	out := LettureTestoPDF{Stato: stato}
	if t == nil {
		out.Frase = FraseTestoPDF(stato, "")
		return out
	}
	out.OCR, out.MotivoOCR, out.Troncato = t.OCR.Stato, t.OCR.Motivo, t.Troncato
	out.Frase, out.DaRileggere = FraseTestoPDF(stato, t.OCR.Stato), testoDiPrima(t)
	if out.DaRileggere && stato == TestoLetto {
		// il testo c'e', ma il suo cartiglio non e' contenuto finche' non si rianalizza (fase 4.6r); un PDF senza
		// testo resta con la frase del suo stato, che dice gia' di piu'
		out.Frase = FraseTestoDiPrima
	}
	e := EvidenzeTestoPDF(m, *t)
	nomeCod, _ := codiceDelNome(m, nomeFile)
	for _, l := range [][]LetturaTesto{e.Cartiglio, e.Metadati, e.Altrove, e.OCR} {
		for _, x := range l {
			x.DipendeDaNome = dipendeDalNome(m, *t, x, nomeFile, nomeCod)
			out.Letture = append(out.Letture, x)
		}
	}
	out.Simili, out.Speculari = chiaviDi(e.Simili), chiaviDi(e.Speculari)
	return out
}

// codiceDelNome e' il codice che si legge dal nome di un file (tolte estensione e rev, nella forma del cliente),
// o "" se il nome non e' un codice: lo stesso della valutazione (Valuta).
func codiceDelNome(m *Motore, nomeFile string) (string, string) {
	nome := strings.TrimSpace(nomeFile)
	c, r := CodiceRev(strings.TrimSuffix(nome, path.Ext(nome)))
	if !sembraCodice(c) {
		return "", ""
	}
	return m.Canonico(c, r)
}

// dipendeDalNome e' la Domanda 7 = B per una lettura del testo (vedi LettureDelPDF).
func dipendeDalNome(m *Motore, t worker.TestoPDF, l LetturaTesto, nomeFile, nomeCod string) bool {
	if nomeCod == "" || !strings.EqualFold(l.Codice, nomeCod) {
		return false
	}
	if l.Fonte == FonteMetadatiPDF {
		return true
	}
	return soloNelNome(m, testoDi(t, l), nomeFile, nomeCod, l.Codice)
}

// testoDi e' il testo della fonte e della zona di una lettura: dove si guarda se il codice c'e' anche fuori
// dal nome del file ripetuto.
func testoDi(t worker.TestoPDF, l LetturaTesto) string {
	fonte := worker.FonteTestoNativo
	if l.Fonte == FonteOCR {
		fonte = worker.FonteTestoOCR
	}
	var b strings.Builder
	for _, f := range t.Frammenti {
		if f.Fonte == fonte && f.Pagina == l.Pagina && f.Zona == l.Zona {
			b.WriteString(f.Testo)
			b.WriteByte('\n')
		}
	}
	for _, c := range t.Cartiglio {
		if c.Fonte == fonte && c.Pagina == l.Pagina && c.Zona == l.Zona {
			b.WriteString(c.Valore)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// codiceIntero e' il codice che un campo dei metadati E' (il titolo «7120010_1.pdf» e' 7120010), o "" se il
// campo ne cita qualcuno fra altre parole.
func codiceIntero(m *Motore, campo string) string {
	s := strings.TrimSpace(campo)
	if ext := path.Ext(s); ext != "" && len(ext) <= 5 {
		s = strings.TrimSuffix(s, ext)
	}
	c, r := CodiceRev(s)
	if !sembraCodice(c) {
		return ""
	}
	c, _ = m.Canonico(c, r)
	return c
}

// estratto e' la riga del testo in cui sta il codice, accorciata intorno a lui.
func estratto(testo, codice string) string {
	up := strings.ToUpper(testo)
	i := strings.Index(up, strings.ToUpper(codice))
	if i < 0 || len(up) != len(testo) {
		return tronca(strings.TrimSpace(testo), maxEstratto)
	}
	inizio := strings.LastIndex(testo[:i], "\n") + 1
	fine := len(testo)
	if j := strings.Index(testo[i:], "\n"); j >= 0 {
		fine = i + j
	}
	riga := strings.TrimSpace(testo[inizio:fine])
	if r := []rune(riga); len(r) > maxEstratto {
		// intorno al codice: da un po' prima, per il tetto
		k := len([]rune(testo[inizio:i]))
		da := k - maxEstratto/4
		if da < 0 {
			da = 0
		}
		if da+maxEstratto > len(r) {
			da = len(r) - maxEstratto
		}
		riga = "…" + strings.TrimSpace(string(r[da:da+maxEstratto])) + "…"
	}
	return riga
}

func tronca(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// ---------------------------------------------------------------- nella valutazione

// RegolaOCRIndizio e' la «regola» di un codice letto con l'OCR nella valutazione di un file. Non e' una riga
// della tabella S1 (Domanda 3 = A: la tabella non cambia, e l'impronta nemmeno): un indizio non ha score
// finche' la calibrazione S2 non l'ha misurato. L'evidenza porta il codice in Indizio, non in Valore, e cosi'
// non vota: non fa una fonte, non alza lo score, non fa discordanza e non preseleziona.
const RegolaOCRIndizio = "pdf_ocr_indizio"

// RegolaCartiglioDiPrima e' la «regola» di un codice di famiglia del cartiglio di un testo letto con una
// sottoversione di prima (LetturaTesto.DaRileggere; giro 4, fase 4.6r). Come l'OCR: un indizio senza score, fuori
// dalla tabella S1, che si mostra e non vota. Senza, quel testo si leggeva con la regola della 4.6 (vota il solo
// campo del codice) e il campo del worker di prima, per un disegno d'assieme l'intestazione dell'elenco
// particolari con il primo figlio come valore, votava a 85 da solo: il bug 3 della Distinta, finche' nessuno
// rianalizzava.
//
// Non vota, ma non tace (fase 4.6r2): se dice un codice diverso da quello che vince, la dimensione e' discorde
// (Componi, contraddiceDiPrima). Quando era soltanto un indizio (fase 4.6r), un disegno d'assieme letto dal worker
// di prima con un nome che non e' il codice del cartiglio («ACME-030P7120100.pdf») aveva il codice «unico» dal
// nome, e il cartiglio che diceva altro spariva dallo stato: prima della 4.6 lo stesso file era discorde (ogni
// codice della zona votava a 85).
const RegolaCartiglioDiPrima = "pdf_cartiglio_da_rileggere"

// Indizi sono le regole delle evidenze senza score, fuori dalla tabella S1, con le parole per la schermata.
var Indizi = map[string]RegolaScore{
	RegolaOCRIndizio: {DimCodice, "cartiglio", 0, "codice letto con l'OCR: indizio, senza score finché non è calibrato", false,
		"l'OCR è un'evidenza, non un automatismo (27/09 «ter»): si mostra, non pesa"},
	RegolaCartiglioDiPrima: {DimCodice, "cartiglio", 0, "codice del cartiglio letto con il worker di prima: indizio, senza score finché il PDF non si rianalizza", false,
		"il worker di prima non distingueva il campo del codice dall'intestazione dell'elenco particolari (fase 4.6): si mostra, non pesa; un codice diverso da quello che vince rende le fonti discordi"},
}

// evidenzeCodiceDelTesto sono le letture del codice che il testo di un PDF porta nella valutazione del file
// (A5.14.3): ogni codice di FAMIGLIA del testo nativo in basso a destra della pagina 1 (pdf_testo_famiglia), il
// codice del titolo o del soggetto (pdf_metadati: quello che il campo E', o un codice di famiglia che vi e'
// citato), e come indizi senza voto i codici di famiglia che l'OCR ha letto in basso a destra della pagina 1 e
// quelli del cartiglio di un testo letto con il worker di prima (RegolaCartiglioDiPrima, fase 4.6r: se dicono un
// codice diverso da quello che vince, la dimensione e' discorde, fase 4.6r2). Gli altri non votano.
//
// Domanda 7 = B, con le letture normalizzate (LettureDelPDF): il titolo o il soggetto con lo stesso codice del
// nome DIPENDONO dal nome; il codice in basso a destra uguale al nome e' una conferma INDIPENDENTE (e
// concorde), tranne quando in quella zona sta soltanto dentro il nome del file ripetuto; un codice diverso dal
// nome non dipende da niente, ed e' la discordanza.
func evidenzeCodiceDelTesto(m *Motore, fatti json.RawMessage, nomeFile string) []Evidenza {
	lt := LettureDelPDF(m, fatti, nomeFile)
	var out []Evidenza
	// un campo dei metadati che E' un codice dice quello e basta, come il nome del file: una famiglia che trova
	// un codice dentro («7120010A» contiene 7120010) non ne fa una seconda lettura (famigliaDi, per il nome)
	intero := map[string]bool{}
	for _, c := range lt.Letture {
		if c.Fonte == FonteMetadatiPDF {
			intero[c.Zona] = intero[c.Zona] || c.Intero
		}
	}
	vistiMeta, vistiOCR := map[string]bool{}, map[string]bool{}
	for _, c := range lt.Letture {
		k := strings.ToUpper(c.Codice)
		switch c.Fonte {
		case FonteTestoCartiglio:
			if c.Origine != "famiglia" {
				continue // un codice generico in basso a destra e' una chiave di ricerca, non il codice del file
			}
			e := evidenza("pdf_testo_famiglia", k, c.Estratto)
			e.Famiglia, e.Dove = c.Famiglia, DoveBassoDestra
			if c.DaRileggere {
				// il cartiglio di un testo letto con il worker di prima (fase 4.6r): un indizio, che non vota; con
				// un codice diverso da quello che vince fa le fonti discordi (Componi, fase 4.6r2), e lo dice
				e = Evidenza{Regola: RegolaCartiglioDiPrima, Fonte: Indizi[RegolaCartiglioDiPrima].Fonte, Indizio: k,
					Testo: tronca(c.Estratto, 200), Famiglia: c.Famiglia, Dove: DoveBassoDestra + ", " + FraseCartiglioDiPrima(k)}
			}
			if c.DipendeDaNome {
				e.DipendeDa = "nome_file"
			}
			out = append(out, e)
		case FonteMetadatiPDF:
			switch {
			case intero[c.Zona] && !c.Intero:
				continue // un pezzo del codice che il campo e'
			case !c.Intero && c.Origine != "famiglia":
				continue // il campo cita un numero fra altre parole: chiave di ricerca
			case vistiMeta[k]:
				continue // titolo e soggetto con lo stesso codice sono una lettura sola
			}
			vistiMeta[k] = true
			e := evidenza("pdf_metadati", k, c.Zona+" «"+c.Estratto+"»")
			e.Famiglia, e.Dove = c.Famiglia, "metadati del PDF"
			if c.DipendeDaNome {
				e.DipendeDa = "nome_file"
			}
			out = append(out, e)
		case FonteOCR:
			if c.Origine != "famiglia" || c.Pagina != 1 || c.Zona != ZonaBassoDestra || vistiOCR[k] {
				continue
			}
			vistiOCR[k] = true
			r := Indizi[RegolaOCRIndizio]
			e := Evidenza{Regola: RegolaOCRIndizio, Fonte: r.Fonte, Indizio: k, Testo: tronca(c.Estratto, 200),
				Famiglia: c.Famiglia, Dove: DoveBassoDestra + ", letto con l'OCR"}
			if c.DipendeDaNome {
				e.DipendeDa = "nome_file"
			}
			out = append(out, e)
		}
	}
	return out
}

// soloNelNome dice se il codice sta nel testo SOLO dentro il nome del file ripetuto: tolto dal testo il nome
// (con l'estensione, e senza quando il nome e' piu' del codice, come «7120010_1»), il codice non c'e' piu'.
func soloNelNome(m *Motore, testo, nomeFile, nomeCod, codice string) bool {
	nome := strings.TrimSpace(nomeFile)
	base := strings.TrimSuffix(nome, path.Ext(nome))
	senza := togliTutti(testo, nome)
	if base != "" && !strings.EqualFold(base, nomeCod) {
		senza = togliTutti(senza, base)
	}
	for _, c := range m.Estrai(Testo{Dove: "testo", Corpo: senza}).Codici {
		if strings.EqualFold(c.Codice, codice) {
			return false
		}
	}
	return true
}

// togliTutti toglie dal testo ogni occorrenza di s, senza distinguere maiuscole.
func togliTutti(testo, s string) string {
	if s == "" {
		return testo
	}
	up, su := strings.ToUpper(testo), strings.ToUpper(s)
	if len(up) != len(testo) {
		// maiuscole di lunghezza diversa (rare lettere non ASCII): si lavora sul testo in maiuscolo
		testo = up
	}
	var b strings.Builder
	for {
		i := strings.Index(up, su)
		if i < 0 {
			b.WriteString(testo)
			return b.String()
		}
		b.WriteString(testo[:i])
		b.WriteByte(' ')
		testo, up = testo[i+len(su):], up[i+len(su):]
	}
}
