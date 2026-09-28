package web

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"promatec/cockpit/internal/ai/agente"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// L'analisi semantica in UI (checkpoint 3R §9).
//
// Due sole cose: un pulsante che ACCODA l'analisi, e un riquadro che mostra la proposta già
// verificata. Non c'è nessun percorso in cui una proposta dell'agente diventi un'azione da sola: per
// agganciare, creare una RFQ o confermare un codice si passa dagli stessi bottoni di sempre, che
// scrivono chi ha deciso.

// analisiUI è ciò che il pannello del messaggio mostra.
type analisiUI struct {
	Proposta agente.Proposta
	Scarti   []agente.Scarto
	// Disponibile: l'agente è acceso per questa casella, quindi il pulsante ha senso.
	Disponibile bool
	// Presente: esiste già un'analisi completata per questo messaggio.
	Presente bool
}

// analisiPer carica la proposta dell'agente per un messaggio, se c'è, e dice se se ne può chiedere una.
func (s *Server) analisiPer(ctx context.Context, q *db.Queries, id uuid.UUID, caselle []string) analisiUI {
	var a analisiUI
	a.Disponibile = s.Agente.Consentito(caselle)
	if p, scarti, ok := agente.LeggiProposta(ctx, q, id); ok {
		a.Proposta, a.Scarti, a.Presente = p, scarti, true
	}
	return a
}

// chiediAnalisi accoda l'analisi di un messaggio.
//
// Il controllo sta QUI e non solo nel template: nascondere un pulsante non è autorizzare, ed è la
// stessa regola di D29 sulla rail. Le caselle su cui l'analisi è permessa sono quelle elencate in
// configurazione, perché il testo di quelle mail esce verso un servizio esterno.
func (s *Server) chiediAnalisi(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	ctx := r.Context()
	q := db.New(s.Pool)
	presenze, err := q.ListPresenze(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	var caselle []string
	for _, p := range presenze {
		caselle = append(caselle, p.CasellaIndirizzo)
	}
	if !s.Agente.Consentito(caselle) {
		s.pannelloConAvviso(w, r, id, "L'analisi semantica non è attiva per questa casella. "+
			"Si accende in [agente] del file di configurazione, per casella, e serve la chiave nella variabile d'ambiente indicata.")
		return
	}
	if _, err := coda.Accoda(ctx, q, db.TipoJobAnalizzaMessaggioAi,
		worker.PayloadAnalizzaMessaggioAI{MessaggioID: id}, "analisi-ai:"+id.String(), 5); err != nil {
		s.pannelloConAvviso(w, r, id, "Analisi non accodata: "+err.Error())
		return
	}
	s.pannelloConAvviso(w, r, id, "Analisi richiesta. La proposta comparirà qui sotto quando sarà pronta: "+
		"è una proposta, e resta da decidere a te.")
}
