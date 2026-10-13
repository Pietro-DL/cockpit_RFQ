package web

// L1 — giro 5, A1d-P4, passo B5.1 dopo la direttiva 12 del 13/10 (§7, rilievi R-01 e R-02; §9, D-18 = A, decisione
// dell'utente). Con httptest e senza database, sulla pagina della Distinta nuova (motore_a_pagina.go,
// web/templates/motore_a.html):
//   - R-01: la scelta (vista e prodotto) e la sua sorgente: la query, poi HX-Current-URL della rilettura htmx di #ma,
//     poi nessuna (sceltaMotoreADa); dalla richiesta alla resa con la composizione pura del servizio (ComponiPagina), su
//     un caso con due prodotti (e17): la pagina aperta sul secondo prodotto nella vista Documenti, la rilettura del ponte
//     dopo un gesto e «Aggiorna» (la stessa richiesta: il pulsante chiama la rilettura del ponte) tengono prodotto e
//     vista; senza scelta, con un prodotto che non c'è più o con una vista che non esiste la pagina lo dice;
//   - R-02: con l'esito incompleto (thread non valutato, una diagnostica del thread di gravità «errore») nessun badge di
//     verifica è positivo, la striscia dice perché, la radice ha data-ma-calcolo="incompleto"; con l'esito completo i
//     positivi restano (controprova: e13 ha prodotto e BOM verificati);
//   - D-18: errori di calcolo o di servizio → 200 con l'errore scritto, senza esito, data-ma-calcolo="errore" e
//     data-ma-errore = la tipologia, uguali nella pagina e nel frammento; 400 per un id non valido e 404 per una RFQ che
//     non c'è, anche nel frammento; la tipologia nel log; nessuna scrittura all'apertura.
// Il comportamento nel browser (motore_a.mjs che riscrive l'indirizzo e legge data-ma-calcolo dopo la rilettura) lo
// prova motore_a_browser_test.go (Scenario H, tag integrazione e browser), NON ESEGUITA qui: queste prove non dicono che
// il JavaScript funzioni. Le guardie sul testo del modulo in fondo servono solo a non perdere i punti d'aggancio.
//
// Solo dati ACME inventati: UUID 00000000-0000-4000-8000-0000000000nn, codici 712xxxx, example.com.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/net/html"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/platform/db"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// ---------------------------------------------------------------- R-01: la scelta e la sua sorgente

func TestMotoreAD12SceltaESorgente(t *testing.T) {
	id := mwUUID(1).String()
	pagina := "/thread/" + id + "/motore-a"
	ril := func(corrente string) map[string]string {
		h := map[string]string{"HX-Request": "true", "HX-Target": "ma"}
		if corrente != "-" {
			h["HX-Current-URL"] = corrente
		}
		return h
	}
	for _, c := range []struct {
		nome, indirizzo string
		intestazioni    map[string]string
		attesa          sceltaMotoreA
	}{
		{"pagina con la query", pagina + "?vista=documenti&prodotto=P2", nil, sceltaMotoreA{"documenti", "P2", sorgenteQuery, false}},
		{"pagina senza query", pagina, nil, sceltaMotoreA{"", "", sorgenteNessuna, false}},
		{"rilettura: l'indirizzo mostrato", pagina, ril("http://example.com" + pagina + "?prodotto=P2&vista=documenti"), sceltaMotoreA{"documenti", "P2", sorgenteIndirizzo, true}},
		{"rilettura: la query vince", pagina + "?prodotto=P3", ril("http://example.com" + pagina + "?prodotto=P2&vista=documenti"), sceltaMotoreA{"", "P3", sorgenteQuery, true}},
		{"rilettura senza HX-Current-URL", pagina, ril("-"), sceltaMotoreA{"", "", sorgenteNessuna, true}},
		{"rilettura con l'indirizzo senza parametri", pagina, ril("http://example.com" + pagina), sceltaMotoreA{"", "", sorgenteNessuna, true}},
		{"rilettura con altri parametri soltanto", pagina, ril("http://example.com" + pagina + "?x=1"), sceltaMotoreA{"", "", sorgenteNessuna, true}},
		{"rilettura dall'indirizzo di un'altra pagina", pagina, ril("http://example.com/thread/" + id + "/distinta?vista=documenti"), sceltaMotoreA{"", "", sorgenteNessuna, true}},
		{"rilettura dall'indirizzo di un altro host", pagina, ril("http://altro.example" + pagina + "?vista=documenti"), sceltaMotoreA{"", "", sorgenteNessuna, true}},
		{"htmx verso un altro bersaglio", pagina, map[string]string{"HX-Request": "true", "HX-Target": "ma-avviso", "HX-Current-URL": pagina + "?vista=rfq"}, sceltaMotoreA{"", "", sorgenteNessuna, false}},
	} {
		req := httptest.NewRequest(http.MethodGet, c.indirizzo, nil)
		for k, v := range c.intestazioni {
			req.Header.Set(k, v)
		}
		if got := sceltaMotoreADa(req); got != c.attesa {
			t.Errorf("%s: %+v, attesa %+v", c.nome, got, c.attesa)
		}
	}
}

// mdRisultato: il risultato di un calcolo riuscito fatto con il caso: la fotografia minima del caso, l'esito del thread,
// il confronto e la fase. ComponiPagina lo compone come il servizio (stessa funzione pura).
func mdRisultato(c *casoMotoreA) *motorea_web.Risultato {
	foto := c.Foto
	r := &motorea_web.Risultato{Thread: &foto, EsitoThread: c.Pagina.Esito, Fase: c.Pagina.Testa.Fase}
	if c.Pagina.Confronto != nil {
		r.Confronto = *c.Pagina.Confronto
	} else {
		r.Confronto = confronto.Esito{}
	}
	return r
}

// mdServi: la richiesta come la serve il gestore quando il calcolo riesce: la scelta (vistaEProdottoMotoreA), la
// composizione pura del servizio (ComponiPagina: VistaDa e ProdottoScelto) e la resa (rendiMotoreA).
func mdServi(t *testing.T, s *Server, c *casoMotoreA, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	v, p := vistaEProdottoMotoreA(req)
	pg := motorea_web.ComponiPagina(mdRisultato(c), motorea_web.RichiestaPagina{Thread: c.Pagina.ThreadID, Vista: v, Prodotto: p, Scrive: true})
	rec := httptest.NewRecorder()
	s.rendiMotoreA(rec, req, pg)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: %d %.300s", req.URL, rec.Code, rec.Body.String())
	}
	return rec
}

// mdRadice: l'unica radice .ma della risposta (pagina o frammento).
func mdRadice(t *testing.T, nome string, b []byte) (*html.Node, *html.Node) {
	t.Helper()
	doc := maRDoc(t, b)
	r := maRTutti(doc, func(n *html.Node) bool { return maRHa(n, "data-ma-pagina") })
	if len(r) != 1 {
		t.Fatalf("%s: %d radici data-ma-pagina", nome, len(r))
	}
	return doc, r[0]
}

// mdCorrente: il testo dell'elemento con aria-current dentro l'elenco con quell'id.
func mdCorrente(doc *html.Node, elenco string) []string {
	var out []string
	for _, e := range maRID(doc, elenco) {
		for _, a := range maRTutti(e, func(n *html.Node) bool { return maRHa(n, "aria-current") }) {
			out = append(out, maRA(a, "href"))
		}
	}
	return out
}

func TestMotoreAD12RiletturaTieneProdottoEVista(t *testing.T) {
	s := maRServer(t)
	c := leggiCasoMotoreA(t, "e17")
	rif := maRProdotti(c)
	if len(rif) < 2 {
		t.Fatalf("e17 con %d prodotti: la prova vuole il secondo", len(rif))
	}
	primo, secondo := rif[0], rif[1]
	id := c.Pagina.ThreadID.String()
	pagina := "/thread/" + id + "/motore-a"
	mostrato := "http://example.com" + pagina + "?" + url.Values{"prodotto": {secondo}, "vista": {"documenti"}}.Encode()
	rilettura := func(corrente string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, pagina, nil)
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Target", "ma")
		if corrente != "" {
			req.Header.Set("HX-Current-URL", corrente)
		}
		return req
	}
	attesa := func(nome string, rec *httptest.ResponseRecorder, prodotto, vista, sorgente string, strisce ...string) {
		t.Helper()
		doc, r := mdRadice(t, nome, rec.Body.Bytes())
		if maRA(r, "data-ma-prodotto") != prodotto || maRA(r, "data-ma-vista") != vista || maRA(r, "data-ma-sorgente") != sorgente ||
			maRA(r, "data-ma-calcolo") != calcoloCompleto {
			t.Errorf("%s: prodotto %q vista %q sorgente %q calcolo %q; attesi %q %q %q %q", nome, maRA(r, "data-ma-prodotto"),
				maRA(r, "data-ma-vista"), maRA(r, "data-ma-sorgente"), maRA(r, "data-ma-calcolo"), prodotto, vista, sorgente, calcoloCompleto)
		}
		// la scheda del prodotto e la vista correnti sono quelle della radice (lo stesso dato, letto da chi guarda)
		voce := "?" + url.Values{"prodotto": {prodotto}, "vista": {vista}}.Encode()
		if cor := mdCorrente(doc, "ma-prodotti"); len(cor) != 1 || !strings.HasSuffix(cor[0], "?prodotto="+url.QueryEscape(prodotto)+"&vista="+vista) {
			t.Errorf("%s: la scheda corrente del prodotto è %v, attesa quella di %s", nome, cor, voce)
		}
		if cor := mdCorrente(doc, "ma-viste"); len(cor) != 1 || !strings.HasSuffix(cor[0], "&vista="+vista) {
			t.Errorf("%s: la vista corrente è %v, attesa %s", nome, cor, vista)
		}
		for _, id := range []string{"ma-scelta-predefinita", "ma-prodotto-non-trovato", "ma-vista-non-valida"} {
			vuole := false
			for _, x := range strisce {
				vuole = vuole || x == id
			}
			if n := len(maRID(doc, id)); (n == 1) != vuole || n > 1 {
				t.Errorf("%s: %d strisce #%s, attesa %v", nome, n, id, vuole)
			}
		}
	}

	// 1. la pagina aperta sul secondo prodotto, vista Documenti (un collegamento semplice: la query)
	attesa("pagina del secondo prodotto", mdServi(t, s, c, httptest.NewRequest(http.MethodGet, pagina+"?"+url.Values{"prodotto": {secondo}, "vista": {"documenti"}}.Encode(), nil)),
		secondo, "documenti", sorgenteQuery)
	// 2. la rilettura del ponte dopo un gesto: il percorso senza query, l'indirizzo mostrato in HX-Current-URL
	fr := mdServi(t, s, c, rilettura(mostrato))
	attesa("rilettura dopo un gesto", fr, secondo, "documenti", sorgenteIndirizzo)
	mpFrammentoAmmesso(t, "rilettura dopo un gesto", fr.Body.Bytes(), id)
	if bytes.Contains(fr.Body.Bytes(), []byte("<html")) {
		t.Error("la rilettura dopo un gesto è la pagina intera, non il frammento")
	}
	// 3. «Aggiorna»: la stessa rilettura (motore_a.mjs → Gesti.aggiorna → la GET del ponte), con l'indirizzo che il modulo
	// ha riscritto (i parametri in un altro ordine, come li scrive URLSearchParams.set su una pagina aperta senza query)
	attesa("Aggiorna", mdServi(t, s, c, rilettura("http://example.com"+pagina+"?"+url.Values{"vista": {"documenti"}}.Encode()+"&prodotto="+url.QueryEscape(secondo))),
		secondo, "documenti", sorgenteIndirizzo)
	// 4. una rilettura senza HX-Current-URL, o con l'indirizzo senza parametri: la scelta predefinita, detta
	attesa("rilettura senza HX-Current-URL", mdServi(t, s, c, rilettura("")), primo, "distinta", sorgenteNessuna, "ma-scelta-predefinita")
	attesa("rilettura con l'indirizzo senza parametri", mdServi(t, s, c, rilettura("http://example.com"+pagina)), primo, "distinta", sorgenteNessuna,
		"ma-scelta-predefinita")
	// 5. il prodotto di prima non c'è più (un gesto l'ha tolto o ne ha cambiato il riferimento): il primo, detto
	sparito := "componente:" + mwUUID(77).String()
	rec := mdServi(t, s, c, rilettura("http://example.com"+pagina+"?"+url.Values{"prodotto": {sparito}, "vista": {"documenti"}}.Encode()))
	attesa("prodotto sparito", rec, primo, "documenti", sorgenteIndirizzo, "ma-prodotto-non-trovato")
	if doc, _ := mdRadice(t, "prodotto sparito", rec.Body.Bytes()); len(maRTutti(doc, func(n *html.Node) bool { return maRA(n, "title") == "prodotto chiesto = "+sparito })) != 1 {
		t.Error("prodotto sparito: la striscia non dice quale prodotto era chiesto")
	}
	// 6. una vista che non esiste: la Distinta, detto
	attesa("vista che non esiste", mdServi(t, s, c, httptest.NewRequest(http.MethodGet, pagina+"?vista=boh&prodotto="+url.QueryEscape(secondo), nil)),
		secondo, "distinta", sorgenteQuery, "ma-vista-non-valida")
}

// ---------------------------------------------------------------- R-02: esito incompleto, mai «verificato»

// mdVerifiche: gli elementi che dicono un booleano di verifica del server vero (prodotto_verificato, bom.verificata):
// quelli con il positivo (ok) e quelli sospesi (data-ma-verifica-sospesa).
func mdVerifiche(doc *html.Node) (positivi, sospesi []*html.Node) {
	for _, n := range maRTutti(doc, func(n *html.Node) bool { return n.Type == html.ElementNode }) {
		ti := maRA(n, "title")
		if !strings.HasPrefix(ti, "prodotto_verificato = true") && !strings.HasPrefix(ti, "bom.verificata = true") {
			continue
		}
		if maRHa(n, "data-ma-verifica-sospesa") {
			sospesi = append(sospesi, n)
		} else {
			positivi = append(positivi, n)
		}
	}
	return positivi, sospesi
}

func TestMotoreAD12EsitoIncompletoMaiVerificato(t *testing.T) {
	s := maRServer(t)
	for _, c := range []struct {
		nome    string
		cambia  func(c *casoMotoreA)
		calcolo string
		motivi  string
	}{
		{"e13 com'è (controprova)", func(*casoMotoreA) {}, calcoloCompleto, ""},
		{"e13, thread non valutato", func(c *casoMotoreA) { c.Pagina.Esito.Valutato = false }, calcoloIncompleto, "thread_non_valutato"},
		{"e13, una diagnostica «errore»", func(c *casoMotoreA) {
			c.Pagina.Esito.Diagnostiche = append(c.Pagina.Esito.Diagnostiche, evidenze.Diagnostica{Codice: "valutazione.errore_valutazione",
				Gravita: evidenze.GravitaErrore, Natura: evidenze.NaturaContratto, Messaggio: "errore ACME inventato"})
		}, calcoloIncompleto, "diagnostiche_errore"},
		{"e13, una diagnostica «avviso» (non basta)", func(c *casoMotoreA) {
			c.Pagina.Esito.Diagnostiche = append(c.Pagina.Esito.Diagnostiche, evidenze.Diagnostica{Codice: "ancoraggio.target_non_leggibile",
				Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaDati, Messaggio: "avviso ACME inventato"})
		}, calcoloCompleto, ""},
		{"e13, tutti e due", func(c *casoMotoreA) {
			c.Pagina.Esito.Valutato = false
			c.Pagina.Esito.Diagnostiche = append(c.Pagina.Esito.Diagnostiche, evidenze.Diagnostica{Codice: "valutazione.errore_valutazione",
				Gravita: evidenze.GravitaErrore, Natura: evidenze.NaturaContratto, Messaggio: "errore ACME inventato"})
		}, calcoloIncompleto, "thread_non_valutato diagnostiche_errore"},
	} {
		caso := leggiCasoMotoreA(t, "e13")
		c.cambia(caso)
		for _, vis := range maRViste {
			r := maRRendi(s, c.nome, caso, vis, maRProdotti(caso)[0], true)
			for nome, rec := range map[string]*httptest.ResponseRecorder{"pagina": r.Pagina, "frammento": r.Frammento} {
				dove := fmt.Sprintf("%s, vista %s, %s", c.nome, vis, nome)
				if rec.Code != http.StatusOK {
					t.Fatalf("%s: %d", dove, rec.Code)
				}
				doc, radice := mdRadice(t, dove, rec.Body.Bytes())
				if maRA(radice, "data-ma-calcolo") != c.calcolo || maRA(radice, "data-ma-incompleto") != c.motivi || maRHa(radice, "data-ma-errore") {
					t.Errorf("%s: data-ma-calcolo %q, data-ma-incompleto %q, data-ma-errore %v; attesi %q %q e niente", dove,
						maRA(radice, "data-ma-calcolo"), maRA(radice, "data-ma-incompleto"), maRHa(radice, "data-ma-errore"), c.calcolo, c.motivi)
				}
				positivi, sospesi := mdVerifiche(doc)
				striscia := maRID(doc, "ma-incompleto")
				if c.calcolo == calcoloCompleto {
					if len(positivi) == 0 || len(sospesi) != 0 || len(striscia) != 0 {
						t.Errorf("%s: %d positivi, %d sospesi, %d strisce: con l'esito completo il «verificato» del server si vede", dove,
							len(positivi), len(sospesi), len(striscia))
					}
					continue
				}
				if len(positivi) != 0 {
					t.Errorf("%s: %d badge di verifica positivi con l'esito incompleto (%q)", dove, len(positivi), maRTesto(positivi[0]))
				}
				if len(sospesi) == 0 {
					t.Errorf("%s: nessun badge sospeso: la prova non prova niente (e13 ha prodotto e BOM verificati)", dove)
				}
				for _, n := range sospesi {
					if maRClasse(n, "ma-st--ok") || strings.Contains(maRTesto(n), "verificat") {
						t.Errorf("%s: badge sospeso %q con il positivo", dove, maRTesto(n))
					}
				}
				if len(striscia) != 1 || maRA(striscia[0], "role") != "alert" || !strings.Contains(maRTesto(striscia[0]), "Esito incompleto") {
					t.Errorf("%s: la striscia dell'esito incompleto manca o non è un alert", dove)
				} else {
					for _, m := range strings.Fields(c.motivi) {
						if len(maRTutti(striscia[0], func(n *html.Node) bool { return maRA(n, "data-ma-motivo-incompleto") == m })) != 1 {
							t.Errorf("%s: la striscia non dice il motivo %s", dove, m)
						}
					}
				}
				if len(maRTutti(doc, func(n *html.Node) bool { return maRClasse(n, "ma-salva-barra--ok") })) != 0 {
					t.Errorf("%s: la barra dell'albero è verde con l'esito incompleto", dove)
				}
				for _, n := range maRTutti(doc, func(n *html.Node) bool { return strings.HasPrefix(maRA(n, "data-ma-domanda"), "Confermare ") }) {
					if !strings.Contains(maRA(n, "data-ma-domanda"), "la struttura non è ancora verificata") {
						t.Errorf("%s: la domanda di conferma non avvisa che la struttura non è verificata: %q", dove, maRA(n, "data-ma-domanda"))
					}
				}
			}
		}
	}
	// le domande di conferma del documento (Da smistare e voci della completezza): con la BOM verificata e l'esito completo
	// dicono solo che la copia parte; con l'esito incompleto avvisano anche che la struttura non è verificata. e17: il
	// primo prodotto ha la BOM verificata e la RFQ ha file da smistare
	for _, incompleto := range []bool{false, true} {
		caso := leggiCasoMotoreA(t, "e17")
		caso.Pagina.Esito.Valutato = !incompleto
		rif := maRProdotti(caso)[0]
		if !caso.Pagina.Esito.ProdottiValutati[0].BOM.Verificata {
			t.Fatal("e17: il primo prodotto non ha la BOM verificata (la prova non prova niente)")
		}
		r := maRRendi(s, "e17", caso, motorea_web.VistaDocumenti, rif, true)
		domande, avvisate := 0, 0
		for _, n := range maRTutti(maRDoc(t, r.Frammento.Body.Bytes()), func(n *html.Node) bool { return strings.HasPrefix(maRA(n, "data-ma-domanda"), "Confermare ") }) {
			domande++
			if strings.Contains(maRA(n, "data-ma-domanda"), "la struttura non è ancora verificata") {
				avvisate++
			}
		}
		if domande == 0 {
			t.Fatal("e17: nessuna domanda di conferma del documento (la prova non prova niente)")
		}
		if incompleto && avvisate != domande {
			t.Errorf("e17 con l'esito incompleto: %d domande di conferma su %d avvisano che la struttura non è verificata", avvisate, domande)
		}
		if !incompleto && avvisate == domande {
			t.Errorf("e17 con l'esito completo: tutte le %d domande avvisano (la controprova non distingue)", domande)
		}
	}
	// su ogni combinazione dei casi: con l'esito incompleto nessun badge di verifica positivo; e20 (una diagnostica di
	// gravità «errore») e e9 (thread non valutato) sono incompleti, gli altri con l'esito no
	incompleti := map[string]bool{}
	maROgniResa(t, s, func(r *maRResa, _ int) {
		for nome, rec := range map[string]*httptest.ResponseRecorder{"pagina": r.Pagina, "frammento": r.Frammento} {
			doc, radice := mdRadice(t, r.String()+", "+nome, rec.Body.Bytes())
			stato := maRA(radice, "data-ma-calcolo")
			if stato == calcoloIncompleto {
				incompleti[r.Caso] = true
				if p, _ := mdVerifiche(doc); len(p) != 0 {
					t.Errorf("%s, %s: badge di verifica positivo con l'esito incompleto", r, nome)
				}
			}
			if atteso := map[bool]string{true: calcoloErrore, false: ""}[r.P.Esito == nil]; atteso != "" && stato != atteso {
				t.Errorf("%s, %s: senza esito data-ma-calcolo = %q", r, nome, stato)
			}
		}
	})
	for _, n := range []string{"e9_thread_non_valutato", "e20_diagnostiche"} {
		if !incompleti[n] {
			t.Errorf("%s non risulta incompleto", n)
		}
	}
	// il CSS spegne i verdi sotto data-ma-calcolo="incompleto" (solo come testo: l'aspetto si guarda nel browser)
	b, err := fs.ReadFile(risorse.FS, "web/static/motore_a.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{`.ma[data-ma-calcolo="incompleto"] .ma-st--ok {`, `.ma[data-ma-calcolo="incompleto"] .ma-st--ok[title$=" = verificato"]::after`} {
		if !strings.Contains(string(b), v) {
			t.Errorf("motore_a.css senza %s", v)
		}
	}
}

// ---------------------------------------------------------------- D-18: gli errori della pagina e del frammento

// mdChiama: il gestore della pagina con un servizio finto, come pagina o come frammento (HX-Request), e il log in buf.
func mdChiama(t *testing.T, s *Server, svc motorea_web.Servizio, id, resto string, frammento bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/thread/"+id+resto, nil)
	req.SetPathValue("id", id)
	if frammento {
		req.Header.Set("HX-Request", "true")
		req.Header.Set("HX-Target", "ma")
		req.Header.Set("HX-Current-URL", "http://example.com/thread/"+id+"/motore-a?vista=documenti&prodotto=P2")
	}
	req = req.WithContext(context.WithValue(req.Context(), ctxUtente, &db.Utente{UtenteID: mwUUID(9), Sigla: "AD", Ruolo: db.RuoloUtenteAdmin}))
	rec, panico := mwServi(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.paginaMotoreA(w, r, svc) }), req)
	if panico != nil {
		t.Fatalf("GET %s: panic: %v", resto, panico)
	}
	return rec
}

func TestMotoreAD12ErroriPaginaEFrammento(t *testing.T) {
	s := mwServer(t, mwPronte(time.Second))
	var registro bytes.Buffer
	s.Log = slog.New(slog.NewTextHandler(&registro, &slog.HandlerOptions{Level: slog.LevelInfo}))
	id := mwUUID(1).String()
	nonPronte := motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoIndiceAssente}
	for _, c := range []struct {
		nome   string
		regole motorea_web.Regole
		tipo   string
		testo  string
	}{
		{"lettura che non riesce", mwPronte(time.Second), erroreLettura, testoMotoreAErrore},
		{"regole non pronte", nonPronte, erroreRegole, ""},
	} {
		var corpi [2]*html.Node
		for i, frammento := range []bool{false, true} {
			dove := fmt.Sprintf("%s, frammento %v", c.nome, frammento)
			registro.Reset()
			ini := &mpIniziatore{tx: &mpTx{altro: errors.New("lettura ACME non riuscita")}}
			rec := mdChiama(t, s, motorea_web.Servizio{Regole: c.regole, Pool: ini, Log: s.Log}, id, "/motore-a?vista=documenti", frammento)
			if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/html; charset=utf-8" || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s: %d %s", dove, rec.Code, mwIntestazioni(rec.Header()))
			}
			if frammento == bytes.Contains(rec.Body.Bytes(), []byte("<html")) {
				t.Errorf("%s: frammento atteso %v", dove, frammento)
			}
			doc, radice := mdRadice(t, dove, rec.Body.Bytes())
			corpi[i] = radice
			if maRA(radice, "data-ma-calcolo") != calcoloErrore || maRA(radice, "data-ma-errore") != c.tipo || maRA(radice, "data-ma-vista") != "documenti" {
				t.Errorf("%s: data-ma-calcolo %q, data-ma-errore %q, vista %q; attesi errore, %s, documenti", dove, maRA(radice, "data-ma-calcolo"),
					maRA(radice, "data-ma-errore"), maRA(radice, "data-ma-vista"), c.tipo)
			}
			e := maRID(doc, "ma-errore")
			if len(e) != 1 || maRA(e[0], "role") != "alert" || (c.testo != "" && !strings.Contains(maRTesto(e[0]), c.testo)) ||
				!strings.Contains(maRTesto(e[0]), "Nessun esito valido") {
				t.Errorf("%s: la striscia dell'errore manca, non è un alert o non dice l'errore", dove)
			}
			// nessun esito: niente prodotti, niente workspace, niente verifiche, niente diagnostiche
			for _, x := range []string{"ma-prodotti", "ma-viste", "ma-albero", "ma-diagnostiche", "ma-incompleto"} {
				if len(maRID(doc, x)) != 0 {
					t.Errorf("%s: #%s con l'errore", dove, x)
				}
			}
			if p, sosp := mdVerifiche(doc); len(p)+len(sosp) != 0 {
				t.Errorf("%s: badge di verifica senza esito", dove)
			}
			// la testa senza esito non inventa dati della RFQ (integrazione E del 13/10, prima corsa nel browser)
			if len(maRID(doc, "ma-testa-non-letta")) != 1 {
				t.Errorf("%s: la testa non dice che i dati della RFQ non sono letti", dove)
			}
			for _, x := range []string{"cliente non indicato", "senza oggetto", "nessuna fase aperta", "senza riferimento"} {
				if strings.Contains(maRTesto(doc), x) {
					t.Errorf("%s: la testa senza esito scrive %q come un dato della RFQ", dove, x)
				}
			}
			if frammento {
				mpFrammentoAmmesso(t, dove, rec.Body.Bytes(), id)
			}
			// il log ha la tipologia (D-18 = A [U])
			if !strings.Contains(registro.String(), "tipo="+c.tipo) || !strings.Contains(registro.String(), "frammento="+fmt.Sprint(frammento)) {
				t.Errorf("%s: il log non ha la tipologia: %s", dove, registro.String())
			}
			// nessuna scrittura
			if x := ini.tx; x.exec != 0 || x.commit != 0 {
				t.Errorf("%s: %d Exec, %d Commit all'apertura", dove, x.exec, x.commit)
			}
		}
		for _, a := range []string{"data-ma-pagina", "data-ma-calcolo", "data-ma-errore", "data-ma-vista", "data-ma-prodotto"} {
			if maRA(corpi[0], a) != maRA(corpi[1], a) {
				t.Errorf("%s: %s %q nella pagina, %q nel frammento", c.nome, a, maRA(corpi[0], a), maRA(corpi[1], a))
			}
		}
	}
	// 404 e 400 uguali nella pagina e nel frammento, senza la pagina d'errore applicativo
	for _, frammento := range []bool{false, true} {
		ini := &mpIniziatore{tx: &mpTx{getThread: pgx.ErrNoRows}}
		rec := mdChiama(t, s, motorea_web.Servizio{Regole: mwPronte(time.Second), Pool: ini, Log: s.Log}, id, "/motore-a", frammento)
		if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "data-ma-pagina") || !strings.Contains(rec.Body.String(), testoMotoreARFQPagina) {
			t.Errorf("RFQ che non c'è, frammento %v: %d %.200s", frammento, rec.Code, rec.Body.String())
		}
		ini = &mpIniziatore{tx: &mpTx{}}
		rec = mdChiama(t, s, motorea_web.Servizio{Regole: mwPronte(time.Second), Pool: ini, Log: s.Log}, "xyz", "/motore-a", frammento)
		if rec.Code != http.StatusBadRequest || len(ini.opzioni) != 0 {
			t.Errorf("id non valido, frammento %v: %d, %d transazioni", frammento, rec.Code, len(ini.opzioni))
		}
	}
}

// La tipologia di ogni errore del servizio, anche avvolto, e dei due errori del contesto.
func TestMotoreAD12TipoErrore(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/thread/x/motore-a", nil)
	for err, tipo := range map[error]string{
		fmt.Errorf("a: %w", motorea_web.ErrRegole):      erroreRegole,
		fmt.Errorf("a: %w", motorea_web.ErrOccupato):    erroreOccupato,
		fmt.Errorf("a: %w", motorea_web.ErrTempo):       erroreTempo,
		context.DeadlineExceeded:                        erroreTempo,
		fmt.Errorf("a: %w", context.Canceled):           erroreAnnullata,
		fmt.Errorf("a: %w", motorea_web.ErrPanic):       errorePanic,
		fmt.Errorf("a: %w", motorea_web.ErrVersione):    erroreVersione,
		fmt.Errorf("a: %w", motorea_web.ErrValutazione): erroreValutazione,
		fmt.Errorf("a: %w", motorea_web.ErrLettura):     erroreLettura,
		errors.New("un errore ACME qualsiasi"):          erroreAltro,
	} {
		if got := tipoErroreMotoreA(req, err); got != tipo {
			t.Errorf("%v: %q, atteso %q", err, got, tipo)
		}
	}
	ctx, chiudi := context.WithCancel(context.Background())
	chiudi()
	if got := tipoErroreMotoreA(req.WithContext(ctx), errors.New("lettura interrotta")); got != erroreAnnullata {
		t.Errorf("richiesta chiusa: %q", got)
	}
	// una pagina senza esito arrivata a rendiMotoreA senza la tipologia la dichiara non dichiarata, mai vuota
	if v := vistaMotoreADa(req, &motorea_web.Pagina{Errore: "x"}, ""); v.TipoErrore != erroreNonDichiarato || v.StatoCalcolo() != calcoloErrore {
		t.Errorf("senza tipologia: %q %q", v.TipoErrore, v.StatoCalcolo())
	}
}

// Init compone la pagina con funzioniMotoreA (R-06, direttiva 12); un Server senza Init non ha il template, e lo dice con
// un errore, mai con un panic.
func TestMotoreAD12InitComponeLaPagina(t *testing.T) {
	if serverTest(t).pagine["motore_a.html"] == nil {
		t.Error("Init non compone motore_a.html")
	}
	if _, err := (&Server{}).templateMotoreA(); err == nil {
		t.Error("senza Init templateMotoreA non dà errore")
	}
}

// Le guardie sul testo del modulo e sui punti d'aggancio: NON provano il comportamento (lo prova il browser, Scenario H,
// NON ESEGUITA in questa corsia), servono a non perdere per strada gli attributi che il modulo legge.
func TestMotoreAD12AgganciDelModulo(t *testing.T) {
	b, err := fs.ReadFile(risorse.FS, "web/static/motore_a.mjs")
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, v := range []string{"function allineaIndirizzo(r)", "history.replaceState(", `r.dataset.maCalcolo === "errore"`, "function controllaRilettura()",
		"controllaRilettura();", "allineaIndirizzo(r);"} {
		if !strings.Contains(js, v) {
			t.Errorf("motore_a.mjs senza %q", v)
		}
	}
	// revisione D-E del 13/10, rilievi 3 e 4: l'esito della rilettura si scrive prima di avvia(), e la frase della pagina
	// senza esito esce dall'avviso quando la pagina torna con l'esito. Ancora testo, non comportamento.
	if i := strings.Index(js, `"htmx:afterSwap"`); i < 0 {
		t.Error("motore_a.mjs non ascolta più htmx:afterSwap")
	} else {
		dopo := js[i:]
		if c, a := strings.Index(dopo, "controllaRilettura();"), strings.Index(dopo, "avvia();"); c < 0 || a < 0 || c > a {
			t.Error("dopo lo scambio di #ma controllaRilettura() non viene più prima di avvia()")
		}
	}
	if !strings.Contains(js, "function togliPaginaSenzaEsito()") || !strings.Contains(js, "togliPaginaSenzaEsito();") ||
		!strings.Contains(js, `"data-ma-esito-prima"`) {
		t.Error("motore_a.mjs non toglie più la frase della pagina senza esito quando l'esito torna")
	}
	if strings.Count(js, "history.replaceState(") != 1 || strings.Contains(js, "history.pushState(") || strings.Contains(js, "location.assign(") ||
		strings.Contains(js, "location.replace(") || strings.Contains(js, "location.reload(") {
		t.Error("motore_a.mjs cambia l'indirizzo fuori da allineaIndirizzo, o ricarica la pagina da sé")
	}
}
