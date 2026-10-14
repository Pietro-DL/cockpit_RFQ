package web

// L1 — la voce della tabella delle classi del cliente nella pagina della Distinta (decisione dell'utente del 14/10, [U]):
// sotto il codice della casella una riga corta «categoria · descrizione · fonte» (escapata, con il testo intero nel
// title), nell'anteprima la riga «Normativa» con valore, descrizione e fonte; il tipo proposto dalla grammatica con la
// sua etichetta e, con la minuteria proposta, il segno «minuteria ?» al posto di «commerciale ?». Nessuna percentuale.
//
// Il caso è e1 (una struttura candidata dal file) costruito in Go, con due nodi cambiati: #30 (7129020) foglia con una
// voce minuteria e #20 (7120110) foglia con una voce commerciale. Solo dati ACME inventati: codici 712xxxx, acme.example.

import (
	"net/http"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"promatec/cockpit/internal/core/valutazione"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// maCasoE1Classi: e1 con le voci della tabella delle classi su #30 (minuteria) e #20 (commerciale). La descrizione di
// #30 ha dei caratteri da escapare, per provare che la riga è solo testo.
func maCasoE1Classi(t *testing.T) casoMotoreA {
	t.Helper()
	c := maCasoE1()
	cambiati := 0
	for i := range c.Pagina.Esito.ProdottiValutati {
		for j := range c.Pagina.Esito.ProdottiValutati[i].StruttureCandidate {
			nodi := c.Pagina.Esito.ProdottiValutati[i].StruttureCandidate[j].Nodi
			for k := range nodi {
				n := &nodi[k]
				switch n.Nodo.Rif {
				case maRN("30", "#30"):
					n.Tipo, n.OrigineTipo = "commerciale", valutazione.OrigineTipoPropostoDallaGrammatica
					n.Classe = &valutazione.ClasseNodo{Categoria: "minuteria", Descrizione: "Vite <M6> & dado ACME", Fonte: "norma ACME 9020", Valore: "9"}
					cambiati++
				case maRN("30", "#20"):
					n.Tipo, n.OrigineTipo = "commerciale", valutazione.OrigineTipoPropostoDallaGrammatica
					n.Classificazione.Proposta = valutazione.CategoriaCommerciale
					n.Classe = &valutazione.ClasseNodo{Categoria: "commerciale", Descrizione: "Boccola ACME", Valore: "0"}
					cambiati++
				}
			}
		}
	}
	if cambiati != 2 {
		t.Fatalf("e1: cambiati %d nodi, attesi #20 e #30", cambiati)
	}
	return c
}

// maRCasella: la casella (li dell'albero) del nodo nel frammento; la prima, se il nodo sta in più schede.
func maRCasella(t *testing.T, doc *html.Node, rif string) *html.Node {
	t.Helper()
	for _, li := range maRTutti(doc, maRCon("li", "data-ma-rif")) {
		if maRA(li, "data-ma-rif") == rif {
			return li
		}
	}
	t.Fatalf("nessuna casella per %s", rif)
	return nil
}

// maRPannello: il pannello dell'anteprima del nodo.
func maRPannello(t *testing.T, doc *html.Node, rif string) *html.Node {
	t.Helper()
	for _, p := range maRTutti(doc, maRCon("section", "data-ma-pannello")) {
		if strings.HasSuffix(maRA(p, "data-ma-pannello"), "|"+rif) {
			return p
		}
	}
	t.Fatalf("nessun pannello per %s", rif)
	return nil
}

// maRPrimo: il primo elemento sotto n con la classe c (dentro la casella, non nei figli: la casella del nodo è la sua
// ma-nodo-box); nil senza.
func maRPrimo(n *html.Node, c string) *html.Node {
	for _, x := range maRTutti(n, func(x *html.Node) bool { return maRClasse(x, "ma-nodo-box") }) {
		if xs := maRTutti(x, func(y *html.Node) bool { return maRClasse(y, c) }); len(xs) > 0 {
			return xs[0]
		}
		return nil
	}
	return nil
}

func TestMotoreALaVoceMinuteriaDellaNormativa(t *testing.T) {
	s := maRServer(t)
	c := maCasoE1Classi(t)
	r := maRRendi(s, "e1_classi", &c, motorea_web.VistaDistinta, maRProdotti(&c)[0], true)
	if r.Pagina.Code != http.StatusOK || r.Frammento.Code != http.StatusOK {
		t.Fatalf("pagina %d, frammento %d", r.Pagina.Code, r.Frammento.Code)
	}
	corpo := r.Frammento.Body.String()
	if strings.Contains(corpo, "<M6>") || !strings.Contains(corpo, "&lt;M6&gt;") {
		t.Error("la descrizione della voce non è escapata")
	}
	doc := maRDoc(t, r.Frammento.Body.Bytes())
	li := maRCasella(t, doc, maRN("30", "#30"))
	riga := maRPrimo(li, "ma-classe")
	if riga == nil {
		t.Fatal("#30: nessuna riga della normativa sotto il codice")
	}
	if got := maRTesto(riga); got != "minuteria · Vite <M6> & dado ACME · norma ACME 9020" {
		t.Errorf("#30: riga della normativa %q", got)
	}
	if ti := maRA(riga, "title"); !strings.Contains(ti, "normativa del cliente") || !strings.Contains(ti, "norma ACME 9020") || !strings.Contains(ti, "valore 9") {
		t.Errorf("#30: title della riga %q", ti)
	}
	tipo := maRPrimo(li, "ma-tipo")
	if tipo == nil || !strings.Contains(maRTesto(tipo), "minuteria") || strings.Contains(maRTesto(tipo), "commerciale") || !strings.Contains(maRTesto(tipo), "?") ||
		!maRClasse(tipo, "ma-tipo--proposto") || !strings.Contains(maRA(tipo, "title"), "proposto dalla normativa del cliente") {
		t.Errorf("#30: il segno del tipo %q (title %q), atteso «minuteria ?»", maRTesto(tipo), maRA(tipo, "title"))
	}
	p := maRPannello(t, doc, maRN("30", "#30"))
	testo := maRTesto(p)
	if !strings.Contains(testo, "Normativa") || !strings.Contains(testo, "Vite <M6> & dado ACME") || !strings.Contains(testo, "norma ACME 9020") ||
		!strings.Contains(testo, "proposto dalla normativa del cliente") || !strings.Contains(testo, "minuteria proposta") {
		t.Errorf("#30: l'anteprima non dice la normativa: %.600s", testo)
	}
}

func TestMotoreALaVoceCommercialeDellaNormativa(t *testing.T) {
	s := maRServer(t)
	c := maCasoE1Classi(t)
	r := maRRendi(s, "e1_classi", &c, motorea_web.VistaDistinta, maRProdotti(&c)[0], true)
	if r.Frammento.Code != http.StatusOK {
		t.Fatalf("frammento %d", r.Frammento.Code)
	}
	doc := maRDoc(t, r.Frammento.Body.Bytes())
	li := maRCasella(t, doc, maRN("30", "#20"))
	riga := maRPrimo(li, "ma-classe")
	if riga == nil || maRTesto(riga) != "commerciale · Boccola ACME" {
		t.Fatalf("#20: riga della normativa %v", riga)
	}
	tipo := maRPrimo(li, "ma-tipo")
	if tipo == nil || !strings.Contains(maRTesto(tipo), "commerciale") || strings.Contains(maRTesto(tipo), "minuteria") || !strings.Contains(maRTesto(tipo), "?") {
		t.Errorf("#20: il segno del tipo %v, atteso «commerciale ?»", tipo)
	}
	testo := maRTesto(maRPannello(t, doc, maRN("30", "#20")))
	if !strings.Contains(testo, "Normativa") || !strings.Contains(testo, "Boccola ACME") || !strings.Contains(testo, "commerciale proposto (non confermato)") {
		t.Errorf("#20: l'anteprima non dice la normativa: %.600s", testo)
	}
	// Un nodo senza voce non ha la riga.
	if maRPrimo(maRCasella(t, doc, maRN("30", "#10")), "ma-classe") != nil {
		t.Error("#10: una riga della normativa senza voce")
	}
}
