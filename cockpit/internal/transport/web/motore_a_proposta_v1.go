package web

// motore_a_proposta_v1.go: R15 (U2 §5.7; ondata C, corsia persistenza), «Rifiuta la proposta aggiornata…» per un prodotto
// confermato in formato 3 senza bozza:
//   - R15 POST /thread/{id}/motore-a/proposta-aggiornata/rifiuta
//     corpo {chiave_idempotenza, prodotto_id, conferma_id, gruppo, impronta_contenuto, motivo}.
// La catena e' quella delle rotte della bozza (conMotoreA → conNoStore → soloAdmin), montata da registraBozzaMotoreA.
// Il calcolo del motore avviene prima della transazione, in sola lettura (come R2 nelle forme che importano): la proposta
// di adesso del gruppo dichiarato (propostabom.ImportaGruppo sull'esito del calcolo) e le differenze con il registro della
// conferma (propostabom.Confronta), che il rifiuto conserva com'erano. Poi distintabozza.RifiutaPropostaAggiornata, con il
// lucchetto della RFQ, scrive la riga del rifiuto e l'esito dell'intento. Niente altro: nessuna riga legacy, nessun
// componente, nessuna conferma, nessuna fonte, nessun cambio della verifica (vincolo [U] di DEC:125).
//
// Codici: 404 la RFQ; 409 ANTEPRIMA_SUPERATA (proposta_step, conferma_non_corrente), CHIAVE_RIUSATA; 422 CORPO_NON_VALIDO
// (conferma_non_formato3, nessuna_proposta_aggiornata, bozza_aperta), FUORI_RFQ, MOTIVO_OBBLIGATORIO; 503 CALCOLO_OCCUPATO
// e NON_CONFIGURATO, 504 CALCOLO_SCADUTO (niente scritto); 5xx «da verificare» (R6 con la stessa chiave).

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/db"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// maxCorpoR15: il corpo di R15 (piccolo: sei campi).
const maxCorpoR15 = 16 << 10

// I codici del calcolo del motore nelle rotte della bozza (EM §10.10).
const (
	codiceCalcoloOccupato = "CALCOLO_OCCUPATO"
	codiceCalcoloScaduto  = "CALCOLO_SCADUTO"
	codiceNonConfigurato  = "NON_CONFIGURATO"
)

// CodiciServerR15: i codici di R15. Non sono ancora nell'elenco unico di T-05 (CodiciServerBozzaV1), che li confronta con
// i cataloghi del ponte: il ponte non ha ancora la funzione di R15 (richiesta alla corsia interfaccia dell'ondata C).
var CodiciServerR15 = []CodiceRotta{
	{"R15", distintabozza.CodiceAnteprimaSuperata, http.StatusConflict},
	{"R15", distintabozza.CodiceChiaveRiusata, http.StatusConflict},
	{"R15", distintabozza.CodiceCorpoNonValido, http.StatusUnprocessableEntity},
	{"R15", distintabozza.CodiceFuoriRFQ, http.StatusUnprocessableEntity},
	{"R15", distintabozza.CodiceMotivoObbligatorio, http.StatusUnprocessableEntity},
	{"R15", codiceCalcoloOccupato, http.StatusServiceUnavailable},
	{"R15", codiceNonConfigurato, http.StatusServiceUnavailable},
	{"R15", codiceCalcoloScaduto, http.StatusGatewayTimeout},
}

// registraPropostaV1 monta R15 e la sua riga gemella (la chiama registraBozzaMotoreA).
func (s *Server) registraPropostaV1(mux *http.ServeMux) {
	mux.HandleFunc("POST /thread/{id}/motore-a/proposta-aggiornata/rifiuta", s.conMotoreA(conNoStore(s.soloAdmin(s.propostaV1Rifiuta))))
	mux.HandleFunc("/thread/{id}/motore-a/proposta-aggiornata/rifiuta", s.conMotoreA(conNoStore(metodoNonAmmessoBozzaV1("POST"))))
	// direttiva 39, corsia motore: S6, l'anteprima dello STEP prima di S8 (motore_a_anteprima_s6.go)
	s.registraAnteprimaS6(mux)
}

// propostaV1Rifiuta: R15.
func (s *Server) propostaV1Rifiuta(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.idDelPercorsoV1(w, r, "id")
	if !ok || !s.rfqEsisteV1(w, r, thread) {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxCorpoR15+1))
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	if len(raw) > maxCorpoR15 {
		s.scriviJSONBozzaV1(w, r, http.StatusRequestEntityTooLarge, rifiutoBozzaV1{Esito: "rifiutato", Codice: codiceTroppoGrandeV1,
			Testo: "Il corpo della richiesta è troppo grande."})
		return
	}
	req, impronta, err := distintabozza.LeggiRichiestaR15(raw)
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	adesso, err := s.propostaDiAdessoV1(r.Context(), thread, req)
	if err != nil {
		s.rispondiErroreCalcoloV1(w, r, thread, err)
		return
	}
	out, err := distintabozza.RifiutaPropostaAggiornata(r.Context(), s.Pool, thread, utenteDa(r.Context()).UtenteID, req, impronta, adesso)
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	s.scriviJSONBozzaV1(w, r, http.StatusOK, out)
}

// propostaDiAdessoV1: la proposta di adesso del gruppo dichiarato (il calcolo del motore, in sola lettura) e le differenze
// con il registro della conferma dichiarata, se e' una conferma formato 3 con un registro; un gruppo che non si importa
// piu' e' una proposta non disponibile (ANTEPRIMA_SUPERATA nella transazione). Gli errori del calcolo (occupato, scaduto,
// regole) tornano a chi chiama, che li traduce in 503/504.
//
// L'adattatore vuole la fotografia del calcolo (Ingresso.Foto, U2 §4.4: la traccia dei nodi fuori per decisione): senza,
// ImportaGruppo rifiuta sempre e la proposta di adesso risulterebbe sempre «non disponibile» (difetto segnalato dalla
// corsia persistenza in C-3, corretto dalla corsia motore con la direttiva 39; prova L4 in motore_a_proposta_v1_db_test.go).
func (s *Server) propostaDiAdessoV1(ctx context.Context, thread uuid.UUID, req distintabozza.RichiestaR15) (distintabozza.PropostaDiAdesso, error) {
	var out distintabozza.PropostaDiAdesso
	ris, err := s.servizioMotoreA().Calcola(ctx, thread)
	if err != nil {
		return out, err
	}
	rif := ""
	for _, pv := range ris.EsitoThread.ProdottiValutati {
		if pv.ComponenteID != nil && *pv.ComponenteID == req.ProdottoID {
			rif = pv.Rif
		}
	}
	if rif == "" {
		return out, nil
	}
	reg, err := propostabom.ImportaGruppo(propostabom.Ingresso{Esito: &ris.Esito, Thread: ris.EsitoThread, Regole: ris.Regole, Prodotto: rif,
		Gruppo: req.Gruppo, NonAttivi: ris.V1.NonAttivi, Foto: &ris.Foto})
	if err != nil {
		// un gruppo che non c'e' piu', non si importa o e' su un file non attivo: la proposta che l'ingegnere ha visto non
		// c'e' piu' (il dettaglio nel log)
		if s.Log != nil {
			s.Log.Info("R15: la proposta del gruppo non si importa", "thread", thread, "err", err)
		}
		return out, nil
	}
	out.Disponibile, out.ImprontaContenuto = true, reg.Contenuto.ImprontaContenuto
	// le differenze con il registro della conferma (se c'e'), lette in sola lettura
	_ = s.inSolaLetturaV1(ctx, func(q *db.Queries) error {
		c, err := q.GetConfermaStrutturale(ctx, db.GetConfermaStrutturaleParams{ThreadID: thread, ConfermaID: req.ConfermaID})
		if err != nil || !c.ImportazioneID.Valid {
			return nil
		}
		riga, err := q.GetDistintaImportazione(ctx, db.GetDistintaImportazioneParams{ImportazioneID: c.ImportazioneID.UUID, ThreadID: thread})
		if errors.Is(err, pgx.ErrNoRows) || err != nil {
			return nil
		}
		vecchio, _, err := distintabozza.RileggiImportazione(riga)
		if err != nil {
			return nil
		}
		if b, err := json.Marshal(propostabom.Confronta(vecchio.Contenuto, reg.Contenuto).Differenze); err == nil {
			out.Differenze = b
		}
		return nil
	})
	return out, nil
}

// rispondiErroreCalcoloV1: gli errori del calcolo del motore nelle rotte della bozza (EM §10.10): 503 CALCOLO_OCCUPATO o
// NON_CONFIGURATO, 504 CALCOLO_SCADUTO, niente scritto; gli altri come le rotte della bozza.
func (s *Server) rispondiErroreCalcoloV1(w http.ResponseWriter, r *http.Request, thread uuid.UUID, err error) {
	switch {
	case errors.Is(err, motorea_web.ErrRFQInesistente):
		s.rispondiErroreBozzaV1(w, r, thread, &distintabozza.Errore{Codice: distintabozza.CodiceRFQInesistente, Messaggio: "la RFQ non c'e'"})
	case errors.Is(err, motorea_web.ErrRegole):
		s.scriviJSONBozzaV1(w, r, http.StatusServiceUnavailable, rifiutoBozzaV1{Esito: "non_registrato", Codice: codiceNonConfigurato,
			Testo: "Le regole del motore A non sono caricate: niente è stato scritto."})
	case errors.Is(err, motorea_web.ErrOccupato):
		s.scriviJSONBozzaV1(w, r, http.StatusServiceUnavailable, rifiutoBozzaV1{Esito: "non_registrato", Codice: codiceCalcoloOccupato,
			Testo: "Il motore è occupato: niente è stato scritto, riprova."})
	case errors.Is(err, motorea_web.ErrTempo) || errors.Is(err, context.DeadlineExceeded):
		s.scriviJSONBozzaV1(w, r, http.StatusGatewayTimeout, rifiutoBozzaV1{Esito: "non_registrato", Codice: codiceCalcoloScaduto,
			Testo: "Il calcolo del motore ha superato il tempo: niente è stato scritto, riprova."})
	default:
		s.rispondiErroreBozzaV1(w, r, thread, err)
	}
}
