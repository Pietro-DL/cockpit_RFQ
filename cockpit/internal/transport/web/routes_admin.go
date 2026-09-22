// Le schermate tecniche: la coda dei job e gli scarti dell'ingest.
//
// Le altre pagine di amministrazione hanno il loro file (integrita_admin.go, anagrafica.go e
// anagrafica_admin.go, convenzioni_admin.go, fornitori_admin.go, postazioni_admin.go): qui c'e'
// quello che guarda il MOTORE — che cosa sta facendo la coda e che cosa non e' entrato.

package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

// registraAdmin monta le schermate tecniche. Sono dell'amministratore: il wrapper `soloAdmin` sta
// qui e non dentro i gestori, perche' il posto in cui si montano le rotte e' l'unico da cui si vede
// che nessuna e' rimasta scoperta.
func (s *Server) registraAdmin(mux *http.ServeMux) {
	// Le schermate tecniche sono dell'amministratore (voce 6.9): `soloAdmin` è `autenticato` più il
	// ruolo, e sta QUI e non dentro i gestori perché il posto in cui si montano le rotte è l'unico da
	// cui si vede che nessuna è rimasta scoperta. Un gestore che si protegge da solo è un gestore che
	// il prossimo verrà scritto senza.
	mux.HandleFunc("GET /admin/job", s.soloAdmin(s.adminJob))
	mux.HandleFunc("POST /admin/job/{id}/riaccoda", s.soloAdmin(s.riaccodaJob))
	mux.HandleFunc("POST /admin/job/{id}/annulla", s.soloAdmin(s.annullaJob))
	// Integrita' NAS (blocco 5B): il confronto fra quello che il database promette e i file veri.
	mux.HandleFunc("GET /admin/nas", s.soloAdmin(s.adminIntegrita))
	mux.HandleFunc("POST /admin/nas/controlla", s.soloAdmin(s.controllaIntegrita))
	mux.HandleFunc("POST /admin/nas/{id}/riaccoda", s.soloAdmin(s.riaccodaDocumento))
	mux.HandleFunc("POST /admin/nas/{id}/allinea", s.soloAdmin(s.allineaDocumento))
	mux.HandleFunc("GET /admin/scarti", s.soloAdmin(s.adminScarti))
	mux.HandleFunc("POST /admin/scarti/{id}/riprova", s.soloAdmin(s.riprovaScarto))
	// Anagrafica e' amministrativa (D29): le regole di riconoscimento di un cliente valgono per
	// la posta di TUTTI, non per una richiesta. Stesso wrapper delle altre, stesso 403.
	mux.HandleFunc("GET /admin/anagrafica", s.soloAdmin(s.adminAnagrafica))
	mux.HandleFunc("GET /admin/anagrafica/articoli", s.soloAdmin(s.adminAnagraficaArticoli))
	mux.HandleFunc("POST /admin/anagrafica", s.soloAdmin(s.nuovoCliente))
	mux.HandleFunc("POST /admin/anagrafica/{id}", s.soloAdmin(s.salvaCliente))
	mux.HandleFunc("POST /admin/anagrafica/{id}/regole", s.soloAdmin(s.salvaRegole))
	// checkpoint 3R §7: il form strutturato e le due entita' che erano visibili ma non modificabili
	mux.HandleFunc("POST /admin/anagrafica/{id}/regole/form", s.soloAdmin(s.salvaRegoleDalForm))
	mux.HandleFunc("POST /admin/anagrafica/{id}/buyer", s.soloAdmin(s.nuovoBuyerCliente))
	mux.HandleFunc("POST /admin/anagrafica/{id}/buyer/elimina", s.soloAdmin(s.eliminaBuyerCliente))
	mux.HandleFunc("POST /admin/anagrafica/{id}/fabbisogno", s.soloAdmin(s.aggiungiFabbisogno))
	mux.HandleFunc("POST /admin/anagrafica/{id}/fabbisogno/elimina", s.soloAdmin(s.eliminaFabbisogno))
	mux.HandleFunc("POST /admin/anagrafica/{id}/dominio", s.soloAdmin(s.aggiungiDominioCliente))
	mux.HandleFunc("POST /admin/anagrafica/{id}/dominio/elimina", s.soloAdmin(s.eliminaDominioCliente))
	mux.HandleFunc("POST /admin/anagrafica/{id}/prova", s.soloAdmin(s.bancoProva))
	// blocco 7A (D39): convenzioni di codice e fornitori qualificati del cliente
	mux.HandleFunc("POST /admin/anagrafica/{id}/convenzione", s.soloAdmin(s.nuovaConvenzione))
	mux.HandleFunc("POST /admin/anagrafica/{id}/convenzione/elimina", s.soloAdmin(s.eliminaConvenzione))
	mux.HandleFunc("POST /admin/anagrafica/{id}/convenzione/attiva", s.soloAdmin(s.attivaConvenzione))
	mux.HandleFunc("POST /admin/anagrafica/{id}/qualifica", s.soloAdmin(s.qualificaFornitore))
	mux.HandleFunc("POST /admin/anagrafica/{id}/qualifica/elimina", s.soloAdmin(s.eliminaQualificaCliente))
	mux.HandleFunc("POST /admin/anagrafica/{id}/lavorazioni/prova", s.soloAdmin(s.provaCodiceCliente))
	// blocco 7A.5: Anagrafica › Fornitori, e l'import del seme con anteprima
	mux.HandleFunc("GET /admin/fornitori", s.soloAdmin(s.adminFornitori))
	mux.HandleFunc("POST /admin/fornitori", s.soloAdmin(s.nuovoFornitore))
	mux.HandleFunc("GET /admin/fornitori/importa", s.soloAdmin(s.importaFornitoriForm))
	mux.HandleFunc("POST /admin/fornitori/importa", s.soloAdmin(s.importaFornitori))
	mux.HandleFunc("POST /admin/fornitori/{id}", s.soloAdmin(s.salvaFornitore))
	mux.HandleFunc("POST /admin/fornitori/{id}/dominio", s.soloAdmin(s.aggiungiDominioFornitore))
	mux.HandleFunc("POST /admin/fornitori/{id}/dominio/elimina", s.soloAdmin(s.eliminaDominioFornitore))
	mux.HandleFunc("POST /admin/fornitori/{id}/contatto", s.soloAdmin(s.nuovoContattoFornitore))
	mux.HandleFunc("POST /admin/fornitori/{id}/contatto/elimina", s.soloAdmin(s.eliminaContattoFornitore))
	mux.HandleFunc("POST /admin/fornitori/{id}/lavorazioni", s.soloAdmin(s.salvaLavorazioniFornitore))
	mux.HandleFunc("POST /admin/fornitori/{id}/qualifica", s.soloAdmin(s.qualificaDalFornitore))
	mux.HandleFunc("POST /admin/fornitori/{id}/qualifica/elimina", s.soloAdmin(s.eliminaQualificaDalFornitore))
}

type jobDati struct {
	Conta []db.ContaJobPerStatoRow
	Job   []db.Job
	Stato string
}

func (s *Server) adminJob(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.Pool)
	var stato db.NullStatoJob
	if st := r.URL.Query().Get("stato"); st != "" && db.StatoJob(st).Valid() {
		stato = db.NullStatoJob{StatoJob: db.StatoJob(st), Valid: true}
	}
	lista, err := q.ListJob(r.Context(), db.ListJobParams{Stato: stato, Limit: 100})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	conta, _ := q.ContaJobPerStato(r.Context())
	s.rendi(w, r, "job.html", "job_tabella", "Coda job", jobDati{Conta: conta, Job: lista, Stato: r.URL.Query().Get("stato")})
}

func (s *Server) riaccodaJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "id", 400)
		return
	}
	_, err = db.New(s.Pool).RiaccodaJob(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		// C'è già un job pendente con la stessa chiave di idempotenza (o il job non è chiuso).
		// Riaccodarlo violerebbe l'indice unico parziale: prima era un 500, adesso è una frase.
		s.avviso(w, "non riaccodato: ce n'è già uno in coda con la stessa chiave")
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

// annullaJob è «Annulla» (Q17): un job pronto o in corso che l'operatore non vuole più. Se è in
// corso il tentativo lo scopre al prossimo heartbeat (409) e si ferma.
func (s *Server) annullaJob(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "id", 400)
		return
	}
	_, err = db.New(s.Pool).AnnullaJob(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		s.avviso(w, "non annullato: il job è già chiuso")
		return
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.Log.Info("job annullato dall'operatore", "job", id, "utente", utenteDa(r.Context()).Sigla)
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

type scartiDati struct {
	Conta   []db.ContaIngestScartiPerOrigineRow
	Scarti  []db.IngestScarto
	Origine string
}

// adminScarti mostra gli elementi che l'ingest ha rifiutato. Le due origini sono due sezioni diverse
// perché si riprovano in due modi diversi: dal payload in database, o rileggendo da Outlook.
func (s *Server) adminScarti(w http.ResponseWriter, r *http.Request) {
	q := db.New(s.Pool)
	var origine pgtype.Text
	if o := r.URL.Query().Get("origine"); o == "ingest" || o == "lettura" {
		origine = pgtype.Text{String: o, Valid: true}
	}
	lista, err := q.ListIngestScarti(r.Context(), db.ListIngestScartiParams{Origine: origine, Limit: 200})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	conta, _ := q.ContaIngestScartiPerOrigine(r.Context())
	s.rendi(w, r, "scarti.html", "scarti_tabella", "Scarti", scartiDati{Conta: conta, Scarti: lista, Origine: r.URL.Query().Get("origine")})
}

func (s *Server) riprovaScarto(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "id", 400)
		return
	}
	if s.Ingest == nil {
		http.Error(w, "servizio di ingest non disponibile", 500)
		return
	}
	esito, err := s.Ingest.Riprova(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		s.avviso(w, "scarto non trovato: forse è già stato ripreso")
		return
	}
	if err != nil {
		s.Log.Error("riprova scarto", "scarto", id, "err", err)
		s.avviso(w, "non riuscita: "+err.Error())
		return
	}
	s.Log.Info("scarto ripreso", "scarto", id, "esito", esito)
	s.avviso(w, esito)
}
