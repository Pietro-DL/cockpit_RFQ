//go:build integrazione

// L4 — giro 5, A1d-Q2: l'equivalenza fra la pagina della Distinta nuova (transport/web/motore_a, Servizio.Calcola) e il
// percorso del banco, sul database di prova (COCKPIT_TEST_DSN, cockpit_test; Q-A1d-02 = A [U]); direttiva A1d §3.1
// («verificare l'equivalenza canonica dell'EsitoThread con il percorso del banco, dichiarando le differenze previste
// dell'impronta complessiva»); bozza delle risposte §14 P2/Q2 e F-05 della revisione della PR #18 (la prova della PR
// rifaceva il percorso della pagina invece di quello del banco).
//
// IL PERCORSO DEL BANCO: LE SUE FUNZIONI, CON LA SEQUENZA RICOPIATA SOTTO GUARDIA. Del banco girano le funzioni vere
// (messaggiFuori, clientiFuori, caricaSenzaIMancanti, inFiles, inProdotti) su un valore banco con le opzioni e gli
// ingressi della corsa; la sequenza che le lega è ricopiata da fotografiaDSN (il letterale caricatore.Richiesta, con
// Thread, Tutti, Messaggi, Clienti come nel banco) e da valuta (valut.Calcola(f, b.ins, b.ingressi), poi inFiles,
// inProdotti e confronto.Confronta senza atteso, come senza -attesi). Che la copia sia ancora quella del banco lo dice
// la guardia L1 TestEquivalenzaMotoreAIlPercorsoDelBancoNonDeriva (motore_a_equivalenza_test.go, Q2-01 della revisione
// di A1d-P2): legge banco.go e questo file e li confronta; se il banco cambia la Richiesta o la sequenza, la guardia
// cade. Il pool del banco si apre come nel banco, con migrazioni.ApriInLettura (default_transaction_read_only=on e il
// controllo dello schema). EseguiBanco intero non si può usare qui: sul database di prova il ruolo può scrivere, e
// ControllaSolaLettura ferma la corsa prima della fotografia (lo fissa TestL4BancoDSNFermoSulRuoloCheScrive): la prova
// chiama i passi che seguono quel controllo.
//
// PERCHÉ QUI. La prova vuole le funzioni non esportate del banco: sta quindi nel package bancoa, e importa
// transport/web/motore_a. È una freccia che esiste solo nei test (app/bancoa → transport/web/motore_a), dichiarata in
// internal/README.md e nel README di bancoa: nel codice di prodotto il banco non importa mai transport (la guardia G1,
// core/estrazione/evidenze/dipendenze_test.go, legge i soli file non di prova). L'opposto (transport → app) non si può
// nemmeno nei test senza esportare le funzioni del banco.
//
// LE SCENE (Q-A1d-10 = B, raccomandazione della bozza: la pagina non usa gli ingressi di scenario):
//   - il banco con -thread sul solo thread, senza un caso di scenario (il file dei casi ha solo un caso senza thread,
//     con un messaggio fuori RFQ e il suo cliente): l'EsitoThread canonico (jsoncanonico) e confronto.Esito.Impronta sono
//     UGUALI; Esito.Impronta ed Esito.ImprontaFotografia sono DIVERSE, perché la fotografia del banco ha il messaggio
//     fuori RFQ e il cliente in più, e la pagina no (la differenza dichiarata);
//   - il banco con -tutti (Q2-02 della revisione di A1d-P2): la fotografia ha anche un secondo thread dello stesso
//     cliente, con lo stesso STEP (lo stesso sha256) e lo stesso codice del prodotto; l'EsitoThread canonico del thread
//     della pagina e l'impronta del suo confronto restano UGUALI a quelli della pagina;
//   - con un caso di scenario per il thread (autorità «scenario»): l'EsitoThread del banco e quello della pagina
//     DIFFERISCONO, e la differenza viene solo dagli ingressi: la stessa fotografia del banco valutata senza ingressi dà
//     l'EsitoThread della pagina.
//
// In ogni scena l'EsitoThread della pagina deve essere pieno (prodotti, file confrontabili, strutture, candidati di
// ancoraggio): uguali per vuotezza non vale (Q2-03).
//
// Nuova in A1d-Q2: sostituisce A1d-L4-06 della PR #18 di Stefano Valo (153117d: motore_a_db_test.go), che rifaceva il
// percorso della pagina.
//
// La scena è inventata (ACME, codici 912xxxx, acme.example, UUID del database di prova): il repository è pubblico.

package bancoa

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit"
	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/fotorfq/caricatore"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	valut "promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
	"promatec/cockpit/internal/platform/migrazioni"
	"promatec/cockpit/internal/platform/testutil"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// eqSorgente: la sorgente della fotografia del banco in questa prova (nel banco è la destinazione del rapporto).
const eqSorgente = "cockpit_test (prova)"

// eqScena: la RFQ ACME, un secondo thread dello stesso cliente e il messaggio fuori RFQ di un secondo cliente.
type eqScena struct {
	cliente, estero, thread, altro, messaggio, fuori uuid.UUID
}

// eqCostruisci scrive la scena: la RFQ ACME (un messaggio agganciato da una persona, uno STEP con la radice 9120100A e il
// figlio 9120101A letto dall'analizzatore corrente, un PDF con la sua proposta, il prodotto e il figlio, le righe di
// proposta dello STEP, il codice confermato); un secondo thread dello stesso cliente («RFQ ACME26-042») con lo stesso
// STEP (lo stesso sha256, un altro allegato), il prodotto con lo stesso codice e le sue righe di proposta; un messaggio
// senza RFQ di «ACME Estero S.p.A.», che solo il banco fotografa (il caso senza thread del file dei casi).
func eqCostruisci(t *testing.T, p *pgxpool.Pool) eqScena {
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
	var s eqScena
	var utente, conv, conv2, conv3, aStep, aPdf, mAltro, aStepAltro uuid.UUID
	shaStep, shaPdf := strings.Repeat("b1", 32), strings.Repeat("b2", 32)
	riga(&utente, `INSERT INTO utente (sigla, nome, ufficio) VALUES ('AQ', 'Prova ACME A1d', 'Tecnico') RETURNING utente_id`)
	riga(&s.cliente, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME', 'ACME S.p.A.') RETURNING cliente_id`)
	riga(&s.estero, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME-ESTERO', 'ACME Estero S.p.A.') RETURNING cliente_id`)
	riga(&s.thread, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, cartella_relativa, oggetto, creato_da)
		VALUES ($1, 'outlook', now() - interval '3 hours', 'ACME\WIP\rfq-a1d-eq', 'RFQ ACME26-041', $2) RETURNING thread_id`, s.cliente, utente)
	riga(&conv, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'C-ACME-A1D-EQ', now()) RETURNING conversazione_id`)
	riga(&s.messaggio, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, thread_id, aggancio, agganciato_da, agganciato_il, direzione,
		data_evento, oggetto, corpo_testo, controparte_tipo, controparte_cliente_id) VALUES ('outlook', '<a1d-eq@acme.example>', $1, $2, 'operatore', $3,
		now() - interval '150 minutes', 'entrata', now() - interval '160 minutes', 'RFQ ACME26-041',
		E'Buongiorno,\r\nvi chiediamo l''offerta per 9120100A.\r\nGrazie', 'cliente', $4) RETURNING messaggio_id`,
		conv, s.thread, utente, s.cliente)
	riga(&conv2, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'C-ACME-A1D-FUORI', now()) RETURNING conversazione_id`)
	riga(&s.fuori, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto, corpo_testo, controparte_tipo,
		controparte_cliente_id) VALUES ('outlook', '<a1d-fuori@acme.example>', $1, 'entrata', now() - interval '60 minutes', 'Richiesta ACME Estero',
		'Buongiorno, un''offerta per 9120200A.', 'cliente', $2) RETURNING messaggio_id`, conv2, s.estero)
	allegato := func(dst *uuid.UUID, messaggio uuid.UUID, indice int, nome, est, sha string) {
		t.Helper()
		riga(dst, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, stato, ricevuto_il)
			VALUES ($1, $2, $3, $4, 'file', 'outlook', 100, $5, 'analizzato', now()) RETURNING allegato_id`, messaggio, indice, nome, est, sha)
	}
	allegato(&aStep, s.messaggio, 1, "9120100A.stp", "stp", shaStep)
	allegato(&aPdf, s.messaggio, 2, "9120101A_1.pdf", "pdf", shaPdf)
	esegui(`INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES (4, $1)`, strings.Repeat("c", 64))
	struttura := `{"struttura": {"versione": 3, "schema": "AP214", "radici": ["#1"], "avvisi": [], "nodi": [` +
		`{"chiave": "#1", "id_grezzo": "9120100A", "nome_grezzo": "", "evidenza": {}}, {"chiave": "#2", "id_grezzo": "9120101A", "nome_grezzo": "", "evidenza": {}}], ` +
		`"relazioni": [{"padre": "#1", "figlio": "#2", "qta": 2, "evidenza": {}}], "limiti": {"troncato": false}, ` +
		`"scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`
	esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 4, $2, $3), ($4, 4, $2, $5)`,
		shaStep, strings.Repeat("c", 64), struttura, shaPdf, `{"testo_pdf": {"pagine": [{"testo": "9120101A2"}]}}`)
	esegui(`INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, '9120100', 'finito', 'manuale', $2)`, s.thread, utente)
	esegui(`INSERT INTO componente (thread_id, codice, rev, tipo, origine, confermato_da) VALUES ($1, '9120101', '2', 'sciolto', 'step', $2)`, s.thread, utente)
	esegui(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, id_grezzo, codice, famiglia, fonte, confidenza)
		VALUES ($1, $2, $3, '#1', '', '9120100A', '9120100', 'acme', 'step', 60), ($1, $2, $3, '#2', '', '9120101A', '9120101', 'acme', 'step', 60)`,
		s.thread, aStep, shaStep)
	esegui(`INSERT INTO relazione_proposta (thread_id, allegato_id, padre_chiave, figlio_chiave, qta) VALUES ($1, $2, '#1', '#2', 2)`, s.thread, aStep)
	esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confermato_da) VALUES ($1, '9120100', 'proposta_corpo', $2)`, s.thread, utente)
	esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte)
		VALUES ($1, $2, 'disegno_2d', '9120101', '2', 50, 'nome_file')`, aPdf, s.thread)

	// il secondo thread dello stesso cliente, con lo stesso STEP (Q2-02): solo il banco con -tutti lo fotografa
	riga(&s.altro, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, cartella_relativa, oggetto, creato_da)
		VALUES ($1, 'outlook', now() - interval '2 hours', 'ACME\WIP\rfq-a1d-eq2', 'RFQ ACME26-042', $2) RETURNING thread_id`, s.cliente, utente)
	riga(&conv3, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'C-ACME-A1D-EQ2', now()) RETURNING conversazione_id`)
	riga(&mAltro, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, thread_id, aggancio, agganciato_da, agganciato_il, direzione,
		data_evento, oggetto, corpo_testo, controparte_tipo, controparte_cliente_id) VALUES ('outlook', '<a1d-eq2@acme.example>', $1, $2, 'operatore', $3,
		now() - interval '100 minutes', 'entrata', now() - interval '110 minutes', 'RFQ ACME26-042',
		E'Buongiorno,\r\nanche questa offerta per 9120100A.\r\nGrazie', 'cliente', $4) RETURNING messaggio_id`,
		conv3, s.altro, utente, s.cliente)
	allegato(&aStepAltro, mAltro, 1, "9120100A copia.stp", "stp", shaStep)
	esegui(`INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, '9120100', 'finito', 'manuale', $2)`, s.altro, utente)
	esegui(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, id_grezzo, codice, famiglia, fonte, confidenza)
		VALUES ($1, $2, $3, '#1', '', '9120100A', '9120100', 'acme', 'step', 60), ($1, $2, $3, '#2', '', '9120101A', '9120101', 'acme', 'step', 60)`,
		s.altro, aStepAltro, shaStep)
	esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confermato_da) VALUES ($1, '9120100', 'proposta_corpo', $2)`, s.altro, utente)
	return s
}

// eqInsieme: l'insieme delle regole con la grammatica ACME di app/runtime/testdata/motore_a (le testdata della PR #18),
// riscritta per il cliente della scena e compilata come all'avvio. Lo stesso insieme va al banco e alla pagina.
func eqInsieme(t *testing.T, cliente uuid.UUID) *motorea.InsiemeRegole {
	t.Helper()
	leggi := func(nome string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "runtime", "testdata", "motore_a", nome))
		if err != nil {
			t.Fatalf("regole ACME: %v", err)
		}
		return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	}
	ind, err := grammatica.LeggiIndice(leggi("indice_acme.v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g map[string]any
	dec := json.NewDecoder(bytes.NewReader(leggi("acme.v1.json")))
	dec.UseNumber()
	if err := dec.Decode(&g); err != nil {
		t.Fatal(err)
	}
	cl := g["cliente"].(map[string]any)
	cl["id"] = cliente.String()
	gram, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	ind.Grammatiche = []grammatica.VoceIndice{{ClienteID: cliente, File: "acme.v1.json", Sha256: jsoncanonico.Impronta(gram)}}
	ind.Casi = nil
	ins, diag, err := motorea.CompilaInsieme(ind, map[string][]byte{"acme.v1.json": gram})
	if err != nil || ins.Motori[cliente] == nil {
		t.Fatalf("regole ACME: %v %v", err, diag)
	}
	return &ins
}

// eqPoolDelBanco: il pool del banco, aperto come nel banco (migrazioni.ApriInLettura con le migrazioni incorporate:
// default_transaction_read_only=on e il controllo dello schema).
func eqPoolDelBanco(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := migrazioni.ApriInLettura(context.Background(), testutil.DSN(t), cockpit.FS)
	if err != nil {
		t.Fatalf("il pool del banco: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// eqPercorsoDelBanco: il percorso del modo -dsn dopo i controlli del collegamento, con le funzioni del banco su un valore
// banco con le opzioni e gli ingressi della corsa. La Richiesta e la sequenza sono quelle di fotografiaDSN e di valuta
// (senza -attesi): la guardia TestEquivalenzaMotoreAIlPercorsoDelBancoNonDeriva le confronta con banco.go, quindi qui
// si scrivono con gli stessi nomi (b.o.Thread, b.o.Tutti, b.clientiFuori(), b.messaggiFuori(), b.ins, b.ingressi).
func eqPercorsoDelBanco(t *testing.T, pool *pgxpool.Pool, thread uuid.UUID, ins *motorea.InsiemeRegole, in valut.Ingressi,
	o Opzioni) (fotorfq.Fotografia, valut.Esito, *valut.EsitoThread, confronto.Esito) {
	t.Helper()
	ctx := context.Background()
	b := &banco{o: o, ins: ins, ingressi: in}
	f, mancanti, err := caricaSenzaIMancanti(func(messaggi []uuid.UUID) (fotorfq.Fotografia, error) {
		return caricatore.Carica(ctx, pool, caricatore.Richiesta{Thread: b.o.Thread, Tutti: b.o.Tutti, Messaggi: messaggi,
			Clienti: b.clientiFuori(), Sorgente: eqSorgente})
	}, b.messaggiFuori())
	if err != nil || len(mancanti) != 0 {
		t.Fatalf("la fotografia del banco: %v (mancanti %v)", err, mancanti)
	}
	esito, err := valut.Calcola(f, b.ins, b.ingressi)
	if err != nil {
		t.Fatalf("Calcola del banco: %v", err)
	}
	for i, et := range esito.Thread {
		if et.ThreadID != thread {
			continue
		}
		righe := inFiles(et.Confrontabili)
		var atteso *confronto.Atteso // senza -attesi: nessun atteso
		ce := confronto.Confronta(righe, inProdotti(et.ProdottiConfrontabili), atteso)
		return f, esito, &esito.Thread[i], ce
	}
	t.Fatal("l'esito del banco non ha il thread")
	return fotorfq.Fotografia{}, valut.Esito{}, nil, confronto.Esito{}
}

func eqCanonico(t *testing.T, et *valut.EsitoThread) []byte {
	t.Helper()
	b, err := jsoncanonico.Codifica(et)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// eqPieno: l'EsitoThread non è vuoto (Q2-03): prodotti valutati, file confrontabili, strutture, candidati di ancoraggio.
func eqPieno(t *testing.T, chi string, et *valut.EsitoThread) {
	t.Helper()
	candidati := 0
	for _, f := range et.Ancoraggi.File {
		candidati += len(f.Candidati)
	}
	if !et.Valutato || len(et.ProdottiValutati) == 0 || len(et.Confrontabili) == 0 || len(et.Ancoraggi.Strutture) == 0 || candidati == 0 {
		t.Fatalf("%s: l'EsitoThread è vuoto (valutato %v, %d prodotti, %d confrontabili, %d strutture, %d candidati): uguali per vuotezza non vale",
			chi, et.Valutato, len(et.ProdottiValutati), len(et.Confrontabili), len(et.Ancoraggi.Strutture), candidati)
	}
}

func TestL4MotoreAStessoEsitoDelBanco(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := eqCostruisci(t, p)
	ins := eqInsieme(t, s.cliente)
	prima := testutil.FotoDelDatabase(t, p)
	pb := eqPoolDelBanco(t)
	fuori := valut.IngressoCaso{ID: "ACME-FUORI", ClienteID: s.estero, Messaggi: []uuid.UUID{s.fuori}}
	tid := s.thread
	scenario := valut.IngressoCaso{ID: "ACME-SCENARIO", ThreadID: &tid, ClienteID: s.cliente, Autorita: "scenario"}

	// la pagina: il servizio della Distinta nuova, con lo stesso insieme e nessun ingresso
	svc := motorea_web.Servizio{Regole: motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoPronta, Insieme: ins, Limite: 20 * time.Second,
		Ingressi: valut.Ingressi{Versione: valut.VersioneCasi, Casi: []valut.IngressoCaso{fuori, scenario}}}, Pool: p}
	pagina, err := svc.Calcola(context.Background(), s.thread)
	if err != nil {
		t.Fatalf("la pagina: %v", err)
	}
	eqPieno(t, "la pagina", pagina.EsitoThread)
	if len(pagina.Foto.Thread) != 1 {
		t.Fatalf("la fotografia della pagina ha %d thread, atteso 1", len(pagina.Foto.Thread))
	}

	t.Run("senza un caso di scenario, -thread: uguale", func(t *testing.T) {
		in := valut.Ingressi{Versione: valut.VersioneCasi, Casi: []valut.IngressoCaso{fuori}}
		fb, eb, etb, cb := eqPercorsoDelBanco(t, pb, s.thread, ins, in, Opzioni{Thread: []uuid.UUID{s.thread}})
		if len(fb.FuoriRFQ) != 1 || len(fb.Clienti) != 2 || len(pagina.Foto.FuoriRFQ) != 0 || len(pagina.Foto.Clienti) != 1 {
			t.Fatalf("le fotografie: banco %d fuori RFQ e %d clienti, pagina %d e %d (attesi 1, 2, 0, 1)",
				len(fb.FuoriRFQ), len(fb.Clienti), len(pagina.Foto.FuoriRFQ), len(pagina.Foto.Clienti))
		}
		eqPieno(t, "il banco", etb)
		if a, b := eqCanonico(t, etb), eqCanonico(t, pagina.EsitoThread); !bytes.Equal(a, b) {
			t.Errorf("l'EsitoThread canonico del banco e della pagina differiscono:\n--- banco\n%.2000s\n--- pagina\n%.2000s", a, b)
		}
		if cb.Impronta != pagina.Confronto.Impronta || len(cb.Impronta) != 64 {
			t.Errorf("l'impronta del confronto: banco %q, pagina %q", cb.Impronta, pagina.Confronto.Impronta)
		}
		// le differenze dichiarate: l'impronta complessiva e quella della fotografia
		if eb.Impronta == pagina.Esito.Impronta || eb.ImprontaFotografia == pagina.Esito.ImprontaFotografia {
			t.Errorf("Esito.Impronta (%q / %q) o ImprontaFotografia (%q / %q) coincidono: la differenza dichiarata non c'è",
				eb.Impronta, pagina.Esito.Impronta, eb.ImprontaFotografia, pagina.Esito.ImprontaFotografia)
		}
		if len(eb.FuoriRFQ) != 1 || len(pagina.Esito.FuoriRFQ) != 0 {
			t.Errorf("i fuori RFQ dell'esito: banco %d, pagina %d", len(eb.FuoriRFQ), len(pagina.Esito.FuoriRFQ))
		}
	})

	// Q2-02: il banco con -tutti fotografa anche il secondo thread dello stesso cliente, con lo stesso STEP; l'EsitoThread
	// del thread della pagina resta quello della pagina
	t.Run("senza un caso di scenario, -tutti: uguale per il thread della pagina", func(t *testing.T) {
		in := valut.Ingressi{Versione: valut.VersioneCasi, Casi: []valut.IngressoCaso{fuori}}
		fb, eb, etb, cb := eqPercorsoDelBanco(t, pb, s.thread, ins, in, Opzioni{Tutti: true})
		visti := map[uuid.UUID]bool{}
		for _, th := range fb.Thread {
			visti[th.ID] = true
		}
		if !visti[s.thread] || !visti[s.altro] || len(fb.FuoriRFQ) != 1 || len(fb.Clienti) != 2 {
			t.Fatalf("la fotografia del banco con -tutti: %d thread (quello della pagina %v, l'altro %v), %d fuori RFQ, %d clienti",
				len(fb.Thread), visti[s.thread], visti[s.altro], len(fb.FuoriRFQ), len(fb.Clienti))
		}
		altroValutato := false
		for i := range eb.Thread {
			if eb.Thread[i].ThreadID == s.altro {
				altroValutato = eb.Thread[i].Valutato
			}
		}
		if !altroValutato {
			t.Errorf("il secondo thread non è valutato dal banco: la scena non prova niente")
		}
		eqPieno(t, "il banco -tutti", etb)
		if a, b := eqCanonico(t, etb), eqCanonico(t, pagina.EsitoThread); !bytes.Equal(a, b) {
			t.Errorf("con -tutti l'EsitoThread canonico del thread differisce da quello della pagina:\n--- banco\n%.2000s\n--- pagina\n%.2000s", a, b)
		}
		if cb.Impronta != pagina.Confronto.Impronta || len(cb.Impronta) != 64 {
			t.Errorf("con -tutti l'impronta del confronto: banco %q, pagina %q", cb.Impronta, pagina.Confronto.Impronta)
		}
	})

	t.Run("con un caso di scenario: diverso, per gli ingressi", func(t *testing.T) {
		in := valut.Ingressi{Versione: valut.VersioneCasi, Casi: []valut.IngressoCaso{fuori, scenario}}
		fb, _, etb, _ := eqPercorsoDelBanco(t, pb, s.thread, ins, in, Opzioni{Thread: []uuid.UUID{s.thread}})
		if bytes.Equal(eqCanonico(t, etb), eqCanonico(t, pagina.EsitoThread)) {
			t.Fatal("con il caso di scenario l'EsitoThread del banco è quello della pagina: la differenza dichiarata (Q-A1d-10 = B) non c'è")
		}
		// la differenza viene solo dagli ingressi: la fotografia del banco, valutata senza ingressi, dà la pagina
		senza, err := valut.Calcola(fb, ins, valut.Ingressi{})
		if err != nil {
			t.Fatal(err)
		}
		var et *valut.EsitoThread
		for i := range senza.Thread {
			if senza.Thread[i].ThreadID == s.thread {
				et = &senza.Thread[i]
			}
		}
		if et == nil || !bytes.Equal(eqCanonico(t, et), eqCanonico(t, pagina.EsitoThread)) {
			t.Error("la fotografia del banco senza ingressi non dà l'EsitoThread della pagina: la differenza non viene solo dallo scenario")
		}
	})

	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("il banco o la pagina hanno cambiato il database: %v", d)
	}
}
