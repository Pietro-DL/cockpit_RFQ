package runtime

// L1 — l'OCR delle [[postazione]] passa al server web senza database (13/10/2026): solo le postazioni che ne
// dichiarano, per nome host normalizzato, con i quattro valori così come config li ha controllati.

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/transport/web"
)

func TestLOCRDellePostazioniArrivaAlServerWeb(t *testing.T) {
	cfg := &config.Config{Postazioni: []config.Postazione{
		{NomeHost: "PC-NOME", OCR: "automatico", OCRLingua: "eng", OCRDPI: 300, OCRTessdata: `C:\Program Files\Tesseract-OCR\tessdata`},
		{NomeHost: "PC-SENZA"},
		{NomeHost: "PC-SOLO-LINGUA", OCRLingua: "ita+eng"},
	}}
	m := ocrDellePostazioni(cfg)
	if len(m) != 2 {
		t.Fatalf("postazioni con OCR: %v; attese PC-NOME e PC-SOLO-LINGUA", m)
	}
	if m["PC-NOME"] != (web.OCRPostazione{Modo: "automatico", Lingua: "eng", DPI: 300, Tessdata: `C:\Program Files\Tesseract-OCR\tessdata`}) {
		t.Errorf("PC-NOME = %+v", m["PC-NOME"])
	}
	if m["PC-SOLO-LINGUA"] != (web.OCRPostazione{Lingua: "ita+eng"}) {
		t.Errorf("PC-SOLO-LINGUA = %+v", m["PC-SOLO-LINGUA"])
	}
	if _, c := m["PC-SENZA"]; c {
		t.Error("una postazione senza OCR è nella mappa")
	}
}

// Il collegamento fra il cockpit.toml e il server web, per la stessa funzione che usa Ascolta (serverWeb): i
// valori presi da cfg sono campi con nome di un letterale, e toglierne uno non rompe la compilazione. Senza
// questa prova il server partirebbe lo stesso e il pacchetto uscirebbe senza le righe ocr*. Con l'OCR si
// controllano anche i giorni di «Carica precedenti» e le righe dell'Inbox, che passano dallo stesso letterale.
func TestIlServerWebRiceveLOCRDalCockpitToml(t *testing.T) {
	d := t.TempDir()
	corpo := "[db]\ndsn = \"postgres://x@localhost/y\"\n[nas]\nstaging = '" + filepath.Join(d, "staging") + "'\n" +
		"[outlook]\ngiorni_carica_precedenti = 45\ninbox_max_messaggi = 150\n" +
		"[[postazione]]\nnome_host = \"pc-nome\"\nocr_lingua = \"eng\"\n" +
		"ocr_tessdata = 'C:\\Program Files\\Tesseract-OCR\\tessdata'\n" +
		"[[postazione]]\nnome_host = \"PC-SENZA\"\n" +
		"[[worker]]\nnome = \"analisi@PC-NOME\"\ntipo = \"analisi\"\npostazione = \"PC-NOME\"\n"
	p := filepath.Join(d, "cockpit.toml")
	if err := os.WriteFile(p, []byte(corpo), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Carica(p)
	if err != nil {
		t.Fatal(err)
	}
	ws := serverWeb(cfg, &Servizi{}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	atteso := web.OCRPostazione{Lingua: "eng", Tessdata: `C:\Program Files\Tesseract-OCR\tessdata`}
	if len(ws.OCRPostazioni) != 1 || ws.OCRPostazioni["PC-NOME"] != atteso {
		t.Errorf("OCRPostazioni = %+v; atteso solo PC-NOME = %+v", ws.OCRPostazioni, atteso)
	}
	if ws.GiorniStorico != 45 || ws.InboxMaxMessaggi != 150 {
		t.Errorf("GiorniStorico = %d, InboxMaxMessaggi = %d; attesi 45 e 150", ws.GiorniStorico, ws.InboxMaxMessaggi)
	}
}
