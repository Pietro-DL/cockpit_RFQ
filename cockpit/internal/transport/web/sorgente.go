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
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
	"promatec/cockpit/internal/platform/storage/staging"
)

// registraSorgente monta S1-S5 con le loro righe gemelle (V1, EM §10.9). E1′ si monta in fascicolo_rotte.go, sulla
// rotta di «carica» che c'era gia' (caricaRFQ sceglie il ramo).
func (s *Server) registraSorgente(mux *http.ServeMux) {
	catena := func(h http.HandlerFunc) http.HandlerFunc { return s.conMotoreA(conXMotivo(conNoStore(s.soloAdmin(h)))) }
	gemella := func(allow string) http.HandlerFunc {
		return s.conMotoreA(conXMotivo(conNoStore(metodoNonAmmessoSorgente(allow))))
	}
	mux.HandleFunc("GET /thread/{id}/sorgente", catena(s.sorgenteModello))
	mux.HandleFunc("/thread/{id}/sorgente", gemella("GET, HEAD"))
	mux.HandleFunc("POST /thread/{id}/sorgente/file/{allegato}/scarta", catena(s.dopoIlGesto(s.sorgenteScarta)))
	mux.HandleFunc("/thread/{id}/sorgente/file/{allegato}/scarta", gemella("POST"))
	mux.HandleFunc("POST /thread/{id}/sorgente/file/{allegato}/altri-file", catena(s.dopoIlGesto(s.sorgenteAltriFile)))
	mux.HandleFunc("/thread/{id}/sorgente/file/{allegato}/altri-file", gemella("POST"))
	mux.HandleFunc("POST /thread/{id}/sorgente/file/{allegato}/riattiva", catena(s.dopoIlGesto(s.sorgenteRiattiva)))
	mux.HandleFunc("/thread/{id}/sorgente/file/{allegato}/riattiva", gemella("POST"))
	mux.HandleFunc("POST /thread/{id}/sorgente/file/{allegato}/riprova-rimozione", catena(s.sorgenteRiprovaRimozione))
	mux.HandleFunc("/thread/{id}/sorgente/file/{allegato}/riprova-rimozione", gemella("POST"))
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

// sorgenteRiprovaRimozione: POST /thread/{id}/sorgente/file/{allegato}/riprova-rimozione (V1, direttiva 35 C-6a; U2
// §3.5): «Riprova la rimozione dalla cartella ALTRO» su un file scartato la cui copia in ALTRO e' ancora da togliere.
// Lo stesso corpo dei gesti (chiave d'intento e versione attesa), lo stesso protocollo della rimozione.
func (s *Server) sorgenteRiprovaRimozione(w http.ResponseWriter, r *http.Request) {
	s.gestoSorgente(w, r, filerfq.GestoRiprovaRimozione, s.cicloFile().RiprovaRimozione)
}

// ---------------------------------------------------------------- S5

// sorgenteContenuto: GET /thread/{id}/sorgente/file/{allegato}/contenuto?modo=anteprima|scarica (EM §10.9, S5): i
// byte di un file per tutti i formati. Un file attivo dallo staging, poi dal documento scritto sul NAS, poi da una copia
// di conservazione dello stesso allegato non ancora rilasciata (anche sotto un'altra RFQ); un file in Altri file dalla
// sua copia (sha verificato dallo stesso handle), poi dallo staging, poi dal NAS, altrimenti 410 BYTE_NON_DISPONIBILI con
// X-Motivo COPIA_NAS_MANCANTE o COPIA_NAS_CAMBIATA (U2 §3.12: mai un 500, mai byte diversi); uno scartato 410. Un PDF (i byte veri: %PDF-) in anteprima e' inline; tutto
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
	var cm copiaNonServita
	if errors.As(err, &cm) {
		testo := "La copia in ALTRO non c'è più al percorso registrato (la cartella può essere stata spostata o rinominata), e il server non ha altri byte del file: ricaricalo."
		if cm.motivo == filerfq.MotivoCopiaCambiata {
			testo = "La copia in ALTRO è stata cambiata sul NAS: il cockpit non la serve, e non ha altri byte del file. Ricaricalo."
		}
		o := map[string]any{"esito": "rifiutato", "codice": "BYTE_NON_DISPONIBILI", "testo": testo, "indicazione": "ricarica"}
		w.Header().Set("X-Motivo", cm.motivo)
		s.scriviJSONSorgente(w, r, http.StatusGone, o)
		return
	}
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

// copiaNonServita: un file in Altri file la cui copia non si serve (mancante o cambiata) e senza altre sorgenti: 410
// con X-Motivo (U2 §3.12).
type copiaNonServita struct{ motivo string }

func (c copiaNonServita) Error() string { return "copia degli Altri file non servita: " + c.motivo }

// sorgenteDelContenuto sceglie da dove arrivano i byte di S5, secondo lo stato del file (EM §10.9). nil, nil = nessuna
// sorgente (410 BYTE_NON_DISPONIBILI per un file attivo). Un file in Altri file senza la sua copia e senza altre
// sorgenti: copiaNonServita con il motivo (410, X-Motivo COPIA_NAS_MANCANTE o COPIA_NAS_CAMBIATA).
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
	dalNAS := func() *sorgente { return s.nasSoloLettura(ctx, q, thread, o) }
	motivoCopia := ""
	dallaCopia := func(soloRFQ bool) (*sorgente, error) {
		f, st, motivo, err := svc.ApriCopia(ctx, thread, o.AllegatoID, soloRFQ)
		motivoCopia = motivo
		if err != nil || f == nil {
			return nil, err
		}
		return &sorgente{f: f, da: "conservazione", dim: st.Size(), modtime: st.ModTime(), etag: o.Sha()}, nil
	}
	if o.Stato == filerfq.StatoAltriFile {
		src, err := dallaCopia(true)
		if err != nil {
			return nil, err
		}
		if src != nil {
			return src, nil
		}
		if motivoCopia == "" {
			motivoCopia = filerfq.MotivoCopiaMancante
		}
		s.Log.Warn("sorgente: il file è in Altri file ma la sua copia non si serve", "allegato", o.AllegatoID, "motivo", motivoCopia)
		if src, err := dalloStaging(); err != nil || src != nil {
			return src, err
		}
		if src := dalNAS(); src != nil {
			return src, nil
		}
		return nil, copiaNonServita{motivo: motivoCopia}
	}
	if src, err := dalloStaging(); err != nil || src != nil {
		return src, err
	}
	if src := dalNAS(); src != nil {
		return src, nil
	}
	return dallaCopia(false)
}

// nasSoloLettura: i byte del documento della RFQ con lo stesso contenuto scritto sul NAS, in sola lettura. S5 e' una
// GET senza effetti (EM §10.9; §8.5: «nessuna scrittura nelle letture», [U] D-18): a differenza dell'anteprima legacy
// (dalNas) qui non si segnala niente e non si registra niente. Il documento e' quello scritto senza anomalie aperte
// (GetDocumentoScrittoPerSha); il percorso passa dal contenimento del NAS (PercorsoSulNas). Un file che manca, una
// cartella al suo posto, un file che non si apre non sono una sorgente (li segnala il ricognitore, o l'anteprima
// legacy): nil, e S5 prova la sorgente dopo o risponde 410. Un file «in dubbio» (dimensione diversa, o toccato dopo
// l'ultima volta che il server l'ha guardato) si rilegge per lo sha da questo handle, senza scrivere la verifica: se lo
// sha torna si serve, altrimenti no (rilievo bloccante della revisione).
func (s *Server) nasSoloLettura(ctx context.Context, q *db.Queries, thread uuid.UUID, o *filerfq.Occorrenza) *sorgente {
	sha := o.Sha()
	if s.NAS == nil || sha == "" || !s.NAS.Raggiungibile() {
		return nil
	}
	d, err := q.GetDocumentoScrittoPerSha(ctx, db.GetDocumentoScrittoPerShaParams{ThreadID: thread, Sha256: sha})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			s.Log.Warn("sorgente: il documento scritto sul NAS non si legge", "allegato", o.AllegatoID, "err", err)
		}
		return nil
	}
	if !d.CartellaRelativa.Valid {
		return nil
	}
	p, err := documenti.PercorsoSulNas(s.NAS.Radice, d.CartellaRelativa.String, d.PathRelativo)
	if err != nil {
		s.Log.Error("sorgente: percorso del documento fuori dal NAS", "allegato", o.AllegatoID, "documento", d.DocumentoID, "err", err)
		return nil
	}
	f, err := os.Open(p.Assoluto)
	if err != nil {
		s.Log.Warn("sorgente: il file del documento sul NAS non si apre: non si serve dal NAS", "allegato", o.AllegatoID,
			"documento", d.DocumentoID, "err", err)
		return nil
	}
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		f.Close()
		s.Log.Warn("sorgente: al posto del file del documento sul NAS non c'e' un file", "allegato", o.AllegatoID, "documento", d.DocumentoID)
		return nil
	}
	src := &sorgente{f: f, da: "NAS", dim: st.Size(), modtime: st.ModTime(), etag: sha, percorso: p.Assoluto}
	if dubbio(db.Documento{Bytes: d.Bytes, ScrittoIl: d.ScrittoIl, VerificatoIl: d.VerificatoIl}, st) {
		src.inDubbio = true
		h, _, err := nas.Sha256Da(f)
		if _, errS := f.Seek(0, io.SeekStart); errS != nil && err == nil {
			err = errS
		}
		if err != nil || !strings.EqualFold(h, sha) {
			f.Close()
			s.Log.Warn("sorgente: il file del documento sul NAS non torna con lo sha256: non si serve dal NAS", "allegato", o.AllegatoID,
				"documento", d.DocumentoID, "err", err)
			return nil
		}
	}
	return src
}

// conXMotivo: ogni risposta d'errore delle rotte della Sorgente porta X-Motivo (EM §10.9 S5; DECA PE-29), anche quelle
// che non scrive il gestore: il 403 di soloAdmin (una pagina), il 401 della sessione scaduta da htmx, il 405 della riga
// gemella, il 416 e il 412 di http.ServeContent (rilievo della revisione). Una risposta che ha gia' il suo X-Motivo (la
// busta della Sorgente) lo tiene; le altre ricevono un codice per lo stato (motivoDelloStato). Sta dentro conMotoreA: a
// flag spento resta il 404 del router, senza intestazioni in piu'.
func conXMotivo(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h(&rispostaConMotivo{ResponseWriter: w}, r)
	}
}

// rispostaConMotivo mette X-Motivo, se manca, al momento dell'intestazione di una risposta con stato ≥ 400.
type rispostaConMotivo struct {
	http.ResponseWriter
	scritta bool
}

func (w *rispostaConMotivo) WriteHeader(stato int) {
	if !w.scritta {
		w.scritta = true
		if stato >= 400 && w.Header().Get("X-Motivo") == "" {
			w.Header().Set("X-Motivo", motivoDelloStato(stato))
		}
	}
	w.ResponseWriter.WriteHeader(stato)
}

func (w *rispostaConMotivo) Write(b []byte) (int, error) {
	if !w.scritta {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lascia a http.ResponseController il writer vero.
func (w *rispostaConMotivo) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// motivoDelloStato: il codice di X-Motivo per una risposta d'errore senza busta (PF-25).
func motivoDelloStato(stato int) string {
	switch stato {
	case http.StatusBadRequest:
		return "RICHIESTA_NON_VALIDA"
	case http.StatusUnauthorized:
		return "SESSIONE_SCADUTA"
	case http.StatusForbidden:
		return "NON_AUTORIZZATO"
	case http.StatusNotFound:
		return "NON_TROVATO"
	case http.StatusMethodNotAllowed:
		return "METODO_NON_AMMESSO"
	case http.StatusPreconditionFailed:
		return "PRECONDIZIONE_FALLITA"
	case http.StatusRequestedRangeNotSatisfiable:
		return "RANGE_NON_VALIDO"
	}
	return "ERRORE"
}
