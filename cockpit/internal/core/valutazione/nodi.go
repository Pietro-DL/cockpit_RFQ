package valutazione

import (
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
)

// nodi.go: il nodo della BOM come lo vedrà la UI (R91, T-B0-39; contratto §1.3, riga «BOM di lavoro proposta», §2.3, §2.5;
// T-E1-19; fase 0 di B6, F.2; T-B6-11). Solo i nodi della BOM di lavoro del prodotto, cioè della struttura sotto la radice
// scelta dello STEP confermato (R76 A): una struttura candidata non ha nodi qui, e i suoi nodi restano negli ancoraggi
// (EsitoThread.Ancoraggi.Strutture). Sui dati veri, senza STEP confermato, PV.Nodi è vuoto (T-B6-11). Il nodo unisce il
// NodoProposto di ancoraggio ai 2D, alle associazioni e alla classificazione, che vengono dalla fotografia.

// NodoBOM: un nodo della BOM di lavoro del prodotto.
//   - Parentela: i padri e le quantità, dagli archi.
//   - Disegni: per il nodo deciso il gruppo dei 2D del componente; per il nodo senza decisione i 2D candidati del nodo.
//   - Associazione: l'origine dell'associazione del 2D primario (confermato, manuale o proposto); nil = nessuna.
//   - Motivi: le ambiguità e i conflitti dei file candidati del nodo, con il vocabolario dello smistamento.
//   - Classificazione: l'esenzione dal 2D solo con EsenteDal2D (README, «la classificazione»). AncheIn: gli altri
//     prodotti con lo stesso componente deciso.
type NodoBOM struct {
	Nodo            ancoraggio.NodoProposto `json:"nodo"`
	Descrizione     *string                 `json:"descrizione,omitempty"`
	Parentela       []Parentela             `json:"parentela,omitempty"`
	Disegni         GruppoDisegni2D         `json:"disegni"`
	Associazione    *ancoraggio.OrigineDato `json:"associazione,omitempty"`
	Motivi          []MotivoSmistamento     `json:"motivi,omitempty"`
	Classificazione Classificazione         `json:"classificazione"`
	AncheIn         []string                `json:"anche_in,omitempty"`
}

// Parentela: un padre del nodo (contratto §2.3): il Rif del nodo padre, la quantità, l'arco confermato accanto.
type Parentela struct {
	Padre    string `json:"padre"`
	Quantita *int   `json:"quantita,omitempty"`
	Decisa   bool   `json:"decisa"`
}

// campoDescrizioneSTEP: il campo dell'unità della descrizione di un PRODUCT (PRODUCT.description) nel documento di uno
// STEP, come lo scrive l'adattatore di A1b (mappatura-step-1); ripetuto qui perché estrazione non lo esporta.
const campoDescrizioneSTEP = "descrizione"

// nodiThread: ciò che serve ai nodi della BOM dei prodotti di un thread, calcolato una volta. Le mappe servono solo a
// cercare, mai a scorrere: si scorrono gli elenchi della fotografia e degli ancoraggi, già in ordine.
type nodiThread struct {
	b            *bomDelThread
	dis          *disegniThread
	gruppi       map[uuid.UUID]*GruppoDisegni2D
	classi       map[uuid.UUID]Classificazione
	componenti   map[uuid.UUID]*fotorfq.Componente
	associazioni []AssociazioneFile // una per allegato, in ordine di AllegatoID
	perAllegato  map[uuid.UUID]*AssociazioneFile
	conflitti    []Conflitto // i conflitti dell'asse smistamento del thread (solo quell'asse), già composti
	documenti    map[uuid.UUID]evidenze.DocumentoEvidenze
	ancoraggi    []ancoraggio.AncoraggioFile // in ordine di ancoraggio
	perimetri    []perimetroDelProdotto      // nell'ordine dei prodotti
}

// perimetroDelProdotto: la BOM confermata di un prodotto (R62 e A: il suo componente e i componenti attivi raggiungibili
// per le relazioni confermate), per AncheIn: è lo stesso perimetro dell'impronta del prodotto (T-E1-19).
type perimetroDelProdotto struct {
	rif    string
	dentro map[uuid.UUID]bool
}

func nuoviNodiThread(t fotorfq.Thread, b *bomDelThread, dis *disegniThread, col *collegamento, disegni []DisegniDelComponente,
	classificazioni []ComponenteClassificato, associazioni []AssociazioneFile, conflitti []Conflitto, prodotti []ProdottoValutato,
	ancoraggi []ancoraggio.AncoraggioFile) *nodiThread {
	x := &nodiThread{b: b, dis: dis, gruppi: map[uuid.UUID]*GruppoDisegni2D{}, classi: map[uuid.UUID]Classificazione{},
		componenti: map[uuid.UUID]*fotorfq.Componente{}, associazioni: associazioni, perAllegato: map[uuid.UUID]*AssociazioneFile{},
		conflitti: conflitti, documenti: map[uuid.UUID]evidenze.DocumentoEvidenze{}, ancoraggi: ancoraggi}
	for i := range disegni {
		x.gruppi[disegni[i].ComponenteID] = &disegni[i].Gruppo
	}
	for _, c := range classificazioni {
		x.classi[c.ComponenteID] = c.Classificazione
	}
	for i := range t.Componenti {
		x.componenti[t.Componenti[i].ID] = &t.Componenti[i]
	}
	for i := range associazioni {
		x.perAllegato[associazioni[i].AllegatoID] = &associazioni[i]
	}
	for _, f := range col.file {
		x.documenti[f.AllegatoID] = f.Documento
	}
	for _, pv := range prodotti {
		dentro, _ := b.perimetro(pv.ComponenteID)
		x.perimetri = append(x.perimetri, perimetroDelProdotto{rif: pv.Rif, dentro: dentro})
	}
	return x
}

// nodiDellaBOM: i nodi della BOM di lavoro del prodotto (T-B6-11; R91; contratto §1.3), nell'ordine della struttura (la
// radice, poi gli altri nodi per Rif). Solo i nodi che la radice raggiunge senza passare da un arco tolto da una persona
// (le righe di relazione_proposta tutte scartate da chi le ha decise: bomDelThread.archiTolti, la stessa raggiungibilità
// dei previsti della completezza, T-B5-67); un nodo scartato ma ancora raggiunto resta, con la sua riga decisa accanto.
// Un nodo per Rif. Vuoto senza BOM di lavoro.
func (x *nodiThread) nodiDellaBOM(pv ProdottoValutato, strutture []ancoraggio.StrutturaProdotto) []NodoBOM {
	var out []NodoBOM
	visti := map[string]bool{}
	for _, st := range strutture {
		if st.Target != pv.Rif || st.Stato != ancoraggio.StatoBOMDiLavoroProposta {
			continue
		}
		raggiunti := x.raggiunti(st)
		for _, n := range st.Nodi {
			if !raggiunti[n.Rif] || visti[n.Rif] {
				continue
			}
			visti[n.Rif] = true
			out = append(out, x.nodo(pv, st, n, raggiunti))
		}
	}
	return out
}

// raggiunti: i nodi della struttura che la radice raggiunge negli archi, senza gli archi tolti da una persona.
func (x *nodiThread) raggiunti(st ancoraggio.StrutturaProdotto) map[string]bool {
	figli := map[string][]string{}
	for _, a := range st.Archi {
		if !x.b.archiTolti[RifArco(a.Padre, a.Figlio)] {
			figli[a.Padre] = append(figli[a.Padre], a.Figlio)
		}
	}
	out := map[string]bool{st.Radice: true}
	coda := []string{st.Radice}
	for len(coda) > 0 {
		p := coda[0]
		coda = coda[1:]
		for _, f := range figli[p] {
			if !out[f] {
				out[f] = true
				coda = append(coda, f)
			}
		}
	}
	return out
}

// nodo: il NodoBOM di un nodo della BOM di lavoro.
//   - Il componente del nodo: la decisione per UUID (NodoProposto.Decisione); per la radice scelta senza una decisione
//     propria, il componente del target (T-B4-38, come la catena del codice e i 2D di B5).
//   - Descrizione: quella del componente deciso, se c'è; altrimenti la descrizione del PRODUCT dello STEP del nodo
//     (PRODUCT.description nei fatti, com'è); nil senza.
//   - Parentela: per ogni arco della struttura verso il nodo, da un padre raggiunto e non tolto da una persona, il padre,
//     la quantità dei fatti e se accanto c'è l'arco confermato (ArcoProposto.Decisione: T-14).
//   - Disegni: per il nodo deciso il gruppo dei 2D del suo componente (ValutazioneProdotti.Disegni); per il nodo senza
//     decisione i 2D candidati del nodo (gruppoDelNodo, la stessa raccolta dei previsti). Il primario è quello di B5:
//     cambia quando un PDF non si apre (T-B5-39).
//   - Associazione: la provenienza del 2D primario nel gruppo del nodo (Disegno2D.Provenienza: confermato, manuale,
//     proposto), cioè l'origine della sua associazione a quel componente o a quel nodo; nil senza primario.
//   - Motivi: motiviDelNodo.
//   - Classificazione: per il componente deciso quella del thread (ValutazioneProdotti.Classificazioni, con il ruolo
//     prodotto per il componente del target); per il nodo senza decisione Classifica sulle categorie della lettura del nodo
//     (CatenaCodice.Forma), senza tipo deciso (non_determinata, con la minuteria solo proposta). L'esenzione dal 2D la
//     dice solo EsenteDal2D (R99 A, R103 C): qui non si decide niente.
//   - AncheIn: gli altri prodotti del thread la cui BOM confermata contiene il componente del nodo (T-E1-19), in ordine.
func (x *nodiThread) nodo(pv ProdottoValutato, st ancoraggio.StrutturaProdotto, n ancoraggio.NodoProposto, raggiunti map[string]bool) NodoBOM {
	var k *uuid.UUID
	switch {
	case n.Decisione != nil:
		id := n.Decisione.ComponenteID
		k = &id
	case n.Rif == st.Radice:
		k = copiaUUID(pv.ComponenteID)
	}
	out := NodoBOM{Nodo: n, Descrizione: x.descrizione(n, k)}
	for _, a := range st.Archi {
		if a.Figlio == n.Rif && raggiunti[a.Padre] && !x.b.archiTolti[RifArco(a.Padre, a.Figlio)] {
			out.Parentela = append(out.Parentela, Parentela{Padre: a.Padre, Quantita: copiaIntero(a.Quantita), Decisa: a.Decisione != nil})
		}
	}
	if k != nil {
		if c := copiaGruppo(x.gruppi[*k]); c != nil {
			out.Disegni = *c
		}
		out.Classificazione = x.classi[*k]
		for _, p := range x.perimetri {
			if p.rif != pv.Rif && p.dentro[*k] {
				out.AncheIn = append(out.AncheIn, p.rif)
			}
		}
		sort.Strings(out.AncheIn)
	} else {
		if g := x.dis.gruppoDelNodo(pv.Rif, n); g != nil {
			out.Disegni = *g
		}
		var categorie []string
		if f := n.Codice.Forma; f != nil {
			categorie = categorieDi(f.Categorie)
		}
		out.Classificazione = Classifica(ComponenteDaClassificare{Categorie: categorie}, nil)
	}
	if p := out.Disegni.Primario; p != nil {
		o := p.Provenienza
		out.Associazione = &o
	}
	out.Motivi = x.motiviDelNodo(pv, n, k)
	return out
}

// descrizione: la descrizione del componente deciso, se non è vuota; altrimenti quella del PRODUCT dello STEP del nodo,
// cioè l'unità della descrizione della sua entità nel documento del file (PRODUCT.description, com'è nei fatti); nil
// senza, o con un testo vuoto.
func (x *nodiThread) descrizione(n ancoraggio.NodoProposto, k *uuid.UUID) *string {
	if k != nil {
		if c := x.componenti[*k]; c != nil && c.Descrizione != nil && strings.TrimSpace(*c.Descrizione) != "" {
			return copiaTesto(c.Descrizione)
		}
	}
	d, ok := x.documenti[n.AllegatoID]
	if !ok {
		return nil
	}
	for _, u := range d.Unita {
		if u.EntitaID == n.EntitaID && u.Selettore.Campo.Variante == evidenze.VarianteStep && u.Selettore.Campo.Valore == campoDescrizioneSTEP &&
			strings.TrimSpace(u.Testo) != "" {
			v := u.Testo
			return &v
		}
	}
	return nil
}

// motiviDelNodo: i motivi del nodo, con il vocabolario dello smistamento (contratto §2.3: «ambiguità e conflitti dei file
// candidati del nodo»), nell'ordine dei valori:
//   - i file del nodo: quelli con un candidato che ha una posizione sul nodo nelle strutture del prodotto e, per il nodo
//     deciso, quelli associati al suo componente (il documento confermato o l'«assegna»);
//   - per ogni file del nodo che conta (dentro il perimetro di R93 b) e non è terminale, i motivi dei suoi due assi e
//     dell'«assegna» (motiviDelFile, come per lo smistamento del prodotto);
//   - per ogni conflitto dell'asse smistamento del prodotto su un file del nodo (un conflitto di associazione ha sempre il
//     file associato al componente), o sul componente del nodo deciso (l'oggetto di un nuovo_file, anche quando il file
//     nuovo non ha una posizione sul nodo), il motivo del suo tipo (motivoDelConflitto).
func (x *nodiThread) motiviDelNodo(pv ProdottoValutato, n ancoraggio.NodoProposto, k *uuid.UUID) []MotivoSmistamento {
	file := map[uuid.UUID]bool{}
	var ordine []uuid.UUID
	aggiungi := func(id uuid.UUID) {
		if !file[id] {
			file[id] = true
			ordine = append(ordine, id)
		}
	}
	for _, a := range x.ancoraggi {
		for _, c := range a.Candidati {
			for _, p := range c.Posizioni {
				if p.Target == pv.Rif && p.Nodo == n.Rif {
					aggiungi(a.AllegatoID)
				}
			}
		}
	}
	if k != nil {
		for _, a := range x.associazioni {
			if (a.Confermata != nil && *a.Confermata == *k) || (a.Manuale != nil && *a.Manuale == *k) {
				aggiungi(a.AllegatoID)
			}
		}
	}
	motivi := map[MotivoSmistamento]bool{}
	for _, id := range ordine {
		a := x.perAllegato[id]
		if a == nil || a.Perimetro != PerimetroDentro || a.Terminale {
			continue
		}
		for _, m := range motiviDelFile(FileDelloSmistamento{AllegatoID: id, Associazione: a.Associazione, Collocazione: a.Collocazione, Manuale: a.Manuale != nil}) {
			motivi[m] = true
		}
	}
	for _, c := range x.conflitti {
		if c.Prodotto != pv.Rif {
			continue
		}
		suFile := c.EvidenzaProposta.AllegatoID != nil && file[*c.EvidenzaProposta.AllegatoID]
		if suFile || (k != nil && c.Rif == ancoraggio.RifComponente(*k)) {
			motivi[motivoDelConflitto(c.Tipo)] = true
		}
	}
	var out []MotivoSmistamento
	for _, m := range ordineMotiviSmistamento {
		if motivi[m] {
			out = append(out, m)
		}
	}
	return out
}

// copiaIntero: un intero puntato che non condivide memoria con quello di partenza.
func copiaIntero(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
