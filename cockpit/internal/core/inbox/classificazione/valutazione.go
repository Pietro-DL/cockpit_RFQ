package classificazione

import (
	"encoding/json"
	"path"
	"regexp"
	"sort"
	"strings"
)

// La valutazione di un file per dimensione (Smistamento, A5.14). Una riga di documento_proposta dice
// insieme tipo, codice e rev con un solo numero e una sola fonte: «CAD 3D = 90» si leggeva «associazione =
// 90», e la stessa lettura dal nome valeva 40, 50, 80 o 90 secondo chi l'aveva scritta. Qui ogni dimensione
// ha il suo valore, il suo score, la regola che lo dà e l'elenco delle evidenze, nella forma di
// `dettagli.valutazione` (v1, A5.14.2).
//
// Lo score e' di REGOLA, non una probabilita': ordina, non decide (U7), e finche' la calibrazione non l'ha
// misurato non si scrive mai come percentuale. La valutazione la scrive ogni lettore di un file con una sola
// funzione (Valuta, in valuta.go) e con gli score della tabella S1 (punteggi.go); si legge da
// `dettagli.valutazione`, e per le righe scritte prima si ricostruisce al volo dalle colonne
// (ValutazioneDaRiga).

// VersioneValutazione e' la forma di `dettagli.valutazione`. Un lettore che non la conosce tratta la riga
// come se non l'avesse (e la ricostruisce).
const VersioneValutazione = 1

// Gli stati di una dimensione (A5.14.2).
const (
	StatoNessuna  = "nessuna"  // nessuna evidenza con un valore
	StatoUnica    = "unica"    // un valore, da una sola fonte indipendente
	StatoConcorde = "concorde" // un valore, da almeno due fonti indipendenti
	StatoDiscorde = "discorde" // valori diversi, e il secondo con score >= SogliaDiscordanza
)

// SogliaDiscordanza: sotto questo score un valore diverso non contraddice il vincente (C4).
const SogliaDiscordanza = 30

// RegolaOperatore e' la «regola» di una decisione: non ha uno score da mostrare (U-C4). Il 100 in colonna
// resta per compatibilita' con chi lo legge.
const RegolaOperatore = "operatore"

// Evidenza e' una lettura di una dimensione: da che regola, da che fonte, che valore, con che score.
type Evidenza struct {
	Regola string `json:"regola"`
	Fonte  string `json:"fonte"` // un valore dell'enum fonte_proposta
	Valore string `json:"valore"`
	Score  int    `json:"score"`
	Testo  string `json:"testo,omitempty"` // il frammento letto: «.stp», «7120001A_1.stp», «_1»
	// Famiglia e Dove: per le letture di un codice di famiglia, quale famiglia e dove l'ha trovato.
	Famiglia string `json:"famiglia,omitempty"`
	Dove     string `json:"dove,omitempty"`
	// DipendeDa: la fonte da cui questa lettura dipende (un PRODUCT uguale al nome del file dipende dal nome):
	// non conta come seconda fonte per `concorde` (C5).
	DipendeDa string `json:"dipende_da,omitempty"`
}

// Dimensione e' una delle letture di un file (tipo, codice, rev): il valore vincente, il suo score e la sua
// regola, lo stato e tutte le evidenze, in ordine di score.
type Dimensione struct {
	Valore   string     `json:"valore"`
	Score    int        `json:"score"`
	Regola   string     `json:"regola"`
	Stato    string     `json:"stato"`
	Evidenze []Evidenza `json:"evidenze"`
}

// Valutazione e' `dettagli.valutazione`: le tre dimensioni del file, con la tabella che ha dato gli score.
type Valutazione struct {
	V           int        `json:"v"`
	Tabella     string     `json:"tabella"`
	Da          string     `json:"da,omitempty"`
	CalcolataIl string     `json:"calcolata_il,omitempty"`
	Tipo        Dimensione `json:"tipo"`
	Codice      Dimensione `json:"codice"`
	Rev         Dimensione `json:"rev"`
	// Ricostruita: letta dalle colonne di una riga scritta prima della valutazione, non salvata (A5.10).
	Ricostruita bool `json:"ricostruita,omitempty"`
	// Trattenuta: la rev che un file caricato a mano porta nel nome (o nel PRODUCT) e che non si propone
	// come rev del cliente (RevisioneProponibile, B8.7). Lo scrittore la mette in `dettagli.rev_letta`; nella
	// valutazione e' l'evidenza `rev_trattenuta_interno`.
	Trattenuta string `json:"-"`
}

// Decisa dice se la valutazione e' la decisione di una persona (riga con fonte `operatore`): allora non ha
// score da mostrare, ha un autore.
func (v Valutazione) Decisa() bool {
	return v.Codice.Regola == RegolaOperatore || v.Tipo.Regola == RegolaOperatore
}

// evidenza costruisce una lettura con lo score e la fonte della sua regola: nessun numero scritto a mano.
func evidenza(regola, valore, testo string) Evidenza {
	r := Punteggi[regola]
	if r := []rune(testo); len(r) > 200 {
		testo = string(r[:200]) // A5.14.2: una valutazione resta piccola
	}
	return Evidenza{Regola: regola, Fonte: r.Fonte, Valore: valore, Score: r.Score, Testo: testo}
}

// MaxEvidenze e' il numero massimo di evidenze che una dimensione conserva (A5.14.2): le piu' forti.
const MaxEvidenze = 8

// Componi fa di un elenco di evidenze una dimensione (A5.14.3). Il valore e' quello della lettura con lo
// score piu' alto: il massimo, mai la somma, perche' tre indizi deboli non battono una prova (D9). A parita'
// di score vince la regola che viene prima nella tabella S1, e se i valori sono diversi la dimensione e'
// comunque discorde. Lo stato conta le fonti indipendenti che dicono il valore vincente (una lettura che
// dipende da un'altra, come il PRODUCT uguale al nome del file, non fa una seconda fonte: C5) e dichiara la
// discordanza quando un altro valore ha almeno SogliaDiscordanza (C4). Nessuna aritmetica di penalita' (K12):
// la concordanza si conta e non alza il numero, la discordanza si dice e non lo abbassa.
//
// Una lettura che dipende da un'altra PUO' vincere la dimensione con il suo score, quando la sua regola vale
// di piu': il PRODUCT «7120001A_1» uguale al nome da' la rev con rev_step_product 45 e non con
// rev_suffisso_nome 40, e una radice di famiglia uguale al nome porta il codice a step_radice_famiglia 80.
// E' una scelta, non un caso: `dipende_da` dice solo che non fa `concorde` (A5.14.2, C5), e il valore non
// cambia. Se si vorra' che vinca la lettura indipendente (P16, onesta' delle fonti), e' qui che si decide:
// a parita' di valore la regola della fonte al posto di quella dipendente. Punto aperto per l'architetto.
//
// Le evidenze senza valore restano nell'elenco e non votano. L'elenco torna in ordine di score e di
// precedenza, e se e' piu' lungo di MaxEvidenze si tengono le piu' forti.
func Componi(ev []Evidenza) Dimensione {
	ord := append([]Evidenza(nil), ev...)
	sort.SliceStable(ord, func(i, j int) bool {
		if ord[i].Score != ord[j].Score {
			return ord[i].Score > ord[j].Score
		}
		return precedenza(ord[i].Regola) < precedenza(ord[j].Regola)
	})
	if len(ord) > MaxEvidenze {
		ord = ord[:MaxEvidenze]
	}
	if ord == nil {
		ord = []Evidenza{}
	}
	d := Dimensione{Stato: StatoNessuna, Evidenze: ord}
	vinc := -1
	for i, e := range ord {
		if e.Valore != "" {
			vinc = i
			break
		}
	}
	if vinc < 0 {
		if len(ord) > 0 {
			d.Regola = ord[0].Regola
		}
		return d
	}
	w := ord[vinc]
	d.Valore, d.Score, d.Regola, d.Stato = w.Valore, w.Score, w.Regola, StatoUnica
	indipendenti := 0
	for _, e := range ord {
		switch {
		case e.Valore == "":
		case strings.EqualFold(e.Valore, w.Valore):
			if e.DipendeDa == "" {
				indipendenti++
			}
		case e.Score >= SogliaDiscordanza:
			d.Stato = StatoDiscorde
		}
	}
	if d.Stato != StatoDiscorde && indipendenti >= 2 {
		d.Stato = StatoConcorde
	}
	return d
}

// LeggiValutazione legge `dettagli.valutazione` di una riga, se c'e' e se e' nella forma che si conosce.
func LeggiValutazione(dettagli []byte) (Valutazione, bool) {
	var d struct {
		Valutazione *Valutazione `json:"valutazione"`
	}
	if len(dettagli) == 0 || json.Unmarshal(dettagli, &d) != nil || d.Valutazione == nil || d.Valutazione.V != VersioneValutazione {
		return Valutazione{}, false
	}
	return *d.Valutazione, true
}

// dettagliRiga sono le chiavi di oggi di documento_proposta.dettagli che servono a rileggere una riga.
type dettagliRiga struct {
	TerminiTrovati     []string `json:"termini_trovati"`
	ProductStep        string   `json:"product_step"`
	Struttura          any      `json:"struttura"`
	Famiglia           string   `json:"famiglia"`
	Dove               string   `json:"dove"`
	Testo              string   `json:"testo"`
	CodiceLetto        string   `json:"codice_letto"`
	RevLetta           string   `json:"rev_letta"`
	CodiciNelNome      []string `json:"codici_nel_nome"`
	TestoLetto         *int     `json:"testo_letto"`
	CodiceRiconosciuto *string  `json:"codice_riconosciuto"`
	ErrorePdf          string   `json:"errore_pdf"` // il worker ha provato a leggere il PDF e non si apre
}

// ValutazioneDaRiga ricostruisce la valutazione di una riga scritta prima che la valutazione esistesse
// (A5.14.2, A5.10): nessun backfill, la si rilegge dalle colonne e dai dettagli di oggi ogni volta che la
// si mostra, con `Ricostruita`. Le colonne dicono insieme tipo, codice e rev con il numero di chi le ha
// scritte; qui ogni dimensione riprende la sua evidenza con lo score della sua regola:
//   - il tipo dall'estensione, o dalla fonte della colonna quando e' il testo del PDF, la direzione, il
//     rumore o la risposta del fornitore;
//   - codice e rev RILETTI dal nome del file (o da `codice_letto`, la lettura che l'assegnazione a un
//     componente ha messo da parte), piu' l'evidenza dello STEP quando la colonna la dice. Un codice non ha
//     mai fonte `estensione` (lo scrivevano l'ingest e il worker per i tipi `altro`) e il «cartiglio» che era
//     il nome del file e' una sola evidenza, del nome (P16);
//   - una riga con fonte `operatore` e' una decisione: le tre dimensioni la riportano, senza score.
//
// La confidenza serve solo a riconoscere il rumore, che la colonna non distingue in altro modo.
func ValutazioneDaRiga(tipo, codice, rev, fonte string, confidenza int, dettagli []byte, nomeFile, estensione string) Valutazione {
	v := Valutazione{V: VersioneValutazione, Tabella: TabellaPunteggi, Ricostruita: true}
	codice, rev = strings.TrimSpace(codice), strings.TrimSpace(rev)
	if fonte == "operatore" {
		dec := func(val string) Dimensione {
			e := evidenza(RegolaOperatore, val, "")
			if val == "" {
				return Dimensione{Stato: StatoNessuna, Regola: RegolaOperatore, Evidenze: []Evidenza{e}}
			}
			return Dimensione{Valore: val, Score: e.Score, Regola: RegolaOperatore, Stato: StatoUnica, Evidenze: []Evidenza{e}}
		}
		if tipo == "da_determinare" {
			tipo = ""
		}
		v.Tipo, v.Codice, v.Rev = dec(tipo), dec(codice), dec(rev)
		return v
	}
	var dt dettagliRiga
	_ = json.Unmarshal(dettagli, &dt)
	ext := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(estensione), "."))
	if ext == "" {
		ext = strings.ToLower(strings.TrimPrefix(path.Ext(nomeFile), "."))
	}
	v.Tipo = Componi(tipoDaRiga(tipo, fonte, confidenza, ext, dt))

	// Il codice del nome: il nome intero, tolta la rev, se ha la forma di un codice (come PropostaDaNome).
	base := strings.TrimSuffix(nomeFile, path.Ext(nomeFile))
	nomeCod, nomeRev := CodiceRev(base)
	if !sembraCodice(nomeCod) {
		nomeCod, nomeRev = "", ""
	}
	daNome := nomeCod != ""
	if daNome && codice != "" && codice != nomeCod && fonte != "step" && fonte != "regola_cliente" &&
		strings.HasPrefix(nomeCod, strings.ToUpper(codice)) && sembraCodice(strings.ToUpper(codice)) {
		// il suffisso decorativo del cliente tolto quando la riga e' stata scritta («X_PRT» → X): la lettura
		// e' sempre quella del nome, nella forma che il motore del cliente le aveva dato
		nomeCod = strings.ToUpper(codice)
	}
	if dt.CodiceLetto != "" {
		// assegnata a un componente con un altro codice: la colonna dice il componente, la lettura e' qui
		nomeCod, daNome = strings.ToUpper(strings.TrimSpace(dt.CodiceLetto)), true
	}

	var ec []Evidenza
	if daNome {
		ec = append(ec, evidenza("nome_codice_generico", nomeCod, nomeFile))
	}
	// La colonna dice un codice dallo STEP: la radice riconosciuta da una famiglia (la D16) o il primo PRODUCT
	// (il worker). Uguale al nome, dipende dal nome e non fa una seconda fonte.
	var eStep *Evidenza
	switch {
	case codice != "" && fonte == "regola_cliente":
		e := evidenza("step_radice_famiglia", strings.ToUpper(codice), dt.Testo)
		e.Famiglia, e.Dove = dt.Famiglia, dt.Dove
		eStep = &e
	case codice != "" && fonte == "step":
		testo := dt.ProductStep
		if testo != "" {
			testo = "PRODUCT('" + testo + "')"
		}
		e := evidenza("step_primo_product", strings.ToUpper(codice), testo)
		eStep = &e
	}
	if eStep != nil {
		if daNome && strings.EqualFold(eStep.Valore, nomeCod) {
			eStep.DipendeDa = "nome_file"
		}
		// la radice di famiglia viene prima nella precedenza: a parita' di score vince lei
		ec = append([]Evidenza{*eStep}, ec...)
	}
	if !daNome && eStep == nil {
		// un nome che non e' un codice ma ne cita: si registra, non vota (resta `codici_nel_nome`)
		citati := dt.CodiciNelNome
		if len(citati) == 0 {
			citati = EstraiCodici(strings.ToUpper(base))
		}
		for _, c := range citati {
			if sembraCodice(c) {
				// il testo e' il codice citato, come in Valuta
				ec = append(ec, evidenza("nome_contiene_codice", "", c))
				break
			}
		}
	}
	v.Codice = Componi(ec)

	// La rev: dal nome, con la regola del suo ramo; dallo STEP se la colonna dice un'altra rev con quella fonte.
	var er []Evidenza
	if dt.RevLetta != "" {
		// un file caricato a mano non propone una rev del cliente (RevisioneProponibile): la si dice e basta
		e := evidenza("rev_trattenuta_interno", "", "rev "+dt.RevLetta+" letta nel file caricato a mano, non del cliente")
		er = append(er, e)
	} else {
		if daNome && nomeRev != "" && dt.CodiceLetto == "" {
			er = append(er, evidenza(regolaRev(base), nomeRev, suffissoRev(base, nomeRev)))
		}
		if rev != "" && !strings.EqualFold(rev, nomeRev) {
			switch fonte {
			case "regola_cliente":
				er = append([]Evidenza{evidenza("rev_famiglia_cliente", strings.ToUpper(rev), dt.Testo)}, er...)
			case "step":
				er = append(er, evidenza("rev_step_product", strings.ToUpper(rev), dt.ProductStep))
			}
		}
	}
	v.Rev = Componi(er)
	return v
}

// tipoDaRiga sono le evidenze del tipo di una riga di prima: l'estensione, piu' quella che la fonte della
// colonna aggiunge.
func tipoDaRiga(tipo, fonte string, confidenza int, ext string, dt dettagliRiga) []Evidenza {
	termini := strings.Join(dt.TerminiTrovati, ", ")
	switch {
	case dt.ErrorePdf != "":
		// il worker l'ha letto e non si apre (tipo da_determinare, fonte estensione): non e' «PDF non ancora
		// letto», che direbbe all'operatore di aspettare una lettura che c'e' gia' stata
		return []Evidenza{evidenza("pdf_illeggibile", "", "")}
	case tipo == "rumore" || fonte == "rumore":
		// la colonna non dice perche': lo dicono i numeri con cui lo scrivevano lo stage (95 e 85) e l'ingest
		switch {
		case confidenza >= 95:
			return []Evidenza{evidenza("rumore_hash_dominio", "rumore", "")}
		case confidenza >= 85:
			return []Evidenza{evidenza("rumore_immagine_ricorrente", "rumore", "")}
		}
		return []Evidenza{evidenza("rumore_immagine", "rumore", "")}
	case tipo == "offerta_fornitore":
		return append(tipoDaEstensione(ext), evidenza("risposta_fornitore", "offerta_fornitore", ""))
	case tipo == "offerta_promatec" && fonte == "direzione":
		return []Evidenza{evidenza("nome_so_uscita", "offerta_promatec", "")}
	case fonte == "cartiglio":
		regola := map[string]string{"disegno_2d": "pdf_termini_cartiglio", "offerta_promatec": "pdf_termini_offerta",
			"commerciale": "pdf_termini_offerta_entrata", "capitolato": "pdf_termini_capitolato",
			"distinta_cliente": "pdf_termini_distinta"}[tipo]
		if regola != "" {
			return []Evidenza{evidenza(regola, tipo, termini)}
		}
	case tipo == "commerciale" && dt.TerminiTrovati != nil:
		// l'offerta in entrata: il worker la diceva «cartiglio», il server la girava in commerciale
		return []Evidenza{evidenza("pdf_termini_offerta_entrata", tipo, termini)}
	}
	ev := tipoDaEstensione(ext)
	if ext == "stp" || ext == "step" {
		if dt.Struttura != nil || dt.ProductStep != "" {
			ev = append(ev, evidenza("step_letto", "cad_3d", ""))
		}
	}
	if (ext == "pdf" || ext == "tif" || ext == "tiff") && (dt.TestoLetto != nil || dt.CodiceRiconosciuto != nil) {
		ev = []Evidenza{evidenza("pdf_nessun_termine", "", "")}
	}
	return ev
}

// tipoDaEstensione e' la lettura del solo formato, con le regole di A5.14.3: un PDF e un archivio non
// dicono il tipo (stato `nessuna`, la colonna resta `da_determinare`).
func tipoDaEstensione(ext string) []Evidenza {
	t := "." + ext
	switch ext {
	case "stp", "step", "sldprt", "sldasm", "igs", "iges", "x_t", "x_b", "prt", "par", "asm":
		return []Evidenza{evidenza("ext_3d", "cad_3d", t)}
	case "dwg":
		return []Evidenza{evidenza("ext_dwg", "disegno_2d", t)}
	case "dxf":
		return []Evidenza{evidenza("ext_dxf", "sviluppo_dxf", t)}
	case "msg", "eml":
		return []Evidenza{evidenza("ext_posta", "corrispondenza", t)}
	case "xls", "xlsx", "csv":
		return []Evidenza{evidenza("ext_foglio", "commerciale", t)}
	case "pdf", "tif", "tiff":
		return []Evidenza{evidenza("ext_pdf", "", t)}
	case "zip", "7z", "rar":
		return []Evidenza{evidenza("ext_archivio", "", t)}
	case "":
		return []Evidenza{evidenza("ext_altro", "altro", "senza estensione")}
	}
	return []Evidenza{evidenza("ext_altro", "altro", t)}
}

// regolaRev e' la regola del ramo di CodiceRev che ha separato la rev dal nome.
func regolaRev(base string) string {
	s := strings.ToUpper(strings.TrimSpace(base))
	if reRevNas.MatchString(s) {
		return "rev_nas"
	}
	if m := reRevEsplicita.FindStringSubmatch(s); m != nil {
		return "rev_esplicita_nome"
	}
	return "rev_suffisso_nome"
}

// reRevEsplicita e' il ramo di reRev con la parola: «_REV3», «-R2».
var reRevEsplicita = regexp.MustCompile(`^(.+?)[_\-](?:REV|R)([0-9]{1,2}|[A-Z])$`)

// suffissoRev e' il pezzo del nome che dice la rev: «_1», «_REV3», «_REV_B».
func suffissoRev(base, rev string) string {
	s := strings.ToUpper(strings.TrimSpace(base))
	for _, sep := range []string{"_REV_", "_REV", "-REV", "_R", "-R", "_", "-"} {
		if i := strings.LastIndex(s, sep+rev); i > 0 {
			return s[i:]
		}
	}
	return rev
}
