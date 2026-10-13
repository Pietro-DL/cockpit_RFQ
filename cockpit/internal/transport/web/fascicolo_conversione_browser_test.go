//go:build integrazione && browser

// L7 — A1d P4, Scenario G della direttiva 12 nel BROWSER VERO: la domanda contestuale del cambio di tipo nel Fascicolo
// legacy (D-K6-01 = A [U] con la precisazione, §6). La L4 TestScenarioGConfermaContestualeDalFascicolo legge l'HTML
// che torna; qui si vede che la domanda compare al posto dell'avviso, che «No» la chiude senza mandare niente, e che
// «Sì» rimanda il gesto con il bersaglio e la sostituzione ereditati da #fascicolo, senza ricaricare la pagina.
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags "integrazione browser" -count=1 -run TestL7LaDomanda ./internal/transport/web/
//
// Stato: eseguita e passata nell'integrazione della corsia D (13/10/2026, Edge).
package web

import (
	"testing"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/testutil"
)

// La scena L7 di sempre, piu' il particolare 77817300 (una radice della BOM) con il suo STEP autorizzato, che propone
// il figlio diretto 77817301. Per questa prova --stp e' quello STEP (lo script lo legge come lo STEP della prova K).
func TestL7LaDomandaDelCambioDiTipo(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaL7(t, "L7K", 0)
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	p := s.componenteTipo("77817300", "sciolto")
	stp := s.stepNellaRfq("77817300.stp", fattiDi("#1", []string{"#1=77817300", "#2=77817301"}, []string{"#1>#2*1"}), an)
	if _, err := fascicolo.RileggiStepDellaRfq(b.ctx, b.q, s.thread, an); err != nil {
		t.Fatal(err)
	}
	testutil.AutorizzaStep(t, b.pool, s.thread, p, stp, s.utente)
	if _, err := fascicolo.RileggiStepDellaRfq(b.ctx, b.q, s.thread, an); err != nil {
		t.Fatal(err)
	}
	s.stp = stp
	lanciaL7(t, s, "K")
	if got := s.valore(`SELECT tipo::text FROM componente WHERE componente_id = $1`, p); got != "sottoassieme" {
		t.Errorf("dopo «Sì» nel browser 77817300 e' un assieme: %s", got)
	}
	if got := s.valore(`SELECT count(*)::text FROM componente_relazione r JOIN componente f ON f.componente_id = r.figlio_id
		WHERE r.padre_id = $1 AND f.codice = '77817301'`, p); got != "1" {
		t.Errorf("l'arco 77817300 → 77817301 entrato con la conferma: %s", got)
	}
}
