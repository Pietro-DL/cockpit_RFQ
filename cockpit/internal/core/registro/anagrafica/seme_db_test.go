//go:build integrazione

// L4 — il seme scrive quello che manca e non tocca quello che c'è (voce 6.6).
package anagrafica

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

func banco(t *testing.T) (context.Context, *db.Queries, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	return context.Background(), db.New(pool), pool
}

// Il seme crea ciò che non c'è: clienti, domini e buyer, con peso e regole.
func TestIlSemeCreaCioCheManca(t *testing.T) {
	ctx, q, pool := banco(t)
	seme, err := Leggi(strings.NewReader(semeBuono))
	if err != nil {
		t.Fatal(err)
	}
	e, err := Semina(ctx, pool, seme)
	if err != nil {
		t.Fatal(err)
	}
	if len(e.ClientiCreati) != 2 || e.DominiAggiunti != 2 || e.BuyerCreati != 1 {
		t.Fatalf("resoconto: %+v", e)
	}
	c, err := q.GetClientePerCartella(ctx, "ACME")
	if err != nil {
		t.Fatal(err)
	}
	if c.Peso != 12 || c.Lingua.String != "it" {
		t.Errorf("peso/lingua non seminati: peso=%d lingua=%q", c.Peso, c.Lingua.String)
	}
	if !strings.Contains(string(c.Regole), "AC12345B") {
		t.Errorf("regole non seminate: %s", c.Regole)
	}
	// l'email del buyer arriva in minuscolo: la colonna ha un CHECK, e un seme che scrive
	// MARIO.ROSSI@… fallirebbe a metà file invece che al primo controllo
	if b, err := q.GetBuyerPerEmail(ctx, "mario.rossi@acme.example"); err != nil {
		t.Errorf("buyer non trovato per email minuscola: %v", err)
	} else if b.Origine != db.OrigineAnagraficaExcel {
		t.Errorf("origine del buyer: %q (dovrebbe dire che viene dal foglio)", b.Origine)
	}
}

// E rilanciato non tocca niente. È il caso che conta: un seme si rilancia per abitudine, e il
// database è dove qualcuno ha già corretto a mano quello che il foglio sbagliava. Un seme che
// «aggiorna» quelle correzioni le cancella, una volta per ogni lancio, in silenzio.
func TestIlSemeRilanciatoNonSovrascriveLeCorrezioniFatteAMano(t *testing.T) {
	ctx, q, pool := banco(t)
	seme, err := Leggi(strings.NewReader(semeBuono))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Semina(ctx, pool, seme); err != nil {
		t.Fatal(err)
	}
	c, _ := q.GetClientePerCartella(ctx, "ACME")

	// l'amministratore corregge dal Cockpit: ragione sociale, peso e regole
	if _, err := q.UpdateCliente(ctx, db.UpdateClienteParams{
		ClienteID: c.ClienteID, RagioneSociale: "ACME Industries S.p.A.", Profilo: c.Profilo,
		Lingua: c.Lingua, PortaleUrl: c.PortaleUrl, PortaleNote: c.PortaleNote, Peso: 3, Attivo: true, Note: c.Note,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.SetRegoleCliente(ctx, db.SetRegoleClienteParams{ClienteID: c.ClienteID, Regole: []byte(`{"lingua_risposta":"en"}`)}); err != nil {
		t.Fatal(err)
	}

	e, err := Semina(ctx, pool, seme)
	if err != nil {
		t.Fatal(err)
	}
	if len(e.ClientiCreati) != 0 || len(e.ClientiPresenti) != 2 {
		t.Fatalf("il secondo lancio ha creato qualcosa: %+v", e)
	}
	dopo, _ := q.GetClientePerCartella(ctx, "ACME")
	if dopo.RagioneSociale != "ACME Industries S.p.A." || dopo.Peso != 3 {
		t.Errorf("il seme ha riscritto le correzioni: %q peso=%d", dopo.RagioneSociale, dopo.Peso)
	}
	// `jsonb` normalizza la punteggiatura (`{"a": 1}`, con lo spazio): si confronta il CONTENUTO,
	// non la stringa. Quello che conta e' che le regole del file non siano tornate al loro posto.
	if !strings.Contains(string(dopo.Regole), "en") || strings.Contains(string(dopo.Regole), "AC12345B") {
		t.Errorf("il seme ha riscritto le regole corrette a mano: %s", dopo.Regole)
	}
}

// Un dominio già di un altro cliente non viene spostato: il seme lo segnala e va avanti. Fermarsi
// lascerebbe il database a metà; spostarlo cambierebbe il cliente di tutta la posta già arrivata.
func TestIlSemeNonRubaUnDominioAUnAltroCliente(t *testing.T) {
	ctx, q, pool := banco(t)
	primo, err := q.InsertCliente(ctx, db.InsertClienteParams{
		CartellaNas: "GIA-PRESENTE", RagioneSociale: "Cliente Preesistente", Regole: []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.InsertDominioCliente(ctx, db.InsertDominioClienteParams{Lower: "acme.example", ClienteID: primo.ClienteID}); err != nil {
		t.Fatal(err)
	}

	seme, err := Leggi(strings.NewReader(semeBuono))
	if err != nil {
		t.Fatal(err)
	}
	e, err := Semina(ctx, pool, seme)
	if err != nil {
		t.Fatalf("il seme si è fermato invece di segnalare: %v", err)
	}
	if len(e.ClientiCreati) != 2 {
		t.Errorf("i clienti non sono stati creati: %+v", e)
	}
	if len(e.Avvisi) != 1 || !strings.Contains(e.Avvisi[0], "Cliente Preesistente") {
		t.Fatalf("nessun avviso sul dominio conteso: %+v", e.Avvisi)
	}
	c, err := q.GetClientePerDominio(ctx, "acme.example")
	if err != nil {
		t.Fatal(err)
	}
	if c.ClienteID != primo.ClienteID {
		t.Errorf("il dominio è passato a %s", c.RagioneSociale)
	}
}
