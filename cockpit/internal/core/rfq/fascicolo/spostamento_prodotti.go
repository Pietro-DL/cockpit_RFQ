package fascicolo

// Lo spostamento diretto di un nodo fra prodotti, vietato su tutte le RFQ (direttiva 39 §6, D-TAB-01 [T-39]: «vietare
// lo spostamento diretto fra prodotti su tutte le RFQ, come D-ASS-02 [U], non solo sulle nuove»). D-ASS-02 [U]: nessun
// trasferimento di nodi fra prodotti; lo spostamento interno cambia solo l'arco in ingresso scelto.
//
// La regola [T-39 corsia file]: «Sposta» da un padre a un altro e' interno se i due padri stanno negli stessi prodotti
// finiti (risalendo gli archi attivi della working, il padre compreso se e' un finito). Con insiemi diversi (anche solo
// un prodotto in piu' o in meno, il caso dei componenti condivisi) il gesto si rifiuta e non scrive niente: il nodo si
// scollega e si collega con i due gesti propri, ognuno con il suo esito. «Sposta» senza padre di partenza o senza padre
// di arrivo non e' uno spostamento fra prodotti (e' un collegamento o uno scollegamento) e resta com'era.

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// spostamentoNelloStessoProdotto: nil se lo spostamento da → a resta negli stessi prodotti (o non e' uno spostamento fra
// due padri), altrimenti il rifiuto. Chi chiama ha gia' bloccato la RFQ.
func spostamentoNelloStessoProdotto(ctx context.Context, q *db.Queries, thread uuid.UUID, da, a uuid.NullUUID) error {
	if !da.Valid || !a.Valid {
		return nil
	}
	archi, err := archiAttivi(ctx, q, thread)
	if err != nil {
		return err
	}
	comp, err := q.ListComponentiThread(ctx, thread)
	if err != nil {
		return err
	}
	finiti := map[uuid.UUID]string{}
	for _, c := range comp {
		if c.Tipo == db.TipoComponenteFinito {
			finiti[c.ComponenteID] = c.Codice
		}
	}
	padri := map[uuid.UUID][]uuid.UUID{}
	for _, x := range archi {
		padri[x.Figlio] = append(padri[x.Figlio], x.Padre)
	}
	pDa, pA := ProdottiDi(da.UUID, padri, finiti), ProdottiDi(a.UUID, padri, finiti)
	if stessiProdotti(pDa, pA) {
		return nil
	}
	return Rifiuto("lo spostamento fra prodotti non si fa (D-ASS-02): il padre di partenza sta in " + elencoProdotti(pDa, finiti) +
		", quello di arrivo in " + elencoProdotti(pA, finiti) + ". Niente è cambiato: si scollega e si collega con i due gesti propri")
}

// ProdottiDi: i prodotti finiti da cui x si raggiunge risalendo gli archi (padri: figlio → padri), x compreso se e' un
// finito. Un ciclo non lo ferma (ogni nodo si visita una volta).
func ProdottiDi(x uuid.UUID, padri map[uuid.UUID][]uuid.UUID, finiti map[uuid.UUID]string) map[uuid.UUID]bool {
	out := map[uuid.UUID]bool{}
	visti := map[uuid.UUID]bool{}
	coda := []uuid.UUID{x}
	for len(coda) > 0 {
		n := coda[0]
		coda = coda[1:]
		if visti[n] {
			continue
		}
		visti[n] = true
		if _, ok := finiti[n]; ok {
			out[n] = true
		}
		coda = append(coda, padri[n]...)
	}
	return out
}

func stessiProdotti(a, b map[uuid.UUID]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func elencoProdotti(p map[uuid.UUID]bool, finiti map[uuid.UUID]string) string {
	if len(p) == 0 {
		return "nessun prodotto"
	}
	var cc []string
	for k := range p {
		cc = append(cc, finiti[k])
	}
	sort.Strings(cc)
	return strings.Join(cc, ", ")
}
