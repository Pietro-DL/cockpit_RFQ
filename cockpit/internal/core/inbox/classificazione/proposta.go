package classificazione

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
	// CodiciNelNome: i codici trovati DENTRO un nome che non e' esso stesso un codice («Offerta
	// 12345678 per le staffe.pdf»). Non finiscono in Codice — quel campo e' il codice del documento,
	// non un elenco — ma non si buttano: vanno nei dettagli della proposta, dove l'operatore e il
	// Dossier li ritrovano.
	CodiciNelNome []string
}

// SogliaStagingAutomatico è la dimensione oltre la quale un allegato non scende da solo (D30).
//
// Venti megabyte. Sotto, un file di un cliente riconosciuto arriva nello staging del server senza che
// nessuno prema niente, perché il tipo di un PDF si sa solo aprendolo e l'operatore ha bisogno di
// vedere «disegno 2D, cartiglio 6674611A rev 4» invece di «PDF, da determinare, premi qui e aspetta».
// Sopra, no: un pacco da mezzo giga lo si scarica quando qualcuno lo decide.
//
// Lo staging è una cartella del server. La regola «niente sul NAS senza una decisione» (D24, criterio E)
// resta intatta: quella riguarda il NAS, e il NAS non viene toccato da qui.
const SogliaStagingAutomatico int64 = 20 * 1024 * 1024

// PropostaDaNome classifica un allegato dal solo nome file. direzione è "entrata" | "uscita".
func PropostaDaNome(nomeFile string, bytes int64, direzione string) Proposta {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(nomeFile), "."))
	base := strings.TrimSuffix(nomeFile, path.Ext(nomeFile))
	tipo, fonte, conf := TipoDaEstensione(ext)
	p := Proposta{Tipo: tipo, Fonte: fonte, Confidenza: conf}

	// Il nome (senza revisione) e' il codice del documento solo se E' un codice: uno solo, senza
	// spazi, entro MaxCodice. «6674611A_4» si'; «Offerta 12345678 per le staffe zincate» no, anche
	// se dentro c'e' un numero che sembra un codice — quello va in CodiciNelNome. Prima bastava che
	// il nome CONTENESSE un codice perche' l'intero nome diventasse il codice (7C.1, P0).
	c, rv := CodiceRev(base)
	if !sembraCodice(c) {
		p.CodiciNelNome = EstraiCodici(base)
		c, rv = "", ""
	}
	if c != "" {
		p.Codice, p.Rev = c, rv
		switch tipo {
		case "disegno_2d", "cad_3d", "sviluppo_dxf":
			// Formati che SONO disegno: un .dxf o un .step non contengono altro. Qui il codice nel
			// nome conferma qualcosa che l'estensione ha già detto.
			p.Confidenza += 20
			p.Fonte = "nome_file"
		case "da_determinare":
			// Un PDF con un codice nel nome resta un PDF con un codice nel nome (checkpoint 3R §5).
			// Il codice si conserva, perché servirà; il TIPO no: «6674611A.pdf» è il disegno, ma
			// anche l'offerta del fornitore per quel pezzo, e anche la conferma d'ordine. Chi lo
			// stabilisce è il worker-analisi leggendo il cartiglio, non chi legge il nome.
			p.Confidenza, p.Fonte = 50, "nome_file"
		}
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
	case "da_determinare":
		// Un tipo da determinare è il motivo per cui vale la pena scaricarlo: finché il file non
		// scende, il tipo non si saprà mai, e la pre-spunta è un suggerimento di download, non
		// un'affermazione su che cosa il file sia. Sopra la soglia no: un file enorme di cui non si
		// sa niente si scarica quando qualcuno lo decide. È la stessa soglia dello staging
		// automatico (D30), così «si scarica da solo» e «arriva pre-spuntato» non si contraddicono.
		return bytes <= SogliaStagingAutomatico
	}
	switch ext {
	case "zip", "7z", "rar", "dwg":
		return true
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
	case "dwg":
		return "disegno_2d", "estensione", 70 // un DWG è un disegno CAD: l'estensione lo dice
	case "pdf", "tif", "tiff":
		// Checkpoint 3R §5: PDF NON significa disegno. Un PDF è un contenitore, e in una richiesta
		// d'offerta vera contiene tanto spesso il capitolato, l'offerta o la conferma d'ordine quanto
		// il disegno. Dire `disegno_2d` per estensione era un'affermazione tecnica su un file che
		// nessuno aveva aperto, e si propagava: pre-spunta, punteggio del triage «allegato tecnico»,
		// sottocartella ELENCO DISEGNI sul NAS. Prima dell'analisi il tipo non si sa, e si dice.
		return "da_determinare", "estensione", 40
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
