package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/registro/fornitori"
	"promatec/cockpit/internal/ingest"
	"promatec/cockpit/internal/platform/db"
)

// Admin → Anagrafica → Fornitori (blocco 7A.5)
//
// La scheda di un fornitore ha quattro sezioni: chi è (generale), da quali indirizzi scrive
// (domini e contatti), che cosa fa e per chi è qualificato (lavorazioni e qualifiche), che cosa
// ha scritto (posta). Niente JSON: un fornitore non ha regole di codice, e la sua anagrafica è
// fatta di caselle da spuntare e tabelle, non di regex.
//
// Le scritture sono non distruttive come per i clienti (6.6): un dominio già di un altro fornitore
// non si sposta, dice di chi è; togliere una lavorazione che ha una qualifica sopra fallisce, e la
// schermata dice quale cliente la usa.

type fornitoriDati struct {
	Sez         string // generale | contatti | lavorazioni | posta
	Fornitori   []db.ListFornitoriRow
	Scelto      *db.Fornitore
	Domini      []db.DominioFornitore
	Contatti    []db.ContattoFornitore
	Lavorazioni []db.Lavorazione // tutte quelle del catalogo
	Sue         map[string]bool  // le capacità di questo fornitore, per spuntare le caselle
	Qualifiche  []db.ListQualificheFornitoreRow
	Clienti     []db.ListClientiTuttiRow
	Messaggi    []db.Messaggio
	Tipi        []db.TipoFornitore
	Errore      string
	Fatto       string
}

var sezioniFornitore = []struct{ Chiave, Nome string }{
	{"generale", "Generale"},
	{"contatti", "Domini e contatti"},
	{"lavorazioni", "Lavorazioni e qualifiche"},
	{"posta", "Posta"},
}

func (d fornitoriDati) Sezioni() []struct{ Chiave, Nome string } { return sezioniFornitore }

func sezioneFornitoreValida(s string) string {
	for _, v := range sezioniFornitore {
		if v.Chiave == s {
			return s
		}
	}
	return "generale"
}

func (s *Server) adminFornitori(w http.ResponseWriter, r *http.Request) {
	s.rendiFornitori(w, r, fornitoriDati{})
}

func (s *Server) rendiFornitori(w http.ResponseWriter, r *http.Request, dati fornitoriDati) {
	ctx := r.Context()
	q := db.New(s.Pool)
	if dati.Sez == "" {
		dati.Sez = sezioneFornitoreValida(r.FormValue("sez"))
	}
	var err error
	if dati.Fornitori, err = q.ListFornitori(ctx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	dati.Tipi = db.AllTipoFornitoreValues()
	if dati.Scelto == nil {
		if id, e := uuid.Parse(r.FormValue("fornitore")); e == nil {
			if f, e := q.GetFornitore(ctx, id); e == nil {
				dati.Scelto = &f
			}
		}
	}
	if f := dati.Scelto; f != nil {
		dati.Domini, _ = q.ListDominiFornitore(ctx, f.FornitoreID)
		dati.Contatti, _ = q.ListContattiFornitore(ctx, f.FornitoreID)
		dati.Lavorazioni, _ = q.ListLavorazioni(ctx)
		dati.Sue = map[string]bool{}
		if sue, err := q.ListLavorazioniFornitore(ctx, f.FornitoreID); err == nil {
			for _, l := range sue {
				dati.Sue[l.Codice] = true
			}
		}
		dati.Qualifiche, _ = q.ListQualificheFornitore(ctx, f.FornitoreID)
		dati.Clienti, _ = q.ListClientiTutti(ctx)
		dati.Messaggi, _ = q.ListMessaggiPerControparteFornitore(ctx, uuid.NullUUID{UUID: f.FornitoreID, Valid: true})
	}
	s.rendi(w, r, "fornitori.html", "fornitori_corpo", "Anagrafica", dati)
}

func (s *Server) fornitoreDaRotta(w http.ResponseWriter, r *http.Request) (db.Fornitore, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return db.Fornitore{}, false
	}
	f, err := db.New(s.Pool).GetFornitore(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "fornitore non trovato", 404)
		} else {
			http.Error(w, err.Error(), 500)
		}
		return db.Fornitore{}, false
	}
	return f, true
}

func tipoFornitore(v string) (db.TipoFornitore, error) {
	t := db.TipoFornitore(v)
	if !t.Valid() {
		return "", fmt.Errorf("tipo di fornitore %q non valido: materie_prime, processi o verniciatore", v)
	}
	return t, nil
}

// ---------------------------------------------------------------- generale

func (s *Server) nuovoFornitore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := db.New(s.Pool)
	nome := strings.TrimSpace(r.FormValue("ragione_sociale"))
	tipo, err := tipoFornitore(r.FormValue("tipo"))
	if nome == "" || err != nil {
		if err == nil {
			err = errors.New("serve la ragione sociale")
		}
		s.rendiFornitori(w, r, fornitoriDati{Errore: err.Error()})
		return
	}
	if altro, e := q.GetFornitorePerRagioneSociale(ctx, nome); e == nil {
		s.rendiFornitori(w, r, fornitoriDati{Scelto: &altro, Errore: fmt.Sprintf("un fornitore «%s» esiste già: è questo. Non ne ho creato un secondo.", altro.RagioneSociale)})
		return
	}
	f, err := q.InsertFornitore(ctx, db.InsertFornitoreParams{
		RagioneSociale: nome, Tipo: tipo, Lingua: txtN(strings.ToLower(strings.TrimSpace(r.FormValue("lingua"))), 2),
		Note: txtN(r.FormValue("note"), 4000)})
	if err != nil {
		s.rendiFornitori(w, r, fornitoriDati{Errore: err.Error()})
		return
	}
	if dom := strings.TrimSpace(r.FormValue("dominio")); dom != "" {
		if err := AggiungiDominioFornitore(ctx, q, dom, f.FornitoreID); err != nil {
			s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "contatti", Errore: "fornitore creato, ma il dominio no: " + err.Error()})
			return
		}
	}
	s.Log.Info("fornitore creato", "fornitore", f.RagioneSociale, "tipo", f.Tipo, "utente", siglaDa(r))
	s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Fatto: "Fornitore creato."})
}

func (s *Server) salvaFornitore(w http.ResponseWriter, r *http.Request) {
	f, ok := s.fornitoreDaRotta(w, r)
	if !ok {
		return
	}
	tipo, err := tipoFornitore(r.FormValue("tipo"))
	if err != nil {
		s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Errore: err.Error()})
		return
	}
	agg, err := db.New(s.Pool).UpdateFornitore(r.Context(), db.UpdateFornitoreParams{
		FornitoreID: f.FornitoreID, RagioneSociale: primo(strings.TrimSpace(r.FormValue("ragione_sociale")), f.RagioneSociale),
		Tipo: tipo, Lingua: txtN(strings.ToLower(strings.TrimSpace(r.FormValue("lingua"))), 2),
		Note: txtN(r.FormValue("note"), 4000), Attivo: r.FormValue("attivo") != ""})
	if err != nil {
		if _, dup := vincoloViolato(err); dup {
			err = errors.New("esiste già un fornitore con questa ragione sociale")
		}
		s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Errore: err.Error()})
		return
	}
	s.Log.Info("fornitore aggiornato", "fornitore", agg.RagioneSociale, "attivo", agg.Attivo, "utente", siglaDa(r))
	s.rendiFornitori(w, r, fornitoriDati{Scelto: &agg, Fatto: "Salvato."})
}

// ---------------------------------------------------------------- domini e contatti

func (s *Server) aggiungiDominioFornitore(w http.ResponseWriter, r *http.Request) {
	f, ok := s.fornitoreDaRotta(w, r)
	if !ok {
		return
	}
	if err := AggiungiDominioFornitore(r.Context(), db.New(s.Pool), r.FormValue("dominio"), f.FornitoreID); err != nil {
		s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "contatti", Errore: err.Error()})
		return
	}
	s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "contatti",
		Fatto: "Dominio aggiunto." + s.ritriagePer(r.Context(), "", strings.ToLower(strings.TrimSpace(r.FormValue("dominio"))))})
}

func (s *Server) eliminaDominioFornitore(w http.ResponseWriter, r *http.Request) {
	f, ok := s.fornitoreDaRotta(w, r)
	if !ok {
		return
	}
	dom := strings.ToLower(strings.TrimSpace(r.FormValue("dominio")))
	if _, err := db.New(s.Pool).EliminaDominioFornitore(r.Context(), db.EliminaDominioFornitoreParams{Lower: dom, FornitoreID: f.FornitoreID}); err != nil {
		s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "contatti", Errore: err.Error()})
		return
	}
	s.Log.Info("dominio fornitore rimosso", "dominio", dom, "fornitore", f.RagioneSociale, "utente", siglaDa(r))
	s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "contatti", Fatto: "Dominio rimosso da questo fornitore."})
}

func (s *Server) nuovoContattoFornitore(w http.ResponseWriter, r *http.Request) {
	f, ok := s.fornitoreDaRotta(w, r)
	if !ok {
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	if !strings.Contains(email, "@") {
		s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "contatti", Errore: "serve un indirizzo e-mail: è il modo in cui il contatto viene riconosciuto"})
		return
	}
	_, err := db.New(s.Pool).InsertContattoFornitore(r.Context(), db.InsertContattoFornitoreParams{
		FornitoreID: f.FornitoreID, Nome: txtN(strings.TrimSpace(r.FormValue("nome")), 120), Lower: email,
		Ruolo: txtN(strings.TrimSpace(r.FormValue("ruolo")), 60), Lingua: txtN(strings.ToLower(strings.TrimSpace(r.FormValue("lingua"))), 2),
		Note: txtN(r.FormValue("note"), 2000)})
	if err != nil {
		if _, dup := vincoloViolato(err); dup {
			err = fmt.Errorf("%s è già fra i contatti di questo fornitore", email)
		}
		s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "contatti", Errore: err.Error()})
		return
	}
	s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "contatti", Fatto: "Contatto aggiunto." + s.ritriagePer(r.Context(), email, "")})
}

func (s *Server) eliminaContattoFornitore(w http.ResponseWriter, r *http.Request) {
	f, ok := s.fornitoreDaRotta(w, r)
	if !ok {
		return
	}
	cid, err := uuid.Parse(r.FormValue("contatto_id"))
	if err != nil {
		s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "contatti", Errore: "contatto non valido"})
		return
	}
	if _, err := db.New(s.Pool).EliminaContattoFornitore(r.Context(), db.EliminaContattoFornitoreParams{ContattoID: cid, FornitoreID: f.FornitoreID}); err != nil {
		s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "contatti", Errore: err.Error()})
		return
	}
	s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "contatti", Fatto: "Contatto rimosso."})
}

// ---------------------------------------------------------------- lavorazioni e qualifiche

// salvaLavorazioniFornitore allinea le capacità alle caselle spuntate: aggiunge le nuove, toglie
// quelle tolte. Una capacità con una qualifica sopra non si toglie (chiave esterna composta): la
// schermata dice per quale cliente, e la qualifica va tolta prima, dalla scheda del cliente o da qui.
func (s *Server) salvaLavorazioniFornitore(w http.ResponseWriter, r *http.Request) {
	f, ok := s.fornitoreDaRotta(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ctx := r.Context()
	q := db.New(s.Pool)
	volute := map[string]bool{}
	for _, l := range r.Form["lavorazione"] {
		volute[strings.TrimSpace(l)] = true
	}
	attuali, err := q.ListLavorazioniFornitore(ctx, f.FornitoreID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var aggiunte, tolte int
	for _, l := range attuali {
		if volute[l.Codice] {
			delete(volute, l.Codice)
			continue
		}
		if _, err := q.EliminaLavorazioneFornitore(ctx, db.EliminaLavorazioneFornitoreParams{FornitoreID: f.FornitoreID, Lavorazione: l.Codice}); err != nil {
			if vincoloEsterno(err) {
				err = fmt.Errorf("«%s» non si toglie: %s è qualificato su questa lavorazione per almeno un cliente. Togli prima la qualifica.", l.Descrizione, f.RagioneSociale)
			}
			s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "lavorazioni", Errore: err.Error()})
			return
		}
		tolte++
	}
	for codice := range volute {
		n, err := q.InsertLavorazioneFornitore(ctx, db.InsertLavorazioneFornitoreParams{FornitoreID: f.FornitoreID, Lavorazione: codice})
		if err != nil {
			if vincoloEsterno(err) {
				err = fmt.Errorf("la lavorazione %q non esiste nel catalogo", codice)
			}
			s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "lavorazioni", Errore: err.Error()})
			return
		}
		aggiunte += int(n)
	}
	s.Log.Info("lavorazioni fornitore salvate", "fornitore", f.RagioneSociale, "aggiunte", aggiunte, "tolte", tolte, "utente", siglaDa(r))
	s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "lavorazioni", Fatto: fmt.Sprintf("Lavorazioni salvate: %d aggiunte, %d tolte.", aggiunte, tolte)})
}

func (s *Server) qualificaDalFornitore(w http.ResponseWriter, r *http.Request) {
	f, ok := s.fornitoreDaRotta(w, r)
	if !ok {
		return
	}
	cid, err := uuid.Parse(r.FormValue("cliente_id"))
	lav := strings.TrimSpace(r.FormValue("lavorazione"))
	if err != nil || lav == "" {
		s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "lavorazioni", Errore: "servono un cliente e una lavorazione"})
		return
	}
	n, err := db.New(s.Pool).InsertQualifica(r.Context(), db.InsertQualificaParams{ClienteID: cid, FornitoreID: f.FornitoreID, Lavorazione: lav})
	if err != nil {
		if vincoloEsterno(err) {
			err = fmt.Errorf("%s non ha «%s» fra le sue lavorazioni: spuntala qui sopra e salva, poi qualificalo", f.RagioneSociale, lav)
		}
		s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "lavorazioni", Errore: err.Error()})
		return
	}
	if n == 0 {
		s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "lavorazioni", Fatto: "Era già qualificato: niente da fare."})
		return
	}
	s.Log.Info("fornitore qualificato", "fornitore", f.RagioneSociale, "cliente", cid, "lavorazione", lav, "utente", siglaDa(r))
	s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "lavorazioni", Fatto: "Qualifica aggiunta."})
}

func (s *Server) eliminaQualificaDalFornitore(w http.ResponseWriter, r *http.Request) {
	f, ok := s.fornitoreDaRotta(w, r)
	if !ok {
		return
	}
	cid, err := uuid.Parse(r.FormValue("cliente_id"))
	lav := strings.TrimSpace(r.FormValue("lavorazione"))
	if err != nil || lav == "" {
		s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "lavorazioni", Errore: "qualifica non valida"})
		return
	}
	if _, err := db.New(s.Pool).EliminaQualifica(r.Context(), db.EliminaQualificaParams{ClienteID: cid, FornitoreID: f.FornitoreID, Lavorazione: lav}); err != nil {
		s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "lavorazioni", Errore: err.Error()})
		return
	}
	s.rendiFornitori(w, r, fornitoriDati{Scelto: &f, Sez: "lavorazioni", Fatto: "Qualifica tolta."})
}

// ---------------------------------------------------------------- import del seme (7A.4, CP7, CP14)

// importDati è la schermata dell'import: il testo del file, l'anteprima, e se è stato applicato.
type importDati struct {
	Testo     string
	Anteprima *fornitori.Anteprima
	Applicato bool
	Errore    string
	// Ritriage: che cosa è successo ai messaggi già arrivati. Un import che censisce un fornitore
	// e lascia la sua posta fra gli sconosciuti è un import che sembra non aver funzionato (7B.5).
	Ritriage string
}

func (s *Server) importaFornitoriForm(w http.ResponseWriter, r *http.Request) {
	s.rendi(w, r, "importa.html", "importa_corpo", "Anagrafica", importDati{})
}

// importaFornitori legge il seme (dal file caricato o dal testo incollato) e, con
// azione=anteprima, mostra che cosa farebbe; con azione=applica, lo fa. Non c'è una strada che
// scriva senza essere passata da qui con «applica» esplicito: l'anteprima è la regola, non
// un'opzione.
func (s *Server) importaFornitori(w http.ResponseWriter, r *http.Request) {
	testo, err := testoDelSeme(r)
	d := importDati{Testo: testo}
	if err != nil {
		d.Errore = err.Error()
		s.rendi(w, r, "importa.html", "importa_corpo", "Anagrafica", d)
		return
	}
	seme, err := fornitori.Leggi(strings.NewReader(testo))
	if err != nil {
		d.Errore = err.Error()
		s.rendi(w, r, "importa.html", "importa_corpo", "Anagrafica", d)
		return
	}
	ctx := r.Context()
	var ant fornitori.Anteprima
	switch r.FormValue("azione") {
	case "applica":
		ant, err = fornitori.Applica(ctx, s.Pool, seme)
		d.Applicato = err == nil
		if err == nil {
			s.Log.Info("seme fornitori applicato", "creati", len(ant.FornitoriDaCreare), "aggiunte", len(ant.DaAggiungere),
				"non_risolti", len(ant.NonRisolti), "utente", siglaDa(r))
			// Il ricalcolo dei messaggi già arrivati, DOPO il commit e per le sole chiavi scritte:
			// è lo stesso ritriage di «Censisci come fornitore», ripetuto (7B.5). Se fallisce,
			// l'import resta valido e lo si dice: i fornitori sono in anagrafica comunque.
			esito, e := (&ingest.Servizio{Pool: s.Pool, Log: s.Log}).RitriageMolti(ctx, ant.IndirizziScritti, ant.DominiScritti)
			if e != nil {
				d.Ritriage = "Il ricalcolo dei messaggi già arrivati non è riuscito: " + e.Error()
				s.Log.Error("ritriage dopo l'import del seme fornitori", "err", e)
			} else {
				d.Ritriage = "Messaggi già arrivati: " + esito.String() + "."
			}
		}
	default:
		ant, err = fornitori.Calcola(ctx, db.New(s.Pool), seme)
	}
	if err != nil {
		d.Errore = err.Error()
	} else {
		d.Anteprima = &ant
	}
	s.rendi(w, r, "importa.html", "importa_corpo", "Anagrafica", d)
}

// testoDelSeme prende il file caricato se c'è, altrimenti il testo incollato. Il testo incollato
// torna in pagina in ogni caso, così «anteprima» e poi «applica» lavorano sullo stesso contenuto.
func testoDelSeme(r *http.Request) (string, error) {
	if err := r.ParseMultipartForm(4 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		return "", err
	}
	if f, _, err := r.FormFile("file"); err == nil {
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, 4<<20))
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(string(b)) != "" {
			return string(b), nil
		}
	}
	testo := r.FormValue("testo")
	if strings.TrimSpace(testo) == "" {
		return "", errors.New("nessun file e nessun testo: incolla il JSON del seme o carica il file")
	}
	return testo, nil
}
