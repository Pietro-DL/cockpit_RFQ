//go:build integrazione

// L4 — Pre-7: lo staging e' una cache, non una coda (D31).
//
// Prima del blocco 7 la pulizia toglieva solo i contenuti che NESSUN allegato nominava piu'. Gli
// allegati non si cancellano mai, quindi in pratica non toglieva niente: `_contenuti` cresceva per
// sempre e sembrava una coda bloccata, mentre era una cache senza politica.
//
// Adesso un contenuto si toglie quando nessuno ne ha bisogno ADESSO — nessun documento che aspetta
// la copia, nessuna proposta aperta, nessun job pendente, nessuna anomalia NAS — e da tanto non lo
// tocca nessuno, oppure quando la cache ha superato la capienza. L'unita' e' il contenuto: alla
// rimozione tutti gli allegati che lo nominano perdono il percorso nella stessa transazione.
package staging

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/testutil"
)

// contenutoDiProva scrive un contenuto nella cache con l'orario indicato e ne restituisce percorso e
// hash.
func contenutoDiProva(t *testing.T, staging, testo string, quando time.Time) (string, string) {
	t.Helper()
	somma := sha256.Sum256([]byte(testo))
	sha := hex.EncodeToString(somma[:])
	p, err := PercorsoContenuto(staging, sha, "x.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(testo), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, quando, quando); err != nil {
		t.Fatal(err)
	}
	return p, sha
}

// bancoCache e' una RFQ con un messaggio, su cui appendere allegati e documenti.
type bancoCache struct {
	thread, messaggio, utente uuid.UUID
}

func nuovoBancoCache(t *testing.T, ctx context.Context, p *pgxpool.Pool, suffisso string) bancoCache {
	t.Helper()
	var b bancoCache
	var cliente, conv uuid.UUID
	deve := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	deve(p.QueryRow(ctx, `INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ($1,$2) RETURNING cliente_id`,
		"CACHE"+suffisso, "Cache "+suffisso).Scan(&cliente))
	deve(p.QueryRow(ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa, priorita)
		VALUES ($1,'outlook',now(),'RFQ cache',$2,1) RETURNING thread_id`,
		cliente, `CACHE`+suffisso+`\WIP\2026 09 18 prova`).Scan(&b.thread))
	deve(p.QueryRow(ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook',$1,now()) RETURNING conversazione_id`, "CONV-cache-"+suffisso).Scan(&conv))
	deve(p.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto, thread_id)
		VALUES ('outlook',$1,$2,'entrata',now() - interval '40 days','prova',$3) RETURNING messaggio_id`,
		"MSG-cache-"+suffisso, conv, b.thread).Scan(&b.messaggio))
	deve(p.QueryRow(ctx, `INSERT INTO utente (sigla, nome, ufficio, ruolo) VALUES ($1,'Prova','tecnico','operatore') RETURNING utente_id`,
		"C"+suffisso).Scan(&b.utente))
	return b
}

// allegato appende al messaggio un allegato che nomina quel contenuto, ricevuto in quel momento.
func (b bancoCache) allegato(t *testing.T, ctx context.Context, p *pgxpool.Pool, indice int, sha, percorso string, ricevuto time.Time, contenitore uuid.NullUUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO allegato (messaggio_id, contenitore_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1,$2,$3,'x.pdf','pdf','file','outlook',10,$4,NULLIF($5,''),'analizzato',$6) RETURNING allegato_id`,
		b.messaggio, contenitore, indice, sha, percorso, ricevuto).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// documento appende alla RFQ un documento confermato quaranta giorni fa con quell'hash.
func (b bancoCache) documento(t *testing.T, ctx context.Context, p *pgxpool.Pool, sha, stato string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO documento (thread_id, tipo, nome_file, estensione, sha256, bytes, path_relativo, stato_nas, confermato_da, confermato_il)
		VALUES ($1,'disegno_2d','x.pdf','pdf',$2,10,$3,$4::stato_nas,$5, now() - interval '40 days') RETURNING documento_id`,
		b.thread, sha, `ELENCO DISEGNI\`+sha[:6]+`.pdf`, stato, b.utente).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func cacheDiProva(p *pgxpool.Pool, staging string) *Cache {
	return &Cache{Pool: p, Staging: staging, Log: testutil.LogSilenzioso(), Retention: 30 * 24 * time.Hour, EtaMinima: time.Minute}
}

func esiste(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func pathStagingDi(t *testing.T, ctx context.Context, p *pgxpool.Pool, id uuid.UUID) (string, bool) {
	t.Helper()
	var path *string
	var stato string
	if err := p.QueryRow(ctx, `SELECT path_staging, stato::text FROM allegato WHERE allegato_id = $1`, id).Scan(&path, &stato); err != nil {
		t.Fatal(err)
	}
	if path == nil {
		return "", false
	}
	return *path, true
}

var vecchio = time.Now().AddDate(0, 0, -40)

// ST1. Un documento confermato che aspetta ancora la copia tiene il contenuto, per quanto vecchio.
// Toglierlo adesso vorrebbe dire far fallire la copia che sta per partire.
func TestCacheST1UnDocumentoCheAspettaLaCopiaTieneIlContenuto(t *testing.T) {
	p, _, ctx := preparaDB(t)
	staging := t.TempDir()
	b := nuovoBancoCache(t, ctx, p, "1")
	for i, stato := range []string{"in_coda", "errore"} {
		percorso, sha := contenutoDiProva(t, staging, "disegno che aspetta "+stato, vecchio)
		aid := b.allegato(t, ctx, p, i+1, sha, percorso, vecchio, uuid.NullUUID{})
		b.documento(t, ctx, p, sha, stato)

		es, err := cacheDiProva(p, staging).Giro(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !esiste(percorso) {
			t.Errorf("stato %s: il contenuto che un documento aspetta e' stato tolto", stato)
		}
		if _, ok := pathStagingDi(t, ctx, p, aid); !ok {
			t.Errorf("stato %s: l'allegato ha perso il percorso di un contenuto che c'e' ancora", stato)
		}
		if es.Pinnati == 0 {
			t.Errorf("stato %s: la passata non ha contato nessun contenuto pinnato", stato)
		}
	}
}

// ST1, seconda forma. Una proposta ancora aperta: l'operatore non ha deciso, e quando decide il
// file deve esserci.
func TestCacheST1UnaPropostaApertaTieneIlContenuto(t *testing.T) {
	p, _, ctx := preparaDB(t)
	staging := t.TempDir()
	b := nuovoBancoCache(t, ctx, p, "2")
	percorso, sha := contenutoDiProva(t, staging, "disegno con la proposta aperta", vecchio)
	aid := b.allegato(t, ctx, p, 1, sha, percorso, vecchio, uuid.NullUUID{})
	if _, err := p.Exec(ctx, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte, stato)
		VALUES ($1,$2,'disegno_2d',80,'estensione','aperta')`, aid, b.thread); err != nil {
		t.Fatal(err)
	}

	es, err := cacheDiProva(p, staging).Giro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !esiste(percorso) || es.Rimossi != 0 {
		t.Errorf("il contenuto di una proposta aperta e' stato tolto (rimossi=%d)", es.Rimossi)
	}

	// decisa la proposta, il contenuto e' libero, e vecchio: se ne va
	if _, err := p.Exec(ctx, `UPDATE documento_proposta SET stato='scartata' WHERE allegato_id=$1`, aid); err != nil {
		t.Fatal(err)
	}
	es, err = cacheDiProva(p, staging).Giro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if esiste(percorso) || es.Rimossi != 1 {
		t.Errorf("decisa la proposta, il contenuto vecchio e' rimasto (rimossi=%d)", es.Rimossi)
	}
}

// ST1, terza forma. Un job pendente che cita il contenuto — per hash, per allegato o per documento.
func TestCacheST1UnJobPendenteTieneIlContenuto(t *testing.T) {
	p, _, ctx := preparaDB(t)
	staging := t.TempDir()
	b := nuovoBancoCache(t, ctx, p, "3")
	perHash, shaHash := contenutoDiProva(t, staging, "citato per hash", vecchio)
	perAllegato, shaAllegato := contenutoDiProva(t, staging, "citato per allegato", vecchio)
	perDocumento, shaDocumento := contenutoDiProva(t, staging, "citato per documento", vecchio)
	b.allegato(t, ctx, p, 1, shaHash, perHash, vecchio, uuid.NullUUID{})
	aid := b.allegato(t, ctx, p, 2, shaAllegato, perAllegato, vecchio, uuid.NullUUID{})
	b.allegato(t, ctx, p, 3, shaDocumento, perDocumento, vecchio, uuid.NullUUID{})
	// il documento e' gia' scritto: da solo non pinna niente, e' il job a farlo
	did := b.documento(t, ctx, p, shaDocumento, "scritto")
	for _, j := range []struct{ tipo, worker, payload string }{
		{"analizza_allegato", "analisi", `{"sha256":"` + shaHash + `"}`},
		{"estrai_archivio", "server", `{"allegato_id":"` + aid.String() + `"}`},
		{"copia_nas", "server", `{"documento_id":"` + did.String() + `"}`},
	} {
		if _, err := p.Exec(ctx, `INSERT INTO job (tipo, worker_tipo, payload, stato) VALUES ($1::tipo_job,$2::worker_tipo,$3::jsonb,'pronto')`,
			j.tipo, j.worker, j.payload); err != nil {
			t.Fatal(err)
		}
	}

	es, err := cacheDiProva(p, staging).Giro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for nome, f := range map[string]string{"per hash": perHash, "per allegato": perAllegato, "per documento": perDocumento} {
		if !esiste(f) {
			t.Errorf("il contenuto citato da un job pendente %s e' stato tolto", nome)
		}
	}
	if es.Pinnati != 3 {
		t.Errorf("pinnati = %d, attesi 3", es.Pinnati)
	}

	// chiusi i job, i tre contenuti sono liberi
	if _, err := p.Exec(ctx, `UPDATE job SET stato='fatto', chiuso_il=now()`); err != nil {
		t.Fatal(err)
	}
	es, err = cacheDiProva(p, staging).Giro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if es.Rimossi != 3 {
		t.Errorf("chiusi i job, rimossi = %d, attesi 3", es.Rimossi)
	}
}

// ST1, quarta forma. Un'anomalia NAS aperta su un documento gia' scritto: il riconciliatore
// potrebbe doverlo ricopiare, ed e' proprio il caso per cui la cache esiste.
func TestCacheST1UnAnomaliaNasApertaTieneIlContenuto(t *testing.T) {
	p, _, ctx := preparaDB(t)
	staging := t.TempDir()
	b := nuovoBancoCache(t, ctx, p, "4")
	percorso, sha := contenutoDiProva(t, staging, "scritto sul NAS, ma il file e' sparito", vecchio)
	b.allegato(t, ctx, p, 1, sha, percorso, vecchio, uuid.NullUUID{})
	did := b.documento(t, ctx, p, sha, "scritto")
	if _, err := p.Exec(ctx, `INSERT INTO nas_anomalia (documento_id, thread_id, problema, stato_db, percorso, sha_atteso)
		VALUES ($1,$2,'mancante','scritto','x',$3)`, did, b.thread, sha); err != nil {
		t.Fatal(err)
	}

	es, err := cacheDiProva(p, staging).Giro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !esiste(percorso) || es.Pinnati != 1 {
		t.Errorf("il contenuto di un'anomalia aperta e' stato tolto (pinnati=%d)", es.Pinnati)
	}

	if _, err := p.Exec(ctx, `UPDATE nas_anomalia SET risolta_il=now()`); err != nil {
		t.Fatal(err)
	}
	if es, err = cacheDiProva(p, staging).Giro(ctx); err != nil || es.Rimossi != 1 {
		t.Errorf("risolta l'anomalia, il contenuto vecchio e' rimasto (rimossi=%d, err=%v)", es.Rimossi, err)
	}
}

// ST2. Un contenuto libero e vecchio si toglie, e TUTTI gli allegati che lo nominavano perdono il
// percorso nella stessa passata. Lo stato resta com'e': il file e' ricostruibile.
func TestCacheST2UnContenutoLiberoEVecchioSiToglieEIPercorsiSiAzzerano(t *testing.T) {
	p, _, ctx := preparaDB(t)
	staging := t.TempDir()
	b := nuovoBancoCache(t, ctx, p, "5")
	percorso, sha := contenutoDiProva(t, staging, "lo stesso disegno in due mail", vecchio)
	a1 := b.allegato(t, ctx, p, 1, sha, percorso, vecchio, uuid.NullUUID{})
	a2 := b.allegato(t, ctx, p, 2, sha, percorso, vecchio, uuid.NullUUID{})
	b.documento(t, ctx, p, sha, "scritto") // gia' sul NAS: la copia non lo aspetta
	// e un file che non abbiamo scritto noi: non si chiama come un hash, quindi non si tocca
	estraneo := filepath.Join(staging, CartellaContenuti, "zz", "appunti.txt")
	if err := os.MkdirAll(filepath.Dir(estraneo), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(estraneo, []byte("nota"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(estraneo, vecchio, vecchio); err != nil {
		t.Fatal(err)
	}

	es, err := cacheDiProva(p, staging).Giro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if es.Rimossi != 1 || es.Liberati <= 0 {
		t.Errorf("rimossi=%d liberati=%d, attesi 1 e > 0", es.Rimossi, es.Liberati)
	}
	if esiste(percorso) {
		t.Errorf("il contenuto libero e vecchio e' ancora li': %s", percorso)
	}
	if !esiste(estraneo) {
		t.Errorf("un file non scritto da noi e' stato tolto: %s", estraneo)
	}
	for _, aid := range []uuid.UUID{a1, a2} {
		if path, ok := pathStagingDi(t, ctx, p, aid); ok {
			t.Errorf("l'allegato %s nomina ancora un file che non c'e': %s", aid, path)
		}
		var stato, hash string
		if err := p.QueryRow(ctx, `SELECT stato::text, sha256 FROM allegato WHERE allegato_id=$1`, aid).Scan(&stato, &hash); err != nil {
			t.Fatal(err)
		}
		if stato != "analizzato" || hash != sha {
			t.Errorf("l'allegato %s ha cambiato stato o hash: %s %s", aid, stato, hash[:8])
		}
	}
}

// ST2, il rovescio: «vecchio» vuol dire che ne' il disco ne' il database lo hanno toccato di
// recente. Un file fermo da quaranta giorni che una mail ha riportato ieri resta; e un file usato
// ieri (la copia sul NAS ne rinfresca l'orario) resta anche se la mail e' di quaranta giorni fa.
func TestCacheST2UnContenutoUsatoDiRecenteResta(t *testing.T) {
	p, _, ctx := preparaDB(t)
	staging := t.TempDir()
	b := nuovoBancoCache(t, ctx, p, "6")
	ieri := time.Now().Add(-24 * time.Hour)
	fileVecchio, sha1 := contenutoDiProva(t, staging, "file vecchio, mail di ieri", vecchio)
	b.allegato(t, ctx, p, 1, sha1, fileVecchio, ieri, uuid.NullUUID{})
	fileDiIeri, sha2 := contenutoDiProva(t, staging, "file di ieri, mail vecchia", ieri)
	b.allegato(t, ctx, p, 2, sha2, fileDiIeri, vecchio, uuid.NullUUID{})

	es, err := cacheDiProva(p, staging).Giro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if es.Rimossi != 0 || !esiste(fileVecchio) || !esiste(fileDiIeri) {
		t.Errorf("un contenuto usato di recente e' stato tolto (rimossi=%d)", es.Rimossi)
	}
}

// ST3. Un archivio non si toglie finche' una sua voce e' pinnata: se la voce sparisse, da lui si
// riestrae senza tornare in Outlook. Un archivio le cui voci sono tutte libere se ne va con loro.
func TestCacheST3UnArchivioConUnaVocePinnataResta(t *testing.T) {
	p, _, ctx := preparaDB(t)
	staging := t.TempDir()
	b := nuovoBancoCache(t, ctx, p, "7")
	zip1, shaZip1 := contenutoDiProva(t, staging, "archivio con una voce che aspetta la copia", vecchio)
	voce1, shaVoce1 := contenutoDiProva(t, staging, "voce che aspetta la copia", vecchio)
	z1 := b.allegato(t, ctx, p, 1, shaZip1, zip1, vecchio, uuid.NullUUID{})
	b.allegato(t, ctx, p, 1, shaVoce1, voce1, vecchio, uuid.NullUUID{UUID: z1, Valid: true})
	b.documento(t, ctx, p, shaVoce1, "in_coda")

	zip2, shaZip2 := contenutoDiProva(t, staging, "archivio con voci libere", vecchio)
	voce2, shaVoce2 := contenutoDiProva(t, staging, "voce libera", vecchio)
	z2 := b.allegato(t, ctx, p, 2, shaZip2, zip2, vecchio, uuid.NullUUID{})
	b.allegato(t, ctx, p, 1, shaVoce2, voce2, vecchio, uuid.NullUUID{UUID: z2, Valid: true})

	es, err := cacheDiProva(p, staging).Giro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !esiste(zip1) {
		t.Errorf("l'archivio con una voce pinnata e' stato tolto")
	}
	if !esiste(voce1) {
		t.Errorf("la voce che un documento aspetta e' stata tolta")
	}
	if esiste(zip2) || esiste(voce2) {
		t.Errorf("l'archivio libero e la sua voce libera sono rimasti")
	}
	if es.Pinnati != 2 || es.Rimossi != 2 {
		t.Errorf("pinnati=%d rimossi=%d, attesi 2 e 2", es.Pinnati, es.Rimossi)
	}
}

// ST3, il caso che conta davvero: la voce non e' piu' sul disco — l'ha tolta una passata
// precedente — ma un documento la aspetta. L'archivio DEVE restare: e' da lui che si riestrae.
func TestCacheST3LArchivioRestaAncheSeLaVocePinnataNonEPiuSulDisco(t *testing.T) {
	p, _, ctx := preparaDB(t)
	staging := t.TempDir()
	b := nuovoBancoCache(t, ctx, p, "8")
	zip, shaZip := contenutoDiProva(t, staging, "archivio della voce sparita", vecchio)
	z := b.allegato(t, ctx, p, 1, shaZip, zip, vecchio, uuid.NullUUID{})
	shaVoce := hex.EncodeToString([]byte("voce sparita dal disco, 32 byte."))
	b.allegato(t, ctx, p, 1, shaVoce, "", vecchio, uuid.NullUUID{UUID: z, Valid: true})
	b.documento(t, ctx, p, shaVoce, "in_coda")

	es, err := cacheDiProva(p, staging).Giro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !esiste(zip) || es.Pinnati != 1 {
		t.Errorf("l'archivio di una voce pinnata ma assente dal disco e' stato tolto (pinnati=%d)", es.Pinnati)
	}
}

// ST4. Oltre la capienza si toglie dai meno usati, finche' non si torna sotto: non tutto, e non a
// caso.
func TestCacheST4OltreLaCapienzaSiPartedaiMenoUsati(t *testing.T) {
	p, _, ctx := preparaDB(t)
	staging := t.TempDir()
	b := nuovoBancoCache(t, ctx, p, "9")
	adesso := time.Now()
	var percorsi []string
	for i, ore := range []int{3, 2, 1} { // il primo e' il meno usato
		quando := adesso.Add(-time.Duration(ore) * time.Hour)
		f, sha := contenutoDiProva(t, staging, "contenuto giovane numero "+string(rune('0'+i))+" di cento byte esatti, per contare la capienza .... "+time.Duration(ore).String(), quando)
		b.allegato(t, ctx, p, i+1, sha, f, quando, uuid.NullUUID{})
		percorsi = append(percorsi, f)
	}
	var totale int64
	for _, f := range percorsi {
		st, _ := os.Stat(f)
		totale += st.Size()
	}
	c := cacheDiProva(p, staging)
	c.MaxByte = totale - 1 // basta togliere il piu' vecchio

	es, err := c.Giro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if es.Rimossi != 1 {
		t.Errorf("rimossi = %d, atteso 1: si toglie il minimo che basta", es.Rimossi)
	}
	if esiste(percorsi[0]) {
		t.Errorf("il meno usato e' ancora li'")
	}
	if !esiste(percorsi[1]) || !esiste(percorsi[2]) {
		t.Errorf("sono stati tolti contenuti piu' usati di quello che bastava")
	}
}

// A retention zero non si toglie niente per eta': resta solo la capienza, se c'e'.
func TestCacheConRetentionZeroNonToglieNientePerEta(t *testing.T) {
	p, _, ctx := preparaDB(t)
	staging := t.TempDir()
	b := nuovoBancoCache(t, ctx, p, "10")
	f, sha := contenutoDiProva(t, staging, "libero da un anno", time.Now().AddDate(-1, 0, 0))
	b.allegato(t, ctx, p, 1, sha, f, time.Now().AddDate(-1, 0, 0), uuid.NullUUID{})
	c := cacheDiProva(p, staging)
	c.Retention = 0

	es, err := c.Giro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if es.Rimossi != 0 || !esiste(f) {
		t.Errorf("con la retention a zero e' stato tolto un contenuto per eta'")
	}
}

// Un contenuto appena nato non si tocca, nemmeno oltre la capienza: fra la rinomina e il commit che
// scrive l'allegato c'e' un istante in cui non lo nomina nessuno.
func TestCacheNonToccaUnContenutoAppenaNato(t *testing.T) {
	p, _, ctx := preparaDB(t)
	staging := t.TempDir()
	f, _ := contenutoDiProva(t, staging, "appena promosso, non ancora in database", time.Now())
	c := cacheDiProva(p, staging)
	c.MaxByte = 1

	es, err := c.Giro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if es.Rimossi != 0 || !esiste(f) {
		t.Errorf("un contenuto appena nato e' stato tolto")
	}
}

// Lo staging di prima del blocco 4A e' organizzato per messaggio, con i nomi veri dei file. La
// cache non ci entra: li' un nome non dice niente sul contenuto.
func TestCacheNonEntraNelloStagingVecchio(t *testing.T) {
	p, _, ctx := preparaDB(t)
	staging := t.TempDir()
	perMessaggio := filepath.Join(staging, "ab12cd34ef56", "01_disegno.pdf")
	if err := os.MkdirAll(filepath.Dir(perMessaggio), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(perMessaggio, []byte("disegno di prima"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(perMessaggio, vecchio, vecchio); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(staging, CartellaContenuti), 0o755); err != nil {
		t.Fatal(err)
	}

	es, err := cacheDiProva(p, staging).Giro(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if es.Rimossi != 0 || !esiste(perMessaggio) {
		t.Errorf("un file dello staging per messaggio e' stato tolto")
	}
}
