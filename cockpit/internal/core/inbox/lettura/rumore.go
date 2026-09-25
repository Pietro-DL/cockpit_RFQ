package lettura

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// IL RUMORE (N1–N7)
//
// Il rumore è il testo che nessuno ha scritto: lo mettono il server di posta, il client, il modello
// dell'invito Teams, l'ufficio legale. Si CHIUDE, non si toglie: il blocco resta nella pagina con
// un'etichetta di una riga e il testo grezzo dentro. Ogni regola è stretta di proposito — posizione,
// lunghezza, e un'espressione che una persona non scrive per caso — perché l'errore che costa è
// chiudere una frase vera, non lasciare aperto un avviso.
//
// L'ordine conta: Teams prima (un invito contiene righe di underscore e link che altrimenti
// prenderebbero altre regole), poi primo contatto, posta esterna, Outlook mobile, ambiente, e per
// ultima la clausola di riservatezza, che guarda la coda della sezione quando il resto è già deciso.

var (
	// N1 — l'avviso «questa mail viene dall'esterno».
	reOrigineEsterna = filtra([]string{"outside", "external", "estern"}, `(?i)`+
		`(?:originated|comes?|came|was\s+sent|arrived)\s+(?:from\s+)?outside\s+(?:of\s+)?(?:the\s+|your\s+|our\s+)?(?:organi[sz]ation|company|network|domain)`+
		`|from\s+an?\s+external\s+(?:sender|source|email\s+address|domain)`+
		`|(?:proviene|arriva|provenient[ei]|(?:è|e')\s+stat[ao]\s+inviat[ao]|inviat[ao])\s+da\s+(?:un[ao]?\s+)?(?:mittente|indirizzo|dominio|fonte|utente)\s+estern[oa]`+
		`|(?:all'|dall')esterno\s+dell[a']\s*(?:organizzazione|azienda|societ[àa])`+
		`|estern[oa]\s+all'(?:organizzazione|azienda)`)
	// N1 — la riga che è solo l'etichetta, in cima alla sezione.
	reSoloEtichetta = regexp.MustCompile(`(?i)^\[?\s*(?:external|ext|extern|esterno|esterna|external\s+(?:email|sender)|e-?mail\s+esterna|mail\s+esterna)\s*\]?\s*[:!]?\s*$`)

	// N2 — il suggerimento di Microsoft 365 sul primo contatto.
	reImparaMittente = filtra([]string{"learnaboutsenderidentification"}, `(?i)aka\.ms/LearnAboutSenderIdentification`)
	rePrimoContatto  = filtra([]string{"often", "spesso"}, `(?i)`+
		`(?:don't|do\s+not)\s+often\s+(?:get|receive)\s+e-?mails?\s+from`+
		`|non\s+(?:ricevi|ricevono|si\s+ricevono)\s+spesso\s+(?:messaggi\s+di\s+posta\s+elettronica|messaggi|e-?mail|posta)\s+da`)
	reIndirizzo = regexp.MustCompile(`[^\s<>@]+@[^\s<>@]+\.[A-Za-z]{2,}`)

	// N3 — il piè di pagina di Outlook per telefono.
	reOutlookMobile = filtra([]string{"outlook"}, `(?i)^(?:get|download|scarica|ottieni|inviato\s+da|sent\s+from|envoyé\s+de|gesendet\s+von)\s+outlook\s+(?:for|per|pour|für)\s+(?:ios|android)\s*(?:<https?://aka\.ms/[^>\s]+>)?$`)
	reIPhone        = filtra([]string{"iphone", "ipad"}, `(?i)^(?:sent\s+from\s+my|inviato\s+dal\s+mio|inviato\s+da)\s+(?:iphone|ipad)$`)

	// N4 — la riga di separazione.
	reSeparatore = regexp.MustCompile(`^(?:_{10,}|-{10,}|={10,})$`)

	// N5 — l'invito a una riunione Teams.
	reInizioTeams       = filtra([]string{"teams"}, `(?i)^(?:microsoft\s+teams(?:\s+meeting)?(?:\s+(?:need\s+help\?|serve\s+aiuto\?))?|riunione\s+(?:di\s+)?microsoft\s+teams)$`)
	reUnderscoreTeams   = regexp.MustCompile(`^_{30,}$`)
	reFineTeams         = filtra([]string{"options", "opzioni", "pin"}, `(?i)meeting\s+options|opzioni\s+(?:della\s+)?riunione|reset\s+(?:dial-in\s+)?pin|reimposta\s+(?:il\s+)?pin`)
	reContenutoTeams    = filtra([]string{"meetup-join", "jointeamsmeeting", "meeting", "riunione", "passcode"}, `(?i)meetup-join|JoinTeamsMeeting|^meeting\s+id\s*:|^id\s+riunione\s*:|^passcode\s*:`)
	reTeamsValido       = filtra([]string{"meetup-join", "jointeamsmeeting"}, `(?i)teams\.microsoft\.com/l/meetup-join|aka\.ms/JoinTeamsMeeting`)
	reIDTeams           = regexp.MustCompile(`(?i)^(?:meeting\s+id|id\s+riunione)\s*:\s*([0-9][0-9 ]{7,22}[0-9])`)
	reAnnotazioneAngoli = regexp.MustCompile(`<[^<>]*>`)

	// N6 — la clausola di riservatezza.
	reTitoloClausola = regexp.MustCompile(`(?i)^(?:disclaimer|confidentiality\s+notice|legal\s+notice|privacy\s+notice|avviso\s+di\s+riservatezza|clausola\s+di\s+riservatezza|nota\s+di\s+riservatezza|informativa\s+(?:sulla\s+)?privacy|riservatezza)\s*:?$`)
	// L'insieme K: ogni voce conta una volta sola, per quante volte compaia. Una frase di lavoro può
	// dire «riservato» o «per errore»; due concetti diversi del gergo legale, in un paragrafo lungo in
	// coda alla mail, sono una clausola.
	reK = compilaTutte(
		// italiano
		`riservat[oaie]|confidenzial[ei]`,
		`destinatari[oa]`,
		`per\s+errore`,
		`(?:d\.?\s?lgs\.?|decreto\s+legislativo)\s*(?:n\.?\s*)?196|2016/679|\bgdpr\b|trattamento\s+dei\s+dati`,
		`vietat[ao]|proibit[ao]`,
		`(?:qualsiasi|ogni|qualunque)\s+(?:uso|utilizzo|diffusione|distribuzione|copia|riproduzione|divulgazione)`,
		`(?:cancellar|distrugger|eliminar)(?:lo|la|e)\b`,
		// inglese
		`confidential|privileged`,
		`intended\s+(?:solely\s+|only\s+)?for\s+the\s+(?:use\s+of\s+the\s+)?(?:named\s+)?(?:addressee|recipient|individual|person|entity)`,
		`if\s+you\s+(?:are\s+not\s+the\s+intended\s+recipient|have\s+received\s+this\s+(?:e-?mail|message|communication)\s+in\s+error)`,
		`prohibited`,
		`dissemination|distribution|copying|disclosure`,
		`notify\s+(?:the\s+sender|us)`,
		`delete\s+(?:it|this\s+(?:e-?mail|message)|all\s+copies)`,
	)

	// N7 — «pensa all'ambiente prima di stampare», col suo glifo Webdings.
	reAmbiente = filtra([]string{"environment", "ambiente", "stampa"}, `(?i)`+
		`(?:consider|think\s+(?:of|about))\s+the\s+environment\s+before\s+printing`+
		`|pensa(?:te)?\s+all'ambiente\s+prima\s+di\s+stampare`+
		`|prima\s+di\s+stampare[^\n]{0,60}(?:pensa|rifletti|considera)[^\n]{0,40}ambiente`+
		`|rispett(?:a|ate|iamo)\s+l'ambiente[^\n]{0,80}stamp`+
		`|non\s+stampare\s+(?:questa|questo)\s+(?:e-?mail|messaggio)\s+se\s+non\s+(?:è\s+|e'\s+)?(?:necessari[ao]|indispensabile)`)
	reGlifo = regexp.MustCompile(`^[PpLN]$`)
)

// filtrata è un'espressione con davanti le parole fisse che ogni sua corrispondenza contiene. Le
// espressioni (?i) di Go non hanno un prefisso letterale da cercare e girano su ogni riga di ogni
// messaggio: cercare prima una parola fissa le salta quasi sempre, e costa molto meno.
type filtrata struct {
	parole []string // in minuscolo, ASCII
	re     *regexp.Regexp
}

func filtra(parole []string, esp string) filtrata {
	return filtrata{parole: parole, re: regexp.MustCompile(esp)}
}

func (f filtrata) MatchString(s string) bool {
	return contieneUna(s, f.parole...) && f.re.MatchString(s)
}

// contieneUna: s contiene almeno una delle parole (in minuscolo ASCII), senza badare alle maiuscole.
func contieneUna(s string, parole ...string) bool {
	for _, p := range parole {
		if contieneFold(s, p) {
			return true
		}
	}
	return false
}

func contieneFold(s, sub string) bool {
	n := len(sub)
	if n == 0 {
		return true
	}
	c := sub[0]
	for i := 0; i+n <= len(s); i++ {
		if s[i]|0x20 == c|0x20 && strings.EqualFold(s[i:i+n], sub) {
			return true
		}
	}
	return false
}

func compilaTutte(esp ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(esp))
	for i, e := range esp {
		out[i] = regexp.MustCompile(`(?i)` + e)
	}
	return out
}

// Le etichette. Quella lunga va sul blocco da solo; quella breve nell'elenco di un blocco unito
// («testo automatico: posta esterna, primo contatto»).
var (
	etichette = map[Motivo]string{
		MotivoBannerEsterno: "avviso: posta esterna",
		MotivoPrimoContatto: "avviso Microsoft 365: primo contatto",
		MotivoOutlookMobile: "firma automatica di Outlook",
		MotivoRiunioneTeams: "riunione Microsoft Teams",
		MotivoRiservatezza:  "clausola di riservatezza",
		MotivoAmbiente:      "invito a non stampare",
	}
	etichetteBrevi = map[Motivo]string{
		MotivoBannerEsterno: "posta esterna",
		MotivoPrimoContatto: "primo contatto",
		MotivoOutlookMobile: "firma di Outlook",
		MotivoRiunioneTeams: "riunione Teams",
		MotivoRiservatezza:  "riservatezza",
		MotivoAmbiente:      "invito a non stampare",
	}
)

func etichettaUnita(mm []Motivo) string {
	nomi := make([]string, len(mm))
	for i, m := range mm {
		nomi[i] = etichetteBrevi[m]
	}
	return "testo automatico: " + strings.Join(nomi, ", ")
}

func rumoreDi(da, a int, m Motivo) regione {
	return regione{tipo: BloccoRumore, da: da, a: a, motivo: m, etichetta: etichette[m]}
}

// rumore applica N5, N2, N1, N3, N7 e N6, in quest'ordine.
func (s *sezione) rumore() {
	if len(s.norm) == 0 {
		return
	}
	s.teams()
	s.primoContatto()
	s.bannerEsterno()
	s.outlookMobile()
	s.ambiente()
	s.riservatezza()
}

// ---------------------------------------------------------------- N5: la riunione Teams

// teams chiude l'invito Teams incollato in un messaggio. L'inizio è una riga che dice solo
// «Microsoft Teams meeting» (o i suoi parenti); il blocco vale SOLO se contiene il link per entrare
// nella riunione: una frase che parla di Teams non è un invito.
func (s *sezione) teams() {
	var f *righeTeams
	for i := 0; i < len(s.norm); i++ {
		if !s.libera(i) || len(s.norm[i]) > 300 || !contieneFold(s.norm[i], "teams") {
			continue
		}
		inizio := strings.TrimSpace(reAnnotazioneAngoli.ReplaceAllString(s.norm[i], ""))
		if !reInizioTeams.MatchString(inizio) {
			continue
		}
		if f == nil {
			f = s.righeTeams()
		}
		// Indietro: la riga di underscore che apre l'invito, con le righe vuote in mezzo.
		da := i
		j := i - 1
		for j >= 0 && s.libera(j) && s.vuota(j) {
			j--
		}
		if j >= 0 && s.libera(j) && f.underscore[j] {
			da = j
		}
		fine := s.fineTeams(i, f)
		if fine < 0 {
			continue
		}
		valido := false
		for k := da; k <= fine && !valido; k++ {
			valido = f.valido[k]
		}
		if !valido {
			continue
		}
		r := rumoreDi(da, fine+1, MotivoRiunioneTeams)
		if id := idTeams(s.norm[da : fine+1]); id != "" {
			r.etichetta += " · ID " + id
		}
		if s.occupa(r) {
			i = fine
		}
	}
}

// righeTeams sono le domande di N5 già fatte a ogni riga, una volta sola: ogni riga d'inizio
// guarda le 45 righe dopo, e rifare le regex ogni volta costerebbe 45 volte il testo su una mail
// fatta di righe «Microsoft Teams».
type righeTeams struct {
	underscore, fine, contenuto, valido []bool
}

func (s *sezione) righeTeams() *righeTeams {
	n := len(s.norm)
	f := &righeTeams{make([]bool, n), make([]bool, n), make([]bool, n), make([]bool, n)}
	for k, r := range s.norm {
		f.underscore[k] = reUnderscoreTeams.MatchString(r)
		f.fine[k] = reFineTeams.MatchString(r)
		f.contenuto[k] = reContenutoTeams.MatchString(r)
		// Un indirizzo non va mai a capo: guardare riga per riga è guardare l'invito intero.
		f.valido[k] = reTeamsValido.MatchString(srotolaTutto(r))
	}
	return f
}

// fineTeams è l'ultima riga (compresa) dell'invito che comincia alla riga i, -1 se non si trova.
//
// Si cerca entro 45 righe, e la riga «Meeting options / Reset dial-in PIN» vince sulla prima riga
// di underscore: il formato nuovo dell'invito ha una riga di 32 underscore IN MEZZO (fra il link e i
// numeri per chiamare) e fermarsi lì lascerebbe mezzo invito fuori dal blocco. La riga di underscore
// che chiude l'invito, subito dopo le opzioni, entra nel blocco.
func (s *sezione) fineTeams(i int, f *righeTeams) int {
	limite := min(len(s.norm), i+46)
	for k := i + 1; k < limite; k++ {
		if !s.libera(k) {
			limite = k
			break
		}
	}
	for k := i + 1; k < limite; k++ {
		if f.fine[k] {
			j := k + 1
			for j < len(s.norm) && j <= k+2 && s.libera(j) && s.vuota(j) {
				j++
			}
			if j < len(s.norm) && s.libera(j) && f.underscore[j] {
				return j
			}
			return k
		}
	}
	for k := i + 1; k < limite; k++ {
		if f.underscore[k] {
			return k
		}
	}
	// Senza marcatore di fine: fino alla prima riga vuota dopo l'ultima riga dell'invito.
	ultima := -1
	for k := i; k < limite; k++ {
		if f.contenuto[k] {
			ultima = k
		}
	}
	if ultima < 0 {
		return -1
	}
	for ultima+1 < limite && !s.vuota(ultima+1) {
		ultima++
	}
	return ultima
}

func idTeams(righe []string) string {
	for _, r := range righe {
		if m := reIDTeams.FindStringSubmatch(r); m != nil {
			return strings.Join(strings.Fields(m[1]), " ")
		}
	}
	return ""
}

// ---------------------------------------------------------------- N2: primo contatto

// primoContatto chiude il suggerimento «Non ricevi spesso messaggi da …»: un paragrafo breve, in
// qualunque punto, con il link di Microsoft o con la frase E un indirizzo.
func (s *sezione) primoContatto() {
	for _, p := range s.paragrafi() {
		if p[1]-p[0] > 4 {
			continue
		}
		t := s.testoNorm(p[0], p[1])
		if utf8.RuneCountInString(t) > 500 {
			continue
		}
		if èPrimoContatto(srotolaTutto(t)) {
			s.occupa(rumoreDi(p[0], p[1], MotivoPrimoContatto))
		}
	}
}

func èPrimoContatto(t string) bool {
	return reImparaMittente.MatchString(t) || (rePrimoContatto.MatchString(t) && reIndirizzo.MatchString(t))
}

// ---------------------------------------------------------------- N1: posta esterna

// bannerEsterno chiude l'avviso di posta esterna. Solo fra i primi due o gli ultimi due paragrafi
// della sezione: è lì che lo mettono i server, e una frase simile a metà mail l'ha scritta qualcuno.
func (s *sezione) bannerEsterno() {
	pp := s.paragrafi()
	for k, p := range pp {
		if k >= 2 && k < len(pp)-2 {
			continue
		}
		if p[1]-p[0] > 6 {
			continue
		}
		t := s.testoNorm(p[0], p[1])
		if utf8.RuneCountInString(t) > 600 {
			continue
		}
		if reOrigineEsterna.MatchString(t) {
			s.occupa(rumoreDi(p[0], p[1], MotivoBannerEsterno))
		}
	}
	// La riga che è solo «[EXTERNAL]», in cima: la prima riga non vuota che non sia già rumore.
	for i := range s.norm {
		if s.vuota(i) {
			continue
		}
		if !s.libera(i) {
			if s.regioni[s.prop[i]].tipo == BloccoRumore {
				continue
			}
			return
		}
		if len(s.norm[i]) <= 80 && reSoloEtichetta.MatchString(s.norm[i]) {
			s.occupa(rumoreDi(i, i+1, MotivoBannerEsterno))
		}
		return
	}
}

// ---------------------------------------------------------------- N3: Outlook per telefono

func (s *sezione) outlookMobile() {
	for i := range s.norm {
		if !s.libera(i) || s.vuota(i) || len(s.norm[i]) > 300 {
			continue
		}
		if reOutlookMobile.MatchString(s.norm[i]) || reIPhone.MatchString(s.norm[i]) {
			s.occupa(rumoreDi(i, i+1, MotivoOutlookMobile))
		}
	}
}

// ---------------------------------------------------------------- N7: l'ambiente

// ambiente chiude l'invito a non stampare. Il glifo della fogliolina è un carattere Webdings (una
// «P»), che nel testo semplice diventa una riga con una lettera sola: se sta subito sopra (al più
// una riga vuota in mezzo) entra nel blocco — da sola non è niente, ma lasciarla lì è rumore.
func (s *sezione) ambiente() {
	for i := range s.norm {
		if !s.libera(i) || s.vuota(i) || utf8.RuneCountInString(s.norm[i]) > 250 || !reAmbiente.MatchString(s.norm[i]) {
			continue
		}
		da := i
		j := i - 1
		if j >= 0 && s.libera(j) && s.vuota(j) {
			j--
		}
		if j >= 0 && s.libera(j) && reGlifo.MatchString(s.norm[j]) {
			da = j
		}
		s.occupa(rumoreDi(da, i+1, MotivoAmbiente))
	}
}

// ---------------------------------------------------------------- N6: la clausola di riservatezza

// riservatezza chiude la clausola legale in coda alla sezione. Si risale dal fondo scavalcando le
// righe vuote, i separatori, il rumore di N3/N7 e i paragrafi di clausola; ci si ferma al primo
// paragrafo che non lo è. Per questo una clausola seguita da «Confermate la consegna?» non si
// chiude mai: sotto c'è testo di lavoro, e il fondo della mail non è dove si pensava.
func (s *sezione) riservatezza() {
	primo, ultimo := -1, -1
	for i := len(s.norm) - 1; i >= 0; {
		if s.libera(i) && s.vuota(i) {
			i--
			continue
		}
		if !s.libera(i) {
			r := s.regioni[s.prop[i]]
			if r.tipo == BloccoRumore && (r.motivo == MotivoOutlookMobile || r.motivo == MotivoAmbiente) {
				i = r.da - 1
				continue
			}
			break
		}
		if reSeparatore.MatchString(s.norm[i]) {
			i--
			continue
		}
		inizio := i
		for inizio > 0 && s.libera(inizio-1) && !s.vuota(inizio-1) && !reSeparatore.MatchString(s.norm[inizio-1]) {
			inizio--
		}
		da, ok := s.clausola(inizio, i+1)
		if !ok {
			break
		}
		if ultimo < 0 {
			ultimo = i + 1
		}
		primo = da
		i = da - 1
	}
	if primo < 0 {
		return
	}
	// Le righe libere fra la prima e l'ultima clausola sono clausola; il rumore N3/N7 in mezzo resta
	// suo (e si unisce dopo, perché si tocca). Un tratto di sole righe vuote non è un blocco.
	for k := primo; k < ultimo; {
		if !s.libera(k) {
			k++
			continue
		}
		j := k
		for j < ultimo && s.libera(j) {
			j++
		}
		if !s.tuttoVuoto(k, j) {
			s.occupa(rumoreDi(k, j, MotivoRiservatezza))
		}
		k = j
	}
}

// clausola dice se il paragrafo [inizio,fine) è una clausola, e da che riga comincia il blocco: la
// riga-titolo («Disclaimer:», «Avviso di riservatezza») entra, sia che apra il paragrafo sia che
// stia da sola subito sopra.
func (s *sezione) clausola(inizio, fine int) (int, bool) {
	corpo, da := inizio, inizio
	titolo := false
	if reTitoloClausola.MatchString(s.norm[inizio]) {
		titolo = true
		corpo = inizio + 1
	} else {
		j := inizio - 1
		for j >= 0 && s.libera(j) && s.vuota(j) {
			j--
		}
		if j >= 0 && s.libera(j) && reTitoloClausola.MatchString(s.norm[j]) && (j == 0 || !s.libera(j-1) || s.vuota(j-1)) {
			titolo = true
			da = j
		}
	}
	if corpo >= fine {
		return 0, false
	}
	t := s.testoNorm(corpo, fine)
	if n := utf8.RuneCountInString(t); n < 120 || n > 3000 {
		return 0, false
	}
	soglia := 2
	if titolo {
		soglia = 1
	}
	return da, contaK(t) >= soglia
}

func contaK(t string) int {
	n := 0
	for _, re := range reK {
		if re.MatchString(t) {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------- N4: i separatori

// separatori: le righe di underscore, trattini o uguali rimaste libere diventano un filetto. Quelle
// dentro un invito Teams o in cima alla storia citata sono già state prese.
func (s *sezione) separatori() {
	for i := range s.norm {
		if s.libera(i) && reSeparatore.MatchString(s.norm[i]) {
			s.occupa(regione{tipo: BloccoSeparatore, da: i, a: i + 1})
		}
	}
}

// èRumoreDiTabella: il criterio (6) delle tabelle di dati. Il banner esterno, il primo contatto e
// l'invito Teams arrivano spesso come tabelle HTML: non sono dati, e mostrarli come tabella li
// metterebbe in evidenza proprio mentre li si vuole chiudere.
func èRumoreDiTabella(t string) bool {
	t = srotolaTutto(perConfronto(t))
	if reOrigineEsterna.MatchString(t) || èPrimoContatto(t) || reTeamsValido.MatchString(t) {
		return true
	}
	if !contieneFold(t, "teams") {
		return false
	}
	for _, r := range strings.Split(t, "\n") {
		r = strings.TrimSpace(r)
		if len(r) <= 300 && reInizioTeams.MatchString(r) {
			return true
		}
	}
	return false
}
