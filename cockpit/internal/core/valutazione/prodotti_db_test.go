//go:build integrazione

// L4 — lo stato del prodotto e il fascicolo sulla fotografia letta dal database di prova (B6, V3; PO-01, PO-02 con e senza
// la bom_versione congelata, PO-31 e PO-19 in L4 sintetica; R79, R88, R89, R96 (b) B e (c) A [R]; T-B0-28, LD-23): la
// scena ACME della BOM (costruisciScenaAlberoDB: il finito con lo STEP autorizzato e «Conferma l'albero» sul figlio) con i
// 2D del finito e del figlio confermati e i loro file, scritta con SQL; letta con caricatore.Carica in sola lettura, poi
// Calcola, che non parla con il database. Il finito è verificato e pronto, e il fascicolo è congelabile ma non congelato
// (gesto_non_registrato), anche con la V1 congelata del legacy, che si mostra in Legacy; poi un identificativo confermato
// senza componente, un target non verificato: il fascicolo non è congelabile, con il motivo della bom_versione; poi la V2
// in bozza: il fascicolo legacy è riaperto.
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è
// pubblico.

package valutazione_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq/caricatore"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/testutil"
)

// TestL4IlFascicoloDalDatabase (PO-01, PO-02, PO-19, PO-31 in L4 sintetica).
func TestL4IlFascicoloDalDatabase(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScenaAlberoDB(t, p)
	ctx := context.Background()
	esegui := func(sql string, arg ...any) {
		t.Helper()
		if _, err := p.Exec(ctx, sql, arg...); err != nil {
			t.Fatalf("scena: %v\n%s", err, sql)
		}
	}
	riga := func(dst any, sql string, arg ...any) {
		t.Helper()
		if err := p.QueryRow(ctx, sql, arg...).Scan(dst); err != nil {
			t.Fatalf("scena: %v\n%s", err, sql)
		}
	}
	var cliente, m1 uuid.UUID
	var fase string
	riga(&cliente, `SELECT cliente_id FROM thread_offerta WHERE thread_id = $1`, s.thread)
	riga(&fase, `SELECT stato::text FROM thread_offerta WHERE thread_id = $1`, s.thread)
	riga(&m1, `SELECT messaggio_id FROM messaggio WHERE thread_id = $1`, s.thread)
	// I 2D del finito e del figlio: il file PDF (una scansione, valida), il documento confermato sul componente con il suo
	// codice (il vincolo del DB), la provenienza.
	disegno := func(comp uuid.UUID, codice string, indice int, nome, s256 string) {
		t.Helper()
		var a, d uuid.UUID
		riga(&a, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, stato, ricevuto_il)
			VALUES ($1, $2, $3, 'pdf', 'file', 'outlook', 100, $4, 'analizzato', now()) RETURNING allegato_id`, m1, indice, nome, s256)
		esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, $2, $3, $4)`,
			s256, versioneAnalizzatore, hashConfigurazione, payloadPDF(t, false))
		riga(&d, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
			VALUES ($1, $2, 'disegno_2d', $3, $4, 'pdf', $5, $6, $7) RETURNING documento_id`, s.thread, comp, codice, nome, s256, `DISEGNI\`+nome, s.operatore)
		esegui(`INSERT INTO documento_provenienza (documento_id, allegato_id, ricevuto_il) VALUES ($1, $2, now())`, d, a)
	}
	disegno(s.finito, "7120300", 2, "assieme-acme.pdf", sha("7"))
	disegno(s.figlio, "7120301", 3, "particolare-acme.pdf", sha("8"))

	pr, reg := testutil.PoolConRegistro(t, testutil.DSN(t))
	r := insiemeDelDB(t, cliente)
	esito := func(nome string) valutazione.EsitoThread {
		t.Helper()
		prima := testutil.FotoDelDatabase(t, p)
		f, err := caricatore.Carica(ctx, pr, caricatore.Richiesta{Thread: []uuid.UUID{s.thread}})
		if err != nil {
			t.Fatalf("%s: Carica: %v", nome, err)
		}
		reg.Azzera()
		e := calcola(t, f, r, valutazione.Ingressi{})
		if len(reg.Voci()) != 0 {
			t.Errorf("%s: Calcola ha parlato con il database: %v", nome, reg.Voci())
		}
		if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
			t.Errorf("%s: la lettura ha cambiato il database: %v", nome, d)
		}
		if len(e.Thread) != 1 || !e.Thread[0].Valutato {
			t.Fatalf("%s: esito %+v", nome, e.Thread)
		}
		return e.Thread[0]
	}
	rifFinito := "componente:" + s.finito.String()

	// PO-02 senza bom_versione: l'unico prodotto è verificato, il fascicolo congelabile e non congelato.
	et := esito("il finito verificato")
	pv := prodottoDi(t, et, rifFinito)
	if statoProdotto(pv) != "sì/pronto_fattibilita/" || len(pv.Impronta) != 64 || len(pv.Nodi) != 2 {
		t.Fatalf("il finito: %s (fonte %s, BOM %v, smistamento %s, documenti %s), %d nodi", statoProdotto(pv), pv.Fonte.Stato, pv.BOM.Verificata,
			pv.Smistamento.Stato, statoDi(pv.Documenti), len(pv.Nodi))
	}
	if fa := et.Fascicolo; fascicoloTesto(fa) != "congelabile sì/ congelato no/gesto_non_registrato conflitti [] avvisi []" || fa.Legacy != nil ||
		fa.FaseThread != fase || fa.NumeroTarget != 1 {
		t.Errorf("PO-02, senza bom_versione: %+v", fa)
	}
	impronta := pv.Impronta

	// PO-02 con la V1 congelata del legacy (R96 b B): il fascicolo resta non congelato, e la V1 si mostra in Legacy.
	var faseLog, v1 uuid.UUID
	riga(&faseLog, `INSERT INTO fase_log (thread_id, nome_fase, inizio) VALUES ($1, 'FATTIBILITA', now() - interval '1 hour') RETURNING fase_log_id`, s.thread)
	riga(&v1, `INSERT INTO bom_versione (thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo, creata_da)
		VALUES ($1, 1, 'preventivo', 'FATTIBILITA', $2, 'prima versione ACME', $3) RETURNING bom_versione_id`, s.thread, faseLog, s.operatore)
	esegui(`UPDATE bom_versione SET stato = 'congelata', congelata_da = $2, congelata_il = now() WHERE bom_versione_id = $1`, v1, s.operatore)
	et = esito("la V1 congelata")
	fa := et.Fascicolo
	if fascicoloTesto(fa) != "congelabile sì/ congelato no/gesto_non_registrato conflitti [] avvisi []" || legacyTesto(fa.Legacy) != "1/congelata/1/no" ||
		fa.Legacy.CongelataDa == nil || *fa.Legacy.CongelataDa != s.operatore || fa.Legacy.CongelataIl == nil {
		t.Errorf("PO-02, con la V1 congelata: %+v, legacy %+v", fa, fa.Legacy)
	}
	if pv := prodottoDi(t, et, rifFinito); pv.Impronta != impronta {
		t.Error("la bom_versione del legacy cambia l'impronta del prodotto (T-B0-29)")
	}

	// PO-01 e PO-19: un identificativo confermato senza componente, un target confermato non verificato; con la V1 congelata
	// corrente il motivo è quello della bom_versione (T-B0-28), con prodotti_non_pronti fra gli avvisi.
	esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confermato_da) VALUES ($1, '7120399', 'manuale', $2)`, s.thread, s.operatore)
	et = esito("il secondo target")
	q := prodottoDi(t, et, "identificativo:7120399")
	if statoProdotto(prodottoDi(t, et, rifFinito)) != "sì/pronto_fattibilita/" || !valutazione.TargetConfermato(q) ||
		!strings.HasPrefix(statoProdotto(q), "no/non_pronto/fonte_strutturale_step_mancante_o_non_confermata") {
		t.Errorf("PO-01: il finito %s, l'identificativo %s", statoProdotto(prodottoDi(t, et, rifFinito)), statoProdotto(q))
	}
	fa = et.Fascicolo
	if fascicoloTesto(fa) != "congelabile no/bom_versione_con_prodotti_non_verificati congelato no/gesto_non_registrato conflitti [] avvisi [non_congelabile:prodotti_non_pronti]" ||
		fa.NumeroTarget != 2 || fa.NumeroVerificati != 1 || len(fa.Pronti) != 1 || fa.Pronti[0] != rifFinito || len(fa.Bloccati) != 1 {
		t.Errorf("PO-01, con la V1 congelata: %+v", fa)
	}

	// PO-31: la V2 in bozza dopo la V1 congelata (R96 c A): il legacy è riaperto, e il motivo torna prodotti_non_pronti.
	esegui(`UPDATE fase_log SET fine = now(), esito = 'OK' WHERE thread_id = $1 AND fine IS NULL`, s.thread)
	riga(&faseLog, `INSERT INTO fase_log (thread_id, nome_fase, inizio) VALUES ($1, 'SCHEDA_COSTO', now()) RETURNING fase_log_id`, s.thread)
	esegui(`INSERT INTO bom_versione (thread_id, numero, contesto, fase_all_apertura, fase_log_id_all_apertura, versione_precedente_id,
		numero_precedente, stato_precedente, motivo, creata_da) VALUES ($1, 2, 'preventivo', 'SCHEDA_COSTO', $2, $3, 1, 'congelata',
		'revisione ACME', $4)`, s.thread, faseLog, v1, s.operatore)
	fa = esito("la V2 in bozza").Fascicolo
	if fascicoloTesto(fa) != "congelabile no/prodotti_non_pronti congelato no/gesto_non_registrato conflitti [] avvisi []" ||
		legacyTesto(fa.Legacy) != "2/bozza/1/sì" {
		t.Errorf("PO-31, la V2 in bozza: %+v, legacy %+v", fa, fa.Legacy)
	}
}
