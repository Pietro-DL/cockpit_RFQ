package web

// L1 — la pagina Richieste senza database: i filtri letti dall'indirizzo, il testo della ricerca preso
// alla lettera, la scelta dell'anteprima di un prodotto, le sei schede di una card, la firma del poll e
// il template eseguito con dati sintetici.

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

func TestIFiltriDelleRichiesteSiLegganoDallIndirizzo(t *testing.T) {
	cli := uuid.New()
	casi := []struct {
		query string
		atteso filtriRichieste
	}{
		{"", filtriRichieste{Stato: statoAperte, Ordine: "priorita", N: cardPerPagina}},
		{"q=++staffa++&cliente=" + cli.String() + "&fase=FATTIBILITA&stato=tutte&bloccanti=1&smistare=1&sla=1&sort=scadenza&n=60",
			filtriRichieste{Q: "staffa", Cliente: cli, Fase: db.FaseFATTIBILITA, Stato: statoTutte, Bloccanti: true, DaSmistare: true, SLA: true, Ordine: "scadenza", N: 60}},
		{"stato=CHIUSA", filtriRichieste{Stato: statoChiuse, Ordine: "priorita", N: cardPerPagina}},
		{"stato=chiusa&sort=aggiornamento", filtriRichieste{Stato: statoChiuse, Ordine: "aggiornamento", N: cardPerPagina}},
		// un segnalibro vecchio o scritto a mano apre la pagina: quello che non si capisce si ignora
		{"cliente=acme&fase=INVENTATA&stato=forse&bloccanti=si&sort=peso&n=-3", filtriRichieste{Stato: statoAperte, Ordine: "priorita", N: cardPerPagina}},
		// n a passi di una pagina, e mai oltre il tetto
		{"n=31", filtriRichieste{Stato: statoAperte, Ordine: "priorita", N: 60}},
		{"n=100000", filtriRichieste{Stato: statoAperte, Ordine: "priorita", N: maxCardRichieste}},
	}
	for _, c := range casi {
		v, _ := url.ParseQuery(c.query)
		if got := leggiFiltriRichieste(v); got != c.atteso {
			t.Errorf("%q:\n got  %+v\n want %+v", c.query, got, c.atteso)
		}
	}
	lungo := strings.Repeat("è", maxTestoRicerca+20)
	if f := leggiFiltriRichieste(url.Values{"q": {lungo}}); len([]rune(f.Q)) != maxTestoRicerca {
		t.Errorf("il testo di ricerca non si tronca a %d caratteri: %d", maxTestoRicerca, len([]rune(f.Q)))
	}
}

func TestLIndirizzoDeiFiltriEPulitoETornaIndietroUguale(t *testing.T) {
	if u := leggiFiltriRichieste(url.Values{}).URL(); u != "/richieste" {
		t.Errorf("senza filtri l'indirizzo e' %q, non /richieste", u)
	}
	// i valori predefiniti non si scrivono: stato=APERTA, sort=priorita, n=30
	v, _ := url.ParseQuery("q=&cliente=&fase=&stato=APERTA&sort=priorita&n=30")
	if u := leggiFiltriRichieste(v).URL(); u != "/richieste" {
		t.Errorf("i filtri predefiniti finiscono nell'indirizzo: %q", u)
	}
	f := filtriRichieste{Q: "staffa 7_1", Cliente: uuid.New(), Fase: db.FaseSCHEDACOSTO, Stato: statoChiuse, SLA: true, Ordine: "aggiornamento", N: 90}
	u, err := url.Parse(f.URL())
	if err != nil {
		t.Fatal(err)
	}
	if got := leggiFiltriRichieste(u.Query()); got != f {
		t.Errorf("andata e ritorno dall'indirizzo:\n got  %+v\n want %+v", got, f)
	}
	if !strings.Contains(f.Altre(), "n=120") {
		t.Errorf("«Mostra altre» non aggiunge una pagina: %q", f.Altre())
	}
	if (filtriRichieste{Stato: statoAperte, Ordine: "scadenza", N: 60}).Attivi() {
		t.Error("ordinamento e pagine non sono filtri: «Togli i filtri» non deve comparire")
	}
}

func TestLaRicercaPrendeIlTestoAllaLettera(t *testing.T) {
	casi := map[string]string{
		"":         "",
		"staffa":   "%staffa%",
		"7120_400": `%7120\_400%`,
		"50%":      `%50\%%`,
		`a\b`:      `%a\\b%`,
	}
	for q, atteso := range casi {
		m := modelloRicerca(q)
		if q == "" {
			if m.Valid {
				t.Errorf("il testo vuoto deve essere NULL (nessun filtro), non %q", m.String)
			}
			continue
		}
		if !m.Valid || m.String != atteso {
			t.Errorf("modelloRicerca(%q) = %q, atteso %q", q, m.String, atteso)
		}
	}
}

// candidato e' un disegno che puo' fare da anteprima.
func candidato(thread uuid.UUID, comp uuid.UUID, codice, fonte string, quando time.Time) db.ListAnteprimePanoramicaRow {
	return db.ListAnteprimePanoramicaRow{ThreadID: thread, ComponenteID: uuid.NullUUID{UUID: comp, Valid: comp != uuid.Nil},
		Codice: pgtype.Text{String: codice, Valid: codice != ""}, AllegatoID: uuid.New(), NomeFile: codice + ".pdf", Fonte: fonte, Quando: quando}
}

func TestLAnteprimaEIlDisegnoConfermatoPoiLaProposta(t *testing.T) {
	th, comp := uuid.New(), uuid.New()
	ora := time.Now()
	pr := db.ListProdottiPanoramicaRow{ThreadID: th, Codice: "7120A", ComponenteID: uuid.NullUUID{UUID: comp, Valid: true}}
	vecchioDoc := candidato(th, comp, "7120A", "documento", ora.Add(-2*time.Hour))
	nuovoDoc := candidato(th, uuid.Nil, "7120a", "documento", ora.Add(-time.Hour)) // senza componente, codice in minuscolo
	proposta := candidato(th, uuid.Nil, "7120A", "proposta", ora)                  // piu' recente, ma e' una proposta
	altro := candidato(th, uuid.New(), "9999", "documento", ora.Add(time.Hour))   // un altro prodotto

	a, ok := sceglieAnteprima(pr, []db.ListAnteprimePanoramicaRow{proposta, vecchioDoc, altro, nuovoDoc})
	if !ok || a.AllegatoID != nuovoDoc.AllegatoID {
		t.Errorf("l'anteprima dev'essere il documento piu' recente del codice, non la proposta ne' un altro codice: %+v", a)
	}
	a, ok = sceglieAnteprima(pr, []db.ListAnteprimePanoramicaRow{proposta, altro})
	if !ok || a.AllegatoID != proposta.AllegatoID {
		t.Errorf("senza documenti l'anteprima e' la proposta aperta: %+v", a)
	}
	if _, ok := sceglieAnteprima(pr, []db.ListAnteprimePanoramicaRow{altro}); ok {
		t.Error("il disegno di un altro codice non e' l'anteprima di questo")
	}
	// a parita' di fonte e di momento la scelta non dipende dall'ordine in cui arrivano le righe
	x, y := candidato(th, comp, "7120A", "documento", ora), candidato(th, comp, "7120A", "documento", ora)
	a1, _ := sceglieAnteprima(pr, []db.ListAnteprimePanoramicaRow{x, y})
	a2, _ := sceglieAnteprima(pr, []db.ListAnteprimePanoramicaRow{y, x})
	if a1.AllegatoID != a2.AllegatoID {
		t.Error("due letture uguali in ordine diverso scelgono anteprime diverse")
	}
}

// sintetica e' una panoramica di due RFQ: una con otto prodotti, una chiusa con uno.
func sintetica(f filtriRichieste) *panoramicaRichieste {
	t1, t2, comp := uuid.New(), uuid.New(), uuid.New()
	scad := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	righe := []db.ListRichiestePanoramicaRow{
		{ThreadID: t1, Cliente: "ACME", RagioneSociale: "ACME S.p.A.", Oggetto: txtT("RFQ 118.26 Staffe carrello"),
			RiferimentoCliente: txtT("118.26"), Buyer: txtT("Rossi"), StatoThread: db.StatoThreadAPERTA,
			NomeFase: db.NullFase{Fase: db.FaseFATTIBILITA, Valid: true}, GgInFase: pgtype.Int4{Int32: 3, Valid: true}, SlaGg: pgtype.Int4{Int32: 2, Valid: true},
			Semaforo: txtT("rosso"), DataScadenza: &scad, DataInizio: time.Now(), NBloccanti: pgtype.Int8{Int64: 18, Valid: true}, NDaSmistare: 1, Totale: 2},
		{ThreadID: t2, Cliente: "BETA", RagioneSociale: "Beta Meccanica", Oggetto: txtT("Richiesta d'offerta 700100200"), StatoThread: db.StatoThreadCHIUSA,
			DataInizio: time.Now(), Totale: 2},
	}
	var prodotti []db.ListProdottiPanoramicaRow
	for i := 0; i < 8; i++ {
		p := db.ListProdottiPanoramicaRow{ThreadID: t1, Codice: fmt.Sprintf("0210%d", i)}
		if i == 0 {
			p.ComponenteID, p.Rev, p.Descrizione = uuid.NullUUID{UUID: comp, Valid: true}, txtT("B"), txtT("Staffa sinistra")
		}
		prodotti = append(prodotti, p)
	}
	prodotti = append(prodotti, db.ListProdottiPanoramicaRow{ThreadID: t1, Codice: "02100"}) // lo stesso codice due volte: una scheda
	prodotti = append(prodotti, db.ListProdottiPanoramicaRow{ThreadID: t2, Codice: "7700100A1"})
	anteprime := []db.ListAnteprimePanoramicaRow{
		candidato(t1, comp, "02100", "documento", time.Now()),
		candidato(t1, uuid.Nil, "02101", "proposta", time.Now()),
	}
	return costruisciPanoramica(f, righe, prodotti, anteprime)
}

func TestLeSchedeDiUnaCard(t *testing.T) {
	p := sintetica(leggiFiltriRichieste(url.Values{}))
	if len(p.Card) != 2 || p.Totale != 2 || p.Altre() != 0 {
		t.Fatalf("card %d, totale %d, altre %d", len(p.Card), p.Totale, p.Altre())
	}
	c := p.Card[0]
	if len(c.Prodotti) != 8 {
		t.Fatalf("il codice ripetuto fa due schede: %d prodotti", len(c.Prodotti))
	}
	if len(c.Mostrati()) != prodottiInCard || c.Nascosti() != 2 || c.PuoRichiudere() {
		t.Errorf("una card chiusa mostra %d schede e ne nasconde %d", len(c.Mostrati()), c.Nascosti())
	}
	c.Tutti = true
	if len(c.Mostrati()) != 8 || c.Nascosti() != 0 || !c.PuoRichiudere() {
		t.Error("la card aperta mostra tutto e si puo' richiudere")
	}
	p0, p1, p2 := c.Prodotti[0], c.Prodotti[1], c.Prodotti[2]
	if !strings.HasSuffix(p0.Link, "?nodo="+c.Prodotti[0].Link[strings.LastIndex(p0.Link, "=")+1:]) || !strings.Contains(p0.Link, "/fascicolo?nodo=") {
		t.Errorf("la scheda con il componente apre il Fascicolo sul componente: %q", p0.Link)
	}
	if p0.Anteprima == "" || p0.DaConfermare || p0.Rev != "B" || p0.Descrizione != "Staffa sinistra" {
		t.Errorf("prima scheda: %+v", p0)
	}
	if p1.Anteprima == "" || !p1.DaConfermare || strings.Contains(p1.Link, "nodo=") {
		t.Errorf("la scheda senza componente con una proposta: %+v", p1)
	}
	if p2.Anteprima != "" {
		t.Errorf("la scheda senza disegni non ha anteprima: %+v", p2)
	}
	if !strings.HasPrefix(p0.Anteprima, "/allegato/") || !strings.Contains(p0.Anteprima, "/anteprima#") {
		t.Errorf("l'anteprima passa dalla rotta che controlla il PDF: %q", p0.Anteprima)
	}
	if got := c.FaseBreve(); got != "FATTIBILITÀ" {
		t.Errorf("fase %q", got)
	}
	if got := nomeFase(db.FaseATTESADISEGNI); got != "ATTESA DISEGNI" {
		t.Errorf("fase %q", got)
	}
	if c.FraseBloccanti() != "18 bloccanti" || p.Card[1].FraseProdotti() != "1 prodotto" {
		t.Errorf("frasi: %q, %q", c.FraseBloccanti(), p.Card[1].FraseProdotti())
	}
}

func TestLaFirmaDellElencoCambiaSoloSeCambiaQuelloCheSiVede(t *testing.T) {
	f := leggiFiltriRichieste(url.Values{})
	p := sintetica(f)
	// cinquanta volte la stessa lettura: la stessa firma (niente mappe nell'ordine)
	for i := 0; i < 50; i++ {
		if firmaPanoramica(p) != p.Firma {
			t.Fatal("la stessa panoramica da' firme diverse")
		}
	}
	q := *p
	q.Card = append([]richiestaCard(nil), p.Card...)
	q.Card[0].Prodotti = append([]prodottoCard(nil), p.Card[0].Prodotti...)
	q.Card[0].Prodotti[2].allegato = uuid.New()
	if firmaPanoramica(&q) == p.Firma {
		t.Error("un disegno nuovo su una scheda non cambia la firma: il poll non lo porterebbe")
	}
	g := f
	g.N = 60
	q2 := *p
	q2.Filtri = g
	if firmaPanoramica(&q2) == p.Firma {
		t.Error("filtri diversi, stessa firma")
	}
}

func TestIlTemplateDelleRichieste(t *testing.T) {
	s := serverTest(t)
	p := sintetica(leggiFiltriRichieste(url.Values{"q": {"staffa"}}))
	p.Intervallo = 60
	p.opzioniBarra([]db.ListClientiTuttiRow{{ClienteID: uuid.New(), CartellaNas: "ACME", RagioneSociale: "ACME S.p.A.", NRichieste: 3}},
		[]db.FaseCatalogo{{NomeFase: db.FaseRICEVUTA, Ordine: 10, Descrizione: "Thread creato dal triage"}, {NomeFase: db.FaseFATTIBILITA, Ordine: 20}})
	var buf bytes.Buffer
	if err := s.pagine["richieste.html"].ExecuteTemplate(&buf, "layout", vista{Titolo: "Richieste", Dati: p}); err != nil {
		t.Fatal(err)
	}
	h := buf.String()
	for _, atteso := range []string{
		`<form id="filtri"`, `name="q" value="staffa"`, `hx-get="/richieste"`, `hx-target="#elenco"`,
		`value="APERTA" checked`, `>FATTIBILITÀ</option>`, `>ACME</option>`,
		`<b>2</b> richieste`, `Togli i filtri`, `RFQ 118.26 Staffe carrello`, `class="rq-badge fase">FATTIBILITÀ`,
		`class="semaforo rosso"`, `⚠ 18 bloccanti`, `1 file da smistare`, `8 prodotti`, `+ 2 altri`,
		`class="rq-badge chiusa">CHIUSA`, `loading="lazy"`, `da confermare`, `nessun PDF`,
		`hx-trigger="every 60s"`, `hx-vals='{"firma":"` + p.Firma + `","n":"30"}'`,
	} {
		if !strings.Contains(h, atteso) {
			t.Errorf("manca %q", atteso)
		}
	}
	if strings.Contains(h, `<table`) {
		t.Error("la pagina e' ancora una tabella")
	}
	if n := strings.Count(h, "<iframe"); n != 2 {
		t.Errorf("iframe nella pagina: %d, attesi 2 (le sole schede con un disegno)", n)
	}
	// l'UUID della RFQ non si legge: sta negli attributi (id, href, title), mai nel testo
	testo := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(h, " ")
	if strings.Contains(testo, p.Card[0].ThreadID.String()) {
		t.Error("l'UUID della RFQ compare nel testo della pagina")
	}
	// il frammento dell'elenco non porta la barra: cambiando un filtro il form resta quello
	buf.Reset()
	if err := s.pagine["richieste.html"].ExecuteTemplate(&buf, "richieste_elenco", vista{Dati: p, Frammento: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `<form`) || !strings.Contains(buf.String(), `rq-card`) {
		t.Error("il frammento dell'elenco deve portare le card e non la barra")
	}
}
