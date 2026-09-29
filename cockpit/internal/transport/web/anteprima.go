package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
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
//
// # La cache del browser (cache C1, domanda 31a = A)
//
// Un file gia' visto il browser lo tiene su disco, e alla riapertura chiede solo «e' cambiato?»: un 304,
// zero byte, ma comunque una richiesta con la sessione da toccare, le query e, dal NAS, le operazioni sul
// NAS. Quella richiesta si toglie — `immutable` per un anno — solo quando i byte non possono essere
// diversi da quelli che l'indirizzo promette, cioe' quando tutte queste cose sono vere insieme:
//
//  1. l'indirizzo porta l'impronta (`?v=<sha256>`) ed e' quella dell'allegato ADESSO. Lo sha256 di un
//     allegato puo' cambiare a parita' di id (un «Riscarica» che scende con un altro hash, una nuova
//     estrazione dell'archivio che riscrive le voci per posizione): con l'id soltanto, il browser terrebbe
//     per sempre il contenuto di prima;
//  2. i byte vengono dallo staging, e dal posto di un contenuto verificato: `_contenuti\<ab>\<sha256>.<ext>`
//     con lo sha256 dell'allegato (`staging.EContenuto`). Il nome e' l'hash, e un file ci arriva solo dopo
//     che l'hash e' stato verificato;
//  3. in questa richiesta non e' scattato nessun ramo di dubbio.
//
// Dal NAS mai: li' il file e' verificato solo per dimensione e data, e un file sostituito con la stessa
// dimensione e la stessa data resterebbe in cache per sempre sotto l'indirizzo dell'hash giusto, mentre
// oggi la prima riapertura dopo la segnalazione del ricognitore risponde 409. Dal NAS resta la
// rivalidazione a ogni apertura, come prima. Le risposte d'errore non si tengono (`no-store`): un 404 o un
// 409 non deve restare in memoria quando il file torna.
//
// Il 304 e' l'altra strada per cui una copia puo' diventare definitiva: non porta byte, rinfresca le
// intestazioni della copia che il browser ha gia', qualunque sia la risposta da cui l'aveva presa. Un file
// visto dal NAS, e poi tornato nello staging (esce dopo 30 giorni, «Riscarica» lo riporta), riceverebbe un
// 304 con `immutable`, e la copia del NAS varrebbe un anno. Per questo l'ETag dice anche QUANTO i byte sono
// garantiti: `"<sha256>"` solo per il contenuto verificato, `"nas-<sha256>"` e `"staging-<sha256>"` per il
// resto (`etagAnteprima`). Una copia presa altrove non combacia, ne' con If-None-Match ne' con If-Range, e il
// browser riceve il file intero, quello verificato. E la risposta definitiva non porta la data: la data di un
// file dello staging e' quella del suo ultimo uso (`staging.ToccaContenuto`), non del contenuto, e un
// If-Modified-Since da solo non deve poter dare un 304 definitivo.

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
	etag    string // sha256 dal database, non ricalcolato (l'intestazione la compone etagAnteprima)
	// percorso: il file aperto, com'e' stato controllato. Serve alla cache (C1): `immutable` solo per il
	// posto di un contenuto verificato.
	percorso string
	// inDubbio: in questa richiesta e' scattato un ramo di dubbio (il file riletto per l'hash). Anche se poi
	// l'hash torna, una risposta cosi' non si dichiara definitiva.
	inDubbio bool
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

// Le tre intestazioni `Cache-Control` dell'anteprima (cache C1).
const (
	cacheDefinitiva = "private, max-age=31536000, immutable" // i byte non possono cambiare: non si richiede
	cacheRivalida   = "private, max-age=0, must-revalidate"  // si tiene, ma a ogni apertura si chiede (304)
	cacheMai        = "no-store"                             // le risposte d'errore
)

// impronta e' lo sha256 come sta in un indirizzo: 64 cifre esadecimali in minuscolo, oppure "" se quello
// che c'e' non e' un hash (un allegato non ancora sceso non ne ha uno).
func impronta(sha string) string {
	h := strings.ToLower(strings.TrimSpace(sha))
	if len(h) != 64 || strings.Trim(h, "0123456789abcdef") != "" {
		return ""
	}
	return h
}

// indirizzoAnteprima e' l'indirizzo dell'anteprima di un allegato. Porta l'impronta del contenuto quando la
// pagina la conosce: e' l'unico indirizzo che il browser puo' tenere senza richiederlo (cache C1), perche'
// lo stesso allegato con un contenuto diverso avrebbe un indirizzo diverso. Senza impronta resta
// l'indirizzo di sempre, che si rivalida a ogni apertura.
func indirizzoAnteprima(id uuid.UUID, sha string) string {
	u := "/allegato/" + id.String() + "/anteprima"
	if h := impronta(sha); h != "" {
		u += "?v=" + h
	}
	return u
}

// contenutoVerificato: i byte di questa sorgente sono, per costruzione, quelli dello sha256 dell'allegato. Cioe'
// vengono dallo staging, dal posto del contenuto con quello sha256 (_contenuti\<ab>\<sha256>.<ext>), e nessun
// dubbio e' scattato. E' la sola garanzia che non passa da una dimensione o da una data, e decide due cose
// (cache C1): la risposta definitiva e l'ETag nudo.
func contenutoVerificato(radiceStaging, shaAllegato string, src *sorgente) bool {
	h := impronta(shaAllegato)
	if src == nil || h == "" || src.da != "staging" || src.inDubbio {
		return false
	}
	return staging.EContenuto(radiceStaging, src.percorso, h)
}

// cacheAnteprima decide il `Cache-Control` di un'anteprima servita (cache C1): definitiva SOLO se l'impronta
// chiesta `v` e' lo sha256 dell'allegato e i byte sono il contenuto verificato con quello sha256. Tutto il
// resto si rivalida, come prima di C1. E' una funzione pura perche' e' la decisione che conta: le prove la
// guardano caso per caso, compresi quelli che devono dire no.
func cacheAnteprima(radiceStaging, shaAllegato, v string, src *sorgente) string {
	if v == "" || v != impronta(shaAllegato) || !contenutoVerificato(radiceStaging, shaAllegato, src) {
		return cacheRivalida
	}
	return cacheDefinitiva
}

// etagAnteprima e' l'ETag di un'anteprima servita: lo sha256 dal database, non ricalcolato, e davanti da dove
// vengono i byte quando non sono il contenuto verificato (cache C1). Con lo stesso ETag per tutte le fonti,
// una copia presa dal NAS — verificata solo per dimensione e data — riceverebbe un 304 definitivo il giorno
// in cui lo stesso contenuto torna nello staging; cosi' non combacia, e il browser riceve il file intero. Il
// NAS e lo staging non verificato restano distinti anche fra loro: sono due posti diversi, e che abbiano gli
// stessi byte non l'ha verificato nessuno. Fra due risposte della stessa fonte l'ETag resta lo stesso, e il
// 304 c'e' come prima.
func etagAnteprima(radiceStaging, shaAllegato string, src *sorgente) string {
	if src == nil || src.etag == "" {
		return ""
	}
	if contenutoVerificato(radiceStaging, shaAllegato, src) {
		return `"` + impronta(shaAllegato) + `"`
	}
	return `"` + strings.ToLower(src.da) + "-" + src.etag + `"`
}

// rispostaAnteprima e' la risposta della rotta: ricorda lo stato per la riga di log (la misura della cache,
// domanda 31d), e fa partire ogni errore con `no-store`. Ogni errore, anche quelli che scrive ServeContent
// (un Range fuori dal file, un If-Match che non torna): lui toglie il Cache-Control che avevamo messo per il
// file, e senza questo l'errore uscirebbe senza.
type rispostaAnteprima struct{ rispostaConStato }

func (w *rispostaAnteprima) WriteHeader(stato int) {
	if w.stato == 0 && stato >= 400 {
		w.Header().Set("Cache-Control", cacheMai)
	}
	w.rispostaConStato.WriteHeader(stato)
}

// nonCeLaRiga distingue le due risposte che una query puo' dare quando non torna niente: «quella riga
// non c'e'» e «il database non ha risposto». Sembrano la stessa cosa da dove le si riceve, e non lo
// sono: la prima e' un caso normale — un allegato non ancora agganciato a una RFQ non ha una cartella
// — e manda l'anteprima a cercare altrove; la seconda e' un guasto, e chiamarla «il file non c'e'»
// vuol dire mandare chi guarda a premere «Riscarica» per un problema che nessun download risolve.
func nonCeLaRiga(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// nonServita e' l'unica uscita d'errore della rotta: uno stato, un messaggio per chi guarda, una riga
// di log. Sta in un posto solo perche' quando stava in due, i due posti scrivevano campi diversi.
func (s *Server) nonServita(w http.ResponseWriter, aid uuid.UUID, err error) {
	var e erroreHTTP
	if !errors.As(err, &e) {
		e = erroreHTTP{http.StatusInternalServerError, "anteprima non disponibile", err}
	}
	if e.stato >= 500 {
		s.Log.Error("anteprima non servita", "allegato", aid, "stato", e.stato, "err", e.Error())
	} else {
		s.Log.Warn("anteprima non servita", "allegato", aid, "stato", e.stato, "err", e.Error())
	}
	http.Error(w, e.messaggio, e.stato)
}

// pdfDavvero guarda i primi cinque byte, e distingue TRE casi dove a occhio ce ne sono due: e' un
// PDF, non e' un PDF, oppure non si e' riusciti a leggerlo.
//
// Il terzo non e' il secondo. Un NAS che si stacca a meta' lettura, rispondendo «anteprima
// disponibile solo per i PDF», manda a cercare un problema di formato dove c'e' un problema di rete —
// e non lascia niente nel log, perche' un 415 e' una risposta normale. Un file piu' corto di cinque
// byte, invece, e' davvero un file che non e' un PDF.
func pdfDavvero(r io.Reader) error {
	var testa [5]byte
	n, err := io.ReadFull(r, testa[:])
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return erroreHTTP{http.StatusInternalServerError, "anteprima non disponibile", err}
	}
	if string(testa[:n]) != "%PDF-" {
		// Non e' un PDF: non si serve. Non per il tipo dichiarato, per i byte veri — un file
		// rinominato `.pdf` resta cio' che e', e il viewer del browser non deve provare ad aprirlo.
		return erroreHTTP{http.StatusUnsupportedMediaType, "anteprima disponibile solo per i PDF", nil}
	}
	return nil
}

func (s *Server) anteprima(rw http.ResponseWriter, r *http.Request) {
	w := &rispostaAnteprima{rispostaConStato{ResponseWriter: rw}}
	aid, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "id non valido", http.StatusBadRequest)
		return
	}
	// I byte di un file si aprono come documento (l'iframe, pdf.js, una scheda nuova), mai dentro la
	// pagina: nessuno di quei tre manda HX-Request, e una richiesta htmx qui vorrebbe dire che qualcuno
	// ha fatto chiedere alla schermata un file da innestare fra i suoi pezzi. Un «PDF» caricato da un
	// operatore puo' cominciare per %PDF- e contenere HTML con uno script: nella pagina si eseguirebbe.
	if r.Header.Get("HX-Request") != "" {
		s.Log.Warn("anteprima chiesta da htmx: rifiutata", "allegato", aid, "percorso", r.Header.Get("HX-Current-URL"))
		http.Error(w, "l'anteprima si apre come documento, non dentro la pagina", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	q := db.New(s.Pool)
	a, err := q.GetAllegato(ctx, aid)
	if err != nil {
		if nonCeLaRiga(err) {
			http.Error(w, "allegato non trovato", http.StatusNotFound)
			return
		}
		s.nonServita(w, aid, err)
		return
	}
	src, err := s.sorgenteAnteprima(ctx, q, a)
	if err != nil {
		s.nonServita(w, aid, err)
		return
	}
	defer src.Close()

	if err := pdfDavvero(src); err != nil {
		s.nonServita(w, aid, err)
		return
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		s.Log.Error("anteprima: il file non si riavvolge", "allegato", aid, "err", err)
		http.Error(w, "anteprima non disponibile", http.StatusInternalServerError)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "application/pdf")
	h.Set("Content-Disposition", nomePerIlBrowser(a.NomeFile))
	h.Set("X-Content-Type-Options", "nosniff")
	// Un PDF puo' contenere JavaScript e moduli: nel viewer non deve eseguire niente e non deve
	// poter chiamare nessuno.
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'")
	// Prima di ServeContent, e quindi uguale per il 200, il 206 e il 304 della stessa richiesta: un 304
	// rinfresca la copia che il browser ha, e con un'intestazione diversa la cambierebbe (cache C1).
	cache := cacheAnteprima(s.Staging, a.Sha256.String, r.URL.Query().Get("v"), src)
	h.Set("Cache-Control", cache)
	if etag := etagAnteprima(s.Staging, a.Sha256.String, src); etag != "" {
		h.Set("ETag", etag)
	}
	// La risposta definitiva si rivalida solo con l'ETag, mai con la data: senza data ServeContent non
	// scrive Last-Modified, ignora If-Modified-Since e a un If-Range con la data risponde col file intero
	// (cache C1). La data di un file dello staging e' quella del suo ultimo uso, non del contenuto.
	modtime := src.modtime
	if cache == cacheDefinitiva {
		modtime = time.Time{}
	}
	// ServeContent fa il resto: Range, If-Range, If-None-Match, HEAD. Il nome e' vuoto di proposito,
	// perche' il Content-Type l'abbiamo gia' deciso noi e non va dedotto dall'estensione.
	http.ServeContent(w, r, "", modtime, src)
	s.registraServita(r, aid, w.stato, src, cache)
}

// registraServita scrive la riga «anteprima servita». E' la misura della cache (C0/C1, domanda 31d): lo stato
// dice se il file e' uscito (200, 206) o se il browser l'aveva gia' (304), senza doverlo indovinare dai byte
// letti — un 304 dopo una rilettura per dubbio ne ha letti quanti un 200 —; utente, postazione e indirizzo
// dicono se i 200 dello stesso allegato tornano dallo stesso PC, cioe' dove la cache del browser non c'e'
// (il certificato non attendibile, una policy). Solo una riga di log: la sessione non si tocca.
func (s *Server) registraServita(r *http.Request, aid uuid.UUID, stato int, src *sorgente, cache string) {
	if stato == 0 {
		stato = http.StatusOK
	}
	sess := sessioneDa(r.Context())
	utente, ip := "", ""
	if sess.Utente != nil {
		utente = sess.Utente.Sigla
	}
	if a, ok := s.indirizzoDi(r); ok {
		ip = a.String()
	}
	politica := "rivalida"
	if cache == cacheDefinitiva {
		politica = "definitiva"
	}
	s.Log.Info("anteprima servita", "allegato", aid, "stato", stato, "da", src.da, "byte_letti", src.letti,
		"dimensione", src.dim, "range", r.Header.Get("Range"), "cache", politica,
		"utente", utente, "postazione", sess.NomeHost, "ip", ip)
}

// nomePerIlBrowser scrive il nome del file in `Content-Disposition` nelle DUE forme che
// l'intestazione prevede: `filename=` in ASCII, che leggono tutti, e `filename*` in percentuale —
// che dichiara la codifica e porta il nome intero.
//
// Un'intestazione HTTP e' fatta di byte, non di caratteri. «Staffa – rev À.pdf» scritto dentro
// `filename=` ci viaggia come byte non-ASCII, e da li' in poi ogni browser decide per conto suo: chi
// li legge come latin-1 e propone «Staffa â€“ rev Ã€.pdf», chi taglia il nome al primo byte strano,
// chi non capisce piu' dove finisce il valore. Con `filename*` il nome e' scritto in una forma sola,
// che dice anche in quale codifica e' scritto, e l'ASCII resta come ripiego per chi non la conosce.
//
// I nomi qui dentro sono gia' passati da `NomeFileSicuro`, che toglie virgolette, barre e caratteri
// di controllo: il rimpiazzo con «_» qui sotto e' il secondo controllo di una cosa gia' vera, e resta
// perche' e' questa funzione, non quella, a promettere un'intestazione che non si rompe.
func nomePerIlBrowser(nome string) string {
	sicuro := documenti.NomeFileSicuro(nome)
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, sicuro)
	if ext := filepath.Ext(ascii); strings.Trim(strings.TrimSuffix(ascii, ext), "_ ") == "" {
		// Un nome fatto solo di caratteri non-ASCII: «___.pdf» non dice niente a nessuno, e il nome
		// vero arriva comunque con filename*.
		if ext == "" {
			ext = ".pdf"
		}
		ascii = "documento" + ext
	}
	d := `inline; filename="` + ascii + `"`
	if ascii != sicuro {
		d += "; filename*=UTF-8''" + percentuale(sicuro)
	}
	return d
}

// percentuale codifica come vuole la RFC 5987: restano i caratteri che non possono rompere
// l'intestazione, tutto il resto diventa %XX sui byte UTF-8 (lo spazio compreso).
func percentuale(s string) string {
	const ammessi = "!#$&+-.^_`|~"
	var b strings.Builder
	for _, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte(ammessi, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

// sorgenteAnteprima decide DA DOVE arrivano i byte: prima lo staging, poi il NAS, altrimenti niente.
//
// L'ordine non e' casuale. Lo staging e' sul disco di questo server, il suo nome E' l'hash del
// contenuto e non c'e' niente da verificare; il NAS e' una condivisione di rete che puo' essere
// lenta, montata a meta' o cambiata da qualcun altro. Si guarda il posto vicino e certo prima di
// quello lontano.
func (s *Server) sorgenteAnteprima(ctx context.Context, q *db.Queries, a db.Allegato) (*sorgente, error) {
	if a.PathStaging.Valid && a.PathStaging.String != "" {
		percorso, err := s.nelloStaging(a.PathStaging.String)
		if err != nil {
			return nil, err
		}
		if percorso != "" {
			f, err := os.Open(percorso)
			switch {
			case err == nil:
				st, err := f.Stat()
				if err != nil || st.IsDir() {
					f.Close()
					return nil, erroreHTTP{http.StatusInternalServerError, "anteprima non disponibile", err}
				}
				return &sorgente{f: f, da: "staging", dim: st.Size(), modtime: st.ModTime(),
					etag: a.Sha256.String, percorso: percorso}, nil
			case !os.IsNotExist(err):
				// Il file c'e' e non si apre (permessi, disco): il NAS puo' avere lo stesso contenuto,
				// e provarci e' meglio che rispondere di no. Resta nel log, perche' un errore di
				// lettura sullo staging non e' normale.
				s.Log.Warn("anteprima: lo staging non si legge", "allegato", a.AllegatoID, "err", err)
			}
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

// nelloStaging verifica che il percorso scritto in `allegato.path_staging` stia davvero dentro la
// cartella di staging di QUESTO server, e lo restituisce ripulito.
//
// Il NAS aveva gia' il suo controllo (`documenti.PercorsoSulNas`) e lo staging no, per una ragione
// che sembrava buona: quel percorso lo scrive il server stesso, mentre `documento.path_relativo`
// viene da lontano. Ma «lo scrive il server» e' esattamente cio' che si diceva anche dell'altro, e le
// due radici non possono avere due regole diverse: una riga cambiata a mano, una migrazione che
// ricalcola i percorsi, un `path_staging` rimasto da una configurazione precedente in cui lo staging
// stava altrove, e la rotta aprirebbe un file qualunque del disco del server.
//
// Tre risposte, non due: percorso buono; percorso che esce dalla radice, che e' un bug e vale 500;
// radice non dichiarata, e allora dallo staging non si serve — cio' che non si puo' verificare non si
// serve — e si prova il NAS. In produzione la radice c'e' sempre (la configurazione la crea
// all'avvio); se un giorno non ci fosse, lo dice il log invece di lasciar passare tutto.
func (s *Server) nelloStaging(percorso string) (string, error) {
	radice := strings.TrimSpace(s.Staging)
	if radice == "" {
		s.Log.Warn("anteprima: la radice dello staging non e' dichiarata, non si serve dallo staging",
			"percorso", percorso)
		return "", nil
	}
	p := filepath.Clean(percorso)
	if !filepath.IsAbs(p) {
		return "", erroreHTTP{http.StatusInternalServerError, "anteprima non disponibile",
			fmt.Errorf("path_staging non e' un percorso assoluto: %q", percorso)}
	}
	base := filepath.Clean(strings.TrimRight(radice, `\/`))
	if !documenti.DentroLaRadice(p, base) {
		return "", erroreHTTP{http.StatusInternalServerError, "anteprima non disponibile",
			fmt.Errorf("percorso fuori radice: %q non sta sotto lo staging %q", p, base)}
	}
	return p, nil
}

// dalNas serve il file del DOCUMENTO che ha lo stesso contenuto di questo allegato, se c'e', se e'
// scritto e se niente fa dubitare che sia ancora quello. Nil senza errore = non e' il caso, prova
// altrove; errore = e' il caso ma qualcosa non torna, e la differenza va detta a chi guarda.
func (s *Server) dalNas(ctx context.Context, q *db.Queries, a db.Allegato) (*sorgente, error) {
	if s.NAS == nil || !a.Sha256.Valid || a.Sha256.String == "" {
		return nil, nil
	}
	m, err := q.GetMessaggio(ctx, a.MessaggioID)
	if err != nil {
		if !nonCeLaRiga(err) {
			return nil, erroreHTTP{http.StatusInternalServerError, "anteprima non disponibile", err}
		}
		return nil, nil
	}
	if !m.ThreadID.Valid {
		return nil, nil // l'allegato non e' ancora agganciato a una RFQ: non c'e' nessuna cartella
	}
	d, err := q.GetDocumentoPerHash(ctx, db.GetDocumentoPerHashParams{
		ThreadID: m.ThreadID.UUID, Sha256: a.Sha256.String})
	if err != nil {
		if !nonCeLaRiga(err) {
			return nil, erroreHTTP{http.StatusInternalServerError, "anteprima non disponibile", err}
		}
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
	if err != nil {
		if !nonCeLaRiga(err) {
			return nil, erroreHTTP{http.StatusInternalServerError, "anteprima non disponibile", err}
		}
		return nil, nil
	}
	if !t.CartellaRelativa.Valid {
		return nil, nil // la RFQ non ha ancora una cartella sul NAS
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
	src := &sorgente{f: f, da: "NAS", dim: st.Size(), modtime: st.ModTime(), etag: d.Sha256, percorso: p.Assoluto}
	if dubbio(d, st) {
		src.inDubbio = true
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
