//go:build integrazione

package web

// L4 — V1, ondata B, corsia file, secondo passaggio: le prove dei rilievi della revisione sulle rotte della Sorgente.
// S5 in sola lettura anche quando i byte vengono dal NAS (EM §8.5, §10.9; PP-08 e T-04 con il documento scritto il
// cui file manca o e' stato toccato), X-Motivo su ogni risposta d'errore di S5 (PE-29), E1′ con il 413 prima del
// corpo, le parti senza nome nel limite dei file e i nomi che il database non accetta, la conferma del lotto su un file
// in Altri file (AF-10). DB di prova, staging, deposito e NAS temporanei; nomi e contenuti sintetici (ACME).

import (
	"bufio"
	"bytes"
	"fmt"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/nas"
)

// documentoSulNas: un file attivo della RFQ senza byte nello staging, e il documento confermato della RFQ con lo stesso
// contenuto scritto sul NAS (verificato ieri). sulNas: i byte da mettere sul NAS (nil = nessun file). Restituisce
// l'allegato, il documento e il file sul NAS.
func (s *bancoSorgente) documentoSulNas(t *testing.T, radiceNAS, nome string, c, sulNas []byte) (uuid.UUID, uuid.UUID, string) {
	t.Helper()
	a, sha := s.file(s.thread, nome, c, db.OrigineAllegatoOutlook)
	var p string
	if err := s.pool.QueryRow(s.ctx, `SELECT path_staging FROM allegato WHERE allegato_id = $1`, a).Scan(&p); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(s.ctx, `UPDATE allegato SET path_staging = NULL WHERE allegato_id = $1`, a); err != nil {
		t.Fatal(err)
	}
	var cartella string
	if err := s.pool.QueryRow(s.ctx, `SELECT cartella_relativa FROM thread_offerta WHERE thread_id = $1`, s.thread).Scan(&cartella); err != nil {
		t.Fatal(err)
	}
	codice := strings.TrimSuffix(nome, filepath.Ext(nome))
	rel := `ELENCO DISEGNI\` + codice + `\` + nome
	var doc uuid.UUID
	if err := s.pool.QueryRow(s.ctx, `INSERT INTO documento (thread_id, tipo, codice, nome_file, estensione, sha256, bytes, path_relativo,
		stato_nas, scritto_il, verificato_il, confermato_da, confermato_il)
		VALUES ($1, 'disegno_2d', $2, $3, 'pdf', $4, $5, $6, 'scritto', now() - interval '1 day', now() - interval '1 day',
		        (SELECT utente_id FROM utente WHERE sigla = 'AD'), now() - interval '1 day') RETURNING documento_id`,
		s.thread, codice, nome, sha, len(c), rel).Scan(&doc); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(radiceNAS, cartella, rel)
	if sulNas != nil {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, sulNas, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return a, doc, f
}

// Rilievo bloccante (EM §8.5: «nessuna scrittura nelle letture», [U] D-18; EM §10.9 S5 «Effetti: nessuno»): S5 con i
// byte dal NAS non scrive niente nemmeno quando il documento scritto non torna. Il file che manca, una cartella al suo
// posto, un file toccato dopo l'ultima verifica con un altro contenuto: niente nas_anomalia, niente verificato_il, e S5
// risponde 410 (nessun'altra sorgente). Un file toccato con lo stesso contenuto si rilegge per lo sha e si serve, senza
// registrare la verifica. PP-08 e T-04 per questi rami.
func TestS5DalNASInSolaLettura(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	radiceNAS := t.TempDir()
	b.ws.NAS = &nas.Scrittore{Radice: radiceNAS}
	t.Cleanup(func() { b.ws.NAS = nil })
	buono := byteCasuali("x.pdf", 200, 3_000)
	aBuono, _, fBuono := s.documentoSulNas(t, radiceNAS, "ACME-NAS1.pdf", buono, buono)
	mancante := byteCasuali("x.pdf", 201, 3_000)
	aMancante, _, _ := s.documentoSulNas(t, radiceNAS, "ACME-NAS2.pdf", mancante, nil)
	cambiato := byteCasuali("x.pdf", 202, 3_000)
	altro := byteCasuali("x.pdf", 203, 3_000) // stessa dimensione, un altro contenuto
	aCambiato, _, _ := s.documentoSulNas(t, radiceNAS, "ACME-NAS3.pdf", cambiato, altro)
	cartella := byteCasuali("x.pdf", 204, 3_000)
	aCartella, _, fCartella := s.documentoSulNas(t, radiceNAS, "ACME-NAS4.pdf", cartella, nil)
	if err := os.MkdirAll(fCartella, 0o755); err != nil { // al posto del file, una cartella con lo stesso nome
		t.Fatal(err)
	}
	// il file buono «toccato» adesso: e' in dubbio, si rilegge per lo sha
	ora := time.Now()
	if err := os.Chtimes(fBuono, ora, ora); err != nil {
		t.Fatal(err)
	}
	prima := fotoDelDatabase(t, b)
	r := s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, aBuono, "contenuto"), nil, nil)
	if r.stato != 200 || !bytes.Equal(r.corpo, buono) || r.h.Get("X-Motivo") != "" {
		t.Errorf("S5 dal NAS con il file toccato e lo sha giusto: %d %q", r.stato, r.h.Get("X-Motivo"))
	}
	for _, c := range []struct {
		nome string
		a    uuid.UUID
	}{{"file mancante", aMancante}, {"file con un altro contenuto", aCambiato}, {"una cartella al posto del file", aCartella}} {
		r := s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, c.a, "contenuto"), nil, nil)
		if m := r.mappa(t); r.stato != http.StatusGone || r.h.Get("X-Motivo") != "BYTE_NON_DISPONIBILI" || m["codice"] != "BYTE_NON_DISPONIBILI" {
			t.Errorf("%s: %d %q %v", c.nome, r.stato, r.h.Get("X-Motivo"), m)
		}
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("S5 dal NAS ha scritto: %s", d)
	}
	var anomalie int
	if err := s.pool.QueryRow(s.ctx, `SELECT count(*) FROM nas_anomalia`).Scan(&anomalie); err != nil || anomalie != 0 {
		t.Errorf("anomalie NAS aperte da S5: %d %v", anomalie, err)
	}
}

// Rilievo minore (PE-29, EM §10.9): ogni risposta d'errore di S5 porta X-Motivo, anche quelle che non scrive il
// gestore: il 403 di un operatore, il 405 della riga gemella, il 416 di un Range fuori misura. Una risposta riuscita no.
func TestR30XMotivoSuOgniErroreDiS5(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	pdf := byteCasuali("x.pdf", 210, 3_000)
	a, _ := s.file(s.thread, "ACME-XM.pdf", pdf, db.OrigineAllegatoOutlook)
	if r := s.chiama(s.op, http.MethodGet, rottaFile(s.thread, a, "contenuto"), nil, nil); r.stato != http.StatusForbidden ||
		r.h.Get("X-Motivo") != "NON_AUTORIZZATO" {
		t.Errorf("operatore: %d X-Motivo %q", r.stato, r.h.Get("X-Motivo"))
	}
	anonimo := b.browser("10.0.0.7")
	if r := s.chiama(anonimo, http.MethodDelete, rottaFile(s.thread, a, "contenuto"), nil, nil); r.stato != http.StatusMethodNotAllowed ||
		r.h.Get("X-Motivo") != "METODO_NON_AMMESSO" {
		t.Errorf("riga gemella: %d X-Motivo %q", r.stato, r.h.Get("X-Motivo"))
	}
	if r := s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, a, "contenuto"), nil, map[string]string{"Range": "bytes=999999-"}); r.stato != http.StatusRequestedRangeNotSatisfiable ||
		r.h.Get("X-Motivo") != "RANGE_NON_VALIDO" {
		t.Errorf("Range fuori misura: %d X-Motivo %q", r.stato, r.h.Get("X-Motivo"))
	}
	if r := s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, a, "contenuto"), nil, map[string]string{"Range": "bytes=0-9"}); r.stato != http.StatusPartialContent ||
		r.h.Get("X-Motivo") != "" {
		t.Errorf("Range buono: %d X-Motivo %q", r.stato, r.h.Get("X-Motivo"))
	}
	// a flag spento resta il 404 del router, senza intestazioni in piu'
	b.ws.MotoreA.Accesa = false
	if r := s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, a, "contenuto"), nil, nil); r.stato != http.StatusNotFound || r.h.Get("X-Motivo") != "" {
		t.Errorf("flag spento: %d X-Motivo %q", r.stato, r.h.Get("X-Motivo"))
	}
	b.ws.MotoreA.Accesa = true
}

// Rilievo minore (EM §10.9: 413 fra gli errori di E1′): una richiesta che dichiara di superare il limite totale si
// rifiuta con 413 TROPPO_GRANDE prima di leggere il corpo, senza scrivere niente nello staging ne' nel database.
func TestE1PrimoTroppoGrandePrimaDelCorpo(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	prima := fotoDelDatabase(t, b)
	u, _ := url.Parse(s.srv.URL)
	var cs []string
	for _, c := range s.ad.c.Jar.Cookies(u) {
		cs = append(cs, c.Name+"="+c.Value)
	}
	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	dichiarato := b.ws.maxRichiestaCaricamento() + (2 << 20)
	richiesta := fmt.Sprintf("POST /thread/%s/carica HTTP/1.1\r\nHost: %s\r\nCookie: %s\r\nAccept: application/json\r\nX-Prova-IP: %s\r\n"+
		"Content-Type: multipart/form-data; boundary=acmeconfine\r\nContent-Length: %d\r\n\r\n--acmeconfine\r\n",
		s.thread, u.Host, strings.Join(cs, "; "), s.ad.ip, dichiarato)
	if _, err := conn.Write([]byte(richiesta)); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge || resp.Header.Get("X-Motivo") != "TROPPO_GRANDE" {
		t.Errorf("richiesta oltre il limite: %d X-Motivo %q", resp.StatusCode, resp.Header.Get("X-Motivo"))
	}
	if p := s.partiDelloStaging(); len(p) != 0 {
		t.Errorf("parziali nello staging dopo il 413: %v", p)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("il 413 ha scritto: %s", d)
	}
}

// Rilievi minori di E1′: le parti «file» senza nome contano nel limite dei 50 file (la risposta non cresce senza fine);
// un nome che il database non accetterebbe (oltre 300 caratteri, caratteri di controllo) e' un rifiuto del solo file,
// e gli altri file si registrano.
func TestE1PartiSenzaNomeENomiNonValidi(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	// 60 parti «file» senza nome: 50 SENZA_NOME, poi TROPPO_FILE, e la lettura si ferma
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("origine", "portale")
	for i := 0; i < 60; i++ {
		f, _ := mw.CreateFormFile("file", "")
		_, _ = f.Write([]byte("x"))
	}
	_ = mw.Close()
	r := s.caricaMultipart(s.ad, s.thread, mw.FormDataContentType(), &buf)
	m := r.mappa(t)
	file, _ := m["file"].([]any)
	if r.stato != http.StatusUnprocessableEntity || m["codice"] != "NESSUN_FILE_CARICATO" || len(file) != 51 {
		t.Fatalf("60 parti senza nome: %d %v, %d esiti", r.stato, m["codice"], len(file))
	}
	if ultimo := file[50].(map[string]any)["motivo"].(map[string]any); ultimo["codice"] != "TROPPO_FILE" {
		t.Errorf("l'ultimo esito: %v", ultimo)
	}
	// un nome troppo lungo e un file buono: il primo rifiutato da solo, il secondo caricato
	lungo := strings.Repeat("A", 297) + ".pdf"
	buono := byteCasuali("x.pdf", 220, 1_500)
	r = s.carica(s.ad, s.thread, true, "portale", map[string][]byte{lungo: byteCasuali("x.pdf", 221, 1_500), "ACME-OK.pdf": buono}, nil)
	m = r.mappa(t)
	if r.stato != 200 || m["esito"] != "parziale" {
		t.Fatalf("nome lungo e file buono: %d %v", r.stato, m)
	}
	esiti := map[string]map[string]any{}
	for _, x := range m["file"].([]any) {
		f := x.(map[string]any)
		esiti[f["esito"].(string)] = f
	}
	if mot, _ := esiti["rifiutato"]["motivo"].(map[string]any); mot["codice"] != "SENZA_NOME" || !strings.Contains(mot["testo"].(string), "300") {
		t.Errorf("il nome lungo: %v", esiti["rifiutato"])
	}
	if esiti["caricato"]["nome_file"] != "ACME-OK.pdf" || esiti["caricato"]["origine"] != "portale" {
		t.Errorf("il file buono: %v", esiti["caricato"])
	}
	var n int
	if err := s.pool.QueryRow(s.ctx, `SELECT count(*) FROM allegato WHERE char_length(nome_file) > 300`).Scan(&n); err != nil || n != 0 {
		t.Errorf("allegati con un nome oltre 300 caratteri: %d %v", n, err)
	}
	// l'origine di ogni file nella risposta e' quella della richiesta (portale | interna), come in testa
	r = s.carica(s.ad, s.thread, true, "interna", map[string][]byte{"ACME-INT.stp": byteCasuali("x.stp", 222, 1_500)}, nil)
	if m := r.mappa(t); r.stato != 200 || m["origine"] != "interna" || m["file"].([]any)[0].(map[string]any)["origine"] != "interna" {
		t.Errorf("caricamento interno: %d %v", r.stato, m)
	}
	if p := s.partiDelloStaging(); len(p) != 0 {
		t.Errorf("parziali rimasti: %v", p)
	}
}

// caricaMultipart: E1′ con un corpo multipart gia' composto.
func (s *bancoSorgente) caricaMultipart(w *browser, thread uuid.UUID, tipo string, corpo *bytes.Buffer) risposta {
	s.t.Helper()
	r, _ := http.NewRequest(http.MethodPost, s.srv.URL+"/thread/"+thread.String()+"/carica", corpo)
	r.Header.Set("Content-Type", tipo)
	r.Header.Set("Accept", "application/json")
	r.Header.Set("X-Prova-IP", w.ip)
	resp, err := w.c.Do(r)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	var b bytes.Buffer
	_, _ = b.ReadFrom(resp.Body)
	return risposta{stato: resp.StatusCode, h: resp.Header, corpo: b.Bytes()}
}

// AF-10, la conferma del lotto (rilievo della revisione: mancava la prova): «✓ Conferma» dal Fascicolo su un file in
// Altri file rifiuta con «Niente è cambiato», senza scrivere.
func TestAF10ConfermaDelLottoSuUnFileInAltriFile(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	a, _ := s.file(s.thread, "ACME-AF10L.pdf", byteCasuali("x.pdf", 230, 2_000), db.OrigineAllegatoOutlook)
	if r, _ := s.gesto(s.thread, a, "altri-file", "", nil); r.stato != 200 {
		t.Fatalf("Altri file: %d %s", r.stato, r.corpo)
	}
	var pid uuid.UUID
	if err := s.pool.QueryRow(s.ctx, `SELECT proposta_id FROM documento_proposta WHERE allegato_id = $1`, a).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	prima := fotoDominio(t, b)
	resp, testo := s.op.fai(http.MethodPost, "/thread/"+s.thread.String()+"/fascicolo/conferma", url.Values{"voce": {pid.String()}}, true)
	if !strings.Contains(testo, "Niente è cambiato") {
		t.Errorf("conferma del lotto su un file in Altri file: %d, %q", resp.StatusCode, avvisoF(testo))
	}
	if d := differenze(prima, fotoDominio(t, b)); d != "" {
		t.Errorf("la conferma del lotto ha scritto: %s", d)
	}
}
