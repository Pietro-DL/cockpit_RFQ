//go:build integrazione

// L4 — blocco 4A: la pulizia dello staging toglie i contenuti che non servono piu' a nessuno.
//
// Un contenuto e' di TUTTI gli allegati che lo hanno: finche' esiste una riga con quel sha256 il file
// serve, e cancellarlo lascerebbe senza file anche allegati che non c'entrano con quello che ha
// fatto scattare la pulizia. Per questo la domanda si fa sull'hash — che e' il nome del file — e non
// sul percorso: su Windows due percorsi possono differire per maiuscole o separatori e indicare lo
// stesso file, e una pulizia che sbaglia il confronto cancella il disegno che stava per essere
// copiato sul NAS.
package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/testutil"
)

// contenutoDiProva scrive un contenuto nello staging con l'eta' indicata e ne restituisce percorso e
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

// allegatoConSha inserisce un allegato che nomina quel contenuto.
func allegatoConSha(t *testing.T, ctx context.Context, p *pgxpool.Pool, sha string) {
	t.Helper()
	var convID, msgID string
	if err := p.QueryRow(ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook', $1, now()) RETURNING conversazione_id`, "CONV-gc-"+sha[:8]).Scan(&convID); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento)
		VALUES ('outlook', $1, $2, 'entrata', now()) RETURNING messaggio_id`,
		"<gc-"+sha[:8]+"@x>", convID).Scan(&msgID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, natura, origine, sha256, ricevuto_il)
		VALUES ($1, 1, 'x.pdf', 'file', 'outlook', $2, now())`, msgID, sha); err != nil {
		t.Fatal(err)
	}
}

func TestPulisciContenutiTogliesoloQuelliCheNessunoNomina(t *testing.T) {
	p, q, ctx := preparaDB(t)
	staging := t.TempDir()
	vecchio := time.Now().AddDate(0, 0, -40)

	usato, shaUsato := contenutoDiProva(t, staging, "un disegno che serve ancora", vecchio)
	orfano, _ := contenutoDiProva(t, staging, "un disegno che non nomina piu' nessuno", vecchio)
	fresco, _ := contenutoDiProva(t, staging, "appena promosso, non ancora scritto in database", time.Now())
	allegatoConSha(t, ctx, p, shaUsato)

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

	n, liberati, err := PulisciContenuti(ctx, q, staging, 30, 10*time.Minute, testutil.LogSilenzioso())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("rimossi %d contenuti, atteso 1", n)
	}
	if liberati <= 0 {
		t.Errorf("byte liberati = %d: la misura serve a chi legge il log", liberati)
	}
	if _, err := os.Stat(orfano); err == nil {
		t.Errorf("il contenuto che nessuno nomina e' ancora li': %s", orfano)
	}
	for nome, f := range map[string]string{"nominato da un allegato": usato, "appena promosso": fresco, "non scritto da noi": estraneo} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("il contenuto %s e' stato rimosso: %s", nome, f)
		}
	}
}

// A zero giorni non si cancella niente, ed e' il valore predefinito: la pulizia dello staging toglie
// FILE, e un file cancellato per sbaglio si riprende solo se l'elemento e' ancora in Outlook.
func TestPulisciContenutiSpentaNonTocccaNiente(t *testing.T) {
	_, q, ctx := preparaDB(t)
	staging := t.TempDir()
	orfano, _ := contenutoDiProva(t, staging, "nessuno lo nomina", time.Now().AddDate(0, 0, -400))

	n, _, err := PulisciContenuti(ctx, q, staging, 0, 10*time.Minute, testutil.LogSilenzioso())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("con la retention a zero sono stati rimossi %d contenuti", n)
	}
	if _, err := os.Stat(orfano); err != nil {
		t.Errorf("con la retention a zero il file e' stato rimosso: %s", orfano)
	}
}

// Lo staging di prima del blocco 4A e' organizzato per messaggio, con i nomi veri dei file. La
// pulizia non ci entra: li' un nome non dice niente sul contenuto, quindi non c'e' modo di sapere che
// cosa sia ancora in uso, e cancellare a occhio non e' una pulizia.
func TestPulisciContenutiNonEntraNelloStagingVecchio(t *testing.T) {
	_, q, ctx := preparaDB(t)
	staging := t.TempDir()
	vecchio := time.Now().AddDate(0, 0, -400)
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

	n, _, err := PulisciContenuti(ctx, q, staging, 30, 10*time.Minute, testutil.LogSilenzioso())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("la pulizia ha toccato %d file dello staging vecchio", n)
	}
	if _, err := os.Stat(perMessaggio); err != nil {
		t.Errorf("un file dello staging per messaggio e' stato rimosso: %s", perMessaggio)
	}
}
