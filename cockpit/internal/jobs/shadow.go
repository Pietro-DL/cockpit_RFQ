package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"

	"promatec/cockpit/internal/db"
)

// Modalità shadow (piano §2.7, D16, voce 9.5).
//
// In shadow il Cockpit legge il mondo e non lo tocca. Serve a far girare tutto sulla posta vera —
// sync, triage, proposte, fascicolo — senza che un difetto si trasformi in una mail modificata, una
// bozza aperta sul PC di qualcuno o un file scritto nel fascicolo di un cliente. È il modo di
// scoprire i difetti prima che costino qualcosa.
//
// Il blocco è in DUE punti, e servono entrambi:
//
//   - all'accodamento: quei job non entrano in coda, e chi ha premuto il pulsante se lo sente dire;
//   - al claim: quei job non si eseguono NEMMENO SE SONO GIÀ IN CODA. La coda sopravvive al cambio
//     di modalità, e un `copia_nas` della settimana scorsa scriverebbe sul NAS vero appena un worker
//     lo prende. Questo è il punto che la v2.1 del piano ha aggiunto dopo, ed è quello che conta.
//
// Al ritorno in produzione i job rimasti in coda durante la shadow vengono ANNULLATI (AllineaCoda):
// nulla che sia stato deciso in un'altra modalità parte a sorpresa. I documenti restano `in_coda` e
// chi le vuole davvero rimette le copie in coda a mano.
type Modalita string

const (
	ModalitaShadow     Modalita = "shadow"
	ModalitaProduzione Modalita = "produzione"
)

// bloccati sono i tipi di job che la shadow non accoda e non esegue. `apri_elemento_outlook` NON è
// qui di proposito: apre una finestra e non modifica niente, ed è l'unica azione Outlook che la
// shadow lascia passare (§2.7). Nemmeno `sync_outlook`, `stage_allegato` e `analizza_allegato`:
// leggono, e leggere è tutto ciò che la shadow vuole fare.
var bloccati = []db.TipoJob{
	db.TipoJobCreaBozzaOutlook,
	db.TipoJobSegnaLetto,
	db.TipoJobSpostaInCartella,
	db.TipoJobCopiaNas,
	db.TipoJobCreaCartellaThread,
}

// modalita è una sola per processo e si decide all'avvio, come la radice del NAS o il DSN: non è uno
// stato che cambia mentre il server gira. Sta qui, e non in un campo, perché l'accodamento avviene
// in una dozzina di punti (UI, ingest, esecutore) che non hanno nessun motivo di conoscersi fra
// loro: farla passare di mano in mano vorrebbe dire aggiungere un parametro a tutti e dimenticarlo
// in uno — e il punto dimenticato sarebbe esattamente il buco.
//
// Il valore di partenza è `produzione`: il default SICURO sta nel file di configurazione (dove un
// silenzio vale shadow), non qui, perché qui un default diverso da «il comportamento di sempre»
// renderebbe silenziosamente inerte ogni test che non nomina la modalità.
var modalita atomic.Value

// ImpostaModalita fissa la modalità del processo. La chiama main all'avvio; nei test serve a
// simulare la shadow, e va sempre rimessa a posto con un defer.
func ImpostaModalita(m Modalita) { modalita.Store(m) }

// ModalitaAttuale è la modalità di questo processo.
func ModalitaAttuale() Modalita {
	m, _ := modalita.Load().(Modalita)
	if m == "" {
		return ModalitaProduzione
	}
	return m
}

// InShadow dice se il server sta girando in sola lettura verso il mondo.
func InShadow() bool { return ModalitaAttuale() == ModalitaShadow }

// BloccatoInShadow dice se questo tipo di job tocca il mondo fuori dal Cockpit.
func BloccatoInShadow(t db.TipoJob) bool {
	for _, b := range bloccati {
		if b == t {
			return true
		}
	}
	return false
}

// TipiBloccatiOra sono i tipi che il claim deve escludere adesso: l'elenco in shadow, un array VUOTO
// in produzione. Vuoto e non nil: `tipo <> ALL (NULL)` non è vero per nessuna riga, e un claim che
// non assegna mai niente è il modo più silenzioso di fermare un sistema.
//
// Stringhe e non db.TipoJob perché il confronto in SQL avviene su `tipo::text`: pgx non sa
// trasmettere un array di un tipo enum di PostgreSQL senza che gli venga registrato, e registrare un
// tipo a runtime per un confronto di uguaglianza è complicare una cosa semplice.
func TipiBloccatiOra() []string {
	if !InShadow() {
		return []string{}
	}
	return tipiTesto(bloccati)
}

func tipiTesto(tipi []db.TipoJob) []string {
	out := make([]string, 0, len(tipi))
	for _, t := range tipi {
		out = append(out, string(t))
	}
	return out
}

// ErrShadow: il job non è stato accodato perché il server è in shadow. Chi accoda su richiesta di un
// operatore deve riconoscerlo e dirglielo — «in attesa di produzione» — invece di trattarlo come un
// errore interno o, peggio, come un successo.
var ErrShadow = errors.New("modalità shadow: il job non viene accodato")

// AllineaCoda applica alla coda il cambio di modalità, all'avvio del server.
//
//	verso shadow      i job bloccati già in coda vengono MARCATI «in attesa di produzione»: restano
//	                  visibili in admin con scritto perché non partono. Il marcatore è anche la
//	                  memoria del fatto che hanno attraversato una shadow;
//	verso produzione  quelli marcati vengono ANNULLATI: nulla che abbia aspettato durante la shadow
//	                  si mette in moto da solo (§2.7). Le copie che servono davvero si rimettono in
//	                  coda con «riprova copie», che è un gesto di una persona.
//
// Un avvio in produzione senza shadow precedente non annulla niente: senza marcatore non c'è niente
// da annullare, e le copie NAS accodate cinque minuti fa da un operatore partono regolarmente.
func AllineaCoda(ctx context.Context, q *db.Queries, m Modalita, log *slog.Logger) error {
	if m == ModalitaShadow {
		n, err := q.MarcaJobInAttesaDiProduzione(ctx, tipiTesto(bloccati))
		if err != nil {
			return fmt.Errorf("marcatura dei job in attesa di produzione: %w", err)
		}
		if n > 0 {
			log.Warn("modalità shadow: job che toccano il mondo lasciati in coda e non eseguiti", "n", n,
				"tipi", bloccati, "nota", "in admin compaiono come «in attesa di produzione»")
		}
		return nil
	}
	righe, err := q.AnnullaJobDellaShadow(ctx, tipiTesto(bloccati))
	if err != nil {
		return fmt.Errorf("annullamento dei job della shadow: %w", err)
	}
	for _, r := range righe {
		log.Warn("job annullato al ritorno in produzione: era in coda durante la shadow", "job", r.JobID, "tipo", r.Tipo)
	}
	if len(righe) > 0 {
		log.Warn("passaggio a produzione: nulla di ciò che era in coda durante la shadow è stato eseguito",
			"annullati", len(righe), "nota", "i documenti restano in_coda: rimetterli in copia con «riprova copie»")
	}
	return nil
}
