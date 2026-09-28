package web

// Le proposte di struttura, B8.5: accettare o scartare un nodo, un arco, un sottoalbero, un file intero,
// una rimozione; scrivere il codice di un nodo che il server non ha saputo classificare; rileggere gli
// STEP della RFQ. Le regole stanno in core/rfq/fascicolo (decisioni.go, rianalisi.go): qui si leggono i
// parametri, si apre la transazione e si risponde con l'avviso, come la schermata da cui arriva il gesto
// (threadFrammento: il Fascicolo con i suoi pannelli, o la pagina della RFQ).

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// MaxAccodatiSuRichiesta: quanti STEP senza fatti correnti accoda il gesto esplicito «Rianalizza».
const MaxAccodatiSuRichiesta = 20

func (s *Server) registraProposte(mux *http.ServeMux) {
	mux.HandleFunc("POST /thread/{id}/fascicolo/rianalizza", s.autenticato(s.dopoIlGesto(s.rianalizza)))
	mux.HandleFunc("POST /thread/{id}/fascicolo/nodo/{pid}/accetta", s.autenticato(s.dopoIlGesto(s.accettaNodo)))
	mux.HandleFunc("POST /thread/{id}/fascicolo/nodo/{pid}/scarta", s.autenticato(s.dopoIlGesto(s.scartaNodo)))
	mux.HandleFunc("POST /thread/{id}/fascicolo/nodo/{pid}/codice", s.autenticato(s.dopoIlGesto(s.codiceNodo)))
	mux.HandleFunc("POST /thread/{id}/fascicolo/relazione/accetta", s.autenticato(s.dopoIlGesto(s.accettaRelazione)))
	mux.HandleFunc("POST /thread/{id}/fascicolo/relazione/scarta", s.autenticato(s.dopoIlGesto(s.scartaRelazione)))
	mux.HandleFunc("POST /thread/{id}/fascicolo/file/{aid}/accetta", s.autenticato(s.dopoIlGesto(s.accettaFile)))
	mux.HandleFunc("POST /thread/{id}/fascicolo/rimozione/accetta", s.autenticato(s.dopoIlGesto(s.accettaRimozione)))
	mux.HandleFunc("POST /thread/{id}/fascicolo/rimozione/scarta", s.autenticato(s.dopoIlGesto(s.scartaRimozione)))
}

// gesto esegue fai in una transazione e risponde con la pagina della RFQ: l'avviso di fai, oppure il
// rifiuto con «niente è cambiato». Tutto o niente.
func (s *Server) gesto(w http.ResponseWriter, r *http.Request, fai func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error)) {
	s.gestoPoi(w, r, fai, nil)
}

// gestoPoi e' gesto con un passo prima della risposta: poi riceve l'esito (true = fatto e salvato, dopo il
// commit) con la frase per l'operatore, e puo' mettere un'intestazione, per esempio l'evento dell'editor.
func (s *Server) gestoPoi(w http.ResponseWriter, r *http.Request, fai func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error),
	poi func(w http.ResponseWriter, ok bool, testo string)) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form non valido", 400)
		return
	}
	u := utenteDa(r.Context())
	ctx := r.Context()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer tx.Rollback(ctx)
	msg, err := fai(ctx, db.New(tx), thread, u.UtenteID)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		msg := "Niente è cambiato: " + spiegaErrore(err)
		if poi != nil {
			poi(w, false, msg)
		}
		s.threadFrammento(w, r, thread, msg)
		return
	}
	if poi != nil {
		poi(w, true, msg)
	}
	s.threadFrammento(w, r, thread, msg)
}

// idDa legge un id dal path o dal form; un id non valido e' un rifiuto, non un 400: il resto della
// pagina deve tornare.
func idDa(v, cosa string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(v))
	if err != nil {
		return uuid.Nil, rifiuto(cosa + " non valido")
	}
	return id, nil
}

// rianalizza: POST /thread/{id}/fascicolo/rianalizza. Rilegge gli STEP che hanno i fatti correnti e
// accoda gli altri, fino a MaxAccodatiSuRichiesta: le due meta' di RianalizzaRfq insieme. E' il solo gesto
// che rilegge gli STEP a richiesta; aprire una pagina non lo fa piu' (Smistamento F1, R8). Ed e' il solo che
// fa rileggere i PDF letti da un analizzatore precedente (F9, analizzatore 4): nessuna rianalisi parte da sola.
func (s *Server) rianalizza(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, _ uuid.UUID) (string, error) {
		if s.Analizzatore.Versione == 0 {
			return "", rifiuto("l'analisi non è configurata ([analisi].versione)")
		}
		ri, err := fascicolo.RianalizzaRfq(ctx, q, thread, s.Analizzatore, MaxAccodatiSuRichiesta)
		if err != nil {
			return "", err
		}
		return fraseRianalisi(ri), nil
	})
}

func fraseRianalisi(ri fascicolo.Rianalisi) string {
	parti := []string{}
	if ri.Riletti > 0 {
		parti = append(parti, conta(ri.Riletti, "STEP riletto", "STEP riletti")+" con le regole del cliente")
	}
	if ri.Accodati > 0 {
		parti = append(parti, conta(ri.Accodati, "analisi accodata", "analisi accodate"))
	}
	if ri.GiaInCoda > 0 {
		parti = append(parti, conta(ri.GiaInCoda, "analisi già in coda", "analisi già in coda"))
	}
	if ri.Rimandati > 0 {
		parti = append(parti, conta(ri.Rimandati, "STEP rimandato", "STEP rimandati")+" al prossimo giro")
	}
	if ri.SenzaStaging > 0 {
		parti = append(parti, conta(ri.SenzaStaging, "STEP non più in staging", "STEP non più in staging")+" (Riscarica)")
	}
	// i PDF letti da un analizzatore precedente (F9): si dice di quali file si parla
	if ri.PdfAccodati > 0 {
		parti = append(parti, conta(ri.PdfAccodati, "PDF accodato", "PDF accodati")+" per rileggerne il testo")
	}
	if ri.PdfGiaInCoda > 0 {
		parti = append(parti, conta(ri.PdfGiaInCoda, "PDF già in coda", "PDF già in coda"))
	}
	if ri.PdfRimandati > 0 {
		parti = append(parti, conta(ri.PdfRimandati, "PDF rimandato", "PDF rimandati")+" al prossimo giro")
	}
	if ri.PdfSenzaStaging > 0 {
		parti = append(parti, conta(ri.PdfSenzaStaging, "PDF non più in staging", "PDF non più in staging")+" (Riscarica)")
	}
	if len(parti) == 0 {
		// dal F9 il gesto guarda anche i PDF: la frase dice tutti e due, non solo gli STEP
		return "Niente da rileggere in questa RFQ: STEP e PDF sono già letti con l'analizzatore corrente."
	}
	return strings.Join(parti, ", ") + "."
}

// conta scrive un numero con il nome giusto: «1 analisi accodata», «3 analisi accodate».
func conta(n int, uno, tanti string) string {
	return fmt.Sprintf("%d %s", n, plurale(n, uno, tanti))
}

// accettaNodo: POST .../nodo/{pid}/accetta, campo facoltativo `tipo` (finito, sottoassieme, sciolto,
// commerciale): senza, vale quello suggerito.
func (s *Server) accettaNodo(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		pid, err := idDa(r.PathValue("pid"), "proposta")
		if err != nil {
			return "", err
		}
		tipo := db.TipoComponente(strings.TrimSpace(r.FormValue("tipo")))
		if tipo != "" && !tipo.Valid() {
			return "", rifiuto("tipo di componente non valido")
		}
		return fascicolo.AccettaNodo(ctx, q, thread, pid, utente, tipo)
	})
}

func (s *Server) scartaNodo(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		pid, err := idDa(r.PathValue("pid"), "proposta")
		if err != nil {
			return "", err
		}
		return fascicolo.ScartaNodo(ctx, q, thread, pid, utente)
	})
}

// codiceNodo: POST .../nodo/{pid}/codice, campi `codice` e `rev`.
func (s *Server) codiceNodo(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, _ uuid.UUID) (string, error) {
		pid, err := idDa(r.PathValue("pid"), "proposta")
		if err != nil {
			return "", err
		}
		return fascicolo.CodiceDelNodo(ctx, q, thread, pid, r.FormValue("codice"), r.FormValue("rev"))
	})
}

// chiaveRelazione legge `allegato`, `padre`, `figlio` (le chiavi dei due nodi nel file).
func chiaveRelazione(r *http.Request) (fascicolo.ChiaveRelazione, error) {
	a, err := idDa(r.FormValue("allegato"), "allegato")
	if err != nil {
		return fascicolo.ChiaveRelazione{}, err
	}
	k := fascicolo.ChiaveRelazione{Allegato: a, Padre: strings.TrimSpace(r.FormValue("padre")), Figlio: strings.TrimSpace(r.FormValue("figlio"))}
	if k.Padre == "" || k.Figlio == "" {
		return k, rifiuto("relazione non indicata")
	}
	return k, nil
}

func (s *Server) accettaRelazione(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		k, err := chiaveRelazione(r)
		if err != nil {
			return "", err
		}
		return fascicolo.AccettaRelazione(ctx, q, thread, k, utente)
	})
}

func (s *Server) scartaRelazione(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		k, err := chiaveRelazione(r)
		if err != nil {
			return "", err
		}
		return fascicolo.ScartaRelazione(ctx, q, thread, k, utente)
	})
}

// accettaFile: POST .../file/{aid}/accetta; con `chiave` accetta solo il sottoalbero di quel nodo.
func (s *Server) accettaFile(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		aid, err := idDa(r.PathValue("aid"), "allegato")
		if err != nil {
			return "", err
		}
		if chiave := strings.TrimSpace(r.FormValue("chiave")); chiave != "" {
			return fascicolo.AccettaSottoalbero(ctx, q, thread, aid, chiave, utente)
		}
		return fascicolo.AccettaFile(ctx, q, thread, aid, utente)
	})
}

// chiaveRimozione legge `step`, `padre`, `figlio` (documento e componenti).
func chiaveRimozione(r *http.Request) (fascicolo.ChiaveRimozione, error) {
	var k fascicolo.ChiaveRimozione
	var err error
	if k.Step, err = idDa(r.FormValue("step"), "STEP"); err != nil {
		return k, err
	}
	if k.Padre, err = idDa(r.FormValue("padre"), "padre"); err != nil {
		return k, err
	}
	if k.Figlio, err = idDa(r.FormValue("figlio"), "figlio"); err != nil {
		return k, err
	}
	return k, nil
}

func (s *Server) accettaRimozione(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		k, err := chiaveRimozione(r)
		if err != nil {
			return "", err
		}
		return fascicolo.AccettaRimozione(ctx, q, thread, k, utente)
	})
}

func (s *Server) scartaRimozione(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		k, err := chiaveRimozione(r)
		if err != nil {
			return "", err
		}
		return fascicolo.ScartaRimozione(ctx, q, thread, k, utente)
	})
}
