package config

import (
	"strings"
	"testing"
)

// L1 — l'OCR del worker di analisi dichiarato per postazione (13/10/2026, richiesta dal collaudo): le chiavi
// ocr, ocr_lingua, ocr_dpi e ocr_tessdata di una [[postazione]] finiscono, copiate tali e quali, nel worker.toml
// del pacchetto di quel PC. Si leggono e si normalizzano qui; un valore che là romperebbe il TOML, o che il
// worker non saprebbe usare, ferma l'avvio del server con la postazione e la chiave nell'errore.

// postazioneConOCR è una [[postazione]] con un worker di analisi e le righe OCR date.
func postazioneConOCR(righe string) string {
	return "[[postazione]]\nnome_host = \"pc-nome \"\n" + righe +
		"\n[[worker]]\nnome = \"analisi@PC-NOME\"\ntipo = \"analisi\"\npostazione = \"PC-NOME\"\n"
}

func TestLOCRDellaPostazioneSiLeggeESiNormalizza(t *testing.T) {
	c, err := Carica(scrivi(t, postazioneConOCR(
		"ocr = \" Automatico \"\nocr_lingua = \"ita+eng\"\nocr_dpi = 400\n"+
			"ocr_tessdata = 'C:\\Program Files\\Tesseract-OCR\\tessdata'\n")))
	if err != nil {
		t.Fatal(err)
	}
	p := c.Postazioni[0]
	if p.NomeHost != "PC-NOME" || p.OCR != "automatico" || p.OCRLingua != "ita+eng" || p.OCRDPI != 400 ||
		p.OCRTessdata != `C:\Program Files\Tesseract-OCR\tessdata` {
		t.Errorf("postazione letta: %+v", p)
	}
	if !p.HaOCR() {
		t.Error("HaOCR = false con quattro chiavi dichiarate")
	}
	for _, a := range c.Avvisi {
		if strings.Contains(a, "OCR") {
			t.Errorf("avviso inatteso con un worker di analisi sulla postazione: %s", a)
		}
	}
}

// Senza chiavi OCR la postazione non ne ha: il pacchetto non scrive niente e il worker usa i suoi default.
func TestSenzaRigheOCRLaPostazioneNonNeHa(t *testing.T) {
	c, err := Carica(scrivi(t, postazioneConOCR("")))
	if err != nil {
		t.Fatal(err)
	}
	if p := c.Postazioni[0]; p.HaOCR() {
		t.Errorf("HaOCR = true senza righe: %+v", p)
	}
}

// Le forme ammesse che non sono le più ovvie: una lingua sola, lingue con il trattino basso, «spento», gli
// estremi dei dpi, un UNC e un percorso con la barra in avanti.
func TestLeFormeAmmesseDellOCR(t *testing.T) {
	for _, righe := range []string{
		"ocr = \"spento\"",
		"ocr = \"SPENTO\"",
		"ocr_lingua = \"eng\"",
		"ocr_lingua = \"chi_sim+eng\"",
		"ocr_dpi = 72",
		"ocr_dpi = 1200",
		"ocr_tessdata = '\\\\server\\condivisa\\tessdata'",
		"ocr_tessdata = 'D:/Tesseract/tessdata'",
		"ocr_tessdata = '/usr/share/tesseract-ocr/5/tessdata'",
	} {
		t.Run(righe, func(t *testing.T) {
			c, err := Carica(scrivi(t, postazioneConOCR(righe)))
			if err != nil {
				t.Fatalf("rifiutata: %v", err)
			}
			if !c.Postazioni[0].HaOCR() {
				t.Errorf("HaOCR = false con %q", righe)
			}
		})
	}
}

// Fuori dalle forme ammesse il server non parte, e l'errore nomina la postazione e la chiave.
func TestLOCRSbagliatoFermaLAvvio(t *testing.T) {
	for _, caso := range []struct{ righe, chiave string }{
		{"ocr = \"sempre\"", "ocr"},
		{"ocr_lingua = \"ita eng\"", "ocr_lingua"},
		{"ocr_lingua = \"ita, eng\"", "ocr_lingua"},
		{"ocr_lingua = 'ita\"+eng'", "ocr_lingua"},
		{"ocr_lingua = \"ENG\"", "ocr_lingua"},
		{"ocr_lingua = \"ita+\"", "ocr_lingua"},
		{"ocr_dpi = 50", "ocr_dpi"},
		{"ocr_dpi = 2000", "ocr_dpi"},
		{"ocr_dpi = -300", "ocr_dpi"},
		{"ocr_tessdata = 'tessdata'", "ocr_tessdata"},
		{"ocr_tessdata = '.\\tessdata'", "ocr_tessdata"},
		{"ocr_tessdata = 'C:tessdata'", "ocr_tessdata"},
		{"ocr_tessdata = '\\tessdata'", "ocr_tessdata"},
		{"ocr_tessdata = \"C:\\\\Programmi\\\\l'OCR\\\\tessdata\"", "ocr_tessdata"},
		{"ocr_tessdata = \"C:\\\\tess\\ndata\"", "ocr_tessdata"},
		{"ocr_tessdata = \"C:\\\\tess\\tdata\"", "ocr_tessdata"},
		{"ocr_tessdata = \"C:\\\\tess\\u007Fdata\"", "ocr_tessdata"},
		{"ocr_lingua = \"en\"", "ocr_lingua"},
		{"ocr_lingua = \"ita+en\"", "ocr_lingua"},
	} {
		t.Run(caso.righe, func(t *testing.T) {
			_, err := Carica(scrivi(t, postazioneConOCR(caso.righe)))
			if err == nil {
				t.Fatalf("il server è partito con %s", caso.righe)
			}
			for _, atteso := range []string{caso.chiave + " = ", `"PC-NOME"`} {
				if !strings.Contains(err.Error(), atteso) {
					t.Errorf("l'errore non dice %q: %v", atteso, err)
				}
			}
		})
	}
}

// L'OCR su una postazione senza worker di analisi non ferma l'avvio (il worker si aggiunge dopo, o la riga è
// rimasta), ma il log lo dice: il pacchetto lo scrive solo in [analisi], e lì non c'è.
func TestLOCRSenzaWorkerDiAnalisiEUnAvviso(t *testing.T) {
	c, err := Carica(scrivi(t, "[[postazione]]\nnome_host = \"PC-NOME\"\nocr_lingua = \"eng\"\n"+
		"[[postazione]]\nnome_host = \"PC-ALTRO\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	var trovati []string
	for _, a := range c.Avvisi {
		if strings.Contains(a, "OCR") {
			trovati = append(trovati, a)
		}
	}
	if len(trovati) != 1 || !strings.Contains(trovati[0], `"PC-NOME"`) || !strings.Contains(trovati[0], "analisi") {
		t.Errorf("avvisi sull'OCR: %q; atteso uno solo, su PC-NOME, che nomina il worker di analisi", trovati)
	}
}

// Il caso più comune dell'avviso: la postazione ha il suo worker Outlook ma non quello di analisi. Un worker di
// un altro tipo non basta a far tacere l'avviso, perché le chiavi ocr* il pacchetto le scrive solo in [analisi].
func TestLOCRConIlSoloWorkerOutlookEUnAvviso(t *testing.T) {
	c, err := Carica(scrivi(t, "[[postazione]]\nnome_host = \"PC-NOME\"\nocr_lingua = \"eng\"\n"+
		"[[worker]]\nnome = \"outlook@PC-NOME\"\ntipo = \"outlook\"\npostazione = \"PC-NOME\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	var trovati []string
	for _, a := range c.Avvisi {
		if strings.Contains(a, "OCR") {
			trovati = append(trovati, a)
		}
	}
	if len(trovati) != 1 || !strings.Contains(trovati[0], `"PC-NOME"`) || !strings.Contains(trovati[0], "analisi") {
		t.Errorf("avvisi sull'OCR: %q; atteso uno su PC-NOME, che ha solo il worker Outlook", trovati)
	}
}
