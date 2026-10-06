//go:build integrazione && privato

// L4 — la copia intatta del dump (A1c-L4D-01; piano A, 6.4.7 e 6.7.4; R10, R44): PoolDump passa sulla copia del
// manifest; con una copia attesa alterata in memoria (una sentinella, lo schema, il ruolo) ControllaCopia dà
// l'errore che PoolDump trasforma in NON ESEGUITA; le sentinelle si ricontrollano in t.Cleanup. Senza
// COCKPIT_DATASET_A o COCKPIT_DUMP_DSN la prova è NON ESEGUITA, mai saltata.
//
// Nessun valore reale nel codice (E-18): nome della copia, ruolo, schema e sentinelle vengono dal manifest
// privato; i messaggi non li ripetono oltre a ciò che dicono gli errori, che finiscono solo nei log privati.

package testutil

import (
	"context"
	"sort"
	"strings"
	"testing"

	"promatec/cockpit/internal/platform/dataset"
)

// copiaDellaCopia: una copia indipendente di una CopiaAttesa, da alterare senza toccare il manifest.
func copiaDellaCopia(a dataset.CopiaAttesa) dataset.CopiaAttesa {
	b := a
	b.Escluse = append([]string(nil), a.Escluse...)
	b.Sentinelle = make(map[string]int64, len(a.Sentinelle))
	for k, v := range a.Sentinelle {
		b.Sentinelle[k] = v
	}
	return b
}

func TestL4DumpPoolDumpControllaLaCopia(t *testing.T) {
	m := DatasetA(t)
	p := PoolDump(t, m.Copia)
	ctx := context.Background()
	if err := ControllaCopia(ctx, leggiDalPool(p), m.Copia); err != nil {
		t.Fatalf("la copia del manifest, ricontrollata: %v", err)
	}

	sentinelle := make([]string, 0, len(m.Copia.Sentinelle))
	for tab := range m.Copia.Sentinelle {
		sentinelle = append(sentinelle, tab)
	}
	sort.Strings(sentinelle)
	alterazioni := []struct {
		nome   string
		altera func(*dataset.CopiaAttesa)
		frase  string
	}{
		{"una sentinella", func(a *dataset.CopiaAttesa) { a.Sentinelle[sentinelle[0]]++ }, "sentinelle"},
		{"lo schema", func(a *dataset.CopiaAttesa) { a.Schema-- }, "schema"},
		{"il ruolo", func(a *dataset.CopiaAttesa) { a.Ruolo += "_altro" }, "utente"},
	}
	for _, c := range alterazioni {
		t.Run(c.nome, func(t *testing.T) {
			a := copiaDellaCopia(m.Copia)
			c.altera(&a)
			if err := ControllaCopia(ctx, leggiDalPool(p), a); err == nil || !strings.Contains(err.Error(), c.frase) {
				t.Errorf("con %s alterato: %v, atteso un errore che nomini %q", c.nome, err, c.frase)
			}
			fatale, saltato := conTBFinto(t, func(tb testing.TB) { PoolDump(tb, a) })
			if !strings.HasPrefix(fatale, "NON ESEGUITA: ") || !strings.Contains(fatale, c.frase) || saltato != "" {
				t.Errorf("PoolDump con %s alterato: fatale %q, saltato %q; attesa una NON ESEGUITA", c.nome, fatale, saltato)
			}
		})
	}
}
