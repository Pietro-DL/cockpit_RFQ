package testutil

// analisi.go: l'unico punto, nelle prove, che scrive la tabella dei suggerimenti dell'agente (piano A, A1c,
// 6.4.7, 3.3.9 e 3.5, MOTORE-SENZA-LLM). Le prove del motore A la riempiono di suggerimenti discordanti per
// mostrare che il motore non li legge: lo fanno solo qui, e solo su un database che lo ammette.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// marcatoreCopiaUsaEGetta è l'inizio del commento (COMMENT ON DATABASE) che lo script del database di prova
// scrive su una copia TEMPLATE del dump fatta per essere scritta. Il resto del commento (la copia da cui
// viene, la data) lo scrive lo script e non sta nel codice.
const marcatoreCopiaUsaEGetta = "cockpit giro5: copia usa e getta di "

// scritturaDiProvaAmmessa dice se un database può ricevere righe scritte apposta dalle prove: il database di
// prova (il nome contiene «test») oppure una copia usa e getta (il marcatore nel commento). La copia intatta
// del dump non ha né l'uno né l'altro, e non si tocca mai (R10). Pura: le prove L1 la chiamano con i valori.
func scritturaDiProvaAmmessa(database, commento string) error {
	if strings.Contains(strings.ToLower(database), "test") {
		return nil
	}
	if strings.HasPrefix(commento, marcatoreCopiaUsaEGetta) {
		return nil
	}
	return fmt.Errorf("il database %q non è quello di prova (nessun «test» nel nome) e non è una copia usa e getta "+
		"(nessun marcatore nel commento): qui le prove non scrivono (R10)", database)
}

// nomeECommentoDelDatabase: current_database() e il suo commento, "" se non ce l'ha.
func nomeECommentoDelDatabase(ctx context.Context, p *pgxpool.Pool) (string, string, error) {
	var nome, commento string
	err := p.QueryRow(ctx, `SELECT current_database()::text,
		coalesce(shobj_description(d.oid, 'pg_database'), '')::text
		  FROM pg_database d WHERE d.datname = current_database()`).Scan(&nome, &commento)
	return nome, commento, err
}

// AnalisiMessaggioDiProva scrive una riga dei suggerimenti dell'agente (0008:133-150) per il messaggio, con il
// risultato dato e lo stato «completata», e ne restituisce l'ID. Rifiuta, prima di scrivere, un database il cui
// nome non contiene «test» e che non ha il marcatore delle copie usa e getta nel commento: la copia intatta non
// si tocca mai (R10). L'impronta dell'ingresso è lo sha256 del messaggio e del risultato, così due risultati
// diversi per lo stesso messaggio sono due righe.
func AnalisiMessaggioDiProva(t testing.TB, p *pgxpool.Pool, messaggio uuid.UUID, risultato json.RawMessage) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	nome, commento, err := nomeECommentoDelDatabase(ctx, p)
	if err != nil {
		t.Fatalf("suggerimento di prova: nome del database: %v", err)
	}
	if err := scritturaDiProvaAmmessa(nome, commento); err != nil {
		t.Fatalf("suggerimento di prova: %v", err)
	}
	h := sha256.Sum256(append(append([]byte(messaggio.String()), 0), risultato...))
	var id uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO analisi_messaggio (messaggio_id, input_hash, versione, prompt, modello, stato, risultato)
		VALUES ($1, $2, 'prova', 'prova', 'prova', 'completata', $3) RETURNING analisi_id`,
		messaggio, hex.EncodeToString(h[:]), risultato).Scan(&id); err != nil {
		t.Fatalf("suggerimento di prova: %v", err)
	}
	return id
}
