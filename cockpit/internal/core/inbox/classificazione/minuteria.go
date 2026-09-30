package classificazione

// La minuteria dal NOME di un pezzo (giro 4, fase 4.4a.1a; domanda 30, seconda risposta; studio
// docs/specs/studio_normati_29-09.md § 6, con la verifica in coda): viti, dadi, rondelle, spine, copiglie, anelli,
// grani, rivetti, inserti e prigionieri si riconoscono dal nome, quando il pezzo ne ha uno oltre al codice. Due regole:
//   - A, «normato»: una norma di PEZZO di minuteria di una lista chiusa (UNI 5739, DIN 933, ISO 4762, EN 24017 = ISO
//     4017), e niente che dica «pezzo fatto da noi»;
//   - B, «probabile minuteria»: un nome di minuteria in testa (VITE, DADO, SCREW, NUT, SCHRAUBE, VIS…) con un segnale
//     da minuteria (M8, M8X20, 8.8, TCEI, SDPR, 1/4"-20…), senza un nome di pezzo custom in testa (SUPPORTO, PIASTRA,
//     BRACKET…) e senza segni di lavorazione (A DIS., SPECIALE, S235, SP.6, DA TAGLIARE…).
// Un indizio con un segno contrario, o senza misura, e' «da vedere»: nessuna proposta, solo la frase.
//
// Si applica SOLO al nome di un pezzo (il PRODUCT dello STEP, la descrizione di una riga dell'elenco particolari),
// MAI al testo intero del disegno: le norme di tolleranze, di rugosita', di saldatura e dei materiali (ISO 2768, EN
// 10025) stanno su ogni cartiglio, e una nota «dadi a saldare M8» farebbe nascere un dado che nella distinta non c'e'.
// E' una PROPOSTA per una persona: ogni proposta vuole il clic dell'ingegnere, ✓ o ✗ (domanda 30, seconda risposta).
// Chi la usa la mostra come una domanda aperta sul nodo; il riconoscitore non scrive mai un tipo.
//
// E' la sonda dello studio (sonda_normati.py, fuori da git) portata in Go, con le stesse liste e le stesse regole,
// tranne una correzione della sua verifica: «UNI n» vale solo con i numeri UNI della lista, non con quelli della
// lista ISO. Sono due numerazioni diverse e il rischio non era misurato; un numero che manca non fa danni (il pezzo
// passa per la regola B), uno sbagliato si'. Le espressioni della sonda con lookbehind e lookahead, che RE2 non ha,
// sono scritte senza e con un controllo dei caratteri accanto alla lettura (primaLettura).
// Le famiglie di codici del cliente con il ruolo commerciale (una versione del codice, un gruppo di codici) non sono qui: sono
// regole scritte da una persona (fase 4.9, domanda 14), e questo riconoscitore guarda soltanto il nome.

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Gli esiti del riconoscitore, dal piu' forte.
const (
	MinuteriaNormato   = "normato"
	MinuteriaProbabile = "probabile_minuteria"
	MinuteriaDaVedere  = "da_vedere"
	MinuteriaNo        = "no"
)

var rangoMinuteria = map[string]int{MinuteriaNormato: 3, MinuteriaProbabile: 2, MinuteriaDaVedere: 1, MinuteriaNo: 0}

// EsitoMinuteria e' quello che il nome di un pezzo dice della minuteria.
type EsitoMinuteria struct {
	Esito string `json:"esito"`
	// Norma: la norma di pezzo letta, per un «normato» («UNI 5739»).
	Norma string `json:"norma,omitempty"`
	// Nome: il nome di minuteria in testa, per un «probabile» («VITE», «WELD NUT»).
	Nome string `json:"nome,omitempty"`
	// Segnali: i segnali da minuteria letti nel nome (la filettatura, la classe, la sigla), al piu' due.
	Segnali []string `json:"segnali,omitempty"`
	// Motivi: perche' e' «da vedere» (l'indizio e il segno contrario), o la norma non di pezzo di un «no».
	Motivi []string `json:"motivi,omitempty"`
}

// Proposta dice se l'esito propone il tipo «particolare commerciale» (regola A o B): una domanda per una persona.
func (e EsitoMinuteria) Proposta() bool {
	return e.Esito == MinuteriaNormato || e.Esito == MinuteriaProbabile
}

// Frase e' il motivo in parole: «normato: UNI 5739 · M8X20 · 8.8», «probabile minuteria: VITE · M6X22 · SDPR», «da
// vedere: nome «RONDELLA»; segno di pezzo fatto: S235JR». Vuota per un «no».
func (e EsitoMinuteria) Frase() string {
	switch e.Esito {
	case MinuteriaNormato:
		return strings.Join(append([]string{"normato: " + e.Norma}, e.Segnali...), " · ")
	case MinuteriaProbabile:
		return strings.Join(append([]string{"probabile minuteria: " + e.Nome}, e.Segnali...), " · ")
	case MinuteriaDaVedere:
		return "da vedere: " + strings.Join(e.Motivi, "; ")
	}
	return ""
}

// Minuteria legge il nome di UN pezzo (non un testo intero) e dice se e' minuteria. Un nome bilingue («DADO SDPR M6 /
// NUT PROJ WELD M6») si legge meta' per meta' e vale la meta' piu' forte; ma un nome custom in testa a una meta' ferma
// anche l'altra («SUPPORTO PERNO / PIN SUPPORT» e' da vedere). Pura.
func Minuteria(nome string) EsitoMinuteria {
	s := normaNome(nome)
	meta := metaDelNome(s)
	esiti := make([]EsitoMinuteria, len(meta))
	custom := false
	for i, m := range meta {
		esiti[i] = valutaMeta(m)
		custom = custom || len(nomiCustom(paroleDi(togliCodiceInTesta(m)))) > 0
	}
	migliore := esiti[0]
	for _, e := range esiti[1:] {
		if rangoMinuteria[e.Esito] > rangoMinuteria[migliore.Esito] {
			migliore = e
		}
	}
	if custom && migliore.Proposta() {
		return EsitoMinuteria{Esito: MinuteriaDaVedere, Motivi: []string{"nome custom nell'altra lingua", migliore.Frase()}}
	}
	return migliore
}

// valutaMeta e' il riconoscitore su una lingua sola (una meta' di un nome bilingue).
func valutaMeta(s string) EsitoMinuteria {
	corpo := togliCodiceInTesta(s)
	w := paroleDi(corpo)
	pezzo, trappole := norme(corpo)
	neg := primoNegativo(corpo)
	custom := nomiCustom(w)
	composto := primoComposto(corpo)
	var testa []string
	for _, x := range w[:min(3, len(w))] {
		if !reNonTesta.MatchString(x) {
			testa = append(testa, x)
		}
	}
	univoco, ambiguo := "", ""
	for _, x := range testa {
		if univociMinuteria[x] {
			univoco = x
			break
		}
	}
	if univoco == "" {
		univoco = composto
	}
	if univoco == "" {
		for _, x := range testa {
			if ambiguiMinuteria[x] {
				ambiguo = x
				break
			}
		}
	}
	filetto := cercaFiletto(corpo)
	if filetto == "" {
		filetto = cercaPollici(corpo)
	}
	var segnali []string
	for _, x := range []string{filetto, cercaClasse(corpo), reSigleMinuteria.FindString(corpo)} {
		if x != "" {
			segnali = append(segnali, x)
		}
	}
	misura := reMisuraGenerica.FindString(corpo)
	pulito := neg == "" && len(custom) == 0
	if len(pezzo) > 0 && pulito {
		return EsitoMinuteria{Esito: MinuteriaNormato, Norma: pezzo[0], Segnali: primi(segnali, 2)}
	}
	// una misura generica (D4 L.20, 4,8X20) basta solo a un nome italiano in testa o a un composto: in inglese il
	// nome di minuteria puo' essere un modificatore («SCREW CONVEYOR D300»), e allora serve un segnale da minuteria
	genericaOk := misura != "" && (univociItaliani[univoco] || composto != "")
	if univoco != "" && pulito && (len(segnali) > 0 || genericaOk) {
		if len(segnali) == 0 {
			segnali = []string{misura}
		}
		return EsitoMinuteria{Esito: MinuteriaProbabile, Nome: univoco, Segnali: primi(segnali, 2)}
	}
	if ambiguo != "" && pulito && len(segnali) > 0 {
		return EsitoMinuteria{Esito: MinuteriaProbabile, Nome: ambiguo, Segnali: primi(segnali, 2)}
	}
	if len(pezzo) > 0 || univoco != "" || ambiguo != "" {
		var motivi []string
		if len(pezzo) > 0 {
			motivi = append(motivi, "norma "+pezzo[0])
		}
		if univoco != "" || ambiguo != "" {
			motivi = append(motivi, "nome «"+univoco+ambiguo+"»")
		}
		if neg != "" {
			motivi = append(motivi, "segno di pezzo fatto: "+neg)
		}
		if len(custom) > 0 {
			motivi = append(motivi, "nome custom: "+strings.Join(custom, ", "))
		}
		if pulito {
			motivi = append(motivi, "nessuna misura da minuteria")
		}
		return EsitoMinuteria{Esito: MinuteriaDaVedere, Motivi: motivi}
	}
	e := EsitoMinuteria{Esito: MinuteriaNo}
	if len(trappole) > 0 {
		e.Motivi = []string{"norma non di pezzo: " + trappole[0]}
	}
	return e
}

// ------------------------------------------------------------------ il nome, normalizzato

var (
	reDiametroNome   = regexp.MustCompile(`[ØΦ⌀∅]`)
	reUniEnAttaccato = regexp.MustCompile(`\bUNIEN\b`)
	reCorpoAttaccato = regexp.MustCompile(`\b(UNI|DIN|ISO|EN)(\d)`)
)

// normaNome mette il nome nella forma su cui lavorano le regole: maiuscolo, «Ø» → « D», «×» e «*» → «X», «_» →
// spazio, «UNI5739» → «UNI 5739», «UNIEN» → «UNI EN», gli spazi uno per volta. Della normalizzazione NFKC della sonda
// tiene cio' che serve ai nomi (gli spazi Unicode e le forme a larghezza piena): senza dipendenze nuove.
func normaNome(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '�':
			b.WriteString(" D") // un Ø rotto da una codifica sbagliata
		case unicode.IsSpace(r):
			b.WriteByte(' ')
		case r >= 0xFF01 && r <= 0xFF5E:
			b.WriteRune(r - 0xFEE0)
		default:
			b.WriteRune(r)
		}
	}
	s = strings.ToUpper(strings.ReplaceAll(b.String(), "ß", "ss"))
	s = reDiametroNome.ReplaceAllString(s, " D")
	s = strings.NewReplacer("×", "X", "*", "X", "_", " ", "É", "E", "È", "E").Replace(s)
	s = reUniEnAttaccato.ReplaceAllString(s, "UNI EN")
	s = reCorpoAttaccato.ReplaceAllString(s, "$1 $2")
	return strings.Join(strings.Fields(s), " ")
}

// metaDelNome divide un nome bilingue su « / » e su « - » seguito da una lettera: le meta' non vuote, o il nome
// intero.
func metaDelNome(s string) []string {
	var out []string
	inizio := 0
	for i := 0; i+3 <= len(s); {
		sep := s[i:i+3] == " / " || (s[i:i+3] == " - " && i+3 < len(s) && s[i+3] >= 'A' && s[i+3] <= 'Z')
		if !sep {
			i++
			continue
		}
		if m := strings.TrimSpace(s[inizio:i]); m != "" {
			out = append(out, m)
		}
		i += 3
		inizio = i
	}
	if m := strings.TrimSpace(s[inizio:]); m != "" {
		out = append(out, m)
	}
	if len(out) == 0 {
		return []string{s}
	}
	return out
}

var (
	reTreLettere    = regexp.MustCompile(`[A-Z]{3,}`)
	reFilettoIntero = regexp.MustCompile(`^M ?\d{1,2}(?:[.,]\d+)?(?: ?X ?\d+(?:[.,]\d+)?){0,2}$`)
	reNonTesta      = regexp.MustCompile(`^(?:[\d.,X]+|M\d+.*|D\d+.*)$`)
)

// togliCodiceInTesta toglie in testa i pezzi con cifre (il codice del pezzo, la posizione): «00715 DADO…» → «DADO…».
// Non una filettatura («M8 DADO»), non una parola («PART3»).
func togliCodiceInTesta(s string) string {
	t := strings.Split(s, " ")
	for len(t) > 1 && strings.ContainsAny(t[0], "0123456789") && !reTreLettere.MatchString(t[0]) && !reFilettoIntero.MatchString(t[0]) {
		t = t[1:]
	}
	return strings.Join(t, " ")
}

// paroleDi sono le parole del nome: le lettere A-Z e le cifre.
func paroleDi(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return (r < 'A' || r > 'Z') && (r < '0' || r > '9') })
}

// nomiCustom sono i nomi di pezzo custom fra le prime tre parole, in ordine («ASS» solo in testa: dentro e' altro).
func nomiCustom(w []string) []string {
	visti := map[string]bool{}
	var out []string
	for i, x := range w[:min(3, len(w))] {
		if nomiCustomMinuteria[x] && (x != "ASS" || i == 0) && !visti[x] {
			visti[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

// ------------------------------------------------------------------ le norme

var reNormaNome = regexp.MustCompile(`\b(UNI EN ISO|DIN EN ISO|UNI ISO|DIN ISO|EN ISO|UNI EN|DIN EN|ISO|DIN|UNI|EN) ?-? ?(\d+)`)

// norme sono le norme lette nel nome: quelle di pezzo di minuteria (la regola A) e quelle che stanno sui disegni ma
// non sono pezzi (tolleranze, materiali: solo per dire perche' un nome non e' minuteria). Un numero di norma ha da due
// a cinque cifre.
func norme(s string) (pezzo, nonPezzo []string) {
	for _, m := range reNormaNome.FindAllStringSubmatch(s, -1) {
		corpo, cifre := m[1], m[2]
		if len(cifre) < 2 || len(cifre) > 5 {
			continue
		}
		n, _ := strconv.Atoi(cifre)
		sigla := fmt.Sprintf("%s %d", corpo, n)
		di, non := false, false
		switch {
		case corpo == "ISO" || strings.HasSuffix(corpo, " ISO"):
			di, non = normeISO[n], nonPezzoISO[n]
		case corpo == "EN" || strings.HasSuffix(corpo, " EN"):
			// le vecchie EN 2xxxx sono le ISO con 20000 in piu' (EN 24017 = ISO 4017)
			di, non = normeEN[n] || (n > 20000 && n < 30000 && normeISO[n-20000]), nonPezzoEN[n]
		case corpo == "DIN":
			di, non = normeDIN[n], nonPezzoDIN[n]
		case corpo == "UNI":
			di, non = normeUNI[n], nonPezzoUNI[n]
		}
		switch {
		case di:
			pezzo = append(pezzo, sigla)
		case non:
			nonPezzo = append(nonPezzo, sigla)
		}
	}
	return pezzo, nonPezzo
}

// ------------------------------------------------------------------ i segnali

var (
	// la filettatura metrica: M8, M 8, M8X20, M10X1.25X30; non dentro una parola (AM8) e non davanti a una cifra
	reFilettoNome = regexp.MustCompile(`M ?\d{1,2}(?:[.,]\d+)?(?: ?X ?\d+(?:[.,]\d+)?){0,2}`)
	// la filettatura in pollici: 1/4"-20, 3/8 16 (e non davanti a una cifra); UNC, UNF
	rePolliciNome = regexp.MustCompile(`\b\d{1,2}/\d{1,2}"? ?-? ?\d{2}`)
	reUNCNome     = regexp.MustCompile(`\bUN[CF]\b`)
	// la classe di resistenza: 8.8 (non dopo una cifra o un punto, non davanti a una cifra), CL 10, A2-70, 8-8
	reClasseNumero = regexp.MustCompile(`4\.6|4\.8|5\.6|5\.8|6\.8|8\.8|10\.9|12\.9`)
	reClasseCL     = regexp.MustCompile(`\bCL\.? ?(?:8|10|12)\b`)
	reClasseInox   = regexp.MustCompile(`\bA[24] ?- ?(?:50|70|80)\b`)
	reClasseTratto = regexp.MustCompile(`8-8|10-9`)
	// le sigle da minuteria: la testa (TE, TCEI…), a saldare (SDPR, WELD), autobloccante, esagonale, la chiave (CH13)
	reSigleMinuteria = regexp.MustCompile(`\b(?:TE|T\.E\.|TCEI|TSPEI|TBEI|TPEI|TSEI|TSVEI|SDPR|SD\. ?PR|SALDOPROIE\w*|PROIEZ\w*|PROJ` +
		`|WELD|AUTOBLOC\w*|NYLOCK|CIECO|FLANGIAT\w*|ESAG\w*|EXAG\w*|HEX|SOCKET|PIANA|PIANE|GROWER|ELASTIC\w*|PARZ\.? ?FIL\w*` +
		`|INTER\w* FILET\w*|CH ?\d{1,2}|ZNB|6PA|DN ?\d{1,2}|HHC|FHCS|BHCS|PASSO|AF)\b`)
	// una misura qualunque (4X20, D8, L.20, 1/4", DN8): basta solo a un nome italiano o a un composto
	reMisuraGenerica = regexp.MustCompile(`\b\d+(?:[.,]\d+)? ?X ?\d+(?:[.,]\d+)?|\bD(?:IAM(?:ETRO)?)?\.? ?\d+(?:[.,]\d+)?` +
		`|\bL\.? ?=? ?\d|\b\d{1,2}/\d{1,2}"|\bDN ?\d`)
)

// primaLettura e' la prima lettura di re in s i cui caratteri accanto vanno bene (prima il carattere che precede, 0
// in testa; dopo quello che segue, 0 in fondo): e' il lookbehind e il lookahead della sonda, che RE2 non ha. Le
// letture sono una dopo l'altra, senza sovrapporsi: nessuna lettura buona comincia dentro una scartata, perche' le
// espressioni che la usano cominciano con un carattere che non ricompare dentro (la M, una cifra dopo un confine).
// Restituisce anche dove comincia (-1 se niente).
func primaLettura(re *regexp.Regexp, s string, prima, dopo func(byte) bool) (string, int) {
	for _, m := range re.FindAllStringIndex(s, -1) {
		var p, d byte
		if m[0] > 0 {
			p = s[m[0]-1]
		}
		if m[1] < len(s) {
			d = s[m[1]]
		}
		if prima(p) && dopo(d) {
			return s[m[0]:m[1]], m[0]
		}
	}
	return "", -1
}

func cifra(c byte) bool           { return c >= '0' && c <= '9' }
func nonCifra(c byte) bool        { return !cifra(c) }
func sempre(byte) bool            { return true }
func nonAlfanumerico(c byte) bool { return !cifra(c) && (c < 'A' || c > 'Z') }

func cercaFiletto(s string) string {
	x, _ := primaLettura(reFilettoNome, s, nonAlfanumerico, nonCifra)
	return x
}

// cercaPollici e' la prima filettatura in pollici o UNC/UNF, la piu' a sinistra.
func cercaPollici(s string) string {
	x, i := primaLettura(rePolliciNome, s, sempre, nonCifra)
	if loc := reUNCNome.FindStringIndex(s); loc != nil && (i < 0 || loc[0] < i) {
		return s[loc[0]:loc[1]]
	}
	return x
}

// cercaClasse e' la classe di resistenza piu' a sinistra; a pari posizione quella che la sonda prova prima.
func cercaClasse(s string) string {
	migliore, dove := "", -1
	prova := func(x string, i int) {
		if i >= 0 && (dove < 0 || i < dove) {
			migliore, dove = x, i
		}
	}
	prova(primaLettura(reClasseNumero, s, func(c byte) bool { return !cifra(c) && c != '.' }, nonCifra))
	prova(primaLettura(reClasseCL, s, sempre, sempre))
	prova(primaLettura(reClasseInox, s, sempre, sempre))
	prova(primaLettura(reClasseTratto, s, func(c byte) bool { return !cifra(c) && c != '-' }, func(c byte) bool { return !cifra(c) && c != '-' }))
	return migliore
}

// ------------------------------------------------------------------ i nomi di minuteria, i composti, i segni contrari

// compostiMinuteria rendono univoco un nome ambiguo («SPINA ELASTICA», «WELD NUT»), nell'ordine della sonda.
var compostiMinuteria = compila([]string{`SPIN[AE] ELASTIC`, `SPINA CILINDRIC`, `ANELL[OI] ELASTIC`, `ANELL[OI] SEE?GER`, `ANELLO ARR`,
	`INSERT[OI] FILETT`, `INS\.? ?FIL`, `BARR[AE] FILETTAT`, `COPP?IGLIA ELASTICA`, `SPLIT PIN`, `COTTER PIN`, `SPRING PIN`, `ROLL PIN`,
	`DOWEL PIN`, `CLEVIS PIN`, `TAPER PIN`, `RETAINING RING`, `SNAP RING`, `SET SCREW`, `GRUB SCREW`, `WELD STUD`, `RIVET NUT`,
	`THREADED INSERT`, `ANNEAU ELASTIQUE`, `INSERT TARAUDE`, `NUT[- ]WELD`, `WELD NUT`, `WELD BOLT`, `WELD SCREW`})

// negativiMinuteria sono i segni di un pezzo fatto (a disegno, tagliato, lavorato, trattato da noi): mai una
// proposta.
var negativiMinuteria = compila([]string{`A\s?DIS\b`, `DIS\.`, `DISEGNO\b`, `SPECIAL[EI]?\b`, `MODIFICAT`, `RILAVORAT`, `ACCORCIAT`,
	`FORAT[OAIE]\b`, `LASER\b`, `TORNIT`, `LAVORAT[OA]\b`,
	`DA (?:ZINCARE|VERNICIARE|CEMENTARE|TEMPRARE|BONIFIC\w*|PIEGARE|TAGLIARE|TORNIRE|FORARE|FARE)\b`,
	`S ?[23]\d\d(?:J[R0-2]|MC|JR)?\b`, `DC0\d\b`, `DD1\d\b`, `SP\. ?\d`, `VERNICIAT`, `PART\. ?\d`, `AS PER DRAWING\b`, `DWG\b`,
	`MACHINED\b`, `SALDATURA D`})

// compila fa di ogni espressione una lettura che comincia a un confine di parola.
func compila(ee []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(ee))
	for i, e := range ee {
		out[i] = regexp.MustCompile(`\b` + e)
	}
	return out
}

func primoComposto(s string) string { return primoDi(compostiMinuteria, s) }
func primoNegativo(s string) string { return primoDi(negativiMinuteria, s) }

// primoDi e' la lettura della prima espressione della lista che si trova nel nome.
func primoDi(ee []*regexp.Regexp, s string) string {
	for _, re := range ee {
		if x := re.FindString(s); x != "" {
			return x
		}
	}
	return ""
}

func parole(pp ...string) map[string]bool {
	out := make(map[string]bool, len(pp))
	for _, p := range pp {
		out[p] = true
	}
	return out
}

func numeri(nn ...int) map[int]bool {
	out := make(map[int]bool, len(nn))
	for _, n := range nn {
		out[n] = true
	}
	return out
}

// univociMinuteria: in testa al nome sono minuteria (basta un segnale, o una misura per quelli italiani).
var univociMinuteria = parole("BLINDNIET", "BOLT", "BOLTS", "BOULON", "BOULONS", "BULLONE", "BULLONI", "CAPSCREW", "CIRCLIP", "CIRCLIPS",
	"COPIGLIA", "COPIGLIE", "COPPIGLIA", "COPPIGLIE", "DADI", "DADO", "ECROU", "ECROUS", "EINPRESSMUTTER", "FEDERRING",
	"FEDERSCHEIBE", "FLANSCHMUTTER", "GEWINDESTANGE", "GEWINDESTIFT", "GOUPILLE", "GOUPILLES", "GRANI", "GRANO",
	"GROWER", "HHCS", "HUTMUTTER", "KEGELSTIFT", "KERBSTIFT", "LINSENSCHRAUBE", "MUTTER", "MUTTERN", "NIET",
	"NIETMUTTER", "NUT", "NUTS", "PRIGIONIERI", "PRIGIONIERO", "RIVET", "RIVETS", "RIVETTI", "RIVETTO", "SCHRAUBE",
	"SCHRAUBEN", "SCHWEISSBOLZEN", "SCHWEISSMUTTER", "SCREW", "SCREWS", "SECHSKANTMUTTER", "SECHSKANTSCHRAUBE",
	"SEEGER", "SENKSCHRAUBE", "SHCS", "SICHERUNGSMUTTER", "SICHERUNGSRING", "SPANNSCHEIBE", "SPANNSTIFT", "SPLINT",
	"SPRENGRING", "UNTERLEGSCHEIBE", "VIS", "VITE", "VITI", "WASHER", "WASHERS", "ZYLINDERSCHRAUBE", "ZYLINDERSTIFT")

// univociItaliani: i nomi italiani fra gli univoci, a cui basta anche una misura qualunque.
var univociItaliani = parole("BULLONE", "BULLONI", "COPIGLIA", "COPIGLIE", "COPPIGLIA", "COPPIGLIE", "DADI", "DADO", "GRANI", "GRANO",
	"GROWER", "PRIGIONIERI", "PRIGIONIERO", "RIVETTI", "RIVETTO", "SEEGER", "VITE", "VITI")

// ambiguiMinuteria esistono anche custom (rondelle tagliate al laser, spine, perni, inserti di tornitura): vogliono un
// segnale da minuteria.
var ambiguiMinuteria = parole("ANELLI", "ANELLO", "BOLZEN", "CHIAVETTA", "INS", "INSERT", "INSERTI", "INSERTO", "KEY", "LINGUETTA", "PIN",
	"PINS", "ROND", "RONDELLA", "RONDELLE", "ROSETTA", "ROSETTE", "SCHEIBE", "SPINA", "SPINE", "STIFT", "STUD")

// nomiCustomMinuteria: un nome di pezzo custom fra le prime parole ferma ogni proposta, anche con una norma
// («PIASTRA CON DADI DIN 928 SALDATI»).
var nomiCustomMinuteria = parole("ALBERO", "ARM", "ASS", "ASSEMBLY", "ASSIEME", "ASSY", "ASTA", "ATTREZZATURA", "BEAD", "BLOCCA",
	"BLOCCHETTO", "BLOCCO", "BRACCIO", "BRACKET", "BUSSOLA", "CARTER", "CHIUSURA", "CINTURA", "COMPLESSIVO", "COMPOSTO",
	"COPERCHIO", "CORDONE", "COVER", "DISTANZIALE", "FERMA", "FERMO", "FILLET", "FORMING", "FRAME", "GRUPPO", "GUIDA", "GUIDE",
	"HOLDER", "HOUSING", "KIT", "LAMIERA", "LAMIERINA", "LEVA", "MASCHERA", "MOZZO", "NERVATURA", "PIASTRA",
	"PIASTRINA", "PIATTO", "PINZA", "PLATE", "PLUG", "PROFILO", "RINFORZO", "ROD", "SALDATO", "SALDATURA",
	"SEMILAVORATO", "SHAFT", "SHEET", "SOSTEGNO", "STAFFA", "STRUCTURE", "STRUTTURA", "SUPPORT", "SUPPORTI", "SUPPORTO",
	"TAPPO", "TELAIO", "TRAVERSO", "TUBE", "TUBO", "TUBOLARE", "WELDED")

// Le norme di PEZZO di minuteria (viti, dadi, rondelle, spine, copiglie, perni normati, rivetti, inserti): solo numeri
// di cui si e' sicuri, da rivedere con l'ufficio tecnico. Un numero che manca non fa danni, uno sbagliato si'.
var (
	normeISO = numeri(1051, 1207, 1234, 1479, 1481, 1482, 1483, 1580, 2009, 2010, 2338, 2339, 2341, 4014, 4015, 4016, 4017, 4018, 4026,
		4027, 4028, 4029, 4032, 4033, 4034, 4035, 4036, 4161, 4762, 4766, 7040, 7041, 7042, 7043, 7044, 7045, 7046, 7047,
		7048, 7049, 7050, 7051, 7089, 7090, 7091, 7092, 7093, 7094, 7379, 7380, 7434, 7435, 7436, 7719, 7720, 8673, 8674,
		8675, 8676, 8677, 8734, 8735, 8736, 8737, 8738, 8739, 8740, 8741, 8742, 8743, 8744, 8745, 8746, 8747, 8748, 8750,
		8752, 8765, 10511, 10512, 10513, 10642, 10673, 12125, 12126, 13337, 13918, 14579, 14580, 14581, 14582, 14583, 14584,
		14588, 14589, 15071, 15072, 15480, 15481, 15482, 15483, 15977, 15978, 15979, 15980, 15981, 15982, 15983, 15984,
		16582, 16583, 21269, 21670)
	normeEN  = numeri(1661, 1662, 1663, 1664, 1665, 14399, 15048)
	normeDIN = numeri(84, 85, 125, 126, 127, 128, 137, 186, 188, 261, 315, 316, 319, 404, 427, 433, 434, 435, 436, 439, 440, 441, 444,
		466, 467, 471, 472, 508, 529, 546, 547, 551, 553, 555, 557, 558, 561, 562, 571, 580, 582, 603, 604, 605, 607, 608,
		609, 610, 660, 661, 662, 674, 675, 741, 787, 797, 835, 908, 910, 912, 913, 914, 915, 916, 917, 921, 923, 927, 928,
		929, 931, 933, 934, 935, 936, 937, 938, 939, 940, 960, 961, 963, 964, 965, 966, 971, 975, 976, 979, 980, 982, 983,
		984, 985, 986, 988, 1440, 1441, 1444, 1471, 1472, 1473, 1474, 1475, 1476, 1481, 1587, 1804, 3404, 3405, 3570, 6319,
		6325, 6330, 6331, 6334, 6340, 6796, 6797, 6798, 6799, 6885, 6888, 6912, 6914, 6915, 6916, 6921, 6923, 6926, 6927,
		7337, 7338, 7343, 7344, 7346, 7349, 7500, 7504, 7513, 7516, 7603, 7967, 7968, 7969, 7980, 7981, 7982, 7983, 7984,
		7985, 7989, 7990, 7991, 7993, 9021, 11024, 32501, 34820, 34821, 71412, 71752)
	normeUNI = numeri(1336, 1707, 1751, 5587, 5588, 5589, 5721, 5737, 5738, 5739, 5740, 5923, 5925, 5927, 5931, 5933, 6592, 6593, 6873,
		6954, 6955, 7433, 7434, 7435, 7436, 7437, 7473, 7474, 8833)
)

// Le norme che stanno sui disegni ma non sono pezzi (tolleranze, rugosita', saldatura, materiali): non fanno mai una
// proposta, e un «no» le nomina.
var (
	nonPezzoISO = numeri(68, 128, 129, 261, 262, 286, 724, 898, 965, 1101, 1302, 1461, 2081, 2553, 2768, 3040, 3269, 3834, 4042, 4063, 4759,
		5455, 5459, 5817, 6410, 7200, 8015, 8062, 9001, 9013, 9227, 9606, 9692, 10683, 10684, 12944, 13920, 14175, 14731,
		15614, 16016, 22768)
	nonPezzoEN = numeri(287, 1011, 1090, 1461, 3834, 9001, 10025, 10029, 10051, 10083, 10088, 10111, 10130, 10143, 10149, 10152, 10204,
		10210, 10217, 10219, 10268, 10277, 10293, 10297, 10305, 10346, 12944, 13920, 15085, 20273, 20898, 22553, 22768)
	nonPezzoDIN = numeri(13, 76, 267, 1016, 1025, 1026, 1028, 1029, 1541, 1623, 2391, 2393, 2395, 2448, 6935, 7168, 7526, 8580, 16901, 17100,
		50049, 59410, 59411)
	nonPezzoUNI = numeri(7070, 10025, 10217, 10219, 10305, 22768)
)
