package grammatica

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// Decodifica è la porta stretta di un file v1 (parte 1 §7.1, §7.5; par.3.4.3 del piano A). Rifiuta:
//   - la BOM (contratto.bom), l'UTF-8 non valido e i surrogati soli (contratto.utf8_non_valido), il testo
//     dopo l'oggetto (contratto.valore_dopo_oggetto), il JSON malformato (contratto.json_non_valido);
//   - versione_schema assente o diversa da 1 (contratto.versione_schema_ignota): con uno schema ignoto non
//     si guarda altro, perché le sue chiavi potrebbero essere giuste per un'altra versione;
//   - le chiavi sconosciute, anche i campi legacy di cliente.regole (R20 c), ripetute o con maiuscole
//     diverse da quelle del tag; i null; i numeri non interi;
//   - un booleano o un intero obbligatorio (senza omitempty) assente (contratto.campo_obbligatorio): dopo la
//     decodifica non si distinguerebbe da false o da 0.
//
// Il passaggio a token conosce i tag di ogni livello del DTO per riflessione, come CampiRegole del legacy:
// encoding/json accetterebbe in silenzio una chiave con le maiuscole sbagliate, lascerebbe vincere un
// doppione e cambierebbe un surrogato solo in U+FFFD (T21). Solo dopo, json.Decoder con
// DisallowUnknownFields e UseNumber riempie il DTO.
//
// L'errore è un *evidenze.ErroreContratto con tutte le diagnostiche trovate, ciascuna con il percorso del
// campo; con un errore la grammatica restituita è vuota. Decodifica non valida il contenuto: lo fa Valida.
func Decodifica(raw []byte) (Grammatica, error) {
	var g Grammatica
	if d := decodificaStretta(raw, &g, "versione_schema", VersioneSchema); len(d) > 0 {
		return Grammatica{}, &evidenze.ErroreContratto{Diagnostiche: d}
	}
	return g, nil
}

// profonditaMassima: gli oggetti e gli array annidati che la lettura accetta. Il DTO più profondo ne ha
// meno di dieci; il tetto protegge la pila da un file costruito apposta.
const profonditaMassima = 64

// decodificaStretta legge raw in dst (un puntatore) con le regole di Decodifica, controllando che la chiave
// di versione in testa all'oggetto valga versione. La usano Decodifica e LeggiIndice. nil vuol dire letto.
func decodificaStretta(raw []byte, dst any, chiaveVersione string, versione int) []evidenze.Diagnostica {
	if bytes.HasPrefix(raw, []byte("\xef\xbb\xbf")) {
		return []evidenze.Diagnostica{{
			Codice:    CodiceBOM,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Messaggio: "il file comincia con la firma UTF-8 (BOM): va salvato in UTF-8 senza BOM",
		}}
	}
	if !utf8.Valid(raw) {
		return []evidenze.Diagnostica{{
			Codice:    CodiceUTF8NonValido,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Messaggio: "il file non è UTF-8 valido",
		}}
	}
	l := &lettoreJSON{b: raw}
	radice, d := l.documento()
	if d != nil {
		return []evidenze.Diagnostica{*d}
	}
	if radice.tipo != nodoOggetto {
		return []evidenze.Diagnostica{{
			Codice:    CodiceJSONNonValido,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Messaggio: "il file deve contenere un oggetto JSON",
		}}
	}
	if d := controllaVersione(radice, chiaveVersione, versione); d != nil {
		return []evidenze.Diagnostica{*d}
	}
	c := &controlloTipi{}
	c.nodo(radice, reflect.TypeOf(dst).Elem(), "")
	if len(c.out) > 0 {
		return c.out
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(dst); err != nil {
		// Dopo i controlli sopra non dovrebbe succedere: resta come seconda rete.
		return []evidenze.Diagnostica{{
			Codice:    CodiceJSONNonValido,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Messaggio: "lettura del JSON: " + err.Error(),
		}}
	}
	return nil
}

// controllaVersione: la chiave di versione c'è, è un intero ed è quella attesa. Ogni sua occorrenza conta.
func controllaVersione(radice *nodoJSON, chiave string, attesa int) *evidenze.Diagnostica {
	trovata := false
	for _, c := range radice.coppie {
		if c.chiave != chiave {
			continue
		}
		trovata = true
		if c.valore.tipo != nodoNumero || c.valore.testo != strconv.Itoa(attesa) {
			return &evidenze.Diagnostica{
				Codice:    CodiceVersioneSchemaIgnota,
				Gravita:   evidenze.GravitaErrore,
				Natura:    evidenze.NaturaContratto,
				Percorso:  chiave,
				Messaggio: fmt.Sprintf("%s %s: questo codice sa leggere solo la versione %d; nessuna attivazione", chiave, c.valore.descrivi(), attesa),
			}
		}
	}
	if !trovata {
		return &evidenze.Diagnostica{
			Codice:    CodiceVersioneSchemaIgnota,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  chiave,
			Messaggio: fmt.Sprintf("manca %s: lo schema è ignoto e il file non si attiva", chiave),
		}
	}
	return nil
}

// ---- il passaggio a token: un albero che conserva ordine, doppioni e testo dei numeri ----

type tipoNodo int

const (
	nodoOggetto tipoNodo = iota
	nodoArray
	nodoStringa
	nodoNumero
	nodoBooleano
	nodoNull
)

type nodoJSON struct {
	tipo     tipoNodo
	coppie   []coppiaJSON // oggetto: nell'ordine del file, doppioni compresi
	elementi []*nodoJSON  // array
	testo    string       // stringa decodificata; numero com'è scritto; booleano
	intero   bool         // numero senza decimali né esponente
}

type coppiaJSON struct {
	chiave string
	valore *nodoJSON
}

func (n *nodoJSON) descrivi() string {
	switch n.tipo {
	case nodoOggetto:
		return "un oggetto"
	case nodoArray:
		return "un array"
	case nodoStringa:
		return "una stringa"
	case nodoNumero:
		return "il numero " + n.testo
	case nodoBooleano:
		return "un booleano"
	}
	return "null"
}

// lettoreJSON: un lettore JSON (RFC 8259) che si ferma al primo errore di sintassi. L'UTF-8 dei byte è già
// controllato; qui si controllano gli escape \u, che possono scrivere un surrogato solo.
type lettoreJSON struct {
	b    []byte
	i    int
	prof int
}

func (l *lettoreJSON) documento() (*nodoJSON, *evidenze.Diagnostica) {
	l.spazi()
	n, d := l.valore()
	if d != nil {
		return nil, d
	}
	l.spazi()
	if l.i < len(l.b) {
		return nil, &evidenze.Diagnostica{
			Codice:    CodiceValoreDopoOggetto,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Messaggio: fmt.Sprintf("al byte %d, dopo l'oggetto JSON, c'è dell'altro testo: il file contiene un valore solo", l.i),
		}
	}
	return n, nil
}

func (l *lettoreJSON) sintassi(cosa string) *evidenze.Diagnostica {
	return &evidenze.Diagnostica{
		Codice:    CodiceJSONNonValido,
		Gravita:   evidenze.GravitaErrore,
		Natura:    evidenze.NaturaContratto,
		Messaggio: fmt.Sprintf("JSON non valido al byte %d: %s", l.i, cosa),
	}
}

func (l *lettoreJSON) spazi() {
	for l.i < len(l.b) {
		switch l.b[l.i] {
		case ' ', '\t', '\n', '\r':
			l.i++
		default:
			return
		}
	}
}

func (l *lettoreJSON) valore() (*nodoJSON, *evidenze.Diagnostica) {
	if l.i >= len(l.b) {
		return nil, l.sintassi("il testo finisce dove serve un valore")
	}
	switch c := l.b[l.i]; {
	case c == '{':
		return l.oggetto()
	case c == '[':
		return l.array()
	case c == '"':
		s, d := l.stringa()
		if d != nil {
			return nil, d
		}
		return &nodoJSON{tipo: nodoStringa, testo: s}, nil
	case c == '-' || (c >= '0' && c <= '9'):
		return l.numero()
	case bytes.HasPrefix(l.b[l.i:], []byte("true")):
		l.i += 4
		return &nodoJSON{tipo: nodoBooleano, testo: "true"}, nil
	case bytes.HasPrefix(l.b[l.i:], []byte("false")):
		l.i += 5
		return &nodoJSON{tipo: nodoBooleano, testo: "false"}, nil
	case bytes.HasPrefix(l.b[l.i:], []byte("null")):
		l.i += 4
		return &nodoJSON{tipo: nodoNull}, nil
	}
	return nil, l.sintassi("carattere inatteso")
}

func (l *lettoreJSON) entra() *evidenze.Diagnostica {
	l.prof++
	if l.prof > profonditaMassima {
		return l.sintassi(fmt.Sprintf("più di %d livelli annidati", profonditaMassima))
	}
	return nil
}

func (l *lettoreJSON) oggetto() (*nodoJSON, *evidenze.Diagnostica) {
	if d := l.entra(); d != nil {
		return nil, d
	}
	defer func() { l.prof-- }()
	n := &nodoJSON{tipo: nodoOggetto}
	l.i++ // «{»
	l.spazi()
	if l.i < len(l.b) && l.b[l.i] == '}' {
		l.i++
		return n, nil
	}
	for {
		l.spazi()
		if l.i >= len(l.b) || l.b[l.i] != '"' {
			return nil, l.sintassi("serve una chiave tra virgolette")
		}
		k, d := l.stringa()
		if d != nil {
			return nil, d
		}
		l.spazi()
		if l.i >= len(l.b) || l.b[l.i] != ':' {
			return nil, l.sintassi("serve «:» dopo la chiave")
		}
		l.i++
		l.spazi()
		v, d := l.valore()
		if d != nil {
			return nil, d
		}
		n.coppie = append(n.coppie, coppiaJSON{chiave: k, valore: v})
		l.spazi()
		if l.i >= len(l.b) {
			return nil, l.sintassi("l'oggetto non è chiuso")
		}
		switch l.b[l.i] {
		case ',':
			l.i++
		case '}':
			l.i++
			return n, nil
		default:
			return nil, l.sintassi("serve «,» o «}»")
		}
	}
}

func (l *lettoreJSON) array() (*nodoJSON, *evidenze.Diagnostica) {
	if d := l.entra(); d != nil {
		return nil, d
	}
	defer func() { l.prof-- }()
	n := &nodoJSON{tipo: nodoArray}
	l.i++ // «[»
	l.spazi()
	if l.i < len(l.b) && l.b[l.i] == ']' {
		l.i++
		return n, nil
	}
	for {
		l.spazi()
		v, d := l.valore()
		if d != nil {
			return nil, d
		}
		n.elementi = append(n.elementi, v)
		l.spazi()
		if l.i >= len(l.b) {
			return nil, l.sintassi("l'array non è chiuso")
		}
		switch l.b[l.i] {
		case ',':
			l.i++
		case ']':
			l.i++
			return n, nil
		default:
			return nil, l.sintassi("serve «,» o «]»")
		}
	}
}

// stringa legge una stringa JSON e la decodifica. Un surrogato alto senza il basso subito dopo, o un basso
// da solo, è contratto.utf8_non_valido: encoding/json lo cambierebbe in U+FFFD senza dirlo.
func (l *lettoreJSON) stringa() (string, *evidenze.Diagnostica) {
	l.i++ // «"»
	var sb strings.Builder
	for {
		if l.i >= len(l.b) {
			return "", l.sintassi("la stringa non è chiusa")
		}
		c := l.b[l.i]
		switch {
		case c == '"':
			l.i++
			return sb.String(), nil
		case c < 0x20:
			return "", l.sintassi("carattere di controllo non in escape dentro una stringa")
		case c == '\\':
			if l.i+1 >= len(l.b) {
				return "", l.sintassi("escape incompleto")
			}
			e := l.b[l.i+1]
			l.i += 2
			switch e {
			case '"', '\\', '/':
				sb.WriteByte(e)
			case 'b':
				sb.WriteByte('\b')
			case 'f':
				sb.WriteByte('\f')
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case 'u':
				r, d := l.escapeU()
				if d != nil {
					return "", d
				}
				sb.WriteRune(r)
			default:
				return "", l.sintassi("escape sconosciuto")
			}
		default:
			r, n := utf8.DecodeRune(l.b[l.i:])
			sb.WriteRune(r)
			l.i += n
		}
	}
}

// escapeU legge le quattro cifre dopo «\u» (e, per un surrogato alto, la coppia che segue).
func (l *lettoreJSON) escapeU() (rune, *evidenze.Diagnostica) {
	r, ok := l.quattroCifre()
	if !ok {
		return 0, l.sintassi("\\u senza quattro cifre esadecimali")
	}
	if !utf16.IsSurrogate(r) {
		return r, nil
	}
	if r < 0xDC00 && bytes.HasPrefix(l.b[l.i:], []byte(`\u`)) {
		salva := l.i
		l.i += 2
		if basso, ok := l.quattroCifre(); ok && basso >= 0xDC00 && basso <= 0xDFFF {
			return utf16.DecodeRune(r, basso), nil
		}
		l.i = salva
	}
	return 0, &evidenze.Diagnostica{
		Codice:    CodiceUTF8NonValido,
		Gravita:   evidenze.GravitaErrore,
		Natura:    evidenze.NaturaContratto,
		Messaggio: fmt.Sprintf("al byte %d un surrogato solo (\\u%04x): non è un carattere", l.i, r),
	}
}

func (l *lettoreJSON) quattroCifre() (rune, bool) {
	if l.i+4 > len(l.b) {
		return 0, false
	}
	v, err := strconv.ParseUint(string(l.b[l.i:l.i+4]), 16, 32)
	if err != nil {
		return 0, false
	}
	l.i += 4
	return rune(v), true
}

// numero legge un numero con la grammatica di RFC 8259 e ne conserva il testo.
func (l *lettoreJSON) numero() (*nodoJSON, *evidenze.Diagnostica) {
	inizio := l.i
	cifre := func() int {
		n := 0
		for l.i < len(l.b) && l.b[l.i] >= '0' && l.b[l.i] <= '9' {
			l.i++
			n++
		}
		return n
	}
	if l.b[l.i] == '-' {
		l.i++
	}
	if l.i < len(l.b) && l.b[l.i] == '0' {
		l.i++
	} else if cifre() == 0 {
		return nil, l.sintassi("numero senza cifre")
	}
	intero := true
	if l.i < len(l.b) && l.b[l.i] == '.' {
		l.i++
		intero = false
		if cifre() == 0 {
			return nil, l.sintassi("numero senza cifre dopo il punto")
		}
	}
	if l.i < len(l.b) && (l.b[l.i] == 'e' || l.b[l.i] == 'E') {
		l.i++
		intero = false
		if l.i < len(l.b) && (l.b[l.i] == '+' || l.b[l.i] == '-') {
			l.i++
		}
		if cifre() == 0 {
			return nil, l.sintassi("esponente senza cifre")
		}
	}
	return &nodoJSON{tipo: nodoNumero, testo: string(l.b[inizio:l.i]), intero: intero}, nil
}

// ---- il controllo dei tipi: l'albero contro i tag del DTO ----

var tipoTextUnmarshaler = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()

// controlloTipi confronta l'albero con il tipo Go che lo riceverà: chiavi, doppioni, maiuscole, null,
// numeri interi, lunghezza degli array a dimensione fissa, valori testuali come gli UUID. Raccoglie tutte
// le diagnostiche, nell'ordine del file.
type controlloTipi struct {
	out []evidenze.Diagnostica
}

func unisci(percorso, chiave string) string {
	if percorso == "" {
		return chiave
	}
	return percorso + "." + chiave
}

func (c *controlloTipi) tipoSbagliato(n *nodoJSON, percorso, atteso string) {
	c.out = append(c.out, evidenze.Diagnostica{
		Codice:    CodiceJSONNonValido,
		Gravita:   evidenze.GravitaErrore,
		Natura:    evidenze.NaturaContratto,
		Percorso:  percorso,
		Messaggio: fmt.Sprintf("serve %s, c'è %s", atteso, n.descrivi()),
	})
}

func (c *controlloTipi) nodo(n *nodoJSON, t reflect.Type, percorso string) {
	if n.tipo == nodoNull {
		c.out = append(c.out, evidenze.Diagnostica{
			Codice:    CodiceNull,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  percorso,
			Messaggio: "null non è ammesso: un campo facoltativo si omette",
		})
		return
	}
	if reflect.PointerTo(t).Implements(tipoTextUnmarshaler) {
		if n.tipo != nodoStringa {
			c.tipoSbagliato(n, percorso, "una stringa")
			return
		}
		if err := reflect.New(t).Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(n.testo)); err != nil {
			c.out = append(c.out, evidenze.Diagnostica{
				Codice:    CodiceJSONNonValido,
				Gravita:   evidenze.GravitaErrore,
				Natura:    evidenze.NaturaContratto,
				Percorso:  percorso,
				Messaggio: "valore illeggibile: " + err.Error(),
			})
		}
		return
	}
	switch t.Kind() {
	case reflect.Pointer:
		c.nodo(n, t.Elem(), percorso)
	case reflect.Struct:
		if n.tipo != nodoOggetto {
			c.tipoSbagliato(n, percorso, "un oggetto")
			return
		}
		c.oggetto(n, t, percorso)
	case reflect.Slice:
		if n.tipo != nodoArray {
			c.tipoSbagliato(n, percorso, "un array")
			return
		}
		for i, e := range n.elementi {
			c.nodo(e, t.Elem(), fmt.Sprintf("%s[%d]", percorso, i))
		}
	case reflect.Array:
		if n.tipo != nodoArray {
			c.tipoSbagliato(n, percorso, "un array")
			return
		}
		if len(n.elementi) != t.Len() {
			// encoding/json scarterebbe gli elementi in più e riempirebbe di vuoto quelli in meno.
			c.out = append(c.out, evidenze.Diagnostica{
				Codice:    CodiceJSONNonValido,
				Gravita:   evidenze.GravitaErrore,
				Natura:    evidenze.NaturaContratto,
				Percorso:  percorso,
				Messaggio: fmt.Sprintf("servono esattamente %d elementi, ce ne sono %d", t.Len(), len(n.elementi)),
			})
			return
		}
		for i, e := range n.elementi {
			c.nodo(e, t.Elem(), fmt.Sprintf("%s[%d]", percorso, i))
		}
	case reflect.String:
		if n.tipo != nodoStringa {
			c.tipoSbagliato(n, percorso, "una stringa")
		}
	case reflect.Bool:
		if n.tipo != nodoBooleano {
			c.tipoSbagliato(n, percorso, "un booleano")
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if n.tipo != nodoNumero {
			c.tipoSbagliato(n, percorso, "un numero intero")
			return
		}
		if !n.intero {
			c.out = append(c.out, evidenze.Diagnostica{
				Codice:    CodiceNumeroNonIntero,
				Gravita:   evidenze.GravitaErrore,
				Natura:    evidenze.NaturaContratto,
				Percorso:  percorso,
				Messaggio: fmt.Sprintf("il numero %s non è intero: niente decimali né esponente", n.testo),
			})
			return
		}
		if _, err := strconv.ParseInt(n.testo, 10, t.Bits()); err != nil {
			c.out = append(c.out, evidenze.Diagnostica{
				Codice:    CodiceJSONNonValido,
				Gravita:   evidenze.GravitaErrore,
				Natura:    evidenze.NaturaContratto,
				Percorso:  percorso,
				Messaggio: fmt.Sprintf("il numero %s è fuori dall'intervallo ammesso", n.testo),
			})
		}
	default:
		// Nessun campo dei DTO ha un altro tipo: se ne nasce uno, il controllo va esteso, non saltato.
		c.out = append(c.out, evidenze.Diagnostica{
			Codice:    CodiceJSONNonValido,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  percorso,
			Messaggio: fmt.Sprintf("tipo Go %s non previsto dalla lettura stretta", t),
		})
	}
}

// oggetto controlla le chiavi di un oggetto contro i tag della struct. Un booleano o un intero senza
// omitempty è obbligatorio: dopo la decodifica la sua assenza non si distingue più da false o da 0 (una
// parte senza «min» diventerebbe facoltativa, un segmento senza «identitario» non identitario, un'etichetta
// senza «obbligatoria» facoltativa), quindi si controlla qui (contratto.campo_obbligatorio). Stringhe ed
// elenchi obbligatori li controlla Valida, che vede il valore vuoto.
func (c *controlloTipi) oggetto(n *nodoJSON, t reflect.Type, percorso string) {
	campi := map[string]reflect.Type{}
	var nomi, obbligatori []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		nome, opzioni, _ := strings.Cut(f.Tag.Get("json"), ",")
		if nome == "" || nome == "-" || !f.IsExported() {
			continue
		}
		campi[nome] = f.Type
		nomi = append(nomi, nome)
		if k := f.Type.Kind(); (k == reflect.Bool || (k >= reflect.Int && k <= reflect.Int64)) && !in("omitempty", strings.Split(opzioni, ",")) {
			obbligatori = append(obbligatori, nome)
		}
	}
	var viste []string
	for _, cp := range n.coppie {
		p := unisci(percorso, cp.chiave)
		if in(cp.chiave, viste) {
			c.out = append(c.out, evidenze.Diagnostica{
				Codice:    CodiceChiaveRipetuta,
				Gravita:   evidenze.GravitaErrore,
				Natura:    evidenze.NaturaContratto,
				Percorso:  p,
				Messaggio: fmt.Sprintf("la chiave %q compare più di una volta nello stesso oggetto", cp.chiave),
			})
			continue
		}
		viste = append(viste, cp.chiave)
		if ft, ok := campi[cp.chiave]; ok {
			c.nodo(cp.valore, ft, p)
			continue
		}
		simile := ""
		for _, nome := range nomi {
			if strings.EqualFold(nome, cp.chiave) {
				simile = nome
				break
			}
		}
		if simile != "" {
			c.out = append(c.out, evidenze.Diagnostica{
				Codice:    CodiceChiaveMaiuscole,
				Gravita:   evidenze.GravitaErrore,
				Natura:    evidenze.NaturaContratto,
				Percorso:  p,
				Messaggio: fmt.Sprintf("la chiave %q si scrive %q: le maiuscole contano", cp.chiave, simile),
			})
			continue
		}
		c.out = append(c.out, evidenze.Diagnostica{
			Codice:    CodiceChiaveSconosciuta,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  p,
			Messaggio: fmt.Sprintf("la chiave %q non esiste (ammesse: %s)", cp.chiave, strings.Join(nomi, ", ")),
		})
	}
	for _, nome := range obbligatori {
		presente := false
		for _, cp := range n.coppie {
			if strings.EqualFold(cp.chiave, nome) { // con le maiuscole sbagliate è già una diagnosi
				presente = true
				break
			}
		}
		if !presente {
			c.out = append(c.out, evidenze.Diagnostica{
				Codice:    CodiceCampoObbligatorio,
				Gravita:   evidenze.GravitaErrore,
				Natura:    evidenze.NaturaContratto,
				Percorso:  unisci(percorso, nome),
				Messaggio: fmt.Sprintf("manca la chiave %q: un booleano o un intero non ha un valore sottinteso", nome),
			})
		}
	}
}
