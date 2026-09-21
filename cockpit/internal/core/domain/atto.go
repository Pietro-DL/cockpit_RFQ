package domain

import (
	"regexp"
	"strings"
)

// L'ATTO BUSINESS E IL LEGAME OPERATIVO (blocco 7C.0)
//
// Tre domande, tre risposte separate. CHI scrive e' la controparte (controparte.go), un fatto. CHE
// COSA sta facendo il mittente in questo messaggio e' l'ATTO: una richiesta d'offerta, un'offerta,
// una revisione di documenti, una domanda, un sollecito. A CHE COSA appartiene la mail rispetto a
// cio' che il Cockpit conosce gia' e' il LEGAME: nuova, risposta, aggiornamento, inoltro, niente.
// L'esito del triage (nuova_rfq, aggancia, ignora) resta la proposta di AZIONE per la schermata e
// non e' una seconda rappresentazione del significato.
//
// L'atto e' neutro rispetto alla direzione: e' la terna (controparte, direzione, atto) a dare il
// senso. «Cliente in entrata + richiesta_offerta» e' una RFQ; «fornitore in uscita +
// richiesta_offerta» e' la nostra richiesta a lui. Fino alla 0015 c'erano due vocabolari (uno
// dell'enum intento_messaggio, uno del pacchetto agente) che portavano dentro la direzione:
// questo li sostituisce tutti e due.
//
// Gli atti sono righe della tabella `atto_business` (0016), non un enum: il vocabolario e' in
// evoluzione, e un atto ritirato deve restare leggibile sulle proposte vecchie. Le costanti qui
// devono coincidere con quelle righe; un valore inventato qui non entra in database (chiave
// esterna). Il legame e' l'enum `legame_operativo`: sei valori strutturali.
const (
	AttoRichiestaOfferta      = "richiesta_offerta"
	AttoOfferta               = "offerta"
	AttoRevisioneDocumenti    = "revisione_documenti"
	AttoDocumentiAggiuntivi   = "documenti_aggiuntivi"
	AttoDomandaChiarimento    = "domanda_chiarimento"
	AttoRispostaChiarimento   = "risposta_chiarimento"
	AttoSollecito             = "sollecito"
	AttoOrdine                = "ordine"
	AttoAccettazione          = "accettazione"
	AttoRifiuto               = "rifiuto"
	AttoConfermaRicezione     = "conferma_ricezione"
	AttoInoltro               = "inoltro"
	AttoNotifica              = "notifica"
	AttoComunicazioneGenerica = "comunicazione_generica"
	AttoNonBusiness           = "non_business"
	AttoIncerto               = "incerto"
)

// Atti e' il vocabolario nell'ordine della schermata: e' cio' che uno schema o un menu elencano.
var Atti = []string{AttoRichiestaOfferta, AttoOfferta, AttoRevisioneDocumenti, AttoDocumentiAggiuntivi,
	AttoDomandaChiarimento, AttoRispostaChiarimento, AttoSollecito, AttoOrdine, AttoAccettazione, AttoRifiuto,
	AttoConfermaRicezione, AttoInoltro, AttoNotifica, AttoComunicazioneGenerica, AttoNonBusiness, AttoIncerto}

// AttoValido dice se una stringa e' uno degli atti: un classificatore che ne inventa uno viene rifiutato.
func AttoValido(a string) bool {
	for _, x := range Atti {
		if x == a {
			return true
		}
	}
	return false
}

// Il legame operativo: «questa mail e' nuova, oppure continua qualcosa che il Cockpit conosce
// gia'?». E' una proposta con un bersaglio (una RFQ, una richiesta a un fornitore); il fatto
// resta thread_id / richiesta_fornitore_id dopo la decisione.
const (
	LegameNuovo         = "nuovo"         // apre qualcosa che non c'era: una RFQ, una richiesta a un fornitore
	LegameRisposta      = "risposta"      // risponde a un oggetto esistente (evidenza: R0, R1, candidato richiesta)
	LegameAggiornamento = "aggiornamento" // porta qualcosa in piu' a un oggetto esistente (una revisione, altri documenti)
	LegameInoltro       = "inoltro"       // un collega gira una mail: il legame e' quello della mail girata
	LegameNessuno       = "nessuno"       // non riguarda niente che il Cockpit conosca
	LegameIncerto       = "incerto"       // c'e' un indizio, ma non basta
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

// reNonBusiness: i segni di posta che non e' di lavoro. Si cerca nel mittente e nell'oggetto, e nel
// corpo SCRITTO ADESSO (non nella storia citata): una newsletter cita «RFQ» quanto vuole, resta
// una newsletter. E' una lista corta di proposito: meglio un «incerto» che una richiesta vera
// chiamata newsletter. Lo spam di Outlook non arriva fin qui: resta fuori dall'ingest.
var reNonBusiness = regexp.MustCompile(`(?i)(\bunsubscribe\b|\bdisiscri|\bnewsletter\b|\bno-?reply\b|\bnoreply\b|\bdo not reply\b|\bnon rispondere a questa\b|\bmailer-daemon\b|\bundeliverable\b|\bmancata consegna\b|\bout of office\b|\bfuori ufficio\b|\brisposta automatica\b|\bauto-?reply\b|\bautomatic reply\b|\bwebinar\b|\biscriviti\b|\bpromozione\b|\bofferta speciale\b)`)

// NonBusiness dice se il messaggio ha i segni di una newsletter, una notifica automatica, un fuori
// ufficio. Il motivo e' la frase che l'operatore legge.
func NonBusiness(mittente, oggetto, corpo string) (bool, string) {
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
	if m := reNonBusiness.FindString(testo); m != "" {
		return true, "il testo contiene «" + strings.ToLower(strings.TrimSpace(m)) + "»: newsletter, notifica o risposta automatica"
	}
	return false, ""
}

var reOffertaFornitore = regexp.MustCompile(`(?i)\b(offerta|quotazione|preventivo|quotation|quote|offer|prezzo|prezzi|price|pricing|listino|ns\.? offerta|vs\.? richiesta)\b`)
var reDomandaFornitore = regexp.MustCompile(`(?i)(\?|\b(chiediamo|vi chiediamo|potete|potreste|ci serve|serve sapere|servirebbe|could you|can you|please confirm|please advise|confermate|da chiarire|chiarimento|domanda|dubbio|specificare|quale materiale|quanti pezzi)\b)`)
var reConfermaRicezione = regexp.MustCompile(`(?i)\b(ricevuto|ricevuta|ben ricevuto|abbiamo ricevuto|received|well received|vi rispondiamo|vi rispondiamo entro|vi faremo sapere|we will revert|we will get back)\b`)
var estensioniOfferta = map[string]bool{"pdf": true, "xls": true, "xlsx": true, "doc": true, "docx": true}

// AttoFornitore legge una mail IN ENTRATA di un fornitore censito e dice che atto e': un'offerta
// (parole d'offerta e, di solito, un PDF o un foglio), una domanda (un punto interrogativo o le
// parole di chi chiede), una conferma di ricezione («ricevuto, vi rispondiamo»), posta che non e'
// di lavoro, oppure una comunicazione generica. Non decide a quale richiesta appartiene: quello lo
// fanno i candidati. E non decide lo stato della richiesta: quello lo fa una persona confermando
// l'atto (7C.0: una risposta non e' un'offerta). Il motivo e' per l'operatore.
func AttoFornitore(mittente, oggetto, corpo string, nomiAllegati []string) (atto, motivo string) {
	if si, m := NonBusiness(mittente, oggetto, corpo); si {
		return AttoNonBusiness, m
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
			return AttoOfferta, "parla di «" + strings.ToLower(m) + "» e allega un documento: l'offerta del fornitore"
		}
		return AttoOfferta, "parla di «" + strings.ToLower(m) + "»: l'offerta del fornitore, senza documento allegato"
	}
	if m := reDomandaFornitore.FindString(testo); m != "" {
		return AttoDomandaChiarimento, "il fornitore chiede qualcosa (" + strings.TrimSpace(strings.ToLower(m)) + ")"
	}
	if documenti > 0 {
		return AttoOfferta, "allega un documento senza parole d'offerta: probabilmente l'offerta"
	}
	if m := reConfermaRicezione.FindString(CorpoUtilePerInterpretazione(corpo)); m != "" {
		return AttoConfermaRicezione, "il fornitore conferma di aver ricevuto («" + strings.ToLower(m) + "»): la richiesta resta aperta"
	}
	return AttoComunicazioneGenerica, "risposta del fornitore senza offerta né domanda"
}
