//go:build integrazione

package web

// L4 — V1, corsia file: le rotte della Sorgente (S1-S5, EM §10.9) e il ramo JSON di «carica» (E1′), sul server montato
// come in produzione (CSRF, catena, workerapi come strada d'ingresso, il servizio del ciclo dei file con lo staging e
// il deposito temporanei). PM23 §10 righe 7, 8, 9 e 13: PP-01…PP-08, PP-11, R-20, R-20b, R-21 (la parte di S1), R-30,
// AF-02 (la rotta), AF-08, AF-10, CO-04; T-15 (nessun percorso nelle risposte JSON) in ogni risposta letta qui.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
	"promatec/cockpit/internal/platform/testutil"
	"promatec/cockpit/internal/transport/workerapi"
)

// bancoSorgente: il banco web con la Sorgente montata.
type bancoSorgente struct {
	*bancoWeb
	svc        *filerfq.Servizio
	wa         *workerapi.Server
	ad, op, co *browser
	cliente    uuid.UUID
	thread     uuid.UUID
	n          int
}

// sorgente: il banco web, la strada dell'analisi e lo staging veri (temporanei), il deposito temporaneo, il servizio del
// ciclo dei file; acceso: il flag di [motore_a].
func (b *bancoWeb) sorgente(t *testing.T, acceso bool) *bancoSorgente {
	t.Helper()
	if acceso {
		acmeAccendi(t, b)
	}
	wa := b.caricamentoAcceso(t)
	dep, err := filerfq.PreparaDeposito(b.ctx, b.q, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := &filerfq.Servizio{Pool: b.pool, Staging: b.ws.Staging, Deposito: dep, Pipeline: wa, Log: testutil.LogSilenzioso(),
		EtaMinima: time.Nanosecond}
	b.ws.CicloFile = svc
	t.Cleanup(func() { b.ws.CicloFile = nil })
	s := &bancoSorgente{bancoWeb: b, svc: svc, wa: wa}
	s.ad = b.browser("10.0.0.8")
	s.ad.login("AD", "prova-ad")
	s.op = operatore(b)
	s.co = b.browser("10.0.0.9")
	s.co.login("CO", "prova-co")
	s.cliente = b.unCliente("Acme Prova", "ACME", "acme.example")
	s.thread = s.rfq("RFQ-A")
	return s
}

func (s *bancoSorgente) rfq(chiave string) uuid.UUID {
	s.t.Helper()
	var id uuid.UUID
	if err := s.pool.QueryRow(s.ctx, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa, priorita)
		VALUES ($1, 'outlook', now(), $2, $3, 1) RETURNING thread_id`, s.cliente, "RFQ "+chiave, `ACME\WIP\2026 10 18 `+chiave).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	return id
}

// file: un allegato di un messaggio della RFQ con i byte nel posto del contenuto dello staging e la proposta aperta.
func (s *bancoSorgente) file(thread uuid.UUID, nome string, c []byte, origine db.OrigineAllegato) (uuid.UUID, string) {
	s.t.Helper()
	s.n++
	msg := s.messaggioIn(fmt.Sprintf("SORG-%d", s.n), s.commerciale)
	if _, err := s.pool.Exec(s.ctx, `UPDATE messaggio SET thread_id = $2 WHERE messaggio_id = $1`, msg, thread); err != nil {
		s.t.Fatal(err)
	}
	var sha string
	var p *string
	var n *int64
	if c != nil {
		sha = shaDiByte(c)
		pc, err := staging.PercorsoContenuto(s.ws.Staging, sha, nome)
		if err != nil {
			s.t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(pc), 0o755); err != nil {
			s.t.Fatal(err)
		}
		if err := os.WriteFile(pc, c, 0o644); err != nil {
			s.t.Fatal(err)
		}
		p = &pc
		l := int64(len(c))
		n = &l
	}
	var shaP *string
	if sha != "" {
		shaP = &sha
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(nome), "."))
	var caricato *uuid.UUID
	if origine != db.OrigineAllegatoOutlook {
		var u uuid.UUID
		if err := s.pool.QueryRow(s.ctx, `SELECT utente_id FROM utente WHERE sigla = 'AD'`).Scan(&u); err != nil {
			s.t.Fatal(err)
		}
		caricato = &u
	}
	stato := "analizzato"
	if c == nil {
		stato = "grezzo"
	}
	var id uuid.UUID
	if err := s.pool.QueryRow(s.ctx, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256,
		path_staging, stato, ricevuto_il, caricato_da) VALUES ($1, $2, $3, $4, 'file', $5, $6, $7, $8, $9::stato_allegato, now(), $10)
		RETURNING allegato_id`, msg, s.n, nome, ext, origine, n, shaP, p, stato, caricato).Scan(&id); err != nil {
		s.t.Fatal(err)
	}
	tipo := "da_determinare"
	if ext == "pdf" {
		tipo = "disegno_2d"
	}
	if _, err := s.pool.Exec(s.ctx, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte)
		VALUES ($1, $2, $3::tipo_documento, 60, 'estensione')`, id, thread, tipo); err != nil {
		s.t.Fatal(err)
	}
	return id, sha
}

func shaDiByte(c []byte) string {
	h := sha256.Sum256(c)
	return hex.EncodeToString(h[:])
}

func byteCasuali(nome string, seme int64, n int) []byte {
	c := make([]byte, n)
	rand.New(rand.NewSource(seme)).Read(c)
	if strings.HasSuffix(strings.ToLower(nome), ".pdf") {
		return append([]byte("%PDF-1.4\n"), c...)
	}
	return c
}

// risposta: stato, intestazioni e corpo di una risposta.
type risposta struct {
	stato int
	h     http.Header
	corpo []byte
}

// rePercorsoNelJSON: una lettera di unita' o un percorso UNC in un testo JSON (le barre rovesciate vi sono raddoppiate).
var rePercorsoNelJSON = regexp.MustCompile(`[A-Za-z]:\\\\|\\\\\\\\`)

// mappa: il corpo JSON; T-15: nessun percorso nella risposta.
func (r risposta) mappa(t *testing.T) map[string]any {
	t.Helper()
	if rePercorsoNelJSON.Match(r.corpo) {
		t.Errorf("T-15: un percorso nella risposta: %s", r.corpo)
	}
	var m map[string]any
	if err := json.Unmarshal(r.corpo, &m); err != nil {
		t.Fatalf("risposta non JSON (%d): %v\n%s", r.stato, err, r.corpo)
	}
	return m
}

// chiama: una richiesta di un browser; corpo JSON se c != nil (string = gia' JSON); intestazioni in piu'.
func (s *bancoSorgente) chiama(w *browser, metodo, percorso string, c any, intest map[string]string) risposta {
	s.t.Helper()
	var corpo io.Reader
	switch v := c.(type) {
	case nil:
	case string:
		corpo = strings.NewReader(v)
	default:
		j, err := json.Marshal(v)
		if err != nil {
			s.t.Fatal(err)
		}
		corpo = bytes.NewReader(j)
	}
	r, _ := http.NewRequest(metodo, s.srv.URL+percorso, corpo)
	if c != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	r.Header.Set("Accept", "application/json")
	r.Header.Set("X-Prova-IP", w.ip)
	for k, v := range intest {
		r.Header.Set(k, v)
	}
	resp, err := w.c.Do(r)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return risposta{stato: resp.StatusCode, h: resp.Header, corpo: b}
}

// carica: E1′ (o il ramo HTML senza json), multipart con l'origine e i file.
func (s *bancoSorgente) carica(w *browser, thread uuid.UUID, json bool, origine string, file map[string][]byte, campi map[string]string) risposta {
	s.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if origine != "" {
		_ = mw.WriteField("origine", origine)
	}
	for k, v := range campi {
		_ = mw.WriteField(k, v)
	}
	for nome, c := range file {
		f, _ := mw.CreateFormFile("file", nome)
		_, _ = f.Write(c)
	}
	_ = mw.Close()
	r, _ := http.NewRequest(http.MethodPost, s.srv.URL+"/thread/"+thread.String()+"/carica", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	if json {
		r.Header.Set("Accept", "application/json")
	} else {
		r.Header.Set("HX-Request", "true")
	}
	r.Header.Set("X-Prova-IP", w.ip)
	resp, err := w.c.Do(r)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return risposta{stato: resp.StatusCode, h: resp.Header, corpo: b}
}

func base(thread uuid.UUID) string { return "/thread/" + thread.String() + "/sorgente" }

func rottaFile(thread, allegato uuid.UUID, gesto string) string {
	return base(thread) + "/file/" + allegato.String() + "/" + gesto
}

// modello: S1 dalla rotta, con il dettaglio del file.
func (s *bancoSorgente) modello(thread, allegato uuid.UUID) map[string]any {
	s.t.Helper()
	r := s.chiama(s.ad, http.MethodGet, base(thread)+"?scartati=1&file="+allegato.String(), nil, nil)
	if r.stato != 200 {
		s.t.Fatalf("S1: %d %s", r.stato, r.corpo)
	}
	return r.mappa(s.t)
}

func fileNelModello(m map[string]any, allegato uuid.UUID) map[string]any {
	l, _ := m["file"].([]any)
	for _, x := range l {
		f, _ := x.(map[string]any)
		if f["allegato_id"] == allegato.String() {
			return f
		}
	}
	return nil
}

// corpo: il corpo di un gesto come lo compone la pagina dal modello (versione, impronta, perimetro).
func (s *bancoSorgente) corpo(thread, allegato uuid.UUID, gesto, motivo string, extra map[string]any) map[string]any {
	s.t.Helper()
	m := s.modello(thread, allegato)
	f := fileNelModello(m, allegato)
	if f == nil {
		s.t.Fatalf("il file %s non e' nel modello", allegato)
	}
	c := map[string]any{"versione_attesa": f["versione"], "chiave_idempotenza": uuid.New().String()}
	if motivo != "" {
		c["motivo"] = motivo
	}
	d, _ := m["dettaglio"].(map[string]any)
	if gesto != "riattiva" && d != nil {
		c["impronta_dipendenze"] = d["impronta_dipendenze"]
	}
	if d != nil && d["n_voci"] != nil {
		c["n_voci"], c["impronta_voci"] = d["n_voci"], d["impronta_voci"]
	}
	for k, v := range extra {
		c[k] = v
	}
	return c
}

// gesto: un gesto della Sorgente dall'amministratore, con il corpo dal modello.
func (s *bancoSorgente) gesto(thread, allegato uuid.UUID, gesto, motivo string, extra map[string]any) (risposta, map[string]any) {
	s.t.Helper()
	c := s.corpo(thread, allegato, gesto, motivo, extra)
	return s.chiama(s.ad, http.MethodPost, rottaFile(thread, allegato, gesto), c, nil), c
}

func (s *bancoSorgente) statoDB(thread, allegato uuid.UUID) string {
	s.t.Helper()
	var v string
	if err := s.pool.QueryRow(s.ctx, `SELECT coalesce((SELECT stato FROM file_rfq WHERE thread_id = $1 AND allegato_id = $2), '')`,
		thread, allegato).Scan(&v); err != nil {
		s.t.Fatal(err)
	}
	return v
}

// conteggio: un count(*) sul DB di prova.
func (s *bancoSorgente) conteggio(sql string, arg ...any) int {
	s.t.Helper()
	var n int
	if err := s.pool.QueryRow(s.ctx, sql, arg...).Scan(&n); err != nil {
		s.t.Fatalf("%v: %s", err, sql)
	}
	return n
}

// partiDelloStaging: i parziali rimasti nello staging.
func (s *bancoSorgente) partiDelloStaging() []string {
	voci, _ := os.ReadDir(filepath.Join(s.ws.Staging, staging.CartellaParti))
	var out []string
	for _, v := range voci {
		out = append(out, v.Name())
	}
	return out
}

// ---------------------------------------------------------------- PP-01, PP-02: flag e ruoli

// PP-01: a flag spento 404 su tutte le rotte nuove e sul ramo JSON di «carica»; il ramo HTML funziona.
func TestPP01FlagSpento(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, false)
	a, _ := s.file(s.thread, "ACME-P01.pdf", byteCasuali("x.pdf", 1, 2_000), db.OrigineAllegatoOutlook)
	prima := fotoDelDatabase(t, b)
	for _, c := range []struct{ metodo, percorso string }{
		{http.MethodGet, base(s.thread)},
		{http.MethodPost, rottaFile(s.thread, a, "scarta")},
		{http.MethodPost, rottaFile(s.thread, a, "altri-file")},
		{http.MethodPost, rottaFile(s.thread, a, "riattiva")},
		{http.MethodGet, rottaFile(s.thread, a, "contenuto")},
		{http.MethodDelete, rottaFile(s.thread, a, "scarta")},
	} {
		if r := s.chiama(s.ad, c.metodo, c.percorso, `{}`, nil); r.stato != http.StatusNotFound {
			t.Errorf("%s %s a flag spento: %d", c.metodo, c.percorso, r.stato)
		}
	}
	if r := s.carica(s.ad, s.thread, true, "portale", map[string][]byte{"ACME-P01b.pdf": byteCasuali("x.pdf", 2, 900)}, nil); r.stato != http.StatusNotFound {
		t.Errorf("E1′ a flag spento: %d", r.stato)
	}
	if p := s.partiDelloStaging(); len(p) != 0 {
		t.Errorf("parziali nello staging dopo un 404: %v", p)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("le rotte spente hanno scritto: %s", d)
	}
	html := byteCasuali("x.pdf", 3, 900)
	r := s.carica(s.op, s.thread, false, "portale", map[string][]byte{"ACME-P01c.pdf": html}, nil)
	if r.stato != 200 || s.conteggio(`SELECT count(*) FROM allegato WHERE sha256 = $1`, shaDiByte(html)) != 1 {
		t.Errorf("il ramo HTML di «carica» a flag spento: %d", r.stato)
	}
}

// PP-02: operatore e consultazione → 403 sulle rotte nuove e sul ramo JSON; il ramo HTML resta agli operatori.
func TestPP02OperatoreEConsultazione(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	a, _ := s.file(s.thread, "ACME-P02.pdf", byteCasuali("x.pdf", 4, 2_000), db.OrigineAllegatoOutlook)
	prima := fotoDelDatabase(t, b)
	for _, w := range []*browser{s.op, s.co} {
		for _, c := range []struct{ metodo, percorso string }{
			{http.MethodGet, base(s.thread)},
			{http.MethodPost, rottaFile(s.thread, a, "scarta")},
			{http.MethodPost, rottaFile(s.thread, a, "altri-file")},
			{http.MethodPost, rottaFile(s.thread, a, "riattiva")},
			{http.MethodGet, rottaFile(s.thread, a, "contenuto")},
		} {
			if r := s.chiama(w, c.metodo, c.percorso, `{}`, nil); r.stato != http.StatusForbidden {
				t.Errorf("%s %s: %d, atteso 403", c.metodo, c.percorso, r.stato)
			}
		}
		if r := s.carica(w, s.thread, true, "portale", map[string][]byte{"ACME-P02b.pdf": byteCasuali("x.pdf", 5, 900)}, nil); r.stato != http.StatusForbidden {
			t.Errorf("E1′ di un non amministratore: %d", r.stato)
		}
	}
	if p := s.partiDelloStaging(); len(p) != 0 {
		t.Errorf("parziali nello staging dopo un 403: %v", p)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("le rotte negate hanno scritto: %s", d)
	}
	html := byteCasuali("x.pdf", 6, 900)
	r := s.carica(s.op, s.thread, false, "portale", map[string][]byte{"ACME-P02c.pdf": html}, nil)
	if r.stato != 200 || s.conteggio(`SELECT count(*) FROM allegato WHERE sha256 = $1`, shaDiByte(html)) != 1 {
		t.Errorf("il ramo HTML di «carica» per l'operatore: %d", r.stato)
	}
	// la riga gemella a flag acceso: 405 con Allow, senza leggere la sessione
	anonimo := b.browser("10.0.0.7")
	if r := s.chiama(anonimo, http.MethodDelete, rottaFile(s.thread, a, "scarta"), nil, nil); r.stato != http.StatusMethodNotAllowed || r.h.Get("Allow") != "POST" {
		t.Errorf("metodo non ammesso: %d Allow %q", r.stato, r.h.Get("Allow"))
	}
}

// ---------------------------------------------------------------- PP-03, PP-11, R-30: appartenenza

// PP-03 e PP-11: un allegato di un'altra RFQ (anche con lo stesso contenuto) o inesistente → 422 FUORI_RFQ su ogni
// rotta (S2-S5 per il percorso, S1 per ?file=), una RFQ inesistente → 404 (e per «carica» niente nello staging); nessuna
// scrittura. R-30: l'errore di S5 porta X-Motivo.
func TestPP03AppartenenzaAllaRFQ(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	c := byteCasuali("x.pdf", 7, 2_000)
	a, _ := s.file(s.thread, "ACME-P03.pdf", c, db.OrigineAllegatoOutlook)
	altraRFQ := s.rfq("RFQ-B")
	altro, _ := s.file(altraRFQ, "ACME-P03.pdf", c, db.OrigineAllegatoOutlook) // stesso contenuto, altra RFQ
	_ = a
	prima := fotoDelDatabase(t, b)
	for _, id := range []uuid.UUID{altro, uuid.New()} {
		for _, g := range []string{"scarta", "altri-file", "riattiva"} {
			corpo := map[string]any{"versione_attesa": 0, "chiave_idempotenza": uuid.New().String(), "motivo": "prova",
				"impronta_dipendenze": strings.Repeat("0", 64)}
			if g == "riattiva" {
				delete(corpo, "impronta_dipendenze")
			}
			r := s.chiama(s.ad, http.MethodPost, rottaFile(s.thread, id, g), corpo, nil)
			if m := r.mappa(t); r.stato != http.StatusUnprocessableEntity || m["codice"] != "FUORI_RFQ" || m["esito"] != "rifiutato" {
				t.Errorf("%s di un file non della RFQ: %d %v", g, r.stato, m)
			}
		}
		r := s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, id, "contenuto"), nil, nil)
		if r.stato != http.StatusUnprocessableEntity || r.h.Get("X-Motivo") != "FUORI_RFQ" || r.mappa(t)["codice"] != "FUORI_RFQ" {
			t.Errorf("S5 di un file non della RFQ: %d X-Motivo %q", r.stato, r.h.Get("X-Motivo"))
		}
		r = s.chiama(s.ad, http.MethodGet, base(s.thread)+"?file="+id.String(), nil, nil)
		if r.stato != http.StatusUnprocessableEntity || r.mappa(t)["codice"] != "FUORI_RFQ" {
			t.Errorf("S1 con il dettaglio di un file non della RFQ: %d %s", r.stato, r.corpo)
		}
	}
	nessuna := uuid.New()
	for _, c := range []struct{ metodo, percorso string }{
		{http.MethodGet, base(nessuna)},
		{http.MethodPost, rottaFile(nessuna, a, "scarta")},
		{http.MethodGet, rottaFile(nessuna, a, "contenuto")},
	} {
		if r := s.chiama(s.ad, c.metodo, c.percorso, `{}`, nil); r.stato != http.StatusNotFound || r.mappa(t)["codice"] != "RFQ_INESISTENTE" {
			t.Errorf("%s %s di una RFQ inesistente: %d %s", c.metodo, c.percorso, r.stato, r.corpo)
		}
	}
	if r := s.carica(s.ad, nessuna, true, "portale", map[string][]byte{"ACME-P03b.pdf": c}, nil); r.stato != http.StatusNotFound {
		t.Errorf("E1′ per una RFQ inesistente: %d", r.stato)
	}
	if r := s.chiama(s.ad, http.MethodGet, "/thread/non-un-uuid/sorgente", nil, nil); r.stato != http.StatusBadRequest {
		t.Errorf("id della RFQ non valido: %d", r.stato)
	}
	if p := s.partiDelloStaging(); len(p) != 0 {
		t.Errorf("parziali nello staging: %v", p)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("i rifiuti hanno scritto: %s", d)
	}
}

// ---------------------------------------------------------------- PP-04: nessun percorso dal client

// PP-04: un corpo con percorso, path_staging o radice → 422 CORPO_NON_VALIDO con il campo.
func TestPP04CorpoConCampiPercorso(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	a, _ := s.file(s.thread, "ACME-P04.pdf", byteCasuali("x.pdf", 8, 2_000), db.OrigineAllegatoOutlook)
	prima := fotoDelDatabase(t, b)
	for _, campo := range []string{"percorso", "path_staging", "radice"} {
		c := s.corpo(s.thread, a, "scarta", "prova", map[string]any{campo: `C:\Windows\System32`})
		r := s.chiama(s.ad, http.MethodPost, rottaFile(s.thread, a, "scarta"), c, nil)
		var m map[string]any
		_ = json.Unmarshal(r.corpo, &m)
		if r.stato != http.StatusUnprocessableEntity || m["codice"] != "CORPO_NON_VALIDO" || m["campo"] != campo {
			t.Errorf("corpo con %s: %d %v", campo, r.stato, m)
		}
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("i rifiuti hanno scritto: %s", d)
	}
}

// ---------------------------------------------------------------- PP-05, PP-06: percorsi alterati nel DB

// PP-05: path_staging alterato nel DB (fuori dalla radice, con «..», il posto di un altro contenuto): nessuna
// cancellazione (errore non ritentabile) e S5 non serve i byte.
func TestPP05PathStagingAlterato(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	fuori := filepath.Join(t.TempDir(), "fuori.pdf")
	if err := os.WriteFile(fuori, byteCasuali("x.pdf", 9, 1_000), 0o644); err != nil {
		t.Fatal(err)
	}
	altroC := byteCasuali("altro.pdf", 10, 1_000)
	altroP, _ := staging.PercorsoContenuto(s.ws.Staging, shaDiByte(altroC), "altro.pdf")
	_ = os.MkdirAll(filepath.Dir(altroP), 0o755)
	if err := os.WriteFile(altroP, altroC, 0o644); err != nil {
		t.Fatal(err)
	}
	casi := []struct {
		nome, percorso string
		statoS5        int
		motivo         string
	}{
		{"fuori dalla radice", fuori, http.StatusInternalServerError, "percorso_non_valido"},
		{"con ..", s.ws.Staging + `\` + staging.CartellaContenuti + `\..\..\` + filepath.Base(filepath.Dir(fuori)) + `\fuori.pdf`, http.StatusInternalServerError, "percorso_non_valido"},
		{"un altro contenuto", altroP, http.StatusGone, "fuori_cache"},
	}
	for i, c := range casi {
		a, _ := s.file(s.thread, fmt.Sprintf("ACME-P05-%d.pdf", i), byteCasuali("x.pdf", int64(20+i), 1_500), db.OrigineAllegatoOutlook)
		if _, err := s.pool.Exec(s.ctx, `UPDATE allegato SET path_staging = $2 WHERE allegato_id = $1`, a, c.percorso); err != nil {
			t.Fatal(err)
		}
		r := s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, a, "contenuto"), nil, nil)
		if r.stato != c.statoS5 || r.h.Get("X-Motivo") == "" || bytes.Contains(r.corpo, altroC[:32]) {
			t.Errorf("%s: S5 %d X-Motivo %q", c.nome, r.stato, r.h.Get("X-Motivo"))
		}
		r, _ = s.gesto(s.thread, a, "scarta", "percorso alterato", nil)
		if r.stato != 200 {
			t.Fatalf("%s: lo scarto: %d %s", c.nome, r.stato, r.corpo)
		}
		var stato, motivo string
		var ritentabile bool
		if err := s.pool.QueryRow(s.ctx, `SELECT stato, motivo, ritentabile FROM file_rfq_rimozione WHERE allegato_id = $1`, a).Scan(&stato, &motivo, &ritentabile); err != nil {
			t.Fatal(err)
		}
		if stato != "errore" || motivo != c.motivo || ritentabile {
			t.Errorf("%s: rimozione %s/%s ritentabile %v", c.nome, stato, motivo, ritentabile)
		}
	}
	if _, err := os.Stat(fuori); err != nil {
		t.Error("il file fuori dalla radice e' stato toccato")
	}
	if _, err := os.Stat(altroP); err != nil {
		t.Error("il file di un altro contenuto e' stato toccato")
	}
}

// PP-06: percorso_relativo della conservazione alterato: il database lo rifiuta (CHECK e trigger), e in lettura il
// contenimento nel deposito rifiuta un percorso che esce (L1: TestDepositoContenimento).
func TestPP06PercorsoDellaCopiaAlterato(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	a, _ := s.file(s.thread, "ACME-P06.pdf", byteCasuali("x.pdf", 30, 2_000), db.OrigineAllegatoOutlook)
	if r, _ := s.gesto(s.thread, a, "altri-file", "", nil); r.stato != 200 {
		t.Fatalf("Altri file: %d %s", r.stato, r.corpo)
	}
	for _, p := range []string{`..\..\Windows\win.ini`, `C:\Windows\win.ini`, s.thread.String() + `\altro.pdf`} {
		if _, err := s.pool.Exec(s.ctx, `UPDATE file_rfq_conservazione SET percorso_relativo = $2 WHERE allegato_id = $1`, a, p); err == nil {
			t.Errorf("il database accetta il percorso della copia %q", p)
		}
	}
	if _, err := s.svc.Deposito.PercorsoCopia(`..\..\fuori.pdf`); err == nil {
		t.Error("il contenimento del deposito lascia uscire un percorso")
	}
}

// ---------------------------------------------------------------- PP-07, PP-08

// PP-07: una POST da un'altra origine e' rifiutata da CrossOriginProtection.
func TestPP07AltraOrigine(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	a, _ := s.file(s.thread, "ACME-P07.pdf", byteCasuali("x.pdf", 31, 2_000), db.OrigineAllegatoOutlook)
	prima := fotoDelDatabase(t, b)
	c := s.corpo(s.thread, a, "scarta", "da un altro sito", nil)
	r := s.chiama(s.ad, http.MethodPost, rottaFile(s.thread, a, "scarta"), c,
		map[string]string{"Origin": "https://altro.example", "Sec-Fetch-Site": "cross-site"})
	if r.stato != http.StatusForbidden {
		t.Errorf("POST da un'altra origine: %d", r.stato)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("la POST da un'altra origine ha scritto: %s", d)
	}
}

// PP-08 (T-04 per le rotte della corsia file): S1 e S5, in tutte le forme, non scrivono e non accodano.
func TestPP08NessunaGetDellaSorgenteScrive(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	a, _ := s.file(s.thread, "ACME-P08.pdf", byteCasuali("x.pdf", 32, 3_000), db.OrigineAllegatoOutlook)
	g, _ := s.file(s.thread, "ACME-P08b.pdf", nil, db.OrigineAllegatoOutlook) // mai scaricato
	z, _ := s.file(s.thread, "ACME-P08.stp", byteCasuali("x.stp", 33, 3_000), db.OrigineAllegatoOutlook)
	if r, _ := s.gesto(s.thread, z, "altri-file", "", nil); r.stato != 200 {
		t.Fatalf("Altri file: %d %s", r.stato, r.corpo)
	}
	prima := fotoDelDatabase(t, b)
	for _, c := range []struct {
		metodo, percorso string
		intest           map[string]string
	}{
		{http.MethodGet, base(s.thread), nil},
		{http.MethodGet, base(s.thread) + "?scartati=1", nil},
		{http.MethodGet, base(s.thread) + "?file=" + a.String(), nil},
		{http.MethodHead, base(s.thread), nil},
		{http.MethodGet, rottaFile(s.thread, a, "contenuto"), nil},
		{http.MethodGet, rottaFile(s.thread, a, "contenuto") + "?modo=scarica", nil},
		{http.MethodGet, rottaFile(s.thread, a, "contenuto"), map[string]string{"Range": "bytes=0-99"}},
		{http.MethodHead, rottaFile(s.thread, a, "contenuto"), nil},
		{http.MethodGet, rottaFile(s.thread, g, "contenuto"), nil},
		{http.MethodGet, rottaFile(s.thread, z, "contenuto"), nil},
	} {
		if r := s.chiama(s.ad, c.metodo, c.percorso, nil, c.intest); r.stato >= 500 {
			t.Errorf("%s %s: %d", c.metodo, c.percorso, r.stato)
		}
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("una GET della Sorgente ha scritto: %s", d)
	}
}

// ---------------------------------------------------------------- R-20, R-20b: E1′

// R-20: E1′ file per file: caricato, gia_presente (anche in Altri file), rifiutato; parziale; ricaricato_dopo_scarto
// con l'evento del servizio del ciclo dei file (origine ponte; dal ramo HTML origine legacy); 404 e 403 prima del corpo
// e dello staging (TestPP01, TestPP02, TestPP03).
func TestR20CaricamentoJSON(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	pdf := byteCasuali("a.pdf", 40, 2_000)
	r := s.carica(s.ad, s.thread, true, "portale", map[string][]byte{"ACME-R20.pdf": pdf, "VUOTO.pdf": {}}, nil)
	m := r.mappa(t)
	if r.stato != 200 || m["esito"] != "parziale" || m["origine"] != "portale" {
		t.Fatalf("primo invio: %d %v", r.stato, m)
	}
	esiti := map[string]map[string]any{}
	for _, x := range m["file"].([]any) {
		f := x.(map[string]any)
		esiti[f["nome_file"].(string)] = f
	}
	if esiti["ACME-R20.pdf"]["esito"] != "caricato" || esiti["VUOTO.pdf"]["esito"] != "rifiutato" {
		t.Errorf("esiti: %v", esiti)
	}
	if mot, _ := esiti["VUOTO.pdf"]["motivo"].(map[string]any); mot["codice"] != "FILE_VUOTO" {
		t.Errorf("il file vuoto: %v", esiti["VUOTO.pdf"])
	}
	a, _ := uuid.Parse(esiti["ACME-R20.pdf"]["allegato_id"].(string))
	// lo stesso file di nuovo: gia_presente, nessun allegato nuovo (422 NESSUN_FILE_CARICATO no: c'e' un gia_presente)
	r = s.carica(s.ad, s.thread, true, "portale", map[string][]byte{"ACME-R20.pdf": pdf}, nil)
	m = r.mappa(t)
	if r.stato != 200 || m["esito"] != "caricati" || m["file"].([]any)[0].(map[string]any)["esito"] != "gia_presente" {
		t.Errorf("il secondo invio: %d %v", r.stato, m)
	}
	// in Altri file: ancora gia_presente, con lo stato
	if g, _ := s.gesto(s.thread, a, "altri-file", "", nil); g.stato != 200 {
		t.Fatalf("Altri file: %d %s", g.stato, g.corpo)
	}
	r = s.carica(s.ad, s.thread, true, "portale", map[string][]byte{"ACME-R20.pdf": pdf}, nil)
	f := r.mappa(t)["file"].([]any)[0].(map[string]any)
	if f["esito"] != "gia_presente" || f["stato_file"] != "altri_file" {
		t.Errorf("il file in Altri file ricaricato: %v", f)
	}
	// scartato (da Altri file, con la conferma dell'unica copia): il ricaricamento e' un'occorrenza nuova, con l'evento
	if g, _ := s.gesto(s.thread, a, "scarta", "sbagliato", map[string]any{"conferma_unica_copia": true}); g.stato != 200 {
		t.Fatalf("Scarta: %d %s", g.stato, g.corpo)
	}
	r = s.carica(s.ad, s.thread, true, "portale", map[string][]byte{"ACME-R20.pdf": pdf}, nil)
	f = r.mappa(t)["file"].([]any)[0].(map[string]any)
	if f["esito"] != "caricato" || f["ricaricato_dopo_scarto"] != true || f["allegato_id"] == a.String() {
		t.Errorf("il ricaricamento dopo lo scarto: %v", f)
	}
	var n int
	if err := s.pool.QueryRow(s.ctx, `SELECT count(*) FROM file_rfq_evento WHERE allegato_id = $1 AND azione = 'ricaricato_dopo_scarto' AND origine = 'ponte'`, a).Scan(&n); err != nil || n != 1 {
		t.Errorf("evento ricaricato_dopo_scarto (ponte): %d %v", n, err)
	}
	// dal ramo HTML: lo scarto dell'occorrenza nuova e il ricaricamento HTML → evento con origine legacy
	nuovo, _ := uuid.Parse(f["allegato_id"].(string))
	if g, _ := s.gesto(s.thread, nuovo, "scarta", "di nuovo", map[string]any{"conferma_unica_copia": true}); g.stato != 200 {
		t.Fatalf("Scarta: %d %s", g.stato, g.corpo)
	}
	h := s.carica(s.op, s.thread, false, "portale", map[string][]byte{"ACME-R20.pdf": pdf}, nil)
	if h.stato != 200 {
		t.Fatalf("ramo HTML: %d", h.stato)
	}
	if err := s.pool.QueryRow(s.ctx, `SELECT count(*) FROM file_rfq_evento WHERE allegato_id = $1 AND azione = 'ricaricato_dopo_scarto' AND origine = 'legacy'`, nuovo).Scan(&n); err != nil || n != 1 {
		t.Errorf("evento ricaricato_dopo_scarto (legacy): %d %v", n, err)
	}
	// nessun file entrato: 422 NESSUN_FILE_CARICATO; origine sconosciuta: 422 CORPO_NON_VALIDO prima dello staging
	r = s.carica(s.ad, s.thread, true, "portale", map[string][]byte{"VUOTO.pdf": {}}, nil)
	if r.stato != http.StatusUnprocessableEntity || r.mappa(t)["codice"] != "NESSUN_FILE_CARICATO" {
		t.Errorf("nessun file: %d %s", r.stato, r.corpo)
	}
	r = s.carica(s.ad, s.thread, true, "fornitore", map[string][]byte{"X.pdf": pdf}, nil)
	if r.stato != http.StatusUnprocessableEntity || r.mappa(t)["campo"] != "origine" {
		t.Errorf("origine sconosciuta: %d %s", r.stato, r.corpo)
	}
	r = s.carica(s.ad, s.thread, true, "", map[string][]byte{"X.pdf": pdf}, map[string]string{"percorso": `C:\x`})
	if r.stato != http.StatusUnprocessableEntity || r.mappa(t)["campo"] != "percorso" {
		t.Errorf("un campo percorso nel caricamento: %d %s", r.stato, r.corpo)
	}
	if p := s.partiDelloStaging(); len(p) != 0 {
		t.Errorf("parziali rimasti: %v", p)
	}
}

// R-20b: due invii concorrenti dello stesso file nella stessa nota: un caricato e un gia_presente, una sola riga.
func TestR20bDueInviiConcorrenti(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	pdf := byteCasuali("b.pdf", 41, 4_000)
	var wg sync.WaitGroup
	risposte := make([]risposta, 2)
	partenza := make(chan struct{})
	for i := range risposte {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-partenza
			risposte[i] = s.carica(s.ad, s.thread, true, "portale", map[string][]byte{"ACME-R20b.pdf": pdf}, nil)
		}(i)
	}
	close(partenza)
	wg.Wait()
	esiti := map[string]int{}
	for _, r := range risposte {
		if r.stato != 200 {
			t.Fatalf("invio: %d %s", r.stato, r.corpo)
		}
		esiti[r.mappa(t)["file"].([]any)[0].(map[string]any)["esito"].(string)]++
	}
	if esiti["caricato"] != 1 || esiti["gia_presente"] != 1 {
		t.Errorf("esiti: %v", esiti)
	}
	var n int
	if err := s.pool.QueryRow(s.ctx, `SELECT count(*) FROM allegato WHERE sha256 = $1`, shaDiByte(pdf)).Scan(&n); err != nil || n != 1 {
		t.Errorf("righe allegato: %d %v", n, err)
	}
}

// ---------------------------------------------------------------- R-21: il modello della Sorgente (PE-06)

// R-21 (la parte di S1) e DECA PE-06: contatori, dettaglio con le dipendenze e l'impronta, byte come stati, azioni come
// {disponibile, motivo}, scarto{autore, quando, motivo}, altri_file{autore, quando, motivo}, documento solo per un
// documento confermato, conservazione in errore come {stato: "errore", causa}.
func TestR21IlModelloDellaSorgente(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	a, _ := s.file(s.thread, "ACME-R21a.pdf", byteCasuali("x.pdf", 50, 2_000), db.OrigineAllegatoOutlook)
	x, _ := s.file(s.thread, "ACME-R21x.pdf", byteCasuali("x.pdf", 51, 2_000), db.OrigineAllegatoOutlook)
	k, _ := s.file(s.thread, "ACME-R21k.pdf", byteCasuali("x.pdf", 52, 2_000), db.OrigineAllegatoOutlook)
	if r, _ := s.gesto(s.thread, a, "altri-file", "da tenere", nil); r.stato != 200 {
		t.Fatalf("Altri file: %d %s", r.stato, r.corpo)
	}
	if r, _ := s.gesto(s.thread, x, "scarta", "non serve", nil); r.stato != 200 {
		t.Fatalf("Scarta: %d %s", r.stato, r.corpo)
	}
	// una copia di conservazione in errore: il deposito non accetta i file
	parti := filepath.Join(s.svc.Deposito.Radice, filerfq.CartellaPartiDeposito)
	_ = os.RemoveAll(parti)
	if err := os.WriteFile(parti, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.gesto(s.thread, k, "altri-file", "", nil); r.stato != http.StatusUnprocessableEntity {
		t.Fatalf("la copia doveva fallire: %d %s", r.stato, r.corpo)
	}
	m := s.modello(s.thread, a)
	cont, _ := m["contatori"].(map[string]any)
	if cont["attivi"] != float64(1) || cont["altri_file"] != float64(1) || cont["scartati"] != float64(1) {
		t.Errorf("contatori %v", cont)
	}
	fa := fileNelModello(m, a)
	if tr, _ := fa["altri_file"].(map[string]any); tr == nil || tr["autore"] == "" || tr["quando"] == nil || tr["motivo"] != "da tenere" {
		t.Errorf("altri_file %v", fa["altri_file"])
	}
	if fa["documento"] != nil {
		t.Errorf("documento non nullo per un file non confermato: %v", fa["documento"])
	}
	if by, _ := fa["byte"].(map[string]any); by["conservazione"] != "presente" || by["staging"] == nil {
		t.Errorf("byte %v", fa["byte"])
	}
	fx := fileNelModello(m, x)
	if tr, _ := fx["scarto"].(map[string]any); tr == nil || tr["motivo"] != "non serve" || tr["autore"] == "" {
		t.Errorf("scarto %v", fx["scarto"])
	}
	fk := fileNelModello(m, k)
	if c, _ := fk["conservazione"].(map[string]any); c["stato"] != "errore" || c["causa"] != "permessi" {
		t.Errorf("conservazione in errore %v", fk["conservazione"])
	}
	for _, f := range []map[string]any{fa, fx, fk} {
		az, _ := f["azioni"].(map[string]any)
		for _, nome := range []string{"scarta", "altri_file", "riattiva", "contenuto"} {
			v, _ := az[nome].(map[string]any)
			if _, ok := v["disponibile"].(bool); !ok {
				t.Errorf("azione %s senza disponibile: %v", nome, az[nome])
			}
			if _, ok := v["motivo"].(string); !ok {
				t.Errorf("azione %s senza motivo: %v", nome, az[nome])
			}
		}
	}
	d, _ := m["dettaglio"].(map[string]any)
	if imp, _ := d["impronta_dipendenze"].(string); len(imp) != 64 {
		t.Errorf("impronta_dipendenze %v", d["impronta_dipendenze"])
	}
	deps, _ := d["dipendenze"].([]any)
	for _, x := range deps {
		dep := x.(map[string]any)
		for _, campo := range []string{"allegato_id", "codice", "esito", "testo"} {
			if dep[campo] == nil {
				t.Errorf("dipendenza senza %s: %v", campo, dep)
			}
		}
	}
}

// ---------------------------------------------------------------- R-30, AF-08, AF-02: S5

// R-30 (DECA PE-29): gli errori di S5 portano X-Motivo con il codice della busta (410 FILE_SCARTATO, 410
// BYTE_NON_DISPONIBILI con l'indicazione); una risposta riuscita no. AF-02 (la rotta): un file in Altri file con lo
// staging tolto dalla cache si legge dal deposito con lo sha giusto.
func TestR30XMotivoEContenutoDalDeposito(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	pdf := byteCasuali("x.pdf", 60, 3_000)
	a, sha := s.file(s.thread, "ACME-R30.pdf", pdf, db.OrigineAllegatoOutlook)
	r := s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, a, "contenuto"), nil, nil)
	if r.stato != 200 || r.h.Get("X-Motivo") != "" || !bytes.Equal(r.corpo, pdf) {
		t.Fatalf("S5 riuscita: %d X-Motivo %q", r.stato, r.h.Get("X-Motivo"))
	}
	if g, _ := s.gesto(s.thread, a, "altri-file", "", nil); g.stato != 200 {
		t.Fatalf("Altri file: %d %s", g.stato, g.corpo)
	}
	cache := &staging.Cache{Pool: s.pool, Staging: s.ws.Staging, Log: testutil.LogSilenzioso(), Retention: 30 * 24 * time.Hour,
		Adesso: func() time.Time { return time.Now().Add(40 * 24 * time.Hour) }, CopiaNelDeposito: s.svc.CopiaNelDeposito()}
	if e, err := cache.Giro(s.ctx); err != nil || e.Rimossi != 1 {
		t.Fatalf("la cache: %+v %v", e, err)
	}
	r = s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, a, "contenuto"), nil, nil)
	if r.stato != 200 || shaDiByte(r.corpo) != sha || r.h.Get("X-Motivo") != "" {
		t.Errorf("S5 dal deposito: %d", r.stato)
	}
	// mai scaricato: 410 BYTE_NON_DISPONIBILI con l'indicazione «scarica»
	g, _ := s.file(s.thread, "ACME-R30g.pdf", nil, db.OrigineAllegatoOutlook)
	r = s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, g, "contenuto"), nil, nil)
	if m := r.mappa(t); r.stato != http.StatusGone || r.h.Get("X-Motivo") != "BYTE_NON_DISPONIBILI" || m["codice"] != "BYTE_NON_DISPONIBILI" || m["indicazione"] != "scarica" {
		t.Errorf("byte non disponibili: %d %q %v", r.stato, r.h.Get("X-Motivo"), m)
	}
	// scartato: 410 FILE_SCARTATO
	x, _ := s.file(s.thread, "ACME-R30x.pdf", byteCasuali("x.pdf", 61, 1_000), db.OrigineAllegatoOutlook)
	if g, _ := s.gesto(s.thread, x, "scarta", "via", nil); g.stato != 200 {
		t.Fatalf("Scarta: %d %s", g.stato, g.corpo)
	}
	r = s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, x, "contenuto"), nil, nil)
	if r.stato != http.StatusGone || r.h.Get("X-Motivo") != "FILE_SCARTATO" || r.mappa(t)["codice"] != "FILE_SCARTATO" {
		t.Errorf("scartato: %d %q", r.stato, r.h.Get("X-Motivo"))
	}
}

// AF-08: la lettura per tutti i formati: i non PDF octet-stream, attachment, nosniff, CSP sandbox; un PDF inline
// (anteprima) dopo il controllo dei byte; con HX-Request 400; Range sul PDF.
func TestAF08LetturaPerTuttiIFormati(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	for _, nome := range []string{"ACME.stp", "ACME.dwg", "ACME.xlsx", "ACME.zip", "FINTO.pdf"} {
		c := byteCasuali("x.bin", int64(len(nome)), 1_500) // senza l'intestazione %PDF-: anche FINTO.pdf non e' un PDF
		a, _ := s.file(s.thread, nome, c, db.OrigineAllegatoOutlook)
		r := s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, a, "contenuto"), nil, nil)
		if r.stato != 200 || r.h.Get("Content-Type") != "application/octet-stream" || !strings.HasPrefix(r.h.Get("Content-Disposition"), "attachment") ||
			r.h.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(r.h.Get("Content-Security-Policy"), "sandbox") {
			t.Errorf("%s: %d %q %q %q %q", nome, r.stato, r.h.Get("Content-Type"), r.h.Get("Content-Disposition"),
				r.h.Get("X-Content-Type-Options"), r.h.Get("Content-Security-Policy"))
		}
	}
	pdf := byteCasuali("x.pdf", 70, 5_000)
	a, _ := s.file(s.thread, "ACME-AF08.pdf", pdf, db.OrigineAllegatoOutlook)
	r := s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, a, "contenuto"), nil, nil)
	if r.stato != 200 || r.h.Get("Content-Type") != "application/pdf" || !strings.HasPrefix(r.h.Get("Content-Disposition"), "inline") ||
		r.h.Get("Cache-Control") != "no-store" {
		t.Errorf("PDF in anteprima: %d %q %q %q", r.stato, r.h.Get("Content-Type"), r.h.Get("Content-Disposition"), r.h.Get("Cache-Control"))
	}
	r = s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, a, "contenuto")+"?modo=scarica", nil, nil)
	if !strings.HasPrefix(r.h.Get("Content-Disposition"), "attachment") {
		t.Errorf("PDF da scaricare: %q", r.h.Get("Content-Disposition"))
	}
	r = s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, a, "contenuto"), nil, map[string]string{"Range": "bytes=0-9"})
	if r.stato != http.StatusPartialContent || len(r.corpo) != 10 {
		t.Errorf("Range: %d, %d byte", r.stato, len(r.corpo))
	}
	r = s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, a, "contenuto"), nil, map[string]string{"HX-Request": "true"})
	if r.stato != http.StatusBadRequest || r.h.Get("X-Motivo") != "RICHIESTA_NON_VALIDA" {
		t.Errorf("con HX-Request: %d %q", r.stato, r.h.Get("X-Motivo"))
	}
	r = s.chiama(s.ad, http.MethodGet, rottaFile(s.thread, a, "contenuto")+"?modo=altro", nil, nil)
	if r.stato != http.StatusBadRequest {
		t.Errorf("modo sconosciuto: %d", r.stato)
	}
}

// ---------------------------------------------------------------- AF-10, CO-04: le pagine legacy

// AF-10: i gesti legacy su un file in Altri file (assegna, conferma della voce, «Documentazione generale», il vecchio
// «Scarta il file») rifiutano con la frase, senza scrivere.
func TestAF10GestiLegacySuUnFileInAltriFile(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	a, _ := s.file(s.thread, "ACME-AF10.pdf", byteCasuali("x.pdf", 80, 2_000), db.OrigineAllegatoOutlook)
	if r, _ := s.gesto(s.thread, a, "altri-file", "", nil); r.stato != 200 {
		t.Fatalf("Altri file: %d %s", r.stato, r.corpo)
	}
	var pid, utente uuid.UUID
	if err := s.pool.QueryRow(s.ctx, `SELECT proposta_id FROM documento_proposta WHERE allegato_id = $1`, a).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(s.ctx, `SELECT utente_id FROM utente WHERE sigla = 'FP'`).Scan(&utente); err != nil {
		t.Fatal(err)
	}
	var comp uuid.UUID
	if err := s.pool.QueryRow(s.ctx, `INSERT INTO componente (thread_id, codice, tipo, confermato_da) VALUES ($1, 'ACME-AF10', 'sciolto', $2)
		RETURNING componente_id`, s.thread, utente).Scan(&comp); err != nil {
		t.Fatal(err)
	}
	casi := []struct {
		nome, percorso string
		form           url.Values
	}{
		{"assegna", "/thread/" + s.thread.String() + "/fascicolo/assegna", url.Values{"proposta": {pid.String()}, "componente": {comp.String()}}},
		{"conferma della voce", "/proposta/" + pid.String() + "/conferma", url.Values{"tipo": {"disegno_2d"}, "codice": {"ACME-AF10"}, "componente_id": {comp.String()}}},
		{"documentazione generale", "/thread/" + s.thread.String() + "/fascicolo/file/" + pid.String() + "/generale", url.Values{"tipo": {"altro"}}},
		{"vecchio Scarta il file", "/proposta/" + pid.String() + "/scarta", url.Values{}},
	}
	for _, c := range casi {
		prima := fotoDominio(t, b)
		resp, testo := s.op.fai(http.MethodPost, c.percorso, c.form, true)
		if !strings.Contains(testo, "Altri file") {
			t.Errorf("%s: %d, la risposta non dice del file in Altri file: %q", c.nome, resp.StatusCode, avvisoF(testo))
		}
		if d := differenze(prima, fotoDominio(t, b)); d != "" {
			t.Errorf("%s: tabelle cambiate %s", c.nome, d)
		}
	}
}

// CO-04: Scarica salta le occorrenze scartate o in Altri file e le conta; Riscarica le rifiuta con la frase; nessun job.
func TestCO04ScaricaERiscarica(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	x, _ := s.file(s.thread, "ACME-C04x.pdf", byteCasuali("x.pdf", 90, 1_000), db.OrigineAllegatoOutlook)
	k, _ := s.file(s.thread, "ACME-C04k.pdf", byteCasuali("x.pdf", 91, 1_000), db.OrigineAllegatoOutlook)
	if r, _ := s.gesto(s.thread, x, "scarta", "via", nil); r.stato != 200 {
		t.Fatalf("Scarta: %d %s", r.stato, r.corpo)
	}
	if r, _ := s.gesto(s.thread, k, "altri-file", "", nil); r.stato != 200 {
		t.Fatalf("Altri file: %d %s", r.stato, r.corpo)
	}
	jobPrima := len(b.jobInterattivi())
	for _, c := range []struct {
		id    uuid.UUID
		frase string
	}{{x, "scartato"}, {k, "Altri file"}} {
		_, testo := s.op.fai(http.MethodPost, "/allegato/"+c.id.String()+"/riscarica", url.Values{}, true)
		if !strings.Contains(testo, c.frase) {
			t.Errorf("Riscarica di un file non attivo: %q", avvisoF(testo))
		}
	}
	var msgX uuid.UUID
	if err := s.pool.QueryRow(s.ctx, `SELECT messaggio_id FROM allegato WHERE allegato_id = $1`, x).Scan(&msgX); err != nil {
		t.Fatal(err)
	}
	_, testo := s.op.fai(http.MethodPost, "/messaggio/"+msgX.String()+"/scarica", url.Values{"allegato_id": {x.String()}}, true)
	if !strings.Contains(testo, "1 saltati perché in Altri file o scartati nella Distinta nuova") {
		t.Errorf("Scarica di un file scartato: %q", avvisoF(testo))
	}
	if n := len(b.jobInterattivi()); n != jobPrima {
		t.Errorf("i gesti legacy hanno accodato %d job per file non attivi", n-jobPrima)
	}
}

// ---------------------------------------------------------------- S2-S4 dalla rotta: la busta e l'intento

// I gesti della Sorgente dalla rotta (EM §10.9, §10.10): l'esito 200 con gli stati, il reinvio con la stessa chiave e
// lo stesso corpo (riproposto), la stessa chiave con un corpo diverso (409, busta «conflitto»), il motivo che manca
// (422 MOTIVO_OBBLIGATORIO), il corpo oltre 64 KiB (413), la conservazione non configurata (503 NON_CONFIGURATO, AF-06
// dalla rotta), la versione vecchia (409 FILE_VERSIONE); «Riattiva» e «Metti in Altri file» con i loro esiti.
func TestGestiDellaSorgenteDallaRotta(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.sorgente(t, true)
	a, _ := s.file(s.thread, "ACME-G1.pdf", byteCasuali("x.pdf", 100, 2_000), db.OrigineAllegatoOutlook)
	// motivo obbligatorio per «Scarta»
	c := s.corpo(s.thread, a, "scarta", "", nil)
	if r := s.chiama(s.ad, http.MethodPost, rottaFile(s.thread, a, "scarta"), c, nil); r.stato != http.StatusUnprocessableEntity || r.mappa(t)["codice"] != "MOTIVO_OBBLIGATORIO" {
		t.Errorf("Scarta senza motivo: %d %s", r.stato, r.corpo)
	}
	// corpo oltre il limite
	grande := `{"versione_attesa": 0, "motivo": "` + strings.Repeat("x", 70<<10) + `"}`
	if r := s.chiama(s.ad, http.MethodPost, rottaFile(s.thread, a, "scarta"), grande, nil); r.stato != http.StatusRequestEntityTooLarge || r.h.Get("X-Motivo") != "TROPPO_GRANDE" {
		t.Errorf("corpo oltre 64 KiB: %d %s", r.stato, r.corpo)
	}
	// «Metti in Altri file» senza il deposito: 503 NON_CONFIGURATO
	dep := s.svc.Deposito
	s.svc.Deposito = nil
	r, _ := s.gesto(s.thread, a, "altri-file", "", nil)
	if m := r.mappa(t); r.stato != http.StatusServiceUnavailable || m["codice"] != "NON_CONFIGURATO" || m["motivo"] != "conservazione" {
		t.Errorf("Altri file senza deposito: %d %v", r.stato, m)
	}
	s.svc.Deposito = dep
	// «Metti in Altri file», poi «Riattiva»
	r, _ = s.gesto(s.thread, a, "altri-file", "da tenere", nil)
	if m := r.mappa(t); r.stato != 200 || m["esito"] != "in_altri_file" || m["stato"] != "altri_file" || m["versione"] != float64(1) {
		t.Fatalf("Altri file: %d %v", r.stato, m)
	}
	r, _ = s.gesto(s.thread, a, "riattiva", "", nil)
	if m := r.mappa(t); r.stato != 200 || m["esito"] != "riattivato" || m["stato"] != "attivo" || m["versione"] != float64(2) {
		t.Fatalf("Riattiva: %d %v", r.stato, m)
	}
	// «Scarta» con la versione vecchia: 409 FILE_VERSIONE, busta «conflitto»
	vecchio := s.corpo(s.thread, a, "scarta", "via", map[string]any{"versione_attesa": 1})
	if r := s.chiama(s.ad, http.MethodPost, rottaFile(s.thread, a, "scarta"), vecchio, nil); r.stato != http.StatusConflict || r.mappa(t)["esito"] != "conflitto" || r.mappa(t)["codice"] != "FILE_VERSIONE" {
		t.Errorf("versione vecchia: %d %s", r.stato, r.corpo)
	}
	// «Scarta», il reinvio, la chiave riusata con un altro corpo
	r, corpo := s.gesto(s.thread, a, "scarta", "via", nil)
	m := r.mappa(t)
	if r.stato != 200 || m["esito"] != "scartato" || m["riproposto"] != false || m["versione"] != float64(3) {
		t.Fatalf("Scarta: %d %v", r.stato, m)
	}
	if rim, _ := m["rimozioni"].([]any); len(rim) != 1 || rim[0].(map[string]any)["testo"] == "" {
		t.Errorf("rimozioni nella risposta: %v", m["rimozioni"])
	}
	r = s.chiama(s.ad, http.MethodPost, rottaFile(s.thread, a, "scarta"), corpo, nil)
	if m := r.mappa(t); r.stato != 200 || m["riproposto"] != true || m["esito"] != "scartato" {
		t.Errorf("reinvio: %d %v", r.stato, m)
	}
	diverso := map[string]any{}
	for k, v := range corpo {
		diverso[k] = v
	}
	diverso["motivo"] = "un altro motivo"
	r = s.chiama(s.ad, http.MethodPost, rottaFile(s.thread, a, "scarta"), diverso, nil)
	if m := r.mappa(t); r.stato != http.StatusConflict || m["esito"] != "conflitto" || m["codice"] != "CHIAVE_RIUSATA" {
		t.Errorf("chiave riusata: %d %v", r.stato, m)
	}
	if got := s.statoDB(s.thread, a); got != "scartato" {
		t.Errorf("stato %q", got)
	}
	var eventi int
	if err := s.pool.QueryRow(s.ctx, `SELECT count(*) FROM file_rfq_evento WHERE allegato_id = $1 AND azione = 'scarta'`, a).Scan(&eventi); err != nil || eventi != 1 {
		t.Errorf("eventi di scarto: %d %v", eventi, err)
	}
}
