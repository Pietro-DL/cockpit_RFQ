//go:build integrazione

// L4 — Pre-7: la copia sul NAS USA la cache, non la consuma; e un contenuto sparito dalla cache si
// riprende da solo.
//
// Il documento e' confermato: l'operatore ha gia' deciso che quel file va sul NAS. Se il contenuto
// non c'e' piu' — l'ha tolto il custode della cache, o un disco rifatto — il file e' ancora in
// Outlook o dentro l'archivio da cui era stato estratto, e chiedere a una persona di premere
// «Riscarica» per una cosa che il sistema sa fare da solo e' un vicolo cieco travestito da pulsante.
package jobs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
	"promatec/cockpit/internal/platform/storage/staging"
)

// bancoRipresa e' una RFQ con un messaggio presente in una casella (cosi' da Outlook si puo'
// riscaricare), un allegato il cui contenuto sta nella cache con l'hash VERO, e il documento
// confermato che lo aspetta.
type bancoRipresa struct {
	staging, percorso, sha   string
	doc, allegato, messaggio uuid.UUID
}

func nuovoBancoRipresa(t *testing.T, ctx context.Context, p *pgxpool.Pool, suffisso string) bancoRipresa {
	t.Helper()
	var b bancoRipresa
	b.staging = t.TempDir()
	testo := contenutoProva + " " + suffisso
	tmp := filepath.Join(b.staging, "tmp.pdf")
	if err := os.WriteFile(tmp, []byte(testo), 0o644); err != nil {
		t.Fatal(err)
	}
	sha, _, err := nas.Sha256File(tmp)
	if err != nil {
		t.Fatal(err)
	}
	b.sha = sha
	b.percorso, err = staging.PercorsoContenuto(b.staging, sha, "disegno.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(b.percorso), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, b.percorso); err != nil {
		t.Fatal(err)
	}

	var cliente, thread, conv, casella, utente uuid.UUID
	deve := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	deve(p.QueryRow(ctx, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ($1,$2) RETURNING cliente_id`,
		"RIPRESA"+suffisso, "Ripresa "+suffisso).Scan(&cliente))
	deve(p.QueryRow(ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa, priorita)
		VALUES ($1,'outlook',now(),'RFQ ripresa',$2,1) RETURNING thread_id`,
		cliente, `RIPRESA`+suffisso+`\WIP\2026 09 18 prova`).Scan(&thread))
	deve(p.QueryRow(ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook',$1,now()) RETURNING conversazione_id`, "CONV-ripresa-"+suffisso).Scan(&conv))
	deve(p.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto, thread_id)
		VALUES ('outlook',$1,$2,'entrata',now(),'prova',$3) RETURNING messaggio_id`,
		"<ripresa-"+suffisso+"@acme.example>", conv, thread).Scan(&b.messaggio))
	deve(p.QueryRow(ctx, `INSERT INTO casella (canale, indirizzo, nome, condivisa) VALUES ('outlook','commerciale@azienda.it','Commerciale',true)
		ON CONFLICT (canale, indirizzo) DO UPDATE SET nome = EXCLUDED.nome RETURNING casella_id`).Scan(&casella))
	_, err = p.Exec(ctx, `INSERT INTO messaggio_outlook (messaggio_id) VALUES ($1)`, b.messaggio)
	deve(err)
	_, err = p.Exec(ctx, `INSERT INTO messaggio_casella (messaggio_id, casella_id, entry_id, cartella, ricevuto_il)
		VALUES ($1,$2,$3,'Posta in arrivo',now())`, b.messaggio, casella, "ENTRY-ripresa-"+suffisso)
	deve(err)
	deve(p.QueryRow(ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1,1,'disegno.pdf','pdf','file','outlook',$2,$3,$4,'analizzato',now()) RETURNING allegato_id`,
		b.messaggio, len(testo), sha, b.percorso).Scan(&b.allegato))
	deve(p.QueryRow(ctx, `INSERT INTO utente (sigla, nome, ufficio, ruolo) VALUES ($1,'Prova','tecnico','operatore') RETURNING utente_id`,
		"R"+suffisso).Scan(&utente))
	deve(p.QueryRow(ctx, `INSERT INTO documento (thread_id, tipo, nome_file, estensione, sha256, bytes, path_relativo, stato_nas, confermato_da)
		VALUES ($1,'disegno_2d','disegno.pdf','pdf',$2,$3,$4,'in_coda',$5) RETURNING documento_id`,
		thread, sha, len(testo), `ELENCO DISEGNI\disegno.pdf`, utente).Scan(&b.doc))
	return b
}

func contaJobPerChiave(t *testing.T, ctx context.Context, p *pgxpool.Pool, chiave string) (pendenti, totali int) {
	t.Helper()
	if err := p.QueryRow(ctx, `SELECT count(*) FILTER (WHERE stato IN ('pronto','in_corso')), count(*) FROM job WHERE chiave_idempotenza = $1`,
		chiave).Scan(&pendenti, &totali); err != nil {
		t.Fatal(err)
	}
	return
}

// ST6. Dopo la copia il contenuto e' ANCORA nella cache — non e' una coda che si svuota al successo —
// e il suo orario e' stato rinfrescato, cosi' il custode sa che serve.
func TestLaCopiaSulNasNonConsumaLaCache(t *testing.T) {
	p, q, ctx := preparaDB(t)
	conCapacita(t, coda.Capacita{NasScrittura: true})
	b := nuovoBancoRipresa(t, ctx, p, "A")
	vecchio := time.Now().AddDate(0, 0, -40)
	if err := os.Chtimes(b.percorso, vecchio, vecchio); err != nil {
		t.Fatal(err)
	}
	if _, err := coda.AccodaCopia(ctx, q, b.doc); err != nil {
		t.Fatal(err)
	}
	e := &EsecutoreServer{Pool: p, NAS: &nas.Scrittore{Radice: t.TempDir()}}
	if err := eseguiLaCopia(t, ctx, q, e); err != nil {
		t.Fatalf("la copia non e' riuscita: %v", err)
	}
	if s := statoNas(t, ctx, q, b.doc); s != db.StatoNasScritto {
		t.Fatalf("stato_nas = %q, atteso scritto", s)
	}
	st, err := os.Stat(b.percorso)
	if err != nil {
		t.Fatalf("dopo la copia il contenuto non e' piu' nella cache: %v", err)
	}
	if time.Since(st.ModTime()) > time.Minute {
		t.Errorf("l'orario del contenuto non e' stato rinfrescato dalla copia: %s", st.ModTime())
	}
}

// ST7. Il contenuto non c'e' piu': la copia fallisce QUESTO tentativo dicendo che cosa sta facendo,
// e il download da Outlook e' gia' in coda. Nessuno deve premere niente.
func TestUnContenutoSparitoSiRiprendeDaOutlookDaSolo(t *testing.T) {
	p, q, ctx := preparaDB(t)
	conCapacita(t, coda.Capacita{NasScrittura: true})
	b := nuovoBancoRipresa(t, ctx, p, "B")
	if err := os.Remove(b.percorso); err != nil {
		t.Fatal(err)
	}
	if _, err := coda.AccodaCopia(ctx, q, b.doc); err != nil {
		t.Fatal(err)
	}
	e := &EsecutoreServer{Pool: p, NAS: &nas.Scrittore{Radice: t.TempDir()}}
	err := eseguiLaCopia(t, ctx, q, e)
	if err == nil {
		t.Fatal("la copia e' riuscita senza il contenuto")
	}
	if !errors.Is(err, documenti.ErrContenutoMancante) {
		t.Errorf("l'errore non e' «contenuto mancante»: %v", err)
	}
	if !strings.Contains(err.Error(), "download da Outlook accodato") {
		t.Errorf("l'errore non dice che il download e' stato accodato: %v", err)
	}
	if strings.Contains(err.Error(), "Riscarica") {
		t.Errorf("l'errore chiede all'operatore quello che il sistema ha appena fatto da solo: %v", err)
	}
	if pend, _ := contaJobPerChiave(t, ctx, p, "stage:"+b.allegato.String()); pend != 1 {
		t.Errorf("job di download pendenti = %d, atteso 1", pend)
	}
	d, err := q.GetDocumento(ctx, b.doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.ErroreNas.String, "Outlook") {
		t.Errorf("il fascicolo non dice che il contenuto si sta riprendendo da Outlook: %q", d.ErroreNas.String)
	}
}

// ST7, seconda forma. La voce viene da un archivio che e' ANCORA in cache: si riestrae da li', senza
// tornare in Outlook. E' la riparazione a buon mercato per cui la dipendenza archivio → voci esiste.
func TestUnaVoceDiArchivioSparitaSiRiestraeSenzaTornareInOutlook(t *testing.T) {
	p, q, ctx := preparaDB(t)
	conCapacita(t, coda.Capacita{NasScrittura: true})
	b := nuovoBancoRipresa(t, ctx, p, "C")
	// l'archivio: un contenuto qualunque, in cache, con un allegato suo
	zip, shaZip := contenutoDiProva(t, b.staging, "l'archivio da cui viene la voce", time.Now())
	var zid uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1,2,'disegni.zip','zip','file','outlook',10,$2,$3,'analizzato',now()) RETURNING allegato_id`,
		b.messaggio, shaZip, zip).Scan(&zid); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `UPDATE allegato SET contenitore_id = $2, indice = 1 WHERE allegato_id = $1`, b.allegato, zid); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(b.percorso); err != nil {
		t.Fatal(err)
	}
	if _, err := coda.AccodaCopia(ctx, q, b.doc); err != nil {
		t.Fatal(err)
	}
	e := &EsecutoreServer{Pool: p, NAS: &nas.Scrittore{Radice: t.TempDir()}}
	err := eseguiLaCopia(t, ctx, q, e)
	if err == nil || !strings.Contains(err.Error(), "riestrazione accodata") {
		t.Fatalf("attesa la riestrazione accodata, ottenuto: %v", err)
	}
	if pend, _ := contaJobPerChiave(t, ctx, p, "estrai:"+zid.String()); pend != 1 {
		t.Errorf("job di estrazione pendenti = %d, atteso 1", pend)
	}
	if _, tot := contaJobPerChiave(t, ctx, p, "stage:"+zid.String()); tot != 0 {
		t.Errorf("e' stato accodato un download da Outlook mentre l'archivio era in cache")
	}
}

// ST7, il limite. Se per QUESTA copia il download da Outlook e' gia' stato provato ed e' fallito, non
// si insiste: torna il messaggio con «Riscarica», perche' a quel punto serve una persona. Senza
// questo limite una mail cancellata da Outlook farebbe accodare un download a ogni tentativo.
func TestSeIlDownloadEGiaFallitoNonSiInsiste(t *testing.T) {
	p, q, ctx := preparaDB(t)
	conCapacita(t, coda.Capacita{NasScrittura: true})
	b := nuovoBancoRipresa(t, ctx, p, "D")
	if err := os.Remove(b.percorso); err != nil {
		t.Fatal(err)
	}
	if _, err := coda.AccodaCopia(ctx, q, b.doc); err != nil {
		t.Fatal(err)
	}
	chiave := "stage:" + b.allegato.String()
	if _, err := p.Exec(ctx, `INSERT INTO job (tipo, worker_tipo, payload, chiave_idempotenza, stato, errore, chiuso_il)
		VALUES ('stage_allegato','outlook','{}',$1,'fallito','elemento non trovato in Outlook', now() + interval '1 second')`, chiave); err != nil {
		t.Fatal(err)
	}
	e := &EsecutoreServer{Pool: p, NAS: &nas.Scrittore{Radice: t.TempDir()}}
	err := eseguiLaCopia(t, ctx, q, e)
	if err == nil {
		t.Fatal("la copia e' riuscita senza il contenuto")
	}
	if !strings.Contains(err.Error(), "Riscarica") || !strings.Contains(err.Error(), "gia' stato provato") {
		t.Errorf("con il download gia' fallito l'errore deve rimandare a una persona: %v", err)
	}
	if pend, tot := contaJobPerChiave(t, ctx, p, chiave); pend != 0 || tot != 1 {
		t.Errorf("e' stato accodato un altro download (pendenti=%d, totali=%d)", pend, tot)
	}
}

// Il limite guarda QUESTA copia: un download fallito un'ora fa, per un'altra ragione o per un
// altro tentativo, non impedisce di riprovare adesso. Altrimenti un solo fallimento nella storia
// dell'allegato lo condannerebbe per sempre al pulsante.
func TestUnDownloadFallitoPrimaDiQuestaCopiaNonBlocca(t *testing.T) {
	p, q, ctx := preparaDB(t)
	conCapacita(t, coda.Capacita{NasScrittura: true})
	b := nuovoBancoRipresa(t, ctx, p, "E")
	if err := os.Remove(b.percorso); err != nil {
		t.Fatal(err)
	}
	chiave := "stage:" + b.allegato.String()
	if _, err := p.Exec(ctx, `INSERT INTO job (tipo, worker_tipo, payload, chiave_idempotenza, stato, errore, chiuso_il)
		VALUES ('stage_allegato','outlook','{}',$1,'fallito','Outlook chiuso', now() - interval '1 hour')`, chiave); err != nil {
		t.Fatal(err)
	}
	if _, err := coda.AccodaCopia(ctx, q, b.doc); err != nil {
		t.Fatal(err)
	}
	e := &EsecutoreServer{Pool: p, NAS: &nas.Scrittore{Radice: t.TempDir()}}
	err := eseguiLaCopia(t, ctx, q, e)
	if err == nil || !strings.Contains(err.Error(), "download da Outlook accodato") {
		t.Fatalf("attesa la ripresa accodata, ottenuto: %v", err)
	}
	if pend, tot := contaJobPerChiave(t, ctx, p, chiave); pend != 1 || tot != 2 {
		t.Errorf("pendenti=%d totali=%d, attesi 1 e 2", pend, tot)
	}
}
