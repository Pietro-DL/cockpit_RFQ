package web

// LE RICHIESTE NUOVE (cockpit/_fasi/mockup_richieste.html): a sinistra le viste e i filtri con i loro numeri,
// al centro l'elenco (o le colonne per fase), a destra la RFQ scelta: a che punto e', che cosa manca, i
// prodotti, i fornitori, le ultime mail e che cosa vuole il cliente, e i passaggi di fase a mano.
//
// La scadenza in parole, il prossimo passo e «di chi e'» si leggono dalla riga della panoramica: non scrivono
// niente e non decidono niente. L'unico gesto nuovo e' il passaggio di fase (fascicolo.PassaFase), e chiede una
// conferma scritta nella pagina prima di partire.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// ufficiRichieste: gli uffici di fase_catalogo che hanno qualcosa da fare. «Sistema» (RICEVUTA, ORDINE) oggi
// e' un'attesa di un gesto del commerciale, e si conta con lui.
var ufficiRichieste = []string{"Commerciale", "Tecnico", "Acquisti", "Uff. tecnico", "Produzione"}

// Con e' l'indirizzo dell'elenco con un filtro cambiato (valore "" = tolto). La RFQ aperta resta aperta,
// l'elenco torna alla prima pagina.
func (f filtriRichieste) Con(k, v string) string {
	g := f
	g.N = cardPerPagina
	switch k {
	case "vista":
		g.Vista, g.Stato = "", statoAperte
		switch v {
		case "oggi":
			g.Vista = "oggi"
		case "chiuse":
			g.Stato = statoChiuse
		case "tutte":
			g.Stato = statoTutte
		}
	case "fase":
		g.Fase = db.Fase(v)
	case "cliente":
		g.Cliente = uuid.Nil
		if id, err := uuid.Parse(v); err == nil {
			g.Cliente = id
		}
	case "uff":
		g.Ufficio = v
	case "bloccanti":
		g.Bloccanti = v != ""
	case "smistare":
		g.DaSmistare = v != ""
	case "sla":
		g.SLA = v != ""
	case "scade":
		g.Scade = v != ""
	case "fornitori":
		g.Fornitori = v != ""
	case "modo":
		g.Modo = v
	case "q":
		g.Q = v
	case "sel":
		g.Sel = v
	case "tutti":
		g = filtriRichieste{Stato: f.Stato, Vista: f.Vista, Ordine: f.Ordine, N: cardPerPagina, Modo: f.Modo, Sel: f.Sel}
	}
	return g.URL()
}

// NomeVista e' il titolo dell'elenco.
func (f filtriRichieste) NomeVista() string {
	switch {
	case f.Vista == "oggi":
		return "Da seguire oggi"
	case f.Stato == statoChiuse:
		return "Richieste chiuse"
	case f.Stato == statoTutte:
		return "Tutte le richieste"
	}
	return "Richieste aperte"
}

// NomeFaseFiltro e' la fase scelta in parole, per i filtri attivi.
func (f filtriRichieste) NomeFaseFiltro() string { return nomeFaseBreve(f.Fase) }

// ---------------------------------------------------------------- i numeri della colonna dei filtri

type contiRichieste struct {
	Oggi, Aperte, Chiuse, Tutte int
	Segnali                     map[string]int
	Fasi, Clienti, Uffici       []voceConta
}

type voceConta struct {
	Chiave, Nome string
	N            int
	Peso         int16
	Fine         bool
}

// daSeguire: la stessa regola del filtro della query («Da seguire oggi»), letta da una riga.
func daSeguire(r db.ListRichiestePanoramicaRow, oggi time.Time) bool {
	if r.StatoThread != db.StatoThreadAPERTA {
		return false
	}
	return (r.DataScadenza != nil && !r.DataScadenza.After(oggi.AddDate(0, 0, 3))) || r.Semaforo.String == "rosso" || !r.SollecitoIl.IsZero()
}

// conta legge tutte le RFQ (al piu' maxCardRichieste) e conta, dentro la vista scelta, quante ne vedresti
// premendo ogni filtro. Una lettura in piu' per pagina: la colonna si rifa' con l'elenco.
func (p *panoramicaRichieste) conta(ctx context.Context, q *db.Queries) error {
	tutte, err := q.ListRichiestePanoramica(ctx, db.ListRichiestePanoramicaParams{Ordine: "priorita", Limite: maxCardRichieste})
	if err != nil {
		return err
	}
	f, c := p.Filtri, contiRichieste{Segnali: map[string]int{}}
	oggi := time.Now().Truncate(24 * time.Hour)
	fasi, clienti, uffici := map[string]*voceConta{}, map[string]*voceConta{}, map[string]*voceConta{}
	var ordFasi, ordClienti []string
	for _, r := range tutte {
		aperta := r.StatoThread == db.StatoThreadAPERTA
		c.Tutte++
		if aperta {
			c.Aperte++
		} else {
			c.Chiuse++
		}
		seguire := daSeguire(r, oggi)
		if seguire {
			c.Oggi++
		}
		switch {
		case f.Vista == "oggi" && !seguire, f.Vista != "oggi" && f.Stato == statoAperte && !aperta, f.Stato == statoChiuse && aperta:
			continue
		}
		if r.NBloccanti > 0 {
			c.Segnali["bloccanti"]++
		}
		if r.NDaSmistare > 0 {
			c.Segnali["smistare"]++
		}
		if r.Semaforo.String == "rosso" {
			c.Segnali["sla"]++
		}
		if aperta && r.DataScadenza != nil && !r.DataScadenza.After(oggi.AddDate(0, 0, 3)) {
			c.Segnali["scade"]++
		}
		if r.NFornAttesa > 0 {
			c.Segnali["fornitori"]++
		}
		if r.NomeFase.Valid {
			k := string(r.NomeFase.Fase)
			if fasi[k] == nil {
				fasi[k] = &voceConta{Chiave: k, Nome: nomeFaseBreve(r.NomeFase.Fase), Fine: fascicolo.Terminale(r.NomeFase.Fase)}
				ordFasi = append(ordFasi, k)
			}
			fasi[k].N++
		}
		k := r.ClienteID.String()
		if clienti[k] == nil {
			clienti[k] = &voceConta{Chiave: k, Nome: r.Cliente, Peso: r.PesoCliente}
			ordClienti = append(ordClienti, k)
		}
		clienti[k].N++
		u := chiDellaFase(r.Ufficio)
		if uffici[u] == nil {
			uffici[u] = &voceConta{Chiave: u, Nome: u}
		}
		uffici[u].N++
	}
	for _, fa := range db.AllFaseValues() {
		if v := fasi[string(fa)]; v != nil {
			c.Fasi = append(c.Fasi, *v)
		}
	}
	for _, k := range ordClienti {
		c.Clienti = append(c.Clienti, *clienti[k])
	}
	for _, u := range ufficiRichieste {
		if v := uffici[u]; v != nil {
			c.Uffici = append(c.Uffici, *v)
		}
	}
	p.Conta = c
	return nil
}

// chiDellaFase: l'ufficio della fase, con «Sistema» che oggi aspetta il commerciale.
func chiDellaFase(u string) string {
	if u == "" || u == "Sistema" || u == "-" {
		return "Commerciale"
	}
	return u
}

func nomeFaseBreve(f db.Fase) string {
	s := strings.ToLower(nomeFase(f))
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ---------------------------------------------------------------- una RFQ in parole

// Chi e' di chi e' la palla: l'ufficio della fase.
func (c richiestaCard) Chi() string { return chiDellaFase(c.Ufficio) }

// Sollecito dice se il buyer ha sollecitato negli ultimi 14 giorni.
func (c richiestaCard) Sollecito() bool { return !c.SollecitoIl.IsZero() }

// Scadenza e' la scadenza in parole: «domani», «fra 3 gg», «scaduta da 2 gg»; Sotto e' la data, Classe il colore.
func (c richiestaCard) Scadenza() (testo, sotto, classe string) {
	if c.DataScadenza == nil {
		return "—", "senza scadenza", "grigio"
	}
	d := c.DataScadenza.Local()
	sotto = giorniIt[d.Weekday()][:3] + " " + d.Format("02/01")
	if c.Chiusa() || (c.NomeFase.Valid && (c.NomeFase.Fase == db.FaseOFFERTAINVIATA || c.NomeFase.Fase == db.FaseACCETTATA || c.NomeFase.Fase == db.FaseORDINE || c.NomeFase.Fase == db.FasePRODUZIONE || c.NomeFase.Fase == db.FaseDISTINTAERP)) {
		return d.Format("02/01"), "scadenza dell'offerta", "grigio"
	}
	oggi := time.Now()
	a := time.Date(oggi.Year(), oggi.Month(), oggi.Day(), 0, 0, 0, 0, time.Local)
	g := int(time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.Local).Sub(a).Hours() / 24)
	switch {
	case g < 0:
		return fmt.Sprintf("scaduta da %d gg", -g), sotto, "rosso"
	case g == 0:
		return "oggi", sotto, "rosso"
	case g == 1:
		return "domani", sotto, "rosso"
	case g <= 3:
		return fmt.Sprintf("fra %d gg", g), sotto, "giallo"
	}
	return fmt.Sprintf("fra %d gg", g), sotto, "verde"
}
func (c richiestaCard) ScadTesto() string  { t, _, _ := c.Scadenza(); return t }
func (c richiestaCard) ScadSotto() string  { _, s, _ := c.Scadenza(); return s }
func (c richiestaCard) ScadClasse() string { _, _, k := c.Scadenza(); return k }

// Urgenza e' il colore del bordo della riga: rosso se scade oggi o domani o e' oltre i giorni della fase,
// giallo se scade entro tre giorni, e' al limite o manca qualcosa di bloccante.
func (c richiestaCard) Urgenza() string {
	if c.Chiusa() {
		return ""
	}
	k := c.ScadClasse()
	switch {
	case k == "rosso" || c.Semaforo.String == "rosso":
		return "rosso"
	case k == "giallo" || c.Semaforo.String == "giallo" || c.NBloccanti > 0:
		return "giallo"
	}
	return ""
}

// SlaPerc e' quanta barra dei giorni di fase e' piena.
func (c richiestaCard) SlaPerc() int {
	if !c.SlaGg.Valid || c.SlaGg.Int32 <= 0 || !c.GgInFase.Valid {
		return 0
	}
	p := int(c.GgInFase.Int32 * 100 / c.SlaGg.Int32)
	return max(6, min(100, p))
}

// Prossimo e' il prossimo passo in parole: dalla fase e dai segnali della riga. Non e' un compito assegnato:
// e' la lettura di quello che manca.
func (c richiestaCard) Prossimo() string {
	if c.Chiusa() || !c.NomeFase.Valid {
		return ""
	}
	p := ""
	switch c.NomeFase.Fase {
	case db.FaseRICEVUTA:
		switch {
		case c.NDaSmistare > 0:
			p = fmt.Sprintf("smistare %d file nella Distinta", c.NDaSmistare)
		case c.NBloccanti > 0:
			p = "procurare i documenti che mancano"
		default:
			p = "confermare la Distinta e passare a Fattibilità"
		}
	case db.FaseATTESADISEGNI:
		p = "chiedere al cliente i documenti che mancano"
	case db.FaseFATTIBILITA:
		p = "fattibilità: si chiude congelando la BOM nella Distinta"
	case db.FaseSCHEDACOSTO:
		p = "compilare la scheda costo"
	case db.FaseOFFERTEFORN:
		if c.NFornAttesa > 0 {
			p = fmt.Sprintf("aspettare %d fornitori", c.NFornAttesa)
			if c.NFornAttesa == 1 {
				p = "aspettare un fornitore"
			}
		} else {
			p = "le quotazioni sono rientrate: tornare alla scheda costo"
		}
	case db.FaseOFFERTAINVIATA:
		p = "aspettare la risposta del buyer"
	case db.FaseACCETTATA:
		p = "aspettare l'ordine"
	case db.FaseDISTINTAERP:
		p = "distinta e cicli nel gestionale"
	case db.FaseORDINE:
		p = "mandare la conferma d'ordine"
	case db.FasePRODUZIONE:
		p = "in produzione"
	}
	if c.Sollecito() {
		p = "il buyer ha sollecitato il " + c.SollecitoIl.Local().Format("02/01") + ": " + p
	}
	return p
}

// Miniature sono le prime quattro schede con un disegno, per la riga.
func (c richiestaCard) Miniature() []prodottoCard {
	var out []prodottoCard
	for _, p := range c.Prodotti {
		if len(out) == 4 {
			break
		}
		out = append(out, p)
	}
	return out
}

// DaConfermare conta le schede con un disegno solo proposto.
func (c richiestaCard) NDaConfermare() int {
	n := 0
	for _, p := range c.Prodotti {
		if p.DaConfermare {
			n++
		}
	}
	return n
}

// AllegatoID e' l'allegato del disegno della scheda, per la miniatura disegnata con pdf.js.
func (p prodottoCard) AllegatoID() string {
	if p.allegato == uuid.Nil {
		return ""
	}
	return p.allegato.String()
}

// ---------------------------------------------------------------- le colonne per fase

type colonnaFase struct {
	Fase                 db.Fase
	Nome, Ufficio, Sotto string
	Card                 []richiestaCard
	Fine                 bool
}

// Colonne mette le RFQ dell'elenco nelle colonne delle fasi, nell'ordine di fase_catalogo. Le fasi di
// chiusura compaiono solo se c'e' una RFQ.
func (p *panoramicaRichieste) Colonne() []colonnaFase {
	per := map[db.Fase][]richiestaCard{}
	for _, c := range p.Card {
		if c.NomeFase.Valid {
			per[c.NomeFase.Fase] = append(per[c.NomeFase.Fase], c)
		}
	}
	var out []colonnaFase
	for _, f := range db.AllFaseValues() {
		fine := fascicolo.Terminale(f)
		if fine && len(per[f]) == 0 {
			continue
		}
		out = append(out, colonnaFase{Fase: f, Nome: nomeFaseBreve(f), Card: per[f], Fine: fine})
	}
	return out
}

// ---------------------------------------------------------------- la RFQ scelta

type tappaFase struct {
	Nome, Quando, Nota string
	Stato              string // fatta | ora | futura
}

type passaggioFase struct {
	A               db.Fase
	Nome, Etichetta string
	Fine            bool
	Link            string // il passaggio si fa altrove (il congelamento, nella Distinta)
}

type dettaglioRichiesta struct {
	Card       richiestaCard
	Tappe      []tappaFase
	Passaggi   []passaggioFase
	Automatico string
	Mancano    []db.VFascicolo
	Fornitori  []db.ListRichiesteThreadRow
	Mail       []db.Messaggio
	Cliente    db.Cliente
	Regole     regole.Regole
	Conferma   *passaggioFase // il passaggio chiesto, in attesa della conferma scritta
	Errore     string
	Fatto      string
	Tutti      bool // i prodotti tutti, non i primi sei
}

// Mostrati sono le schede dei prodotti del pannello.
func (d *dettaglioRichiesta) Mostrati() []prodottoCard {
	if d.Tutti || len(d.Card.Prodotti) <= prodottiInCard {
		return d.Card.Prodotti
	}
	return d.Card.Prodotti[:prodottiInCard]
}

// il percorso di sempre, per le tappe che non sono ancora successe
var percorsoFasi = []db.Fase{db.FaseRICEVUTA, db.FaseFATTIBILITA, db.FaseSCHEDACOSTO, db.FaseOFFERTAINVIATA, db.FaseORDINE, db.FasePRODUZIONE}

// frasiPassaggio: i passaggi a mano in parole, per (da, a). Quelli che mancano usano il fatto richiesto della
// tabella `transizione`.
var frasiPassaggio = map[string]string{
	"RICEVUTA>FATTIBILITA": "Documenti completi", "RICEVUTA>ATTESA_DISEGNI": "Manca qualcosa di bloccante",
	"ATTESA_DISEGNI>FATTIBILITA": "Documenti arrivati", "FATTIBILITA>ATTESA_DISEGNI": "Manca un particolare",
	"FATTIBILITA>RESPINTA": "Non fattibile", "FATTIBILITA>SCHEDA_COSTO": "Fattibilità OK",
	"SCHEDA_COSTO>OFFERTE_FORN": "Servono processi esterni", "SCHEDA_COSTO>OFFERTA_INVIATA": "Offerta mandata al cliente",
	"OFFERTE_FORN>SCHEDA_COSTO": "Quotazioni rientrate", "OFFERTA_INVIATA>SCHEDA_COSTO": "Si rinegozia",
	"OFFERTA_INVIATA>ACCETTATA": "Il buyer accetta", "OFFERTA_INVIATA>ORDINE": "Arrivato l'ordine",
	"OFFERTA_INVIATA>PERSA": "Rifiutata", "OFFERTA_INVIATA>SCADUTA": "Nessuna risposta",
	"ACCETTATA>DISTINTA_ERP": "Accettazione confermata", "ACCETTATA>ORDINE": "Arrivato l'ordine",
	"DISTINTA_ERP>ORDINE": "Arrivato l'ordine", "ORDINE>PRODUZIONE": "Distinta nel gestionale",
}

func (s *Server) caricaDettaglioRichiesta(ctx context.Context, id uuid.UUID) (*dettaglioRichiesta, error) {
	q := db.New(s.Pool)
	righe, err := q.ListRichiestePanoramica(ctx, db.ListRichiestePanoramicaParams{ThreadID: uuid.NullUUID{UUID: id, Valid: true}, Ordine: "priorita", Limite: 1})
	if err != nil {
		return nil, err
	}
	if len(righe) == 0 {
		return nil, errors.New("RFQ non trovata")
	}
	prodotti, err := q.ListProdottiPanoramica(ctx, []uuid.UUID{id})
	if err != nil {
		return nil, err
	}
	anteprime, err := q.ListAnteprimePanoramica(ctx, []uuid.UUID{id})
	if err != nil {
		return nil, err
	}
	p := costruisciPanoramica(filtriRichieste{}, righe, prodotti, anteprime)
	d := &dettaglioRichiesta{Card: p.Card[0]}
	d.Card.Tutti = true

	// le tappe: quelle del registro, poi il percorso di sempre da qui in avanti
	log, err := q.ListFaseLog(ctx, id)
	if err != nil {
		return nil, err
	}
	cat, _ := q.ListFaseCatalogo(ctx)
	desc, uff, sla := map[db.Fase]string{}, map[db.Fase]string{}, map[db.Fase]pgtype.Int4{}
	ordine := map[db.Fase]int32{}
	for _, c := range cat {
		desc[c.NomeFase], uff[c.NomeFase], sla[c.NomeFase], ordine[c.NomeFase] = c.Descrizione, c.Ufficio, c.SlaGg, c.Ordine
	}
	var corrente db.Fase
	for _, l := range log {
		t := tappaFase{Nome: nomeFaseBreve(l.NomeFase), Quando: l.Inizio.Local().Format("02/01"), Stato: "fatta", Nota: desc[l.NomeFase]}
		if l.Fine == nil {
			corrente, t.Stato = l.NomeFase, "ora"
			t.Nota = fmt.Sprintf("da %d gg", int(time.Since(l.Inizio).Hours()/24))
			if v := sla[l.NomeFase]; v.Valid && v.Int32 > 0 {
				t.Nota += fmt.Sprintf(" · SLA %d gg", v.Int32)
			}
			t.Nota += " · " + chiDellaFase(uff[l.NomeFase])
		}
		d.Tappe = append(d.Tappe, t)
	}
	if corrente != "" && !fascicolo.Terminale(corrente) {
		for _, f := range percorsoFasi {
			if ordine[f] > ordine[corrente] {
				d.Tappe = append(d.Tappe, tappaFase{Nome: nomeFaseBreve(f), Stato: "futura", Nota: desc[f]})
			}
		}
		archi, _ := q.ListTransizioniDa(ctx, corrente)
		for _, a := range archi {
			ps := passaggioFase{A: a.A, Nome: nomeFaseBreve(a.A), Etichetta: frasiPassaggio[string(a.Da)+">"+string(a.A)], Fine: fascicolo.Terminale(a.A)}
			if ps.Etichetta == "" {
				ps.Etichetta = a.FattoRichiesto.String
			}
			ok, _ := fascicolo.PassaggioAMano(a)
			switch {
			case a.Da == db.FaseFATTIBILITA && a.A == db.FaseSCHEDACOSTO:
				ps.Link = "/thread/" + id.String() + "/distinta"
			case !ok:
				continue
			}
			d.Passaggi = append(d.Passaggi, ps)
		}
		if automaticiDa := corrente == db.FaseRICEVUTA || corrente == db.FaseATTESADISEGNI; automaticiDa {
			d.Automatico = "Questi passaggi un giorno saranno automatici (quando la Distinta è completa); finché non lo sono, si fanno qui."
		}
	}
	if fasc, err := q.ListFascicolo(ctx, id); err == nil {
		for _, f := range fasc {
			if f.Bloccante && f.Esito != "ok" && f.Esito != "ok_in_coda" && f.Esito != "ok_errore_nas" && f.Esito != "derogato" {
				d.Mancano = append(d.Mancano, f)
			}
		}
	}
	d.Fornitori, _ = q.ListRichiesteThread(ctx, id)
	if mm, err := q.ListMessaggiThread(ctx, uuid.NullUUID{UUID: id, Valid: true}); err == nil {
		for i := len(mm) - 1; i >= 0 && len(d.Mail) < 5; i-- {
			d.Mail = append(d.Mail, mm[i])
		}
	}
	if c, err := q.GetCliente(ctx, d.Card.ClienteID); err == nil {
		d.Cliente = c
		d.Regole, _ = regole.LeggiRegole(c.Regole)
	}
	return d, nil
}

// richiestaDettaglio: GET /richieste/{id}/dettaglio, la RFQ scelta accanto all'elenco. Con ?passo=FASE mostra
// la conferma scritta del passaggio; con ?prodotti=tutti tutte le schede.
func (s *Server) richiestaDettaglio(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	d, err := s.caricaDettaglioRichiesta(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	if a := db.Fase(r.URL.Query().Get("passo")); a != "" {
		for i := range d.Passaggi {
			if d.Passaggi[i].A == a && d.Passaggi[i].Link == "" {
				d.Conferma = &d.Passaggi[i]
			}
		}
	}
	d.Tutti = r.URL.Query().Get("prodotti") == "tutti"
	s.rendiDettaglioRichiesta(w, r, d)
}

func (s *Server) rendiDettaglioRichiesta(w http.ResponseWriter, r *http.Request, d *dettaglioRichiesta) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pagine["richieste.html"].ExecuteTemplate(w, "richiesta_dettaglio", vista{Utente: utenteDa(r.Context()), Dati: d, Frammento: true}); err != nil {
		s.Log.Error("template", "frammento", "richiesta_dettaglio", "err", err)
	}
}

// passaFase: POST /thread/{id}/fase, il passaggio di fase a mano dopo la conferma scritta. Risponde con la RFQ
// ridisegnata e dice all'elenco di rifarsi.
func (s *Server) passaFase(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	a := db.Fase(r.FormValue("a"))
	if !a.Valid() {
		http.Error(w, "fase non valida", 400)
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
	errPasso := fascicolo.PassaFase(ctx, db.New(tx), id, a, u.UtenteID)
	var no fascicolo.Rifiuto
	if errPasso != nil && !errors.As(errPasso, &no) {
		http.Error(w, errPasso.Error(), 500)
		return
	}
	if errPasso == nil {
		if err := tx.Commit(ctx); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	d, err := s.caricaDettaglioRichiesta(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	if errPasso != nil {
		d.Errore = "Non passata: " + errPasso.Error() + "."
	} else {
		d.Fatto = "Passata a " + nomeFaseBreve(a) + "."
		w.Header().Set("HX-Trigger", "richieste-aggiorna")
	}
	s.rendiDettaglioRichiesta(w, r, d)
}

// linkPosta e' la ricerca dell'Inbox con il riferimento (o l'oggetto) della RFQ.
func (c richiestaCard) LinkPosta() string {
	k := c.RiferimentoCliente.String
	if k == "" {
		k = c.Oggetto.String
	}
	return "/inbox?" + url.Values{"cerca": {k}}.Encode()
}
