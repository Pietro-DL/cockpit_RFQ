//go:build integrazione

// L4 — l'anteprima PDF (blocco 8, B8.1), contro un PostgreSQL vero, file veri su disco e il giro
// HTTP completo con sessione e CSRF.
//
// Le quattro domande a cui queste prove rispondono sono le quattro che l'addendum pone:
//
//	da dove arrivano i byte      → staging se c'e', altrimenti il NAS, e si vede nel log quale dei due
//	quanti byte si leggono       → quelli chiesti, non quelli del file: un Range da 512 KB su un file
//	                               da 2 MB legge 512 KB, e un hash ricalcolato si vedrebbe subito
//	che cosa resta di una scoperta → un'anomalia scritta, non un messaggio a chi ha cliccato
//	dove si apre il file          → solo dentro la radice del NAS, comunque sia fatta la riga in database
//
// Il conto dei byte non e' un attrezzo da prova: la rotta lo scrive nel log a ogni richiesta servita,
// perche' e' la risposta alla sola domanda che si fa a un'anteprima lenta. Qui si legge quel log.
package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
	"promatec/cockpit/internal/platform/testutil"
)

const cartellaRFQProva = `ACME\WIP\2026 09 22 Rossi prova`
const pathDocProva = `ELENCO DISEGNI\0.000.0000.0\disegno.pdf`

// registro raccoglie le righe che il server scrive nel suo log, per poterle leggere nella prova.
type registro struct {
	mu    sync.Mutex
	righe []string
}

func (r *registro) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.righe = append(r.righe, string(p))
	return len(p), nil
}

var reByteLetti = regexp.MustCompile(`byte_letti=(\d+)`)
var reDa = regexp.MustCompile(`da=(\S+)`)

// servita legge dal log l'ultima anteprima servita: da dove e quanti byte ha letto davvero.
func (r *registro) servita(t *testing.T) (string, int64) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.righe) - 1; i >= 0; i-- {
		riga := r.righe[i]
		if !strings.Contains(riga, "anteprima servita") {
			continue
		}
		m := reByteLetti.FindStringSubmatch(riga)
		d := reDa.FindStringSubmatch(riga)
		if m == nil || d == nil {
			t.Fatalf("la riga di log non dice quanto ha letto: %s", riga)
		}
		n, _ := strconv.ParseInt(m[1], 10, 64)
		return strings.Trim(d[1], `"`), n
	}
	t.Fatalf("il server non ha registrato nessuna anteprima servita:\n%s", strings.Join(r.righe, ""))
	return "", 0
}

type scenaAnteprima struct {
	b        *bancoWeb
	reg      *registro
	fp       *browser
	allegato uuid.UUID
	doc      uuid.UUID
	thread   uuid.UUID
	radice   string // la radice del NAS di questo server
	suNas    string // il percorso assoluto del file del documento

	radiceStaging string // la radice dello staging locale, quella che il server verifica
	staging       string // il percorso del contenuto in staging ("" se non c'e')
	pdf           []byte
}

// pdfFinto e' un PDF vero quanto basta: comincia per %PDF- (che e' l'unica cosa che la rotta guarda)
// ed e' grande quanto serve per distinguere «ha letto il pezzo chiesto» da «ha letto tutto».
func pdfFinto(n int) []byte {
	p := make([]byte, n)
	copy(p, []byte("%PDF-1.7\n% prova del cockpit\n"))
	riempi := []byte("0123456789abcdef")
	for i := 29; i < n; i++ {
		p[i] = riempi[i%len(riempi)]
	}
	copy(p[n-6:], []byte("%%EOF\n"))
	return p
}

// preparaAnteprima costruisce una RFQ con un messaggio agganciato, un allegato PDF e il documento
// confermato che ne porta lo stesso contenuto. `conStaging` e `conNas` dicono in quale dei due posti
// il file esiste davvero: e' la differenza che le prove qui sotto interrogano.
func preparaAnteprima(t *testing.T, pdf []byte, conStaging, conNas bool) *scenaAnteprima {
	t.Helper()
	b := preparaBancoWeb(t)
	reg := &registro{}
	b.ws.Log = slog.New(slog.NewTextHandler(reg, &slog.HandlerOptions{Level: slog.LevelInfo}))
	s := &scenaAnteprima{b: b, reg: reg, radice: t.TempDir(), pdf: pdf}
	b.ws.NAS = &nas.Scrittore{Radice: s.radice}

	sha := shaDi(pdf)
	cliente := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	deve := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	deve(b.pool.QueryRow(b.ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa, priorita)
		VALUES ($1,'outlook',now(),'RFQ anteprima',$2,1) RETURNING thread_id`, cliente, cartellaRFQProva).Scan(&s.thread))
	var conv, msg uuid.UUID
	deve(b.pool.QueryRow(b.ctx, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il)
		VALUES ('outlook','CONV-ANT',now()) RETURNING conversazione_id`).Scan(&conv))
	deve(b.pool.QueryRow(b.ctx, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento,
		oggetto, mittente_indirizzo, thread_id, aggancio) VALUES ('outlook','MSG-ANT',$1,'entrata',now(),
		'RFQ con disegno','acquisti@acme.example',$2,'operatore') RETURNING messaggio_id`, conv, s.thread).Scan(&msg))

	// La radice dello staging si dichiara SEMPRE, anche quando il contenuto non c'e': e' quella che
	// il server usa per verificare che `path_staging` non punti da un'altra parte del disco, e un
	// banco che la lasciasse vuota proverebbe l'anteprima di un server configurato male.
	s.radiceStaging = t.TempDir()
	b.ws.Staging = s.radiceStaging
	if conStaging {
		s.staging = filepath.Join(s.radiceStaging, "_contenuti", sha[:2], sha+".pdf")
		deve(os.MkdirAll(filepath.Dir(s.staging), 0o755))
		deve(os.WriteFile(s.staging, pdf, 0o644))
	}
	deve(b.pool.QueryRow(b.ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, bytes, sha256,
		path_staging, stato, ricevuto_il) VALUES ($1,1,'disegno.pdf','pdf','file',$2,$3,$4,'analizzato',now())
		RETURNING allegato_id`, msg, len(pdf), sha, sePresente(s.staging)).Scan(&s.allegato))

	var utente uuid.UUID
	deve(b.pool.QueryRow(b.ctx, `SELECT utente_id FROM utente WHERE sigla='FP'`).Scan(&utente))
	deve(b.pool.QueryRow(b.ctx, `INSERT INTO documento (thread_id, tipo, codice, nome_file, estensione, sha256, bytes,
		path_relativo, stato_nas, scritto_il, confermato_da, confermato_il)
		VALUES ($1,'disegno_2d','D1','disegno.pdf','pdf',$2,$3,$4,'scritto', now(), $5, now() - interval '1 day')
		RETURNING documento_id`, s.thread, sha, len(pdf), pathDocProva, utente).Scan(&s.doc))

	// la provenienza lega il documento all'allegato da cui e' nato: e' quella che fa comparire «NAS:
	// scritto» accanto alla riga, e quindi anche il pulsante «Anteprima».
	deve(esegui(b, `INSERT INTO documento_provenienza (documento_id, allegato_id, messaggio_id, ricevuto_il, caricato_da)
		VALUES ($1,$2,$3, now(), $4)`, s.doc, s.allegato, msg, utente))

	s.suNas = filepath.Join(s.radice, cartellaRFQProva, pathDocProva)
	if conNas {
		deve(os.MkdirAll(filepath.Dir(s.suNas), 0o755))
		deve(os.WriteFile(s.suNas, pdf, 0o644))
	}
	s.fp = b.browser("10.0.0.5:51000")
	s.fp.login("FP", "prova-fp")
	return s
}

func esegui(b *bancoWeb, sql string, arg ...any) error {
	_, err := b.pool.Exec(b.ctx, sql, arg...)
	return err
}

func shaDi(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func sePresente(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// chiedi e' una richiesta con intestazioni proprie (Range, If-None-Match): `fai` manda form e legge
// testo, qui servono i byte e le intestazioni della risposta.
func (w *browser) chiedi(metodo, percorso string, intestazioni map[string]string) (*http.Response, []byte) {
	w.b.t.Helper()
	r, _ := http.NewRequest(metodo, w.b.srv.URL+percorso, nil)
	r.Header.Set("X-Prova-IP", w.ip)
	for k, v := range intestazioni {
		r.Header.Set(k, v)
	}
	resp, err := w.c.Do(r)
	if err != nil {
		w.b.t.Fatal(err)
	}
	defer resp.Body.Close()
	corpo, _ := io.ReadAll(resp.Body)
	return resp, corpo
}

func (s *scenaAnteprima) url() string { return "/allegato/" + s.allegato.String() + "/anteprima" }

func (s *scenaAnteprima) anomalia(t *testing.T) (db.NasAnomalium, bool) {
	t.Helper()
	a, err := s.b.q.GetAnomaliaNas(s.b.ctx, s.doc)
	return a, err == nil
}

// Lo staging vince: e' sul disco di questo server, il suo nome E' l'hash del contenuto e non c'e'
// niente da verificare. Qui il file sul NAS non esiste nemmeno, e l'anteprima si apre lo stesso —
// senza che nessuno vada a guardare il NAS, e quindi senza che nasca nessuna segnalazione.
func TestLAnteprimaPreferisceLoStaging(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(64*1024), true, false)

	resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), nil)
	if resp.StatusCode != 200 {
		t.Fatalf("stato %d: %s", resp.StatusCode, primi400(string(corpo)))
	}
	if !bytes.Equal(corpo, s.pdf) {
		t.Errorf("il corpo non e' il PDF: %d byte invece di %d", len(corpo), len(s.pdf))
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "inline;") {
		t.Errorf("Content-Disposition = %q: l'anteprima si guarda, non si scarica", cd)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("mancano le intestazioni che impediscono al PDF di eseguire qualcosa: %v", resp.Header)
	}
	da, letti := s.reg.servita(t)
	if da != "staging" {
		t.Errorf("i byte arrivano da %q: con il contenuto in staging non si tocca la rete", da)
	}
	if letti != int64(len(s.pdf))+5 {
		t.Errorf("byte letti = %d, attesi %d (il file piu' i cinque del controllo %%PDF-)", letti, len(s.pdf)+5)
	}
	if a, c := s.anomalia(t); c {
		t.Errorf("e' nata una segnalazione (%s) guardando un file che sta in staging: nessuno e' andato sul NAS", a.Problema)
	}
}

// Senza staging si passa al NAS: il documento di questa RFQ che ha lo stesso contenuto dell'allegato.
func TestSenzaStagingLAnteprimaArrivaDalNas(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(64*1024), false, true)

	resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), nil)
	if resp.StatusCode != 200 {
		t.Fatalf("stato %d: %s", resp.StatusCode, primi400(string(corpo)))
	}
	if !bytes.Equal(corpo, s.pdf) {
		t.Errorf("il corpo non e' il PDF: %d byte invece di %d", len(corpo), len(s.pdf))
	}
	d, err := s.b.q.GetDocumento(s.b.ctx, s.doc)
	if err != nil {
		t.Fatal(err)
	}
	if etag := resp.Header.Get("ETag"); etag != `"`+d.Sha256+`"` {
		t.Errorf("ETag = %q, atteso l'hash gia' in database (%q): ricalcolarlo vorrebbe dire rileggere il file", etag, d.Sha256)
	}
	if da, _ := s.reg.servita(t); da != "NAS" {
		t.Errorf("i byte arrivano da %q", da)
	}
	// Il documento non e' stato riletto, quindi non risulta verificato adesso: l'anteprima non e' una
	// verifica, e non deve spostare la coda del ricognitore.
	if d.VerificatoIl != nil {
		t.Error("servire un'anteprima ha segnato il documento come verificato: nessuno ha ricalcolato l'hash")
	}
}

// LA PROVA DEI BYTE LETTI. Il viewer del browser chiede pezzi: se per servirne uno il server
// rileggesse il file intero — per ricalcolare l'hash, per esempio — si vedrebbe qui e solo qui.
func TestUnRangeNonRileggeTuttoIlFile(t *testing.T) {
	const pezzo = 512 * 1024
	s := preparaAnteprima(t, pdfFinto(2*1024*1024), false, true)

	resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), map[string]string{"Range": fmt.Sprintf("bytes=0-%d", pezzo-1)})
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("stato %d, atteso 206: senza Range un PDF da 60 MB si apre solo dopo averlo scaricato tutto", resp.StatusCode)
	}
	if len(corpo) != pezzo {
		t.Errorf("il pezzo servito e' %d byte, chiesti %d", len(corpo), pezzo)
	}
	if !bytes.Equal(corpo, s.pdf[:pezzo]) {
		t.Error("il pezzo servito non e' l'inizio del file")
	}
	if cr := resp.Header.Get("Content-Range"); cr != fmt.Sprintf("bytes 0-%d/%d", pezzo-1, len(s.pdf)) {
		t.Errorf("Content-Range = %q", cr)
	}
	da, letti := s.reg.servita(t)
	if da != "NAS" {
		t.Fatalf("i byte arrivano da %q", da)
	}
	if letti != pezzo+5 {
		t.Errorf("per servire %d byte ne ha letti %d dal NAS (il file intero e' %d): un Range che rilegge "+
			"tutto e' un Range buttato via", pezzo, letti, len(s.pdf))
	}
}

// Un file toccato dopo l'ultima volta che il server l'ha guardato fa scattare il dubbio: si rilegge
// per intero UNA volta, e se corrisponde si serve — e da quel momento risulta verificato, cosi' la
// volta dopo non si rilegge piu'. E' il prezzo che si paga quando qualcosa e' cambiato davvero, ed e'
// esattamente il prezzo che le altre prove qui sopra dimostrano che NON si paga sempre.
func TestUnFileToccatoDopoLUltimoSguardoSiRilegge(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(256*1024), false, true)
	if _, err := s.b.pool.Exec(s.b.ctx, `UPDATE documento SET scritto_il = now() - interval '1 day' WHERE documento_id=$1`, s.doc); err != nil {
		t.Fatal(err)
	}

	resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), nil)
	if resp.StatusCode != 200 {
		t.Fatalf("stato %d: %s", resp.StatusCode, primi400(string(corpo)))
	}
	if !bytes.Equal(corpo, s.pdf) {
		t.Error("il corpo non e' il PDF")
	}
	_, letti := s.reg.servita(t)
	if letti < int64(len(s.pdf)) {
		t.Errorf("byte letti = %d: il dubbio era scattato e il file non e' stato riletto per intero", letti)
	}
	d, err := s.b.q.GetDocumento(s.b.ctx, s.doc)
	if err != nil {
		t.Fatal(err)
	}
	if d.VerificatoIl == nil {
		t.Fatal("il file e' stato riletto per intero e il documento non risulta verificato: si rileggerebbe a ogni clic")
	}
	if a, c := s.anomalia(t); c {
		t.Errorf("e' nata una segnalazione (%s) su un file che corrisponde", a.Problema)
	}

	// e adesso che risulta guardato, la seconda apertura non rilegge piu' niente
	resp2, _ := s.fp.chiedi(http.MethodGet, s.url(), map[string]string{"Range": "bytes=0-1023"})
	if resp2.StatusCode != http.StatusPartialContent {
		t.Fatalf("seconda apertura: stato %d", resp2.StatusCode)
	}
	if _, letti2 := s.reg.servita(t); letti2 != 1024+5 {
		t.Errorf("la seconda apertura ha letto %d byte per servirne 1024", letti2)
	}
}

// Una segnalazione aperta su quel documento ferma l'anteprima prima di aprire qualsiasi file:
// mostrare come «il disegno» un file che il server stesso ha segnalato sarebbe peggio che non
// mostrare niente.
func TestUnaSegnalazioneApertaFermaLAnteprima(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(32*1024), false, true)
	if _, err := s.b.q.ApriAnomaliaNas(s.b.ctx, db.ApriAnomaliaNasParams{
		DocumentoID: s.doc, ThreadID: s.thread, Problema: db.ProblemaNasConflitto,
		StatoDb: db.StatoNasScritto, Percorso: pathDocProva, ShaAtteso: shaDi(s.pdf),
		Dettaglio: "segnalato dal ricognitore",
	}); err != nil {
		t.Fatal(err)
	}

	resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stato %d, atteso 409: %s", resp.StatusCode, primi400(string(corpo)))
	}
	if bytes.Contains(corpo, []byte("%PDF-")) {
		t.Error("ha servito il file lo stesso")
	}
	if !strings.Contains(string(corpo), "segnalazione") {
		t.Errorf("la risposta non dice perche': %q", primi400(string(corpo)))
	}
}

// IL CASO CHE HA FATTO NASCERE `VerificaFileAperto`. Il file sul NAS non e' piu' quello del
// fascicolo: l'anteprima se ne accorge (la dimensione non torna), rilegge il file intero UNA volta,
// non serve niente e LASCIA SCRITTA la scoperta. Chi ha cliccato chiude la finestra; la segnalazione
// resta in Admin.
func TestSeIlFileNonCorrispondeNasceUnaSegnalazioneENonSiServeNiente(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(128*1024), false, true)
	altro := pdfFinto(96 * 1024) // altro contenuto E altra dimensione: il dubbio scatta senza leggere
	if err := os.WriteFile(s.suNas, altro, 0o644); err != nil {
		t.Fatal(err)
	}

	resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stato %d, atteso 409: %s", resp.StatusCode, primi400(string(corpo)))
	}
	if bytes.Contains(corpo, []byte("%PDF-")) {
		t.Fatal("ha servito un file che non e' il documento del fascicolo")
	}
	a, c := s.anomalia(t)
	if !c {
		t.Fatal("nessuna segnalazione: la scoperta e' andata persa con la finestra di chi ha cliccato")
	}
	if a.Problema != db.ProblemaNasConflitto {
		t.Errorf("problema = %q, atteso conflitto", a.Problema)
	}
	if a.ShaTrovato.String != shaDi(altro) {
		t.Errorf("sha_trovato = %q, atteso quello del file che c'e' davvero", a.ShaTrovato.String)
	}
	if !strings.Contains(a.Percorso, "disegno.pdf") {
		t.Errorf("la segnalazione non dice dove si e' guardato: %q", a.Percorso)
	}
	// Una seconda richiesta non rilegge piu' niente: adesso c'e' una segnalazione aperta, e basta
	// quella a fermare l'anteprima.
	resp2, _ := s.fp.chiedi(http.MethodGet, s.url(), nil)
	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("seconda richiesta: stato %d", resp2.StatusCode)
	}
}

// Il documento dice «scritto» e il file non c'e' piu': il ricognitore lo direbbe alla prossima
// passata, e chi guarda adesso sta guardando adesso.
func TestIlFileSparitoDalNasDiventaUnaSegnalazione(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(32*1024), false, true)
	if err := os.Remove(s.suNas); err != nil {
		t.Fatal(err)
	}

	resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stato %d, atteso 409: %s", resp.StatusCode, primi400(string(corpo)))
	}
	a, c := s.anomalia(t)
	if !c {
		t.Fatal("nessuna segnalazione per un documento che promette un file che non c'e'")
	}
	if a.Problema != db.ProblemaNasMancante {
		t.Errorf("problema = %q, atteso mancante", a.Problema)
	}
}

// Il percorso si ricava dal database e si verifica nel contenimento: una riga cambiata a mano non
// deve poter far servire un file che sta fuori dalla radice del NAS. Non e' un caso d'uso — nessuna
// rotta puo' scrivere quel `path_relativo` — ed e' per questo che il controllo c'e'.
func TestUnPercorsoFuoriDallaRadiceNonSiApre(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(16*1024), false, true)
	fuori := filepath.Join(filepath.Dir(s.radice), "fuori.pdf")
	if err := os.WriteFile(fuori, s.pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.b.pool.Exec(s.b.ctx, `UPDATE documento SET path_relativo=$2 WHERE documento_id=$1`,
		s.doc, `..\..\fuori.pdf`); err != nil {
		t.Fatal(err)
	}

	resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), nil)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("stato %d, atteso 500: un percorso fuori radice e' un errore del server, non un «non trovato»", resp.StatusCode)
	}
	if bytes.Contains(corpo, []byte("%PDF-")) {
		t.Fatal("ha servito un file che sta fuori dalla radice del NAS")
	}
}

// Si serve solo cio' che comincia per %PDF-: non l'estensione, non il tipo dichiarato da qualcuno.
func TestNonSiServeCioCheNonEUnPDF(t *testing.T) {
	finto := []byte("PK\x03\x04 questo e' uno zip con il nome sbagliato")
	s := preparaAnteprima(t, finto, true, false)

	resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), nil)
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("stato %d, atteso 415: %s", resp.StatusCode, primi400(string(corpo)))
	}
	if bytes.Contains(corpo, []byte("PK")) {
		t.Error("ha servito il contenuto lo stesso")
	}
}

// La seconda apertura non rilegge il file: l'ETag e' l'hash che il database ha gia', e un 304 costa
// i cinque byte del controllo e niente altro.
func TestLaSecondaAperturaNonRileggeIlFile(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(256*1024), false, true)
	resp, _ := s.fp.chiedi(http.MethodGet, s.url(), nil)
	etag := resp.Header.Get("ETag")
	if etag == "" {
		t.Fatal("nessun ETag")
	}

	resp2, corpo2 := s.fp.chiedi(http.MethodGet, s.url(), map[string]string{"If-None-Match": etag})
	if resp2.StatusCode != http.StatusNotModified {
		t.Fatalf("stato %d, atteso 304", resp2.StatusCode)
	}
	if len(corpo2) != 0 {
		t.Errorf("un 304 con %d byte di corpo", len(corpo2))
	}
	if _, letti := s.reg.servita(t); letti != 5 {
		t.Errorf("per rispondere «non e' cambiato» ha letto %d byte dal NAS", letti)
	}
}

// Un allegato che non e' ancora in nessuna RFQ non ha un file da nessuna parte: lo dice, e dice cosa
// si puo' fare (il pulsante «Riscarica» c'e' gia', nella stessa riga).
func TestUnAllegatoSenzaContenutoDiceDiRiscaricare(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(8*1024), false, false)

	resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), nil)
	if resp.StatusCode != http.StatusConflict && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("stato %d: %s", resp.StatusCode, primi400(string(corpo)))
	}
	if bytes.Contains(corpo, []byte("%PDF-")) {
		t.Error("ha servito qualcosa")
	}
}

// L'anteprima e' dietro la sessione come tutto il resto: senza login non si apre.
func TestSenzaSessioneLAnteprimaNonSiApre(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(8*1024), true, false)
	anonimo := s.b.browser("10.0.0.9:51000")

	resp, corpo := anonimo.chiedi(http.MethodGet, s.url(), nil)
	if resp.StatusCode == 200 {
		t.Fatalf("un browser senza sessione ha ricevuto il file (%d byte)", len(corpo))
	}
	if bytes.Contains(corpo, []byte("%PDF-")) {
		t.Error("il file e' uscito lo stesso")
	}
}

// ─── I quattro micro-fix di chiusura di B8.1 ────────────────────────────────────────────────────

// Lo staging ha lo stesso contenimento del NAS. Il file fuori dalla radice e' un PDF vero, leggibile,
// e il NAS ha lo stesso contenuto a disposizione: la prova chiede che la rotta si FERMI — 500, niente
// byte — e non che vada a prenderlo altrove in silenzio. Un percorso che esce dalla radice e' un bug,
// e un bug coperto da un ripiego che funziona non lo trova piu' nessuno.
func TestUnPercorsoDiStagingFuoriDallaRadiceNonSiApre(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(16*1024), true, true)
	fuori := filepath.Join(filepath.Dir(s.radiceStaging), "fuori-dallo-staging.pdf")
	if err := os.WriteFile(fuori, s.pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(fuori) })
	// Composto come lo comporrebbe un codice sbagliato: dalla radice giusta, risalendo.
	risalita := filepath.Join(s.radiceStaging, "_contenuti") + `\..\..\fuori-dallo-staging.pdf`
	for _, p := range []string{fuori, risalita} {
		if _, err := s.b.pool.Exec(s.b.ctx, `UPDATE allegato SET path_staging=$2 WHERE allegato_id=$1`, s.allegato, p); err != nil {
			t.Fatal(err)
		}
		resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), nil)
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("path_staging=%q: stato %d, atteso 500 — un percorso fuori dallo staging e' un errore "+
				"del server, e ripiegare sul NAS lo nasconderebbe", p, resp.StatusCode)
		}
		if bytes.Contains(corpo, []byte("%PDF-")) {
			t.Fatalf("path_staging=%q: ha servito un file che sta fuori dallo staging", p)
		}
	}
	if !s.reg.contiene("percorso fuori radice") {
		t.Errorf("il 500 non dice nel log perche': chi lo legge deve trovare il percorso che esce dalla radice")
	}
}

// Senza la radice dichiarata non si verifica niente, e cio' che non si verifica non si serve: lo
// staging si salta (lo dice il log), e se il documento e' sul NAS l'anteprima arriva da li'.
func TestSenzaRadiceDelloStagingNonSiServeDalloStaging(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(16*1024), true, true)
	s.b.ws.Staging = ""

	resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), nil)
	if resp.StatusCode != 200 {
		t.Fatalf("stato %d: %s", resp.StatusCode, primi400(string(corpo)))
	}
	if da, _ := s.reg.servita(t); da != "NAS" {
		t.Errorf("i byte arrivano da %q: senza radice dichiarata lo staging non si puo' verificare", da)
	}
	if !s.reg.contiene("radice dello staging non e' dichiarata") {
		t.Error("il salto dello staging non e' nel log: un server configurato male deve dirlo")
	}
}

// Il nome accentato arriva intero: `filename*` in UTF-8 percentuale, e `filename=` in solo ASCII come
// ripiego. Il nome e' quello di un disegno vero come se ne ricevono: trattino lungo, lettera accentata
// maiuscola, spazi.
func TestIlNomeAccentatoArrivaInteroAlBrowser(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(8*1024), true, false)
	nome := "Staffa – rev À.pdf"
	if _, err := s.b.pool.Exec(s.b.ctx, `UPDATE allegato SET nome_file=$2 WHERE allegato_id=$1`, s.allegato, nome); err != nil {
		t.Fatal(err)
	}

	resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), nil)
	if resp.StatusCode != 200 {
		t.Fatalf("stato %d: %s", resp.StatusCode, primi400(string(corpo)))
	}
	cd := resp.Header.Get("Content-Disposition")
	for i := 0; i < len(cd); i++ {
		if cd[i] < 0x20 || cd[i] > 0x7e {
			t.Fatalf("Content-Disposition contiene il byte 0x%02x: un'intestazione HTTP con byte non-ASCII "+
				"ogni browser la legge a modo suo (%q)", cd[i], cd)
		}
	}
	const prefisso = "filename*=UTF-8''"
	i := strings.Index(cd, prefisso)
	if i < 0 {
		t.Fatalf("manca filename*: il nome accentato non arriva (%q)", cd)
	}
	codificato := cd[i+len(prefisso):]
	if j := strings.IndexByte(codificato, ';'); j >= 0 {
		codificato = codificato[:j]
	}
	decodificato, err := url.PathUnescape(codificato)
	if err != nil {
		t.Fatalf("filename* non si decodifica: %v (%q)", err, codificato)
	}
	if decodificato != nome {
		t.Errorf("il browser ricostruisce %q invece di %q", decodificato, nome)
	}
	if !strings.HasPrefix(cd, `inline; filename="`) {
		t.Errorf("manca il ripiego ASCII per chi non conosce filename*: %q", cd)
	}
}

// «Non c'e' la riga» e «il database non risponde» sono due cose. La prima e' un 404 onesto; la
// seconda, detta come «allegato non trovato», manderebbe chi guarda a cercare un problema che non c'e'.
// Il database che non risponde si ottiene davvero: un pool chiuso, sotto un server vivo.
func TestUnDatabaseCheNonRispondeNonEUnAllegatoCheNonCE(t *testing.T) {
	s := preparaAnteprima(t, pdfFinto(8*1024), true, false)

	// La riga che non c'e': un identificativo che nessuno ha mai scritto.
	resp, corpo := s.fp.chiedi(http.MethodGet, "/allegato/"+uuid.New().String()+"/anteprima", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("allegato inesistente: stato %d, atteso 404 (%s)", resp.StatusCode, primi400(string(corpo)))
	}

	// Il database che non risponde. Il controllo della sessione passa dallo stesso pool, e con il pool
	// chiuso la richiesta si fermerebbe li' — la prova passerebbe senza aver chiesto niente alla rotta.
	// Quindi si chiama la rotta e basta, con un server identico a quello vero tranne il pool.
	chiuso := testutil.Pool(t)
	chiuso.Close()
	ws := *s.b.ws
	ws.Pool = chiuso
	req := httptest.NewRequest(http.MethodGet, s.url(), nil)
	req.SetPathValue("id", s.allegato.String())
	rec := httptest.NewRecorder()
	ws.anteprima(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatalf("un database che non risponde e' diventato «allegato non trovato»: %s", primi400(rec.Body.String()))
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("stato %d, atteso 500: un guasto del database e' un errore del server", rec.Code)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("%PDF-")) {
		t.Error("e' uscito un file senza che il database potesse dire di chi fosse")
	}
	if !s.reg.contiene("anteprima non servita") {
		t.Error("il guasto non e' nel log")
	}
}

// Le tre query di dalNas, dal lato «la riga non c'e'»: l'allegato non agganciato, il documento che non
// esiste per quel contenuto, la RFQ senza cartella. Nessuno dei tre e' un guasto, e nessuno dei tre
// deve diventare un 500 adesso che i guasti non si confondono piu' con le assenze.
func TestLeAssenzeInDalNasRestanoAssenze(t *testing.T) {
	casi := []struct {
		nome string
		sql  string
	}{
		{"documento con un altro contenuto", `UPDATE documento SET sha256=repeat('0',64) WHERE thread_id=$1`},
		{"RFQ senza cartella sul NAS", `UPDATE thread_offerta SET cartella_relativa=NULL WHERE thread_id=$1`},
		{"messaggio non agganciato", `UPDATE messaggio SET thread_id=NULL WHERE thread_id=$1`},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			s := preparaAnteprima(t, pdfFinto(8*1024), false, true)
			if _, err := s.b.pool.Exec(s.b.ctx, c.sql, s.thread); err != nil {
				t.Fatal(err)
			}
			resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), nil)
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("stato %d, atteso 404 «usa Riscarica»: %s", resp.StatusCode, primi400(string(corpo)))
			}
			if s.reg.contiene("level=ERROR") {
				t.Errorf("un'assenza e' finita nel log come errore:\n%s", s.reg.tutto())
			}
		})
	}
}

// Un file piu' corto di cinque byte non e' un guasto: e' un file che non e' un PDF. 415, non 500.
// (Il guasto vero — la lettura che si interrompe — e' provato a L1 su pdfDavvero, con un lettore che
// si rompe: sul disco di una prova non lo si provoca in modo ripetibile.)
func TestUnFileCortissimoNonEUnGuasto(t *testing.T) {
	for _, contenuto := range [][]byte{{}, []byte("%PD")} {
		t.Run(fmt.Sprintf("%d byte", len(contenuto)), func(t *testing.T) {
			s := preparaAnteprima(t, contenuto, true, false)
			resp, corpo := s.fp.chiedi(http.MethodGet, s.url(), nil)
			if resp.StatusCode != http.StatusUnsupportedMediaType {
				t.Errorf("stato %d, atteso 415 (%s)", resp.StatusCode, primi400(string(corpo)))
			}
			if s.reg.contiene("level=ERROR") {
				t.Errorf("un file che non e' un PDF e' finito nel log come guasto:\n%s", s.reg.tutto())
			}
		})
	}
}

func (r *registro) contiene(s string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, riga := range r.righe {
		if strings.Contains(riga, s) {
			return true
		}
	}
	return false
}

func (r *registro) tutto() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.righe, "")
}
