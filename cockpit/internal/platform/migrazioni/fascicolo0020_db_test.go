//go:build integrazione

// L4 — la 0019 e la 0020 (Blocco 8, A4, B8.A4a): revisioni dei documenti, versioni della BOM, STEP
// strutturale, deroga strutturale, archiviazione, e le tabelle dello spostamento sul NAS.
//
// Qui si prova il DATABASE: le guardie della 0020, i vincoli e i trigger contro scritture dirette, le
// viste. Chi scrive davvero (congelaBom, la revisione, la sostituzione, la conferma) ha le sue prove
// nei package che lo fanno; qui una versione congelata si costruisce a mano, con le stesse righe che
// scrivera' il congelamento, perche' la domanda e' «che cosa rifiuta il database», non «che cosa fa
// il Go».
//
// Nomi delle prove di A4.12 dell'addendum che stanno qui, in tutto o nella parte sul database:
// 10 TestUnaBaselineCongelataNonSiModifica, 23 TestLoStepDelFinitoMancanteSiVede,
// 40 TestIlRiferimentoDeveEssereUnoStepDelComponente, 48 TestLaCatenaDelleRevisioniNonFaCicli,
// 49 TestUnaRevisioneHaLoStessoTipoELoStessoComponente, 50 TestUnaSostituzioneNonSiRiscrive,
// 64 TestLaBaselineRicordaLaDerogaStrutturale (FK e CHECK), 65 (il CHECK del confine),
// 66 TestLaBaselineRifiutaUnoStepDiUnAltroComponente, 67 (la FK fra nome e riga della fase),
// 70 TestLaWorkingCongelataProteggeDocumentiEDeroghe (il trigger), 72 TestLaCatenaDelleVersioniNonHaSalti,
// piu' TestLaMigrazione20SiFermaSuDatiIncoerenti e TestLaWorkingCongelataNonSiModificaSenzaRevisione.

package migrazioni_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/migrazioni"
	"promatec/cockpit/internal/platform/testutil"
)

// Gli identificativi della 0018 piu' quelli nuovi: documenti, versioni, righe di fase, deroghe.
var sostituzioni20 = strings.NewReplacer(
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
	"{F1}", "60000000-0000-0000-0000-000000000001",
	"{F2}", "60000000-0000-0000-0000-000000000002",
	"{F3}", "60000000-0000-0000-0000-000000000003",
	"{F4}", "60000000-0000-0000-0000-000000000004",
	"{D1}", "70000000-0000-0000-0000-000000000001",
	"{D2}", "70000000-0000-0000-0000-000000000002",
	"{D3}", "70000000-0000-0000-0000-000000000003",
	"{D4}", "70000000-0000-0000-0000-000000000004",
	"{D5}", "70000000-0000-0000-0000-000000000005",
	"{D6}", "70000000-0000-0000-0000-000000000006",
	"{G1}", "80000000-0000-0000-0000-000000000001",
	"{V1}", "90000000-0000-0000-0000-000000000001",
	"{V2}", "90000000-0000-0000-0000-000000000002",
	"{V3}", "90000000-0000-0000-0000-000000000003",
	"{VX}", "90000000-0000-0000-0000-000000000009",
)

func esegui20(t *testing.T, p *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := p.Exec(context.Background(), sostituzioni20.Replace(sql)); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
}

func uno20[T any](t *testing.T, p *pgxpool.Pool, sql string) T {
	t.Helper()
	var v T
	if err := p.QueryRow(context.Background(), sostituzioni20.Replace(sql)).Scan(&v); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return v
}

// codice e' lo SQLSTATE dell'errore: i trigger della 0020 hanno i loro (BOM01–BOM05).
func codice(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

// rifiuta esegue sql e vuole il rifiuto atteso: un codice BOMxx del trigger, oppure il nome del
// vincolo violato.
func rifiuta(t *testing.T, p *pgxpool.Pool, atteso, sql string) {
	t.Helper()
	_, err := p.Exec(context.Background(), sostituzioni20.Replace(sql))
	if err == nil {
		t.Fatalf("accettato: doveva rifiutarlo %s\n%s", atteso, sql)
	}
	if strings.HasPrefix(atteso, "BOM") {
		if got := codice(err); got != atteso {
			t.Fatalf("rifiutato con %s, atteso %s: %v", got, atteso, err)
		}
		return
	}
	if got := vincolo(err); got != atteso {
		t.Fatalf("rifiutato da %q, atteso %q: %v", got, atteso, err)
	}
}

func applica20(p *pgxpool.Pool) error {
	_, err := migrazioni.ApplicaFinoA(context.Background(), p, risorse.FS, 20, testutil.LogSilenzioso())
	return err
}

// fino19 ricrea lo schema alla 19 con i dati di base, due componenti (V1 in T1, W1 in T2) e i dati
// della prova.
func fino19(t *testing.T, p *pgxpool.Pool, dati string) {
	t.Helper()
	testutil.SchemaFinoA(t, p, 19)
	esegui20(t, p, base18)
	esegui20(t, p, `INSERT INTO componente (componente_id, thread_id, codice, confermato_da)
		VALUES ('{K1}', '{T1}', 'V1', '{U}'), ('{K2}', '{T2}', 'W1', '{U}');`)
	if dati != "" {
		esegui20(t, p, dati)
	}
}

// doc20 e' l'INSERT di un documento della prova: id, thread, componente (o NULL), tipo, codice, sha
// (una cifra ripetuta), percorso.
func doc20(id, thread, comp, tipo, codice, sha, percorso string) string {
	c := "NULL"
	if comp != "" {
		c = "'" + comp + "'"
	}
	k := "NULL"
	if codice != "" {
		k = "'" + codice + "'"
	}
	return `INSERT INTO documento (documento_id, thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
	     VALUES ('` + id + `', '` + thread + `', ` + c + `, '` + tipo + `', ` + k + `, 'f.stp', 'stp', repeat('` + sha + `', 64), '` + percorso + `', '{U}');`
}

// ------------------------------------------------------------------ guardie

var lettere20 = []string{"a", "b", "c", "d", "e"}

func TestLaMigrazione20SiFermaSuDatiIncoerenti(t *testing.T) {
	casi := []struct {
		guardia, nome, dati string
	}{
		{"a", "due documenti dichiarano lo stesso file (maiuscole diverse)",
			doc20("{D1}", "{T1}", "", "capitolato", "", "1", `CAPITOLATI\Specifica.pdf`) +
				doc20("{D2}", "{T1}", "", "capitolato", "", "2", `capitolati\SPECIFICA.PDF`)},
		{"b", "sostituito da un documento di un'altra RFQ",
			doc20("{D1}", "{T1}", "{K1}", "disegno_2d", "V1", "1", `ELENCO DISEGNI\V1\a.pdf`) +
				doc20("{D2}", "{T2}", "{K2}", "disegno_2d", "W1", "2", `ELENCO DISEGNI\W1\b.pdf`) +
				`UPDATE documento SET sostituito_da = '{D2}' WHERE documento_id = '{D1}';`},
		{"b", "sostituito da un documento di un altro tipo",
			doc20("{D1}", "{T1}", "{K1}", "disegno_2d", "V1", "1", `ELENCO DISEGNI\V1\a.pdf`) +
				doc20("{D2}", "{T1}", "{K1}", "cad_3d", "V1", "2", `ELENCO DISEGNI\V1\a.stp`) +
				`UPDATE documento SET sostituito_da = '{D2}' WHERE documento_id = '{D1}';`},
		{"c", "sostituito senza componente",
			doc20("{D1}", "{T1}", "", "capitolato", "", "1", `CAPITOLATI\a.pdf`) +
				doc20("{D2}", "{T1}", "", "capitolato", "", "2", `CAPITOLATI\b.pdf`) +
				`UPDATE documento SET sostituito_da = '{D2}' WHERE documento_id = '{D1}';`},
		{"d", "ciclo lungo A → B → C → A",
			doc20("{D1}", "{T1}", "{K1}", "disegno_2d", "V1", "1", `ELENCO DISEGNI\V1\a.pdf`) +
				doc20("{D2}", "{T1}", "{K1}", "disegno_2d", "V1", "2", `ELENCO DISEGNI\V1\b.pdf`) +
				doc20("{D3}", "{T1}", "{K1}", "disegno_2d", "V1", "3", `ELENCO DISEGNI\V1\c.pdf`) +
				`UPDATE documento SET sostituito_da = '{D2}' WHERE documento_id = '{D1}';
				 UPDATE documento SET sostituito_da = '{D3}' WHERE documento_id = '{D2}';
				 UPDATE documento SET sostituito_da = '{D1}' WHERE documento_id = '{D3}';`},
		{"d", "un documento sostituito da se stesso",
			doc20("{D1}", "{T1}", "{K1}", "disegno_2d", "V1", "1", `ELENCO DISEGNI\V1\a.pdf`) +
				`UPDATE documento SET sostituito_da = '{D1}' WHERE documento_id = '{D1}';`},
		{"e", "due documenti sostituiti dallo stesso",
			doc20("{D1}", "{T1}", "{K1}", "disegno_2d", "V1", "1", `ELENCO DISEGNI\V1\a.pdf`) +
				doc20("{D2}", "{T1}", "{K1}", "disegno_2d", "V1", "2", `ELENCO DISEGNI\V1\b.pdf`) +
				doc20("{D3}", "{T1}", "{K1}", "disegno_2d", "V1", "3", `ELENCO DISEGNI\V1\c.pdf`) +
				`UPDATE documento SET sostituito_da = '{D3}' WHERE documento_id IN ('{D1}', '{D2}');`},
	}
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	coperte := map[string]bool{}
	for _, c := range casi {
		t.Run(c.guardia+" "+c.nome, func(t *testing.T) {
			fino19(t, p, c.dati)
			prima := uno20[string](t, p, `SELECT md5(string_agg(d::text, '|' ORDER BY documento_id)) FROM documento d`)

			err := applica20(p)
			if err == nil {
				t.Fatal("la 0020 e' passata: la guardia doveva fermarla")
			}
			msg := err.Error()
			if !strings.Contains(msg, "dati da riconciliare prima della migrazione") || !strings.Contains(msg, "\n("+c.guardia+") ") {
				t.Fatalf("attesa la guardia (%s), ottenuto: %v", c.guardia, err)
			}
			for _, l := range lettere20 {
				if l != c.guardia && strings.Contains(msg, "\n("+l+") ") {
					t.Errorf("scatta anche la guardia (%s): la prova non isola la (%s)\n%v", l, c.guardia, err)
				}
			}
			if applicata(t, p, 20) {
				t.Error("la 0020 fermata risulta registrata")
			}
			if v := uno20[int32](t, p, `SELECT max(versione) FROM schema_versione`); v != 19 {
				t.Errorf("schema_versione e' a %d dopo la guardia: doveva restare a 19", v)
			}
			if uno20[bool](t, p, `SELECT to_regclass('public.bom_versione') IS NOT NULL OR to_regclass('public.ux_documento_percorso') IS NOT NULL
			                          OR to_regtype('public.stato_bom') IS NOT NULL`) {
				t.Error("la migrazione fermata ha lasciato qualcosa nello schema")
			}
			if dopo := uno20[string](t, p, `SELECT md5(string_agg(d::text, '|' ORDER BY documento_id)) FROM documento d`); dopo != prima {
				t.Error("la migrazione fermata ha toccato i documenti")
			}
			coperte[c.guardia] = true
		})
	}
	for _, l := range lettere20 {
		if !coperte[l] {
			t.Errorf("la guardia (%s) non ha una prova che la faccia scattare", l)
		}
	}
}

// Le guardie si valutano tutte prima di fermarsi: un giro solo dice tutto quello che non va.
func TestLeGuardieDella20SiLeggonoTutteInUnGiro(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	fino19(t, p, doc20("{D1}", "{T1}", "", "capitolato", "", "1", `CAPITOLATI\a.pdf`)+
		doc20("{D2}", "{T1}", "", "capitolato", "", "2", `CAPITOLATI\A.PDF`)+
		doc20("{D3}", "{T1}", "{K1}", "disegno_2d", "V1", "3", `ELENCO DISEGNI\V1\a.pdf`)+
		doc20("{D4}", "{T1}", "{K1}", "disegno_2d", "V1", "4", `ELENCO DISEGNI\V1\b.pdf`)+
		doc20("{D5}", "{T1}", "{K1}", "disegno_2d", "V1", "5", `ELENCO DISEGNI\V1\c.pdf`)+
		`UPDATE documento SET sostituito_da = '{D2}' WHERE documento_id = '{D1}';
		 UPDATE documento SET sostituito_da = '{D5}' WHERE documento_id IN ('{D3}', '{D4}');`)
	err := applica20(p)
	if err == nil {
		t.Fatal("la 0020 e' passata")
	}
	for _, l := range []string{"a", "c", "e"} {
		if !strings.Contains(err.Error(), "\n("+l+") ") {
			t.Errorf("manca la guardia (%s) nell'elenco: %v", l, err)
		}
	}
}

// Su dati buoni la 0020 non cambia niente di quello che c'era: stesse righe, stesse viste.
func TestLa20NonCambiaIlFascicoloDiPrima(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	fino19(t, p, `
		INSERT INTO componente (componente_id, thread_id, codice, tipo, confermato_da) VALUES ('{K3}', '{T1}', 'P1', 'finito', '{U}');
		INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da)
		     VALUES ('{T1}', '{K3}', '{K1}', 3, 'manuale', '{U}');
		INSERT INTO deroga_fabbisogno (thread_id, componente_id, tipo, motivo, utente_id)
		     VALUES ('{T1}', '{K1}', 'sviluppo_dxf', 'lo facciamo noi', '{U}');
		INSERT INTO fase_log (fase_log_id, thread_id, nome_fase, inizio) VALUES ('{F1}', '{T1}', 'FATTIBILITA', now());`+
		doc20("{D1}", "{T1}", "{K1}", "disegno_2d", "V1", "1", `ELENCO DISEGNI\V1\a.pdf`)+
		doc20("{D2}", "{T1}", "{K3}", "cad_3d", "P1", "2", `ELENCO DISEGNI\P1\p.stp`)+
		doc20("{D3}", "{T1}", "", "capitolato", "", "3", `CAPITOLATI\c.pdf`))
	fotografia := func() string {
		return uno20[string](t, p, `SELECT concat_ws(' # ',
			(SELECT string_agg(row_to_json(v)::text, '|' ORDER BY row_to_json(v)::text) FROM v_fascicolo v),
			(SELECT string_agg(row_to_json(v)::text, '|' ORDER BY row_to_json(v)::text) FROM v_componente_albero v),
			(SELECT string_agg(row_to_json(v)::text, '|' ORDER BY row_to_json(v)::text) FROM v_thread_bloccanti v),
			(SELECT string_agg((to_jsonb(d))::text, '|' ORDER BY documento_id) FROM documento d),
			(SELECT string_agg((to_jsonb(c) - 'archiviato_il' - 'archiviato_da' - 'motivo_archiviazione' - 'step_strutturale_id')::text, '|' ORDER BY componente_id) FROM componente c),
			(SELECT string_agg((to_jsonb(f) - 'bom_versione_id')::text, '|' ORDER BY fase_log_id) FROM fase_log f))`)
	}
	prima := fotografia()
	if err := applica20(p); err != nil {
		t.Fatal(err)
	}
	if dopo := fotografia(); dopo != prima {
		t.Errorf("la 0020 ha cambiato il fascicolo di prima:\nprima %s\ndopo  %s", prima, dopo)
	}
	if n := uno20[int64](t, p, `SELECT count(*) FROM componente WHERE archiviato_il IS NOT NULL OR step_strutturale_id IS NOT NULL`); n != 0 {
		t.Errorf("%d componenti con colonne nuove valorizzate", n)
	}
}

// ------------------------------------------------------------------ schema 20

// schema20 ricrea lo schema completo con i dati di base e una RFQ di prova: in T1 il finito P1 (K1)
// con il figlio F1 (K2, ×2) e la fase FATTIBILITA aperta (F1); in T2 il finito Q1 (K3).
func schema20(t *testing.T) *pgxpool.Pool {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	esegui20(t, p, base18)
	esegui20(t, p, `
		INSERT INTO componente (componente_id, thread_id, codice, tipo, confermato_da) VALUES
		    ('{K1}', '{T1}', 'P1', 'finito', '{U}'), ('{K2}', '{T1}', 'F1', 'sciolto', '{U}'), ('{K3}', '{T2}', 'Q1', 'finito', '{U}');
		INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da)
		     VALUES ('{T1}', '{K1}', '{K2}', 2, 'manuale', '{U}');
		INSERT INTO fase_log (fase_log_id, thread_id, nome_fase, inizio) VALUES ('{F1}', '{T1}', 'FATTIBILITA', now() - interval '1 hour');`)
	return p
}

// congelaV1 fa a mano quello che fara' congelaBom sulla RFQ T1: la V1 in bozza, le istantanee dei
// componenti attivi, delle relazioni e dei documenti correnti, poi il congelamento.
func congelaV1(t *testing.T, p *pgxpool.Pool) {
	t.Helper()
	esegui20(t, p, `
		INSERT INTO bom_versione (bom_versione_id, thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da)
		     VALUES ('{V1}', '{T1}', 1, 'preventivo', 'FATTIBILITA', '{F1}', 'prima baseline', '{U}');
		INSERT INTO bom_versione_componente (bom_versione_id, thread_id, componente_id, codice, rev, tipo, descrizione, qta,
		                                     esito_fattibilita, note_fattibilita, step_strutturale_id, step_sha256)
		SELECT '{V1}', c.thread_id, c.componente_id, c.codice, c.rev, c.tipo, c.descrizione, c.qta, c.esito_fattibilita, c.note_fattibilita,
		       c.step_strutturale_id, d.sha256
		  FROM componente c LEFT JOIN documento d ON d.documento_id = c.step_strutturale_id
		 WHERE c.thread_id = '{T1}' AND c.archiviato_il IS NULL;
		INSERT INTO bom_versione_relazione (bom_versione_id, padre_id, figlio_id, qta, posizione)
		SELECT '{V1}', padre_id, figlio_id, qta, posizione FROM componente_relazione WHERE thread_id = '{T1}';
		INSERT INTO bom_versione_documento (bom_versione_id, componente_id, documento_id, tipo, rev, sha256, path_al_congelamento)
		SELECT '{V1}', componente_id, documento_id, tipo, rev, sha256, path_relativo FROM documento
		 WHERE thread_id = '{T1}' AND componente_id IS NOT NULL AND sostituito_da IS NULL;
		UPDATE bom_versione SET stato = 'congelata', congelata_da = '{U}', congelata_il = now() WHERE bom_versione_id = '{V1}';`)
}

// ------------------------------------------------------------------ documento: percorso e catena

func TestDueDocumentiNonDichiaranoLoStessoFile(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esegui20(t, p, doc20("{D1}", "{T1}", "{K1}", "cad_3d", "P1", "1", `ELENCO DISEGNI\P1\P1_REV_B.stp`))
	rifiuta(t, p, "ux_documento_percorso", doc20("{D2}", "{T1}", "", "capitolato", "", "2", `elenco disegni\p1\p1_rev_b.STP`))
	// lo stesso percorso in un'altra RFQ e' un altro file: il percorso e' relativo alla cartella del thread
	esegui20(t, p, doc20("{D2}", "{T2}", "", "capitolato", "", "2", `ELENCO DISEGNI\P1\P1_REV_B.stp`))
}

// Prova 48.
func TestLaCatenaDelleRevisioniNonFaCicli(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esegui20(t, p, doc20("{D1}", "{T1}", "{K1}", "disegno_2d", "P1", "1", `ELENCO DISEGNI\P1\a.pdf`)+
		doc20("{D2}", "{T1}", "{K1}", "disegno_2d", "P1", "2", `ELENCO DISEGNI\P1\b.pdf`)+
		doc20("{D3}", "{T1}", "{K1}", "disegno_2d", "P1", "3", `ELENCO DISEGNI\P1\c.pdf`)+
		`UPDATE documento SET sostituito_da = '{D2}' WHERE documento_id = '{D1}';
		 UPDATE documento SET sostituito_da = '{D3}' WHERE documento_id = '{D2}';`)
	rifiuta(t, p, "BOM03", `UPDATE documento SET sostituito_da = '{D1}' WHERE documento_id = '{D3}'`)
	rifiuta(t, p, "ck_documento_non_sostituisce_se_stesso", `UPDATE documento SET sostituito_da = '{D3}' WHERE documento_id = '{D3}'`)
	if got := uno20[string](t, p, `SELECT string_agg(right(documento_id::text, 1) || ':' || passo || ':' || corrente, ' ' ORDER BY passo)
	                                  FROM v_documento_storia WHERE catena_id = '{D1}'`); got != "1:1:false 2:2:false 3:3:true" {
		t.Errorf("storia: %s", got)
	}

	t.Run("due sostituzioni incrociate concorrenti", func(t *testing.T) {
		esegui20(t, p, doc20("{D4}", "{T1}", "{K1}", "disegno_2d", "P1", "4", `ELENCO DISEGNI\P1\d.pdf`)+
			doc20("{D5}", "{T1}", "{K1}", "disegno_2d", "P1", "5", `ELENCO DISEGNI\P1\e.pdf`))
		ctx := context.Background()
		tx1, err := p.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx1.Rollback(ctx)
		tx2, err := p.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx2.Rollback(ctx)
		if _, err := tx1.Exec(ctx, sostituzioni20.Replace(`UPDATE documento SET sostituito_da = '{D5}' WHERE documento_id = '{D4}'`)); err != nil {
			t.Fatal(err)
		}
		esito := make(chan error, 1)
		go func() {
			_, err := tx2.Exec(ctx, sostituzioni20.Replace(`UPDATE documento SET sostituito_da = '{D4}' WHERE documento_id = '{D5}'`))
			esito <- err
		}()
		aspettaUnLucchetto(t, p)
		if err := tx1.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-esito; codice(err) != "BOM03" {
			t.Fatalf("la seconda sostituzione incrociata doveva fermarsi sul trigger, ottenuto %v", err)
		}
		_ = tx2.Rollback(ctx)
		if got := uno20[string](t, p, `SELECT string_agg(right(documento_id::text, 1) || '→' || coalesce(right(sostituito_da::text, 1), '-'), ' ' ORDER BY documento_id)
		                                  FROM documento WHERE documento_id IN ('{D4}', '{D5}')`); got != "4→5 5→-" {
			t.Errorf("dopo le due sostituzioni incrociate: %s", got)
		}
	})
}

// aspettaUnLucchetto aspetta che una sessione sia ferma su un lucchetto: la seconda transazione e'
// davvero in fila dietro la prima, e la prova non dipende da un'attesa a tempo.
func aspettaUnLucchetto(t *testing.T, p *pgxpool.Pool) {
	t.Helper()
	limite := time.Now().Add(10 * time.Second)
	for time.Now().Before(limite) {
		var n int
		if err := p.QueryRow(context.Background(),
			`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("nessuna sessione in attesa di un lucchetto")
}

// Prova 49.
func TestUnaRevisioneHaLoStessoTipoELoStessoComponente(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esegui20(t, p, doc20("{D1}", "{T1}", "{K1}", "disegno_2d", "P1", "1", `ELENCO DISEGNI\P1\a.pdf`)+
		doc20("{D2}", "{T1}", "{K1}", "cad_3d", "P1", "2", `ELENCO DISEGNI\P1\a.stp`)+
		doc20("{D3}", "{T1}", "{K2}", "disegno_2d", "F1", "3", `ELENCO DISEGNI\F1\a.pdf`)+
		doc20("{D4}", "{T2}", "{K3}", "disegno_2d", "Q1", "4", `ELENCO DISEGNI\Q1\a.pdf`)+
		doc20("{D5}", "{T1}", "", "capitolato", "", "5", `CAPITOLATI\a.pdf`)+
		doc20("{D6}", "{T1}", "", "capitolato", "", "6", `CAPITOLATI\b.pdf`))
	rifiuta(t, p, "fk_documento_sostituito_stesso_tipo", `UPDATE documento SET sostituito_da = '{D2}' WHERE documento_id = '{D1}'`)
	rifiuta(t, p, "fk_documento_sostituito_stesso_tipo", `UPDATE documento SET sostituito_da = '{D3}' WHERE documento_id = '{D1}'`)
	rifiuta(t, p, "fk_documento_sostituito_stessa_rfq", `UPDATE documento SET sostituito_da = '{D4}' WHERE documento_id = '{D1}'`)
	rifiuta(t, p, "ck_documento_sostituito_ha_componente", `UPDATE documento SET sostituito_da = '{D6}' WHERE documento_id = '{D5}'`)
	// in una catena, tipo e componente non cambiano da nessuno dei due lati
	esegui20(t, p, `UPDATE documento SET componente_id = '{K1}', codice = 'P1' WHERE documento_id = '{D3}';
		UPDATE documento SET sostituito_da = '{D3}' WHERE documento_id = '{D1}';`)
	rifiuta(t, p, "fk_documento_sostituito_stesso_tipo", `UPDATE documento SET tipo = 'sviluppo_dxf' WHERE documento_id = '{D3}'`)
	rifiuta(t, p, "fk_documento_sostituito_stesso_tipo", `UPDATE documento SET componente_id = '{K2}', codice = 'F1' WHERE documento_id = '{D1}'`)
}

// Prova 50.
func TestUnaSostituzioneNonSiRiscrive(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esegui20(t, p, doc20("{D1}", "{T1}", "{K1}", "disegno_2d", "P1", "1", `ELENCO DISEGNI\P1\a.pdf`)+
		doc20("{D2}", "{T1}", "{K1}", "disegno_2d", "P1", "2", `ELENCO DISEGNI\P1\b.pdf`)+
		doc20("{D3}", "{T1}", "{K1}", "disegno_2d", "P1", "3", `ELENCO DISEGNI\P1\c.pdf`)+
		`UPDATE documento SET sostituito_da = '{D2}' WHERE documento_id = '{D1}';`)
	rifiuta(t, p, "BOM03", `UPDATE documento SET sostituito_da = '{D3}' WHERE documento_id = '{D1}'`)
	rifiuta(t, p, "BOM03", `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da, sostituito_da)
		VALUES ('{T1}', '{K1}', 'disegno_2d', 'P1', 'x.pdf', 'pdf', repeat('9', 64), 'ELENCO DISEGNI\P1\x.pdf', '{U}', '{D3}')`)
	rifiuta(t, p, "ux_documento_sostituito_da", `UPDATE documento SET sostituito_da = '{D2}' WHERE documento_id = '{D3}'`)
	// Y → NULL solo se Y e' ancora corrente
	esegui20(t, p, `UPDATE documento SET sostituito_da = '{D3}' WHERE documento_id = '{D2}'`)
	rifiuta(t, p, "BOM03", `UPDATE documento SET sostituito_da = NULL WHERE documento_id = '{D1}'`)
	esegui20(t, p, `UPDATE documento SET sostituito_da = NULL WHERE documento_id = '{D2}';
		UPDATE documento SET sostituito_da = NULL WHERE documento_id = '{D1}';`)
}

// ------------------------------------------------------------------ STEP strutturale e deroga strutturale

// Prova 40.
func TestIlRiferimentoDeveEssereUnoStepDelComponente(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esegui20(t, p, doc20("{D1}", "{T1}", "{K1}", "cad_3d", "P1", "1", `ELENCO DISEGNI\P1\a.stp`)+
		doc20("{D2}", "{T1}", "{K2}", "cad_3d", "F1", "2", `ELENCO DISEGNI\F1\a.stp`)+
		doc20("{D3}", "{T1}", "{K1}", "disegno_2d", "P1", "3", `ELENCO DISEGNI\P1\a.pdf`)+
		doc20("{D4}", "{T1}", "{K1}", "cad_3d", "P1", "4", `ELENCO DISEGNI\P1\b.stp`)+
		doc20("{D5}", "{T1}", "{K1}", "cad_3d", "P1", "5", `ELENCO DISEGNI\P1\c.igs`)+
		doc20("{D6}", "{T1}", "{K1}", "cad_3d", "P1", "6", `ELENCO DISEGNI\P1\d.stp`)+
		`UPDATE documento SET estensione = 'igs' WHERE documento_id = '{D5}';
		 UPDATE documento SET sostituito_da = '{D6}' WHERE documento_id = '{D4}';`)
	rifiuta(t, p, "fk_componente_step_strutturale", `UPDATE componente SET step_strutturale_id = '{D2}' WHERE componente_id = '{K1}'`)
	rifiuta(t, p, "ck_componente_step_solo_finito", `UPDATE componente SET step_strutturale_id = '{D2}' WHERE componente_id = '{K2}'`)
	rifiuta(t, p, "BOM04", `UPDATE componente SET step_strutturale_id = '{D3}' WHERE componente_id = '{K1}'`) // un 2D
	rifiuta(t, p, "BOM04", `UPDATE componente SET step_strutturale_id = '{D5}' WHERE componente_id = '{K1}'`) // un 3D che non e' STEP
	rifiuta(t, p, "BOM04", `UPDATE componente SET step_strutturale_id = '{D4}' WHERE componente_id = '{K1}'`) // sostituito
	esegui20(t, p, `UPDATE componente SET step_strutturale_id = '{D1}' WHERE componente_id = '{K1}'`)
	// finche' e' il riferimento, il documento non cambia componente
	rifiuta(t, p, "fk_componente_step_strutturale", `UPDATE documento SET componente_id = '{K2}', codice = 'F1' WHERE documento_id = '{D1}'`)
}

// La deroga strutturale: non si modifica, e le FK la legano a quel componente, a quello STEP con quello
// SHA, e a un'analisi che esiste (parte di database delle prove 62–64).
func TestLaDerogaStrutturaleELegataAlloStepEAllAnalisi(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esegui20(t, p, doc20("{D1}", "{T1}", "{K1}", "cad_3d", "P1", "1", `ELENCO DISEGNI\P1\a.stp`)+
		doc20("{D2}", "{T1}", "{K2}", "cad_3d", "F1", "2", `ELENCO DISEGNI\F1\a.stp`)+`
		INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES (repeat('1', 64), 2, repeat('h', 64), '{}');
		INSERT INTO deroga_struttura (deroga_struttura_id, thread_id, componente_id, step_documento_id, step_sha256,
		                              versione_analizzatore, hash_configurazione, analisi_calcolata_il, motivo_parziale, motivo, concessa_da)
		SELECT '{G1}', '{T1}', '{K1}', '{D1}', sha256, versione_analizzatore, hash_configurazione, calcolato_il, 'struttura v2', 'va bene cosi''', '{U}'
		  FROM analisi_fatti;`)
	rifiuta(t, p, "BOM05", `UPDATE deroga_struttura SET motivo = 'altro'`)
	nuova := func(comp, step, sha, v, h, il, motivo string) string {
		return `INSERT INTO deroga_struttura (thread_id, componente_id, step_documento_id, step_sha256, versione_analizzatore, hash_configurazione,
		                                      analisi_calcolata_il, motivo_parziale, motivo, concessa_da)
		        VALUES ('{T1}', '` + comp + `', '` + step + `', repeat('` + sha + `', 64), ` + v + `, ` + h + `, ` + il + `, 'x', '` + motivo + `', '{U}')`
	}
	rifiuta(t, p, "fk_deroga_struttura_step", nuova("{K1}", "{D2}", "2", "NULL", "NULL", "NULL", "m")) // STEP di un altro componente
	rifiuta(t, p, "fk_deroga_struttura_step", nuova("{K1}", "{D1}", "2", "NULL", "NULL", "NULL", "m")) // SHA diverso da quello del documento
	rifiuta(t, p, "fk_deroga_struttura_analisi", nuova("{K1}", "{D1}", "1", "3", "repeat('h', 64)", "now()", "m"))
	rifiuta(t, p, "ck_deroga_struttura_analisi", nuova("{K1}", "{D1}", "1", "2", "repeat('h', 64)", "NULL", "m"))
	rifiuta(t, p, "deroga_struttura_motivo_check", nuova("{K1}", "{D1}", "1", "NULL", "NULL", "NULL", "  "))
	esegui20(t, p, nuova("{K1}", "{D1}", "1", "NULL", "NULL", "NULL", "non ancora analizzato"))
}

// Prova 23: ogni esito di v_step_prodotto.
func TestLoStepDelFinitoMancanteSiVede(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esito := func() string {
		t.Helper()
		return uno20[string](t, p, `SELECT esito || coalesce(' | ' || motivo_parziale, '') FROM v_step_prodotto WHERE componente_id = '{K1}'`)
	}
	if got := esito(); got != "mancante" {
		t.Errorf("senza niente: %s", got)
	}
	esegui20(t, p, `INSERT INTO riferimento_portale (messaggio_id, thread_id, codice, stato, tipo_atteso, testo_citato)
		VALUES ('{M}', '{T1}', 'p1', 'da_scaricare', 'cad_3d', 'scaricate il 3D dal portale')`)
	if got := esito(); got != "sul_portale" {
		t.Errorf("con il riferimento del portale: %s", got)
	}
	esegui20(t, p, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte)
		VALUES ('{A1}', '{T1}', 'cad_3d', 'P1', 80, 'step')`)
	if got := esito(); got != "da_confermare" {
		t.Errorf("con la proposta aperta: %s", got)
	}
	esegui20(t, p, doc20("{D1}", "{T1}", "{K1}", "cad_3d", "P1", "1", `ELENCO DISEGNI\P1\a.igs`)+
		`UPDATE documento SET estensione = 'igs' WHERE documento_id = '{D1}';`)
	if got := esito(); got != "solo_altro_3d" {
		t.Errorf("con un IGES: %s", got)
	}
	esegui20(t, p, doc20("{D2}", "{T1}", "{K1}", "cad_3d", "P1", "2", `ELENCO DISEGNI\P1\a.stp`))
	if got := esito(); got != "da_scegliere" {
		t.Errorf("con uno STEP non scelto: %s", got)
	}
	esegui20(t, p, `UPDATE componente SET step_strutturale_id = '{D2}' WHERE componente_id = '{K1}'`)
	if got := esito(); got != "presente_non_analizzato | nessuna analisi corrente dello STEP" {
		t.Errorf("scelto, senza analizzatore corrente: %s", got)
	}
	esegui20(t, p, `INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES (3, repeat('h', 64));
		INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti)
		     VALUES (repeat('2', 64), 2, repeat('h', 64), '{"struttura": {"versione": 2}}')`)
	if got := esito(); got != "presente_non_analizzato | nessuna analisi corrente dello STEP" {
		t.Errorf("analizzato solo con una versione vecchia: %s", got)
	}
	esegui20(t, p, `INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti)
		     VALUES (repeat('2', 64), 3, repeat('h', 64), '{"struttura": {"versione": 3, "radici": ["#1"], "relazioni": [], "avvisi": [],
		              "limiti": {"troncato": false}, "scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0,
		              "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}')`)
	if got := esito(); got != "presente_parziale | nessuna occorrenza di assieme: solo parti" {
		t.Errorf("analisi corrente di un file di sole parti: %s", got)
	}
	esegui20(t, p, `UPDATE analisi_fatti SET fatti = jsonb_set(fatti, '{struttura,relazioni}', '[{"padre": "#1", "figlio": "#2", "qta": 2}]')
		WHERE versione_analizzatore = 3`)
	if got := esito(); got != "presente_analizzato" {
		t.Errorf("analisi corrente completa: %s", got)
	}
	if !uno20[bool](t, p, `SELECT analisi_completa FROM v_step_prodotto WHERE componente_id = '{K1}'`) {
		t.Error("analisi_completa falsa con un'analisi completa")
	}
	esegui20(t, p, doc20("{D3}", "{T1}", "{K1}", "cad_3d", "P1", "3", `ELENCO DISEGNI\P1\b.stp`)+
		`UPDATE documento SET sostituito_da = '{D3}' WHERE documento_id = '{D2}';`)
	if got := esito(); got != "riferimento_superato" {
		t.Errorf("riferimento sostituito: %s", got)
	}
	// una radice che non e' un finito e' un avviso; un figlio non e' una riga
	esegui20(t, p, `INSERT INTO componente (componente_id, thread_id, codice, tipo, confermato_da) VALUES ('{K4}', '{T1}', 'S1', 'sottoassieme', '{U}')`)
	if got := uno20[string](t, p, `SELECT string_agg(c.codice || ':' || s.esito, ' ' ORDER BY c.codice) FROM v_step_prodotto s JOIN componente c USING (componente_id)
	                                  WHERE s.thread_id = '{T1}'`); got != "P1:riferimento_superato S1:radice_senza_qualifica" {
		t.Errorf("righe della RFQ: %s", got)
	}
}

// struttura_completa (A4.4, D35): una lettura v3 senza scarti che contano, con una radice e almeno
// un'occorrenza di assieme. Parte di database della prova 35, che arriva con B8.5.
func TestStrutturaCompletaVuoleLaV3SenzaScarti(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPresente(t, p)
	buona := `{"versione": 3, "radici": ["#1"], "relazioni": [{"padre": "#1", "figlio": "#2", "qta": 1}], "avvisi": [],
	           "limiti": {"troncato": false}, "scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0,
	           "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}`
	casi := []struct{ nome, modifica, motivo string }{
		{"completa", `$1`, ""},
		{"le occorrenze su se stesse non contano", `jsonb_set($1, '{scarti,occorrenze_su_se_stesse}', '7')`, ""},
		{"nessuna struttura", `NULL`, "nessuna struttura nei fatti"},
		{"lettura fallita", `jsonb_set($1, '{avvisi}', '["struttura non letta: file corrotto"]')`, "struttura non letta"},
		{"non STEP", `jsonb_set($1, '{avvisi}', '["non e'' un file STEP Part 21"]')`, "struttura non letta"},
		{"v2", `jsonb_set($1, '{versione}', '2')`, "struttura v2: gli scarti non sono numerati, serve la rianalisi v3"},
		{"troncata", `jsonb_set($1, '{limiti}', '{"troncato": true, "motivo": "troppi nodi"}')`, "lettura troncata: troppi nodi"},
		{"troncato non indicato", `$1 #- '{limiti,troncato}'`, "lettura troncata: motivo non indicato"},
		{"scarti mancanti", `$1 - 'scarti'`, "scarti non indicati"},
		{"prodotti senza definizione", `jsonb_set($1, '{scarti,prodotti_senza_definizione}', '1')`, "1 PRODUCT senza PRODUCT_DEFINITION"},
		{"occorrenze non risolte", `jsonb_set($1, '{scarti,occorrenze_non_risolte}', '2')`, "2 occorrenze con estremi non risolti"},
		{"testi troncati", `jsonb_set($1, '{scarti,testi_troncati}', '3')`, "3 testi troncati"},
		{"due radici", `jsonb_set($1, '{radici}', '["#1", "#9"]')`, "2 radici nel file: una distinta ne ha una"},
		{"sole parti", `jsonb_set($1, '{relazioni}', '[]')`, "nessuna occorrenza di assieme: solo parti"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			var motivo *string
			var completa bool
			q := `WITH b AS (SELECT $1::jsonb AS j)
			      SELECT struttura_motivo_parziale(x), struttura_completa(x) FROM (SELECT (` + strings.ReplaceAll(c.modifica, "$1", "j") + `)::jsonb AS x FROM b) s`
			if err := p.QueryRow(context.Background(), q, buona).Scan(&motivo, &completa); err != nil {
				t.Fatal(err)
			}
			got := ""
			if motivo != nil {
				got = *motivo
			}
			if got != c.motivo || completa != (c.motivo == "") {
				t.Errorf("motivo %q completa %v, atteso %q", got, completa, c.motivo)
			}
		})
	}
}

// ------------------------------------------------------------------ versioni e istantanee

// Prova 10.
func TestUnaBaselineCongelataNonSiModifica(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esegui20(t, p, doc20("{D1}", "{T1}", "{K2}", "disegno_2d", "F1", "1", `ELENCO DISEGNI\F1\a.pdf`)+
		`INSERT INTO componente (componente_id, thread_id, codice, confermato_da) VALUES ('{K4}', '{T1}', 'S9', '{U}');`)
	congelaV1(t, p)
	for _, c := range []struct{ nome, sql string }{
		{"UPDATE della versione", `UPDATE bom_versione SET motivo = 'altro' WHERE bom_versione_id = '{V1}'`},
		{"ritorno in bozza", `UPDATE bom_versione SET stato = 'bozza', congelata_da = NULL, congelata_il = NULL WHERE bom_versione_id = '{V1}'`},
		{"DELETE della versione", `DELETE FROM bom_versione WHERE bom_versione_id = '{V1}'`},
		{"UPDATE di un componente dell'istantanea", `UPDATE bom_versione_componente SET qta = 9`},
		{"DELETE di una relazione dell'istantanea", `DELETE FROM bom_versione_relazione`},
		{"INSERT di una relazione nell'istantanea", `INSERT INTO bom_versione_relazione (bom_versione_id, padre_id, figlio_id, qta) VALUES ('{V1}', '{K2}', '{K1}', 1)`},
		{"UPDATE di un documento dell'istantanea", `UPDATE bom_versione_documento SET path_al_congelamento = 'altrove'`},
		{"INSERT di una deroga nell'istantanea", `INSERT INTO bom_versione_deroga (bom_versione_id, componente_id, deroga_id, tipo, motivo, utente_id, creata_il)
			VALUES ('{V1}', '{K2}', gen_random_uuid(), 'sviluppo_dxf', 'x', '{U}', now())`},
		{"TRUNCATE di un'istantanea", `TRUNCATE bom_versione_documento`},
		{"TRUNCATE di tutte", `TRUNCATE bom_versione_componente, bom_versione_relazione, bom_versione_documento, bom_versione_deroga`},
	} {
		t.Run(c.nome, func(t *testing.T) { rifiuta(t, p, "BOM02", c.sql) })
	}
	// Nessuna cascata: cio' che sta in una baseline non si cancella da sotto. Con la V1 congelata e
	// nessuna bozza lo ferma prima la working bloccata (BOM01); con una revisione aperta, le FK.
	rifiuta(t, p, "BOM01", `DELETE FROM documento WHERE documento_id = '{D1}'`)
	esegui20(t, p, `UPDATE fase_log SET fine = now(), esito = 'OK' WHERE fase_log_id = '{F1}';
		INSERT INTO fase_log (fase_log_id, thread_id, nome_fase, inizio) VALUES ('{F2}', '{T1}', 'SCHEDA_COSTO', now());
		INSERT INTO bom_versione (bom_versione_id, thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da,
		                          versione_precedente_id, numero_precedente, stato_precedente)
		     VALUES ('{V2}', '{T1}', 2, 'preventivo', 'SCHEDA_COSTO', '{F2}', 'revisione', '{U}', '{V1}', 1, 'congelata');`)
	rifiuta(t, p, "bom_versione_documento_documento_id_fkey", `DELETE FROM documento WHERE documento_id = '{D1}'`)
	rifiuta(t, p, "fk_bvc_componente", `DELETE FROM componente WHERE componente_id = '{K4}'`)
	if n := uno20[int64](t, p, `SELECT (SELECT count(*) FROM bom_versione_componente) * 100 + (SELECT count(*) FROM bom_versione_relazione) * 10
	                                  + (SELECT count(*) FROM bom_versione_documento)`); n != 311 {
		t.Errorf("istantanee cambiate: %d", n)
	}
}

// Una bozza si cancella o si congela, e il congelamento cambia solo stato, congelata_da e congelata_il.
func TestUnaBozzaSiPuoSoloCongelare(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esegui20(t, p, `INSERT INTO bom_versione (bom_versione_id, thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da)
		VALUES ('{V1}', '{T1}', 1, 'preventivo', 'FATTIBILITA', '{F1}', 'prima baseline', '{U}')`)
	rifiuta(t, p, "BOM02", `UPDATE bom_versione SET motivo = 'altro'`)
	rifiuta(t, p, "BOM02", `UPDATE bom_versione SET numero = 2`)
	rifiuta(t, p, "BOM02", `UPDATE bom_versione SET stato = 'congelata', congelata_da = '{U}', congelata_il = now(), contesto = 'tecnica'`)
	rifiuta(t, p, "ck_bom_versione_congelata", `UPDATE bom_versione SET stato = 'congelata'`)
	esegui20(t, p, `DELETE FROM bom_versione`) // l'abbandono
	// una versione nasce bozza: gia' congelata sarebbe una baseline senza istantanee
	rifiuta(t, p, "BOM02", `INSERT INTO bom_versione (thread_id, numero, stato, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da,
		congelata_da, congelata_il) VALUES ('{T1}', 1, 'congelata', 'preventivo', 'FATTIBILITA', '{F1}', 'x', '{U}', '{U}', now())`)
	congelaV1(t, p)
}

// Prova 72.
func TestLaCatenaDelleVersioniNonHaSalti(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esegui20(t, p, `
		UPDATE fase_log SET fine = now() - interval '30 minutes', esito = 'OK' WHERE fase_log_id = '{F1}';
		INSERT INTO fase_log (fase_log_id, thread_id, nome_fase, inizio) VALUES ('{F2}', '{T1}', 'SCHEDA_COSTO', now() - interval '30 minutes');
		INSERT INTO fase_log (fase_log_id, thread_id, nome_fase, inizio) VALUES ('{F3}', '{T2}', 'FATTIBILITA', now());`)
	versione := func(id, thread string, numero int, fase, riga, prec string, numPrec, statoPrec string) string {
		precedente := "NULL"
		if prec != "" {
			precedente = "'" + prec + "'"
		}
		return fmt.Sprintf(`INSERT INTO bom_versione (bom_versione_id, thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da,
		                                  versione_precedente_id, numero_precedente, stato_precedente)
		        VALUES ('%s', '%s', %d, 'preventivo', '%s', '%s', 'x', '{U}', %s, %s, %s)`, id, thread, numero, fase, riga, precedente, numPrec, statoPrec)
	}
	// la V1 della seconda RFQ, congelata
	esegui20(t, p, versione("{VX}", "{T2}", 1, "FATTIBILITA", "{F3}", "", "NULL", "NULL")+
		`; UPDATE bom_versione SET stato = 'congelata', congelata_da = '{U}', congelata_il = now() WHERE bom_versione_id = '{VX}'`)
	// La V1 di T1 in bozza: nessuna V2 la puo' referenziare. Una V2 nasce bozza anche lei, e due
	// bozze della stessa RFQ non ci stanno; la FK con lo stato sarebbe la seconda linea.
	esegui20(t, p, versione("{V1}", "{T1}", 1, "FATTIBILITA", "{F1}", "", "NULL", "NULL"))
	rifiuta(t, p, "ux_bom_una_bozza", versione("{V2}", "{T1}", 2, "SCHEDA_COSTO", "{F2}", "{V1}", "1", "'congelata'"))
	esegui20(t, p, `UPDATE bom_versione SET stato = 'congelata', congelata_da = '{U}', congelata_il = now() WHERE bom_versione_id = '{V1}'`)
	rifiuta(t, p, "fk_bom_versione_precedente", versione("{V2}", "{T1}", 2, "SCHEDA_COSTO", "{F2}", "{VX}", "1", "'congelata'")) // un'altra RFQ
	rifiuta(t, p, "ck_bom_versione_precedente", versione("{V2}", "{T1}", 2, "SCHEDA_COSTO", "{F2}", "", "NULL", "NULL"))         // senza precedente
	rifiuta(t, p, "ck_bom_versione_precedente", versione("{V2}", "{T1}", 2, "SCHEDA_COSTO", "{F2}", "{V1}", "1", "'bozza'"))     // precedente dichiarato in bozza
	rifiuta(t, p, "ck_bom_versione_precedente", versione("{V3}", "{T1}", 3, "SCHEDA_COSTO", "{F2}", "{V1}", "1", "'congelata'")) // un salto
	rifiuta(t, p, "fk_bom_versione_precedente", versione("{V3}", "{T1}", 3, "SCHEDA_COSTO", "{F2}", "{V1}", "2", "'congelata'")) // V3 → V1 con il numero di V2
	rifiuta(t, p, "ck_bom_versione_precedente", `INSERT INTO bom_versione (thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da,
		versione_precedente_id, numero_precedente, stato_precedente) VALUES ('{T1}', 1, 'preventivo', 'FATTIBILITA', '{F1}', 'x', '{U}', '{VX}', 0, 'congelata')`)
	esegui20(t, p, versione("{V2}", "{T1}", 2, "SCHEDA_COSTO", "{F2}", "{V1}", "1", "'congelata'"))
	rifiuta(t, p, "ux_bom_versione_numero", versione("{V3}", "{T1}", 2, "SCHEDA_COSTO", "{F2}", "{V1}", "1", "'congelata'")) // una seconda V2
	rifiuta(t, p, "BOM02", `UPDATE bom_versione SET numero_precedente = 1 WHERE bom_versione_id = '{V2}'`)
	rifiuta(t, p, "BOM02", `UPDATE bom_versione SET versione_precedente_id = '{VX}' WHERE bom_versione_id = '{V2}'`)
	if got := uno20[string](t, p, `SELECT string_agg(numero || ':' || stato || ':' || superata || ':' || corrente, ' ' ORDER BY numero) FROM v_bom_versioni WHERE thread_id = '{T1}'`); got != "1:congelata:false:true 2:bozza:false:false" {
		t.Errorf("v_bom_versioni: %s", got)
	}
}

// Prove 65 (il CHECK del confine) e 67 (la FK fra il nome della fase e la sua riga).
func TestIlConfineFraLeRevisioniStaNelDatabase(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esegui20(t, p, `UPDATE fase_log SET fine = now() - interval '50 minutes', esito = 'OK' WHERE fase_log_id = '{F1}';
		INSERT INTO fase_log (fase_log_id, thread_id, nome_fase, inizio, fine, esito)
		     VALUES ('{F2}', '{T1}', 'OFFERTA_INVIATA', now() - interval '50 minutes', now() - interval '40 minutes', 'OK'),
		            ('{F3}', '{T1}', 'ACCETTATA', now() - interval '40 minutes', now() - interval '30 minutes', 'OK');
		INSERT INTO fase_log (fase_log_id, thread_id, nome_fase, inizio) VALUES ('{F4}', '{T1}', 'ORDINE', now() - interval '30 minutes');`)
	v1 := func(contesto, fase, riga string) string {
		return `INSERT INTO bom_versione (bom_versione_id, thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da)
		        VALUES ('{V1}', '{T1}', 1, '` + contesto + `', '` + fase + `', '` + riga + `', 'x', '{U}')`
	}
	v2 := func(contesto, fase, riga string) string {
		return `INSERT INTO bom_versione (bom_versione_id, thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da,
		                                  versione_precedente_id, numero_precedente, stato_precedente)
		        VALUES ('{V2}', '{T1}', 2, '` + contesto + `', '` + fase + `', '` + riga + `', 'x', '{U}', '{V1}', 1, 'congelata')`
	}
	rifiuta(t, p, "ck_bom_versione_confine", v1("preventivo", "ACCETTATA", "{F3}"))
	rifiuta(t, p, "ck_bom_versione_confine", v1("tecnica", "ACCETTATA", "{F3}"))
	rifiuta(t, p, "ck_bom_versione_confine", v1("tecnica", "FATTIBILITA", "{F1}"))
	rifiuta(t, p, "ck_bom_versione_confine", v1("preventivo", "ORDINE", "{F4}"))
	rifiuta(t, p, "fk_bom_versione_fase", v1("tecnica", "ORDINE", "{F3}")) // il nome non e' quello della riga
	esegui20(t, p, v1("preventivo", "FATTIBILITA", "{F1}")+`;
		UPDATE bom_versione SET stato = 'congelata', congelata_da = '{U}', congelata_il = now() WHERE bom_versione_id = '{V1}';`)
	rifiuta(t, p, "ck_bom_versione_confine", v2("tecnica", "OFFERTA_INVIATA", "{F2}"))
	rifiuta(t, p, "ck_bom_versione_confine", v2("preventivo", "ORDINE", "{F4}"))
	rifiuta(t, p, "ck_bom_versione_confine", v2("preventivo", "FATTIBILITA", "{F1}"))
	// in ACCETTATA e DISTINTA_ERP il contesto lo sceglie una persona: tutti e due passano
	esegui20(t, p, v2("tecnica", "ACCETTATA", "{F3}")+`; DELETE FROM bom_versione WHERE bom_versione_id = '{V2}';`)
	esegui20(t, p, v2("preventivo", "ACCETTATA", "{F3}")+`; DELETE FROM bom_versione WHERE bom_versione_id = '{V2}';`)
	esegui20(t, p, v2("tecnica", "ORDINE", "{F4}"))
	rifiuta(t, p, "ck_fase_log_bom_solo_scheda", `UPDATE fase_log SET bom_versione_id = '{V1}' WHERE fase_log_id = '{F3}'`)
}

// ------------------------------------------------------------------ la working bloccata (D26)

func TestLaWorkingCongelataNonSiModificaSenzaRevisione(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esegui20(t, p, doc20("{D1}", "{T1}", "{K1}", "cad_3d", "P1", "1", `ELENCO DISEGNI\P1\a.stp`)+
		doc20("{D2}", "{T1}", "{K1}", "cad_3d", "P1", "2", `ELENCO DISEGNI\P1\b.stp`)+`
		INSERT INTO componente (componente_id, thread_id, codice, confermato_da) VALUES ('{K4}', '{T1}', 'S9', '{U}');`)
	congelaV1(t, p)
	for _, c := range []struct{ nome, sql string }{
		{"nuovo componente", `INSERT INTO componente (thread_id, codice, confermato_da) VALUES ('{T1}', 'N1', '{U}')`},
		{"descrizione di un componente", `UPDATE componente SET descrizione = 'staffa' WHERE componente_id = '{K2}'`},
		{"codice di un componente", `UPDATE componente SET codice = 'F2' WHERE componente_id = '{K4}'`},
		{"archiviazione", `UPDATE componente SET archiviato_il = now(), archiviato_da = '{U}', motivo_archiviazione = 'tolto' WHERE componente_id = '{K4}'`},
		{"scelta dello STEP strutturale", `UPDATE componente SET step_strutturale_id = '{D1}' WHERE componente_id = '{K1}'`},
		{"cancellazione di un componente", `DELETE FROM componente WHERE componente_id = '{K4}'`},
		{"nuovo arco", `INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, origine, confermato_da) VALUES ('{T1}', '{K1}', '{K4}', 'manuale', '{U}')`},
		{"qta di un arco", `UPDATE componente_relazione SET qta = 3`},
		{"arco tolto", `DELETE FROM componente_relazione`},
	} {
		t.Run(c.nome, func(t *testing.T) { rifiuta(t, p, "BOM01", c.sql) })
	}
	// un UPDATE che non cambia niente non cambia la BOM; un'altra RFQ non c'entra
	esegui20(t, p, `UPDATE componente SET codice = codice WHERE componente_id = '{K2}';
		INSERT INTO componente (thread_id, codice, confermato_da) VALUES ('{T2}', 'N1', '{U}');`)
	// una revisione aperta la libera
	esegui20(t, p, `UPDATE fase_log SET fine = now(), esito = 'OK' WHERE fase_log_id = '{F1}';
		INSERT INTO fase_log (fase_log_id, thread_id, nome_fase, inizio) VALUES ('{F2}', '{T1}', 'SCHEDA_COSTO', now());
		INSERT INTO bom_versione (bom_versione_id, thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da,
		                          versione_precedente_id, numero_precedente, stato_precedente)
		     VALUES ('{V2}', '{T1}', 2, 'preventivo', 'SCHEDA_COSTO', '{F2}', 'revisione', '{U}', '{V1}', 1, 'congelata');
		UPDATE componente SET step_strutturale_id = '{D1}' WHERE componente_id = '{K1}';
		INSERT INTO componente (thread_id, codice, confermato_da) VALUES ('{T1}', 'N1', '{U}');`)
	// l'abbandono della bozza la blocca di nuovo
	esegui20(t, p, `DELETE FROM bom_versione WHERE bom_versione_id = '{V2}'`)
	rifiuta(t, p, "BOM01", `UPDATE componente SET descrizione = 'staffa' WHERE componente_id = '{K2}'`)
}

// Prova 70, la parte del trigger: documenti assegnati e deroghe.
func TestLaWorkingCongelataProteggeDocumentiEDeroghe(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esegui20(t, p, doc20("{D1}", "{T1}", "{K2}", "disegno_2d", "F1", "1", `ELENCO DISEGNI\F1\a.pdf`)+
		doc20("{D2}", "{T1}", "{K2}", "disegno_2d", "F1", "2", `ELENCO DISEGNI\F1\b.pdf`)+
		doc20("{D3}", "{T1}", "", "capitolato", "", "3", `CAPITOLATI\c.pdf`)+
		doc20("{D4}", "{T1}", "{K1}", "cad_3d", "P1", "4", `ELENCO DISEGNI\P1\a.stp`)+`
		INSERT INTO deroga_fabbisogno (deroga_id, thread_id, componente_id, tipo, motivo, utente_id)
		     VALUES ('{G1}', '{T1}', '{K2}', 'sviluppo_dxf', 'lo facciamo noi', '{U}');
		INSERT INTO deroga_struttura (thread_id, componente_id, step_documento_id, step_sha256, motivo_parziale, motivo, concessa_da)
		     VALUES ('{T1}', '{K1}', '{D4}', repeat('4', 64), 'non analizzato', 'si congela cosi''', '{U}');`)
	congelaV1(t, p)
	rifiutati := []struct{ nome, sql string }{
		{"documento inserito con componente", `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
			VALUES ('{T1}', '{K2}', 'sviluppo_dxf', 'F1', 'f.dxf', 'dxf', repeat('5', 64), 'SVILUPPI\F1\f.dxf', '{U}')`},
		{"DELETE di un documento assegnato", `DELETE FROM documento WHERE documento_id = '{D2}'`},
		{"componente_id", `UPDATE documento SET componente_id = NULL, codice = 'F1' WHERE documento_id = '{D2}'`},
		{"assegnare un documento senza componente", `UPDATE documento SET componente_id = '{K2}', codice = 'F1' WHERE documento_id = '{D3}'`},
		{"tipo", `UPDATE documento SET tipo = 'sviluppo_dxf' WHERE documento_id = '{D2}'`},
		{"codice", `UPDATE documento SET codice = 'f1' WHERE documento_id = '{D2}'`},
		{"rev", `UPDATE documento SET rev = 'B' WHERE documento_id = '{D2}'`},
		{"estensione", `UPDATE documento SET estensione = 'PDF' WHERE documento_id = '{D2}'`},
		{"sha256", `UPDATE documento SET sha256 = repeat('7', 64) WHERE documento_id = '{D2}'`},
		{"sostituito_da", `UPDATE documento SET sostituito_da = '{D1}' WHERE documento_id = '{D2}'`},
		{"INSERT di una deroga del fabbisogno", `INSERT INTO deroga_fabbisogno (thread_id, componente_id, tipo, motivo, utente_id) VALUES ('{T1}', '{K1}', 'disegno_2d', 'x', '{U}')`},
		{"DELETE di una deroga del fabbisogno", `DELETE FROM deroga_fabbisogno WHERE deroga_id = '{G1}'`},
		{"INSERT di una deroga strutturale", `INSERT INTO deroga_struttura (thread_id, componente_id, step_documento_id, step_sha256, motivo_parziale, motivo, concessa_da)
			VALUES ('{T1}', '{K1}', '{D4}', repeat('4', 64), 'x', 'y', '{U}')`},
		{"DELETE di una deroga strutturale", `DELETE FROM deroga_struttura`},
	}
	for _, c := range rifiutati {
		t.Run(c.nome, func(t *testing.T) { rifiuta(t, p, "BOM01", c.sql) })
	}
	// lo stato del NAS resta libero: una copia o uno spostamento accodati prima del congelamento finiscono
	esegui20(t, p, `
		UPDATE documento SET path_relativo = 'ELENCO DISEGNI\F1\F1_REV_ND.pdf', stato_nas = 'scritto', scritto_il = now(), errore_nas = NULL,
		                     verificato_il = now(), nome_file = 'originale.pdf', nota = 'ok' WHERE documento_id = '{D2}';
		INSERT INTO documento (thread_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		     VALUES ('{T1}', 'disegno_2d', 'X9', 'x9.pdf', 'pdf', repeat('6', 64), 'ELENCO DISEGNI\X9\x9.pdf', '{U}');
		UPDATE documento SET rev = 'B' WHERE documento_id = '{D3}';
		DELETE FROM documento WHERE documento_id = '{D3}';`)
}

// ------------------------------------------------------------------ baseline: STEP e deroga (R2.5, D36)

// Prova 66 e la parte di database della 64.
func TestLaBaselineRifiutaUnoStepDiUnAltroComponente(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esegui20(t, p, doc20("{D1}", "{T1}", "{K1}", "cad_3d", "P1", "1", `ELENCO DISEGNI\P1\a.stp`)+
		doc20("{D2}", "{T1}", "{K2}", "cad_3d", "F1", "2", `ELENCO DISEGNI\F1\a.stp`)+`
		INSERT INTO componente (componente_id, thread_id, codice, tipo, confermato_da) VALUES ('{K4}', '{T1}', 'P2', 'finito', '{U}');`+
		doc20("{D5}", "{T1}", "{K4}", "cad_3d", "P2", "5", `ELENCO DISEGNI\P2\a.stp`)+`
		INSERT INTO deroga_struttura (deroga_struttura_id, thread_id, componente_id, step_documento_id, step_sha256, motivo_parziale, motivo, concessa_da)
		     VALUES ('{G1}', '{T1}', '{K1}', '{D1}', repeat('1', 64), 'non analizzato', 'si congela cosi''', '{U}');
		INSERT INTO bom_versione (bom_versione_id, thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da)
		     VALUES ('{V1}', '{T1}', 1, 'preventivo', 'FATTIBILITA', '{F1}', 'prima baseline', '{U}');`)
	riga := func(comp, codice, step, sha, deroga string) string {
		return `INSERT INTO bom_versione_componente (bom_versione_id, thread_id, componente_id, codice, tipo, qta, step_strutturale_id, step_sha256, deroga_struttura_id)
		        VALUES ('{V1}', '{T1}', '` + comp + `', '` + codice + `', 'finito', 1, ` + step + `, ` + sha + `, ` + deroga + `)`
	}
	rifiuta(t, p, "fk_bvc_step", riga("{K1}", "P1", "'{D2}'", "repeat('2', 64)", "NULL"))       // STEP di un altro componente
	rifiuta(t, p, "fk_bvc_step", riga("{K1}", "P1", "'{D1}'", "repeat('9', 64)", "NULL"))       // SHA diverso da quello del documento
	rifiuta(t, p, "ck_bvc_step_con_sha", riga("{K1}", "P1", "'{D1}'", "NULL", "NULL"))          // STEP senza SHA
	rifiuta(t, p, "ck_bvc_step_con_sha", riga("{K1}", "P1", "NULL", "repeat('1', 64)", "NULL")) // SHA senza STEP
	rifiuta(t, p, "ck_bvc_deroga_con_step", riga("{K1}", "P1", "NULL", "NULL", "'{G1}'"))       // deroga senza STEP
	rifiuta(t, p, "fk_bvc_deroga", riga("{K2}", "F1", "'{D2}'", "repeat('2', 64)", "'{G1}'"))   // deroga di un altro componente
	esegui20(t, p, riga("{K1}", "P1", "'{D1}'", "repeat('1', 64)", "'{G1}'")+`;`+
		riga("{K4}", "P2", "'{D5}'", "repeat('5', 64)", "NULL")+`;
		UPDATE bom_versione SET stato = 'congelata', congelata_da = '{U}', congelata_il = now();`)
	// la deroga e lo STEP di una baseline non si cancellano, e lo STEP non cambia componente
	esegui20(t, p, `INSERT INTO fase_log (fase_log_id, thread_id, nome_fase, inizio, fine, esito) VALUES ('{F2}', '{T1}', 'SCHEDA_COSTO', now(), now(), 'OK');
		INSERT INTO bom_versione (bom_versione_id, thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da,
		                          versione_precedente_id, numero_precedente, stato_precedente)
		     VALUES ('{V2}', '{T1}', 2, 'preventivo', 'SCHEDA_COSTO', '{F2}', 'revisione', '{U}', '{V1}', 1, 'congelata');`)
	rifiuta(t, p, "fk_bvc_deroga", `DELETE FROM deroga_struttura WHERE deroga_struttura_id = '{G1}'`)
	// D5 lo tiene solo la baseline: la riassegnazione la ferma la FK dello STEP (prova 69, lato database)
	rifiuta(t, p, "fk_bvc_step", `UPDATE documento SET componente_id = '{K2}', codice = 'F1' WHERE documento_id = '{D5}'`)
}

// ------------------------------------------------------------------ archiviazione nelle viste (A4.9)

func TestUnComponenteArchiviatoEsceDallaWorking(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	albero := func() string {
		return uno20[string](t, p, `SELECT coalesce(string_agg(c.codice || '@' || a.profondita, ' ' ORDER BY c.codice), '') FROM v_componente_albero a
		                                  JOIN componente c USING (componente_id) WHERE a.thread_id = '{T1}'`)
	}
	fascicolo := func() string {
		return uno20[string](t, p, `SELECT coalesce(string_agg(DISTINCT codice, ' ' ORDER BY codice), '') FROM v_fascicolo WHERE thread_id = '{T1}'`)
	}
	if a, f := albero(), fascicolo(); a != "F1@1 P1@0" || f != "F1 P1" {
		t.Fatalf("prima: albero %q, fascicolo %q", a, f)
	}
	rifiuta(t, p, "ck_componente_archiviazione", `UPDATE componente SET archiviato_il = now() WHERE componente_id = '{K2}'`)
	rifiuta(t, p, "ck_componente_archiviazione", `UPDATE componente SET archiviato_il = now(), archiviato_da = '{U}', motivo_archiviazione = ' ' WHERE componente_id = '{K2}'`)
	esegui20(t, p, `UPDATE componente SET archiviato_il = now(), archiviato_da = '{U}', motivo_archiviazione = 'tolto dal cliente' WHERE componente_id = '{K2}'`)
	if a, f := albero(), fascicolo(); a != "P1@0" || f != "P1" {
		t.Errorf("figlio archiviato: albero %q, fascicolo %q", a, f)
	}
	// un padre archiviato con l'arco ancora li': il figlio diventa una radice da sistemare, non sparisce
	esegui20(t, p, `UPDATE componente SET archiviato_il = NULL, archiviato_da = NULL, motivo_archiviazione = NULL WHERE componente_id = '{K2}';
		UPDATE componente SET archiviato_il = now(), archiviato_da = '{U}', motivo_archiviazione = 'prova' WHERE componente_id = '{K1}';`)
	if a := albero(); a != "F1@0" {
		t.Errorf("padre archiviato: albero %q", a)
	}
	if got := uno20[string](t, p, `SELECT string_agg(c.codice || ':' || s.esito, ' ') FROM v_step_prodotto s JOIN componente c USING (componente_id) WHERE s.thread_id = '{T1}'`); got != "F1:radice_senza_qualifica" {
		t.Errorf("v_step_prodotto con il padre archiviato: %s", got)
	}
	// il codice resta occupato dall'archiviato
	rifiuta(t, p, "ux_componente_thread_codice", `INSERT INTO componente (thread_id, codice, confermato_da) VALUES ('{T1}', 'p1', '{U}')`)
}

// ------------------------------------------------------------------ l'analizzatore corrente

func TestLAnalizzatoreCorrenteEUnaRigaSola(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esegui20(t, p, `INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES (1, repeat('a', 64))`)
	rifiuta(t, p, "analizzatore_corrente_pkey", `INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES (2, repeat('b', 64))`)
	rifiuta(t, p, "analizzatore_corrente_unico_check", `INSERT INTO analizzatore_corrente (unico, versione_analizzatore, hash_configurazione) VALUES (false, 2, repeat('b', 64))`)
}

// ------------------------------------------------------------------ 0019: orfani e prove di creazione

func TestLeTabelleDelloSpostamento(t *testing.T) {
	p := schema20(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	esegui20(t, p, `
		INSERT INTO job (job_id, tipo, worker_tipo, payload, stato, chiuso_il) OVERRIDING SYSTEM VALUE
		     VALUES (9001, 'copia_nas', 'server', '{}', 'fallito', now());
		INSERT INTO nas_orfano (thread_id, percorso, motivo, job_id) VALUES ('{T1}', 'ELENCO DISEGNI\P1\a.pdf', 'rimozione_fallita', 9001);
		INSERT INTO nas_creazione (job_id, percorso, sha256) VALUES (9001, 'ELENCO DISEGNI\P1\b.pdf', repeat('1', 64));`)
	// una sola riga aperta per file, a meno delle maiuscole (prova 56, la parte dell'indice)
	rifiuta(t, p, "ux_nas_orfano_aperto", `INSERT INTO nas_orfano (thread_id, percorso, motivo) VALUES ('{T1}', 'elenco disegni\p1\A.PDF', 'non_nostro')`)
	if n := uno20[int64](t, p, `WITH x AS (INSERT INTO nas_orfano (thread_id, percorso, motivo) VALUES ('{T1}', 'elenco disegni\p1\A.PDF', 'non_nostro')
	                              ON CONFLICT (thread_id, lower(percorso)) WHERE risolto_il IS NULL DO NOTHING RETURNING 1) SELECT count(*) FROM x`); n != 0 {
		t.Errorf("ON CONFLICT DO NOTHING ha inserito %d righe", n)
	}
	rifiuta(t, p, "nas_orfano_check", `UPDATE nas_orfano SET risolto_da = '{U}'`)
	rifiuta(t, p, "nas_orfano_percorso_check", `INSERT INTO nas_orfano (thread_id, percorso, motivo) VALUES ('{T1}', '  ', 'non_nostro')`)
	esegui20(t, p, `UPDATE nas_orfano SET risolto_il = now(), risolto_da = '{U}';
		INSERT INTO nas_orfano (thread_id, percorso, motivo) VALUES ('{T1}', 'elenco disegni\p1\A.PDF', 'non_nostro');`)
	// il job se ne va: l'orfano resta senza job, la prova di creazione se ne va con lui
	esegui20(t, p, `DELETE FROM job WHERE job_id = 9001`)
	if got := uno20[string](t, p, `SELECT count(*) || ' ' || count(job_id) || ' ' || (SELECT count(*) FROM nas_creazione) FROM nas_orfano`); got != "2 0 0" {
		t.Errorf("dopo la cancellazione del job (orfani, orfani con job, prove): %s", got)
	}
}
