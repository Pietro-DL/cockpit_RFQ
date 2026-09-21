package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/domain"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
)

// RICONCILIAZIONE DEL NAS (blocco 5B del checkpoint 3R).
//
// `stato_nas = 'scritto'` e' una promessa fatta UNA VOLTA SOLA, nel momento in cui la copia e'
// riuscita, e da allora mai piu' verificata. Dopo quel momento la cartella puo' essere stata
// spostata, il file cancellato, oppure sostituito a mano da qualcun altro con un contenuto diverso:
// il fascicolo continuerebbe a dire di si', e chi si fida di quella riga scoprirebbe il contrario nel
// momento peggiore — quando il cliente chiede il disegno.
//
// E il contrario: un documento `in_coda` da tre settimane, perche' la scrittura era spenta o perche'
// la copia e' fallita e nessuno se n'e' accorto, guardando la riga non si distingue da uno confermato
// cinque minuti fa. E' il punto 6 del checkpoint: uno stato incompleto deve potersi riconciliare, e
// non puo' restare fermo per sempre senza un'azione visibile.
//
// # Che cosa NON fa
//
// Non ripara niente da solo, e in particolare NON SOVRASCRIVE MAI un file con un contenuto diverso.
// Quello e' un conflitto: vorrebbe dire buttare via il file di qualcun altro senza sapere di chi
// fosse ne' perche' fosse li'. Si segnala e si lascia decidere a una persona.
//
// Le uniche cose che scrive sono `documento.verificato_il` (quando ha guardato) e le righe di
// `nas_anomalia` (che cosa ha trovato). Legge il NAS — e leggere e' sempre consentito, anche con
// `nas_scrittura` spenta: un server che non scrive puo' benissimo accorgersi che manca un file.
type Ricognitore struct {
	Pool *pgxpool.Pool
	NAS  *nas.Scrittore
	Log  *slog.Logger
	// Ogni: intervallo fra una passata e l'altra. Zero o negativo = il ricognitore non parte
	// ([nas].intervallo_integrita_s = 0). «Controlla ora» dall'Admin funziona lo stesso: spegnere il
	// giro automatico non vuol dire rinunciare a guardare.
	Ogni time.Duration
	// Attesa: da quanto un documento deve essere fermo `in_coda` prima di essere segnalato. Zero =
	// mezz'ora. Senza questa soglia ogni conferma comparirebbe in elenco per i pochi secondi che
	// separano la decisione dalla copia, e un elenco che si riempie da solo e' un elenco che nessuno
	// guarda piu'.
	Attesa time.Duration
	// PerGiro: quanti documenti guardare per passata. Zero = 200. Il limite non e' prudenza
	// generica: ogni documento costa uno stat e la lettura INTERA del file per calcolarne l'hash, su
	// una condivisione di rete. Rileggere diecimila file ogni quarto d'ora sarebbe un modo molto
	// costoso di scoprire che va tutto bene.
	PerGiro int
	// Adesso: l'orologio. Nil = time.Now. E' un campo perche' la prova deve poter far invecchiare un
	// documento di tre giorni senza aspettarli.
	Adesso func() time.Time
}

// EsitoRicognizione e' che cosa ha fatto una passata.
type EsitoRicognizione struct {
	Guardati    int
	Aperte      int // anomalie nuove o aggiornate
	Chiuse      int // documenti che erano segnalati e adesso sono a posto
	PerProblema map[db.ProblemaNas]int
	// Saltato: se non e' vuoto, la passata NON e' stata fatta, e questo dice perche'. Non e' un
	// errore: e' la differenza fra «ho guardato e va tutto bene» e «non ho guardato».
	Saltato string
}

func (r *Ricognitore) attesa() time.Duration {
	if r.Attesa <= 0 {
		return 30 * time.Minute
	}
	return r.Attesa
}

// PerPassata e' quanti documenti guarda una passata, tenendo conto del predefinito. Esportata perche'
// la schermata lo DICE: un clic su «Controlla ora» che guarda duecento documenti su novemila e'
// un'altra cosa da un clic che li guarda tutti, e chi preme deve saperlo.
func (r *Ricognitore) PerPassata() int {
	if r.PerGiro <= 0 {
		return 200
	}
	return r.PerGiro
}

func (r *Ricognitore) adesso() time.Time {
	if r.Adesso == nil {
		return time.Now()
	}
	return r.Adesso()
}

// Avvia fa partire il giro periodico. La prima passata non e' immediata: all'avvio il server ha da
// fare cose piu' urgenti, e una condivisione di rete puo' non essere ancora montata.
func (r *Ricognitore) Avvia(ctx context.Context) {
	if r.Ogni <= 0 {
		r.Log.Warn("ricognizione dell'integrita' del NAS disattivata: nessuna passata automatica",
			"nota", "[nas].intervallo_integrita_s = 0; «Controlla ora» in Admin funziona lo stesso")
		return
	}
	primo := time.Minute
	if r.Ogni < primo {
		primo = r.Ogni
	}
	go func() {
		t := time.NewTimer(primo)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			e, err := r.Giro(ctx)
			if err != nil {
				r.Log.Error("ricognizione integrita' NAS", "err", err)
			} else {
				r.registra(e)
			}
			t.Reset(r.Ogni)
		}
	}()
}

func (r *Ricognitore) registra(e EsitoRicognizione) {
	if e.Saltato != "" {
		r.Log.Warn("integrita' NAS: passata saltata", "motivo", e.Saltato)
		return
	}
	if e.Aperte == 0 && e.Chiuse == 0 {
		r.Log.Debug("integrita' NAS: niente da segnalare", "guardati", e.Guardati)
		return
	}
	r.Log.Warn("integrita' NAS", "guardati", e.Guardati, "segnalati", e.Aperte, "rientrati", e.Chiuse,
		"per_problema", fmt.Sprint(e.PerProblema))
}

// Giro e' UNA passata: prende i documenti guardati meno di recente, li confronta con i file veri e
// scrive che cosa ha trovato. E' esportata perche' e' la stessa funzione che chiama «Controlla ora»
// in Admin e la stessa che provano i test: un controllo che in produzione e nelle prove passa da due
// strade diverse e' un controllo che in una delle due prima o poi si comporta in un altro modo.
func (r *Ricognitore) Giro(ctx context.Context) (EsitoRicognizione, error) {
	e := EsitoRicognizione{PerProblema: map[db.ProblemaNas]int{}}
	// Se il NAS non c'e', NON si guarda. Una passata fatta adesso direbbe che mancano tutti i file e
	// riempirebbe l'elenco di anomalie inventate da un cavo staccato: il danno non sarebbe il rumore,
	// sarebbe che la volta in cui manca un file davvero non si distinguerebbe piu' dalle altre.
	if !r.NAS.Raggiungibile() {
		e.Saltato = "il NAS non e' raggiungibile (" + r.NAS.Radice + "): una passata adesso direbbe che mancano tutti i file"
		return e, nil
	}
	q := db.New(r.Pool)
	righe, err := q.ListDocumentiDaVerificare(ctx, int32(r.PerPassata()))
	if err != nil {
		return e, err
	}
	pendenti, err := r.copiePendenti(ctx, q)
	if err != nil {
		return e, err
	}
	c := controllo{
		radice:    r.NAS.Radice,
		attesa:    r.attesa(),
		adesso:    r.adesso(),
		scrittura: CapacitaAttuali().NasScrittura,
	}
	for _, riga := range righe {
		d := riga.Documento
		c.inCoda = pendenti[ChiaveCopia(d.DocumentoID)]
		es := c.esamina(d, riga.CartellaRelativa)
		if es.Problema == "" {
			n, err := q.ChiudiAnomaliaNas(ctx, d.DocumentoID)
			if err != nil {
				return e, err
			}
			e.Chiuse += int(n)
		} else {
			if _, err := q.ApriAnomaliaNas(ctx, db.ApriAnomaliaNasParams{
				DocumentoID: d.DocumentoID, ThreadID: d.ThreadID, Problema: es.Problema,
				StatoDb: d.StatoNas, Percorso: es.Percorso, ShaAtteso: d.Sha256,
				ShaTrovato: ptesto(es.ShaTrovato), Dettaglio: es.Dettaglio,
			}); err != nil {
				return e, err
			}
			e.Aperte++
			e.PerProblema[es.Problema]++
		}
		if err := q.SetDocumentoVerificato(ctx, d.DocumentoID); err != nil {
			return e, err
		}
		e.Guardati++
	}
	return e, nil
}

func (r *Ricognitore) copiePendenti(ctx context.Context, q *db.Queries) (map[string]bool, error) {
	chiavi, err := q.ListCopieNasPendenti(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(chiavi))
	for _, c := range chiavi {
		m[c.String] = true
	}
	return m, nil
}

func ptesto(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

// controllo sono le condizioni in cui si guarda un documento: dove sta il NAS, che ora e', da quanto
// un'attesa diventa troppa, se questo server scrive, e se per questo documento c'e' gia' una copia in
// coda. Sono raggruppate perche' cambiano tutte insieme all'inizio della passata e nessuna cambia
// dentro.
type controllo struct {
	radice    string
	attesa    time.Duration
	adesso    time.Time
	scrittura bool // [sicurezza].nas_scrittura di questo server
	inCoda    bool // c'e' gia' una copia pendente per questo documento
}

// Esame e' il verdetto su un documento. Problema vuoto = niente da segnalare.
type Esame struct {
	Problema   db.ProblemaNas
	ShaTrovato string
	Dettaglio  string
	Percorso   string // relativo alla radice del NAS
}

// esamina confronta un documento con il file vero.
//
// L'ordine dei controlli non e' casuale: si guarda PRIMA il disco e poi il database, perche' il disco
// e' il fatto e il database e' l'opinione. Un documento che dice «scritto» con il file al suo posto e
// l'hash giusto non e' un problema; lo stesso documento senza il file lo e', e la differenza si vede
// solo guardando.
func (c controllo) esamina(d db.Documento, cartella pgtype.Text) Esame {
	relativo := percorsoDocumento(cartella.String, d.PathRelativo)
	e := Esame{Percorso: relativo}
	if !cartella.Valid || cartella.String == "" {
		// La RFQ non ha ancora una cartella sul NAS: non c'e' nessun posto in cui guardare. Non e' un
		// caso di laboratorio — succede se la creazione della cartella e' fallita o se la RFQ e' nata
		// mentre la scrittura era spenta — e senza questo ramo il documento resterebbe invisibile
		// proprio perche' non si sa dove cercarlo.
		e.Percorso = d.PathRelativo
		if d.StatoNas == db.StatoNasScritto {
			e.Problema, e.Dettaglio = db.ProblemaNasMancante, "il documento risulta scritto, ma la RFQ non ha una cartella sul NAS"
			return e
		}
		e.Problema = db.ProblemaNasInAttesa
		e.Dettaglio = "la RFQ non ha ancora una cartella sul NAS: la copia non ha una destinazione"
		return e
	}

	assoluto := domain.UNC(c.radice, relativo)
	st, err := os.Stat(assoluto)
	if err == nil && st.IsDir() {
		// Al posto del file c'e' una cartella con lo stesso nome. Non e' «manca il file» — il nome e'
		// occupato, e la copia non potrebbe scriverci nemmeno volendo — e non e' un conflitto, perche'
		// non c'e' nessun contenuto da confrontare con questo documento.
		e.Problema = db.ProblemaNasIlleggibile
		e.Dettaglio = "al posto del file, sul NAS c'e' una CARTELLA con lo stesso nome: la copia non puo' scriverci"
		return e
	}
	if err == nil {
		sha, _, err := nas.Sha256File(assoluto)
		switch {
		case err != nil:
			// Il file c'e' e non si e' riusciti a leggerlo. Non si puo' dire ne' che sia quello giusto
			// ne' che sia un altro: dire «conflitto» qui vorrebbe dire accusare qualcuno di aver
			// sostituito un file sulla base di un permesso negato.
			e.Problema, e.Dettaglio = db.ProblemaNasIlleggibile, "il file c'e' ma non si riesce a leggerlo: "+err.Error()
		case sha == d.Sha256:
			if d.StatoNas != db.StatoNasScritto {
				// Il file e' gia' li' ed e' byte per byte quello giusto: non c'e' NIENTE da copiare.
				// E' il caso in cui una copia sarebbe lavoro inutile, e il database e' semplicemente
				// rimasto indietro rispetto al disco.
				e.Problema = db.ProblemaNasGiaPresente
				e.ShaTrovato = sha
				e.Dettaglio = "il file e' gia' sul NAS con l'hash giusto: il documento risulta ancora da copiare"
			}
		default:
			// MAI sovrascrivere: si segnala e basta.
			e.Problema = db.ProblemaNasConflitto
			e.ShaTrovato = sha
			e.Dettaglio = "sul NAS c'e' un file diverso da questo documento. Non viene sovrascritto: " +
				"decidere quale dei due e' quello buono e' una cosa da fare guardandoli"
		}
		return e
	}

	// Il file non c'e'.
	switch {
	case d.StatoNas == db.StatoNasScritto:
		e.Problema = db.ProblemaNasMancante
		e.Dettaglio = "il documento risulta scritto sul NAS, ma il file non c'e' piu'"
	case c.inCoda:
		// La copia e' in coda: non e' un'anomalia, e' lavoro in corso. Metterlo in elenco vorrebbe
		// dire chiedere a una persona di occuparsi di qualcosa che il sistema sta gia' facendo.
	case d.StatoNas == db.StatoNasErrore:
		e.Problema = db.ProblemaNasErrore
		e.Dettaglio = "la copia e' stata tentata e non e' riuscita: " + d.ErroreNas.String
	case c.adesso.Sub(d.ConfermatoIl) > c.attesa:
		e.Problema = db.ProblemaNasInAttesa
		if !c.scrittura {
			e.Dettaglio = "in attesa da " + durata(c.adesso.Sub(d.ConfermatoIl)) +
				": la scrittura sul NAS e' spenta su questo server ([sicurezza].nas_scrittura)"
		} else {
			e.Dettaglio = "in attesa da " + durata(c.adesso.Sub(d.ConfermatoIl)) +
				": nessuna copia in coda per questo documento"
		}
	}
	return e
}

// percorsoDocumento e' il percorso del file relativo alla RADICE del NAS: quello che si mostra in
// Admin. La radice assoluta non ci sta dentro di proposito — cambia da un server all'altro, e una
// riga di anomalia deve poter essere letta da chiunque, anche da un'altra postazione.
func percorsoDocumento(cartellaThread, pathDoc string) string {
	if cartellaThread == "" {
		return pathDoc
	}
	return trimDestra(cartellaThread) + `\` + pathDoc
}

func trimDestra(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\\' || s[len(s)-1] == '/') {
		s = s[:len(s)-1]
	}
	return s
}

// durata e' «3 giorni», «5 ore», «40 minuti»: la frase la legge chi decide se e' tanto, e «72h13m»
// non e' una risposta a quella domanda.
func durata(d time.Duration) string {
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%d giorni", int(d.Hours()/24))
	case d >= 2*time.Hour:
		return fmt.Sprintf("%d ore", int(d.Hours()))
	case d >= 2*time.Minute:
		return fmt.Sprintf("%d minuti", int(d.Minutes()))
	}
	return "meno di due minuti"
}

// AllineaDocumento segna «scritto» un documento il cui file e' GIA' sul NAS con l'hash giusto.
//
// Non e' una scorciatoia e non salta nessun controllo: l'hash viene ricalcolato ADESSO, sul file
// vero, e se non corrisponde piu' non si allinea niente. Copiare un file identico sopra se stesso non
// aggiungerebbe nessuna garanzia — sarebbe la stessa verifica fatta due volte, con in mezzo una
// scrittura inutile su una condivisione di rete.
//
// Resta un gesto di una persona e non una riparazione automatica: e' la stessa regola per cui, quando
// una capacita' si accende, i job che avevano aspettato vengono annullati invece di partire da soli.
func AllineaDocumento(ctx context.Context, q *db.Queries, scrittore *nas.Scrittore, documentoID uuid.UUID) error {
	d, err := q.GetDocumento(ctx, documentoID)
	if err != nil {
		return err
	}
	t, err := q.GetThread(ctx, d.ThreadID)
	if err != nil {
		return err
	}
	if !t.CartellaRelativa.Valid {
		return fmt.Errorf("la RFQ non ha una cartella sul NAS: non c'e' nessun file da allineare")
	}
	assoluto := domain.UNC(scrittore.Radice, percorsoDocumento(t.CartellaRelativa.String, d.PathRelativo))
	sha, _, err := nas.Sha256File(assoluto)
	if err != nil {
		return fmt.Errorf("il file sul NAS non si legge: %w", err)
	}
	if sha != d.Sha256 {
		return fmt.Errorf("il file sul NAS NON e' questo documento (sha256 %.12s… invece di %.12s…): "+
			"e' un conflitto, e un conflitto non si allinea", sha, d.Sha256)
	}
	if err := q.SetDocumentoScritto(ctx, documentoID); err != nil {
		return err
	}
	_, err = q.ChiudiAnomaliaNas(ctx, documentoID)
	return err
}
