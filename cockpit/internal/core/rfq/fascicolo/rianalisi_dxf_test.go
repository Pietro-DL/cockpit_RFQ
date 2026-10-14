package fascicolo

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

// K4. «Rianalizza» guarda anche i DXF gia' analizzati (dxfGiaAnalizzato): un file con il contenuto scaricato e
// l'estensione dxf (in qualunque forma), gia' analizzato. Non un PDF, non un DXF ancora in staging o senza sha256,
// non un collegamento. Se i fatti correnti hanno il testo di oggi lo dice worker.TestoDXFDaRileggere (contratti_test.go).
func TestIDxfGiaAnalizzatiSonoCandidatiAllaRianalisi(t *testing.T) {
	a := func(est, sha string, stato db.StatoAllegato, natura db.NaturaAllegato) db.Allegato {
		return db.Allegato{Estensione: pgtype.Text{String: est, Valid: est != ""}, Sha256: pgtype.Text{String: sha, Valid: sha != ""},
			Stato: stato, Natura: natura}
	}
	sha := "ab12"
	casi := []struct {
		nome string
		a    db.Allegato
		dxf  bool
	}{
		{"dxf analizzato", a("dxf", sha, db.StatoAllegatoAnalizzato, db.NaturaAllegatoFile), true},
		{"estensione con il punto e maiuscola", a(".DXF", sha, db.StatoAllegatoAnalizzato, db.NaturaAllegatoFile), true},
		{"pdf", a("pdf", sha, db.StatoAllegatoAnalizzato, db.NaturaAllegatoFile), false},
		{"in staging", a("dxf", sha, db.StatoAllegatoInStaging, db.NaturaAllegatoFile), false},
		{"senza contenuto", a("dxf", "", db.StatoAllegatoAnalizzato, db.NaturaAllegatoFile), false},
		{"un collegamento", a("dxf", sha, db.StatoAllegatoAnalizzato, db.NaturaAllegatoCollegamento), false},
	}
	for _, c := range casi {
		if got := dxfGiaAnalizzato(c.a); got != c.dxf {
			t.Errorf("%s: %v, atteso %v", c.nome, got, c.dxf)
		}
		if c.dxf && pdfGiaAnalizzato(c.a) {
			t.Errorf("%s: un DXF non e' un PDF da rileggere", c.nome)
		}
	}
}
