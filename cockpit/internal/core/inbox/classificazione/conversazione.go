package classificazione

import (
	"regexp"
	"strings"
)

// LA CONVERSAZIONE E LE CHIAVI CITATE (Smistamento M1, A5.16.3, P36)
//
// Il ConversationID di Outlook non dice «questa mail risponde a quella». Per la posta che non nasce in
// Outlook (portali, ERP, Gmail) Exchange lo calcola dall'OGGETTO: tutte le «Richiesta di offerta» di
// clienti diversi finiscono nella stessa conversazione, e R1 a 95 le proponeva per la RFQ del primo
// cliente (revisione del 25/09). Quello che lo dice davvero è il ConversationIndex: 22 byte di
// intestazione (44 caratteri esadecimali) e 5 byte (10 caratteri) in più a ogni risposta, e la risposta
// COMINCIA con l'indice della mail a cui risponde. Una mail messa nella conversazione per l'oggetto ha
// un indice suo, di sola intestazione, che non discende da niente.
//
// Lo stesso vale per In-Reply-To e References: sono la prova più forte che esista, ma solo se chi scrive
// adesso era fra chi si scriveva allora. Una catena girata a un terzo, o un inoltro, si porta dietro le
// References di un'altra conversazione.

// Le misure del ConversationIndex (MailItem.ConversationIndex, in esadecimale). Da verificare sui dati
// veri (M0, V1): se il formato fosse un altro, DiscendeDa non riconosce niente e R1 resta debole, mai
// più forte.
const (
	LunghezzaIntestazioneIndice = 44
	LunghezzaBloccoIndice       = 10
)

// normalizzaIndice toglie gli spazi e porta a maiuscole; false se non è esadecimale o se non ha la forma
// di un indice (intestazione più blocchi interi).
func normalizzaIndice(s string) (string, bool) {
	s = strings.ToUpper(strings.Join(strings.Fields(s), ""))
	if len(s) < LunghezzaIntestazioneIndice || (len(s)-LunghezzaIntestazioneIndice)%LunghezzaBloccoIndice != 0 {
		return "", false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'A' && r <= 'F') {
			return "", false
		}
	}
	return s, true
}

// EUnaRisposta: l'indice ha almeno un blocco di risposta dopo l'intestazione. Un valore che non ha la
// forma di un indice non è una risposta.
func EUnaRisposta(indice string) bool {
	s, ok := normalizzaIndice(indice)
	return ok && len(s) > LunghezzaIntestazioneIndice
}

// DiscendeDa: il figlio comincia con il padre ed è più lungo di un numero intero di blocchi. Maiuscole
// indifferenti, spazi tolti; un valore che non è un indice non discende da niente e non è il padre di
// niente.
func DiscendeDa(figlio, padre string) bool {
	f, ok1 := normalizzaIndice(figlio)
	p, ok2 := normalizzaIndice(padre)
	return ok1 && ok2 && len(f) > len(p) && strings.HasPrefix(f, p)
}

// TipoR1 sceglie la variante di R1 per una RFQ della stessa conversazione: forte solo se l'indice del
// messaggio discende da quello di un messaggio della RFQ E il cliente è lo stesso; debole con l'indice e
// un altro cliente (o un cliente non noto); debole con il solo ConversationID, anche con l'indice vuoto
// (posta sincronizzata prima, posta non Outlook).
func TipoR1(indice string, indiciRFQ []string, stessoCliente bool) string {
	for _, p := range indiciRFQ {
		if DiscendeDa(indice, p) {
			if stessoCliente {
				return TipoR1Forte
			}
			return TipoR1Indice
		}
	}
	return TipoR1Solo
}

// reInoltroOggetto: l'oggetto COMINCIA con un prefisso d'inoltro. «R: I: …» è una risposta a un inoltro,
// non un inoltro: conta solo il primo prefisso.
var reInoltroOggetto = regexp.MustCompile(`(?i)^\s*(fw|fwd|i|tr|wg)\s*:`)

// frasiInoltro sono i separatori espliciti che aprono il contenuto girato (catena.go, frasiSeparatore).
var frasiInoltro = map[string]bool{
	"messaggio inoltrato":        true,
	"forwarded message":          true,
	"inizio messaggio inoltrato": true,
	"begin forwarded message":    true,
	"weitergeleitete nachricht":  true,
}

// EInoltro dice se il messaggio è un inoltro: il prefisso nell'oggetto, oppure la storia citata che
// comincia con un separatore d'inoltro. Chi inoltra si porta dietro le References della mail girata:
// per R0 non sono una risposta alla nostra RFQ.
func EInoltro(oggetto, corpo string) bool {
	if reInoltroOggetto.MatchString(oggetto) {
		return true
	}
	righe := strings.Split(normalizza(corpo), "\n")
	i := primaRigaDellaStoria(righe)
	if i < 0 {
		return false
	}
	t := strings.ToLower(pulisciCitazione(righe[i]))
	return frasiInoltro[strings.TrimSpace(strings.Trim(t, "-_=* \t"))]
}

// R0Verificato dice se una chiave citata vale come prova (A5.16.3, P36). Servono due cose: il mittente
// del messaggio nuovo era fra mittente e destinatari del messaggio citato (o c'era il suo dominio, se non
// è un dominio pubblico: un collega del buyer risponde per lui), e il messaggio nuovo non è un inoltro.
// Se manca qualcosa, il secondo valore dice che cosa, con parole da leggere.
func R0Verificato(mittente string, partecipanti []string, inoltro bool) (bool, string) {
	if inoltro {
		return false, "è un inoltro: la catena citata è quella della mail girata"
	}
	m := strings.ToLower(strings.TrimSpace(mittente))
	if m == "" {
		return false, "il mittente di questa mail non è noto"
	}
	dominio := ""
	if i := strings.LastIndex(m, "@"); i > 0 {
		dominio = m[i+1:]
	}
	for _, p := range partecipanti {
		if strings.ToLower(strings.TrimSpace(p)) == m {
			return true, ""
		}
	}
	if dominio != "" && !DominioPubblico(dominio) {
		for _, p := range partecipanti {
			p = strings.ToLower(strings.TrimSpace(p))
			if i := strings.LastIndex(p, "@"); i > 0 && p[i+1:] == dominio {
				return true, ""
			}
		}
	}
	return false, "chi scrive non era fra i destinatari della mail citata"
}
