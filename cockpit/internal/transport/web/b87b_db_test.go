//go:build integrazione

// L4 — B8.7b sulla porta HTTP vera: la RFQ che nasce fa dei codici della richiesta i prodotti e prepara da
// sola i file utili; l'aggancio fa lo stesso; aprire il Fascicolo non scrive, e la preparazione la chiede la
// pagina con una POST, solo per chi lavora (Smistamento F1); «Conferma Fascicolo» porta nel
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

// firmaDellaPagina e' la firma del modulo che la pagina mostra: dal giro 4, fase 4.1b, quella del riepilogo di
// «Conferma Fascicolo» (il cassetto «Rivedi»), non piu' quella del piano.
func firmaDellaPagina(t *testing.T, html string) string {
	t.Helper()
	m := reFirma.FindStringSubmatch(html)
	if m == nil {
		t.Fatal("la pagina non porta la firma del riepilogo")
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
//
// Riscritta per lo Smistamento (R8, E11, fase F1): prima si fermava alla creazione, e la GET del Fascicolo
// avrebbe rifatto prodotti e preparazione a ogni apertura. Adesso i prodotti nascono solo qui: dopo la
// creazione, aprire la RFQ e il Fascicolo non cambia il numero di nessuna riga, e la pagina del Fascicolo
// chiede la preparazione con la sua POST.
func TestCreareLaRfqFaDelCodiceUnProdottoEPreparaLoZip(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	msg := b.mailConAllegati("Richiesta di offerta 77722757", "Offerta per il supporto 77722757, 200 pezzi.",
		worker.AllegatoIn{Indice: 1, NomeFile: "RFQ ACME 77722757.zip", Estensione: "zip", Natura: "file", Bytes: 4000},
		worker.AllegatoIn{Indice: 2, NomeFile: "logo.png", Estensione: "png", Natura: "file", Bytes: 2000})
	w := operatore(b)

	_, form := w.fai(http.MethodGet, "/messaggio/"+msg.String()+"/triage?azione=nuova", nil, true)
	if !strings.Contains(form, "si prepara da solo") || !strings.Contains(form, "scarica anche questo") {
		t.Fatalf("il form dice che lo zip si prepara da solo e offre la spunta per l'immagine:\n%s", estrai(form, "Allegati"))
	}
	if strings.Contains(form, "e scarica i file spuntati") {
		t.Error("il bottone non parla piu' di file spuntati")
	}
	if !strings.Contains(form, `value="77722757"`) {
		t.Fatalf("il codice 77722757 e' fra i candidati del form")
	}

	_, html := w.fai(http.MethodPost, "/messaggio/"+msg.String()+"/rfq", url.Values{"cliente_id": {acme.String()}, "oggetto": {"Supporto 77722757"},
		"codice": {"77722757"}, "priorita": {"1"}}, true)
	a := leggibile(html)
	for _, c := range []string{"RFQ creata", "77722757 nella BOM come prodotto finito", "1 file utile in preparazione"} {
		if !strings.Contains(a, c) {
			t.Errorf("avviso: manca %q in %q", c, estrai(html, "avviso"))
		}
	}
	var thread uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT thread_id FROM messaggio WHERE messaggio_id = $1`, msg).Scan(&thread); err != nil {
		t.Fatal(err)
	}
	var tipo, origine string
	if err := b.pool.QueryRow(b.ctx, `SELECT tipo::text, origine::text FROM componente WHERE thread_id = $1 AND codice = '77722757'`, thread).Scan(&tipo, &origine); err != nil {
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

	prima := righeDelDatabase(t, b)
	_, pagina := w.fai(http.MethodGet, "/thread/"+thread.String()+"/fascicolo", nil, false)
	w.fai(http.MethodGet, "/thread/"+thread.String(), nil, false)
	if dopo := righeDelDatabase(t, b); dopo != prima {
		t.Errorf("aprire la RFQ appena creata ha cambiato il database:\nprima %s\ndopo  %s", prima, dopo)
	}
	if !strings.Contains(pagina, `hx-post="/thread/`+thread.String()+`/fascicolo/prepara" hx-trigger="load"`) {
		t.Error("il Fascicolo chiede la preparazione con la sua POST")
	}
}

// Agganciare un messaggio a una RFQ fa lo stesso: lo STEP del messaggio scende, e il codice della richiesta
// che chi aggancia conferma diventa il prodotto (una RFQ di prima, che non l'aveva).
//
// Riscritta per lo Smistamento (R8, E11, fase F1): prima una GET del Fascicolo avrebbe gia' fatto il prodotto
// della RFQ di prima, a chiunque l'avesse aperta. Adesso aprirla non crea niente: il prodotto nasce con
// l'aggancio, che e' la decisione di una persona. Che l'aggancio rilegga gli STEP gia' analizzati lo fissa la
// prova 100 (TestAgganciareRileggeGliStepGiaAnalizzati).
//
// Riscritta di nuovo per lo Smistamento (scelta 6, 27/09): prima fissava che l'aggancio, senza confermare
// niente, facesse il prodotto di OGNI codice confermato della richiesta che non ne aveva uno (ed e' cosi' che
// un prodotto tolto rinasceva). Adesso l'aggancio che non conferma codici prepara i file e non crea prodotti;
// il prodotto nasce con l'aggancio che spunta il codice della richiesta. La rinascita di un prodotto tolto la
// fissa TestUnProdottoToltoNonRinasceAllAggancio.
func TestAgganciareUnMessaggioPreparaIFileEAssicuraIProdotti(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	var fp uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT utente_id FROM utente WHERE sigla = 'FP'`).Scan(&fp); err != nil {
		t.Fatal(err)
	}
	thread := b.rfqDa(uuid.Nil, acme, `ACME\WIP\2026 09 24 Rossi RFQ 77722757`, "77722757")
	if _, err := b.pool.Exec(b.ctx, `UPDATE identificativo_thread SET confermato_da = $2 WHERE thread_id = $1`, thread, fp); err != nil {
		t.Fatal(err)
	}
	msg := b.mailConAllegati("RE: RFQ 77722757", "Ecco lo STEP.", worker.AllegatoIn{Indice: 1, NomeFile: "77722757.stp", Estensione: "stp", Natura: "file", Bytes: 9000})
	w := operatore(b)
	if resp, _ := w.fai(http.MethodGet, "/thread/"+thread.String()+"/fascicolo", nil, false); resp.StatusCode != 200 {
		t.Fatalf("apertura del Fascicolo: %d", resp.StatusCode)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM componente WHERE thread_id = $1`, thread); n != 0 {
		t.Fatalf("aprire il Fascicolo ha creato %d componenti: i prodotti nascono dal triage", n)
	}
	// l'aggancio che non conferma codici: lo STEP scende, il prodotto no
	_, html := w.fai(http.MethodPost, "/messaggio/"+msg.String()+"/aggancia", url.Values{"thread_id": {thread.String()}}, true)
	a := leggibile(html)
	if !strings.Contains(a, "Agganciato") || strings.Contains(a, "nella BOM come prodott") || !strings.Contains(a, "1 file utile in preparazione") {
		t.Fatalf("avviso: %q", estrai(html, "avviso"))
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM job WHERE tipo = 'stage_allegato'`); n != 1 {
		t.Errorf("download dello STEP: %d", n)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM componente WHERE thread_id = $1`, thread); n != 0 {
		t.Errorf("l'aggancio senza codici confermati ha creato %d componenti", n)
	}
	// l'aggancio che spunta il codice della richiesta: il prodotto nasce
	msg2 := b.mailConAllegati("RE: RFQ 77722757", "Confermiamo 77722757.")
	_, html = w.fai(http.MethodPost, "/messaggio/"+msg2.String()+"/aggancia", url.Values{"thread_id": {thread.String()}, "codice": {"77722757"}}, true)
	a = leggibile(html)
	if !strings.Contains(a, "Agganciato") || !strings.Contains(a, "77722757 nella BOM come prodotto finito") {
		t.Fatalf("avviso: %q", estrai(html, "avviso"))
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM componente WHERE thread_id = $1 AND tipo = 'finito' AND codice = '77722757'`, thread); n != 1 {
		t.Errorf("il prodotto nato con l'aggancio: %d", n)
	}
}

// Aprire il Fascicolo lo fa preparare, a chi lavora: una RFQ di prima con il codice della richiesta e un
// allegato utile non ancora sceso. Chi consulta la guarda e non mette in moto niente.
//
// Riscritta per lo Smistamento (R8, E11, E29, fase F1): prima fissava che la GET dell'operatore facesse il
// prodotto dal codice della richiesta e il download, e che la pagina nascesse con il poll dell'avanzamento.
// Adesso la GET non scrive, per nessuno: la pagina dell'operatore porta la POST della preparazione
// (hx-trigger="load"), che fa scendere il disegno e fa partire il poll, ma non fa il prodotto (quello nasce
// al triage); la pagina di chi consulta non la porta, e la stessa POST gli risponde 403.
func TestAprireIlFascicoloLoPreparaPerChiLavora(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	msg := b.mailConAllegati("RFQ 77722757", "Richiesta per 77722757.", worker.AllegatoIn{Indice: 1, NomeFile: "77722757.pdf", Estensione: "pdf", Natura: "file", Bytes: 9000})
	thread := b.rfqDa(msg, acme, `ACME\WIP\2026 09 24 Rossi RFQ 77722757`, "77722757")
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
	resp, pagina := co.fai(http.MethodGet, base, nil, false)
	if resp.StatusCode != 200 {
		t.Fatalf("consultazione: %d", resp.StatusCode)
	}
	if strings.Contains(pagina, "/fascicolo/prepara") {
		t.Error("la pagina di chi consulta non chiede la preparazione e non ha il bottone")
	}
	if resp, _ := co.fai(http.MethodPost, base+"/prepara", url.Values{"auto": {"1"}}, true); resp.StatusCode != http.StatusForbidden {
		t.Errorf("la preparazione chiesta da chi consulta: %d, atteso 403", resp.StatusCode)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM componente`); n != 0 {
		t.Errorf("chi consulta non crea prodotti: %d", n)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM job WHERE tipo = 'stage_allegato'`); n != 0 {
		t.Errorf("chi consulta non scarica: %d", n)
	}

	w := operatore(b)
	_, html := w.fai(http.MethodGet, base, nil, false)
	if n := contaSQL(t, b, `SELECT count(*) FROM componente`); n != 0 {
		t.Errorf("la GET dell'operatore ha creato %d componenti", n)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM job WHERE tipo = 'stage_allegato'`); n != 0 {
		t.Errorf("la GET dell'operatore ha accodato %d download", n)
	}
	for _, c := range []string{`hx-post="/thread/` + thread.String() + `/fascicolo/prepara" hx-trigger="load" hx-vals='{"auto": "1"}'`, ">Prepara i file</button>"} {
		if !strings.Contains(leggibile(html), c) {
			t.Errorf("la pagina dell'operatore: manca %q", c)
		}
	}
	if strings.Contains(html, `hx-trigger="every 2s"`) {
		t.Error("prima della preparazione non c'e' lavoro da seguire")
	}

	resp, html = w.daFascicolo(http.MethodPost, base+"/prepara", url.Values{"auto": {"1"}}, thread, "")
	if resp.StatusCode != 200 {
		t.Fatalf("preparazione: %d", resp.StatusCode)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM job WHERE tipo = 'stage_allegato' AND stato = 'pronto'`); n != 1 {
		t.Errorf("il disegno scende da solo: %d", n)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM componente`); n != 0 {
		t.Errorf("la preparazione ha creato %d componenti: il prodotto nasce al triage", n)
	}
	for _, c := range []string{"Preparazione dei file", "1 download", `hx-trigger="every 2s"`, `hx-swap-oob`} {
		if !strings.Contains(html, c) {
			t.Errorf("la risposta della preparazione: manca %q", c)
		}
	}
	if a := avvisoF(html); a != "" {
		t.Errorf("la preparazione automatica non ha avviso: %q", a)
	}
	// una seconda preparazione automatica non trova niente da fare, e la pagina resta com'e'
	if resp, _ := w.daFascicolo(http.MethodPost, base+"/prepara", url.Values{"auto": {"1"}}, thread, ""); resp.StatusCode != http.StatusNoContent {
		t.Errorf("seconda preparazione automatica: %d, atteso 204", resp.StatusCode)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM job WHERE tipo = 'stage_allegato'`); n != 1 {
		t.Errorf("la seconda preparazione ha accodato di nuovo: %d", n)
	}
}

// scenaConferma: il prodotto (dal codice della richiesta), il suo STEP che propone l'assieme 77720517 ×2, i
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
	s.prodotto = r.componenteTipo("77722757", "finito")
	var sha string
	s.allStep, sha = r.allegatoExt("77722757.stp", "stp")
	r.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1,$2,'cad_3d','77722757',95,'step')`, s.allStep, r.thread)
	s.stp = uuidSQL(b.t, b, `SELECT proposta_id FROM documento_proposta WHERE allegato_id = $1`, s.allStep)
	r.esegui(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, codice, origine_codice, tipo_proposto, fonte, confidenza, stato, componente_id)
		VALUES ($1,$2,$3,'#1','77722757','77722757','generico','finito','step',90,'duplicato',$4)`, r.thread, s.allStep, sha, s.prodotto)
	r.esegui(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, codice, origine_codice, tipo_proposto, fonte, confidenza)
		VALUES ($1,$2,$3,'#2','77720517','77720517','generico','sottoassieme','step',90)`, r.thread, s.allStep, sha)
	r.esegui(`INSERT INTO relazione_proposta (thread_id, allegato_id, padre_chiave, figlio_chiave, qta) VALUES ($1,$2,'#1','#2',2)`, r.thread, s.allStep)
	s.pdfProdotto, _ = r.propostaDa("77722757.pdf", "77722757")
	s.pdfAssieme, _ = r.propostaDa("77720517.pdf", "77720517")
	s.boh, _ = r.propostaDa("anonimo.pdf", "")
	r.esegui(`UPDATE documento_proposta SET tipo_proposto = 'da_determinare', confidenza = 30, fonte = 'estensione' WHERE proposta_id = $1`, s.boh)
	return s
}

// «Conferma Fascicolo» porta nel fascicolo il piano pronto in una transazione: i documenti e lo STEP
// strutturale. Fascicolo v3: la struttura dello STEP non entra con quel gesto, si conferma nell'editor della
// Struttura BOM (bom/applica); il disegno dell'assieme che nasce da lei la aspetta, e diventa pronto dopo.
// Con una firma vecchia, o con un file senza la struttura da cui dipende, non cambia niente. Il PDF anonimo
// resta da decidere.
//
// Riscritta per lo Smistamento (F5, A5.4): prima la struttura dello STEP era visibile e aspettata dal disegno
// dell'assieme gia' prima della conferma, perche' la sua radice era agganciata al prodotto per il codice. Adesso
// finche' lo STEP non e' autorizzato per il prodotto la sua struttura e' guida: niente banner, una riga di guida,
// e il disegno dell'assieme non la aspetta (77720517 non e' nella BOM, e resta da verificare con il PDF anonimo: due, prima tre con la struttura). La conferma fa
// dello STEP lo STEP strutturale del prodotto (la forma di prima dell'autorizzazione, fino a F5b): da li' la
// radice e' il prodotto, e la struttura compare nella Struttura BOM e si conferma nell'editor come prima.
//
// Riscritta di nuovo per lo Smistamento (F5b, P26, U7; A5.4.7 supera A4.4): prima fissava che «Conferma
// Fascicolo» (la firma del piano, o la casella di «Rivedi» nata spuntata) facesse dell'unico STEP del prodotto lo
// STEP strutturale, con la marcatura. Adesso la conferma del piano non autorizza niente: l'unico STEP e' una voce
// «Da verificare», il campo `strutturale` si rifiuta, e dopo la conferma non ci sono ne' lo STEP strutturale ne'
// la marcatura, e la struttura resta guida. Lo STEP si autorizza con l'anteprima e la casella (autorizzaDallaScheda,
// che vuole l'analisi corrente del file: la scena ha i fatti dello STEP); da li' la struttura si vede e si
// conferma nell'editor come prima.
//
// Riscritta di nuovo per il giro 4, fase 4.1b (domanda 9b = A): prima la conferma prendeva la firma del piano dalla
// pagina (il modulo in fondo, con la firma gia' dentro) e con quella sola confermava tutto il pronto. Adesso la firma
// e' quella del riepilogo (la GET del cassetto «Rivedi», che il bottone apre) e serve anche `conferma=1`: una firma
// inventata dice che il riepilogo e' cambiato, la firma giusta senza la conferma non basta. Il resto e' com'era.
func TestConfermaFascicoloPortaNelFascicoloSoloIlPianoVisto(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	s := b.scenaConferma("CNF7B")
	s.analisiCorrente(s.valore(`SELECT sha256 FROM allegato WHERE allegato_id = $1`, s.allStep),
		fattiB87(false))
	w := operatore(b)

	_, html := w.fai(http.MethodGet, s.base(), nil, false)
	for _, c := range []string{"<b>2</b> pronti", "<b>3</b> da verificare"} {
		if !strings.Contains(html, c) {
			t.Errorf("la pagina: manca %q", c)
		}
	}
	if strings.Contains(html, "· STEP strutturale di 77722757") {
		t.Error("lo STEP strutturale non e' fra le voci pronte del piano")
	}
	_, rivedi := w.fai(http.MethodGet, s.base()+"/parti?cassetto=piano", nil, true)
	if strings.Contains(rivedi, `name="strutturale"`) || regexp.MustCompile(`<input type="checkbox" name="[a-z]+" value="`+s.prodotto.String()+`"[^>]*checked`).MatchString(rivedi) {
		t.Error("«Rivedi» non ha la casella dello STEP strutturale, tanto meno spuntata")
	}
	_, verifica := w.fai(http.MethodGet, s.base()+"/parti?cassetto=verifica", nil, true)
	if !strings.Contains(verifica, "77722757.stp sarà l&#39;unico STEP di 77722757") {
		t.Errorf("«Da verificare» dice lo STEP da autorizzare:\n%s", estratto(verifica, "STEP strutturale"))
	}
	if strings.Contains(html, `name="firma"`) {
		t.Error("la pagina porta un modulo con la firma: la conferma si fa dal riepilogo")
	}
	firma := firmaDellaPagina(t, rivedi)
	_, bom := w.fai(http.MethodGet, s.base()+"?vista=bom", nil, false)
	if strings.Contains(bom, "Apri proposta BOM") || strings.Contains(bom, "+ proposto · assieme?") {
		t.Error("la struttura di uno STEP non autorizzato e' guida: niente banner e niente proposte nella BOM")
	}
	if !strings.Contains(bom, "1 STEP analizzato non è autorizzato per nessun componente") {
		t.Error("la Struttura BOM dice la guida in una riga")
	}
	if strings.Contains(bom, "Applica struttura proposta") {
		t.Error("la struttura dello STEP non si applica piu' dal banner: si apre nell'editor")
	}

	if a := s.gestoC(w, url.Values{"firma": {"deadbeef00000000"}, "conferma": {"1"}}); !strings.HasPrefix(a, "Niente è cambiato: il riepilogo è cambiato") {
		t.Fatalf("firma vecchia: %q", a)
	}
	if a := s.gestoC(w, url.Values{"firma": {firma}}); !strings.HasPrefix(a, "Niente è cambiato: manca la conferma") {
		t.Fatalf("la firma del riepilogo senza la conferma: %q", a)
	}
	if a := s.gestoC(w, url.Values{"voce": {s.pdfAssieme.String()}}); !strings.Contains(a, "77720517 non è nella BOM") {
		t.Fatalf("il disegno di un pezzo che non c'e': %q", a)
	}
	if a := s.gestoC(w, url.Values{"struttura": {s.allStep.String()}}); !strings.HasPrefix(a, "Niente è cambiato") {
		t.Fatalf("la struttura mandata alla conferma: %q", a)
	}
	if a := s.gestoC(w, url.Values{"voce": {s.stp.String()}, "strutturale": {s.prodotto.String()}}); !strings.Contains(a, "lo STEP strutturale non si conferma con il piano") {
		t.Fatalf("la casella dello STEP strutturale di prima mandata alla conferma: %q", a)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM documento`); n != 0 {
		t.Fatalf("i rifiuti hanno lasciato %d documenti", n)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM componente WHERE codice = '77720517'`); n != 0 {
		t.Fatalf("i rifiuti hanno fatto nascere l'assieme: %d", n)
	}

	a := s.gestoC(w, url.Values{"firma": {firma}, "conferma": {"1"}})
	for _, c := range []string{"Fascicolo confermato", "2 documenti (copie sul NAS in coda)"} {
		if !strings.Contains(a, c) {
			t.Errorf("avviso: manca %q in %q", c, a)
		}
	}
	if strings.Contains(a, "struttura di 77722757.stp") || strings.Contains(a, "STEP strutturale") {
		t.Errorf("«Conferma Fascicolo» ha confermato la struttura o autorizzato lo STEP: %q", a)
	}
	// la conferma del piano non autorizza niente: ne' lo STEP strutturale ne' la marcatura, e la struttura e' guida
	if got := s.valore(`SELECT step_strutturale_id::text FROM componente WHERE componente_id = $1`, s.prodotto); got != "NULL" {
		t.Errorf("la conferma del piano ha fissato lo STEP strutturale: %s", got)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM componente_proposta WHERE thread_id = $1 AND evidenza ? 'strutturale'`, s.thread); n != 0 {
		t.Errorf("la conferma del piano ha scritto %d marcature", n)
	}
	_, bom = w.fai(http.MethodGet, s.base()+"?vista=bom", nil, false)
	if strings.Contains(bom, "Apri proposta BOM") {
		t.Error("dopo la conferma del piano la struttura dello STEP e' ancora guida")
	}
	doc := uuidSQL(t, b, `SELECT documento_id FROM documento WHERE thread_id = $1 AND nome_file = '77722757.stp'`, s.thread)
	_, verifica = w.fai(http.MethodGet, s.base()+"/parti?cassetto=verifica", nil, true)
	if !strings.Contains(verifica, "77722757.stp è l&#39;unico STEP di 77722757") || !strings.Contains(verifica, `<option value="">— scegli lo STEP —</option>`) ||
		strings.Contains(verifica, `value="`+doc.String()+`" selected`) {
		t.Errorf("«Da verificare» chiede l'autorizzazione senza STEP scelto:\n%s", estratto(verifica, "STEP strutturale"))
	}
	// l'autorizzazione e' il gesto di una persona: l'anteprima, la casella, la firma vista
	if a := s.autorizzaDallaScheda(w, s.prodotto, url.Values{"documento": {doc.String()}}, nil); !strings.Contains(a, "77722757.stp è lo STEP autorizzato di 77722757") {
		t.Fatalf("l'autorizzazione dalla scheda: %q", a)
	}
	if got := s.valore(`SELECT string_agg(c.codice || ':' || c.tipo::text, ' ' ORDER BY c.codice) FROM componente c WHERE thread_id = $1`, s.thread); got != "77722757:finito" {
		t.Errorf("componenti dopo la conferma: %s", got)
	}
	if got := s.valore(`SELECT stato::text FROM documento_proposta WHERE proposta_id = $1`, s.pdfAssieme); got != "aperta" {
		t.Errorf("il disegno dell'assieme aspetta la struttura: %s", got)
	}
	// adesso lo STEP e' lo STEP strutturale del prodotto: la sua struttura e' nell'autorita', e si vede
	_, bom = w.fai(http.MethodGet, s.base()+"?vista=bom", nil, false)
	for _, c := range []string{"Apri proposta BOM", "77720517", "+ proposto · assieme?", "1 nodo e 1 arco proposti: la struttura si rivede e si conferma nell&#39;editor"} {
		if !strings.Contains(bom, c) {
			t.Errorf("la Struttura BOM dopo la conferma: manca %q", c)
		}
	}

	// la struttura nell'editor, com'e' proposta: nasce l'assieme con l'arco ×2, la proposta dell'arco e' presa
	nodo := uuidSQL(t, b, `SELECT proposta_id FROM componente_proposta WHERE allegato_id = $1 AND chiave = '#2'`, s.allStep)
	struttura := fmt.Sprintf(`{"radice":%q,"archi":[{"padre":"c:%s","figlio":"p:%s","qta":2}],"visti":[],"relazioni_viste":[{"allegato":%q,"padre":"#1","figlio":"#2"}]}`,
		s.prodotto, s.prodotto, nodo, s.allStep)
	resp, html := w.daFascicolo(http.MethodPost, s.base()+"/bom/applica", url.Values{"struttura": {struttura}}, s.thread, "?vista=bom")
	if a := avvisoF(html); a != "Struttura di 77722757 confermata: 1 componente nuovo, 1 legame aggiunto." {
		t.Fatalf("editor: %q", a)
	}
	if h := resp.Header.Get("HX-Trigger"); !strings.Contains(h, `"bom-esito":{"ok":true`) || strings.ContainsFunc(h, func(r rune) bool { return r > 127 }) {
		t.Errorf("l'evento per l'editor, in ASCII: %q", h)
	}
	if got := s.valore(`SELECT string_agg(stato::text, ' ') FROM relazione_proposta WHERE allegato_id = $1 AND padre_chiave = '#1'`, s.allStep); got != "confermata" {
		t.Errorf("la proposta dell'arco dopo l'editor: %s", got)
	}

	// adesso il disegno dell'assieme e' pronto
	_, html = w.fai(http.MethodGet, s.base(), nil, false)
	if !strings.Contains(html, "<b>1</b> pronto") {
		t.Errorf("dopo l'editor il disegno dell'assieme e' pronto:\n%s", estratto(html, `id="piano"`))
	}
	_, rivedi = w.fai(http.MethodGet, s.base()+"/parti?cassetto=piano", nil, true)
	a = s.gestoC(w, url.Values{"firma": {firmaDellaPagina(t, rivedi)}, "conferma": {"1"}})
	if !strings.Contains(a, "Fascicolo confermato") || !strings.Contains(a, "1 documento") {
		t.Errorf("la seconda conferma: %q", a)
	}
	if got := s.valore(`SELECT string_agg(c.codice || ':' || c.tipo::text, ' ' ORDER BY c.codice) FROM componente c WHERE thread_id = $1`, s.thread); got != "77720517:sottoassieme 77722757:finito" {
		t.Errorf("componenti: %s", got)
	}
	if got := s.valore(`SELECT string_agg(p.codice || '>' || f.codice || 'x' || r.qta, ' ') FROM componente_relazione r JOIN componente p ON p.componente_id = r.padre_id
		JOIN componente f ON f.componente_id = r.figlio_id WHERE r.thread_id = $1`, s.thread); got != "77722757>77720517x2" {
		t.Errorf("archi: %s", got)
	}
	if got := s.valore(`SELECT string_agg(d.nome_file || '@' || coalesce(c.codice, '-'), ' ' ORDER BY d.nome_file) FROM documento d LEFT JOIN componente c ON c.componente_id = d.componente_id
		WHERE d.thread_id = $1`, s.thread); got != "77720517.pdf@77720517 77722757.pdf@77722757 77722757.stp@77722757" {
		t.Errorf("documenti: %s", got)
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM job WHERE tipo = 'copia_nas'`); n != 3 {
		t.Errorf("copie sul NAS: %d", n)
	}
	if got := s.valore(`SELECT d.nome_file FROM componente c JOIN documento d ON d.documento_id = c.step_strutturale_id WHERE c.componente_id = $1`, s.prodotto); got != "77722757.stp" {
		t.Errorf("STEP strutturale: %s", got)
	}
	if got := s.valore(`SELECT evidenza -> 'strutturale' ->> 'ruolo' FROM componente_proposta WHERE allegato_id = $1 AND chiave = '#1'`, s.allStep); got != "radice" {
		t.Errorf("l'autorizzazione dalla scheda scrive la marcatura: %s", got)
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
	s.esegui(`UPDATE documento_proposta SET componente_id = $2, codice = '77722757' WHERE proposta_id = $1`, s.pdfProdotto, s.prodotto)
	if a := decidi(s.pdfProdotto, url.Values{"tipo": {"disegno_2d"}, "codice": {"77800000"}}); !strings.Contains(a, "è assegnato a un componente") {
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
	scrivi(filepath.Join(radice, "ACME", "ARCHIVIO 2025", "77722757", "77722757.dxf"), "0\nSECTION\n0\nEOF\n")
	scrivi(filepath.Join(radice, "ACME", "ARCHIVIO 2025", "listino.xlsx"), "xlsx")
	scrivi(filepath.Join(fuori, "segreto.dxf"), "0\nSEGRETO\n")
	w := operatore(b)

	_, html := w.daFascicolo(http.MethodGet, s.base()+"/nas?cassetto=nas&nas_cerca=77722757", nil, s.thread, "?cassetto=nas")
	for _, c := range []string{"Trovati sotto ACME", "77722757.dxf", `name="percorso" value="ACME/ARCHIVIO 2025/77722757/77722757.dxf"`, "Importa"} {
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

	if a := importa("ACME/ARCHIVIO 2025/77722757/77722757.dxf"); !strings.Contains(a, "77722757.dxf importato dal NAS") {
		t.Fatalf("importazione: %q", a)
	}
	if got := s.valore(`SELECT a.path_interno || '|' || a.stato::text || '|' || p.tipo_proposto::text FROM allegato a JOIN documento_proposta p ON p.allegato_id = a.allegato_id
		WHERE a.origine = 'manuale'`); got != "NAS: ACME/ARCHIVIO 2025/77722757/77722757.dxf|in_staging|sviluppo_dxf" {
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
