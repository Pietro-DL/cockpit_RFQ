package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/db"
	"promatec/cockpit/internal/jobs"
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
	// NDaCopiare: documenti confermati che NON sono sul NAS — in attesa oppure in errore.
	//
	// «In attesa» e' la conseguenza normale di una conferma data mentre [sicurezza].nas_scrittura era
	// spenta: la decisione dell'operatore e' registrata, la scrittura aspetta. «In errore» e' la copia
	// che ci ha provato e non ce l'ha fatta — il NAS non c'era, il contenuto era sparito dallo staging.
	//
	// Contano insieme perche' per chi guarda sono la stessa cosa — quel disegno non e' nel fascicolo —
	// e perche' un documento in errore senza un modo di riprovare sarebbe il secondo vicolo cieco dopo
	// quello che questo pulsante e' nato per togliere.
	NDaCopiare int
	Avviso     string
	Selezion   string
}

type messaggioThread struct {
	M db.Messaggio
	// Copia: quella servita dalla postazione della sessione; nil = «Apri» non disponibile, e Motivo
	// dice perché (messaggio non Outlook, nessuna postazione, nessun worker idoneo).
	Copia    *jobs.Copia
	Motivo   string
	Allegati []AllegatoUI
}

func (s *Server) thread(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	d, err := s.caricaThread(r.Context(), id, sessioneDa(r.Context()))
	if err != nil {
		http.Error(w, "thread non trovato", 404)
		return
	}
	d.Selezion = r.URL.Query().Get("sel")
	s.rendi(w, r, "thread.html", "thread_corpo", "RFQ", d)
}

// threadFrammento ri-renderizza il corpo della pagina thread dopo un'azione (conferma, scarta, download).
func (s *Server) threadFrammento(w http.ResponseWriter, r *http.Request, id uuid.UUID, avviso string) {
	d, err := s.caricaThread(r.Context(), id, sessioneDa(r.Context()))
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

// riprovaCopie rimette in coda la copia sul NAS dei documenti di questa RFQ che sono rimasti
// «in_coda» (checkpoint 3R, punto 6: uno stato incompleto deve potersi riconciliare con un'azione
// VISIBILE).
//
// E' il gesto di una persona, ed e' voluto che lo sia. Quando una capacita' si accende, i job che
// avevano aspettato vengono ANNULLATI e non eseguiti — nulla si mette in moto da solo perche'
// qualcuno ha cambiato una riga in un file. Quei documenti restano pero' confermati, e senza questo
// pulsante non esisteva nessun modo di portarli sul NAS: la conferma andava rifatta, cioe' la stessa
// decisione presa due volte, oppure il file non ci arrivava mai.
//
// Se la capacita' e' ancora spenta non si finge niente: l'accodamento rifiuta, e l'avviso lo dice con
// il nome della capacita'.
func (s *Server) riprovaCopie(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	ctx := r.Context()
	q := db.New(s.Pool)
	documenti, err := q.ListDocumentiThread(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	accodati, gia := 0, 0
	var spenta string
	for _, d := range documenti {
		if !daCopiare(d.StatoNas) {
			continue
		}
		j, err := jobs.Accoda(ctx, q, db.TipoJobCopiaNas, api.PayloadCopiaNAS{DocumentoID: d.DocumentoID},
			"nas:"+d.DocumentoID.String(), 1)
		switch {
		case errors.Is(err, jobs.ErrCapacitaSpenta):
			spenta = jobs.CapacitaMancante(err)
		case err != nil:
			s.Log.Error("riprova copie", "thread", id, "documento", d.DocumentoID, "err", err)
			http.Error(w, err.Error(), 500)
			return
		case j == nil:
			gia++ // la copia di questo documento era gia' in coda: non se ne accoda una seconda
		default:
			accodati++
		}
	}
	s.threadFrammento(w, r, id, avvisoCopie(accodati, gia, spenta))
}

// daCopiare: un documento confermato che non e' sul NAS. In attesa perche' la scrittura era spenta,
// oppure in errore perche' la copia non e' riuscita: in tutti e due i casi il file non c'e' e
// qualcuno lo sta aspettando.
func daCopiare(s db.StatoNas) bool {
	return s == db.StatoNasInCoda || s == db.StatoNasErrore
}

// avvisoCopie e' la frase che legge l'operatore: dice che cosa e' successo, non solo che l'azione e'
// riuscita. «Niente da rimettere in coda» e «due copie accodate» non sono la stessa notizia.
func avvisoCopie(accodati, gia int, spenta string) string {
	switch {
	case spenta != "":
		return fmt.Sprintf("Copie NON rimesse in coda: la capacita' [sicurezza].%s e' spenta su questo server.", spenta)
	case accodati == 0 && gia == 0:
		return "Nessun documento in attesa: sono tutti gia' sul NAS."
	case accodati == 0:
		return fmt.Sprintf("Nessuna copia nuova: %d erano gia' in coda.", gia)
	case gia == 0:
		return fmt.Sprintf("%d copi%s rimess%s in coda.", accodati, plurale(accodati, "a", "e"), plurale(accodati, "a", "e"))
	}
	return fmt.Sprintf("%d copi%s rimess%s in coda (%d erano gia' in attesa).",
		accodati, plurale(accodati, "a", "e"), plurale(accodati, "a", "e"), gia)
}

func plurale(n int, uno, molti string) string {
	if n == 1 {
		return uno
	}
	return molti
}

func (s *Server) caricaThread(ctx context.Context, id uuid.UUID, sess sessioneUI) (*threadDati, error) {
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
	for _, doc := range d.Documenti {
		if daCopiare(doc.StatoNas) {
			d.NDaCopiare++
		}
	}
	d.Fascicolo, _ = q.ListFascicolo(ctx, id)
	d.Bozze, _ = q.ListBozzeThread(ctx, uuid.NullUUID{UUID: id, Valid: true})
	d.Componenti, _ = q.ListComponentiThread(ctx, id)
	msgs, _ := q.ListMessaggiThread(ctx, uuid.NullUUID{UUID: id, Valid: true})
	for _, m := range msgs {
		mt := messaggioThread{M: m}
		if presenze, _ := q.ListPresenze(ctx, m.MessaggioID); len(presenze) > 0 {
			mt.Copia, mt.Motivo = s.copiaInterattiva(ctx, q, m.MessaggioID, sess)
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
