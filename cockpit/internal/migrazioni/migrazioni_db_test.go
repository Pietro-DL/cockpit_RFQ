//go:build integrazione

package migrazioni_test

import (
	"context"
	"strings"
	"testing"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/migrazioni"
	"promatec/cockpit/internal/testutil"
)

// S1 — da vuoto: 0001…000n in ordine; il riavvio è idempotente e non riapplica nulla.
func TestS1DaVuotoEIdempotente(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaVuoto(t, p)
	ctx := context.Background()

	n, err := migrazioni.Applica(ctx, p, risorse.FS, testutil.LogSilenzioso())
	if err != nil {
		t.Fatalf("prima applicazione: %v", err)
	}
	migs, _ := migrazioni.Elenca(risorse.FS)
	if n != len(migs) {
		t.Fatalf("applicate %d migrazioni su %d", n, len(migs))
	}
	fatte, err := migrazioni.Applicate(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migs {
		if !fatte[m.Versione] {
			t.Errorf("versione %d non registrata in schema_versione", m.Versione)
		}
	}

	n2, err := migrazioni.Applica(ctx, p, risorse.FS, testutil.LogSilenzioso())
	if err != nil {
		t.Fatalf("seconda applicazione: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("il riavvio ha riapplicato %d migrazioni: non è idempotente", n2)
	}
}

// S3 — una migrazione che fallisce a metà non lascia nulla: la transazione è annullata e la versione
// non risulta applicata.
func TestS3MigrazioneFallitaAMeta(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaVuoto(t, p)
	ctx := context.Background()

	migs := []migrazioni.Migrazione{
		{Versione: 1, Nome: "0001_base.sql", SQL: `
			CREATE TABLE schema_versione (versione int PRIMARY KEY, applicata_il timestamptz NOT NULL DEFAULT now());
			INSERT INTO schema_versione (versione) VALUES (1);`},
		{Versione: 2, Nome: "0002_rotta.sql", SQL: `
			CREATE TABLE prima_di_rompere (id int);
			SELECT 1/0;
			INSERT INTO schema_versione (versione) VALUES (2);`},
	}
	n, err := migrazioni.ApplicaElenco(ctx, p, migs, 2, testutil.LogSilenzioso())
	if err == nil {
		t.Fatal("attesa una migrazione fallita")
	}
	if n != 1 {
		t.Fatalf("applicate %d migrazioni, attesa solo la 0001", n)
	}
	var esiste bool
	if err := p.QueryRow(ctx, "SELECT to_regclass('public.prima_di_rompere') IS NOT NULL").Scan(&esiste); err != nil {
		t.Fatal(err)
	}
	if esiste {
		t.Error("la tabella creata prima dell'errore è rimasta: la migrazione non era in transazione")
	}
	fatte, _ := migrazioni.Applicate(ctx, p)
	if fatte[2] {
		t.Error("la versione 2 risulta applicata benché la migrazione sia fallita")
	}
}

// Un file che non registra la propria versione viene annullato anche se il suo SQL è valido.
func TestMigrazioneSenzaRegistrazioneAnnullata(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaVuoto(t, p)
	ctx := context.Background()

	migs := []migrazioni.Migrazione{
		{Versione: 1, Nome: "0001_base.sql", SQL: `
			CREATE TABLE schema_versione (versione int PRIMARY KEY, applicata_il timestamptz NOT NULL DEFAULT now());
			INSERT INTO schema_versione (versione) VALUES (1);`},
		// L'INSERT c'è ma non viene eseguito: la verifica statica lo vede, il DB no. È il caso che la
		// seconda cintura (controllo dentro la transazione) deve prendere.
		{Versione: 2, Nome: "0002_smemorata.sql", SQL: `
			CREATE TABLE smemorata (id int);
			DO $$ BEGIN IF false THEN INSERT INTO schema_versione (versione) VALUES (2); END IF; END $$;`},
	}
	_, err := migrazioni.ApplicaElenco(ctx, p, migs, 2, testutil.LogSilenzioso())
	if err == nil || !strings.Contains(err.Error(), "non registra la propria versione") {
		t.Fatalf("atteso rifiuto della migrazione che non si registra, ottenuto %v", err)
	}
	var esiste bool
	_ = p.QueryRow(ctx, "SELECT to_regclass('public.smemorata') IS NOT NULL").Scan(&esiste)
	if esiste {
		t.Error("la tabella è rimasta: la transazione non è stata annullata")
	}
}

// Un DB più avanti del binario non va toccato: è il caso di un cockpit.exe vecchio su un DB nuovo.
func TestDBPiuRecenteDelBinario(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()
	if _, err := p.Exec(ctx, "INSERT INTO schema_versione (versione) VALUES (99)"); err != nil {
		t.Fatal(err)
	}
	_, err := migrazioni.Applica(ctx, p, risorse.FS, testutil.LogSilenzioso())
	if err == nil || !strings.Contains(err.Error(), "aggiornare cockpit.exe") {
		t.Fatalf("atteso rifiuto con invito ad aggiornare il binario, ottenuto %v", err)
	}
}

// S2 (0002) — su un DB con dati alla versione 1: la 0002 non tocca i dati esistenti e aggiunge le
// quattro tabelle delle fondazioni.
func TestS2FondazioniSuDBConDati(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaFinoA(t, p, 1)
	ctx := context.Background()

	if _, err := p.Exec(ctx, `
		INSERT INTO utente (sigla, nome, ufficio, ruolo) VALUES ('S2', 'Utente S2', 'Test', 'operatore');
		INSERT INTO cliente (ragione_sociale, cartella_nas) VALUES ('Cliente S2 spa', 'CLIENTE S2');`); err != nil {
		t.Fatal(err)
	}
	primaUtenti := testutil.Conta(t, p, "utente")
	primaClienti := testutil.Conta(t, p, "cliente")

	n, err := migrazioni.Applica(ctx, p, risorse.FS, testutil.LogSilenzioso())
	if err != nil {
		t.Fatalf("0002: %v", err)
	}
	if n < 1 {
		t.Fatal("la 0002 non è stata applicata")
	}
	if dopo := testutil.Conta(t, p, "utente"); dopo != primaUtenti {
		t.Errorf("utenti %d → %d: la migrazione ha toccato i dati", primaUtenti, dopo)
	}
	if dopo := testutil.Conta(t, p, "cliente"); dopo != primaClienti {
		t.Errorf("clienti %d → %d: la migrazione ha toccato i dati", primaClienti, dopo)
	}
	for _, tab := range []string{"casella", "postazione", "worker_credenziale", "casella_store"} {
		var esiste bool
		if err := p.QueryRow(ctx, "SELECT to_regclass('public.'||$1) IS NOT NULL", tab).Scan(&esiste); err != nil {
			t.Fatal(err)
		}
		if !esiste {
			t.Errorf("tabella %s assente dopo la 0002", tab)
		}
	}
	// La 0002 non deve aver toccato `messaggio` (voce 0.6: nessuna modifica allo schema dei messaggi).
	var colonne int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns WHERE table_name = 'messaggio' AND column_name IN ('casella_id','interno')`).Scan(&colonne); err != nil {
		t.Fatal(err)
	}
	if colonne != 0 {
		t.Errorf("la 0002 ha aggiunto %d colonne a messaggio: spettano alla 0004", colonne)
	}
}

// I vincoli dichiarati nella 0002 devono davvero rifiutare i dati sbagliati.
func TestVincoliFondazioni(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()

	casi := []struct {
		nome, sql, atteso string
	}{
		{"indirizzo maiuscolo", `INSERT INTO casella (indirizzo, nome) VALUES ('Mario@ACME.it', 'Mario')`, "ck_casella_indirizzo_minuscolo"},
		{"condivisa con proprietario", `INSERT INTO casella (indirizzo, nome, condivisa, utente_id) VALUES ('c@acme.it','C',true,(SELECT utente_id FROM utente LIMIT 1))`, "ck_casella_condivisa"},
		{"host minuscolo", `INSERT INTO postazione (nome_host) VALUES ('pc-francesco')`, "ck_postazione_host_maiuscolo"},
		{"token non esadecimale", `INSERT INTO worker_credenziale (worker_nome, worker_tipo, token_hash) VALUES ('w','outlook','non-un-hash')`, "ck_worker_token_hash_hex"},
	}
	if _, err := p.Exec(ctx, `INSERT INTO utente (sigla, nome, ufficio, ruolo) VALUES ('VC','Vincoli','Test','operatore')`); err != nil {
		t.Fatal(err)
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			_, err := p.Exec(ctx, c.sql)
			if err == nil {
				t.Fatalf("inserimento accettato: manca il vincolo %s", c.atteso)
			}
			if !strings.Contains(err.Error(), c.atteso) {
				t.Fatalf("atteso il vincolo %s, ottenuto: %v", c.atteso, err)
			}
		})
	}
	// Due caselle con lo stesso indirizzo sullo stesso canale sono la stessa casella.
	if _, err := p.Exec(ctx, `INSERT INTO casella (indirizzo, nome) VALUES ('uguale@acme.it','Uno')`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO casella (indirizzo, nome) VALUES ('uguale@acme.it','Due')`); err == nil {
		t.Error("due caselle con lo stesso indirizzo accettate: manca ux_casella_canale_indirizzo")
	}
}
