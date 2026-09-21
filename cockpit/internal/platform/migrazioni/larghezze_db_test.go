//go:build integrazione

package migrazioni_test

import (
	"context"
	"testing"

	"promatec/cockpit/internal/core/domain"
	"promatec/cockpit/internal/platform/testutil"
)

// Le colonne `codice` e `rev` devono essere almeno larghe quanto il dominio dichiara (7C.1, P0).
//
// Il dominio dice che un codice sta in MaxCodice caratteri e una revisione in MaxRev; il server
// scarta prima di scrivere cio' che non ci sta. La promessa «cio' che passa il dominio non rompe
// una INSERT» vale pero' solo se nessuna colonna e' piu' stretta del dominio: questa prova lo
// verifica sullo schema applicato, cosi' una migrazione futura che restringesse una colonna, o una
// costante alzata senza allargare le colonne, si vede qui e non in un 422 sul banco.
//
// Le tabelle sono quelle in cui un codice PRODOTTO arriva da un nome di file, da un testo o da un
// worker. `lavorazione.codice` (varchar(30)) resta fuori apposta: e' la chiave di catalogo di una
// lavorazione («cataforesi»), scritta dal seme, non un codice prodotto letto da fuori.
func TestLeColonneCodiceERevSonoLargheQuantoIlDominio(t *testing.T) {
	pool := testutil.Pool(t)
	testutil.SchemaPulito(t, pool)
	rows, err := pool.Query(context.Background(), `
		SELECT table_name, column_name, character_maximum_length
		FROM information_schema.columns
		WHERE table_schema = 'public' AND data_type = 'character varying'
		  AND column_name IN ('codice', 'rev')
		  AND table_name IN ('documento_proposta', 'documento', 'componente', 'candidato_codice',
		                     'riferimento_portale', 'identificativo_thread')
		ORDER BY table_name, column_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	viste := 0
	for rows.Next() {
		var tabella, colonna string
		var larghezza *int32
		if err := rows.Scan(&tabella, &colonna, &larghezza); err != nil {
			t.Fatal(err)
		}
		viste++
		if larghezza == nil {
			continue // varchar senza limite: va bene
		}
		minimo := int32(domain.MaxCodice)
		if colonna == "rev" {
			minimo = int32(domain.MaxRev)
		}
		if *larghezza < minimo {
			t.Errorf("%s.%s e' varchar(%d): piu' stretta del dominio (%d)", tabella, colonna, *larghezza, minimo)
		}
	}
	if viste == 0 {
		t.Fatal("nessuna colonna codice/rev trovata: la query non guarda lo schema giusto")
	}
}
