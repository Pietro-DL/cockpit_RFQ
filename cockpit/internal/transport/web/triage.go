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

	"promatec/cockpit/internal/core/inbox/aggancio"
	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/inbox/ingest"
	"promatec/cockpit/internal/core/inbox/lettura"
	"promatec/cockpit/internal/core/registro/anagrafica"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
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
	// Smistamento M1: raggruppati per RFQ e ordinati a livelli, come nel pannello (CarteForm, CarteAvviso).
	Candidati aggancio.Lettura
	// MaxAuto: oltre questa dimensione un file utile non scende da solo (il limite degli upload dei worker).
	MaxAuto int64
	// L'Inbox nuova: il modulo si apre con la mail accanto (Corpo), e le mail vicine che forse sono della
	// stessa richiesta si possono mettere nella RFQ insieme a questa, spuntandole (Vicini, ListVicini).
	Corpo  lettura.Corpo
	Vicini []db.ListViciniRow
}

// CarteForm sono i candidati dentro «Aggancia a…»: ogni card è un bottone del form, che porta con sé
// il buyer e gli allegati da scaricare.
func (d *triageDati) CarteForm() candidatiVista {
	return vistaCandidati(d.M.MessaggioID, d.Candidati, modoForm)
}

// CarteAvviso sono i candidati nell'avviso del form «Nuova RFQ»: si leggono, e per agganciare si va ad
// «Aggancia a…». Un bottone qui manderebbe il form della RFQ nuova.
func (d *triageDati) CarteAvviso() candidatiVista {
	return vistaCandidati(d.M.MessaggioID, d.Candidati, modoAvviso)
}

// SiPrepara dice se un allegato scende da solo quando la RFQ nasce o il messaggio si aggancia (B8.7b): e'
// utile (la pre-spunta di sempre), e' un file diretto del messaggio e sta nel limite degli upload.
func (d *triageDati) SiPrepara(a AllegatoUI) bool {
	return a.PreSpunta && a.Natura == db.NaturaAllegatoFile && !a.ContenitoreID.Valid && (!a.Bytes.Valid || a.Bytes.Int64 <= d.MaxAuto)
}

// Spuntato dice se un candidato di codice nasce già spuntato nel form. Le famiglie del cliente sì,
// l'estrattore generico no: «questo è un codice di questo cliente» e «questo ha la forma di un codice»
// non possono entrare nella RFQ con lo stesso gesto.
//
// E nemmeno un codice di famiglia trovato solo nella storia citata: è la regola di
// classificazione.Estrazione.Proponibili (blocco 6). Era già stato deciso in un altro messaggio, e
// spuntarlo da solo a ogni risposta è il modo in cui un «ricevuto, grazie» diventa una richiesta di
// sei pezzi. Si vede, con la sua evidenza, e per entrare serve un clic.
//
// Nemmeno un codice visto SOLO nel nome di un allegato (Smistamento P17): un nome di file non e' una
// richiesta, e la spunta gia' messa faceva di «7120001A_1.stp» un prodotto 7120001A con il clic su «Crea
// RFQ» (E11). Si vede, con l'etichetta che lo dice (SoloNelNome), e lo spunta chi lo riconosce.
//
// E nell'aggancio niente nasce spuntato (scelta 6): la RFQ ha gia' i suoi codici, e un codice spuntato
// all'aggancio diventa un prodotto. Una risposta che cita un prodotto tolto dalla BOM non deve farlo rinascere
// con il clic su «Aggancia a questa RFQ»: lo rimette nella richiesta solo chi lo spunta.
func (d *triageDati) Spuntato(c db.CandidatoCodice) bool {
	return d.Azione != "aggancia" && c.Origine == db.OrigineCodiceFamiglia && c.Evidenza != classificazione.DoveStoria && !SoloNelNome(c)
}

// SoloNelNome dice che il codice e' stato visto solo nel nome di un allegato: l'evidenza di candidato_codice
// e' «allegato <nome del file>» (classificazione.Testi). Se il codice c'era anche nell'oggetto o nel corpo,
// l'evidenza e' quella.
func SoloNelNome(c db.CandidatoCodice) bool {
	return strings.HasPrefix(c.Evidenza, "allegato ")
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
	// La posta di un fornitore non apre una RFQ cliente (7A). Il pannello non offre il pulsante, ma
	// nascondere non è autorizzare: un pannello rimasto aperto da prima del censimento, o un indirizzo
	// scritto a mano, chiedono il form lo stesso. Si offre quello dell'aggancio, con il motivo.
	if d.Azione == "nuova" && d.M.ControparteTipo == db.TipoControparteFornitore {
		d.Azione, d.Errore = "aggancia", motivoFornitoreSenzaRFQ
	}
	s.frammento(w, "triage_form", d)
}

func (s *Server) datiTriage(ctx context.Context, q *db.Queries, id uuid.UUID) (*triageDati, error) {
	m, err := q.GetMessaggio(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &triageDati{M: m, Oggetto: classificazione.OggettoPulito(m.Oggetto.String), MaxAuto: s.maxCaricamentoEffettivo()}
	d.Riga, _ = q.GetInboxRiga(ctx, id)
	d.Clienti, _ = q.ListClienti(ctx)
	d.ClienteID = d.Riga.ClienteID
	d.Email = strings.ToLower(m.MittenteIndirizzo.String)
	if i := strings.LastIndex(d.Email, "@"); i > 0 {
		d.Dominio = d.Email[i+1:]
	}
	d.Nome, d.Cognome = anagrafica.NomeCognome(m.MittenteNome.String, d.Email)
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
				r, _ := regole.LeggiRegole(c.Regole)
				famiglie = classificazione.Compila(c.RagioneSociale, r).HaFamiglie()
			}
		}
		if !famiglie {
			d.Proponibili, d.Altri = d.Altri, nil
		}
	}
	d.Candidati, _ = aggancio.InLettura(ctx, q, id)
	d.Allegati, _ = s.allegatiUI(ctx, q, id)
	d.Corpo = lettura.Presenta(m.CorpoTesto.String, m.CorpoHtml.String)
	d.Vicini, _ = q.ListVicini(ctx, db.ListViciniParams{MessaggioID: id, Pubblici: dominiPubblici})
	d.cartella(ctx, q, "")
	d.Anteprima = documenti.CartellaThread(d.CartellaCliente, m.DataEvento.Local(), d.Cognome, d.Oggetto)
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
	d.Anteprima = documenti.CartellaThread(d.CartellaCliente, quando, d.Cognome, d.Oggetto)
	s.frammento(w, "buyer_select", d)
}

// cercaThread è la ricerca incrementale di "Aggancia a…" (oggetto, cliente, codici) sui thread aperti.
// La query mette il testo fra due '%': % _ e \ scritti dall'operatore si prendono alla lettera, come
// nella barra della pagina Richieste (testoLetterale). Chi cerca «7120_400» cerca quello.
func (s *Server) cercaThread(w http.ResponseWriter, r *http.Request) {
	righe, err := db.New(s.Pool).CercaThreadAperti(r.Context(), testoLetterale(strings.TrimSpace(r.URL.Query().Get("q"))))
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
	// La regola del pannello, qui dove la decisione si scrive: un fornitore non apre una RFQ cliente.
	// La controparte si legge dal messaggio appena bloccato, non da quello che la pagina mostrava.
	if m.ControparteTipo == db.TipoControparteFornitore {
		s.triageErrore(w, r, id, "aggancia", motivoFornitoreSenzaRFQ)
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
		oggetto = classificazione.OggettoPulito(m.Oggetto.String)
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
	cartellaRFQ, err := cartellaLibera(ctx, tx, documenti.CartellaThread(cliente.CartellaNas, m.DataEvento.Local(), cognome, oggetto))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	t, err := q.InsertThread(ctx, db.InsertThreadParams{
		ClienteID: cliente.ClienteID, BuyerID: buyerID, Canale: m.Canale, DataInizio: m.DataEvento, DataScadenza: scadenza, ScadenzaOrigine: origine,
		Oggetto: ptxt(oggetto), CartellaRelativa: ptxt(cartellaRFQ),
		Priorita: int16(prio), Campionatura: r.FormValue("campionatura") == "1", Note: ptxt(r.FormValue("note")), CreatoDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true},
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// Gli identificativi della RFQ (checkpoint 3R §4): quelli spuntati e quelli scritti, confermati da chi
	// crea la RFQ. Sono i soli che diventano prodotti con questo gesto (scelta 6).
	confermati, err := confermaCodiciDelGesto(ctx, q, r, id, t.ThreadID, u.UtenteID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var riferimento string
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
	if err := s.agganciaMessaggioAThread(ctx, q, u, m, t.ThreadID, buyerID, classificazione.GestoNuovaRFQ); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// «Della stessa richiesta»: le mail vicine che chi crea la RFQ ha spuntato, una per una (ListVicini). Ognuna
	// e' una decisione sua, con la sua fotografia e la sua riga nel registro: nessuna segue da sola (T21). Una
	// mail gia' decisa nel frattempo, o di un fornitore, resta dov'e'.
	insieme := 0
	for _, v := range r.Form["insieme"] {
		vid, err := uuid.Parse(v)
		if err != nil || vid == id {
			continue
		}
		mm, deciso, err := messaggioDaDecidere(ctx, q, vid)
		if err != nil || deciso || mm.ControparteTipo == db.TipoControparteFornitore {
			continue
		}
		if err := s.agganciaMessaggioAThread(ctx, q, u, mm, t.ThreadID, uuid.NullUUID{}, classificazione.GestoAggancia); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		insieme++
	}
	// La RFQ nasce comunque: è una decisione dell'operatore e vive nel database. Con `nas_scrittura`
	// spenta resta senza la sua cartella sul NAS finche' qualcuno non l'accende (SH1).
	cartella := "Cartella in creazione."
	if _, err := coda.Accoda(ctx, q, db.TipoJobCreaCartellaThread, worker.PayloadCreaCartellaThread{ThreadID: t.ThreadID}, "cartella:"+t.ThreadID.String(), 1); err != nil {
		if !errors.Is(err, coda.ErrCapacitaSpenta) {
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
	prep, err := s.preparaDopoLaDecisione(ctx, q, t.ThreadID, confermati)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// il flusso ancorato al prodotto, dopo il commit (Smistamento F8, IN1): i prodotti appena confermati e i
	// file gia' analizzati per un'altra RFQ danno subito le loro destinazioni
	s.rismista(ctx, t.ThreadID)
	conInsieme := ""
	if insieme > 0 {
		conInsieme = fmt.Sprintf(" Con la RFQ anche %d mail della stessa richiesta.", insieme)
	}
	// «Crea la RFQ e apri la Distinta»: la RFQ appena nata si apre dove si lavora.
	if r.FormValue("poi") == "distinta" {
		w.Header().Set("HX-Redirect", "/thread/"+t.ThreadID.String()+"/distinta")
	}
	s.pannelloConAvviso(w, r, id, fmt.Sprintf("RFQ creata: %s. %s%s%s%s", t.CartellaRelativa.String, cartella, esiti.fraseSeCe(), prep, conInsieme))
}

// confermaCodiciDelGesto scrive gli identificativi che chi decide ha confermato in questo gesto (la creazione
// della RFQ o l'aggancio) e restituisce i loro codici: sono i soli che il gesto fa diventare prodotti
// (preparaDopoLaDecisione, scelta 6). Due strade, e non si confondono (checkpoint 3R §4):
//
//	`codice`         le caselle spuntate fra i candidati del messaggio. Entrano con l'origine della PROPOSTA
//	                 (famiglia del cliente o estrattore generico) e con il punteggio che avevano. Il giorno
//	                 in cui si vuole misurare quanto il motore ci prende, la misura esiste solo se questa
//	                 differenza è stata scritta.
//	`identificativi` quelli digitati a mano nella casella di testo: quelli sì, `manuale`, 100.
//
// Prima c'era solo la seconda strada, e ci passava anche la prima: la casella di testo arrivava PRECOMPILATA
// con tutto ciò che l'estrattore generico aveva visto, e al submit ogni numero diventava un identificativo
// confermato a mano. Bastava non guardare quella riga. Un codice spuntato che non era fra i candidati del
// messaggio non si inventa, e un riferimento della richiesta non diventa un codice prodotto nemmeno spuntato.
func confermaCodiciDelGesto(ctx context.Context, q *db.Queries, r *http.Request, messaggio, thread, utente uuid.UUID) ([]string, error) {
	cand, err := q.ListCandidatiCodice(ctx, messaggio)
	if err != nil {
		return nil, err
	}
	perCodice := map[string]db.CandidatoCodice{}
	for _, c := range cand {
		perCodice[strings.ToUpper(c.Codice)] = c
	}
	chi := uuid.NullUUID{UUID: utente, Valid: true}
	var confermati []string
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
		if _, err := q.UpsertIdentificativo(ctx, db.UpsertIdentificativoParams{ThreadID: thread, Codice: k.Codice, Origine: origine,
			Confidenza: pgtype.Int2{Int16: k.Punteggio, Valid: true}, ConfermatoDa: chi}); err != nil {
			return nil, err
		}
		confermati = append(confermati, k.Codice)
	}
	for _, c := range splitCodici(r.FormValue("identificativi")) {
		if visti[strings.ToUpper(c)] {
			continue
		}
		visti[strings.ToUpper(c)] = true
		if _, err := q.UpsertIdentificativo(ctx, db.UpsertIdentificativoParams{ThreadID: thread, Codice: c, Origine: db.OrigineIdentificativoManuale,
			Confidenza: pgtype.Int2{Int16: 100, Valid: true}, ConfermatoDa: chi}); err != nil {
			return nil, err
		}
		confermati = append(confermati, c)
	}
	return confermati, nil
}

// preparaDopoLaDecisione e' la preparazione del Fascicolo nella transazione di chi ha appena creato la RFQ o
// agganciato il messaggio (B8.7b): i codici della richiesta confermati IN QUESTO GESTO (confermati) diventano
// prodotti finiti, e i file utili della RFQ scendono nello staging senza che nessuno li spunti. Il NAS aspetta
// la conferma. Restituisce la frase per l'avviso.
//
// Scelta 6 dello Smistamento (confermata il 27/09): prima nascevano tutti i codici confermati della richiesta
// senza un componente, e un prodotto tolto dalla BOM rinasceva all'aggancio di una mail che non lo citava.
// Adesso il gesto crea soltanto i codici che chi decide ha spuntato o scritto nello stesso gesto.
//
// Smistamento F1 (addendum A5.4.5): e' l'unico posto in cui i prodotti della richiesta nascono da soli (la
// GET del Fascicolo non li fa piu'), ed e', con «Rianalizza», l'unico gesto che rilegge gli STEP gia'
// analizzati. Quelli analizzati prima che il messaggio entrasse nella RFQ (D30 li scarica e li analizza prima
// dell'aggancio) non hanno mai avuto le loro proposte in questa RFQ, perche' il risultato dell'analisi le
// scrive solo nelle RFQ che hanno gia' quel contenuto (critica C6). PreparaFile senza rileggi: i file fermi
// li completa la preparazione della pagina (POST …/fascicolo/prepara), che accoda e basta.
func (s *Server) preparaDopoLaDecisione(ctx context.Context, q *db.Queries, thread uuid.UUID, confermati []string) (string, error) {
	prodotti, err := fascicolo.AssicuraProdottiDellaRichiesta(ctx, q, thread, confermati)
	if err != nil {
		return "", err
	}
	prep, err := fascicolo.PreparaFile(ctx, q, thread, s.Analizzatore, s.maxCaricamentoEffettivo(), MaxPreparatiPerApertura, nil)
	if err != nil {
		return "", err
	}
	// dopo i prodotti: la radice di uno STEP riletto ritrova il prodotto appena nato
	var riletti fascicolo.Rianalisi
	if s.Analizzatore.Versione != 0 {
		if riletti, err = fascicolo.RileggiStepDellaRfq(ctx, q, thread, s.Analizzatore); err != nil {
			return "", err
		}
	}
	frase := ""
	if n := len(prodotti.Creati); n > 0 {
		frase += fmt.Sprintf(" %s nella BOM come %s.", strings.Join(prodotti.Creati, ", "), plurale(n, "prodotto finito", "prodotti finiti"))
	}
	if riletti.Riletti > 0 {
		frase += " " + conta(riletti.Riletti, "STEP già analizzato letto", "STEP già analizzati letti") + " in questa RFQ: le proposte di struttura sono nel Fascicolo."
	}
	if n := prep.Download + prep.Riusati; n > 0 {
		frase += fmt.Sprintf(" %s in preparazione (staging e analisi): il Fascicolo si aggiorna da solo.", conta(n, "file utile", "file utili"))
	}
	if len(prep.Saltati) > 0 {
		frase += " Non scaricati da soli: " + strings.Join(prep.Saltati, "; ") + "."
	}
	return frase, nil
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
	// mittente non ancora censito come buyer del cliente del thread → lo si crea se richiesto.
	//
	// Un «no» del buyer ferma l'aggancio, come in nuovaRFQ. Prima lo si ingoiava: un errore logico
	// (l'indirizzo di un altro cliente) agganciava senza buyer e senza dirlo, e un errore del database
	// (un cognome troppo lungo, un'e-mail presa nello stesso istante) lasciava la transazione
	// interrotta, e l'aggancio falliva dopo con un 500 che non diceva niente.
	var buyerID uuid.NullUUID
	if r.FormValue("crea_buyer") == "1" {
		cl, err := q.GetCliente(ctx, t.ClienteID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		b, err := s.buyerDaForm(ctx, q, r, &cl)
		if err != nil {
			s.triageErrore(w, r, id, "aggancia", err.Error())
			return
		}
		if b != nil {
			buyerID = uuid.NullUUID{UUID: b.BuyerID, Valid: true}
		}
	}
	if err := s.agganciaMessaggioAThread(ctx, q, u, m, tid, buyerID, classificazione.GestoAggancia); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// I codici che chi aggancia ha spuntato o scritto (nell'aggancio nessuno nasce spuntato: Spuntato): entrano
	// fra quelli della richiesta, e sono i soli che l'aggancio fa diventare prodotti (scelta 6).
	confermati, err := confermaCodiciDelGesto(ctx, q, r, id, tid, u.UtenteID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	esiti, err := s.downloadDaForm(ctx, q, m, r)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	prep, err := s.preparaDopoLaDecisione(ctx, q, tid, confermati)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.rismista(ctx, tid) // IN1, dopo il commit (Smistamento F8)
	s.pannelloConAvviso(w, r, id, fmt.Sprintf("Agganciato alla RFQ %s.%s%s", t.CartellaRelativa.String, esiti.fraseSeCe(), prep))
}

// ricalcolaCandidati è «Ricalcola» del pannello (Smistamento M1, A5.10): i candidati di un messaggio
// scritti con le regole di prima si ricalcolano con quelle di adesso. Un POST, un messaggio: nessun
// ricalcolo in blocco, e nessuna GET che scrive.
func (s *Server) ricalcolaCandidati(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	cambio, err := (&ingest.Servizio{Pool: s.Pool, Log: s.Log}).RitriageMessaggio(r.Context(), id)
	switch {
	case errors.Is(err, ingest.ErrGiaDeciso):
		s.pannelloConAvviso(w, r, id, "Il messaggio è già stato deciso: i suoi candidati non si ricalcolano.")
		return
	case err != nil:
		http.Error(w, err.Error(), 500)
		return
	}
	frase := "Candidati ricalcolati con le regole di adesso."
	if cambio != "" {
		frase += " La proposta è cambiata."
	}
	s.pannelloConAvviso(w, r, id, frase)
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
	// la fotografia prima del gesto: i candidati come li aveva davanti chi ha premuto «Ignora» (M3)
	scelta, err := fotografia(ctx, q, m, classificazione.GestoIgnora, uuid.Nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := q.IgnoraMessaggio(ctx, db.IgnoraMessaggioParams{MessaggioID: id, DecisoDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true}}); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := logDecisione(ctx, q, id, uuid.NullUUID{}, "ignora", u, scelta.Motivo()); err != nil {
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
//
// Smistamento M3 (A5.16.6): il motivo è la fotografia della decisione (classificazione.Scelta), un JSON
// con la frase di sempre dentro. È il materiale della calibrazione (`cockpit -calibrazione`).
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

// fotografia è la Scelta di una decisione sul messaggio m (Smistamento M3, A5.16.6): i candidati come il
// pannello li mostra (le righe di `candidato_aggancio` più il marcatore, raggruppati e ordinati a
// livelli), la posizione della RFQ scelta fra loro, l'evento e l'atto del messaggio. Il rango lo calcola
// il server qui, nella transazione della decisione, rileggendo i candidati: le righe di un messaggio non
// ancora deciso cambiano solo con un ricalcolo, quindi sono quelle che l'operatore aveva davanti; e non
// c'è niente da fidarsi di un rango mandato dal browser. `scelto` è uuid.Nil per «Ignora».
func fotografia(ctx context.Context, q *db.Queries, m db.Messaggio, gesto string, scelto uuid.UUID) (classificazione.Scelta, error) {
	l, err := aggancio.InLettura(ctx, q, m.MessaggioID)
	if err != nil {
		return classificazione.Scelta{}, fmt.Errorf("candidati per la fotografia: %w", err)
	}
	evento, atto, err := eventoDellaDecisione(ctx, q, m, l.Origini)
	if err != nil {
		return classificazione.Scelta{}, err
	}
	id := ""
	if scelto != uuid.Nil {
		id = scelto.String()
	}
	return classificazione.SceltaDa(l.Candidati, gesto, id, evento, atto), nil
}

// eventoDellaDecisione è l'evento del messaggio come lo mostra il pannello (M2): dall'atto salvato dal
// triage, o calcolato adesso dal messaggio se il triage non l'ha interpretato (una nostra mail a un
// cliente). Serve alla calibrazione per separare, domani, la precisione dei candidati per evento: una
// revisione CAD e una RFQ nuova non hanno la stessa difficoltà.
func eventoDellaDecisione(ctx context.Context, q *db.Queries, m db.Messaggio, origini []aggancio.Origine) (evento, atto string, err error) {
	p, err := q.GetTriageMessaggio(ctx, m.MessaggioID)
	switch {
	case err == nil && p.Atto.Valid && p.Atto.String != "":
		legame := ""
		if p.Legame.Valid {
			legame = string(p.Legame.LegameOperativo)
		}
		return classificazione.EventoDa(string(m.ControparteTipo), string(m.Direzione), p.Atto.String, legame), p.Atto.String, nil
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return "", "", fmt.Errorf("triage per la fotografia: %w", err)
	}
	v := eventoInLettura(ctx, q, m, origini)
	return v.Codice, v.Atto, nil
}

// ---------------------------------------------------------------- pezzi della transazione

// agganciaMessaggioAThread: la DECISIONE di aggancio, con tutto ciò che la segue (conversazione, orfani della
// stessa conversazione, proposte e riferimenti portale del messaggio, chiusura del triage, buyer).
//
// `gesto` è quello della fotografia (M3): classificazione.GestoAggancia per una RFQ esistente,
// GestoNuovaRFQ per la RFQ appena creata dal messaggio. Nel log l'azione resta `aggancia` per tutti e due,
// come prima: il CHECK della 0003 non conosce `nuova_rfq`, e il gesto sta nel JSON.
func (s *Server) agganciaMessaggioAThread(ctx context.Context, q *db.Queries, u *db.Utente, m db.Messaggio, threadID uuid.UUID, buyerID uuid.NullUUID, gesto string) error {
	tid := uuid.NullUUID{UUID: threadID, Valid: true}
	op := uuid.NullUUID{UUID: u.UtenteID, Valid: true}
	// prima di agganciare: la fotografia è di ciò che si vedeva prima del gesto
	scelta, err := fotografia(ctx, q, m, gesto, threadID)
	if err != nil {
		return err
	}
	if err := q.AgganciaMessaggio(ctx, db.AgganciaMessaggioParams{MessaggioID: m.MessaggioID, ThreadID: tid, Aggancio: db.AggancioOperatore, AgganciatoDa: op}); err != nil {
		return err
	}
	if err := logDecisione(ctx, q, m.MessaggioID, tid, "aggancia", u, scelta.Motivo()); err != nil {
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
	// Adesso quegli stessi messaggi ricevono un CANDIDATO e restano in Inbox. Sono un clic ciascuno, ed
	// è un clic che qualcuno deve dare.
	//
	// Smistamento M1 (A5.16.5): il candidato non è più un R1 fisso a 95. È la variante che l'indice e il
	// cliente dell'orfano danno: forte se l'orfano è davvero una risposta nella catena di una mail di
	// questa RFQ e il cliente è lo stesso, debole se sta nella conversazione solo per l'oggetto. La
	// proposta dell'orfano cambia solo se il candidato supera la soglia ed è il primo dei suoi, senza pari
	// merito: un indizio debole si vede, non sposta niente.
	altri, err := q.ListOrfaniConversazione(ctx, db.ListOrfaniConversazioneParams{
		ConversazioneID: m.ConversazioneID, MessaggioID: m.MessaggioID})
	if err != nil {
		return err
	}
	for _, mid := range altri {
		k, err := aggancio.CandidatoConversazione(ctx, q, mid, threadID)
		if err != nil {
			return err
		}
		if err := aggancio.SalvaPiuForte(ctx, q, mid, k); err != nil {
			return err
		}
		// l'elenco dell'orfano dopo il candidato nuovo: serve alla proposta e al rango della fotografia
		l, err := aggancio.InLettura(ctx, q, mid)
		if err != nil {
			return err
		}
		proposto := false
		if k.Punteggio >= classificazione.SogliaEvidenza {
			if len(l.Candidati) > 0 && !l.PariMerito && l.Candidati[0].ThreadID == threadID.String() {
				primo := l.Candidati[0]
				motivo, _ := json.Marshal([]string{primo.Evidenze[0].Evidenza})
				if _, err := q.AggiornaTriageCandidato(ctx, db.AggiornaTriageCandidatoParams{
					MessaggioID: mid, ThreadProposto: tid, Confidenza: int16(primo.Score), Motivo: motivo,
				}); err != nil {
					return err
				}
				proposto = true
			}
		}
		// la traccia dice che cos'è successo: una proposta, non una decisione presa per procura
		frase := fmt.Sprintf("orfano della stessa conversazione: candidato %s · score %d, non agganciato", k.Livello(), k.Punteggio)
		if proposto {
			frase += ", proposto"
		}
		// la fotografia del giro (M3): il candidato proposto con il suo rango fra quelli dell'orfano. Il
		// gesto `candidato` è automatico: le misure lo tengono fuori, ma dice che cosa l'orfano ha visto
		// cambiare, e da chi è partito (l'utente del log è chi ha deciso il messaggio di partenza).
		om, err := q.GetMessaggio(ctx, mid)
		if err != nil {
			return err
		}
		evento, atto, err := eventoDellaDecisione(ctx, q, om, l.Origini)
		if err != nil {
			return err
		}
		scelta := classificazione.SceltaDa(l.Candidati, classificazione.GestoCandidato, threadID.String(), evento, atto)
		scelta.Frase = frase
		if err := logDecisione(ctx, q, mid, tid, "candidato", u, scelta.Motivo()); err != nil {
			return err
		}
	}
	if buyerID.Valid {
		if !m.BuyerID.Valid {
			if err := q.SetBuyerMessaggio(ctx, db.SetBuyerMessaggioParams{MessaggioID: m.MessaggioID, BuyerID: buyerID}); err != nil {
				return err
			}
		}
		if err := q.SetBuyerThread(ctx, db.SetBuyerThreadParams{ThreadID: threadID, BuyerID: buyerID}); err != nil {
			return err
		}
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
	cartella := documenti.NomeSicuro(strings.ToUpper(strings.TrimSpace(r.FormValue("cliente_cartella"))), 80)
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
	if dom := strings.ToLower(strings.TrimSpace(r.FormValue("cliente_dominio"))); dom != "" && !classificazione.DominioPubblico(dom) {
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
			if _, err := q.SetBuyerMessaggiPerIndirizzo(ctx, db.SetBuyerMessaggiPerIndirizzoParams{Lower: email, BuyerID: uuid.NullUUID{UUID: b.BuyerID, Valid: true}}); err != nil {
				return nil, fmt.Errorf("buyer: %w", err)
			}
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

// motivoFornitoreSenzaRFQ e' la frase di chi chiede «Nuova RFQ» sulla posta di un fornitore: la stessa
// regola del pannello (7A), detta anche dove la decisione si scrive.
const motivoFornitoreSenzaRFQ = "Il mittente è un fornitore censito: la sua posta non apre una RFQ cliente, si aggancia a quella per cui lavora (o si ignora)."

// cartellaLibera e' la cartella NAS di una RFQ che nasce: quella calcolata, oppure la stessa con un
// progressivo, « (2)», « (3)», se un'altra RFQ ce l'ha gia'. Stesso cliente, stesso buyer, stesso
// oggetto e stesso giorno danno lo stesso nome, e due RFQ nella stessa cartella si contenderebbero i
// nomi dei file sul NAS: la riserva dei nomi e' per RFQ, e la copia della seconda finirebbe in un
// conflitto che non si risolve da solo. Il confronto e' senza maiuscole, come i nomi su Windows.
//
// Il nome si sceglie sotto un lucchetto sul nome calcolato (fino al COMMIT): due «Nuova RFQ» sulla
// stessa richiesta arrivata due volte non prendono lo stesso nome nello stesso istante.
func cartellaLibera(ctx context.Context, tx pgx.Tx, base string) (string, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('cartella_rfq:' || lower($1), 0))`, base); err != nil {
		return "", err
	}
	for n := 1; n <= 99; n++ {
		nome := base
		if n > 1 {
			nome = fmt.Sprintf("%s (%d)", base, n)
		}
		var presa bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM thread_offerta WHERE lower(cartella_relativa) = lower($1))`, nome).Scan(&presa); err != nil {
			return "", err
		}
		if !presa {
			return nome, nil
		}
	}
	return "", fmt.Errorf("cartella %s: novantanove RFQ con lo stesso nome, e nessun progressivo libero", base)
}
