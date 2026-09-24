//go:build integrazione

// L4 — i gesti del Fascicolo di A4 (B8.A4a) contro PostgreSQL vero: congelare, aprire e abbandonare
// una revisione, sostituire un documento, scegliere lo STEP strutturale, derogare, archiviare. Le
// prove dei soli vincoli del database stanno in platform/migrazioni (fascicolo0020_db_test.go); qui si
// prova che cosa fanno le funzioni e che cosa rispondono.
//
// Nomi delle prove di A4.12 dell'addendum: 1, 11, 12, 14, 15, 16, 17, 22, 24, 25, 39, 41, 42, 43, 44,
// 45, 46, 47, 62, 63, 64, 65, 67, 71, 73.

package fascicolo_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

const struttura3 = `{"struttura": {"versione": 3, "radici": ["#1"], "relazioni": [{"padre": "#1", "figlio": "#2", "qta": 2}],
	"avvisi": [], "limiti": {"troncato": false}, "scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0,
	"occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`

const struttura3Troncata = `{"struttura": {"versione": 3, "radici": ["#1"], "relazioni": [{"padre": "#1", "figlio": "#2", "qta": 2}],
	"avvisi": [], "limiti": {"troncato": true, "motivo": "troppi nodi"}, "scarti": {"prodotti_senza_definizione": 0,
	"occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`

const hashCfg = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

// banco e' una RFQ di prova con la fase FATTIBILITA aperta.
type banco struct {
	t      *testing.T
	p      *pgxpool.Pool
	ctx    context.Context
	utente uuid.UUID
	thread uuid.UUID
	n      int
}

func nuovoBanco(t *testing.T) *banco {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	b := &banco{t: t, p: p, ctx: context.Background()}
	b.riga(`INSERT INTO utente (sigla, nome, ufficio) VALUES ('A4', 'Prova A4', 'Tecnico') RETURNING utente_id`, &b.utente)
	var cliente uuid.UUID
	b.riga(`INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('PROVAA4', 'Prova A4') RETURNING cliente_id`, &cliente)
	b.riga(`INSERT INTO thread_offerta (cliente_id, canale, data_inizio, cartella_relativa) VALUES ($1, 'outlook', now(), 'PROVAA4\WIP\rfq')
		RETURNING thread_id`, &b.thread, cliente)
	b.esegui(`INSERT INTO fase_log (thread_id, nome_fase, inizio) VALUES ($1, 'FATTIBILITA', now() - interval '1 hour')`, b.thread)
	return b
}

func (b *banco) esegui(sql string, arg ...any) {
	b.t.Helper()
	if _, err := b.p.Exec(b.ctx, sql, arg...); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
}

func (b *banco) riga(sql string, dst any, arg ...any) {
	b.t.Helper()
	if err := b.p.QueryRow(b.ctx, sql, arg...).Scan(dst); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
}

func uno[T any](b *banco, sql string, arg ...any) T {
	b.t.Helper()
	var v T
	b.riga(sql, &v, arg...)
	return v
}

func (b *banco) componente(codice string, tipo db.TipoComponente) uuid.UUID {
	b.t.Helper()
	return uno[uuid.UUID](b, `INSERT INTO componente (thread_id, codice, tipo, confermato_da) VALUES ($1, $2, $3, $4) RETURNING componente_id`,
		b.thread, codice, tipo, b.utente)
}

func (b *banco) arco(padre, figlio uuid.UUID, qta int) {
	b.t.Helper()
	b.esegui(`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ($1, $2, $3, $4, 'manuale', $5)`,
		b.thread, padre, figlio, qta, b.utente)
}

// documento e' un documento confermato del componente: ogni chiamata un contenuto e un percorso nuovi.
func (b *banco) documento(comp uuid.UUID, tipo db.TipoDocumento, codice, ext string) uuid.UUID {
	b.t.Helper()
	b.n++
	sha := fmt.Sprintf("%064x", b.n)
	return uno[uuid.UUID](b, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING documento_id`,
		b.thread, uuid.NullUUID{UUID: comp, Valid: comp != uuid.Nil}, tipo, codice, fmt.Sprintf("file%d.%s", b.n, ext), ext, sha,
		fmt.Sprintf(`ELENCO DISEGNI\%s\%s_%d.%s`, codice, codice, b.n, ext), b.utente)
}

func (b *banco) sha(doc uuid.UUID) string {
	return uno[string](b, `SELECT sha256 FROM documento WHERE documento_id = $1`, doc)
}

// analisi scrive l'analizzatore corrente e i fatti dello STEP per quella chiave.
func (b *banco) analisi(doc uuid.UUID, fatti string) {
	b.t.Helper()
	b.esegui(`INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES (3, $1)
		ON CONFLICT (unico) DO UPDATE SET versione_analizzatore = 3, hash_configurazione = $1`, hashCfg)
	b.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 3, $2, $3)
		ON CONFLICT (sha256, versione_analizzatore, hash_configurazione) DO UPDATE SET fatti = EXCLUDED.fatti, calcolato_il = now()`,
		b.sha(doc), hashCfg, fatti)
}

// rfq e' una BOM congelabile: il finito P1 con lo STEP strutturale letto per intero e un 2D, lo
// sciolto F1 con il suo 2D, l'arco P1 → F1 ×2.
type rfq struct {
	p1, f1, step, d2p, d2f uuid.UUID
}

func (b *banco) rfqCongelabile() rfq {
	b.t.Helper()
	var r rfq
	r.p1 = b.componente("P1", db.TipoComponenteFinito)
	r.f1 = b.componente("F1", db.TipoComponenteSciolto)
	b.arco(r.p1, r.f1, 2)
	r.step = b.documento(r.p1, db.TipoDocumentoCad3d, "P1", "stp")
	r.d2p = b.documento(r.p1, db.TipoDocumentoDisegno2d, "P1", "pdf")
	r.d2f = b.documento(r.f1, db.TipoDocumentoDisegno2d, "F1", "pdf")
	b.analisi(r.step, struttura3)
	b.esegui(`UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, r.p1, r.step)
	return r
}

// tx esegue f in una transazione: COMMIT se va, ROLLBACK se no, come una rotta.
func (b *banco) tx(f func(q *db.Queries) error) error {
	b.t.Helper()
	tx, err := b.p.Begin(b.ctx)
	if err != nil {
		b.t.Fatal(err)
	}
	defer tx.Rollback(b.ctx)
	if err := f(db.New(tx)); err != nil {
		return err
	}
	return tx.Commit(b.ctx)
}

func (b *banco) congela(motivo string) (fascicolo.Congelamento, error) {
	var c fascicolo.Congelamento
	err := b.tx(func(q *db.Queries) (err error) {
		c, err = fascicolo.CongelaBom(b.ctx, q, b.thread, b.utente, motivo)
		return err
	})
	return c, err
}

func (b *banco) apri(scelta *db.ContestoBom, motivo string) (db.BomVersione, error) {
	var v db.BomVersione
	err := b.tx(func(q *db.Queries) (err error) {
		v, err = fascicolo.ApriRevisione(b.ctx, q, b.thread, b.utente, scelta, motivo)
		return err
	})
	return v, err
}

func (b *banco) abbandona() (string, error) {
	var msg string
	err := b.tx(func(q *db.Queries) (err error) {
		msg, err = fascicolo.AbbandonaBozza(b.ctx, q, b.thread)
		return err
	})
	return msg, err
}

func (b *banco) fase() string {
	return uno[string](b, `SELECT nome_fase::text FROM fase_log WHERE thread_id = $1 AND fine IS NULL`, b.thread)
}

// passaA chiude la fase aperta e ne apre un'altra: le fasi della vita commerciale, che A4 non scrive.
func (b *banco) passaA(fase string) {
	b.t.Helper()
	b.esegui(`UPDATE fase_log SET fine = now(), esito = 'OK' WHERE thread_id = $1 AND fine IS NULL`, b.thread)
	b.esegui(`INSERT INTO fase_log (thread_id, nome_fase, inizio) VALUES ($1, $2, now())`, b.thread, fase)
}

func deveRifiutare(t *testing.T, err error, frase string) {
	t.Helper()
	var r fascicolo.Rifiuto
	if !errors.As(err, &r) {
		t.Fatalf("atteso un rifiuto con «%s», ottenuto %v", frase, err)
	}
	if !strings.Contains(string(r), frase) {
		t.Fatalf("rifiuto %q, atteso che contenga %q", r, frase)
	}
}

func ok(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// impronta delle istantanee di una versione: tutto quello che le quattro tabelle dicono.
func (b *banco) impronta(versione uuid.UUID) string {
	return uno[string](b, `SELECT concat_ws(' # ',
		(SELECT string_agg(to_jsonb(x)::text, '|' ORDER BY componente_id) FROM bom_versione_componente x WHERE bom_versione_id = $1),
		(SELECT string_agg(to_jsonb(x)::text, '|' ORDER BY padre_id, figlio_id) FROM bom_versione_relazione x WHERE bom_versione_id = $1),
		(SELECT string_agg(to_jsonb(x)::text, '|' ORDER BY documento_id) FROM bom_versione_documento x WHERE bom_versione_id = $1),
		(SELECT string_agg(to_jsonb(x)::text, '|' ORDER BY componente_id, tipo) FROM bom_versione_deroga x WHERE bom_versione_id = $1),
		(SELECT to_jsonb(v)::text FROM bom_versione v WHERE bom_versione_id = $1))`, versione)
}

// ------------------------------------------------------------------ congelamento

// Prove 14 e 42: la V1 nasce al primo congelamento, fotografa la working (con lo STEP strutturale e il
// suo SHA), e la Scheda Costo nasce su di lei.
func TestLaFaseSchedaCostoRicordaLaSuaBaseline(t *testing.T) {
	b := nuovoBanco(t)
	r := b.rfqCongelabile()
	c, err := b.congela("prima baseline")
	ok(t, err)
	if c.Versione.Numero != 1 || c.Versione.Stato != db.StatoBomCongelata || c.Versione.Contesto != db.ContestoBomPreventivo || c.Fase != db.FaseSCHEDACOSTO {
		t.Fatalf("congelamento: V%d %s %s, fase %s", c.Versione.Numero, c.Versione.Stato, c.Versione.Contesto, c.Fase)
	}
	if got := uno[string](b, `SELECT nome_fase || ' ' || bom_versione_id FROM fase_log WHERE thread_id = $1 AND fine IS NULL`, b.thread); got != "SCHEDA_COSTO "+c.Versione.BomVersioneID.String() {
		t.Errorf("fase aperta: %s", got)
	}
	if got := uno[string](b, `SELECT esito || ' ' || note FROM fase_log WHERE thread_id = $1 AND nome_fase = 'FATTIBILITA'`, b.thread); got != "OK BOM V1 congelata" {
		t.Errorf("FATTIBILITA chiusa con: %s", got)
	}
	if got := uno[string](b, `SELECT count(*) || ' ' || (SELECT count(*) FROM bom_versione_relazione) || ' ' || (SELECT count(*) FROM bom_versione_documento)
		FROM bom_versione_componente`); got != "2 1 3" {
		t.Errorf("istantanee (componenti, archi, documenti): %s", got)
	}
	if got := uno[string](b, `SELECT step_strutturale_id || ' ' || step_sha256 || ' ' || (deroga_struttura_id IS NULL) FROM bom_versione_componente WHERE componente_id = $1`, r.p1); got != r.step.String()+" "+b.sha(r.step)+" true" {
		t.Errorf("lo STEP strutturale nella baseline: %s", got)
	}
	// una seconda volta non si congela: prima si apre una revisione
	_, err = b.congela("ancora")
	deveRifiutare(t, err, "la V1 è già congelata")
}

// Prova 24: una prova per condizione del gate, contro il database vero.
func TestIlCongelamentoSiRifiutaConProposteAperte(t *testing.T) {
	casi := []struct {
		nome  string
		prep  func(b *banco, r rfq)
		frase string
	}{
		{"un requisito bloccante", func(b *banco, r rfq) { b.esegui(`DELETE FROM documento WHERE documento_id = $1`, r.d2f) }, "requisiti bloccanti"},
		{"una proposta di componente", func(b *banco, r rfq) {
			b.esegui(`INSERT INTO conversazione (conversazione_id, canale, chiave_esterna, primo_messaggio_il) VALUES ('40000000-0000-0000-0000-00000000000a', 'outlook', 'c-a4', now());
				INSERT INTO messaggio (messaggio_id, canale, chiave_esterna, conversazione_id, direzione, data_evento) VALUES ('40000000-0000-0000-0000-00000000000b', 'outlook', '<m-a4>', '40000000-0000-0000-0000-00000000000a', 'entrata', now());
				INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, ricevuto_il) VALUES ('40000000-0000-0000-0000-00000000000c', '40000000-0000-0000-0000-00000000000b', 1, 'p1.stp', now())`)
			b.esegui(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, fonte, confidenza)
				VALUES ($1, '40000000-0000-0000-0000-00000000000c', repeat('a', 64), '#3', 'STAFFA', 'step', 80)`, b.thread)
		}, "1 proposte strutturali aperte (1 componenti"},
		{"una proposta di rimozione", func(b *banco, r rfq) {
			b.esegui(`INSERT INTO rimozione_proposta (thread_id, step_documento_id, padre_id, figlio_id, qta_working) VALUES ($1, $2, $3, $4, 2)`, b.thread, r.step, r.p1, r.f1)
		}, "1 rimozioni"},
		{"un documento in errore", func(b *banco, r rfq) {
			b.esegui(`UPDATE documento SET stato_nas = 'errore', errore_nas = 'NAS assente' WHERE documento_id = $1`, r.d2p)
		}, "documenti della BOM in errore"},
		{"uno STEP letto in parte", func(b *banco, r rfq) { b.analisi(r.step, struttura3Troncata) }, "serve una deroga strutturale"},
		{"uno STEP da scegliere", func(b *banco, r rfq) {
			b.esegui(`UPDATE componente SET step_strutturale_id = NULL WHERE componente_id = $1`, r.p1)
		}, "si sceglie lo STEP strutturale"},
		{"un ciclo", func(b *banco, r rfq) { b.arco(r.f1, r.p1, 1) }, "ciclo: F1 → P1 → F1"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			b := nuovoBanco(t)
			r := b.rfqCongelabile()
			c.prep(b, r)
			_, err := b.congela("prova")
			deveRifiutare(t, err, c.frase)
			if n := uno[int64](b, `SELECT count(*) FROM bom_versione`); n != 0 {
				t.Errorf("un congelamento rifiutato ha lasciato %d versioni", n)
			}
			if f := b.fase(); f != "FATTIBILITA" {
				t.Errorf("fase dopo il rifiuto: %s", f)
			}
		})
	}
}

// Prova 25: un guasto dopo le istantanee non lascia niente, e due congelamenti insieme ne fanno uno solo.
func TestCongelamentoEFaseSonoAtomici(t *testing.T) {
	b := nuovoBanco(t)
	b.rfqCongelabile()
	b.esegui(`CREATE FUNCTION pg_temp_guasto() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'guasto iniettato'; END $$`)
	b.esegui(`CREATE TRIGGER guasto BEFORE INSERT ON fase_log FOR EACH ROW EXECUTE FUNCTION pg_temp_guasto()`)
	_, err := b.congela("prova")
	if err == nil || !strings.Contains(err.Error(), "guasto iniettato") {
		t.Fatalf("atteso il guasto, ottenuto %v", err)
	}
	b.esegui(`DROP TRIGGER guasto ON fase_log; DROP FUNCTION pg_temp_guasto()`)
	if got := uno[string](b, `SELECT (SELECT count(*) FROM bom_versione) || ' ' || (SELECT count(*) FROM bom_versione_componente) || ' ' ||
		(SELECT nome_fase::text FROM fase_log WHERE thread_id = $1 AND fine IS NULL)`, b.thread); got != "0 0 FATTIBILITA" {
		t.Errorf("dopo il guasto (versioni, istantanee, fase): %s", got)
	}

	t.Run("due congelamenti insieme", func(t *testing.T) {
		esiti := make(chan error, 2)
		for i := 0; i < 2; i++ {
			go func() {
				_, err := b.congela("insieme")
				esiti <- err
			}()
		}
		var riusciti, rifiutati int
		for i := 0; i < 2; i++ {
			err := <-esiti
			var r fascicolo.Rifiuto
			switch {
			case err == nil:
				riusciti++
			case errors.As(err, &r):
				rifiutati++
			default:
				t.Errorf("errore che non e' un rifiuto: %v", err)
			}
		}
		if riusciti != 1 || rifiutati != 1 {
			t.Errorf("riusciti %d, rifiutati %d: attesi 1 e 1", riusciti, rifiutati)
		}
		if n := uno[int64](b, `SELECT count(*) FROM bom_versione`); n != 1 {
			t.Errorf("versioni: %d", n)
		}
	})
}

// ------------------------------------------------------------------ revisioni della BOM

// Prove 11 e 12: il co-design apre una versione nuova, e la vecchia resta identica.
func TestIlCoDesignApreUnaVersioneNuova(t *testing.T) {
	b := nuovoBanco(t)
	r := b.rfqCongelabile()
	v1, err := b.congela("prima baseline")
	ok(t, err)
	prima := b.impronta(v1.Versione.BomVersioneID)
	b.passaA("OFFERTA_INVIATA")

	v2, err := b.apri(nil, "il cliente cambia la staffa")
	ok(t, err)
	if v2.Numero != 2 || v2.Stato != db.StatoBomBozza || v2.Contesto != db.ContestoBomPreventivo || v2.FaseAllApertura != db.FaseOFFERTAINVIATA ||
		v2.VersionePrecedenteID.UUID != v1.Versione.BomVersioneID {
		t.Fatalf("V2: %+v", v2)
	}
	if f := b.fase(); f != "FATTIBILITA" {
		t.Errorf("una revisione preventivo porta in FATTIBILITA: %s", f)
	}
	// Luigi lavora: una qta, un componente nuovo, un 2D sostituito
	b.esegui(`UPDATE componente_relazione SET qta = 3 WHERE padre_id = $1`, r.p1)
	n1 := b.componente("N1", db.TipoComponenteSciolto)
	b.arco(r.p1, n1, 1)
	b.documento(n1, db.TipoDocumentoDisegno2d, "N1", "pdf")
	nuovo2d := b.documento(r.f1, db.TipoDocumentoDisegno2d, "F1", "pdf")
	ok(t, b.tx(func(q *db.Queries) error {
		_, err := fascicolo.Sostituisci(b.ctx, q, b.thread, r.d2f, nuovo2d, true)
		return err
	}))
	v2c, err := b.congela("revisione")
	ok(t, err)
	if v2c.Versione.Numero != 2 || v2c.Fase != db.FaseSCHEDACOSTO {
		t.Fatalf("V2 congelata: V%d fase %s", v2c.Versione.Numero, v2c.Fase)
	}
	if dopo := b.impronta(v1.Versione.BomVersioneID); dopo != prima {
		t.Errorf("la V1 e' cambiata:\nprima %s\ndopo  %s", prima, dopo)
	}
	if got := uno[string](b, `SELECT string_agg(numero || ':' || superata || ':' || corrente, ' ' ORDER BY numero) FROM v_bom_versioni WHERE thread_id = $1`, b.thread); got != "1:true:false 2:false:true" {
		t.Errorf("versioni: %s", got)
	}
	if got := uno[string](b, `SELECT qta::text FROM bom_versione_relazione WHERE bom_versione_id = $1 AND padre_id = $2 AND figlio_id = $3`, v1.Versione.BomVersioneID, r.p1, r.f1); got != "2" {
		t.Errorf("la V1 ricorda la qta di allora: %s", got)
	}
}

// Prova 44: prima dell'ordine una revisione torna a FATTIBILITA; abbandonata a differenza vuota, torna alla fase di apertura.
func TestUnaRevisionePrimaDellOrdineTornaAFattibilita(t *testing.T) {
	for _, fase := range []string{"SCHEDA_COSTO", "OFFERTE_FORN", "OFFERTA_INVIATA"} {
		t.Run(fase, func(t *testing.T) {
			b := nuovoBanco(t)
			b.rfqCongelabile()
			_, err := b.congela("prima baseline")
			ok(t, err)
			if fase != "SCHEDA_COSTO" {
				b.passaA(fase)
			}
			v, err := b.apri(nil, "cambia il prezzo")
			ok(t, err)
			if v.Contesto != db.ContestoBomPreventivo || b.fase() != "FATTIBILITA" {
				t.Fatalf("revisione %s, fase %s", v.Contesto, b.fase())
			}
			tecnica := db.ContestoBomTecnica
			if _, err := b.apri(&tecnica, "altra"); err == nil {
				t.Error("una seconda revisione aperta")
			}
			msg, err := b.abbandona()
			ok(t, err)
			if b.fase() != fase || !strings.Contains(msg, "V2 abbandonata") {
				t.Errorf("dopo l'abbandono: fase %s, %q", b.fase(), msg)
			}
		})
	}
}

// Prova 45: dopo l'ordine la revisione e' tecnica e la fase non si muove; la working e' libera durante
// la bozza e bloccata dopo.
func TestUnaRevisioneTecnicaDopoLOrdineNonCambiaFase(t *testing.T) {
	for _, fase := range []string{"ORDINE", "PRODUZIONE"} {
		t.Run(fase, func(t *testing.T) {
			b := nuovoBanco(t)
			r := b.rfqCongelabile()
			_, err := b.congela("prima baseline")
			ok(t, err)
			b.passaA(fase)
			righe := uno[int64](b, `SELECT count(*) FROM fase_log WHERE thread_id = $1`, b.thread)
			preventivo := db.ContestoBomPreventivo
			_, err = b.apri(&preventivo, "no")
			deveRifiutare(t, err, "la revisione è tecnica, non preventivo")
			v, err := b.apri(nil, "il cliente modifica la staffa dopo il primo lotto")
			ok(t, err)
			if v.Contesto != db.ContestoBomTecnica || b.fase() != fase {
				t.Fatalf("revisione %s, fase %s", v.Contesto, b.fase())
			}
			b.esegui(`UPDATE componente_relazione SET qta = 5 WHERE padre_id = $1`, r.p1) // libera durante la bozza
			c, err := b.congela("revisione tecnica")
			ok(t, err)
			if c.Fase != db.Fase(fase) || b.fase() != fase {
				t.Errorf("fase dopo il congelamento tecnico: %s / %s", c.Fase, b.fase())
			}
			if n := uno[int64](b, `SELECT count(*) FROM fase_log WHERE thread_id = $1`, b.thread); n != righe {
				t.Errorf("fase_log riscritto: %d righe, erano %d", n, righe)
			}
			if _, err := b.p.Exec(b.ctx, `UPDATE componente_relazione SET qta = 6 WHERE padre_id = $1`, r.p1); err == nil {
				t.Error("dopo il congelamento la working doveva essere bloccata")
			}
		})
	}
}

// Prove 15 e 46: una revisione tecnica congelata fa comparire la RFQ fra quelle da riesaminare, con il
// motivo che viene dalla fase di apertura; fase_log non si riscrive.
func TestUnaRevisioneTecnicaSegnalaIlRiallineamento(t *testing.T) {
	casi := []struct{ fase, tipo, motivo string }{
		{"ORDINE", "revisione_tecnica_dopo_ordine", "revisione tecnica dopo l'ordine: la Scheda Costo poggia su V1, l'ultima baseline e' V2"},
		{"DISTINTA_ERP", "revisione_tecnica_prima_ordine", "revisione tecnica prima dell'ordine: l'offerta accettata poggia su V1, l'ultima baseline e' V2"},
	}
	for _, c := range casi {
		t.Run(c.fase, func(t *testing.T) {
			b := nuovoBanco(t)
			b.rfqCongelabile()
			_, err := b.congela("prima baseline")
			ok(t, err)
			b.passaA(c.fase)
			tecnica := db.ContestoBomTecnica
			_, err = b.apri(&tecnica, "modifica")
			ok(t, err)
			if n := uno[int64](b, `SELECT count(*) FROM v_thread_da_riesaminare WHERE thread_id = $1`, b.thread); n != 0 {
				t.Errorf("con la bozza aperta la RFQ non e' ancora da riesaminare: %d righe", n)
			}
			_, err = b.congela("tecnica")
			ok(t, err)
			got := uno[string](b, `SELECT tipo_motivo || ' | ' || motivo FROM v_thread_da_riesaminare WHERE thread_id = $1`, b.thread)
			if got != c.tipo+" | "+c.motivo {
				t.Errorf("riesame: %s", got)
			}
		})
	}
}

// Prova 47.
func TestNessunaRevisioneSuUnaRfqChiusa(t *testing.T) {
	for _, fase := range []string{"PERSA", "RESPINTA", "SCADUTA"} {
		t.Run(fase, func(t *testing.T) {
			b := nuovoBanco(t)
			b.rfqCongelabile()
			_, err := b.congela("prima baseline")
			ok(t, err)
			b.passaA(fase)
			_, err = b.apri(nil, "tardi")
			deveRifiutare(t, err, "la RFQ è chiusa ("+fase+")")
		})
	}
}

// Prova 65: in ACCETTATA e DISTINTA_ERP il tipo di revisione lo sceglie una persona.
func TestInAccettataEInDistintaErpLaRevisioneVuoleUnaScelta(t *testing.T) {
	for _, fase := range []string{"ACCETTATA", "DISTINTA_ERP"} {
		t.Run(fase, func(t *testing.T) {
			b := nuovoBanco(t)
			b.rfqCongelabile()
			_, err := b.congela("prima baseline")
			ok(t, err)
			b.passaA(fase)
			_, err = b.apri(nil, "senza scelta")
			deveRifiutare(t, err, "lo sceglie chi la apre")
			preventivo := db.ContestoBomPreventivo
			v, err := b.apri(&preventivo, "cambia il prezzo")
			ok(t, err)
			if v.Contesto != db.ContestoBomPreventivo || b.fase() != "FATTIBILITA" {
				t.Fatalf("preventivo: %s, fase %s", v.Contesto, b.fase())
			}
			c, err := b.congela("nuova offerta")
			ok(t, err)
			if c.Fase != db.FaseSCHEDACOSTO {
				t.Errorf("il congelamento di una preventivo porta a SCHEDA_COSTO: %s", c.Fase)
			}
			b.passaA(fase)
			tecnica := db.ContestoBomTecnica
			v, err = b.apri(&tecnica, "il prezzo resta")
			ok(t, err)
			if v.Contesto != db.ContestoBomTecnica || b.fase() != fase {
				t.Errorf("tecnica: %s, fase %s", v.Contesto, b.fase())
			}
		})
	}
	// una V1 in ACCETTATA non nasce: senza baseline non c'e' niente da rivedere
	b := nuovoBanco(t)
	b.rfqCongelabile()
	b.passaA("ACCETTATA")
	_, err := b.congela("prima")
	deveRifiutare(t, err, "la prima baseline nasce in FATTIBILITA")
}

// Prova 67: l'abbandono torna alla riga di apertura, con il suo responsabile, anche passando per
// ATTESA_DISEGNI; si rifiuta se la fase si e' mossa altrove, o se la working non e' piu' la Vn.
func TestLAbbandonoTornaAllaRigaDiApertura(t *testing.T) {
	b := nuovoBanco(t)
	r := b.rfqCongelabile()
	_, err := b.congela("prima baseline")
	ok(t, err)
	b.passaA("OFFERTA_INVIATA")
	b.esegui(`UPDATE fase_log SET responsabile_id = $2 WHERE thread_id = $1 AND fine IS NULL`, b.thread, b.utente)
	_, err = b.apri(nil, "prova")
	ok(t, err)
	b.passaA("ATTESA_DISEGNI")

	// la working cambiata: si rifiuta, e dice che cosa e' cambiato
	b.esegui(`UPDATE componente_relazione SET qta = 9 WHERE padre_id = $1`, r.p1)
	_, err = b.abbandona()
	deveRifiutare(t, err, "la BOM working non è più la V1 (1 differenze: arco")
	b.esegui(`UPDATE componente_relazione SET qta = 2 WHERE padre_id = $1`, r.p1)

	_, err = b.abbandona()
	ok(t, err)
	if got := uno[string](b, `SELECT nome_fase || ' ' || coalesce(responsabile_id::text, '-') FROM fase_log WHERE thread_id = $1 AND fine IS NULL`, b.thread); got != "OFFERTA_INVIATA "+b.utente.String() {
		t.Errorf("fase dopo l'abbandono: %s", got)
	}
	if n := uno[int64](b, `SELECT count(*) FROM fase_log WHERE thread_id = $1 AND nome_fase = 'OFFERTA_INVIATA'`, b.thread); n != 2 {
		t.Errorf("la riga interrotta si riapre invece di nascerne una nuova: %d righe OFFERTA_INVIATA", n)
	}

	// la fase si e' mossa per altre vie: rifiuto
	_, err = b.apri(nil, "di nuovo")
	ok(t, err)
	b.passaA("SCHEDA_COSTO")
	_, err = b.abbandona()
	deveRifiutare(t, err, "è passata per SCHEDA_COSTO")
}

// ------------------------------------------------------------------ revisioni dei documenti

// Prova 1: la vecchia resta con sostituito_da, la nuova e' nel fascicolo, la storia ha la catena.
func TestUnaRevisioneNuovaSostituisceEConservaLaVecchia(t *testing.T) {
	b := nuovoBanco(t)
	r := b.rfqCongelabile()
	nuovo := b.documento(r.f1, db.TipoDocumentoDisegno2d, "F1", "pdf")
	var msg string
	ok(t, b.tx(func(q *db.Queries) (err error) {
		msg, err = fascicolo.Sostituisci(b.ctx, q, b.thread, r.d2f, nuovo, true)
		return err
	}))
	if !strings.Contains(msg, "sostituito da") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := uno[string](b, `SELECT sostituito_da::text FROM documento WHERE documento_id = $1`, r.d2f); got != nuovo.String() {
		t.Errorf("la vecchia non punta alla nuova: %s", got)
	}
	if got := uno[string](b, `SELECT documento_id::text FROM v_fascicolo WHERE componente_id = $1 AND tipo_documento = 'disegno_2d'`, r.f1); got != nuovo.String() {
		t.Errorf("v_fascicolo mostra %s", got)
	}
	if got := uno[string](b, `SELECT string_agg(passo || ':' || corrente, ' ' ORDER BY passo) FROM v_documento_storia WHERE catena_id = $1`, r.d2f); got != "1:false 2:true" {
		t.Errorf("storia: %s", got)
	}
	// e i rifiuti che la regola dice prima del database
	altro := b.documento(r.p1, db.TipoDocumentoDisegno2d, "P1", "pdf")
	err := b.tx(func(q *db.Queries) error {
		_, err := fascicolo.Sostituisci(b.ctx, q, b.thread, nuovo, altro, true)
		return err
	})
	deveRifiutare(t, err, "non sono dello stesso componente")
	err = b.tx(func(q *db.Queries) error {
		_, err := fascicolo.Sostituisci(b.ctx, q, b.thread, r.d2f, altro, true)
		return err
	})
	deveRifiutare(t, err, "non sono dello stesso componente")
	err = b.tx(func(q *db.Queries) error {
		_, err := fascicolo.Sostituisci(b.ctx, q, b.thread, r.d2p, r.step, true)
		return err
	})
	deveRifiutare(t, err, "si sostituisce con un disegno_2d")
	// annullare: la vecchia torna corrente
	ok(t, b.tx(func(q *db.Queries) error {
		_, err := fascicolo.AnnullaSostituzione(b.ctx, q, b.thread, r.d2f)
		return err
	}))
	if n := uno[int64](b, `SELECT count(*) FROM documento WHERE documento_id IN ($1, $2) AND sostituito_da IS NULL`, r.d2f, nuovo); n != 2 {
		t.Errorf("dopo l'annullamento i correnti sono %d", n)
	}
}

// Prove 39 e 41: lo STEP strutturale lo sceglie una persona e non segue da solo le sostituzioni; le
// rimozioni aperte del vecchio riferimento si chiudono.
func TestLoStepStrutturaleLoSceglieUnaPersona(t *testing.T) {
	b := nuovoBanco(t)
	p1 := b.componente("P1", db.TipoComponenteFinito)
	s1 := b.documento(p1, db.TipoDocumentoCad3d, "P1", "stp")
	if got := uno[string](b, `SELECT esito || ' ' || n_step_correnti FROM v_step_prodotto WHERE componente_id = $1`, p1); got != "da_scegliere 1" {
		t.Errorf("con un solo STEP corrente nessuno lo fissa: %s", got)
	}
	ok(t, b.tx(func(q *db.Queries) error {
		_, err := fascicolo.ScegliStepStrutturale(b.ctx, q, b.thread, p1, s1)
		return err
	}))
	f1 := b.componente("F1", db.TipoComponenteSciolto)
	b.arco(p1, f1, 1)
	b.esegui(`INSERT INTO rimozione_proposta (thread_id, step_documento_id, padre_id, figlio_id, qta_working) VALUES ($1, $2, $3, $4, 1)`, b.thread, s1, p1, f1)

	t.Run("rifiuti della scelta", func(t *testing.T) {
		d2 := b.documento(p1, db.TipoDocumentoDisegno2d, "P1", "pdf")
		err := b.tx(func(q *db.Queries) error {
			_, err := fascicolo.ScegliStepStrutturale(b.ctx, q, b.thread, p1, d2)
			return err
		})
		deveRifiutare(t, err, "non è un file STEP")
		sf := b.documento(f1, db.TipoDocumentoCad3d, "F1", "stp")
		err = b.tx(func(q *db.Queries) error {
			_, err := fascicolo.ScegliStepStrutturale(b.ctx, q, b.thread, f1, sf)
			return err
		})
		deveRifiutare(t, err, "non è un prodotto finito")
		err = b.tx(func(q *db.Queries) error {
			_, err := fascicolo.ScegliStepStrutturale(b.ctx, q, b.thread, p1, sf)
			return err
		})
		deveRifiutare(t, err, "non è assegnato a P1")
	})

	s2 := b.documento(p1, db.TipoDocumentoCad3d, "P1", "stp")
	var msg string
	ok(t, b.tx(func(q *db.Queries) (err error) {
		msg, err = fascicolo.Sostituisci(b.ctx, q, b.thread, s1, s2, false)
		return err
	}))
	if !strings.Contains(msg, "va scelto il nuovo riferimento") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := uno[string](b, `SELECT esito FROM v_step_prodotto WHERE componente_id = $1`, p1); got != "riferimento_superato" {
		t.Errorf("il riferimento sostituito: %s", got)
	}
	if got := uno[string](b, `SELECT stato || ' ' || coalesce(deciso_da::text, '-') || ' ' || nota FROM rimozione_proposta`); !strings.HasPrefix(got, "scartata - superata da ") {
		t.Errorf("la rimozione del vecchio riferimento: %s", got)
	}
	ok(t, b.tx(func(q *db.Queries) error {
		_, err := fascicolo.ScegliStepStrutturale(b.ctx, q, b.thread, p1, s2)
		return err
	}))
	if got := uno[string](b, `SELECT esito FROM v_step_prodotto WHERE componente_id = $1`, p1); got != "presente_non_analizzato" {
		t.Errorf("dopo la scelta del nuovo: %s", got)
	}
}

// ------------------------------------------------------------------ deroghe

// Prove 43 e 62: la deroga strutturale fa passare il gate con uno STEP letto in parte, e decade con
// ciascuno dei quattro casi di D36.
func TestLaDerogaStrutturaleValeSoloPerQuelloStepEQuellAnalisi(t *testing.T) {
	prepara := func(t *testing.T) (*banco, rfq) {
		b := nuovoBanco(t)
		r := b.rfqCongelabile()
		b.analisi(r.step, struttura3Troncata)
		_, err := b.congela("prova")
		deveRifiutare(t, err, "serve una deroga strutturale")
		ok(t, b.tx(func(q *db.Queries) error {
			_, err := fascicolo.ConcediDerogaStruttura(b.ctx, q, b.thread, r.p1, b.utente, "la macchina e' enorme, la distinta la controlliamo a mano")
			return err
		}))
		if got := uno[bool](b, `SELECT deroga_struttura_id IS NOT NULL FROM v_step_prodotto WHERE componente_id = $1`, r.p1); !got {
			t.Fatal("la deroga appena concessa non vale")
		}
		return b, r
	}
	gateRosso := func(t *testing.T, b *banco) {
		t.Helper()
		if n := uno[bool](b, `SELECT deroga_struttura_id IS NULL FROM v_step_prodotto WHERE esito <> 'radice_senza_qualifica'`); !n {
			t.Error("la deroga doveva decadere")
		}
		_, err := b.congela("prova")
		var r fascicolo.Rifiuto
		if !errors.As(err, &r) {
			t.Errorf("il gate doveva essere rosso, ottenuto %v", err)
		}
	}
	t.Run("passa con la deroga", func(t *testing.T) {
		b, r := prepara(t)
		c, err := b.congela("con deroga")
		ok(t, err)
		if got := uno[bool](b, `SELECT deroga_struttura_id IS NOT NULL FROM bom_versione_componente WHERE bom_versione_id = $1 AND componente_id = $2`,
			c.Versione.BomVersioneID, r.p1); !got {
			t.Error("la baseline non ricorda la deroga strutturale (prova 64)")
		}
		if _, err := b.p.Exec(b.ctx, `DELETE FROM deroga_struttura`); err == nil {
			t.Error("la deroga di una baseline si e' cancellata")
		}
	})
	t.Run("lo STEP strutturale sostituito", func(t *testing.T) {
		b, r := prepara(t)
		s2 := b.documento(r.p1, db.TipoDocumentoCad3d, "P1", "stp")
		ok(t, b.tx(func(q *db.Queries) error {
			_, err := fascicolo.Sostituisci(b.ctx, q, b.thread, r.step, s2, true)
			return err
		}))
		gateRosso(t, b)
	})
	t.Run("la stessa chiave rianalizzata", func(t *testing.T) {
		b, r := prepara(t)
		b.analisi(r.step, struttura3Troncata) // UpsertAnalisiFatti: stessa chiave, calcolato_il nuovo
		gateRosso(t, b)
	})
	t.Run("una versione nuova dell'analizzatore", func(t *testing.T) {
		b, r := prepara(t)
		b.esegui(`UPDATE analizzatore_corrente SET versione_analizzatore = 4`)
		b.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 4, $2, $3)`, b.sha(r.step), hashCfg, struttura3Troncata)
		gateRosso(t, b)
	})
	t.Run("la prima analisi di uno STEP derogato prima di essere analizzato", func(t *testing.T) {
		b := nuovoBanco(t)
		r := b.rfqCongelabile()
		b.esegui(`DELETE FROM analisi_fatti`)
		ok(t, b.tx(func(q *db.Queries) error {
			_, err := fascicolo.ConcediDerogaStruttura(b.ctx, q, b.thread, r.p1, b.utente, "non ancora analizzato")
			return err
		}))
		if got := uno[string](b, `SELECT (versione_analizzatore IS NULL) || ' ' || motivo_parziale FROM deroga_struttura`); got != "true nessuna analisi corrente dello STEP" {
			t.Errorf("la deroga di uno STEP non analizzato: %s", got)
		}
		b.analisi(r.step, struttura3Troncata)
		gateRosso(t, b)
	})
}

// Prova 63: la deroga del fabbisogno cad_3d non copre uno STEP letto in parte; copre l'assenza di uno STEP.
func TestUnaDerogaDelFabbisognoNonCopreUnoStepParziale(t *testing.T) {
	b := nuovoBanco(t)
	r := b.rfqCongelabile()
	b.esegui(`INSERT INTO deroga_fabbisogno (thread_id, componente_id, tipo, motivo, utente_id) VALUES ($1, $2, 'cad_3d', 'niente distinta', $3)`, b.thread, r.p1, b.utente)
	b.analisi(r.step, struttura3Troncata)
	_, err := b.congela("prova")
	deveRifiutare(t, err, "serve una deroga strutturale")
	// senza STEP scelto: da_scegliere non si deroga
	b.esegui(`UPDATE componente SET step_strutturale_id = NULL WHERE componente_id = $1`, r.p1)
	_, err = b.congela("prova")
	deveRifiutare(t, err, "si sceglie lo STEP strutturale")
	// senza STEP del tutto: la deroga del fabbisogno basta
	b.esegui(`DELETE FROM documento WHERE documento_id = $1`, r.step)
	if got := uno[string](b, `SELECT esito FROM v_step_prodotto WHERE componente_id = $1`, r.p1); got != "mancante" {
		t.Fatalf("esito: %s", got)
	}
	_, err = b.congela("senza distinta letta")
	ok(t, err)
}

// Prova 71: le deroghe del fabbisogno nella baseline, e la differenza che le vede.
func TestLaBaselineFotografaLeDerogheDelFabbisogno(t *testing.T) {
	b := nuovoBanco(t)
	r := b.rfqCongelabile()
	b.esegui(`INSERT INTO deroga_fabbisogno (thread_id, componente_id, tipo, motivo, utente_id) VALUES ($1, $2, 'sviluppo_dxf', 'lo facciamo noi', $3)`, b.thread, r.f1, b.utente)
	v1, err := b.congela("prima baseline")
	ok(t, err)
	if got := uno[string](b, `SELECT tipo || ' ' || motivo FROM bom_versione_deroga WHERE bom_versione_id = $1`, v1.Versione.BomVersioneID); got != "sviluppo_dxf lo facciamo noi" {
		t.Fatalf("deroga nella baseline: %s", got)
	}
	_, err = b.apri(nil, "revisione")
	ok(t, err)
	diff := func() string {
		var out []string
		ok(t, b.tx(func(q *db.Queries) error {
			d, err := fascicolo.DifferenzeWorking(b.ctx, q, b.thread, v1.Versione.BomVersioneID)
			for _, x := range d {
				out = append(out, x.Oggetto+" "+x.Tipo+" "+strings.Join(x.Campi, ","))
			}
			return err
		}))
		return strings.Join(out, "; ")
	}
	b.esegui(`UPDATE deroga_fabbisogno SET motivo = 'lo manda il cliente'`)
	b.esegui(`INSERT INTO deroga_fabbisogno (thread_id, componente_id, tipo, motivo, utente_id) VALUES ($1, $2, 'cad_3d', 'niente 3D', $3)`, b.thread, r.f1, b.utente)
	if got := diff(); got != "deroga aggiunto ; deroga cambiato motivo" { // in ordine di chiave: cad_3d prima di sviluppo_dxf
		t.Errorf("differenze: %q", got)
	}
	_, err = b.abbandona()
	deveRifiutare(t, err, "non è più la V1")
	b.esegui(`DELETE FROM deroga_fabbisogno`)
	if got := diff(); got != "deroga tolto " {
		t.Errorf("tolte: %q", got)
	}
	if got := uno[string](b, `SELECT tipo || ' ' || motivo FROM bom_versione_deroga WHERE bom_versione_id = $1`, v1.Versione.BomVersioneID); got != "sviluppo_dxf lo facciamo noi" {
		t.Errorf("la baseline vecchia e' cambiata: %s", got)
	}
}

// ------------------------------------------------------------------ archiviazione

// Prove 16 e 17: archiviare toglie dalla working e non tocca le baseline; documenti e storia restano;
// un componente con storia non si cancella.
func TestTogliereUnComponenteNonToccaLeBaseline(t *testing.T) {
	b := nuovoBanco(t)
	r := b.rfqCongelabile()
	v1, err := b.congela("prima baseline")
	ok(t, err)
	prima := b.impronta(v1.Versione.BomVersioneID)
	_, err = b.apri(nil, "togliere F1")
	ok(t, err)

	err = b.tx(func(q *db.Queries) error { _, err := fascicolo.RimuoviComponente(b.ctx, q, b.thread, r.f1); return err })
	deveRifiutare(t, err, "F1 non si cancella")
	var msg string
	ok(t, b.tx(func(q *db.Queries) (err error) {
		msg, err = fascicolo.ArchiviaComponente(b.ctx, q, b.thread, r.f1, b.utente, "il cliente lo ha tolto")
		return err
	}))
	if !strings.Contains(msg, "tolti 1 archi") {
		t.Errorf("messaggio: %q", msg)
	}
	if got := uno[string](b, `SELECT (SELECT count(*) FROM componente_relazione) || ' ' || (SELECT count(*) FROM documento WHERE componente_id = $1) || ' ' ||
		(SELECT count(*) FROM v_fascicolo WHERE componente_id = $1) || ' ' || (SELECT count(*) FROM v_componente_albero WHERE componente_id = $1)`, r.f1); got != "0 1 0 0" {
		t.Errorf("dopo l'archiviazione (archi, documenti, righe fascicolo, albero): %s", got)
	}
	if b.impronta(v1.Versione.BomVersioneID) != prima {
		t.Error("l'archiviazione ha toccato la baseline")
	}
	ok(t, b.tx(func(q *db.Queries) error {
		_, err := fascicolo.RipristinaComponente(b.ctx, q, b.thread, r.f1)
		return err
	}))
	if got := uno[int64](b, `SELECT count(*) FROM v_componente_albero WHERE componente_id = $1`, r.f1); got != 1 {
		t.Errorf("ripristinato: %d righe nell'albero (una radice da sistemare)", got)
	}
	// un componente senza storia si cancella
	n1 := b.componente("N1", db.TipoComponenteSciolto)
	ok(t, b.tx(func(q *db.Queries) error { _, err := fascicolo.RimuoviComponente(b.ctx, q, b.thread, n1); return err }))
}

// Prova 22: i cambi di BOM non toccano lo staging.
func TestICambiDiBomNonToccanoLoStaging(t *testing.T) {
	b := nuovoBanco(t)
	r := b.rfqCongelabile()
	contenuto := filepath.Join(t.TempDir(), "_contenuti", "00", "contenuto.pdf")
	ok(t, os.MkdirAll(filepath.Dir(contenuto), 0o755))
	ok(t, os.WriteFile(contenuto, []byte("byte del disegno"), 0o644))
	b.esegui(`INSERT INTO conversazione (conversazione_id, canale, chiave_esterna, primo_messaggio_il) VALUES ('40000000-0000-0000-0000-0000000000a1', 'outlook', 'c-st', now());
		INSERT INTO messaggio (messaggio_id, canale, chiave_esterna, conversazione_id, direzione, data_evento) VALUES ('40000000-0000-0000-0000-0000000000a2', 'outlook', '<m-st>', '40000000-0000-0000-0000-0000000000a1', 'entrata', now())`)
	b.esegui(`INSERT INTO allegato (messaggio_id, indice, nome_file, sha256, path_staging, ricevuto_il) VALUES ('40000000-0000-0000-0000-0000000000a2', 1, 'd.pdf', $1, $2, now())`,
		b.sha(r.d2f), contenuto)
	foto := func() string {
		info, err := os.Stat(contenuto)
		ok(t, err)
		return uno[string](b, `SELECT string_agg(path_staging, '|') FROM allegato`) + fmt.Sprintf(" %d %s", info.Size(), info.ModTime())
	}
	prima := foto()
	b.esegui(`UPDATE componente_relazione SET qta = 4`)
	n1 := b.componente("N1", db.TipoComponenteSciolto)
	b.esegui(`UPDATE componente_relazione SET padre_id = $1 WHERE figlio_id = $2`, n1, r.f1) // un altro padre
	b.esegui(`UPDATE componente_relazione SET padre_id = $1 WHERE figlio_id = $2`, r.p1, r.f1)
	ok(t, b.tx(func(q *db.Queries) error {
		_, err := fascicolo.ArchiviaComponente(b.ctx, q, b.thread, n1, b.utente, "prova")
		return err
	}))
	b.esegui(`INSERT INTO deroga_fabbisogno (thread_id, componente_id, tipo, motivo, utente_id) VALUES ($1, $2, 'cad_3d', 'x', $3)`, b.thread, r.f1, b.utente)
	_, err := b.congela("prima baseline")
	ok(t, err)
	if dopo := foto(); dopo != prima {
		t.Errorf("lo staging e' cambiato:\nprima %s\ndopo  %s", prima, dopo)
	}
}

// propostaTecnica e' un allegato arrivato e non ancora confermato, con la sua proposta aperta di 2D.
func (b *banco) propostaTecnica(chiave string) uuid.UUID {
	b.t.Helper()
	var conv, msg, allegato uuid.UUID
	b.riga(`INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', $1, now()) RETURNING conversazione_id`, &conv, "C-"+chiave)
	b.riga(`INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, thread_id)
		VALUES ('outlook', $1, $2, 'entrata', now(), $3) RETURNING messaggio_id`, &msg, "<"+chiave+">", conv, b.thread)
	b.riga(`INSERT INTO allegato (messaggio_id, indice, nome_file, ricevuto_il) VALUES ($1, 1, $2, now()) RETURNING allegato_id`, &allegato, msg, chiave+".pdf")
	return uno[uuid.UUID](b, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte)
		VALUES ($1, $2, 'disegno_2d', 60, 'estensione') RETURNING proposta_id`, allegato, b.thread)
}

// Prova 73: dopo il congelamento un file tecnico confermato, o una proposta tecnica nata dopo, fanno
// comparire la RFQ con il motivo e i conteggi. Il segnale sparisce per la proposta scartata, per tutto
// con una revisione aperta o con il congelamento successivo; non compare per un capitolato, ne' per una
// proposta nata prima del congelamento.
func TestUnaNuovaEvidenzaDopoIlCongelamentoSiVede(t *testing.T) {
	b := nuovoBanco(t)
	b.rfqCongelabile()
	b.documento(uuid.Nil, db.TipoDocumentoCapitolato, "CAP", "pdf") // un capitolato prima: non conta mai
	b.propostaTecnica("prima")                                      // una proposta nata prima del congelamento
	_, err := b.congela("prima baseline")
	ok(t, err)
	riesame := func() string {
		return uno[string](b, `SELECT coalesce(string_agg(motivo, ' | '), '') FROM v_thread_da_riesaminare WHERE thread_id = $1`, b.thread)
	}
	if got := riesame(); got != "" {
		t.Fatalf("subito dopo il congelamento: %s", got)
	}
	b.documento(uuid.Nil, db.TipoDocumentoCapitolato, "CAP2", "pdf")
	if got := riesame(); got != "" {
		t.Errorf("un capitolato fa comparire la RFQ: %s", got)
	}
	p := b.propostaTecnica("dopo")
	if got := riesame(); got != "evidenze tecniche nuove dopo V1: 1 file, 0 proposte" {
		t.Errorf("con una proposta tecnica nata dopo: %s", got)
	}
	b.esegui(`UPDATE documento_proposta SET stato = 'scartata' WHERE proposta_id = $1`, p)
	if got := riesame(); got != "" {
		t.Errorf("la proposta scartata non conta piu': %s", got)
	}
	b.documento(uuid.Nil, db.TipoDocumentoDisegno2d, "X9", "pdf") // un file tecnico confermato, per forza senza componente
	if got := riesame(); got != "evidenze tecniche nuove dopo V1: 1 file, 0 proposte" {
		t.Errorf("con un file tecnico confermato: %s", got)
	}
	_, err = b.apri(nil, "guardiamo il file nuovo")
	ok(t, err)
	if got := riesame(); got != "" {
		t.Errorf("con la revisione aperta: %s", got)
	}
	_, err = b.congela("revisione")
	ok(t, err)
	if got := riesame(); got != "" {
		t.Errorf("dopo il congelamento successivo: %s", got)
	}
}
