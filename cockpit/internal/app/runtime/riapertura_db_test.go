//go:build integrazione

// L4 — il comando U5 dalla riga di comando (Smistamento F7, addendum A5.15.2, P38): il database giusto prima
// di tutto. La prima riga dice il database di destinazione, senza password e con il file assoluto; con un nome
// diverso da quello del DSN il comando esce senza scrivere; non migra uno schema vecchio (ne' l'anteprima ne'
// l'applicazione); l'anteprima scrive il rapporto JSON e non il database; l'applicazione riapre e una seconda
// non trova niente. La logica delle forme e le RFQ congelate o in revisione sono provate in core/rfq/fascicolo.

package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/migrazioni"
	"promatec/cockpit/internal/platform/testutil"
)

// scenaU5 e' una RFQ aperta con un nodo agganciato per codice prima dello Smistamento (F-A) e l'arco chiuso in
// automatico che lo tocca (F-B). Restituisce la RFQ e il nodo.
func scenaU5(t *testing.T, p *pgxpool.Pool) (thread, nodo uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	riga := func(sql string, dst any, arg ...any) {
		t.Helper()
		if err := p.QueryRow(ctx, sql, arg...).Scan(dst); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	var utente, cliente, comp, conv, msg, allegato uuid.UUID
	riga(`INSERT INTO utente (sigla, nome, ufficio) VALUES ('U5', 'Prova U5', 'Tecnico') RETURNING utente_id`, &utente)
	riga(`INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME', 'ACME') RETURNING cliente_id`, &cliente)
	riga(`INSERT INTO thread_offerta (cliente_id, canale, data_inizio, cartella_relativa) VALUES ($1, 'outlook', now(), 'ACME\WIP\u5')
		RETURNING thread_id`, &thread, cliente)
	riga(`INSERT INTO componente (thread_id, codice, tipo, confermato_da) VALUES ($1, '7120010', 'sottoassieme', $2) RETURNING componente_id`, &comp, thread, utente)
	riga(`INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'C-U5', now()) RETURNING conversazione_id`, &conv)
	riga(`INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, thread_id)
		VALUES ('outlook', '<u5@acme.example>', $1, 'entrata', now(), $2) RETURNING messaggio_id`, &msg, conv, thread)
	sha := strings.Repeat("8", 64)
	riga(`INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, ricevuto_il)
		VALUES ($1, 1, '7120001A_1.stp', 'stp', 'file', 'outlook', 100, $2, now()) RETURNING allegato_id`, &allegato, msg, sha)
	var radice uuid.UUID
	riga(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, codice, fonte, confidenza)
		VALUES ($1, $2, $3, '#1', '7120001', '7120001', 'step', 60) RETURNING proposta_id`, &radice, thread, allegato, sha)
	riga(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, codice, fonte, confidenza, stato, componente_id, deciso_il)
		VALUES ($1, $2, $3, '#2', '7120010', '7120010', 'step', 60, 'duplicato', $4, now() - interval '30 days') RETURNING proposta_id`,
		&nodo, thread, allegato, sha, comp)
	var n int
	riga(`WITH x AS (INSERT INTO relazione_proposta (thread_id, allegato_id, padre_chiave, figlio_chiave, stato, deciso_il)
		VALUES ($1, $2, '#1', '#2', 'duplicato', now() - interval '30 days') RETURNING 1) SELECT count(*) FROM x`, &n, thread, allegato)
	return thread, nodo
}

// improntaU5 sono le righe delle tabelle che il comando potrebbe scrivere, e lo schema applicato.
func improntaU5(t *testing.T, p *pgxpool.Pool) string {
	t.Helper()
	var s string
	if err := p.QueryRow(context.Background(), `SELECT concat_ws(' # ',
		(SELECT string_agg(x::text, '|' ORDER BY x::text) FROM componente_proposta x),
		(SELECT string_agg(x::text, '|' ORDER BY x::text) FROM relazione_proposta x),
		(SELECT string_agg(x::text, '|' ORDER BY x::text) FROM componente x),
		(SELECT string_agg(x::text, '|' ORDER BY x::text) FROM thread_offerta x),
		(SELECT count(*)::text FROM analizzatore_corrente))`).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func statoDelNodo(t *testing.T, p *pgxpool.Pool, nodo uuid.UUID) string {
	t.Helper()
	var s string
	if err := p.QueryRow(context.Background(), `SELECT stato::text || ':' || (componente_id IS NULL)::text FROM componente_proposta WHERE proposta_id = $1`,
		nodo).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// Il database giusto, prima di tutto (P38): con un nome diverso da quello del DSN il comando esce con un errore
// (il codice d'uscita non e' zero) prima di collegarsi, e il database non cambia. La prima riga stampata e' il
// database di destinazione, senza password, con il percorso assoluto del file.
func TestIlComandoSulDatabaseSbagliatoNonScrive(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	_, nodo := scenaU5(t, p)
	cfgFile, _ := tomlDiProva(t)
	prima := improntaU5(t, p)

	// dalla riga di comando: Esegui torna l'errore, main lo stampa ed esce con 1
	err := Esegui(cfgFile, config.Rete{}, Opzioni{RiapriAgganci: true, DatabaseRiapertura: "cockpit_altro_test"})
	nome := p.Config().ConnConfig.Database
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("il file punta a %q, hai scritto %q", nome, "cockpit_altro_test")) {
		t.Fatalf("un nome sbagliato: %v, atteso il rifiuto", err)
	}
	if got := improntaU5(t, p); got != prima {
		t.Errorf("con il nome sbagliato il database e' cambiato")
	}
	if s := statoDelNodo(t, p, nodo); s != "duplicato:false" {
		t.Errorf("il nodo: %s", s)
	}

	// la stampa: la prima riga e' il database, e dopo il rifiuto non c'e' altro
	cfg, err := config.Carica(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	err = RiapriAgganci(context.Background(), cfg, cfgFile, &b, Opzioni{RiapriAgganci: true, DatabaseRiapertura: "cockpit_altro_test"})
	if err == nil {
		t.Fatalf("un nome sbagliato non si rifiuta")
	}
	righe := strings.Split(strings.TrimSpace(b.String()), "\n")
	cc := p.Config().ConnConfig
	abs, _ := filepath.Abs(cfgFile)
	atteso := fmt.Sprintf("database di destinazione: %s:%d/%s come %s (dal file %s)", cc.Host, cc.Port, cc.Database, cc.User, abs)
	if len(righe) != 1 || righe[0] != atteso {
		t.Errorf("stampa con il nome sbagliato:\n%s\natteso:\n%s", b.String(), atteso)
	}
	// la riga e' quella attesa, quindi senza password; e nessuna forma «utente:password@» del DSN
	if cc.Password != "" && (strings.Contains(b.String(), ":"+cc.Password+"@") || strings.Contains(b.String(), "password")) {
		t.Errorf("la stampa dice la password")
	}
}

// Un rapporto che non si puo' scrivere ferma l'applicazione prima di qualunque RFQ (A5.15.2: il JSON si scrive
// man mano): con -uscita dentro un file che non e' una cartella il comando esce con un errore (il codice
// d'uscita non e' zero) e il database non cambia, nemmeno la prima RFQ.
func TestIlComandoConUnRapportoNonScrivibileNonScrive(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	_, nodo := scenaU5(t, p)
	cfgFile, _ := tomlDiProva(t)
	cfg, err := config.Carica(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	prima := improntaU5(t, p)
	unFile := filepath.Join(t.TempDir(), "non-una-cartella")
	if err := os.WriteFile(unFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	nome := p.Config().ConnConfig.Database
	var b bytes.Buffer
	err = RiapriAgganci(context.Background(), cfg, cfgFile, &b, Opzioni{RiapriAgganci: true, DatabaseRiapertura: nome,
		UscitaRiapertura: filepath.Join(unFile, "u5.json")})
	if err == nil || !strings.Contains(err.Error(), "nessuna RFQ toccata") || !strings.Contains(err.Error(), "rapporto") {
		t.Fatalf("un rapporto che non si scrive: %v, atteso che il comando si fermi prima delle RFQ", err)
	}
	if got := improntaU5(t, p); got != prima {
		t.Errorf("con il rapporto che non si scrive il database e' cambiato")
	}
	if s := statoDelNodo(t, p, nodo); s != "duplicato:false" {
		t.Errorf("il nodo: %s", s)
	}
}

// Il comando U5 non migra (P38): con uno schema piu' vecchio di quello del binario l'anteprima e l'applicazione
// si fermano, dicono backup e -migra, e lo schema resta dov'era.
func TestIlComandoU5NonMigra(t *testing.T) {
	p := testutil.Pool(t)
	ctx := context.Background()
	ultima := ultimaDelBinario(t)
	testutil.SchemaFinoA(t, p, ultima-1)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	cfgFile, _ := tomlDiProva(t)
	cfg, err := config.Carica(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	nome := p.Config().ConnConfig.Database
	for _, o := range []Opzioni{{RiapriAgganci: true}, {RiapriAgganci: true, DatabaseRiapertura: nome}} {
		var b bytes.Buffer
		err := RiapriAgganci(ctx, cfg, cfgFile, &b, o)
		if err == nil || !strings.Contains(err.Error(), "-migra") || !strings.Contains(err.Error(), "backup") {
			t.Errorf("%+v su uno schema vecchio: %v, atteso che si fermi e dica backup e -migra", o, err)
		}
		if !strings.HasPrefix(b.String(), "database di destinazione: ") {
			t.Errorf("%+v: la prima riga non e' il database: %q", o, b.String())
		}
		applicate, err := migrazioni.Applicate(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		if v := ultimaApplicata(applicate); v != ultima-1 {
			t.Fatalf("%+v ha portato lo schema alla %d: il comando non migra", o, v)
		}
	}
}

// Il comando intero, sul database di prova: l'anteprima stampa prima il database, poi che il pool e' in sola
// lettura (default_transaction_read_only del pool di ApriDatabaseInLettura, non la transazione ReadOnly del
// core: con un pool scrivibile l'anteprima si ferma), scrive il rapporto JSON (nella cartella del log) e non il
// database, e finisce dicendo come si applica e quando (solo con lo Smistamento F10/F11 in produzione, dopo il
// backup); l'applicazione con il nome giusto dice il database collegato (current_database()), riapre il nodo e
// l'arco; una seconda applicazione non trova niente.
func TestIlComandoU5DallaRigaDiComando(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	thread, nodo := scenaU5(t, p)
	cfgFile, staging := tomlDiProva(t)
	cfg, err := config.Carica(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	nome := p.Config().ConnConfig.Database
	prima := improntaU5(t, p)

	var b bytes.Buffer
	if err := RiapriAgganci(ctx, cfg, cfgFile, &b, Opzioni{RiapriAgganci: true}); err != nil {
		t.Fatalf("anteprima: %v\n%s", err, b.String())
	}
	out := b.String()
	righe := strings.Split(strings.TrimSpace(out), "\n")
	if !strings.HasPrefix(righe[0], "database di destinazione: ") || !strings.Contains(righe[0], "/"+nome+" come ") {
		t.Errorf("la prima riga: %q", righe[0])
	}
	if len(righe) < 2 || righe[1] != "collegato in sola lettura (default_transaction_read_only=on)" {
		t.Errorf("la seconda riga dell'anteprima deve dire il pool in sola lettura:\n%s", out)
	}
	for _, frase := range []string{"modalita=anteprima", "sola_lettura=sì", "fa_nodi=1", "fb_archi=1", "rfq_nel_perimetro=1",
		"rfq=" + thread.String(), "avviso: sui dati veri si applica solo con lo Smistamento (F10/F11) in produzione"} {
		if !strings.Contains(out, frase) {
			t.Errorf("l'anteprima non dice %q:\n%s", frase, out)
		}
	}
	if ultima := righe[len(righe)-1]; ultima != "anteprima: non e' stato scritto niente. Per applicare: -riapri-agganci "+nome+
		" (sui dati veri solo con lo Smistamento F10/F11 in produzione, dopo il backup)" {
		t.Errorf("l'ultima riga: %q", ultima)
	}
	if got := improntaU5(t, p); got != prima {
		t.Errorf("l'anteprima ha scritto nel database")
	}
	// il rapporto nella cartella del log, un JSON intero
	trovati, _ := filepath.Glob(filepath.Join(staging, "log", "riapri-agganci-*.json"))
	if len(trovati) != 1 {
		t.Fatalf("il rapporto dell'anteprima: %v", trovati)
	}
	var rap fascicolo.RapportoRiapertura
	raw, err := os.ReadFile(trovati[0])
	if err != nil || json.Unmarshal(raw, &rap) != nil || rap.Modalita != "anteprima" || rap.Fine == nil || rap.Totali.Agganci != 1 || len(rap.Rfq) != 1 {
		t.Errorf("il rapporto: %v %+v", err, rap)
	}

	// l'applicazione con il nome giusto, con il rapporto dove lo si chiede
	uscita := filepath.Join(t.TempDir(), "u5.json")
	b.Reset()
	if err := RiapriAgganci(ctx, cfg, cfgFile, &b, Opzioni{RiapriAgganci: true, DatabaseRiapertura: nome, UscitaRiapertura: uscita}); err != nil {
		t.Fatalf("applicazione: %v\n%s", err, b.String())
	}
	if righe := strings.Split(b.String(), "\n"); len(righe) < 2 || righe[1] != "collegato a "+nome+" (SELECT current_database())" {
		t.Errorf("la seconda riga dell'applicazione deve dire il database collegato:\n%s", b.String())
	}
	if !strings.HasPrefix(b.String(), "database di destinazione: ") || !strings.Contains(b.String(), "nodi_riaperti=1") ||
		!strings.Contains(b.String(), "archi_riaperti=1") || strings.Contains(b.String(), "anteprima: non e' stato scritto niente") {
		t.Errorf("applicazione:\n%s", b.String())
	}
	if s := statoDelNodo(t, p, nodo); s != "aperta:true" {
		t.Errorf("il nodo dopo l'applicazione: %s", s)
	}
	raw, err = os.ReadFile(uscita)
	if err != nil || json.Unmarshal(raw, &rap) != nil || rap.Modalita != "applicazione" || rap.Totali.NodiRiaperti != 1 || rap.Totali.ArchiRiaperti != 1 {
		t.Errorf("il rapporto dell'applicazione: %v %+v", err, rap.Totali)
	}
	dopo := improntaU5(t, p)

	// una seconda applicazione: zero righe, niente cambia
	b.Reset()
	if err := RiapriAgganci(ctx, cfg, cfgFile, &b, Opzioni{RiapriAgganci: true, DatabaseRiapertura: nome, UscitaRiapertura: uscita}); err != nil {
		t.Fatalf("seconda applicazione: %v", err)
	}
	if !strings.Contains(b.String(), "\nfa_nodi=0\n") || !strings.Contains(b.String(), "\nnodi_riaperti=0\n") {
		t.Errorf("seconda applicazione:\n%s", b.String())
	}
	if got := improntaU5(t, p); got != dopo {
		t.Errorf("la seconda applicazione ha cambiato il database")
	}
}
