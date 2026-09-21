// Package nas è l'unico scrittore del NAS: copia idempotente con file .parte, verifica hash, mai sovrascrive.
package nas

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Scrittore struct {
	Radice string
	DryRun bool
}

var ErrConflitto = errors.New("file già presente con hash diverso")

// Sha256File calcola l'hash di un file.
func Sha256File(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// UNC compone radice + relativo per l'accesso reale al NAS; aggiunge il prefisso \\?\ quando il percorso
// supera i 260 caratteri (limite MAX_PATH di Windows). Per i percorsi di rete (\\server\share) il prefisso
// long-path è \\?\UNC\server\share.
func UNC(radice, relativo string) string {
	radice = strings.TrimRight(radice, `\`)
	p := radice + `\` + strings.TrimLeft(relativo, `\`)
	if len(p) >= 250 && !strings.HasPrefix(p, `\\?\`) {
		if strings.HasPrefix(p, `\\`) {
			return `\\?\UNC\` + strings.TrimPrefix(p, `\\`)
		}
		return `\\?\` + p
	}
	return p
}

// CreaCartella crea la cartella del thread (relativa alla radice) con la sottostruttura standard.
func (s *Scrittore) CreaCartella(relativa string, sottocartelle []string) (string, error) {
	base := UNC(s.Radice, relativa)
	if s.DryRun {
		return base, nil
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return base, err
	}
	for _, sc := range sottocartelle {
		if sc == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Join(base, sc), 0o755); err != nil {
			return base, err
		}
	}
	return base, nil
}

// Copia porta src in radice\relativoThread\relativoDoc verificando l'hash atteso.
// Se il file di destinazione esiste con lo stesso hash non fa nulla; con hash diverso restituisce ErrConflitto.
func (s *Scrittore) Copia(src, relativoThread, relativoDoc, shaAtteso string) (string, error) {
	dst := UNC(s.Radice, strings.TrimRight(relativoThread, `\`)+`\`+relativoDoc)
	if s.DryRun {
		return dst, nil
	}
	if st, err := os.Stat(dst); err == nil && !st.IsDir() {
		h, _, err := Sha256File(dst)
		if err != nil {
			return dst, err
		}
		if h == shaAtteso {
			return dst, nil
		}
		return dst, fmt.Errorf("%w: %s", ErrConflitto, dst)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return dst, err
	}
	parte := dst + ".parte"
	if err := copiaFile(src, parte); err != nil {
		os.Remove(parte)
		return dst, err
	}
	h, _, err := Sha256File(parte)
	if err != nil || h != shaAtteso {
		os.Remove(parte)
		if err == nil {
			err = fmt.Errorf("hash dopo copia %s ≠ atteso %s", h, shaAtteso)
		}
		return dst, err
	}
	if err := os.Rename(parte, dst); err != nil {
		os.Remove(parte)
		return dst, err
	}
	return dst, nil
}

func copiaFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Raggiungibile verifica l'accesso in lettura alla radice.
func (s *Scrittore) Raggiungibile() bool {
	st, err := os.Stat(s.Radice)
	return err == nil && st.IsDir()
}
