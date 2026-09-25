//go:build integrazione

// L4 — le decisioni del triage sulla porta HTTP vera (revisione del 25/09): il dominio di un webmail non
// diventa di un cliente; un «no» sul buyer ferma l'aggancio invece di sparire o di rompere la
// transazione; la posta di un fornitore non apre una RFQ nemmeno chiedendola a mano; due RFQ con lo
// stesso nome non finiscono nella stessa cartella del NAS.
package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

func (b *bancoWeb) filoDelMessaggio(msg uuid.UUID) uuid.NullUUID {
	b.t.Helper()
	m, err := b.q.GetMessaggio(b.ctx, msg)
	if err != nil {
		b.t.Fatal(err)
	}
	return m.ThreadID
}

// Un cliente nuovo creato da una mail su un webmail non prende quel dominio: googlemail.com mancava
// dalla lista di triage.go (11 domini) e c'e' in quella di classificazione (26), che adesso e' l'unica.
func TestUnWebmailNonDiventaIlDominioDiUnClienteNuovo(t *testing.T) {
	b := preparaBancoWeb(t)
	msg := b.posta("entrata", "ufficio.acquisti@googlemail.com", "Richiesta PZ-001", false)
	w := operatore(b)
	_, html := w.fai(http.MethodPost, "/messaggio/"+msg.String()+"/rfq", url.Values{
		"cliente_id": {"__nuovo__"}, "cliente_nome": {"ACME S.p.A."}, "cliente_cartella": {"ACME"},
		"cliente_dominio": {"googlemail.com"}, "oggetto": {"Richiesta PZ-001"}, "priorita": {"1"}}, true)
	if !strings.Contains(leggibile(html), "RFQ creata") {
		t.Fatalf("la RFQ non e' nata: %s", estrai(html, "avviso"))
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM dominio_cliente WHERE dominio = 'googlemail.com'`); n != 0 {
		t.Errorf("googlemail.com e' diventato il dominio del cliente nuovo (%d righe)", n)
	}
}

// «Aggancia a…» con «crea il buyer»: un no sul buyer ferma l'aggancio e lo dice. Prima un errore logico
// (l'indirizzo di un altro cliente) agganciava senza buyer e senza dirlo, e un errore del database (qui
// un cognome oltre i 60 caratteri della colonna) interrompeva la transazione: l'aggancio falliva dopo,
// con un 500 che non diceva niente.
func TestUnNoSulBuyerFermaLAggancio(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("ACME S.p.A.", "ACME", "acme.example")
	beta := b.unCliente("Beta S.r.l.", "BETA", "beta.example")
	thread := b.rfqDa(uuid.Nil, acme, `ACME\WIP\2026 09 25 RFQ PZ-001`, "PZ-001")
	if _, err := b.q.InsertBuyer(b.ctx, db.InsertBuyerParams{ClienteID: beta, Cognome: "Bianchi", Email: pgtype.Text{String: "l.bianchi@beta.example", Valid: true},
		Tipo: db.TipoBuyerBuyer, Origine: db.OrigineAnagraficaCensimento}); err != nil {
		t.Fatal(err)
	}
	w := operatore(b)
	casi := []struct {
		nome, da string
		form     url.Values
		frase    string
	}{
		{"l'indirizzo e' di un altro cliente", "l.bianchi@beta.example",
			url.Values{"buyer_cognome": {"Bianchi"}, "buyer_email": {"l.bianchi@beta.example"}},
			"già censito per un altro cliente"},
		{"il database rifiuta il buyer", "anna.verdi@acme.example",
			url.Values{"buyer_cognome": {strings.Repeat("Verdi", 13)}, "buyer_nome": {"Anna"}, "buyer_email": {"anna.verdi@acme.example"}},
			"buyer:"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			msg := b.posta("entrata", c.da, "RE: RFQ PZ-001", false)
			form := url.Values{"thread_id": {thread.String()}, "crea_buyer": {"1"}, "buyer_id": {"__nuovo__"}}
			for k, v := range c.form {
				form[k] = v
			}
			resp, html := w.fai(http.MethodPost, "/messaggio/"+msg.String()+"/aggancia", form, true)
			if resp.StatusCode != 200 {
				t.Fatalf("stato %d: il no sul buyer e' diventato un errore del server\n%s", resp.StatusCode, primi400(html))
			}
			if !strings.Contains(leggibile(html), c.frase) {
				t.Errorf("la risposta non dice %q:\n%s", c.frase, estrai(html, "errore-box"))
			}
			if f := b.filoDelMessaggio(msg); f.Valid {
				t.Errorf("il messaggio e' stato agganciato lo stesso, senza il buyer")
			}
		})
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM buyer WHERE cliente_id = $1`, acme); n != 0 {
		t.Errorf("buyer di ACME: %d, atteso nessuno", n)
	}
}

// La regola del pannello («un fornitore non apre una RFQ cliente») vale anche dove la decisione si
// scrive: il form di «Nuova RFQ» chiesto a mano offre l'aggancio con il motivo, e il POST non crea niente.
func TestLaPostaDiUnFornitoreNonApreUnaRFQNemmenoAMano(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("ACME S.p.A.", "ACME", "acme.example")
	b.unFornitore("Fornitore Esempio", db.TipoFornitoreProcessi, "fornitore.example")
	msg := b.posta("entrata", "offerte@fornitore.example", "Offerta PZ-001", false)
	if tipo, _ := b.controparteDi(msg); tipo != "fornitore" {
		t.Fatalf("controparte %q: il resto della prova non direbbe niente", tipo)
	}
	w := operatore(b)
	_, form := w.fai(http.MethodGet, "/messaggio/"+msg.String()+"/triage?azione=nuova", nil, true)
	if strings.Contains(form, `/messaggio/`+msg.String()+`/rfq"`) {
		t.Error("il form di «Nuova RFQ» si apre sulla posta di un fornitore")
	}
	if !strings.Contains(leggibile(form), "Il mittente è un fornitore censito") || !strings.Contains(form, "Aggancia a una RFQ aperta") {
		t.Errorf("il form non offre l'aggancio con il motivo:\n%s", estrai(form, "<h2>"))
	}

	_, html := w.fai(http.MethodPost, "/messaggio/"+msg.String()+"/rfq", url.Values{
		"cliente_id": {acme.String()}, "oggetto": {"Offerta PZ-001"}, "priorita": {"1"}}, true)
	if !strings.Contains(leggibile(html), "Il mittente è un fornitore censito") {
		t.Errorf("il POST non dice perché:\n%s", estrai(html, "errore-box"))
	}
	if n := contaSQL(t, b, `SELECT count(*) FROM thread_offerta`); n != 0 {
		t.Errorf("il POST ha creato %d RFQ dalla posta di un fornitore", n)
	}
	if f := b.filoDelMessaggio(msg); f.Valid {
		t.Error("il messaggio del fornitore e' stato agganciato")
	}
}

// Stesso cliente, stesso buyer (nessuno), stesso oggetto, stesso giorno: la seconda RFQ prende « (2)»,
// la terza, scritta con altre maiuscole, « (3)». Sul NAS di Windows due nomi che differiscono per le
// maiuscole sono la stessa cartella.
func TestDueRFQConLoStessoNomeNonCondividonoLaCartella(t *testing.T) {
	b := preparaBancoWeb(t)
	acme := b.unCliente("ACME S.p.A.", "ACME", "acme.example")
	giorno := time.Date(2026, 9, 25, 9, 0, 0, 0, time.Local)
	w := operatore(b)
	crea := func(ora int, oggetto string) string {
		t.Helper()
		m := b.mail("entrata", "acquisti@acme.example", "Richiesta "+oggetto, "Offerta per "+oggetto+".", "commerciale@azienda.example")
		quando := giorno.Add(time.Duration(ora) * time.Hour)
		m.DataEvento, m.RicevutoIl = quando, &quando
		msg := b.postaMsg(m)
		_, html := w.fai(http.MethodPost, "/messaggio/"+msg.String()+"/rfq", url.Values{
			"cliente_id": {acme.String()}, "buyer_id": {"__nessuno__"}, "oggetto": {oggetto}, "priorita": {"1"}}, true)
		if !strings.Contains(leggibile(html), "RFQ creata") {
			t.Fatalf("RFQ non creata: %s", estrai(html, "avviso"))
		}
		var cartella string
		if err := b.pool.QueryRow(b.ctx, `SELECT t.cartella_relativa FROM thread_offerta t JOIN messaggio m ON m.thread_id = t.thread_id
			WHERE m.messaggio_id = $1`, msg).Scan(&cartella); err != nil {
			t.Fatal(err)
		}
		return cartella
	}
	prima := crea(0, "Supporto PZ-001")
	seconda := crea(1, "Supporto PZ-001")
	terza := crea(2, "supporto pz-001")
	if !strings.HasPrefix(prima, `ACME\WIP\2026 09 25 `) || strings.HasSuffix(prima, ")") {
		t.Fatalf("prima cartella: %q", prima)
	}
	if seconda != prima+" (2)" {
		t.Errorf("seconda cartella: %q, attesa %q", seconda, prima+" (2)")
	}
	if !strings.EqualFold(terza, prima+" (3)") {
		t.Errorf("terza cartella: %q, attesa %q (senza badare alle maiuscole)", terza, prima+" (3)")
	}
}
