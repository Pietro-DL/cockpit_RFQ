// Package jsoncanonico scrive un valore Go come JSON canonico, sempre con gli stessi byte:
//   - le chiavi degli oggetti in ordine di byte UTF-8, e nessuno spazio;
//   - le stringhe in UTF-8 grezzo, con l'escape solo di «"», «\» e dei caratteri di controllo
//     U+0000–U+001F (\b \t \n \f \r, gli altri come \u00xx); anche U+2028 e U+2029 restano grezzi;
//   - i numeri solo interi, oppure letterali JSON già scritti (json.Number, json.RawMessage), copiati come testo.
//
// Un float di Go o una stringa non UTF-8 sono un errore: niente arrotondamenti, niente U+FFFD silenziosi.
// L'ordine degli array è quello dato: ordinare gli elenchi senza significato tocca a chi chiama.
// Non è RFC 8785 completo, perché non scrive numeri decimali.
//
// Perché non encoding/json (piano A, par.3.4.3): fa l'escape di U+2028 e U+2029, sostituisce l'UTF-8 non
// valido con U+FFFD e, in lettura, lascia vincere una chiave ripetuta. Qui ognuno di questi casi è un
// errore o un byte fisso. Il package non sa niente di RFQ né del motore: è un'utilità senza dominio, per
// questo sta in platform (P1 §7.1: «serializzazione canonica versionata, riproducibile»).
package jsoncanonico

import (
	"bytes"
	"crypto/sha256"
	"encoding"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Versione della canonicalizzazione. Entra in ogni impronta che la usa: cambiarla cambia tutti gli hash, e si
// fa solo con un commit che lo dichiara.
const Versione = "canonico-1"

// profonditaMassima ferma un valore che punta a sé stesso (o un JSON annidato oltre misura) con un errore,
// invece di consumare tutto lo stack.
const profonditaMassima = 512

var (
	tipoMarshaler     = reflect.TypeFor[json.Marshaler]()
	tipoTextMarshaler = reflect.TypeFor[encoding.TextMarshaler]()
	tipoNumber        = reflect.TypeFor[json.Number]()
	tipoRawMessage    = reflect.TypeFor[json.RawMessage]()
)

// Codifica restituisce il JSON canonico di v. Accetta: struct (i campi seguono i tag json, omitempty e
// omitzero compresi), mappe con chiave stringa, slice, array, puntatori, stringhe, booleani, interi,
// json.Number e json.RawMessage. Un json.RawMessage viene riletto e ricanonicalizzato, conservando il
// testo dei numeri. Un tipo che ha MarshalJSON o MarshalText (time.Time, uuid.UUID) passa dal suo metodo, e
// il risultato si ricanonicalizza.
func Codifica(v any) ([]byte, error) {
	var b bytes.Buffer
	if v == nil {
		b.WriteString("null")
		return b.Bytes(), nil
	}
	// Una copia indirizzabile: così anche i metodi con ricevitore puntatore valgono per il valore di testa,
	// come per i campi delle struct.
	rv := reflect.New(reflect.TypeOf(v)).Elem()
	rv.Set(reflect.ValueOf(v))
	if err := codificaValore(&b, rv, 0); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Impronta: lo sha256 dei byte, in esadecimale minuscolo (64 caratteri, il char(64) della parte 1 §9.2).
func Impronta(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// ImprontaDi: Codifica seguita da Impronta.
func ImprontaDi(v any) (string, error) {
	b, err := Codifica(v)
	if err != nil {
		return "", err
	}
	return Impronta(b), nil
}

func codificaValore(b *bytes.Buffer, v reflect.Value, prof int) error {
	if prof > profonditaMassima {
		return errors.New("jsoncanonico: valore annidato oltre la profondità massima (riferimento circolare?)")
	}
	if !v.IsValid() {
		b.WriteString("null")
		return nil
	}
	t := v.Type()

	// I due letterali già scritti vengono prima dei metodi: json.RawMessage ha MarshalJSON, ma va riletto
	// qui, con le regole strette di questo package.
	switch t {
	case tipoNumber:
		s := v.String()
		if !numeroValido(s) {
			return fmt.Errorf("jsoncanonico: json.Number %q non è un numero JSON", s)
		}
		b.WriteString(s)
		return nil
	case tipoRawMessage:
		if v.IsNil() {
			b.WriteString("null")
			return nil
		}
		return ricanonicalizza(b, v.Bytes(), prof)
	}

	if m, ok := metodo(v, tipoMarshaler); ok {
		// Un puntatore nil, o un campo di tipo interfaccia (json.Marshaler) senza valore, è null come in
		// encoding/json: chiamarne il metodo andrebbe in panico.
		if nilla(m) {
			b.WriteString("null")
			return nil
		}
		raw, err := m.Interface().(json.Marshaler).MarshalJSON()
		if err != nil {
			return fmt.Errorf("jsoncanonico: MarshalJSON di %s: %w", t, err)
		}
		return ricanonicalizza(b, raw, prof)
	}
	if m, ok := metodo(v, tipoTextMarshaler); ok {
		if nilla(m) {
			b.WriteString("null")
			return nil
		}
		testo, err := m.Interface().(encoding.TextMarshaler).MarshalText()
		if err != nil {
			return fmt.Errorf("jsoncanonico: MarshalText di %s: %w", t, err)
		}
		return scriviStringa(b, string(testo))
	}

	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b.WriteString(strconv.FormatInt(v.Int(), 10))
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		b.WriteString(strconv.FormatUint(v.Uint(), 10))
		return nil
	case reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return fmt.Errorf("jsoncanonico: %s non ammesso (solo numeri interi o letterali già scritti)", t)
	case reflect.String:
		return scriviStringa(b, v.String())
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			b.WriteString("null")
			return nil
		}
		return codificaValore(b, v.Elem(), prof+1)
	case reflect.Slice:
		if v.IsNil() {
			b.WriteString("null")
			return nil
		}
		if t.Elem().Kind() == reflect.Uint8 && !haMetodi(t.Elem()) {
			// Come encoding/json: i byte grezzi sono una stringa base64 standard.
			return scriviStringa(b, base64.StdEncoding.EncodeToString(v.Bytes()))
		}
		return codificaElenco(b, v, prof)
	case reflect.Array:
		return codificaElenco(b, v, prof)
	case reflect.Map:
		return codificaMappa(b, v, prof)
	case reflect.Struct:
		return codificaStruct(b, v, prof)
	}
	return fmt.Errorf("jsoncanonico: tipo %s non ammesso", t)
}

// metodo dice se v (o il suo indirizzo, quando c'è) implementa l'interfaccia, come fa encoding/json.
func metodo(v reflect.Value, iface reflect.Type) (reflect.Value, bool) {
	if v.Type().Implements(iface) {
		return v, true
	}
	if v.Kind() != reflect.Pointer && v.CanAddr() && reflect.PointerTo(v.Type()).Implements(iface) {
		return v.Addr(), true
	}
	return reflect.Value{}, false
}

// nilla: un puntatore o un'interfaccia senza valore, su cui un metodo non si può chiamare.
func nilla(v reflect.Value) bool {
	return (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) && v.IsNil()
}

func haMetodi(t reflect.Type) bool {
	p := reflect.PointerTo(t)
	return t.Implements(tipoMarshaler) || t.Implements(tipoTextMarshaler) ||
		p.Implements(tipoMarshaler) || p.Implements(tipoTextMarshaler)
}

func codificaElenco(b *bytes.Buffer, v reflect.Value, prof int) error {
	b.WriteByte('[')
	for i := 0; i < v.Len(); i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		if err := codificaValore(b, v.Index(i), prof+1); err != nil {
			return err
		}
	}
	b.WriteByte(']')
	return nil
}

func codificaMappa(b *bytes.Buffer, v reflect.Value, prof int) error {
	if v.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("jsoncanonico: mappa con chiave %s non ammessa (solo chiavi stringa)", v.Type().Key())
	}
	if v.IsNil() {
		b.WriteString("null")
		return nil
	}
	// L'ordine d'iterazione di Go non conta: le chiavi si ordinano per byte.
	chiavi := make([]string, 0, v.Len())
	valori := make(map[string]reflect.Value, v.Len())
	it := v.MapRange()
	for it.Next() {
		k := it.Key().String()
		chiavi = append(chiavi, k)
		valori[k] = it.Value()
	}
	sort.Strings(chiavi)
	b.WriteByte('{')
	for i, k := range chiavi {
		if i > 0 {
			b.WriteByte(',')
		}
		if err := scriviStringa(b, k); err != nil {
			return err
		}
		b.WriteByte(':')
		// Il valore di una mappa non è indirizzabile: una copia lo rende tale, per i metodi con ricevitore
		// puntatore.
		x := reflect.New(valori[k].Type()).Elem()
		x.Set(valori[k])
		if err := codificaValore(b, x, prof+1); err != nil {
			return err
		}
	}
	b.WriteByte('}')
	return nil
}

// campo: un campo esportato di una struct come lo vede il JSON.
type campo struct {
	nome      string
	indice    []int
	omitempty bool
	omitzero  bool
	livello   int
}

func campiDi(t reflect.Type) ([]campo, error) {
	var tutti []campo
	if err := raccogliCampi(t, nil, 0, &tutti); err != nil {
		return nil, err
	}
	// Un nome a un livello più basso vince su uno più profondo (come encoding/json); due nomi uguali allo
	// stesso livello sono un errore, non un campo che sparisce in silenzio.
	scelti := map[string]campo{}
	for _, c := range tutti {
		prima, c2 := scelti[c.nome]
		switch {
		case !c2 || c.livello < prima.livello:
			scelti[c.nome] = c
		case c.livello == prima.livello:
			return nil, fmt.Errorf("jsoncanonico: %s ha due campi con il nome JSON %q", t, c.nome)
		}
	}
	out := make([]campo, 0, len(scelti))
	for _, c := range scelti {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].nome < out[j].nome })
	return out, nil
}

func raccogliCampi(t reflect.Type, prefisso []int, livello int, out *[]campo) error {
	if livello > 16 {
		return fmt.Errorf("jsoncanonico: %s incorporato troppo in profondità", t)
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		nome, opzioni, _ := strings.Cut(tag, ",")
		indice := append(append([]int(nil), prefisso...), i)
		if f.Anonymous && nome == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct && !haMetodi(ft) {
				if f.Type.Kind() == reflect.Pointer {
					return fmt.Errorf("jsoncanonico: %s incorpora il puntatore %s: non ammesso", t, f.Type)
				}
				if err := raccogliCampi(ft, indice, livello+1, out); err != nil {
					return err
				}
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if nome == "" {
			nome = f.Name
		}
		c := campo{nome: nome, indice: indice, livello: livello}
		for _, o := range strings.Split(opzioni, ",") {
			switch o {
			case "":
			case "omitempty":
				c.omitempty = true
			case "omitzero":
				c.omitzero = true
			default:
				return fmt.Errorf("jsoncanonico: %s.%s: opzione del tag json %q non ammessa", t, f.Name, o)
			}
		}
		*out = append(*out, c)
	}
	return nil
}

func codificaStruct(b *bytes.Buffer, v reflect.Value, prof int) error {
	campi, err := campiDi(v.Type())
	if err != nil {
		return err
	}
	b.WriteByte('{')
	primo := true
	for _, c := range campi {
		f := v.FieldByIndex(c.indice)
		if c.omitempty && vuoto(f) {
			continue
		}
		if c.omitzero && zero(f) {
			continue
		}
		if !primo {
			b.WriteByte(',')
		}
		primo = false
		if err := scriviStringa(b, c.nome); err != nil {
			return err
		}
		b.WriteByte(':')
		if err := codificaValore(b, f, prof+1); err != nil {
			return err
		}
	}
	b.WriteByte('}')
	return nil
}

// vuoto: la regola di omitempty di encoding/json (una struct non è mai vuota).
func vuoto(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	}
	return false
}

// zero: la regola di omitzero di encoding/json: il metodo IsZero, se il tipo lo ha, altrimenti il valore zero.
func zero(v reflect.Value) bool {
	if z, ok := v.Interface().(interface{ IsZero() bool }); ok {
		if v.Kind() == reflect.Pointer && v.IsNil() {
			return true
		}
		return z.IsZero()
	}
	return v.IsZero()
}

// scriviStringa: UTF-8 grezzo; escape solo di «"», «\» e dei controlli U+0000–U+001F.
func scriviStringa(b *bytes.Buffer, s string) error {
	if !utf8.ValidString(s) {
		return errors.New("jsoncanonico: stringa non UTF-8")
	}
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			b.WriteString(`\"`)
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\b':
			b.WriteString(`\b`)
		case c == '\t':
			b.WriteString(`\t`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\f':
			b.WriteString(`\f`)
		case c == '\r':
			b.WriteString(`\r`)
		case c < 0x20:
			b.WriteString(`\u00`)
			b.WriteByte("0123456789abcdef"[c>>4])
			b.WriteByte("0123456789abcdef"[c&0xf])
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return nil
}

// ---- rilettura stretta di un JSON già scritto (json.RawMessage, MarshalJSON) ----

// ricanonicalizza rilegge un testo JSON e lo riscrive canonico. La lettura è stretta: UTF-8 non valido,
// surrogati soli, chiavi ripetute, testo dopo il valore sono errori. I numeri restano come sono scritti.
func ricanonicalizza(b *bytes.Buffer, raw []byte, prof int) error {
	if !utf8.Valid(raw) {
		return errors.New("jsoncanonico: JSON non UTF-8")
	}
	l := &lettore{s: raw}
	l.spazi()
	if err := l.valore(b, prof); err != nil {
		return err
	}
	l.spazi()
	if l.i != len(l.s) {
		return fmt.Errorf("jsoncanonico: testo dopo il valore JSON, alla posizione %d", l.i)
	}
	return nil
}

type lettore struct {
	s []byte
	i int
}

func (l *lettore) spazi() {
	for l.i < len(l.s) {
		switch l.s[l.i] {
		case ' ', '\t', '\n', '\r':
			l.i++
		default:
			return
		}
	}
}

func (l *lettore) errore(cosa string) error {
	return fmt.Errorf("jsoncanonico: JSON non valido alla posizione %d: %s", l.i, cosa)
}

func (l *lettore) valore(b *bytes.Buffer, prof int) error {
	if prof > profonditaMassima {
		return l.errore("annidato oltre la profondità massima")
	}
	if l.i >= len(l.s) {
		return l.errore("valore mancante")
	}
	switch c := l.s[l.i]; {
	case c == '{':
		return l.oggetto(b, prof)
	case c == '[':
		return l.elenco(b, prof)
	case c == '"':
		s, err := l.stringa()
		if err != nil {
			return err
		}
		return scriviStringa(b, s)
	case c == 't':
		return l.letterale(b, "true")
	case c == 'f':
		return l.letterale(b, "false")
	case c == 'n':
		return l.letterale(b, "null")
	case c == '-' || (c >= '0' && c <= '9'):
		inizio := l.i
		l.i++
		for l.i < len(l.s) && strings.IndexByte("0123456789+-.eE", l.s[l.i]) >= 0 {
			l.i++
		}
		n := string(l.s[inizio:l.i])
		if !numeroValido(n) {
			l.i = inizio
			return l.errore("numero non valido")
		}
		b.WriteString(n)
		return nil
	}
	return l.errore("carattere inatteso")
}

func (l *lettore) letterale(b *bytes.Buffer, lett string) error {
	if !bytes.HasPrefix(l.s[l.i:], []byte(lett)) {
		return l.errore("letterale non valido")
	}
	l.i += len(lett)
	b.WriteString(lett)
	return nil
}

func (l *lettore) oggetto(b *bytes.Buffer, prof int) error {
	l.i++ // '{'
	type voce struct {
		chiave string
		valore []byte
	}
	var voci []voce
	viste := map[string]bool{}
	l.spazi()
	if l.i < len(l.s) && l.s[l.i] == '}' {
		l.i++
		b.WriteString("{}")
		return nil
	}
	for {
		l.spazi()
		if l.i >= len(l.s) || l.s[l.i] != '"' {
			return l.errore("attesa una chiave")
		}
		k, err := l.stringa()
		if err != nil {
			return err
		}
		if viste[k] {
			return l.errore(fmt.Sprintf("chiave ripetuta %q", k))
		}
		viste[k] = true
		l.spazi()
		if l.i >= len(l.s) || l.s[l.i] != ':' {
			return l.errore("attesi i due punti")
		}
		l.i++
		l.spazi()
		var vb bytes.Buffer
		if err := l.valore(&vb, prof+1); err != nil {
			return err
		}
		voci = append(voci, voce{k, vb.Bytes()})
		l.spazi()
		if l.i >= len(l.s) {
			return l.errore("oggetto non chiuso")
		}
		if l.s[l.i] == ',' {
			l.i++
			continue
		}
		if l.s[l.i] == '}' {
			l.i++
			break
		}
		return l.errore("attesa una virgola o la chiusura dell'oggetto")
	}
	sort.Slice(voci, func(i, j int) bool { return voci[i].chiave < voci[j].chiave })
	b.WriteByte('{')
	for i, v := range voci {
		if i > 0 {
			b.WriteByte(',')
		}
		if err := scriviStringa(b, v.chiave); err != nil {
			return err
		}
		b.WriteByte(':')
		b.Write(v.valore)
	}
	b.WriteByte('}')
	return nil
}

func (l *lettore) elenco(b *bytes.Buffer, prof int) error {
	l.i++ // '['
	l.spazi()
	b.WriteByte('[')
	if l.i < len(l.s) && l.s[l.i] == ']' {
		l.i++
		b.WriteByte(']')
		return nil
	}
	for primo := true; ; primo = false {
		if !primo {
			b.WriteByte(',')
		}
		l.spazi()
		if err := l.valore(b, prof+1); err != nil {
			return err
		}
		l.spazi()
		if l.i >= len(l.s) {
			return l.errore("elenco non chiuso")
		}
		if l.s[l.i] == ',' {
			l.i++
			continue
		}
		if l.s[l.i] == ']' {
			l.i++
			b.WriteByte(']')
			return nil
		}
		return l.errore("attesa una virgola o la chiusura dell'elenco")
	}
}

// stringa legge una stringa JSON e ne restituisce il valore. Un surrogato solo (\ud800) è un errore: non
// diventa U+FFFD.
func (l *lettore) stringa() (string, error) {
	l.i++ // '"'
	var sb strings.Builder
	for {
		if l.i >= len(l.s) {
			return "", l.errore("stringa non chiusa")
		}
		c := l.s[l.i]
		switch {
		case c == '"':
			l.i++
			return sb.String(), nil
		case c < 0x20:
			return "", l.errore("carattere di controllo non in escape dentro una stringa")
		case c == '\\':
			l.i++
			if l.i >= len(l.s) {
				return "", l.errore("escape non chiuso")
			}
			e := l.s[l.i]
			l.i++
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
				r, err := l.esadecimale()
				if err != nil {
					return "", err
				}
				if utf16.IsSurrogate(r) {
					if r >= 0xdc00 || !bytes.HasPrefix(l.s[l.i:], []byte(`\u`)) {
						return "", l.errore("surrogato solo")
					}
					l.i += 2
					r2, err := l.esadecimale()
					if err != nil {
						return "", err
					}
					r = utf16.DecodeRune(r, r2)
					if r == utf8.RuneError {
						return "", l.errore("coppia di surrogati non valida")
					}
				}
				sb.WriteRune(r)
			default:
				return "", l.errore("escape sconosciuto")
			}
		default:
			r, n := utf8.DecodeRune(l.s[l.i:])
			if r == utf8.RuneError && n <= 1 {
				return "", l.errore("UTF-8 non valido")
			}
			sb.Write(l.s[l.i : l.i+n])
			l.i += n
		}
	}
}

func (l *lettore) esadecimale() (rune, error) {
	if l.i+4 > len(l.s) {
		return 0, l.errore("escape \\u troncato")
	}
	n, err := strconv.ParseUint(string(l.s[l.i:l.i+4]), 16, 32)
	if err != nil {
		return 0, l.errore("escape \\u non esadecimale")
	}
	l.i += 4
	return rune(n), nil
}

// numeroValido: la grammatica dei numeri di RFC 8259 (niente zeri in testa, niente «+», niente punto solo).
func numeroValido(s string) bool {
	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	switch {
	case i < len(s) && s[i] == '0':
		i++
	case i < len(s) && s[i] >= '1' && s[i] <= '9':
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	default:
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		inizio := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == inizio {
			return false
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		inizio := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == inizio {
			return false
		}
	}
	return i == len(s)
}
