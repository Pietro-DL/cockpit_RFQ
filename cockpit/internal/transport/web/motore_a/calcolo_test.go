package motore_a

// L1 — giro 5, A1d-Q2: il calcolo del servizio (direttiva A1d §3.1; piano P2-P3, «il calcolo» in 10 passi), senza
// database: la lettura e la valutazione sono sostituite con i due agganci non esportati del Servizio, che fuori da questo
// package non si vedono. Derivate dalle prove della PR #18 di Stefano Valo (67dcc02: TestMotoreACalcoloConLimite,
// TestMotoreALaVersioneDellEsito), con gli errori esportati e distinti di A1d-P2.
//   - regole non pronte: ErrRegole PRIMA di leggere (nessuna lettura, nessuna valutazione);
//   - la valutazione riceve le regole dell'avvio e NESSUN ingresso di scenario (Q-A1d-10 = B), anche se le regole hanno
//     gli ingressi del file dei casi;
//   - un panic è ErrPanic e il processo resta vivo; un calcolo che non finisce è ErrTempo entro la guardia; un errore di
//     lettura che arriva a limite scaduto è ErrTempo, una RFQ inesistente no (Q2-04 della revisione di A1d-P2); senza
//     limite nelle regole il calcolo ha il limite di ripiego;
//   - un errore del database è ErrLettura, distinto dagli altri;
//   - la versione: il letterale «valutazione-3» è la costante del core; un'altra versione è ErrVersione;
//   - un errore della valutazione è ErrValutazione; un esito senza il thread chiesto anche;
//   - il risultato porta la fotografia, l'esito, il thread, il confronto senza atteso, la fase e le dichiarazioni.
//
// I clienti di questi test sono inventati: la scena ACME dell'indice (indice_test.go).

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/valutazione"
)

// maRegolePronte: regole pronte con un insieme vuoto ma non nil, gli ingressi di un caso (che il calcolo non deve
// passare) e il limite dato.
func maRegolePronte(limite time.Duration) Regole {
	tid := maUUID(1)
	return Regole{Accesa: true, Stato: StatoPronta, Insieme: &motorea.InsiemeRegole{}, Limite: limite,
		Ingressi: valutazione.Ingressi{Versione: valutazione.VersioneCasi, Casi: []valutazione.IngressoCaso{
			{ID: "ACME-SCENARIO", ThreadID: &tid, ClienteID: maUUID(2), Autorita: "scenario"}}}}
}

// maServizioScena: un servizio che legge la scena ACME e la «valuta» restituendo l'esito della scena, con il contatore
// delle letture e delle valutazioni e gli ingressi che la valutazione ha ricevuto.
type maConta struct {
	letture, valutazioni int
	ingressi             valutazione.Ingressi
	regole               *motorea.InsiemeRegole
	scadenza             bool
}

func maServizioScena(r Regole, c *maConta) Servizio {
	f, e, _ := scenaIndiceACME()
	// un semaforo suo: le prove non dipendono dai calcoli orfani delle altre prove (limiti_test.go)
	return Servizio{Regole: r, posti: make(chan struct{}, MassimoCalcoli),
		leggi: func(ctx context.Context, thread uuid.UUID) (lettura, error) {
			c.letture++
			_, c.scadenza = ctx.Deadline()
			return lettura{Foto: f, Fase: &Fase{Nome: "FATTIBILITA"}}, nil
		},
		valuta: func(_ fotorfq.Fotografia, ins *motorea.InsiemeRegole, in valutazione.Ingressi) (valutazione.Esito, error) {
			c.valutazioni++
			c.ingressi, c.regole = in, ins
			return e, nil
		}}
}

func TestCalcolaRegoleNonPronteNonLegge(t *testing.T) {
	for _, r := range []Regole{
		{},
		{Accesa: true, Stato: StatoIndiceAssente},
		{Accesa: true, Stato: StatoIndiceNonValido},
		{Accesa: true, Stato: StatoNessunaGrammatica, Insieme: &motorea.InsiemeRegole{}},
		{Accesa: true, Stato: StatoPronta}, // pronta senza insieme: non è pronta
	} {
		var c maConta
		_, err := maServizioScena(r, &c).Calcola(context.Background(), maUUID(1))
		if !errors.Is(err, ErrRegole) || c.letture != 0 || c.valutazioni != 0 {
			t.Errorf("regole %+v: errore %v, %d letture, %d valutazioni; attesi ErrRegole e nessuna lettura", r.Stato, err, c.letture, c.valutazioni)
		}
	}
}

func TestCalcolaRiuscito(t *testing.T) {
	var c maConta
	r := maRegolePronte(20 * time.Second)
	s := maServizioScena(r, &c)
	s.Log = nil
	ris, err := s.Calcola(context.Background(), maUUID(1))
	if err != nil {
		t.Fatal(err)
	}
	if c.letture != 1 || c.valutazioni != 1 || !c.scadenza {
		t.Errorf("%d letture, %d valutazioni, scadenza nel contesto %v", c.letture, c.valutazioni, c.scadenza)
	}
	if len(c.ingressi.Casi) != 0 || c.ingressi.Versione != 0 {
		t.Errorf("la valutazione ha ricevuto gli ingressi %+v: la pagina non passa gli ingressi di scenario (Q-A1d-10 = B)", c.ingressi)
	}
	if c.regole != r.Insieme {
		t.Error("la valutazione non ha ricevuto le regole dell'avvio")
	}
	if ris.Thread == nil || ris.Thread.ID != maUUID(1) || ris.EsitoThread == nil || ris.EsitoThread.ThreadID != maUUID(1) {
		t.Fatalf("thread %+v, esito del thread %+v", ris.Thread, ris.EsitoThread)
	}
	if ris.Thread != &ris.Foto.Thread[0] || ris.EsitoThread != &ris.Esito.Thread[0] {
		t.Error("Thread ed EsitoThread non puntano dentro la fotografia e l'esito del risultato")
	}
	if ris.Fase == nil || ris.Fase.Nome != "FATTIBILITA" {
		t.Errorf("fase %+v", ris.Fase)
	}
	// il confronto è quello senza atteso: le stesse righe del passaggio, nil come atteso
	atteso := confronto.Confronta(inFiles(ris.EsitoThread.Confrontabili), inProdotti(ris.EsitoThread.ProdottiConfrontabili), nil)
	if ris.Confronto.Impronta != atteso.Impronta || len(ris.Confronto.Impronta) != 64 || len(ris.Confronto.ControAtteso) != 0 {
		t.Errorf("confronto %q (contro l'atteso %d), atteso %q", ris.Confronto.Impronta, len(ris.Confronto.ControAtteso), atteso.Impronta)
	}
	// regole senza limite (solo una prova: grammatica.Limiti.Valida rifiuta lo zero): il calcolo finisce, con il limite
	// di ripiego (R-05 della revisione di A1d-P2: mai senza scadenza)
	c = maConta{}
	if _, err := maServizioScena(maRegolePronte(0), &c).Calcola(context.Background(), maUUID(1)); err != nil || !c.scadenza {
		t.Errorf("senza limite nelle regole: %v, scadenza %v (attesa quella di ripiego)", err, c.scadenza)
	}
}

// La versione (direttiva §0): il letterale del package è la costante del core, e un esito di un'altra versione è
// ErrVersione con le due versioni nel testo (per il log).
func TestCalcolaLaVersioneDellEsito(t *testing.T) {
	letterale := "valutazione-3" // la direttiva A1d, §0
	if VersioneAttesa != letterale {
		t.Fatalf("VersioneAttesa %q, la direttiva vuole %q", VersioneAttesa, letterale)
	}
	if VersioneAttesa != valutazione.VersioneValutazione {
		t.Fatalf("VersioneAttesa %q, valutazione.VersioneValutazione %q: la pagina va riletta contro la versione nuova",
			VersioneAttesa, valutazione.VersioneValutazione)
	}
	var c maConta
	s := maServizioScena(maRegolePronte(time.Second), &c)
	vecchia := s.valuta
	s.valuta = func(f fotorfq.Fotografia, r *motorea.InsiemeRegole, in valutazione.Ingressi) (valutazione.Esito, error) {
		e, err := vecchia(f, r, in)
		e.VersioneValutazione = "valutazione-2"
		return e, err
	}
	_, err := s.Calcola(context.Background(), maUUID(1))
	if !errors.Is(err, ErrVersione) || !strings.Contains(err.Error(), "valutazione-2") || !strings.Contains(err.Error(), "valutazione-3") {
		t.Errorf("errore %v, atteso ErrVersione con le due versioni", err)
	}
}

func TestCalcolaGliErroriDistinti(t *testing.T) {
	errDB := errors.New("connessione persa")
	errContratto := errors.New("fotografia fuori contratto")
	libera := make(chan struct{})
	defer close(libera)
	for _, c := range []struct {
		nome    string
		limite  time.Duration
		leggi   func(ctx context.Context, thread uuid.UUID) (lettura, error)
		valuta  func(fotorfq.Fotografia, *motorea.InsiemeRegole, valutazione.Ingressi) (valutazione.Esito, error)
		atteso  error
		dentro  error
		piccolo bool
	}{
		{nome: "la RFQ non c'è", limite: time.Second, leggi: func(context.Context, uuid.UUID) (lettura, error) {
			return lettura{}, ErrRFQInesistente
		}, atteso: ErrRFQInesistente},
		{nome: "il database", limite: time.Second, leggi: func(context.Context, uuid.UUID) (lettura, error) {
			return lettura{}, fmt.Errorf("%w: la RFQ non si legge: %w", ErrLettura, errDB) // come leggiSnapshot
		}, atteso: ErrLettura, dentro: errDB},
		{nome: "la valutazione", limite: time.Second, valuta: func(fotorfq.Fotografia, *motorea.InsiemeRegole, valutazione.Ingressi) (valutazione.Esito, error) {
			return valutazione.Esito{}, errContratto
		}, atteso: ErrValutazione, dentro: errContratto},
		{nome: "un esito senza il thread", limite: time.Second, valuta: func(fotorfq.Fotografia, *motorea.InsiemeRegole, valutazione.Ingressi) (valutazione.Esito, error) {
			return valutazione.Esito{VersioneValutazione: VersioneAttesa}, nil
		}, atteso: ErrValutazione},
		{nome: "un panic nella lettura", limite: time.Second, leggi: func(context.Context, uuid.UUID) (lettura, error) {
			panic("lettura ACME rotta")
		}, atteso: ErrPanic},
		{nome: "un panic nella valutazione", limite: time.Second, valuta: func(fotorfq.Fotografia, *motorea.InsiemeRegole, valutazione.Ingressi) (valutazione.Esito, error) {
			panic("valutazione ACME rotta")
		}, atteso: ErrPanic},
		{nome: "un calcolo che non guarda il contesto", limite: 30 * time.Millisecond, valuta: func(fotorfq.Fotografia, *motorea.InsiemeRegole, valutazione.Ingressi) (valutazione.Esito, error) {
			<-libera
			return valutazione.Esito{}, nil
		}, atteso: ErrTempo, piccolo: true},
		{nome: "una lettura che risponde con il contesto scaduto", limite: 30 * time.Millisecond, leggi: func(ctx context.Context, _ uuid.UUID) (lettura, error) {
			<-ctx.Done()
			return lettura{}, ctx.Err()
		}, atteso: ErrTempo, piccolo: true},
		{nome: "una lettura del database interrotta dal limite", limite: 30 * time.Millisecond, leggi: func(ctx context.Context, _ uuid.UUID) (lettura, error) {
			<-ctx.Done()
			return lettura{}, fmt.Errorf("%w: la fase non si legge: %w", ErrLettura, ctx.Err())
		}, atteso: ErrTempo, piccolo: true},
	} {
		t.Run(c.nome, func(t *testing.T) {
			var conta maConta
			s := maServizioScena(maRegolePronte(c.limite), &conta)
			if c.leggi != nil {
				s.leggi = c.leggi
			}
			if c.valuta != nil {
				s.valuta = c.valuta
			}
			inizio := time.Now()
			_, err := s.Calcola(context.Background(), maUUID(1))
			if !errors.Is(err, c.atteso) || (c.dentro != nil && !errors.Is(err, c.dentro)) {
				t.Fatalf("errore %v, atteso %v", err, c.atteso)
			}
			if c.piccolo && time.Since(inizio) > 2*time.Second {
				t.Errorf("Calcola è tornato dopo %s: il limite non ha fermato l'attesa", time.Since(inizio))
			}
			for _, altro := range []error{ErrRegole, ErrRFQInesistente, ErrTempo, ErrPanic, ErrVersione, ErrValutazione, ErrLettura, ErrOccupato} {
				if altro != c.atteso && errors.Is(err, altro) {
					t.Errorf("errore %v: è anche %v (gli errori sono distinti)", err, altro)
				}
			}
			if errors.Is(err, ErrTempo) && !strings.Contains(err.Error(), c.limite.String()) {
				t.Errorf("il tempo scaduto non dice il limite: %v", err)
			}
		})
	}
	// la richiesta chiusa da chi l'ha fatta non è tempo scaduto
	ctx, cancel := context.WithCancel(context.Background())
	var conta maConta
	s := maServizioScena(maRegolePronte(time.Minute), &conta)
	s.leggi = func(ctx context.Context, _ uuid.UUID) (lettura, error) {
		cancel()
		<-ctx.Done()
		return lettura{}, ctx.Err()
	}
	if _, err := s.Calcola(ctx, maUUID(1)); errors.Is(err, ErrTempo) || !errors.Is(err, context.Canceled) {
		t.Errorf("richiesta annullata: %v", err)
	}
}

// Q2-04 della revisione di A1d-P2: l'esito che la goroutine consegna quando il limite è già scaduto (i due rami della
// select pronti insieme, o la consegna che vince il CompareAndSwap). Diventa ErrTempo solo un errore di lettura (ErrLettura
// o un errore che avvolge context.DeadlineExceeded); una RFQ che non c'è resta 404 e non 504; un panic, una versione o
// una valutazione sbagliate restano quello che sono (R-02); un risultato riuscito resta il risultato. Si prova la funzione
// stessa, con un contesto già scaduto: il caso intero dipende da quale ramo della select arriva prima, e non sarebbe
// ripetibile.
func TestCalcolaLEsitoConsegnatoALimiteScaduto(t *testing.T) {
	s := Servizio{Regole: maRegolePronte(30 * time.Millisecond)}
	ctx, fine := context.WithTimeout(context.Background(), time.Nanosecond)
	defer fine()
	<-ctx.Done()
	ris := &Risultato{}
	for _, c := range []struct {
		nome   string
		err    error
		atteso error
	}{
		{"la RFQ non c'è", ErrRFQInesistente, ErrRFQInesistente},
		{"un panic", fmt.Errorf("%w: rotto", ErrPanic), ErrPanic},
		{"una versione", fmt.Errorf("%w: valutazione-2", ErrVersione), ErrVersione},
		{"una valutazione", fmt.Errorf("%w: contratto", ErrValutazione), ErrValutazione},
		{"una lettura", fmt.Errorf("%w: la fase non si legge: connessione chiusa", ErrLettura), ErrTempo},
		{"il contesto", fmt.Errorf("lettura: %w", context.DeadlineExceeded), ErrTempo},
	} {
		r, err := s.esitoAlLimite(ctx, nil, c.err) // la goroutine consegna un risultato nil con un errore
		if !errors.Is(err, c.atteso) || r != nil {
			t.Errorf("%s: %v (risultato %v), atteso %v", c.nome, err, r, c.atteso)
		}
		if c.atteso != ErrTempo && errors.Is(err, ErrTempo) {
			t.Errorf("%s: %v è diventato tempo scaduto", c.nome, err)
		}
	}
	if r, err := s.esitoAlLimite(ctx, ris, nil); r != ris || err != nil {
		t.Errorf("un risultato riuscito a limite scaduto: %v, %v", r, err)
	}
	// a limite non scaduto un errore di lettura resta ErrLettura
	if _, err := s.esitoAlLimite(context.Background(), nil, fmt.Errorf("%w: x", ErrLettura)); !errors.Is(err, ErrLettura) || errors.Is(err, ErrTempo) {
		t.Errorf("una lettura entro il limite: %v", err)
	}
}
