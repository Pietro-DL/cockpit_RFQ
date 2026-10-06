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

// L'adattatore della completezza (B5, fase 3; contratto §1.6; T-B0-22, T-12; R62 d C, R62 e A; R82; T-B5-90…93): dalla
// fotografia, dalla struttura della verifica della BOM (fase 1) e dai gruppi dei 2D (fase 2), l'ingresso astratto della
// regola (IngressoCompletezza) per ogni prodotto target, poi Completezza. Qui si sceglie solo che cosa della fotografia
// entra; le regole sono in completezza.go e classificazione.go.

// sezioniDellaCompletezza: le sezioni della fotografia da cui la completezza dipende (T-12): senza una di loro non si
// calcola (non_determinabile).
var sezioniDellaCompletezza = []string{fotorfq.SezioneComponenti, fotorfq.SezioneRelazioni, fotorfq.SezioneDocumenti,
	fotorfq.SezioneFascicolo, fotorfq.SezioneFabbisogni, fotorfq.SezioneDeroghe, fotorfq.SezioneRimozioniAperte,
	fotorfq.SezioneRigheComponenteProposta, fotorfq.SezioneRigheRelazioneProposta, fotorfq.SezioneProposteDocumento}

// MotivoPerimetroNonDeterminabile: il perimetro non si calcola, per una sezione assente (T-12).
const MotivoPerimetroNonDeterminabile = "non_determinabile"

// I valori del DB che la completezza guarda, ripetuti qui perché i pacchetti che li dichiarano non li esportano: il tipo
// documentale da_determinare (0008, LD-17) e gli stati del NAS che la vista distingue (0018).
const (
	tipoDocumentoDaDeterminare = "da_determinare"
	statoNasInCoda             = "in_coda"
	statoNasErrore             = "errore"
)

// ComponenteClassificato: la classificazione di un componente attivo del thread (tipo della fase 3, dubbio T-B5-51): B6
// la porta in NodoBOM.Classificazione.
type ComponenteClassificato struct {
	ComponenteID    uuid.UUID       `json:"componente_id"`
	Classificazione Classificazione `json:"classificazione"`
}

// completezzaThread: ciò che serve alla completezza dei prodotti di un thread, calcolato una volta. Le mappe servono solo
// a cercare, mai a scorrere.
type completezzaThread struct {
	t          fotorfq.Thread
	m          *motorea.Motore
	sezioni    map[string]fotorfq.StatoSezione
	b          *bomDelThread
	dis        *disegniThread
	gruppi     map[uuid.UUID]*GruppoDisegni2D
	componenti map[uuid.UUID]*fotorfq.Componente     // i componenti attivi
	vista      map[uuid.UUID][]fotorfq.RigaFascicolo // le righe di v_fascicolo per componente, in ordine di tipo di documento
	proposte   []fotorfq.PropostaAttuale             // in ordine di ID
	scartati   []uuid.UUID                           // gli allegati con la proposta scartata da una persona, in ordine
	tipi       map[string]string                     // il tipo proposto di ogni nodo (RifNodo), dalle righe legacy
	categorie  map[uuid.UUID][]string
	prodotti   map[uuid.UUID]bool // i componenti dei prodotti target
}

func nuovaCompletezzaThread(f fotorfq.Fotografia, t fotorfq.Thread, m *motorea.Motore, b *bomDelThread, dis *disegniThread,
	disegni []DisegniDelComponente, prodotti []ProdottoValutato) *completezzaThread {
	c := &completezzaThread{t: t, m: m, sezioni: f.Sezioni, b: b, dis: dis, gruppi: map[uuid.UUID]*GruppoDisegni2D{},
		componenti: map[uuid.UUID]*fotorfq.Componente{}, vista: map[uuid.UUID][]fotorfq.RigaFascicolo{}, tipi: map[string]string{},
		categorie: map[uuid.UUID][]string{}, prodotti: map[uuid.UUID]bool{}}
	for _, pv := range prodotti {
		if pv.ComponenteID != nil {
			c.prodotti[*pv.ComponenteID] = true
		}
	}
	for i := range disegni {
		c.gruppi[disegni[i].ComponenteID] = &disegni[i].Gruppo
	}
	for i := range t.Componenti {
		if t.Componenti[i].ArchiviatoIl == nil {
			c.componenti[t.Componenti[i].ID] = &t.Componenti[i]
		}
	}
	for _, r := range t.Fascicolo {
		c.vista[r.ComponenteID] = append(c.vista[r.ComponenteID], r)
	}
	for k := range c.vista {
		sort.SliceStable(c.vista[k], func(i, j int) bool { return c.vista[k][i].TipoDocumento < c.vista[k][j].TipoDocumento })
	}
	c.proposte = append([]fotorfq.PropostaAttuale(nil), t.Proposte...)
	sort.SliceStable(c.proposte, func(i, j int) bool { return c.proposte[i].ID.String() < c.proposte[j].ID.String() })
	for _, p := range c.proposte {
		if p.Stato == statoPropostaScartata && p.DecisoDa != nil {
			c.scartati = append(c.scartati, p.AllegatoID)
		}
	}
	righe := append([]fotorfq.RigaComponenteProposta(nil), t.RigheComponenteProposta...)
	sort.SliceStable(righe, func(i, j int) bool { return righe[i].ID.String() < righe[j].ID.String() })
	for _, r := range righe {
		k := ancoraggio.RifNodo(r.Sha256, r.Chiave)
		if _, ok := c.tipi[k]; !ok && r.TipoProposto != nil && strings.TrimSpace(*r.TipoProposto) != "" && r.Sha256 != "" && r.Chiave != "" {
			c.tipi[k] = strings.TrimSpace(*r.TipoProposto)
		}
	}
	return c
}

// nonDeterminabile: una sezione della completezza è assente (T-12).
func (c *completezzaThread) nonDeterminabile() bool {
	for _, k := range sezioniDellaCompletezza {
		if sezioneAssente(c.sezioni, k) {
			return true
		}
	}
	return false
}

// completezzaDelProdotto: la completezza di un prodotto target (contratto §1.6), con documenti.deroga_non_sostituisce_2d
// per ogni voce del 2D con una deroga. L'ingresso della regola:
//   - la struttura della verifica della BOM del prodotto (fase 1), per il perimetro (T-B5-90, T-B5-92);
//   - le righe certe: il componente del prodotto e i componenti attivi raggiungibili da lui per le relazioni confermate,
//     di qualunque origine (bomDelThread.perimetro: R62 e A, T-B5-91), ognuna con i fabbisogni del suo tipo dalla vista
//     (R62 d C), o dalle regole effettive senza righe nella vista, e con il suo gruppo dei 2D; una riga che il prodotto
//     raggiunge solo attraverso archi con una rimozione aperta ha RimozioneAperta (dubbio T-B5-56, deciso dall'orchestratore
//     [T] con la raggiungibilità, R-22: un figlio di una rimozione aperta raggiungibile anche per un'altra via resta fra
//     le voci certe, perché non può uscire dal perimetro, e una sua mancanza certa non si perde; il perimetro resta
//     comunque aperto, con rimozioni_aperte);
//   - i nodi da prevedere delle strutture che contano (struttureDellaVerifica: la BOM di lavoro, se c'è, altrimenti le
//     strutture candidate del prodotto), esclusa la radice, che è il prodotto (nodiDaPrevedere);
//   - le conferme della categoria (in A1c nessuna: LD-19).
//
// Con una sezione assente la completezza non si calcola: non_calcolabile, non_determinabile (T-12).
func (c *completezzaThread) completezzaDelProdotto(pv ProdottoValutato, s StrutturaDaVerificare, strutture []ancoraggio.StrutturaProdotto,
	conferme []ConfermaCategoria) (CompletezzaDocumentale, []evidenze.Diagnostica) {
	if c.nonDeterminabile() {
		return CompletezzaDocumentale{Stato: DocumentiNonCalcolabile, Motivo: MotivoDocumentiNonDeterminabile,
			MotivoPerimetro: MotivoPerimetroNonDeterminabile}, nil
	}
	in := IngressoCompletezza{Struttura: s, Conferme: conferme}
	perimetro, _ := c.b.perimetro(pv.ComponenteID)
	certi := c.perimetroSenzaRimozioni(pv.ComponenteID)
	var ids []uuid.UUID
	for k := range perimetro {
		ids = append(ids, k)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	for _, k := range ids {
		comp := c.componenti[k]
		if comp == nil {
			continue
		}
		r := c.rigaDelPerimetro(comp, pv)
		r.RimozioneAperta = !certi[k]
		in.Righe = append(in.Righe, r)
	}
	in.Nodi = c.nodiDaPrevedere(pv, strutture, perimetro)
	doc := Completezza(in)

	var diag []evidenze.Diagnostica
	for _, v := range doc.Voci {
		if v.TipoDocumento == tipoDocumentoDisegno2D && v.DerogaID != nil {
			diag = append(diag, evidenze.Diagnostica{
				Codice:    CodiceDocumentiDerogaNonSostituisce2D,
				Gravita:   evidenze.GravitaAvviso,
				Natura:    evidenze.NaturaDati,
				Percorso:  "prodotti[" + pv.Rif + "].documenti",
				Messaggio: "una deroga sul disegno 2D non lo sostituisce: la voce del 2D resta, con il suo esito (R62 D.4)",
				Rif:       []string{pv.Rif, ancoraggio.RifComponente(v.ComponenteID), "deroga_fabbisogno:" + v.DerogaID.String()},
			})
		}
	}
	return doc, diag
}

// perimetroSenzaRimozioni: i componenti attivi che il componente del prodotto raggiunge per le relazioni confermate senza
// passare da un arco con una rimozione proposta aperta (dubbio T-B5-56).
func (c *completezzaThread) perimetroSenzaRimozioni(componente *uuid.UUID) map[uuid.UUID]bool {
	dentro := map[uuid.UUID]bool{}
	if componente == nil {
		return dentro
	}
	tolti := map[string]bool{}
	for _, r := range c.t.RimozioniAperte {
		tolti[r.PadreID.String()+"\x00"+r.FiglioID.String()] = true
	}
	dentro[*componente] = true
	coda := []uuid.UUID{*componente}
	for len(coda) > 0 {
		p := coda[0]
		coda = coda[1:]
		for _, r := range c.b.figli[p] {
			if tolti[r.PadreID.String()+"\x00"+r.FiglioID.String()] || dentro[r.FiglioID] {
				continue
			}
			dentro[r.FiglioID] = true
			coda = append(coda, r.FiglioID)
		}
	}
	return dentro
}

// rigaDelPerimetro: una riga certa, con i fabbisogni e la voce del 2D:
//   - i fabbisogni: le righe di v_fascicolo del componente (R62 d C: la vista è la base dei componenti confermati), con
//     l'esito, il documento, lo stato del NAS, la deroga, la proposta aperta e il suo allegato (CalcolataDa = vista); senza
//     righe nella vista, le regole effettive del suo tipo (FabbisogniDelTipo), con l'esito calcolato come lo calcola la
//     vista (esitoComeLaVista, CalcolataDa = go);
//   - RegolaCliente: dalle regole effettive del suo tipo (RigaFabbisogno.Proprio: decisioni sull'analista, punto 8);
//   - le categorie della grammatica sul codice (Classificazione.Proposta);
//   - la voce del 2D (disegnoDellaVoce).
func (c *completezzaThread) rigaDelPerimetro(comp *fotorfq.Componente, pv ProdottoValutato) RigaDelPerimetro {
	r := RigaDelPerimetro{ComponenteID: comp.ID, Codice: comp.Codice, TipoComponente: comp.Tipo,
		DelProdotto: pv.ComponenteID != nil && *pv.ComponenteID == comp.ID, Categorie: c.categorieDel(comp)}
	regole, cliente := FabbisogniDelTipo(comp.Tipo, c.t.Fabbisogni)
	r.RegolaCliente = cliente
	var del2D *fotorfq.RigaFascicolo
	if righe := c.vista[comp.ID]; len(righe) > 0 {
		for i := range righe {
			v := righe[i]
			if v.TipoDocumento == tipoDocumentoDisegno2D && del2D == nil {
				del2D = &righe[i]
			}
			r.Fabbisogni = append(r.Fabbisogni, FabbisognoDellaRiga{TipoDocumento: v.TipoDocumento, Bloccante: v.Bloccante, EsitoVista: v.Esito,
				CalcolataDa: CalcolataDaVista, DocumentoID: copiaUUID(v.DocumentoID), StatoNas: strings.TrimSpace(testoDi(v.StatoNas)),
				DerogaID: copiaUUID(v.DerogaID), PropostaAperta: copiaUUID(v.PropostaAperta), FileCandidato: c.allegatoDellaProposta(v.PropostaAperta)})
		}
	} else {
		for _, g := range regole {
			f := c.esitoComeLaVista(comp, g.TipoDocumento)
			f.Bloccante = g.Bloccante
			r.Fabbisogni = append(r.Fabbisogni, f)
		}
	}
	r.Disegno = c.disegnoDellaVoce(comp, del2D)
	return r
}

// esitoComeLaVista: l'esito di un fabbisogno di un componente senza righe nella vista, calcolato come lo calcola
// v_fascicolo (0020), con CalcolataDa = go: il documento corrente di quel tipo più recente (ok, ok_in_coda, ok_errore_nas
// dallo stato del NAS; l'anomalia aperta non è nella fotografia), poi la deroga (derogato), poi la proposta aperta di quel
// tipo assegnata al componente o con il suo codice (da_confermare), altrimenti manca. sul_portale non si calcola: il
// riferimento al portale non è nella fotografia.
func (c *completezzaThread) esitoComeLaVista(comp *fotorfq.Componente, tipo string) FabbisognoDellaRiga {
	f := FabbisognoDellaRiga{TipoDocumento: tipo, CalcolataDa: CalcolataDaGo, EsitoVista: esitoFascicoloManca}
	var doc *fotorfq.DocumentoConfermato
	for i := range c.t.Documenti {
		d := &c.t.Documenti[i]
		if d.ComponenteID == nil || *d.ComponenteID != comp.ID || d.Tipo != tipo || d.SostituitoDa != nil {
			continue
		}
		if doc == nil || d.ConfermatoIl.After(doc.ConfermatoIl) || (d.ConfermatoIl.Equal(doc.ConfermatoIl) && d.ID.String() < doc.ID.String()) {
			doc = d
		}
	}
	f.DerogaID = c.deroga(comp.ID, tipo)
	if p := c.propostaAperta(comp, tipo); p != nil {
		id := p.ID
		f.PropostaAperta, f.FileCandidato = &id, c.allegatoDellaProposta(&id)
	}
	switch {
	case doc != nil:
		id := doc.ID
		f.DocumentoID, f.StatoNas = &id, doc.StatoNas
		switch doc.StatoNas {
		case statoNasErrore:
			f.EsitoVista = esitoFascicoloOKErroreNas
		case statoNasInCoda:
			f.EsitoVista = esitoFascicoloOKInCoda
		default:
			f.EsitoVista = esitoFascicoloOK
		}
	case f.DerogaID != nil:
		f.EsitoVista = esitoFascicoloDerogato
	case f.PropostaAperta != nil:
		f.EsitoVista = esitoFascicoloDaConfermare
	}
	return f
}

// deroga: la deroga sul tipo di documento del componente (deroga_fabbisogno; la prima per ID); nil senza.
func (c *completezzaThread) deroga(componente uuid.UUID, tipo string) *uuid.UUID {
	var out *uuid.UUID
	for _, d := range c.t.Deroghe {
		if d.ComponenteID == componente && d.Tipo == tipo && (out == nil || d.ID.String() < out.String()) {
			id := d.ID
			out = &id
		}
	}
	return out
}

// propostaAperta: la proposta aperta di quel tipo assegnata al componente, o senza componente con il suo codice (senza
// maiuscole), come la cerca v_fascicolo; la prima per ID (la vista prende la più vecchia per creato_il, che la fotografia
// non porta). nil senza.
func (c *completezzaThread) propostaAperta(comp *fotorfq.Componente, tipo string) *fotorfq.PropostaAttuale {
	for i := range c.proposte {
		p := &c.proposte[i]
		if p.Stato != statoPropostaAperta || p.Tipo != tipo {
			continue
		}
		if (p.ComponenteID != nil && *p.ComponenteID == comp.ID) ||
			(p.ComponenteID == nil && p.Codice != nil && strings.EqualFold(*p.Codice, comp.Codice)) {
			return p
		}
	}
	return nil
}

// allegatoDellaProposta: l'allegato di una proposta della fotografia; nil senza.
func (c *completezzaThread) allegatoDellaProposta(id *uuid.UUID) *uuid.UUID {
	if id == nil {
		return nil
	}
	for _, p := range c.proposte {
		if p.ID == *id {
			a := p.AllegatoID
			return &a
		}
	}
	return nil
}

// disegnoDellaVoce: ciò che serve alla voce del 2D di un componente (VoceDelDisegno): il suo gruppo dei 2D (fase 2), la
// deroga e la proposta aperta della riga del 2D della vista (senza la riga, calcolate come la vista), l'allegato della
// proposta se il file è in un formato configurato (T-B0-30: un DWG proposto non soddisferebbe la voce nemmeno confermato,
// dubbio T-B5-57), i candidati da_determinare del motore A sui nodi del componente (LD-17), gli allegati scartati.
func (c *completezzaThread) disegnoDellaVoce(comp *fotorfq.Componente, del2D *fotorfq.RigaFascicolo) DisegnoDellaVoce {
	d := DisegnoDellaVoce{Gruppo: c.gruppi[comp.ID], Scartati: append([]uuid.UUID(nil), c.scartati...)}
	if del2D != nil {
		d.DerogaID, d.PropostaAperta = copiaUUID(del2D.DerogaID), copiaUUID(del2D.PropostaAperta)
	} else {
		d.DerogaID = c.deroga(comp.ID, tipoDocumentoDisegno2D)
		if p := c.propostaAperta(comp, tipoDocumentoDisegno2D); p != nil {
			id := p.ID
			d.PropostaAperta = &id
		}
	}
	if a := c.allegatoDellaProposta(d.PropostaAperta); a != nil {
		if all := c.dis.col.allegati[*a]; all != nil && FormatoDi(estensioneDi(all)) != "" {
			d.FileProposto = a
		}
	}
	d.CandidatiDaDeterminare = c.dis.candidatiDaDeterminare(comp.ID)
	if len(d.Scartati) == 0 {
		d.Scartati = nil
	}
	return d
}

// nodiDaPrevedere: i nodi delle strutture che contano del prodotto (struttureDellaVerifica), esclusa la radice (il
// prodotto), uno per Rif (lo stesso contenuto in due allegati, o in due strutture, è lo stesso nodo):
//   - un nodo che la radice raggiunge solo attraverso archi tolti da una persona (le righe di relazione_proposta tutte
//     scartate da chi l'ha deciso) è uscito dalla BOM: non è previsto, nemmeno se è deciso (dubbio 2 del revisore della
//     fase 3, deciso dall'orchestratore [T]; dubbio T-B5-67);
//   - un nodo deciso come un componente del perimetro è una riga certa: non è previsto;
//   - un nodo deciso come un componente confermato che il perimetro non raggiunge: previsto con il tipo e il codice del
//     componente e il suo gruppo dei 2D (nodo_deciso_fuori_perimetro);
//   - un nodo senza decisione con la riga decisa da una persona (scartato, o con un componente archiviato) è uscito
//     dalla BOM: non è previsto;
//   - gli altri nodi: previsti con il tipo proposto (tipoProposto), il codice proposto della catena (CatenaCodice.Proposto),
//     le categorie della sua lettura e i 2D candidati del nodo (gruppoDelNodo) (nodo_proposto).
//
// Il Rif è un riferimento di testo, mai analizzato (T-B5-93).
func (c *completezzaThread) nodiDaPrevedere(pv ProdottoValutato, strutture []ancoraggio.StrutturaProdotto, perimetro map[uuid.UUID]bool) []NodoDaPrevedere {
	type infoNodo struct {
		n         ancoraggio.NodoProposto
		deciso    *uuid.UUID
		tolto     bool
		conFigli  bool
		raggiunto bool
	}
	per := map[string]*infoNodo{}
	var ordine []string
	for _, st := range struttureDellaVerifica(strutture, pv.Rif) {
		padri := map[string]bool{}
		figli := map[string][]string{}
		for _, a := range st.Archi {
			padri[a.Padre] = true
			if !c.b.archiTolti[RifArco(a.Padre, a.Figlio)] {
				figli[a.Padre] = append(figli[a.Padre], a.Figlio)
			}
		}
		raggiunti := map[string]bool{st.Radice: true}
		coda := []string{st.Radice}
		for len(coda) > 0 {
			x := coda[0]
			coda = coda[1:]
			for _, f := range figli[x] {
				if !raggiunti[f] {
					raggiunti[f] = true
					coda = append(coda, f)
				}
			}
		}
		for _, n := range st.Nodi {
			if n.Rif == st.Radice {
				continue
			}
			i := per[n.Rif]
			if i == nil {
				i = &infoNodo{n: n}
				per[n.Rif] = i
				ordine = append(ordine, n.Rif)
			}
			if n.Decisione != nil && i.deciso == nil {
				id := n.Decisione.ComponenteID
				i.deciso = &id
			}
			if n.Decisione == nil && n.RigaDecisa != nil && n.DecisoDaPersona {
				i.tolto = true
			}
			i.conFigli = i.conFigli || padri[n.Rif]
			i.raggiunto = i.raggiunto || raggiunti[n.Rif]
		}
	}
	sort.Strings(ordine)
	var out []NodoDaPrevedere
	for _, rif := range ordine {
		i := per[rif]
		switch {
		case !i.raggiunto:
		case i.deciso != nil:
			comp := c.componenti[*i.deciso]
			if perimetro[*i.deciso] || comp == nil {
				continue
			}
			regole, cliente := FabbisogniDelTipo(comp.Tipo, c.t.Fabbisogni)
			out = append(out, NodoDaPrevedere{Nodo: rif, ComponenteID: copiaUUID(i.deciso), CodiceProposto: comp.Codice, TipoComponente: comp.Tipo,
				Categorie: c.categorieDel(comp), RegolaCliente: cliente, Regole: regole, Disegni: copiaGruppo(c.gruppi[comp.ID]),
				Motivo: MotivoPrevistoNodoDecisoFuoriPerimetro})
		case i.tolto:
		default:
			tipo := c.tipoProposto(rif, i.conFigli)
			regole, cliente := FabbisogniDelTipo(tipo, c.t.Fabbisogni)
			var categorie []string
			if f := i.n.Codice.Forma; f != nil {
				categorie = categorieDi(f.Categorie)
			}
			out = append(out, NodoDaPrevedere{Nodo: rif, CodiceProposto: i.n.Codice.Proposto, TipoComponente: tipo, Categorie: categorie,
				RegolaCliente: cliente, Regole: regole, Disegni: c.dis.gruppoDelNodo(pv.Rif, i.n), Motivo: MotivoPrevistoNodoProposto})
		}
	}
	return out
}

// tipoProposto: il tipo proposto di un nodo (tabella dell'analista, T29): componente_proposta.tipo_proposto della sua riga
// legacy (la prima per ID, su qualunque allegato); senza, la regola con cui il legacy lo scrive (rfq/fascicolo,
// Pianifica): sottoassieme se nella struttura il nodo ha dei figli, altrimenti sciolto; mai finito (dubbio T-B5-58).
func (c *completezzaThread) tipoProposto(rif string, conFigli bool) string {
	if t, ok := c.tipi[rif]; ok {
		return t
	}
	if conFigli {
		return tipoSottoassieme
	}
	return tipoSciolto
}

// categorieDel: le categorie della grammatica sulla lettura del codice registrato del componente (6.4.6,
// LetturaRegistrata; R31 c); nessuna se il codice non si legge.
func (c *completezzaThread) categorieDel(comp *fotorfq.Componente) []string {
	if x, ok := c.categorie[comp.ID]; ok {
		return x
	}
	var out []string
	if l, ok := leggiCodiceRegistrato(c.m, comp.Codice); ok {
		out = categorieDi(l.Categorie)
	}
	c.categorie[comp.ID] = out
	return out
}

// classificazioniDelThread: la classificazione di ogni componente attivo del thread (Classifica), in ordine di
// componente, con le conferme date (in A1c nessuna: LD-19). Il componente di un prodotto target ha il ruolo prodotto,
// qualunque sia il suo tipo (R-23).
func (c *completezzaThread) classificazioniDelThread(conferme []ConfermaCategoria) []ComponenteClassificato {
	var ids []uuid.UUID
	for k := range c.componenti {
		ids = append(ids, k)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	var out []ComponenteClassificato
	for _, k := range ids {
		comp := c.componenti[k]
		id := k
		out = append(out, ComponenteClassificato{ComponenteID: k,
			Classificazione: Classifica(ComponenteDaClassificare{ComponenteID: &id, Tipo: comp.Tipo, DelProdotto: c.prodotti[k],
				Categorie: c.categorieDel(comp)}, conferme)})
	}
	return out
}
