package fascicolo

// L'albero proposto della Distinta (giro 4, fase 4.4a.1a; domande 27 = A, 28 = A, 29a, 29b, 29e = A, 30 seconda
// risposta, 6a, 6b; studio docs/specs/studio_albero_distinta_29-09.md § 1-2, con la verifica in coda). E' la LETTURA:
// niente qui scrive. La conferma dell'albero, che scrive, e' la fase 4.4a.1b (albero_conferma.go); la pagina la 4.4a.3.
//
// Per la RFQ una foresta: un albero per ogni prodotto confermato (ProdottiDellaRfq). L'albero parte dai file del
// prodotto (lo STEP che F8 riconosce come la sua ancora, o lo STEP autorizzato) e scende a TUTTI i livelli di quel file
// (le righe di componente_proposta e relazione_proposta, gia' nel database), e poi negli STEP dei sottoassiemi che si
// raggiungono per codice: sono gli stessi che l'indice di F8 cammina (IndiceCodici: Sottoassiemi, UsatoDa), cosi'
// l'albero e le destinazioni dello smistamento dicono la stessa struttura. Senza uno STEP del prodotto l'albero ha un
// livello solo, il prodotto: con il solo disegno le righe dell'elenco particolari entrano nell'albero con la fase 4.12.
// Sopra ci sta la working: i componenti e gli archi che ci sono gia' sotto il prodotto.
//
// L'identita' e' quella della RFQ, (thread, codice): un figlio in comune e' un nodo solo con piu' padri, e un arco e'
// uno per (padre, figlio), con i file che lo dicono e la quantita' di ciascuno. Quantita' diverse fra i file sono
// discordi, e non vince nessuna (6c): la sceglie una persona. Un nodo senza codice e' un nodo a se', con la sua riga.
//
// Per ogni nodo:
//   - il codice proposto con la sua fonte: oggi quello che la normalizzazione di oggi scrive sulla riga (le regole del
//     cliente che ci sono, il suffisso decorativo tolto); le regole nuove del cliente arrivano con la fase 4.9;
//   - il tipo proposto: assieme se nell'albero ha dei figli, particolare se e' una foglia. Un particolare non ha mai
//     figli (6b): un componente particolare a cui lo STEP mette sotto dei pezzi e' proposto come assieme, e il
//     riepilogo lo dice. Mai commerciale: il commerciale e' una domanda (qui sotto), e un commerciale che c'e' e' una
//     foglia (6a), con i suoi figli dello STEP che restano guida;
//   - lo stato rispetto alla working: nella distinta, proposto, scartato da una persona, tolto dallo STEP (la
//     rimozione che lo STEP autorizzato propone su un arco della working);
//   - il componente che c'e' con lo stesso codice, per un nodo che lo STEP propone ancora («ritrovato per codice»,
//     P13): la conferma lo lega a quel componente, e il riepilogo lo dichiara;
//   - i codici quasi uguali (P4) di un pezzo che nascerebbe: una domanda, mai «diverso» segnato da solo;
//   - la proposta commerciale di una foglia che nascerebbe: il riconoscitore della minuteria sul NOME del pezzo (il
//     PRODUCT dello STEP, mai il testo del disegno), con il motivo in parole. E' una domanda aperta su quel nodo, ✓ o
//     ✗, mai un tipo scritto (domanda 30, seconda risposta). Le famiglie di codici del cliente con il ruolo commerciale
//     non ci sono ancora: sono regole scritte da una persona (fase 4.9, domanda 14).
//
// I cordoni di saldatura restano come oggi, nodi dello STEP come gli altri (la fase 4.7 non e' fatta): l'operatore li toglie.
// La firma dell'albero (il suo stato, la working, l'indice di F8) e' quella che la conferma della fase 4.4a.1b rimanda.
// Tutto qui e' puro, tranne LeggiAlberoProposto, che legge soltanto.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// AlgoritmoAlbero e' la versione dell'albero proposto: entra nella firma.
const AlgoritmoAlbero = "albero-proposto-1"

// Gli stati di un nodo e di un arco dell'albero rispetto alla working.
const (
	StatoAlberoNellaDistinta = "nella_distinta"   // il pezzo (o il legame) c'e' gia' nella working
	StatoAlberoProposto      = "proposto"         // lo propone uno STEP: nasce solo con la conferma dell'albero
	StatoAlberoScartato      = "scartato"         // una persona ha scartato la riga dello STEP: si vede, non si propone
	StatoAlberoTolto         = "tolto_dallo_step" // nella working, e lo STEP autorizzato ne propone la rimozione
)

// Da dove viene il codice di un nodo.
const (
	FonteCodiceStep      = "step"       // la riga dello STEP, con la normalizzazione di oggi
	FonteCodiceOperatore = "operatore"  // scritto da una persona sulla riga dello STEP (CodiceDelNodo)
	FonteCodiceWorking   = "componente" // il componente che c'e'
	FonteCodiceRichiesta = "richiesta"  // il codice della richiesta confermato al triage
)

// ChiaveDelCodice e' la chiave di un nodo con un codice: il codice canonico (le regole del cliente, maiuscolo).
func ChiaveDelCodice(codice string) string { return "cod:" + codice }

// chiaveSenzaCodice e' la chiave di un nodo di uno STEP senza codice: la sua riga.
func chiaveSenzaCodice(proposta uuid.UUID) string { return "nodo:" + proposta.String() }

// AlberoProposto e' la foresta della RFQ: i prodotti, i nodi (uno per pezzo) e gli archi (uno per padre e figlio).
type AlberoProposto struct {
	Algoritmo string           `json:"algoritmo"`
	Prodotti  []ProdottoAlbero `json:"prodotti"`
	Nodi      []NodoAlbero     `json:"nodi"`  // in ordine di chiave
	Archi     []ArcoAlbero     `json:"archi"` // in ordine di padre e figlio
	// SenzaPosto: gli STEP della RFQ che non stanno nell'albero, con il perche': non si raggiungono da nessun
	// prodotto («di quale pezzo e'?»), o sono un'alternativa al file che il pezzo usa.
	SenzaPosto  []FileSenzaPosto `json:"senza_posto"`
	IndiceFirma string           `json:"indice_firma"`
	Firma       string           `json:"firma"`

	per map[string]int // chiave → indice in Nodi
}

// ProdottoAlbero e' la radice di un albero: il prodotto, con i file da cui l'albero viene.
type ProdottoAlbero struct {
	Chiave string `json:"chiave"`
	Codice string `json:"codice"`
	// Ancora: il livello dell'ancora di F8 (autorizzata, piena, da_confermare, piatta, in_attesa, assente), con i suoi
	// motivi e le sue discordanze. Solo con un'ancora strutturale (le prime tre) l'albero scende nello STEP.
	Ancora      string   `json:"ancora"`
	File        []string `json:"file,omitempty"`
	Motivi      []string `json:"motivi,omitempty"`
	Discordanze []string `json:"discordanze,omitempty"`
	// SoloProdotto: l'albero ha un livello solo (nessuno STEP del prodotto); Perche' lo dice.
	SoloProdotto bool   `json:"solo_prodotto,omitempty"`
	Perche       string `json:"perche,omitempty"`
}

// NodoAlbero e' un pezzo dell'albero proposto.
type NodoAlbero struct {
	Chiave string `json:"chiave"`
	Codice string `json:"codice,omitempty"` // canonico; vuoto per un nodo senza codice
	// CodiceNelFile: il codice come lo dice la riga, quando la normalizzazione lo cambia (il suffisso del cliente tolto).
	CodiceNelFile string `json:"codice_nel_file,omitempty"`
	FonteCodice   string `json:"fonte_codice,omitempty"`
	// OrigineCodice: come la lettura della riga ha trovato il codice (famiglia del cliente, generico, nome_file…).
	OrigineCodice string `json:"origine_codice,omitempty"`
	Nome          string `json:"nome,omitempty"` // il PRODUCT dello STEP, o la descrizione del componente
	Rev           string `json:"rev,omitempty"`
	Descrizione   string `json:"descrizione,omitempty"`
	// Tipo: il tipo proposto, con il motivo. TipoAttuale: quello del componente che c'e'.
	Tipo        db.TipoComponente `json:"tipo"`
	TipoMotivo  string            `json:"tipo_motivo,omitempty"`
	TipoAttuale db.TipoComponente `json:"tipo_attuale,omitempty"`
	Stato       string            `json:"stato"`
	Prodotto    bool              `json:"prodotto,omitempty"`
	// Componente: il componente attivo con il codice del nodo, se c'e'.
	Componente uuid.NullUUID `json:"componente"`
	// Ritrovato: il nodo e' proposto da righe ancora aperte e un componente con lo stesso codice c'e' (P13).
	Ritrovato *RitrovatoAlbero `json:"ritrovato,omitempty"`
	// Vicini: i codici quasi uguali di un pezzo che nascerebbe (P4). Una domanda, senza risposta preselezionata.
	Vicini []Vicino `json:"vicini,omitempty"`
	// Commerciale: la proposta «particolare commerciale» di una foglia che nascerebbe: una domanda aperta, ✓ o ✗.
	Commerciale *DomandaCommerciale `json:"commerciale,omitempty"`
	Padri       []string            `json:"padri,omitempty"`
	Figli       int                 `json:"figli"`
	Prodotti    []string            `json:"prodotti,omitempty"` // i prodotti sotto cui sta
	// Step: lo STEP proprio di un sottoassieme, che l'albero cammina (i suoi figli vengono anche da li').
	Step        *StepDelNodo `json:"step,omitempty"`
	Righe       []RigaAlbero `json:"righe,omitempty"`
	Discordanze []string     `json:"discordanze,omitempty"`
	Note        []string     `json:"note,omitempty"`
}

// RitrovatoAlbero e' il componente con lo stesso codice di un nodo proposto (P13).
type RitrovatoAlbero struct {
	Componente uuid.UUID         `json:"componente"`
	Codice     string            `json:"codice"`
	Tipo       db.TipoComponente `json:"tipo"`
	Archiviato bool              `json:"archiviato,omitempty"`
	// Agganciato: fra le righe da decidere c'e' un aggancio per codice di prima dello Smistamento (RigaAlbero.Agganciata):
	// il riepilogo lo dice, perche' la conferma lo fa diventare la decisione di una persona (fase 4.4a.1b).
	Agganciato bool `json:"agganciato,omitempty"`
}

// DomandaCommerciale e' la proposta del tipo «particolare commerciale» per una foglia (domanda 30, seconda risposta):
// l'esito del riconoscitore, il motivo in parole e il nome da cui viene.
type DomandaCommerciale struct {
	Esito  string `json:"esito"`
	Motivo string `json:"motivo"`
	Nome   string `json:"nome"`
}

// StepDelNodo e' lo STEP di un sottoassieme che l'albero cammina, con il livello della sua ancora.
type StepDelNodo struct {
	File        []string `json:"file"`
	Livello     string   `json:"livello"`
	Discordanze []string `json:"discordanze,omitempty"`
}

// RigaAlbero e' una riga di uno STEP che porta il nodo.
type RigaAlbero struct {
	Proposta uuid.UUID        `json:"proposta"`
	Allegato uuid.UUID        `json:"allegato"`
	File     string           `json:"file"`
	Chiave   string           `json:"chiave"`
	Stato    db.StatoProposta `json:"stato"`
	// Rev: la revisione che la riga dice (maiuscola; vuota se non ne dice una). Di un componente che c'e', una
	// revisione nuova detta dalle righe ancora da decidere e' la revisione nuova dello stesso pezzo (fase 4.4a.1b).
	Rev string `json:"rev,omitempty"`
	// Agganciata: un aggancio per codice di prima dello Smistamento (AgganciatoPerCodice: duplicato con il componente,
	// senza chi l'ha deciso, la forma F-A). Non e' una decisione (U5: il gate lo conta da decidere, una persona lo
	// conferma): per l'albero e' una riga ancora da decidere come un'aperta, che il riepilogo elenca fra i ritrovati e
	// che la conferma decide (fase 4.4a.1b, dalla verifica: senza, la conferma la faceva diventare la decisione di una
	// persona senza che il riepilogo la dicesse).
	Agganciata bool `json:"agganciata,omitempty"`
	// decisa: la riga l'ha decisa una persona (deciso_da). Non va nel JSON: serve a contare, nel riepilogo, le righe
	// che la conferma chiude quando il nodo esce dall'albero (siChiudeNellAlbero).
	decisa bool
}

// daDecidere: la riga non e' ancora una decisione di nessuno, e la conferma dell'albero la decide se il riepilogo
// elenca il suo nodo: aperta, o agganciata per codice prima dello Smistamento. Una chiusa da un automatismo
// (scartata senza chi l'ha decisa) non lo e': la rilettura la riscrive, e la conferma la lascia com'e'.
func (r RigaAlbero) daDecidere() bool {
	return r.Stato == db.StatoPropostaAperta || r.Agganciata
}

// siChiudeNellAlbero: la riga che la conferma dell'albero chiude quando il suo nodo esce dall'albero, «tolto nell'albero
// confermato»: una ancora da decidere, o una decisa da una persona che non e' uno scarto (una decisione nuova sulla
// stessa riga: quella di prima va nella storia). Una chiusa da un automatismo (scartata senza chi l'ha decisa: un file
// sostituito, un riferimento cambiato) resta com'e' (fase 4.4a.1br): non e' una decisione di nessuno, la rilettura la
// riscrive (E33), e il riepilogo non la conta. Una scartata da una persona resta la sua decisione. Il riepilogo conta
// queste righe (ScartataRiepilogo.Righe) e la conferma chiude queste: gli stessi conti.
func (r RigaAlbero) siChiudeNellAlbero() bool {
	return r.daDecidere() || (r.decisa && r.Stato != db.StatoPropostaScartata)
}

// ArcoAlbero e' un legame dell'albero: il padre, il figlio, la quantita' proposta e i file che lo dicono.
type ArcoAlbero struct {
	Padre  string `json:"padre"`
	Figlio string `json:"figlio"`
	// Qta: quella della working se il legame c'e'; altrimenti quella dei file, se dicono tutti la stessa; 0 se sono
	// discordi (QtaDiscordi: non vince nessuna, 6c).
	Qta         int32       `json:"qta"`
	QtaWorking  int32       `json:"qta_working,omitempty"`
	QtaDiscordi bool        `json:"qta_discordi,omitempty"`
	Stato       string      `json:"stato"`
	Fonti       []FonteArco `json:"fonti,omitempty"`
	// Rimozione: la rimozione che lo STEP autorizzato propone su un legame della working.
	Rimozione *RimozioneAlbero `json:"rimozione,omitempty"`
}

// FonteArco e' un file che dice il legame, con la sua quantita' (la somma dei nodi con lo stesso codice sotto lo stesso
// nodo del file) e lo stato delle sue righe.
type FonteArco struct {
	Allegato uuid.UUID `json:"allegato"`
	File     string    `json:"file"`
	Qta      int32     `json:"qta"`
	Scartata bool      `json:"scartata,omitempty"`
	// Righe: le righe di relazione_proposta che fanno la fonte (piu' d'una quando lo stesso codice sta piu' volte sotto
	// lo stesso nodo del file). Sono quelle che la conferma dell'albero decide (fase 4.4a.1b).
	Righe []RigaArco `json:"righe,omitempty"`
}

// RigaArco e' una riga di relazione_proposta: il file che la porta, i due nodi del file e la quantita'.
type RigaArco struct {
	Allegato uuid.UUID `json:"allegato"`
	Padre    string    `json:"padre"`
	Figlio   string    `json:"figlio"`
	Qta      int32     `json:"qta"`
}

// RimozioneAlbero e' la rimozione proposta dallo STEP su un legame della working (rimozione_proposta aperta).
type RimozioneAlbero struct {
	Step     uuid.UUID `json:"step"`
	QtaPrima int32     `json:"qta_prima"`
}

// FileSenzaPosto e' uno STEP della RFQ che non sta nell'albero.
type FileSenzaPosto struct {
	Allegato uuid.UUID `json:"allegato"`
	File     string    `json:"file"`
	Radici   []string  `json:"radici,omitempty"`
	Motivo   string    `json:"motivo"`
}

// Nodo e' il nodo con quella chiave.
func (a *AlberoProposto) Nodo(chiave string) (*NodoAlbero, bool) {
	if a.per == nil {
		a.indicizza()
	}
	i, ok := a.per[chiave]
	if !ok {
		return nil, false
	}
	return &a.Nodi[i], true
}

func (a *AlberoProposto) indicizza() {
	a.per = make(map[string]int, len(a.Nodi))
	for i, n := range a.Nodi {
		a.per[n.Chiave] = i
	}
}

// ------------------------------------------------------------------ dal database

// LeggiAlberoProposto legge la RFQ come la legge il flusso di F8 (LeggiStatoFlusso: nessuna scrittura), con le
// rimozioni aperte, e ne fa l'albero proposto. Solo letture: aprire l'albero non cambia una riga.
func LeggiAlberoProposto(ctx context.Context, q *db.Queries, thread uuid.UUID, an coda.Analizzatore) (AlberoProposto, error) {
	s, err := LeggiStatoFlusso(ctx, q, thread, an)
	if err != nil {
		return AlberoProposto{}, err
	}
	rim, err := q.ListRimozioniAperte(ctx, thread)
	if err != nil {
		return AlberoProposto{}, err
	}
	p := ProdottiDellaRfq(&s)
	a := Ancore(p, &s)
	return NuovoAlberoProposto(&s, p, a, IndiceCodici(p, a, &s), rim), nil
}

// ------------------------------------------------------------------ la costruzione (pura)

// costruzione e' l'albero mentre si costruisce: i nodi e gli archi per chiave.
type costruzione struct {
	d      *derivati
	ix     Indice
	nodi   map[string]*NodoAlbero
	archi  map[[2]string]*arcoInCostruzione
	perCan map[string]db.Componente // codice canonico → componente (l'attivo prima dell'archiviato)
	// working: i componenti raggiunti dagli archi della working sotto un prodotto
	working map[uuid.UUID]bool
	// camminati: i pezzi il cui STEP proprio e' gia' stato camminato
	camminati map[string]bool
	rim       map[Arco]db.RimozioneProposta
	grezze    map[string]*strutturaGrezza
}

type arcoInCostruzione struct {
	ArcoAlbero
	working bool
	// perFonte: allegato + nodo padre nel file → la fonte (la quantita' si somma sui figli con lo stesso codice)
	perFonte map[string]*FonteArco
	ordine   []string
}

// strutturaGrezza e' uno STEP con TUTTE le sue righe, anche quelle scartate da una persona (strutturaStep le toglie):
// l'albero le mostra, scartate, e non scende sotto.
type strutturaGrezza struct {
	nodi  map[string]db.ComponenteProposta
	figli map[string][]db.RelazioneProposta
}

// NuovoAlberoProposto costruisce l'albero proposto dallo stato del flusso, dai prodotti confermati, dalle loro ancore
// e dall'indice di F8 (che dice quali STEP dei sottoassiemi si raggiungono), con le rimozioni aperte. Pura.
func NuovoAlberoProposto(s *StatoFlusso, prodotti []Prodotto, ancore map[string]Ancora, ix Indice, rimozioni []db.RimozioneProposta) AlberoProposto {
	d := s.derivati()
	c := &costruzione{d: d, ix: ix, nodi: map[string]*NodoAlbero{}, archi: map[[2]string]*arcoInCostruzione{},
		perCan: map[string]db.Componente{}, working: map[uuid.UUID]bool{}, camminati: map[string]bool{},
		rim: map[Arco]db.RimozioneProposta{}, grezze: map[string]*strutturaGrezza{}}
	comp := append([]db.Componente(nil), s.Componenti...)
	sort.Slice(comp, func(i, j int) bool { return comp[i].Codice < comp[j].Codice })
	for _, x := range comp {
		k := d.canonico(x.Codice)
		if prima, gia := c.perCan[k]; !gia || (prima.ArchiviatoIl != nil && x.ArchiviatoIl == nil) {
			c.perCan[k] = x
		}
	}
	for _, r := range rimozioni {
		c.rim[Arco{Padre: r.PadreID, Figlio: r.FiglioID}] = r
	}
	for _, n := range s.Nodi {
		if d.strutture[n.Sha256] == nil {
			continue // le righe di un file che non e' nel flusso
		}
		g := c.grezze[n.Sha256]
		if g == nil {
			g = &strutturaGrezza{nodi: map[string]db.ComponenteProposta{}, figli: map[string][]db.RelazioneProposta{}}
			c.grezze[n.Sha256] = g
		}
		g.nodi[n.Chiave] = n
	}
	shaDi := map[uuid.UUID]string{}
	for _, n := range s.Nodi {
		shaDi[n.AllegatoID] = n.Sha256
	}
	for _, r := range s.Archi {
		if g := c.grezze[shaDi[r.AllegatoID]]; g != nil {
			g.figli[r.PadreChiave] = append(g.figli[r.PadreChiave], r)
		}
	}
	for _, g := range c.grezze {
		for k := range g.figli {
			sort.Slice(g.figli[k], func(i, j int) bool { return g.figli[k][i].FiglioChiave < g.figli[k][j].FiglioChiave })
		}
	}

	var out AlberoProposto
	out.Algoritmo, out.IndiceFirma = AlgoritmoAlbero, ix.Firma
	ordinati := append([]Prodotto(nil), prodotti...)
	sort.Slice(ordinati, func(i, j int) bool { return ordinati[i].Codice < ordinati[j].Codice })
	for _, p := range ordinati {
		out.Prodotti = append(out.Prodotti, c.prodotto(p, ancore[p.Codice]))
	}
	// gli STEP dei sottoassiemi, fino al punto fisso: un sottoassieme raggiunto dallo STEP di un altro sottoassieme
	// porta anche il suo
	for {
		nuovi := 0
		for _, k := range chiaviOrdinate(c.nodi) {
			n := c.nodi[k]
			if n.Codice == "" || n.Prodotto || c.camminati[n.Codice] || !c.vivo(k) {
				continue
			}
			sa, ok := ix.Sottoassiemi[n.Codice]
			if !ok {
				continue
			}
			if sa.SoloGuida || c.commerciale(n.Codice, uuid.NullUUID{}) {
				// un commerciale e' una foglia (6a): il suo STEP si legge e resta guida
				c.camminati[n.Codice] = true
				var file []string
				for _, x := range sa.Portatori {
					file = append(file, x.Nome)
				}
				n.nota(fmt.Sprintf("è un particolare commerciale: il suo STEP (%s) resta guida (6a)", strings.Join(file, ", ")))
				continue
			}
			c.camminati[n.Codice] = true
			var file []string
			for _, x := range portatoriUsati(sa) {
				if ix.UsatoDa[x.Sha] != n.Codice {
					continue
				}
				file = append(file, x.Nome)
				c.cammina(x.Sha, x.Sorgenti, k, true)
				nuovi++
			}
			if len(file) > 0 {
				n.Step = &StepDelNodo{File: file, Livello: sa.Livello, Discordanze: discordanzeAncora(sa)}
			}
		}
		if nuovi == 0 {
			break
		}
	}
	c.chiudi(&out, s)
	return out
}

// prodotto mette nell'albero la radice di un prodotto e ci cammina sotto: gli archi della working, e lo STEP della
// sua ancora strutturale.
func (c *costruzione) prodotto(p Prodotto, a Ancora) ProdottoAlbero {
	k := ChiaveDelCodice(p.Codice)
	n := c.nodo(k)
	n.Codice, n.Prodotto = p.Codice, true
	if n.FonteCodice == "" {
		n.FonteCodice = FonteCodiceWorking
		if p.Identificativo {
			n.FonteCodice = FonteCodiceRichiesta
		}
	}
	c.camminati[p.Codice] = true
	pa := ProdottoAlbero{Chiave: k, Codice: p.Codice, Ancora: a.Livello, Motivi: a.Motivi, Discordanze: discordanzeAncora(a)}
	if p.ComponenteID.Valid {
		c.working[p.ComponenteID.UUID] = true
		c.dallaWorking(p.ComponenteID.UUID)
	}
	if !a.Ancorato() {
		pa.SoloProdotto = true
		switch a.Livello {
		case LivelloPiatta:
			pa.Perche = "c'è solo il disegno del prodotto: le righe del suo elenco particolari entrano nell'albero con la fase 4.12"
		case LivelloInAttesa:
			pa.Perche = "lo STEP del prodotto è ancora in analisi"
		default:
			pa.Perche = "nessuno STEP del prodotto"
		}
		for _, x := range a.Portatori {
			pa.File = append(pa.File, x.Nome)
		}
		return pa
	}
	for _, x := range portatoriUsati(a) {
		if c.ix.UsatoDa[x.Sha] != p.Codice {
			continue
		}
		pa.File = append(pa.File, x.Nome)
		c.cammina(x.Sha, x.Sorgenti, k, false)
	}
	return pa
}

// discordanzeAncora sono le discordanze dell'ancora e quelle dei file che usa (la radice diversa, la lettera finale):
// cio' che il passo 1 chiedera' a una persona.
func discordanzeAncora(a Ancora) []string {
	out := append([]string(nil), a.Discordanze...)
	for _, x := range portatoriUsati(a) {
		for _, d := range x.Discordanze {
			if !contiene(out, d) {
				out = append(out, d)
			}
		}
	}
	sort.Strings(out)
	return out
}

// dallaWorking porta nell'albero gli archi attivi della working che si raggiungono dal componente radice.
func (c *costruzione) dallaWorking(radice uuid.UUID) {
	figli := map[uuid.UUID][]db.ComponenteRelazione{}
	for _, r := range c.d.s.Relazioni {
		figli[r.PadreID] = append(figli[r.PadreID], r)
	}
	visti := map[uuid.UUID]bool{radice: true}
	coda := []uuid.UUID{radice}
	for len(coda) > 0 {
		p := coda[0]
		coda = coda[1:]
		pc, ok := c.d.compPerID[p]
		if !ok {
			continue
		}
		for _, r := range figli[p] {
			f, ok := c.d.compPerID[r.FiglioID]
			if !ok || f.ArchiviatoIl != nil || pc.ArchiviatoIl != nil {
				continue
			}
			pk, fk := ChiaveDelCodice(c.d.canonico(pc.Codice)), ChiaveDelCodice(c.d.canonico(f.Codice))
			if pk == fk {
				continue
			}
			n := c.nodo(fk)
			n.Codice = c.d.canonico(f.Codice)
			if n.FonteCodice == "" {
				n.FonteCodice = FonteCodiceWorking
			}
			c.working[f.ComponenteID] = true
			a := c.arco(pk, fk)
			a.working, a.QtaWorking = true, r.Qta
			if rp, ok := c.rim[Arco{Padre: r.PadreID, Figlio: r.FiglioID}]; ok {
				a.Rimozione = &RimozioneAlbero{Step: rp.StepDocumentoID, QtaPrima: rp.QtaWorking}
			}
			if !visti[f.ComponenteID] {
				visti[f.ComponenteID] = true
				coda = append(coda, f.ComponenteID)
			}
		}
	}
}

// cammina porta nell'albero lo STEP sha dai nodi sorgente (il pezzo stesso nel file) in giu', a tutti i livelli, sotto
// il nodo radice. conSorgenti: le righe delle sorgenti sono righe del nodo radice (lo STEP proprio di un sottoassieme,
// la cui radice dice il suo codice); per un prodotto no (la sorgente e' il prodotto, e la dice la sua ancora). Un
// commerciale che c'e' e' una foglia (6a): il suo STEP e i suoi discendenti restano guida. Sotto una riga scartata da
// una persona non si scende.
func (c *costruzione) cammina(sha string, sorgenti []string, radice string, conSorgenti bool) {
	st, g := c.d.strutture[sha], c.grezze[sha]
	if st == nil || g == nil {
		return
	}
	f, _ := c.d.primoFile(sha)
	type passo struct{ chiave, padre string }
	visti := map[string]bool{}
	var coda []passo
	for _, s := range sorgenti {
		visti[s] = true
		coda = append(coda, passo{s, radice})
		if n, ok := st.Nodi[s]; ok && conSorgenti {
			c.riga(c.nodo(radice), n, f)
		}
	}
	for len(coda) > 0 {
		x := coda[0]
		coda = coda[1:]
		// le righe scartate da una persona sotto questo nodo: si vedono, e non si scende
		for _, r := range g.figli[x.chiave] {
			n, ok := g.nodi[r.FiglioChiave]
			if !ok || (!scartatoDaUnaPersona(n.Stato, n.DecisoDa) && !scartatoDaUnaPersona(r.Stato, r.DecisoDa)) {
				continue
			}
			k := c.chiaveRiga(n)
			if k == x.padre {
				continue
			}
			c.riga(c.nodo(k), n, f)
			a := c.arco(x.padre, k)
			a.fonte(f, x.chiave, RigaArco{Allegato: r.AllegatoID, Padre: r.PadreChiave, Figlio: r.FiglioChiave, Qta: r.Qta}, true)
		}
		for _, a := range st.Figli[x.chiave] {
			n, ok := st.Nodi[a.Figlio]
			if !ok {
				continue
			}
			k := c.chiaveRiga(n)
			if k == x.padre {
				// due PRODUCT con lo stesso codice uno dentro l'altro (STEP veri, A1.1): sono lo stesso pezzo, e i figli
				// del secondo sono figli del primo
				if !visti[a.Figlio] {
					visti[a.Figlio] = true
					coda = append(coda, passo{a.Figlio, x.padre})
				}
				continue
			}
			nodo := c.nodo(k)
			c.riga(nodo, n, f)
			c.arco(x.padre, k).fonte(f, x.chiave, RigaArco{Allegato: st.Portatore, Padre: x.chiave, Figlio: a.Figlio, Qta: int32(a.Qta)}, false)
			if c.commerciale(nodo.Codice, nullo(DecisoDaUnaPersona(n))) {
				if len(st.Figli[a.Figlio]) > 0 {
					nodo.nota(fmt.Sprintf("è un particolare commerciale: i pezzi che %s gli mette sotto restano guida (6a)", f.Nome))
				}
				continue
			}
			if !visti[a.Figlio] {
				visti[a.Figlio] = true
				coda = append(coda, passo{a.Figlio, k})
			}
		}
	}
}

// commerciale: il pezzo e' un particolare commerciale che c'e' (il componente che una persona ha dato alla riga, o
// quello con il suo codice canonico, che ritrova anche il pezzo nato con il suffisso del cliente). Anche archiviato
// (fase 4.4a.1b, dalla verifica della 4.4a.1a): il nodo lo ritrova, la conferma lo ripristina con il suo tipo, e un
// commerciale e' una foglia (6a) come quando e' attivo.
func (c *costruzione) commerciale(codice string, acc uuid.NullUUID) bool {
	if acc.Valid {
		if x, ok := c.d.compPerID[acc.UUID]; ok {
			return x.Tipo == db.TipoComponenteCommerciale
		}
	}
	x, ok := c.perCan[codice]
	return ok && codice != "" && x.Tipo == db.TipoComponenteCommerciale
}

// chiaveRiga e' la chiave del nodo di una riga: il codice canonico, o la riga stessa se non ha codice.
func (c *costruzione) chiaveRiga(n db.ComponenteProposta) string {
	if k := c.d.codiceNodo(n); k != "" {
		return ChiaveDelCodice(k)
	}
	return chiaveSenzaCodice(n.PropostaID)
}

func (c *costruzione) nodo(k string) *NodoAlbero {
	n, ok := c.nodi[k]
	if !ok {
		n = &NodoAlbero{Chiave: k}
		if codice, ok := strings.CutPrefix(k, "cod:"); ok {
			n.Codice = codice
		}
		c.nodi[k] = n
	}
	return n
}

// riga aggiunge al nodo una riga di uno STEP, con quello che dice del pezzo (la prima riga viva da' nome, revisione e
// descrizione).
func (c *costruzione) riga(n *NodoAlbero, p db.ComponenteProposta, f FileFlusso) {
	for _, r := range n.Righe {
		if r.Proposta == p.PropostaID {
			return
		}
	}
	n.Righe = append(n.Righe, RigaAlbero{Proposta: p.PropostaID, Allegato: f.AllegatoID, File: f.Nome, Chiave: p.Chiave, Stato: p.Stato,
		Rev: strings.ToUpper(strings.TrimSpace(p.Rev.String)), Agganciata: AgganciatoPerCodice(p), decisa: p.DecisoDa.Valid})
	if scartatoDaUnaPersona(p.Stato, p.DecisoDa) {
		return
	}
	if n.Nome == "" {
		n.Nome = strings.TrimSpace(p.NomeGrezzo)
	}
	if n.Rev == "" && p.Rev.Valid {
		n.Rev = strings.TrimSpace(p.Rev.String)
	}
	if n.Descrizione == "" && p.Descrizione.Valid && !nonSpecificato(p.Descrizione.String) {
		n.Descrizione = strings.TrimSpace(p.Descrizione.String)
	}
	if n.FonteCodice == "" {
		if p.Codice.Valid && strings.TrimSpace(p.Codice.String) != "" {
			n.FonteCodice = FonteCodiceStep
			if p.OrigineCodice.Valid && p.OrigineCodice.OrigineCodice == db.OrigineCodiceOperatore {
				n.FonteCodice = FonteCodiceOperatore
			}
			if p.OrigineCodice.Valid {
				n.OrigineCodice = string(p.OrigineCodice.OrigineCodice)
			}
			if grezzo := strings.ToUpper(strings.TrimSpace(p.Codice.String)); grezzo != n.Codice {
				n.CodiceNelFile = strings.TrimSpace(p.Codice.String)
			}
		}
	}
}

// nonSpecificato: la descrizione vuota dei CAD («NOT SPECIFIED») vale vuoto.
func nonSpecificato(s string) bool {
	s = strings.ToUpper(strings.TrimSpace(s))
	return s == "" || s == "NOT SPECIFIED"
}

func (n *NodoAlbero) nota(s string) {
	if !contiene(n.Note, s) {
		n.Note = append(n.Note, s)
	}
}

func (c *costruzione) arco(padre, figlio string) *arcoInCostruzione {
	k := [2]string{padre, figlio}
	a, ok := c.archi[k]
	if !ok {
		a = &arcoInCostruzione{ArcoAlbero: ArcoAlbero{Padre: padre, Figlio: figlio}, perFonte: map[string]*FonteArco{}}
		c.archi[k] = a
	}
	return a
}

// fonte aggiunge al legame il file che lo dice, sotto il nodo padre del file: i figli con lo stesso codice sotto lo
// stesso nodo sono pezzi in piu', e le quantita' si sommano. La riga di relazione_proposta resta con la fonte: e'
// quella che la conferma decide.
func (a *arcoInCostruzione) fonte(f FileFlusso, padreNelFile string, riga RigaArco, scartata bool) {
	k := f.AllegatoID.String() + "|" + padreNelFile
	if scartata {
		k += "|scartata"
	}
	x, ok := a.perFonte[k]
	if !ok {
		x = &FonteArco{Allegato: f.AllegatoID, File: f.Nome, Scartata: scartata}
		a.perFonte[k] = x
		a.ordine = append(a.ordine, k)
	}
	x.Qta += riga.Qta
	x.Righe = append(x.Righe, riga)
}

// chiudi calcola quello che dipende dall'albero intero: lo stato e il tipo di ogni nodo, i padri e i figli, i
// prodotti, i ritrovati, i vicini, le proposte commerciali, le quantita' e lo stato degli archi; poi i file senza posto
// e la firma.
func (c *costruzione) chiudi(out *AlberoProposto, s *StatoFlusso) {
	d := c.d
	// gli archi
	vivi := map[string][]string{} // padre → figli (archi non scartati)
	padri := map[string][]string{}
	for _, k := range c.chiaviArchi() {
		a := c.archi[k]
		var qta []int32
		for _, fk := range a.ordine {
			x := a.perFonte[fk]
			a.Fonti = append(a.Fonti, *x)
			if !x.Scartata {
				qta = append(qta, x.Qta)
			}
		}
		sort.SliceStable(a.Fonti, func(i, j int) bool {
			if a.Fonti[i].File != a.Fonti[j].File {
				return a.Fonti[i].File < a.Fonti[j].File
			}
			return a.Fonti[i].Allegato.String() < a.Fonti[j].Allegato.String()
		})
		switch {
		case a.working && a.Rimozione != nil:
			a.Stato = StatoAlberoTolto
		case a.working:
			a.Stato = StatoAlberoNellaDistinta
		case len(qta) > 0:
			a.Stato = StatoAlberoProposto
		default:
			a.Stato = StatoAlberoScartato
		}
		switch {
		case a.working:
			a.Qta = a.QtaWorking
		case len(qta) > 0:
			a.Qta = qta[0]
			for _, q := range qta[1:] {
				if q != qta[0] {
					a.Qta, a.QtaDiscordi = 0, true
				}
			}
		}
		if a.Stato != StatoAlberoScartato {
			vivi[a.Padre] = append(vivi[a.Padre], a.Figlio)
			padri[a.Figlio] = append(padri[a.Figlio], a.Padre)
		}
		out.Archi = append(out.Archi, a.ArcoAlbero)
	}
	// i prodotti sotto cui sta ogni nodo
	sotto := map[string][]string{}
	for _, p := range out.Prodotti {
		visti := map[string]bool{p.Chiave: true}
		coda := []string{p.Chiave}
		for len(coda) > 0 {
			x := coda[0]
			coda = coda[1:]
			sotto[x] = append(sotto[x], p.Codice)
			for _, f := range vivi[x] {
				if !visti[f] {
					visti[f] = true
					coda = append(coda, f)
				}
			}
		}
	}
	// i codici per i vicini: i componenti (anche archiviati), la richiesta, i nodi dell'albero
	var codici []string
	for _, x := range s.Componenti {
		codici = append(codici, x.Codice)
	}
	for _, i := range s.Identificativi {
		codici = append(codici, i.Codice)
	}
	for _, k := range chiaviOrdinate(c.nodi) {
		if n := c.nodi[k]; n.Codice != "" {
			codici = append(codici, n.Codice)
		}
	}
	sort.Strings(codici)
	righe := make(map[uuid.UUID]db.ComponenteProposta, len(s.Nodi))
	for _, p := range s.Nodi {
		righe[p.PropostaID] = p
	}

	for _, k := range chiaviOrdinate(c.nodi) {
		n := c.nodi[k]
		n.Padri = unici(padri[k])
		n.Figli = len(unici(vivi[k]))
		n.Prodotti = unici(sotto[k])
		comp, esiste := db.Componente{}, false
		if n.Codice != "" {
			comp, esiste = c.perCan[n.Codice]
		}
		attivo := esiste && comp.ArchiviatoIl == nil
		if attivo {
			n.Componente = uuid.NullUUID{UUID: comp.ComponenteID, Valid: true}
			n.TipoAttuale = comp.Tipo
			if !n.Prodotto {
				n.FonteCodice = FonteCodiceWorking // il codice c'e' gia': non e' una proposta
			}
			if n.Descrizione == "" && comp.Descrizione.Valid {
				n.Descrizione = comp.Descrizione.String
			}
			if n.Rev == "" && comp.Rev.Valid {
				n.Rev = comp.Rev.String
			}
			if n.Nome == "" && comp.Descrizione.Valid {
				n.Nome = comp.Descrizione.String
			}
		}
		// aperte: le righe ancora da decidere, con gli agganci per codice di prima dello Smistamento (non sono decisioni:
		// fase 4.4a.1b, RigaAlbero.Agganciata)
		aperte, decise, vive, agganciate := 0, 0, 0, 0
		for _, r := range n.Righe {
			switch {
			case r.daDecidere():
				aperte++
				vive++
				if r.Agganciata {
					agganciate++
				}
			case r.Stato == db.StatoPropostaScartata:
			default:
				decise++
				vive++
			}
		}
		// lo stato: nella distinta se il pezzo c'e' nella working (raggiunto dagli archi della working, o deciso da una
		// persona); tolto se tutti i suoi legami della working nell'albero sono proposti da togliere; scartato se una
		// persona ha scartato le sue righe, o il solo legame che lo portava sotto un prodotto
		switch {
		case attivo && (c.working[comp.ComponenteID] || decise > 0 || n.Prodotto):
			n.Stato = StatoAlberoNellaDistinta
			if !n.Prodotto && c.tuttiTolti(k) {
				n.Stato = StatoAlberoTolto
			}
		case n.Prodotto:
			n.Stato = StatoAlberoProposto
		case vive > 0 && len(n.Prodotti) > 0:
			n.Stato = StatoAlberoProposto
		default:
			n.Stato = StatoAlberoScartato
		}
		// il ritrovato per codice (P13): righe ancora da decidere e un componente con lo stesso codice. Un componente
		// archiviato con lo stesso codice e' ritrovato anche con le righe gia' decise (fase 4.4a.1b): la conferma lo
		// ripristina, e il riepilogo lo deve dire. Un aggancio per codice di prima e' ritrovato come una riga aperta: la
		// conferma lo decide, quindi il riepilogo lo elenca (P13, U5)
		if esiste && !n.Prodotto && (aperte > 0 || (!attivo && n.Stato == StatoAlberoProposto)) {
			n.Ritrovato = &RitrovatoAlbero{Componente: comp.ComponenteID, Codice: comp.Codice, Tipo: comp.Tipo, Archiviato: comp.ArchiviatoIl != nil,
				Agganciato: agganciate > 0}
		}
		// il tipo proposto. Un componente archiviato che il nodo ritrova segue 6a e 6b come un attivo (fase 4.4a.1b,
		// dalla verifica della 4.4a.1a): la conferma lo ripristina con il tipo che il riepilogo dice
		switch {
		case n.Prodotto:
			n.Tipo, n.TipoMotivo = db.TipoComponenteFinito, "il prodotto della richiesta"
		case esiste && n.Figli > 0 && comp.Tipo == db.TipoComponenteSciolto:
			n.Tipo = db.TipoComponenteSottoassieme
			n.TipoMotivo = fmt.Sprintf("è un particolare, ma nell'albero ha %d figli: un particolare non ha figli, è proposto come assieme (6b)", n.Figli)
			if !attivo {
				n.TipoMotivo = fmt.Sprintf("era un particolare (archiviato), ma nell'albero ha %d figli: un particolare non ha figli, è proposto come assieme (6b)", n.Figli)
			}
		case attivo:
			n.Tipo, n.TipoMotivo = comp.Tipo, "il tipo del componente"
		case esiste:
			n.Tipo, n.TipoMotivo = comp.Tipo, "il tipo del componente archiviato con lo stesso codice"
		case n.Figli > 0:
			n.Tipo, n.TipoMotivo = db.TipoComponenteSottoassieme, fmt.Sprintf("ha %d figli nell'albero: un assieme", n.Figli)
		default:
			n.Tipo, n.TipoMotivo = db.TipoComponenteSciolto, "una foglia: un particolare"
		}
		if n.Codice == "" && n.Stato != StatoAlberoScartato {
			n.nota("senza codice: lo si scrive prima della conferma")
		}
		if n.Prodotto && len(n.Padri) > 0 {
			n.nota("è un prodotto della RFQ, e uno STEP lo mette sotto " + strings.Join(senzaPrefisso(n.Padri), ", "))
		}
		if v := c.ix.Voci[n.Codice]; v != nil && n.Codice != "" {
			n.Discordanze = append([]string(nil), v.Discordanze...)
		}
		nasce := !esiste && !n.Prodotto && n.Stato == StatoAlberoProposto && n.Codice != ""
		if nasce {
			var altri []string
			for _, x := range codici {
				if x != n.Codice {
					altri = append(altri, x)
				}
			}
			n.Vicini = Vicini(n.Codice, altri, d.m)
		}
		// la proposta commerciale: solo una foglia che nascerebbe, dal nome del pezzo (mai dal testo del disegno). Anche
		// una foglia senza codice (fase 4.4a.1b, dalla verifica della 4.4a.1a): nasce quando nella bozza prende un
		// codice, e allora la domanda vale (il riepilogo la conta solo da li')
		senzaCodice := n.Codice == "" && !n.Prodotto && n.Stato == StatoAlberoProposto
		if (nasce || senzaCodice) && n.Figli == 0 {
			nome := nomeDelPezzo(n, righe)
			switch e := classificazione.Minuteria(nome); {
			case e.Proposta():
				n.Commerciale = &DomandaCommerciale{Esito: e.Esito, Motivo: e.Frase(), Nome: nome}
			case e.Esito == classificazione.MinuteriaDaVedere:
				n.nota("forse minuteria, " + e.Frase() + ": nessuna proposta, decide una persona")
			}
		}
		sort.Slice(n.Righe, func(i, j int) bool {
			if n.Righe[i].File != n.Righe[j].File {
				return n.Righe[i].File < n.Righe[j].File
			}
			return n.Righe[i].Chiave < n.Righe[j].Chiave
		})
		out.Nodi = append(out.Nodi, *n)
	}
	out.indicizza()
	// gli STEP senza posto: quelli che non si raggiungono, e le alternative
	for _, sha := range d.shaStep {
		f, _ := d.primoFile(sha)
		switch {
		case c.ix.Scollegati[sha]:
			out.SenzaPosto = append(out.SenzaPosto, FileSenzaPosto{Allegato: f.AllegatoID, File: f.Nome, Radici: d.codiciRadici(d.strutture[sha]),
				Motivo: "la sua radice non si raggiunge da nessun prodotto: di quale pezzo è?"})
		case c.ix.Alternative[sha] != "":
			out.SenzaPosto = append(out.SenzaPosto, FileSenzaPosto{Allegato: f.AllegatoID, File: f.Nome, Radici: d.codiciRadici(d.strutture[sha]),
				Motivo: fmt.Sprintf("un'alternativa al file che %s usa: si vede, non propone", c.ix.Alternative[sha])})
		}
	}
	out.Firma = firmaAlbero(*out, s)
}

// vivo: il nodo sta nell'albero per un legame che nessuno ha scartato (della working, o di uno STEP). Sotto un nodo
// che c'e' solo per righe scartate non si cammina il suo STEP.
func (c *costruzione) vivo(k string) bool {
	for _, a := range c.archi {
		if a.Figlio != k {
			continue
		}
		if a.working {
			return true
		}
		for _, f := range a.perFonte {
			if !f.Scartata {
				return true
			}
		}
	}
	return false
}

// tuttiTolti: ogni legame della working che porta al nodo nell'albero e' proposto da togliere dallo STEP.
func (c *costruzione) tuttiTolti(k string) bool {
	n := 0
	for _, a := range c.archi {
		if a.Figlio != k || !a.working {
			continue
		}
		if a.Rimozione == nil {
			return false
		}
		n++
	}
	return n > 0
}

func (c *costruzione) chiaviArchi() [][2]string {
	out := make([][2]string, 0, len(c.archi))
	for k := range c.archi {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}

// nomeDelPezzo e' il nome su cui si guarda la minuteria: i nomi e le descrizioni delle righe vive dello STEP (il
// PRODUCT), diversi fra loro, uniti come un nome bilingue. Mai il testo del disegno.
func nomeDelPezzo(n *NodoAlbero, righe map[uuid.UUID]db.ComponenteProposta) string {
	var nomi []string
	for _, r := range n.Righe {
		p, ok := righe[r.Proposta]
		if !ok || r.Stato == db.StatoPropostaScartata {
			continue
		}
		for _, x := range []string{p.NomeGrezzo, p.Descrizione.String} {
			if x = strings.TrimSpace(x); x != "" && !nonSpecificato(x) && !contiene(nomi, x) {
				nomi = append(nomi, x)
			}
		}
	}
	return strings.Join(nomi, " / ")
}

func unici(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	out := append([]string(nil), s...)
	sort.Strings(out)
	j := 0
	for i, x := range out {
		if i == 0 || x != out[j-1] {
			out[j] = x
			j++
		}
	}
	return out[:j]
}

// senzaPrefisso sono i codici delle chiavi (o la chiave, per un nodo senza codice).
func senzaPrefisso(chiavi []string) []string {
	out := make([]string, len(chiavi))
	for i, k := range chiavi {
		out[i] = strings.TrimPrefix(k, "cod:")
	}
	return out
}

// firmaAlbero e' lo sha256 del JSON canonico dello stato dell'albero e della working: i nodi e gli archi con i loro
// stati, i tipi, i componenti, le domande aperte e le righe (dalla fase 4.4a.1b con la revisione di ogni riga, se e'
// un aggancio per codice di prima, e le righe di ogni fonte: sono quello che la conferma decide); tutti i componenti e gli archi attivi della working; la
// firma dell'indice di F8 (ancore, regole del cliente, algoritmo). Le parole dei motivi e delle note non ci entrano:
// cambiano con le frasi, non con lo stato. La conferma (ConfermaAlbero) la ricalcola sotto il lucchetto della RFQ.
func firmaAlbero(a AlberoProposto, s *StatoFlusso) string {
	type nodoFirma struct {
		Chiave, Codice, Rev, Stato string
		Tipo, TipoAttuale          db.TipoComponente
		Componente                 uuid.NullUUID
		Ritrovato                  string
		Vicini                     []string
		Commerciale                string
		Padri                      []string
		Righe                      []string
	}
	type arcoFirma struct {
		Padre, Figlio, Stato string
		Qta, QtaWorking      int32
		QtaDiscordi          bool
		Fonti                []string
		Rimozione            string
	}
	type compFirma struct {
		ID                uuid.UUID
		Codice, Rev, Tipo string
		Archiviato        bool
	}
	type prodFirma struct {
		Chiave, Ancora string
		File           []string
		Solo           bool
	}
	var f struct {
		Algoritmo, Indice string
		Prodotti          []prodFirma
		Nodi              []nodoFirma
		Archi             []arcoFirma
		Componenti        []compFirma
		Relazioni         []string
	}
	f.Algoritmo, f.Indice = a.Algoritmo, a.IndiceFirma
	for _, p := range a.Prodotti {
		f.Prodotti = append(f.Prodotti, prodFirma{Chiave: p.Chiave, Ancora: p.Ancora, File: p.File, Solo: p.SoloProdotto})
	}
	for _, n := range a.Nodi {
		x := nodoFirma{Chiave: n.Chiave, Codice: n.Codice, Rev: n.Rev, Stato: n.Stato, Tipo: n.Tipo, TipoAttuale: n.TipoAttuale,
			Componente: n.Componente, Padri: n.Padri}
		if n.Ritrovato != nil {
			x.Ritrovato = n.Ritrovato.Componente.String()
		}
		for _, v := range n.Vicini {
			x.Vicini = append(x.Vicini, v.Codice)
		}
		if n.Commerciale != nil {
			x.Commerciale = n.Commerciale.Esito + ":" + n.Commerciale.Nome
		}
		for _, r := range n.Righe {
			// l'aggancio per codice di prima: confermato da una persona (ConfermaNodoAgganciato) resta duplicato, e cambia
			// solo chi l'ha deciso
			x.Righe = append(x.Righe, fmt.Sprintf("%s:%s:%s:%v", r.Proposta, r.Stato, r.Rev, r.Agganciata))
		}
		f.Nodi = append(f.Nodi, x)
	}
	for _, r := range a.Archi {
		x := arcoFirma{Padre: r.Padre, Figlio: r.Figlio, Stato: r.Stato, Qta: r.Qta, QtaWorking: r.QtaWorking, QtaDiscordi: r.QtaDiscordi}
		for _, fo := range r.Fonti {
			fonte := fmt.Sprintf("%s:%d:%v", fo.Allegato, fo.Qta, fo.Scartata)
			for _, rr := range fo.Righe {
				fonte += fmt.Sprintf(":%s>%s*%d@%s", rr.Padre, rr.Figlio, rr.Qta, rr.Allegato)
			}
			x.Fonti = append(x.Fonti, fonte)
		}
		if r.Rimozione != nil {
			x.Rimozione = r.Rimozione.Step.String()
		}
		f.Archi = append(f.Archi, x)
	}
	for _, x := range s.Componenti {
		f.Componenti = append(f.Componenti, compFirma{ID: x.ComponenteID, Codice: x.Codice, Rev: x.Rev.String, Tipo: string(x.Tipo),
			Archiviato: x.ArchiviatoIl != nil})
	}
	sort.Slice(f.Componenti, func(i, j int) bool { return f.Componenti[i].ID.String() < f.Componenti[j].ID.String() })
	for _, r := range s.Relazioni {
		f.Relazioni = append(f.Relazioni, fmt.Sprintf("%s>%s*%d", r.PadreID, r.FiglioID, r.Qta))
	}
	sort.Strings(f.Relazioni)
	raw, _ := json.Marshal(f)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
