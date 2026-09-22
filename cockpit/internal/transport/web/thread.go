package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// Schermata B — il thread RFQ: la "chat" (tutti i messaggi agganciati, dal DB), gli allegati da smistare,
// i documenti confermati sul NAS e il fascicolo calcolato da v_fascicolo.

type threadDati struct {
	T              db.ThreadOfferta
	Riga           db.VCruscotto
	Cliente        db.Cliente
	Identificativi []db.IdentificativoThread
	Messaggi       []messaggioThread
	Documenti      []db.Documento
	Fascicolo      []db.VFascicolo
	Bozze          []db.Bozza
	Componenti     []db.Componente
	// NDaSmistare: proposte ancora aperte della RFQ, letto da v_cruscotto.n_da_smistare (vedi caricaThread).
	NDaSmistare int
	// NDaCopiare: documenti confermati che NON sono sul NAS — in attesa oppure in errore.
	//
	// «In attesa» e' la conseguenza normale di una conferma data mentre [sicurezza].nas_scrittura era
	// spenta: la decisione dell'operatore e' registrata, la scrittura aspetta. «In errore» e' la copia
	// che ci ha provato e non ce l'ha fatta — il NAS non c'era, il contenuto era sparito dallo staging.
	//
	// Contano insieme perche' per chi guarda sono la stessa cosa — quel disegno non e' nel fascicolo —
	// e perche' un documento in errore senza un modo di riprovare sarebbe il secondo vicolo cieco dopo
	// quello che questo pulsante e' nato per togliere.
	NDaCopiare int
	// NAnomalie: segnalazioni aperte dell'integrita' NAS su questa RFQ (blocco 5B).
	//
	// Un documento dichiarato «scritto» il cui file non c'e' piu' e' l'unica riga della tabella qui
	// sopra che MENTE, e mente in un modo che non si vede: lo stato dice scritto perche' la copia era
	// riuscita, una volta. Chi aspetta quel disegno guarda questa pagina, non l'Admin — quindi la
	// notizia deve arrivare qui, anche se il posto in cui la si risolve e' un altro.
	NAnomalie int
	Admin     bool
	Avviso    string
	Selezion  string
	// Blocco 7B: le richieste ai fornitori di questa RFQ, e le tendine per crearne una: i
	// fornitori attivi (con le lavorazioni per cui il cliente li ha qualificati) e le lavorazioni.
	Richieste      []db.ListRichiesteThreadRow
	Fornitori      []db.Fornitore
	Lavorazioni    []db.Lavorazione
	QualificatoPer map[uuid.UUID]string
}

type messaggioThread struct {
	M db.Messaggio
	// Copia: quella servita dalla postazione della sessione; nil = «Apri» non disponibile, e Motivo
	// dice perché (messaggio non Outlook, nessuna postazione, nessun worker idoneo).
	Copia    *coda.Copia
	Motivo   string
	Allegati []AllegatoUI
}

func (s *Server) thread(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	d, err := s.caricaThread(r.Context(), id, sessioneDa(r.Context()))
	if err != nil {
		http.Error(w, "thread non trovato", 404)
		return
	}
	d.Selezion = r.URL.Query().Get("sel")
	s.rendi(w, r, "thread.html", "thread_corpo", "RFQ", d)
}

// threadFrammento ri-renderizza il corpo della pagina thread dopo un'azione (conferma, scarta, download).
func (s *Server) threadFrammento(w http.ResponseWriter, r *http.Request, id uuid.UUID, avviso string) {
	d, err := s.caricaThread(r.Context(), id, sessioneDa(r.Context()))
	if err != nil {
		http.Error(w, "thread non trovato", 404)
		return
	}
	d.Avviso = avviso
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.pagine["thread.html"].ExecuteTemplate(w, "thread_corpo", vista{Utente: utenteDa(r.Context()), Titolo: "RFQ", Dati: d, Frammento: true}); err != nil {
		s.Log.Error("template", "frammento", "thread_corpo", "err", err)
	}
}

// riprovaCopie rimette in coda la copia sul NAS dei documenti di questa RFQ che sono rimasti
// «in_coda» (checkpoint 3R, punto 6: uno stato incompleto deve potersi riconciliare con un'azione
// VISIBILE).
//
// E' il gesto di una persona, ed e' voluto che lo sia. Quando una capacita' si accende, i job che
// avevano aspettato vengono ANNULLATI e non eseguiti — nulla si mette in moto da solo perche'
// qualcuno ha cambiato una riga in un file. Quei documenti restano pero' confermati, e senza questo
// pulsante non esisteva nessun modo di portarli sul NAS: la conferma andava rifatta, cioe' la stessa
// decisione presa due volte, oppure il file non ci arrivava mai.
//
// Se la capacita' e' ancora spenta non si finge niente: l'accodamento rifiuta, e l'avviso lo dice con
// il nome della capacita'.
func (s *Server) riprovaCopie(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", 400)
		return
	}
	ctx := r.Context()
	q := db.New(s.Pool)
	documenti, err := q.ListDocumentiThread(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	e := s.rimettiInCoda(ctx, q, id, documenti)
	s.threadFrammento(w, r, id, e.Frase())
}

// rimettiInCoda accoda la copia dei documenti che ne hanno bisogno e CONTA che cosa e' successo a
// ciascuno. Non si ferma al primo intoppo.
//
// Un singolo documento storto non deve poter fermare gli altri: e' lo stesso principio per cui un
// elemento anomalo di Outlook non ferma la sincronizzazione della cartella. Prima, un errore del
// database su un documento faceva uscire l'intera azione con un 500, e gli altri documenti della
// stessa RFQ — che non avevano niente che non andasse — restavano fermi senza che nessuno lo
// dicesse. Adesso quel documento diventa un numero nella frase, e gli altri partono.
func (s *Server) rimettiInCoda(ctx context.Context, q *db.Queries, thread uuid.UUID, documenti []db.Documento) esitoCopie {
	var e esitoCopie
	for _, d := range documenti {
		if !daCopiare(d.StatoNas) {
			// gia' sul NAS: non c'e' niente da rimettere in coda, e non e' un errore
			e.NonApplicabili++
			continue
		}
		j, err := coda.AccodaCopia(ctx, q, d.DocumentoID)
		switch {
		case errors.Is(err, coda.ErrCapacitaSpenta):
			// la capacita' e' del server, non del documento: vale per tutti, e il giro finisce qui
			e.Spenta = coda.CapacitaMancante(err)
			return e
		case err != nil:
			s.Log.Error("riprova copie", "thread", thread, "documento", d.DocumentoID, "err", err)
			e.Errori++
			if e.Motivo == "" {
				e.Motivo = err.Error()
			}
		case j == nil:
			e.GiaInCoda++ // la copia di questo documento era gia' in coda: non se ne accoda una seconda
		default:
			e.Accodati++
		}
	}
	return e
}

// esitoCopie e' che cosa e' successo a ogni documento guardato. I quattro numeri non sono un
// dettaglio dell'implementazione: sono la differenza fra «ho premuto e non e' successo niente» e
// «c'erano tre documenti, due erano gia' in coda e uno non si e' potuto accodare per questo motivo».
type esitoCopie struct {
	Accodati       int    // copie nuove entrate in coda adesso
	GiaInCoda      int    // c'era gia' una copia pendente con la stessa chiave: non se ne fa una seconda
	Errori         int    // l'accodamento di quel documento non e' riuscito; gli altri sono andati avanti
	NonApplicabili int    // documenti gia' sul NAS: niente da rimettere in coda
	Spenta         string // capacita' spenta: il giro non e' nemmeno cominciato
	Motivo         string // il primo errore, per intero, cosi' non va cercato nel log
}

// daCopiare: un documento confermato che non e' sul NAS. In attesa perche' la scrittura era spenta,
// oppure in errore perche' la copia non e' riuscita: in tutti e due i casi il file non c'e' e
// qualcuno lo sta aspettando.
func daCopiare(s db.StatoNas) bool {
	return s == db.StatoNasInCoda || s == db.StatoNasErrore
}

// Frase e' quello che legge l'operatore: dice che cosa e' successo, non solo che l'azione e'
// riuscita. «Niente da rimettere in coda» e «due copie accodate» non sono la stessa notizia, e
// «una non si e' potuta accodare» e' una notizia che prima non esisteva affatto — l'azione usciva
// con un errore del server e le altre copie non partivano.
func (e esitoCopie) Frase() string {
	if e.Spenta != "" {
		return fmt.Sprintf("Copie NON rimesse in coda: la capacita' [sicurezza].%s e' spenta su questo server.", e.Spenta)
	}
	var coda []string
	if e.GiaInCoda > 0 {
		coda = append(coda, fmt.Sprintf("%d gia' in attesa", e.GiaInCoda))
	}
	if e.Errori > 0 {
		coda = append(coda, fmt.Sprintf("%d non accodat%s: %s", e.Errori, plurale(e.Errori, "o", "i"), e.Motivo))
	}
	if e.NonApplicabili > 0 {
		coda = append(coda, fmt.Sprintf("%d gia' sul NAS", e.NonApplicabili))
	}
	dettaglio := ""
	if len(coda) > 0 {
		dettaglio = " (" + strings.Join(coda, ", ") + ")"
	}
	switch {
	case e.Accodati == 0 && e.GiaInCoda == 0 && e.Errori == 0 && e.NonApplicabili == 0:
		return "Nessun documento in attesa: questa RFQ non ha documenti confermati."
	case e.Accodati == 0 && e.GiaInCoda == 0 && e.Errori == 0:
		return fmt.Sprintf("Nessun documento in attesa: sono tutti gia' sul NAS (%d).", e.NonApplicabili)
	case e.Accodati == 0:
		return "Nessuna copia nuova" + dettaglio + "."
	}
	return fmt.Sprintf("%d copi%s rimess%s in coda", e.Accodati, plurale(e.Accodati, "a", "e"),
		plurale(e.Accodati, "a", "e")) + dettaglio + "."
}

func plurale(n int, uno, molti string) string {
	if n == 1 {
		return uno
	}
	return molti
}

func (s *Server) caricaThread(ctx context.Context, id uuid.UUID, sess sessioneUI) (*threadDati, error) {
	q := db.New(s.Pool)
	t, err := q.GetThread(ctx, id)
	if err != nil {
		return nil, err
	}
	d := &threadDati{T: t}
	d.Admin = almeno(utenteDa(ctx), db.RuoloUtenteAdmin)
	if n, err := q.ContaAnomalieThread(ctx, id); err == nil {
		d.NAnomalie = int(n)
	}
	d.Riga, _ = q.GetCruscottoRiga(ctx, id)
	// «Da smistare» è UN numero solo, quello di v_cruscotto: le proposte ancora aperte di questa RFQ.
	//
	// Fino al blocco 8 questa pagina se lo contava da sé girando gli allegati dei messaggi, e il
	// conto veniva diverso da quello della lista Richieste — stessa etichetta, due numeri, e nessun
	// modo di sapere quale fosse quello buono. Il conto della vista è anche l'unico che vede una
	// proposta il cui allegato la pagina non mostra.
	d.NDaSmistare = int(d.Riga.NDaSmistare)
	d.Cliente, _ = q.GetCliente(ctx, t.ClienteID)
	d.Identificativi, _ = q.ListIdentificativi(ctx, id)
	d.Documenti, _ = q.ListDocumentiThread(ctx, id)
	for _, doc := range d.Documenti {
		if daCopiare(doc.StatoNas) {
			d.NDaCopiare++
		}
	}
	d.Fascicolo, _ = q.ListFascicolo(ctx, id)
	d.Bozze, _ = q.ListBozzeThread(ctx, uuid.NullUUID{UUID: id, Valid: true})
	d.Componenti, _ = q.ListComponentiThread(ctx, id)
	d.Richieste, _ = q.ListRichiesteThread(ctx, id)
	d.Fornitori, _ = q.ListFornitoriAttivi(ctx)
	d.Lavorazioni, _ = q.ListLavorazioni(ctx)
	d.QualificatoPer = map[uuid.UUID]string{}
	if qs, err := q.ListQualificheCliente(ctx, t.ClienteID); err == nil {
		for _, x := range qs {
			if d.QualificatoPer[x.FornitoreID] != "" {
				d.QualificatoPer[x.FornitoreID] += ", "
			}
			d.QualificatoPer[x.FornitoreID] += x.Lavorazione
		}
	}
	msgs, _ := q.ListMessaggiThread(ctx, uuid.NullUUID{UUID: id, Valid: true})
	for _, m := range msgs {
		mt := messaggioThread{M: m}
		if presenze, _ := q.ListPresenze(ctx, m.MessaggioID); len(presenze) > 0 {
			mt.Copia, mt.Motivo = s.copiaInterattiva(ctx, q, m.MessaggioID, sess)
		}
		mt.Allegati, _ = s.allegatiUI(ctx, q, m.MessaggioID)
		d.Messaggi = append(d.Messaggi, mt)
	}
	return d, nil
}
