package web

// I codici della RFQ, B8.6: il pannello che mette insieme i codici che la RFQ ha visto nei messaggi, nei
// nomi e nei cartigli dei file e negli STEP (fascicolo.Unisci). Sta nella pagina della RFQ e nel cassetto
// «Codici» del Fascicolo (B8.7).
//
// Smistamento F2 (R1): il pannello si legge e basta. I gesti che c'erano («+ Prodotto / + Assieme /
// + Particolare» con la rotta …/fascicolo/codice/aggiungi, «Accetta la proposta» e «Scarta» del nodo,
// «Ripristina») facevano di un'evidenza un componente con un clic, e sono tolti. Il ripristino di un
// archiviato resta, con la sua rotta, negli Archiviati della Struttura BOM.

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

func (s *Server) registraCodici(mux *http.ServeMux) {
	mux.HandleFunc("POST /thread/{id}/fascicolo/componente/{cid}/ripristina", s.autenticato(s.dopoIlGesto(s.ripristinaComponente)))
}

// ripristinaComponente: POST .../fascicolo/componente/{cid}/ripristina.
func (s *Server) ripristinaComponente(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, _ uuid.UUID) (string, error) {
		cid, err := idDa(r.PathValue("cid"), "componente")
		if err != nil {
			return "", err
		}
		return fascicolo.RipristinaComponente(ctx, q, thread, cid)
	})
}

// rigaCodice e' una riga del pannello: il codice e i documenti del suo componente se ce n'e' uno (e' quello
// che si vede «aprendolo»).
type rigaCodice struct {
	C         fascicolo.CodiceCandidato
	Documenti []db.Documento
}

func nuovaRigaCodice(c fascicolo.CodiceCandidato, documenti map[uuid.UUID][]db.Documento) rigaCodice {
	r := rigaCodice{C: c}
	if c.Stato.Componente != nil {
		r.Documenti = documenti[c.Stato.Componente.ComponenteID]
	}
	return r
}

// codiciDellaRfq riempie il pannello: i codici nelle due liste e, per aprire un componente, i suoi
// documenti. Se la lettura non riesce la pagina si apre lo stesso, e il pannello dice perche' e' vuoto.
func (s *Server) codiciDellaRfq(ctx context.Context, q *db.Queries, d *threadDati) {
	c, err := fascicolo.CandidatiDellaRfq(ctx, q, d.T.ThreadID)
	if err != nil {
		s.Log.Warn("codici della RFQ non letti", "rfq", d.T.ThreadID, "err", err)
		d.CodiciErrore = "i codici della richiesta non si sono potuti leggere: " + err.Error()
		return
	}
	d.Codici = c
	d.DocumentiDi = map[uuid.UUID][]db.Documento{}
	for _, doc := range d.Documenti {
		if doc.ComponenteID.Valid {
			d.DocumentiDi[doc.ComponenteID.UUID] = append(d.DocumentiDi[doc.ComponenteID.UUID], doc)
		}
	}
}
