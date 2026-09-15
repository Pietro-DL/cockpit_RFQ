package web

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"promatec/cockpit/internal/db"
)

// Schermata B — il thread RFQ: la "chat" (tutti i messaggi agganciati, dal DB), gli allegati da smistare,
// i documenti confermati sul NAS e il fascicolo calcolato da v_fascicolo.

type threadDati struct {
	T              db.ThreadOfferta
	Riga           db.VCruscotto
	Cliente        db.Cliente
	Identificativi []db.IdentificativoThread
	Messaggi       []messaggioThread
	Documenti      []db.Documento
	Fascicolo      []db.VFascicolo
	Bozze          []db.Bozza
	Componenti     []db.Componente
	NDaSmistare    int
	Avviso         string
	Selezion       string
}

type messaggioThread struct {
	M        db.Messaggio
	Presenza *db.PresenzaDaAprireRow // nil = messaggio non Outlook, o non più in nessuna casella attiva
	Allegati []AllegatoUI
}

func (s *Server) thread(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	d, err := s.caricaThread(r.Context(), id)
	if err != nil {
		http.Error(w, "thread non trovato", 404)
		return
	}
	d.Selezion = r.URL.Query().Get("sel")
	s.rendi(w, r, "thread.html", "thread_corpo", "RFQ", d)
}

// threadFrammento ri-renderizza il corpo della pagina thread dopo un'azione (conferma, scarta, download).
func (s *Server) threadFrammento(w http.ResponseWriter, r *http.Request, id uuid.UUID, avviso string) {
	d, err := s.caricaThread(r.Context(), id)
	if err != nil {
		http.Error(w, "thread non trovato", 404)
		return
	}
	d.Avviso = avviso
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pagine["thread.html"].ExecuteTemplate(w, "thread_corpo", vista{Utente: utenteDa(r.Context()), Titolo: "RFQ", Dati: d, Frammento: true}); err != nil {
		s.Log.Error("template", "frammento", "thread_corpo", "err", err)
	}
}

func (s *Server) caricaThread(ctx context.Context, id uuid.UUID) (*threadDati, error) {
	q := db.New(s.Pool)
	t, err := q.GetThread(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &threadDati{T: t}
	d.Riga, _ = q.GetCruscottoRiga(ctx, id)
	d.Cliente, _ = q.GetCliente(ctx, t.ClienteID)
	d.Identificativi, _ = q.ListIdentificativi(ctx, id)
	d.Documenti, _ = q.ListDocumentiThread(ctx, id)
	d.Fascicolo, _ = q.ListFascicolo(ctx, id)
	d.Bozze, _ = q.ListBozzeThread(ctx, uuid.NullUUID{UUID: id, Valid: true})
	d.Componenti, _ = q.ListComponentiThread(ctx, id)
	msgs, _ := q.ListMessaggiThread(ctx, uuid.NullUUID{UUID: id, Valid: true})
	for _, m := range msgs {
		mt := messaggioThread{M: m}
		if pr, err := q.PresenzaDaAprire(ctx, m.MessaggioID); err == nil {
			mt.Presenza = &pr
		}
		mt.Allegati, _ = s.allegatiUI(ctx, q, m.MessaggioID)
		for _, a := range mt.Allegati {
			if a.Proposta != nil && a.Proposta.Stato == db.StatoPropostaAperta && a.Natura == db.NaturaAllegatoFile {
				d.NDaSmistare++
			}
			for _, f := range a.Figli {
				if f.Proposta != nil && f.Proposta.Stato == db.StatoPropostaAperta {
					d.NDaSmistare++
				}
			}
		}
		d.Messaggi = append(d.Messaggi, mt)
	}
	return d, nil
}
