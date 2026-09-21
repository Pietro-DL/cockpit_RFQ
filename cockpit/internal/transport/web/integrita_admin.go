package web

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/platform/db"
)

// INTEGRITÀ NAS (blocco 5B del checkpoint 3R).
//
// La schermata risponde a una domanda che prima non si poteva fare: «il fascicolo dice la verità?».
// `stato_nas = 'scritto'` era una promessa fatta una volta sola, nel momento in cui la copia riusciva,
// e da allora mai più verificata — e `in_coda` non aveva una scadenza, quindi un documento fermo da
// tre settimane era indistinguibile da uno confermato cinque minuti fa.
//
// È una schermata da amministratore per lo stesso motivo dell'Anagrafica: qui si guarda il NAS di
// TUTTI, non il fascicolo di una richiesta. Chi lavora su una RFQ vede il proprio pezzo nella pagina
// della RFQ.
//
// Le azioni sono due, e una terza manca di proposito:
//
//	Riaccoda   in attesa / errore / mancante — rimette in coda la copia. Sicuro: il copiatore
//	           verifica l'hash e non sovrascrive mai un file diverso;
//	Allinea    già presente — il file è lì e ha l'hash giusto, quindi non c'è NIENTE da copiare:
//	           si allinea il database al disco, dopo aver ricalcolato l'hash sul momento;
//	(niente)   conflitto — sul NAS c'è un file diverso. Non esiste un pulsante che decida per una
//	           persona quale dei due sia quello buono, e sovrascrivere vorrebbe dire buttare via il
//	           file di qualcun altro senza sapere di chi fosse.
type integritaDati struct {
	Radice          string
	Raggiungibile   bool
	Scrittura       bool
	Automatico      time.Duration // 0 = nessuna passata automatica
	Lotto           int           // quanti documenti guarda UNA passata
	Documenti       int32
	Verificati      int32
	UltimoControllo *time.Time
	Conta           []db.ContaAnomalieNasRow
	Righe           []rigaIntegrita
	Avviso          string
	Errore          bool
}

// rigaIntegrita è una riga della tabella. I due metodi traducono il problema in due colonne che
// l'operatore può leggere senza sapere come sono fatti gli enum: che cosa c'è sul disco, e che cosa
// si può fare.
type rigaIntegrita struct {
	db.ListAnomalieNasRow
}

func (r rigaIntegrita) StatoFilesystem() string {
	switch r.NasAnomalium.Problema {
	case db.ProblemaNasGiaPresente:
		return "presente, hash corretto"
	case db.ProblemaNasConflitto:
		return "presente, contenuto DIVERSO"
	case db.ProblemaNasIlleggibile:
		return "presente, non leggibile"
	}
	return "assente"
}

// Azione è il nome del pulsante, oppure "" quando non esiste nessuna azione sicura.
func (r rigaIntegrita) Azione() string {
	switch r.NasAnomalium.Problema {
	case db.ProblemaNasInAttesa, db.ProblemaNasErrore, db.ProblemaNasMancante:
		return "riaccoda"
	case db.ProblemaNasGiaPresente:
		return "allinea"
	}
	return ""
}

// OgniQuanto e' l'intervallo del controllo automatico come lo legge una persona: «15 minuti», non
// «15m0s».
func (d *integritaDati) OgniQuanto() string {
	m := int(d.Automatico.Minutes())
	switch {
	case m >= 120:
		return fmt.Sprintf("%d ore", m/60)
	case m >= 2:
		return fmt.Sprintf("%d minuti", m)
	}
	return fmt.Sprintf("%d secondi", int(d.Automatico.Seconds()))
}

func (s *Server) adminIntegrita(w http.ResponseWriter, r *http.Request) {
	d, err := s.datiIntegrita(r)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.rendi(w, r, "integrita.html", "integrita_corpo", "Integrità NAS", d)
}

func (s *Server) datiIntegrita(r *http.Request) (*integritaDati, error) {
	ctx := r.Context()
	q := db.New(s.Pool)
	d := &integritaDati{Scrittura: jobs.CapacitaAttuali().NasScrittura}
	if s.NAS != nil {
		d.Radice = s.NAS.Radice
		d.Raggiungibile = s.NAS.Raggiungibile()
	}
	if s.Ricognitore != nil {
		d.Automatico = s.Ricognitore.Ogni
		d.Lotto = s.Ricognitore.PerPassata()
	}
	st, err := q.StatoIntegritaNas(ctx)
	if err != nil {
		return nil, err
	}
	d.Documenti, d.Verificati = st.Documenti, st.Verificati
	if t, err := q.UltimoControlloNas(ctx); err == nil {
		d.UltimoControllo = &t
	}
	d.Conta, _ = q.ContaAnomalieNas(ctx)
	righe, err := q.ListAnomalieNas(ctx)
	if err != nil {
		return nil, err
	}
	for _, x := range righe {
		d.Righe = append(d.Righe, rigaIntegrita{x})
	}
	return d, nil
}

// integritaFrammento ridisegna la schermata dopo un'azione. La tabella intera e non la sola riga:
// un'azione su un documento cambia i conteggi in cima, e due numeri che non corrispondono più alla
// tabella sotto sono peggio di nessun numero.
func (s *Server) integritaFrammento(w http.ResponseWriter, r *http.Request, avviso string, errore bool) {
	d, err := s.datiIntegrita(r)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	d.Avviso, d.Errore = avviso, errore
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	v := vista{Utente: utenteDa(r.Context()), Titolo: "Integrità NAS", Dati: d, Frammento: true}
	v.Admin = true
	if err := s.pagine["integrita.html"].ExecuteTemplate(w, "integrita_corpo", v); err != nil {
		s.Log.Error("template", "frammento", "integrita_corpo", "err", err)
	}
}

// controllaIntegrita è «Controlla ora»: la stessa passata del giro periodico, chiesta da una persona.
// Funziona anche con [nas].intervallo_integrita_s = 0, perché spegnere il controllo automatico non
// vuol dire rinunciare a guardare, e perché leggere il NAS è sempre consentito.
func (s *Server) controllaIntegrita(w http.ResponseWriter, r *http.Request) {
	if s.Ricognitore == nil {
		s.integritaFrammento(w, r, "Il ricognitore dell'integrità non è configurato su questo server.", true)
		return
	}
	e, err := s.Ricognitore.Giro(r.Context())
	if err != nil {
		s.Log.Error("controlla integrità NAS", "err", err)
		s.integritaFrammento(w, r, "Controllo non riuscito: "+err.Error(), true)
		return
	}
	if e.Saltato != "" {
		s.integritaFrammento(w, r, "NON controllato: "+e.Saltato, true)
		return
	}
	s.Log.Info("integrità NAS controllata a richiesta", "utente", utenteDa(r.Context()).Sigla,
		"guardati", e.Guardati, "segnalati", e.Aperte, "rientrati", e.Chiuse)
	s.integritaFrammento(w, r, frasePassata(e), false)
}

// frasePassata dice che cosa ha fatto la passata. «Controllati 12 documenti» e «non ho guardato
// niente» sono due risposte diverse, e una schermata vuota non le distingue.
func frasePassata(e jobs.EsitoRicognizione) string {
	switch {
	case e.Guardati == 0:
		return "Nessun documento da controllare: non ce ne sono di confermati."
	case e.Aperte == 0 && e.Chiuse == 0:
		return plurale2(e.Guardati, "Controllato 1 documento", "Controllati %d documenti") + ": niente da segnalare."
	case e.Aperte == 0:
		return plurale2(e.Guardati, "Controllato 1 documento", "Controllati %d documenti") +
			": " + plurale2(e.Chiuse, "1 segnalazione rientrata", "%d segnalazioni rientrate") + "."
	}
	f := plurale2(e.Guardati, "Controllato 1 documento", "Controllati %d documenti") +
		": " + plurale2(e.Aperte, "1 da guardare", "%d da guardare")
	if e.Chiuse > 0 {
		f += ", " + plurale2(e.Chiuse, "1 rientrata", "%d rientrate")
	}
	return f + "."
}

func plurale2(n int, uno, molti string) string {
	if n == 1 {
		return uno
	}
	return fmt.Sprintf(molti, n)
}

// riaccodaDocumento rimette in coda la copia di UN documento. Rispetta `nas_scrittura` come tutte le
// altre strade: l'accodamento rifiuta, e l'avviso nomina la riga da cambiare.
func (s *Server) riaccodaDocumento(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	ctx := r.Context()
	q := db.New(s.Pool)
	a, err := q.GetAnomaliaNas(ctx, id)
	if err != nil {
		s.integritaFrammento(w, r, "Segnalazione non trovata: forse è già rientrata.", true)
		return
	}
	// Un conflitto NON si riaccoda da qui. Il copiatore rifiuterebbe comunque di sovrascrivere — la
	// garanzia vera sta lì — ma offrire l'azione vorrebbe dire far premere un pulsante che non può
	// funzionare, e poi spiegare perché.
	if a.Problema == db.ProblemaNasConflitto || a.Problema == db.ProblemaNasIlleggibile {
		s.integritaFrammento(w, r, "Non si riaccoda: sul NAS c'è un file che non è questo documento. "+
			"Va guardato da una persona — il Cockpit non lo sovrascrive.", true)
		return
	}
	j, err := jobs.AccodaCopia(ctx, q, id)
	switch {
	case errors.Is(err, jobs.ErrCapacitaSpenta):
		s.integritaFrammento(w, r, "Copia NON rimessa in coda: la capacità [sicurezza]."+
			jobs.CapacitaMancante(err)+" è spenta su questo server.", true)
	case err != nil:
		s.Log.Error("riaccoda documento", "documento", id, "err", err)
		s.integritaFrammento(w, r, "Non riuscita: "+err.Error(), true)
	case j == nil:
		s.integritaFrammento(w, r, "Era già in coda: non se ne accoda una seconda.", false)
	default:
		s.Log.Info("copia NAS rimessa in coda dall'Admin", "documento", id, "utente", utenteDa(ctx).Sigla)
		s.integritaFrammento(w, r, "Copia rimessa in coda.", false)
	}
}

// allineaDocumento è il caso «il file è già lì ed è quello giusto»: non c'è niente da copiare, e il
// database è rimasto indietro rispetto al disco. L'hash viene ricalcolato ADESSO, sul file vero: se
// nel frattempo è cambiato, non si allinea niente.
func (s *Server) allineaDocumento(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	ctx := r.Context()
	q := db.New(s.Pool)
	a, err := q.GetAnomaliaNas(ctx, id)
	if err != nil {
		s.integritaFrammento(w, r, "Segnalazione non trovata: forse è già rientrata.", true)
		return
	}
	if a.Problema != db.ProblemaNasGiaPresente {
		s.integritaFrammento(w, r, "Non si allinea: questo documento non ha un file già corretto sul NAS.", true)
		return
	}
	if err := jobs.AllineaDocumento(ctx, q, s.NAS, id); err != nil {
		s.Log.Warn("allinea documento", "documento", id, "err", err)
		s.integritaFrammento(w, r, "Non allineato: "+err.Error(), true)
		return
	}
	s.Log.Info("documento allineato al file gia' sul NAS", "documento", id, "utente", utenteDa(ctx).Sigla)
	s.integritaFrammento(w, r, "Allineato: il file era già sul NAS con l'hash giusto, non è stato copiato niente.", false)
}
