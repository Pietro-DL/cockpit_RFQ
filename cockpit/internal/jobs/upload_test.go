package jobs

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
)

// L1 — voce 2.3: il nome nello staging lo decide il server, e un payload storto non esce dallo staging.
func TestNomeFileStaging(t *testing.T) {
	casi := []struct {
		nome   string
		natura db.NaturaAllegato
		indice int16
		atteso string
	}{
		{"disegno.pdf", db.NaturaAllegatoFile, 1, "01_disegno.pdf"},
		{`rev:2/"finale".stp`, db.NaturaAllegatoFile, 3, "03_rev_2__finale_.stp"},
		{"", db.NaturaAllegatoFile, 2, "02_allegato"},
		{"RFQ inoltrata", db.NaturaAllegatoElementoOutlook, 1, "01_RFQ inoltrata.msg"},
		{"gia.msg", db.NaturaAllegatoElementoOutlook, 1, "01_gia.msg"},
	}
	for _, c := range casi {
		got := NomeFileStaging(db.Allegato{NomeFile: c.nome, Natura: c.natura, Indice: c.indice})
		if got != c.atteso {
			t.Errorf("%q (%s) → %q, atteso %q", c.nome, c.natura, got, c.atteso)
		}
	}
	lungo := NomeFileStaging(db.Allegato{NomeFile: strings.Repeat("a", 300) + ".pdf", Indice: 1})
	if len([]rune(lungo)) > 130 {
		t.Errorf("nome non troncato: %d caratteri", len([]rune(lungo)))
	}
}

func TestPercorsiStagingRifiutaCartelleNonSemplici(t *testing.T) {
	a := db.Allegato{NomeFile: "x.pdf", Indice: 1, Estensione: pgtype.Text{String: "pdf", Valid: true}}
	tok := uuid.New()
	for _, c := range []string{"", "..", `..\altrove`, "a/b", "ABC", "x"} {
		if _, _, err := PercorsiStaging(`C:\staging`, api.PayloadStageAllegato{Cartella: c}, a, tok); err == nil {
			t.Errorf("cartella %q accettata", c)
		}
	}
	def, parte, err := PercorsiStaging(`C:\staging`, api.PayloadStageAllegato{Cartella: CartellaStaging("<m@x>")}, a, tok)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(def) != "01_x.pdf" || parte != def+".parte."+tok.String() {
		t.Errorf("percorsi: %s / %s", def, parte)
	}
	got, ok := TokenDiParte(filepath.Base(parte))
	if !ok || got != tok {
		t.Errorf("TokenDiParte(%s) = %v, %v", filepath.Base(parte), got, ok)
	}
	if _, ok := TokenDiParte("01_x.pdf.parte"); ok {
		t.Error("un .parte senza token (quello del NAS) non deve essere riconosciuto")
	}
}
