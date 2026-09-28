// Package logfile scrive il log del server su un file che si rinnova da solo.
//
// I worker hanno sempre avuto il loro (`<staging>/log/worker_outlook.log`, RotatingFileHandler);
// cockpit.exe no: scriveva solo sullo stdout della finestra che lo aveva avviato. Finché la finestra
// resta aperta si legge, ma basta chiuderla, o avviare il server in un altro modo, e del pomeriggio
// non resta niente. Il 15/09/2026 questo è costato la metà della diagnosi: del difetto si vedeva
// solo il lato worker, e il lato server — chi ha risposto 400, a chi, quante volte — era già
// scomparso.
//
// Non è una libreria di logging: è un io.Writer che si rinnova quando il file supera una soglia.
// Lo slog del server ci scrive dentro insieme allo stdout (io.MultiWriter), così la finestra resta
// leggibile e il file resta.
package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Predefiniti uguali a quelli dei worker Python (5 file da 5 MB): un log del server che occupa più
// dei suoi worker sarebbe una sorpresa, e il confronto fra i due si fa a occhio.
const (
	MaxByteDefault = 5 << 20
	CopieDefault   = 5
)

// Scrittore è un file di log che ruota: cockpit.log → cockpit.log.1 → … → cockpit.log.<copie>.
// È sicuro da usare da più goroutine: slog non serializza le scritture al posto suo.
type Scrittore struct {
	mu       sync.Mutex
	percorso string
	maxByte  int64
	copie    int
	f        *os.File
	scritti  int64
	// riprovaA: dopo una rotazione non riuscita, la dimensione del file corrente oltre la quale si
	// riprova. Zero = nessun rinvio, si ruota appena si supera maxByte.
	riprovaA int64
	// rinomina e' os.Rename; e' un campo perche' la prova possa simulare il file tenuto aperto da un
	// altro processo, che su Windows non si lascia spostare.
	rinomina func(da, a string) error
}

// Apri prepara il file (creando la cartella) e si posiziona in coda a quello che c'è già: un
// riavvio del server non deve cancellare il log del tentativo precedente, che di solito è proprio
// quello che si sta cercando.
func Apri(percorso string, maxByte int64, copie int) (*Scrittore, error) {
	if percorso == "" {
		return nil, fmt.Errorf("percorso del log vuoto")
	}
	if maxByte <= 0 {
		maxByte = MaxByteDefault
	}
	if copie < 0 {
		copie = 0
	}
	if err := os.MkdirAll(filepath.Dir(percorso), 0o755); err != nil {
		return nil, err
	}
	s := &Scrittore{percorso: percorso, maxByte: maxByte, copie: copie, rinomina: os.Rename}
	if err := s.apri(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Scrittore) apri() error {
	f, err := os.OpenFile(s.percorso, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	s.f, s.scritti = f, info.Size()
	return nil
}

func (s *Scrittore) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		if err := s.apri(); err != nil {
			return 0, err
		}
	}
	// Si ruota PRIMA di scrivere e solo se il file ha già qualcosa dentro: una singola riga più
	// lunga della soglia finisce comunque intera in un file, perché una riga di log spezzata in due
	// file è peggio di un file un po' più grande del previsto.
	if s.scritti > 0 && s.scritti+int64(len(p)) > max(s.maxByte, s.riprovaA) {
		if err := s.ruota(); err != nil {
			return 0, err
		}
	}
	n, err := s.f.Write(p)
	s.scritti += int64(n)
	return n, err
}

// Percorso è dove sta scrivendo adesso: serve per dirlo nel log d'avvio.
func (s *Scrittore) Percorso() string { return s.percorso }

func (s *Scrittore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return nil
	}
	f := s.f
	s.f = nil
	return f.Close()
}

// ruota fa scalare le copie e riparte da un file vuoto. Con copie = 0 il file viene solo troncato.
//
// Il file corrente si toglie di mezzo PER PRIMO, con una rinomina, e solo se ci riesce si toccano le
// copie vecchie. Prima l'ordine era l'inverso: su Windows il file corrente non si sposta se un altro
// processo lo tiene aperto (un editor, un antivirus, un `Get-Content -Wait`), e intanto le copie erano gia'
// scalate e la piu' vecchia cancellata; la scrittura falliva, e alla riga dopo la rotazione ripartiva da
// capo, cancellando un'altra copia. In pochi secondi restava un solo file, e il log si fermava.
//
// Adesso, se il file corrente non si sposta, si continua a scriverci in coda — il file cresce oltre la
// soglia, che e' il danno minore — e si riprova quando e' cresciuto di un altro decimo della soglia:
// riprovare a ogni riga vorrebbe dire chiudere e riaprire il file a ogni scrittura. Le copie vecchie
// restano dove sono. Un intoppo sulle copie vecchie, a file corrente gia' spostato, non ferma il log:
// al peggio si perde una copia vecchia.
func (s *Scrittore) ruota() error {
	if err := s.f.Close(); err != nil {
		return err
	}
	s.f = nil
	parcheggio := s.percorso + ".ruota"
	if err := s.rinomina(s.percorso, parcheggio); err != nil && !os.IsNotExist(err) {
		passo := max(s.maxByte/10, 1)
		if err := s.apri(); err != nil {
			return err
		}
		s.riprovaA = s.scritti + passo
		return nil
	}
	s.riprovaA = 0
	if s.copie == 0 {
		_ = os.Remove(parcheggio)
		return s.apri()
	}
	// la più vecchia esce di scena, le altre scalano di uno: .4 → .5, .3 → .4, …, log → .1
	_ = os.Remove(s.nome(s.copie))
	for i := s.copie - 1; i >= 1; i-- {
		_ = s.rinomina(s.nome(i), s.nome(i+1))
	}
	_ = s.rinomina(parcheggio, s.nome(1))
	return s.apri()
}

func (s *Scrittore) nome(i int) string { return fmt.Sprintf("%s.%d", s.percorso, i) }
