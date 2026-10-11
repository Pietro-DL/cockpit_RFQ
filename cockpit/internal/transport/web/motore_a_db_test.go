//go:build integrazione

// L4 — giro 5, A1d-Q2: le due rotte JSON della Distinta nuova sul database di prova (COCKPIT_TEST_DSN, cockpit_test;
// Q-A1d-02 = A [U]), con le sessioni vere. Derivate dalle prove A1d-L4-01, -02 e -05 della PR #18 di Stefano Valo
// (153117d: motore_a_db_test.go), sulle rotte di A1d-P2 (solo le due JSON) e con i codici di A1d-P2.
//   - Flag spento: 404 a tutti (anonimo, consultazione, operatore, amministratore), con ogni metodo, con lo stesso corpo
//     di un indirizzo che non esiste, e nessuna sessione toccata (R36).
//   - Flag acceso: solo l'amministratore (200 JSON); operatore e consultazione 403; l'anonimo 302; sempre no-store.
//   - RFQ inesistente: 404 su /dati, sull'anteprima e con HEAD.
//   - L'anteprima dello STEP: la stessa firma e le stesse frasi del modulo legacy del Fascicolo, nessuna riga cambiata.
// Le righe delle due GET in TestNessunaGetScrive (nessunaget_db_test.go) provano che non scrivono e non accodano job.
//
// Le regole sono quelle ACME dell'avvio (app/runtime/testdata/motore_a, le testdata della PR #18), lette dal file e
// riscritte per il cliente della scena: transport non importa app. La scena è la B87 di autorizzazione_db_test.go,
// presa così com'è: i suoi codici 777xxxxx erano già pubblicati e non sono nell'elenco privato del controllo (residuo
// R46; PD-03 della revisione di A1d-P2: qui non si certifica che siano inventati). Il resto è inventato (ACME,
// acme.example, UUID sintetici): il repository è pubblico.

package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/jsoncanonico"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// mwRegoleACME: le regole pronte con la grammatica ACME di app/runtime/testdata/motore_a, riscritta per il cliente
// (UUID e ragione sociale), compilata con motorea.CompilaInsieme come all'avvio; nessun file dei casi.
func mwRegoleACME(t *testing.T, cliente uuid.UUID, ragione string, limite time.Duration) motorea_web.Regole {
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
		t.Fatal(err)
	}
	cl := g["cliente"].(map[string]any)
	cl["id"], cl["ragione_sociale"] = cliente.String(), ragione
	gram, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	ind.Grammatiche = []grammatica.VoceIndice{{ClienteID: cliente, File: "acme.v1.json", Sha256: jsoncanonico.Impronta(gram)}}
	ind.Casi = nil
	ins, diag, err := motorea.CompilaInsieme(ind, map[string][]byte{"acme.v1.json": gram})
	if err != nil || ins.Motori[cliente] == nil {
		t.Fatalf("regole ACME: la grammatica del cliente della scena non si attiva: %v %v", err, diag)
	}
	return motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoPronta, Insieme: &ins, Limite: limite, Attivi: []uuid.UUID{cliente}}
}

// reTagMW: un tag HTML, da togliere per confrontare le parole.
var reTagMW = regexp.MustCompile(`<[^>]*>`)

// mwAccendi mette le regole sul server della prova (il flag si legge a ogni richiesta) e lo rispegne alla fine.
func mwAccendi(t *testing.T, b *bancoWeb, r motorea_web.Regole) {
	t.Helper()
	b.ws.MotoreA = r
	t.Cleanup(func() { b.ws.MotoreA = motorea_web.Regole{} })
}

// mwScena: la scena B87 (il prodotto 77722757 finito con lo STEP e il 2D, l'assieme e il particolare), lo STEP letto
// dall'analizzatore corrente, le regole ACME del suo cliente, e i quattro browser.
type mwScena struct {
	*scenaB87
	regole              motorea_web.Regole
	anonimo, op, co, ad *browser
}

func (b *bancoWeb) mwScena(t *testing.T, chiave string) *mwScena {
	t.Helper()
	s := &mwScena{scenaB87: b.scenaB87(chiave)}
	s.analisiCorrente(s.shaDoc(s.step), fattiB87(false))
	cliente := uuidSQL(t, b, `SELECT cliente_id FROM thread_offerta WHERE thread_id = $1`, s.thread)
	s.regole = mwRegoleACME(t, cliente, "Acme S.p.A.", 20*time.Second)
	s.anonimo = b.browser("10.0.0.7")
	s.op = operatore(b)
	s.co = b.browser("10.0.0.9")
	s.co.login("CO", "prova-co")
	s.ad = b.browser("10.0.0.8")
	s.ad.login("AD", "prova-ad")
	return s
}

func (s *mwScena) dati() string { return "/thread/" + s.thread.String() + "/motore-a/dati" }
func (s *mwScena) anteprima() string {
	return "/thread/" + s.thread.String() + "/motore-a/anteprima/step-strutturale?" +
		url.Values{"componente": {s.prodotto.String()}, "documento": {s.step.String()}}.Encode()
}

// mwAccessi: l'ultimo accesso di ogni sessione aperta, per token.
func mwAccessi(t *testing.T, b *bancoWeb) map[string]string {
	t.Helper()
	righe, err := b.pool.Query(b.ctx, `SELECT token, coalesce(ultimo_accesso::text, '-') FROM sessione`)
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

func TestMotoreAL4SpentoRisponde404ATutti(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.mwScena(t, "MA401")
	r := s.regole
	r.Accesa, r.Stato = false, motorea_web.StatoSpenta
	mwAccendi(t, b, r)
	_, rif := s.ad.fai(http.MethodGet, "/a1d-indirizzo-che-non-esiste", nil, false)
	prima := mwAccessi(t, b)
	for nome, w := range map[string]*browser{"anonimo": s.anonimo, "operatore": s.op, "consultazione": s.co, "amministratore": s.ad} {
		for _, percorso := range []string{s.dati(), s.anteprima()} {
			for _, metodo := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodDelete} {
				resp, corpo := w.fai(metodo, percorso, nil, false)
				if resp.StatusCode != http.StatusNotFound || resp.Header.Get("Cache-Control") != "" || resp.Header.Get("Allow") != "" ||
					resp.Header.Get("Location") != "" {
					t.Errorf("%s, %s %s: %d, Cache-Control %q, Allow %q, Location %q", nome, metodo, percorso, resp.StatusCode,
						resp.Header.Get("Cache-Control"), resp.Header.Get("Allow"), resp.Header.Get("Location"))
				}
				if metodo != http.MethodHead && corpo != rif {
					t.Errorf("%s, %s %s: corpo %q, quello di un indirizzo che non esiste %q", nome, metodo, percorso, corpo, rif)
				}
			}
		}
	}
	if dopo := mwAccessi(t, b); !maMappe(prima, dopo) {
		t.Errorf("a flag spento una sessione è stata toccata: %v → %v", prima, dopo)
	}
}

func maMappe(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestMotoreAL4AccesoSoloLAmministratore(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.mwScena(t, "MA402")
	mwAccendi(t, b, s.regole)
	for _, percorso := range []string{s.dati(), s.anteprima()} {
		resp, corpo := s.ad.fai(http.MethodGet, percorso, nil, false)
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/json; charset=utf-8" ||
			resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("amministratore, GET %s: %d %q %q: %.300s", percorso, resp.StatusCode, resp.Header.Get("Content-Type"),
				resp.Header.Get("Cache-Control"), corpo)
		}
		if resp, _ := s.ad.fai(http.MethodHead, percorso, nil, false); resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("amministratore, HEAD %s: %d %q", percorso, resp.StatusCode, resp.Header.Get("Cache-Control"))
		}
		for nome, w := range map[string]*browser{"operatore": s.op, "consultazione": s.co} {
			if resp, _ := w.fai(http.MethodGet, percorso, nil, false); resp.StatusCode != http.StatusForbidden || resp.Header.Get("Cache-Control") != "no-store" {
				t.Errorf("%s, GET %s: %d %q, attesi 403 e no-store", nome, percorso, resp.StatusCode, resp.Header.Get("Cache-Control"))
			}
		}
		if resp, _ := s.anonimo.fai(http.MethodGet, percorso, nil, false); resp.StatusCode != http.StatusFound ||
			resp.Header.Get("Location") != "/login" || resp.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("anonimo, GET %s: %d %q %q", percorso, resp.StatusCode, resp.Header.Get("Location"), resp.Header.Get("Cache-Control"))
		}
	}
	// il JSON di /dati: l'esito vero del thread della scena, nella versione che la pagina legge
	_, corpo := s.ad.fai(http.MethodGet, s.dati(), nil, false)
	var d struct {
		ThreadID string `json:"thread_id"`
		Errore   string `json:"errore"`
		Testa    struct {
			Cliente string `json:"cliente"`
			Fase    *struct {
				Nome string `json:"nome"`
			} `json:"fase"`
		} `json:"testa"`
		Esito *struct {
			ThreadID string `json:"thread_id"`
			Valutato bool   `json:"valutato"`
		} `json:"esito"`
	}
	if err := json.Unmarshal([]byte(corpo), &d); err != nil {
		t.Fatal(err)
	}
	if d.ThreadID != s.thread.String() || d.Errore != "" || d.Esito == nil || d.Esito.ThreadID != s.thread.String() || !d.Esito.Valutato ||
		d.Testa.Cliente != "Acme S.p.A." || d.Testa.Fase == nil || d.Testa.Fase.Nome != "FATTIBILITA" {
		t.Errorf("dati: %+v (esito %+v)", d, d.Esito)
	}
	// le regole non pronte: 503 con il testo generico
	r := s.regole
	r.Stato, r.Insieme = motorea_web.StatoNessunaGrammatica, nil
	b.ws.MotoreA = r
	resp, corpo := s.ad.fai(http.MethodGet, s.dati(), nil, false)
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(corpo, "nessuna grammatica") || resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("regole non pronte: %d %q %s", resp.StatusCode, resp.Header.Get("Cache-Control"), corpo)
	}
}

func TestMotoreAL4RFQInesistente(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.mwScena(t, "MA403")
	mwAccendi(t, b, s.regole)
	nessuna := uuid.MustParse("00000000-0000-4000-8000-00000000a1d0").String()
	for _, percorso := range []string{"/thread/" + nessuna + "/motore-a/dati",
		"/thread/" + nessuna + "/motore-a/anteprima/step-strutturale?componente=" + s.prodotto.String() + "&documento=" + s.step.String()} {
		resp, corpo := s.ad.fai(http.MethodGet, percorso, nil, false)
		if resp.StatusCode != http.StatusNotFound || !strings.Contains(corpo, `"errore":"RFQ non trovata"`) || resp.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("GET %s: %d %q %s", percorso, resp.StatusCode, resp.Header.Get("Cache-Control"), corpo)
		}
		if resp, _ := s.ad.fai(http.MethodHead, percorso, nil, false); resp.StatusCode != http.StatusNotFound {
			t.Errorf("HEAD %s: %d", percorso, resp.StatusCode)
		}
	}
}

// L'anteprima dello STEP (A1d-L4-05 della PR, con la sola lettura provata dalla foto del database): la firma e le frasi
// del JSON sono quelle del modulo legacy che il Fascicolo rende per la stessa richiesta.
func TestMotoreAL4AnteprimaStessaFirmaDelLegacy(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.mwScena(t, "MA405")
	mwAccendi(t, b, s.regole)
	prima := fotoDelDatabase(t, b)
	resp, corpo := s.ad.fai(http.MethodGet, s.anteprima(), nil, false)
	if resp.StatusCode != 200 {
		t.Fatalf("anteprima: %d %s", resp.StatusCode, corpo)
	}
	var a motorea_web.AnteprimaSTEP
	dec := json.NewDecoder(strings.NewReader(corpo))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		t.Fatal(err)
	}
	html := s.anteprimaAutorizzazione(s.ad, s.prodotto, url.Values{"documento": {s.step.String()}})
	m := reFirmaAutorizzazione.FindStringSubmatch(html)
	if m == nil || a.Firma != m[1] {
		t.Errorf("firma del JSON %q, del modulo legacy %v", a.Firma, m)
	}
	l := leggibile(html)
	for _, f := range a.Frasi {
		// il modulo legacy mette i codici e il nome del file fra <b class="mono">: si confrontano le parole
		if !strings.Contains(strings.Join(strings.Fields(reTagMW.ReplaceAllString(l, "")), " "), strings.Join(strings.Fields(f), " ")) {
			t.Errorf("la frase %q non è nel modulo legacy", f)
		}
	}
	if !a.Autorizzabile || !a.Inviabile || a.Motore != motorea_web.MotoreAnteprima || len(a.Figli) != 1 || a.Sorgente == "" {
		t.Errorf("anteprima: %+v", a)
	}
	if dopo := fotoDelDatabase(t, b); !stessaFoto(prima, dopo) {
		t.Errorf("l'anteprima ha scritto: %v", differenzeFoto(prima, dopo))
	}
}
