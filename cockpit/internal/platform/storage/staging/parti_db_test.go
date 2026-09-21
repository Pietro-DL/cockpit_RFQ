//go:build integrazione

// L4 — N8 (parte 2.3): i .parte.<token> dei tentativi che non esistono più vengono rimossi dallo
// staging; quelli dei tentativi in corso no, qualunque età abbiano.
package staging_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
	"promatec/cockpit/internal/platform/testutil"
)

func TestPulisciPartiRimuoveSoloGliOrfaniVecchi(t *testing.T) {
	p, q, ctx := preparaDB(t)
	radice := t.TempDir()

	// un tentativo vivo: il suo token è in corso
	accoda(t, ctx, q, db.TipoJobStageAllegato, "stage:vivo", coda.Opzioni{})
	vivo := claim(t, ctx, q, db.WorkerTipoOutlook, "outlook@PC")
	if vivo == nil || !vivo.LeaseToken.Valid {
		t.Fatal("nessun tentativo in corso")
	}
	_ = p

	vecchio := time.Now().Add(-2 * time.Hour)
	scrivi := func(rel string, quando time.Time) string {
		t.Helper()
		f := filepath.Join(radice, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(f, quando, quando); err != nil {
			t.Fatal(err)
		}
		return f
	}
	inCorso := scrivi("a/01_x.pdf.parte."+vivo.LeaseToken.UUID.String(), vecchio) // vivo: resta anche se vecchio
	orfano := scrivi("a/01_y.pdf.parte."+uuid.NewString(), vecchio)               // orfano vecchio: via
	fresco := scrivi("b/02_z.pdf.parte."+uuid.NewString(), time.Now())            // orfano ma appena scritto: resta un giro
	definitivo := scrivi("a/01_w.pdf", vecchio)                                   // non è un .parte
	nas := scrivi("c/03_v.pdf.parte", vecchio)                                    // il .parte del NAS non ha token: non è nostro

	n, err := staging.PulisciParti(ctx, q, radice, 10*time.Minute, testutil.LogSilenzioso())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("rimossi %d file, atteso 1", n)
	}
	for nome, f := range map[string]string{"in corso": inCorso, "fresco": fresco, "definitivo": definitivo, "parte del NAS": nas} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("il file %s è stato rimosso: %s", nome, f)
		}
	}
	if _, err := os.Stat(orfano); err == nil {
		t.Errorf("l'orfano vecchio è ancora lì: %s", orfano)
	}
}

// Anche una cartella di estrazione rimasta a meta' e' un orfano. Non e' un file .parte, quindi la
// camminata che cerca i .parte non la vedrebbe mai: un processo morto mentre scompattava un archivio
// lascerebbe li' i suoi file per sempre.
func TestPulisciPartiRimuoveLeEstrazioniInterrotte(t *testing.T) {
	_, q, ctx := preparaDB(t)
	radice := t.TempDir()

	accoda(t, ctx, q, db.TipoJobStageAllegato, "stage:zip-vivo", coda.Opzioni{})
	vivo := claim(t, ctx, q, db.WorkerTipoOutlook, "outlook@PC")
	if vivo == nil || !vivo.LeaseToken.Valid {
		t.Fatal("nessun tentativo in corso")
	}
	vecchio := time.Now().Add(-2 * time.Hour)

	cartella := func(token uuid.UUID, quando time.Time) string {
		t.Helper()
		d := staging.PercorsoEstrazione(radice, token)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "001_disegno.pdf"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(d, quando, quando); err != nil {
			t.Fatal(err)
		}
		return d
	}
	inCorso := cartella(vivo.LeaseToken.UUID, vecchio) // il tentativo sta ancora estraendo: non si tocca
	orfana := cartella(uuid.New(), vecchio)            // nessuno la finira' piu': via
	fresca := cartella(uuid.New(), time.Now())         // appena nata: resta un giro

	if _, err := staging.PulisciParti(ctx, q, radice, 10*time.Minute, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orfana); err == nil {
		t.Errorf("l'estrazione interrotta e' ancora li': %s", orfana)
	}
	for nome, d := range map[string]string{"di un tentativo in corso": inCorso, "appena cominciata": fresca} {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("l'estrazione %s e' stata rimossa: %s", nome, d)
		}
	}
}
