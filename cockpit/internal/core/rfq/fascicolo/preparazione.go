package fascicolo

// La preparazione del Fascicolo (B8.7b): quello che il sistema fa da solo prima che qualcuno decida.
//
// Due cose, e nessuna delle due tocca il NAS.
//
//   - I codici della richiesta sono prodotti finiti gia' decisi. L'operatore li spunta quando crea la RFQ
//     (identificativo_thread, «codici PRODOTTO FINITO della RFQ» nello schema): da li' ciascuno e' anche
//     un componente radice di tipo finito, senza chiedere di nuovo «+ Prodotto» nel Fascicolo. Senza
//     STEP il prodotto e' una radice senza figli, e la BOM e' gia' valida; lo STEP, quando arriva, propone
//     la struttura sotto quella radice (la sua radice ritrova il componente, A2.3).
//   - I file utili della RFQ scendono nello staging da soli, gli archivi fermi si estraggono e i file
//     fermi si analizzano; i fatti gia' calcolati per lo stesso contenuto si riusano. Lo staging e' una
//     cache del server (staging/cache.go): i documenti nascono solo con la conferma, e solo allora parte
//     la copia sul NAS.

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
)

// EsitoProdotti dice che cosa ha fatto AssicuraProdottiDellaRichiesta.
type EsitoProdotti struct {
	Creati   []string // i codici della richiesta diventati componenti adesso
	Saltati  []string // codici della richiesta che non possono essere il codice di un componente
	Bloccata int32    // la BOM e' congelata in questa versione: niente e' cambiato
}

// AssicuraProdottiDellaRichiesta fa di ogni codice della richiesta confermato da una persona un componente
// radice di tipo finito, se nella RFQ non c'e' gia' un componente con quel codice (upper). Idempotente:
// la seconda volta non fa niente. Un componente che c'e' gia' non si tocca, qualunque sia il suo tipo o
// il suo stato: quello e' gia' una decisione, e un archiviato non si ripristina da solo.
//
// Chi ha confermato il codice e' chi ha deciso il componente: confermato_da viene dall'identificativo. Un
// identificativo senza chi l'ha confermato non e' una decisione, e resta fuori. Con la BOM congelata non
// cambia niente (D26): il codice resta fra quelli della richiesta, e il prodotto entra aprendo una
// revisione. Le proposte STEP aperte con lo stesso codice ritrovano il componente (A2.3), come quando si
// accetta un nodo.
func AssicuraProdottiDellaRichiesta(ctx context.Context, q *db.Queries, thread uuid.UUID) (EsitoProdotti, error) {
	var es EsitoProdotti
	if err := bloccaThread(ctx, q, thread); err != nil {
		return es, err
	}
	n, bloccata, err := WorkingBloccata(ctx, q, thread)
	if err != nil {
		return es, err
	}
	if bloccata {
		es.Bloccata = n
		return es, nil
	}
	ids, err := q.ListIdentificativi(ctx, thread)
	if err != nil {
		return es, err
	}
	for _, i := range ids {
		codice := strings.TrimSpace(i.Codice)
		if codice == "" || !i.ConfermatoDa.Valid {
			continue
		}
		switch _, err := q.GetComponentePerCodice(ctx, db.GetComponentePerCodiceParams{ThreadID: thread, Upper: codice}); {
		case err == nil:
			continue
		case !errors.Is(err, pgx.ErrNoRows):
			return es, err
		}
		if !classificazione.CodiceAmmissibile(codice) {
			es.Saltati = append(es.Saltati, codice)
			continue
		}
		origine := db.OrigineComponenteCodiceRilevato
		if i.Origine == db.OrigineIdentificativoManuale {
			origine = db.OrigineComponenteManuale
		}
		c, err := q.InsertComponente(ctx, db.InsertComponenteParams{ThreadID: thread, Codice: codice, Qta: 1,
			Tipo: db.TipoComponenteFinito, Origine: origine, ConfermatoDa: i.ConfermatoDa.UUID})
		if err != nil {
			return es, err
		}
		if _, err := q.RiconciliaProposteNodo(ctx, db.RiconciliaProposteNodoParams{ThreadID: thread, Codice: c.Codice,
			ComponenteID: uid(c.ComponenteID), Esclusa: uuid.Nil}); err != nil {
			return es, err
		}
		es.Creati = append(es.Creati, c.Codice)
	}
	if len(es.Creati) > 0 {
		if _, err := AggiornaTutteLeRimozioni(ctx, q, thread); err != nil {
			return es, err
		}
	}
	return es, nil
}

// Preparazione dice che cosa ha fatto PreparaFile.
type Preparazione struct {
	Download   int      // download accodati adesso (o gia' in coda)
	Riusati    int      // lo stesso contenuto era gia' nello staging: nessun download
	Estrazioni int      // archivi fermi rimessi all'estrazione
	Analisi    int      // file fermi rimessi all'analisi
	Riletti    int      // file fermi che avevano gia' i fatti: letti adesso
	Rimandati  int      // oltre il limite di questo giro: al prossimo
	Saltati    []string // file che non si possono scaricare da soli (nessuna casella attiva), con il motivo
}

// Qualcosa dice se la preparazione ha messo in moto del lavoro.
func (p Preparazione) Qualcosa() bool {
	return p.Download+p.Riusati+p.Estrazioni+p.Analisi+p.Riletti > 0
}

// RiletturaFatti e' chi applica a un allegato fermo i fatti gia' calcolati per il suo contenuto: la strada
// del workerapi (dopoStaging), la stessa di un file appena sceso.
type RiletturaFatti func(ctx context.Context, q *db.Queries, allegato uuid.UUID) error

// PreparaFile mette in moto il lavoro che manca ai file della RFQ, al piu' max cose per giro:
//
//   - gli allegati utili (pre_spunta) non ancora scesi si scaricano, fino a maxBytes (il limite degli
//     upload dei worker): e' lo stesso download di «Scarica», con la stessa guardia che riusa un contenuto
//     gia' presente;
//   - un archivio sceso e mai estratto si rimette all'estrazione, un file sceso e mai analizzato
//     all'analisi, se l'ultimo tentativo con la stessa chiave non e' fallito: un file che fa fallire
//     l'analizzatore non si riaccoda a ogni apertura, lo guarda una persona;
//   - un file fermo i cui fatti ci sono gia' (lo stesso contenuto analizzato per un'altra RFQ) li riceve
//     adesso, con rileggi.
//
// Idempotente per costruzione: le chiavi dei job sono per allegato e per contenuto.
func PreparaFile(ctx context.Context, q *db.Queries, thread uuid.UUID, an coda.Analizzatore, maxBytes int64, max int,
	rileggi RiletturaFatti) (Preparazione, error) {
	var p Preparazione
	tid := uuid.NullUUID{UUID: thread, Valid: true}
	fatti := 0
	da, err := q.ListAllegatiDaPreparare(ctx, db.ListAllegatiDaPreparareParams{ThreadID: tid, MaxBytes: maxBytes})
	if err != nil {
		return p, err
	}
	for _, x := range da {
		if fatti >= max {
			p.Rimandati++
			continue
		}
		a, err := q.GetAllegato(ctx, x.AllegatoID)
		if err != nil {
			return p, err
		}
		m, err := q.GetMessaggio(ctx, a.MessaggioID)
		if err != nil {
			return p, err
		}
		copia, err := coda.CopiaPerDownload(ctx, q, m.MessaggioID, uuid.NullUUID{}, uuid.Nil)
		if err != nil {
			p.Saltati = append(p.Saltati, a.NomeFile+": nessuna casella attiva da cui scaricarlo")
			continue
		}
		esito, _, err := coda.AccodaStage(ctx, q, staging.FileStaging{}, a, m, copia, 2)
		if err != nil {
			return p, err
		}
		fatti++
		switch esito {
		case coda.StageRiusato, coda.StageGiaPresente:
			p.Riusati++
		default:
			p.Download++
		}
	}

	fermi, err := q.ListAllegatiFermiInStaging(ctx, tid)
	if err != nil {
		return p, err
	}
	for _, a := range fermi {
		if !inStaging(a.PathStaging.String) {
			continue // sparito dalla cache: si riprende con «Riscarica», non con un'analisi che fallirebbe
		}
		if fatti >= max {
			p.Rimandati++
			continue
		}
		if strings.EqualFold(strings.TrimPrefix(a.Estensione.String, "."), "zip") {
			chiave := "estrai:" + a.AllegatoID.String()
			if provato, err := giaProvato(ctx, q, chiave); err != nil || provato {
				if err != nil {
					return p, err
				}
				continue
			}
			j, err := coda.Accoda(ctx, q, db.TipoJobEstraiArchivio, worker.PayloadEstraiArchivio{AllegatoID: a.AllegatoID}, chiave, 4)
			if err != nil {
				return p, err
			}
			if j != nil {
				p.Estrazioni++
				fatti++
			}
			continue
		}
		if an.Versione == 0 {
			continue
		}
		_, err := q.GetAnalisiFatti(ctx, db.GetAnalisiFattiParams{Sha256: a.Sha256.String, VersioneAnalizzatore: int16(an.Versione),
			HashConfigurazione: an.Hash()})
		switch {
		case err == nil:
			if rileggi == nil {
				continue
			}
			if err := rileggi(ctx, q, a.AllegatoID); err != nil {
				return p, err
			}
			p.Riletti++
			fatti++
			continue
		case !errors.Is(err, pgx.ErrNoRows):
			return p, err
		}
		if provato, err := giaProvato(ctx, q, coda.ChiaveAnalisi(a.Sha256.String, an)); err != nil || provato {
			if err != nil {
				return p, err
			}
			continue
		}
		m, err := q.GetMessaggio(ctx, a.MessaggioID)
		if err != nil {
			return p, err
		}
		j, err := coda.AccodaAnalisi(ctx, q, a, m.ThreadID, an)
		if err != nil {
			return p, err
		}
		if j != nil {
			p.Analisi++
			fatti++
		}
	}
	return p, nil
}

// giaProvato dice se l'ultimo job con quella chiave e' ancora pendente o e' fallito: nel primo caso non
// c'e' niente da accodare, nel secondo non si insiste da soli.
func giaProvato(ctx context.Context, q *db.Queries, chiave string) (bool, error) {
	j, err := q.UltimoJobPerChiave(ctx, testo(chiave))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	switch j.Stato {
	case db.StatoJobPronto, db.StatoJobInCorso, db.StatoJobFallito:
		return true, nil
	}
	return false, nil
}

// Lavoro e' il lavoro ancora in corso sui file della RFQ: quello per cui la schermata del Fascicolo si
// aggiorna da sola, e che dice in testata.
type Lavoro struct {
	Download, Estrazioni, Analisi int
	File                          []string // i nomi dei file coinvolti, senza ripetizioni
}

// InCorso dice se c'e' ancora lavoro.
func (l Lavoro) InCorso() bool { return l.Download+l.Estrazioni+l.Analisi > 0 }

// Totale e' il numero di lavori.
func (l Lavoro) Totale() int { return l.Download + l.Estrazioni + l.Analisi }

// LavoroInCorso conta i job pendenti sui file della RFQ, uno per job: un'analisi di un contenuto che la RFQ
// ha in due allegati e' un lavoro solo.
func LavoroInCorso(ctx context.Context, q *db.Queries, thread uuid.UUID) (Lavoro, error) {
	var l Lavoro
	righe, err := q.ListLavoroPendenteRfq(ctx, uuid.NullUUID{UUID: thread, Valid: true})
	if err != nil {
		return l, err
	}
	visti := map[int64]bool{}
	nomi := map[string]bool{}
	for _, r := range righe {
		if !nomi[r.NomeFile] {
			nomi[r.NomeFile] = true
			l.File = append(l.File, r.NomeFile)
		}
		if visti[r.JobID] {
			continue
		}
		visti[r.JobID] = true
		switch r.Tipo {
		case db.TipoJobStageAllegato:
			l.Download++
		case db.TipoJobEstraiArchivio:
			l.Estrazioni++
		case db.TipoJobAnalizzaAllegato:
			l.Analisi++
		}
	}
	return l, nil
}
