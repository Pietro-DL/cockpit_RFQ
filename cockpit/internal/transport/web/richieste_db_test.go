//go:build integrazione

// L4 — blocco 7B sulla porta HTTP vera: le richieste ai fornitori (IB2–IB5), «Ignora» in Da validare
// (IB6), la newsletter (IB7), materiali e norme che non sono codici (IB8). Nomi e domini inventati.
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/transport/web/
//
// Che cosa NON provano: che Outlook conservi le UserProperties sulla mail inviata (IB2 reale, L5) e
// che il browser mostri i pulsanti (L7).
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/ingest"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// postaMsg fa entrare un MessaggioIn gia' costruito per la strada vera (Commerciale) e restituisce l'id.
func (b *bancoWeb) postaMsg(m worker.MessaggioIn) uuid.UUID {
	b.t.Helper()
	casella, err := b.q.GetCasellaPerIndirizzo(b.ctx, db.GetCasellaPerIndirizzoParams{Canale: db.CanaleOutlook, Indirizzo: "commerciale@azienda.example"})
	if err != nil {
		b.t.Fatal(err)
	}
	s := &ingest.Servizio{Pool: b.pool, Log: testutil.LogSilenzioso()}
	if _, err := s.Ingerisci(b.ctx, ingest.Lotto{Casella: casella, Messaggi: []worker.MessaggioIn{m}}); err != nil {
		b.t.Fatal(err)
	}
	var id uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT messaggio_id FROM messaggio WHERE chiave_esterna = $1`, m.MessageID).Scan(&id); err != nil {
		b.t.Fatalf("messaggio %s non registrato: %v", m.MessageID, err)
	}
	return id
}

// mail costruisce un MessaggioIn con id unici; i campi si ritoccano prima di `postaMsg`.
func (b *bancoWeb) mail(direzione, da, oggetto, corpo string, a ...string) worker.MessaggioIn {
	nPosta++
	quando := time.Now().Add(-time.Duration(nPosta) * time.Minute)
	m := worker.MessaggioIn{
		MessageID: fmt.Sprintf("<b7b-%d@prova.example>", nPosta), EntryID: fmt.Sprintf("ENTRY-B7B-%d", nPosta),
		StoreID: "STORE-B7", ConversationID: fmt.Sprintf("CONV-B7B-%d", nPosta), Cartella: "Posta in arrivo",
		Direzione: direzione, DataEvento: quando, RicevutoIl: &quando, MittenteNome: "Ufficio",
		MittenteIndirizzo: da, Oggetto: oggetto, CorpoTesto: corpo, Riferimenti: []string{}, Categorie: []string{},
	}
	if direzione == "uscita" {
		m.Cartella = "Sent Items"
	}
	for _, d := range a {
		m.Destinatari = append(m.Destinatari, worker.Destinatario{Indirizzo: d, Tipo: "a"})
	}
	if m.Destinatari == nil {
		m.Destinatari = []worker.Destinatario{{Indirizzo: "commerciale@azienda.example", Tipo: "a"}}
	}
	return m
}

// rfqDa crea una RFQ del cliente con quel messaggio dentro e l'identificativo dato.
func (b *bancoWeb) rfqDa(msg uuid.UUID, cliente uuid.UUID, cartella, codice string) uuid.UUID {
	b.t.Helper()
	var thread uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa)
		VALUES ($1, 'outlook', now(), $2, $3) RETURNING thread_id`, cliente, "RFQ "+codice, cartella).Scan(&thread); err != nil {
		b.t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO identificativo_thread (thread_id, codice, origine) VALUES ($1, $2, 'manuale')`, thread, codice); err != nil {
		b.t.Fatal(err)
	}
	if msg != uuid.Nil {
		if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET thread_id = $2, aggancio = 'operatore' WHERE messaggio_id = $1`, msg, thread); err != nil {
			b.t.Fatal(err)
		}
	}
	return thread
}

func (b *bancoWeb) richiesta(id uuid.UUID) db.RichiestaFornitore {
	b.t.Helper()
	r, err := b.q.GetRichiesta(b.ctx, id)
	if err != nil {
		b.t.Fatal(err)
	}
	return r
}

func (b *bancoWeb) propostaDi(id uuid.UUID) db.PropostaTriage {
	b.t.Helper()
	p, err := b.q.GetTriageMessaggio(b.ctx, id)
	if err != nil {
		b.t.Fatalf("nessuna proposta per %s: %v", id, err)
	}
	return p
}

// IB2 — richiesta creata dal Cockpit: nasce la bozza con il marcatore, e quando la mail compare
// nella Posta inviata con quel marcatore la richiesta prende la sua mail (inviata) e la mail entra
// nella RFQ. Nessuna euristica sull'oggetto.
func TestIB2LaRichiestaCreataDalCockpitSiLegaDalMarcatore(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, coda.Capacita{Bozze: true})
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	euro := b.unFornitore("Fresature Esempio", db.TipoFornitoreVerniciatore, "fresature-esempio.example", "cataforesi")
	if _, err := b.q.InsertContattoFornitore(b.ctx, db.InsertContattoFornitoreParams{FornitoreID: euro.FornitoreID, Lower: "ordini@fresature-esempio.example"}); err != nil {
		t.Fatal(err)
	}
	mid := b.posta("entrata", "acquisti@acme.example", "RFQ 1234567A", true)
	thread := b.rfqDa(mid, acme, `ACME\WIP\2026 09 18 Rossi RFQ 1234567A`, "1234567A")
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)
	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")

	resp, corpo := fp.fai(http.MethodPost, "/thread/"+thread.String()+"/richiesta", url.Values{
		"fornitore_id": {euro.FornitoreID.String()}, "lavorazione": {"cataforesi"}, "codice": {"1234567A"}, "note": {"10 pezzi"}, "bozza": {"1"}}, true)
	if resp.StatusCode != 200 {
		t.Fatalf("stato %d", resp.StatusCode)
	}
	// Lo stato «bozza» non si legge piu' da una chip della pagina (il box non c'e' piu', correzione
	// prima di B8.2): lo dice la riga di richiesta_fornitore qui sotto.
	for _, atteso := range []string{"Richiesta a Fresature Esempio creata", "Bozza in preparazione", "ordini@fresature-esempio.example"} {
		if !strings.Contains(corpo, atteso) {
			t.Errorf("la risposta non dice %q: %s", atteso, estratto(corpo, "avviso"))
		}
	}
	var rid uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT richiesta_id FROM richiesta_fornitore WHERE thread_id = $1`, thread).Scan(&rid); err != nil {
		t.Fatal("richiesta non creata:", err)
	}
	ric := b.richiesta(rid)
	if ric.Stato != db.StatoRichiestaFornitoreBozza || ric.Lavorazione.String != "cataforesi" || len(ric.Codici) != 1 || ric.Codici[0] != "1234567A" {
		t.Errorf("richiesta: %+v", ric)
	}
	var bid uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT bozza_id FROM bozza WHERE richiesta_fornitore_id = $1`, rid).Scan(&bid); err != nil {
		t.Fatal("bozza non creata:", err)
	}
	jobs := b.jobDiTipo(db.TipoJobCreaBozzaOutlook)
	if len(jobs) != 1 {
		t.Fatalf("job crea_bozza_outlook: %d, atteso 1", len(jobs))
	}
	var p worker.PayloadCreaBozza
	if err := json.Unmarshal(jobs[0].Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p.Tipo != "nuovo" || p.Marcatori[ingest.MarcatoreRichiesta] != rid.String() || !strings.HasPrefix(p.Oggetto, "RFQ ACME") || !strings.Contains(p.Oggetto, "1234567A") {
		t.Errorf("payload della bozza: tipo %s, marcatori %v, oggetto %q", p.Tipo, p.Marcatori, p.Oggetto)
	}
	if len(p.Destinatari) != 1 || p.Destinatari[0].Indirizzo != "ordini@fresature-esempio.example" {
		t.Errorf("destinatari: %+v", p.Destinatari)
	}

	// il sync della Posta inviata: la mail arriva con i marcatori scritti dal worker
	m := b.mail("uscita", "commerciale@azienda.example", p.Oggetto, p.CorpoTesto, "ordini@fresature-esempio.example")
	m.Marcatori = map[string]string{ingest.MarcatoreRichiesta: rid.String(), ingest.MarcatoreBozza: bid.String()}
	sent := b.postaMsg(m)
	ric = b.richiesta(rid)
	if ric.Stato != db.StatoRichiestaFornitoreInviata || !ric.MessaggioID.Valid || ric.MessaggioID.UUID != sent || ric.InviataIl == nil {
		t.Errorf("dopo il sync la richiesta e' inviata con la sua mail: %+v", ric)
	}
	var tid, ridMsg uuid.NullUUID
	var aggancio string
	if err := b.pool.QueryRow(b.ctx, `SELECT thread_id, richiesta_fornitore_id, aggancio::text FROM messaggio WHERE messaggio_id = $1`, sent).Scan(&tid, &ridMsg, &aggancio); err != nil {
		t.Fatal(err)
	}
	if !tid.Valid || tid.UUID != thread || !ridMsg.Valid || ridMsg.UUID != rid {
		t.Errorf("la mail inviata non e' entrata nella RFQ con la sua richiesta: thread %v, richiesta %v", tid, ridMsg)
	}
	var statoBozza string
	var inviataMsg uuid.NullUUID
	if err := b.pool.QueryRow(b.ctx, `SELECT stato::text, inviata_messaggio_id FROM bozza WHERE bozza_id = $1`, bid).Scan(&statoBozza, &inviataMsg); err != nil {
		t.Fatal(err)
	}
	if statoBozza != "inviata" || !inviataMsg.Valid || inviataMsg.UUID != sent {
		t.Errorf("la bozza non risulta partita: %s %v", statoBozza, inviataMsg)
	}
	// un secondo sync della stessa mail non cambia niente (idempotente)
	b.postaMsg(m)
	if r2 := b.richiesta(rid); r2.Stato != db.StatoRichiestaFornitoreInviata || r2.MessaggioID.UUID != sent {
		t.Errorf("il secondo sync ha toccato la richiesta: %+v", r2)
	}
	// La pagina della RFQ mostra la mail partita fra i suoi messaggi, ma non ha piu' il box delle
	// richieste ai fornitori (correzione prima di B8.2: torneranno nel tab Luigi, per lavorazione
	// di un componente). La richiesta resta nel database, come si e' appena verificato.
	_, pagina := fp.fai(http.MethodGet, "/thread/"+thread.String(), nil, false)
	if !strings.Contains(pagina, "/messaggio/"+sent.String()) {
		t.Error("la pagina della RFQ non mostra la mail inviata fra i suoi messaggi")
	}
	for _, vietato := range []string{"Richieste ai fornitori", "Nuova richiesta a un fornitore", `class="chip richiesta`} {
		if strings.Contains(pagina, vietato) {
			t.Errorf("la pagina della RFQ mostra ancora %q", vietato)
		}
	}
	if testutil.Conta(t, b.pool, "thread_offerta") != 1 {
		t.Error("e' nato un thread in piu'")
	}
}

// Senza la capacita' `bozze` la richiesta nasce lo stesso e la frase dice perche' la bozza no.
func TestLaRichiestaNasceAncheSenzaLaBozza(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, coda.Capacita{})
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	euro := b.unFornitore("Fresature Esempio", db.TipoFornitoreVerniciatore, "fresature-esempio.example", "cataforesi")
	mid := b.posta("entrata", "acquisti@acme.example", "RFQ 1234567A", true)
	thread := b.rfqDa(mid, acme, `ACME\WIP\x`, "1234567A")
	b.workerClaim("outlook@PC-FRANCESCO", "10.0.0.5:4000", b.francesco, b.commerciale)
	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")
	_, corpo := fp.fai(http.MethodPost, "/thread/"+thread.String()+"/richiesta", url.Values{"fornitore_id": {euro.FornitoreID.String()}, "bozza": {"1"}}, true)
	if !strings.Contains(corpo, "Richiesta a Fresature Esempio creata") || !strings.Contains(corpo, "Bozza non preparata") || !strings.Contains(corpo, "bozze") {
		t.Errorf("la risposta: %s", estratto(corpo, "avviso"))
	}
	if testutil.Conta(t, b.pool, "richiesta_fornitore") != 1 || testutil.Conta(t, b.pool, "bozza") != 0 {
		t.Errorf("richieste %d (attesa 1), bozze %d (attese 0)", testutil.Conta(t, b.pool, "richiesta_fornitore"), testutil.Conta(t, b.pool, "bozza"))
	}
	// la stessa richiesta due volte: rifiutata con il nome
	_, corpo = fp.fai(http.MethodPost, "/thread/"+thread.String()+"/richiesta", url.Values{"fornitore_id": {euro.FornitoreID.String()}}, true)
	if !strings.Contains(corpo, "esiste già una richiesta a Fresature Esempio") {
		t.Errorf("il doppione: %s", estratto(corpo, "avviso"))
	}
	// annulla
	var rid uuid.UUID
	_ = b.pool.QueryRow(b.ctx, `SELECT richiesta_id FROM richiesta_fornitore LIMIT 1`).Scan(&rid)
	_, corpo = fp.fai(http.MethodPost, "/thread/"+thread.String()+"/richiesta/"+rid.String()+"/annulla", nil, true)
	if !strings.Contains(corpo, "Richiesta annullata") || b.richiesta(rid).Stato != db.StatoRichiestaFornitoreAnnullata {
		t.Errorf("annulla: %s", estratto(corpo, "avviso"))
	}
}

// IB3 + IB4 + IB5 — la richiesta mandata a mano si propone e si conferma senza thread nuovi; l'offerta
// del fornitore in risposta (In-Reply-To) e' un candidato R0 al 95 e alla conferma la richiesta passa a
// `risposta` con l'allegato proposto come offerta; senza In-Reply-To ma con il codice, due RFQ con
// quel codice danno due candidati R3f.
func TestIB3IB4IB5LaRichiestaAManoELOffertaDelFornitore(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("Beta Sport di prova", "BETA SPORT", "betasport.example")
	if _, err := b.q.SetRegoleCliente(b.ctx, db.SetRegoleClienteParams{ClienteID: acme,
		Regole: json.RawMessage(`{"famiglie_codice":[{"regex":"\\b0[A-Z]\\d{6}[A-Z]{2}\\b","descrizione":"codice TG","esempio":"0X001234AB"}]}`)}); err != nil {
		t.Fatal(err)
	}
	minuterie := b.unFornitore("Minuterie Esempio di prova", db.TipoFornitoreProcessi, "minuterie-esempio.example", "tornitura", "fresatura")
	thread := b.rfqDa(uuid.Nil, acme, `BETA SPORT\WIP\2026 09 18 Rossi RFQ 0X001234AB`, "0X001234AB")
	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")

	// IB3: la nostra mail a Minuterie Esempio, mandata a mano da Outlook
	sent := b.postaMsg(b.mail("uscita", "commerciale@azienda.example", "RFQ PROGETTO ALFA 0X001234AB", "Buongiorno, vi chiediamo offerta per il particolare 0X001234AB in allegato.", "info@minuterie-esempio.example"))
	p := b.propostaDi(sent)
	if p.Esito != db.EsitoTriageAggancia || p.Atto.String != "richiesta_offerta" || !p.ThreadProposto.Valid || p.ThreadProposto.UUID != thread ||
		!p.FornitoreProposto.Valid || p.FornitoreProposto.UUID != minuterie.FornitoreID {
		t.Fatalf("IB3: la proposta non e' «richiesta a Minuterie Esempio per la RFQ»: %+v", p)
	}
	if testutil.Conta(t, b.pool, "thread_offerta") != 1 {
		t.Fatal("IB3: e' nato un thread")
	}
	_, pannello := fp.fai(http.MethodGet, "/messaggio/"+sent.String(), nil, true)
	if !strings.Contains(pannello, "Richiesta mandata a mano?") || !strings.Contains(pannello, "Minuterie Esempio di prova") || !strings.Contains(pannello, "richiesta-fornitore") {
		t.Fatalf("IB3: il pannello non propone la richiesta: %s", estratto(pannello, "candidati-richiesta"))
	}
	_, esito := fp.fai(http.MethodPost, "/messaggio/"+sent.String()+"/richiesta-fornitore", url.Values{
		"thread_id": {thread.String()}, "fornitore_id": {minuterie.FornitoreID.String()}, "lavorazione": {"tornitura"}}, true)
	if !strings.Contains(esito, "Registrata come richiesta a Minuterie Esempio") {
		t.Fatalf("IB3: la conferma: %s", estratto(esito, "avviso"))
	}
	var rid uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT richiesta_id FROM richiesta_fornitore WHERE thread_id = $1 AND fornitore_id = $2`, thread, minuterie.FornitoreID).Scan(&rid); err != nil {
		t.Fatal("IB3: richiesta non creata:", err)
	}
	ric := b.richiesta(rid)
	if ric.Stato != db.StatoRichiestaFornitoreInviata || ric.MessaggioID.UUID != sent || ric.Lavorazione.String != "tornitura" || len(ric.Codici) == 0 || ric.Codici[0] != "0X001234AB" {
		t.Errorf("IB3: richiesta %+v", ric)
	}
	if tipo, _ := b.controparteDi(sent); tipo != "fornitore" {
		t.Errorf("la nostra mail ha controparte %s", tipo)
	}
	var tid uuid.NullUUID
	_ = b.pool.QueryRow(b.ctx, `SELECT thread_id FROM messaggio WHERE messaggio_id = $1`, sent).Scan(&tid)
	if !tid.Valid || tid.UUID != thread || testutil.Conta(t, b.pool, "thread_offerta") != 1 {
		t.Errorf("IB3: la mail non e' nella RFQ, o e' nato un thread")
	}

	// IB4: Minuterie Esempio risponde con In-Reply-To alla nostra mail
	var chiaveSent string
	_ = b.pool.QueryRow(b.ctx, `SELECT chiave_esterna FROM messaggio WHERE messaggio_id = $1`, sent).Scan(&chiaveSent)
	risposta := b.mail("entrata", "info@minuterie-esempio.example", "R: RFQ PROGETTO ALFA 0X001234AB", "Buongiorno, in allegato la nostra offerta. Materiale S235JR come da ISO 2768.")
	risposta.InReplyTo = chiaveSent
	risposta.Allegati = []worker.AllegatoIn{{Indice: 1, NomeFile: "offerta_MINUTERIE_123.pdf", Estensione: "pdf", Natura: "file", Bytes: 5000}}
	rispID := b.postaMsg(risposta)
	if tipo, _ := b.controparteDi(rispID); tipo != "fornitore" {
		t.Fatalf("IB4: controparte %s", tipo)
	}
	p = b.propostaDi(rispID)
	if p.Esito != db.EsitoTriageAggancia || p.Confidenza != 95 || !p.RichiestaProposta.Valid || p.RichiestaProposta.UUID != rid || p.Atto.String != "offerta" || p.Legame.LegameOperativo != db.LegameOperativoRisposta {
		t.Fatalf("IB4: la proposta: %+v", p)
	}
	_, pannello = fp.fai(http.MethodGet, "/messaggio/"+rispID.String(), nil, true)
	if !strings.Contains(pannello, "Risposta a una nostra richiesta?") || !strings.Contains(pannello, ">95<") || !strings.Contains(pannello, "R0_reply") || strings.Contains(pannello, "triage?azione=nuova") {
		t.Fatalf("IB4: il pannello: %s", estratto(pannello, "candidati-richiesta"))
	}
	// 7C.0, invariante 8: agganciata SENZA atto, la mail e' dentro e la richiesta resta aperta
	_, esito = fp.fai(http.MethodPost, "/messaggio/"+rispID.String()+"/risposta-fornitore", url.Values{"richiesta_id": {rid.String()}}, true)
	if !strings.Contains(esito, "Agganciata come risposta di Minuterie Esempio") || !strings.Contains(esito, "La richiesta resta aperta") || strings.Contains(esito, "allegato proposto") {
		t.Fatalf("IB4: la conferma senza atto: %s", estratto(esito, "avviso"))
	}
	if r := b.richiesta(rid); r.Stato != db.StatoRichiestaFornitoreInviata || r.OffertaRicevutaIl != nil {
		t.Errorf("IB4: una risposta collegata ha cambiato lo stato da sola: %+v", r)
	}
	var ridMsg uuid.NullUUID
	_ = b.pool.QueryRow(b.ctx, `SELECT richiesta_fornitore_id FROM messaggio WHERE messaggio_id = $1`, rispID).Scan(&ridMsg)
	if !ridMsg.Valid || ridMsg.UUID != rid {
		t.Errorf("IB4: la mail non e' legata alla richiesta: %v", ridMsg)
	}
	// poi l'operatore conferma l'ATTO: e' l'offerta → offerta ricevuta, allegati proposti
	_, esito = fp.fai(http.MethodPost, "/messaggio/"+rispID.String()+"/risposta-fornitore", url.Values{"richiesta_id": {rid.String()}, "atto": {"offerta"}}, true)
	if !strings.Contains(esito, "Offerta ricevuta") || !strings.Contains(esito, "1 allegato proposto come offerta del fornitore") {
		t.Fatalf("IB4: la conferma dell'offerta: %s", estratto(esito, "avviso"))
	}
	if r := b.richiesta(rid); r.Stato != db.StatoRichiestaFornitoreOffertaRicevuta || r.OffertaRicevutaIl == nil {
		t.Errorf("IB4: la richiesta non e' passata a offerta_ricevuta: %+v", r)
	}
	var tipoProposto string
	_ = b.pool.QueryRow(b.ctx, `SELECT p.tipo_proposto::text FROM documento_proposta p JOIN allegato a USING (allegato_id) WHERE a.messaggio_id = $1`, rispID).Scan(&tipoProposto)
	if tipoProposto != "offerta_fornitore" {
		t.Errorf("IB4: l'allegato e' proposto come %q, atteso offerta_fornitore", tipoProposto)
	}
	_ = b.pool.QueryRow(b.ctx, `SELECT thread_id FROM messaggio WHERE messaggio_id = $1`, rispID).Scan(&tid)
	if !tid.Valid || tid.UUID != thread {
		t.Errorf("IB4: l'offerta non e' nella RFQ cliente")
	}
	// IB8: nella mail del fornitore S235JR e ISO 2768 non sono codici; il codice del cliente si'
	var nCodici int
	_ = b.pool.QueryRow(b.ctx, `SELECT count(*) FROM candidato_codice WHERE messaggio_id = $1 AND upper(codice) IN ('S235JR','2768','ISO 2768')`, rispID).Scan(&nCodici)
	if nCodici != 0 {
		t.Errorf("IB8: materiali o norme registrati come codici: %d", nCodici)
	}
	if len(p.Identificativi) != 1 || p.Identificativi[0] != "0X001234AB" {
		t.Errorf("IB8: identificativi proposti %v, atteso il solo codice del cliente", p.Identificativi)
	}

	// IB5: una seconda RFQ con lo stesso codice e una richiesta a Minuterie Esempio; un'offerta senza In-Reply-To
	thread2 := b.rfqDa(uuid.Nil, acme, `BETA SPORT\WIP\2026 09 18 Rossi RFQ bis`, "0X001234AB")
	if _, err := b.q.InsertRichiestaFornitore(b.ctx, db.InsertRichiestaFornitoreParams{ThreadID: thread2, FornitoreID: minuterie.FornitoreID,
		Codici: []string{"0X001234AB"}, Stato: db.StatoRichiestaFornitoreInviata}); err != nil {
		t.Fatal(err)
	}
	altra := b.postaMsg(b.mail("entrata", "vendite@minuterie-esempio.example", "Offerta 0X001234AB", "Vi inviamo la quotazione per il codice 0X001234AB."))
	cand, _ := b.q.ListCandidatiRichiesta(b.ctx, altra)
	if len(cand) != 2 {
		t.Fatalf("IB5: candidati %d, attesi 2 (una per RFQ con quel codice): %+v", len(cand), cand)
	}
	for _, c := range cand {
		if c.Regola != db.RegolaRichiestaR3fCodice || c.Punteggio != 60 {
			t.Errorf("IB5: candidato %+v", c)
		}
	}
	_, pannello = fp.fai(http.MethodGet, "/messaggio/"+altra.String(), nil, true)
	if strings.Count(pannello, "È la risposta a questa richiesta") != 2 {
		t.Errorf("IB5: il pannello deve mostrare tutti e due i candidati: %s", estratto(pannello, "candidati-richiesta"))
	}
}

// IB6 — «Ignora» su un messaggio Da validare: esce dal quadrante e non torna con i sync.
func TestIB6IgnoraInDaValidareNonTornaConISync(t *testing.T) {
	b := preparaBancoWeb(t)
	m := b.mail("entrata", "nessuno@altrove.example", "Cose varie IB6", "ciao")
	id := b.postaMsg(m)
	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")
	_, lista := fp.fai(http.MethodGet, "/inbox?q=validare&filtro=orfani", nil, true)
	if !strings.Contains(lista, "Cose varie IB6") {
		t.Fatal("prima di «Ignora» il messaggio sta in Da validare")
	}
	if _, esito := fp.fai(http.MethodPost, "/messaggio/"+id.String()+"/ignora", nil, true); !strings.Contains(esito, "ignorato") {
		t.Fatalf("ignora: %s", estratto(esito, "avviso"))
	}
	_, lista = fp.fai(http.MethodGet, "/inbox?q=validare&filtro=orfani", nil, true)
	if strings.Contains(lista, "Cose varie IB6") {
		t.Fatal("dopo «Ignora» il messaggio e' ancora fra gli orfani")
	}
	b.postaMsg(m) // il sync la rilegge (stesso Message-ID)
	_, lista = fp.fai(http.MethodGet, "/inbox?q=validare&filtro=orfani", nil, true)
	if strings.Contains(lista, "Cose varie IB6") {
		t.Fatal("il sync ha riportato fra gli orfani un messaggio ignorato")
	}
	_, lista = fp.fai(http.MethodGet, "/inbox?q=validare&filtro=ignorati", nil, true)
	if !strings.Contains(lista, "Cose varie IB6") {
		var stati []string
		righe, _ := b.pool.Query(b.ctx, `SELECT stato::text || '/' || fonte::text || '/' || esito::text FROM proposta_triage WHERE messaggio_id = $1`, id)
		for righe.Next() {
			var s string
			_ = righe.Scan(&s)
			stati = append(stati, s)
		}
		righe.Close()
		var ign bool
		var ctipo string
		var tid uuid.NullUUID
		_ = b.pool.QueryRow(b.ctx, `SELECT ignorato, controparte_tipo, thread_id FROM v_inbox WHERE messaggio_id = $1`, id).Scan(&ign, &ctipo, &tid)
		t.Fatalf("il messaggio ignorato deve stare fra gli ignorati: proposte %v, v_inbox ignorato=%v controparte=%s thread=%v -- %s", stati, ign, ctipo, tid, primi400(lista))
	}
}

// IB7 — una newsletter da un dominio sconosciuto con «RFQ» nel testo: non_rfq, Da validare, nessuna
// proposta di RFQ. E la notifica automatica di un cliente censito va in Da validare, non in Buyer.
// IB7 riletto con il 7C.0: la newsletter di uno sconosciuto e' non_business e sta in Da validare
// (e' uno sconosciuto); la notifica automatica dal dominio di un CLIENTE e' non_business e sta fra i
// Clienti, perche' il quadrante dipende solo dalla controparte. Per toglierla da li' si censisce
// l'indirizzo come Altro (7C.0, decisione del 18/09/2026); fino alla 0015 andava in Da validare.
func TestIB7LaNewsletterENonBusinessEIlQuadranteLoDiceLaControparte(t *testing.T) {
	b := preparaBancoWeb(t)
	b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	news := b.postaMsg(b.mail("entrata", "newsletter@promo.example", "RFQ in un clic con il nostro portale!",
		"Richiesta d'offerta automatica per tutti i tuoi fornitori. Iscriviti al webinar. Unsubscribe qui."))
	p := b.propostaDi(news)
	if p.Esito == db.EsitoTriageNuovaRfq || p.Atto.String != "non_business" {
		t.Fatalf("IB7: %+v", p)
	}
	notifica := b.postaMsg(b.mail("entrata", "noreply@acme.example", "Portale: nuova notifica", "Do not reply to this message."))
	if tipo, _ := b.controparteDi(notifica); tipo != "cliente" {
		t.Fatalf("la notifica viene da un dominio cliente: %s", tipo)
	}
	if p := b.propostaDi(notifica); p.Atto.String != "non_business" || p.Esito == db.EsitoTriageNuovaRfq {
		t.Fatalf("la notifica automatica del cliente: %+v", p)
	}
	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")
	_, validare := fp.fai(http.MethodGet, "/inbox?q=validare&filtro=orfani", nil, true)
	_, clienti := fp.fai(http.MethodGet, "/inbox?q=clienti&filtro=orfani", nil, true)
	if !strings.Contains(validare, "RFQ in un clic") || strings.Contains(validare, "Portale: nuova notifica") {
		t.Errorf("Da validare deve avere lo sconosciuto e non la notifica del cliente: %s", primi400(validare))
	}
	if !strings.Contains(clienti, "Portale: nuova notifica") || !strings.Contains(clienti, `class="chip atto non_business"`) || strings.Contains(clienti, "RFQ in un clic") {
		t.Errorf("Clienti deve avere la notifica del cliente con il chip non_business, e non lo sconosciuto: %s", primi400(clienti))
	}
	_, pagina := fp.fai(http.MethodGet, "/inbox", nil, false)
	if !strings.Contains(pagina, `class="quadrante validare `) || !strings.Contains(estratto(pagina, `class="quadrante validare `), ">1<") {
		t.Errorf("il conteggio di Da validare deve essere 1: %s", estratto(pagina, "quadrante validare"))
	}
}
