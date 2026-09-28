package testutil

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AutorizzaStep e' l'aiuto comune delle prove dello Smistamento (F5, addendum A5.8): il file dell'allegato e'
// autorizzato a proporre i figli diretti del componente. Scrive direttamente quello che scrivera' il gesto
// di F5b (DichiaraStrutturale), senza le sue guardie:
//   - il documento del file per il componente, se nella RFQ non c'e' ancora (tipo cad_3d, il codice del
//     componente, lo sha dell'allegato);
//   - la radice del file (la riga del portatore senza un arco entrante nel file, criterio K1) decisa da
//     utente come il componente, con la marcatura evidenza.strutturale (ruolo radice);
//   - per un finito, anche step_strutturale_id sul documento (I10).
//
// Le righe del file devono esserci gia' (ApplicaStruttura prima): senza, non c'e' un nodo da marcare, e la
// prova si ferma. Dopo, chi prova rilegge il file, come fara' il gesto. Restituisce il documento.
func AutorizzaStep(t testing.TB, p *pgxpool.Pool, thread, componente, allegato, utente uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var sha, nome, codice, tipo string
	if err := p.QueryRow(ctx, `SELECT a.sha256, a.nome_file, c.codice, c.tipo::text FROM allegato a, componente c
		WHERE a.allegato_id = $1 AND c.componente_id = $2`, allegato, componente).Scan(&sha, &nome, &codice, &tipo); err != nil {
		t.Fatalf("autorizza: allegato o componente: %v", err)
	}
	var doc uuid.UUID
	var docComp uuid.NullUUID
	err := p.QueryRow(ctx, `SELECT documento_id, componente_id FROM documento WHERE thread_id = $1 AND sha256 = $2`, thread, sha).Scan(&doc, &docComp)
	switch {
	case err == nil && docComp.Valid && docComp.UUID != componente:
		t.Fatalf("autorizza: il documento di %s e' di un altro componente", nome)
	case err == nil && !docComp.Valid:
		if _, err := p.Exec(ctx, `UPDATE documento SET componente_id = $2, codice = $3 WHERE documento_id = $1`, doc, componente, codice); err != nil {
			t.Fatalf("autorizza: documento: %v", err)
		}
	case err != nil:
		if err := p.QueryRow(ctx, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
			VALUES ($1, $2, 'cad_3d', $3, $4, 'stp', $5, $6, $7) RETURNING documento_id`,
			thread, componente, codice, nome, sha, `AUTORIZZATI\`+sha[:16]+`.stp`, utente).Scan(&doc); err != nil {
			t.Fatalf("autorizza: documento: %v", err)
		}
	}
	righe, err := p.Query(ctx, `SELECT cp.proposta_id FROM componente_proposta cp
		WHERE cp.thread_id = $1 AND cp.sha256 = $2
		  AND cp.allegato_id = (SELECT allegato_id FROM componente_proposta WHERE thread_id = $1 AND sha256 = $2 ORDER BY creato_il, allegato_id LIMIT 1)
		  AND NOT EXISTS (SELECT 1 FROM relazione_proposta r WHERE r.thread_id = cp.thread_id AND r.allegato_id = cp.allegato_id AND r.figlio_chiave = cp.chiave)`,
		thread, sha)
	if err != nil {
		t.Fatalf("autorizza: radice: %v", err)
	}
	var radici []uuid.UUID
	for righe.Next() {
		var id uuid.UUID
		if err := righe.Scan(&id); err != nil {
			t.Fatal(err)
		}
		radici = append(radici, id)
	}
	righe.Close()
	if len(radici) != 1 {
		t.Fatalf("autorizza: %s ha %d radici fra le proposte (le righe del file ci sono?)", nome, len(radici))
	}
	marcatura, _ := json.Marshal(map[string]any{"v": 1, "ruolo": "radice", "componente_id": componente, "documento_id": doc,
		"dichiarato_da": utente, "dichiarato_il": time.Now().UTC().Format(time.RFC3339)})
	tag, err := p.Exec(ctx, `UPDATE componente_proposta SET stato = CASE WHEN stato = 'aperta' THEN 'duplicato'::stato_proposta ELSE stato END,
		componente_id = $2, deciso_da = $3, deciso_il = now(), evidenza = evidenza || jsonb_build_object('strutturale', $4::jsonb)
		WHERE proposta_id = $1 AND (stato = 'aperta' OR componente_id = $2)`, radici[0], componente, utente, marcatura)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("autorizza: marcatura della radice di %s: %v (righe %d)", nome, err, tag.RowsAffected())
	}
	if tipo == "finito" {
		if _, err := p.Exec(ctx, `UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, componente, doc); err != nil {
			t.Fatalf("autorizza: step_strutturale_id: %v", err)
		}
	}
	return doc
}
