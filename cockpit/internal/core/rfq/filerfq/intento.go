package filerfq

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

// EsitoGesto e' la risposta 200 di S2, S3 e S4 (EM §10.9), registrata in esito_intento prima del COMMIT. Il reinvio con
// la stessa chiave e lo stesso corpo la ridà con riproposto = true e gli stati di rimozione e conservazione letti adesso.
type EsitoGesto struct {
	Esito             string               `json:"esito"`
	Riproposto        bool                 `json:"riproposto"`
	ChiaveIdempotenza uuid.UUID            `json:"chiave_idempotenza"`
	AllegatoID        uuid.UUID            `json:"allegato_id"`
	Stato             string               `json:"stato"`
	Versione          int32                `json:"versione"`
	Rimozioni         []StatoRimozione     `json:"rimozioni,omitempty"`
	Conservazioni     []StatoConservazione `json:"conservazioni,omitempty"`
	ByteRipristinati  *bool                `json:"byte_ripristinati,omitempty"`
	Analisi           string               `json:"analisi,omitempty"`
	Conservazione     *StatoConservazione  `json:"conservazione,omitempty"`
	Testo             string               `json:"testo"`
	// occorrenze: tutte le occorrenze del gesto (l'archivio e le voci), per rileggere gli stati e fare i tentativi.
	Occorrenze []uuid.UUID `json:"occorrenze"`
}

// StatoRimozione: lo stato della rimozione di un'occorrenza scartata, con il testo della tabella di EM §10.4.
type StatoRimozione struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	Stato      string    `json:"stato"`
	Motivo     string    `json:"motivo"`
	Testo      string    `json:"testo"`
}

// StatoConservazione: lo stato dell'ultima copia di conservazione di un'occorrenza.
type StatoConservazione struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	Stato      string    `json:"stato"`
	// Motivo: l'esito della rimozione dalla cartella ALTRO (esito_rimozione, U2 §3.7), quando c'e'.
	Motivo       string     `json:"motivo,omitempty"`
	VerificatoIl *time.Time `json:"verificato_il,omitempty"`
	Bytes        *int64     `json:"bytes,omitempty"`
}

// intento: la chiave d'intento di un gesto con il suo contesto (EM §8.4).
type intento struct {
	chiave   uuid.UUID
	gesto    string
	utente   uuid.UUID
	impronta string
}

// cercaIntento cerca la chiave subito dopo BloccaThread (EM §8.4; C3 §5.3 n. 2): la stessa RFQ, lo stesso gesto, lo
// stesso utente e la stessa impronta → l'esito registrato; un'altra RFQ → 422 FUORI_RFQ; altrimenti 409 CHIAVE_RIUSATA.
// Nessuna riga → nil, e il gesto va avanti.
func cercaIntento(ctx context.Context, q *db.Queries, thread uuid.UUID, it intento) (*EsitoGesto, error) {
	e, err := q.GetEsitoIntentoPerChiave(ctx, it.chiave)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if e.ThreadID != thread {
		return nil, fuoriRFQ("Niente è cambiato: questa chiave d'intento è di un'altra RFQ.")
	}
	if e.Gesto != it.gesto || e.UtenteID != it.utente || e.ImprontaCorpo != it.impronta {
		return nil, chiaveRiusata()
	}
	var out EsitoGesto
	if err := json.Unmarshal(e.Esito, &out); err != nil {
		return nil, err
	}
	out.Riproposto = true
	return &out, nil
}

// registraIntento scrive l'esito nella transazione del gesto, prima del COMMIT.
func registraIntento(ctx context.Context, q *db.Queries, thread uuid.UUID, it intento, esito *EsitoGesto) error {
	b, err := json.Marshal(esito)
	if err != nil {
		return err
	}
	_, err = q.InsertEsitoIntento(ctx, db.InsertEsitoIntentoParams{Chiave: it.chiave, ThreadID: thread, Gesto: it.gesto,
		UtenteID: it.utente, ImprontaCorpo: it.impronta, Esito: b})
	return err
}

// statiCorrenti rilegge le rimozioni e le ultime copie di conservazione delle occorrenze (per la risposta e il reinvio).
func statiCorrenti(ctx context.Context, q *db.Queries, thread uuid.UUID, ids []uuid.UUID) ([]StatoRimozione, []StatoConservazione, error) {
	if len(ids) == 0 {
		return []StatoRimozione{}, []StatoConservazione{}, nil
	}
	rr, err := q.ListRimozioniOccorrenze(ctx, db.ListRimozioniOccorrenzeParams{ThreadID: thread, Allegati: ids})
	if err != nil {
		return nil, nil, err
	}
	rim := make([]StatoRimozione, 0, len(rr))
	for _, r := range rr {
		rim = append(rim, StatoRimozione{AllegatoID: r.AllegatoID, Stato: r.Stato, Motivo: r.Motivo, Testo: TestoRimozione(r.Stato, r.Motivo)})
	}
	cc, err := q.ListUltimaConservazione(ctx, db.ListUltimaConservazioneParams{ThreadID: thread, Allegati: ids})
	if err != nil {
		return nil, nil, err
	}
	cons := make([]StatoConservazione, 0, len(cc))
	for _, c := range cc {
		sc := StatoConservazione{AllegatoID: c.AllegatoID, Stato: c.Stato, Motivo: c.EsitoRimozione.String, VerificatoIl: c.VerificatoIl}
		if c.Bytes.Valid {
			b := c.Bytes.Int64
			sc.Bytes = &b
		}
		cons = append(cons, sc)
	}
	return rim, cons, nil
}

// inTx esegue fn in una transazione: COMMIT se fn non sbaglia.
func (s *Servizio) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(db.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
