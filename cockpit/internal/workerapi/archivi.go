package workerapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/domain"
	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/archivio"
)

// L'estrazione di un archivio è un JOB, non un pezzo di una richiesta HTTP (blocco 4A).
//
// Prima uno zip veniva scompattato dentro il gestore del result: non dentro la transazione — quello
// era già stato corretto alla voce 1.4 — ma comunque dentro la richiesta. Il server teneva aperta una
// connessione HTTP mentre scriveva su disco fino a mezzo gigabyte di file, e il worker Outlook, che è
// uno per PC ed è seriale, restava fermo ad aspettare la risposta di un lavoro che non era il suo.
//
// Ora il result fa una cosa sola: promuove il contenuto verificato e accoda `estrai_archivio`. Il job
// è di `worker_tipo = 'server'`, quindi lo prende l'esecutore interno — la stessa goroutine che
// scrive sul NAS — che sta accanto al disco e non occupa il web server.
//
// Perché non farlo fare al worker Outlook, che il file ce l'ha già in mano: perché sulla rete
// passerebbero i file scompattati invece dell'archivio. Uno zip da 8 MB con dentro 120 MB di disegni
// viaggia oggi come 8 MB; scompattarlo sulla postazione ne farebbe passare 120. La compressione è il
// motivo per cui l'archivio esiste, e va disfatta il più tardi possibile.

// EstraiArchivio scompatta un archivio già in staging, mette ogni voce fra i contenuti e la registra
// come allegato figlio con la sua proposta e la sua analisi. Restituisce quante voci ha trovato.
//
// È ripetibile, e deve esserlo: se il server muore a metà, il job torna in coda e riparte da capo.
// Le voci finiscono fra i contenuti con il proprio sha256 per nome — quindi la seconda volta ci sono
// già — e le righe si riscrivono per upsert. Per lo stesso motivo non c'è bisogno che il tentativo
// sia verificato qui dentro: due tentativi che si sovrappongono scrivono le stesse cose, e a chiudere
// il job sarà comunque uno solo (Completa verifica il tentativo e risponde 409 a quello scaduto).
//
// Un archivio illeggibile NON fa fallire il job: riprovarlo darebbe all'infinito lo stesso esito.
// L'allegato va in errore con il motivo visibile, e il job si chiude.
func (s *Server) EstraiArchivio(ctx context.Context, allegatoID uuid.UUID, token uuid.UUID) (int, error) {
	q := db.New(s.Pool)
	a, err := q.GetAllegato(ctx, allegatoID)
	if err != nil {
		return 0, fmt.Errorf("allegato %s: %w", allegatoID, err)
	}
	if !a.PathStaging.Valid || !(jobs.FileStaging{}).Presente(a.PathStaging.String) {
		// Il file non c'è più: qualcuno ha svuotato lo staging fra il download e adesso. Non è un
		// guasto da ritentare, è un allegato da riscaricare.
		return 0, q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: allegatoID,
			Stato: db.StatoAllegatoErrore, Errore: txt("archivio non presente in staging: usa Riscarica")})
	}
	m, err := q.GetMessaggio(ctx, a.MessaggioID)
	if err != nil {
		return 0, err
	}

	voci, errEstrazione := s.estraiInContenuti(a.PathStaging.String, token)
	troncato := errors.Is(errEstrazione, archivio.ErrLimite)
	if errEstrazione != nil && !troncato {
		s.Log.Warn("archivio non leggibile", "allegato", allegatoID, "file", a.NomeFile, "err", errEstrazione)
		return 0, q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: allegatoID,
			Stato: db.StatoAllegatoErrore, Errore: txt("zip non leggibile: " + errEstrazione.Error())})
	}

	// Le righe in una transazione sola: o l'archivio ha le sue voci con le loro proposte e le loro
	// analisi, oppure non ne ha nessuna. Un archivio registrato a metà si presenterebbe all'operatore
	// come completo, e mancherebbero proprio i disegni che il job non ha fatto in tempo a scrivere.
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	qt := db.New(tx)
	for i, v := range voci {
		figlio, err := qt.UpsertAllegato(ctx, db.UpsertAllegatoParams{
			MessaggioID: a.MessaggioID, ContenitoreID: uuid.NullUUID{UUID: a.AllegatoID, Valid: true}, Indice: int16(i + 1),
			NomeFile: v.NomeFile, PathInterno: txt(v.PathInterno), Estensione: txt(strings.ToLower(strings.TrimPrefix(filepath.Ext(v.NomeFile), "."))),
			Natura: db.NaturaAllegatoFile, Origine: a.Origine, Bytes: pgtype.Int8{Int64: v.Bytes, Valid: true}, Sha256: txt(v.Sha256), RicevutoIl: a.RicevutoIl,
		})
		if err != nil {
			return 0, fmt.Errorf("voce zip %s: %w", v.NomeFile, err)
		}
		if err := qt.SetAllegatoStaging(ctx, db.SetAllegatoStagingParams{AllegatoID: figlio.AllegatoID, PathStaging: txt(v.Path), Sha256: txt(v.Sha256), Bytes: pgtype.Int8{Int64: v.Bytes, Valid: true}}); err != nil {
			return 0, err
		}
		figlio.PathStaging, figlio.Sha256 = txt(v.Path), txt(v.Sha256)
		pr := domain.PropostaDaNome(v.NomeFile, v.Bytes, string(m.Direzione))
		if err := s.scriviProposta(ctx, qt, figlio, m.ThreadID, pr, map[string]any{"path_interno": v.PathInterno, "bytes": v.Bytes, "zip": a.NomeFile}); err != nil {
			return 0, err
		}
		if pr.Tipo == "rumore" {
			_ = qt.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: figlio.AllegatoID, Stato: db.StatoAllegatoAnalizzato})
			continue
		}
		if _, err := jobs.AccodaAnalisi(ctx, qt, figlio, m.ThreadID, s.Analizzatore); err != nil {
			return 0, err
		}
	}
	// Lo zip stesso resta un contenitore: non va sul NAS a meno di una conferma esplicita.
	dettagli := map[string]any{"voci": len(voci), "estensione": "zip", "bytes": a.Bytes.Int64}
	if troncato {
		dettagli["troncato"] = true
	}
	if err := s.scriviProposta(ctx, qt, a, m.ThreadID, domain.Proposta{Tipo: "altro", Fonte: "estensione", Confidenza: 20}, dettagli); err != nil {
		return 0, err
	}
	if err := qt.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: a.AllegatoID, Stato: db.StatoAllegatoAnalizzato}); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	s.Log.Info("archivio estratto", "allegato", allegatoID, "file", a.NomeFile, "voci", len(voci), "troncato", troncato)
	return len(voci), nil
}

// estraiInContenuti estrae un archivio e mette ogni voce fra i contenuti, con il suo hash per nome.
//
// L'estrazione passa per una cartella temporanea perché non c'è modo di fare altrimenti: il nome
// definitivo di una voce è il suo sha256, e lo sha256 si conosce dopo averla scritta. La temporanea
// sta sotto _parti, con il token del tentativo nel nome, così anche lei è di QUESTO tentativo e la
// pulizia sa di chi era se il tentativo non arriva in fondo.
//
// Le voci già presenti non si riscrivono: è il caso dello stesso disegno dentro due archivi diversi,
// che è la norma quando un cliente rimanda la stessa commessa con una revisione in più.
func (s *Server) estraiInContenuti(zip string, token uuid.UUID) ([]archivio.Voce, error) {
	tmp := jobs.PercorsoEstrazione(s.Staging, token)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	voci, errEstrazione := archivio.Estrai(zip, tmp)
	for i := range voci {
		dest, err := jobs.PercorsoContenuto(s.Staging, voci[i].Sha256, voci[i].NomeFile)
		if err != nil {
			return voci, err
		}
		if jobs.ContenutoGiaPresente(dest, voci[i].Sha256) {
			voci[i].Path = dest
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return voci, err
		}
		if err := os.Rename(voci[i].Path, dest); err != nil {
			return voci, fmt.Errorf("voce %s dello zip: %w", voci[i].NomeFile, err)
		}
		voci[i].Path = dest
	}
	return voci, errEstrazione
}
