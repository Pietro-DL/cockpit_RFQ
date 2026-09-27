//go:build integrazione

// L4 — Smistamento F4: le letture di un file per dimensione, contro PostgreSQL vero. Ingest, stage, archivio,
// analisi e fatti esistenti scrivono tutti `dettagli.valutazione` con Valuta, e le colonne sono il suo
// riepilogo: la lettura dal nome non si perde quando arriva l'analisi (230), il «cartiglio» che era il nome si
// dice nome (76), il codice del nome di un file non passa a un'altra copia dello stesso contenuto (122, K9), il
// rumore non tocca i file tecnici (118, parte di F4), e una lettura nuova conserva la destinazione e la storia
// e aggiorna la valutazione anche sulle righe decise dall'operatore (190, 215 parte, 231), e il gesto «e' la
// risposta del fornitore» resta un'evidenza del tipo a ogni rilettura del file (A5.14.7).

package workerapi

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/inbox/ingest"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// bancoLetture e' un server con lo staging in una cartella temporanea e un analizzatore alla versione 1.
type bancoLetture struct {
	t    *testing.T
	pool *pgxpool.Pool
	q    *db.Queries
	ctx  context.Context
	s    *Server
	n    int
}

func nuovoBancoLetture(t *testing.T) *bancoLetture {
	t.Helper()
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	s := &Server{Pool: pool, Log: testutil.LogSilenzioso(), Analizzatore: coda.Analizzatore{Versione: 1}, Staging: t.TempDir()}
	return &bancoLetture{t: t, pool: pool, q: db.New(pool), ctx: context.Background(), s: s}
}

func (b *bancoLetture) id(sql string, arg ...any) uuid.UUID {
	b.t.Helper()
	var id uuid.UUID
	if err := b.pool.QueryRow(b.ctx, sql, arg...).Scan(&id); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
	return id
}

func (b *bancoLetture) esegui(sql string, arg ...any) {
	b.t.Helper()
	if _, err := b.pool.Exec(b.ctx, sql, arg...); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
}

func (b *bancoLetture) testo(sql string, arg ...any) string {
	b.t.Helper()
	var s string
	if err := b.pool.QueryRow(b.ctx, sql, arg...).Scan(&s); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
	return s
}

// rfq e' una RFQ del cliente ACME, con le regole date.
func (b *bancoLetture) rfq(regole string) uuid.UUID {
	b.t.Helper()
	cliente := b.id(`INSERT INTO cliente (cartella_nas, ragione_sociale, regole) VALUES ('ACME', 'ACME', $1) RETURNING cliente_id`, regole)
	return b.id(`INSERT INTO thread_offerta (cliente_id, canale, data_inizio) VALUES ($1, 'outlook', now()) RETURNING thread_id`, cliente)
}

// mail e' un messaggio in entrata da mario.rossi@acme.example, nella RFQ thread (o in nessuna).
func (b *bancoLetture) mail(thread uuid.UUID) uuid.UUID {
	b.t.Helper()
	b.n++
	conv := b.id(`INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', $1, now()) RETURNING conversazione_id`,
		fmt.Sprintf("CONV-F4-%d", b.n))
	return b.id(`INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto, mittente_indirizzo, thread_id)
		VALUES ('outlook', $1, $2, 'entrata', now(), 'RFQ 7120001', 'mario.rossi@acme.example', $3) RETURNING messaggio_id`,
		fmt.Sprintf("<f4-%d@acme.example>", b.n), conv, uuid.NullUUID{UUID: thread, Valid: thread != uuid.Nil})
}

// allegato e' un file sceso in staging, con il contenuto sha.
func (b *bancoLetture) allegato(msg uuid.UUID, nome, sha string, bytes int64) db.Allegato {
	b.t.Helper()
	b.n++
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(nome), "."))
	id := b.id(`INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1, $2, $3, $4, 'file', 'outlook', $5, $6, $7, 'in_staging', now()) RETURNING allegato_id`,
		msg, b.n, nome, ext, bytes, sha, `C:\staging\f4\`+sha+"."+ext)
	a, err := b.q.GetAllegato(b.ctx, id)
	if err != nil {
		b.t.Fatal(err)
	}
	return a
}

// scende e' il result dello stage: la proposta dal nome, il rumore, l'analisi o i fatti che ci sono gia'.
func (b *bancoLetture) scende(a db.Allegato) {
	b.t.Helper()
	if err := b.s.dopoStaging(b.ctx, b.q, worker.RisultatoStage{AllegatoID: a.AllegatoID, Sha256: a.Sha256.String, Bytes: a.Bytes.Int64}); err != nil {
		b.t.Fatalf("dopo lo staging di %s: %v", a.NomeFile, err)
	}
}

// analisi e' il result del worker di analisi per l'allegato prima (il job e' il suo).
func (b *bancoLetture) analisi(prima db.Allegato, r worker.RisultatoAnalisi) {
	b.t.Helper()
	an := b.s.Analizzatore
	r.AllegatoID, r.VersioneAnalizzatore, r.HashConfigurazione = prima.AllegatoID, an.Versione, an.Hash()
	payload, _ := json.Marshal(worker.PayloadAnalizzaAllegato{AllegatoID: prima.AllegatoID, Bytes: prima.Bytes.Int64, Sha256: prima.Sha256.String,
		NomeFile: prima.NomeFile, VersioneAnalizzatore: an.Versione, HashConfigurazione: an.Hash()})
	var jobID int64
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO job (tipo, worker_tipo, payload, lease_s, durata_max_s, stato)
		VALUES ('analizza_allegato', 'analisi', $1, 120, 600, 'in_corso') RETURNING job_id`, payload).Scan(&jobID); err != nil {
		b.t.Fatal(err)
	}
	j, err := b.q.GetJob(b.ctx, jobID)
	if err != nil {
		b.t.Fatal(err)
	}
	dati, _ := json.Marshal(r)
	tx, err := b.pool.Begin(b.ctx)
	if err != nil {
		b.t.Fatal(err)
	}
	defer tx.Rollback(b.ctx)
	if err := b.s.applicaRisultato(b.ctx, db.New(tx), &j, dati, nil); err != nil {
		b.t.Fatalf("il risultato dell'analisi non si applica: %v", err)
	}
	if err := tx.Commit(b.ctx); err != nil {
		b.t.Fatal(err)
	}
}

// lettura: le colonne e la valutazione della proposta dell'allegato.
type lettura struct {
	tipo, codice, rev, fonte string
	conf                     int
	v                        classificazione.Valutazione
	dettagli                 map[string]json.RawMessage
}

func (b *bancoLetture) lettura(allegato uuid.UUID) lettura {
	b.t.Helper()
	var l lettura
	var dett []byte
	if err := b.pool.QueryRow(b.ctx, `SELECT tipo_proposto::text, coalesce(codice, ''), coalesce(rev, ''), fonte::text, confidenza, dettagli
		FROM documento_proposta WHERE allegato_id = $1`, allegato).Scan(&l.tipo, &l.codice, &l.rev, &l.fonte, &l.conf, &dett); err != nil {
		b.t.Fatalf("proposta dell'allegato %s: %v", allegato, err)
	}
	var ok bool
	if l.v, ok = classificazione.LeggiValutazione(dett); !ok {
		b.t.Fatalf("la proposta non ha la valutazione: %s", dett)
	}
	_ = json.Unmarshal(dett, &l.dettagli)
	return l
}

func (l lettura) colonne() string {
	return fmt.Sprintf("%s:%s:%s:%d:%s", l.tipo, l.codice, l.rev, l.conf, l.fonte)
}

// laRiassume: le colonne sono il riepilogo della valutazione.
func (l lettura) laRiassume() bool {
	r := l.v.Riepilogo()
	return r.Tipo == l.tipo && r.Codice == l.codice && r.Rev == l.rev && r.Confidenza == l.conf && r.Fonte == l.fonte
}

// TestLaLetturaDalNomeNonSiPerdeDopoLAnalisi (Smistamento, prova 230): lo stesso file dall'ingest allo stage
// all'analisi. A ogni passo c'e' la valutazione, le colonne sono il suo riepilogo, e la lettura dal nome resta
// con lo score del nome: prima l'analisi la sostituiva con «step 95» e la lettura dal nome spariva.
func TestLaLetturaDalNomeNonSiPerdeDopoLAnalisi(t *testing.T) {
	b := nuovoBancoLetture(t)
	casella, err := b.q.UpsertCasella(b.ctx, db.UpsertCasellaParams{Canale: db.CanaleOutlook, Indirizzo: "commerciale@azienda.example", Nome: "Commerciale"})
	if err != nil {
		t.Fatal(err)
	}
	quando := time.Date(2026, 9, 20, 8, 0, 0, 0, time.UTC)
	servizio := &ingest.Servizio{Pool: b.pool, Log: testutil.LogSilenzioso()}
	if ris, err := servizio.Ingerisci(b.ctx, ingest.Lotto{Casella: casella, Messaggi: []worker.MessaggioIn{{
		MessageID: "<f4-230@acme.example>", EntryID: "ENTRY-F4-230", ConversationID: "CONV-F4-230", Cartella: "Inbox", Direzione: "entrata",
		DataEvento: quando, RicevutoIl: &quando, MittenteIndirizzo: "mario.rossi@acme.example", Oggetto: "RFQ 7120001",
		CorpoTesto: "In allegato il 3D.", Riferimenti: []string{}, Categorie: []string{},
		Allegati: []worker.AllegatoIn{{Indice: 1, NomeFile: "7120001A_1.stp", Estensione: "stp", Natura: "file", Bytes: 2_000_000}},
	}}}); err != nil || ris.Falliti != 0 || ris.Inseriti != 1 {
		t.Fatalf("ingest: %+v %v", ris, err)
	}
	id := b.id(`SELECT allegato_id FROM allegato WHERE nome_file = '7120001A_1.stp'`)

	passi := []struct {
		nome, da, colonne string
	}{{"ingest", classificazione.DaIngest, "cad_3d:7120001A:1:45:nome_file"}}
	controlla := func(passo, da, colonne string) lettura {
		t.Helper()
		l := b.lettura(id)
		if l.v.Da != da || l.colonne() != colonne || !l.laRiassume() {
			t.Errorf("%s: da %q, colonne %s (attese %s), riassume %v", passo, l.v.Da, l.colonne(), colonne, l.laRiassume())
		}
		var nome *classificazione.Evidenza
		for i, e := range l.v.Codice.Evidenze {
			if e.Regola == "nome_codice_generico" {
				nome = &l.v.Codice.Evidenze[i]
			}
		}
		if nome == nil || nome.Valore != "7120001A" || nome.Score != 45 || nome.Testo != "7120001A_1.stp" {
			t.Errorf("%s: la lettura dal nome non c'e' piu': %+v", passo, l.v.Codice.Evidenze)
		}
		return l
	}
	controlla(passi[0].nome, passi[0].da, passi[0].colonne)

	// lo stage: il file scende
	contenuto := []byte("ISO-10303-21; 7120001A")
	sha := shaDi(contenuto)
	b.esegui(`UPDATE allegato SET sha256 = $2, path_staging = $3, stato = 'in_staging' WHERE allegato_id = $1`, id, sha, `C:\staging\f4\230.stp`)
	a, err := b.q.GetAllegato(b.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	b.scende(a)
	controlla("stage", classificazione.DaStage, "cad_3d:7120001A:1:45:nome_file")

	// l'analisi: il worker dice «step 95» con il PRODUCT uguale al nome; il PRODUCT dipende dal nome
	b.analisi(a, worker.RisultatoAnalisi{TipoProposto: "cad_3d", Codice: "7120001A", Rev: "1", Confidenza: 95, Fonte: "step",
		Dettagli: json.RawMessage(`{"product_step": "7120001A_1", "struttura": {"versione": 3, "radici": ["#1"], "nodi": [{"chiave": "#1", "id_grezzo": "7120001A_1"}], "relazioni": []}}`)})
	l := controlla("analisi", classificazione.DaAnalisi, "cad_3d:7120001A:1:45:nome_file")
	if l.v.Tipo.Stato != classificazione.StatoConcorde || l.v.Codice.Stato != classificazione.StatoUnica || len(l.v.Codice.Evidenze) != 2 {
		t.Errorf("dopo l'analisi: tipo %s (due fonti), codice %s con %d evidenze", l.v.Tipo.Stato, l.v.Codice.Stato, len(l.v.Codice.Evidenze))
	}
	if l.dettagli["product_step"] == nil || l.dettagli["struttura"] == nil {
		t.Errorf("i dettagli del worker restano accanto: %v", l.dettagli)
	}
}

// TestIlCartiglioCheEIlNomeSiDiceNome (Smistamento, prova 76, P16): il worker dice «cartiglio 95» per un
// codice che e' il nome del file. In colonna la fonte e' `nome_file` con lo score del nome, e i «Codici visti»
// della RFQ lo dicono «nome del file», non «cartiglio di».
func TestIlCartiglioCheEIlNomeSiDiceNome(t *testing.T) {
	b := nuovoBancoLetture(t)
	thread := b.rfq(`{}`)
	a := b.allegato(b.mail(thread), "7120010.pdf", strings.Repeat("7", 64), 300_000)
	b.analisi(a, worker.RisultatoAnalisi{TipoProposto: "disegno_2d", Codice: "7120010", Confidenza: 95, Fonte: "cartiglio",
		Dettagli: json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA", "TOLLERANZE GENERALI"]}`)})
	l := b.lettura(a.AllegatoID)
	if l.colonne() != "disegno_2d:7120010::45:nome_file" || len(l.v.Codice.Evidenze) != 1 || l.v.Codice.Evidenze[0].Fonte != "nome_file" {
		t.Errorf("colonne %s, evidenze del codice %+v", l.colonne(), l.v.Codice.Evidenze)
	}
	if l.v.Tipo.Regola != "pdf_termini_cartiglio" || l.v.Tipo.Score != 75 {
		t.Errorf("il tipo dal testo: %+v", l.v.Tipo)
	}
	righe, err := b.q.ListCodiciCandidatiThread(b.ctx, uuid.NullUUID{UUID: thread, Valid: true})
	if err != nil {
		t.Fatal(err)
	}
	k, ok := fascicolo.Unisci(righe, fascicolo.ContestoCodici{}).Trova("7120010")
	if !ok || len(k.Evidenze) != 1 {
		t.Fatalf("7120010 fra i codici visti: %+v %v", k, ok)
	}
	if f := k.Evidenze[0].Frase; !strings.HasPrefix(f, "nome del file 7120010.pdf") || strings.Contains(f, "cartiglio") {
		t.Errorf("il codice visto si dice %q", f)
	}
	if k.Punteggio != 45 {
		t.Errorf("score del codice visto: %d, atteso 45", k.Punteggio)
	}
}

// TestIlCodiceDelNomeNonPassaAUnAltroFile (Smistamento, prova 122, E03, K9, P15): lo stesso contenuto con
// tre nomi. Il worker legge il nome della copia che ha analizzato; le altre copie ricevono il tipo (e' del
// contenuto) e rileggono il LORO nome: nel result dell'analisi (propostaDaAnalisi), in una copia che scende
// dopo (applicaFattiEsistenti) e in una voce di un archivio (l'estrazione).
func TestIlCodiceDelNomeNonPassaAUnAltroFile(t *testing.T) {
	b := nuovoBancoLetture(t)
	contenuto := []byte("%PDF-1.4 disegno 7120001A")
	sha := shaDi(contenuto)
	prima := b.allegato(b.mail(uuid.Nil), "7120001A_1.pdf", sha, int64(len(contenuto)))
	altra := b.allegato(b.mail(uuid.Nil), "tavola.pdf", sha, int64(len(contenuto)))
	for _, a := range []db.Allegato{prima, altra} {
		b.esegui(`INSERT INTO documento_proposta (allegato_id, tipo_proposto, confidenza, fonte) VALUES ($1, 'da_determinare', 40, 'estensione')`, a.AllegatoID)
	}
	b.analisi(prima, worker.RisultatoAnalisi{TipoProposto: "disegno_2d", Codice: "7120001A", Rev: "1", Confidenza: 95, Fonte: "cartiglio",
		Dettagli: json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA"]}`)})
	if got := b.lettura(prima.AllegatoID).colonne(); got != "disegno_2d:7120001A:1:45:nome_file" {
		t.Errorf("la copia analizzata: %s", got)
	}
	if l := b.lettura(altra.AllegatoID); l.colonne() != "disegno_2d:::75:cartiglio" || l.v.Codice.Stato != classificazione.StatoNessuna || l.v.Da != classificazione.DaAnalisi {
		t.Errorf("l'altra copia nel result: %s, codice %+v", l.colonne(), l.v.Codice)
	}

	// una copia che scende dopo: i fatti ci sono gia', e il nome e' il suo
	dopo := b.allegato(b.mail(uuid.Nil), "7120099_B.pdf", sha, int64(len(contenuto)))
	b.scende(dopo)
	if l := b.lettura(dopo.AllegatoID); l.colonne() != "disegno_2d:7120099:B:45:nome_file" || l.v.Da != classificazione.DaFattiEsistenti {
		t.Errorf("la copia arrivata dopo: %s (da %s)", l.colonne(), l.v.Da)
	}

	// una voce di un archivio con lo stesso contenuto: il nome della voce
	zipPath := filepath.Join(t.TempDir(), "disegni.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	fw, err := w.Create("cartella/7120055.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(contenuto); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	msgZip := b.mail(uuid.Nil)
	zipID := b.id(`INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1, 1, 'disegni.zip', 'zip', 'file', 'outlook', 1000, $2, $3, 'in_staging', now()) RETURNING allegato_id`, msgZip, strings.Repeat("e", 64), zipPath)
	if n, err := b.s.EstraiArchivio(b.ctx, zipID, uuid.New()); err != nil || n != 1 {
		t.Fatalf("estrazione: %d voci, %v", n, err)
	}
	voce := b.id(`SELECT allegato_id FROM allegato WHERE contenitore_id = $1`, zipID)
	if l := b.lettura(voce); l.colonne() != "disegno_2d:7120055::45:nome_file" || l.v.Da != classificazione.DaFattiEsistenti {
		t.Errorf("la voce dell'archivio: %s (da %s)", l.colonne(), l.v.Da)
	}
	// l'archivio stesso: il formato non dice il tipo
	if l := b.lettura(zipID); l.colonne() != "da_determinare:::0:estensione" || l.v.Tipo.Regola != "ext_archivio" || l.v.Da != classificazione.DaArchivio {
		t.Errorf("l'archivio: %s, %+v", l.colonne(), l.v.Tipo)
	}
	// i fatti restano la lettura del worker, per contenuto
	if got := b.testo(`SELECT fatti -> 'esito' ->> 'codice' FROM analisi_fatti WHERE sha256 = $1`, sha); got != "7120001A" {
		t.Errorf("i fatti: %s", got)
	}
}

// TestIlRumoreNonToccaITecniciDB (Smistamento, prova 118, parte di F4: P14): lo stesso contenuto scartato una
// volta come rumore per il dominio del mittente fa sparire un'immagine, ma non uno STEP ne' un PDF, che
// scendono, si leggono e si analizzano. La parte «Riporta» che rianalizza e' della F11, e si aggiunge li'.
func TestIlRumoreNonToccaITecniciDB(t *testing.T) {
	b := nuovoBancoLetture(t)
	sha := strings.Repeat("9", 64)
	b.esegui(`INSERT INTO hash_rumore (sha256, dominio) VALUES ($1, 'acme.example')`, sha)
	for _, c := range []struct {
		nome, colonne string
		rumore        bool
	}{
		{"logo.png", "rumore:::90:rumore", true},
		{"7120001A_1.stp", "cad_3d:7120001A:1:45:nome_file", false},
		{"7120010.pdf", "da_determinare:7120010::45:nome_file", false},
	} {
		a := b.allegato(b.mail(uuid.Nil), c.nome, sha, 500_000)
		b.scende(a)
		l := b.lettura(a.AllegatoID)
		if l.colonne() != c.colonne || l.v.HaEvidenza("rumore_hash_dominio") != c.rumore {
			t.Errorf("%s: colonne %s, attese %s; evidenza del rumore %v", c.nome, l.colonne(), c.colonne, l.v.HaEvidenza("rumore_hash_dominio"))
		}
		stato := b.testo(`SELECT stato::text FROM allegato WHERE allegato_id = $1`, a.AllegatoID)
		jobs := b.testo(`SELECT count(*)::text FROM job WHERE tipo = 'analizza_allegato' AND payload ->> 'allegato_id' = $1`, a.AllegatoID.String())
		switch {
		case c.rumore && (stato != "analizzato" || jobs != "0"):
			t.Errorf("%s: il rumore non si analizza (stato %s, analisi %s)", c.nome, stato, jobs)
		case !c.rumore && jobs == "0" && b.testo(`SELECT count(*)::text FROM job WHERE tipo = 'analizza_allegato' AND payload ->> 'sha256' = $1`, sha) == "0":
			t.Errorf("%s: un file tecnico si analizza", c.nome)
		}
	}
}

// TestUnaNuovaLetturaNonCancellaLaDestinazione (Smistamento, prova 190): la destinazione e' del flusso (F8), non
// di una lettura: lo stage rifatto e l'analisi che arriva riscrivono la valutazione e le colonne, e la
// destinazione resta com'era.
func TestUnaNuovaLetturaNonCancellaLaDestinazione(t *testing.T) {
	b := nuovoBancoLetture(t)
	a := b.allegato(b.mail(b.rfq(`{}`)), "7120010.pdf", strings.Repeat("1", 64), 300_000)
	b.scende(a)
	const destinazione = `{"v": 1, "tabella": "S1", "valore": "generale", "score": 0, "stato": "unica", "candidati": [{"rango": 1, "chiave": "generale"}]}`
	b.esegui(`UPDATE documento_proposta SET dettagli = dettagli || jsonb_build_object('destinazione', $2::jsonb) WHERE allegato_id = $1`, a.AllegatoID, destinazione)
	b.scende(a) // «Riscarica»
	if l := b.lettura(a.AllegatoID); l.dettagli["destinazione"] == nil || l.v.Da != classificazione.DaStage {
		t.Errorf("dopo lo stage rifatto: %v", l.dettagli)
	}
	b.analisi(a, worker.RisultatoAnalisi{TipoProposto: "disegno_2d", Codice: "7120010", Confidenza: 95, Fonte: "cartiglio",
		Dettagli: json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA"]}`)})
	l := b.lettura(a.AllegatoID)
	if got := b.testo(`SELECT (dettagli -> 'destinazione' = $2::jsonb)::text FROM documento_proposta WHERE allegato_id = $1`, a.AllegatoID, destinazione); got != "true" {
		t.Errorf("la destinazione dopo l'analisi: %s", l.dettagli["destinazione"])
	}
	if l.v.Da != classificazione.DaAnalisi || l.colonne() != "disegno_2d:7120010::45:nome_file" {
		t.Errorf("la lettura nuova: da %s, %s", l.v.Da, l.colonne())
	}
}

// TestLaStoriaSopravviveAllaRilettura (Smistamento, prova 215, parte della proposta del documento): la storia
// delle decisioni revocate sta in `dettagli.storia`, e una rilettura del file (stage, analisi) non la cancella.
// (La parte dei nodi e' della F5.)
func TestLaStoriaSopravviveAllaRilettura(t *testing.T) {
	b := nuovoBancoLetture(t)
	a := b.allegato(b.mail(b.rfq(`{}`)), "7120001A_1.stp", strings.Repeat("2", 64), 300_000)
	b.scende(a)
	const storia = `[{"esito": "componente", "deciso_da": "FP", "revocata_il": "2026-10-02T10:20:00Z"}]`
	b.esegui(`UPDATE documento_proposta SET dettagli = dettagli || jsonb_build_object('storia', $2::jsonb) WHERE allegato_id = $1`, a.AllegatoID, storia)
	b.scende(a)
	b.analisi(a, worker.RisultatoAnalisi{TipoProposto: "cad_3d", Codice: "7120001A", Rev: "1", Confidenza: 95, Fonte: "step",
		Dettagli: json.RawMessage(`{"product_step": "7120001A_1"}`)})
	if got := b.testo(`SELECT (dettagli -> 'storia' = $2::jsonb)::text || ':' || (dettagli #>> '{valutazione,da}') FROM documento_proposta WHERE allegato_id = $1`,
		a.AllegatoID, storia); got != "true:analisi" {
		t.Errorf("la storia dopo le riletture: %s", got)
	}
}

// TestUpsertPropostaConservaEAggiorna (Smistamento, prova 231, A5.14.7): UpsertProposta su una riga
// dell'operatore lascia colonne e decisione come sono, mette la lettura in `lettura_dopo` come prima e
// AGGIORNA la valutazione (e' una lettura, non una decisione); su una riga aperta conserva destinazione e
// storia; su una riga assegnata a un componente tiene il codice del componente, mette la lettura in
// `codice_letto` e conserva anche li' destinazione e storia. Una riga chiusa non cambia.
func TestUpsertPropostaConservaEAggiorna(t *testing.T) {
	b := nuovoBancoLetture(t)
	thread := b.rfq(`{}`)
	msg := b.mail(thread)
	analizza := func(a db.Allegato) {
		b.analisi(a, worker.RisultatoAnalisi{TipoProposto: "disegno_2d", Codice: "X", Confidenza: 95, Fonte: "cartiglio",
			Dettagli: json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA"]}`)})
	}
	chiavi := `jsonb_build_object('destinazione', '{"v": 1, "valore": "generale"}'::jsonb, 'storia', '[{"esito": "generale"}]'::jsonb)`

	// la riga dell'operatore
	op := b.allegato(msg, "7120010.pdf", strings.Repeat("3", 64), 300_000)
	b.scende(op)
	pid := b.id(`SELECT proposta_id FROM documento_proposta WHERE allegato_id = $1`, op.AllegatoID)
	if n, err := b.q.DecidiPropostaDocumento(b.ctx, db.DecidiPropostaDocumentoParams{PropostaID: pid, TipoProposto: db.TipoDocumentoDisegno2d,
		Codice: pgtype.Text{String: "7120011", Valid: true}, Rev: pgtype.Text{String: "C", Valid: true}}); err != nil || n != 1 {
		t.Fatalf("la decisione dell'operatore: %d %v", n, err)
	}
	analizza(op)
	got := b.testo(`SELECT tipo_proposto || ':' || codice || ':' || rev || ':' || confidenza || ':' || fonte || ':' || (dettagli ? 'deciso_da_operatore')::text || ':' ||
		(dettagli #>> '{lettura_dopo,codice}') || ':' || (dettagli #>> '{lettura_dopo,fonte}') || ':' || (dettagli #>> '{valutazione,da}') || ':' ||
		(dettagli #>> '{valutazione,tipo,regola}') FROM documento_proposta WHERE allegato_id = $1`, op.AllegatoID)
	if got != "disegno_2d:7120011:C:100:operatore:true:7120010:nome_file:analisi:pdf_termini_cartiglio" {
		t.Errorf("la riga dell'operatore: %s", got)
	}

	// la riga aperta
	aperta := b.allegato(msg, "7120012.pdf", strings.Repeat("4", 64), 300_000)
	b.scende(aperta)
	b.esegui(`UPDATE documento_proposta SET dettagli = dettagli || `+chiavi+` WHERE allegato_id = $1`, aperta.AllegatoID)
	analizza(aperta)
	if got := b.testo(`SELECT (dettagli #>> '{destinazione,valore}') || ':' || (dettagli #>> '{storia,0,esito}') || ':' || (dettagli #>> '{valutazione,da}') || ':' ||
		tipo_proposto FROM documento_proposta WHERE allegato_id = $1`, aperta.AllegatoID); got != "generale:generale:analisi:disegno_2d" {
		t.Errorf("la riga aperta: %s", got)
	}

	// la riga assegnata a un componente (legacy): il codice e' quello del componente
	utente := b.id(`INSERT INTO utente (sigla, nome, ufficio) VALUES ('FP', 'Prova F4', 'Tecnico') RETURNING utente_id`)
	comp := b.id(`INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, '7120020', 'sciolto', 'manuale', $2)
		RETURNING componente_id`, thread, utente)
	assegnata := b.allegato(msg, "7120013.pdf", strings.Repeat("5", 64), 300_000)
	b.scende(assegnata)
	b.esegui(`UPDATE documento_proposta SET componente_id = $2, codice = '7120020', dettagli = dettagli || `+chiavi+` WHERE allegato_id = $1`, assegnata.AllegatoID, comp)
	analizza(assegnata)
	if got := b.testo(`SELECT codice || ':' || (dettagli ->> 'codice_letto') || ':' || (dettagli #>> '{destinazione,valore}') || ':' || (dettagli #>> '{storia,0,esito}') || ':' ||
		(dettagli #>> '{valutazione,da}') FROM documento_proposta WHERE allegato_id = $1`, assegnata.AllegatoID); got != "7120020:7120013:generale:generale:analisi" {
		t.Errorf("la riga assegnata: %s", got)
	}

	// la riga chiusa non cambia
	chiusa := b.allegato(msg, "7120014.pdf", strings.Repeat("6", 64), 300_000)
	b.scende(chiusa)
	b.esegui(`UPDATE documento_proposta SET stato = 'scartata' WHERE allegato_id = $1`, chiusa.AllegatoID)
	prima := b.testo(`SELECT dettagli::text || tipo_proposto FROM documento_proposta WHERE allegato_id = $1`, chiusa.AllegatoID)
	analizza(chiusa)
	if dopo := b.testo(`SELECT dettagli::text || tipo_proposto FROM documento_proposta WHERE allegato_id = $1`, chiusa.AllegatoID); dopo != prima {
		t.Errorf("la riga chiusa e' cambiata:\nprima %s\ndopo  %s", prima, dopo)
	}
}

// rispostaFornitore e' il gesto «e' la risposta del fornitore» sulla mail msg come lo scrive la schermata
// (web.riproponiComeOffertaFornitore, provata dalla 123 con il suo gestore): le proposte aperte dei file della
// mail ricevono l'evidenza `risposta_fornitore` nella valutazione, e le colonne sono il riepilogo.
func (b *bancoLetture) rispostaFornitore(msg uuid.UUID) {
	b.t.Helper()
	righe, err := b.q.ListProposteRispostaFornitore(b.ctx, msg)
	if err != nil || len(righe) == 0 {
		b.t.Fatalf("le proposte della risposta del fornitore: %d, %v", len(righe), err)
	}
	for _, p := range righe {
		prima := classificazione.ValutazioneDellaRiga(string(p.TipoProposto), p.Codice.String, p.Rev.String, string(p.Fonte),
			int(p.Confidenza), p.Dettagli, p.NomeFile, p.Estensione.String)
		dett, rp := classificazione.ConValutazione(nil, prima.ConRispostaFornitore(), time.Now())
		if _, err := b.q.AggiornaValutazioneProposta(b.ctx, db.AggiornaValutazionePropostaParams{PropostaID: p.PropostaID,
			TipoProposto: db.TipoDocumento(rp.Tipo), Codice: pgtype.Text{String: rp.Codice, Valid: rp.Codice != ""},
			Rev: pgtype.Text{String: rp.Rev, Valid: rp.Rev != ""}, Confidenza: int16(rp.Confidenza), Fonte: db.FonteProposta(rp.Fonte), Dettagli: dett}); err != nil {
			b.t.Fatal(err)
		}
	}
}

// TestIlGestoDelFornitoreResisteAllaRilettura (Smistamento, A5.14.7, prove 123 e 230): l'operatore conferma
// la risposta del fornitore prima che i PDF siano analizzati. L'analisi che arriva dopo, e uno stage rifatto,
// rileggono il file con Valuta: il gesto resta un'evidenza del tipo. Il PDF letto senza termini resta
// `offerta_fornitore`; il disegno rimandato, con i termini del cartiglio (75 contro 70), e' `disegno_2d` ma
// discorde, e nessuno lo precompila. Prima di F4 l'analisi riscriveva il tipo e il gesto si perdeva.
func TestIlGestoDelFornitoreResisteAllaRilettura(t *testing.T) {
	b := nuovoBancoLetture(t)
	msg := b.mail(b.rfq(`{}`))
	offerta := b.allegato(msg, "offerta 7120001.pdf", strings.Repeat("a", 64), 80_000)
	disegno := b.allegato(msg, "7120001_disegno.pdf", strings.Repeat("b", 64), 90_000)
	b.scende(offerta)
	b.scende(disegno)
	b.rispostaFornitore(msg)
	if l := b.lettura(offerta.AllegatoID); l.colonne() != "offerta_fornitore:::70:direzione" || l.v.Da != classificazione.DaRispostaFornitore {
		t.Fatalf("dopo il gesto: %s (da %s)", l.colonne(), l.v.Da)
	}
	b.analisi(offerta, worker.RisultatoAnalisi{TipoProposto: "da_determinare", Confidenza: 40, Fonte: "estensione",
		Dettagli: json.RawMessage(`{"codice_riconosciuto": "", "testo_letto": 120}`)})
	b.analisi(disegno, worker.RisultatoAnalisi{TipoProposto: "disegno_2d", Codice: "7120001", Confidenza: 95, Fonte: "cartiglio",
		Dettagli: json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA"]}`)})
	for _, passo := range []string{"analisi", "stage rifatto"} {
		if passo == "stage rifatto" {
			b.scende(offerta)
			b.scende(disegno)
		}
		l := b.lettura(offerta.AllegatoID)
		if l.colonne() != "offerta_fornitore:::70:direzione" || !l.v.HaEvidenza("risposta_fornitore") || !l.laRiassume() ||
			l.v.Tipo.Regola != "risposta_fornitore" || !l.v.HaEvidenza("pdf_nessun_termine") || l.v.Da == classificazione.DaRispostaFornitore {
			t.Errorf("%s, l'offerta: %s (da %s), tipo %+v", passo, l.colonne(), l.v.Da, l.v.Tipo)
		}
		l = b.lettura(disegno.AllegatoID)
		if l.tipo != "disegno_2d" || !l.v.HaEvidenza("risposta_fornitore") || !l.laRiassume() ||
			l.v.Tipo.Regola != "pdf_termini_cartiglio" || l.v.Tipo.Score != 75 || l.v.Tipo.Stato != classificazione.StatoDiscorde {
			t.Errorf("%s, il disegno rimandato: %s (da %s), tipo %+v", passo, l.colonne(), l.v.Da, l.v.Tipo)
		}
	}
}
