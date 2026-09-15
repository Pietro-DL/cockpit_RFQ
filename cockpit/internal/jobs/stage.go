package jobs

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// Analizzatore descrive con che cosa si analizza: versione e configurazione mandata al worker. Il suo
// Hash entra nella chiave di idempotenza dei job e nella chiave dei fatti conservati (voce 1.12).
type Analizzatore struct {
	Versione  int
	Parametri map[string]any
}

// Hash è lo sha256 della configurazione in forma canonica. Si passa da json.Marshal di una map, che in
// Go ordina le chiavi: lo stesso blocco di parametri dà sempre lo stesso hash, anche se nel file di
// configurazione le righe sono in ordine diverso.
func (an Analizzatore) Hash() string {
	raw, err := json.Marshal(struct {
		V int            `json:"v"`
		P map[string]any `json:"p"`
	}{an.Versione, an.Parametri})
	if err != nil {
		raw = []byte(fmt.Sprintf("%d:%v", an.Versione, an.Parametri))
	}
	somma := sha256.Sum256(raw)
	return hex.EncodeToString(somma[:])
}

// AccodaAnalisi accoda l'analisi Python di un file già in staging (cartiglio PDF, PRODUCT dello STEP).
//
// La chiave di idempotenza è `analizza:<sha256>:<versione>:<configurazione>`, non più per allegato
// (A15). La differenza si vede con lo stesso disegno allegato a tre richieste di due clienti diversi,
// scaricate tutte prima che la prima analisi finisca: con la chiave per allegato partivano tre job che
// leggevano lo stesso file e producevano gli stessi fatti; con questa ne parte uno, e al risultato i
// fatti vengono distribuiti a tutte e tre le proposte ancora aperte.
//
// Se i fatti per quella terna sono già in `analisi_fatti` non si accoda niente e si restituisce
// (nil, nil): il chiamante li riuserà. Cambiare la versione o un parametro cambia la chiave, quindi
// fa ripartire l'analisi — che è precisamente ciò che si vuole quando il dizionario cambia.
func AccodaAnalisi(ctx context.Context, q *db.Queries, a db.Allegato, threadID uuid.NullUUID, an Analizzatore) (*db.Job, error) {
	var tid *uuid.UUID
	if threadID.Valid {
		t := threadID.UUID
		tid = &t
	}
	cfg := an.Hash()
	if a.Sha256.Valid && a.Sha256.String != "" {
		_, err := q.GetAnalisiFatti(ctx, db.GetAnalisiFattiParams{
			Sha256: a.Sha256.String, VersioneAnalizzatore: int16(an.Versione), HashConfigurazione: cfg,
		})
		if err == nil {
			return nil, nil // già calcolati con questa versione e questa configurazione
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	return Accoda(ctx, q, db.TipoJobAnalizzaAllegato, api.PayloadAnalizzaAllegato{
		AllegatoID: a.AllegatoID, PathStaging: a.PathStaging.String, Sha256: a.Sha256.String, NomeFile: a.NomeFile,
		ThreadID: tid, MessaggioID: a.MessaggioID,
		VersioneAnalizzatore: an.Versione, HashConfigurazione: cfg, Parametri: an.Parametri,
	}, fmt.Sprintf("analizza:%s:%d:%s", a.Sha256.String, an.Versione, cfg), 6)
}
