package motore_a

// L1 — giro 5, A1d-Q2, dopo la revisione di A1d-P2: i limiti delle risorse del servizio, senza database.
//   - R-01: il tetto dei calcoli. Al più MassimoCalcoli insieme, contando gli orfani (finiti oltre il limite ma non
//     ancora conclusi): oltre, ErrOccupato subito e senza leggere; quando un orfano finisce, il posto torna libero.
//   - R-02: l'esito di un calcolo che la richiesta non aspetta più va nel log: un panic (con la pila) o un errore a
//     livello Error, un calcolo riuscito tardi a Info.
//   - R-04: il ROLLBACK dello snapshot usa un contesto staccato dalla richiesta, vivo anche se la richiesta è chiusa,
//     con un suo limite breve.
//   - R-05, HTTP-03: Esiste, AnteprimaSTEP e Calcola hanno sempre una scadenza, anche con le regole senza limite
//     (LimiteDiRipiego); Esiste e AnteprimaSTEP al limite danno ErrTempo, non ErrLettura (HTTP-01).
//   - R-06: la composizione della pagina gira sotto lo stesso recover e lo stesso limite del calcolo.
//
// I clienti di questi test sono inventati: la scena ACME dell'indice (indice_test.go).

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/core/valutazione"
)

// maLogSicuro: un buffer per il log, scritto anche dalle goroutine orfane.
type maLogSicuro struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *maLogSicuro) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *maLogSicuro) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// maAspettaPostiLiberi: aspetta (al più 5 s) che il semaforo sia vuoto, cioè che gli orfani siano finiti.
func maAspettaPostiLiberi(t *testing.T, posti chan struct{}) {
	t.Helper()
	fine := time.Now().Add(5 * time.Second)
	for len(posti) > 0 {
		if time.Now().After(fine) {
			t.Fatalf("dopo 5 s il semaforo ha ancora %d posti presi", len(posti))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// R-01: il tetto dei calcoli, con gli orfani.
func TestCalcolaIlTettoDeiCalcoli(t *testing.T) {
	if MassimoCalcoli != 2 || cap(postiCalcolo) != MassimoCalcoli {
		t.Fatalf("MassimoCalcoli %d, semaforo del processo da %d: il commento di servizio.go dice due", MassimoCalcoli, cap(postiCalcolo))
	}
	if (Servizio{}).semaforo() != postiCalcolo {
		t.Fatal("un Servizio dell'adattatore non usa il semaforo del processo")
	}
	posti := make(chan struct{}, MassimoCalcoli)
	libera := make(chan struct{})
	var c maConta
	s := maServizioScena(maRegolePronte(20*time.Millisecond), &c)
	s.posti = posti
	vecchia := s.valuta
	s.valuta = func(f fotorfq.Fotografia, r *motorea.InsiemeRegole, in valutazione.Ingressi) (valutazione.Esito, error) {
		<-libera
		return vecchia(f, r, in)
	}
	// due calcoli oltre il limite: ErrTempo, ma restano orfani e tengono il posto
	for i := range MassimoCalcoli {
		if _, err := s.Calcola(context.Background(), maUUID(1)); !errors.Is(err, ErrTempo) {
			close(libera)
			t.Fatalf("calcolo %d: %v, atteso ErrTempo", i+1, err)
		}
	}
	if len(posti) != MassimoCalcoli {
		close(libera)
		t.Fatalf("%d posti presi con due orfani, attesi %d", len(posti), MassimoCalcoli)
	}
	// il terzo: ErrOccupato subito, senza leggere
	inizio := time.Now()
	_, err := s.Calcola(context.Background(), maUUID(1))
	if !errors.Is(err, ErrOccupato) || time.Since(inizio) > time.Second {
		t.Errorf("il terzo calcolo: %v dopo %s, atteso ErrOccupato subito", err, time.Since(inizio))
	}
	for _, altro := range []error{ErrRegole, ErrRFQInesistente, ErrTempo, ErrPanic, ErrVersione, ErrValutazione, ErrLettura} {
		if errors.Is(err, altro) {
			t.Errorf("ErrOccupato è anche %v", altro)
		}
	}
	// le regole non pronte vengono prima del semaforo: ErrRegole anche a semaforo pieno
	spente := s
	spente.Regole = Regole{}
	if _, err := spente.Calcola(context.Background(), maUUID(1)); !errors.Is(err, ErrRegole) {
		t.Errorf("regole non pronte a semaforo pieno: %v", err)
	}
	// gli orfani finiscono: i posti tornano liberi e il calcolo riparte
	close(libera)
	maAspettaPostiLiberi(t, posti)
	if _, err := s.Calcola(context.Background(), maUUID(1)); err != nil {
		t.Errorf("dopo gli orfani: %v", err)
	}
	if len(posti) != 0 {
		t.Errorf("un calcolo riuscito tiene ancora il posto (%d)", len(posti))
	}
}

// R-02: un panic, un errore o un successo arrivati quando la richiesta non aspetta più vanno nel log.
func TestCalcolaDopoIlLimiteNelLog(t *testing.T) {
	for _, c := range []struct {
		nome    string
		valuta  func() (valutazione.Esito, error)
		livello string
		dentro  string
	}{
		{"un panic", func() (valutazione.Esito, error) { panic("ACME-TARDI-PANIC") }, "level=ERROR", "ACME-TARDI-PANIC"},
		{"una valutazione sbagliata", func() (valutazione.Esito, error) { return valutazione.Esito{}, errors.New("ACME-TARDI-ERRORE") },
			"level=ERROR", "ACME-TARDI-ERRORE"},
		{"un calcolo riuscito", nil, "level=INFO", "non lo aspettava più"},
	} {
		t.Run(c.nome, func(t *testing.T) {
			var registro maLogSicuro
			var conta maConta
			libera := make(chan struct{})
			s := maServizioScena(maRegolePronte(20*time.Millisecond), &conta)
			s.Log = slog.New(slog.NewTextHandler(&registro, nil))
			vecchia := s.valuta
			s.valuta = func(f fotorfq.Fotografia, r *motorea.InsiemeRegole, in valutazione.Ingressi) (valutazione.Esito, error) {
				<-libera
				if c.valuta != nil {
					return c.valuta()
				}
				return vecchia(f, r, in)
			}
			if _, err := s.Calcola(context.Background(), maUUID(1)); !errors.Is(err, ErrTempo) {
				close(libera)
				t.Fatalf("%v, atteso ErrTempo", err)
			}
			close(libera)
			maAspettaPostiLiberi(t, s.posti)
			log := registro.String()
			riga := ""
			for _, r := range strings.Split(log, "\n") {
				if strings.Contains(r, "non lo aspettava più") || strings.Contains(r, "non aspettava più") {
					riga = r
				}
			}
			if riga == "" || !strings.Contains(riga, c.livello) || !strings.Contains(log, c.dentro) || !strings.Contains(riga, maUUID(1).String()) {
				t.Errorf("il log non dice l'esito arrivato dopo il limite (%s, %q):\n%s", c.livello, c.dentro, log)
			}
			if c.nome == "un panic" && !strings.Contains(log, "goroutine") {
				t.Errorf("il panic dopo il limite è nel log senza la pila:\n%s", log)
			}
		})
	}
	// un errore del contesto (la lettura interrotta dal limite) non è un difetto: nessuna riga Error
	var registro maLogSicuro
	var conta maConta
	s := maServizioScena(maRegolePronte(20*time.Millisecond), &conta)
	s.Log = slog.New(slog.NewTextHandler(&registro, nil))
	s.leggi = func(ctx context.Context, _ uuid.UUID) (lettura, error) {
		<-ctx.Done()
		time.Sleep(20 * time.Millisecond) // arriva quando la select ha già risposto
		return lettura{}, ctx.Err()
	}
	if _, err := s.Calcola(context.Background(), maUUID(1)); !errors.Is(err, ErrTempo) {
		t.Fatalf("%v, atteso ErrTempo", err)
	}
	maAspettaPostiLiberi(t, s.posti)
	if strings.Contains(registro.String(), "level=ERROR") {
		t.Errorf("una lettura interrotta dal limite è finita nel log come errore:\n%s", registro.String())
	}
}

// maRollbackTx: una transazione finta che registra il contesto del Rollback.
type maRollbackTx struct {
	maStepTx
	errCtx     error
	scadenza   bool
	rollbackDi int
}

func (x *maRollbackTx) Rollback(ctx context.Context) error {
	x.rollbackDi++
	x.errCtx = ctx.Err()
	_, x.scadenza = ctx.Deadline()
	return nil
}

// R-04: la richiesta chiusa fra la lettura e il ROLLBACK: il ROLLBACK riceve un contesto vivo, con una scadenza.
func TestIlRollbackNonUsaIlContestoDellaRichiesta(t *testing.T) {
	tx := &maRollbackTx{}
	ctx, chiudi := context.WithCancel(context.Background())
	err := nelloSnapshot(ctx, &maIniziatoreFisso{tx: tx}, func(pgx.Tx) error { chiudi(); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if tx.rollbackDi != 1 || tx.errCtx != nil || !tx.scadenza {
		t.Errorf("rollback %d, contesto del rollback %v, scadenza %v: attesi un rollback con un contesto vivo e una scadenza",
			tx.rollbackDi, tx.errCtx, tx.scadenza)
	}
}

// maIniziatoreFisso: un Iniziatore che dà la transazione data (per i finti con un Rollback proprio).
type maIniziatoreFisso struct{ tx pgx.Tx }

func (i *maIniziatoreFisso) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) { return i.tx, nil }

// maScadenza: un Iniziatore che registra se il contesto ha una scadenza, e quanto lontana; con blocca aspetta la fine
// del contesto.
type maScadenza struct {
	ha     bool
	entro  time.Duration
	blocca bool
}

func (i *maScadenza) BeginTx(ctx context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
	var d time.Time
	d, i.ha = ctx.Deadline()
	i.entro = time.Until(d)
	if i.blocca {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return nil, errors.New("finto: nessuna transazione")
}

// R-05, HTTP-03, HTTP-01: le letture hanno sempre una scadenza, e al limite dicono ErrTempo.
func TestLeLettureHannoSempreUnLimite(t *testing.T) {
	req := fascicolo.RichiestaAutorizzazione{Componente: maUUID(10)}
	// regole senza limite (indice assente: app/runtime non legge i limiti): il limite di ripiego
	for _, r := range []Regole{{}, {Accesa: true, Stato: StatoIndiceAssente}} {
		ini := &maScadenza{}
		_ = Servizio{Regole: r, Pool: ini}.Esiste(context.Background(), maUUID(1))
		if !ini.ha || ini.entro > LimiteDiRipiego || ini.entro < LimiteDiRipiego-5*time.Second {
			t.Errorf("Esiste con le regole %q: scadenza %v fra %s, attesa fra %s", r.Stato, ini.ha, ini.entro, LimiteDiRipiego)
		}
		ini = &maScadenza{}
		_, _ = Servizio{Regole: r, Pool: ini}.AnteprimaSTEP(context.Background(), maUUID(1), req, true)
		if !ini.ha || ini.entro > LimiteDiRipiego || ini.entro < LimiteDiRipiego-5*time.Second {
			t.Errorf("AnteprimaSTEP con le regole %q: scadenza %v fra %s, attesa fra %s", r.Stato, ini.ha, ini.entro, LimiteDiRipiego)
		}
	}
	if LimiteDiRipiego <= 0 || LimiteDiRipiego > 60*time.Second {
		t.Errorf("LimiteDiRipiego %s: deve stare sotto il tetto di 60 s dei limiti dell'indice", LimiteDiRipiego)
	}
	// con il limite delle regole, quello
	ini := &maScadenza{}
	_ = Servizio{Regole: maRegolePronte(3 * time.Second), Pool: ini}.Esiste(context.Background(), maUUID(1))
	if !ini.ha || ini.entro > 3*time.Second {
		t.Errorf("Esiste con il limite delle regole: scadenza %v fra %s", ini.ha, ini.entro)
	}
	// al limite: ErrTempo, non ErrLettura
	for _, chi := range []string{"Esiste", "AnteprimaSTEP"} {
		s := Servizio{Regole: maRegolePronte(20 * time.Millisecond), Pool: &maScadenza{blocca: true}}
		var err error
		if chi == "Esiste" {
			err = s.Esiste(context.Background(), maUUID(1))
		} else {
			_, err = s.AnteprimaSTEP(context.Background(), maUUID(1), req, true)
		}
		if !errors.Is(err, ErrTempo) || errors.Is(err, ErrLettura) || !strings.Contains(err.Error(), (20*time.Millisecond).String()) {
			t.Errorf("%s al limite: %v, atteso ErrTempo con il limite e non ErrLettura", chi, err)
		}
	}
	// la richiesta chiusa da chi l'ha fatta non è tempo scaduto
	ctx, chiudi := context.WithCancel(context.Background())
	chiudi()
	if err := (Servizio{Regole: maRegolePronte(time.Minute), Pool: &maScadenza{blocca: true}}).Esiste(ctx, maUUID(1)); errors.Is(err, ErrTempo) {
		t.Errorf("Esiste con la richiesta chiusa: %v", err)
	}
}

// R-06: un panic nella composizione è ErrPanic; una composizione oltre il limite è ErrTempo.
func TestPaginaLaComposizioneProtetta(t *testing.T) {
	var c maConta
	s := maServizioScena(maRegolePronte(time.Second), &c)
	s.componi = func(*Risultato, RichiestaPagina) *Pagina { panic("composizione ACME rotta") }
	if _, err := s.Pagina(context.Background(), RichiestaPagina{Thread: maUUID(1)}); !errors.Is(err, ErrPanic) ||
		!strings.Contains(err.Error(), "composizione ACME rotta") {
		t.Errorf("un panic nella composizione: %v, atteso ErrPanic", err)
	}
	libera := make(chan struct{})
	s = maServizioScena(maRegolePronte(20*time.Millisecond), &c)
	s.componi = func(r *Risultato, rp RichiestaPagina) *Pagina { <-libera; return ComponiPagina(r, rp) }
	if _, err := s.Pagina(context.Background(), RichiestaPagina{Thread: maUUID(1)}); !errors.Is(err, ErrTempo) {
		t.Errorf("una composizione oltre il limite: %v, atteso ErrTempo", err)
	}
	close(libera)
	maAspettaPostiLiberi(t, s.posti)
	// senza aggancio la composizione è ComponiPagina
	s = maServizioScena(maRegolePronte(time.Second), &c)
	p, err := s.Pagina(context.Background(), RichiestaPagina{Thread: maUUID(1), Vista: "rfq"})
	if err != nil || p == nil || p.Vista != VistaRFQ || p.Indice == nil || p.Esito == nil {
		t.Errorf("la pagina: %v, %+v", err, p)
	}
}
