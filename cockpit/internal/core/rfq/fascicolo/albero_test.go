package fascicolo

// L1 — B8.7 (piano §14.1): l'albero della BOM per la schermata, da componenti e relazioni. Ordine stabile,
// radici multiple, un componente con due padri che e' una riga sola, gli archiviati fuori, un ciclo che
// non ferma niente, le quantita' complessive.

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

type bancoAlbero struct {
	comp []db.Componente
	rel  []db.ComponenteRelazione
	id   map[string]uuid.UUID
}

func nuovoBancoAlbero() *bancoAlbero { return &bancoAlbero{id: map[string]uuid.UUID{}} }

func (b *bancoAlbero) c(codice string, tipo db.TipoComponente, qta int32) *bancoAlbero {
	id := uuid.New()
	b.id[codice] = id
	b.comp = append(b.comp, db.Componente{ComponenteID: id, Codice: codice, Tipo: tipo, Qta: qta})
	return b
}

func (b *bancoAlbero) r(padre, figlio string, qta int32) *bancoAlbero {
	b.rel = append(b.rel, db.ComponenteRelazione{PadreID: b.id[padre], FiglioID: b.id[figlio], Qta: qta})
	return b
}

// disegno scrive l'albero come lo legge chi guarda: un nodo per riga, rientrato, con la quantita' e i segni.
func disegno(a Albero) string {
	var sb strings.Builder
	var giu func(n *Nodo)
	giu = func(n *Nodo) {
		sb.WriteString(strings.Repeat("  ", n.Livello) + n.C.Codice)
		if !n.Radice() {
			sb.WriteString(" ×" + itoa(int(n.Qta)))
		}
		if n.Padri > 1 {
			sb.WriteString(" (condiviso)")
		}
		if n.Ripetuto {
			sb.WriteString(" (sopra)")
		}
		if n.Giro {
			sb.WriteString(" (ciclo)")
		}
		sb.WriteString("\n")
		for _, f := range n.Figli {
			giu(f)
		}
	}
	for _, r := range a.Radici {
		giu(r)
	}
	return sb.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func TestLAlberoHaRadiciMultipleInOrdineStabile(t *testing.T) {
	b := nuovoBancoAlbero().
		c("Z-SCIOLTO", db.TipoComponenteSciolto, 1).
		c("B-PRODOTTO", db.TipoComponenteFinito, 2).
		c("A-PRODOTTO", db.TipoComponenteFinito, 1).
		c("C-ASSIEME", db.TipoComponenteSottoassieme, 1).
		c("D-PARTE", db.TipoComponenteSciolto, 1).
		c("E-PARTE", db.TipoComponenteSciolto, 1).
		r("A-PRODOTTO", "C-ASSIEME", 2).r("C-ASSIEME", "E-PARTE", 3).r("C-ASSIEME", "D-PARTE", 1)
	atteso := "A-PRODOTTO\n  C-ASSIEME ×2\n    D-PARTE ×1\n    E-PARTE ×3\nB-PRODOTTO\nZ-SCIOLTO\n"
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for giro := 0; giro < 20; giro++ {
		r.Shuffle(len(b.comp), func(i, j int) { b.comp[i], b.comp[j] = b.comp[j], b.comp[i] })
		r.Shuffle(len(b.rel), func(i, j int) { b.rel[i], b.rel[j] = b.rel[j], b.rel[i] })
		a := NuovoAlbero(b.comp, b.rel)
		if got := disegno(a); got != atteso {
			t.Fatalf("giro %d: l'ordine dipende dall'ordine dei dati\n%s\natteso\n%s", giro, got, atteso)
		}
		if a.Componenti() != 6 || len(a.Righe) != 6 {
			t.Fatalf("componenti %d, righe %d", a.Componenti(), len(a.Righe))
		}
	}
}

// Un sottoassieme sotto due prodotti e' una riga sola: nell'albero compare sotto ciascuno, il suo
// sottoalbero si apre una volta, nelle righe (griglia e completezza) c'e' una volta, e la quantita'
// complessiva somma i due padri.
func TestUnSottoassiemeCondivisoCompareSottoOgniPadreEContaUnaVolta(t *testing.T) {
	b := nuovoBancoAlbero().
		c("P1", db.TipoComponenteFinito, 2).
		c("P2", db.TipoComponenteFinito, 5).
		c("S", db.TipoComponenteSottoassieme, 1).
		c("V", db.TipoComponenteSciolto, 1).
		r("P1", "S", 1).r("P2", "S", 2).r("S", "V", 4)
	a := NuovoAlbero(b.comp, b.rel)
	atteso := "P1\n  S ×1 (condiviso)\n    V ×4\nP2\n  S ×2 (condiviso) (sopra)\n"
	if got := disegno(a); got != atteso {
		t.Fatalf("albero\n%s\natteso\n%s", got, atteso)
	}
	visti := map[string]int{}
	for _, n := range a.Righe {
		visti[n.C.Codice]++
	}
	if len(a.Righe) != 4 || visti["S"] != 1 || visti["V"] != 1 {
		t.Errorf("righe: %v", visti)
	}
	// P1 = 2, P2 = 5; S = 1·2 + 2·5 = 12; V = 4·12 = 48
	for codice, q := range map[string]int64{"P1": 2, "P2": 5, "S": 12, "V": 48} {
		if a.Totali[b.id[codice]] != q {
			t.Errorf("totale di %s: %d, atteso %d", codice, a.Totali[b.id[codice]], q)
		}
	}
}

// Gli archiviati escono dalla working con i loro archi (A4.9): non sono nell'albero, ma nella lista a parte.
func TestGliArchiviatiNonSonoNellAlbero(t *testing.T) {
	b := nuovoBancoAlbero().
		c("P", db.TipoComponenteFinito, 1).
		c("VIA", db.TipoComponenteSciolto, 1).
		c("RESTA", db.TipoComponenteSciolto, 1).
		r("P", "VIA", 1).r("P", "RESTA", 1).r("VIA", "RESTA", 2)
	ieri := time.Now().Add(-24 * time.Hour)
	b.comp[1].ArchiviatoIl = &ieri
	a := NuovoAlbero(b.comp, b.rel)
	if got := disegno(a); got != "P\n  RESTA ×1\n" {
		t.Fatalf("albero\n%s", got)
	}
	if len(a.Archiviati) != 1 || a.Archiviati[0].Codice != "VIA" {
		t.Errorf("archiviati: %v", a.Archiviati)
	}
}

// Un ciclo non ferma la pagina che lo mostra: l'arco che chiude il giro si segna, sotto non si scende, e
// i componenti che solo il giro raggiunge compaiono lo stesso.
func TestUnCicloNonFermaLAlbero(t *testing.T) {
	b := nuovoBancoAlbero().
		c("P", db.TipoComponenteFinito, 1).
		c("A", db.TipoComponenteSottoassieme, 1).
		c("B", db.TipoComponenteSottoassieme, 1).
		c("X", db.TipoComponenteSottoassieme, 1).
		c("Y", db.TipoComponenteSottoassieme, 1).
		r("P", "A", 1).r("A", "B", 1).r("B", "A", 1).r("X", "Y", 1).r("Y", "X", 1)
	a := NuovoAlbero(b.comp, b.rel)
	got := disegno(a)
	// A ha due padri, P e B: e' condiviso anche se uno dei due archi chiude il giro
	for _, c := range []string{"P\n  A ×1 (condiviso)\n    B ×1\n      A ×1 (condiviso) (ciclo)\n", "X\n  Y ×1\n    X ×1 (ciclo)\n"} {
		if !strings.Contains(got, c) {
			t.Errorf("manca\n%s\nin\n%s", c, got)
		}
	}
	if a.Componenti() != 5 {
		t.Errorf("componenti: %d", a.Componenti())
	}
}
