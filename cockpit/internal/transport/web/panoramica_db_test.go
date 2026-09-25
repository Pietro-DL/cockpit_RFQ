//go:build integrazione

// L4 — la pagina Richieste sul database vero, dal mux vero con la protezione CSRF: le RFQ aperte e
// chiuse, i filtri della barra, l'anteprima di ogni prodotto (documento corrente, poi proposta, mai un
// documento sostituito), le sei schede e «+ N altri», l'elenco chiesto da HTMX con l'indirizzo pulito,
// il poll che risponde 204 quando niente e' cambiato.
package web

import (
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// scenaRichieste: quattro RFQ di tre clienti.
//
//	t1 ACME (peso 10)  «RFQ 118.26 Staffe carrello», buyer Rossi, scade 30/10; tre prodotti: il primo con
//	                   un 2D corrente (e uno piu' recente ma sostituito, e una proposta), il secondo con la sola
//	                   proposta, il terzo senza disegni; un particolare senza 2D (bloccante); ATTESA_DISEGNI
//	t2 BETA            chiusa (PERSA), un prodotto
//	t3 GAMMA           «Carter motore», FATTIBILITA da 5 giorni (SLA rosso), otto prodotti fra cui AB_12
//	t4 GAMMA           «Carter pompa», scade 01/10, il prodotto ABX12, ATTESA_DISEGNI da oggi
type scenaRichieste struct {
	b                  *bancoWeb
	w                  *browser
	acme, beta, gamma  uuid.UUID
	t1, t2, t3, t4     *rfqFascicolo
	comp1              uuid.UUID
	docCorrente        uuid.UUID // l'allegato del 2D corrente di 7120001
	docSostituito      uuid.UUID // l'allegato del 2D sostituito, piu' recente
	propostaP1         uuid.UUID // l'allegato della proposta di 7120001
	propostaP2         uuid.UUID // l'allegato della proposta di 7120002
}

func (s *scenaRichieste) esegui(sql string, arg ...any) {
	s.b.t.Helper()
	if _, err := s.b.pool.Exec(s.b.ctx, sql, arg...); err != nil {
		s.b.t.Fatalf("%v\n%s", err, sql)
	}
}

func (s *scenaRichieste) codici(r *rfqFascicolo, codici ...string) {
	for _, c := range codici {
		s.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine) VALUES ($1, $2, 'manuale')`, r.thread, c)
	}
}

// fase chiude la fase aperta e ne apre una cominciata giorni fa.
func (s *scenaRichieste) fase(r *rfqFascicolo, nome string, giorni int) {
	s.esegui(`UPDATE fase_log SET fine = now() - make_interval(days => $2) WHERE thread_id = $1 AND fine IS NULL`, r.thread, giorni)
	s.esegui(`INSERT INTO fase_log (thread_id, nome_fase, inizio) VALUES ($1, $2, now() - make_interval(days => $3))`, r.thread, nome, giorni)
}

// documento2D e' un 2D confermato con la sua provenienza: l'allegato da cui e' nato, ricevuto quando.
func (s *scenaRichieste) documento2D(r *rfqFascicolo, nome, codice string, comp uuid.UUID, ricevuto time.Time) (doc, allegato uuid.UUID) {
	s.b.t.Helper()
	allegato, sha := r.allegato(nome)
	doc = uuid.New()
	s.esegui(`INSERT INTO documento (documento_id, thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, bytes, path_relativo, stato_nas, confermato_da)
		VALUES ($1,$2,$3,'disegno_2d',$4,$5,'pdf',$6,10,$7,'scritto',$8)`, doc, r.thread, comp, codice, nome, sha, `ELENCO DISEGNI\`+codice+`\`+nome, r.utente)
	s.esegui(`INSERT INTO documento_provenienza (documento_id, allegato_id, ricevuto_il) VALUES ($1,$2,$3)`, doc, allegato, ricevuto)
	return doc, allegato
}

func (s *scenaRichieste) allegatoDi(proposta uuid.UUID) uuid.UUID {
	var a uuid.UUID
	if err := s.b.pool.QueryRow(s.b.ctx, `SELECT allegato_id FROM documento_proposta WHERE proposta_id = $1`, proposta).Scan(&a); err != nil {
		s.b.t.Fatal(err)
	}
	return a
}

func preparaRichieste(t *testing.T) *scenaRichieste {
	t.Helper()
	b := preparaBancoWeb(t)
	s := &scenaRichieste{b: b}
	s.acme = b.unCliente("ACME S.p.A.", "ACME", "acme.example")
	s.beta = b.unCliente("Beta Meccanica S.r.l.", "BETA", "beta.example")
	s.gamma = b.unCliente("Gamma Carpenteria", "GAMMA", "gamma.example")
	s.esegui(`UPDATE cliente SET peso = 10 WHERE cliente_id = $1`, s.acme)
	var rossi uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO buyer (cliente_id, cognome) VALUES ($1, 'Rossi') RETURNING buyer_id`, s.acme).Scan(&rossi); err != nil {
		t.Fatal(err)
	}

	s.t1 = b.rfqFascicolo(s.acme, "P1")
	s.esegui(`UPDATE thread_offerta SET oggetto = 'RFQ 118.26 Staffe carrello', riferimento_cliente = '118.26', buyer_id = $2, data_scadenza = '2026-10-30' WHERE thread_id = $1`, s.t1.thread, rossi)
	s.codici(s.t1, "7120001", "7120002", "7120003")
	s.fase(s.t1, "ATTESA_DISEGNI", 0)
	s.comp1 = s.t1.componente("7120001")
	s.esegui(`UPDATE componente SET tipo = 'finito', rev = 'B', descrizione = 'Staffa sinistra' WHERE componente_id = $1`, s.comp1)
	ora := time.Now()
	corrente, a1 := s.documento2D(s.t1, "7120001_B.pdf", "7120001", s.comp1, ora.Add(-48*time.Hour))
	s.docCorrente = a1
	vecchio, a2 := s.documento2D(s.t1, "7120001_A.pdf", "7120001", s.comp1, ora.Add(-time.Hour)) // piu' recente, ma sostituito
	s.esegui(`UPDATE documento SET sostituito_da = $2 WHERE documento_id = $1`, vecchio, corrente)
	s.docSostituito = a2
	s.propostaP1 = s.allegatoDi(s.t1.proposta("7120001_C.pdf", "7120001", uuid.Nil))
	s.propostaP2 = s.allegatoDi(s.t1.proposta("7120002.pdf", "7120002", uuid.Nil))
	s.t1.componente("7120009") // un particolare senza 2D: bloccante

	s.t2 = b.rfqFascicolo(s.beta, "P2")
	s.esegui(`UPDATE thread_offerta SET oggetto = 'Richiesta d''offerta 700100200' WHERE thread_id = $1`, s.t2.thread)
	s.codici(s.t2, "7700100A1")
	s.fase(s.t2, "PERSA", 1)

	s.t3 = b.rfqFascicolo(s.gamma, "P3")
	s.esegui(`UPDATE thread_offerta SET oggetto = 'Carter motore' WHERE thread_id = $1`, s.t3.thread)
	s.codici(s.t3, "AB_12", "7130001", "7130002", "7130003", "7130004", "7130005", "7130006", "7130007")
	s.fase(s.t3, "FATTIBILITA", 5)

	s.t4 = b.rfqFascicolo(s.gamma, "P4")
	s.esegui(`UPDATE thread_offerta SET oggetto = 'Carter pompa', data_scadenza = '2026-10-01' WHERE thread_id = $1`, s.t4.thread)
	s.codici(s.t4, "ABX12")
	s.fase(s.t4, "ATTESA_DISEGNI", 0) // non RICEVUTA: ha SLA di 0 giorni, e sarebbe rossa subito

	s.w = b.browser("10.0.0.5:51000")
	s.w.login("FP", "prova-fp")
	return s
}

// card sono le RFQ nell'elenco, nell'ordine in cui compaiono.
var reCard = regexp.MustCompile(`<article class="rq-card[^"]*" id="rfq-([0-9a-f-]{36})"`)

func (s *scenaRichieste) elenco(percorso string, hx bool) (*http.Response, string, []uuid.UUID) {
	s.b.t.Helper()
	resp, corpo := s.w.fai(http.MethodGet, percorso, nil, hx)
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		s.b.t.Fatalf("GET %s: %d\n%s", percorso, resp.StatusCode, primi400(corpo))
	}
	var ids []uuid.UUID
	for _, m := range reCard.FindAllStringSubmatch(corpo, -1) {
		ids = append(ids, uuid.MustParse(m[1]))
	}
	return resp, corpo, ids
}

func (s *scenaRichieste) nomi(ids []uuid.UUID) string {
	nome := map[uuid.UUID]string{s.t1.thread: "t1", s.t2.thread: "t2", s.t3.thread: "t3", s.t4.thread: "t4"}
	var out []string
	for _, id := range ids {
		out = append(out, nome[id])
	}
	return strings.Join(out, ",")
}

func TestRichiesteLaPaginaDiLavoroELeAperte(t *testing.T) {
	s := preparaRichieste(t)
	_, pagina, ids := s.elenco("/richieste", false)
	// aperte, ACME (peso 10) in cima, poi le altre per scadenza (t4 scade, t3 no)
	if got := s.nomi(ids); got != "t1,t4,t3" {
		t.Errorf("elenco predefinito %q, atteso t1,t4,t3", got)
	}
	for _, atteso := range []string{`<form id="filtri"`, `<b>3</b> richieste`, `class="rq-badge fase">ATTESA DISEGNI`, `>ACME</option>`,
		`Buyer: <b>Rossi</b>`, `Scadenza: <b>30/10/2026</b>`, `class="rq-rif">118.26`} {
		if !strings.Contains(pagina, atteso) {
			t.Errorf("manca %q", atteso)
		}
	}
	if strings.Contains(pagina, "<table") {
		t.Error("la pagina e' ancora la tabella")
	}
	_, _, ids = s.elenco("/richieste?stato=tutte", false)
	if got := s.nomi(ids); got != "t1,t4,t3,t2" {
		t.Errorf("tutte: %q, la chiusa va in fondo", got)
	}
	_, pagina, ids = s.elenco("/richieste?stato=CHIUSA", false)
	if got := s.nomi(ids); got != "t2" || !strings.Contains(pagina, `class="rq-badge chiusa">CHIUSA`) {
		t.Errorf("chiuse: %q", got)
	}
	_, _, ids = s.elenco("/richieste?sort=scadenza", false)
	if got := s.nomi(ids); got != "t4,t1,t3" {
		t.Errorf("per scadenza: %q", got)
	}
}

func TestRichiesteIFiltriDellaBarra(t *testing.T) {
	s := preparaRichieste(t)
	casi := []struct{ percorso, atteso string }{
		{"/richieste?cliente=" + s.beta.String() + "&stato=tutte", "t2"},
		{"/richieste?cliente=" + s.gamma.String(), "t4,t3"},
		{"/richieste?fase=FATTIBILITA", "t3"},
		{"/richieste?bloccanti=1", "t1"},
		{"/richieste?smistare=1", "t1"},
		{"/richieste?sla=1", "t3"},
		{"/richieste?q=carrello", "t1"},       // oggetto
		{"/richieste?q=118.26", "t1"},           // riferimento del cliente
		{"/richieste?q=ROSSI", "t1"},            // buyer, senza badare alle maiuscole
		{"/richieste?q=beta&stato=tutte", "t2"}, // cliente
		{"/richieste?q=meccanica&stato=tutte", "t2"},
		{"/richieste?q=7130003", "t3"}, // un codice della richiesta
		{"/richieste?q=AB_12", "t3"},   // _ alla lettera: non prende ABX12
		{"/richieste?q=ab", "t4,t3"},
		{"/richieste?q=nessuno", ""},
	}
	for _, c := range casi {
		_, corpo, ids := s.elenco(c.percorso, false)
		if got := s.nomi(ids); got != c.atteso {
			t.Errorf("%s: %q, atteso %q", c.percorso, got, c.atteso)
		}
		if c.atteso == "" && !strings.Contains(corpo, "Nessuna richiesta con questi filtri.") {
			t.Errorf("%s: l'elenco vuoto non lo dice", c.percorso)
		}
	}
}

func TestRichiesteLAnteprimaDiOgniProdotto(t *testing.T) {
	s := preparaRichieste(t)
	_, pagina, _ := s.elenco("/richieste", false)
	i := strings.Index(pagina, `id="prodotti-`+s.t1.thread.String()+`"`)
	if i < 0 {
		t.Fatal("le schede di t1 non ci sono")
	}
	schede := pagina[i:]
	schede = schede[:strings.Index(schede, "</article>")]
	if !strings.Contains(schede, `src="/allegato/`+s.docCorrente.String()+`/anteprima#`) {
		t.Error("l'anteprima di 7120001 dev'essere il 2D corrente")
	}
	if strings.Contains(schede, s.docSostituito.String()) {
		t.Error("un 2D sostituito non fa da anteprima, anche se e' piu' recente")
	}
	if strings.Contains(schede, s.propostaP1.String()) {
		t.Error("con un documento corrente la proposta non fa da anteprima")
	}
	if !strings.Contains(schede, `src="/allegato/`+s.propostaP2.String()+`/anteprima#`) || strings.Count(schede, "da confermare") != 1 {
		t.Error("7120002 ha solo la proposta: anteprima segnata «da confermare»")
	}
	if !strings.Contains(schede, "nessun PDF") {
		t.Error("7120003 non ha disegni: la scheda lo dice")
	}
	if !strings.Contains(schede, `/thread/`+s.t1.thread.String()+`/fascicolo?nodo=`+s.comp1.String()) {
		t.Error("la scheda di 7120001 apre il Fascicolo sul suo componente")
	}
	if !strings.Contains(schede, `rev B`) || !strings.Contains(schede, `Staffa sinistra`) {
		t.Error("revisione e descrizione vengono dal componente")
	}
	if strings.Count(schede, `loading="lazy"`) != strings.Count(schede, "<iframe") {
		t.Error("ogni iframe si carica solo quando arriva sullo schermo")
	}
	// e l'anteprima si apre davvero: la rotta e' quella del Fascicolo, con i suoi controlli
	b := s.b
	b.ws.Staging = s.t1.staging
	percorso := ""
	if err := b.pool.QueryRow(b.ctx, `SELECT path_staging FROM allegato WHERE allegato_id = $1`, s.docCorrente).Scan(&percorso); err != nil {
		t.Fatal(err)
	}
	pdf := pdfFinto(4096)
	if err := os.WriteFile(percorso, pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	s.esegui(`UPDATE allegato SET sha256 = $2, bytes = $3 WHERE allegato_id = $1`, s.docCorrente, shaDi(pdf), len(pdf))
	resp, _ := s.w.fai(http.MethodGet, "/allegato/"+s.docCorrente.String()+"/anteprima", nil, false)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/pdf" {
		t.Errorf("l'anteprima della scheda: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

func TestRichiesteSeiSchedeEPoiGliAltri(t *testing.T) {
	s := preparaRichieste(t)
	_, pagina, _ := s.elenco("/richieste?q=carter+motore", false)
	if n := strings.Count(pagina, `class="rq-prod-link"`); n != prodottiInCard {
		t.Errorf("schede visibili %d, attese %d", n, prodottiInCard)
	}
	if !strings.Contains(pagina, "+ 2 altri") || !strings.Contains(pagina, `hx-get="/richieste/`+s.t3.thread.String()+`/prodotti"`) {
		t.Error("manca «+ 2 altri»")
	}
	resp, tutte := s.w.fai(http.MethodGet, "/richieste/"+s.t3.thread.String()+"/prodotti", nil, true)
	if resp.StatusCode != 200 || strings.Count(tutte, `class="rq-prod-link"`) != 8 || !strings.Contains(tutte, "Mostra meno") {
		t.Errorf("la card aperta: %d, %d schede", resp.StatusCode, strings.Count(tutte, `class="rq-prod-link"`))
	}
	if strings.Contains(tutte, "<html") || !strings.Contains(tutte, `id="prodotti-`+s.t3.thread.String()+`"`) {
		t.Error("la card aperta e' un frammento con lo stesso id, da mettere al posto delle schede")
	}
	_, meno := s.w.fai(http.MethodGet, "/richieste/"+s.t3.thread.String()+"/prodotti?meno=1", nil, true)
	if strings.Count(meno, `class="rq-prod-link"`) != prodottiInCard || !strings.Contains(meno, "+ 2 altri") {
		t.Error("«Mostra meno» torna a sei schede")
	}
	if r, _ := s.w.fai(http.MethodGet, "/richieste/"+uuid.NewString()+"/prodotti", nil, true); r.StatusCode != 404 {
		t.Errorf("una RFQ che non c'e': %d", r.StatusCode)
	}
	if r, _ := s.w.fai(http.MethodGet, "/richieste/abc/prodotti", nil, true); r.StatusCode != 400 {
		t.Errorf("un id non valido: %d", r.StatusCode)
	}
}

var reFirmaElenco = regexp.MustCompile(`"firma":"([0-9a-f]+)"`)

func TestRichiesteHTMXIndirizzoEPoll(t *testing.T) {
	s := preparaRichieste(t)
	resp, frammento, ids := s.elenco("/richieste?q=carrello&stato=APERTA&sort=priorita&cliente=&fase=", true)
	if s.nomi(ids) != "t1" || strings.Contains(frammento, "<html") || strings.Contains(frammento, "<form") {
		t.Fatalf("il filtro via HTMX deve dare solo l'elenco: %q", s.nomi(ids))
	}
	if got := resp.Header.Get("HX-Push-Url"); got != "/richieste?q=carrello" {
		t.Errorf("HX-Push-Url %q: l'indirizzo va pulito dai valori predefiniti", got)
	}
	m := reFirmaElenco.FindStringSubmatch(frammento)
	if m == nil {
		t.Fatal("l'elenco non porta la firma per il poll")
	}
	poll := "/richieste?q=carrello&firma=" + m[1] + "&n=30"
	resp, corpo, _ := s.elenco(poll, true)
	if resp.StatusCode != 204 || corpo != "" || resp.Header.Get("HX-Push-Url") != "" {
		t.Errorf("poll senza cambiamenti: %d, %d byte, push %q", resp.StatusCode, len(corpo), resp.Header.Get("HX-Push-Url"))
	}
	// arriva un prodotto nuovo: il poll porta l'elenco rifatto
	s.codici(s.t1, "7120004")
	resp, corpo, _ = s.elenco(poll, true)
	if resp.StatusCode != 200 || !strings.Contains(corpo, "7120004") || resp.Header.Get("HX-Push-Url") != "" {
		t.Errorf("poll dopo un cambiamento: %d, nuovo codice %v", resp.StatusCode, strings.Contains(corpo, "7120004"))
	}
	// «indietro» senza la copia in cache: HTMX chiede la pagina intera, e la pagina intera arriva
	r, pagina := s.w.chiedi(http.MethodGet, "/richieste?q=carrello", map[string]string{"HX-Request": "true", "HX-History-Restore-Request": "true"})
	if r.StatusCode != 200 || !strings.Contains(string(pagina), "<form id=\"filtri\"") || !strings.Contains(string(pagina), `value="carrello"`) {
		t.Error("il ripristino della cronologia vuole la pagina intera, con il filtro nella barra")
	}
}

func TestRichiesteMostraAltre(t *testing.T) {
	s := preparaRichieste(t)
	for i := 0; i < 35; i++ {
		var id uuid.UUID
		if err := s.b.pool.QueryRow(s.b.ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto) VALUES ($1,'outlook', now() - make_interval(mins => $2), $3) RETURNING thread_id`,
			s.beta, i, fmt.Sprintf("Richiesta %02d", i)).Scan(&id); err != nil {
			t.Fatal(err)
		}
	}
	_, pagina, ids := s.elenco("/richieste", false)
	if len(ids) != cardPerPagina || !strings.Contains(pagina, "Mostra altre (8 ancora)") || !strings.Contains(pagina, `hx-get="/richieste?n=60"`) {
		t.Errorf("prima pagina: %d card", len(ids))
	}
	_, pagina, ids = s.elenco("/richieste?n=60", false)
	if len(ids) != 38 || strings.Contains(pagina, "Mostra altre") {
		t.Errorf("con n=60: %d card", len(ids))
	}
}

// Cento RFQ con dieci prodotti e cinque disegni ciascuna: tre letture, e la pagina e' pronta in meno di un
// secondo anche su questo PC.
func TestRichiesteCentoRfqSiDisegnanoInMenoDiUnSecondo(t *testing.T) {
	s := preparaRichieste(t)
	for i := 0; i < 100; i++ {
		r := s.b.rfqFascicolo(s.beta, fmt.Sprintf("C%03d", i))
		for j := 0; j < 10; j++ {
			s.codici(r, fmt.Sprintf("9%03d%03d", i, j))
		}
		for j := 0; j < 5; j++ {
			r.proposta(fmt.Sprintf("9%03d%03d.pdf", i, j), fmt.Sprintf("9%03d%03d", i, j), uuid.Nil)
		}
	}
	s.elenco("/richieste?n=120", false) // la prima volta scalda le cache del database
	inizio := time.Now()
	_, pagina, ids := s.elenco("/richieste?n=120", false)
	durata := time.Since(inizio)
	if len(ids) != 103 {
		t.Fatalf("card %d, attese 103", len(ids))
	}
	if n := strings.Count(pagina, "<iframe"); n > 103*prodottiInCard {
		t.Errorf("%d iframe: al piu' %d per card", n, prodottiInCard)
	}
	t.Logf("cento RFQ: %v, %d KB, %d iframe", durata, len(pagina)/1024, strings.Count(pagina, "<iframe"))
	if durata > time.Second {
		t.Errorf("la pagina con cento RFQ ci mette %v", durata)
	}
}
