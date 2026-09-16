//go:build integrazione

// L4 — PK2: il percorso di migrazione dal token CONDIVISO alle credenziali individuali (voce 2.4).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 -run PK2 ./internal/web/
//
// PK1 (rete_db_test.go) prova che il pacchetto di una postazione porta un token che funziona. Qui si
// prova la strada che ci si percorre davvero, ed è quella su cui il banco di prova si è fermato:
//
//	prima          un token solo, copiato in tutti i [[worker]] → in DB due righe con la STESSA impronta
//	poi            token = "" in cockpit.toml e riavvio      → il seed NON tocca i token: le due
//	               impronte duplicate restano, e i worker continuano a ricevere 401
//	infine         pacchetto dalla pagina Postazioni         → due segreti diversi, uno per worker
//
// Il passaggio che sorprende è il secondo, ed è corretto che sia così: un token vuoto significa «il
// segreto lo tiene il database» (altrimenti il pacchetto scaricato ieri smetterebbe di funzionare
// stanotte), quindi svuotare il file non ripulisce niente. Da qui due conseguenze provate qui sotto:
// la pagina Postazioni deve DIRLO invece di lasciarlo scoprire al primo 401, e la rigenerazione deve
// essere ciò che chiude la storia.
package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"promatec/cockpit/internal/config"
	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/fondazioni"
	"promatec/cockpit/internal/rete"
)

const tokenCondivisoLegacy = "IL-TOKEN-CONDIVISO-DI-PRIMA"

// dueWorkerSullaStessaPostazione porta il banco nello stato di partenza reale: su PC-FRANCESCO
// girano un worker Outlook e un worker di analisi, e tutti e due presentano lo stesso token.
func dueWorkerSullaStessaPostazione(b *bancoWeb) {
	b.t.Helper()
	b.cfg.Worker = []config.Worker{
		{Nome: "outlook@PC-FRANCESCO", Tipo: "outlook", Token: tokenCondivisoLegacy, Postazione: "PC-FRANCESCO",
			Caselle: []string{"francesco@azienda.example", "commerciale@azienda.example"}},
		{Nome: "analisi@PC-FRANCESCO", Tipo: "analisi", Token: tokenCondivisoLegacy, Postazione: "PC-FRANCESCO"},
		{Nome: "outlook@PC-LUIGI", Tipo: "outlook", Token: tokenWorkerProva + "-luigi", Postazione: "PC-LUIGI",
			Caselle: []string{"luigi@azienda.example"}},
	}
	b.riavvia()
}

// svuotaITokenDelFile è la mossa giusta di chi migra: i segreti non stanno più in cockpit.toml.
func svuotaITokenDelFile(b *bancoWeb) {
	b.t.Helper()
	for i := range b.cfg.Worker {
		if strings.HasSuffix(b.cfg.Worker[i].Postazione, "PC-FRANCESCO") {
			b.cfg.Worker[i].Token = ""
		}
	}
	b.riavvia()
}

// claimDi fa un claim come lo farebbe quel worker e restituisce lo stato HTTP. La postazione
// dichiarata è quella del nome (`analisi@PC-LUIGI` gira su PC-LUIGI): dichiararne un'altra sarebbe un
// worker.toml copiato sul PC sbagliato, e il claim lo rifiuta comunque.
func (b *bancoWeb) claimDi(tipo, nome, token string) int {
	b.t.Helper()
	postazione := nome
	if i := strings.Index(nome, "@"); i >= 0 {
		postazione = nome[i+1:]
	}
	corpo, _ := json.Marshal(map[string]any{"worker": tipo, "worker_id": nome, "attesa_s": 1,
		"postazione": postazione})
	r, _ := http.NewRequest(http.MethodPost, b.srv.URL+"/api/v1/jobs/claim", strings.NewReader(string(corpo)))
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

// caselleCon chiede GET /api/v1/worker/caselle con un token: è la prima cosa che fa il worker
// Outlook all'avvio, e la seconda prova che quella credenziale lavora davvero.
func (b *bancoWeb) caselleCon(token, nome string) (int, string) {
	b.t.Helper()
	r, _ := http.NewRequest(http.MethodGet, b.srv.URL+"/api/v1/worker/caselle?worker_id="+nome, nil)
	r.Header.Set("X-Cockpit-Token", token)
	r.Header.Set("X-Prova-IP", "10.0.0.5:5000")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	corpo, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(corpo)
}

func TestPK2DalTokenCondivisoADueCredenzialiIndividuali(t *testing.T) {
	b := preparaBancoWeb(t)
	dueWorkerSullaStessaPostazione(b)

	// ---------------------------------------------------------------- lo stato di partenza
	// Due righe, una impronta sola. Nessuno dei due worker può entrare, e il motivo li nomina tutti e
	// due: assegnare l'identità al primo in ordine alfabetico funzionerebbe quasi sempre, e
	// sbaglierebbe senza dirlo.
	if s := b.claimDi("outlook", "outlook@PC-FRANCESCO", tokenCondivisoLegacy); s != 401 {
		t.Fatalf("il token condiviso autentica ancora qualcuno: %d (atteso 401)", s)
	}
	if s := b.claimDi("analisi", "analisi@PC-FRANCESCO", tokenCondivisoLegacy); s != 401 {
		t.Fatalf("il token condiviso autentica ancora qualcuno: %d (atteso 401)", s)
	}

	// ---------------------------------------------------------------- si svuotano i token nel file
	// È la mossa giusta, e da sola NON basta: il seed non tocca i token che il file non dichiara,
	// quindi le due impronte duplicate restano dov'erano. È esattamente ciò che è successo sul banco.
	svuotaITokenDelFile(b)
	cred, err := b.q.CredenzialiPerToken(b.ctx, rete.ImprontaToken(tokenCondivisoLegacy))
	if err != nil {
		t.Fatal(err)
	}
	if len(cred) != 2 {
		t.Fatalf("svuotare i token nel file ha cambiato i segreti in database: %d righe con l'impronta di prima, attese 2", len(cred))
	}
	if s := b.claimDi("outlook", "outlook@PC-FRANCESCO", tokenCondivisoLegacy); s != 401 {
		t.Fatalf("dopo aver svuotato il file il vecchio token dovrebbe restare 401: %d", s)
	}

	// ---------------------------------------------------------------- la pagina lo dice
	admin := b.browser("10.0.0.9:5000")
	admin.login("AD", "prova-ad")
	_, pagina := admin.fai(http.MethodGet, "/admin/postazioni", nil, false)
	for _, atteso := range []string{"Credenziali da rigenerare", "token condiviso", "Rigenera credenziali della postazione"} {
		if !strings.Contains(pagina, atteso) {
			t.Errorf("la pagina Postazioni non segnala lo stato delle credenziali: manca %q", atteso)
		}
	}
	if !strings.Contains(pagina, "analisi@PC-FRANCESCO") {
		t.Errorf("la pagina non nomina il worker che sta condividendo il token")
	}

	// ---------------------------------------------------------------- si rigenera
	dentro := scarica(t, admin, "PC-FRANCESCO")
	toml := dentro["worker.toml"]
	tokenOutlook := tokenDa(t, toml, "outlook")
	tokenAnalisi := tokenDa(t, toml, "analisi")
	if tokenOutlook == tokenAnalisi {
		t.Fatalf("il pacchetto dà lo STESSO token ai due worker: si ricomincerebbe da capo\n%s", toml)
	}
	if tokenOutlook == tokenCondivisoLegacy || tokenAnalisi == tokenCondivisoLegacy {
		t.Fatal("il pacchetto riusa il token condiviso di prima")
	}

	// Ogni token identifica UNA sola credenziale: è la definizione di credenziale individuale.
	for _, c := range []struct{ token, nome string }{
		{tokenOutlook, "outlook@PC-FRANCESCO"},
		{tokenAnalisi, "analisi@PC-FRANCESCO"},
	} {
		righe, err := b.q.CredenzialiPerToken(b.ctx, rete.ImprontaToken(c.token))
		if err != nil {
			t.Fatal(err)
		}
		if len(righe) != 1 || righe[0].WorkerNome != c.nome {
			t.Fatalf("il token di %s identifica %d credenziali (%v)", c.nome, len(righe), righe)
		}
	}

	// ---------------------------------------------------------------- il vecchio token è morto
	if s := b.claimDi("outlook", "outlook@PC-FRANCESCO", tokenCondivisoLegacy); s != 401 {
		t.Errorf("il token condiviso funziona ancora dopo la rigenerazione: %d", s)
	}

	// ---------------------------------------------------------------- nessuno dei due è l'altro
	// Il token è l'identità: presentarsi con quello del collega non è un modo per entrare, ed è
	// proprio il difetto che la voce 2.4 chiude. 403 e non 401: la credenziale è valida, il nome no.
	if s := b.claimDi("outlook", "outlook@PC-FRANCESCO", tokenAnalisi); s != 403 {
		t.Errorf("il worker Outlook si è autenticato con il token del worker di analisi: %d (atteso 403)", s)
	}
	if s := b.claimDi("analisi", "analisi@PC-FRANCESCO", tokenOutlook); s != 403 {
		t.Errorf("il worker di analisi si è autenticato con il token di Outlook: %d (atteso 403)", s)
	}

	// ---------------------------------------------------------------- e adesso lavorano tutti e due
	if s := b.claimDi("outlook", "outlook@PC-FRANCESCO", tokenOutlook); s != 200 && s != 204 {
		t.Errorf("il worker Outlook non lavora con la sua credenziale: %d", s)
	}
	if s := b.claimDi("analisi", "analisi@PC-FRANCESCO", tokenAnalisi); s != 200 && s != 204 {
		t.Errorf("il worker di analisi non lavora con la sua credenziale: %d", s)
	}
	// Il worker Outlook chiede anche le proprie caselle: sono le sue due, non quelle di Luigi.
	stato, corpo := b.caselleCon(tokenOutlook, "outlook@PC-FRANCESCO")
	if stato != 200 {
		t.Fatalf("GET /worker/caselle con la credenziale nuova: %d", stato)
	}
	if !strings.Contains(corpo, "francesco@azienda.example") || !strings.Contains(corpo, "commerciale@azienda.example") {
		t.Errorf("le caselle servite non sono quelle della credenziale: %s", corpo)
	}
	if strings.Contains(corpo, "luigi@azienda.example") {
		t.Errorf("la credenziale di PC-FRANCESCO si è fatta dare la casella di Luigi: %s", corpo)
	}

	// ---------------------------------------------------------------- il riavvio non li cancella
	// La regola della voce 2.4 vista dal lato che conta: con `token` vuoto nel file, il seed del
	// riavvio successivo NON deve toccare i segreti appena generati. Se li riscrivesse, il pacchetto
	// copiato sul PC smetterebbe di funzionare stanotte senza che nessuno abbia cambiato niente.
	b.riavvia()
	if s := b.claimDi("outlook", "outlook@PC-FRANCESCO", tokenOutlook); s != 200 && s != 204 {
		t.Errorf("dopo il riavvio la credenziale generata non vale più: %d", s)
	}
	if s := b.claimDi("analisi", "analisi@PC-FRANCESCO", tokenAnalisi); s != 200 && s != 204 {
		t.Errorf("dopo il riavvio la credenziale generata non vale più: %d", s)
	}
	// e la pagina non ha più niente da segnalare su quella postazione
	_, pagina = admin.fai(http.MethodGet, "/admin/postazioni", nil, false)
	if strings.Contains(pagina, "Credenziali da rigenerare") {
		t.Errorf("la pagina segnala ancora un problema sulle credenziali appena generate")
	}
}

// Il terzo stato, quello di un PC censito e mai provvisto: la credenziale esiste, dice quali caselle
// serve, e non fa entrare nessuno. Prima era indistinguibile da un segreto vero — un token casuale in
// database — e lo si scopriva soltanto dal primo 401.
func TestUnaCredenzialeMaiGenerataSiVedePrimaDelPrimo401(t *testing.T) {
	b := preparaBancoWeb(t)
	b.cfg.Worker = append(b.cfg.Worker, config.Worker{
		Nome: "analisi@PC-LUIGI", Tipo: "analisi", Token: "", Postazione: "PC-LUIGI",
	})
	b.riavvia()

	c, err := b.q.GetWorkerCredenziale(b.ctx, "analisi@PC-LUIGI")
	if err != nil {
		t.Fatal(err)
	}
	if c.TokenHash != rete.ImprontaNonGenerata {
		t.Fatalf("una credenziale senza token non è riconoscibile come tale: %s", c.TokenHash)
	}
	// Il segnaposto non è presentabile: l'impronta è quella della stringa vuota, e un header vuoto è
	// già «credenziale mancante». Che il server risponda comunque 401 va provato, non dedotto.
	if s := b.claimDi("analisi", "analisi@PC-LUIGI", ""); s != 401 {
		t.Errorf("claim senza token: %d (atteso 401)", s)
	}
	if s := b.claimDi("analisi", "analisi@PC-LUIGI", "   "); s != 401 {
		t.Errorf("claim con un token di soli spazi: %d (atteso 401)", s)
	}

	admin := b.browser("10.0.0.9:5000")
	admin.login("AD", "prova-ad")
	_, pagina := admin.fai(http.MethodGet, "/admin/postazioni", nil, false)
	if !strings.Contains(pagina, "credenziale da generare") {
		t.Errorf("la pagina non dice che la credenziale di analisi@PC-LUIGI è ancora da generare")
	}

	// Generato il pacchetto, il segnaposto sparisce e quel worker lavora.
	dentro := scarica(t, admin, "PC-LUIGI")
	token := tokenDa(t, dentro["worker.toml"], "analisi")
	if s := b.claimDi("analisi", "analisi@PC-LUIGI", token); s != 200 && s != 204 {
		t.Errorf("il worker di analisi non lavora con la credenziale appena generata: %d", s)
	}
}

// La diagnosi è una funzione pura, e questo è il suo test L1: sta qui perché è qui che si legge
// accanto ai due casi veri.
func TestStatoDelleCredenziali(t *testing.T) {
	righe := []db.WorkerCredenziale{
		{WorkerNome: "outlook@PC-A", TokenHash: rete.ImprontaToken("uno"), Attivo: true},
		{WorkerNome: "analisi@PC-A", TokenHash: rete.ImprontaToken("uno"), Attivo: true},
		{WorkerNome: "outlook@PC-B", TokenHash: rete.ImprontaToken("due"), Attivo: true},
		{WorkerNome: "analisi@PC-B", TokenHash: rete.ImprontaNonGenerata, Attivo: true},
		// Una credenziale disattivata non toglie l'identità a nessuno: non autentica comunque.
		{WorkerNome: "outlook@PC-VECCHIO", TokenHash: rete.ImprontaToken("due"), Attivo: false},
	}
	s := fondazioni.StatoCredenziali(righe)
	if s["outlook@PC-A"].Stato != fondazioni.CredenzialeCondivisa ||
		len(s["outlook@PC-A"].ConChi) != 1 || s["outlook@PC-A"].ConChi[0] != "analisi@PC-A" {
		t.Errorf("token condiviso non riconosciuto, o non dice con chi: %+v", s["outlook@PC-A"])
	}
	if s["analisi@PC-B"].Stato != fondazioni.CredenzialeDaGenerare {
		t.Errorf("credenziale mai generata non riconosciuta: %+v", s["analisi@PC-B"])
	}
	if !s["outlook@PC-B"].Utilizzabile() {
		t.Errorf("una credenziale sana è stata dichiarata guasta perché un worker DISATTIVATO aveva lo stesso token")
	}
}
