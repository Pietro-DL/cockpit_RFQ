package web

// L1 — Fascicolo v3: i file statici. Il viewer della vista Documenti e' un modulo (fascicolo.mjs) che carica
// pdf.js (un altro modulo, un lavoratore, dei .wasm, le mappe dei caratteri): se il server li manda con il tipo
// sbagliato il browser non li esegue e la vista resta vuota, e il tipo non lo deve decidere il registro di
// Windows. I file con l'impronta nell'indirizzo, e la cartella di pdf.js con la sua versione, il browser li
// tiene; gli altri no. La copia di pdf.js e' quella dichiarata in VERSIONE.txt, byte per byte.

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"

	risorse "promatec/cockpit"
)

func TestIFileStaticiHannoIlTipoGiustoELaCacheGiusta(t *testing.T) {
	s := serverTest(t)
	h := http.StripPrefix("/static/", s.statici())
	chiedi := func(percorso string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, percorso, nil))
		return rec
	}
	casi := []struct {
		percorso, tipo string
		immutabile     bool
	}{
		{"/static/fascicolo.mjs", "text/javascript; charset=utf-8", false},
		{s.statico("fascicolo.mjs"), "text/javascript; charset=utf-8", true},
		{"/static/style.css", "text/css; charset=utf-8", false},
		{"/static/pdfjs-6.3.289/pdf.min.mjs", "text/javascript; charset=utf-8", true},
		{"/static/pdfjs-6.3.289/pdf.worker.min.mjs", "text/javascript; charset=utf-8", true},
		{"/static/pdfjs-6.3.289/wasm/openjpeg.wasm", "application/wasm", true},
		{"/static/pdfjs-6.3.289/wasm/jbig2.wasm", "application/wasm", true},
		{"/static/pdfjs-6.3.289/cmaps/78-H.bcmap", "application/octet-stream", true},
	}
	for _, c := range casi {
		rec := chiedi(c.percorso)
		if rec.Code != 200 {
			t.Errorf("%s: %d", c.percorso, rec.Code)
			continue
		}
		if got := rec.Header().Get("Content-Type"); got != c.tipo {
			t.Errorf("%s: Content-Type %q, atteso %q", c.percorso, got, c.tipo)
		}
		immutabile := strings.Contains(rec.Header().Get("Cache-Control"), "immutable")
		if immutabile != c.immutabile {
			t.Errorf("%s: immutabile %v, atteso %v (%q)", c.percorso, immutabile, c.immutabile, rec.Header().Get("Cache-Control"))
		}
	}
	// un font standard di pdf.js, qualunque sia
	fonts, _ := fs.ReadDir(risorse.FS, "web/static/pdfjs-6.3.289/standard_fonts")
	for _, f := range fonts {
		nome := f.Name()
		atteso := map[string]string{".pfb": "application/octet-stream", ".ttf": "font/ttf"}[path.Ext(nome)]
		if atteso == "" {
			continue
		}
		if got := chiedi("/static/pdfjs-6.3.289/standard_fonts/" + nome).Header().Get("Content-Type"); got != atteso {
			t.Errorf("%s: %q", nome, got)
		}
	}

	// l'impronta e' quella del contenuto
	b, err := fs.ReadFile(risorse.FS, "web/static/fascicolo.mjs")
	if err != nil {
		t.Fatal(err)
	}
	h6 := sha256.Sum256(b)
	if got, atteso := s.statico("fascicolo.mjs"), "/static/fascicolo.mjs?v="+hex.EncodeToString(h6[:6]); got != atteso {
		t.Errorf("statico: %q, atteso %q", got, atteso)
	}
	if got := s.statico("non-c-e.js"); got != "/static/non-c-e.js" {
		t.Errorf("un file che non c'e': %q", got)
	}
	// fuori dalla cartella statica non si esce
	for _, p := range []string{"/static/../templates/fascicolo.html", "/static/..%2ftemplates%2ffascicolo.html", "/static/pdfjs-6.3.289/../../templates/layout.html"} {
		if rec := chiedi(p); rec.Code == 200 && strings.Contains(rec.Body.String(), "{{define") {
			t.Errorf("%s: esce dalla cartella statica", p)
		}
	}
}

// Si servono file, non cartelle: la radice degli statici e la cartella di pdf.js rispondono 404, con o
// senza la barra finale, invece dell'elenco di quello che contengono. I file dentro restano serviti.
func TestLeCartelleStaticheNonSiElencano(t *testing.T) {
	s := serverTest(t)
	h := http.StripPrefix("/static/", s.statici())
	for _, p := range []string{"/static/", "/static/pdfjs-6.3.289/", "/static/pdfjs-6.3.289", "/static/pdfjs-6.3.289/wasm/", "/static/./"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d, atteso 404", p, rec.Code)
		}
		if strings.Contains(rec.Body.String(), "pdf.min.mjs") || strings.Contains(rec.Body.String(), "fascicolo.mjs") {
			t.Errorf("%s: la risposta elenca il contenuto della cartella", p)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/pdfjs-6.3.289/pdf.min.mjs", nil))
	if rec.Code != 200 {
		t.Errorf("un file dentro la cartella di pdf.js: %d", rec.Code)
	}
}

// Ogni pagina carica il foglio di stile e htmx con l'impronta del contenuto (statico): un cockpit.exe
// nuovo porta file nuovi, e il browser non tiene quelli di ieri. E nella pagina c'e' il controllo che
// non lascia entrare fra i pezzi della schermata una risposta che non e' HTML.
func TestIlLayoutCaricaGliStaticiConLImpronta(t *testing.T) {
	s := serverTest(t)
	var buf bytes.Buffer
	if err := s.pagine["vietato.html"].ExecuteTemplate(&buf, "layout", vista{Titolo: "Prova", Dati: "prova"}); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, nome := range []string{"style.css", "htmx.min.js"} {
		if !strings.Contains(html, `"`+s.statico(nome)+`"`) || !strings.Contains(s.statico(nome), "?v=") {
			t.Errorf("la pagina non carica %s con l'impronta (%s)", nome, s.statico(nome))
		}
		if strings.Contains(html, `"/static/`+nome+`"`) {
			t.Errorf("la pagina carica ancora %s senza impronta", nome)
		}
	}
	for _, frammento := range []string{`"htmx:beforeSwap"`, `indexOf("text/html")`, "shouldSwap = false"} {
		if !strings.Contains(html, frammento) {
			t.Errorf("il layout non scarta le risposte che non sono HTML: manca %q", frammento)
		}
	}
}

// Le bozze dell'editor della struttura sono di chi le scrive: la chiave nel sessionStorage porta
// l'utente (che la pagina del Fascicolo dichiara in data-utente), e all'uscita il layout butta tutte le
// cockpit.editor.*. Chi entra dopo sulla stessa scheda non trova la bozza di un altro, e non la conferma
// a nome suo. Nel browser non c'è ancora una prova L7 di questo: qui si guarda che i tre pezzi ci siano
// e parlino della stessa chiave.
func TestLaBozzaDellEditorEDiChiLaScrive(t *testing.T) {
	leggi := func(p string) string {
		t.Helper()
		b, err := fs.ReadFile(risorse.FS, p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	mjs := leggi("web/static/fascicolo.mjs")
	if !strings.Contains(mjs, `"cockpit.editor." + (ds.utente || "") + "."`) {
		t.Error("fascicolo.mjs: la chiave della bozza non porta l'utente")
	}
	if !strings.Contains(leggi("web/templates/fascicolo.html"), `data-utente="{{with .Utente}}{{.UtenteID}}{{end}}"`) {
		t.Error("fascicolo.html: la pagina non dichiara l'utente per la chiave della bozza")
	}
	layout := leggi("web/templates/layout.html")
	if !strings.Contains(layout, `k.indexOf("cockpit.editor.") === 0`) || !strings.Contains(layout, "sessionStorage.removeItem(k)") {
		t.Error("layout.html: all'uscita le bozze dell'editor non si buttano")
	}
}

// La copia di pdf.js e' quella del pacchetto npm dichiarato: ogni file elencato in VERSIONE.txt c'e' con la sua
// impronta, e non c'e' niente che l'elenco non dica (una mappa .map, un file cambiato a mano).
func TestLaCopiaDiPdfjsEQuellaDichiarata(t *testing.T) {
	const dir = "web/static/pdfjs-6.3.289"
	elenco, err := fs.ReadFile(risorse.FS, dir+"/VERSIONE.txt")
	if err != nil {
		t.Fatal(err)
	}
	dichiarati := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(string(elenco)))
	for sc.Scan() {
		riga := strings.TrimSpace(sc.Text())
		sha, nome, ok := strings.Cut(riga, " *./")
		if !ok || len(sha) != 64 {
			continue
		}
		dichiarati[nome] = sha
	}
	if len(dichiarati) < 50 {
		t.Fatalf("VERSIONE.txt elenca %d file", len(dichiarati))
	}
	for _, nome := range []string{"pdf.min.mjs", "pdf.worker.min.mjs", "LICENSE", "wasm/openjpeg.wasm"} {
		if _, ok := dichiarati[nome]; !ok {
			t.Errorf("%s non e' nell'elenco", nome)
		}
	}
	trovati := 0
	err = fs.WalkDir(risorse.FS, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		nome := strings.TrimPrefix(p, dir+"/")
		if nome == "VERSIONE.txt" {
			return nil
		}
		trovati++
		atteso, ok := dichiarati[nome]
		if !ok {
			t.Errorf("%s non e' nell'elenco di VERSIONE.txt", nome)
			return nil
		}
		b, err := fs.ReadFile(risorse.FS, p)
		if err != nil {
			return err
		}
		h := sha256.Sum256(b)
		if hex.EncodeToString(h[:]) != atteso {
			t.Errorf("%s: il contenuto non e' quello dichiarato", nome)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if trovati != len(dichiarati) {
		t.Errorf("file nella cartella %d, nell'elenco %d", trovati, len(dichiarati))
	}
}
