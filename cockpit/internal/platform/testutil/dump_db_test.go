//go:build integrazione && privato

// L4 — la copia intatta del dump (A1c-L4D-01; piano A, 6.4.7 e 6.7.4; R10, R44; R117 b): PoolDump passa sulla copia
// del manifest; con una copia attesa alterata in memoria (una sentinella, un'impronta, lo schema, il ruolo)
// ControllaCopia dà l'errore che PoolDump trasforma in NON ESEGUITA; sentinelle e impronte si ricontrollano in
// t.Cleanup. Un manifest senza le impronte della copia: PoolDump segna la loro parte NON ESEGUITA, con il motivo, e
// la prova non è mai verde. Senza COCKPIT_DATASET_A o COCKPIT_DUMP_DSN la prova è NON ESEGUITA, mai saltata.
//
// Nessun valore reale nel codice (E-18): nome della copia, ruolo, schema, sentinelle e impronte vengono dal manifest
// privato; i messaggi non li ripetono oltre a ciò che dicono gli errori, che finiscono solo nei log privati.

package testutil

import (
	"context"
	"errors"
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
	if a.Impronte != nil {
		imp := dataset.ImpronteCopia{Versione: a.Impronte.Versione, Tabelle: make(map[string]dataset.ImprontaTabella, len(a.Impronte.Tabelle))}
		for k, v := range a.Impronte.Tabelle {
			imp.Tabelle[k] = v
		}
		b.Impronte = &imp
	}
	return b
}

// altraImpronta: un'impronta diversa da quella data, della stessa forma (la prima cifra cambiata).
func altraImpronta(s string) string {
	if strings.HasPrefix(s, "0") {
		return "1" + s[1:]
	}
	return "0" + s[1:]
}

func TestL4DumpPoolDumpControllaLaCopia(t *testing.T) {
	m := DatasetA(t)
	p := PoolDump(t, m.Copia)
	ctx := context.Background()
	// senza le impronte dichiarate PoolDump ha già segnato la loro parte NON ESEGUITA, con il motivo: qui non si ripete
	if err := ControllaCopia(ctx, leggiDalPool(p), m.Copia); err != nil && !errors.Is(err, ErrImpronteNonDichiarate) {
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
	if m.Copia.Impronte != nil {
		// a righe uguali, un'impronta diversa: «valori cambiati» (R117 b)
		tabella := tabelleInOrdine(m.Copia.Impronte.Tabelle)[0]
		alterazioni = append(alterazioni, struct {
			nome   string
			altera func(*dataset.CopiaAttesa)
			frase  string
		}{"un'impronta", func(a *dataset.CopiaAttesa) {
			d := a.Impronte.Tabelle[tabella]
			d.Sha256 = altraImpronta(strings.ToLower(d.Sha256))
			a.Impronte.Tabelle[tabella] = d
		}, "valori cambiati"})
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
