// L1 — le riscritture in linea (piano B, I1–I5) e l'oggetto (S1).
package lettura

import (
	"net/url"
	"reflect"
	"testing"
)

// datiAzzerati è il parametro data di Safe Links, con ogni identificativo a zero.
const datiAzzerati = "&data=05%7C02%7C%7C00000000000000000000000000000000%7C00000000000000000000000000000000%7C0%7C0%7C000000000000000000%7CUnknown%7C0%7C%7C%7C&sdata=0000000000000000000000000000000000000000000%3D&reserved=0"

// safeLinks avvolge un indirizzo (già codificato per stare in una query) nell'involucro Safe Links.
func safeLinks(codificato string) string {
	return "https://eur01.safelinks.protection.outlook.com/?url=" + codificato + datiAzzerati
}

// pezziDi è il blocco unico di testo di un corpo di una riga.
func pezziDi(t *testing.T, corpo string) []Pezzo {
	t.Helper()
	c := verifica(t, corpo, "")
	soloBlocchi(t, c.Blocchi, BloccoTesto)
	return c.Blocchi[0].Pezzi
}

func testo(s string) Pezzo { return Pezzo{Tipo: PezzoTesto, Testo: s} }

// ---------------------------------------------------------------- I1

func TestSrotolaSafeLinks(t *testing.T) {
	semplice := safeLinks(url.QueryEscape("https://www.acme.example/ordini?id=1"))
	doppio := safeLinks(url.QueryEscape(semplice))
	ap := "https://nam12.safelinks.protection.outlook.com/ap/b-59584e83/?url=" + url.QueryEscape("https://fornitore.example/doc") + datiAzzerati
	for nome, caso := range map[string]struct {
		in, out string
		ok      bool
	}{
		"singolo":    {semplice, "https://www.acme.example/ordini?id=1", true},
		"doppio":     {doppio, "https://www.acme.example/ordini?id=1", true},
		"/ap/":       {ap, "https://fornitore.example/doc", true},
		"mailto":     {safeLinks(url.QueryEscape("mailto:acquisti@acme.example")), "mailto:acquisti@acme.example", true},
		"javascript": {safeLinks(url.QueryEscape("javascript:alert(1)")), "", false},
		"senza url":  {"https://eur01.safelinks.protection.outlook.com/?data=0", "", false},
		"non Safe":   {"https://www.acme.example/?url=https%3A%2F%2Fx.example", "", false},
		"spazzatura": {"%%%", "", false},
	} {
		t.Run(nome, func(t *testing.T) {
			got, ok := SrotolaSafeLinks(caso.in)
			if ok != caso.ok || (ok && got != caso.out) || (!ok && got != caso.in) {
				t.Errorf("SrotolaSafeLinks = %q, %v; atteso %q, %v", got, ok, caso.out, caso.ok)
			}
		})
	}
}

func TestI1SafeLinksNelTesto(t *testing.T) {
	u := safeLinks(url.QueryEscape("https://www.acme.example/ordini?id=1"))
	got := pezziDi(t, "Vedi "+u+". Grazie")
	want := []Pezzo{
		testo("Vedi "),
		{Tipo: PezzoLink, Testo: "https://www.acme.example/ordini?id=1", Nota: notaSafeLinks},
		testo(". Grazie"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pezzi:\n%+v\nattesi:\n%+v", got, want)
	}
}

func TestI1JavascriptRestaComEra(t *testing.T) {
	u := safeLinks(url.QueryEscape("javascript:alert(1)"))
	got := pezziDi(t, "Clicca "+u)
	if !reflect.DeepEqual(got, []Pezzo{testo("Clicca " + u)}) {
		t.Errorf("il link javascript è stato toccato: %+v", got)
	}
}

// ---------------------------------------------------------------- I2

func TestI2Annotazioni(t *testing.T) {
	for nome, caso := range map[string]struct {
		in   string
		want []Pezzo
	}{
		"doppione": {"Scrivete a acquisti@acme.example<mailto:acquisti@acme.example> entro venerdì",
			[]Pezzo{testo("Scrivete a acquisti@acme.example entro venerdì")}},
		"maiuscole": {"Acquisti@ACME.example<mailto:acquisti@acme.example>",
			[]Pezzo{testo("Acquisti@ACME.example")}},
		"subject": {"info@acme.example<mailto:info@acme.example?subject=RFQ%20PZ-001>",
			[]Pezzo{testo("info@acme.example")}},
		"sito": {"www.acme.example<http://www.acme.example/>",
			[]Pezzo{testo("www.acme.example")}},
		"nome": {"Ufficio Acquisti<mailto:acquisti@acme.example>",
			[]Pezzo{testo("Ufficio Acquisti "), {Tipo: PezzoLink, Testo: "<acquisti@acme.example>"}}},
		"tel doppione": {"Tel. +39 000 000 0000<tel:+390000000000>",
			[]Pezzo{testo("Tel. +39 000 000 0000")}},
		"tel diverso": {"Chiamateci<tel:+390000000000>",
			[]Pezzo{testo("Chiamateci "), {Tipo: PezzoLink, Testo: "<tel:+390000000000>"}}},
		"ancora testo": {"il catalogo<https://www.acme.example/catalogo> aggiornato",
			[]Pezzo{testo("il catalogo "), {Tipo: PezzoLink, Testo: "<https://www.acme.example/catalogo>"}, testo(" aggiornato")}},
		"safe links": {"il portale<" + safeLinks(url.QueryEscape("https://fornitore.example/portale")) + ">",
			[]Pezzo{testo("il portale "), {Tipo: PezzoLink, Testo: "<https://fornitore.example/portale>", Nota: notaSafeLinks}}},
	} {
		t.Run(nome, func(t *testing.T) {
			if got := pezziDi(t, caso.in); !reflect.DeepEqual(got, caso.want) {
				t.Errorf("pezzi:\n%+v\nattesi:\n%+v", got, caso.want)
			}
		})
	}
}

// ---------------------------------------------------------------- I3–I5

func TestI3I5Immagini(t *testing.T) {
	got := pezziDi(t, "Logo [cid:image001.png@01DA0000.00000000] e [Immagine che contiene testo, logo. Descrizione generata automaticamente] e [https://www.acme.example/logo.png] per [PZ-001]")
	want := []Pezzo{
		testo("Logo "),
		{Tipo: PezzoImmagine, Testo: "[immagine]", Nota: "image001.png"},
		testo(" e "),
		{Tipo: PezzoImmagine, Testo: "[immagine]", Nota: "Immagine che contiene testo, logo. Descrizione generata automaticamente"},
		testo(" e "),
		{Tipo: PezzoImmagine, Testo: "[immagine]", Nota: "https://www.acme.example/logo.png"},
		testo(" per [PZ-001]"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pezzi:\n%+v\nattesi:\n%+v", got, want)
	}
}

// ---------------------------------------------------------------- S1

func TestOggettoVisibile(t *testing.T) {
	for in, want := range map[string]struct {
		out     string
		esterno bool
	}{
		"[EXT] RE: RFQ PZ-001":         {"RE: RFQ PZ-001", true},
		"RE: [EXTERNAL] Offerta":       {"RE: Offerta", true},
		"R: I: [Esterno]  Disegno 7":   {"R: I: Disegno 7", true},
		"RE: [EXT] RE: [EXT] Offerta":  {"RE: RE: Offerta", true},
		"[E-mail esterna] Conferma":    {"Conferma", true},
		"Offerta [EXT-2]":              {"Offerta [EXT-2]", false},
		"RFQ PZ-001 [EXTERNAL] review": {"RFQ PZ-001 [EXTERNAL] review", false},
		"":                             {"", false},
	} {
		got, esterno := OggettoVisibile(in)
		if got != want.out || esterno != want.esterno {
			t.Errorf("OggettoVisibile(%q) = %q, %v; atteso %q, %v", in, got, esterno, want.out, want.esterno)
		}
	}
}
