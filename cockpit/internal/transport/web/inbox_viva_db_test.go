//go:build integrazione

// L4 — voce 2.16 «Inbox viva» contro PostgreSQL vero: SV1, SV2, SV3.
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/web/
//
// Che cosa NON provano: che il browser mostri davvero la chip e il pallino, e che un operatore se ne
// accorga. Quello è L7 e sta in esiti_reali.md. Qui si prova ciò che il server risponde.
package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// SV1 — «Aggiorna ora» premuto dieci volte accoda UN sync per casella, non dieci.
//
// È la prima cosa che farebbe un operatore davanti a un'Inbox che non si aggiorna: premere. Se ogni
// clic accodasse un job, il worker passerebbe la mezz'ora successiva a rileggere la stessa finestra
// dieci volte, cioè a non aggiornare l'Inbox — l'esatto contrario di ciò che gli è stato chiesto.
func TestSV1AggiornaOraNonAccodaDieciVolte(t *testing.T) {
	b := preparaBancoWeb(t)
	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")

	for i := 0; i < 10; i++ {
		resp, corpo := fp.fai(http.MethodPost, "/inbox/aggiorna", nil, true)
		if resp.StatusCode != 200 {
			t.Fatalf("clic %d: stato %d", i+1, resp.StatusCode)
		}
		if i == 0 && !strings.Contains(corpo, "Sincronizzazione richiesta per 3 caselle") {
			t.Errorf("il primo clic non dice che cosa ha fatto: %s", estratto(corpo, "avviso-testata"))
		}
		if i == 9 && !strings.Contains(corpo, "già in corso") {
			t.Errorf("il decimo clic non dice che non c'era niente da accodare: %s", estratto(corpo, "avviso-testata"))
		}
	}

	pendenti := map[uuid.UUID]int{}
	for _, j := range b.jobInterattivi() {
		if j.Tipo == db.TipoJobSyncOutlook && j.CasellaID.Valid {
			pendenti[j.CasellaID.UUID]++
		}
	}
	if len(pendenti) != 3 {
		t.Fatalf("sync accodati per %d caselle, attese 3 (una per casella attiva)", len(pendenti))
	}
	for casella, n := range pendenti {
		if n != 1 {
			t.Errorf("casella %s: %d job di sync in coda, atteso 1", casella, n)
		}
	}
}

// SV2 — la testata dice «in corso» mentre il sync gira, e torna ad «attiva» quando ha finito; e si
// ricarica più spesso finché c'è lavoro.
func TestSV2LaTestataDiceQuandoIlSyncStaGirando(t *testing.T) {
	b := preparaBancoWeb(t)
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)
	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")

	_, testata := fp.fai(http.MethodGet, "/stato/worker", nil, true)
	for _, atteso := range []string{">Francesco attiva<", ">Commerciale attiva<", ">Luigi OFFLINE<"} {
		if !strings.Contains(testata, atteso) {
			t.Fatalf("la testata non dice %q: %s", atteso, testata)
		}
	}
	if strings.Contains(testata, "in corso") {
		t.Errorf("nessun sync accodato, ma la testata dice «in corso»: %s", testata)
	}
	if !strings.Contains(testata, "every 15s") {
		t.Errorf("senza lavoro la testata deve ricaricarsi al passo lento: %s", estratto(testata, "hx-trigger"))
	}

	// accodato un sync, la casella passa a «in corso» e il poll si stringe
	fp.fai(http.MethodPost, "/inbox/aggiorna", nil, true)
	_, testata = fp.fai(http.MethodGet, "/stato/worker", nil, true)
	if !strings.Contains(testata, "in corso") {
		t.Errorf("sync in coda, ma la testata non lo dice: %s", testata)
	}
	if !strings.Contains(testata, "every 3s") {
		t.Errorf("con un sync in corso la testata deve ricaricarsi più spesso: %s", estratto(testata, "hx-trigger"))
	}

	// chiuso il sync, la chip torna attiva e porta l'ora dell'ultimo giro
	if _, err := b.pool.Exec(b.ctx, `UPDATE job SET stato='fatto', chiuso_il=now(), lease_token=NULL, lease_fino_a=NULL
		WHERE tipo='sync_outlook'`); err != nil {
		t.Fatal(err)
	}
	_, testata = fp.fai(http.MethodGet, "/stato/worker", nil, true)
	if strings.Contains(testata, "in corso") {
		t.Errorf("sync finito, ma la testata lo dà ancora in corso: %s", testata)
	}
	if !strings.Contains(testata, "ultimo sync") {
		t.Errorf("la testata non dice quando è finito l'ultimo sync: %s", testata)
	}
}

// SV3 — «nuove dall'ultima visita»: conteggio e pallino contro `registrato_il`, e l'orologio della
// visita che si sposta SOLO quando la pagina viene aperta davvero.
func TestSV3NuoveDallUltimaVisita(t *testing.T) {
	b := preparaBancoWeb(t)
	b.messaggioIn("<vecchia@acme.example>", b.francesco)
	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")

	// prima apertura in assoluto: `ultima_vista_inbox` è NULL, quindi tutto è nuovo
	_, pagina := fp.fai(http.MethodGet, "/inbox?filtro=tutti", nil, false)
	if !strings.Contains(pagina, "1 nuove") {
		t.Errorf("alla prima apertura il messaggio esistente doveva risultare nuovo: %s", estratto(pagina, "nuove"))
	}

	// seconda apertura senza niente di nuovo: il contatore è sparito
	_, pagina = fp.fai(http.MethodGet, "/inbox?filtro=tutti", nil, false)
	if strings.Contains(pagina, "nuove") {
		t.Errorf("niente è arrivato, ma l'Inbox segnala novità: %s", estratto(pagina, "nuove"))
	}

	// arrivano due messaggi
	nuovo := b.messaggioIn("<nuova1@acme.example>", b.francesco, b.commerciale)
	b.messaggioIn("<nuova2@acme.example>", b.commerciale)

	// il POLL HTMX non deve azzerare il contatore: se lo facesse, il numero sarebbe sempre zero.
	// Dal blocco 7 la lista e' a quadranti e il predefinito e' Buyer: questi messaggi vengono da un
	// dominio non censito, quindi stanno in «Da validare», e la prova chiede «tutti».
	_, lista := fp.fai(http.MethodGet, "/inbox?q=tutti&filtro=tutti", nil, true)
	if !strings.Contains(lista, "pallino") {
		t.Errorf("le righe nuove non sono segnate: %s", estratto(lista, "riga"))
	}
	_, testata := fp.fai(http.MethodGet, "/stato/worker", nil, true)
	if !strings.Contains(testata, "2 nuove") {
		t.Errorf("la testata non conta le due mail nuove: %s", testata)
	}
	// e il conteggio per casella: la prima è arrivata a due caselle, la seconda a una sola
	if !strings.Contains(testata, "Commerciale") || !strings.Contains(testata, "1 nuove") {
		t.Errorf("il conteggio per casella non compare: %s", testata)
	}

	// l'apertura VERA della pagina sposta l'orologio: da lì in poi nessuna è più nuova
	fp.fai(http.MethodGet, "/inbox?filtro=tutti", nil, false)
	_, testata = fp.fai(http.MethodGet, "/stato/worker", nil, true)
	if strings.Contains(testata, "nuove") {
		t.Errorf("dopo l'apertura il contatore doveva azzerarsi: %s", testata)
	}
	var quando any
	if err := b.pool.QueryRow(b.ctx, `SELECT ultima_vista_inbox FROM utente WHERE sigla = 'FP'`).Scan(&quando); err != nil {
		t.Fatal(err)
	}
	if quando == nil {
		t.Fatal("ultima_vista_inbox non è stata aggiornata dall'apertura della pagina")
	}
	// l'altro utente non è stato in Inbox: per lui sono ancora tutte nuove
	lu := b.browser("10.0.0.9:51000")
	lu.login("LU", "prova-lu")
	_, pagina = lu.fai(http.MethodGet, "/inbox?filtro=tutti", nil, false)
	if !strings.Contains(pagina, "3 nuove") {
		t.Errorf("la visita di Francesco ha azzerato il contatore di Luigi: %s", estratto(pagina, "nuove"))
	}
	_ = nuovo
}
