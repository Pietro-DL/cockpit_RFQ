package web

// L1 — messaggio 46 dell'utente (21/10) [U], corsia MOTIVO: «la conferma con motivo non mi piace, anzi dobbiamo
// rimuoverlo da ogni widget in cui appare». Il codice della pagina della Distinta unica non ha piu' un campo del motivo in
// nessun dialogo (né obbligatorio né facoltativo), né la scelta del «Motivo (codice)», né un pulsante spento con «scrivi il
// motivo», né i testi che chiedono o promettono un motivo; le decisioni non_includere portano la traccia del gesto scritta
// dalla pagina (motivo_codice dal segnale visto [T]). Fallisce su 342010f. La prova nel browser e' DU-M46
// (distinta_unica_motivo46_browser_test.go).
//
// Revisione della corsia (rilievi 2 e 3): il motivo registrato non si MOSTRA in nessun widget (pannello del nodo, wizard,
// dettaglio dei file scartati o in Altri file, categoria), e il dialogo della conferma della sorgente dice quando il codice
// letto dal file non e' quello del prodotto (il documento prende quello del prodotto). Fallisce su 342010f.

import (
	"regexp"
	"strings"
	"testing"
)

func TestDistintaUnicaM46NessunMotivoNeiDialoghi(t *testing.T) {
	src := duFile(t, duPagina)
	vietati := []struct{ re, cosa string }{
		{`area\("f-motivo`, "un campo del motivo (textarea f-motivo…)"},
		{`campo\("Motivo`, "un campo con l'etichetta «Motivo»"},
		{`id: "f-motivo-codice"`, "la scelta del «Motivo (codice)»"},
		{`"scrivi il motivo"`, "un pulsante spento con «scrivi il motivo»"},
		{`serve un motivo`, "un testo che chiede un motivo"},
		{`Scrivi il motivo`, "un testo che chiede un motivo"},
		{`con il motivo…`, "un pulsante che apre un dialogo «con il motivo»"},
		{`rifiuto motivato`, "un rifiuto «motivato»"},
		{`decisione motivata`, "una decisione «motivata»"},
		{`il motivo vale anche per questo`, "il motivo della ripartenza"},
		{`fissalo con un motivo`, "il motivo dell'emittente"},
		{`dichiaralo con il motivo`, "il motivo della dichiarazione mono_pezzo"},
	}
	for _, v := range vietati {
		if n := len(regexp.MustCompile(v.re).FindAllStringIndex(src, -1)); n > 0 {
			t.Errorf("la pagina ha ancora %s: %d volte (%s)", v.cosa, n, v.re)
		}
	}
	// i corpi dei gesti non portano un motivo scritto dalla pagina per conto dell'utente
	for _, f := range []string{"async function apriRevocaSorgente(", "async function apriConfermaSorgente(", "function apriRifiutaProposta(", "async function apriScarta(",
		"async function apriAltriFile(", "function apriEscludi(", "function apriCorreggi(", "function apriMonoPezzo(", "function apriRebase(", "function apriConverti(",
		"function apriAbbandona(", "function apriAdotta(", "function apriFissaEmittente(", "function apriConfermaIdentita(", "function apriScollega(",
		"function apriRiconosci(", "function apriCategoria("} {
		corpo := duFunzione(t, src, f)
		if strings.Contains(corpo, "f-motivo") {
			t.Errorf("%s legge ancora il campo del motivo", f)
		}
	}
	// non_includere: la traccia del gesto e motivo_codice dal segnale visto, mai una scelta dell'utente
	ni := duFunzione(t, src, "function apriNonIncludere(")
	if !strings.Contains(ni, `motivo_codice: motivoCodiceDelGesto(seg)`) || !strings.Contains(ni, `tracciaDelGesto("Non includere nella BOM"`) {
		t.Error("la decisione non_includere non porta la traccia del gesto e motivo_codice dal segnale visto")
	}
	mc := duFunzione(t, src, "function motivoCodiceDelGesto(")
	if !strings.Contains(mc, `seg.codice === "ancoraggio.possibile_elemento_cad" ? "elemento_cad" : "altro"`) || !strings.Contains(src, `S.sigla = radice.dataset.duSigla || "";`) {
		t.Error("motivo_codice non viene dal segnale visto, o manca la sigla per la traccia del gesto")
	}
}

// Revisione della corsia, rilievo 2: il motivo registrato (il testo di prima o la traccia del gesto) non si mostra.
func TestDistintaUnicaM46NessunMotivoMostrato(t *testing.T) {
	src := duFile(t, duPagina)
	vietati := []struct{ re, cosa string }{
		{`riga\("Motivo"`, "una riga «Motivo» (pannello del nodo o wizard)"},
		{`h\("th", \{\}, "Motivo"\)`, "una colonna «Motivo» (wizard)"},
		{`senza motivo scritto`, "un testo sul motivo che manca"},
		{`ev\.motivo \|\| ""`, "il motivo dello scarto nel dettaglio del file"},
		{`af\.motivo \? ": " \+ af\.motivo`, "il motivo di Altri file nel dettaglio del file"},
		{`dec\.motivo \?`, "il motivo della decisione nel pannello"},
		{`d\.motivo \? ": " \+ d\.motivo`, "il motivo della categoria"},
		{`messaggio 46\. Aggiorna`, "il gergo di progetto nel testo per l'utente"},
	}
	for _, v := range vietati {
		if n := len(regexp.MustCompile(v.re).FindAllStringIndex(src, -1)); n > 0 {
			t.Errorf("la pagina mostra ancora %s: %d volte (%s)", v.cosa, n, v.re)
		}
	}
	for _, f := range []string{"function tabellaNonInclusi(", "function sezioneDecisione("} {
		if !strings.Contains(duFunzione(t, src, f), "decisoDa(") {
			t.Errorf("%s non dice chi ha deciso e quando (decisoDa)", f)
		}
	}
}

// Revisione della corsia, rilievo 3: il dialogo della conferma della sorgente dice il codice letto dal file diverso.
func TestDistintaUnicaM46IlCodiceLettoNelDialogo(t *testing.T) {
	c := duFunzione(t, duFile(t, duPagina), "async function apriConfermaSorgente(")
	for _, s := range []string{`id: "f-codice-letto"`, `!f.documento`, `toUpperCase()`, "il documento prende il codice del prodotto"} {
		if !strings.Contains(c, s) {
			t.Errorf("il dialogo della conferma della sorgente non dice il codice letto dal file diverso: manca %q", s)
		}
	}
}

// la sigla dell'utente arriva alla pagina (data-du-sigla) per la traccia del gesto
func TestDistintaUnicaM46LaSiglaNelGuscio(t *testing.T) {
	g := duFile(t, "web/templates/distinta_unica.html")
	if !strings.Contains(g, `data-du-sigla="{{$d.Sigla}}"`) {
		t.Error("il guscio non porta la sigla dell'utente in data-du-sigla")
	}
}
