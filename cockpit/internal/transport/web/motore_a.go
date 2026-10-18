package web

// motore_a.go: l'adattatore fra il server HTTP e la Distinta nuova del motore A (giro 5, A1d-P2; direttiva A1d dell'11/10,
// §1 e §3.2; piano P2-P3). È l'unico file Go della Distinta nuova in questo package: i modelli, il calcolo in sola
// lettura, l'indice, le etichette e l'anteprima dello STEP stanno nel sottopackage transport/web/motore_a (alias
// motorea_web), che non importa questo package. Qui restano le rotte, la porta del flag, il no-store, soloAdmin, la
// traduzione degli errori in codici HTTP e il JSON; nessuna logica di dominio. Derivato dalla PR #18 di Stefano Valo
// (c543e89: registraMotoreA, conMotoreA, metodoNonAmmessoMotoreA, motoreADati, scriviDatiMotoreA, motoreAAnteprimaStep),
// ristrutturato.
//
// In A1d-P2 si montano solo le due rotte JSON:
//   - GET /thread/{id}/motore-a/dati: il JSON della pagina (contratto §2.4);
//   - GET /thread/{id}/motore-a/anteprima/step-strutturale: l'anteprima dell'autorizzazione dello STEP (§3.4 A).
//
// A1d-P4, passo B5.1: la pagina GET /thread/{id}/motore-a e il suo frammento htmx, con il template, la funzione
// etichetta e la loro catena, stanno in motore_a_pagina.go; registraMotoreA li monta con registraPaginaMotoreA. Questo
// file resta quello delle rotte JSON. Le rotte nuove di P4 (R1-R14 del contratto rev. 3) non ci sono ancora.
//
// A1d-P3 aggiunge qui il ramo Motore A di threadFrammento (dalMotoreA, esitoMotoreA, rispondiMotoreA, scriviEsitoMotoreA:
// in fondo al file),
// e in thread.go le sole righe che lo chiamano. Il ponte del browser che manda i gesti è web/static/motore_a_gesti.mjs.
//
// La catena di ogni rotta: conMotoreA (flag spento = http.NotFound, prima della sessione e senza intestazioni in più:
// Q-A1d-12 = A, raccomandazione della bozza; R36) → conNoStore (Cache-Control: no-store su ogni risposta che segue,
// compresi il 302/401 di aLogin, il 403 di nega, il 405 e gli errori) → soloAdmin (autenticato + ruolo admin) → gestore.
// Ogni GET ha una riga gemella senza metodo sullo stesso indirizzo: senza, a flag spento il mux risponderebbe 405 con
// Allow agli altri metodi, e la rotta si distinguerebbe da un indirizzo che non esiste (S-01 della revisione della PR).
//
// I codici: 400 richiesta non valida; 404 RFQ inesistente; 503 regole non pronte (il testo generico di
// Regole.Errore()) o troppi calcoli in corso (ErrOccupato, il tetto MassimoCalcoli); 504 tempo scaduto, anche
// nell'anteprima e negli HEAD (HTTP-01 della revisione di A1d-P2); 500 per ogni altro errore, con un testo generico (il
// dettaglio va nel log, mai al client). Il corpo è JSON ({"errore": "…"}), con no-store e nosniff, nelle risposte che
// scrivono i gestori di questo file, 2xx e 4xx/5xx; non lo sono le risposte della catena che non arrivano al gestore:
// il 404 a flag spento (quello del router), il 302/401 di aLogin e il 403 di nega (HTML del legacy), il 405 (testo, con
// Allow) (S-03 della revisione di A1d-P2). HEAD fa solo il controllo d'esistenza della RFQ (GetThread in sola lettura),
// senza calcolo, e risponde con il codice che darebbe la GET: 400 per un id o (nell'anteprima) un componente non validi,
// 404 per una RFQ che non c'è, 503 a regole non pronte (solo /dati), 504 al limite (HTTP-02).
//
// Le regole che decidono e quelle che danno il testo del 503 sono le stesse: quelle del servizio della richiesta
// (S-05 della revisione di A1d-P2).
//
// Una GET di motore-a non scrive niente del dominio: nessun job, nessun prepara, nessuna sincronizzazione. La riga
// della sessione la tocca autenticato, come su ogni rotta (ToccaSessione: eccezione RO-DOMINIO già dichiarata).

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// I testi generici delle risposte d'errore di motore-a: il dettaglio è nel log del server.
const (
	testoMotoreARFQ           = "RFQ non trovata"
	testoMotoreARegole        = "Le regole del motore A non sono caricate: il dettaglio è nel log dell'avvio."
	testoMotoreAOccupato      = "Il motore A sta già facendo altri calcoli: riprovare fra poco."
	testoMotoreATempo         = "Il calcolo del motore A non è finito nel tempo che l'indice delle regole concede: il dettaglio è nel log del server."
	testoMotoreATempoAnteprim = "L'anteprima dello STEP non è finita nel tempo concesso: il dettaglio è nel log del server."
	testoMotoreAErrore        = "Il calcolo del motore A non è riuscito: il dettaglio è nel log del server."
	testoMotoreAAnteprim      = "L'anteprima dello STEP non è riuscita: il dettaglio è nel log del server."
)

// corpoMotoreANonCodificato: il corpo di ripiego quando un valore non si codifica in JSON (testoMotoreAErrore), scritto
// a mano perché non dipenda dalla codifica che è appena fallita.
const corpoMotoreANonCodificato = `{"errore":"Il calcolo del motore A non è riuscito: il dettaglio è nel log del server."}`

// erroreMotoreA: il corpo JSON di una risposta d'errore di motore-a.
type erroreMotoreA struct {
	Errore string `json:"errore"`
}

// registraMotoreA monta le due rotte JSON della Distinta nuova (A1d-P2) e le loro righe gemelle, poi la pagina (A1d-P4,
// B5.1: registraPaginaMotoreA) e il guscio della Distinta unica di V1 (registraPaginaDistintaUnica). Prima il flag, poi
// il no-store, poi soloAdmin: la protezione sta nel punto di montaggio.
func (s *Server) registraMotoreA(mux *http.ServeMux) {
	mux.HandleFunc("GET /thread/{id}/motore-a/dati", s.conMotoreA(conNoStore(s.soloAdmin(s.motoreADati))))
	mux.HandleFunc("GET /thread/{id}/motore-a/anteprima/step-strutturale", s.conMotoreA(conNoStore(s.soloAdmin(s.motoreAAnteprimaStep))))
	// Gli altri metodi sugli stessi indirizzi: lo schema con il metodo è più specifico, quindi GET e HEAD restano alle
	// righe qui sopra; il resto arriva qui, a flag spento 404 come un indirizzo che non c'è, a flag acceso 405.
	mux.HandleFunc("/thread/{id}/motore-a/dati", s.conMotoreA(conNoStore(metodoNonAmmessoMotoreA)))
	mux.HandleFunc("/thread/{id}/motore-a/anteprima/step-strutturale", s.conMotoreA(conNoStore(metodoNonAmmessoMotoreA)))
	// A1d-P4, B5.1: la pagina e la sua riga gemella, con la stessa catena (motore_a_pagina.go)
	s.registraPaginaMotoreA(mux)
	// V1 (direttiva 23), ondata B: il guscio della Distinta unica e la sua riga gemella, con la stessa catena
	// (distinta_unica_pagina.go). Resta su /thread/{id}/distinta-unica finché la pagina di A1d-P4 tiene
	// /thread/{id}/motore-a: lo scambio è del passo 9 (decisioni dell'ondata A, PE-12)
	s.registraPaginaDistintaUnica(mux)
}

// conMotoreA è la porta: a flag spento il 404 standard del router (http.NotFound) a tutti, anonimi compresi, PRIMA della
// sessione (nessun GetSessione né ToccaSessione: R36) e senza intestazioni in più (Q-A1d-12 = A); a flag acceso passa
// la mano. Il flag si legge a ogni richiesta dal campo del Server, che app/runtime imposta una volta all'avvio.
func (s *Server) conMotoreA(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.MotoreA.Accesa {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	}
}

// conNoStore: Cache-Control: no-store prima di tutto ciò che segue, così lo portano anche le risposte che non scrive il
// gestore (il 302/401 di aLogin, il 403 di nega, il 405): i dati di motore-a cambiano con ogni gesto, anche di un
// collega (direttiva §3.2; Q-A1d-12 = A).
func conNoStore(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h(w, r)
	}
}

// metodoNonAmmessoMotoreA: un metodo diverso da GET e HEAD su un indirizzo di motore-a, a flag acceso: il 405 che il mux
// darebbe da sé (Allow: GET, HEAD), senza leggere la sessione.
func metodoNonAmmessoMotoreA(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD")
	http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
}

// servizioMotoreA: il servizio della richiesta, dai campi del Server. Un Pool nil resta un'interfaccia nil (nessun
// database: il servizio risponde ErrLettura invece di andare in panic).
func (s *Server) servizioMotoreA() motorea_web.Servizio {
	svc := motorea_web.Servizio{Regole: s.MotoreA, Log: s.Log}
	if s.Pool != nil {
		svc.Pool = s.Pool
	}
	return svc
}

// motoreADati: GET /thread/{id}/motore-a/dati.
func (s *Server) motoreADati(w http.ResponseWriter, r *http.Request) {
	s.datiMotoreA(w, r, s.servizioMotoreA())
}

// datiMotoreA: il gestore di /dati con il servizio passato da fuori (le prove del package lo chiamano con un Iniziatore
// finto). HEAD: le regole e l'esistenza della RFQ, nessun calcolo. GET: la pagina calcolata, in JSON.
func (s *Server) datiMotoreA(w http.ResponseWriter, r *http.Request, svc motorea_web.Servizio) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.scriviJSONMotoreA(w, r, http.StatusBadRequest, erroreMotoreA{Errore: "id non valido"})
		return
	}
	if r.Method == http.MethodHead {
		if !svc.Regole.Pronta() {
			s.erroreMotoreA(w, r, thread, svc, motorea_web.ErrRegole)
			return
		}
		if err := svc.Esiste(r.Context(), thread); err != nil {
			s.erroreMotoreA(w, r, thread, svc, err)
			return
		}
		headMotoreA(w)
		return
	}
	q := r.URL.Query()
	p, err := svc.Pagina(r.Context(), motorea_web.RichiestaPagina{Thread: thread, Vista: q.Get("vista"),
		Prodotto: q.Get("prodotto"), Scrive: almeno(utenteDa(r.Context()), db.RuoloUtenteOperatore)})
	if err != nil {
		s.erroreMotoreA(w, r, thread, svc, err)
		return
	}
	s.scriviJSONMotoreA(w, r, http.StatusOK, p)
}

// headMotoreA: la risposta 200 di un HEAD, con le intestazioni della GET e senza corpo.
func headMotoreA(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
}

// motoreAAnteprimaStep: GET /thread/{id}/motore-a/anteprima/step-strutturale?componente={cid}&documento=…|nodo=…
// &sorgente=….
func (s *Server) motoreAAnteprimaStep(w http.ResponseWriter, r *http.Request) {
	s.anteprimaStepMotoreA(w, r, s.servizioMotoreA())
}

// anteprimaStepMotoreA: il gestore dell'anteprima con il servizio passato da fuori. Riusa gli aiuti privati del
// Fascicolo legacy, che restano in questo package (direttiva §3.3): richiestaAutorizzazione legge la richiesta come la
// rotta legacy (con il componente in query al posto di {cid}), spiegaErrore dice un rifiuto, almeno e utenteDa decidono
// la firma. Come il legacy: id o componente non validi e form illeggibile → 400; un rifiuto della richiesta o del
// Fascicolo → 200 con «errore» (la finestra lo mostra); un errore del database → 500 generico. Diversamente dal legacy
// (e dalla PR #18): una RFQ che non esiste → 404 (F-10); il limite scaduto → 504. HEAD: id e componente come la GET
// (400), poi solo l'esistenza della RFQ, senza effetto (HTTP-02 della revisione di A1d-P2).
func (s *Server) anteprimaStepMotoreA(w http.ResponseWriter, r *http.Request, svc motorea_web.Servizio) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		s.scriviJSONMotoreA(w, r, http.StatusBadRequest, erroreMotoreA{Errore: "id non valido"})
		return
	}
	if r.Method == http.MethodHead {
		// gli stessi 400 della GET, senza leggere niente: la query (non c'è un corpo) e il componente
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			s.scriviJSONMotoreA(w, r, http.StatusBadRequest, erroreMotoreA{Errore: "richiesta non valida"})
			return
		}
		if _, err := uuid.Parse(q.Get("componente")); err != nil {
			s.scriviJSONMotoreA(w, r, http.StatusBadRequest, erroreMotoreA{Errore: "componente non valido"})
			return
		}
		if err := svc.Esiste(r.Context(), thread); err != nil {
			s.erroreMotoreA(w, r, thread, svc, err)
			return
		}
		headMotoreA(w)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.scriviJSONMotoreA(w, r, http.StatusBadRequest, erroreMotoreA{Errore: "richiesta non valida"})
		return
	}
	cid, err := uuid.Parse(r.Form.Get("componente"))
	if err != nil {
		s.scriviJSONMotoreA(w, r, http.StatusBadRequest, erroreMotoreA{Errore: "componente non valido"})
		return
	}
	req, err := richiestaAutorizzazione(r, cid)
	if err != nil {
		// richiestaAutorizzazione sbaglia solo con un rifiuto (documento o nodo non validi, tutti e due insieme)
		s.scriviJSONMotoreA(w, r, http.StatusOK, motorea_web.AnteprimaConErrore(spiegaErrore(err)))
		return
	}
	a, err := svc.AnteprimaSTEP(r.Context(), thread, req, almeno(utenteDa(r.Context()), db.RuoloUtenteOperatore))
	if err != nil {
		var rf rifiuto
		if errors.As(err, &rf) {
			s.scriviJSONMotoreA(w, r, http.StatusOK, motorea_web.AnteprimaConErrore(spiegaErrore(err)))
			return
		}
		s.erroreMotoreA(w, r, thread, svc, err)
		return
	}
	s.scriviJSONMotoreA(w, r, http.StatusOK, a)
}

// erroreMotoreA traduce un errore del servizio nel codice HTTP (commento di testa), con un testo generico nel corpo e il
// dettaglio nel log. Il testo del 503 lo danno le regole del servizio della richiesta, le stesse che hanno deciso (S-05).
// Una richiesta chiusa da chi l'ha fatta va nel log a livello Info, non Error.
func (s *Server) erroreMotoreA(w http.ResponseWriter, r *http.Request, thread uuid.UUID, svc motorea_web.Servizio, err error) {
	anteprima := strings.HasSuffix(r.URL.Path, "/anteprima/step-strutturale")
	switch {
	case errors.Is(err, motorea_web.ErrRFQInesistente):
		s.scriviJSONMotoreA(w, r, http.StatusNotFound, erroreMotoreA{Errore: testoMotoreARFQ})
	case errors.Is(err, motorea_web.ErrRegole):
		s.scriviJSONMotoreA(w, r, http.StatusServiceUnavailable, erroreMotoreA{Errore: cmp.Or(svc.Regole.Errore(), testoMotoreARegole)})
	case errors.Is(err, motorea_web.ErrOccupato):
		s.Log.Warn("motore A: troppi calcoli in corso", "percorso", r.URL.Path, "thread", thread, "err", err)
		s.scriviJSONMotoreA(w, r, http.StatusServiceUnavailable, erroreMotoreA{Errore: testoMotoreAOccupato})
	case errors.Is(err, motorea_web.ErrTempo) || errors.Is(err, context.DeadlineExceeded):
		// il testo dell'errore dice il limite
		s.Log.Warn("motore A: tempo scaduto", "percorso", r.URL.Path, "thread", thread, "err", err)
		testo := testoMotoreATempo
		if anteprima {
			testo = testoMotoreATempoAnteprim
		}
		s.scriviJSONMotoreA(w, r, http.StatusGatewayTimeout, erroreMotoreA{Errore: testo})
	default:
		if errors.Is(err, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) {
			s.Log.Info("motore A: la richiesta è stata chiusa da chi l'aveva fatta", "percorso", r.URL.Path, "thread", thread, "err", err)
		} else {
			s.Log.Error("motore A: richiesta non riuscita", "percorso", r.URL.Path, "thread", thread, "err", err)
		}
		testo := testoMotoreAErrore
		if anteprima {
			testo = testoMotoreAAnteprim
		}
		s.scriviJSONMotoreA(w, r, http.StatusInternalServerError, erroreMotoreA{Errore: testo})
	}
}

// scriviJSONMotoreA: il JSON prima in un buffer (mai un JSON a metà con un 200 già partito), poi nosniff e
// scriviJSONNoStore (Content-Type, no-store, lo stato, i byte). Un valore che non si codifica (HTTP-04 della revisione di
// A1d-P2): l'errore va nel log con il percorso, e il client riceve il 500 generico, in JSON come ogni altro errore.
func (s *Server) scriviJSONMotoreA(w http.ResponseWriter, r *http.Request, stato int, v any) {
	var buf bytes.Buffer
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		if s.Log != nil {
			s.Log.Error("motore A: la risposta non si codifica in JSON", "percorso", r.URL.Path, "stato", stato, "err", err)
		}
		scriviJSONNoStore(w, http.StatusInternalServerError, json.RawMessage(corpoMotoreANonCodificato))
		return
	}
	scriviJSONNoStore(w, stato, json.RawMessage(buf.Bytes()))
}

// ---------------------------------------------------------------- il ramo dei gesti (A1d-P3)
//
// I gesti della Distinta nuova partono dal ponte (web/static/motore_a_gesti.mjs) verso le rotte operative che già
// esistono (la lista chiusa del ponte: mappa dei gesti A1d-P3, §2.1), con HX-Request e con HX-Current-URL = la pagina
// /thread/{id}/motore-a. Le loro risposte passano da threadFrammento, che sceglie la vecchia interfaccia
// dall'intestazione (la Distinta, il Fascicolo, la pagina della RFQ) e dice un rifiuto con HTTP 200 dentro il frammento.
// Per la pagina nuova threadFrammento ha un ramo, il primo, solo a flag acceso: un solo <div data-ma-esito="ok|no">
// con il testo dell'avviso, mai la vecchia interfaccia. Il ponte legge l'esito come testo e, dopo un «ok», rilegge la
// pagina dal server.
//
// L'esito è avvisoNegativo (distinta.go) così com'è (Q-A1d-09 = A, raccomandazione della bozza): nessun elenco
// aggiuntivo di frasi. Il rifiutiMotoreA della PR #18 (le 9 frasi che avvisoNegativo legge «ok» e che la PR leggeva
// «no») non c'è. Le letture ambigue che restano (rifiuti con 200 letti «ok», errori tecnici letti «no», successi con
// testo negativo, idempotenti, successi parziali) sono fissate dalle prove di caratterizzazione
// (motore_a_gesti_test.go) e scritte nel README del package; il problema degli esiti HTTP NON è risolto qui.
//
// Vincolo per A1d-P4 (P3-GO-04 della revisione di P3): il ramo guarda solo HX-Current-URL, che htmx manda da solo con
// l'indirizzo della pagina. Una richiesta htmx della pagina motore-a (un hx-post del template) verso una rotta che
// finisce in threadFrammento riceverebbe il div compatto al posto della vecchia interfaccia: dalla pagina nuova quelle
// rotte si chiamano solo tramite il ponte, e il template non ha hx-post/hx-put/hx-delete verso /thread/{id}/… o
// /proposta/… (prova statica di Q4).
//
// Derivati dalla PR #18 di Stefano Valo (c543e89: dallaMotoreA, rispondiMotoreA; esitoMotoreA ridotto ad
// avvisoNegativo, rifiutiMotoreA tolto). Ristretti dopo la revisione di P3: dalMotoreA riconosce solo l'indirizzo
// esatto (P3-GO-01), rispondiMotoreA toglie le intestazioni HX-* messe prima (P3-GO-02) e conserva il 404 del legacy
// per una RFQ che non c'è (P3-GO-03).

// dalMotoreA dice se la richiesta viene dalla Distinta nuova di questa RFQ: HX-Current-URL è esattamente la pagina
// /thread/{id}/motore-a, come la manda il ponte (pagina() in motore_a_gesti.mjs):
//   - il percorso esatto, senza / finale, senza query, senza frammento, senza segmenti codificati (%2F);
//   - un indirizzo assoluto vale solo con lo stesso host della richiesta (r.Host) e lo schema http o https;
//   - l'id nella forma canonica di 36 caratteri, uguale al thread della rotta senza badare alle maiuscole: il ponte copia
//     l'id da location.pathname, che può essere scritto maiuscolo, e il router lo accetta. Niente graffe, urn: o 32
//     cifre senza trattini (forme che uuid.Parse accetterebbe).
//
// Non guarda il flag: lo guarda chi lo chiama (threadFrammento). L'intestazione la scrive il client: il controllo non è
// una difesa (le POST d'altra origine le ferma già CrossOriginProtection), sceglie solo la forma della risposta.
func dalMotoreA(r *http.Request, thread uuid.UUID) bool {
	u, err := url.Parse(r.Header.Get("HX-Current-URL"))
	if err != nil || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return false
	}
	if u.Scheme != "" || u.Host != "" {
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || !strings.EqualFold(u.Host, r.Host) {
			return false
		}
	}
	id, ok := strings.CutPrefix(u.EscapedPath(), "/thread/")
	if !ok {
		return false
	}
	id, ok = strings.CutSuffix(id, "/motore-a")
	return ok && len(id) == 36 && strings.EqualFold(id, thread.String())
}

// esitoMotoreA: «no» se avvisoNegativo legge il testo come un gesto che non ha fatto niente, altrimenti «ok». Nessuna
// frase in più: la semantica è quella della Distinta e del Fascicolo legacy (Q-A1d-09 = A).
func esitoMotoreA(avviso string) string {
	if avvisoNegativo(avviso) {
		return "no"
	}
	return "ok"
}

// intestazioniHXMotoreA: le intestazioni di risposta che htmx eseguirebbe. Nessuna esce dal ramo motore-a: oggi l'unica
// messa prima è l'HX-Trigger: inbox-aggiorna di pannelloConAvviso (allegati.go) su /proposta/{u}/scarta, che resta nel
// gestore legacy e si toglie solo qui.
var intestazioniHXMotoreA = []string{"HX-Trigger", "HX-Trigger-After-Settle", "HX-Trigger-After-Swap", "HX-Redirect",
	"HX-Refresh", "HX-Location", "HX-Push-Url", "HX-Replace-Url", "HX-Retarget", "HX-Reswap", "HX-Reselect"}

// rispondiMotoreA risponde a un gesto partito dalla Distinta nuova. Toglie le intestazioni HX-* che un gestore legacy
// ha messo prima (P3-GO-02). Poi legge soltanto se la RFQ esiste (GetThread, la stessa prima lettura di caricaThread):
// se non c'è, 404 «thread non trovato», come il percorso legacy di threadFrammento, che il ponte legge «errore»; mai un
// «ok» per una RFQ che non esiste (P3-GO-03: riprova-copie su un id inventato rispondeva «ok»). Altrimenti
// scriviEsitoMotoreA. Nessun'altra lettura: la vecchia interfaccia non si compone.
func (s *Server) rispondiMotoreA(w http.ResponseWriter, r *http.Request, thread uuid.UUID, avviso string) {
	for _, k := range intestazioniHXMotoreA {
		w.Header().Del(k)
	}
	if _, err := db.New(s.Pool).GetThread(r.Context(), thread); err != nil {
		http.Error(w, "thread non trovato", 404)
		return
	}
	scriviEsitoMotoreA(w, avviso)
}

// scriviEsitoMotoreA: 200, text/html, Cache-Control: no-store, un solo elemento <div data-ma-esito="ok|no">testo</div>,
// con l'escape di html/template; mai la vecchia interfaccia, nessuna intestazione HX-* (anche quelle messe prima si
// tolgono). Un avviso vuoto dà <div data-ma-esito="ok"></div>.
func scriviEsitoMotoreA(w http.ResponseWriter, avviso string) {
	for _, k := range intestazioniHXMotoreA {
		w.Header().Del(k)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, `<div data-ma-esito="%s">%s</div>`, esitoMotoreA(avviso), template.HTMLEscapeString(avviso))
}
