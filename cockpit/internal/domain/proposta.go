package domain

import (
	"path"
	"strings"
)

// Proposta è la prima INTERPRETAZIONE di un allegato, fatta a ingest SENZA aprire il file: solo nome,
// estensione, dimensione e direzione del messaggio. Serve a mostrare le chip in Inbox e a pre-spuntare
// gli allegati da scaricare; il worker-analisi la raffina dopo il download (cartiglio, STEP).
type Proposta struct {
	Tipo       string // valori di tipo_documento
	Fonte      string // estensione | nome_file | direzione | rumore
	Confidenza int
	Codice     string
	Rev        string
	PreSpunta  bool // suggerimento: vale la pena scaricarlo (CAD, offerte, fogli di calcolo, zip)
}

// PropostaDaNome classifica un allegato dal solo nome file. direzione è "entrata" | "uscita".
func PropostaDaNome(nomeFile string, bytes int64, direzione string) Proposta {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(nomeFile), "."))
	base := strings.TrimSuffix(nomeFile, path.Ext(nomeFile))
	tipo, fonte, conf := TipoDaEstensione(ext)
	p := Proposta{Tipo: tipo, Fonte: fonte, Confidenza: conf}

	if c, rv := CodiceRev(base); len(EstraiCodici(c)) > 0 {
		p.Codice, p.Rev = c, rv
		switch tipo {
		case "disegno_2d", "cad_3d", "sviluppo_dxf":
			p.Confidenza += 20
			p.Fonte = "nome_file"
		}
	} else if tipo == "disegno_2d" && ext == "pdf" {
		// un PDF senza codice nel nome è più spesso un documento qualunque che un disegno
		p.Tipo, p.Confidenza = "altro", 30
	}

	// offerta Promatec in uscita: "SO 5467.pdf"
	if direzione == "uscita" && ext == "pdf" && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(nomeFile)), "SO ") {
		p = Proposta{Tipo: "offerta_promatec", Fonte: "direzione", Confidenza: 90}
	}

	// rumore evidente: immagini piccole (loghi, firme), file da telefono
	if EstImmagine(ext) && (bytes < 100*1024 || rePrefissiNonCodice.MatchString(strings.ToUpper(base))) {
		p = Proposta{Tipo: "rumore", Fonte: "rumore", Confidenza: 70}
	}
	if p.Confidenza > 100 {
		p.Confidenza = 100
	}
	p.PreSpunta = daScaricare(p, ext, bytes)
	return p
}

// daScaricare è il suggerimento di pre-spunta: tecnici, offerte, fogli di calcolo e archivi sì; immagini,
// rumore e PDF anonimi no. L'operatore resta libero di cambiare.
func daScaricare(p Proposta, ext string, bytes int64) bool {
	switch p.Tipo {
	case "rumore", "corrispondenza":
		return false
	case "cad_3d", "disegno_2d", "sviluppo_dxf", "offerta_promatec", "commerciale", "distinta_cliente":
		return true
	}
	switch ext {
	case "zip", "7z", "rar", "pdf", "tif", "tiff", "dwg":
		return ext != "pdf" || p.Codice != ""
	}
	return false
}

// TipoDaEstensione è la mappa estensione → tipo_documento, fonte, confidenza di base.
func TipoDaEstensione(ext string) (tipo, fonte string, conf int) {
	switch ext {
	case "stp", "step", "sldprt", "sldasm", "igs", "iges", "x_t", "x_b", "prt", "par", "asm":
		return "cad_3d", "estensione", 70
	case "dxf":
		return "sviluppo_dxf", "estensione", 70
	case "dwg", "tif", "tiff", "pdf":
		return "disegno_2d", "estensione", 50
	case "xls", "xlsx", "csv":
		return "commerciale", "estensione", 50
	case "zip", "7z", "rar":
		return "altro", "estensione", 20 // lo zip viene estratto dopo il download e le voci proposte una per una
	case "msg", "eml":
		return "corrispondenza", "estensione", 60
	}
	return "altro", "estensione", 20
}

func EstImmagine(ext string) bool {
	switch ext {
	case "png", "jpg", "jpeg", "gif", "bmp", "svg", "webp", "ico":
		return true
	}
	return false
}
