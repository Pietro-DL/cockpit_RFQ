package distintabozza

// L1 — R4 formato 3 per le parti dei registrati che R5 applica dal blocco «R5 completo» (direttiva 39; EM §5.2, §9.5):
// il cambio di tipo e il ripristino di un archiviato che toccano un'autorizzazione STEP restano blocchi del catalogo
// (TIPO_EFFETTO_CAMBIATO effetto_non_applicabile, RIUSO_INCOMPATIBILE archiviato_con_autorizzazione); due nodi dello stesso
// componente vogliono la stessa identita' e gli stessi dati (COLLISIONE_EMITTENTI, IDENTITA_INCOMPATIBILE,
// RIUSO_INCOMPATIBILE stesso_componente); il nodo di un registrato porta il codice della working o della ricodifica.

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestR4CambioDiTipoDelRegistrato(t *testing.T) {
	firma := strings.Repeat("ab", 12)
	prepara := func(f *fixR4, x uuid.UUID, tipo string) {
		f.arcoW(f.prodotto, x, 1)
		f.nodoReg("k-x", x, "sciolto")
		f.arco("k-root", "k-x", 1)
		f.decisioni = append(f.decisioni, map[string]any{"id": "ct-1", "tipo": "cambia_tipo", "componente_id": x, "tipo_nuovo": tipo, "firma_effetto": firma})
	}
	t.Run("verso commerciale senza autorizzazione: si applica", func(t *testing.T) {
		f := nuovaFixR4(t)
		x := f.componente("x", "ACM-2100", "sciolto")
		prepara(f, x, "commerciale")
		r := f.calcolaCon(func(in *ingressiR4) { in.effettiTipo[x.String()+"|commerciale"] = firma })
		if len(trova(r.Blocchi, BloccoTipoEffettoCambiato)) != 0 {
			t.Fatalf("blocchi: %+v", r.Blocchi)
		}
	})
	t.Run("verso commerciale con un'autorizzazione STEP: blocco", func(t *testing.T) {
		f := nuovaFixR4(t)
		x := f.componente("x", "ACM-2100", "sciolto")
		prepara(f, x, "commerciale")
		r := f.calcolaCon(func(in *ingressiR4) {
			in.effettiTipo[x.String()+"|commerciale"] = firma
			in.dichiarati = map[uuid.UUID]bool{x: true}
		})
		b := trova(r.Blocchi, BloccoTipoEffettoCambiato)
		if len(b) != 1 || b[0].Motivo != MotivoTipoEffettoNonApplicabile || b[0].ChiaveBozza != "k-x" || r.Confermabile {
			t.Fatalf("blocco: %+v", r.Blocchi)
		}
	})
	t.Run("D1 fra assieme e particolare: si applica anche con l'autorizzazione", func(t *testing.T) {
		f := nuovaFixR4(t)
		x := f.componente("x", "ACM-2100", "sottoassieme")
		f.arcoW(f.prodotto, x, 1)
		f.nodoReg("k-x", x, "sottoassieme")
		f.arco("k-root", "k-x", 1)
		f.decisioni = append(f.decisioni, map[string]any{"id": "d1-x", "tipo": "d1_a_particolare", "chiave_bozza": "k-x"})
		r := f.calcolaCon(func(in *ingressiR4) { in.dichiarati = map[uuid.UUID]bool{x: true} })
		if len(trova(r.Blocchi, BloccoTipoEffettoCambiato)) != 0 || len(trova(r.Blocchi, BloccoD1UltimoFiglio)) != 0 {
			t.Fatalf("blocchi: %+v", r.Blocchi)
		}
	})
}

func TestR4RiusoDiUnArchiviato(t *testing.T) {
	prepara := func(t *testing.T) (*fixR4, uuid.UUID) {
		f := nuovaFixR4(t)
		x := f.componente("x", "ACM-2200", "sciolto")
		f.comp[len(f.comp)-1].Archiviato = true
		f.nodoReg("k-x", x, "sciolto")
		f.arco("k-root", "k-x", 2)
		return f, x
	}
	f, _ := prepara(t)
	if r := f.calcola(); len(trova(r.Blocchi, BloccoRiusoIncompatibile)) != 0 {
		t.Fatalf("riuso di un archiviato senza autorizzazione: %+v", r.Blocchi)
	}
	f, x := prepara(t)
	r := f.calcolaCon(func(in *ingressiR4) { in.dichiarati = map[uuid.UUID]bool{x: true} })
	if b := trova(r.Blocchi, BloccoRiusoIncompatibile); len(b) != 1 || b[0].Motivo != MotivoArchiviatoConAutorizzazione {
		t.Fatalf("archiviato con un'autorizzazione: %+v", r.Blocchi)
	}
}

func TestR4DueNodiDelloStessoComponente(t *testing.T) {
	prepara := func(t *testing.T) (*fixR4, uuid.UUID) {
		f := nuovaFixR4(t)
		sa := f.componente("sa", "ACM-2300", "sottoassieme")
		x := f.componente("x", "ACM-2301", "sciolto")
		f.arcoW(f.prodotto, sa, 1)
		f.arcoW(f.prodotto, x, 1)
		f.arcoW(sa, x, 1)
		f.nodoReg("k-sa", sa, "sottoassieme")
		f.nodoReg("k-x1", x, "sciolto")
		f.nodoReg("k-x2", x, "sciolto")
		f.arco("k-root", "k-sa", 1)
		f.arco("k-root", "k-x1", 1)
		f.arco("k-sa", "k-x2", 1)
		f.sorgenteValida(shaProva('e'))
		return f, x
	}
	nodo := func(f *fixR4, k string) map[string]any {
		for _, n := range f.nodi {
			if n.(map[string]any)["chiave_bozza"] == k {
				return n.(map[string]any)
			}
		}
		t.Fatalf("nodo %s", k)
		return nil
	}
	f, _ := prepara(t)
	if r := f.calcola(); len(r.Blocchi) != 0 {
		t.Fatalf("due nodi uguali dello stesso componente: %v %+v", codici(r.Blocchi), r.Blocchi)
	}
	// due revisioni diverse (nodi modifica): RIUSO_INCOMPATIBILE stesso_componente
	f, _ = prepara(t)
	nodo(f, "k-x1")["op"], nodo(f, "k-x2")["op"] = "modifica", "modifica"
	nodo(f, "k-x2")["identita"].(map[string]any)["rev"] = "B"
	r := f.calcola()
	if b := trova(r.Blocchi, BloccoRiusoIncompatibile); len(b) != 1 || b[0].Motivo != MotivoStessoComponente || len(b[0].Differenze) != 1 || b[0].Differenze[0] != "rev" {
		t.Fatalf("revisioni diverse: %v %+v", codici(r.Blocchi), r.Blocchi)
	}
	// emittenti diversi: COLLISIONE_EMITTENTI
	f, _ = prepara(t)
	nodo(f, "k-x2")["op"] = "modifica"
	id := nodo(f, "k-x2")["identita"].(map[string]any)
	id["emittente"], id["emittente_presunto"] = "partner:beta", false
	r = f.calcola()
	if len(trova(r.Blocchi, BloccoCollisioneEmittenti)) != 1 {
		t.Fatalf("emittenti diversi: %v %+v", codici(r.Blocchi), r.Blocchi)
	}
}

func TestR4CodiceDelNodoDiUnRegistrato(t *testing.T) {
	f := nuovaFixR4(t)
	x := f.componente("x", "ACM-2400", "sciolto")
	f.arcoW(f.prodotto, x, 1)
	f.nodoReg("k-x", x, "sciolto")
	f.arco("k-root", "k-x", 1)
	n := f.nodi[len(f.nodi)-1].(map[string]any)
	n["op"] = "modifica"
	n["identita"].(map[string]any)["codice"] = "ACM-2401"
	r := f.calcola()
	b := trova(r.Blocchi, BloccoIdentitaIncompatibile)
	if len(b) != 1 || b[0].ChiaveBozza != "k-x" || len(b[0].Differenze) != 1 || b[0].Differenze[0] != "codice" {
		t.Fatalf("codice diverso dal registrato senza ricodifica: %v %+v", codici(r.Blocchi), r.Blocchi)
	}
}
