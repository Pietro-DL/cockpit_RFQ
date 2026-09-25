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

// MaxPreparatiPerApertura: quanti file la preparazione mette in moto a ogni apertura del Fascicolo. Gli
// altri al giro dopo: la coda e' condivisa, e aprire una RFQ con cento allegati non deve riempirla.
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

// preparaFascicolo e' quello che il sistema fa da solo all'apertura del Fascicolo: i codici della
// richiesta diventano prodotti (se non lo sono gia'), i file utili scendono, quelli fermi si rimettono in
// moto, gli STEP si rileggono con le regole del cliente. Ognuna nella sua transazione: se una non riesce
// la pagina si apre lo stesso, e il log dice perche'.
func (s *Server) preparaFascicolo(ctx context.Context, thread uuid.UUID) {
	s.inTransazione(ctx, "prodotti della richiesta", thread, func(q *db.Queries) error {
		es, err := fascicolo.AssicuraProdottiDellaRichiesta(ctx, q, thread)
		if err == nil && len(es.Creati) > 0 {
			s.Log.Info("codici della richiesta diventati prodotti", "rfq", thread, "codici", es.Creati)
		}
		return err
	})
	s.inTransazione(ctx, "preparazione dei file", thread, func(q *db.Queries) error {
		p, err := fascicolo.PreparaFile(ctx, q, thread, s.Analizzatore, s.maxCaricamentoEffettivo(), MaxPreparatiPerApertura, s.rileggiFatti())
		if err == nil && (p.Qualcosa() || len(p.Saltati) > 0) {
			s.Log.Info("file della RFQ preparati", "rfq", thread, "download", p.Download, "riusati", p.Riusati,
				"estrazioni", p.Estrazioni, "analisi", p.Analisi, "riletti", p.Riletti, "rimandati", p.Rimandati, "saltati", p.Saltati)
		}
		return err
	})
	s.rileggiAllApertura(ctx, thread)
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

// inTransazione esegue fai in una transazione sua, e se non riesce lo scrive nel log.
func (s *Server) inTransazione(ctx context.Context, cosa string, thread uuid.UUID, fai func(q *db.Queries) error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		s.Log.Warn(cosa+" non riuscita", "rfq", thread, "err", err)
		return
	}
	defer tx.Rollback(ctx)
	if err := fai(db.New(tx)); err != nil {
		s.Log.Warn(cosa+" non riuscita", "rfq", thread, "err", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		s.Log.Warn(cosa+" non riuscita", "rfq", thread, "err", err)
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
