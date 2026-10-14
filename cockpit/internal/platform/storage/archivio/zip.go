// Package archivio estrae gli archivi scaricati in staging (zip e 7z) in voci singole, così ogni file
// dentro l'archivio diventa un allegato figlio con la sua proposta. Il formato si riconosce dai primi
// byte, non dall'estensione: uno «.zip» che in realtà è un 7z si apre lo stesso. Il rar non è gestito
// (resta "altro").
package archivio

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bodgit/sevenzip"
)

// Voce è un file estratto dall'archivio, già scritto in staging con l'hash calcolato.
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

// ErrFormato: i primi byte non sono né uno zip né un 7z.
var ErrFormato = errors.New("formato d'archivio non riconosciuto (né zip né 7z)")

// I formati che si sanno aprire, come li dice Formato.
const (
	FormatoZip = "zip"
	Formato7z  = "7z"
)

// Estraibile dice se un'estensione (senza punto, minuscola o no) è di un archivio che il server apre.
// È l'unico posto che lo sa: chi decide se accodare l'estrazione, o se un file è un contenitore e non un
// documento, chiede qui invece di confrontare a mano con "zip".
func Estraibile(ext string) bool {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case FormatoZip, Formato7z:
		return true
	}
	return false
}

var (
	firmaZip      = []byte{'P', 'K', 0x03, 0x04}
	firmaZipVuoto = []byte{'P', 'K', 0x05, 0x06}
	firma7z       = []byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C}
)

// Formato legge i primi byte del file e dice se è uno zip o un 7z; ErrFormato altrimenti.
func Formato(percorso string) (string, error) {
	f, err := os.Open(percorso)
	if err != nil {
		return "", err
	}
	defer f.Close()
	testa := make([]byte, 6)
	n, _ := io.ReadFull(f, testa)
	testa = testa[:n]
	switch {
	case bytes.HasPrefix(testa, firma7z):
		return Formato7z, nil
	case bytes.HasPrefix(testa, firmaZip), bytes.HasPrefix(testa, firmaZipVuoto):
		return FormatoZip, nil
	}
	return "", ErrFormato
}

// voceArchivio: una voce come la vedono zip e 7z, con quel che serve al ciclo comune.
type voceArchivio struct {
	nome     string
	cartella bool
	apri     func() (io.ReadCloser, error)
	// scarta legge la voce fino in fondo senza scriverla (al più limite byte): serve al 7z, dove un blocco
	// compresso lasciato a metà tiene vivo il suo decodificatore (settezip.go). Nil per lo zip.
	scarta func(limite int64) error
}

// apriArchivio apre l'archivio nel suo formato e ne dà le voci in ordine, più la chiusura. Se i primi byte non
// sono né zip né 7z si prova comunque lo zip: archive/zip apre anche gli zip con dati in testa (autoestraenti),
// come prima di questa funzione.
func apriArchivio(percorso string) ([]voceArchivio, func() error, error) {
	formato, err := Formato(percorso)
	if err != nil && !errors.Is(err, ErrFormato) {
		return nil, nil, err
	}
	if formato == Formato7z {
		r, err := sevenzip.OpenReader(percorso)
		if err != nil {
			return nil, nil, fmt.Errorf("apri 7z: %w", err)
		}
		return vociSettezip(r), r.Close, nil
	}
	r, err := zip.OpenReader(percorso)
	if err != nil {
		if formato == "" {
			return nil, nil, fmt.Errorf("%w: %v", ErrFormato, err)
		}
		return nil, nil, fmt.Errorf("apri zip: %w", err)
	}
	voci := make([]voceArchivio, 0, len(r.File))
	for _, f := range r.File {
		voci = append(voci, voceArchivio{nome: f.Name, cartella: f.FileInfo().IsDir(), apri: f.Open})
	}
	return voci, r.Close, nil
}

// Estrai apre l'archivio (zip o 7z) e scrive le voci in destDir (una sottocartella per voce, per non far
// collidere nomi uguali in cartelle diverse dell'archivio). Salta cartelle, __MACOSX e file nascosti di
// sistema.
func Estrai(zipPath, destDir string) ([]Voce, error) {
	return EstraiCon(zipPath, destDir, maxVoci, maxByteTotali)
}

// EstraiCon è Estrai con i limiti espliciti. I limiti sono parametri e non costanti perché una
// garanzia che non si può provare non è una garanzia: con le costanti, per arrivare al budget un test
// dovrebbe costruire un archivio da mezzo gigabyte, e nessuno lo farebbe.
//
// Un panic del lettore (un archivio costruito apposta) diventa un errore: il job si chiude con «archivio non
// leggibile» invece di far cadere il server, perché l'estrazione gira nel suo processo.
func EstraiCon(zipPath, destDir string, limiteVoci int, limiteByte int64) (out []Voce, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("lettura dell'archivio interrotta: %v", p)
		}
	}()
	voci, chiudi, err := apriArchivio(zipPath)
	if err != nil {
		return nil, err
	}
	defer chiudi()
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	// Il budget si misura sui byte EFFETTIVAMENTE SCRITTI, non su UncompressedSize64 (N10).
	// L'header di uno zip è un'affermazione di chi lo ha creato, non un fatto: una «zip bomb» dichiara
	// 1 KB e ne consegna 4 GB, e il controllo sull'header la lascia passare per intero prima di
	// accorgersene. Qui la scrittura viene interrotta appena il budget è finito, quindi il disco non
	// può essere riempito nemmeno da un archivio che mente.
	restante := limiteByte
	for i, f := range voci {
		if f.cartella || ignora(f.nome) {
			if err := scartaVoce(f, restante); err != nil {
				return out, err
			}
			continue
		}
		if len(out) >= limiteVoci {
			return out, ErrLimite
		}
		// zip-slip: il percorso pulito non deve uscire da destDir
		interno := path.Clean(strings.ReplaceAll(f.nome, `\`, "/"))
		if strings.HasPrefix(interno, "../") || path.IsAbs(interno) {
			if err := scartaVoce(f, restante); err != nil {
				return out, err
			}
			continue
		}
		nome := path.Base(interno)
		dest := filepath.Join(destDir, fmt.Sprintf("%03d_%s", i+1, nomeSicuro(nome)))
		if !dentro(destDir, dest) {
			if err := scartaVoce(f, restante); err != nil {
				return out, err
			}
			continue
		}
		n, sha, err := scrivi(f.apri, dest, restante)
		if errors.Is(err, ErrLimite) {
			return out, ErrLimite
		}
		if err != nil {
			return out, fmt.Errorf("voce %s: %w", f.nome, err)
		}
		restante -= n
		out = append(out, Voce{PathInterno: interno, NomeFile: nome, Path: dest, Bytes: n, Sha256: sha})
	}
	return out, nil
}

// scartaVoce legge fino in fondo una voce che non si scrive, quando il formato lo chiede (7z). Il budget è
// quello dei byte che restano: una voce scartata enorme è un archivio oltre i limiti come una scritta.
func scartaVoce(f voceArchivio, restante int64) error {
	if f.scarta == nil || f.cartella {
		return nil
	}
	if err := f.scarta(restante); err != nil {
		if errors.Is(err, ErrLimite) {
			return ErrLimite
		}
		return fmt.Errorf("voce %s: %w", f.nome, err)
	}
	return nil
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
func scrivi(apri func() (io.ReadCloser, error), dest string, restante int64) (int64, string, error) {
	if restante <= 0 {
		return 0, "", ErrLimite
	}
	rc, err := apri()
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
