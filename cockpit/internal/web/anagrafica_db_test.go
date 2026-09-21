//go:build integrazione

// L4 — blocco 3: l'anagrafica non distruttiva (voce 6.6: T7, T8), la schermata Admin → Anagrafica
// e il suo banco di prova (AN5), il 403 sulla nuova sezione.
//
// Tutto passa dal mux vero con la protezione CSRF sopra, come le altre prove di questo pacchetto:
// un controllo che esistesse solo nei gestori chiamati a mano non direbbe niente su ciò che
// risponde alla porta.
package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/ingest"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/testutil"
)

// unCliente crea un cliente direttamente in database: è lo stato di partenza, non ciò che si prova.
func (b *bancoWeb) unCliente(ragione, cartella, dominio string) uuid.UUID {
	b.t.Helper()
	c, err := CreaCliente(b.ctx, b.q, db.InsertClienteParams{CartellaNas: cartella, RagioneSociale: ragione})
	if err != nil {
		b.t.Fatal(err)
	}
	if dominio != "" {
		if err := AggiungiDominio(b.ctx, b.q, dominio, c.ClienteID); err != nil {
			b.t.Fatal(err)
		}
	}
	return c.ClienteID
}

func (b *bancoWeb) cliente(id uuid.UUID) db.Cliente {
	b.t.Helper()
	c, err := b.q.GetCliente(b.ctx, id)
	if err != nil {
		b.t.Fatal(err)
	}
	return c
}

// T7 — un cliente nuovo su una cartella NAS già presa NON riusa e NON rinomina: fallisce, dice di
// chi è la cartella, e il cliente che cEra resta identico.
//
// Prima questa era una `ON CONFLICT (cartella_nas) DO UPDATE`: chi scriveva «DELTA MECC»
// credendo di creare un cliente nuovo si portava via ragione sociale, domini, buyer e RFQ del
// cliente che quella cartella ce l'aveva gia'. Nessun errore, nessuna traccia, e l'unico modo di
// accorgersene era guardare l'elenco dei clienti il giorno dopo.
func TestT7UnClienteNonRiusaLaCartellaDiUnAltro(t *testing.T) {
	b := preparaBancoWeb(t)
	delta := b.unCliente("Delta Meccanica S.p.A.", "DELTA MECC", "delta.example")
	prima := b.cliente(delta)

	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")
	resp, corpo := ad.fai(http.MethodPost, "/admin/anagrafica",
		url.Values{"ragione_sociale": {"Pippo S.r.l."}, "cartella_nas": {"DELTA MECC"}}, true)
	if resp.StatusCode != 200 {
		t.Fatalf("la pagina non ha risposto: %d", resp.StatusCode)
	}
	if !strings.Contains(corpo, "già di Delta Meccanica S.p.A.") {
		t.Errorf("la schermata non dice di chi è la cartella:\n%s", primi400(corpo))
	}

	dopo := b.cliente(delta)
	if dopo.RagioneSociale != prima.RagioneSociale {
		t.Fatalf("Delta Meccanica è stato rinominato in %q: creare un cliente ne ha sovrascritto un altro", dopo.RagioneSociale)
	}
	var n int
	if err := b.pool.QueryRow(b.ctx, "SELECT count(*) FROM cliente").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("clienti in tabella: %d (atteso 1: il secondo non doveva nascere)", n)
	}
	// e il dominio è rimasto al suo cliente, non è passato a nessuno
	if c, err := b.q.GetClientePerDominio(b.ctx, "delta.example"); err != nil || c.ClienteID != delta {
		t.Errorf("il dominio non è più del suo cliente: %v %v", c.RagioneSociale, err)
	}
}

// T8 — un dominio già censito per un cliente non si sposta su un altro: l'assegnazione fallisce e
// dice di chi è.
//
// Spostarlo non cambia solo il futuro. `v_inbox` risolve il cliente dal dominio del mittente,
// quindi tutti i messaggi già arrivati da quel dominio cambierebbero cliente insieme a lui, e il
// triage di quelli ancora orfani cambierebbe proposta. Una UPDATE, novecento righe diverse.
func TestT8UnDominioNonPassaDaUnClienteAUnAltro(t *testing.T) {
	b := preparaBancoWeb(t)
	delta := b.unCliente("Delta Meccanica S.p.A.", "DELTA MECC", "delta.example")
	altro := b.unCliente("Beta S.r.l.", "BETA", "")

	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")
	resp, corpo := ad.fai(http.MethodPost, "/admin/anagrafica/"+altro.String()+"/dominio",
		url.Values{"dominio": {"DELTA.example"}}, true) // maiuscole: il dominio si normalizza
	if resp.StatusCode != 200 {
		t.Fatalf("la pagina non ha risposto: %d", resp.StatusCode)
	}
	if !strings.Contains(corpo, "già censito per Delta Meccanica S.p.A.") {
		t.Errorf("la schermata non dice di chi è il dominio:\n%s", primi400(corpo))
	}
	c, err := b.q.GetClientePerDominio(b.ctx, "delta.example")
	if err != nil {
		t.Fatal(err)
	}
	if c.ClienteID != delta {
		t.Fatalf("il dominio delta.example è passato a %s", c.RagioneSociale)
	}
	// l'altra metà: assegnare a un cliente un dominio che è GIÀ suo non è un errore
	if err := AggiungiDominio(b.ctx, b.q, "delta.example", delta); err != nil {
		t.Errorf("riassegnare il dominio al suo stesso cliente ha dato errore: %v", err)
	}
}

// AN1 e D17 dalla porta della schermata: un JSON che non rispetta lo schema non entra in database,
// e il testo rifiutato torna nel riquadro invece di sparire.
func TestUnaRegolaRottaNonEntraDallaSchermata(t *testing.T) {
	b := preparaBancoWeb(t)
	id := b.unCliente("ACME S.p.A.", "ACME", "acme.example")
	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")

	rotto := `{"famiglie_codice":[{"regex":"\\bAC\\d{5}\\b","esempio":"NON-CORRISPONDE"}]}`
	resp, corpo := ad.fai(http.MethodPost, "/admin/anagrafica/"+id.String()+"/regole",
		url.Values{"regole": {rotto}}, true)
	if resp.StatusCode != 200 {
		t.Fatalf("stato %d", resp.StatusCode)
	}
	if !strings.Contains(corpo, "non corrisponde") {
		t.Errorf("la schermata non dice perché è stato rifiutato:\n%s", primi400(corpo))
	}
	if !strings.Contains(corpo, "NON-CORRISPONDE") {
		t.Error("il testo rifiutato non è tornato nel riquadro: chi lo stava scrivendo dovrebbe ribatterlo da capo")
	}
	if c := b.cliente(id); strings.Contains(string(c.Regole), "NON-CORRISPONDE") {
		t.Fatalf("la regola rotta è finita in database: %s", c.Regole)
	}

	// e una regola buona entra, con la spunta
	buono := `{"famiglie_codice":[{"regex":"\\bAC\\d{5}[A-Z]\\b","descrizione":"codici ACME","esempio":"AC12345B"}]}`
	if resp, corpo := ad.fai(http.MethodPost, "/admin/anagrafica/"+id.String()+"/regole",
		url.Values{"regole": {buono}}, true); resp.StatusCode != 200 || !strings.Contains(corpo, "Regole salvate") {
		t.Fatalf("la regola buona non è stata salvata: %d\n%s", resp.StatusCode, primi400(corpo))
	}
	if c := b.cliente(id); !strings.Contains(string(c.Regole), "AC12345B") {
		t.Fatalf("la regola buona non è in database: %s", c.Regole)
	}
}

// AN5 — il banco di prova della schermata dà lo STESSO risultato che l'Inbox darà sul messaggio
// vero. Non «un risultato simile»: lo stesso, perché è la stessa funzione.
//
// La prova è in due tempi: si incolla un testo nel banco, e poi si fa arrivare lo stesso testo
// dall'ingest come messaggio di quel cliente. Il codice riconosciuto e l'esito devono coincidere.
// Un banco che chiamasse funzioni sue passerebbe questo test solo per caso, e smetterebbe di
// passarlo il giorno in cui qualcuno tocca il triage — che è esattamente quando deve accorgersene.
func TestAN5IlBancoDiProvaUsaIlMotoreVero(t *testing.T) {
	b := preparaBancoWeb(t)
	id := b.unCliente("ACME S.p.A.", "ACME", "acme.example")
	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")

	regole := `{"famiglie_codice":[{"regex":"\\bAC(?P<codice>\\d{5})(?P<rev>[A-Z])\\b","descrizione":"codici ACME","esempio":"AC12345B","rev_nel_codice":true}],
	            "riferimento_rfq":{"regex":"\\bRDO-\\d{4}\\b","descrizione":"numero RDO","esempio":"RDO-7781"}}`
	if resp, corpo := ad.fai(http.MethodPost, "/admin/anagrafica/"+id.String()+"/regole",
		url.Values{"regole": {regole}}, true); resp.StatusCode != 200 || !strings.Contains(corpo, "Regole salvate") {
		t.Fatalf("regole non salvate: %d\n%s", resp.StatusCode, primi400(corpo))
	}

	const oggetto = "RDO-7781 richiesta d'offerta"
	const corpoMail = "Buongiorno, vi chiediamo quotazione per AC12345B. Grazie."

	resp, pagina := ad.fai(http.MethodPost, "/admin/anagrafica/"+id.String()+"/prova",
		url.Values{"testo": {oggetto + "\n" + corpoMail}}, true)
	if resp.StatusCode != 200 {
		t.Fatalf("banco di prova: %d", resp.StatusCode)
	}
	for _, atteso := range []string{"12345", "rev B", "RDO-7781", "codici ACME", "nuova_rfq"} {
		if !strings.Contains(pagina, atteso) {
			t.Errorf("il banco non mostra %q:\n%s", atteso, primi400(pagina))
		}
	}

	// ora lo stesso testo, ma per la strada vera: un messaggio che arriva dal dominio del cliente
	b.ingerisciDaCliente("acme.example", oggetto, corpoMail)
	var esito string
	var identificativi []string
	if err := b.pool.QueryRow(b.ctx, `SELECT t.esito, t.identificativi FROM proposta_triage t
		JOIN messaggio m ON m.messaggio_id = t.messaggio_id WHERE m.oggetto = $1`, oggetto).Scan(&esito, &identificativi); err != nil {
		t.Fatalf("il messaggio non è arrivato al triage: %v", err)
	}
	if esito != "nuova_rfq" {
		t.Errorf("l'ingest propone %q dove il banco proponeva nuova_rfq", esito)
	}
	trovato := false
	for _, c := range identificativi {
		if c == "12345" {
			trovato = true
		}
	}
	if !trovato {
		t.Errorf("l'ingest non ha estratto il codice della famiglia del cliente: %v", identificativi)
	}
}

// Un operatore non vede la voce *Anagrafica* e non entra scrivendo l'indirizzo: è amministrativa
// (D29), perché una regex cambiata lì cambia il riconoscimento della posta di tutti.
func TestUnOperatoreNonEntraInAnagrafica(t *testing.T) {
	b := preparaBancoWeb(t)
	id := b.unCliente("ACME S.p.A.", "ACME", "acme.example")
	fp := b.browser("10.0.0.7")
	fp.login("FP", "prova-fp")

	rotte := []struct{ metodo, percorso string }{
		{http.MethodGet, "/admin/anagrafica"},
		{http.MethodGet, "/admin/anagrafica/articoli"},
		{http.MethodPost, "/admin/anagrafica"},
		{http.MethodPost, "/admin/anagrafica/" + id.String()},
		{http.MethodPost, "/admin/anagrafica/" + id.String() + "/regole"},
		{http.MethodPost, "/admin/anagrafica/" + id.String() + "/dominio"},
		{http.MethodPost, "/admin/anagrafica/" + id.String() + "/dominio/elimina"},
		{http.MethodPost, "/admin/anagrafica/" + id.String() + "/prova"},
	}
	for _, r := range rotte {
		var form url.Values
		if r.metodo == http.MethodPost {
			form = url.Values{"ragione_sociale": {"X"}, "cartella_nas": {"X"}, "dominio": {"x.example"}, "regole": {"{}"}, "peso": {"9"}}
		}
		resp, _ := fp.fai(r.metodo, r.percorso, form, true)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s: %d invece di 403", r.metodo, r.percorso, resp.StatusCode)
		}
	}
	// e niente è successo: il cliente è quello di prima, e non ne sono nati altri
	if c := b.cliente(id); c.RagioneSociale != "ACME S.p.A." || string(c.Regole) != "{}" {
		t.Errorf("qualcosa è cambiato nonostante i 403: %+v", c)
	}
	var n int
	if err := b.pool.QueryRow(b.ctx, "SELECT count(*) FROM cliente").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("clienti: %d (i POST respinti ne hanno creato uno)", n)
	}

	// la barra dell'operatore non offre la voce; quella dell'admin sì
	_, pagina := fp.fai(http.MethodGet, "/inbox", nil, false)
	if strings.Contains(pagina, `href="/admin/anagrafica"`) {
		t.Error("la rail dell'operatore porta /admin/anagrafica")
	}
	if !strings.Contains(pagina, `href="/richieste"`) {
		t.Error("la rail dell'operatore non porta /richieste")
	}
	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")
	if _, pagina := ad.fai(http.MethodGet, "/inbox", nil, false); !strings.Contains(pagina, `href="/admin/anagrafica"`) {
		t.Error("la rail dell'admin non porta /admin/anagrafica")
	}
}

// Il peso è 0–15 e il dominio lo tiene il database: un 99 battuto per errore in un campo di testo
// non deve poter diventare l'ordine di lavoro di tutti.
func TestIlPesoRestaNelSuoDominio(t *testing.T) {
	b := preparaBancoWeb(t)
	id := b.unCliente("ACME S.p.A.", "ACME", "acme.example")
	ad := b.browser("10.0.0.9")
	ad.login("AD", "prova-ad")

	for _, brutto := range []string{"99", "-1", "sedici"} {
		resp, corpo := ad.fai(http.MethodPost, "/admin/anagrafica/"+id.String(),
			url.Values{"ragione_sociale": {"ACME S.p.A."}, "peso": {brutto}, "attivo": {"1"}}, true)
		if resp.StatusCode != 200 || !strings.Contains(corpo, "da 0 a 15") {
			t.Errorf("peso %q: %d, la schermata non lo rifiuta con un motivo", brutto, resp.StatusCode)
		}
		if c := b.cliente(id); c.Peso != 0 {
			t.Fatalf("peso %q è entrato: adesso vale %d", brutto, c.Peso)
		}
	}
	if resp, _ := ad.fai(http.MethodPost, "/admin/anagrafica/"+id.String(),
		url.Values{"ragione_sociale": {"ACME S.p.A."}, "peso": {"14"}, "attivo": {"1"}}, true); resp.StatusCode != 200 {
		t.Fatalf("un peso buono non è stato salvato: %d", resp.StatusCode)
	}
	if c := b.cliente(id); c.Peso != 14 {
		t.Errorf("peso salvato: %d", c.Peso)
	}
	// e il database non si fida del Go: il vincolo sta anche lì, per chi scrive da psql
	if _, err := b.pool.Exec(b.ctx, "UPDATE cliente SET peso = 99 WHERE cliente_id = $1", id); err == nil {
		t.Error("il database ha accettato peso = 99: il CHECK non c'è")
	}
}

func primi400(s string) string {
	if len(s) > 400 {
		return s[:400] + "…"
	}
	return s
}

// ingerisciDaCliente fa arrivare un messaggio dal dominio di un cliente censito, per la strada
// vera: `ingest.Ingerisci`, lo stesso che riceve i lotti del worker. Non e' una scorciatoia — se
// scrivesse la riga a mano in tabella, il triage non passerebbe di li' e AN5 non proverebbe niente.
func (b *bancoWeb) ingerisciDaCliente(dominio, oggetto, corpo string) {
	b.t.Helper()
	casella, err := b.q.GetCasellaPerIndirizzo(b.ctx, db.GetCasellaPerIndirizzoParams{
		Canale: db.CanaleOutlook, Indirizzo: "commerciale@azienda.example"})
	if err != nil {
		b.t.Fatal(err)
	}
	ricevuto := time.Now().Add(-time.Hour)
	s := &ingest.Servizio{Pool: b.pool, Log: testutil.LogSilenzioso()}
	_, err = s.Ingerisci(b.ctx, ingest.Lotto{Casella: casella, Messaggi: []api.MessaggioIn{{
		MessageID: "<an5-" + oggetto + "@" + dominio + ">", EntryID: "ENTRY-AN5", StoreID: "STORE-AN5",
		ConversationID: "CONV-AN5", Cartella: "Posta in arrivo", Direzione: "entrata",
		DataEvento: ricevuto, RicevutoIl: &ricevuto, MittenteNome: "Ufficio Acquisti",
		MittenteIndirizzo: "acquisti@" + dominio, Oggetto: oggetto, CorpoTesto: corpo,
		Riferimenti: []string{}, Categorie: []string{},
		Destinatari: []api.Destinatario{{Indirizzo: "commerciale@azienda.example", Tipo: "a"}},
	}}})
	if err != nil {
		b.t.Fatal(err)
	}
}

// La scheda *Fascicolo atteso* dell'Anagrafica e il fascicolo vero devono rispondere la stessa
// cosa. E' una promessa esplicita — sta scritta nel COMMENT della tabella — e la regola che la
// rende non ovvia e' che il fabbisogno si risolve IN BLOCCO per `tipo_componente`: se un cliente
// scrive anche una sola riga per «sciolto», per lui valgono le sue e NON piu' i default.
//
// Una schermata che risolvesse per (tipo_componente, tipo) mostrerebbe le sue righe PIU' i default
// rimasti, cioe' un fascicolo piu' ricco di quello che il sistema poi pretende: un amministratore
// leggerebbe «serve anche il 3D» e il fascicolo non lo chiederebbe mai.
func TestLaSchedaFabbisognoDiceQuelloCheIlFascicoloPretende(t *testing.T) {
	b := preparaBancoWeb(t)
	id := b.unCliente("ACME S.p.A.", "ACME", "acme.example")

	// il cliente dichiara UNA riga per «sciolto»: il DXF, non bloccante, lo facciamo noi.
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO fabbisogno_documento (cliente_id, tipo_componente, tipo, bloccante, fonte_attesa)
		VALUES ($1, 'sciolto', 'sviluppo_dxf', false, 'promatec')`, id); err != nil {
		t.Fatal(err)
	}

	// una RFQ con un particolare, per interrogare il fascicolo vero
	var thread uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto)
		VALUES ($1, 'outlook', now(), 'prova fabbisogno') RETURNING thread_id`, id).Scan(&thread); err != nil {
		t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO componente (thread_id, codice, tipo) VALUES ($1, 'AC12345B', 'sciolto')`, thread); err != nil {
		t.Fatal(err)
	}

	// quello che il FASCICOLO pretende davvero, per questo particolare
	righe, err := b.pool.Query(b.ctx, `SELECT tipo_documento::text, bloccante FROM v_fascicolo WHERE thread_id = $1 ORDER BY tipo_documento`, thread)
	if err != nil {
		t.Fatal(err)
	}
	fascicolo := map[string]bool{}
	for righe.Next() {
		var tipo string
		var blocca bool
		if err := righe.Scan(&tipo, &blocca); err != nil {
			t.Fatal(err)
		}
		fascicolo[tipo] = blocca
	}
	righe.Close()

	// quello che la SCHEDA mostra, per lo stesso tipo di componente
	scheda := map[string]bool{}
	elenco, err := b.q.ListFabbisognoEffettivo(b.ctx, uuid.NullUUID{UUID: id, Valid: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range elenco {
		if r.TipoComponente == db.TipoComponenteSciolto {
			scheda[string(r.Tipo)] = r.Bloccante
		}
	}

	if len(fascicolo) == 0 {
		t.Fatal("il fascicolo non pretende niente: il test non proverebbe nulla")
	}
	if len(scheda) != len(fascicolo) {
		t.Fatalf("la scheda mostra %v, il fascicolo pretende %v", scheda, fascicolo)
	}
	for tipo, blocca := range fascicolo {
		if s, cE := scheda[tipo]; !cE || s != blocca {
			t.Errorf("%s: il fascicolo dice bloccante=%v, la scheda dice %v (presente=%v)", tipo, blocca, s, cE)
		}
	}
	// e il default per «sciolto» che il cliente NON ha ripetuto non cE' piu': e' la regola in blocco
	if _, cE := scheda["disegno_2d"]; cE {
		t.Error("la scheda mostra ancora il default disegno_2d per «sciolto»: sta mescolando le righe del cliente con i default")
	}
}
