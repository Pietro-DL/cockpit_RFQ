package distintabozza

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
)

// Lo stato di partenza della prima versione (contratto rev. 3 §6.6, «Stato di partenza della versione 1»): il server
// lo ricostruisce da solo e lo confronta con quello che le azioni della pagina descrivono (Riconciliazione.
// StatoDiPartenza). Cosi' la prima versione non e' adottata com'e' arriva: un nodo «step» o «registrato» inventato,
// senza un'azione che lo aggiunga, non torna (RC-02; direttiva 06 B1: «nessun evento inventato dal browser deve
// diventare prova senza confronto server»).
//
// Il confronto e' per identita', non per chiave: le chiavi della bozza sono della pagina (opache, P4-WZ-01/02), i
// componenti registrati si riconoscono dal componente_id. Si confrontano anche codice, rev, descrizione e tipo, e gli
// archi con qta e posizione. Nello stato di partenza non ci sono ancora fonte, caselle STEP, righe di proposta,
// decisioni ne' preassociazioni: arrivano con le loro azioni.

// ComponenteRegistrato e' un componente della working, come lo vede lo stato di partenza.
type ComponenteRegistrato struct {
	ComponenteID uuid.UUID
	Codice       string
	Rev          string
	Descrizione  string
	Tipo         string
}

// ArcoRegistrato e' un arco della working, con la qta e la posizione.
type ArcoRegistrato struct {
	Padre, Figlio uuid.UUID
	Qta           int
	Posizione     string
}

// Registrato e' cio' che il server ricostruisce dal database per una partenza: per «vuota» il solo prodotto, per
// «registrato» il perimetro della working (il prodotto e tutto cio' che gli sta sotto) con i suoi archi.
type Registrato struct {
	Prodotto   uuid.UUID
	Componenti []ComponenteRegistrato
	Archi      []ArcoRegistrato
}

// ControllaPartenza confronta lo stato di partenza descritto dalle azioni con quello ricostruito dal server. Per
// «vuota» conta solo il prodotto di reg (gli altri componenti e gli archi si ignorano). Un errore e' un
// CORPO_NON_VALIDO con motivo azioni_non_riconciliate, sul campo azioni[0]; una partenza che qui non si
// ricostruisce (conferma, step) e' partenza_non_ricostruibile. Restituisce error, con nil esplicito: un *Errore nil
// dentro un error non sarebbe nil.
func ControllaPartenza(partenza string, s0 Snapshot, reg Registrato) error {
	if e := controllaPartenza(partenza, s0, reg); e != nil {
		return e
	}
	return nil
}

func controllaPartenza(partenza string, s0 Snapshot, reg Registrato) *Errore {
	diverso := func(formato string, a ...any) *Errore {
		return nonRiconciliate("azioni[0]", "partenza %s: "+formato, append([]any{partenza}, a...)...)
	}
	attesi := map[uuid.UUID]ComponenteRegistrato{}
	var archi []ArcoRegistrato
	switch partenza {
	case PartenzaVuota:
		for _, c := range reg.Componenti {
			if c.ComponenteID == reg.Prodotto {
				attesi[c.ComponenteID] = c
			}
		}
	case PartenzaRegistrato:
		for _, c := range reg.Componenti {
			attesi[c.ComponenteID] = c
		}
		archi = reg.Archi
	default:
		return &Errore{Codice: CodiceCorpoNonValido, Motivo: MotivoPartenzaNonRicostruibile, Campo: "azioni[0].dopo.partenza",
			Messaggio: fmt.Sprintf("la partenza %q non si ricostruisce qui", partenza)}
	}
	if _, ok := attesi[reg.Prodotto]; !ok || s0.ProdottoID != reg.Prodotto {
		return diverso("il prodotto %s non e' quello ricostruito dal server", s0.ProdottoID)
	}
	if s0.Sorgente != nil || len(s0.Decisioni) > 0 || len(s0.DocumentiPreassociati) > 0 {
		return diverso("lo stato di partenza non ha fonte, decisioni ne' preassociazioni: arrivano con le loro azioni")
	}
	if len(s0.Nodi) != len(attesi) {
		return diverso("%d nodi, il server ne ricostruisce %d", len(s0.Nodi), len(attesi))
	}
	chiaveDi := map[string]uuid.UUID{}
	visti := map[uuid.UUID]bool{}
	for _, n := range s0.Nodi {
		if n.ComponenteID == nil {
			return diverso("il nodo %s non e' un componente registrato", n.ChiaveBozza)
		}
		c, ok := attesi[*n.ComponenteID]
		if !ok || visti[c.ComponenteID] {
			return diverso("il nodo %s (componente %s) non e' nel perimetro ricostruito, o c'e' due volte", n.ChiaveBozza, *n.ComponenteID)
		}
		visti[c.ComponenteID] = true
		chiaveDi[n.ChiaveBozza] = c.ComponenteID
		if n.Op != OpRiusa || n.Codice != nfc(c.Codice) || n.Rev != nfc(c.Rev) || n.Descrizione != nfc(c.Descrizione) || n.Tipo != c.Tipo {
			return diverso("il nodo %s non ha op riusa con codice, rev, descrizione e tipo del componente registrato %s", n.ChiaveBozza, c.Codice)
		}
		// solo registrato: le partenze che qui si ricostruiscono (vuota, registrato) non vengono da una conferma;
		// l'origine conferma arrivera' con la partenza conferma di B2, confrontata con lo snapshot dell'ultima
		// conferma (revisione «scrittori» della notte, RSC-11)
		if len(n.Caselle) > 0 || len(n.RigheProposta) > 0 || n.Origine != OrigineRegistrato {
			return diverso("il nodo %s parte senza caselle e righe di proposta, con origine registrato", n.ChiaveBozza)
		}
	}
	type coppia struct{ p, f uuid.UUID }
	attese := map[coppia]ArcoRegistrato{}
	for _, a := range archi {
		attese[coppia{a.Padre, a.Figlio}] = a
	}
	if len(s0.Archi) != len(attese) {
		return diverso("%d archi, il server ne ricostruisce %d", len(s0.Archi), len(attese))
	}
	for _, a := range s0.Archi {
		x, ok := attese[coppia{chiaveDi[a.Padre], chiaveDi[a.Figlio]}]
		if !ok || x.Qta != a.Qta || nfc(x.Posizione) != a.Posizione {
			return diverso("l'arco %s → %s (qta %d) non e' un arco della working con quella qta e posizione", a.Padre, a.Figlio, a.Qta)
		}
		if a.Origine != OrigineRegistrato {
			return diverso("l'arco %s → %s parte con origine registrato", a.Padre, a.Figlio)
		}
	}
	return nil
}

// SnapshotDiPartenza scrive lo stato di partenza ricostruito come snapshot formato 2, con le chiavi date da chiave
// (componente → chiave della bozza): e' quello che la pagina riceve per cominciare una bozza nuova (B2 lo esporra'
// con R1) e quello che le prove usano. Base e firma sono quelle date.
func SnapshotDiPartenza(partenza string, reg Registrato, base VersioneBomBase, firma string, chiave func(uuid.UUID) string) Snapshot {
	s := Snapshot{Formato: FormatoSnapshot, ProdottoID: reg.Prodotto, VersioneBomBase: &base, FirmaBase: firma}
	dentro := map[uuid.UUID]bool{}
	for _, c := range reg.Componenti {
		if partenza == PartenzaVuota && c.ComponenteID != reg.Prodotto {
			continue
		}
		dentro[c.ComponenteID] = true
		id := c.ComponenteID
		s.Nodi = append(s.Nodi, Nodo{ChiaveBozza: chiave(id), ComponenteID: &id, Op: OpRiusa, Codice: nfc(c.Codice), Rev: nfc(c.Rev),
			Descrizione: nfc(c.Descrizione), Tipo: c.Tipo, Origine: OrigineRegistrato})
	}
	if partenza == PartenzaRegistrato {
		for _, a := range reg.Archi {
			if dentro[a.Padre] && dentro[a.Figlio] {
				s.Archi = append(s.Archi, Arco{Padre: chiave(a.Padre), Figlio: chiave(a.Figlio), Qta: a.Qta, Origine: OrigineRegistrato, Posizione: nfc(a.Posizione)})
			}
		}
	}
	sort.Slice(s.Nodi, func(i, j int) bool { return s.Nodi[i].ChiaveBozza < s.Nodi[j].ChiaveBozza })
	s.normalizza()
	return s
}
