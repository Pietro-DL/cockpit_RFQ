//go:build integrazione

// L4 — B8.6 nelle rotte: aprire la RFQ mostra i codici che ha visto, ciascuno con il suo gesto; «+ come…»
// e il ripristino rispondono con la pagina, e un rifiuto dice perche' senza cambiare niente. Con la BOM
// congelata il pannello resta e i gesti che la cambiano no.

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

const famiglieDiProva = `{"famiglie_codice": [{"regex": "(?P<codice>529\\d{5})(?:_(?P<rev>[A-Z]))?", "descrizione": "disegni 529", "rev_nel_codice": true, "esempio": "52922757_B"}]}`

// rfqConCodici e' una RFQ di un cliente con la famiglia 529, con i codici visti nella sua mail, uno STEP
// con i fatti correnti (52922757 → 52920517 ×4), un componente, uno archiviato e un codice della
// richiesta. 52960000 ha due revisioni: A nell'oggetto, B nel cartiglio.
func (b *bancoWeb) rfqConCodici(chiave string) (*rfqFascicolo, map[string]uuid.UUID) {
	b.t.Helper()
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	if _, err := b.pool.Exec(b.ctx, `UPDATE cliente SET regole = $1 WHERE cliente_id = $2`, famiglieDiProva, cliente); err != nil {
		b.t.Fatal(err)
	}
	r := b.rfqFascicolo(cliente, chiave)
	for _, c := range []struct{ codice, rev, ruolo, origine, dove string }{
		{"52922757", "", "prodotto", "famiglia", "oggetto"},
		{"52931111", "", "prodotto", "famiglia", "corpo"},
		{"52940000", "", "prodotto", "famiglia", "corpo"},
		{"52950000", "", "prodotto", "famiglia", "oggetto"},
		{"52960000", "A", "prodotto", "famiglia", "oggetto"},
		{"20260908", "", "non_classificato", "generico", "corpo"},
	} {
		punti, famiglia := 30, ""
		if c.origine == "famiglia" {
			punti, famiglia = 80, "disegni 529"
		}
		r.esegui(`INSERT INTO candidato_codice (messaggio_id, codice, ruolo, rev, origine, famiglia, punteggio, evidenza)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, r.msg, c.codice, c.ruolo, c.rev, c.origine, famiglia, punti, c.dove)
	}
	a, _ := r.allegato("52960000_B.pdf")
	r.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte) VALUES ($1, $2, 'disegno_2d', '52960000', 'B', 95, 'cartiglio')`,
		a, r.thread)
	id := map[string]uuid.UUID{"componente": r.componente("52940000"), "archiviato": r.componente("52931111")}
	r.esegui(`UPDATE componente SET archiviato_il = now(), archiviato_da = $2, motivo_archiviazione = 'tolto dal cliente' WHERE componente_id = $1`,
		id["archiviato"], r.utente)
	r.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine) VALUES ($1, '52950000', 'manuale')`, r.thread)
	return r, id
}

func (r *rfqFascicolo) aggiungiCodice(w *browser, form url.Values) string {
	r.b.t.Helper()
	_, html := w.fai(http.MethodPost, "/thread/"+r.thread.String()+"/fascicolo/codice/aggiungi", form, true)
	return avvisoDi(html)
}

// Aprire la RFQ rilegge lo STEP e mostra i codici: quello del nodo aperto porta alla proposta, il
// componente si apre, l'archiviato si ripristina, il codice della richiesta entra come prodotto, quello
// nuovo offre i tre tipi e, con due revisioni, chiede quale. Poi i gesti dalla pagina.
func TestIlPannelloDeiCodiciNellaRfqEISuoiGesti(t *testing.T) {
	b := preparaBancoWeb(t)
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	r, id := b.rfqConCodici("CODICI86")
	r.stepNellaRfq("assieme.stp", fattiAssieme, an)
	w := operatore(b)

	_, html := w.fai(http.MethodGet, "/thread/"+r.thread.String(), nil, false)
	casi := map[string][]string{
		"52922757": {"proposta aperta dallo STEP <b>assieme.stp</b>", "Accetta la proposta"},
		"52920517": {"proposta aperta dallo STEP <b>assieme.stp</b>", "Accetta la proposta"},
		"52940000": {"✓ nel Fascicolo"},
		"52931111": {"tolto dal cliente", "/fascicolo/componente/" + id["archiviato"].String() + "/ripristina"},
		"52950000": {"codice della richiesta", "+ Prodotto"},
		"52960000": {"revisioni discordanti", `name="rev"`, "+ Assieme"},
		"20260908": {"+ Particolare"},
	}
	for chiave, attesi := range casi {
		riga := rigaDelCodice(t, html, chiave)
		for _, s := range attesi {
			if !strings.Contains(riga, s) {
				t.Errorf("%s: manca %q", chiave, s)
			}
		}
	}
	if strings.Contains(rigaDelCodice(t, html, "52950000"), "+ Assieme") {
		t.Error("un codice della richiesta entra come prodotto: niente «+ Assieme»")
	}
	if n := r.conta(`SELECT count(*) FROM componente WHERE thread_id = $1`, r.thread); n != 2 {
		t.Fatalf("aprire la RFQ ha creato componenti: %d", n)
	}

	for _, c := range []struct {
		form  url.Values
		frase string
	}{
		{url.Values{"codice": {"52960000"}, "tipo": {"sottoassieme"}}, "revisioni discordanti"},
		{url.Values{"codice": {"52920517"}, "tipo": {"sciolto"}}, "ha una proposta aperta dallo STEP assieme.stp"},
		{url.Values{"codice": {"52940000"}, "tipo": {"sciolto"}}, "è già nella BOM"},
		{url.Values{"codice": {"52931111"}, "tipo": {"sciolto"}}, "è archiviato: si ripristina"},
		{url.Values{"codice": {"52950000"}, "tipo": {"sciolto"}}, "è un codice della richiesta: entra come prodotto"},
		{url.Values{"codice": {"20260908"}, "tipo": {"boh"}}, "tipo di componente non valido"},
	} {
		if a := r.aggiungiCodice(w, c.form); !strings.HasPrefix(a, "Niente è cambiato: ") || !strings.Contains(a, c.frase) {
			t.Errorf("%v: %q, atteso il rifiuto con «%s»", c.form, a, c.frase)
		}
	}
	if n := r.conta(`SELECT count(*) FROM componente WHERE thread_id = $1`, r.thread); n != 2 {
		t.Fatalf("un rifiuto ha creato componenti: %d", n)
	}

	if a := r.aggiungiCodice(w, url.Values{"codice": {"52960000"}, "tipo": {"sottoassieme"}, "rev": {"B"}}); a != "52960000 entra nella BOM come assieme, rev B." {
		t.Errorf("aggiungi: %q", a)
	}
	if a := r.aggiungiCodice(w, url.Values{"codice": {"52950000"}, "tipo": {"finito"}}); a != "52950000 entra nella BOM come prodotto." {
		t.Errorf("codice della richiesta: %q", a)
	}
	_, html = w.fai(http.MethodPost, fmt.Sprintf("/thread/%s/fascicolo/componente/%s/ripristina", r.thread, id["archiviato"]), url.Values{}, true)
	if a := avvisoDi(html); a != "52931111 ripristinato nella BOM working." {
		t.Errorf("ripristina: %q", a)
	}
	if n := r.conta(`SELECT count(*) FROM componente WHERE thread_id = $1 AND archiviato_il IS NULL`, r.thread); n != 4 {
		t.Errorf("componenti attivi dopo i gesti: %d, attesi 4", n)
	}
	for _, chiave := range []string{"52960000", "52950000", "52931111"} {
		if !strings.Contains(rigaDelCodice(t, html, chiave), "✓ nel Fascicolo") {
			t.Errorf("%s: dopo il gesto la pagina deve mostrarlo nel Fascicolo", chiave)
		}
	}
}

// Con la BOM congelata il pannello c'e', dice perche' non offre gesti, e le rotte rifiutano.
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
	if a := r.aggiungiCodice(w, url.Values{"codice": {"20260908"}, "tipo": {"sciolto"}}); !strings.Contains(a, "la BOM è congelata nella V1") {
		t.Errorf("aggiungi: %q", a)
	}
	_, html = w.fai(http.MethodPost, fmt.Sprintf("/thread/%s/fascicolo/componente/%s/ripristina", r.thread, id["archiviato"]), url.Values{}, true)
	if a := avvisoDi(html); !strings.Contains(a, "la BOM è congelata nella V1") {
		t.Errorf("ripristina: %q", a)
	}
	if n := r.conta(`SELECT count(*) FROM componente WHERE thread_id = $1 AND archiviato_il IS NULL`, r.thread); n != 1 {
		t.Errorf("componenti attivi: %d, atteso 1", n)
	}
}
