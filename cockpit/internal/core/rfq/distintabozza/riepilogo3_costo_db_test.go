//go:build integrazione

// L4 — la misura del costo di R4 formato 3 con 5000 nodi sul database di prova (EM §9.4: il costo si misura con 5000 nodi;
// direttiva 39, blocco «R4 completo» della corsia persistenza, punto 8): una working di 5000 componenti (il prodotto, 50
// sottoassiemi, 4949 particolari) e la bozza che li porta tutti come registrati; R4 intero (lettura del corpo, letture nella
// transazione REPEATABLE READ READ ONLY, calcolo, firma), tre volte. Le durate vanno nel log (go test -v); la prova fallisce
// solo oltre un tetto largo (60 s). Dati sintetici.

package distintabozza

import (
	"crypto/md5"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

// uuidDaTesto: md5(testo)::uuid di Postgres.
func uuidDaTesto(s string) uuid.UUID { return uuid.UUID(md5.Sum([]byte(s))) }

func TestV1D39R4Costo5000NodiDB(t *testing.T) {
	p, s := poolPulito(t)
	esegui3(t, p, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da)
		SELECT md5('costo-s' || i)::uuid, $1, 'ACM-S' || lpad(i::text, 4, '0'), 'sottoassieme', 'manuale', $2 FROM generate_series(0, 49) i`, s.thread, s.utente)
	esegui3(t, p, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da)
		SELECT md5('costo-p' || i || '-' || j)::uuid, $1, 'ACM-P' || lpad(i::text, 2, '0') || lpad(j::text, 2, '0'), 'sciolto', 'manuale', $2
		  FROM generate_series(0, 49) i, generate_series(0, 98) j WHERE NOT (i = 49 AND j = 98)`, s.thread, s.utente)
	esegui3(t, p, `INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da)
		SELECT $1, $2, md5('costo-s' || i)::uuid, 1, 'manuale', $3 FROM generate_series(0, 49) i`, s.thread, s.altroFinito, s.utente)
	esegui3(t, p, `INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da)
		SELECT $1, md5('costo-s' || i)::uuid, md5('costo-p' || i || '-' || j)::uuid, j % 5 + 1, 'manuale', $2
		  FROM generate_series(0, 49) i, generate_series(0, 98) j WHERE NOT (i = 49 AND j = 98)`, s.thread, s.utente)
	em := EmittenteCliente(idDi("cliente"))
	reg := func(chiave string, id uuid.UUID, codice, tipo string) map[string]any {
		return map[string]any{"chiave_bozza": chiave, "componente_id": id, "op": "riusa", "tipo": tipo, "origine": "registrato",
			"identita": map[string]any{"emittente": em, "emittente_presunto": true, "codice": codice, "modo": "registrato"}}
	}
	radice := ChiaveDelComponente(s.altroFinito)
	nodi := []any{reg(radice, s.altroFinito, "ACM-6000", "finito")}
	archi := []any{}
	for i := 0; i < 50; i++ {
		ks := fmt.Sprintf("k-s%02d", i)
		nodi = append(nodi, reg(ks, uuidDaTesto(fmt.Sprintf("costo-s%d", i)), fmt.Sprintf("ACM-S%04d", i), "sottoassieme"))
		archi = append(archi, map[string]any{"padre": radice, "figlio": ks, "qta": 1, "origine": "utente"})
		for j := 0; j < 99 && !(i == 49 && j == 98); j++ {
			kp := fmt.Sprintf("k-p%02d-%02d", i, j)
			nodi = append(nodi, reg(kp, uuidDaTesto(fmt.Sprintf("costo-p%d-%d", i, j)), fmt.Sprintf("ACM-P%02d%02d", i, j), "sciolto"))
			archi = append(archi, map[string]any{"padre": ks, "figlio": kp, "qta": j%5 + 1, "origine": "utente"})
		}
	}
	corpo := map[string]any{"formato": 3, "prodotto_id": s.altroFinito,
		"versione_bom_base": map[string]any{"conferma_id": nil, "numero": 0, "contatore": conta3(t, p, `SELECT COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = $1), 0)`, s.thread)},
		"firma_base":        firmaDiDB(t, p, s.thread, s.altroFinito), "nodi": nodi, "archi": archi, "decisioni": []any{}, "documenti_preassociati": []any{}}
	var durate []time.Duration
	for giro := 0; giro < 3; giro++ {
		t0 := time.Now()
		r := mustRiepilogoDB(t, p, s.thread, corpo, s.utente)
		durate = append(durate, time.Since(t0))
		if len(r.Identita) != 5000 || len(r.Archi) != 4999 || len(r.Blocchi) != 1 || r.Blocchi[0].Codice != BloccoStepSorgenteNonConfermata {
			t.Fatalf("R4 con 5000 nodi: identita %d archi %d blocchi %v", len(r.Identita), len(r.Archi), codici(r.Blocchi))
		}
	}
	t.Logf("R4 con 5000 nodi registrati sul database (corpo, letture, calcolo, firma): %v, %v, %v", durate[0], durate[1], durate[2])
	for _, d := range durate {
		if d > 60*time.Second {
			t.Fatalf("R4 con 5000 nodi dura %v (tetto 60 s)", d)
		}
	}
}
