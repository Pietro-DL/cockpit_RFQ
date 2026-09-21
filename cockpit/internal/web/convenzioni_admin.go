package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"promatec/cockpit/internal/domain"
	"promatec/cockpit/internal/platform/db"
)

// Admin → Anagrafica → cliente → «Lavorazioni e fornitori» (blocco 7A, D39)
//
// Due cose, entrambe del cliente: le CONVENZIONI DI CODICE (come scrive la lavorazione superficiale
// nel codice del pezzo: un suffisso, o una regex) e i FORNITORI QUALIFICATI (chi è abilitato a fare
// quale lavorazione per lui). Con le due insieme, da un codice si arriva ai fornitori a cui chiedere
// l'offerta: codice → convenzione → lavorazione → `cliente_fornitore_lavorazione`.
//
// Le convenzioni passano dalla stessa doppia porta delle regole di riconoscimento (D17): in
// scrittura `domain.ValidaConvenzione` rifiuta ciò che non torna (esempio che non corrisponde,
// controesempio che corrisponde, regex che non compila, nessuna lavorazione); in lettura
// `domain.LeggiConvenzioni` segna ✗ una riga rotta scritta a mano in database e non la usa. Il
// ✓ della schermata e il salvataggio riuscito vogliono dire la stessa cosa.

// convenzioniDominio converte le righe del database nel tipo puro del dominio, nello stesso ordine.
func convenzioniDominio(righe []db.ListConvenzioniClienteRow) []domain.Convenzione {
	out := make([]domain.Convenzione, 0, len(righe))
	for _, r := range righe {
		out = append(out, domain.Convenzione{
			ID: r.ConvenzioneID, Modo: string(r.Modo), Espressione: r.Espressione, Esempio: r.Esempio,
			Controesempio: r.Controesempio.String, Descrizione: r.Descrizione, Attiva: r.Attiva, Lavorazioni: r.Lavorazioni,
		})
	}
	return out
}

// provaCodice è l'esito del banco «Prova un codice»: le lavorazioni trovate con l'evidenza, e per
// ciascuna i fornitori qualificati per questo cliente.
type provaCodice struct {
	Codice      string
	Trovate     []domain.LavorazioneTrovata
	Qualificati map[string][]db.Fornitore
}

// caricaLavorazioniCliente riempie la sezione: convenzioni con la diagnosi, qualifiche, e le
// tendine (lavorazioni, fornitori attivi).
func (s *Server) caricaLavorazioniCliente(ctx context.Context, q *db.Queries, d *anagraficaDati, clienteID uuid.UUID) {
	d.Convenzioni, _ = q.ListConvenzioniCliente(ctx, clienteID)
	_, d.DiagnosiConvenzioni = domain.LeggiConvenzioni(convenzioniDominio(d.Convenzioni))
	d.Qualifiche, _ = q.ListQualificheCliente(ctx, clienteID)
	d.Lavorazioni, _ = q.ListLavorazioni(ctx)
	d.Fornitori, _ = q.ListFornitoriAttivi(ctx)
}

// vincoloEsterno riconosce il 23503 di PostgreSQL (chiave esterna violata).
func vincoloEsterno(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23503"
}

func (s *Server) nuovaConvenzione(w http.ResponseWriter, r *http.Request) {
	id, c, ok := s.clienteDaRotta(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ctx := r.Context()
	cv := domain.Convenzione{
		Modo: strings.TrimSpace(r.FormValue("modo")), Espressione: strings.TrimSpace(r.FormValue("espressione")),
		Esempio: strings.TrimSpace(r.FormValue("esempio")), Controesempio: strings.TrimSpace(r.FormValue("controesempio")),
		Descrizione: strings.TrimSpace(r.FormValue("descrizione")), Attiva: true,
	}
	for _, l := range r.Form["lavorazione"] {
		if l = strings.TrimSpace(l); l != "" {
			cv.Lavorazioni = append(cv.Lavorazioni, l)
		}
	}
	if cv.Descrizione == "" {
		cv.Descrizione = cv.Espressione
	}
	// la porta in scrittura: prima del database, con la stessa funzione che decide il ✓
	if err := domain.ValidaConvenzione(cv); err != nil {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Errore: "convenzione rifiutata: " + err.Error()})
		return
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	riga, err := q.InsertConvenzione(ctx, db.InsertConvenzioneParams{
		ClienteID: id, Modo: db.ModoConvenzione(cv.Modo), Espressione: cv.Espressione, Esempio: cv.Esempio,
		Controesempio: ptxt(cv.Controesempio), Descrizione: cv.Descrizione,
	})
	if err != nil {
		if _, dup := vincoloViolato(err); dup {
			err = fmt.Errorf("esiste già una convenzione con l'espressione %q per questo cliente", cv.Espressione)
		}
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Errore: err.Error()})
		return
	}
	for _, l := range cv.Lavorazioni {
		if err := q.InsertConvenzioneLavorazione(ctx, db.InsertConvenzioneLavorazioneParams{ConvenzioneID: riga.ConvenzioneID, Lavorazione: l}); err != nil {
			if vincoloEsterno(err) {
				err = fmt.Errorf("la lavorazione %q non esiste nella tabella delle lavorazioni", l)
			}
			s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Errore: err.Error()})
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.Log.Info("convenzione di codice aggiunta", "cliente", c.RagioneSociale, "modo", cv.Modo, "espressione", cv.Espressione,
		"lavorazioni", strings.Join(cv.Lavorazioni, ","), "utente", siglaDa(r))
	s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni",
		Fatto: "Convenzione aggiunta: l'esempio corrisponde e il controesempio no."})
}

func (s *Server) eliminaConvenzione(w http.ResponseWriter, r *http.Request) {
	id, c, ok := s.clienteDaRotta(w, r)
	if !ok {
		return
	}
	cid, err := uuid.Parse(r.FormValue("convenzione_id"))
	if err != nil {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Errore: "convenzione non valida"})
		return
	}
	n, err := db.New(s.Pool).EliminaConvenzione(r.Context(), db.EliminaConvenzioneParams{ConvenzioneID: cid, ClienteID: id})
	switch {
	case err != nil:
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Errore: err.Error()})
	case n == 0:
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Errore: "quella convenzione non è di questo cliente"})
	default:
		s.Log.Info("convenzione di codice tolta", "cliente", c.RagioneSociale, "convenzione", cid, "utente", siglaDa(r))
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Fatto: "Convenzione tolta, con le sue lavorazioni."})
	}
}

func (s *Server) attivaConvenzione(w http.ResponseWriter, r *http.Request) {
	id, c, ok := s.clienteDaRotta(w, r)
	if !ok {
		return
	}
	cid, err := uuid.Parse(r.FormValue("convenzione_id"))
	if err != nil {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Errore: "convenzione non valida"})
		return
	}
	attiva := r.FormValue("attiva") == "1"
	n, err := db.New(s.Pool).SetConvenzioneAttiva(r.Context(), db.SetConvenzioneAttivaParams{ConvenzioneID: cid, ClienteID: id, Attiva: attiva})
	switch {
	case err != nil:
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Errore: err.Error()})
	case n == 0:
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Errore: "quella convenzione non è di questo cliente"})
	case attiva:
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Fatto: "Convenzione riattivata."})
	default:
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Fatto: "Convenzione spenta: resta scritta, non si usa."})
	}
}

// qualificaFornitore scrive in cliente_fornitore_lavorazione. La chiave esterna composta verso
// fornitore_lavorazione fa sì che non si possa qualificare un fornitore su una lavorazione che
// non ha dichiarato di fare: la schermata lo traduce in una frase.
func (s *Server) qualificaFornitore(w http.ResponseWriter, r *http.Request) {
	id, c, ok := s.clienteDaRotta(w, r)
	if !ok {
		return
	}
	fid, err := uuid.Parse(r.FormValue("fornitore_id"))
	lav := strings.TrimSpace(r.FormValue("lavorazione"))
	if err != nil || lav == "" {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Errore: "servono un fornitore e una lavorazione"})
		return
	}
	q := db.New(s.Pool)
	n, err := q.InsertQualifica(r.Context(), db.InsertQualificaParams{ClienteID: id, FornitoreID: fid, Lavorazione: lav})
	if err != nil {
		if vincoloEsterno(err) {
			nome := "il fornitore"
			if f, e := q.GetFornitore(r.Context(), fid); e == nil {
				nome = f.RagioneSociale
			}
			err = fmt.Errorf("%s non ha «%s» fra le sue lavorazioni: aggiungila prima nella sua scheda, poi qualificalo qui", nome, lav)
		}
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Errore: err.Error()})
		return
	}
	if n == 0 {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Fatto: "Era già qualificato: niente da fare."})
		return
	}
	s.Log.Info("fornitore qualificato", "cliente", c.RagioneSociale, "fornitore", fid, "lavorazione", lav, "utente", siglaDa(r))
	s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Fatto: "Fornitore qualificato per questo cliente."})
}

func (s *Server) eliminaQualificaCliente(w http.ResponseWriter, r *http.Request) {
	id, c, ok := s.clienteDaRotta(w, r)
	if !ok {
		return
	}
	fid, err := uuid.Parse(r.FormValue("fornitore_id"))
	lav := strings.TrimSpace(r.FormValue("lavorazione"))
	if err != nil || lav == "" {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Errore: "qualifica non valida"})
		return
	}
	if _, err := db.New(s.Pool).EliminaQualifica(r.Context(), db.EliminaQualificaParams{ClienteID: id, FornitoreID: fid, Lavorazione: lav}); err != nil {
		s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Errore: err.Error()})
		return
	}
	s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", Fatto: "Qualifica tolta."})
}

// provaCodiceCliente: da un codice alle lavorazioni e da queste ai fornitori qualificati, con lo
// STESSO codice che userà l'ingest. Non c'è un secondo motore.
func (s *Server) provaCodiceCliente(w http.ResponseWriter, r *http.Request) {
	id, c, ok := s.clienteDaRotta(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	q := db.New(s.Pool)
	codice := strings.TrimSpace(r.FormValue("codice"))
	righe, _ := q.ListConvenzioniCliente(ctx, id)
	conv, _ := domain.LeggiConvenzioni(convenzioniDominio(righe))
	p := &provaCodice{Codice: codice, Qualificati: map[string][]db.Fornitore{}}
	if codice != "" {
		p.Trovate = conv.Lavorazioni(codice)
		for _, t := range p.Trovate {
			p.Qualificati[t.Lavorazione], _ = q.ListFornitoriQualificati(ctx, db.ListFornitoriQualificatiParams{ClienteID: id, Lavorazione: t.Lavorazione})
		}
	}
	s.rendiAnagrafica(w, r, anagraficaDati{Scelto: &c, Sez: "lavorazioni", ProvaCodice: p})
}
