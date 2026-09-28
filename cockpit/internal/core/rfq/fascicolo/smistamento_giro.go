package fascicolo

// Il flusso ancorato al prodotto, passi 5 e 6 (Smistamento F8, addendum A5.13.5-A5.13.6, A5.14.3-A5.14.4): il
// secondo giro sui file della RFQ e la destinazione proposta di ciascuno.
//
// documento_proposta descrive la LETTURA del file (tipo, codice, rev: la valutazione, A5.14): il flusso non la
// riscrive. Qui si AGGIUNGE la destinazione, usando tutto il contesto della RFQ: per ogni codice che il file
// dice (il suo nome, la radice del suo STEP, il suo contenuto) le voci dell'indice con quel codice, con le
// regole di destinazione della tabella S1 (nessuno score nuovo o cambiato, Domanda 3 = A). Le evidenze
// ORDINANO: la preselezione dipende dallo stato, mai dallo score (U7), e non c'e' mai con una discordanza,
// con due candidati diversi, con un'ancora da confermare, con un nodo da accettare (scelta 1), verso un pezzo
// che nessuno ha deciso (da_rivedere) o per una destinazione che si regge solo sul nome del file (scelta 2).
//
// Il nome del file e' l'evidenza piu' debole (decisioni del 27/09 ter). Un codice letto nella struttura di uno
// STEP o nel cartiglio di un PDF e' un'evidenza indipendente dal nome: quando il nome dice un'altra cosa la
// discordanza si mostra con le fonti di ciascun codice, il candidato sostenuto dal contenuto viene PRIMO, e
// niente e' preselezionato. Il nome del file non si corregge mai, e resta quello che e'.
//
// Senza un riferimento strutturale affidabile (un prodotto senza ancora) le destinazioni TECNICHE si
// fermano: il file resta «da verificare», anche quando ha una pre-assegnazione di prima (che si dice fra le
// evidenze). I documenti non tecnici ricevono comunque il «documento generale», preselezionabile solo se il
// tipo viene dal contenuto (Domanda 4 = B).

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
)

// VersioneDestinazione e' la forma di `dettagli.destinazione`.
const VersioneDestinazione = 1

// MaxCandidati: una destinazione ne mostra al piu' tanti (A5.13.6).
const MaxCandidati = 8

// Gli esiti di una destinazione (A5.13.5).
const (
	EsitoProposta    = "proposta"     // almeno un candidato, anche bloccato
	EsitoInAttesa    = "in_attesa"    // «In elaborazione»: un'analisi o un'ancora devono ancora arrivare
	EsitoSospesa     = "sospesa"      // «Da verificare»: l'automatismo si ferma, con il motivo
	EsitoNessuna     = "nessuna"      // «Da verificare — nessun pezzo corrispondente»
	EsitoFuoriFlusso = "fuori_flusso" // il file non entra nel flusso, con il motivo
)

// I motivi di un esito senza candidati.
const (
	MotivoManca          = "manca_riferimento_strutturale"
	MotivoNessunProdotto = "nessun_prodotto_confermato"
	MotivoRadiceLontana  = "radice_non_raggiungibile"
	MotivoArchivio       = "archivio_contenitore"
	MotivoRumore         = "rumore"
	MotivoNonDelCliente  = "non_del_cliente"
	MotivoEscluso        = "escluso"
)

// Le chiavi del catalogo delle discordanze che il flusso usa (A5.5). Stabili: la schermata (F10) e la
// calibrazione le leggono.
const (
	DiscLetteraFinale      = "lettera_finale"
	DiscSuffisso           = "suffisso"
	DiscRevFonti           = "rev_fonti"
	DiscFontiDiverse       = "fonti_diverse"
	DiscRadiceDiversa      = "radice_diversa"
	DiscPiuRadici          = "piu_radici"
	DiscAncoraDaConfermare = "ancora_da_confermare"
	DiscAncoreConcorrenti  = "ancore_concorrenti"
	DiscStepNonAutorizzato = "step_non_autorizzato"
	DiscNessunaAutorita    = "nessuna_autorita"
	DiscStrutturaDiversa   = "struttura_diversa"
	DiscQuasiUguale        = "quasi_uguale"
	DiscPreassegnato       = "preassegnato_altrove"
	DiscTipoDiverso        = "tipo_diverso"
	DiscBomCongelata       = "bom_congelata"
)

// Le fonti di un codice del file: da dove il flusso l'ha letto.
const (
	FonteNome        = "nome_file"
	FonteNomeCitato  = "nome_citato"
	FonteStepRadice  = "step_radice"
	FonteStepProduct = "step_product"
	FonteCartiglio   = "cartiglio_pdf"
	FonteTestoPDF    = "testo_pdf"
	FonteMetadatiPDF = "metadati_pdf"
	FonteOCR         = "ocr_pdf"
	FontePrecedente  = "assegnazione_precedente"
)

// Che cosa sostiene un candidato oltre al nome del file (scelta 2). Per la preselezione serve almeno una
// delle evidenze ammesse (sostegnoBasta): il contenuto del file, un prodotto confermato della RFQ, uno STEP
// (autorizzato, o quello che ancora il pezzo). «componente_deciso» si mostra ma da solo non basta: un
// componente aggiunto a mano sotto il prodotto non lega il file al pezzo, lo lega solo il nome.
const (
	SostegnoContenuto       = "contenuto"         // il codice e' scritto dentro il file
	SostegnoProdotto        = "prodotto_rfq"      // il bersaglio e' un prodotto confermato della RFQ
	SostegnoComponente      = "componente_deciso" // il bersaglio e' un componente deciso da una persona
	SostegnoStepAutorizzato = "step_autorizzato"  // il bersaglio e' nell'autorita' di uno STEP autorizzato
	SostegnoNodoAncora      = "nodo_dell_ancora"  // il codice e' un nodo dello STEP che ancora il pezzo sopra
)

// sostegnoBasta dice se fra i sostegni c'e' un'evidenza che basta alla preselezione (scelta 2: STEP, testo
// del PDF, prodotto della RFQ).
func sostegnoBasta(ss []string) bool {
	for _, s := range ss {
		switch s {
		case SostegnoContenuto, SostegnoProdotto, SostegnoStepAutorizzato, SostegnoNodoAncora:
			return true
		}
	}
	return false
}

// Destinazione e' `dettagli.destinazione` (v1, A5.13.6).
type Destinazione struct {
	V                   int           `json:"v"`
	Tabella             string        `json:"tabella"`
	Algoritmo           string        `json:"algoritmo"`
	Lettura             LetturaDest   `json:"lettura"`
	IndiceFirma         string        `json:"indice_firma"`
	Firma               string        `json:"firma"`
	Esito               string        `json:"esito"`
	Motivo              string        `json:"motivo,omitempty"`
	Stato               string        `json:"stato,omitempty"` // preselezionabile | da_scegliere | bloccata
	Preselezionabile    bool          `json:"preselezionabile"`
	Candidati           []Candidato   `json:"candidati"`
	Codici              []CodiceLetto `json:"codici,omitempty"` // i codici che il file dice, con le fonti
	Discordanze         []string      `json:"discordanze,omitempty"`
	Evidenze            []string      `json:"evidenze,omitempty"`
	ProdottiSenzaAncora []string      `json:"prodotti_senza_ancora,omitempty"`
	Ancore              []AncoraDest  `json:"ancore,omitempty"`
}

// LetturaDest e' la lettura del file su cui la destinazione e' stata calcolata: se la valutazione cambia dopo,
// chi legge la dichiara superata.
type LetturaDest struct {
	Tipo   string `json:"tipo"`
	Codice string `json:"codice"`
}

// CodiceLetto e' un codice che il file dice, con le sue fonti.
type CodiceLetto struct {
	Codice       string   `json:"codice"`
	Fonti        []string `json:"fonti"`
	DalContenuto bool     `json:"dal_contenuto"`
}

// Candidato e' una destinazione possibile del file.
type Candidato struct {
	Rango int `json:"rango"`
	// Chiave: componente:<uuid>, nodo:<proposta_id>, identificativo:<CODICE>, generale.
	Chiave       string         `json:"chiave"`
	Bersaglio    string         `json:"bersaglio"`
	Ruolo        string         `json:"ruolo,omitempty"`
	Codice       string         `json:"codice,omitempty"`
	Portatore    *PortatoreDest `json:"portatore,omitempty"`
	Posizioni    []Posizione    `json:"posizioni,omitempty"`
	Score        int            `json:"score"`
	Regola       string         `json:"regola"`
	Bloccato     string         `json:"bloccato,omitempty"`
	DalContenuto bool           `json:"dal_contenuto"`
	Fonti        []string       `json:"fonti"`
	Sostegno     []string       `json:"sostegno,omitempty"`
	Evidenze     []string       `json:"evidenze"`
	Discordanze  []string       `json:"discordanze,omitempty"`
	// DaRivedere: il bersaglio e' un pezzo che nessuno ha deciso (Voce.DaRivedere): mai preselezionato.
	DaRivedere bool   `json:"da_rivedere,omitempty"`
	livello    string // il livello dell'ancora da cui dipende
}

// PortatoreDest e' il nodo dello STEP che porta il bersaglio.
type PortatoreDest struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	File       string    `json:"file"`
	Chiave     string    `json:"chiave"`
}

// AncoraDest e' l'ancora di un prodotto come la fotografa ogni destinazione.
type AncoraDest struct {
	Prodotto string   `json:"prodotto"`
	Livello  string   `json:"livello"`
	File     []string `json:"file,omitempty"`
	Regole   []string `json:"regole,omitempty"`
	Motivi   []string `json:"motivi,omitempty"`
}

// Ambito dice su quali file il flusso scrive: tutta la RFQ, o solo alcuni file con l'indice corrente.
type Ambito struct {
	Rfq  bool
	File []uuid.UUID
}

// AmbitoRfq e' tutta la RFQ.
func AmbitoRfq() Ambito { return Ambito{Rfq: true} }

// AmbitoFile sono i soli file dati.
func AmbitoFile(ids ...uuid.UUID) Ambito { return Ambito{File: ids} }

func (a Ambito) contiene(id uuid.UUID) bool {
	if a.Rfq {
		return true
	}
	for _, x := range a.File {
		if x == id {
			return true
		}
	}
	return false
}

// Calcolo e' tutto il flusso su uno stato: i prodotti, le ancore, l'indice, le destinazioni.
type Calcolo struct {
	Prodotti     []Prodotto
	Ancore       map[string]Ancora
	Indice       Indice
	Destinazioni map[uuid.UUID]Destinazione
}

// Calcola esegue i passi 1-5 sullo stato. Pura: stesso stato, stesse destinazioni, qualunque sia l'ordine in
// cui gli eventi l'hanno prodotto (FP7).
func Calcola(s *StatoFlusso, ambito Ambito) Calcolo {
	p := ProdottiDellaRfq(s)
	a := Ancore(p, s)
	ix := IndiceCodici(p, a, s)
	return Calcolo{Prodotti: p, Ancore: a, Indice: ix, Destinazioni: SecondoGiro(ix, s, ambito)}
}

// SecondoGiro calcola la destinazione di ogni file dell'ambito che ha una proposta (A5.13.5). Pura.
func SecondoGiro(ix Indice, s *StatoFlusso, ambito Ambito) map[uuid.UUID]Destinazione {
	d := s.derivati()
	out := map[uuid.UUID]Destinazione{}
	for _, f := range s.File {
		if f.Proposta == nil || !ambito.contiene(f.AllegatoID) {
			continue
		}
		out[f.AllegatoID] = d.destinazione(ix, f)
	}
	return out
}

// ------------------------------------------------------------------ la destinazione di un file

// lettura e' un codice che il file dice.
type lettura struct {
	codice    string
	fonti     []string
	contenuto bool // almeno una fonte dentro il file, indipendente dal nome
	radice    bool // e' la radice (unica) dello STEP stesso
	citato    bool // solo citato (nel nome, o nel corpo del PDF): una chiave di ricerca, non il codice del file
	testi     []string
}

func (d *derivati) destinazione(ix Indice, f FileFlusso) Destinazione {
	v := f.Proposta.Valutazione
	dest := Destinazione{V: VersioneDestinazione, Tabella: classificazione.TabellaPunteggi, Algoritmo: AlgoritmoFlusso,
		Lettura: LetturaDest{Tipo: v.Tipo.Valore, Codice: v.Codice.Valore}, IndiceFirma: ix.Firma, Candidati: []Candidato{}}
	dest.Ancore = ancoreDest(ix)
	switch {
	case f.Proposta.Esclusa:
		dest.Esito, dest.Motivo = EsitoFuoriFlusso, MotivoEscluso
	case f.Contenitore:
		dest.Esito, dest.Motivo = EsitoFuoriFlusso, MotivoArchivio
		dest.Evidenze = []string{"è un archivio: le sue voci si smistano una per una"}
	case v.Tipo.Valore == "rumore":
		dest.Esito, dest.Motivo = EsitoFuoriFlusso, MotivoRumore
	case !f.DelCliente:
		dest.Esito, dest.Motivo = EsitoFuoriFlusso, MotivoNonDelCliente
		dest.Evidenze = []string{"il file non viene dal cliente (un fornitore, o una nostra mail in uscita): resta fuori dal flusso"}
	case nonTecnico(v.Tipo.Valore):
		d.destinazioneGenerale(&dest, f)
	default:
		d.destinazioneTecnica(&dest, ix, f)
	}
	return firmata(dest)
}

// nonTecnico: i tipi che non sono un disegno ne' un modello (un tipo non riconosciuto resta nella strada dei
// tecnici: Domanda 4 = B ferma anche quelli).
func nonTecnico(tipo string) bool {
	switch tipo {
	case "capitolato", "distinta_cliente", "commerciale", "offerta_fornitore", "offerta_promatec", "ordine_cliente", "corrispondenza":
		return true
	}
	return false
}

// tipoDalContenuto: il tipo viene dal contenuto del file (i termini nel testo del PDF, lo STEP letto), non dal
// solo formato o dal nome.
func tipoDalContenuto(v classificazione.Valutazione) bool {
	switch classificazione.Punteggi[v.Tipo.Regola].Fonte {
	case "cartiglio", "step":
		return true
	}
	return false
}

// destinazioneGenerale: un documento non tecnico va fra i documenti generali della RFQ (G1). Anche quando i
// prodotti non hanno un riferimento strutturale (Domanda 4 = B). Preselezionabile solo con il tipo dal
// contenuto, non discorde, e senza un altro candidato.
func (d *derivati) destinazioneGenerale(dest *Destinazione, f FileFlusso) {
	v := f.Proposta.Valutazione
	c := Candidato{Chiave: "generale", Bersaglio: "generale", Fonti: []string{}}
	if tipoDalContenuto(v) {
		c.Regola, c.DalContenuto, c.Sostegno = "dest_generale_contenuto", true, []string{SostegnoContenuto}
		c.Evidenze = []string{fmt.Sprintf("il contenuto dice %s (%s)", v.Tipo.Valore, parole(v.Tipo.Regola))}
	} else {
		c.Regola = "dest_generale_estensione"
		c.Evidenze = []string{fmt.Sprintf("il tipo %s viene solo da %s: la proposta non è preselezionata", v.Tipo.Valore, parole(v.Tipo.Regola))}
	}
	c.Score = classificazione.Punteggi[c.Regola].Score
	cands := []Candidato{c}
	if p, ok := d.precedente(f); ok {
		cands = append(cands, p)
	}
	var disc []string
	if v.Tipo.Stato == classificazione.StatoDiscorde {
		disc = append(disc, DiscTipoDiverso)
	}
	d.chiudiDestinazione(dest, cands, disc)
}

// destinazioneTecnica: i codici del file contro l'indice (A5.13.5).
func (d *derivati) destinazioneTecnica(dest *Destinazione, ix Indice, f FileFlusso) {
	v := f.Proposta.Valutazione
	letture, radici := d.codiciDelFile(f)
	var disc, evidenze []string
	distinti := 0
	for _, l := range letture {
		if l.citato {
			// un codice solo citato conta se e' un pezzo dell'indice: altrimenti e' una quota, una norma, un
			// numero qualunque, e non si dice nemmeno (P33)
			if x, ok := ix.Voci[l.codice]; !ok || x.Autorita == AutoritaScollegata {
				continue
			}
		} else {
			distinti++
		}
		dest.Codici = append(dest.Codici, CodiceLetto{Codice: l.codice, Fonti: l.fonti, DalContenuto: l.contenuto})
		evidenze = append(evidenze, l.testi...)
	}
	if distinti > 1 || v.Codice.Stato == classificazione.StatoDiscorde {
		// il nome, la radice dello STEP, il cartiglio dicono codici diversi: si mostrano tutti, con le fonti, e
		// nessuno vince da solo (U7). Il nome del file non si corregge
		disc = append(disc, DiscFontiDiverse)
	}
	if len(radici) > 1 {
		disc = append(disc, DiscPiuRadici)
	}
	if v.Tipo.Stato == classificazione.StatoDiscorde {
		disc = append(disc, DiscTipoDiverso)
	}
	if v.Rev.Stato == classificazione.StatoDiscorde {
		disc = append(disc, DiscRevFonti)
	}
	if f.pdf() && len(f.EvidenzePDF) == 0 {
		// oggi nessun PDF ha evidenze dal contenuto (la lettura strutturata e' di F9): il PDF si comporta come un
		// PDF senza testo, e il suo codice viene solo dal nome
		evidenze = append(evidenze, "il PDF non ha evidenze dal contenuto (testo o cartiglio): il suo codice viene solo dal nome")
	}

	var cands []Candidato
	for _, l := range letture {
		voce, ok := ix.Voci[l.codice]
		switch {
		case ok && voce.Autorita == AutoritaScollegata:
			if len(voce.Nodi) > 0 {
				evidenze = append(evidenze, fmt.Sprintf("%s compare nello STEP %s, non collegato a nessun prodotto ancorato", l.codice, voce.Nodi[0].File))
			}
		case ok:
			cands = unisci(cands, d.candidato(voce, l))
		}
		if l.citato {
			continue
		}
		// i codici quasi uguali (P4): bloccati, con le due letture «è X» / «è un pezzo diverso»
		for _, k := range ix.codici() {
			w := ix.Voci[k]
			if w.Autorita == AutoritaScollegata {
				continue
			}
			if motivo := vicino(l.codice, w.Codice, d.m); motivo != "" {
				cands = unisci(cands, d.candidatoVicino(w, l, motivo))
			}
		}
	}
	pre, conPre := d.precedente(f)
	if conPre && len(cands) > 0 {
		cands = unisci(cands, pre)
	}
	congela := func(cc []Candidato) {
		if d.s.BomCongelata {
			for i := range cc {
				if cc[i].Chiave != "generale" {
					cc[i].Discordanze = aggiungi(cc[i].Discordanze, DiscBomCongelata)
				}
			}
		}
	}
	congela(cands)
	dest.Evidenze = evidenze
	if len(cands) > 0 {
		d.chiudiDestinazione(dest, cands, disc)
		return
	}
	dest.Discordanze = ordinate(disc)

	// nessun candidato: perche'
	var senza, attesa []string
	for _, p := range ix.Prodotti {
		switch ix.Ancore[p.Codice].Livello {
		case LivelloInAttesa:
			attesa = append(attesa, p.Codice)
		case LivelloAssente:
			senza = append(senza, p.Codice)
		}
	}
	for _, l := range letture {
		for _, p := range senza {
			if l.codice == p && contiene(l.fonti, FonteNome) {
				dest.Evidenze = append(dest.Evidenze, fmt.Sprintf("il nome dice %s, prodotto della RFQ senza un riferimento strutturale", p))
			}
		}
	}
	switch {
	case f.Analisi == AnalisiInCorso || len(attesa) > 0:
		dest.Esito = EsitoInAttesa
		if len(attesa) > 0 {
			dest.Evidenze = append(dest.Evidenze, "si aspetta lo STEP di "+strings.Join(attesa, ", "))
		}
	case len(ix.Prodotti) == 0:
		dest.Esito, dest.Motivo = EsitoSospesa, MotivoNessunProdotto
		dest.Evidenze = append(dest.Evidenze, "la RFQ non ha un prodotto confermato da una persona: nessuna destinazione tecnica")
	case f.step() && ix.Scollegati[f.Sha]:
		dest.Esito, dest.Motivo = EsitoSospesa, MotivoRadiceLontana
		dest.Evidenze = append(dest.Evidenze, "la radice di questo STEP non si raggiunge da nessun prodotto ancorato")
	case len(senza) > 0:
		dest.Esito, dest.Motivo, dest.ProdottiSenzaAncora = EsitoSospesa, MotivoManca, senza
		for _, p := range senza {
			dest.Evidenze = append(dest.Evidenze, ix.Ancore[p].Motivi...)
		}
	case conPre:
		// tutti i prodotti hanno il loro riferimento e nessun codice del file e' un pezzo: la pre-assegnazione
		// di prima resta l'unico candidato, mai preselezionato
		cands = []Candidato{pre}
		congela(cands)
		d.chiudiDestinazione(dest, cands, disc)
		return
	default:
		dest.Esito = EsitoNessuna
		dest.Evidenze = append(dest.Evidenze, "nessun codice del file è un pezzo della RFQ")
	}
	if conPre {
		// l'automatismo e' fermo (Domanda 4 = B): la pre-assegnazione di prima non diventa una destinazione
		// tecnica proposta, e si dice fra le evidenze
		dest.Evidenze = append(dest.Evidenze, pre.Evidenze...)
	}
}

// codiciDelFile sono i codici che il file dice (P15: il SUO nome, mai quello di un'altra copia): il nome, la
// radice del suo STEP (dalle righe della struttura, con le correzioni dell'operatore; se non ci sono, le
// evidenze step_* della valutazione), le evidenze dal contenuto del PDF. Una lettura del contenuto uguale al
// nome dipende dal nome (Domanda 7 = B): resta, ma non fa del codice un codice «dal contenuto».
func (d *derivati) codiciDelFile(f FileFlusso) ([]lettura, []string) {
	per := map[string]*lettura{}
	var ordine []string
	agg := func(codice, fonte string, contenuto, radice, citato bool, testo string) {
		codice = d.canonico(codice)
		if codice == "" {
			return
		}
		l, ok := per[codice]
		if !ok {
			l = &lettura{codice: codice, citato: true}
			per[codice] = l
			ordine = append(ordine, codice)
		}
		if !contiene(l.fonti, fonte) {
			l.fonti = append(l.fonti, fonte)
		}
		l.contenuto = l.contenuto || contenuto
		l.radice = l.radice || radice
		l.citato = l.citato && citato
		if testo != "" && !contiene(l.testi, testo) {
			l.testi = append(l.testi, testo)
		}
	}
	nome, citati := codiciDelNome(f.Nome, d.m)
	if nome != "" {
		agg(nome, FonteNome, false, false, false, "il nome dice "+nome)
	}
	for _, c := range citati {
		agg(c, FonteNomeCitato, false, false, true, "il nome cita "+c)
	}
	var radici []string
	if st := d.strutture[f.Sha]; f.step() && st != nil {
		radici = d.codiciRadici(st)
		for _, r := range st.Radici {
			c := d.codiceNodo(st.Nodi[r])
			if c == "" {
				continue
			}
			testo := "la radice dello STEP dice " + c
			if c == nome {
				testo += ": ripete il nome del file, non è una seconda fonte"
			}
			agg(c, FonteStepRadice, c != nome, len(st.Radici) == 1, false, testo)
		}
	} else if f.Proposta != nil {
		for _, e := range f.Proposta.Valutazione.Codice.Evidenze {
			if e.Valore == "" {
				continue
			}
			switch e.Regola {
			case "step_radice_famiglia", "step_radice_generico":
				agg(e.Valore, FonteStepRadice, e.DipendeDa == "" && d.canonico(e.Valore) != nome, f.step(), false, "la radice dello STEP dice "+e.Valore)
			case "step_primo_product":
				agg(e.Valore, FonteStepProduct, e.DipendeDa == "" && d.canonico(e.Valore) != nome, false, false, "il PRODUCT dello STEP dice "+e.Valore)
			case "pdf_testo_famiglia":
				agg(e.Valore, FonteCartiglio, true, false, false, "il testo del PDF, dove sta il cartiglio, dice "+e.Valore)
			case "pdf_metadati":
				agg(e.Valore, FonteMetadatiPDF, e.DipendeDa == "" && d.canonico(e.Valore) != nome, false, false, "i metadati del PDF dicono "+e.Valore)
			}
		}
	}
	for _, e := range f.EvidenzePDF {
		dove := ""
		if e.Pagina > 0 {
			dove = fmt.Sprintf(" (pagina %d)", e.Pagina)
		}
		switch e.Fonte {
		case FontePDFCartiglio:
			agg(e.Codice, FonteCartiglio, true, false, false, "il cartiglio del PDF dice "+e.Codice+dove)
		case FontePDFOCR:
			agg(e.Codice, FonteOCR, true, false, false, "l'OCR del cartiglio legge "+e.Codice+dove+": un'evidenza, non una decisione")
		case FontePDFMetadati:
			agg(e.Codice, FonteMetadatiPDF, d.canonico(e.Codice) != nome, false, false, "i metadati del PDF dicono "+e.Codice)
		case FontePDFTesto:
			// un token del corpo del PDF vale solo come chiave di ricerca nell'indice (P33): non e' il codice del
			// file, e da solo non sostiene una destinazione piu' del nome
			agg(e.Codice, FonteTestoPDF, false, false, true, "il testo del PDF cita "+e.Codice+dove)
		}
	}
	sort.Strings(ordine)
	out := make([]lettura, 0, len(ordine))
	for _, c := range ordine {
		l := per[c]
		sort.Strings(l.fonti)
		out = append(out, *l)
	}
	return out, radici
}

// candidato e' la destinazione verso la voce v per il codice letto l, con la regola S1 del caso (A5.14.3).
func (d *derivati) candidato(v *Voce, l lettura) Candidato {
	c := Candidato{Codice: v.Codice, Fonti: append([]string(nil), l.fonti...), DalContenuto: l.contenuto, livello: v.Livello,
		Discordanze: append([]string(nil), v.Discordanze...), Posizioni: posizioni(v)}
	var nodo *RifNodo
	if len(v.Nodi) > 0 {
		nodo = &v.Nodi[0]
		c.Portatore = &PortatoreDest{AllegatoID: nodo.AllegatoID, File: nodo.File, Chiave: nodo.Chiave}
	}
	if l.contenuto {
		c.Sostegno = append(c.Sostegno, SostegnoContenuto)
	}
	suffisso := "_nome"
	if l.contenuto {
		suffisso = "_contenuto"
	}
	nellAutorita := nodo != nil && nodo.NellAutorita
	chiaveComp := func() string { return "componente:" + v.ComponenteID.UUID.String() }
	// un componente e' «deciso» solo se nessun motivo lo fa da rivedere (F7)
	deciso := len(v.DaRivedere) == 0
	sostegnoComp := func() {
		if deciso {
			c.Sostegno = append(c.Sostegno, SostegnoComponente)
		}
	}
	switch {
	case l.radice && v.Prodotto():
		// lo STEP la cui radice e' il prodotto: il suo 3D, e il file da autorizzare (l'autorizzazione e' un
		// gesto a parte, mai preselezionato)
		c.Regola, c.Bersaglio, c.Ruolo = "dest_radice_uguale", "prodotto", "candidato_strutturale"
		c.Chiave = "identificativo:" + v.Codice
		if v.ComponenteID.Valid {
			c.Chiave = chiaveComp()
		}
		c.Sostegno = append(c.Sostegno, SostegnoProdotto)
		c.Evidenze = []string{fmt.Sprintf("la radice dello STEP è %s, prodotto della RFQ: è il suo 3D, e il file da autorizzare per %s", v.Codice, v.Codice)}
	case l.radice:
		c.Regola, c.Bersaglio, c.Ruolo = "dest_3d_sottoassieme", "componente", "3d_del_sottoassieme"
		c.Evidenze = []string{fmt.Sprintf("la radice dello STEP è %s, un pezzo dell'indice: è il 3D di %s, e il file che ne proporrebbe i figli diretti", v.Codice, v.Codice)}
		switch {
		case v.Commerciale:
			c.Evidenze = []string{fmt.Sprintf("la radice dello STEP è %s, un pezzo dell'indice: è il 3D di %s, un commerciale; il suo STEP è solo guida: per usarlo come struttura cambia prima il tipo", v.Codice, v.Codice)}
		case v.SottoCommerciale != "":
			c.Evidenze = []string{fmt.Sprintf("la radice dello STEP è %s, un pezzo dell'indice: è il 3D di %s, che sta sotto %s, un commerciale; il suo STEP è solo guida: per usarlo come struttura cambia prima il tipo di %s",
				v.Codice, v.Codice, v.SottoCommerciale, v.SottoCommerciale)}
		}
		switch {
		case v.ComponenteID.Valid:
			c.Chiave = chiaveComp()
			sostegnoComp()
		case nodo != nil:
			c.Chiave, c.Bersaglio = "nodo:"+nodo.PropostaID.String(), "nodo"
			c.Evidenze = append(c.Evidenze, testoNodo(v, *nodo))
		}
	case v.ComponenteID.Valid && nellAutorita:
		c.Regola, c.Bersaglio, c.Chiave = "dest_nodo_diretto"+suffisso, "figlio_diretto", chiaveComp()
		sostegnoComp()
		c.Evidenze = []string{testoNodo(v, *nodo)}
		sostegnoAutorizzato(&c, v, *nodo)
		if deciso {
			c.Evidenze = append(c.Evidenze, v.Codice+" è già un componente deciso")
		}
	case v.ComponenteID.Valid:
		c.Regola, c.Bersaglio, c.Chiave = "dest_componente"+suffisso, "componente", chiaveComp()
		if v.Prodotto() {
			c.Bersaglio = "prodotto"
			c.Sostegno = append(c.Sostegno, SostegnoProdotto)
			c.Evidenze = []string{v.Codice + " è un prodotto della RFQ"}
		} else {
			sostegnoComp()
			// sotto quali prodotti il componente sta nella working: i prodotti dei cammini negli STEP (un nodo
			// di uno STEP non confermato) non lo fanno un componente di quel prodotto
			switch {
			case deciso && len(v.ProdottiDecisi) > 0:
				c.Evidenze = []string{v.Codice + " è un componente deciso sotto " + strings.Join(v.ProdottiDecisi, ", ")}
			case len(v.ProdottiWorking) > 0:
				c.Evidenze = []string{v.Codice + " è un componente sotto " + strings.Join(v.ProdottiWorking, ", ")}
			case deciso:
				c.Evidenze = []string{v.Codice + " è un componente deciso"}
			default:
				c.Evidenze = []string{v.Codice + " è un componente"}
			}
		}
	case v.Identificativo:
		c.Regola, c.Bersaglio, c.Chiave = "dest_identificativo", "identificativo", "identificativo:"+v.Codice
		c.Sostegno = append(c.Sostegno, SostegnoProdotto)
		c.Evidenze = []string{v.Codice + " è un codice della richiesta senza il suo componente: «Crea il prodotto»"}
	case nodo != nil && nodo.NellAutorita:
		c.Regola, c.Bersaglio, c.Chiave = "dest_nodo_diretto"+suffisso, "figlio_diretto", "nodo:"+nodo.PropostaID.String()
		c.Evidenze = []string{testoNodo(v, *nodo), "il nodo è da accettare: associare e accettare è un gesto, mai preselezionato"}
		sostegnoAutorizzato(&c, v, *nodo)
	case nodo != nil && nodo.Autorita == AutoritaDiretta:
		c.Regola, c.Bersaglio, c.Chiave, c.Bloccato = "dest_nodo_non_autorizzato", "nodo_guida", "nodo:"+nodo.PropostaID.String(), DiscStepNonAutorizzato
		c.Evidenze = []string{testoNodo(v, *nodo), "autorizza prima lo STEP"}
	case nodo != nil:
		c.Regola, c.Bersaglio, c.Chiave, c.Bloccato = "dest_nodo_profondo", "nodo_guida", "nodo:"+nodo.PropostaID.String(), DiscNessunaAutorita
		c.Evidenze = []string{testoNodo(v, *nodo)}
		// la guida di un commerciale e delle alternative si dice sempre, anche se il primo nodo e' un altro
		for _, n := range v.Nodi[1:] {
			if n.guida() {
				c.Evidenze = aggiungi(c.Evidenze, testoNodo(v, n))
			}
		}
	default:
		return Candidato{}
	}
	if strings.HasPrefix(c.Chiave, "componente:") {
		if n, ok := nodoDellAncora(v); ok {
			// quale STEP sostiene la destinazione: chi legge vede da dove viene l'evidenza oltre al nome
			c.Sostegno = append(c.Sostegno, SostegnoNodoAncora)
			c.Evidenze = aggiungi(c.Evidenze, fmt.Sprintf("%s è un nodo dello STEP %s, che ancora %s: un'evidenza strutturale oltre al nome del file",
				v.Codice, n.File, n.Ancora))
		} else {
			// il codice e' un nodo solo di STEP con un'ancora da confermare o concorrente, o raggiunta solo
			// attraverso un pezzo da rivedere: si dice, ma non sostiene la destinazione (U7)
			for _, n := range v.Nodi {
				if n.Ancora != "" && !n.guida() {
					c.Evidenze = aggiungi(c.Evidenze, testoNodoNonAffidabile(v, n))
				}
			}
		}
	}
	if !deciso {
		c.DaRivedere = true
		c.Evidenze = append(c.Evidenze, fmt.Sprintf("%s è da rivedere (%s): lo decide una persona, niente è preselezionato",
			v.Codice, strings.Join(v.DaRivedere, "; ")))
	}
	c.Evidenze = append(append([]string(nil), l.testi...), c.Evidenze...)
	if c.Bloccato != "" {
		c.Discordanze = aggiungi(c.Discordanze, c.Bloccato)
	}
	switch {
	case v.Livello == LivelloDaConfermare:
		c.Discordanze = aggiungi(c.Discordanze, DiscAncoraDaConfermare)
	case v.Concorrenti:
		c.Discordanze = aggiungi(c.Discordanze, DiscAncoreConcorrenti)
	}
	c.Score = classificazione.Punteggi[c.Regola].Score
	return c
}

// candidatoVicino e' la destinazione verso una voce quasi uguale al codice letto (P4): sempre bloccata. Per la
// radice di uno STEP vicina a un prodotto e' la «radice vicina» (A-S3: variante o revisione?).
func (d *derivati) candidatoVicino(v *Voce, l lettura, motivo string) Candidato {
	c := Candidato{Codice: v.Codice, Fonti: append([]string(nil), l.fonti...), DalContenuto: l.contenuto, livello: v.Livello,
		Posizioni: posizioni(v), Regola: "dest_quasi_uguale", Bersaglio: "componente", Bloccato: DiscQuasiUguale}
	switch {
	case v.ComponenteID.Valid:
		c.Chiave = "componente:" + v.ComponenteID.UUID.String()
	case v.Identificativo:
		c.Chiave, c.Bersaglio = "identificativo:"+v.Codice, "identificativo"
	case len(v.Nodi) > 0:
		c.Chiave, c.Bersaglio = "nodo:"+v.Nodi[0].PropostaID.String(), "nodo_guida"
	default:
		return Candidato{}
	}
	if l.radice && v.Prodotto() {
		c.Regola, c.Bloccato, c.Bersaglio, c.Ruolo = "dest_radice_vicina", DiscRadiceDiversa, "prodotto", "candidato_strutturale"
	}
	c.Evidenze = append(append([]string(nil), l.testi...), fmt.Sprintf("%s è vicino a %s (%s): è %s, o un pezzo diverso?", l.codice, v.Codice, motivo, v.Codice))
	c.Discordanze = ordinate(append([]string{c.Bloccato, DiscQuasiUguale}, discordanzeVicino(motivo)...))
	c.Score = classificazione.Punteggi[c.Regola].Score
	return c
}

// precedente e' la pre-assegnazione di prima (componente_id sulla proposta, un gesto senza autore registrato):
// si mostra come candidato, non si prende da sola.
func (d *derivati) precedente(f FileFlusso) (Candidato, bool) {
	if !f.Proposta.ComponenteID.Valid {
		return Candidato{}, false
	}
	comp, ok := d.compPerID[f.Proposta.ComponenteID.UUID]
	if !ok || comp.ArchiviatoIl != nil {
		return Candidato{}, false
	}
	c := Candidato{Chiave: "componente:" + comp.ComponenteID.String(), Bersaglio: "componente", Codice: d.canonico(comp.Codice),
		Regola: "dest_assegnazione_precedente", Fonti: []string{FontePrecedente},
		Evidenze: []string{"assegnato a " + comp.Codice + " da un gesto di prima, senza autore registrato"}}
	c.Score = classificazione.Punteggi[c.Regola].Score
	return c, true
}

// nodoDellAncora: il codice della voce e' un nodo di uno STEP che ancora un pezzo (quello del prodotto o di un
// sottoassieme) con un'ancora affidabile, autorizzata o piena e senza file concorrenti per tutto il cammino,
// fuori dalla sola guida di un commerciale o di un'alternativa, e non raggiunta solo attraverso un pezzo da
// rivedere: un'evidenza strutturale che lega il pezzo alla RFQ indipendentemente dal nome del file (scelta 2).
// Il nodo di un'ancora da confermare, concorrente o dietro un pezzo che nessuno ha deciso no: il livello del
// candidato e' quello migliore della voce, e non deve prestare il sostegno a un nodo di un'altra ancora (U7).
// Restituisce il primo nodo affidabile, per dire quale STEP sostiene la destinazione.
func nodoDellAncora(v *Voce) (RifNodo, bool) {
	for _, n := range v.Nodi {
		if n.affidabile() {
			return n, true
		}
	}
	return RifNodo{}, false
}

// sostegnoAutorizzato aggiunge al candidato il sostegno dello STEP autorizzato nella cui autorita' sta il nodo
// n, se quello STEP si raggiunge dal prodotto per un cammino che una persona ha deciso. L'autorizzazione dice
// quali sono i figli diretti del pezzo dell'ancora, non che quel pezzo sta sotto il prodotto: se lo si
// raggiunge solo attraverso un pezzo da rivedere (il pezzo stesso, o uno sopra di lui, nati da un'evidenza),
// il legame fra il file e la RFQ passa ancora da un anello che nessuno ha deciso, e il sostegno non c'e', come
// per nodo_dell_ancora (RifNodo.affidabile). Il nodo si mostra, e l'evidenza dice perche' non sostiene (U7:
// nessuna evidenza che insieme sostiene e non sostiene la destinazione).
func sostegnoAutorizzato(c *Candidato, v *Voce, n RifNodo) {
	if n.TramiteDaRivedere == "" {
		c.Sostegno = append(c.Sostegno, SostegnoStepAutorizzato)
		return
	}
	c.Evidenze = aggiungi(c.Evidenze, testoNodoNonAffidabile(v, n))
}

// testoNodoNonAffidabile dice perche' il nodo di uno STEP non sostiene la destinazione.
func testoNodoNonAffidabile(v *Voce, n RifNodo) string {
	switch {
	case n.TramiteDaRivedere == n.Ancora:
		return fmt.Sprintf("%s compare nello STEP %s, che ancora %s, a sua volta da rivedere: non sostiene la destinazione", v.Codice, n.File, n.Ancora)
	case n.TramiteDaRivedere != "":
		return fmt.Sprintf("%s compare nello STEP %s, che ancora %s, raggiunto solo attraverso %s, da rivedere: non sostiene la destinazione",
			v.Codice, n.File, n.Ancora, n.TramiteDaRivedere)
	}
	perche := "un'ancora da confermare"
	if n.Concorrenti {
		perche = "due file concorrenti"
	}
	return fmt.Sprintf("%s compare nello STEP %s, che ancora %s con %s: non sostiene la destinazione", v.Codice, n.File, n.Ancora, perche)
}

// testoNodo dice dove sta il nodo, con l'autorita'.
func testoNodo(v *Voce, n RifNodo) string {
	switch {
	case n.NellAutorita:
		return fmt.Sprintf("%s è un figlio diretto di %s nello STEP autorizzato %s", v.Codice, n.Padre, n.File)
	case n.Alternativa != "":
		return fmt.Sprintf("%s compare nello STEP %s, un'alternativa a %s per %s: è guida, senza autorità, finché una persona non sceglie quale file autorizzare",
			v.Codice, n.File, n.Alternativa, n.AlternativaDi)
	case n.SottoCommerciale != "":
		return fmt.Sprintf("%s è nello STEP %s sotto %s, un commerciale: i suoi discendenti sono solo guida; per usarne lo STEP cambia prima il tipo di %s",
			v.Codice, n.File, n.SottoCommerciale, n.SottoCommerciale)
	case n.Autorita == AutoritaDiretta:
		return fmt.Sprintf("%s è un figlio diretto della radice dello STEP %s, non ancora autorizzato", v.Codice, n.File)
	}
	return fmt.Sprintf("%s è un nodo profondo dello STEP %s, sotto %s: la struttura sotto %s la propone lo STEP di %s, se arriva",
		v.Codice, n.File, n.Padre, n.Padre, n.Padre)
}

func posizioni(v *Voce) []Posizione {
	if len(v.Posizioni) > MaxCandidati {
		return append([]Posizione(nil), v.Posizioni[:MaxCandidati]...)
	}
	return append([]Posizione(nil), v.Posizioni...)
}

// unisci aggiunge un candidato: la stessa chiave e' lo stesso bersaglio, e si fonde (le fonti e le evidenze
// insieme, la regola piu' forte).
func unisci(cc []Candidato, c Candidato) []Candidato {
	if c.Chiave == "" {
		return cc
	}
	for i := range cc {
		if cc[i].Chiave != c.Chiave {
			continue
		}
		x := cc[i]
		forte := x
		if c.Score > x.Score || (c.Score == x.Score && c.DalContenuto && !x.DalContenuto) {
			forte = c
		}
		forte.Fonti = unione(x.Fonti, c.Fonti)
		forte.Evidenze = unioneOrdine(x.Evidenze, c.Evidenze)
		forte.Discordanze = unione(x.Discordanze, c.Discordanze)
		forte.Sostegno = unione(x.Sostegno, c.Sostegno)
		forte.DalContenuto = x.DalContenuto || c.DalContenuto
		forte.DaRivedere = x.DaRivedere || c.DaRivedere
		if forte.DalContenuto {
			forte.Sostegno = unione(forte.Sostegno, []string{SostegnoContenuto})
		}
		cc[i] = forte
		return cc
	}
	return append(cc, c)
}

// chiudiDestinazione ordina i candidati, ne tiene al piu' MaxCandidati, e dice se il primo e' preselezionabile.
// L'ordine: prima i candidati il cui codice e' scritto dentro il file (il nome e' l'evidenza piu' debole: se
// cartiglio o STEP dicono una cosa e il nome un'altra, la destinazione coerente con il contenuto viene prima),
// poi lo score della regola, poi la chiave.
func (d *derivati) chiudiDestinazione(dest *Destinazione, cands []Candidato, disc []string) {
	for i := range cands {
		sort.Strings(cands[i].Fonti)
		sort.Strings(cands[i].Sostegno)
		cands[i].Discordanze = ordinate(cands[i].Discordanze)
	}
	chiavi := map[string]bool{}
	for _, c := range cands {
		chiavi[c.Chiave] = true
	}
	for _, c := range cands {
		if contiene(c.Fonti, FontePrecedente) && len(chiavi) > 1 {
			disc = append(disc, DiscPreassegnato)
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.DalContenuto != b.DalContenuto {
			return a.DalContenuto
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		return a.Chiave < b.Chiave
	})
	if len(cands) > MaxCandidati {
		cands = cands[:MaxCandidati]
	}
	for i := range cands {
		cands[i].Rango = i + 1
	}
	dest.Esito, dest.Candidati, dest.Discordanze = EsitoProposta, cands, ordinate(disc)
	dest.Preselezionabile = preselezionabile(cands, dest.Discordanze)
	switch {
	case dest.Preselezionabile:
		dest.Stato = "preselezionabile"
	case cands[0].Bloccato != "":
		dest.Stato = "bloccata"
	default:
		dest.Stato = "da_scegliere"
	}
}

// preselezionabile dice se il primo candidato puo' arrivare gia' scelto (A5.14.4, U7; scelte 1 e 2). Tutte:
//   - un candidato non bloccato, senza discordanze, e nessuna discordanza sul file;
//   - nessun'altra chiave con score >= SogliaDiscordanza (nemmeno bloccata): niente pari merito;
//   - il bersaglio e' un componente (o il documento generale): mai un nodo da accettare o un prodotto da creare,
//     che fanno nascere un componente (scelta 1);
//   - per un componente, un'ancora autorizzata o piena, un pezzo non da rivedere, e un sostegno che basta oltre
//     al nome del file (scelta 2: il contenuto, un prodotto confermato, lo STEP autorizzato o che ancora il
//     pezzo, raggiunti dal prodotto per un cammino deciso; mai il solo «componente deciso»); per il generale,
//     il tipo dal contenuto.
func preselezionabile(cands []Candidato, discFile []string) bool {
	if len(cands) == 0 || len(discFile) > 0 {
		return false
	}
	top := cands[0]
	if top.Bloccato != "" || len(top.Discordanze) > 0 {
		return false
	}
	for _, c := range cands[1:] {
		if c.Chiave != top.Chiave && c.Score >= classificazione.SogliaDiscordanza {
			return false
		}
	}
	switch {
	case top.Chiave == "generale":
		return top.Regola == "dest_generale_contenuto"
	case strings.HasPrefix(top.Chiave, "componente:"):
		if top.Regola == "dest_assegnazione_precedente" || top.DaRivedere {
			return false // un gesto di prima senza autore, o un pezzo che nessuno ha deciso, non sono evidenze
		}
		return (top.livello == LivelloAutorizzata || top.livello == LivelloPiena) && sostegnoBasta(top.Sostegno)
	}
	return false
}

// firmata calcola la firma della destinazione: lo sha256 del suo JSON senza la firma. Stesso stato, stessa
// firma: ScriviDestinazione non riscrive (idempotenza, A5.13.6).
func firmata(dest Destinazione) Destinazione {
	dest.Firma = ""
	raw, _ := json.Marshal(dest)
	h := sha256.Sum256(raw)
	dest.Firma = hex.EncodeToString(h[:])
	return dest
}

// ancoreDest fotografa le ancore dei prodotti nella destinazione.
func ancoreDest(ix Indice) []AncoraDest {
	var out []AncoraDest
	for _, p := range ix.Prodotti {
		a := ix.Ancore[p.Codice]
		ad := AncoraDest{Prodotto: p.Codice, Livello: a.Livello, Motivi: a.Motivi}
		for _, x := range a.Portatori {
			if x.Livello == a.Livello {
				ad.File = append(ad.File, x.Nome)
				ad.Regole = unione(ad.Regole, x.Regole)
			}
		}
		out = append(out, ad)
	}
	return out
}

// parole e' la regola della tabella S1 in parole, per le evidenze.
func parole(regola string) string {
	if r, ok := classificazione.Punteggi[regola]; ok && r.Parole != "" {
		return r.Parole
	}
	return regola
}

func aggiungi(xx []string, s string) []string {
	if contiene(xx, s) {
		return xx
	}
	return append(xx, s)
}

func unione(a, b []string) []string {
	out := append([]string(nil), a...)
	for _, x := range b {
		out = aggiungi(out, x)
	}
	sort.Strings(out)
	return out
}

func unioneOrdine(a, b []string) []string {
	out := append([]string(nil), a...)
	for _, x := range b {
		out = aggiungi(out, x)
	}
	return out
}

func ordinate(xx []string) []string {
	var out []string
	for _, x := range xx {
		if x != "" {
			out = aggiungi(out, x)
		}
	}
	sort.Strings(out)
	return out
}
