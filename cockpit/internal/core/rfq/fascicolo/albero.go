package fascicolo

// L'albero della BOM working per la schermata (Blocco 8, B8.7; piano §7.3, §14.1): i componenti attivi e
// gli archi fra loro, messi in ordine. E' una funzione pura, sui dati gia' letti: la stessa BOM da'
// sempre lo stesso albero.
//
// Un componente e' una riga sola anche quando ha due padri (A1.1, D1): nell'albero compare sotto ciascuno,
// ma il suo sottoalbero si apre la prima volta e le altre dice «condiviso», e nella completezza conta una
// volta. Un arco che chiuderebbe un giro non si segue: il gate lo rifiuta (Ciclo), qui non deve fermare la
// pagina che lo mostra.

import (
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// Nodo e' un componente in un punto dell'albero: sotto un padre, con la quantita' dell'arco, o radice.
type Nodo struct {
	C       db.Componente
	Padre   uuid.UUID // zero = radice
	Qta     int32     // la quantita' dell'arco; per una radice quella richiesta del componente
	Livello int
	// Padri e' quanti padri attivi ha il componente: piu' di uno = sottoassieme condiviso.
	Padri int
	// Ripetuto: il sottoalbero di questo componente e' gia' aperto altrove nell'albero, qui c'e' solo la
	// riga. Giro: l'arco chiuderebbe un ciclo, e sotto non si scende.
	Ripetuto, Giro bool
	Figli          []*Nodo
}

// Radice dice se il nodo sta in cima.
func (n *Nodo) Radice() bool { return n.Padre == uuid.Nil }

// Albero e' la BOM working in ordine, con quello che non ci sta: gli archiviati (fuori dalla working, A4.9)
// e i componenti che solo un ciclo raggiunge.
type Albero struct {
	Radici     []*Nodo
	Archiviati []db.Componente
	// Totali e' la quantita' complessiva di ogni componente attivo: per una radice la sua, per un figlio la
	// somma, su tutti i padri, della quantita' dell'arco per il totale del padre; per un componente che sta
	// in un ciclo la sua, qualunque sia l'ordine dei dati.
	Totali map[uuid.UUID]int64
	// Righe sono i componenti attivi, uno per riga e nell'ordine dell'albero: la griglia e la completezza.
	Righe []*Nodo
	n     int
}

// Componenti e' quanti componenti attivi ha la working.
func (a Albero) Componenti() int { return a.n }

// NuovoAlbero mette in ordine la BOM: in cima i prodotti finiti, poi le altre radici; sotto ogni nodo i
// figli per codice.
func NuovoAlbero(comp []db.Componente, rel []db.ComponenteRelazione) Albero {
	attivi := map[uuid.UUID]db.Componente{}
	var out Albero
	for _, c := range comp {
		if c.ArchiviatoIl != nil {
			out.Archiviati = append(out.Archiviati, c)
			continue
		}
		attivi[c.ComponenteID] = c
	}
	sort.SliceStable(out.Archiviati, func(i, j int) bool { return codiceMinore(out.Archiviati[i], out.Archiviati[j]) })
	type arco struct {
		figlio uuid.UUID
		qta    int32
	}
	figli := map[uuid.UUID][]arco{}
	padri := map[uuid.UUID]int{}
	padriDi := map[uuid.UUID]map[uuid.UUID]int32{} // figlio → padre → qta, per i totali
	for _, r := range rel {
		if _, ok := attivi[r.PadreID]; !ok {
			continue
		}
		if _, ok := attivi[r.FiglioID]; !ok || r.PadreID == r.FiglioID {
			continue
		}
		figli[r.PadreID] = append(figli[r.PadreID], arco{r.FiglioID, r.Qta})
		padri[r.FiglioID]++
		if padriDi[r.FiglioID] == nil {
			padriDi[r.FiglioID] = map[uuid.UUID]int32{}
		}
		padriDi[r.FiglioID][r.PadreID] = r.Qta
	}
	for p := range figli {
		f := figli[p]
		sort.SliceStable(f, func(i, j int) bool { return codiceMinore(attivi[f[i].figlio], attivi[f[j].figlio]) })
	}
	var radici []db.Componente
	for _, c := range attivi {
		if padri[c.ComponenteID] == 0 {
			radici = append(radici, c)
		}
	}
	sort.SliceStable(radici, func(i, j int) bool {
		fi, fj := radici[i].Tipo == db.TipoComponenteFinito, radici[j].Tipo == db.TipoComponenteFinito
		if fi != fj {
			return fi
		}
		return codiceMinore(radici[i], radici[j])
	})

	aperto := map[uuid.UUID]bool{} // il sottoalbero e' gia' stato aperto
	inCorso := map[uuid.UUID]bool{}
	var visita func(c db.Componente, padre uuid.UUID, qta int32, livello int) *Nodo
	visita = func(c db.Componente, padre uuid.UUID, qta int32, livello int) *Nodo {
		n := &Nodo{C: c, Padre: padre, Qta: qta, Livello: livello, Padri: padri[c.ComponenteID]}
		if aperto[c.ComponenteID] {
			n.Ripetuto = true
			return n
		}
		aperto[c.ComponenteID] = true
		out.Righe = append(out.Righe, n)
		inCorso[c.ComponenteID] = true
		for _, a := range figli[c.ComponenteID] {
			if inCorso[a.figlio] {
				n.Figli = append(n.Figli, &Nodo{C: attivi[a.figlio], Padre: c.ComponenteID, Qta: a.qta, Livello: livello + 1,
					Padri: padri[a.figlio], Giro: true})
				continue
			}
			n.Figli = append(n.Figli, visita(attivi[a.figlio], c.ComponenteID, a.qta, livello+1))
		}
		inCorso[c.ComponenteID] = false
		return n
	}
	for _, c := range radici {
		out.Radici = append(out.Radici, visita(c, uuid.Nil, c.Qta, 0))
	}
	// Quello che nessuna radice raggiunge sta in un ciclo: lo si mostra lo stesso, in fondo, perche' chi
	// deve rompere il giro lo deve vedere.
	var resto []db.Componente
	for id, c := range attivi {
		if !aperto[id] {
			resto = append(resto, c)
		}
	}
	sort.SliceStable(resto, func(i, j int) bool { return codiceMinore(resto[i], resto[j]) })
	for _, c := range resto {
		if !aperto[c.ComponenteID] {
			out.Radici = append(out.Radici, visita(c, uuid.Nil, c.Qta, 0))
		}
	}
	out.n = len(attivi)
	out.Totali = totali(attivi, padriDi)
	return out
}

// totali calcola la quantita' complessiva di ogni componente sul grafo aciclico degli archi; un
// componente in un ciclo resta con la sua quantita' e basta, e chi sta sotto un ciclo moltiplica quella.
//
// Prima il giro si scopriva camminando, e il pezzo del ciclo da cui si partiva — il primo che l'ordine
// della mappa proponeva — prendeva la sua quantita' mentre gli altri sommavano un pezzo di giro: la stessa
// BOM dava totali diversi da un caricamento all'altro. Adesso chi sta in un ciclo si decide prima, sul
// grafo intero (inUnCiclo), e il resto e' un grafo senza giri, dove l'ordine della somma non conta.
func totali(attivi map[uuid.UUID]db.Componente, padriDi map[uuid.UUID]map[uuid.UUID]int32) map[uuid.UUID]int64 {
	nelGiro := inUnCiclo(attivi, padriDi)
	tot := map[uuid.UUID]int64{}
	fatto := map[uuid.UUID]bool{}
	var calcola func(id uuid.UUID) int64
	calcola = func(id uuid.UUID) int64 {
		if fatto[id] {
			return tot[id]
		}
		var t int64
		if len(padriDi[id]) == 0 || nelGiro[id] {
			t = int64(attivi[id].Qta)
		} else {
			// i padri sono in un ciclo (e si fermano li') o fuori da ogni ciclo: la discesa finisce
			for p, q := range padriDi[id] {
				t += int64(q) * calcola(p)
			}
		}
		fatto[id] = true
		tot[id] = t
		return t
	}
	for id := range attivi {
		calcola(id)
	}
	return tot
}

// inUnCiclo dice quali componenti stanno in un giro di archi: le componenti fortemente connesse con piu'
// di un nodo (Tarjan; un arco di un pezzo su se stesso non entra nemmeno nel grafo). L'appartenenza a un
// ciclo non dipende da dove si comincia a guardare.
func inUnCiclo(attivi map[uuid.UUID]db.Componente, padriDi map[uuid.UUID]map[uuid.UUID]int32) map[uuid.UUID]bool {
	indice, basso := map[uuid.UUID]int{}, map[uuid.UUID]int{}
	sullaPila := map[uuid.UUID]bool{}
	var pila []uuid.UUID
	out := map[uuid.UUID]bool{}
	var visita func(v uuid.UUID)
	visita = func(v uuid.UUID) {
		indice[v], basso[v] = len(indice), len(indice)
		pila = append(pila, v)
		sullaPila[v] = true
		for w := range padriDi[v] {
			if _, visto := indice[w]; !visto {
				visita(w)
				basso[v] = min(basso[v], basso[w])
			} else if sullaPila[w] {
				basso[v] = min(basso[v], indice[w])
			}
		}
		if basso[v] != indice[v] {
			return
		}
		var comp []uuid.UUID
		for {
			w := pila[len(pila)-1]
			pila = pila[:len(pila)-1]
			sullaPila[w] = false
			comp = append(comp, w)
			if w == v {
				break
			}
		}
		if len(comp) > 1 {
			for _, w := range comp {
				out[w] = true
			}
		}
	}
	for id := range attivi {
		if _, visto := indice[id]; !visto {
			visita(id)
		}
	}
	return out
}

func codiceMinore(a, b db.Componente) bool {
	x, y := strings.ToUpper(a.Codice), strings.ToUpper(b.Codice)
	if x != y {
		return x < y
	}
	return a.ComponenteID.String() < b.ComponenteID.String()
}
