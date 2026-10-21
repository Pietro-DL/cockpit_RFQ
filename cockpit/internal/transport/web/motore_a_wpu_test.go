package web

// L1 — giro 5, A1d, notte 14/10, WP-U: la Distinta nuova del motore A resa dal template vero, sui casi ACME cambiati in
// memoria (come TestMotoreARamiSenzaCasoSiRendono: i file JSON dei casi non cambiano). Diagnosi UI del 14/10:
//   - U1 (G2): il collegamento «Distinta nuova (Motore A)» nelle pagine Richieste e RFQ, con la guardia della rotta
//     (distintaNuovaVisibile: flag acceso e admin);
//   - U2 (G4): il perché dell'ancoraggio accanto al prodotto (#ma-perche-prodotto), alla RFQ (#ma-perche-rfq) e al file
//     (la scheda di Da smistare), con i codici che WP-A introduce (radice_dal_nome_file, namespace_non_confrontabile);
//   - U3 (G1): sotto un prodotto con struttura «nessuna» gli STEP della RFQ (#ma-step-rfq) con il testo delle radici e
//     i gesti esistenti: «Autorizza come STEP di …» (data-ma-azione="step" con il documento) e «Usa come STEP di …»
//     (data-ma-azione="usa-step", con la correzione del codice dichiarata); spenti in sola lettura; la pagina del
//     Fascicolo sul prodotto; «Autorizza» anche su un documento del prodotto fra gli altri candidati alla fonte;
//   - U4: gli elementi della mail (natura_non_file) marcati data-ma-elemento-mail, l'interruttore «mostra elementi della
//     mail», la regola del CSS che li nasconde.
// Ogni resa passa anche dai controlli generici della pagina (maRControlliGenerici: rotte della lista chiusa, nessun Rif
// dove va un ID, nessun valore fra quadre, id unici).
//
// Sta a L1: nessun database, nessun server. Solo dati ACME inventati (codici 712xxxx e ACM-nnnn).

import (
	"bytes"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/net/html"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/db"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// wpuDocSTEP: il documento delle evidenze di uno STEP con i nodi dati; le radici hanno l'unità del campo id nel
// contesto radice_step, con il testo dato.
func wpuDocSTEP(nodi []string, radici map[string]string) evidenze.DocumentoEvidenze {
	var d evidenze.DocumentoEvidenze
	for _, k := range nodi {
		id := "e:step:" + k
		d.Entita = append(d.Entita, evidenze.EntitaLocale{ID: id, Tipo: "nodo_step", ChiaveOriginale: k})
		contesto, testo := evidenze.ContestoNodoSTEP, "nodo "+k
		if r, ok := radici[k]; ok {
			contesto, testo = evidenze.ContestoRadiceSTEP, r
		}
		d.Unita = append(d.Unita, evidenze.UnitaEvidenza{ID: "u:step:" + k + ":id", EntitaID: id, Testo: testo,
			Selettore: evidenze.Selettore{Contesto: contesto, Campo: evidenze.CampoFonte{Variante: evidenze.VarianteStep, Valore: "id"}}})
	}
	return d
}

// wpuCasoSenzaStruttura: e10 (il prodotto 7120100 senza strutture, i file da smistare, l'immagine inline 3c) con
// quello che una RFQ come quella della diagnosi porta in più, in forma ACME:
//   - 3e «ACM-9001.stp»: uno STEP con una proposta aperta di 3D con un altro codice (→ nessun gesto, perché il prodotto
//     ha già il 3D corrente 5e; senza 5e «Usa come STEP», con la correzione: TestMotoreAWPUUsaSenza3DCorrente);
//   - 3f «7120100_B.stp»: uno STEP già documento corrente del prodotto (→ «Autorizza come STEP»);
//   - le diagnostiche dell'ancoraggio: target_senza_struttura del prodotto, radice_dal_nome_file della RFQ;
//   - i motivi dell'ancoraggio di 3a (Scansione_ACME.pdf, in Da smistare) e di 3e.
func wpuCasoSenzaStruttura(t *testing.T) *casoMotoreA {
	t.Helper()
	c := leggiCasoMotoreA(t, "e10")
	e := c.Pagina.Esito
	p := &e.ProdottiValutati[0]
	if p.Struttura != ancoraggio.StatoStrutturaNessuna || p.ComponenteID == nil || *p.ComponenteID != maU("10") {
		t.Fatalf("e10: il prodotto non è quello atteso (struttura %q)", p.Struttura)
	}
	for _, a := range []struct{ xx, nome string }{{"3e", "ACM-9001.stp"}, {"3f", "7120100_B.stp"}} {
		c.Foto.Allegati = append(c.Foto.Allegati, fotorfq.Allegato{ID: maU(a.xx), MessaggioID: maU("03"), NomeFile: a.nome,
			Estensione: maStr("stp"), Natura: string(db.NaturaAllegatoFile), Origine: string(db.OrigineAllegatoOutlook),
			Stato: string(db.StatoAllegatoAnalizzato), Sha256: maStr(maShaA(a.xx)), RicevutoIl: maPresa})
	}
	e.File = append(e.File,
		valutazione.FileInterpretato{AllegatoID: maU("3e"), Disponibilita: ancoraggio.DisponibilitaDisponibile,
			Documento: wpuDocSTEP([]string{"#1", "#2", "#3"}, map[string]string{"#1": "ACM-5001_01  -  staffa sinistra"})},
		valutazione.FileInterpretato{AllegatoID: maU("3f"), Disponibilita: ancoraggio.DisponibilitaDisponibile,
			Documento: wpuDocSTEP([]string{"#1", "#2"}, map[string]string{"#1": "ACM-5002_00  -  staffa destra"})})
	c.Foto.Proposte = append(c.Foto.Proposte, fotorfq.PropostaAttuale{ID: maU("7e"), AllegatoID: maU("3e"), ThreadID: maUp("01"),
		Tipo: string(db.TipoDocumentoCad3d), Codice: maStr("ACM-9001"), Fonte: string(db.FontePropostaNomeFile), Stato: string(db.StatoPropostaAperta)})
	c.Foto.Documenti = append(c.Foto.Documenti, fotorfq.DocumentoConfermato{ID: maU("5e"), ThreadID: maU("01"), ComponenteID: maUp("10"),
		Tipo: string(db.TipoDocumentoCad3d), Codice: maStr("7120100"), NomeFile: "7120100_B.stp", Estensione: "stp", Sha256: maShaA("3f"),
		StatoNas: string(db.StatoNasScritto), ConfermatoDa: maU("b0"), ConfermatoIl: maPresa, Allegati: []uuid.UUID{maU("3f")}})
	e.Ancoraggi.Diagnostiche = []evidenze.Diagnostica{
		{Codice: ancoraggio.CodiceTargetSenzaStruttura, Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaDati,
			Messaggio: "Messaggio ACME: nessun nodo ha la base di 7120100.", Rif: []string{p.Rif}},
		{Codice: "ancoraggio.radice_dal_nome_file", Gravita: evidenze.GravitaNota, Natura: evidenze.NaturaDati,
			Messaggio: "Messaggio ACME: radice dal nome di un file.", Rif: []string{"allegato:" + maU("3e").String()}},
	}
	e.Ancoraggi.File = []ancoraggio.AncoraggioFile{
		{AllegatoID: maU("3a"), Motivi: []string{ancoraggio.MotivoAncoraggioTargetSenzaStruttura, "namespace_non_confrontabile"}},
		{AllegatoID: maU("3e"), Motivi: []string{ancoraggio.MotivoAncoraggioNessunaLettura}},
	}
	return c
}

// wpuPrimo: il primo nodo che soddisfa ok, o la prova si ferma.
func wpuPrimo(t *testing.T, r *maRResa, radice *html.Node, che string, ok func(*html.Node) bool) *html.Node {
	t.Helper()
	nn := maRTutti(radice, ok)
	if len(nn) == 0 {
		t.Fatalf("%s: manca %s", r, che)
	}
	return nn[0]
}

func TestMotoreAWPUPercheEStepDellaRFQ(t *testing.T) {
	s := maRServer(t)
	c := wpuCasoSenzaStruttura(t)
	thread := c.Pagina.ThreadID
	rotte := maRRotte(t, thread)
	p := c.Pagina.Esito.ProdottiValutati[0]
	for _, vis := range []string{motorea_web.VistaDistinta, motorea_web.VistaDocumenti} {
		for _, scrive := range []bool{true, false} {
			r := maRRendi(s, "e10+wpu", c, vis, p.Rif, scrive)
			maRControlliGenerici(t, r, rotte)
			doc := maRDoc(t, r.Frammento.Body.Bytes())

			// U2: il perché del prodotto, della RFQ e del file
			perche := wpuPrimo(t, r, doc, "#ma-perche-prodotto", func(n *html.Node) bool { return maRA(n, "id") == "ma-perche-prodotto" })
			if tx := maRTesto(perche); !strings.Contains(tx, "prodotto senza struttura") || !strings.Contains(tx, "nessuno STEP è stato autorizzato") ||
				!strings.Contains(tx, "Messaggio ACME: nessun nodo ha la base di 7120100.") {
				t.Errorf("%s: #ma-perche-prodotto = %q", r, tx)
			}
			rfq := wpuPrimo(t, r, doc, "#ma-perche-rfq", func(n *html.Node) bool { return maRA(n, "id") == "ma-perche-rfq" })
			if tx := maRTesto(rfq); !strings.Contains(tx, "radice dal nome del file") || strings.Contains(tx, "prodotto senza struttura") {
				t.Errorf("%s: #ma-perche-rfq = %q", r, tx)
			}
			scheda := wpuPrimo(t, r, doc, "la scheda di 3a in Da smistare", func(n *html.Node) bool {
				return n.Data == "article" && maRA(n, "data-ma-allegato") == maU("3a").String()
			})
			if tx := maRTesto(scheda); !strings.Contains(tx, "codice di un'altra famiglia") || !strings.Contains(tx, "prodotto senza struttura") {
				t.Errorf("%s: la scheda di 3a non dice il perché: %q", r, tx)
			}
			for _, li := range maRTutti(doc, maRCon("li", "data-ma-motivo-ancoraggio")) {
				if b := maRTutti(li, maRCon("b", "title")); len(b) == 0 || !strings.HasPrefix(maRA(b[0], "title"), "codice = ") {
					t.Errorf("%s: una spiegazione senza il codice grezzo nel title", r)
				}
			}

			// U3: gli STEP della RFQ sotto il prodotto
			step := wpuPrimo(t, r, doc, "#ma-step-rfq", func(n *html.Node) bool { return maRA(n, "id") == "ma-step-rfq" })
			if !maRDentro(step, wpuPrimo(t, r, doc, "#ma-fonte", func(n *html.Node) bool { return maRA(n, "id") == "ma-fonte" })) {
				t.Errorf("%s: #ma-step-rfq fuori dall'analisi del prodotto", r)
			}
			// maRTesto riduce gli spazi, come il browser
			if tx := maRTesto(step); !strings.Contains(tx, "ACM-5001_01 - staffa sinistra") || !strings.Contains(tx, "ACM-5002_00 - staffa destra") ||
				!strings.Contains(tx, "nessun codice letto dal file") {
				t.Errorf("%s: #ma-step-rfq senza le radici o il perché: %q", r, tx)
			}
			// revisione della notte 14/10: il prodotto ha già il 3D corrente 5e, quindi 3e non ha «Usa come STEP» e il
			// motivo lo dice (con il collegamento al Fascicolo)
			if n := maRTutti(step, func(n *html.Node) bool { return maRA(n, "data-ma-azione") == "usa-step" }); len(n) != 0 {
				t.Errorf("%s: «Usa come STEP» con il 3D corrente del prodotto", r)
			}
			if tx := maRTesto(step); !strings.Contains(tx, "7120100 ha già un 3D corrente: si aggiunge o si sostituisce dalla pagina del Fascicolo.") {
				t.Errorf("%s: #ma-step-rfq senza il motivo del 3D corrente: %q", r, tx)
			}
			aut := wpuPrimo(t, r, step, "«Autorizza come STEP» di 3f", func(n *html.Node) bool { return maRA(n, "data-ma-azione") == "step" })
			if maRA(aut, "data-ma-documento") != maU("5e").String() || maRA(aut, "data-ma-componente") != maU("10").String() ||
				maRHa(aut, "data-ma-sorgente") {
				t.Errorf("%s: «Autorizza come STEP» con documento %q, componente %q, sorgente %v", r, maRA(aut, "data-ma-documento"),
					maRA(aut, "data-ma-componente"), maRHa(aut, "data-ma-sorgente"))
			}
			if maRSpento(aut) == scrive {
				t.Errorf("%s: «Autorizza come STEP» acceso %v con Scrive %v", r, !maRSpento(aut), scrive)
			}
			fasc := "/thread/" + thread.String() + "/fascicolo?nodo=" + maU("10").String()
			if n := maRTutti(step, func(n *html.Node) bool { return n.Data == "a" && maRA(n, "href") == fasc }); len(n) != 2 {
				t.Errorf("%s: %d collegamenti al Fascicolo sul prodotto (%s), attesi 2", r, len(n), fasc)
			}
			// nessun campo del gesto nel template: correggi_codice lo mette solo motore_a.mjs, dopo la finestra
			if bytes.Contains(r.Frammento.Body.Bytes(), []byte("correggi_codice")) || bytes.Contains(r.Frammento.Body.Bytes(), []byte("correzione_confermata")) {
				t.Errorf("%s: il frammento porta correggi_codice o correzione_confermata", r)
			}

			// U4: solo l'immagine inline 3c è un elemento della mail; l'interruttore c'è una volta, non spuntato
			mail := maRTutti(doc, maRCon("", "data-ma-elemento-mail"))
			if len(mail) == 0 {
				t.Errorf("%s: nessun elemento della mail marcato (3c è natura_non_file)", r)
			}
			for _, n := range mail {
				if !strings.Contains(maRTesto(n), "image001.png") {
					t.Errorf("%s: marcato come elemento della mail: %q", r, maRTesto(n))
				}
			}
			tg := maRTutti(doc, func(n *html.Node) bool { return maRA(n, "id") == "ma-mostra-mail" })
			if len(tg) != 1 || maRA(tg[0], "type") != "checkbox" || maRHa(tg[0], "checked") || !strings.Contains(maRTesto(tg[0].Parent), "mostra elementi della mail (1)") {
				t.Errorf("%s: l'interruttore degli elementi della mail: %d", r, len(tg))
			}
		}
	}
}

// TestMotoreAWPUUsaSenza3DCorrente (revisione della notte 14/10): senza il 3D corrente del prodotto (il documento 5e
// tolto dalla fotografia) 3e ha «Usa come STEP» con la proposta, la correzione dichiarata e lo stato che la finestra
// mostra e manda ad assegna (data-ma-codice-file, data-ma-componente-attuale: vuoto, la proposta non è associata); 3f
// non è più un documento del prodotto e non ha «Autorizza». Acceso solo con Scrive. Con la proposta associata a un altro
// componente, data-ma-componente-attuale ne porta l'id.
func TestMotoreAWPUUsaSenza3DCorrente(t *testing.T) {
	s := maRServer(t)
	c := wpuCasoSenzaStruttura(t)
	var docs []fotorfq.DocumentoConfermato
	for _, d := range c.Foto.Documenti {
		if d.ID != maU("5e") {
			docs = append(docs, d)
		}
	}
	c.Foto.Documenti = docs
	p := c.Pagina.Esito.ProdottiValutati[0]
	rotte := maRRotte(t, c.Pagina.ThreadID)
	for _, scrive := range []bool{true, false} {
		r := maRRendi(s, "e10+wpu-5e", c, motorea_web.VistaDistinta, p.Rif, scrive)
		maRControlliGenerici(t, r, rotte)
		doc := maRDoc(t, r.Frammento.Body.Bytes())
		step := wpuPrimo(t, r, doc, "#ma-step-rfq", func(n *html.Node) bool { return maRA(n, "id") == "ma-step-rfq" })
		usa := wpuPrimo(t, r, step, "«Usa come STEP» di 3e", func(n *html.Node) bool { return maRA(n, "data-ma-azione") == "usa-step" })
		for k, v := range map[string]string{"data-ma-proposta": maU("7e").String(), "data-ma-componente": maU("10").String(),
			"data-ma-correggi": "1", "data-ma-codice": "7120100", "data-ma-codice-file": "ACM-9001", "data-ma-file": "ACM-9001.stp",
			"data-ma-associato": "", "data-ma-componente-attuale": ""} {
			if maRA(usa, k) != v || !maRHa(usa, k) {
				t.Errorf("%s: «Usa come STEP» con %s=%q, atteso %q", r, k, maRA(usa, k), v)
			}
		}
		if maRSpento(usa) == scrive {
			t.Errorf("%s: «Usa come STEP» acceso %v con Scrive %v", r, !maRSpento(usa), scrive)
		}
		if n := maRTutti(step, func(n *html.Node) bool { return maRA(n, "data-ma-azione") == "step" }); len(n) != 0 {
			t.Errorf("%s: «Autorizza» senza il documento del prodotto", r)
		}
	}

	// la proposta associata a un altro componente (20, dalla scena e10 se c'è, altrimenti un id ACME qualsiasi)
	altro := maU("20")
	for i := range c.Foto.Proposte {
		if c.Foto.Proposte[i].ID == maU("7e") {
			c.Foto.Proposte[i].ComponenteID = &altro
		}
	}
	r := maRRendi(s, "e10+wpu-5e+associata", c, motorea_web.VistaDistinta, p.Rif, true)
	usa := wpuPrimo(t, r, maRDoc(t, r.Frammento.Body.Bytes()), "«Usa come STEP» di 3e", func(n *html.Node) bool { return maRA(n, "data-ma-azione") == "usa-step" })
	if maRA(usa, "data-ma-componente-attuale") != altro.String() || maRA(usa, "data-ma-associato") == "" {
		t.Errorf("%s: la proposta associata a un altro componente: attuale %q, associato %q", r, maRA(usa, "data-ma-componente-attuale"), maRA(usa, "data-ma-associato"))
	}
}

// TestAssegnaConLaPropostaAttesa (revisione della notte 14/10): codice_atteso e componente_atteso del form di assegna
// (leggiAttesa) e il confronto con la proposta bloccata (attesaProposta.controlla). Assenti, nessun controllo (il
// Fascicolo legacy); presenti, il codice (senza spazi ai bordi) e il componente ("" = nessuno) devono essere ancora
// quelli, altrimenti il rifiuto «la proposta è cambiata nel frattempo: rileggi la pagina». Senza database.
func TestAssegnaConLaPropostaAttesa(t *testing.T) {
	c10, c20 := maU("10"), maU("20")
	proposta := func(codice string, comp *uuid.UUID) db.DocumentoProposta {
		p := db.DocumentoProposta{Stato: db.StatoPropostaAperta}
		if codice != "" {
			p.Codice = pgtype.Text{String: codice, Valid: true}
		}
		if comp != nil {
			p.ComponenteID = uuid.NullUUID{UUID: *comp, Valid: true}
		}
		return p
	}
	for _, x := range []struct {
		nome     string
		form     url.Values
		p        db.DocumentoProposta
		cambiata bool
	}{
		{"assenti: come prima", url.Values{"correggi_codice": {"1"}}, proposta("ACM-7777", &c20), false},
		{"lo stesso codice, nessun componente", url.Values{"codice_atteso": {"ACM-9001"}, "componente_atteso": {""}}, proposta(" ACM-9001 ", nil), false},
		{"senza codice, come atteso", url.Values{"codice_atteso": {""}, "componente_atteso": {""}}, proposta("", nil), false},
		{"lo stesso componente", url.Values{"codice_atteso": {"ACM-9001"}, "componente_atteso": {c20.String()}}, proposta("ACM-9001", &c20), false},
		{"un collega l'ha associata", url.Values{"codice_atteso": {"ACM-9001"}, "componente_atteso": {""}}, proposta("ACM-2000", &c20), true},
		{"solo il componente è cambiato", url.Values{"codice_atteso": {"ACM-9001"}, "componente_atteso": {c20.String()}}, proposta("ACM-9001", &c10), true},
		{"il componente tolto", url.Values{"componente_atteso": {c20.String()}}, proposta("ACM-9001", nil), true},
		{"il codice scritto nel frattempo", url.Values{"codice_atteso": {""}}, proposta("ACM-9001", nil), true},
		{"il codice con altre maiuscole è un altro codice", url.Values{"codice_atteso": {"acm-9001"}}, proposta("ACM-9001", nil), true},
	} {
		t.Run(x.nome, func(t *testing.T) {
			a, err := leggiAttesa(x.form)
			if err != nil {
				t.Fatal(err)
			}
			_, codice := x.form["codice_atteso"]
			_, comp := x.form["componente_atteso"]
			if a.presente() != (codice || comp) {
				t.Errorf("presente = %v", a.presente())
			}
			err = a.controlla(x.p, "ACM-9001.stp")
			if (err != nil) != x.cambiata {
				t.Fatalf("controlla = %v, atteso cambiata %v", err, x.cambiata)
			}
			if err != nil && err.Error() != "ACM-9001.stp: la proposta è cambiata nel frattempo: rileggi la pagina" {
				t.Errorf("il rifiuto: %q", err.Error())
			}
		})
	}
	for _, form := range []url.Values{{"componente_atteso": {"non-un-uuid"}}, {"codice_atteso": {"A", "B"}}, {"componente_atteso": {"", ""}}} {
		if _, err := leggiAttesa(form); err == nil {
			t.Errorf("leggiAttesa(%v) senza errore", form)
		}
	}
}

// TestMotoreAWPUSenzaStepNeMail: un prodotto con la BOM di lavoro (e3) non ha l'elenco degli STEP della RFQ; senza
// elementi della mail niente interruttore; senza diagnostiche dell'ancoraggio niente «perché».
func TestMotoreAWPUSenzaStepNeMail(t *testing.T) {
	s := maRServer(t)
	for _, nome := range maNomiCasi {
		c := leggiCasoMotoreA(t, nome)
		for _, prodotto := range maRProdotti(c) {
			r := maRRendi(s, nome, c, motorea_web.VistaDistinta, prodotto, true)
			if r.Frammento.Code != http.StatusOK {
				continue
			}
			doc := maRDoc(t, r.Frammento.Body.Bytes())
			for _, id := range []string{"ma-perche-prodotto", "ma-perche-rfq"} {
				if len(maRID(doc, id)) != 0 {
					t.Errorf("%s: #%s senza diagnostiche dell'ancoraggio nel caso", r, id)
				}
			}
			if r.P.Esito == nil || r.P.Indice == nil {
				continue
			}
			pv := r.P.Indice.Prodotto(prodotto)
			if pv != nil && pv.Struttura != ancoraggio.StatoStrutturaNessuna && len(maRID(doc, "ma-step-rfq")) != 0 {
				t.Errorf("%s: #ma-step-rfq con struttura %q", r, pv.Struttura)
			}
			if r.P.Indice.ElementiDellaMail() == 0 && len(maRID(doc, "ma-mostra-mail")) != 0 {
				t.Errorf("%s: l'interruttore senza elementi della mail", r)
			}
		}
	}
	// e3 ha la BOM di lavoro: niente STEP della RFQ
	r := maRCaso(t, s, "e3_bom_di_lavoro", motorea_web.VistaDistinta, "", true)
	if len(maRID(maRDoc(t, r.Frammento.Body.Bytes()), "ma-step-rfq")) != 0 {
		t.Errorf("%s: gli STEP della RFQ con la BOM di lavoro", r)
	}
}

// TestMotoreAWPUAutorizzaDocumentoDelProdotto: un documento del prodotto fra gli altri candidati alla fonte (origine
// documento_del_prodotto, con il documento) ha «Autorizza come STEP di …» con il documento e senza sorgente; una
// proposta aperta fra i candidati no.
func TestMotoreAWPUAutorizzaDocumentoDelProdotto(t *testing.T) {
	s := maRServer(t)
	c := wpuCasoSenzaStruttura(t)
	p := &c.Pagina.Esito.ProdottiValutati[0]
	p.Fonte.Candidati = []ancoraggio.DocumentoCandidato{
		{Origine: ancoraggio.CandidatoDaDocumentoDelProdotto, AllegatoID: maUp("3f"), DocumentoID: maUp("5e"), Sha256: maShaA("3f"),
			Estrazione: ancoraggio.EstrazioneRiuscita, GrafoCompleto: true},
		{Origine: ancoraggio.CandidatoDaPropostaAperta, AllegatoID: maUp("3e"), Sha256: maShaA("3e"), Estrazione: ancoraggio.EstrazioneRiuscita,
			GrafoCompleto: true},
	}
	rotte := maRRotte(t, c.Pagina.ThreadID)
	for _, scrive := range []bool{true, false} {
		r := maRRendi(s, "e10+wpu+candidati", c, motorea_web.VistaDistinta, p.Rif, scrive)
		maRControlliGenerici(t, r, rotte)
		doc := maRDoc(t, r.Frammento.Body.Bytes())
		det := wpuPrimo(t, r, doc, "gli altri documenti candidati", func(n *html.Node) bool {
			return n.Data == "details" && maRA(n, "data-ma-ricorda") == "candidati"
		})
		bb := maRTutti(det, func(n *html.Node) bool { return maRA(n, "data-ma-azione") == "step" })
		if len(bb) != 1 || maRA(bb[0], "data-ma-documento") != maU("5e").String() || maRHa(bb[0], "data-ma-sorgente") ||
			maRA(bb[0], "data-ma-componente") != maU("10").String() || maRSpento(bb[0]) == scrive {
			t.Errorf("%s: %d «Autorizza» fra gli altri candidati (atteso 1, sul documento 5e, acceso solo con Scrive)", r, len(bb))
		}
	}
}

// TestMotoreAWPUStatici: il CSS nasconde gli elementi della mail finché l'interruttore non è spuntato; motore_a.mjs ha
// il ramo usa-step, e apriUsaStep manda prima «assegna» e poi «Conferma il documento», con correggi_codice solo con la
// correzione dichiarata dal template (data-ma-correggi) e la presa d'atto spuntata. Revisione della notte 14/10: con
// la correzione anche codice_atteso e componente_atteso; mentre la catena è in corso la finestra non si chiude
// (annulla e ✕ spenti, cancel fermato), e la conferma parte solo con la finestra ancora aperta.
func TestMotoreAWPUStatici(t *testing.T) {
	css := maSFile(t, maSCSS)
	if !strings.Contains(css, ".ma:not(:has(#ma-mostra-mail:checked)) [data-ma-elemento-mail] { display: none; }") {
		t.Error("motore_a.css non nasconde gli elementi della mail")
	}
	js := maSFile(t, maSJS)
	if !strings.Contains(js, `case "usa-step": apriUsaStep(x); break;`) {
		t.Error("motore_a.mjs: manca il ramo usa-step di azione")
	}
	f := maSCorpoFunzione(t, js, "apriUsaStep")
	a := strings.Index(f, "Gesti.esegui(`/thread/${t}/fascicolo/assegna`, campi)")
	k := strings.Index(f, `if (!esito || esito.esito !== "ok") {`)
	cf := strings.Index(f, "Gesti.esegui(`/thread/${t}/fascicolo/conferma`, { voce: ds.maProposta })")
	if a < 0 || cf < 0 || k < a || cf < k {
		t.Error("apriUsaStep: prima «assegna», poi il controllo dell'esito, poi «Conferma il documento»")
	}
	if !strings.Contains(f, `const correggi = ds.maCorreggi === "1";`) || !regexp.MustCompile(`if \(correggi\) \{\s*campi\.correggi_codice = "1";\s*campi\.correzione_confermata = `).MatchString(f) ||
		strings.Count(f, "correggi_codice = ") != 1 {
		t.Error("apriUsaStep: correggi_codice solo dentro if (correggi), con correzione_confermata")
	}
	if !strings.Contains(f, "if (invia.disabled || !presa.checked) return;") || !strings.Contains(f, `presa.addEventListener("change", () => { invia.disabled = !presa.checked; });`) {
		t.Error("apriUsaStep: l'invio senza la presa d'atto")
	}
	if strings.Contains(f, "apriStep(") {
		t.Error("apriUsaStep apre da sé l'autorizzazione: il terzo gesto resta di chi guarda")
	}
	if !regexp.MustCompile(`campi\.correzione_confermata = [^\n]*\n(\s*//[^\n]*\n)?\s*if \("maCodiceFile" in ds\) campi\.codice_atteso = ds\.maCodiceFile;\s*if \("maComponenteAttuale" in ds\) campi\.componente_atteso = ds\.maComponenteAttuale;`).MatchString(f) {
		t.Error("apriUsaStep: codice_atteso e componente_atteso insieme alla correzione")
	}
	if !strings.Contains(f, "const blocca = (si) => { inCorso = si; annulla.disabled = si; w.x.disabled = si; };") ||
		!strings.Contains(f, `w.d.addEventListener("cancel", (e) => { if (inCorso) e.preventDefault(); });`) {
		t.Error("apriUsaStep: la finestra si chiude a catena in corso")
	}
	ap := strings.Index(f, "if (!aperta()) {")
	if ap < 0 || ap < k || cf < ap {
		t.Error("apriUsaStep: la conferma parte senza controllare che la finestra sia ancora aperta")
	}
	if !strings.Contains(js, "return { d, corpo, piede, x, chiudi:") {
		t.Error("dialogo() non dà la ✕ a chi la deve spegnere")
	}
}

// ---------------------------------------------------------------- U1: il collegamento dalle pagine Richieste e RFQ

func TestDistintaNuovaVisibileConLaGuardiaDellaRotta(t *testing.T) {
	admin, operatore := &db.Utente{Ruolo: db.RuoloUtenteAdmin}, &db.Utente{Ruolo: db.RuoloUtenteOperatore}
	for _, c := range []struct {
		accesa bool
		u      *db.Utente
		ok     bool
	}{{true, admin, true}, {true, operatore, false}, {true, nil, false}, {false, admin, false}, {false, operatore, false}} {
		s := &Server{MotoreA: motorea_web.Regole{Accesa: c.accesa}}
		if got := s.distintaNuovaVisibile(c.u); got != c.ok {
			t.Errorf("flag %v, utente %v: %v, atteso %v", c.accesa, c.u, got, c.ok)
		}
	}
}

func TestIlCollegamentoAllaDistintaNuova(t *testing.T) {
	s := serverTest(t)
	p := sintetica(leggiFiltriRichieste(nil))
	rendi := func() string {
		var buf bytes.Buffer
		if err := s.pagine["richieste.html"].ExecuteTemplate(&buf, "richieste_elenco", vista{Dati: p, Frammento: true}); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	if h := rendi(); strings.Contains(h, "/motore-a") || strings.Contains(h, "/distinta-unica") {
		t.Error("Richieste: il collegamento alla Distinta nuova senza la guardia")
	}
	p.Card[0].DistintaNuova = true
	h := rendi()
	if want := `href="/thread/` + p.Card[0].ThreadID.String() + `/motore-a"`; !strings.Contains(h, want) || strings.Count(h, "/motore-a") != 1 ||
		!strings.Contains(h, "Distinta nuova (Motore A)") {
		t.Errorf("Richieste: il collegamento della prima card (%s) non c'è, o è anche sulle altre", want)
	}
	if want := `href="/thread/` + p.Card[0].ThreadID.String() + `/distinta-unica"`; !strings.Contains(h, want) || strings.Count(h, "/distinta-unica") != 1 ||
		!strings.Contains(h, "Distinta unica (V1)") {
		t.Errorf("Richieste: il collegamento alla Distinta unica della prima card (%s) non c'è, o è anche sulle altre", want)
	}

	_, _, thd := datiSintetici()
	rendiT := func() string {
		var buf bytes.Buffer
		if err := s.pagine["thread.html"].ExecuteTemplate(&buf, "thread_corpo", vista{Dati: thd, Frammento: true}); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	if h := rendiT(); strings.Contains(h, "/motore-a") || strings.Contains(h, "/distinta-unica") {
		t.Error("RFQ: il collegamento alla Distinta nuova senza la guardia")
	}
	thd.DistintaNuova = true
	if h := rendiT(); strings.Count(h, `href="/thread/`+thd.T.ThreadID.String()+`/motore-a"`) != 2 || !strings.Contains(h, "Distinta nuova (Motore A)") {
		t.Error("RFQ: il collegamento alla Distinta nuova non c'è nella testata e nel riepilogo del Fascicolo")
	}
	if h := rendiT(); strings.Count(h, `href="/thread/`+thd.T.ThreadID.String()+`/distinta-unica"`) != 2 || !strings.Contains(h, "Distinta unica (V1)") {
		t.Error("RFQ: il collegamento alla Distinta unica non c'è nella testata e nel riepilogo del Fascicolo")
	}
}
