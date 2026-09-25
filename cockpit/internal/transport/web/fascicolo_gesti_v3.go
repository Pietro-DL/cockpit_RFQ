package web

// I gesti nuovi del Fascicolo v3: le note puntate sui disegni (0021), la struttura confermata in un colpo
// dall'editor della BOM, e un file che non e' di nessun componente (documentazione generale, capitolato).
// Sono gesti come gli altri (s.gesto): una transazione, «Niente è cambiato» se qualcosa non va, e dalla
// schermata del Fascicolo la risposta rifa' i pannelli fuori banda.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// MaxTestoNota e' quanto puo' essere lunga una nota su un disegno: e' anche il CHECK della 0021.
const MaxTestoNota = 2000

// maxStrutturaByte e' il limite del JSON della struttura voluta: cinquemila archi ci stanno larghi.
const maxStrutturaByte = 2 << 20

func (s *Server) registraFascicoloV3(mux *http.ServeMux) {
	mux.HandleFunc("POST /thread/{id}/fascicolo/nota", s.autenticato(s.nuovaNota))
	mux.HandleFunc("POST /thread/{id}/fascicolo/nota/{nid}/modifica", s.autenticato(s.modificaNota))
	mux.HandleFunc("POST /thread/{id}/fascicolo/nota/{nid}/elimina", s.autenticato(s.eliminaNota))
	mux.HandleFunc("POST /thread/{id}/fascicolo/bom/applica", s.autenticato(s.applicaStruttura))
	mux.HandleFunc("POST /thread/{id}/fascicolo/file/{pid}/generale", s.autenticato(s.fileGenerale))
	mux.HandleFunc("GET /thread/{id}/fascicolo/bom/dati", s.autenticato(s.fascicoloDatiEditor))
	mux.HandleFunc("GET /thread/{id}/fascicolo/sezione", s.autenticato(s.fascicoloSezione))
}

// ------------------------------------------------------------------ note sui disegni

// testoNota controlla il testo di una nota: non vuoto, entro MaxTestoNota caratteri.
func testoNota(v string) (string, error) {
	t := strings.TrimSpace(v)
	if t == "" {
		return "", rifiuto("la nota è vuota: scrivi che cosa va guardato in quel punto")
	}
	if utf8.RuneCountInString(t) > MaxTestoNota {
		return "", rifiuto(fmt.Sprintf("la nota ha più di %d caratteri", MaxTestoNota))
	}
	return t, nil
}

// coordinata legge un punto normalizzato: un numero da 0 a 1.
func coordinata(v, cosa string) (float64, error) {
	x, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || math.IsNaN(x) || x < 0 || x > 1 {
		return 0, rifiuto("il punto della nota non è sul foglio (" + cosa + ")")
	}
	return x, nil
}

// nuovaNota: POST .../nota, campi componente, allegato, pagina, x, y, testo. Una nota si scrive anche sulla
// BOM congelata: non e' struttura, e' lavoro di validazione.
func (s *Server) nuovaNota(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		comp, err := idFacoltativo(r.FormValue("componente"), "componente")
		if err != nil {
			return "", err
		}
		all, err := idDa(r.FormValue("allegato"), "file")
		if err != nil {
			return "", err
		}
		pagina, err := strconv.Atoi(strings.TrimSpace(r.FormValue("pagina")))
		if err != nil || pagina < 1 || pagina > 10000 {
			return "", rifiuto("la pagina della nota non è valida")
		}
		x, err := coordinata(r.FormValue("x"), "x")
		if err != nil {
			return "", err
		}
		y, err := coordinata(r.FormValue("y"), "y")
		if err != nil {
			return "", err
		}
		testo, err := testoNota(r.FormValue("testo"))
		if err != nil {
			return "", err
		}
		dove := "senza componente"
		if comp.Valid {
			c, err := q.GetComponente(ctx, comp.UUID)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.ThreadID != thread) {
				return "", rifiuto("il componente non è di questa RFQ")
			}
			if err != nil {
				return "", err
			}
			dove = c.Codice
		}
		del, err := q.AllegatoDelThread(ctx, db.AllegatoDelThreadParams{AllegatoID: all, ThreadID: thread})
		if err != nil {
			return "", err
		}
		if !del {
			return "", rifiuto("il file non è di questa RFQ")
		}
		a, err := q.GetAllegato(ctx, all)
		if err != nil {
			return "", err
		}
		// la nota si mostra per contenuto: un file senza impronta (non ancora sceso) non la porterebbe
		if !a.Sha256.Valid || !strings.EqualFold(strings.TrimPrefix(a.Estensione.String, "."), "pdf") {
			return "", rifiuto("le note si mettono su un PDF già scaricato")
		}
		if _, err := q.InsertAnnotazione(ctx, db.InsertAnnotazioneParams{ThreadID: thread, ComponenteID: comp, AllegatoID: all,
			Pagina: int32(pagina), X: x, Y: y, Testo: testo, CreataDa: utente}); err != nil {
			return "", err
		}
		return fmt.Sprintf("Nota aggiunta su %s, pagina %d (%s).", a.NomeFile, pagina, dove), nil
	})
}

// modificaNota: POST .../nota/{nid}/modifica, campo testo. Solo chi l'ha scritta.
func (s *Server) modificaNota(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		nid, err := idDa(r.PathValue("nid"), "nota")
		if err != nil {
			return "", err
		}
		testo, err := testoNota(r.FormValue("testo"))
		if err != nil {
			return "", err
		}
		n, err := q.ModificaAnnotazione(ctx, db.ModificaAnnotazioneParams{Testo: testo, UtenteID: utente, AnnotazioneID: nid, ThreadID: thread})
		if err != nil {
			return "", err
		}
		if n != 1 {
			return "", rifiuto("la nota non c'è più, oppure non l'hai scritta tu: la cambia solo chi l'ha scritta")
		}
		return "Nota cambiata.", nil
	})
}

// eliminaNota: POST .../nota/{nid}/elimina. Solo chi l'ha scritta.
func (s *Server) eliminaNota(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		nid, err := idDa(r.PathValue("nid"), "nota")
		if err != nil {
			return "", err
		}
		n, err := q.DeleteAnnotazione(ctx, db.DeleteAnnotazioneParams{AnnotazioneID: nid, ThreadID: thread, UtenteID: utente})
		if err != nil {
			return "", err
		}
		if n != 1 {
			return "", rifiuto("la nota non c'è più, oppure non l'hai scritta tu: la toglie solo chi l'ha scritta")
		}
		return "Nota tolta.", nil
	})
}

// ------------------------------------------------------------------ la struttura dall'editor

// applicaStruttura: POST .../bom/applica, campo struttura (JSON, fascicolo.StrutturaVoluta). Tutto o niente;
// se riesce la risposta porta l'evento bom-applicata, e l'editor si chiude.
func (s *Server) applicaStruttura(w http.ResponseWriter, r *http.Request) {
	s.gestoPoi(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		raw := r.FormValue("struttura")
		if len(raw) > maxStrutturaByte {
			return "", rifiuto("la struttura mandata è troppo grande")
		}
		var v fascicolo.StrutturaVoluta
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return "", rifiuto("la struttura mandata non si legge: riapri l'editor")
		}
		return fascicolo.ApplicaStrutturaVoluta(ctx, q, thread, utente, v)
	}, func(w http.ResponseWriter, ok bool, testo string) {
		// l'editor sta davanti alla pagina: l'esito glielo si dice con un evento, sia che la struttura sia
		// entrata sia che sia stata rifiutata (in quel caso l'editor resta aperto con le modifiche)
		w.Header().Set("HX-Trigger", eventoAscii("bom-esito", map[string]any{"ok": ok, "testo": testo}))
	})
}

// eventoAscii scrive un'intestazione HX-Trigger con un evento e i suoi dati, tutta in ASCII: le intestazioni
// HTTP il browser le legge come latin-1, e una «è» arriverebbe rotta. Il JSON dice le lettere accentate
// con \uXXXX, che htmx rilegge giuste.
func eventoAscii(nome string, dati any) string {
	b, err := json.Marshal(map[string]any{nome: dati})
	if err != nil {
		return nome
	}
	var out strings.Builder
	for _, r := range string(b) {
		switch {
		case r < 0x80:
			out.WriteRune(r)
		case r <= 0xFFFF:
			fmt.Fprintf(&out, `\u%04x`, r)
		default:
			r -= 0x10000
			fmt.Fprintf(&out, `\u%04x\u%04x`, 0xD800+(r>>10), 0xDC00+(r&0x3FF))
		}
	}
	return out.String()
}

// ------------------------------------------------------------------ un file di nessun componente

// fileGenerale: POST .../file/{pid}/generale, campo tipo (altro = documentazione generale, capitolato). Il
// file entra nel fascicolo come documento della RFQ, senza componente: se la proposta era agganciata a un
// componente la si sgancia prima, nella stessa transazione.
func (s *Server) fileGenerale(w http.ResponseWriter, r *http.Request) {
	s.gesto(w, r, func(ctx context.Context, q *db.Queries, thread, utente uuid.UUID) (string, error) {
		pid, err := idDa(r.PathValue("pid"), "file")
		if err != nil {
			return "", err
		}
		tipo := db.TipoDocumento(r.FormValue("tipo"))
		if tipo != db.TipoDocumentoAltro && tipo != db.TipoDocumentoCapitolato {
			return "", rifiuto("un file senza componente entra come documentazione generale o come capitolato")
		}
		p, err := q.BloccaProposta(ctx, pid)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!p.ThreadID.Valid || p.ThreadID.UUID != thread)) {
			return "", rifiuto("il file non è di questa RFQ")
		}
		if err != nil {
			return "", err
		}
		if p.Stato != db.StatoPropostaAperta {
			return "", rifiuto("il file è già stato deciso")
		}
		if p.ComponenteID.Valid {
			if _, err := q.SetComponenteProposta(ctx, db.SetComponentePropostaParams{PropostaID: pid, Codice: p.Codice}); err != nil {
				return "", err
			}
			if p, err = q.BloccaProposta(ctx, pid); err != nil {
				return "", err
			}
		}
		a, err := q.GetAllegato(ctx, p.AllegatoID)
		if err != nil {
			return "", err
		}
		m, err := q.GetMessaggio(ctx, a.MessaggioID)
		if err != nil {
			return "", err
		}
		return s.confermaProposta(ctx, q, utenteDa(ctx), p, a, m, tipo, "", "", "", uuid.NullUUID{}, false, sceltaRevisione{})
	})
}
