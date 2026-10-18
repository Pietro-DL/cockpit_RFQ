//go:build integrazione

package workerapi

// L4 — V1, corsia file: il worker e il ciclo dei file della Sorgente (PM23 §10 righe 7 e 9; EM §10.7, G-1, G-2, G-4,
// G-5, G-7): AF-07 (l'analisi che arriva mentre il file e' in Altri file), CO-01 (i fatti vanno alle occorrenze
// attive), CO-02 (il risultato tardivo di un download annullato, e la guardia G-1 quando il job non si e' potuto
// annullare), CO-03 (la riestrazione dopo lo scarto di una voce e con una voce in Altri file), M-33 (la parte del Go:
// stage, estrazione, caricamento e «Riattiva» scrivono proposte allineate alla RFQ del messaggio).

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
	"promatec/cockpit/internal/platform/testutil"
)

// bancoCiclo: il server del worker con lo staging temporaneo e il servizio del ciclo dei file che lo usa come strada
// d'ingresso.
type bancoCiclo struct {
	t       *testing.T
	ctx     context.Context
	pool    *pgxpool.Pool
	q       *db.Queries
	s       *Server
	svc     *filerfq.Servizio
	staging string
	utente  db.Utente
	cliente uuid.UUID
	n       int
}

func preparaBancoCiclo(t *testing.T) *bancoCiclo {
	t.Helper()
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	ctx := context.Background()
	q := db.New(pool)
	dir := t.TempDir()
	an := coda.Analizzatore{Versione: 1, Parametri: map[string]any{"termini": []any{"scala"}}}
	s := &Server{Pool: pool, Log: testutil.LogSilenzioso(), Staging: dir, Analizzatore: an}
	dep, err := filerfq.PreparaDeposito(ctx, q, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b := &bancoCiclo{t: t, ctx: ctx, pool: pool, q: q, s: s, staging: dir,
		svc: &filerfq.Servizio{Pool: pool, Staging: dir, Deposito: dep, Pipeline: s, Log: testutil.LogSilenzioso(), EtaMinima: time.Nanosecond}}
	var uid uuid.UUID
	b.riga(`INSERT INTO utente (sigla, nome, ufficio) VALUES ('CF', 'Prova Ciclo', 'Tecnico') RETURNING utente_id`, &uid)
	if b.utente, err = q.GetUtente(ctx, uid); err != nil {
		t.Fatal(err)
	}
	b.riga(`INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME', 'Acme Prova') RETURNING cliente_id`, &b.cliente)
	return b
}

func (b *bancoCiclo) riga(sql string, dst any, arg ...any) {
	b.t.Helper()
	if err := b.pool.QueryRow(b.ctx, sql, arg...).Scan(dst); err != nil {
		b.t.Fatalf("%v: %s", err, sql)
	}
}

func (b *bancoCiclo) esegui(sql string, arg ...any) {
	b.t.Helper()
	if _, err := b.pool.Exec(b.ctx, sql, arg...); err != nil {
		b.t.Fatalf("%v: %s", err, sql)
	}
}

func (b *bancoCiclo) rfq(chiave string) uuid.UUID {
	b.t.Helper()
	var id uuid.UUID
	b.riga(`INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa) VALUES ($1, 'outlook', now(), $2, $3)
		RETURNING thread_id`, &id, b.cliente, "RFQ "+chiave, `ACME\WIP\2026 10 18 `+chiave)
	return id
}

func (b *bancoCiclo) mail(thread uuid.UUID) uuid.UUID {
	b.t.Helper()
	b.n++
	var conv, msg uuid.UUID
	b.riga(`INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', $1, now()) RETURNING conversazione_id`,
		&conv, fmt.Sprintf("CONV-CICLO-%d", b.n))
	b.riga(`INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto, mittente_indirizzo, thread_id)
		VALUES ('outlook', $1, $2, 'entrata', now(), 'RFQ ACME', 'mario.rossi@acme.example', $3) RETURNING messaggio_id`,
		&msg, fmt.Sprintf("<ciclo-%d@acme.example>", b.n), conv, thread)
	return msg
}

// sceso: un allegato della RFQ sceso nello staging (il posto del contenuto), con la strada dello stage fatta come la fa
// il risultato del download (dopoStaging: la proposta dal nome, l'analisi accodata o l'estrazione).
func (b *bancoCiclo) sceso(thread uuid.UUID, nome string, c []byte) db.Allegato {
	b.t.Helper()
	msg := b.mail(thread)
	sha := shaDi(c)
	p, err := staging.PercorsoContenuto(b.staging, sha, nome)
	if err != nil {
		b.t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		b.t.Fatal(err)
	}
	if err := os.WriteFile(p, c, 0o644); err != nil {
		b.t.Fatal(err)
	}
	var id uuid.UUID
	b.riga(`INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1, $2, $3, $4, 'file', 'outlook', $5, $6, $7, 'in_staging', now()) RETURNING allegato_id`, &id,
		msg, b.n, nome, strings.ToLower(strings.TrimPrefix(filepath.Ext(nome), ".")), len(c), sha, p)
	b.inTx(func(q *db.Queries) error {
		return b.s.dopoStaging(b.ctx, q, worker.RisultatoStage{AllegatoID: id, Sha256: sha, Bytes: int64(len(c))})
	})
	a, err := b.q.GetAllegato(b.ctx, id)
	if err != nil {
		b.t.Fatal(err)
	}
	return a
}

func (b *bancoCiclo) inTx(fn func(q *db.Queries) error) {
	b.t.Helper()
	tx, err := b.pool.Begin(b.ctx)
	if err != nil {
		b.t.Fatal(err)
	}
	defer tx.Rollback(b.ctx)
	if err := fn(db.New(tx)); err != nil {
		b.t.Fatal(err)
	}
	if err := tx.Commit(b.ctx); err != nil {
		b.t.Fatal(err)
	}
}

// pdfDisegno: un PDF sintetico con il termine del cartiglio.
func pdfDisegno(seme int64) []byte {
	c, _ := contenutoCasuale(3_000, seme)
	return append([]byte("%PDF-1.4 SCALA\n"), c...)
}

// analisiFatta: il risultato dell'analisi del job dell'allegato, applicato come lo applica il result del worker.
func (b *bancoCiclo) analisiFatta(a db.Allegato, r worker.RisultatoAnalisi) {
	b.t.Helper()
	var jobID int64
	b.riga(`SELECT job_id FROM job WHERE tipo = 'analizza_allegato' AND payload ->> 'allegato_id' = $1::text ORDER BY job_id DESC LIMIT 1`,
		&jobID, a.AllegatoID.String())
	j, err := b.q.GetJob(b.ctx, jobID)
	if err != nil {
		b.t.Fatal(err)
	}
	r.AllegatoID, r.VersioneAnalizzatore, r.HashConfigurazione = a.AllegatoID, b.s.Analizzatore.Versione, b.s.Analizzatore.Hash()
	dati, _ := json.Marshal(r)
	b.inTx(func(q *db.Queries) error { return b.s.applicaRisultato(b.ctx, q, &j, dati, nil) })
	b.esegui(`UPDATE job SET stato = 'fatto', chiuso_il = now() WHERE job_id = $1`, jobID)
}

func letturaCartiglio() worker.RisultatoAnalisi {
	return worker.RisultatoAnalisi{TipoProposto: "disegno_2d", Codice: "1234567A", Rev: "4", Confidenza: 92, Fonte: "cartiglio",
		Dettagli: json.RawMessage(`{"cartiglio": true, "termini": ["scala"]}`)}
}

// proposta: la proposta di documento dell'allegato, nei campi che una lettura cambia.
func (b *bancoCiclo) proposta(a uuid.UUID) string {
	b.t.Helper()
	var v string
	b.riga(`SELECT stato::text || '|' || tipo_proposto::text || '|' || fonte::text || '|' || confidenza || '|' || coalesce(codice, '') || '|' ||
		coalesce(rev, '') || '|' || dettagli::text FROM documento_proposta WHERE allegato_id = $1`, &v, a)
	return v
}

// corpo: il corpo di un gesto dal modello della Sorgente letto adesso.
func (b *bancoCiclo) corpo(gesto string, thread, allegato uuid.UUID, motivo string, mod ...func(*filerfq.CorpoGesto)) filerfq.CorpoGesto {
	b.t.Helper()
	m, err := b.svc.Modello(b.ctx, thread, filerfq.OpzioniModello{File: &allegato, Scartati: true})
	if err != nil {
		b.t.Fatal(err)
	}
	var ver int32
	for _, f := range m.File {
		if f.AllegatoID == allegato {
			ver = f.Versione
		}
	}
	k := uuid.New()
	c := filerfq.CorpoGesto{VersioneAttesa: &ver, ChiaveIdempotenza: &k}
	if motivo != "" {
		c.Motivo = &motivo
	}
	if gesto != filerfq.GestoRiattiva {
		imp := m.Dettaglio.ImprontaDipendenze
		c.ImprontaDipendenze = &imp
	}
	if m.Dettaglio.NVoci != nil {
		n, iv := *m.Dettaglio.NVoci, m.Dettaglio.ImprontaVoci
		c.NVoci, c.ImprontaVoci = &n, &iv
	}
	for _, fn := range mod {
		fn(&c)
	}
	return c
}

func (b *bancoCiclo) gesto(e *filerfq.EsitoGesto, err error) *filerfq.EsitoGesto {
	b.t.Helper()
	if err != nil {
		if rf, ok := filerfq.ComeRifiuto(err); ok {
			b.t.Fatalf("gesto rifiutato: %d %s %q %v", rf.Stato, rf.Codice, rf.Testo, rf.Extra)
		}
		b.t.Fatalf("gesto non riuscito: %v", err)
	}
	return e
}

func (b *bancoCiclo) scarta(thread, a uuid.UUID) *filerfq.EsitoGesto {
	b.t.Helper()
	return b.gesto(b.svc.Scarta(b.ctx, thread, a, b.utente.UtenteID, b.corpo(filerfq.GestoScarta, thread, a, "non serve")))
}

func (b *bancoCiclo) altriFile(thread, a uuid.UUID) *filerfq.EsitoGesto {
	b.t.Helper()
	return b.gesto(b.svc.MettiInAltriFile(b.ctx, thread, a, b.utente.UtenteID, b.corpo(filerfq.GestoAltriFile, thread, a, "")))
}

func (b *bancoCiclo) riattiva(thread, a uuid.UUID) *filerfq.EsitoGesto {
	b.t.Helper()
	return b.gesto(b.svc.Riattiva(b.ctx, thread, a, b.utente.UtenteID, b.corpo(filerfq.GestoRiattiva, thread, a, "")))
}

// disallineate: le proposte la cui RFQ non e' quella del messaggio del loro allegato (M-33).
func (b *bancoCiclo) disallineate() int {
	b.t.Helper()
	var n int
	b.riga(`SELECT count(*) FROM documento_proposta p JOIN allegato a ON a.allegato_id = p.allegato_id
		JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE p.thread_id IS DISTINCT FROM m.thread_id`, &n)
	return n
}

// ---------------------------------------------------------------- AF-07, CO-01: i fatti

// AF-07: l'analisi del contenuto arriva mentre l'occorrenza e' in Altri file: i fatti si scrivono per sha, l'occorrenza
// non riceve la lettura (G-2); dopo «Riattiva» la strada d'ingresso applica i fatti (G-4).
func TestAF07AnalisiMentreIlFileEInAltriFile(t *testing.T) {
	b := preparaBancoCiclo(t)
	th := b.rfq("RFQ-A")
	a := b.sceso(th, "1234567A_4.pdf", pdfDisegno(70))
	b.altriFile(th, a.AllegatoID)
	prima := b.proposta(a.AllegatoID)
	b.analisiFatta(a, letturaCartiglio())
	var n int
	b.riga(`SELECT count(*) FROM analisi_fatti WHERE sha256 = $1`, &n, a.Sha256.String)
	if n != 1 {
		t.Errorf("fatti per sha: %d righe", n)
	}
	if got := b.proposta(a.AllegatoID); got != prima {
		t.Errorf("l'occorrenza in Altri file ha ricevuto la lettura:\n prima %s\n dopo  %s", prima, got)
	}
	b.riattiva(th, a.AllegatoID)
	if got := b.proposta(a.AllegatoID); got == prima || !strings.Contains(got, "fatti_esistenti") {
		t.Errorf("dopo «Riattiva» i fatti non si sono applicati: %s", got)
	}
}

// CO-01: un'analisi in coda con A nel payload e lo scarto di A: il job resta, i fatti arrivano alla copia attiva B e
// non ad A; poi la rimozione di A si chiude.
func TestCO01IFattiVannoAlleOccorrenzeAttive(t *testing.T) {
	b := preparaBancoCiclo(t)
	thA, thB := b.rfq("RFQ-A"), b.rfq("RFQ-B")
	c := pdfDisegno(71)
	a := b.sceso(thA, "1234567A_4.pdf", c)
	bb := b.sceso(thB, "1234567A_4.pdf", c)
	b.scarta(thA, a.AllegatoID)
	var rim string
	b.riga(`SELECT stato || '/' || motivo FROM file_rfq_rimozione WHERE allegato_id = $1`, &rim, a.AllegatoID)
	if rim != "in_attesa/lavoro_in_corso" {
		t.Fatalf("rimozione %s", rim)
	}
	primaA, primaB := b.proposta(a.AllegatoID), b.proposta(bb.AllegatoID)
	b.analisiFatta(a, letturaCartiglio())
	if got := b.proposta(bb.AllegatoID); got == primaB {
		t.Error("la copia attiva B non ha ricevuto i fatti")
	}
	if got := b.proposta(a.AllegatoID); got != primaA {
		t.Errorf("l'occorrenza scartata A ha ricevuto i fatti: %s", got)
	}
	var id int64
	b.riga(`SELECT rimozione_id FROM file_rfq_rimozione WHERE allegato_id = $1`, &id, a.AllegatoID)
	if p, err := b.svc.EseguiRimozione(b.ctx, id, false); err != nil || (p.Stato != "eseguita" && p.Stato != "non_necessaria_condiviso") {
		t.Errorf("finito il lavoro, la rimozione: %+v %v", p, err)
	}
}

// ---------------------------------------------------------------- CO-02: il download

// CO-02: un download pendente di A, lo scarto, il risultato tardivo: il job e' annullato, il risultato riceve 409, nessuna
// promozione e nessun path_staging. Con il job in mano a un'altra transazione durante lo scarto (SKIP LOCKED: il job
// non si annulla), il risultato passa per la guardia G-1: i byte non si registrano.
func TestCO02RisultatoTardivoDiUnDownload(t *testing.T) {
	for _, preso := range []bool{false, true} {
		t.Run(map[bool]string{false: "job annullato", true: "job in mano a un altro (G-1)"}[preso], func(t *testing.T) {
			b := preparaBanco(t, 0)
			th := b.rfqDelCliente(b.clienteConRegole("ACME", `{}`))
			b.msg = b.mettiNellaRfq(b.msg, th)
			svc := &filerfq.Servizio{Pool: b.pool, Staging: b.cartella, Log: testutil.LogSilenzioso(), EtaMinima: time.Nanosecond}
			var uid uuid.UUID
			if err := b.pool.QueryRow(b.ctx, `INSERT INTO utente (sigla, nome, ufficio) VALUES ('C2', 'Prova', 'Tecnico') RETURNING utente_id`).Scan(&uid); err != nil {
				t.Fatal(err)
			}
			tent := b.claim("outlook@PC-FRANCESCO")
			contenuto, sha := contenutoCasuale(5_000, 77)
			stato(t, b.put(tent, b.allegato.AllegatoID, bytes.NewReader(contenuto), int64(len(contenuto))), 204)
			m, err := svc.Modello(b.ctx, th, filerfq.OpzioniModello{File: &b.allegato.AllegatoID})
			if err != nil {
				t.Fatal(err)
			}
			ver, k, mot, imp := int32(0), uuid.New(), "non serve", m.Dettaglio.ImprontaDipendenze
			corpo := filerfq.CorpoGesto{VersioneAttesa: &ver, ChiaveIdempotenza: &k, Motivo: &mot, ImprontaDipendenze: &imp}
			var blocco interface{ Rollback(context.Context) error }
			if preso {
				tx, err := b.pool.Begin(b.ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := tx.Exec(b.ctx, `SELECT 1 FROM job WHERE job_id = $1 FOR UPDATE`, b.job.JobID); err != nil {
					t.Fatal(err)
				}
				blocco = tx
			}
			if _, err := svc.Scarta(b.ctx, th, b.allegato.AllegatoID, uid, corpo); err != nil {
				t.Fatalf("scarta: %v", err)
			}
			if blocco != nil {
				_ = blocco.Rollback(b.ctx)
			}
			j := b.jobOra()
			resp := b.result(tent, worker.RisultatoStage{AllegatoID: b.allegato.AllegatoID, Sha256: sha, Bytes: int64(len(contenuto))})
			if preso {
				if j.Stato == db.StatoJobAnnullato {
					t.Fatal("il job in mano a un'altra transazione e' stato annullato: SKIP LOCKED non ha saltato")
				}
				stato(t, resp, 204)
			} else {
				if j.Stato != db.StatoJobAnnullato {
					t.Errorf("il download pendente: %s", j.Stato)
				}
				stato(t, resp, 409)
			}
			if a := b.allegatoOra(); a.PathStaging.Valid {
				t.Errorf("path_staging su un file scartato: %q", a.PathStaging.String)
			}
			if c := b.contenuti(); len(c) != 0 {
				t.Errorf("il download di un file scartato e' stato promosso: %v", c)
			}
		})
	}
}

// ---------------------------------------------------------------- CO-03: la riestrazione

// zipOrdinato: uno zip con le voci nell'ordine dato.
func zipOrdinato(t *testing.T, voci ...[2]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, v := range voci {
		f, err := w.Create(v[0])
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(v[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// CO-03: la riestrazione dopo lo scarto di una voce e con una voce in Altri file: la voce scartata non si riscrive, quella
// in Altri file non riceve la lettura; con un'estrazione che mette un altro file allo stesso indice (le regole
// d'estrazione cambiate: qui lo zip nello staging sostituito), l'evento anomalia_riestrazione.
func TestCO03RiestrazioneConVociNonAttive(t *testing.T) {
	b := preparaBancoCiclo(t)
	th := b.rfq("RFQ-A")
	z := zipOrdinato(t, [2]string{"V1.pdf", "%PDF-1.4 prima voce"}, [2]string{"V2.pdf", "%PDF-1.4 seconda voce"},
		[2]string{"V3.txt", "terza voce"})
	arch := b.sceso(th, "ACME.zip", z)
	if _, err := b.s.EstraiArchivio(b.ctx, arch.AllegatoID, uuid.New()); err != nil {
		t.Fatal(err)
	}
	voce := func(indice int) db.Allegato {
		var id uuid.UUID
		b.riga(`SELECT allegato_id FROM allegato WHERE contenitore_id = $1 AND indice = $2`, &id, arch.AllegatoID, indice)
		a, err := b.q.GetAllegato(b.ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	v1, v2 := voce(1), voce(2)
	// le analisi delle voci accodate dall'estrazione sono finite (altrimenti il tempo 2 aspetta: CO-01)
	b.esegui(`UPDATE job SET stato = 'fatto', chiuso_il = now() WHERE tipo = 'analizza_allegato' AND stato IN ('pronto', 'in_corso')`)
	b.scarta(th, v1.AllegatoID)
	b.altriFile(th, v2.AllegatoID)
	if a := voce(1); a.PathStaging.Valid {
		t.Fatalf("la voce scartata ha ancora il suo file: %q", a.PathStaging.String)
	}
	propV2 := b.proposta(v2.AllegatoID)
	if _, err := b.s.EstraiArchivio(b.ctx, arch.AllegatoID, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if a := voce(1); a.PathStaging.Valid {
		t.Errorf("la riestrazione ha riscritto la voce scartata: %q", a.PathStaging.String)
	}
	if got := b.proposta(v2.AllegatoID); got != propV2 {
		t.Errorf("la voce in Altri file ha ricevuto la lettura della riestrazione:\n prima %s\n dopo  %s", propV2, got)
	}
	var stati string
	b.riga(`SELECT string_agg(coalesce(f.stato, 'attivo'), ',' ORDER BY a.indice) FROM allegato a
		LEFT JOIN file_rfq f ON f.allegato_id = a.allegato_id AND f.thread_id = $2 WHERE a.contenitore_id = $1`, &stati, arch.AllegatoID, th)
	if stati != "scartato,altri_file,attivo" {
		t.Errorf("stati delle voci dopo la riestrazione: %s", stati)
	}
	var anomalie int
	b.riga(`SELECT count(*) FROM file_rfq_evento WHERE azione = 'anomalia_riestrazione'`, &anomalie)
	if anomalie != 0 {
		t.Errorf("anomalie con la stessa estrazione: %d", anomalie)
	}
	// un'estrazione che mette un altro file all'indice della voce scartata
	z2 := zipOrdinato(t, [2]string{"ALTRO.pdf", "%PDF-1.4 un altro file"}, [2]string{"V2.pdf", "%PDF-1.4 seconda voce"},
		[2]string{"V3.txt", "terza voce"})
	if err := os.WriteFile(arch.PathStaging.String, z2, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := b.s.EstraiArchivio(b.ctx, arch.AllegatoID, uuid.New()); err != nil {
		t.Fatal(err)
	}
	var nome string
	b.riga(`SELECT coalesce(string_agg(esito ->> 'nome_trovato', ','), '') FROM file_rfq_evento WHERE azione = 'anomalia_riestrazione' AND allegato_id = $1`,
		&nome, v1.AllegatoID)
	if nome != "ALTRO.pdf" {
		t.Errorf("l'anomalia della riestrazione sulla voce scartata: %q", nome)
	}
	if a := voce(1); a.NomeFile != "V1.pdf" || a.PathStaging.Valid {
		t.Errorf("la voce scartata e' stata cambiata: %s %q", a.NomeFile, a.PathStaging.String)
	}
}

// ---------------------------------------------------------------- M-33

// M-33 (la parte del Go, DECA P-5 (persistenza)): dopo lo stage, l'estrazione, il caricamento (E1′, con la strada
// d'ingresso vera) e la «Riattiva», ogni proposta ha la RFQ del messaggio del suo allegato.
func TestM33ProposteAllineateDopoLeStradeDelGo(t *testing.T) {
	b := preparaBancoCiclo(t)
	th := b.rfq("RFQ-A")
	pdf := b.sceso(th, "1234567A_4.pdf", pdfDisegno(72))
	z := zipOrdinato(t, [2]string{"V1.pdf", "%PDF-1.4 voce"}, [2]string{"V2.stp", "ISO-10303-21; voce"})
	arch := b.sceso(th, "ACME.zip", z)
	if _, err := b.s.EstraiArchivio(b.ctx, arch.AllegatoID, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if n := b.disallineate(); n != 0 {
		t.Errorf("dopo lo stage e l'estrazione: %d proposte disallineate", n)
	}
	for i, interna := range []bool{true, false} {
		c := pdfDisegno(int64(80 + i))
		f, err := filerfq.ScriviParziale(b.staging, bytes.NewReader(c), fmt.Sprintf("ACME-M33-%d.pdf", i), "application/pdf", 1<<30)
		if err != nil {
			t.Fatal(err)
		}
		es, err := b.svc.RegistraFile(b.ctx, th, b.utente, interna, f)
		if err != nil || es.Esito != "caricato" {
			t.Fatalf("caricamento: %+v %v", es, err)
		}
		if es.PropostaID == nil {
			t.Error("il caricamento con la strada d'ingresso vera non ha scritto la proposta")
		}
	}
	if n := b.disallineate(); n != 0 {
		t.Errorf("dopo il caricamento: %d proposte disallineate", n)
	}
	b.altriFile(th, pdf.AllegatoID)
	b.riattiva(th, pdf.AllegatoID)
	if n := b.disallineate(); n != 0 {
		t.Errorf("dopo la «Riattiva»: %d proposte disallineate", n)
	}
	var n int
	b.riga(`SELECT count(*) FROM documento_proposta`, &n)
	if n < 6 {
		t.Errorf("proposte scritte dalle strade del Go: %d", n)
	}
}
