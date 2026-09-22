package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/db"
)

// L'ANTEPRIMA: il PDF si guarda dal Cockpit, senza scaricarlo e senza cercarlo sul NAS a mano
// (blocco 8, B8.1).
//
// Fino al blocco 8 nessuna rotta del sito serviva i byte di un allegato: l'unico download era quello
// dei worker, autenticato per token e lease. Chi doveva decidere se un PDF fosse il disegno o il
// capitolato apriva Esplora risorse, cercava la cartella, apriva il file e tornava indietro — e
// intanto la proposta restava aperta.
//
// Tre regole, e sono quelle che rendono la rotta noiosa, che e' la qualita' che serve a una rotta che
// serve file:
//
//  1. il percorso non arriva MAI da fuori. Nella URL c'e' l'identificativo dell'allegato; dove stiano
//     i byte lo dicono `allegato.path_staging` e, per il NAS, `documento.path_relativo` con la
//     cartella della RFQ, e il risultato passa comunque da `documenti.PercorsoSulNas`, che verifica
//     che quel che si e' composto stia dentro la radice;
//  2. si serve solo cio' che comincia per `%PDF-`. Non l'estensione, non il `Content-Type` dichiarato
//     da qualcuno: i primi cinque byte del file vero;
//  3. i byte si fanno scorrere, non si caricano in memoria. `http.ServeContent` fa `Range`,
//     `If-Range`, `If-None-Match` e `HEAD`: il viewer del browser chiede l'ultimo chilobyte per
//     leggere la tabella delle pagine e poi le pagine che servono, e cosi' un PDF da 60 MB si apre.
//
// # Perche' l'hash NON si ricalcola a ogni clic
//
// Il file sul NAS e' gia' stato verificato due volte — alla copia, byte per byte, e dal ricognitore
// del blocco 5B — e rileggere sessanta megabyte dalla rete per servirne cinquecento chilo vorrebbe
// dire buttare via il `Range` che il viewer usa apposta. Qui si guardano le cose che costano niente:
// che non ci sia un'anomalia aperta su quel documento, che la dimensione sia quella scritta nel
// fascicolo, e che il file non sia stato toccato dopo l'ultima volta che il server l'ha guardato.
//
// Se una di queste non torna — e SOLO allora — si rilegge il file intero una volta sola. Ma non con
// `AllineaDocumento`, che e' il gesto di un amministratore e su un conflitto si limita a rifiutare:
// con `documenti.VerificaFileAperto`, che sul file GIA' APERTO dal percorso GIA' contenuto apre
// l'anomalia se l'hash non corrisponde. Una scoperta fatta durante un'anteprima e taciuta e' una
// scoperta persa: chi ha cliccato chiude la finestra, e il fascicolo continua a promettere un file
// che non e' quello.
//
// # Chi puo' guardare
//
// `autenticato`, come tutto il resto: nel Cockpit chi e' entrato vede le RFQ dell'ufficio, e un
// allegato non e' piu' riservato del messaggio da cui si apre. Il giorno in cui i ruoli diventeranno
// una matrice (voce 6.9) questa rotta si muovera' insieme a `/messaggio/{id}`, non da sola.

// sorgente e' un file aperto pronto da servire, con cio' che serve alle intestazioni — e il conto dei
// byte letti davvero.
//
// Il contatore non e' un attrezzo da prova: e' la risposta alla sola domanda che si fa a un'anteprima
// lenta, «quanto ha letto dalla rete per mostrarmi due pagine?». Finisce nel log a ogni richiesta
// servita, accanto a `Range` e alla dimensione del file, e quando un giorno dira' sessanta megabyte
// per cinquecento chilo si sapra' che il dubbio e' scattato e si andra' a vedere perche'.
type sorgente struct {
	f       *os.File
	letti   int64
	da      string // "staging" | "NAS": da quale dei due posti arrivano i byte
	dim     int64
	modtime time.Time
	etag    string // sha256 dal database, non ricalcolato
}

func (s *sorgente) Read(p []byte) (int, error) {
	n, err := s.f.Read(p)
	s.letti += int64(n)
	return n, err
}

func (s *sorgente) Seek(offset int64, da int) (int64, error) { return s.f.Seek(offset, da) }
func (s *sorgente) Close() error                             { return s.f.Close() }

// erroreHTTP porta con se' lo stato: la rotta non deve indovinare se un errore sia 404, 409 o 500.
type erroreHTTP struct {
	stato     int
	messaggio string
	err       error
}

func (e erroreHTTP) Error() string {
	if e.err != nil {
		return e.messaggio + ": " + e.err.Error()
	}
	return e.messaggio
}

func (e erroreHTTP) Unwrap() error { return e.err }

func (s *Server) anteprima(w http.ResponseWriter, r *http.Request) {
	aid, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	q := db.New(s.Pool)
	a, err := q.GetAllegato(ctx, aid)
	if err != nil {
		http.Error(w, "allegato non trovato", http.StatusNotFound)
		return
	}
	src, err := s.sorgenteAnteprima(ctx, q, a)
	if err != nil {
		var e erroreHTTP
		if errors.As(err, &e) {
			if e.stato >= 500 {
				s.Log.Error("anteprima non servita", "allegato", aid, "err", e.Error())
			} else {
				s.Log.Warn("anteprima non servita", "allegato", aid, "stato", e.stato, "err", e.Error())
			}
			http.Error(w, e.messaggio, e.stato)
			return
		}
		s.Log.Error("anteprima non servita", "allegato", aid, "err", err)
		http.Error(w, "anteprima non disponibile", http.StatusInternalServerError)
		return
	}
	defer src.Close()

	var testa [5]byte
	n, _ := io.ReadFull(src, testa[:])
	if string(testa[:n]) != "%PDF-" {
		// Non e' un PDF: non si serve. Non per il tipo dichiarato, per i byte veri — un file
		// rinominato `.pdf` resta cio' che e', e il viewer del browser non deve provare ad aprirlo.
		http.Error(w, "anteprima disponibile solo per i PDF", http.StatusUnsupportedMediaType)
		return
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		s.Log.Error("anteprima: il file non si riavvolge", "allegato", aid, "err", err)
		http.Error(w, "anteprima non disponibile", http.StatusInternalServerError)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "application/pdf")
	h.Set("Content-Disposition", `inline; filename="`+documenti.NomeFileSicuro(a.NomeFile)+`"`)
	h.Set("X-Content-Type-Options", "nosniff")
	// Un PDF puo' contenere JavaScript e moduli: nel viewer non deve eseguire niente e non deve
	// poter chiamare nessuno.
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	h.Set("Cache-Control", "private, max-age=0, must-revalidate")
	if src.etag != "" {
		h.Set("ETag", `"`+src.etag+`"`)
	}
	// ServeContent fa il resto: Range, If-Range, If-None-Match, HEAD. Il nome e' vuoto di proposito,
	// perche' il Content-Type l'abbiamo gia' deciso noi e non va dedotto dall'estensione.
	http.ServeContent(w, r, "", src.modtime, src)
	s.Log.Info("anteprima servita", "allegato", aid, "da", src.da, "byte_letti", src.letti,
		"dimensione", src.dim, "range", r.Header.Get("Range"))
}

// sorgenteAnteprima decide DA DOVE arrivano i byte: prima lo staging, poi il NAS, altrimenti niente.
//
// L'ordine non e' casuale. Lo staging e' sul disco di questo server, il suo nome E' l'hash del
// contenuto e non c'e' niente da verificare; il NAS e' una condivisione di rete che puo' essere
// lenta, montata a meta' o cambiata da qualcun altro. Si guarda il posto vicino e certo prima di
// quello lontano.
func (s *Server) sorgenteAnteprima(ctx context.Context, q *db.Queries, a db.Allegato) (*sorgente, error) {
	if a.PathStaging.Valid && a.PathStaging.String != "" {
		f, err := os.Open(a.PathStaging.String)
		switch {
		case err == nil:
			st, err := f.Stat()
			if err != nil || st.IsDir() {
				f.Close()
				return nil, erroreHTTP{http.StatusInternalServerError, "anteprima non disponibile", err}
			}
			return &sorgente{f: f, da: "staging", dim: st.Size(), modtime: st.ModTime(),
				etag: a.Sha256.String}, nil
		case !os.IsNotExist(err):
			// Il file c'e' e non si apre (permessi, disco): il NAS puo' avere lo stesso contenuto, e
			// provarci e' meglio che rispondere di no. Resta nel log, perche' un errore di lettura
			// sullo staging non e' normale.
			s.Log.Warn("anteprima: lo staging non si legge", "allegato", a.AllegatoID, "err", err)
		}
	}
	src, err := s.dalNas(ctx, q, a)
	if err != nil {
		return nil, err
	}
	if src != nil {
		return src, nil
	}
	return nil, erroreHTTP{http.StatusNotFound, "il contenuto non e' su questo server: usa «Riscarica»", nil}
}

// dalNas serve il file del DOCUMENTO che ha lo stesso contenuto di questo allegato, se c'e', se e'
// scritto e se niente fa dubitare che sia ancora quello. Nil senza errore = non e' il caso, prova
// altrove; errore = e' il caso ma qualcosa non torna, e la differenza va detta a chi guarda.
func (s *Server) dalNas(ctx context.Context, q *db.Queries, a db.Allegato) (*sorgente, error) {
	if s.NAS == nil || !a.Sha256.Valid || a.Sha256.String == "" {
		return nil, nil
	}
	m, err := q.GetMessaggio(ctx, a.MessaggioID)
	if err != nil || !m.ThreadID.Valid {
		return nil, nil // l'allegato non e' ancora agganciato a una RFQ: non c'e' nessuna cartella
	}
	d, err := q.GetDocumentoPerHash(ctx, db.GetDocumentoPerHashParams{
		ThreadID: m.ThreadID.UUID, Sha256: a.Sha256.String})
	if err != nil {
		return nil, nil // nessun documento con questo contenuto in questa RFQ
	}
	if d.StatoNas != db.StatoNasScritto {
		return nil, nil // in coda o in errore: il file sul NAS non c'e'
	}
	// Un'anomalia aperta dice che il ricognitore ha gia' trovato qualcosa che non torna su QUESTO
	// documento. Servirlo lo stesso vorrebbe dire mostrare come «il disegno» un file che il server
	// stesso ha segnalato: finche' l'anomalia e' aperta, l'anteprima tace.
	if _, err := q.GetAnomaliaNas(ctx, d.DocumentoID); err == nil {
		return nil, erroreHTTP{http.StatusConflict,
			"il file sul NAS ha una segnalazione aperta: risolvila in Admin prima di guardarlo", nil}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, erroreHTTP{http.StatusInternalServerError, "anteprima non disponibile", err}
	}
	if !s.NAS.Raggiungibile() {
		return nil, erroreHTTP{http.StatusServiceUnavailable, "il NAS non risponde: riprova fra poco", nil}
	}
	t, err := q.GetThread(ctx, d.ThreadID)
	if err != nil || !t.CartellaRelativa.Valid {
		return nil, nil
	}
	p, err := documenti.PercorsoSulNas(s.NAS.Radice, t.CartellaRelativa.String, d.PathRelativo)
	if err != nil {
		// Non e' un caso d'uso: e' un bug o una riga cambiata a mano nel database. 500, e nel log.
		return nil, erroreHTTP{http.StatusInternalServerError, "anteprima non disponibile", err}
	}
	f, err := os.Open(p.Assoluto)
	if err != nil {
		// Il documento dice «scritto» e il file non si apre: e' esattamente cio' che il ricognitore
		// segnalerebbe alla prossima passata, e non c'e' ragione di aspettarla — chi guarda adesso sta
		// guardando adesso.
		problema, dettaglio := db.ProblemaNasIlleggibile, "il file c'e' ma non si riesce ad aprirlo: "+err.Error()
		if os.IsNotExist(err) {
			problema, dettaglio = db.ProblemaNasMancante, "il documento risulta scritto sul NAS, ma il file non c'e' piu'"
		}
		if errA := documenti.Segnala(ctx, q, d, p.Relativo, problema, "", dettaglio); errA != nil {
			return nil, erroreHTTP{http.StatusInternalServerError, "anteprima non disponibile", errA}
		}
		return nil, erroreHTTP{http.StatusConflict,
			"il file non e' al suo posto sul NAS: e' stato segnalato al controllo di integrita'", err}
	}
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		f.Close()
		dettaglio := "al posto del file, sul NAS c'e' una CARTELLA con lo stesso nome"
		if err != nil {
			dettaglio = "il file c'e' ma non si riesce a guardarlo: " + err.Error()
		}
		if errA := documenti.Segnala(ctx, q, d, p.Relativo, db.ProblemaNasIlleggibile, "", dettaglio); errA != nil {
			return nil, erroreHTTP{http.StatusInternalServerError, "anteprima non disponibile", errA}
		}
		return nil, erroreHTTP{http.StatusConflict,
			"il file non e' al suo posto sul NAS: e' stato segnalato al controllo di integrita'", err}
	}
	src := &sorgente{f: f, da: "NAS", dim: st.Size(), modtime: st.ModTime(), etag: d.Sha256}
	if dubbio(d, st) {
		// Qui, e solo qui, si rilegge il file intero — e lo si fa DA QUESTO handle, contando i byte:
		// se l'hash non corrisponde l'anomalia resta scritta e non esce un solo byte.
		if err := documenti.VerificaFileAperto(ctx, q, d, p.Relativo, src); err != nil {
			f.Close()
			if errors.Is(err, documenti.ErrNonCorrisponde) {
				return nil, erroreHTTP{http.StatusConflict,
					"il file sul NAS non e' questo documento: e' stato segnalato al controllo di integrita'", err}
			}
			return nil, erroreHTTP{http.StatusConflict,
				"il file sul NAS non si e' potuto verificare: e' stato segnalato al controllo di integrita'", err}
		}
	}
	return src, nil
}

// dubbio: il file sul disco non e' piu' quello che il database descrive?
//
// Sono due domande che costano uno `Stat` e rispondono senza leggere un byte: la dimensione e' quella
// scritta, e il file e' stato toccato dopo l'ultima volta che il server l'ha guardato? Un minuto di
// tolleranza perche' l'orologio del NAS e quello del server non sono lo stesso orologio.
//
// Non e' una verifica di integrita' e non pretende di esserlo: un file sostituito con un altro della
// stessa identica dimensione, senza che la data cambi, passerebbe di qui. A quello rispondono la
// verifica byte per byte della copia e il ricognitore, che sono le due sole scritture di verita' sul
// NAS. Questo e' il filtro che decide QUANDO vale la pena di rileggere sessanta megabyte.
func dubbio(d db.Documento, st os.FileInfo) bool {
	if d.Bytes.Valid && d.Bytes.Int64 > 0 && st.Size() != d.Bytes.Int64 {
		return true
	}
	ultimo := d.ScrittoIl
	if d.VerificatoIl != nil && (ultimo == nil || d.VerificatoIl.After(*ultimo)) {
		ultimo = d.VerificatoIl
	}
	if ultimo == nil {
		return false // non sappiamo quando l'abbiamo guardato: lo `Stat` non dice niente
	}
	return st.ModTime().After(ultimo.Add(time.Minute))
}
