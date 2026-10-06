package testutil

// dataset.go: il dataset privato della consegna A nelle prove Go private (piano A, A1c, 3.3.9, 3.7.3 e 6.4.7;
// R24, R44, P-11). Le prove con il tag privato leggono il manifest, le grammatiche e il dump attraverso questi
// aiuti; gli attesi mai: li legge solo il runner del banco (bancoa.LeggiAttesi).

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"promatec/cockpit/internal/platform/dataset"
)

// Le variabili del dataset privato (piano A, 3.7.3). I nomi sono definitivi; i valori non stanno mai nel codice.
const (
	variabileDataset  = "COCKPIT_DATASET_A"
	variabileRapporti = "COCKPIT_RAPPORTI_A"
)

// errAttesiSoloAlRunner: una prova ha chiesto la voce degli attesi. Non è un dato che manca, è un difetto della
// prova: gli attesi li legge solo il runner (P-11).
var errAttesiSoloAlRunner = errors.New("gli attesi li legge solo il runner (bancoa.LeggiAttesi), mai una prova attraverso testutil (P-11)")

// DatasetA legge il manifest di COCKPIT_DATASET_A, con la decodifica stretta di platform/dataset. Senza la
// variabile, con un manifest dentro il modulo, che non si legge o che non rispetta il contratto: NON ESEGUITA,
// mai un salto.
func DatasetA(t testing.TB) dataset.Manifest {
	t.Helper()
	percorso := os.Getenv(variabileDataset)
	if percorso == "" {
		NonEseguita(t, variabileDataset+" non impostata: serve il percorso assoluto del manifest del dataset privato")
	}
	m, err := leggiManifest(percorso)
	if err != nil {
		NonEseguita(t, variabileDataset+": "+err.Error())
	}
	return m
}

// leggiManifest: il manifest dal disco, fuori dal modulo; i percorsi delle voci si leggono dalla sua cartella.
func leggiManifest(percorso string) (dataset.Manifest, error) {
	abs, err := filepath.Abs(percorso)
	if err != nil {
		return dataset.Manifest{}, fmt.Errorf("il percorso del manifest non si risolve: %w", err)
	}
	if err := dataset.FuoriDalModulo(abs); err != nil {
		return dataset.Manifest{}, err
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return dataset.Manifest{}, fmt.Errorf("il manifest non si legge: %w", err)
	}
	return dataset.Leggi(raw, filepath.Dir(abs))
}

// FileDelDataset dà i byte di una voce del manifest, dopo il controllo di sha256 e byte. La voce con il ruolo
// attesi si rifiuta con un fallimento della prova (P-11, E-10): non manca niente, la prova chiede ciò che non
// può avere. Senza la variabile, con un file mancante o con uno sha256 diverso dal manifest: NON ESEGUITA.
func FileDelDataset(t testing.TB, nome string) []byte {
	t.Helper()
	b, err := fileDelManifest(DatasetA(t), nome)
	switch {
	case errors.Is(err, errAttesiSoloAlRunner):
		t.Fatalf("FileDelDataset(%q): %v", nome, err)
	case err != nil:
		NonEseguita(t, fmt.Sprintf("voce %q del dataset: %v", nome, err))
	}
	return b
}

// fileDelManifest: la parte pura di FileDelDataset, sul manifest già letto.
func fileDelManifest(m dataset.Manifest, nome string) ([]byte, error) {
	for _, v := range m.Voci {
		if v.Nome == nome && v.Ruolo == dataset.RuoloAttesi {
			return nil, errAttesiSoloAlRunner
		}
	}
	return m.LeggiFile(nome)
}

// CartellaRapporti: la cartella delle uscite delle prove private (COCKPIT_RAPPORTI_A), che deve stare fuori dal
// modulo; senza la variabile, una cartella temporanea della prova. Una cartella dentro il modulo o che non si
// crea: NON ESEGUITA.
func CartellaRapporti(t testing.TB) string {
	t.Helper()
	cartella := os.Getenv(variabileRapporti)
	if cartella == "" {
		return t.TempDir()
	}
	if err := dataset.FuoriDalModulo(cartella); err != nil {
		NonEseguita(t, variabileRapporti+": "+err.Error())
	}
	if err := os.MkdirAll(cartella, 0o755); err != nil {
		NonEseguita(t, variabileRapporti+": la cartella non si crea: "+err.Error())
	}
	return cartella
}
