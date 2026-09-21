package classificazione

import (
	"regexp"
	"strings"
)

// IL TAGLIO DELLA CATENA DI RISPOSTA (blocco 6 del checkpoint 3R)
//
// Una mail alla quarta risposta contiene quattro messaggi, e tre sono già stati letti da qualcuno.
// L'interpretazione deterministica però li legge tutti insieme: da lì arrivano quasi tutti i falsi
// multi-codice — «questa richiesta parla di sei codici» quando ne nomina uno e cita gli altri cinque
// dalla conversazione di settembre.
//
// Il corpo ORIGINALE non si tocca. Resta intero in `messaggio.corpo_testo`, resta intero nella
// schermata, resta intero nel messaggio in Outlook: è la prova di che cosa è stato scritto, e un
// sistema che riscrive la posta che riceve non è più una prova di niente. Quello che cambia è quale
// pezzo viene DATO IN PASTO all'interpretazione.
//
// # Perché non basta tagliare alla prima riga che contiene «Da:»
//
// Perché «Da:» e «From:» compaiono nel testo normale: «Da: Mario, ti confermo che…», «From the
// desk of…», «Il pezzo va rifinito. Da: disegno 4, nota 3». Una regex che taglia qualunque riga con
// «Da:» butterebbe via il messaggio vero di chi scrive così — e lo farebbe in silenzio, perché
// nessuno vede mai il testo tagliato.
//
// Qui si taglia solo su qualcosa di NON ambiguo:
//
//	separatore esplicito   «-----Messaggio originale-----», «-----Original Message-----»,
//	                       «---------- Forwarded message ----------»;
//	apertura di citazione  «Il giorno … ha scritto:», «On … wrote:», «Am … schrieb:» — prefisso E
//	                       chiusura sulla stessa riga (o su due, perché i client mandano a capo);
//	blocco di intestazione DUE o più intestazioni di seguito (Da:/Inviato:/A:/Oggetto:), non una
//	                       sola: è questa la differenza fra un'intestazione citata e una frase;
//	testo marcato          due righe consecutive che cominciano con «>».
//
// # Se il taglio non lascia niente, non si taglia
//
// Un inoltro senza commento — «ti giro questa» e sotto la richiesta del cliente — diventerebbe un
// messaggio vuoto, e l'interpretazione non vedrebbe più niente proprio dove c'è tutto. In quel caso
// si tiene il corpo intero: è meglio un codice di troppo, che si vede e si toglie, che una richiesta
// che non arriva sul tavolo di nessuno.

// TagliaCatena divide il corpo in due: quello che è stato scritto ADESSO e la storia citata.
//
// Non perde niente — la somma dei due è il corpo, a meno degli spazi ai bordi — perché chi la chiama
// deve poter usare tutti e due: la storia citata è rumore quando si cerca «di che cosa parla questo
// messaggio», ed è invece la prova migliore quando si cerca «a quale richiesta appartiene».
func TagliaCatena(corpo string) (utile, storia string) {
	righe := strings.Split(normalizza(corpo), "\n")
	i := primaRigaDellaStoria(righe)
	if i < 0 {
		return strings.TrimSpace(corpo), ""
	}
	i = arretra(righe, i)
	utile = strings.TrimSpace(strings.Join(righe[:i], "\n"))
	storia = strings.TrimSpace(strings.Join(righe[i:], "\n"))
	if utile == "" {
		// Il taglio non ha lasciato niente: era un inoltro senza commento. Vedi sopra.
		return strings.TrimSpace(corpo), ""
	}
	return utile, storia
}

// CorpoUtilePerInterpretazione è il testo scritto adesso: quello su cui si cercano il riferimento
// della richiesta, i codici e l'esito del triage. È il nome con cui il checkpoint chiama questa cosa.
func CorpoUtilePerInterpretazione(corpo string) string {
	utile, _ := TagliaCatena(corpo)
	return utile
}

// DoveStoria è l'etichetta con cui la storia citata entra nell'estrazione. Un codice che viene da lì
// si VEDE — sta fra gli «altri numeri trovati» — ma non si propone da solo: era già stato deciso in
// un altro messaggio, e riproporlo qui è il modo in cui una risposta diventa una richiesta di sei
// pezzi.
const DoveStoria = "storia citata"

// normalizza porta tutto a righe separate da \n e toglie gli spazi unificatori che i client di posta
// infilano ovunque: senza, «Da:\u00a0Mario» non è «Da: Mario» e l'intestazione non si riconosce.
func normalizza(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Map(func(r rune) rune {
		switch r {
		case '\u00a0', '\u2007', '\u202f':
			return ' '
		}
		return r
	}, s)
}

func primaRigaDellaStoria(righe []string) int {
	for i := range righe {
		t := pulisciCitazione(righe[i])
		if t == "" {
			continue
		}
		if separatoreEsplicito(t) || aperturaDiCitazione(righe, i) ||
			intestazioneCitata(righe, i) || testoMarcato(righe, i) {
			return i
		}
	}
	return -1
}

// arretra sposta il taglio indietro sulle righe che appartengono già al separatore: la riga di
// underscore che Outlook mette prima dell'intestazione, e le righe vuote in mezzo. Sono cosmetica,
// ma la cosmetica qui è il testo che un domani leggerà l'agente.
func arretra(righe []string, i int) int {
	for i > 0 {
		t := strings.TrimSpace(righe[i-1])
		if t == "" || rigaDiSeparazione(t) {
			i--
			continue
		}
		return i
	}
	return i
}

// rigaDiSeparazione: una riga fatta solo di trattini, underscore o uguali. Da sola non taglia niente
// — è troppo comune sotto una firma — ma se sta appena sopra a un separatore vero gli appartiene.
func rigaDiSeparazione(t string) bool {
	if len(t) < 3 {
		return false
	}
	return strings.Trim(t, "-_=*") == ""
}

// pulisciCitazione toglie gli spazi e i «>» con cui molti client marcano il testo citato: una
// intestazione citata dentro una citazione è comunque un'intestazione citata.
func pulisciCitazione(r string) string {
	r = strings.TrimSpace(r)
	for strings.HasPrefix(r, ">") {
		r = strings.TrimSpace(r[1:])
	}
	return r
}

var frasiSeparatore = map[string]bool{
	"messaggio originale":        true,
	"original message":           true,
	"messaggio inoltrato":        true,
	"forwarded message":          true,
	"inizio messaggio inoltrato": true,
	"begin forwarded message":    true,
	"ursprüngliche nachricht":    true,
	"weitergeleitete nachricht":  true,
}

// separatoreEsplicito riconosce «-----Messaggio originale-----» e i suoi parenti, con qualunque
// numero di trattini (o nessuno: c'è chi scrive solo «Messaggio originale»).
func separatoreEsplicito(t string) bool {
	l := strings.ToLower(t)
	return frasiSeparatore[strings.TrimSpace(strings.Trim(l, "-_=* \t"))]
}

// «Il giorno … ha scritto:» e i suoi equivalenti. Il prefisso E la chiusura devono esserci tutti e
// due: «On the drawing you sent the tolerance is wrong» comincia con «On» e non chiude con «wrote:»,
// quindi non taglia niente.
var apertureCitazione = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^il giorno\b.*\bha scritto\s*:`),
	regexp.MustCompile(`(?i)^on\b.*\bwrote\s*:`),
	regexp.MustCompile(`(?i)^am\b.*\bschrieb\s*:`),
	regexp.MustCompile(`(?i)^le\b.*\ba écrit\s*:`),
}

// aperturaDiCitazione guarda la riga e, se non basta, la riga unita alle due successive: quella
// frase contiene una data e un indirizzo, ed è lunga abbastanza che i client la mandino a capo.
func aperturaDiCitazione(righe []string, i int) bool {
	unita := pulisciCitazione(righe[i])
	for j := i; j < len(righe) && j <= i+2; j++ {
		if j > i {
			unita += " " + pulisciCitazione(righe[j])
		}
		for _, re := range apertureCitazione {
			if re.MatchString(unita) {
				return true
			}
		}
	}
	return false
}

// Le intestazioni di un messaggio citato, in italiano, inglese, tedesco e francese. Il valore è il
// RUOLO: due righe «Da:» e «From:» di seguito sono la stessa informazione ripetuta, non due
// intestazioni diverse, e non devono bastare a far scattare il taglio.
var chiaviIntestazione = map[string]string{
	"da": "mittente", "from": "mittente", "von": "mittente", "de": "mittente",
	"a": "destinatario", "to": "destinatario", "an": "destinatario",
	"cc": "copia", "ccn": "copia", "bcc": "copia", "copia": "copia",
	"inviato": "inviato", "sent": "inviato", "gesendet": "inviato", "envoyé": "inviato",
	"data": "data", "date": "data", "datum": "data",
	"oggetto": "oggetto", "subject": "oggetto", "betreff": "oggetto", "objet": "oggetto",
}

// chiaveIntestazione restituisce il ruolo di «Da:», «Subject:», … oppure "" se la riga non comincia
// con un'intestazione. I due punti devono stare VICINO all'inizio: «Il giorno 16/09 alle 14:32» ha
// dei due punti, ma a trentaquattro caratteri di distanza, e non è un'intestazione di niente.
func chiaveIntestazione(t string) string {
	i := strings.Index(t, ":")
	if i <= 0 || i > 12 {
		return ""
	}
	return chiaviIntestazione[strings.ToLower(strings.TrimSpace(t[:i]))]
}

// Un'intestazione citata porta sempre con sé almeno una di queste tre cose: l'indirizzo di posta di
// chi ha scritto, la data in cui l'ha scritto, o l'oggetto del messaggio. Sono i valori, non le
// etichette, a distinguerla — ed è quello che serve, perché le etichette da sole le scrive chiunque.
var reIndirizzoCitato = regexp.MustCompile(`[^\s<>@]+@[^\s<>@]+\.[A-Za-z]{2,}`)
var reDataCitata = regexp.MustCompile(`(?i)\d{1,2}:\d{2}|\d{1,2}[/.\-]\d{1,2}[/.\-]\d{2,4}|` +
	`\b(gennaio|febbraio|marzo|aprile|maggio|giugno|luglio|agosto|settembre|ottobre|novembre|dicembre|` +
	`january|february|march|april|may|june|july|august|september|october|november|december|` +
	`jan|feb|mar|apr|jun|jul|aug|sep|sept|oct|nov|dec)\b`)

// intestazioneCitata: qui sta la risposta all'avvertimento del checkpoint.
//
// Una riga «Da: …» da sola non taglia NIENTE. Ne servono almeno due, di ruolo diverso, una dietro
// l'altra; e la prima deve dire chi ha scritto, quando, o di che cosa — un «A:» isolato in mezzo a
// un elenco non apre nessuna storia.
//
// Non basta ancora. Queste tre righe
//
//	Riepilogo lavorazione del 6674611A:
//	Da: tornitura
//	A: rettifica
//
// hanno due etichette di ruolo diverso una dietro l'altra, e non sono l'intestazione di niente: sono
// un ciclo di lavorazione. La differenza non sta nelle etichette ma nei VALORI — un'intestazione vera
// porta un indirizzo di posta, una data, o l'oggetto del messaggio citato — e senza questo controllo
// il ciclo di lavorazione di un pezzo sparirebbe dal testo, in silenzio.
func intestazioneCitata(righe []string, i int) bool {
	primo := chiaveIntestazione(pulisciCitazione(righe[i]))
	if primo != "mittente" && primo != "oggetto" && primo != "inviato" {
		return false
	}
	viste := map[string]bool{primo: true}
	blocco := pulisciCitazione(righe[i])
	vuote := 0
	for j := i + 1; j < len(righe) && j <= i+6; j++ {
		t := pulisciCitazione(righe[j])
		if t == "" {
			if vuote++; vuote > 1 {
				break
			}
			continue
		}
		k := chiaveIntestazione(t)
		if k == "" {
			break // la riga dopo è prosa: quello sopra non era un blocco di intestazioni
		}
		viste[k] = true
		blocco += "\n" + t
	}
	if len(viste) < 2 {
		return false
	}
	return viste["oggetto"] || reIndirizzoCitato.MatchString(blocco) || reDataCitata.MatchString(blocco)
}

// testoMarcato: due righe consecutive che cominciano con «>». Una sola non basta — capita di
// cominciare una riga con una freccia — due sono una citazione.
func testoMarcato(righe []string, i int) bool {
	n := 0
	for j := i; j < len(righe) && n < 2; j++ {
		t := strings.TrimSpace(righe[j])
		if t == "" {
			continue
		}
		if !strings.HasPrefix(t, ">") {
			return false
		}
		n++
	}
	return n >= 2
}
