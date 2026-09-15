package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/domain"
	"promatec/cockpit/internal/jobs"
)

// Blocco 4 — triage in UI (SPEC schermata A): Nuova RFQ / Aggancia a… / Ignora.
// Il triage deterministico propone; qui l'operatore decide, e ogni decisione è una riga in DB.

type triageDati struct {
	M              db.Messaggio
	Riga           db.VInbox
	Azione         string // nuova | aggancia
	Clienti        []db.Cliente
	ClienteID      uuid.NullUUID
	Buyers         []db.Buyer
	BuyerID        uuid.NullUUID
	Nome, Cognome  string
	Email, Dominio string
	Oggetto        string
	Identificativi string
	Scadenza       string
	Allegati       []AllegatoUI
	Anteprima      string
	Errore         string
}

// triageForm prepara il form con tutto precompilato da mittente, triage deterministico e allegati.
func (s *Server) triageForm(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	ctx := r.Context()
	q := db.New(s.Pool)
	d, err := s.datiTriage(ctx, q, id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	d.Azione = r.URL.Query().Get("azione")
	if d.Azione != "aggancia" {
		d.Azione = "nuova"
	}
	s.frammento(w, "triage_form", d)
}

func (s *Server) datiTriage(ctx context.Context, q *db.Queries, id uuid.UUID) (*triageDati, error) {
	m, err := q.GetMessaggio(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &triageDati{M: m, Oggetto: domain.OggettoPulito(m.Oggetto.String)}
	d.Riga, _ = q.GetInboxRiga(ctx, id)
	d.Clienti, _ = q.ListClienti(ctx)
	d.ClienteID = d.Riga.ClienteID
	d.Email = strings.ToLower(m.MittenteIndirizzo.String)
	if i := strings.LastIndex(d.Email, "@"); i > 0 {
		d.Dominio = d.Email[i+1:]
	}
	d.Nome, d.Cognome = domain.NomeCognome(m.MittenteNome.String, d.Email)
	if m.BuyerID.Valid {
		d.BuyerID = m.BuyerID
	}
	if d.ClienteID.Valid {
		d.Buyers, _ = q.ListBuyerCliente(ctx, d.ClienteID.UUID)
	}
	if tr, err := q.GetTriageMessaggio(ctx, id); err == nil {
		d.Identificativi = strings.Join(tr.Identificativi, ", ")
		if tr.ScadenzaProposta != nil {
			d.Scadenza = tr.ScadenzaProposta.Format("2006-01-02")
		}
		if !d.BuyerID.Valid && tr.BuyerProposto.Valid {
			d.BuyerID = tr.BuyerProposto
		}
	}
	d.Allegati, _ = s.allegatiUI(ctx, q, id)
	cartellaCliente := "<CLIENTE>"
	for _, c := range d.Clienti {
		if d.ClienteID.Valid && c.ClienteID == d.ClienteID.UUID {
			cartellaCliente = c.CartellaNas
		}
	}
	d.Anteprima = domain.CartellaThread(cartellaCliente, m.DataEvento.Local(), d.Cognome, d.Oggetto)
	return d, nil
}

// buyerSelect è il frammento ricaricato quando cambia il cliente nel form.
func (s *Server) buyerSelect(w http.ResponseWriter, r *http.Request) {
	d := &triageDati{}
	if cid, err := uuid.Parse(r.URL.Query().Get("cliente_id")); err == nil {
		d.ClienteID = uuid.NullUUID{UUID: cid, Valid: true}
		d.Buyers, _ = db.New(s.Pool).ListBuyerCliente(r.Context(), cid)
	}
	s.frammento(w, "buyer_select", d)
}

// cercaThread è la ricerca incrementale di "Aggancia a…" (oggetto, cliente, codici) sui thread aperti.
func (s *Server) cercaThread(w http.ResponseWriter, r *http.Request) {
	righe, err := db.New(s.Pool).CercaThreadAperti(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.frammento(w, "thread_risultati", righe)
}

// ---------------------------------------------------------------- decisioni

// messaggioDaDecidere apre la decisione su un messaggio: lo blocca per la durata della transazione e
// dice se qualcuno ha già deciso.
//
// Ogni percorso che decide passa di qui — nuova RFQ, aggancio a una RFQ esistente, ignora — e non
// perché faccia risparmiare righe: perché il blocco è la garanzia, e una garanzia ripetuta in tre
// punti è una garanzia che prima o poi resta in due. Il secondo valore è true quando il messaggio
// risulta già agganciato: chi lo riceve deve dare un esito esplicito, mai proseguire (T13).
func messaggioDaDecidere(ctx context.Context, q *db.Queries, id uuid.UUID) (db.Messaggio, bool, error) {
	m, err := q.BloccaMessaggio(ctx, id)
	if err != nil {
		return m, false, err
	}
	return m, m.ThreadID.Valid, nil
}

// nuovaRFQ: una transazione che crea (se serve) cliente e buyer, il thread con la sua cartella, gli identificativi,
// la fase RICEVUTA, aggancia messaggio e conversazione, chiude il triage e accoda cartella + download richiesti.
func (s *Server) nuovaRFQ(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
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
	// BloccaMessaggio, non GetMessaggio: da qui in poi si decide, e la decisione deve essere una sola
	// (voce 1.9, T13). Un secondo operatore che preme "Nuova RFQ" sullo stesso messaggio si ferma qui
	// finché questa transazione non ha finito, poi rilegge e trova il messaggio già agganciato.
	m, deciso, err := messaggioDaDecidere(ctx, q, id)
	if err != nil {
		http.Error(w, "messaggio non trovato", 404)
		return
	}
	if deciso {
		// Esito esplicito, non silenzio: chi ha perso la corsa deve sapere dov'è finito il messaggio,
		// altrimenti riprova e crea la seconda RFQ a mano.
		s.avvisoRFQEsistente(w, r, ctx, q, m)
		return
	}

	cliente, err := s.clienteDaForm(ctx, q, r)
	if err != nil {
		s.triageErrore(w, r, id, "nuova", err.Error())
		return
	}
	buyer, err := s.buyerDaForm(ctx, q, r, cliente)
	if err != nil {
		s.triageErrore(w, r, id, "nuova", err.Error())
		return
	}
	oggetto := strings.TrimSpace(r.FormValue("oggetto"))
	if oggetto == "" {
		oggetto = domain.OggettoPulito(m.Oggetto.String)
	}
	var buyerID uuid.NullUUID
	cognome := ""
	if buyer != nil {
		buyerID = uuid.NullUUID{UUID: buyer.BuyerID, Valid: true}
		cognome = buyer.Cognome
	}
	var scadenza *time.Time
	var origine db.NullScadenzaOrigine
	if sc := r.FormValue("scadenza"); sc != "" {
		if d, err := time.ParseInLocation("2006-01-02", sc, time.Local); err == nil {
			scadenza = &d
			origine = db.NullScadenzaOrigine{ScadenzaOrigine: db.ScadenzaOrigineMail, Valid: true}
		}
	}
	prio, _ := strconv.Atoi(r.FormValue("priorita"))
	if prio < 1 || prio > 3 {
		prio = 1
	}
	t, err := q.InsertThread(ctx, db.InsertThreadParams{
		ClienteID: cliente.ClienteID, BuyerID: buyerID, Canale: m.Canale, DataInizio: m.DataEvento, DataScadenza: scadenza, ScadenzaOrigine: origine,
		Oggetto: ptxt(oggetto), CartellaRelativa: ptxt(domain.CartellaThread(cliente.CartellaNas, m.DataEvento.Local(), cognome, oggetto)),
		Priorita: int16(prio), Campionatura: r.FormValue("campionatura") == "1", Note: ptxt(r.FormValue("note")), CreatoDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true},
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	for _, c := range splitCodici(r.FormValue("identificativi")) {
		if _, err := q.UpsertIdentificativo(ctx, db.UpsertIdentificativoParams{ThreadID: t.ThreadID, Codice: c, Origine: db.OrigineIdentificativoManuale,
			Confidenza: pgtype.Int2{Int16: 100, Valid: true}, ConfermatoDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true}}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	if _, err := q.ApriFase(ctx, db.ApriFaseParams{ThreadID: t.ThreadID, NomeFase: db.FaseRICEVUTA, Inizio: m.DataEvento,
		ResponsabileID: uuid.NullUUID{UUID: u.UtenteID, Valid: true}}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := s.agganciaMessaggioAThread(ctx, q, u, m, t.ThreadID, buyerID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if _, err := jobs.Accoda(ctx, q, db.TipoJobCreaCartellaThread, api.PayloadCreaCartellaThread{ThreadID: t.ThreadID}, "cartella:"+t.ThreadID.String(), 1); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	esiti, err := s.downloadDaForm(ctx, q, m, r)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.pannelloConAvviso(w, r, id, fmt.Sprintf("RFQ creata: %s. Cartella in creazione. %s", t.CartellaRelativa.String, esiti.frase()))
}

// agganciaEsistente collega il messaggio (e gli orfani della sua conversazione) a un thread scelto dall'operatore.
func (s *Server) agganciaEsistente(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	tid, err := uuid.Parse(r.FormValue("thread_id"))
	if err != nil {
		s.triageErrore(w, r, id, "aggancia", "Scegli una RFQ dall'elenco.")
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
		s.triageErrore(w, r, id, "aggancia", "RFQ non trovata.")
		return
	}
	// mittente non ancora censito come buyer del cliente del thread → lo si crea se richiesto
	var buyerID uuid.NullUUID
	if r.FormValue("crea_buyer") == "1" {
		cl, _ := q.GetCliente(ctx, t.ClienteID)
		if b, err := s.buyerDaForm(ctx, q, r, &cl); err == nil && b != nil {
			buyerID = uuid.NullUUID{UUID: b.BuyerID, Valid: true}
		}
	}
	if err := s.agganciaMessaggioAThread(ctx, q, u, m, tid, buyerID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	esiti, err := s.downloadDaForm(ctx, q, m, r)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.pannelloConAvviso(w, r, id, fmt.Sprintf("Agganciato alla RFQ %s. %s", t.CartellaRelativa.String, esiti.frase()))
}

// ignora chiude il triage senza RFQ: il messaggio esce da "orfani" e finisce nel filtro "ignorati".
func (s *Server) ignora(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
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
	if deciso {
		s.avvisoRFQEsistente(w, r, ctx, q, m)
		return
	}
	if err := q.IgnoraMessaggio(ctx, db.IgnoraMessaggioParams{MessaggioID: id, DecisoDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true}}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := logDecisione(ctx, q, id, uuid.NullUUID{}, "ignora", u, "chiuso senza RFQ"); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.pannelloConAvviso(w, r, id, "Messaggio ignorato (lo ritrovi nel filtro «ignorati»).")
}

// avvisoRFQEsistente è l'esito esplicito di chi ha perso una corsa: dice dov'è finito il messaggio,
// invece di lasciare l'operatore davanti a un pannello che non è cambiato (T13).
func (s *Server) avvisoRFQEsistente(w http.ResponseWriter, r *http.Request, ctx context.Context, q *db.Queries, m db.Messaggio) {
	dove := "un'altra RFQ"
	if t, err := q.GetThread(ctx, m.ThreadID.UUID); err == nil && t.CartellaRelativa.Valid {
		dove = t.CartellaRelativa.String
	}
	s.pannelloConAvviso(w, r, m.MessaggioID, fmt.Sprintf(
		"Nel frattempo il messaggio è stato agganciato a %s: non ne è stata creata una seconda. Ricarica per vedere la RFQ.", dove))
}

// logDecisione scrive in messaggio_aggancio_log. Ogni decisione di aggancio lascia una traccia con chi
// l'ha presa e perché: è il materiale con cui, mesi dopo, si ricostruisce come un messaggio sia
// arrivato dov'è — e, dalla fase 3, la base della propagazione ai messaggi annidati.
func logDecisione(ctx context.Context, q *db.Queries, messaggioID uuid.UUID, threadID uuid.NullUUID, azione string, u *db.Utente, motivo string) error {
	var utente uuid.NullUUID
	if u != nil {
		utente = uuid.NullUUID{UUID: u.UtenteID, Valid: true}
	}
	return q.InsertAgganciaLog(ctx, db.InsertAgganciaLogParams{
		MessaggioID: messaggioID, ThreadID: threadID, Azione: azione, UtenteID: utente,
		Motivo: pgtype.Text{String: motivo, Valid: motivo != ""},
	})
}

// ---------------------------------------------------------------- pezzi della transazione

// agganciaMessaggioAThread: la DECISIONE di aggancio, con tutto ciò che la segue (conversazione, orfani della
// stessa conversazione, proposte e riferimenti portale del messaggio, chiusura del triage, buyer).
func (s *Server) agganciaMessaggioAThread(ctx context.Context, q *db.Queries, u *db.Utente, m db.Messaggio, threadID uuid.UUID, buyerID uuid.NullUUID) error {
	tid := uuid.NullUUID{UUID: threadID, Valid: true}
	op := uuid.NullUUID{UUID: u.UtenteID, Valid: true}
	if err := q.AgganciaMessaggio(ctx, db.AgganciaMessaggioParams{MessaggioID: m.MessaggioID, ThreadID: tid, Aggancio: db.AggancioOperatore, AgganciatoDa: op}); err != nil {
		return err
	}
	if err := logDecisione(ctx, q, m.MessaggioID, tid, "aggancia", u, "decisione dell'operatore"); err != nil {
		return err
	}
	if _, err := q.AssegnaThreadProposte(ctx, db.AssegnaThreadProposteParams{MessaggioID: m.MessaggioID, ThreadID: tid}); err != nil {
		return err
	}
	if _, err := q.AssegnaThreadRiferimenti(ctx, db.AssegnaThreadRiferimentiParams{MessaggioID: m.MessaggioID, ThreadID: tid}); err != nil {
		return err
	}
	if _, err := q.DecidiTriage(ctx, db.DecidiTriageParams{MessaggioID: m.MessaggioID, Stato: db.StatoTriageAccettata, DecisoDa: op}); err != nil {
		return err
	}
	conv, err := q.GetConversazione(ctx, m.ConversazioneID)
	if err == nil && !conv.ThreadID.Valid {
		if err := q.CollegaConversazione(ctx, db.CollegaConversazioneParams{ConversazioneID: conv.ConversazioneID, ThreadID: tid, CollegataDa: db.AggancioOperatore}); err != nil {
			return err
		}
	}
	// gli altri messaggi orfani della stessa conversazione (es. la prima mail se si parte dalla risposta) seguono
	altri, err := q.AgganciaOrfaniConversazione(ctx, db.AgganciaOrfaniConversazioneParams{ConversazioneID: m.ConversazioneID, ThreadID: tid, MessaggioID: m.MessaggioID})
	if err != nil {
		return err
	}
	for _, mid := range altri {
		_, _ = q.AssegnaThreadProposte(ctx, db.AssegnaThreadProposteParams{MessaggioID: mid, ThreadID: tid})
		_, _ = q.AssegnaThreadRiferimenti(ctx, db.AssegnaThreadRiferimentiParams{MessaggioID: mid, ThreadID: tid})
		_, _ = q.DecidiTriage(ctx, db.DecidiTriageParams{MessaggioID: mid, Stato: db.StatoTriageAccettata, DecisoDa: op})
		// l'operatore ha deciso su uno solo: gli altri sono stati trascinati dalla conversazione, e la
		// differenza va registrata, altrimenti sembrerebbero decisioni prese una per una
		if err := logDecisione(ctx, q, mid, tid, "propaga", u, "orfano della stessa conversazione"); err != nil {
			return err
		}
	}
	if buyerID.Valid {
		if !m.BuyerID.Valid {
			_ = q.SetBuyerMessaggio(ctx, db.SetBuyerMessaggioParams{MessaggioID: m.MessaggioID, BuyerID: buyerID})
		}
		_ = q.SetBuyerThread(ctx, db.SetBuyerThreadParams{ThreadID: threadID, BuyerID: buyerID})
	}
	return nil
}

// clienteDaForm: cliente scelto dalla select, oppure creato inline (ragione sociale + cartella NAS + dominio).
func (s *Server) clienteDaForm(ctx context.Context, q *db.Queries, r *http.Request) (*db.Cliente, error) {
	sel := r.FormValue("cliente_id")
	if sel != "" && sel != "__nuovo__" {
		cid, err := uuid.Parse(sel)
		if err != nil {
			return nil, errors.New("cliente non valido")
		}
		c, err := q.GetCliente(ctx, cid)
		if err != nil {
			return nil, errors.New("cliente non trovato")
		}
		return &c, nil
	}
	nome := strings.TrimSpace(r.FormValue("cliente_nome"))
	cartella := domain.NomeSicuro(strings.ToUpper(strings.TrimSpace(r.FormValue("cliente_cartella"))), 80)
	if nome == "" || cartella == "" || cartella == "senza nome" {
		return nil, errors.New("per un cliente nuovo servono ragione sociale e nome della cartella NAS")
	}
	c, err := q.UpsertCliente(ctx, db.UpsertClienteParams{CartellaNas: cartella, RagioneSociale: nome, Regole: []byte("{}")})
	if err != nil {
		return nil, fmt.Errorf("cliente: %w", err)
	}
	if dom := strings.ToLower(strings.TrimSpace(r.FormValue("cliente_dominio"))); dom != "" && !dominioPubblico(dom) {
		if err := q.UpsertDominioCliente(ctx, db.UpsertDominioClienteParams{Lower: dom, ClienteID: c.ClienteID}); err != nil {
			return nil, fmt.Errorf("dominio: %w", err)
		}
	}
	return &c, nil
}

// buyerDaForm: buyer scelto dalla select, creato inline, oppure nessuno.
func (s *Server) buyerDaForm(ctx context.Context, q *db.Queries, r *http.Request, cliente *db.Cliente) (*db.Buyer, error) {
	sel := r.FormValue("buyer_id")
	switch sel {
	case "", "__nessuno__":
		return nil, nil
	case "__nuovo__":
		cognome := strings.TrimSpace(r.FormValue("buyer_cognome"))
		if cognome == "" {
			return nil, errors.New("per un buyer nuovo serve almeno il cognome")
		}
		email := strings.ToLower(strings.TrimSpace(r.FormValue("buyer_email")))
		if email != "" {
			if b, err := q.GetBuyerPerEmail(ctx, email); err == nil {
				if b.ClienteID != cliente.ClienteID {
					return nil, fmt.Errorf("l'indirizzo %s è già censito per un altro cliente", email)
				}
				return &b, nil
			}
		}
		b, err := q.InsertBuyer(ctx, db.InsertBuyerParams{ClienteID: cliente.ClienteID, Cognome: cognome, Nome: ptxt(r.FormValue("buyer_nome")), Email: ptxt(email),
			Telefono: ptxt(r.FormValue("buyer_telefono")), Tipo: db.TipoBuyerBuyer, Origine: db.OrigineAnagraficaCensimento})
		if err != nil {
			return nil, fmt.Errorf("buyer: %w", err)
		}
		if email != "" {
			_, _ = q.SetBuyerMessaggiPerIndirizzo(ctx, db.SetBuyerMessaggiPerIndirizzoParams{Lower: email, BuyerID: uuid.NullUUID{UUID: b.BuyerID, Valid: true}})
		}
		return &b, nil
	}
	bid, err := uuid.Parse(sel)
	if err != nil {
		return nil, errors.New("buyer non valido")
	}
	b, err := q.GetBuyer(ctx, bid)
	if err != nil || b.ClienteID != cliente.ClienteID {
		return nil, errors.New("buyer non appartiene al cliente scelto")
	}
	return &b, nil
}

// downloadDaForm accoda lo staging degli allegati spuntati nel form di triage.
func (s *Server) downloadDaForm(ctx context.Context, q *db.Queries, m db.Messaggio, r *http.Request) (contiDownload, error) {
	ids := r.Form["allegato_id"]
	if len(ids) == 0 {
		return contiDownload{}, nil
	}
	pr, err := q.PresenzaDaAprire(ctx, m.MessaggioID)
	if err != nil {
		return contiDownload{}, nil // messaggio non Outlook (telefono/whatsapp): niente da scaricare
	}
	return s.accodaDownload(ctx, q, m, pr, ids)
}

func (s *Server) triageErrore(w http.ResponseWriter, r *http.Request, id uuid.UUID, azione, msg string) {
	d, err := s.datiTriage(r.Context(), db.New(s.Pool), id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	d.Azione, d.Errore = azione, msg
	s.frammento(w, "triage_form", d)
}

func splitCodici(s string) []string {
	var out []string
	visti := map[string]bool{}
	for _, c := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' }) {
		c = strings.ToUpper(strings.TrimSpace(c))
		if c != "" && !visti[c] {
			visti[c] = true
			out = append(out, c)
		}
	}
	return out
}

// dominioPubblico: i webmail non identificano un cliente.
func dominioPubblico(d string) bool {
	switch d {
	case "gmail.com", "outlook.com", "hotmail.com", "hotmail.it", "live.com", "live.it", "yahoo.com", "yahoo.it", "libero.it", "icloud.com", "pec.it":
		return true
	}
	return false
}

var _ = pgx.ErrNoRows
