package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// I contesti di lavorazione del motore A per il banco e la QA (GP-1, direttiva 21 §1: «un modo esplicito di
// alimentarlo nel banco/QA, senza anticipare l'interfaccia completa del workspace»). Il manifest elenca i file con il
// loro sha256, come l'indice delle regole (R29 d: un file estraneo in una cartella non si carica in silenzio):
//
//	{"contesti": [{"file": "acme_rfq.contesto.json", "sha256": "…"}]}
//
// I file sono relativi alla cartella del manifest e non ne escono (filepath.IsLocal, os.Root). Ogni contesto si legge
// con motorea.LeggiContesto (decodifica stretta); il controllo contro il motore del cliente lo fa la valutazione
// (motorea.RisolviContesto), thread per thread.

type manifestContesti struct {
	Contesti []struct {
		File   string `json:"file"`
		Sha256 string `json:"sha256"`
	} `json:"contesti"`
}

// leggiContestiMotoreA: i contesti del manifest. Un errore per il manifest o per un file: nessun contesto (chi chiama
// lo dice nel log); un contesto a metà non si carica.
func leggiContestiMotoreA(manifest string) ([]motorea.ContestoLavorazione, error) {
	raw, err := os.ReadFile(manifest)
	if err != nil {
		return nil, fmt.Errorf("il manifest dei contesti non si legge: %w", err)
	}
	var m manifestContesti
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("manifest dei contesti non valido: %w", err)
	}
	root, err := os.OpenRoot(filepath.Dir(manifest))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	var out []motorea.ContestoLavorazione
	for _, v := range m.Contesti {
		nome, ok := nomeLocale(v.File)
		if !ok {
			return nil, fmt.Errorf("il contesto %q esce dalla cartella del manifest", v.File)
		}
		b, err := root.ReadFile(nome)
		if err != nil {
			return nil, fmt.Errorf("il contesto %q non si legge: %w", v.File, err)
		}
		if strings.TrimSpace(v.Sha256) == "" || !strings.EqualFold(jsoncanonico.Impronta(b), v.Sha256) {
			return nil, fmt.Errorf("il contesto %q ha uno sha256 diverso da quello del manifest", v.File)
		}
		c, err := motorea.LeggiContesto(b)
		if err != nil {
			return nil, fmt.Errorf("il contesto %q: %w", v.File, err)
		}
		out = append(out, c)
	}
	return out, nil
}
