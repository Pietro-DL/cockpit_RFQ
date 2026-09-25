// Package workerapi espone le rotte usate dai worker Python: coda job, ingest, upload degli allegati.
//
// Autenticazione (voce 2.4): header X-Cockpit-Token con il token INDIVIDUALE del worker. Il server ne
// cerca lo sha256 in `worker_credenziale` e da lì ricava nome, tipo, postazione e caselle: il token
// non autorizza soltanto, identifica. Il token condiviso di prima non autentica piu' niente.
package workerapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/inbox/ingest"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/rete"
	"promatec/cockpit/internal/platform/storage/nas"
	"promatec/cockpit/internal/platform/storage/staging"
)

type Server struct {
	Pool *pgxpool.Pool
	Log  *slog.Logger
	// Nessun campo Token: dalla voce 2.4 il server non ha un segreto suo da confrontare. Le
	// credenziali stanno in `worker_credenziale`, una per worker, e ognuna dice anche chi è.
	Ingest  *ingest.Servizio
	Staging string
	// CasellaDefault: indirizzo attribuito ai lotti che non dichiarano una casella. Serve finché il
	// worker non manda sempre casella_id (fase 2); una casella sbagliata qui è meglio di una assente,
	// perché almeno è dichiarata e verificabile.
	CasellaDefault string
	// Analizzatore: versione e configurazione con cui si chiedono le analisi (voce 1.12).
	Analizzatore coda.Analizzatore
	// MaxUpload: byte massimi di un singolo file caricato con PUT /api/v1/allegati/{id}/file
	// (voce 2.3). Zero = 64 MB.
	MaxUpload int64
	// IndirizzoClient dice da quale IP arriva una richiesta: finisce in worker_presenza.indirizzo_ip
	// ed è ciò su cui la voce 2.7 abbina una sessione UI a una postazione. Nil = RemoteAddr. Un
	// proxy davanti al server non c'è e non deve esserci (P7.1): X-Forwarded-For NON viene letto in
	// produzione, perché chiunque potrebbe scriverci dentro l'IP di un'altra postazione. I test lo
	// sostituiscono per simulare due PC.
	IndirizzoClient func(*http.Request) string
}

func (s *Server) Registra(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/jobs/claim", s.auth(s.claim))
	mux.HandleFunc("GET /api/v1/worker/caselle", s.auth(s.caselleWorker))
	mux.HandleFunc("POST /api/v1/jobs/{id}/heartbeat", s.auth(s.heartbeat))
	mux.HandleFunc("POST /api/v1/jobs/{id}/result", s.auth(s.result))
	mux.HandleFunc("POST /api/v1/ingest/messaggi", s.auth(s.ingest))
	mux.HandleFunc("PUT /api/v1/allegati/{id}/file", s.auth(s.caricaFile))
	mux.HandleFunc("GET /api/v1/allegati/{id}/contenuto", s.auth(s.scaricaContenuto))
	mux.HandleFunc("GET /api/v1/sync/cursori", s.auth(s.cursori))
}

type chiaveCtx int

const ctxCredenziale chiaveCtx = 1

// ImprontaToken è in `rete`: la calcolano anche le fondazioni quando seminano la credenziale, e due
// copie che divergessero su uno spazio in fondo darebbero un 401 che nessuno spiega.
func ImprontaToken(token string) string { return rete.ImprontaToken(token) }

// credenzialeDa restituisce il worker autenticato da `auth`. Le rotte sotto `auth` ce l'hanno sempre.
func credenzialeDa(ctx context.Context) (db.WorkerCredenziale, bool) {
	c, ok := ctx.Value(ctxCredenziale).(db.WorkerCredenziale)
	return c, ok
}

// auth riconosce CHI sta chiamando (voce 2.4).
//
// Fino al blocco 1 c'era un token solo, uguale per tutti i worker e scritto in chiaro in ogni
// worker.toml: autenticava «un worker», non «questo worker». L'identità la dichiarava poi il JSON —
// `worker_id` — e da lì venivano postazione e caselle autorizzate. Chiunque avesse il token, cioè
// chiunque avesse letto un worker.toml, poteva presentarsi come il worker di un altro PC e farsi
// assegnare i suoi job interattivi e la sua posta.
//
// Ora il token È l'identità: lo si cerca per sha256 in `worker_credenziale` e la riga trovata dice
// nome, tipo, postazione e caselle. Il `worker_id` del JSON resta nel contratto ma non decide più
// niente: deve solo coincidere, e se non coincide è 403 — un worker.toml con il nome di un altro è un
// file copiato, e va detto invece che assecondato.
func (s *Server) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := strings.TrimSpace(r.Header.Get("X-Cockpit-Token"))
		if t == "" {
			http.Error(w, `{"errore":"credenziale mancante: header X-Cockpit-Token"}`, http.StatusUnauthorized)
			return
		}
		cred, err := db.New(s.Pool).CredenzialiPerToken(r.Context(), ImprontaToken(t))
		if err != nil {
			errore(w, 500, err)
			return
		}
		switch len(cred) {
		case 1:
			// Una credenziale censita ma MAI generata non fa entrare nessuno, ed è irraggiungibile per
			// costruzione: l'impronta del segnaposto è quella della stringa vuota, e un header vuoto è
			// già stato scartato qui sopra. Il caso resta scritto perché è ciò che rende il segnaposto
			// sicuro anche se un domani questa funzione cambia forma.
			if cred[0].TokenHash == rete.ImprontaNonGenerata {
				s.Log.Warn("credenziale senza segreto", "worker", cred[0].WorkerNome)
				http.Error(w, `{"errore":"la credenziale di questo worker esiste ma non ha ancora un segreto: generare il pacchetto dalla pagina Postazioni"}`,
					http.StatusUnauthorized)
				return
			}
			// Prova di vita (0009), qui e non dentro le rotte. `claim` resta appeso in long-poll
			// fino ad worker.AttesaClaim: una presenza scritta DOPO racconta il momento in cui il
			// worker ha smesso di aspettare, non quello in cui si e' fatto vivo, e per tutta
			// l'attesa — o per tutta la durata di un job — la testata lo dava per spento. Questo
			// e' l'unico punto attraversato da tutte le rotte dei worker: nessuna rotta futura da
			// ricordarsi di istruire.
			s.contatto(r, cred[0])
			h(w, r.WithContext(context.WithValue(r.Context(), ctxCredenziale, cred[0])))
		case 0:
			s.Log.Warn("credenziale non riconosciuta", "ip", r.RemoteAddr, "percorso", r.URL.Path)
			http.Error(w, `{"errore":"credenziale non riconosciuta: dalla voce 2.4 ogni worker ha il suo token ([[worker]].token in cockpit.toml). Il token condiviso [server].token_worker non autentica più"}`,
				http.StatusUnauthorized)
		default:
			// Due credenziali con lo stesso segreto non sono credenziali individuali: assegnare
			// l'identità alla prima riga in ordine alfabetico sarebbe peggio che rifiutare, perché
			// funzionerebbe quasi sempre e sbaglierebbe senza dirlo.
			nomi := make([]string, 0, len(cred))
			for _, c := range cred {
				nomi = append(nomi, c.WorkerNome)
			}
			s.Log.Error("token condiviso fra più worker", "worker", nomi)
			http.Error(w, fmt.Sprintf(`{"errore":"questo token è di piu' worker (%s): non identifica nessuno. Dare a ciascun [[worker]] di cockpit.toml un token diverso e riportarlo nel worker.toml del suo PC"}`,
				strings.Join(nomi, ", ")), http.StatusUnauthorized)
		}
	}
}

// contatto registra che questo worker si e' appena fatto riconoscere: e' la sola cosa su cui si
// decide online/offline (0009). Non crea la riga — un claim rifiutato non deve far comparire in
// testata un worker che non servira' niente — e non tocca IP ne' postazione, che il claim scrive solo
// dopo aver verificato che il worker sia dove dice di essere.
//
// Non fallisce mai la richiesta: se la presenza non si scrive il worker deve poter lavorare lo stesso.
// La testata lo mostrerebbe spento, che e' un difetto di ciò che si vede, non una ragione per
// smettere di sincronizzare la posta.
func (s *Server) contatto(r *http.Request, cred db.WorkerCredenziale) {
	if err := db.New(s.Pool).ContattoWorker(r.Context(), cred.WorkerNome); err != nil {
		s.Log.Warn("presenza: contatto non registrato", "worker", cred.WorkerNome, "err", err)
	}
}

// stessoWorker verifica che il nome dichiarato nel JSON sia quello della credenziale che ha aperto la
// richiesta. Scrive la risposta e restituisce false quando non lo è.
func stessoWorker(w http.ResponseWriter, r *http.Request, dichiarato string) (db.WorkerCredenziale, bool) {
	cred, ok := credenzialeDa(r.Context())
	if !ok {
		errore(w, 500, errors.New("rotta senza credenziale: difetto di registrazione delle rotte"))
		return cred, false
	}
	if d := strings.TrimSpace(dichiarato); d != "" && !strings.EqualFold(d, cred.WorkerNome) {
		errore(w, 403, fmt.Errorf("questa credenziale è di %q, ma la richiesta dichiara %q: worker.toml con il nome di un altro worker",
			cred.WorkerNome, d))
		return cred, false
	}
	return cred, true
}

func scriviJSON(w http.ResponseWriter, stato int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(stato)
	_ = json.NewEncoder(w).Encode(v)
}

func errore(w http.ResponseWriter, stato int, err error) {
	scriviJSON(w, stato, map[string]string{"errore": err.Error()})
}

func leggi(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64<<20))
	return dec.Decode(v)
}

// ---------------------------------------------------------------- coda

// indirizzoDi è l'IP del chiamante, senza porta; nil se non è un IP.
func (s *Server) indirizzoDi(r *http.Request) *netip.Addr {
	grezzo := r.RemoteAddr
	if s.IndirizzoClient != nil {
		grezzo = s.IndirizzoClient(r)
	}
	if h, _, err := net.SplitHostPort(grezzo); err == nil {
		grezzo = h
	}
	a, err := netip.ParseAddr(strings.TrimSpace(grezzo))
	if err != nil {
		return nil
	}
	a = a.Unmap()
	return &a
}

// destinazione è ciò che il claim decide su un worker PRIMA di cercare un job: la credenziale, la
// postazione (dalla credenziale, mai dal JSON), le caselle che serve davvero e l'avviso da mostrare
// quando dichiara più di quanto gli è permesso.
type destinazione struct {
	cred     db.WorkerCredenziale
	dest     coda.Destinazione
	aperte   []worker.CasellaAperta // dichiarate E autorizzate: finiscono in casella_store
	ignorate []uuid.UUID            // dichiarate e NON autorizzate: avviso (Q18)
	avviso   string
}

// risolviDestinazione interseca ciò che il worker dichiara con ciò che la credenziale autorizza
// (voce 2.2). Il server non si fida del JSON: una casella dichiarata e non autorizzata non entra
// nel claim, viene scritta nell'avviso della presenza e nel log, e il worker continua a lavorare
// sulle altre. Un worker senza credenziale non ha una destinazione e non prende niente.
func risolviDestinazione(cred db.WorkerCredenziale, req worker.ClaimRichiesta) destinazione {
	d := destinazione{cred: cred, dest: coda.Destinazione{Postazione: cred.PostazioneID}}
	autorizzate := map[uuid.UUID]bool{}
	for _, c := range cred.Caselle {
		autorizzate[c] = true
	}
	viste := map[uuid.UUID]bool{}
	for _, c := range req.CaselleAperte {
		if viste[c.CasellaID] {
			continue
		}
		viste[c.CasellaID] = true
		if !autorizzate[c.CasellaID] {
			d.ignorate = append(d.ignorate, c.CasellaID)
			continue
		}
		d.dest.Caselle = append(d.dest.Caselle, c.CasellaID)
		d.aperte = append(d.aperte, c)
	}
	if len(d.ignorate) > 0 {
		ids := make([]string, 0, len(d.ignorate))
		for _, c := range d.ignorate {
			ids = append(ids, c.String()[:8])
		}
		d.avviso = fmt.Sprintf("%d caselle dichiarate ma non autorizzate per questo worker (%s): ignorate", len(d.ignorate), strings.Join(ids, ", "))
	}
	return d
}

func (s *Server) claim(w http.ResponseWriter, r *http.Request) {
	var req worker.ClaimRichiesta
	if err := leggi(r, &req); err != nil {
		errore(w, 400, err)
		return
	}
	wt := db.WorkerTipo(req.Worker)
	if !wt.Valid() || wt == db.WorkerTipoServer {
		errore(w, 400, fmt.Errorf("worker non valido: %q", req.Worker))
		return
	}
	if strings.TrimSpace(req.WorkerID) == "" {
		errore(w, 400, errors.New("worker_id mancante"))
		return
	}
	q := db.New(s.Pool)
	// La credenziale decide che cosa il worker può fare, e dalla voce 2.4 non la cerca più il nome
	// dichiarato nel JSON: è quella con cui la richiesta si è autenticata. Il nome dichiarato deve
	// solo coincidere — un worker.toml con il nome di un altro è un file copiato.
	cred, ok := stessoWorker(w, r, req.WorkerID)
	if !ok {
		return
	}
	if cred.WorkerTipo != wt {
		errore(w, 403, fmt.Errorf("worker %q è censito come %s, non %s", req.WorkerID, cred.WorkerTipo, wt))
		return
	}
	// Un worker.toml copiato su un altro PC farebbe eseguire lì i job interattivi destinati alla
	// postazione della credenziale: la finestra si aprirebbe sul PC sbagliato, senza errori.
	if req.Postazione != "" && cred.PostazioneID.Valid {
		if p, err := q.GetPostazione(r.Context(), cred.PostazioneID.UUID); err == nil && !strings.EqualFold(p.NomeHost, req.Postazione) {
			errore(w, 403, fmt.Errorf("worker %q gira su %s ma la sua credenziale è della postazione %s: worker.toml copiato su un altro PC?", req.WorkerID, strings.ToUpper(req.Postazione), p.NomeHost))
			return
		}
	}
	d := risolviDestinazione(cred, req)
	if d.avviso != "" {
		s.Log.Warn("claim con caselle non autorizzate", "worker", req.WorkerID, "ignorate", d.ignorate)
	}
	// Lo store locale di ogni casella servita si registra PRIMA di cercare un job: è così che un'altra
	// postazione — o questa, dopo un cambio di profilo — trova come aprire la casella (voce 2.6, M1).
	if cred.PostazioneID.Valid {
		for _, c := range d.aperte {
			if c.StoreID == "" {
				continue
			}
			if n, err := q.UpsertCasellaStore(r.Context(), db.UpsertCasellaStoreParams{PostazioneID: cred.PostazioneID.UUID, CasellaID: c.CasellaID, StoreID: c.StoreID}); err != nil {
				s.Log.Warn("casella_store", "worker", req.WorkerID, "casella", c.CasellaID, "err", err)
			} else if n > 0 {
				s.Log.Info("store locale registrato", "worker", req.WorkerID, "casella", c.CasellaID, "store", fmt.Sprintf("%.24s…", c.StoreID))
			}
		}
	}

	// Che cosa questo worker e' e che cosa serve: si scrive ADESSO, prima di mettersi ad aspettare
	// (0009). Tutto cio' che serve e' gia' noto — postazione dalla credenziale, caselle intersecate,
	// Outlook raggiungibile, avviso — e registrarlo dopo il long-poll voleva dire che per venti
	// secondi la testata raccontava di un worker «attivo ma che non trova questa casella nel proprio
	// profilo»: falso, e allarmante per niente. E' anche cio' che la voce 2.7 usa per abbinare la
	// sessione alla postazione.
	if err := q.DichiarazioneWorker(r.Context(), db.DichiarazioneWorkerParams{
		WorkerNome: req.WorkerID, WorkerTipo: wt, PostazioneID: cred.PostazioneID, IndirizzoIp: s.indirizzoDi(r),
		OutlookOk: req.OutlookOk || wt != db.WorkerTipoOutlook, CaselleAperte: nonNil(d.dest.Caselle),
		UltimoArresto: txt(req.UltimoArresto), Avviso: txt(d.avviso),
	}); err != nil {
		s.Log.Warn("worker_presenza: dichiarazione", "worker", req.WorkerID, "err", err)
	}

	attesa := time.Duration(req.AttesaS) * time.Second
	if attesa <= 0 || attesa > worker.AttesaClaimMax {
		attesa = worker.AttesaClaim
	}
	ctx, cancel := context.WithTimeout(r.Context(), attesa+5*time.Second)
	defer cancel()
	j, err := coda.Claim(ctx, q, wt, req.WorkerID, d.dest, attesa)
	if err != nil {
		errore(w, 500, err)
		return
	}
	// La fine del claim. Qui dentro non c'è più niente che riguardi l'essere vivi: quello è scritto
	// due volte da prima che l'attesa cominciasse. Resta ultimo_claim, che serve a chi si chiede da
	// quanto questo worker non riceve lavoro.
	if err := q.ClaimConcluso(r.Context(), db.ClaimConclusoParams{WorkerNome: req.WorkerID, ConJob: j != nil}); err != nil {
		s.Log.Warn("worker_presenza: fine del claim", "worker", req.WorkerID, "err", err)
	}
	if j == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	scriviJSON(w, 200, coda.InJob(j))
}

func nonNil(u []uuid.UUID) []uuid.UUID {
	if u == nil {
		return []uuid.UUID{}
	}
	return u
}

// caselleWorker dice a un worker QUALI caselle deve risolvere nel proprio profilo Outlook: quelle
// attive su cui la sua credenziale è autorizzata (voce 2.6, M1). Il worker parte da questo elenco e
// non dal profilo: uno store che c'è nel profilo e non c'è qui — la casella di un collega, un archivio
// — non viene aperto, letto né censito. È il motivo per cui il terzo store del profilo di prova
// (non censito) deve restare invisibile al Cockpit.
func (s *Server) caselleWorker(w http.ResponseWriter, r *http.Request) {
	cred, ok := stessoWorker(w, r, r.URL.Query().Get("worker_id"))
	if !ok {
		return
	}
	nome := cred.WorkerNome
	q := db.New(s.Pool)
	righe, err := q.ListCaselleAutorizzate(r.Context(), nome)
	if err != nil {
		errore(w, 500, err)
		return
	}
	out := make([]worker.CasellaServita, 0, len(righe))
	for _, c := range righe {
		if c.Canale != db.CanaleOutlook {
			continue
		}
		out = append(out, worker.CasellaServita{CasellaID: c.CasellaID, Indirizzo: c.Indirizzo, Nome: c.Nome, Condivisa: c.Condivisa})
	}
	scriviJSON(w, 200, out)
}

// tentativo estrae dalla richiesta l'identità del tentativo. Senza token non si prosegue: accettare
// una scrittura «di qualcuno che dice di essere il worker» vanificherebbe tutto il resto.
func tentativo(jobID int64, workerID, token string) (coda.Tentativo, error) {
	if workerID == "" {
		return coda.Tentativo{}, errors.New("worker_id mancante")
	}
	t, err := uuid.Parse(strings.TrimSpace(token))
	if err != nil {
		return coda.Tentativo{}, fmt.Errorf("lease_token mancante o non valido: %w", err)
	}
	return coda.Tentativo{JobID: jobID, LeaseToken: t, WorkerID: workerID}, nil
}

// nonValido risponde 409 e, se il tentativo è caduto per durata massima, riporta il job a 'pronto'
// con il motivo scritto: altrimenti resterebbe «in corso» senza che nessuno lo stia eseguendo.
func (s *Server) nonValido(w http.ResponseWriter, ctx context.Context, jobID int64) {
	if coda.ChiudiSeDurataSuperata(ctx, db.New(s.Pool), jobID) {
		s.Log.Warn("tentativo oltre la durata massima: job riaccodato", "job", jobID)
		errore(w, 409, errors.New("durata massima superata: il tentativo non vale più"))
		return
	}
	errore(w, 409, errors.New("tentativo non più valido (lease perso o job ripreso da un altro tentativo)"))
}

func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		errore(w, 400, err)
		return
	}
	var req worker.HeartbeatRichiesta
	_ = leggi(r, &req)
	if _, ok := stessoWorker(w, r, req.WorkerID); !ok {
		return
	}
	t, err := tentativo(id, req.WorkerID, req.LeaseToken)
	if err != nil {
		s.Log.Warn("battito senza tentativo dichiarato: il lease non viene rinnovato",
			"job", id, "worker_id", req.WorkerID, "err", err)
		errore(w, 400, err)
		return
	}
	switch err := coda.Batte(r.Context(), db.New(s.Pool), t); {
	case errors.Is(err, coda.ErrTentativoNonValido):
		s.nonValido(w, r.Context(), id)
	case err != nil:
		errore(w, 500, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) result(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		errore(w, 400, err)
		return
	}
	var req worker.RisultatoRichiesta
	if err := leggi(r, &req); err != nil {
		errore(w, 400, err)
		return
	}
	if _, ok := stessoWorker(w, r, req.WorkerID); !ok {
		return
	}
	t, err := tentativo(id, req.WorkerID, req.LeaseToken)
	if err != nil {
		// Va detto forte, perché il worker ha già fatto il lavoro e nessuno lo saprà: il job resta
		// in corso finché il lease non scade, poi torna in coda e viene rifatto da capo. È il
		// difetto del 15/09/2026, e in un log di sole righe del worker era invisibile da questa
		// parte.
		s.Log.Warn("result rifiutato: il tentativo non è dichiarato, il lavoro andrà perso e il job sarà ripetuto",
			"job", id, "worker_id", req.WorkerID, "esito", req.Esito, "err", err)
		errore(w, 400, err)
		return
	}
	ctx := r.Context()
	// Il job si legge PRIMA di aprire la transazione, perché l'eventuale estrazione di uno zip va
	// fatta fuori (voce 1.4). La lettura non ha bisogno di essere nella stessa transazione delle
	// scritture: ciò che protegge il job è il predicato del tentativo, verificato al momento di
	// scrivere, non l'istante in cui si è letta la riga.
	j, err := db.New(s.Pool).GetJob(ctx, id)
	if err != nil {
		errore(w, 404, err)
		return
	}
	var prep *stagePronto
	if j.Tipo == db.TipoJobStageAllegato && req.Esito == "ok" {
		// Prima il tentativo, poi il disco: un result di un tentativo scaduto è 409 qualunque cosa
		// contenga (M13), e non vale la pena fare l'hash di un file che nessuno promuoverà. La
		// verifica vera, con la riga bloccata, resta quella dentro la transazione.
		if _, err := coda.Verifica(ctx, db.New(s.Pool), t); err != nil {
			if errors.Is(err, coda.ErrTentativoNonValido) {
				if parte := s.parteDi(ctx, db.New(s.Pool), id, allegatoDi(req.Dati), t.LeaseToken); parte != "" {
					staging.RimuoviParte(parte)
				}
				s.nonValido(w, ctx, id)
				return
			}
			errore(w, 500, err)
			return
		}
		prep, err = s.preparaStage(ctx, &j, t, req.Dati)
		if err != nil {
			s.risultatoNonApplicabile(w, ctx, &j, t, err, errors.Is(err, errHashDiverso))
			return
		}
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		errore(w, 500, err)
		return
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	// Il tentativo si verifica e si blocca QUI, prima di ogni scrittura, non solo alla fine con
	// Completa: la promozione di un file da .parte a definitivo è una rinomina sul disco, che il
	// rollback non annulla. Con la riga bloccata nessun altro tentativo può diventare valido fra
	// la rinomina e il commit (voce 2.3, M13).
	if _, err := coda.Blocca(ctx, q, t); err != nil {
		if errors.Is(err, coda.ErrTentativoNonValido) {
			if prep != nil {
				staging.RimuoviParte(prep.parte)
			}
			s.nonValido(w, ctx, id)
			return
		}
		errore(w, 500, err)
		return
	}

	// --------------------------------------------------- il worker riporta un errore
	if req.Esito != "ok" {
		msg := req.Errore
		if msg == "" {
			msg = "errore non specificato"
		}
		// Prima si verifica il tentativo, poi si tocca l'entità del job: un fallimento riportato da un
		// tentativo scaduto non deve marcare in errore un allegato che il tentativo nuovo sta
		// scaricando bene (Q19).
		if _, err := coda.Fallisci(ctx, q, t, msg, req.Definitivo); err != nil {
			if errors.Is(err, coda.ErrTentativoNonValido) {
				s.nonValido(w, ctx, id)
				return
			}
			errore(w, 500, err)
			return
		}
		if req.Definitivo {
			s.fallimentoDefinitivo(ctx, q, &j, msg)
		}
		if err := tx.Commit(ctx); err != nil {
			errore(w, 500, err)
			return
		}
		s.Log.Warn("job fallito", "job", id, "tipo", j.Tipo, "tentativi", j.Tentativi, "definitivo", req.Definitivo, "err", msg)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// --------------------------------------------------- il worker riporta un successo
	if err := s.applicaRisultato(ctx, q, &j, req.Dati, prep); err != nil {
		_ = tx.Rollback(ctx)
		s.risultatoNonApplicabile(w, ctx, &j, t, err, false)
		return
	}
	dati := req.Dati
	if len(dati) == 0 {
		dati = json.RawMessage("{}")
	}
	if _, err := coda.Completa(ctx, q, t, dati); err != nil {
		if errors.Is(err, coda.ErrTentativoNonValido) {
			// tutto ciò che applicaRisultato ha scritto sparisce con il rollback: «nulla applicato»
			// non è una promessa, è la transazione (Q15, Q20)
			s.nonValido(w, ctx, id)
			return
		}
		errore(w, 500, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		errore(w, 500, err)
		return
	}
	s.Log.Info("job fatto", "job", id, "tipo", j.Tipo)
	w.WriteHeader(http.StatusNoContent)
}

// allegatoDi legge il solo allegato_id da un RisultatoStage grezzo; uuid zero se non c'è.
func allegatoDi(dati json.RawMessage) uuid.UUID {
	var r struct {
		AllegatoID uuid.UUID `json:"allegato_id"`
	}
	_ = json.Unmarshal(dati, &r)
	return r.AllegatoID
}

// risultatoNonApplicabile chiude un result il cui contenuto è sbagliato: non la rete, il contenuto.
// Ritentarlo darebbe lo stesso esito all'infinito, quindi il job fallisce in modo definitivo in una
// transazione nuova (N6): senza, restava «in corso» fino alla scadenza del lease e l'operatore non
// vedeva nessun motivo. `ritentabile` è l'eccezione per l'hash che non torna (voce 2.3): un file
// corrotto nel trasferimento si ricarica, non si dichiara perso.
func (s *Server) risultatoNonApplicabile(w http.ResponseWriter, ctx context.Context, j *db.Job, t coda.Tentativo, err error, ritentabile bool) {
	motivo := fmt.Sprintf("risultato %s non applicabile: %v", j.Tipo, err)
	fuori := db.New(s.Pool)
	if _, e := coda.Fallisci(ctx, fuori, t, motivo, !ritentabile); e != nil && !errors.Is(e, coda.ErrTentativoNonValido) {
		s.Log.Error("fallimento dopo 422 non registrato", "job", j.JobID, "err", e)
	} else if e == nil && !ritentabile {
		s.fallimentoDefinitivo(ctx, fuori, j, motivo)
	}
	s.Log.Warn("risultato non applicabile", "job", j.JobID, "tipo", j.Tipo, "ritentabile", ritentabile, "err", err)
	errore(w, 422, errors.New(motivo))
}

// fallimentoDefinitivo scrive gli effetti di un errore non recuperabile sull'entità del job.
func (s *Server) fallimentoDefinitivo(ctx context.Context, q *db.Queries, j *db.Job, msg string) {
	switch j.Tipo {
	case db.TipoJobStageAllegato:
		var p worker.PayloadStageAllegato
		if json.Unmarshal(j.Payload, &p) == nil {
			_ = q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: p.AllegatoID, Stato: db.StatoAllegatoErrore, Errore: pgtype.Text{String: msg, Valid: true}})
		}
	case db.TipoJobAnalizzaAllegato:
		var p worker.PayloadAnalizzaAllegato
		if json.Unmarshal(j.Payload, &p) == nil {
			_ = q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: p.AllegatoID, Stato: db.StatoAllegatoErrore, Errore: pgtype.Text{String: msg, Valid: true}})
		}
	case db.TipoJobCreaBozzaOutlook:
		var p worker.PayloadCreaBozza
		if json.Unmarshal(j.Payload, &p) == nil {
			_ = q.SetBozzaErrore(ctx, db.SetBozzaErroreParams{BozzaID: p.BozzaID, Errore: pgtype.Text{String: msg, Valid: true}})
		}
	}
}

// stagePronto è tutto ciò che il result di un download ha bisogno di sapere e che si calcola PRIMA
// della transazione: dove sta il file caricato dal tentativo, dove deve finire, l'hash verificato,
// l'eventuale estrazione dello zip.
type stagePronto struct {
	r          worker.RisultatoStage
	p          worker.PayloadStageAllegato
	a          db.Allegato
	definitivo string // <staging>\_contenuti\<ab>\<sha256>.<ext>: dove il contenuto sta o andra'
	parte      string // <staging>\_parti\<allegato>.parte.<lease_token>: dove il tentativo ha caricato
	// riusato: quel contenuto c'era già, con l'hash giusto. Non si scrive niente e il .parte si
	// butta. E' il caso dello stesso disegno allegato a due richieste diverse, scaricate insieme
	// prima che l'una sapesse dell'altra: la deduplica per hash di AccodaStage non poteva vederlo,
	// perché prima di scaricare l'hash non lo conosce nessuno.
	riusato bool
}

// errHashDiverso: il file caricato non ha lo sha256 che il worker dichiara. Non è un contenuto
// sbagliato ma un trasferimento andato male: il job torna in coda e si ricarica.
var errHashDiverso = errors.New("sha256 del file caricato diverso da quello dichiarato")

// preparaStage fa il lavoro su disco del result di un download (voce 2.3), fuori dalla transazione:
// trova il .parte.<token> di questo tentativo, ne verifica lo sha256, decide dove va il contenuto.
// Un errore qui è un result non applicabile (422); l'unico ritentabile è l'hash che non torna.
//
// Verificare l'hash dentro la transazione era il difetto della voce 1.4: un file da qualche centinaio
// di megabyte teneva bloccata la riga del job per tutto il tempo dell'I/O. Qui la transazione arriva
// dopo, e contiene solo la rinomina e le scritture in database.
//
// Dal blocco 4A qui NON si estrae più niente: scompattare un archivio dentro una richiesta HTTP tiene
// fermo il worker che aspetta la risposta. L'estrazione è un job suo (archivi.go).
func (s *Server) preparaStage(ctx context.Context, j *db.Job, t coda.Tentativo, dati json.RawMessage) (*stagePronto, error) {
	var pr stagePronto
	if err := json.Unmarshal(dati, &pr.r); err != nil {
		return nil, err
	}
	if pr.r.Sha256 == "" {
		return nil, errors.New("sha256 obbligatorio")
	}
	q := db.New(s.Pool)
	p, a, err := s.stageDelJob(ctx, q, j, pr.r.AllegatoID)
	if err != nil {
		return nil, err
	}
	pr.p, pr.a = p, a
	pr.parte = staging.PercorsoParte(s.Staging, pr.r.AllegatoID, t.LeaseToken)
	if _, err := os.Stat(pr.parte); err != nil {
		return nil, fmt.Errorf("nessun file caricato da questo tentativo per l'allegato %s: il worker deve fare PUT /api/v1/allegati/{id}/file prima del result", a.AllegatoID)
	}
	h, n, err := nas.Sha256File(pr.parte)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(h, pr.r.Sha256) || n != pr.r.Bytes {
		staging.RimuoviParte(pr.parte)
		return nil, fmt.Errorf("%w: caricato %.12s (%d byte), dichiarato %.12s (%d byte)", errHashDiverso, h, n, strings.ToLower(pr.r.Sha256), pr.r.Bytes)
	}
	// Da qui il contenuto è verificato, e solo adesso si sa DOVE va: il suo nome è il suo hash.
	pr.definitivo, err = staging.PercorsoContenuto(s.Staging, h, a.NomeFile)
	if err != nil {
		return nil, err
	}
	// La cartella si crea QUI e non dentro la transazione: creare una cartella è I/O, e la transazione
	// del risultato deve contenere solo scritture in database (voce 1.4). Là dentro resta la sola
	// rinomina, che è atomica e non può fallire per una cartella che non c'è.
	if err := os.MkdirAll(filepath.Dir(pr.definitivo), 0o755); err != nil {
		return nil, err
	}
	pr.riusato = staging.ContenutoGiaPresente(pr.definitivo, h)
	if pr.riusato {
		s.Log.Info("contenuto già in staging: non se ne scrive una seconda copia",
			"allegato", a.AllegatoID, "file", a.NomeFile, "sha", h[:12], "byte", n)
	}
	return &pr, nil
}

// enumValido converte una stringa del contratto in un valore dell'enum del database, rifiutando ciò
// che l'enum non prevede (N7, voce 1.3).
//
// Prima questa conversione era un cast e basta. Un `tipo_proposto: "boh"` arrivava così com'era fino
// a PostgreSQL, che lo rifiutava con «invalid input value for enum»: il comportamento finale era
// giusto per caso, ma il motivo era illeggibile e — cosa peggiore — dipendeva dal fatto che la
// colonna fosse davvero un enum. Il giorno in cui diventasse `text`, il valore inventato entrerebbe
// in database senza che nessuno se ne accorga.
//
// Non esiste un valore «più vicino» a cui ricondurre un termine fuori enum: sarebbe un dato inventato
// dal server. Il risultato non è applicabile, quindi il job fallisce in modo definitivo (Q9):
// ritentarlo darebbe all'infinito lo stesso esito.
func enumValido[T interface {
	~string
	Valid() bool
}](campo, v string) (T, error) {
	e := T(v)
	if !e.Valid() {
		return "", fmt.Errorf("%s fuori enum: %q", campo, v)
	}
	return e, nil
}

// applicaRisultato scrive nel DB gli effetti di un job riuscito. `prep` è valorizzato solo per un
// download di allegato, ed è stato calcolato fuori dalla transazione.
func (s *Server) applicaRisultato(ctx context.Context, q *db.Queries, j *db.Job, dati json.RawMessage, prep *stagePronto) error {
	switch j.Tipo {
	case db.TipoJobSyncOutlook:
		var r worker.RisultatoSync
		if err := json.Unmarshal(dati, &r); err != nil {
			return err
		}
		var p worker.PayloadSyncOutlook
		_ = json.Unmarshal(j.Payload, &p)
		// Il cursore è per (casella, cartella): senza sapere di quale casella sia questo sync, il
		// risultato non è applicabile. Non si sceglie una casella per difetto — sarebbe il cursore di
		// una casella fatto avanzare dal sync di un'altra, cioè il difetto che la 0004 elimina.
		cas := j.CasellaID
		if !cas.Valid && p.CasellaID != nil {
			cas = uuid.NullUUID{UUID: *p.CasellaID, Valid: true}
		}
		if !cas.Valid {
			return fmt.Errorf("risultato di sync senza casella: il cursore è per (casella, cartella) e non si può attribuire")
		}
		finestre := map[string]worker.CartellaCursore{}
		for _, c := range p.Cartelle {
			finestre[c.Cartella] = c
		}
		for _, c := range r.Cartelle {
			// `ultimo_received` è la mail più recente che abbiamo di quella cartella, e si scrive
			// comunque: è un fatto sui messaggi consegnati, non una promessa su ciò che è stato
			// guardato. Anche una finestra interrotta a metà ha consegnato dei messaggi, e il più
			// recente di quelli è il più recente che abbiamo.
			if err := q.UpsertSyncCursore(ctx, db.UpsertSyncCursoreParams{
				CasellaID: cas.UUID, Cartella: c.Cartella, UltimoReceived: c.UltimoReceived,
				NMessaggi: int32(c.NMessaggi), Errore: txt(c.Errore),
			}); err != nil {
				return err
			}
			// Le FRONTIERE avanzano solo su dichiarazione esplicita del worker. Non «se il job non è
			// esploso», non «se il campo errore è vuoto»: quelle due condizioni erano vere anche
			// quando un'enumerazione COM si fermava a metà senza alzare niente, e in lettura dal più
			// recente al più vecchio ciò che resta fuori è la parte VECCHIA della finestra — cioè
			// proprio quella che, dichiarata coperta, nessuno riaprirebbe mai più (blocco 1 del 3R,
			// il caso lasciato aperto dal revisore).
			if !c.Completa {
				s.Log.Warn("finestra non conclusa: la frontiera resta dov'era", "job", j.JobID,
					"cartella", c.Cartella, "messaggi", c.NMessaggi, "errore", c.Errore)
				continue
			}
			f := finestre[c.Cartella]
			if p.ModoEffettivo() == worker.ModoStorico {
				// la finestra [dal, al] di QUESTA cartella è coperta: il prossimo «Carica precedenti»
				// riparte dal suo dal
				dal := p.Dal
				if f.Dal != nil {
					dal = *f.Dal
				}
				if err := q.SetStoricoFinoA(ctx, db.SetStoricoFinoAParams{CasellaID: cas.UUID, Cartella: c.Cartella, StoricoFinoA: &dal}); err != nil {
					return err
				}
				continue
			}
			al := f.Al
			if al == nil {
				al = p.Al
			}
			if al == nil {
				// payload accodato prima del blocco 3: non aveva un limite superiore fissato, quindi
				// non esiste un istante di cui si possa dire «scandito fino a qui». Si rilegge.
				s.Log.Warn("sync senza limite superiore: la copertura non avanza", "job", j.JobID, "cartella", c.Cartella)
				continue
			}
			if err := q.SetCopertoFinoA(ctx, db.SetCopertoFinoAParams{CasellaID: cas.UUID, Cartella: c.Cartella, CopertoFinoA: al}); err != nil {
				return err
			}
		}
		return nil

	case db.TipoJobStageAllegato:
		if prep == nil {
			return errors.New("download senza preparazione: il file non è stato verificato")
		}
		// da qui in poi il file è dell'allegato: la riga del job è bloccata (Blocca), quindi nessun
		// altro tentativo può promuovere il suo nel frattempo
		if prep.riusato {
			staging.RimuoviParte(prep.parte)
		} else if err := staging.Promuovi(prep.parte, prep.definitivo); err != nil {
			return err
		}
		r := prep.r
		if err := q.SetAllegatoStaging(ctx, db.SetAllegatoStagingParams{AllegatoID: r.AllegatoID, PathStaging: txt(prep.definitivo), Sha256: txt(strings.ToLower(r.Sha256)), Bytes: pgtype.Int8{Int64: r.Bytes, Valid: true}}); err != nil {
			return err
		}
		s.riallineaEntryID(ctx, q, prep.p.RiferimentoElemento, prep.p.EntryID, r.RisultatoElemento)
		return s.dopoStaging(ctx, q, r)

	case db.TipoJobAnalizzaAllegato:
		var r worker.RisultatoAnalisi
		if err := json.Unmarshal(dati, &r); err != nil {
			return err
		}
		var p worker.PayloadAnalizzaAllegato
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return err
		}
		// Il worker deve rimandare indietro la combinazione che gli era stata chiesta. Se dichiara una
		// versione o una configurazione diverse, i suoi fatti finirebbero archiviati sotto una chiave
		// che non descrive come sono stati ottenuti, e verrebbero riusati per file che non c’entrano.
		// È un errore di contenuto: il risultato non è applicabile e il job fallisce in modo definitivo.
		if r.VersioneAnalizzatore != p.VersioneAnalizzatore || r.HashConfigurazione != p.HashConfigurazione {
			return fmt.Errorf("il risultato dichiara analizzatore v%d/%.8s ma era stato chiesto v%d/%.8s",
				r.VersioneAnalizzatore, r.HashConfigurazione, p.VersioneAnalizzatore, p.HashConfigurazione)
		}
		tipo, err := enumValido[db.TipoDocumento]("tipo_proposto", r.TipoProposto)
		if err != nil {
			return err
		}
		fonte, err := enumValido[db.FonteProposta]("fonte", r.Fonte)
		if err != nil {
			return err
		}
		dett := r.Dettagli
		if len(dett) == 0 {
			dett = json.RawMessage("{}")
		}

		// I FATTI si conservano per (contenuto, versione, configurazione): non appartengono
		// all’allegato che ha fatto partire l’analisi, ma al file. È ciò che rende possibile non
		// rianalizzare lo stesso disegno per ogni RFQ in cui compare.
		a, err := q.GetAllegato(ctx, r.AllegatoID)
		if err != nil {
			return fmt.Errorf("allegato dell'analisi: %w", err)
		}
		if a.Sha256.Valid && a.Sha256.String != "" {
			if _, err := q.UpsertAnalisiFatti(ctx, db.UpsertAnalisiFattiParams{
				Sha256: a.Sha256.String, VersioneAnalizzatore: int16(r.VersioneAnalizzatore),
				HashConfigurazione: r.HashConfigurazione, Fatti: conEsito(dett, r),
			}); err != nil {
				return fmt.Errorf("analisi_fatti: %w", err)
			}
		}

		// L’INTERPRETAZIONE, invece, è di ogni singola proposta: dipende dalla direzione del messaggio
		// e dalle regole del cliente di quella RFQ. Gli stessi fatti danno proposte diverse in RFQ
		// diverse, e ognuna va scritta con le sue regole (A15).
		destinatari := []db.Allegato{a}
		if a.Sha256.Valid && a.Sha256.String != "" {
			altre, err := q.ListProposteAperteStessoFile(ctx, a.Sha256)
			if err != nil {
				return fmt.Errorf("proposte aperte con lo stesso contenuto: %w", err)
			}
			for _, pr := range altre {
				if pr.AllegatoID == a.AllegatoID {
					continue
				}
				al, err := q.GetAllegato(ctx, pr.AllegatoID)
				if err != nil {
					return err
				}
				destinatari = append(destinatari, al)
			}
		}
		for _, al := range destinatari {
			if err := s.propostaDaAnalisi(ctx, q, al, tipo, fonte, r, dett); err != nil {
				return err
			}
		}
		// La STRUTTURA di uno STEP diventa proposte in ogni RFQ che ha quel file, ciascuna con le regole
		// del suo cliente (B8.5, A1.2). Non solo dove la proposta del documento e' ancora aperta: un
		// disegno gia' confermato porta lo stesso la sua distinta.
		return s.strutturaNelleRfq(ctx, q, a, dett)

	case db.TipoJobCreaBozzaOutlook:
		var p worker.PayloadCreaBozza
		var r worker.RisultatoBozza
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			return err
		}
		if err := json.Unmarshal(dati, &r); err != nil {
			return err
		}
		return q.SetBozzaAperta(ctx, db.SetBozzaApertaParams{BozzaID: p.BozzaID, EntryID: txt(r.EntryID)})

	case db.TipoJobApriElementoOutlook, db.TipoJobSegnaLetto, db.TipoJobSpostaInCartella:
		// nessun effetto sul dominio; se il worker ha ritrovato l'elemento altrove, si riallinea l'EntryID
		var p struct {
			EntryID string `json:"entry_id"`
			worker.RiferimentoElemento
		}
		var r worker.RisultatoElemento
		if json.Unmarshal(j.Payload, &p) == nil && json.Unmarshal(dati, &r) == nil {
			s.riallineaEntryID(ctx, q, p.RiferimentoElemento, p.EntryID, r)
		}
		return nil
	}
	return nil
}

// propostaDaAnalisi scrive la proposta di UN allegato a partire dai fatti dell’analisi. È separata
// perché gli stessi fatti vengono applicati a più allegati con lo stesso contenuto, ciascuno nel suo
// messaggio: la direzione del messaggio può cambiare la lettura, e va riletta per ognuno (A15).
func (s *Server) propostaDaAnalisi(ctx context.Context, q *db.Queries, a db.Allegato,
	tipo db.TipoDocumento, fonte db.FonteProposta, r worker.RisultatoAnalisi, dett json.RawMessage) error {
	var threadID uuid.NullUUID
	entrata := false
	if m, err := q.GetMessaggio(ctx, a.MessaggioID); err == nil {
		threadID = m.ThreadID
		entrata = m.Direzione == db.DirezioneEntrata
	}
	codice, rev, conf := r.Codice, r.Rev, r.Confidenza
	if tipo == db.TipoDocumentoOffertaPromatec {
		codice, rev = "", ""
		// un'offerta Promatec la mandiamo noi: in entrata (senza "SO " nel nome) è un documento commerciale del cliente
		if entrata && !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(a.NomeFile)), "SO ") {
			tipo, conf = db.TipoDocumentoCommerciale, conf-30
		}
	}
	if conf < 0 {
		conf = 0
	}
	// Il codice e la revisione arrivano da un worker, cioe' da fuori: prima di finire in una colonna
	// passano dai limiti del dominio (7C.1, P0). Fuori misura → non nel campo, ma nei dettagli.
	// I suffissi decorativi del cliente («…_PRT») non sono il codice: il cartiglio letto da un nome di file
	// li porterebbe dentro, e il disegno non troverebbe il suo componente.
	codice, rev = motoreDelFile(ctx, q, threadID, a.MessaggioID).Canonico(codice, rev)
	var scarti map[string]any
	codice, rev, scarti = s.codiceRevSicuri(codice, rev, "analisi di "+a.NomeFile)
	if len(scarti) > 0 {
		dett = conDettagli(dett, scarti)
	}
	// Nemmeno il cartiglio di un file caricato a mano diventa la revisione del cliente (B8.7).
	if r, trattenuta := fascicolo.RevisioneProponibile(a.Origine, rev); trattenuta != "" {
		rev = r
		dett = conDettagli(dett, map[string]any{"rev_letta": trattenuta})
	}
	if _, err := q.UpsertProposta(ctx, db.UpsertPropostaParams{
		AllegatoID: a.AllegatoID, ThreadID: threadID, TipoProposto: tipo, Codice: txt(codice), Rev: txt(rev),
		Confidenza: int16(conf), Fonte: fonte, Dettagli: dett,
	}); err != nil {
		return fmt.Errorf("upsert proposta da analisi: %w", err)
	}
	return q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: a.AllegatoID, Stato: db.StatoAllegatoAnalizzato})
}

// esitoAnalisi è la LETTURA del worker — che cosa ha concluso, non solo che cosa ha visto — conservata
// dentro `analisi_fatti.fatti` sotto la chiave `esito`.
//
// I dettagli da soli non bastano a riscrivere una proposta: dicono i termini trovati e il PRODUCT
// dello STEP, non il tipo di documento né la confidenza. Finché l'unico a scrivere la proposta era il
// result dell'analisi la cosa non si vedeva, perché tipo e codice arrivavano nello stesso messaggio;
// per applicare i fatti a un allegato che arriva DOPO (applicaFattiEsistenti) servono anche quelli.
//
// Sta dentro il JSON e non in una colonna nuova perché `analisi_fatti` è la tabella dei fatti per
// contenuto e la sua chiave non cambia: si aggiunge una chiave al documento, non una migrazione.
type esitoAnalisi struct {
	TipoProposto string `json:"tipo_proposto"`
	Codice       string `json:"codice,omitempty"`
	Rev          string `json:"rev,omitempty"`
	Confidenza   int    `json:"confidenza"`
	Fonte        string `json:"fonte"`
}

// conEsito mette l'esito accanto ai dettagli, senza toccarli.
func conEsito(dett json.RawMessage, r worker.RisultatoAnalisi) json.RawMessage {
	return conDettagli(dett, map[string]any{"esito": esitoAnalisi{
		TipoProposto: r.TipoProposto, Codice: r.Codice, Rev: r.Rev,
		Confidenza: r.Confidenza, Fonte: r.Fonte,
	}})
}

// separaEsito rilegge quel documento: da una parte i dettagli come li ha scritti il worker, dall'altra
// l'esito. `ok` falso = fatti scritti prima di questa versione (nessun `esito`): non si indovina.
func separaEsito(fatti json.RawMessage) (json.RawMessage, esitoAnalisi, bool) {
	var campi map[string]json.RawMessage
	if err := json.Unmarshal(fatti, &campi); err != nil || campi == nil {
		return json.RawMessage("{}"), esitoAnalisi{}, false
	}
	grezzo, presente := campi["esito"]
	if !presente {
		return fatti, esitoAnalisi{}, false
	}
	var es esitoAnalisi
	if err := json.Unmarshal(grezzo, &es); err != nil || es.TipoProposto == "" || es.Fonte == "" {
		return fatti, esitoAnalisi{}, false
	}
	delete(campi, "esito")
	dett, err := json.Marshal(campi)
	if err != nil {
		dett = json.RawMessage("{}")
	}
	return dett, es, true
}

// applicaFattiEsistenti dà a un allegato appena sceso in staging l'analisi che un altro allegato con lo
// STESSO contenuto ha già fatto fare.
//
// È la seconda metà di AccodaAnalisi (`coda/stage.go`): quella restituisce (nil, nil) quando i fatti
// per (contenuto, versione, configurazione) ci sono già, e il commento diceva «il chiamante li
// riuserà» mentre i due chiamanti scartavano il valore. Il risultato era che il secondo allegato con
// lo stesso sha256 non riceveva MAI la lettura dell'analisi: restava con la proposta dal nome, e in
// una RFQ dove lo stesso disegno arriva in due mail il secondo file era sempre meno preciso del primo
// senza che nessuno potesse dire perché (voce 1.4 del piano B8).
//
// Non si accoda niente: i fatti sono già qui, e rianalizzare lo stesso contenuto per la seconda copia
// è esattamente ciò che la chiave per contenuto serve a evitare.
func (s *Server) applicaFattiEsistenti(ctx context.Context, q *db.Queries, a db.Allegato) error {
	if !a.Sha256.Valid || a.Sha256.String == "" {
		return nil
	}
	f, err := q.GetAnalisiFatti(ctx, db.GetAnalisiFattiParams{
		Sha256: a.Sha256.String, VersioneAnalizzatore: int16(s.Analizzatore.Versione),
		HashConfigurazione: s.Analizzatore.Hash(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // l'analisi è in coda o non è mai partita: la proposta resta quella dal nome
	}
	if err != nil {
		return fmt.Errorf("fatti già calcolati: %w", err)
	}
	dett, es, ok := separaEsito(f.Fatti)
	if !ok {
		// Fatti scritti prima che l'esito ci fosse: i dettagli da soli non dicono il tipo proposto, e
		// inventarlo sarebbe peggio che non scriverlo. Si riapplicheranno dopo una rianalisi, che una
		// versione nuova dell'analizzatore fa partire da sé.
		s.Log.Info("fatti senza esito: l'allegato resta con la proposta dal nome",
			"allegato", a.AllegatoID, "file", a.NomeFile, "versione", s.Analizzatore.Versione)
		return nil
	}
	tipo, err := enumValido[db.TipoDocumento]("tipo_proposto", es.TipoProposto)
	if err != nil {
		return fmt.Errorf("esito dei fatti di %s: %w", a.NomeFile, err)
	}
	fonte, err := enumValido[db.FonteProposta]("fonte", es.Fonte)
	if err != nil {
		return fmt.Errorf("esito dei fatti di %s: %w", a.NomeFile, err)
	}
	// La stessa funzione del result: la proposta è dell'allegato, quindi la direzione del messaggio e
	// le regole del suo cliente vanno rilette per questa copia, non copiate dalla prima (A15).
	if err := s.propostaDaAnalisi(ctx, q, a, tipo, fonte, worker.RisultatoAnalisi{
		AllegatoID: a.AllegatoID, TipoProposto: es.TipoProposto, Codice: es.Codice, Rev: es.Rev,
		Confidenza: es.Confidenza, Fonte: es.Fonte,
		VersioneAnalizzatore: s.Analizzatore.Versione, HashConfigurazione: s.Analizzatore.Hash(),
	}, dett); err != nil {
		return err
	}
	// e la struttura, se il file ne ha una, nella RFQ di questa copia
	m, err := q.GetMessaggio(ctx, a.MessaggioID)
	if err != nil || !m.ThreadID.Valid {
		return nil
	}
	mo, err := fascicolo.MotoreDellaRfq(ctx, q, m.ThreadID.UUID)
	if err != nil {
		return err
	}
	_, err = fascicolo.ApplicaStruttura(ctx, q, m.ThreadID.UUID, a, dett, mo)
	return err
}

// strutturaNelleRfq applica la struttura letta da uno STEP a ogni RFQ che ha un allegato con lo stesso
// contenuto. Il worker ha prodotto un FATTO; qui diventa interpretazione (proposte di nodi, archi,
// quantita' e rimozioni), con il Motore del cliente di ciascuna RFQ. Nessuna riga della BOM cambia: la
// cambia solo una decisione (fascicolo.AccettaNodo e le altre).
func (s *Server) strutturaNelleRfq(ctx context.Context, q *db.Queries, a db.Allegato, dett json.RawMessage) error {
	if !a.Sha256.Valid || a.Sha256.String == "" {
		return nil
	}
	if _, ok := worker.DecodificaStruttura(dett); !ok {
		return nil
	}
	righe, err := q.ListAllegatiStessoFileConRfq(ctx, a.Sha256)
	if err != nil {
		return fmt.Errorf("allegati con lo stesso contenuto: %w", err)
	}
	motori := map[uuid.UUID]*classificazione.Motore{}
	for _, r := range righe {
		thread := r.RfqID.UUID
		mo, visto := motori[thread]
		if !visto {
			if mo, err = fascicolo.MotoreDellaRfq(ctx, q, thread); err != nil {
				return err
			}
			motori[thread] = mo
		}
		es, err := fascicolo.ApplicaStruttura(ctx, q, thread, r.Allegato, dett, mo)
		if err != nil {
			return fmt.Errorf("struttura nella RFQ %s: %w", thread, err)
		}
		if es.Nodi+es.Relazioni > 0 {
			s.Log.Info("proposte di struttura", "rfq", thread, "file", r.Allegato.NomeFile,
				"nodi_aperti", es.NodiAperti, "relazioni_aperte", es.RelazioniAperte,
				"completa", es.Completa, "rimozioni", es.Rimozioni.Proposte, "rimozioni_sospese", es.Rimozioni.Sospese)
		}
	}
	return nil
}

// riallineaEntryID aggiorna la PRESENZA quando il worker ha trovato l'elemento con un EntryID diverso
// da quello del payload (elemento spostato di cartella dopo l'ultimo sync).
//
// Dalla 0004 l'EntryID è per copia, quindi serve sapere in QUALE casella il worker ha cercato: senza
// casella_id si scriverebbe l'EntryID trovato in una casella sopra a quello di un'altra, e da quel
// momento l'elemento nell'altra casella non si aprirebbe più. Un payload senza casella_id — un worker
// più vecchio del server — non viene indovinato: si lascia la presenza com'è e lo si scrive nel log,
// perché un EntryID stantio fa fallire un'operazione e si vede, mentre uno sbagliato ne fa fallire
// un'altra, in un altro momento, senza che nessuno colleghi le due cose.
func (s *Server) riallineaEntryID(ctx context.Context, q *db.Queries, rif worker.RiferimentoElemento, entryPayload string, r worker.RisultatoElemento) {
	if rif.MessaggioID == nil || r.EntryID == "" || r.EntryID == entryPayload {
		return
	}
	if rif.CasellaID == nil {
		s.Log.Warn("entry_id nuovo senza casella_id nel payload: presenza non riallineata",
			"messaggio", rif.MessaggioID, "entry_id", r.EntryID)
		return
	}
	if err := q.SetEntryIDPresenza(ctx, db.SetEntryIDPresenzaParams{
		MessaggioID: *rif.MessaggioID, CasellaID: *rif.CasellaID, EntryID: r.EntryID,
		Cartella: txt(r.Cartella),
	}); err != nil {
		s.Log.Warn("riallinea entry_id", "messaggio", rif.MessaggioID, "err", err)
		return
	}
	s.Log.Info("entry_id riallineato", "messaggio", rif.MessaggioID, "casella", rif.CasellaID, "cartella", r.Cartella)
}

// DopoCaricamento fa, per un file caricato a mano nel Fascicolo (B8.7), quello che dopoStaging fa per un
// allegato che un worker ha appena portato in staging: la proposta dal nome, il rumore, l'estrazione di un
// archivio, l'analisi (o i fatti che ci sono gia'). E' la stessa funzione, nella transazione di chi
// chiama: un caricamento interno fa la strada di tutti gli altri file.
func (s *Server) DopoCaricamento(ctx context.Context, q *db.Queries, allegatoID uuid.UUID) error {
	a, err := q.GetAllegato(ctx, allegatoID)
	if err != nil {
		return err
	}
	if !a.Sha256.Valid || !a.PathStaging.Valid {
		return fmt.Errorf("allegato %s: non e' in staging", allegatoID)
	}
	return s.dopoStaging(ctx, q, worker.RisultatoStage{AllegatoID: a.AllegatoID, Sha256: a.Sha256.String, Bytes: a.Bytes.Int64})
}

// dopoStaging: il file richiesto dall'operatore è in staging. Si raffina la proposta con ciò che ora si sa
// (hash → rumore già scartato), si estraggono gli zip in allegati figli e si accoda l'analisi Python
// (cartiglio, STEP) che raffina ancora finché la proposta resta aperta.
func (s *Server) dopoStaging(ctx context.Context, q *db.Queries, r worker.RisultatoStage) error {
	a, err := q.GetAllegato(ctx, r.AllegatoID)
	if err != nil {
		return err
	}
	m, err := q.GetMessaggio(ctx, a.MessaggioID)
	if err != nil {
		return err
	}
	dominio := ""
	if m.MittenteIndirizzo.Valid {
		if i := strings.LastIndex(m.MittenteIndirizzo.String, "@"); i >= 0 {
			dominio = m.MittenteIndirizzo.String[i+1:]
		}
	}
	pr := classificazione.PropostaDaNome(a.NomeFile, r.Bytes, string(m.Direzione))
	ext := strings.ToLower(a.Estensione.String)
	// rumore: hash già scartato da un operatore, o immagine vista ≥ 3 volte dallo stesso dominio (firme, loghi)
	if dominio != "" && pr.Tipo != "rumore" {
		if seen, _ := q.IsHashRumore(ctx, db.IsHashRumoreParams{Sha256: r.Sha256, Lower: dominio}); seen {
			pr = classificazione.Proposta{Tipo: "rumore", Fonte: "rumore", Confidenza: 95}
		} else if n, _ := q.ContaHashVisto(ctx, db.ContaHashVistoParams{Sha256: txt(r.Sha256), Lower: dominio}); n >= 3 && classificazione.EstImmagine(ext) {
			pr = classificazione.Proposta{Tipo: "rumore", Fonte: "rumore", Confidenza: 85}
		}
	}
	if err := s.scriviProposta(ctx, q, a, m.ThreadID, pr, map[string]any{"estensione": ext, "bytes": r.Bytes}); err != nil {
		return err
	}
	if pr.Tipo == "rumore" {
		return q.SetAllegatoStato(ctx, db.SetAllegatoStatoParams{AllegatoID: a.AllegatoID, Stato: db.StatoAllegatoAnalizzato})
	}
	if ext == "zip" {
		// L'archivio si scompatta in un job suo: qui si sta ancora dentro la richiesta HTTP con cui il
		// worker consegna il download, e il worker aspetta. La chiave di idempotenza è per allegato,
		// quindi un «Riscarica» che rimette lo stesso zip non accoda una seconda estrazione finché la
		// prima è in coda.
		_, err := coda.Accoda(ctx, q, db.TipoJobEstraiArchivio,
			worker.PayloadEstraiArchivio{AllegatoID: a.AllegatoID}, "estrai:"+a.AllegatoID.String(), 4)
		return err
	}
	j, err := coda.AccodaAnalisi(ctx, q, a, m.ThreadID, s.Analizzatore)
	if err != nil {
		return err
	}
	if j == nil {
		// i fatti di questo contenuto ci sono già: non partirà nessuna analisi, quindi la lettura la
		// applica qui il server, adesso
		return s.applicaFattiEsistenti(ctx, q, a)
	}
	return nil
}

func (s *Server) scriviProposta(ctx context.Context, q *db.Queries, a db.Allegato, threadID uuid.NullUUID, pr classificazione.Proposta, dettagli map[string]any) error {
	// pr arriva dal nostro dominio, non da un worker: qui un valore fuori enum sarebbe un errore di
	// programmazione. Si controlla lo stesso, perché è il punto in cui un tipo nuovo aggiunto al
	// dominio e dimenticato nella migrazione si vedrebbe subito e con il nome giusto.
	tipo, err := enumValido[db.TipoDocumento]("tipo_proposto", pr.Tipo)
	if err != nil {
		return err
	}
	fonte, err := enumValido[db.FonteProposta]("fonte", pr.Fonte)
	if err != nil {
		return err
	}
	if dettagli == nil {
		dettagli = map[string]any{}
	}
	// I suffissi decorativi del cliente («…_PRT») non sono il codice: il file deve trovare il suo componente.
	motore := motoreDelFile(ctx, q, threadID, a.MessaggioID)
	pr.Codice, pr.Rev = motore.Canonico(pr.Codice, pr.Rev)
	for i, c := range pr.CodiciNelNome {
		pr.CodiciNelNome[i] = motore.CanonicoNome(c)
	}
	if len(pr.CodiciNelNome) > 0 {
		// il nome non e' un codice, ma ne contiene: si conservano qui, non nella colonna
		dettagli["codici_nel_nome"] = pr.CodiciNelNome
	}
	codice, rev, scarti := s.codiceRevSicuri(pr.Codice, pr.Rev, a.NomeFile)
	for k, v := range scarti {
		dettagli[k] = v
	}
	// Un file caricato a mano non inventa una revisione del cliente (B8.7): quella scritta nel nome resta
	// nei dettagli, e la revisione del documento la scrive chi decide.
	if r, trattenuta := fascicolo.RevisioneProponibile(a.Origine, rev); trattenuta != "" {
		rev = r
		dettagli["rev_letta"] = trattenuta
	}
	dett, _ := json.Marshal(dettagli)
	if _, err := q.UpsertProposta(ctx, db.UpsertPropostaParams{
		AllegatoID: a.AllegatoID, ThreadID: threadID, TipoProposto: tipo, Codice: txt(codice), Rev: txt(rev),
		Confidenza: int16(pr.Confidenza), Fonte: fonte, Dettagli: dett,
	}); err != nil {
		return fmt.Errorf("proposta: %w", err)
	}
	return nil
}

// motoreDelFile e' il motore delle regole del cliente a cui un file appartiene: quello della RFQ del messaggio,
// altrimenti quello del cliente riconosciuto come controparte, altrimenti, per la posta di un fornitore, quello
// dei clienti che gli hanno chiesto qualcosa (come l'ingest, ingest.Motori.PerFornitore: lo stesso file non
// cambia codice fra l'arrivo e lo staging). Serve a togliere dal codice letto i suffissi decorativi; nil se
// non si sa, e un motore nil non toglie niente.
func motoreDelFile(ctx context.Context, q *db.Queries, threadID uuid.NullUUID, messaggio uuid.UUID) *classificazione.Motore {
	if threadID.Valid {
		if m, err := fascicolo.MotoreDellaRfq(ctx, q, threadID.UUID); err == nil {
			return m
		}
		return nil
	}
	m, err := q.GetMessaggio(ctx, messaggio)
	if err != nil {
		return nil
	}
	if !m.ControparteClienteID.Valid {
		if m.ControparteFornitoreID.Valid {
			return ingest.NuoviMotori().PerFornitore(ctx, q, m.ControparteFornitoreID.UUID)
		}
		return nil
	}
	c, err := q.GetCliente(ctx, m.ControparteClienteID.UUID)
	if err != nil {
		return nil
	}
	lette, _ := regole.LeggiRegole(c.Regole)
	return classificazione.Compila(c.RagioneSociale, lette)
}

// codiceRevSicuri applica i limiti del dominio (classificazione.MaxCodice, classificazione.MaxRev) a un codice e a una
// revisione che arrivano da fuori — dal nome di un file, dal risultato di un worker — PRIMA che
// finiscano in una colonna (7C.1, P0).
//
// Un valore fuori misura NON si tronca: un codice tagliato a 60 caratteri e' un codice diverso, e
// nessuno se ne accorgerebbe. Si toglie dal campo e si restituisce grezzo, con il suo nome
// (`codice_scartato`, `rev_scartata`), perche' vada nei dettagli della proposta: chi la guarda sa
// che c'era e com'era. E' la guardia che ha mancato al banco del 20/09/2026, quando un nome di
// file lungo ha fatto rifiutare il result di uno stage a file gia' caricato e verificato.
func (s *Server) codiceRevSicuri(codice, rev, dove string) (string, string, map[string]any) {
	scarti := map[string]any{}
	if codice != "" && !classificazione.CodiceAmmissibile(codice) {
		scarti["codice_scartato"] = codice
		s.Log.Warn("codice fuori misura: non scritto nella proposta, conservato nei dettagli",
			"dove", dove, "lunghezza", len(codice), "max", classificazione.MaxCodice)
		codice = ""
	}
	if rev != "" && !classificazione.RevAmmissibile(rev) {
		scarti["rev_scartata"] = rev
		s.Log.Warn("revisione fuori misura: non scritta nella proposta, conservata nei dettagli",
			"dove", dove, "lunghezza", len(rev), "max", classificazione.MaxRev)
		rev = ""
	}
	return codice, rev, scarti
}

// conDettagli aggiunge delle chiavi a un oggetto JSON di dettagli senza perdere quelle che ha.
func conDettagli(dett json.RawMessage, extra map[string]any) json.RawMessage {
	m := map[string]any{}
	if len(dett) > 0 {
		if err := json.Unmarshal(dett, &m); err != nil || m == nil {
			m = map[string]any{"dettagli_grezzi": string(dett)}
		}
	}
	for k, v := range extra {
		m[k] = v
	}
	out, err := json.Marshal(m)
	if err != nil {
		return dett
	}
	return out
}

func nullUUID(u uuid.NullUUID) *uuid.UUID {
	if !u.Valid {
		return nil
	}
	return &u.UUID
}

func txt(s string) pgtype.Text {
	if strings.TrimSpace(s) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// ---------------------------------------------------------------- ingest e cursori

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	var req worker.IngestRichiesta
	if err := leggi(r, &req); err != nil {
		errore(w, 400, err)
		return
	}
	if _, ok := stessoWorker(w, r, req.WorkerID); !ok {
		return
	}
	// Un lotto arriva sempre dentro un tentativo di sync: senza, il server non potrebbe distinguere
	// un worker vivo da uno scaduto che sta ancora scrivendo (precisazione P5).
	t, err := tentativo(req.JobID, req.WorkerID, req.LeaseToken)
	if err != nil {
		errore(w, 400, fmt.Errorf("ingest senza tentativo valido: %w", err))
		return
	}
	ctx := r.Context()

	// La casella si verifica PRIMA della transazione: se non è censita è un errore di configurazione
	// che riguarda tutto il lotto, non un dato da scartare elemento per elemento (I23).
	casella, err := ingest.RisolviCasella(ctx, db.New(s.Pool), req.CasellaID, s.CasellaDefault)
	if err != nil {
		if errors.Is(err, ingest.ErrCasellaNonCensita) {
			// il sync di questa casella non può funzionare finché qualcuno non corregge la
			// configurazione: farlo ritentare cinque volte non serve a nulla
			if _, e := coda.Fallisci(ctx, db.New(s.Pool), t, err.Error(), true); e != nil && !errors.Is(e, coda.ErrTentativoNonValido) {
				s.Log.Error("fallimento per casella non censita non registrato", "job", req.JobID, "err", e)
			}
			s.Log.Error("lotto rifiutato", "job", req.JobID, "err", err)
			errore(w, 422, err)
			return
		}
		errore(w, 500, err)
		return
	}

	inizio := time.Now()
	res, err := s.Ingest.Ingerisci(ctx, ingest.Lotto{
		Casella:   casella,
		Tentativo: &ingest.Tentativo{JobID: t.JobID, LeaseToken: t.LeaseToken, WorkerID: t.WorkerID},
		Messaggi:  req.Messaggi,
		Saltati:   req.Saltati,
		Cursore:   req.Cursore,
	})
	// il tempo del server, dichiarato al worker (7C.1, P1): la differenza con la sua misura della
	// chiamata e' la rete, e «la rete e' lenta» smette di essere un'ipotesi
	res.DurataMs = time.Since(inizio).Milliseconds()
	switch {
	case errors.Is(err, ingest.ErrTentativoNonValido):
		s.nonValido(w, ctx, req.JobID)
		return
	case err != nil:
		// Un errore qui è un guasto (database irraggiungibile, commit fallito), non un dato sbagliato:
		// un dato sbagliato finisce in scarto e il lotto continua. 5xx dice al worker di ripetere il
		// lotto identico, ed è sicuro farlo perché non è stato scritto niente (I16).
		s.Log.Error("ingest lotto", "job", req.JobID, "casella", casella.Indirizzo, "err", err)
		errore(w, 500, err)
		return
	}
	if res.Esiti == nil {
		res.Esiti = []worker.EsitoMessaggio{}
	}
	if res.Falliti > 0 {
		s.Log.Warn("lotto con scarti", "job", req.JobID, "inseriti", res.Inseriti, "aggiornati", res.Aggiornati, "falliti", res.Falliti)
	}
	scriviJSON(w, 200, res)
}

func (s *Server) cursori(w http.ResponseWriter, r *http.Request) {
	c, err := db.New(s.Pool).ListSyncCursori(r.Context())
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		errore(w, 500, err)
		return
	}
	out := make([]worker.CartellaCursore, 0, len(c))
	for _, x := range c {
		out = append(out, worker.CartellaCursore{Cartella: x.Cartella, UltimoReceived: x.UltimoReceived, CopertoFinoA: x.CopertoFinoA})
	}
	scriviJSON(w, 200, out)
}
