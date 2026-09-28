package web

// Il tipo del componente, scelto da una persona (Smistamento, fase T; decisioni del 27/09 ter, «Tipo componente
// / commerciale»), e la riattivazione esplicita di un'autorizzazione sospesa (precisazione dell'utente del 27/09
// sera). Dalla scheda del componente e dall'editor della Struttura BOM.
//
// Tutti e due i gesti hanno l'anteprima in GET, che non scrive (F1) e porta la firma di quello che si vede, e la
// scrittura in POST, che la ricontrolla sotto il lucchetto della RFQ. La POST risponde come gli altri gesti a
// htmx (l'avviso e i pannelli fuori banda); senza htmx (il modulo mandato da un browser senza JavaScript)
// risponde con la pagina intera del Fascicolo sulla scheda del componente, con l'esito nell'avviso.

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// tipoVista e' il riquadro del tipo: la tendina (E.Tipo vuoto) o l'anteprima di un cambio.
type tipoVista struct {
	Base   string // /thread/{id}/fascicolo
	Cid    uuid.UUID
	E      fascicolo.EffettoTipo
	Errore string
	Scrive bool
}

// riattivazioneVista e' il riquadro dell'anteprima della riattivazione.
type riattivazioneVista struct {
	Base   string
	Cid    uuid.UUID
	E      fascicolo.EffettoRiattivazione
	Errore string
	Scrive bool
}

// anteprimaTipo: GET .../componente/{cid}/tipo?tipo=… L'anteprima del cambio di tipo (fase T): che cosa si
// sospende, che cosa resta, perche' non si puo'; senza tipo, la tendina con i motivi. Non scrive niente.
func (s *Server) anteprimaTipo(w http.ResponseWriter, r *http.Request) {
	thread, cid, ok := s.threadEComponente(w, r)
	if !ok {
		return
	}
	v := tipoVista{Base: "/thread/" + thread.String() + "/fascicolo", Cid: cid, Scrive: almeno(utenteDa(r.Context()), db.RuoloUtenteOperatore)}
	var err error
	v.E, err = fascicolo.EffettoCambioTipo(r.Context(), db.New(s.Pool), thread, cid, db.TipoComponente(strings.TrimSpace(r.FormValue("tipo"))))
	if err != nil {
		var rf rifiuto
		if !errors.As(err, &rf) {
			http.Error(w, "anteprima non riuscita", 500)
			return
		}
		v.Errore = spiegaErrore(err)
	}
	s.frammentoFascicolo(w, "fasc_tipo", v)
}

// cambiaTipo: POST .../componente/{cid}/tipo, campi `tipo` e `firma` (quella dell'anteprima vista). Il gesto
// della fase T: il tipo lo decide una persona, con le regole di fascicolo.CambiaTipoComponente.
func (s *Server) cambiaTipo(w http.ResponseWriter, r *http.Request) {
	s.gestoConEsito(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		cid, err := s.componenteDaPercorso(r)
		if err != nil {
			return "", err
		}
		return fascicolo.CambiaTipoComponenteVisto(ctx, q, thread, cid, db.TipoComponente(strings.TrimSpace(r.FormValue("tipo"))), utente,
			strings.TrimSpace(r.FormValue("firma")))
	})
}

// anteprimaRiattivazione: GET .../componente/{cid}/step-strutturale/riattiva?sha=… L'anteprima della
// riattivazione esplicita di un'autorizzazione sospesa: che cosa torna nell'autorita', quali deleghe tornano con
// lei, oppure perche' non si puo' (un commerciale, un'autorizzazione che riattivata non varrebbe). Non scrive.
func (s *Server) anteprimaRiattivazione(w http.ResponseWriter, r *http.Request) {
	thread, cid, ok := s.threadEComponente(w, r)
	if !ok {
		return
	}
	v := riattivazioneVista{Base: "/thread/" + thread.String() + "/fascicolo", Cid: cid, Scrive: almeno(utenteDa(r.Context()), db.RuoloUtenteOperatore)}
	var err error
	v.E, err = fascicolo.EffettoRiattivazioneDi(r.Context(), db.New(s.Pool), thread, cid, strings.TrimSpace(r.FormValue("sha")))
	if err != nil {
		var rf rifiuto
		if !errors.As(err, &rf) {
			http.Error(w, "anteprima non riuscita", 500)
			return
		}
		v.Errore = spiegaErrore(err)
	}
	s.frammentoFascicolo(w, "fasc_riattiva", v)
}

// riattivaStepStrutturale: POST .../componente/{cid}/step-strutturale/riattiva, campi `sha` e `firma`. La scelta
// esplicita di una persona: l'autorizzazione sospesa torna valida (fascicolo.RiattivaStrutturale).
func (s *Server) riattivaStepStrutturale(w http.ResponseWriter, r *http.Request) {
	s.gestoConEsito(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		cid, err := s.componenteDaPercorso(r)
		if err != nil {
			return "", err
		}
		return fascicolo.RiattivaStrutturale(ctx, q, thread, utente, cid, strings.TrimSpace(r.FormValue("sha")), strings.TrimSpace(r.FormValue("firma")))
	})
}

// threadEComponente legge la RFQ e il componente dal percorso di una GET, e il form; se non vanno, ha gia'
// risposto.
func (s *Server) threadEComponente(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return uuid.Nil, uuid.Nil, false
	}
	cid, err := uuid.Parse(r.PathValue("cid"))
	if err != nil {
		http.Error(w, "componente non valido", 400)
		return uuid.Nil, uuid.Nil, false
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "richiesta non valida", 400)
		return uuid.Nil, uuid.Nil, false
	}
	return thread, cid, true
}

// frammentoFascicolo disegna un frammento del Fascicolo con i suoi dati.
func (s *Server) frammentoFascicolo(w http.ResponseWriter, nome string, dati any) {
	var buf bytes.Buffer
	if err := s.pagine["fascicolo.html"].ExecuteTemplate(&buf, nome, vista{Dati: dati, Frammento: true}); err != nil {
		s.Log.Error("template", "frammento", nome, "err", err)
		http.Error(w, "errore nel disegnare il riquadro: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// gestoConEsito e' gesto con il ramo senza htmx: il modulo mandato da un browser senza JavaScript riceve la
// pagina intera del Fascicolo, aperta sulla scheda del componente, con l'esito nell'avviso. Fatto: 200; non
// fatto: «Niente è cambiato: …» con 422 per un rifiuto (la richiesta non si puo' fare cosi') o 500. Prima un
// gesto senza htmx riceveva il frammento nudo della pagina della RFQ, senza il layout e senza dove dire l'esito.
func (s *Server) gestoConEsito(w http.ResponseWriter, r *http.Request, fai func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error)) {
	if r.Header.Get("HX-Request") == "true" {
		s.gesto(w, r, fai)
		return
	}
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form non valido", 400)
		return
	}
	ctx := r.Context()
	u := utenteDa(ctx)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback(ctx)
	msg, err := fai(ctx, db.New(tx), thread, u.UtenteID)
	if err == nil {
		err = tx.Commit(ctx)
	}
	stato := http.StatusOK
	if err != nil {
		_ = tx.Rollback(ctx)
		msg = "Niente è cambiato: " + spiegaErrore(err)
		stato = http.StatusInternalServerError
		var rf rifiuto
		if errors.As(err, &rf) {
			stato = http.StatusUnprocessableEntity
		}
	}
	st := url.Values{"vista": {"bom"}}
	if cid, err := uuid.Parse(r.PathValue("cid")); err == nil {
		st.Set("nodo", cid.String())
	}
	d, err := s.caricaFascicolo(ctx, thread, leggiStatoFascicolo(st), u)
	if err != nil {
		s.Log.Warn("pagina del gesto senza htmx non letta", "rfq", thread, "err", err)
		http.Error(w, msg, stato)
		return
	}
	d.Avviso = msg
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(stato)
	s.rendi(w, r, "fascicolo.html", "fasc_corpo", "Fascicolo", d)
}
