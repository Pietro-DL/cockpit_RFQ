package web

// L1 — giro 5, A1d: i controlli statici dei tre file della pagina (QFE-05; piano A1d frontend §9.4; RAC-18, RAC-21,
// RAC-22; MOTORE-SENZA-LLM). Il template si analizza con le funzioni vere e ha i define con il prefisso ma_ (salvo
// contenuto e motore_a_corpo); motore_a.mjs fa solo presentazione: nessuna API che inserisca HTML, una sola fetch (la GET
// del PDF in scaricaPdf, senza metodo e senza intestazioni htmx), un solo import dinamico (pdf.js), Gesti.aggiorna solo
// nel ramo «aggiorna» di azione, ogni indirizzo di una rotta del thread dentro Gesti.esegui o Gesti.leggi e nella lista
// chiusa del ponte (RAC-13); nessun indirizzo esterno, carattere o @import; il CSS vale solo dentro la pagina; nessun
// resto del vecchio impianto; nessun nome della tabella dell'agente. I file si leggono dall'embed (risorse.FS): è quello
// che il server serve.
//
// Sta a L1: nessun database, nessun server, nessun Node (node --check è un comando a mano, nel kit).

import (
	"html/template"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	risorse "promatec/cockpit"
)

// maSFile legge un file della pagina dall'embed.
func maSFile(t *testing.T, nome string) string {
	t.Helper()
	b, err := fs.ReadFile(risorse.FS, nome)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const (
	maSTemplate = "web/templates/motore_a.html"
	maSCSS      = "web/static/motore_a.css"
	maSJS       = "web/static/motore_a.mjs"
)

// TestMotoreATemplateSiAnalizza: layout, frammenti e motore_a.html si analizzano insieme con le funzioni vere (e uno
// statico finto), senza eseguirli; i define di motore_a.html hanno il prefisso ma_, salvo contenuto e motore_a_corpo;
// ogni {{template "x"}} del file porta a un define che c'è.
func TestMotoreATemplateSiAnalizza(t *testing.T) {
	templ, _ := fs.Sub(risorse.FS, "web/templates")
	tt, err := template.New("layout").Funcs(funzioni).Funcs(template.FuncMap{"statico": func(n string) string { return "/static/" + n }}).
		ParseFS(templ, "layout.html", "frammenti.html", "motore_a.html")
	if err != nil {
		t.Fatalf("i template non si analizzano: %v", err)
	}
	src := maSFile(t, maSTemplate)
	definiti := regexp.MustCompile(`\{\{define "([^"]+)"\}\}`).FindAllStringSubmatch(src, -1)
	if len(definiti) == 0 {
		t.Fatal("nessun define in motore_a.html")
	}
	for _, m := range definiti {
		if n := m[1]; n != "contenuto" && n != "motore_a_corpo" && !strings.HasPrefix(n, "ma_") {
			t.Errorf("define %q senza il prefisso ma_: si scontrerebbe con i nomi delle altre pagine", n)
		}
	}
	for _, m := range regexp.MustCompile(`\{\{-?\s*template "([^"]+)"`).FindAllStringSubmatch(src, -1) {
		if tt.Lookup(m[1]) == nil {
			t.Errorf("{{template %q}} senza il suo define", m[1])
		}
	}
	t.Logf("%d define in motore_a.html", len(definiti))
}

// maSCorpoFunzione: il testo di una funzione di primo livello del modulo, dalla sua riga «function nome(» alla riga «}»
// che la chiude (le funzioni del modulo cominciano e finiscono a colonna zero).
func maSCorpoFunzione(t *testing.T, js, nome string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^(async )?function ` + regexp.QuoteMeta(nome) + `\(`)
	loc := re.FindStringIndex(js)
	if loc == nil {
		t.Fatalf("motore_a.mjs non ha la funzione %s", nome)
	}
	resto := js[loc[0]:]
	fine := strings.Index(resto, "\n}\n")
	if fine < 0 {
		t.Fatalf("la funzione %s non si chiude a colonna zero", nome)
	}
	return resto[:fine+3]
}

// TestMotoreAJSSoloPresentazione: motore_a.mjs non inserisce HTML e non scrive da sé (piano A1d frontend §7.1, §9.4).
func TestMotoreAJSSoloPresentazione(t *testing.T) {
	js := maSFile(t, maSJS)
	for _, v := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "new Function", "iframe"} {
		if strings.Contains(js, v) {
			t.Errorf("motore_a.mjs contiene %q", v)
		}
	}
	// una sola fetch: la GET del PDF dentro scaricaPdf, senza metodo e senza intestazioni htmx
	if n := strings.Count(js, "fetch("); n != 1 {
		t.Errorf("%d fetch( in motore_a.mjs, attesa una (la GET del PDF)", n)
	}
	if corpo := maSCorpoFunzione(t, js, "scaricaPdf"); !strings.Contains(corpo, "fetch(") || strings.Contains(corpo, "method:") ||
		strings.Contains(corpo, "HX-") {
		t.Error("la fetch non sta in scaricaPdf, o ha un metodo o un'intestazione htmx")
	}
	if strings.Contains(js, "HX-") || strings.Contains(js, "method:") {
		t.Error("motore_a.mjs scrive un'intestazione htmx o un metodo: le richieste che scrivono sono del ponte")
	}
	// revisione P3, S-01: nessun'altra strada verso il server (htmx.ajax, sendBeacon, XHR, socket, form.submit) e nessun
	// verbo che scrive scritto fra virgolette
	for _, v := range []*regexp.Regexp{
		regexp.MustCompile(`\bhtmx\s*\.`), regexp.MustCompile(`sendBeacon`), regexp.MustCompile(`XMLHttpRequest`),
		regexp.MustCompile(`WebSocket`), regexp.MustCompile(`EventSource`), regexp.MustCompile(`\.submit\s*\(`),
		regexp.MustCompile(`requestSubmit`), regexp.MustCompile("(?i)[\"'`](post|put|patch|delete)[\"'`]"),
	} {
		if m := v.FindString(js); m != "" {
			t.Errorf("motore_a.mjs contiene %q: le richieste che scrivono sono solo del ponte (S-01)", m)
		}
	}
	// un solo import dinamico (il caricatore di pdf.js, §7.4) e un solo import statico, il ponte
	if n := strings.Count(js, "import("); n != 1 {
		t.Errorf("%d import( in motore_a.mjs, atteso uno (pdf.js)", n)
	}
	statici := regexp.MustCompile(`(?m)^\s*(import|export)\b`).FindAllString(js, -1)
	if len(statici) != 1 || !strings.Contains(js, `import * as Gesti from "./motore_a_gesti.mjs";`) {
		t.Errorf("import ed export di primo livello: %v; atteso solo il ponte", statici)
	}
	// Gesti.aggiorna solo nel ramo «aggiorna» di azione (X-08: dopo «Conferma l'albero» rilegge il ponte)
	if n := strings.Count(js, "Gesti.aggiorna("); n != 1 {
		t.Errorf("%d Gesti.aggiorna( in motore_a.mjs, atteso uno (MF14)", n)
	} else if corpo := maSCorpoFunzione(t, js, "azione"); !regexp.MustCompile(`case "aggiorna":[^\n]*Gesti\.aggiorna\(`).MatchString(corpo) {
		t.Error("Gesti.aggiorna( fuori dal ramo «aggiorna» di azione")
	}
	// ogni indirizzo di una rotta del thread sta in Gesti.esegui (lista ROTTE) o Gesti.leggi (lista LETTURE)
	tipo := map[string][]string{"esegui": ponteElenco(t, "ROTTE"), "leggi": ponteElenco(t, "LETTURE")}
	chiamata := regexp.MustCompile("Gesti\\.(esegui|leggi)\\(`([^`?]*)")
	// revisione P3, S-01: un indirizzo fra virgolette di ogni tipo; il solo che non passa dal ponte è la GET del PDF
	letterali := regexp.MustCompile("[\"'`]/(thread|proposta|nodo|allegato|fascicolo|componente|documento|deroga)/")
	const pdf = `return "/allegato/" + enc(a) + "/anteprima?v=" + enc(v);`
	visti, pdfVisti := 0, 0
	for _, riga := range strings.Split(js, "\n") {
		if !letterali.MatchString(riga) {
			continue
		}
		if strings.TrimSpace(riga) == pdf {
			pdfVisti++
			continue
		}
		m := chiamata.FindStringSubmatch(riga)
		if m == nil {
			t.Errorf("un indirizzo di una rotta fuori da Gesti.esegui o Gesti.leggi: %s", strings.TrimSpace(riga))
			continue
		}
		visti++
		forma := regexp.MustCompile(`\$\{[^}]*\}`).ReplaceAllStringFunc(m[2], func(x string) string {
			if x == "${t}" {
				return "{id}"
			}
			return "{u}"
		})
		ok := false
		for _, r := range tipo[m[1]] {
			if r == forma {
				ok = true
			}
		}
		if !ok {
			t.Errorf("Gesti.%s(%q): la rotta non è nella lista del ponte", m[1], forma)
		}
	}
	if visti == 0 {
		t.Error("nessuna chiamata al ponte con un indirizzo: la prova non prova niente")
	}
	if pdfVisti != 1 {
		t.Errorf("%d righe con l'indirizzo del PDF in indirizzoPdf, attesa una (S-04)", pdfVisti)
	}
}

// TestMotoreANessunURLEsterno: nessun indirizzo esterno nei tre file; nel CSS nessun url(, @import, @font-face
// (RAC-22: motore_a.mjs usa new URL( per gli indirizzi della stessa origine).
func TestMotoreANessunURLEsterno(t *testing.T) {
	for _, f := range []string{maSTemplate, maSCSS, maSJS} {
		src := maSFile(t, f)
		for _, v := range []string{"http://", "https://"} {
			if strings.Contains(src, v) {
				t.Errorf("%s contiene %q", f, v)
			}
		}
	}
	css := maSFile(t, maSCSS)
	for _, v := range []string{"url(", "@import", "@font-face"} {
		if strings.Contains(css, v) {
			t.Errorf("motore_a.css contiene %q: niente caratteri né immagini da fuori", v)
		}
	}
}

// maSSelettori: i selettori delle regole di un foglio di stile, anche dentro @media, @container e @supports (una lettura
// minima: commenti tolti, le graffe contate, le virgole fuori dalle parentesi).
func maSSelettori(t *testing.T, css string) []string {
	t.Helper()
	css = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(css, "")
	var out []string
	var giro func(s string)
	giro = func(s string) {
		i := 0
		for i < len(s) {
			j := strings.IndexAny(s[i:], "{;}")
			if j < 0 {
				return
			}
			j += i
			prelude := strings.TrimSpace(s[i:j])
			if s[j] != '{' {
				i = j + 1
				continue
			}
			d, k := 0, j
			for ; k < len(s); k++ {
				if s[k] == '{' {
					d++
				} else if s[k] == '}' {
					d--
					if d == 0 {
						break
					}
				}
			}
			if k >= len(s) {
				t.Fatalf("una graffa non chiusa dopo %q", prelude)
			}
			corpo := s[j+1 : k]
			switch {
			case regexp.MustCompile(`^@(media|container|supports)\b`).MatchString(prelude):
				giro(corpo)
			case strings.HasPrefix(prelude, "@"):
				t.Errorf("at-rule %q: il foglio della pagina non ne ha bisogno", prelude)
			default:
				prof, cur := 0, ""
				for _, ch := range prelude {
					switch {
					case ch == '(':
						prof++
					case ch == ')':
						prof--
					}
					if ch == ',' && prof == 0 {
						out = append(out, strings.Join(strings.Fields(cur), " "))
						cur = ""
						continue
					}
					cur += string(ch)
				}
				out = append(out, strings.Join(strings.Fields(cur), " "))
			}
			i = k + 1
		}
	}
	giro(css)
	return out
}

// TestMotoreACSSDentroLaPagina: ogni selettore di motore_a.css comincia con .ma, #ma-avviso o .ma-finestra (le finestre
// stanno in body), anche dentro @media e @container: il foglio non tocca il layout né le altre pagine (MF22).
func TestMotoreACSSDentroLaPagina(t *testing.T) {
	sel := maSSelettori(t, maSFile(t, maSCSS))
	re := regexp.MustCompile(`^(\.ma|#ma-avviso|\.ma-finestra)([\s.:#\[>+~]|$)`)
	for _, s := range sel {
		if !re.MatchString(s) {
			t.Errorf("selettore %q fuori dalla pagina", s)
		}
	}
	if len(sel) < 100 {
		t.Errorf("%d selettori letti: la lettura del foglio non ha funzionato", len(sel))
	}
	t.Logf("%d selettori", len(sel))
}

// TestMotoreAVecchioImpiantoTolto: nei tre file nessun resto del vecchio impianto (FE-01…FE-03, U-06).
func TestMotoreAVecchioImpiantoTolto(t *testing.T) {
	for _, f := range []string{maSTemplate, maSCSS, maSJS} {
		src := maSFile(t, f)
		for _, v := range []string{"ma-steps", "ma-navsteps", "ma-dviste", "ma-prossimo", "ma_passi", "ma_prossimo",
			"ma_conta_tipi", "ma_passo_", "Prossimo gesto"} {
			if strings.Contains(src, v) {
				t.Errorf("%s contiene %q", f, v)
			}
		}
	}
}

// TestMotoreASenzaAnalisiMessaggio: il nome della tabella dell'agente non c'è nel template, nel CSS e nel JS
// (MOTORE-SENZA-LLM; i .go li prova TestMotoreANessunaTabellaDellAgente; la stringa si compone, RAC-21).
func TestMotoreASenzaAnalisiMessaggio(t *testing.T) {
	vietata := "analisi" + "_messaggio"
	for _, f := range []string{maSTemplate, maSCSS, maSJS} {
		if strings.Contains(strings.ToLower(maSFile(t, f)), vietata) {
			t.Errorf("%s nomina la tabella dell'agente", f)
		}
	}
}
