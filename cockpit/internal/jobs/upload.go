package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
)

// Upload degli allegati legato al tentativo (voce 2.3, D8, N45).
//
// Fino alla fase 1 il worker salvava l'allegato direttamente nella cartella di staging del server e
// nel result dichiarava il percorso. Funzionava per un solo motivo: worker e server erano sullo
// stesso PC. Con il worker su un'altra postazione quel percorso è su un altro disco, e il server non
// ha niente in mano.
//
// Ora il file viaggia con PUT /api/v1/allegati/{id}/file, e viaggia DENTRO il tentativo: la richiesta
// porta job_id e lease_token, il server lo scrive come <definitivo>.parte.<lease_token>, e solo il
// result valido dello stesso tentativo lo promuove a definitivo, dopo aver verificato lo sha256.
// Il token nel nome del file non è un dettaglio: è ciò che impedisce a un tentativo scaduto, che
// finisce di caricare in ritardo, di consegnare il file al posto del tentativo che gli è subentrato
// (M13). Due tentativi scrivono due file diversi; a diventare definitivo è quello del result che
// passa il predicato di validità, e l'altro viene rimosso.

// ErrCartellaStaging: la sottocartella nel payload non è un nome semplice. Il payload lo scrive il
// server (CartellaStaging), quindi qui non dovrebbe mai succedere; si controlla lo stesso perché il
// percorso finisce in una os.Create e un `..` non deve poter uscire dallo staging.
var ErrCartellaStaging = errors.New("sottocartella di staging non valida")

var reCartellaStaging = regexp.MustCompile(`^[a-z0-9]{6,40}$`)
var reNomeVietato = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

// NomeFileStaging è il nome con cui l'allegato sta nello staging: <indice>_<nome sanificato>, con
// .msg aggiunto agli elementi Outlook annidati (che Outlook salva come .msg qualunque nome abbiano).
// Lo decide il server, non il worker: è il server che possiede lo staging.
func NomeFileStaging(a db.Allegato) string {
	nome := reNomeVietato.ReplaceAllString(strings.TrimSpace(a.NomeFile), "_")
	if nome == "" {
		nome = "allegato"
	}
	if r := []rune(nome); len(r) > 120 {
		nome = string(r[:120])
	}
	if a.Natura == db.NaturaAllegatoElementoOutlook && !strings.HasSuffix(strings.ToLower(nome), ".msg") {
		nome += ".msg"
	}
	return fmt.Sprintf("%02d_%s", a.Indice, nome)
}

// PercorsiStaging restituisce il percorso definitivo del file di un allegato e quello del suo
// .parte.<token>. Entrambi stanno sotto staging/<cartella del payload>.
func PercorsiStaging(staging string, p api.PayloadStageAllegato, a db.Allegato, token uuid.UUID) (definitivo, parte string, err error) {
	if !reCartellaStaging.MatchString(p.Cartella) {
		return "", "", fmt.Errorf("%w: %q", ErrCartellaStaging, p.Cartella)
	}
	definitivo = filepath.Join(staging, p.Cartella, NomeFileStaging(a))
	return definitivo, NomeParte(definitivo, token), nil
}

// NomeParte è il file in cui un tentativo carica: <definitivo>.parte.<lease_token>.
func NomeParte(definitivo string, token uuid.UUID) string {
	return definitivo + ".parte." + token.String()
}

var reParte = regexp.MustCompile(`\.parte\.([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

// TokenDiParte riconosce un file .parte.<token> e ne restituisce il token.
func TokenDiParte(nome string) (uuid.UUID, bool) {
	m := reParte.FindStringSubmatch(strings.ToLower(nome))
	if m == nil {
		return uuid.UUID{}, false
	}
	t, err := uuid.Parse(m[1])
	return t, err == nil
}

// Promuovi rende definitivo il file caricato da un tentativo: rinomina atomica di parte su
// definitivo. Se definitivo esiste già (un «Riscarica») viene sostituito: il contenuto nuovo ha
// l'hash che il result dichiara, e quello vecchio non lo vuole più nessuno.
//
// Va chiamata DENTRO la transazione del result, dopo BloccaTentativo: la riga del job è bloccata,
// quindi nessun altro tentativo può diventare valido mentre il file cambia nome. Se il commit poi
// fallisce, il file definitivo resta con il contenuto giusto e il tentativo successivo lo ricarica
// e lo ripromuove tale e quale: l'operazione è ripetibile (M7).
func Promuovi(parte, definitivo string) error {
	if err := os.Rename(parte, definitivo); err != nil {
		return fmt.Errorf("promozione di %s: %w", filepath.Base(parte), err)
	}
	return nil
}

// RimuoviParte toglie il file di un tentativo che non vale più. Un file assente non è un errore.
func RimuoviParte(parte string) {
	if parte == "" {
		return
	}
	if err := os.Remove(parte); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("file parziale non rimosso", "file", parte, "err", err)
	}
}

// PulisciParti rimuove dallo staging i .parte.<token> dei tentativi che non sono più in corso (N8,
// voce 2.3). Sono i file di upload interrotti a metà, di tentativi scaduti durante il trasferimento,
// o di result mai arrivati: nessuno li promuoverà più.
//
// Un file il cui token è ancora in corso non si tocca, qualunque età abbia: un upload da centinaia
// di megabyte su una LAN lenta è ancora un upload. E un file orfano più giovane di etaMinima si
// lascia stare per un giro: fra la fine dell'upload e il result c'è una finestra in cui il token è
// ancora in corso, e non vale la pena giocare sul filo dei secondi.
func PulisciParti(ctx context.Context, q *db.Queries, staging string, etaMinima time.Duration, log *slog.Logger) (int, error) {
	if strings.TrimSpace(staging) == "" {
		return 0, nil
	}
	token, err := q.ListLeaseTokenInCorso(ctx)
	if err != nil {
		return 0, err
	}
	vivi := map[uuid.UUID]bool{}
	for _, t := range token {
		if t.Valid {
			vivi[t.UUID] = true
		}
	}
	limite := time.Now().Add(-etaMinima)
	rimossi := 0
	err = filepath.WalkDir(staging, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil // una cartella illeggibile non ferma la pulizia delle altre
		}
		t, ok := TokenDiParte(d.Name())
		if !ok || vivi[t] {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.ModTime().After(limite) {
			return nil
		}
		if err := os.Remove(p); err != nil {
			log.Warn("file parziale orfano non rimosso", "file", p, "err", err)
			return nil
		}
		rimossi++
		log.Info("file parziale orfano rimosso", "file", p, "token", t)
		return nil
	})
	return rimossi, err
}
