package web

// Gli inneschi web del flusso ancorato al prodotto (Smistamento F8, addendum A5.13.6): l'aggancio della RFQ
// (IN1), le decisioni dell'operatore (IN7), «Rianalizza» (IN8), «Aggiorna le proposte» (IN9), e i file che
// una rotta fa scendere in staging (IN4, e i file fermi completati dalla preparazione, IN3). Ognuno gira DOPO
// il commit del gesto, in una transazione sua (FP6): un errore del flusso finisce nel log e non disfa il
// gesto, che e' gia' salvato (P31). Nessuna GET: aprire una pagina, il poll e la preparazione in se' non
// muovono il flusso (F1, R8).
//
// Non c'e' una schermata: la vista dello Smistamento e' F10. Le destinazioni si leggono nel JSON di
// documento_proposta.dettagli.destinazione.

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

func (s *Server) registraSmistamento(mux *http.ServeMux) {
	mux.HandleFunc("POST /thread/{id}/smistamento/aggiorna", s.autenticato(s.aggiornaSmistamento))
}

// smistatore e' il giro del flusso di questo server: il pool e l'analizzatore della configurazione.
func (s *Server) smistatore() fascicolo.Smistatore {
	return fascicolo.SmistatoreDi(s.Pool, s.Analizzatore)
}

// rismista fa un giro del flusso sulla RFQ dopo un gesto gia' salvato. Un errore si scrive e basta.
func (s *Server) rismista(ctx context.Context, thread uuid.UUID) {
	r := fascicolo.NuoviRismistamenti()
	r.Segna(thread)
	r.Esegui(context.WithoutCancel(ctx), s.smistatore(), s.Log)
}

// rispostaConStato ricorda lo stato della risposta: il flusso gira solo dopo un gesto andato a buon fine.
type rispostaConStato struct {
	http.ResponseWriter
	stato int
}

func (w *rispostaConStato) WriteHeader(stato int) {
	if w.stato == 0 {
		w.stato = stato
	}
	w.ResponseWriter.WriteHeader(stato)
}

func (w *rispostaConStato) Write(b []byte) (int, error) {
	if w.stato == 0 {
		w.stato = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lascia a http.ResponseController il writer vero.
func (w *rispostaConStato) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// dopoIlGesto e' l'innesco delle decisioni sulla RFQ del percorso (IN7, IN8): il gesto gira con una raccolta
// nel contesto (i file che fa scendere in staging la riempiono), e dopo, se il gesto e' andato a buon fine,
// il flusso rifa' tutta la RFQ. Un gesto rifiutato con «Niente è cambiato» fa un giro che non riscrive niente:
// le destinazioni sono una funzione dello stato, e lo stato non e' cambiato.
func (s *Server) dopoIlGesto(h http.HandlerFunc) http.HandlerFunc {
	return s.conRismistamento(h, true)
}

// dopoICaricamenti e' l'innesco dei gesti che fanno arrivare file (IN4) o completano un file fermo (IN3, la
// preparazione): il flusso rifa' solo i file che il gesto ha segnato, e tutta la RFQ se l'indice e' cambiato.
// La preparazione in se' non muove il flusso: accoda, e il flusso partira' dal risultato.
func (s *Server) dopoICaricamenti(h http.HandlerFunc) http.HandlerFunc {
	return s.conRismistamento(h, false)
}

func (s *Server) conRismistamento(h http.HandlerFunc, rfq bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, raccolta := fascicolo.ConRismistamenti(r.Context())
		rw := &rispostaConStato{ResponseWriter: w}
		h(rw, r.WithContext(ctx))
		if rw.stato >= http.StatusBadRequest {
			return
		}
		if thread, err := uuid.Parse(r.PathValue("id")); err == nil && rfq {
			raccolta.Segna(thread)
		}
		raccolta.Esegui(context.WithoutCancel(r.Context()), s.smistatore(), s.Log)
	}
}

// dopoLaProposta e' dopoIlGesto per le rotte che nominano una proposta di documento e non la RFQ
// (/proposta/{id}/…): la RFQ e' quella del file della proposta.
func (s *Server) dopoLaProposta(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, raccolta := fascicolo.ConRismistamenti(r.Context())
		rw := &rispostaConStato{ResponseWriter: w}
		h(rw, r.WithContext(ctx))
		if rw.stato >= http.StatusBadRequest {
			return
		}
		if pid, err := uuid.Parse(r.PathValue("id")); err == nil {
			if thread, ok := rfqDellaProposta(r.Context(), db.New(s.Pool), pid); ok {
				raccolta.Segna(thread)
			}
		}
		raccolta.Esegui(context.WithoutCancel(r.Context()), s.smistatore(), s.Log)
	}
}

// rfqDellaProposta e' la RFQ del messaggio da cui viene il file della proposta.
func rfqDellaProposta(ctx context.Context, q *db.Queries, pid uuid.UUID) (uuid.UUID, bool) {
	p, err := q.GetProposta(ctx, pid)
	if err != nil {
		return uuid.Nil, false
	}
	if p.ThreadID.Valid {
		return p.ThreadID.UUID, true
	}
	a, err := q.GetAllegato(ctx, p.AllegatoID)
	if err != nil {
		return uuid.Nil, false
	}
	m, err := q.GetMessaggio(ctx, a.MessaggioID)
	if err != nil || !m.ThreadID.Valid {
		return uuid.Nil, false
	}
	return m.ThreadID.UUID, true
}

// aggiornaSmistamento: POST /thread/{id}/smistamento/aggiorna, «Aggiorna le proposte» (IN9, almeno
// operatore: `autenticato` rifiuta chi consulta). Rifa' le destinazioni di tutta la RFQ con lo stato di
// adesso: e' il rimedio di tutti i casi in cui un evento non ha fatto girare il flusso (le regole del cliente
// cambiate, un errore, un riavvio fra il commit e il flusso), e la GET lo offre quando la firma dell'indice
// non torna. Risponde con la pagina della RFQ e l'avviso.
func (s *Server) aggiornaSmistamento(w http.ResponseWriter, r *http.Request) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	es, err := fascicolo.Rismista(r.Context(), s.Pool, thread, fascicolo.AmbitoRfq(), s.Analizzatore)
	var avviso string
	switch {
	case err != nil:
		s.Log.Warn("aggiorna le proposte: non riuscito", "rfq", thread, "err", err)
		avviso = "Niente è cambiato: " + spiegaErrore(err)
	case es.Saltata != "":
		avviso = "Le proposte di una RFQ " + es.Saltata + " non si aggiornano."
	case es.Scritte == 0:
		avviso = fmt.Sprintf("Le proposte erano già aggiornate (%s).", conta(es.File, "file", "file"))
	default:
		avviso = fmt.Sprintf("Proposte aggiornate: %s su %s.", conta(int(es.Scritte), "destinazione riscritta", "destinazioni riscritte"),
			conta(es.File, "file", "file"))
	}
	s.threadFrammento(w, r, thread, avviso)
}
