//go:build integrazione

// L4 — Calcola sulla fotografia letta dal database di prova (A1c-L4S-07, la parte di Calcola; A-C12; piano A, 6.7.3;
// contratto §7; IM.1): la scena ACME di B1 scritta con SQL (costruisciScenaDB), letta con caricatore.Carica in sola
// lettura; poi un'analisi nuova di uno STEP alla stessa terna, e una seconda fotografia. La fotografia conservata,
// valutata di nuovo, ridà l'esito di prima, byte per byte e con la stessa impronta; la seconda fotografia ha un'altra
// impronta, e l'esito pure. Calcola non parla con il database. La parte del DB (Digest e CalcolatoIl cambiano) è la prova
// del caricatore (TestDueFotografieDellaStessaScena).
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è
// pubblico.

package valutazione_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/fotorfq/caricatore"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
	"promatec/cockpit/internal/platform/testutil"
)

// insiemeDelDB: l'insieme delle regole con la grammatica ACME per il cliente della scena sul DB, il cui UUID nasce dal DB:
// costruito a mano, perché CompilaInsieme scarterebbe una grammatica che dice un altro cliente. L'impronta dell'indice è
// quella del suo canonico, come in CompilaInsieme.
func insiemeDelDB(t *testing.T, cliente uuid.UUID) *motorea.InsiemeRegole {
	t.Helper()
	ind := grammatica.IndiceRegole{VersioneIndice: grammatica.VersioneIndice, Limiti: limitiACME(),
		Grammatiche: []grammatica.VoceIndice{{ClienteID: cliente, File: "acme.v1.json", Sha256: sha("a")}}}
	h, err := jsoncanonico.ImprontaDi(ind)
	if err != nil {
		t.Fatal(err)
	}
	return &motorea.InsiemeRegole{Indice: ind, ImprontaIndice: h, Motori: map[uuid.UUID]*motorea.Motore{cliente: motoreACME(t)},
		Scartati: map[uuid.UUID][]evidenze.Diagnostica{}}
}

// TestL4CalcolaRidaLEsitoDellaFotografiaConservata (A1c-L4S-07, la parte di Calcola; A-C12): la fotografia conservata ridà
// l'esito di prima dopo un'analisi nuova alla stessa terna; la fotografia nuova dà un altro esito.
func TestL4CalcolaRidaLEsitoDellaFotografiaConservata(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := costruisciScenaDB(t, p)
	pr, reg := testutil.PoolConRegistro(t, testutil.DSN(t))
	ctx := context.Background()
	r := insiemeDelDB(t, s.cliente)
	carica := func(nome string) fotorfq.Fotografia {
		t.Helper()
		f, err := caricatore.Carica(ctx, pr, caricatore.Richiesta{Thread: []uuid.UUID{s.thread}})
		if err != nil {
			t.Fatalf("%s: Carica: %v", nome, err)
		}
		return f
	}
	calcolaSenzaDB := func(nome string, f fotorfq.Fotografia) (valutazione.Esito, string) {
		t.Helper()
		reg.Azzera()
		e := calcola(t, f, r, valutazione.Ingressi{})
		if len(reg.Voci()) != 0 {
			t.Errorf("%s: Calcola ha parlato con il database: %v", nome, reg.Voci())
		}
		return e, canonicoDi(t, e)
	}

	f1 := carica("prima fotografia")
	e1, b1 := calcolaSenzaDB("prima fotografia", f1)
	if len(e1.Thread) != 1 {
		t.Fatalf("thread nell'esito: %d", len(e1.Thread))
	}
	et := e1.Thread[0]
	if !et.Valutato || len(et.ProdottiValutati) != 2 || len(et.Confrontabili) != 2 || len(et.File) != 2 || et.HashSnapshot == "" {
		t.Fatalf("esito della scena: valutato %v, %d prodotti, %d record piatti, %d file, motivo %q, diagnostiche %v", et.Valutato,
			len(et.ProdottiValutati), len(et.Confrontabili), len(et.File), et.Motivo, codiciDi(et.Diagnostiche))
	}

	// Un'analisi nuova dello STEP dell'assieme, alla stessa terna (A-C12): i fatti cambiano.
	// Lo stesso testo di UpsertAnalisiFatti (queries/analisi.sql), scritto qui come le altre righe della scena.
	if _, err := p.Exec(ctx, `INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, $2, $3, $4)
		ON CONFLICT (sha256, versione_analizzatore, hash_configurazione) DO UPDATE SET fatti = EXCLUDED.fatti, calcolato_il = now()`,
		sha("2"), versioneAnalizzatore, hashConfigurazione, json.RawMessage(strutturaDB("7120100", "7120102"))); err != nil {
		t.Fatal(err)
	}
	f2 := carica("seconda fotografia")
	e2, _ := calcolaSenzaDB("seconda fotografia", f2)
	if e2.ImprontaFotografia == e1.ImprontaFotografia || e2.Impronta == e1.Impronta {
		t.Errorf("l'analisi nuova non cambia l'impronta della fotografia (%v) o dell'esito (%v)", e2.ImprontaFotografia == e1.ImprontaFotografia,
			e2.Impronta == e1.Impronta)
	}

	// La fotografia conservata è un valore in memoria: ridà l'esito di prima.
	e3, b3 := calcolaSenzaDB("fotografia conservata", f1)
	if e3.Impronta != e1.Impronta || b3 != b1 {
		t.Errorf("la fotografia conservata non ridà l'esito di prima: %s e %s", e1.Impronta, e3.Impronta)
	}
}
