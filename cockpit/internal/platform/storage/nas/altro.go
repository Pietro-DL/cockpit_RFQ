package nas

// altro.go — le copie del cockpit in una cartella del NAS che e' anche delle persone (V1, U2 §3.2-§3.9, §3.11-§3.12:
// la cartella ALTRO della RFQ, dove stanno anche i documenti confermati di tipo «altro»). Il pacchetto non sa che cosa
// sia una RFQ ne' una riga di conservazione: riceve una cartella base, un percorso relativo e lo sha atteso, e fa solo
// operazioni sui file che non sostituiscono mai un file esistente.
//
// Le regole, tutte [T] di U2:
//   - nessuna sovrascrittura, mai: niente os.Rename (sostituisce la destinazione, su Windows come altrove); la
//     promozione del parziale e il ritorno indietro di una rimozione usano una rinomina senza sostituzione
//     (MoveFileW senza MOVEFILE_REPLACE_EXISTING su Windows, il link esclusivo altrove);
//   - il ripiego della copia diretta sul nome definitivo (copiaEsclusiva di nas.go) NON si usa qui: un'interruzione a
//     meta' lascerebbe nella cartella di una persona un file troncato con un nome credibile. Senza rinomina esclusiva
//     ne' link la copia non si fa (ErrPromozioneNonDisponibile);
//   - con DryRun (la capacita' nas_scrittura spenta) niente si scrive e niente si toglie: ErrScritturaSpenta, mai un
//     falso successo;
//   - la rimozione tocca solo il file indicato, solo se ha ancora lo sha atteso, e passa per un nome di servizio
//     privato (.<nome>.cockpit-rimozione.<token>, nascosto): un file cambiato da una persona torna al suo nome e resta.

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

// Posto e' dove sta una copia: la cartella base sotto la radice (per la cartella ALTRO la cartella della RFQ; vuota =
// la radice stessa) e il percorso relativo a lei (ALTRO\<nome>), con lo sha256 e la dimensione attesi. RadiceID e'
// l'impronta della radice in cui la copia e' stata scritta: una copia di un'altra radice non si tocca.
type Posto struct {
	RadiceID uuid.UUID
	Cartella string
	Percorso string
	Sha256   string
	Bytes    int64 // < 0 = non nota
}

// Gli errori delle copie. Le cause in parole (Causa) sono quelle delle righe di conservazione.
var (
	// ErrScritturaSpenta: DryRun, la capacita' nas_scrittura spenta. Niente e' stato scritto ne' tolto.
	ErrScritturaSpenta = errors.New("la scrittura sul NAS e' spenta su questo server")
	// ErrNomeOccupato: sul nome definitivo c'e' gia' un file (di chiunque): la copia non lo sostituisce.
	ErrNomeOccupato = errors.New("sul nome definitivo c'e' gia' un file")
	// ErrPromozioneNonDisponibile: ne' la rinomina senza sostituzione ne' il link esclusivo funzionano qui.
	ErrPromozioneNonDisponibile = errors.New("la promozione senza sostituzione non e' disponibile su questa cartella")
	// ErrCopiaMancante: al percorso registrato non c'e' nessun file.
	ErrCopiaMancante = errors.New("la copia non c'e' al percorso registrato")
	// ErrCopiaCambiata: al percorso registrato c'e' un file con un altro contenuto.
	ErrCopiaCambiata = errors.New("la copia al percorso registrato e' stata cambiata")
	// ErrRadiceDiversa: la copia e' stata scritta sotto un'altra radice.
	ErrRadiceDiversa = errors.New("la copia e' di un'altra radice")
	// ErrPercorsoNonValido: cartella o percorso con «..», due punti, una radice, o vuoti.
	ErrPercorsoNonValido = errors.New("percorso della copia non valido")
	// ErrParzialePresente: il parziale di questo token c'e' gia' (un altro tentativo con lo stesso token e' in corso, o
	// ne e' rimasto uno da un processo morto): non si tocca, la copia non si fa.
	ErrParzialePresente = errors.New("il parziale di questo tentativo c'e' gia'")
)

// Conservatore e' lo scrittore delle copie del cockpit sotto una radice (il NAS in produzione, una cartella temporanea
// nelle prove). DryRun spegne ogni scrittura.
type Conservatore struct {
	Radice string
	DryRun bool
	// Scrittura: la capacita' nas_scrittura letta al momento del gesto (U2 §3.5: S3 controlla la capacita' E il DryRun).
	// Nil = accesa (le prove); in produzione la cabla app/runtime.
	Scrittura func() bool
	// Promozione: solo per le prove (la rinomina esclusiva sostituita da un errore, per provare che non c'e' ripiego).
	Promozione func(parte, dst string) error
}

// RadiceID e' l'impronta della radice normalizzata (senza maiuscole, barre uniformi, senza la barra finale): due
// configurazioni con la stessa radice scritta in modo diverso hanno la stessa impronta.
func (c *Conservatore) RadiceID() uuid.UUID { return ImprontaRadice(c.Radice) }

// ImprontaRadice: l'impronta di una radice normalizzata.
func ImprontaRadice(radice string) uuid.UUID {
	r := strings.ReplaceAll(strings.TrimSpace(radice), "/", `\`)
	for len(r) > 3 && strings.HasSuffix(r, `\`) {
		r = strings.TrimSuffix(r, `\`)
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("cockpit:altro:"+strings.ToLower(r)))
}

// Disponibile dice se le copie si possono scrivere ora: la radice configurata e la scrittura accesa.
func (c *Conservatore) Disponibile() (bool, string) {
	switch {
	case c == nil || strings.TrimSpace(c.Radice) == "":
		return false, "nas"
	case c.DryRun, c.Scrittura != nil && !c.Scrittura():
		return false, "nas_scrittura"
	}
	return true, ""
}

// Cartelle e' lo scrittore delle cartelle sotto la stessa radice, con la stessa scrittura: crea la cartella di una RFQ
// che non c'e' ancora (documenti.CreaCartellaThread, U2 §3.2 e U2-P07), mai altro.
func (c *Conservatore) Cartelle() *Scrittore {
	ok, _ := c.Disponibile()
	return &Scrittore{Radice: c.Radice, DryRun: !ok}
}

// pezzi controlla una parte di percorso relativo: nessun pezzo vuoto, «.», «..», nessun due punti (niente volumi ne'
// flussi alternativi NTFS), niente radice. Restituisce i pezzi.
func pezzi(rel string, vuotoOk bool) ([]string, error) {
	rel = strings.ReplaceAll(strings.TrimSpace(rel), "/", `\`)
	if rel == "" {
		if vuotoOk {
			return nil, nil
		}
		return nil, ErrPercorsoNonValido
	}
	if strings.HasPrefix(rel, `\`) || strings.Contains(rel, ":") {
		return nil, ErrPercorsoNonValido
	}
	out := strings.Split(strings.TrimSuffix(rel, `\`), `\`)
	for _, p := range out {
		if p == "" || p == "." || p == ".." || strings.TrimRight(p, ". ") == "" {
			return nil, ErrPercorsoNonValido
		}
	}
	return out, nil
}

// percorsi: la cartella base e il file del posto, assoluti, contenuti per costruzione (i pezzi sono controllati).
func (c *Conservatore) percorsi(p Posto) (base, file string, err error) {
	if strings.TrimSpace(c.Radice) == "" {
		return "", "", ErrPercorsoNonValido
	}
	cart, err := pezzi(p.Cartella, true)
	if err != nil {
		return "", "", err
	}
	rel, err := pezzi(p.Percorso, false)
	if err != nil {
		return "", "", err
	}
	base = UNC(c.Radice, strings.Join(cart, `\`))
	if len(cart) == 0 {
		base = strings.TrimRight(c.Radice, `\/`)
		if filepath.VolumeName(base) == base {
			base += `\`
		}
	}
	file = UNC(c.Radice, strings.Join(append(cart, rel...), `\`))
	return base, file, nil
}

// controllaRadice: una copia di un'altra radice non si legge ne' si tocca.
func (c *Conservatore) controllaRadice(p Posto) error {
	if p.RadiceID != c.RadiceID() {
		return ErrRadiceDiversa
	}
	return nil
}

// EsitoCopia e' l'esito di Copia: Creato = il file definitivo l'ha creato questa chiamata (la prova di creazione da
// registrare); Causa = la causa di un errore, nelle parole delle righe di conservazione (spazio, permessi,
// sorgente_illeggibile, verifica_sha, conflitto, nas_scrittura); GiaPresenteStessoSha = sul nome c'era gia' un file con
// lo stesso contenuto (per la ripresa di chi chiama: non e' una prova di creazione).
type EsitoCopia struct {
	Creato               bool
	Causa                string
	GiaPresenteStessoSha bool
	Bytes                int64
}

// lettore tiene l'errore di lettura della sorgente, per distinguerlo da un errore di scrittura.
type lettore struct {
	r   io.Reader
	err error
}

func (l *lettore) Read(b []byte) (int, error) {
	n, err := l.r.Read(b)
	if err != nil && err != io.EOF {
		l.err = err
	}
	return n, err
}

// Copia scrive i byte di src nel definitivo del posto (U2 §3.3, passi 2 e 3): il parziale <definitivo>.parte.<token> in
// creazione esclusiva, lo sha calcolato scrivendo, Sync, il confronto con lo sha atteso, poi la promozione senza
// sostituzione. La cartella base deve esistere; la cartella del file (ALTRO) si crea se manca, un livello solo. Un
// file gia' presente sul nome definitivo, prima o al momento della promozione, e' ErrNomeOccupato: non si sostituisce
// mai. Ogni errore toglie il parziale di questa chiamata.
func (c *Conservatore) Copia(p Posto, token uuid.UUID, src io.Reader) (EsitoCopia, error) {
	if ok, motivo := c.Disponibile(); !ok {
		return EsitoCopia{Causa: motivo}, ErrScritturaSpenta
	}
	if err := c.controllaRadice(p); err != nil {
		return EsitoCopia{Causa: "radice_cambiata"}, err
	}
	base, dst, err := c.percorsi(p)
	if err != nil {
		return EsitoCopia{Causa: "fuori_cartella"}, err
	}
	if st, err := os.Stat(base); err != nil || !st.IsDir() {
		if err == nil {
			err = fmt.Errorf("%s non e' una cartella", base)
		}
		return EsitoCopia{Causa: "cartella_assente"}, err
	}
	if esito, presente, err := c.giaSulNome(dst, p.Sha256); presente {
		return esito, err
	}
	dir := filepath.Dir(dst)
	if !stessaCartella(dir, base) && !stessaCartella(filepath.Dir(dir), base) {
		return EsitoCopia{Causa: "fuori_cartella"}, ErrPercorsoNonValido
	}
	if err := os.Mkdir(dir, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return EsitoCopia{Causa: causaScrittura(err)}, err
	}
	parte := dst + suffissoParte + token.String()
	out, err := os.OpenFile(parte, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return EsitoCopia{Causa: "parziale_presente"}, ErrParzialePresente
	}
	if err != nil {
		return EsitoCopia{Causa: causaScrittura(err)}, err
	}
	h := sha256.New()
	l := &lettore{r: src}
	n, errCopia := io.Copy(io.MultiWriter(out, h), l)
	errSync := out.Sync()
	errChiudi := out.Close()
	fallita := func(causa string, err error) (EsitoCopia, error) {
		_ = os.Remove(parte)
		return EsitoCopia{Causa: causa, Bytes: n}, err
	}
	switch {
	case l.err != nil:
		return fallita("sorgente_illeggibile", l.err)
	case errCopia != nil:
		return fallita(causaScrittura(errCopia), errCopia)
	case errSync != nil:
		return fallita(causaScrittura(errSync), errSync)
	case errChiudi != nil:
		return fallita(causaScrittura(errChiudi), errChiudi)
	}
	if hex.EncodeToString(h.Sum(nil)) != strings.ToLower(p.Sha256) {
		return fallita("verifica_sha", errors.New("lo sha256 dei byte copiati non e' quello atteso"))
	}
	if p.Bytes >= 0 && n != p.Bytes {
		return fallita("verifica_sha", fmt.Errorf("copiati %d byte, attesi %d", n, p.Bytes))
	}
	promuovi := c.Promozione
	if promuovi == nil {
		promuovi = rinominaSenzaSostituire
	}
	if err := promuovi(parte, dst); err != nil {
		if errors.Is(err, fs.ErrExist) {
			esito, _, errPresente := c.giaSulNome(dst, p.Sha256)
			_ = os.Remove(parte)
			esito.Bytes = n
			return esito, errPresente
		}
		if errors.Is(err, ErrPromozioneNonDisponibile) {
			return fallita("permessi", err)
		}
		return fallita(causaScrittura(err), err)
	}
	return EsitoCopia{Creato: true, Bytes: n}, nil
}

// giaSulNome: se sul nome definitivo c'e' gia' un file, l'esito del conflitto (con lo stesso sha o no).
func (c *Conservatore) giaSulNome(dst, sha string) (EsitoCopia, bool, error) {
	st, err := os.Lstat(dst)
	if err != nil {
		return EsitoCopia{}, false, nil
	}
	e := EsitoCopia{Causa: "conflitto"}
	if st.Mode().IsRegular() {
		if h, _, err := Sha256File(dst); err == nil && h == strings.ToLower(sha) {
			e.GiaPresenteStessoSha = true
		}
	}
	return e, true, ErrNomeOccupato
}

// stessaCartella: due percorsi di cartella uguali senza maiuscole ne' barra finale.
func stessaCartella(a, b string) bool {
	n := func(p string) string { return strings.ToLower(strings.TrimRight(filepath.Clean(p), `\/`)) }
	return n(a) == n(b)
}

// causaScrittura: la causa di un errore di scrittura, nelle parole delle righe di conservazione.
func causaScrittura(err error) string {
	switch {
	case errors.Is(err, os.ErrPermission):
		return "permessi"
	case spazioEsaurito(err):
		return "spazio"
	}
	return "permessi"
}

// Leggi apre la copia in sola lettura, senza verificarla: chi legge calcola lo sha mentre copia (la «Riattiva» e la
// copia da un'altra copia lo confrontano alla fine). La dimensione registrata, se nota, deve tornare.
func (c *Conservatore) Leggi(p Posto) (*os.File, error) {
	if err := c.controllaRadice(p); err != nil {
		return nil, err
	}
	_, file, err := c.percorsi(p)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrCopiaMancante
	}
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || (p.Bytes >= 0 && st.Size() != p.Bytes) {
		f.Close()
		if err == nil {
			err = ErrCopiaCambiata
		}
		return nil, err
	}
	return f, nil
}

// Apri apre la copia per servirla (S5): lo sha256 si verifica dallo stesso handle con cui poi si servono i byte, e il
// file torna all'inizio. ErrCopiaMancante, ErrCopiaCambiata: mai byte diversi con il nome e lo sha di prima.
func (c *Conservatore) Apri(p Posto) (*os.File, os.FileInfo, error) {
	f, err := c.Leggi(p)
	if err != nil {
		return nil, nil, err
	}
	h, _, err := Sha256Da(f)
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if h != strings.ToLower(p.Sha256) {
		f.Close()
		return nil, nil, ErrCopiaCambiata
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return f, st, nil
}

// StatoCopia e' lo stato di una copia visto dal disco.
type StatoCopia string

const (
	CopiaPresente   StatoCopia = "presente"
	CopiaCambiata   StatoCopia = "cambiata"
	CopiaNonTrovata StatoCopia = "non_trovata"
	CopiaEstranea   StatoCopia = "estranea" // di un'altra radice: non si guarda
)

// Stato guarda la copia: con conSha falso solo la dimensione (il giro frequente del custode, la cache), con conSha vero
// anche il contenuto.
func (c *Conservatore) Stato(p Posto, conSha bool) (StatoCopia, error) {
	if c.controllaRadice(p) != nil {
		return CopiaEstranea, nil
	}
	_, file, err := c.percorsi(p)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(file)
	if errors.Is(err, fs.ErrNotExist) {
		return CopiaNonTrovata, nil
	}
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() || (p.Bytes >= 0 && st.Size() != p.Bytes) {
		return CopiaCambiata, nil
	}
	if conSha {
		h, _, err := Sha256File(file)
		if err != nil {
			return "", err
		}
		if h != strings.ToLower(p.Sha256) {
			return CopiaCambiata, nil
		}
	}
	return CopiaPresente, nil
}

// PulisciPartiCopie toglie dalla cartella del posto i parziali <nome>.parte.<token> scaduti: il token non e' fra i vivi
// (l'unione dei tentativi di chi scrive in quella cartella: U2 §3.9) E il file e' piu' vecchio di limite. Un nome di
// servizio della rimozione non e' un parziale e non si tocca.
func (c *Conservatore) PulisciPartiCopie(p Posto, vivi map[uuid.UUID]bool, limite time.Time) (int, error) {
	if c.DryRun {
		return 0, nil
	}
	_, file, err := c.percorsi(p)
	if err != nil {
		return 0, err
	}
	s := &Scrittore{Radice: filepath.Dir(file)}
	return s.PulisciParti("", vivi, limite)
}
