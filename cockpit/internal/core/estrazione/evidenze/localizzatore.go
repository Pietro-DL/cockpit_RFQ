package evidenze

// Localizzatore: dove si è letto, in modo verificabile (parte 1 §3.3). È un'unione: è valorizzata solo la
// variante indicata da Tipo (testo | pdf | step | tabella | nome_file). Le coordinate che i fatti non
// hanno restano assenti, mai inventate.
type Localizzatore struct {
	Tipo     string       `json:"tipo"`
	Testo    *PosTesto    `json:"testo,omitempty"`
	PDF      *PosPDF      `json:"pdf,omitempty"`
	STEP     *PosSTEP     `json:"step,omitempty"`
	Tabella  *PosTabella  `json:"tabella,omitempty"`
	NomeFile *PosNomeFile `json:"nome_file,omitempty"`
}

// PosTesto: un intervallo su un TestoOriginale del documento, indicato per ID.
type PosTesto struct {
	TestoID    string     `json:"testo_id"`
	Intervallo Intervallo `json:"intervallo"`
}

// PosPDF: pagina 1-based, riquadro, zona e fonte, come i fatti li danno. Il riquadro è in DECIMI DI PUNTO
// interi (il worker arrotonda a 0,1 pt): così il JSON canonico resta senza numeri decimali.
type PosPDF struct {
	Pagina         int         `json:"pagina"`
	RiquadroDecimi *[4]int     `json:"riquadro_decimi,omitempty"` // [x0,y0,x1,y1] sulla pagina vista, origine in alto a sinistra
	Zona           string      `json:"zona,omitempty"`            // basso_destra | pagina
	Fonte          string      `json:"fonte,omitempty"`           // nativo | ocr
	Intervallo     *Intervallo `json:"intervallo,omitempty"`      // dentro il testo del frammento o del valore, già normalizzato dal worker
}

// PosSTEP: la chiave del PRODUCT nel file ("#19": non è un numero di riga), l'attributo letto e i
// riferimenti originali che i fatti danno.
type PosSTEP struct {
	Chiave      string   `json:"chiave"`
	Attributo   string   `json:"attributo"`             // id | nome | descrizione | formazione
	Riferimenti []string `json:"riferimenti,omitempty"` // "#f", "#d" (formation, definition)
	Occorrenze  []string `json:"occorrenze,omitempty"`  // riferimenti alle occorrenze "#n" (quelli che i fatti elencano)
}

// PosTabella: indici 1-based della tabella, della riga e della cella. Percorso è un indirizzo nel DOM
// ricostruito dal parser HTML, non una posizione nei byte del corpo HTML. RigheTesto sono le righe del testo
// a cui la cella si aggancia. Esatto è l'intervallo sul corpo_testo quando la cella coincide con le sue righe
// del testo; nil quando l'aggancio è solo per righe, e allora la localizzazione è «parziale».
type PosTabella struct {
	Tabella      int         `json:"tabella"`
	Riga         int         `json:"riga"`
	Cella        int         `json:"cella"`
	TabellaHTML  int         `json:"tabella_html,omitempty"`
	RigaHTML     int         `json:"riga_html,omitempty"`
	Colonna      int         `json:"colonna,omitempty"`
	Percorso     string      `json:"percorso,omitempty"`
	Intestazione string      `json:"intestazione,omitempty"`
	RigheTesto   *[2]int     `json:"righe_testo,omitempty"`
	Esatto       *Intervallo `json:"esatto,omitempty"`
}

// PosNomeFile: il campo acquisito (allegato.nome_file | allegato.path_interno), il percorso ricevuto e gli
// intervalli del nome intero, dello stem e dell'estensione. Gli intervalli si misurano sul TestoOriginale
// che ha per ID il campo acquisito.
type PosNomeFile struct {
	Campo      string      `json:"campo"`
	Percorso   string      `json:"percorso,omitempty"`
	Intervallo Intervallo  `json:"intervallo"`
	Stem       *Intervallo `json:"stem,omitempty"`
	Estensione *Intervallo `json:"estensione,omitempty"`
}
