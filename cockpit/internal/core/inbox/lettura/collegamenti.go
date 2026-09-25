package lettura

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// LE RISCRITTURE IN LINEA (I1–I6)
//
// Dentro un blocco di testo non si nasconde niente: si riscrive solo quello che Outlook ha aggiunto
// da sé e che rende il testo illeggibile. L'involucro Safe Links (duecento caratteri di
// «eur01.safelinks.protection.outlook.com/?url=…&data=…» al posto di un indirizzo di dieci), il
// doppione «m.rossi@acme.example<mailto:m.rossi@acme.example>», il segnaposto «[cid:image001.png@…]»
// delle immagini. Chi vuole il testo com'era lo ha nel «testo originale».
//
// Gli indirizzi NON diventano cliccabili: l'URL srotolato ha perso la protezione al clic di Safe
// Links, e la pagina non deve offrire un modo più comodo di Outlook per aprire un link di posta.

const notaSafeLinks = "indirizzo originale (tolto l'involucro Safe Links; la protezione al clic non vale qui)"

// maxIndirizzo: un'annotazione più lunga di così non è un indirizzo da mostrare (RE2 non accetta
// ripetizioni oltre 1000, quindi il limite di 2048 del design si controlla dopo la corrispondenza).
const maxIndirizzo = 2048

const espSafeLinks = `https?://[a-z0-9-]+\.safelinks\.protection\.(?:outlook\.com|office365\.us|outlook\.de|partner\.outlook\.cn)(?:/[^\s?<>"]*)?\?[^\s<>"]+`

var (
	reSafeLinks     = regexp.MustCompile(`(?i)` + espSafeLinks)
	reHostSafeLinks = regexp.MustCompile(`(?i)^[a-z0-9-]+\.safelinks\.protection\.(?:outlook\.com|office365\.us|outlook\.de|partner\.outlook\.cn)$`)

	// I2 — l'annotazione che Outlook mette dopo il testo di un'ancora.
	reAnnotazione = regexp.MustCompile(`(?i)<((?:https?|ftp|mailto|tel):[^<>\s]+)>`)

	// I3, I4, I5 e l'I1 «nudo» (fuori da un'annotazione), in un'espressione sola: si cerca la
	// corrispondenza più a sinistra, e a parità di posizione vale l'ordine qui sotto.
	reInLinea = regexp.MustCompile(`(?i)` +
		`\[cid:([^\]\s]{1,256})\]` + // 1: I3 [cid:image001.png@01DA…]
		`|\[([^\]\n]{0,300}?(?:descrizione\s+generata\s+automaticamente|description\s+automatically\s+generated))\]` + // 2: I4
		`|\[(https?://[^\]\s]+\.(?:png|jpe?g|gif|bmp)(?:\?[^\]\s]*)?)\]` + // 3: I5
		`|(` + espSafeLinks + `)`) // 4: I1
)

// SrotolaSafeLinks toglie l'involucro Safe Links da un indirizzo: restituisce l'indirizzo originale
// (il parametro «url»), fino a tre involucri uno dentro l'altro. L'originale deve essere http,
// https, mailto o ftp: se non lo è (javascript:, niente, spazzatura) restituisce u com'era e false,
// e il testo resta quello del messaggio.
func SrotolaSafeLinks(u string) (originale string, ok bool) {
	cur := u
	srotolato := false
	for range 3 {
		p, err := url.Parse(cur)
		if err != nil || !reHostSafeLinks.MatchString(p.Hostname()) {
			break
		}
		if sc := strings.ToLower(p.Scheme); sc != "http" && sc != "https" {
			break
		}
		dentro := p.Query().Get("url")
		if !indirizzoAmmesso(dentro) {
			return u, false
		}
		cur = dentro
		srotolato = true
	}
	if !srotolato {
		return u, false
	}
	return cur, true
}

// indirizzoAmmesso: un indirizzo che si può scrivere nella pagina al posto dell'involucro.
func indirizzoAmmesso(s string) bool {
	if s == "" || len(s) > 4096 {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == '<' || r == '>' || r == '"' || unicode.IsSpace(r) || r == unicode.ReplacementChar {
			return false
		}
	}
	p, err := url.Parse(s)
	if err != nil {
		return false
	}
	switch strings.ToLower(p.Scheme) {
	case "http", "https", "ftp":
		return p.Host != ""
	case "mailto":
		return p.Opaque != ""
	}
	return false
}

// srotolaTutto srotola ogni indirizzo Safe Links di s: serve alle regole (un invito Teams ha il
// link per entrare dentro l'involucro), non a quello che si mostra.
func srotolaTutto(s string) string {
	if !contieneFold(s, "safelinks") {
		return s
	}
	return reSafeLinks.ReplaceAllStringFunc(s, func(m string) string {
		if o, ok := SrotolaSafeLinks(m); ok {
			return o
		}
		return m
	})
}

// ---------------------------------------------------------------- i pezzi

// raccolta mette insieme i pezzi di un blocco: i tratti di testo consecutivi diventano uno.
type raccolta struct {
	pezzi []Pezzo
	testo strings.Builder
}

func (r *raccolta) scrivi(s string) { r.testo.WriteString(s) }

func (r *raccolta) aggiungi(p Pezzo) {
	r.chiudi()
	r.pezzi = append(r.pezzi, p)
}

func (r *raccolta) chiudi() {
	if r.testo.Len() > 0 {
		r.pezzi = append(r.pezzi, Pezzo{Tipo: PezzoTesto, Testo: r.testo.String()})
		r.testo.Reset()
	}
}

// pezziDaRighe riscrive le righe (già passate da righeVista) in pezzi; gli a capo restano «\n»
// dentro i pezzi di testo.
func pezziDaRighe(righe []string) []Pezzo {
	var r raccolta
	for i, riga := range righe {
		if i > 0 {
			r.scrivi("\n")
		}
		pezziRiga(riga, &r)
	}
	r.chiudi()
	return r.pezzi
}

// pezziRiga: prima le annotazioni (I2, con dentro I1), poi nel testo fra l'una e l'altra gli
// indirizzi Safe Links nudi e i segnaposto delle immagini.
func pezziRiga(riga string, r *raccolta) {
	if strings.IndexByte(riga, '<') < 0 {
		pezziSegmento(riga, r)
		return
	}
	pos := 0
	for _, m := range reAnnotazione.FindAllStringSubmatchIndex(riga, -1) {
		x := riga[m[2]:m[3]]
		if len(x) > maxIndirizzo {
			continue // resta testo: verrà scritto con il segmento che lo contiene
		}
		pezziSegmento(riga[pos:m[0]], r)
		nota := ""
		if o, ok := SrotolaSafeLinks(x); ok {
			x, nota = o, notaSafeLinks
		}
		if !doppione(riga[:m[0]], x) {
			if m[0] > 0 && riga[m[0]-1] != ' ' && riga[m[0]-1] != '\t' {
				r.scrivi(" ")
			}
			r.aggiungi(Pezzo{Tipo: PezzoLink, Testo: "<" + mostraIndirizzo(x) + ">", Nota: nota})
		}
		pos = m[1]
	}
	pezziSegmento(riga[pos:], r)
}

func pezziSegmento(seg string, r *raccolta) {
	if strings.IndexByte(seg, '[') < 0 && !contieneFold(seg, "safelinks") {
		r.scrivi(seg)
		return
	}
	pos := 0
	for _, m := range reInLinea.FindAllStringSubmatchIndex(seg, -1) {
		switch {
		case m[2] >= 0: // I3: il nome prima della «@»
			r.scrivi(seg[pos:m[0]])
			nome := seg[m[2]:m[3]]
			if i := strings.IndexByte(nome, '@'); i >= 0 {
				nome = nome[:i]
			}
			r.aggiungi(immagine(nome))
		case m[4] >= 0: // I4: il testo alternativo automatico di Office
			r.scrivi(seg[pos:m[0]])
			r.aggiungi(immagine(seg[m[4]:m[5]]))
		case m[6] >= 0: // I5: un'immagine collegata
			if m[7]-m[6] > maxIndirizzo {
				continue
			}
			r.scrivi(seg[pos:m[0]])
			r.aggiungi(immagine(seg[m[6]:m[7]]))
		case m[8] >= 0: // I1: un indirizzo Safe Links nel testo
			r.scrivi(seg[pos:m[0]])
			u := seg[m[8]:m[9]]
			// La punteggiatura in coda è della frase, non dell'indirizzo.
			base := strings.TrimRight(u, ".,;:!?)]}'")
			if o, ok := SrotolaSafeLinks(base); ok {
				r.aggiungi(Pezzo{Tipo: PezzoLink, Testo: o, Nota: notaSafeLinks})
				r.scrivi(u[len(base):])
			} else {
				r.scrivi(u)
			}
		default:
			continue
		}
		pos = m[1]
	}
	r.scrivi(seg[pos:])
}

func immagine(nota string) Pezzo {
	return Pezzo{Tipo: PezzoImmagine, Testo: "[immagine]", Nota: nota}
}

// doppione dice se l'annotazione x ripete il testo dell'ancora che la precede: «acme.example
// <http://acme.example/>», «m.rossi@acme.example<mailto:M.Rossi@acme.example?subject=RFQ>». Per i
// numeri di telefono contano solo le cifre, e l'ancora può contenere spazi e parentesi.
func doppione(prima, x string) bool {
	p := strings.TrimRight(prima, " \t")
	if len(x) >= 4 && strings.EqualFold(x[:4], "tel:") {
		k := len(p)
		for k > 0 && len(p)-k < 25 && strings.IndexByte("0123456789+ ()./-", p[k-1]) >= 0 {
			k--
		}
		a, b := soloCifre(p[k:]), soloCifre(x)
		return a != "" && a == b
	}
	k := len(p)
	for k > 0 && p[k-1] != ' ' && p[k-1] != '\t' && p[k-1] != '>' {
		k--
	}
	ancora := strings.TrimLeft(p[k:], "([{\"'")
	if o, ok := SrotolaSafeLinks(ancora); ok {
		ancora = o
	}
	a, b := confrontoIndirizzo(ancora), confrontoIndirizzo(x)
	return a != "" && a == b
}

func confrontoIndirizzo(s string) string {
	s = strings.ToLower(s)
	for _, pre := range []string{"mailto:", "tel:", "https://", "http://"} {
		s = strings.TrimPrefix(s, pre)
	}
	if i := strings.Index(s, "?subject="); i >= 0 {
		s = s[:i]
	}
	return strings.TrimRight(s, "/")
}

func soloCifre(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// mostraIndirizzo: «mailto:» non si mostra mai, l'indirizzo sì.
func mostraIndirizzo(x string) string {
	if len(x) >= 7 && strings.EqualFold(x[:7], "mailto:") {
		return x[7:]
	}
	return x
}
