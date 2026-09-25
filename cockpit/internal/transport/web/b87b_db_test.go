//go:build integrazione

// L4 — B8.7b sulla porta HTTP vera: la RFQ che nasce fa dei codici della richiesta i prodotti e prepara da
// sola i file utili; l'aggancio e l'apertura del Fascicolo fanno lo stesso; «Conferma Fascicolo» porta nel
// fascicolo solo il piano che l'operatore ha visto, tutto o niente; le decisioni di «Da verificare» non si
// fanno riscrivere da una lettura che arriva dopo; «Importa dal NAS» resta sotto la radice; il poll rifa' i
// pannelli solo quando qualcosa e' cambiato, e si ferma quando il lavoro finisce.

package web

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
)

var reFirma = regexp.MustCompile(`name="firma" value="([0-9a-f]+)"`)

// firmaDellaPagina e' la firma del piano che la pagina mostra.
func firmaDellaPagina(t *testing.T, html string) string {
	t.Helper()
	m := reFirma.FindStringSubmatch(html)
	if m == nil {
		t.Fatal("la pagina non porta la firma del piano")
	}
	return m[1]
}

// mailConAllegati e' una mail di ACME con gli allegati dati, entrata per la strada vera (l'ingest).
func (b *bancoWeb) mailConAllegati(oggetto, corpo string, allegati ...worker.AllegatoIn) uuid.UUID {
	b.t.Helper()
	m := b.mail("entrata", "mario.rossi@acme.example", oggetto, corpo, "commerciale@azienda.example")
	m.Allegati = allegati
	return b.postaMsg(m)
}

// La RFQ nasce: il codice spuntato diventa il prodotto finito, lo zip scende da solo (senza spunta) e
// l'immagine no; il form lo dice prima.
func TestCreareLaRfqFaDelCodiceUnProdottoEPreparaLoZip(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	msg := b.mailConAllegati("Richiesta di offerta 52922757", "Offerta per il supporto 52922757, 200 pezzi.",
		worker.AllegatoIn{Indice: 1, NomeFile: "RFQ ACME 52922757.zip", Estensione: "zip", Natura: "file", Bytes: 4000},
		worker.AllegatoIn{Indice: 2, NomeFile: "logo.png", Estensione: "png", Natura: "file", Bytes: 2000})
	w := operatore(b)

	_, form := w.fai(http.MethodGet, "/messaggio/"+msg.String()+"/triage?azione=nuova", nil, true)
	if !strings.Contains(form, "si prepara da solo") || !strings.Contains(form, "scarica anche questo") {
		t.Fatalf("il form dice che lo zip si prepara da solo e offre la spunta per l'immagine:\n%s", estrai(form, "Allegati"))
	}
	if strings.Contains(form, "e scarica i file spuntati") {
		t.Error("il bottone non parla piu' di file spuntati")
	}
	if !strings.Contains(form, `value="52922757"`) {
		t.Fatalf("il codice 52922757 e' fra i candidati del form")
	}

	_, html := w.fai(http.MethodPost, "/messaggio/"+msg.String()+"/rfq", url.Values{"cliente_id": {acme.String()}, "oggetto": {"Supporto 52922757"},
		"codice": {"52922757"}, "priorita": {"1"}}, true)
	a := leggibile(html)
	for _, c := range []string{"RFQ creata", "52922757 nella BOM come prodotto finito", "1 file utile in preparazione"} {
		if !strings.Contains(a, c) {
			t.Errorf("avviso: manca %q in %q", c, estrai(html, "avviso"))
		}
	}
	var thread uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT thread_id FROM messaggio WHERE messaggio_id = $1`, msg).Scan(&thread); err != nil {
		t.Fatal(err)
	}
	var tipo, origine string
	if err := b.pool.QueryRow(b.ctx, `SELECT tipo::text, origine::text FROM componente WHERE thread_id = $1 AND codice = '52922757'`, thread).Scan(&tipo, &origine); err != nil {
		t.Fatalf("il prodotto non e' nato: %v", err)
	}
	if tipo != "finito" || origine != "codice_rilevato" {
		t.Errorf("prodotto: %s/%s", tipo, origine)
	}
	var zip, logo int
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FILTER (WHERE a.nome_file LIKE '%.zip'), count(*) FILTER (WHERE a.nome_file = 'logo.png')
		FROM job j JOIN allegato a ON a.allegato_id::text = j.payload ->> 'allegato_id' WHERE j.tipo = 'stage_allegato'`).Scan(&zip, &logo); err != nil {
		t.Fatal(err)
	}
	if zip != 1 || logo != 0 {
		t.Errorf("download: zip %d, logo %d", zip, logo)
	}
}

// Agganciare un messaggio a una RFQ fa lo stesso: il codice della richiesta confermato diventa il prodotto
// (una RFQ di prima, che non l'aveva), e lo STEP del messaggio scende.
func TestAgganciareUnMessaggioPreparaIFileEAssicuraIProdotti(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	var fp uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT utente_id FROM utente WHERE sigla = 'FP'`).Scan(&fp); err != nil {
		t.Fatal(err)
	}
	thread := b.rfqDa(uuid.Nil, acme, `ACME\WIP\2026 09 24 Rossi RFQ 52922757`, "52922757")
	if _, err := b.pool.Exec(b.ctx, `UPDATE identificativo_thread SET confermato_da = $2 WHERE thread_id = $1`, thread, fp); err != nil {
		t.Fatal(err)
	}
	msg := b.mailConAllegati("RE: RFQ 52922757", "Ecco lo STEP.", worker.AllegatoIn{Indice: 1, NomeFile: "52922757.stp", Estensione: "stp", Natura: "file", Bytes: 9000})
	w := operatore(b)
	_, html := w.fai(http.MethodPost, "/messaggio/"+msg.String()+"/aggancia", url.Values{"thread_id": {thread.String()}}, true)
	a := leggibile(html)
	if !strings.Contains(a, "Agganciato") || !strings.Contains(a, "52922757 nella BOM come prodotto finito") || !strings.Contains(a, "1 file utile in preparazione") {
		t.Fatalf("avviso: %q", estrai(html, "avviso"))
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM job WHERE tipo = 'stage_allegato'`); n != 1 {
		t.Errorf("download dello STEP: %d", n)
	}
}

// Aprire il Fascicolo lo prepara, per chi lavora: una RFQ di prima con il codice della richiesta e un
// allegato utile non ancora sceso. Chi consulta la guarda e non mette in moto niente.
func TestAprireIlFascicoloLoPreparaPerChiLavora(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	msg := b.mailConAllegati("RFQ 52922757", "Richiesta per 52922757.", worker.AllegatoIn{Indice: 1, NomeFile: "52922757.pdf", Estensione: "pdf", Natura: "file", Bytes: 9000})
	thread := b.rfqDa(msg, acme, `ACME\WIP\2026 09 24 Rossi RFQ 52922757`, "52922757")
	var fp uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT utente_id FROM utente WHERE sigla = 'FP'`).Scan(&fp); err != nil {
		t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `UPDATE identificativo_thread SET confermato_da = $2 WHERE thread_id = $1`, thread, fp); err != nil {
		t.Fatal(err)
	}
	base := "/thread/" + thread.String() + "/fascicolo"

	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")
	if resp, _ := co.fai(http.MethodGet, base, nil, false); resp.StatusCode != 200 {
		t.Fatalf("consultazione: %d", resp.StatusCode)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM componente`); n != 0 {
		t.Errorf("chi consulta non crea prodotti: %d", n)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM job WHERE tipo = 'stage_allegato'`); n != 0 {
		t.Errorf("chi consulta non scarica: %d", n)
	}

	_, html := operatore(b).fai(http.MethodGet, base, nil, false)
	if n := contaSQL(t, b, `SELECT count(*) FROM componente WHERE tipo = 'finito' AND codice = '52922757'`); n != 1 {
		t.Errorf("il codice della richiesta e' il prodotto: %d", n)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM job WHERE tipo = 'stage_allegato' AND stato = 'pronto'`); n != 1 {
		t.Errorf("il disegno scende da solo: %d", n)
	}
	for _, c := range []string{"prodotto finito", "Preparazione dei file", "1 download", `hx-trigger="every 2s"`} {
		if !strings.Contains(html, c) {
			t.Errorf("la pagina: manca %q", c)
		}
	}
}

// scenaConferma: il prodotto (dal codice della richiesta), il suo STEP che propone l'assieme 52920517 ×2, i
// disegni dei due, un PDF anonimo. Tutti i file nello staging, analizzati.
type scenaConferma struct {
	*rfqFascicolo
	prodotto                          uuid.UUID
	stp, pdfProdotto, pdfAssieme, boh uuid.UUID // proposte dei file
	allStep                           uuid.UUID
}

func (b *bancoWeb) scenaConferma(chiave string) *scenaConferma {
	b.t.Helper()
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), chiave)
	s := &scenaConferma{rfqFascicolo: r}
	r.fase("FATTIBILITA")
	s.prodotto = r.componenteTipo("52922757", "finito")
	var sha string
	s.allStep, sha = r.allegatoExt("52922757.stp", "stp")
	r.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1,$2,'cad_3d','52922757',95,'step')`, s.allStep, r.thread)
	s.stp = uuidSQL(b.t, b, `SELECT proposta_id FROM documento_proposta WHERE allegato_id = $1`, s.allStep)
	r.esegui(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, codice, origine_codice, tipo_proposto, fonte, confidenza, stato, componente_id)
		VALUES ($1,$2,$3,'#1','52922757','52922757','generico','finito','step',90,'duplicato',$4)`, r.thread, s.allStep, sha, s.prodotto)
	r.esegui(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, codice, origine_codice, tipo_proposto, fonte, confidenza)
		VALUES ($1,$2,$3,'#2','52920517','52920517','generico','sottoassieme','step',90)`, r.thread, s.allStep, sha)
	r.esegui(`INSERT INTO relazione_proposta (thread_id, allegato_id, padre_chiave, figlio_chiave, qta) VALUES ($1,$2,'#1','#2',2)`, r.thread, s.allStep)
	s.pdfProdotto, _ = r.propostaDa("52922757.pdf", "52922757")
	s.pdfAssieme, _ = r.propostaDa("52920517.pdf", "52920517")
	s.boh, _ = r.propostaDa("anonimo.pdf", "")
	r.esegui(`UPDATE documento_proposta SET tipo_proposto = 'da_determinare', confidenza = 30, fonte = 'estensione' WHERE proposta_id = $1`, s.boh)
	return s
}

// «Conferma Fascicolo» porta nel fascicolo il piano pronto in una transazione: i documenti e lo STEP
// strutturale. Fascicolo v3: la struttura dello STEP non entra con quel gesto, si conferma nell'editor della
// Struttura BOM (bom/applica); il disegno dell'assieme che nasce da lei la aspetta, e diventa pronto dopo.
// Con una firma vecchia, o con un file senza la struttura da cui dipende, non cambia niente. Il PDF anonimo
// resta da decidere.
func TestConfermaFascicoloPortaNelFascicoloSoloIlPianoVisto(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	s := b.scenaConferma("CNF7B")
	w := operatore(b)

	_, html := w.fai(http.MethodGet, s.base(), nil, false)
	for _, c := range []string{"<b>3</b> pronti", "<b>3</b> da verificare"} {
		if !strings.Contains(html, c) {
			t.Errorf("la pagina: manca %q", c)
		}
	}
	firma := firmaDellaPagina(t, html)
	_, bom := w.fai(http.MethodGet, s.base()+"?vista=bom", nil, false)
	for _, c := range []string{"Apri proposta BOM", "52920517", "+ proposto · assieme?", "1 nodo e 1 arco proposti: la struttura si rivede e si conferma nell&#39;editor"} {
		if !strings.Contains(bom, c) {
			t.Errorf("la Struttura BOM: manca %q", c)
		}
	}
	if strings.Contains(bom, "Applica struttura proposta") {
		t.Error("la struttura dello STEP non si applica piu' dal banner: si apre nell'editor")
	}

	if a := s.gestoC(w, url.Values{"firma": {"deadbeef00000000"}}); !strings.HasPrefix(a, "Niente è cambiato: il piano è cambiato") {
		t.Fatalf("firma vecchia: %q", a)
	}
	if a := s.gestoC(w, url.Values{"voce": {s.pdfAssieme.String()}}); !strings.Contains(a, "52920517 nasce dalla struttura dello STEP 52922757.stp: si conferma prima quella, nell'editor della Struttura BOM") {
		t.Fatalf("senza la struttura da cui dipende: %q", a)
	}
	if a := s.gestoC(w, url.Values{"struttura": {s.allStep.String()}}); !strings.HasPrefix(a, "Niente è cambiato") {
		t.Fatalf("la struttura mandata alla conferma: %q", a)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM documento`); n != 0 {
		t.Fatalf("i rifiuti hanno lasciato %d documenti", n)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM componente WHERE codice = '52920517'`); n != 0 {
		t.Fatalf("i rifiuti hanno fatto nascere l'assieme: %d", n)
	}

	a := s.gestoC(w, url.Values{"firma": {firma}})
	for _, c := range []string{"Fascicolo confermato", "2 documenti (copie sul NAS in coda)", "STEP strutturale di 52922757: 52922757.stp"} {
		if !strings.Contains(a, c) {
			t.Errorf("avviso: manca %q in %q", c, a)
		}
	}
	if strings.Contains(a, "struttura di 52922757.stp") {
		t.Errorf("«Conferma Fascicolo» ha confermato la struttura dello STEP: %q", a)
	}
	if got := s.valore(`SELECT string_agg(c.codice || ':' || c.tipo::text, ' ' ORDER BY c.codice) FROM componente c WHERE thread_id = $1`, s.thread); got != "52922757:finito" {
		t.Errorf("componenti dopo la conferma: %s", got)
	}
	if got := s.valore(`SELECT stato::text FROM documento_proposta WHERE proposta_id = $1`, s.pdfAssieme); got != "aperta" {
		t.Errorf("il disegno dell'assieme aspetta la struttura: %s", got)
	}

	// la struttura nell'editor, com'e' proposta: nasce l'assieme con l'arco ×2, la proposta dell'arco e' presa
	nodo := uuidSQL(t, b, `SELECT proposta_id FROM componente_proposta WHERE allegato_id = $1 AND chiave = '#2'`, s.allStep)
	struttura := fmt.Sprintf(`{"radice":%q,"archi":[{"padre":"c:%s","figlio":"p:%s","qta":2}],"visti":[],"relazioni_viste":[{"allegato":%q,"padre":"#1","figlio":"#2"}]}`,
		s.prodotto, s.prodotto, nodo, s.allStep)
	resp, html := w.daFascicolo(http.MethodPost, s.base()+"/bom/applica", url.Values{"struttura": {struttura}}, s.thread, "?vista=bom")
	if a := avvisoF(html); a != "Struttura di 52922757 confermata: 1 componente nuovo, 1 legame aggiunto." {
		t.Fatalf("editor: %q", a)
	}
	if h := resp.Header.Get("HX-Trigger"); !strings.Contains(h, `"bom-esito":{"ok":true`) || strings.ContainsFunc(h, func(r rune) bool { return r > 127 }) {
		t.Errorf("l'evento per l'editor, in ASCII: %q", h)
	}
	if got := s.valore(`SELECT string_agg(stato::text, ' ') FROM relazione_proposta WHERE allegato_id = $1`, s.allStep); got != "confermata" {
		t.Errorf("la proposta dell'arco dopo l'editor: %s", got)
	}

	// adesso il disegno dell'assieme e' pronto
	_, html = w.fai(http.MethodGet, s.base(), nil, false)
	if !strings.Contains(html, "<b>1</b> pronto") {
		t.Errorf("dopo l'editor il disegno dell'assieme e' pronto:\n%s", estratto(html, `id="piano"`))
	}
	a = s.gestoC(w, url.Values{"firma": {firmaDellaPagina(t, html)}})
	if !strings.Contains(a, "Fascicolo confermato") || !strings.Contains(a, "1 documento") {
		t.Errorf("la seconda conferma: %q", a)
	}
	if got := s.valore(`SELECT string_agg(c.codice || ':' || c.tipo::text, ' ' ORDER BY c.codice) FROM componente c WHERE thread_id = $1`, s.thread); got != "52920517:sottoassieme 52922757:finito" {
		t.Errorf("componenti: %s", got)
	}
	if got := s.valore(`SELECT string_agg(p.codice || '>' || f.codice || 'x' || r.qta, ' ') FROM componente_relazione r JOIN componente p ON p.componente_id = r.padre_id
		JOIN componente f ON f.componente_id = r.figlio_id WHERE r.thread_id = $1`, s.thread); got != "52922757>52920517x2" {
		t.Errorf("archi: %s", got)
	}
	if got := s.valore(`SELECT string_agg(d.nome_file || '@' || coalesce(c.codice, '-'), ' ' ORDER BY d.nome_file) FROM documento d LEFT JOIN componente c ON c.componente_id = d.componente_id
		WHERE d.thread_id = $1`, s.thread); got != "52920517.pdf@52920517 52922757.pdf@52922757 52922757.stp@52922757" {
		t.Errorf("documenti: %s", got)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM job WHERE tipo = 'copia_nas'`); n != 3 {
		t.Errorf("copie sul NAS: %d", n)
	}
	if got := s.valore(`SELECT d.nome_file FROM componente c JOIN documento d ON d.documento_id = c.step_strutturale_id WHERE c.componente_id = $1`, s.prodotto); got != "52922757.stp" {
		t.Errorf("STEP strutturale: %s", got)
	}
	if got := s.valore(`SELECT stato::text FROM documento_proposta WHERE proposta_id = $1`, s.boh); got != "aperta" {
		t.Errorf("il PDF anonimo resta da decidere: %s", got)
	}
	_, html = w.fai(http.MethodGet, s.base(), nil, false)
	if !strings.Contains(html, "<b>0</b> pronti") || !strings.Contains(html, `id="conferma-fascicolo" disabled`) {
		t.Error("dopo la conferma non resta niente di pronto")
	}
}

func (s *scenaConferma) valore(sql string, arg ...any) string {
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

// gestoC e' la conferma dalla schermata.
func (s *scenaConferma) gestoC(w *browser, form url.Values) string {
	s.b.t.Helper()
	_, html := w.daFascicolo(http.MethodPost, s.base()+"/conferma", form, s.thread, "")
	return avvisoF(html)
}

// Decidere un file: il tipo e il codice diventano della proposta, con fonte operatore, e una lettura che
// arriva dopo (l'analisi rifatta) non li riscrive; resta nei dettagli. Un documento tecnico senza codice, un
// tipo «da determinare», una proposta gia' agganciata si rifiutano.
func TestDecidereUnFileNonSiFaRiscrivereDaUnaLettura(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaConferma("DEC7B")
	w := operatore(b)
	decidi := func(pid uuid.UUID, form url.Values) string {
		_, html := w.daFascicolo(http.MethodPost, s.base()+"/proposta/"+pid.String()+"/decidi", form, s.thread, "?cassetto=verifica")
		return avvisoF(html)
	}
	if a := decidi(s.boh, url.Values{"tipo": {"disegno_2d"}}); !strings.Contains(a, "solo con il codice del pezzo") {
		t.Errorf("tecnico senza codice: %q", a)
	}
	if a := decidi(s.boh, url.Values{"tipo": {"da_determinare"}}); !strings.Contains(a, "scegli che cos'è il file") {
		t.Errorf("da determinare: %q", a)
	}
	s.esegui(`UPDATE documento_proposta SET componente_id = $2, codice = '52922757' WHERE proposta_id = $1`, s.pdfProdotto, s.prodotto)
	if a := decidi(s.pdfProdotto, url.Values{"tipo": {"disegno_2d"}, "codice": {"53000000"}}); !strings.Contains(a, "è assegnato a un componente") {
		t.Errorf("proposta agganciata: %q", a)
	}
	if a := decidi(s.boh, url.Values{"tipo": {"capitolato"}}); !strings.HasPrefix(a, "anonimo.pdf: capitolato.") {
		t.Fatalf("decisione: %q", a)
	}
	if got := s.valore(`SELECT tipo_proposto::text || '/' || fonte::text FROM documento_proposta WHERE proposta_id = $1`, s.boh); got != "capitolato/operatore" {
		t.Errorf("proposta decisa: %s", got)
	}
	// la lettura dopo: l'analisi rifatta dice ancora «da determinare»
	var aid uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT allegato_id FROM documento_proposta WHERE proposta_id = $1`, s.boh).Scan(&aid); err != nil {
		t.Fatal(err)
	}
	if _, err := b.q.UpsertProposta(b.ctx, db.UpsertPropostaParams{AllegatoID: aid, ThreadID: uuid.NullUUID{UUID: s.thread, Valid: true},
		TipoProposto: db.TipoDocumentoDaDeterminare, Confidenza: 30, Fonte: db.FontePropostaEstensione, Dettagli: []byte(`{"testo_letto": 0}`)}); err != nil {
		t.Fatal(err)
	}
	if got := s.valore(`SELECT tipo_proposto::text || '/' || fonte::text || '/' || (dettagli -> 'lettura_dopo' ->> 'tipo') FROM documento_proposta WHERE proposta_id = $1`, s.boh); got != "capitolato/operatore/da_determinare" {
		t.Errorf("dopo la lettura: %s", got)
	}
}

// «Importa dal NAS»: il server sfoglia e cerca sotto la radice; il file scelto va nello staging come un
// caricamento interno, con la sua provenienza. Un percorso che esce dalla radice, assoluto, o un
// collegamento che porta fuori, si rifiutano; chi consulta non sfoglia.
func TestImportaDalNasRestaSottoLaRadice(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaConferma("NAS7B")
	b.caricamentoAcceso(t)
	radice, fuori := t.TempDir(), t.TempDir()
	b.ws.NAS = &nas.Scrittore{Radice: radice, DryRun: true}
	t.Cleanup(func() { b.ws.NAS = nil })
	scrivi := func(p, contenuto string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(contenuto), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	scrivi(filepath.Join(radice, "ACME", "ARCHIVIO 2025", "52922757", "52922757.dxf"), "0\nSECTION\n0\nEOF\n")
	scrivi(filepath.Join(radice, "ACME", "ARCHIVIO 2025", "listino.xlsx"), "xlsx")
	scrivi(filepath.Join(fuori, "segreto.dxf"), "0\nSEGRETO\n")
	w := operatore(b)

	_, html := w.daFascicolo(http.MethodGet, s.base()+"/nas?cassetto=nas&nas_cerca=52922757", nil, s.thread, "?cassetto=nas")
	for _, c := range []string{"Trovati sotto ACME", "52922757.dxf", `name="percorso" value="ACME/ARCHIVIO 2025/52922757/52922757.dxf"`, "Importa"} {
		if !strings.Contains(html, c) {
			t.Errorf("ricerca: manca %q", c)
		}
	}
	_, html = w.daFascicolo(http.MethodGet, s.base()+"/nas?cassetto=nas&nas=ACME/ARCHIVIO%202025", nil, s.thread, "?cassetto=nas")
	if !strings.Contains(html, "listino.xlsx") || !strings.Contains(html, "si importano CAD 3D, disegni (PDF, DWG) e sviluppi DXF") {
		t.Error("la cartella mostra anche i file che non si importano, con il motivo")
	}
	_, html = w.daFascicolo(http.MethodGet, s.base()+"/nas?cassetto=nas&nas=../..", nil, s.thread, "?cassetto=nas")
	if !strings.Contains(html, "esce dalla radice del NAS") {
		t.Error("una cartella fuori dalla radice non si apre")
	}

	importa := func(p string) string {
		_, html := w.daFascicolo(http.MethodPost, s.base()+"/nas/importa", url.Values{"percorso": {p}}, s.thread, "?cassetto=nas")
		return avvisoF(html)
	}
	rel, _ := filepath.Rel(radice, filepath.Join(fuori, "segreto.dxf"))
	for _, p := range []string{rel, filepath.ToSlash(rel), filepath.Join(fuori, "segreto.dxf"), "/etc/passwd", `\\server\share\x.dxf`} {
		if a := importa(p); !strings.HasPrefix(a, "Niente è cambiato:") {
			t.Errorf("%q: accettato (%q)", p, a)
		}
	}
	if err := os.Symlink(filepath.Join(fuori, "segreto.dxf"), filepath.Join(radice, "ACME", "collegamento.dxf")); err == nil {
		if a := importa("ACME/collegamento.dxf"); !strings.HasPrefix(a, "Niente è cambiato:") {
			t.Errorf("un collegamento che esce dalla radice: accettato (%q)", a)
		}
	} else {
		t.Logf("collegamento simbolico non creabile qui (%v): la prova del collegamento e' saltata", err)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM allegato WHERE origine = 'manuale'`); n != 0 {
		t.Fatalf("i rifiuti hanno registrato %d file", n)
	}

	if a := importa("ACME/ARCHIVIO 2025/52922757/52922757.dxf"); !strings.Contains(a, "52922757.dxf importato dal NAS") {
		t.Fatalf("importazione: %q", a)
	}
	if got := s.valore(`SELECT a.path_interno || '|' || a.stato::text || '|' || p.tipo_proposto::text FROM allegato a JOIN documento_proposta p ON p.allegato_id = a.allegato_id
		WHERE a.origine = 'manuale'`); got != "NAS: ACME/ARCHIVIO 2025/52922757/52922757.dxf|in_staging|sviluppo_dxf" {
		t.Errorf("il file importato: %s", got)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM documento`); n != 0 {
		t.Error("importare non conferma niente: il documento nasce con la conferma")
	}

	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")
	if resp, _ := co.daFascicolo(http.MethodGet, s.base()+"/nas", nil, s.thread, ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("chi consulta non sfoglia il NAS: %d", resp.StatusCode)
	}
}

// Il poll: con del lavoro in corso la pagina chiede; se la firma e' quella della pagina risponde con il solo
// avanzamento, se e' cambiata rifa' i pannelli; finito il lavoro l'elemento torna senza poll.
func TestIlPollRifaIPannelliSoloSeQualcosaECambiato(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaConferma("POLL7B")
	w := operatore(b)
	var sha string
	if err := b.pool.QueryRow(b.ctx, `SELECT sha256 FROM allegato WHERE allegato_id = $1`, s.allStep).Scan(&sha); err != nil {
		t.Fatal(err)
	}
	var job int64
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO job (tipo, worker_tipo, payload, chiave_idempotenza, priorita)
		VALUES ('analizza_allegato', 'analisi', jsonb_build_object('allegato_id', $1::text, 'sha256', $2::text), 'analizza:poll', 6) RETURNING job_id`,
		s.allStep, sha).Scan(&job); err != nil {
		t.Fatal(err)
	}
	_, html := w.fai(http.MethodGet, s.base(), nil, false)
	m := regexp.MustCompile(`hx-get="(/thread/[^"]+/avanzamento\?firma=([0-9a-f]+)&amp;giro=0)"`).FindStringSubmatch(html)
	if m == nil || !strings.Contains(html, `hx-trigger="every 2s"`) {
		t.Fatalf("con un'analisi in corso la pagina chiede:\n%s", estratto(html, `id="fasc-avanzamento"`))
	}
	firma := m[2]
	// htmx aggiunge al poll quello che mostra il corpo del pannello di destra (hx-include sulla radice)
	corpo := "&corpo_chiave=" + url.QueryEscape(chiaveCorpoDi(t, html))
	_, risposta := w.daFascicolo(http.MethodGet, s.base()+"/avanzamento?firma="+firma+"&giro=0"+corpo, nil, s.thread, "")
	if !strings.Contains(risposta, `id="fasc-avanzamento"`) || !strings.Contains(risposta, "giro=1") || strings.Contains(risposta, "hx-swap-oob") {
		t.Errorf("stessa firma: solo l'avanzamento, con il giro dopo:\n%.400s", risposta)
	}
	_, risposta = w.daFascicolo(http.MethodGet, s.base()+"/avanzamento?firma=0000&giro=4"+corpo, nil, s.thread, "")
	for _, id := range []string{"fasc-testata", "vista", "piano", "anteprima-testa", "cassetto"} {
		if !strings.Contains(risposta, `id="`+id+`" hx-swap-oob="innerHTML"`) {
			t.Errorf("firma cambiata: manca il pannello %s", id)
		}
	}
	if strings.Contains(risposta, "anteprima-corpo") {
		t.Error("il poll non tocca il corpo del pannello di destra che la pagina sta mostrando")
	}
	// una pagina che mostra un altro corpo lo riceve rifatto, anche se la firma e' la stessa
	_, risposta = w.daFascicolo(http.MethodGet, s.base()+"/avanzamento?firma="+firma+"&giro=0&corpo_chiave=pdf%3Aaltro", nil, s.thread, "")
	if !strings.Contains(risposta, `id="anteprima-corpo" hx-swap-oob="innerHTML"`) || !strings.Contains(risposta, `id="anteprima-testa" hx-swap-oob="innerHTML"`) {
		t.Errorf("il corpo che la pagina mostra non e' piu' quello: il poll lo rifa' con la sua intestazione:\n%.400s", risposta)
	}
	if _, err := b.pool.Exec(b.ctx, `UPDATE job SET stato = 'fatto', chiuso_il = now() WHERE job_id = $1`, job); err != nil {
		t.Fatal(err)
	}
	_, risposta = w.daFascicolo(http.MethodGet, s.base()+"/avanzamento?firma="+firma+"&giro=1"+corpo, nil, s.thread, "")
	elemento := regexp.MustCompile(`(?s)<div id="fasc-avanzamento"[^>]*>`).FindString(risposta)
	if elemento == "" || strings.Contains(elemento, "hx-trigger") || strings.Contains(elemento, "hx-get") || !strings.Contains(risposta, "hx-swap-oob") {
		t.Errorf("finito il lavoro: niente poll, e i pannelli rifatti:\n%s\n%.300s", elemento, risposta)
	}
}

func contaSQL(t *testing.T, b *bancoWeb, sql string, arg ...any) int {
	t.Helper()
	var n int
	if err := b.pool.QueryRow(b.ctx, sql, arg...).Scan(&n); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return n
}

func uuidSQL(t *testing.T, b *bancoWeb, sql string, arg ...any) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := b.pool.QueryRow(b.ctx, sql, arg...).Scan(&id); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return id
}

var _ = coda.Capacita{}
