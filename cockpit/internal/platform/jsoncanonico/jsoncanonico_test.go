package jsoncanonico

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// L1 — il JSON canonico e le impronte (A1a-CAN; P1 §7.1, v3 §4.2, piano A par.3.4.3): stessi byte per lo
// stesso valore, qualunque sia l'ordine delle chiavi, gli spazi o l'escape con cui è arrivato; errore, mai
// un ripiego, per ciò che non avrebbe una forma unica (float, UTF-8 non valido, chiavi ripetute).
//
// I valori di questi test sono inventati: il repository è pubblico, e un encoder non ha bisogno di dati
// veri per dimostrare che scrive sempre gli stessi byte.

// TestA1aCANVersioneFissa: la versione entra in ogni impronta. Cambiarla cambia tutti gli hash: si fa solo
// con un commit che lo dichiara, e questa prova va riscritta con il titolo «Riscritta per …» (par.3.4.1).
func TestA1aCANVersioneFissa(t *testing.T) {
	if Versione != "canonico-1" {
		t.Fatalf("Versione = %q, attesa canonico-1", Versione)
	}
}

type golden struct {
	Zeta  map[string]any  `json:"m"`
	Alfa  string          `json:"a"`
	Lista []any           `json:"l"`
	Num   json.RawMessage `json:"n"`
	ID    uuid.UUID       `json:"u"`
	Vuoto *int            `json:"o,omitempty"`
	Mai   string          `json:"-"`
}

// TestA1aCANGoldenInLinea: i byte e lo sha256 di un valore fisso. Chiavi in ordine di byte (anche dentro la
// mappa: «B» < «a» < «à»), nessuno spazio, «è» e U+2028 grezzi, il controllo U+001F in escape, il numero
// del RawMessage come testo, l'UUID dal suo MarshalText, omitempty e «-» rispettati.
func TestA1aCANGoldenInLinea(t *testing.T) {
	v := golden{
		Zeta:  map[string]any{"a": -3, "à": "z", "B": 2},
		Alfa:  "è\u2028x",
		Lista: []any{1, true, nil, "\x1f"},
		Num:   json.RawMessage(" 1.50 "),
		ID:    uuid.MustParse("00000000-0000-0000-0000-00000000000a"),
		Mai:   "non compare",
	}
	attesi := `{"a":"è` + "\u2028" + `x","l":[1,true,null,"\u001f"],"m":{"B":2,"a":-3,"à":"z"},"n":1.50,"u":"00000000-0000-0000-0000-00000000000a"}`
	b, err := Codifica(v)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != attesi {
		t.Fatalf("byte diversi:\n got %s\nwant %s", b, attesi)
	}
	const sha = "67441b5413349b357ba9f2eb505cc11910b0d26065f615e158b178f5ea38f697"
	if got := Impronta(b); got != sha {
		t.Fatalf("Impronta = %s, attesa %s", got, sha)
	}
	if got, err := ImprontaDi(v); err != nil || got != sha {
		t.Fatalf("ImprontaDi = %s, %v; attesa %s", got, err, sha)
	}
}

func TestA1aCANImprontaVuota(t *testing.T) {
	// sha256 della sequenza vuota: 64 caratteri esadecimali minuscoli.
	if got := Impronta(nil); got != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("Impronta(nil) = %s", got)
	}
}

// TestA1aCANStessiByteDaScrittureDiverse: chiavi permutate, spazi, «\u00e8» contro «è», «\/» contro «/»
// danno gli stessi byte.
func TestA1aCANStessiByteDaScrittureDiverse(t *testing.T) {
	forme := []string{
		`{"b":[1,{"y":2,"x":"è"}],"a":"a/b"}`,
		"{ \"a\" : \"a\\/b\" ,\n\t\"b\" : [ 1 , { \"x\" : \"\\u00e8\", \"y\" : 2 } ] }",
		`{"a":"a/b","b":[1,{"x":"\u00E8","y":2}]}`,
	}
	const attesi = `{"a":"a/b","b":[1,{"x":"è","y":2}]}`
	for _, f := range forme {
		b, err := Codifica(json.RawMessage(f))
		if err != nil {
			t.Fatalf("%q: %v", f, err)
		}
		if string(b) != attesi {
			t.Errorf("%q → %s, atteso %s", f, b, attesi)
		}
	}
}

// TestA1aCANSeparatoriDiRigaGrezzi: U+2028 e U+2029 restano grezzi (encoding/json li metterebbe in escape),
// anche quando arrivano in escape da un RawMessage.
func TestA1aCANSeparatoriDiRigaGrezzi(t *testing.T) {
	attesi := "\"a\u2028b\u2029c\""
	if b, _ := Codifica("a\u2028b\u2029c"); string(b) != attesi {
		t.Errorf("stringa: %q, atteso %q", b, attesi)
	}
	if b, _ := Codifica(json.RawMessage(`"a\u2028b\u2029c"`)); string(b) != attesi {
		t.Errorf("RawMessage: %q, atteso %q", b, attesi)
	}
}

// TestA1aCANControlliInEscape: in escape solo «"», «\» e U+0000–U+001F (\b \t \n \f \r per nome, gli altri
// come \u00xx minuscolo); «/» e U+007F restano grezzi.
func TestA1aCANControlliInEscape(t *testing.T) {
	b, err := Codifica("\x00\x01\b\t\n\f\r\x1f\"\\/\x7f")
	if err != nil {
		t.Fatal(err)
	}
	attesi := `"\u0000\u0001\b\t\n\f\r\u001f\"\\/` + "\x7f" + `"`
	if string(b) != attesi {
		t.Fatalf("%q, atteso %q", b, attesi)
	}
}

// TestA1aCANErrori: ciò che non ha una forma canonica unica è un errore, mai un arrotondamento o un U+FFFD.
func TestA1aCANErrori(t *testing.T) {
	casi := []struct {
		nome   string
		valore any
	}{
		{"float64", 1.5},
		{"float32 in una struct", struct {
			X float32 `json:"x"`
		}{1}},
		{"float in un elenco", []any{1, 2.0}},
		{"stringa non UTF-8", "a\xffb"},
		{"chiave non UTF-8", map[string]int{"\xff": 1}},
		{"mappa con chiave intera", map[int]string{1: "a"}},
		{"RawMessage non UTF-8", json.RawMessage("\"a\xffb\"")},
		{"RawMessage con un surrogato solo", json.RawMessage(`"\ud800"`)},
		{"RawMessage con un surrogato basso solo", json.RawMessage(`"\udc00x"`)},
		{"RawMessage con una chiave ripetuta", json.RawMessage(`{"a":1,"a":2}`)},
		{"RawMessage con testo dopo il valore", json.RawMessage(`{"a":1} x`)},
		{"RawMessage con un numero non JSON", json.RawMessage(`01`)},
		{"RawMessage con un letterale sbagliato", json.RawMessage(`tru`)},
		{"json.Number non JSON", json.Number("1.")},
		{"json.Number con il più", json.Number("+1")},
		{"canale", make(chan int)},
		{"tag con l'opzione string", struct {
			X int `json:"x,string"`
		}{1}},
	}
	for _, c := range casi {
		if b, err := Codifica(c.valore); err == nil {
			t.Errorf("%s: atteso un errore, ottenuto %q", c.nome, b)
		}
	}
}

// TestA1aCANMappeOrdinate: l'ordine d'iterazione delle mappe di Go non conta: le chiavi sono in ordine di
// byte, e venti codifiche danno gli stessi byte.
func TestA1aCANMappeOrdinate(t *testing.T) {
	m := map[string]int{}
	for _, k := range strings.Fields("zeta alfa Beta beta 10 9 _ à a b c d e f g h") {
		m[k] = len(k)
	}
	primo, err := Codifica(m)
	if err != nil {
		t.Fatal(err)
	}
	const attesi = `{"10":2,"9":1,"Beta":4,"_":1,"a":1,"alfa":4,"b":1,"beta":4,"c":1,"d":1,"e":1,"f":1,"g":1,"h":1,"zeta":4,"à":2}`
	if string(primo) != attesi {
		t.Fatalf("%s\natteso %s", primo, attesi)
	}
	for i := 0; i < 20; i++ {
		b, _ := Codifica(m)
		if string(b) != string(primo) {
			t.Fatalf("codifica %d diversa: %s", i, b)
		}
	}
}

// TestA1aCANNumeriComeTesto: un RawMessage si ricanonicalizza conservando il testo dei numeri (niente
// 1.50 → 1.5, niente 1E3 → 1000); un json.Number si copia com'è.
func TestA1aCANNumeriComeTesto(t *testing.T) {
	b, err := Codifica(json.RawMessage(`{"n":1.50,"e":1E3,"z":-0,"g":[0.000,12e-2]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"e":1E3,"g":[0.000,12e-2],"n":1.50,"z":-0}`; got != want {
		t.Fatalf("%s, atteso %s", got, want)
	}
	b, err = Codifica(map[string]json.Number{"x": "12.0"})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != `{"x":12.0}` {
		t.Fatalf("json.Number: %s", got)
	}
	b, _ = Codifica(map[string]json.RawMessage{"x": nil})
	if got := string(b); got != `{"x":null}` {
		t.Fatalf("RawMessage nil: %s", got)
	}
}

// TestA1aCANMetodiDelTipo: uuid.UUID passa dal suo MarshalText, time.Time dal suo MarshalJSON, e il
// risultato si ricanonicalizza. Un puntatore nil è null, e così un campo interfaccia (json.Marshaler)
// senza valore; omitzero toglie il tempo zero, omitempty no.
func TestA1aCANMetodiDelTipo(t *testing.T) {
	id := uuid.MustParse("7f3c2a10-0000-4000-8000-0000000000ac")
	quando := time.Date(2026, 10, 4, 9, 30, 0, 123000000, time.UTC)
	v := struct {
		ID      uuid.UUID      `json:"id"`
		Nessuno *uuid.UUID     `json:"nessuno"`
		Vuoto   json.Marshaler `json:"vuoto"`
		Quando  time.Time      `json:"quando"`
		Zero    time.Time      `json:"zero,omitzero"`
		ZeroE   time.Time      `json:"zero_e,omitempty"`
	}{ID: id, Quando: quando}
	b, err := Codifica(v)
	if err != nil {
		t.Fatal(err)
	}
	const attesi = `{"id":"7f3c2a10-0000-4000-8000-0000000000ac","nessuno":null,"quando":"2026-10-04T09:30:00.123Z","vuoto":null,"zero_e":"0001-01-01T00:00:00Z"}`
	if string(b) != attesi {
		t.Fatalf("%s\natteso %s", b, attesi)
	}
}

// TestA1aCANElenchiInOrdine: l'ordine degli array è quello dato (ordinare tocca a chi chiama); nil e vuoto
// restano distinti come in encoding/json, e omitempty li toglie tutti e due.
func TestA1aCANElenchiInOrdine(t *testing.T) {
	type e struct {
		A []int `json:"a"`
		B []int `json:"b,omitempty"`
		C [2]string
	}
	b, err := Codifica(e{A: []int{3, 1, 2}, B: []int{}, C: [2]string{"y", "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"C":["y","x"],"a":[3,1,2]}`; got != want {
		t.Fatalf("%s, atteso %s", got, want)
	}
	b, _ = Codifica(e{})
	if got, want := string(b), `{"C":["",""],"a":null}`; got != want {
		t.Fatalf("%s, atteso %s", got, want)
	}
}

// TestA1aCANRiferimentoCircolare: un valore che punta a sé stesso dà un errore, non consuma lo stack.
func TestA1aCANRiferimentoCircolare(t *testing.T) {
	type nodo struct {
		Dopo *nodo `json:"dopo"`
	}
	n := &nodo{}
	n.Dopo = n
	if _, err := Codifica(n); err == nil {
		t.Fatal("atteso un errore per il riferimento circolare")
	}
}
