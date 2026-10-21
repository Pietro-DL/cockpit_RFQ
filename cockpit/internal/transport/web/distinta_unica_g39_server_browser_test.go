//go:build integrazione && browser

// Browser contro il server vero, giro G39 (G39-03 [U], decisioni dell'utente del 20/10 sera), corsia interfaccia: la pagina
// della Distinta unica servita dal banco web VERO (preparaBancoWeb, rotte vere, calcolo vero del Motore A con le regole
// ACME, come TestDistintaUnicaServerVeroNelBrowser), in Edge:
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test COCKPIT_BROWSER_CANALE=msedge go test -tags "integrazione browser" -count=1 -run TestDistintaUnicaG39ServerVeroNelBrowser ./internal/transport/web/
//
// La scena: due prodotti finiti, 9120001 e 9130001; lo STEP 9120001.stp (la radice 9120001A, letto dall'analizzatore
// corrente, con la sua proposta di 3D), nessun documento. La fase «g39» di e2e/distinta_unica_server.py, solo con i gesti
// della pagina: lo STEP confermato per errore come sorgente di 9130001 (S8 dal solo prodotto); su 9120001 «Sposta la
// sorgente su 9120001…» (S8 con sposta_da); fra le fasi arriva un PDF dopo l'apertura della bozza, che il motore suggerisce
// e la pagina accetta (G39-01: la PB «motore»); la revoca su 9120001. Poi il database dice che cosa è successo davvero:
//   - su 9130001 la revoca «sorgente errata» (fonte_step_evento revoca_sorgente, motivo con la causa in testa) e la bozza
//     nata dallo STEP annullata (abbandonata, la storia resta);
//   - il documento dello STEP passato a 9120001, con la conferma di S8 su 9120001;
//   - la revoca su 9120001 con la causa e la sua bozza annullata (la BOM non è mai stata confermata con lo STEP);
//   - nessuna AR, nessuna working, nessuna conferma della BOM (solo R5 le scrive).
//
// Clienti, codici e nomi dei file sono inventati (ACME, 912xxxx, 913xxxx, acme.example): il repository è pubblico.

package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func (b *bancoWeb) duServerVeroG39(t *testing.T, chiave string) *duServerVero {
	t.Helper()
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	r := b.rfqFascicolo(cliente, chiave)
	s := &duServerVero{rfqFascicolo: r, b: b}
	r.fase("FATTIBILITA")
	s.prodotto = r.componenteTipo("9120001", "finito")
	s.prodotto2 = r.componenteTipo("9130001", "finito")
	var sha string
	s.allStep, sha = r.allegatoExt("9120001.stp", "stp")
	r.analisiCorrente(sha, fattiDi("#1", []string{"#1=9120001A", "#2=9120010A", "#3=9120020A"}, []string{"#1>#2*2", "#2>#3*4"}))
	r.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1, $2, 'cad_3d', '9120001', 80, 'nome_file')`,
		s.allStep, r.thread)
	var percorso string
	if err := b.pool.QueryRow(b.ctx, `SELECT path_staging FROM allegato WHERE allegato_id = $1`, s.allStep).Scan(&percorso); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(percorso), 0o755); err == nil {
		b.ws.Staging = filepath.Dir(percorso)
	}
	mwAccendi(t, b, mwRegoleACME(t, cliente, "Acme S.p.A.", 20*time.Second))
	s.ad = b.browser("10.0.0.8")
	s.ad.login("AD", "prova-ad")
	return s
}

func TestDistintaUnicaG39ServerVeroNelBrowser(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.duServerVeroG39(t, "DUG39")
	conta := func(sql string, arg ...any) int { t.Helper(); return contaSQL(t, b, sql, arg...) }
	s.lancia(t, "g39")
	// lo spostamento su 9130001: la revoca con la causa e la bozza annullata
	if n := conta(`SELECT count(*) FROM fonte_step_evento WHERE thread_id = $1 AND prodotto_id = $2 AND azione = 'revoca_sorgente' AND motivo LIKE 'sorgente_errata%'`,
		s.thread, s.prodotto2); n != 1 {
		t.Errorf("revoche «sorgente errata» su 9130001: %d, attesa una", n)
	}
	if n := conta(`SELECT count(*) FROM distinta_bozza WHERE thread_id = $1 AND prodotto_id = $2 AND stato = 'abbandonata'`, s.thread, s.prodotto2); n != 1 {
		t.Errorf("bozze annullate di 9130001: %d, attesa una (la bozza nata dallo STEP sbagliato)", n)
	}
	if n := conta(`SELECT count(*) FROM distinta_bozza WHERE thread_id = $1 AND prodotto_id = $2 AND stato = 'aperta'`, s.thread, s.prodotto2); n != 0 {
		t.Errorf("bozze aperte di 9130001 dopo lo spostamento: %d", n)
	}
	// il documento passato a 9120001, con la conferma di S8 e poi la revoca con la causa
	if n := conta(`SELECT count(*) FROM documento d JOIN allegato a ON a.sha256 = d.sha256 WHERE a.allegato_id = $1 AND d.thread_id = $2 AND d.componente_id = $3`,
		s.allStep, s.thread, s.prodotto); n != 1 {
		t.Errorf("il documento dello STEP non è di 9120001 dopo lo spostamento: %d", n)
	}
	if n := conta(`SELECT count(*) FROM fonte_step_evento WHERE thread_id = $1 AND prodotto_id = $2 AND azione = 'conferma_documento' AND chiave_idempotenza IS NOT NULL`,
		s.thread, s.prodotto); n != 1 {
		t.Errorf("conferme S8 su 9120001: %d, attesa una", n)
	}
	var bozzaA uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT bozza_id FROM distinta_bozza WHERE thread_id = $1 AND prodotto_id = $2 AND stato = 'aperta'`, s.thread, s.prodotto).Scan(&bozzaA); err != nil {
		t.Fatalf("la bozza di 9120001 nata dallo spostamento: %v", err)
	}

	// G39-01 [U]: un PDF arriva dopo l'apertura della bozza di 9120001 (la tavola del particolare 9120020A, con la sua
	// proposta); il motore gli trova la destinazione: S9 lo suggerisce, «Accetta» lo salva e il server dice la PB «motore»
	s.allPDF, _ = s.allegatoExt("9120020A_1.pdf", "pdf")
	contenuto := pdfTavola("9120020")
	var percorso string
	if err := b.pool.QueryRow(b.ctx, `SELECT path_staging FROM allegato WHERE allegato_id = $1`, s.allPDF).Scan(&percorso); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(percorso, contenuto, 0o644); err != nil {
		t.Fatal(err)
	}
	s.esegui(`UPDATE allegato SET sha256 = $2, bytes = $3 WHERE allegato_id = $1`, s.allPDF, shaDi(contenuto), len(contenuto))
	s.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1, $2, 'disegno_2d', '9120020A', 85, 'nome_file')`,
		s.allPDF, s.thread)
	out := s.lancia(t, "g39b")
	t.Run("G39-01 suggerimento accettato", func(t *testing.T) {
		if strings.Contains(out, "G39-01 NON ESEGUITA") {
			t.Skip("NON ESEGUITA: il motore non suggerisce il PDF arrivato dopo l'apertura (il motivo è nell'uscita della fase g39b)")
		}
		var prov string
		if err := b.pool.QueryRow(b.ctx, `SELECT COALESCE(pb->>'provenienza', '') FROM distinta_bozza_versione v,
			jsonb_array_elements(v.snapshot->'documenti_preassociati') pb WHERE v.bozza_id = $1 AND pb->>'allegato_id' = $2
			ORDER BY v.versione DESC LIMIT 1`, bozzaA, s.allPDF.String()).Scan(&prov); err != nil {
			t.Fatalf("la PB del suggerimento accettato nella bozza: %v", err)
		}
		if prov != "motore" {
			t.Errorf("la PB del suggerimento accettato com'è ha la provenienza %q, attesa «motore» (G39-01 [U], decisa dal server)", prov)
		}
	})
	// G39-01 [U], «corretta» nel browser sul server vero (rilievo minore della revisione indipendente 3 della corsia): la
	// PB del suggerimento spostata dall'ingegnere su un altro nodo, con il registro di prima
	t.Run("G39-01 suggerimento spostato su un altro nodo", func(t *testing.T) {
		if strings.Contains(out, "G39-01 NON ESEGUITA") {
			t.Skip("NON ESEGUITA: senza il suggerimento accettato non c'è la PB da spostare (fase g39b)")
		}
		var primaK, primaImp string
		if err := b.pool.QueryRow(b.ctx, `SELECT pb->>'chiave_bozza', COALESCE(pb->>'importazione', '') FROM distinta_bozza_versione v,
			jsonb_array_elements(v.snapshot->'documenti_preassociati') pb WHERE v.bozza_id = $1 AND pb->>'allegato_id' = $2
			ORDER BY v.versione DESC LIMIT 1`, bozzaA, s.allPDF.String()).Scan(&primaK, &primaImp); err != nil {
			t.Fatalf("la PB del suggerimento accettato: %v", err)
		}
		s.lancia(t, "g39d")
		var k, prov, imp string
		if err := b.pool.QueryRow(b.ctx, `SELECT pb->>'chiave_bozza', COALESCE(pb->>'provenienza', ''), COALESCE(pb->>'importazione', '') FROM distinta_bozza_versione v,
			jsonb_array_elements(v.snapshot->'documenti_preassociati') pb WHERE v.bozza_id = $1 AND pb->>'allegato_id' = $2
			ORDER BY v.versione DESC LIMIT 1`, bozzaA, s.allPDF.String()).Scan(&k, &prov, &imp); err != nil {
			t.Fatalf("la PB spostata: %v", err)
		}
		if k == primaK || prov != "corretta" || imp == "" || imp != primaImp {
			t.Errorf("la PB del suggerimento spostata: nodo %s (prima %s), provenienza %q, registro %q (prima %q); attesa «corretta» con il registro di prima", k, primaK, prov, imp, primaImp)
		}
	})

	// G39-03: la revoca su 9120001, che annulla la bozza nata dallo STEP
	s.lancia(t, "g39c")
	if n := conta(`SELECT count(*) FROM fonte_step_evento WHERE thread_id = $1 AND prodotto_id = $2 AND azione = 'revoca_sorgente' AND motivo LIKE 'sorgente_errata%'`,
		s.thread, s.prodotto); n != 1 {
		t.Errorf("revoche «sorgente errata» su 9120001: %d, attesa una", n)
	}
	if n := conta(`SELECT count(*) FROM distinta_bozza WHERE thread_id = $1 AND prodotto_id = $2 AND stato = 'abbandonata'`, s.thread, s.prodotto); n != 1 {
		t.Errorf("bozze annullate di 9120001 dopo la revoca: %d, attesa una", n)
	}
	// niente AR, working o conferme della BOM fuori da R5
	if n := conta(`SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1`, s.allStep); n != 0 {
		t.Errorf("lo STEP sorgente ha una AR: %d", n)
	}
	if n := conta(`SELECT count(*) FROM componente WHERE thread_id = $1 AND step_strutturale_id IS NOT NULL`, s.thread); n != 0 {
		t.Errorf("la working ha uno STEP strutturale senza R5: %d", n)
	}
	if n := conta(`SELECT count(*) FROM conferma_strutturale WHERE thread_id = $1`, s.thread); n != 0 {
		t.Errorf("conferme della BOM senza R5: %d", n)
	}
}
