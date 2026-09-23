// L1 — i pezzi dell'anteprima che non hanno bisogno di un database (blocco 8, chiusura di B8.1).
//
// Qui si prova cio' che sul disco di una prova non si provoca in modo ripetibile: una lettura che si
// interrompe dopo due byte. E il nome del file nell'intestazione, carattere per carattere, perche' e'
// una stringa che finisce in un browser e un errore li' non lo vede nessun test d'integrazione.
package web

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// lettoreRotto da' due byte e poi si rompe: il NAS che si stacca a meta' lettura.
type lettoreRotto struct{ dati []byte }

func (l *lettoreRotto) Read(p []byte) (int, error) {
	if len(l.dati) == 0 {
		return 0, errors.New("la rete e' caduta")
	}
	n := copy(p, l.dati)
	l.dati = l.dati[n:]
	return n, nil
}

func statoDi(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return http.StatusOK
	}
	var e erroreHTTP
	if !errors.As(err, &e) {
		t.Fatalf("pdfDavvero ha restituito un errore senza stato: %v", err)
	}
	return e.stato
}

func TestPdfDavveroDistingueIlGuastoDalFormato(t *testing.T) {
	casi := []struct {
		nome  string
		r     io.Reader
		stato int
	}{
		{"un PDF", strings.NewReader("%PDF-1.7\n..."), http.StatusOK},
		{"esattamente cinque byte", strings.NewReader("%PDF-"), http.StatusOK},
		{"uno zip rinominato", strings.NewReader("PK\x03\x04..."), http.StatusUnsupportedMediaType},
		{"un file vuoto", strings.NewReader(""), http.StatusUnsupportedMediaType},
		{"un file di tre byte", strings.NewReader("%PD"), http.StatusUnsupportedMediaType},
		// Il caso che il fix esiste per distinguere: prima era un 415, e nessuno lo scriveva nel log.
		{"una lettura che si interrompe", &lettoreRotto{dati: []byte("%P")}, http.StatusInternalServerError},
		{"una lettura che non comincia nemmeno", &lettoreRotto{}, http.StatusInternalServerError},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			if got := statoDi(t, pdfDavvero(c.r)); got != c.stato {
				t.Errorf("stato %d, atteso %d", got, c.stato)
			}
		})
	}
}

// L'errore di lettura porta con se' la causa vera: e' quella che finisce nel log.
func TestIlGuastoDiLetturaPortaLaCausa(t *testing.T) {
	err := pdfDavvero(&lettoreRotto{dati: []byte("%P")})
	if err == nil || !strings.Contains(err.Error(), "la rete e' caduta") {
		t.Errorf("la causa si e' persa: %v", err)
	}
}

func TestIlNomeNellIntestazione(t *testing.T) {
	casi := []struct {
		nome        string
		ascii       string // il valore di filename=
		conEsteso   bool   // se deve esserci filename*
		ricostruito string // cio' che il browser ricava da filename*
	}{
		{"disegno.pdf", "disegno.pdf", false, ""},
		{"0.000.0000.0 rev B.PDF", "0.000.0000.0 rev B.pdf", false, ""},
		{"Staffa – rev À.pdf", "Staffa _ rev _.pdf", true, "Staffa – rev À.pdf"},
		{"perno è 100%.pdf", "perno _ 100%.pdf", true, "perno è 100%.pdf"},
		// NomeFileSicuro toglie le virgolette prima di arrivare qui: l'intestazione non si spezza.
		{`capitolato "finale".pdf`, "capitolato finale.pdf", false, ""},
		{"日本語.pdf", "documento.pdf", true, "日本語.pdf"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			cd := nomePerIlBrowser(c.nome)
			for i := 0; i < len(cd); i++ {
				if cd[i] < 0x20 || cd[i] > 0x7e {
					t.Fatalf("byte 0x%02x nell'intestazione: %q", cd[i], cd)
				}
			}
			if !strings.HasPrefix(cd, `inline; filename="`+c.ascii+`"`) {
				t.Errorf("intestazione %q, attesa la forma ASCII %q", cd, c.ascii)
			}
			i := strings.Index(cd, "filename*=UTF-8''")
			if (i >= 0) != c.conEsteso {
				t.Fatalf("filename* presente=%v, atteso %v: %q", i >= 0, c.conEsteso, cd)
			}
			if !c.conEsteso {
				return
			}
			dec, err := url.PathUnescape(cd[i+len("filename*=UTF-8''"):])
			if err != nil {
				t.Fatal(err)
			}
			if dec != c.ricostruito {
				t.Errorf("il browser ricostruisce %q, atteso %q", dec, c.ricostruito)
			}
		})
	}
}

// La percentuale non lascia passare niente che possa chiudere il valore o aprirne un altro.
func TestLaPercentualeNonLasciaPassareSeparatori(t *testing.T) {
	got := percentuale(`a b;c,d"e'f\g/h=i%j`)
	for _, vietato := range []string{" ", ";", ",", `"`, "'", `\`, "/", "=", "%j"} {
		if strings.Contains(got, vietato) {
			t.Errorf("%q e' passato cosi' com'e': %q", vietato, got)
		}
	}
	if dec, _ := url.PathUnescape(got); dec != `a b;c,d"e'f\g/h=i%j` {
		t.Errorf("non torna indietro: %q", dec)
	}
}
