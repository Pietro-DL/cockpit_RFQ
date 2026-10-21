package web

// motore_a_bozza_v1_r5.go: R5 formato 3, la conferma atomica della BOM (EM §9.5, §7.5, App. I.6; direttiva 39 §1, §2, §7
// [U + T-39]), corsia persistenza. In un file a parte, montato da registraBozzaMotoreA:
//   - R5 POST /thread/{id}/motore-a/bom/conferma   {firma_anteprima, bozza, presa_d_atto_in_analisi?}
// con la riga gemella che risponde 405 agli altri metodi (Allow: POST).
//
// La catena e' quella di R1-R4: conMotoreA → conNoStore → soloAdmin → gestore. L'ordine: l'id del percorso (400) → la RFQ
// (404) → il corpo (413) → la forma del corpo (422, LeggiRichiestaR5; R5 non ha parametri) → distintabozza.Conferma3, che
// apre lei la transazione T e la connessione S (strategia A). Dopo il COMMIT, nella stessa richiesta, F8 sull'intera RFQ
// (C3 §5.3 passo 19) con l'esito in f8_stato (eseguito, risolto_da stessa_richiesta, oppure errore): un errore di F8 non
// revoca niente e la risposta resta 200 con f8_stato «errore» (R11 lo riprende).
//
// Le risposte (EM §9.5; una sola forma {esito, codice, testo, …}):
//   - 200 la conferma (ConfermaR5), anche riproposta con la stessa chiave e lo stesso corpo;
//   - 409 BASE_SUPERATA (motivi, versione_corrente), ANTEPRIMA_SUPERATA (motivo riepilogo_bom con il riepilogo nuovo,
//     documenti_in_analisi_cambiati, ingressi_in_movimento, riprova), CHIAVE_RIUSATA, BOZZA_VERSIONE (motivo
//     versione_diversa, bozza_chiusa, snapshot_diverso);
//   - 422 BLOCCHI con i blocchi (compreso STEP_SORGENTE_NON_CONFERMATA di una bozza senza sorgente S8 valida), NESSUNA_MODIFICA,
//     CORPO_NON_VALIDO (anche bozza_obbligatoria, formato_da_convertire, presa_d_atto_obbligatoria), FUORI_RFQ;
//   - 500 CONTROLLO_FINALE con esito non_registrato (un controllo differito al COMMIT non e' passato: niente e' scritto);
//   - 503 CALCOLO_OCCUPATO con esito non_registrato (la connessione S non e' arrivata, il motore e' occupato o non pronto);
//   - 504 CALCOLO_SCADUTO e 500 ERRORE_CALCOLO con esito non_registrato (il calcolo del motore nella connessione S e' scaduto
//     o non e' riuscito: motore_a_bozza_v1_motore.go);
//   - un altro 5xx: {esito: da_verificare, codice: ERRORE, chiave_idempotenza}: si rilegge con R6 prima di riprovare.

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// registraConfermaV1 monta R5 con la sua riga gemella.
func (s *Server) registraConfermaV1(mux *http.ServeMux) {
	catena := func(h http.HandlerFunc) http.HandlerFunc { return s.conMotoreA(conNoStore(s.soloAdmin(h))) }
	gemella := func(ammessi string) http.HandlerFunc {
		return s.conMotoreA(conNoStore(metodoNonAmmessoBozzaV1(ammessi)))
	}
	mux.HandleFunc("POST /thread/{id}/motore-a/bom/conferma", catena(s.bozzaV1Conferma))
	mux.HandleFunc("/thread/{id}/motore-a/bom/conferma", gemella("POST"))
}

// CodiciServerR5Altri: i codici di R5 fuori dai cataloghi 409/422 del ponte (T-05 confronta solo quelli): 500
// CONTROLLO_FINALE, 503 CALCOLO_OCCUPATO, 504 CALCOLO_SCADUTO e 500 ERRORE_CALCOLO, tutti con esito non_registrato (EM
// §9.5, §8.4).
var CodiciServerR5Altri = []CodiceRotta{
	{"R5", distintabozza.CodiceControlloFinale, http.StatusInternalServerError},
	{"R5", distintabozza.CodiceCalcoloOccupato, http.StatusServiceUnavailable},
	{"R5", distintabozza.CodiceCalcoloScaduto, http.StatusGatewayTimeout},
	{"R5", distintabozza.CodiceErroreCalcolo, http.StatusInternalServerError},
}

// rifiutoR5 e' il corpo di un rifiuto di R5 con le parti in piu': i blocchi, il riepilogo nuovo, la chiave.
type rifiutoR5 struct {
	rifiutoBozzaV1
	Blocchi           []distintabozza.Rilievo    `json:"blocchi,omitempty"`
	Riepilogo         *distintabozza.RiepilogoR4 `json:"riepilogo,omitempty"`
	ChiaveIdempotenza *uuid.UUID                 `json:"chiave_idempotenza,omitempty"`
}

// rispostaErroreR5 traduce un errore di R5 (commento di testa). Pura.
func rispostaErroreR5(err error, chiave *uuid.UUID) (int, rifiutoR5) {
	var r5 *distintabozza.RifiutoR5
	if errors.As(err, &r5) && r5 != nil && r5.Err != nil {
		_, base := rispostaErroreBozzaV1(r5.Err)
		out := rifiutoR5{rifiutoBozzaV1: base, Blocchi: r5.Blocchi, Riepilogo: r5.Riepilogo}
		switch r5.Err.Codice {
		case distintabozza.CodiceBlocchi:
			out.Esito = "rifiutato"
			if out.Blocchi == nil {
				out.Blocchi = []distintabozza.Rilievo{}
			}
			return http.StatusUnprocessableEntity, out
		case distintabozza.CodiceAnteprimaSuperata:
			out.Esito = "conflitto"
			return http.StatusConflict, out
		case distintabozza.CodiceControlloFinale, distintabozza.CodiceCalcoloOccupato, distintabozza.CodiceCalcoloScaduto, distintabozza.CodiceErroreCalcolo:
			// i codici fuori dalle rotte della bozza: la busta si scrive qui; il dettaglio tecnico va nel log, non al client
			out.rifiutoBozzaV1 = rifiutoBozzaV1{Esito: "non_registrato", Codice: r5.Err.Codice, Motivo: r5.Err.Motivo,
				Testo: "La conferma non è stata registrata: niente è cambiato. Rileggi il riepilogo e riprova con la stessa chiave."}
			out.ChiaveIdempotenza = chiave
			switch r5.Err.Codice {
			case distintabozza.CodiceCalcoloOccupato:
				out.Testo = "Il server è occupato: la conferma non è partita e niente è cambiato. Riprova fra poco con la stessa chiave."
				return http.StatusServiceUnavailable, out
			case distintabozza.CodiceCalcoloScaduto:
				out.Testo = "Il calcolo del motore non è finito in tempo: niente è cambiato. Riprova con la stessa chiave."
				return http.StatusGatewayTimeout, out
			}
			return http.StatusInternalServerError, out
		}
	}
	stato, base := rispostaErroreBozzaV1(err)
	out := rifiutoR5{rifiutoBozzaV1: base}
	if stato >= 500 {
		out.Esito, out.ChiaveIdempotenza = "da_verificare", chiave
		out.Testo = "Non so se la conferma è stata registrata: verifica con la stessa chiave (R6) prima di qualunque altro tentativo."
	}
	return stato, out
}

// bozzaV1Conferma: R5 (commento di testa).
func (s *Server) bozzaV1Conferma(w http.ResponseWriter, r *http.Request) {
	thread, ok := s.idDelPercorsoV1(w, r, "id")
	if !ok || !s.rfqEsisteV1(w, r, thread) {
		return
	}
	raw, ok := s.leggiCorpoV1(w, r)
	if !ok {
		return
	}
	for k := range r.URL.Query() {
		s.rispondiErroreBozzaV1(w, r, thread, &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Campo: k,
			Messaggio: "R5 non ha parametri"})
		return
	}
	richiesta, err := distintabozza.LeggiRichiestaR5(raw)
	if err != nil {
		s.rispondiErroreBozzaV1(w, r, thread, err)
		return
	}
	chiave := richiesta.Bozza.ChiaveIdempotenza
	ctx := r.Context()
	out, err := distintabozza.Conferma3(ctx, s.Pool, thread, richiesta, distintabozza.OpzioniR5{UtenteID: utenteDa(ctx).UtenteID,
		ImprontaRegole: improntaRegoleV1(s.MotoreA), Motore: s.motoreR5})
	if err != nil {
		stato, corpo := rispostaErroreR5(err, chiave)
		if stato >= 500 && s.Log != nil {
			s.Log.Error("motore A, R5: conferma non riuscita", "thread", thread, "stato", stato, "err", err)
		}
		s.scriviJSONBozzaV1(w, r, stato, corpo)
		return
	}
	if !out.Riproposto {
		out.F8Stato = s.f8DopoR5(ctx, thread, out.ConfermaID)
	}
	s.scriviJSONBozzaV1(w, r, http.StatusOK, out)
}

// f8DopoR5: F8 sull'intera RFQ dopo il COMMIT della conferma, e il suo esito in f8_stato (C3 §5.3 passo 19). Restituisce lo
// stato scritto; se non si riesce a scriverlo, quello che c'e' (di solito in_attesa).
func (s *Server) f8DopoR5(ctx context.Context, thread, conferma uuid.UUID) string {
	errF8 := s.smistatore()(context.WithoutCancel(ctx), thread, fascicolo.Ambito{Rfq: true})
	q := db.New(s.Pool)
	c := uuid.NullUUID{UUID: conferma, Valid: true}
	var err error
	stato := "eseguito"
	if errF8 == nil {
		_, err = q.SegnaF8DellaConfermaEseguito(context.WithoutCancel(ctx), db.SegnaF8DellaConfermaEseguitoParams{ThreadID: thread, ConfermaID: c})
	} else {
		stato = "errore"
		if s.Log != nil {
			s.Log.Error("motore A, R5: F8 dopo la conferma non riuscito", "thread", thread, "err", errF8)
		}
		_, err = q.SegnaF8DellaConfermaErrore(context.WithoutCancel(ctx), db.SegnaF8DellaConfermaErroreParams{ThreadID: thread, ConfermaID: c,
			Errore: pgtype.Text{String: testoErroreF8(errF8), Valid: true}})
	}
	if err != nil {
		if s.Log != nil {
			s.Log.Error("motore A, R5: lo stato di F8 non si scrive", "thread", thread, "err", err)
		}
		if f, e := q.GetF8StatoDellaConferma(context.WithoutCancel(ctx), db.GetF8StatoDellaConfermaParams{ThreadID: thread, ConfermaID: c}); e == nil {
			return f
		}
		return "in_attesa"
	}
	return stato
}
