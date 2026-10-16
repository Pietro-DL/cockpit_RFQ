package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"promatec/cockpit/internal/platform/jsoncanonico"
)

// GP-1: i contesti di lavorazione del banco e della QA. Dati inventati.

const contestoProva = `{"versione_contesto": 1, "id": "ctx-acme", "rfq": "00000000-0000-4000-8000-000000000071",
 "cliente_id": "00000000-0000-4000-8000-000000000001", "stato": "dichiarato",
 "provenienza": {"autore": "qa", "data": "2026-10-17"},
 "partner": [{"emittente": "beta", "hash_snapshot": "aa"}],
 "perimetro": [{"sha256": "bb", "nome": "a.pdf"}]}`

func scriviManifest(t *testing.T, dir, sha string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "acme.contesto.json"), []byte(contestoProva), 0o644); err != nil {
		t.Fatal(err)
	}
	m := `{"contesti": [{"file": "acme.contesto.json", "sha256": "` + sha + `"}]}`
	p := filepath.Join(dir, "contesti.json")
	if err := os.WriteFile(p, []byte(m), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestGP1ContestiDalManifest(t *testing.T) {
	dir := t.TempDir()
	cs, err := leggiContestiMotoreA(scriviManifest(t, dir, jsoncanonico.Impronta([]byte(contestoProva))))
	if err != nil || len(cs) != 1 || cs[0].ID != "ctx-acme" {
		t.Fatalf("contesti: %v %+v", err, cs)
	}
	if _, err := leggiContestiMotoreA(scriviManifest(t, dir, strings.Repeat("0", 64))); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Errorf("uno sha256 diverso deve fermare il caricamento: %v", err)
	}
	fuori := filepath.Join(dir, "fuori.json")
	if err := os.WriteFile(fuori, []byte(`{"contesti": [{"file": "../x.json", "sha256": "aa"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := leggiContestiMotoreA(fuori); err == nil {
		t.Error("un contesto fuori dalla cartella del manifest non si legge")
	}
}

// L'avvio legge anche i file degli emittenti dell'indice (GP-1): senza, il partner sarebbe «file assente».
func TestGP1AvvioLeggeGliEmittenti(t *testing.T) {
	dir := t.TempDir()
	scrivi := func(nome, testo string) {
		if err := os.WriteFile(filepath.Join(dir, nome), []byte(testo), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	scrivi("cliente.json", `{}`)
	scrivi("beta.json", `{}`)
	scrivi("indice.json", `{"versione_indice": 1,
 "limiti": {"versione_limiti": "l", "grammatica": {"max_famiglie": 1, "max_forme_per_famiglia": 1, "max_parti_per_forma": 1, "max_esempi": 1, "max_lunghezza_pattern": 1, "max_ripetizione": 1, "max_lunghezza_letterale": 1},
  "riconoscimento": {"max_byte_unita": 1, "max_unita_documento": 1, "max_letture_per_unita": 1, "max_letture_documento": 1},
  "anteprima": {"tempo_massimo_ms": 1}},
 "grammatiche": [{"cliente_id": "00000000-0000-4000-8000-000000000001", "file": "cliente.json", "sha256": "aa"}],
 "emittenti": [{"id": "beta", "file": "beta.json", "sha256": "bb"}]}`)
	r, err := leggiRegoleMotoreA(filepath.Join(dir, "indice.json"))
	if err != nil {
		t.Fatalf("indice: %v", err)
	}
	if _, ok := r.Grammatiche["beta.json"]; !ok {
		t.Errorf("il file dell'emittente non è stato letto: %v", r.Grammatiche)
	}
}
