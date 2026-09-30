package web

// L'INBOX NUOVA (cockpit/_fasi/mockup_inbox.html): le righe raggruppate, «Da fare adesso», la proposta del
// Cockpit scritta in parole accanto alla mail, «Rimetti fra i da decidere».
//
// Niente qui decide al posto di chi legge. Il gruppo di una riga e la frase «→ Nuova RFQ» sono la stessa
// proposta del triage che la riga mostrava gia' (chipTriage, coloreTriage), detta senza numeri; la decisione
// resta un gesto: un bottone che porta la sua RFQ, un form, «Ignora».

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// vistaAdesso e' il valore di `q` di «Da fare adesso»: tutte le mail da decidere, di ogni quadrante,
// raggruppate per tipo di decisione. E' la vista con cui l'Inbox si apre.
const vistaAdesso = "adesso"

// gruppi di «Da fare adesso», nell'ordine in cui si lavorano.
var gruppiInbox = []struct{ Chiave, Nome, Sotto string }{
	{"nuove", "Richieste nuove", "si crea la RFQ"},
	{"aggancia", "Da mettere in una RFQ", "si aggancia"},
	{"fornitori", "Fornitori", "offerte e nostre richieste"},
	{"chi", "Mittenti da riconoscere", "si censisce"},
	{"altre", "Da guardare", "nessuna proposta"},
	{"ignora", "Probabilmente da ignorare", "newsletter e notifiche"},
	{"fatte", "Gia' decise", ""},
}

// rigaInbox e' una riga della lista: la riga di v_inbox, l'intestazione del gruppo o del giorno che la
// precede (solo sulla prima), e la proposta in parole.
type rigaInbox struct {
	db.VInbox
	Testa      *testaGruppo
	Gruppo     string
	Cosa       string // «→ Nuova RFQ», «→ RFQ 490020884», vuoto se non c'e' niente da proporre
	CosaColore string // verde | giallo | rosso | grigio
	Striscia   string // il colore del bordo: verde | giallo | rosso | ""
}

type testaGruppo struct {
	Nome, Sotto string
	N           int
}

// gruppoRiga da' a una riga il suo gruppo e il suo colore, e le parole che il chip del triage non dice: chi e'
// il mittente quando non si sa, e il nome della RFQ proposta. Legge soltanto la riga (e quel nome): e' la
// stessa lettura di chipTriage e coloreTriage.
func gruppoRiga(r db.VInbox, nomi map[uuid.UUID]string) (gruppo, cosa, colore string) {
	switch {
	case r.ThreadID.Valid:
		return "fatte", "", ""
	case r.Ignorato:
		return "fatte", "", ""
	}
	ct := r.ControparteTipo
	if ct == string(db.TipoControparteSconosciuto) || ct == string(db.TipoControparteAmbiguo) {
		if ct == string(db.TipoControparteAmbiguo) {
			return "chi", "→ cliente o fornitore?", "rosso"
		}
		return "chi", "→ chi è?", "rosso"
	}
	pari := pariMeritoNeiMotivi(r.TriageMotivi)
	colore = coloreDelTriage(r.TriageEsito, r.TriageConfidenza, r.ThreadProposto, ct, pari)
	suo := "aggancia"
	if ct == string(db.TipoControparteFornitore) {
		suo = "fornitori"
	}
	switch r.TriageEsito.String {
	case "ignora":
		return "ignora", "", "grigio"
	case "nuova_rfq":
		return "nuove", "", colore
	case "aggancia":
		if rispostaARichiesta(r.ThreadProposto, ct, pari) {
			return "fornitori", "", "verde"
		}
		if pari || !r.ThreadProposto.Valid {
			return suo, "", "giallo"
		}
		if nome := nomi[r.ThreadProposto.UUID]; nome != "" {
			return suo, "→ RFQ " + nome, colore
		}
		return suo, "", colore
	}
	if ct == string(db.TipoControparteFornitore) {
		return "fornitori", "", ""
	}
	return "altre", "", ""
}

// righeInbox costruisce la lista: in «Da fare adesso» per gruppo, altrimenti per giorno. L'ordine dentro un
// gruppo o un giorno resta quello della query (la mail piu' recente in cima).
func righeInbox(ctx context.Context, q *db.Queries, righe []db.VInbox, perGruppo bool) []rigaInbox {
	var ids []uuid.UUID
	for _, r := range righe {
		if r.ThreadProposto.Valid {
			ids = append(ids, r.ThreadProposto.UUID)
		}
	}
	nomi := map[uuid.UUID]string{}
	if len(ids) > 0 && q != nil {
		if nn, err := q.NomiThread(ctx, ids); err == nil {
			for _, n := range nn {
				nomi[n.ThreadID] = breve(n.Nome, 28)
			}
		}
	}
	out := make([]rigaInbox, 0, len(righe))
	for _, r := range righe {
		g, cosa, col := gruppoRiga(r, nomi)
		x := rigaInbox{VInbox: r, Gruppo: g, Cosa: cosa, CosaColore: col}
		if col != "grigio" {
			x.Striscia = col
		}
		out = append(out, x)
	}
	if perGruppo {
		ord := map[string]int{}
		for i, g := range gruppiInbox {
			ord[g.Chiave] = i
		}
		sort.SliceStable(out, func(i, j int) bool { return ord[out[i].Gruppo] < ord[out[j].Gruppo] })
	}
	chiave := func(x rigaInbox) string {
		if perGruppo {
			return x.Gruppo
		}
		return x.DataEvento.Local().Format("2006-01-02")
	}
	for i := 0; i < len(out); {
		k, j := chiave(out[i]), i
		for j < len(out) && chiave(out[j]) == k {
			j++
		}
		t := &testaGruppo{N: j - i}
		if perGruppo {
			for _, g := range gruppiInbox {
				if g.Chiave == k {
					t.Nome, t.Sotto = g.Nome, g.Sotto
				}
			}
		} else {
			t.Nome, t.Sotto = nomeGiorno(out[i].DataEvento)
		}
		out[i].Testa = t
		i = j
	}
	return out
}

var giorniIt = []string{"domenica", "lunedì", "martedì", "mercoledì", "giovedì", "venerdì", "sabato"}
var mesiIt = []string{"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno", "luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre"}

// nomeGiorno e' l'intestazione di un giorno della lista: «Oggi · martedì 29 settembre», «Ieri · …», o la data.
func nomeGiorno(t time.Time) (string, string) {
	t = t.Local()
	pieno := fmt.Sprintf("%s %d %s", giorniIt[t.Weekday()], t.Day(), mesiIt[t.Month()-1])
	oggi := time.Now().Local()
	a := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	b := time.Date(oggi.Year(), oggi.Month(), oggi.Day(), 0, 0, 0, 0, time.Local)
	switch int(b.Sub(a).Hours() / 24) {
	case 0:
		return "Oggi", pieno
	case 1:
		return "Ieri", pieno
	}
	if t.Year() != oggi.Year() {
		pieno += fmt.Sprintf(" %d", t.Year())
	}
	return pieno, ""
}

// quandoBreve e' l'ora di oggi, «ieri 16:20», o la data: per le righe che non stanno sotto il loro giorno.
func quandoBreve(t time.Time) string {
	t = t.Local()
	g, _ := nomeGiorno(t)
	switch g {
	case "Oggi":
		return t.Format("15:04")
	case "Ieri":
		return "ieri " + t.Format("15:04")
	}
	return t.Format("02/01")
}

func breve(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "…"
}

// ---------------------------------------------------------------- «Che cosa fare», accanto alla mail

// decisioneVista e' la carta in cima a «Che cosa fare»: che cosa propone il Cockpit, in parole, e con quale
// gesto. Il Tipo sceglie i bottoni nel template; ThreadID e' la RFQ che il bottone principale porta, solo
// quando la proposta e' una e forte (con piu' RFQ alla pari nessuna e' proposta).
type decisioneVista struct {
	Tipo       string // nuova | aggancia | scegli | richiesta | manuale | chi | ignora | altre | agganciato | ignorato
	Titolo     string
	Sotto      string
	Tono       string // ok | warn | bad | acc | ""
	Motivi     []string
	ThreadID   uuid.UUID
	ThreadNome string
}

func (d *messaggioDati) decisione(ctx context.Context, q *db.Queries) decisioneVista {
	r := d.Riga
	motivi := motiviProposta(r.TriageMotivi)
	ct := r.ControparteTipo
	switch {
	case d.Thread != nil:
		return decisioneVista{Tipo: "agganciato", Tono: "acc", Titolo: "Nella RFQ " + nomeThread(*d.Thread),
			Sotto: strings.TrimSpace(r.Cliente.String + " · " + d.Thread.CartellaRelativa.String), ThreadID: d.Thread.ThreadID}
	case r.Ignorato:
		return decisioneVista{Tipo: "ignorato", Titolo: "Ignorato", Sotto: "Resta in archivio, fra gli ignorati."}
	case len(d.CandidatiRichiesta) > 0:
		return decisioneVista{Tipo: "richiesta", Tono: "ok", Titolo: "Sembra la risposta a una nostra richiesta",
			Sotto: "Scegli qui sotto a quale, e che cos'è.", Motivi: motivi}
	case d.PropostaThread != nil && d.PropostaFornitore != nil:
		return decisioneVista{Tipo: "manuale", Tono: "warn", Titolo: "Sembra una richiesta a " + d.PropostaFornitore.RagioneSociale + " mandata a mano",
			Sotto: "Per la RFQ " + nomeThread(*d.PropostaThread) + " · non è partita dal Cockpit", Motivi: motivi}
	case ct == string(db.TipoControparteAmbiguo):
		return decisioneVista{Tipo: "chi", Tono: "bad", Titolo: "Cliente o fornitore?",
			Sotto: "L'indirizzo o il dominio stanno in più anagrafiche: decidi tu.", Motivi: motivi}
	case ct == string(db.TipoControparteSconosciuto):
		return decisioneVista{Tipo: "chi", Tono: "bad", Titolo: "Prima di tutto: chi è?",
			Sotto: "Mittente non censito. Censito lui, questa mail e le altre non decise dello stesso indirizzo cambiano quadrante e proposta.", Motivi: motivi}
	}
	switch r.TriageEsito.String {
	case "ignora":
		return decisioneVista{Tipo: "ignora", Titolo: "Sembra da ignorare", Sotto: "Nessuna richiesta e nessuna RFQ.", Motivi: motivi}
	case "nuova_rfq":
		if ct == string(db.TipoControparteFornitore) {
			// 7A: un fornitore non apre una RFQ cliente, per costruzione e non per punteggio
			return decisioneVista{Tipo: "altre", Titolo: "Posta di un fornitore",
				Sotto: "Il triage la legge come una richiesta nuova, ma un fornitore non apre una RFQ cliente: si aggancia alla RFQ per cui lavora, o si ignora.", Motivi: motivi}
		}
		tono := "warn"
		if coloreTriage(r) == "verde" {
			tono = "ok"
		}
		sotto := r.Cliente.String
		if d.M.Direzione == db.DirezioneEntrata && ct == string(db.TipoControparteInterno) {
			sotto = "Un collega la gira: il buyer si legge dal messaggio inoltrato."
		}
		return decisioneVista{Tipo: "nuova", Tono: tono, Titolo: "Sembra una richiesta nuova", Sotto: sotto, Motivi: motivi}
	case "aggancia":
		if pariMeritoNeiMotivi(r.TriageMotivi) || !r.ThreadProposto.Valid {
			return decisioneVista{Tipo: "scegli", Tono: "warn", Titolo: "Più RFQ con la stessa forza",
				Sotto: "Nessuna è proposta: scegli fra i candidati qui sotto, guardando le evidenze.", Motivi: motivi}
		}
		v := decisioneVista{Tipo: "aggancia", Tono: "ok", Titolo: "Va in una RFQ aperta", Motivi: motivi, ThreadID: r.ThreadProposto.UUID}
		if coloreTriage(r) != "verde" {
			v.Tono = "warn"
		}
		if q == nil {
			return v
		}
		if t, err := q.GetThread(ctx, r.ThreadProposto.UUID); err == nil {
			v.ThreadNome = nomeThread(t)
			v.Titolo = "Va nella RFQ " + v.ThreadNome
			v.Sotto = t.CartellaRelativa.String
		}
		return v
	}
	return decisioneVista{Tipo: "altre", Titolo: "Nessuna proposta", Sotto: "Il Cockpit non ha trovato una richiesta a cui legarla: decidi tu.", Motivi: motivi}
}

// Scelta e' la carta «Che cosa fare» che il pannello mostra: quella del gestore o, per chi disegna il pannello
// senza passarci (le prove dei template), la stessa ricavata dai dati che ha, senza il nome della RFQ proposta.
func (d messaggioDati) Scelta() decisioneVista {
	if d.Decisione.Tipo != "" {
		return d.Decisione
	}
	return d.decisione(context.Background(), nil)
}

// nomeThread e' il nome breve di una RFQ: il riferimento del cliente, o l'oggetto.
func nomeThread(t db.ThreadOfferta) string {
	if t.RiferimentoCliente.Valid && t.RiferimentoCliente.String != "" {
		return t.RiferimentoCliente.String
	}
	if t.Oggetto.Valid && t.Oggetto.String != "" {
		return breve(t.Oggetto.String, 40)
	}
	return t.CartellaRelativa.String
}

// dominiPubblici: un dominio di posta di tutti non fa «stessa richiesta» (ListVicini).
var dominiPubblici = []string{"gmail.com", "libero.it", "hotmail.com", "hotmail.it", "outlook.com", "outlook.it", "yahoo.com", "yahoo.it", "icloud.com", "pec.it", "legalmail.it", "tiscali.it", "alice.it", "virgilio.it"}

// ---------------------------------------------------------------- «Rimetti fra i da decidere»

// ripristina toglie un «Ignora»: la riga che il gesto aveva scritto da sola sparisce, una proposta vera che
// aveva rifiutato torna proposta. Nel registro delle decisioni resta traccia di tutti e due i gesti.
func (s *Server) ripristina(w http.ResponseWriter, r *http.Request) {
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
	a, err := q.TogliIgnoraOperatore(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	b, err := q.RiapriPropostaRifiutata(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if a+b == 0 {
		s.pannelloConAvviso(w, r, id, "Il messaggio non era ignorato: niente da rimettere.")
		return
	}
	// «sgancia»: il registro ammette quattro gesti (0003) e questo e' il rovescio di una decisione.
	if err := logDecisione(ctx, q, id, uuid.NullUUID{}, "sgancia", u, "rimesso fra i da decidere dopo un «Ignora»"); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("HX-Trigger", "inbox-aggiorna")
	s.pannelloConAvviso(w, r, id, "Rimesso fra i da decidere.")
}

// ---------------------------------------------------------------- lo stato della posta, in parole

// postaInbox e' il riquadro «La posta» in fondo alle cartelle, e l'avviso sopra la lista quando una casella
// non risponde. Si ricarica da solo: e' la stessa lettura della testata (stato), detta in parole.
func (s *Server) postaInbox(w http.ResponseWriter, r *http.Request) {
	st := s.stato(r.Context(), sessioneDa(r.Context()))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pagine["inbox.html"].ExecuteTemplate(w, "inbox_posta", vista{Utente: utenteDa(r.Context()), Stato: st, Dati: s.descrizioneSync(), Frammento: true}); err != nil {
		s.Log.Error("template", "frammento", "inbox_posta", "err", err)
	}
}

// censisciAltro mette il mittente fra i soggetti «altro» (corrieri, banche, newsletter): con la sua etichetta,
// l'indirizzo o il dominio come recapito. La posta di un «altro» va nel quadrante Altro e non chiede piu' di
// essere riconosciuta. Se l'etichetta c'e' gia' il recapito si aggiunge a lei.
func (s *Server) censisciAltro(ctx context.Context, d *censisciDati, etichetta string) (string, error) {
	q := db.New(s.Pool)
	sog, err := q.GetSoggettoAltroPerEtichetta(ctx, etichetta)
	if err != nil {
		if sog, err = q.InsertSoggettoAltro(ctx, db.InsertSoggettoAltroParams{Etichetta: etichetta, Note: pgtype.Text{}}); err != nil {
			return "", err
		}
	}
	recapito := d.Indirizzo
	if d.UsaDominio {
		recapito = d.Dominio
	}
	if err := q.InsertRecapitoAltro(ctx, db.InsertRecapitoAltroParams{Lower: recapito, AltroID: sog.AltroID}); err != nil {
		if _, dup := vincoloViolato(err); dup {
			return "", fmt.Errorf("%s è già fra i recapiti di un soggetto «altro»", recapito)
		}
		return "", err
	}
	return fmt.Sprintf("%s messo fra «altro» come «%s».", recapito, sog.Etichetta), nil
}

// ignoraDopoIlCensimento e' la seconda meta' di «Ignora e mettilo fra Altro»: la stessa scrittura di «Ignora».
func (s *Server) ignoraDopoIlCensimento(ctx context.Context, id uuid.UUID, u *db.Utente) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	m, deciso, err := messaggioDaDecidere(ctx, q, id)
	if err != nil || deciso {
		return err
	}
	scelta, err := fotografia(ctx, q, m, classificazione.GestoIgnora, uuid.Nil)
	if err != nil {
		return err
	}
	if err := q.IgnoraMessaggio(ctx, db.IgnoraMessaggioParams{MessaggioID: id, DecisoDa: uuid.NullUUID{UUID: u.UtenteID, Valid: true}}); err != nil {
		return err
	}
	if err := logDecisione(ctx, q, id, uuid.NullUUID{}, "ignora", u, scelta.Motivo()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ---------------------------------------------------------------- i link della schermata

// Voci sono le righe della lista con le intestazioni: quelle del gestore o, per chi disegna la lista senza
// passarci (le prove dei template), le stesse ricavate dalle righe, senza il nome delle RFQ proposte.
func (d inboxDati) Voci() []rigaInbox {
	if d.Lista != nil || len(d.Righe) == 0 {
		return d.Lista
	}
	return righeInbox(context.Background(), nil, d.Righe, d.Adesso)
}

// Link e' l'indirizzo della lista con quadrante, direzione e filtro dati, e con la casella e la mail aperta di
// adesso: cambiare cartella non e' un motivo per chiudere la mail che si sta leggendo. La ricerca non segue:
// chi sceglie una cartella la lascia.
func (d inboxDati) Link(q, dir, filtro string) string {
	v := url.Values{}
	v.Set("q", q)
	v.Set("dir", dir)
	v.Set("filtro", filtro)
	v.Set("casella", d.Casella)
	v.Set("sel", d.Selezion)
	return "/inbox?" + v.Encode()
}

// LinkSel e' l'indirizzo della schermata di adesso con un'altra mail aperta (la barra degli indirizzi e'
// lo stato: il poll la rilegge).
func (d inboxDati) LinkSel(id uuid.UUID) string {
	v := url.Values{}
	v.Set("q", d.QuadranteURL())
	v.Set("dir", d.Direzione)
	v.Set("filtro", d.Filtro)
	v.Set("casella", d.Casella)
	v.Set("sel", id.String())
	if d.Cerca != "" {
		v.Set("cerca", d.Cerca)
	}
	return "/inbox?" + v.Encode()
}

// FiltroQuadrante e' il filtro con cui si apre un quadrante: quello scelto, o «da decidere» uscendo da
// «Da fare adesso» e dalla ricerca.
func (d inboxDati) FiltroQuadrante() string {
	if d.Adesso || d.Cerca != "" {
		return "orfani"
	}
	return d.Filtro
}

// AiutoQuadrante e' la frase del quadrante scelto.
func (d inboxDati) AiutoQuadrante() string {
	for _, q := range quadranti {
		if q.Chiave == d.Quadrante {
			return q.Aiuto
		}
	}
	return "Tutti i quadranti insieme."
}

// oraRiga e' l'ora di una riga che sta sotto il suo giorno.
func oraRiga(t time.Time) string { return t.Local().Format("15:04") }

// iniziali sono le lettere dell'avatar: dal nome («Monterastelli Noemi» → MN) o dall'indirizzo.
func iniziali(nome, indirizzo string) string {
	parti := strings.Fields(nome)
	if len(parti) == 0 {
		locale, _, _ := strings.Cut(indirizzo, "@")
		parti = strings.FieldsFunc(locale, func(r rune) bool { return r == '.' || r == '_' || r == '-' })
	}
	out := ""
	for _, p := range parti {
		if r := []rune(p); len(r) > 0 && len([]rune(out)) < 2 {
			out += strings.ToUpper(string(r[0]))
		}
	}
	if out == "" {
		return "?"
	}
	return out
}

// estensione e' l'etichetta del file nella lista degli allegati: PDF, STP, ZIP. Quattro lettere al piu'.
func estensione(nome string) string {
	i := strings.LastIndex(nome, ".")
	if i < 0 || i == len(nome)-1 {
		return "FILE"
	}
	e := strings.ToUpper(nome[i+1:])
	if r := []rune(e); len(r) > 4 {
		e = string(r[:4])
	}
	return e
}
