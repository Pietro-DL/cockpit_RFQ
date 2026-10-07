//go:build integrazione

// L4 — RO-DOMINIO sul database di prova (A1c-L4S-09; v3 §10.1; T-B6-01): la foto del database prima e dopo
// caricatore.Carica, valutazione.Calcola e confronto.Confronta sulla scena ACME, con e senza gli ingressi dell'agente.
// Nessuna tabella cambia: la sessione si guarda solo nel conteggio, e la coda dei job resta com'è. Due giri danno gli
// stessi esiti.
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è
// pubblico.

package confronto_test

import (
	"bytes"
	"testing"

	"promatec/cockpit/internal/platform/jsoncanonico"
	"promatec/cockpit/internal/platform/testutil"
)

func TestL4LaValutazioneEIlConfrontoNonScrivono(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScenaDB(t, p)
	conIngressiDellAgente(t, p, s)
	pr, reg := testutil.PoolConRegistro(t, testutil.DSN(t))
	r := insiemeACME(t, s.cliente)

	prima := testutil.FotoDelDatabase(t, p, "sessione")
	jobPrima := testutil.Conta(t, p, "job")
	primo := giro(t, "primo", pr, reg, s.thread, r)
	secondo := giro(t, "secondo", pr, reg, s.thread, r)
	dopo := testutil.FotoDelDatabase(t, p, "sessione")
	if d := testutil.Differenze(prima, dopo); len(d) > 0 {
		t.Errorf("Carica, Calcola e Confronta hanno cambiato il database: %v", d)
	}
	if prima["job"] != dopo["job"] || testutil.Conta(t, p, "job") != jobPrima {
		t.Errorf("la coda dei job è cambiata: %q → %q", prima["job"], dopo["job"])
	}
	if len(prima) < 20 {
		t.Fatalf("la foto ha %d tabelle: non legge lo schema giusto", len(prima))
	}

	for _, x := range []percorso{primo, secondo} {
		if len(x.sqlVietato) > 0 {
			t.Errorf("testi vietati nel registro: %q", x.sqlVietato)
		}
	}
	b1, err := jsoncanonico.Codifica(primo.confronti)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := jsoncanonico.Codifica(secondo.confronti)
	if err != nil {
		t.Fatal(err)
	}
	if primo.valutato.Impronta != secondo.valutato.Impronta || !bytes.Equal(b1, b2) {
		t.Errorf("due giri sulla stessa scena: impronte di valutazione %s e %s, confronti uguali %v",
			primo.valutato.Impronta, secondo.valutato.Impronta, bytes.Equal(b1, b2))
	}
}
