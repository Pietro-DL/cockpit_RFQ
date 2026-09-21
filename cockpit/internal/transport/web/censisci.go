package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/inbox/ingest"
	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/db"
)

// «Censisci come fornitore» / «Censisci come cliente» dal pannello del messaggio (blocco 7A.3)
//
// Il quadrante «Da validare» è pieno di mittenti che il Cockpit non conosce. Prima l'unico modo
// di dirgli chi erano era andare in Admin → Anagrafica, creare il cliente, tornare nell'Inbox e
// aspettare il prossimo messaggio: quelli già arrivati restavano «sconosciuti» per sempre. Qui il
// censimento si fa dal messaggio, con l'indirizzo e il dominio già scritti, e al salvataggio il
// server ricalcola i messaggi NON DECISI dello stesso indirizzo o dominio: cambiano quadrante,
// controparte e proposta. Le decisioni prese (una RFQ, una proposta accettata o rifiutata) non si
// toccano: è la regola di 7A.3 e la query `ListMessaggiDaRitriage` la applica.
//
// È un'azione dell'operatore, non dell'amministratore: chi legge la posta è chi sa se quello che
// scrive è un fornitore. Creare un fornitore non cambia regole di riconoscimento dei clienti
// (D29): aggiunge una voce all'anagrafica, e si vede in Admin → Anagrafica → Fornitori.

type censisciDati struct {
	M    db.Messaggio
	Riga db.VInbox
	Come string // fornitore | cliente
	// Indirizzo e Dominio sono quelli che il resolver ha guardato: il mittente in entrata, il primo
	// destinatario esterno in uscita. DominioPubblico (gmail, pec…) sposta il predefinito
	// sull'indirizzo: censire gmail.com come fornitore farebbe fornitore chiunque.
	Indirizzo       string
	Dominio         string
	DominioPubblico bool
	Lavorazioni     []db.Lavorazione
	Tipi            []db.TipoFornitore
	Errore          string
	// i valori del form, rimessi in pagina quando il salvataggio è rifiutato
	RagioneSociale string
	Tipo           string
	Cartella       string
	Lingua         string
	ContattoNome   string
	UsaDominio     bool
	UsaContatto    bool
	Scelte         map[string]bool
}

func (s *Server) censisciForm(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	q := db.New(s.Pool)
	m, err := q.GetMessaggio(r.Context(), id)
	if err != nil {
		http.Error(w, "messaggio non trovato", 404)
		return
	}
	d, err := s.datiCensisci(r.Context(), q, m, r.URL.Query().Get("come"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.frammento(w, "censisci_form", d)
}

func (s *Server) datiCensisci(ctx context.Context, q *db.Queries, m db.Messaggio, come string) (*censisciDati, error) {
	if come != "cliente" {
		come = "fornitore"
	}
	d := &censisciDati{M: m, Come: come, Lingua: "it", Tipo: string(db.TipoFornitoreProcessi), Scelte: map[string]bool{}}
	d.Riga, _ = q.GetInboxRiga(ctx, m.MessaggioID)
	var err error
	if d.Indirizzo, d.Dominio, err = ingest.IndirizzoDaCensire(ctx, q, m); err != nil {
		return nil, err
	}
	d.DominioPubblico = domain.DominioPubblico(d.Dominio)
	d.UsaDominio = d.Dominio != "" && !d.DominioPubblico
	d.UsaContatto = d.DominioPubblico
	if m.Direzione == db.DirezioneEntrata {
		d.ContattoNome = strings.TrimSpace(m.MittenteNome.String)
	}
	if d.Lavorazioni, err = q.ListLavorazioni(ctx); err != nil {
		return nil, err
	}
	d.Tipi = db.AllTipoFornitoreValues()
	return d, nil
}

func (s *Server) censisci(w http.ResponseWriter, r *http.Request) {
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
	q := db.New(s.Pool)
	m, err := q.GetMessaggio(ctx, id)
	if err != nil {
		http.Error(w, "messaggio non trovato", 404)
		return
	}
	d, err := s.datiCensisci(ctx, q, m, r.FormValue("come"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	d.RagioneSociale = strings.TrimSpace(r.FormValue("ragione_sociale"))
	d.Tipo = r.FormValue("tipo")
	d.Cartella = documenti.NomeSicuro(strings.ToUpper(strings.TrimSpace(r.FormValue("cartella_nas"))), 80)
	d.Lingua = strings.ToLower(strings.TrimSpace(r.FormValue("lingua")))
	d.ContattoNome = strings.TrimSpace(r.FormValue("contatto_nome"))
	d.UsaDominio = r.FormValue("usa_dominio") == "1" && d.Dominio != ""
	d.UsaContatto = r.FormValue("usa_contatto") == "1"
	for _, l := range r.Form["lavorazione"] {
		d.Scelte[l] = true
	}

	rifiuta := func(motivo string) {
		d.Errore = motivo
		s.frammento(w, "censisci_form", d)
	}
	switch {
	case d.Indirizzo == "":
		rifiuta("questo messaggio non ha un indirizzo da censire")
		return
	case d.RagioneSociale == "":
		rifiuta("serve la ragione sociale")
		return
	case !d.UsaDominio && !d.UsaContatto:
		rifiuta("senza il dominio né l'indirizzo il " + d.Come + " esisterebbe in anagrafica ma nessun messaggio verrebbe riconosciuto: spunta almeno uno dei due")
		return
	case d.Come == "cliente" && (d.Cartella == "" || d.Cartella == "senza nome"):
		rifiuta("serve il nome della cartella NAS")
		return
	}

	var avviso string
	if d.Come == "fornitore" {
		avviso, err = s.censisciFornitore(ctx, d)
	} else {
		avviso, err = s.censisciCliente(ctx, d)
	}
	if err != nil {
		rifiuta(err.Error())
		return
	}

	// Il ritriage mirato: DOPO il commit, con il servizio dell'ingest, perché è lo stesso codice che
	// decide controparte, triage e candidati a ogni messaggio nuovo. Un secondo calcolo qui
	// divergerebbe dal primo.
	dominio := ""
	if d.UsaDominio {
		dominio = d.Dominio
	}
	esito, err := (&ingest.Servizio{Pool: s.Pool, Log: s.Log}).Ritriage(ctx, d.Indirizzo, dominio)
	if err != nil {
		avviso += " Il ricalcolo dei messaggi però non è riuscito: " + err.Error()
		s.Log.Error("ritriage dopo il censimento", "come", d.Come, "indirizzo", d.Indirizzo, "dominio", dominio, "err", err)
	} else {
		avviso += " Ricalcolo: " + esito.String() + "."
		s.Log.Info("censimento dal pannello", "come", d.Come, "ragione_sociale", d.RagioneSociale,
			"indirizzo", d.Indirizzo, "dominio", dominio, "ritriage", esito.String(), "utente", siglaDa(r))
	}
	s.pannelloConAvviso(w, r, id, avviso)
}

// censisciFornitore crea il fornitore, o lo riusa se la ragione sociale c'è già, e gli aggiunge
// dominio, contatto e capacità. Non tocca nessun campo di un fornitore esistente.
//
// Non è una transazione, di proposito: le scritture non distruttive (`AggiungiDominioFornitore`)
// rispondono a un vincolo violato con una seconda query che dice DI CHI è il dominio, e dentro una
// transazione abortita quella query non risponde più. È la stessa forma di `nuovoCliente` in
// anagrafica: ogni passo è idempotente o fallisce da solo, e l'esito dice fin dove si è arrivati.
func (s *Server) censisciFornitore(ctx context.Context, d *censisciDati) (string, error) {
	tipo := db.TipoFornitore(d.Tipo)
	if !tipo.Valid() {
		return "", fmt.Errorf("tipo di fornitore %q non valido", d.Tipo)
	}
	q := db.New(s.Pool)
	f, err := q.GetFornitorePerRagioneSociale(ctx, d.RagioneSociale)
	riusato := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		f, err = q.InsertFornitore(ctx, db.InsertFornitoreParams{
			RagioneSociale: d.RagioneSociale, Tipo: tipo, Lingua: txtN(d.Lingua, 2)})
	}
	if err != nil {
		return "", err
	}
	creato := "censito"
	if riusato {
		creato = "già in anagrafica: aggiunti"
	}
	var fatti []string
	if d.UsaDominio {
		if err := AggiungiDominioFornitore(ctx, q, d.Dominio, f.FornitoreID); err != nil {
			return "", fmt.Errorf("fornitore %s %s, ma il dominio no: %w", f.RagioneSociale, creato, err)
		}
		fatti = append(fatti, "dominio "+d.Dominio)
	}
	if d.UsaContatto {
		_, err := q.InsertContattoFornitore(ctx, db.InsertContattoFornitoreParams{
			FornitoreID: f.FornitoreID, Nome: txtN(d.ContattoNome, 120), Lower: d.Indirizzo, Lingua: txtN(d.Lingua, 2)})
		if _, dup := vincoloViolato(err); err != nil && !dup {
			return "", err
		}
		fatti = append(fatti, "contatto "+d.Indirizzo)
	}
	for codice := range d.Scelte {
		if _, err := q.InsertLavorazioneFornitore(ctx, db.InsertLavorazioneFornitoreParams{FornitoreID: f.FornitoreID, Lavorazione: codice}); err != nil {
			return "", fmt.Errorf("lavorazione %q: %w", codice, err)
		}
	}

	avviso := fmt.Sprintf("Fornitore %s %s %s.", f.RagioneSociale, creato, strings.Join(fatti, ", "))
	if !riusato {
		avviso = fmt.Sprintf("Fornitore %s censito (%s).", f.RagioneSociale, strings.Join(fatti, ", "))
	}
	if d.UsaDominio {
		// un dominio che è anche di un cliente non è un errore (un gruppo che compra e vende), ma
		// i suoi messaggi risulteranno «ambigui» finché un contatto esatto non decide: meglio dirlo
		if c, err := q.GetClientePerDominio(ctx, d.Dominio); err == nil {
			avviso += fmt.Sprintf(" Attenzione: %s è anche il dominio del cliente %s, quindi la posta da quel dominio resta «ambigua» finché non censisci gli indirizzi esatti.", d.Dominio, c.CartellaNas)
		}
	}
	return avviso, nil
}

// censisciCliente crea il cliente con la stessa porta non distruttiva dell'anagrafica
// (`CreaCliente`, `AggiungiDominio`): una cartella già di un altro ferma tutto e dice di chi è; un
// dominio già di un altro lascia il cliente creato e lo dice. Senza transazione, per lo stesso
// motivo di `censisciFornitore`.
func (s *Server) censisciCliente(ctx context.Context, d *censisciDati) (string, error) {
	q := db.New(s.Pool)
	c, err := CreaCliente(ctx, q, db.InsertClienteParams{
		CartellaNas: d.Cartella, RagioneSociale: d.RagioneSociale, Lingua: txtN(d.Lingua, 2)})
	if err != nil {
		return "", err
	}
	var fatti []string
	if d.UsaDominio {
		if err := AggiungiDominio(ctx, q, d.Dominio, c.ClienteID); err != nil {
			return "", fmt.Errorf("cliente %s creato, ma il dominio no: %w", c.RagioneSociale, err)
		}
		fatti = append(fatti, "dominio "+d.Dominio)
	}
	if d.UsaContatto {
		cognome, nome := cognomeENome(d.ContattoNome, d.Indirizzo)
		_, err := q.InsertBuyer(ctx, db.InsertBuyerParams{
			ClienteID: c.ClienteID, Cognome: cognome, Nome: txtN(nome, 60), Email: txtN(d.Indirizzo, 120),
			Tipo: db.TipoBuyerBuyer, Origine: db.OrigineAnagraficaCensimento, Lingua: txtN(d.Lingua, 2)})
		if _, dup := vincoloViolato(err); dup {
			return "", fmt.Errorf("l'indirizzo %s è già di una persona censita presso un altro cliente: l'indirizzo è unico", d.Indirizzo)
		}
		if err != nil {
			return "", err
		}
		fatti = append(fatti, "buyer "+d.Indirizzo)
	}
	return fmt.Sprintf("Cliente %s censito (%s).", c.RagioneSociale, strings.Join(fatti, ", ")), nil
}

// cognomeENome ricava un cognome da «Nome Cognome» (l'ultima parola) o, in mancanza, dalla parte
// dell'indirizzo prima della chiocciola: il buyer vuole un cognome, perché è il nome della cartella.
func cognomeENome(nomeIntero, indirizzo string) (cognome, nome string) {
	parti := strings.Fields(nomeIntero)
	switch {
	case len(parti) >= 2:
		return parti[len(parti)-1], strings.Join(parti[:len(parti)-1], " ")
	case len(parti) == 1:
		return parti[0], ""
	}
	locale, _, _ := strings.Cut(indirizzo, "@")
	if locale == "" {
		locale = indirizzo
	}
	return locale, ""
}

// ritriagePer esegue il ritriage mirato dopo una scrittura in anagrafica (dominio o contatto di
// un fornitore, dominio o buyer di un cliente) e restituisce la frase da accodare all'esito.
//
// Serve perché dalla 0014 la controparte è un FATTO scritto sul messaggio all'ingest: prima
// `v_inbox` risolveva il cliente dal dominio del mittente a ogni lettura, e aggiungere un dominio
// cambiava subito l'Inbox; adesso senza questo ricalcolo i messaggi già arrivati resterebbero
// «sconosciuti». Tocca solo i messaggi non decisi (7A.3).
func (s *Server) ritriagePer(ctx context.Context, indirizzo, dominio string) string {
	esito, err := (&ingest.Servizio{Pool: s.Pool, Log: s.Log}).Ritriage(ctx, indirizzo, dominio)
	if err != nil {
		s.Log.Error("ritriage dopo la scrittura in anagrafica", "indirizzo", indirizzo, "dominio", dominio, "err", err)
		return " Il ricalcolo dei messaggi già arrivati però non è riuscito: " + err.Error()
	}
	return " Messaggi già arrivati: " + esito.String() + "."
}

// AggiungiDominioFornitore assegna un dominio a un fornitore. Come per i clienti (T8): un dominio
// appartiene a UN fornitore, e se è già di un altro l'assegnazione fallisce e dice di chi è. Se è
// già suo non è un errore.
func AggiungiDominioFornitore(ctx context.Context, q *db.Queries, dominio string, fornitoreID uuid.UUID) error {
	dominio = strings.ToLower(strings.TrimSpace(dominio))
	if dominio == "" {
		return errors.New("il dominio è vuoto")
	}
	if strings.Contains(dominio, "@") {
		return fmt.Errorf("%q è un indirizzo, non un dominio: qui va la parte dopo la chiocciola", dominio)
	}
	err := q.InsertDominioFornitore(ctx, db.InsertDominioFornitoreParams{Lower: dominio, FornitoreID: fornitoreID})
	if _, ok := vincoloViolato(err); ok {
		altro, e := q.GetFornitorePerDominio(ctx, dominio)
		if e == nil && altro.FornitoreID == fornitoreID {
			return nil
		}
		if e == nil {
			return fmt.Errorf("il dominio %s è già censito per il fornitore %s: un dominio appartiene a un fornitore solo", dominio, altro.RagioneSociale)
		}
		return fmt.Errorf("il dominio %s è già censito per un altro fornitore", dominio)
	}
	return err
}
