package classificazione

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"
)

// La lettura di un file per dimensione (Smistamento F4, A5.14.2, A5.14.7). Prima ogni scrittore di
// documento_proposta aveva i suoi numeri e la sua idea di che cosa fosse la colonna `confidenza`: l'ingest
// sommava estensione e nome, il worker scriveva «cartiglio 95» per un codice che era il nome del file, la D16
// riscriveva codice e fonte con la radice dello STEP. Adesso tutti (ingest, stage, archivio, analisi, fatti
// esistenti, struttura, risposta del fornitore) passano da Valuta con quello che sanno del file, e scrivono
// la valutazione e le colonne con ConValutazione: le colonne sono il suo riepilogo, e un solo posto le
// definisce (Riepilogo).
//
// Valuta e' pura: nessun database, nessuna ora. Non decide niente: dice che cosa dicono le evidenze, con gli
// score della tabella S1, e dichiara quando non sono d'accordo.

// Chi ha calcolato una valutazione (`valutazione.da`, A5.14.2).
const (
	DaIngest            = "ingest"
	DaStage             = "stage"
	DaArchivio          = "archivio"
	DaAnalisi           = "analisi"
	DaFattiEsistenti    = "fatti_esistenti"
	DaStruttura         = "struttura"
	DaRispostaFornitore = "risposta_fornitore"
)

// IngressoFile e' tutto cio' che uno scrittore sa di un file quando lo legge. Ogni campo e' facoltativo
// tranne il nome: chi non ha l'analisi non passa Esito e Fatti, chi non ha una RFQ passa il motore del
// cliente riconosciuto (o nil).
type IngressoFile struct {
	Da        string // chi valuta: DaIngest, DaStage, …
	NomeFile  string // il nome di QUESTO allegato: il codice del nome non passa fra file con lo stesso contenuto (P15)
	Bytes     int64  // la dimensione: un'immagine piccola e' rumore
	Direzione string // "entrata" | "uscita"
	Interno   bool   // caricato a mano: la rev che porta non e' del cliente (RevisioneProponibile, B8.7)
	Motore    *Motore
	// Esito e Fatti sono la lettura del worker di analisi, quando c'e' stata: il tipo che ha concluso con la
	// sua fonte, e i dettagli che ha visto (termini trovati, PRODUCT, struttura). Il codice e la rev
	// dell'esito NON si usano: il worker li legge dal nome del file che ha analizzato, che puo' essere
	// un'altra copia (P15), e quando li legge da un «cartiglio» sono comunque il nome (P16). Del contenuto
	// passano i termini e il PRODUCT, che sono del file.
	Esito *Esito
	Fatti json.RawMessage
	// Radice e' la radice dello STEP classificata con le regole della RFQ (ApplicaStruttura): al posto del
	// primo PRODUCT, quando c'e'.
	Radice *Radice
	// Rumore e' la regola di rumore che lo stage ha riconosciuto sul contenuto (rumore_hash_dominio,
	// rumore_immagine_ricorrente). Mai per un file tecnico (P14).
	Rumore string
	// RispostaFornitore: l'operatore ha detto che la mail e' la risposta del fornitore (e resta vero a ogni
	// rilettura del file).
	RispostaFornitore bool
}

// Esito e' la conclusione del worker di analisi: tipo e fonte (valori degli enum).
type Esito struct {
	Tipo, Fonte string
}

// Radice e' la radice di uno STEP con il codice che il motore del cliente le riconosce.
type Radice struct {
	Codice, Rev string
	DiFamiglia  bool   // riconosciuta da una famiglia del cliente (altrimenti dall'estrattore generico)
	Famiglia    string // la famiglia che l'ha riconosciuta
	Dove        string // id, nome o descrizione del PRODUCT
	Testo       string // il testo grezzo da cui viene il codice
	RevDalFile  bool   // la rev e' quella del file (rev_grezza del nodo), non separata dalla famiglia
}

// fattiAnalisi sono le chiavi dei dettagli del worker che fanno evidenza.
type fattiAnalisi struct {
	TerminiTrovati     []string        `json:"termini_trovati"`
	ProductStep        string          `json:"product_step"`
	Struttura          json.RawMessage `json:"struttura"`
	TestoLetto         *int            `json:"testo_letto"`
	CodiceRiconosciuto *string         `json:"codice_riconosciuto"`
	ErrorePdf          string          `json:"errore_pdf"`
}

// Valuta legge un file per dimensione: tipo, codice e rev, ciascuna con le sue evidenze e lo score della
// regola che la sostiene (A5.14.3). Pura.
//
// Tipo: l'estensione, la nostra offerta «SO …» in uscita, i termini che il worker ha trovato nel testo di un
// PDF, lo STEP letto, il rumore (mai per un file tecnico, P14: il rumore sostituisce le altre letture, come
// prima, e un file che e' rumore non ha un codice), la risposta del fornitore.
//
// Codice: il nome di QUESTO file, tolti la rev e il suffisso decorativo del cliente (famiglia 70, generico
// 45), la radice dello STEP o, senza struttura letta, il primo PRODUCT (30); un nome che non e' un codice ma
// ne cita qualcuno lo registra senza valore. Un PRODUCT o una radice uguali al nome dipendono dal nome: non
// fanno una seconda fonte (C5) e non valgono piu' del nome (Domanda 7 = B, Componi). Di un PDF letto
// dall'analizzatore 4 contano anche il codice di famiglia del testo nativo in basso a destra della pagina 1
// (85) e il codice del titolo o del soggetto (40), con la stessa regola: il titolo uguale al nome dipende dal
// nome, il cartiglio uguale al nome lo conferma, il cartiglio diverso e' una discordanza; quello che ha letto
// l'OCR si registra come indizio, senza voto (evidenzeCodiceDelTesto, F9).
//
// Rev: dal nome con la regola del suo ramo (_REV_ del NAS 80, _REV/-R 55, _n 40), dal PRODUCT GREZZO (e non
// dall'esito del worker, che quando il PRODUCT non ha una rev gli mette quella del nome: K6, P10), dalla
// radice. Un file caricato a mano la dice e non la propone.
func Valuta(in IngressoFile) Valutazione {
	v := Valutazione{V: VersioneValutazione, Tabella: TabellaPunteggi, Da: in.Da}
	nome := strings.TrimSpace(in.NomeFile)
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(nome), "."))
	base := strings.TrimSuffix(nome, path.Ext(nome))
	var f fattiAnalisi
	if len(in.Fatti) > 0 {
		_ = json.Unmarshal(in.Fatti, &f)
	}

	// ---- tipo
	so := in.Direzione == "uscita" && ext == "pdf" && strings.HasPrefix(strings.ToUpper(nome), "SO ")
	if r := regolaRumore(in, ext, base); r != "" {
		// rumore: come prima, sostituisce la lettura del file; un logo non ha un codice
		v.Tipo = Componi([]Evidenza{evidenza(r, "rumore", "")})
		v.Codice, v.Rev = Componi(nil), Componi(nil)
		return v
	}
	et := tipoDaEstensione(ext)
	if so {
		et = append(et, evidenza("nome_so_uscita", "offerta_promatec", nome))
	}
	et = tipoDaAnalisi(et, ext, in, f)
	if in.RispostaFornitore {
		et = append(et, evidenza("risposta_fornitore", "offerta_fornitore", ""))
	}
	v.Tipo = Componi(et)

	// ---- codice
	nomeCod, nomeRev := CodiceRev(base)
	if sembraCodice(nomeCod) {
		nomeCod, nomeRev = in.Motore.Canonico(nomeCod, nomeRev)
	} else {
		nomeCod, nomeRev = "", ""
	}
	// un numero d'ordine nella forma del cliente («ODA_0001234.pdf») non e' il codice del file e non lo cita
	// (Smistamento 4.13b): e' l'ordine, come nel testo della mail
	if in.Motore.dentroUnOrdine(base, nomeCod) {
		nomeCod, nomeRev = "", ""
	}
	var ec, er []Evidenza
	if nomeCod != "" {
		e := evidenza("nome_codice_generico", nomeCod, nome)
		if fam := famigliaDi(in.Motore, nomeCod); fam != "" {
			e = evidenza("nome_codice_famiglia", nomeCod, nome)
			e.Famiglia = fam
		}
		ec = append(ec, e)
		if nomeRev != "" {
			er = append(er, evidenza(regolaRev(base), nomeRev, suffissoRev(base, nomeRev)))
		}
	} else {
		// un nome che non e' un codice ma ne cita: si registrano, non votano (restano `codici_nel_nome`). Il
		// testo e' il solo codice citato: il nome e' gia' il file, e ripeterlo in ogni evidenza porta una
		// valutazione oltre la misura di A5.14.2. Senza i numeri d'ordine del cliente, come `codici_nel_nome`
		// (CitatiNelNome, 4.13b)
		for _, c := range in.Motore.CitatiNelNome(nome, EstraiCodici(base)) {
			ec = append(ec, evidenza("nome_contiene_codice", "", c))
		}
	}
	dipende := func(e *Evidenza) {
		if nomeCod != "" && strings.EqualFold(e.Valore, nomeCod) {
			e.DipendeDa = "nome_file"
		}
	}
	switch {
	case in.Radice != nil && strings.TrimSpace(in.Radice.Codice) != "":
		r := in.Radice
		regola := "step_radice_generico"
		if r.DiFamiglia {
			regola = "step_radice_famiglia"
		}
		e := evidenza(regola, strings.ToUpper(strings.TrimSpace(r.Codice)), r.Testo)
		e.Famiglia, e.Dove = r.Famiglia, r.Dove
		dipende(&e)
		ec = append(ec, e)
		if rv := strings.ToUpper(strings.TrimSpace(r.Rev)); rv != "" {
			regolaR := "rev_step_product"
			switch {
			case r.RevDalFile:
				regolaR = "rev_step_nodo"
			case r.DiFamiglia:
				regolaR = "rev_famiglia_cliente"
			}
			e2 := evidenza(regolaR, rv, r.Testo)
			e2.Famiglia = r.Famiglia
			e2.DipendeDa = revDipende(rv, nomeCod, nomeRev, e.DipendeDa)
			er = append(er, e2)
		}
	case strings.TrimSpace(f.ProductStep) != "":
		ps := strings.TrimSpace(f.ProductStep)
		pc, pr := CodiceRev(ps)
		if !sembraCodice(pc) {
			pc, pr = strings.ToUpper(ps), ""
		}
		if sembraCodice(pc) {
			pc, pr = in.Motore.Canonico(pc, pr)
			e := evidenza("step_primo_product", pc, "PRODUCT('"+ps+"')")
			dipende(&e)
			ec = append(ec, e)
			if pr != "" {
				// la rev separata dal PRODUCT grezzo: se il PRODUCT non ne ha, non ne ha (K6)
				e2 := evidenza("rev_step_product", pr, "PRODUCT('"+ps+"')")
				e2.DipendeDa = revDipende(pr, nomeCod, nomeRev, e.DipendeDa)
				er = append(er, e2)
			}
		}
	}
	if ext == "pdf" {
		// il testo del PDF (F9): i codici li cerca qui il motore del cliente, non il worker
		ec = append(ec, evidenzeCodiceDelTesto(in.Motore, in.Fatti, nome)...)
	}
	v.Codice = Componi(ec)

	// ---- rev
	if in.Interno {
		// un file caricato a mano non inventa una revisione del cliente: la si dice e basta
		if d := Componi(er); d.Valore != "" {
			v.Trattenuta = d.Valore
			er = []Evidenza{evidenza("rev_trattenuta_interno", "", "rev "+d.Valore+" letta nel file caricato a mano, non del cliente")}
		}
	}
	v.Rev = Componi(er)
	return v
}

// revDipende: una rev letta nello STEP e' la stessa lettura del nome quando il codice dipende dal nome e la
// rev e' uguale.
func revDipende(rev, nomeCod, nomeRev, codiceDipende string) string {
	if codiceDipende != "" && nomeCod != "" && strings.EqualFold(rev, nomeRev) {
		return "nome_file"
	}
	return ""
}

// regolaRumore e' la regola di rumore che vale per il file, o "". Mai per un file tecnico (P14): un file che
// per il suo formato e' un disegno, o un PDF di cui non si sa ancora che cosa sia, non sparisce perche' un
// giorno qualcuno ha scartato lo stesso contenuto (A5.4.8: un file di cui non si sa che cosa sia non lo
// dichiara non tecnico nessuno).
func regolaRumore(in IngressoFile, ext, base string) string {
	r := in.Rumore
	if r == "" && EstImmagine(ext) && (in.Bytes < 100*1024 || rePrefissiNonCodice.MatchString(strings.ToUpper(base))) {
		r = "rumore_immagine"
	}
	if r == "" || possibileTecnico(ext) {
		return ""
	}
	return r
}

// possibileTecnico: il formato e' un disegno (3D, DWG, DXF) o un PDF/TIF, che puo' esserlo.
func possibileTecnico(ext string) bool {
	switch ext {
	case "pdf", "tif", "tiff":
		return true
	}
	for _, e := range tipoDaEstensione(ext) {
		switch e.Valore {
		case "cad_3d", "disegno_2d", "sviluppo_dxf":
			return true
		}
	}
	return false
}

// tipoDaAnalisi aggiunge (o sostituisce) le evidenze del tipo che vengono dalla lettura del worker. Contano
// solo quelle del CONTENUTO: i termini trovati nel testo di un PDF (l'esito con fonte `cartiglio`), il testo
// letto senza termini, il PDF che non si apre, lo STEP letto. Il resto dell'esito (DXF, DWG, fogli, altro) e'
// l'estensione, che si e' gia' letta qui.
func tipoDaAnalisi(et []Evidenza, ext string, in IngressoFile, f fattiAnalisi) []Evidenza {
	pdf := ext == "pdf" || ext == "tif" || ext == "tiff"
	termini := strings.Join(f.TerminiTrovati, ", ")
	switch {
	case f.ErrorePdf != "":
		return []Evidenza{evidenza("pdf_illeggibile", "", "")}
	case in.Esito != nil && in.Esito.Fonte == "cartiglio":
		var e Evidenza
		switch in.Esito.Tipo {
		case "offerta_promatec":
			nomeSO := strings.HasPrefix(strings.ToUpper(strings.TrimSpace(in.NomeFile)), "SO ")
			switch {
			case termini == "" && nomeSO && in.Direzione == "uscita":
				// il worker l'ha detta offerta per il nome «SO …», che qui si e' gia' letto (nome_so_uscita)
				return et
			case in.Direzione == "entrata" && !nomeSO:
				// un'offerta che arriva e' un documento commerciale di chi la manda
				e = evidenza("pdf_termini_offerta_entrata", "commerciale", termini)
			default:
				e = evidenza("pdf_termini_offerta", "offerta_promatec", termini)
			}
		case "disegno_2d":
			e = evidenza("pdf_termini_cartiglio", "disegno_2d", termini)
		case "capitolato":
			e = evidenza("pdf_termini_capitolato", "capitolato", termini)
		case "distinta_cliente":
			e = evidenza("pdf_termini_distinta", "distinta_cliente", termini)
		default:
			return et
		}
		out := []Evidenza{e}
		for _, x := range et {
			if x.Valore != "" { // la nostra «SO …» resta accanto; il «PDF non ancora letto» no
				out = append(out, x)
			}
		}
		return out
	case pdf && (f.TestoLetto != nil || f.CodiceRiconosciuto != nil):
		// un PDF senza testo lo dice, con quello che ha fatto l'OCR (F9): non e' che il testo non parli, e' che
		// non c'e'. E un PDF il cui testo non e' stato letto (fatti di prima della 4, o di un worker non
		// aggiornato) dice «da rianalizzare»
		testo := ""
		switch t, stato := testoDelPDF(in.Fatti); stato {
		case TestoAssente:
			testo = FraseTestoPDF(stato, t.OCR.Stato)
		case TestoNonLetto:
			testo = FraseTestoPDF(stato, "")
		}
		return []Evidenza{evidenza("pdf_nessun_termine", "", testo)}
	case (ext == "stp" || ext == "step") && (len(f.Struttura) > 0 && !bytes.Equal(f.Struttura, []byte("null")) || f.ProductStep != ""):
		return append(et, evidenza("step_letto", "cad_3d", testoStruttura(f.Struttura)))
	}
	return et
}

// testoStruttura: «18 PRODUCT, 22 relazioni», se la struttura si legge.
func testoStruttura(s json.RawMessage) string {
	var st struct {
		Nodi      []json.RawMessage `json:"nodi"`
		Relazioni []json.RawMessage `json:"relazioni"`
	}
	if len(s) == 0 || json.Unmarshal(s, &st) != nil || len(st.Nodi) == 0 {
		return ""
	}
	return fmt.Sprintf("%d PRODUCT, %d relazioni", len(st.Nodi), len(st.Relazioni))
}

// famigliaDi e' la famiglia del cliente che riconosce il codice INTERO, o "": una famiglia che trova un
// codice dentro («X_PRT» contiene X) non fa del nome un codice di famiglia.
func famigliaDi(m *Motore, codice string) string {
	for _, f := range DiFamiglia(m.Codici(codice)) {
		if strings.EqualFold(f.Codice, codice) {
			return f.Famiglia
		}
	}
	return ""
}

// ConRispostaFornitore e' la valutazione dopo il gesto «e' la risposta del fornitore» (A5.14.7): al tipo si
// aggiunge l'evidenza `risposta_fornitore`, e le altre restano. Vince se nessuna lettura del contenuto e' piu'
// forte: la mail e' un'offerta, ma il singolo PDF puo' essere il disegno rimandato (pdf_termini_cartiglio 75
// contro 70), e allora il tipo e' discorde e nessuno lo precompila (U7).
func (v Valutazione) ConRispostaFornitore() Valutazione {
	if v.HaEvidenza("risposta_fornitore") {
		return v
	}
	v.Tipo = Componi(append(append([]Evidenza(nil), v.Tipo.Evidenze...), evidenza("risposta_fornitore", "offerta_fornitore", "")))
	v.Da, v.Ricostruita = DaRispostaFornitore, false
	return v
}

// HaEvidenza dice se una dimensione della valutazione ha un'evidenza della regola.
func (v Valutazione) HaEvidenza(regola string) bool {
	for _, d := range []Dimensione{v.Tipo, v.Codice, v.Rev} {
		for _, e := range d.Evidenze {
			if e.Regola == regola {
				return true
			}
		}
	}
	return false
}

// StesseLetture dice se due valutazioni dicono le stesse cose (dimensioni ed evidenze), a prescindere da chi
// le ha calcolate e quando.
func (v Valutazione) StesseLetture(w Valutazione) bool {
	a, _ := json.Marshal([]Dimensione{v.Tipo, v.Codice, v.Rev})
	b, _ := json.Marshal([]Dimensione{w.Tipo, w.Codice, w.Rev})
	return bytes.Equal(a, b)
}

// ---------------------------------------------------------------- il riepilogo in colonna

// Riepilogo sono le colonne di documento_proposta che la valutazione riassume (A5.14.3).
type Riepilogo struct {
	Tipo       string // tipo_proposto
	Codice     string
	Rev        string
	Confidenza int    // lo score della lettura in colonna del codice; senza codice, quello del tipo
	Fonte      string // la fonte della regola di quella lettura
}

// Riepilogo e' la sola definizione delle colonne (A5.14.3):
//   - tipo_proposto: il valore del tipo, `da_determinare` se non c'e';
//   - codice e rev: i valori delle dimensioni; ma se il codice e' DISCORDE resta la lettura del nome del file
//     stesso, quando c'e' (con la sua rev): e' quella che il Fascicolo di oggi confronta, e cosi' la radice di
//     famiglia di uno STEP non riscrive la colonna (niente E04 sotto un altro nome, D49). Una rev discorde con
//     il codice concorde tiene anche lei la lettura del nome;
//   - confidenza: lo score della lettura del codice in colonna se c'e' un codice, altrimenti quello del tipo;
//   - fonte: la fonte della regola di quella stessa lettura (`estensione` per un tipo senza evidenza).
//
// E' cio' che l'unico lettore automatico della colonna gia' intende (`v_codici_candidati_thread` → «Codici
// visti»): lo score del CODICE, con la sua fonte. Una decisione di una persona resta la sua: operatore, 100.
func (v Valutazione) Riepilogo() Riepilogo {
	r := Riepilogo{Tipo: v.Tipo.Valore}
	if r.Tipo == "" {
		r.Tipo = "da_determinare"
	}
	if v.Decisa() {
		r.Codice, r.Rev, r.Confidenza, r.Fonte = v.Codice.Valore, v.Rev.Valore, 100, RegolaOperatore
		return r
	}
	cod, dalNome := letturaInColonna(v.Codice, prefissiNomeCodice)
	switch {
	case dalNome:
		if rv, ok := letturaDelNome(v.Rev, regoleRevNome); ok {
			r.Rev = rv.Valore
		}
	default:
		if rv, _ := letturaInColonna(v.Rev, regoleRevNome); rv.Valore != "" {
			r.Rev = rv.Valore
		}
	}
	if cod.Valore != "" {
		r.Codice, r.Confidenza, r.Fonte = cod.Valore, cod.Score, cod.Fonte
		return r
	}
	r.Confidenza, r.Fonte = 0, "estensione"
	if t, _ := letturaInColonna(v.Tipo, nil); t.Valore != "" {
		r.Confidenza, r.Fonte = t.Score, t.Fonte
	}
	return r
}

// Le letture del nome del file, per il riepilogo di una dimensione discorde.
var (
	prefissiNomeCodice = []string{"nome_codice_famiglia", "nome_codice_generico"}
	regoleRevNome      = []string{"rev_nas", "rev_esplicita_nome", "rev_suffisso_nome"}
)

// letturaInColonna e' l'evidenza che va in colonna per una dimensione: la vincente, o con la dimensione
// discorde la lettura del nome (dalNome = true) se ce n'e' una.
func letturaInColonna(d Dimensione, delNome []string) (Evidenza, bool) {
	if d.Stato == StatoDiscorde {
		if e, ok := letturaDelNome(d, delNome); ok {
			return e, true
		}
	}
	for _, e := range d.Evidenze {
		if e.Regola == d.Regola && e.Valore != "" && strings.EqualFold(e.Valore, d.Valore) {
			return e, false
		}
	}
	return Evidenza{}, false
}

// letturaDelNome e' la prima evidenza con valore di una delle regole del nome.
func letturaDelNome(d Dimensione, regole []string) (Evidenza, bool) {
	for _, e := range d.Evidenze {
		for _, r := range regole {
			if e.Regola == r && e.Valore != "" {
				return e, true
			}
		}
	}
	return Evidenza{}, false
}

// ConValutazione mette la valutazione nei dettagli di una riga di documento_proposta, senza toccare le altre
// chiavi, e restituisce le colonne dal suo riepilogo. E' la sola strada per cui chi scrive una lettura arriva
// alle colonne (prova 229): nessuno scrittore mette una confidenza sua. `rev_letta` (la rev trattenuta di un
// file caricato a mano) resta accanto, con il nome di sempre.
func ConValutazione(dettagli json.RawMessage, v Valutazione, ora time.Time) (json.RawMessage, Riepilogo) {
	v.CalcolataIl = ora.UTC().Format(time.RFC3339)
	m := map[string]any{}
	if len(dettagli) > 0 {
		dec := json.NewDecoder(bytes.NewReader(dettagli))
		dec.UseNumber()
		if err := dec.Decode(&m); err != nil || m == nil {
			m = map[string]any{"dettagli_grezzi": string(dettagli)}
		}
	}
	m["valutazione"] = v
	if v.Trattenuta != "" {
		m["rev_letta"] = v.Trattenuta
	}
	out, err := json.Marshal(m)
	if err != nil {
		out = dettagli
	}
	return out, v.Riepilogo()
}

// ValutazioneDellaRiga e' la valutazione di una riga di documento_proposta: `dettagli.valutazione` quando
// c'e', altrimenti ricostruita dalle colonne (ValutazioneDaRiga). Una riga con fonte `operatore` e' una
// decisione, e si legge dalle colonne anche se porta la lettura della macchina su cui si e' deciso.
func ValutazioneDellaRiga(tipo, codice, rev, fonte string, confidenza int, dettagli []byte, nomeFile, estensione string) Valutazione {
	if fonte != RegolaOperatore {
		if v, ok := LeggiValutazione(dettagli); ok {
			return v
		}
	}
	return ValutazioneDaRiga(tipo, codice, rev, fonte, confidenza, dettagli, nomeFile, estensione)
}
