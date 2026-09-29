//go:build integrazione

// L4 — Admin › Anagrafica › Riconoscimento, le voci della posta della 4.13b (le scelte del 28/09 confermate
// il 29/09): i mittenti di sistema, il numero d'ordine del cliente e l'ordine da verificare. Si salvano dal
// form e dal JSON passando dalla stessa porta (`regole.ValidaRegole`), si rifiutano con il motivo, il form
// riletto le riporta e salvato così com'è non le cambia; il banco di prova le usa. Le regole le scrive una
// persona, una alla volta (domanda 14 = A): qui c'è solo il codice che lo permette, con un cliente inventato.
package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/db"
)

// formVociPosta è il form di Riconoscimento con le voci nuove: due mittenti di sistema e la riga vuota
// per aggiungerne, il numero d'ordine, la voce più debole dell'ordine.
func formVociPosta() url.Values {
	return url.Values{
		"mit_mittente":         {"avvisi@acme.example", "srv.acme.example", ""},
		"mit_evento":           {"ALTRO", "ORDINE", "ALTRO"},
		"mit_descrizione":      {"avvisi del gestionale", "", ""},
		"mit_esempio":          {"avvisi@acme.example", "ordini@srv.acme.example", ""},
		"ord_regex":            {`\bODA_\d{7}\b`},
		"ord_descrizione":      {"ordini ODA"},
		"ord_esempio":          {"ODA_0001234"},
		"ordine_da_verificare": {"1"},
	}
}

func TestLeVociDellaPostaSiSalvanoESiRifiutanoConIlMotivo(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("ACME S.p.A.", "ACME", "acme.example")
	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")
	percorso := "/admin/anagrafica/" + acme.String()

	resp, html := ad.fai(http.MethodPost, percorso+"/regole/form", formVociPosta(), false)
	if resp.StatusCode != 200 || !strings.Contains(leggibile(html), "Regole salvate") {
		t.Fatalf("salvataggio rifiutato (%d): %s", resp.StatusCode, estrai(html, "errore"))
	}
	salvate := func() regole.Regole {
		r, _ := regole.LeggiRegole(b.cliente(acme).Regole)
		return r
	}
	r := salvate()
	if len(r.MittentiSistema) != 2 || r.MittentiSistema[0].Mittente != "avvisi@acme.example" || r.MittentiSistema[0].Descrizione != "avvisi del gestionale" ||
		r.MittentiSistema[1].Evento != regole.EventoMittenteOrdine || r.MittentiSistema[1].Esempio != "ordini@srv.acme.example" {
		t.Errorf("mittenti salvati: %+v", r.MittentiSistema)
	}
	if r.NumeroOrdine == nil || r.NumeroOrdine.Regex != `\bODA_\d{7}\b` || r.NumeroOrdine.Esempio != "ODA_0001234" {
		t.Errorf("numero d'ordine salvato: %+v", r.NumeroOrdine)
	}
	if !r.OrdineDaVerificare || r.NumeroOrdineAnticipato {
		t.Errorf("le caselle dell'ordine: da verificare %v, anticipato %v", r.OrdineDaVerificare, r.NumeroOrdineAnticipato)
	}

	// il form riletto riporta le voci, con la tendina sull'evento giusto e la diagnosi ✓
	_, pagina := ad.fai(http.MethodGet, "/admin/anagrafica?cliente="+acme.String()+"&sez=riconoscimento", nil, false)
	for _, atteso := range []string{`name="mit_mittente" value="avvisi@acme.example"`, `name="mit_mittente" value="srv.acme.example"`,
		`<option value="ORDINE" selected>`, `name="ord_esempio" value="ODA_0001234"`, `name="ordine_da_verificare" value="1" checked`,
		"mittente di sistema 1", "mittente di sistema 2", "numero d&#39;ordine"} {
		if !strings.Contains(pagina, atteso) {
			t.Errorf("la pagina non contiene %q:\n%s", atteso, estratto(pagina, "Mittenti di sistema"))
		}
	}
	// e il form salvato di nuovo così com'è non cambia niente
	prima := string(b.cliente(acme).Regole)
	if _, html := ad.fai(http.MethodPost, percorso+"/regole/form", formVociPosta(), false); !strings.Contains(leggibile(html), "Regole salvate") {
		t.Fatalf("secondo salvataggio rifiutato: %s", estrai(html, "errore"))
	}
	if dopo := string(b.cliente(acme).Regole); dopo != prima {
		t.Errorf("il secondo salvataggio ha cambiato le regole:\n%s\n%s", prima, dopo)
	}

	// rifiutata con il motivo, dal form: l'esempio è di un altro indirizzo. In database resta quello di prima
	rotto := formVociPosta()
	rotto["mit_esempio"] = []string{"buyer@acme.example", "ordini@srv.acme.example", ""}
	_, html = ad.fai(http.MethodPost, percorso+"/regole/form", rotto, false)
	if l := leggibile(html); !strings.Contains(l, "mittente di sistema 1") || !strings.Contains(l, "non corrisponde") {
		t.Errorf("il rifiuto non dice il motivo: %s", estrai(html, "errore"))
	}
	if dopo := string(b.cliente(acme).Regole); dopo != prima {
		t.Fatalf("una regola rifiutata è entrata: %s", dopo)
	}
	// e dal JSON: un evento che lo schema non conosce, e un numero d'ordine senza esempio
	for _, c := range []struct{ json, atteso string }{
		{`{"mittenti_sistema":[{"mittente":"avvisi@acme.example","evento":"SOLLECITO","esempio":"avvisi@acme.example"}]}`, "non è previsto"},
		{`{"numero_ordine":{"regex":"\\bODA_\\d{7}\\b"}}`, "manca l'esempio"},
	} {
		_, html := ad.fai(http.MethodPost, percorso+"/regole", url.Values{"regole": {c.json}}, true)
		if !strings.Contains(leggibile(html), c.atteso) {
			t.Errorf("%s: la schermata non dice %q:\n%s", c.json, c.atteso, primi400(html))
		}
		if dopo := string(b.cliente(acme).Regole); dopo != prima {
			t.Fatalf("una regola rifiutata dal JSON è entrata: %s", dopo)
		}
	}

	// una riga scritta a mano nel database con un evento che lo schema non conosce: la pagina la segna ✗ e la
	// tendina la tiene, così salvare il form senza toccarla non la cambia in silenzio in ALTRO
	if _, err := b.q.SetRegoleCliente(b.ctx, db.SetRegoleClienteParams{ClienteID: acme, Regole: []byte(
		`{"mittenti_sistema":[{"mittente":"avvisi@acme.example","evento":"SOLLECITO","esempio":"avvisi@acme.example"}]}`)}); err != nil {
		t.Fatal(err)
	}
	_, pagina = ad.fai(http.MethodGet, "/admin/anagrafica?cliente="+acme.String()+"&sez=riconoscimento", nil, false)
	if !strings.Contains(pagina, `<option value="SOLLECITO" selected>`) || !strings.Contains(leggibile(pagina), "non è previsto") {
		t.Errorf("la riga scritta a mano: %s", estratto(pagina, "Mittenti di sistema"))
	}
}

// Il banco di prova usa la voce: lo stesso testo, dal mittente di sistema, non propone niente e dice
// perché; senza il mittente (la controprova) propone una RFQ nuova come prima.
func TestIlBancoDiProvaConosceIMittentiDiSistema(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("ACME S.p.A.", "ACME", "acme.example")
	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")
	percorso := "/admin/anagrafica/" + acme.String()
	if _, html := ad.fai(http.MethodPost, percorso+"/regole/form", formVociPosta(), false); !strings.Contains(leggibile(html), "Regole salvate") {
		t.Fatalf("regole non salvate: %s", estrai(html, "errore"))
	}
	const testo = "RFQ 7120001 - richiesta d'offerta\nNuova RFQ disponibile per il codice 7120001."

	_, pagina := ad.fai(http.MethodPost, percorso+"/prova", url.Values{"testo": {testo}, "mittente": {"avvisi@acme.example"}}, true)
	l := leggibile(pagina)
	if !strings.Contains(l, "mittente di sistema del cliente: avvisi@acme.example") || !strings.Contains(pagina, "<strong>ignora</strong>") {
		t.Errorf("il banco con il mittente di sistema:\n%s", estratto(pagina, "esito proposto"))
	}
	if !strings.Contains(pagina, `name="mittente" value="avvisi@acme.example"`) {
		t.Errorf("il mittente provato non torna nel campo: %s", estrai(pagina, `name="mittente"`))
	}
	_, pagina = ad.fai(http.MethodPost, percorso+"/prova", url.Values{"testo": {testo}}, true)
	if strings.Contains(leggibile(pagina), "mittente di sistema del cliente") || !strings.Contains(pagina, "<strong>nuova_rfq</strong>") {
		t.Errorf("il banco senza mittente:\n%s", estratto(pagina, "esito proposto"))
	}
}

// Ritocco della 4.13b: un mittente di sistema che prenderebbe la posta dei buyer non si salva, né dal form né
// dal JSON, e il rifiuto dice perché. Un dominio intero lo rifiuta la convalida senza database; un dominio
// principale del cliente (anche con tre parti) e il dominio di un suo contatto li rifiuta il salvataggio, che
// legge l'Anagrafica. In database resta quello di prima. L'host di un sistema registrato dentro il dominio del
// cliente si salva: il riconoscimento della controparte porta la sua posta al cliente per quel dominio, e la
// posta diventa un avviso, mentre la richiesta del buyer resta una richiesta. La controprova: lo stesso host
// del contatto, per un cliente che non ha quel contatto, si salva.
func TestUnMittenteDiSistemaSullaPostaDeiBuyerNonSiSalva(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("ACME S.p.A.", "ACME", "acme.example")
	for _, d := range []string{"acme-italia.gruppo.example", "srv.acme.example"} {
		if err := AggiungiDominio(b.ctx, b.q, d, acme); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.q.InsertBuyer(b.ctx, db.InsertBuyerParams{ClienteID: acme, Cognome: "Rossi", Nome: txtN("Mario", 60),
		Email: txtN("mario.rossi@uffici.acme.example", 120), Tipo: db.TipoBuyerBuyer, Origine: db.OrigineAnagraficaManuale}); err != nil {
		t.Fatal(err)
	}
	beta := b.unCliente("Beta S.r.l.", "BETA", "beta.example")
	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")
	form := func(mittente, esempio string) url.Values {
		return url.Values{"mit_mittente": {mittente, ""}, "mit_evento": {"ALTRO", "ALTRO"},
			"mit_descrizione": {"", ""}, "mit_esempio": {esempio, ""}}
	}
	prima := string(b.cliente(acme).Regole)
	for _, c := range []struct {
		mittente, esempio string
		attesi            []string
	}{
		{"acme.example", "avvisi@acme.example", []string{"mittente di sistema 1", "«acme.example» è un dominio intero"}},
		{"acme-italia.gruppo.example", "avvisi@acme-italia.gruppo.example",
			[]string{"mittente di sistema 1", "«acme-italia.gruppo.example» è un dominio di questo cliente"}},
		{"uffici.acme.example", "avvisi@uffici.acme.example",
			[]string{"mittente di sistema 1", "è il dominio di Rossi Mario (mario.rossi@uffici.acme.example), un contatto di questo cliente"}},
	} {
		_, html := ad.fai(http.MethodPost, "/admin/anagrafica/"+acme.String()+"/regole/form", form(c.mittente, c.esempio), false)
		l := leggibile(html)
		for _, atteso := range c.attesi {
			if !strings.Contains(l, atteso) {
				t.Errorf("%s dal form: il rifiuto non dice %q: %s", c.mittente, atteso, estrai(html, "errore"))
			}
		}
		if strings.Contains(l, "Regole salvate") {
			t.Errorf("%s dal form: salvato", c.mittente)
		}
		j := fmt.Sprintf(`{"mittenti_sistema":[{"mittente":%q,"evento":"ALTRO","esempio":%q}]}`, c.mittente, c.esempio)
		_, html = ad.fai(http.MethodPost, "/admin/anagrafica/"+acme.String()+"/regole", url.Values{"regole": {j}}, true)
		l = leggibile(html)
		for _, atteso := range c.attesi {
			if !strings.Contains(l, atteso) {
				t.Errorf("%s dal JSON: il rifiuto non dice %q:\n%s", c.mittente, atteso, primi400(html))
			}
		}
		if dopo := string(b.cliente(acme).Regole); dopo != prima {
			t.Fatalf("%s: una regola rifiutata è entrata: %s", c.mittente, dopo)
		}
	}

	// la controprova: l'host del contatto di ACME, per BETA, che non ha quel contatto
	if _, html := ad.fai(http.MethodPost, "/admin/anagrafica/"+beta.String()+"/regole/form",
		form("uffici.acme.example", "avvisi@uffici.acme.example"), false); !strings.Contains(leggibile(html), "Regole salvate") {
		t.Errorf("per BETA l'host si rifiuta: %s", estrai(html, "errore"))
	}

	// l'host del sistema, registrato dentro il dominio di ACME, si salva
	if _, html := ad.fai(http.MethodPost, "/admin/anagrafica/"+acme.String()+"/regole/form",
		form("srv.acme.example", "avvisi@srv.acme.example"), false); !strings.Contains(leggibile(html), "Regole salvate") {
		t.Fatalf("l'host del sistema si rifiuta: %s", estrai(html, "errore"))
	}
	// e la sua posta arriva alle regole di ACME per quel dominio, e diventa un avviso; la richiesta del buyer no
	avviso := b.posta("entrata", "avvisi@srv.acme.example", "RFQ 7120001 - richiesta d'offerta", false)
	if tipo, via := b.controparteDi(avviso); tipo != "cliente" || via != "dominio" {
		t.Errorf("la posta del sistema: controparte %s via %s", tipo, via)
	}
	if ev, atto := b.eventoSalvato(avviso); ev != classificazione.EventoAltro || atto != classificazione.AttoNotifica {
		t.Errorf("la posta del sistema: evento %s, atto %s", ev, atto)
	}
	richiesta := b.posta("entrata", "mario.rossi@uffici.acme.example", "RFQ 7120001 - richiesta d'offerta", false)
	if ev, atto := b.eventoSalvato(richiesta); ev != classificazione.EventoNuovaRFQ {
		t.Errorf("la richiesta del buyer: evento %s, atto %s", ev, atto)
	}
}
