// Package classificazione contiene le regole pure del Cockpit: nessun HTTP, nessun DB.
// Prende struct in ingresso e restituisce struct/errori; è qui che si concentrano i test unitari.
package classificazione

import (
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

// reCodice riconosce i codici prodotto tipici dei clienti: almeno 5 caratteri, almeno 3 cifre,
// lettere/cifre con eventuali separatori interni (1234567A, 1234567A_4, 12-34567, TS9 8712-4).
var reCodice = regexp.MustCompile(`\b[A-Z0-9]{2,}(?:[-_/][A-Z0-9]+)*\b`)

// parole che compaiono nelle mail RFQ e che non sono codici
var stopCodici = map[string]bool{
	"RFQ": true, "RDO": true, "REV": true, "PDF": true, "STEP": true, "STP": true, "DXF": true, "DWG": true, "ZIP": true,
	"ISO": true, "UNI": true, "EN": true, "RE": true, "FW": true, "FWD": true, "TR": true, "CAD": true, "MM": true, "KG": true,
	"S235": false, "S355": false,
}

// EstraiCodici restituisce i possibili codici prodotto presenti in un testo, deduplicati, in ordine di apparizione.
func EstraiCodici(testi ...string) []string {
	visti := map[string]bool{}
	var out []string
	for _, t := range testi {
		// FIX: Rimuoviamo gli URL prima di cercare i codici per evitare falsi positivi
		// generati dai link di tracciamento (es. newsletter, Teams, ecc.)
		tSenzaURL := reURL.ReplaceAllString(t, " ")
		// e i numeri della firma e dei piè di pagina, che hanno la forma di un codice (Smistamento 4.13)
		tSenzaURL = senzaNumeriDiFirma(tSenzaURL)

		// Ora facciamo la ricerca sul testo pulito
		for _, m := range reCodice.FindAllString(strings.ToUpper(tSenzaURL), -1) {
			if !sembraCodice(m) || visti[m] {
				continue
			}
			visti[m] = true
			out = append(out, m)
		}
	}
	return out
}

// I NUMERI DELLA FIRMA (Smistamento 4.13).
//
// Il piè di pagina di una mail porta numeri che hanno la forma di un codice e non lo sono: il CAP
// dell'indirizzo, la legge sulla privacy («D.Lgs. 999/2099»), i segnaposto che Exchange dà agli allegati
// incorporati («ATT00001.bin», i loghi della firma), il numero di una tabella del cliente («COD-12-345»).
// Da ogni mail del portale di un cliente l'estrattore generico ne tirava fuori quattro o cinque: finivano
// fra gli «altri numeri» e, per un cliente senza famiglie, fra i codici proponibili.
//
// CAP e leggi si riconoscono dalla forma INTORNO, non dal numero: un CAP è cinque cifre dopo «CAP», o
// dopo il nome e il numero civico di una via, o davanti a un luogo con la sigla di una provincia, ma
// solo vicino a un indirizzo o nella firma; una legge è n/aaaa dopo le parole della legge. Cinque cifre
// da sole restano un candidato, perché possono essere un codice vero: «71200 Boccola (PA)» in un
// elenco di codici ha la forma di «40999 Città (BO)», e le sigle dei materiali e dei lati (PA, PE, AL,
// CR…) coincidono con quelle delle province. E una via comincia dove comincia un indirizzo (a capo o
// dopo un separatore): in «vi inviamo via PEC il disegno 71200» «via» è la preposizione. I segnaposto
// ATT e la forma COD-nn-nnn invece non sono mai un codice prodotto. Il gruppo 1 di ogni regex è il
// numero da togliere: il resto del testo resta com'è.
var reNumeriDiFirma = []*regexp.Regexp{
	// CAP dopo la parola («CAP 40999», «C.A.P.: 40999») e nella forma postale «I-40999»
	regexp.MustCompile(`(?:(?i:\bc\.?a\.?p\.?)\s*:?\s*|\bI-)(\d{5})\b`),
	// CAP in una riga d'indirizzo: «Via dell'Esempio 1 - 40999 Città», la via col civico (viaConCivico),
	// poi una virgola, un trattino o un a capo, poi il CAP e il luogo
	regexp.MustCompile(viaConCivico + `(?:[ \t]?[A-Za-z]\b|/\w+)?[ \t]*(?:[,\-–]|\r?\n)\s*(\d{5})[ \t]+\p{Lu}`),
	// leggi e regolamenti: «D.Lgs. 999/2099», «Regolamento (UE) 2099/679», «Legge n. 123/2099»
	regexp.MustCompile(`(?i)\b(?:d\.?\s?lgs\.?|decreto legislativo|legge|l\.|d\.?p\.?r\.?|reg(?:olamento)?\.?(?:\s*\(?(?:ue|ce)\)?)?|gdpr|direttiva(?:\s*(?:ue|ce))?)\s*(?:n\.?\s*)?(\d{1,4}/\d{2,4})\b`),
	// i segnaposto di Exchange per gli allegati incorporati: ATT00001.bin, ATT00002.htm
	regexp.MustCompile(`(?i)\b(ATT\d{5})\b`),
	// il numero di una tabella del cliente: COD-12-345
	regexp.MustCompile(`(?i)\b(COD-\d{2}-\d{3})\b`),
}

// viaConCivico è l'inizio di una riga d'indirizzo: la parola della via dove comincia un indirizzo (a
// capo, o dopo «:», «,», «;», «|», «(» o un trattino; mai in mezzo a una frase, come in «spedito via
// DHL»), un nome proprio di al più quattro parole senza punteggiatura (la maiuscola, anche dopo «del»,
// «dell'» …) e il numero civico, anche dopo una virgola («Via Esempio, 12»).
const viaConCivico = `(?m)(?:^|[,;:|(\-–])[ \t]*` +
	`(?i:via|viale|v\.le|piazza|p\.zza|piazzale|corso|c\.so|strada|largo|località|loc\.)[ \t]+` +
	`(?:(?i:dell['’]|della|dello|delle|degli|del|dei|di|d['’])[ \t]*)?` +
	`\p{Lu}[\p{L}'’.]*(?:[ \t]+[\p{L}'’.]+){0,3},?[ \t]+\d{1,4}`

var reViaConCivico = regexp.MustCompile(viaConCivico + `\b`)

// reCAPConProvincia è il CAP davanti al luogo con la provincia: «- 40999 Città Esempio (BO)», a capo o
// dopo un separatore; il luogo è al più cinque parole, la sigla una delle province (siglaProvincia).
// Da sola la forma non basta (un elenco di codici con la sigla del materiale ha la stessa forma): vale
// solo vicino a un indirizzo o nella firma (capInFirma).
var reCAPConProvincia = regexp.MustCompile(`(?m)(?:^|[,;|\-–])[ \t]*(\d{5})[ \t]+\p{Lu}[\p{L}'’.\-]*(?:[ \t]+[\p{L}'’.\-]+){0,4}` +
	`[ \t]*\([ \t]*` + siglaProvincia + `[ \t]*\)`)

// reSaluto è la formula di chiusura che apre la firma: una riga che comincia con i saluti.
var reSaluto = regexp.MustCompile(`(?im)^[ \t]*(?:grazie[ \t,.]*(?:e[ \t]+)?|thanks?[ \t,.]*(?:and[ \t]+)?)?` +
	`(?:(?:cordiali|distinti|cari|molti)[ \t]+saluti|saluti|un[ \t]+(?:cordiale[ \t]+)?saluto|cordialmente|` +
	`(?:best|kind|warm)[ \t]+regards|regards|mit[ \t]+freundlichen[ \t]+gr(?:ü|ue)(?:ß|ss)en|` +
	`freundliche[ \t]+gr(?:ü|ue)(?:ß|ss)e|(?:bien[ \t]+)?cordialement|saludos|atentamente)\b`)

// righeDiFirma è quante righe dopo la formula di chiusura si leggono come firma: una firma ne ha
// poche, e più sotto comincia di solito il messaggio citato, con i suoi codici.
const righeDiFirma = 8

// le sigle delle province italiane, per il CAP davanti al luogo (reCAPConProvincia): con anche le
// quattro sarde soppresse (CI, OG, OT, VS), che le firme scrivono ancora
const siglaProvincia = `(?:AG|AL|AN|AO|AP|AQ|AR|AT|AV|BA|BG|BI|BL|BN|BO|BR|BS|BT|BZ|CA|CB|CE|CH|CI|CL|CN|CO|CR|CS|CT|CZ|` +
	`EN|FC|FE|FG|FI|FM|FR|GE|GO|GR|IM|IS|KR|LC|LE|LI|LO|LT|LU|MB|MC|ME|MI|MN|MO|MS|MT|NA|NO|NU|OG|OR|OT|PA|PC|PD|` +
	`PE|PG|PI|PN|PO|PR|PT|PU|PV|PZ|RA|RC|RE|RG|RI|RM|RN|RO|SA|SI|SO|SP|SR|SS|SU|SV|TA|TE|TN|TO|TP|TR|TS|TV|UD|VA|` +
	`VB|VC|VE|VI|VR|VS|VT|VV)`

// senzaNumeriDiFirma toglie dal testo i numeri della firma (reNumeriDiFirma, e reCAPConProvincia dove
// capInFirma lo ammette), uno spazio al loro posto.
func senzaNumeriDiFirma(t string) string {
	for _, re := range reNumeriDiFirma {
		t = togli(t, re.FindAllStringSubmatchIndex(t, -1))
	}
	trovati := reCAPConProvincia.FindAllStringSubmatchIndex(t, -1)
	if len(trovati) == 0 {
		return t
	}
	f := leggiFirma(t)
	var inFirma [][]int
	for _, m := range trovati {
		if f.capInFirma(m[2]) {
			inFirma = append(inFirma, m)
		}
	}
	return togli(t, inFirma)
}

// lettoreFirma è il testo letto UNA VOLTA per capInFirma: le vie col civico e i saluti di tutto il
// testo, e un cursore delle righe. I CAP arrivano in ordine, quindi il cursore e i due indici vanno solo
// avanti, e il filtro costa quanto una lettura del testo.
//
// La prima versione (4.13) rileggeva, per ogni CAP, i saluti di tutto il testo che lo precede e le vie
// delle sue due righe: il costo cresceva col quadrato della lunghezza della mail, e 5000 righe
// «71200 Boccola (PA) …» costavano mezzo minuto dentro l'ingest (TestFirmaInUnaLettura). Gli esiti
// sono gli stessi, per due ragioni:
//   - un saluto sta su una riga sola (nessuna classe di reSaluto prende l'a capo): i saluti di
//     t[:inizioRiga] sono quelli di tutto il testo che finiscono entro inizioRiga. E basta il più vicino,
//     perché gli a capo fra un saluto e la riga del CAP calano andando avanti;
//   - anche una via col civico sta su una riga sola, e non contiene mai un CAP: le sole cifre di una via
//     sono il civico, da una a quattro dopo uno spazio e prima di un confine di parola, mentre il CAP
//     sono cinque cifre non precedute da una cifra (reCAPConProvincia). Per questo «c'è una via in
//     t[inizioPrima:i]» vuol dire «la prima via del testo che comincia da inizioPrima in poi finisce
//     entro i». Se una delle tre regex cambia, TestFirmaInUnaLettura confronta con la lettura di prima.
type lettoreFirma struct {
	t          string
	vie        [][]int // reViaConCivico su tutto il testo, in ordine
	saluti     [][]int // reSaluto su tutto il testo, in ordine
	rigaSaluto []int   // per ogni saluto, gli a capo prima della sua fine

	pos, aCapo              int // il cursore delle righe: gli a capo in t[:pos]
	inizioRiga, inizioPrima int // l'inizio della riga di pos e di quella prima
	via                     int // la prima via che non comincia prima di inizioPrima
	saluto                  int // quanti saluti finiscono entro inizioRiga
}

func leggiFirma(t string) *lettoreFirma {
	f := &lettoreFirma{t: t, vie: reViaConCivico.FindAllStringIndex(t, -1), saluti: reSaluto.FindAllStringIndex(t, -1)}
	f.rigaSaluto = make([]int, len(f.saluti))
	da, n := 0, 0
	for k, s := range f.saluti {
		n += strings.Count(t[da:s[1]], "\n")
		f.rigaSaluto[k], da = n, s[1]
	}
	return f
}

// capInFirma dice se il CAP con la provincia che comincia in i sta vicino a un indirizzo (una via col
// civico sulla stessa riga, prima del CAP, o sulla riga prima) o nella firma (entro righeDiFirma righe
// dopo una formula di chiusura). Le i arrivano in ordine crescente.
func (f *lettoreFirma) capInFirma(i int) bool {
	for {
		k := strings.IndexByte(f.t[f.pos:i], '\n')
		if k < 0 {
			break
		}
		f.pos += k + 1
		f.inizioPrima, f.inizioRiga = f.inizioRiga, f.pos
		f.aCapo++
	}
	f.pos = i
	for f.via < len(f.vie) && f.vie[f.via][0] < f.inizioPrima {
		f.via++
	}
	if f.via < len(f.vie) && f.vie[f.via][1] <= i {
		return true
	}
	for f.saluto < len(f.saluti) && f.saluti[f.saluto][1] <= f.inizioRiga {
		f.saluto++
	}
	return f.saluto > 0 && f.aCapo-f.rigaSaluto[f.saluto-1] <= righeDiFirma
}

// togli mette uno spazio al posto del gruppo 1 di ogni corrispondenza (gli indici di
// FindAllStringSubmatchIndex, in ordine).
func togli(t string, trovati [][]int) string {
	if len(trovati) == 0 {
		return t
	}
	var b strings.Builder
	prec := 0
	for _, m := range trovati {
		b.WriteString(t[prec:m[2]])
		b.WriteByte(' ')
		prec = m[3]
	}
	b.WriteString(t[prec:])
	return b.String()
}

// nomi di file generati da telefoni e strumenti di cattura: non sono codici prodotto
var rePrefissiNonCodice = regexp.MustCompile(`^(SCREENSHOT|IMG|IMAGE|WHATSAPP|DSC|PHOTO|FOTO|SCAN|DOC|PXL|VID)[_\-]?\d`)

// I LIMITI DI UN CODICE (7C.1, P0).
//
// Un codice prodotto e' lungo al massimo MaxCodice caratteri e non contiene spazi; una revisione al
// massimo MaxRev. Sono regole del dominio, non del database: le colonne `codice varchar(60)` e
// `rev varchar(10)` sono piu' larghe apposta, e un test (migrazioni) verifica che restino almeno
// cosi' larghe. Cosi' un valore che passa di qui non puo' rompere una INSERT, e un valore che non
// passa si scarta con il suo nome — non si tronca in silenzio.
//
// Il difetto che ha fatto nascere questa regola: `CodiceRev` restituiva l'intero nome del file
// quando non riconosceva un suffisso di revisione, e `PropostaDaNome` lo scriveva come codice se
// dentro c'era ANCHE UN SOLO token che sembrava un codice. «Offerta 12345678 per fornitura staffe
// zincate rev finale allegato tecnico completo.pdf» diventava un codice di 82 caratteri, e il
// result dello stage — con il file gia' caricato e verificato — veniva rifiutato dal database con
// «value too long for type character varying(60)».
const (
	MaxCodice = 40
	MaxRev    = 10
)

// CodiceAmmissibile dice se una stringa puo' stare nella colonna `codice` di una proposta o di un
// documento: non vuota, entro MaxCodice, senza spazi ne' controlli. Non dice che SIA un codice —
// per quello c'e' sembraCodice — dice che non rompe niente. E' la guardia che il server applica
// a cio' che arriva da fuori (il risultato del worker analisi) prima di scriverlo.
func CodiceAmmissibile(s string) bool {
	if s == "" || len(s) > MaxCodice {
		return false
	}
	return !strings.ContainsFunc(s, spazioOControllo)
}

// RevAmmissibile: come CodiceAmmissibile, per la revisione (MaxRev).
func RevAmmissibile(s string) bool {
	if s == "" || len(s) > MaxRev {
		return false
	}
	return !strings.ContainsFunc(s, spazioOControllo)
}

// spazioOControllo: i caratteri che un codice non contiene. Non solo quelli fino allo spazio ASCII:
// dal cartiglio di un PDF, da un nome di file o da un corpo HTML arrivano lo spazio unificatore
// (U+00A0), quello stretto (U+202F), lo spazio a larghezza zero e gli altri caratteri di formato, e
// «77720517» + U+00A0 + «B» a occhio e' identico a «77720517 B» — due token — ma passava come codice unico.
func spazioOControllo(r rune) bool {
	return r <= ' ' || unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
}

func sembraCodice(s string) bool {
	if len(s) < 5 || len(s) > MaxCodice || stopCodici[s] || rePrefissiNonCodice.MatchString(s) {
		return false
	}
	// un codice non ha spazi dentro: «AB 12345» e' due token, non uno (la stessa regola del
	// worker analisi, `sembra_codice`)
	if strings.ContainsFunc(s, spazioOControllo) {
		return false
	}
	cifre, lettere := 0, 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			cifre++
		case r >= 'A' && r <= 'Z':
			lettere++
		}
	}
	if cifre < 3 {
		return false
	}
	// date (2026-09-08, 08/09/2026), orari, numeri di telefono, CAP
	if lettere == 0 && (strings.Count(s, "/") >= 2 || strings.Count(s, "-") >= 2 || strings.Count(s, ".") >= 2) {
		return false
	}
	if lettere == 0 && strings.HasPrefix(s, "+") {
		return false
	}
	return true
}

// CodiceRev separa "1234567A_4" in codice "1234567A" e rev "4" secondo la convenzione più diffusa
// (suffisso _n, -n, _REVn, _Rn). Se non riconosce nulla restituisce il codice intero e rev vuota.
var reRev = regexp.MustCompile(`^(.+?)[_\-](?:REV|R)?([0-9]{1,2}|[A-Z])$`)

// reRevNas è la forma dei NOSTRI nomi sul NAS (D21, D22): <CODICE>_REV_<REV>, con `_n` in coda per il
// secondo file con lo stesso nome e `ND` per la revisione che non si sa (documenti.NomeTecnico,
// documenti.ConProgressivo). Si prova prima della convenzione generica, che su «X_REV_B» vedeva il
// codice «X_REV» e su «X_REV_B_2» la revisione «2».
var reRevNas = regexp.MustCompile(`^(.+?)_REV_([A-Z0-9][A-Z0-9.\-]{0,9})(?:_[0-9]{1,3})?$`)

func CodiceRev(s string) (codice, rev string) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if m := reRevNas.FindStringSubmatch(s); m != nil {
		if m[2] == "ND" {
			return m[1], "" // «non determinata» non è una revisione: è la sua assenza, scritta
		}
		return m[1], m[2]
	}
	if m := reRev.FindStringSubmatch(s); m != nil {
		return m[1], m[2]
	}
	return s, ""
}

// ---------------------------------------------------------------- portale

var frasiPortale = []string{
	"sul portale", "nel portale", "on the portal", "caricato sul", "caricati sul", "uploaded to", "uploaded on",
	"disponibili sul", "disponibile sul", "scaricare dal", "scaricabili dal", "download from", "in piattaforma",
}

// RiferimentoPortale è la frase della mail che dice "i file sono sul portale", con i codici citati vicino.
type RiferimentoPortale struct {
	TestoCitato string
	Codici      []string
	URL         string
}

var reURL = regexp.MustCompile(`https?://[^\s<>"']+`)

// RilevaPortale cerca nel corpo le frasi tipo "vi abbiamo caricato i CAD sul portale" e restituisce
// al più un riferimento per frase trovata, con i codici presenti nella stessa frase.
//
// `extra` sono le frasi del cliente (`regole.frasi_portale`), che si AGGIUNGONO a quelle generiche
// e non le sostituiscono: «caricato sul portale» resta vero anche per un cliente che ha frasi sue,
// e un cliente con una frase sbagliata non deve smettere di riconoscere quelle di tutti.
func RilevaPortale(corpo string, extra ...string) []RiferimentoPortale {
	frasi := frasiPortale
	if len(extra) > 0 {
		frasi = append(append([]string{}, frasiPortale...), extra...)
	}
	var out []RiferimentoPortale
	for _, frase := range spezzaFrasi(corpo) {
		low := strings.ToLower(frase)
		trovato := false
		for _, f := range frasi {
			if strings.Contains(low, f) {
				trovato = true
				break
			}
		}
		if !trovato {
			continue
		}
		r := RiferimentoPortale{TestoCitato: strings.TrimSpace(frase), Codici: EstraiCodici(frase)}
		if u := reURL.FindString(frase); u != "" {
			r.URL = u
		}
		out = append(out, r)
	}
	return out
}

// reFineFrase compilata una volta: spezzaFrasi la usava ricompilandola a ogni riga di ogni corpo.
var reFineFrase = regexp.MustCompile(`[.;!?]\s+`)

func spezzaFrasi(t string) []string {
	t = strings.ReplaceAll(t, "\r\n", "\n")
	var out []string
	for _, riga := range strings.Split(t, "\n") {
		for _, f := range reFineFrase.Split(riga, -1) {
			if strings.TrimSpace(f) != "" {
				out = append(out, f)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- scadenza

var reData = regexp.MustCompile(`\b(\d{1,2})[/.\-](\d{1,2})[/.\-](\d{2,4})\b`)
var paroleScadenza = []string{"entro", "scadenza", "deadline", "by ", "before", "termine", "due date", "risposta entro"}

// RilevaScadenza cerca una data preceduta da parole tipo "entro"/"scadenza" nelle vicinanze.
func RilevaScadenza(corpo string, riferimento time.Time) (time.Time, bool) {
	low := strings.ToLower(corpo)
	for _, m := range reData.FindAllStringSubmatchIndex(low, -1) {
		inizio := m[0] - 40
		if inizio < 0 {
			inizio = 0
		}
		ctx := low[inizio:m[0]]
		ok := false
		for _, p := range paroleScadenza {
			if strings.Contains(ctx, p) {
				ok = true
				break
			}
		}
		if !ok {
			continue
		}
		d, err := parseData(low[m[2]:m[3]], low[m[4]:m[5]], low[m[6]:m[7]])
		if err != nil || d.Before(riferimento.AddDate(0, 0, -1)) || d.After(riferimento.AddDate(1, 0, 0)) {
			continue
		}
		return d, true
	}
	return time.Time{}, false
}

func parseData(g, m, a string) (time.Time, error) {
	if len(a) == 2 {
		a = "20" + a
	}
	return time.Parse("2/1/2006", g+"/"+m+"/"+a)
}

// ---------------------------------------------------------------- triage deterministico (SPEC §6.3)

type IngressoTriage struct {
	Oggetto      string
	Corpo        string
	NomiAllegati []string
	Direzione    string // entrata | uscita
	Interno      bool   // mittente e destinatari tutti nostri: il collega che gira una mail
	ClienteNoto  bool   // dominio mittente censito
	BuyerNoto    bool   // indirizzo mittente censito come buyer
	// Controparte è chi c'è dall'altra parte, risolta dall'anagrafica (blocco 7A, D33): uno dei
	// Controparte* di controparte.go. Vuota = non risolta (messaggi di prima della 0014, o prove
	// che non la dichiarano): vale come «sconosciuto», cioè il comportamento di sempre.
	//
	// Un `fornitore` in entrata NON può produrre `nuova_rfq`: non è un punteggio da superare, è
	// un ramo che non arriva mai a contare i punti. `ambiguo` non produce nessuna proposta: decide
	// una persona.
	Controparte string
	// Motore sono le regole del cliente riconosciuto, già compilate (voce 6.11). Nil = cliente
	// sconosciuto, o cliente senza regole: il triage continua a funzionare con il solo
	// estrattore generico, perché un cliente non censito è il caso NORMALE del primo giorno.
	Motore *Motore
	// Candidati sono le proposte di aggancio già calcolate (R0–R5, internal/core/inbox/aggancio). La loro
	// presenza è ciò che impedisce di valutare `nuova_rfq`: vedi la precedenza in Triage.
	Candidati []Candidato
	// Blocco 7B: il mittente (per riconoscere gli indirizzi automatici), i candidati verso una
	// richiesta ai fornitori (per la posta IN ENTRATA di un fornitore) e le RFQ aperte che una
	// NOSTRA mail a un fornitore cita (RF_oggetto: la richiesta mandata a mano).
	Mittente           string
	CandidatiRichiesta []CandidatoRichiesta
	RichiesteManuali   []Candidato
	// Smistamento M2: l'In-Reply-To del messaggio, per l'evento. Dice «è una risposta» anche quando
	// l'oggetto non ha il prefisso; non serve all'aggancio, che le chiavi citate le riceve a parte.
	InReplyTo string
}

// ingressoEvento è la parte del messaggio che legge l'evento: tutto tranne i candidati. Con le regole del
// cliente (4.13b), che dicono chi scrive per lui senza essere una persona e come chiama i suoi ordini.
func (in IngressoTriage) ingressoEvento() IngressoEvento {
	return IngressoEvento{Controparte: in.Controparte, Direzione: in.Direzione, Interno: in.Interno,
		Oggetto: in.Oggetto, Corpo: in.Corpo, InReplyTo: in.InReplyTo, NomiAllegati: in.NomiAllegati, Mittente: in.Mittente,
		Motore: in.Motore}
}

// mittenteDiSistema dice se il messaggio viene da un mittente di sistema del cliente (Smistamento 4.13b), con
// la frase dei motivi. Vale per la posta in entrata di un cliente, anche con la controparte non dichiarata
// (il banco di prova e i messaggi di prima della 0014 che hanno comunque il motore del cliente); mai per un
// collega che gira una mail, né per un fornitore. La condizione è quella dell'evento (postaDelCliente, E1b).
func (in IngressoTriage) mittenteDiSistema() (string, bool) {
	if !postaDelCliente(in.Controparte, in.Direzione, in.Interno) {
		return "", false
	}
	ms, ok := in.Motore.MittenteDiSistema(in.Mittente)
	if !ok {
		return "", false
	}
	return "mittente di sistema del cliente: " + descriviMittente(ms) + ", " + ms.Evento +
		". Non è una richiesta nuova, e i suoi codici non si propongono come prodotti", true
}

// Testi sono i pezzi di messaggio in cui si cercano numeri, ognuno con l'etichetta di dove sta.
//
// È un metodo e non tre righe dentro Triage perché lo usano in due: il triage, per proporre un esito,
// e l'ingest, per calcolare i candidati di aggancio PRIMA di chiamare il triage. Se fossero due elenchi
// scritti in due posti, il giorno in cui uno dei due impara a leggere un campo in più l'altro no, e i
// candidati verrebbero calcolati su un testo diverso da quello su cui viene calcolato l'esito.
func (in IngressoTriage) Testi() []Testo {
	// Il corpo arriva TAGLIATO (blocco 6): quello che e' stato scritto adesso sta in «corpo», la
	// catena di risposta precedente sta in «storia citata». Non si perde niente — i codici della
	// storia sono l'evidenza migliore per agganciare una risposta alla sua richiesta — ma i due
	// pezzi non pesano uguale: vedi Proponibili.
	//
	// Il taglio NON si applica ai nomi degli allegati: li' non c'e' nessuna catena di risposta, e un
	// nome di file che comincia per «Da» o contiene «On» non e' una citazione di niente.
	utile, storia := TagliaCatena(in.Corpo)
	testi := []Testo{{Dove: "oggetto", Corpo: in.Oggetto}, {Dove: "corpo", Corpo: utile}}
	if storia != "" {
		testi = append(testi, Testo{Dove: DoveStoria, Corpo: storia})
	}
	for _, n := range in.NomiAllegati {
		base := n
		if i := strings.LastIndex(base, "."); i > 0 {
			base = base[:i]
		}
		testi = append(testi, Testo{Dove: "allegato " + n, Corpo: base})
	}
	return testi
}

type EsitoTriage struct {
	Esito      string // nuova_rfq | aggancia | ignora
	Confidenza int    // 0–100
	Motivi     []string
	Codici     []string
	// Trovati sono gli stessi codici con la loro provenienza (famiglia del cliente o generico):
	// è l'evidenza che la schermata mostra accanto alla proposta. `Codici` resta l'elenco piatto
	// perché è ciò che finisce in `proposta_triage.identificativi`.
	Trovati []CodiceTrovato
	// Riferimento è il numero della richiesta secondo il cliente (RDO, Anfrage, ODA), con il
	// nome della regola che l'ha riconosciuto. Non è un codice prodotto.
	Riferimento     string
	RiferimentoNome string
	// Estrazione è tutto ciò che è stato trovato, già diviso per ruolo: è quello che la schermata
	// mostra come «proponibili» e «altri numeri trovati».
	Estrazione Estrazione
	// Candidato è il candidato di aggancio più forte, quando ce n'è uno. Non è una scelta: è il
	// primo della lista che l'operatore vede per intero.
	Candidato *Candidato
	// Atto è che cosa sta facendo il mittente (7C.0): uno degli Atto* di atto.go, vuoto se la
	// controparte non è stata dichiarata (prove che non la conoscono, messaggi di prima della 0014).
	// Dal M2 viene dall'evento, non dall'esito. Legame è che cosa la mail è rispetto a ciò che il
	// Cockpit conosce già: uno dei Legame*. Sono proposte, come l'esito; il deterministico dice quello
	// che sa e «incerto» dove non sa.
	Atto   string
	Legame string
	// Smistamento M2: l'evento (uno degli Evento*), la sua forza e le evidenze. Si salva come Atto e
	// fra i motivi (MotiviEvento); non tocca Esito, Candidato né Confidenza.
	Evento         string
	ForzaEvento    string
	EvidenzeEvento []string
	// CandidatoRichiesta è la richiesta ai fornitori più forte a cui questa posta risponde.
	CandidatoRichiesta *CandidatoRichiesta
}

// confini di parola: «rdo» e «rfq» compaiono dentro parole comuni (ricordo, bernardo). L'apostrofo
// può essere quello tipografico (U+2019): Outlook e Word lo mettono da soli mentre si scrive, e
// «richiesta d’offerta» è la stessa richiesta di «richiesta d'offerta».
var reParoleRFQ = regexp.MustCompile(`\b(rfq|rdo|richiesta d['’]offerta|richiesta di offerta|richiesta offerta|quotazione|preventivo|quotation|request for quotation|offer request|angebot|devis)\b`)

// estensioniTecniche sono i formati che DA SOLI dicono «qui dentro c'è del disegno»: un file STEP o un
// DXF non è nient'altro. Gli archivi ci stanno perché un allegato compresso in una richiesta d'offerta
// è quasi sempre un pacco di disegni, e comunque l'affermazione riguarda il messaggio, non il file.
//
// PDF, TIF e TIFF NON ci sono più, ed è il punto §5 del checkpoint. Un PDF può essere un disegno, un
// capitolato, un'offerta del fornitore, una conferma d'ordine o la firma di qualcuno: dire «allegato
// tecnico» prima di averlo aperto è un'affermazione su un file che nessuno ha letto. Contano lo stesso,
// ma per quello che sono: allegati di tipo ancora da determinare.
//
// Il disegno (`.dft`) e la lamiera 3D (`.psm`) di Solid Edge ci sono dalla 4.13b, per tutti i clienti:
// sono disegno quanto un DWG o uno STEP.
var estensioniTecniche = map[string]bool{"stp": true, "step": true, "sldprt": true, "sldasm": true,
	"igs": true, "iges": true, "x_t": true, "x_b": true, "dxf": true, "dwg": true, "dft": true, "psm": true,
	"zip": true, "7z": true, "rar": true}

// estensioniDaDeterminare sono documenti che potrebbero essere tecnici e potrebbero non esserlo.
var estensioniDaDeterminare = map[string]bool{"pdf": true, "tif": true, "tiff": true}

// Triage propone un esito per un messaggio orfano. È solo un suggerimento: l'operatore resta l'ultimo a confermare.
//
// L'EVENTO (Smistamento M2, A5.16.2) si calcola a parte, con Evento, che legge solo il messaggio, e
// dà l'atto. L'esito, il candidato e la confidenza restano della precedenza qui sotto: l'evento non li
// sposta. Un'eccezione sola, voluta e vecchia: per un cliente con l'evidenza di una RFQ aperta le
// parole di posta non di lavoro non contano (evidenza_test.go), e l'evento si legge senza E2 — una
// firma con «newsletter» sotto una risposta con In-Reply-To non fa di quella risposta una newsletter.
//
// PRECEDENZA (checkpoint 3R §3). L'esito non è più il risultato di una somma che supera una soglia. Un
// punteggio di contenuto misura quanto un messaggio SEMBRA una richiesta nuova; non può retrocedere una
// risposta di cui esiste la prova. L'ordine è:
//
//  1. c'è evidenza di una RFQ esistente (un candidato R0–R3, o R2, sopra la soglia) → «aggancia», e
//     `nuova_rfq` non viene nemmeno valutata;
//  2. non c'è nessuna evidenza → si contano i punti del contenuto, e sopra 50 si propone `nuova_rfq`;
//  3. sotto → «ignora», cioè «non ho niente da dire»: resta in Inbox, senza proposta.
//
// Il caso che ha aperto il checkpoint: «R: RICHIESTA OFFERTA COD 0.012.3456.7» con un PDF allegato
// faceva 35 (parola «offerta») + 25 (allegato «tecnico») = 60, e diventava una richiesta NUOVA mentre
// era la risposta a una richiesta nostra.
func Triage(in IngressoTriage) EsitoTriage {
	out := triageEsito(in)
	ev := Evento(in.ingressoEvento())
	if in.Controparte == ControparteCliente && EvidenzaDiRFQEsistente(in.Candidati) {
		ev = evento(in.ingressoEvento(), false)
	}
	out.Evento, out.ForzaEvento, out.EvidenzeEvento = ev.Evento, ev.Forza, ev.Evidenze
	// atto vuoto per una controparte non dichiarata: è il comportamento di prima della 0014
	if in.Controparte != "" {
		out.Atto = ev.Atto
	}
	return out
}

// MotiviEvento sono le righe dell'evento da mettere in testa ai motivi salvati.
func (e EsitoTriage) MotiviEvento() []string {
	return MotiviEvento(EsitoEvento{Evento: e.Evento, Atto: e.Atto, Forza: e.ForzaEvento, Evidenze: e.EvidenzeEvento})
}

// triageEsito è la precedenza: esito, candidato, confidenza, motivi e legame. L'atto lo mette Triage.
func triageEsito(in IngressoTriage) EsitoTriage {
	var motivi []string
	punti := 0
	// Una mail in uscita è roba nostra già vista: non c'è niente da smistare. Una mail INTERNA è in
	// uscita anch'essa — parte da un nostro indirizzo — ma «ti giro questa richiesta» è uno dei modi
	// in cui una RFQ arriva davvero sul tavolo, e ignorarla per il mittente significherebbe non
	// proporre niente proprio sui messaggi che un collega ha inoltrato apposta perché qualcuno li
	// guardasse (voce 2.1, D10).
	if in.Direzione == "uscita" && !in.Interno {
		return triageUscita(in)
	}
	if in.Interno {
		motivi = append(motivi, "mail interna: inoltrata da un collega")
	}
	// Blocco 7A: la controparte viene PRIMA dei punti, e non è un punto. Un fornitore che scrive
	// «richiesta d'offerta» con un PDF allegato sta rispondendo a una richiesta nostra, o ne sta
	// facendo una a noi che non è una RFQ cliente: in nessuno dei due casi si apre una RFQ. Se c'è
	// l'evidenza di una nostra richiesta esistente si propone l'aggancio (è la sua offerta),
	// altrimenti non si propone niente. I codici si estraggono lo stesso: la schermata li mostra.
	//
	// Smistamento 4.13b: un mittente di sistema del cliente si sa prima di leggere le parole, come
	// nell'evento (E1b prima di E2): le parole generiche di NonBusiness («noreply») non lo spengono.
	perche, sistema := in.mittenteDiSistema()
	switch in.Controparte {
	case ControparteFornitore:
		return triageFornitore(in, motivi)
	case ControparteAmbiguo:
		motivi = append(motivi, "controparte ambigua: l'indirizzo o il dominio sono censiti sia come cliente sia come fornitore. Decide una persona")
		return EsitoTriage{Esito: "ignora", Confidenza: 0, Motivi: motivi, Codici: []string{}, Legame: LegameIncerto}
	case ControparteSconosciuto:
		// Blocco 7B (D34): da un mittente non censito non nasce una proposta di RFQ. Prima si decide
		// CHI è («Censisci come cliente / fornitore», che ricalcola), poi che cosa vuole.
		return triageSconosciuto(in, motivi)
	case ControparteCliente:
		// L'evidenza viene prima delle parole. NonBusiness legge il testo, e il testo di un cliente
		// vero porta «newsletter», «fuori ufficio», «webinar» nella firma o in una riga di cortesia:
		// senza questa guardia una risposta con In-Reply-To verso una nostra RFQ (R0 a 98) diventava
		// «ignora, non è posta di lavoro». Se un candidato dice che la richiesta esiste ed è aperta,
		// decide lui (più sotto, nella precedenza); le parole contano solo quando non c'è evidenza.
		if !EvidenzaDiRFQEsistente(in.Candidati) && !sistema {
			if si, m := NonBusiness(in.Mittente, in.Oggetto, in.Corpo); si {
				motivi = append(motivi, m, "posta di un cliente che non è di lavoro: se l'indirizzo è automatico, censiscilo come Altro")
				return EsitoTriage{Esito: "ignora", Confidenza: 0, Motivi: motivi, Codici: []string{}, Legame: LegameNessuno}
			}
		}
	}
	// Blocco 6: le parole «richiesta d'offerta» si cercano in quello che e' stato scritto ADESSO.
	// In una catena di risposta quelle parole ci sono sempre — stanno nel primo messaggio — e
	// contarle vorrebbe dire dare trentacinque punti di «sembra una richiesta nuova» a ogni
	// «ricevuto, grazie».
	testo := strings.ToLower(in.Oggetto + "\n" + CorpoUtilePerInterpretazione(in.Corpo))
	if m := reParoleRFQ.FindString(testo); m != "" {
		punti += 35
		motivi = append(motivi, "testo contiene «"+m+"»")
	}
	nTecnici, nIgnoti := 0, 0
	for _, n := range in.NomiAllegati {
		i := strings.LastIndex(n, ".")
		if i < 0 {
			continue
		}
		switch ext := strings.ToLower(n[i+1:]); {
		case estensioniTecniche[ext]:
			nTecnici++
		case estensioniDaDeterminare[ext]:
			nIgnoti++
		}
	}
	if nTecnici > 0 {
		punti += 25
		motivi = append(motivi, "allegati tecnici presenti")
	}
	if nIgnoti > 0 {
		punti += 10
		motivi = append(motivi, "allegati di tipo da determinare (l'analisi dirà che cosa sono)")
	}
	if in.BuyerNoto {
		punti += 25
		motivi = append(motivi, "mittente è un buyer censito")
	} else if in.ClienteNoto {
		punti += 15
		motivi = append(motivi, "dominio mittente è un cliente censito")
	}
	e := in.Motore.Estrai(in.Testi()...)
	// Smistamento 4.13b: un mittente di sistema del cliente (un avviso, un ordine generato da un sistema)
	// non apre una RFQ e non propone i suoi codici come prodotti. I codici si vedono, fra gli altri numeri
	// trovati, e l'aggancio resta quello delle evidenze: un ordine di una RFQ che c'è si aggancia.
	if sistema {
		// in testa: è la frase che spiega perché le parole trovate («rfq», «richiesta») non contano
		e = e.senzaProdotti(perche)
		motivi = append([]string{perche}, motivi...)
	}
	// `Codici` è ciò che può diventare identificativo della RFQ se l'operatore lo spunta. Con un
	// cliente che ha famiglie dichiarate, l'estrattore generico non ci entra (Proponibili).
	codici := dedup(SoloCodici(e.Proponibili()))
	if len(codici) > 0 {
		punti += 10
		motivi = append(motivi, "codici rilevati: "+strings.Join(primi(codici, 4), ", "))
	}
	// Un codice riconosciuto da una FAMIGLIA del cliente vale più di uno pescato dall'estrattore
	// generico: il primo dice «questo è un codice DI QUESTO CLIENTE», il secondo dice «questo ha la forma di
	// un codice». Sono due affermazioni diverse e non devono pesare uguale.
	//
	// E contano solo i PROPONIBILI, come i dieci punti qui sopra: il codice di famiglia che sta nella
	// storia citata dice a quale richiesta si risponde, non che questa ne sia una nuova, e contarlo
	// darebbe punti di «richiesta nuova» proprio alle risposte (la stessa regola del riferimento, sotto).
	if f := primaFamiglia(e.Proponibili()); f != "" {
		punti += 10
		motivi = append(motivi, "codici della famiglia «"+f+"» del cliente")
	}
	// Un riferimento trovato nella STORIA citata non dice che questo messaggio e' una richiesta
	// nuova: dice a quale richiesta risponde, ed e' l'aggancio a occuparsene. Contarlo qui
	// significherebbe far salire il punteggio di «nuova RFQ» proprio sulle risposte.
	if e.Riferimento != "" && e.RiferimentoDove != DoveStoria {
		punti += 15
		motivi = append(motivi, "riferimento "+e.Riferimento+" ("+e.RiferimentoNome+")")
	}
	if punti > 100 {
		punti = 100
	}

	out := EsitoTriage{Confidenza: punti, Codici: codici, Trovati: e.Codici,
		Riferimento: e.Riferimento, RiferimentoNome: e.RiferimentoNome, Estrazione: e}

	// ---- la precedenza. Prima si guarda se la richiesta esiste già, POI se ne sembra una nuova.
	if k, ok := MiglioreCandidato(in.Candidati); ok {
		if a, aperta := MiglioreCandidatoAperto(in.Candidati); aperta {
			// «aggancia» si propone verso la richiesta APERTA più forte, non verso il candidato più
			// forte in assoluto: se quello è una richiesta chiusa, l'esito «aggancia» lo deve a un
			// altro candidato, ed è quello che finisce in `thread_proposto`. Proporre di agganciare a
			// una RFQ chiusa vorrebbe dire far confermare con un clic la cosa che T2 esclude.
			out.Candidato = &a
			motivi = append(motivi, a.Evidenza)
			if k.Chiuso {
				motivi = append(motivi, "c'è anche una richiesta CHIUSA che somiglia di più ("+k.Evidenza+"): si propone quella aperta")
			}
			out.Esito, out.Confidenza = "aggancia", a.Punteggio
			out.Motivi = motivi
			out.Legame = legameCliente(in, out.Esito)
			return out
		}
		// A PARI MERITO (Smistamento M1, P37) la richiesta esiste — l'esito resta «aggancia» e `nuova_rfq`
		// non si valuta — ma quale sia non lo dice il sistema: le prime due RFQ aperte hanno evidenze della
		// stessa forza, e proporne una vorrebbe dire scegliere a caso con l'aria di sapere.
		if g, pari := pariMerito(in.Candidati); pari {
			motivi = append(motivi, FrasePariMerito)
			out.Esito, out.Confidenza, out.Motivi = "aggancia", g.Score, motivi
			out.Legame = legameCliente(in, out.Esito)
			return out
		}
		out.Candidato = &k
		motivi = append(motivi, k.Evidenza)
		// Nessuna evidenza verso una richiesta APERTA. Restano due casi, e in tutti e due il
		// candidato si vede ma non decide: una richiesta chiusa (che è finita), oppure un indizio
		// debole come R5, «stesso buyer di recente» — che ogni richiesta nuova di un buyer noto
		// avrebbe, e che quindi non può impedire di proporne una nuova.
		if c, chiuso := CandidatoChiuso(in.Candidati); chiuso {
			motivi = append(motivi, "attenzione: esiste già una richiesta simile, ma è CHIUSA ("+c.Evidenza+")")
		} else {
			motivi = append(motivi, "l'indizio è debole: non basta a dire che la richiesta esiste già")
		}
	}
	out.Esito = "ignora"
	if punti >= 50 && !sistema {
		out.Esito = "nuova_rfq"
	}
	if sistema {
		// «ignora» come per la posta che non è di lavoro: a zero, perché i punti del contenuto misurano
		// quanto la mail SEMBRA una richiesta, e di questa si sa che non lo è
		out.Confidenza = 0
	}
	if len(motivi) == 0 {
		motivi = []string{"nessun indizio RFQ"}
	}
	out.Motivi = motivi
	out.Legame = legameCliente(in, out.Esito)
	return out
}

// legameCliente è il legame del ramo «cliente» e del ramo «interno» (7C.0).
//
// Fino al M2 dava anche l'atto, e lo ricavava dall'esito: nuova_rfq → richiesta_offerta, il resto
// «incerto». Evento e aggancio erano così la stessa informazione, e una risposta con lo STEP rifatto
// dentro una RFQ esistente non poteva essere una revisione. Dal M2 l'atto viene dall'evento (Triage) e
// qui resta il legame: nuovo, risposta, incerto o nessuno secondo l'esito; per un collega che gira una
// mail è sempre `inoltro`, e l'atto è quello del contenuto girato. Vuoto se la controparte non è
// dichiarata: è il comportamento di prima della 0014, e le prove che non la conoscono lo tengono.
func legameCliente(in IngressoTriage, esito string) string {
	var legame string
	switch esito {
	case "nuova_rfq":
		legame = LegameNuovo
	case "aggancia":
		legame = LegameRisposta
	default:
		legame = LegameNessuno
		if len(in.Candidati) > 0 {
			legame = LegameIncerto
		}
	}
	switch in.Controparte {
	case ControparteInterno:
		return LegameInoltro
	case ControparteCliente:
		return legame
	}
	return ""
}

// triageUscita: una nostra mail. A un fornitore è una richiesta d'offerta (o una risposta a lui):
// se cita un codice di una RFQ aperta, si propone «richiesta a questo fornitore per quella RFQ»
// (RF_oggetto, la richiesta mandata a mano: 7B.2) e la conferma crea la richiesta, mai un thread.
// A un cliente è la nostra offerta o una risposta.
func triageUscita(in IngressoTriage) EsitoTriage {
	out := EsitoTriage{Esito: "ignora", Confidenza: 60, Motivi: []string{"messaggio in uscita"}, Codici: []string{}}
	switch in.Controparte {
	case ControparteFornitore:
		out.Legame = LegameNessuno
		out.Motivi = []string{"nostra mail a un fornitore censito: una richiesta d'offerta a lui, o una risposta"}
		e := in.Motore.Estrai(in.Testi()...)
		out.Trovati, out.Estrazione = e.Codici, e
		// i codici citati nella nostra mail: sono quelli che la richiesta porta con se' alla conferma
		if codici := dedup(SoloCodici(e.Proponibili())); len(codici) > 0 {
			out.Codici = codici
		}
		if k, ok := MiglioreCandidato(in.RichiesteManuali); ok {
			out.Candidato = &k
			// nasce una richiesta (un oggetto nuovo) dentro una RFQ esistente: il legame è «nuovo»
			out.Esito, out.Confidenza, out.Legame = "aggancia", k.Punteggio, LegameNuovo
			out.Motivi = append(out.Motivi, k.Evidenza,
				"se è la richiesta a questo fornitore per quella RFQ, confermala: nasce la richiesta, non una RFQ nuova")
		}
	case ControparteCliente:
		// la regola di sempre, rinominata: una nostra mail a un cliente si presume la nostra
		// offerta. Il legame non si calcola sulla posta in uscita ai clienti: incerto.
		out.Legame = LegameIncerto
		out.Motivi = []string{"nostra mail a un cliente: la nostra offerta, o una risposta"}
	}
	return out
}

// triageFornitore: la posta in entrata di un fornitore censito. Mai `nuova_rfq`, per costruzione.
// L'atto (offerta, domanda, conferma, non di lavoro) viene dal testo; l'esito è «aggancia» se
// c'è una richiesta nostra a cui risponde (candidati richiesta) o una RFQ già collegata (R0/R1),
// altrimenti «ignora» a zero: si vede, e decide una persona.
func triageFornitore(in IngressoTriage, motivi []string) EsitoTriage {
	e := in.Motore.Estrai(in.Testi()...)
	// IB8: nella posta di un fornitore contano SOLO i codici di famiglia dei clienti che hanno
	// richieste aperte a lui (è così che l'ingest costruisce il motore). Un numero pescato
	// dall'estrattore generico non è un codice di nessuno: S235JR, ISO 2768, DIN 933 restano parole.
	e.Codici = DiFamiglia(e.Codici)
	out := EsitoTriage{Codici: dedup(SoloCodici(e.Proponibili())), Trovati: e.Codici,
		Riferimento: e.Riferimento, RiferimentoNome: e.RiferimentoNome, Estrazione: e}
	if out.Codici == nil {
		out.Codici = []string{}
	}
	// l'atto è dell'evento (E3, che chiama lo stesso AttoFornitore): qui serve la frase
	_, perche := AttoFornitore(in.Mittente, in.Oggetto, in.Corpo, in.NomiAllegati)
	out.Legame = LegameNessuno
	motivi = append(motivi, "mittente censito come fornitore: non è una richiesta di un cliente, quindi mai una RFQ nuova", perche)
	if k, ok := MiglioreCandidatoRichiesta(in.CandidatiRichiesta); ok {
		out.CandidatoRichiesta = &k
		motivi = append(motivi, k.Evidenza)
		out.Esito, out.Confidenza, out.Motivi, out.Legame = "aggancia", k.Punteggio, motivi, LegameRisposta
		return out
	}
	if k, ok := MiglioreCandidato(in.Candidati); ok {
		// come nel ramo cliente: «aggancia» va verso la richiesta aperta più forte
		if a, aperta := MiglioreCandidatoAperto(in.Candidati); aperta {
			k = a
		}
		// a pari merito nessuna RFQ si propone (P37), come nel ramo cliente: e nemmeno l'evidenza di una
		// sola delle due finisce fra i motivi, che direbbero di una RFQ ciò che vale per tutte e due
		if g, pari := pariMerito(in.Candidati); pari {
			out.Esito, out.Confidenza, out.Motivi, out.Legame = "aggancia", g.Score, append(motivi, FrasePariMerito), LegameRisposta
			return out
		}
		out.Candidato = &k
		motivi = append(motivi, k.Evidenza)
		out.Legame = LegameIncerto
		if EvidenzaDiRFQEsistente(in.Candidati) {
			out.Esito, out.Confidenza, out.Motivi, out.Legame = "aggancia", k.Punteggio, motivi, LegameRisposta
			return out
		}
	}
	out.Esito, out.Confidenza, out.Motivi = "ignora", 0, motivi
	return out
}

// triageSconosciuto: un mittente che l'anagrafica non conosce. Si estraggono i codici (la schermata
// li mostra) e si riconosce la posta che non è di lavoro; una RFQ nuova non si propone: chi è, lo
// decide una persona con «Censisci», e il ricalcolo fa il resto. Resta l'aggancio se una regola
// forte lo dice (una risposta dentro una catena già agganciata).
func triageSconosciuto(in IngressoTriage, motivi []string) EsitoTriage {
	e := in.Motore.Estrai(in.Testi()...)
	out := EsitoTriage{Esito: "ignora", Confidenza: 0, Codici: dedup(SoloCodici(e.Proponibili())), Trovati: e.Codici,
		Riferimento: e.Riferimento, RiferimentoNome: e.RiferimentoNome, Estrazione: e, Legame: LegameNessuno}
	if out.Codici == nil {
		out.Codici = []string{}
	}
	if si, m := NonBusiness(in.Mittente, in.Oggetto, in.Corpo); si {
		motivi = append(motivi, m) // l'atto non_business lo dà l'evento (E2)
	}
	motivi = append(motivi, "mittente non censito: prima si decide chi è (Censisci come cliente o fornitore), poi che cosa vuole. Nessuna RFQ nuova da qui")
	if k, ok := MiglioreCandidato(in.Candidati); ok {
		// come nel ramo cliente: «aggancia» va verso la richiesta aperta più forte
		if a, aperta := MiglioreCandidatoAperto(in.Candidati); aperta {
			k = a
		}
		if g, pari := pariMerito(in.Candidati); pari {
			// a pari merito nessuna RFQ si propone (P37), come nel ramo cliente, e fra i motivi non va
			// l'evidenza di una sola delle due
			out.Esito, out.Confidenza, out.Legame = "aggancia", g.Score, LegameRisposta
			motivi = append(motivi, FrasePariMerito)
		} else {
			out.Candidato = &k
			motivi = append(motivi, k.Evidenza)
			out.Legame = LegameIncerto
			if EvidenzaDiRFQEsistente(in.Candidati) {
				out.Esito, out.Confidenza, out.Legame = "aggancia", k.Punteggio, LegameRisposta
			}
		}
	}
	out.Motivi = motivi
	return out
}

// primaFamiglia restituisce la descrizione della prima famiglia del cliente che ha riconosciuto
// qualcosa, o "" se hanno parlato solo le regole generiche.
func primaFamiglia(in []CodiceTrovato) string {
	for _, c := range in {
		if c.Origine == "famiglia" {
			return c.Famiglia
		}
	}
	return ""
}

func dedup(in []string) []string {
	visti := map[string]bool{}
	var out []string
	for _, s := range in {
		if !visti[s] {
			visti[s] = true
			out = append(out, s)
		}
	}
	return out
}

func primi(in []string, n int) []string {
	if len(in) <= n {
		return in
	}
	return in[:n]
}

// Ordina è un helper per i test: ordina in place e restituisce lo slice.
func Ordina(s []string) []string { sort.Strings(s); return s }
