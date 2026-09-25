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
	return EstraiCon(zipPath, destDir, maxVoci, maxByteTotali)
}

// EstraiCon è Estrai con i limiti espliciti. I limiti sono parametri e non costanti perché una
// garanzia che non si può provare non è una garanzia: con le costanti, per arrivare al budget un test
// dovrebbe costruire un archivio da mezzo gigabyte, e nessuno lo farebbe.
func EstraiCon(zipPath, destDir string, limiteVoci int, limiteByte int64) ([]Voce, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("apri zip: %w", err)
	}
	defer r.Close()
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	var out []Voce
	// Il budget si misura sui byte EFFETTIVAMENTE SCRITTI, non su UncompressedSize64 (N10).
	// L'header di uno zip è un'affermazione di chi lo ha creato, non un fatto: una «zip bomb» dichiara
	// 1 KB e ne consegna 4 GB, e il controllo sull'header la lascia passare per intero prima di
	// accorgersene. Qui la scrittura viene interrotta appena il budget è finito, quindi il disco non
	// può essere riempito nemmeno da un archivio che mente.
	restante := limiteByte
	for i, f := range r.File {
		if f.FileInfo().IsDir() || ignora(f.Name) {
			continue
		}
		if len(out) >= limiteVoci {
			return out, ErrLimite
		}
		// zip-slip: il percorso pulito non deve uscire da destDir
		interno := path.Clean(strings.ReplaceAll(f.Name, `\`, "/"))
		if strings.HasPrefix(interno, "../") || path.IsAbs(interno) {
			continue
		}
		nome := path.Base(interno)
		dest := filepath.Join(destDir, fmt.Sprintf("%03d_%s", i+1, nomeSicuro(nome)))
		if !dentro(destDir, dest) {
			continue
		}
		n, sha, err := scrivi(f, dest, restante)
		if errors.Is(err, ErrLimite) {
			return out, ErrLimite
		}
		if err != nil {
			return out, fmt.Errorf("voce %s: %w", f.Name, err)
		}
		restante -= n
		out = append(out, Voce{PathInterno: interno, NomeFile: nome, Path: dest, Bytes: n, Sha256: sha})
	}
	return out, nil
}

// dentro dice se percorso sta davvero sotto radice (N11).
//
// Il confronto per prefisso di stringa sbagliava due volte: «C:\staging-vecchio\x» ha come prefisso
// «C:\staging» senza starci dentro, e su Windows «C:\Staging\x» non ha come prefisso «C:\staging»
// pur essendo lo stesso posto. filepath.Rel risponde alla domanda giusta — «che strada faccio da
// radice a percorso?» — e un percorso che esce comincia per «..».
func dentro(radice, percorso string) bool {
	rel, err := filepath.Rel(filepath.Clean(radice), filepath.Clean(percorso))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func ignora(nome string) bool {
	n := strings.ReplaceAll(nome, `\`, "/")
	base := path.Base(n)
	return strings.HasPrefix(n, "__MACOSX/") || strings.Contains(n, "/__MACOSX/") ||
		base == ".DS_Store" || base == "Thumbs.db" || base == "desktop.ini" || strings.HasPrefix(base, "._")
}

// scrivi copia una voce in dest consumando al massimo restante byte. Si legge un byte in più del
// budget apposta: se arriva, la voce è più grande di quanto resta e l'archivio va troncato. Il file
// parziale viene rimosso, così in staging non resta mai un file a metà con un hash che non è il suo.
func scrivi(f *zip.File, dest string, restante int64) (int64, string, error) {
	if restante <= 0 {
		return 0, "", ErrLimite
	}
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
	n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(rc, restante+1))
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > restante {
		err = ErrLimite
	}
	if err != nil {
		_ = os.Remove(dest)
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

var vietati = strings.NewReplacer("<", "_", ">", "_", ":", "_", `"`, "_", "/", "_", `\`, "_", "|", "_", "?", "_", "*", "_")

// nomeSicuro e' il nome con cui una voce si scrive nello staging. Si taglia a 120 CARATTERI, non byte: un
// nome con lettere accentate tagliato a byte poteva finire a meta' di un carattere, e il file nasceva con
// un nome che non e' UTF-8 valido.
func nomeSicuro(s string) string {
	s = strings.TrimSpace(vietati.Replace(s))
	if r := []rune(s); len(r) > 120 {
		s = string(r[:120])
	}
	if s == "" {
		return "voce"
	}
	return s
}
