//go:build integrazione

// L4 — Smistamento M1 (A5.16.3, A5.16.5): i candidati della posta a livelli, sull'ingest vero. R0
// verificato e attraverso la nostra mail del Cockpit, R1 con il ConversationIndex e il cliente, R3 con
// il buyer, il pari merito, e la guardia di I14: nessuna regola scrive `messaggio.thread_id`. Nomi e
// domini inventati (ACME, @acme.example, Fornitore Esempio), codici finti (7120001A).

package ingest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// indiceRFQ è il ConversationIndex di una mail che apre una catena: 44 caratteri esadecimali. Una
// risposta ci aggiunge 10 caratteri.
const indiceRFQ = "01DB2C3D4E5F60718293A4B5C6D7E8F901A2B3C4D5E6"

// buyerDi censisce un buyer e ne restituisce l'id.
func (b *bancoControparte) buyerDi(c db.Cliente, cognome, email string) uuid.UUID {
	b.t.Helper()
	x, err := b.q.InsertBuyer(b.ctx, db.InsertBuyerParams{ClienteID: c.ClienteID, Cognome: cognome,
		Email: pgtype.Text{String: email, Valid: true}, Tipo: db.AllTipoBuyerValues()[0], Origine: db.AllOrigineAnagraficaValues()[0]})
	if err != nil {
		b.t.Fatal(err)
	}
	return x.BuyerID
}

// dalCliente è una mail in entrata da un indirizzo del cliente, nella conversazione data.
func (b *bancoControparte) dalCliente(chiave, conversazione, indice, da, oggetto, corpo string) worker.MessaggioIn {
	m := b.rispostaDelFornitore(chiave, conversazione, da, oggetto, corpo) // la stessa forma: in entrata, a noi
	m.ConversationIndex = indice
	return m
}

// agganciaAMano mette un messaggio in una RFQ come farebbe una decisione presa prima (senza log: la
// guardia di I14 conta solo le righe che scrive l'ingest).
func (b *bancoControparte) agganciaAMano(chiave string, th db.ThreadOfferta) {
	b.t.Helper()
	if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET thread_id = $1, aggancio = 'operatore' WHERE chiave_esterna = $2`, th.ThreadID, chiave); err != nil {
		b.t.Fatal(err)
	}
}

func (b *bancoControparte) candidatiAggancio(chiave string) []db.ListCandidatiAggancioRow {
	b.t.Helper()
	c, err := b.q.ListCandidatiAggancio(b.ctx, b.messaggio(chiave).MessaggioID)
	if err != nil {
		b.t.Fatal(err)
	}
	return c
}

func (b *bancoControparte) utente(sigla string) uuid.UUID {
	b.t.Helper()
	var id uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO utente (sigla, nome, ufficio, ruolo) VALUES ($1, 'Prova', 'commerciale', 'operatore') RETURNING utente_id`, sigla).Scan(&id); err != nil {
		b.t.Fatal(err)
	}
	return id
}

func (b *bancoControparte) bozzaPer(th db.ThreadOfferta, inRispostaA string, utente uuid.UUID) uuid.UUID {
	b.t.Helper()
	var id uuid.UUID
	var th1 uuid.NullUUID
	if th.ThreadID != uuid.Nil {
		th1 = uuid.NullUUID{UUID: th.ThreadID, Valid: true}
	}
	if err := b.pool.QueryRow(b.ctx, `INSERT INTO bozza (thread_id, in_risposta_a, tipo, creata_da) VALUES ($1, $2, 'risposta', $3) RETURNING bozza_id`,
		th1, b.messaggio(inRispostaA).MessaggioID, utente).Scan(&id); err != nil {
		b.t.Fatal(err)
	}
	return id
}

// 251 — la risposta del cliente alla nostra mail preparata dal Cockpit trova la sua RFQ (D84). La nostra
// mail S resta orfana: la bozza dice per quale RFQ è nata, e R0 passa attraverso di lei (97). Nessun
// aggancio, nessuna riga di log. Il controllo: senza la bozza, la stessa risposta non ha nessun R0.
func TestR0AttraversaLaNostraMailDelCockpit(t *testing.T) {
	b := nuovoBancoControparte(t)
	acme := b.cliente("ACME", "acme.example")
	th := b.rfq(acme, "RFQ 7120001", "")
	const m, s, r = "<m1-251-m@acme.example>", "<m1-251-s@azienda.example>", "<m1-251-r@acme.example>"
	b.ingerisci(b.dalCliente(m, "CONV-251", "", "buyer@acme.example", "RFQ 7120001", "Richiesta d'offerta."))
	b.agganciaAMano(m, th)
	bozza := b.bozzaPer(th, m, b.utente("M1A"))

	nostra := b.nostraMail(s, "CONV-251", "buyer@acme.example", "R: RFQ 7120001", "Ecco la nostra offerta.")
	nostra.Marcatori = map[string]string{MarcatoreBozza: bozza.String()}
	b.ingerisci(nostra)
	var inviata uuid.NullUUID
	if err := b.pool.QueryRow(b.ctx, `SELECT inviata_messaggio_id FROM bozza WHERE bozza_id = $1`, bozza).Scan(&inviata); err != nil || !inviata.Valid || inviata.UUID != b.messaggio(s).MessaggioID {
		t.Fatalf("la bozza non risulta partita con la nostra mail: %v %v", inviata, err)
	}
	b.nessunLegame(s, "la nostra mail preparata dal Cockpit")

	// la risposta del cliente: conversazione DIVERSA apposta, così a parlare è solo l'In-Reply-To
	risposta := b.dalCliente(r, "CONV-251-ALTRA", "", "buyer@acme.example", "R: R: RFQ 7120001", "Grazie, confermiamo la quantità.")
	risposta.InReplyTo, risposta.Riferimenti = s, []string{s}
	b.ingerisci(risposta)
	b.nessunLegame(r, "la risposta alla nostra mail del Cockpit")
	k, ok := candidatoDi(b.candidatiAggancio(r), th.ThreadID, db.RegolaAggancioR0Reply)
	if !ok || k.Punteggio != 97 || classificazione.TipoDaRiga(string(k.Regola), int(k.Punteggio)) != classificazione.TipoR0Cockpit {
		t.Fatalf("R0 attraverso la nostra mail del Cockpit: %+v (trovato %v), atteso 97", k, ok)
	}
	if !strings.Contains(k.Evidenza, "preparata dal Cockpit") || !strings.Contains(k.Evidenza, "buyer@acme.example") {
		t.Errorf("la frase non dice da dove viene la prova: %q", k.Evidenza)
	}
	if p, ok := b.proposta(b.messaggio(r).MessaggioID); !ok || p.Esito != db.EsitoTriageAggancia || !p.ThreadProposto.Valid || p.ThreadProposto.UUID != th.ThreadID {
		t.Errorf("la proposta della risposta: %+v", p)
	}

	// il controllo: la stessa risposta a una nostra mail SENZA bozza non ha R0
	const s2, r2 = "<m1-251-s2@azienda.example>", "<m1-251-r2@acme.example>"
	b.ingerisci(b.nostraMail(s2, "CONV-251-B", "buyer@acme.example", "R: RFQ 7120001", "Ecco la nostra offerta."))
	senza := b.dalCliente(r2, "CONV-251-C", "", "buyer@acme.example", "R: R: RFQ 7120001", "Grazie, confermiamo la quantità.")
	senza.InReplyTo, senza.Riferimenti = s2, []string{s2}
	b.ingerisci(senza)
	if _, ok := candidatoDi(b.candidatiAggancio(r2), th.ThreadID, db.RegolaAggancioR0Reply); ok {
		t.Error("senza la bozza la nostra mail non è di nessuna RFQ: R0 non deve trovare niente")
	}
	// e una risposta da un terzo alla nostra mail del Cockpit non è verificata: resta forte, non molto forte
	const r3 = "<m1-251-r3@esterno.example>"
	terzo := b.dalCliente(r3, "CONV-251-D", "", "qualcuno@esterno.example", "R: R: RFQ 7120001", "Vi rispondo io.")
	terzo.InReplyTo, terzo.Riferimenti = s, []string{s}
	b.ingerisci(terzo)
	if k, ok := candidatoDi(b.candidatiAggancio(r3), th.ThreadID, db.RegolaAggancioR0Reply); !ok || k.Punteggio != 88 || !strings.Contains(k.Evidenza, "non era fra i destinatari") {
		t.Errorf("R0 da un terzo: %+v (trovato %v), atteso 88 non verificato", k, ok)
	}
}

// 253 — R1 è forte solo con l'indice che discende da una mail della RFQ E lo stesso cliente; con
// l'indice di sola intestazione (la conversazione messa insieme per l'oggetto) è debole e non aggancia.
func TestR1ForteVuoleIndiceEClienteR1DeboleSenza(t *testing.T) {
	b := nuovoBancoControparte(t)
	acme := b.cliente("ACME", "acme.example")
	th := b.rfq(acme, "RFQ 7120001", "")
	const primo, forte, debole = "<m1-253-p@acme.example>", "<m1-253-f@acme.example>", "<m1-253-d@acme.example>"
	b.ingerisci(b.dalCliente(primo, "CONV-253", indiceRFQ, "buyer@acme.example", "RFQ 7120001", "Richiesta d'offerta."))
	b.agganciaAMano(primo, th)
	if err := b.q.CollegaConversazione(b.ctx, db.CollegaConversazioneParams{ConversazioneID: b.messaggio(primo).ConversazioneID,
		ThreadID: uuid.NullUUID{UUID: th.ThreadID, Valid: true}, CollegataDa: db.AggancioOperatore}); err != nil {
		t.Fatal(err)
	}
	b.ingerisci(b.dalCliente(forte, "CONV-253", indiceRFQ+"0000A1B2C3", "buyer@acme.example", "R: RFQ 7120001", "Ecco i disegni."))
	b.ingerisci(b.dalCliente(debole, "CONV-253", strings.Repeat("7F", 22), "buyer@acme.example", "RFQ 7120001", "Ecco i disegni."))

	k, ok := candidatoDi(b.candidatiAggancio(forte), th.ThreadID, db.RegolaAggancioR1Conversazione)
	if !ok || k.Punteggio != 86 {
		t.Errorf("indice che discende e stesso cliente: %+v (trovato %v), atteso 86", k, ok)
	}
	if p, ok := b.proposta(b.messaggio(forte).MessaggioID); !ok || p.Esito != db.EsitoTriageAggancia || p.Confidenza != 86 {
		t.Errorf("R1 forte: proposta %+v", p)
	}
	k, ok = candidatoDi(b.candidatiAggancio(debole), th.ThreadID, db.RegolaAggancioR1Conversazione)
	if !ok || k.Punteggio != 40 || !strings.Contains(k.Evidenza, "non risulta una risposta") {
		t.Errorf("indice di sola intestazione: %+v (trovato %v), atteso 40", k, ok)
	}
	if p, ok := b.proposta(b.messaggio(debole).MessaggioID); ok && p.Esito == db.EsitoTriageAggancia {
		t.Errorf("R1 solo ConversationID: l'esito è «aggancia» (%d)", p.Confidenza)
	}
	b.nessunLegame(forte, "R1 forte")
	b.nessunLegame(debole, "R1 debole")
}

// 253, le due origini — la nostra mail del Cockpit nata da una bozza per T1 in risposta a una mail che
// oggi sta in T2: R1 porta a tutte e due, come R0 e il box del pannello (OrigineDaBozza). La risposta del
// cliente discende dalla nostra mail (stessa conversazione, indice che discende, nessun In-Reply-To):
// due R1 forti, stessa forza, nessuna proposta. Prima R1 seguiva solo la RFQ della bozza, e proponeva T1.
func TestR1SegueLeDueOriginiDellaNostraMail(t *testing.T) {
	b := nuovoBancoControparte(t)
	acme := b.cliente("ACME", "acme.example")
	t1 := b.rfq(acme, "RFQ 7120001", "")
	t2 := b.rfq(acme, "RFQ 7120002", "")
	const m, s, r = "<m1-253o-m@acme.example>", "<m1-253o-s@azienda.example>", "<m1-253o-r@acme.example>"
	// la mail a cui la bozza risponde sta in T2, in un'altra conversazione: T2 lo porta solo la bozza
	b.ingerisci(b.dalCliente(m, "CONV-253O-M", "", "buyer@acme.example", "RFQ 7120002", "Richiesta d'offerta."))
	b.agganciaAMano(m, t2)
	bozza := b.bozzaPer(t1, m, b.utente("M1O"))
	nostra := b.nostraMail(s, "CONV-253O", "buyer@acme.example", "RFQ 7120001", "Ecco la nostra offerta.")
	nostra.ConversationIndex = indiceRFQ
	nostra.Marcatori = map[string]string{MarcatoreBozza: bozza.String()}
	b.ingerisci(nostra)
	b.nessunLegame(s, "la nostra mail preparata dal Cockpit")

	b.ingerisci(b.dalCliente(r, "CONV-253O", indiceRFQ+"0000A1B2C3", "buyer@acme.example", "R: RFQ 7120001", "Ricevuto, grazie."))
	for _, th := range []db.ThreadOfferta{t1, t2} {
		if k, ok := candidatoDi(b.candidatiAggancio(r), th.ThreadID, db.RegolaAggancioR1Conversazione); !ok || k.Punteggio != 86 {
			t.Errorf("R1 verso %s: %+v (trovato %v), atteso 86", th.Oggetto.String, k, ok)
		}
	}
	if p, ok := b.proposta(b.messaggio(r).MessaggioID); !ok || p.Esito != db.EsitoTriageAggancia || p.ThreadProposto.Valid {
		t.Errorf("due origini della stessa forza: proposta %+v, attesa «aggancia» senza RFQ", p)
	}
	b.nessunLegame(r, "la risposta alla nostra mail con due origini")
}

// 254 — R1 non attraversa i clienti: la conversazione di una RFQ di BETA non fa agganciare una mail di
// ACME. Con l'indice che discende è al più «debole» (45); con la sola conversazione 40, e una richiesta
// vera (parole e STEP) può essere proposta come nuova.
func TestR1NonAttraversaIClienti(t *testing.T) {
	b := nuovoBancoControparte(t)
	b.cliente("ACME", "acme.example")
	beta := b.cliente("BETA", "beta.example")
	thB := b.rfq(beta, "Richiesta di offerta", "")
	const mb, indice, oggetto = "<m1-254-b@beta.example>", "<m1-254-i@acme.example>", "<m1-254-o@acme.example>"
	b.ingerisci(b.dalCliente(mb, "CONV-254", indiceRFQ, "acquisti@beta.example", "Richiesta di offerta", "Richiesta d'offerta."))
	b.agganciaAMano(mb, thB)

	b.ingerisci(b.dalCliente(indice, "CONV-254", indiceRFQ+"0000A1B2C3", "buyer@acme.example", "R: Richiesta di offerta", "Ricevuto."))
	k, ok := candidatoDi(b.candidatiAggancio(indice), thB.ThreadID, db.RegolaAggancioR1Conversazione)
	if !ok || k.Punteggio != 45 || !strings.Contains(k.Evidenza, "cliente è un altro") {
		t.Errorf("indice che discende, cliente diverso: %+v (trovato %v), atteso 45", k, ok)
	}
	if p, ok := b.proposta(b.messaggio(indice).MessaggioID); ok && p.Esito == db.EsitoTriageAggancia {
		t.Errorf("una mail di ACME è proposta per la RFQ di BETA: %+v", p)
	}

	nuova := b.dalCliente(oggetto, "CONV-254", strings.Repeat("3A", 22), "buyer@acme.example", "Richiesta di offerta", "Buongiorno, richiesta d'offerta per il particolare in allegato.")
	nuova.Allegati = []worker.AllegatoIn{{Indice: 1, NomeFile: "7120001A_1.stp", Estensione: "stp", Natura: "file", Bytes: 2000}}
	b.ingerisci(nuova)
	k, ok = candidatoDi(b.candidatiAggancio(oggetto), thB.ThreadID, db.RegolaAggancioR1Conversazione)
	if !ok || k.Punteggio != 40 {
		t.Errorf("solo la conversazione: %+v (trovato %v), atteso 40", k, ok)
	}
	if p, ok := b.proposta(b.messaggio(oggetto).MessaggioID); !ok || p.Esito != db.EsitoTriageNuovaRfq {
		t.Errorf("una richiesta nuova di ACME nella conversazione di BETA: proposta %+v, attesa nuova_rfq", p)
	}
}

// 255 — R3 con lo stesso buyer della RFQ viene sopra R3 con un altro buyer: 72 contro 62, in ordine, e
// la proposta va alla prima (i livelli sono diversi: non è un pari merito).
func TestR3ConLoStessoBuyerVieneSopra(t *testing.T) {
	b := nuovoBancoControparte(t)
	acme := b.clienteConRegole("ACME", regoleCliente, "acme.example")
	suo := b.buyerDi(acme, "Rossi", "buyer@acme.example")
	altro := b.buyerDi(acme, "Bianchi", "altro.buyer@acme.example")
	thSuo := b.rfq(acme, "RFQ staffa", "7120001A")
	thAltro := b.rfq(acme, "RFQ supporto", "7120001A")
	for th, buyer := range map[uuid.UUID]uuid.UUID{thSuo.ThreadID: suo, thAltro.ThreadID: altro} {
		if _, err := b.pool.Exec(b.ctx, `UPDATE thread_offerta SET buyer_id = $2 WHERE thread_id = $1`, th, buyer); err != nil {
			t.Fatal(err)
		}
	}
	const c = "<m1-255@acme.example>"
	b.ingerisci(b.dalCliente(c, "CONV-255", "", "buyer@acme.example", "Codice 7120001A", "Vi scrivo per il codice 7120001A."))
	cand := b.candidatiAggancio(c)
	s, ok1 := candidatoDi(cand, thSuo.ThreadID, db.RegolaAggancioR3Codice)
	a, ok2 := candidatoDi(cand, thAltro.ThreadID, db.RegolaAggancioR3Codice)
	if !ok1 || !ok2 || s.Punteggio != 72 || a.Punteggio != 62 {
		t.Fatalf("R3: stesso buyer %+v, altro buyer %+v", s, a)
	}
	if !strings.Contains(s.Evidenza, "stesso buyer") || !strings.Contains(a.Evidenza, "buyer è un altro") {
		t.Errorf("le frasi: %q / %q", s.Evidenza, a.Evidenza)
	}
	// in ordine: la riga del buyer giusto prima di quella dell'altro
	pos := map[uuid.UUID]int{}
	for i, k := range cand {
		if k.Regola == db.RegolaAggancioR3Codice {
			pos[k.ThreadID] = i
		}
	}
	if pos[thSuo.ThreadID] > pos[thAltro.ThreadID] {
		t.Errorf("la RFQ dello stesso buyer non viene sopra: %+v", cand)
	}
	if p, ok := b.proposta(b.messaggio(c).MessaggioID); !ok || p.Esito != db.EsitoTriageAggancia || p.ThreadProposto.UUID != thSuo.ThreadID {
		t.Errorf("proposta %+v, attesa la RFQ dello stesso buyer", p)
	}
}

// 262 — due RFQ aperte di ACME con lo stesso codice e lo stesso buyer: stessa forza, nessuna proposta.
// L'esito resta «aggancia» (la richiesta esiste), il motivo lo dice, e thread_proposto resta NULL.
func TestIlPariMeritoNonSiPropone(t *testing.T) {
	b := nuovoBancoControparte(t)
	acme := b.clienteConRegole("ACME", regoleCliente, "acme.example")
	suo := b.buyerDi(acme, "Rossi", "buyer@acme.example")
	uno := b.rfq(acme, "RFQ staffa", "7120001A")
	due := b.rfq(acme, "RFQ supporto", "7120001A")
	if _, err := b.pool.Exec(b.ctx, `UPDATE thread_offerta SET buyer_id = $1 WHERE thread_id IN ($2, $3)`, suo, uno.ThreadID, due.ThreadID); err != nil {
		t.Fatal(err)
	}
	const c = "<m1-262@acme.example>"
	b.ingerisci(b.dalCliente(c, "CONV-262", "", "buyer@acme.example", "Codice 7120001A", "Vi scrivo per il codice 7120001A."))
	p, ok := b.proposta(b.messaggio(c).MessaggioID)
	if !ok || p.Esito != db.EsitoTriageAggancia || p.ThreadProposto.Valid {
		t.Fatalf("pari merito: proposta %+v", p)
	}
	var motivi []string
	_ = json.Unmarshal(p.Motivi, &motivi)
	if !strings.Contains(strings.Join(motivi, " | "), "stessa forza") {
		t.Errorf("manca il motivo del pari merito: %v", motivi)
	}
	if n := len(b.candidatiAggancio(c)); n < 2 {
		t.Errorf("le due RFQ devono restare visibili: %d candidati", n)
	}
	b.nessunLegame(c, "pari merito")
	// il controllo: con la sola RFQ uno, la proposta c'è
	if _, err := b.pool.Exec(b.ctx, `DELETE FROM identificativo_thread WHERE thread_id = $1`, due.ThreadID); err != nil {
		t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `UPDATE thread_offerta SET buyer_id = NULL WHERE thread_id = $1`, due.ThreadID); err != nil {
		t.Fatal(err)
	}
	const c2 = "<m1-262-b@acme.example>"
	b.ingerisci(b.dalCliente(c2, "CONV-262-B", "", "buyer@acme.example", "Codice 7120001A", "Vi scrivo per il codice 7120001A."))
	if p, ok := b.proposta(b.messaggio(c2).MessaggioID); !ok || !p.ThreadProposto.Valid || p.ThreadProposto.UUID != uno.ThreadID {
		t.Errorf("con una sola RFQ forte: proposta %+v", p)
	}
}

// 261 (I14) — nessuna regola della posta scrive `messaggio.thread_id`: una scena con R0 verificato,
// R0 attraverso la nostra mail del Cockpit, R1 forte, R4, R3, il marcatore della bozza e quello della
// richiesta a un fornitore. Alla fine l'unico messaggio in una RFQ è quello messo lì a mano, e il registro
// degli agganci è vuoto. Il controllo che la scena non sia vuota: i candidati di ogni tipo ci sono.
func TestNessunaRegolaScriveThreadID(t *testing.T) {
	b := nuovoBancoControparte(t)
	acme := b.clienteConRegole("ACME", regoleCliente, "acme.example")
	b.buyerDi(acme, "Rossi", "buyer@acme.example")
	th := b.rfq(acme, "RFQ 7120001A", "7120001A")
	if err := b.q.SetRiferimentoCliente(b.ctx, db.SetRiferimentoClienteParams{ThreadID: th.ThreadID, Riferimento: pgtype.Text{String: "RDO 400012345", Valid: true}}); err != nil {
		t.Fatal(err)
	}
	fornitore := b.fornitore("Fornitore Esempio", db.TipoFornitoreProcessi, "fornitore-esempio.example")
	b.contatto(fornitore, "info@fornitore-esempio.example")
	ric := b.richiesta(th, fornitore, db.StatoRichiestaFornitoreBozza, uuid.NullUUID{}, "")

	const m, s, f, tutto, via = "<m1-261-m@acme.example>", "<m1-261-s@azienda.example>", "<m1-261-f@azienda.example>", "<m1-261-t@acme.example>", "<m1-261-v@acme.example>"
	b.ingerisci(b.dalCliente(m, "CONV-261", indiceRFQ, "buyer@acme.example", "RFQ 7120001A", "Richiesta d'offerta RDO 400012345."))
	b.agganciaAMano(m, th)
	bozza := b.bozzaPer(th, m, b.utente("M1I"))
	nostra := b.nostraMail(s, "CONV-261-S", "buyer@acme.example", "R: RFQ 7120001A", "La nostra offerta.")
	nostra.Marcatori = map[string]string{MarcatoreBozza: bozza.String()}
	aFornitore := b.nostraMail(f, "CONV-261-F", "info@fornitore-esempio.example", "RFQ ACME 7120001A", "Vi chiediamo offerta.")
	aFornitore.Marcatori = map[string]string{MarcatoreRichiesta: ric.RichiestaID.String()}
	b.ingerisci(nostra, aFornitore)

	// una risposta che ha tutto: In-Reply-To verificato, indice che discende, riferimento e codice
	x := b.dalCliente(tutto, "CONV-261", indiceRFQ+"0000A1B2C3", "buyer@acme.example", "R: RFQ 7120001A", "Per la RDO 400012345, il codice 7120001A: ecco i disegni.")
	x.InReplyTo, x.Riferimenti = m, []string{m}
	// e una che passa dalla nostra mail del Cockpit
	y := b.dalCliente(via, "CONV-261-V", "", "buyer@acme.example", "R: R: RFQ 7120001A", "Confermiamo.")
	y.InReplyTo, y.Riferimenti = s, []string{s}
	b.ingerisci(x, y)

	// la scena non è vuota
	cand := b.candidatiAggancio(tutto)
	for regola, punti := range map[db.RegolaAggancio]int16{db.RegolaAggancioR0Reply: 98, db.RegolaAggancioR1Conversazione: 86, db.RegolaAggancioR4Riferimento: 84, db.RegolaAggancioR3Codice: 62} {
		if k, ok := candidatoDi(cand, th.ThreadID, regola); !ok || k.Punteggio != punti {
			t.Errorf("%s: %+v (trovato %v), atteso %d", regola, k, ok, punti)
		}
	}
	if k, ok := candidatoDi(b.candidatiAggancio(via), th.ThreadID, db.RegolaAggancioR0Reply); !ok || k.Punteggio != 97 {
		t.Errorf("R0 attraverso la bozza: %+v (trovato %v)", k, ok)
	}
	if r := b.statoRichiesta(ric.RichiestaID); r.Stato != db.StatoRichiestaFornitoreInviata {
		t.Errorf("il marcatore della richiesta non l'ha legata: %+v", r)
	}

	// la guardia
	var agganciati, righeLog int
	var quale string
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*), COALESCE(min(chiave_esterna), '') FROM messaggio WHERE thread_id IS NOT NULL`).Scan(&agganciati, &quale); err != nil {
		t.Fatal(err)
	}
	if agganciati != 1 || quale != m {
		t.Errorf("messaggi in una RFQ: %d (%s), atteso solo quello messo a mano", agganciati, quale)
	}
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FROM messaggio_aggancio_log`).Scan(&righeLog); err != nil {
		t.Fatal(err)
	}
	if righeLog != 0 {
		t.Errorf("righe nel registro degli agganci: %d, nessuna regola deve scriverne", righeLog)
	}
	for _, k := range []string{s, f, tutto, via} {
		b.nessunLegameAllaRFQ(k)
	}
}

// nessunLegameAllaRFQ: il messaggio non è in nessuna RFQ e non ha righe nel registro (può essere legato
// alla sua richiesta: è il legame del marcatore, D85).
func (b *bancoControparte) nessunLegameAllaRFQ(chiave string) {
	b.t.Helper()
	tid, _, agg, n := b.legami(chiave)
	if tid.Valid || agg != db.AggancioNessuno || n != 0 {
		b.t.Errorf("%s: thread %v, aggancio %s, %d righe di log — nessuna regola aggancia", chiave, tid, agg, n)
	}
}
