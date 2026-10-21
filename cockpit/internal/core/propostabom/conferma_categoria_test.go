package propostabom

// L1 — V1, ondata C: l'adattatore della conferma della categoria (emendamento U2 §6.4.2; CAT-07, CAT-08 la parte del
// motore). Dal registro delle decisioni dell'ultima conferma valida: solo conferma_categoria con esito conferma e una
// categoria del vocabolario chiuso; il componente dal soggetto o dalla mappa; l'autore e il momento della voce (anche
// {utente_id}), altrimenti quelli della conferma; niente da una conferma non valida, da un rifiuto, da un registro che
// non si legge. Gli ID sono inventati.

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/valutazione"
)

func TestConfermeCategoriaDalRegistro(t *testing.T) {
	comp1 := uuid.MustParse("00000000-0000-4000-8000-0000000000a1")
	comp2 := uuid.MustParse("00000000-0000-4000-8000-0000000000a2")
	conferma := uuid.MustParse("00000000-0000-4000-8000-0000000000c1")
	autore := uuid.MustParse("00000000-0000-4000-8000-0000000000d1")
	il := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	decisioni := `[
	  {"id": "cc-1", "tipo": "conferma_categoria", "soggetto": {"chiave_bozza": "k-1"}, "categoria": "minuteria", "esito": "conferma",
	   "autore": "` + autore.String() + `", "quando": "2026-10-17T08:30:00Z"},
	  {"id": "cc-2", "tipo": "conferma_categoria", "soggetto": {"componente_id": "` + comp2.String() + `"}, "categoria": "commerciale",
	   "esito": "conferma", "autore": {"utente_id": "` + autore.String() + `"}},
	  {"id": "cc-3", "tipo": "conferma_categoria", "soggetto": {"chiave_bozza": "k-2"}, "categoria": "minuteria", "esito": "rifiuto"},
	  {"id": "cc-4", "tipo": "conferma_categoria", "soggetto": {"chiave_bozza": "k-1"}, "categoria": "vite", "esito": "conferma"},
	  {"id": "cc-5", "tipo": "conferma_categoria", "soggetto": {"chiave_bozza": "k-ignota"}, "categoria": "minuteria", "esito": "conferma"},
	  {"id": "mp-1", "tipo": "mono_pezzo", "soggetto": {"chiave_bozza": "k-1"}}
	]`
	mappa := `{"k-1": "` + comp1.String() + `", "k-2": "` + comp2.String() + `"}`
	c := ConfermaDelProdotto{Valida: true, ConfermataDa: conferma, ConfermataIl: il, Decisioni: []byte(decisioni), Mappa: []byte(mappa)}
	got := ConfermeCategoria([]ConfermaDelProdotto{c})
	want := []valutazione.ConfermaCategoria{
		{ComponenteID: comp1, Categoria: valutazione.CategoriaMinuteria, Da: autore, Il: time.Date(2026, 10, 17, 8, 30, 0, 0, time.UTC)},
		{ComponenteID: comp2, Categoria: valutazione.CategoriaCommerciale, Da: autore, Il: il},
	}
	if len(got) != len(want) {
		t.Fatalf("%+v, attese %+v", got, want)
	}
	for i := range want {
		if got[i].ComponenteID != want[i].ComponenteID || got[i].Categoria != want[i].Categoria || got[i].Da != want[i].Da || !got[i].Il.Equal(want[i].Il) {
			t.Errorf("%d: %+v, attesa %+v", i, got[i], want[i])
		}
	}
	// una conferma non più valida, un registro vuoto o illeggibile: niente (mai un'esenzione per errore)
	c.Valida = false
	if x := ConfermeCategoria([]ConfermaDelProdotto{c}); len(x) != 0 {
		t.Errorf("una conferma non valida: %+v", x)
	}
	for _, d := range []string{"", "[]", "{", `{"tipo": "conferma_categoria"}`} {
		if x := ConfermeCategoria([]ConfermaDelProdotto{{Valida: true, Decisioni: []byte(d), Mappa: []byte(mappa)}}); len(x) != 0 {
			t.Errorf("il registro %q: %+v", d, x)
		}
	}
	// senza autore né momento nella voce: quelli della conferma
	solo := `[{"tipo": "conferma_categoria", "soggetto": {"chiave_bozza": "k-1"}, "categoria": "minuteria", "esito": "conferma"}]`
	x := ConfermeCategoria([]ConfermaDelProdotto{{Valida: true, ConfermataDa: conferma, ConfermataIl: il, Decisioni: []byte(solo), Mappa: []byte(mappa)}})
	if len(x) != 1 || x[0].Da != conferma || !x[0].Il.Equal(il) {
		t.Errorf("senza autore nella voce: %+v", x)
	}
}
