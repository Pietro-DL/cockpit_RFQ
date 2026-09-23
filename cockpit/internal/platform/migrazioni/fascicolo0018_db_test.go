//go:build integrazione

// L4 — la 0018 (Blocco 8, B8.2): la struttura del Fascicolo.
//
// Tre gruppi di prove:
//   - le GUARDIE: su dati alla versione 17 che la migrazione non sa fondere da sola, si ferma, dice
//     quale guardia e su quali righe, e non cambia niente (la versione resta 17, padre_id c'e' ancora);
//   - la FUSIONE e il TRAVASO: sui dati che sa fondere, lo fa senza perdere riferimenti;
//   - lo schema 18: le FK composite, i CHECK, le viste, e il ritorno manuale 17 → 18 → 17 con
//     scripts/0018_indietro.sql.
//
// Nomi delle prove di A1.6.4 dell'addendum: TestUnaRelazioneFraDueRfqEImpossibile,
// TestUnDocumentoNonSiAggancaAUnComponenteDiUnAltraRfq, TestUnDocumentoAgganciatoHaIlCodiceDelComponente,
// TestLaMigrazione18NormalizzaLeMaiuscoleDeiCodici, TestCorreggereIlCodiceSenzaDeferredFallisceSubito.

package migrazioni_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/migrazioni"
	"promatec/cockpit/internal/platform/testutil"
)

// Identificativi fissi: i messaggi delle guardie li riportano, e un id leggibile dice subito quale
// riga della prova e' finita nell'elenco.
var sostituzioni18 = strings.NewReplacer(
	"{U}", "00000000-0000-0000-0000-000000000018",
	"{C}", "10000000-0000-0000-0000-000000000018",
	"{T1}", "30000000-0000-0000-0000-000000000001",
	"{T2}", "30000000-0000-0000-0000-000000000002",
	"{CV}", "40000000-0000-0000-0000-000000000001",
	"{M}", "40000000-0000-0000-0000-000000000002",
	"{A1}", "40000000-0000-0000-0000-000000000003",
	"{K1}", "50000000-0000-0000-0000-000000000001",
	"{K2}", "50000000-0000-0000-0000-000000000002",
	"{K3}", "50000000-0000-0000-0000-000000000003",
	"{K4}", "50000000-0000-0000-0000-000000000004",
	"{K5}", "50000000-0000-0000-0000-000000000005",
	"{K6}", "50000000-0000-0000-0000-000000000006",
	"{K7}", "50000000-0000-0000-0000-000000000007",
)

const base18 = `
INSERT INTO utente (utente_id, sigla, nome, ufficio) VALUES ('{U}', 'M18', 'Prova 0018', 'Test');
INSERT INTO cliente (cliente_id, cartella_nas, ragione_sociale) VALUES ('{C}', 'PROVA0018', 'Prova 0018');
INSERT INTO thread_offerta (thread_id, cliente_id, canale, data_inizio)
     VALUES ('{T1}', '{C}', 'outlook', now()), ('{T2}', '{C}', 'outlook', now());
INSERT INTO conversazione (conversazione_id, canale, chiave_esterna, primo_messaggio_il)
     VALUES ('{CV}', 'outlook', 'conv-0018', now());
INSERT INTO messaggio (messaggio_id, canale, chiave_esterna, conversazione_id, direzione, data_evento)
     VALUES ('{M}', 'outlook', '<m-0018@prova>', '{CV}', 'entrata', now());
INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, ricevuto_il)
     VALUES ('{A1}', '{M}', 1, 'D1_rev2.pdf', now());`

func esegui18(t *testing.T, p *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := p.Exec(context.Background(), sostituzioni18.Replace(sql)); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
}

// fino17 ricrea lo schema alla 17 con i dati di base e poi quelli della prova.
func fino17(t *testing.T, p *pgxpool.Pool, dati string) {
	t.Helper()
	testutil.SchemaFinoA(t, p, 17)
	esegui18(t, p, base18)
	if dati != "" {
		esegui18(t, p, dati)
	}
}

func applica18(p *pgxpool.Pool) error {
	_, err := migrazioni.ApplicaFinoA(context.Background(), p, risorse.FS, 18, testutil.LogSilenzioso())
	return err
}

func applicata(t *testing.T, p *pgxpool.Pool, v int) bool {
	t.Helper()
	fatte, err := migrazioni.Applicate(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	return fatte[v]
}

func uno[T any](t *testing.T, p *pgxpool.Pool, sql string) T {
	t.Helper()
	var v T
	if err := p.QueryRow(context.Background(), sostituzioni18.Replace(sql)).Scan(&v); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return v
}

// vincolo restituisce il nome del vincolo violato (o la colonna, per un NOT NULL).
func vincolo(err error) string {
	var pe *pgconn.PgError
	if !errors.As(err, &pe) {
		return ""
	}
	if pe.ConstraintName != "" {
		return pe.ConstraintName
	}
	if pe.Code == "23502" {
		return "not null " + pe.ColumnName
	}
	return pe.Code
}

// ------------------------------------------------------------------ guardie

var lettere18 = []string{"a", "a2", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l"}

func TestLe13GuardieDellaMigrazione18(t *testing.T) {
	casi := []struct {
		guardia, nome, dati string
	}{
		{"a", "stesso codice, revisioni note diverse", `
			INSERT INTO componente (componente_id, thread_id, codice, rev, confermato_da)
			     VALUES ('{K1}', '{T1}', 'X1', 'A', '{U}'), ('{K2}', '{T1}', 'x1', 'B', '{U}');`},
		{"a2", "stesso codice, tipo diverso", `
			INSERT INTO componente (componente_id, thread_id, codice, tipo, confermato_da)
			     VALUES ('{K1}', '{T1}', 'X2', 'sciolto', '{U}'), ('{K2}', '{T1}', 'x2', 'finito', '{U}');`},
		{"a2", "stesso codice, descrizione diversa", `
			INSERT INTO componente (componente_id, thread_id, codice, descrizione, confermato_da)
			     VALUES ('{K1}', '{T1}', 'X3', 'staffa', '{U}'), ('{K2}', '{T1}', 'x3', 'supporto', '{U}');`},
		{"a2", "stesso codice, due radici con qta diverse", `
			INSERT INTO componente (componente_id, thread_id, codice, qta, confermato_da)
			     VALUES ('{K1}', '{T1}', 'X4', 1, '{U}'), ('{K2}', '{T1}', 'x4', 5, '{U}');`},
		{"b", "documento agganciato a un componente di un'altra RFQ", `
			INSERT INTO componente (componente_id, thread_id, codice, confermato_da) VALUES ('{K1}', '{T1}', 'B1', '{U}');
			INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
			     VALUES ('{T2}', '{K1}', 'capitolato', 'B1', 'b1.pdf', 'pdf', repeat('1', 64), 'CAPITOLATI\b1.pdf', '{U}');`},
		{"b", "padre in un'altra RFQ", `
			INSERT INTO componente (componente_id, thread_id, codice, confermato_da) VALUES ('{K1}', '{T1}', 'P1', '{U}');
			INSERT INTO componente (componente_id, thread_id, padre_id, codice, confermato_da) VALUES ('{K2}', '{T2}', '{K1}', 'F1', '{U}');`},
		{"c", "documento agganciato con un altro codice", `
			INSERT INTO componente (componente_id, thread_id, codice, confermato_da) VALUES ('{K1}', '{T1}', 'C1', '{U}');
			INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
			     VALUES ('{T1}', '{K1}', 'capitolato', 'C9', 'c.pdf', 'pdf', repeat('1', 64), 'CAPITOLATI\c.pdf', '{U}');`},
		{"d", "proposta con componente ma senza thread", `
			INSERT INTO componente (componente_id, thread_id, codice, confermato_da) VALUES ('{K1}', '{T1}', 'D1', '{U}');
			INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte)
			     VALUES ('{A1}', NULL, 'disegno_2d', 'D1', '{K1}', 50, 'nome_file');`},
		{"e", "documento agganciato senza codice", `
			INSERT INTO componente (componente_id, thread_id, codice, confermato_da) VALUES ('{K1}', '{T1}', 'E1', '{U}');
			INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
			     VALUES ('{T1}', '{K1}', 'capitolato', NULL, 'e.pdf', 'pdf', repeat('1', 64), 'CAPITOLATI\e.pdf', '{U}');`},
		{"f", "componente senza chi l'ha confermato", `
			INSERT INTO componente (componente_id, thread_id, codice) VALUES ('{K1}', '{T1}', 'F1');`},
		{"g", "componente con codice di soli spazi", `
			INSERT INTO componente (componente_id, thread_id, codice, confermato_da) VALUES ('{K1}', '{T1}', '   ', '{U}');`},
		{"h", "disegno confermato senza codice", `
			INSERT INTO documento (thread_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
			     VALUES ('{T1}', 'disegno_2d', NULL, 'h.pdf', 'pdf', repeat('1', 64), 'ELENCO DISEGNI\h.pdf', '{U}');`},
		{"i", "la fusione metterebbe un pezzo dentro se stesso", `
			INSERT INTO componente (componente_id, thread_id, codice, confermato_da) VALUES ('{K1}', '{T1}', 'I1', '{U}');
			INSERT INTO componente (componente_id, thread_id, padre_id, codice, confermato_da) VALUES ('{K2}', '{T1}', '{K1}', 'i1', '{U}');`},
		{"j", "la fusione unirebbe due archi con qta diverse", `
			INSERT INTO componente (componente_id, thread_id, codice, confermato_da, creato_il)
			     VALUES ('{K1}', '{T1}', 'J1', '{U}', now() - interval '2 hours'), ('{K2}', '{T1}', 'j1', '{U}', now() - interval '1 hour');
			INSERT INTO componente (componente_id, thread_id, padre_id, codice, qta, confermato_da, creato_il)
			     VALUES ('{K3}', '{T1}', '{K1}', 'JC', 1, '{U}', now() - interval '2 hours'),
			            ('{K4}', '{T1}', '{K2}', 'jc', 2, '{U}', now() - interval '1 hour');`},
		{"k", "un ciclo nei padre_id", `
			INSERT INTO componente (componente_id, thread_id, codice, confermato_da) VALUES ('{K1}', '{T1}', 'K1', '{U}');
			INSERT INTO componente (componente_id, thread_id, padre_id, codice, confermato_da) VALUES ('{K2}', '{T1}', '{K1}', 'K2', '{U}');
			UPDATE componente SET padre_id = '{K2}' WHERE componente_id = '{K1}';`},
		{"l", "due deroghe per lo stesso pezzo dopo la fusione", `
			INSERT INTO componente (componente_id, thread_id, codice, confermato_da) VALUES ('{K1}', '{T1}', 'L1', '{U}'), ('{K2}', '{T1}', 'l1', '{U}');
			INSERT INTO deroga_fabbisogno (thread_id, componente_id, tipo, motivo, utente_id)
			     VALUES ('{T1}', '{K1}', 'disegno_2d', 'lo facciamo noi', '{U}'), ('{T1}', '{K2}', 'disegno_2d', 'il cliente lo manda dopo', '{U}');`},
	}
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	coperte := map[string]bool{}
	for _, c := range casi {
		t.Run(c.guardia+" "+c.nome, func(t *testing.T) {
			fino17(t, p, c.dati)
			prima := testutil.Conta(t, p, "componente")

			err := applica18(p)
			if err == nil {
				t.Fatal("la 0018 e' passata: la guardia doveva fermarla")
			}
			msg := err.Error()
			if !strings.Contains(msg, "dati da riconciliare prima della migrazione") || !strings.Contains(msg, "\n("+c.guardia+") ") {
				t.Fatalf("attesa la guardia (%s), ottenuto: %v", c.guardia, err)
			}
			for _, l := range lettere18 {
				if l != c.guardia && strings.Contains(msg, "\n("+l+") ") {
					t.Errorf("scatta anche la guardia (%s): la prova non isola la (%s)\n%v", l, c.guardia, err)
				}
			}
			if applicata(t, p, 18) {
				t.Error("la 0018 fermata risulta registrata")
			}
			if v := uno[int32](t, p, `SELECT max(versione) FROM schema_versione`); v != 17 {
				t.Errorf("schema_versione e' a %d dopo la guardia: doveva restare a 17", v)
			}
			if !uno[bool](t, p, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'componente' AND column_name = 'padre_id')`) {
				t.Error("padre_id non c'e' piu': la migrazione fermata ha cambiato lo schema")
			}
			if uno[bool](t, p, `SELECT to_regclass('public.componente_relazione') IS NOT NULL`) {
				t.Error("componente_relazione esiste: la migrazione fermata ha lasciato qualcosa")
			}
			if dopo := testutil.Conta(t, p, "componente"); dopo != prima {
				t.Errorf("componenti %d → %d: la migrazione fermata ha toccato i dati", prima, dopo)
			}
			coperte[c.guardia] = true
		})
	}
	for _, l := range lettere18 {
		if !coperte[l] {
			t.Errorf("la guardia (%s) non ha una prova che la faccia scattare", l)
		}
	}
}

// Le guardie si valutano tutte prima di fermarsi: un giro solo dice tutto quello che non va.
func TestLeGuardieDella18SiLeggonoTutteInUnGiro(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	fino17(t, p, `
		INSERT INTO componente (componente_id, thread_id, codice) VALUES ('{K1}', '{T1}', 'F1');
		INSERT INTO componente (componente_id, thread_id, codice, confermato_da) VALUES ('{K2}', '{T1}', ' ', '{U}');
		INSERT INTO documento (thread_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		     VALUES ('{T1}', 'cad_3d', '', 'h.stp', 'stp', repeat('1', 64), 'ELENCO DISEGNI\h.stp', '{U}');`)
	err := applica18(p)
	if err == nil {
		t.Fatal("la 0018 e' passata")
	}
	for _, l := range []string{"f", "g", "h"} {
		if !strings.Contains(err.Error(), "\n("+l+") ") {
			t.Errorf("manca la guardia (%s) nell'elenco: %v", l, err)
		}
	}
}

// «Revisione nota» = non NULL e non di soli spazi. Una revisione vuota non e' un disaccordo, e
// due revisioni che differiscono solo per spazi ai bordi o maiuscole nemmeno: la 0018 fonde.
func TestLa18NonSiFermaSuRevisioniVuoteOUgualiAMenoDelleMaiuscole(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	fino17(t, p, `
		INSERT INTO componente (componente_id, thread_id, codice, rev, confermato_da, creato_il) VALUES
		    ('{K1}', '{T1}', 'R1', '',    '{U}', now() - interval '2 hours'), ('{K2}', '{T1}', 'r1', 'A',   '{U}', now() - interval '1 hour'),
		    ('{K3}', '{T1}', 'R2', 'b',   '{U}', now() - interval '2 hours'), ('{K4}', '{T1}', 'r2', ' B ', '{U}', now() - interval '1 hour'),
		    ('{K5}', '{T1}', 'R3', '  ',  '{U}', now() - interval '2 hours'), ('{K6}', '{T1}', 'r3', NULL,  '{U}', now() - interval '1 hour');`)
	if err := applica18(p); err != nil {
		t.Fatalf("la 0018 si e' fermata su revisioni che non sono in disaccordo: %v", err)
	}
	attese := map[string]string{"{K1}": "A", "{K3}": "b", "{K5}": "  "}
	for id, rev := range attese {
		if got := uno[string](t, p, `SELECT coalesce(rev, '<NULL>') FROM componente WHERE componente_id = '`+id+`'`); got != rev {
			t.Errorf("componente %s: rev %q, attesa %q", id, got, rev)
		}
	}
	if n := testutil.Conta(t, p, "componente"); n != 3 {
		t.Errorf("componenti dopo la fusione: %d, attesi 3", n)
	}
}

// ------------------------------------------------------------------ fusione e travaso

func poolConAvvisi(t *testing.T) (*pgxpool.Pool, func() []string) {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(testutil.DSN(t))
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var avvisi []string
	cfg.ConnConfig.OnNotice = func(_ *pgconn.PgConn, n *pgconn.Notice) {
		mu.Lock()
		avvisi = append(avvisi, n.Message)
		mu.Unlock()
	}
	p, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), avvisi...) }
}

// Due righe con lo stesso codice a meno delle maiuscole diventano una: resta la piu' vecchia, prende
// cio' che sapeva l'altra dove lei non sapeva niente, e tutto quello che puntava all'altra (documenti,
// proposte, deroghe, figli) punta a lei. La qta del figlio passa sull'arco.
func TestLa18FondeIDuplicatiSenzaPerdereRiferimenti(t *testing.T) {
	p, avvisi := poolConAvvisi(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	fino17(t, p, `
		INSERT INTO componente (componente_id, thread_id, codice, rev, descrizione, confermato_da, creato_il) VALUES
		    ('{K1}', '{T1}', 'ab10', NULL, NULL,     '{U}', now() - interval '2 hours'),
		    ('{K2}', '{T1}', 'AB10', 'B',  'staffa', '{U}', now() - interval '1 hour');
		INSERT INTO componente (componente_id, thread_id, padre_id, codice, qta, confermato_da)
		     VALUES ('{K3}', '{T1}', '{K2}', 'FIGLIO', 3, '{U}');
		INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da) VALUES
		    ('{T1}', '{K1}', 'cad_3d',     'ab10', 'ab10.stp', 'stp', repeat('1', 64), 'ELENCO DISEGNI\AB10\ab10.stp', '{U}'),
		    ('{T1}', '{K2}', 'disegno_2d', 'AB10', 'AB10.pdf', 'pdf', repeat('2', 64), 'ELENCO DISEGNI\AB10\AB10.pdf', '{U}');
		INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte)
		     VALUES ('{A1}', '{T1}', 'disegno_2d', 'Ab10', '{K2}', 50, 'nome_file');
		INSERT INTO deroga_fabbisogno (thread_id, componente_id, tipo, motivo, utente_id)
		     VALUES ('{T1}', '{K2}', 'sviluppo_dxf', 'lo facciamo noi', '{U}');`)
	if err := applica18(p); err != nil {
		t.Fatal(err)
	}
	if n := uno[int](t, p, `SELECT count(*) FROM componente WHERE thread_id = '{T1}' AND upper(codice) = 'AB10'`); n != 1 {
		t.Fatalf("righe con codice AB10: %d, attesa 1", n)
	}
	var codice, rev, descrizione string
	if err := p.QueryRow(context.Background(), sostituzioni18.Replace(`SELECT codice, rev, descrizione FROM componente WHERE componente_id = '{K1}'`)).Scan(&codice, &rev, &descrizione); err != nil {
		t.Fatalf("la riga piu' vecchia non c'e' piu': %v", err)
	}
	if codice != "ab10" || rev != "B" || descrizione != "staffa" {
		t.Errorf("riga rimasta: codice %q rev %q descrizione %q; attesi ab10, B, staffa", codice, rev, descrizione)
	}
	if n := uno[int](t, p, `SELECT count(*) FROM documento WHERE componente_id = '{K1}' AND codice = 'ab10'`); n != 2 {
		t.Errorf("documenti passati alla riga rimasta con il suo codice: %d, attesi 2", n)
	}
	if got := uno[string](t, p, `SELECT path_relativo FROM documento WHERE sha256 = repeat('2', 64)`); got != `ELENCO DISEGNI\AB10\AB10.pdf` {
		t.Errorf("path_relativo cambiato: %q (sul NAS il file non si e' mosso)", got)
	}
	if got := uno[string](t, p, `SELECT componente_id::text || ' ' || codice FROM documento_proposta WHERE allegato_id = '{A1}'`); got != sostituzioni18.Replace("{K1} ab10") {
		t.Errorf("proposta: %q", got)
	}
	if got := uno[string](t, p, `SELECT componente_id::text FROM deroga_fabbisogno`); got != sostituzioni18.Replace("{K1}") {
		t.Errorf("deroga rimasta sul componente fuso: %s", got)
	}
	if got := uno[string](t, p, `SELECT padre_id::text || ' ' || qta FROM componente_relazione WHERE figlio_id = '{K3}'`); got != sostituzioni18.Replace("{K1} 3") {
		t.Errorf("arco del figlio: %q, atteso padre {K1} e qta 3", got)
	}
	tutti := strings.Join(avvisi(), "\n")
	for _, atteso := range []string{"0018: 1 componenti fusi", "0018: codice allineato al componente (solo maiuscole) su 1 documenti e 1 proposte"} {
		if !strings.Contains(tutti, atteso) {
			t.Errorf("manca l'avviso %q fra:\n%s", atteso, tutti)
		}
	}
}

// ------------------------------------------------------------------ schema 18: vincoli

// schema18 ricrea lo schema completo (con la 0018) e i dati di base, piu' due componenti: V1 nella
// prima RFQ, W1 nella seconda.
func schema18(t *testing.T) *pgxpool.Pool {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	esegui18(t, p, base18)
	esegui18(t, p, `INSERT INTO componente (componente_id, thread_id, codice, rev, tipo, confermato_da)
		VALUES ('{K1}', '{T1}', 'V1', 'A', 'sciolto', '{U}'), ('{K2}', '{T2}', 'W1', NULL, 'sciolto', '{U}');`)
	return p
}

func rifiutato(t *testing.T, p *pgxpool.Pool, atteso, sql string) {
	t.Helper()
	_, err := p.Exec(context.Background(), sostituzioni18.Replace(sql))
	if err == nil {
		t.Fatalf("accettato: doveva rifiutarlo %s\n%s", atteso, sql)
	}
	if got := vincolo(err); got != atteso {
		t.Fatalf("rifiutato da %q, atteso %q: %v", got, atteso, err)
	}
}

func TestUnaRelazioneFraDueRfqEImpossibile(t *testing.T) {
	p := schema18(t)
	rifiutato(t, p, "fk_relazione_figlio", `INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, origine, confermato_da)
		VALUES ('{T1}', '{K1}', '{K2}', 'manuale', '{U}')`)
	rifiutato(t, p, "fk_relazione_padre", `INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, origine, confermato_da)
		VALUES ('{T2}', '{K1}', '{K2}', 'manuale', '{U}')`)
}

func TestUnDocumentoNonSiAggancaAUnComponenteDiUnAltraRfq(t *testing.T) {
	p := schema18(t)
	rifiutato(t, p, "fk_documento_componente", `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ('{T2}', '{K1}', 'disegno_2d', 'V1', 'v1.pdf', 'pdf', repeat('1', 64), 'x', '{U}')`)
	rifiutato(t, p, "fk_proposta_componente", `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte)
		VALUES ('{A1}', '{T2}', 'disegno_2d', 'V1', '{K1}', 50, 'nome_file')`)
	rifiutato(t, p, "fk_deroga_componente", `INSERT INTO deroga_fabbisogno (thread_id, componente_id, tipo, motivo, utente_id)
		VALUES ('{T2}', '{K1}', 'disegno_2d', 'prova', '{U}')`)
	rifiutato(t, p, "ck_proposta_componente_thread", `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte)
		VALUES ('{A1}', NULL, 'disegno_2d', 'V1', '{K1}', 50, 'nome_file')`)
}

func TestUnDocumentoAgganciatoHaIlCodiceDelComponente(t *testing.T) {
	p := schema18(t)
	rifiutato(t, p, "fk_documento_componente", `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ('{T1}', '{K1}', 'disegno_2d', 'v1', 'v1.pdf', 'pdf', repeat('1', 64), 'x', '{U}')`)
	rifiutato(t, p, "ck_documento_agganciato_ha_codice", `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ('{T1}', '{K1}', 'capitolato', ' ', 'v1.pdf', 'pdf', repeat('1', 64), 'x', '{U}')`)
	rifiutato(t, p, "ck_documento_tecnico_ha_codice", `INSERT INTO documento (thread_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ('{T1}', 'sviluppo_dxf', NULL, 'v1.dxf', 'dxf', repeat('1', 64), 'x', '{U}')`)
	rifiutato(t, p, "ck_proposta_agganciata_ha_codice", `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte)
		VALUES ('{A1}', '{T1}', 'disegno_2d', NULL, '{K1}', 50, 'nome_file')`)
	// senza componente un capitolato senza codice resta legittimo
	esegui18(t, p, `INSERT INTO documento (thread_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ('{T1}', 'capitolato', NULL, 'cap.pdf', 'pdf', repeat('3', 64), 'CAPITOLATI\cap.pdf', '{U}')`)
}

func TestIlComponenteDella18(t *testing.T) {
	p := schema18(t)
	rifiutato(t, p, "ux_componente_thread_codice", `INSERT INTO componente (thread_id, codice, confermato_da) VALUES ('{T1}', 'v1', '{U}')`)
	rifiutato(t, p, "ck_componente_codice", `INSERT INTO componente (thread_id, codice, confermato_da) VALUES ('{T1}', '  ', '{U}')`)
	rifiutato(t, p, "not null confermato_da", `INSERT INTO componente (thread_id, codice) VALUES ('{T1}', 'Z9')`)
	// lo stesso codice in un'altra RFQ e' un altro pezzo
	esegui18(t, p, `INSERT INTO componente (thread_id, codice, confermato_da) VALUES ('{T2}', 'V1', '{U}')`)
	rifiutato(t, p, "componente_relazione_check", `INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, origine, confermato_da)
		VALUES ('{T1}', '{K1}', '{K1}', 'manuale', '{U}')`)
}

func TestLaPropostaDiComponenteDecisaHaIlSuoComponente(t *testing.T) {
	p := schema18(t)
	for _, stato := range []string{"confermata", "duplicato"} {
		rifiutato(t, p, "ck_componente_proposta_decisa", `INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, fonte, confidenza, stato)
			VALUES ('{T1}', '{A1}', repeat('a', 64), '#12', 'STAFFA', 'step', 80, '`+stato+`')`)
	}
	rifiutato(t, p, "fk_componente_proposta_componente", `INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, fonte, confidenza, stato, componente_id)
		VALUES ('{T1}', '{A1}', repeat('a', 64), '#12', 'STAFFA', 'step', 80, 'confermata', '{K2}')`)
	esegui18(t, p, `INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, fonte, confidenza, stato, componente_id)
		VALUES ('{T1}', '{A1}', repeat('a', 64), '#12', 'STAFFA', 'step', 80, 'confermata', '{K1}'),
		       ('{T1}', '{A1}', repeat('a', 64), '#13', 'VITE', 'step', 80, 'aperta', NULL)`)
	rifiutato(t, p, "fk_relazione_proposta_figlio", `INSERT INTO relazione_proposta (thread_id, allegato_id, padre_chiave, figlio_chiave)
		VALUES ('{T1}', '{A1}', '#12', '#99')`)
	esegui18(t, p, `INSERT INTO relazione_proposta (thread_id, allegato_id, padre_chiave, figlio_chiave, qta) VALUES ('{T1}', '{A1}', '#12', '#13', 4)`)
}

// Nella 17 il documento poteva avere il codice con maiuscole diverse da quello del componente; la 18
// lo allinea (la FK composita lo vuole identico) e non tocca path_relativo.
func TestLaMigrazione18NormalizzaLeMaiuscoleDeiCodici(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	fino17(t, p, `
		INSERT INTO componente (componente_id, thread_id, codice, confermato_da) VALUES ('{K1}', '{T1}', 'AB10', '{U}');
		INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		     VALUES ('{T1}', '{K1}', 'disegno_2d', 'ab10', 'ab10.pdf', 'pdf', repeat('1', 64), 'ELENCO DISEGNI\AB10\ab10.pdf', '{U}');
		INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte)
		     VALUES ('{A1}', '{T1}', 'disegno_2d', 'Ab10', '{K1}', 50, 'nome_file');`)
	if err := applica18(p); err != nil {
		t.Fatal(err)
	}
	if got := uno[string](t, p, `SELECT codice || ' ' || path_relativo FROM documento`); got != `AB10 ELENCO DISEGNI\AB10\ab10.pdf` {
		t.Errorf("documento dopo la 18: %q", got)
	}
	if got := uno[string](t, p, `SELECT codice FROM documento_proposta`); got != "AB10" {
		t.Errorf("proposta dopo la 18: %q", got)
	}
}

// Correggere il codice di un componente che ha documenti e' un'operazione in due passi (componente,
// poi documenti). La FK e' DEFERRABLE INITIALLY IMMEDIATE: chi se ne dimentica fallisce alla prima
// UPDATE; chi apre la transazione con SET CONSTRAINTS ALL DEFERRED ci riesce.
func TestCorreggereIlCodiceSenzaDeferredFallisceSubito(t *testing.T) {
	p := schema18(t)
	ctx := context.Background()
	esegui18(t, p, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ('{T1}', '{K1}', 'disegno_2d', 'V1', 'v1.pdf', 'pdf', repeat('1', 64), 'x', '{U}')`)
	rifiutato(t, p, "fk_documento_componente", `UPDATE componente SET codice = 'V1-NUOVO' WHERE componente_id = '{K1}'`)

	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, sql := range []string{
		`SET CONSTRAINTS ALL DEFERRED`,
		`UPDATE componente SET codice = 'V1-NUOVO' WHERE componente_id = '{K1}'`,
		`UPDATE documento SET codice = 'V1-NUOVO' WHERE componente_id = '{K1}'`,
	} {
		if _, err := tx.Exec(ctx, sostituzioni18.Replace(sql)); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("con SET CONSTRAINTS ALL DEFERRED la correzione in due passi doveva riuscire: %v", err)
	}
}

// ------------------------------------------------------------------ schema 18: viste e fabbisogni

func TestIDefaultDelFabbisognoDella18(t *testing.T) {
	p := schema18(t)
	attesi := map[string]bool{"sottoassieme cad_3d": false, "sottoassieme sviluppo_dxf": false, "sciolto cad_3d": false,
		"finito cad_3d": true, "sottoassieme disegno_2d": true, "sciolto disegno_2d": true}
	for k, blocca := range attesi {
		parti := strings.Fields(k)
		got := uno[bool](t, p, `SELECT bloccante FROM fabbisogno_documento WHERE cliente_id IS NULL AND tipo_componente = '`+parti[0]+`' AND tipo = '`+parti[1]+`'`)
		if got != blocca {
			t.Errorf("%s: bloccante %v, atteso %v", k, got, blocca)
		}
	}
}

// rev_diversa e' vera solo con due revisioni note e diverse a meno di spazi e maiuscole; un documento
// in coda non blocca (D9), uno in errore si'.
func TestIlFascicoloDella18(t *testing.T) {
	p := schema18(t)
	esegui18(t, p, `INSERT INTO documento (documento_id, thread_id, componente_id, tipo, codice, rev, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ('60000000-0000-0000-0000-000000000001', '{T1}', '{K1}', 'disegno_2d', 'V1', ' a ', 'v1.pdf', 'pdf', repeat('1', 64), 'x', '{U}')`)
	riga := func() (string, bool, int) {
		t.Helper()
		var esito string
		var diversa bool
		var bloccanti int
		if err := p.QueryRow(context.Background(), sostituzioni18.Replace(`
			SELECT f.esito, f.rev_diversa, b.n_bloccanti FROM v_fascicolo f JOIN v_thread_bloccanti b USING (thread_id)
			 WHERE f.componente_id = '{K1}' AND f.tipo_documento = 'disegno_2d'`)).Scan(&esito, &diversa, &bloccanti); err != nil {
			t.Fatal(err)
		}
		return esito, diversa, bloccanti
	}
	if esito, diversa, n := riga(); esito != "ok_in_coda" || diversa || n != 0 {
		t.Errorf("in coda, rev ' a ' contro 'A': esito %s, rev_diversa %v, bloccanti %d; attesi ok_in_coda, false, 0", esito, diversa, n)
	}
	for _, c := range []struct {
		rev     string
		diversa bool
	}{{"", false}, {"   ", false}, {"B", true}} {
		esegui18(t, p, `UPDATE documento SET rev = NULLIF('`+c.rev+`', '<NULL>') WHERE componente_id = '{K1}'`)
		if _, diversa, _ := riga(); diversa != c.diversa {
			t.Errorf("rev del documento %q contro 'A': rev_diversa %v, atteso %v", c.rev, diversa, c.diversa)
		}
	}
	esegui18(t, p, `UPDATE componente SET rev = NULL WHERE componente_id = '{K1}'`)
	if _, diversa, _ := riga(); diversa {
		t.Error("componente senza revisione: rev_diversa deve essere false")
	}
	esegui18(t, p, `UPDATE documento SET stato_nas = 'errore' WHERE componente_id = '{K1}'`)
	if esito, _, n := riga(); esito != "ok_errore_nas" || n != 1 {
		t.Errorf("in errore: esito %s, bloccanti %d; attesi ok_errore_nas, 1", esito, n)
	}
	esegui18(t, p, `UPDATE documento SET stato_nas = 'scritto' WHERE componente_id = '{K1}'`)
	if esito, _, n := riga(); esito != "ok" || n != 0 {
		t.Errorf("scritto: esito %s, bloccanti %d; attesi ok, 0", esito, n)
	}
}

// Un sottoassieme condiviso da due prodotti e' UN componente con due archi: nell'albero compare una
// volta per percorso, con la qta moltiplicata lungo il percorso.
func TestLAlberoConUnSottoassiemeCondiviso(t *testing.T) {
	p := schema18(t)
	esegui18(t, p, `
		INSERT INTO componente (componente_id, thread_id, codice, tipo, confermato_da) VALUES
		    ('{K3}', '{T1}', 'P1', 'finito', '{U}'), ('{K4}', '{T1}', 'P2', 'finito', '{U}'),
		    ('{K5}', '{T1}', 'S1', 'sottoassieme', '{U}'), ('{K6}', '{T1}', 'L1', 'sciolto', '{U}');
		INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES
		    ('{T1}', '{K3}', '{K5}', 2, 'manuale', '{U}'), ('{T1}', '{K4}', '{K5}', 3, 'manuale', '{U}'),
		    ('{T1}', '{K5}', '{K6}', 4, 'manuale', '{U}');`)
	got := uno[string](t, p, `SELECT string_agg(c.codice || '×' || a.qta_cumulata || '@' || a.profondita, ' ' ORDER BY c.codice, a.qta_cumulata)
		FROM v_componente_albero a JOIN componente c USING (componente_id) WHERE a.thread_id = '{T1}'`)
	if atteso := "L1×8@2 L1×12@2 P1×1@0 P2×1@0 S1×2@1 S1×3@1 V1×1@0"; got != atteso {
		t.Errorf("albero: %s, atteso %s", got, atteso)
	}
}

// ------------------------------------------------------------------ ritorno 17 → 18 → 17

// impronta18 descrive la forma dello schema: le relazioni (tipo, proprietario, permessi), le colonne
// con la loro posizione fisica, il tipo completo, NOT NULL, default, identity, generated e collation,
// i vincoli, gli indici, le viste, i trigger, le regole, le policy, le funzioni, le sequenze, i tipi
// propri, le estensioni, tutti i commenti e i fabbisogni di default. Due schemi con la stessa
// impronta sono lo stesso schema.
func impronta18(t *testing.T, p *pgxpool.Pool) []string {
	t.Helper()
	righe, err := p.Query(context.Background(), `
		SELECT 'relazione ' || c.relname || ' ' || c.relkind::text || ' ' || c.relpersistence::text || ' ' || c.relowner::regrole
		       || ' rls=' || c.relrowsecurity || ' acl=' || coalesce(c.relacl::text, '')
		  FROM pg_class c WHERE c.relnamespace = 'public'::regnamespace AND c.relkind IN ('r', 'p', 'v', 'm', 'S', 'f', 'c')
		UNION ALL
		SELECT 'colonna ' || c.relname || '.' || a.attname || ' #' || a.attnum || ' ' || format_type(a.atttypid, a.atttypmod)
		       || CASE WHEN a.attnotnull THEN ' NOT NULL' ELSE '' END
		       || coalesce(' DEFAULT ' || pg_get_expr(d.adbin, d.adrelid), '')
		       || CASE WHEN a.attidentity <> '' THEN ' IDENTITY ' || a.attidentity::text ELSE '' END
		       || CASE WHEN a.attgenerated <> '' THEN ' GENERATED ' || a.attgenerated::text ELSE '' END
		       || CASE WHEN a.attcollation <> ty.typcollation THEN ' COLLATE ' || a.attcollation::regcollation ELSE '' END
		  FROM pg_class c JOIN pg_attribute a ON a.attrelid = c.oid JOIN pg_type ty ON ty.oid = a.atttypid
		  LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		 WHERE c.relnamespace = 'public'::regnamespace AND c.relkind IN ('r', 'p', 'v', 'm', 'f', 'c')
		   AND a.attnum > 0 AND NOT a.attisdropped
		UNION ALL
		SELECT 'vincolo ' || conrelid::regclass || ' ' || conname || ' ' || pg_get_constraintdef(oid)
		  FROM pg_constraint WHERE connamespace = 'public'::regnamespace
		UNION ALL
		SELECT 'indice ' || indexname || ' ' || indexdef FROM pg_indexes WHERE schemaname = 'public'
		UNION ALL
		SELECT 'vista ' || viewname || ' ' || definition FROM pg_views WHERE schemaname = 'public'
		UNION ALL
		SELECT 'vista materializzata ' || matviewname || ' ' || definition FROM pg_matviews WHERE schemaname = 'public'
		UNION ALL
		SELECT 'trigger ' || pg_get_triggerdef(g.oid) || ' ' || g.tgenabled::text
		  FROM pg_trigger g JOIN pg_class c ON c.oid = g.tgrelid
		 WHERE c.relnamespace = 'public'::regnamespace AND NOT g.tgisinternal
		UNION ALL
		SELECT 'regola ' || tablename || ' ' || rulename || ' ' || definition FROM pg_rules WHERE schemaname = 'public'
		UNION ALL
		SELECT 'policy ' || tablename || ' ' || policyname || ' ' || cmd || ' ' || coalesce(qual, '') || ' ' || coalesce(with_check, '')
		  FROM pg_policies WHERE schemaname = 'public'
		UNION ALL
		SELECT 'funzione ' || f.oid::regprocedure || ' ' || f.prokind::text || ' ' || CASE WHEN f.prokind IN ('f', 'p') THEN pg_get_functiondef(f.oid) ELSE '' END
		  FROM pg_proc f WHERE f.pronamespace = 'public'::regnamespace
		UNION ALL
		SELECT 'sequenza ' || s.seqrelid::regclass || ' ' || format_type(s.seqtypid, NULL) || ' ' || s.seqstart || ' ' || s.seqincrement
		       || ' ' || s.seqmin || ' ' || s.seqmax || ' ' || s.seqcache || ' ' || s.seqcycle
		  FROM pg_sequence s JOIN pg_class c ON c.oid = s.seqrelid WHERE c.relnamespace = 'public'::regnamespace
		UNION ALL
		SELECT 'tipo ' || ty.typname || ' ' || ty.typtype::text || ' ' || format_type(ty.typbasetype, ty.typtypmod) || ' ' || ty.typnotnull
		       || ' ' || coalesce((SELECT string_agg(e.enumlabel, ',' ORDER BY e.enumsortorder) FROM pg_enum e WHERE e.enumtypid = ty.oid), '')
		  FROM pg_type ty WHERE ty.typnamespace = 'public'::regnamespace AND ty.typtype IN ('e', 'd', 'r', 'm')
		UNION ALL
		SELECT 'estensione ' || extname || ' ' || extversion FROM pg_extension
		UNION ALL
		SELECT 'commento ' || o.type || ' ' || o.identity || ' ' || d.description
		  FROM pg_description d, LATERAL pg_identify_object(d.classoid, d.objoid, d.objsubid) o
		 WHERE o.schema = 'public'
		UNION ALL
		SELECT 'fabbisogno ' || tipo_componente || ' ' || tipo || ' ' || bloccante FROM fabbisogno_documento WHERE cliente_id IS NULL
		ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer righe.Close()
	var out []string
	for righe.Next() {
		var s string
		if err := righe.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := righe.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func indietro18(t *testing.T) string {
	t.Helper()
	sql, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "0018_indietro.sql"))
	if err != nil {
		t.Fatal(err)
	}
	return string(sql)
}

func TestIlRitornoManualeDalla18Alla17(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	ctx := context.Background()

	testutil.SchemaFinoA(t, p, 17)
	prima := impronta18(t, p)
	esegui18(t, p, base18)
	esegui18(t, p, `
		INSERT INTO componente (componente_id, thread_id, codice, tipo, confermato_da) VALUES ('{K1}', '{T1}', 'P1', 'finito', '{U}');
		INSERT INTO componente (componente_id, thread_id, padre_id, codice, qta, confermato_da) VALUES ('{K2}', '{T1}', '{K1}', 'F1', 2, '{U}');
		INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		     VALUES ('{T1}', '{K2}', 'disegno_2d', 'F1', 'f1.pdf', 'pdf', repeat('1', 64), 'ELENCO DISEGNI\F1\f1.pdf', '{U}');`)

	if err := applica18(p); err != nil {
		t.Fatal(err)
	}
	if got := uno[string](t, p, `SELECT padre_id::text || ' ' || qta FROM componente_relazione WHERE figlio_id = '{K2}'`); got != sostituzioni18.Replace("{K1} 2") {
		t.Fatalf("arco dopo la 18: %q", got)
	}
	// dati entrati con lo schema 18, che il ritorno deve conservare
	esegui18(t, p, `INSERT INTO componente (componente_id, thread_id, codice, confermato_da) VALUES ('{K3}', '{T1}', 'NUOVO18', '{U}');
		INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, fonte, confidenza)
		     VALUES ('{T1}', '{A1}', repeat('a', 64), '#1', 'STAFFA', 'step', 80);`)

	if _, err := p.Exec(ctx, indietro18(t)); err != nil {
		t.Fatalf("0018_indietro.sql: %v", err)
	}
	// L'unica differenza ammessa (decisione del 23/09/2026, scritta in testa allo script):
	// padre_id, ricreato con ADD COLUMN, torna in fondo a componente invece che al terzo posto.
	// Tutto il resto, comprese le posizioni delle altre colonne, deve essere identico.
	fondo := uno[int32](t, p, `SELECT max(attnum)::int FROM pg_attribute WHERE attrelid = 'componente'::regclass AND NOT attisdropped`)
	var atteso []string
	spostate := 0
	for _, s := range prima {
		if resto, ok := strings.CutPrefix(s, "colonna componente.padre_id #3 "); ok {
			s = fmt.Sprintf("colonna componente.padre_id #%d %s", fondo, resto)
			spostate++
		}
		atteso = append(atteso, s)
	}
	if spostate != 1 {
		t.Fatalf("nella 17 padre_id doveva essere la terza colonna di componente: righe trovate %d", spostate)
	}
	dopo := impronta18(t, p)
	if d := differenze(atteso, dopo); d != "" {
		t.Errorf("lo schema dopo 17 → 18 → 17 non e' quello della 17 con padre_id in fondo:\n%s", d)
	}
	if applicata(t, p, 18) {
		t.Error("schema_versione dice ancora 18")
	}
	if v := uno[int32](t, p, `SELECT max(versione) FROM schema_versione`); v != 17 {
		t.Errorf("schema_versione e' a %d dopo il ritorno: doveva tornare a 17", v)
	}
	if got := uno[string](t, p, `SELECT padre_id::text || ' ' || qta FROM componente WHERE componente_id = '{K2}'`); got != sostituzioni18.Replace("{K1} 2") {
		t.Errorf("il figlio non ha ritrovato padre e qta: %q", got)
	}
	if n := uno[int](t, p, `SELECT count(*) FROM componente WHERE componente_id IN ('{K1}', '{K2}', '{K3}')`); n != 3 {
		t.Errorf("componenti conservati: %d, attesi 3", n)
	}
	if got := uno[string](t, p, `SELECT componente_id::text FROM documento`); got != sostituzioni18.Replace("{K2}") {
		t.Errorf("documento non piu' agganciato: %q", got)
	}

	// e la 18 si riapplica sopra il ritorno
	if err := applica18(p); err != nil {
		t.Fatalf("la 0018 non si riapplica dopo il ritorno: %v", err)
	}
	if !applicata(t, p, 18) {
		t.Error("la 0018 riapplicata non risulta registrata")
	}
}

// Un figlio con due padri il modello della 17 non lo sa dire: il ritorno si ferma e non cambia niente.
func TestIlRitornoSiFermaSuUnFiglioConDuePadri(t *testing.T) {
	p := schema18(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esegui18(t, p, `
		INSERT INTO componente (componente_id, thread_id, codice, confermato_da) VALUES ('{K3}', '{T1}', 'P1', '{U}'), ('{K4}', '{T1}', 'P2', '{U}');
		INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, origine, confermato_da)
		     VALUES ('{T1}', '{K3}', '{K1}', 'manuale', '{U}'), ('{T1}', '{K4}', '{K1}', 'manuale', '{U}');`)
	_, err := p.Exec(context.Background(), indietro18(t))
	if err == nil || !strings.Contains(err.Error(), "ha 2 padri") {
		t.Fatalf("atteso il rifiuto del figlio con due padri, ottenuto %v", err)
	}
	// la connessione che ha eseguito lo script puo' essere rimasta in una transazione fallita: si
	// controlla da una nuova
	if !applicata(t, p, 18) || !uno[bool](t, p, `SELECT to_regclass('public.componente_relazione') IS NOT NULL`) {
		t.Error("il ritorno fermato ha cambiato lo schema")
	}
}

func differenze(a, b []string) string {
	in := func(xs []string) map[string]bool {
		m := map[string]bool{}
		for _, x := range xs {
			m[x] = true
		}
		return m
	}
	ma, mb := in(a), in(b)
	var out []string
	for _, x := range a {
		if !mb[x] {
			out = append(out, "- "+x)
		}
	}
	for _, x := range b {
		if !ma[x] {
			out = append(out, "+ "+x)
		}
	}
	return strings.Join(out, "\n")
}
