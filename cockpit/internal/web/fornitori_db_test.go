//go:build integrazione

// L4 — blocco 7A sulla porta HTTP vera: l'Inbox a quadranti (IB1), «Censisci come fornitore» con il
// ritriage mirato (CP5), la scheda Admin › Anagrafica › Fornitori con le scritture non distruttive,
// le convenzioni di codice dalla schermata (CP9, CP10, CP12) e l'import del seme con anteprima e
// conferma (CP14). Nomi e domini sono inventati (.example).
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags integrazione -p 1 ./internal/web/
//
// Che cosa NON provano: che un browser mostri i quadranti e i pulsanti. Quello è L7, e senza
// Playwright resta NON ESEGUITO in esiti_reali.md.
package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/ingest"
	"promatec/cockpit/internal/testutil"
)

var nPosta int

// posta fa arrivare (o partire) un messaggio per la strada vera, `ingest.Ingerisci`, sulla casella
// Commerciale. Restituisce l'id del messaggio.
func (b *bancoWeb) posta(direzione, da, oggetto string, cad bool, a ...string) uuid.UUID {
	b.t.Helper()
	nPosta++
	casella, err := b.q.GetCasellaPerIndirizzo(b.ctx, db.GetCasellaPerIndirizzoParams{Canale: db.CanaleOutlook, Indirizzo: "commerciale@azienda.example"})
	if err != nil {
		b.t.Fatal(err)
	}
	quando := time.Now().Add(-time.Duration(nPosta) * time.Minute)
	m := api.MessaggioIn{
		MessageID: fmt.Sprintf("<b7-%d@prova.example>", nPosta), EntryID: fmt.Sprintf("ENTRY-B7-%d", nPosta),
		StoreID: "STORE-B7", ConversationID: fmt.Sprintf("CONV-B7-%d", nPosta), Cartella: "Posta in arrivo",
		Direzione: direzione, DataEvento: quando, RicevutoIl: &quando, MittenteNome: "Ufficio",
		MittenteIndirizzo: da, Oggetto: oggetto,
		CorpoTesto:  "Buongiorno, richiesta d'offerta per i particolari in allegato. Quotazione urgente.",
		Riferimenti: []string{}, Categorie: []string{},
	}
	if direzione == "uscita" {
		m.Cartella = "Sent Items"
	}
	if cad {
		m.Allegati = []api.AllegatoIn{{Indice: 1, NomeFile: "RDO.pdf", Estensione: "pdf", Natura: "file", Bytes: 1000},
			{Indice: 2, NomeFile: "particolare.stp", Estensione: "stp", Natura: "file", Bytes: 2000}}
	}
	if len(a) == 0 {
		a = []string{"commerciale@azienda.example"}
	}
	for _, d := range a {
		m.Destinatari = append(m.Destinatari, api.Destinatario{Indirizzo: d, Tipo: "a"})
	}
	s := &ingest.Servizio{Pool: b.pool, Log: testutil.LogSilenzioso()}
	if _, err := s.Ingerisci(b.ctx, ingest.Lotto{Casella: casella, Messaggi: []api.MessaggioIn{m}}); err != nil {
		b.t.Fatal(err)
	}
	var id uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT messaggio_id FROM messaggio WHERE chiave_esterna = $1`, m.MessageID).Scan(&id); err != nil {
		b.t.Fatalf("messaggio %s non registrato: %v", m.MessageID, err)
	}
	return id
}

func (b *bancoWeb) controparteDi(id uuid.UUID) (tipo, via string) {
	b.t.Helper()
	if err := b.pool.QueryRow(b.ctx, `SELECT controparte_tipo::text, COALESCE(controparte_via::text, '') FROM messaggio WHERE messaggio_id = $1`, id).Scan(&tipo, &via); err != nil {
		b.t.Fatal(err)
	}
	return
}

func (b *bancoWeb) unFornitore(nome string, tipo db.TipoFornitore, dominio string, lavorazioni ...string) db.Fornitore {
	b.t.Helper()
	f, err := b.q.InsertFornitore(b.ctx, db.InsertFornitoreParams{RagioneSociale: nome, Tipo: tipo})
	if err != nil {
		b.t.Fatal(err)
	}
	if dominio != "" {
		if err := AggiungiDominioFornitore(b.ctx, b.q, dominio, f.FornitoreID); err != nil {
			b.t.Fatal(err)
		}
	}
	for _, l := range lavorazioni {
		if _, err := b.q.InsertLavorazioneFornitore(b.ctx, db.InsertLavorazioneFornitoreParams{FornitoreID: f.FornitoreID, Lavorazione: l}); err != nil {
			b.t.Fatal(err)
		}
	}
	return f
}

// IB1 — quattro messaggi: cliente↓, cliente↑, fornitore↓, sconosciuto. Ciascuno sta nel suo
// quadrante e in nessun altro; la direzione è un filtro dentro il quadrante; il pannello di un
// fornitore non ha «Nuova RFQ».
func TestIB1OgniMessaggioStaNelSuoQuadrante(t *testing.T) {
	b := preparaBancoWeb(t)
	b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	b.unFornitore("Euroforesi", db.TipoFornitoreVerniciatore, "euroforesi.example", "cataforesi")

	clienteIn := b.posta("entrata", "acquisti@acme.example", "RFQ IB1 cliente in entrata", true)
	clienteOut := b.posta("uscita", "commerciale@azienda.example", "Offerta IB1 cliente in uscita", false, "acquisti@acme.example")
	fornitoreIn := b.posta("entrata", "info@euroforesi.example", "Offerta IB1 fornitore", true)
	ignoto := b.posta("entrata", "nessuno@altrove.example", "Newsletter IB1 sconosciuto", false)

	for id, atteso := range map[uuid.UUID]string{clienteIn: "cliente", clienteOut: "cliente", fornitoreIn: "fornitore", ignoto: "sconosciuto"} {
		if tipo, _ := b.controparteDi(id); tipo != atteso {
			t.Fatalf("messaggio %s: controparte %s, attesa %s", id, tipo, atteso)
		}
	}

	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")
	lista := func(q string) string {
		_, corpo := fp.fai(http.MethodGet, "/inbox?q="+q+"&filtro=tutti", nil, true)
		return corpo
	}
	casi := []struct {
		quadrante string
		dentro    []string
		fuori     []string
	}{
		{"buyer", []string{"RFQ IB1 cliente in entrata", "Offerta IB1 cliente in uscita"}, []string{"Offerta IB1 fornitore", "Newsletter IB1"}},
		{"fornitori", []string{"Offerta IB1 fornitore", "chip fornitore"}, []string{"RFQ IB1 cliente", "Newsletter IB1"}},
		{"validare", []string{"Newsletter IB1", "sconosciuto"}, []string{"RFQ IB1 cliente", "Offerta IB1 fornitore"}},
		{"tutti", []string{"RFQ IB1 cliente in entrata", "Offerta IB1 fornitore", "Newsletter IB1"}, nil},
		{"buyer&dir=entrata", []string{"RFQ IB1 cliente in entrata"}, []string{"Offerta IB1 cliente in uscita"}},
		{"buyer&dir=uscita", []string{"Offerta IB1 cliente in uscita", "in uscita"}, []string{"RFQ IB1 cliente in entrata"}},
	}
	for _, c := range casi {
		corpo := lista(c.quadrante)
		for _, d := range c.dentro {
			if !strings.Contains(corpo, d) {
				t.Errorf("quadrante %s: manca %q:\n%s", c.quadrante, d, primi400(corpo))
			}
		}
		for _, f := range c.fuori {
			if strings.Contains(corpo, f) {
				t.Errorf("quadrante %s: contiene %q che è di un altro quadrante", c.quadrante, f)
			}
		}
	}

	// la pagina intera ha i tre quadranti con i conteggi degli orfani: 2, 1, 1
	_, pagina := fp.fai(http.MethodGet, "/inbox", nil, false)
	for _, atteso := range []string{`class="quadrante buyer attivo"`, `class="quadrante fornitori `, `class="quadrante validare `, `<small class="pieno">2</small>`, `<small class="pieno">1</small>`} {
		if !strings.Contains(pagina, atteso) {
			t.Errorf("la pagina non ha %q: %s", atteso, estratto(pagina, "quadranti"))
		}
	}

	// il pannello del fornitore: si aggancia, non apre una RFQ cliente
	_, pannello := fp.fai(http.MethodGet, "/messaggio/"+fornitoreIn.String(), nil, true)
	if strings.Contains(pannello, "triage?azione=nuova") {
		t.Errorf("il pannello di un fornitore offre «Nuova RFQ»: %s", estratto(pannello, "azioni"))
	}
	if !strings.Contains(pannello, "triage?azione=aggancia") || !strings.Contains(pannello, "fornitore · Euroforesi") {
		t.Errorf("il pannello di un fornitore deve dire chi è e offrire l'aggancio: %s", estratto(pannello, "azioni"))
	}
	// e quello dello sconosciuto offre il censimento
	_, pannello = fp.fai(http.MethodGet, "/messaggio/"+ignoto.String(), nil, true)
	if !strings.Contains(pannello, "censisci?come=fornitore") || !strings.Contains(pannello, "censisci?come=cliente") {
		t.Errorf("il pannello di uno sconosciuto non offre «Censisci»: %s", estratto(pannello, "azioni"))
	}
	// il link diretto a un messaggio riapre l'Inbox nel quadrante giusto
	resp, _ := fp.fai(http.MethodGet, "/messaggio/"+fornitoreIn.String(), nil, false)
	if resp.StatusCode != 302 || !strings.Contains(resp.Header.Get("Location"), "q=fornitori") {
		t.Errorf("il link diretto non riapre il quadrante Fornitori: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// CP5 — «Censisci come fornitore» con tre orfani dello stesso dominio, uno già deciso: i due non
// decisi vengono ricalcolati (controparte fornitore, niente `nuova_rfq`), il deciso resta intatto,
// e la risposta dice i conteggi. Lo fa l'operatore, non l'amministratore.
func TestCP5CensisciComeFornitoreRicalcolaSoloINonDecisi(t *testing.T) {
	b := preparaBancoWeb(t)
	altro := b.unCliente("Altro S.p.A.", "ALTRO", "altro.example")
	uno := b.posta("entrata", "info@pftorniture.example", "RICHIESTA D'OFFERTA n. 1 - TG FIORE", true)
	due := b.posta("entrata", "ordini@pftorniture.example", "RICHIESTA D'OFFERTA n. 2 - TG FIORE", true)
	deciso := b.posta("entrata", "x@pftorniture.example", "RICHIESTA D'OFFERTA n. 3 - TG FIORE", true)

	// il terzo è già deciso: sta in una RFQ (di un altro cliente, per assurdo che sia: è una decisione presa)
	var thread uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto) VALUES ($1, 'outlook', now(), 'decisa') RETURNING thread_id`, altro).Scan(&thread); err != nil {
		t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET thread_id = $2, aggancio = 'operatore' WHERE messaggio_id = $1`, deciso, thread); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{uno, due, deciso} {
		if tipo, _ := b.controparteDi(id); tipo != "sconosciuto" {
			t.Fatalf("prima del censimento la controparte deve essere sconosciuta, non %s", tipo)
		}
	}
	var proposte int
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FROM proposta_triage WHERE messaggio_id IN ($1,$2) AND esito = 'nuova_rfq' AND stato = 'proposta'`, uno, due).Scan(&proposte); err != nil {
		t.Fatal(err)
	}
	if proposte != 2 {
		t.Fatalf("da sconosciuti con PDF e STP il triage doveva proporre nuova_rfq a tutti e due, non a %d", proposte)
	}

	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")
	resp, form := fp.fai(http.MethodGet, "/messaggio/"+uno.String()+"/censisci?come=fornitore", nil, true)
	if resp.StatusCode != 200 || !strings.Contains(form, "Censisci come fornitore") || !strings.Contains(form, "pftorniture.example") {
		t.Fatalf("il form di censimento non è arrivato: %d %s", resp.StatusCode, primi400(form))
	}
	resp, esito := fp.fai(http.MethodPost, "/messaggio/"+uno.String()+"/censisci", url.Values{
		"come": {"fornitore"}, "ragione_sociale": {"PF Torniture"}, "tipo": {"processi"}, "lingua": {"it"},
		"usa_dominio": {"1"}, "usa_contatto": {"1"}, "contatto_nome": {"Ufficio Commerciale"},
		"lavorazione": {"tornitura", "fresatura"}}, true)
	if resp.StatusCode != 200 {
		t.Fatalf("censimento: stato %d", resp.StatusCode)
	}
	for _, atteso := range []string{"Fornitore PF Torniture censito", "2 messaggi ricalcolati", "2 controparti cambiate", "fornitore · PF Torniture"} {
		if !strings.Contains(esito, atteso) {
			t.Errorf("la risposta non dice %q: %s", atteso, estratto(esito, "avviso"))
		}
	}

	for _, id := range []uuid.UUID{uno, due} {
		tipo, via := b.controparteDi(id)
		if tipo != "fornitore" {
			t.Errorf("messaggio non deciso %s: controparte %s, attesa fornitore", id, tipo)
		}
		if id == uno && via != "contatto" {
			t.Errorf("info@ è censito come contatto esatto: via %q, attesa contatto", via)
		}
		var nuove int
		if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FROM proposta_triage WHERE messaggio_id = $1 AND esito = 'nuova_rfq' AND stato = 'proposta'`, id).Scan(&nuove); err != nil {
			t.Fatal(err)
		}
		if nuove != 0 {
			t.Errorf("messaggio %s: dopo il censimento propone ancora nuova_rfq — un fornitore non apre una RFQ cliente", id)
		}
	}
	if tipo, _ := b.controparteDi(deciso); tipo != "sconosciuto" {
		t.Errorf("il messaggio già deciso è stato toccato: controparte %s", tipo)
	}
	var threadDopo uuid.NullUUID
	if err := b.pool.QueryRow(b.ctx, `SELECT thread_id FROM messaggio WHERE messaggio_id = $1`, deciso).Scan(&threadDopo); err != nil {
		t.Fatal(err)
	}
	if !threadDopo.Valid || threadDopo.UUID != thread {
		t.Errorf("il messaggio deciso ha perso la sua RFQ")
	}

	// in anagrafica c'è UN fornitore con dominio, contatto e due lavorazioni
	f, err := b.q.GetFornitorePerRagioneSociale(b.ctx, "pf torniture")
	if err != nil {
		t.Fatal("il fornitore non è in anagrafica:", err)
	}
	if n := testutil.Conta(t, b.pool, "fornitore"); n != 1 {
		t.Errorf("fornitori in anagrafica: %d, atteso 1", n)
	}
	dom, _ := b.q.ListDominiFornitore(b.ctx, f.FornitoreID)
	con, _ := b.q.ListContattiFornitore(b.ctx, f.FornitoreID)
	lav, _ := b.q.ListLavorazioniFornitore(b.ctx, f.FornitoreID)
	if len(dom) != 1 || len(con) != 1 || len(lav) != 2 {
		t.Errorf("fornitore censito con %d domini, %d contatti, %d lavorazioni: attesi 1, 1, 2", len(dom), len(con), len(lav))
	}

	// censire di nuovo con la stessa ragione sociale aggiunge, non duplica
	tre := b.posta("entrata", "mario@gmail.example", "RICHIESTA D'OFFERTA n. 4", true)
	_, esito = fp.fai(http.MethodPost, "/messaggio/"+tre.String()+"/censisci", url.Values{
		"come": {"fornitore"}, "ragione_sociale": {"PF TORNITURE"}, "tipo": {"processi"}, "usa_contatto": {"1"}}, true)
	if !strings.Contains(esito, "già in anagrafica: aggiunti contatto mario@gmail.example") {
		t.Errorf("il secondo censimento doveva riusare il fornitore: %s", estratto(esito, "avviso"))
	}
	if n := testutil.Conta(t, b.pool, "fornitore"); n != 1 {
		t.Errorf("fornitori in anagrafica dopo il secondo censimento: %d, atteso 1", n)
	}
	// il Cockpit si rifiuta di censire un fornitore che nessun messaggio riconoscerebbe
	_, esito = fp.fai(http.MethodPost, "/messaggio/"+tre.String()+"/censisci", url.Values{
		"come": {"fornitore"}, "ragione_sociale": {"Nessuno"}, "tipo": {"processi"}}, true)
	if !strings.Contains(esito, "spunta almeno uno dei due") || testutil.Conta(t, b.pool, "fornitore") != 1 {
		t.Errorf("senza dominio né contatto il censimento doveva essere rifiutato: %s", estratto(esito, "errore-box"))
	}
}

// «Censisci come cliente» dallo stesso pannello: cliente, dominio, buyer, e il ricalcolo.
func TestCensisciComeClienteDalPannello(t *testing.T) {
	b := preparaBancoWeb(t)
	uno := b.posta("entrata", "acquisti@nuovocliente.example", "RFQ 12345 cliente nuovo", true)
	fp := b.browser("10.0.0.5:51000")
	fp.login("FP", "prova-fp")
	_, esito := fp.fai(http.MethodPost, "/messaggio/"+uno.String()+"/censisci", url.Values{
		"come": {"cliente"}, "ragione_sociale": {"Nuovo Cliente S.r.l."}, "cartella_nas": {"NUOVO CLIENTE"},
		"usa_dominio": {"1"}, "usa_contatto": {"1"}, "contatto_nome": {"Anna Bianchi"}}, true)
	for _, atteso := range []string{"Cliente Nuovo Cliente S.r.l. censito", "1 messaggi ricalcolati"} {
		if !strings.Contains(esito, atteso) {
			t.Errorf("la risposta non dice %q: %s", atteso, estratto(esito, "avviso"))
		}
	}
	if tipo, via := b.controparteDi(uno); tipo != "cliente" || via != "contatto" {
		t.Errorf("controparte %s/%s, attesa cliente/contatto", tipo, via)
	}
	c, err := b.q.GetClientePerCartella(b.ctx, "NUOVO CLIENTE")
	if err != nil {
		t.Fatal(err)
	}
	buyer, _ := b.q.ListBuyerCliente(b.ctx, c.ClienteID)
	if len(buyer) != 1 || buyer[0].Cognome != "Bianchi" {
		t.Errorf("buyer censito: %+v", buyer)
	}
	// una seconda volta con la stessa cartella: rifiutato, dice di chi è
	_, esito = fp.fai(http.MethodPost, "/messaggio/"+uno.String()+"/censisci", url.Values{
		"come": {"cliente"}, "ragione_sociale": {"Altro"}, "cartella_nas": {"NUOVO CLIENTE"}, "usa_dominio": {"1"}}, true)
	if !strings.Contains(esito, "già di Nuovo Cliente S.r.l.") {
		t.Errorf("la cartella presa doveva fermare il censimento: %s", estratto(esito, "errore-box"))
	}
}

// La scheda Admin › Anagrafica › Fornitori: creazione, dominio non distruttivo, lavorazioni,
// qualifica dalla scheda del cliente (CP12 in scrittura), capacità con qualifica sopra che non
// si toglie, e il 403 per chi non è amministratore.
func TestAdminFornitoriSchedaEScrittureNonDistruttive(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")

	resp, corpo := ad.fai(http.MethodPost, "/admin/fornitori",
		url.Values{"ragione_sociale": {"Galvar"}, "tipo": {"processi"}, "dominio": {"galvar.example"}}, false)
	if resp.StatusCode != 200 || !strings.Contains(corpo, "Fornitore creato.") {
		t.Fatalf("creazione: %d %s", resp.StatusCode, primi400(corpo))
	}
	galvar, err := b.q.GetFornitorePerRagioneSociale(b.ctx, "galvar")
	if err != nil {
		t.Fatal(err)
	}
	// lo stesso nome due volte: non ne nasce un secondo
	_, corpo = ad.fai(http.MethodPost, "/admin/fornitori", url.Values{"ragione_sociale": {"GALVAR"}, "tipo": {"processi"}}, false)
	if !strings.Contains(corpo, "esiste già") || testutil.Conta(t, b.pool, "fornitore") != 1 {
		t.Errorf("il doppione doveva essere rifiutato: %s", estratto(corpo, "errore-box"))
	}
	// un altro fornitore che prova a prendersi il dominio di Galvar
	_, corpo = ad.fai(http.MethodPost, "/admin/fornitori", url.Values{"ragione_sociale": {"Bonvini"}, "tipo": {"processi"}, "dominio": {"galvar.example"}}, false)
	if !strings.Contains(corpo, "già censito per il fornitore Galvar") {
		t.Errorf("il dominio preso doveva dire di chi è: %s", estratto(corpo, "errore-box"))
	}
	if f, err := b.q.GetFornitorePerDominio(b.ctx, "galvar.example"); err != nil || f.FornitoreID != galvar.FornitoreID {
		t.Errorf("il dominio è stato spostato via da Galvar")
	}

	// lavorazioni: caselle spuntate
	_, corpo = ad.fai(http.MethodPost, "/admin/fornitori/"+galvar.FornitoreID.String()+"/lavorazioni", url.Values{"lavorazione": {"zincatura", "lavaggio_zinco"}}, false)
	if !strings.Contains(corpo, "Lavorazioni salvate: 2 aggiunte, 0 tolte") {
		t.Errorf("lavorazioni: %s", estratto(corpo, "avviso"))
	}
	// qualifica dalla scheda del cliente: ok su una capacità, rifiutata su una che non ha
	_, corpo = ad.fai(http.MethodPost, "/admin/anagrafica/"+acme.String()+"/qualifica", url.Values{"fornitore_id": {galvar.FornitoreID.String()}, "lavorazione": {"zincatura"}}, false)
	if !strings.Contains(corpo, "Fornitore qualificato per questo cliente") {
		t.Errorf("qualifica: %s", estratto(corpo, "avviso"))
	}
	_, corpo = ad.fai(http.MethodPost, "/admin/anagrafica/"+acme.String()+"/qualifica", url.Values{"fornitore_id": {galvar.FornitoreID.String()}, "lavorazione": {"cataforesi"}}, false)
	if !strings.Contains(corpo, "non ha «cataforesi» fra le sue lavorazioni") {
		t.Errorf("la qualifica su una capacità assente doveva essere rifiutata con il motivo: %s", estratto(corpo, "errore-box"))
	}
	if n := testutil.Conta(t, b.pool, "cliente_fornitore_lavorazione"); n != 1 {
		t.Errorf("qualifiche in database: %d, attesa 1", n)
	}
	// togliere la zincatura mentre ACME lo ha qualificato: rifiutato, e le altre non si toccano
	_, corpo = ad.fai(http.MethodPost, "/admin/fornitori/"+galvar.FornitoreID.String()+"/lavorazioni", url.Values{"lavorazione": {"lavaggio_zinco"}}, false)
	if !strings.Contains(corpo, "non si toglie") {
		t.Errorf("la capacità con la qualifica sopra doveva restare: %s", estratto(corpo, "errore-box"))
	}
	if lav, _ := b.q.ListLavorazioniFornitore(b.ctx, galvar.FornitoreID); len(lav) != 2 {
		t.Errorf("lavorazioni dopo il rifiuto: %d, attese 2", len(lav))
	}
	// la scheda mostra la qualifica e la posta vuota
	_, pagina := ad.fai(http.MethodGet, "/admin/fornitori?fornitore="+galvar.FornitoreID.String()+"&sez=lavorazioni", nil, false)
	for _, atteso := range []string{"Galvar", "ACME", `value="zincatura" checked`, "Qualifiche per cliente"} {
		if !strings.Contains(pagina, atteso) {
			t.Errorf("la scheda non mostra %q", atteso)
		}
	}
	// l'operatore non entra
	fp := b.browser("10.0.0.5")
	fp.login("FP", "prova-fp")
	if resp, _ := fp.fai(http.MethodGet, "/admin/fornitori", nil, false); resp.StatusCode != 403 {
		t.Errorf("l'operatore ha aperto la scheda fornitori: %d", resp.StatusCode)
	}
	if resp, _ := fp.fai(http.MethodPost, "/admin/fornitori", url.Values{"ragione_sociale": {"X"}, "tipo": {"processi"}}, false); resp.StatusCode != 403 {
		t.Errorf("l'operatore ha creato un fornitore: %d", resp.StatusCode)
	}
}

// CP9 / CP10 / CP12 dalla schermata: una convenzione rifiutata in scrittura non entra; una riga
// rotta scritta a mano in database si vede con ✗ e non si usa; «Prova un codice» arriva ai soli
// fornitori qualificati per il cliente.
func TestConvenzioniDiCodiceDallaSchermata(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	beta := b.unCliente("Beta S.r.l.", "BETA", "beta.example")
	gamma := b.unCliente("Gamma S.r.l.", "GAMMA", "gamma.example")
	galvar := b.unFornitore("Galvar", db.TipoFornitoreProcessi, "galvar.example", "zincatura")
	bonvini := b.unFornitore("Bonvini", db.TipoFornitoreProcessi, "bonvini.example", "zincatura")
	// Galvar e' qualificato da ACME, Bonvini da BETA: tutti e due fanno la zincatura, e la qualifica
	// e' PER CLIENTE. Se la query dimenticasse il cliente, Bonvini comparirebbe anche per ACME.
	if _, err := b.q.InsertQualifica(b.ctx, db.InsertQualificaParams{ClienteID: acme, FornitoreID: galvar.FornitoreID, Lavorazione: "zincatura"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.q.InsertQualifica(b.ctx, db.InsertQualificaParams{ClienteID: beta, FornitoreID: bonvini.FornitoreID, Lavorazione: "zincatura"}); err != nil {
		t.Fatal(err)
	}
	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")
	base := "/admin/anagrafica/" + acme.String()

	// CP9: esempio che non corrisponde → rifiutata, niente in database
	_, corpo := ad.fai(http.MethodPost, base+"/convenzione", url.Values{"modo": {"suffisso"}, "espressione": {"-ZN"},
		"esempio": {"AB123"}, "descrizione": {"zincato"}, "lavorazione": {"zincatura"}}, false)
	if !strings.Contains(corpo, "convenzione rifiutata") || testutil.Conta(t, b.pool, "convenzione_codice") != 0 {
		t.Errorf("l'esempio che non corrisponde doveva fermare il salvataggio: %s", estratto(corpo, "errore-box"))
	}
	// CP9: senza lavorazioni → rifiutata
	_, corpo = ad.fai(http.MethodPost, base+"/convenzione", url.Values{"modo": {"suffisso"}, "espressione": {"-ZN"},
		"esempio": {"AB123-ZN"}, "descrizione": {"zincato"}}, false)
	if !strings.Contains(corpo, "convenzione rifiutata") || testutil.Conta(t, b.pool, "convenzione_codice") != 0 {
		t.Errorf("senza lavorazioni doveva essere rifiutata: %s", estratto(corpo, "errore-box"))
	}
	// quella buona entra
	_, corpo = ad.fai(http.MethodPost, base+"/convenzione", url.Values{"modo": {"suffisso"}, "espressione": {"-ZN"},
		"esempio": {"AB123-ZN"}, "controesempio": {"AB123-ZV"}, "descrizione": {"zincato"}, "lavorazione": {"zincatura"}}, false)
	if !strings.Contains(corpo, "Convenzione aggiunta") {
		t.Fatalf("la convenzione buona non è entrata: %s", estratto(corpo, "errore-box"))
	}
	// CP10: una riga rotta scritta a mano (regex che non compila in RE2)
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO convenzione_codice (cliente_id, modo, espressione, esempio, descrizione) VALUES ($1, 'regex', '([unclosed', 'X', 'rotta')`, acme); err != nil {
		t.Fatal(err)
	}
	_, pagina := ad.fai(http.MethodGet, "/admin/anagrafica?cliente="+acme.String()+"&sez=lavorazioni", nil, false)
	if !strings.Contains(pagina, `class="spunta si"`) || !strings.Contains(pagina, `class="spunta no"`) {
		t.Errorf("la scheda deve mostrare ✓ per la buona e ✗ per la rotta: %s", estratto(pagina, "convenzioni"))
	}
	// CP12: la prova arriva al solo fornitore qualificato, e la riga rotta non rompe niente
	_, corpo = ad.fai(http.MethodPost, base+"/lavorazioni/prova", url.Values{"codice": {"ab123-zn"}}, false)
	// si guarda l'ESITO della prova, non la pagina intera: Galvar compare anche nella tabella delle
	// qualifiche, e una prova che rispondesse «nessuno» passerebbe lo stesso
	if !strings.Contains(estratto(corpo, "esito-prova"), "<b>zincatura</b>") || !strings.Contains(estratto(corpo, "esito-prova"), "Galvar") {
		t.Errorf("la prova non trova la zincatura o Galvar: %s", estratto(corpo, "esito-prova"))
	}
	if strings.Contains(estratto(corpo, "esito-prova"), "Bonvini") {
		t.Errorf("Bonvini fa la zincatura ed è qualificato da BETA, non da ACME: non deve comparire")
	}
	_, corpo = ad.fai(http.MethodPost, base+"/lavorazioni/prova", url.Values{"codice": {"AB123-ZV"}}, false)
	if !strings.Contains(corpo, "non corrisponde a nessuna convenzione attiva") {
		t.Errorf("il controesempio non deve dare lavorazioni: %s", estratto(corpo, "esito-prova"))
	}
	// un cliente senza qualifiche: nessuno, non «tutti quelli che la fanno»
	_, _ = ad.fai(http.MethodPost, "/admin/anagrafica/"+gamma.String()+"/convenzione", url.Values{"modo": {"suffisso"}, "espressione": {"-ZN"},
		"esempio": {"G1-ZN"}, "descrizione": {"zincato"}, "lavorazione": {"zincatura"}}, false)
	_, corpo = ad.fai(http.MethodPost, "/admin/anagrafica/"+gamma.String()+"/lavorazioni/prova", url.Values{"codice": {"G1-ZN"}}, false)
	if !strings.Contains(estratto(corpo, "esito-prova"), "nessuno") || strings.Contains(estratto(corpo, "esito-prova"), "Galvar") || strings.Contains(estratto(corpo, "esito-prova"), "Bonvini") {
		t.Errorf("GAMMA non ha qualificato nessuno: %s", estratto(corpo, "esito-prova"))
	}
	// spegnere la convenzione la toglie dall'uso, non dal database
	var cid uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT convenzione_id FROM convenzione_codice WHERE cliente_id = $1 AND modo = 'suffisso'`, acme).Scan(&cid); err != nil {
		t.Fatal(err)
	}
	_, _ = ad.fai(http.MethodPost, base+"/convenzione/attiva", url.Values{"convenzione_id": {cid.String()}, "attiva": {"0"}}, false)
	_, corpo = ad.fai(http.MethodPost, base+"/lavorazioni/prova", url.Values{"codice": {"AB123-ZN"}}, false)
	if !strings.Contains(corpo, "non corrisponde a nessuna convenzione attiva") || testutil.Conta(t, b.pool, "convenzione_codice") != 3 {
		t.Errorf("la convenzione spenta non deve usarsi ma deve restare: %s", estratto(corpo, "esito-prova"))
	}
	// togliere la convenzione porta via le figlie (cascata)
	_, _ = ad.fai(http.MethodPost, base+"/convenzione/elimina", url.Values{"convenzione_id": {cid.String()}}, false)
	var figlie int
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FROM convenzione_codice_lavorazione WHERE convenzione_id = $1`, cid).Scan(&figlie); err != nil {
		t.Fatal(err)
	}
	if figlie != 0 || testutil.Conta(t, b.pool, "convenzione_codice") != 2 {
		t.Errorf("la cancellazione non ha portato via le figlie: %d", figlie)
	}
}

// CP14 — import del seme dalla schermata: l'anteprima non scrive; la conferma scrive solo ciò che
// l'anteprima ha mostrato; i nomi non risolti restano tali; il secondo import non scrive niente.
func TestCP14ImportDelSemeConAnteprimaEConferma(t *testing.T) {
	b := preparaBancoWeb(t)
	b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	seme := `{"fornitori": [
	  {"ragione_sociale": "Euroforesi", "tipo": "verniciatore", "domini": ["euroforesi.example"],
	   "contatti": [{"nome": "Ufficio", "email": "info@euroforesi.example"}],
	   "lavorazioni": ["cataforesi", "verniciatura_polvere"],
	   "qualifiche": [{"cliente": "ACME", "lavorazione": "cataforesi"}, {"cliente": "CLIENTE-IGNOTO", "lavorazione": "cataforesi"}]},
	  {"ragione_sociale": "Ideal System", "tipo": "verniciatore"}
	]}`
	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")

	_, corpo := ad.fai(http.MethodPost, "/admin/fornitori/importa", url.Values{"azione": {"anteprima"}, "testo": {seme}}, false)
	for _, atteso := range []string{"Anteprima: che cosa farebbe", "Euroforesi", "Ideal System", "Non risolti", "CLIENTE-IGNOTO", `value="applica"`} {
		if !strings.Contains(corpo, atteso) {
			t.Errorf("l'anteprima non dice %q: %s", atteso, primi400(corpo))
		}
	}
	if n := testutil.Conta(t, b.pool, "fornitore"); n != 0 {
		t.Fatalf("l'anteprima ha scritto %d fornitori", n)
	}

	_, corpo = ad.fai(http.MethodPost, "/admin/fornitori/importa", url.Values{"azione": {"applica"}, "testo": {seme}}, false)
	if !strings.Contains(corpo, "Seme applicato") || !strings.Contains(corpo, "CLIENTE-IGNOTO") {
		t.Fatalf("la conferma: %s", primi400(corpo))
	}
	if n := testutil.Conta(t, b.pool, "fornitore"); n != 2 {
		t.Errorf("fornitori dopo la conferma: %d, attesi 2", n)
	}
	if n := testutil.Conta(t, b.pool, "cliente_fornitore_lavorazione"); n != 1 {
		t.Errorf("qualifiche dopo la conferma: %d, attesa 1 (quella su ACME; CLIENTE-IGNOTO non si inventa)", n)
	}

	// idempotente: il secondo import non ha niente da scrivere
	_, corpo = ad.fai(http.MethodPost, "/admin/fornitori/importa", url.Values{"azione": {"applica"}, "testo": {seme}}, false)
	if !strings.Contains(corpo, "Niente da scrivere") {
		t.Errorf("il secondo import doveva dire che non c'è niente da scrivere: %s", primi400(corpo))
	}
	if testutil.Conta(t, b.pool, "fornitore") != 2 || testutil.Conta(t, b.pool, "dominio_fornitore") != 1 || testutil.Conta(t, b.pool, "contatto_fornitore") != 1 {
		t.Errorf("il secondo import ha duplicato qualcosa")
	}
	// un file rotto non scrive
	_, corpo = ad.fai(http.MethodPost, "/admin/fornitori/importa", url.Values{"azione": {"applica"}, "testo": {`{"fornitori": [{"ragione_sociale": "X", "tipo": "boh"}]}`}}, false)
	if !strings.Contains(corpo, "Non importato") || testutil.Conta(t, b.pool, "fornitore") != 2 {
		t.Errorf("il tipo sconosciuto doveva fermare tutto: %s", estratto(corpo, "errore-box"))
	}
	// e la posta di Euroforesi, ora censita, finisce nel quadrante Fornitori
	id := b.posta("entrata", "chiunque@euroforesi.example", "Offerta cataforesi", true)
	if tipo, via := b.controparteDi(id); tipo != "fornitore" || via != "dominio" {
		t.Errorf("dopo l'import la posta di Euroforesi è %s/%s", tipo, via)
	}
}

// Aggiungere un dominio o un buyer dall'anagrafica ricalcola i messaggi già arrivati e non
// decisi: dalla 0014 la controparte è scritta sul messaggio, e senza questo passaggio un
// cliente censito dopo la sua prima mail la lascerebbe «sconosciuta» per sempre.
func TestLAnagraficaRicalcolaIMessaggiGiaArrivati(t *testing.T) {
	b := preparaBancoWeb(t)
	prima := b.posta("entrata", "acquisti@tardivo.example", "RFQ arrivata prima del censimento", true)
	if tipo, _ := b.controparteDi(prima); tipo != "sconosciuto" {
		t.Fatalf("prima: %s", tipo)
	}
	c := b.unCliente("Tardivo S.p.A.", "TARDIVO", "")
	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")
	_, corpo := ad.fai(http.MethodPost, "/admin/anagrafica/"+c.String()+"/dominio", url.Values{"dominio": {"tardivo.example"}}, false)
	if !strings.Contains(corpo, "1 messaggi ricalcolati") {
		t.Errorf("l'esito non dice del ricalcolo: %s", estratto(corpo, "avviso"))
	}
	if tipo, via := b.controparteDi(prima); tipo != "cliente" || via != "dominio" {
		t.Errorf("dopo il dominio: %s/%s, atteso cliente/dominio", tipo, via)
	}
	// il fornitore, dalla sua scheda
	altro := b.posta("entrata", "info@nuovofornitore.example", "Offerta arrivata prima", true)
	f := b.unFornitore("Nuovo Fornitore", db.TipoFornitoreProcessi, "")
	_, corpo = ad.fai(http.MethodPost, "/admin/fornitori/"+f.FornitoreID.String()+"/contatto", url.Values{"email": {"INFO@nuovofornitore.example"}}, false)
	if !strings.Contains(corpo, "1 messaggi ricalcolati") {
		t.Errorf("l'esito del contatto non dice del ricalcolo: %s", estratto(corpo, "avviso"))
	}
	if tipo, via := b.controparteDi(altro); tipo != "fornitore" || via != "contatto" {
		t.Errorf("dopo il contatto: %s/%s, atteso fornitore/contatto", tipo, via)
	}
}
