package web

// La BOM visuale del Fascicolo (B8.7b): la BOM al centro della schermata, disegnata come un grafo di card.
//
// Ogni componente della working e' una card piena, con il tipo, la completezza (3D, 2D, DXF...) e quante
// cose restano da decidere. Le proposte degli STEP stanno nella gerarchia, sotto il componente a cui
// appartengono, con il bordo tratteggiato e il file che le propone; la quantita' sta sull'arco (×2); un
// componente con piu' padri si riconosce (↗ condiviso) e si disegna per intero una volta sola, le altre e'
// un rimando; un arco che lo STEP strutturale non contiene piu' e' rosso. Un prodotto senza STEP e' una card
// radice con la struttura da definire: lo STEP, quando arriva, disegna la sua proposta sotto quella card.
//
// Qui si decide soltanto CHE COSA disegnare, da dati gia' letti: nessuna query, e le prove sono L1.

import (
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// carta e' un nodo della BOM visuale: un componente della working, o un nodo proposto da uno STEP.
type carta struct {
	ID     string // l'id dell'elemento: nodo-<comp>[-<padre>] o proposta-<allegato>-<chiave>
	Comp   *db.Componente
	Prop   *db.ComponenteProposta
	File   string    // lo STEP della proposta, o dell'arco proposto
	STEP   uuid.UUID // l'allegato di quello STEP
	Radice bool
	Qta    int32 // la quantita' sull'arco che porta qui; 0 per una radice
	// ArcoProposto: l'arco verso questo componente, che c'e' gia' nella BOM, lo propone uno STEP (un
	// secondo padre, una quantita' nuova). La card e' un rimando, l'arco e' tratteggiato.
	ArcoProposto *db.RelazioneProposta
	// Rimozione: lo STEP strutturale letto per intero non contiene piu' questo arco.
	Rimozione *rimozione
	Padri     int  // quanti padri attivi ha il componente: con piu' di uno e' condiviso
	Ripetuto  bool // il suo sottoalbero e' gia' disegnato altrove: qui e' un rimando
	Giro      bool // l'arco chiude un ciclo: il congelamento lo rifiuta
	Scelta    bool // e' il nodo scelto
	Trovata   bool // corrisponde alla ricerca
	Figli     []*carta

	// quello che la card mostra, calcolato qui perche' il template resti una lettura
	Celle     []cella
	Step      *db.VStepProdotto
	NDoc      int    // documenti correnti del componente
	NDecidere int    // decisioni che lo riguardano (file, STEP strutturale, rimozioni sotto di lui)
	InArrivo  int    // per un nodo proposto: i file che lo aspettano per entrare nel fascicolo
	Struttura string // per un prodotto: a che punto e' la sua struttura
	Classe    string // ok | warn | urg | "" : il colore della card
}

// cartaVista e' una card con la schermata, per il template ricorsivo.
type cartaVista struct {
	D *fascicoloDati
	C *carta
}

// nasRigaVista e' una voce del NAS con la schermata.
type nasRigaVista struct {
	D *fascicoloDati
	V nasVoce
}

// Proposta dice se la card e' un nodo proposto (tratteggiato).
func (c *carta) Proposta() bool { return c.Prop != nil }

// Codice e' come si chiama la card.
func (c *carta) Codice() string {
	if c.Comp != nil {
		return c.Comp.Codice
	}
	if c.Prop != nil {
		return etichettaNodo(*c.Prop)
	}
	return ""
}

// NomeTipo e' il tipo con le parole della schermata; per una proposta, quello suggerito.
func (c *carta) NomeTipo() string {
	switch {
	case c.Comp != nil:
		return fascicolo.NomeTipo(c.Comp.Tipo)
	case c.Prop != nil && c.Prop.TipoProposto.Valid:
		return fascicolo.NomeTipo(c.Prop.TipoProposto.TipoComponente)
	}
	return ""
}

// Finito dice se la card e' un prodotto finito della working.
func (c *carta) Finito() bool { return c.Comp != nil && c.Comp.Tipo == db.TipoComponenteFinito }

type chiaveNodo struct {
	a uuid.UUID
	k string
}

// grafoProposte sono le proposte aperte degli STEP della RFQ, per file.
type grafoProposte struct {
	nodi      map[chiaveNodo]db.ComponenteProposta
	file      map[uuid.UUID]string
	figli     map[chiaveNodo][]db.RelazioneProposta // le relazioni aperte, per nodo padre
	padri     map[chiaveNodo]int                    // quante relazioni aperte arrivano al nodo
	sottoComp map[uuid.UUID][]db.RelazioneProposta  // le relazioni aperte il cui padre e' un componente della working
	ordine    []chiaveNodo                          // i nodi, nell'ordine dei file e delle chiavi
}

func nuovoGrafoProposte(nodi []db.ListComponenteProposteThreadRow, rel []db.ListRelazioneProposteThreadRow) grafoProposte {
	g := grafoProposte{nodi: map[chiaveNodo]db.ComponenteProposta{}, file: map[uuid.UUID]string{}, figli: map[chiaveNodo][]db.RelazioneProposta{},
		padri: map[chiaveNodo]int{}, sottoComp: map[uuid.UUID][]db.RelazioneProposta{}}
	for _, n := range nodi {
		k := chiaveNodo{n.ComponenteProposta.AllegatoID, n.ComponenteProposta.Chiave}
		g.nodi[k] = n.ComponenteProposta
		g.file[k.a] = n.NomeFile
		g.ordine = append(g.ordine, k)
	}
	for _, r := range rel {
		x := r.RelazioneProposta
		g.file[x.AllegatoID] = r.NomeFile
		if x.Stato != db.StatoPropostaAperta {
			continue
		}
		pk, fk := chiaveNodo{x.AllegatoID, x.PadreChiave}, chiaveNodo{x.AllegatoID, x.FiglioChiave}
		pn, fn := g.nodi[pk], g.nodi[fk]
		if pn.Stato == db.StatoPropostaScartata || fn.Stato == db.StatoPropostaScartata {
			continue // un arco verso un nodo scartato non si disegna: si decide nella vista tecnica
		}
		g.padri[fk]++
		if pn.Stato == db.StatoPropostaAperta {
			g.figli[pk] = append(g.figli[pk], x)
			continue
		}
		if pn.ComponenteID.Valid {
			g.sottoComp[pn.ComponenteID.UUID] = append(g.sottoComp[pn.ComponenteID.UUID], x)
		}
	}
	return g
}

// costruttoreBom mette insieme la BOM visuale da quello che la schermata ha gia' letto.
type costruttoreBom struct {
	d         *fascicoloDati
	g         grafoProposte
	disegnate map[chiaveNodo]bool
	decidere  map[uuid.UUID]int
	arrivo    map[uuid.UUID]int // proposta del nodo → file del piano che la aspettano
	padri     map[uuid.UUID]int // componente → quanti padri attivi
	cerca     string
}

// costruisciBom restituisce le radici della BOM visuale: i componenti radice della working, poi le radici
// proposte dagli STEP che non pendono da un componente. Le proposte che non si riescono a disegnare (sotto
// un nodo scartato, un giro nel file) restano nella vista tecnica «Albero».
func costruisciBom(d *fascicoloDati) []*carta {
	b := &costruttoreBom{d: d, g: nuovoGrafoProposte(d.NodiProposti, d.RelazioniProposte), disegnate: map[chiaveNodo]bool{},
		decidere: d.Piano.DaVerificare(), arrivo: map[uuid.UUID]int{}, padri: map[uuid.UUID]int{}, cerca: strings.ToUpper(strings.TrimSpace(d.Stato.Cerca))}
	for _, r := range d.Albero.Righe {
		b.padri[r.C.ComponenteID] = r.Padri
	}
	for _, v := range d.Piano.File {
		if v.DaStep != nil {
			b.arrivo[v.DaStep.Proposta]++
		}
	}
	for padre, rr := range d.Rimozioni {
		b.decidere[padre] += len(rr)
	}
	var out []*carta
	for _, n := range d.Albero.Radici {
		out = append(out, b.componente(n))
	}
	// le radici dei file: nodi aperti che nessuna relazione aperta mette sotto un altro nodo, e che nessun
	// componente ha gia' portato nella BOM visuale
	for _, k := range b.g.ordine {
		n := b.g.nodi[k]
		if n.Stato != db.StatoPropostaAperta || b.disegnate[k] || b.g.padri[k] > 0 {
			continue
		}
		c := b.proposta(k, 0, nil)
		c.Radice = true
		out = append(out, c)
	}
	return out
}

// componente e' la card di un componente della working, con i suoi figli: prima quelli confermati, poi
// quelli che gli STEP propongono.
func (b *costruttoreBom) componente(n *fascicolo.Nodo) *carta {
	id := n.C.ComponenteID
	c := &carta{Comp: &n.C, Radice: n.Radice(), Padri: n.Padri, Ripetuto: n.Ripetuto, Giro: n.Giro, ID: "nodo-" + id.String()}
	if !c.Radice {
		c.Qta = n.Qta
		c.ID += "-" + n.Padre.String()
		for i, r := range b.d.Rimozioni[n.Padre] {
			if r.R.FiglioID == id {
				c.Rimozione = &b.d.Rimozioni[n.Padre][i]
			}
		}
	}
	b.riempiComponente(c)
	if n.Ripetuto || n.Giro {
		return c
	}
	for _, f := range n.Figli {
		c.Figli = append(c.Figli, b.componente(f))
	}
	for _, r := range b.g.sottoComp[id] {
		r := r
		fk := chiaveNodo{r.AllegatoID, r.FiglioChiave}
		fn := b.g.nodi[fk]
		if fn.Stato == db.StatoPropostaAperta {
			c.Figli = append(c.Figli, b.proposta(fk, r.Qta, &r))
			continue
		}
		if fn.ComponenteID.Valid {
			if x, ok := b.d.Componenti[fn.ComponenteID.UUID]; ok && x.ArchiviatoIl == nil {
				rimando := &carta{Comp: &x, Qta: r.Qta, ArcoProposto: &r, File: b.g.file[r.AllegatoID], STEP: r.AllegatoID, Ripetuto: true,
					Padri: b.padri[x.ComponenteID], ID: "arco-" + r.AllegatoID.String() + "-" + r.PadreChiave + "-" + r.FiglioChiave}
				b.riempiComponente(rimando)
				c.Figli = append(c.Figli, rimando)
			}
		}
	}
	return c
}

// proposta e' la card di un nodo proposto, con il suo sottoalbero nel file.
func (b *costruttoreBom) proposta(k chiaveNodo, qta int32, arco *db.RelazioneProposta) *carta {
	n := b.g.nodi[k]
	c := &carta{Prop: &n, File: b.g.file[k.a], STEP: k.a, Qta: qta, ID: "proposta-" + k.a.String() + "-" + k.k}
	if arco != nil {
		c.ArcoProposto = arco
	}
	c.Scelta = b.d.Stato.Prop == n.PropostaID
	c.InArrivo = b.arrivo[n.PropostaID]
	c.Trovata = b.cerca != "" && strings.Contains(strings.ToUpper(c.Codice()), b.cerca)
	c.Classe = "proposta"
	if !n.Codice.Valid || strings.TrimSpace(n.Codice.String) == "" || n.Nota.Valid {
		c.Classe = "proposta decidere"
	}
	if b.disegnate[k] {
		c.Ripetuto = true
		return c
	}
	b.disegnate[k] = true
	for _, r := range b.g.figli[k] {
		r := r
		fk := chiaveNodo{r.AllegatoID, r.FiglioChiave}
		fn := b.g.nodi[fk]
		switch {
		case fn.Stato == db.StatoPropostaAperta:
			c.Figli = append(c.Figli, b.proposta(fk, r.Qta, &r))
		case fn.ComponenteID.Valid:
			if x, ok := b.d.Componenti[fn.ComponenteID.UUID]; ok && x.ArchiviatoIl == nil {
				rimando := &carta{Comp: &x, Qta: r.Qta, ArcoProposto: &r, File: c.File, STEP: k.a, Ripetuto: true,
					Padri: b.padri[x.ComponenteID], ID: "arco-" + r.AllegatoID.String() + "-" + r.PadreChiave + "-" + r.FiglioChiave}
				b.riempiComponente(rimando)
				c.Figli = append(c.Figli, rimando)
			}
		}
	}
	return c
}

// riempiComponente calcola quello che la card di un componente mostra.
func (b *costruttoreBom) riempiComponente(c *carta) {
	d := b.d
	id := c.Comp.ComponenteID
	c.Celle = d.Completezza[id]
	c.Step = d.Step(id)
	c.Scelta = d.Stato.Nodo == id
	c.Trovata = b.cerca != "" && strings.Contains(strings.ToUpper(c.Comp.Codice), b.cerca)
	for _, x := range d.DocumentiDi[id] {
		if !x.SostituitoDa.Valid {
			c.NDoc++
		}
	}
	c.NDecidere = b.decidere[id]
	urg, warn, tutti := false, c.NDecidere > 0, len(c.Celle) > 0
	for _, x := range c.Celle {
		switch x.Classe {
		case "urg":
			urg = true
			tutti = false
		case "warn":
			warn = true
			tutti = false
		case "ok", "info", "coda":
		default:
			tutti = false
		}
	}
	switch {
	case urg:
		c.Classe = "urg"
	case warn:
		c.Classe = "warn"
	case tutti:
		c.Classe = "ok"
	}
	if c.Finito() {
		c.Struttura = b.struttura(c)
	}
}

// struttura dice a che punto e' la struttura di un prodotto finito: senza STEP, in analisi, con le
// proposte dello STEP sotto la card, letta.
func (b *costruttoreBom) struttura(c *carta) string {
	proposte, file := 0, ""
	for _, r := range b.g.sottoComp[c.Comp.ComponenteID] {
		proposte++
		file = b.g.file[r.AllegatoID]
	}
	if proposte > 0 {
		return conta(proposte, "modifica proposta", "modifiche proposte") + " dallo STEP " + file
	}
	if c.Step == nil {
		return ""
	}
	switch c.Step.Esito {
	case fascicolo.StepMancante:
		if len(c.Figli) == 0 {
			return "struttura da definire: STEP non ancora disponibile"
		}
		return "STEP non ancora disponibile"
	case fascicolo.StepSulPortale:
		return "STEP sul portale del cliente: da scaricare"
	case fascicolo.StepDaConfermare:
		return "STEP arrivato: da confermare con il Fascicolo"
	case fascicolo.StepNonAnalizzato:
		return "STEP in analisi"
	case fascicolo.StepParziale:
		return "struttura dallo STEP, letta in parte"
	case fascicolo.StepDaScegliere, fascicolo.StepRiferimentoSuperato:
		return "STEP strutturale da scegliere"
	case fascicolo.StepSoloAltro3d:
		return "un 3D, ma non uno STEP"
	case fascicolo.StepAnalizzato:
		return "struttura dallo STEP"
	}
	return ""
}

// componentiInCard sono le card della linguetta «Componenti»: i componenti attivi, uno per card, in ordine di
// codice. Senza gerarchia: e' la vista per chi cerca un pezzo.
func componentiInCard(d *fascicoloDati) []*carta {
	b := &costruttoreBom{d: d, decidere: d.Piano.DaVerificare(), cerca: strings.ToUpper(strings.TrimSpace(d.Stato.Cerca))}
	for padre, rr := range d.Rimozioni {
		b.decidere[padre] += len(rr)
	}
	var out []*carta
	for _, r := range d.Albero.Righe {
		if r.Ripetuto {
			continue
		}
		c := r.C
		if b.cerca != "" && !strings.Contains(strings.ToUpper(c.Codice), b.cerca) {
			continue
		}
		k := &carta{Comp: &c, Padri: r.Padri, ID: "card-" + c.ComponenteID.String()}
		b.riempiComponente(k)
		out = append(out, k)
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToUpper(out[i].Comp.Codice) < strings.ToUpper(out[j].Comp.Codice) })
	return out
}
