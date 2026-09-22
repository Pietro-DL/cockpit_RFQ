//go:build integrazione

// L4 — blocco 5B: la riconciliazione, contro un PostgreSQL vero e file veri su disco.
//
// Tre dei cinque casi chiesti dal checkpoint, piu' due sul comportamento di «Allinea»:
//
//	file gia' corretto  → niente copia inutile
//	file assente ma DB «scritto» → segnalazione
//	NAS irraggiungibile → non si guarda affatto (che e' diverso da «va tutto bene»)
//
// Gli altri due — documento in_coda → Riprova → scritto, e hash diverso → conflitto — fanno eseguire
// davvero il job di copia, e stanno con l'esecutore.
package documenti

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
	"promatec/cockpit/internal/platform/testutil"
)

const contenutoProva = "questo e' il disegno vero"

// bancoIntegrita prepara una RFQ con un documento confermato il cui contenuto e' in staging, con
// l'hash VERO del contenuto: la copia sul NAS verifica l'hash dopo aver copiato, e un hash finto
// farebbe fallire la copia per il motivo sbagliato.
func bancoIntegrita(t *testing.T, ctx context.Context, p *pgxpool.Pool, suffisso string) (uuid.UUID, string) {
	t.Helper()
	staging := t.TempDir()
	percorso := filepath.Join(staging, "disegno"+suffisso+".pdf")
	if err := os.WriteFile(percorso, []byte(contenutoProva), 0o644); err != nil {
		t.Fatal(err)
	}
	sha, _, err := nas.Sha256File(percorso)
	if err != nil {
		t.Fatal(err)
	}

	var cliente, thread, conv, msg, allegato, utente, documento uuid.UUID
	deve := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	deve(p.QueryRow(ctx, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ($1,$2) RETURNING cliente_id`,
		"ACME"+suffisso, "Acme "+suffisso).Scan(&cliente))
	deve(p.QueryRow(ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa, priorita)
		VALUES ($1,'outlook',now(),'RFQ integrita',$2,1) RETURNING thread_id`,
		cliente, `ACME`+suffisso+`\WIP\2026 09 17 prova`).Scan(&thread))
	deve(p.QueryRow(ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook',$1,now()) RETURNING conversazione_id`, "CONV-"+suffisso).Scan(&conv))
	deve(p.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto, thread_id)
		VALUES ('outlook',$1,$2,'entrata',now(),'prova',$3) RETURNING messaggio_id`,
		"MSG-"+suffisso, conv, thread).Scan(&msg))
	deve(p.QueryRow(ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1,1,'disegno.pdf','pdf','file',$2,$3,$4,'analizzato',now()) RETURNING allegato_id`,
		msg, len(contenutoProva), sha, percorso).Scan(&allegato))
	deve(p.QueryRow(ctx, `INSERT INTO utente (sigla, nome, ufficio, ruolo) VALUES ($1,'Prova','tecnico','operatore') RETURNING utente_id`,
		"I"+suffisso).Scan(&utente))
	deve(p.QueryRow(ctx, `INSERT INTO documento (thread_id, tipo, nome_file, estensione, sha256, bytes, path_relativo, stato_nas, confermato_da, confermato_il)
		VALUES ($1,'disegno_2d','disegno.pdf','pdf',$2,$3,$4,'in_coda',$5, now() - interval '3 days') RETURNING documento_id`,
		thread, sha, len(contenutoProva), `ELENCO DISEGNI\disegno.pdf`, utente).Scan(&documento))
	return documento, sha
}

// destinazione e' il percorso assoluto del file sul NAS di questo documento.
func destinazione(t *testing.T, ctx context.Context, q *db.Queries, radice string, doc uuid.UUID) string {
	t.Helper()
	d, err := q.GetDocumento(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	th, err := q.GetThread(ctx, d.ThreadID)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(radice, filepath.FromSlash(strings.ReplaceAll(th.CartellaRelativa.String, `\`, "/")),
		filepath.FromSlash(strings.ReplaceAll(d.PathRelativo, `\`, "/")))
}

func statoNas(t *testing.T, ctx context.Context, q *db.Queries, doc uuid.UUID) db.StatoNas {
	t.Helper()
	d, err := q.GetDocumento(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	return d.StatoNas
}

// 2. File assente ma il database dice «scritto»: si segnala. E' la bugia che nessuno andava mai a
// controllare, perche' la promessa era stata fatta una volta sola.
func TestFileAssenteMaDatabaseScrittoVieneSegnalato(t *testing.T) {
	p, q, ctx := preparaDB(t)
	doc, _ := bancoIntegrita(t, ctx, p, "B")
	if _, err := p.Exec(ctx, `UPDATE documento SET stato_nas='scritto', scritto_il=now() WHERE documento_id=$1`, doc); err != nil {
		t.Fatal(err)
	}
	r := &Ricognitore{Pool: p, NAS: &nas.Scrittore{Radice: t.TempDir()}, Log: testutil.LogSilenzioso()}

	es, err := r.Giro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if es.Saltato != "" {
		t.Fatalf("la passata e' stata saltata: %s", es.Saltato)
	}
	a, err := q.GetAnomaliaNas(ctx, doc)
	if err != nil {
		t.Fatalf("nessuna anomalia aperta per un documento che dice di essere sul NAS senza esserci: %v", err)
	}
	if a.Problema != db.ProblemaNasMancante {
		t.Errorf("problema = %q, atteso mancante", a.Problema)
	}
	if !strings.Contains(a.Percorso, "disegno.pdf") {
		t.Errorf("l'anomalia non dice dove si e' guardato: %q", a.Percorso)
	}

	// una seconda passata non crea una seconda riga: la stessa cartella mancante non deve diventare
	// novanta righe al giorno
	if _, err := r.Giro(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := p.QueryRow(ctx, `SELECT count(*)::int FROM nas_anomalia WHERE documento_id=$1`, doc).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("righe di anomalia per lo stesso documento: %d, attesa 1", n)
	}

	// e quando il file torna, l'anomalia si chiude da sola: non resta un allarme spento da spegnere
	dst := destinazione(t, ctx, q, r.NAS.Radice, doc)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte(contenutoProva), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Giro(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := q.GetAnomaliaNas(ctx, doc); err == nil {
		t.Error("il file e' tornato al suo posto e l'anomalia e' ancora aperta")
	}
}

// 4. File gia' corretto: niente copia inutile. Il file e' li' ed e' byte per byte quello giusto —
// copiarlo sopra se stesso sarebbe la stessa verifica fatta due volte, con in mezzo una scrittura su
// una condivisione di rete.
func TestFileGiaCorrettoNienteCopiaInutile(t *testing.T) {
	p, q, ctx := preparaDB(t)
	conCapacita(t, coda.Capacita{NasScrittura: true})
	doc, sha := bancoIntegrita(t, ctx, p, "D")
	radice := t.TempDir()
	dst := destinazione(t, ctx, q, radice, doc)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte(contenutoProva), 0o644); err != nil {
		t.Fatal(err)
	}
	prima, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	r := &Ricognitore{Pool: p, NAS: &nas.Scrittore{Radice: radice}, Log: testutil.LogSilenzioso()}

	if _, err := r.Giro(ctx); err != nil {
		t.Fatal(err)
	}
	a, err := q.GetAnomaliaNas(ctx, doc)
	if err != nil {
		t.Fatalf("un documento «in_coda» il cui file e' gia' sul NAS non viene segnalato: resterebbe in_coda per sempre (%v)", err)
	}
	if a.Problema != db.ProblemaNasGiaPresente {
		t.Fatalf("problema = %q, atteso gia_presente", a.Problema)
	}
	// il ricognitore non accoda niente da solo
	if n, err := q.ListCopieNasPendenti(ctx); err != nil || len(n) != 0 {
		t.Errorf("il ricognitore ha accodato %d copie da solo (err %v)", len(n), err)
	}

	// «Allinea»: ricalcola l'hash ADESSO e segna scritto senza copiare niente
	if err := AllineaDocumento(ctx, q, r.NAS, doc); err != nil {
		t.Fatalf("allinea: %v", err)
	}
	if s := statoNas(t, ctx, q, doc); s != db.StatoNasScritto {
		t.Errorf("stato_nas = %q, atteso scritto", s)
	}
	if _, err := q.GetAnomaliaNas(ctx, doc); err == nil {
		t.Error("l'anomalia resta aperta dopo l'allineamento")
	}
	dopo, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !dopo.ModTime().Equal(prima.ModTime()) || dopo.Size() != prima.Size() {
		t.Errorf("il file e' stato riscritto: non c'era niente da copiare")
	}
	if trovato, _, _ := nas.Sha256File(dst); trovato != sha {
		t.Errorf("il contenuto del file e' cambiato")
	}
}

// «Allinea» non e' una scorciatoia: su un conflitto rifiuta, e il documento resta com'era.
func TestAllineaRifiutaUnConflitto(t *testing.T) {
	p, q, ctx := preparaDB(t)
	doc, _ := bancoIntegrita(t, ctx, p, "E")
	radice := t.TempDir()
	dst := destinazione(t, ctx, q, radice, doc)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("un altro file"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := AllineaDocumento(ctx, q, &nas.Scrittore{Radice: radice}, doc)
	if err == nil {
		t.Fatal("un file diverso e' stato dichiarato «scritto»: il fascicolo direbbe di avere una cosa che non ha")
	}
	if !strings.Contains(err.Error(), "conflitto") {
		t.Errorf("l'errore non dice che si tratta di un conflitto: %v", err)
	}
	if s := statoNas(t, ctx, q, doc); s != db.StatoNasInCoda {
		t.Errorf("stato_nas = %q, atteso in_coda (invariato)", s)
	}
}

// 5. NAS irraggiungibile: NON si guarda. Una passata fatta adesso direbbe che mancano tutti i file, e
// da domani la volta in cui ne manca uno davvero non si distinguerebbe piu' dalle altre.
func TestSenzaNasNonSiGuardaAffatto(t *testing.T) {
	p, q, ctx := preparaDB(t)
	doc, _ := bancoIntegrita(t, ctx, p, "F")
	if _, err := p.Exec(ctx, `UPDATE documento SET stato_nas='scritto', scritto_il=now() WHERE documento_id=$1`, doc); err != nil {
		t.Fatal(err)
	}
	assente := filepath.Join(t.TempDir(), "nas-che-non-c-e")
	r := &Ricognitore{Pool: p, NAS: &nas.Scrittore{Radice: assente}, Log: testutil.LogSilenzioso()}

	es, err := r.Giro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if es.Saltato == "" {
		t.Fatal("con il NAS irraggiungibile la passata e' stata fatta lo stesso: ogni file risulterebbe mancante")
	}
	if es.Guardati != 0 {
		t.Errorf("documenti guardati con il NAS assente: %d", es.Guardati)
	}
	if _, err := q.GetAnomaliaNas(ctx, doc); err == nil {
		t.Error("un cavo staccato ha prodotto un'anomalia sul documento")
	}
	d, err := q.GetDocumento(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	if d.VerificatoIl != nil {
		t.Error("il documento risulta verificato e non e' stato guardato: la schermata direbbe che il controllo e' recente")
	}
}

// Una copia gia' in coda non e' un lavoro per una persona: non deve comparire in elenco.
func TestUnaCopiaInCodaNonCompareFraLeAnomalie(t *testing.T) {
	p, q, ctx := preparaDB(t)
	conCapacita(t, coda.Capacita{NasScrittura: true})
	doc, _ := bancoIntegrita(t, ctx, p, "G")
	if _, err := coda.AccodaCopia(ctx, q, doc); err != nil {
		t.Fatal(err)
	}
	r := &Ricognitore{Pool: p, NAS: &nas.Scrittore{Radice: t.TempDir()}, Log: testutil.LogSilenzioso(),
		Attesa: time.Minute}

	if _, err := r.Giro(ctx); err != nil {
		t.Fatal(err)
	}
	if a, err := q.GetAnomaliaNas(ctx, doc); err == nil {
		t.Errorf("segnalato come %q un documento la cui copia e' gia' in coda", a.Problema)
	}
}
