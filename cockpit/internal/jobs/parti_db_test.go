//go:build integrazione

// L4 — N8 (parte 2.3): i .parte.<token> dei tentativi che non esistono più vengono rimossi dallo
// staging; quelli dei tentativi in corso no, qualunque età abbiano.
package jobs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/testutil"
)

func TestPulisciPartiRimuoveSoloGliOrfaniVecchi(t *testing.T) {
	p, q, ctx := preparaDB(t)
	staging := t.TempDir()

	// un tentativo vivo: il suo token è in corso
	accoda(t, ctx, q, db.TipoJobStageAllegato, "stage:vivo", Opzioni{})
	vivo := claim(t, ctx, q, db.WorkerTipoOutlook, "outlook@PC")
	if vivo == nil || !vivo.LeaseToken.Valid {
		t.Fatal("nessun tentativo in corso")
	}
	_ = p

	vecchio := time.Now().Add(-2 * time.Hour)
	scrivi := func(rel string, quando time.Time) string {
		t.Helper()
		f := filepath.Join(staging, filepath.FromSlash(rel))
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

	n, err := PulisciParti(ctx, q, staging, 10*time.Minute, testutil.LogSilenzioso())
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
