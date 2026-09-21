//go:build integrazione

// L4 — Blocco 7C.0, gli invarianti 4 e 5 del contratto di classificazione:
//
//	4. R0 (In-Reply-To) e R1 (ConversationID) non scrivono mai `thread_id` né `richiesta_fornitore_id`:
//	   sono evidenze, e diventano candidati. Il legame lo scrive una decisione.
//	5. Solo il marcatore `CockpitRichiestaFornitore` scrive il legame da solo, perché quel rapporto
//	   lo ha creato il Cockpit quando l'operatore ha chiesto la bozza. E lo scrive SOLO dove l'ha
//	   messo lui: sulla nostra mail in uscita, verso una richiesta che esiste, su un messaggio che
//	   nessuno ha già messo altrove.
//
// T22 e IB2 provano già il lato cliente di R0 e il caso buono del marcatore. Qui c'è il resto.
// Nomi e domini sono inventati (.example).

package ingest

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/contratti/api"
	"promatec/cockpit/internal/platform/db"
)

// nostraMail costruisce una mail IN USCITA dalla casella del banco a un destinatario esterno. La
// direzione la decide il server dalle caselle censite, non questo campo (vedi caselle_db_test).
func (b *bancoControparte) nostraMail(chiave, conversazione, a, oggetto, corpo string) api.MessaggioIn {
	b.n++
	quando := ppBase.Add(time.Duration(b.n) * time.Minute)
	return api.MessaggioIn{
		MessageID: chiave, EntryID: fmt.Sprintf("ENTRY-CP-%d", b.n), StoreID: "STORE-CP", ConversationID: conversazione,
		Cartella: "Posta inviata", Direzione: "uscita", DataEvento: quando, RicevutoIl: &quando,
		MittenteNome: "Commerciale", MittenteIndirizzo: b.casella.Indirizzo, Oggetto: oggetto, CorpoTesto: corpo,
		Destinatari: []api.Destinatario{{Indirizzo: a, Tipo: "a"}}, Riferimenti: []string{}, Categorie: []string{},
	}
}

// rispostaDelFornitore costruisce una mail IN ENTRATA da un contatto del fornitore.
func (b *bancoControparte) rispostaDelFornitore(chiave, conversazione, da, oggetto, corpo string) api.MessaggioIn {
	b.n++
	quando := ppBase.Add(time.Duration(b.n) * time.Minute)
	return api.MessaggioIn{
		MessageID: chiave, EntryID: fmt.Sprintf("ENTRY-CP-%d", b.n), StoreID: "STORE-CP", ConversationID: conversazione,
		Cartella: cartellaPP, Direzione: "entrata", DataEvento: quando, RicevutoIl: &quando,
		MittenteNome: "Ufficio vendite", MittenteIndirizzo: da, Oggetto: oggetto, CorpoTesto: corpo,
		Destinatari: []api.Destinatario{{Indirizzo: b.casella.Indirizzo, Tipo: "a"}}, Riferimenti: []string{}, Categorie: []string{},
	}
}

func (b *bancoControparte) rfq(c db.Cliente, oggetto, codice string) db.ThreadOfferta {
	b.t.Helper()
	th, err := b.q.InsertThread(b.ctx, db.InsertThreadParams{ClienteID: c.ClienteID, Canale: db.CanaleOutlook, DataInizio: ppBase,
		Oggetto: pgtype.Text{String: oggetto, Valid: true}, CartellaRelativa: pgtype.Text{String: c.CartellaNas + `\WIP\x`, Valid: true}, Priorita: 1})
	if err != nil {
		b.t.Fatal(err)
	}
	if codice != "" {
		if _, err := b.q.UpsertIdentificativo(b.ctx, db.UpsertIdentificativoParams{ThreadID: th.ThreadID, Codice: codice, Origine: db.OrigineIdentificativoManuale}); err != nil {
			b.t.Fatal(err)
		}
	}
	return th
}

// richiesta inserisce una richiesta a un fornitore; la lavorazione fa parte della chiave unica
// (RFQ, fornitore, lavorazione), quindi due richieste allo stesso fornitore sulla stessa RFQ ne
// vogliono due diverse.
func (b *bancoControparte) richiesta(th db.ThreadOfferta, f db.Fornitore, stato db.StatoRichiestaFornitore, nostra uuid.NullUUID, lavorazione string) db.RichiestaFornitore {
	b.t.Helper()
	r, err := b.q.InsertRichiestaFornitore(b.ctx, db.InsertRichiestaFornitoreParams{ThreadID: th.ThreadID, FornitoreID: f.FornitoreID,
		Codici: []string{"6674611A"}, Stato: stato, MessaggioID: nostra, Lavorazione: pgtype.Text{String: lavorazione, Valid: lavorazione != ""}})
	if err != nil {
		b.t.Fatal(err)
	}
	return r
}

func (b *bancoControparte) legami(chiave string) (thread, richiesta uuid.NullUUID, aggancio db.Aggancio, nLog int) {
	b.t.Helper()
	m := b.messaggio(chiave)
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FROM messaggio_aggancio_log WHERE messaggio_id = $1`, m.MessaggioID).Scan(&nLog); err != nil {
		b.t.Fatal(err)
	}
	return m.ThreadID, m.RichiestaFornitoreID, m.Aggancio, nLog
}

func (b *bancoControparte) statoRichiesta(id uuid.UUID) db.RichiestaFornitore {
	b.t.Helper()
	r, err := b.q.GetRichiesta(b.ctx, id)
	if err != nil {
		b.t.Fatal(err)
	}
	return r
}

// nessunLegame è l'affermazione dell'invariante 4: dopo l'ingest il messaggio è orfano su tutti e
// due i fronti, e nessuno ha scritto nel registro degli agganci.
func (b *bancoControparte) nessunLegame(chiave, perche string) {
	b.t.Helper()
	tid, rid, agg, n := b.legami(chiave)
	if tid.Valid || rid.Valid || agg != db.AggancioNessuno || n != 0 {
		b.t.Errorf("%s: %s ha thread %v, richiesta %v, aggancio %s, %d righe di log — l'ingest propone, non decide",
			perche, chiave, tid, rid, agg, n)
	}
}

// I4 — la risposta di un fornitore con In-Reply-To alla nostra richiesta (R0) e quella nella stessa
// conversazione (R1) sono candidati al 95 e all'80 verso la richiesta: né il messaggio né la
// richiesta cambiano. Lo stato resta `inviata` finché una persona non conferma.
func TestI4R0ER1SullaPostaDelFornitoreSonoCandidatiENonLegami(t *testing.T) {
	b := nuovoBancoControparte(t)
	acme := b.cliente("ACME", "acme.example")
	mgm := b.fornitore("MGM Lavorazioni di prova", db.TipoFornitoreProcessi, "mgm.example")
	b.contatto(mgm, "info@mgm.example")
	th := b.rfq(acme, "RFQ 6674611A", "6674611A")

	const nostra = "<i4-nostra@azienda.it>"
	b.ingerisci(b.nostraMail(nostra, "CONV-I4", "info@mgm.example", "RFQ ACME 6674611A", "Vi chiediamo offerta per il codice 6674611A."))
	b.nessunLegame(nostra, "la nostra mail mandata a mano, senza marcatore")
	r := b.richiesta(th, mgm, db.StatoRichiestaFornitoreInviata, uuid.NullUUID{UUID: b.messaggio(nostra).MessaggioID, Valid: true}, "")

	// R0: In-Reply-To verso la nostra mail, conversazione DIVERSA apposta così parla solo l'header
	const r0 = "<i4-r0@mgm.example>"
	m := b.rispostaDelFornitore(r0, "CONV-I4-ALTRA", "info@mgm.example", "R: RFQ ACME 6674611A", "In allegato la nostra offerta.")
	m.InReplyTo = nostra
	m.Riferimenti = []string{nostra}
	b.ingerisci(m)
	b.nessunLegame(r0, "R0")
	b.candidatoVerso(r0, r.RichiestaID, db.RegolaRichiestaR0Reply, 95)

	// R1: stessa conversazione della nostra mail, senza In-Reply-To
	const r1 = "<i4-r1@mgm.example>"
	b.ingerisci(b.rispostaDelFornitore(r1, "CONV-I4", "info@mgm.example", "RFQ ACME 6674611A", "Ricevuto, vi rispondiamo domani."))
	b.nessunLegame(r1, "R1")
	b.candidatoVerso(r1, r.RichiestaID, db.RegolaRichiestaR1Conversazione, 80)

	// la richiesta è quella di prima: due risposte arrivate, nessuna registrata
	if x := b.statoRichiesta(r.RichiestaID); x.Stato != db.StatoRichiestaFornitoreInviata || x.OffertaRicevutaIl != nil {
		t.Errorf("due risposte ingerite e la richiesta è cambiata da sola: %+v", x)
	}
	// la proposta del triage dice «aggancia a QUESTA richiesta»: una proposta, che la schermata mostra
	p, ok := b.proposta(b.messaggio(r0).MessaggioID)
	if !ok || p.Esito != db.EsitoTriageAggancia || !p.RichiestaProposta.Valid || p.RichiestaProposta.UUID != r.RichiestaID {
		t.Errorf("la proposta per la risposta R0: %+v", p)
	}
}

func (b *bancoControparte) candidatoVerso(chiave string, rid uuid.UUID, regola db.RegolaRichiesta, punti int16) {
	b.t.Helper()
	cand, err := b.q.ListCandidatiRichiesta(b.ctx, b.messaggio(chiave).MessaggioID)
	if err != nil {
		b.t.Fatal(err)
	}
	for _, c := range cand {
		if c.RichiestaID == rid && c.Regola == regola {
			if c.Punteggio != punti {
				b.t.Errorf("%s: il candidato %s vale %d, atteso %d", chiave, regola, c.Punteggio, punti)
			}
			return
		}
	}
	b.t.Errorf("%s: manca il candidato %s verso la richiesta: %+v", chiave, regola, cand)
}

// I5 — il marcatore scrive il legame da solo, ma solo dove l'ha messo il Cockpit.
//
//	(a) sulla nostra mail in uscita: thread, richiesta, log, e la richiesta passa a `inviata`;
//	(b) lo stesso marcatore su una mail IN ENTRATA non vale niente: il Cockpit non scrive
//	    UserProperties sulla posta che arriva, quindi chiunque ce l'abbia messa non siamo noi;
//	(c) un marcatore che punta a una richiesta inesistente, o che non è nemmeno un uuid, si
//	    registra nel log e non ferma l'ingest;
//	(d) un messaggio che l'operatore ha già messo in un'ALTRA RFQ non si sposta: il risync con il
//	    marcatore lo lascia dov'è, e non lega nemmeno la richiesta.
func TestI5IlMarcatoreScriveIlLegameSoloDoveLoHaMessoIlCockpit(t *testing.T) {
	b := nuovoBancoControparte(t)
	acme := b.cliente("ACME", "acme.example")
	mgm := b.fornitore("MGM Lavorazioni di prova", db.TipoFornitoreProcessi, "mgm.example")
	b.contatto(mgm, "info@mgm.example")
	thA := b.rfq(acme, "RFQ 6674611A", "6674611A")
	thB := b.rfq(acme, "RFQ altra", "")
	r := b.richiesta(thA, mgm, db.StatoRichiestaFornitoreBozza, uuid.NullUUID{}, "")

	// (a) il caso buono, a livello di ingest: è il controllo per i tre che seguono
	const buona = "<i5-buona@azienda.it>"
	m := b.nostraMail(buona, "CONV-I5-A", "info@mgm.example", "RFQ ACME 6674611A", "Vi chiediamo offerta.")
	m.Marcatori = map[string]string{MarcatoreRichiesta: r.RichiestaID.String()}
	b.ingerisci(m)
	tid, rid, agg, n := b.legami(buona)
	if !tid.Valid || tid.UUID != thA.ThreadID || !rid.Valid || rid.UUID != r.RichiestaID || agg != db.AggancioOperatore || n != 1 {
		t.Fatalf("(a) la nostra mail con il marcatore: thread %v, richiesta %v, aggancio %s, log %d", tid, rid, agg, n)
	}
	inviata := b.statoRichiesta(r.RichiestaID)
	if inviata.Stato != db.StatoRichiestaFornitoreInviata || !inviata.MessaggioID.Valid || inviata.MessaggioID.UUID != b.messaggio(buona).MessaggioID {
		t.Fatalf("(a) la richiesta non è passata a inviata con la sua mail: %+v", inviata)
	}

	// (b) la risposta del fornitore porta lo stesso marcatore
	const entrata = "<i5-entrata@mgm.example>"
	x := b.rispostaDelFornitore(entrata, "CONV-I5-B", "info@mgm.example", "R: RFQ ACME 6674611A", "In allegato l'offerta.")
	x.Marcatori = map[string]string{MarcatoreRichiesta: r.RichiestaID.String()}
	b.ingerisci(x)
	b.nessunLegame(entrata, "(b) marcatore su una mail in entrata")
	if dopo := b.statoRichiesta(r.RichiestaID); dopo.MessaggioID != inviata.MessaggioID || dopo.Stato != inviata.Stato || dopo.OffertaRicevutaIl != nil {
		t.Errorf("(b) la mail in entrata con il marcatore ha toccato la richiesta: %+v", dopo)
	}

	// (c) marcatori rotti: uno che non è un uuid, uno che punta a niente
	const rotto, nulla = "<i5-rotto@azienda.it>", "<i5-nulla@azienda.it>"
	m1 := b.nostraMail(rotto, "CONV-I5-C1", "info@mgm.example", "RFQ", "corpo")
	m1.Marcatori = map[string]string{MarcatoreRichiesta: "non-un-uuid"}
	m2 := b.nostraMail(nulla, "CONV-I5-C2", "info@mgm.example", "RFQ", "corpo")
	m2.Marcatori = map[string]string{MarcatoreRichiesta: uuid.New().String()}
	b.ingerisci(m1, m2)
	b.nessunLegame(rotto, "(c) marcatore che non è un uuid")
	b.nessunLegame(nulla, "(c) marcatore verso una richiesta inesistente")

	// (d) la nostra mail è già stata messa a mano nella RFQ B; al risync arriva con il marcatore
	// della richiesta di A (un marcatore rimasto su una bozza riusata, per esempio)
	const decisa = "<i5-decisa@azienda.it>"
	b.ingerisci(b.nostraMail(decisa, "CONV-I5-D", "info@mgm.example", "RFQ", "corpo"))
	if _, err := b.pool.Exec(b.ctx, `UPDATE messaggio SET thread_id = $1, aggancio = 'operatore' WHERE chiave_esterna = $2`, thB.ThreadID, decisa); err != nil {
		t.Fatal(err)
	}
	r2 := b.richiesta(thA, mgm, db.StatoRichiestaFornitoreBozza, uuid.NullUUID{}, "tornitura")
	ri := b.nostraMail(decisa, "CONV-I5-D", "info@mgm.example", "RFQ", "corpo")
	ri.Marcatori = map[string]string{MarcatoreRichiesta: r2.RichiestaID.String()}
	b.ingerisci(ri)
	tid, rid, _, _ = b.legami(decisa)
	if !tid.Valid || tid.UUID != thB.ThreadID {
		t.Errorf("(d) il risync con il marcatore ha spostato un messaggio già deciso: thread %v, attesa la RFQ B", tid)
	}
	if rid.Valid {
		t.Errorf("(d) il messaggio deciso nella RFQ B è stato legato a una richiesta della RFQ A: %v", rid)
	}
	if dopo := b.statoRichiesta(r2.RichiestaID); dopo.Stato != db.StatoRichiestaFornitoreBozza || dopo.MessaggioID.Valid {
		t.Errorf("(d) la richiesta della RFQ A ha preso una mail che sta nella RFQ B: %+v", dopo)
	}
}
