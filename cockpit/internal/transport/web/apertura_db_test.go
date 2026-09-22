//go:build integrazione

// L4 — blocco 3 del 3R: l'aggiornamento automatico alla PRIMA apertura dell'Inbox.
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/transport/web/
//
// Sul banco reale `intervallo_sync_s = 0`: nessun sync periodico, ed è voluto. Ma allora chi apre il
// Cockpit la mattina vede la posta di ieri finché non preme «Aggiorna ora», e un'Inbox che mostra la
// posta di ieri è un'Inbox che nessuno guarderà più: si riapre Outlook, e da lì in poi il Cockpit è
// un doppione. La voce 2.16 esiste per questo, e questa ne è la conseguenza.
//
// La difficoltà sta tutta nel non esagerare. La chiave di idempotenza dei job non basta: impedisce
// due job pendenti insieme, non dieci job in un'ora a furia di F5, e non impedisce niente al poll
// HTMX che chiede lo stesso indirizzo ogni quindici secondi. Perciò il diritto è della SESSIONE, e
// si prende una volta sola.
package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"promatec/cockpit/internal/platform/db"
)

// syncAccodati sono i job di sincronizzazione presenti in coda, di qualunque stato.
func (b *bancoWeb) syncAccodati() []db.Job {
	b.t.Helper()
	return b.jobDiTipo(db.TipoJobSyncOutlook)
}

func (b *bancoWeb) chiudiTuttiISync() {
	b.t.Helper()
	if _, err := b.pool.Exec(b.ctx, "UPDATE job SET stato = 'fatto', chiuso_il = now() WHERE tipo = 'sync_outlook'"); err != nil {
		b.t.Fatal(err)
	}
}

// L — la prima apertura dell'Inbox accoda un aggiornamento per casella attiva.
func TestAperturaLPrimaAperturaDellInboxAccodaUnSync(t *testing.T) {
	b := preparaBancoWeb(t)
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)
	if n := len(b.syncAccodati()); n != 0 {
		t.Fatalf("%d sync in coda prima che qualcuno aprisse l'Inbox", n)
	}

	chi := b.browser("10.0.0.5:4000")
	chi.login("FP", "prova-fp")
	// il login da solo non basta: un admin puo' entrare soltanto per l'Anagrafica, e non deve
	// svegliare nessun worker
	if n := len(b.syncAccodati()); n != 0 {
		t.Fatalf("il login ha accodato %d sync senza che nessuno abbia aperto l'Inbox", n)
	}

	chi.fai(http.MethodGet, "/inbox", nil, false)
	caselle, err := b.q.ListCaselleAttive(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n, attesi := len(b.syncAccodati()), len(caselle); n != attesi {
		t.Fatalf("aperta l'Inbox: %d sync accodati, attesi %d (uno per casella attiva)", n, attesi)
	}
	// ed e' l'aggiornamento ordinario, non un job storico: muove la frontiera recente
	for _, j := range b.syncAccodati() {
		if j.Priorita == 9 {
			t.Errorf("job %d accodato con la priorita' dell'archivio: l'apertura chiede la posta nuova", j.JobID)
		}
	}
}

// M — il refresh del browser e il poll HTMX non ne accodano un secondo.
//
// Sono i due modi in cui questa comodita' diventerebbe un danno: la pagina ricaricata dieci volte in
// un'ora, e il poll che chiede lo stesso indirizzo ogni quindici secondi per sempre. Il secondo e'
// escluso dall'intestazione HX-Request; il primo no, e per quello serve il diritto della sessione.
func TestAperturaMIlRefreshEIlPollNonAccodanoAltro(t *testing.T) {
	b := preparaBancoWeb(t)
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)
	chi := b.browser("10.0.0.5:4000")
	chi.login("FP", "prova-fp")

	// Un poll HTMX prima di qualunque pagina vera non deve nemmeno CONSUMARE il diritto della
	// sessione. Sembra un caso di scuola e non lo e': una scheda ripresa dalla cronologia fa partire
	// il poll su un HTML che il browser aveva gia', senza passare da una GET completa. Se il diritto
	// se ne andasse li', l'apertura vera non accoderebbe piu' niente e nessuno saprebbe perche'.
	chi.fai(http.MethodGet, "/inbox?filtro=tutti", nil, true)
	if n := len(b.syncAccodati()); n != 0 {
		t.Fatalf("un poll HTMX ha accodato %d sync", n)
	}

	chi.fai(http.MethodGet, "/inbox", nil, false)
	primi := len(b.syncAccodati())
	if primi == 0 {
		t.Fatal("la prima apertura non ha accodato niente: il diritto se n'era andato con il poll che l'ha preceduta")
	}

	// il poll HTMX, che chiede lo stesso indirizzo
	for i := 0; i < 3; i++ {
		chi.fai(http.MethodGet, "/inbox?filtro=tutti", nil, true)
	}
	if n := len(b.syncAccodati()); n != primi {
		t.Errorf("il poll HTMX ha accodato %d sync in piu'", n-primi)
	}

	// il refresh, con i job precedenti gia' chiusi: e' il caso che la sola chiave di idempotenza
	// non fermerebbe, perche' la chiave si libera appena il job finisce
	b.chiudiTuttiISync()
	for i := 0; i < 3; i++ {
		chi.fai(http.MethodGet, "/inbox", nil, false)
	}
	if n := len(b.syncAccodati()); n != primi {
		t.Errorf("tre refresh hanno accodato %d sync in piu': il diritto e' della sessione e si prende una volta sola", n-primi)
	}

	// una sessione NUOVA lo ha di nuovo: e' un'apertura vera, non un refresh
	dopo := b.browser("10.0.0.5:4000")
	dopo.login("FP", "prova-fp")
	dopo.fai(http.MethodGet, "/inbox", nil, false)
	if n := len(b.syncAccodati()); n != 2*primi {
		t.Errorf("dopo un login nuovo ci sono %d sync, attesi %d: chi rientra la mattina dopo deve avere il suo aggiornamento", n, 2*primi)
	}
}

// N — «Aggiorna ora» premuto mentre l'apertura ne ha appena accodato uno non ne aggiunge un secondo,
// e lo dice invece di far finta di aver fatto qualcosa (SV1, dal lato dell'apertura automatica).
func TestAperturaNAggiornaOraDopoLAperturaNonDuplica(t *testing.T) {
	b := preparaBancoWeb(t)
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)
	chi := b.browser("10.0.0.5:4000")
	chi.login("FP", "prova-fp")
	chi.fai(http.MethodGet, "/inbox", nil, false)
	primi := len(b.syncAccodati())

	resp, corpo := chi.fai(http.MethodPost, "/inbox/aggiorna", url.Values{}, true)
	if resp.StatusCode != 200 {
		t.Fatalf("aggiorna ora: %d", resp.StatusCode)
	}
	if n := len(b.syncAccodati()); n != primi {
		t.Errorf("«Aggiorna ora» ha accodato %d sync in piu' su job gia' pendenti", n-primi)
	}
	if !strings.Contains(corpo, "già in corso") {
		t.Errorf("la risposta non dice che era gia' in corso: %s", estratto(corpo, "avviso"))
	}
}

// Senza un worker Outlook vivo non si accoda niente.
//
// Un job che nessuno prendera' non fa perdere posta — la finestra successiva riparte comunque dalla
// copertura — ma occupa la chiave di idempotenza con una finestra che invecchia: accodato alle 09:00
// e preso alle 14:00, avrebbe `al = 09:00`, e l'aggiornamento vero arriverebbe piu' tardi di quanto
// sarebbe bastato.
func TestAperturaSenzaWorkerVivoNonAccodaNiente(t *testing.T) {
	b := preparaBancoWeb(t)
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)
	// il worker si spegne: l'ultimo contatto scivola oltre la soglia della testata
	if _, err := b.pool.Exec(b.ctx, "UPDATE worker_presenza SET ultimo_contatto = now() - interval '5 minutes'"); err != nil {
		t.Fatal(err)
	}
	chi := b.browser("10.0.0.5:4000")
	chi.login("FP", "prova-fp")
	chi.fai(http.MethodGet, "/inbox", nil, false)
	if n := len(b.syncAccodati()); n != 0 {
		t.Fatalf("%d sync accodati con tutti i worker spenti", n)
	}

	// e quando il worker torna, la stessa sessione lo ottiene: il diritto non si consuma a vuoto
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)
	chi.fai(http.MethodGet, "/inbox", nil, false)
	if n := len(b.syncAccodati()); n == 0 {
		t.Error("il worker e' tornato e l'apertura non ha accodato niente: il diritto della sessione era stato consumato senza usarlo")
	}
}

// La voce si puo' spegnere, e spenta non accoda niente.
func TestAperturaSiPuoSpegnere(t *testing.T) {
	b := preparaBancoWeb(t)
	b.ws.SyncAperturaInbox = false
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)
	chi := b.browser("10.0.0.5:4000")
	chi.login("FP", "prova-fp")
	chi.fai(http.MethodGet, "/inbox", nil, false)
	if n := len(b.syncAccodati()); n != 0 {
		t.Fatalf("%d sync accodati con sync_apertura_inbox = false", n)
	}
	// e la schermata non promette una cosa che non fa
	_, pagina := chi.fai(http.MethodGet, "/inbox", nil, false)
	if strings.Contains(pagina, "parte da solo alla prima apertura") {
		t.Error("la schermata annuncia l'aggiornamento di apertura mentre e' spento")
	}
}
