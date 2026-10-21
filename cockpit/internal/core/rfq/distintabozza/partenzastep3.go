package distintabozza

// La partenza step, algoritmo importazione-1 (EM §6.1 «Stato di partenza della partenza step», P-1...P-8; §6.3; U2 §2.8;
// direttiva 35, blocco C-3): la versione 1 dal registro d'importazione del Motore A, costruita dal server.
//
//	P-1 un nodo della bozza per ogni voce del registro raggiunta e non tolta;
//	P-2 la radice diventa il nodo del prodotto (registrato, casella della radice, origine registrato), sempre;
//	P-3 una voce con decisione_esistente: riusa del componente registrato (casella, origine step, registrato), salvo il
//	    raggruppamento (decisione_esistente = il prodotto, su una voce che non e' la radice), che segue P-4 con il tipo
//	    sottoassieme;
//	P-4 le altre voci: crea, proposta con il codice proposto (identita' della lettura, rev_proposta) o da_decidere senza;
//	    descrizione del PRODUCT nei 200 caratteri, altrimenti vuota con la traccia (mai troncata); tipo proposto, oppure
//	    la regola di sempre (foglia: sciolto; altrimenti sottoassieme) [T]; casella; origine step;
//	P-5 un arco per ogni arco del registro fra nodi inclusi, qta dei fatti, origine step; una quantita' assente o fuori da
//	    1...100000 rifiuta la partenza (422 arco_senza_quantita);
//	P-6 gruppo bom: i confermati senza STEP (i componenti della working sotto un nodo registrato, che il registro non
//	    rappresenta) diventano nodi riusa con origine registrato, senza casella, con i loro archi confermati [T: la
//	    discesa dalla working a partire dai nodi registrati, come /dati li mostra nel gruppo bom quando un loro padre ci sta];
//	P-7 nessuna decisione e nessuna preassociazione: le proposte d'associazione del motore restano proposte ([U] dir. 35
//	    §8 C-3: nessuna PA diventa PB senza il gesto);
//	P-8 i nodi tolti o non raggiunti non entrano: restano fra i fuori per decisione del registro (FuoriDelRegistro).

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// MaxQtaArco: la quantita' massima di un arco del registro che la partenza accetta (EM §6.1, P-5: 1…100000).
const MaxQtaArco = MaxQta

// VersioneDaRegistro costruisce la versione 1 della partenza step dal registro (commento di testa). Il registro e' quello del
// gruppo scelto, gia' composto (importazione.Componi) e confrontato con l'impronta dichiarata (V-7) da chi chiama.
func VersioneDaRegistro(reg importazione.Registro, in IngressoPartenza) (Snapshot3, TracciaPartenza, error) {
	var tr TracciaPartenza
	c := reg.Contenuto
	allegato, err := uuid.Parse(c.Proposta.Fonte.AllegatoID)
	if err != nil {
		return Snapshot3{}, tr, corpoMotivo("crea.sorgente", MotivoGruppoNonImportabile, "il registro non ha la fonte: niente da importare")
	}
	base, firma, err := in.base()
	if err != nil {
		return Snapshot3{}, tr, err
	}
	s := Snapshot3{Formato: FormatoSnapshot3, ProdottoID: in.Prodotto, VersioneBomBase: &base, FirmaBase: firma,
		Sorgente: &Sorgente3{AllegatoID: &allegato, Sha256: c.Proposta.Fonte.Sha256, Gruppo: c.Gruppo.Chiave, Importazione: reg.Impronta},
		Nodi:     []Nodo3{}, Archi: []Arco3{}, Decisioni: []Decisione3{}, DocumentiPreassociati: []Preassociazione3{}}
	casella := func(v importazione.Voce) []Casella3 {
		return []Casella3{{AllegatoID: allegato, Gruppo: c.Gruppo.Chiave, Rif: v.Origine.Rif}}
	}
	chiaveDi := map[string]string{} // rif → chiave_bozza dei nodi inclusi
	registrati := map[uuid.UUID]string{}
	prodotto, ok := in.Stato.Componente(in.Prodotto)
	if !ok {
		return Snapshot3{}, tr, fuori("prodotto_id", "il prodotto %s non e' della RFQ", in.Prodotto)
	}
	radiceVista := false
	for _, v := range c.Proposta.Nodi {
		if (v.StatoMotore.Raggiunto != nil && !*v.StatoMotore.Raggiunto) || (v.StatoMotore.Tolto != nil && *v.StatoMotore.Tolto) {
			continue // P-8
		}
		switch {
		case c.EVoceDellaRadice(v): // P-2
			n := in.nodoRegistrato(prodotto, ChiaveDelComponente(prodotto.ID), OrigineRegistrato, &tr)
			n.Caselle = casella(v)
			s.Nodi = append(s.Nodi, n)
			chiaveDi[v.Origine.Rif] = n.ChiaveBozza
			registrati[prodotto.ID] = n.ChiaveBozza
			radiceVista = true
			continue
		}
		if comp, ok := decisioneEsistente(v); ok && comp != in.Prodotto { // P-3 (il raggruppamento segue P-4)
			if w, ok := in.Stato.Componente(comp); ok && !w.Archiviato {
				k := ChiaveDellaVoce(allegato, v.Origine.Rif)
				n := in.nodoRegistrato(w, k, OrigineStep, &tr)
				n.Caselle = casella(v)
				s.Nodi = append(s.Nodi, n)
				chiaveDi[v.Origine.Rif] = k
				if _, gia := registrati[comp]; !gia {
					registrati[comp] = k
				}
				continue
			}
		}
		n, err := nodoProposto(v, allegato, c, in.Cliente, &tr) // P-4
		if err != nil {
			return Snapshot3{}, tr, err
		}
		if comp, ok := decisioneEsistente(v); ok && comp == in.Prodotto {
			n.Tipo = "sottoassieme" // il raggruppamento dichiarato con «Autorizza» (EM §6.1, P-3; O-01)
		}
		n.Caselle = casella(v)
		s.Nodi = append(s.Nodi, n)
		chiaveDi[v.Origine.Rif] = n.ChiaveBozza
	}
	if !radiceVista {
		// P-2 vale sempre: senza la voce della radice fra le raggiunte il nodo del prodotto c'e' comunque, senza casella
		n := in.nodoRegistrato(prodotto, ChiaveDelComponente(prodotto.ID), OrigineRegistrato, &tr)
		s.Nodi = append(s.Nodi, n)
		registrati[prodotto.ID] = n.ChiaveBozza
	}
	step := &StepArco3{AllegatoID: allegato, Gruppo: c.Gruppo.Chiave}
	for _, a := range c.Proposta.Archi { // P-5
		kp, okp := chiaveDi[a.Padre]
		kf, okf := chiaveDi[a.Figlio]
		if !okp || !okf || a.AllegatoID != allegato.String() {
			continue
		}
		if a.Quantita == nil || *a.Quantita < 1 || *a.Quantita > MaxQtaArco {
			return Snapshot3{}, tr, corpoMotivo("crea", MotivoArcoSenzaQuantita,
				"l'arco %s → %s del registro non ha una quantita' fra 1 e %d: la partenza non la inventa", a.Padre, a.Figlio, MaxQtaArco)
		}
		st := *step
		s.Archi = append(s.Archi, Arco3{Padre: kp, Figlio: kf, Qta: *a.Quantita, Origine: OrigineStep, Step: &st})
	}
	if c.Gruppo.Chiave == GruppoBOMDiLavoro { // P-6
		if err := in.confermatiSenzaStep(&s, registrati, &tr); err != nil {
			return Snapshot3{}, tr, err
		}
	}
	tr.ordina()
	return s, tr, nil
}

// GruppoBOMDiLavoro: la chiave del gruppo della BOM di lavoro (propostabom.GruppoBOM).
const GruppoBOMDiLavoro = "bom"

// FuoriDelRegistro: le voci fuori per decisione del registro (P-8), che la partenza non mette nella bozza.
func FuoriDelRegistro(c importazione.Contenuto) []string {
	out := []string{}
	for _, x := range c.Proposta.FuoriPerDecisione {
		out = append(out, x.Voce)
	}
	sort.Strings(out)
	return out
}

// decisioneEsistente: il componente della decisione_esistente di una voce, se c'e'.
func decisioneEsistente(v importazione.Voce) (uuid.UUID, bool) {
	if len(v.DecisioneEsistente) == 0 || string(v.DecisioneEsistente) == "null" {
		return uuid.Nil, false
	}
	var de struct {
		ComponenteID string `json:"componente_id"`
	}
	if json.Unmarshal(v.DecisioneEsistente, &de) != nil {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(de.ComponenteID)
	return id, err == nil
}

// nodoProposto: il nodo P-4 di una voce (commento di testa), senza la casella.
func nodoProposto(v importazione.Voce, allegato uuid.UUID, c importazione.Contenuto, cliente string, tr *TracciaPartenza) (Nodo3, error) {
	k := ChiaveDellaVoce(allegato, v.Origine.Rif)
	n := Nodo3{ChiaveBozza: k, Op: OpCrea, Origine: OrigineStep, Tipo: tipoDellaVoce(v)}
	id, ok, err := IdentitaDellaProposta(v)
	switch {
	case err != nil:
		return Nodo3{}, fmt.Errorf("la voce %s del registro: %w", v.Voce, err)
	case ok:
		n.Identita = id
	default:
		n.Identita = Identita{Emittente: cliente, EmittentePresunto: true, Modo: ModoDaDecidere}
	}
	var g struct {
		Descrizione string `json:"descrizione"`
	}
	if len(v.Grezzi) > 0 {
		_ = json.Unmarshal(v.Grezzi, &g)
	}
	if d := nfc(g.Descrizione); runeOltre(d, MaxDescrizione) || haControlli(d) {
		tr.DescrizioniOltreLimite = append(tr.DescrizioniOltreLimite, k)
	} else {
		n.Descrizione = d
	}
	return n, nil
}

// tipoDellaVoce: il tipo proposto della voce (tipo_proposto.valore, se e' un tipo di componente diverso da finito: finito
// sta solo sulla radice), altrimenti la regola di sempre: foglia → sciolto, altrimenti sottoassieme [T].
func tipoDellaVoce(v importazione.Voce) string {
	var tp struct {
		Valore string `json:"valore"`
	}
	if len(v.TipoProposto) > 0 && json.Unmarshal(v.TipoProposto, &tp) == nil && tp.Valore != string(db.TipoComponenteFinito) &&
		db.TipoComponente(tp.Valore).Valid() {
		return tp.Valore
	}
	var foglia bool
	if len(v.Foglia) > 0 {
		_ = json.Unmarshal(v.Foglia, &foglia)
	}
	if foglia {
		return string(db.TipoComponenteSciolto)
	}
	return string(db.TipoComponenteSottoassieme)
}

// confermatiSenzaStep: P-6 (commento di testa). Dai nodi registrati della bozza si scende per gli archi della working verso
// i figli non archiviati che la bozza non ha: ognuno diventa un nodo riusa, origine registrato, senza casella, con l'arco
// confermato (origine registrato, qta e posizione della working); si continua dai nodi aggiunti.
func (in IngressoPartenza) confermatiSenzaStep(s *Snapshot3, registrati map[uuid.UUID]string, tr *TracciaPartenza) error {
	figli := map[uuid.UUID][]int{}
	for i, a := range in.Stato.Archi {
		figli[a.Padre] = append(figli[a.Padre], i)
	}
	coda := make([]uuid.UUID, 0, len(registrati))
	for id := range registrati {
		coda = append(coda, id)
	}
	sort.Slice(coda, func(i, j int) bool { return coda[i].String() < coda[j].String() })
	aggiunti := map[uuid.UUID]bool{}
	for len(coda) > 0 {
		padre := coda[0]
		coda = coda[1:]
		if !aggiunti[padre] {
			if _, ok := registrati[padre]; !ok {
				continue
			}
		}
		for _, i := range figli[padre] {
			a := in.Stato.Archi[i]
			w, ok := in.Stato.Componente(a.Figlio)
			if !ok || w.Archiviato {
				continue
			}
			if _, rappresentato := registrati[a.Figlio]; rappresentato {
				continue
			}
			k := ChiaveDelComponente(a.Figlio)
			s.Nodi = append(s.Nodi, in.nodoRegistrato(w, k, OrigineRegistrato, tr))
			registrati[a.Figlio] = k
			aggiunti[a.Figlio] = true
			x := Arco3{Padre: registrati[padre], Figlio: k, Qta: int(a.Qta), Origine: OrigineRegistrato}
			if p := nfc(a.Posizione); runeOltre(p, MaxPosizione) {
				tr.PosizioniOltreLimite = append(tr.PosizioniOltreLimite, x.Padre+">"+x.Figlio)
			} else {
				x.Posizione = p
			}
			s.Archi = append(s.Archi, x)
			coda = append(coda, a.Figlio)
		}
	}
	return nil
}
