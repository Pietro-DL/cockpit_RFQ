package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/inbox/ingest"
	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/platform/contratti/api"
	"promatec/cockpit/internal/platform/db"
)

// LE RICHIESTE AI FORNITORI (blocco 7B)
//
// Dalla pagina della RFQ si crea una richiesta a un fornitore (chi, quale lavorazione, quali
// codici) e, se si vuole, la bozza in Outlook che la porta: sulla bozza il worker scrive il marcatore
// `CockpitRichiestaFornitore`, e quando la mail compare nella Posta inviata il sync la lega alla
// richiesta e la aggancia alla RFQ senza indovinare (IB2). Se la mail è stata mandata a mano, il
// triage propone «richiesta a X per la RFQ Y» e l'operatore conferma (IB3). Quando il fornitore
// risponde, i candidati (R0, R1, R3f) dicono a quale richiesta, e l'operatore conferma: la mail
// entra nella RFQ cliente, la richiesta passa a `risposta`, gli allegati vengono proposti come
// `offerta_fornitore` (IB4, IB5).
//
// Nessuna di queste scritture avviene da sola: sono tutte dietro un pulsante.

// ---------------------------------------------------------------- dalla RFQ: nuova richiesta

func (s *Server) nuovaRichiestaFornitore(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ctx := r.Context()
	u := utenteDa(ctx)
	sess := sessioneDa(ctx)
	q := db.New(s.Pool)
	t, err := q.GetThread(ctx, id)
	if err != nil {
		http.Error(w, "thread non trovato", 404)
		return
	}
	fid, err := uuid.Parse(r.FormValue("fornitore_id"))
	if err != nil {
		s.threadFrammento(w, r, id, "Scegli un fornitore.")
		return
	}
	f, err := q.GetFornitore(ctx, fid)
	if err != nil {
		s.threadFrammento(w, r, id, "Fornitore non trovato.")
		return
	}
	var lav pgtype.Text
	if l := strings.TrimSpace(r.FormValue("lavorazione")); l != "" {
		lav = pgtype.Text{String: l, Valid: true}
	}
	codici := splitCodici(strings.Join(r.Form["codice"], " "))
	if codici == nil {
		codici = []string{}
	}
	ric, err := q.InsertRichiestaFornitore(ctx, db.InsertRichiestaFornitoreParams{
		ThreadID: id, FornitoreID: fid, Lavorazione: lav, Codici: codici, Note: txtN(r.FormValue("note"), 2000),
		CreataDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true}, Stato: db.StatoRichiestaFornitoreBozza,
	})
	if err != nil {
		if _, dup := vincoloViolato(err); dup {
			err = fmt.Errorf("esiste già una richiesta a %s per questa RFQ e questa lavorazione", f.RagioneSociale)
		}
		if vincoloEsterno(err) {
			err = fmt.Errorf("la lavorazione %q non esiste nel catalogo", lav.String)
		}
		s.threadFrammento(w, r, id, "Richiesta non creata: "+err.Error())
		return
	}
	s.Log.Info("richiesta a fornitore creata", "thread", id, "fornitore", f.RagioneSociale, "lavorazione", lav.String, "codici", codici, "utente", siglaDa(r))
	avviso := fmt.Sprintf("Richiesta a %s creata.", f.RagioneSociale)
	if r.FormValue("bozza") == "1" {
		avviso += " " + s.bozzaPerRichiesta(ctx, u, sess, t, f, ric)
	} else {
		avviso += " Quando la mail parte da Outlook, il sync la lega da solo se ha il marcatore; altrimenti confermala dall'Inbox."
	}
	s.threadFrammento(w, r, id, avviso)
}

// bozzaPerRichiesta prepara la bozza «nuovo» in Outlook con il marcatore della richiesta. La bozza si
// apre sul PC della sessione, dalla casella in cui la RFQ è arrivata: stessa regola di «Rispondi». La
// richiesta esiste comunque: se la bozza non parte (capacità `bozze` spenta, nessuna postazione), la
// frase lo dice e la mail si può scrivere a mano.
func (s *Server) bozzaPerRichiesta(ctx context.Context, u *db.Utente, sess sessioneUI, t db.ThreadOfferta, f db.Fornitore, ric db.RichiestaFornitore) string {
	q := db.New(s.Pool)
	// la copia: il primo messaggio della RFQ servito dalla postazione della sessione
	msgs, _ := q.ListMessaggiThread(ctx, uuid.NullUUID{UUID: t.ThreadID, Valid: true})
	var copia *jobs.Copia
	var motivo string
	var origine db.Messaggio
	for _, m := range msgs {
		if c, mot := s.copiaInterattiva(ctx, q, m.MessaggioID, sess); c != nil {
			copia, origine = c, m
			break
		} else if motivo == "" {
			motivo = mot
		}
	}
	if copia == nil {
		if motivo == "" {
			motivo = "la RFQ non ha messaggi in una casella servita da questa postazione"
		}
		return "Bozza non preparata: " + motivo + ". Scrivila a mano: al sync la si riconosce dal codice, e si conferma dall'Inbox."
	}
	cl, _ := q.GetCliente(ctx, t.ClienteID)
	buyer := ""
	if t.BuyerID.Valid {
		if b, err := q.GetBuyer(ctx, t.BuyerID.UUID); err == nil {
			buyer = b.Cognome
		}
	}
	oggetto := oggettoRichiesta(cl.CartellaNas, buyer, ric.Codici)
	corpo := corpoRichiesta(ric, f)
	contatti, _ := q.ListContattiFornitore(ctx, f.FornitoreID)
	var dest []api.Destinatario
	for _, c := range contatti {
		dest = append(dest, api.Destinatario{Nome: c.Nome.String, Indirizzo: c.Email, Tipo: "a"})
	}
	if dest == nil {
		dest = []api.Destinatario{}
	}
	destJSON, _ := json.Marshal(dest)

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "Bozza non preparata: " + err.Error()
	}
	defer tx.Rollback(ctx)
	qt := db.New(tx)
	b, err := qt.InsertBozzaRichiesta(ctx, db.InsertBozzaRichiestaParams{
		ThreadID: uuid.NullUUID{UUID: t.ThreadID, Valid: true}, Destinatari: destJSON,
		Oggetto: pgtype.Text{String: oggetto, Valid: true}, Corpo: pgtype.Text{String: corpo, Valid: true},
		CreataDa: u.UtenteID, RichiestaFornitoreID: uuid.NullUUID{UUID: ric.RichiestaID, Valid: true},
	})
	if err != nil {
		return "Bozza non preparata: " + err.Error()
	}
	html := "<div style=\"font-family:Calibri,sans-serif;font-size:11pt\">" + strings.ReplaceAll(template.HTMLEscapeString(corpo), "\n", "<br>") + "</div><br>"
	if _, err := jobs.AccodaCon(ctx, qt, db.TipoJobCreaBozzaOutlook, api.PayloadCreaBozza{
		BozzaID: b.BozzaID, Tipo: string(db.TipoBozzaNuovo), Destinatari: dest, Oggetto: oggetto,
		CorpoHTML: html, CorpoTesto: corpo, Allegati: []string{}, Mostra: true, Invia: false,
		Marcatori:           map[string]string{ingest.MarcatoreRichiesta: ric.RichiestaID.String()},
		RiferimentoElemento: rifIn(origine, copia.CasellaID),
	}, "bozza:"+b.BozzaID.String(), 1, jobs.OpzioniInterattive(db.TipoJobCreaBozzaOutlook, *copia, sess.Postazione, u.UtenteID)); err != nil {
		if errors.Is(err, jobs.ErrCapacitaSpenta) {
			return "Bozza non preparata: " + motivoCapacita(err, db.TipoJobCreaBozzaOutlook) + ". La richiesta è creata: scrivi la mail a mano e confermala dall'Inbox."
		}
		return "Bozza non preparata: " + err.Error()
	}
	if err := tx.Commit(ctx); err != nil {
		return "Bozza non preparata: " + err.Error()
	}
	if len(dest) == 0 {
		return fmt.Sprintf("Bozza in preparazione su %s, senza destinatari: %s non ha contatti censiti, scrivi tu l'indirizzo. Rileggi e premi Invia in Outlook.", sess.NomeHost, f.RagioneSociale)
	}
	return fmt.Sprintf("Bozza in preparazione: si apre in Outlook su %s tra pochi secondi, a %s. Rileggi e premi Invia lì.", sess.NomeHost, dest[0].Indirizzo)
}

// oggettoRichiesta è la convenzione «RFQ <cliente> <buyer> <codici>» (7B.2): è ciò che la regola
// RF_oggetto ritrova quando la mail viene mandata a mano, ed è leggibile da chi la riceve.
func oggettoRichiesta(cartella, buyer string, codici []string) string {
	parti := []string{"RFQ", cartella}
	if buyer != "" {
		parti = append(parti, buyer)
	}
	parti = append(parti, codici...)
	return strings.TrimSpace(strings.Join(parti, " "))
}

func corpoRichiesta(ric db.RichiestaFornitore, f db.Fornitore) string {
	var b strings.Builder
	b.WriteString("Buongiorno,\n\nvi chiediamo la vostra migliore offerta")
	if ric.Lavorazione.Valid {
		b.WriteString(" per la lavorazione «" + ric.Lavorazione.String + "»")
	}
	if len(ric.Codici) > 0 {
		b.WriteString(" sui codici: " + strings.Join(ric.Codici, ", "))
	}
	b.WriteString(".\n")
	if ric.Note.Valid && strings.TrimSpace(ric.Note.String) != "" {
		b.WriteString("\n" + strings.TrimSpace(ric.Note.String) + "\n")
	}
	b.WriteString("\nRestiamo in attesa di un vostro riscontro.\n")
	return b.String()
}

func (s *Server) annullaRichiestaFornitore(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	rid, err2 := uuid.Parse(r.PathValue("rid"))
	if err != nil || err2 != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	n, err := db.New(s.Pool).SetRichiestaStato(r.Context(), db.SetRichiestaStatoParams{RichiestaID: rid, Stato: db.StatoRichiestaFornitoreAnnullata, ThreadID: id})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if n == 0 {
		s.threadFrammento(w, r, id, "Richiesta non trovata in questa RFQ.")
		return
	}
	s.Log.Info("richiesta a fornitore annullata", "richiesta", rid, "utente", siglaDa(r))
	s.threadFrammento(w, r, id, "Richiesta annullata. La mail, se è partita, resta nel suo quadrante.")
}

// ---------------------------------------------------------------- dall'Inbox: le conferme

// rispostaFornitore: «è la risposta a questa richiesta». La mail entra nella RFQ cliente (stesso
// aggancio di «Aggancia a…», con il suo log) e viene legata alla richiesta.
//
// Lo STATO della richiesta cambia solo con l'atto che l'operatore conferma (7C.0, invariante 8):
// `offerta` → offerta_ricevuta, e gli allegati ancora da smistare vengono proposti come offerta del
// fornitore; `declinata` → il fornitore non quota. Senza atto (o «collegata») la mail è dentro e
// la richiesta resta `inviata`: «ricevuto, vi rispondiamo domani» non chiude niente. Fino alla
// 0015 qualunque aggancio la chiudeva.
func (s *Server) rispostaFornitore(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	rid, err := uuid.Parse(r.FormValue("richiesta_id"))
	if err != nil {
		s.pannelloConAvviso(w, r, id, "Scegli una richiesta.")
		return
	}
	u := utenteDa(r.Context())
	ctx := r.Context()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	m, deciso, err := messaggioDaDecidere(ctx, q, id)
	if err != nil {
		http.Error(w, "messaggio non trovato", 404)
		return
	}
	ric, err := q.GetRichiesta(ctx, rid)
	if err != nil {
		s.pannelloConAvviso(w, r, id, "Richiesta non trovata.")
		return
	}
	if deciso && m.ThreadID.UUID != ric.ThreadID {
		s.avvisoRFQEsistente(w, r, ctx, q, m)
		return
	}
	if !deciso {
		if err := s.agganciaMessaggioAThread(ctx, q, u, m, ric.ThreadID, uuid.NullUUID{}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	if err := q.AgganciaRispostaFornitore(ctx, db.AgganciaRispostaFornitoreParams{MessaggioID: id, RichiestaFornitoreID: uuid.NullUUID{UUID: rid, Valid: true}}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	atto := r.FormValue("atto")
	var n int64
	switch atto {
	case "offerta":
		if _, err := q.SetRichiestaOffertaRicevuta(ctx, db.SetRichiestaOffertaRicevutaParams{RichiestaID: rid, OffertaRicevutaIl: &m.DataEvento}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if n, err = q.RiproponiAllegatiComeOffertaFornitore(ctx, id); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	case "declinata":
		if _, err := q.SetRichiestaDeclinata(ctx, db.SetRichiestaDeclinataParams{RichiestaID: rid, DeclinataIl: &m.DataEvento}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	case "", "collegata":
		// solo il legame: la richiesta resta com'è
	default:
		s.pannelloConAvviso(w, r, id, "Atto non previsto: "+atto+".")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	f, _ := db.New(s.Pool).GetFornitore(ctx, ric.FornitoreID)
	t, _ := db.New(s.Pool).GetThread(ctx, ric.ThreadID)
	s.Log.Info("risposta di fornitore agganciata", "messaggio", id, "richiesta", rid, "fornitore", f.RagioneSociale, "atto", primoNonVuoto(atto, "collegata"), "allegati_offerta", n, "utente", siglaDa(r))
	frase := fmt.Sprintf("Agganciata come risposta di %s alla richiesta per la RFQ %s.", f.RagioneSociale, t.CartellaRelativa.String)
	switch atto {
	case "offerta":
		frase += " Offerta ricevuta: la richiesta è chiusa."
		if n > 0 {
			frase += fmt.Sprintf(" %d allegat%s propost%s come offerta del fornitore: confermali per portarli in OFFERTE FORNITORI.", n, plurale(int(n), "o", "i"), plurale(int(n), "o", "i"))
		}
	case "declinata":
		frase += " Il fornitore non quota: richiesta declinata."
	default:
		frase += " La richiesta resta aperta: quando arriva l'offerta, confermala come tale."
	}
	s.pannelloConAvviso(w, r, id, frase)
}

func primoNonVuoto(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}

// richiestaFornitoreManuale: «sì, è la richiesta a X per la RFQ Y» su una nostra mail mandata a
// mano. Nasce la richiesta (già `inviata`, con questa mail), la mail entra nella RFQ. Mai un thread
// nuovo (IB3).
func (s *Server) richiestaFornitoreManuale(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	tid, err1 := uuid.Parse(r.FormValue("thread_id"))
	fid, err2 := uuid.Parse(r.FormValue("fornitore_id"))
	if err1 != nil || err2 != nil {
		s.pannelloConAvviso(w, r, id, "Servono la RFQ e il fornitore.")
		return
	}
	u := utenteDa(r.Context())
	ctx := r.Context()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	m, deciso, err := messaggioDaDecidere(ctx, q, id)
	if err != nil {
		http.Error(w, "messaggio non trovato", 404)
		return
	}
	if deciso && m.ThreadID.UUID != tid {
		s.avvisoRFQEsistente(w, r, ctx, q, m)
		return
	}
	t, err := q.GetThread(ctx, tid)
	if err != nil {
		s.pannelloConAvviso(w, r, id, "RFQ non trovata.")
		return
	}
	f, err := q.GetFornitore(ctx, fid)
	if err != nil {
		s.pannelloConAvviso(w, r, id, "Fornitore non trovato.")
		return
	}
	var lav pgtype.Text
	if l := strings.TrimSpace(r.FormValue("lavorazione")); l != "" {
		lav = pgtype.Text{String: l, Valid: true}
	}
	codici := []string{}
	if p, err := q.GetTriageMessaggio(ctx, id); err == nil && len(p.Identificativi) > 0 {
		codici = p.Identificativi
	}
	ric, err := q.InsertRichiestaFornitore(ctx, db.InsertRichiestaFornitoreParams{
		ThreadID: tid, FornitoreID: fid, Lavorazione: lav, Codici: codici, Note: pgtype.Text{},
		CreataDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true}, Stato: db.StatoRichiestaFornitoreInviata,
		MessaggioID: uuid.NullUUID{UUID: id, Valid: true}, InviataIl: &m.DataEvento,
	})
	if err != nil {
		if _, dup := vincoloViolato(err); dup {
			err = fmt.Errorf("esiste già una richiesta a %s per questa RFQ e questa lavorazione: se questa mail è quella, agganciala a mano alla RFQ", f.RagioneSociale)
		}
		s.pannelloConAvviso(w, r, id, "Richiesta non creata: "+err.Error())
		return
	}
	if !deciso {
		if err := s.agganciaMessaggioAThread(ctx, q, u, m, tid, uuid.NullUUID{}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	if err := q.AgganciaRispostaFornitore(ctx, db.AgganciaRispostaFornitoreParams{MessaggioID: id, RichiestaFornitoreID: uuid.NullUUID{UUID: ric.RichiestaID, Valid: true}}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.Log.Info("richiesta a fornitore confermata da una mail mandata a mano", "messaggio", id, "richiesta", ric.RichiestaID, "fornitore", f.RagioneSociale, "thread", tid, "utente", siglaDa(r))
	s.pannelloConAvviso(w, r, id, fmt.Sprintf("Registrata come richiesta a %s per la RFQ %s. La risposta del fornitore verrà proposta su questa richiesta.", f.RagioneSociale, t.CartellaRelativa.String))
}
