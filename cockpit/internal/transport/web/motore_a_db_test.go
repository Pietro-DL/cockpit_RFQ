//go:build integrazione

// L4 — giro 5, A1d: la Distinta nuova sul DB di prova (A1d-L4-01…08; piano A1d backend §9.2; BE-18; V-29). La pagina
// /thread/{id}/motore-a, il suo frammento, i dati JSON e l'anteprima dello STEP contro PostgreSQL vero, con le regole ACME
// dell'avvio (le stesse di app/runtime/testdata/motore_a, lette dal file perché transport non importa app: T-A1d-34):
//   - A1d-L4-01: flag spento, 404 a tutti e prima della sessione (R36); un gesto con l'indirizzo della pagina riceve la
//     risposta di sempre;
//   - A1d-L4-02: flag acceso, solo l'amministratore (operatore e consultazione 403);
//   - A1d-L4-03: ogni GET di motore-a non scrive niente (ogni tabella, la coda dei job, il testo SQL mandato);
//   - A1d-L4-04: ogni gesto del ponte, partito dalla pagina, risponde con un solo <div data-ma-esito> (contratto §3.1,
//     P-07 b, T-A1d-11) e scrive solo le sue tabelle;
//   - A1d-L4-05: l'anteprima dello STEP in sola lettura, senza SQLSTATE 25006, con la firma del modulo legacy (T-A1d-03);
//   - A1d-L4-06: lo stesso EsitoThread canonico di caricatore.Carica + valutazione.Calcola fatti dalla prova (T-A1d-01:
//     non l'impronta globale dell'esito);
//   - A1d-L4-07: R-B7-04, caratterizzazione (U-07; D-A1d-20);
//   - A1d-L4-08: il tempo scaduto (R43 B, T-A1d-15).
//
// NON ESEGUIBILI QUI → PC originale (COCKPIT_TEST_DSN su cockpit_test):
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -count=1 -run 'TestMotoreAL4' ./internal/transport/web/
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è pubblico.
// La stringa della tabella dell'agente non c'è nemmeno qui: dove serve si compone (MOTORE-SENZA-LLM; RAC-21).

package web

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/core/fotorfq/caricatore"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
	"promatec/cockpit/internal/platform/testutil"
)

// ------------------------------------------------------------------ le regole ACME dell'avvio

// regoleACMEPerLaScena: l'anteprima accesa con le regole ACME di A1d-Q1 (app/runtime/testdata/motore_a), riscritte per il
// cliente della scena, che nasce dal database con un UUID suo (piano A1d backend §9.2). La grammatica prende l'UUID e la
// ragione sociale della scena (ACME S.p.A. e Acme S.p.A. coincidono comunque a meno delle maiuscole ASCII,
// grammatica.ControllaRagioneSociale); l'indice, letto con LeggiIndice, prende la voce di quel cliente con lo sha del
// contenuto nuovo, nessun file dei casi (ingressi vuoti) e tempo_massimo_ms = limiteMs. Si compila con
// motorea.CompilaInsieme come all'avvio (app/runtime/motore_a.go), senza importare app/runtime (T-A1d-34). I fine riga
// CRLF di un checkout Windows diventano LF (T-A1d-33).
func regoleACMEPerLaScena(t *testing.T, cliente uuid.UUID, ragione string, limiteMs int) AnteprimaMotoreA {
	t.Helper()
	leggi := func(nome string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", "app", "runtime", "testdata", "motore_a", nome))
		if err != nil {
			t.Fatalf("regole ACME: %v", err)
		}
		return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	}
	ind, err := grammatica.LeggiIndice(leggi("indice_acme.v1.json"))
	if err != nil {
		t.Fatalf("regole ACME: l'indice non si legge: %v", err)
	}
	var g map[string]any
	dec := json.NewDecoder(bytes.NewReader(leggi("acme.v1.json")))
	dec.UseNumber()
	if err := dec.Decode(&g); err != nil {
		t.Fatalf("regole ACME: la grammatica non si legge: %v", err)
	}
	cl, ok := g["cliente"].(map[string]any)
	if !ok {
		t.Fatal("regole ACME: la grammatica non ha il cliente")
	}
	cl["id"], cl["ragione_sociale"] = cliente.String(), ragione
	gram, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	const file = "acme.v1.json"
	ind.Grammatiche = []grammatica.VoceIndice{{ClienteID: cliente, File: file, Sha256: jsoncanonico.Impronta(gram)}}
	ind.Casi = nil
	ind.Limiti.Anteprima.TempoMassimoMs = limiteMs
	ins, diag, err := motorea.CompilaInsieme(ind, map[string][]byte{file: gram})
	if err != nil {
		t.Fatalf("regole ACME: l'indice non attiva nessuna grammatica: %v", err)
	}
	if ins.Motori[cliente] == nil {
		t.Fatalf("regole ACME: la grammatica del cliente della scena è scartata: %v (diagnostiche %v)", ins.Scartati[cliente], diag)
	}
	return AnteprimaMotoreA{Accesa: true, Regole: &ins, Limite: time.Duration(limiteMs) * time.Millisecond}
}

// maAccendi mette a sul server della prova (il flag si legge a ogni richiesta: contratto A1d §2.1) e lo rispegne alla fine.
func maAccendi(t *testing.T, b *bancoWeb, a AnteprimaMotoreA) {
	t.Helper()
	b.ws.MotoreA = a
	t.Cleanup(func() { b.ws.MotoreA = AnteprimaMotoreA{} })
}

// maPagina: l'indirizzo della Distinta nuova di una RFQ.
func maPagina(thread uuid.UUID) string { return "/thread/" + thread.String() + "/motore-a" }

// ------------------------------------------------------------------ la scena

// maScenaL4: la scena delle L4 di A1d (piano A1d backend §9.2; RAC-20). È scenaSenzaGet (nessunaget_db_test.go: il
// cliente «Acme S.p.A.», il codice 7120001 della richiesta confermato, gli STEP letto, vecchio e fermo, il PDF fermo, il
// componente 7120099 con il suo 2D sul NAS, una proposta aperta) con in più:
//   - le righe di proposta dello STEP letto (7120001A_1.stp): i nodi #1 e #2 e una relazione_proposta APERTA #1 → #2 ×2;
//   - una proposta di documento aperta, 7120099_1.pdf con il codice 7120099 (lo «Scarta» di A1d-L4-04 e della prova nel
//     browser);
//   - il finito 7120050 (un secondo prodotto: i finiti manuali sono target, valutazione/target.go) con 7120099 sotto ×1;
//   - un DXF di 7120099 confermato e con la copia sul NAS in errore (per «Riprova le copie»);
//   - un particolare 7120030 senza file (la deroga del 2D);
//   - i tre browser: amministratore (AD, il solo che vede la pagina), operatore (FP), consultazione (CO).
type maScenaL4 struct {
	*scenaSenzaGet
	cliente  uuid.UUID
	proposta uuid.UUID // la proposta aperta di 7120099_1.pdf
	finito   uuid.UUID // il componente 7120050
	sciolto  uuid.UUID // il componente 7120030, senza file
	dxf      uuid.UUID // il documento DXF di 7120099, con la copia in errore
	ad       *browser
	op       *browser
	co       *browser
}

func (b *bancoWeb) maScenaL4(t *testing.T, chiave string) *maScenaL4 {
	t.Helper()
	s := &maScenaL4{scenaSenzaGet: b.scenaSenzaGet(t, chiave)}
	s.cliente = uuidSQL(t, b, `SELECT cliente_id FROM thread_offerta WHERE thread_id = $1`, s.thread)
	sha := valoreR(s.rfqFascicolo, `SELECT sha256 FROM allegato WHERE allegato_id = $1`, s.letto)
	s.esegui(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, id_grezzo, codice, famiglia, fonte, confidenza)
		VALUES ($1, $2, $3, '#1', '', '7120001A', '7120001', 'acme', 'step', 60), ($1, $2, $3, '#2', '', '7120010', '7120010', 'acme', 'step', 60)`,
		s.thread, s.letto, sha)
	s.esegui(`INSERT INTO relazione_proposta (thread_id, allegato_id, padre_chiave, figlio_chiave, qta) VALUES ($1, $2, '#1', '#2', 2)`, s.thread, s.letto)
	s.proposta, _ = s.propostaDa("7120099_1.pdf", "7120099")
	s.finito = s.componenteTipo("7120050", "finito")
	s.arco(s.finito, s.pezzo, 1)
	s.dxf, _ = s.documentoDa("7120099.dxf", "dxf", db.TipoDocumentoSviluppoDxf, "7120099", s.pezzo, db.StatoNasErrore)
	s.sciolto = s.componenteTipo("7120030", "sciolto")
	s.ad = b.browser("10.0.0.8")
	s.ad.login("AD", "prova-ad")
	s.op = operatore(b)
	s.co = b.browser("10.0.0.9")
	s.co.login("CO", "prova-co")
	return s
}

// maRegole: l'anteprima accesa per il cliente della scena, con il limite dell'indice ACME (20 s) o quello dato.
func (s *maScenaL4) maRegole(t *testing.T, limiteMs int) AnteprimaMotoreA {
	t.Helper()
	return regoleACMEPerLaScena(t, s.cliente, "Acme S.p.A.", limiteMs)
}

// ------------------------------------------------------------------ le richieste

// daMotoreA è una richiesta htmx partita dalla Distinta nuova, come la manda il ponte (motore_a_gesti.mjs): HX-Request e
// HX-Current-URL = l'indirizzo della pagina (corrente: il percorso, con la sua query). Contratto A1d §3.1.
func (w *browser) daMotoreA(metodo, percorso string, form url.Values, corrente string) (*http.Response, string) {
	w.b.t.Helper()
	var corpo io.Reader
	if form != nil {
		corpo = strings.NewReader(form.Encode())
	}
	r, _ := http.NewRequest(metodo, w.b.srv.URL+percorso, corpo)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	r.Header.Set("HX-Request", "true")
	r.Header.Set("HX-Current-URL", w.b.srv.URL+corrente)
	r.Header.Set("X-Prova-IP", w.ip)
	resp, err := w.c.Do(r)
	if err != nil {
		w.b.t.Fatal(err)
	}
	defer resp.Body.Close()
	testo, _ := io.ReadAll(resp.Body)
	return resp, string(testo)
}

// reEsitoMotoreA: la sola risposta che un gesto partito dalla pagina può dare con 200 (piano A1d backend §5.1.8; A1d-L4-04).
var reEsitoMotoreA = regexp.MustCompile(`^<div data-ma-esito="(ok|no)">([^<]*)</div>$`)

// maEsito legge l'esito di un gesto partito dalla pagina: 200, text/html, no-store, e un solo <div data-ma-esito>. Il testo
// è quello leggibile (senza l'escape di html/template).
func maEsito(t *testing.T, contesto string, resp *http.Response, corpo string) (esito, testo string) {
	t.Helper()
	m := reEsitoMotoreA.FindStringSubmatch(corpo)
	if resp.StatusCode != http.StatusOK || m == nil {
		t.Fatalf("%s: %d, il corpo non è un solo <div data-ma-esito>: %.400q", contesto, resp.StatusCode, corpo)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("%s: Content-Type %q", contesto, ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("%s: Cache-Control %q, atteso no-store", contesto, cc)
	}
	return m[1], leggibile(m[2])
}

// maDatiL4: la parte di GET /thread/{id}/motore-a/dati (contratto A1d §2.4) che le prove leggono. Le forme sono le più
// piccole che servono: il JSON intero lo confronta A1d-L4-06, come byte canonici.
type maDatiL4 struct {
	ThreadID string `json:"thread_id"`
	Vista    string `json:"vista"`
	Prodotto string `json:"prodotto"`
	Errore   string `json:"errore"`
	Testa    struct {
		Cliente     string `json:"cliente"`
		StatoThread string `json:"stato_thread"`
	} `json:"testa"`
	Esito *struct {
		Valutato         bool           `json:"valutato"`
		Motivo           string         `json:"motivo"`
		ProdottiValutati []maProdottoL4 `json:"prodotti_valutati"`
	} `json:"esito"`
}

// maProdottoL4: di un prodotto valutato, il riferimento, il componente e la verifica della BOM (gli assi 3 e 4).
type maProdottoL4 struct {
	Rif             string     `json:"rif"`
	ComponenteID    *uuid.UUID `json:"componente_id"`
	CodiceRichiesto string     `json:"codice_richiesto"`
	BOM             struct {
		Verificata   bool `json:"verificata"`
		Nomenclatura struct {
			Stato      string `json:"stato"`
			DaDecidere int    `json:"da_decidere"`
		} `json:"nomenclatura"`
		Gerarchia struct {
			Stato      string `json:"stato"`
			DaDecidere int    `json:"da_decidere"`
		} `json:"gerarchia"`
	} `json:"bom"`
}

// maDati: GET …/motore-a/dati dall'amministratore, con il JSON letto; il corpo intero per i messaggi.
func maDati(t *testing.T, w *browser, thread uuid.UUID) (maDatiL4, string) {
	t.Helper()
	resp, corpo := w.fai(http.MethodGet, maPagina(thread)+"/dati", nil, false)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("dati: %d %q: %.300s", resp.StatusCode, resp.Header.Get("Content-Type"), corpo)
	}
	var d maDatiL4
	if err := json.Unmarshal([]byte(corpo), &d); err != nil {
		t.Fatalf("dati: %v: %.300s", err, corpo)
	}
	return d, corpo
}

// maUltimiAccessi: l'ultimo accesso di ogni sessione aperta, per token (R36: una rotta spenta non tocca la sessione).
func maUltimiAccessi(t *testing.T, b *bancoWeb) map[string]string {
	t.Helper()
	righe, err := b.pool.Query(b.ctx, `SELECT token, ultimo_accesso::text FROM sessione`)
	if err != nil {
		t.Fatal(err)
	}
	defer righe.Close()
	out := map[string]string{}
	for righe.Next() {
		var k, v string
		if err := righe.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		out[k] = v
	}
	return out
}

// maRotteDalPonte: le rotte di scrittura del ponte, fra i marcatori «// ROTTE:inizio» e «// ROTTE:fine» di
// web/static/motore_a_gesti.mjs, come le scrive il ponte ({id}, {u}): una fonte sola per la lista (RAC-13).
func maRotteDalPonte(t *testing.T) []string {
	t.Helper()
	b, err := fs.ReadFile(risorse.FS, "web/static/motore_a_gesti.mjs")
	if err != nil {
		t.Fatalf("il ponte non c'è: %v", err)
	}
	testo := string(b)
	i, j := strings.Index(testo, "// ROTTE:inizio"), strings.Index(testo, "// ROTTE:fine")
	if i < 0 || j < i {
		t.Fatal("il ponte non ha i marcatori ROTTE:inizio e ROTTE:fine")
	}
	var out []string
	for _, m := range regexp.MustCompile(`"(/[^"]+)"`).FindAllStringSubmatch(testo[i:j], -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

// ------------------------------------------------------------------ A1d-L4-01

// A1d-L4-01 (contratto A1d §2.1; piano A1d backend §5.1.3; R36): a flag spento la pagina, il frammento, i dati e
// l'anteprima dello STEP rispondono 404 a tutti — anonimo, consultazione, operatore, amministratore — anche con le regole
// caricate, con lo stesso corpo di un indirizzo che non esiste; nessuna sessione è toccata (la porta sta prima di
// autenticato, quindi niente ToccaSessione). Un gesto partito con l'indirizzo della pagina riceve la risposta di sempre:
// il ramo di threadFrammento c'è solo a flag acceso (T-A1d-07).
func TestMotoreAL4SpentoRisponde404ATutti(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.maScenaL4(t, "MA401")
	a := s.maRegole(t, 20000)
	a.Accesa = false
	maAccendi(t, b, a)
	anonimo := b.browser("10.0.0.7")
	pagina := maPagina(s.thread)
	_, ignoto := anonimo.fai(http.MethodGet, pagina+"-che-non-esiste", nil, false)

	prima := fotoDelDatabase(t, b)
	accessi := maUltimiAccessi(t, b)
	for _, chi := range []struct {
		nome string
		w    *browser
	}{{"anonimo", anonimo}, {"consultazione", s.co}, {"operatore", s.op}, {"amministratore", s.ad}} {
		for _, r := range []struct {
			metodo, percorso string
			hx               bool
		}{
			{http.MethodGet, pagina, false},
			{http.MethodGet, pagina + "?vista=documenti", false},
			{http.MethodGet, pagina, true},
			{http.MethodHead, pagina, false},
			{http.MethodGet, pagina + "/dati", false},
			{http.MethodHead, pagina + "/dati", false},
			{http.MethodGet, pagina + "/anteprima/step-strutturale?componente=" + s.pezzo.String(), false},
		} {
			resp, corpo := chi.w.fai(r.metodo, r.percorso, nil, r.hx)
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("%s, %s %s (htmx %v): %d, atteso 404", chi.nome, r.metodo, r.percorso, r.hx, resp.StatusCode)
			}
			if r.metodo == http.MethodGet && corpo != ignoto {
				t.Errorf("%s, %s: il 404 si distingue da un indirizzo che non esiste: %q contro %q", chi.nome, r.percorso, corpo, ignoto)
			}
		}
	}
	if dopo := maUltimiAccessi(t, b); !maMappeUguali(accessi, dopo) {
		t.Errorf("a flag spento una richiesta ha toccato una sessione (R36): %v → %v", accessi, dopo)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("a flag spento le richieste hanno scritto in: %s", d)
	}

	// il gesto con l'indirizzo della pagina: la risposta di sempre (il corpo della RFQ), mai il <div data-ma-esito>
	resp, html := s.ad.daMotoreA(http.MethodPost, "/thread/"+s.thread.String()+"/riprova-copie", url.Values{}, pagina)
	if resp.StatusCode != http.StatusOK || strings.Contains(html, "data-ma-esito") {
		t.Errorf("a flag spento il gesto ha preso il ramo della pagina nuova: %d %.300q", resp.StatusCode, html)
	}
	if !strings.Contains(strings.ToLower(leggibile(html)), "copi") {
		t.Errorf("a flag spento il gesto non ha la risposta di sempre: %.300q", html)
	}
}

// maMappeUguali: due mappe di stringhe uguali.
func maMappeUguali(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// ------------------------------------------------------------------ A1d-L4-02

// A1d-L4-02 (contratto A1d §2.1-§2.4; piano A1d backend §5.1.3, §5.1.7, §8): a flag acceso l'amministratore apre la
// pagina (200, no-store, #ma-avviso e #ma), il frammento con HX-Request (niente <html>, niente #ma-avviso: solo
// motore_a_corpo), i dati (le chiavi di §2.4, mai Scrive né Indice; il calcolo riuscito) e l'anteprima dello STEP (200
// JSON); HEAD dà le intestazioni. Operatore e consultazione ricevono 403 su tutte; l'anonimo va al login. Con le regole
// non caricate la pagina c'è, con l'errore generico dell'avvio (ErroreRegoleMotoreA) e senza esito.
func TestMotoreAL4AccesoSoloLAmministratore(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.maScenaL4(t, "MA402")
	maAccendi(t, b, s.maRegole(t, 20000))
	pagina := maPagina(s.thread)
	step := pagina + "/anteprima/step-strutturale?componente=" + s.pezzo.String()

	resp, html := s.ad.fai(http.MethodGet, pagina, nil, false)
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-store" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("la pagina: %d %q %q", resp.StatusCode, resp.Header.Get("Cache-Control"), resp.Header.Get("Content-Type"))
	}
	for _, c := range []string{`id="ma-avviso"`, `id="ma"`, `data-ma-thread="` + s.thread.String() + `"`, `type="importmap"`} {
		if !strings.Contains(html, c) {
			t.Errorf("la pagina: manca %s", c)
		}
	}
	resp, html = s.ad.fai(http.MethodGet, pagina, nil, true)
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-store" || strings.Contains(html, "<html") ||
		strings.Contains(html, `id="ma-avviso"`) || !strings.Contains(html, `data-ma-thread="`+s.thread.String()+`"`) {
		t.Errorf("il frammento: %d %q, %.300q", resp.StatusCode, resp.Header.Get("Cache-Control"), html)
	}
	for _, p := range []string{pagina, pagina + "/dati"} {
		resp, corpo := s.ad.fai(http.MethodHead, p, nil, false)
		if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-store" || corpo != "" {
			t.Errorf("HEAD %s: %d %q %q", p, resp.StatusCode, resp.Header.Get("Cache-Control"), corpo)
		}
	}

	d, corpo := maDati(t, s.ad, s.thread)
	var chiavi map[string]json.RawMessage
	if err := json.Unmarshal([]byte(corpo), &chiavi); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"thread_id", "vista", "prodotto", "testa", "esito", "confronto", "errore", "calcolato"} {
		if _, ok := chiavi[k]; !ok {
			t.Errorf("dati: manca la chiave %q (contratto A1d §2.4)", k)
		}
	}
	for k := range chiavi {
		if l := strings.ToLower(k); l == "scrive" || l == "indice" {
			t.Errorf("dati: c'è la chiave %q, che non è del contratto", k)
		}
	}
	if d.Errore != "" || d.Esito == nil || d.ThreadID != s.thread.String() || d.Vista != "distinta" || d.Testa.Cliente != "Acme S.p.A." ||
		d.Testa.StatoThread != "APERTA" {
		t.Errorf("dati: errore %q, esito %v, thread %s, vista %q, testa %+v", d.Errore, d.Esito != nil, d.ThreadID, d.Vista, d.Testa)
	}
	resp, corpo = s.ad.fai(http.MethodGet, step, nil, false)
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") || resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("anteprima dello STEP: %d %q %q: %.300s", resp.StatusCode, resp.Header.Get("Content-Type"), resp.Header.Get("Cache-Control"), corpo)
	}

	for _, chi := range []struct {
		nome string
		w    *browser
	}{{"operatore", s.op}, {"consultazione", s.co}} {
		for _, r := range []struct {
			percorso string
			hx       bool
		}{{pagina, false}, {pagina, true}, {pagina + "?vista=rfq", false}, {pagina + "/dati", false}, {step, false}} {
			if resp, _ := chi.w.fai(http.MethodGet, r.percorso, nil, r.hx); resp.StatusCode != http.StatusForbidden {
				t.Errorf("%s, GET %s (htmx %v): %d, atteso 403", chi.nome, r.percorso, r.hx, resp.StatusCode)
			}
		}
	}
	if resp, _ := b.browser("10.0.0.7").fai(http.MethodGet, pagina, nil, false); resp.StatusCode != http.StatusFound ||
		!strings.HasPrefix(resp.Header.Get("Location"), "/login") {
		t.Errorf("anonimo a flag acceso: %d → %q, atteso il login", resp.StatusCode, resp.Header.Get("Location"))
	}

	// le regole non caricate (indice rotto all'avvio): la pagina c'è, con il testo generico, e nessun esito
	maAccendi(t, b, AnteprimaMotoreA{Accesa: true, Errore: ErroreRegoleMotoreA})
	d, _ = maDati(t, s.ad, s.thread)
	if d.Errore != ErroreRegoleMotoreA || d.Esito != nil {
		t.Errorf("regole non caricate: errore %q, esito %v", d.Errore, d.Esito != nil)
	}
	if resp, html := s.ad.fai(http.MethodGet, pagina, nil, false); resp.StatusCode != 200 || !strings.Contains(leggibile(html), ErroreRegoleMotoreA) {
		t.Errorf("regole non caricate: la pagina %d non dice l'errore", resp.StatusCode)
	}
}

// ------------------------------------------------------------------ A1d-L4-03

// reSQLCheScrive: il testo SQL che scrive o prende un lucchetto (le parole di testutil.RegistroSQL.Vietati, senza le
// tabelle: la sessione e le credenziali dei worker le legge il layout di ogni pagina).
var reSQLCheScrive = regexp.MustCompile(`(?i)\b(insert|update|delete|truncate|copy)\b|(\b|_)lock\b|\bfor\s+(no\s+key\s+update|key\s+share|share|update)\b`)

// A1d-L4-03 (contratto A1d §2.1, §5; DB7-6; piano A1d backend §9.2): ogni GET di motore-a — la pagina in ogni vista, con
// un prodotto che non c'è, il frammento, HEAD, i dati, l'anteprima dello STEP — non scrive niente, per l'amministratore che
// la vede e per chi riceve 403: ogni tabella dello schema ha le stesse righe con lo stesso contenuto, la coda dei job non
// cambia (nessun prepara, nessun sync), e nel testo SQL mandato al database non c'è una scrittura né un lucchetto, salvo
// l'ultimo accesso della sessione di chi chiede (ToccaSessione, di ogni rotta autenticata). Il calcolo non legge mai la
// tabella dell'agente (MOTORE-SENZA-LLM). Le stesse GET stanno anche in TestNessunaGetScrive (nessunaget_db_test.go).
func TestMotoreAL4LeGetNonScrivono(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.maScenaL4(t, "MA403")
	maAccendi(t, b, s.maRegole(t, 20000))
	pagina := maPagina(s.thread)
	pr, reg := testutil.PoolConRegistro(t, testutil.DSN(t))
	pool := b.ws.Pool
	b.ws.Pool = pr
	t.Cleanup(func() { b.ws.Pool = pool })

	get := []struct {
		metodo, percorso string
		hx               bool
	}{
		{http.MethodGet, pagina, false},
		{http.MethodGet, pagina + "?vista=distinta", false},
		{http.MethodGet, pagina + "?vista=documenti", false},
		{http.MethodGet, pagina + "?vista=rfq", false},
		{http.MethodGet, pagina + "?vista=boh", false},
		{http.MethodGet, pagina + "?prodotto=boh", false},
		{http.MethodGet, pagina + "?vista=documenti&prodotto=componente:" + s.finito.String(), false},
		{http.MethodGet, pagina, true},
		{http.MethodGet, pagina + "?vista=rfq", true},
		{http.MethodHead, pagina, false},
		{http.MethodGet, pagina + "/dati", false},
		{http.MethodGet, pagina + "/dati?vista=documenti", false},
		{http.MethodHead, pagina + "/dati", false},
		{http.MethodGet, pagina + "/anteprima/step-strutturale?componente=" + s.pezzo.String(), false},
		{http.MethodGet, pagina + "/anteprima/step-strutturale?componente=" + s.pezzo.String() + "&nodo=" + uuid.NewString(), false},
		{http.MethodGet, pagina + "/anteprima/step-strutturale?componente=non-un-id", false},
	}
	job := contaSQL(t, b, `SELECT count(*) FROM job`)
	prima := fotoDelDatabase(t, b)
	reg.Azzera()
	for _, chi := range []struct {
		nome  string
		w     *browser
		admin bool
	}{{"amministratore", s.ad, true}, {"operatore", s.op, false}, {"consultazione", s.co, false}} {
		for _, g := range get {
			resp, _ := chi.w.fai(g.metodo, g.percorso, nil, g.hx)
			atteso := http.StatusOK
			if !chi.admin {
				atteso = http.StatusForbidden
			}
			if resp.StatusCode != atteso {
				t.Errorf("%s, %s %s (htmx %v): %d, atteso %d", chi.nome, g.metodo, g.percorso, g.hx, resp.StatusCode, atteso)
			}
		}
		if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
			t.Errorf("le GET di motore-a dell'%s hanno scritto in: %s", chi.nome, d)
		}
	}
	b.ws.Pool = pool
	if n := contaSQL(t, b, `SELECT count(*) FROM job`); n != job {
		t.Errorf("la coda dei job è cambiata: %d → %d (prepara o sync all'apertura? DB7-6)", job, n)
	}
	agente := "analisi" + "_messaggio"
	var vietati []string
	for _, v := range reg.Voci() {
		sql := v.SQL
		// la sessione di chi chiede: l'ultimo accesso di ogni rotta autenticata, e la postazione riabbinata per IP
		if strings.Contains(sql, "-- name: ToccaSessione ") || strings.Contains(sql, "-- name: SetSessionePostazione ") {
			continue
		}
		if reSQLCheScrive.MatchString(sql) || strings.Contains(strings.ToLower(sql), agente) {
			vietati = append(vietati, sql)
		}
	}
	if len(vietati) > 0 {
		t.Errorf("le GET di motore-a hanno mandato SQL che scrive, blocca o legge la tabella dell'agente:\n%s", strings.Join(vietati, "\n---\n"))
	}
	if len(reg.Voci()) == 0 {
		t.Error("il registro SQL è vuoto: le GET non sono passate dal pool registrato e la prova non prova niente")
	}
}

// ------------------------------------------------------------------ A1d-L4-04

// A1d-L4-04 (contratto A1d §3.1, §3.4; v1.1 §4.10; P-07 b; T-A1d-07, T-A1d-11; piano A1d backend §6): ogni gesto della
// lista del ponte, partito dalla pagina (HX-Request e HX-Current-URL della pagina), risponde con 200 e un solo
// <div data-ma-esito="ok|no">testo</div>, oppure con un errore 4xx/5xx testuale; mai la vecchia interfaccia. Prima i casi
// con l'esito noto, ciascuno con le tabelle che può toccare:
//   - «Riprova le copie» con [sicurezza].nas_scrittura spenta → «no», «Copie NON rimesse in coda», nessuna tabella (l'elenco
//     chiuso; e l'indirizzo della pagina scritto con le maiuscole è la stessa pagina, T-A1d-07);
//   - «Scarta» di una proposta di documento (con ritorna_thread, come lo manda il ponte) → «ok», solo documento_proposta;
//     ripetuto → «no» («La proposta era già stata decisa»), nessuna tabella;
//   - «Associa» senza file → «no» («Nessun file scelto»), nessuna tabella;
//   - deroga del 2D e la sua revoca → «ok» e «ok», solo deroga_fabbisogno;
//   - «Rifai l'analisi» con l'analisi non configurata → «no», nessuna tabella;
//   - «Rifai lo smistamento» su una RFQ CHIUSA → «no» (l'elenco chiuso), nessuna tabella.
//
// Poi tutte le 29 rotte, lette dal ponte (RAC-13), con campi minimi: solo la forma della risposta.
func TestMotoreAL4OgniGestoRispondeConLEsito(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.maScenaL4(t, "MA404")
	maAccendi(t, b, s.maRegole(t, 20000))
	pagina := maPagina(s.thread)
	th := "/thread/" + s.thread.String()
	ad := s.ad
	// gesto manda un POST dalla pagina e controlla l'esito, la frase e le tabelle: scrive sono le tabelle del gesto (tutte
	// devono cambiare); con flusso il gesto passa da dopoIlGesto o dopoLaProposta, e il flusso dello smistamento che parte
	// dopo ogni risposta 2xx può riscrivere le destinazioni delle proposte aperte (documento_proposta, ScriviDestinazione):
	// quella tabella è del flusso, non del gesto, ed è ammessa anche dopo un «no».
	gesto := func(contesto, percorso string, form url.Values, corrente, esito, frase string, scrive []string, flusso bool) {
		t.Helper()
		prima := fotoDelDatabase(t, b)
		if form == nil {
			form = url.Values{}
		}
		resp, corpo := ad.daMotoreA(http.MethodPost, percorso, form, corrente)
		e, testo := maEsito(t, contesto, resp, corpo)
		if e != esito || !strings.Contains(testo, frase) {
			t.Errorf("%s: %q «%s», atteso %q con «%s»", contesto, e, testo, esito, frase)
		}
		ammesse := slices.Clone(scrive)
		if flusso {
			ammesse = append(ammesse, "documento_proposta")
		}
		var cambiate, fuori []string
		for _, tab := range strings.Split(differenze(prima, fotoDelDatabase(t, b)), ", ") {
			if tab == "" {
				continue
			}
			cambiate = append(cambiate, tab)
			if !slices.Contains(ammesse, tab) {
				fuori = append(fuori, tab)
			}
		}
		if len(fuori) > 0 {
			t.Errorf("%s: il gesto ha scritto fuori dalle sue tabelle %v: %v", contesto, ammesse, fuori)
		}
		for _, tab := range scrive {
			if !slices.Contains(cambiate, tab) {
				t.Errorf("%s: il gesto riuscito non ha scritto in %s (cambiate: %v)", contesto, tab, cambiate)
			}
		}
	}

	ImpostaCapacitaProva(t, coda.Capacita{OutlookScrittura: true, Bozze: true, NasScrittura: false})
	gesto("riprova-copie, NAS spento", th+"/riprova-copie", nil, pagina, "no", "Copie NON rimesse in coda", nil, false)
	gesto("riprova-copie, indirizzo maiuscolo", th+"/riprova-copie", nil, "/thread/"+strings.ToUpper(s.thread.String())+"/motore-a?vista=documenti",
		"no", "Copie NON rimesse in coda", nil, false)
	ImpostaCapacitaProva(t, tutteAccese)

	scarta := "/proposta/" + s.proposta.String() + "/scarta"
	ritorna := url.Values{"ritorna_thread": {s.thread.String()}}
	gesto("scarta la proposta", scarta, ritorna, pagina, "ok", "Proposta scartata.", []string{"documento_proposta"}, true)
	gesto("scarta la proposta, di nuovo", scarta, ritorna, pagina, "no", "La proposta era già stata decisa", nil, true)
	gesto("associa senza file", th+"/fascicolo/assegna", url.Values{"componente": {s.pezzo.String()}}, pagina, "no", "Nessun file scelto", nil, true)
	gesto("deroga del 2D", th+"/fascicolo/componente/"+s.sciolto.String()+"/deroga",
		url.Values{"tipo": {"disegno_2d"}, "motivo": {"lo disegniamo noi"}}, pagina, "ok", "derogato", []string{"deroga_fabbisogno"}, false)
	deroga := valoreR(s.rfqFascicolo, `SELECT deroga_id::text FROM deroga_fabbisogno WHERE componente_id = $1`, s.sciolto)
	gesto("revoca della deroga", th+"/fascicolo/deroga/"+deroga+"/revoca", nil, pagina, "ok", "Deroga revocata", []string{"deroga_fabbisogno"}, false)

	an := b.ws.Analizzatore
	b.ws.Analizzatore = coda.Analizzatore{}
	gesto("rianalizza senza analisi", th+"/fascicolo/rianalizza", nil, pagina, "no", "l'analisi non è configurata", nil, true)
	b.ws.Analizzatore = an

	s.esegui(`UPDATE thread_offerta SET stato = 'CHIUSA' WHERE thread_id = $1`, s.thread)
	gesto("aggiorna lo smistamento di una RFQ chiusa", th+"/smistamento/aggiorna", nil, pagina, "no", "non si aggiornano", nil, false)
	s.esegui(`UPDATE thread_offerta SET stato = 'APERTA' WHERE thread_id = $1`, s.thread)

	// le 29 rotte del ponte: campi minimi, UUID che non esistono dove serve un id; solo la forma della risposta
	u := uuid.NewString()
	campi := map[string]url.Values{
		"/thread/{id}/fascicolo/componente/{u}/step-strutturale":        {"documento": {u}, "firma": {"x"}, "autorizza": {"1"}},
		"/thread/{id}/fascicolo/componente/{u}/step-strutturale/revoca": {"sha": {strings.Repeat("0", 64)}},
		"/thread/{id}/fascicolo/nodo/{u}/accetta":                       {"tipo": {""}},
		"/thread/{id}/fascicolo/nodo/{u}/scarta":                        {},
		"/thread/{id}/fascicolo/nodo/{u}/codice":                        {"codice": {"7129999"}, "rev": {""}},
		"/thread/{id}/fascicolo/nodo/{u}/riapri":                        {},
		"/thread/{id}/fascicolo/componente/{u}/codice":                  {"codice": {"7129999"}},
		"/thread/{id}/fascicolo/componente/{u}/modifica":                {"rev": {""}, "descrizione": {""}},
		"/thread/{id}/fascicolo/componente/{u}/tipo":                    {"tipo": {"sciolto"}, "firma": {"x"}},
		"/thread/{id}/fascicolo/componente/{u}/collega":                 {"padre": {u}, "qta": {"1"}},
		"/thread/{id}/fascicolo/componente/{u}/sposta":                  {"da": {""}, "a": {""}, "qta": {"1"}},
		"/thread/{id}/fascicolo/componente/{u}/scollega":                {"padre": {u}},
		"/thread/{id}/fascicolo/componente/{u}/archivia":                {"motivo": {"prova"}},
		"/thread/{id}/fascicolo/componente/{u}/rimuovi":                 {},
		"/thread/{id}/fascicolo/componente/{u}/ripristina":              {},
		"/thread/{id}/fascicolo/relazione/accetta":                      {"allegato": {u}, "padre": {"#1"}, "figlio": {"#2"}},
		"/thread/{id}/fascicolo/relazione/scarta":                       {"allegato": {u}, "padre": {"#1"}, "figlio": {"#2"}},
		"/thread/{id}/fascicolo/rimozione/accetta":                      {"step": {u}, "padre": {u}, "figlio": {u}},
		"/thread/{id}/fascicolo/rimozione/scarta":                       {"step": {u}, "padre": {u}, "figlio": {u}},
		"/thread/{id}/fascicolo/conferma":                               {"voce": {u}},
		"/proposta/{u}/scarta":                                          {"ritorna_thread": {s.thread.String()}},
		"/thread/{id}/fascicolo/assegna":                                {"proposta": {u}, "componente": {""}},
		"/thread/{id}/fascicolo/documento/{u}/sostituisci":              {"vecchio": {u}, "motivo": {"prova"}},
		"/thread/{id}/fascicolo/componente/{u}/deroga":                  {"tipo": {"disegno_2d"}, "motivo": {"prova"}},
		"/thread/{id}/fascicolo/deroga/{u}/revoca":                      {},
		"/thread/{id}/riprova-copie":                                    {},
		"/thread/{id}/fascicolo/prepara":                                {},
		"/thread/{id}/fascicolo/rianalizza":                             {},
		"/thread/{id}/smistamento/aggiorna":                             {},
	}
	ponte := maRotteDalPonte(t)
	var nostre []string
	for k := range campi {
		nostre = append(nostre, k)
	}
	sort.Strings(nostre)
	if !slices.Equal(ponte, nostre) || len(ponte) != 29 {
		t.Fatalf("le rotte del ponte non sono quelle della prova (contratto A1d §3.4; piano A1d backend §5.7.2):\nponte %v\nprova %v", ponte, nostre)
	}
	for _, r := range ponte {
		percorso := strings.NewReplacer("{id}", s.thread.String(), "{u}", u).Replace(r)
		resp, corpo := ad.daMotoreA(http.MethodPost, percorso, campi[r], pagina)
		switch {
		case resp.StatusCode == http.StatusOK:
			if !reEsitoMotoreA.MatchString(corpo) {
				t.Errorf("POST %s: 200 senza un solo <div data-ma-esito>: %.300q", r, corpo)
			}
		case resp.StatusCode >= 400:
			if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") || strings.Contains(corpo, "<") {
				t.Errorf("POST %s: %d non testuale (%q): %.300q", r, resp.StatusCode, resp.Header.Get("Content-Type"), corpo)
			}
		default:
			t.Errorf("POST %s: %d, atteso 200 con l'esito o un errore 4xx/5xx", r, resp.StatusCode)
		}
	}
}

// ------------------------------------------------------------------ A1d-L4-05

// A1d-L4-05 (contratto A1d §3.4 A; v1.1 §4.8; T-A1d-03): l'anteprima dello STEP per un componente con il suo STEP
// confermato e l'analisi corrente risponde 200 in JSON senza errore, in una transazione READ ONLY: nessun SQLSTATE 25006
// nel log del server (una lettura che scrivesse o bloccasse lo direbbe lì, con il codice); la firma è quella che il
// modulo legacy della scheda del componente mette nel campo nascosto; nessuna tabella cambia.
func TestMotoreAL4AnteprimaStepInSolaLettura(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.maScenaL4(t, "MA405")
	maAccendi(t, b, s.maRegole(t, 20000))
	comp := s.componenteTipo("7120040", "sottoassieme")
	doc, _ := s.documentoDa("7120040.stp", "stp", db.TipoDocumentoCad3d, "7120040", comp, db.StatoNasScritto)
	s.analisiCorrente(s.shaDoc(doc), fattiDi("#1", []string{"#1=7120040", "#2=7120041", "#3=7120042"}, []string{"#1>#2*2", "#1>#3*1"}))
	var log bytes.Buffer
	vecchio := b.ws.Log
	b.ws.Log = slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	t.Cleanup(func() { b.ws.Log = vecchio })

	legacy := s.anteprimaAutorizzazione(s.ad, comp, url.Values{"documento": {doc.String()}})
	m := reFirmaAutorizzazione.FindStringSubmatch(legacy)
	if m == nil {
		t.Fatalf("il modulo legacy non ha la firma: %.400s", estrai(legacy, "Autorizzazione"))
	}
	prima := fotoDelDatabase(t, b)
	q := url.Values{"componente": {comp.String()}, "documento": {doc.String()}}
	resp, corpo := s.ad.fai(http.MethodGet, maPagina(s.thread)+"/anteprima/step-strutturale?"+q.Encode(), nil, false)
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("anteprima: %d %q: %.300s", resp.StatusCode, resp.Header.Get("Cache-Control"), corpo)
	}
	var a struct {
		Errore, Spento, Firma string
		Frasi, Figli          json.RawMessage
		Autorizzabile         bool
		Inviabile             bool
	}
	if err := json.Unmarshal([]byte(corpo), &a); err != nil {
		t.Fatalf("anteprima: %v: %.300s", err, corpo)
	}
	if a.Errore != "" || a.Spento != "" || a.Firma == "" || string(a.Frasi) == "null" || string(a.Figli) == "null" {
		t.Errorf("anteprima: errore %q, spento %q, firma %q, frasi %s, figli %s", a.Errore, a.Spento, a.Firma, a.Frasi, a.Figli)
	}
	if a.Firma != m[1] {
		t.Errorf("la firma dell'anteprima (%s) non è quella del modulo legacy (%s)", a.Firma, m[1])
	}
	if !a.Inviabile {
		t.Error("l'amministratore (almeno operatore) con lo STEP sorgente: l'anteprima doveva essere inviabile")
	}
	if strings.Contains(log.String(), "25006") {
		t.Errorf("l'anteprima ha provato a scrivere in sola lettura (SQLSTATE 25006):\n%s", log.String())
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("l'anteprima dello STEP ha scritto in: %s", d)
	}
}

// ------------------------------------------------------------------ A1d-L4-06

// maCanonico: i byte canonici di un JSON (jsoncanonico.Codifica di un RawMessage, che lo rilegge).
func maCanonico(t *testing.T, contesto string, raw []byte) []byte {
	t.Helper()
	c, err := jsoncanonico.Codifica(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("%s: il JSON non si canonicalizza: %v", contesto, err)
	}
	return c
}

// A1d-L4-06 (T-A1d-01; piano A1d backend §5.1.5; R42 B): l'esito della pagina è quello del banco. La prova legge la RFQ
// con caricatore.Carica (il solo thread, senza messaggi fuori RFQ) e la calcola con valutazione.Calcola e le stesse regole
// e gli stessi ingressi dell'anteprima; l'esito di /motore-a/dati e quello della prova hanno lo stesso JSON canonico. Non
// si confronta Esito.Impronta: la pagina calcola un thread solo, il banco tutti quelli chiesti, e l'impronta globale
// cambia con l'insieme (T-A1d-01).
func TestMotoreAL4LoStessoEsitoDelBanco(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.maScenaL4(t, "MA406")
	a := s.maRegole(t, 20000)
	maAccendi(t, b, a)

	resp, corpo := s.ad.fai(http.MethodGet, maPagina(s.thread)+"/dati", nil, false)
	if resp.StatusCode != 200 {
		t.Fatalf("dati: %d", resp.StatusCode)
	}
	var d struct {
		Errore string          `json:"errore"`
		Esito  json.RawMessage `json:"esito"`
	}
	if err := json.Unmarshal([]byte(corpo), &d); err != nil || d.Errore != "" || len(d.Esito) == 0 || string(d.Esito) == "null" {
		t.Fatalf("dati: %v, errore %q, esito %.200s", err, d.Errore, d.Esito)
	}

	f, err := caricatore.Carica(b.ctx, b.pool, caricatore.Richiesta{Thread: []uuid.UUID{s.thread}})
	if err != nil {
		t.Fatalf("Carica: %v", err)
	}
	e, err := valutazione.Calcola(f, a.Regole, a.Ingressi)
	if err != nil {
		t.Fatalf("Calcola: %v", err)
	}
	var et *valutazione.EsitoThread
	for i := range e.Thread {
		if e.Thread[i].ThreadID == s.thread {
			et = &e.Thread[i]
		}
	}
	if et == nil {
		t.Fatal("l'esito della prova non ha il thread")
	}
	if !et.Valutato {
		t.Errorf("la scena non è valutata (motivo %q): il confronto non prova abbastanza", et.Motivo)
	}
	nostro, err := json.Marshal(et)
	if err != nil {
		t.Fatal(err)
	}
	pg, banco := maCanonico(t, "pagina", d.Esito), maCanonico(t, "banco", nostro)
	if !bytes.Equal(pg, banco) {
		i := 0
		for i < len(pg) && i < len(banco) && pg[i] == banco[i] {
			i++
		}
		da := max(0, i-120)
		t.Errorf("l'esito della pagina non è quello del banco (primo byte diverso: %d):\npagina …%s…\nbanco  …%s…",
			i, pg[da:min(len(pg), i+120)], banco[da:min(len(banco), i+120)])
	}
}

// ------------------------------------------------------------------ A1d-L4-07

// maScenaRB704: una RFQ ACME con il prodotto <base>00 (il codice della richiesta confermato e il suo finito), il suo STEP
// autorizzato e con l'analisi corrente (<base>00 → <base>01 ×2, <base>00 → <base>02 ×1), e le righe dello STEP fatte
// nascere dalla rilettura, come nella scena della Distinta (distinta_browser_test.go). Le famiglie 712xxxx del cliente
// (cliente.regole, quelle del legacy: le regole del motore A non vengono da lì) fanno dei nodi dello STEP dei codici.
func (b *bancoWeb) maScenaRB704(t *testing.T, chiave, base string) (r *rfqFascicolo, prodotto, step uuid.UUID) {
	t.Helper()
	b.ws.Analizzatore = anScena
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	r = b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), chiave)
	r.esegui(`UPDATE cliente SET regole = $1 WHERE cliente_id = (SELECT cliente_id FROM thread_offerta WHERE thread_id = $2)`, famiglieDistintaDB, r.thread)
	r.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confermato_da) VALUES ($1, $2, 'proposta_famiglia', $3)`,
		r.thread, base+"00", r.utente)
	prodotto = r.componenteTipo(base+"00", "finito")
	var sha string
	step, sha = r.allegatoExt(base+"00.stp", "stp")
	r.analisiCorrente(sha, fattiDi("#1", []string{"#1=" + base + "00", "#2=" + base + "01", "#3=" + base + "02"}, []string{"#1>#2*2", "#1>#3*1"}))
	if _, err := fascicolo.RileggiStepDellaRfq(b.ctx, b.q, r.thread, anScena); err != nil {
		t.Fatal(err)
	}
	testutil.AutorizzaStep(t, b.pool, r.thread, prodotto, step, r.utente)
	if _, err := fascicolo.RileggiStepDellaRfq(b.ctx, b.q, r.thread, anScena); err != nil {
		t.Fatal(err)
	}
	return r, prodotto, step
}

// maConfermaAlberoVuoto: «Conferma l'albero» con la bozza vuota, come la pagina (contratto A1d §3.4 B; DB7-4, DB7-5): il
// riepilogo della bozza vuota, poi la conferma con la sua firma. Restituisce il testo della conferma.
func maConfermaAlberoVuoto(t *testing.T, w *browser, thread uuid.UUID) string {
	t.Helper()
	base := "/thread/" + thread.String() + "/distinta/albero"
	_, corpo := w.chiediJSON(http.MethodGet, base, "")
	var a fascicolo.AlberoProposto
	if err := json.Unmarshal([]byte(corpo), &a); err != nil {
		t.Fatalf("l'albero: %v: %.300s", err, corpo)
	}
	vuota := `{"formato": 1, "base": "` + a.Firma + `"}`
	_, corpo = w.chiediJSON(http.MethodPost, base+"/riepilogo", vuota)
	var rp fascicolo.RiepilogoAlbero
	if err := json.Unmarshal([]byte(corpo), &rp); err != nil || !rp.Confermabile {
		t.Fatalf("il riepilogo della bozza vuota: %v, confermabile %v: %.400s", err, rp.Confermabile, corpo)
	}
	resp, corpo := w.chiediJSON(http.MethodPost, base+"/conferma", corpoConferma(t, rp.Firma, vuota))
	var esito struct{ Testo string }
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(corpo), &esito) != nil {
		t.Fatalf("la conferma dell'albero: %d %.400s", resp.StatusCode, corpo)
	}
	return esito.Testo
}

// maProdottoDi: il prodotto valutato del componente dato, dai dati della pagina; fallisce con l'elenco se non c'è.
func maProdottoDi(t *testing.T, d maDatiL4, comp uuid.UUID) maProdottoL4 {
	t.Helper()
	if d.Errore != "" || d.Esito == nil {
		t.Fatalf("dati: errore %q, esito %v", d.Errore, d.Esito != nil)
	}
	var visti []string
	for _, p := range d.Esito.ProdottiValutati {
		if p.ComponenteID != nil && *p.ComponenteID == comp {
			return p
		}
		visti = append(visti, p.Rif)
	}
	t.Fatalf("nessun prodotto valutato del componente %s (valutato %v, motivo %q; prodotti %v)", comp, d.Esito.Valutato, d.Esito.Motivo, visti)
	return maProdottoL4{}
}

// A1d-L4-07 (R-B7-04; U-07; ELENCO RK-08; ANALISI_A1D_BACKEND §5.1; D-A1d-20): caratterizzazione, fissa il comportamento
// di oggi e non lo giudica. Una relazione dello STEP autorizzato accettata a mano (il figlio prima deciso, come vuole
// AccettaRelazione) e poi «Conferma l'albero» con la bozza vuota: la conferma non ritocca le righe decise da una persona,
// la relazione resta senza il segno dell'albero, e la pagina dice la BOM non verificata, con almeno una voce da decidere.
// I gesti partono dalla pagina (il ramo dell'esito). La variante (AccettaNodo su un figlio il cui legame c'è già nella
// working, poi la rilettura del file) non ha ancora un valore atteso (D-A1d-20, «plausibile, non provato»): la prova la
// esegue, scrive che cosa la pagina dice e controlla solo che una BOM verificata non abbia voci da decidere; il valore
// atteso si fissa alla prima esecuzione sul PC originale.
func TestMotoreAL4RB704Caratterizzazione(t *testing.T) {
	banco := func(t *testing.T) (*bancoWeb, *browser) {
		t.Helper()
		b := preparaBancoWeb(t)
		ad := b.browser("10.0.0.8")
		ad.login("AD", "prova-ad")
		return b, ad
	}

	t.Run("relazione accettata a mano", func(t *testing.T) {
		b, ad := banco(t)
		r, prodotto, step := b.maScenaRB704(t, "MA407", "71201")
		maAccendi(t, b, regoleACMEPerLaScena(t, uuidSQL(t, b, `SELECT cliente_id FROM thread_offerta WHERE thread_id = $1`, r.thread), "Acme S.p.A.", 20000))
		pagina, th := maPagina(r.thread), "/thread/"+r.thread.String()
		figlio := uuidSQL(t, b, `SELECT proposta_id FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#2'`, r.thread, step)
		resp, corpo := ad.daMotoreA(http.MethodPost, th+"/fascicolo/nodo/"+figlio.String()+"/accetta", url.Values{"tipo": {"sciolto"}}, pagina)
		if e, testo := maEsito(t, "accetta il nodo", resp, corpo); e != "ok" {
			t.Fatalf("accetta il nodo: %s «%s»", e, testo)
		}
		resp, corpo = ad.daMotoreA(http.MethodPost, th+"/fascicolo/relazione/accetta",
			url.Values{"allegato": {step.String()}, "padre": {"#1"}, "figlio": {"#2"}}, pagina)
		if e, testo := maEsito(t, "accetta la relazione", resp, corpo); e != "ok" {
			t.Fatalf("accetta la relazione: %s «%s»", e, testo)
		}
		testo := maConfermaAlberoVuoto(t, ad, r.thread)
		if n := r.conta(`SELECT count(*) FROM relazione_proposta WHERE thread_id = $1 AND allegato_id = $2 AND padre_chiave = '#1'
			AND figlio_chiave = '#2' AND stato = 'confermata' AND deciso_da IS NOT NULL AND NOT (evidenza ? 'albero')`, r.thread, step); n != 1 {
			t.Errorf("la relazione accettata a mano dopo la conferma (%q): righe confermate senza il segno %d, attesa 1", testo, n)
		}
		d, _ := maDati(t, ad, r.thread)
		p := maProdottoDi(t, d, prodotto)
		if p.BOM.Verificata || p.BOM.Nomenclatura.DaDecidere+p.BOM.Gerarchia.DaDecidere < 1 {
			t.Errorf("R-B7-04: dopo la conferma dell'albero la BOM è verificata %v, da decidere %d + %d (atteso: non verificata, almeno 1)",
				p.BOM.Verificata, p.BOM.Nomenclatura.DaDecidere, p.BOM.Gerarchia.DaDecidere)
		}
	})

	t.Run("AccettaNodo e rilettura", func(t *testing.T) {
		b, ad := banco(t)
		r, prodotto, step := b.maScenaRB704(t, "MA407B", "71202")
		maAccendi(t, b, regoleACMEPerLaScena(t, uuidSQL(t, b, `SELECT cliente_id FROM thread_offerta WHERE thread_id = $1`, r.thread), "Acme S.p.A.", 20000))
		pagina, th := maPagina(r.thread), "/thread/"+r.thread.String()
		// il legame c'è già nella working, con la stessa quantità: la rilettura dopo AccettaNodo lo trova
		r.arco(prodotto, r.componenteTipo("7120201", "sciolto"), 2)
		figlio := uuidSQL(t, b, `SELECT proposta_id FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2 AND chiave = '#2'`, r.thread, step)
		resp, corpo := ad.daMotoreA(http.MethodPost, th+"/fascicolo/nodo/"+figlio.String()+"/accetta", url.Values{"tipo": {""}}, pagina)
		if e, testo := maEsito(t, "accetta il nodo", resp, corpo); e != "ok" {
			t.Fatalf("accetta il nodo: %s «%s»", e, testo)
		}
		riga := valoreR(r, `SELECT stato::text || '|' || CASE WHEN deciso_da IS NULL THEN 'automatismo' ELSE 'persona' END
			|| '|' || CASE WHEN evidenza ? 'albero' THEN 'segno' ELSE 'senza segno' END FROM relazione_proposta
			WHERE thread_id = $1 AND allegato_id = $2 AND padre_chiave = '#1' AND figlio_chiave = '#2'`, r.thread, step)
		testo := maConfermaAlberoVuoto(t, ad, r.thread)
		dopo := valoreR(r, `SELECT stato::text || '|' || CASE WHEN deciso_da IS NULL THEN 'automatismo' ELSE 'persona' END
			|| '|' || CASE WHEN evidenza ? 'albero' THEN 'segno' ELSE 'senza segno' END FROM relazione_proposta
			WHERE thread_id = $1 AND allegato_id = $2 AND padre_chiave = '#1' AND figlio_chiave = '#2'`, r.thread, step)
		d, _ := maDati(t, ad, r.thread)
		p := maProdottoDi(t, d, prodotto)
		t.Logf("R-B7-04, variante (D-A1d-20): la relazione #1 → #2 dopo AccettaNodo %s, dopo la conferma (%q) %s; BOM verificata %v, "+
			"nomenclatura %s da decidere %d, gerarchia %s da decidere %d", riga, testo, dopo, p.BOM.Verificata,
			p.BOM.Nomenclatura.Stato, p.BOM.Nomenclatura.DaDecidere, p.BOM.Gerarchia.Stato, p.BOM.Gerarchia.DaDecidere)
		if p.BOM.Verificata && p.BOM.Nomenclatura.DaDecidere+p.BOM.Gerarchia.DaDecidere > 0 {
			t.Errorf("una BOM verificata con %d voci da decidere", p.BOM.Nomenclatura.DaDecidere+p.BOM.Gerarchia.DaDecidere)
		}
	})
}

// ------------------------------------------------------------------ A1d-L4-08

// A1d-L4-08 (R43 B; T-A1d-15; piano A1d backend §5.1.5): con il limite dell'indice di 1 ms il calcolo non finisce in
// tempo: la pagina e i dati rispondono 200 con il testo del tempo scaduto (erroreTempoMotoreA, così com'è: RAC-05), senza
// esito; nessuna tabella cambia. Con il limite di sempre, subito dopo, la stessa pagina si calcola: il tempo scaduto non
// lascia niente dietro di sé.
func TestMotoreAL4TempoScaduto(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.maScenaL4(t, "MA408")
	maAccendi(t, b, s.maRegole(t, 1))
	prima := fotoDelDatabase(t, b)
	d, _ := maDati(t, s.ad, s.thread)
	if d.Errore != erroreTempoMotoreA || d.Esito != nil {
		t.Errorf("dati con 1 ms: errore %q, esito %v; atteso %q e nessun esito", d.Errore, d.Esito != nil, erroreTempoMotoreA)
	}
	resp, html := s.ad.fai(http.MethodGet, maPagina(s.thread), nil, false)
	if resp.StatusCode != 200 || !strings.Contains(leggibile(html), erroreTempoMotoreA) {
		t.Errorf("la pagina con 1 ms: %d, senza il testo del tempo scaduto", resp.StatusCode)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("il tempo scaduto ha scritto in: %s", d)
	}
	maAccendi(t, b, s.maRegole(t, 20000))
	if d, _ := maDati(t, s.ad, s.thread); d.Errore != "" || d.Esito == nil {
		t.Errorf("con il limite di sempre: errore %q, esito %v", d.Errore, d.Esito != nil)
	}
}
