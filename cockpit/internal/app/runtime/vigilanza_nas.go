// La vigilanza sul NAS assente: un NAS irraggiungibile non e' un errore del job, e' una condizione
// del mondo.
//
// Tre momenti. Prima di eseguire si guarda se il job ha bisogno del NAS e il NAS non c'e'
// (rinviaSeNasAssente); intanto un guardiano controlla se e' tornato (vigilaNas); quando torna, le
// scritture che avevano esaurito i tentativi tornano in coda (RiaccodaAlRitornoDelNas).
//
// ScrivePerNas sta qui e non in `core/rfq/documenti`: non dice che cosa significhi copiare un
// documento, dice di quale infrastruttura un TIPO DI JOB ha bisogno per poter partire. E' una
// domanda che si fa l'esecutore, e la risposta la usa solo lui.

package runtime

import (
	"context"
	"time"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// ScrivePerNas dice se il job ha bisogno che il NAS ci sia. Sono i due che scrivono sotto la radice:
// senza NAS non possono nemmeno cominciare, e provarci non insegna niente a nessuno.
func ScrivePerNas(t db.TipoJob) bool {
	return t == db.TipoJobCopiaNas || t == db.TipoJobCreaCartellaThread
}

// rinviaSeNasAssente restituisce true se il job è stato rimesso in coda senza essere eseguito.
//
// Il tentativo non viene consumato: il job non è fallito, non lo abbiamo nemmeno provato. Contarlo
// come fallimento significherebbe bruciare il budget dei tentativi mentre il problema è che il NAS
// non c'è — e dichiarare persa la copia proprio per aver provato tante volte (voce 1.7, N8).
func (e *EsecutoreServer) rinviaSeNasAssente(ctx context.Context, q *db.Queries, t coda.Tentativo, j *db.Job) bool {
	if e.NAS.DryRun || !ScrivePerNas(j.Tipo) || e.NAS.Raggiungibile() {
		return false
	}
	fra := e.RitardoNasAssente
	if fra <= 0 {
		fra = time.Minute
	}
	if _, err := coda.Rinvia(ctx, q, t, fra, "NAS non raggiungibile: tentativo non consumato"); err != nil {
		e.Log.Error("rinvio non riuscito", "job", j.JobID, "tipo", j.Tipo, "err", err)
		return false // meglio provarci: il peggio che può succedere è un fallimento onesto
	}
	e.Log.Warn("NAS non raggiungibile: job rinviato senza consumare il tentativo",
		"job", j.JobID, "tipo", j.Tipo, "tentativi", j.Tentativi, "fra", fra)
	return true
}

// vigilaNas guarda se il NAS è tornato e, quando torna, rimette in coda le scritture che avevano
// finito i tentativi (N3: «NAS torna → scritto senza intervento»).
//
// Il primo controllo è all'avvio e non aspetta la transizione: se il server viene riavviato dopo che
// il NAS è già tornato, una transizione non ci sarà mai più e le copie resterebbero ferme per sempre.
func (e *EsecutoreServer) vigilaNas(ctx context.Context, q *db.Queries) {
	ogni := e.ControlloNas
	if ogni <= 0 {
		ogni = time.Minute
	}
	presente := e.NAS.Raggiungibile()
	if presente {
		e.RiaccodaAlRitornoDelNas(ctx, q)
	}
	t := time.NewTicker(ogni)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		ora := e.NAS.Raggiungibile()
		if ora && !presente {
			e.Log.Info("NAS di nuovo raggiungibile", "radice", e.NAS.Radice)
			e.RiaccodaAlRitornoDelNas(ctx, q)
		}
		if !ora && presente {
			e.Log.Warn("NAS non più raggiungibile: le copie restano in coda", "radice", e.NAS.Radice)
		}
		presente = ora
	}
}

// RiaccodaAlRitornoDelNas rimette 'pronto' le scritture NAS che avevano esaurito i tentativi.
// Esportata perché è il punto che il test N3 chiama: quello che gira in produzione e quello che si
// verifica devono essere la stessa funzione.
func (e *EsecutoreServer) RiaccodaAlRitornoDelNas(ctx context.Context, q *db.Queries) int {
	ids, err := q.RiaccodaScrittureNasEsaurite(ctx)
	if err != nil {
		e.Log.Error("riaccodo al ritorno del NAS", "err", err)
		return 0
	}
	if len(ids) > 0 {
		e.Log.Info("scritture NAS rimesse in coda al ritorno del NAS", "n", len(ids), "job", ids)
	}
	return len(ids)
}
