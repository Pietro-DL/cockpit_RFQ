package web

import (
	"context"
	"encoding/json"
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
	Scadenza       string
	Allegati       []AllegatoUI
	Errore         string

	// CartellaCliente e Anteprima: DOVE finira' questa RFQ sul NAS.
	//
	// La cartella non e' un campo da riempire: sta in anagrafica, e per un cliente gia' censito il
	// backend la usa gia' correttamente. Il difetto era tutto in interfaccia — la cartella non si
	// vedeva da nessuna parte per un cliente esistente (il campo «Cartella NAS» compare solo nel
	// riquadro «nuovo cliente», che per un cliente esistente e' nascosto), e l'anteprima veniva
	// calcolata UNA VOLTA all'apertura del form. Cambiando cliente dalla tendina, HTMX ricaricava
	// solo l'elenco dei buyer: l'anteprima restava quella di prima, oppure «<CLIENTE>\WIP\...».
	// Chi guardava ne ricavava che il Cockpit non sapesse dove mettere i file di quel cliente.
	CartellaCliente string
	DaAnagrafica    bool // la cartella viene dall'anagrafica, non da cio' che si sta digitando
	Anteprima       string
	// Oob: questo frammento sta tornando da un cambio di cliente, quindi le due righe della cartella
	// vanno sostituite dove sono (hx-swap-oob) invece di essere disegnate dentro il form.
	Oob bool

	// I candidati di codice, già divisi per ruolo (checkpoint 3R §4). Prima qui c'era una sola
	// stringa, `Identificativi`, che il form mostrava in una casella di testo precompilata: al
	// submit diventava TUTTA identificativi confermati con origine `manuale`. Nessuno li aveva
	// digitati, e il sistema registrava che una persona li aveva scritti.
	Proponibili []db.CandidatoCodice // spuntabili, e pre-spuntati: vengono dalle famiglie del cliente
	Altri       []db.CandidatoCodice // visibili e NON spuntati: l'estrattore generico
	Riferimento string               // il numero con cui il cliente chiama la richiesta: campo suo
	// Candidati di aggancio (R0–R5) con evidenza: si vedono anche nel form «Nuova RFQ», perché la
	// domanda «sei sicuro che non sia questa?» va fatta prima di creare un doppione, non dopo.
	Candidati []db.ListCandidatiAggancioRow
}

// Spuntato dice se un candidato di codice nasce già spuntato nel form. Le famiglie del cliente sì,
// l'estrattore generico no: «questo è un codice di questo cliente» e «questo ha la forma di un codice»
// non possono entrare nella RFQ con lo stesso gesto.
func (d *triageDati) Spuntato(c db.CandidatoCodice) bool {
	return c.Origine == db.OrigineCodiceFamiglia
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
		if tr.ScadenzaProposta != nil {
			d.Scadenza = tr.ScadenzaProposta.Format("2006-01-02")
		}
		if !d.BuyerID.Valid && tr.BuyerProposto.Valid {
			d.BuyerID = tr.BuyerProposto
		}
	}
	// I candidati di codice, divisi per ruolo. Il riferimento della richiesta è un campo suo e non
	// compare fra i codici: è il nome che il cliente dà alla richiesta, non un pezzo.
	cand, _ := q.ListCandidatiCodice(ctx, id)
	for _, c := range cand {
		switch c.Ruolo {
		case db.RuoloCodiceRiferimentoRfq:
			d.Riferimento = c.Codice
		case db.RuoloCodiceProdotto:
			d.Proponibili = append(d.Proponibili, c)
		default:
			d.Altri = append(d.Altri, c)
		}
	}
	// Se il cliente non ha famiglie dichiarate, i numeri generici sono tutto quello che c'è: si
	// possono spuntare, ma restano marcati come generici e non nascono spuntati.
	if len(d.Proponibili) == 0 {
		famiglie := false
		if d.ClienteID.Valid {
			if c, err := q.GetCliente(ctx, d.ClienteID.UUID); err == nil {
				r, _ := domain.LeggiRegole(c.Regole)
				famiglie = domain.Compila(c.RagioneSociale, r).HaFamiglie()
			}
		}
		if !famiglie {
			d.Proponibili, d.Altri = d.Altri, nil
		}
	}
	d.Candidati, _ = q.ListCandidatiAggancio(ctx, id)
	d.Allegati, _ = s.allegatiUI(ctx, q, id)
	d.cartella(ctx, q, "")
	d.Anteprima = domain.CartellaThread(d.CartellaCliente, m.DataEvento.Local(), d.Cognome, d.Oggetto)
	return d, nil
}

// cartella stabilisce sotto quale cartella del NAS finira' questa RFQ.
//
// Per un cliente gia' censito la risposta e' in ANAGRAFICA e non altrove: `cliente.cartella_nas`, che
// e' anche quella che il backend usa davvero quando crea la RFQ. Per un cliente nuovo e' quella che
// si sta digitando nel riquadro «nuovo cliente»; finche' e' vuota si mostra `<CLIENTE>`, che e' un
// segnaposto e si vede che lo e'.
//
// `digitata` serve solo al secondo caso: per un cliente esistente non viene nemmeno guardata. Non e'
// una precauzione teorica — e' la regola che impedisce a un campo lasciato in pagina di scavalcare
// l'anagrafica e portare i disegni di un cliente nella cartella di un altro.
func (d *triageDati) cartella(ctx context.Context, q *db.Queries, digitata string) {
	if d.ClienteID.Valid {
		if c, err := q.GetCliente(ctx, d.ClienteID.UUID); err == nil {
			d.CartellaCliente, d.DaAnagrafica = c.CartellaNas, true
			return
		}
	}
	d.DaAnagrafica = false
	if n := strings.ToUpper(strings.TrimSpace(digitata)); n != "" {
		d.CartellaCliente = n
		return
	}
	d.CartellaCliente = "<CLIENTE>"
}

// buyerSelect è il frammento ricaricato quando cambia il cliente nel form.
//
// Ricarica TRE cose, non una: i buyer di quel cliente, la sua cartella NAS e l'anteprima della
// destinazione. Prima ricaricava solo i buyer, e l'anteprima restava quella calcolata all'apertura —
// cioe' la destinazione mostrata all'operatore non era la destinazione che la RFQ avrebbe avuto.
func (s *Server) buyerSelect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := db.New(s.Pool)
	par := r.URL.Query()
	d := &triageDati{Oob: true}
	if cid, err := uuid.Parse(par.Get("cliente_id")); err == nil {
		d.ClienteID = uuid.NullUUID{UUID: cid, Valid: true}
		d.Buyers, _ = q.ListBuyerCliente(ctx, cid)
	}
	d.Oggetto = strings.TrimSpace(par.Get("oggetto"))
	d.Cognome = strings.TrimSpace(par.Get("buyer_cognome"))
	// La data e' quella del MESSAGGIO, non di oggi: e' lei a dare il nome alla cartella, e una RFQ
	// aperta oggi su una mail di settimana scorsa si chiama con il giorno della mail.
	quando := time.Now()
	if mid, err := uuid.Parse(par.Get("messaggio")); err == nil {
		if m, err := q.GetMessaggio(ctx, mid); err == nil {
			// Il messaggio serve tutto, non solo la sua data: il riquadro «nuovo cliente» torna
			// indietro con questo stesso frammento, e i suoi campi sono legati alla mail che si sta
			// smistando (l'URL di ricarica, il dominio proposto). Senza, passare da un cliente
			// esistente a «nuovo cliente» rimetteva in pagina tre campi senza mittente.
			d.M = m
			quando = m.DataEvento.Local()
			d.Email = strings.ToLower(m.MittenteIndirizzo.String)
			if i := strings.LastIndex(d.Email, "@"); i > 0 {
				d.Dominio = d.Email[i+1:]
			}
		}
	}
	d.cartella(ctx, q, par.Get("cliente_cartella"))
	d.Anteprima = domain.CartellaThread(d.CartellaCliente, quando, d.Cognome, d.Oggetto)
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
	// Gli identificativi della RFQ (checkpoint 3R §4). Due strade, e non si confondono:
	//
	//   `codice`         le caselle spuntate fra i candidati. Entrano con l'origine della PROPOSTA
	//                    (famiglia del cliente o estrattore generico) e con il punteggio che avevano.
	//                    Il giorno in cui si vuole misurare quanto il motore ci prende, la misura
	//                    esiste solo se questa differenza è stata scritta.
	//   `identificativi` quelli digitati a mano nella casella di testo: quelli sì, `manuale`, 100.
	//
	// Prima c'era solo la seconda strada, e ci passava anche la prima: la casella di testo arrivava
	// PRECOMPILATA con tutto ciò che l'estrattore generico aveva visto, e al submit ogni numero
	// diventava un identificativo confermato a mano. Bastava non guardare quella riga.
	cand, _ := q.ListCandidatiCodice(ctx, id)
	perCodice := map[string]db.CandidatoCodice{}
	for _, c := range cand {
		perCodice[strings.ToUpper(c.Codice)] = c
	}
	var riferimento string
	visti := map[string]bool{}
	for _, c := range r.Form["codice"] {
		c = strings.ToUpper(strings.TrimSpace(c))
		k, ok := perCodice[c]
		if !ok || visti[c] {
			continue // spuntato qualcosa che non era fra i candidati: non si inventa
		}
		if k.Ruolo == db.RuoloCodiceRiferimentoRfq {
			continue // un riferimento non diventa un codice prodotto nemmeno se qualcuno lo spunta
		}
		visti[c] = true
		origine := db.OrigineIdentificativoPropostaGenerico
		if k.Origine == db.OrigineCodiceFamiglia {
			origine = db.OrigineIdentificativoPropostaFamiglia
		}
		if _, err := q.UpsertIdentificativo(ctx, db.UpsertIdentificativoParams{ThreadID: t.ThreadID, Codice: k.Codice, Origine: origine,
			Confidenza: pgtype.Int2{Int16: k.Punteggio, Valid: true}, ConfermatoDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true}}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	for _, c := range splitCodici(r.FormValue("identificativi")) {
		if visti[strings.ToUpper(c)] {
			continue
		}
		visti[strings.ToUpper(c)] = true
		if _, err := q.UpsertIdentificativo(ctx, db.UpsertIdentificativoParams{ThreadID: t.ThreadID, Codice: c, Origine: db.OrigineIdentificativoManuale,
			Confidenza: pgtype.Int2{Int16: 100, Valid: true}, ConfermatoDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true}}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	// Il riferimento del cliente ha un campo suo: è il nome della richiesta, non un codice prodotto.
	if riferimento = strings.TrimSpace(r.FormValue("riferimento_cliente")); riferimento != "" {
		if err := q.SetRiferimentoCliente(ctx, db.SetRiferimentoClienteParams{
			ThreadID: t.ThreadID, Riferimento: txtN(riferimento, 60)}); err != nil {
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
	// La RFQ nasce comunque: è una decisione dell'operatore e vive nel database. Con `nas_scrittura`
	// spenta resta senza la sua cartella sul NAS finche' qualcuno non l'accende (SH1).
	cartella := "Cartella in creazione."
	if _, err := jobs.Accoda(ctx, q, db.TipoJobCreaCartellaThread, api.PayloadCreaCartellaThread{ThreadID: t.ThreadID}, "cartella:"+t.ThreadID.String(), 1); err != nil {
		if !errors.Is(err, jobs.ErrCapacitaSpenta) {
			http.Error(w, err.Error(), 500)
			return
		}
		cartella = "Cartella sul NAS IN ATTESA: la capacità [sicurezza].nas_scrittura è spenta."
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
	s.pannelloConAvviso(w, r, id, fmt.Sprintf("RFQ creata: %s. %s %s", t.CartellaRelativa.String, cartella, esiti.frase()))
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
	// La conversazione viene COLLEGATA, perché collegarla è una decisione dell'operatore e da qui in
	// avanti è l'evidenza su cui si regge la regola R1. Non è la stessa cosa che agganciare i messaggi:
	// dice «questa catena di Outlook riguarda questa richiesta», non «questi messaggi sono di questa
	// richiesta».
	conv, err := q.GetConversazione(ctx, m.ConversazioneID)
	if err == nil && !conv.ThreadID.Valid {
		if err := q.CollegaConversazione(ctx, db.CollegaConversazioneParams{ConversazioneID: conv.ConversazioneID, ThreadID: tid, CollegataDa: db.AggancioOperatore}); err != nil {
			return err
		}
	}

	// GLI ALTRI ORFANI DELLA CONVERSAZIONE NON SEGUONO (checkpoint 3R §2, T21).
	//
	// Qui prima c'era `AgganciaOrfaniConversazione`: una decisione su UN messaggio agganciava tutti gli
	// altri orfani con lo stesso ConversationID, ne accettava il triage e ne assegnava proposte e
	// riferimenti. Bastava che nella catena ci fosse una mail che parlava d'altro — e in una
	// conversazione lunga c'è quasi sempre — perché finisse dentro una RFQ senza che nessuno l'avesse
	// guardata; e un aggancio, una volta scritto, non si annulla da solo.
	//
	// Adesso quegli stessi messaggi ricevono un CANDIDATO R1 al 95 e restano in Inbox, con la proposta
	// aggiornata. Sono un clic ciascuno, ed è un clic che qualcuno deve dare.
	altri, err := q.ListOrfaniConversazione(ctx, db.ListOrfaniConversazioneParams{
		ConversazioneID: m.ConversazioneID, MessaggioID: m.MessaggioID})
	if err != nil {
		return err
	}
	const evidenzaR1 = "la conversazione di Outlook è stata collegata a questa richiesta da un operatore"
	for _, mid := range altri {
		if err := q.InsertCandidatoAggancio(ctx, db.InsertCandidatoAggancioParams{
			MessaggioID: mid, ThreadID: threadID, Regola: db.RegolaAggancioR1Conversazione,
			Punteggio: int16(domain.PuntiRegola[domain.R1Conversazione]), Evidenza: evidenzaR1,
			ThreadStato: db.StatoThreadAPERTA,
		}); err != nil {
			return err
		}
		motivo, _ := json.Marshal([]string{evidenzaR1})
		if _, err := q.AggiornaTriageCandidato(ctx, db.AggiornaTriageCandidatoParams{
			MessaggioID: mid, ThreadProposto: tid,
			Confidenza: int16(domain.PuntiRegola[domain.R1Conversazione]), Motivo: motivo,
		}); err != nil {
			return err
		}
		// la traccia dice che cos'è successo: una proposta, non una decisione presa per procura
		if err := logDecisione(ctx, q, mid, tid, "candidato", u, "orfano della stessa conversazione: proposto, non agganciato"); err != nil {
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
	// Non un upsert (voce 6.6): se la cartella NAS e' gia' di un altro cliente questo INSERT
	// fallisce e dice di chi e'. Prima rinominava quel cliente e gli lasciava domini, buyer e
	// RFQ — cioe' chi credeva di crearne uno nuovo se ne portava via un altro, in silenzio.
	c, err := CreaCliente(ctx, q, db.InsertClienteParams{CartellaNas: cartella, RagioneSociale: nome})
	if err != nil {
		return nil, fmt.Errorf("cliente: %w", err)
	}
	if dom := strings.ToLower(strings.TrimSpace(r.FormValue("cliente_dominio"))); dom != "" && !dominioPubblico(dom) {
		if err := AggiungiDominio(ctx, q, dom, c.ClienteID); err != nil {
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
	c, err := s.copiaDownload(ctx, q, m.MessaggioID, sessioneDa(ctx))
	if err != nil {
		return contiDownload{}, nil // messaggio non Outlook (telefono/whatsapp): niente da scaricare
	}
	return s.accodaDownload(ctx, q, m, c, ids)
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
