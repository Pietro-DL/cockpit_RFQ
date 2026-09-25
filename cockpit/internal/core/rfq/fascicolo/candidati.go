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
// Componenti e identificativi della RFQ non sono evidenze: dicono se il codice e' gia' deciso, e quindi
// che cosa si fa da qui. Un codice con una proposta STEP aperta porta a quella proposta; uno che e' gia'
// un componente si apre; uno archiviato si ripristina; un codice della richiesta entra come prodotto; solo
// un codice davvero nuovo si aggiunge come prodotto, assieme o particolare.

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
	// Frase e' come la legge chi guarda: «famiglia «disegni 529» · oggetto», «STEP: 52920517 — a.stp».
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

// Situazione dice se il codice e' gia' deciso, e quindi quale gesto porta.
type Situazione string

const (
	SituazioneProposta   Situazione = "proposta"   // un nodo STEP aperto con questo codice: si decide quello
	SituazioneComponente Situazione = "componente" // gia' nella BOM: si apre
	SituazioneArchiviato Situazione = "archiviato" // si propone il ripristino, con la sua storia
	SituazioneRichiesta  Situazione = "richiesta"  // codice della richiesta (triage): entra come prodotto
	SituazioneNuovo      Situazione = "nuovo"      // + Prodotto / + Assieme / + Particolare
)

// TipiDaCodice sono i tipi con cui un codice davvero nuovo entra nella BOM da qui.
var TipiDaCodice = []db.TipoComponente{db.TipoComponenteFinito, db.TipoComponenteSottoassieme, db.TipoComponenteSciolto}

// Stato e' quello che la RFQ ha gia' deciso su un codice.
type Stato struct {
	Situazione     Situazione
	Componente     *db.Componente                 // attivo o archiviato, se c'e'
	Proposte       []db.ListProposteNodoAperteRow // i nodi STEP aperti con questo codice
	Identificativo bool                           // e' un codice della richiesta
	// Tipi: con quali tipi si puo' aggiungere da qui. Vuoto quando il codice ha gia' la sua strada (una
	// proposta, un componente, un ripristino) e quando la BOM e' congelata.
	Tipi []db.TipoComponente
	// Bloccata: la BOM e' congelata in questa versione (D26). I gesti che la cambiano aspettano una
	// revisione; la situazione resta quella, e si mostra.
	Bloccata int32
}

// Proposta e' il nodo a cui il codice porta: il primo, per data del file.
func (s Stato) Proposta() db.ListProposteNodoAperteRow {
	if len(s.Proposte) == 0 {
		return db.ListProposteNodoAperteRow{}
	}
	return s.Proposte[0]
}

// AltreProposte: quanti altri nodi aperti hanno lo stesso codice. Accettarne uno li riconcilia.
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
	richiesta := map[string]bool{}
	for _, i := range c.Identificativi {
		richiesta[strings.ToUpper(strings.TrimSpace(i.Codice))] = true
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

// eRiferimento: il codice e' il riferimento della richiesta, o un suo pezzo («RDO 490020618» e
// «490020618» sono lo stesso numero visto due volte). E' la regola con cui Motore.Estrai toglie il
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
// aperta (accettarla porta il file e ritrova o ripristina il componente), il componente, l'archiviato, il
// codice della richiesta, il codice nuovo.
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
		s.Situazione, s.Tipi = SituazioneRichiesta, []db.TipoComponente{db.TipoComponenteFinito}
	default:
		s.Situazione, s.Tipi = SituazioneNuovo, TipiDaCodice
	}
	if bloccata > 0 {
		s.Tipi = nil
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
	}
	return Unisci(righe, c), nil
}

// ------------------------------------------------------------------ il gesto

// AggiungiDaCodice e' «+ Prodotto / + Assieme / + Particolare» su un codice della RFQ: il componente nasce
// da un codice rilevato, con una decisione. La situazione del codice si rilegge qui, con la RFQ bloccata,
// perche' quella che la pagina mostrava puo' essere cambiata: un codice con una proposta STEP aperta si
// decide li' (il nodo porta con se' il file e le sue relazioni), uno gia' nella BOM si apre, uno
// archiviato si ripristina, un codice della richiesta entra come prodotto. Con revisioni discordanti la
// revisione la sceglie chi aggiunge, fra quelle viste.
func AggiungiDaCodice(ctx context.Context, q *db.Queries, thread, utente uuid.UUID, codice string, tipo db.TipoComponente, rev string) (string, error) {
	if err := prepara(ctx, q, thread, "si aggiunge un componente"); err != nil {
		return "", err
	}
	cand, err := CandidatiDellaRfq(ctx, q, thread)
	if err != nil {
		return "", err
	}
	k, ok := cand.Trova(codice)
	if !ok {
		return "", Rifiuto(fmt.Sprintf("%s non è fra i codici trovati in questa RFQ", strings.TrimSpace(codice)))
	}
	s := k.Stato
	switch s.Situazione {
	case SituazioneProposta:
		p := s.Proposta()
		return "", Rifiuto(fmt.Sprintf("%s ha una proposta aperta dallo STEP %s (nodo «%s»): si decide quella, non si crea un componente parallelo",
			k.Codice, p.NomeFile, p.NomeGrezzo))
	case SituazioneComponente:
		return "", Rifiuto(fmt.Sprintf("%s è già nella BOM come %s: si apre quello", s.Componente.Codice, NomeTipo(s.Componente.Tipo)))
	case SituazioneArchiviato:
		return "", Rifiuto(fmt.Sprintf("%s è archiviato: si ripristina, con la sua storia, invece di crearne un altro", s.Componente.Codice))
	}
	valido := false
	for _, t := range s.Tipi {
		valido = valido || t == tipo
	}
	if !valido {
		if s.Situazione == SituazioneRichiesta {
			return "", Rifiuto(fmt.Sprintf("%s è un codice della richiesta: entra come prodotto", k.Codice))
		}
		return "", Rifiuto("un codice si aggiunge come prodotto, assieme o particolare")
	}
	if !classificazione.CodiceAmmissibile(k.Codice) {
		return "", Rifiuto(fmt.Sprintf("%s: il codice ha più di %d caratteri o caratteri non ammessi", k.Codice, classificazione.MaxCodice))
	}
	r, err := revisioneScelta(k, rev)
	if err != nil {
		return "", err
	}
	c, err := q.InsertComponente(ctx, db.InsertComponenteParams{ThreadID: thread, Codice: k.Codice, Rev: testo(r), Qta: 1, Tipo: tipo,
		Origine: db.OrigineComponenteCodiceRilevato, ConfermatoDa: utente})
	if err != nil {
		return "", err
	}
	msg := fmt.Sprintf("%s entra nella BOM come %s", c.Codice, NomeTipo(tipo))
	if r != "" {
		msg += ", rev " + r
	}
	return dopoLaDecisione(ctx, q, thread, msg+".", nil)
}

// revisioneScelta e' la revisione con cui il componente nasce: quella che le evidenze dicono, se ne dicono
// una sola; con revisioni discordanti la sceglie chi aggiunge, fra quelle viste. Il punteggio non sceglie.
func revisioneScelta(k CodiceCandidato, scelta string) (string, error) {
	scelta = strings.TrimSpace(scelta)
	if scelta == "" {
		switch len(k.Revisioni) {
		case 0:
			return "", nil
		case 1:
			return revAmmissibile(k, k.Revisioni[0].Rev)
		}
		return "", Rifiuto(fmt.Sprintf("%s: revisioni discordanti (%s). Si sceglie quale vale", k.Codice, elencoRevisioni(k.Revisioni)))
	}
	for _, r := range k.Revisioni {
		if strings.EqualFold(r.Rev, scelta) {
			return revAmmissibile(k, r.Rev)
		}
	}
	if len(k.Revisioni) == 0 {
		return "", Rifiuto(fmt.Sprintf("%s: nessuna evidenza dice la revisione %s", k.Codice, scelta))
	}
	return "", Rifiuto(fmt.Sprintf("%s: la revisione %s non è fra quelle viste (%s)", k.Codice, scelta, elencoRevisioni(k.Revisioni)))
}

func revAmmissibile(k CodiceCandidato, r string) (string, error) {
	if !classificazione.RevAmmissibile(r) {
		return "", Rifiuto(fmt.Sprintf("%s: la revisione %s ha più di %d caratteri o caratteri non ammessi", k.Codice, r, classificazione.MaxRev))
	}
	return r, nil
}

// elencoRevisioni: «A (2 evidenze), B (1 evidenza)».
func elencoRevisioni(rr []Revisione) string {
	parti := make([]string, len(rr))
	for i, r := range rr {
		parti[i] = fmt.Sprintf("%s (%d evidenz%s)", r.Rev, r.Evidenze, map[bool]string{true: "a", false: "e"}[r.Evidenze == 1])
	}
	return strings.Join(parti, ", ")
}
