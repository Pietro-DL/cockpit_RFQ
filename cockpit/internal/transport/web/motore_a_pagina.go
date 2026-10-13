package web

// motore_a_pagina.go: la pagina della Distinta nuova, GET /thread/{id}/motore-a, e il suo frammento htmx (giro 5,
// A1d-P4, passo B5.1 del piano esecutivo B2-B5 del 13/10; piano dell'addendum P4, P4-0a e P4-0b). Il template è
// web/templates/motore_a.html, il comportamento web/static/motore_a.mjs, l'aspetto web/static/motore_a.css: vengono dalla
// PR #18 di Stefano Valo (ba1a1e8, 153117d: motoreAPagina, rendiMotoreA e i tre file), riallineati al sottopackage
// transport/web/motore_a e al ponte di A1d-P3 (scelta D-T-11 dell'orchestratore [T]: base = PR #18 riallineata).
//
// La catena è quella delle rotte JSON (motore_a.go): conMotoreA (flag spento = il 404 del router, prima della sessione)
// → conNoStore → soloAdmin → gestore, con la riga gemella senza metodo per gli altri metodi (405 a flag acceso, 404 a
// flag spento). All'apertura la pagina non scrive niente: le sole letture sono quelle del servizio (Servizio.Pagina, in
// REPEATABLE READ READ ONLY con ROLLBACK) e, su un errore prima della lettura, Servizio.Esiste. Nessun job, nessun
// prepara (DB7-6), nessuna sincronizzazione. La riga della sessione la tocca autenticato, come su ogni rotta.
//
// Gli errori (D-18 = A, decisione dell'utente nella direttiva 12 del 13/10, §9): una RFQ che non esiste è un 404; un
// id non valido un 400; ogni altro problema (regole non pronte, troppi calcoli, tempo scaduto, calcolo non riuscito) è
// la pagina o il frammento 200 con Errore scritto e SENZA esito, mai una pagina vuota, con la tipologia nel log (chiave
// "tipo", tipoErroreMotoreA) e il dettaglio solo nel log. Pagina intera e frammento htmx si comportano allo stesso modo:
// stesso codice, stessi attributi. La radice del frammento dice lo stato del calcolo con un attributo suo,
// data-ma-calcolo = "completo" | "incompleto" | "errore", e con l'errore data-ma-errore = la tipologia: motore_a.mjs lo
// legge dopo ogni rilettura e non lascia che la riuscita del trasporto (il 200, lo scambio di #ma) sembri la riuscita del
// calcolo. Nomi e forma degli attributi sono scelte tecniche [T] di questa corsia.
//
// Esito incompleto (R-02 della revisione di B5.1, direttiva 12 §7): con il thread non valutato, o con almeno una
// diagnostica del thread di gravità «errore», la pagina lo dice in una striscia (MotiviIncompleto) e nessun
// «verificato» appare come positivo: i badge di verifica (prodotto e BOM) diventano «verifica non valida: esito
// incompleto», con il valore del server nel title. La regola è di presentazione [T]: non cambia l'esito, lo legge.
//
// Due scelte tecniche di B5.1 [T], dettate allora dal perimetro dei file della corsia:
//   - il template della pagina entra in Init (server.go) dalla direttiva 12, con le funzioni di tutte le pagine più
//     funzioniMotoreA (etichetta, che resta fuori dalla FuncMap comune): un errore del template ferma l'avvio invece di
//     dare un 500 alla prima apertura (R-06 della revisione di B5.1). TestMotoreAPaginaInitConLeFunzioni lo sorveglia;
//   - i dati che arriveranno con i blocchi B2-B4 (tipo_visualizzato, avviso dello STEP secondario, blocchi e avvisi del
//     riepilogo R4 come CODICE_DA_ASSEGNARE) hanno un posto nella vista (postiP4), vuoto finché il backend non li dà: il
//     sottopackage (WM/modelli.go) lo cambierà B3.6, non questa corsia.
//
// Vista e prodotto (R-01 della revisione di B5.1, direttiva 12 §7). La sorgente di verità [T] è la scelta resa: la
// radice del frammento porta data-ma-prodotto e data-ma-vista, e motore_a.mjs li riscrive nell'indirizzo della pagina
// con history.replaceState (all'avvio e dopo ogni rilettura riuscita, mai dopo un frammento d'errore). Il server li
// legge, in quest'ordine (sceltaMotoreADa):
//  1. dalla query della richiesta;
//  2. se la query non ha né vista né prodotto e la richiesta è la rilettura htmx di #ma (HX-Request "true", HX-Target
//     "ma"), da HX-Current-URL, che htmx manda con l'indirizzo del browser (quindi, grazie al modulo, sempre con la
//     scelta), purché sia la stessa pagina;
//  3. altrimenti nessuna scelta: il servizio dà la vista distinta e il primo prodotto, e se era una rilettura il
//     frammento lo dice in una striscia (data-ma-sorgente="nessuna"), perché il server non può sapere che cosa si
//     guardava prima. Un prodotto chiesto che non c'è più (o una vista che non esiste) si dice anche lui, mai in silenzio.
// Il ponte (motore_a_gesti.mjs, fuori dal perimetro) non cambia: la richiesta che controlla resta la stessa, il server
// legge un'intestazione che htmx manda già. Se B5.2 farà mandare al ponte vista e prodotto nella query, la query vince.

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/platform/db"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// testoMotoreARFQPagina: il corpo del 404 della pagina (testo, come http.Error del legacy).
const testoMotoreARFQPagina = "RFQ non trovata"

// titoloPaginaMotoreA: il titolo della pagina («<titolo> · Cockpit RFQ»), diverso da quello della Distinta legacy.
const titoloPaginaMotoreA = "Distinta nuova"

// registraPaginaMotoreA monta la pagina e la sua riga gemella (la chiama registraMotoreA). Stessa catena delle rotte
// JSON: prima il flag, poi il no-store, poi soloAdmin.
func (s *Server) registraPaginaMotoreA(mux *http.ServeMux) {
	mux.HandleFunc("GET /thread/{id}/motore-a", s.conMotoreA(conNoStore(s.soloAdmin(s.motoreAPagina))))
	mux.HandleFunc("/thread/{id}/motore-a", s.conMotoreA(conNoStore(metodoNonAmmessoMotoreA)))
}

// distintaNuovaVisibile: il collegamento «Distinta nuova (Motore A)» delle pagine Richieste e RFQ (giro 5, A1d, notte
// 14/10, WP-U, U1; diagnosi UI del 14/10, G2: la pagina non era raggiungibile dal flusso dell'operatore). Si mostra con
// la stessa guardia della rotta (registraPaginaMotoreA): il flag [motore_a].anteprima acceso (conMotoreA) e il ruolo
// admin (soloAdmin, con almeno, la stessa funzione del wrapper). Un collegamento che porterebbe a un 404 o a un 403 non
// si mostra. Quando la pagina si aprirà agli operatori [D], cambiano insieme la rotta e questa funzione.
func (s *Server) distintaNuovaVisibile(u *db.Utente) bool {
	return s.MotoreA.Accesa && almeno(u, db.RuoloUtenteAdmin)
}

// motoreAPagina: GET /thread/{id}/motore-a.
func (s *Server) motoreAPagina(w http.ResponseWriter, r *http.Request) {
	s.paginaMotoreA(w, r, s.servizioMotoreA())
}

// paginaMotoreA: il gestore della pagina con il servizio passato da fuori (le prove lo chiamano con un Iniziatore finto).
//   - id non valido: 400;
//   - HEAD: il solo controllo d'esistenza (Servizio.Esiste), nessun calcolo: 404 per una RFQ che non c'è, altrimenti 200
//     con le intestazioni della GET e senza corpo (anche se la lettura non riesce: la GET darebbe la pagina con l'errore);
//   - GET: la pagina intera, o con HX-Request il frammento motore_a_corpo (rendiMotoreA); 404 per una RFQ che non c'è,
//     la pagina con Errore per ogni altro problema (D-18, commento di testa).
func (s *Server) paginaMotoreA(w http.ResponseWriter, r *http.Request, svc motorea_web.Servizio) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodHead {
		if err := svc.Esiste(r.Context(), thread); errors.Is(err, motorea_web.ErrRFQInesistente) {
			// le intestazioni di http.Error, senza corpo
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(http.StatusNotFound)
			return
		} else if err != nil {
			s.Log.Warn("motore A: HEAD della pagina, l'esistenza della RFQ non si legge", "thread", thread, "err", err)
		}
		intestazioniPaginaMotoreA(w)
		w.WriteHeader(http.StatusOK)
		return
	}
	vista, prodotto := vistaEProdottoMotoreA(r)
	scrive := almeno(utenteDa(r.Context()), db.RuoloUtenteOperatore)
	p, err := svc.Pagina(r.Context(), motorea_web.RichiestaPagina{Thread: thread, Vista: vista, Prodotto: prodotto, Scrive: scrive})
	if err != nil {
		// regole non pronte e troppi calcoli si dicono prima di leggere: una RFQ che non c'è resta un 404
		if errors.Is(err, motorea_web.ErrRegole) || errors.Is(err, motorea_web.ErrOccupato) {
			if e := svc.Esiste(r.Context(), thread); errors.Is(e, motorea_web.ErrRFQInesistente) {
				err = e
			}
		}
		if errors.Is(err, motorea_web.ErrRFQInesistente) {
			http.Error(w, testoMotoreARFQPagina, http.StatusNotFound)
			return
		}
		testo, tipo := s.errorePaginaMotoreA(r, thread, svc, err)
		p = &motorea_web.Pagina{ThreadID: thread, Vista: motorea_web.VistaDa(vista), Scrive: scrive, Errore: testo}
		s.rendiVistaMotoreA(w, r, vistaMotoreADa(r, p, tipo))
		return
	}
	s.rendiMotoreA(w, r, p)
}

// sceltaMotoreA: la vista e il prodotto che la richiesta chiede, e da dove vengono (R-01, commento di testa).
type sceltaMotoreA struct {
	Vista, Prodotto string
	Sorgente        string // sorgenteQuery | sorgenteIndirizzo | sorgenteNessuna
	Rilettura       bool   // la rilettura htmx di #ma: HX-Request "true" e HX-Target "ma"
}

// Le sorgenti della scelta (data-ma-sorgente sulla radice del frammento) [T].
const (
	sorgenteQuery     = "query"     // la query della richiesta
	sorgenteIndirizzo = "indirizzo" // HX-Current-URL della rilettura htmx di #ma
	sorgenteNessuna   = "nessuna"   // nessuna delle due: la scelta predefinita del servizio
)

// sceltaMotoreADa: la scelta della richiesta. Dalla query; se la query non ha né vista né prodotto e la richiesta è la
// rilettura htmx di #ma, da HX-Current-URL, solo se è un indirizzo della stessa pagina (stesso percorso, a meno della /
// finale; lo stesso host quando c'è) e porta almeno uno dei due. Altrimenti nessuna: il servizio sceglie distinta e il
// primo prodotto. I valori non si controllano qui: li controllano VistaDa e ProdottoScelto, come per la query, e la
// vista della pagina dice se non sono stati presi (ProdottoNonTrovato, VistaNonValida).
func sceltaMotoreADa(r *http.Request) sceltaMotoreA {
	c := sceltaMotoreA{Rilettura: r.Header.Get("HX-Request") == "true" && r.Header.Get("HX-Target") == "ma"}
	q := r.URL.Query()
	if c.Vista, c.Prodotto = q.Get("vista"), q.Get("prodotto"); c.Vista != "" || c.Prodotto != "" {
		c.Sorgente = sorgenteQuery
		return c
	}
	c.Sorgente = sorgenteNessuna
	if !c.Rilettura {
		return c
	}
	corrente, err := url.Parse(r.Header.Get("HX-Current-URL"))
	if err != nil || corrente.Path == "" || strings.TrimSuffix(corrente.Path, "/") != strings.TrimSuffix(r.URL.Path, "/") ||
		(corrente.Host != "" && !strings.EqualFold(corrente.Host, r.Host)) {
		return c
	}
	cq := corrente.Query()
	if v, p := cq.Get("vista"), cq.Get("prodotto"); v != "" || p != "" {
		c.Vista, c.Prodotto, c.Sorgente = v, p, sorgenteIndirizzo
	}
	return c
}

// vistaEProdottoMotoreA: la vista e il prodotto chiesti (sceltaMotoreADa senza la sorgente).
func vistaEProdottoMotoreA(r *http.Request) (vista, prodotto string) {
	c := sceltaMotoreADa(r)
	return c.Vista, c.Prodotto
}

// Le tipologie degli errori della pagina (D-18 = A [U]): nel log (chiave "tipo") e sulla radice del frammento
// (data-ma-errore). I nomi sono [T].
const (
	erroreRegole        = "regole_non_pronte"
	erroreOccupato      = "occupato"
	erroreTempo         = "tempo"
	erroreAnnullata     = "annullata"
	errorePanic         = "panic"
	erroreVersione      = "versione"
	erroreValutazione   = "valutazione"
	erroreLettura       = "lettura"
	erroreAltro         = "altro"
	erroreNonDichiarato = "non_dichiarato" // una pagina senza esito arrivata a rendiMotoreA senza la tipologia
)

// tipoErroreMotoreA: la tipologia di un errore del servizio (gli errori di servizio.go, poi i due del contesto).
func tipoErroreMotoreA(r *http.Request, err error) string {
	switch {
	case errors.Is(err, motorea_web.ErrRegole):
		return erroreRegole
	case errors.Is(err, motorea_web.ErrOccupato):
		return erroreOccupato
	case errors.Is(err, motorea_web.ErrTempo) || errors.Is(err, context.DeadlineExceeded):
		return erroreTempo
	case errors.Is(err, context.Canceled) || errors.Is(r.Context().Err(), context.Canceled):
		return erroreAnnullata
	case errors.Is(err, motorea_web.ErrPanic):
		return errorePanic
	case errors.Is(err, motorea_web.ErrVersione):
		return erroreVersione
	case errors.Is(err, motorea_web.ErrValutazione):
		return erroreValutazione
	case errors.Is(err, motorea_web.ErrLettura):
		return erroreLettura
	}
	return erroreAltro
}

// errorePaginaMotoreA: il testo generico di un errore della pagina (gli stessi di /dati), la tipologia e la riga di log
// con la tipologia (D-18 = A [U]: «registrare nei log la tipologia dell'errore»), il percorso, la RFQ, se è il frammento,
// la sorgente della scelta e l'errore; mai il dettaglio nella pagina. Le regole non pronte vanno nel log anche qui
// (a differenza di /dati, dove il 503 basta): la pagina risponde 200 e senza la riga non resterebbe traccia.
func (s *Server) errorePaginaMotoreA(r *http.Request, thread uuid.UUID, svc motorea_web.Servizio, err error) (testo, tipo string) {
	tipo = tipoErroreMotoreA(r, err)
	attr := []any{"tipo", tipo, "percorso", r.URL.Path, "thread", thread, "frammento", frammentoRichiesto(r),
		"sorgente", sceltaMotoreADa(r).Sorgente, "err", err}
	switch tipo {
	case erroreRegole:
		s.Log.Warn("motore A: pagina senza esito, regole non pronte", attr...)
		return cmp.Or(svc.Regole.Errore(), testoMotoreARegole), tipo
	case erroreOccupato:
		s.Log.Warn("motore A: pagina senza esito, troppi calcoli in corso", attr...)
		return testoMotoreAOccupato, tipo
	case erroreTempo:
		s.Log.Warn("motore A: pagina senza esito, tempo scaduto", attr...)
		return testoMotoreATempo, tipo
	case erroreAnnullata:
		s.Log.Info("motore A: pagina senza esito, la richiesta è stata chiusa da chi l'aveva fatta", attr...)
		return testoMotoreAErrore, tipo
	}
	s.Log.Error("motore A: pagina senza esito, calcolo non riuscito", attr...)
	return testoMotoreAErrore, tipo
}

// intestazioniPaginaMotoreA: le intestazioni di una risposta 200 della pagina e del frammento.
func intestazioniPaginaMotoreA(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// ---------------------------------------------------------------- la vista della pagina

// paginaMotoreAVista: i dati del template (.Dati): la pagina del sottopackage (i suoi campi si leggono come prima,
// .Dati.Esito, .Dati.Indice…) e i posti dei dati di P4.
type paginaMotoreAVista struct {
	*motorea_web.Pagina
	P4         postiP4
	Scelta     sceltaMotoreA // la scelta chiesta e la sua sorgente (R-01)
	TipoErrore string        // la tipologia dell'errore, solo senza esito (D-18)
}

// Gli stati del calcolo (data-ma-calcolo sulla radice) [T].
const (
	calcoloCompleto   = "completo"
	calcoloIncompleto = "incompleto"
	calcoloErrore     = "errore"
)

// vistaMotoreADa: la vista della pagina con la scelta della richiesta e, senza esito, la tipologia dell'errore.
func vistaMotoreADa(r *http.Request, p *motorea_web.Pagina, tipo string) paginaMotoreAVista {
	v := paginaMotoreAVista{Pagina: p, P4: postiP4Da(p), Scelta: sceltaMotoreADa(r)}
	if p == nil || p.Esito == nil {
		v.TipoErrore = cmp.Or(tipo, erroreNonDichiarato)
	}
	return v
}

// StatoCalcolo: errore senza esito; incompleto con l'esito e almeno un motivo di MotiviIncompleto; completo altrimenti.
// «Completo» dice solo che l'esito c'è ed è stato calcolato per intero: non che vada tutto bene.
func (v paginaMotoreAVista) StatoCalcolo() string {
	switch {
	case v.Pagina == nil || v.Esito == nil:
		return calcoloErrore
	case len(v.MotiviIncompleto()) > 0:
		return calcoloIncompleto
	}
	return calcoloCompleto
}

// motivoIncompleto: perché l'esito è incompleto, con un codice per il frammento e il testo per chi guarda.
type motivoIncompleto struct {
	Codice, Testo string
}

// MotiviIncompleto: il thread non valutato (EsitoThread.Valutato falso) e le diagnostiche del thread di gravità
// «errore» («blocca l'attivazione o il risultato operativo», evidenze.GravitaErrore), nell'ordine. Senza esito niente:
// lì c'è l'errore. Si legge l'esito com'è: niente si ricalcola.
func (v paginaMotoreAVista) MotiviIncompleto() []motivoIncompleto {
	if v.Pagina == nil || v.Esito == nil {
		return nil
	}
	var out []motivoIncompleto
	if !v.Esito.Valutato {
		out = append(out, motivoIncompleto{"thread_non_valutato", "il thread non è valutato"})
	}
	n := 0
	for _, d := range v.Esito.Diagnostiche {
		if d.Gravita == evidenze.GravitaErrore {
			n++
		}
	}
	switch {
	case n == 1:
		out = append(out, motivoIncompleto{"diagnostiche_errore", "una diagnostica del thread ha gravità «errore»"})
	case n > 1:
		out = append(out, motivoIncompleto{"diagnostiche_errore", fmt.Sprintf("%d diagnostiche del thread hanno gravità «errore»", n)})
	}
	return out
}

// CodiciIncompleto: i codici di MotiviIncompleto separati da uno spazio (data-ma-incompleto).
func (v paginaMotoreAVista) CodiciIncompleto() string {
	var c []string
	for _, m := range v.MotiviIncompleto() {
		c = append(c, m.Codice)
	}
	return strings.Join(c, " ")
}

// ProdottoNonTrovato: il prodotto chiesto, quando c'è un esito e il servizio non l'ha preso (non è fra i prodotti
// valutati: ProdottoScelto ha dato il primo); "" altrimenti.
func (v paginaMotoreAVista) ProdottoNonTrovato() string {
	if v.Pagina == nil || v.Esito == nil || v.Scelta.Prodotto == "" || v.Prodotto == v.Scelta.Prodotto {
		return ""
	}
	return v.Scelta.Prodotto
}

// VistaNonValida: la vista chiesta, quando non è una delle tre (VistaDa ha dato distinta); "" altrimenti.
func (v paginaMotoreAVista) VistaNonValida() string {
	if v.Scelta.Vista == "" || motorea_web.VistaDa(v.Scelta.Vista) == v.Scelta.Vista {
		return ""
	}
	return v.Scelta.Vista
}

// RiletturaSenzaScelta: la rilettura htmx di #ma non ha portato né vista né prodotto (né nella query né in
// HX-Current-URL): la pagina mostra la scelta predefinita e lo dice.
func (v paginaMotoreAVista) RiletturaSenzaScelta() bool {
	return v.Scelta.Rilettura && v.Scelta.Sorgente == sorgenteNessuna
}

// postiP4: i dati della pagina che arrivano con i blocchi B2-B4 del piano esecutivo, per Rif del prodotto. Oggi sono
// sempre vuoti (postiP4Da): il template ha già il posto e, senza dati, non mostra niente. Nessuno di questi valori si
// calcola qui o nel template: li dà il server quando i blocchi ci sono.
//   - TipoVisualizzato: il badge «finito mono_pezzo», distinto da «finito con N figli» (contratto rev. 3, E2.11:
//     tipo_visualizzato derivato dall'ultima conferma valida, scelta [T] consigliata dalla direttiva e non ratificata;
//     B3.6). Assente = nessun badge. PropostoInBozza è l'etichetta separata della dichiarazione nella bozza (D-T-12 [T]);
//   - AvvisiStepSecondario: l'avviso STEP_SECONDARIO_DISPONIBILE nella conferma della fonte (D-SHOCK-S4 A [U]; codice
//     scelto dall'orchestratore, D-T-02 [T]; B4.5). Assente = niente;
//   - Blocchi: i blocchi del riepilogo R4 (B2.2, B2.4), fra cui CODICE_DA_ASSEGNARE per i nodi creati senza codice
//     (D-SHOCK-S3 C [U], E2.3), con i nodi «(senza codice)». Assente = niente. Stanno nel gruppo della distinta in
//     bozza (formato 2), non accanto alla conferma formato 1, che ha un riepilogo suo (R-07 della revisione di B5.1);
//   - AvvisiR4: gli avvisi del riepilogo R4, che non bloccano (PRODOTTO_SENZA_FIGLI, VERIFICA_DOPO_COMMIT,
//     STEP_SECONDARIO_DISPONIBILE anche qui, D-T-02 [T]), accanto ai blocchi (R-14 della revisione). Assente = niente.
type postiP4 struct {
	TipoVisualizzato     map[string]tipoVisualizzatoP4
	AvvisiStepSecondario map[string][]avvisoP4
	Blocchi              map[string][]bloccoP4
	AvvisiR4             map[string][]avvisoP4
}

// I valori di tipo_visualizzato e i codici che il template riconosce. Sono [P]: il backend (B3.6, B2.2, B4.5) li fissa;
// se cambiano, cambiano qui e nella prova dei posti. R-04 della revisione di B5.1: il contratto rev. 3 (E2.11) nomina
// solo «finito mono_pezzo» = tipo_visualizzato, senza fissare la grafia del valore (la direttiva 11 e D-T-12 scrivono
// mono_pezzo, il piano B3.6 "finito mono_pezzo"), né con_figli, né da dove viene N. Finché il contratto non li fissa,
// la pagina riconosce solo queste costanti e dice «non riconosciuto» per ogni altro valore, compreso "finito mono_pezzo":
// mai un badge indovinato.
const (
	tipoVisualizzatoMonoPezzo = "mono_pezzo"
	tipoVisualizzatoConFigli  = "con_figli"
	bloccoCodiceDaAssegnare   = "CODICE_DA_ASSEGNARE"
	avvisoStepSecondario      = "STEP_SECONDARIO_DISPONIBILE"
)

// tipoVisualizzatoP4: il tipo visualizzato di un finito, come lo darà il server. Motivo e Conferma sono il posto della
// precisazione [U] della direttiva 11 (D-P4A-02: mono_pezzo «persistita, motivata, visibile nella UI e riferita al
// prodotto»): il motivo della dichiarazione si scrive accanto al badge, il numero della conferma nel titolo. Vuoti =
// niente; nomi e forma li fissa il contratto (R-04 della revisione di B5.1), qui sono [P].
type tipoVisualizzatoP4 struct {
	Valore          string // tipoVisualizzatoMonoPezzo | tipoVisualizzatoConFigli; "" = nessun badge
	Figli           int    // con tipoVisualizzatoConFigli: i figli del finito nell'ultima conferma valida, dal server
	PropostoInBozza bool   // la dichiarazione mono_pezzo sta nella bozza aperta: proposta, non ufficiale (D-T-12)
	Motivo          string // il motivo della dichiarazione mono_pezzo, dal server; "" = non scritto
	Conferma        int    // il numero della conferma valida da cui viene il valore, dal server; 0 = non dato
}

// avvisoP4: un avviso del server (codice e testo), con il file che lo riguarda se c'è.
type avvisoP4 struct {
	Codice   string
	Testo    string
	Allegato *uuid.UUID
	NomeFile string
}

// bloccoP4: un blocco del riepilogo (codice e testo del server) e i nodi che lo causano.
type bloccoP4 struct {
	Codice string
	Testo  string
	Nodi   []nodoP4
}

// nodoP4: un nodo della bozza nominato da un blocco: la chiave della bozza e il codice ("" = «(senza codice)»).
type nodoP4 struct {
	Chiave string
	Codice string
}

// badgeP4: il badge del tipo visualizzato, pronto per il template.
type badgeP4 struct {
	Testo, Tono, Titolo string
}

// postiP4Da: i posti di P4 della pagina. Oggi vuoti: nessuna rotta di P4 e nessun campo del sottopackage li porta
// ancora (B2-B4). Quando B3.6 aggiunge tipo_visualizzato al DTO della pagina, o B2.4 il riepilogo R4, li copia qui.
func postiP4Da(*motorea_web.Pagina) postiP4 { return postiP4{} }

// Badge: il badge del tipo visualizzato del prodotto rif; nil senza un valore (nessun badge, mai un default). La sola
// traduzione è da valore a parole, come etichetta: niente si deduce dai figli dell'albero.
func (p postiP4) Badge(rif string) *badgeP4 {
	tv, ok := p.TipoVisualizzato[rif]
	if !ok {
		return nil
	}
	titolo := "tipo_visualizzato = " + tv.Valore + ", dall'ultima conferma valida della BOM"
	if tv.Conferma > 0 {
		titolo = fmt.Sprintf("tipo_visualizzato = %s, dalla conferma n. %d della BOM (l'ultima valida)", tv.Valore, tv.Conferma)
	}
	switch tv.Valore {
	case "":
		if tv.PropostoInBozza {
			return &badgeP4{Testo: "mono_pezzo proposto in bozza", Tono: "prop",
				Titolo: "la dichiarazione mono_pezzo è nella bozza aperta: non è ancora ufficiale"}
		}
		return nil
	case tipoVisualizzatoMonoPezzo:
		return &badgeP4{Testo: "finito mono_pezzo", Tono: "ok", Titolo: titolo}
	case tipoVisualizzatoConFigli:
		testo := fmt.Sprintf("finito con %d figli", tv.Figli)
		if tv.Figli == 1 {
			testo = "finito con 1 figlio"
		}
		return &badgeP4{Testo: testo, Tono: "neu", Titolo: titolo}
	default:
		return &badgeP4{Testo: "tipo visualizzato «" + tv.Valore + "» non riconosciuto dalla pagina", Tono: "ver", Titolo: titolo}
	}
}

// InBozza: la dichiarazione mono_pezzo del prodotto sta nella bozza aperta (D-T-12), accanto al badge ufficiale.
func (p postiP4) InBozza(rif string) bool {
	tv, ok := p.TipoVisualizzato[rif]
	return ok && tv.Valore != "" && tv.PropostoInBozza
}

// DaAssegnare: il blocco CODICE_DA_ASSEGNARE è uno di quelli del prodotto.
func (b bloccoP4) DaAssegnare() bool { return b.Codice == bloccoCodiceDaAssegnare }

// StepSecondario: l'avviso è STEP_SECONDARIO_DISPONIBILE.
func (a avvisoP4) StepSecondario() bool { return a.Codice == avvisoStepSecondario }

// ---------------------------------------------------------------- la resa

// rendiMotoreA: la pagina con i posti di P4 dati dal server (oggi vuoti) e la scelta della richiesta. Una pagina senza
// esito arrivata qui (le prove dei casi) ha la tipologia erroreNonDichiarato: il gestore dà la sua (vistaMotoreADa).
func (s *Server) rendiMotoreA(w http.ResponseWriter, r *http.Request, p *motorea_web.Pagina) {
	s.rendiVistaMotoreA(w, r, vistaMotoreADa(r, p, ""))
}

// rendiVistaMotoreA è rendi (server.go) in un buffer: la pagina intera («layout») o, con HX-Request senza
// HX-History-Restore-Request (frammentoRichiesto), «motore_a_corpo». Utente e Admin sempre; la testata (Stato) solo per
// la pagina intera, con un utente e con il database (senza Pool non si legge). Un errore di template, anche il panic di
// una ricerca dell'indice (text/template lo restituisce come errore), è un 500 testuale, mai una pagina a metà: le
// intestazioni e i byte si scrivono solo dopo l'esecuzione riuscita.
func (s *Server) rendiVistaMotoreA(w http.ResponseWriter, r *http.Request, pv paginaMotoreAVista) {
	t, err := s.templateMotoreA()
	if err != nil {
		s.Log.Error("motore A: il template della pagina non si compone", "err", err)
		http.Error(w, "pagina non disponibile", http.StatusInternalServerError)
		return
	}
	// il titolo non è quello della Distinta legacy (distinta.go), che resta in uso: R-11 (d) della revisione di B5.1
	v := vista{Utente: utenteDa(r.Context()), Titolo: titoloPaginaMotoreA, Dati: pv, Frammento: frammentoRichiesto(r)}
	v.Admin = almeno(v.Utente, db.RuoloUtenteAdmin)
	if !v.Frammento && v.Utente != nil && s.Pool != nil {
		v.Stato = s.stato(r.Context(), sessioneDa(r.Context()))
	}
	nome := "layout"
	if v.Frammento {
		nome = "motore_a_corpo"
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, nome, v); err != nil {
		s.Log.Error("template", "pagina", "motore_a.html", "err", err)
		http.Error(w, "errore nel disegnare la pagina", http.StatusInternalServerError)
		return
	}
	intestazioniPaginaMotoreA(w)
	_, _ = w.Write(buf.Bytes())
}

// funzioniMotoreA: le funzioni in più del template della pagina. etichetta (contratto A1d §2.3) è solo qui, non nella
// FuncMap di tutte le pagine: Init le dà al clone di motore_a.html (server.go; scelta [T] di B5.1, R-06).
var funzioniMotoreA = template.FuncMap{"etichetta": motorea_web.Etichetta}

// templateMotoreA: il template della pagina, composto da Init (layout.html + frammenti.html + motore_a.html, con le
// funzioni di tutte le pagine e funzioniMotoreA). Senza Init (un Server costruito a mano) è un errore, mai un panic.
func (s *Server) templateMotoreA() (*template.Template, error) {
	if t := s.pagine["motore_a.html"]; t != nil {
		return t, nil
	}
	return nil, errors.New("il template motore_a.html non è composto: Init non è stato chiamato")
}
