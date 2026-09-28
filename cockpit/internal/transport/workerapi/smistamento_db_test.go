//go:build integrazione

// L4 — Smistamento F8: gli inneschi dei worker del flusso ancorato al prodotto (addendum A5.13.6-A5.13.7). Il
// file sceso in staging (IN3, IN4), l'archivio estratto (IN2), il risultato dell'analisi (IN5), i fatti
// riusati (IN6): ognuno fa girare il flusso DOPO il suo commit, in una transazione sua. Prove 181, 182, 183,
// 185, 187 dell'addendum; nella 183 anche la RFQ senza riferimento strutturale in cui il capitolato riconosciuto
// dal contenuto riceve il «documento generale» e i disegni nessuna destinazione tecnica.

package workerapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
	"promatec/cockpit/internal/platform/testutil"
)

const workerF8 = "analisi-f8"

// bancoFlusso: RFQ di ACME con i loro file, il server dei worker vero dietro httptest, un worker di analisi
// censito.
type bancoFlusso struct {
	t      *testing.T
	ctx    context.Context
	pool   *pgxpool.Pool
	q      *db.Queries
	s      *Server
	srv    *httptest.Server
	an     coda.Analizzatore
	utente uuid.UUID
	n      int
}

func nuovoBancoFlusso(t *testing.T) *bancoFlusso {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{}}
	s := &Server{Pool: p, Log: testutil.LogSilenzioso(), Staging: t.TempDir(), Analizzatore: an}
	mux := http.NewServeMux()
	s.Registra(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	b := &bancoFlusso{t: t, ctx: context.Background(), pool: p, q: db.New(p), s: s, srv: srv, an: an}
	b.utente = b.id(`INSERT INTO utente (sigla, nome, ufficio) VALUES ('F8', 'Prova F8', 'Tecnico') RETURNING utente_id`)
	b.esegui(`INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES (3, $1)`, an.Hash())
	if _, err := b.q.UpsertWorkerCredenziale(b.ctx, db.UpsertWorkerCredenzialeParams{WorkerNome: workerF8, WorkerTipo: db.WorkerTipoAnalisi,
		TokenHash: ImprontaToken(tokenDi(workerF8)), Caselle: []uuid.UUID{}}); err != nil {
		t.Fatal(err)
	}
	return b
}

func (b *bancoFlusso) id(sql string, arg ...any) uuid.UUID {
	b.t.Helper()
	var id uuid.UUID
	if err := b.pool.QueryRow(b.ctx, sql, arg...).Scan(&id); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
	return id
}

func (b *bancoFlusso) esegui(sql string, arg ...any) {
	b.t.Helper()
	if _, err := b.pool.Exec(b.ctx, sql, arg...); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
}

func (b *bancoFlusso) testo(sql string, arg ...any) string {
	b.t.Helper()
	var s string
	if err := b.pool.QueryRow(b.ctx, sql, arg...).Scan(&s); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
	return s
}

// rfq e' una RFQ di ACME con i prodotti dati, codici della richiesta confermati al triage con il loro finito.
func (b *bancoFlusso) rfq(prodotti ...string) uuid.UUID {
	b.t.Helper()
	b.n++
	cliente := b.id(`INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ($1, 'ACME') RETURNING cliente_id`, fmt.Sprintf("ACME%d", b.n))
	th := b.id(`INSERT INTO thread_offerta (cliente_id, canale, data_inizio) VALUES ($1, 'outlook', now()) RETURNING thread_id`, cliente)
	for _, p := range prodotti {
		b.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confidenza, confermato_da) VALUES ($1, $2, 'proposta_famiglia', 80, $3)`, th, p, b.utente)
		b.esegui(`INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, $2, 'finito', 'codice_rilevato', $3)`, th, p, b.utente)
	}
	return th
}

// messaggio e' una mail di ACME agganciata alla RFQ.
func (b *bancoFlusso) messaggio(thread uuid.UUID) uuid.UUID {
	b.t.Helper()
	b.n++
	conv := b.id(`INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', $1, now()) RETURNING conversazione_id`,
		fmt.Sprintf("C-F8-%d", b.n))
	return b.id(`INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, thread_id, mittente_indirizzo)
		VALUES ('outlook', $1, $2, 'entrata', now(), $3, 'ufficio.tecnico@acme.example') RETURNING messaggio_id`, fmt.Sprintf("<f8-%d@acme.example>", b.n), conv, thread)
}

// scende: un file della mail arriva nello staging, con il contenuto sha, e percorre la strada di ogni file
// (DopoCaricamento: la lettura dal nome, l'analisi in coda o i fatti gia' calcolati), con il flusso dopo il
// commit come nel result di un download.
func (b *bancoFlusso) scende(msg uuid.UUID, nome, sha string) uuid.UUID {
	b.t.Helper()
	b.n++
	ext := strings.TrimPrefix(filepath.Ext(nome), ".")
	a := b.id(`INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1, $2, $3, $4, 'file', 'outlook', 100, $5, $6, 'in_staging', now()) RETURNING allegato_id`,
		msg, b.n, nome, ext, sha, `C:\staging\f8\`+sha[:12])
	ctx, raccolta := fascicolo.ConRismistamenti(b.ctx)
	tx, err := b.pool.Begin(ctx)
	if err != nil {
		b.t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := b.s.DopoCaricamento(ctx, db.New(tx), a); err != nil {
		b.t.Fatalf("il file %s non scende: %v", nome, err)
	}
	if err := tx.Commit(ctx); err != nil {
		b.t.Fatal(err)
	}
	raccolta.Esegui(b.ctx, b.s.smistatore(), b.s.Log)
	return a
}

// analizza: il worker di analisi prende l'analisi del contenuto sha e consegna il risultato con la POST vera.
func (b *bancoFlusso) analizza(allegato uuid.UUID, sha, tipo, fonte, dettagli string) int {
	b.t.Helper()
	chiave := coda.ChiaveAnalisi(sha, b.an)
	var jobID int64
	var token uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `UPDATE job SET stato = 'in_corso', lease_token = gen_random_uuid(), avviato_il = now(),
		lease_fino_a = now() + interval '10 minutes', worker_id = $2, tentativi = tentativi + 1
		WHERE chiave_idempotenza = $1 AND stato = 'pronto' RETURNING job_id, lease_token`, chiave, workerF8).Scan(&jobID, &token); err != nil {
		b.t.Fatalf("nessuna analisi in coda per %s: %v", sha[:12], err)
	}
	dati, _ := json.Marshal(worker.RisultatoAnalisi{AllegatoID: allegato, TipoProposto: tipo, Fonte: fonte, Confidenza: 90,
		Dettagli: json.RawMessage(dettagli), VersioneAnalizzatore: b.an.Versione, HashConfigurazione: b.an.Hash()})
	corpo, _ := json.Marshal(worker.RisultatoRichiesta{Esito: "ok", Dati: dati, WorkerID: workerF8, LeaseToken: token.String()})
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/jobs/%d/result", b.srv.URL, jobID), bytes.NewReader(corpo))
	req.Header.Set("X-Cockpit-Token", tokenDi(workerF8))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		b.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// destinazione e' dettagli.destinazione della proposta dell'allegato (nil se non c'e').
func (b *bancoFlusso) destinazione(allegato uuid.UUID) *fascicolo.Destinazione {
	b.t.Helper()
	var raw []byte
	if err := b.pool.QueryRow(b.ctx, `SELECT coalesce(dettagli -> 'destinazione', 'null'::jsonb) FROM documento_proposta WHERE allegato_id = $1`,
		allegato).Scan(&raw); err != nil {
		b.t.Fatal(err)
	}
	var d *fascicolo.Destinazione
	if err := json.Unmarshal(raw, &d); err != nil {
		b.t.Fatal(err)
	}
	return d
}

func candidatiDi(d *fascicolo.Destinazione) string {
	if d == nil {
		return "(nessuna destinazione)"
	}
	var parti []string
	for _, c := range d.Candidati {
		x := fmt.Sprintf("%d %s %s", c.Rango, c.Codice, c.Regola)
		if c.Codice == "" {
			x = fmt.Sprintf("%d %s %s", c.Rango, c.Chiave, c.Regola)
		}
		if c.Bloccato != "" {
			x += " [" + c.Bloccato + "]"
		}
		parti = append(parti, x)
	}
	return d.Esito + ": " + strings.Join(parti, " | ")
}

// strutturaSTEP sono i fatti di uno STEP: nodi "#1=7120001", archi "#1>#2*2"; la radice e' il primo nodo.
func strutturaSTEP(nodi, archi []string) string {
	var n, r []string
	for _, x := range nodi {
		k, testo, _ := strings.Cut(x, "=")
		n = append(n, `{"chiave": "`+k+`", "id_grezzo": "`+testo+`", "nome_grezzo": "`+testo+`", "evidenza": {}}`)
	}
	for _, x := range archi {
		pf, q, _ := strings.Cut(x, "*")
		if q == "" {
			q = "1"
		}
		p, f, _ := strings.Cut(pf, ">")
		r = append(r, `{"padre": "`+p+`", "figlio": "`+f+`", "qta": `+q+`, "evidenza": {}}`)
	}
	radice, _, _ := strings.Cut(nodi[0], "=")
	return `{"struttura": {"versione": 3, "schema": "AP214", "radici": ["` + radice + `"], "avvisi": [], "nodi": [` + strings.Join(n, ", ") +
		`], "relazioni": [` + strings.Join(r, ", ") + `], "limiti": {"troncato": false}, "scarti": {"prodotti_senza_definizione": 0,
		"occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`
}

var stepDelProdotto = strutturaSTEP([]string{"#1=7120001", "#2=7120010", "#3=7120012", "#4=7120011"}, []string{"#1>#2", "#1>#3*2", "#2>#4*2"})

func shaF8(s string) string { return shaDi([]byte("f8:" + s)) }

// fotoFuoriDalFlusso: le righe e le colonne che il flusso non scrive mai. prop: le proposte di documento da
// guardare colonna per colonna (i dettagli senza la destinazione).
func (b *bancoFlusso) fotoFuoriDalFlusso(prop ...uuid.UUID) string {
	b.t.Helper()
	return b.testo(`SELECT concat_ws(' # ',
		(SELECT count(*) FROM componente)::text, (SELECT count(*) FROM componente_relazione)::text, (SELECT count(*) FROM documento)::text,
		(SELECT count(*) FROM documento_provenienza)::text, (SELECT count(*) FROM identificativo_thread)::text,
		(SELECT count(*) FROM documento_proposta)::text,
		(SELECT coalesce(string_agg(concat_ws(':', allegato_id, tipo_proposto, codice, rev, componente_id, confidenza, fonte, stato,
		        (dettagli - 'destinazione')::text), '|' ORDER BY allegato_id), '') FROM documento_proposta WHERE allegato_id = ANY($1)))`, prop)
}

// Prova 182 (A5.13.6, A5.13.9, IN3, IN5): la scena del caso guida. I file scendono (le loro destinazioni
// aspettano lo STEP del prodotto, che si sta analizzando); il risultato dello STEP scrive le destinazioni e
// solo quelle: nessuna riga nuova in componente, componente_relazione, documento, documento_provenienza,
// identificativo_thread, documento_proposta, e le colonne delle proposte degli altri file restano com'erano.
func TestIlRisultatoDelloStepScriveLeDestinazioniESoloQuelle(t *testing.T) {
	b := nuovoBancoFlusso(t)
	th := b.rfq("7120001")
	msg := b.messaggio(th)
	step := b.scende(msg, "7120001A_1.stp", shaF8("step"))
	d10 := b.scende(msg, "7120010.pdf", shaF8("7120010"))
	d11 := b.scende(msg, "7120011_1.pdf", shaF8("7120011"))
	dxf := b.scende(msg, "7120099.dxf", shaF8("dxf"))
	cap := b.scende(msg, "Capitolato fornitura.pdf", shaF8("capitolato"))

	if d := b.destinazione(d10); d == nil || d.Esito != fascicolo.EsitoInAttesa {
		t.Fatalf("prima dello STEP il disegno aspetta: %s", candidatiDi(d))
	}
	prima := b.fotoFuoriDalFlusso(d10, d11, dxf, cap)
	if st := b.analizza(step, shaF8("step"), "cad_3d", "step", stepDelProdotto); st != http.StatusNoContent {
		t.Fatalf("il risultato dello STEP: %d", st)
	}
	if dopo := b.fotoFuoriDalFlusso(d10, d11, dxf, cap); dopo != prima {
		t.Errorf("il flusso ha scritto altro che le destinazioni:\nprima %s\ndopo  %s", prima, dopo)
	}
	casi := map[uuid.UUID]string{
		step: "proposta: 1 7120001 dest_radice_uguale",
		d10:  "proposta: 1 7120010 dest_nodo_non_autorizzato [step_non_autorizzato]",
		d11:  "proposta: 1 7120011 dest_nodo_profondo [nessuna_autorita]",
		// la loro analisi e' ancora in coda: «in elaborazione», non «nessun pezzo»
		dxf: "in_attesa: ",
		cap: "in_attesa: ",
	}
	for a, atteso := range casi {
		if got := candidatiDi(b.destinazione(a)); got != atteso {
			t.Errorf("%s: %s, atteso %s", a, got, atteso)
		}
	}
	if n := b.testo(`SELECT count(*)::text FROM componente_proposta WHERE componente_id IS NOT NULL OR deciso_da IS NOT NULL`); n != "0" {
		t.Errorf("nodi con un'identita' dopo il flusso: %s", n)
	}
}

// Prova 183 (Domanda 4 = B): senza riferimento strutturale i file restano da verificare e si analizzano lo
// stesso: le analisi sono in coda, i disegni sospesi. Il capitolato, riconosciuto dal contenuto quando la
// sua analisi arriva, riceve il «documento generale» preselezionabile; i disegni nessuna destinazione tecnica.
func TestSenzaRiferimentoIFileRestanoDaVerificareEVengonoAnalizzati(t *testing.T) {
	b := nuovoBancoFlusso(t)
	th := b.rfq("7120002")
	msg := b.messaggio(th)
	d2 := b.scende(msg, "7120002.pdf", shaF8("7120002"))
	d10 := b.scende(msg, "7120010.pdf", shaF8("7120010"))
	cap := b.scende(msg, "Capitolato fornitura.pdf", shaF8("capitolato"))
	for _, x := range []struct {
		a   uuid.UUID
		sha string
	}{{d2, shaF8("7120002")}, {d10, shaF8("7120010")}, {cap, shaF8("capitolato")}} {
		if st := b.testo(`SELECT stato::text FROM job WHERE chiave_idempotenza = $1`, coda.ChiaveAnalisi(x.sha, b.an)); st != "pronto" {
			t.Errorf("l'analisi di %s: %s", x.a, st)
		}
	}
	for _, a := range []uuid.UUID{d2, d10} {
		if b.analizza(a, map[uuid.UUID]string{d2: shaF8("7120002"), d10: shaF8("7120010")}[a], "disegno_2d", "cartiglio",
			`{"cartiglio": true, "termini_trovati": ["SCALA"]}`) != http.StatusNoContent {
			t.Fatal("il risultato del disegno")
		}
	}
	if b.analizza(cap, shaF8("capitolato"), "capitolato", "cartiglio", `{"termini_trovati": ["REQUISITI", "FORNITURA"]}`) != http.StatusNoContent {
		t.Fatal("il risultato del capitolato")
	}
	for _, a := range []uuid.UUID{d2, d10} {
		d := b.destinazione(a)
		if d == nil || d.Esito != fascicolo.EsitoSospesa || d.Motivo != fascicolo.MotivoManca || len(d.Candidati) != 0 {
			t.Errorf("il disegno senza riferimento: %s %+v", candidatiDi(d), d)
		}
	}
	if d := b.destinazione(cap); d == nil || candidatiDi(d) != "proposta: 1 generale dest_generale_contenuto" || !d.Preselezionabile {
		t.Errorf("il capitolato dal contenuto: %s", candidatiDi(d))
	}
	if n := b.testo(`SELECT count(*)::text FROM job WHERE tipo = 'analizza_allegato' AND stato = 'fatto'`); n != "3" {
		t.Errorf("analisi fatte: %s", n)
	}
}

// Prova 181 (A5.13.6, IN2): dopo lo ZIP gli STEP compatibili con un prodotto si analizzano prima (priorita' 5
// contro 6); AlzaPrioritaJob porta avanti un'analisi gia' in coda e non tocca un job gia' preso.
func TestDopoLoZipGliStepCompatibiliSiAnalizzanoPrima(t *testing.T) {
	b := nuovoBancoFlusso(t)
	th := b.rfq("7120001")
	msg := b.messaggio(th)
	archivio, sha := zipCon(t, map[string][]byte{
		"STEP/7120001A_1.stp": []byte("ISO-10303-21; prodotto"),
		"PDF/7120010.pdf":     []byte("%PDF disegno 7120010"),
		"STEP/7120555.stp":    []byte("ISO-10303-21; altro"),
	})
	percorso, err := staging.PercorsoContenuto(b.s.Staging, sha, "disegni.zip")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(percorso), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(percorso, archivio, 0o644); err != nil {
		t.Fatal(err)
	}
	zipID := b.id(`INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1, 1, 'disegni.zip', 'zip', 'file', 'outlook', $2, $3, $4, 'in_staging', now()) RETURNING allegato_id`, msg, len(archivio), sha, percorso)
	// l'estrazione accoda da sola le analisi con la priorita' giusta: il flusso dopo il commit (che la
	// alzerebbe comunque) qui non gira, e si guarda il lavoro dell'estrazione
	giri := 0
	b.s.Smistatore = func(ctx context.Context, thread uuid.UUID, a fascicolo.Ambito) error { giri++; return nil }
	if _, err := b.s.EstraiArchivio(b.ctx, zipID, uuid.New()); err != nil {
		t.Fatalf("estrazione: %v", err)
	}
	b.s.Smistatore = nil
	priorita := b.testo(`SELECT string_agg(j.payload ->> 'nome_file' || '=' || j.priorita, ' ' ORDER BY j.payload ->> 'nome_file')
		FROM job j WHERE j.tipo = 'analizza_allegato'`)
	if priorita != "7120001A_1.stp=5 7120010.pdf=6 7120555.stp=6" {
		t.Errorf("le priorita' delle analisi dopo lo ZIP: %s", priorita)
	}
	if giri != 1 {
		t.Errorf("l'estrazione deve far girare il flusso sulla RFQ una volta, dopo il commit: %d", giri)
	}
	// il giro del flusso sulla RFQ: lo STEP del prodotto e' atteso
	if _, err := fascicolo.Rismista(b.ctx, b.s.Pool, th, fascicolo.AmbitoRfq(), b.an); err != nil {
		t.Fatal(err)
	}
	var voce uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT allegato_id FROM allegato WHERE nome_file = '7120010.pdf'`).Scan(&voce); err != nil {
		t.Fatal(err)
	}
	if d := b.destinazione(voce); d == nil || d.Esito != fascicolo.EsitoInAttesa {
		t.Errorf("la voce dello ZIP dopo l'estrazione: %s", candidatiDi(d))
	}

	// un'analisi gia' in coda con la priorita' di sempre, e una gia' presa da un worker
	gia := b.scendeSenzaFlusso(msg, "7120001.stp", shaF8("gia"))
	presa := b.scendeSenzaFlusso(msg, "7120001_v2.stp", shaF8("presa"))
	b.esegui(`UPDATE job SET stato = 'in_corso', lease_token = gen_random_uuid(), avviato_il = now(), lease_fino_a = now() + interval '10 minutes',
		worker_id = $2 WHERE chiave_idempotenza = $1`, coda.ChiaveAnalisi(shaF8("presa"), b.an), workerF8)
	prio := func(sha string) string {
		return b.testo(`SELECT priorita::text FROM job WHERE chiave_idempotenza = $1`, coda.ChiaveAnalisi(sha, b.an))
	}
	if prio(shaF8("gia")) != "6" || prio(shaF8("presa")) != "6" {
		t.Fatalf("le analisi di prima: %s %s", prio(shaF8("gia")), prio(shaF8("presa")))
	}
	if _, err := fascicolo.Rismista(b.ctx, b.s.Pool, th, fascicolo.AmbitoRfq(), b.an); err != nil {
		t.Fatal(err)
	}
	if prio(shaF8("gia")) != "5" || prio(shaF8("presa")) != "6" {
		t.Errorf("AlzaPrioritaJob: pronta %s (attesa 5), presa %s (attesa 6)", prio(shaF8("gia")), prio(shaF8("presa")))
	}
	_, _ = gia, presa
}

// scendeSenzaFlusso: il file arriva nello staging e percorre la strada di sempre (la lettura dal nome,
// l'analisi in coda), ma senza che il flusso giri dopo: un evento di prima del flusso, o perso.
func (b *bancoFlusso) scendeSenzaFlusso(msg uuid.UUID, nome, sha string) uuid.UUID {
	b.t.Helper()
	b.n++
	ext := strings.TrimPrefix(filepath.Ext(nome), ".")
	a := b.id(`INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1, $2, $3, $4, 'file', 'outlook', 100, $5, 'C:\staging\x', 'in_staging', now()) RETURNING allegato_id`, msg, b.n, nome, ext, sha)
	tx, err := b.pool.Begin(b.ctx)
	if err != nil {
		b.t.Fatal(err)
	}
	defer tx.Rollback(b.ctx)
	if err := b.s.DopoCaricamento(b.ctx, db.New(tx), a); err != nil {
		b.t.Fatal(err)
	}
	if err := tx.Commit(b.ctx); err != nil {
		b.t.Fatal(err)
	}
	return a
}

// Prova 185 (A5.13.7): il fan-out usa i prodotti di ogni RFQ. Lo stesso disegno in due RFQ di ACME: in quella
// il cui prodotto ha lo STEP e' un figlio diretto (bloccato finche' lo STEP non e' autorizzato), nell'altra e'
// sospeso. Una RFQ unita a un'altra e una chiusa non ricevono niente.
func TestIlFanOutUsaIProdottiDiOgniRfq(t *testing.T) {
	b := nuovoBancoFlusso(t)
	conStep, senza, unita, chiusa := b.rfq("7120001"), b.rfq("7120002"), b.rfq("7120001"), b.rfq("7120001")
	b.esegui(`UPDATE thread_offerta SET unito_in = $2 WHERE thread_id = $1`, unita, conStep)
	b.esegui(`UPDATE thread_offerta SET stato = 'CHIUSA' WHERE thread_id = $1`, chiusa)
	m1 := b.messaggio(conStep)
	step := b.scende(m1, "7120001.stp", shaF8("step"))
	if b.analizza(step, shaF8("step"), "cad_3d", "step", stepDelProdotto) != http.StatusNoContent {
		t.Fatal("lo STEP")
	}
	sha := shaF8("disegno condiviso")
	var copie []uuid.UUID
	for _, th := range []uuid.UUID{conStep, senza, unita, chiusa} {
		copie = append(copie, b.scendeSenzaFlusso(b.messaggio(th), "7120010.pdf", sha))
	}
	// il disegno e' analizzato una volta sola (la chiave e' del contenuto) per la prima copia
	if b.analizza(copie[0], sha, "disegno_2d", "cartiglio", `{"cartiglio": true, "termini_trovati": ["SCALA"]}`) != http.StatusNoContent {
		t.Fatal("il risultato del disegno")
	}
	if got := candidatiDi(b.destinazione(copie[0])); got != "proposta: 1 7120010 dest_nodo_non_autorizzato [step_non_autorizzato]" {
		t.Errorf("nella RFQ con lo STEP: %s", got)
	}
	if d := b.destinazione(copie[1]); d == nil || d.Esito != fascicolo.EsitoSospesa || strings.Join(d.ProdottiSenzaAncora, " ") != "7120002" {
		t.Errorf("nella RFQ senza STEP: %s", candidatiDi(d))
	}
	for i, nome := range map[int]string{2: "unita", 3: "chiusa"} {
		if d := b.destinazione(copie[i]); d != nil {
			t.Errorf("la RFQ %s ha ricevuto una destinazione: %s", nome, candidatiDi(d))
		}
	}
}

// Prova 185, il fan-out della struttura: lo STEP analizzato per una RFQ e' anche in un'altra, dove la sua
// proposta di documento e' gia' decisa (niente lettura da riscrivere la'). La struttura entra comunque come
// proposte nell'altra RFQ, e l'altra RFQ rifa' le sue destinazioni: il disegno che aspettava lo STEP trova il
// figlio.
func TestLaStrutturaInUnAltraRfqNeRifaLeDestinazioni(t *testing.T) {
	b := nuovoBancoFlusso(t)
	prima, altra := b.rfq("7120001"), b.rfq("7120001")
	sha := shaF8("step condiviso")
	step := b.scende(b.messaggio(prima), "7120001.stp", sha)
	mAltra := b.messaggio(altra)
	copia := b.scendeSenzaFlusso(mAltra, "7120001.stp", sha)
	b.esegui(`UPDATE documento_proposta SET stato = 'confermata', deciso_da = $2, deciso_il = now() WHERE allegato_id = $1`, copia, b.utente)
	disegno := b.scende(mAltra, "7120010.pdf", shaF8("disegno altra"))
	if d := b.destinazione(disegno); d == nil || d.Esito != fascicolo.EsitoInAttesa {
		t.Fatalf("nell'altra RFQ il disegno aspetta lo STEP: %s", candidatiDi(d))
	}
	if b.analizza(step, sha, "cad_3d", "step", stepDelProdotto) != http.StatusNoContent {
		t.Fatal("lo STEP")
	}
	if got := candidatiDi(b.destinazione(disegno)); got != "proposta: 1 7120010 dest_nodo_non_autorizzato [step_non_autorizzato]" {
		t.Errorf("nell'altra RFQ, dopo lo STEP: %s", got)
	}
}

// Prova 187 (P31, FP6): un errore del flusso non fa fallire l'analisi. Il flusso forzato in errore: il
// risultato risponde 204, il job e' fatto, i fatti e la lettura sono scritti, la destinazione no, e il log
// dice perche'.
func TestUnErroreDelFlussoNonFaFallireLAnalisi(t *testing.T) {
	b := nuovoBancoFlusso(t)
	var log bytes.Buffer
	b.s.Log = slog.New(slog.NewTextHandler(&log, nil))
	chiamate := 0
	b.s.Smistatore = func(ctx context.Context, thread uuid.UUID, a fascicolo.Ambito) error {
		chiamate++
		return errors.New("flusso guasto per prova")
	}
	th := b.rfq("7120001")
	msg := b.messaggio(th)
	step := b.scende(msg, "7120001.stp", shaF8("step"))
	if st := b.analizza(step, shaF8("step"), "cad_3d", "step", stepDelProdotto); st != http.StatusNoContent {
		t.Fatalf("con il flusso guasto il risultato risponde %d", st)
	}
	if st := b.testo(`SELECT stato::text FROM job WHERE chiave_idempotenza = $1`, coda.ChiaveAnalisi(shaF8("step"), b.an)); st != "fatto" {
		t.Errorf("il job dell'analisi: %s", st)
	}
	if n := b.testo(`SELECT count(*)::text FROM analisi_fatti WHERE sha256 = $1`, shaF8("step")); n != "1" {
		t.Errorf("i fatti: %s", n)
	}
	if v := b.testo(`SELECT coalesce(dettagli -> 'valutazione' ->> 'da', '') FROM documento_proposta WHERE allegato_id = $1`, step); v == "" {
		t.Error("la lettura dell'analisi non e' scritta")
	}
	if n := b.testo(`SELECT count(*)::text FROM componente_proposta WHERE thread_id = $1`, th); n != "4" {
		t.Errorf("la struttura dello STEP: %s righe", n)
	}
	if d := b.destinazione(step); d != nil {
		t.Errorf("il flusso guasto ha scritto una destinazione: %s", candidatiDi(d))
	}
	if chiamate == 0 || !strings.Contains(log.String(), "flusso guasto per prova") || !strings.Contains(log.String(), "destinazioni non aggiornate") {
		t.Errorf("il log non dice l'errore del flusso (%d chiamate): %s", chiamate, log.String())
	}
}
