//go:build integrazione

// L4 — blocco 4A: l'estrazione di un archivio la esegue l'esecutore interno del server.
//
// Non e' una raffinatezza di architettura: prima uno zip veniva scompattato dentro il gestore del
// result, cioe' dentro la richiesta HTTP con cui il worker Outlook consegnava il download. Il worker
// e' uno per PC ed e' seriale, quindi restava fermo ad aspettare la risposta di un lavoro che non era
// il suo, mentre il web server scriveva su disco fino a mezzo gigabyte di file.
//
// Questi test guardano l'esecutore: che prenda quel tipo di job, che lo affidi a chi sa scompattare, e
// che dica chiaramente di non saperlo fare quando nessuno gliel'ha insegnato — invece di lasciare i
// job in coda a tempo indeterminato mentre gli operatori aspettano le voci di uno zip.
package jobs

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// estrattoreFinto registra che cosa gli e' stato chiesto.
type estrattoreFinto struct {
	allegato uuid.UUID
	token    uuid.UUID
	chiamate int
	voci     int
	err      error
}

func (e *estrattoreFinto) EstraiArchivio(ctx context.Context, allegatoID uuid.UUID, token uuid.UUID) (int, error) {
	e.chiamate++
	e.allegato, e.token = allegatoID, token
	return e.voci, e.err
}

func TestEsecutoreAffidaLArchivioAChiSaScompattarlo(t *testing.T) {
	_, q, ctx := preparaDB(t)
	e := esecutore(t, t.TempDir())
	finto := &estrattoreFinto{voci: 7}
	e.Archivi = finto

	allegato := uuid.New()
	j, err := coda.AccodaCon(ctx, q, db.TipoJobEstraiArchivio, worker.PayloadEstraiArchivio{AllegatoID: allegato},
		"estrai:"+allegato.String(), 4, coda.Opzioni{})
	if err != nil || j == nil {
		t.Fatalf("accodamento: job=%v err=%v", j, err)
	}

	// Il claim e' quello vero: se il tipo finisse su un worker che non esiste, il job resterebbe in
	// coda per sempre e nessuno se ne accorgerebbe.
	preso := claim(t, ctx, q, db.WorkerTipoServer, "server")
	if preso == nil {
		t.Fatal("l'esecutore interno non ha preso il job: estrai_archivio non e' un job del server")
	}
	if preso.JobID != j.JobID {
		t.Fatalf("preso il job %d invece del %d", preso.JobID, j.JobID)
	}
	tent := coda.Tentativo{JobID: preso.JobID, LeaseToken: preso.LeaseToken.UUID, WorkerID: "server"}

	res, err := e.esegui(ctx, q, preso, tent)
	if err != nil {
		t.Fatalf("esecuzione: %v", err)
	}
	if finto.chiamate != 1 {
		t.Errorf("l'estrattore e' stato chiamato %d volte, attesa 1", finto.chiamate)
	}
	if finto.allegato != allegato {
		t.Errorf("estratto l'allegato %s invece di %s", finto.allegato, allegato)
	}
	// il token del tentativo deve arrivare fino in fondo: la cartella temporanea in cui l'archivio si
	// scompatta lo porta nel nome, ed e' cosi' che la pulizia sa di chi era se il job non finisce
	if finto.token != preso.LeaseToken.UUID {
		t.Errorf("token passato = %s, atteso quello del tentativo %s", finto.token, preso.LeaseToken.UUID)
	}
	m, ok := res.(map[string]any)
	if !ok || m["voci"] != 7 {
		t.Errorf("l'esito non riporta quante voci sono state trovate: %v", res)
	}
}

// Senza un estrattore configurato il job FALLISCE dicendolo. L'alternativa — restare in coda — e' il
// modo piu' silenzioso di non funzionare: in admin il job resta 'pronto' e sembra che stia per
// partire, mentre non partira' mai.
func TestEsecutoreSenzaEstrattoreLoDice(t *testing.T) {
	_, q, ctx := preparaDB(t)
	e := esecutore(t, t.TempDir())

	allegato := uuid.New()
	j, err := coda.AccodaCon(ctx, q, db.TipoJobEstraiArchivio, worker.PayloadEstraiArchivio{AllegatoID: allegato},
		"estrai:"+allegato.String(), 4, coda.Opzioni{})
	if err != nil || j == nil {
		t.Fatalf("accodamento: job=%v err=%v", j, err)
	}
	preso := claim(t, ctx, q, db.WorkerTipoServer, "server")
	if preso == nil {
		t.Fatal("job non assegnato")
	}
	tent := coda.Tentativo{JobID: preso.JobID, LeaseToken: preso.LeaseToken.UUID, WorkerID: "server"}

	if _, err := e.esegui(ctx, q, preso, tent); err == nil {
		t.Fatal("senza estrattore il job doveva fallire dicendolo")
	}
}

// L'estrazione non tocca il NAS: un NAS assente non deve rinviarla. Le voci di uno zip servono per
// decidere, e decidere si fa anche quando il fascicolo non e' raggiungibile.
func TestEstrarreUnArchivioNonDipendeDalNas(t *testing.T) {
	if documenti.ScrivePerNas(db.TipoJobEstraiArchivio) {
		t.Error("estrai_archivio risulta una scrittura sul NAS: un NAS assente ne rinvierebbe l'esecuzione")
	}
	if !coda.Consentito(db.TipoJobEstraiArchivio) {
		t.Error("estrai_archivio risulta bloccato in shadow: scompattare in staging non tocca il mondo fuori dal Cockpit")
	}
	if coda.WorkerPer(db.TipoJobEstraiArchivio) != db.WorkerTipoServer {
		t.Errorf("estrai_archivio va a %s invece che al server", coda.WorkerPer(db.TipoJobEstraiArchivio))
	}
}
