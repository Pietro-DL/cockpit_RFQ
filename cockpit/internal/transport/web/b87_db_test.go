//go:build integrazione

// L4 — B8.7, la schermata del Fascicolo e i suoi gesti. Per ogni rotta nuova: com'era prima, com'e'
// dopo, il rifiuto con le sue parole, e che un rifiuto non lasci niente a meta' (la transazione). Poi le
// regole di A4 viste dalla schermata: D25c senza preselezione, la BOM congelata che si legge e non si
// cambia (ma accetta caricamenti e proposte), il caricamento interno che fa la strada di tutti gli
// allegati e non inventa una revisione del cliente, la sostituzione con il motivo e con la domanda
// esplicita sullo STEP strutturale. E la risposta fuori banda: struttura, documenti e completezza si
// rifanno, l'anteprima no.

package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
	"promatec/cockpit/internal/transport/workerapi"
)

// scenaB87 e' la RFQ delle prove: in FATTIBILITA, con una BOM piccola che ha i casi della schermata. Un
// prodotto con il suo 2D e il suo STEP, un assieme sotto il prodotto (×2), un particolare sotto l'assieme
// (×4) e anche sotto il prodotto (×1): condiviso. Due PDF arrivati e non ancora assegnati.
type scenaB87 struct {
	*rfqFascicolo
	prodotto, assieme, particolare uuid.UUID
	disegno, step                  uuid.UUID // documenti del prodotto
	pdfLibero, pdfLibero2          uuid.UUID // proposte aperte, senza componente
	allDisegno, allStep, allLibero uuid.UUID // i loro allegati
}

func (b *bancoWeb) scenaB87(chiave string) *scenaB87 {
	b.t.Helper()
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), chiave)
	s := &scenaB87{rfqFascicolo: r}
	r.fase("FATTIBILITA")
	s.prodotto = r.componenteTipo("52922757", "finito")
	s.assieme = r.componenteTipo("52920517", "sottoassieme")
	s.particolare = r.componenteTipo("53017189", "sciolto")
	r.arco(s.prodotto, s.assieme, 2)
	r.arco(s.assieme, s.particolare, 4)
	r.arco(s.prodotto, s.particolare, 1)
	s.disegno, s.allDisegno = r.documentoDa("52922757 foglio 1.pdf", "pdf", db.TipoDocumentoDisegno2d, "52922757", s.prodotto, db.StatoNasScritto)
	s.step, s.allStep = r.documentoDa("52922757.stp", "stp", db.TipoDocumentoCad3d, "52922757", s.prodotto, db.StatoNasScritto)
	s.pdfLibero, s.allLibero = r.propostaDa("53017189.pdf", "53017189")
	s.pdfLibero2, _ = r.propostaDa("52920517.pdf", "52920517")
	return s
}

// fase apre la fase data, chiudendo quella aperta.
func (r *rfqFascicolo) fase(nome string) {
	r.b.t.Helper()
	r.esegui(`UPDATE fase_log SET fine = now(), esito = 'OK' WHERE thread_id = $1 AND fine IS NULL`, r.thread)
	r.esegui(`INSERT INTO fase_log (thread_id, nome_fase, inizio) VALUES ($1, $2, now())`, r.thread, nome)
}

func (r *rfqFascicolo) componenteTipo(codice, tipo string) uuid.UUID {
	r.b.t.Helper()
	var id uuid.UUID
	if err := r.b.pool.QueryRow(r.b.ctx, `INSERT INTO componente (thread_id, codice, tipo, confermato_da)
		VALUES ($1, $2, $3, $4) RETURNING componente_id`, r.thread, codice, tipo, r.utente).Scan(&id); err != nil {
		r.b.t.Fatal(err)
	}
	return id
}

func (r *rfqFascicolo) arco(padre, figlio uuid.UUID, qta int) {
	r.b.t.Helper()
	r.esegui(`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ($1,$2,$3,$4,'manuale',$5)`,
		r.thread, padre, figlio, qta, r.utente)
}

// allegatoExt e' come allegato, con l'estensione vera del file.
func (r *rfqFascicolo) allegatoExt(nome, ext string) (uuid.UUID, string) {
	r.b.t.Helper()
	id, sha := r.allegato(nome)
	r.esegui(`UPDATE allegato SET estensione = $2 WHERE allegato_id = $1`, id, ext)
	return id, sha
}

// documentoDa e' un documento confermato da un allegato della RFQ, con la sua provenienza: la riga del
// pannello Documenti lo trova dal file.
func (r *rfqFascicolo) documentoDa(nome, ext string, tipo db.TipoDocumento, codice string, comp uuid.UUID, stato db.StatoNas) (uuid.UUID, uuid.UUID) {
	r.b.t.Helper()
	a, sha := r.allegatoExt(nome, ext)
	var id uuid.UUID
	cid := uuid.NullUUID{UUID: comp, Valid: comp != uuid.Nil}
	if err := r.b.pool.QueryRow(r.b.ctx, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione,
		sha256, bytes, path_relativo, stato_nas, confermato_da)
		VALUES ($1,$2,$3,$4,$5,$6,$7,10,$8,$9,$10) RETURNING documento_id`,
		r.thread, cid, tipo, codice, nome, ext, sha, `DOC\`+strings.ToUpper(codice)+`\`+nome, stato, r.utente).Scan(&id); err != nil {
		r.b.t.Fatal(err)
	}
	r.esegui(`INSERT INTO documento_provenienza (documento_id, allegato_id, messaggio_id, ricevuto_il) VALUES ($1,$2,$3, now())`, id, a, r.msg)
	r.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte, stato)
		VALUES ($1,$2,$3,$4,90,'nome_file','confermata')`, a, r.thread, tipo, codice)
	return id, a
}

// propostaDa e' un PDF arrivato e non ancora confermato, gia' letto come disegno.
func (r *rfqFascicolo) propostaDa(nome, codice string) (uuid.UUID, uuid.UUID) {
	r.b.t.Helper()
	a, _ := r.allegatoExt(nome, "pdf")
	var id uuid.UUID
	if err := r.b.pool.QueryRow(r.b.ctx, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte)
		VALUES ($1,$2,'disegno_2d',NULLIF($3,''),85,'cartiglio') RETURNING proposta_id`, a, r.thread, codice).Scan(&id); err != nil {
		r.b.t.Fatal(err)
	}
	return id, a
}

func (r *rfqFascicolo) base() string { return "/thread/" + r.thread.String() + "/fascicolo" }

// daFascicolo e' una richiesta htmx fatta dalla schermata del Fascicolo: HX-Current-URL dice da dove, con
// il suo stato, come lo manda htmx.
func (w *browser) daFascicolo(metodo, percorso string, form url.Values, thread uuid.UUID, stato string) (*http.Response, string) {
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
	r.Header.Set("HX-Current-URL", w.b.srv.URL+"/thread/"+thread.String()+"/fascicolo"+stato)
	r.Header.Set("X-Prova-IP", w.ip)
	resp, err := w.c.Do(r)
	if err != nil {
		w.b.t.Fatal(err)
	}
	defer resp.Body.Close()
	testo, _ := io.ReadAll(resp.Body)
	return resp, string(testo)
}

// gesto e' un POST dalla schermata, e ne restituisce l'avviso.
func (s *scenaB87) gesto(w *browser, percorso string, form url.Values) string {
	s.b.t.Helper()
	if form == nil {
		form = url.Values{}
	}
	_, html := w.daFascicolo(http.MethodPost, percorso, form, s.thread, "")
	return avvisoF(html)
}

func (s *scenaB87) comp(cid uuid.UUID, gesto string) string {
	return s.base() + "/componente/" + cid.String() + "/" + gesto
}

var reAvvisoF = regexp.MustCompile(`(?s)<div class="avviso-f[^"]*" role="status">(.*?)</div>`)

// avvisoF e' l'avviso della schermata del Fascicolo, come lo legge l'operatore.
func avvisoF(html string) string {
	if m := reAvvisoF.FindStringSubmatch(html); m != nil {
		return leggibile(m[1])
	}
	return ""
}

func (s *scenaB87) archi() string {
	s.b.t.Helper()
	var out string
	if err := s.b.pool.QueryRow(s.b.ctx, `SELECT COALESCE(string_agg(p.codice || '>' || f.codice || 'x' || r.qta, ' ' ORDER BY p.codice, f.codice), '')
		FROM componente_relazione r JOIN componente p ON p.componente_id = r.padre_id JOIN componente f ON f.componente_id = r.figlio_id
		WHERE r.thread_id = $1`, s.thread).Scan(&out); err != nil {
		s.b.t.Fatal(err)
	}
	return out
}

func (s *scenaB87) valore(sql string, arg ...any) string {
	s.b.t.Helper()
	var v *string
	if err := s.b.pool.QueryRow(s.b.ctx, sql, arg...).Scan(&v); err != nil {
		s.b.t.Fatalf("%v\n%s", err, sql)
	}
	if v == nil {
		return "NULL"
	}
	return *v
}

// ---------------------------------------------------------------- la pagina

// La schermata si apre sulla vista Documenti (Fascicolo v3), con il piano e il cassetto; la Struttura BOM ha
// la BOM visuale e il dettaglio (B8.7b), e mette il particolare sotto tutti e due i padri; l'Elenco file parte
// dai file non assegnati, e il file picker non e' sempre davanti: sta in «Aggiungi file». Chi consulta la
// vede senza gesti, e se prova a scrivere il server rifiuta.
func TestLaSchermataDelFascicoloSiApreConTuttiIPannelli(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("PAG87")
	w := operatore(b)
	b.caricamentoAcceso(t)

	resp, html := w.fai(http.MethodGet, s.base(), nil, false)
	if resp.StatusCode != 200 {
		t.Fatalf("GET: %d", resp.StatusCode)
	}
	for _, atteso := range []string{`id="fasc-testata"`, `id="vista"`, `id="doc-vista"`, `id="doc-stage" class="doc-stage" hx-preserve="true"`,
		`id="doc-film"`, `id="doc-indice"`, `id="doc-sezione"`, `id="piano"`, `id="cassetto"`, `id="fasc-avanzamento-box"`,
		`data-vista="documenti">Documenti<`, `data-vista="bom">Struttura BOM<`, `data-vista="completezza">Completezza`,
		">Elenco file <", ">Componenti <", ">Albero<", ">Codici<", "Avvisi", `data-k="c:` + s.prodotto.String() + `"`,
		`data-k="c:` + s.particolare.String() + `"`, `data-scrive="1"`, "Congela…", "Carica dal PC", "Conferma Fascicolo"} {
		if !strings.Contains(html, atteso) {
			t.Errorf("la pagina non ha %q", atteso)
		}
	}
	for _, vietato := range []string{`id="tela"`, "<iframe", `class="doc-tabella"`} {
		if strings.Contains(html, vietato) {
			t.Errorf("la vista Documenti ha %q", vietato)
		}
	}
	if strings.Count(html, `type="file"`) != 1 {
		t.Error("il file picker sta solo in «Carica dal PC»")
	}
	_, html = w.fai(http.MethodGet, s.base()+"?vista=bom", nil, false)
	for _, atteso := range []string{`id="tela"`, `id="anteprima"`, `id="anteprima-corpo"`, `id="nodo-` + s.prodotto.String() + `"`,
		`id="nodo-` + s.particolare.String() + `-` + s.prodotto.String() + `"`, `id="nodo-` + s.particolare.String() + `-` + s.assieme.String() + `"`,
		"condiviso", `data-editor="` + s.prodotto.String() + `"`, "Modifica la struttura di 52922757"} {
		if !strings.Contains(html, atteso) {
			t.Errorf("la Struttura BOM non ha %q", atteso)
		}
	}
	if strings.Contains(html, `class="doc-tabella"`) || strings.Count(html, `type="file"`) != 1 {
		t.Error("la tabella dei documenti e' una vista a parte, e il file picker sta solo in «Carica dal PC»")
	}
	_, html = w.fai(http.MethodGet, s.base()+"?vista=documenti", nil, false)
	for _, atteso := range []string{`class="doc-tabella"`, "53017189.pdf", "52920517.pdf"} {
		if !strings.Contains(html, atteso) {
			t.Errorf("la vista Documenti non ha %q", atteso)
		}
	}
	if strings.Contains(html, "52922757 foglio 1.pdf") {
		t.Error("il filtro predefinito e' «non assegnati»: il 2D del prodotto non ci sta")
	}
	if strings.Contains(html, "sola-lettura") {
		t.Error("l'operatore non e' in sola lettura")
	}

	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")
	resp, html = co.fai(http.MethodGet, s.base(), nil, false)
	if resp.StatusCode != 200 || !strings.Contains(html, `class="fasc sola-lettura"`) || !strings.Contains(html, `data-scrive=""`) {
		t.Fatalf("consultazione: %d, sola lettura %v", resp.StatusCode, strings.Contains(html, "sola-lettura"))
	}
	_, html = co.fai(http.MethodGet, s.base()+"?vista=bom", nil, false)
	if strings.Contains(html, "Modifica la struttura") || strings.Contains(html, "data-editor") {
		t.Error("chi consulta non apre l'editor della struttura")
	}
	resp, _ = co.daFascicolo(http.MethodPost, s.comp(s.assieme, "archivia"), url.Values{"motivo": {"prova"}}, s.thread, "")
	if resp.StatusCode < 400 {
		t.Errorf("consultazione: un POST deve essere rifiutato, stato %d", resp.StatusCode)
	}
	if n := s.conta(`SELECT count(*) FROM componente WHERE thread_id = $1 AND archiviato_il IS NOT NULL`, s.thread); n != 0 {
		t.Errorf("la consultazione ha archiviato %d componenti", n)
	}
}

// Un gesto dalla schermata risponde con l'avviso e con i pannelli fuori banda; il corpo dell'anteprima
// non c'e', quindi il PDF aperto resta aperto. Lo stesso gesto da altrove risponde con la pagina della RFQ.
// Dalla vista Documenti (v3) lo stage del disegno torna vuoto con hx-preserve: il browser tiene il suo.
func TestUnGestoDalFascicoloRifaIPannelliSenzaToccareLAnteprima(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("OOB87")
	w := operatore(b)

	// dalla vista Documenti
	_, html := w.daFascicolo(http.MethodPost, s.comp(s.particolare, "modifica"), url.Values{"tipo": {"sciolto"}, "rev": {"A"}, "corpo_chiave": {"documenti"}},
		s.thread, "?nodo="+s.particolare.String())
	if a := avvisoF(html); a != "53017189: rev — → A." {
		t.Fatalf("avviso dalla vista Documenti: %q", a)
	}
	if !strings.Contains(html, `id="vista" hx-swap-oob="innerHTML"`) || strings.Count(html, `id="doc-stage" class="doc-stage" hx-preserve="true"`) != 1 {
		t.Error("dalla vista Documenti: l'area principale fuori banda, con lo stage preservato")
	}
	if strings.Contains(html, "<iframe") || strings.Contains(html, `id="anteprima-corpo"`) || strings.Contains(html, "<canvas") {
		t.Error("dalla vista Documenti la risposta non porta un disegno")
	}
	if !strings.Contains(html, `class="docv-riga sel" role="treeitem" data-k="c:`+s.particolare.String()+`"`) {
		t.Error("dalla vista Documenti: il componente scelto nell'indirizzo resta scelto")
	}

	stato := "?vista=bom&file=" + s.allLibero.String() + "&nodo=" + s.particolare.String()
	// la pagina rimanda con ogni richiesta che cosa mostra il corpo del pannello di destra (hx-include)
	_, pagina := w.fai(http.MethodGet, s.base()+stato, nil, false)
	chiave := chiaveCorpoDi(t, pagina)
	_, html = w.daFascicolo(http.MethodPost, s.comp(s.particolare, "modifica"), url.Values{"tipo": {"sciolto"}, "rev": {"B"}, "corpo_chiave": {chiave}}, s.thread, stato)
	if a := avvisoF(html); a != "53017189: rev A → B." {
		t.Fatalf("avviso: %q", a)
	}
	for _, id := range []string{"fasc-testata", "fasc-avanzamento-box", "vista", "piano", "cassetto", "anteprima-testa"} {
		if !strings.Contains(html, `id="`+id+`" hx-swap-oob="innerHTML"`) {
			t.Errorf("manca il pannello fuori banda %s", id)
		}
	}
	for _, vietato := range []string{`id="anteprima-corpo"`, "<iframe", `id="anteprima"`} {
		if strings.Contains(html, vietato) {
			t.Errorf("la risposta tocca l'anteprima: %q", vietato)
		}
	}
	// lo stato della pagina (il nodo scelto, il file aperto) si rilegge da HX-Current-URL
	if !strings.Contains(html, `class="carta ok sel`) && !strings.Contains(html, `sel" id="nodo-`+s.particolare.String()) {
		t.Error("il nodo scelto nell'indirizzo deve restare scelto nella BOM")
	}
	if !strings.Contains(estratto(html, `id="anteprima-testa"`), "53017189.pdf") {
		t.Error("l'intestazione dell'anteprima deve dire il file aperto")
	}

	_, html = w.fai(http.MethodPost, s.comp(s.particolare, "modifica"), url.Values{"tipo": {"sciolto"}, "rev": {"C"}}, true)
	if !strings.Contains(html, `class="thread-testata"`) || avvisoDi(html) != "53017189: rev B → C." {
		t.Errorf("senza HX-Current-URL la risposta e' la pagina della RFQ: %q", avvisoDi(html))
	}

	// B8.7b: una pagina che mostra un altro corpo (dopo una conferma il pannello passa da un file a un
	// componente) lo riceve rifatto fuori banda, con la chiave nuova
	_, html = w.daFascicolo(http.MethodPost, s.comp(s.particolare, "modifica"), url.Values{"tipo": {"sciolto"}, "rev": {"D"}, "corpo_chiave": {"vuoto"}}, s.thread, stato)
	if a := avvisoF(html); a != "53017189: rev C → D." {
		t.Fatalf("avviso: %q", a)
	}
	corpo := estratto(html, `id="anteprima-corpo" hx-swap-oob="innerHTML"`)
	if !strings.Contains(html, `id="anteprima-corpo" hx-swap-oob="innerHTML"`) || !strings.Contains(corpo, `value="`+chiave+`"`) || !strings.Contains(corpo, "<iframe") {
		t.Errorf("la pagina mostrava un altro corpo: la risposta lo rifa', con il PDF del file aperto:\n%.300s", corpo)
	}
}

// chiaveCorpoDi e' quello che la pagina rimanda con ogni richiesta (hx-include): che cosa mostra il corpo
// del pannello di destra.
func chiaveCorpoDi(t *testing.T, pagina string) string {
	t.Helper()
	m := regexp.MustCompile(`id="corpo-chiave" name="corpo_chiave" value="([^"]*)"`).FindStringSubmatch(pagina)
	if m == nil {
		t.Fatal("la pagina non dice che cosa mostra il pannello di destra (corpo-chiave)")
	}
	return html.UnescapeString(m[1])
}

// ---------------------------------------------------------------- struttura

// Tipo, revisione e descrizione; archi messi, tolti, spostati; un ciclo rifiutato; uno spostamento che non
// riesce a meta' non cambia niente.
func TestLaStrutturaSiCorreggeDallaSchermata(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("STR87")
	w := operatore(b)
	prima := "52920517>53017189x4 52922757>52920517x2 52922757>53017189x1"
	if s.archi() != prima {
		t.Fatalf("scena: %s", s.archi())
	}

	if a := s.gesto(w, s.comp(s.assieme, "modifica"), url.Values{"tipo": {"boh"}}); a != "Niente è cambiato: tipo di componente non valido" {
		t.Errorf("tipo non valido: %q", a)
	}
	if a := s.gesto(w, s.comp(s.assieme, "modifica"), url.Values{"tipo": {"sottoassieme"}, "rev": {"A B"}}); !strings.Contains(a, "revisione non valida") {
		t.Errorf("rev con lo spazio: %q", a)
	}
	if a := s.gesto(w, s.comp(s.assieme, "modifica"), url.Values{"tipo": {"sciolto"}, "rev": {"c"}, "descrizione": {"staffa"}}); a != "52920517: tipo assieme → particolare, rev — → C, descrizione." {
		t.Errorf("modifica: %q", a)
	}
	if got := s.valore(`SELECT tipo || '/' || rev || '/' || descrizione FROM componente WHERE componente_id = $1`, s.assieme); got != "sciolto/C/staffa" {
		t.Errorf("dopo la modifica: %s", got)
	}

	if a := s.gesto(w, s.comp(s.prodotto, "step-strutturale"), url.Values{"documento": {s.step.String()}}); !strings.Contains(a, "è lo STEP strutturale di 52922757") {
		t.Fatalf("STEP strutturale: %q", a)
	}
	if a := s.gesto(w, s.comp(s.prodotto, "modifica"), url.Values{"tipo": {"sottoassieme"}}); !strings.Contains(a, "ha uno STEP strutturale") {
		t.Errorf("un finito con lo STEP strutturale non cambia tipo: %q", a)
	}

	// un ciclo: l'assieme sotto il particolare, che sta sotto l'assieme
	if a := s.gesto(w, s.comp(s.assieme, "collega"), url.Values{"padre": {s.particolare.String()}, "qta": {"1"}}); !strings.Contains(a, "chiuderebbe un ciclo") {
		t.Errorf("ciclo: %q", a)
	}
	if a := s.gesto(w, s.comp(s.particolare, "collega"), url.Values{"padre": {s.assieme.String()}, "qta": {"0"}}); !strings.Contains(a, "la quantità va da 1") {
		t.Errorf("qta zero: %q", a)
	}
	if s.archi() != prima {
		t.Fatalf("i rifiuti hanno cambiato gli archi: %s", s.archi())
	}
	if a := s.gesto(w, s.comp(s.particolare, "collega"), url.Values{"padre": {s.assieme.String()}, "qta": {"6"}}); a != "53017189 sotto 52920517: quantità 4 → 6." {
		t.Errorf("quantita': %q", a)
	}

	// spostamento che fallisce nella seconda meta': l'assieme lascia il prodotto e va sotto il suo figlio
	if a := s.gesto(w, s.comp(s.assieme, "sposta"), url.Values{"da": {s.prodotto.String()}, "a": {s.particolare.String()}, "qta": {"1"}}); !strings.Contains(a, "chiuderebbe un ciclo") {
		t.Errorf("sposta con ciclo: %q", a)
	}
	if got := s.archi(); got != "52920517>53017189x6 52922757>52920517x2 52922757>53017189x1" {
		t.Fatalf("uno spostamento rifiutato ha lasciato %s: la transazione doveva annullare anche lo scollegamento", got)
	}
	if a := s.gesto(w, s.comp(s.particolare, "sposta"), url.Values{"da": {s.assieme.String()}, "a": {s.prodotto.String()}, "qta": {"3"}}); a != "53017189 non è più sotto 52920517. 53017189 sotto 52922757: quantità 1 → 3." {
		t.Errorf("sposta: %q", a)
	}
	if a := s.gesto(w, s.comp(s.particolare, "scollega"), url.Values{"padre": {s.assieme.String()}}); a != "Niente è cambiato: 53017189 non è sotto 52920517" {
		t.Errorf("scollega un arco che non c'e': %q", a)
	}
	if a := s.gesto(w, s.comp(s.particolare, "sposta"), url.Values{"da": {s.prodotto.String()}, "a": {""}}); a != "53017189 non è più sotto 52922757. 53017189 è una radice." {
		t.Errorf("diventa radice: %q", a)
	}
	if got := s.archi(); got != "52922757>52920517x2" {
		t.Errorf("archi alla fine: %s", got)
	}
}

// Archiviare vuole il motivo e toglie gli archi; togliere riesce solo senza storia e, altrimenti, dice che
// cosa trattiene; ripristinare riporta lo stesso componente.
func TestArchiviareTogliereRipristinare(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("ARC87")
	w := operatore(b)

	if a := s.gesto(w, s.comp(s.assieme, "archivia"), url.Values{"motivo": {"  "}}); a != "Niente è cambiato: si archivia con un motivo" {
		t.Errorf("senza motivo: %q", a)
	}
	if a := s.gesto(w, s.comp(s.assieme, "archivia"), url.Values{"motivo": {"tolto dal cliente"}}); !strings.Contains(a, "52920517 archiviato: tolti 2 archi") {
		t.Fatalf("archivia: %q", a)
	}
	if got := s.archi(); got != "52922757>53017189x1" {
		t.Errorf("archi dopo l'archiviazione: %s", got)
	}
	if a := s.gesto(w, s.comp(s.prodotto, "rimuovi"), nil); !strings.Contains(a, "52922757 non si cancella: ha dei documenti. Si archivia") {
		t.Errorf("togliere un componente con documenti: %q", a)
	}
	nuovo := s.componenteTipo("99999999", "sciolto")
	if a := s.gesto(w, s.comp(nuovo, "rimuovi"), nil); a != "99999999 tolto: non aveva storia." {
		t.Errorf("togliere senza storia: %q", a)
	}
	if n := s.conta(`SELECT count(*) FROM componente WHERE componente_id = $1`, nuovo); n != 0 {
		t.Error("il componente senza storia doveva sparire")
	}
	_, html := w.daFascicolo(http.MethodGet, s.base()+"/parti?vista=bom", nil, s.thread, "?vista=bom")
	if !strings.Contains(html, "Archiviati (1)") || !strings.Contains(html, "/componente/"+s.assieme.String()+"/ripristina") {
		t.Error("la struttura deve mostrare l'archiviato con il suo «Ripristina»")
	}
	if a := s.gesto(w, s.comp(s.assieme, "ripristina"), nil); a != "52920517 ripristinato nella BOM working." {
		t.Errorf("ripristina: %q", a)
	}
}

// ---------------------------------------------------------------- deroghe e STEP strutturale

func TestDerogheESTEPStrutturaleDallaSchermata(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("DER87")
	w := operatore(b)
	esito := func(comp uuid.UUID, tipo string) string {
		return s.valore(`SELECT esito FROM v_fascicolo WHERE componente_id = $1 AND tipo_documento = $2`, comp, tipo)
	}
	// un particolare senza file: il suo 2D manca, ed e' bloccante
	vite := s.componenteTipo("54000000", "sciolto")
	if esito(vite, "disegno_2d") != "manca" {
		t.Fatalf("scena: il 2D del particolare %s", esito(vite, "disegno_2d"))
	}
	if a := s.gesto(w, s.comp(vite, "deroga"), url.Values{"tipo": {"disegno_2d"}}); a != "Niente è cambiato: una deroga si concede con il suo motivo" {
		t.Errorf("senza motivo: %q", a)
	}
	if a := s.gesto(w, s.comp(vite, "deroga"), url.Values{"tipo": {"commerciale"}, "motivo": {"x"}}); !strings.Contains(a, "il fascicolo non chiede un commerciale") {
		t.Errorf("tipo non chiesto: %q", a)
	}
	if a := s.gesto(w, s.comp(vite, "deroga"), url.Values{"tipo": {"disegno_2d"}, "motivo": {"lo disegniamo noi"}}); a != "54000000: disegno_2d derogato («lo disegniamo noi»)." {
		t.Fatalf("deroga: %q", a)
	}
	if esito(vite, "disegno_2d") != "derogato" {
		t.Errorf("dopo la deroga: %s", esito(vite, "disegno_2d"))
	}
	_, html := w.fai(http.MethodGet, s.base()+"?vista=griglia", nil, false)
	if !strings.Contains(html, `title="2D: derogato"`) {
		t.Error("la griglia deve mostrare il 2D derogato")
	}
	deroga := s.valore(`SELECT deroga_id::text FROM deroga_fabbisogno WHERE componente_id = $1`, vite)
	if a := s.gesto(w, s.base()+"/deroga/"+deroga+"/revoca", nil); a != "Deroga revocata: il requisito torna a contare." {
		t.Errorf("revoca: %q", a)
	}
	if esito(vite, "disegno_2d") != "manca" {
		t.Errorf("dopo la revoca: %s", esito(vite, "disegno_2d"))
	}

	if a := s.gesto(w, s.comp(s.prodotto, "step-strutturale"), url.Values{"documento": {s.disegno.String()}}); !strings.Contains(a, "non è un file STEP") {
		t.Errorf("un 2D come STEP strutturale: %q", a)
	}
	if a := s.gesto(w, s.comp(s.assieme, "step-strutturale"), url.Values{"documento": {s.step.String()}}); !strings.Contains(a, "non è un prodotto finito") {
		t.Errorf("STEP strutturale di un assieme: %q", a)
	}
	passo := s.valore(`SELECT esito FROM v_step_prodotto WHERE componente_id = $1`, s.prodotto)
	if passo != "da_scegliere" {
		t.Fatalf("prima della scelta: %s", passo)
	}
	if a := s.gesto(w, s.comp(s.prodotto, "step-strutturale"), url.Values{"documento": {s.step.String()}}); !strings.HasPrefix(a, "52922757.stp è lo STEP strutturale di 52922757. Rimozioni non calcolate") {
		t.Fatalf("scelta: %q (uno STEP non analizzato non propone rimozioni, e lo si dice)", a)
	}
	if got := s.valore(`SELECT esito FROM v_step_prodotto WHERE componente_id = $1`, s.prodotto); got != "presente_non_analizzato" {
		t.Fatalf("dopo la scelta: %s", got)
	}
	if a := s.gesto(w, s.comp(s.prodotto, "deroga-struttura"), url.Values{"motivo": {""}}); !strings.Contains(a, "una deroga si concede con il suo motivo") {
		t.Errorf("deroga strutturale senza motivo: %q", a)
	}
	if a := s.gesto(w, s.comp(s.prodotto, "deroga-struttura"), url.Values{"motivo": {"STEP del cliente, va bene così"}}); !strings.Contains(a, "deroga strutturale concessa") {
		t.Fatalf("deroga strutturale: %q", a)
	}
	ds := s.valore(`SELECT deroga_struttura_id::text FROM v_step_prodotto WHERE componente_id = $1`, s.prodotto)
	if ds == "NULL" {
		t.Fatal("la deroga strutturale deve valere per lo STEP e la lettura di adesso")
	}
	_, html = w.daFascicolo(http.MethodGet, s.base()+"/parti?vista=bom&nodo="+s.prodotto.String(), nil, s.thread, "?vista=bom")
	if !strings.Contains(html, "deroga strutturale") || !strings.Contains(html, "/deroga-struttura/"+ds+"/revoca") {
		t.Error("la scheda del nodo deve mostrare la deroga strutturale con la revoca")
	}
	if a := s.gesto(w, s.base()+"/deroga-struttura/"+ds+"/revoca", nil); a != "Deroga strutturale revocata." {
		t.Errorf("revoca della deroga strutturale: %q", a)
	}
}

// ---------------------------------------------------------------- revisioni dei documenti

// «Questo sostituisce quello», con un predecessore preciso; annullare riporta il vecchio corrente. Se il
// vecchio e' lo STEP strutturale, la domanda sul riferimento vuole una risposta.
func TestSostituireEAnnullareDallaSchermata(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("SOS87")
	w := operatore(b)
	nuovo2d, _ := s.documentoDa("52922757 foglio 1 rev B.pdf", "pdf", db.TipoDocumentoDisegno2d, "52922757", s.prodotto, db.StatoNasInCoda)
	step2, _ := s.documentoDa("52922757 v2.stp", "stp", db.TipoDocumentoCad3d, "52922757", s.prodotto, db.StatoNasInCoda)
	doc := func(d uuid.UUID, gesto string) string { return s.base() + "/documento/" + d.String() + "/" + gesto }

	if a := s.gesto(w, doc(step2, "sostituisci"), url.Values{"vecchio": {s.disegno.String()}}); !strings.Contains(a, "un disegno_2d si sostituisce con un disegno_2d") {
		t.Errorf("tipi diversi: %q", a)
	}
	if a := s.gesto(w, doc(nuovo2d, "sostituisci"), url.Values{"vecchio": {s.disegno.String()}}); a != "52922757 foglio 1.pdf sostituito da 52922757 foglio 1 rev B.pdf." {
		t.Fatalf("sostituisci: %q", a)
	}
	if b.sostituitoDa(s.disegno) != nuovo2d.String() {
		t.Fatal("il vecchio 2D deve puntare al nuovo")
	}
	_, html := w.daFascicolo(http.MethodGet, s.base()+"/anteprima?vista=bom&file="+s.allDisegno.String(), nil, s.thread, "?vista=bom")
	if !strings.Contains(html, "Storico delle revisioni") || !strings.Contains(html, doc(s.disegno, "annulla-sostituzione")) {
		t.Errorf("l'anteprima del vecchio deve mostrare la storia e l'annullamento")
	}
	if a := s.gesto(w, doc(s.disegno, "annulla-sostituzione"), nil); !strings.Contains(a, "Sostituzione annullata") {
		t.Errorf("annulla: %q", a)
	}
	if b.sostituitoDa(s.disegno) != "-" {
		t.Error("dopo l'annullamento il vecchio e' di nuovo corrente")
	}

	s.gesto(w, s.comp(s.prodotto, "step-strutturale"), url.Values{"documento": {s.step.String()}})
	if a := s.gesto(w, doc(step2, "sostituisci"), url.Values{"vecchio": {s.step.String()}}); !strings.Contains(a, "sostituisce lo STEP strutturale di 52922757: si dice se il nuovo diventa il riferimento") {
		t.Errorf("senza risposta sul riferimento: %q", a)
	}
	if b.sostituitoDa(s.step) != "-" {
		t.Fatal("senza risposta non si sostituisce")
	}
	if a := s.gesto(w, doc(step2, "sostituisci"), url.Values{"vecchio": {s.step.String()}, "nuovo_riferimento": {"1"}}); !strings.Contains(a, "è il nuovo STEP strutturale di 52922757") {
		t.Fatalf("con il sì: %q", a)
	}
	if got := s.valore(`SELECT step_strutturale_id::text FROM componente WHERE componente_id = $1`, s.prodotto); got != step2.String() {
		t.Errorf("il riferimento doveva passare al nuovo STEP: %s", got)
	}
}

// ---------------------------------------------------------------- versioni della BOM (A4.6, A4.7, D25c)

// Il congelamento con il gate: rosso con i suoi motivi, verde dopo i gesti che servono. Poi la BOM
// congelata si legge e non si cambia, ma accetta un caricamento. In ACCETTATA la revisione vuole la
// scelta, senza preselezione; l'abbandono solo a differenza vuota.
func TestCongelareERivedereLaBomDallaSchermata(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	s := b.scenaB87("BOM87")
	w := operatore(b)
	ws := b.caricamentoAcceso(t)
	_ = ws

	a := s.gesto(w, s.base()+"/congela", url.Values{"motivo": {"prima baseline"}})
	if !strings.HasPrefix(a, "Niente è cambiato: non si congela: ") || !strings.Contains(a, "requisiti bloccanti") || !strings.Contains(a, "DA SCEGLIERE") {
		t.Fatalf("gate rosso: %q", a)
	}
	if n := s.conta(`SELECT count(*) FROM bom_versione WHERE thread_id = $1`, s.thread); n != 0 {
		t.Fatalf("un congelamento rifiutato ha lasciato %d versioni", n)
	}
	_, html := w.fai(http.MethodGet, s.base(), nil, false)
	if !strings.Contains(html, "Non si congela ancora") {
		t.Error("il dialogo del congelamento deve dire perche' non si congela")
	}
	s.gesto(w, s.comp(s.assieme, "deroga"), url.Values{"tipo": {"disegno_2d"}, "motivo": {"lo facciamo noi"}})
	s.gesto(w, s.comp(s.particolare, "deroga"), url.Values{"tipo": {"disegno_2d"}, "motivo": {"a commessa"}})
	s.gesto(w, s.comp(s.prodotto, "step-strutturale"), url.Values{"documento": {s.step.String()}})
	s.gesto(w, s.comp(s.prodotto, "deroga-struttura"), url.Values{"motivo": {"non ancora analizzato, va bene"}})
	if a := s.gesto(w, s.base()+"/congela", url.Values{"motivo": {"prima baseline"}}); !strings.HasPrefix(a, "BOM congelata: V1 (preventivo). La fase è SCHEDA_COSTO.") {
		t.Fatalf("congela: %q", a)
	}

	// congelata: la working non cambia, i gesti non ci sono, il caricamento si'
	if a := s.gesto(w, s.comp(s.particolare, "archivia"), url.Values{"motivo": {"x"}}); !strings.Contains(a, "la BOM è congelata nella V1") {
		t.Errorf("archivia con la BOM congelata: %q", a)
	}
	if a := s.gesto(w, s.comp(s.particolare, "modifica"), url.Values{"tipo": {"sciolto"}, "rev": {"Z"}}); !strings.Contains(a, "la BOM è congelata nella V1") {
		t.Errorf("modifica con la BOM congelata: %q", a)
	}
	_, html = w.fai(http.MethodGet, s.base()+"?nodo="+s.particolare.String(), nil, false)
	for _, c := range []string{"BOM congelata nella V1", "Apri una revisione…", "Carica dal PC", "Sarà una revisione <b>preventivo</b>"} {
		if !strings.Contains(html, c) {
			t.Errorf("congelata: manca %q", c)
		}
	}
	for _, v := range []string{"/componente/" + s.particolare.String() + "/modifica", "/componente/" + s.particolare.String() + "/archivia", "Assegna i selezionati", "Congela…"} {
		if strings.Contains(html, v) {
			t.Errorf("congelata: la pagina offre %q", v)
		}
	}
	if a := b.carica(w, s, "52922757_C.stp", []byte("ISO-10303-21; versione interna C")); !strings.Contains(a, "caricato come versione interna") {
		t.Errorf("caricamento con la BOM congelata: %q", a)
	}

	// ACCETTATA: la scelta preventivo/tecnica, senza default (D25c)
	s.fase("ACCETTATA")
	_, html = w.fai(http.MethodGet, s.base(), nil, false)
	if !strings.Contains(html, `name="contesto" value="preventivo" required>`) || !strings.Contains(html, `name="contesto" value="tecnica" required>`) {
		t.Error("in ACCETTATA la pagina deve offrire le due scelte")
	}
	if regexp.MustCompile(`name="contesto"[^>]*checked`).MatchString(html) {
		t.Error("D25c: nessuna preselezione del tipo di revisione")
	}
	if a := s.gesto(w, s.base()+"/revisione/apri", url.Values{"motivo": {"il cliente cambia la staffa"}}); !strings.Contains(a, "il tipo di revisione lo sceglie chi la apre") {
		t.Errorf("senza scelta: %q", a)
	}
	if a := s.gesto(w, s.base()+"/revisione/apri", url.Values{"contesto": {"tecnica"}}); !strings.Contains(a, "una revisione si apre con il suo motivo") {
		t.Errorf("senza motivo: %q", a)
	}
	if n := s.conta(`SELECT count(*) FROM bom_versione WHERE thread_id = $1`, s.thread); n != 1 {
		t.Fatalf("i rifiuti hanno aperto versioni: %d", n)
	}
	if a := s.gesto(w, s.base()+"/revisione/apri", url.Values{"contesto": {"tecnica"}, "motivo": {"il cliente cambia la staffa"}}); a != "Revisione V2 aperta (tecnica): la fase resta ACCETTATA. La BOM working si modifica di nuovo." {
		t.Fatalf("apri: %q", a)
	}
	if got := s.valore(`SELECT nome_fase::text FROM fase_log WHERE thread_id = $1 AND fine IS NULL`, s.thread); got != "ACCETTATA" {
		t.Errorf("una revisione tecnica non cambia fase: %s", got)
	}
	s.gesto(w, s.comp(s.particolare, "modifica"), url.Values{"tipo": {"sciolto"}, "rev": {"B"}})
	if a := s.gesto(w, s.base()+"/revisione/abbandona", nil); !strings.Contains(a, "la BOM working non è più la V1") {
		t.Errorf("abbandono con differenze: %q", a)
	}
	_, html = w.fai(http.MethodGet, s.base(), nil, false)
	if !strings.Contains(html, "1 differenza dalla V1") || !strings.Contains(html, "rev") {
		t.Error("la testata deve mostrare la differenza dalla V1")
	}
	s.gesto(w, s.comp(s.particolare, "modifica"), url.Values{"tipo": {"sciolto"}, "rev": {""}})
	if a := s.gesto(w, s.base()+"/revisione/abbandona", nil); a != "Revisione V2 abbandonata: la BOM resta la V1." {
		t.Errorf("abbandono a differenza vuota: %q", a)
	}
}

// ---------------------------------------------------------------- caricamento interno

// caricamentoAcceso monta sulla schermata la strada del workerapi e uno staging vero.
func (b *bancoWeb) caricamentoAcceso(t *testing.T) *workerapi.Server {
	t.Helper()
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	dir := t.TempDir()
	wa := &workerapi.Server{Pool: b.pool, Log: testutil.LogSilenzioso(), Staging: dir, Analizzatore: an}
	b.ws.Pipeline, b.ws.Staging = wa, dir
	t.Cleanup(func() { b.ws.Pipeline, b.ws.Staging = nil, "" })
	return wa
}

// carica manda un file con il modulo della schermata (multipart) e restituisce l'avviso.
func (b *bancoWeb) carica(w *browser, s *scenaB87, nome string, contenuto []byte) string {
	b.t.Helper()
	var corpo bytes.Buffer
	mw := multipart.NewWriter(&corpo)
	f, _ := mw.CreateFormFile("file", nome)
	_, _ = f.Write(contenuto)
	_ = mw.Close()
	r, _ := http.NewRequest(http.MethodPost, b.srv.URL+s.base()+"/carica", &corpo)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("HX-Request", "true")
	r.Header.Set("HX-Current-URL", b.srv.URL+s.base())
	r.Header.Set("X-Prova-IP", w.ip)
	resp, err := w.c.Do(r)
	if err != nil {
		b.t.Fatal(err)
	}
	defer resp.Body.Close()
	testo, _ := io.ReadAll(resp.Body)
	return avvisoF(string(testo))
}

// Il file caricato a mano entra nella nota interna della RFQ come allegato di origine manuale, e' fra i
// contenuti dello staging con il suo hash, ha la proposta dal nome e l'analisi in coda. La revisione nel
// nome non diventa del cliente. Un secondo file va nello stesso contenitore.
func TestUnaVersioneInternaFaLaStradaDegliAllegati(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	s := b.scenaB87("CAR87")
	w := operatore(b)

	if a := b.carica(w, s, "52922757_B.stp", []byte("x")); !strings.Contains(a, "il caricamento interno non è configurato") {
		t.Errorf("senza pipeline: %q", a)
	}
	wa := b.caricamentoAcceso(t)
	if a := b.carica(w, s, "listino.xlsx", []byte("x")); !strings.Contains(a, "qui si caricano versioni interne di CAD 3D") {
		t.Errorf("un file non tecnico: %q", a)
	}
	if a := b.carica(w, s, "52922757_B.stp", nil); !strings.Contains(a, "è vuoto") {
		t.Errorf("file vuoto: %q", a)
	}
	contenuto := []byte("ISO-10303-21;\nHEADER; versione interna B del prodotto\nENDSEC;")
	somma := sha256.Sum256(contenuto)
	sha := hex.EncodeToString(somma[:])
	if a := b.carica(w, s, "52922757_B.stp", contenuto); !strings.Contains(a, "52922757_B.stp caricato come versione interna") {
		t.Fatalf("caricamento: %q", a)
	}
	var (
		allegato             uuid.UUID
		origine, stato, perc string
		msgCanale, chiave    string
		msgThread            uuid.UUID
		interno              bool
		indice               int
	)
	if err := b.pool.QueryRow(b.ctx, `SELECT a.allegato_id, a.origine, a.stato, a.path_staging, a.indice, m.canale, m.chiave_esterna, m.thread_id, m.interno
		FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE a.sha256 = $1`, sha).
		Scan(&allegato, &origine, &stato, &perc, &indice, &msgCanale, &chiave, &msgThread, &interno); err != nil {
		t.Fatal(err)
	}
	if origine != "manuale" || msgCanale != "nota" || chiave != "nota:caricamenti:"+s.thread.String() || msgThread != s.thread || !interno || indice != 1 {
		t.Errorf("allegato %s dalla nota %s/%s (thread %v, interno %v, indice %d)", origine, msgCanale, chiave, msgThread == s.thread, interno, indice)
	}
	if stato != "in_staging" || filepath.Base(perc) != sha+".stp" || !strings.HasPrefix(perc, wa.Staging) {
		t.Errorf("staging: stato %s, percorso %s", stato, perc)
	}
	if letto, err := os.ReadFile(perc); err != nil || !bytes.Equal(letto, contenuto) {
		t.Errorf("il contenuto nello staging non e' il file caricato: %v", err)
	}
	if got := s.valore(`SELECT tipo_proposto || '/' || COALESCE(codice, '-') || '/' || COALESCE(rev, '-') || '/' || COALESCE(dettagli ->> 'rev_letta', '-')
		FROM documento_proposta WHERE allegato_id = $1`, allegato); got != "cad_3d/52922757/-/B" {
		t.Errorf("proposta: %s (la rev B del nome resta nei dettagli, non diventa del cliente)", got)
	}
	if n := s.conta(`SELECT count(*) FROM job WHERE tipo = 'analizza_allegato' AND payload ->> 'sha256' = $1`, sha); n != 1 {
		t.Errorf("analisi accodate: %d", n)
	}
	if n := s.conta(`SELECT count(*) FROM documento WHERE sha256 = $1`, sha); n != 0 {
		t.Error("il caricamento non decide: nessun documento prima della conferma")
	}

	altro := []byte("ISO-10303-21; un altro file interno")
	if a := b.carica(w, s, "52922757 alternativa.stp", altro); !strings.Contains(a, "caricato come versione interna") {
		t.Fatalf("secondo caricamento: %q", a)
	}
	if n := s.conta(`SELECT count(*) FROM messaggio WHERE canale = 'nota' AND thread_id = $1`, s.thread); n != 1 {
		t.Errorf("i caricamenti della RFQ stanno in un contenitore solo: %d", n)
	}
	if n := s.conta(`SELECT max(a.indice) FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE m.canale = 'nota' AND m.thread_id = $1`, s.thread); n != 2 {
		t.Errorf("il secondo file prende l'indice 2: %d", n)
	}
	_, html := w.fai(http.MethodGet, s.base()+"?vista=documenti", nil, false)
	if !strings.Contains(html, "52922757_B.stp") || !strings.Contains(html, ">interno<") || !strings.Contains(html, "(B?)") {
		t.Error("il pannello Documenti deve mostrare il file interno, con la revisione letta e non attribuita")
	}
}

// La decisione su una versione interna: la conferma con «sostituisce» vuole il motivo, e se il vecchio e'
// lo STEP strutturale la risposta sul riferimento. Con tutti e due il documento nasce, senza revisione del
// cliente, sostituisce il vecchio, diventa il riferimento e porta il motivo nella nota.
func TestConfermareUnaVersioneInternaCheSostituisce(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	s := b.scenaB87("CNF87")
	w := operatore(b)
	b.caricamentoAcceso(t)
	s.gesto(w, s.comp(s.prodotto, "step-strutturale"), url.Values{"documento": {s.step.String()}})
	if a := b.carica(w, s, "52922757_B.stp", []byte("ISO-10303-21; interna B")); !strings.Contains(a, "caricato come versione interna") {
		t.Fatalf("caricamento: %q", a)
	}
	var pid, aid uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT p.proposta_id, p.allegato_id FROM documento_proposta p JOIN allegato a ON a.allegato_id = p.allegato_id
		WHERE a.origine = 'manuale' AND p.thread_id = $1`, s.thread).Scan(&pid, &aid); err != nil {
		t.Fatal(err)
	}
	_, html := w.daFascicolo(http.MethodGet, s.base()+"/anteprima?vista=bom&file="+aid.String()+"&nodo="+s.prodotto.String(), nil, s.thread, "?vista=bom")
	for _, c := range []string{"Conferma questo file…", "versione interna, non del cliente", `name="scelta"`, "sostituisce 52922757.stp", `name="nuovo_riferimento" value="1" checked`, `name="motivo"`} {
		if !strings.Contains(html, c) {
			t.Errorf("anteprima del file interno: manca %q", c)
		}
	}
	conferma := func(form url.Values) string {
		form.Set("ritorna_thread", s.thread.String())
		_, html := w.daFascicolo(http.MethodPost, "/proposta/"+pid.String()+"/conferma", form, s.thread, "?file="+aid.String())
		return avvisoF(html)
	}
	base := url.Values{"componente_id": {s.prodotto.String()}, "tipo": {"cad_3d"}, "codice": {"52922757"}, "scelta": {s.step.String()}}
	if a := conferma(base); !strings.Contains(a, "sostituisce lo STEP strutturale di 52922757") {
		t.Errorf("senza la risposta sul riferimento: %q", a)
	}
	f := url.Values{}
	for k, v := range base {
		f[k] = v
	}
	f.Set("nuovo_riferimento", "1")
	if a := conferma(f); !strings.Contains(a, "è una versione interna: sostituisce un documento solo con un motivo") {
		t.Errorf("senza motivo: %q", a)
	}
	if n := s.conta(`SELECT count(*) FROM documento WHERE thread_id = $1`, s.thread); n != 2 {
		t.Fatalf("i rifiuti hanno creato documenti: %d", n)
	}
	f.Set("motivo", "rifatto in casa con gli smussi")
	a := conferma(f)
	if !strings.Contains(a, "Confermato") || !strings.Contains(a, "52922757.stp sostituito da 52922757_B.stp") || !strings.Contains(a, "è il nuovo STEP strutturale di 52922757") {
		t.Fatalf("conferma: %q", a)
	}
	var nuovo uuid.UUID
	var rev, nota, percorso string
	if err := b.pool.QueryRow(b.ctx, `SELECT documento_id, COALESCE(rev, '-'), COALESCE(nota, ''), path_relativo FROM documento WHERE thread_id = $1 AND nome_file = '52922757_B.stp'`, s.thread).
		Scan(&nuovo, &rev, &nota, &percorso); err != nil {
		t.Fatal(err)
	}
	if rev != "-" || !strings.HasSuffix(percorso, "52922757_REV_ND.stp") {
		t.Errorf("una versione interna non inventa la revisione del cliente: rev %s, percorso %s", rev, percorso)
	}
	if nota != "sostituisce 52922757.stp: rifatto in casa con gli smussi" {
		t.Errorf("nota: %q", nota)
	}
	if b.sostituitoDa(s.step) != nuovo.String() {
		t.Error("il vecchio STEP deve puntare al nuovo")
	}
	if got := s.valore(`SELECT step_strutturale_id::text FROM componente WHERE componente_id = $1`, s.prodotto); got != nuovo.String() {
		t.Errorf("STEP strutturale: %s", got)
	}
}

// ---------------------------------------------------------------- assegnazione multipla

// Tre file con un gesto: scelti nel pannello Documenti, assegnati al nodo scelto. Tutto o niente.
func TestAssegnareTreFileConUnGesto(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("TRE87")
	w := operatore(b)
	p1, _ := s.propostaDa("foglio A.pdf", "")
	p2, _ := s.propostaDa("foglio B.pdf", "")
	d3, _ := s.documentoDa("foglio C.pdf", "pdf", db.TipoDocumentoDisegno2d, "53017189", uuid.Nil, db.StatoNasInCoda)

	_, html := w.fai(http.MethodGet, s.base()+"?vista=documenti&nodo="+s.particolare.String()+"&filtro=non_assegnati", nil, false)
	for _, c := range []string{`name="proposta" value="` + p1.String() + `"`, `name="proposta" value="` + p2.String() + `"`,
		`name="documento" value="` + d3.String() + `"`, "Assegna i selezionati a ▸ 53017189"} {
		if !strings.Contains(html, c) {
			t.Errorf("manca %q", c)
		}
	}
	form := url.Values{"componente": {s.particolare.String()}, "proposta": {p1.String(), p2.String()}, "documento": {d3.String()}}
	if a := s.gesto(w, s.base()+"/assegna", form); a != "3 file assegnati al componente 53017189." {
		t.Fatalf("assegna: %q", a)
	}
	if n := s.conta(`SELECT count(*) FROM documento_proposta WHERE componente_id = $1 AND stato = 'aperta'`, s.particolare); n != 2 {
		t.Errorf("proposte assegnate: %d", n)
	}
	if b.fotoProposta(p1) != "componente="+s.particolare.String()[:8]+" codice=53017189 stato=aperta" {
		t.Errorf("proposta: %s", b.fotoProposta(p1))
	}
	if !strings.Contains(b.foto(d3), "componente="+s.particolare.String()[:8]) {
		t.Errorf("documento: %s", b.foto(d3))
	}
}

// ---------------------------------------------------------------- tempo con cento allegati

// Cento file nella RFQ: la pagina si disegna in meno di un secondo sul banco (piano §13 B8.7), anche con
// il filtro «tutti».
func TestCentoAllegatiSiDisegnanoInMenoDiUnSecondo(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("CENTO87")
	for i := 0; i < 100; i++ {
		s.propostaDa(fmt.Sprintf("disegno %03d.pdf", i), fmt.Sprintf("5300%04d", i))
	}
	w := operatore(b)
	w.fai(http.MethodGet, s.base(), nil, false) // la prima apertura rilegge gli STEP e riscalda le cache
	inizio := time.Now()
	resp, html := w.fai(http.MethodGet, s.base()+"?vista=documenti&filtro=tutti", nil, false)
	durata := time.Since(inizio)
	if resp.StatusCode != 200 {
		t.Fatalf("GET: %d", resp.StatusCode)
	}
	if n := strings.Count(html, `<tr id="file-`); n != 104 {
		t.Errorf("righe dei file: %d, attese 104", n)
	}
	if durata > time.Second {
		t.Errorf("la pagina con cento allegati ha impiegato %v", durata)
	}
	t.Logf("pagina con 104 file: %v", durata)
	inizio = time.Now()
	resp, _ = w.fai(http.MethodGet, s.base()+"?vista=bom", nil, false)
	if durata := time.Since(inizio); resp.StatusCode != 200 || durata > time.Second {
		t.Errorf("la BOM visuale con cento allegati: %d in %v", resp.StatusCode, durata)
	} else {
		t.Logf("BOM visuale con 104 file: %v", durata)
	}
	// la vista Documenti (v3, quella che si apre): cento file da associare nel pannello, in meno di un secondo
	inizio = time.Now()
	resp, html = w.fai(http.MethodGet, s.base(), nil, false)
	if durata := time.Since(inizio); resp.StatusCode != 200 || durata > time.Second || !strings.Contains(html, `id="doc-vista"`) {
		t.Errorf("la vista Documenti con cento allegati: %d in %v", resp.StatusCode, durata)
	} else {
		t.Logf("vista Documenti con 104 file: %v", durata)
	}
}

// ---------------------------------------------------------------- una proposta assegnata e la lettura che arriva dopo

// Una proposta assegnata a un componente ha il codice del componente (A1.4, A2.2). Se dopo arriva una
// lettura del file con un altro codice (l'analisi, o la proposta dal nome rifatta), la proposta resta
// del componente con il suo codice, e quello letto si conserva nei dettagli: prima la lettura riscriveva
// il codice, la FK (thread, componente, codice) la rifiutava, e il risultato dell'analisi non entrava mai.
func TestUnaPropostaAssegnataReggeLaLetturaCheArrivaDopo(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaB87("LET87")
	w := operatore(b)
	wa := b.caricamentoAcceso(t)
	if a := b.carica(w, s, "52922757_B.stp", []byte("ISO-10303-21; letto dopo")); !strings.Contains(a, "caricato come versione interna") {
		t.Fatalf("caricamento: %q", a)
	}
	var pid, aid uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT p.proposta_id, p.allegato_id FROM documento_proposta p JOIN allegato a ON a.allegato_id = p.allegato_id
		WHERE a.origine = 'manuale' AND p.thread_id = $1`, s.thread).Scan(&pid, &aid); err != nil {
		t.Fatal(err)
	}
	if a := s.gesto(w, s.base()+"/assegna", url.Values{"componente": {s.particolare.String()}, "proposta": {pid.String()}, "correggi_codice": {"1"}}); a != "1 file assegnato al componente 53017189." {
		t.Fatalf("assegna: %q", a)
	}
	// la lettura del file arriva adesso, e dice ancora 52922757
	tx, err := b.pool.Begin(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(b.ctx)
	if err := wa.DopoCaricamento(b.ctx, db.New(tx), aid); err != nil {
		t.Fatalf("la lettura arrivata dopo l'assegnazione e' stata rifiutata: %v", err)
	}
	if err := tx.Commit(b.ctx); err != nil {
		t.Fatal(err)
	}
	if got := s.valore(`SELECT COALESCE(codice, '-') || '/' || COALESCE(componente_id::text, '-') || '/' || COALESCE(dettagli ->> 'codice_letto', '-')
		FROM documento_proposta WHERE proposta_id = $1`, pid); got != "53017189/"+s.particolare.String()+"/52922757" {
		t.Errorf("proposta dopo la lettura: %s", got)
	}
}
