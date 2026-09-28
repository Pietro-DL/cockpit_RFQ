// Package nas è l'unico scrittore del NAS: copia idempotente con file .parte.<token>, verifica hash,
// promozione esclusiva: mai sovrascrive.
package nas

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
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
	return Sha256Da(f)
}

// Sha256Da calcola l'hash di cio' che si legge da r, e dice quanti byte ha letto.
//
// Esiste separata da Sha256File perche' chi deve SERVIRE quei byte deve poterli verificare dallo
// stesso file che ha gia' aperto. Riaprire per nome vorrebbe dire verificarne uno e servirne un
// altro: fra le due aperture, su una condivisione di rete, il file puo' essere stato sostituito, e
// la verifica direbbe di si' a proposito di byte che nessuno mandera' mai a nessuno.
func Sha256Da(r io.Reader) (string, int64, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
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
//
// token e' il tentativo che scrive: il file intermedio si chiama <destinazione>.parte.<token> e nasce
// in modo esclusivo, cosi' due tentativi dello stesso job, uno scaduto e uno nuovo, non lo condividono
// mai (addendum A4.1). Gli esiti:
//   - la destinazione c'e' gia' con lo stesso hash: niente da fare;
//   - c'e' con un altro hash: ErrConflitto, e il file non si tocca;
//   - non c'e': copia nel .parte, verifica dell'hash, e promozione SENZA sostituzione (promuovi).
//
// creato dice se il file finale l'ha creato questa chiamata. E' la prova che uno spostamento scrivera'
// in nas_creazione (D38): un file trovato gia' li', anche con lo stesso hash, non e' nostro.
func (s *Scrittore) Copia(src, relativoThread, relativoDoc, shaAtteso string, token uuid.UUID) (dst string, creato bool, err error) {
	dst = UNC(s.Radice, strings.TrimRight(relativoThread, `\`)+`\`+relativoDoc)
	if s.DryRun {
		return dst, false, nil
	}
	if st, err := os.Stat(dst); err == nil && !st.IsDir() {
		return dst, false, giaPresente(dst, shaAtteso)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return dst, false, err
	}
	parte := dst + suffissoParte + token.String()
	if err := creaParte(src, parte); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			os.Remove(parte)
		}
		return dst, false, err
	}
	h, _, err := Sha256File(parte)
	if err != nil || h != shaAtteso {
		os.Remove(parte)
		if err == nil {
			err = fmt.Errorf("hash dopo copia %s ≠ atteso %s", h, shaAtteso)
		}
		return dst, false, err
	}
	creato, err = promuovi(parte, dst, shaAtteso)
	return dst, creato, err
}

// suffissoParte separa la destinazione dal token nel nome del file intermedio.
const suffissoParte = ".parte."

// giaPresente e' l'esito di una destinazione che esiste: lo stesso contenuto non e' un errore, un
// contenuto diverso e' un conflitto.
func giaPresente(dst, shaAtteso string) error {
	h, _, err := Sha256File(dst)
	if err != nil {
		return err
	}
	if h != shaAtteso {
		return fmt.Errorf("%w: %s", ErrConflitto, dst)
	}
	return nil
}

// creaParte copia src nel file intermedio, che deve essere nuovo: O_EXCL. Un .parte con lo stesso
// token vorrebbe dire lo stesso tentativo due volte, e non si tronca un file che qualcuno sta usando.
func creaParte(src, parte string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(parte, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// promuovi porta il .parte al suo nome definitivo senza mai sostituire un file comparso nel frattempo.
//
// os.Rename non basta: sostituisce la destinazione, su Windows come su Linux, e fra il controllo
// iniziale e la promozione un altro processo puo' averla creata. La promozione quindi e' esclusiva:
//   - un hard link (link(.parte, dst)), che fallisce se dst esiste; poi il .parte si toglie;
//   - dove il link non c'e' (certe condivisioni di rete), la creazione esclusiva del file finale
//     (O_CREATE|O_EXCL), la copia e la verifica.
//
// Se la destinazione e' comparsa, vale giaPresente: stesso hash, niente da fare; altro hash, conflitto.
func promuovi(parte, dst, shaAtteso string) (bool, error) {
	err := os.Link(parte, dst)
	if err == nil {
		os.Remove(parte)
		return true, nil
	}
	if errors.Is(err, fs.ErrExist) {
		os.Remove(parte)
		return false, giaPresente(dst, shaAtteso)
	}
	creato, err := copiaEsclusiva(parte, dst, shaAtteso)
	os.Remove(parte)
	return creato, err
}

// copiaEsclusiva e' il ripiego di promuovi: crea dst solo se non esiste, ci copia il .parte e lo
// verifica. Un file finale che non torna si toglie: l'ha creato questa chiamata, in modo esclusivo.
func copiaEsclusiva(parte, dst, shaAtteso string) (bool, error) {
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return false, giaPresente(dst, shaAtteso)
	}
	if err != nil {
		return false, err
	}
	in, err := os.Open(parte)
	if err == nil {
		_, err = io.Copy(out, in)
		in.Close()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = giaPresente(dst, shaAtteso)
	}
	if err != nil {
		os.Remove(dst)
		return false, err
	}
	return true, nil
}

// ------------------------------------------------------------------ i .parte.<token> scaduti (R2.12)

// Parte e' il file intermedio di un tentativo: <destinazione>.parte.<token>.
type Parte struct {
	Percorso   string
	Token      uuid.UUID
	Modificato time.Time
}

// TokenDiParte legge il token dal nome di un file intermedio. Un nome che non ha la forma
// <destinazione>.parte.<uuid> — un .parte senza token dei tentativi di prima di A4, un file
// dell'utente — non e' un file intermedio, e chi pulisce non lo tocca.
func TokenDiParte(nome string) (uuid.UUID, bool) {
	i := strings.LastIndex(nome, suffissoParte)
	if i <= 0 {
		return uuid.Nil, false
	}
	resto := nome[i+len(suffissoParte):]
	t, err := uuid.Parse(resto)
	if err != nil || len(resto) != 36 {
		return uuid.Nil, false
	}
	return t, true
}

// PartiNellaCartella elenca i file intermedi di una cartella (relativa alla radice).
func (s *Scrittore) PartiNellaCartella(relativa string) ([]Parte, error) {
	cartella := UNC(s.Radice, relativa)
	voci, err := os.ReadDir(cartella)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var parti []Parte
	for _, v := range voci {
		if v.IsDir() {
			continue
		}
		t, ok := TokenDiParte(v.Name())
		if !ok {
			continue
		}
		info, err := v.Info()
		if err != nil {
			continue
		}
		parti = append(parti, Parte{Percorso: filepath.Join(cartella, v.Name()), Token: t, Modificato: info.ModTime()})
	}
	return parti, nil
}

// PulisciParti toglie da una cartella i file intermedi scaduti. Servono DUE condizioni insieme: il
// token non e' fra i tentativi vivi, e il file e' piu' vecchio di limite (la durata massima di un
// tentativo di scrittura). La seconda copre il tentativo appena partito, il cui token non era ancora
// nell'elenco letto. Nessuna scansione di tutto il NAS: lo fa chi scrive in quella cartella, o chi la
// vuole togliere.
func (s *Scrittore) PulisciParti(relativa string, vivi map[uuid.UUID]bool, limite time.Time) (int, error) {
	if s.DryRun {
		return 0, nil
	}
	parti, err := s.PartiNellaCartella(relativa)
	if err != nil {
		return 0, err
	}
	tolti := 0
	for _, p := range parti {
		if vivi[p.Token] || p.Modificato.After(limite) {
			continue
		}
		if err := os.Remove(p.Percorso); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return tolti, err
		}
		tolti++
	}
	return tolti, nil
}

// RimuoviCartellaVuota toglie una cartella (relativa alla radice) solo se e' vuota: os.Remove, mai
// RemoveAll. Una cartella con dentro qualcosa, anche un .parte vivo, resta.
func (s *Scrittore) RimuoviCartellaVuota(relativa string) error {
	if s.DryRun {
		return nil
	}
	return os.Remove(UNC(s.Radice, relativa))
}

// Raggiungibile verifica l'accesso in lettura alla radice.
func (s *Scrittore) Raggiungibile() bool {
	st, err := os.Stat(s.Radice)
	return err == nil && st.IsDir()
}
