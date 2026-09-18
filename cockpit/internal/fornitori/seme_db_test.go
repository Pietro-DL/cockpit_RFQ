//go:build integrazione

// L4 — blocco 7A: il seme dei fornitori (CP7, CP14) e i vincoli della 0014 sul database vero (CP13).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/fornitori/
package fornitori

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/testutil"
)

type banco struct {
	t    *testing.T
	ctx  context.Context
	pool *pgxpool.Pool
	q    *db.Queries
}

func prepara(t *testing.T) *banco {
	t.Helper()
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	return &banco{t: t, ctx: context.Background(), pool: pool, q: db.New(pool)}
}

func (b *banco) cliente(cartella string) uuid.UUID {
	b.t.Helper()
	c, err := b.q.InsertCliente(b.ctx, db.InsertClienteParams{CartellaNas: cartella, RagioneSociale: cartella + " S.p.A.", Regole: json.RawMessage("{}")})
	if err != nil {
		b.t.Fatal(err)
	}
	return c.ClienteID
}

func (b *banco) fornitore(nome string, lavorazioni ...string) uuid.UUID {
	b.t.Helper()
	f, err := b.q.InsertFornitore(b.ctx, db.InsertFornitoreParams{RagioneSociale: nome, Tipo: db.TipoFornitoreProcessi})
	if err != nil {
		b.t.Fatal(err)
	}
	for _, l := range lavorazioni {
		if _, err := b.q.InsertLavorazioneFornitore(b.ctx, db.InsertLavorazioneFornitoreParams{FornitoreID: f.FornitoreID, Lavorazione: l}); err != nil {
			b.t.Fatal(err)
		}
	}
	return f.FornitoreID
}

const semeDiProva = `{"fornitori": [
  {"ragione_sociale": "Euroforesi", "tipo": "verniciatore", "lingua": "it",
   "domini": ["euroforesi.example"],
   "contatti": [{"nome": "Ufficio", "email": "Info@Euroforesi.example", "ruolo": "commerciale"}],
   "lavorazioni": ["cataforesi", "verniciatura_polvere"],
   "qualifiche": [{"cliente": "ACME", "lavorazione": "cataforesi"},
                  {"cliente": "CLIENTE-IGNOTO", "lavorazione": "cataforesi"},
                  {"cliente": "ACME", "lavorazione": "zincatura"}]},
  {"ragione_sociale": "Galvar", "tipo": "processi", "domini": ["galvar.example"], "lavorazioni": ["zincatura", "lavorazione-inventata"]},
  {"ragione_sociale": "Ideal System", "tipo": "verniciatore"}
]}`

// CP7 / CP14 — anteprima senza scritture, conferma che scrive solo il risolto, secondo import
// idempotente, nomi non risolti segnalati e non inventati.
func TestCP7CP14IlSemeSiVedePrimaEScriveSoloIlRisolto(t *testing.T) {
	b := prepara(t)
	acme := b.cliente("ACME")
	seme, err := Leggi(strings.NewReader(semeDiProva))
	if err != nil {
		t.Fatal(err)
	}

	ant, err := Calcola(b.ctx, b.q, seme)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ant.FornitoriDaCreare, ","); got != "Euroforesi,Galvar,Ideal System" {
		t.Errorf("da creare: %s", got)
	}
	if testutil.Conta(t, b.pool, "fornitore") != 0 {
		t.Fatal("Calcola ha scritto")
	}
	nonRisolti := func(a Anteprima) string {
		var s []string
		for _, r := range a.NonRisolti {
			s = append(s, r.String())
		}
		return strings.Join(s, "\n")
	}
	nr := nonRisolti(ant)
	for _, atteso := range []string{"CLIENTE-IGNOTO", "zincatura", "lavorazione-inventata"} {
		if !strings.Contains(nr, atteso) {
			t.Errorf("i non risolti non citano %q:\n%s", atteso, nr)
		}
	}
	if len(ant.NonRisolti) != 3 {
		t.Errorf("non risolti: %d, attesi 3 (cliente ignoto, qualifica su capacità non dichiarata, lavorazione fuori catalogo):\n%s", len(ant.NonRisolti), nr)
	}

	fatto, err := Applica(b.ctx, b.pool, seme)
	if err != nil {
		t.Fatal(err)
	}
	if len(fatto.FornitoriDaCreare) != 3 || len(fatto.NonRisolti) != 3 {
		t.Errorf("Applica ha riportato %d creati e %d non risolti", len(fatto.FornitoriDaCreare), len(fatto.NonRisolti))
	}
	if n := testutil.Conta(t, b.pool, "fornitore"); n != 3 {
		t.Errorf("fornitori: %d, attesi 3", n)
	}
	euro, err := b.q.GetFornitorePerRagioneSociale(b.ctx, "euroforesi")
	if err != nil {
		t.Fatal(err)
	}
	if euro.Tipo != db.TipoFornitoreVerniciatore || euro.Lingua.String != "it" {
		t.Errorf("Euroforesi: %+v", euro)
	}
	con, _ := b.q.ListContattiFornitore(b.ctx, euro.FornitoreID)
	if len(con) != 1 || con[0].Email != "info@euroforesi.example" {
		t.Errorf("contatto non normalizzato: %+v", con)
	}
	q, _ := b.q.ListQualificheCliente(b.ctx, acme)
	if len(q) != 1 || q[0].Lavorazione != "cataforesi" {
		t.Errorf("qualifiche di ACME: %+v (attesa la sola cataforesi: la zincatura non è fra le capacità di Euroforesi)", q)
	}
	galvar, _ := b.q.GetFornitorePerRagioneSociale(b.ctx, "galvar")
	if lav, _ := b.q.ListLavorazioniFornitore(b.ctx, galvar.FornitoreID); len(lav) != 1 || lav[0].Codice != "zincatura" {
		t.Errorf("Galvar: %+v (la lavorazione inventata non si scrive)", lav)
	}

	// secondo import: niente da scrivere, niente duplicato, i non risolti restano tali
	di_nuovo, err := Applica(b.ctx, b.pool, seme)
	if err != nil {
		t.Fatal(err)
	}
	if !di_nuovo.Vuota() || len(di_nuovo.NonRisolti) != 3 {
		t.Errorf("il secondo import doveva essere vuoto con gli stessi non risolti: %+v", di_nuovo)
	}
	for tab, atteso := range map[string]int{"fornitore": 3, "dominio_fornitore": 2, "contatto_fornitore": 1, "fornitore_lavorazione": 3, "cliente_fornitore_lavorazione": 1} {
		if n := testutil.Conta(t, b.pool, tab); n != atteso {
			t.Errorf("%s: %d righe, attese %d", tab, n, atteso)
		}
	}

	// un fornitore che c'è già con un tipo diverso: il seme NON lo cambia, ma lo dice
	altro, _ := Leggi(strings.NewReader(`{"fornitori": [{"ragione_sociale": "GALVAR", "tipo": "verniciatore", "note": "x"}]}`))
	ant, _ = Calcola(b.ctx, b.q, altro)
	if len(ant.FornitoriPresenti) != 1 || len(ant.Avvisi) == 0 {
		t.Errorf("il fornitore presente con tipo diverso: presenti %d, avvisi %d", len(ant.FornitoriPresenti), len(ant.Avvisi))
	}
	if g, _ := b.q.GetFornitorePerRagioneSociale(b.ctx, "galvar"); g.Tipo != db.TipoFornitoreProcessi {
		t.Errorf("il seme ha cambiato il tipo di Galvar")
	}
}

// Il file si convalida prima di guardare il database: tipo sconosciuto, ragione sociale vuota,
// doppione, chiave sconosciuta, contatto senza chiocciola.
func TestIlSemeRifiutaIlFileSbagliato(t *testing.T) {
	casi := map[string]string{
		"tipo sconosciuto":   `{"fornitori": [{"ragione_sociale": "X", "tipo": "boh"}]}`,
		"senza nome":         `{"fornitori": [{"ragione_sociale": " ", "tipo": "processi"}]}`,
		"doppione":           `{"fornitori": [{"ragione_sociale": "X", "tipo": "processi"}, {"ragione_sociale": "x", "tipo": "processi"}]}`,
		"chiave sconosciuta": `{"fornitori": [{"ragione_sociale": "X", "tipo": "processi", "celle_verdi": true}]}`,
		"email senza @":      `{"fornitori": [{"ragione_sociale": "X", "tipo": "processi", "contatti": [{"email": "x"}]}]}`,
		"vuoto":              `{"fornitori": []}`,
	}
	for nome, testo := range casi {
		if _, err := Leggi(strings.NewReader(testo)); err == nil {
			t.Errorf("%s: accettato", nome)
		}
	}
	// Un file giusto salvato da Windows con la firma UTF-8 in testa e un file giusto: i tre byte
	// invisibili non sono un motivo per rifiutarlo con un messaggio che nessuno sa leggere (7B.5).
	buono := `{"fornitori": [{"ragione_sociale": "X", "tipo": "processi"}]}`
	for nome, testo := range map[string]string{"senza BOM": buono, "con BOM": "\ufeff" + buono} {
		s, err := Leggi(strings.NewReader(testo))
		if err != nil {
			t.Errorf("%s: rifiutato (%v)", nome, err)
		} else if len(s.Fornitori) != 1 {
			t.Errorf("%s: letti %d fornitori", nome, len(s.Fornitori))
		}
	}
}

// CP13 — i vincoli della 0014 sul database di test: ciò che il Go rifiuta, il database lo rifiuta
// anche da solo, e cancellare la convenzione porta via le figlie.
func TestCP13IVincoliDella0014(t *testing.T) {
	b := prepara(t)
	acme := b.cliente("ACME")
	galvar := b.fornitore("Galvar", "zincatura")
	rifiuta := func(nome, sql string, args ...any) {
		t.Helper()
		if _, err := b.pool.Exec(b.ctx, sql, args...); err == nil {
			t.Errorf("%s: il database ha accettato", nome)
		}
	}
	ins := `INSERT INTO convenzione_codice (cliente_id, modo, espressione, esempio, controesempio, descrizione) VALUES ($1, $2, $3, $4, $5, 'd')`
	rifiuta("esempio vuoto", ins, acme, "suffisso", "-ZN", " ", nil)
	rifiuta("controesempio = esempio", ins, acme, "suffisso", "-ZN", "AB-ZN", "AB-ZN")
	rifiuta("suffisso con spazio", ins, acme, "suffisso", "- ZN", "AB- ZN", nil)
	rifiuta("suffisso troppo lungo", ins, acme, "suffisso", "-ABCDEFGHIJKLM", "X-ABCDEFGHIJKLM", nil)
	rifiuta("espressione vuota", ins, acme, "regex", "  ", "X", nil)
	var cid uuid.UUID
	if err := b.pool.QueryRow(b.ctx, ins+" RETURNING convenzione_id", acme, "suffisso", "-ZN", "AB-ZN", "AB-ZV").Scan(&cid); err != nil {
		t.Fatal("la convenzione buona:", err)
	}
	rifiuta("stessa espressione due volte", ins, acme, "suffisso", "-ZN", "CD-ZN", nil)
	rifiuta("lavorazione inesistente", `INSERT INTO convenzione_codice_lavorazione (convenzione_id, lavorazione) VALUES ($1, 'inventata')`, cid)
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO convenzione_codice_lavorazione (convenzione_id, lavorazione) VALUES ($1, 'zincatura')`, cid); err != nil {
		t.Fatal(err)
	}
	rifiuta("qualifica senza capacità", `INSERT INTO cliente_fornitore_lavorazione (cliente_id, fornitore_id, lavorazione) VALUES ($1, $2, 'cataforesi')`, acme, galvar)
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO cliente_fornitore_lavorazione (cliente_id, fornitore_id, lavorazione) VALUES ($1, $2, 'zincatura')`, acme, galvar); err != nil {
		t.Fatal("la qualifica buona:", err)
	}
	rifiuta("capacità con qualifica sopra", `DELETE FROM fornitore_lavorazione WHERE fornitore_id = $1 AND lavorazione = 'zincatura'`, galvar)
	rifiuta("dominio con chiocciola", `INSERT INTO dominio_fornitore (dominio, fornitore_id) VALUES ('x@y.example', $1)`, galvar)
	rifiuta("dominio maiuscolo", `INSERT INTO dominio_fornitore (dominio, fornitore_id) VALUES ('Galvar.example', $1)`, galvar)
	rifiuta("ragione sociale doppia a meno delle maiuscole", `INSERT INTO fornitore (ragione_sociale, tipo) VALUES ('GALVAR', 'processi')`)
	var conv uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'c', now()) RETURNING conversazione_id`).Scan(&conv); err != nil {
		t.Fatal(err)
	}
	msg := `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, controparte_tipo, controparte_fornitore_id, controparte_cliente_id) VALUES ('outlook', $1, $2, 'entrata', now(), $3, $4, $5)`
	rifiuta("controparte cliente con id fornitore", msg, "m1", conv, "cliente", galvar, nil)
	rifiuta("controparte fornitore senza id", msg, "m2", conv, "fornitore", nil, nil)
	rifiuta("controparte sconosciuta con un id", msg, "m3", conv, "sconosciuto", galvar, nil)
	if _, err := b.pool.Exec(b.ctx, msg, "m4", conv, "fornitore", galvar, nil); err != nil {
		t.Fatal("la controparte coerente:", err)
	}

	// la cascata
	if _, err := b.pool.Exec(b.ctx, `DELETE FROM convenzione_codice WHERE convenzione_id = $1`, cid); err != nil {
		t.Fatal(err)
	}
	if n := testutil.Conta(t, b.pool, "convenzione_codice_lavorazione"); n != 0 {
		t.Errorf("le figlie sono rimaste: %d", n)
	}
}
