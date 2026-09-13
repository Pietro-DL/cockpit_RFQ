package jobs

import (
	"context"
	"crypto/sha1"
	"encoding/hex"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
)

// CartellaStaging è la sottocartella di staging di un messaggio: hash breve del Message-ID.
func CartellaStaging(messageID string) string {
	h := sha1.Sum([]byte(messageID))
	return hex.EncodeToString(h[:])[:12]
}

// AccodaStage accoda il download di un allegato diretto (non dentro uno zip) dal suo elemento Outlook.
// È sempre conseguenza di un'azione dell'operatore: priorità alta. Chiave per allegato: un solo download
// pendente alla volta, ma riaccodabile dopo (file cancellato dallo staging → "Riscarica").
func AccodaStage(ctx context.Context, q *db.Queries, a db.Allegato, m db.Messaggio, o db.MessaggioOutlook, priorita int16) (*db.Job, error) {
	// "in coda" in errore è il marcatore letto dalla UI finché il worker non consegna il file (SetAllegatoStaging lo azzera)
	if err := q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: a.AllegatoID, Stato: db.StatoAllegatoGrezzo, Errore: pgtype.Text{String: "in coda", Valid: true}}); err != nil {
		return nil, err
	}
	mid := m.MessaggioID
	return Accoda(ctx, q, db.TipoJobStageAllegato, api.PayloadStageAllegato{
		AllegatoID: a.AllegatoID, EntryID: o.EntryID, StoreID: o.StoreID, Indice: int(a.Indice), NomeFile: a.NomeFile,
		Cartella: CartellaStaging(m.ChiaveEsterna), RiferimentoElemento: api.RiferimentoElemento{MessaggioID: &mid, MessageID: m.ChiaveEsterna},
	}, "stage:"+a.AllegatoID.String(), priorita)
}

// AccodaAnalisi accoda l'analisi Python di un file già in staging (cartiglio PDF, PRODUCT dello STEP).
func AccodaAnalisi(ctx context.Context, q *db.Queries, a db.Allegato, threadID uuid.NullUUID) (*db.Job, error) {
	var tid *uuid.UUID
	if threadID.Valid {
		t := threadID.UUID
		tid = &t
	}
	return Accoda(ctx, q, db.TipoJobAnalizzaAllegato, api.PayloadAnalizzaAllegato{
		AllegatoID: a.AllegatoID, PathStaging: a.PathStaging.String, Sha256: a.Sha256.String, NomeFile: a.NomeFile,
		ThreadID: tid, MessaggioID: a.MessaggioID,
	}, "analizza:"+a.AllegatoID.String(), 6)
}
