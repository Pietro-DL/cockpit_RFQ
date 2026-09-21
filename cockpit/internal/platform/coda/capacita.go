package coda

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync/atomic"

	"promatec/cockpit/internal/platform/db"
)

// CAPACITÀ DI SCRITTURA, non un interruttore solo (blocco 4 del checkpoint 3R).
//
// Fino a qui c'era la modalità shadow: o il Cockpit non toccava niente, o toccava tutto. Serviva a
// far girare il sistema sulla posta vera senza che un difetto diventasse una mail modificata, ed è
// stata la scelta giusta finché l'unica domanda era «possiamo fidarci?». Ma quando si arriva a
// provare la copia sul NAS di prova, quella domanda si sdoppia: si vuole scrivere un file in una
// cartella di prova, e NON si vuole che una mail vera diventi letta o si sposti.
//
// Con un interruttore solo quelle due cose sono la stessa cosa. Da qui in avanti sono tre:
//
//	outlook_scrittura  segna letto, sposta in cartella: modifica elementi di posta VERI;
//	bozze              crea_bozza_outlook: apre una finestra di composizione sul PC di qualcuno;
//	nas_scrittura      crea_cartella_thread, copia_nas: scrive nel fascicolo del cliente.
//
// Tutto il resto è SEMPRE consentito, perché legge e basta: sincronizzare, scaricare un allegato in
// staging, analizzarlo, scompattare un archivio, «Apri in Outlook» (apre una finestra e non modifica
// niente). Un elenco di eccezioni sarebbe stato più corto da scrivere e più facile da sbagliare: qui
// una capacità si NOMINA, e un tipo di job che non ne nomina nessuna è per definizione una lettura.
//
// Il blocco resta in DUE punti, e servono entrambi:
//
//   - all'accodamento: quei job non entrano in coda, e chi ha premuto il pulsante se lo sente dire;
//   - al claim: quei job non si eseguono NEMMENO SE SONO GIÀ IN CODA. La coda sopravvive al cambio di
//     configurazione, e un `copia_nas` della settimana scorsa scriverebbe sul NAS vero appena un
//     worker lo prende.
type Capacita struct {
	OutlookScrittura bool
	Bozze            bool
	NasScrittura     bool
}

// I nomi sono quelli della sezione [sicurezza] del file di configurazione: compaiono nei log, negli
// errori che legge l'operatore e nel file. Tre spelling diversi della stessa cosa sarebbero tre
// occasioni di cercare nel posto sbagliato.
const (
	CapOutlookScrittura = "outlook_scrittura"
	CapBozze            = "bozze"
	CapNasScrittura     = "nas_scrittura"
)

// TutteLeCapacita è l'elenco, in ordine fisso, per i log e per l'interfaccia.
var TutteLeCapacita = []string{CapOutlookScrittura, CapBozze, CapNasScrittura}

// CapacitaPer dice quale capacità serve per ESEGUIRE questo tipo di job. Stringa vuota = nessuna:
// l'azione legge e basta, e quelle non si spengono.
//
// `apri_elemento_outlook` NON è qui di proposito: apre una finestra e non modifica niente. È l'unica
// azione Outlook che passava anche in shadow, e resta consentita adesso.
func CapacitaPer(t db.TipoJob) string {
	switch t {
	case db.TipoJobSegnaLetto, db.TipoJobSpostaInCartella:
		return CapOutlookScrittura
	case db.TipoJobCreaBozzaOutlook:
		return CapBozze
	case db.TipoJobCopiaNas, db.TipoJobCreaCartellaThread:
		return CapNasScrittura
	}
	return ""
}

// Ha dice se questa capacità è accesa. Un nome sconosciuto vale NO: se un domani qualcuno aggiungesse
// un tipo di job con una capacità nuova e dimenticasse di aggiungerla qui, il job resterebbe fermo —
// che è il modo giusto di sbagliare.
func (c Capacita) Ha(nome string) bool {
	switch nome {
	case CapOutlookScrittura:
		return c.OutlookScrittura
	case CapBozze:
		return c.Bozze
	case CapNasScrittura:
		return c.NasScrittura
	}
	return false
}

// Consente dice se questo tipo di job si può accodare ed eseguire adesso.
func (c Capacita) Consente(t db.TipoJob) bool {
	cap := CapacitaPer(t)
	return cap == "" || c.Ha(cap)
}

// Attive e Spente sono i due elenchi che vanno nel log di avvio. Servono a chi legge quel log a
// rispondere in un secondo alla domanda «che cosa può toccare, questo server, adesso».
func (c Capacita) Attive() []string { return c.elenco(true) }
func (c Capacita) Spente() []string { return c.elenco(false) }

func (c Capacita) elenco(accese bool) []string {
	out := []string{}
	for _, n := range TutteLeCapacita {
		if c.Ha(n) == accese {
			out = append(out, n)
		}
	}
	return out
}

// TuttoSpento dice se questo server non può modificare niente fuori da sé: è il preset «shadow».
func (c Capacita) TuttoSpento() bool { return len(c.Attive()) == 0 }

// capacita sono le capacità di questo processo. Una sola per processo e si decidono all'avvio, come
// la radice del NAS o il DSN: non sono uno stato che cambia mentre il server gira. Stanno qui, e non
// in un campo, perché l'accodamento avviene in una dozzina di punti (UI, ingest, esecutore) che non
// hanno nessun motivo di conoscersi fra loro: farle passare di mano in mano vorrebbe dire aggiungere
// un parametro a tutti e dimenticarlo in uno — e il punto dimenticato sarebbe esattamente il buco.
//
// Il valore di partenza è TUTTO ACCESO, e va spiegato perché sembra il contrario di ciò che serve.
// Il default SICURO sta nel file di configurazione, dove un silenzio vale «non scrivere niente»
// (config.normalizzaSicurezza); qui un default diverso da «il comportamento di sempre» renderebbe
// silenziosamente inerte ogni test che non nomina le capacità, e un test inerte non protegge nulla
// pur restando verde. Il binario le imposta all'avvio, prima che scheduler ed esecutore partano.
var capacita atomic.Value

// ImpostaCapacita fissa le capacità del processo. La chiama main all'avvio; nei test serve a
// simulare una configurazione, e va sempre rimessa a posto con un defer.
func ImpostaCapacita(c Capacita) { capacita.Store(c) }

// CapacitaAttuali sono le capacità di questo processo.
func CapacitaAttuali() Capacita {
	c, ok := capacita.Load().(Capacita)
	if !ok {
		return Capacita{OutlookScrittura: true, Bozze: true, NasScrittura: true}
	}
	return c
}

// Consentito dice se questo tipo di job si può accodare ed eseguire adesso.
func Consentito(t db.TipoJob) bool { return CapacitaAttuali().Consente(t) }

// TipiBloccatiOra sono i tipi che il claim deve escludere adesso. Vuoto e non nil: `tipo <> ALL
// (NULL)` non è vero per nessuna riga, e un claim che non assegna mai niente è il modo più silenzioso
// di fermare un sistema.
//
// Stringhe e non db.TipoJob perché il confronto in SQL avviene su `tipo::text`: pgx non sa
// trasmettere un array di un tipo enum di PostgreSQL senza che gli venga registrato, e registrare un
// tipo a runtime per un confronto di uguaglianza è complicare una cosa semplice.
func TipiBloccatiOra() []string { return TipiBloccati(CapacitaAttuali()) }

// TipiBloccati sono i tipi che quelle capacità non permettono.
func TipiBloccati(c Capacita) []string {
	out := []string{}
	for _, t := range db.AllTipoJobValues() {
		if !c.Consente(t) {
			out = append(out, string(t))
		}
	}
	sort.Strings(out)
	return out
}

// ErrCapacitaSpenta: il job non è stato accodato perché la capacità che gli serve è spenta. Chi
// accoda su richiesta di un operatore deve riconoscerlo e dirglielo — con il nome della capacità,
// così chi legge sa quale riga del file cambiare — invece di trattarlo come un errore interno o,
// peggio, come un successo.
var ErrCapacitaSpenta = errors.New("capacità di scrittura spenta: il job non viene accodato")

// CapacitaMancante è la capacità che manca a un errore di accodamento, oppure "".
func CapacitaMancante(err error) string {
	var m *erroreCapacita
	if errors.As(err, &m) {
		return m.capacita
	}
	return ""
}

type erroreCapacita struct {
	capacita string
	tipo     db.TipoJob
}

func (e *erroreCapacita) Error() string {
	return fmt.Sprintf("%s: %s richiede [sicurezza].%s = true", ErrCapacitaSpenta, e.tipo, e.capacita)
}
func (e *erroreCapacita) Unwrap() error { return ErrCapacitaSpenta }

// AllineaCoda applica alla coda la configurazione di sicurezza, all'avvio del server.
//
//	capacità SPENTA   i job di quei tipi già in coda vengono MARCATI «in attesa di produzione»:
//	                  restano visibili in admin con scritto perché non partono. Il marcatore è anche
//	                  la memoria del fatto che hanno aspettato;
//	capacità ACCESA   quelli marcati vengono ANNULLATI: nulla che abbia aspettato si mette in moto da
//	                  solo quando qualcuno accende un interruttore (§2.7). Le copie che servono
//	                  davvero si rimettono in coda con «riprova copie», che è un gesto di una persona.
//
// Un avvio con le capacità già accese e nessun job marcato non annulla niente: senza marcatore non
// c'è niente da annullare, e le copie NAS accodate cinque minuti fa da un operatore partono
// regolarmente.
func AllineaCoda(ctx context.Context, q *db.Queries, c Capacita, log *slog.Logger) error {
	spenti := TipiBloccati(c)
	if len(spenti) > 0 {
		n, err := q.MarcaJobInAttesaDiProduzione(ctx, spenti)
		if err != nil {
			return fmt.Errorf("marcatura dei job in attesa di produzione: %w", err)
		}
		if n > 0 {
			log.Warn("capacità spente: job che toccano il mondo lasciati in coda e non eseguiti", "n", n,
				"tipi", spenti, "nota", "in admin compaiono come «in attesa di produzione»")
		}
	}
	// I tipi ORA CONSENTITI che portano ancora il marcatore: hanno aspettato con la capacità spenta,
	// e adesso quella capacità è accesa. Non partono lo stesso.
	var riammessi []string
	for _, t := range db.AllTipoJobValues() {
		if CapacitaPer(t) != "" && c.Consente(t) {
			riammessi = append(riammessi, string(t))
		}
	}
	if len(riammessi) == 0 {
		return nil
	}
	righe, err := q.AnnullaJobDellaShadow(ctx, riammessi)
	if err != nil {
		return fmt.Errorf("annullamento dei job che avevano aspettato: %w", err)
	}
	for _, r := range righe {
		log.Warn("job annullato: era in coda mentre la sua capacità era spenta", "job", r.JobID, "tipo", r.Tipo)
	}
	if len(righe) > 0 {
		log.Warn("capacità riaccese: nulla di ciò che era in coda è stato eseguito",
			"annullati", len(righe), "nota", "i documenti restano in_coda: rimetterli in copia con «riprova copie»")
	}
	return nil
}
