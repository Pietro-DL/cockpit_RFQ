package distintabozza

// L1 — il completamento di R4 formato 3 (direttiva 39, blocco «R4 completo» della corsia persistenza): i blocchi del
// canonico del cliente (riepilogo3_canonico.go), gli avvisi del registro, gli effetti dei non inclusi, le condizioni (c),
// (e) e (f) di prima-istanza-1, gli avvisi e i conflitti del motore. Dati sintetici.

import (
	"testing"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
)

func motoreConSuffisso(t *testing.T, suffisso string) *classificazione.Motore {
	t.Helper()
	r, err := regole.ValidaRegole([]byte(`{"suffissi_decorativi": ["` + suffisso + `"]}`))
	if err != nil {
		t.Fatal(err)
	}
	return classificazione.Compila("ACME", r)
}

func (f *fixR4) calcolaCon(prepara func(in *ingressiR4)) RiepilogoR4 {
	f.t.Helper()
	in := f.ingressi()
	if prepara != nil {
		prepara(in)
	}
	r, err := calcolaR4(in)
	if err != nil {
		f.t.Fatalf("riepilogo: %v", err)
	}
	return r
}

// EM §5.2 W5, §8.1 righe 11, 12, 15: CANONICO_COLLIDE, CANONICO_AMBIGUO, RICODIFICA_COLLIDE per canonico, il riuso per
// canonico che non torna; niente senza le regole del cliente.
func TestR4CanonicoDelCliente(t *testing.T) {
	m := motoreConSuffisso(t, "_PRT")
	base := func() *fixR4 {
		f := nuovaFixR4(t)
		f.componente("staffa-prt", "ACM-77720_PRT", "sciolto")
		f.nodoMan("k-staffa", "ACM-77720", "sciolto")
		f.arco("k-root", "k-staffa", 1)
		return f
	}
	// senza regole del cliente: nessun blocco del canonico
	r := base().calcolaCon(nil)
	if len(trova(r.Blocchi, BloccoCanonicoCollide))+len(trova(r.Blocchi, BloccoCanonicoAmbiguo)) != 0 {
		t.Fatalf("senza regole: %v", codici(r.Blocchi))
	}
	// con il suffisso decorativo: CANONICO_COLLIDE verso il componente con il suffisso
	r = base().calcolaCon(func(in *ingressiR4) { in.motore = m })
	cc := trova(r.Blocchi, BloccoCanonicoCollide)
	if len(cc) != 1 || cc[0].ChiaveBozza != "k-staffa" || cc[0].ComponenteID == nil || *cc[0].ComponenteID != idDi("r4-staffa-prt") ||
		cc[0].ChiaveWorking != "ACM-77720" || len(cc[0].Rimedi) != 2 {
		t.Fatalf("CANONICO_COLLIDE: %+v (blocchi %v)", cc, codici(r.Blocchi))
	}
	if r.Confermabile {
		t.Fatal("un blocco del canonico non e' confermabile")
	}
	// riusa_per_canonico verso quel componente: il blocco cade
	f := base()
	f.decisioni = append(f.decisioni, map[string]any{"id": "d-rpc", "tipo": DecisioneRiusaPerCanonico, "chiave_bozza": "k-staffa",
		"componente_id": idDi("r4-staffa-prt")})
	r = f.calcolaCon(func(in *ingressiR4) { in.motore = m })
	if len(trova(r.Blocchi, BloccoCanonicoCollide)) != 0 || len(trova(r.Blocchi, BloccoRiusoIncompatibile)) != 0 {
		t.Fatalf("con riusa_per_canonico: %v", codici(r.Blocchi))
	}
	// un partner non ha il canonico del cliente (il nodo passa al partner dopo la lettura: un nodo a mano e' sempre del cliente)
	r = base().calcolaCon(func(in *ingressiR4) {
		in.motore = m
		for i := range in.snap.Nodi {
			if in.snap.Nodi[i].ChiaveBozza == "k-staffa" {
				in.snap.Nodi[i].Identita.Emittente, in.snap.Nodi[i].Identita.Modo = "partner:acme", ModoProposta
			}
		}
	})
	if len(trova(r.Blocchi, BloccoCanonicoCollide)) != 0 {
		t.Fatalf("partner: %v", codici(r.Blocchi))
	}
	// due registrati con lo stesso canonico (due suffissi del cliente): CANONICO_AMBIGUO, anche con un riusa_per_canonico
	m2 := func() *classificazione.Motore {
		r, err := regole.ValidaRegole([]byte(`{"suffissi_decorativi": ["_PRT", "_ASM"]}`))
		if err != nil {
			t.Fatal(err)
		}
		return classificazione.Compila("ACME", r)
	}()
	for _, conRiuso := range []bool{false, true} {
		f = base()
		f.componente("staffa-asm", "ACM-77720_ASM", "sciolto")
		if conRiuso {
			f.decisioni = append(f.decisioni, map[string]any{"id": "d-rpc", "tipo": DecisioneRiusaPerCanonico, "chiave_bozza": "k-staffa",
				"componente_id": idDi("r4-staffa-prt")})
		}
		r = f.calcolaCon(func(in *ingressiR4) { in.motore = m2 })
		amb := trova(r.Blocchi, BloccoCanonicoAmbiguo)
		if len(amb) != 1 || len(amb[0].Parti) != 3 || len(trova(r.Blocchi, BloccoCanonicoCollide)) != 0 {
			t.Fatalf("CANONICO_AMBIGUO (riuso %v): %+v (blocchi %v)", conRiuso, amb, codici(r.Blocchi))
		}
	}
	// due registrati con lo stesso canonico portati per UUID: due componenti, nessun blocco (Q04 [U])
	f = nuovaFixR4(t)
	x := f.componente("x", "ACM-55500", "sciolto")
	y := f.componente("x-prt", "ACM-55500_PRT", "sciolto")
	f.arcoW(f.prodotto, x, 1)
	f.arcoW(f.prodotto, y, 1)
	f.nodoReg("k-x", x, "sciolto")
	f.nodoReg("k-y", y, "sciolto")
	f.arco("k-root", "k-x", 1)
	f.arco("k-root", "k-y", 1)
	r = f.calcolaCon(func(in *ingressiR4) { in.motore = m })
	if len(trova(r.Blocchi, BloccoCanonicoAmbiguo))+len(trova(r.Blocchi, BloccoCanonicoCollide)) != 0 {
		t.Fatalf("registrati per UUID: %v", codici(r.Blocchi))
	}
	// riusa_per_canonico verso un componente con un altro canonico: RIUSO_INCOMPATIBILE «canonico»
	f = base()
	altro := f.componente("altro", "ACM-99999", "sciolto")
	f.decisioni = append(f.decisioni, map[string]any{"id": "d-rpc", "tipo": DecisioneRiusaPerCanonico, "chiave_bozza": "k-staffa", "componente_id": altro})
	r = f.calcolaCon(func(in *ingressiR4) { in.motore = m })
	ri := trova(r.Blocchi, BloccoRiusoIncompatibile)
	if len(ri) != 1 || ri[0].Motivo != MotivoCanonico || len(trova(r.Blocchi, BloccoCanonicoCollide)) != 1 {
		t.Fatalf("riuso per canonico sbagliato: %+v (blocchi %v)", ri, codici(r.Blocchi))
	}
	// RICODIFICA_COLLIDE per canonico
	f = nuovaFixR4(t)
	z := f.componente("z", "ACM-12300", "sciolto")
	f.componente("w-prt", "ACM-45600_PRT", "sciolto")
	f.arcoW(f.prodotto, z, 1)
	f.nodoReg("k-z", z, "sciolto")
	f.arco("k-root", "k-z", 1)
	f.decisioni = append(f.decisioni, map[string]any{"id": "d-ric", "tipo": DecisioneRicodifica, "componente_id": z, "codice_prima": "ACM-12300",
		"codice_dopo": "ACM-45600", "motivo": "allineato al cartiglio", "emittente_dopo": f.emittente()})
	r = f.calcolaCon(func(in *ingressiR4) { in.motore = m })
	rc := trova(r.Blocchi, BloccoRicodificaCollide)
	if len(rc) != 1 || rc[0].Motivo != MotivoCanonico {
		t.Fatalf("RICODIFICA_COLLIDE per canonico: %+v (blocchi %v)", rc, codici(r.Blocchi))
	}
	r = f.calcolaCon(nil)
	if len(trova(r.Blocchi, BloccoRicodificaCollide)) != 0 {
		t.Fatalf("senza regole la ricodifica non collide: %v", codici(r.Blocchi))
	}
}
