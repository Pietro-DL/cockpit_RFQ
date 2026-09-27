package fascicolo

// I codici candidati di una RFQ (Blocco 8, B8.6; piano §6, addendum A1.2).
//
// Qui non si classifica niente. Le evidenze le hanno gia' prodotte altri, ciascuno con le sue regole: la
// classificazione dei messaggi (candidato_codice: famiglia del cliente o generico, e dove l'ha visto), le
// proposte dei documenti (documento_proposta: il nome del file, il cartiglio, la radice di uno STEP) e i
// nodi degli STEP classificati dal server (componente_proposta). Unisci le mette insieme per
// upper(codice) senza perderne nessuna, e senza scegliere fra le revisioni che dicono: due revisioni
// diverse sono un conflitto da mostrare a chi decide, non un punteggio da vincere.
//
// Componenti e identificativi della RFQ non sono evidenze: dicono se il codice e' gia' deciso, e si
// mostrano accanto al codice (una proposta STEP aperta, un componente, un archiviato, un codice della
// richiesta, un codice nuovo).
//
// Smistamento F2 (addendum A5, R1): da qui non si decide niente. Un codice trovato e' un'evidenza, e
// un'evidenza non fa nascere un componente: «+ Prodotto / + Assieme / + Particolare» (AggiungiDaCodice),
// «Accetta la proposta» e «Scarta» del pannello sono tolti. Un componente nuovo ha il codice scritto da una
// persona (l'editor della struttura, la carta «n:»); un prodotto nasce dai codici della richiesta confermati nel
// triage; un nodo si decide dove si vede lo STEP.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// Da dove viene un'evidenza: la colonna sorgente di v_codici_candidati_thread. Le righe di
// componente_proposta portano la fonte della proposta del nodo, oggi sempre «step».
const (
	SorgenteMessaggio = "messaggio"          // candidato_codice
	SorgenteDocumento = "proposta_documento" // documento_proposta.codice
	SorgenteNomeFile  = "nome_file"          // documento_proposta.dettagli.codici_nel_nome
	SorgenteStep      = "step"               // componente_proposta
)

// Evidenza e' un punto in cui la RFQ ha visto un codice: una riga della vista, com'e'.
type Evidenza struct {
	Codice    string // come lo scrive questa evidenza
	Rev       string
	Sorgente  string
	Origine   string // famiglia | generico | operatore per messaggi e nodi; la fonte per i documenti
	Famiglia  string
	Punteggio int
	Dove      string // la colonna evidenza: dove nel messaggio, il nome del file, «nodo — file»
	TipoFile  string // il tipo proposto del file, per le righe che vengono dal suo nome
	Messaggio uuid.UUID
	Allegato  uuid.NullUUID
	// Forte: l'evidenza basta a fare del codice un candidato prodotto (piano §6.1).
	Forte bool
	// Frase e' come la legge chi guarda: «famiglia «disegni 777» · oggetto», «STEP: 77720517 — a.stp».
	Frase string
}

// Revisione e' una revisione vista, con quante evidenze la dicono.
type Revisione struct {
	Rev      string // come la scrive la prima evidenza che la dice
	Evidenze int
}

// CodiceCandidato e' un codice della RFQ con tutte le sue evidenze.
type CodiceCandidato struct {
	Codice    string // la grafia dell'evidenza piu' forte; l'identita' e' Chiave
	Chiave    string // upper(codice), l'identita' del componente (D2)
	Evidenze  []Evidenza
	Revisioni []Revisione // tutte quelle viste; nessuna e' scelta
	Conflitto bool        // piu' di una revisione: la sceglie chi decide
	Punteggio int         // il piu' alto, per ordinare la lista: non decide niente
	Prodotto  bool        // fra i codici prodotto candidati; altrimenti fra gli altri riferimenti
	Motivo    string      // perche' sta in quella lista
	Stato     Stato
}

// Situazione dice che cosa la RFQ ha gia' deciso sul codice. Si legge e basta: nessuna situazione porta
// un gesto (Smistamento F2).
type Situazione string

const (
	SituazioneProposta   Situazione = "proposta"   // un nodo STEP aperto con questo codice: si decide dove si vede lo STEP
	SituazioneComponente Situazione = "componente" // gia' nella BOM
	SituazioneArchiviato Situazione = "archiviato" // archiviato: si ripristina dalla Struttura BOM
	SituazioneRichiesta  Situazione = "richiesta"  // codice della richiesta senza componente: vedi Stato.EntraConLaRevisione
	SituazioneNuovo      Situazione = "nuovo"      // nessun componente: un codice trovato, e basta
)

// TipiDaCodice sono i tipi con cui un nodo proposto entra nella BOM (le tendine dei nodi). Non sono piu' i
// tipi con cui un codice trovato diventa un componente: da un codice trovato non nasce niente (F2).
var TipiDaCodice = []db.TipoComponente{db.TipoComponenteFinito, db.TipoComponenteSottoassieme, db.TipoComponenteSciolto}

// Stato e' quello che la RFQ ha gia' deciso su un codice.
type Stato struct {
	Situazione     Situazione
	Componente     *db.Componente                 // attivo o archiviato, se c'e'
	Proposte       []db.ListProposteNodoAperteRow // i nodi STEP aperti con questo codice
	Identificativo bool                           // e' un codice della richiesta
	// Bloccata: la BOM e' congelata in questa versione (D26): il pannello lo dice.
	Bloccata int32
	// EntraConLaRevisione: un codice della richiesta senza componente che il congelamento ha fermato
	// (confermato dopo l'ultimo congelamento, con la BOM ancora congelata): il prodotto entra aprendo la
	// revisione (AssicuraProdottiDellaRevisione). Negli altri casi il prodotto era stato tolto, o non era mai
	// nato, e aprire la revisione non lo fa rinascere: il pannello non deve promettere quella strada.
	EntraConLaRevisione bool
}

// Proposta e' il nodo a cui il codice porta: il primo, per data del file.
func (s Stato) Proposta() db.ListProposteNodoAperteRow {
	if len(s.Proposte) == 0 {
		return db.ListProposteNodoAperteRow{}
	}
	return s.Proposte[0]
}

// AltreProposte: quanti altri nodi aperti hanno lo stesso codice.
func (s Stato) AltreProposte() int {
	if len(s.Proposte) == 0 {
		return 0
	}
	return len(s.Proposte) - 1
}

// ContestoCodici e' quello che serve a Unisci oltre alle evidenze: niente di tutto questo e' un'evidenza.
type ContestoCodici struct {
	// Riferimento e' il nome che il cliente da' alla richiesta (thread_offerta.riferimento_cliente): si
	// mostra in testata, mai fra i codici, anche quando arriva dal nome di un file o da un'altra mail.
	Riferimento string
	// HaFamiglie: il cliente ha famiglie di codice utilizzabili. Allora il generico da solo non e' un
	// candidato prodotto (la regola di Proponibili).
	HaFamiglie     bool
	Componenti     []db.Componente // tutti, archiviati compresi
	Proposte       []db.ListProposteNodoAperteRow
	Identificativi []db.IdentificativoThread
	Bloccata       int32 // 0 = working libera; n = congelata nella Vn
	// CongelataIl e' il momento dell'ultimo congelamento, con la BOM congelata: dice quali codici della
	// richiesta entrano aprendo la revisione (Stato.EntraConLaRevisione). nil = working libera.
	CongelataIl *time.Time
	// Motore: le regole del cliente. Con i suffissi decorativi, il codice «X» trova il pezzo nato come «X_PRT»
	// prima della regola: e' gia' nella BOM, non si aggiunge un secondo componente.
	Motore *classificazione.Motore
}

// Candidati sono i codici della RFQ nelle due liste del pannello.
type Candidati struct {
	Prodotto []CodiceCandidato // CODICI PRODOTTO CANDIDATI
	Altri    []CodiceCandidato // ALTRI RIFERIMENTI TROVATI
	Bloccata int32             // la BOM e' congelata in questa versione: 0 = libera
}

// Trova il codice nelle due liste, per upper(codice).
func (c Candidati) Trova(codice string) (CodiceCandidato, bool) {
	k := strings.ToUpper(strings.TrimSpace(codice))
	for _, lista := range [][]CodiceCandidato{c.Prodotto, c.Altri} {
		for _, x := range lista {
			if x.Chiave == k {
				return x, true
			}
		}
	}
	return CodiceCandidato{}, false
}

// Unisci aggrega le evidenze della RFQ per upper(codice). Pura: stesse righe, stesso risultato.
//
// Per ogni codice tiene tutte le evidenze e tutte le revisioni viste; il punteggio serve solo a ordinare.
// Il codice va fra i candidati prodotto se almeno un'evidenza e' forte (piano §6.1): una famiglia del
// cliente nel messaggio (non nella storia citata), un nodo di uno STEP, il cartiglio o la radice di uno
// STEP, il nome di un file tecnico; senza famiglie anche il generico, come in Proponibili. Il resto va
// fra gli altri riferimenti. Il riferimento della richiesta non e' un codice e non c'e'.
func Unisci(righe []db.ListCodiciCandidatiThreadRow, c ContestoCodici) Candidati {
	perChiave := map[string]*CodiceCandidato{}
	var chiavi []string
	for _, r := range righe {
		codice := strings.TrimSpace(r.Codice)
		if codice == "" || r.Ruolo == classificazione.RuoloRiferimento || r.Origine == "riferimento" || eRiferimento(c.Riferimento, codice) {
			continue
		}
		e := Evidenza{Codice: codice, Rev: strings.TrimSpace(r.Rev), Sorgente: r.Sorgente, Origine: r.Origine, Famiglia: r.Famiglia,
			Punteggio: int(r.Punteggio), Dove: r.Evidenza, TipoFile: r.TipoFile, Messaggio: r.MessaggioID, Allegato: r.AllegatoID}
		if r.Sorgente == SorgenteNomeFile {
			// un codice citato nel nome di un file che non e' un codice: la vista gli da' 50 fisso (0018), lo
			// score e' quello della regola della tabella S1 (Smistamento F4). La colonna `confidenza` delle
			// proposte invece e' gia' lo score del codice del file.
			e.Punteggio = classificazione.Punteggi["nome_contiene_codice"].Score
		}
		e.Forte = forte(e, c.HaFamiglie)
		e.Frase = frase(e)
		k := strings.ToUpper(codice)
		x, ok := perChiave[k]
		if !ok {
			x = &CodiceCandidato{Chiave: k}
			perChiave[k] = x
			chiavi = append(chiavi, k)
		}
		x.Evidenze = append(x.Evidenze, e)
	}

	componenti := map[string]db.Componente{}
	for _, x := range c.Componenti {
		componenti[strings.ToUpper(strings.TrimSpace(x.Codice))] = x
	}
	if c.Motore.HaSuffissi() {
		for _, x := range c.Componenti {
			can, _ := c.Motore.Canonico(x.Codice, "")
			if k := strings.ToUpper(strings.TrimSpace(can)); k != "" {
				if _, gia := componenti[k]; !gia {
					componenti[k] = x
				}
			}
		}
	}
	proposte := map[string][]db.ListProposteNodoAperteRow{}
	for _, p := range c.Proposte {
		k := strings.ToUpper(strings.TrimSpace(p.Codice))
		proposte[k] = append(proposte[k], p)
	}
	richiesta, conLaRevisione := map[string]bool{}, map[string]bool{}
	for _, i := range c.Identificativi {
		k := strings.ToUpper(strings.TrimSpace(i.Codice))
		richiesta[k] = true
		if c.Bloccata != 0 && fermatoDalCongelamento(i, c.CongelataIl) {
			conLaRevisione[k] = true
		}
	}

	out := Candidati{Bloccata: c.Bloccata}
	for _, k := range chiavi {
		x := perChiave[k]
		ordinaEvidenze(x.Evidenze)
		x.Codice = x.Evidenze[0].Codice
		x.Revisioni = revisioni(x.Evidenze)
		x.Conflitto = len(x.Revisioni) > 1
		for _, e := range x.Evidenze {
			if e.Punteggio > x.Punteggio {
				x.Punteggio = e.Punteggio
			}
		}
		x.Prodotto, x.Motivo = classe(x.Evidenze, c.HaFamiglie)
		x.Stato = situa(k, c.Bloccata, componenti, proposte, richiesta)
		x.Stato.EntraConLaRevisione = x.Stato.Situazione == SituazioneRichiesta && conLaRevisione[k]
		if x.Prodotto {
			out.Prodotto = append(out.Prodotto, *x)
		} else {
			out.Altri = append(out.Altri, *x)
		}
	}
	ordinaCodici(out.Prodotto)
	ordinaCodici(out.Altri)
	return out
}

// eRiferimento: il codice e' il riferimento della richiesta, o un suo pezzo («RDO 400012345» e
// «400012345» sono lo stesso numero visto due volte). E' la regola con cui Motore.Estrai toglie il
// riferimento dai codici di un messaggio, applicata a tutta la RFQ.
func eRiferimento(riferimento, codice string) bool {
	r, c := strings.ToUpper(strings.TrimSpace(riferimento)), strings.ToUpper(codice)
	return r != "" && c != "" && strings.Contains(r, c)
}

// tecnico: un file che e' disegno per il suo tipo. Un PDF da determinare non lo e' ancora: lo dira'
// l'analisi (checkpoint 3R §5).
func tecnico(tipo string) bool {
	switch db.TipoDocumento(tipo) {
	case db.TipoDocumentoCad3d, db.TipoDocumentoDisegno2d, db.TipoDocumentoSviluppoDxf:
		return true
	}
	return false
}

// forte dice se un'evidenza basta a fare del codice un candidato prodotto (piano §6.1). Deboli: la storia
// citata (gia' decisa altrove), il generico quando il cliente ha famiglie, il nome di un file che non e'
// tecnico quando il cliente ha famiglie.
func forte(e Evidenza, haFamiglie bool) bool {
	switch e.Sorgente {
	case SorgenteMessaggio:
		if e.Dove == classificazione.DoveStoria {
			return false
		}
		return e.Origine == "famiglia" || !haFamiglie
	case SorgenteDocumento:
		switch e.Origine {
		case "cartiglio", "step", "regola_cliente", "operatore":
			return true
		}
		return tecnico(e.TipoFile) || !haFamiglie
	case SorgenteNomeFile:
		return tecnico(e.TipoFile) || !haFamiglie
	}
	return true // un nodo di un file: e' un pezzo di una distinta
}

// frase e' l'evidenza come la legge chi guarda.
func frase(e Evidenza) string {
	switch e.Sorgente {
	case SorgenteMessaggio:
		if e.Origine == "famiglia" {
			return fmt.Sprintf("famiglia «%s» · %s", e.Famiglia, e.Dove)
		}
		return "generico · " + e.Dove
	case SorgenteDocumento:
		switch e.Origine {
		case "cartiglio":
			return "cartiglio di " + e.Dove
		case "step", "regola_cliente":
			return "radice dello STEP " + e.Dove
		case "operatore":
			return "scritto dall'operatore per " + e.Dove
		}
		return "nome del file " + e.Dove + tipoTra(e.TipoFile)
	case SorgenteNomeFile:
		return "nel nome di " + e.Dove + tipoTra(e.TipoFile)
	}
	s := "STEP: " + e.Dove
	switch e.Origine {
	case "famiglia":
		s += fmt.Sprintf(" · famiglia «%s»", e.Famiglia)
	case "generico":
		s += " · generico"
	case "operatore":
		s += " · codice scritto dall'operatore"
	}
	return s
}

func tipoTra(tipo string) string {
	if tipo == "" {
		return ""
	}
	return " (" + tipo + ")"
}

// classe decide la lista: prodotto con la prima evidenza forte come motivo, altrimenti altri, con che
// cosa manca.
func classe(ev []Evidenza, haFamiglie bool) (bool, string) {
	for _, e := range ev {
		if e.Forte {
			return true, e.Frase
		}
	}
	visto := map[string]bool{}
	var perche []string
	aggiungi := func(s string) {
		if !visto[s] {
			visto[s] = true
			perche = append(perche, s)
		}
	}
	for _, e := range ev {
		switch {
		case e.Sorgente == SorgenteMessaggio && e.Dove == classificazione.DoveStoria:
			aggiungi("nella storia citata")
		case e.Sorgente == SorgenteMessaggio:
			aggiungi("dall'estrattore generico")
		default:
			aggiungi("nel nome di un file non tecnico")
		}
	}
	m := "solo " + strings.Join(perche, " o ")
	if haFamiglie && visto["dall'estrattore generico"] {
		m += ": il cliente ha famiglie di codice, e nessuna lo riconosce"
	}
	return false, m
}

// ordineSorgente: a parita' di forza e punteggio, prima il file che dice la struttura, poi il documento,
// il messaggio, il nome che contiene il codice.
func ordineSorgente(s string) int {
	switch s {
	case SorgenteDocumento:
		return 1
	case SorgenteMessaggio:
		return 2
	case SorgenteNomeFile:
		return 3
	}
	return 0
}

func ordinaEvidenze(ev []Evidenza) {
	sort.SliceStable(ev, func(i, j int) bool {
		a, b := ev[i], ev[j]
		switch {
		case a.Forte != b.Forte:
			return a.Forte
		case a.Punteggio != b.Punteggio:
			return a.Punteggio > b.Punteggio
		case ordineSorgente(a.Sorgente) != ordineSorgente(b.Sorgente):
			return ordineSorgente(a.Sorgente) < ordineSorgente(b.Sorgente)
		case a.Dove != b.Dove:
			return a.Dove < b.Dove
		}
		return a.Messaggio.String() < b.Messaggio.String()
	})
}

// revisioni sono le revisioni viste, in ordine di evidenza, contate. Una revisione vuota non dice niente
// e non conta; «b» e «B» sono la stessa.
func revisioni(ev []Evidenza) []Revisione {
	var out []Revisione
	pos := map[string]int{}
	for _, e := range ev {
		k := strings.ToUpper(e.Rev)
		if k == "" {
			continue
		}
		i, ok := pos[k]
		if !ok {
			i = len(out)
			pos[k] = i
			out = append(out, Revisione{Rev: e.Rev})
		}
		out[i].Evidenze++
	}
	return out
}

func ordinaCodici(l []CodiceCandidato) {
	sort.SliceStable(l, func(i, j int) bool {
		if l[i].Punteggio != l[j].Punteggio {
			return l[i].Punteggio > l[j].Punteggio
		}
		return l[i].Chiave < l[j].Chiave
	})
}

// situa dice che cosa la RFQ ha gia' deciso sul codice k, nell'ordine in cui conta: una proposta STEP
// aperta, il componente, l'archiviato, il codice della richiesta, il codice nuovo.
func situa(k string, bloccata int32, componenti map[string]db.Componente, proposte map[string][]db.ListProposteNodoAperteRow,
	richiesta map[string]bool) Stato {
	s := Stato{Proposte: proposte[k], Identificativo: richiesta[k], Bloccata: bloccata}
	if x, ok := componenti[k]; ok {
		s.Componente = &x
	}
	switch {
	case len(s.Proposte) > 0:
		s.Situazione = SituazioneProposta
	case s.Componente != nil && s.Componente.ArchiviatoIl == nil:
		s.Situazione = SituazioneComponente
	case s.Componente != nil:
		s.Situazione = SituazioneArchiviato
	case s.Identificativo:
		s.Situazione = SituazioneRichiesta
	default:
		s.Situazione = SituazioneNuovo
	}
	return s
}

// NomeTipo e' il tipo di componente con le parole della schermata.
func NomeTipo(t db.TipoComponente) string {
	switch t {
	case db.TipoComponenteFinito:
		return "prodotto"
	case db.TipoComponenteSottoassieme:
		return "assieme"
	case db.TipoComponenteSciolto:
		return "particolare"
	}
	return string(t)
}

// ------------------------------------------------------------------ dal database

// CandidatiDellaRfq legge le evidenze della RFQ e quello che e' gia' deciso, e le unisce. Non scrive
// niente. Il Motore del cliente serve solo a sapere se il cliente ha famiglie: nessuna evidenza passa di
// nuovo dalle sue regole.
func CandidatiDellaRfq(ctx context.Context, q *db.Queries, thread uuid.UUID) (Candidati, error) {
	t, err := q.GetThread(ctx, thread)
	if err != nil {
		return Candidati{}, err
	}
	righe, err := q.ListCodiciCandidatiThread(ctx, uuid.NullUUID{UUID: thread, Valid: true})
	if err != nil {
		return Candidati{}, err
	}
	m, err := MotoreDellaRfq(ctx, q, thread)
	if err != nil {
		return Candidati{}, err
	}
	c := ContestoCodici{Riferimento: t.RiferimentoCliente.String, HaFamiglie: m.HaFamiglie(), Motore: m}
	if c.Componenti, err = q.ListComponentiThread(ctx, thread); err != nil {
		return Candidati{}, err
	}
	if c.Proposte, err = q.ListProposteNodoAperte(ctx, thread); err != nil {
		return Candidati{}, err
	}
	if c.Identificativi, err = q.ListIdentificativi(ctx, thread); err != nil {
		return Candidati{}, err
	}
	n, bloccata, err := WorkingBloccata(ctx, q, thread)
	if err != nil {
		return Candidati{}, err
	}
	if bloccata {
		c.Bloccata = n
		u, err := q.GetUltimaCongelata(ctx, thread)
		if err != nil {
			return Candidati{}, err
		}
		c.CongelataIl = u.CongelataIl
	}
	return Unisci(righe, c), nil
}
