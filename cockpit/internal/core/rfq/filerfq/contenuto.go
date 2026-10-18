package filerfq

import (
	"context"
	"errors"
	"os"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// L'apertura dei byte di un file per S5 (EM §10.9): l'occorrenza e le copie di conservazione. Lo staging e il NAS li
// apre il gestore web con gli aiuti dell'anteprima (nelloStaging, dalNas), che hanno gia' il loro contenimento; qui
// l'occorrenza, il suo stato e il deposito.

// ErrCopiaMancante: l'occorrenza e' in Altri file e la sua copia verificata non c'e' nel deposito (un'anomalia
// d'integrita': S5 risponde 500 con una frase generica, il dettaglio nel log).
var ErrCopiaMancante = errors.New("copia di conservazione mancante")

// OccorrenzaPerContenuto legge l'occorrenza per S5, senza lucchetti; nil se l'allegato non e' della RFQ.
func (s *Servizio) OccorrenzaPerContenuto(ctx context.Context, thread, allegato uuid.UUID) (*Occorrenza, error) {
	return leggiOccorrenza(ctx, db.New(s.Pool), thread, allegato)
}

// ApriCopia apre una copia di conservazione dell'allegato (la copiata della RFQ prima, poi le altre non rilasciate, anche
// sotto un'altra RFQ: EM §10.1, §10.9): il file, la sua dimensione e il suo orario. soloRFQ: solo la copia dell'occorrenza
// della RFQ (per un file in Altri file). Nessuna copia apribile: (nil, nil, nil).
func (s *Servizio) ApriCopia(ctx context.Context, thread, allegato uuid.UUID, soloRFQ bool) (*os.File, os.FileInfo, error) {
	if s.Deposito == nil {
		return nil, nil, nil
	}
	copie, err := db.New(s.Pool).ListCopieConservazioneAllegato(ctx, allegato)
	if err != nil {
		return nil, nil, err
	}
	for _, c := range copie {
		if soloRFQ && (c.ThreadID != thread || c.Stato != "copiata") {
			continue
		}
		if c.DepositoID != s.Deposito.ID {
			continue
		}
		p, err := s.Deposito.PercorsoCopia(c.PercorsoRelativo)
		if err != nil {
			// un percorso che esce dal deposito: un difetto, mai un file da aprire
			s.log().Error("ciclo dei file: percorso di conservazione fuori dal deposito", "conservazione", c.ConservazioneID, "err", err)
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		st, err := f.Stat()
		if err != nil || st.IsDir() || (c.Bytes.Valid && st.Size() != c.Bytes.Int64) {
			f.Close()
			continue
		}
		return f, st, nil
	}
	return nil, nil, nil
}
