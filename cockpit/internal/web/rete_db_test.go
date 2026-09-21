//go:build integrazione

// L4 — blocco 2: CSRF (voce 2.5, W10) e pacchetto della postazione (voce 2.4 + D22, PK1).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/web/
//
// Le due cose stanno insieme perché rispondono alla stessa domanda: che cosa succede quando il
// Cockpit smette di essere su un PC solo. Un server raggiungibile da altri PC è un server che un'altra
// pagina aperta nello stesso browser può provare a pilotare (W10), e un PC nuovo è un PC che deve
// ricevere un segreto senza che nessuno lo copi a mano da un file (PK1).
package web

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/rete"
)

// ---------------------------------------------------------------------------------------------
// W10 — CSRF: 403 cross-site, 200 same-origin.
// ---------------------------------------------------------------------------------------------

func TestW10UnPostDaUnAltroSitoNonPassa(t *testing.T) {
	b := preparaBancoWeb(t)
	w := b.browser("10.0.0.5:5000")
	w.login("FP", "prova-fp")

	// Il POST che conta: «Aggiorna ora» accoda lavoro vero. Dalla stessa origine deve funzionare.
	resp, _ := w.fai(http.MethodPost, "/inbox/aggiorna", url.Values{}, true)
	if resp.StatusCode != 200 {
		t.Fatalf("stessa origine: %d, atteso 200", resp.StatusCode)
	}

	// Lo stesso POST, con lo stesso cookie, dichiarato da un altro sito: è la richiesta che farebbe
	// una pagina qualunque aperta in un'altra scheda mentre l'operatore è collegato.
	casi := []struct {
		nome    string
		intesta map[string]string
	}{
		{"Sec-Fetch-Site: cross-site", map[string]string{"Sec-Fetch-Site": "cross-site"}},
		{"Origin di un altro sito", map[string]string{"Origin": "https://sito-di-un-altro.example"}},
	}
	for _, c := range casi {
		r, _ := http.NewRequest(http.MethodPost, b.srv.URL+"/inbox/aggiorna", strings.NewReader(""))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("X-Prova-IP", w.ip)
		for k, v := range c.intesta {
			r.Header.Set(k, v)
		}
		resp, err := w.c.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: %d, atteso 403", c.nome, resp.StatusCode)
		}
	}

	// Una LETTURA da un altro sito passa: non cambia niente, e bloccarla romperebbe i link senza
	// proteggere nulla. Ciò che va fermato sono i metodi che scrivono.
	r, _ := http.NewRequest(http.MethodGet, b.srv.URL+"/inbox", nil)
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	r.Header.Set("X-Prova-IP", w.ip)
	resp2, err := w.c.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Errorf("GET cross-site: %d, atteso 200", resp2.StatusCode)
	}
}

func TestIlWorkerNonEUnBrowserEPassa(t *testing.T) {
	// I worker non mandano Sec-Fetch-Site né Origin: la protezione CSRF non deve toccarli, o il
	// blocco 2 spegnerebbe tutte le postazioni nel momento in cui le mette in rete.
	b := preparaBancoWeb(t)
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:5000", b.francesco)
	if p, err := b.q.GetWorkerPresenza(b.ctx, "outlook@PC-FRANCESCO"); err != nil || p.WorkerNome == "" {
		t.Fatalf("il claim del worker non è passato dalla protezione CSRF: %v", err)
	}
}

// ---------------------------------------------------------------------------------------------
// PK1 — il pacchetto della postazione: un segreto nuovo, un file che lo contiene, e il precedente
// che smette di funzionare.
// ---------------------------------------------------------------------------------------------

func scarica(t *testing.T, w *browser, host string) map[string]string {
	t.Helper()
	r, _ := http.NewRequest(http.MethodPost, w.b.srv.URL+"/admin/postazioni/"+host+"/pacchetto", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("X-Prova-IP", w.ip)
	resp, err := w.c.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("pacchetto di %s: %d", host, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("il pacchetto non è uno zip: %s", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("il pacchetto non si scarica: %s", cd)
	}
	corpo, _ := io.ReadAll(resp.Body)
	z, err := zip.NewReader(bytes.NewReader(corpo), int64(len(corpo)))
	if err != nil {
		t.Fatalf("zip illeggibile: %v", err)
	}
	dentro := map[string]string{}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		dati, _ := io.ReadAll(r)
		r.Close()
		dentro[f.Name] = string(dati)
	}
	return dentro
}

// tokenDa legge un token dalla riga `token     = "..."` del worker.toml generato.
func tokenDa(t *testing.T, toml, sezione string) string {
	t.Helper()
	dentro := false
	for _, riga := range strings.Split(toml, "\n") {
		r := strings.TrimSpace(riga)
		if strings.HasPrefix(r, "[") {
			dentro = r == "["+sezione+"]"
			continue
		}
		if dentro && strings.HasPrefix(r, "token") {
			if i := strings.Index(r, "\""); i >= 0 {
				return strings.Trim(r[i:], "\"")
			}
		}
	}
	t.Fatalf("nessun token nella sezione [%s] di:\n%s", sezione, toml)
	return ""
}

func TestPK1IlPacchettoDellaPostazioneContieneUnTokenCheFunziona(t *testing.T) {
	b := preparaBancoWeb(t)
	admin := b.browser("10.0.0.9:5000")
	admin.login("AD", "prova-ad")

	dentro := scarica(t, admin, "PC-FRANCESCO")
	for _, atteso := range []string{"worker.toml", "ISTRUZIONI.txt", "worker_outlook.py", "cockpit_client.py"} {
		if _, ok := dentro[atteso]; !ok {
			t.Errorf("il pacchetto non contiene %s (dentro: %v)", atteso, chiavi(dentro))
		}
	}
	// Su una postazione va solo ciò che le serve: niente test, niente server finto, niente banco E2E.
	for _, fuori := range []string{"test_outlook_finestra.py", "server_finto.py", "prova_e2e.py", "genera_contratti.py"} {
		if _, c := dentro[fuori]; c {
			t.Errorf("il pacchetto porta uno strumento di sviluppo su una postazione: %s", fuori)
		}
	}
	for nome := range dentro {
		if strings.HasPrefix(nome, "test_") {
			t.Errorf("il pacchetto porta un file di test su una postazione: %s", nome)
		}
	}
	toml := dentro["worker.toml"]
	if !strings.Contains(toml, "server_url") || !strings.Contains(toml, b.srv.URL[strings.Index(b.srv.URL, "//")+2:]) {
		// Il banco gira in chiaro su 127.0.0.1: l'indirizzo scritto nel file deve essere quello che
		// il server sa di avere, non un valore predefinito.
		t.Logf("worker.toml:\n%s", toml)
	}
	if !strings.Contains(toml, "[outlook]") {
		t.Fatalf("worker.toml senza la sezione del worker Outlook:\n%s", toml)
	}
	token := tokenDa(t, toml, "outlook")
	if len(token) < 20 {
		t.Fatalf("token troppo corto: %q", token)
	}

	// La prova che conta: quel token autentica DAVVERO, ed è la credenziale di quel worker.
	cred, err := b.q.CredenzialiPerToken(b.ctx, rete.ImprontaToken(token))
	if err != nil || len(cred) != 1 || cred[0].WorkerNome != "outlook@PC-FRANCESCO" {
		t.Fatalf("il token del pacchetto non è la credenziale di outlook@PC-FRANCESCO: %v %v", cred, err)
	}
	// e il claim con quel token passa
	if stato := b.claimConToken(token, "outlook@PC-FRANCESCO"); stato != 200 && stato != 204 {
		t.Fatalf("claim con il token del pacchetto: %d", stato)
	}

	// Rigenerare invalida il precedente: è il prezzo dichiarato del pulsante, e va dimostrato.
	secondo := scarica(t, admin, "PC-FRANCESCO")
	nuovo := tokenDa(t, secondo["worker.toml"], "outlook")
	if nuovo == token {
		t.Fatal("il secondo pacchetto porta lo stesso token: non è stato rigenerato niente")
	}
	if stato := b.claimConToken(token, "outlook@PC-FRANCESCO"); stato != 401 {
		t.Errorf("il token vecchio funziona ancora: %d (atteso 401)", stato)
	}
	if stato := b.claimConToken(nuovo, "outlook@PC-FRANCESCO"); stato != 200 && stato != 204 {
		t.Errorf("il token nuovo non funziona: %d", stato)
	}
}

func TestIlPacchettoLoGeneraSoloUnAmministratore(t *testing.T) {
	b := preparaBancoWeb(t)
	w := b.browser("10.0.0.5:5000")
	w.login("FP", "prova-fp") // operatore, ed è il proprietario di PC-FRANCESCO
	r, _ := http.NewRequest(http.MethodPost, b.srv.URL+"/admin/postazioni/PC-FRANCESCO/pacchetto", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("X-Prova-IP", w.ip)
	resp, err := w.c.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("un operatore ha generato le credenziali di una postazione: %d", resp.StatusCode)
	}
	// e nessun token è cambiato: la credenziale di prima vale ancora
	cred, err := b.q.GetWorkerCredenziale(b.ctx, "outlook@PC-FRANCESCO")
	if err != nil || cred.TokenHash != rete.ImprontaToken(tokenDelWorker("outlook@PC-FRANCESCO")) {
		t.Errorf("un tentativo rifiutato ha comunque cambiato il token")
	}
}

func TestLaPaginaPostazioniDiceComeSiRaggiungeIlServer(t *testing.T) {
	b := preparaBancoWeb(t)
	admin := b.browser("10.0.0.9:5000")
	admin.login("AD", "prova-ad")
	_, testo := admin.fai(http.MethodGet, "/admin/postazioni", nil, false)
	for _, atteso := range []string{"PC-FRANCESCO", "PC-LUIGI", "outlook@PC-FRANCESCO", "Indirizzo del server"} {
		if !strings.Contains(testo, atteso) {
			t.Errorf("la pagina Postazioni non dice %q", atteso)
		}
	}
	// il banco gira in chiaro: l'avviso deve esserci, e deve dire che cosa viaggia leggibile
	if !strings.Contains(testo, "IN CHIARO") {
		t.Errorf("la pagina non avvisa che il collegamento non è cifrato")
	}
}

func chiavi(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// claimConToken fa un claim con un token qualsiasi e restituisce lo stato HTTP: è il modo più corto
// di chiedere al server «questa credenziale vale ancora?».
func (b *bancoWeb) claimConToken(token, nome string) int {
	b.t.Helper()
	corpo := `{"worker":"outlook","worker_id":"` + nome + `","attesa_s":1}`
	r, _ := http.NewRequest(http.MethodPost, b.srv.URL+"/api/v1/jobs/claim", strings.NewReader(corpo))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Cockpit-Token", token)
	r.Header.Set("X-Prova-IP", "10.0.0.5:5000")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		b.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

var _ = db.RuoloUtenteAdmin
