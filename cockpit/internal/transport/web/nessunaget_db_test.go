//go:build integrazione

// L4 — Smistamento, fase F1 (addendum A5.4.5, A5.8.1, prove 98-100): nessuna GET scrive. Aprire la pagina
// della RFQ, il Fascicolo, i suoi pannelli e le sue anteprime non cambia una riga del database, ne' per chi
// lavora ne' per chi consulta; il lavoro che l'apertura faceva da sola lo chiede la pagina con una POST
// (…/fascicolo/prepara) che accoda soltanto, e che chi consulta non puo' mandare; gli STEP gia' analizzati
// prima che il messaggio entrasse nella RFQ li rilegge la decisione di una persona, la creazione o l'aggancio.

package web

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// fattiCasoGuida: lo STEP del caso guida, la cui radice il file chiama 7120001A, con 7120010 ×2 e 7120011
// sotto.
var fattiCasoGuida = fattiDi("#1", []string{"#1=7120001A", "#2=7120010", "#3=7120011"}, []string{"#1>#2*2", "#1>#3*1"})

// tabelleDelloSchema sono tutte le tabelle dello schema corrente (non le viste), in ordine di nome.
func tabelleDelloSchema(t *testing.T, b *bancoWeb) []string {
	t.Helper()
	righe, err := b.pool.Query(b.ctx, `SELECT table_name FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_type = 'BASE TABLE' ORDER BY table_name`)
	if err != nil {
		t.Fatal(err)
	}
	tabelle, err := pgx.CollectRows(righe, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if len(tabelle) < 20 {
		t.Fatalf("lo schema ha %d tabelle: non e' quello del Cockpit", len(tabelle))
	}
	return tabelle
}

// righeDelDatabase conta le righe di ogni tabella dello schema, in una riga sola: due letture uguali
// vogliono dire che nessuna tabella ha guadagnato o perso righe.
func righeDelDatabase(t *testing.T, b *bancoWeb) string {
	t.Helper()
	var out []string
	for _, tab := range tabelleDelloSchema(t, b) {
		out = append(out, fmt.Sprintf("%s=%d", tab, contaSQL(t, b, `SELECT count(*) FROM `+pgx.Identifier{tab}.Sanitize())))
	}
	return strings.Join(out, " ")
}

// fotoDelDatabase e' lo stato di ogni tabella dello schema: quante righe e l'impronta del contenuto, cosi'
// si vede anche una GET che riscrive una riga senza aggiungerne (una proposta riclassificata, un file
// segnato letto). Della sessione si contano solo le righe: ogni richiesta autenticata ne aggiorna
// ultimo_accesso (ToccaSessione), ed e' la sessione di chi guarda, non la RFQ.
func fotoDelDatabase(t *testing.T, b *bancoWeb) map[string]string {
	t.Helper()
	foto := map[string]string{}
	for _, tab := range tabelleDelloSchema(t, b) {
		id := pgx.Identifier{tab}.Sanitize()
		sql := `SELECT count(*)::text || ' ' || coalesce(md5(string_agg(x::text, '|' ORDER BY x::text)), '-') FROM ` + id + ` x`
		if tab == "sessione" {
			sql = `SELECT count(*)::text FROM ` + id
		}
		var v string
		if err := b.pool.QueryRow(b.ctx, sql).Scan(&v); err != nil {
			t.Fatalf("%s: %v", tab, err)
		}
		foto[tab] = v
	}
	return foto
}

// differenze dice quali tabelle sono cambiate fra due foto; "" se nessuna.
func differenze(prima, dopo map[string]string) string {
	var diverse []string
	for tab, v := range dopo {
		if prima[tab] != v {
			diverse = append(diverse, tab)
		}
	}
	for tab := range prima {
		if _, ok := dopo[tab]; !ok {
			diverse = append(diverse, tab)
		}
	}
	sort.Strings(diverse)
	return strings.Join(diverse, ", ")
}

// scenaSenzaGet e' una RFQ con tutto quello che l'apertura di una pagina, fino a B8.7b, metteva in moto da
// sola: il codice della richiesta confermato e senza prodotto (i prodotti della richiesta), un disegno utile
// non ancora sceso (il download), uno STEP analizzato con i fatti correnti (la rilettura), uno STEP senza i
// fatti dell'analizzatore corrente (l'analisi da accodare), uno STEP fermo nello staging i cui fatti ci sono
// gia' (lo stage riusato, che non ha mai prodotto una lettura) e un PDF fermo mai analizzato. In piu' un
// componente con il suo disegno e un PDF gia' letto, perche' le pagine abbiano qualcosa da mostrare. La strada
// del workerapi per i file fermi c'e', come in produzione.
type scenaSenzaGet struct {
	*rfqFascicolo
	an                                  coda.Analizzatore
	letto, vecchio, stepFermo, pdfFermo uuid.UUID // allegati
	disegno                             uuid.UUID // l'allegato del documento di 7120099
	pezzo                               uuid.UUID // il componente 7120099
}

func (b *bancoWeb) scenaSenzaGet(t *testing.T, chiave string) *scenaSenzaGet {
	t.Helper()
	wa := b.caricamentoAcceso(t)
	an := wa.Analizzatore
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	msg := b.mailConAllegati("RFQ 7120001 "+chiave, "Richiesta di offerta per 7120001.",
		worker.AllegatoIn{Indice: 1, NomeFile: "7120001.pdf", Estensione: "pdf", Natura: "file", Bytes: 9000})
	cartella := `ACME\WIP\2026 09 26 RFQ 7120001 ` + chiave
	thread := b.rfqDa(msg, acme, cartella, "7120001")
	// i file stanno sotto lo staging del server, come quelli scesi davvero: l'anteprima non serve altro
	r := &rfqFascicolo{b: b, thread: thread, msg: msg, cartella: cartella, staging: wa.Staging, n: 10}
	r.utente = uuidSQL(t, b, `SELECT utente_id FROM utente WHERE sigla = 'FP'`)
	r.esegui(`UPDATE identificativo_thread SET confermato_da = $2 WHERE thread_id = $1`, thread, r.utente)
	s := &scenaSenzaGet{rfqFascicolo: r, an: an}
	s.letto = r.stepNellaRfq("7120001A_1.stp", fattiCasoGuida, an)
	s.vecchio = r.stepNellaRfq("7120011.stp", "", an)
	var sha string
	s.stepFermo, sha = r.allegatoExt("7120012.stp", "stp")
	// i fatti di un'analisi vera portano l'esito: senza, la strada del workerapi lascerebbe il file fermo
	fermo := fattiDi("#1", []string{"#1=7120012"}, nil)
	fermo = `{"esito": {"tipo_proposto": "cad_3d", "codice": "7120012", "confidenza": 90, "fonte": "step"}, ` + fermo[1:]
	r.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, $2, $3, $4)`,
		sha, an.Versione, an.Hash(), fermo)
	s.pdfFermo, _ = r.allegato("7120010.pdf")
	r.esegui(`UPDATE allegato SET stato = 'in_staging' WHERE allegato_id IN ($1, $2)`, s.stepFermo, s.pdfFermo)
	s.pezzo = r.componente("7120099")
	_, s.disegno = r.documentoDa("7120099.pdf", "pdf", db.TipoDocumentoDisegno2d, "7120099", s.pezzo, db.StatoNasScritto)
	r.propostaDa("7120010_1.pdf", "7120010")
	return s
}

// Prova 98 (R8, E29): nessuna GET scrive. Le GET della pagina della RFQ, del Fascicolo in ogni vista e
// cassetto, dei suoi pannelli, anteprime, dati dell'editor, avanzamento e cassetto del NAS, dell'anteprima
// di un file e della pagina Richieste, con l'operatore e con la consultazione: prima e dopo, ogni tabella
// dello schema ha le stesse righe con lo stesso contenuto. Poi la controprova: la stessa pagina, con la sua
// POST di preparazione, il database lo cambia (altrimenti la scena non proverebbe niente).
//
// Giro 4, fase 4.3: fra le GET anche le tre della Distinta (la pagina in ogni passo, i dati, l'anteprima del tipo),
// che con la PR #8 e' la pagina con cui si apre una richiesta. Prima l'elenco non le aveva (la Distinta non era nel
// ramo delle prove); le verifiche di prima restano tutte. La prova dedicata, con ogni gruppo del passo 3, e'
// TestLeGetDellaDistintaNonScrivono (distinta_db_test.go).
//
// Giro 4, fase 4.1b: anche il riepilogo di «Conferma Fascicolo» (il cassetto «Rivedi», con htmx e senza), che
// sceglie i percorsi sul NAS come la conferma ma senza lucchetto e senza riservarli. Perche' calcoli davvero dei
// percorsi, la scena ha due file pronti: il disegno di 7120020 (un componente senza disegni) e un capitolato.
//
// Riscritta per lo Smistamento (giro 4, fase 4.17a): prima fissava che le GET le facessero soltanto
// l'operatore e la consultazione, tutte con 200 dove la pagina c'e'. Adesso ci sono anche la pagina «Forme
// viste» dell'Anagrafica e il suo export Markdown, che sono solo dell'amministratore: per l'operatore e la
// consultazione rispondono 403, per l'amministratore 200. Per questo le GET le fa anche l'amministratore,
// tutte: le asserzioni di prima restano, e per lui valgono le stesse.
func TestNessunaGetScrive(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaSenzaGet(t, "GET98")
	s.componente("7120020")
	s.propostaDa("7120020.pdf", "7120020")
	capitolato, _ := s.propostaDa("Capitolato ACME.pdf", "")
	s.esegui(`UPDATE documento_proposta SET tipo_proposto = 'capitolato', fonte = 'operatore', confidenza = 100 WHERE proposta_id = $1`, capitolato)
	base := s.base()
	distinta := "/thread/" + s.thread.String() + "/distinta"
	op := operatore(b)
	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")

	get := []struct {
		percorso  string
		hx        bool // richiesta htmx dalla schermata del Fascicolo
		pagina    bool // deve rispondere 200
		soloAdmin bool // dell'amministratore: 200 per lui, 403 per gli altri
	}{
		{"/thread/" + s.thread.String(), false, true, false},
		{base, false, true, false},
		{base + "?vista=bom", false, true, false},
		{base + "?vista=completezza", false, true, false},
		{base + "?vista=file", false, true, false},
		{base + "?vista=componenti", false, true, false},
		{base + "?vista=albero", false, true, false},
		{base + "?cassetto=verifica", false, true, false},
		{base + "?cassetto=piano", false, true, false},
		{base + "?cassetto=codici", false, true, false},
		{base + "?cassetto=avvisi", false, true, false},
		{base + "?nodo=" + s.pezzo.String(), false, true, false},
		{base + "?file=" + s.letto.String(), false, true, false},
		{base + "/parti?vista=bom", true, true, false},
		{base + "/parti?cassetto=verifica", true, true, false},
		{base + "/parti?cassetto=piano", true, true, false},
		{base + "/parti?nodo=" + s.pezzo.String(), true, true, false},
		{base + "/anteprima?nodo=" + s.pezzo.String(), true, true, false},
		{base + "/anteprima?file=" + s.letto.String(), true, true, false},
		{base + "/anteprima?file=" + s.pdfFermo.String(), true, true, false},
		{base + "/vista?q=7120", true, true, false},
		{base + "/avanzamento?firma=0&giro=0", true, true, false},
		{base + "/sezione?nodo=" + s.pezzo.String(), true, true, false},
		{base + "/bom/dati", true, true, false},
		{base + "/bom/dati?step=" + s.letto.String(), true, true, false},
		// Smistamento F2: il controllo del codice che l'operatore scrive nell'editor (U4) legge soltanto
		{base + "/bom/codice?codice=7120001A", true, true, false},
		{base + "/bom/codice?codice=7120099", true, true, false},
		// Smistamento, fase T: la tendina e l'anteprima del tipo, l'anteprima della riattivazione
		{base + "/componente/" + s.pezzo.String() + "/tipo", true, true, false},
		{base + "/componente/" + s.pezzo.String() + "/tipo?tipo=commerciale", true, true, false},
		{base + "/componente/" + s.pezzo.String() + "/step-strutturale/riattiva", true, true, false},
		{base + "/nas", true, false, false},
		{"/allegato/" + s.disegno.String() + "/anteprima", false, false, false},
		{"/richieste", false, true, false},
		{"/richieste/" + s.thread.String() + "/prodotti", true, false, false},
		// Smistamento, giro 4, fase 4.17a: le forme viste e il loro export
		{"/admin/anagrafica/forme", false, true, true},
		{"/admin/anagrafica/forme.md", false, true, true},
		// Giro 4, fase 4.3: le tre GET della Distinta, la pagina con cui si apre la RFQ (PR #8), anche in ogni passo
		// e con un passo che non c'e'; i dati per distinta.mjs; l'anteprima del tipo, un tipo che si fa e uno spento
		{distinta, false, true, false},
		{distinta + "?passo=richiesta", false, true, false},
		{distinta + "?passo=distinta", false, true, false},
		{distinta + "?passo=documenti", false, true, false},
		{distinta + "?passo=fattibilita", false, true, false},
		{distinta + "?passo=boh", false, true, false},
		{distinta + "/dati", false, true, false},
		{distinta + "/tipo?componente=" + s.pezzo.String() + "&tipo=commerciale", false, true, false},
		{distinta + "/tipo?componente=" + s.pezzo.String() + "&tipo=finito", false, true, false},
	}
	prima := fotoDelDatabase(t, b)
	// il riepilogo c'e' davvero, con i percorsi che la conferma darebbe (e chi consulta lo vede senza il modulo)
	for _, chi := range []struct {
		nome   string
		w      *browser
		modulo bool
	}{{"operatore", op, true}, {"consultazione", co, false}} {
		_, html := chi.w.fai(http.MethodGet, base+"?cassetto=piano", nil, false)
		for _, c := range []string{`ELENCO DISEGNI\7120020\7120020_REV_ND.pdf`, `CAPITOLATI\Capitolato ACME.pdf`} {
			if !strings.Contains(leggibile(html), c) {
				t.Errorf("%s: il riepilogo non mostra %s", chi.nome, c)
			}
		}
		if strings.Contains(html, "Conferma e copia sul NAS</button>") != chi.modulo {
			t.Errorf("%s: il modulo della conferma nel riepilogo: %v", chi.nome, !chi.modulo)
		}
	}
	for _, chi := range []struct {
		nome  string
		w     *browser
		admin bool
	}{{"operatore", op, false}, {"consultazione", co, false}, {"amministratore", ad, true}} {
		for _, g := range get {
			var resp *http.Response
			if g.hx {
				resp, _ = chi.w.daFascicolo(http.MethodGet, g.percorso, nil, s.thread, "")
			} else {
				resp, _ = chi.w.fai(http.MethodGet, g.percorso, nil, false)
			}
			negata := g.soloAdmin && !chi.admin
			if resp.StatusCode >= 500 || (g.pagina && !negata && resp.StatusCode != 200) || (negata && resp.StatusCode != http.StatusForbidden) {
				t.Errorf("%s, GET %s: %d", chi.nome, g.percorso, resp.StatusCode)
			}
		}
		if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
			t.Errorf("le GET di %s hanno scritto in: %s", chi.nome, d)
		}
	}

	if resp, _ := op.daFascicolo(http.MethodPost, base+"/prepara", url.Values{"auto": {"1"}}, s.thread, ""); resp.StatusCode != 200 {
		t.Fatalf("la preparazione: %d", resp.StatusCode)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); !strings.Contains(d, "job") {
		t.Fatalf("la preparazione doveva accodare lavoro (tabelle cambiate: %q): la scena non prova niente", d)
	}
}

// Prova 99 (R8, D52): la preparazione accoda soltanto, e chi consulta non la fa. La consultazione riceve 403
// in ogni forma (automatica, bottone, senza htmx), le sue pagine non la chiedono, e il database non cambia.
// L'operatore la manda: il disegno scende, le analisi mancanti si accodano (il PDF fermo e lo STEP senza i
// fatti correnti), lo STEP fermo con i fatti gia' calcolati si completa; ma lo STEP gia' letto non si
// rilegge (niente proposte nuove) e il codice della richiesta non diventa un prodotto. Una seconda
// preparazione non raddoppia niente; quella automatica senza niente da fare risponde 204; senza htmx il
// bottone di riserva torna alla pagina con un 303.
func TestPreparaAccodaSoltantoEChiConsultaNonLaFa(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaSenzaGet(t, "PREP99")
	base := s.base()
	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")

	prima := fotoDelDatabase(t, b)
	for _, pagina := range []string{"/thread/" + s.thread.String(), base} {
		if _, html := co.fai(http.MethodGet, pagina, nil, false); strings.Contains(html, "/fascicolo/prepara") {
			t.Errorf("%s: la pagina di chi consulta chiede la preparazione o ha il bottone", pagina)
		}
	}
	for _, form := range []url.Values{{"auto": {"1"}}, {"da": {"fascicolo"}}} {
		if resp, _ := co.daFascicolo(http.MethodPost, base+"/prepara", form, s.thread, ""); resp.StatusCode != http.StatusForbidden {
			t.Errorf("consultazione, prepara %v: %d, atteso 403", form, resp.StatusCode)
		}
	}
	if resp, _ := co.fai(http.MethodPost, base+"/prepara", url.Values{"da": {"fascicolo"}}, false); resp.StatusCode != http.StatusForbidden {
		t.Errorf("consultazione, prepara senza htmx: %d, atteso 403", resp.StatusCode)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Fatalf("la consultazione ha scritto in: %s", d)
	}

	op := operatore(b)
	resp, html := op.daFascicolo(http.MethodPost, base+"/prepara", url.Values{}, s.thread, "")
	if resp.StatusCode != 200 {
		t.Fatalf("prepara: %d", resp.StatusCode)
	}
	if a := avvisoF(html); a != "Preparazione dei file: 1 download accodato, 2 analisi accodate, 1 file fermo completato con i fatti già calcolati." {
		t.Errorf("avviso: %q", a)
	}
	if !strings.Contains(html, `hx-trigger="every 2s"`) {
		t.Error("la risposta fa partire il poll dell'avanzamento")
	}
	if n := s.conta(`SELECT count(*) FROM job WHERE tipo = 'stage_allegato'`); n != 1 {
		t.Errorf("download: %d", n)
	}
	if n := s.conta(`SELECT count(*) FROM job j JOIN allegato a ON a.sha256 = j.payload ->> 'sha256'
		WHERE j.tipo = 'analizza_allegato' AND a.allegato_id IN ($1, $2)`, s.vecchio, s.pdfFermo); n != 2 {
		t.Errorf("analisi accodate per lo STEP vecchio e il PDF fermo: %d, attese 2", n)
	}
	if n := s.conta(`SELECT count(*) FROM job WHERE tipo = 'analizza_allegato'`); n != 2 {
		t.Errorf("analisi accodate in tutto: %d, attese 2", n)
	}
	if n := s.conta(`SELECT count(*) FROM componente_proposta WHERE allegato_id = $1`, s.letto); n != 0 {
		t.Errorf("la preparazione ha riletto lo STEP gia' analizzato: %d proposte", n)
	}
	if n := s.conta(`SELECT count(*) FROM componente WHERE thread_id = $1 AND codice = '7120001'`, s.thread); n != 0 {
		t.Errorf("la preparazione ha fatto il prodotto della richiesta: %d", n)
	}
	if got := s.conta(`SELECT count(*) FROM allegato WHERE allegato_id = $1 AND stato = 'in_staging'`, s.stepFermo); got != 0 {
		t.Error("lo STEP fermo con i fatti gia' calcolati doveva essere completato")
	}

	for _, c := range []struct {
		form  url.Values
		torna string
	}{{url.Values{"da": {"fascicolo"}}, base}, {url.Values{}, "/thread/" + s.thread.String()}} {
		resp, _ := op.fai(http.MethodPost, base+"/prepara", c.form, false)
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != c.torna {
			t.Errorf("senza htmx %v: %d → %q, atteso 303 → %q", c.form, resp.StatusCode, resp.Header.Get("Location"), c.torna)
		}
	}
	_, html = op.daFascicolo(http.MethodPost, base+"/prepara", url.Values{}, s.thread, "")
	if a := avvisoF(html); a != "Preparazione dei file: 1 analisi già in coda." {
		t.Errorf("seconda preparazione: %q", a)
	}
	if resp, _ := op.daFascicolo(http.MethodPost, base+"/prepara", url.Values{"auto": {"1"}}, s.thread, ""); resp.StatusCode != http.StatusNoContent {
		t.Errorf("preparazione automatica senza niente da fare: %d, atteso 204", resp.StatusCode)
	}
	if n := s.conta(`SELECT count(*) FROM job WHERE tipo IN ('stage_allegato', 'analizza_allegato')`); n != 3 {
		t.Errorf("le preparazioni ripetute hanno accodato di nuovo: %d job", n)
	}
}

// Prova 99, seconda parte (R8, D52): la preparazione parte senza che nessuno la chieda, quindi non insiste.
// Uno STEP la cui analisi con l'analizzatore corrente e' fallita non si riaccoda aprendo la pagina, ne' la
// prima volta ne' la seconda: lo guarda una persona. Uno STEP fermo nello staging si accoda una volta e si
// conta una volta (non «accodata» e «gia' in coda» nello stesso avviso). «Rianalizza», che e' il gesto di una
// persona, riprova quello fallito.
func TestLaPreparazioneNonRiprovaDaSolaUnAnalisiFallita(t *testing.T) {
	b := preparaBancoWeb(t)
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "PREP99F")
	fallito := r.stepNellaRfq("7120011.stp", "", an)
	fermo := r.stepNellaRfq("7120012.stp", "", an)
	r.esegui(`UPDATE allegato SET stato = 'in_staging' WHERE allegato_id = $1`, fermo)
	a, err := b.q.GetAllegato(b.ctx, fallito)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coda.AccodaAnalisi(b.ctx, b.q, a, uuid.NullUUID{UUID: r.thread, Valid: true}, an); err != nil {
		t.Fatal(err)
	}
	chiave := coda.ChiaveAnalisi(a.Sha256.String, an)
	r.esegui(`UPDATE job SET stato = 'fallito', chiuso_il = now() WHERE chiave_idempotenza = $1`, chiave)

	w := operatore(b)
	for i, atteso := range []string{"Preparazione dei file: 1 analisi accodata.",
		"Niente da preparare: nessun file da scaricare, estrarre o analizzare."} {
		_, html := w.daFascicolo(http.MethodPost, r.base()+"/prepara", url.Values{}, r.thread, "")
		if a := avvisoF(html); a != atteso {
			t.Errorf("preparazione %d: %q, atteso %q", i+1, a, atteso)
		}
		if n := r.conta(`SELECT count(*) FROM job WHERE chiave_idempotenza = $1`, chiave); n != 1 {
			t.Errorf("preparazione %d: l'analisi fallita ha %d job, la preparazione non la riprova", i+1, n)
		}
		if n := r.conta(`SELECT count(*) FROM job j JOIN allegato a ON a.sha256 = j.payload ->> 'sha256'
			WHERE j.tipo = 'analizza_allegato' AND j.stato = 'pronto' AND a.allegato_id = $1`, fermo); n != 1 {
			t.Errorf("preparazione %d: analisi dello STEP fermo in coda: %d, attesa 1", i+1, n)
		}
	}

	_, html := w.fai(http.MethodPost, r.base()+"/rianalizza", url.Values{}, true)
	if a := avvisoDi(html); a != "1 analisi accodata, 1 analisi già in coda." {
		t.Errorf("rianalizza: %q", a)
	}
	if n := r.conta(`SELECT count(*) FROM job WHERE chiave_idempotenza = $1 AND stato = 'pronto'`, chiave); n != 1 {
		t.Errorf("«Rianalizza» riprova l'analisi fallita: %d job pronti", n)
	}
}

// Prova 100 (C6): creare la RFQ o agganciarle un messaggio rilegge gli STEP gia' analizzati. Lo STEP del
// caso guida e' sceso e analizzato quando il messaggio non era ancora in nessuna RFQ (D30): il risultato
// dell'analisi non ha trovato RFQ in cui scrivere, e aprire le pagine non lo rilegge piu'. La decisione
// della persona si': le sue proposte nascono nella RFQ, con il prodotto della richiesta, senza accodare
// un'altra analisi.
//
// Riscritta per lo Smistamento (scelta 6, 27/09): prima l'aggancio senza nessun codice spuntato faceva il
// prodotto 7120001, confermato nella richiesta da prima. Adesso l'aggancio crea solo i codici confermati nel
// suo gesto: chi aggancia spunta 7120001, e il prodotto nasce accanto alle proposte dello STEP come prima.
// Che un aggancio senza codici non faccia rinascere un prodotto lo fissa TestUnProdottoToltoNonRinasceAllAggancio.
func TestAgganciareRileggeGliStepGiaAnalizzati(t *testing.T) {
	// stepAnalizzato: una mail di ACME con lo STEP del caso guida, sceso e analizzato fuori da ogni RFQ.
	stepAnalizzato := func(t *testing.T, b *bancoWeb) (msg, stp uuid.UUID) {
		t.Helper()
		an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
		b.ws.Analizzatore = an
		t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
		msg = b.mailConAllegati("RFQ 7120001", "Richiesta di offerta per 7120001, in allegato lo STEP.",
			worker.AllegatoIn{Indice: 1, NomeFile: "7120001A_1.stp", Estensione: "stp", Natura: "file", Bytes: 9000})
		stp = uuidSQL(t, b, `SELECT allegato_id FROM allegato WHERE messaggio_id = $1`, msg)
		sha := fmt.Sprintf("%064x", 7120001)
		if _, err := b.pool.Exec(b.ctx, `UPDATE allegato SET sha256 = $2, path_staging = $3, stato = 'analizzato' WHERE allegato_id = $1`,
			stp, sha, filepath.Join(t.TempDir(), sha+".stp")); err != nil {
			t.Fatal(err)
		}
		if _, err := b.pool.Exec(b.ctx, `INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, $2, $3, $4)`,
			sha, an.Versione, an.Hash(), fattiCasoGuida); err != nil {
			t.Fatal(err)
		}
		if n := contaSQL(t, b, `SELECT count(*) FROM componente_proposta`); n != 0 {
			t.Fatalf("prima della decisione ci sono %d proposte di nodo", n)
		}
		return msg, stp
	}
	// riletto controlla che lo STEP abbia le sue proposte nella RFQ, accanto al prodotto, e nessuna analisi
	// in piu'.
	riletto := func(t *testing.T, b *bancoWeb, html string, thread, stp uuid.UUID) {
		t.Helper()
		if a := leggibile(html); !strings.Contains(a, "1 STEP già analizzato letto in questa RFQ") || !strings.Contains(a, "7120001 nella BOM come prodotto finito") {
			t.Errorf("avviso: %q", estrai(html, "avviso"))
		}
		if n := contaSQL(t, b, `SELECT count(*) FROM componente_proposta WHERE thread_id = $1 AND allegato_id = $2`, thread, stp); n != 3 {
			t.Errorf("proposte di nodo dello STEP nella RFQ: %d, attese 3", n)
		}
		if n := contaSQL(t, b, `SELECT count(*) FROM relazione_proposta WHERE thread_id = $1 AND allegato_id = $2`, thread, stp); n != 2 {
			t.Errorf("proposte di arco dello STEP nella RFQ: %d, attese 2", n)
		}
		if n := contaSQL(t, b, `SELECT count(*) FROM componente WHERE thread_id = $1 AND codice = '7120001' AND tipo = 'finito'`, thread); n != 1 {
			t.Errorf("il prodotto della richiesta: %d", n)
		}
		if n := contaSQL(t, b, `SELECT count(*) FROM job WHERE tipo = 'analizza_allegato'`); n != 0 {
			t.Errorf("rileggere non accoda analisi: %d", n)
		}
	}

	t.Run("aggancio", func(t *testing.T) {
		b := preparaBancoWeb(t)
		acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
		thread := b.rfqDa(uuid.Nil, acme, `ACME\WIP\2026 09 26 RFQ 7120001`, "7120001")
		if _, err := b.pool.Exec(b.ctx, `UPDATE identificativo_thread SET confermato_da = (SELECT utente_id FROM utente WHERE sigla = 'FP') WHERE thread_id = $1`, thread); err != nil {
			t.Fatal(err)
		}
		msg, stp := stepAnalizzato(t, b)
		_, html := operatore(b).fai(http.MethodPost, "/messaggio/"+msg.String()+"/aggancia", url.Values{"thread_id": {thread.String()}, "codice": {"7120001"}}, true)
		if !strings.Contains(leggibile(html), "Agganciato") {
			t.Fatalf("aggancio: %q", estrai(html, "avviso"))
		}
		riletto(t, b, html, thread, stp)
	})
	t.Run("creazione", func(t *testing.T) {
		b := preparaBancoWeb(t)
		acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
		msg, stp := stepAnalizzato(t, b)
		_, html := operatore(b).fai(http.MethodPost, "/messaggio/"+msg.String()+"/rfq", url.Values{"cliente_id": {acme.String()},
			"oggetto": {"Supporto 7120001"}, "codice": {"7120001"}, "priorita": {"1"}}, true)
		if !strings.Contains(leggibile(html), "RFQ creata") {
			t.Fatalf("creazione: %q", estrai(html, "avviso"))
		}
		thread := uuidSQL(t, b, `SELECT thread_id FROM messaggio WHERE messaggio_id = $1`, msg)
		riletto(t, b, html, thread, stp)
	})
}
