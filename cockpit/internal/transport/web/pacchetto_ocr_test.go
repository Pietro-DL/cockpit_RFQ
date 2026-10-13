package web

import (
	"regexp"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"promatec/cockpit/internal/platform/db"
)

// L1 — l'OCR della postazione nel worker.toml del pacchetto (13/10/2026, richiesta dal collaudo). Il pacchetto
// riscrive il worker.toml da zero: le chiavi dell'OCR scritte a mano sul PC si perdevano a ogni rigenerazione.
// Ora le dichiara la [[postazione]] nel cockpit.toml del server, e workerTOML le scrive nella sola tabella
// [analisi]. Senza database: workerTOML si chiama con credenziali e segreti finti.

// credenzialiFinte: un worker per tipo dato, con il nome <tipo>@<host>.
func credenzialiFinte(host string, tipi ...db.WorkerTipo) ([]db.WorkerCredenziale, map[string]string) {
	var cred []db.WorkerCredenziale
	segreti := map[string]string{}
	for _, tipo := range tipi {
		nome := string(tipo) + "@" + host
		cred = append(cred, db.WorkerCredenziale{WorkerNome: nome, WorkerTipo: tipo})
		segreti[nome] = "segreto-" + string(tipo)
	}
	return cred, segreti
}

// rileggi decodifica il worker.toml generato come lo leggerebbe il worker (tomllib): se non è TOML valido la
// prova si ferma qui, con il testo del file.
func rileggi(t *testing.T, testo string) map[string]any {
	t.Helper()
	var m map[string]any
	if _, err := toml.Decode(testo, &m); err != nil {
		t.Fatalf("il worker.toml generato non è TOML valido: %v\n%s", err, testo)
	}
	return m
}

func tabella(t *testing.T, m map[string]any, nome string) map[string]any {
	t.Helper()
	v, ok := m[nome].(map[string]any)
	if !ok {
		t.Fatalf("manca la tabella [%s] nel worker.toml", nome)
	}
	return v
}

var chiaviOCR = []string{"ocr", "ocr_lingua", "ocr_dpi", "ocr_tessdata"}

// conOCR: il testo ha una riga che comincia con una chiave ocr, o il commento che la accompagna.
var righeOCR = regexp.MustCompile(`(?m)^(ocr|# OCR)`)

func conOCR(testo string) bool { return righeOCR.MatchString(testo) }

// (a) + (c): con l'OCR dichiarato, le quattro chiavi stanno sotto [analisi] e non sotto [outlook] né in cima, il
// file è TOML valido e rileggendolo i valori sono identici, i backslash del percorso Windows compresi.
func TestIlPacchettoScriveLOCRDellaPostazioneSoloInAnalisi(t *testing.T) {
	ocr := OCRPostazione{Modo: "automatico", Lingua: "ita+eng", DPI: 400, Tessdata: `C:\Program Files\Tesseract-OCR\tessdata`}
	s := &Server{Indirizzo: "127.0.0.1:8080", OCRPostazioni: map[string]OCRPostazione{"PC-NOME": ocr}}
	cred, segreti := credenzialiFinte("PC-NOME", db.WorkerTipoOutlook, db.WorkerTipoAnalisi)
	// l'host arriva dalla rotta: minuscolo o con spazi deve trovare la stessa postazione
	testo := s.workerTOML(" pc-nome", cred, segreti)
	m := rileggi(t, testo)

	a := tabella(t, m, "analisi")
	if a["worker_id"] != "analisi@PC-NOME" || a["token"] != "segreto-analisi" {
		t.Errorf("[analisi] worker_id/token: %v / %v", a["worker_id"], a["token"])
	}
	attesi := map[string]any{"ocr": "automatico", "ocr_lingua": "ita+eng", "ocr_dpi": int64(400),
		"ocr_tessdata": `C:\Program Files\Tesseract-OCR\tessdata`}
	for k, v := range attesi {
		if a[k] != v {
			t.Errorf("[analisi].%s = %#v; atteso %#v", k, a[k], v)
		}
	}
	o := tabella(t, m, "outlook")
	for _, k := range chiaviOCR {
		if _, c := o[k]; c {
			t.Errorf("[outlook] ha %s: l'OCR è del solo worker di analisi", k)
		}
		if _, c := m[k]; c {
			t.Errorf("%s è in cima al file: vale per tutti e due i worker", k)
		}
	}
	// stringa letterale fra apici singoli: è la forma che tiene i backslash senza raddoppiarli
	if !strings.Contains(testo, `ocr_tessdata = 'C:\Program Files\Tesseract-OCR\tessdata'`) {
		t.Errorf("ocr_tessdata non è scritto come stringa letterale:\n%s", testo)
	}
	if !strings.Contains(testo, "cockpit.toml del server") {
		t.Errorf("manca il commento che dice da dove viene l'OCR:\n%s", testo)
	}
}

// (b): una postazione senza OCR dichiarato (assente dalla mappa, o mappa nil) non riceve nessuna chiave ocr.
func TestSenzaOCRDichiaratoIlPacchettoNonScriveChiaviOCR(t *testing.T) {
	altro := map[string]OCRPostazione{"PC-ALTRO": {Lingua: "eng"}}
	for nome, mappa := range map[string]map[string]OCRPostazione{"mappa nil": nil, "host assente": altro} {
		t.Run(nome, func(t *testing.T) {
			s := &Server{Indirizzo: "127.0.0.1:8080", OCRPostazioni: mappa}
			cred, segreti := credenzialiFinte("PC-NOME", db.WorkerTipoOutlook, db.WorkerTipoAnalisi)
			testo := s.workerTOML("PC-NOME", cred, segreti)
			if conOCR(testo) {
				t.Errorf("chiavi ocr senza OCR dichiarato:\n%s", testo)
			}
			rileggi(t, testo)
		})
	}
}

// (c): solo le chiavi dichiarate, e un UNC fra apici singoli si rilegge con i suoi doppi backslash.
func TestIlPacchettoScriveSoloLeChiaviOCRDichiarate(t *testing.T) {
	s := &Server{Indirizzo: "127.0.0.1:8080", OCRPostazioni: map[string]OCRPostazione{
		"PC-NOME": {Tessdata: `\\server\condivisa\Tesseract-OCR\tessdata`}}}
	cred, segreti := credenzialiFinte("PC-NOME", db.WorkerTipoAnalisi)
	a := tabella(t, rileggi(t, s.workerTOML("PC-NOME", cred, segreti)), "analisi")
	if a["ocr_tessdata"] != `\\server\condivisa\Tesseract-OCR\tessdata` {
		t.Errorf("ocr_tessdata riletto = %#v", a["ocr_tessdata"])
	}
	for _, k := range []string{"ocr", "ocr_lingua", "ocr_dpi"} {
		if _, c := a[k]; c {
			t.Errorf("[analisi].%s scritto senza essere dichiarato: il worker deve usare il suo default", k)
		}
	}
}

// (d): una postazione con il solo worker Outlook non riceve chiavi OCR, anche se le dichiara (il server lo dice
// nel log all'avvio, in config).
func TestUnaPostazioneSoloOutlookNonRiceveLOCR(t *testing.T) {
	s := &Server{Indirizzo: "127.0.0.1:8080", OCRPostazioni: map[string]OCRPostazione{
		"PC-NOME": {Modo: "spento", Lingua: "eng", DPI: 300, Tessdata: `C:\tessdata`}}}
	cred, segreti := credenzialiFinte("PC-NOME", db.WorkerTipoOutlook)
	testo := s.workerTOML("PC-NOME", cred, segreti)
	if conOCR(testo) {
		t.Errorf("chiavi ocr in un pacchetto senza worker di analisi:\n%s", testo)
	}
	m := rileggi(t, testo)
	if _, c := m["analisi"]; c {
		t.Error("tabella [analisi] in un pacchetto senza worker di analisi")
	}
}

// Il foglio delle istruzioni dice da dove arriva l'OCR, perché chi lo cerca nel worker.toml del PC sappia dove
// cambiarlo.
func TestLeIstruzioniDiconoDoveSiCambiaLOCR(t *testing.T) {
	if testo := istruzioni("PC-NOME", "http://127.0.0.1:8080", false); !strings.Contains(testo, "[[postazione]]") ||
		!strings.Contains(testo, "ocr_tessdata") {
		t.Errorf("le istruzioni non dicono da dove viene l'OCR:\n%s", testo)
	}
	// Il server legge il cockpit.toml all'avvio: un pacchetto generato prima del riavvio esce senza le righe ocr*,
	// sovrascrive il worker.toml del PC e l'OCR resta spento senza segnali. Il foglio lo deve dire.
	if testo := istruzioni("PC-NOME", "http://127.0.0.1:8080", false); !strings.Contains(testo, "RIAVVIO del server") {
		t.Errorf("le istruzioni non dicono che una modifica all'OCR vale dal riavvio del server:\n%s", testo)
	}
}
