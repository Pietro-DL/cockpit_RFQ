package nas

// altro_rimozione.go — la rimozione di una copia del cockpit da una cartella delle persone (U2 §3.7), senza mai
// sostituire un file e senza dire «eliminato» per un file che c'e' ancora:
//   (a) rinomina esclusiva del file in un nome di servizio privato, .<nome>.cockpit-rimozione.<token>, subito nascosto
//       (il punto iniziale su Windows non nasconde niente): il file sparisce dal nome che le persone usano;
//   (b) apertura esclusiva del nome di servizio, dimensione e sha letti dallo stesso handle;
//   (b′) apertura esclusiva non riuscita (un altro programma ha il file aperto con la condivisione in cancellazione):
//       rinomina all'indietro, «in attesa / file occupato»;
//   (c) se tornano: chiusura e rimozione del nome di servizio, che solo il cockpit conosce;
//   (d) se non tornano (una persona l'ha cambiato): rinomina all'indietro; se il nome originale e' stato ripreso, il
//       file resta con il nome di servizio e l'esito e' l'errore nome_ripreso.
// La ripresa dopo un'interruzione cerca il nome di servizio con il token: presente, si riparte da (b).

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// Gli esiti e i motivi della rimozione dal NAS (U2 §3.7, la tabella degli stati).
const (
	RimozioneEseguita       = "eseguita"
	RimozioneInAttesa       = "in_attesa"
	RimozioneNonNecessaria  = "non_necessaria"
	RimozioneErrore         = "errore"
	MotivoRimosso           = "rimosso"
	MotivoGiaAssente        = "gia_assente"
	MotivoFileCambiato      = "file_cambiato"
	MotivoFileOccupato      = "file_occupato"
	MotivoScritturaSpenta   = "scrittura_nas_spenta"
	MotivoNasIrraggiungibil = "nas_irraggiungibile"
	MotivoRadiceCambiata    = "radice_cambiata"
	MotivoFuoriCartella     = "fuori_cartella"
	MotivoNomeRipreso       = "nome_ripreso"
	// i motivi che decide chi chiama, dal database (U2 §3.7): la copia non ha la prova di creazione, o il suo percorso
	// e' quello di un documento confermato. Il file resta.
	MotivoNonNostro             = "non_nostro"
	MotivoPercorsoDiUnDocumento = "percorso_di_un_documento"
)

// EsitoRimozione: l'esito e il motivo (U2 §3.7); NomeServizio e' il nome, relativo alla cartella base, con cui il file
// resta quando l'esito e' nome_ripreso (la Sorgente lo dice all'amministratore).
type EsitoRimozione struct {
	Esito        string
	Motivo       string
	NomeServizio string
}

// Ritentabile: un esito in attesa si riprova; un errore lo chiude solo una bonifica.
func (e EsitoRimozione) Ritentabile() bool { return e.Esito == RimozioneInAttesa }

// NomeServizio: il nome di servizio della rimozione di <nome> con il token.
func NomeServizio(nome string, token uuid.UUID) string {
	return "." + nome + ".cockpit-rimozione." + token.String()
}

// Rimuovi toglie la copia del posto con il protocollo della rinomina (U2 §3.7). Chi chiama ha gia' controllato, nel
// database, che la copia e' del cockpit (la prova di creazione), che nessun documento nomina quel percorso e che la
// causa e' autorizzata; qui si controllano la radice, il contenimento e lo sha. Il token e' quello della rimozione,
// lo stesso nelle riprese. ripresa: la riga era gia' in rimozione (un'esecuzione interrotta): senza il nome di servizio,
// un file sul nome originale con un altro contenuto e' di una persona e non si tocca nemmeno per un istante.
func (c *Conservatore) Rimuovi(p Posto, token uuid.UUID, ripresa bool) EsitoRimozione {
	if ok, _ := c.Disponibile(); !ok {
		return EsitoRimozione{Esito: RimozioneInAttesa, Motivo: MotivoScritturaSpenta}
	}
	if c.controllaRadice(p) != nil {
		return EsitoRimozione{Esito: RimozioneErrore, Motivo: MotivoRadiceCambiata}
	}
	base, file, err := c.percorsi(p)
	if err != nil {
		return EsitoRimozione{Esito: RimozioneErrore, Motivo: MotivoFuoriCartella}
	}
	if st, err := os.Stat(base); err != nil || !st.IsDir() {
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return EsitoRimozione{Esito: RimozioneInAttesa, Motivo: MotivoNasIrraggiungibil}
		}
		// la cartella base non c'e' (spostata o rinominata): la copia non e' al percorso registrato
		return EsitoRimozione{Esito: RimozioneEseguita, Motivo: MotivoGiaAssente}
	}
	dir, nome := filepath.Split(file)
	servizio := filepath.Join(dir, NomeServizio(nome, token))
	relServizio := servizio
	if r, err := filepath.Rel(base, servizio); err == nil {
		relServizio = r
	}
	// (a) la rinomina esclusiva, o la ripresa se il nome di servizio c'e' gia'
	if _, err := os.Lstat(servizio); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return EsitoRimozione{Esito: RimozioneInAttesa, Motivo: MotivoNasIrraggiungibil}
		}
		if ripresa {
			h, n, err := Sha256File(file)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				return EsitoRimozione{Esito: RimozioneEseguita, Motivo: MotivoGiaAssente}
			case err != nil:
				if fileInUso(err) {
					return EsitoRimozione{Esito: RimozioneInAttesa, Motivo: MotivoFileOccupato}
				}
				return EsitoRimozione{Esito: RimozioneInAttesa, Motivo: MotivoNasIrraggiungibil}
			case h != strings.ToLower(p.Sha256) || (p.Bytes >= 0 && n != p.Bytes):
				return EsitoRimozione{Esito: RimozioneNonNecessaria, Motivo: MotivoFileCambiato}
			}
		}
		if err := rinominaSenzaSostituire(file, servizio); err != nil {
			switch {
			case errors.Is(err, fs.ErrNotExist):
				return EsitoRimozione{Esito: RimozioneEseguita, Motivo: MotivoGiaAssente}
			case fileInUso(err):
				return EsitoRimozione{Esito: RimozioneInAttesa, Motivo: MotivoFileOccupato}
			}
			return EsitoRimozione{Esito: RimozioneInAttesa, Motivo: MotivoNasIrraggiungibil}
		}
		nascondi(servizio)
	}
	indietro := func(motivoSeRiesce EsitoRimozione) EsitoRimozione {
		if err := rinominaSenzaSostituire(servizio, file); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return EsitoRimozione{Esito: RimozioneErrore, Motivo: MotivoNomeRipreso, NomeServizio: relServizio}
			}
			// il ritorno non e' riuscito per un altro motivo: il file resta con il nome di servizio, si riprova
			return EsitoRimozione{Esito: RimozioneInAttesa, Motivo: MotivoNasIrraggiungibil}
		}
		return motivoSeRiesce
	}
	// (b) l'apertura esclusiva e la verifica dallo stesso handle
	f, err := apriEsclusivo(servizio)
	if err != nil {
		if fileInUso(err) {
			// (b′) aperto da un altro programma con la condivisione in cancellazione
			return indietro(EsitoRimozione{Esito: RimozioneInAttesa, Motivo: MotivoFileOccupato})
		}
		return indietro(EsitoRimozione{Esito: RimozioneInAttesa, Motivo: MotivoNasIrraggiungibil})
	}
	h, n, errSha := Sha256Da(f)
	f.Close()
	if errSha != nil {
		return indietro(EsitoRimozione{Esito: RimozioneInAttesa, Motivo: MotivoNasIrraggiungibil})
	}
	if h != strings.ToLower(p.Sha256) || (p.Bytes >= 0 && n != p.Bytes) {
		// (d) cambiato da una persona: torna al suo nome e resta
		return indietro(EsitoRimozione{Esito: RimozioneNonNecessaria, Motivo: MotivoFileCambiato})
	}
	// (c) il nome di servizio, privato, si toglie
	if err := os.Remove(servizio); err != nil && !errors.Is(err, fs.ErrNotExist) {
		if fileInUso(err) {
			return EsitoRimozione{Esito: RimozioneInAttesa, Motivo: MotivoFileOccupato}
		}
		return EsitoRimozione{Esito: RimozioneInAttesa, Motivo: MotivoNasIrraggiungibil}
	}
	return EsitoRimozione{Esito: RimozioneEseguita, Motivo: MotivoRimosso}
}
