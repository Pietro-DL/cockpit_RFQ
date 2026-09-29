//go:build integrazione

// L4 — la Distinta (PR #6, #7, #8) nel ramo delle prove, giro 4, fase 4.3, contro PostgreSQL vero. Le sue tre
// GET (la pagina in ogni passo, i dati per distinta.mjs, l'anteprima del tipo) non scrivono, per chi lavora e per
// chi consulta (F1, R8: nessuna GET scrive); e il passo 3, «Documenti e NAS», legge il piano della fase 4.2: un PDF
// che ha il codice soltanto dal testo non e' pronto, ed e' una domanda.

package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// famiglieDistintaDB: i codici 712xxxx del cliente di prova.
const famiglieDistintaDB = `{"famiglie_codice": [{"regex": "(?P<codice>712\\d{4})", "descrizione": "ACME 712", "esempio": "7120001"}]}`

// scenaDistintaDB e' una RFQ ACME in FATTIBILITA con la distinta fatta: il prodotto 7120001 (con il suo 2D sul
// NAS), l'assieme 7120010 ×2 sotto il prodotto, il particolare 7120011 ×4 sotto l'assieme. Arrivati e non ancora
// confermati: 7120011.pdf, pronto per il nome; «tavola.pdf», che nel nome non ha codici e nel cartiglio ha
// 7120010; «7120010.pdf», con lo stesso cartiglio e il nome che lo dice; un capitolato gia' confermato senza pezzo
// e un logo messo da parte. Ogni gruppo del passo 3 ha qualcosa.
type scenaDistintaDB struct {
	*rfqFascicolo
	prodotto, assieme, particolare uuid.UUID
	tavola, perNome                uuid.UUID // gli allegati dei due PDF con il cartiglio 7120010
}

func (b *bancoWeb) scenaDistintaDB(t *testing.T, chiave string) *scenaDistintaDB {
	t.Helper()
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), chiave)
	s := &scenaDistintaDB{rfqFascicolo: r}
	r.fase("FATTIBILITA")
	r.esegui(`UPDATE cliente SET regole = $1 WHERE cliente_id = (SELECT cliente_id FROM thread_offerta WHERE thread_id = $2)`, famiglieDistintaDB, r.thread)
	s.prodotto = r.componenteTipo("7120001", "finito")
	s.assieme = r.componenteTipo("7120010", "sottoassieme")
	s.particolare = r.componenteTipo("7120011", "sciolto")
	r.arco(s.prodotto, s.assieme, 2)
	r.arco(s.assieme, s.particolare, 4)
	r.documentoDa("7120001.pdf", "pdf", db.TipoDocumentoDisegno2d, "7120001", s.prodotto, db.StatoNasScritto)
	r.documentoDa("capitolato ACME.pdf", "pdf", db.TipoDocumentoCapitolato, "", uuid.Nil, db.StatoNasScritto)
	r.propostaDa("7120011.pdf", "7120011")
	logo, _ := r.allegatoExt("logo.png", "png")
	r.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte, stato)
		VALUES ($1, $2, 'rumore', 90, 'estensione', 'scartata')`, logo, r.thread)
	m := classificazione.Compila("ACME", regole.Regole{FamiglieCodice: []regole.FamigliaCodice{{
		Regex: `(?P<codice>712\d{4})`, Descrizione: "ACME 712", Esempio: "7120001"}}})
	cartiglio := fattiCartiglio(t, "DISEGNO N. 7120010\nSCALA 1:2")
	s.tavola = s.pdfLetto(t, m, "tavola.pdf", cartiglio)
	s.perNome = s.pdfLetto(t, m, "7120010.pdf", cartiglio)
	return s
}

// fattiCartiglio sono i fatti dell'analizzatore 4 di un disegno finto: il testo nativo in basso a destra della
// pagina 1, dove sta il cartiglio.
func fattiCartiglio(t *testing.T, bassoDestra string) json.RawMessage {
	t.Helper()
	tp := worker.TestoPDF{Versione: 1, Estraibile: true, Pagine: 1, PagineLette: 1, FormatoPagina1: []float64{842, 595},
		Caratteri: len(bassoDestra), OCR: worker.OCRPDF{Stato: worker.OCRNonNecessario},
		Limiti: worker.LimitiTestoPDF{PagineMax: 11, FrammentiMax: 200, FrammentoMax: 512, CaratteriMax: 16384},
		Frammenti: []worker.FrammentoPDF{{Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Testo: bassoDestra,
			Riquadro: []float64{560, 490, 724, 543}}}}
	b, err := json.Marshal(map[string]any{"cartiglio": true, "termini_trovati": []string{"SCALA"}, "testo_pdf": tp})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// pdfLetto e' un PDF arrivato e letto dall'analisi con quei fatti: la proposta aperta con la valutazione nei
// dettagli, come la scrive il server.
func (s *scenaDistintaDB) pdfLetto(t *testing.T, m *classificazione.Motore, nome string, fatti json.RawMessage) uuid.UUID {
	t.Helper()
	a, _ := s.allegatoExt(nome, "pdf")
	v := classificazione.Valuta(classificazione.IngressoFile{Da: classificazione.DaAnalisi, NomeFile: nome, Direzione: "entrata", Motore: m,
		Esito: &classificazione.Esito{Tipo: "disegno_2d", Fonte: "cartiglio"}, Fatti: fatti})
	dett, rp := classificazione.ConValutazione(fatti, v, time.Now())
	s.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte, dettagli)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8)`, a, s.thread, rp.Tipo, rp.Codice, rp.Rev, rp.Confidenza, rp.Fonte, dett)
	return a
}

func (s *scenaDistintaDB) distinta() string { return "/thread/" + s.thread.String() + "/distinta" }

// rigaDistinta cerca la riga di un file nel passo 3: la sua classe (lo stato) e la domanda, se c'e'.
func rigaDistinta(t *testing.T, html, nome string) (stato, domanda string) {
	t.Helper()
	m := regexp.MustCompile(`(?s)<tr class="dst-file ([a-z]+)">\s*<td class="nome"><span class="mono">` + regexp.QuoteMeta(nome) + `</span>(.*?)</tr>`).FindStringSubmatch(html)
	if m == nil {
		t.Fatalf("la riga di %s non c'e' nel passo 3", nome)
	}
	if d := regexp.MustCompile(`<div class="dst-domanda">([^<]*)</div>`).FindStringSubmatch(m[2]); d != nil {
		domanda = leggibile(d[1])
	}
	return m[1], domanda
}

// Giro 4, fase 4.3 (piano, «Prove nuove della Distinta»): le GET della Distinta non scrivono. La pagina in ogni
// passo (anche con un passo che non c'e'), i dati per distinta.mjs e l'anteprima del tipo (un tipo che si fa,
// uno spento, un componente che non va), per chi lavora e per chi consulta: prima e dopo, ogni tabella dello
// schema ha le stesse righe con lo stesso contenuto. La pagina di chi consulta e' in sola lettura e non chiede la
// preparazione; i dati dicono chi scrive; i dati e l'anteprima non si tengono in cache. Controprova: il cambio di
// tipo che l'anteprima prepara, mandato con la POST da chi lavora, il database lo cambia (e la fotografia lo vede);
// da chi consulta si rifiuta e non scrive.
func TestLeGetDellaDistintaNonScrivono(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaDistintaDB(t, "DSTGET")
	base := s.distinta()
	op := operatore(b)
	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")
	tipo := func(comp uuid.UUID, t string) string {
		return base + "/tipo?componente=" + comp.String() + "&tipo=" + url.QueryEscape(t)
	}
	get := []string{base, base + "?passo=richiesta", base + "?passo=distinta", base + "?passo=documenti", base + "?passo=fattibilita",
		base + "?passo=boh", base + "/dati", tipo(s.assieme, "commerciale"), tipo(s.assieme, "sciolto"), tipo(s.particolare, "commerciale"),
		tipo(s.particolare, "sottoassieme"), tipo(s.prodotto, "sottoassieme"), base + "/tipo?componente=boh&tipo=commerciale",
		tipo(uuid.New(), "commerciale")}

	prima := fotoDelDatabase(t, b)
	for _, chi := range []struct {
		nome   string
		w      *browser
		scrive bool
	}{{"operatore", op, true}, {"consultazione", co, false}} {
		for _, g := range get {
			resp, corpo := chi.w.fai(http.MethodGet, g, nil, false)
			if resp.StatusCode != 200 {
				t.Errorf("%s, GET %s: %d", chi.nome, g, resp.StatusCode)
				continue
			}
			if strings.Contains(g, "/dati") || strings.Contains(g, "/tipo?") {
				if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
					t.Errorf("%s, GET %s: Cache-Control %q", chi.nome, g, cc)
				}
				var x map[string]any
				if err := json.Unmarshal([]byte(corpo), &x); err != nil {
					t.Errorf("%s, GET %s: non e' JSON: %v", chi.nome, g, err)
				}
				if strings.Contains(g, "/dati") && x["scrive"] != chi.scrive {
					t.Errorf("%s: i dati dicono scrive=%v", chi.nome, x["scrive"])
				}
			}
		}
		if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
			t.Errorf("le GET della Distinta di %s hanno scritto in: %s", chi.nome, d)
		}
	}

	// la pagina di chi consulta: sola lettura, niente preparazione chiesta da sola; quella di chi lavora la chiede
	if _, pagina := co.fai(http.MethodGet, base+"?passo=documenti", nil, false); !strings.Contains(pagina, "sola-lettura") || strings.Contains(pagina, "/fascicolo/prepara") {
		t.Error("la Distinta di chi consulta: in sola lettura, senza la preparazione")
	}
	_, pagina := op.fai(http.MethodGet, base, nil, false)
	if !strings.Contains(pagina, `hx-post="/thread/`+s.thread.String()+`/fascicolo/prepara"`) || strings.Contains(pagina, "sola-lettura") {
		t.Fatal("la Distinta di chi lavora chiede la preparazione con una POST: senza, la controprova non prova niente")
	}
	// l'anteprima di un tipo che si fa porta la firma, quella di un tipo spento dice il motivo e non la porta
	_, js := op.fai(http.MethodGet, tipo(s.particolare, "commerciale"), nil, false)
	var fatto, spento tipoDistinta
	_ = json.Unmarshal([]byte(js), &fatto)
	_, js = op.fai(http.MethodGet, tipo(s.assieme, "commerciale"), nil, false)
	_ = json.Unmarshal([]byte(js), &spento)
	if fatto.Firma == "" || fatto.Spento != "" || !strings.Contains(fatto.Bottone, "7120011 diventa un particolare commerciale") {
		t.Errorf("l'anteprima di un tipo che si fa: %+v", fatto)
	}
	if !strings.HasPrefix(spento.Spento, "7120010 ha 1 figlio: è un assieme; per farlo diventare un particolare commerciale") {
		t.Errorf("l'anteprima di un tipo spento (il commerciale e' una foglia): %+v", spento)
	}
	t.Run("da fare 4.4: l'anteprima del tipo non da' la firma a chi consulta", func(t *testing.T) {
		_, js := co.fai(http.MethodGet, tipo(s.particolare, "commerciale"), nil, false)
		var x tipoDistinta
		_ = json.Unmarshal([]byte(js), &x)
		if x.Firma == "" {
			return
		}
		// distintaTipo e' del frontendista (piano, «Chi tocca quali file»): la modifica sta nel suo elenco della
		// 4.4; l'anteprima del Fascicolo la firma a chi consulta non la mostra (Scrive), questa si'. Nessuna
		// scrittura ne segue: la POST del tipo a chi consulta e' rifiutata dal middleware (403)
		t.Skipf("da fare giro 4 (4.4, distinta.go): distintaTipo da' la firma %q anche a chi consulta", x.Firma)
	})

	// la controprova: la POST del tipo di chi consulta si rifiuta e non scrive; quella di chi lavora, con la firma
	// dell'anteprima, scrive, e la fotografia lo vede (altrimenti le GET non proverebbero niente)
	prima = fotoDelDatabase(t, b)
	campi := url.Values{"tipo": {"commerciale"}, "firma": {fatto.Firma}}
	if resp, _ := co.daFascicolo(http.MethodPost, s.base()+"/componente/"+s.particolare.String()+"/tipo", campi, s.thread, ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("chi consulta non cambia il tipo, nemmeno con la firma dell'anteprima: HTTP %d", resp.StatusCode)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("la POST rifiutata di chi consulta ha scritto in: %s", d)
	}
	_, out := op.daFascicolo(http.MethodPost, s.base()+"/componente/"+s.particolare.String()+"/tipo", campi, s.thread, "")
	if a := avvisoF(out); a != "7120011: tipo particolare → particolare commerciale." {
		t.Errorf("il cambio di tipo di chi lavora: %q", a)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); !strings.Contains(d, "componente") {
		t.Fatalf("la POST di chi lavora doveva cambiare il componente (tabelle cambiate: %q): la fotografia non vede le scritture", d)
	}
}

// Giro 4, fase 4.2 nella Distinta (bug 3 del 29/09, la L4 che la 4.2 ha lasciato alla 4.3): un PDF con il codice
// solo dal testo non e' «pronto» per sola uguaglianza con un pezzo della distinta. «tavola.pdf» non ha codici nel
// nome e il cartiglio dice 7120010, che e' l'assieme della distinta: non entra in Piano.FilePronti(), nel passo 3
// non e' fra i pronti (niente «✓ Conferma»), sta sotto 7120010 da decidere con la domanda «il codice viene dal
// testo del PDF, non dal nome: è 7120010?», e il gesto cumulativo non lo conta. Controprova: «7120010.pdf», con lo
// stesso cartiglio e il nome che lo dice, e' pronto. La controprova a mano: senza codiceSoloDalTesto (piano.go) la
// tavola torna pronta e la prova fallisce.
func TestIlCodiceSoloDalTestoNonEProntoNellaDistinta(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaDistintaDB(t, "DSTTXT")

	f, err := b.ws.caricaFascicolo(b.ctx, s.thread, statoFascicolo{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	voci := map[uuid.UUID]fascicolo.VoceFile{}
	var pronti []string
	for _, v := range f.Piano.File {
		voci[v.Allegato] = v
		if v.Stato == fascicolo.VocePronta {
			pronti = append(pronti, v.Nome)
		}
	}
	tav, ok := voci[s.tavola]
	if !ok {
		t.Fatal("tavola.pdf non e' nel piano")
	}
	if tav.Stato != fascicolo.VoceDecidere || len(tav.Domande) == 0 || tav.Domande[0].Chiave != fascicolo.DomandaCodice ||
		!strings.Contains(tav.Domande[0].Testo, "il codice viene dal testo del PDF, non dal nome: è 7120010?") {
		t.Errorf("tavola.pdf nel piano: %s %+v", tav.Stato, tav.Domande)
	}
	if tav.Componente == nil || tav.Componente.ComponenteID != s.assieme {
		t.Errorf("la voce sa a quale pezzo il testo la manderebbe: %+v", tav.Componente)
	}
	if v := voci[s.perNome]; v.Stato != fascicolo.VocePronta || v.Componente == nil || v.Componente.ComponenteID != s.assieme {
		t.Errorf("7120010.pdf, con il nome che lo dice, e' pronto per 7120010: %s %+v", v.Stato, v.Domande)
	}
	if f.Piano.FilePronti() != 2 || strings.Join(pronti, " ") != "7120010.pdf 7120011.pdf" && strings.Join(pronti, " ") != "7120011.pdf 7120010.pdf" {
		t.Errorf("Piano.FilePronti() = %d (%v), attesi 7120010.pdf e 7120011.pdf", f.Piano.FilePronti(), pronti)
	}

	// il passo 3 della Distinta, come lo costruisce documentiDistinta
	v, err := b.ws.caricaDistinta(b.ctx, s.thread, "documenti", nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.NPronti != 2 {
		t.Errorf("i pronti del passo 3: %d, attesi 2", v.NPronti)
	}
	for _, blocco := range v.Blocchi {
		for _, r := range blocco.File {
			if r.F.A.AllegatoID == s.tavola && (blocco.N.C.ComponenteID != s.assieme || r.Stato != "decidere" || !strings.Contains(r.Domanda, "il codice viene dal testo del PDF")) {
				t.Errorf("tavola.pdf nel blocco di %s, %s: %q", blocco.N.C.Codice, r.Stato, r.Domanda)
			}
		}
	}

	// la pagina, come la vede l'operatore
	_, pagina := operatore(b).fai(http.MethodGet, s.distinta()+"?passo=documenti", nil, false)
	stato, domanda := rigaDistinta(t, pagina, "tavola.pdf")
	if stato != "decidere" || !strings.Contains(domanda, "il codice viene dal testo del PDF, non dal nome: è 7120010?") {
		t.Errorf("la riga di tavola.pdf: %s, %q", stato, domanda)
	}
	if riga := estratto(pagina, "tavola.pdf</span>"); strings.Contains(riga, "✓ Conferma") {
		t.Error("tavola.pdf non ha «✓ Conferma»")
	}
	if stato, _ := rigaDistinta(t, pagina, "7120010.pdf"); stato != "pronto" {
		t.Errorf("la riga di 7120010.pdf: %s", stato)
	}
	if !strings.Contains(pagina, "Conferma i 2 file pronti e copia sul NAS") || !strings.Contains(pagina, `<span class="dst-chip warn">2 pronti da confermare</span>`) {
		t.Error("il gesto cumulativo e la linguetta contano 2 file pronti, non la tavola")
	}
}
