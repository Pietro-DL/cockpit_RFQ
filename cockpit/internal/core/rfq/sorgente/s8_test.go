package sorgente

// L1 — la forma del corpo di S8 e della revoca, le chiavi dei passi, l'esito parziale (direttiva 39 §2, §3).

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"promatec/cockpit/internal/core/rfq/distintabozza"
)

func corpoL1(t *testing.T, campi map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(campi)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestV1S8LaFormaDelCorpo(t *testing.T) {
	k, prod := uuid.New(), uuid.New()
	imp := strings.Repeat("a", 64)
	base := func() map[string]any {
		return map[string]any{"chiave_idempotenza": k, "prodotto_id": prod, "gruppo": "bom", "impronta_contenuto": imp}
	}
	r, i1, err := LeggiRichiestaS8(corpoL1(t, base()))
	if err != nil || r.Partenza != PartenzaStep || len(i1) != 64 {
		t.Fatalf("step: %+v %v", r, err)
	}
	// l'impronta e' della forma canonica: lo stesso corpo con le chiavi in un altro ordine e' lo stesso atto
	_, i2, _ := LeggiRichiestaS8([]byte(`{"impronta_contenuto":"` + imp + `","gruppo":"bom","prodotto_id":"` + prod.String() + `","chiave_idempotenza":"` + k.String() + `"}`))
	if i1 != i2 {
		t.Fatal("l'impronta dipende dall'ordine dei campi")
	}
	casi := []struct {
		nome   string
		cambia func(m map[string]any)
		campo  string
		motivo string
	}{
		{"chiave v1", func(m map[string]any) { m["chiave_idempotenza"] = "6ba7b810-9dad-11d1-80b4-00c04fd430c8" }, "chiave_idempotenza", ""},
		{"senza prodotto", func(m map[string]any) { delete(m, "prodotto_id") }, "prodotto_id", ""},
		{"sostituisci falso", func(m map[string]any) { m["sostituisci"] = map[string]any{"conferma": false} }, "sostituisci", ""},
		{"campo sconosciuto", func(m map[string]any) { m["sostituisci_documento"] = true }, "richiesta", ""},
		{"autorizza", func(m map[string]any) { m["autorizza"] = true }, "richiesta", ""},
		{"senza gruppo", func(m map[string]any) { delete(m, "gruppo") }, "gruppo", ""},
		{"gruppo senza", func(m map[string]any) { m["gruppo"] = "senza" }, "gruppo", distintabozza.MotivoGruppoNonImportabile},
		{"gruppo riservato", func(m map[string]any) { m["gruppo"] = "solo_prodotto" }, "gruppo", distintabozza.MotivoGruppoNonImportabile},
		{"impronta corta", func(m map[string]any) { m["impronta_contenuto"] = "abc" }, "impronta_contenuto", ""},
		{"stato dell'analisi con step", func(m map[string]any) { m["stato_analisi"] = "in_analisi" }, "stato_analisi", ""},
		{"partenza ignota", func(m map[string]any) { m["partenza"] = "vuota" }, "partenza", ""},
		{"scelte senza bozza", func(m map[string]any) {
			m["scelte"] = []any{map[string]any{"chiave_bozza": "k", "scelta": "recepisci"}}
		}, "scelte", ""},
		{"scelta togli", func(m map[string]any) {
			m["bozza"] = map[string]any{"bozza_id": uuid.New(), "versione_attesa": 2}
			m["scelte"] = []any{map[string]any{"chiave_bozza": "k", "scelta": "togli"}}
		}, "scelte[0].scelta", ""},
		{"bozza senza versione", func(m map[string]any) { m["bozza"] = map[string]any{"bozza_id": uuid.New()} }, "bozza", ""},
		{"sha non valido", func(m map[string]any) { m["sha256"] = "XYZ" }, "sha256", ""},
		{"motivo lungo", func(m map[string]any) { m["motivo"] = strings.Repeat("x", 501) }, "motivo", ""},
	}
	for _, c := range casi {
		m := base()
		c.cambia(m)
		_, _, err := LeggiRichiestaS8(corpoL1(t, m))
		var e *distintabozza.Errore
		if !errors.As(err, &e) || e.Codice != distintabozza.CodiceCorpoNonValido || e.Campo != c.campo || (c.motivo != "" && e.Motivo != c.motivo) {
			t.Errorf("%s: %v (%+v)", c.nome, err, e)
		}
	}
	// solo_prodotto: stato dell'analisi e presa d'atto, niente gruppo
	solo := map[string]any{"chiave_idempotenza": k, "prodotto_id": prod, "partenza": "solo_prodotto", "stato_analisi": "errore", "presa_d_atto": true, "motivo": "m"}
	if r, _, err := LeggiRichiestaS8(corpoL1(t, solo)); err != nil || r.gruppoFonte() != distintabozza.GruppoSoloProdotto {
		t.Fatalf("solo_prodotto: %+v %v", r, err)
	}
	solo["presa_d_atto"] = false
	if _, _, err := LeggiRichiestaS8(corpoL1(t, solo)); err == nil || !strings.Contains(err.Error(), "presa d'atto") {
		t.Fatalf("senza presa d'atto: %v", err)
	}
	solo["presa_d_atto"], solo["gruppo"] = true, "bom"
	if _, _, err := LeggiRichiestaS8(corpoL1(t, solo)); err == nil {
		t.Fatal("solo_prodotto con un gruppo passa")
	}
	delete(solo, "gruppo")
	solo["stato_analisi"] = "boh"
	if _, _, err := LeggiRichiestaS8(corpoL1(t, solo)); err == nil {
		t.Fatal("uno stato dell'analisi fuori elenco passa")
	}
}

func TestV1S8LeChiaviDeiPassi(t *testing.T) {
	k := uuid.New()
	a, b := ChiavePasso(k, "2"), ChiavePasso(k, "2")
	if a != b || a.Version() != 5 || a == ChiavePasso(k, "3") || a == ChiavePasso(uuid.New(), "2") {
		t.Fatalf("chiavi dei passi: %s %s", a, b)
	}
	if improntaPasso("x", "3") == improntaPasso("x", "3a") || len(improntaPasso("x", "3")) != 64 {
		t.Fatal("impronte dei passi")
	}
}

func TestV1S8LErroreTemporaneoEIlRifiuto(t *testing.T) {
	if !transitorio(&pgconn.PgError{Code: "40P01"}) || !transitorio(&pgconn.PgError{Code: "55P03"}) || transitorio(errors.New("x")) {
		t.Fatal("transitorio")
	}
	if _, ok := definitivo(&distintabozza.Errore{Codice: "X"}); !ok {
		t.Fatal("definitivo")
	}
	out := EsitoS8{Passi: []PassoS8{{Passo: PassoConfermaDocumento, Esito: EsitoFatto}, {Passo: PassoBozza, Esito: EsitoRifiutato}}}
	if tx := testoParziale(out); !strings.Contains(tx, "Riuscito: conferma_documento") || !strings.Contains(tx, "Non riuscito: bozza") ||
		!strings.Contains(tx, "Manca: niente") {
		t.Fatalf("testo del parziale: %s", tx)
	}
}

func TestV1S8LaFormaDellaRevoca(t *testing.T) {
	k, prod := uuid.New(), uuid.New()
	if _, _, err := LeggiRichiestaRevoca(corpoL1(t, map[string]any{"chiave_idempotenza": k, "prodotto_id": prod, "motivo": "errore"})); err != nil {
		t.Fatal(err)
	}
	// messaggio 46 [U]: la revoca senza motivo passa (il motivo della riga lo scrive il server, TracciaRevoca)
	r, _, err := LeggiRichiestaRevoca(corpoL1(t, map[string]any{"chiave_idempotenza": k, "prodotto_id": prod, "sorgente_errata": true}))
	if err != nil || r.Motivo != "" {
		t.Fatalf("revoca senza motivo: %v %+v", err, r)
	}
	r.traccia = TracciaRevoca("PS", "ACM-1000.stp", "ACM-1000")
	if m := r.motivoRegistrato(); m != "sorgente_errata: gesto di PS da «Revoca come sorgente»: lo STEP «ACM-1000.stp» non è più la sorgente confermata di ACM-1000" {
		t.Fatalf("il motivo della revoca senza il motivo dell'utente: %q", m)
	}
	var e *distintabozza.Errore
	if _, _, err := LeggiRichiestaRevoca(corpoL1(t, map[string]any{"chiave_idempotenza": k, "prodotto_id": prod, "motivo": strings.Repeat("x", 1001)})); !errors.As(err, &e) || e.Campo != "motivo" {
		t.Fatalf("il motivo di un client di prima oltre 1000 caratteri passa: %v", err)
	}
	if _, _, err := LeggiRichiestaRevoca(corpoL1(t, map[string]any{"chiave_idempotenza": k, "prodotto_id": prod, "motivo": "m", "documento_id": uuid.New()})); err == nil {
		t.Fatal("un campo in piu' passa")
	}
}
