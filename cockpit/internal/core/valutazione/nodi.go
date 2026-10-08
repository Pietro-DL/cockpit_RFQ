package valutazione

import (
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// nodi.go: il nodo della BOM come lo vedrà la UI (R91, T-B0-39; contratto §1.3, riga «BOM di lavoro proposta», §2.3, §2.5;
// T-E1-19; fase 0 di B6, F.2; T-B6-11). PV.Nodi sono solo i nodi della BOM di lavoro del prodotto, cioè della struttura
// sotto la radice scelta dello STEP confermato (R76 A); sui dati veri, senza STEP confermato, PV.Nodi è vuoto (T-B6-11). Il
// nodo unisce il NodoProposto di ancoraggio ai 2D, alle associazioni e alla classificazione, che vengono dalla fotografia.
//
// Con EB7-1 B (ratificata dall'utente l'08/10; bozza del contratto con il frontendista §1.3, §1.4) gli stessi nodi si
// danno anche, a parte, per le strutture candidate (PV.StruttureCandidate, una per allegato e radice) e per i componenti
// del perimetro confermato che nessun nodo della BOM di lavoro rappresenta (PV.ConfermatiSenzaSTEP). Con EB7-2 A ogni nodo
// porta il suo tipo con l'origine (NodoBOM.Tipo, OrigineTipo; bozza §1.6). Sono quattro cose da non confondere [U]:
// struttura candidata, fonte confermata, BOM di lavoro, BOM verificata. Esporre i nodi non cambia niente: stato, assi,
// motivi, autorità e impronta del prodotto si calcolano prima e senza questi campi.

// OrigineTipo: da dove viene NodoBOM.Tipo (EB7-2 A; bozza §1.6).
//   - confermato: il tipo del componente deciso del nodo (per la radice della BOM di lavoro senza una decisione propria,
//     il componente del target, T-B4-38; per un confermato senza STEP, il suo componente);
//   - proposto_dalla_riga: componente_proposta.tipo_proposto della riga legacy del nodo;
//   - proposto_dalla_regola: senza la riga, la regola con cui il legacy lo scrive (sottoassieme con dei figli nella
//     struttura, altrimenti sciolto).
//
// Le due proposte sono la stessa ricerca dei nodi previsti della completezza (bomDelThread.tipoProposto): una regola sola.
type OrigineTipo string

const (
	OrigineTipoConfermato          OrigineTipo = "confermato"
	OrigineTipoPropostoDallaRiga   OrigineTipo = "proposto_dalla_riga"
	OrigineTipoPropostoDallaRegola OrigineTipo = "proposto_dalla_regola"
)

// NodoBOM: un nodo della BOM di lavoro del prodotto, di una sua struttura candidata o, con il Rif «componente:<uuid>», un
// componente confermato senza STEP (EB7-1).
//   - Parentela: i padri e le quantità, dagli archi; per un confermato senza STEP dalle relazioni confermate.
//   - Disegni: per il nodo deciso il gruppo dei 2D del componente; per il nodo senza decisione i 2D candidati del nodo.
//   - Associazione: l'origine dell'associazione del 2D primario (confermato, manuale o proposto); nil = nessuna.
//   - Motivi: le ambiguità e i conflitti dei file candidati del nodo, con il vocabolario dello smistamento.
//   - Classificazione: l'esenzione dal 2D solo con EsenteDal2D (README, «la classificazione»). AncheIn: gli altri
//     prodotti con lo stesso componente deciso.
//   - Tipo, OrigineTipo (EB7-2 A): il tipo del nodo (finito, sottoassieme, sciolto, commerciale: l'enum tipo_componente)
//     e da dove viene (OrigineTipo*); tutti e due "" se il tipo non si determina. Il frontend legge il tipo da qui e non
//     lo deduce dai figli.
type NodoBOM struct {
	Nodo            ancoraggio.NodoProposto `json:"nodo"`
	Descrizione     *string                 `json:"descrizione,omitempty"`
	Parentela       []Parentela             `json:"parentela,omitempty"`
	Disegni         GruppoDisegni2D         `json:"disegni"`
	Associazione    *ancoraggio.OrigineDato `json:"associazione,omitempty"`
	Motivi          []MotivoSmistamento     `json:"motivi,omitempty"`
	Classificazione Classificazione         `json:"classificazione"`
	AncheIn         []string                `json:"anche_in,omitempty"`
	Tipo            string                  `json:"tipo"`
	OrigineTipo     OrigineTipo             `json:"origine_tipo"`
}

// Parentela: un padre del nodo (contratto §2.3): il Rif del nodo padre, la quantità, l'arco confermato accanto. Per un
// confermato senza STEP (EB7-1) la quantità è quella della relazione confermata, e Decisa è sempre vero.
type Parentela struct {
	Padre    string `json:"padre"`
	Quantita *int   `json:"quantita,omitempty"`
	Decisa   bool   `json:"decisa"`
}

// StrutturaCandidata: una struttura candidata del prodotto con i suoi nodi (EB7-1 B, ratificata dall'utente l'08/10;
// bozza del contratto con il frontendista §1.3). La sua chiave è la coppia (AllegatoID, Radice). Non è la BOM di lavoro
// (quella sta in ProdottoValutato.Nodi), e non promuove niente: nessuna conferma della fonte, nessuna BOM verificata.
//   - AllegatoID, Sha256, Radice, RadiceDelFile, Compatibilita, GrafoCompleto, MotivoGrafo: quelli della struttura di
//     ancoraggio (ancoraggio.StrutturaProdotto con lo stato struttura_candidata), com'è.
//   - Nodi: i nodi che la radice raggiunge senza gli archi tolti da una persona, uno per Rif, la radice per prima, come i
//     nodi della BOM di lavoro, con una differenza: la radice non prende il componente del target (T-B4-38 vale solo per
//     la radice scelta della BOM di lavoro). Il componente di un nodo viene solo dalla sua decisione.
type StrutturaCandidata struct {
	AllegatoID    uuid.UUID             `json:"allegato_id"`
	Sha256        string                `json:"sha256"`
	Radice        string                `json:"radice"`
	RadiceDelFile bool                  `json:"radice_del_file"`
	Compatibilita motorea.Compatibilita `json:"compatibilita"`
	GrafoCompleto bool                  `json:"grafo_completo"`
	MotivoGrafo   string                `json:"motivo_grafo,omitempty"`
	Nodi          []NodoBOM             `json:"nodi,omitempty"`
}

// campoDescrizioneSTEP: il campo dell'unità della descrizione di un PRODUCT (PRODUCT.description) nel documento di uno
// STEP, come lo scrive l'adattatore di A1b (mappatura-step-1); ripetuto qui perché estrazione non lo esporta.
const campoDescrizioneSTEP = "descrizione"

// nodiThread: ciò che serve ai nodi dei prodotti di un thread, calcolato una volta. Le mappe servono solo a cercare, mai a
// scorrere: si scorrono gli elenchi della fotografia e degli ancoraggi, già in ordine.
type nodiThread struct {
	b            *bomDelThread
	dis          *disegniThread
	col          *collegamento
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
	x := &nodiThread{b: b, dis: dis, col: col, gruppi: map[uuid.UUID]*GruppoDisegni2D{}, classi: map[uuid.UUID]Classificazione{},
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
// radice, poi gli altri nodi per Rif), con nodiDellaStruttura. Vuoto senza BOM di lavoro.
func (x *nodiThread) nodiDellaBOM(pv ProdottoValutato, strutture []ancoraggio.StrutturaProdotto) []NodoBOM {
	var out []NodoBOM
	visti := map[string]bool{}
	for _, st := range strutture {
		if st.Target != pv.Rif || st.Stato != ancoraggio.StatoBOMDiLavoroProposta {
			continue
		}
		out = append(out, x.nodiDellaStruttura(pv, st, visti)...)
	}
	return out
}

// struttureCandidate: le strutture candidate del prodotto con i loro nodi (EB7-1 B; bozza §1.3): una per ogni struttura di
// ancoraggio del prodotto con lo stato struttura_candidata, nello stesso ordine (quello di ancoraggio: sha256, allegato,
// radice). Due STEP candidati danno due elementi; la BOM di lavoro non c'è (i suoi nodi sono in PV.Nodi). I nodi di ogni
// struttura con nodiDellaStruttura, uno per Rif dentro la struttura. Niente qui tocca stato, assi o impronta del prodotto.
func (x *nodiThread) struttureCandidate(pv ProdottoValutato, strutture []ancoraggio.StrutturaProdotto) []StrutturaCandidata {
	var out []StrutturaCandidata
	for _, st := range strutture {
		if st.Target != pv.Rif || st.Stato != ancoraggio.StatoStrutturaCandidata {
			continue
		}
		out = append(out, StrutturaCandidata{AllegatoID: st.AllegatoID, Sha256: st.Sha256, Radice: st.Radice, RadiceDelFile: st.RadiceDelFile,
			Compatibilita: st.Compatibilita, GrafoCompleto: st.GrafoCompleto, MotivoGrafo: st.MotivoGrafo,
			Nodi: x.nodiDellaStruttura(pv, st, map[string]bool{})})
	}
	return out
}

// nodiDellaStruttura: i nodi di una struttura del prodotto, nell'ordine della struttura (la radice, poi gli altri per
// Rif). Solo i nodi che la radice raggiunge senza passare da un arco tolto da una persona (le righe di relazione_proposta
// tutte scartate da chi le ha decise: bomDelThread.archiTolti, la stessa raggiungibilità dei previsti della completezza,
// T-B5-67); un nodo scartato ma ancora raggiunto resta, con la sua riga decisa accanto. Un nodo per Rif (visti).
func (x *nodiThread) nodiDellaStruttura(pv ProdottoValutato, st ancoraggio.StrutturaProdotto, visti map[string]bool) []NodoBOM {
	var out []NodoBOM
	raggiunti, conFigli := x.raggiunti(st)
	for _, n := range st.Nodi {
		if !raggiunti[n.Rif] || visti[n.Rif] {
			continue
		}
		visti[n.Rif] = true
		out = append(out, x.nodo(pv, st, n, raggiunti, conFigli[n.Rif]))
	}
	return out
}

// componenteDelNodo: il componente di un nodo di una struttura del prodotto: la decisione per UUID
// (NodoProposto.Decisione); per la radice scelta della BOM di lavoro senza una decisione propria, il componente del target
// (T-B4-38, come la catena del codice e i 2D di B5). Sulla struttura candidata la radice non prende il componente del
// target (EB7-1; bozza §1.3): lì il componente viene solo dalla decisione. nil senza.
func componenteDelNodo(pv ProdottoValutato, st ancoraggio.StrutturaProdotto, n ancoraggio.NodoProposto) *uuid.UUID {
	switch {
	case n.Decisione != nil:
		id := n.Decisione.ComponenteID
		return &id
	case n.Rif == st.Radice && st.Stato == ancoraggio.StatoBOMDiLavoroProposta:
		return copiaUUID(pv.ComponenteID)
	}
	return nil
}

// raggiunti: i nodi della struttura che la radice raggiunge negli archi, senza gli archi tolti da una persona, e i nodi
// che in quegli archi hanno dei figli (per la regola del tipo proposto, EB7-2: «con figli» guarda gli archi della
// struttura del nodo che una persona non ha tolto).
func (x *nodiThread) raggiunti(st ancoraggio.StrutturaProdotto) (map[string]bool, map[string]bool) {
	figli := map[string][]string{}
	conFigli := map[string]bool{}
	for _, a := range st.Archi {
		if !x.b.archiTolti[RifArco(a.Padre, a.Figlio)] {
			figli[a.Padre] = append(figli[a.Padre], a.Figlio)
			conFigli[a.Padre] = true
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
	return out, conFigli
}

// nodo: il NodoBOM di un nodo di una struttura del prodotto (la BOM di lavoro o una candidata).
//   - Il componente del nodo: componenteDelNodo.
//   - Parentela: per ogni arco della struttura verso il nodo, da un padre raggiunto e non tolto da una persona, il padre,
//     la quantità dei fatti e se accanto c'è l'arco confermato (ArcoProposto.Decisione: T-14).
//   - Il resto: datiDelNodo.
func (x *nodiThread) nodo(pv ProdottoValutato, st ancoraggio.StrutturaProdotto, n ancoraggio.NodoProposto, raggiunti map[string]bool,
	conFigli bool) NodoBOM {
	out := NodoBOM{Nodo: n}
	for _, a := range st.Archi {
		if a.Figlio == n.Rif && raggiunti[a.Padre] && !x.b.archiTolti[RifArco(a.Padre, a.Figlio)] {
			out.Parentela = append(out.Parentela, Parentela{Padre: a.Padre, Quantita: copiaIntero(a.Quantita), Decisa: a.Decisione != nil})
		}
	}
	x.datiDelNodo(pv, &out, componenteDelNodo(pv, st, n), conFigli)
	return out
}

// datiDelNodo: ciò che un nodo prende dal suo componente k, o, senza, dal nodo stesso:
//   - Descrizione: quella del componente deciso, se c'è; altrimenti la descrizione del PRODUCT dello STEP del nodo
//     (PRODUCT.description nei fatti, com'è); nil senza.
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
//   - Tipo, OrigineTipo (EB7-2 A; bozza §1.6): per il nodo con il componente il tipo del componente, confermato (un
//     componente che la fotografia non ha, o senza tipo, non dà un tipo: ""); per il nodo senza componente il tipo
//     proposto della sua riga o della regola (bomDelThread.tipoProposto, con conFigli: il nodo ha dei figli negli archi
//     non tolti della sua struttura). Il tipo non cambia la classificazione: un commerciale resta un pezzo con il suo 2D.
func (x *nodiThread) datiDelNodo(pv ProdottoValutato, out *NodoBOM, k *uuid.UUID, conFigli bool) {
	n := out.Nodo
	out.Descrizione = x.descrizione(n, k)
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
		if c := x.componenti[*k]; c != nil && c.Tipo != "" {
			out.Tipo, out.OrigineTipo = c.Tipo, OrigineTipoConfermato
		}
	} else {
		if g := x.dis.gruppoDelNodo(pv.Rif, n); g != nil {
			out.Disegni = *g
		}
		var categorie []string
		if f := n.Codice.Forma; f != nil {
			categorie = categorieDi(f.Categorie)
		}
		out.Classificazione = Classifica(ComponenteDaClassificare{Categorie: categorie}, nil)
		out.Tipo, out.OrigineTipo = x.b.tipoProposto(n.Rif, conFigli)
	}
	if p := out.Disegni.Primario; p != nil {
		o := p.Provenienza
		out.Associazione = &o
	}
	out.Motivi = x.motiviDelNodo(pv, n, k)
}

// confermatiSenzaSTEP: i componenti del perimetro confermato del prodotto (il componente del target e i componenti attivi
// raggiungibili per le relazioni confermate: bomDelThread.perimetro, lo stesso dell'impronta e di AncheIn) che nessun nodo
// della BOM di lavoro rappresenta (EB7-1 B; bozza §1.4): una BOM costruita a mano, o confermata senza STEP, e i componenti
// aggiunti accanto a una BOM di lavoro. Senza BOM di lavoro ci sono tutti, il componente del target compreso. Un nodo di
// PV.Nodi rappresenta il suo componente (componenteDelNodo sulla BOM di lavoro). In ordine: il componente del target, poi
// per ComponenteID. Vuoto per un target senza componente.
//
// Ogni elemento è un NodoBOM senza file:
//   - Nodo: Rif «componente:<uuid>», AllegatoID nullo, EntitaID "", SenzaFile, la catena del codice vuota (il codice si
//     legge da Decisione: non c'è un file che lo porti); Decisione è il componente confermato com'è nel contesto di
//     ancoraggio (collegamento.componenteDeciso: la stessa costruzione di contestoStrutturale, con l'autorità e l'origine
//     di lì). Un componente che il contesto non ha (archiviato, o fuori dalla fotografia: con le relazioni attive non
//     capita) resta senza Decisione: il valore non si costruisce. Padri, Leggibile e le righe legacy restano vuoti.
//   - Parentela: le relazioni confermate del perimetro verso il componente, con la quantità confermata e Decisa vera; il
//     Padre è il Rif con cui il padre compare nella pagina: il primo nodo di PV.Nodi che lo rappresenta, altrimenti
//     «componente:<uuid>». In ordine di Padre.
//   - Il resto come per un nodo deciso (datiDelNodo con il componente): il tipo è quello del componente, confermato.
func (x *nodiThread) confermatiSenzaSTEP(pv ProdottoValutato, strutture []ancoraggio.StrutturaProdotto) []NodoBOM {
	if pv.ComponenteID == nil {
		return nil
	}
	rappresentati := map[uuid.UUID]string{}
	for _, st := range strutture {
		if st.Target != pv.Rif || st.Stato != ancoraggio.StatoBOMDiLavoroProposta {
			continue
		}
		for _, nb := range pv.Nodi {
			if k := componenteDelNodo(pv, st, nb.Nodo); k != nil {
				if _, ok := rappresentati[*k]; !ok {
					rappresentati[*k] = nb.Nodo.Rif
				}
			}
		}
	}
	dentro, relazioni := x.b.perimetro(pv.ComponenteID)
	var ids []uuid.UUID
	for k := range dentro {
		if _, ok := rappresentati[k]; !ok {
			ids = append(ids, k)
		}
	}
	radice := *pv.ComponenteID
	sort.Slice(ids, func(i, j int) bool {
		if (ids[i] == radice) != (ids[j] == radice) {
			return ids[i] == radice
		}
		return ids[i].String() < ids[j].String()
	})
	var out []NodoBOM
	for _, k := range ids {
		n := ancoraggio.NodoProposto{Rif: ancoraggio.RifComponente(k), SenzaFile: true}
		if c := x.componenti[k]; c != nil && c.ArchiviatoIl == nil {
			d := x.col.componenteDeciso(*c)
			n.Decisione = &d
		}
		nb := NodoBOM{Nodo: n}
		for _, r := range relazioni {
			if r.FiglioID != k {
				continue
			}
			padre, ok := rappresentati[r.PadreID]
			if !ok {
				padre = ancoraggio.RifComponente(r.PadreID)
			}
			q := r.Qta
			nb.Parentela = append(nb.Parentela, Parentela{Padre: padre, Quantita: &q, Decisa: true})
		}
		sort.SliceStable(nb.Parentela, func(i, j int) bool { return nb.Parentela[i].Padre < nb.Parentela[j].Padre })
		id := k
		x.datiDelNodo(pv, &nb, &id, false)
		out = append(out, nb)
	}
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
