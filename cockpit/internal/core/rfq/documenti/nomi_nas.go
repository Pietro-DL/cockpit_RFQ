package documenti

// I nomi dei file sul NAS dopo A4 (addendum A4.1, A4.3; decisioni D21, D22).
//
// Un CAD, un disegno o uno sviluppo si chiama come il pezzo e la sua revisione, non come l'allegato
// da cui e' arrivato: <CODICE>_REV_<REV>.<ext>, con `ND` quando la revisione non si sa (negli STEP
// veri e' il caso normale). Un secondo file diverso con lo stesso nome — il secondo foglio di un 2D,
// una revisione senza rev — riceve `_2`, `_3`. Gli altri tipi tengono il nome originale sanificato.
// Il nome originale non si perde: sta in documento.nome_file e nella provenienza.
//
// Chi sceglie un nome nuovo in una cartella lo sceglie dentro la sua transazione, dopo aver preso il
// lucchetto della cartella: due transazioni che scelgono nella stessa cartella si mettono in fila, e
// la seconda vede il nome che la prima ha preso. Un nome e' occupato se lo dichiara un documento, uno
// spostamento pendente (da o a) o un file rimasto sul NAS che nessuno dichiara (nas_orfano).

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// Tecnico dice se un tipo di documento descrive un pezzo: esiste solo con il codice, e sul NAS si
// chiama come il pezzo (D21).
func Tecnico(tipo db.TipoDocumento) bool {
	return tipo == db.TipoDocumentoCad3d || tipo == db.TipoDocumentoDisegno2d || tipo == db.TipoDocumentoSviluppoDxf
}

// NomeTecnico e' il nome sul NAS di un documento tecnico: <CODICE>_REV_<REV>.<ext> (D21), con ND per
// una revisione che non si sa (D22). Codice e revisione passano dalle stesse regole della cartella
// del codice, in maiuscolo; l'estensione e' quella del file, in minuscolo. Il risultato ripassato da
// NomeFileSicuro resta uguale.
func NomeTecnico(codice, rev, estensione string) string {
	r := strings.ToUpper(strings.TrimSpace(rev))
	if r == "" {
		r = "ND"
	}
	nome := NomeSicuro(strings.ToUpper(codice), 60) + "_REV_" + NomeSicuro(r, 10)
	if e := strings.Trim(strings.TrimSpace(estensione), "."); e != "" {
		nome += "." + strings.ToLower(e)
	}
	return NomeFileSicuro(nome)
}

// NomeSulNas e' il nome che un documento nuovo riceve sul NAS prima del progressivo: il nome tecnico
// per i tipi tecnici, il nome originale sanificato per gli altri.
func NomeSulNas(tipo db.TipoDocumento, codice, rev, estensione, originale string) string {
	if Tecnico(tipo) {
		return NomeTecnico(codice, rev, estensione)
	}
	return NomeFileSicuro(originale)
}

// ConProgressivo e' il nome con il progressivo n (n <= 1: il nome com'e'): «X_REV_B.pdf» → «X_REV_B_2.pdf».
// Il nome resta un nome che NomeFileSicuro non cambia, anche quando il progressivo lo allunga oltre
// il limite: si accorcia la base, non il progressivo.
func ConProgressivo(nome string, n int) string {
	if n <= 1 {
		return nome
	}
	ext := estensione(nome)
	suffisso := "_" + strconv.Itoa(n)
	base := senzaCoda(taglia(strings.TrimSuffix(nome, ext), maxNomeFile-utf8.RuneCountInString(suffisso)))
	return base + suffisso + ext
}

// maxProgressivo e' un limite di sicurezza: novecentonovantanove file con lo stesso nome nella stessa
// cartella non sono un fascicolo, sono un difetto da guardare.
const maxProgressivo = 999

// ScegliPercorso sceglie il percorso di un file nuovo nella cartella, nella transazione di q: prende il
// lucchetto della cartella, poi il primo nome libero fra nome, nome_2, nome_3... Il nome scelto e'
// riservato finche' la transazione non finisce, e dopo lo dichiara chi l'ha scritto (il documento o il
// job di spostamento).
func ScegliPercorso(ctx context.Context, q *db.Queries, thread uuid.UUID, cartella, nome string) (string, error) {
	if err := BloccaCartella(ctx, q, thread, cartella); err != nil {
		return "", err
	}
	return primoLibero(ctx, q, thread, cartella, nome)
}

// BloccaCartella prende il lucchetto di transazione della cartella (A4.3). Chi tocca piu' cartelle li
// prende in ordine di percorso.
func BloccaCartella(ctx context.Context, q *db.Queries, thread uuid.UUID, cartella string) error {
	return q.BloccaCartella(ctx, db.BloccaCartellaParams{ThreadID: thread, Cartella: cartella})
}

// RiscegliPercorso e' ScegliPercorso per un documento che ha gia' un percorso (attuale): quello conta
// come libero, perche' lo occupa lui. Una correzione che non cambia cartella ne' nome non gli da' un
// progressivo contro se stesso. Il lucchetto lo prende chi chiama, per tutte le cartelle che tocca e
// in ordine di percorso (BloccaCartella).
func RiscegliPercorso(ctx context.Context, q *db.Queries, thread uuid.UUID, cartella, nome, attuale string) (string, error) {
	return primoLiberoPer(ctx, q, thread, cartella, nome, attuale)
}

// primoLibero e' la scelta del nome senza il lucchetto: da sola non basta, perche' due transazioni
// possono vedere libero lo stesso nome. Le prove la usano per mostrarlo.
func primoLibero(ctx context.Context, q *db.Queries, thread uuid.UUID, cartella, nome string) (string, error) {
	return primoLiberoPer(ctx, q, thread, cartella, nome, "")
}

func primoLiberoPer(ctx context.Context, q *db.Queries, thread uuid.UUID, cartella, nome, attuale string) (string, error) {
	for n := 1; n <= maxProgressivo; n++ {
		p := NellaCartella(cartella, ConProgressivo(nome, n))
		if attuale != "" && strings.EqualFold(p, attuale) {
			return attuale, nil
		}
		occupato, err := q.PercorsoOccupato(ctx, db.PercorsoOccupatoParams{ThreadID: thread, Percorso: p})
		if err != nil {
			return "", err
		}
		if !occupato.Bool {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s: nessun nome libero fino a %d nella cartella %q", nome, maxProgressivo, cartella)
}

// ------------------------------------------------------------------ lo spostamento, passo 0 (A4.3)

// PayloadSpostamento e' il lavoro di un sposta_nas: dal passo 0 al passo 6 il job nomina `da` e `a`,
// e PercorsoOccupato li tiene riservati tutti e due.
type PayloadSpostamento struct {
	ThreadID    uuid.UUID `json:"thread_id"`
	DocumentoID uuid.UUID `json:"documento_id"`
	Da          string    `json:"da"`
	A           string    `json:"a"`
	Sha256      string    `json:"sha256"`
}

// ErrSpostamentoInCorso: il documento ha gia' uno spostamento da finire. Un documento non si sposta
// mentre si sta gia' spostando.
var ErrSpostamentoInCorso = errors.New("spostamento in corso, riprova fra poco")

// AccodaSpostamento e' il passo 0 di sposta_nas, nella transazione di chi decide lo spostamento (la
// correzione di un codice, una sostituzione): blocca gli spostamenti pendenti del documento, prende
// il lucchetto della cartella di destinazione, sceglie il nome e accoda il job. path_relativo resta
// `da`, e cambia solo al passo 4, in un'altra transazione. Restituisce `a`.
//
// L'esecuzione del job (passi 1–6) arriva con B8.8: fino ad allora il job resta in coda.
func AccodaSpostamento(ctx context.Context, q *db.Queries, d db.Documento, cartella, nome string) (string, error) {
	chiave := coda.ChiaveSpostamento(d.DocumentoID)
	if _, err := q.BloccaSpostamentoPendente(ctx, pgtype.Text{String: chiave, Valid: true}); err == nil {
		return "", ErrSpostamentoInCorso
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	a, err := ScegliPercorso(ctx, q, d.ThreadID, cartella, nome)
	if err != nil {
		return "", err
	}
	j, err := coda.Accoda(ctx, q, db.TipoJobSpostaNas,
		PayloadSpostamento{ThreadID: d.ThreadID, DocumentoID: d.DocumentoID, Da: d.PathRelativo, A: a, Sha256: d.Sha256}, chiave, 1)
	if err != nil {
		return "", err
	}
	if j == nil {
		// fra il blocco e l'inserimento un altro l'ha accodato: l'indice dei pendenti lo ha rifiutato
		return "", ErrSpostamentoInCorso
	}
	return a, nil
}
