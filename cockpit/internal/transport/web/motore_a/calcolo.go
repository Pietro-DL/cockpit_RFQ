package motore_a

// calcolo.go: il calcolo in sola lettura della Distinta nuova (giro 5, A1d-P2; direttiva A1d §3.1; piano P2-P3, «il
// calcolo» in 10 passi). Derivato dalla PR #18 di Stefano Valo (c543e89: calcolaConLimite, calcolaMotoreA), con lo
// snapshot unico (snapshot.go), gli errori esportati e senza gli ingressi di scenario.
//
// Il percorso è quello della direttiva: caricatore.Carica → valutazione.Calcola → la copia di passaggio →
// confronto.Confronta, senza atteso (nil: nessun atteso artificiale). Non salva niente, non accoda niente, non tocca
// le decisioni dell'operatore.
//
// Gli ingressi del file dei casi (Regole.Ingressi) NON entrano nel calcolo della pagina (Q-A1d-10 = B, raccomandazione
// della bozza da confermare; F-13): la pagina lavora sulla fotografia reale della RFQ e sulle grammatiche operative,
// senza i segmenti e l'autorità «scenario» del banco. Il banco continua a usare i suoi casi. Di conseguenza:
//   - per un thread che non ha un caso nel file, l'EsitoThread canonico è lo stesso del banco (esitoDelThread dipende
//     dal thread, dal suo cliente, dalle sezioni, dalle regole e dal caso del thread: valutazione/calcola.go);
//   - per un thread che ha un caso di scenario, la pagina e il banco possono differire, ed è una differenza voluta;
//   - Esito.Impronta e Esito.ImprontaFotografia sono sempre diverse da quelle del banco: la fotografia della pagina ha il
//     solo thread, nessun messaggio fuori RFQ e il solo cliente del thread (Richiesta senza Messaggi né Clienti);
//   - confronto.Esito.Impronta del thread è la stessa del banco quando l'EsitoThread è lo stesso.
//
// Il tempo: il contesto con il limite (Regole.Limite, o LimiteDiRipiego) copre la transazione e le letture (pgx lo
// guarda); valutazione.Calcola non riceve un contesto, quindi lettura, calcolo e composizione della pagina girano in una
// goroutine e il limite lo fa la select. Oltre il limite la risposta è ErrTempo e il risultato, se arriva, si butta: il
// calcolo orfano finisce da solo, in memoria e senza scritture (dichiarato, [T]); un suo errore o un suo panic arrivati
// dopo il limite vanno nel log (R-02 della revisione di A1d-P2). Un panic diventa ErrPanic, con la pila nel testo
// dell'errore (solo per il log). I calcoli insieme sono al più MassimoCalcoli, contando gli orfani (R-01): oltre,
// ErrOccupato prima di leggere.

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/core/valutazione"
)

// Risultato: ciò che un calcolo riuscito consegna. Thread ed EsitoThread puntano dentro Foto ed Esito (il thread
// chiesto); Fase e Dichiarazioni sono dello stesso snapshot della fotografia. V1: Regole sono le regole di lettura del
// thread sulla stessa fotografia e sulle stesse regole del calcolo (valutazione.RegoleDiLettura: il contesto con il
// perimetro, per l'adattatore del registro), V1 le letture di V1 dello stesso snapshot (dati_v1.go).
type Risultato struct {
	Foto          fotorfq.Fotografia
	Esito         valutazione.Esito
	Thread        *fotorfq.Thread
	EsitoThread   *valutazione.EsitoThread
	Confronto     confronto.Esito
	Fase          *Fase
	Dichiarazioni fascicolo.Dichiarazioni
	Regole        valutazione.RegoleDelThread
	V1            DatiV1
	// Insieme: le regole caricate all'avvio con cui il calcolo è fatto (l'identificativo verso il cliente, U2 §4.2).
	Insieme *motorea.InsiemeRegole
}

// Calcola: il calcolo di un thread, in sola lettura, con il limite del servizio.
//  1. Regole non pronte → ErrRegole, prima di tutto (nessuna lettura).
//  2. Il posto nel semaforo dei calcoli: senza un posto libero → ErrOccupato, subito e senza leggere.
//  3. Nella goroutine, che tiene il posto finché non finisce (anche oltre il limite): lo snapshot (GetThread, fase,
//     fotografia, dichiarazioni), poi valutazione.Calcola con le regole dell'avvio e nessun ingresso, il controllo della
//     versione, il passaggio e il confronto senza atteso.
//  4. La select: il risultato, oppure ErrTempo al limite. Un errore di lettura che arriva a limite già scaduto è anch'esso
//     ErrTempo (le letture di pgx rispondono con l'errore del contesto); ErrRFQInesistente resta una risposta, e un panic,
//     una versione o una valutazione sbagliate restano quello che sono (R-02).
//
// Una richiesta chiusa da chi l'ha fatta dà un errore che avvolge context.Canceled.
func (s Servizio) Calcola(ctx context.Context, thread uuid.UUID) (*Risultato, error) {
	return s.calcolaProtetto(ctx, thread, nil)
}

// Lo stato della consegna di un calcolo: chi arriva primo fra la goroutine (consegnato) e la select scaduta
// (abbandonato) lo decide con un CompareAndSwap, così un esito che arriva dopo il limite si sa e va nel log.
const (
	inAttesa int32 = iota
	consegnato
	abbandonato
)

// calcolaProtetto: Calcola con un passo in più, dopo, nella stessa goroutine e quindi sotto lo stesso limite e lo stesso
// recover (R-06 della revisione di A1d-P2: la composizione della pagina). dopo nil = nessun passo; dopo riceve il contesto
// con il limite (le letture che fa, come i suggerimenti di adesso di S9, stanno sotto lo stesso limite).
func (s Servizio) calcolaProtetto(ctx context.Context, thread uuid.UUID, dopo func(context.Context, *Risultato)) (*Risultato, error) {
	if !s.Regole.Pronta() {
		return nil, ErrRegole
	}
	posti := s.semaforo()
	select {
	case posti <- struct{}{}:
	default:
		return nil, fmt.Errorf("%w (al più %d insieme)", ErrOccupato, cap(posti))
	}
	ctx, cancel := s.conLimite(ctx)
	defer cancel()
	type uscita struct {
		r   *Risultato
		err error
	}
	fine := make(chan uscita, 1) // un posto: la goroutine finisce anche dopo il tempo scaduto
	var stato atomic.Int32
	inizio := time.Now()
	go func() {
		defer func() { <-posti }() // per ultimo: il posto si lascia quando il calcolo è finito davvero
		var u uscita
		// il recover sta qui dentro: un panic in una goroutine senza recupero fermerebbe il server
		defer func() {
			if v := recover(); v != nil {
				u = uscita{err: fmt.Errorf("%w: %v\n%s", ErrPanic, v, debug.Stack())}
			}
			if !stato.CompareAndSwap(inAttesa, consegnato) {
				s.dopoIlLimite(thread, time.Since(inizio), u.err)
				return
			}
			fine <- u
		}()
		u.r, u.err = s.calcola(ctx, thread)
		if u.err == nil && dopo != nil {
			dopo(ctx, u.r)
		}
	}()
	select {
	case u := <-fine:
		return s.esitoAlLimite(ctx, u.r, u.err)
	case <-ctx.Done():
		if !stato.CompareAndSwap(inAttesa, abbandonato) {
			// la goroutine ha consegnato nello stesso istante: il suo esito vale
			u := <-fine
			return s.esitoAlLimite(ctx, u.r, u.err)
		}
		if scaduto(ctx) {
			return nil, s.tempoScaduto(ctx, nil)
		}
		return nil, fmt.Errorf("motore A: richiesta annullata: %w", ctx.Err())
	}
}

// esitoAlLimite: l'esito consegnato dalla goroutine, quando il limite può essere già scaduto. Solo un errore di lettura
// (ErrLettura, o un errore che avvolge context.DeadlineExceeded) a limite scaduto diventa ErrTempo: le letture di pgx
// rispondono con l'errore del contesto. ErrRFQInesistente resta una risposta (404, non 504: Q2-04 della revisione di
// A1d-P2), e un panic, una versione o una valutazione sbagliate restano quello che sono (R-02).
func (s Servizio) esitoAlLimite(ctx context.Context, r *Risultato, err error) (*Risultato, error) {
	if err != nil && scaduto(ctx) && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrLettura)) {
		return nil, s.tempoScaduto(ctx, nil)
	}
	return r, err
}

// dopoIlLimite: l'esito di un calcolo che nessuno aspetta più (la richiesta ha già avuto ErrTempo o è stata chiusa). Un
// errore che non è del contesto (un panic con la sua pila, una versione, una valutazione) va nel log a livello Error:
// altrimenti un difetto sistematico sulle RFQ lente non si vedrebbe mai (R-02). Un calcolo riuscito tardi va a Info, con
// la durata: dice di quanto il limite dell'indice è stretto.
func (s Servizio) dopoIlLimite(thread uuid.UUID, durata time.Duration, err error) {
	switch {
	case err == nil:
		s.log().Info("motore A: un calcolo è finito quando la richiesta non lo aspettava più (limite scaduto o richiesta chiusa)", "thread", thread,
			"durata", durata, "limite", s.limite())
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
	default:
		s.log().Error("motore A: un calcolo che la richiesta non aspettava più non è riuscito", "thread", thread, "durata", durata,
			"limite", s.limite(), "err", err)
	}
}

// calcola: il passo 3 di Calcola, senza limite né recover.
func (s Servizio) calcola(ctx context.Context, thread uuid.UUID) (*Risultato, error) {
	leggi := s.leggi
	if leggi == nil {
		leggi = s.leggiSnapshot
	}
	l, err := leggi(ctx, thread)
	if err != nil {
		return nil, err
	}
	valuta := s.valuta
	if valuta == nil {
		valuta = valutazione.Calcola
	}
	// Nessun ingresso di scenario (Q-A1d-10 = B): vedi il commento di testa. Le conferme della categoria (V1, ondata C:
	// U2 §6.4.2) vengono dalle ultime conferme valide lette nella stessa fotografia (leggiV1), accanto al caricatore.
	// Le AR attive (direttiva 35, blocco C-4) sono la fonte «manuale» del Motore A, lette nella stessa fotografia dal
	// servizio unico (leggiV1, registrateDelThread): senza AR l'esito è quello di prima.
	e, err := valuta(l.Foto, s.Regole.Insieme, valutazione.Ingressi{Categorie: categorieDelThread(thread, l.V1),
		Registrate: registrateDelThread(thread, l.V1)})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrValutazione, err)
	}
	if e.VersioneValutazione != VersioneAttesa {
		return nil, fmt.Errorf("%w: l'esito è «%s», la pagina legge «%s»", ErrVersione, e.VersioneValutazione, VersioneAttesa)
	}
	r := &Risultato{Foto: l.Foto, Esito: e, Fase: l.Fase, Dichiarazioni: l.Dichiarazioni, V1: l.V1, Insieme: s.Regole.Insieme}
	r.Regole, _ = valutazione.RegoleDiLettura(r.Foto, s.Regole.Insieme, thread)
	for i := range r.Foto.Thread {
		if r.Foto.Thread[i].ID == thread {
			r.Thread = &r.Foto.Thread[i]
			break
		}
	}
	for i := range r.Esito.Thread {
		if r.Esito.Thread[i].ThreadID == thread {
			r.EsitoThread = &r.Esito.Thread[i]
			break
		}
	}
	if r.Thread == nil || r.EsitoThread == nil {
		return nil, fmt.Errorf("%w: la fotografia o l'esito non hanno il thread chiesto", ErrValutazione)
	}
	r.Confronto = confronto.Confronta(inFiles(r.EsitoThread.Confrontabili), inProdotti(r.EsitoThread.ProdottiConfrontabili), nil)
	return r, nil
}
