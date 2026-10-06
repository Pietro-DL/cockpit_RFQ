//go:build integrazione

// L4 — la verifica della BOM dalla fotografia letta sul database di prova (contratto di A1c, §7, famiglia B5; R80, R61
// A, T-B0-24; PO-15 sul DB): una scena ACME scritta con SQL sul database di prova, con lo STEP strutturale autorizzato
// e «Conferma l'albero» com'è nel DB (il componente e la relazione confermati, la riga del nodo e la riga dell'arco
// decise da una persona con il segno evidenza.albero), letta con caricatore.Carica in sola lettura, poi ValutaProdotti:
// la BOM di lavoro, nomenclatura e gerarchia verificate dall'adattatore legacy, la fonte non registrata; poi una
// quantità confermata che lo STEP contraddice e una rimozione aperta, che portano la gerarchia in conflitto.
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è
// pubblico.

package valutazione_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq/caricatore"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/testutil"
)

type scenaAlberoDB struct {
	operatore, thread, finito, figlio, docStep uuid.UUID
}

// costruisciScenaAlberoDB: la RFQ ACME con il finito manuale 7120300, il suo STEP (la radice #1 = 7120300 e il figlio
// #2 = 7120301, due volte) autorizzato con il gesto 3, e «Conferma l'albero» sul figlio, come la scrive il legacy: il
// componente 7120301 e la relazione confermati (qta 2), la riga del nodo confermata con il segno, la riga dell'arco
// tenuta con il segno e i due componenti. La riga della radice è quella del gesto 3.
func costruisciScenaAlberoDB(t *testing.T, p *pgxpool.Pool) scenaAlberoDB {
	t.Helper()
	ctx := context.Background()
	riga := func(dst any, sql string, arg ...any) {
		t.Helper()
		if err := p.QueryRow(ctx, sql, arg...).Scan(dst); err != nil {
			t.Fatalf("scena: %v\n%s", err, sql)
		}
	}
	esegui := func(sql string, arg ...any) {
		t.Helper()
		if _, err := p.Exec(ctx, sql, arg...); err != nil {
			t.Fatalf("scena: %v\n%s", err, sql)
		}
	}
	var s scenaAlberoDB
	var cliente, conv, m1, aStep uuid.UUID
	riga(&s.operatore, `INSERT INTO utente (sigla, nome, ufficio) VALUES ('AC', 'Prova ACME', 'Tecnico') RETURNING utente_id`)
	riga(&cliente, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME', 'ACME S.p.A.') RETURNING cliente_id`)
	riga(&s.thread, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, cartella_relativa, oggetto, creato_da)
		VALUES ($1, 'outlook', now() - interval '3 hours', 'ACME\WIP\rfq-b5', 'RFQ ACME26-050', $2) RETURNING thread_id`, cliente, s.operatore)
	riga(&conv, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'C-ACME-B5', now()) RETURNING conversazione_id`)
	riga(&m1, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, thread_id, aggancio, agganciato_da, agganciato_il, direzione,
		data_evento, oggetto, corpo_testo, controparte_tipo, controparte_cliente_id) VALUES ('outlook', '<b5@acme.example>', $1, $2, 'operatore', $3,
		now() - interval '150 minutes', 'entrata', now() - interval '160 minutes', 'RFQ ACME26-050',
		E'Buongiorno,\r\nvi chiediamo l''offerta per l''assieme.\r\nGrazie', 'cliente', $4) RETURNING messaggio_id`, conv, s.thread, s.operatore, cliente)
	shaStep := sha("5")
	riga(&aStep, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, stato, ricevuto_il)
		VALUES ($1, 1, '7120300.stp', 'stp', 'file', 'outlook', 100, $2, 'analizzato', now()) RETURNING allegato_id`, m1, shaStep)
	esegui(`INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES ($1, $2)`, versioneAnalizzatore, hashConfigurazione)
	esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, $2, $3, $4)`,
		shaStep, versioneAnalizzatore, hashConfigurazione, strutturaDB("7120300", "7120301"))

	riga(&s.finito, `INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, '7120300', 'finito', 'manuale', $2)
		RETURNING componente_id`, s.thread, s.operatore)
	esegui(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, id_grezzo, codice, famiglia, fonte, confidenza)
		VALUES ($1, $2, $3, '#1', '', '7120300', '7120300', 'acme', 'step', 60), ($1, $2, $3, '#2', '', '7120301', '7120301', 'acme', 'step', 60)`,
		s.thread, aStep, shaStep)
	esegui(`INSERT INTO relazione_proposta (thread_id, allegato_id, padre_chiave, figlio_chiave, qta) VALUES ($1, $2, '#1', '#2', 2)`, s.thread, aStep)
	s.docStep = testutil.AutorizzaStep(t, p, s.thread, s.finito, aStep, s.operatore)
	esegui(`INSERT INTO documento_provenienza (documento_id, allegato_id, ricevuto_il) VALUES ($1, $2, now()) ON CONFLICT DO NOTHING`, s.docStep, aStep)

	// «Conferma l'albero» sul figlio (albero_conferma.go: il segno sulle righe decise da una persona).
	riga(&s.figlio, `INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, '7120301', 'sciolto', 'step', $2)
		RETURNING componente_id`, s.thread, s.operatore)
	esegui(`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ($1, $2, $3, 2, 'step', $4)`,
		s.thread, s.finito, s.figlio, s.operatore)
	segnoNodo, _ := json.Marshal(map[string]any{"da": s.operatore, "il": "2026-10-02T09:00:00Z", "firma": strings.Repeat("f", 64)})
	segnoArco, _ := json.Marshal(map[string]any{"da": s.operatore, "il": "2026-10-02T09:00:00Z", "firma": strings.Repeat("f", 64),
		"padre": s.finito, "figlio": s.figlio})
	esegui(`UPDATE componente_proposta SET stato = 'confermata', componente_id = $3, deciso_da = $4, deciso_il = now(),
		evidenza = evidenza || jsonb_build_object('albero', $5::jsonb) WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#2'`,
		s.thread, aStep, s.figlio, s.operatore, segnoNodo)
	esegui(`UPDATE relazione_proposta SET stato = 'confermata', deciso_da = $3, deciso_il = now(),
		evidenza = evidenza || jsonb_build_object('albero', $4::jsonb) WHERE thread_id = $1 AND allegato_id = $2 AND padre_chiave = '#1'`,
		s.thread, aStep, s.operatore, segnoArco)
	return s
}

// TestL4LaBOMDaConfermaLAlbero (PO-15 sul DB; R80, R61 A, T-B0-24; A1c-L4, famiglia B5): dalla fotografia del
// caricatore, la BOM di lavoro sotto la radice scelta e la verifica legacy di nomenclatura e gerarchia, con la fonte
// non registrata; la lettura non cambia il database e ValutaProdotti non ci parla. Poi la relazione confermata con
// un'altra quantità, e una rimozione aperta: la gerarchia in conflitto, la nomenclatura no.
func TestL4LaBOMDaConfermaLAlbero(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScenaAlberoDB(t, p)
	pr, reg := testutil.PoolConRegistro(t, testutil.DSN(t))
	m := motoreACME(t)
	rif := "componente:" + s.finito.String()

	leggi := func(nome string) valutazione.ValutazioneProdotti {
		t.Helper()
		prima := testutil.FotoDelDatabase(t, p)
		f, err := caricatore.Carica(context.Background(), pr, caricatore.Richiesta{Thread: []uuid.UUID{s.thread}})
		if err != nil {
			t.Fatalf("%s: Carica: %v", nome, err)
		}
		reg.Azzera()
		v, err := valutazione.ValutaProdotti(f, f.Thread[0], m, nil)
		if err != nil {
			t.Fatalf("%s: ValutaProdotti: %v", nome, err)
		}
		if len(reg.Voci()) != 0 {
			t.Errorf("%s: ValutaProdotti ha parlato con il database: %v", nome, reg.Voci())
		}
		if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
			t.Errorf("%s: la lettura ha cambiato il database: %v", nome, d)
		}
		return v
	}

	v := leggi("albero confermato")
	pv := prodotto(t, v, rif)
	if statoMotivo(pv.Fonte) != "confermata/estrazione_riuscita" || pv.Struttura != ancoraggio.StatoBOMDiLavoroProposta {
		t.Fatalf("fonte %s, struttura %s", statoMotivo(pv.Fonte), pv.Struttura)
	}
	b := pv.BOM
	if !b.Verificata || asse(b.Nomenclatura) != "verificata/" || asse(b.Gerarchia) != "verificata/" || !b.Nomenclatura.Legacy || b.FonteRegistrata ||
		b.SenzaFigli || b.RimozioniAperte != 0 {
		t.Errorf("BOM %+v", b)
	}
	if len(conCodice(v.Diagnostiche, valutazione.CodiceBOMFonteNonRegistrata)) != 1 || len(v.Conflitti) != 0 {
		t.Errorf("diagnostiche %+v, conflitti %+v", v.Diagnostiche, v.Conflitti)
	}

	ctx := context.Background()
	if _, err := p.Exec(ctx, `UPDATE componente_relazione SET qta = 3 WHERE padre_id = $1 AND figlio_id = $2`, s.finito, s.figlio); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO rimozione_proposta (thread_id, step_documento_id, padre_id, figlio_id, qta_working) VALUES ($1, $2, $3, $4, 2)`,
		s.thread, s.docStep, s.finito, s.figlio); err != nil {
		t.Fatal(err)
	}
	v = leggi("quantità e rimozione")
	b = prodotto(t, v, rif).BOM
	if asse(b.Gerarchia) != "conflitto/conflitto" || asse(b.Nomenclatura) != "verificata/" || b.Verificata || b.RimozioniAperte != 1 {
		t.Errorf("BOM %+v", b)
	}
	motivi := map[string]bool{}
	for _, c := range v.Conflitti {
		motivi[c.Motivo] = c.Asse == valutazione.AsseGerarchia && c.Tipo == valutazione.ConflittoArco && c.EvidenzaDecisione.Da != nil &&
			*c.EvidenzaDecisione.Da == s.operatore
	}
	if len(v.Conflitti) != 2 || !motivi[valutazione.MotivoConflittoQuantita] || !motivi[valutazione.MotivoConflittoRimozione] {
		t.Errorf("conflitti %+v", v.Conflitti)
	}
}
