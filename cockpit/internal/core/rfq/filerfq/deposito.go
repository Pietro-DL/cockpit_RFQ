package filerfq

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/db"
)

// NomeSegnaposto e' il file che il server scrive nella radice del deposito alla prima creazione, con un UUID nuovo:
// ogni copia registra quell'UUID (deposito_id). All'avvio il server controlla che le copie vive siano di questa radice
// (EM §10.5, «creazione e segnaposto»; giro 1, SIC-02).
const NomeSegnaposto = ".cockpit-deposito"

// CartellaPartiDeposito: i parziali delle copie in corso, <radice>\_parti\<conservazione_id>.<casuale>.
const CartellaPartiDeposito = "_parti"

// Deposito e' il deposito degli «Altri file»: una cartella su un disco locale del server, fuori dallo staging e dal NAS
// (i percorsi li controlla platform/config), con il suo segnaposto. Un file per copia:
// <radice>\<thread_id>\<allegato_id>.<conservazione_id>.<ext>, calcolato dal database (trigger di
// file_rfq_conservazione), mai dal client ne' dal Go.
type Deposito struct {
	Radice string
	ID     uuid.UUID
}

// PreparaDeposito apre il deposito all'avvio (EM §10.5):
//   - radice vuota: nessun deposito (nil, nil): «Metti in Altri file» rispondera' 503 NON_CONFIGURATO;
//   - segnaposto presente: le copie vive (copiata, da_rilasciare) devono essere tutte di questo deposito, altrimenti il
//     server non parte e lo dice (radice cambiata, cartella ricreata vuota);
//   - segnaposto assente: se esistono copie vive il server non parte (le copie registrate non sono in quella cartella);
//     senza copie vive crea la cartella e il segnaposto con un UUID nuovo.
//
// Le righe in_copia non contano: un tentativo di copia in corso con un altro deposito si chiude da solo (il custode
// lo porta a da_rilasciare, e il rilascio non tocca il disco di un altro deposito).
func PreparaDeposito(ctx context.Context, q *db.Queries, radice string) (*Deposito, error) {
	radice = strings.TrimSpace(radice)
	if radice == "" {
		return nil, nil
	}
	radice = filepath.Clean(radice)
	id, presente, err := leggiSegnaposto(radice)
	if err != nil {
		return nil, err
	}
	if presente {
		n, err := q.ContaConservazioniViveDeposito(ctx, id)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			return nil, fmt.Errorf("deposito degli Altri file %s: il segnaposto dice il deposito %s, ma %d copie di conservazione "+
				"registrate sono di un altro deposito: le copie non sono in questa cartella (radice cambiata o cartella ricreata). "+
				"Rimettere la cartella giusta in [conservazione].radice: il server non parte", radice, id, n)
		}
		if err := os.MkdirAll(filepath.Join(radice, CartellaPartiDeposito), 0o755); err != nil {
			return nil, fmt.Errorf("deposito degli Altri file %s: %w", radice, err)
		}
		return &Deposito{Radice: radice, ID: id}, nil
	}
	n, err := q.ContaConservazioniVive(ctx)
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, fmt.Errorf("deposito degli Altri file %s: manca il segnaposto %s, ma ci sono %d copie di conservazione "+
			"registrate: le copie non sono in questa cartella (radice cambiata, cartella ricreata vuota o segnaposto tolto). "+
			"Rimettere la cartella giusta in [conservazione].radice: il server non parte", radice, NomeSegnaposto, n)
	}
	if err := os.MkdirAll(filepath.Join(radice, CartellaPartiDeposito), 0o755); err != nil {
		return nil, fmt.Errorf("deposito degli Altri file %s: %w", radice, err)
	}
	id = uuid.New()
	f, err := os.OpenFile(filepath.Join(radice, NomeSegnaposto), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("deposito degli Altri file %s: segnaposto: %w", radice, err)
	}
	_, err = fmt.Fprintf(f, "%s\n# Deposito degli «Altri file» del Cockpit: copie verificate dei file messi da parte nella Sorgente.\n"+
		"# Lo scrive solo il server. Non cancellare questo file: dice al server che le copie registrate sono qui.\n", id)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, fmt.Errorf("deposito degli Altri file %s: segnaposto: %w", radice, err)
	}
	return &Deposito{Radice: radice, ID: id}, nil
}

// leggiSegnaposto: l'UUID del segnaposto (la prima riga che non e' un commento); presente=false se il file non c'e'.
// Un segnaposto illeggibile ferma l'avvio: non si indovina.
func leggiSegnaposto(radice string) (uuid.UUID, bool, error) {
	f, err := os.Open(filepath.Join(radice, NomeSegnaposto))
	if errors.Is(err, os.ErrNotExist) {
		return uuid.UUID{}, false, nil
	}
	if err != nil {
		return uuid.UUID{}, false, fmt.Errorf("deposito degli Altri file %s: segnaposto: %w", radice, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		r := strings.TrimSpace(sc.Text())
		if r == "" || strings.HasPrefix(r, "#") {
			continue
		}
		id, err := uuid.Parse(r)
		if err != nil {
			return uuid.UUID{}, false, fmt.Errorf("deposito degli Altri file %s: il segnaposto %s non contiene un UUID valido",
				radice, NomeSegnaposto)
		}
		return id, true, nil
	}
	return uuid.UUID{}, false, fmt.Errorf("deposito degli Altri file %s: il segnaposto %s e' vuoto", radice, NomeSegnaposto)
}

// PercorsoCopia ricostruisce il file di una copia da radice e percorso_relativo, e controlla che stia sotto la radice
// (DentroLaRadice, come la lettura dal NAS). Un percorso che ne esce e' un difetto, mai un file da aprire.
func (d *Deposito) PercorsoCopia(percorsoRelativo string) (string, error) {
	rel := strings.ReplaceAll(strings.TrimSpace(percorsoRelativo), "/", string(filepath.Separator))
	rel = strings.ReplaceAll(rel, `\`, string(filepath.Separator))
	if rel == "" || filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" || strings.HasPrefix(rel, string(filepath.Separator)) {
		return "", fmt.Errorf("percorso di conservazione non valido: %q", percorsoRelativo)
	}
	base := filepath.Clean(d.Radice)
	p := filepath.Clean(filepath.Join(base, rel))
	if !documenti.DentroLaRadice(p, base) {
		return "", fmt.Errorf("percorso di conservazione fuori dal deposito: %q", percorsoRelativo)
	}
	return p, nil
}

// percorsoParte: un parziale nuovo di una copia, con un suffisso casuale (creazione esclusiva).
func (d *Deposito) percorsoParte(conservazioneID int64) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return filepath.Join(d.Radice, CartellaPartiDeposito, strconv.FormatInt(conservazioneID, 10)+"."+hex.EncodeToString(b[:]))
}

// partiDellaRiga: i parziali di una copia (qualunque suffisso), cioe' i file di <radice>\_parti il cui nome e'
// <conservazione_id>.<casuale>. Si leggono con os.ReadDir, non con filepath.Glob: la radice non entra in un pattern
// (una radice con «[» o «]» diventerebbe un pattern, e i parziali si cercherebbero, e si toglierebbero, in un'altra
// cartella: rilievo della revisione), e ogni percorso si controlla dentro la radice prima di restituirlo.
func (d *Deposito) partiDellaRiga(conservazioneID int64) []string {
	base := filepath.Clean(d.Radice)
	cartella := filepath.Join(base, CartellaPartiDeposito)
	voci, err := os.ReadDir(cartella)
	if err != nil {
		return nil
	}
	var out []string
	for _, v := range voci {
		if v.IsDir() {
			continue
		}
		if id, ok := idDelParziale(v.Name()); !ok || id != conservazioneID {
			continue
		}
		p := filepath.Join(cartella, v.Name())
		if !documenti.DentroLaRadice(p, base) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// idDelParziale: il conservazione_id nel nome di un parziale del deposito, <conservazione_id>.<casuale>.
func idDelParziale(nome string) (int64, bool) {
	punto := strings.IndexByte(nome, '.')
	if punto <= 0 {
		return 0, false
	}
	id, err := strconv.ParseInt(nome[:punto], 10, 64)
	return id, err == nil
}

// CopiaPresente dice se il file di una copia verificata c'e' nel deposito con la dimensione attesa: il custode della
// cache non conta il pin «proposta aperta» di un'occorrenza in Altri file solo dopo averlo visto (EM §10.5). Una copia
// di un altro deposito non c'e' per definizione.
func (d *Deposito) CopiaPresente(percorsoRelativo string, depositoID uuid.UUID, bytes int64) bool {
	if d == nil || depositoID != d.ID {
		return false
	}
	p, err := d.PercorsoCopia(percorsoRelativo)
	if err != nil {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && !st.IsDir() && st.Size() == bytes
}

// CopiaNelDeposito: la verifica che la cache usa per non contare il pin di un file in Altri file (staging.Cache); nil
// senza deposito (il pin resta).
func (s *Servizio) CopiaNelDeposito() func(string, uuid.UUID, int64) bool {
	if s == nil || s.Deposito == nil {
		return nil
	}
	return s.Deposito.CopiaPresente
}
