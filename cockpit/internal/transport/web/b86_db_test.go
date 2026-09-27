//go:build integrazione

// L4 — B8.6 nelle rotte: aprire la RFQ mostra i codici che ha visto, ciascuno con la sua situazione. Dallo
// Smistamento (F2, R1) il pannello si legge e basta: la rotta …/fascicolo/codice/aggiungi non c'e' piu', e il
// ripristino resta con la sua rotta per gli Archiviati della Struttura BOM. Con la BOM congelata il pannello
// resta, e il ripristino si rifiuta.

package web

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
)

const famiglieDiProva = `{"famiglie_codice": [{"regex": "(?P<codice>777\\d{5})(?:_(?P<rev>[A-Z]))?", "descrizione": "disegni 777", "rev_nel_codice": true, "esempio": "77722757_B"}]}`

// rfqConCodici e' una RFQ di un cliente con la famiglia 777, con i codici visti nella sua mail, uno STEP
// con i fatti correnti (77722757 → 77720517 ×4), un componente, uno archiviato e un codice della
// richiesta. 77760000 ha due revisioni: A nell'oggetto, B nel cartiglio.
func (b *bancoWeb) rfqConCodici(chiave string) (*rfqFascicolo, map[string]uuid.UUID) {
	b.t.Helper()
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	if _, err := b.pool.Exec(b.ctx, `UPDATE cliente SET regole = $1 WHERE cliente_id = $2`, famiglieDiProva, cliente); err != nil {
		b.t.Fatal(err)
	}
	r := b.rfqFascicolo(cliente, chiave)
	for _, c := range []struct{ codice, rev, ruolo, origine, dove string }{
		{"77722757", "", "prodotto", "famiglia", "oggetto"},
		{"77731111", "", "prodotto", "famiglia", "corpo"},
		{"77740000", "", "prodotto", "famiglia", "corpo"},
		{"77750000", "", "prodotto", "famiglia", "oggetto"},
		{"77760000", "A", "prodotto", "famiglia", "oggetto"},
		{"20260908", "", "non_classificato", "generico", "corpo"},
	} {
		punti, famiglia := 30, ""
		if c.origine == "famiglia" {
			punti, famiglia = 80, "disegni 777"
		}
		r.esegui(`INSERT INTO candidato_codice (messaggio_id, codice, ruolo, rev, origine, famiglia, punteggio, evidenza)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, r.msg, c.codice, c.ruolo, c.rev, c.origine, famiglia, punti, c.dove)
	}
	a, _ := r.allegato("77760000_B.pdf")
	r.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte) VALUES ($1, $2, 'disegno_2d', '77760000', 'B', 95, 'cartiglio')`,
		a, r.thread)
	id := map[string]uuid.UUID{"componente": r.componente("77740000"), "archiviato": r.componente("77731111")}
	r.esegui(`UPDATE componente SET archiviato_il = now(), archiviato_da = $2, motivo_archiviazione = 'tolto dal cliente' WHERE componente_id = $1`,
		id["archiviato"], r.utente)
	r.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine) VALUES ($1, '77750000', 'manuale')`, r.thread)
	return r, id
}

// aggiungiCodice manda il POST della rotta tolta (…/fascicolo/codice/aggiungi): lo stato HTTP e l'avviso.
func (r *rfqFascicolo) aggiungiCodice(w *browser, form url.Values) (int, string) {
	r.b.t.Helper()
	resp, html := w.fai(http.MethodPost, "/thread/"+r.thread.String()+"/fascicolo/codice/aggiungi", form, true)
	return resp.StatusCode, avvisoDi(html)
}

// Aprire la RFQ mostra i codici, ciascuno con la sua situazione: quello del nodo aperto dice la proposta, il
// componente si apre, l'archiviato si dice, il codice della richiesta e quello nuovo si leggono. Nessuno
// porta un gesto; la rotta che faceva nascere un componente da un codice trovato non c'e' piu'; il ripristino
// (la rotta degli Archiviati) risponde ancora con la pagina.
//
// Riscritta per lo Smistamento (R8, fase F1), nella parte dell'apertura: prima fissava che aprire la RFQ
// rileggesse lo STEP. Adesso la GET non scrive e lo STEP lo rilegge il gesto «Rianalizza».
//
// Riscritta per lo Smistamento (R1, fase F2), nei gesti: prima era TestIlPannelloDeiCodiciNellaRfqEISuoiGesti
// e fissava «Accetta la proposta», «+ Prodotto/Assieme/Particolare» con i rifiuti della rotta …/codice/aggiungi
// (revisioni discordanti, proposta aperta, gia' nella BOM, archiviato, codice della richiesta, tipo non valido)
// e le aggiunte riuscite (77760000 assieme rev B, 77750000 prodotto). Adesso fissa che nessuna riga ha un
// gesto, che la rotta non esiste per nessuna di quelle forme e non scrive niente, e che 77760000 e 77750000
// restano codici senza componente.
//
// Smistamento F3 (rilievo della verifica di F2): il codice della richiesta senza componente, con la BOM mai
// congelata, non promette piu' «aprendo una revisione della BOM congelata»: aprirla non lo farebbe nascere.
func TestIlPannelloDeiCodiciNellaRfqSiLegge(t *testing.T) {
	b := preparaBancoWeb(t)
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	r, id := b.rfqConCodici("CODICI86")
	r.stepNellaRfq("assieme.stp", fattiAssieme, an)
	w := operatore(b)

	_, html := w.fai(http.MethodGet, "/thread/"+r.thread.String(), nil, false)
	if n := r.conta(`SELECT count(*) FROM componente_proposta WHERE thread_id = $1`, r.thread); n != 0 {
		t.Fatalf("aprire la RFQ ha riletto lo STEP: %d proposte di nodo", n)
	}
	if strings.Contains(rigaDelCodice(t, html, "77722757"), "proposta aperta dallo STEP") {
		t.Error("prima di «Rianalizza» il pannello non ha proposte dello STEP")
	}
	_, html = w.fai(http.MethodPost, "/thread/"+r.thread.String()+"/fascicolo/rianalizza", url.Values{}, true)
	if a := avvisoDi(html); a != "1 STEP riletto con le regole del cliente." {
		t.Fatalf("rianalizza: %q", a)
	}
	_, html = w.fai(http.MethodGet, "/thread/"+r.thread.String(), nil, false)
	casi := map[string][]string{
		"77722757": {"proposta aperta dallo STEP <b>assieme.stp</b>", "si decide nel Fascicolo"},
		"77720517": {"proposta aperta dallo STEP <b>assieme.stp</b>", "si decide nel Fascicolo"},
		"77740000": {"✓ nel Fascicolo"},
		"77731111": {"tolto dal cliente", "Archiviati della Struttura BOM"},
		"77750000": {"codice della richiesta senza componente", fraseRichiestaSenzaRevisione},
		"77760000": {"revisioni discordanti", ">rev A<", ">rev B<", "nessun componente con questo codice"},
		"20260908": {"nessun componente con questo codice"},
	}
	for chiave, attesi := range casi {
		riga := rigaDelCodice(t, html, chiave)
		for _, s := range attesi {
			if !strings.Contains(riga, s) {
				t.Errorf("%s: manca %q", chiave, s)
			}
		}
		for _, s := range []string{"hx-post", "Accetta la proposta", "+ Prodotto", "+ Assieme", "+ Particolare", "/ripristina", `name="rev"`} {
			if strings.Contains(riga, s) {
				t.Errorf("%s: c'e' ancora %q", chiave, s)
			}
		}
	}
	if n := r.conta(`SELECT count(*) FROM componente WHERE thread_id = $1`, r.thread); n != 2 {
		t.Fatalf("aprire la RFQ ha creato componenti: %d", n)
	}

	// la rotta non c'e' piu': nessuna delle forme di prima scrive qualcosa
	for _, form := range []url.Values{
		{"codice": {"77760000"}, "tipo": {"sottoassieme"}, "rev": {"B"}},
		{"codice": {"77750000"}, "tipo": {"finito"}},
		{"codice": {"77720517"}, "tipo": {"sciolto"}},
		{"codice": {"20260908"}, "tipo": {"sciolto"}},
	} {
		if stato, a := r.aggiungiCodice(w, form); stato != http.StatusNotFound && stato != http.StatusMethodNotAllowed {
			t.Errorf("%v: la rotta risponde %d %q, attesa assente", form, stato, a)
		}
	}
	if n := r.conta(`SELECT count(*) FROM componente WHERE thread_id = $1`, r.thread); n != 2 {
		t.Fatalf("la rotta tolta ha creato componenti: %d", n)
	}

	_, html = w.fai(http.MethodPost, fmt.Sprintf("/thread/%s/fascicolo/componente/%s/ripristina", r.thread, id["archiviato"]), url.Values{}, true)
	if a := avvisoDi(html); a != "77731111 ripristinato nella BOM working." {
		t.Errorf("ripristina: %q", a)
	}
	if n := r.conta(`SELECT count(*) FROM componente WHERE thread_id = $1 AND archiviato_il IS NULL`, r.thread); n != 2 {
		t.Errorf("componenti attivi dopo il ripristino: %d, attesi 2", n)
	}
	if !strings.Contains(rigaDelCodice(t, html, "77731111"), "✓ nel Fascicolo") {
		t.Error("77731111: dopo il ripristino la pagina deve mostrarlo nel Fascicolo")
	}
	for _, chiave := range []string{"77760000", "77750000"} {
		if strings.Contains(rigaDelCodice(t, html, chiave), "✓ nel Fascicolo") {
			t.Errorf("%s: un codice trovato non e' entrato nel Fascicolo da solo", chiave)
		}
	}
}

// Con la BOM congelata il pannello c'e', dice che e' congelata, e le rotte non cambiano la BOM.
//
// Riscritta per lo Smistamento (R1, fase F2): prima la rotta …/codice/aggiungi rifiutava con il messaggio
// della BOM congelata; adesso non c'e' (404/405) e non scrive. Il ripristino rifiuta come prima.
func TestConLaBomCongelataIlPannelloDeiCodiciNonCambiaLaBom(t *testing.T) {
	b := preparaBancoWeb(t)
	r, id := b.rfqConCodici("CODICI86B")
	r.chiudiV1(r.congelaV1(), false)
	w := operatore(b)

	_, html := w.fai(http.MethodGet, "/thread/"+r.thread.String(), nil, false)
	if !strings.Contains(html, "La BOM è congelata nella V1") {
		t.Error("il pannello deve dire che la BOM e' congelata")
	}
	for _, vietato := range []string{"+ Prodotto", "+ Assieme", ">Ripristina<", "/codice/aggiungi"} {
		if strings.Contains(html, vietato) {
			t.Errorf("con la BOM congelata la pagina offre %q", vietato)
		}
	}
	if stato, a := r.aggiungiCodice(w, url.Values{"codice": {"20260908"}, "tipo": {"sciolto"}}); stato != http.StatusNotFound && stato != http.StatusMethodNotAllowed {
		t.Errorf("aggiungi: %d %q, attesa la rotta assente", stato, a)
	}
	_, html = w.fai(http.MethodPost, fmt.Sprintf("/thread/%s/fascicolo/componente/%s/ripristina", r.thread, id["archiviato"]), url.Values{}, true)
	if a := avvisoDi(html); !strings.Contains(a, "la BOM è congelata nella V1") {
		t.Errorf("ripristina: %q", a)
	}
	if n := r.conta(`SELECT count(*) FROM componente WHERE thread_id = $1 AND archiviato_il IS NULL`, r.thread); n != 1 {
		t.Errorf("componenti attivi: %d, atteso 1", n)
	}
}
