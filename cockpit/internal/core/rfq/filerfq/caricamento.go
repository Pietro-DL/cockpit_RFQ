package filerfq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
)

// L'ingresso dei file caricati (E1′, il ramo JSON di POST /thread/{id}/carica; EM §10.9). Per file, una transazione
// sua: BloccaMessaggio(nota) e il controllo dei doppioni sotto il lucchetto (attivo o in Altri file → gia_presente;
// scartato, anche col gesto precedente, non conta: il file diventa un'occorrenza nuova; giro 1, TR-10), poi il contenuto
// adottato sotto BloccaContenuto (fascicolo.AdottaContenuto: il parziale si promuove se il contenuto e' sparito), la
// registrazione, la strada d'ingresso, e l'evento ricaricato_dopo_scarto sulle occorrenze scartate dello stesso
// contenuto, che lo scrive questo pacchetto per tutti e due i rami (origine ponte o legacy; giro 1, SIC-13). Nessuna
// riga di stato, nessun legame a componenti o nodi, nessuna copia sul NAS.

// FileArrivato e' un file letto dalla richiesta e scritto in un parziale dello staging, con lo sha calcolato scrivendo.
type FileArrivato struct {
	Nome   string
	Sha256 string
	Bytes  int64
	Tipo   string // content type dichiarato, solo informativo
	Parte  string // il parziale (non ancora promosso)
}

// ErrFileVuoto, ErrTroppoGrande: i rifiuti di un file mentre arriva.
var (
	ErrFileVuoto    = errors.New("file vuoto")
	ErrTroppoGrande = errors.New("file oltre il limite")
)

// PrefissoParteCiclo: i parziali dello staging che scrive il servizio del ciclo dei file (E1′ e «Riattiva») sono
// <staging>\_parti\ciclo.<uuid>. Non hanno la forma «<x>.parte.<token>» dei tentativi dei worker: quelli li toglie la
// pulizia della coda (staging.PulisciParti, ogni 15 minuti) dopo 10 minuti se il token non e' il lease di un job vivo,
// e un caricamento lungo tiene il parziale di un file gia' arrivato finche' non ha letto tutta la richiesta (EM §10.9:
// la registrazione viene dopo il corpo; rilievo della revisione). Questi li toglie chi li scrive (promossi o buttati);
// quelli rimasti da un processo morto li toglie il custode del ciclo dei file dopo EtaPartiCiclo.
const PrefissoParteCiclo = "ciclo."

// EtaPartiCiclo: un parziale del ciclo dei file piu' vecchio di cosi' non e' di una richiesta ancora in corso (otto
// volte max_upload_mb su un collegamento lento sono qualche ora, non un giorno).
const EtaPartiCiclo = 24 * time.Hour

// percorsoParteCiclo: un parziale nuovo del ciclo dei file nello staging (creazione esclusiva: il nome e' nuovo).
func percorsoParteCiclo(radice string) string {
	return filepath.Join(radice, staging.CartellaParti, PrefissoParteCiclo+uuid.NewString())
}

// PulisciPartiCiclo toglie i parziali del ciclo dei file piu' vecchi di EtaPartiCiclo, cioe' quelli di una richiesta
// che il processo non ha finito (chi la finisce toglie i suoi). Restituisce quanti ne ha tolti. Senza staging, niente.
func PulisciPartiCiclo(radice string, adesso time.Time, log *slog.Logger) int {
	if strings.TrimSpace(radice) == "" {
		return 0
	}
	cartella := filepath.Join(radice, staging.CartellaParti)
	voci, err := os.ReadDir(cartella)
	if err != nil {
		return 0
	}
	n := 0
	for _, v := range voci {
		if v.IsDir() || !strings.HasPrefix(v.Name(), PrefissoParteCiclo) {
			continue
		}
		info, err := v.Info()
		if err != nil || adesso.Sub(info.ModTime()) < EtaPartiCiclo {
			continue
		}
		if err := os.Remove(filepath.Join(cartella, v.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Warn("ciclo dei file: parziale dello staging rimasto non rimosso", "file", v.Name(), "err", err)
			continue
		}
		n++
	}
	return n
}

// ScriviParziale legge un file della richiesta in un parziale dello staging (creazione esclusiva, sha calcolato
// scrivendo), senza promuoverlo: lo promuove la registrazione sotto il lucchetto per contenuto (EM §10.6; SIC-06).
// Oltre max byte: ErrTroppoGrande; vuoto: ErrFileVuoto; il parziale si toglie. Il nome del parziale non e' quello dei
// tentativi dei worker (PrefissoParteCiclo): la pulizia della coda non lo tocca mentre il resto della richiesta arriva.
func ScriviParziale(radice string, r io.Reader, nome, tipo string, max int64) (FileArrivato, error) {
	parte := percorsoParteCiclo(radice)
	if err := os.MkdirAll(filepath.Dir(parte), 0o755); err != nil {
		return FileArrivato{}, err
	}
	out, err := os.OpenFile(parte, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return FileArrivato{}, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(r, max+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	switch {
	case err != nil:
		staging.RimuoviParte(parte)
		return FileArrivato{}, err
	case n > max:
		staging.RimuoviParte(parte)
		return FileArrivato{}, ErrTroppoGrande
	case n == 0:
		staging.RimuoviParte(parte)
		return FileArrivato{}, ErrFileVuoto
	}
	return FileArrivato{Nome: nome, Sha256: hex.EncodeToString(h.Sum(nil)), Bytes: n, Tipo: tipo, Parte: parte}, nil
}

// EsitoFile e' l'esito di un file di E1′ (EM §10.9).
type EsitoFile struct {
	NomeFile             string         `json:"nome_file"`
	Esito                string         `json:"esito"` // caricato | gia_presente | rifiutato
	AllegatoID           *uuid.UUID     `json:"allegato_id"`
	PropostaID           *uuid.UUID     `json:"proposta_id"`
	Sha256               string         `json:"sha256"`
	Origine              string         `json:"origine"`
	Analisi              string         `json:"analisi"`
	StatoFile            string         `json:"stato_file"`
	RicaricatoDopoScarto bool           `json:"ricaricato_dopo_scarto"`
	Motivo               *MotivoRifiuto `json:"motivo"`
}

// MotivoRifiuto: il codice e il testo del rifiuto di un file.
type MotivoRifiuto struct {
	Codice string `json:"codice"`
	Testo  string `json:"testo"`
}

// Doppione: i doppioni di un contenuto nel contenitore, letti dopo BloccaMessaggio (EM §10.9, E1′): presente e' il
// doppione attivo o in Altri file (gia_presente), scartate le occorrenze scartate (anche col gesto precedente), che non
// contano e ricevono l'evento ricaricato_dopo_scarto.
type Doppione struct {
	Presente      *db.ListAllegatiDirettiStessoShaRow
	Scartate      []uuid.UUID
	StatoPresente string
}

// CercaDoppione: chi chiama ha bloccato il messaggio (BloccaMessaggio) nella stessa transazione.
func CercaDoppione(ctx context.Context, q *db.Queries, messaggio uuid.UUID, sha string) (Doppione, error) {
	var d Doppione
	righe, err := q.ListAllegatiDirettiStessoSha(ctx, db.ListAllegatiDirettiStessoShaParams{MessaggioID: messaggio, Sha256: sha})
	if err != nil {
		return d, err
	}
	for i := range righe {
		r := righe[i]
		if r.Stato == StatoScartato || r.ScartatoLegacy {
			d.Scartate = append(d.Scartate, r.AllegatoID)
			continue
		}
		if d.Presente == nil {
			d.Presente = &righe[i]
			d.StatoPresente = r.Stato
		}
	}
	return d, nil
}

// SegnaRicaricato scrive l'evento informativo ricaricato_dopo_scarto su ogni occorrenza scartata dello stesso
// contenuto, con l'allegato della nuova (EM §10.4, «Ricaricare dopo uno scarto»). origine: ponte (E1′) o legacy (il
// ramo HTML di carica).
func SegnaRicaricato(ctx context.Context, q *db.Queries, thread uuid.UUID, scartate []uuid.UUID, nuovo, autore uuid.UUID, origine string) error {
	for _, id := range scartate {
		if _, err := scriviEvento(ctx, q, thread, evento{o: Occorrenza{db.ListOccorrenzeSorgenteRow{AllegatoID: id}},
			azione: "ricaricato_dopo_scarto", altro: uid(nuovo), origine: origine, autore: uid(autore)}); err != nil {
			return err
		}
	}
	return nil
}

// OriginePortale, OrigineInterna: le due origini di un caricamento E1′ (il campo «origine» della richiesta e di ogni file
// della risposta, EM §10.9). Nella riga dell'allegato diventano portale e manuale.
const (
	OriginePortale = "portale"
	OrigineInterna = "interna"
)

// RegistraFile registra un file arrivato nella RFQ (E1′), in una transazione sua (EM §10.9). interna: il contenitore
// dei caricamenti interni (origine dell'allegato manuale); altrimenti quello dei file del cliente (origine portale). La
// risposta usa il vocabolario della richiesta (portale | interna) per ogni file, come per i file rifiutati e in testa
// (rilievo della revisione). Il parziale si promuove o si butta qui; in ogni caso, all'uscita, non resta.
func (s *Servizio) RegistraFile(ctx context.Context, thread uuid.UUID, u db.Utente, interna bool, f FileArrivato) (EsitoFile, error) {
	origine := string(db.OrigineAllegatoPortale)
	risposta := OriginePortale
	if interna {
		origine, risposta = string(db.OrigineAllegatoManuale), OrigineInterna
	}
	es := EsitoFile{NomeFile: f.Nome, Sha256: f.Sha256, Origine: risposta}
	defer staging.RimuoviParte(f.Parte) // promosso o buttato, il parziale non resta (un file assente non e' un errore)
	def, err := staging.PercorsoContenuto(s.Staging, f.Sha256, f.Nome)
	if err != nil {
		return es, err
	}
	err = s.inTx(ctx, func(q *db.Queries) error {
		var nota db.Messaggio
		var err error
		if interna {
			nota, err = fascicolo.NotaInterna(ctx, q, thread, u)
		} else {
			nota, err = fascicolo.NotaPortale(ctx, q, thread, u)
		}
		if err != nil {
			return err
		}
		if _, err := q.BloccaMessaggio(ctx, nota.MessaggioID); err != nil {
			return err
		}
		d, err := CercaDoppione(ctx, q, nota.MessaggioID, f.Sha256)
		if err != nil {
			return err
		}
		if d.Presente != nil {
			id := d.Presente.AllegatoID
			es.Esito, es.AllegatoID, es.StatoFile = "gia_presente", &id, d.StatoPresente
			if p, err := q.GetPropostaDocumentoDiAllegato(ctx, id); err == nil {
				pid := p.PropostaID
				es.PropostaID = &pid
			}
			return nil
		}
		car := fascicolo.Caricato{Nome: f.Nome, Sha256: f.Sha256, Bytes: f.Bytes, Percorso: def, Tipo: f.Tipo, Parte: f.Parte,
			Origine: db.OrigineAllegato(origine)}
		a, err := fascicolo.RegistraCaricamento(ctx, q, nota, u.UtenteID, car)
		if err != nil {
			return err
		}
		if s.Pipeline != nil {
			if err := s.Pipeline.DopoCaricamento(ctx, q, a.AllegatoID); err != nil {
				return err
			}
		}
		if err := SegnaRicaricato(ctx, q, thread, d.Scartate, a.AllegatoID, u.UtenteID, OriginePonte); err != nil {
			return err
		}
		id := a.AllegatoID
		es.Esito, es.AllegatoID, es.StatoFile, es.RicaricatoDopoScarto = "caricato", &id, StatoAttivo, len(d.Scartate) > 0
		if p, err := q.GetPropostaDocumentoDiAllegato(ctx, id); err == nil {
			pid := p.PropostaID
			es.PropostaID = &pid
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return es, err
	}
	es.Analisi = s.analisiDi(ctx, thread, es.AllegatoID)
	return es, nil
}

// analisiDi: lo stato dell'analisi di un allegato (lo stesso di S1), letto dopo il COMMIT.
func (s *Servizio) analisiDi(ctx context.Context, thread uuid.UUID, id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	q := db.New(s.Pool)
	a, err := q.GetAllegato(ctx, *id)
	if err != nil {
		return ""
	}
	lav, err := q.ListLavoriPendentiSorgente(ctx, thread)
	if err != nil {
		return ""
	}
	pend := map[string]bool{}
	for _, l := range lav {
		if l.AllegatoID == *id {
			pend[l.Tipo] = true
		}
	}
	return statoAnalisi(a.Stato, a.Errore.String, pend).Stato
}

// SegnaAnomaliaRiestrazione scrive l'evento informativo anomalia_riestrazione su una voce non attiva di un archivio,
// quando la riestrazione mette allo stesso indice un file con un altro nome o un altro sha della sua fotografia (EM
// §10.7, G-7). Lo chiama l'estrazione (workerapi): l'evento lo scrive questo pacchetto (CO-12).
func SegnaAnomaliaRiestrazione(ctx context.Context, q *db.Queries, thread, voce uuid.UUID, esito map[string]any) error {
	_, err := scriviEvento(ctx, q, thread, evento{o: Occorrenza{db.ListOccorrenzeSorgenteRow{AllegatoID: voce}},
		azione: "anomalia_riestrazione", esito: esito, origine: OrigineServer})
	return err
}
