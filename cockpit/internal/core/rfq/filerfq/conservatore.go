package filerfq

// conservatore.go — il posto delle copie degli «Altri file» dietro un'interfaccia (V1, U2 §3.13; [U] Q06 e C-02 B): il
// ciclo dei file (fase A, copia, fase C, battito, rilascio, «Riattiva», S5, custode) non sa se scrive sul NAS o in una
// cartella di prova.
//   - In produzione l'implementazione e' nas.Conservatore sulla radice del NAS, l'unico scrittore del NAS: le copie
//     stanno nella cartella ALTRO della RFQ. Il cablaggio in app/runtime arriva con il delta della 0023 (righe 1-5 di
//     U2 §3.14: cartella_thread, percorso ALTRO\<nome>, prova di creazione): finche' manca, il servizio di produzione
//     non ha conservatore e «Metti in Altri file» risponde 503 NON_CONFIGURATO.
//   - Nelle prove L1 e L4 lo stesso nas.Conservatore su una cartella temporanea (conservatoreprova.Locale, il doppio
//     che ha preso il posto del deposito locale): mai in configurazione, mai costruito da un file di produzione (prova
//     statica TestConservatoreLocaleSoloNelleProve).

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
)

// Conservatore e' il contratto del posto delle copie (U2 §3.13): il comportamento dei metodi e' quello di
// nas.Conservatore, che lo implementa.
type Conservatore interface {
	// Disponibile: le copie si possono scrivere ora (radice configurata, scrittura sul NAS accesa e non DryRun); il
	// motivo va nel 503 (nas, nas_scrittura).
	Disponibile() (ok bool, motivo string)
	// RadiceID: l'impronta della radice, registrata in ogni riga (radice_id, 0023).
	RadiceID() uuid.UUID
	// Copia: parziale esclusivo, sha calcolato scrivendo, verifica, promozione senza sostituzione; mai sovrascrive.
	Copia(p nas.Posto, token uuid.UUID, src io.Reader) (nas.EsitoCopia, error)
	// Leggi: sola lettura, senza verifica (chi legge confronta lo sha mentre copia).
	Leggi(p nas.Posto) (*os.File, error)
	// Apri: sola lettura, sha verificato dallo stesso handle con cui si servono i byte.
	Apri(p nas.Posto) (*os.File, os.FileInfo, error)
	// Stato: presente, cambiata, non trovata, estranea (per il custode e per la cache).
	Stato(p nas.Posto, conSha bool) (nas.StatoCopia, error)
	// Rimuovi: la rimozione con la rinomina (U2 §3.7); chi chiama ha controllato la prova di creazione.
	Rimuovi(p nas.Posto, token uuid.UUID, ripresa bool) nas.EsitoRimozione
	// PulisciPartiCopie: i parziali scaduti e non vivi della cartella del posto (U2 §3.9).
	PulisciPartiCopie(p nas.Posto, vivi map[uuid.UUID]bool, limite time.Time) (int, error)
	// Cartelle: lo scrittore delle cartelle sotto la stessa radice e con la stessa scrittura, per creare la cartella
	// della RFQ che non c'e' ancora (documenti.CreaCartellaThread; U2 §3.2, U2-P07).
	Cartelle() *nas.Scrittore
}

// Il conservatore di produzione soddisfa il contratto.
var _ Conservatore = (*nas.Conservatore)(nil)

// postoDi: il posto della copia di una riga di conservazione (U2 §3.2, §3.4): la radice in cui e' stata scritta, la
// cartella della RFQ fotografata nella riga (cartella_thread) e il percorso ALTRO\<nome> relativo a lei.
func postoDi(id uuid.UUID, cartella, percorso, sha string, bytes int64) nas.Posto {
	return nas.Posto{RadiceID: id, Cartella: cartella, Percorso: percorso, Sha256: strings.ToLower(strings.TrimSpace(sha)), Bytes: bytes}
}

// postoDellaRiga: il posto di una riga letta dal database.
func postoDellaRiga(c db.FileRfqConservazione) nas.Posto {
	b := int64(-1)
	if c.Bytes.Valid {
		b = c.Bytes.Int64
	}
	return postoDi(c.RadiceID, c.CartellaThread.String, c.PercorsoRelativo, c.Sha256, b)
}

// disponibile: il conservatore c'e' ed e' scrivibile; altrimenti il motivo del 503 (conservazione = nessun posto per gli
// Altri file su questo server).
func (s *Servizio) disponibile() (bool, string) {
	if s.Conservatore == nil {
		return false, "conservazione"
	}
	return s.Conservatore.Disponibile()
}

// CopiaNelDeposito: la verifica che la cache usa per non contare il pin di un file in Altri file (staging.Cache): la
// copia verificata c'e' con la dimensione attesa, nella cartella ALTRO della RFQ sotto la radice del conservatore. Nil
// senza conservatore (il pin resta: la cache non toglie lo staging di un file in Altri file).
func (s *Servizio) CopiaNelDeposito() func(string, string, uuid.UUID, int64) bool {
	if s == nil || s.Conservatore == nil {
		return nil
	}
	return func(cartella, percorso string, radice uuid.UUID, bytes int64) bool {
		st, err := s.Conservatore.Stato(nas.Posto{RadiceID: radice, Cartella: cartella, Percorso: percorso, Bytes: bytes}, false)
		return err == nil && st == nas.CopiaPresente
	}
}
