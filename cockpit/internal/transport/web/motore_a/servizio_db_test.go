//go:build integrazione

// L4 — giro 5, A1d-Q2: il servizio della Distinta nuova sul database di prova (COCKPIT_TEST_DSN, cockpit_test; Q-A1d-02
// = A [U]). Una RFQ ACME inventata (scenaMA), le regole ACME dell'avvio (app/runtime/testdata/motore_a, le testdata
// della PR #18, lette dal file e riscritte per il cliente che il database ha dato alla scena: transport non importa app).
//   - Il calcolo vero: caricatore.Carica → valutazione.Calcola → passaggio → confronto, con la versione valutazione-3,
//     l'esito del thread valutato, la fase e le dichiarazioni dello STEP dello stesso snapshot (l'indice dice
//     l'autorizzazione valida della scena).
//   - Lo snapshot unico, provato con il registro del testo SQL: un solo BEGIN, REPEATABLE READ READ ONLY, tutto sulla
//     stessa connessione, il caricatore dentro un SAVEPOINT (con il suo ROLLBACK TO SAVEPOINT), il ROLLBACK finale,
//     nessun testo che scrive o prende lucchetti; e con le risposte del database dentro il savepoint: i due SHOW del
//     caricatore dicono «on» e «repeatable read».
//   - La sola lettura provata davvero (F-20): una sonda di scrittura nella transazione dello snapshot e nel savepoint del
//     caricatore riceve SQLSTATE 25006; la foto del database prima e dopo il calcolo e l'anteprima è la stessa.
//   - La RFQ inesistente: ErrRFQInesistente da Calcola, da Esiste e da AnteprimaSTEP (F-10).
//   - L'anteprima dello STEP: in una transazione REPEATABLE READ READ ONLY sua, senza SQL che scrive, con la stessa firma
//     di fascicolo.EffettoAutorizzazione sul pool (il modulo legacy).
//   - Dopo la revisione di A1d-P2: l'esito del calcolo vero non è vuoto (Q2-03); la copia della regola T-B4-38
//     nell'indice (ComponenteDelNodo) dà su ogni nodo di ogni gruppo lo stesso risultato di valutazione (IX-03: la prova
//     gemella); il limite che scade a metà di una lettura vera dà ErrTempo, chiude la transazione con il ROLLBACK,
//     restituisce la connessione sana al pool e non cambia il database (R-03, Q2-05, R-04).
//
// Derivate da A1d-L4-05, -06 e -08 della PR #18 di Stefano Valo (153117d: motore_a_db_test.go), rifatte sul Servizio
// (PD-02 della revisione di A1d-P2).
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 912xxxx, acme.example): il repository è pubblico.

package motore_a

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
	"promatec/cockpit/internal/platform/testutil"
)

// regoleACMEPer: le regole pronte con la grammatica ACME di app/runtime/testdata/motore_a (le testdata della PR #18),
// riscritta per il cliente della scena (UUID e ragione sociale), compilata con motorea.CompilaInsieme come all'avvio;
// nessun file dei casi; il limite dato. I fine riga CRLF di un checkout Windows diventano LF.
func regoleACMEPer(t *testing.T, cliente uuid.UUID, ragione string, limite time.Duration) Regole {
	t.Helper()
	leggi := func(nome string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", "..", "app", "runtime", "testdata", "motore_a", nome))
		if err != nil {
			t.Fatalf("regole ACME: %v", err)
		}
		return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	}
	ind, err := grammatica.LeggiIndice(leggi("indice_acme.v1.json"))
	if err != nil {
		t.Fatalf("regole ACME: l'indice non si legge: %v", err)
	}
	var g map[string]any
	dec := json.NewDecoder(bytes.NewReader(leggi("acme.v1.json")))
	dec.UseNumber()
	if err := dec.Decode(&g); err != nil {
		t.Fatal(err)
	}
	cl := g["cliente"].(map[string]any)
	cl["id"], cl["ragione_sociale"] = cliente.String(), ragione
	gram, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	ind.Grammatiche = []grammatica.VoceIndice{{ClienteID: cliente, File: "acme.v1.json", Sha256: jsoncanonico.Impronta(gram)}}
	ind.Casi = nil
	ins, diag, err := motorea.CompilaInsieme(ind, map[string][]byte{"acme.v1.json": gram})
	if err != nil || ins.Motori[cliente] == nil {
		t.Fatalf("regole ACME: la grammatica del cliente della scena non si attiva: %v %v", err, diag)
	}
	return Regole{Accesa: true, Stato: StatoPronta, Insieme: &ins, Limite: limite, Attivi: []uuid.UUID{cliente}}
}

// scenaMA: la RFQ della scena.
type scenaMA struct {
	utente, cliente, thread, messaggio uuid.UUID
	aStep, aPdf                        uuid.UUID
	prodotto, figlio                   uuid.UUID
	docStep                            uuid.UUID
}

// costruisciScenaMA scrive sul database di prova (schema pulito) una RFQ ACME: un messaggio agganciato da una persona,
// uno STEP con la radice 9120100A e il figlio 9120101A (×2) letto dall'analizzatore corrente, un PDF, il prodotto
// 9120100 finito e il figlio 9120101, le righe di proposta dello STEP, il codice della richiesta confermato, la fase
// FATTIBILITA; poi lo STEP autorizzato per il prodotto (testutil.AutorizzaStep: il documento, la marcatura della radice,
// lo STEP strutturale del finito).
func costruisciScenaMA(t *testing.T, p *pgxpool.Pool) scenaMA {
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
	var s scenaMA
	shaStep, shaPdf := strings.Repeat("a1", 32), strings.Repeat("a2", 32)
	riga(&s.utente, `INSERT INTO utente (sigla, nome, ufficio) VALUES ('AM', 'Prova ACME A1d', 'Tecnico') RETURNING utente_id`)
	riga(&s.cliente, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME', 'ACME S.p.A.') RETURNING cliente_id`)
	riga(&s.thread, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, cartella_relativa, oggetto, riferimento_cliente, creato_da)
		VALUES ($1, 'outlook', now() - interval '3 hours', 'ACME\WIP\rfq-a1d', 'RFQ ACME26-040', 'ACME-040', $2) RETURNING thread_id`, s.cliente, s.utente)
	var conv uuid.UUID
	riga(&conv, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', 'C-ACME-A1D', now()) RETURNING conversazione_id`)
	riga(&s.messaggio, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, thread_id, aggancio, agganciato_da, agganciato_il, direzione,
		data_evento, oggetto, corpo_testo, controparte_tipo, controparte_cliente_id) VALUES ('outlook', '<a1d@acme.example>', $1, $2, 'operatore', $3,
		now() - interval '150 minutes', 'entrata', now() - interval '160 minutes', 'RFQ ACME26-040',
		E'Buongiorno,\r\nvi chiediamo l''offerta per 9120100A.\r\nGrazie', 'cliente', $4) RETURNING messaggio_id`,
		conv, s.thread, s.utente, s.cliente)
	allegato := func(dst *uuid.UUID, indice int, nome, est, sha string) {
		t.Helper()
		riga(dst, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, stato, ricevuto_il)
			VALUES ($1, $2, $3, $4, 'file', 'outlook', 100, $5, 'analizzato', now()) RETURNING allegato_id`, s.messaggio, indice, nome, est, sha)
	}
	allegato(&s.aStep, 1, "9120100A.stp", "stp", shaStep)
	allegato(&s.aPdf, 2, "9120101A_1.pdf", "pdf", shaPdf)
	esegui(`INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES (4, $1)`, strings.Repeat("c", 64))
	struttura := `{"struttura": {"versione": 3, "schema": "AP214", "radici": ["#1"], "avvisi": [], "nodi": [` +
		`{"chiave": "#1", "id_grezzo": "9120100A", "nome_grezzo": "", "evidenza": {}}, {"chiave": "#2", "id_grezzo": "9120101A", "nome_grezzo": "", "evidenza": {}}], ` +
		`"relazioni": [{"padre": "#1", "figlio": "#2", "qta": 2, "evidenza": {}}], "limiti": {"troncato": false}, ` +
		`"scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`
	esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 4, $2, $3), ($4, 4, $2, $5)`,
		shaStep, strings.Repeat("c", 64), struttura, shaPdf, `{"testo_pdf": {"pagine": [{"testo": "9120101A2"}]}}`)
	riga(&s.prodotto, `INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, '9120100', 'finito', 'manuale', $2)
		RETURNING componente_id`, s.thread, s.utente)
	riga(&s.figlio, `INSERT INTO componente (thread_id, codice, rev, tipo, origine, confermato_da) VALUES ($1, '9120101', '2', 'sciolto', 'step', $2)
		RETURNING componente_id`, s.thread, s.utente)
	esegui(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, id_grezzo, codice, famiglia, fonte, confidenza)
		VALUES ($1, $2, $3, '#1', '', '9120100A', '9120100', 'acme', 'step', 60), ($1, $2, $3, '#2', '', '9120101A', '9120101', 'acme', 'step', 60)`,
		s.thread, s.aStep, shaStep)
	esegui(`INSERT INTO relazione_proposta (thread_id, allegato_id, padre_chiave, figlio_chiave, qta) VALUES ($1, $2, '#1', '#2', 2)`, s.thread, s.aStep)
	esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confermato_da) VALUES ($1, '9120100', 'proposta_corpo', $2)`, s.thread, s.utente)
	esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte)
		VALUES ($1, $2, 'disegno_2d', '9120101', '2', 50, 'nome_file')`, s.aPdf, s.thread)
	esegui(`UPDATE fase_log SET fine = now(), esito = 'OK' WHERE thread_id = $1 AND fine IS NULL`, s.thread)
	esegui(`INSERT INTO fase_log (thread_id, nome_fase, inizio) VALUES ($1, 'FATTIBILITA', now())`, s.thread)
	s.docStep = testutil.AutorizzaStep(t, p, s.thread, s.prodotto, s.aStep, s.utente)
	return s
}

// preparaMA: il pool del database di prova con lo schema pulito, la scena e le regole ACME.
func preparaMA(t *testing.T) (*pgxpool.Pool, scenaMA, Regole) {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScenaMA(t, p)
	return p, s, regoleACMEPer(t, s.cliente, "ACME S.p.A.", 20*time.Second)
}

func TestL4MotoreACalcoloVero(t *testing.T) {
	p, s, r := preparaMA(t)
	prima := testutil.FotoDelDatabase(t, p)
	svc := Servizio{Regole: r, Pool: p}
	ris, err := svc.Calcola(context.Background(), s.thread)
	if err != nil {
		t.Fatalf("Calcola: %v", err)
	}
	if ris.Esito.VersioneValutazione != VersioneAttesa || ris.EsitoThread == nil || ris.EsitoThread.ThreadID != s.thread {
		t.Fatalf("versione %q, esito del thread %+v", ris.Esito.VersioneValutazione, ris.EsitoThread)
	}
	if !ris.EsitoThread.Valutato {
		t.Errorf("il thread non è valutato: %s", ris.EsitoThread.Motivo)
	}
	if ris.Foto.SolaLettura != "on" || ris.Foto.Isolamento != "repeatable read" {
		t.Errorf("la fotografia dice sola lettura %q, isolamento %q", ris.Foto.SolaLettura, ris.Foto.Isolamento)
	}
	if len(ris.Foto.Thread) != 1 || len(ris.Foto.FuoriRFQ) != 0 || len(ris.Foto.Clienti) != 1 {
		t.Errorf("la fotografia: %d thread, %d fuori RFQ, %d clienti; attesi 1, 0, 1", len(ris.Foto.Thread), len(ris.Foto.FuoriRFQ), len(ris.Foto.Clienti))
	}
	if len(ris.Confronto.Impronta) != 64 {
		t.Errorf("l'impronta del confronto %q", ris.Confronto.Impronta)
	}
	if ris.Fase == nil || ris.Fase.Nome != "FATTIBILITA" {
		t.Errorf("la fase dello snapshot: %+v", ris.Fase)
	}
	// Q2-03 della revisione di A1d-P2: un esito pieno, non uguale per vuotezza (prodotti, file confrontabili, strutture,
	// un file ancorato con un candidato)
	et := ris.EsitoThread
	candidati := 0
	for _, f := range et.Ancoraggi.File {
		candidati += len(f.Candidati)
	}
	if len(et.ProdottiValutati) == 0 || len(et.Confrontabili) == 0 || len(et.Ancoraggi.Strutture) == 0 || candidati == 0 {
		t.Errorf("l'esito del calcolo vero è vuoto: %d prodotti valutati, %d file confrontabili, %d strutture, %d candidati di ancoraggio",
			len(et.ProdottiValutati), len(et.Confrontabili), len(et.Ancoraggi.Strutture), candidati)
	}
	t.Logf("fase %+v; prodotti valutati %d; confrontabili %d; strutture %d; candidati %d; diagnostiche %d", ris.Fase,
		len(et.ProdottiValutati), len(et.Confrontabili), len(et.Ancoraggi.Strutture), candidati, len(et.Diagnostiche))
	// le dichiarazioni dello stesso snapshot: l'autorizzazione della scena, come la valuta il Fascicolo
	x := NuovoIndice(ris.Thread, ris.EsitoThread, &ris.Confronto, &ris.Dichiarazioni)
	// IX-03 della revisione di A1d-P2: la prova gemella della copia di T-B4-38 su questa scena (la radice ha la sua riga
	// decisa: il componente viene dalla decisione); il caso della regola della radice è TestL4MotoreALaRadiceSenzaDecisione
	if nodi, _ := maGemellaTB438(t, ris, x); nodi == 0 {
		t.Error("la prova gemella di T-B4-38 non ha nodi")
	}
	aa := x.Autorizzazioni(s.prodotto)
	if len(aa) != 1 || aa[0].Stato != AutorizzazioneValida || aa[0].Documento == nil || *aa[0].Documento != s.docStep || aa[0].NomeFile != "9120100A.stp" {
		t.Errorf("le autorizzazioni del prodotto: %+v", aa)
	}
	if m := x.Marcatura(s.aStep, "#1"); m == nil || m.Ruolo != fascicolo.RuoloRadice {
		t.Errorf("la marcatura della radice: %+v", m)
	}
	// la pagina composta e il JSON di /dati
	pg, err := svc.Pagina(context.Background(), RichiestaPagina{Thread: s.thread})
	if err != nil {
		t.Fatal(err)
	}
	if pg.Testa.Cliente != "ACME S.p.A." || pg.Testa.Oggetto != "RFQ ACME26-040" || pg.Errore != "" || pg.Esito == nil {
		t.Errorf("pagina: testa %+v, errore %q", pg.Testa, pg.Errore)
	}
	if _, err := json.Marshal(pg); err != nil {
		t.Errorf("il JSON della pagina: %v", err)
	}
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("il calcolo ha cambiato il database: %v", d)
	}
}

// maGemellaTB438: la prova gemella della copia di T-B4-38 (IX-03 della revisione di A1d-P2; indice.go,
// componenteDelNodo, copia di componenteDelNodo di core/valutazione, nodi.go). valutazione dà a un nodo il tipo
// confermato del componente (datiDelNodo con il componente, OrigineTipo confermato) se e solo se ha un componente
// (componenti attivi della fotografia, tutti con un tipo); senza, il tipo è proposto. Quindi su ogni nodo di ogni gruppo
// dell'esito vero ComponenteDelNodo c'è se e solo se OrigineTipo è confermato, e allora il tipo del nodo è quello del
// componente nella fotografia. Restituisce i nodi confrontati e quelli con il componente dalla regola della radice
// (nessuna Decisione propria).
func maGemellaTB438(t *testing.T, ris *Risultato, x *Indice) (nodi, dallaRadice int) {
	t.Helper()
	tipoDi := map[uuid.UUID]string{}
	for _, c := range ris.Thread.Componenti {
		tipoDi[c.ID] = c.Tipo
	}
	for _, pv := range ris.EsitoThread.ProdottiValutati {
		for _, g := range x.Gruppi(pv.Rif) {
			for _, n := range g.Nodi {
				nodi++
				k := x.ComponenteDelNodo(pv.Rif, g.Chiave, n.Nodo.Rif)
				confermato := n.OrigineTipo == valutazione.OrigineTipoConfermato
				if (k != nil) != confermato {
					t.Errorf("%s, %s, %s: ComponenteDelNodo %v, ma valutazione dà il tipo %s/%s (la copia di T-B4-38 non è più quella del core)",
						pv.Rif, g.Chiave, n.Nodo.Rif, k, n.Tipo, n.OrigineTipo)
					continue
				}
				if k != nil && n.Tipo != tipoDi[*k] {
					t.Errorf("%s, %s, %s: tipo %q, quello del componente %s è %q", pv.Rif, g.Chiave, n.Nodo.Rif, n.Tipo, k, tipoDi[*k])
				}
				if k != nil && n.Nodo.Decisione == nil {
					dallaRadice++
				}
			}
		}
	}
	t.Logf("T-B4-38: %d nodi confrontati, %d con il componente dalla regola della radice", nodi, dallaRadice)
	return nodi, dallaRadice
}

// IX-03: la regola della radice sull'esito vero. La scena con la radice dello STEP marcata (il gesto 3) ma senza la sua
// riga decisa (stato aperta, nessun componente_id): la radice della BOM di lavoro prende il componente del target
// (T-B4-38), e ComponenteDelNodo deve dire lo stesso di valutazione.
func TestL4MotoreALaRadiceSenzaDecisione(t *testing.T) {
	p, s, r := preparaMA(t)
	if _, err := p.Exec(context.Background(), `UPDATE componente_proposta SET stato = 'aperta', componente_id = NULL, deciso_da = NULL,
		deciso_il = NULL WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#1'`, s.thread, s.aStep); err != nil {
		t.Fatalf("scena: la radice senza la sua riga decisa: %v", err)
	}
	ris, err := (Servizio{Regole: r, Pool: p}).Calcola(context.Background(), s.thread)
	if err != nil {
		t.Fatalf("Calcola: %v", err)
	}
	x := NuovoIndice(ris.Thread, ris.EsitoThread, &ris.Confronto, &ris.Dichiarazioni)
	nodi, dallaRadice := maGemellaTB438(t, ris, x)
	if nodi == 0 || dallaRadice == 0 {
		var strutture []string
		for _, st := range ris.EsitoThread.Ancoraggi.Strutture {
			strutture = append(strutture, string(st.Stato))
		}
		t.Errorf("la regola della radice non si esercita: %d nodi, %d dalla regola della radice (strutture %v)", nodi, dallaRadice, strutture)
	}
}

// Lo snapshot unico, dal registro del testo SQL.
func TestL4MotoreAUnoSnapshotSolo(t *testing.T) {
	_, s, r := preparaMA(t)
	pr, reg := testutil.PoolConRegistro(t, testutil.DSN(t))
	reg.Azzera()
	if _, err := (Servizio{Regole: r, Pool: pr}).Calcola(context.Background(), s.thread); err != nil {
		t.Fatalf("Calcola: %v", err)
	}
	voci := reg.Voci()
	if len(voci) < 6 {
		t.Fatalf("%d testi SQL", len(voci))
	}
	norm := func(q string) string { return strings.ToLower(strings.Join(strings.Fields(q), " ")) }
	if !strings.HasPrefix(norm(voci[0].SQL), "begin isolation level repeatable read read only") {
		t.Errorf("il primo testo non è il BEGIN dello snapshot: %q", voci[0].SQL)
	}
	if norm(voci[len(voci)-1].SQL) != "rollback" {
		t.Errorf("l'ultimo testo non è il ROLLBACK: %q", voci[len(voci)-1].SQL)
	}
	begin, savepoint, ritorno, getThread := 0, 0, 0, -1
	var mostra []string
	for i, v := range voci {
		q := norm(v.SQL)
		if v.Connessione != voci[0].Connessione {
			t.Errorf("un testo su un'altra connessione: %q", v.SQL)
		}
		switch {
		case strings.HasPrefix(q, "begin"):
			begin++
		case strings.HasPrefix(q, "savepoint "):
			savepoint++
		case strings.HasPrefix(q, "rollback to savepoint "):
			ritorno++
		case strings.Contains(q, "name: getthread ") && getThread < 0:
			getThread = i
		}
		if i < 8 {
			mostra = append(mostra, q[:min(len(q), 60)])
		}
	}
	if begin != 1 || savepoint != 1 || ritorno != 1 || getThread != 1 {
		t.Errorf("%d BEGIN, %d SAVEPOINT, %d ROLLBACK TO SAVEPOINT, GetThread al testo %d; attesi 1, 1, 1, 1. Primi testi: %q", begin, savepoint, ritorno, getThread, mostra)
	}
	if v := reg.Vietati(); len(v) > 0 {
		t.Errorf("testi che scrivono o prendono lucchetti: %q", v)
	}
	// l'anteprima: una transazione sua, nella stessa forma
	reg.Azzera()
	if _, err := (Servizio{Regole: r, Pool: pr}).AnteprimaSTEP(context.Background(), s.thread,
		fascicolo.RichiestaAutorizzazione{Componente: s.prodotto, Documento: s.docStep}, true); err != nil {
		t.Fatalf("AnteprimaSTEP: %v", err)
	}
	voci = reg.Voci()
	if len(voci) < 3 || !strings.HasPrefix(norm(voci[0].SQL), "begin isolation level repeatable read read only") || norm(voci[len(voci)-1].SQL) != "rollback" {
		t.Errorf("l'anteprima: primo %q, ultimo %q", voci[0].SQL, voci[len(voci)-1].SQL)
	}
	if !strings.Contains(norm(voci[1].SQL), "name: getthread ") {
		t.Errorf("l'anteprima non comincia con GetThread: %q", voci[1].SQL)
	}
	if v := reg.Vietati(); len(v) > 0 {
		t.Errorf("l'anteprima: testi che scrivono o prendono lucchetti: %q", v)
	}
}

// La sola lettura provata davvero (F-20): nella transazione dello snapshot e nel savepoint del caricatore i due SHOW
// dicono «on» e «repeatable read», e una sonda di scrittura riceve SQLSTATE 25006.
func TestL4MotoreALaSondaDiScrittura(t *testing.T) {
	p, _, _ := preparaMA(t)
	ctx := context.Background()
	type lettoreScrittore interface {
		QueryRow(context.Context, string, ...any) pgx.Row
		Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	}
	prova := func(nome string, q lettoreScrittore) {
		t.Helper()
		var ro, iso string
		if err := q.QueryRow(ctx, "SHOW transaction_read_only").Scan(&ro); err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		if err := q.QueryRow(ctx, "SHOW transaction_isolation").Scan(&iso); err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		if ro != "on" || iso != "repeatable read" {
			t.Errorf("%s: transaction_read_only %q, transaction_isolation %q", nome, ro, iso)
		}
		_, err := q.Exec(ctx, "CREATE TEMP TABLE sonda_a1d (x int)")
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != "25006" {
			t.Errorf("%s: la sonda di scrittura ha dato %v, atteso SQLSTATE 25006", nome, err)
		}
	}
	// la transazione dello snapshot (dopo l'errore della sonda è abortita: il savepoint si prova in una nuova)
	if err := nelloSnapshot(ctx, p, func(tx pgx.Tx) error { prova("snapshot", tx); return nil }); err != nil {
		t.Fatal(err)
	}
	// il savepoint che riceve il caricatore
	if err := nelloSnapshot(ctx, p, func(tx pgx.Tx) error {
		sp, err := savepoint{tx}.BeginTx(ctx, opzioniSnapshot)
		if err != nil {
			return err
		}
		prova("savepoint", sp)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if n := testutil.Conta(t, p, "componente"); n != 2 {
		t.Errorf("%d componenti dopo le sonde, attesi 2", n)
	}
}

func TestL4MotoreARFQInesistente(t *testing.T) {
	p, s, r := preparaMA(t)
	nessuna := uuid.MustParse("00000000-0000-4000-8000-00000000a1d0")
	svc := Servizio{Regole: r, Pool: p}
	if _, err := svc.Calcola(context.Background(), nessuna); !errors.Is(err, ErrRFQInesistente) {
		t.Errorf("Calcola: %v", err)
	}
	if err := svc.Esiste(context.Background(), nessuna); !errors.Is(err, ErrRFQInesistente) {
		t.Errorf("Esiste: %v", err)
	}
	if err := svc.Esiste(context.Background(), s.thread); err != nil {
		t.Errorf("Esiste sulla RFQ della scena: %v", err)
	}
	if _, err := svc.AnteprimaSTEP(context.Background(), nessuna, fascicolo.RichiestaAutorizzazione{Componente: s.prodotto, Documento: s.docStep}, true); !errors.Is(err, ErrRFQInesistente) {
		t.Errorf("AnteprimaSTEP: %v", err)
	}
}

// L'anteprima dello STEP: la stessa firma e gli stessi effetti di fascicolo.EffettoAutorizzazione sul pool (il gestore
// legacy), in sola lettura.
func TestL4MotoreAAnteprimaComeIlLegacy(t *testing.T) {
	p, s, r := preparaMA(t)
	prima := testutil.FotoDelDatabase(t, p)
	req := fascicolo.RichiestaAutorizzazione{Componente: s.prodotto, Documento: s.docStep}
	a, err := (Servizio{Regole: r, Pool: p}).AnteprimaSTEP(context.Background(), s.thread, req, true)
	if err != nil {
		t.Fatalf("AnteprimaSTEP: %v", err)
	}
	e, err := fascicolo.EffettoAutorizzazione(context.Background(), db.New(p), s.thread, req)
	if err != nil {
		t.Fatal(err)
	}
	if a.Firma == "" || a.Firma != e.Firma || a.Motore != MotoreAnteprima {
		t.Errorf("firma %q, quella del legacy %q, motore %q", a.Firma, e.Firma, a.Motore)
	}
	if atteso := AnteprimaDa(e, true); !jsonUguale(t, a, atteso) {
		t.Errorf("l'anteprima non è quella dell'effetto legacy:\n %+v\n %+v", a, atteso)
	}
	t.Logf("anteprima: gia %q, spento %q, frasi %q", a.Gia, a.Spento, a.Frasi)
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("l'anteprima ha cambiato il database: %v", d)
	}
	// le regole del motore A non servono all'anteprima
	if _, err := (Servizio{Pool: p}).AnteprimaSTEP(context.Background(), s.thread, req, false); err != nil {
		t.Errorf("anteprima senza regole: %v", err)
	}
}

func jsonUguale(t *testing.T, a, b any) bool {
	t.Helper()
	x, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	y, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Equal(x, y)
}

// Il tempo scaduto sul database vero, con il limite già scaduto all'apertura: un limite di un nanosecondo dà ErrTempo,
// senza scritture. Questa prova non legge né calcola (pgx rifiuta già l'apertura della transazione): il limite che scade
// a metà di una lettura lo prova TestL4MotoreATempoScadutoDuranteLaLettura.
func TestL4MotoreATempoScaduto(t *testing.T) {
	p, s, r := preparaMA(t)
	prima := testutil.FotoDelDatabase(t, p)
	r.Limite = time.Nanosecond
	svc := Servizio{Regole: r, Pool: p, posti: make(chan struct{}, MassimoCalcoli)}
	if _, err := svc.Calcola(context.Background(), s.thread); !errors.Is(err, ErrTempo) {
		t.Errorf("con un limite di 1 ns: %v, atteso ErrTempo", err)
	}
	maAspettaPostiLiberi(t, svc.posti) // il calcolo orfano finisce
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("il calcolo scaduto ha cambiato il database: %v", d)
	}
}

// maIniziatoreLento: apre la transazione vera e la dà avvolta in maTxLenta, che ferma la prima lettura (GetThread)
// finché il limite non scade.
type maIniziatoreLento struct{ p Iniziatore }

func (i maIniziatoreLento) BeginTx(ctx context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	tx, err := i.p.BeginTx(ctx, o)
	if err != nil {
		return nil, err
	}
	return &maTxLenta{Tx: tx}, nil
}

type maTxLenta struct {
	pgx.Tx
	ferma bool
}

func (x *maTxLenta) QueryRow(ctx context.Context, sql string, a ...any) pgx.Row {
	if !x.ferma {
		x.ferma = true
		<-ctx.Done()
	}
	return x.Tx.QueryRow(ctx, sql, a...)
}

// R-03, Q2-05, R-04 della revisione di A1d-P2: il limite scade DOPO l'apertura della transazione vera, durante la prima
// lettura. Calcola dà ErrTempo; il calcolo orfano finisce; la transazione si chiude con il ROLLBACK sulla stessa
// connessione (il ROLLBACK ha un contesto suo, staccato dalla richiesta scaduta), la connessione torna al pool (nessuna
// presa), un secondo calcolo sullo stesso pool riesce, e il database non cambia.
func TestL4MotoreATempoScadutoDuranteLaLettura(t *testing.T) {
	p, s, r := preparaMA(t)
	prima := testutil.FotoDelDatabase(t, p)
	pr, reg := testutil.PoolConRegistro(t, testutil.DSN(t))
	reg.Azzera()
	r.Limite = 500 * time.Millisecond
	svc := Servizio{Regole: r, Pool: maIniziatoreLento{p: pr}, posti: make(chan struct{}, MassimoCalcoli)}
	if _, err := svc.Calcola(context.Background(), s.thread); !errors.Is(err, ErrTempo) {
		t.Fatalf("con la lettura ferma oltre il limite: %v, atteso ErrTempo", err)
	}
	maAspettaPostiLiberi(t, svc.posti)
	voci := reg.Voci()
	norm := func(q string) string { return strings.ToLower(strings.Join(strings.Fields(q), " ")) }
	if len(voci) < 2 || !strings.HasPrefix(norm(voci[0].SQL), "begin isolation level repeatable read read only") ||
		norm(voci[len(voci)-1].SQL) != "rollback" || voci[len(voci)-1].Connessione != voci[0].Connessione {
		var testi []string
		for _, v := range voci {
			testi = append(testi, norm(v.SQL)[:min(len(norm(v.SQL)), 60)])
		}
		t.Errorf("la transazione interrotta dal limite: testi %q; attesi il BEGIN dello snapshot e, sulla stessa connessione, il ROLLBACK", testi)
	}
	if v := reg.Vietati(); len(v) > 0 {
		t.Errorf("testi che scrivono o prendono lucchetti: %q", v)
	}
	fine := time.Now().Add(2 * time.Second)
	for pr.Stat().AcquiredConns() != 0 && time.Now().Before(fine) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := pr.Stat().AcquiredConns(); n != 0 {
		t.Errorf("%d connessioni ancora prese dopo il tempo scaduto", n)
	}
	r.Limite = 20 * time.Second
	if _, err := (Servizio{Regole: r, Pool: pr, posti: make(chan struct{}, MassimoCalcoli)}).Calcola(context.Background(), s.thread); err != nil {
		t.Errorf("il calcolo dopo il tempo scaduto, sullo stesso pool: %v", err)
	}
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("il calcolo scaduto ha cambiato il database: %v", d)
	}
}
