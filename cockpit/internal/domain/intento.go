package domain

import (
	"regexp"
	"strings"
)

// L'INTENTO del messaggio (blocco 7B, 7B.3)
//
// L'esito del triage dice che cosa si propone di FARE (nuova_rfq, aggancia, ignora). L'intento dice
// che cosa il messaggio E': la richiesta di un cliente, l'offerta di un fornitore, una nostra
// richiesta a un fornitore, una newsletter. Sono due cose diverse: un'offerta di fornitore ha esito
// «aggancia» verso una richiesta, non verso una RFQ nuova, e un «ignora» puo' essere una newsletter
// o un messaggio che nessuno sa leggere. Il ramo lo decide la controparte (7A), non un punteggio.
//
// Gli intenti sono l'enum `intento_messaggio` della 0015: stringhe qui, enum in database.
const (
	IntentoRfqCliente        = "rfq_cliente"
	IntentoOffertaPromatec   = "offerta_promatec"
	IntentoRfqFornitore      = "rfq_fornitore"
	IntentoOffertaFornitore  = "offerta_fornitore"
	IntentoRispostaFornitore = "risposta_fornitore"
	IntentoDomandaFornitore  = "domanda_fornitore"
	IntentoInoltroInterno    = "inoltro_interno"
	IntentoNonRfq            = "non_rfq"
	IntentoIncerto           = "incerto"
)

// Le regole della richiesta ai fornitori (candidato_richiesta.regola).
const (
	RichiestaR0Reply         = "R0_reply"
	RichiestaR1Conversazione = "R1_conversazione"
	RichiestaR3fCodice       = "R3f_codice"
	RichiestaRFOggetto       = "RF_oggetto"
)

// PuntiRichiesta e' il punteggio di ogni regola verso una richiesta. Come per R0–R5: un ordine di
// fiducia, non una probabilita'. R0 e' l'unico legame scritto dal programma di posta; R1 e' la
// conversazione della nostra mail; R3f e' un codice che appartiene a una RFQ con una richiesta a
// questo fornitore, e vale meno perche' due RFQ possono chiedere lo stesso codice.
var PuntiRichiesta = map[string]int{RichiestaR0Reply: 95, RichiestaR1Conversazione: 80, RichiestaR3fCodice: 60, RichiestaRFOggetto: 70}

// CandidatoRichiesta e' una proposta «e' la risposta a QUESTA richiesta»: con l'evidenza, come i
// candidati di aggancio. Piu' d'uno si vedono tutti (IB5); nessuno viene scelto dal sistema.
type CandidatoRichiesta struct {
	RichiestaID string
	Regola      string
	Punteggio   int
	Evidenza    string
	Fornitore   string
	Cliente     string
	OggettoRFQ  string
}

// MiglioreCandidatoRichiesta e' il primo per punteggio; con lo stesso punteggio vince l'ordine.
func MiglioreCandidatoRichiesta(c []CandidatoRichiesta) (CandidatoRichiesta, bool) {
	if len(c) == 0 {
		return CandidatoRichiesta{}, false
	}
	best := c[0]
	for _, k := range c[1:] {
		if k.Punteggio > best.Punteggio {
			best = k
		}
	}
	return best, true
}

// reNonRFQ: i segni di posta che non e' di lavoro. Si cerca nel mittente e nell'oggetto, e nel
// corpo SCRITTO ADESSO (non nella storia citata): una newsletter cita «RFQ» quanto vuole, resta
// una newsletter. E' una lista corta di proposito: meglio un «incerto» in Da validare che una
// richiesta vera chiamata newsletter.
var reNonRFQ = regexp.MustCompile(`(?i)(\bunsubscribe\b|\bdisiscri|\bnewsletter\b|\bno-?reply\b|\bnoreply\b|\bdo not reply\b|\bnon rispondere a questa\b|\bmailer-daemon\b|\bundeliverable\b|\bmancata consegna\b|\bout of office\b|\bfuori ufficio\b|\brisposta automatica\b|\bauto-?reply\b|\bautomatic reply\b|\bwebinar\b|\biscriviti\b|\bpromozione\b|\bofferta speciale\b)`)

// NonRFQ dice se il messaggio ha i segni di una newsletter, una notifica automatica, un fuori
// ufficio. Il motivo e' la frase che l'operatore legge.
func NonRFQ(mittente, oggetto, corpo string) (bool, string) {
	mitt := strings.ToLower(strings.TrimSpace(mittente))
	if i := strings.LastIndex(mitt, "@"); i > 0 {
		locale := mitt[:i]
		for _, p := range []string{"noreply", "no-reply", "no_reply", "newsletter", "mailer-daemon", "postmaster", "notification", "notifications", "donotreply"} {
			if strings.HasPrefix(locale, p) || locale == p {
				return true, "mittente «" + mitt + "»: un indirizzo automatico, non una persona"
			}
		}
	}
	testo := oggetto + "\n" + CorpoUtilePerInterpretazione(corpo)
	if m := reNonRFQ.FindString(testo); m != "" {
		return true, "il testo contiene «" + strings.ToLower(strings.TrimSpace(m)) + "»: newsletter, notifica o risposta automatica"
	}
	return false, ""
}

var reOffertaFornitore = regexp.MustCompile(`(?i)\b(offerta|quotazione|preventivo|quotation|quote|offer|prezzo|prezzi|price|pricing|listino|ns\.? offerta|vs\.? richiesta)\b`)
var reDomandaFornitore = regexp.MustCompile(`(?i)(\?|\b(chiediamo|vi chiediamo|potete|potreste|ci serve|serve sapere|servirebbe|could you|can you|please confirm|please advise|confermate|da chiarire|chiarimento|domanda|dubbio|specificare|quale materiale|quanti pezzi)\b)`)
var estensioniOfferta = map[string]bool{"pdf": true, "xls": true, "xlsx": true, "doc": true, "docx": true}

// IntentoFornitore legge una mail IN ENTRATA di un fornitore censito e dice che cos'e': un'offerta
// (parole d'offerta e, di solito, un PDF o un foglio), una domanda (un punto interrogativo o le
// parole di chi chiede), una risposta (tutto il resto), o posta che non e' di lavoro. Non decide
// a quale richiesta appartiene: quello lo fanno i candidati. Il motivo e' per l'operatore.
func IntentoFornitore(mittente, oggetto, corpo string, nomiAllegati []string) (intento, motivo string) {
	if si, m := NonRFQ(mittente, oggetto, corpo); si {
		return IntentoNonRfq, m
	}
	testo := oggetto + "\n" + CorpoUtilePerInterpretazione(corpo)
	documenti := 0
	for _, n := range nomiAllegati {
		if i := strings.LastIndex(n, "."); i >= 0 && estensioniOfferta[strings.ToLower(n[i+1:])] {
			documenti++
		}
	}
	if m := reOffertaFornitore.FindString(testo); m != "" {
		if documenti > 0 {
			return IntentoOffertaFornitore, "parla di «" + strings.ToLower(m) + "» e allega un documento: l'offerta del fornitore"
		}
		return IntentoOffertaFornitore, "parla di «" + strings.ToLower(m) + "»: l'offerta del fornitore, senza documento allegato"
	}
	if m := reDomandaFornitore.FindString(testo); m != "" {
		return IntentoDomandaFornitore, "il fornitore chiede qualcosa (" + strings.TrimSpace(strings.ToLower(m)) + ")"
	}
	if documenti > 0 {
		return IntentoOffertaFornitore, "allega un documento senza parole d'offerta: probabilmente l'offerta"
	}
	return IntentoRispostaFornitore, "risposta del fornitore senza offerta né domanda"
}
