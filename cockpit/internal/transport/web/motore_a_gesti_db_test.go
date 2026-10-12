//go:build integrazione

// L4 — giro 5, A1d-Q3: i gesti del ponte della Distinta nuova sulle rotte legacy, sul database di prova (COCKPIT_TEST_DSN,
// cockpit_test; Q-A1d-02 = A [U]), con le sessioni vere (mappa dei gesti A1d-P3, §5). Ogni gesto parte come lo manda
// web/static/motore_a_gesti.mjs: POST application/x-www-form-urlencoded, HX-Request: true, HX-Current-URL = la pagina
// /thread/{id}/motore-a, flag acceso. Lo schema (§5.1):
//   - rifiuto noto → il corpo è il solo <div data-ma-esito="no">, 200 e no-store, e nessuna tabella di dominio cambia
//     (fotoDominio: ogni tabella dello schema; di documento_proposta e job si tolgono i due campi che il rismistamento
//     scrive, dettagli.destinazione e job.priorita, misurati a parte da fotoRismistamento: TEC-03);
//   - successo → <div data-ma-esito="ok"> e l'effetto riletto dal database.
// Le aree: nodi (P3-L4-01…04), struttura (P3-L4-08…10, -19…-21, G-22), «Conferma l'albero» (P3-L4-11, la chiamata
// ALBERO), STEP (P3-L4-12, -13), documenti (P3-L4-17, -22…-24), operazioni (P3-L4-25, -27, -28); P3-L4-00 (flag spento
// e un'altra RFQ); CQ-03 (il rismistamento dopo un rifiuto con 200); le caratterizzazioni di R-B7-04 (CR-01…05d, con
// Q-A1d-06 [U]: la relazione accettata da sola si prova sul gestore legacy, che il ponte non chiama); dopo la revisione
// di P3: la RFQ inesistente (404 dal ramo, P3-GO-03), DB7-3 (la rinomina del prodotto) e D1 (l'ultimo figlio tolto).
// Il flag si accende sempre con le regole in errore (acmeAccendi): ogni gesto prova anche che il ramo guarda Accesa e
// non Pronta (TEC-01). Ogni risposta del ramo è senza intestazioni HX-* (gestoPonte; P3-GO-02, Q3-05).
//
// LE CARATTERIZZAZIONI FISSANO IL COMPORTAMENTO ATTUALE (anche dove la mappa lo registra come disallineamento: TEC-04,
// TEC-06, TEC-13, DOM-01, DOM-03, DOM-07, DOM-08, DOM-10) e non lo correggono. Nessuna prova dichiara BOM.Verificata =
// true come effetto voluto di gesti singoli: le costanti acmeDopo… fissano il falso verde di DOM-10 (la verifica che
// resta dopo un gesto singolo), non una regola.
//
// Le scene sono quelle delle prove legacy (B87, l'albero ACME, la RFQ con lo STEP del flusso F8, le copie), con codici
// già pubblicati nelle prove esistenti; il resto è inventato (ACME, acme.example, UUID sintetici).

package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// pontePagina: HX-Current-URL del ponte per la RFQ (la pagina, senza query).
func (b *bancoWeb) pontePagina(thread uuid.UUID) string {
	return b.srv.URL + "/thread/" + thread.String() + "/motore-a"
}

// dalPonteDa: una POST come la manda il ponte, con HX-Current-URL dato.
func (w *browser) dalPonteDa(percorso string, form url.Values, corrente string) (*http.Response, string) {
	w.b.t.Helper()
	if form == nil {
		form = url.Values{}
	}
	r, _ := http.NewRequest(http.MethodPost, w.b.srv.URL+percorso, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	r.Header.Set("HX-Request", "true")
	r.Header.Set("HX-Current-URL", corrente)
	r.Header.Set("X-Prova-IP", w.ip)
	resp, err := w.c.Do(r)
	if err != nil {
		w.b.t.Fatal(err)
	}
	defer resp.Body.Close()
	testo, _ := io.ReadAll(resp.Body)
	return resp, string(testo)
}

func (w *browser) dalPonte(percorso string, form url.Values, thread uuid.UUID) (*http.Response, string) {
	w.b.t.Helper()
	return w.dalPonteDa(percorso, form, w.b.pontePagina(thread))
}

var reEsitoPonte = regexp.MustCompile(`^<div data-ma-esito="(ok|no)">([^<]*)</div>$`)

// gestoPonte: l'esito («ok» / «no») e il testo di un gesto dal ponte. La prova cade se la risposta non è il solo
// <div data-ma-esito> con 200, text/html e no-store, o se porta un'intestazione HX-* (P3-GO-02, Q3-05: su
// /proposta/{u}/scarta pannelloConAvviso mette HX-Trigger prima del ramo, che lo toglie).
func (w *browser) gestoPonte(t *testing.T, percorso string, form url.Values, thread uuid.UUID) (string, string) {
	t.Helper()
	resp, corpo := w.dalPonte(percorso, form, thread)
	m := reEsitoPonte.FindStringSubmatch(corpo)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "no-store" ||
		resp.Header.Get("Content-Type") != "text/html; charset=utf-8" || m == nil {
		t.Fatalf("POST %s dal ponte: %d %q %q, non è il solo <div data-ma-esito>: %.400s", percorso, resp.StatusCode,
			resp.Header.Get("Cache-Control"), resp.Header.Get("Content-Type"), corpo)
	}
	for k := range resp.Header {
		if strings.HasPrefix(strings.ToLower(k), "hx-") {
			t.Errorf("POST %s dal ponte: intestazione %s: %q nella risposta del ramo", percorso, k, resp.Header.Get(k))
		}
	}
	return m[1], leggibile(m[2])
}

// alberoDalPonte: «Conferma l'albero» come la manda confermaAlbero del ponte: JSON {firma, bozza: {formato: 1, base}}
// con le intestazioni htmx della pagina.
func (w *browser) alberoDalPonte(thread uuid.UUID, firma, base string) (*http.Response, string) {
	w.b.t.Helper()
	corpo, _ := json.Marshal(map[string]any{"firma": firma, "bozza": map[string]any{"formato": 1, "base": base}})
	r, _ := http.NewRequest(http.MethodPost, w.b.srv.URL+"/thread/"+thread.String()+"/distinta/albero/conferma", strings.NewReader(string(corpo)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json")
	r.Header.Set("HX-Request", "true")
	r.Header.Set("HX-Current-URL", w.b.pontePagina(thread))
	r.Header.Set("X-Prova-IP", w.ip)
	resp, err := w.c.Do(r)
	if err != nil {
		w.b.t.Fatal(err)
	}
	defer resp.Body.Close()
	testo, _ := io.ReadAll(resp.Body)
	return resp, string(testo)
}

// acmeAccendi: il flag di [motore_a] acceso sul server della prova (il ramo di threadFrammento guarda solo Accesa).
func acmeAccendi(t *testing.T, b *bancoWeb) {
	t.Helper()
	mwAccendi(t, b, motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoNessunaGrammatica})
}

// fotoDominio: fotoDelDatabase, con documento_proposta e job senza dettagli.destinazione e job.priorita, che il
// rismistamento può scrivere anche dopo un rifiuto con 200 (TEC-03): quelli li misura fotoRismistamento.
func fotoDominio(t *testing.T, b *bancoWeb) map[string]string {
	t.Helper()
	f := fotoDelDatabase(t, b)
	for tab, espr := range map[string]string{
		"documento_proposta": `(to_jsonb(x) #- '{dettagli,destinazione}')::text`,
		"job":                `(to_jsonb(x) - 'priorita')::text`,
	} {
		var v string
		if err := b.pool.QueryRow(b.ctx, `SELECT count(*)::text || ' ' || coalesce(md5(string_agg(`+espr+`, '|' ORDER BY `+espr+`)), '-') FROM `+tab+` x`).Scan(&v); err != nil {
			t.Fatalf("%s: %v", tab, err)
		}
		f[tab] = v
	}
	return f
}

// fotoRismistamento: le destinazioni delle proposte e le priorità dei job.
func fotoRismistamento(t *testing.T, b *bancoWeb) string {
	t.Helper()
	var v string
	if err := b.pool.QueryRow(b.ctx, `SELECT
		(SELECT coalesce(md5(string_agg(proposta_id::text || ':' || coalesce((dettagli -> 'destinazione')::text, '-'), '|' ORDER BY proposta_id)), '-') FROM documento_proposta)
		|| ' ' ||
		(SELECT coalesce(md5(string_agg(job_id::text || ':' || priorita::text, '|' ORDER BY job_id)), '-') FROM job)`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// rifiutoNoto: il gesto deve rispondere «no» e non cambiare nessuna tabella di dominio. Restituisce il testo.
func rifiutoNoto(t *testing.T, b *bancoWeb, w *browser, thread uuid.UUID, percorso string, form url.Values) string {
	t.Helper()
	prima := fotoDominio(t, b)
	e, testo := w.gestoPonte(t, percorso, form, thread)
	if e != "no" {
		t.Errorf("rifiuto noto %s %v: esito %q (%q), atteso «no»", percorso, form, e, testo)
	}
	if d := differenze(prima, fotoDominio(t, b)); d != "" {
		t.Errorf("rifiuto noto %s %v (%q): tabelle cambiate: %s", percorso, form, testo, d)
	}
	return testo
}

// successo: il gesto deve rispondere «ok». Restituisce il testo.
func successo(t *testing.T, w *browser, thread uuid.UUID, percorso string, form url.Values) string {
	t.Helper()
	e, testo := w.gestoPonte(t, percorso, form, thread)
	if e != "ok" {
		t.Fatalf("%s %v: esito %q (%q), atteso «ok»", percorso, form, e, testo)
	}
	return testo
}

func valoreDi(t *testing.T, b *bancoWeb, sql string, arg ...any) string {
	t.Helper()
	var v *string
	if err := b.pool.QueryRow(b.ctx, sql, arg...).Scan(&v); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	if v == nil {
		return "NULL"
	}
	return *v
}

// ---------------------------------------------------------------- P3-L4-00 e i nodi

// P3-L4-00 e P3-L4-01…04 (rifiuti): l'albero ACME (scenaAlberoWeb), due STEP non autorizzati. A flag spento, o con
// HX-Current-URL di un'altra RFQ, il gesto scrive ma la risposta è la vecchia interfaccia (thread_corpo): il ponte la
// leggerebbe «errore» dopo una scrittura riuscita (TEC-01). A flag acceso: scarta, riapri, codice del nodo (successi e
// rifiuti), accetta su una riga guida di uno STEP non autorizzato (rifiuto).
func TestPonteL4FlagSpentoAltraRFQENodi(t *testing.T) {
	b := preparaBancoWeb(t)
	r := b.scenaAlberoWeb(t, "ACM01")
	op := operatore(b)
	pid := func(codice string) uuid.UUID {
		return uuidSQL(t, b, `SELECT proposta_id FROM componente_proposta WHERE thread_id = $1 AND codice = $2 ORDER BY proposta_id LIMIT 1`, r.thread, codice)
	}
	stato := func(p uuid.UUID) string {
		return valoreDi(t, b, `SELECT stato || ':' || coalesce(deciso_da::text, '-') FROM componente_proposta WHERE proposta_id = $1`, p)
	}
	vite, p11, p10 := pid("7121007"), pid("7120011"), pid("7120010")

	// P3-L4-00: flag spento → thread_corpo, e lo scarto è scritto lo stesso (TEC-01)
	b.ws.MotoreA = motorea_web.Regole{}
	resp, corpo := op.dalPonte(r.base()+"/nodo/"+vite.String()+"/scarta", nil, r.thread)
	if resp.StatusCode != 200 || strings.Contains(corpo, "data-ma-esito") || !strings.Contains(corpo, "<") {
		t.Errorf("flag spento: %d, la risposta doveva essere la vecchia interfaccia: %.200s", resp.StatusCode, corpo)
	}
	if got := stato(vite); got != "scartata:"+r.utente.String() {
		t.Errorf("flag spento: lo scarto doveva essere scritto (TEC-01): %s", got)
	}
	// flag acceso, HX-Current-URL di un'altra RFQ → thread_corpo, e la riapertura è scritta
	acmeAccendi(t, b)
	altra := b.srv.URL + "/thread/" + uuid.NewString() + "/motore-a"
	resp, corpo = op.dalPonteDa(r.base()+"/nodo/"+vite.String()+"/riapri", nil, altra)
	if resp.StatusCode != 200 || strings.Contains(corpo, "data-ma-esito") {
		t.Errorf("un'altra RFQ: %d, la risposta doveva essere la vecchia interfaccia: %.200s", resp.StatusCode, corpo)
	}
	if got := stato(vite); !strings.HasPrefix(got, "aperta:") {
		t.Errorf("un'altra RFQ: la riapertura doveva essere scritta: %s", got)
	}

	// P3-L4-02: scarta → «ok», scartata con deciso_da; di nuovo → «no», niente cambia
	if testo := successo(t, op, r.thread, r.base()+"/nodo/"+vite.String()+"/scarta", nil); !strings.Contains(testo, "scartato") {
		t.Errorf("scarta: %q", testo)
	}
	if got := stato(vite); got != "scartata:"+r.utente.String() {
		t.Errorf("scarta: %s", got)
	}
	if testo := rifiutoNoto(t, b, op, r.thread, r.base()+"/nodo/"+vite.String()+"/scarta", nil); !strings.HasPrefix(testo, "Niente è cambiato") {
		t.Errorf("scarta di nuovo: %q", testo)
	}
	// P3-L4-03: riapri una riga aperta → «no»; la riga scartata → «ok», aperta con la storia
	if testo := rifiutoNoto(t, b, op, r.thread, r.base()+"/nodo/"+p11.String()+"/riapri", nil); !strings.Contains(testo, "si riapre solo un nodo scartato") {
		t.Errorf("riapri una riga aperta: %q", testo)
	}
	if testo := successo(t, op, r.thread, r.base()+"/nodo/"+vite.String()+"/riapri", nil); !strings.Contains(testo, "riaperto: torna fra le proposte") {
		t.Errorf("riapri: %q", testo)
	}
	if got := valoreDi(t, b, `SELECT stato || ':' || coalesce(evidenza -> 'storia' -> -1 ->> 'evento', '-') FROM componente_proposta WHERE proposta_id = $1`, vite); got != "aperta:nodo_riaperto" {
		t.Errorf("riapri: %s", got)
	}
	// P3-L4-04: codice vuoto → «no»; codice e rev vuota → «ok», origine operatore, rev NULL, nessun autore (DOM-08)
	rifiutoNoto(t, b, op, r.thread, r.base()+"/nodo/"+p11.String()+"/codice", url.Values{"codice": {""}, "rev": {""}})
	if testo := successo(t, op, r.thread, r.base()+"/nodo/"+p11.String()+"/codice", url.Values{"codice": {"7120012"}, "rev": {""}}); !strings.Contains(testo, "7120012") {
		t.Errorf("codice del nodo: %q", testo)
	}
	if got := valoreDi(t, b, `SELECT codice || ':' || coalesce(rev, 'NULL') || ':' || origine_codice::text || ':' || stato || ':' || coalesce(deciso_da::text, '-')
		FROM componente_proposta WHERE proposta_id = $1`, p11); got != "7120012:NULL:operatore:aperta:-" {
		t.Errorf("codice del nodo: %s", got)
	}
	// P3-L4-01 (rifiuto): accetta su una riga di uno STEP non autorizzato → «no», niente cambia
	testo := rifiutoNoto(t, b, op, r.thread, r.base()+"/nodo/"+p10.String()+"/accetta", url.Values{"tipo": {""}})
	t.Logf("accetta su una riga di uno STEP non autorizzato: %q", testo)
}

// ---------------------------------------------------------------- la struttura

// P3-L4-08…10, -19…-21 e G-22 sulla scena B87 (prodotto finito, assieme ×2, particolare sotto l'assieme ×4 e sotto il
// prodotto ×1).
func TestPonteL4Struttura(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("ACM02")
	acmeAccendi(t, b)
	op := operatore(b)
	nuovo := s.componenteTipo("77700001", "sciolto")
	nuovo2 := s.componenteTipo("77700002", "sciolto")
	ha := func(arco string) bool { return strings.Contains(" "+s.archi()+" ", " "+arco+" ") }

	// P3-L4-08: collega sotto un particolare → «no»; sotto l'assieme ×3 → «ok», arco manuale con confermato_da
	rifiutoNoto(t, b, op, s.thread, s.comp(nuovo, "collega"), url.Values{"padre": {s.particolare.String()}, "qta": {"1"}})
	rifiutoNoto(t, b, op, s.thread, s.comp(s.prodotto, "collega"), url.Values{"padre": {s.particolare.String()}, "qta": {"1"}}) // ciclo o non contenitore
	successo(t, op, s.thread, s.comp(nuovo, "collega"), url.Values{"padre": {s.assieme.String()}, "qta": {"3"}})
	if !ha("77720517>77700001x3") {
		t.Errorf("collega: %s", s.archi())
	}
	if got := valoreDi(t, b, `SELECT origine::text || ':' || coalesce(confermato_da::text, '-') FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2`,
		s.assieme, nuovo); got != "manuale:"+s.utente.String() {
		t.Errorf("collega, l'arco: %s", got)
	}
	// TEC-13 (caratterizzazione): il gestore legge una qta assente come 1; il ponte non la manda mai assente (guardia)
	successo(t, op, s.thread, s.comp(nuovo2, "collega"), url.Values{"padre": {s.assieme.String()}})
	if !ha("77720517>77700002x1") {
		t.Errorf("collega senza qta (TEC-13): %s", s.archi())
	}
	// P3-L4-09: da == a → «no»; da l'assieme al prodotto ×2 → «ok»
	rifiutoNoto(t, b, op, s.thread, s.comp(nuovo, "sposta"), url.Values{"da": {s.assieme.String()}, "a": {s.assieme.String()}, "qta": {"3"}})
	successo(t, op, s.thread, s.comp(nuovo, "sposta"), url.Values{"da": {s.assieme.String()}, "a": {s.prodotto.String()}, "qta": {"2"}})
	if !ha("77722757>77700001x2") || ha("77720517>77700001x3") {
		t.Errorf("sposta: %s", s.archi())
	}
	// P3-L4-10: arco assente → «no»; il particolare dal prodotto → «ok», nessun autore (DOM-08)
	rifiutoNoto(t, b, op, s.thread, s.comp(nuovo2, "scollega"), url.Values{"padre": {s.prodotto.String()}})
	successo(t, op, s.thread, s.comp(s.particolare, "scollega"), url.Values{"padre": {s.prodotto.String()}})
	if ha("77722757>77817189x1") || !ha("77720517>77817189x4") {
		t.Errorf("scollega: %s", s.archi())
	}
	// P3-L4-19: motivo vuoto → «no»; archivia → «ok», archi tolti nei due versi, archiviato_da
	rifiutoNoto(t, b, op, s.thread, s.comp(nuovo2, "archivia"), url.Values{"motivo": {""}})
	successo(t, op, s.thread, s.comp(nuovo2, "archivia"), url.Values{"motivo": {"doppione"}})
	if ha("77720517>77700002x1") {
		t.Errorf("archivia: l'arco resta: %s", s.archi())
	}
	if got := valoreDi(t, b, `SELECT coalesce(archiviato_da::text, '-') || ':' || coalesce(motivo_archiviazione, '-') FROM componente WHERE componente_id = $1`, nuovo2); got != s.utente.String()+":doppione" {
		t.Errorf("archivia: %s", got)
	}
	rifiutoNoto(t, b, op, s.thread, s.comp(nuovo2, "archivia"), url.Values{"motivo": {"ancora"}}) // già archiviato
	// P3-L4-21: non archiviato → «no»; ripristina → «ok», archiviato_* azzerati (si perde chi e perché: DOM-08)
	rifiutoNoto(t, b, op, s.thread, s.comp(nuovo, "ripristina"), nil)
	successo(t, op, s.thread, s.comp(nuovo2, "ripristina"), nil)
	if got := valoreDi(t, b, `SELECT coalesce(archiviato_il::text, '-') || ':' || coalesce(archiviato_da::text, '-') || ':' || coalesce(motivo_archiviazione, '-')
		FROM componente WHERE componente_id = $1`, nuovo2); got != "-:-:-" {
		t.Errorf("ripristina: %s", got)
	}
	// G-22 (P3-L4-20): rimuovi un componente senza storia → «ok», cancellato
	successo(t, op, s.thread, s.comp(nuovo2, "rimuovi"), nil)
	if n := s.conta(`SELECT count(*) FROM componente WHERE componente_id = $1`, nuovo2); n != 0 {
		t.Errorf("rimuovi: il componente c'è ancora")
	}
}

// ---------------------------------------------------------------- lo STEP

// P3-L4-12 e -13 sulla scena B87 con lo STEP letto (mwScena): la firma viene dalla GET JSON di P2
// (/motore-a/anteprima/step-strutturale, dell'amministratore), come nel ponte; la POST è la rotta legacy.
func TestPonteL4AutorizzaERevocaLoSTEP(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.mwScena(t, "ACM03")
	mwAccendi(t, b, s.regole)
	resp, corpo := s.ad.fai(http.MethodGet, s.anteprima(), nil, false)
	var a motorea_web.AnteprimaSTEP
	if resp.StatusCode != 200 || json.Unmarshal([]byte(corpo), &a) != nil || a.Firma == "" || !a.Inviabile {
		t.Fatalf("anteprima: %d %.300s", resp.StatusCode, corpo)
	}
	form := url.Values{"documento": {s.step.String()}, "autorizza": {"1"}, "firma": {strings.Repeat("0", 24)}}
	if a.PresaDAtto != "" {
		form.Set("presa_d_atto", "1")
	}
	stepDi := func() string {
		return valoreDi(t, b, `SELECT step_strutturale_id::text FROM componente WHERE componente_id = $1`, s.prodotto)
	}
	if testo := rifiutoNoto(t, b, s.ad, s.thread, s.comp(s.prodotto, "step-strutturale"), form); !strings.HasPrefix(testo, "Niente è cambiato") {
		t.Errorf("firma cambiata: %q", testo)
	}
	nd := url.Values{"documento": {s.step.String()}, "nodo": {s.allStep.String() + ":#1"}, "autorizza": {"1"}, "firma": {a.Firma}}
	rifiutoNoto(t, b, s.ad, s.thread, s.comp(s.prodotto, "step-strutturale"), nd) // documento e nodo insieme
	form.Set("firma", a.Firma)
	if testo := successo(t, s.ad, s.thread, s.comp(s.prodotto, "step-strutturale"), form); !strings.Contains(testo, "è lo STEP autorizzato di 77722757") {
		t.Errorf("autorizza: %q", testo)
	}
	if got := stepDi(); got != s.step.String() {
		t.Errorf("autorizza: step_strutturale_id %s, atteso %s", got, s.step)
	}
	// P3-L4-13: senza STEP autorizzato → «no»; revoca → «ok», step_strutturale_id NULL, gli archi restano
	rifiutoNoto(t, b, s.ad, s.thread, s.comp(s.assieme, "step-strutturale/revoca"), url.Values{"sha": {""}, "motivo": {""}})
	archi := s.archi()
	if testo := successo(t, s.ad, s.thread, s.comp(s.prodotto, "step-strutturale/revoca"), url.Values{"sha": {s.shaDoc(s.step)}, "motivo": {"prova"}}); !strings.Contains(testo, "Revocata l'autorizzazione") {
		t.Errorf("revoca: %q", testo)
	}
	if got := stepDi(); got != "NULL" {
		t.Errorf("revoca: step_strutturale_id %s", got)
	}
	if s.archi() != archi {
		t.Errorf("revoca: gli archi sono cambiati: %s → %s", archi, s.archi())
	}
}

// ---------------------------------------------------------------- i documenti

// P3-L4-17, -22, -23, -24 sulla scena B87 (due PDF arrivati e non assegnati).
func TestPonteL4Documenti(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("ACM04")
	acmeAccendi(t, b)
	op := operatore(b)
	// P3-L4-24: scarta il file → «ok» (ritorna_thread lo mette il ponte); di nuovo → «no», niente cambia
	scarta := "/proposta/" + s.pdfLibero.String() + "/scarta"
	if testo := successo(t, op, s.thread, scarta, url.Values{"ritorna_thread": {s.thread.String()}}); !strings.Contains(testo, "scartata") {
		t.Errorf("scarta il file: %q", testo)
	}
	if got := valoreDi(t, b, `SELECT stato::text || ':' || coalesce(deciso_da::text, '-') FROM documento_proposta WHERE proposta_id = $1`, s.pdfLibero); got != "scartata:"+s.utente.String() {
		t.Errorf("scarta il file: %s", got)
	}
	if testo := rifiutoNoto(t, b, op, s.thread, scarta, url.Values{"ritorna_thread": {s.thread.String()}}); testo != "La proposta era già stata decisa: nessun cambiamento." {
		t.Errorf("scarta di nuovo: %q", testo)
	}
	// P3-L4-17: senza file → «ok» senza effetto (TEC-04, caratterizzazione); il PDF all'assieme → «ok», nessun autore
	prima := fotoDominio(t, b)
	if e, testo := op.gestoPonte(t, s.base()+"/assegna", url.Values{"componente": {s.assieme.String()}}, s.thread); e != "ok" || testo != "Nessun file scelto: niente da assegnare." {
		t.Errorf("assegna senza file (TEC-04): %q %q", e, testo)
	}
	if d := differenze(prima, fotoDominio(t, b)); d != "" {
		t.Errorf("assegna senza file ha scritto in: %s", d)
	}
	successo(t, op, s.thread, s.base()+"/assegna", url.Values{"componente": {s.assieme.String()}, "proposta": {s.pdfLibero2.String()}})
	if got := valoreDi(t, b, `SELECT coalesce(componente_id::text, '-') || ':' || stato::text FROM documento_proposta WHERE proposta_id = $1`, s.pdfLibero2); got != s.assieme.String()+":aperta" {
		t.Errorf("assegna: %s (nessuna conferma)", got)
	}
	// P3-L4-22: senza motivo → «no»; un motivo con «nessun cambiamento» → «no» DOPO la scrittura (TEC-06, caratterizzazione)
	vite := s.componenteTipo("54000000", "sciolto")
	rifiutoNoto(t, b, op, s.thread, s.comp(vite, "deroga"), url.Values{"tipo": {"disegno_2d"}, "motivo": {""}})
	if e, testo := op.gestoPonte(t, s.comp(vite, "deroga"), url.Values{"tipo": {"disegno_2d"}, "motivo": {"nessun cambiamento sul disegno"}}, s.thread); e != "no" ||
		testo != "54000000: disegno_2d derogato («nessun cambiamento sul disegno»)." {
		t.Errorf("deroga con il motivo negativo (TEC-06): %q %q", e, testo)
	}
	deroga := valoreDi(t, b, `SELECT deroga_id::text || ':' || utente_id::text FROM deroga_fabbisogno WHERE componente_id = $1`, vite)
	if !strings.HasSuffix(deroga, ":"+s.utente.String()) {
		t.Fatalf("la deroga doveva essere scritta (TEC-06): %s", deroga)
	}
	// P3-L4-23: una deroga che non c'è → «no»; la revoca → «ok», la riga cancellata
	rifiutoNoto(t, b, op, s.thread, s.base()+"/deroga/"+uuid.NewString()+"/revoca", nil)
	id, _, _ := strings.Cut(deroga, ":")
	if testo := successo(t, op, s.thread, s.base()+"/deroga/"+id+"/revoca", nil); testo != "Deroga revocata: il requisito torna a contare." {
		t.Errorf("revoca la deroga: %q", testo)
	}
	if n := s.conta(`SELECT count(*) FROM deroga_fabbisogno WHERE componente_id = $1`, vite); n != 0 {
		t.Errorf("revoca la deroga: %d righe", n)
	}
}

// ---------------------------------------------------------------- le operazioni

// P3-L4-27, -28 e CQ-03 sulla RFQ con lo STEP del flusso F8 (rfqConStep) e un disegno letto senza destinazione.
func TestPonteL4Operazioni(t *testing.T) {
	b := preparaBancoWeb(t)
	r, op, _ := rfqConStep(t, b, "ACM05", "7120010")
	acmeAccendi(t, b)
	disegno := disegnoLetto(t, r, r.msg, "7120010.pdf", uuid.NullUUID{UUID: r.thread, Valid: true})

	// P3-L4-27 + CQ-03: rianalizza senza analizzatore → «no», nessuna tabella di dominio cambiata; il rismistamento
	// gira lo stesso (TEC-03): si misura la destinazione del disegno
	an := b.ws.Analizzatore
	b.ws.Analizzatore = coda.Analizzatore{}
	rism := fotoRismistamento(t, b)
	if testo := rifiutoNoto(t, b, op, r.thread, r.base()+"/rianalizza", nil); !strings.Contains(testo, "l'analisi non è configurata") {
		t.Errorf("rianalizza senza analizzatore: %q", testo)
	}
	dopoRifiuto := destinazioneDi(t, b, disegno)
	t.Logf("CQ-03: dopo il rifiuto di rianalizza la destinazione del disegno è %s; rismistamento cambiato: %v", righeDi(dopoRifiuto), rism != fotoRismistamento(t, b))
	if dopoRifiuto == nil {
		t.Errorf("CQ-03 (caratterizzazione di TEC-03): dopo un rifiuto con 200 il rismistamento doveva aver scritto la destinazione")
	}
	b.ws.Analizzatore = an
	// rianalizza con l'analizzatore → «ok» (niente da rileggere: lo STEP è già letto)
	successo(t, op, r.thread, r.base()+"/rianalizza", nil)

	// P3-L4-28: aggiorna → «ok» e la destinazione scritta da aggiorna stesso; di nuovo → «già aggiornate», «ok». La
	// destinazione l'hanno già scritta il rismistamento dopo il rifiuto e dopo «Rianalizza» (CQ-03): si toglie prima,
	// perché la prova non sia vuota (Q3-03 della revisione di P3)
	r.esegui(`UPDATE documento_proposta SET dettagli = dettagli #- '{destinazione}' WHERE allegato_id = $1`, disegno)
	if d := destinazioneDi(t, b, disegno); d != nil {
		t.Fatalf("aggiorna: la destinazione doveva essere tolta prima: %s", righeDi(d))
	}
	rism = fotoRismistamento(t, b)
	if testo := successo(t, op, r.thread, "/thread/"+r.thread.String()+"/smistamento/aggiorna", nil); strings.Contains(testo, "già aggiornate") {
		t.Errorf("aggiorna con una destinazione tolta: %q, doveva aggiornarla", testo)
	}
	if d := destinazioneDi(t, b, disegno); d == nil || rism == fotoRismistamento(t, b) {
		t.Errorf("aggiorna: nessuna destinazione scritta (%s), rismistamento cambiato %v", righeDi(d), rism != fotoRismistamento(t, b))
	}
	if testo := successo(t, op, r.thread, "/thread/"+r.thread.String()+"/smistamento/aggiorna", nil); !strings.Contains(testo, "già aggiornate") {
		t.Errorf("aggiorna di nuovo: %q", testo)
	}
	// una RFQ chiusa → «ok» senza scrittura (TEC-04, caratterizzazione)
	r.esegui(`UPDATE thread_offerta SET stato = 'CHIUSA' WHERE thread_id = $1`, r.thread)
	prima, rism := fotoDominio(t, b), fotoRismistamento(t, b)
	if e, testo := op.gestoPonte(t, "/thread/"+r.thread.String()+"/smistamento/aggiorna", nil, r.thread); e != "ok" ||
		testo != "Le proposte di una RFQ "+fascicolo.EsclusaChiusa+" non si aggiornano." {
		t.Errorf("aggiorna su una RFQ chiusa (TEC-04): %q %q", e, testo)
	}
	if d := differenze(prima, fotoDominio(t, b)); d != "" || rism != fotoRismistamento(t, b) {
		t.Errorf("aggiorna su una RFQ chiusa ha scritto: %q, rismistamento cambiato %v", d, rism != fotoRismistamento(t, b))
	}

}

// P3-L4-25: «Riprova copie» con la capacità spenta → «ok» e nessun job (TEC-04, caratterizzazione); accesa → «ok», la
// copia in coda.
func TestPonteL4RiprovaCopie(t *testing.T) {
	b := preparaBancoWeb(t)
	acmeAccendi(t, b)
	op := operatore(b)
	thread, _ := b.rfqConDocumentoInCoda(1)
	ImpostaCapacitaProva(t, coda.Capacita{})
	prima := fotoDominio(t, b)
	if e, testo := op.gestoPonte(t, "/thread/"+thread.String()+"/riprova-copie", nil, thread); e != "ok" || !strings.HasPrefix(testo, "Copie NON rimesse in coda") {
		t.Errorf("riprova con la capacità spenta (TEC-04): %q %q", e, testo)
	}
	if d := differenze(prima, fotoDominio(t, b)); d != "" {
		t.Errorf("riprova con la capacità spenta ha scritto in: %s", d)
	}
	ImpostaCapacitaProva(t, coda.Capacita{NasScrittura: true})
	if testo := successo(t, op, thread, "/thread/"+thread.String()+"/riprova-copie", nil); !strings.Contains(testo, "1 copia rimessa in coda") {
		t.Errorf("riprova: %q", testo)
	}
	if n := len(b.copieInCoda()); n != 1 {
		t.Errorf("riprova: %d copie in coda", n)
	}
}

// ---------------------------------------------------------------- R-B7-04 (Q-A1d-06 [U])

// acmeScenaAut: il prodotto 77722757 (finito, identificativo confermato) con il suo STEP (fattiB87: l'assieme 77720517 ×2,
// sotto l'assieme il particolare 77817189 ×4), letto con «Rianalizza» e autorizzato dalla scheda: l'assieme è un figlio
// diretto nell'autorità, proposto (non c'è ancora nella BOM); il particolare resta guida. Le regole ACME del cliente
// per leggere /motore-a/dati.
type acmeScenaAut struct {
	*rfqFascicolo
	prodotto, step, allStep uuid.UUID
	op, ad                  *browser
	regole                  motorea_web.Regole
}

func (b *bancoWeb) acmeScenaAutorizzata(t *testing.T, chiave string) *acmeScenaAut {
	t.Helper()
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), chiave)
	s := &acmeScenaAut{rfqFascicolo: r}
	r.fase("FATTIBILITA")
	r.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confidenza, confermato_da) VALUES ($1, '77722757', 'proposta_famiglia', 80, $2)`,
		r.thread, r.utente)
	s.prodotto = r.componenteTipo("77722757", "finito")
	s.step, s.allStep = r.documentoDa("77722757.stp", "stp", db.TipoDocumentoCad3d, "77722757", s.prodotto, db.StatoNasScritto)
	r.analisiCorrente(r.shaDoc(s.step), fattiB87(false))
	b.ws.Analizzatore = anScena
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	s.op = operatore(b)
	if _, html := s.op.daFascicolo(http.MethodPost, r.base()+"/rianalizza", url.Values{}, r.thread, ""); !strings.Contains(avvisoF(html), "STEP riletto") {
		t.Fatalf("rianalizza: %q", avvisoF(html))
	}
	if a := r.autorizzaDallaScheda(s.op, s.prodotto, url.Values{"documento": {s.step.String()}}, nil); !strings.Contains(a, "è lo STEP autorizzato di 77722757") {
		t.Fatalf("autorizzazione: %q", a)
	}
	cliente := uuidSQL(t, b, `SELECT cliente_id FROM thread_offerta WHERE thread_id = $1`, r.thread)
	s.regole = mwRegoleACME(t, cliente, "Acme S.p.A.", 20*time.Second)
	s.ad = b.browser("10.0.0.8")
	s.ad.login("AD", "prova-ad")
	return s
}

func (s *acmeScenaAut) riga(t *testing.T, chiave string) uuid.UUID {
	t.Helper()
	return uuidSQL(t, s.b, `SELECT proposta_id FROM componente_proposta WHERE allegato_id = $1 AND chiave = $2`, s.allStep, chiave)
}

// relazione: lo stato, chi ha deciso e il segno di «Conferma l'albero» della riga dell'arco del file.
func (s *acmeScenaAut) relazione(t *testing.T, padre, figlio string) string {
	t.Helper()
	return valoreDi(t, s.b, `SELECT stato::text || ':' || CASE WHEN deciso_da IS NULL THEN 'auto' ELSE 'persona' END || ':' ||
		CASE WHEN evidenza ? 'albero' THEN 'segno' ELSE 'senza_segno' END
		FROM relazione_proposta WHERE allegato_id = $1 AND padre_chiave = $2 AND figlio_chiave = $3`, s.allStep, padre, figlio)
}

// nodo: come relazione, per la riga di un nodo del file.
func (s *acmeScenaAut) nodo(t *testing.T, chiave string) string {
	t.Helper()
	return valoreDi(t, s.b, `SELECT stato::text || ':' || CASE WHEN deciso_da IS NULL THEN 'auto' ELSE 'persona' END || ':' ||
		CASE WHEN evidenza ? 'albero' THEN 'segno' ELSE 'senza_segno' END
		FROM componente_proposta WHERE allegato_id = $1 AND chiave = $2`, s.allStep, chiave)
}

// bom: la verifica della BOM del prodotto come la dà GET /motore-a/dati (l'adattatore di «Conferma l'albero» in
// valutazione-3), con lo stato della fonte.
func (s *acmeScenaAut) bom(t *testing.T) (valutazione.VerificaBOM, string) {
	t.Helper()
	mwAccendi(t, s.b, s.regole)
	resp, corpo := s.ad.fai(http.MethodGet, "/thread/"+s.thread.String()+"/motore-a/dati", nil, false)
	var p struct {
		Esito *valutazione.EsitoThread `json:"esito"`
	}
	if resp.StatusCode != 200 || json.Unmarshal([]byte(corpo), &p) != nil || p.Esito == nil {
		t.Fatalf("dati: %d %.300s", resp.StatusCode, corpo)
	}
	for _, pv := range p.Esito.ProdottiValutati {
		if pv.ComponenteID != nil && *pv.ComponenteID == s.prodotto {
			return pv.BOM, string(pv.Fonte.Stato)
		}
	}
	t.Fatalf("dati: il prodotto 77722757 non è fra i prodotti valutati: %.500s", corpo)
	return valutazione.VerificaBOM{}, ""
}

// riepilogo: la firma del riepilogo della bozza vuota e la firma dell'albero (la base), come le legge la pagina.
func (s *acmeScenaAut) riepilogo(t *testing.T) fascicolo.RiepilogoAlbero {
	t.Helper()
	resp, corpo := s.op.chiediJSON(http.MethodGet, "/thread/"+s.thread.String()+"/distinta/albero/riepilogo", "")
	var rp fascicolo.RiepilogoAlbero
	if resp.StatusCode != 200 || json.Unmarshal([]byte(corpo), &rp) != nil {
		t.Fatalf("riepilogo: %d %.300s", resp.StatusCode, corpo)
	}
	return rp
}

// CR-01…CR-05b, P3-L4-01 (successo) e P3-L4-11: il nodo dell'assieme accettato dal ponte, poi la relazione prodotto →
// assieme accettata da sola sul gestore legacy (il ponte non la chiama: Q-A1d-06 [U]), poi «Conferma l'albero» dal
// ponte con la bozza vuota, poi la lettura dell'adattatore, poi «Rianalizza».
func TestPonteL4CaratterizzazioneRB704(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.acmeScenaAutorizzata(t, "ACM06")
	acmeAccendi(t, b)
	op := s.op
	t.Logf("dopo l'autorizzazione: #1 %s, #2 %s, #3 %s; #1>#2 %s, #2>#3 %s", s.nodo(t, "#1"), s.nodo(t, "#2"), s.nodo(t, "#3"),
		s.relazione(t, "#1", "#2"), s.relazione(t, "#2", "#3"))

	// P3-L4-01: accetta il nodo dell'assieme → «ok», il componente nasce confermato da chi accetta, nessun arco
	assieme := s.riga(t, "#2")
	archi := s.conta(`SELECT count(*) FROM componente_relazione WHERE thread_id = $1`, s.thread)
	if testo := successo(t, op, s.thread, s.base()+"/nodo/"+assieme.String()+"/accetta", url.Values{"tipo": {""}}); !strings.Contains(testo, "77720517") {
		t.Errorf("accetta il nodo: %q", testo)
	}
	if got := valoreDi(t, b, `SELECT c.tipo::text || ':' || coalesce(c.confermato_da::text, '-') FROM componente c WHERE c.thread_id = $1 AND c.codice = '77720517'`, s.thread); !strings.HasSuffix(got, ":"+s.utente.String()) {
		t.Errorf("accetta il nodo, il componente: %s", got)
	}
	if n := s.conta(`SELECT count(*) FROM componente_relazione WHERE thread_id = $1`, s.thread); n != archi {
		t.Errorf("accetta il nodo ha scritto archi: %d → %d", archi, n)
	}
	if got := s.nodo(t, "#2"); got != "confermata:persona:senza_segno" {
		t.Errorf("accetta il nodo, la riga: %s", got)
	}
	// P3-L4-01 (rifiuto): di nuovo → «no», niente cambia
	rifiutoNoto(t, b, op, s.thread, s.base()+"/nodo/"+assieme.String()+"/accetta", url.Values{"tipo": {""}})

	// CR-01: la relazione accettata da sola, sul gestore legacy: riga confermata da una persona, senza segno; l'arco
	// d'origine step nella BOM
	_, html := op.daFascicolo(http.MethodPost, s.base()+"/relazione/accetta", url.Values{"allegato": {s.allStep.String()}, "padre": {"#1"}, "figlio": {"#2"}}, s.thread, "")
	if a := avvisoF(html); !strings.Contains(a, "77720517") || avvisoNegativo(a) {
		t.Fatalf("CR-01, accetta la relazione: %q", a)
	}
	if got := s.relazione(t, "#1", "#2"); got != "confermata:persona:senza_segno" {
		t.Errorf("CR-01, la riga: %s", got)
	}
	if got := valoreDi(t, b, `SELECT r.origine::text || ':' || r.qta FROM componente_relazione r JOIN componente f ON f.componente_id = r.figlio_id
		WHERE r.padre_id = $1 AND f.codice = '77720517'`, s.prodotto); got != "step:2" {
		t.Errorf("CR-01, l'arco: %s", got)
	}
	bomPrima, fontePrima := s.bom(t)
	t.Logf("CR-01, la BOM prima della conferma: %+v (fonte %s)", bomPrima, fontePrima)
	if bomPrima.Verificata {
		t.Errorf("CR-01: gesti singoli e BOM verificata: %+v", bomPrima)
	}

	// P3-L4-11 (rifiuto): firma sbagliata → 422 {"errore"}, niente cambia
	rp := s.riepilogo(t)
	t.Logf("riepilogo della bozza vuota: confermabile %v, blocchi %v", rp.Confermabile, rp.Blocchi)
	prima := fotoDominio(t, b)
	resp, corpo := op.alberoDalPonte(s.thread, "abc", rp.AlberoFirma)
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(corpo, `"errore"`) {
		t.Errorf("conferma con la firma sbagliata: %d %s", resp.StatusCode, corpo)
	}
	if d := differenze(prima, fotoDominio(t, b)); d != "" {
		t.Errorf("conferma rifiutata ha scritto in: %s", d)
	}
	if !rp.Confermabile {
		t.Fatalf("CR-02: il riepilogo della bozza vuota non è confermabile (%v): la caratterizzazione non prosegue", rp.Blocchi)
	}
	// CR-02 / P3-L4-11 (successo): «Conferma l'albero» dal ponte → 200 {"testo"}
	resp, corpo = op.alberoDalPonte(s.thread, rp.Firma, rp.AlberoFirma)
	var esito struct{ Testo string }
	if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != "no-store" || json.Unmarshal([]byte(corpo), &esito) != nil || esito.Testo == "" {
		t.Fatalf("CR-02, conferma l'albero: %d %s", resp.StatusCode, corpo)
	}
	t.Logf("CR-02: %q; #2 %s, #3 %s; #1>#2 %s, #2>#3 %s", esito.Testo, s.nodo(t, "#2"), s.nodo(t, "#3"), s.relazione(t, "#1", "#2"), s.relazione(t, "#2", "#3"))
	// CR-02 / CR-05a: le righe decise prima restano senza segno (anche=false)
	if got := s.relazione(t, "#1", "#2"); got != "confermata:persona:senza_segno" {
		t.Errorf("CR-02, la relazione accettata da sola dopo la conferma: %s", got)
	}
	if got := s.nodo(t, "#2"); got != "confermata:persona:senza_segno" {
		t.Errorf("CR-05a, il nodo accettato da solo dopo la conferma: %s", got)
	}
	// CR-02 (Q3-07, SEM-05): le righe ancora aperte, anche quelle di guida (il particolare #3 e l'arco #2>#3), le decide
	// «Conferma l'albero», da persona e col segno (comportamento dichiarato in albero_conferma.go, giro 4)
	if got := s.nodo(t, "#3"); got != "confermata:persona:segno" {
		t.Errorf("CR-02, il nodo di guida #3 dopo la conferma: %s", got)
	}
	if got := s.relazione(t, "#2", "#3"); got != "confermata:persona:segno" {
		t.Errorf("CR-02, l'arco di guida #2>#3 dopo la conferma: %s", got)
	}
	// CR-03 / CR-04 (Q3-07, SEM-06): l'adattatore conta l'arco confermato non coperto dal segno. La causa si restringe:
	// gerarchia legacy da_verificare per «da_decidere» (non conflitto, non fonte_non_confermata), con una sola voce da
	// decidere, e l'unica riga confermata senza segno della scena è la relazione #1>#2 accettata da sola (sopra)
	bomDopo, fonte := s.bom(t)
	t.Logf("CR-03, la BOM dopo la conferma: %+v (fonte %s)", bomDopo, fonte)
	g := bomDopo.Gerarchia
	if bomDopo.Verificata || !g.Legacy || g.Stato != valutazione.StatoAsseDaVerificare || g.Motivo != valutazione.MotivoAsseDaDecidere ||
		g.DaDecidere != 1 || len(g.Conflitti) != 0 {
		t.Errorf("CR-03/CR-04: attesi gerarchia legacy, da_verificare per da_decidere, una voce, nessun conflitto, BOM non verificata: %+v", bomDopo)
	}
	if n := s.conta(`SELECT count(*) FROM relazione_proposta WHERE allegato_id = $1 AND stato = 'confermata' AND NOT (evidenza ? 'albero')`, s.allStep); n != 1 {
		t.Errorf("CR-03: righe confermate senza segno %d, attesa una (#1>#2)", n)
	}
	// la nomenclatura (R81): fissata com'è oggi
	if bomDopo.Nomenclatura.Stato != acmeNomenclaturaDopoCR04 {
		t.Errorf("CR-04, la nomenclatura: %+v, la caratterizzazione dice %s", bomDopo.Nomenclatura, acmeNomenclaturaDopoCR04)
	}
	// CR-05b: «Rianalizza» (la rilettura del file) non tocca le righe decise da una persona, non aggiunge segni, la BOM
	// resta la stessa
	n2, r12 := s.nodo(t, "#2"), s.relazione(t, "#1", "#2")
	successo(t, op, s.thread, s.base()+"/rianalizza", nil)
	if s.nodo(t, "#2") != n2 || s.relazione(t, "#1", "#2") != r12 {
		t.Errorf("CR-05b: rianalizza ha cambiato le righe decise: %s → %s, %s → %s", n2, s.nodo(t, "#2"), r12, s.relazione(t, "#1", "#2"))
	}
	if b3, _ := s.bom(t); !reflect.DeepEqual(b3, bomDopo) {
		t.Errorf("CR-05b: la BOM è cambiata dopo rianalizza: %+v → %+v", bomDopo, b3)
	}
}

// confermaComePrimoGesto: «Conferma l'albero» dal ponte, con la bozza vuota, come primo gesto dopo l'autorizzazione;
// restituisce la verifica della BOM che ne segue.
func (s *acmeScenaAut) confermaComePrimoGesto(t *testing.T) valutazione.VerificaBOM {
	t.Helper()
	rp := s.riepilogo(t)
	if !rp.Confermabile {
		t.Fatalf("il riepilogo della bozza vuota non è confermabile: %v", rp.Blocchi)
	}
	if resp, corpo := s.op.alberoDalPonte(s.thread, rp.Firma, rp.AlberoFirma); resp.StatusCode != 200 {
		t.Fatalf("conferma l'albero: %d %s", resp.StatusCode, corpo)
	}
	v, fonte := s.bom(t)
	t.Logf("la BOM dopo la sola conferma: %+v (fonte %s)", v, fonte)
	return v
}

// CR-05d: il controllo positivo. «Conferma l'albero» come primo gesto, con la fonte STEP confermata (K-02): la BOM
// risulta verificata. Poi le varianti di DOM-10, ognuna sulla sua scena: il cambio di quantità dell'arco confermato
// prodotto → assieme (con «Collega» sullo stesso arco), lo scollega dell'arco di guida assieme → particolare (#2>#3) e lo
// scollega dell'arco d'autorità prodotto → assieme (#1>#2). Fissano quello che succede oggi alla verifica, con la causa
// (Q3-04, SEM-02). CR-05e (riaprire un nodo tolto dalla conferma) NON si fa: con la bozza vuota della v1 del ponte
// nessun nodo è tolto dalla conferma.
func TestPonteL4CaratterizzazioneRB704ControlloPositivo(t *testing.T) {
	arcoC := func(t *testing.T, s *acmeScenaAut, padre, figlio uuid.UUID) int {
		t.Helper()
		return s.conta(`SELECT count(*) FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2`, padre, figlio)
	}
	t.Run("cambio di quantità", func(t *testing.T) {
		b := preparaBancoWeb(t)
		s := b.acmeScenaAutorizzata(t, "ACM07")
		acmeAccendi(t, b)
		if v := s.confermaComePrimoGesto(t); !v.Verificata {
			t.Fatalf("CR-05d: dopo «Conferma l'albero» come primo gesto con la fonte confermata la BOM doveva essere verificata: %+v", v)
		}
		assieme := uuidSQL(t, b, `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = '77720517'`, s.thread)
		successo(t, s.op, s.thread, s.base()+"/componente/"+assieme.String()+"/collega", url.Values{"padre": {s.prodotto.String()}, "qta": {"3"}})
		if got := valoreDi(t, b, `SELECT qta::text FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2`, s.prodotto, assieme); got != "3" {
			t.Errorf("DOM-10: la quantità dell'arco doveva essere 3: %s", got)
		}
		v2, _ := s.bom(t)
		t.Logf("DOM-10, dopo il cambio di quantità 2 → 3: %+v; la riga #1>#2: %s", v2, s.relazione(t, "#1", "#2"))
		if v2.Verificata != acmeDopoCambioQta {
			t.Errorf("DOM-10 (caratterizzazione): dopo il cambio di quantità Verificata = %v, la caratterizzazione dice %v", v2.Verificata, acmeDopoCambioQta)
		}
		// la causa (SEM-02): non il segno, ma il conflitto R95 con la quantità dello STEP confermato
		arco := false
		for _, c := range v2.Gerarchia.Conflitti {
			arco = arco || strings.HasPrefix(c, valutazione.PrefissoRifArco)
		}
		if v2.Gerarchia.Stato != valutazione.StatoAsseConflitto || v2.Gerarchia.Motivo != valutazione.MotivoAsseConflitto || !arco {
			t.Errorf("DOM-10: la verifica doveva cadere per il conflitto della gerarchia su un arco: %+v", v2.Gerarchia)
		}
	})
	t.Run("scollega dell'arco di guida", func(t *testing.T) {
		b := preparaBancoWeb(t)
		s := b.acmeScenaAutorizzata(t, "ACM08")
		acmeAccendi(t, b)
		if v := s.confermaComePrimoGesto(t); !v.Verificata {
			t.Fatalf("CR-05d: la BOM doveva essere verificata: %+v", v)
		}
		assieme := uuidSQL(t, b, `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = '77720517'`, s.thread)
		particolare := uuidSQL(t, b, `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = '77817189'`, s.thread)
		if n := arcoC(t, s, assieme, particolare); n != 1 {
			t.Fatalf("l'arco assieme → particolare doveva esserci prima dello scollega: %d", n)
		}
		successo(t, s.op, s.thread, s.base()+"/componente/"+particolare.String()+"/scollega", url.Values{"padre": {assieme.String()}})
		// lo scollega ha tolto l'arco davvero (Q3-04: senza, la caratterizzazione passava anche con un gesto a vuoto)
		if n := arcoC(t, s, assieme, particolare); n != 0 {
			t.Errorf("DOM-10: lo scollega doveva togliere l'arco assieme → particolare: %d", n)
		}
		v2, _ := s.bom(t)
		t.Logf("DOM-10, dopo lo scollega dell'arco di guida assieme → particolare: %+v; la riga #2>#3: %s", v2, s.relazione(t, "#2", "#3"))
		if got := s.relazione(t, "#2", "#3"); got != "confermata:persona:segno" {
			t.Errorf("DOM-10: la riga dell'arco tolto doveva restare confermata col segno, orfana: %s", got)
		}
		if v2.Verificata != acmeDopoScollegaGuida {
			t.Errorf("DOM-10 (caratterizzazione del falso verde): dopo lo scollega Verificata = %v, la caratterizzazione dice %v", v2.Verificata, acmeDopoScollegaGuida)
		}
	})
	t.Run("scollega dell'arco d'autorità", func(t *testing.T) {
		b := preparaBancoWeb(t)
		s := b.acmeScenaAutorizzata(t, "ACM10")
		acmeAccendi(t, b)
		if v := s.confermaComePrimoGesto(t); !v.Verificata {
			t.Fatalf("CR-05d: la BOM doveva essere verificata: %+v", v)
		}
		assieme := uuidSQL(t, b, `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = '77720517'`, s.thread)
		if n := arcoC(t, s, s.prodotto, assieme); n != 1 {
			t.Fatalf("l'arco prodotto → assieme doveva esserci prima dello scollega: %d", n)
		}
		successo(t, s.op, s.thread, s.base()+"/componente/"+assieme.String()+"/scollega", url.Values{"padre": {s.prodotto.String()}})
		if n := arcoC(t, s, s.prodotto, assieme); n != 0 {
			t.Errorf("DOM-10: lo scollega doveva togliere l'arco prodotto → assieme: %d", n)
		}
		v2, _ := s.bom(t)
		t.Logf("DOM-10, dopo lo scollega dell'arco d'autorità prodotto → assieme: %+v; la riga #1>#2: %s", v2, s.relazione(t, "#1", "#2"))
		if got := s.relazione(t, "#1", "#2"); got != "confermata:persona:segno" {
			t.Errorf("DOM-10: la riga dell'arco d'autorità tolto doveva restare confermata col segno, orfana: %s", got)
		}
		if v2.Verificata != acmeDopoScollegaAutorita {
			t.Errorf("DOM-10 (caratterizzazione del falso verde): dopo lo scollega dell'arco d'autorità Verificata = %v, la caratterizzazione dice %v", v2.Verificata, acmeDopoScollegaAutorita)
		}
	})
	t.Run("arco a mano", func(t *testing.T) {
		// CR-05c: un arco messo a mano, senza riga, sotto l'assieme: la BOM non è più verificata
		b := preparaBancoWeb(t)
		s := b.acmeScenaAutorizzata(t, "ACM09")
		acmeAccendi(t, b)
		if v := s.confermaComePrimoGesto(t); !v.Verificata {
			t.Fatalf("CR-05d: la BOM doveva essere verificata: %+v", v)
		}
		assieme := uuidSQL(t, b, `SELECT componente_id FROM componente WHERE thread_id = $1 AND codice = '77720517'`, s.thread)
		nuovo := s.componenteTipo("77700003", "sciolto")
		successo(t, s.op, s.thread, s.base()+"/componente/"+nuovo.String()+"/collega", url.Values{"padre": {assieme.String()}, "qta": {"1"}})
		if v3, _ := s.bom(t); v3.Verificata {
			t.Errorf("CR-05c: con un arco a mano senza riga la BOM risulta verificata: %+v", v3)
		}
	})
}

// TestPonteL4RFQInesistenteECaratterizzazioniB7: dopo la revisione di P3.
//   - P3-GO-03: un gesto dal ponte per una RFQ che non c'è (riprova-copie su un id inventato) risponde 404 «thread non
//     trovato» come il percorso legacy, mai «ok» (prima del ramo corretto il ramo rispondeva «ok»);
//   - DB7-3 / DOM-07 (SEM-04, caratterizzazione, nessuna correzione): «Correggi il codice» sul prodotto della RFQ riesce
//     con «ok», e il codice dell'identificativo della RFQ resta il vecchio: il legame per codice fra la RFQ e il suo
//     prodotto si stacca. La sola protezione è il template di P4;
//   - D1 / DOM-03 (SEM-04, caratterizzazione): lo scollega dell'ultimo figlio di un sottoassieme riesce con «ok», il
//     sottoassieme resta sottoassieme, senza figli, e l'esito non lo segnala.
func TestPonteL4RFQInesistenteECaratterizzazioniB7(t *testing.T) {
	t.Run("RFQ inesistente", func(t *testing.T) {
		b := preparaBancoWeb(t)
		acmeAccendi(t, b)
		op := operatore(b)
		altra := uuid.New()
		prima := fotoDominio(t, b)
		resp, corpo := op.dalPonte("/thread/"+altra.String()+"/riprova-copie", nil, altra)
		if resp.StatusCode != http.StatusNotFound || strings.Contains(corpo, "data-ma-esito") || !strings.Contains(corpo, "thread non trovato") {
			t.Errorf("riprova su una RFQ inesistente dal ponte: %d %q, atteso il 404 del legacy", resp.StatusCode, corpo)
		}
		if d := differenze(prima, fotoDominio(t, b)); d != "" {
			t.Errorf("riprova su una RFQ inesistente ha scritto in: %s", d)
		}
	})
	t.Run("DB7-3, la rinomina del prodotto", func(t *testing.T) {
		b := preparaBancoWeb(t)
		r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "ACM11")
		r.fase("FATTIBILITA")
		r.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confidenza, confermato_da) VALUES ($1, '77722757', 'proposta_famiglia', 80, $2)`,
			r.thread, r.utente)
		prodotto := r.componenteTipo("77722757", "finito")
		acmeAccendi(t, b)
		op := operatore(b)
		testo := successo(t, op, r.thread, r.base()+"/componente/"+prodotto.String()+"/codice", url.Values{"codice": {"77722758"}})
		if !strings.HasPrefix(testo, "Codice corretto: 77722757 → 77722758") {
			t.Errorf("DB7-3: la rinomina del prodotto: %q", testo)
		}
		if got := valoreDi(t, b, `SELECT c.codice || ':' || i.codice FROM componente c JOIN identificativo_thread i ON i.thread_id = c.thread_id
			WHERE c.componente_id = $1`, prodotto); got != "77722758:77722757" {
			t.Errorf("DB7-3 (caratterizzazione): componente e identificativo della RFQ: %s, atteso 77722758:77722757 (legame staccato)", got)
		}
	})
	t.Run("D1, l'ultimo figlio", func(t *testing.T) {
		b := preparaBancoWeb(t)
		s := b.scenaB87("ACM12")
		acmeAccendi(t, b)
		op := operatore(b)
		testo := successo(t, op, s.thread, s.comp(s.particolare, "scollega"), url.Values{"padre": {s.assieme.String()}})
		t.Logf("D1: %q", testo)
		if n := s.conta(`SELECT count(*) FROM componente_relazione WHERE padre_id = $1`, s.assieme); n != 0 {
			t.Errorf("D1: il sottoassieme doveva restare senza figli: %d", n)
		}
		if got := valoreDi(t, b, `SELECT tipo::text FROM componente WHERE componente_id = $1`, s.assieme); got != "sottoassieme" {
			t.Errorf("D1 (caratterizzazione): il tipo del sottoassieme senza figli: %s, atteso sottoassieme (nessuna trasformazione)", got)
		}
		if strings.Contains(testo, "senza figli") || strings.Contains(testo, "particolare") {
			t.Errorf("D1 (caratterizzazione): l'esito oggi non segnala il sottoassieme vuoto: %q", testo)
		}
	})
}

// Le letture di DOM-10 fissate dalla caratterizzazione (corse del 12/10 su cockpit_test). Sono il comportamento di oggi,
// NON una regola: un true qui è il falso verde di DOM-10 (la BOM che resta verificata dopo un gesto singolo), non un
// effetto voluto. DOM-10 resta vero nel suo fatto centrale (il segno copre la coppia, non la quantità): la verifica cade
// dopo il cambio di quantità solo per il conflitto R95 con la quantità dello STEP confermato (Gerarchia.Stato =
// conflitto, l'arco fra i conflitti, asserito sopra); con uno STEP che non dà la quantità non cadrebbe (SEM-02). Lo
// scollega lascia la BOM verificata sia sull'arco di guida (#2>#3) sia sull'arco d'autorità (#1>#2: il sottoassieme
// intero staccato dal prodotto), e la riga dell'arco tolto resta confermata col segno, orfana.
const (
	acmeDopoCambioQta        = false
	acmeDopoScollegaGuida    = true
	acmeDopoScollegaAutorita = true
)

// acmeNomenclaturaDopoCR04: lo stato della nomenclatura (R81) dopo la relazione accettata da sola e «Conferma l'albero»
// (CR-04), com'è oggi.
const acmeNomenclaturaDopoCR04 = valutazione.StatoAsseDaVerificare
