package filerfq

import (
	"context"
	"errors"
	"os"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
)

// L'apertura dei byte di un file per S5 (EM §10.9; U2 §3.12): l'occorrenza e le copie di conservazione. Lo staging e
// il NAS dei documenti li apre il gestore web con gli aiuti dell'anteprima (nelloStaging, nasSoloLettura), che hanno
// gia' il loro contenimento; qui l'occorrenza, il suo stato e le copie, aperte dal Conservatore con lo sha verificato
// dallo stesso handle con cui si servono i byte.

// I motivi di una copia degli Altri file che non si serve (X-Motivo di S5, U2 §3.12): mai un 500, mai byte diversi.
const (
	MotivoCopiaMancante = "COPIA_NAS_MANCANTE"
	MotivoCopiaCambiata = "COPIA_NAS_CAMBIATA"
)

// OccorrenzaPerContenuto legge l'occorrenza per S5, senza lucchetti; nil se l'allegato non e' della RFQ.
func (s *Servizio) OccorrenzaPerContenuto(ctx context.Context, thread, allegato uuid.UUID) (*Occorrenza, error) {
	return leggiOccorrenza(ctx, db.New(s.Pool), thread, allegato)
}

// ApriCopia apre una copia di conservazione dell'allegato (la copiata della RFQ prima, poi le altre non rilasciate, anche
// sotto un'altra RFQ: EM §10.1, §10.9): il file, la sua dimensione e il suo orario, con lo sha verificato. soloRFQ: solo
// la copia dell'occorrenza della RFQ (per un file in Altri file); se non si apre, motivo dice perche'
// (MotivoCopiaMancante, MotivoCopiaCambiata). Nessuna copia apribile: (nil, nil, motivo, nil).
func (s *Servizio) ApriCopia(ctx context.Context, thread, allegato uuid.UUID, soloRFQ bool) (*os.File, os.FileInfo, string, error) {
	if s.Conservatore == nil {
		return nil, nil, "", nil
	}
	copie, err := db.New(s.Pool).ListCopieConservazioneAllegato(ctx, allegato)
	if err != nil {
		return nil, nil, "", err
	}
	motivo := ""
	for _, c := range copie {
		if soloRFQ && (c.ThreadID != thread || c.Stato != "copiata") {
			continue
		}
		if c.Stato == "rilasciata" || c.Stato == "errore_copia" || !c.SorgenteByte.Valid {
			continue
		}
		f, st, err := s.Conservatore.Apri(postoDellaRiga(c))
		switch {
		case err == nil:
			return f, st, "", nil
		case errors.Is(err, nas.ErrCopiaCambiata):
			motivo = MotivoCopiaCambiata
		case errors.Is(err, nas.ErrCopiaMancante), errors.Is(err, nas.ErrRadiceDiversa):
			if motivo == "" {
				motivo = MotivoCopiaMancante
			}
		default:
			if errors.Is(err, nas.ErrPercorsoNonValido) {
				// un percorso che esce: un difetto, mai un file da aprire
				s.log().Error("ciclo dei file: percorso di conservazione non valido", "conservazione", c.ConservazioneID, "err", err)
			}
			if motivo == "" {
				motivo = MotivoCopiaMancante
			}
		}
	}
	if soloRFQ && motivo == "" {
		motivo = MotivoCopiaMancante
	}
	return nil, nil, motivo, nil
}
