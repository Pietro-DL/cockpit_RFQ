package motore_a

// indice.go: l'indice delle ricerche della Distinta nuova (giro 5, A1d-P2; direttiva A1d §3.4; contratto A1d §2.3; v1.1
// §4.3; X-02…X-06, X-09; T-A1d-09, T-A1d-10, T-A1d-19…T-A1d-27). Derivato dalla PR #18 di Stefano Valo (c543e89:
// motore_a_indice.go), spostato nel sottopackage con i nomi esportati e con le correzioni della revisione:
//   - le autorizzazioni dello STEP: oltre ai parametri del gesto, la marcatura e la riga di v_step_prodotto così come
//     sono, e la validità delle dichiarazioni come la dice il Fascicolo (fascicolo.LeggiDichiarazioni, nello stesso
//     snapshot della fotografia), senza ricostruirne le regole (F-16, rilievo R4 della revisione dell'indice);
//   - la regola del componente del nodo (T-B4-38) sta in un punto solo del web, ComponenteDelNodo (F-15, R3): è una
//     copia di componenteDelNodo di core/valutazione (nodi.go), confrontata con l'esito vero da una L4 del QA (S-04,
//     IX-03 della revisione di A1d-P2);
//   - gli archi del file e le relazioni confermate restano distinti: Figlio.Origine dice da dove viene un figlio (R9),
//     e LegameConfermato.NelFile confronta i componenti, non il primo Rif (R8);
//   - CandidatiFuori esclude anche lo STEP della BOM di lavoro (R7);
//   - AutorizzazioneDiStruttura non spezza più il Rif della radice: senza la riga di componente_proposta risponde nil
//     (R10, T-A1d-FE-01).
//
// Il template di P4 non scorrerà gli elenchi dell'esito per trovare un dato: chiamerà questi metodi, che fanno solo
// ricerche sulla fotografia, sull'esito e sulle dichiarazioni dello stesso calcolo. Leggono campi e confrontano valori:
// nessuna regola di classificazione nuova, nessuno stato calcolato, nessuna query. Le eccezioni dichiarate sono due
// copie di regole di valutazione che l'esito non espone: il componente di un nodo (componenteDelNodo, T-B4-38) e il
// formato di un allegato dall'estensione (estensioneDi). L'indice non va nel JSON di /dati (Q-A1d-11 = A,
// raccomandazione della bozza).
//
// Il tipo di un nodo dipende dalla casella (valutazione-3, D-CHIUS-4: «finito» proposto vale solo nella BOM di lavoro
// del suo target): lo stesso Rif può avere Tipo e OrigineTipo diversi nella BOM di lavoro, in una candidata, in un
// altro prodotto. Anche la catena dei codici dipende dalla struttura: il codice confermato del prodotto sta solo sulla
// radice della BOM di lavoro (T-B4-38, R59 A). Per il tipo, i disegni, la classificazione e la catena dei codici si
// legge sempre NodoIn(prodotto, gruppo, rif), mai Nodo (IX-05 della revisione di A1d-P2).

import (
	"path"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/db"
)

// Indice: le ricerche della pagina (contratto A1d §2.3; v1.1 §4.3) sulla stessa fotografia e sullo stesso esito
// del calcolo: niente SQL in più, nessuna regola, nessuna corrispondenza inventata, nessun Rif spezzato. Nil-sicuro: un
// indice nil risponde con il valore zero. Le mappe servono solo a cercare, mai a scorrere. Vale per una richiesta:
// gruppi e nodi si riempiono alla prima ricerca, senza lucchetti (non si condivide fra goroutine).
type Indice struct {
	t          *fotorfq.Thread
	e          *valutazione.EsitoThread
	c          *confronto.Esito
	d          *fascicolo.Dichiarazioni
	allegati   map[uuid.UUID]*fotorfq.Allegato
	file       map[uuid.UUID]*valutazione.FileInterpretato
	assoc      map[uuid.UUID]*valutazione.AssociazioneFile
	badge      map[uuid.UUID]*confronto.EsitoFile
	prodotti   map[string]*valutazione.ProdottoValutato
	componenti map[uuid.UUID]*fotorfq.Componente
	anteprime  map[uuid.UUID]*valutazione.RiferimentoAnteprima // il primo 2D dell'esito per allegato (Anteprima, X-09)
	gruppi     map[string][]Gruppo                             // per prodotto, alla prima richiesta
	nodi       map[string][]*valutazione.NodoBOM               // per prodotto+"\x00"+gruppo, alla prima richiesta
}

// NuovoIndice: nil se l'esito è nil; con t, c o d nil le ricerche che li usano rispondono vuoto. Le mappe tengono il
// primo elemento per chiave, come una ricerca lineare dall'inizio.
func NuovoIndice(t *fotorfq.Thread, e *valutazione.EsitoThread, c *confronto.Esito, d *fascicolo.Dichiarazioni) *Indice {
	if e == nil {
		return nil
	}
	x := &Indice{t: t, e: e, c: c, d: d,
		allegati:   map[uuid.UUID]*fotorfq.Allegato{},
		file:       map[uuid.UUID]*valutazione.FileInterpretato{},
		assoc:      map[uuid.UUID]*valutazione.AssociazioneFile{},
		badge:      map[uuid.UUID]*confronto.EsitoFile{},
		prodotti:   map[string]*valutazione.ProdottoValutato{},
		componenti: map[uuid.UUID]*fotorfq.Componente{},
		anteprime:  map[uuid.UUID]*valutazione.RiferimentoAnteprima{},
		gruppi:     map[string][]Gruppo{},
		nodi:       map[string][]*valutazione.NodoBOM{},
	}
	if t != nil {
		for i := range t.Allegati {
			if _, ok := x.allegati[t.Allegati[i].ID]; !ok {
				x.allegati[t.Allegati[i].ID] = &t.Allegati[i]
			}
		}
		for i := range t.Componenti {
			if _, ok := x.componenti[t.Componenti[i].ID]; !ok {
				x.componenti[t.Componenti[i].ID] = &t.Componenti[i]
			}
		}
	}
	for i := range e.File {
		if _, ok := x.file[e.File[i].AllegatoID]; !ok {
			x.file[e.File[i].AllegatoID] = &e.File[i]
		}
	}
	for i := range e.Associazioni {
		if _, ok := x.assoc[e.Associazioni[i].AllegatoID]; !ok {
			x.assoc[e.Associazioni[i].AllegatoID] = &e.Associazioni[i]
		}
	}
	if c != nil {
		for i := range c.File {
			if _, ok := x.badge[c.File[i].File.AllegatoID]; !ok {
				x.badge[c.File[i].File.AllegatoID] = &c.File[i]
			}
		}
	}
	for i := range e.ProdottiValutati {
		if _, ok := x.prodotti[e.ProdottiValutati[i].Rif]; !ok {
			x.prodotti[e.ProdottiValutati[i].Rif] = &e.ProdottiValutati[i]
		}
	}
	// Il primo 2D dell'esito per allegato (X-09; T-A1d-19), in quest'ordine: per ogni prodotto le voci dei documenti, i
	// previsti, i nodi della BOM di lavoro, i nodi delle candidate, i confermati senza STEP.
	for i := range e.ProdottiValutati {
		p := &e.ProdottiValutati[i]
		for j := range p.Documenti.Voci {
			x.anteprimeDelGruppo(p.Documenti.Voci[j].Disegni)
		}
		for j := range p.Documenti.Previsti {
			x.anteprimeDelGruppo(p.Documenti.Previsti[j].Disegni)
		}
		for j := range p.Nodi {
			x.anteprimeDelGruppo(&p.Nodi[j].Disegni)
		}
		for k := range p.StruttureCandidate {
			for j := range p.StruttureCandidate[k].Nodi {
				x.anteprimeDelGruppo(&p.StruttureCandidate[k].Nodi[j].Disegni)
			}
		}
		for j := range p.ConfermatiSenzaSTEP {
			x.anteprimeDelGruppo(&p.ConfermatiSenzaSTEP[j].Disegni)
		}
	}
	return x
}

// anteprimeDelGruppo: il primario se c'è, poi gli alternativi; un gruppo nil (un puntatore dell'esito) si salta.
func (x *Indice) anteprimeDelGruppo(g *valutazione.GruppoDisegni2D) {
	if g == nil {
		return
	}
	if g.Primario != nil {
		x.anteprimaDelDisegno(g.Primario)
	}
	for i := range g.Alternativi {
		x.anteprimaDelDisegno(&g.Alternativi[i])
	}
}

// anteprimaDelDisegno: la copia dell'Anteprima del 2D, per il suo allegato (Disegno2D.AllegatoID è un puntatore), se
// l'allegato non ne ha già una.
func (x *Indice) anteprimaDelDisegno(d *valutazione.Disegno2D) {
	if d.AllegatoID == nil {
		return
	}
	if _, ok := x.anteprime[*d.AllegatoID]; ok {
		return
	}
	r := copiaAnteprima(d.Anteprima)
	x.anteprime[*d.AllegatoID] = &r
}

// vuoto: l'indice non risponde (nil, o senza esito).
func (x *Indice) vuoto() bool { return x == nil || x.e == nil }

// ---- le forme di ritorno (v1.1 §4.3; piano del frontend §4.4) ----

// Autorizzazione: i campi dell'anteprima dello STEP strutturale (contratto §3.4 A; T-A1d-23).
type Autorizzazione struct {
	Documento *uuid.UUID // documento.id dello STEP: il campo «documento» del gesto
	Nodo      string     // «<allegato_id>:<chiave>»: il campo «nodo» (la delega)
	Sorgente  string     // la chiave della radice: il campo «sorgente»
}

// RelazioneAperta: la riga aperta di relazione_proposta di un arco (contratto §3.4 B).
type RelazioneAperta struct {
	Allegato uuid.UUID
	Padre    string // relazione_proposta.padre_chiave
	Figlio   string // relazione_proposta.figlio_chiave
}

// RimozioneAperta: la rimozione_proposta aperta di un conflitto (contratto §3.4 B).
type RimozioneAperta struct{ Step, Padre, Figlio uuid.UUID }

// Gruppo: una scheda dell'albero (X-02, X-03; T-A1d-09, T-A1d-27). Chiave opaca, mai spezzata (T-A1d-FE-01).
type Gruppo struct {
	Chiave     string                          // "bom" | "cand:<allegato_id>:<radice>" | "senza"
	Tipo       string                          // "bom" | "cand" | "senza"
	AllegatoID *uuid.UUID                      // lo STEP del gruppo; nil per "senza"
	Radice     string                          // il Rif della radice della struttura; "" per "senza"
	NomeFile   string                          // il nome del file dello STEP; "" per "senza"
	Struttura  *valutazione.StrutturaCandidata // solo per "cand"
	Nodi       []*valutazione.NodoBOM          // i nodi del gruppo, ognuno una volta, nell'ordine del gruppo
}

// Figlio: un figlio di un nodo nel gruppo (X-04; contratto §1.6.1).
type Figlio struct {
	Nodo *valutazione.NodoBOM
	// Origine: OrigineFile per un arco della struttura dello STEP (la Parentela di un nodo del file), OrigineConfermata
	// per un confermato senza STEP, che nel gruppo bom entra come figlio del suo padre attraverso la relazione confermata
	// (R9 della revisione dell'indice). È una composizione di dati, non una regola.
	Origine            string
	Quantita           *int // dai fatti dello STEP; nil per un confermato senza STEP (T-A1d-20)
	QuantitaConfermata *int // la quantità della relazione confermata (contratto §1.6.1)
	Decisa             bool // Parentela.Decisa
}

// Legami: «nel file sotto X» e «confermato sotto Y», sempre accanto, mai fusi (X-05; v1.1 §4.5).
type Legami struct {
	DelFile    []LegameFile
	Confermati []LegameConfermato
}

// LegameFile: un padre del nodo nel file (la Parentela), con la quantità dei fatti e quella confermata accanto.
type LegameFile struct {
	Padre              string // il Rif del padre nel file
	Quantita           *int
	QuantitaConfermata *int
	Decisa             bool
}

// LegameConfermato: una relazione confermata corrente verso il componente del nodo (fotorfq.Thread.Relazioni, cioè
// componente_relazione dopo sposta, collega e scollega).
type LegameConfermato struct {
	PadreID  uuid.UUID // componente.id del padre: i campi «padre» e «da» dei gesti
	PadreRif string    // il Rif del padre nel gruppo, o in Nodi; «componente:<uuid>» se non c'è (rifNelGruppo)
	Codice   string    // il codice del padre
	Qta      int       // componente_relazione.qta
	NelFile  bool      // il padre confermato è il componente di un padre del file del nodo (R8)
}

// ComponenteRegistrato: il componente registrato (v1.1 §4.6): Rev e Descrizione come sono nel DB.
type ComponenteRegistrato struct {
	ID              uuid.UUID
	Codice          string
	Rev             *string // registrata
	Descrizione     *string // registrata; nil = nessuna («Rev e descrizione» resta spento)
	Tipo            string
	Archiviato      bool
	StepStrutturale *uuid.UUID
}

// DocumentoCorrente: un documento corrente dello stesso componente e tipo: le risposte di «scelta» (aggiungi o
// sostituisci:<id>) di assegna (v1.1 §4.11).
type DocumentoCorrente struct {
	DocumentoID uuid.UUID
	NomeFile    string
	Rev         *string
}

// Le origini di un figlio nel gruppo (Figlio.Origine).
const (
	OrigineFile       = "file"
	OrigineConfermata = "confermata"
)

// Le chiavi e i tipi dei gruppi (X-02; T-A1d-09): stringhe opache e stabili, che non dipendono dall'ordine. Da V1 stanno
// nel core (propostabom, emendamento di V1 §6.1), che le usa anche per il registro d'importazione e per il gruppo
// corrente della sorgente di una bozza: qui restano i nomi della pagina, gli stessi valori.
const (
	GruppoBOM       = propostabom.GruppoBOM
	GruppoCandidata = propostabom.GruppoCandidata
	GruppoSenzaSTEP = propostabom.GruppoSenzaSTEP
)

// AsseStruttura: l'asse di ricerca «struttura» di Conflitti, tutti gli assi tranne lo smistamento (T-A1d-25; piano
// del frontend D-PF-15). Non è un valore di valutazione.AsseConflitto: è un raggruppamento di ricerca (D-A1d-11).
const AsseStruttura = "struttura"

// prefissoComponente: «componente:», il prefisso di ancoraggio.RifComponente, preso dalla funzione stessa.
var prefissoComponente = strings.TrimSuffix(ancoraggio.RifComponente(uuid.Nil), uuid.Nil.String())

// ---- i metodi del contratto §2.3 (11) ----

// File: il record del file (e.File con AllegatoID == a); nil se assente.
func (x *Indice) File(a uuid.UUID) *valutazione.FileInterpretato {
	if x.vuoto() {
		return nil
	}
	return x.file[a]
}

// NomeFile: il nome dell'allegato, fra tutti gli allegati del thread; "" se assente.
func (x *Indice) NomeFile(a uuid.UUID) string {
	if x.vuoto() {
		return ""
	}
	if al := x.allegati[a]; al != nil {
		return al.NomeFile
	}
	return ""
}

// Associazione: l'associazione del file (e.Associazioni con AllegatoID == a); nil se assente.
func (x *Indice) Associazione(a uuid.UUID) *valutazione.AssociazioneFile {
	if x.vuoto() {
		return nil
	}
	return x.assoc[a]
}

// Badge: il badge vecchio/nuovo del file (c.File con File.AllegatoID == a); nil se assente o senza confronto.
func (x *Indice) Badge(a uuid.UUID) *confronto.EsitoFile {
	if x.vuoto() {
		return nil
	}
	return x.badge[a]
}

// Prodotto: il prodotto valutato con quel Rif; nil se assente.
func (x *Indice) Prodotto(rif string) *valutazione.ProdottoValutato {
	if x.vuoto() {
		return nil
	}
	return x.prodotti[rif]
}

// Nodo: il primo nodo con quel Rif (T-A1d-10), in ordine: Nodi, i Nodi di ogni struttura candidata (nell'ordine
// dell'esito), ConfermatiSenzaSTEP. È un metodo del contratto §2.3, ma con valutazione-3 non basta per il tipo: lo stesso
// Rif in due gruppi (D6 di P7e) può avere Tipo, OrigineTipo, disegni e classificazione diversi (D-CHIUS-4), e la radice
// della BOM di lavoro prende il componente del target solo lì; anche la catena dei codici (Nodo.Codice) dipende dalla
// struttura: il codice confermato del prodotto sta solo sulla radice della BOM di lavoro (confermatoDellaRadiceScelta,
// T-B4-38; R59 A). Resta come metodo del contratto, senza un uso consigliato: la pagina legge NodoIn (IX-05).
func (x *Indice) Nodo(prodotto, rif string) *valutazione.NodoBOM {
	p := x.Prodotto(prodotto)
	if p == nil {
		return nil
	}
	for i := range p.Nodi {
		if p.Nodi[i].Nodo.Rif == rif {
			return &p.Nodi[i]
		}
	}
	for k := range p.StruttureCandidate {
		for i := range p.StruttureCandidate[k].Nodi {
			if p.StruttureCandidate[k].Nodi[i].Nodo.Rif == rif {
				return &p.StruttureCandidate[k].Nodi[i]
			}
		}
	}
	for i := range p.ConfermatiSenzaSTEP {
		if p.ConfermatiSenzaSTEP[i].Nodo.Rif == rif {
			return &p.ConfermatiSenzaSTEP[i]
		}
	}
	return nil
}

// QuantitaConfermata: la quantità dell'arco confermato fra padre e figlio nel gruppo (contratto §1.6.1), una copia; nil
// se non c'è. Il gruppo conta (IX-01 della revisione di A1d-P2): lo stesso STEP in due allegati dà gli stessi Rif nella
// BOM di lavoro e in una candidata, e una candidata non prende la quantità confermata della BOM di lavoro (R59 A; la
// chiave è prodotto, struttura, Rif: D6, D-CHIUS-4).
//   - bom: l'arco della struttura della BOM di lavoro con la sua Decisione (Ancoraggi.Strutture[].Archi[].Decisione);
//     poi, per un confermato senza STEP appeso alla BOM, la sua Parentela decisa (lì la quantità è quella della
//     relazione confermata);
//   - cand:<allegato>:<radice>: solo l'arco della struttura con quell'allegato e quella radice;
//   - senza: solo la Parentela decisa del confermato senza STEP;
//   - altro: nil.
//
// Il contratto §2.3 dava QuantitaConfermata(prodotto, padre, figlio): il gruppo è un parametro in più, da ratificare con
// la v1.1.
func (x *Indice) QuantitaConfermata(prodotto, gruppo, padre, figlio string) *int {
	if x.vuoto() {
		return nil
	}
	var st *ancoraggio.StrutturaProdotto
	switch gruppo {
	case GruppoBOM:
		st = x.bomDi(prodotto)
	case GruppoSenzaSTEP:
	default:
		st = x.strutturaCandidata(prodotto, gruppo)
		if st == nil {
			return nil
		}
	}
	if st != nil {
		for a := range st.Archi {
			arco := &st.Archi[a]
			if arco.Padre == padre && arco.Figlio == figlio && arco.Decisione != nil && arco.Decisione.Quantita != nil {
				return copiaIntero(arco.Decisione.Quantita)
			}
		}
	}
	if gruppo != GruppoBOM && gruppo != GruppoSenzaSTEP {
		return nil
	}
	if p := x.Prodotto(prodotto); p != nil {
		for i := range p.ConfermatiSenzaSTEP {
			n := &p.ConfermatiSenzaSTEP[i]
			if n.Nodo.Rif != figlio {
				continue
			}
			for _, par := range n.Parentela {
				if par.Padre == padre && par.Decisa {
					return copiaIntero(par.Quantita)
				}
			}
		}
	}
	return nil
}

// PropostaAperta: il documento_proposta.id aperto del file, una copia; nil se non c'è (contratto §3.4 C). La proposta
// aperta di un allegato è una sola (UNIQUE).
func (x *Indice) PropostaAperta(a uuid.UUID) *uuid.UUID {
	if x.vuoto() || x.t == nil {
		return nil
	}
	for i := range x.t.Proposte {
		if x.t.Proposte[i].AllegatoID == a && x.t.Proposte[i].Stato == string(db.StatoPropostaAperta) {
			id := x.t.Proposte[i].ID
			return &id
		}
	}
	return nil
}

// AutorizzazioneDi: i campi dell'anteprima dello STEP strutturale per il prodotto e l'allegato (contratto §3.4 A;
// T-A1d-23). Nessuna struttura del prodotto nell'allegato → nil; una → AutorizzazioneDiStruttura; più d'una → il modo
// documento senza Sorgente (l'anteprima elenca le radici e non ne sceglie nessuna, U7), nil senza il documento. Se lo
// STEP si può autorizzare lo dice l'anteprima, non questa ricerca.
func (x *Indice) AutorizzazioneDi(prodotto string, a uuid.UUID) *Autorizzazione {
	ss := x.struttureNellAllegato(prodotto, a)
	switch len(ss) {
	case 0:
		return nil
	case 1:
		return x.AutorizzazioneDiStruttura(prodotto, a, ss[0].Radice)
	}
	p := x.Prodotto(prodotto)
	if p == nil || p.ComponenteID == nil {
		return nil
	}
	doc := x.documentoDelComponente(*p.ComponenteID, a)
	if doc == nil {
		return nil
	}
	return &Autorizzazione{Documento: doc}
}

// RelazioneProposta: la riga aperta di relazione_proposta dell'arco (padre, figlio sono Rif di nodi) fra gli allegati
// delle strutture del prodotto (contratto §3.4 B): il Rif si ricompone con ancoraggio.RifNodo e lo sha256 della
// struttura, mai spezzato. nil se non c'è, o se la riga è in più allegati del prodotto (lo stesso contenuto in due file:
// si decide nel Fascicolo).
func (x *Indice) RelazioneProposta(prodotto, padre, figlio string) *RelazioneAperta {
	if x.vuoto() || x.t == nil {
		return nil
	}
	sha := map[uuid.UUID]string{}
	for s := range x.e.Ancoraggi.Strutture {
		if st := &x.e.Ancoraggi.Strutture[s]; st.Target == prodotto {
			sha[st.AllegatoID] = st.Sha256
		}
	}
	var trovata *RelazioneAperta
	for i := range x.t.RigheRelazioneProposta {
		r := &x.t.RigheRelazioneProposta[i]
		s, ok := sha[r.AllegatoID]
		if !ok || r.Stato != string(db.StatoPropostaAperta) ||
			ancoraggio.RifNodo(s, r.PadreChiave) != padre || ancoraggio.RifNodo(s, r.FiglioChiave) != figlio {
			continue
		}
		if trovata == nil {
			trovata = &RelazioneAperta{Allegato: r.AllegatoID, Padre: r.PadreChiave, Figlio: r.FiglioChiave}
		} else if trovata.Allegato != r.AllegatoID {
			return nil
		}
	}
	return trovata
}

// Rimozione: la rimozione aperta di un conflitto (contratto §3.4 B); nil se il conflitto non è una rimozione (tipo
// arco, motivo rimozione, con il documento dello STEP). La riga si trova ricostruendo il Rif del conflitto come lo
// costruisce valutazione (RifArco fra i due RifComponente): la stringa del conflitto non si spezza.
func (x *Indice) Rimozione(c valutazione.Conflitto) *RimozioneAperta {
	if x.vuoto() || x.t == nil {
		return nil
	}
	if c.Tipo != valutazione.ConflittoArco || c.Motivo != valutazione.MotivoConflittoRimozione ||
		c.EvidenzaProposta.DocumentoID == nil {
		return nil
	}
	step := *c.EvidenzaProposta.DocumentoID
	for _, r := range x.t.RimozioniAperte {
		if r.StepDocumentoID == step &&
			valutazione.RifArco(ancoraggio.RifComponente(r.PadreID), ancoraggio.RifComponente(r.FiglioID)) == c.Rif {
			return &RimozioneAperta{Step: r.StepDocumentoID, Padre: r.PadreID, Figlio: r.FiglioID}
		}
	}
	return nil
}

// ---- le ricerche in più (v1.1 §4.3; X-02…X-06, X-09) ----

// Gruppi: le schede dell'albero del prodotto, nessuna preselezionata (nessun campo dice «scelta»; T-A1d-09, T-A1d-27):
// «bom» se il prodotto ha la BOM di lavoro (Nodi), con lo STEP e la radice della sua struttura; una «cand» per ogni
// struttura candidata, nell'ordine dell'esito; «senza» se ci sono confermati senza STEP. In cache per prodotto.
func (x *Indice) Gruppi(prodotto string) []Gruppo {
	p := x.Prodotto(prodotto)
	if p == nil {
		return nil
	}
	if g, ok := x.gruppi[prodotto]; ok {
		return g
	}
	var out []Gruppo
	if len(p.Nodi) > 0 {
		g := Gruppo{Chiave: GruppoBOM, Tipo: GruppoBOM, Nodi: x.nodiDelGruppo(prodotto, GruppoBOM)}
		if st := x.bomDi(prodotto); st != nil {
			id := st.AllegatoID
			g.AllegatoID, g.Radice, g.NomeFile = &id, st.Radice, x.NomeFile(st.AllegatoID)
		}
		out = append(out, g)
	}
	for k := range p.StruttureCandidate {
		sc := &p.StruttureCandidate[k]
		id := sc.AllegatoID
		chiave := chiaveCandidata(sc)
		out = append(out, Gruppo{Chiave: chiave, Tipo: GruppoCandidata, AllegatoID: &id, Radice: sc.Radice,
			NomeFile: x.NomeFile(sc.AllegatoID), Struttura: sc, Nodi: x.nodiDelGruppo(prodotto, chiave)})
	}
	if len(p.ConfermatiSenzaSTEP) > 0 {
		out = append(out, Gruppo{Chiave: GruppoSenzaSTEP, Tipo: GruppoSenzaSTEP, Nodi: x.nodiDelGruppo(prodotto, GruppoSenzaSTEP)})
	}
	if x.gruppi == nil {
		x.gruppi = map[string][]Gruppo{}
	}
	x.gruppi[prodotto] = out
	return out
}

// NodoIn: il nodo con quel Rif nel solo gruppo (D6 di P7e: lo stesso Rif in due strutture); nil fuori dal gruppo.
func (x *Indice) NodoIn(prodotto, gruppo, rif string) *valutazione.NodoBOM {
	for _, n := range x.nodiDelGruppo(prodotto, gruppo) {
		if n.Nodo.Rif == rif {
			return n
		}
	}
	return nil
}

// Radici: i nodi del gruppo senza un padre nel gruppo, nell'ordine del gruppo: un nodo è radice se nessuna sua
// Parentela ha per Padre il Rif di un nodo dello stesso gruppo.
func (x *Indice) Radici(prodotto, gruppo string) []*valutazione.NodoBOM {
	nn := x.nodiDelGruppo(prodotto, gruppo)
	nel := make(map[string]bool, len(nn))
	for _, n := range nn {
		nel[n.Nodo.Rif] = true
	}
	var out []*valutazione.NodoBOM
	for _, n := range nn {
		if !unPadreNelGruppo(n, nel) {
			out = append(out, n)
		}
	}
	return out
}

// Figli: i figli di rif nel gruppo, nell'ordine del gruppo: ogni nodo con una Parentela verso rif, una voce per arco.
// Un figlio con due padri compare sotto tutti e due (contratto §1.6.1). Quantita è quella dei fatti (Parentela); per un
// confermato senza STEP è nil, perché lì la Parentela porta la quantità confermata (T-A1d-20), che sta in
// QuantitaConfermata come per ogni altro arco.
func (x *Indice) Figli(prodotto, gruppo, rif string) []Figlio {
	var out []Figlio
	for _, n := range x.nodiDelGruppo(prodotto, gruppo) {
		for _, par := range n.Parentela {
			if par.Padre != rif {
				continue
			}
			f := Figlio{Nodo: n, Origine: OrigineConfermata, QuantitaConfermata: x.QuantitaConfermata(prodotto, gruppo, rif, n.Nodo.Rif),
				Decisa: par.Decisa}
			if !confermatoSenzaSTEP(n) {
				f.Origine, f.Quantita = OrigineFile, copiaIntero(par.Quantita)
			}
			out = append(out, f)
		}
	}
	return out
}

// Legami: «nel file sotto X» e «confermato sotto Y» del nodo nel gruppo, sempre accanto (X-05; D5 di P7e).
//   - DelFile: una voce per Parentela, con la quantità dei fatti e quella confermata; vuoto per un confermato senza STEP,
//     che non ha un file (T-A1d-20).
//   - Confermati: le relazioni confermate correnti verso il componente del nodo (componenteDelNodo), nell'ordine della
//     fotografia, con il Rif del padre nel gruppo (rifNelGruppo) e NelFile se il padre confermato è il componente di
//     uno dei padri del file.
func (x *Indice) Legami(prodotto, gruppo, rif string) Legami {
	var out Legami
	n := x.NodoIn(prodotto, gruppo, rif)
	if n == nil {
		return out
	}
	if !confermatoSenzaSTEP(n) {
		for _, par := range n.Parentela {
			out.DelFile = append(out.DelFile, LegameFile{Padre: par.Padre, Quantita: copiaIntero(par.Quantita),
				QuantitaConfermata: x.QuantitaConfermata(prodotto, gruppo, par.Padre, rif), Decisa: par.Decisa})
		}
	}
	p := x.Prodotto(prodotto)
	k := x.componenteDelNodo(p, gruppo, n)
	if k == nil || x.t == nil {
		return out
	}
	for _, r := range x.t.Relazioni {
		if r.FiglioID != *k {
			continue
		}
		c := LegameConfermato{PadreID: r.PadreID, PadreRif: x.rifNelGruppo(p, gruppo, r.PadreID), Qta: r.Qta}
		if comp := x.componenti[r.PadreID]; comp != nil {
			c.Codice = comp.Codice
		}
		// NelFile: il padre confermato è il componente di uno dei padri del file del nodo, confrontato per componente
		// e non per il primo Rif che lo rappresenta (R8: lo stesso componente deciso su due Rif del gruppo)
		for _, d := range out.DelFile {
			if pk := x.componenteDelNodo(p, gruppo, x.NodoIn(prodotto, gruppo, d.Padre)); pk != nil && *pk == r.PadreID {
				c.NelFile = true
				break
			}
		}
		out.Confermati = append(out.Confermati, c)
	}
	return out
}

// Componente: il componente registrato (v1.1 §4.6). v è un uuid.UUID, un *uuid.UUID (nil → nil) o un Rif
// «componente:<uuid>», che si accetta solo se è esattamente ancoraggio.RifComponente di quell'UUID; altro → nil.
func (x *Indice) Componente(v any) *ComponenteRegistrato {
	if x.vuoto() {
		return nil
	}
	var id uuid.UUID
	switch w := v.(type) {
	case uuid.UUID:
		id = w
	case *uuid.UUID:
		if w == nil {
			return nil
		}
		id = *w
	case string:
		resto, ok := strings.CutPrefix(w, prefissoComponente)
		if !ok {
			return nil
		}
		u, err := uuid.Parse(resto)
		if err != nil || ancoraggio.RifComponente(u) != w {
			return nil
		}
		id = u
	default:
		return nil
	}
	c := x.componenti[id]
	if c == nil {
		return nil
	}
	return &ComponenteRegistrato{ID: c.ID, Codice: c.Codice, Rev: copiaTesto(c.Rev), Descrizione: copiaTesto(c.Descrizione),
		Tipo: c.Tipo, Archiviato: c.ArchiviatoIl != nil, StepStrutturale: copiaUUID(c.StepStrutturaleID)}
}

// Correnti: i documenti correnti (SostituitoDa nil) del componente con quel tipo, nell'ordine della fotografia
// (T-A1d-26): le risposte possibili di «scelta» in assegna.
func (x *Indice) Correnti(componente uuid.UUID, tipo string) []DocumentoCorrente {
	if x.vuoto() || x.t == nil {
		return nil
	}
	var out []DocumentoCorrente
	for i := range x.t.Documenti {
		d := &x.t.Documenti[i]
		if d.ComponenteID != nil && *d.ComponenteID == componente && d.Tipo == tipo && d.SostituitoDa == nil {
			out = append(out, DocumentoCorrente{DocumentoID: d.ID, NomeFile: d.NomeFile, Rev: copiaTesto(d.Rev)})
		}
	}
	return out
}

// Anteprima: il riferimento per l'anteprima di un allegato, nella forma di Disegno2D.Anteprima (X-09), una copia.
// Prima il primo 2D dell'esito con quell'allegato, con il Disponibile di valutazione; altrimenti dall'allegato stesso:
// nil senza sha256, il formato dall'estensione dichiarata o da quella del nome (come valutazione), e Disponibile falso
// (IX-07 della revisione di A1d-P2): la regola di valutazione (anteprimaDisponibile) esclude il PDF che il worker non
// apre, e per un allegato che non è un 2D dell'esito il web non la ricostruisce. Si dice solo che c'è un originale; se
// si apre lo dice la GET dell'anteprima (404/409/415, D-A1d-09).
func (x *Indice) Anteprima(a uuid.UUID) *valutazione.RiferimentoAnteprima {
	if x.vuoto() {
		return nil
	}
	if r := x.anteprime[a]; r != nil {
		c := copiaAnteprima(*r)
		return &c
	}
	al := x.allegati[a]
	if al == nil || al.Sha256 == nil || *al.Sha256 == "" {
		return nil
	}
	id := a
	f := valutazione.FormatoDi(estensioneDi(al))
	return &valutazione.RiferimentoAnteprima{Sha256: *al.Sha256, Formato: f, AllegatoID: &id}
}

// Conflitti: i conflitti del prodotto (T-A1d-25): asse "" tutti; "struttura" tutti tranne lo smistamento; uno dei tre
// assi di valutazione.AsseConflitto quell'asse; ogni altro valore nessuno.
func (x *Indice) Conflitti(prodotto, asse string) []valutazione.Conflitto {
	if x.vuoto() {
		return nil
	}
	switch valutazione.AsseConflitto(asse) {
	case "", AsseStruttura, valutazione.AsseNomenclatura, valutazione.AsseGerarchia, valutazione.AsseSmistamento:
	default:
		return nil
	}
	var out []valutazione.Conflitto
	for _, c := range x.e.Conflitti {
		if c.Prodotto != prodotto {
			continue
		}
		if asse == "" || (asse == AsseStruttura && c.Asse != valutazione.AsseSmistamento) || string(c.Asse) == asse {
			out = append(out, c)
		}
	}
	return out
}

// ConflittiDelThread: i conflitti senza prodotto (Prodotto "").
func (x *Indice) ConflittiDelThread() []valutazione.Conflitto {
	if x.vuoto() {
		return nil
	}
	var out []valutazione.Conflitto
	for _, c := range x.e.Conflitti {
		if c.Prodotto == "" {
			out = append(out, c)
		}
	}
	return out
}

// Voce: la voce certa del fabbisogno del prodotto per (componente, tipo di documento); nil se non c'è.
func (x *Indice) Voce(prodotto string, componente uuid.UUID, tipo string) *valutazione.VoceFabbisogno {
	p := x.Prodotto(prodotto)
	if p == nil {
		return nil
	}
	for i := range p.Documenti.Voci {
		if v := &p.Documenti.Voci[i]; v.ComponenteID == componente && v.TipoDocumento == tipo {
			return v
		}
	}
	return nil
}

// CopieAperte: le voci del prodotto con la copia sul NAS non completata (StatoNas in coda o in errore), in ordine
// (T-A1d-26).
func (x *Indice) CopieAperte(prodotto string) []*valutazione.VoceFabbisogno {
	p := x.Prodotto(prodotto)
	if p == nil {
		return nil
	}
	var out []*valutazione.VoceFabbisogno
	for i := range p.Documenti.Voci {
		if v := &p.Documenti.Voci[i]; v.StatoNas == string(db.StatoNasInCoda) || v.StatoNas == string(db.StatoNasErrore) {
			out = append(out, v)
		}
	}
	return out
}

// CandidatiFuori: i candidati della fonte che non sono né una struttura candidata né lo STEP della BOM di lavoro
// (R-FE-11; T-A1d-24; R7 della revisione dell'indice), copie: un candidato è «dentro» se ha l'allegato o lo sha256 di una
// struttura candidata del prodotto o della struttura della sua BOM di lavoro.
func (x *Indice) CandidatiFuori(prodotto string) []ancoraggio.DocumentoCandidato {
	p := x.Prodotto(prodotto)
	if p == nil {
		return nil
	}
	var out []ancoraggio.DocumentoCandidato
	for i := range p.Fonte.Candidati {
		c := &p.Fonte.Candidati[i]
		dentro := false
		if st := x.bomDi(prodotto); st != nil &&
			((c.AllegatoID != nil && *c.AllegatoID == st.AllegatoID) || (c.Sha256 != "" && c.Sha256 == st.Sha256)) {
			dentro = true
		}
		for k := 0; !dentro && k < len(p.StruttureCandidate); k++ {
			sc := &p.StruttureCandidate[k]
			if (c.AllegatoID != nil && *c.AllegatoID == sc.AllegatoID) || (c.Sha256 != "" && c.Sha256 == sc.Sha256) {
				dentro = true
				break
			}
		}
		if !dentro {
			out = append(out, copiaCandidato(*c))
		}
	}
	return out
}

// AutorizzazioneDiStruttura: i campi dell'anteprima dello STEP strutturale per una struttura del prodotto (l'allegato e
// la radice; contratto §3.4 A; T-A1d-23):
//  1. il prodotto con il suo componente (la rotta vuole {cid}), altrimenti nil;
//  2. la struttura del prodotto in quell'allegato con quella radice, altrimenti nil;
//  3. la chiave della radice: quella della riga di componente_proposta dell'allegato il cui RifNodo è la radice; senza
//     la riga nil (il Rif non si spezza: R10 della revisione dell'indice, T-A1d-FE-01; la PR #18 ricavava la chiave
//     togliendo il prefisso dal Rif);
//  4. radice del file → modo documento: il primo documento corrente del componente del prodotto che ha quell'allegato
//     fra i suoi (lo STEP si autorizza con un documento confermato: P-14, D-A1d-08), con la chiave come Sorgente; senza
//     il documento nil;
//  5. radice interna → modo delega: Nodo «<allegato_id>:<chiave>», senza Sorgente.
func (x *Indice) AutorizzazioneDiStruttura(prodotto string, a uuid.UUID, radice string) *Autorizzazione {
	if x.vuoto() || x.t == nil {
		return nil
	}
	p := x.Prodotto(prodotto)
	if p == nil || p.ComponenteID == nil {
		return nil
	}
	var st *ancoraggio.StrutturaProdotto
	for _, s := range x.struttureNellAllegato(prodotto, a) {
		if s.Radice == radice {
			st = s
			break
		}
	}
	if st == nil {
		return nil
	}
	chiave := ""
	for i := range x.t.RigheComponenteProposta {
		r := &x.t.RigheComponenteProposta[i]
		if r.AllegatoID == a && ancoraggio.RifNodo(r.Sha256, r.Chiave) == radice {
			chiave = r.Chiave
			break
		}
	}
	if chiave == "" {
		return nil
	}
	if !st.RadiceDelFile {
		return &Autorizzazione{Nodo: a.String() + ":" + chiave}
	}
	doc := x.documentoDelComponente(*p.ComponenteID, a)
	if doc == nil {
		return nil
	}
	return &Autorizzazione{Documento: doc, Sorgente: chiave}
}

// ---- le autorizzazioni dello STEP (direttiva §3.4; F-16) ----

// StatoAutorizzazione: in quale insieme il Fascicolo mette una dichiarazione (fascicolo.Dichiarazioni): valida, da
// sistemare (conflitto, superata, incoerente, P7, catena rotta: fermano il gate), sospesa (un avviso). Si legge dai
// predicati del Fascicolo (Dichiarazione.Valida, Dichiarazione.Sospesa), senza regole nuove.
type StatoAutorizzazione string

const (
	AutorizzazioneValida      StatoAutorizzazione = "valida"
	AutorizzazioneDaSistemare StatoAutorizzazione = "da_sistemare"
	AutorizzazioneSospesa     StatoAutorizzazione = "sospesa"
)

// AutorizzazioneSTEP: una dichiarazione dello STEP di un componente, come la valuta il Fascicolo
// (fascicolo.ValutaDichiarazioni, nello stesso snapshot della fotografia). Problema e Frase sono i testi del Fascicolo,
// così come sono; Sorgenti le chiavi dei nodi sorgente, in ordine.
type AutorizzazioneSTEP struct {
	Stato     StatoAutorizzazione
	Problema  string
	Frase     string
	Sha256    string
	NomeFile  string
	Allegato  uuid.UUID
	Documento *uuid.UUID
	Sorgenti  []string
	Delega    bool
	Origine   string // fascicolo.OrigineSmistamento | fascicolo.OrigineStepStrutturale
}

// Autorizzazioni: le dichiarazioni dello STEP del componente, valide o no (fascicolo.Dichiarazioni.DelComponente),
// nell'ordine del Fascicolo; nil senza dichiarazioni o senza la lettura. Dice lo stato attuale; se un gesto si può
// fare lo dice l'anteprima (AnteprimaSTEP), che rilegge in una transazione sua.
func (x *Indice) Autorizzazioni(componente uuid.UUID) []AutorizzazioneSTEP {
	if x.vuoto() || x.d == nil {
		return nil
	}
	var out []AutorizzazioneSTEP
	for _, d := range x.d.DelComponente(componente) {
		a := AutorizzazioneSTEP{Stato: AutorizzazioneDaSistemare, Problema: d.Problema, Frase: d.Frase(), Sha256: d.Sha256,
			NomeFile: d.NomeFile, Allegato: d.Allegato, Sorgenti: d.ChiaviSorgenti(), Delega: d.Delega(), Origine: d.Origine}
		switch {
		case d.Valida():
			a.Stato = AutorizzazioneValida
		case d.Sospesa():
			a.Stato = AutorizzazioneSospesa
		}
		if d.Documento.Valid {
			id := d.Documento.UUID
			a.Documento = &id
		}
		out = append(out, a)
	}
	return out
}

// Marcatura: la marcatura (evidenza.strutturale, il gesto 3) della riga di componente_proposta dell'allegato con quella
// chiave, una copia così com'è nella fotografia (ruolo, sospesa, chi e quando); nil senza riga o senza marcatura.
// Nessuna regola: se la marcatura vale lo dice Autorizzazioni.
func (x *Indice) Marcatura(a uuid.UUID, chiave string) *fotorfq.MarcaturaStrutturale {
	if x.vuoto() || x.t == nil {
		return nil
	}
	for i := range x.t.RigheComponenteProposta {
		r := &x.t.RigheComponenteProposta[i]
		if r.AllegatoID == a && r.Chiave == chiave && r.Marcatura != nil {
			m := *r.Marcatura
			m.ComponenteID, m.DocumentoID, m.DichiaratoDa = copiaUUID(m.ComponenteID), copiaUUID(m.DocumentoID), copiaUUID(m.DichiaratoDa)
			return &m
		}
	}
	return nil
}

// StepProdotto: la riga di v_step_prodotto del componente (fotorfq.Thread.StepProdotto), una copia così com'è; nil se
// non c'è.
func (x *Indice) StepProdotto(componente uuid.UUID) *fotorfq.RigaStepProdotto {
	if x.vuoto() || x.t == nil {
		return nil
	}
	for i := range x.t.StepProdotto {
		if r := x.t.StepProdotto[i]; r.ComponenteID == componente {
			r.StepStrutturaleID, r.DerogaStrutturaID = copiaUUID(r.StepStrutturaleID), copiaUUID(r.DerogaStrutturaID)
			r.Altro3DDocumentoID, r.PropostaAperta = copiaUUID(r.Altro3DDocumentoID), copiaUUID(r.PropostaAperta)
			r.AttesoDaPortale, r.MotivoParziale = copiaUUID(r.AttesoDaPortale), copiaTesto(r.MotivoParziale)
			return &r
		}
	}
	return nil
}

// ---- i nodi di un gruppo, il componente di un nodo e il Rif di un padre (T-A1d-21, T-A1d-22) ----

// nodiDelGruppo: i nodi di un gruppo, ognuno una volta (T-A1d-21; X-03), in cache per prodotto e gruppo:
//   - bom: i nodi di Nodi, in ordine; poi, ripetendo sui confermati senza STEP in ordine finché se ne aggiunge uno,
//     quelli con un padre (Parentela.Padre) già nel gruppo: compaiono come figli del loro padre, e la chiusura porta
//     anche i figli dei figli;
//   - cand:…: i nodi della struttura candidata la cui chiave ricomposta è il gruppo (la chiave non si spezza);
//   - senza: tutti i confermati senza STEP;
//   - altro: nil.
func (x *Indice) nodiDelGruppo(prodotto, gruppo string) []*valutazione.NodoBOM {
	p := x.Prodotto(prodotto)
	if p == nil {
		return nil
	}
	chiave := prodotto + "\x00" + gruppo
	if n, ok := x.nodi[chiave]; ok {
		return n
	}
	var out []*valutazione.NodoBOM
	switch gruppo {
	case GruppoBOM:
		nel := map[string]bool{}
		for i := range p.Nodi {
			out = append(out, &p.Nodi[i])
			nel[p.Nodi[i].Nodo.Rif] = true
		}
		aggiunto := make([]bool, len(p.ConfermatiSenzaSTEP))
		for altri := true; altri; {
			altri = false
			for i := range p.ConfermatiSenzaSTEP {
				if n := &p.ConfermatiSenzaSTEP[i]; !aggiunto[i] && unPadreNelGruppo(n, nel) {
					out = append(out, n)
					nel[n.Nodo.Rif], aggiunto[i], altri = true, true, true
				}
			}
		}
	case GruppoSenzaSTEP:
		for i := range p.ConfermatiSenzaSTEP {
			out = append(out, &p.ConfermatiSenzaSTEP[i])
		}
	default:
		for k := range p.StruttureCandidate {
			if sc := &p.StruttureCandidate[k]; chiaveCandidata(sc) == gruppo {
				for i := range sc.Nodi {
					out = append(out, &sc.Nodi[i])
				}
				break
			}
		}
	}
	if x.nodi == nil {
		x.nodi = map[string][]*valutazione.NodoBOM{}
	}
	x.nodi[chiave] = out
	return out
}

// ComponenteDelNodo: il componente del nodo rif nel gruppo del prodotto, una copia; nil senza (F-15: la sola ricerca, al
// posto delle copie nel template della PR #18). Vedi componenteDelNodo.
func (x *Indice) ComponenteDelNodo(prodotto, gruppo, rif string) *uuid.UUID {
	return x.componenteDelNodo(x.Prodotto(prodotto), gruppo, x.NodoIn(prodotto, gruppo, rif))
}

// componenteDelNodo: il componente di un nodo nel gruppo (T-A1d-22): la sua Decisione; altrimenti, solo nel gruppo bom
// e solo per la radice della BOM di lavoro, il componente del target (contratto §1.3, §1.4; T-B4-38: la radice della BOM
// di lavoro prende il componente del target, quella di una candidata no); altrimenti nil. Una copia.
//
// CONTRASTO dichiarato (F-15, tecnico): è la stessa regola di componenteDelNodo di core/valutazione (nodi.go), che non è
// esportata e che il web non può cambiare (nessuna modifica a core in A1d-P2). La copia sta solo qui, in un punto, ed è
// dichiarata nel README del modulo. La prova gemella è la L4 TestL4MotoreACalcoloVero (servizio_db_test.go, IX-03 della
// revisione di A1d-P2): su ogni nodo di ogni gruppo dell'esito vero, ComponenteDelNodo c'è se e solo se valutazione ha
// dato al nodo il tipo confermato del componente (datiDelNodo con il componente), e il tipo è quello del componente.
// Esportarla da valutazione è una scelta separata, che tocca core.
func (x *Indice) componenteDelNodo(p *valutazione.ProdottoValutato, gruppo string,
	n *valutazione.NodoBOM) *uuid.UUID {
	switch {
	case p == nil || n == nil:
		return nil
	case n.Nodo.Decisione != nil:
		id := n.Nodo.Decisione.ComponenteID
		return &id
	case gruppo == GruppoBOM && p.ComponenteID != nil:
		if st := x.bomDi(p.Rif); st != nil && n.Nodo.Rif == st.Radice {
			return copiaUUID(p.ComponenteID)
		}
	}
	return nil
}

// rifNelGruppo: il Rif con cui un componente padre compare nella pagina (T-A1d-22), in quest'ordine: il primo nodo del
// gruppo che lo rappresenta; solo nei gruppi bom e senza, il primo nodo di Nodi che lo rappresenta;
// ancoraggio.RifComponente. Il secondo e il terzo passo sono la regola con cui valutazione scrive il Padre della
// Parentela di un confermato senza STEP (P7e): così, nella scheda «senza», PadreRif coincide con il Padre che Figli e
// Radici leggono. In una candidata il secondo passo non vale (IX-02 della revisione di A1d-P2): un Rif della BOM di
// lavoro, con lo stesso sha256, può essere nella candidata un altro nodo (la radice della candidata non è il prodotto:
// D4, D-CHIUS-4), quindi si passa subito a ancoraggio.RifComponente.
func (x *Indice) rifNelGruppo(p *valutazione.ProdottoValutato, gruppo string, id uuid.UUID) string {
	if p == nil {
		return ancoraggio.RifComponente(id)
	}
	for _, n := range x.nodiDelGruppo(p.Rif, gruppo) {
		if k := x.componenteDelNodo(p, gruppo, n); k != nil && *k == id {
			return n.Nodo.Rif
		}
	}
	if gruppo != GruppoBOM && gruppo != GruppoSenzaSTEP {
		return ancoraggio.RifComponente(id)
	}
	for i := range p.Nodi {
		if k := x.componenteDelNodo(p, GruppoBOM, &p.Nodi[i]); k != nil && *k == id {
			return p.Nodi[i].Nodo.Rif
		}
	}
	return ancoraggio.RifComponente(id)
}

// bomDi: la prima struttura del prodotto che è la BOM di lavoro (al più una per prodotto, R76 A); nil senza.
func (x *Indice) bomDi(prodotto string) *ancoraggio.StrutturaProdotto {
	if x.vuoto() {
		return nil
	}
	for s := range x.e.Ancoraggi.Strutture {
		if st := &x.e.Ancoraggi.Strutture[s]; st.Target == prodotto && st.Stato == ancoraggio.StatoBOMDiLavoroProposta {
			return st
		}
	}
	return nil
}

// strutturaCandidata: la struttura di ancoraggio del prodotto che è la candidata del gruppo «cand:<allegato>:<radice>»
// (la chiave ricomposta, mai spezzata); nil se il gruppo non è una candidata del prodotto.
func (x *Indice) strutturaCandidata(prodotto, gruppo string) *ancoraggio.StrutturaProdotto {
	if x.vuoto() {
		return nil
	}
	for s := range x.e.Ancoraggi.Strutture {
		st := &x.e.Ancoraggi.Strutture[s]
		if st.Target == prodotto && st.Stato != ancoraggio.StatoBOMDiLavoroProposta &&
			propostabom.ChiaveCandidata(st.AllegatoID, st.Radice) == gruppo {
			return st
		}
	}
	return nil
}

// struttureNellAllegato: le strutture del prodotto in quell'allegato (BOM di lavoro o candidate), nell'ordine di
// ancoraggio (T-A1d-23).
func (x *Indice) struttureNellAllegato(prodotto string, a uuid.UUID) []*ancoraggio.StrutturaProdotto {
	if x.vuoto() {
		return nil
	}
	var out []*ancoraggio.StrutturaProdotto
	for s := range x.e.Ancoraggi.Strutture {
		if st := &x.e.Ancoraggi.Strutture[s]; st.Target == prodotto && st.AllegatoID == a {
			out = append(out, st)
		}
	}
	return out
}

// documentoDelComponente: l'ID del primo documento corrente del componente che ha l'allegato fra i suoi
// (documento_provenienza); nil senza. È il documento confermato che l'autorizzazione dello STEP vuole
// (fascicolo.fileProprio).
func (x *Indice) documentoDelComponente(componente, a uuid.UUID) *uuid.UUID {
	if x.vuoto() || x.t == nil {
		return nil
	}
	for i := range x.t.Documenti {
		d := &x.t.Documenti[i]
		if d.SostituitoDa != nil || d.ComponenteID == nil || *d.ComponenteID != componente {
			continue
		}
		for _, al := range d.Allegati {
			if al == a {
				id := d.ID
				return &id
			}
		}
	}
	return nil
}

// chiaveCandidata: la chiave del gruppo di una struttura candidata, «cand:<allegato_id>:<radice>» (X-02), quella del
// core (propostabom.ChiaveCandidata, V1): la pagina e il registro d'importazione usano la stessa.
func chiaveCandidata(sc *valutazione.StrutturaCandidata) string {
	return propostabom.ChiaveCandidata(sc.AllegatoID, sc.Radice)
}

// unPadreNelGruppo: il nodo ha almeno un padre (Parentela.Padre) fra i Rif dati.
func unPadreNelGruppo(n *valutazione.NodoBOM, nel map[string]bool) bool {
	for _, par := range n.Parentela {
		if nel[par.Padre] {
			return true
		}
	}
	return false
}

// confermatoSenzaSTEP: il nodo è un componente confermato senza STEP (EB7-1; contratto §1.4: il Rif è
// «componente:<uuid>», e non c'è un file). Non si guarda SenzaFile: anche un nodo dello STEP senza un file ancorato ha
// SenzaFile (ancoraggio, ricalcolaSenzaFile), e la sua Parentela porta la quantità dei fatti (contratto §1.6.1).
func confermatoSenzaSTEP(n *valutazione.NodoBOM) bool {
	return strings.HasPrefix(n.Nodo.Rif, prefissoComponente)
}

// estensioneDi: l'estensione dichiarata dell'allegato, o quella del nome, senza il punto e senza maiuscole (la stessa
// lettura di valutazione per la fonte, che non la esporta).
func estensioneDi(a *fotorfq.Allegato) string {
	if a.Estensione != nil && *a.Estensione != "" {
		return strings.ToLower(strings.TrimPrefix(*a.Estensione, "."))
	}
	return strings.ToLower(strings.TrimPrefix(path.Ext(a.NomeFile), "."))
}

func copiaIntero(v *int) *int {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

func copiaTesto(v *string) *string {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

func copiaAnteprima(r valutazione.RiferimentoAnteprima) valutazione.RiferimentoAnteprima {
	r.DocumentoID = copiaUUID(r.DocumentoID)
	r.AllegatoID = copiaUUID(r.AllegatoID)
	return r
}

func copiaCandidato(c ancoraggio.DocumentoCandidato) ancoraggio.DocumentoCandidato {
	c.AllegatoID = copiaUUID(c.AllegatoID)
	c.DocumentoID = copiaUUID(c.DocumentoID)
	c.Radici = copiaStringhe(c.Radici)
	return c
}
