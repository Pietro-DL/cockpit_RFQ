package web

// La schermata del Fascicolo, B8.7 (piano §7; addendum A4): GET /thread/{id}/fascicolo.
//
// Il modello al centro e' il componente, cioe' la BOM, non il singolo disegno: STRUTTURA mostra la BOM
// working con le proposte degli STEP tratteggiate sotto i nodi, DOCUMENTI i file della RFQ con il loro
// stato, ANTEPRIMA il file aperto (il PDF nel viewer del browser, per STEP e DXF quello che l'analisi ha
// letto), COMPLETEZZA che cosa manca a ogni componente. Codici e Avvisi stanno in un cassetto.
//
// Lo stato della schermata e' l'indirizzo, come per l'Inbox: il nodo scelto, il file aperto, il filtro,
// la vista, il cassetto. Un gesto (POST) risponde con l'avviso e con i pannelli da rifare fuori banda
// (hx-swap-oob), e l'anteprima non la tocca: il PDF aperto resta aperto. Il server sa che il gesto viene
// da qui dall'intestazione HX-Current-URL, che htmx manda da solo a ogni richiesta, e da li' rilegge lo
// stato; senza, le rotte rispondono come prima, con la pagina della RFQ.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// ------------------------------------------------------------------ lo stato nell'indirizzo

// I filtri del pannello Documenti (piano §7.4).
const (
	filtroNonAssegnati = "non_assegnati"
	filtroNodo         = "nodo"
	filtroCandidati    = "candidati"
	filtroTutti        = "tutti"
	filtroDaScaricare  = "da_scaricare"
	filtroRumore       = "rumore"
)

var filtriFascicolo = []struct{ Chiave, Nome string }{
	{filtroNonAssegnati, "Non assegnati"}, {filtroNodo, "Del nodo"}, {filtroCandidati, "Candidati per il nodo"},
	{filtroTutti, "Tutti"}, {filtroDaScaricare, "Da scaricare"}, {filtroRumore, "Rumore e scartati"},
}

// Le viste dell'area principale (B8.7b): la BOM visuale e' la schermata; le card dei componenti e la tabella
// dei documenti sono le altre due linguette; griglia, completezza e albero testuale sono le viste tecniche
// di B8.7, per la diagnostica.
var visteFascicolo = map[string]bool{"componenti": true, "documenti": true, "griglia": true, "completezza": true, "albero": true}

// I cassetti: «Da verificare» (il lavoro dell'operatore), «Rivedi» il piano prima della conferma, «Importa
// dal NAS», e le due viste tecniche di B8.6/B8.7, Codici e Avvisi.
var cassettiFascicolo = map[string]bool{"verifica": true, "piano": true, "nas": true, "codici": true, "avvisi": true}

// Le schede del dettaglio di un componente.
var schedeNodo = map[string]bool{"3d": true, "2d": true, "dxf": true, "altri": true, "storico": true}

// statoFascicolo e' lo stato della schermata, scritto nell'indirizzo.
type statoFascicolo struct {
	Nodo     uuid.UUID // il componente scelto; zero = nessuno
	Prop     uuid.UUID // il nodo proposto scelto nella BOM; zero = nessuno
	File     uuid.UUID // l'allegato aperto nell'anteprima; zero = nessuno
	Doc      uuid.UUID // il documento del componente scelto aperto nel dettaglio; zero = quello della scheda
	Scheda   string    // la scheda del dettaglio: 3d, 2d, dxf, altri, storico; "" = la prima con qualcosa
	Filtro   string    // uno di filtriFascicolo; "" = non assegnati
	Tipo     string    // con il filtro «candidati»: solo quel tipo di documento
	Vista    string    // "" (la BOM) oppure una di visteFascicolo
	Cassetto string    // "" oppure uno di cassettiFascicolo
	Scartate bool      // mostra anche le proposte scartate
	Cerca    string    // i codici e i file che contengono questo testo
	Nas      string    // la cartella aperta nel cassetto «Importa dal NAS», relativa alla radice
	NasCerca string    // la ricerca per nome nel NAS
}

func leggiStatoFascicolo(v url.Values) statoFascicolo {
	st := statoFascicolo{Filtro: v.Get("filtro"), Vista: v.Get("vista"), Cassetto: v.Get("cassetto"), Scartate: v.Get("scartate") == "1",
		Scheda: v.Get("scheda"), Cerca: strings.TrimSpace(v.Get("q")), Nas: strings.TrimSpace(v.Get("nas")), NasCerca: strings.TrimSpace(v.Get("nas_cerca"))}
	st.Nodo, _ = uuid.Parse(v.Get("nodo"))
	st.Prop, _ = uuid.Parse(v.Get("prop"))
	st.File, _ = uuid.Parse(v.Get("file"))
	st.Doc, _ = uuid.Parse(v.Get("doc"))
	valido := false
	for _, f := range filtriFascicolo {
		valido = valido || f.Chiave == st.Filtro
	}
	if !valido || st.Filtro == filtroNonAssegnati {
		st.Filtro = ""
	}
	if !visteFascicolo[st.Vista] {
		st.Vista = ""
	}
	if !cassettiFascicolo[st.Cassetto] {
		st.Cassetto = ""
	}
	if !schedeNodo[st.Scheda] {
		st.Scheda = ""
	}
	if t := db.TipoDocumento(v.Get("tipo")); t.Valid() {
		st.Tipo = string(t)
	}
	if len([]rune(st.Cerca)) > 60 {
		st.Cerca = string([]rune(st.Cerca)[:60])
	}
	if len([]rune(st.NasCerca)) > 60 {
		st.NasCerca = string([]rune(st.NasCerca)[:60])
	}
	return st
}

func (st statoFascicolo) valori() url.Values {
	v := url.Values{}
	for k, id := range map[string]uuid.UUID{"nodo": st.Nodo, "prop": st.Prop, "file": st.File, "doc": st.Doc} {
		if id != uuid.Nil {
			v.Set(k, id.String())
		}
	}
	for k, x := range map[string]string{"scheda": st.Scheda, "filtro": st.Filtro, "tipo": st.Tipo, "vista": st.Vista,
		"cassetto": st.Cassetto, "q": st.Cerca, "nas": st.Nas, "nas_cerca": st.NasCerca} {
		if x != "" {
			v.Set(k, x)
		}
	}
	if st.Scartate {
		v.Set("scartate", "1")
	}
	return v
}

// HaFile dice se nell'anteprima c'e' un file aperto. Un uuid e' un array di 16 byte, e per i template un
// array non vuoto e' sempre «vero»: {{if .Stato.File}} non si puo' scrivere.
func (st statoFascicolo) HaFile() bool { return st.File != uuid.Nil }

// Query e' lo stato come stringa di interrogazione, con il «?» se non e' vuota.
func (st statoFascicolo) Query() string {
	if e := st.valori().Encode(); e != "" {
		return "?" + e
	}
	return ""
}

// Con e' lo stato con coppie chiave/valore cambiate (valore "" = tolta), come stringa di interrogazione:
// {{$.Stato.Con "filtro" "tutti"}}, {{$.Stato.Con "nodo" $id "filtro" "nodo"}}. Cambiare il nodo toglie
// il tipo del filtro candidati, che vale per un nodo solo.
func (st statoFascicolo) Con(coppie ...string) string {
	v := st.valori()
	for i := 0; i+1 < len(coppie); i += 2 {
		switch coppie[i] {
		case "nodo":
			// il tipo del filtro candidati, il documento e la scheda del dettaglio sono di un nodo solo;
			// scegliere un nodo toglie anche il nodo proposto scelto
			v.Del("tipo")
			v.Del("doc")
			v.Del("scheda")
			v.Del("prop")
		case "prop":
			v.Del("nodo")
			v.Del("doc")
			v.Del("scheda")
		case "scheda":
			v.Del("doc")
		}
		if coppie[i+1] == "" {
			v.Del(coppie[i])
		} else {
			v.Set(coppie[i], coppie[i+1])
		}
	}
	if e := v.Encode(); e != "" {
		return "?" + e
	}
	return ""
}

// FiltroAttivo e' il filtro come lo vede la schermata: vuoto vale «non assegnati».
func (st statoFascicolo) FiltroAttivo() string {
	if st.Filtro == "" {
		return filtroNonAssegnati
	}
	return st.Filtro
}

// dalFascicolo dice se la richiesta viene dalla schermata del Fascicolo di questa RFQ, e con che stato:
// htmx manda a ogni richiesta l'indirizzo della pagina in HX-Current-URL.
func dalFascicolo(h interface{ Get(string) string }, thread uuid.UUID) (statoFascicolo, bool) {
	u, err := url.Parse(h.Get("HX-Current-URL"))
	if err != nil || u.Path == "" {
		return statoFascicolo{}, false
	}
	if strings.TrimSuffix(u.Path, "/") != "/thread/"+thread.String()+"/fascicolo" {
		return statoFascicolo{}, false
	}
	return leggiStatoFascicolo(u.Query()), true
}

// ------------------------------------------------------------------ i dati della schermata

// cella e' un requisito del fascicolo per un componente, con il simbolo della barra (piano §7.6).
type cella struct {
	Tipo      db.TipoDocumento
	Etichetta string // 3D, 2D, DXF...
	Simbolo   string
	Classe    string // ok, coda, urg, warn, info, no
	Titolo    string
	Deroga    uuid.NullUUID
	Bloccante bool
}

// rigaFile e' un file della RFQ nel pannello Documenti: il fatto (allegato), l'interpretazione (proposta),
// la decisione (documento) e il componente a cui e' agganciato.
type rigaFile struct {
	A            db.ListAllegatiFascicoloRow
	Proposta     *db.DocumentoProposta
	Doc          *db.Documento
	Comp         *db.Componente
	FileMancante bool
	Tipo         string // quello del documento, se c'e'; altrimenti quello proposto
	Codice, Rev  string
	RevLetta     string // la revisione letta in un file caricato a mano, che non diventa del cliente
	CodiceLetto  string // il codice che il file dice, se e' diverso da quello del componente a cui e' assegnato
	Stato        string
	ClasseStato  string
	// Scelta: i documenti correnti dello stesso tipo nel nodo scelto, quando il file ci entrerebbe. Con
	// almeno uno, «assegna» vuole la risposta: aggiungi o sostituisce quale (A4.10).
	Scelta []db.Documento
	// StepStrutturale: fra quelli della Scelta, lo STEP strutturale del nodo; se il file lo sostituisce,
	// la schermata chiede se il nuovo diventa il riferimento.
	StepStrutturale uuid.UUID
}

// Nome e' il valore della casella di selezione: un documento si assegna come documento, un file non
// ancora confermato come proposta aperta.
func (f rigaFile) Campo() string {
	switch {
	case f.Doc != nil:
		return "documento"
	case f.Proposta != nil && f.Proposta.Stato == db.StatoPropostaAperta:
		return "proposta"
	}
	return ""
}

// Valore e' l'id della casella di selezione.
func (f rigaFile) Valore() string {
	switch f.Campo() {
	case "documento":
		return f.Doc.DocumentoID.String()
	case "proposta":
		return f.Proposta.PropostaID.String()
	}
	return ""
}

// HaStepStrutturale: fra i documenti che il file puo' sostituire nel nodo scelto c'e' lo STEP strutturale.
func (f rigaFile) HaStepStrutturale() bool { return f.StepStrutturale != uuid.Nil }

func (f rigaFile) estensione() string {
	return strings.ToLower(strings.TrimPrefix(f.A.Estensione.String, "."))
}

// Pdf: si apre nel viewer del browser, dall'anteprima di B8.1.
func (f rigaFile) Pdf() bool { return f.estensione() == "pdf" }

// Modello: STEP, IGES, DXF e simili si riassumono dall'analisi, senza viewer (niente 3D in B8.7).
func (f rigaFile) Modello() bool {
	switch f.estensione() {
	case "stp", "step", "igs", "iges", "dxf", "dwg", "x_t", "x_b", "sldprt", "sldasm":
		return true
	}
	return false
}

// Confermabile: in staging, con una proposta aperta; uno zip no (si estrae).
func (f rigaFile) Confermabile() bool {
	return f.Proposta != nil && f.Proposta.Stato == db.StatoPropostaAperta && f.A.PathStaging.Valid && !f.FileMancante &&
		(f.A.Stato == db.StatoAllegatoInStaging || f.A.Stato == db.StatoAllegatoAnalizzato) && f.estensione() != "zip"
}

// DaScaricare: non ancora in staging, o sparito.
func (f rigaFile) DaScaricare() bool {
	return f.FileMancante || f.A.Stato == db.StatoAllegatoGrezzo || f.A.Stato == db.StatoAllegatoErrore
}

// Rumore: scartato, o rumore per la proposta.
func (f rigaFile) Rumore() bool {
	return f.Doc == nil && f.Proposta != nil && (f.Proposta.Stato == db.StatoPropostaScartata || f.Proposta.TipoProposto == db.TipoDocumentoRumore)
}

// Interno: caricato a mano dal Fascicolo (origine manuale), non arrivato da una mail.
func (f rigaFile) Interno() bool { return f.A.Origine == db.OrigineAllegatoManuale }

// figlioProposto e' una relazione proposta da uno STEP sotto un componente della working: la riga
// tratteggiata dell'albero (piano §7.3).
type figlioProposto struct {
	R      db.RelazioneProposta
	File   string
	Padre  db.ComponenteProposta
	Figlio db.ComponenteProposta
}

// rimozione e' un arco della working che lo STEP strutturale, letto per intero, non contiene piu'.
type rimozione struct {
	R      db.RimozioneProposta
	Figlio db.Componente
	Step   string // il nome dello STEP strutturale
}

// bloccoFile sono le proposte ancora da decidere di un file STEP che non pendono sotto un componente:
// le radici del file, i nodi sotto un nodo non ancora accettato, gli archi fra nodi del file.
type bloccoFile struct {
	Allegato  uuid.UUID
	Nome      string
	Nodi      []nodoProposto
	Relazioni []relazioneProposta
	Aperte    int
}

type nodoProposto struct {
	P     db.ComponenteProposta
	Sotto string // il nodo padre nel file, per chi legge
}

type relazioneProposta struct {
	R             db.RelazioneProposta
	Padre, Figlio db.ComponenteProposta
}

// avvisoFascicolo e' una riga del cassetto Avvisi.
type avvisoFascicolo struct {
	Livello string // urg, warn, info
	Testo   string
}

// schedaNodo e' quello che serve ai gesti sul nodo scelto.
type schedaNodo struct {
	C          db.Componente
	Padri      []arcoNodo // gli archi verso i padri attivi
	Documenti  []db.Documento
	Step       []db.Documento // gli STEP correnti del componente: candidati a STEP strutturale
	Richiesti  []cella        // i requisiti del fascicolo, per la deroga
	Deroghe    []db.DerogaFabbisogno
	StepEsito  *db.VStepProdotto
	DerogaStep *db.DerogaStruttura // quella valida adesso, se c'e'
	Candidati  []db.Componente     // i possibili padri
}

type arcoNodo struct {
	Padre db.Componente
	Qta   int32
}

// anteprimaDati e' il file aperto: la riga, la storia delle sue revisioni, e per uno STEP quello che
// l'analisi ha letto.
type anteprimaDati struct {
	F rigaFile
	// Per la conferma: il componente proposto (quello della proposta, altrimenti il nodo scelto) e i suoi
	// documenti correnti dello stesso tipo, fra cui si sceglie che cosa il file sostituisce.
	ConfermaComp     *db.Componente
	ConfermaCorrenti []db.Documento
	// StepFraCorrenti: uno dei documenti sostituibili e' lo STEP strutturale del componente, e la
	// schermata chiede se il nuovo diventa il riferimento.
	StepFraCorrenti bool
	// Annullabile: nella storia, il documento che il corrente ha sostituito; l'unica sostituzione che si
	// annulla (A4.1: solo l'ultima).
	Annullabile  *db.VDocumentoStoria
	Storia       []db.VDocumentoStoria
	Sostituibili []db.Documento // gli altri documenti correnti dello stesso tipo nel componente
	// StepDi: il prodotto finito di cui e' lo STEP strutturale, se lo e'.
	StepDi   *db.Componente
	Analisi  *riepilogoAnalisi
	Proposte []db.ComponenteProposta // i nodi che questo file propone
}

// riepilogoAnalisi e' la lettura di un modello: la struttura dello STEP, o i fatti grezzi di un altro file.
type riepilogoAnalisi struct {
	Nomi           map[string]string // chiave del nodo → come si chiama, per le relazioni
	Versione       int16
	Struttura      *worker.StrutturaSTEP
	MotivoParziale string   // "" = completa
	Altri          []string // le altre chiavi dei fatti, per chi vuole sapere che cosa c'e'
}

// fascicoloDati e' tutta la schermata.
type fascicoloDati struct {
	T        db.ThreadOfferta
	Riga     db.VCruscotto
	Cliente  db.Cliente
	Base     string
	Stato    statoFascicolo
	Avviso   string
	Scrive   bool // consultazione vede e non cambia: i gesti non si mostrano
	Fase     *db.FaseLog
	Versioni []db.VBomVersioni
	Ultima   *db.VBomVersioni
	// Bloccata e' il numero della versione congelata che blocca la working (D26); 0 = libera.
	Bloccata int32
	Bozza    *db.VBomVersioni
	// Differenze: la working contro la versione da cui la bozza e' partita.
	Differenze []fascicolo.Differenza
	Gate       fascicolo.Gate
	// SceltaContesto: in ACCETTATA e DISTINTA_ERP il tipo di revisione lo sceglie chi la apre, senza
	// preselezione (D25c). ContestoFisso: negli altri casi, quello della fase.
	SceltaContesto bool
	ContestoFisso  string
	PuoCongelare   bool // la fase ammette un congelamento (A4.6 passo 2)
	PuoAprire      bool // la fase ammette una revisione (A4.7)

	Albero       fascicolo.Albero
	Componenti   map[uuid.UUID]db.Componente
	Completezza  map[uuid.UUID][]cella
	StepProdotto map[uuid.UUID]db.VStepProdotto
	Inline       map[uuid.UUID][]figlioProposto
	Rimozioni    map[uuid.UUID][]rimozione
	Blocchi      []bloccoFile
	NProposte    int

	File      []rigaFile
	Visibili  []rigaFile
	Conteggi  map[string]int
	Nodo      *schedaNodo
	Anteprima *anteprimaDati

	Codici       fascicolo.Candidati
	DocumentiDi  map[uuid.UUID][]db.Documento
	CodiciErrore string
	Avvisi       []avvisoFascicolo
	NBloccanti   int64
	NAnalisi     int64
	NAnomalie    int
	Caricamento  bool // il caricamento interno e' configurato

	// B8.7b
	Piano       fascicolo.PianoFascicolo // il piano di riconciliazione: che cosa entra con «Conferma Fascicolo»
	ErrorePiano string
	Lavoro      fascicolo.Lavoro // download, estrazioni e analisi ancora in corso sui file della RFQ
	Avanzamento avanzamento
	Rifai       bool // la risposta del poll rifa' i pannelli: la firma e' cambiata
	// NodiProposti e RelazioniProposte sono le proposte degli STEP come le legge la BOM visuale.
	NodiProposti      []db.ListComponenteProposteThreadRow
	RelazioniProposte []db.ListRelazioneProposteThreadRow
	Carte             []*carta                // la BOM visuale
	Schede            []*carta                // la linguetta «Componenti»
	AllegatoDi        map[uuid.UUID]uuid.UUID // documento → l'allegato da cui e' nato, per aprirlo
	Dettaglio         *dettaglioNodo          // il componente scelto, nel pannello di destra
	PropScelta        *propostaScelta         // il nodo proposto scelto, nel pannello di destra
	Nas               *nasVista               // il cassetto «Importa dal NAS»
	NasConfigurato    bool                    // c'e' una radice del NAS da cui importare
	// ChiaveCorpo dice che cosa mostra il corpo del pannello di destra (vedi chiaveCorpo); RifaiCorpo: la
	// pagina ne mostra un altro, e la risposta lo rifa' fuori banda.
	ChiaveCorpo string
	RifaiCorpo  bool
}

// chiaveCorpo dice che cosa mostra il corpo del pannello di destra: il PDF di un file (l'iframe), il
// riepilogo dell'analisi di un modello con gli stati dei nodi che propone, un file senza anteprima, niente.
// La pagina la rimanda con ogni richiesta (hx-include) e la risposta rifa' il corpo solo se e' cambiata:
// un gesto non ricarica il PDF che si sta guardando, ma dopo una conferma il pannello non resta a mostrare
// un file o degli stati che non sono piu' quelli.
func chiaveCorpo(a *anteprimaDati) string {
	if a == nil {
		return "vuoto"
	}
	f, id := a.F, a.F.A.AllegatoID.String()
	switch {
	case f.Pdf():
		if (f.A.PathStaging.Valid && !f.FileMancante) || (f.Doc != nil && f.Doc.StatoNas == db.StatoNasScritto) {
			return "pdf:" + id
		}
		return "pdf-mancante:" + id
	case f.Modello():
		h := sha256.New()
		if a.Analisi == nil {
			fmt.Fprint(h, "senza analisi|")
		} else {
			fmt.Fprintf(h, "v%d|%s|", a.Analisi.Versione, a.Analisi.MotivoParziale)
		}
		for _, p := range a.Proposte {
			fmt.Fprintf(h, "%s=%s|", p.Chiave, p.Stato)
		}
		return "modello:" + id + ":" + hex.EncodeToString(h.Sum(nil)[:6])
	}
	return "altro:" + id
}

// NDaVerificare e' il numero del cassetto «Da verificare»: le decisioni del piano, le rimozioni proposte e
// gli STEP strutturali superati da uno nuovo.
func (d *fascicoloDati) NDaVerificare() int {
	n := d.Piano.Decisioni()
	for _, rr := range d.Rimozioni {
		n += len(rr)
	}
	for _, sp := range d.StepProdotto {
		if sp.Esito == fascicolo.StepRiferimentoSuperato {
			n++
		}
	}
	return n
}

// VistaBom dice se l'area principale e' la BOM visuale.
func (d *fascicoloDati) VistaBom() bool { return d.Stato.Vista == "" }

// VistaTecnica dice se l'area principale e' una delle viste tecniche.
func (d *fascicoloDati) VistaTecnica() bool {
	return d.Stato.Vista == "griglia" || d.Stato.Vista == "completezza" || d.Stato.Vista == "albero"
}

// NonAssegnati e' il conteggio della testata.
func (d *fascicoloDati) NonAssegnati() int { return d.Conteggi[filtroNonAssegnati] }

// Filtri sono le linguette del pannello Documenti.
func (d *fascicoloDati) Filtri() []struct{ Chiave, Nome string } { return filtriFascicolo }

// NodoScelto dice se un componente e' il nodo scelto.
func (d *fascicoloDati) NodoScelto(id uuid.UUID) bool {
	return d.Nodo != nil && d.Nodo.C.ComponenteID == id
}

// CodiceDi e' il codice di un componente, per le frasi.
func (d *fascicoloDati) CodiceDi(id uuid.UUID) string {
	if c, ok := d.Componenti[id]; ok {
		return c.Codice
	}
	return ""
}

// ColonneGriglia sono i tipi di documento della griglia: i tre tecnici, piu' gli altri che il
// fabbisogno di questa RFQ chiede.
func (d *fascicoloDati) ColonneGriglia() []db.TipoDocumento {
	cols := []db.TipoDocumento{db.TipoDocumentoCad3d, db.TipoDocumentoDisegno2d, db.TipoDocumentoSviluppoDxf}
	visto := map[db.TipoDocumento]bool{}
	for _, c := range cols {
		visto[c] = true
	}
	var altri []db.TipoDocumento
	for _, cc := range d.Completezza {
		for _, c := range cc {
			if !visto[c.Tipo] {
				visto[c.Tipo] = true
				altri = append(altri, c.Tipo)
			}
		}
	}
	sort.Slice(altri, func(i, j int) bool { return altri[i] < altri[j] })
	return append(cols, altri...)
}

// Cella e' il requisito di un tipo per un componente, per la griglia; nil = il fascicolo non lo chiede.
func (d *fascicoloDati) Cella(comp uuid.UUID, tipo db.TipoDocumento) *cella {
	for i, c := range d.Completezza[comp] {
		if c.Tipo == tipo {
			return &d.Completezza[comp][i]
		}
	}
	return nil
}

// Step e' l'esito dello STEP del prodotto finito, se il componente e' in v_step_prodotto.
func (d *fascicoloDati) Step(comp uuid.UUID) *db.VStepProdotto {
	if s, ok := d.StepProdotto[comp]; ok {
		return &s
	}
	return nil
}

func etichettaDoc(t db.TipoDocumento) string {
	switch t {
	case db.TipoDocumentoCad3d:
		return "3D"
	case db.TipoDocumentoDisegno2d:
		return "2D"
	case db.TipoDocumentoSviluppoDxf:
		return "DXF"
	}
	return string(t)
}

// cellaDa traduce una riga di v_fascicolo nel simbolo della barra (piano §7.6).
func cellaDa(r db.VFascicolo) cella {
	c := cella{Tipo: r.TipoDocumento, Etichetta: etichettaDoc(r.TipoDocumento), Deroga: r.DerogaID, Bloccante: r.Bloccante}
	switch r.Esito {
	case "ok":
		c.Simbolo, c.Classe, c.Titolo = "✓", "ok", "presente, sul NAS"
	case "ok_in_coda":
		c.Simbolo, c.Classe, c.Titolo = "✓°", "coda", "presente, copia sul NAS in coda"
	case "ok_errore_nas":
		c.Simbolo, c.Classe, c.Titolo = "✓!", "urg", "presente, ma il NAS è in errore o ha un'anomalia"
	case "derogato":
		c.Simbolo, c.Classe, c.Titolo = "≈", "info", "derogato"
	case "da_confermare":
		c.Simbolo, c.Classe, c.Titolo = "?", "warn", "proposto, non ancora confermato"
	case "sul_portale":
		c.Simbolo, c.Classe, c.Titolo = "⇢", "info", "sul portale del cliente, da scaricare"
	default:
		if r.Bloccante {
			c.Simbolo, c.Classe, c.Titolo = "✗", "urg", "mancante, obbligatorio"
		} else {
			c.Simbolo, c.Classe, c.Titolo = "○", "no", "mancante, facoltativo"
		}
	}
	if r.RevDiversa {
		c.Titolo += "; la revisione del documento non è quella del componente"
	}
	return c
}

// caricaFascicolo legge tutta la schermata. Un pezzo che non si legge (i codici, per esempio) diventa un
// avviso; la pagina si apre lo stesso.
func (s *Server) caricaFascicolo(ctx context.Context, thread uuid.UUID, st statoFascicolo, u *db.Utente) (*fascicoloDati, error) {
	q := db.New(s.Pool)
	t, err := q.GetThread(ctx, thread)
	if err != nil {
		return nil, err
	}
	d := &fascicoloDati{T: t, Stato: st, Base: "/thread/" + thread.String() + "/fascicolo",
		Scrive: almeno(u, db.RuoloUtenteOperatore), Caricamento: s.Pipeline != nil && s.Staging != ""}
	d.Riga, _ = q.GetCruscottoRiga(ctx, thread)
	d.Cliente, _ = q.GetCliente(ctx, t.ClienteID)
	if f, err := q.GetFaseApertaPerThread(ctx, thread); err == nil {
		d.Fase = &f
	}
	if err := s.versioniFascicolo(ctx, q, d); err != nil {
		return nil, err
	}

	comp, err := q.ListComponentiThread(ctx, thread)
	if err != nil {
		return nil, err
	}
	rel, err := q.ListRelazioniAttive(ctx, thread)
	if err != nil {
		return nil, err
	}
	d.Albero = fascicolo.NuovoAlbero(comp, rel)
	d.Componenti = map[uuid.UUID]db.Componente{}
	for _, c := range comp {
		d.Componenti[c.ComponenteID] = c
	}
	righe, err := q.ListFascicolo(ctx, thread)
	if err != nil {
		return nil, err
	}
	d.Completezza = map[uuid.UUID][]cella{}
	for _, r := range righe {
		d.Completezza[r.ComponenteID] = append(d.Completezza[r.ComponenteID], cellaDa(r))
	}
	sp, err := q.ListStepProdotto(ctx, thread)
	if err != nil {
		return nil, err
	}
	d.StepProdotto = map[uuid.UUID]db.VStepProdotto{}
	for _, x := range sp {
		d.StepProdotto[x.ComponenteID] = x
	}
	if b, err := q.GetBloccantiThread(ctx, thread); err == nil {
		d.NBloccanti = b.NBloccanti
	}
	d.NAnalisi, _ = q.ContaAnalisiInCorso(ctx, uuid.NullUUID{UUID: thread, Valid: true})
	if n, err := q.ContaAnomalieThread(ctx, thread); err == nil {
		d.NAnomalie = int(n)
	}

	documenti, err := q.ListDocumentiThread(ctx, thread)
	if err != nil {
		return nil, err
	}
	perId := map[uuid.UUID]db.Documento{}
	d.DocumentiDi = map[uuid.UUID][]db.Documento{}
	for _, x := range documenti {
		perId[x.DocumentoID] = x
		if x.ComponenteID.Valid {
			d.DocumentiDi[x.ComponenteID.UUID] = append(d.DocumentiDi[x.ComponenteID.UUID], x)
		}
	}
	if err := s.proposteFascicolo(ctx, q, d, perId); err != nil {
		return nil, err
	}
	if err := s.fileFascicolo(ctx, q, d, perId); err != nil {
		return nil, err
	}
	if st.Nodo != uuid.Nil {
		if c, ok := d.Componenti[st.Nodo]; ok {
			d.Nodo, err = s.schedaDelNodo(ctx, q, d, c, rel)
			if err != nil {
				return nil, err
			}
		}
	}
	d.filtraFile()
	if st.Cassetto == "codici" {
		if c, err := fascicolo.CandidatiDellaRfq(ctx, q, thread); err != nil {
			d.CodiciErrore = "i codici della richiesta non si sono potuti leggere: " + err.Error()
		} else {
			d.Codici = c
		}
	}
	s.avvisiFascicolo(ctx, q, d)

	// B8.7b: il piano, il lavoro in corso, la BOM visuale, il dettaglio a destra
	if d.Piano, err = fascicolo.LeggiPianoFascicolo(ctx, q, thread); err != nil {
		d.ErrorePiano = "il piano del Fascicolo non si è potuto calcolare: " + err.Error()
	}
	if d.Lavoro, err = fascicolo.LavoroInCorso(ctx, q, thread); err != nil {
		d.Avanzamento.Errore = err.Error()
	}
	switch st.Vista {
	case "":
		d.Carte = costruisciBom(d)
	case "componenti":
		d.Schede = componentiInCard(d)
	}
	if err := s.pannelloDestro(ctx, q, d); err != nil {
		return nil, err
	}
	d.ChiaveCorpo = chiaveCorpo(d.Anteprima)
	d.NasConfigurato = s.NAS != nil && s.NAS.Radice != "" && d.Caricamento
	if st.Cassetto == "nas" && d.Scrive {
		d.Nas = s.nasVista(ctx, q, d)
	}
	d.Avanzamento = avanzamento{Base: d.Base, Stato: st, Lavoro: d.Lavoro, Firma: firmaDi(d), Attesa: attesaDopo(0), Errore: d.Avanzamento.Errore}
	return d, nil
}

// versioniFascicolo legge le versioni della BOM, la differenza della bozza e che cosa la fase ammette.
func (s *Server) versioniFascicolo(ctx context.Context, q *db.Queries, d *fascicoloDati) error {
	v, err := q.ListBomVersioni(ctx, d.T.ThreadID)
	if err != nil {
		return err
	}
	d.Versioni = v
	if len(v) > 0 {
		u := v[len(v)-1]
		d.Ultima = &u
		switch u.Stato {
		case db.StatoBomCongelata:
			d.Bloccata = u.Numero
		case db.StatoBomBozza:
			d.Bozza = &u
			if u.VersionePrecedenteID.Valid {
				if d.Differenze, err = fascicolo.DifferenzeWorking(ctx, q, d.T.ThreadID, u.VersionePrecedenteID.UUID); err != nil {
					return err
				}
			}
		}
	}
	if d.Fase != nil {
		f := d.Fase.NomeFase
		switch f {
		case db.FaseACCETTATA, db.FaseDISTINTAERP:
			d.SceltaContesto = true
		case db.FaseSCHEDACOSTO, db.FaseOFFERTEFORN, db.FaseOFFERTAINVIATA:
			d.ContestoFisso = string(db.ContestoBomPreventivo)
		case db.FaseORDINE, db.FasePRODUZIONE:
			d.ContestoFisso = string(db.ContestoBomTecnica)
		}
		d.PuoAprire = d.Bloccata > 0 && (d.SceltaContesto || d.ContestoFisso != "")
		switch {
		case d.Bozza != nil && d.Bozza.Contesto == db.ContestoBomPreventivo:
			d.PuoCongelare = f == db.FaseFATTIBILITA
		case d.Bozza != nil:
			d.PuoCongelare = f == db.FaseACCETTATA || f == db.FaseDISTINTAERP || f == db.FaseORDINE || f == db.FasePRODUZIONE
		case d.Ultima == nil:
			d.PuoCongelare = f == db.FaseFATTIBILITA || f == db.FaseORDINE || f == db.FasePRODUZIONE
		}
	}
	if d.Bloccata == 0 {
		if d.Gate, err = fascicolo.LeggiGate(ctx, q, d.T.ThreadID); err != nil {
			return err
		}
	}
	return nil
}

// proposteFascicolo mette le proposte di struttura dove la schermata le mostra: sotto il componente del
// padre proposto, se il padre e' gia' nella working; altrimenti nel blocco del loro file.
func (s *Server) proposteFascicolo(ctx context.Context, q *db.Queries, d *fascicoloDati, documenti map[uuid.UUID]db.Documento) error {
	nodi, err := q.ListComponenteProposteThread(ctx, d.T.ThreadID)
	if err != nil {
		return err
	}
	rel, err := q.ListRelazioneProposteThread(ctx, d.T.ThreadID)
	if err != nil {
		return err
	}
	d.NodiProposti, d.RelazioniProposte = nodi, rel
	type chiave struct {
		a uuid.UUID
		k string
	}
	perChiave := map[chiave]db.ComponenteProposta{}
	nomi := map[uuid.UUID]string{}
	var ordine []uuid.UUID
	for _, n := range nodi {
		perChiave[chiave{n.ComponenteProposta.AllegatoID, n.ComponenteProposta.Chiave}] = n.ComponenteProposta
		if _, ok := nomi[n.ComponenteProposta.AllegatoID]; !ok {
			nomi[n.ComponenteProposta.AllegatoID] = n.NomeFile
			ordine = append(ordine, n.ComponenteProposta.AllegatoID)
		}
	}
	attivo := func(p db.ComponenteProposta) bool {
		if !p.ComponenteID.Valid {
			return false
		}
		c, ok := d.Componenti[p.ComponenteID.UUID]
		return ok && c.ArchiviatoIl == nil
	}
	mostra := func(stato db.StatoProposta) bool {
		return stato == db.StatoPropostaAperta || (d.Stato.Scartate && stato == db.StatoPropostaScartata)
	}
	d.Inline = map[uuid.UUID][]figlioProposto{}
	blocchi := map[uuid.UUID]*bloccoFile{}
	blocco := func(a uuid.UUID) *bloccoFile {
		if b, ok := blocchi[a]; ok {
			return b
		}
		b := &bloccoFile{Allegato: a, Nome: nomi[a]}
		blocchi[a] = b
		return b
	}
	sotto := map[chiave]string{}
	inline := map[chiave]bool{}
	for _, r := range rel {
		x := r.RelazioneProposta
		pn, fn := perChiave[chiave{x.AllegatoID, x.PadreChiave}], perChiave[chiave{x.AllegatoID, x.FiglioChiave}]
		sotto[chiave{x.AllegatoID, x.FiglioChiave}] = etichettaNodo(pn)
		if !mostra(x.Stato) {
			continue
		}
		if x.Stato == db.StatoPropostaAperta {
			d.NProposte++
		}
		if attivo(pn) && x.Stato == db.StatoPropostaAperta {
			d.Inline[pn.ComponenteID.UUID] = append(d.Inline[pn.ComponenteID.UUID], figlioProposto{R: x, File: r.NomeFile, Padre: pn, Figlio: fn})
			if fn.Stato == db.StatoPropostaAperta {
				inline[chiave{x.AllegatoID, x.FiglioChiave}] = true
			}
			continue
		}
		b := blocco(x.AllegatoID)
		b.Relazioni = append(b.Relazioni, relazioneProposta{R: x, Padre: pn, Figlio: fn})
		if x.Stato == db.StatoPropostaAperta {
			b.Aperte++
		}
	}
	for _, n := range nodi {
		p := n.ComponenteProposta
		if !mostra(p.Stato) {
			continue
		}
		if p.Stato == db.StatoPropostaAperta {
			d.NProposte++
		}
		k := chiave{p.AllegatoID, p.Chiave}
		if inline[k] {
			continue
		}
		b := blocco(p.AllegatoID)
		b.Nodi = append(b.Nodi, nodoProposto{P: p, Sotto: sotto[k]})
		if p.Stato == db.StatoPropostaAperta {
			b.Aperte++
		}
	}
	for _, a := range ordine {
		if b, ok := blocchi[a]; ok {
			d.Blocchi = append(d.Blocchi, *b)
		}
	}
	rim, err := q.ListRimozioniAperte(ctx, d.T.ThreadID)
	if err != nil {
		return err
	}
	d.Rimozioni = map[uuid.UUID][]rimozione{}
	for _, r := range rim {
		d.NProposte++
		d.Rimozioni[r.PadreID] = append(d.Rimozioni[r.PadreID], rimozione{R: r, Figlio: d.Componenti[r.FiglioID], Step: documenti[r.StepDocumentoID].NomeFile})
	}
	return nil
}

// etichettaNodo e' come si chiama un nodo proposto: il codice, altrimenti il nome nel file.
func etichettaNodo(p db.ComponenteProposta) string {
	if c := strings.TrimSpace(p.Codice.String); c != "" {
		return c
	}
	if p.NomeGrezzo != "" {
		return "«" + p.NomeGrezzo + "»"
	}
	return p.Chiave
}

// fileFascicolo costruisce le righe del pannello Documenti.
func (s *Server) fileFascicolo(ctx context.Context, q *db.Queries, d *fascicoloDati, documenti map[uuid.UUID]db.Documento) error {
	tid := uuid.NullUUID{UUID: d.T.ThreadID, Valid: true}
	allegati, err := q.ListAllegatiFascicolo(ctx, tid)
	if err != nil {
		return err
	}
	proposte, err := q.ListProposteDocumentoThread(ctx, tid)
	if err != nil {
		return err
	}
	perAllegato := map[uuid.UUID]db.DocumentoProposta{}
	for _, p := range proposte {
		perAllegato[p.AllegatoID] = p
	}
	prov, err := q.ListProvenienzeThread(ctx, d.T.ThreadID)
	if err != nil {
		return err
	}
	docDi := map[uuid.UUID]uuid.UUID{}
	d.AllegatoDi = map[uuid.UUID]uuid.UUID{}
	for _, p := range prov {
		if _, gia := docDi[p.AllegatoID.UUID]; !gia {
			docDi[p.AllegatoID.UUID] = p.DocumentoID
		}
		if _, gia := d.AllegatoDi[p.DocumentoID]; !gia {
			d.AllegatoDi[p.DocumentoID] = p.AllegatoID.UUID
		}
	}
	for _, a := range allegati {
		f := rigaFile{A: a}
		if a.PathStaging.Valid && a.PathStaging.String != "" {
			if _, err := os.Stat(a.PathStaging.String); err != nil {
				f.FileMancante = true
			}
		}
		if p, ok := perAllegato[a.AllegatoID]; ok {
			p := p
			f.Proposta = &p
			f.Tipo, f.Codice, f.Rev = string(p.TipoProposto), p.Codice.String, p.Rev.String
			var dett struct {
				RevLetta    string `json:"rev_letta"`
				CodiceLetto string `json:"codice_letto"`
			}
			_ = json.Unmarshal(p.Dettagli, &dett)
			f.RevLetta, f.CodiceLetto = dett.RevLetta, dett.CodiceLetto
			if p.ComponenteID.Valid {
				if c, ok := d.Componenti[p.ComponenteID.UUID]; ok {
					f.Comp = &c
				}
			}
		}
		if id, ok := docDi[a.AllegatoID]; ok {
			if x, ok := documenti[id]; ok {
				f.Doc = &x
				f.Tipo, f.Codice, f.Rev = string(x.Tipo), x.Codice.String, x.Rev.String
				f.Comp = nil
				if x.ComponenteID.Valid {
					if c, ok := d.Componenti[x.ComponenteID.UUID]; ok {
						f.Comp = &c
					}
				}
			}
		}
		f.Stato, f.ClasseStato = statoFile(f)
		d.File = append(d.File, f)
	}
	return nil
}

// statoFile e' la colonna «stato» del pannello Documenti (piano §7.4).
func statoFile(f rigaFile) (string, string) {
	switch {
	case f.Doc != nil && f.Doc.SostituitoDa.Valid:
		return "sostituito", "no"
	case f.Doc != nil:
		switch f.Doc.StatoNas {
		case db.StatoNasScritto:
			return "confermato · NAS ✓", "ok"
		case db.StatoNasErrore:
			return "confermato · NAS in errore", "urg"
		}
		return "confermato · NAS in coda", "coda"
	case f.FileMancante:
		return "file mancante", "urg"
	case f.Proposta != nil && f.Proposta.Stato == db.StatoPropostaScartata:
		return "scartato", "no"
	case f.Proposta != nil && f.Proposta.Stato == db.StatoPropostaDuplicato:
		return "già nel fascicolo", "no"
	}
	switch f.A.Stato {
	case db.StatoAllegatoGrezzo:
		if f.A.Errore.Valid && f.A.Errore.String == "in coda" {
			return "download in coda", "warn"
		}
		return "da scaricare", "no"
	case db.StatoAllegatoErrore:
		return "errore: " + f.A.Errore.String, "urg"
	case db.StatoAllegatoInStaging:
		return "in staging · analisi", "warn"
	case db.StatoAllegatoAnalizzato:
		return "analizzato", "info"
	case db.StatoAllegatoIgnorato:
		return "ignorato", "no"
	}
	return string(f.A.Stato), "no"
}

// filtraFile applica il filtro e conta le righe di ogni linguetta. Uno zip e' un contenitore: le sue voci
// sono i file, e lui non si assegna.
func (d *fascicoloDati) filtraFile() {
	d.Conteggi = map[string]int{}
	var nodo *db.Componente
	if d.Nodo != nil {
		nodo = &d.Nodo.C
	}
	for i := range d.File {
		f := &d.File[i]
		zip := f.estensione() == "zip"
		assegnato := f.Comp != nil
		nonAssegnato := !assegnato && !zip && !f.Rumore() && !(f.Doc != nil && f.Doc.SostituitoDa.Valid) &&
			(f.Doc != nil || (f.Proposta != nil && f.Proposta.Stato == db.StatoPropostaAperta))
		delNodo := nodo != nil && assegnato && f.Comp.ComponenteID == nodo.ComponenteID
		candidato := nodo != nil && nonAssegnato && stessoCodice(f.Codice, nodo.Codice) &&
			(d.Stato.Tipo == "" || f.Tipo == d.Stato.Tipo)
		conta := map[string]bool{filtroTutti: true, filtroNonAssegnati: nonAssegnato, filtroNodo: delNodo, filtroCandidati: candidato,
			filtroDaScaricare: f.DaScaricare(), filtroRumore: f.Rumore()}
		for k, v := range conta {
			if v {
				d.Conteggi[k]++
			}
		}
		if conta[d.Stato.FiltroAttivo()] {
			if nodo != nil && f.Doc != nil && !delNodo && !f.Doc.SostituitoDa.Valid {
				for _, x := range d.DocumentiDi[nodo.ComponenteID] {
					if x.Tipo == f.Doc.Tipo && !x.SostituitoDa.Valid && x.DocumentoID != f.Doc.DocumentoID {
						f.Scelta = append(f.Scelta, x)
						if nodo.StepStrutturaleID.Valid && nodo.StepStrutturaleID.UUID == x.DocumentoID {
							f.StepStrutturale = x.DocumentoID
						}
					}
				}
			}
			d.Visibili = append(d.Visibili, *f)
		}
	}
}

// schedaDelNodo raccoglie quello che serve ai gesti sul componente scelto.
func (s *Server) schedaDelNodo(ctx context.Context, q *db.Queries, d *fascicoloDati, c db.Componente, rel []db.ComponenteRelazione) (*schedaNodo, error) {
	n := &schedaNodo{C: c, Documenti: d.DocumentiDi[c.ComponenteID], Richiesti: d.Completezza[c.ComponenteID]}
	for _, r := range rel {
		if r.FiglioID == c.ComponenteID {
			n.Padri = append(n.Padri, arcoNodo{Padre: d.Componenti[r.PadreID], Qta: r.Qta})
		}
	}
	for _, x := range n.Documenti {
		if fascicolo.Step(x) && !x.SostituitoDa.Valid {
			n.Step = append(n.Step, x)
		}
	}
	if sp, ok := d.StepProdotto[c.ComponenteID]; ok {
		n.StepEsito = &sp
		if sp.DerogaStrutturaID.Valid {
			ds, err := q.ListDerogheStrutturaThread(ctx, d.T.ThreadID)
			if err != nil {
				return nil, err
			}
			for _, x := range ds {
				if x.DerogaStrutturaID == sp.DerogaStrutturaID.UUID {
					x := x
					n.DerogaStep = &x
				}
			}
		}
	}
	der, err := q.ListDerogheThread(ctx, d.T.ThreadID)
	if err != nil {
		return nil, err
	}
	for _, x := range der {
		if x.ComponenteID == c.ComponenteID {
			n.Deroghe = append(n.Deroghe, x)
		}
	}
	for _, r := range d.Albero.Righe {
		if r.C.ComponenteID != c.ComponenteID {
			n.Candidati = append(n.Candidati, r.C)
		}
	}
	sort.SliceStable(n.Candidati, func(i, j int) bool {
		return strings.ToUpper(n.Candidati[i].Codice) < strings.ToUpper(n.Candidati[j].Codice)
	})
	return n, nil
}

// anteprimaFascicolo prepara il file aperto: la riga, la storia, e per un modello la lettura dell'analisi.
func (s *Server) anteprimaFascicolo(ctx context.Context, q *db.Queries, d *fascicoloDati, file uuid.UUID) (*anteprimaDati, error) {
	var riga *rigaFile
	for i := range d.File {
		if d.File[i].A.AllegatoID == file {
			riga = &d.File[i]
		}
	}
	if riga == nil {
		return nil, nil
	}
	a := &anteprimaDati{F: *riga}
	if riga.Confermabile() {
		if riga.Proposta.ComponenteID.Valid {
			if c, ok := d.Componenti[riga.Proposta.ComponenteID.UUID]; ok {
				a.ConfermaComp = &c
			}
		} else if d.Nodo != nil {
			c := d.Nodo.C
			a.ConfermaComp = &c
		}
		if a.ConfermaComp != nil {
			for _, x := range d.DocumentiDi[a.ConfermaComp.ComponenteID] {
				if x.Tipo == riga.Proposta.TipoProposto && !x.SostituitoDa.Valid {
					a.ConfermaCorrenti = append(a.ConfermaCorrenti, x)
					if a.ConfermaComp.StepStrutturaleID.Valid && a.ConfermaComp.StepStrutturaleID.UUID == x.DocumentoID {
						a.StepFraCorrenti = true
					}
				}
			}
		}
	}
	if riga.Doc != nil {
		storia, err := q.ListStoriaDocumento(ctx, riga.Doc.DocumentoID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if len(storia) > 1 {
			a.Storia = storia
			for i := range storia {
				for _, c := range storia {
					if c.Corrente && storia[i].SostituitoDa.Valid && storia[i].SostituitoDa.UUID == c.DocumentoID {
						x := storia[i]
						a.Annullabile = &x
					}
				}
			}
		}
		if riga.Doc.ComponenteID.Valid && !riga.Doc.SostituitoDa.Valid {
			for _, x := range d.DocumentiDi[riga.Doc.ComponenteID.UUID] {
				if x.Tipo == riga.Doc.Tipo && !x.SostituitoDa.Valid && x.DocumentoID != riga.Doc.DocumentoID {
					a.Sostituibili = append(a.Sostituibili, x)
				}
			}
			if c, ok := d.Componenti[riga.Doc.ComponenteID.UUID]; ok && c.StepStrutturaleID.Valid && c.StepStrutturaleID.UUID == riga.Doc.DocumentoID {
				a.StepDi = &c
			}
			for _, x := range a.Sostituibili {
				if c, ok := d.Componenti[riga.Doc.ComponenteID.UUID]; ok && c.StepStrutturaleID.Valid && c.StepStrutturaleID.UUID == x.DocumentoID {
					a.StepFraCorrenti = true
				}
			}
		}
	}
	if riga.Modello() && riga.A.Sha256.Valid {
		r, err := s.riepilogoAnalisi(ctx, q, riga.A.Sha256.String)
		if err != nil {
			return nil, err
		}
		a.Analisi = r
		nodi, err := q.ListComponenteProposteFile(ctx, db.ListComponenteProposteFileParams{ThreadID: d.T.ThreadID, AllegatoID: file})
		if err != nil {
			return nil, err
		}
		a.Proposte = nodi
	}
	return a, nil
}

// riepilogoAnalisi legge i fatti correnti di un contenuto: quelli della chiave dell'analizzatore
// corrente (A4.5), o in mancanza quelli della configurazione di questo server.
func (s *Server) riepilogoAnalisi(ctx context.Context, q *db.Queries, sha string) (*riepilogoAnalisi, error) {
	af, err := q.GetAnalisiCorrente(ctx, sha)
	if errors.Is(err, pgx.ErrNoRows) && s.Analizzatore.Versione > 0 {
		af, err = q.GetAnalisiFatti(ctx, db.GetAnalisiFattiParams{Sha256: sha, VersioneAnalizzatore: int16(s.Analizzatore.Versione),
			HashConfigurazione: s.Analizzatore.Hash()})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r := &riepilogoAnalisi{Versione: af.VersioneAnalizzatore}
	var campi map[string]json.RawMessage
	_ = json.Unmarshal(af.Fatti, &campi)
	for k := range campi {
		if k != "struttura" && k != "esito" {
			r.Altri = append(r.Altri, k)
		}
	}
	sort.Strings(r.Altri)
	if st, ok := worker.DecodificaStruttura(af.Fatti); ok {
		r.Struttura = st
		r.Nomi = map[string]string{}
		for _, n := range st.Nodi {
			nome := n.IDGrezzo
			if nome == "" {
				nome = n.NomeGrezzo
			}
			r.Nomi[n.Chiave] = nome
		}
		if grezza, ok := campi["struttura"]; ok {
			m, err := q.MotivoParziale(ctx, &grezza)
			if err != nil {
				return nil, err
			}
			r.MotivoParziale = m
		}
	}
	return r, nil
}

// avvisiFascicolo riempie il cassetto Avvisi: che cosa ferma il congelamento e che cosa va guardato.
func (s *Server) avvisiFascicolo(ctx context.Context, q *db.Queries, d *fascicoloDati) {
	for _, p := range d.Gate.Problemi {
		d.Avvisi = append(d.Avvisi, avvisoFascicolo{"urg", "Per congelare: " + p})
	}
	for _, a := range d.Gate.Avvisi {
		d.Avvisi = append(d.Avvisi, avvisoFascicolo{"warn", a})
	}
	if d.Bloccata > 0 {
		if r, err := q.ListThreadDaRiesaminare(ctx, d.T.ThreadID); err == nil {
			for _, x := range r {
				d.Avvisi = append(d.Avvisi, avvisoFascicolo{"warn", x.Motivo})
			}
		}
	}
	for _, sp := range d.StepProdotto {
		if sp.Esito != fascicolo.StepAnalizzato && sp.Esito != fascicolo.StepRadiceSenzaQualifica {
			d.Avvisi = append(d.Avvisi, avvisoFascicolo{"warn", sp.Codice + ": " + fascicolo.EtichettaStep(sp.Esito)})
		}
	}
	mancanti := 0
	for _, f := range d.File {
		if f.FileMancante {
			mancanti++
		}
	}
	if mancanti > 0 {
		d.Avvisi = append(d.Avvisi, avvisoFascicolo{"warn", conta(mancanti, "file non è più nello staging", "file non sono più nello staging") + ": si riscaricano"})
	}
	if d.NAnomalie > 0 {
		d.Avvisi = append(d.Avvisi, avvisoFascicolo{"urg", conta(d.NAnomalie, "documento non corrisponde", "documenti non corrispondono") + " al file sul NAS (Integrità NAS)"})
	}
	for _, cc := range d.Completezza {
		for _, c := range cc {
			if strings.Contains(c.Titolo, "revisione del documento") {
				d.Avvisi = append(d.Avvisi, avvisoFascicolo{"info", c.Etichetta + ": " + c.Titolo})
			}
		}
	}
	sort.SliceStable(d.Avvisi, func(i, j int) bool { return pesoAvviso(d.Avvisi[i].Livello) < pesoAvviso(d.Avvisi[j].Livello) })
}

// ------------------------------------------------------------------ per il template

// nodoVista e' un nodo dell'albero con la schermata, per il template ricorsivo.
type nodoVista struct {
	D *fascicoloDati
	N *fascicolo.Nodo
}

// propostaVista e' una riga tratteggiata con la schermata.
type propostaVista struct {
	D *fascicoloDati
	P figlioProposto
}

// etichettaTipoDoc e' il nome breve di un tipo di documento: 3D, 2D, DXF, gli altri con gli spazi.
func etichettaTipoDoc(t string) string {
	switch db.TipoDocumento(t) {
	case db.TipoDocumentoCad3d, db.TipoDocumentoDisegno2d, db.TipoDocumentoSviluppoDxf:
		return etichettaDoc(db.TipoDocumento(t))
	}
	return strings.ReplaceAll(t, "_", " ")
}

// classeStep e' il colore dell'esito dello STEP del prodotto finito.
func classeStep(esito string) string {
	switch esito {
	case fascicolo.StepAnalizzato:
		return "ok"
	case fascicolo.StepParziale, fascicolo.StepNonAnalizzato, fascicolo.StepDaScegliere, fascicolo.StepRiferimentoSuperato, fascicolo.StepDaConfermare:
		return "warn"
	case fascicolo.StepSulPortale:
		return "info"
	case fascicolo.StepRadiceSenzaQualifica:
		return "no"
	}
	return "urg"
}

// simboloStep e' il simbolo dell'esito nella barra di completezza.
func simboloStep(esito string) string {
	switch esito {
	case fascicolo.StepAnalizzato:
		return "✓"
	case fascicolo.StepParziale:
		return "½"
	case fascicolo.StepNonAnalizzato:
		return "…"
	case fascicolo.StepDaScegliere, fascicolo.StepRiferimentoSuperato:
		return "?"
	case fascicolo.StepSulPortale:
		return "⇢"
	case fascicolo.StepDaConfermare:
		return "?"
	}
	return "✗"
}

// mittente e' chi ha mandato il file: il nome, altrimenti l'indirizzo.
func mittente(nome, indirizzo pgtype.Text) string {
	if nome.Valid && strings.TrimSpace(nome.String) != "" {
		return nome.String
	}
	if indirizzo.Valid {
		return indirizzo.String
	}
	return "—"
}

func pesoAvviso(l string) int {
	switch l {
	case "urg":
		return 0
	case "warn":
		return 1
	}
	return 2
}
