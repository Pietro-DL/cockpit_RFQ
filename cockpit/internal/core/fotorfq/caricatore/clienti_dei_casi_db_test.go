//go:build integrazione

// L4 — i clienti dichiarati dai casi senza thread nella fotografia (D-Q10-4 = A ristretta, decisa con l'utente l'11/10;
// T-DQ104-1, -2): Richiesta.Clienti si fotografa accanto ai clienti dei thread, con la stessa query
// (ListClientiDellaFotografia, una sola chiamata, lo stesso testo SQL) e solo per la ragione sociale. Senza thread, il
// cliente presente entra; con il thread dello stesso cliente la fotografia e la sua impronta non cambiano; con un cliente
// diverso entrano tutti e due, e i dati dei thread restano gli stessi; un cliente che il database non ha resta fuori,
// senza errore.
//
// La scena è inventata (ACME): nessun dato reale, il repository è pubblico.

package caricatore

import (
	"context"
	"reflect"
	"regexp"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/platform/testutil"
)

// clienteSenzaThread: un secondo cliente ACME, senza nessuna RFQ nella scena.
func clienteSenzaThread(t *testing.T, s scena) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := testutil.Pool(t).QueryRow(context.Background(),
		`INSERT INTO cliente (cartella_nas, ragione_sociale, regole) VALUES ('ACME2', 'ACME Due S.r.l.', $1) RETURNING cliente_id`,
		`{"`+marcatoreRegole+`": true}`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// nomiDelleQuery: i nomi sqlc delle query nel registro, in ordine.
func nomiDelleQuery(reg *testutil.RegistroSQL) []string {
	re := regexp.MustCompile(`-- name: (\w+) `)
	var out []string
	for _, v := range reg.Voci() {
		if m := re.FindStringSubmatch(v.SQL); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}

func clientiDi(f fotorfq.Fotografia) map[uuid.UUID]string {
	out := map[uuid.UUID]string{}
	for _, c := range f.Clienti {
		out[c.ID] = c.RagioneSociale
	}
	return out
}

// TestDQ104SenzaThreadIlClienteDelCaso (D-Q10-4, con e senza thread): senza thread, con il messaggio fuori RFQ e il
// cliente del caso, la fotografia ha quel cliente con la sua ragione sociale, e il messaggio. Senza Clienti il
// cliente non c'è (il difetto di prima).
func TestDQ104SenzaThreadIlClienteDelCaso(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScena(t, p)
	acme2 := clienteSenzaThread(t, s)
	ctx := context.Background()
	f, err := Carica(ctx, p, Richiesta{Messaggi: []uuid.UUID{s.fuori}, Clienti: []uuid.UUID{acme2}})
	if err != nil {
		t.Fatal(err)
	}
	if got := clientiDi(f); !reflect.DeepEqual(got, map[uuid.UUID]string{acme2: "ACME Due S.r.l."}) || len(f.Thread) != 0 || len(f.FuoriRFQ) != 1 {
		t.Errorf("senza thread: clienti %v, %d thread, %d messaggi fuori RFQ", got, len(f.Thread), len(f.FuoriRFQ))
	}
	senza, err := Carica(ctx, p, Richiesta{Messaggi: []uuid.UUID{s.fuori}})
	if err != nil {
		t.Fatal(err)
	}
	if len(senza.Clienti) != 0 {
		t.Errorf("senza Clienti e senza thread la fotografia non ha clienti: %v", clientiDi(senza))
	}
}

// TestDQ104ConIlThreadStessoCliente (non regressione): il cliente del caso uguale al cliente del thread non cambia la
// fotografia né la sua impronta.
func TestDQ104ConIlThreadStessoCliente(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScena(t, p)
	ctx := context.Background()
	r := Richiesta{Thread: []uuid.UUID{s.thread}, Messaggi: []uuid.UUID{s.fuori}}
	f1, err := Carica(ctx, p, r)
	if err != nil {
		t.Fatal(err)
	}
	r.Clienti = []uuid.UUID{s.cliente}
	f2, err := Carica(ctx, p, r)
	if err != nil {
		t.Fatal(err)
	}
	h1, _ := fotorfq.ImprontaFotografia(f1)
	h2, _ := fotorfq.ImprontaFotografia(f2)
	if h1 != h2 || len(f2.Clienti) != 1 {
		t.Errorf("lo stesso cliente del thread: impronta %s → %s, clienti %v", h1, h2, clientiDi(f2))
	}
}

// TestDQ104ConIlThreadClienteDiverso: con un cliente del caso diverso entrano tutti e due, in ordine; i thread e i
// messaggi fuori RFQ sono gli stessi della fotografia senza Clienti.
func TestDQ104ConIlThreadClienteDiverso(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScena(t, p)
	acme2 := clienteSenzaThread(t, s)
	ctx := context.Background()
	r := Richiesta{Thread: []uuid.UUID{s.thread}, Messaggi: []uuid.UUID{s.fuori}}
	f1, err := Carica(ctx, p, r)
	if err != nil {
		t.Fatal(err)
	}
	r.Clienti = []uuid.UUID{acme2}
	f2, err := Carica(ctx, p, r)
	if err != nil {
		t.Fatal(err)
	}
	if got := clientiDi(f2); len(got) != 2 || got[s.cliente] != "ACME S.p.A." || got[acme2] != "ACME Due S.r.l." {
		t.Errorf("due clienti: %v", got)
	}
	if len(f2.Clienti) == 2 && f2.Clienti[0].ID.String() > f2.Clienti[1].ID.String() {
		t.Errorf("i clienti non sono in ordine: %v", f2.Clienti)
	}
	if inJSON(t, f1.Thread) != inJSON(t, f2.Thread) || inJSON(t, f1.FuoriRFQ) != inJSON(t, f2.FuoriRFQ) {
		t.Error("i thread o i messaggi fuori RFQ cambiano con un cliente del caso in più")
	}
}

// TestDQ104ClienteAssente (T-DQ104-1): un cliente che il database non ha resta fuori dalla fotografia, senza errore; il
// messaggio fuori RFQ c'è.
func TestDQ104ClienteAssente(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScena(t, p)
	assente := uuid.MustParse("00000000-0000-4000-8000-0000000000fe")
	f, err := Carica(context.Background(), p, Richiesta{Messaggi: []uuid.UUID{s.fuori}, Clienti: []uuid.UUID{assente}})
	if err != nil {
		t.Fatalf("un cliente assente non è un errore (T-DQ104-1): %v", err)
	}
	if len(f.Clienti) != 0 || len(f.FuoriRFQ) != 1 {
		t.Errorf("cliente assente: clienti %v, %d messaggi fuori RFQ", clientiDi(f), len(f.FuoriRFQ))
	}
}

// TestDQ104LeStesseQuery (A1c-L1-04 resta vera sul database; nessun SQL nuovo): con Clienti il registro ha le stesse
// query, nello stesso ordine, con una sola ListClientiDellaFotografia, e nessun testo vietato.
func TestDQ104LeStesseQuery(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScena(t, p)
	acme2 := clienteSenzaThread(t, s)
	ctx := context.Background()
	pr, reg := testutil.PoolConRegistro(t, testutil.DSN(t))
	r := Richiesta{Thread: []uuid.UUID{s.thread}, Messaggi: []uuid.UUID{s.fuori}}
	reg.Azzera()
	if _, err := Carica(ctx, pr, r); err != nil {
		t.Fatal(err)
	}
	controllaTransazione(t, reg)
	prima := nomiDelleQuery(reg)
	r.Clienti = []uuid.UUID{acme2}
	reg.Azzera()
	if _, err := Carica(ctx, pr, r); err != nil {
		t.Fatal(err)
	}
	controllaTransazione(t, reg)
	dopo := nomiDelleQuery(reg)
	if !reflect.DeepEqual(prima, dopo) {
		t.Errorf("le query cambiano con Clienti:\nprima %v\ndopo  %v", prima, dopo)
	}
	n := 0
	for _, q := range dopo {
		if q == "ListClientiDellaFotografia" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("ListClientiDellaFotografia chiamata %d volte, attesa una", n)
	}
}
