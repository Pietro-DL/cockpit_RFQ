//go:build integrazione

// L4 — il servizio unico delle associazioni registrate (direttiva 35 §2-§3; piano delle dipendenze §3.6, blocco C-1c)
// sul database di prova, con i trigger della 0023 accesi: ogni AR nasce dentro una conferma formato 3 vera della stessa
// transazione (provvisorio, base [T] D-AR01 A), scritta come la scrivera' R5 (bozza formato 3 con le PB, testata,
// componenti e archi certificati, AR, chiusura della bozza). Le prove:
//   - la nascita con la copia e la lettura del codice conservata (INC-G4), l'allegato senza proposta (Prova 2, INC-G24);
//   - l'unicita' della AR attiva (provvisorio, base [T] D-ASS-04 A) e la sostituzione sempre esplicita;
//   - la chiusura, la riassociazione e la sostituzione con la storia; lo svincolo di R5 e svincolo_gia_chiuso;
//   - l'idempotenza per chiave (Ripetuto), la conferma di nuovo senza cambiamenti (Invariata), la copia senza AR;
//   - la copia sempre uguale alla fonte dopo ogni atto (Prova 11, lato servizio), anche per la proposta che nasce dopo
//     la AR dall'ingest o dal worker (INC-G5);
//   - la concorrenza: due R5 sullo stesso file, due gesti FOR KEY SHARE sulla stessa AR, R5 contro un gesto legacy;
//   - il legame della sorgente (provvisorio, base [T] D-AR02 A, NON cablato: INC-S1) e l'allineamento del codice.
// Solo dati sintetici (ACME, example.com).

package associazioni_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// banco: la RFQ T con il prodotto P (ACM-1000) e i figli S1 (ACM-1100, sottoassieme) e S2 (ACM-1200, sciolto) nella
// working, un secondo prodotto P2 (ACM-5000) senza figli, un componente archiviato Z, una RFQ T2 con il suo prodotto Q.
// Gli allegati di T: a1 PDF con la proposta (codice letto in minuscolo, diverso dal codice del componente), a2 PDF senza
// proposta, a3 PDF con la proposta senza codice letto, a4 PDF con la copia legacy su S2 (pre-assegnazione, senza AR),
// a5 STEP con la proposta, a6 PDF senza proposta.
type banco struct {
	t                                *testing.T
	ctx                              context.Context
	p                                *pgxpool.Pool
	utente, cliente, thread, thread2 uuid.UUID
	P, S1, S2, P2, Z, Q              uuid.UUID
	a1, a2, a3, a4, a5, a6, aT2      uuid.UUID
	prop1, prop3, prop4, prop5       uuid.UUID
}

func nuovoBanco(t *testing.T) *banco {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	b := &banco{t: t, ctx: context.Background(), p: p, utente: uuid.New(), cliente: uuid.New(), thread: uuid.New(), thread2: uuid.New(),
		P: uuid.New(), S1: uuid.New(), S2: uuid.New(), P2: uuid.New(), Z: uuid.New(), Q: uuid.New(),
		a1: uuid.New(), a2: uuid.New(), a3: uuid.New(), a4: uuid.New(), a5: uuid.New(), a6: uuid.New(), aT2: uuid.New(),
		prop1: uuid.New(), prop3: uuid.New(), prop4: uuid.New(), prop5: uuid.New()}
	m, m2 := uuid.New(), uuid.New()
	for _, x := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO utente (utente_id, sigla, nome, ufficio, ruolo) VALUES ($1, 'DAR', 'Prova associazioni', 'UT', 'admin')`, []any{b.utente}},
		{`INSERT INTO cliente (cliente_id, cartella_nas, ragione_sociale) VALUES ($1, 'ACMEAR', 'ACME S.p.A.')`, []any{b.cliente}},
		{`INSERT INTO thread_offerta (thread_id, cliente_id, canale, data_inizio, oggetto) VALUES ($1, $3, 'outlook', now(), 'RFQ ACME-1000'),
			($2, $3, 'outlook', now(), 'RFQ ACME-7000')`, []any{b.thread, b.thread2, b.cliente}},
		{`INSERT INTO conversazione (conversazione_id, canale, chiave_esterna, thread_id, primo_messaggio_il) VALUES
			($1, 'outlook', 'conv-ar-1@example.com', $3, now()), ($2, 'outlook', 'conv-ar-2@example.com', $4, now())`,
			[]any{uuid.New(), uuid.New(), b.thread, b.thread2}},
		{`INSERT INTO messaggio (messaggio_id, canale, chiave_esterna, conversazione_id, thread_id, direzione, data_evento, oggetto)
			SELECT $1, 'outlook', 'msg-ar-1@example.com', conversazione_id, $2, 'entrata', now(), 'RFQ ACME' FROM conversazione WHERE thread_id = $2`,
			[]any{m, b.thread}},
		{`INSERT INTO messaggio (messaggio_id, canale, chiave_esterna, conversazione_id, thread_id, direzione, data_evento, oggetto)
			SELECT $1, 'outlook', 'msg-ar-2@example.com', conversazione_id, $2, 'entrata', now(), 'RFQ ACME' FROM conversazione WHERE thread_id = $2`,
			[]any{m2, b.thread2}},
		{`INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il) VALUES
			($1, $6, 1, 'ACM-1100_b.pdf', 'pdf', repeat('1', 64), now()), ($2, $6, 2, 'ACM-1200.pdf', 'pdf', repeat('2', 64), now()),
			($3, $6, 3, 'disegno.pdf', 'pdf', repeat('3', 64), now()), ($4, $6, 4, 'ACM-1200_a.pdf', 'pdf', repeat('4', 64), now()),
			($5, $6, 5, 'ACM-1000.stp', 'stp', repeat('5', 64), now()), ($7, $6, 6, 'ACM-1100_c.pdf', 'pdf', repeat('6', 64), now())`,
			[]any{b.a1, b.a2, b.a3, b.a4, b.a5, m, b.a6}},
		{`INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il) VALUES
			($1, $2, 1, 'ACM-7000.pdf', 'pdf', repeat('7', 64), now())`, []any{b.aT2, m2}},
		{`INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES
			($1, $7, 'ACM-1000', 'finito', 'manuale', $8), ($2, $7, 'ACM-1100', 'sottoassieme', 'manuale', $8),
			($3, $7, 'ACM-1200', 'sciolto', 'manuale', $8), ($4, $7, 'ACM-5000', 'finito', 'manuale', $8),
			($5, $7, 'ACM-1900', 'sciolto', 'manuale', $8), ($6, $9, 'ACM-7000', 'finito', 'manuale', $8)`,
			[]any{b.P, b.S1, b.S2, b.P2, b.Z, b.Q, b.thread, b.utente, b.thread2}},
		{`UPDATE componente SET archiviato_il = now(), archiviato_da = $2, motivo_archiviazione = 'prova AR' WHERE componente_id = $1`, []any{b.Z, b.utente}},
		{`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES
			($1, $2, $3, 1, 'manuale', $5), ($1, $2, $4, 2, 'manuale', $5)`, []any{b.thread, b.P, b.S1, b.S2, b.utente}},
		{`INSERT INTO documento_proposta (proposta_id, allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES
			($1, $2, $9, 'disegno_2d', 'acm-1100 rev b', 80, 'nome_file'), ($3, $4, $9, 'disegno_2d', NULL, 20, 'nome_file'),
			($5, $6, $9, 'disegno_2d', 'ACM-1200', 80, 'nome_file'), ($7, $8, $9, 'cad_3d', 'ACM-1000', 80, 'nome_file')`,
			[]any{b.prop1, b.a1, b.prop3, b.a3, b.prop4, b.a4, b.prop5, b.a5, b.thread}},
	} {
		if _, err := p.Exec(b.ctx, x.sql, x.args...); err != nil {
			t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	// a4: la copia legacy (pre-assegnazione del vecchio «Assegna», senza AR: INC-S2), scritta come dato di prima del
	// trigger della copia derivata (acceso dalla variante 20261019d)
	testutil.CopiaLegacy(t, p, `UPDATE documento_proposta SET componente_id = $2, codice = 'ACM-1200' WHERE proposta_id = $1`, b.prop4, b.S2)
	return b
}

// pb e sv: una preassociazione {allegato_id, chiave_bozza} e uno svincolo scollega_documento {allegato_id, componente_id}.
type pb struct {
	allegato uuid.UUID
	chiave   string
}

type sv struct{ allegato, componente uuid.UUID }

// nodo: un nodo registrato dello snapshot formato 3, con l'identita' del componente (codice, emittente presunto).
type nodo struct {
	chiave     string
	componente uuid.UUID
	codice     string
	tipo       string
}

func (b *banco) nodiDi(prodotto uuid.UUID) ([]nodo, [][3]any) {
	if prodotto == b.P2 {
		return []nodo{{"k-p2", b.P2, "ACM-5000", "finito"}}, nil
	}
	return []nodo{{"k-p", b.P, "ACM-1000", "finito"}, {"k-s1", b.S1, "ACM-1100", "sottoassieme"}, {"k-s2", b.S2, "ACM-1200", "sciolto"}},
		[][3]any{{"k-p", "k-s1", 1}, {"k-p", "k-s2", 2}}
}

// bozzaR5: una bozza formato 3 del prodotto alla versione 1, con le PB e gli svincoli dati, salvata e confermata
// (COMMIT) prima della transazione di R5, come fa R2.
type bozzaR5 struct {
	id, prodotto uuid.UUID
	snapshot     string
	impronta     string
	mappa        string
	nodi         []nodo
	archi        [][3]any
	precedente   uuid.NullUUID
	numero       int
}

func (b *banco) bozza(prodotto uuid.UUID, pbs []pb, svs []sv) bozzaR5 {
	b.t.Helper()
	nodi, archi := b.nodiDi(prodotto)
	emittente := "cliente:" + b.cliente.String()
	var nn, aa, dd, pp []any
	mappa := map[string]string{}
	for _, n := range nodi {
		nn = append(nn, map[string]any{"chiave_bozza": n.chiave, "componente_id": n.componente.String(), "op": "riusa", "tipo": n.tipo,
			"origine": "registrato", "identita": map[string]any{"codice": n.codice, "emittente": emittente, "emittente_presunto": true, "modo": "registrato"}})
		mappa[n.chiave] = n.componente.String()
	}
	for _, a := range archi {
		aa = append(aa, map[string]any{"padre": a[0], "figlio": a[1], "qta": a[2], "origine": "conferma"})
	}
	for i, s := range svs {
		dd = append(dd, map[string]any{"id": fmt.Sprintf("sv-%d", i+1), "tipo": "scollega_documento", "allegato_id": s.allegato.String(),
			"componente_id": s.componente.String()})
	}
	for _, x := range pbs {
		pp = append(pp, map[string]any{"allegato_id": x.allegato.String(), "chiave_bozza": x.chiave, "provenienza": "manuale"})
	}
	vuoto := func(x []any) []any {
		if x == nil {
			return []any{}
		}
		return x
	}
	snap, err := json.Marshal(map[string]any{"formato": 3, "prodotto_id": prodotto.String(), "nodi": nn, "archi": vuoto(aa),
		"decisioni": vuoto(dd), "documenti_preassociati": vuoto(pp)})
	if err != nil {
		b.t.Fatal(err)
	}
	m, _ := json.Marshal(mappa)
	h := sha256.Sum256(snap)
	bz := bozzaR5{prodotto: prodotto, snapshot: string(snap), impronta: hex.EncodeToString(h[:]), mappa: string(m), nodi: nodi, archi: archi}
	var prec uuid.UUID
	err = b.p.QueryRow(b.ctx, `SELECT conferma_id, numero FROM conferma_strutturale WHERE prodotto_id = $1 ORDER BY numero DESC LIMIT 1`, prodotto).
		Scan(&prec, &bz.numero)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		b.t.Fatal(err)
	default:
		bz.precedente = uuid.NullUUID{UUID: prec, Valid: true}
	}
	tx, err := b.p.Begin(b.ctx)
	if err != nil {
		b.t.Fatal(err)
	}
	defer tx.Rollback(b.ctx) //nolint:errcheck
	if err := tx.QueryRow(b.ctx, `INSERT INTO distinta_bozza (thread_id, prodotto_id, formato, base_conferma_id, base_numero, base_contatore, firma_base,
			creata_da, aggiornata_da) VALUES ($1, $2, 3, $3, $4, 0, repeat('1', 64), $5, $5) RETURNING bozza_id`,
		b.thread, prodotto, bz.precedente, bz.numero, b.utente).Scan(&bz.id); err != nil {
		b.t.Fatalf("bozza: %v", err)
	}
	if _, err := tx.Exec(b.ctx, `INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_conferma_id, base_numero, base_contatore,
			firma_base, salvata_da, chiave_salvataggio, impronta_richiesta)
		VALUES ($1, 1, $2::jsonb, $3, $4, $5, 0, repeat('1', 64), $6, gen_random_uuid(), repeat('4', 64))`,
		bz.id, bz.snapshot, bz.impronta, bz.precedente, bz.numero, b.utente); err != nil {
		b.t.Fatalf("versione 1: %v", err)
	}
	if err := tx.Commit(b.ctx); err != nil {
		b.t.Fatal(err)
	}
	return bz
}

// testata: nella transazione di R5, la conferma formato 3 che chiude la bozza, con i componenti e gli archi certificati
// (l'ordine di R5: la testata dopo la working, i certificati dopo la testata). Restituisce l'atto di R5.
func (b *banco) testata(tx pgx.Tx, bz bozzaR5) associazioni.Atto {
	b.t.Helper()
	conferma, chiave := uuid.New(), uuid.New()
	if _, err := tx.Exec(b.ctx, `INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2,
			firma_struttura, firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati, bozza_id, bozza_versione, origine,
			snapshot, impronta_snapshot, delta, mappa, decisioni, criterio_verifica, chiave_idempotenza, confermato_da, importazioni_citate, verifica)
		SELECT $1, $2, $3, $4, $5, 3, false, repeat('3', 64), repeat('2', 64), $5, 0, repeat('1', 64),
			COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = $2), 0), COALESCE((SELECT dati FROM working_contatore WHERE thread_id = $2), 0),
			$6, 1, '{"rotta": "prova-associazioni"}', $7::jsonb, $8, '{"formato": 3}', $9::jsonb, '[]', 'nessuno', $10, $11, '{}'::uuid[],
			'{"regola": "prima-istanza-1"}'`,
		conferma, b.thread, bz.prodotto, bz.numero+1, bz.precedente, bz.id, bz.snapshot, bz.impronta, bz.mappa, chiave, b.utente); err != nil {
		b.t.Fatalf("testata: %v", err)
	}
	emittente := "cliente:" + b.cliente.String()
	for _, n := range bz.nodi {
		if _, err := tx.Exec(b.ctx, `INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev,
				descrizione, tipo, emittente, emittente_presunto, namespace)
			SELECT $1, c.thread_id, c.componente_id, c.componente_id = $3, $4, c.codice, c.rev, c.descrizione, c.tipo, $5, true, NULL
			  FROM componente c WHERE c.componente_id = $2`, conferma, n.componente, bz.prodotto, n.chiave, emittente); err != nil {
			b.t.Fatalf("componente certificato %s: %v", n.codice, err)
		}
	}
	chiavi := map[string]uuid.UUID{}
	for _, n := range bz.nodi {
		chiavi[n.chiave] = n.componente
	}
	for _, a := range bz.archi {
		if _, err := tx.Exec(b.ctx, `INSERT INTO conferma_strutturale_arco (conferma_id, padre_id, figlio_id, qta, origine) VALUES ($1, $2, $3, $4, 'conferma')`,
			conferma, chiavi[a[0].(string)], chiavi[a[1].(string)], a[2]); err != nil {
			b.t.Fatalf("arco certificato: %v", err)
		}
	}
	return associazioni.Atto{Thread: b.thread, Autore: b.utente, Chiave: chiave, Conferma: uuid.NullUUID{UUID: conferma, Valid: true}}
}

func (b *banco) chiudiBozza(tx pgx.Tx, bz bozzaR5) {
	b.t.Helper()
	if _, err := tx.Exec(b.ctx, `UPDATE distinta_bozza SET stato = 'confermata', chiusa_da = $2 WHERE bozza_id = $1`, bz.id, b.utente); err != nil {
		b.t.Fatalf("chiusura della bozza: %v", err)
	}
}

// r5: la transazione di R5 sul prodotto, con la bozza delle PB e degli svincoli dati: BloccaThread, testata, le
// scritture del servizio (f), la chiusura della bozza, COMMIT. Un errore di f annulla tutto, abbandona la bozza e torna
// com'e'.
func (b *banco) r5(prodotto uuid.UUID, pbs []pb, svs []sv, f func(q *db.Queries, a associazioni.Atto) error) (associazioni.Atto, error) {
	b.t.Helper()
	bz := b.bozza(prodotto, pbs, svs)
	tx, err := b.p.Begin(b.ctx)
	if err != nil {
		b.t.Fatal(err)
	}
	defer tx.Rollback(b.ctx) //nolint:errcheck
	q := db.New(tx)
	if _, err := q.BloccaThread(b.ctx, b.thread); err != nil {
		b.t.Fatal(err)
	}
	a := b.testata(tx, bz)
	if err := f(q, a); err != nil {
		// la conferma non c'e' stata: la bozza resta aperta e la si abbandona, come farebbe l'ingegnere
		_ = tx.Rollback(b.ctx)
		if _, e := b.p.Exec(b.ctx, `UPDATE distinta_bozza SET stato = 'abbandonata', chiusa_da = $2, motivo_chiusura = 'prova' WHERE bozza_id = $1`,
			bz.id, b.utente); e != nil {
			b.t.Fatalf("abbandono della bozza: %v", e)
		}
		return a, err
	}
	b.chiudiBozza(tx, bz)
	return a, tx.Commit(b.ctx)
}

// gesto: la transazione di un gesto che non e' R5 (corsia file, pagine legacy): la RFQ FOR KEY SHARE prima di tutto
// (precondizione del servizio, INC-G25), poi f, poi COMMIT.
func (b *banco) gesto(f func(q *db.Queries) error) error {
	b.t.Helper()
	tx, err := b.p.Begin(b.ctx)
	if err != nil {
		b.t.Fatal(err)
	}
	defer tx.Rollback(b.ctx) //nolint:errcheck
	q := db.New(tx)
	if _, err := q.BloccaThreadCondiviso(b.ctx, b.thread); err != nil {
		b.t.Fatal(err)
	}
	if err := f(q); err != nil {
		return err
	}
	return tx.Commit(b.ctx)
}

func (b *banco) atto() associazioni.Atto {
	return associazioni.Atto{Thread: b.thread, Autore: b.utente, Chiave: uuid.New()}
}

func ok(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func rifiutoCon(t *testing.T, err error, codice string) *associazioni.Rifiuto {
	t.Helper()
	var r *associazioni.Rifiuto
	if !errors.As(err, &r) || r.Codice != codice {
		t.Fatalf("atteso il rifiuto %s, arrivato %v", codice, err)
	}
	return r
}

func (b *banco) uno(sql string, args ...any) string {
	b.t.Helper()
	var s *string
	if err := b.p.QueryRow(b.ctx, sql, args...).Scan(&s); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
	if s == nil {
		return "<null>"
	}
	return *s
}

// storia: le righe dell'allegato come «componente:stato:motivo_chiusura», dalla prima.
func (b *banco) storia(allegato uuid.UUID) string {
	b.t.Helper()
	return b.uno(`SELECT string_agg(c.codice || ':' || r.stato || ':' || COALESCE(r.motivo_chiusura, '-'), ' | ' ORDER BY r.creata_il, (r.stato = 'attiva'))
		FROM file_rfq_associazione r JOIN componente c ON c.componente_id = r.componente_id WHERE r.allegato_id = $1`, allegato)
}

// giornale: gli eventi dell'allegato come «azione:origine:prima>dopo:con riga», dal primo.
func (b *banco) giornale(allegato uuid.UUID) string {
	b.t.Helper()
	return b.uno(`SELECT string_agg(e.azione || ':' || e.origine || ':' || COALESCE(cp.codice, '-') || '>' || COALESCE(cd.codice, '-') || ':' ||
			CASE WHEN e.associazione_id IS NULL THEN 'senza_riga' ELSE 'riga' END, ' | ' ORDER BY e.evento_id)
		FROM associazione_evento e LEFT JOIN componente cp ON cp.componente_id = e.componente_prima LEFT JOIN componente cd ON cd.componente_id = e.componente_dopo
		WHERE e.allegato_id = $1`, allegato)
}

// copia: la copia della proposta come «componente:codice:codice_letto».
func (b *banco) copia(proposta uuid.UUID) string {
	b.t.Helper()
	return b.uno(`SELECT COALESCE(c.codice, '-') || ':' || COALESCE(p.codice, '<null>') || ':' ||
			CASE WHEN p.dettagli ? 'codice_letto' THEN COALESCE(p.dettagli ->> 'codice_letto', '<null>') ELSE '-' END
		FROM documento_proposta p LEFT JOIN componente c ON c.componente_id = p.componente_id WHERE p.proposta_id = $1`, proposta)
}

// copieDivergenti (Prova 11, lato servizio): le proposte aperte della RFQ la cui copia non e' la AR attiva (componente o
// codice), o che hanno una copia senza AR dopo che l'allegato ne ha avute (copia lasciata da una chiusura). La copia
// legacy di un allegato che non ha mai avuto AR e' una pre-assegnazione (INC-S2), non una divergenza; un allegato senza
// proposta non e' una divergenza (INC-G24).
func (b *banco) copieDivergenti() int {
	b.t.Helper()
	var n int
	if err := b.p.QueryRow(b.ctx, `SELECT count(*) FROM documento_proposta p
		  LEFT JOIN file_rfq_associazione r ON r.allegato_id = p.allegato_id AND r.stato = 'attiva'
		  LEFT JOIN componente c ON c.componente_id = r.componente_id
		 WHERE p.stato = 'aperta' AND p.thread_id = $1
		   AND ((r.associazione_id IS NOT NULL AND (p.componente_id IS DISTINCT FROM r.componente_id OR p.codice IS DISTINCT FROM c.codice))
		     OR (r.associazione_id IS NULL AND p.componente_id IS NOT NULL
		         AND EXISTS (SELECT 1 FROM file_rfq_associazione h WHERE h.allegato_id = p.allegato_id)))`, b.thread).Scan(&n); err != nil {
		b.t.Fatal(err)
	}
	return n
}

func (b *banco) nessunaDivergenza(dove string) {
	b.t.Helper()
	if n := b.copieDivergenti(); n != 0 {
		b.t.Errorf("%s: %d copie diverse dalla fonte (Prova 11)", dove, n)
	}
}

// La nascita: tre PB applicate da R5 diventano tre AR, ognuna con il suo evento; la copia segue la AR e la lettura del
// codice resta in dettagli.codice_letto (anche vuota); l'allegato senza proposta ha la AR e nessuna copia (Prova 2).
func TestV1C1cLaNascitaConLaCopia(t *testing.T) {
	b := nuovoBanco(t)
	var e1, e2, e3 associazioni.Esito
	a, err := b.r5(b.P, []pb{{b.a1, "k-s1"}, {b.a2, "k-s2"}, {b.a3, "k-s1"}}, nil, func(q *db.Queries, a associazioni.Atto) (err error) {
		if e1, err = associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a1, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale}); err != nil {
			return err
		}
		if e2, err = associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a2, Componente: b.S2, ChiaveBozza: "k-s2", Provenienza: associazioni.ProvenienzaManuale}); err != nil {
			return err
		}
		e3, err = associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a3, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale})
		return err
	})
	ok(t, err)
	if e1.Ripetuto || e1.Invariata || e1.EventoID == 0 || !e1.Copia.Valid || e1.Copia.UUID != b.prop1 || e2.Copia.Valid || e3.Copia.UUID != b.prop3 {
		t.Fatalf("esiti: %+v | %+v | %+v", e1, e2, e3)
	}
	x := e1.Associazione
	if !x.Attiva() || x.Origine != associazioni.OrigineConfermaBOM || x.Conferma != a.Conferma || x.ChiaveBozza != "k-s1" ||
		x.ChiaveIdempotenza != a.Chiave || x.CreataDa != b.utente || x.Sostituisce.Valid {
		t.Errorf("la riga nata: %+v", x)
	}
	if got := b.copia(b.prop1); got != "ACM-1100:ACM-1100:acm-1100 rev b" {
		t.Errorf("la copia di a1: %s", got)
	}
	if got := b.copia(b.prop3); got != "ACM-1100:ACM-1100:<null>" {
		t.Errorf("la copia di a3 (lettura vuota): %s", got)
	}
	if got := b.giornale(b.a1); got != "preassociazione:conferma_bom:->ACM-1100:riga" {
		t.Errorf("il giornale di a1: %s", got)
	}
	if got := b.uno(`SELECT (proposta_id = $2 AND conferma_id = $3 AND autore_id = $4 AND chiave_idempotenza = $5)::text FROM associazione_evento WHERE allegato_id = $1`,
		b.a1, b.prop1, a.Conferma.UUID, b.utente, a.Chiave); got != "true" {
		t.Errorf("l'evento di a1 non porta proposta, conferma, autore e chiave")
	}
	if got := b.giornale(b.a2); got != "preassociazione:conferma_bom:->ACM-1200:riga" {
		t.Errorf("il giornale di a2 (senza proposta): %s", got)
	}
	tx, err := b.p.Begin(b.ctx)
	ok(t, err)
	defer tx.Rollback(b.ctx) //nolint:errcheck
	q := db.New(tx)
	att, err := associazioni.Attiva(b.ctx, q, b.thread, b.a2)
	ok(t, err)
	if att == nil || att.Componente != b.S2 {
		t.Errorf("la AR attiva di a2: %+v", att)
	}
	tutte, err := associazioni.AttiveDellaRfq(b.ctx, q, b.thread)
	ok(t, err)
	if len(tutte) != 3 {
		t.Errorf("le AR attive della RFQ: %d", len(tutte))
	}
	if att, err := associazioni.Attiva(b.ctx, q, b.thread, b.a4); err != nil || att != nil {
		t.Errorf("a4 ha solo la copia legacy, nessuna AR: %+v %v", att, err)
	}
	b.nessunaDivergenza("dopo la nascita")
}

// L'unicita' della AR attiva (provvisorio, base [T] D-ASS-04 A) e la sostituzione esplicita: un'altra R5 non associa lo
// stesso file a un altro componente; lo stesso atto non lo associa due volte; la copia legacy su un altro componente
// ferma la nascita (punto 5 di trg_fra_nasce, qui con il rifiuto leggibile); il componente archiviato o di un'altra RFQ
// non si associa. Niente resta scritto.
func TestV1C1cLUnicitaDellaARAttiva(t *testing.T) {
	b := nuovoBanco(t)
	_, err := b.r5(b.P, []pb{{b.a1, "k-s1"}}, nil, func(q *db.Queries, a associazioni.Atto) error {
		_, err := associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a1, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale})
		return err
	})
	ok(t, err)
	// un'altra R5 (sul prodotto P2) con la PB dello stesso file
	_, err = b.r5(b.P2, []pb{{b.a1, "k-p2"}}, nil, func(q *db.Queries, a associazioni.Atto) error {
		_, err := associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a1, Componente: b.P2, ChiaveBozza: "k-p2", Provenienza: associazioni.ProvenienzaManuale})
		return err
	})
	if r := rifiutoCon(t, err, associazioni.CodiceGiaAssociataAltrove); r.Altrove.UUID != b.S1 {
		t.Errorf("il componente che tiene il file: %v", r.Altrove)
	}
	// lo stesso atto, lo stesso file su due componenti
	_, err = b.r5(b.P, []pb{{b.a2, "k-s1"}}, nil, func(q *db.Queries, a associazioni.Atto) error {
		if _, err := associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a2, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale}); err != nil {
			return err
		}
		_, err := associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a2, Componente: b.S2, ChiaveBozza: "k-s2", Provenienza: associazioni.ProvenienzaManuale})
		return err
	})
	rifiutoCon(t, err, associazioni.CodiceGiaAssociataAltrove)
	// la copia legacy di a4 su S2, senza AR
	_, err = b.r5(b.P, []pb{{b.a4, "k-s1"}}, nil, func(q *db.Queries, a associazioni.Atto) error {
		_, err := associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a4, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale})
		return err
	})
	if r := rifiutoCon(t, err, associazioni.CodiceGiaAssociataAltrove); r.Altrove.UUID != b.S2 {
		t.Errorf("la copia legacy: %v", r.Altrove)
	}
	// Sostituisci senza una AR attiva: si registra
	_, err = b.r5(b.P, []pb{{b.a3, "k-s1"}}, nil, func(q *db.Queries, a associazioni.Atto) error {
		_, err := associazioni.Sostituisci(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a3, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale})
		return err
	})
	rifiutoCon(t, err, associazioni.CodiceNessunaAssociazioneAttiva)
	// componente archiviato, componente di un'altra RFQ
	for comp, codice := range map[uuid.UUID]string{b.Z: associazioni.CodiceComponenteArchiviato, b.Q: associazioni.CodiceComponenteNonDellaRfq} {
		err = b.gesto(func(q *db.Queries) error {
			a := b.atto()
			a.Conferma = uuid.NullUUID{UUID: uuid.New(), Valid: true}
			_, err := associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a3, Componente: comp, ChiaveBozza: "k-x", Provenienza: associazioni.ProvenienzaManuale})
			return err
		})
		rifiutoCon(t, err, codice)
	}
	if got := b.uno(`SELECT count(*)::text FROM file_rfq_associazione`); got != "1" {
		t.Errorf("le righe dopo i rifiuti: %s", got)
	}
	if got := b.uno(`SELECT count(*)::text FROM associazione_evento WHERE associazione_id IS NOT NULL`); got != "1" {
		t.Errorf("gli eventi dopo i rifiuti: %s", got)
	}
	b.nessunaDivergenza("dopo i rifiuti")
}

// La chiusura, la riassociazione e la sostituzione con la storia: le righe chiuse restano con chi, quando e il motivo;
// riassociare e' una riga nuova; la sostituzione chiude e fa nascere nella stessa transazione con un evento solo; lo
// svincolo di R5 chiude con la sua conferma; uno svincolo gia' chiuso dalla corsia file e' soddisfatto senza scrivere.
// La copia segue la fonte a ogni passo e il codice letto torna quando la copia si toglie.
func TestV1C1cChiusuraRiassociazioneEStoria(t *testing.T) {
	b := nuovoBanco(t)
	_, err := b.r5(b.P, []pb{{b.a1, "k-s1"}}, nil, func(q *db.Queries, a associazioni.Atto) error {
		_, err := associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a1, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale})
		return err
	})
	ok(t, err)
	// «Scollega e scarta» della pagina nuova (corsia file): la chiusura scollega_file
	var ch associazioni.EsitoChiusura
	ok(t, b.gesto(func(q *db.Queries) (err error) {
		ch, err = associazioni.Scollega(b.ctx, q, b.atto(), associazioni.Scollegamento{Allegato: b.a1, Componente: b.S1, Motivo: associazioni.MotivoScollegaFile})
		return err
	}))
	if ch.Esito != associazioni.EsitoChiusa || ch.Associazione == nil || ch.Associazione.Attiva() || ch.Associazione.ChiusaDa.UUID != b.utente ||
		ch.Associazione.ChiusaIl == nil || ch.Associazione.MotivoChiusura != associazioni.MotivoScollegaFile || ch.Copia.UUID != b.prop1 {
		t.Fatalf("la chiusura: %+v", ch)
	}
	if got := b.copia(b.prop1); got != "-:acm-1100 rev b:-" {
		t.Errorf("la copia tolta, il codice letto rimesso: %s", got)
	}
	b.nessunaDivergenza("dopo lo scollegamento")
	// la riassociazione: una riga nuova, la chiusa resta
	var e associazioni.Esito
	_, err = b.r5(b.P, []pb{{b.a1, "k-s1"}}, nil, func(q *db.Queries, a associazioni.Atto) (err error) {
		e, err = associazioni.Riassocia(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a1, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale})
		return err
	})
	ok(t, err)
	if e.Precedente == nil || e.Precedente.ID != ch.Associazione.ID || e.Associazione.ID == ch.Associazione.ID || !e.Associazione.Attiva() {
		t.Fatalf("la riassociazione: %+v", e)
	}
	if got := b.storia(b.a1); got != "ACM-1100:chiusa:scollega_file | ACM-1100:attiva:-" {
		t.Errorf("la storia dopo la riassociazione: %s", got)
	}
	// Riassocia di un allegato senza storia
	err = b.gesto(func(q *db.Queries) error {
		a := b.atto()
		a.Conferma = uuid.NullUUID{UUID: uuid.New(), Valid: true}
		_, err := associazioni.Riassocia(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a3, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale})
		return err
	})
	rifiutoCon(t, err, associazioni.CodiceRiassociazioneSenzaStoria)
	// la sostituzione esplicita da S1 a S2 (la bozza porta la PB su k-s2)
	var s associazioni.Esito
	a3, err := b.r5(b.P, []pb{{b.a1, "k-s2"}}, nil, func(q *db.Queries, a associazioni.Atto) (err error) {
		s, err = associazioni.Sostituisci(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a1, Componente: b.S2, ChiaveBozza: "k-s2", Provenienza: associazioni.ProvenienzaManuale})
		return err
	})
	ok(t, err)
	if s.Chiusa == nil || s.Chiusa.ID != e.Associazione.ID || s.Chiusa.MotivoChiusura != associazioni.MotivoSostituita ||
		s.Chiusa.ChiusaConferma != a3.Conferma || !s.Associazione.Sostituisce.Valid || s.Associazione.Sostituisce.UUID != e.Associazione.ID {
		t.Fatalf("la sostituzione: %+v", s)
	}
	if got := b.copia(b.prop1); got != "ACM-1200:ACM-1200:acm-1100 rev b" {
		t.Errorf("la copia dopo la sostituzione (il codice letto resta quello della prima copia): %s", got)
	}
	b.nessunaDivergenza("dopo la sostituzione")
	// lo svincolo di R5 (decisione scollega_documento della bozza)
	var sv1 associazioni.EsitoChiusura
	a4, err := b.r5(b.P, nil, []sv{{b.a1, b.S2}}, func(q *db.Queries, a associazioni.Atto) (err error) {
		sv1, err = associazioni.Scollega(b.ctx, q, a, associazioni.Scollegamento{Allegato: b.a1, Componente: b.S2, Motivo: associazioni.MotivoSvincoloConferma})
		return err
	})
	ok(t, err)
	if sv1.Esito != associazioni.EsitoChiusa || sv1.Associazione.ChiusaConferma != a4.Conferma || sv1.Associazione.MotivoChiusura != associazioni.MotivoSvincoloConferma {
		t.Fatalf("lo svincolo: %+v", sv1)
	}
	if got := b.copia(b.prop1); got != "-:acm-1100 rev b:-" {
		t.Errorf("la copia dopo lo svincolo: %s", got)
	}
	// la stessa decisione in una bozza successiva: la AR e' gia' chiusa (svincolo_gia_chiuso), nessuna scrittura
	var sv2 associazioni.EsitoChiusura
	_, err = b.r5(b.P, nil, []sv{{b.a1, b.S2}}, func(q *db.Queries, a associazioni.Atto) (err error) {
		sv2, err = associazioni.Scollega(b.ctx, q, a, associazioni.Scollegamento{Allegato: b.a1, Componente: b.S2, Motivo: associazioni.MotivoSvincoloConferma})
		return err
	})
	ok(t, err)
	if sv2.Esito != associazioni.EsitoSvincoloGiaChiuso || sv2.EventoID != 0 || sv2.Associazione == nil || sv2.Associazione.ID != s.Associazione.ID {
		t.Fatalf("svincolo gia' chiuso: %+v", sv2)
	}
	if got := b.storia(b.a1); got != "ACM-1100:chiusa:scollega_file | ACM-1100:chiusa:sostituita | ACM-1200:chiusa:svincolo_conferma" {
		t.Errorf("la storia completa: %s", got)
	}
	if got := b.giornale(b.a1); got != "preassociazione:conferma_bom:->ACM-1100:riga | sgancia:ponte:ACM-1100>-:riga | "+
		"preassociazione:conferma_bom:->ACM-1100:riga | sostituisci:conferma_bom:ACM-1100>ACM-1200:riga | sgancia:conferma_bom:ACM-1200>-:riga" {
		t.Errorf("il giornale completo: %s", got)
	}
	tx, err := b.p.Begin(b.ctx)
	ok(t, err)
	defer tx.Rollback(b.ctx) //nolint:errcheck
	st, err := associazioni.StoriaDellAllegato(b.ctx, db.New(tx), b.thread, b.a1)
	ok(t, err)
	if len(st) != 3 || st[0].ID != ch.Associazione.ID || st[2].ID != s.Associazione.ID {
		t.Errorf("StoriaDellAllegato: %d righe", len(st))
	}
	// «Riattiva» non riapre: la riga chiusa non torna attiva
	if _, err := tx.Exec(b.ctx, `UPDATE file_rfq_associazione SET stato = 'attiva', chiusa_il = NULL, chiusa_da = NULL, motivo_chiusura = NULL,
		chiusa_chiave = NULL, chiusa_tx = NULL WHERE associazione_id = $1`, ch.Associazione.ID); codiceSQL(err) != "P4X10" {
		t.Errorf("una AR chiusa si e' riaperta: %v", err)
	}
	b.nessunaDivergenza("alla fine")
}

// L'idempotenza per chiave: lo stesso atto non fa nascere ne' chiude due volte (Ripetuto), nella stessa transazione e
// dopo, anche quando la riga e' stata chiusa nel frattempo; confermare di nuovo senza cambiamenti non tocca la riga
// (Invariata, la conferma d'origine resta); la copia senza AR si toglie una volta.
func TestV1C1cLIdempotenzaPerChiave(t *testing.T) {
	b := nuovoBanco(t)
	var e1, e2 associazioni.Esito
	a1, err := b.r5(b.P, []pb{{b.a1, "k-s1"}, {b.a3, "k-s1"}}, nil, func(q *db.Queries, a associazioni.Atto) (err error) {
		r := associazioni.Registrazione{Allegato: b.a1, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale}
		if e1, err = associazioni.Registra(b.ctx, q, a, r); err != nil {
			return err
		}
		if e2, err = associazioni.Registra(b.ctx, q, a, r); err != nil {
			return err
		}
		_, err = associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a3, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale})
		return err
	})
	ok(t, err)
	if !e2.Ripetuto || e2.Associazione.ID != e1.Associazione.ID || e2.EventoID != 0 {
		t.Fatalf("il reinvio nella stessa transazione: %+v", e2)
	}
	// confermare di nuovo senza cambiamenti: la riga resta quella della prima conferma
	var e3 associazioni.Esito
	_, err = b.r5(b.P, []pb{{b.a3, "k-s1"}}, nil, func(q *db.Queries, a associazioni.Atto) (err error) {
		e3, err = associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a3, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale})
		return err
	})
	ok(t, err)
	if !e3.Invariata || e3.Associazione.Conferma != a1.Conferma || e3.EventoID != 0 {
		t.Fatalf("la conferma senza cambiamenti: %+v", e3)
	}
	// la chiusura e il suo reinvio (stessa transazione e dopo)
	chiude := b.atto()
	var c1, c2, c3 associazioni.EsitoChiusura
	ok(t, b.gesto(func(q *db.Queries) (err error) {
		s := associazioni.Scollegamento{Allegato: b.a1, Motivo: associazioni.MotivoScollegaFile}
		if c1, err = associazioni.Scollega(b.ctx, q, chiude, s); err != nil {
			return err
		}
		c2, err = associazioni.Scollega(b.ctx, q, chiude, s)
		return err
	}))
	ok(t, b.gesto(func(q *db.Queries) (err error) {
		c3, err = associazioni.Scollega(b.ctx, q, chiude, associazioni.Scollegamento{Allegato: b.a1, Motivo: associazioni.MotivoScollegaFile})
		return err
	}))
	if c1.Ripetuto || !c2.Ripetuto || !c3.Ripetuto || c2.Associazione.ID != c1.Associazione.ID || c3.EventoID != 0 {
		t.Fatalf("i reinvii della chiusura: %+v | %+v | %+v", c1, c2, c3)
	}
	// il reinvio della nascita dopo la chiusura: la coppia non rinasce
	var e4 associazioni.Esito
	ok(t, b.gesto(func(q *db.Queries) (err error) {
		e4, err = associazioni.Registra(b.ctx, q, a1, associazioni.Registrazione{Allegato: b.a1, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale})
		return err
	}))
	if !e4.Ripetuto || e4.Associazione.Attiva() || e4.Associazione.ID != e1.Associazione.ID {
		t.Fatalf("il reinvio dopo la chiusura: %+v", e4)
	}
	// la copia senza AR (a4, pre-assegnazione legacy): «Documentazione generale» la toglie una volta
	gen := b.atto()
	var g1, g2, g3 associazioni.EsitoChiusura
	ok(t, b.gesto(func(q *db.Queries) (err error) {
		g1, err = associazioni.Scollega(b.ctx, q, gen, associazioni.Scollegamento{Allegato: b.a4, Motivo: associazioni.MotivoFileGenerale})
		return err
	}))
	ok(t, b.gesto(func(q *db.Queries) (err error) {
		if g2, err = associazioni.Scollega(b.ctx, q, gen, associazioni.Scollegamento{Allegato: b.a4, Motivo: associazioni.MotivoFileGenerale}); err != nil {
			return err
		}
		g3, err = associazioni.Scollega(b.ctx, q, b.atto(), associazioni.Scollegamento{Allegato: b.a4, Motivo: associazioni.MotivoFileGenerale})
		return err
	}))
	if g1.Esito != associazioni.EsitoCopiaTolta || g1.Copia.UUID != b.prop4 || !g2.Ripetuto || g2.Esito != associazioni.EsitoCopiaTolta ||
		g3.Esito != associazioni.EsitoNessunLegame {
		t.Fatalf("la copia senza AR: %+v | %+v | %+v", g1, g2, g3)
	}
	if got := b.giornale(b.a4); got != "sgancia:legacy:ACM-1200>-:senza_riga" {
		t.Errorf("il giornale di a4: %s", got)
	}
	if got := b.copia(b.prop4); got != "-:ACM-1200:-" {
		t.Errorf("la copia legacy tolta (nessuna lettura conservata: il codice resta): %s", got)
	}
	if got := b.uno(`SELECT count(*)::text FROM file_rfq_associazione`); got != "2" {
		t.Errorf("le righe: %s", got)
	}
	if got := b.uno(`SELECT count(*)::text FROM associazione_evento`); got != "4" {
		t.Errorf("gli eventi (due nascite, una chiusura, uno sgancio della copia): %s", got)
	}
	b.nessunaDivergenza("dopo i reinvii")
}

// aspettaInAttesa: il gesto concorrente e' fermo su un lucchetto (pg_stat_activity), entro 10 s.
func (b *banco) aspettaInAttesa() {
	b.t.Helper()
	for i := 0; i < 500; i++ {
		var n int
		if err := b.p.QueryRow(b.ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&n); err != nil {
			b.t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	b.t.Fatal("il gesto concorrente non si e' fermato sul lucchetto")
}

func codiceSQL(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

// La concorrenza (il servizio non prende lucchetti: li prende chi chiama, RFQ prima della proposta):
//   - due R5 (P e P2) sullo stesso file: la seconda aspetta la RFQ, poi trova la AR della prima (gia_associata_altrove);
//   - due gesti FOR KEY SHARE sulla stessa AR (non si serializzano fra loro): il secondo si ferma sulla riga, poi la trova
//     chiusa (nessuna_associazione_attiva), mai una seconda chiusura;
//   - R5 contro un gesto legacy: il gesto aspetta la RFQ e poi vede la AR nuova (associazione_diversa).
//
// Nessuno stallo (40P01), mai due AR attive dello stesso file.
func TestV1C1cLaConcorrenza(t *testing.T) {
	b := nuovoBanco(t)
	// (1) due R5
	bz1, bz2 := b.bozza(b.P, []pb{{b.a1, "k-s1"}}, nil), b.bozza(b.P2, []pb{{b.a1, "k-p2"}}, nil)
	tx1, err := b.p.Begin(b.ctx)
	ok(t, err)
	defer tx1.Rollback(b.ctx) //nolint:errcheck
	q1 := db.New(tx1)
	_, err = q1.BloccaThread(b.ctx, b.thread)
	ok(t, err)
	at1 := b.testata(tx1, bz1)
	_, err = associazioni.Registra(b.ctx, q1, at1, associazioni.Registrazione{Allegato: b.a1, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale})
	ok(t, err)
	fine := make(chan error, 1)
	go func() {
		tx2, err := b.p.Begin(b.ctx)
		if err != nil {
			fine <- err
			return
		}
		defer tx2.Rollback(b.ctx) //nolint:errcheck
		q2 := db.New(tx2)
		if _, err := q2.BloccaThread(b.ctx, b.thread); err != nil {
			fine <- err
			return
		}
		at2 := b.testata(tx2, bz2)
		_, err = associazioni.Registra(b.ctx, q2, at2, associazioni.Registrazione{Allegato: b.a1, Componente: b.P2, ChiaveBozza: "k-p2", Provenienza: associazioni.ProvenienzaManuale})
		fine <- err
	}()
	b.aspettaInAttesa()
	b.chiudiBozza(tx1, bz1)
	ok(t, tx1.Commit(b.ctx))
	err = <-fine
	if codiceSQL(err) == "40P01" {
		t.Fatal("stallo fra due R5")
	}
	rifiutoCon(t, err, associazioni.CodiceGiaAssociataAltrove)
	if got := b.storia(b.a1); got != "ACM-1100:attiva:-" {
		t.Errorf("dopo le due R5: %s", got)
	}

	// (2) due gesti FOR KEY SHARE sulla stessa AR (a1 → S1, senza aspettare la proposta: la riga della AR fa da fila)
	txA, err := b.p.Begin(b.ctx)
	ok(t, err)
	defer txA.Rollback(b.ctx) //nolint:errcheck
	qA := db.New(txA)
	_, err = qA.BloccaThreadCondiviso(b.ctx, b.thread)
	ok(t, err)
	_, err = associazioni.Scollega(b.ctx, qA, b.atto(), associazioni.Scollegamento{Allegato: b.a1, Motivo: associazioni.MotivoScollegaFile})
	ok(t, err)
	go func() {
		txB, err := b.p.Begin(b.ctx)
		if err != nil {
			fine <- err
			return
		}
		defer txB.Rollback(b.ctx) //nolint:errcheck
		qB := db.New(txB)
		if _, err := qB.BloccaThreadCondiviso(b.ctx, b.thread); err != nil {
			fine <- err
			return
		}
		_, err = associazioni.Scollega(b.ctx, qB, b.atto(), associazioni.Scollegamento{Allegato: b.a1, Motivo: associazioni.MotivoFileGenerale})
		fine <- err
	}()
	b.aspettaInAttesa()
	ok(t, txA.Commit(b.ctx))
	err = <-fine
	if codiceSQL(err) == "40P01" {
		t.Fatal("stallo fra due gesti FOR KEY SHARE")
	}
	rifiutoCon(t, err, associazioni.CodiceNessunaAssociazioneAttiva)
	if got := b.storia(b.a1); got != "ACM-1100:chiusa:scollega_file" {
		t.Errorf("dopo i due gesti: %s", got)
	}
	if got := b.uno(`SELECT count(*)::text FROM associazione_evento WHERE allegato_id = $1 AND azione = 'sgancia'`, b.a1); got != "1" {
		t.Errorf("le chiusure nel giornale: %s", got)
	}

	// (3) R5 contro un gesto legacy: R5 registra a2 → S2 e tiene la RFQ FOR UPDATE; il vecchio «Scarta il file» chiede di
	// scollegare a2 da S1, aspetta la RFQ (FOR KEY SHARE) e poi vede la AR di R5
	bz3 := b.bozza(b.P, []pb{{b.a2, "k-s2"}}, nil)
	tx3, err := b.p.Begin(b.ctx)
	ok(t, err)
	defer tx3.Rollback(b.ctx) //nolint:errcheck
	q3 := db.New(tx3)
	_, err = q3.BloccaThread(b.ctx, b.thread)
	ok(t, err)
	at3 := b.testata(tx3, bz3)
	_, err = associazioni.Registra(b.ctx, q3, at3, associazioni.Registrazione{Allegato: b.a2, Componente: b.S2, ChiaveBozza: "k-s2", Provenienza: associazioni.ProvenienzaManuale})
	ok(t, err)
	go func() {
		txL, err := b.p.Begin(b.ctx)
		if err != nil {
			fine <- err
			return
		}
		defer txL.Rollback(b.ctx) //nolint:errcheck
		qL := db.New(txL)
		if _, err := qL.BloccaThreadCondiviso(b.ctx, b.thread); err != nil {
			fine <- err
			return
		}
		_, err = associazioni.Scollega(b.ctx, qL, b.atto(), associazioni.Scollegamento{Allegato: b.a2, Componente: b.S1, Motivo: associazioni.MotivoScartoLegacy})
		fine <- err
	}()
	b.aspettaInAttesa()
	b.chiudiBozza(tx3, bz3)
	ok(t, tx3.Commit(b.ctx))
	err = <-fine
	if codiceSQL(err) == "40P01" {
		t.Fatal("stallo fra R5 e un gesto legacy")
	}
	if r := rifiutoCon(t, err, associazioni.CodiceAssociazioneDiversa); r.Altrove.UUID != b.S2 {
		t.Errorf("il gesto legacy vede la AR di R5: %v", r.Altrove)
	}
	if got := b.storia(b.a2); got != "ACM-1200:attiva:-" {
		t.Errorf("a2 dopo R5 e il gesto legacy: %s", got)
	}
	if got := b.uno(`SELECT count(*)::text FROM (SELECT allegato_id FROM file_rfq_associazione WHERE stato = 'attiva' GROUP BY allegato_id HAVING count(*) > 1) x`); got != "0" {
		t.Errorf("allegati con due AR attive: %s", got)
	}
	b.nessunaDivergenza("dopo la concorrenza")
}

// Il legame della sorgente (provvisorio, base [T] D-AR02 A, NON cablato: INC-S1): la storia della fonte, l'evento
// «assegna» senza riga e la copia; nessuna riga in file_rfq_associazione. Il reinvio non scrive; una AR su un altro
// componente non nasce sullo STEP legato; un file con una AR attiva non si lega.
func TestV1C1cIlLegameDellaSorgente(t *testing.T) {
	b := nuovoBanco(t)
	var l1, l2 associazioni.EsitoSorgente
	lega := b.atto()
	ok(t, b.gesto(func(q *db.Queries) (err error) {
		if l1, err = associazioni.LegaSorgente(b.ctx, q, lega, associazioni.LegameSorgente{Allegato: b.a5, Prodotto: b.P}); err != nil {
			return err
		}
		l2, err = associazioni.LegaSorgente(b.ctx, q, lega, associazioni.LegameSorgente{Allegato: b.a5, Prodotto: b.P})
		return err
	}))
	if l1.Ripetuto || l1.FonteEvento == 0 || l1.Copia.UUID != b.prop5 || !l2.Ripetuto {
		t.Fatalf("il legame: %+v | %+v", l1, l2)
	}
	if got := b.uno(`SELECT azione || ':' || ruolo || ':' || (prodotto_id = $2)::text FROM fonte_step_evento WHERE allegato_id = $1`, b.a5, b.P); got != "caricamento:prodotto:true" {
		t.Errorf("la storia della fonte: %s", got)
	}
	if got := b.giornale(b.a5); got != "assegna:ponte:->ACM-1000:senza_riga" {
		t.Errorf("il giornale dello STEP: %s", got)
	}
	if got := b.copia(b.prop5); got != "ACM-1000:ACM-1000:ACM-1000" {
		t.Errorf("la copia dello STEP: %s", got)
	}
	if got := b.uno(`SELECT count(*)::text FROM file_rfq_associazione`); got != "0" {
		t.Errorf("il legame non e' una AR: %s righe", got)
	}
	// una AR su un altro componente non nasce sullo STEP legato
	_, err := b.r5(b.P, []pb{{b.a5, "k-s1"}}, nil, func(q *db.Queries, a associazioni.Atto) error {
		_, err := associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a5, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale})
		return err
	})
	if r := rifiutoCon(t, err, associazioni.CodiceGiaAssociataAltrove); r.Altrove.UUID != b.P {
		t.Errorf("il legame che tiene lo STEP: %v", r.Altrove)
	}
	// un file con una AR attiva non si lega
	_, err = b.r5(b.P, []pb{{b.a1, "k-s1"}}, nil, func(q *db.Queries, a associazioni.Atto) error {
		_, err := associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a1, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale})
		return err
	})
	ok(t, err)
	err = b.gesto(func(q *db.Queries) error {
		_, err := associazioni.LegaSorgente(b.ctx, q, b.atto(), associazioni.LegameSorgente{Allegato: b.a1, Prodotto: b.P})
		return err
	})
	rifiutoCon(t, err, associazioni.CodiceGiaAssociataAltrove)
	// un componente che non e' un finito non ha uno STEP sorgente
	err = b.gesto(func(q *db.Queries) error {
		_, err := associazioni.LegaSorgente(b.ctx, q, b.atto(), associazioni.LegameSorgente{Allegato: b.a3, Prodotto: b.S1})
		return err
	})
	rifiutoCon(t, err, associazioni.CodiceAttoNonValido)
}

// AllineaCodiceCopia (D-ASS-01): la ricodifica del componente, con la FK a tre campi differita, porta il codice nuovo
// nelle copie; la AR non cambia; la lettura conservata resta.
func TestV1C1cLAllineamentoDelCodice(t *testing.T) {
	b := nuovoBanco(t)
	var e associazioni.Esito
	_, err := b.r5(b.P, []pb{{b.a1, "k-s1"}}, nil, func(q *db.Queries, a associazioni.Atto) (err error) {
		e, err = associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a1, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale})
		return err
	})
	ok(t, err)
	// la ricodifica (come la correzione del codice delle pagine legacy): RFQ bloccata, FK differita, codice nuovo
	tx, err := b.p.Begin(b.ctx)
	ok(t, err)
	defer tx.Rollback(b.ctx) //nolint:errcheck
	q := db.New(tx)
	_, err = q.BloccaThread(b.ctx, b.thread)
	ok(t, err)
	_, err = tx.Exec(b.ctx, `SET CONSTRAINTS fk_proposta_componente DEFERRED`)
	ok(t, err)
	_, err = tx.Exec(b.ctx, `UPDATE componente SET codice = 'ACM-1101' WHERE componente_id = $1`, b.S1)
	ok(t, err)
	n, err := associazioni.AllineaCodiceCopia(b.ctx, q, b.thread, b.S1)
	ok(t, err)
	ok(t, tx.Commit(b.ctx))
	if n != 1 {
		t.Errorf("copie allineate: %d", n)
	}
	if got := b.copia(b.prop1); got != "ACM-1101:ACM-1101:acm-1100 rev b" {
		t.Errorf("la copia dopo la ricodifica: %s", got)
	}
	if got := b.uno(`SELECT (stato = 'attiva' AND componente_id = $2)::text FROM file_rfq_associazione WHERE associazione_id = $1`, e.Associazione.ID, b.S1); got != "true" {
		t.Errorf("la AR e' cambiata con la ricodifica")
	}
	b.nessunaDivergenza("dopo la ricodifica")
}

// INC-G5: una proposta che nasce dopo la AR (l'allegato caricato o estratto prima che l'ingest o il worker scrivano la
// proposta) nasce con la copia, dalla prima proposta dell'ingest (InsertPropostaSeAssente, anche senza RFQ) come dal
// risultato del worker (UpsertProposta); la lettura che arriva dopo resta nei dettagli; senza AR niente copia. La
// copia resta uguale alla fonte (Prova 11) e lo scollegamento rimette l'ultima lettura.
func TestV1C1cLaPropostaDopoLaAR(t *testing.T) {
	b := nuovoBanco(t)
	_, err := b.r5(b.P, []pb{{b.a2, "k-s2"}, {b.a6, "k-s1"}}, nil, func(q *db.Queries, a associazioni.Atto) error {
		if _, err := associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a2, Componente: b.S2, ChiaveBozza: "k-s2", Provenienza: associazioni.ProvenienzaManuale}); err != nil {
			return err
		}
		_, err := associazioni.Registra(b.ctx, q, a, associazioni.Registrazione{Allegato: b.a6, Componente: b.S1, ChiaveBozza: "k-s1", Provenienza: associazioni.ProvenienzaManuale})
		return err
	})
	ok(t, err)
	q := db.New(b.p)
	testo := func(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }
	// l'ingest, senza RFQ (messaggio non ancora agganciato): la proposta prende la RFQ e la copia della AR
	ok(t, q.InsertPropostaSeAssente(b.ctx, db.InsertPropostaSeAssenteParams{AllegatoID: b.a2, TipoProposto: db.TipoDocumentoDisegno2d,
		Codice: testo("acm-1200 letto"), Confidenza: 60, Fonte: db.FontePropostaNomeFile, Dettagli: json.RawMessage(`{"da": "ingest"}`)}))
	p2 := uuid.MustParse(b.uno(`SELECT proposta_id::text FROM documento_proposta WHERE allegato_id = $1`, b.a2))
	if got := b.copia(p2); got != "ACM-1200:ACM-1200:acm-1200 letto" {
		t.Errorf("la proposta dell'ingest dopo la AR: %s", got)
	}
	if got := b.uno(`SELECT (thread_id = $2 AND dettagli ->> 'da' = 'ingest')::text FROM documento_proposta WHERE proposta_id = $1`, p2, b.thread); got != "true" {
		t.Errorf("la proposta dell'ingest: RFQ o dettagli persi")
	}
	// il worker: la prima scrittura nasce con la copia, la lettura dopo resta nei dettagli e non tocca la copia
	up := db.UpsertPropostaParams{AllegatoID: b.a6, ThreadID: uuid.NullUUID{UUID: b.thread, Valid: true}, TipoProposto: db.TipoDocumentoDisegno2d,
		Codice: testo("acm-1100 c"), Confidenza: 70, Fonte: db.FontePropostaNomeFile, Dettagli: json.RawMessage(`{"valutazione": {"x": 1}}`)}
	p6, err := q.UpsertProposta(b.ctx, up)
	ok(t, err)
	if got := b.copia(p6.PropostaID); got != "ACM-1100:ACM-1100:acm-1100 c" {
		t.Errorf("la proposta del worker dopo la AR: %s", got)
	}
	up.Codice = testo("acm-1100 d")
	_, err = q.UpsertProposta(b.ctx, up)
	ok(t, err)
	if got := b.copia(p6.PropostaID); got != "ACM-1100:ACM-1100:acm-1100 d" {
		t.Errorf("la lettura dopo, con la copia: %s", got)
	}
	// senza AR: nessuna copia (a3 ha gia' la proposta: si prova su un allegato nuovo)
	a7 := uuid.New()
	if _, err := b.p.Exec(b.ctx, `INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il)
		SELECT $1, messaggio_id, 7, 'ACM-1300.pdf', 'pdf', repeat('8', 64), now() FROM allegato WHERE allegato_id = $2`, a7, b.a1); err != nil {
		t.Fatal(err)
	}
	ok(t, q.InsertPropostaSeAssente(b.ctx, db.InsertPropostaSeAssenteParams{AllegatoID: a7, ThreadID: uuid.NullUUID{UUID: b.thread, Valid: true},
		TipoProposto: db.TipoDocumentoDisegno2d, Codice: testo("ACM-1300"), Confidenza: 60, Fonte: db.FontePropostaNomeFile, Dettagli: json.RawMessage(`{}`)}))
	if got := b.uno(`SELECT COALESCE(componente_id::text, '-') || ':' || codice || ':' || (dettagli ? 'codice_letto')::text FROM documento_proposta WHERE allegato_id = $1`, a7); got != "-:ACM-1300:false" {
		t.Errorf("la proposta senza AR: %s", got)
	}
	b.nessunaDivergenza("dopo le proposte nate dopo la AR")
	// lo scollegamento rimette l'ultima lettura
	ok(t, b.gesto(func(q *db.Queries) error {
		_, err := associazioni.Scollega(b.ctx, q, b.atto(), associazioni.Scollegamento{Allegato: b.a6, Motivo: associazioni.MotivoScollegaFile})
		return err
	}))
	if got := b.copia(p6.PropostaID); got != "-:acm-1100 d:-" {
		t.Errorf("dopo lo scollegamento: %s", got)
	}
	b.nessunaDivergenza("dopo lo scollegamento")
}
