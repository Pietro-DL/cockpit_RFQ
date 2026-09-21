//go:build integrazione

// L4 — voci 2.2 e 2.6 lato API: il claim non si fida del JSON (Q18), assegna solo caselle aperte
// (M12), registra presenza per worker e store locale (M1 parte server, M2), rifiuta chi non è censito.
package workerapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/platform/config"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/fondazioni"
	"promatec/cockpit/internal/platform/testutil"
)

// bancoClaim: server vero su httptest, fondazioni seminate da una configurazione con due caselle
// autorizzate a Francesco (Francesco, Commerciale), una censita ma non sua (Luigi), e un worker su
// PC-FRANCESCO. L'IP del chiamante è preso da un header, SOLO qui: in produzione conta RemoteAddr.
type bancoClaim struct {
	*banco
	commerciale, francesco, luigi uuid.UUID
	pcFrancesco                   uuid.UUID
}

func preparaBancoClaim(t *testing.T) *bancoClaim {
	t.Helper()
	b := preparaBanco(t, 0)
	b.s.IndirizzoClient = func(r *http.Request) string { return r.Header.Get("X-Prova-IP") }
	cfg := &config.Config{}
	cfg.Outlook.CasellaDefault = "francesco@azienda.example"
	cfg.Caselle = []config.Casella{
		{Indirizzo: "commerciale@azienda.it", Nome: "Commerciale", Canale: "outlook", Condivisa: true}, // quella del banco
		{Indirizzo: "francesco@azienda.example", Nome: "Francesco", Canale: "outlook"},
		{Indirizzo: "luigi@azienda.example", Nome: "Luigi", Canale: "outlook"},
	}
	cfg.Postazioni = []config.Postazione{{NomeHost: "PC-FRANCESCO"}, {NomeHost: "PC-LUIGI"}}
	cfg.Worker = []config.Worker{
		{Nome: "outlook@PC-FRANCESCO", Tipo: "outlook", Token: tokenDi("outlook@PC-FRANCESCO"), Postazione: "PC-FRANCESCO",
			Caselle: []string{"francesco@azienda.example", "commerciale@azienda.it"}},
		{Nome: "outlook@PC-LUIGI", Tipo: "outlook", Token: tokenDi("outlook@PC-LUIGI"), Postazione: "PC-LUIGI", Caselle: []string{"luigi@azienda.example"}},
		{Nome: "analisi@PC-FRANCESCO", Tipo: "analisi", Token: tokenDi("analisi@PC-FRANCESCO"), Postazione: "PC-FRANCESCO"},
	}
	if _, err := fondazioni.Semina(b.ctx, b.q, cfg, testutil.LogSilenzioso()); err != nil {
		t.Fatal(err)
	}
	bc := &bancoClaim{banco: b}
	casella := func(ind string) uuid.UUID {
		c, err := b.q.GetCasellaPerIndirizzo(b.ctx, db.GetCasellaPerIndirizzoParams{Canale: db.CanaleOutlook, Indirizzo: ind})
		if err != nil {
			t.Fatal(err)
		}
		return c.CasellaID
	}
	bc.commerciale, bc.francesco, bc.luigi = casella("commerciale@azienda.it"), casella("francesco@azienda.example"), casella("luigi@azienda.example")
	p, err := b.q.GetPostazionePerHost(b.ctx, "PC-FRANCESCO")
	if err != nil {
		t.Fatal(err)
	}
	bc.pcFrancesco = p.PostazioneID
	return bc
}

// claimHTTP fa il claim come il worker vero, via HTTP, dall'IP indicato.
func (b *bancoClaim) claimHTTP(req worker.ClaimRichiesta, ip string) (*http.Response, *worker.Job) {
	b.t.Helper()
	if req.AttesaS == 0 {
		req.AttesaS = 1
	}
	corpo, _ := json.Marshal(req)
	r, _ := http.NewRequest(http.MethodPost, b.srv.URL+"/api/v1/jobs/claim", bytes.NewReader(corpo))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Cockpit-Token", tokenDi(req.WorkerID))
	r.Header.Set("X-Prova-IP", ip)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return resp, nil
	}
	var j worker.Job
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		b.t.Fatal(err)
	}
	return resp, &j
}

func (b *bancoClaim) presenza(nome string) db.WorkerPresenza {
	b.t.Helper()
	p, err := b.q.GetWorkerPresenza(b.ctx, nome)
	if err != nil {
		b.t.Fatalf("presenza di %s: %v", nome, err)
	}
	return p
}

// Q18 — credenziale [Francesco, Commerciale], claim dichiara anche Luigi: intersezione, avviso.
// Il job del banco è di Commerciale (dichiarata e autorizzata): arriva. Un job di Luigi no.
func TestQ18ClaimConCaselleNonAutorizzate(t *testing.T) {
	b := preparaBancoClaim(t)
	// un job di Luigi, che il worker di Francesco dichiara ma non è autorizzato a servire
	if _, err := jobs.AccodaCon(b.ctx, b.q, db.TipoJobSyncOutlook, map[string]any{}, "sync-luigi", 5,
		jobs.Opzioni{Casella: uuid.NullUUID{UUID: b.luigi, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	dichiara := worker.ClaimRichiesta{Worker: "outlook", WorkerID: "outlook@PC-FRANCESCO", Postazione: "PC-FRANCESCO", OutlookOk: true,
		CaselleAperte: []worker.CasellaAperta{
			{CasellaID: b.francesco, StoreID: "STORE-F"}, {CasellaID: b.commerciale, StoreID: "STORE-C"}, {CasellaID: b.luigi, StoreID: "STORE-L"}}}
	presi := map[uuid.UUID]bool{}
	for i := 0; i < 3; i++ {
		resp, j := b.claimHTTP(dichiara, "10.0.0.5:5000")
		if resp.StatusCode == 204 {
			break
		}
		if resp.StatusCode != 200 {
			t.Fatalf("claim: %d", resp.StatusCode)
		}
		presi[*j.CasellaID] = true
	}
	if !presi[b.commerciale] || presi[b.luigi] {
		t.Fatalf("assegnati: %v — atteso solo il job di Commerciale, mai quello di Luigi", presi)
	}
	// l'avviso è visibile nella presenza, e la presenza dice solo le caselle servite davvero
	p := b.presenza("outlook@PC-FRANCESCO")
	if !p.Avviso.Valid || !strings.Contains(p.Avviso.String, "non autorizzate") {
		t.Errorf("nessun avviso sulla presenza: %+v", p.Avviso)
	}
	if len(p.CaselleAperte) != 2 || contieneUUID(p.CaselleAperte, b.luigi) {
		t.Errorf("caselle_aperte = %v: Luigi non doveva esserci", p.CaselleAperte)
	}
	// casella_store: scritta per le due autorizzate, MAI per Luigi (M1 lato server)
	righe, err := b.q.ListCasellaStorePerPostazione(b.ctx, b.pcFrancesco)
	if err != nil {
		t.Fatal(err)
	}
	store := map[uuid.UUID]string{}
	for _, r := range righe {
		store[r.CasellaID] = r.StoreID
	}
	if store[b.francesco] != "STORE-F" || store[b.commerciale] != "STORE-C" || store[b.luigi] != "" {
		t.Errorf("casella_store di PC-FRANCESCO: %v", store)
	}
}

// M2 — la presenza è per worker e porta postazione, IP, Outlook, caselle. Con outlook_ok=false e
// nessuna casella il worker è vivo ma non serve niente: è il terzo stato della testata.
func TestM2PresenzaPerWorker(t *testing.T) {
	b := preparaBancoClaim(t)
	resp, _ := b.claimHTTP(worker.ClaimRichiesta{Worker: "outlook", WorkerID: "outlook@PC-FRANCESCO", Postazione: "PC-FRANCESCO", OutlookOk: false}, "10.0.0.5:5000")
	if resp.StatusCode != 204 {
		t.Fatalf("claim senza caselle: %d (il job del banco è di Commerciale, non dichiarata)", resp.StatusCode)
	}
	p := b.presenza("outlook@PC-FRANCESCO")
	if p.OutlookOk || len(p.CaselleAperte) != 0 || !p.PostazioneID.Valid || p.PostazioneID.UUID != b.pcFrancesco ||
		p.IndirizzoIp == nil || p.IndirizzoIp.String() != "10.0.0.5" || p.WorkerTipo != db.WorkerTipoOutlook {
		t.Fatalf("presenza: %+v", p)
	}
	// il worker di analisi ha la sua riga, separata
	if resp, _ := b.claimHTTP(worker.ClaimRichiesta{Worker: "analisi", WorkerID: "analisi@PC-FRANCESCO"}, "10.0.0.5:5001"); resp.StatusCode != 204 {
		t.Fatalf("claim analisi: %d", resp.StatusCode)
	}
	righe, _ := b.q.ListWorkerPresenza(b.ctx)
	if len(righe) != 2 {
		t.Fatalf("presenze = %d, attese 2 (una per worker, non una per tipo)", len(righe))
	}
	// ultimo_arresto riportato al riavvio resta anche nei claim successivi che non lo portano
	b.claimHTTP(worker.ClaimRichiesta{Worker: "outlook", WorkerID: "outlook@PC-FRANCESCO", OutlookOk: true, UltimoArresto: "job 7: COM bloccato"}, "10.0.0.5:5000")
	b.claimHTTP(worker.ClaimRichiesta{Worker: "outlook", WorkerID: "outlook@PC-FRANCESCO", OutlookOk: true}, "10.0.0.5:5000")
	if p := b.presenza("outlook@PC-FRANCESCO"); !p.UltimoArresto.Valid || p.UltimoArresto.String != "job 7: COM bloccato" {
		t.Errorf("ultimo_arresto perso: %+v", p.UltimoArresto)
	}
}

// Un worker non censito, di tipo diverso dalla credenziale, o su un'altra postazione: 403 con il
// motivo, nessuna presenza, nessun job.
func TestClaimRifiutaWorkerNonCensitoOSuAltraPostazione(t *testing.T) {
	b := preparaBancoClaim(t)
	// Dalla voce 2.4 il token È l'identità: un nome che non ha una credenziale non arriva nemmeno al
	// gestore, perché non ha un token con cui presentarsi (401). Gli altri casi restano 403: la
	// credenziale è buona, ma dice qualcosa di diverso da ciò che la richiesta dichiara.
	casi := []struct {
		nome   string
		token  string
		req    worker.ClaimRichiesta
		stato  int
		attesa string
	}{
		{"credenziale sconosciuta", tokenProva,
			worker.ClaimRichiesta{Worker: "outlook", WorkerID: "outlook@PC-IGNOTO"}, 401, "credenziale non riconosciuta"},
		{"tipo diverso da quello censito", tokenDi("outlook@PC-FRANCESCO"),
			worker.ClaimRichiesta{Worker: "analisi", WorkerID: "outlook@PC-FRANCESCO"}, 403, "censito come outlook"},
		{"worker.toml copiato su un altro PC", tokenDi("outlook@PC-FRANCESCO"),
			worker.ClaimRichiesta{Worker: "outlook", WorkerID: "outlook@PC-FRANCESCO", Postazione: "PC-LUIGI"}, 403, "copiato su un altro PC"},
		{"la credenziale di uno, il nome di un altro", tokenDi("outlook@PC-FRANCESCO"),
			worker.ClaimRichiesta{Worker: "outlook", WorkerID: "outlook@PC-LUIGI"}, 403, "il nome di un altro worker"},
	}
	for _, c := range casi {
		r, _ := http.NewRequest(http.MethodPost, b.srv.URL+"/api/v1/jobs/claim", bytes.NewReader(mustJSON(c.req)))
		r.Header.Set("X-Cockpit-Token", c.token)
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		var corpo bytes.Buffer
		_, _ = corpo.ReadFrom(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != c.stato || !strings.Contains(corpo.String(), c.attesa) {
			t.Errorf("%s: %d %s (atteso %d con %q)", c.nome, resp.StatusCode, corpo.String(), c.stato, c.attesa)
		}
	}
	if n := testutil.Conta(t, b.pool, "worker_presenza"); n != 0 {
		t.Errorf("un claim rifiutato ha scritto una presenza (%d righe)", n)
	}
	if j := b.jobOra(); j.Stato != db.StatoJobPronto {
		t.Errorf("il job del banco è stato assegnato: %s", j.Stato)
	}
}

// GET /api/v1/worker/caselle — al worker arrivano SOLO le caselle attive autorizzate: né Luigi (non
// sua), né una casella disattivata, né — ovviamente — uno store che non è censito affatto.
func TestCaselleWorkerSoloAutorizzateEAttive(t *testing.T) {
	b := preparaBancoClaim(t)
	if _, err := b.pool.Exec(b.ctx, `UPDATE casella SET attiva = false WHERE casella_id = $1`, b.francesco); err != nil {
		t.Fatal(err)
	}
	r, _ := http.NewRequest(http.MethodGet, b.srv.URL+"/api/v1/worker/caselle?worker_id=outlook@PC-FRANCESCO", nil)
	r.Header.Set("X-Cockpit-Token", tokenDi("outlook@PC-FRANCESCO"))
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out []worker.CasellaServita
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != 200 {
		t.Fatalf("caselle: %d %v", resp.StatusCode, err)
	}
	if len(out) != 1 || out[0].CasellaID != b.commerciale || !out[0].Condivisa || out[0].Indirizzo != "commerciale@azienda.it" {
		t.Fatalf("caselle servite: %+v (attesa la sola Commerciale: Francesco è disattivata, Luigi non è autorizzata)", out)
	}
	// chiedere le caselle di un altro worker: 403, anche con una credenziale buona in mano
	r2, _ := http.NewRequest(http.MethodGet, b.srv.URL+"/api/v1/worker/caselle?worker_id=outlook@PC-LUIGI", nil)
	r2.Header.Set("X-Cockpit-Token", tokenDi("outlook@PC-FRANCESCO"))
	if resp2, err := http.DefaultClient.Do(r2); err != nil || resp2.StatusCode != 403 {
		t.Fatalf("caselle di un altro worker: %v %v", resp2, err)
	}
	// e senza credenziale non si arriva nemmeno al gestore: 401
	r3, _ := http.NewRequest(http.MethodGet, b.srv.URL+"/api/v1/worker/caselle?worker_id=outlook@PC-FRANCESCO", nil)
	if resp3, err := http.DefaultClient.Do(r3); err != nil || resp3.StatusCode != 401 {
		t.Fatalf("senza credenziale: %v %v", resp3, err)
	}
}

// Il download del banco (job di Commerciale) via claim HTTP: arriva solo a chi dichiara Commerciale,
// e il job che arriva porta postazione_id assente (non è interattivo) e casella_id.
func TestClaimHTTPRispettaLaCasellaDelJob(t *testing.T) {
	b := preparaBancoClaim(t)
	resp, _ := b.claimHTTP(worker.ClaimRichiesta{Worker: "outlook", WorkerID: "outlook@PC-LUIGI", OutlookOk: true,
		CaselleAperte: []worker.CasellaAperta{{CasellaID: b.luigi, StoreID: "S"}}}, "10.0.0.7:1")
	if resp.StatusCode != 204 {
		t.Fatalf("PC-LUIGI ha ricevuto il job di Commerciale: %d", resp.StatusCode)
	}
	resp, j := b.claimHTTP(worker.ClaimRichiesta{Worker: "outlook", WorkerID: "outlook@PC-FRANCESCO", OutlookOk: true,
		CaselleAperte: []worker.CasellaAperta{{CasellaID: b.commerciale, StoreID: "S-C"}}}, "10.0.0.5:1")
	if resp.StatusCode != 200 || j == nil || j.CasellaID == nil || *j.CasellaID != b.commerciale || j.PostazioneID != nil {
		t.Fatalf("PC-FRANCESCO: %d %+v", resp.StatusCode, j)
	}
	if strings.Contains(string(j.Payload), `"store_id"`) {
		t.Errorf("il payload consegnato al worker porta uno store_id: %s", j.Payload)
	}
}

func contieneUUID(ids []uuid.UUID, id uuid.UUID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
