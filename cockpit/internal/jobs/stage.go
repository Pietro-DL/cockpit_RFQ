package jobs

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

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

// Staging risponde a una sola domanda: quel file c'è ancora sul disco?
//
// È un'interfaccia e non una chiamata a os.Stat dentro AccodaStage perché altrimenti la guardia della
// voce 1.11 sarebbe verificabile solo costruendo alberi di file veri, e un test che dipende da che
// cosa c'è davvero nello staging della macchina non prova quasi niente.
type Staging interface {
	Presente(percorso string) bool
}

// FileStaging è lo staging vero: il disco.
type FileStaging struct{}

func (FileStaging) Presente(percorso string) bool {
	if strings.TrimSpace(percorso) == "" {
		return false
	}
	st, err := os.Stat(percorso)
	return err == nil && !st.IsDir()
}

// EsitoStage dice che cosa è successo alla richiesta di download. Serve a chi la chiama per dire
// all'operatore la verità: «scaricato» e «c'era già» non sono la stessa frase.
type EsitoStage string

const (
	StageAccodato    EsitoStage = "accodato"     // job creato, il worker lo scaricherà
	StageGiaInCoda   EsitoStage = "gia_in_coda"  // un download per questo allegato è già pendente
	StageGiaPresente EsitoStage = "gia_presente" // il file di questo allegato è già in staging
	StageRiusato     EsitoStage = "riusato"      // stesso contenuto già sceso per un altro allegato
)

// AccodaStage accoda il download di un allegato diretto (non dentro uno zip) dal suo elemento Outlook.
// È sempre conseguenza di un'azione dell'operatore: priorità alta. Chiave per allegato: un solo download
// pendente alla volta, ma riaccodabile dopo (file cancellato dallo staging → "Riscarica").
//
// Prima di accodare guarda se il file c'è già (voce 1.11, elaborazione singola). Due controlli, non uno:
//
//  1. il file di QUESTO allegato è al suo posto → non c'è niente da scaricare. Senza questo controllo un
//     secondo clic su «Scarica» rimetteva l'allegato a 'grezzo', cancellava il marcatore di stato e
//     rifaceva il giro in COM per riportare esattamente lo stesso file;
//  2. lo STESSO CONTENUTO è già sceso per un altro allegato → si riusa quel percorso. È il caso dello
//     stesso disegno allegato a richieste diverse: scaricarlo una seconda volta significa un'altra
//     apertura di Outlook e un'altra copia identica sul disco.
//
// I due allegati che condividono il contenuto condividono anche il percorso in staging: chi cancella
// quel file li lascia entrambi senza, ed entrambi hanno "Riscarica". È la stessa condizione in cui si
// trova oggi un allegato solo, quindi non introduce un modo nuovo di rompersi.
//
// `c` è la COPIA da cui scaricare: dalla 0004 lo stesso messaggio ha un EntryID diverso in ogni
// casella, quindi «da quale copia» non è una domanda che si possa saltare. Il job porta casella_id
// (vincolo del claim: lo prende solo un worker che serve quella casella) ed entry_id; MAI uno
// store_id, che il worker risolve nel proprio profilo (voce 2.6, M12). Un download non apre finestre,
// quindi non è legato alla postazione del richiedente: lo fa qualunque worker autorizzato sulla
// casella. Quale copia preferire, quando ce n'è più d'una, lo decide chi chiama (CopiaPerDownload).
func AccodaStage(ctx context.Context, q *db.Queries, st Staging, a db.Allegato, m db.Messaggio, c Copia, priorita int16) (EsitoStage, *db.Job, error) {
	if a.PathStaging.Valid && st.Presente(a.PathStaging.String) {
		return StageGiaPresente, nil, nil
	}
	if a.Sha256.Valid && a.Sha256.String != "" {
		gemello, err := q.AllegatoInStagingPerHash(ctx, db.AllegatoInStagingPerHashParams{Sha256: a.Sha256, Escluso: a.AllegatoID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", nil, fmt.Errorf("allegato già in staging con lo stesso hash: %w", err)
		}
		if err == nil && st.Presente(gemello.PathStaging.String) {
			dim := gemello.Bytes
			if !dim.Valid {
				dim = a.Bytes
			}
			if err := q.SetAllegatoStaging(ctx, db.SetAllegatoStagingParams{
				AllegatoID: a.AllegatoID, PathStaging: gemello.PathStaging, Sha256: a.Sha256, Bytes: dim,
			}); err != nil {
				return "", nil, err
			}
			return StageRiusato, nil, nil
		}
	}
	// "in coda" in errore è il marcatore letto dalla UI finché il worker non consegna il file (SetAllegatoStaging lo azzera)
	if err := q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: a.AllegatoID, Stato: db.StatoAllegatoGrezzo, Errore: pgtype.Text{String: "in coda", Valid: true}}); err != nil {
		return "", nil, err
	}
	mid := m.MessaggioID
	cid := c.CasellaID
	j, err := AccodaCon(ctx, q, db.TipoJobStageAllegato, api.PayloadStageAllegato{
		AllegatoID: a.AllegatoID, EntryID: c.EntryID, Indice: int(a.Indice), NomeFile: a.NomeFile,
		Cartella:            CartellaStaging(m.ChiaveEsterna),
		RiferimentoElemento: api.RiferimentoElemento{MessaggioID: &mid, CasellaID: &cid, MessageID: m.ChiaveEsterna},
	}, "stage:"+a.AllegatoID.String(), priorita, Opzioni{Casella: uuid.NullUUID{UUID: cid, Valid: true}})
	if err != nil {
		return "", nil, err
	}
	if j == nil {
		return StageGiaInCoda, nil, nil
	}
	return StageAccodato, j, nil
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
