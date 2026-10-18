package web

// sorgente_carica.go: E1′, il ramo JSON di POST /thread/{id}/carica (V1, EM §10.9; base di lavoro V1-Q07 A per i
// permessi; [U] RV3-41 A per il bisogno). La rotta e' quella di sempre: caricaRFQ sceglie il ramo dall'intestazione
// Accept. Senza «application/json» resta il frammento HTML di oggi (caricaNellaRFQ, operatore in su), invariato nella
// risposta; con «application/json» la catena della Distinta nuova: flag (404 a flag spento, prima della sessione),
// no-store, admin (403), poi l'id (400), la RFQ (404), lo staging e l'analisi configurati (503), e solo dopo il corpo.
// Una richiesta rifiutata prima del corpo non scrive niente nello staging (giro 1, SIC-13). Il limite admin del ramo
// JSON e' una scelta di prodotto, non un confine di sicurezza: lo stesso operatore ottiene lo stesso effetto dal ramo
// HTML (EM §12; Appendice A K-13).

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/platform/storage/archivio"
	"promatec/cockpit/internal/platform/storage/staging"
)

// caricaRFQ: POST /thread/{id}/carica, il ramo scelto dall'intestazione Accept (EM §10.9, E1′).
func (s *Server) caricaRFQ() http.HandlerFunc {
	html := s.autenticato(s.dopoICaricamenti(s.caricaNellaRFQ))
	jsonRamo := s.conMotoreA(conNoStore(s.soloAdmin(s.dopoICaricamenti(s.caricaJSON))))
	return func(w http.ResponseWriter, r *http.Request) {
		if vuoleJSON(r) {
			jsonRamo(w, r)
			return
		}
		html(w, r)
	}
}

// vuoleJSON: la richiesta chiede JSON (il ponte della pagina nuova manda «Accept: application/json»).
func vuoleJSON(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Accept")), "application/json")
}

// caricaJSON e' E1′ (EM §10.9): multipart con `origine` (portale | interna; assente = portale) e i file (da 1 a 50),
// con i limiti per file e per richiesta di sempre. Per file una transazione sua (filerfq.Servizio.RegistraFile), con i
// doppioni sotto il lucchetto del contenitore e il contenuto adottato sotto il lucchetto per contenuto. Risposta 200
// {esito: caricati | parziale, origine, file[], testo}; 422 NESSUN_FILE_CARICATO se nessun file e' entrato o era gia'
// presente; 413; 503; 404; 5xx «da verificare» (la verifica e' un nuovo invio: cio' che e' entrato torna gia_presente).
func (s *Server) caricaJSON(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.threadDellaRotta(w, r)
	if !ok {
		return
	}
	switch {
	case s.Staging == "":
		s.rifiutoSorgente(w, r, &filerfq.Rifiuto{Stato: http.StatusServiceUnavailable, Codice: "NON_CONFIGURATO",
			Testo: "Lo staging non è configurato su questo server: niente è cambiato.", Extra: map[string]any{"motivo": "staging"}})
		return
	case s.Pipeline == nil:
		s.rifiutoSorgente(w, r, &filerfq.Rifiuto{Stato: http.StatusServiceUnavailable, Codice: "NON_CONFIGURATO",
			Testo: "La strada dell'analisi non è configurata su questo server: niente è cambiato.", Extra: map[string]any{"motivo": "analisi"}})
		return
	}
	max := s.maxCaricamentoEffettivo()
	totale := s.maxRichiestaCaricamento()
	r.Body = http.MaxBytesReader(w, r.Body, totale+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		var troppo *http.MaxBytesError
		if errors.As(err, &troppo) {
			s.rifiutoSorgente(w, r, &filerfq.Rifiuto{Stato: http.StatusRequestEntityTooLarge, Codice: "TROPPO_GRANDE",
				Testo: fmt.Sprintf("Niente è cambiato: la richiesta supera il limite totale di %d MB.", totale>>20)})
			return
		}
		s.rifiutoSorgente(w, r, &filerfq.Rifiuto{Stato: http.StatusUnprocessableEntity, Codice: "CORPO_NON_VALIDO",
			Testo: "Niente è cambiato: la richiesta non è un caricamento di file (multipart).", Extra: map[string]any{"campo": "file"}})
		return
	}
	origine := "portale"
	var arrivati []filerfq.FileArrivato
	var esiti []filerfq.EsitoFile
	togliTutti := func() {
		for _, f := range arrivati {
			staging.RimuoviParte(f.Parte)
		}
	}
	rifiutato := func(nome, codice, testo string) {
		esiti = append(esiti, filerfq.EsitoFile{NomeFile: nome, Esito: "rifiutato", Origine: origine,
			Motivo: &filerfq.MotivoRifiuto{Codice: codice, Testo: testo}})
	}
	parti := 0
	fermo := false
	for !fermo {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var troppo *http.MaxBytesError
			if errors.As(err, &troppo) {
				rifiutato("", "LIMITE_RICHIESTA", fmt.Sprintf("la richiesta supera il limite totale di %d MB: i file da qui in poi non sono arrivati", totale>>20))
			} else {
				rifiutato("", "ERRORE_STAGING", "la richiesta si è interrotta")
			}
			break
		}
		switch part.FormName() {
		case "origine":
			v, _ := io.ReadAll(io.LimitReader(part, 32))
			o := strings.TrimSpace(string(v))
			if o != "portale" && o != "interna" {
				_ = part.Close()
				togliTutti()
				s.rifiutoSorgente(w, r, &filerfq.Rifiuto{Stato: http.StatusUnprocessableEntity, Codice: "CORPO_NON_VALIDO",
					Testo: "Niente è cambiato: l'origine è «portale» (file del cliente) oppure «interna» (versione interna).",
					Extra: map[string]any{"campo": "origine"}})
				return
			}
			origine = o
		case "file":
			nome := strings.TrimSpace(filepath.Base(strings.ReplaceAll(part.FileName(), `\`, "/")))
			if nome == "" || nome == "." || nome == "/" {
				rifiutato("", "SENZA_NOME", "un file senza nome")
				break
			}
			parti++
			if parti > maxFileCaricamentoRFQ {
				rifiutato(nome, "TROPPO_FILE", fmt.Sprintf("al massimo %d file per volta: da qui in poi non sono stati letti", maxFileCaricamentoRFQ))
				fermo = true
				break
			}
			f, err := filerfq.ScriviParziale(s.Staging, part, nome, part.Header.Get("Content-Type"), max)
			var troppo *http.MaxBytesError
			switch {
			case errors.As(err, &troppo):
				rifiutato(nome, "LIMITE_RICHIESTA", fmt.Sprintf("la richiesta supera il limite totale di %d MB: il file non è arrivato", totale>>20))
				fermo = true
			case errors.Is(err, filerfq.ErrTroppoGrande):
				rifiutato(nome, "TROPPO_GRANDE", fmt.Sprintf("il file supera il limite di %d MB", max>>20))
			case errors.Is(err, filerfq.ErrFileVuoto):
				rifiutato(nome, "FILE_VUOTO", "il file è vuoto")
			case err != nil:
				s.Log.Error("caricamento JSON nella RFQ: staging", "rfq", thread, "file", nome, "err", err)
				rifiutato(nome, "ERRORE_STAGING", "il file non si è potuto scrivere nello staging")
			default:
				arrivati = append(arrivati, f)
			}
		default:
			_ = part.Close()
			togliTutti()
			s.rifiutoSorgente(w, r, &filerfq.Rifiuto{Stato: http.StatusUnprocessableEntity, Codice: "CORPO_NON_VALIDO",
				Testo: "Niente è cambiato: il caricamento porta solo «origine» e i file.", Extra: map[string]any{"campo": part.FormName()}})
			return
		}
		_ = part.Close()
	}
	interna := origine == "interna"
	u := utenteDa(r.Context())
	svc := s.cicloFile()
	caricati, presenti := 0, 0
	for i, f := range arrivati {
		if interna && !tecnicoCaricabile(f.Nome) && !archivio.Estraibile(filepath.Ext(f.Nome)) {
			staging.RimuoviParte(f.Parte)
			rifiutato(f.Nome, "TIPO_FILE", "come versione interna si caricano CAD 3D, disegni, sviluppi DXF e archivi")
			continue
		}
		es, err := svc.RegistraFile(r.Context(), thread, *u, interna, f)
		if err != nil {
			var rf rifiuto
			if errors.As(err, &rf) {
				rifiutato(f.Nome, "ERRORE_STAGING", string(rf))
				continue
			}
			for _, g := range arrivati[i+1:] {
				staging.RimuoviParte(g.Parte)
			}
			s.Log.Error("caricamento JSON nella RFQ: registrazione", "rfq", thread, "file", f.Nome, "err", err)
			w.Header().Set("X-Motivo", "DA_VERIFICARE")
			s.scriviJSONSorgente(w, r, http.StatusInternalServerError, map[string]any{"esito": "da_verificare",
				"testo": "Non so se tutti i file sono entrati: rimanda gli stessi file, quelli già entrati risponderanno «già presente»."})
			return
		}
		switch es.Esito {
		case "caricato":
			caricati++
		case "gia_presente":
			presenti++
		}
		esiti = append(esiti, es)
	}
	if esiti == nil {
		esiti = []filerfq.EsitoFile{}
	}
	nRifiutati := len(esiti) - caricati - presenti
	if caricati+presenti == 0 {
		s.rifiutoSorgente(w, r, &filerfq.Rifiuto{Stato: http.StatusUnprocessableEntity, Codice: "NESSUN_FILE_CARICATO",
			Testo: "Niente è cambiato: nessun file è stato caricato.", Extra: map[string]any{"file": esiti, "origine": origine}})
		return
	}
	esito := "caricati"
	if nRifiutati > 0 {
		esito = "parziale"
	}
	testo := fmt.Sprintf("%s, %s, %s.", fraseConta(caricati, "file caricato", "file caricati"),
		fraseConta(presenti, "già presente", "già presenti"), fraseConta(nRifiutati, "rifiutato", "rifiutati"))
	s.scriviJSONSorgente(w, r, http.StatusOK, map[string]any{"esito": esito, "origine": origine, "file": esiti, "testo": testo})
}
