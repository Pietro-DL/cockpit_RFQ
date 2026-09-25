package web

// La pagina Richieste: una card per RFQ e, dentro, una scheda per ogni prodotto della richiesta.
//
// Attenzione al nome: richieste.go e' la richiesta d'offerta a un FORNITORE. Questa e' la lista delle
// RFQ dei clienti, la rotta GET /richieste di routes_rfq.go.
//
// Fino a qui la pagina era una tabella delle sole RFQ aperte, al massimo 200: una lista di lavoro, non
// una panoramica. Adesso le RFQ sono tutte (aperte e chiuse), e la barra in alto le filtra: testo
// libero, cliente, fase, stato, e i tre filtri rapidi che contano davvero (bloccanti, da smistare, SLA
// critico). I filtri stanno nell'indirizzo, cosi' una ricerca si salva, si manda a un collega e torna
// con «indietro».
//
// La gerarchia e' quella del database, non quella di un mockup:
//
//	RFQ       thread_offerta          una card; l'UUID resta nel tooltip, non in faccia
//	cliente   thread_offerta.cliente  la testata della card e un filtro solo
//	fase      fase_log corrente       il badge; le fasi del filtro nell'ordine di fase_catalogo
//	prodotto  identificativo_thread   una scheda, anche se il componente del Fascicolo non c'e' ancora
//	anteprima dato derivato           il 2D corrente del codice, o la proposta 2D aperta
//
// Le letture sono tre per pagina, qualunque sia il numero di RFQ (panoramica.sql), e le mette insieme
// costruisciPanoramica, che non tocca il database. Un PDF per ogni codice di ogni RFQ distruggerebbe
// la pagina (ci sono richieste con 120 codici): una card ne mostra al piu' sei, gli iframe si caricano
// solo quando arrivano sullo schermo, e il resto si apre su richiesta.
//
// Il poll ogni 60 secondi porta la firma di quello che la pagina mostra: se l'elenco non e' cambiato il
// server risponde 204 e il browser non tocca niente, altrimenti ogni minuto ricaricherebbe tutte le
// anteprime aperte.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

const (
	cardPerPagina     = 30  // le card della prima pagina; «Mostra altre» ne aggiunge altrettante
	maxCardRichieste  = 600 // oltre non si va con «Mostra altre»: si filtra
	prodottiInCard    = 6   // le schede con anteprima visibili in una card chiusa
	maxTestoRicerca   = 100
	pollRichiestePred = 60 * time.Second
)

// Gli stati della barra. Senza «stato» nell'indirizzo la pagina e' la lista di lavoro: le aperte.
const (
	statoAperte = "APERTA"
	statoChiuse = "CHIUSA"
	statoTutte  = "tutte"
)

// Gli ordinamenti. «priorita» e' quello di sempre: peso del cliente, poi scadenza. Il punteggio
// dell'addendum 2 non esiste ancora, e qui non lo si inventa.
var ordiniRichieste = []struct{ Chiave, Etichetta string }{
	{"priorita", "Priorità (peso cliente, scadenza)"},
	{"scadenza", "Scadenza"},
	{"aggiornamento", "Ultimo aggiornamento"},
}

// filtriRichieste e' la barra in alto, letta dall'indirizzo. Un valore che non si capisce si ignora:
// un segnalibro vecchio deve aprire la pagina, non un errore.
type filtriRichieste struct {
	Q          string
	Cliente    uuid.UUID // Nil = tutti
	Fase       db.Fase   // "" = tutte
	Stato      string    // statoAperte, statoChiuse, statoTutte
	Bloccanti  bool
	DaSmistare bool
	SLA        bool
	Ordine     string
	N          int // quante card: cardPerPagina, poi a passi di cardPerPagina
}

func leggiFiltriRichieste(v url.Values) filtriRichieste {
	f := filtriRichieste{Q: strings.TrimSpace(v.Get("q")), Stato: statoAperte, Ordine: "priorita", N: cardPerPagina}
	if r := []rune(f.Q); len(r) > maxTestoRicerca {
		f.Q = string(r[:maxTestoRicerca])
	}
	if id, err := uuid.Parse(v.Get("cliente")); err == nil {
		f.Cliente = id
	}
	if fa := db.Fase(v.Get("fase")); fa.Valid() {
		f.Fase = fa
	}
	switch strings.ToUpper(v.Get("stato")) {
	case statoChiuse:
		f.Stato = statoChiuse
	case strings.ToUpper(statoTutte):
		f.Stato = statoTutte
	}
	f.Bloccanti, f.DaSmistare, f.SLA = v.Get("bloccanti") == "1", v.Get("smistare") == "1", v.Get("sla") == "1"
	for _, o := range ordiniRichieste {
		if v.Get("sort") == o.Chiave {
			f.Ordine = o.Chiave
		}
	}
	if n, err := strconv.Atoi(v.Get("n")); err == nil && n > cardPerPagina {
		n = (n + cardPerPagina - 1) / cardPerPagina * cardPerPagina
		f.N = min(n, maxCardRichieste)
	}
	return f
}

// valori sono i filtri come parametri dell'indirizzo, senza quelli al valore predefinito: e' la forma
// che si legge nella barra del browser e che il poll rimanda.
func (f filtriRichieste) valori() url.Values {
	v := url.Values{}
	if f.Q != "" {
		v.Set("q", f.Q)
	}
	if f.Cliente != uuid.Nil {
		v.Set("cliente", f.Cliente.String())
	}
	if f.Fase != "" {
		v.Set("fase", string(f.Fase))
	}
	if f.Stato != statoAperte {
		v.Set("stato", f.Stato)
	}
	for _, b := range []struct {
		k  string
		on bool
	}{{"bloccanti", f.Bloccanti}, {"smistare", f.DaSmistare}, {"sla", f.SLA}} {
		if b.on {
			v.Set(b.k, "1")
		}
	}
	if f.Ordine != "priorita" {
		v.Set("sort", f.Ordine)
	}
	if f.N != cardPerPagina {
		v.Set("n", strconv.Itoa(f.N))
	}
	return v
}

// URL e' l'indirizzo della pagina con questi filtri.
func (f filtriRichieste) URL() string {
	if q := f.valori().Encode(); q != "" {
		return "/richieste?" + q
	}
	return "/richieste"
}

// Altre e' l'indirizzo di «Mostra altre»: gli stessi filtri con una pagina di card in piu'.
func (f filtriRichieste) Altre() string {
	g := f
	g.N = min(f.N+cardPerPagina, maxCardRichieste)
	return g.URL()
}

// Attivi dice se qualcosa restringe l'elenco rispetto alla lista di lavoro (le aperte, tutte).
func (f filtriRichieste) Attivi() bool {
	return f.Q != "" || f.Cliente != uuid.Nil || f.Fase != "" || f.Stato != statoAperte || f.Bloccanti || f.DaSmistare || f.SLA
}

func (f filtriRichieste) parametri() db.ListRichiestePanoramicaParams {
	p := db.ListRichiestePanoramicaParams{
		ClienteID:     uuid.NullUUID{UUID: f.Cliente, Valid: f.Cliente != uuid.Nil},
		Fase:          db.NullFase{Fase: f.Fase, Valid: f.Fase != ""},
		SoloBloccanti: f.Bloccanti, SoloDaSmistare: f.DaSmistare, SoloSlaCritico: f.SLA,
		Modello: modelloRicerca(f.Q), Ordine: f.Ordine, Limite: int32(f.N),
	}
	if f.Stato != statoTutte {
		p.Stato = db.NullStatoThread{StatoThread: db.StatoThread(f.Stato), Valid: true}
	}
	return p
}

// modelloRicerca e' il testo della barra come modello ILIKE: '%testo%', con % _ e \ presi alla lettera
// (in PostgreSQL \ e' il carattere di escape predefinito di LIKE). Chi cerca «7120_400» cerca quello.
func modelloRicerca(q string) pgtype.Text {
	if q == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: "%" + testoLetterale(q) + "%", Valid: true}
}

// testoLetterale protegge % _ e \ di un testo che una query mette dentro un modello LIKE/ILIKE: la
// meta' di modelloRicerca che serve anche alla ricerca di «Aggancia a…», dove i '%' li aggiunge la query.
func testoLetterale(q string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
}

// panoramicaRichieste e' quello che la pagina disegna.
type panoramicaRichieste struct {
	Filtri     filtriRichieste
	Card       []richiestaCard
	Totale     int64 // le RFQ che rispondono ai filtri
	Firma      string
	Intervallo int // secondi fra un poll e l'altro
	// Per la barra, solo nella pagina intera: i clienti che hanno almeno una RFQ, le fasi nell'ordine
	// di fase_catalogo, gli ordinamenti. Gia' con la scelta corrente segnata.
	Clienti []opzione
	Fasi    []opzione
	Ordini  []opzione
	Stati   []opzione
}

// opzione e' una voce di una tendina o di un gruppo di scelte della barra.
type opzione struct {
	Valore, Etichetta, Titolo string
	Scelta                    bool
}

// Altre sono le RFQ che rispondono ai filtri e non sono ancora nell'elenco.
func (p *panoramicaRichieste) Altre() int64 {
	if d := p.Totale - int64(len(p.Card)); d > 0 {
		return d
	}
	return 0
}

// richiestaCard e' una RFQ: la sua riga di v_cruscotto e i suoi prodotti.
type richiestaCard struct {
	db.ListRichiestePanoramicaRow
	Prodotti []prodottoCard
	Tutti    bool // la card e' aperta su tutti i prodotti (dopo «+ N altri»)
}

// Mostrati sono le schede visibili: tutte se la card e' aperta, altrimenti le prime prodottiInCard.
func (c richiestaCard) Mostrati() []prodottoCard {
	if c.Tutti || len(c.Prodotti) <= prodottiInCard {
		return c.Prodotti
	}
	return c.Prodotti[:prodottiInCard]
}

// Nascosti sono i prodotti che la tile «+ N altri» apre.
func (c richiestaCard) Nascosti() int { return len(c.Prodotti) - len(c.Mostrati()) }

// PuoRichiudere: la card e' aperta su tutti i prodotti e ce ne sono piu' di quelli di una card chiusa.
func (c richiestaCard) PuoRichiudere() bool { return c.Tutti && len(c.Prodotti) > prodottiInCard }

func (c richiestaCard) Chiusa() bool { return c.StatoThread == db.StatoThreadCHIUSA }

// Le frasi dei segnali, al singolare o al plurale.
func (c richiestaCard) FraseProdotti() string { return conta(len(c.Prodotti), "prodotto", "prodotti") }
func (c richiestaCard) FraseBloccanti() string {
	return conta(int(c.NBloccanti.Int64), "bloccante", "bloccanti")
}
func (c richiestaCard) FraseDaSmistare() string {
	return conta(int(c.NDaSmistare), "file da smistare", "file da smistare")
}

// FaseBreve e' il nome della fase sul badge: «ATTESA DISEGNI», «FATTIBILITÀ».
func (c richiestaCard) FaseBreve() string {
	if !c.NomeFase.Valid {
		return ""
	}
	return nomeFase(c.NomeFase.Fase)
}

func nomeFase(f db.Fase) string {
	s := strings.ReplaceAll(string(f), "_", " ")
	return strings.Replace(s, "FATTIBILITA", "FATTIBILITÀ", 1)
}

// SLA e' la classe del semaforo (rosso, giallo, verde, bianco), "" se la fase non ha un semaforo.
func (c richiestaCard) SLA() string { return c.Semaforo.String }

// prodottoCard e' un codice della richiesta con quello che si sa per mostrarlo.
type prodottoCard struct {
	Codice       string
	Rev          string
	Descrizione  string
	Link         string // il Fascicolo della RFQ, sul componente se c'e'
	Anteprima    string // l'indirizzo del PDF nell'iframe; "" = nessun disegno
	NomePDF      string
	DaConfermare bool // l'anteprima e' una proposta, non un documento confermato
	allegato     uuid.UUID
}

// costruisciPanoramica mette insieme le tre letture. Non tocca il database: la scelta dell'anteprima e
// la firma si provano in L1.
func costruisciPanoramica(f filtriRichieste, righe []db.ListRichiestePanoramicaRow, prodotti []db.ListProdottiPanoramicaRow, anteprime []db.ListAnteprimePanoramicaRow) *panoramicaRichieste {
	p := &panoramicaRichieste{Filtri: f}
	if len(righe) > 0 {
		p.Totale = righe[0].Totale
	}
	perThread := raggruppaProdotti(prodotti, anteprime)
	for _, r := range righe {
		p.Card = append(p.Card, richiestaCard{ListRichiestePanoramicaRow: r, Prodotti: perThread[r.ThreadID]})
	}
	p.Firma = firmaPanoramica(p)
	return p
}

// raggruppaProdotti da' a ogni RFQ le sue schede, nell'ordine degli identificativi, ciascuna con la sua
// anteprima.
func raggruppaProdotti(prodotti []db.ListProdottiPanoramicaRow, anteprime []db.ListAnteprimePanoramicaRow) map[uuid.UUID][]prodottoCard {
	candidati := map[uuid.UUID][]db.ListAnteprimePanoramicaRow{}
	for _, a := range anteprime {
		candidati[a.ThreadID] = append(candidati[a.ThreadID], a)
	}
	out := map[uuid.UUID][]prodottoCard{}
	visti := map[string]bool{}
	for _, pr := range prodotti {
		chiave := pr.ThreadID.String() + "|" + strings.ToUpper(pr.Codice)
		if visti[chiave] {
			continue
		}
		visti[chiave] = true
		pc := prodottoCard{Codice: pr.Codice, Rev: pr.Rev.String, Descrizione: pr.Descrizione.String,
			Link: "/thread/" + pr.ThreadID.String() + "/fascicolo"}
		if pr.ComponenteID.Valid {
			pc.Link += "?nodo=" + pr.ComponenteID.UUID.String()
		}
		if a, ok := sceglieAnteprima(pr, candidati[pr.ThreadID]); ok {
			pc.allegato = a.AllegatoID
			pc.Anteprima = "/allegato/" + a.AllegatoID.String() + "/anteprima#toolbar=0&navpanes=0&view=Fit"
			pc.NomePDF = a.NomeFile
			pc.DaConfermare = a.Fonte != "documento"
		}
		out[pr.ThreadID] = append(out[pr.ThreadID], pc)
	}
	return out
}

// sceglieAnteprima: fra i disegni della RFQ quelli del prodotto (stesso componente, o stesso codice
// senza distinguere maiuscole); prima un documento confermato, poi una proposta; fra piu' candidati il
// piu' recente, e a parita' l'allegato con l'identificativo minore, perche' due letture uguali diano la
// stessa scelta.
func sceglieAnteprima(pr db.ListProdottiPanoramicaRow, candidati []db.ListAnteprimePanoramicaRow) (db.ListAnteprimePanoramicaRow, bool) {
	var scelto db.ListAnteprimePanoramicaRow
	trovato := false
	meglio := func(a, b db.ListAnteprimePanoramicaRow) bool {
		if (a.Fonte == "documento") != (b.Fonte == "documento") {
			return a.Fonte == "documento"
		}
		if !a.Quando.Equal(b.Quando) {
			return a.Quando.After(b.Quando)
		}
		return a.AllegatoID.String() < b.AllegatoID.String()
	}
	for _, a := range candidati {
		suo := (pr.ComponenteID.Valid && a.ComponenteID.Valid && a.ComponenteID.UUID == pr.ComponenteID.UUID) ||
			(a.Codice.Valid && strings.EqualFold(a.Codice.String, pr.Codice))
		if suo && (!trovato || meglio(a, scelto)) {
			scelto, trovato = a, true
		}
	}
	return scelto, trovato
}

// firmaPanoramica riassume quello che l'elenco mostra: se il poll trova la stessa firma, rifare le card
// non cambierebbe niente (e ricaricherebbe le anteprime). Solo slice, in ordine: niente mappe.
func firmaPanoramica(p *panoramicaRichieste) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%d|", p.Filtri.URL(), p.Totale)
	for _, c := range p.Card {
		scad, ultimo := "", ""
		if c.DataScadenza != nil {
			scad = c.DataScadenza.Format(time.DateOnly)
		}
		if c.UltimoAggiornamento != nil {
			ultimo = c.UltimoAggiornamento.UTC().Format(time.RFC3339Nano)
		}
		fmt.Fprintf(h, "c:%s:%s:%s:%s:%s:%s:%v:%d:%s:%s:%d:%d:%d|", c.ThreadID, c.Cliente, c.Oggetto.String, c.RiferimentoCliente.String,
			c.Buyer.String, c.StatoThread, c.NomeFase, c.GgInFase.Int32, c.Semaforo.String, scad+"/"+ultimo, c.PesoCliente,
			c.NBloccanti.Int64, c.NDaSmistare)
		for _, pr := range c.Prodotti {
			fmt.Fprintf(h, "p:%s:%s:%s:%s:%t|", pr.Codice, pr.Rev, pr.Descrizione, pr.allegato, pr.DaConfermare)
		}
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// caricaPanoramica fa le tre letture. conBarra aggiunge clienti e fasi per le tendine: servono solo
// alla pagina intera, non all'elenco che il browser chiede quando cambia un filtro.
func (s *Server) caricaPanoramica(ctx context.Context, f filtriRichieste, conBarra bool) (*panoramicaRichieste, error) {
	q := db.New(s.Pool)
	righe, err := q.ListRichiestePanoramica(ctx, f.parametri())
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(righe))
	for _, r := range righe {
		ids = append(ids, r.ThreadID)
	}
	var prodotti []db.ListProdottiPanoramicaRow
	var anteprime []db.ListAnteprimePanoramicaRow
	if len(ids) > 0 {
		if prodotti, err = q.ListProdottiPanoramica(ctx, ids); err != nil {
			return nil, err
		}
		if anteprime, err = q.ListAnteprimePanoramica(ctx, ids); err != nil {
			return nil, err
		}
	}
	p := costruisciPanoramica(f, righe, prodotti, anteprime)
	p.Intervallo = int(s.pollRichieste() / time.Second)
	if conBarra {
		clienti, err := q.ListClientiTutti(ctx)
		if err != nil {
			return nil, err
		}
		fasi, err := q.ListFaseCatalogo(ctx)
		if err != nil {
			return nil, err
		}
		p.opzioniBarra(clienti, fasi)
	}
	return p, nil
}

// opzioniBarra prepara le scelte della barra con quella corrente segnata: il template le elenca e basta.
func (p *panoramicaRichieste) opzioniBarra(clienti []db.ListClientiTuttiRow, fasi []db.FaseCatalogo) {
	f := p.Filtri
	for _, c := range clienti {
		if c.NRichieste > 0 || c.ClienteID == f.Cliente {
			p.Clienti = append(p.Clienti, opzione{Valore: c.ClienteID.String(), Etichetta: c.CartellaNas, Titolo: c.RagioneSociale, Scelta: c.ClienteID == f.Cliente})
		}
	}
	sort.SliceStable(p.Clienti, func(i, j int) bool {
		return strings.ToLower(p.Clienti[i].Etichetta) < strings.ToLower(p.Clienti[j].Etichetta)
	})
	for _, fa := range fasi {
		p.Fasi = append(p.Fasi, opzione{Valore: string(fa.NomeFase), Etichetta: nomeFase(fa.NomeFase), Titolo: fa.Descrizione, Scelta: fa.NomeFase == f.Fase})
	}
	for _, o := range ordiniRichieste {
		p.Ordini = append(p.Ordini, opzione{Valore: o.Chiave, Etichetta: o.Etichetta, Scelta: o.Chiave == f.Ordine})
	}
	for _, st := range []opzione{{Valore: statoTutte, Etichetta: "Tutte"}, {Valore: statoAperte, Etichetta: "Aperte"}, {Valore: statoChiuse, Etichetta: "Chiuse"}} {
		st.Scelta = st.Valore == f.Stato
		p.Stati = append(p.Stati, st)
	}
}

func (s *Server) pollRichieste() time.Duration {
	if s.PollRichieste > 0 {
		return s.PollRichieste
	}
	return pollRichiestePred
}

// richieste: GET /richieste. La pagina intera, o (HTMX) il solo elenco quando cambia un filtro, con
// l'indirizzo pulito da mettere nella barra del browser. Il poll porta la firma: se l'elenco e' lo
// stesso, 204 e nessuno scambio.
func (s *Server) richieste(w http.ResponseWriter, r *http.Request) {
	f := leggiFiltriRichieste(r.URL.Query())
	frammento := frammentoRichiesto(r)
	p, err := s.caricaPanoramica(r.Context(), f, !frammento)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if frammento {
		if firma := r.URL.Query().Get("firma"); firma != "" {
			if firma == p.Firma {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		} else {
			w.Header().Set("HX-Push-Url", f.URL())
		}
	}
	s.rendi(w, r, "richieste.html", "richieste_elenco", "Richieste", p)
}

// richiestaProdotti: GET /richieste/{id}/prodotti. Le schede di una RFQ, tutte («+ N altri») o di nuovo
// le prime sei (?meno=1). Due letture, per la sola RFQ aperta.
func (s *Server) richiestaProdotti(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	q := db.New(s.Pool)
	prodotti, err := q.ListProdottiPanoramica(r.Context(), []uuid.UUID{id})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if len(prodotti) == 0 {
		http.Error(w, "RFQ non trovata o senza prodotti", 404)
		return
	}
	anteprime, err := q.ListAnteprimePanoramica(r.Context(), []uuid.UUID{id})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	c := richiestaCard{Prodotti: raggruppaProdotti(prodotti, anteprime)[id], Tutti: r.URL.Query().Get("meno") != "1"}
	c.ThreadID = id
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pagine["richieste.html"].ExecuteTemplate(w, "richiesta_prodotti", c); err != nil {
		s.Log.Error("template", "frammento", "richiesta_prodotti", "err", err)
	}
}
