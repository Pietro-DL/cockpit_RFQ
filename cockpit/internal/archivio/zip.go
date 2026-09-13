// Package archivio estrae gli archivi scaricati in staging (zip) in voci singole, così ogni file dentro
// lo zip diventa un allegato figlio con la sua proposta. 7z/rar non sono gestiti (restano "altro").
package archivio

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Voce è un file estratto dallo zip, già scritto in staging con l'hash calcolato.
type Voce struct {
	PathInterno string // percorso dentro lo zip, separatore '/'
	NomeFile    string
	Path        string // percorso assoluto in staging
	Bytes       int64
	Sha256      string
}

const (
	maxVoci       = 1000
	maxByteTotali = 500 << 20
)

var ErrLimite = errors.New("archivio oltre i limiti (1000 voci / 500 MB)")

// Estrai apre zipPath e scrive le voci in destDir (una sottocartella per voce, per non far collidere
// nomi uguali in cartelle diverse dello zip). Salta cartelle, __MACOSX e file nascosti di sistema.
func Estrai(zipPath, destDir string) ([]Voce, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("apri zip: %w", err)
	}
	defer r.Close()
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	var out []Voce
	var totale int64
	for i, f := range r.File {
		if f.FileInfo().IsDir() || ignora(f.Name) {
			continue
		}
		if len(out) >= maxVoci {
			return out, ErrLimite
		}
		totale += int64(f.UncompressedSize64)
		if totale > maxByteTotali {
			return out, ErrLimite
		}
		// zip-slip: il percorso pulito non deve uscire da destDir
		interno := path.Clean(strings.ReplaceAll(f.Name, `\`, "/"))
		if strings.HasPrefix(interno, "../") || path.IsAbs(interno) {
			continue
		}
		nome := path.Base(interno)
		dest := filepath.Join(destDir, fmt.Sprintf("%03d_%s", i+1, nomeSicuro(nome)))
		if !strings.HasPrefix(filepath.Clean(dest), filepath.Clean(destDir)+string(filepath.Separator)) {
			continue
		}
		n, sha, err := scrivi(f, dest)
		if err != nil {
			return out, fmt.Errorf("voce %s: %w", f.Name, err)
		}
		out = append(out, Voce{PathInterno: interno, NomeFile: nome, Path: dest, Bytes: n, Sha256: sha})
	}
	return out, nil
}

func ignora(nome string) bool {
	n := strings.ReplaceAll(nome, `\`, "/")
	base := path.Base(n)
	return strings.HasPrefix(n, "__MACOSX/") || strings.Contains(n, "/__MACOSX/") ||
		base == ".DS_Store" || base == "Thumbs.db" || base == "desktop.ini" || strings.HasPrefix(base, "._")
}

func scrivi(f *zip.File, dest string) (int64, string, error) {
	rc, err := f.Open()
	if err != nil {
		return 0, "", err
	}
	defer rc.Close()
	w, err := os.Create(dest)
	if err != nil {
		return 0, "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(rc, maxByteTotali))
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(dest)
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

var vietati = strings.NewReplacer("<", "_", ">", "_", ":", "_", `"`, "_", "/", "_", `\`, "_", "|", "_", "?", "_", "*", "_")

func nomeSicuro(s string) string {
	s = strings.TrimSpace(vietati.Replace(s))
	if len(s) > 120 {
		s = s[:120]
	}
	if s == "" {
		return "voce"
	}
	return s
}
