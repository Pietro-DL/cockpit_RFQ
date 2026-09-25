//go:build integrazione

// L4 — Blocco 7A: la controparte e' un fatto sul messaggio (D33). CP1–CP5 e CP8 a livello di
// servizio, su PostgreSQL vero. Nomi e domini sono inventati (.example).

package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

type bancoControparte struct {
	t       *testing.T
	ctx     context.Context
	pool    *pgxpool.Pool
	q       *db.Queries
	s       *Servizio
	casella db.Casella
	n       int
}

func nuovoBancoControparte(t *testing.T) *bancoControparte {
	t.Helper()
	p, s, c, ctx := preparaPP(t)
	return &bancoControparte{t: t, ctx: ctx, pool: p, q: db.New(p), s: s, casella: c}
}

func (b *bancoControparte) fornitore(nome string, tipo db.TipoFornitore, domini ...string) db.Fornitore {
	b.t.Helper()
	f, err := b.q.InsertFornitore(b.ctx, db.InsertFornitoreParams{RagioneSociale: nome, Tipo: tipo})
	if err != nil {
		b.t.Fatal(err)
	}
	for _, d := range domini {
		if err := b.q.InsertDominioFornitore(b.ctx, db.InsertDominioFornitoreParams{Lower: d, FornitoreID: f.FornitoreID}); err != nil {
			b.t.Fatal(err)
		}
	}
	return f
}

func (b *bancoControparte) contatto(f db.Fornitore, email string) {
	b.t.Helper()
	if _, err := b.q.InsertContattoFornitore(b.ctx, db.InsertContattoFornitoreParams{FornitoreID: f.FornitoreID, Lower: email}); err != nil {
		b.t.Fatal(err)
	}
}

func (b *bancoControparte) cliente(cartella string, domini ...string) db.Cliente {
	b.t.Helper()
	c, err := b.q.InsertCliente(b.ctx, db.InsertClienteParams{CartellaNas: cartella, RagioneSociale: cartella + " S.p.A.", Regole: json.RawMessage("{}")})
	if err != nil {
		b.t.Fatal(err)
	}
	for _, d := range domini {
		if err := b.q.InsertDominioCliente(b.ctx, db.InsertDominioClienteParams{Lower: d, ClienteID: c.ClienteID}); err != nil {
			b.t.Fatal(err)
		}
	}
	return c
}

func (b *bancoControparte) buyer(c db.Cliente, email string) {
	b.t.Helper()
	if _, err := b.q.InsertBuyer(b.ctx, db.InsertBuyerParams{ClienteID: c.ClienteID, Cognome: "Buyer",
		Email: pgtype.Text{String: email, Valid: true}, Tipo: db.AllTipoBuyerValues()[0], Origine: db.AllOrigineAnagraficaValues()[0]}); err != nil {
		b.t.Fatal(err)
	}
}

// richiestaDOfferta e' il messaggio che ha aperto il blocco 7: parole, allegati e forma di una RFQ.
func (b *bancoControparte) richiestaDOfferta(da string, destinatari ...string) worker.MessaggioIn {
	b.n++
	quando := ppBase.Add(time.Duration(b.n) * time.Minute)
	m := worker.MessaggioIn{
		MessageID: fmt.Sprintf("<cp-%d@%s>", b.n, strings.SplitN(da, "@", 2)[1]), EntryID: fmt.Sprintf("ENTRY-CP-%d", b.n),
		StoreID: "STORE-CP", ConversationID: fmt.Sprintf("CONV-CP-%d", b.n), Cartella: cartellaPP,
		Direzione: "entrata", DataEvento: quando, RicevutoIl: &quando, MittenteNome: "Ufficio",
		MittenteIndirizzo: da, Oggetto: "RICHIESTA D'OFFERTA n. 77 - PROGETTO ALFA",
		CorpoTesto:  "Buongiorno, richiesta d'offerta per i particolari in allegato. Quotazione urgente.",
		Riferimenti: []string{}, Categorie: []string{},
		Allegati: []worker.AllegatoIn{{Indice: 1, NomeFile: "RDO_77.pdf", Estensione: "pdf", Natura: "file", Bytes: 1000},
			{Indice: 2, NomeFile: "particolare.stp", Estensione: "stp", Natura: "file", Bytes: 2000}},
	}
	if len(destinatari) == 0 {
		destinatari = []string{"commerciale@azienda.example"}
	}
	for _, d := range destinatari {
		m.Destinatari = append(m.Destinatari, worker.Destinatario{Indirizzo: d, Tipo: "a"})
	}
	return m
}

func (b *bancoControparte) ingerisci(m ...worker.MessaggioIn) {
	b.t.Helper()
	if _, err := b.s.Ingerisci(b.ctx, Lotto{Casella: b.casella, Messaggi: m}); err != nil {
		b.t.Fatal(err)
	}
}

func (b *bancoControparte) messaggio(chiave string) db.Messaggio {
	b.t.Helper()
	m, err := b.q.GetMessaggioPerChiave(b.ctx, db.GetMessaggioPerChiaveParams{Canale: db.CanaleOutlook, ChiaveEsterna: chiave})
	if err != nil {
		b.t.Fatalf("messaggio %s: %v", chiave, err)
	}
	return m
}

func (b *bancoControparte) proposta(id uuid.UUID) (db.PropostaTriage, bool) {
	b.t.Helper()
	p, err := b.q.GetTriageMessaggio(b.ctx, id)
	if err != nil {
		return db.PropostaTriage{}, false
	}
	return p, true
}

func TestCP1UnaRichiestaDOffertaDiUnFornitoreNonDiventaUnaRFQ(t *testing.T) {
	b := nuovoBancoControparte(t)
	// Il controllo (7B.3): la stessa mail da un CLIENTE censito e' una RFQ nuova; da un mittente non
	// censito e' «incerto» senza proposta (D34: da Da validare non nasce una RFQ). Altrimenti la
	// prova non proverebbe niente.
	b.cliente("CONTROLLO", "clientecontrollo.example")
	mc := b.richiestaDOfferta("acquisti@clientecontrollo.example")
	m0 := b.richiestaDOfferta("info@torniture-esempio.example")
	b.ingerisci(mc, m0)
	if p, ok := b.proposta(b.messaggio(mc.MessageID).MessaggioID); !ok || p.Esito != db.EsitoTriageNuovaRfq || p.Atto.String != "richiesta_offerta" {
		t.Fatalf("da un cliente censito il messaggio e' una RFQ nuova, altrimenti la prova non prova niente: %+v", p)
	}
	if p, ok := b.proposta(b.messaggio(m0.MessageID).MessaggioID); !ok || p.Esito == db.EsitoTriageNuovaRfq || p.Atto.String != "incerto" {
		t.Fatalf("da un mittente non censito: incerto e nessuna RFQ nuova (7B.3), non %+v", p)
	}
	f := b.fornitore("Torniture Esempio di prova", db.TipoFornitoreProcessi, "torniture-esempio.example")
	m := b.richiestaDOfferta("info@torniture-esempio.example")
	b.ingerisci(m)
	riga := b.messaggio(m.MessageID)
	if riga.ControparteTipo != db.TipoControparteFornitore || !riga.ControparteFornitoreID.Valid || riga.ControparteFornitoreID.UUID != f.FornitoreID {
		t.Fatalf("controparte = %s / %v, atteso fornitore %s", riga.ControparteTipo, riga.ControparteFornitoreID, f.FornitoreID)
	}
	if riga.ControparteVia.ViaControparte != db.ViaControparteDominio || riga.ControparteIl == nil {
		t.Fatalf("via = %+v, il = %v: il fatto si scrive con la via e l'ora", riga.ControparteVia, riga.ControparteIl)
	}
	p, ok := b.proposta(riga.MessaggioID)
	if !ok {
		t.Fatal("il triage scrive comunque il suo esito, con il motivo")
	}
	if p.Esito == db.EsitoTriageNuovaRfq {
		t.Fatalf("un fornitore in entrata ha prodotto nuova_rfq: %s", string(p.Motivi))
	}
	if p.ClienteProposto.Valid {
		t.Fatal("nessun cliente proposto per un fornitore")
	}
	if !strings.Contains(string(p.Motivi), "fornitore") {
		t.Fatalf("il motivo dice perche': %s", string(p.Motivi))
	}
}

func TestCP2UnContattoEsattoSuUnDominioGenericoEUnFornitore(t *testing.T) {
	b := nuovoBancoControparte(t)
	f := b.fornitore("Tornitore di prova", db.TipoFornitoreProcessi)
	b.contatto(f, "mario.tornitore@gmail.example")
	m := b.richiestaDOfferta("Mario.Tornitore@gmail.example")
	b.ingerisci(m)
	riga := b.messaggio(m.MessageID)
	if riga.ControparteTipo != db.TipoControparteFornitore || riga.ControparteVia.ViaControparte != db.ViaControparteContatto {
		t.Fatalf("%s via %+v", riga.ControparteTipo, riga.ControparteVia)
	}
}

func TestCP3UnDominioCensitoDaTutteEDueLePartiEAmbiguoENonProponeNiente(t *testing.T) {
	b := nuovoBancoControparte(t)
	b.fornitore("Gruppo che vende", db.TipoFornitoreMateriePrime, "gruppo.example")
	b.cliente("GRUPPO CHE COMPRA", "gruppo.example")
	m := b.richiestaDOfferta("acquisti@gruppo.example")
	b.ingerisci(m)
	riga := b.messaggio(m.MessageID)
	if riga.ControparteTipo != db.TipoControparteAmbiguo || riga.ControparteClienteID.Valid || riga.ControparteFornitoreID.Valid {
		t.Fatalf("%s %v %v", riga.ControparteTipo, riga.ControparteClienteID, riga.ControparteFornitoreID)
	}
	p, ok := b.proposta(riga.MessaggioID)
	if !ok || p.Esito != db.EsitoTriageIgnora || p.Confidenza != 0 || p.ClienteProposto.Valid {
		t.Fatalf("nessuna proposta automatica su un ambiguo: %+v", p)
	}
	if !strings.Contains(string(p.Motivi), "ambigua") {
		t.Fatalf("%s", string(p.Motivi))
	}
}

func TestCP4LaPostaInUscitaPrendeLaControparteDalPrimoDestinatarioEsterno(t *testing.T) {
	b := nuovoBancoControparte(t)
	f := b.fornitore("Zincatore di prova", db.TipoFornitoreProcessi, "zincatore.example")
	m := b.richiestaDOfferta("commerciale@azienda.example", "francesco@azienda.example", "ordini@zincatore.example")
	m.Direzione = "uscita"
	m.Oggetto = "Richiesta d'offerta zincatura"
	b.ingerisci(m)
	riga := b.messaggio(m.MessageID)
	if riga.Direzione != db.DirezioneUscita || riga.Interno {
		t.Fatalf("direzione %s interno %v", riga.Direzione, riga.Interno)
	}
	if riga.ControparteTipo != db.TipoControparteFornitore || riga.ControparteFornitoreID.UUID != f.FornitoreID {
		t.Fatalf("la controparte in uscita e' il primo destinatario esterno: %s %v", riga.ControparteTipo, riga.ControparteFornitoreID)
	}
	// e una mail fra colleghi e' interna
	mi := b.richiestaDOfferta("commerciale@azienda.example", "francesco@azienda.example")
	mi.Direzione = "uscita"
	b.ingerisci(mi)
	if r := b.messaggio(mi.MessageID); r.ControparteTipo != db.TipoControparteInterno || r.ControparteVia.ViaControparte != db.ViaControparteCasella {
		t.Fatalf("%s %+v", r.ControparteTipo, r.ControparteVia)
	}
}

func TestCP5IlRitriageMiratoRicalcolaINonDecisiELasciaIlDeciso(t *testing.T) {
	b := nuovoBancoControparte(t)
	m1 := b.richiestaDOfferta("info@nuovofornitore.example")
	m2 := b.richiestaDOfferta("vendite@nuovofornitore.example")
	m3 := b.richiestaDOfferta("info@nuovofornitore.example")
	m4 := b.richiestaDOfferta("ordini@nuovofornitore.example")
	altro := b.richiestaDOfferta("acquisti@altrove.example")
	b.ingerisci(m1, m2, m3, m4, altro)
	for _, m := range []worker.MessaggioIn{m1, m2, m3, m4} {
		r := b.messaggio(m.MessageID)
		if r.ControparteTipo != db.TipoControparteSconosciuto {
			t.Fatalf("prima del censimento: %s", r.ControparteTipo)
		}
		// 7B.3: da uno sconosciuto niente RFQ nuova; l'intento e' «incerto»
		if p, ok := b.proposta(r.MessaggioID); !ok || p.Esito == db.EsitoTriageNuovaRfq || p.Atto.String != "incerto" {
			t.Fatalf("prima del censimento la proposta e' incerta, senza RFQ nuova: %+v", p)
		}
	}
	// il terzo e' gia' deciso: la proposta e' stata accettata
	deciso := b.messaggio(m3.MessageID)
	if _, err := b.q.DecidiTriage(b.ctx, db.DecidiTriageParams{MessaggioID: deciso.MessaggioID, Stato: db.StatoTriageAccettata}); err != nil {
		t.Fatal(err)
	}
	// il quarto e' deciso in un altro modo: sta in una RFQ (thread_id), la proposta e' rimasta «proposta».
	// Sono due decisioni diverse e il ritriage deve rispettarle tutte e due.
	cl := b.cliente("ALTRO")
	var thread uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto) VALUES ($1, 'outlook', now(), 'decisa') RETURNING thread_id`, cl.ClienteID).Scan(&thread); err != nil {
		t.Fatal(err)
	}
	inRFQ := b.messaggio(m4.MessageID)
	if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET thread_id = $2, aggancio = 'operatore' WHERE messaggio_id = $1`, inRFQ.MessaggioID, thread); err != nil {
		t.Fatal(err)
	}
	// «Censisci come fornitore»
	b.fornitore("Nuovo fornitore di prova", db.TipoFornitoreProcessi, "nuovofornitore.example")
	esito, err := b.s.Ritriage(b.ctx, "info@nuovofornitore.example", "nuovofornitore.example")
	if err != nil {
		t.Fatal(err)
	}
	if esito.Guardati != 2 || esito.ContropartiCambiate != 2 || esito.ProposteCambiate != 2 {
		t.Fatalf("attesi 2 ricalcolati, 2 controparti e 2 proposte cambiate: %s\n%s", esito, strings.Join(esito.Dettagli, "\n"))
	}
	for _, m := range []worker.MessaggioIn{m1, m2} {
		r := b.messaggio(m.MessageID)
		if r.ControparteTipo != db.TipoControparteFornitore {
			t.Fatalf("dopo il censimento: %s", r.ControparteTipo)
		}
		p, _ := b.proposta(r.MessaggioID)
		if p.Esito == db.EsitoTriageNuovaRfq || p.Stato != db.StatoTriageProposta {
			t.Fatalf("la proposta ricalcolata non e' piu' una RFQ nuova: %+v", p)
		}
		// il ramo fornitore ha letto la mail: offerta (parla di offerta e allega un PDF)
		if p.Atto.String != "offerta" {
			t.Fatalf("atto dopo il censimento: %+v", p.Atto)
		}
	}
	dopo := b.messaggio(m3.MessageID)
	if dopo.ControparteTipo != db.TipoControparteSconosciuto {
		t.Fatalf("il deciso non si tocca: %s", dopo.ControparteTipo)
	}
	if p, _ := b.proposta(dopo.MessaggioID); p.Atto.String != "incerto" || p.Stato != db.StatoTriageAccettata {
		t.Fatalf("la decisione presa resta: %+v", p)
	}
	if r := b.messaggio(altro.MessageID); r.ControparteTipo != db.TipoControparteSconosciuto {
		t.Fatal("un messaggio di un altro dominio non c'entra")
	}
	if r := b.messaggio(m4.MessageID); r.ControparteTipo != db.TipoControparteSconosciuto || !r.ThreadID.Valid {
		t.Fatalf("il messaggio gia' in una RFQ non si tocca: controparte %s, thread %v", r.ControparteTipo, r.ThreadID.Valid)
	}
}

func TestUnaControparteDecisaAManoNonSiSovrascrive(t *testing.T) {
	b := nuovoBancoControparte(t)
	m := b.richiestaDOfferta("info@manuale.example")
	b.ingerisci(m)
	id := b.messaggio(m.MessageID).MessaggioID
	f := b.fornitore("Deciso a mano", db.TipoFornitoreProcessi)
	if _, err := b.q.SetControparteMessaggio(b.ctx, db.SetControparteMessaggioParams{MessaggioID: id, ControparteTipo: db.TipoControparteFornitore,
		ControparteFornitoreID: uuid.NullUUID{UUID: f.FornitoreID, Valid: true}, Via: db.NullViaControparte{ViaControparte: db.ViaControparteManuale, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.s.Ritriage(b.ctx, "info@manuale.example", "manuale.example"); err != nil {
		t.Fatal(err)
	}
	r := b.messaggio(m.MessageID)
	if r.ControparteTipo != db.TipoControparteFornitore || r.ControparteVia.ViaControparte != db.ViaControparteManuale {
		t.Fatalf("la decisione manuale resta: %s %+v", r.ControparteTipo, r.ControparteVia)
	}
}

func TestCP8IMessaggiDiPrimaDella0014SiRisolvonoAlPrimoAvvio(t *testing.T) {
	b := nuovoBancoControparte(t)
	f := b.fornitore("Vecchio fornitore", db.TipoFornitoreProcessi, "vecchio.example")
	m1 := b.richiestaDOfferta("info@vecchio.example")
	m2 := b.richiestaDOfferta("acquisti@ignoto.example")
	b.ingerisci(m1, m2)
	// com'erano prima della 0014: sconosciuti, senza data
	if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET controparte_tipo = 'sconosciuto', controparte_fornitore_id = NULL,
		controparte_cliente_id = NULL, controparte_via = NULL, controparte_il = NULL`); err != nil {
		t.Fatal(err)
	}
	n, err := RicalcolaControparti(b.ctx, b.pool, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("risolti %d, attesi 2", n)
	}
	if r := b.messaggio(m1.MessageID); r.ControparteTipo != db.TipoControparteFornitore || r.ControparteFornitoreID.UUID != f.FornitoreID || r.ControparteIl == nil {
		t.Fatalf("%s %v", r.ControparteTipo, r.ControparteFornitoreID)
	}
	if r := b.messaggio(m2.MessageID); r.ControparteTipo != db.TipoControparteSconosciuto || r.ControparteIl == nil {
		t.Fatalf("anche lo sconosciuto prende la data: %v", r.ControparteIl)
	}
	// la seconda volta non c'e' niente da fare
	if n, _ := RicalcolaControparti(b.ctx, b.pool, nil); n != 0 {
		t.Fatalf("idempotente: %d", n)
	}
}
