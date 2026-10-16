package motorea

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// Il contesto di lavorazione (GP-1, addendum §1): per una RFQ o un workspace e per un insieme di file dichiara
// quali partner sono attivi, come sono codificati i campi e quali modelli di accoppiamento valgono. È l'unico
// modo di attivare partner, forme contestuali e legami: senza contesto il motore legge come prima. La
// partnership non si eredita da tutte le RFQ del cliente, e un file con lo stesso sha ma un altro nome, o in
// un'altra RFQ, non eredita niente (direttiva 21 §3.1, §6, §9).

// VersioneContesto: la versione del contratto del contesto.
const VersioneContesto = 1

// Stati del contesto: solo «dichiarato» attiva.
const (
	StatoContestoDichiarato = "dichiarato"
	StatoContestoProposto   = "proposto"
	StatoContestoRevocato   = "revocato"
)

// ContestoLavorazione: il file del contesto, come lo scrive il banco o la QA (e in futuro il workspace).
type ContestoLavorazione struct {
	VersioneContesto int                  `json:"versione_contesto"`
	ID               string               `json:"id"`
	RFQ              string               `json:"rfq"`
	Workspace        string               `json:"workspace,omitempty"`
	ClienteID        uuid.UUID            `json:"cliente_id"`
	Stato            string               `json:"stato"`
	Provenienza      ProvenienzaContesto  `json:"provenienza"`
	Partner          []PartnerContesto    `json:"partner,omitempty"`
	Perimetro        []OccorrenzaContesto `json:"perimetro"`
	Codifiche        []CodificaContesto   `json:"codifiche,omitempty"`
	Modelli          []string             `json:"modelli,omitempty"`
	EreditaDa        string               `json:"eredita_da,omitempty"`
}

// ProvenienzaContesto: chi ha dichiarato il contesto, quando, perché.
type ProvenienzaContesto struct {
	Autore string `json:"autore"` // operatore | banco | qa
	Data   string `json:"data"`
	Nota   string `json:"nota,omitempty"`
}

// PartnerContesto: un emittente attivato, con la versione che il contesto si aspetta.
type PartnerContesto struct {
	Emittente    string `json:"emittente"`
	HashSnapshot string `json:"hash_snapshot"`
}

// OccorrenzaContesto: un'occorrenza di file. Coincide con un documento se coincide l'allegato, oppure lo sha256
// E il nome ricevuto (e il contenitore, se scritto): mai per sola impronta.
type OccorrenzaContesto struct {
	Sha256      string `json:"sha256,omitempty"`
	Nome        string `json:"nome,omitempty"`
	Contenitore string `json:"contenitore,omitempty"`
	AllegatoID  string `json:"allegato_id,omitempty"`
	// Autore: chi ha fatto il documento (per esempio il fornitore che ha disegnato il CAD), dichiarato; distinto da chi
	// ha emesso i codici che ci sono scritti dentro (direttiva 21 §3.1). Facoltativo.
	Autore string `json:"autore,omitempty"`
}

// CodificaContesto: chi codifica un campo, su tutto il perimetro o su alcune occorrenze (indici nel perimetro).
type CodificaContesto struct {
	Selettore string `json:"selettore"`
	Emittente string `json:"emittente"` // cliente | <emittente> | misto
	Perimetro []int  `json:"perimetro,omitempty"`
}

// ContestoRisolto: il contesto controllato contro il motore di un cliente, pronto per Interpreta.
type ContestoRisolto struct {
	ID       string
	Impronta string // sha256 del canonico del contesto (e di quello ereditato): entra nell'identità delle interpretazioni toccate
	attivo   bool

	partner   map[string]bool // attivi e caricati
	mancanti  []string        // dichiarati nel contesto e non caricabili (in ordine)
	perimetro []OccorrenzaContesto
	codifiche []CodificaContesto
	modelli   map[string]bool
	modelliNo []string // ammessi dal contesto e assenti dal motore
}

// LeggiContesto: la decodifica stretta del file (chiavi sconosciute e valori dopo l'oggetto rifiutati).
func LeggiContesto(raw []byte) (ContestoLavorazione, error) {
	var c ContestoLavorazione
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return ContestoLavorazione{}, erroreContesto("", "il file del contesto non si legge: "+err.Error())
	}
	if dec.More() {
		return ContestoLavorazione{}, erroreContesto("", "testo dopo l'oggetto del contesto")
	}
	return c, nil
}

func erroreContesto(percorso, msg string) error {
	return &evidenze.ErroreContratto{Diagnostiche: []evidenze.Diagnostica{{
		Codice:    CodiceContestoNonValido,
		Gravita:   evidenze.GravitaErrore,
		Natura:    evidenze.NaturaContratto,
		Percorso:  percorso,
		Messaggio: msg,
	}}}
}

// RisolviContesto controlla il contesto contro il motore del cliente (e il contesto da cui eredita, se c'è) e
// lo prepara. Errore di contratto: versione, ID, cliente, stato, perimetro, codifiche o eredità non validi; il
// contesto non si applica. Avvisi (nelle diagnostiche, e il contesto si applica): un partner non caricabile, un
// modello assente; i documenti del perimetro avranno l'interpretazione parziale.
func RisolviContesto(m *Motore, c ContestoLavorazione, padre *ContestoLavorazione) (*ContestoRisolto, []evidenze.Diagnostica, error) {
	if m == nil {
		return nil, nil, fmt.Errorf("motorea: RisolviContesto su un motore nullo")
	}
	var errs []evidenze.Diagnostica
	errore := func(p, msg string) {
		errs = append(errs, evidenze.Diagnostica{Codice: CodiceContestoNonValido, Gravita: evidenze.GravitaErrore, Natura: evidenze.NaturaContratto, Percorso: p, Messaggio: msg})
	}
	if c.VersioneContesto != VersioneContesto {
		errore("versione_contesto", fmt.Sprintf("versione %d: questo codice legge solo la %d", c.VersioneContesto, VersioneContesto))
	}
	if strings.TrimSpace(c.ID) == "" {
		errore("id", "manca l'ID del contesto")
	}
	if strings.TrimSpace(c.RFQ) == "" {
		errore("rfq", "manca il riferimento della RFQ o del thread")
	}
	if c.ClienteID != m.snap.ClienteID {
		errore("cliente_id", "il contesto è di un altro cliente: non si applica")
	}
	if c.Stato != StatoContestoDichiarato && c.Stato != StatoContestoProposto && c.Stato != StatoContestoRevocato {
		errore("stato", fmt.Sprintf("stato %q fuori elenco (dichiarato, proposto, revocato)", c.Stato))
	}
	if strings.TrimSpace(c.Provenienza.Autore) == "" || strings.TrimSpace(c.Provenienza.Data) == "" {
		errore("provenienza", "mancano l'autore o la data della dichiarazione")
	}
	if c.EreditaDa != "" {
		switch {
		case padre == nil:
			errore("eredita_da", fmt.Sprintf("il contesto eredita da %q, che non è stato fornito: nessuna eredità implicita", c.EreditaDa))
		case padre.ID != c.EreditaDa:
			errore("eredita_da", fmt.Sprintf("il contesto fornito è %q, non %q", padre.ID, c.EreditaDa))
		case padre.EreditaDa != "":
			errore("eredita_da", "l'eredità ha profondità 1")
		case padre.ClienteID != c.ClienteID:
			errore("eredita_da", "il contesto ereditato è di un altro cliente")
		}
	} else if padre != nil {
		errore("eredita_da", "è stato fornito un contesto padre, ma il contesto non dichiara eredita_da")
	}
	if len(errs) > 0 {
		return nil, nil, &evidenze.ErroreContratto{Diagnostiche: errs}
	}

	// L'eredità esplicita: il contesto vale con le dichiarazioni del padre, a cui aggiunge le sue.
	eff := c
	if padre != nil {
		eff.Partner = append(append([]PartnerContesto(nil), padre.Partner...), c.Partner...)
		off := len(padre.Perimetro)
		eff.Perimetro = append(append([]OccorrenzaContesto(nil), padre.Perimetro...), c.Perimetro...)
		eff.Codifiche = append([]CodificaContesto(nil), padre.Codifiche...)
		for _, k := range c.Codifiche {
			kk := k
			kk.Perimetro = nil
			for _, i := range k.Perimetro {
				kk.Perimetro = append(kk.Perimetro, i+off)
			}
			eff.Codifiche = append(eff.Codifiche, kk)
		}
		eff.Modelli = append(append([]string(nil), padre.Modelli...), c.Modelli...)
	}

	r := &ContestoRisolto{
		ID:        c.ID,
		attivo:    c.Stato == StatoContestoDichiarato,
		partner:   map[string]bool{},
		perimetro: eff.Perimetro,
		modelli:   map[string]bool{},
	}
	if len(eff.Perimetro) == 0 {
		errore("perimetro", "il perimetro è vuoto: un contesto vale per un insieme di file dichiarato")
	}
	for i, o := range eff.Perimetro {
		if o.AllegatoID == "" && (o.Sha256 == "" || o.Nome == "") {
			errore(fmt.Sprintf("perimetro[%d]", i), "un'occorrenza ha l'allegato, oppure lo sha256 e il nome ricevuto: mai la sola impronta")
		}
	}
	var diag []evidenze.Diagnostica
	visti := map[string]bool{}
	nomiPartner := map[string]bool{}
	for i, p := range eff.Partner {
		pp := fmt.Sprintf("partner[%d]", i)
		if p.Emittente == "" || visti[p.Emittente] {
			errore(pp, "emittente vuoto o ripetuto")
			continue
		}
		visti[p.Emittente] = true
		nomiPartner[p.Emittente] = true
		d, dichiarato := m.partnerDichiarato(p.Emittente)
		pc, caricato := m.partner[p.Emittente]
		motivo := ""
		switch {
		case !dichiarato:
			motivo = "il cliente non lo dichiara fra i partner ammissibili"
		case !caricato:
			motivo = "dichiarato dal cliente ma non caricato nel motore (assente, in deriva o in conflitto: vedi la compilazione)"
		case !strings.EqualFold(p.HashSnapshot, pc.hash) || !strings.EqualFold(d.HashSnapshot, pc.hash):
			motivo = fmt.Sprintf("il contesto attende l'hash %s, il motore ha %s", p.HashSnapshot, pc.hash)
		}
		if motivo != "" {
			r.mancanti = append(r.mancanti, p.Emittente)
			diag = append(diag, evidenze.Diagnostica{Codice: CodiceContestoPartnerNonCaricabile, Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaDati,
				Percorso: pp, Messaggio: "partner " + p.Emittente + " non caricabile: " + motivo + "; i documenti del perimetro avranno l'interpretazione parziale", Rif: []string{p.Emittente}})
			continue
		}
		r.partner[p.Emittente] = true
	}
	for i, k := range eff.Codifiche {
		pk := fmt.Sprintf("codifiche[%d]", i)
		if _, err := evidenze.LeggiSelettore(k.Selettore); err != nil {
			errore(pk+".selettore", fmt.Sprintf("selettore %q non valido", k.Selettore))
		}
		if k.Emittente != grammatica.CodificaCliente && k.Emittente != grammatica.CodificaMista && !nomiPartner[k.Emittente] {
			errore(pk+".emittente", fmt.Sprintf("codifica %q: vale cliente, misto o un partner del contesto", k.Emittente))
		}
		for _, j := range k.Perimetro {
			if j < 0 || j >= len(eff.Perimetro) {
				errore(pk+".perimetro", fmt.Sprintf("l'indice %d non è nel perimetro", j))
			}
		}
	}
	r.codifiche = eff.Codifiche
	for _, id := range eff.Modelli {
		if m.legame(id) == nil {
			r.modelliNo = append(r.modelliNo, id)
			diag = append(diag, evidenze.Diagnostica{Codice: CodiceContestoModelloNonDisponibile, Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaDati,
				Percorso: "modelli", Messaggio: "il legame " + id + " non è nel motore del cliente (non dichiarato, riservato o non composto): i documenti del perimetro avranno l'interpretazione parziale", Rif: []string{id}})
			continue
		}
		r.modelli[id] = true
	}
	if len(errs) > 0 {
		return nil, nil, &evidenze.ErroreContratto{Diagnostiche: errs}
	}
	if !r.attivo {
		diag = append(diag, evidenze.Diagnostica{Codice: CodiceContestoNonAttivo, Gravita: evidenze.GravitaNota, Natura: evidenze.NaturaDati,
			Percorso: "stato", Messaggio: "contesto " + c.Stato + ": si mostra e non attiva niente"})
	}
	// L'impronta non dipende dall'ordine degli elenchi (direttiva 21 §9: determinismo variando l'ordine di file ed
	// emittenti): si calcola sulla forma normalizzata del contesto e del padre.
	var np *ContestoLavorazione
	if padre != nil {
		x := normalizzaContesto(*padre)
		np = &x
	}
	imp, err := jsoncanonico.ImprontaDi(struct {
		Contesto ContestoLavorazione  `json:"contesto"`
		Padre    *ContestoLavorazione `json:"padre,omitempty"`
	}{normalizzaContesto(c), np})
	if err != nil {
		return nil, nil, fmt.Errorf("motorea: impronta del contesto: %w", err)
	}
	r.Impronta = imp
	sort.Strings(r.mancanti)
	return r, diag, nil
}

// normalizzaContesto: la forma canonica del contesto per l'impronta. Partner e modelli in ordine; il perimetro in
// ordine di (allegato, sha256, nome, contenitore, autore), con gli indici delle codifiche riscritti; le codifiche in
// ordine di (selettore, emittente, perimetro).
func normalizzaContesto(c ContestoLavorazione) ContestoLavorazione {
	n := c
	n.Partner = append([]PartnerContesto(nil), c.Partner...)
	sort.SliceStable(n.Partner, func(i, j int) bool { return n.Partner[i].Emittente < n.Partner[j].Emittente })
	n.Modelli = append([]string(nil), c.Modelli...)
	sort.Strings(n.Modelli)
	type pos struct {
		o OccorrenzaContesto
		i int
	}
	ps := make([]pos, len(c.Perimetro))
	for i, o := range c.Perimetro {
		ps[i] = pos{o, i}
	}
	chiave := func(o OccorrenzaContesto) string {
		return o.AllegatoID + "\x00" + strings.ToLower(o.Sha256) + "\x00" + o.Nome + "\x00" + o.Contenitore + "\x00" + o.Autore
	}
	sort.SliceStable(ps, func(i, j int) bool { return chiave(ps[i].o) < chiave(ps[j].o) })
	nuovo := make(map[int]int, len(ps))
	n.Perimetro = nil
	for k, p := range ps {
		nuovo[p.i] = k
		n.Perimetro = append(n.Perimetro, p.o)
	}
	n.Codifiche = nil
	for _, k := range c.Codifiche {
		kk := k
		kk.Perimetro = nil
		for _, i := range k.Perimetro {
			kk.Perimetro = append(kk.Perimetro, nuovo[i])
		}
		sort.Ints(kk.Perimetro)
		n.Codifiche = append(n.Codifiche, kk)
	}
	sort.SliceStable(n.Codifiche, func(i, j int) bool {
		a, b := n.Codifiche[i], n.Codifiche[j]
		if a.Selettore != b.Selettore {
			return a.Selettore < b.Selettore
		}
		if a.Emittente != b.Emittente {
			return a.Emittente < b.Emittente
		}
		return fmt.Sprint(a.Perimetro) < fmt.Sprint(b.Perimetro)
	})
	return n
}

// ConContesto: lo stesso motore, con il contesto risolto per un'RFQ. I piani e le regole sono gli stessi
// (condivisi, immutabili); cambia solo che cosa accende. nil toglie il contesto.
func (m *Motore) ConContesto(c *ContestoRisolto) *Motore {
	if m == nil {
		return nil
	}
	x := *m
	x.contesto = c
	return &x
}

// Contesto: il contesto risolto del motore, nil senza.
func (m *Motore) Contesto() *ContestoRisolto { return m.contesto }

// occorrenzeDi: gli indici del perimetro che coincidono con la fonte del documento.
func (c *ContestoRisolto) occorrenzeDi(f evidenze.Fonte) []int {
	var out []int
	nome := f.Provenienza.NomeRicevuto
	base := path.Base(strings.ReplaceAll(f.Provenienza.PercorsoRicevuto, "\\", "/"))
	for i, o := range c.perimetro {
		if o.AllegatoID != "" {
			if f.OrigineID != uuid.Nil && strings.EqualFold(o.AllegatoID, f.OrigineID.String()) {
				out = append(out, i)
			}
			continue
		}
		if !strings.EqualFold(o.Sha256, f.RiferimentoFatti.Sha256) {
			continue
		}
		if o.Nome != nome && (f.Provenienza.PercorsoRicevuto == "" || o.Nome != base) {
			continue
		}
		if o.Contenitore != "" && o.Contenitore != f.ContenitoreID {
			continue
		}
		out = append(out, i)
	}
	return out
}

// attivazionePer: l'attivazione del contesto per un documento; nil se il contesto non è attivo o il documento
// non è nel perimetro. Due codifiche diverse dello stesso campo sulla stessa occorrenza valgono «misto».
func (c *ContestoRisolto) attivazionePer(doc evidenze.DocumentoEvidenze) *attivazione {
	if c == nil || !c.attivo || doc.Fonte.Tipo != "allegato" {
		return nil
	}
	occ := c.occorrenzeDi(doc.Fonte)
	if len(occ) == 0 {
		return nil
	}
	a := &attivazione{partner: map[string]bool{}, codifiche: map[string]string{}, modelli: map[string]bool{}}
	for p := range c.partner {
		a.partner[p] = true
	}
	for id := range c.modelli {
		a.modelli[id] = true
	}
	for _, o := range occ {
		if au := c.perimetro[o].Autore; au != "" {
			if a.autore != "" && a.autore != au {
				a.autore = grammatica.CodificaMista // due dichiarazioni diverse della stessa occorrenza: non si sceglie
				break
			}
			a.autore = au
		}
	}
	for _, k := range c.codifiche {
		vale := len(k.Perimetro) == 0
		for _, j := range k.Perimetro {
			for _, o := range occ {
				vale = vale || j == o
			}
		}
		if !vale {
			continue
		}
		if prima, ok := a.codifiche[k.Selettore]; ok && prima != k.Emittente {
			a.codifiche[k.Selettore] = grammatica.CodificaMista
			continue
		}
		a.codifiche[k.Selettore] = k.Emittente
	}
	return a
}

// incompleto: il contesto attiva partner o modelli che il motore non ha. Un documento del perimetro non può
// avere un esito completo: si direbbe completo un risultato a cui manca una grammatica dichiarata.
func (c *ContestoRisolto) incompleto() (bool, []string) {
	if c == nil || (len(c.mancanti) == 0 && len(c.modelliNo) == 0) {
		return false, nil
	}
	return true, append(append([]string(nil), c.mancanti...), c.modelliNo...)
}

// attivazioneDiEsempio: l'attivazione di un esempio (grammatica.AttivazioneEsempio) sul suo selettore.
func attivazioneDiEsempio(a grammatica.AttivazioneEsempio, sel evidenze.Selettore) *attivazione {
	out := &attivazione{partner: map[string]bool{}, codifiche: map[string]string{}, modelli: map[string]bool{}}
	for _, p := range a.Partner {
		out.partner[p] = true
	}
	for _, id := range a.Modelli {
		out.modelli[id] = true
	}
	if a.Codifica != "" {
		out.codifiche[sel.String()] = a.Codifica
	}
	return out
}
