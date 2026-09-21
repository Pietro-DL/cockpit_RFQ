//go:build integrazione

// L4 — blocco 2 del checkpoint 3R: «il worker è vivo» e «il worker ha appena concluso un claim» sono
// due fatti diversi, e la testata deve leggere il primo.
//
// Il difetto, visto sul sistema vero: `worker_presenza.ultimo_claim` la scriveva solo l'handler del
// claim, DOPO `jobs.Claim(...)`, che è un long-poll da venti secondi. Quindi un worker dentro un sync
// di tre minuti non toccava quella riga per tre minuti e la testata — soglia sessanta secondi — lo
// dava per spento proprio mentre lavorava; e chi apriva il browser nei primi venti secondi dopo
// l'accensione leggeva «non è mai stato avviato».
//
// Questi test girano contro PostgreSQL, il server web vero e l'API dei worker vera, sullo stesso mux
// di produzione: il claim è un vero long-poll HTTP, il battito è una vera POST. Niente qui finge il
// tempo se non spostando all'indietro una riga già scritta, che è il solo modo di provare una soglia
// di quaranta secondi senza aspettarne quaranta.
package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/contratti/api"
	"promatec/cockpit/internal/platform/db"
)

// claimInCorso avvia un claim vero su un altro goroutine e restituisce il canale che si chiude quando
// la risposta arriva. Serve a guardare il sistema MENTRE il worker sta aspettando: è la finestra in
// cui prima non esisteva nessuna presenza.
func (b *bancoWeb) claimInCorso(tipo, nome, ip string, attesaS int, caselle ...uuid.UUID) <-chan struct{} {
	b.t.Helper()
	aperte := make([]api.CasellaAperta, 0, len(caselle))
	for _, c := range caselle {
		aperte = append(aperte, api.CasellaAperta{CasellaID: c, StoreID: "STORE-" + c.String()[:8]})
	}
	corpo, _ := json.Marshal(api.ClaimRichiesta{Worker: tipo, WorkerID: nome, AttesaS: attesaS, OutlookOk: true, CaselleAperte: aperte})
	fatto := make(chan struct{})
	go func() {
		defer close(fatto)
		r, _ := http.NewRequest(http.MethodPost, b.srv.URL+"/api/v1/jobs/claim", strings.NewReader(string(corpo)))
		r.Header.Set("X-Cockpit-Token", tokenDelWorker(nome))
		r.Header.Set("X-Prova-IP", ip)
		resp, err := http.DefaultClient.Do(r)
		if err == nil {
			resp.Body.Close()
		}
	}()
	return fatto
}

// presenzaEntro aspetta che la riga di presenza di un worker compaia. Non è una tolleranza sul
// comportamento: è il tempo che ci mette una richiesta HTTP ad arrivare fino all'INSERT. Se la riga
// comparisse solo alla fine del long-poll, questa attesa scadrebbe — ed è esattamente il difetto.
func (b *bancoWeb) presenzaEntro(nome string, entro time.Duration) db.WorkerPresenza {
	b.t.Helper()
	scade := time.Now().Add(entro)
	for {
		p, err := b.q.GetWorkerPresenza(b.ctx, nome)
		if err == nil {
			return p
		}
		if time.Now().After(scade) {
			b.t.Fatalf("dopo %v il worker %s non ha ancora una presenza: si è fatto vivo e il server non l'ha registrato", entro, nome)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// indietro sposta all'indietro le due colonne di tempo della presenza, per provare le soglie senza
// aspettarle davvero. `claim` nullo o negativo lascia ultimo_claim com'è.
func (b *bancoWeb) indietro(nome string, contatto, claim time.Duration) {
	b.t.Helper()
	if _, err := b.pool.Exec(b.ctx, `UPDATE worker_presenza
		SET ultimo_contatto = now() - make_interval(secs => $2::float),
		    ultimo_claim = CASE WHEN $3::float > 0 THEN now() - make_interval(secs => $3::float) ELSE ultimo_claim END
		WHERE worker_nome = $1`, nome, contatto.Seconds(), claim.Seconds()); err != nil {
		b.t.Fatal(err)
	}
}

// operatoreSuPcFrancesco: FP entra dall'IP del worker di PC-FRANCESCO, quindi la sessione prende
// quella postazione (voce 2.7) e la testata è quella che vedrebbe lui.
func (b *bancoWeb) operatoreSuPcFrancesco(ip string) *browser {
	b.t.Helper()
	w := b.browser(ip)
	w.login("FP", "prova-fp")
	return w
}

func (w *browser) testata() string {
	w.b.t.Helper()
	_, testo := w.fai(http.MethodGet, "/stato/worker", nil, true)
	return testo
}

// (1) IL CASO CHE HA FATTO NASCERE IL BLOCCO. Il worker si accende, entra in claim e resta appeso in
// long-poll perché non c'è lavoro. L'operatore apre il browser e fa login PROPRIO IN QUEL MOMENTO:
// deve vedere le sue caselle attive, non «mai avviato» e non OFFLINE. Nessun F5, nessuna attesa.
//
// E il controllo che dice perché: mentre il claim è appeso, ultimo_claim è ancora NULL. Nessun claim
// si è concluso — è vero, ed è scritto così invece che con un now() di comodo. La presenza esiste lo
// stesso, perché la scrive l'autenticazione all'ingresso.
func TestPresenzaOnlineDuranteIlLongPollDelClaim(t *testing.T) {
	b := preparaBancoWeb(t)
	const ip = "10.0.0.11"
	// nessun job in coda: il claim non ha niente da assegnare e resta appeso per tutta l'attesa
	fine := b.claimInCorso("outlook", "outlook@PC-FRANCESCO", ip, 3, b.francesco, b.commerciale)

	p := b.presenzaEntro("outlook@PC-FRANCESCO", 2*time.Second)
	if p.UltimoClaim != nil {
		t.Errorf("ultimo_claim = %v mentre il claim è ancora appeso: quella colonna deve dire «claim concluso», e non se ne è concluso nessuno", p.UltimoClaim)
	}
	if d := time.Since(p.UltimoContatto); d > 2*time.Second {
		t.Errorf("ultimo_contatto vecchio di %v: doveva essere l'ingresso del claim, appena avvenuto", d)
	}

	fp := b.operatoreSuPcFrancesco(ip)
	testata := fp.testata()
	if !strings.Contains(testata, "attiva su PC-FRANCESCO") {
		t.Errorf("la testata non dice «attiva su PC-FRANCESCO» mentre il worker è vivo e in attesa:\n%s", testata)
	}
	for _, vietato := range []string{"il worker outlook@PC-FRANCESCO non", "worker su PC-FRANCESCO OFFLINE"} {
		if strings.Contains(testata, vietato) {
			t.Errorf("la testata dice %q di un worker che sta aspettando in long-poll:\n%s", vietato, testata)
		}
	}

	<-fine
	p = b.presenzaEntro("outlook@PC-FRANCESCO", time.Second)
	if p.UltimoClaim == nil {
		t.Error("claim concluso e ultimo_claim ancora NULL: la colonna ha smesso di dire la sua cosa")
	}
}

// (2) IL BATTITO DI UN JOB LUNGO BASTA. Durante un job il worker non entra più in claim: se la
// liveness dipendesse dal claim, ogni job più lungo della soglia farebbe sparire il worker dalla
// testata. Qui il job c'è, il claim è concluso da cinque minuti, e l'unica cosa che arriva è il
// battito: deve bastare, e non deve far finta che un claim si sia concluso.
func TestPresenzaIlBattitoDiUnJobLungoTieneOnline(t *testing.T) {
	b := preparaBancoWeb(t)
	const ip = "10.0.0.11"
	if _, err := jobs.Accoda(b.ctx, b.q, db.TipoJobSyncOutlook, map[string]any{"casella_id": b.francesco}, "", 5); err != nil {
		t.Fatal(err)
	}
	job := b.claimConJob("outlook@PC-FRANCESCO", ip, b.francesco)
	fp := b.operatoreSuPcFrancesco(ip)

	// il job va avanti da cinque minuti: nessun claim si conclude da cinque minuti. È lo stato in cui
	// la testata diceva OFFLINE.
	b.indietro("outlook@PC-FRANCESCO", 5*time.Minute, 5*time.Minute)
	if testata := fp.testata(); !strings.Contains(testata, "worker su PC-FRANCESCO OFFLINE") {
		t.Fatalf("premessa del test: cinque minuti di silenzio vero devono dare OFFLINE, altrimenti non si sta provando niente:\n%s", testata)
	}

	// arriva un battito, come da un worker dentro un job lungo
	b.battito(job, ip)

	p := b.presenzaEntro("outlook@PC-FRANCESCO", time.Second)
	if d := time.Since(p.UltimoContatto); d > 2*time.Second {
		t.Errorf("dopo il battito ultimo_contatto è vecchio di %v: il battito non conta come prova di vita", d)
	}
	if p.UltimoClaim == nil || time.Since(*p.UltimoClaim) < 4*time.Minute {
		t.Errorf("ultimo_claim = %v: un battito non è un claim concluso e non deve spostarlo", p.UltimoClaim)
	}
	if testata := fp.testata(); !strings.Contains(testata, "attiva su PC-FRANCESCO") {
		t.Errorf("il battito non ha riportato il worker in testata:\n%s", testata)
	}
}

// claimConJob fa un claim che DEVE tornare con un job: senza, il test che lo chiama non prova niente.
func (b *bancoWeb) claimConJob(nome, ip string, caselle ...uuid.UUID) api.Job {
	b.t.Helper()
	aperte := make([]api.CasellaAperta, 0, len(caselle))
	for _, c := range caselle {
		aperte = append(aperte, api.CasellaAperta{CasellaID: c, StoreID: "STORE-" + c.String()[:8]})
	}
	corpo, _ := json.Marshal(api.ClaimRichiesta{Worker: "outlook", WorkerID: nome, AttesaS: 1, OutlookOk: true, CaselleAperte: aperte})
	r, _ := http.NewRequest(http.MethodPost, b.srv.URL+"/api/v1/jobs/claim", strings.NewReader(string(corpo)))
	r.Header.Set("X-Cockpit-Token", tokenDelWorker(nome))
	r.Header.Set("X-Prova-IP", ip)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b.t.Fatalf("nessun job assegnato a %s (%d): questo test ha bisogno di un job in corso", nome, resp.StatusCode)
	}
	var j api.Job
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		b.t.Fatal(err)
	}
	return j
}

func (b *bancoWeb) battito(j api.Job, ip string) {
	b.t.Helper()
	corpo, _ := json.Marshal(api.HeartbeatRichiesta{WorkerID: "outlook@PC-FRANCESCO", LeaseToken: j.LeaseToken})
	r, _ := http.NewRequest(http.MethodPost, b.srv.URL+"/api/v1/jobs/"+strconv.FormatInt(j.JobID, 10)+"/heartbeat", strings.NewReader(string(corpo)))
	r.Header.Set("X-Cockpit-Token", tokenDelWorker("outlook@PC-FRANCESCO"))
	r.Header.Set("X-Prova-IP", ip)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		b.t.Fatalf("battito rifiutato: %d", resp.StatusCode)
	}
}

// (3) UN WORKER DAVVERO FERMO DEVE DIVENTARE OFFLINE, e abbastanza presto da servire a qualcosa.
//
// È il test che impedisce la scorciatoia. Far sparire il falso OFFLINE allargando la soglia a due
// minuti avrebbe fatto passare (1) e (2) — quelli parlano di worker vivi — e deve rompere questo.
// Per riuscirci una parte delle attese è in secondi VERI e non in multipli di api.PresenzaOnlineEntro:
// un test scritto tutto in funzione della soglia si sposta insieme a lei e resta verde qualunque cosa
// le si faccia, cioè non prova niente. Il numero assoluto è la promessa all'operatore: se il worker
// muore, entro un minuto la testata lo dice. Cambiare quel minuto è una decisione da prendere qui,
// non un effetto collaterale di un ritocco altrove.
func TestPresenzaWorkerFermoDiventaOffline(t *testing.T) {
	b := preparaBancoWeb(t)
	const ip = "10.0.0.11"
	b.workerClaim("outlook@PC-FRANCESCO", ip, b.francesco, b.commerciale)
	fp := b.operatoreSuPcFrancesco(ip)

	// la promessa: un minuto di silenzio e chi guarda lo sa
	b.indietro("outlook@PC-FRANCESCO", 61*time.Second, 0)
	if testata := fp.testata(); !strings.Contains(testata, "worker su PC-FRANCESCO OFFLINE") {
		t.Errorf("worker zitto da 61 secondi e la testata lo dà ancora per vivo (soglia %v): un guasto che si scopre dopo minuti è un guasto che si scopre dal risultato, non dalla testata:\n%s",
			api.PresenzaOnlineEntro, testata)
	}

	// un secondo prima della soglia: ancora vivo. Un worker in attesa si fa vivo ogni AttesaClaim,
	// quindi qui dentro ci sta un giro perso intero senza allarmare nessuno.
	b.indietro("outlook@PC-FRANCESCO", api.PresenzaOnlineEntro-time.Second, 0)
	if testata := fp.testata(); !strings.Contains(testata, "attiva su PC-FRANCESCO") {
		t.Errorf("a %v dall'ultimo contatto (soglia %v) il worker risulta spento:\n%s",
			api.PresenzaOnlineEntro-time.Second, api.PresenzaOnlineEntro, testata)
	}
	// un secondo dopo: due giri persi, è un guasto e si vede
	b.indietro("outlook@PC-FRANCESCO", api.PresenzaOnlineEntro+time.Second, 0)
	if testata := fp.testata(); !strings.Contains(testata, "worker su PC-FRANCESCO OFFLINE") {
		t.Errorf("a %v dall'ultimo contatto (soglia %v) il worker risulta ancora vivo:\n%s",
			api.PresenzaOnlineEntro+time.Second, api.PresenzaOnlineEntro, testata)
	}
}

// (4) IL WORKER DI ANALISI SI COMPORTA ALLO STESSO MODO. Non condivide con quello di Outlook il
// codice della testata — è un chip a parte, scritto a parte — ed è il modo in cui una correzione
// viene applicata a metà senza che nessuno se ne accorga.
func TestPresenzaWorkerAnalisiUgualeAQuelloOutlook(t *testing.T) {
	b := preparaBancoWeb(t)
	const ip = "10.0.0.11"
	b.cfg.Worker = append(b.cfg.Worker, config.Worker{Nome: "analisi@PC-FRANCESCO", Tipo: "analisi",
		Token: tokenWorkerProva, Postazione: "PC-FRANCESCO"})
	b.riavvia()

	fine := b.claimInCorso("analisi", "analisi@PC-FRANCESCO", ip, 3)
	p := b.presenzaEntro("analisi@PC-FRANCESCO", 2*time.Second)
	if p.UltimoClaim != nil {
		t.Errorf("analisi: ultimo_claim = %v mentre il claim è appeso", p.UltimoClaim)
	}
	fp := b.operatoreSuPcFrancesco(ip)
	if testata := fp.testata(); !strings.Contains(testata, "analisi attiva") {
		t.Errorf("il chip dell'analisi non è attivo mentre il worker aspetta in long-poll:\n%s", testata)
	}
	<-fine

	b.indietro("analisi@PC-FRANCESCO", api.PresenzaOnlineEntro+time.Second, 0)
	if testata := fp.testata(); !strings.Contains(testata, "analisi OFFLINE") {
		t.Errorf("l'analisi tace da %v e il chip non lo dice:\n%s", api.PresenzaOnlineEntro+time.Second, testata)
	}
}
