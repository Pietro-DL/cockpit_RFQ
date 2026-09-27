package web

// La preparazione del Fascicolo e il suo avanzamento (B8.7b).
//
// Creare la RFQ accoda i download e apre la pagina subito: lo zip scende, si estrae, le voci si analizzano,
// e le proposte arrivano qualche secondo dopo. La prova dello zip dall'arrivo al Fascicolo (24/09/2026) lo
// ha mostrato: download, estrazione, cache per contenuto e analisi in due secondi, e la pagina aperta
// mezzo secondo dopo la creazione ferma a «0 file» finche' qualcuno non premeva F5. Il difetto non era lo
// zip: era la schermata, che non sapeva che c'era ancora lavoro in corso.
//
// Adesso la testata dice quanto lavoro c'e' (download, estrazioni, analisi) e, finche' ce n'e', chiede ogni
// pochi secondi se e' cambiato qualcosa. Se la firma della schermata e' cambiata rifa' i pannelli fuori banda
// (non il corpo dell'anteprima: il PDF aperto resta aperto); quando il lavoro finisce l'elemento torna senza
// il poll, e il browser smette di chiedere. Niente JavaScript oltre a htmx.
//
// Smistamento F1 (addendum A5.4.5, D52): nessuna GET scrive. La preparazione che l'apertura faceva da sola
// e' una POST (prepara), che la pagina manda al caricamento solo per chi scrive; accoda e basta.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// MaxPreparatiPerApertura: quante cose la preparazione mette in moto a ogni giro (la POST che la pagina
// manda aprendosi, il bottone, il triage), analisi degli STEP comprese. Le altre al giro dopo: la coda e'
// condivisa, e aprire una RFQ con cento allegati non deve riempirla.
const MaxPreparatiPerApertura = 20

// intervalliPoll sono le attese fra un controllo e l'altro, in secondi: all'inizio il lavoro finisce in
// pochi secondi, poi (un worker spento, una coda lunga) non serve chiedere cosi' spesso.
var intervalliPoll = []int{2, 2, 3, 3, 5, 5, 8, 10}

// maxCaricamentoEffettivo e' il limite degli upload: quello dei worker, che e' anche quello del caricamento
// interno. Un file piu' grande non scende da solo.
func (s *Server) maxCaricamentoEffettivo() int64 {
	if s.MaxCaricamento > 0 {
		return s.MaxCaricamento
	}
	return maxCaricamentoPredefinito
}

// prepara: POST /thread/{id}/fascicolo/prepara. La preparazione dei file della RFQ (B8.7b), che fino alla
// fase F1 girava dentro la GET del Fascicolo: i file utili scendono, gli archivi fermi si estraggono, i file
// fermi si analizzano, e un file fermo i cui fatti ci sono gia' (lo stage riusato: non ha mai prodotto una
// lettura) li riceve adesso; gli STEP senza i fatti dell'analizzatore corrente si accodano. Al piu'
// MaxPreparatiPerApertura cose, in tutto.
//
// Accoda soltanto (A5.4.5): non fa i prodotti della richiesta (li fa il triage) e non rilegge gli STEP gia'
// analizzati (lo fanno l'aggancio e «Rianalizza»). E' una POST, quindi chi consulta non la puo' mandare
// (autenticato: una regola per metodo); la pagina la manda da sola al caricamento solo a chi scrive, con
// auto=1, e ha il bottone «Prepara i file» di riserva, che va anche senza JavaScript.
//
// Risposte. Senza htmx (il bottone di riserva) si torna con un 303 alla pagina da cui si e' partiti
// (da=fascicolo, altrimenti la pagina della RFQ); se la preparazione non e' riuscita no: la pagina non ha
// dove dirlo, e tornarci in silenzio faceva credere partito un lavoro che non c'era. Si risponde con quella
// pagina intera e l'avviso (preparazioneNonRiuscita). Da htmx come ogni gesto: il Fascicolo con l'avviso e i
// pannelli fuori banda (l'avanzamento comincia a seguire il lavoro accodato), oppure la pagina della RFQ.
// Quella automatica non ha avviso; se non ha messo in moto niente, o non e' riuscita, o viene dalla pagina
// della RFQ, risponde 204 e la pagina resta com'e' (il log dice perche').
func (s *Server) prepara(w http.ResponseWriter, r *http.Request) {
	thread, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form non valido", 400)
		return
	}
	p, ri, err := s.preparaFile(r.Context(), thread)
	avviso := frasePreparazione(p, ri)
	partito := p.Qualcosa() || ri.Accodati > 0
	switch {
	case err != nil:
		s.Log.Warn("preparazione dei file non riuscita", "rfq", thread, "err", err)
		avviso = "Niente è cambiato: " + spiegaErrore(err)
	case partito || len(p.Saltati) > 0:
		s.Log.Info("file della RFQ preparati", "rfq", thread, "download", p.Download, "riusati", p.Riusati,
			"estrazioni", p.Estrazioni, "analisi", p.Analisi+ri.Accodati, "riletti", p.Riletti,
			"rimandati", p.Rimandati+ri.Rimandati, "saltati", p.Saltati)
	}
	if r.Header.Get("HX-Request") != "true" {
		torna := "/thread/" + thread.String()
		if r.FormValue("da") == "fascicolo" {
			torna += "/fascicolo"
		}
		if err != nil {
			s.preparazioneNonRiuscita(w, r, thread, r.FormValue("da") == "fascicolo", "Preparazione dei file non riuscita. "+avviso)
			return
		}
		http.Redirect(w, r, torna, http.StatusSeeOther)
		return
	}
	if r.FormValue("auto") != "" {
		// la pagina della RFQ la manda con hx-swap="none": il lavoro si segue nel Fascicolo, e rifarle il
		// corpo sarebbe una lettura buttata
		_, dalF := dalFascicolo(r.Header, thread)
		if err != nil || !partito || !dalF {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		avviso = ""
	}
	s.threadFrammento(w, r, thread, avviso)
}

// preparazioneNonRiuscita e' la risposta del bottone di riserva, senza htmx, quando la preparazione non e'
// riuscita: la pagina da cui si era partiti, intera (layout, foglio di stile, navigazione, come quando la si
// apre), con l'avviso al suo posto e lo stato 500. Un frammento nudo si leggeva, ma senza la pagina intorno e
// senza lo stile dell'avviso. Se nemmeno la pagina si legge resta il testo semplice.
func (s *Server) preparazioneNonRiuscita(w http.ResponseWriter, r *http.Request, thread uuid.UUID, daFascicolo bool, avviso string) {
	ctx := r.Context()
	var (
		pagina, frammento, titolo string
		dati                      any
		err                       error
	)
	if daFascicolo {
		var d *fascicoloDati
		if d, err = s.caricaFascicolo(ctx, thread, leggiStatoFascicolo(url.Values{}), utenteDa(ctx)); err == nil {
			d.Avviso = avviso
			pagina, frammento, titolo, dati = "fascicolo.html", "fasc_corpo", "Fascicolo", d
		}
	} else {
		var d *threadDati
		if d, err = s.caricaThread(ctx, thread, sessioneDa(ctx)); err == nil {
			d.Avviso = avviso
			pagina, frammento, titolo, dati = "thread.html", "thread_corpo", "RFQ", d
		}
	}
	if err != nil {
		s.Log.Warn("pagina della preparazione non riuscita non letta", "rfq", thread, "err", err)
		http.Error(w, avviso, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	s.rendi(w, r, pagina, frammento, titolo, dati)
}

// preparaFile e' la preparazione in una transazione sola: tutto o niente. Il limite vale per tutta la
// preparazione: le analisi degli STEP si accodano con quello che PreparaFile ha lasciato, e come quelle di
// PreparaFile non riprovano da sole un tentativo fallito (AccodaAnalisiMancantiDaSola).
func (s *Server) preparaFile(ctx context.Context, thread uuid.UUID) (fascicolo.Preparazione, fascicolo.Rianalisi, error) {
	var (
		p  fascicolo.Preparazione
		ri fascicolo.Rianalisi
	)
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return p, ri, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	p, err = fascicolo.PreparaFile(ctx, q, thread, s.Analizzatore, s.maxCaricamentoEffettivo(), MaxPreparatiPerApertura, s.rileggiFatti())
	if err != nil {
		return p, ri, err
	}
	if s.Analizzatore.Versione != 0 {
		if ri, err = fascicolo.AccodaAnalisiMancantiDaSola(ctx, q, thread, s.Analizzatore, max(0, MaxPreparatiPerApertura-p.Fatti())); err != nil {
			return p, ri, err
		}
	}
	return p, ri, tx.Commit(ctx)
}

// frasePreparazione e' l'avviso della preparazione chiesta con il bottone: che cosa e' partito, che cosa
// aspetta il giro dopo, che cosa non puo' partire da solo.
func frasePreparazione(p fascicolo.Preparazione, ri fascicolo.Rianalisi) string {
	var parti []string
	if p.Download > 0 {
		parti = append(parti, conta(p.Download, "download accodato", "download accodati"))
	}
	if p.Riusati > 0 {
		parti = append(parti, conta(p.Riusati, "file già nello staging", "file già nello staging"))
	}
	if p.Estrazioni > 0 {
		parti = append(parti, conta(p.Estrazioni, "archivio da estrarre", "archivi da estrarre"))
	}
	if n := p.Analisi + ri.Accodati; n > 0 {
		parti = append(parti, conta(n, "analisi accodata", "analisi accodate"))
	}
	if p.Riletti > 0 {
		parti = append(parti, conta(p.Riletti, "file fermo completato", "file fermi completati")+" con i fatti già calcolati")
	}
	if ri.GiaInCoda > 0 {
		parti = append(parti, conta(ri.GiaInCoda, "analisi già in coda", "analisi già in coda"))
	}
	if n := p.Rimandati + ri.Rimandati; n > 0 {
		parti = append(parti, conta(n, "file rimandato", "file rimandati")+" al prossimo giro")
	}
	if ri.SenzaStaging > 0 {
		parti = append(parti, conta(ri.SenzaStaging, "STEP non più in staging", "STEP non più in staging")+" (Riscarica)")
	}
	frase := "Niente da preparare: nessun file da scaricare, estrarre o analizzare."
	if len(parti) > 0 {
		frase = "Preparazione dei file: " + strings.Join(parti, ", ") + "."
	}
	if len(p.Saltati) > 0 {
		frase += " Non scaricati da soli: " + strings.Join(p.Saltati, "; ") + "."
	}
	return frase
}

// rileggiFatti e' la strada del workerapi per un file fermo i cui fatti ci sono gia'; nil se il server non
// la conosce (il caricamento interno non e' configurato).
func (s *Server) rileggiFatti() fascicolo.RiletturaFatti {
	if s.Pipeline == nil {
		return nil
	}
	return func(ctx context.Context, q *db.Queries, a uuid.UUID) error {
		return s.Pipeline.DopoCaricamento(ctx, q, a)
	}
}

// avanzamento e' il lavoro in corso sui file della RFQ, come lo mostra la testata, e il poll che lo segue.
type avanzamento struct {
	Base   string
	Stato  statoFascicolo
	Lavoro fascicolo.Lavoro
	Firma  string // la firma della schermata disegnata: il poll rifa' i pannelli solo se cambia
	Giro   int    // quanti controlli senza cambiamenti: allunga l'attesa
	Attesa int    // secondi fino al prossimo controllo
	Errore string // il lavoro non si e' potuto leggere
}

// Frase e' il riepilogo: «2 download · 1 estrazione · 4 analisi».
func (a avanzamento) Frase() string {
	var parti []string
	if n := a.Lavoro.Download; n > 0 {
		parti = append(parti, conta(n, "download", "download"))
	}
	if n := a.Lavoro.Estrazioni; n > 0 {
		parti = append(parti, conta(n, "estrazione", "estrazioni"))
	}
	if n := a.Lavoro.Analisi; n > 0 {
		parti = append(parti, conta(n, "analisi", "analisi"))
	}
	return strings.Join(parti, " · ")
}

// Poll e' il valore di hx-get del controllo successivo: la firma di quello che la pagina mostra e quanti
// controlli sono andati a vuoto. Lo stato della pagina lo porta HX-Current-URL.
func (a avanzamento) Poll() string {
	return a.Base + "/avanzamento?" + url.Values{"firma": {a.Firma}, "giro": {strconv.Itoa(a.Giro)}}.Encode()
}

func attesaDopo(giro int) int {
	if giro < 0 {
		giro = 0
	}
	if giro >= len(intervalliPoll) {
		return intervalliPoll[len(intervalliPoll)-1]
	}
	return intervalliPoll[giro]
}

// firmaDi riassume quello che la schermata mostra: se due letture hanno la stessa firma, rifare i pannelli
// non cambierebbe niente. Non entra lo stato della pagina (il nodo scelto, il filtro): quello e'
// dell'indirizzo, e lo porta la richiesta.
func firmaDi(d *fascicoloDati) string {
	h := sha256.New()
	fmt.Fprintf(h, "bom:%d:%d:%d|", d.Albero.Componenti(), d.NProposte, d.Bloccata)
	for _, f := range d.File {
		fmt.Fprintf(h, "f:%s:%s:%s:%s:%t|", f.A.AllegatoID, f.A.Stato, f.Stato, f.Tipo+"/"+f.Codice+"/"+f.Rev, f.Comp != nil)
	}
	// la completezza e' una mappa: si scorre in un ordine fisso, altrimenti gli stessi dati darebbero firme
	// diverse e ogni poll rifarebbe i pannelli (e ricomincerebbe dall'intervallo piu' breve)
	ids := make([]uuid.UUID, 0, len(d.Completezza))
	for id := range d.Completezza {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	for _, id := range ids {
		fmt.Fprintf(h, "c:%s:", id)
		for _, c := range d.Completezza[id] {
			fmt.Fprintf(h, "%s%s,", c.Etichetta, c.Simbolo)
		}
	}
	fmt.Fprintf(h, "|piano:%s:%d:%d:%d|lavoro:%d:%d:%d", d.Piano.Firma(), d.Piano.Pronte(), d.Piano.Decisioni(), d.Piano.InAttesa(),
		d.Lavoro.Download, d.Lavoro.Estrazioni, d.Lavoro.Analisi)
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// fascicoloAvanzamento: GET /thread/{id}/fascicolo/avanzamento?firma=…&giro=…, con lo stato della pagina.
// Risponde con l'elemento dell'avanzamento (che porta il prossimo poll solo se c'e' ancora lavoro) e, se la
// firma e' cambiata, con i pannelli fuori banda. Non scrive niente: il poll di una pagina aperta non mette in
// moto lavoro, lo guarda.
func (s *Server) fascicoloAvanzamento(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	par := r.URL.Query()
	// Lo stato e' quello della pagina ADESSO (HX-Current-URL): da quando il poll e' stato disegnato
	// l'operatore puo' aver scelto un altro nodo o un'altra linguetta, e rifare i pannelli con lo stato
	// vecchio lo riporterebbe indietro.
	st, ok := dalFascicolo(r.Header, id)
	if !ok {
		st = leggiStatoFascicolo(par)
	}
	d, err := s.caricaFascicolo(r.Context(), id, st, utenteDa(r.Context()))
	if err != nil {
		http.Error(w, "RFQ non trovata", 404)
		return
	}
	giro, _ := strconv.Atoi(par.Get("giro"))
	if par.Get("firma") == d.Avanzamento.Firma {
		giro++
	} else {
		giro = 0
		d.Rifai = true
	}
	d.Avanzamento.Giro, d.Avanzamento.Attesa = giro, attesaDopo(giro)
	// il corpo del pannello di destra, solo se la pagina ne mostra un altro: l'analisi del file aperto e'
	// arrivata, il 2D in arrivo e' sceso
	d.RifaiCorpo = par.Get("corpo_chiave") != d.ChiaveCorpo
	s.eseguiFascicolo(w, "fasc_avanzamento_risposta", d)
}
