package web

// I codici della RFQ, B8.6: il pannello che mette insieme i codici che la RFQ ha visto nei messaggi, nei
// nomi e nei cartigli dei file e negli STEP (fascicolo.Unisci), e i due gesti che mancavano: «+ Prodotto /
// + Assieme / + Particolare» su un codice davvero nuovo, e il ripristino di un componente archiviato. Il
// terzo gesto del pannello, decidere la proposta STEP aperta di un codice, e' quello di B8.5.
//
// La schermata del Fascicolo e' B8.7: fino ad allora il pannello sta nella pagina della RFQ, e le rotte
// rispondono con quella.

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

func (s *Server) registraCodici(mux *http.ServeMux) {
	mux.HandleFunc("POST /thread/{id}/fascicolo/codice/aggiungi", s.autenticato(s.aggiungiCodice))
	mux.HandleFunc("POST /thread/{id}/fascicolo/componente/{cid}/ripristina", s.autenticato(s.ripristinaComponente))
}

// aggiungiCodice: POST .../fascicolo/codice/aggiungi, campi `codice`, `tipo` (finito, sottoassieme,
// sciolto) e, se le evidenze dicono revisioni diverse, `rev`: una di quelle viste.
func (s *Server) aggiungiCodice(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		tipo := db.TipoComponente(strings.TrimSpace(r.FormValue("tipo")))
		if !tipo.Valid() {
			return "", rifiuto("tipo di componente non valido")
		}
		return fascicolo.AggiungiDaCodice(ctx, q, thread, utente, r.FormValue("codice"), tipo, r.FormValue("rev"))
	})
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

// rigaCodice e' una riga del pannello: il codice, la RFQ per le rotte, e i documenti del suo componente
// se ce n'e' uno (e' quello che si vede «aprendolo»).
type rigaCodice struct {
	C         fascicolo.CodiceCandidato
	Thread    uuid.UUID
	Documenti []db.Documento
}

func nuovaRigaCodice(c fascicolo.CodiceCandidato, thread uuid.UUID, documenti map[uuid.UUID][]db.Documento) rigaCodice {
	r := rigaCodice{C: c, Thread: thread}
	if c.Stato.Componente != nil {
		r.Documenti = documenti[c.Stato.Componente.ComponenteID]
	}
	return r
}

// etichettaTipo e' il nome del tipo sul bottone: «Prodotto», «Assieme», «Particolare».
func etichettaTipo(t db.TipoComponente) string {
	n := fascicolo.NomeTipo(t)
	if n == "" {
		return n
	}
	return strings.ToUpper(n[:1]) + n[1:]
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
