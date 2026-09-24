package web

// Le rotte della schermata del Fascicolo, B8.7: la pagina, i pannelli, l'anteprima, e i gesti che fino a
// B8.6 esistevano solo in core/rfq/fascicolo (archiviare, togliere, scegliere lo STEP strutturale,
// derogare, sostituire un documento, aprire e abbandonare una revisione, congelare) o che mancavano
// (tipo, revisione e archi di un componente).
//
// Ogni gesto e' una transazione sola (s.gesto): tutto o niente, e il rifiuto dice perche' con
// «Niente è cambiato». Dalla schermata del Fascicolo la risposta e' l'avviso con i pannelli fuori banda;
// da altrove la pagina della RFQ, come prima (threadFrammento).

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

func (s *Server) registraFascicolo(mux *http.ServeMux) {
	mux.HandleFunc("GET /thread/{id}/fascicolo", s.autenticato(s.fascicoloPagina))
	mux.HandleFunc("GET /thread/{id}/fascicolo/parti", s.autenticato(s.fascicoloParti))
	mux.HandleFunc("GET /thread/{id}/fascicolo/anteprima", s.autenticato(s.fascicoloAnteprima))

	mux.HandleFunc("POST /thread/{id}/fascicolo/componente/{cid}/modifica", s.autenticato(s.modificaComponente))
	mux.HandleFunc("POST /thread/{id}/fascicolo/componente/{cid}/collega", s.autenticato(s.collegaComponente))
	mux.HandleFunc("POST /thread/{id}/fascicolo/componente/{cid}/scollega", s.autenticato(s.scollegaComponente))
	mux.HandleFunc("POST /thread/{id}/fascicolo/componente/{cid}/sposta", s.autenticato(s.spostaComponente))
	mux.HandleFunc("POST /thread/{id}/fascicolo/componente/{cid}/archivia", s.autenticato(s.archiviaComponente))
	mux.HandleFunc("POST /thread/{id}/fascicolo/componente/{cid}/rimuovi", s.autenticato(s.rimuoviComponente))
	mux.HandleFunc("POST /thread/{id}/fascicolo/componente/{cid}/step-strutturale", s.autenticato(s.stepStrutturale))
	mux.HandleFunc("POST /thread/{id}/fascicolo/componente/{cid}/deroga", s.autenticato(s.concediDeroga))
	mux.HandleFunc("POST /thread/{id}/fascicolo/deroga/{did}/revoca", s.autenticato(s.revocaDeroga))
	mux.HandleFunc("POST /thread/{id}/fascicolo/componente/{cid}/deroga-struttura", s.autenticato(s.concediDerogaStruttura))
	mux.HandleFunc("POST /thread/{id}/fascicolo/deroga-struttura/{did}/revoca", s.autenticato(s.revocaDerogaStruttura))
	mux.HandleFunc("POST /thread/{id}/fascicolo/documento/{did}/sostituisci", s.autenticato(s.sostituisciDocumento))
	mux.HandleFunc("POST /thread/{id}/fascicolo/documento/{did}/annulla-sostituzione", s.autenticato(s.annullaSostituzione))
	mux.HandleFunc("POST /thread/{id}/fascicolo/revisione/apri", s.autenticato(s.apriRevisione))
	mux.HandleFunc("POST /thread/{id}/fascicolo/revisione/abbandona", s.autenticato(s.abbandonaRevisione))
	mux.HandleFunc("POST /thread/{id}/fascicolo/congela", s.autenticato(s.congela))
	mux.HandleFunc("POST /thread/{id}/fascicolo/carica", s.autenticato(s.caricaVersioneInterna))
}

// ------------------------------------------------------------------ pagina e pannelli

func (s *Server) fascicoloPagina(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	// Come la pagina della RFQ (B8.5): aprire il Fascicolo rilegge gli STEP con le regole del cliente di
	// adesso, e accoda poche analisi che mancano.
	s.rileggiAllApertura(r.Context(), id)
	d, err := s.caricaFascicolo(r.Context(), id, leggiStatoFascicolo(r.URL.Query()), utenteDa(r.Context()))
	if err != nil {
		http.Error(w, "RFQ non trovata", 404)
		return
	}
	s.rendi(w, r, "fascicolo.html", "fasc_corpo", "Fascicolo", d)
}

// fascicoloParti rifa' tutti i pannelli tranne l'anteprima: e' la navigazione (un nodo, un filtro, la
// vista, il cassetto) e la risposta dei gesti.
func (s *Server) fascicoloParti(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	s.rispondiFascicolo(w, r, id, leggiStatoFascicolo(r.URL.Query()), "")
}

// fascicoloAnteprima apre un file nel terzo pannello.
func (s *Server) fascicoloAnteprima(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	d, err := s.caricaFascicolo(r.Context(), id, leggiStatoFascicolo(r.URL.Query()), utenteDa(r.Context()))
	if err != nil {
		http.Error(w, "RFQ non trovata", 404)
		return
	}
	s.eseguiFascicolo(w, "fasc_anteprima", d)
}

// rispondiFascicolo e' la risposta di un gesto o di una navigazione: l'avviso nel suo posto, e testata,
// struttura, documenti, completezza, cassetto e intestazione dell'anteprima fuori banda. Il corpo
// dell'anteprima non c'e': il PDF aperto resta aperto.
func (s *Server) rispondiFascicolo(w http.ResponseWriter, r *http.Request, thread uuid.UUID, st statoFascicolo, avviso string) {
	d, err := s.caricaFascicolo(r.Context(), thread, st, utenteDa(r.Context()))
	if err != nil {
		http.Error(w, "RFQ non trovata", 404)
		return
	}
	d.Avviso = avviso
	s.eseguiFascicolo(w, "fasc_parti", d)
}

func (s *Server) eseguiFascicolo(w http.ResponseWriter, nome string, d *fascicoloDati) {
	var buf bytes.Buffer
	if err := s.pagine["fascicolo.html"].ExecuteTemplate(&buf, nome, vista{Dati: d, Frammento: true}); err != nil {
		s.Log.Error("template", "frammento", nome, "err", err)
		http.Error(w, "errore nel disegnare il Fascicolo: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// ------------------------------------------------------------------ gesti sui componenti

func (s *Server) componenteDaPercorso(r *http.Request) (uuid.UUID, error) {
	return idDa(r.PathValue("cid"), "componente")
}

// idFacoltativo legge un id dal form: vuoto = nessuno.
func idFacoltativo(v, cosa string) (uuid.NullUUID, error) {
	if strings.TrimSpace(v) == "" {
		return uuid.NullUUID{}, nil
	}
	id, err := idDa(v, cosa)
	return uuid.NullUUID{UUID: id, Valid: err == nil}, err
}

func qtaDal(v string) (int32, error) {
	if strings.TrimSpace(v) == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, rifiuto("quantità non valida")
	}
	return int32(n), nil
}

// modificaComponente: POST .../componente/{cid}/modifica, campi `tipo`, `rev`, `descrizione`.
func (s *Server) modificaComponente(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		cid, err := s.componenteDaPercorso(r)
		if err != nil {
			return "", err
		}
		return fascicolo.ModificaComponente(ctx, q, thread, cid, utente, db.TipoComponente(strings.TrimSpace(r.FormValue("tipo"))),
			r.FormValue("rev"), r.FormValue("descrizione"))
	})
}

// collegaComponente: POST .../componente/{cid}/collega, campi `padre` e `qta` — il componente va (anche)
// sotto padre; se ci sta gia', cambia la quantita'.
func (s *Server) collegaComponente(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		cid, err := s.componenteDaPercorso(r)
		if err != nil {
			return "", err
		}
		padre, err := idDa(r.FormValue("padre"), "padre")
		if err != nil {
			return "", err
		}
		qta, err := qtaDal(r.FormValue("qta"))
		if err != nil {
			return "", err
		}
		return fascicolo.Collega(ctx, q, thread, padre, cid, utente, qta)
	})
}

// scollegaComponente: POST .../componente/{cid}/scollega, campo `padre`.
func (s *Server) scollegaComponente(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, _ uuid.UUID) (string, error) {
		cid, err := s.componenteDaPercorso(r)
		if err != nil {
			return "", err
		}
		padre, err := idDa(r.FormValue("padre"), "padre")
		if err != nil {
			return "", err
		}
		return fascicolo.Scollega(ctx, q, thread, padre, cid)
	})
}

// spostaComponente: POST .../componente/{cid}/sposta, campi `da` (il padre di adesso; vuoto = era una
// radice), `a` (il padre nuovo; vuoto = diventa radice) e `qta`.
func (s *Server) spostaComponente(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		cid, err := s.componenteDaPercorso(r)
		if err != nil {
			return "", err
		}
		da, err := idFacoltativo(r.FormValue("da"), "padre di partenza")
		if err != nil {
			return "", err
		}
		a, err := idFacoltativo(r.FormValue("a"), "padre di arrivo")
		if err != nil {
			return "", err
		}
		qta, err := qtaDal(r.FormValue("qta"))
		if err != nil {
			return "", err
		}
		return fascicolo.Sposta(ctx, q, thread, cid, da, a, utente, qta)
	})
}

// archiviaComponente: POST .../componente/{cid}/archivia, campo `motivo`.
func (s *Server) archiviaComponente(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		cid, err := s.componenteDaPercorso(r)
		if err != nil {
			return "", err
		}
		if err := preparaGesto(ctx, q, thread); err != nil {
			return "", err
		}
		msg, err := fascicolo.ArchiviaComponente(ctx, q, thread, cid, utente, r.FormValue("motivo"))
		return dopo(ctx, q, thread, msg, err)
	})
}

// rimuoviComponente: POST .../componente/{cid}/rimuovi. Riesce solo per un componente senza storia;
// altrimenti il rifiuto dice che cosa lo tiene e propone l'archiviazione (A4.9).
func (s *Server) rimuoviComponente(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, _ uuid.UUID) (string, error) {
		cid, err := s.componenteDaPercorso(r)
		if err != nil {
			return "", err
		}
		if err := preparaGesto(ctx, q, thread); err != nil {
			return "", err
		}
		msg, err := fascicolo.RimuoviComponente(ctx, q, thread, cid)
		return dopo(ctx, q, thread, msg, err)
	})
}

// stepStrutturale: POST .../componente/{cid}/step-strutturale, campo `documento`. Lo sceglie una persona
// (A4.4, D31): la schermata presenta gia' scelto l'unico STEP corrente, ma solo questo gesto lo fissa.
func (s *Server) stepStrutturale(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, _ uuid.UUID) (string, error) {
		cid, err := s.componenteDaPercorso(r)
		if err != nil {
			return "", err
		}
		doc, err := idDa(r.FormValue("documento"), "documento")
		if err != nil {
			return "", err
		}
		if err := preparaGesto(ctx, q, thread); err != nil {
			return "", err
		}
		return fascicolo.ScegliStepStrutturale(ctx, q, thread, cid, doc)
	})
}

// concediDeroga: POST .../componente/{cid}/deroga, campi `tipo` e `motivo` (la deroga del fabbisogno).
func (s *Server) concediDeroga(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		cid, err := s.componenteDaPercorso(r)
		if err != nil {
			return "", err
		}
		tipo := db.TipoDocumento(strings.TrimSpace(r.FormValue("tipo")))
		if !tipo.Valid() {
			return "", rifiuto("tipo di documento non valido")
		}
		return fascicolo.ConcediDeroga(ctx, q, thread, cid, utente, tipo, r.FormValue("motivo"))
	})
}

func (s *Server) revocaDeroga(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, _ uuid.UUID) (string, error) {
		did, err := idDa(r.PathValue("did"), "deroga")
		if err != nil {
			return "", err
		}
		return fascicolo.RevocaDeroga(ctx, q, thread, did)
	})
}

// concediDerogaStruttura: POST .../componente/{cid}/deroga-struttura, campo `motivo`: «si congela con
// QUESTO STEP letto in parte» (D33, D36). Vale per lo STEP strutturale e l'analisi di adesso.
func (s *Server) concediDerogaStruttura(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		cid, err := s.componenteDaPercorso(r)
		if err != nil {
			return "", err
		}
		if err := preparaGesto(ctx, q, thread); err != nil {
			return "", err
		}
		d, err := fascicolo.ConcediDerogaStruttura(ctx, q, thread, cid, utente, r.FormValue("motivo"))
		if err != nil {
			return "", err
		}
		c, err := q.GetComponente(ctx, cid)
		if err != nil {
			return "", err
		}
		return c.Codice + ": deroga strutturale concessa per lo STEP letto in parte (" + d.MotivoParziale + "). Vale finché lo STEP e la sua lettura restano questi.", nil
	})
}

func (s *Server) revocaDerogaStruttura(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, _ uuid.UUID) (string, error) {
		did, err := idDa(r.PathValue("did"), "deroga")
		if err != nil {
			return "", err
		}
		if err := preparaGesto(ctx, q, thread); err != nil {
			return "", err
		}
		if err := fascicolo.RevocaDerogaStruttura(ctx, q, thread, did); err != nil {
			return "", err
		}
		return "Deroga strutturale revocata.", nil
	})
}

// ------------------------------------------------------------------ revisioni dei documenti (A4.1)

// sostituisciDocumento: POST .../documento/{did}/sostituisci, campi `vecchio` (il documento che {did}
// sostituisce: un predecessore preciso), `nuovo_riferimento` ("1"/"0", obbligatorio se il vecchio e' lo
// STEP strutturale) e `motivo` (obbligatorio se {did} e' una versione interna).
func (s *Server) sostituisciDocumento(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, _ uuid.UUID) (string, error) {
		nuovo, err := idDa(r.PathValue("did"), "documento")
		if err != nil {
			return "", err
		}
		vecchio, err := idDa(r.FormValue("vecchio"), "documento da sostituire")
		if err != nil {
			return "", err
		}
		sc, err := leggiScelta("sostituisci:"+vecchio.String(), r.FormValue("nuovo_riferimento"), r.FormValue("motivo"))
		if err != nil {
			return "", err
		}
		v, err := q.GetDocumento(ctx, vecchio)
		if err != nil || v.ThreadID != thread {
			return "", rifiuto("il documento da sostituire non è di questa RFQ")
		}
		n, err := q.GetDocumento(ctx, nuovo)
		if err != nil || n.ThreadID != thread {
			return "", rifiuto("il documento non è di questa RFQ")
		}
		if v.ComponenteID.Valid {
			c, err := q.GetComponente(ctx, v.ComponenteID.UUID)
			if err != nil {
				return "", err
			}
			if sc.interno, err = q.DocumentoInterno(ctx, nuovo); err != nil {
				return "", err
			}
			if err := verificaSostituzione(n.NomeFile, c, sc); err != nil {
				return "", err
			}
		}
		if err := preparaGesto(ctx, q, thread); err != nil {
			return "", err
		}
		msg, err := fascicolo.Sostituisci(ctx, q, thread, vecchio, nuovo, sc.riferimento)
		if err != nil {
			return "", err
		}
		return msg, notaSostituzione(ctx, q, vecchio, nuovo, sc.motivo)
	})
}

// annullaSostituzione: POST .../documento/{did}/annulla-sostituzione: {did} torna corrente (solo
// l'ultima sostituzione della catena).
func (s *Server) annullaSostituzione(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, _ uuid.UUID) (string, error) {
		did, err := idDa(r.PathValue("did"), "documento")
		if err != nil {
			return "", err
		}
		if err := preparaGesto(ctx, q, thread); err != nil {
			return "", err
		}
		return fascicolo.AnnullaSostituzione(ctx, q, thread, did)
	})
}

// ------------------------------------------------------------------ versioni della BOM (A4.6, A4.7)

// apriRevisione: POST .../revisione/apri, campi `motivo` e `contesto`. In ACCETTATA e DISTINTA_ERP il
// contesto lo sceglie chi apre, senza default (D25c): senza, si rifiuta. Altrove lo dice la fase, e un
// contesto diverso si rifiuta.
func (s *Server) apriRevisione(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		var scelta *db.ContestoBom
		if v := strings.TrimSpace(r.FormValue("contesto")); v != "" {
			c := db.ContestoBom(v)
			if !c.Valid() {
				return "", rifiuto("tipo di revisione non valido: preventivo oppure tecnica")
			}
			scelta = &c
		}
		v, err := fascicolo.ApriRevisione(ctx, q, thread, utente, scelta, r.FormValue("motivo"))
		if err != nil {
			return "", err
		}
		msg := "Revisione V" + strconv.Itoa(int(v.Numero)) + " aperta (" + string(v.Contesto) + ")"
		if v.Contesto == db.ContestoBomPreventivo {
			msg += ": la RFQ torna in FATTIBILITA, il congelamento la riporta a SCHEDA_COSTO."
		} else {
			msg += ": la fase resta " + string(v.FaseAllApertura) + "."
		}
		return msg + " La BOM working si modifica di nuovo.", nil
	})
}

// abbandonaRevisione: POST .../revisione/abbandona. Solo a differenza vuota (A4.7).
func (s *Server) abbandonaRevisione(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, _ uuid.UUID) (string, error) {
		return fascicolo.AbbandonaBozza(ctx, q, thread)
	})
}

// congela: POST .../congela, campo `motivo`. Gate, versione, istantanee e, per una versione preventivo,
// FATTIBILITA → SCHEDA_COSTO, in una transazione (A4.6).
func (s *Server) congela(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		c, err := fascicolo.CongelaBom(ctx, q, thread, utente, r.FormValue("motivo"))
		if err != nil {
			return "", err
		}
		msg := "BOM congelata: V" + strconv.Itoa(int(c.Versione.Numero)) + " (" + string(c.Versione.Contesto) + "). La fase è " + string(c.Fase) + "."
		if len(c.Avvisi) > 0 {
			msg += " Avvisi: " + strings.Join(c.Avvisi, "; ") + "."
		}
		return msg + " Da qui la working cambia solo aprendo una revisione.", nil
	})
}

// ------------------------------------------------------------------ aiuti

// preparaGesto blocca la RFQ come le altre decisioni: due gesti sulla stessa RFQ si mettono in fila.
func preparaGesto(ctx context.Context, q *db.Queries, thread uuid.UUID) error {
	if _, err := q.BloccaThread(ctx, thread); err != nil {
		return rifiuto("RFQ non trovata")
	}
	return nil
}

// dopo ricalcola le rimozioni dopo un gesto che ha cambiato la working, come dopo una decisione.
func dopo(ctx context.Context, q *db.Queries, thread uuid.UUID, msg string, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if _, err := fascicolo.AggiornaTutteLeRimozioni(ctx, q, thread); err != nil {
		return "", err
	}
	return msg, nil
}
