package web

// sorgente.go: le rotte della Sorgente del ciclo dei file (V1, EM §10.9; corsia file, ondata B): S1 il modello dei
// file, S2 «Scarta», S3 «Metti in Altri file», S4 «Riattiva», S5 i byte di un file. La logica sta nel servizio
// core/rfq/filerfq; qui la catena, l'ordine dei controlli, il corpo, la traduzione degli esiti nella busta del C3 e i
// byte di S5. Il ramo JSON di POST /thread/{id}/carica (E1′) sta in sorgente_carica.go.
//
// Regole comuni (EM §10.9):
//   - catena conMotoreA → conNoStore → soloAdmin, con la riga gemella senza metodo (404 a flag spento, 405 a flag acceso);
//     CSRF globale (ProtezioneCSRF). Solo admin dietro [motore_a] (base di lavoro V1-Q07 A);
//   - l'ordine dei controlli e' fisso: flag, sessione e ruolo, id della RFQ (400), esistenza della RFQ (404), poi il corpo;
//   - un allegato di un'altra RFQ e' 422 FUORI_RFQ, mai 404;
//   - nessun percorso nei corpi (un campo sconosciuto e' 422) ne' nelle risposte (stati e identificativi);
//   - la busta: {esito, codice, testo, …}; «conflitto» per un 409, «rifiutato» per gli altri; 5xx di un gesto =
//     {"esito": "da_verificare", "chiave_idempotenza": …};
//   - nessuna GET scrive (S1, S5 entrano in TestNessunaGetScrive); dopo il COMMIT dei gesti, la RFQ si segna per il
//     rismistamento F8 (dopoIlGesto).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
)

// registraSorgente monta S1-S5 con le loro righe gemelle (V1, EM §10.9). E1′ si monta in fascicolo_rotte.go, sulla
// rotta di «carica» che c'era gia' (caricaRFQ sceglie il ramo).
func (s *Server) registraSorgente(mux *http.ServeMux) {
	catena := func(h http.HandlerFunc) http.HandlerFunc { return s.conMotoreA(conNoStore(s.soloAdmin(h))) }
	gemella := func(allow string) http.HandlerFunc { return s.conMotoreA(conNoStore(metodoNonAmmessoSorgente(allow))) }
	mux.HandleFunc("GET /thread/{id}/sorgente", catena(s.sorgenteModello))
	mux.HandleFunc("/thread/{id}/sorgente", gemella("GET, HEAD"))
	mux.HandleFunc("POST /thread/{id}/sorgente/file/{allegato}/scarta", catena(s.dopoIlGesto(s.sorgenteScarta)))
	mux.HandleFunc("/thread/{id}/sorgente/file/{allegato}/scarta", gemella("POST"))
	mux.HandleFunc("POST /thread/{id}/sorgente/file/{allegato}/altri-file", catena(s.dopoIlGesto(s.sorgenteAltriFile)))
	mux.HandleFunc("/thread/{id}/sorgente/file/{allegato}/altri-file", gemella("POST"))
	mux.HandleFunc("POST /thread/{id}/sorgente/file/{allegato}/riattiva", catena(s.dopoIlGesto(s.sorgenteRiattiva)))
	mux.HandleFunc("/thread/{id}/sorgente/file/{allegato}/riattiva", gemella("POST"))
	mux.HandleFunc("GET /thread/{id}/sorgente/file/{allegato}/contenuto", catena(s.sorgenteContenuto))
	mux.HandleFunc("/thread/{id}/sorgente/file/{allegato}/contenuto", gemella("GET, HEAD"))
}

// metodoNonAmmessoSorgente: un metodo diverso da quelli della rotta, a flag acceso: 405 con Allow, senza la sessione.
func metodoNonAmmessoSorgente(allow string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Allow", allow)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

// cicloFile: il servizio del ciclo dei file di questo server. Nil (un server montato senza runtime, nelle prove vecchie)
// = un servizio con il pool e lo staging, senza deposito.
func (s *Server) cicloFile() *filerfq.Servizio {
	if s.CicloFile != nil {
		return s.CicloFile
	}
	return &filerfq.Servizio{Pool: s.Pool, Staging: s.Staging, Log: s.Log, Pipeline: s.Pipeline}
}

// ---------------------------------------------------------------- la busta

// scriviJSONSorgente: il JSON prima in un buffer, poi nosniff, no-store, lo stato e i byte.
func (s *Server) scriviJSONSorgente(w http.ResponseWriter, r *http.Request, stato int, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		s.Log.Error("sorgente: la risposta non si codifica in JSON", "percorso", r.URL.Path, "err", err)
		buf.Reset()
		buf.WriteString(`{"esito":"errore","codice":"ERRORE","testo":"La risposta non si è potuta scrivere: il dettaglio è nel log del server."}` + "\n")
		stato = http.StatusInternalServerError
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	scriviJSONNoStore(w, stato, json.RawMessage(buf.Bytes()))
}

// rifiutoSorgente scrive un esito certo che non ha cambiato niente: {esito, codice, testo, …extra}, con l'intestazione
// X-Motivo (il codice) per chi non legge il corpo (PE-29: il visore pdf.js di S5).
func (s *Server) rifiutoSorgente(w http.ResponseWriter, r *http.Request, rf *filerfq.Rifiuto) {
	esito := "rifiutato"
	if rf.Stato == http.StatusConflict {
		esito = "conflitto"
	}
	o := map[string]any{"esito": esito, "codice": rf.Codice, "testo": rf.Testo}
	for k, v := range rf.Extra {
		if _, c := o[k]; !c {
			o[k] = v
		}
	}
	w.Header().Set("X-Motivo", rf.Codice)
	s.scriviJSONSorgente(w, r, rf.Stato, o)
}

// erroreSorgente traduce un errore del servizio: un Rifiuto con il suo stato; la RFQ che non c'e' 404; altrimenti 500.
// chiave: per i gesti con chiave il 500 e' «da verificare», con la chiave (EM §10.9, §10.10).
func (s *Server) erroreSorgente(w http.ResponseWriter, r *http.Request, err error, chiave *uuid.UUID) {
	if rf, ok := filerfq.ComeRifiuto(err); ok {
		if rf.Stato == http.StatusNotFound {
			s.scriviJSONSorgente(w, r, http.StatusNotFound, map[string]any{"esito": "rifiutato", "codice": "RFQ_INESISTENTE", "testo": "RFQ non trovata"})
			return
		}
		s.rifiutoSorgente(w, r, rf)
		return
	}
	if errors.Is(err, filerfq.ErrRFQInesistente) {
		s.scriviJSONSorgente(w, r, http.StatusNotFound, map[string]any{"esito": "rifiutato", "codice": "RFQ_INESISTENTE", "testo": "RFQ non trovata"})
		return
	}
	if errors.Is(err, context.Canceled) {
		s.Log.Info("sorgente: la richiesta è stata chiusa da chi l'aveva fatta", "percorso", r.URL.Path, "err", err)
	} else {
		s.Log.Error("sorgente: richiesta non riuscita", "percorso", r.URL.Path, "err", err)
	}
	if chiave != nil {
		w.Header().Set("X-Motivo", "DA_VERIFICARE")
		s.scriviJSONSorgente(w, r, http.StatusInternalServerError, map[string]any{"esito": "da_verificare", "chiave_idempotenza": *chiave,
			"testo": "Non so se il gesto è registrato: verifico con la stessa chiave prima di qualunque altro tentativo."})
		return
	}
	w.Header().Set("X-Motivo", "ERRORE")
	s.scriviJSONSorgente(w, r, http.StatusInternalServerError, map[string]any{"esito": "errore", "codice": "ERRORE",
		"testo": "La richiesta non è riuscita: il dettaglio è nel log del server."})
}

// threadDellaRotta: l'id della RFQ (400 se non e' un UUID) e la sua esistenza (404), prima del corpo.
func (s *Server) threadDellaRotta(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		w.Header().Set("X-Motivo", "RICHIESTA_NON_VALIDA")
		s.scriviJSONSorgente(w, r, http.StatusBadRequest, map[string]any{"esito": "rifiutato", "codice": "RICHIESTA_NON_VALIDA", "testo": "id della RFQ non valido"})
		return uuid.UUID{}, false
	}
	if _, err := db.New(s.Pool).GetThread(r.Context(), thread); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			w.Header().Set("X-Motivo", "RFQ_INESISTENTE")
			s.scriviJSONSorgente(w, r, http.StatusNotFound, map[string]any{"esito": "rifiutato", "codice": "RFQ_INESISTENTE", "testo": "RFQ non trovata"})
			return uuid.UUID{}, false
		}
		s.erroreSorgente(w, r, err, nil)
		return uuid.UUID{}, false
	}
	return thread, true
}

// allegatoDellaRotta: l'id dell'allegato del percorso (400 se non e' un UUID).
func (s *Server) allegatoDellaRotta(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	a, err := uuid.Parse(r.PathValue("allegato"))
	if err != nil {
		w.Header().Set("X-Motivo", "RICHIESTA_NON_VALIDA")
		s.scriviJSONSorgente(w, r, http.StatusBadRequest, map[string]any{"esito": "rifiutato", "codice": "RICHIESTA_NON_VALIDA", "testo": "id del file non valido"})
		return uuid.UUID{}, false
	}
	return a, true
}

// ---------------------------------------------------------------- S1

// sorgenteModello: GET /thread/{id}/sorgente[?file=…][&scartati=1] (EM §10.9, S1).
func (s *Server) sorgenteModello(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.threadDellaRotta(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	op := filerfq.OpzioniModello{Scartati: q.Get("scartati") == "1"}
	if f := q.Get("file"); f != "" {
		id, err := uuid.Parse(f)
		if err != nil {
			w.Header().Set("X-Motivo", "RICHIESTA_NON_VALIDA")
			s.scriviJSONSorgente(w, r, http.StatusBadRequest, map[string]any{"esito": "rifiutato", "codice": "RICHIESTA_NON_VALIDA", "testo": "id del file non valido"})
			return
		}
		op.File = &id
	}
	if r.Method == http.MethodHead {
		headMotoreA(w)
		return
	}
	m, err := s.cicloFile().Modello(r.Context(), thread, op)
	if err != nil {
		s.erroreSorgente(w, r, err, nil)
		return
	}
	s.scriviJSONSorgente(w, r, http.StatusOK, m)
}

// ---------------------------------------------------------------- S2, S3, S4

// gestoSorgente: le regole comuni dei tre gesti (l'ordine dei controlli, il corpo), poi il gesto del servizio.
func (s *Server) gestoSorgente(w http.ResponseWriter, r *http.Request, nome string,
	fai func(ctx context.Context, thread, allegato, utente uuid.UUID, c filerfq.CorpoGesto) (*filerfq.EsitoGesto, error)) {
	thread, ok := s.threadDellaRotta(w, r)
	if !ok {
		return
	}
	allegato, ok := s.allegatoDellaRotta(w, r)
	if !ok {
		return
	}
	c, rf := filerfq.LeggiCorpo(http.MaxBytesReader(w, r.Body, filerfq.MaxCorpo+1), nome)
	if rf != nil {
		s.rifiutoSorgente(w, r, rf)
		return
	}
	u := utenteDa(r.Context())
	esito, err := fai(r.Context(), thread, allegato, u.UtenteID, c)
	if err != nil {
		s.erroreSorgente(w, r, err, c.ChiaveIdempotenza)
		return
	}
	s.scriviJSONSorgente(w, r, http.StatusOK, esito)
}

// sorgenteScarta: POST /thread/{id}/sorgente/file/{allegato}/scarta (EM §10.9, S2).
func (s *Server) sorgenteScarta(w http.ResponseWriter, r *http.Request) {
	s.gestoSorgente(w, r, filerfq.GestoScarta, s.cicloFile().Scarta)
}

// sorgenteAltriFile: POST /thread/{id}/sorgente/file/{allegato}/altri-file (EM §10.9, S3).
func (s *Server) sorgenteAltriFile(w http.ResponseWriter, r *http.Request) {
	s.gestoSorgente(w, r, filerfq.GestoAltriFile, s.cicloFile().MettiInAltriFile)
}

// sorgenteRiattiva: POST /thread/{id}/sorgente/file/{allegato}/riattiva (EM §10.9, S4).
func (s *Server) sorgenteRiattiva(w http.ResponseWriter, r *http.Request) {
	s.gestoSorgente(w, r, filerfq.GestoRiattiva, s.cicloFile().Riattiva)
}

// ---------------------------------------------------------------- S5

// sorgenteContenuto: GET /thread/{id}/sorgente/file/{allegato}/contenuto?modo=anteprima|scarica (EM §10.9, S5): i
// byte di un file per tutti i formati. Un file attivo dallo staging, poi dal documento scritto sul NAS, poi da una copia
// di conservazione dello stesso allegato non ancora rilasciata (anche sotto un'altra RFQ); un file in Altri file dal
// deposito, poi dallo staging, poi dal NAS; uno scartato 410. Un PDF (i byte veri: %PDF-) in anteprima e' inline; tutto
// il resto e' un allegato da scaricare, application/octet-stream. Sempre nosniff e la CSP sandbox. Range e HEAD li fa
// http.ServeContent. Gli errori portano X-Motivo (PE-29). Nessun effetto: una riga di log.
func (s *Server) sorgenteContenuto(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.threadDellaRotta(w, r)
	if !ok {
		return
	}
	allegato, ok := s.allegatoDellaRotta(w, r)
	if !ok {
		return
	}
	// i byte si aprono come documento (il visore, una scheda nuova), mai dentro la pagina (W/anteprima.go)
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("X-Motivo", "RICHIESTA_NON_VALIDA")
		s.scriviJSONSorgente(w, r, http.StatusBadRequest, map[string]any{"esito": "rifiutato", "codice": "RICHIESTA_NON_VALIDA",
			"testo": "Il contenuto si apre come documento, non dentro la pagina."})
		return
	}
	modo := r.URL.Query().Get("modo")
	if modo == "" {
		modo = "anteprima"
	}
	if modo != "anteprima" && modo != "scarica" {
		w.Header().Set("X-Motivo", "RICHIESTA_NON_VALIDA")
		s.scriviJSONSorgente(w, r, http.StatusBadRequest, map[string]any{"esito": "rifiutato", "codice": "RICHIESTA_NON_VALIDA",
			"testo": "modo è anteprima oppure scarica"})
		return
	}
	svc := s.cicloFile()
	ctx := r.Context()
	o, err := svc.OccorrenzaPerContenuto(ctx, thread, allegato)
	if err != nil {
		s.erroreSorgente(w, r, err, nil)
		return
	}
	if o == nil {
		s.rifiutoSorgente(w, r, &filerfq.Rifiuto{Stato: http.StatusUnprocessableEntity, Codice: "FUORI_RFQ", Testo: "Il file non è di questa RFQ."})
		return
	}
	if o.Natura != db.NaturaAllegatoFile && o.Natura != db.NaturaAllegatoElementoOutlook {
		s.rifiutoSorgente(w, r, &filerfq.Rifiuto{Stato: http.StatusUnprocessableEntity, Codice: "NON_GESTITO",
			Testo: "Il ciclo dei file gestisce i file e gli elementi di Outlook."})
		return
	}
	if o.StatoCiclo() == filerfq.StatoScartato {
		s.rifiutoSorgente(w, r, &filerfq.Rifiuto{Stato: http.StatusGone, Codice: "FILE_SCARTATO",
			Testo: "Il file è stato scartato in questa RFQ: per riaverlo, ricaricalo."})
		return
	}
	src, err := s.sorgenteDelContenuto(ctx, svc, o, thread)
	if err != nil {
		var eh erroreHTTP
		if errors.As(err, &eh) && eh.stato < 500 {
			s.rifiutoSorgente(w, r, &filerfq.Rifiuto{Stato: eh.stato, Codice: "BYTE_NON_DISPONIBILI", Testo: eh.messaggio})
			return
		}
		// un percorso che esce dalla radice, la copia di conservazione che manca: una frase generica, il dettaglio nel log
		s.Log.Error("sorgente: contenuto non servito", "allegato", allegato, "err", err)
		w.Header().Set("X-Motivo", "ERRORE")
		s.scriviJSONSorgente(w, r, http.StatusInternalServerError, map[string]any{"esito": "errore", "codice": "ERRORE",
			"testo": "Il contenuto non si è potuto aprire: il dettaglio è nel log del server."})
		return
	}
	if src == nil {
		indicazione, testo := "ricarica", "I byte del file non sono su questo server: ricaricalo."
		if o.Origine == db.OrigineAllegatoOutlook {
			indicazione, testo = "scarica", "I byte del file non sono su questo server: scaricalo dalla mail con «Scarica»."
		}
		s.rifiutoSorgente(w, r, &filerfq.Rifiuto{Stato: http.StatusGone, Codice: "BYTE_NON_DISPONIBILI", Testo: testo,
			Extra: map[string]any{"indicazione": indicazione}})
		return
	}
	defer src.Close()
	var testa [5]byte
	n, errTesta := io.ReadFull(src, testa[:])
	if errTesta != nil && !errors.Is(errTesta, io.EOF) && !errors.Is(errTesta, io.ErrUnexpectedEOF) {
		s.erroreSorgente(w, r, errTesta, nil)
		return
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		s.erroreSorgente(w, r, err, nil)
		return
	}
	pdf := string(testa[:n]) == "%PDF-"
	h := w.Header()
	disposizione := nomePerIlBrowser(o.NomeFile)
	if pdf {
		h.Set("Content-Type", "application/pdf")
		if modo == "scarica" {
			disposizione = "attachment" + strings.TrimPrefix(disposizione, "inline")
		}
	} else {
		h.Set("Content-Type", "application/octet-stream")
		disposizione = "attachment" + strings.TrimPrefix(disposizione, "inline")
	}
	h.Set("Content-Disposition", disposizione)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	h.Set("Cache-Control", "no-store")
	rw := &rispostaConStato{ResponseWriter: w}
	http.ServeContent(rw, r, "", src.modtime, src)
	stato := rw.stato
	if stato == 0 {
		stato = http.StatusOK
	}
	s.Log.Info("contenuto servito", "allegato", allegato, "stato", stato, "da", src.da, "byte_letti", src.letti,
		"dimensione", src.dim, "modo", modo, "range", r.Header.Get("Range"))
}

// sorgenteDelContenuto sceglie da dove arrivano i byte di S5, secondo lo stato del file (EM §10.9). nil, nil = nessuna
// sorgente (410 BYTE_NON_DISPONIBILI per un file attivo). Un file in Altri file senza la sua copia e senza altre
// sorgenti e' un'anomalia (500).
func (s *Server) sorgenteDelContenuto(ctx context.Context, svc *filerfq.Servizio, o *filerfq.Occorrenza, thread uuid.UUID) (*sorgente, error) {
	q := db.New(s.Pool)
	// dalloStaging: i byte dello staging. Un path_staging fuori dalla radice e' un errore (500 con la frase generica,
	// EM §10.9); sotto la radice si serve solo il posto del contenuto con lo sha dell'occorrenza (EContenuto, come il
	// passo 4 del tempo 2): un percorso alterato, o dello staging vecchio per messaggio, non serve byte che non si sa di
	// chi siano (PM23 PP-05; [T] della corsia file, piu' stretto di «sotto la radice»).
	dalloStaging := func() (*sorgente, error) {
		if !o.PathStaging.Valid || o.PathStaging.String == "" {
			return nil, nil
		}
		p, err := s.nelloStaging(o.PathStaging.String)
		if err != nil {
			return nil, err
		}
		if p == "" {
			return nil, nil
		}
		if !staging.EContenuto(s.Staging, p, o.Sha()) {
			s.Log.Warn("sorgente: path_staging non e' il posto del contenuto: non si serve dallo staging", "allegato", o.AllegatoID)
			return nil, nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil, nil
		}
		st, err := f.Stat()
		if err != nil || st.IsDir() {
			f.Close()
			return nil, nil
		}
		return &sorgente{f: f, da: "staging", dim: st.Size(), modtime: st.ModTime(), etag: o.Sha(), percorso: p}, nil
	}
	dalNAS := func() *sorgente {
		a, err := q.GetAllegato(ctx, o.AllegatoID)
		if err != nil {
			return nil
		}
		src, err := s.dalNas(ctx, q, a)
		if err != nil {
			s.Log.Warn("sorgente: il NAS non serve il file", "allegato", o.AllegatoID, "err", err)
			return nil
		}
		return src
	}
	dalDeposito := func(soloRFQ bool) (*sorgente, error) {
		f, st, err := svc.ApriCopia(ctx, thread, o.AllegatoID, soloRFQ)
		if err != nil || f == nil {
			return nil, err
		}
		return &sorgente{f: f, da: "conservazione", dim: st.Size(), modtime: st.ModTime(), etag: o.Sha()}, nil
	}
	if o.Stato == filerfq.StatoAltriFile {
		src, err := dalDeposito(true)
		if err != nil {
			return nil, err
		}
		if src != nil {
			return src, nil
		}
		s.Log.Error("sorgente: il file è in Altri file ma la sua copia di conservazione non si apre", "allegato", o.AllegatoID)
		if src, err := dalloStaging(); err != nil || src != nil {
			return src, err
		}
		if src := dalNAS(); src != nil {
			return src, nil
		}
		return nil, fmt.Errorf("allegato %s in Altri file: %w", o.AllegatoID, filerfq.ErrCopiaMancante)
	}
	if src, err := dalloStaging(); err != nil || src != nil {
		return src, err
	}
	if src := dalNAS(); src != nil {
		return src, nil
	}
	return dalDeposito(false)
}
