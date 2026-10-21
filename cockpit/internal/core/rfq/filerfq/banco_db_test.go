//go:build integrazione

package filerfq

// L4 — V1, corsia file (PM23 §10 righe 7, 8, 9 e 13; A2 §8): il banco delle prove del ciclo dei file. Un DB di prova
// (COCKPIT_TEST_DSN, schema pulito a ogni prova), lo staging, il posto degli Altri file (conservatoreprova) e il NAS in
// cartelle temporanee, contenuti
// sintetici (D23:79): mai lo staging di una postazione o del server. Codici e nomi sintetici (ACME).

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/filerfq/conservatoreprova"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
	"promatec/cockpit/internal/platform/testutil"
)

// pipelineFinta registra le occorrenze rimandate alla strada d'ingresso (la «Riattiva», E1′).
type pipelineFinta struct {
	mu       sync.Mutex
	chiamate []uuid.UUID
}

func (p *pipelineFinta) DopoCaricamento(_ context.Context, _ *db.Queries, a uuid.UUID) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.chiamate = append(p.chiamate, a)
	return nil
}

func (p *pipelineFinta) quante(a uuid.UUID) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, x := range p.chiamate {
		if x == a {
			n++
		}
	}
	return n
}

type bancoFile struct {
	t        *testing.T
	ctx      context.Context
	p        *pgxpool.Pool
	q        *db.Queries
	u        db.Utente
	cliente  uuid.UUID
	thread   uuid.UUID
	staging  string
	deposito string // la radice del conservatore delle prove
	cons     *conservatoreprova.Locale
	nas      string
	s        *Servizio
	pipe     *pipelineFinta
	n        int
}

// nuovoBancoFile: schema pulito, un utente, un cliente ACME, una RFQ, il conservatore delle prove e il
// servizio con l'eta' minima a un nanosecondo (le prove che vogliono la regola dei 10 minuti la rimettono).
func nuovoBancoFile(t *testing.T) *bancoFile {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	b := &bancoFile{t: t, ctx: context.Background(), p: p, q: db.New(p), staging: t.TempDir(), deposito: t.TempDir(),
		nas: t.TempDir(), pipe: &pipelineFinta{}}
	var uid uuid.UUID
	b.riga(`INSERT INTO utente (sigla, nome, ufficio) VALUES ('FX', 'Prova File', 'Tecnico') RETURNING utente_id`, &uid)
	u, err := b.q.GetUtente(b.ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	b.u = u
	b.riga(`INSERT INTO cliente (cartella_nas, ragione_sociale) VALUES ('ACME', 'Acme Prova') RETURNING cliente_id`, &b.cliente)
	b.thread = b.rfq("RFQ-A")
	b.cons = conservatoreprova.Nuovo(b.deposito)
	b.s = &Servizio{Pool: p, Staging: b.staging, Conservatore: b.cons, NASRadice: b.nas, Pipeline: b.pipe,
		Log: testutil.LogSilenzioso(), EtaMinima: time.Nanosecond}
	return b
}

func (b *bancoFile) esegui(sql string, arg ...any) {
	b.t.Helper()
	if _, err := b.p.Exec(b.ctx, sql, arg...); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
}

func (b *bancoFile) riga(sql string, dst ...any) {
	b.t.Helper()
	b.rigaCon(sql, nil, dst...)
}

func (b *bancoFile) rigaCon(sql string, arg []any, dst ...any) {
	b.t.Helper()
	if err := b.p.QueryRow(b.ctx, sql, arg...).Scan(dst...); err != nil {
		b.t.Fatalf("%v\n%s", err, sql)
	}
}

func valore[T any](b *bancoFile, sql string, arg ...any) T {
	b.t.Helper()
	var v T
	b.rigaCon(sql, arg, &v)
	return v
}

// rfq: una RFQ nuova del cliente ACME.
func (b *bancoFile) rfq(chiave string) uuid.UUID {
	b.t.Helper()
	return valore[uuid.UUID](b, `INSERT INTO thread_offerta (cliente_id, canale, data_inizio, oggetto, cartella_relativa)
		VALUES ($1, 'outlook', now(), $2, $3) RETURNING thread_id`, b.cliente, "RFQ "+chiave, `ACME\WIP\2026 10 18 `+chiave)
}

// cartellaRFQ: la cartella della RFQ sul NAS (cartella_relativa), relativa alla radice del conservatore delle prove.
func (b *bancoFile) cartellaRFQ(thread uuid.UUID) string {
	b.t.Helper()
	return valore[string](b, `SELECT cartella_relativa FROM thread_offerta WHERE thread_id = $1`, thread)
}

// mail: un messaggio di posta della RFQ.
func (b *bancoFile) mail(thread uuid.UUID) uuid.UUID {
	b.t.Helper()
	b.n++
	conv := valore[uuid.UUID](b, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', $1, now())
		RETURNING conversazione_id`, fmt.Sprintf("CONV-FILE-%d", b.n))
	return valore[uuid.UUID](b, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, oggetto, thread_id)
		VALUES ('outlook', $1, $2, 'entrata', now(), 'RFQ ACME', $3) RETURNING messaggio_id`, fmt.Sprintf("<file-%d@prova>", b.n), conv, thread)
}

// contenuto: byte sintetici; un PDF vero per l'intestazione (%PDF-) se il nome finisce in .pdf, uno STEP per .stp/.step.
func contenuto(nome string, seme int64, n int) []byte {
	c := make([]byte, n)
	rand.New(rand.NewSource(seme)).Read(c)
	switch strings.ToLower(filepath.Ext(nome)) {
	case ".pdf":
		return append([]byte("%PDF-1.4\n"), c...)
	case ".stp", ".step":
		return append([]byte("ISO-10303-21;\n"), c...)
	}
	return c
}

func bytesReader(c []byte) *bytes.Reader { return bytes.NewReader(c) }

func shaDi(c []byte) string {
	h := sha256.Sum256(c)
	return hex.EncodeToString(h[:])
}

// opzFile: come nasce un allegato del banco.
type opzFile struct {
	origine      db.OrigineAllegato // vuota = outlook
	contenitore  uuid.UUID          // la voce di un archivio
	pathInterno  string
	senzaByte    bool   // un allegato di posta mai scaricato: niente sha, niente byte, niente path
	fuoriStaging string // path_staging dato (un percorso fuori dalla cache)
	tipo         string // tipo_proposto; vuoto = dal formato; "-" = nessuna proposta
	componente   uuid.UUID
	codice       string
	natura       db.NaturaAllegato
}

// file mette i byte nel posto del contenuto dello staging (se non ci sono gia') e ne fa un allegato del messaggio, con
// la proposta aperta come la lascerebbe la pipeline. Restituisce l'allegato e lo sha.
func (b *bancoFile) file(msg uuid.UUID, nome string, c []byte, o opzFile) (uuid.UUID, string) {
	b.t.Helper()
	b.n++
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(nome), "."))
	origine := o.origine
	if origine == "" {
		origine = db.OrigineAllegatoOutlook
	}
	natura := o.natura
	if natura == "" {
		natura = db.NaturaAllegatoFile
	}
	var sha, percorso *string
	var bytes *int64
	stato := "analizzato"
	if !o.senzaByte {
		h := shaDi(c)
		sha = &h
		n := int64(len(c))
		bytes = &n
		p := o.fuoriStaging
		if p == "" {
			var err error
			p, err = staging.PercorsoContenuto(b.staging, h, nome)
			if err != nil {
				b.t.Fatal(err)
			}
		}
		if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				b.t.Fatal(err)
			}
			if err := os.WriteFile(p, c, 0o644); err != nil {
				b.t.Fatal(err)
			}
		}
		percorso = &p
	} else {
		stato = "grezzo"
	}
	var cont *uuid.UUID
	if o.contenitore != uuid.Nil {
		cont = &o.contenitore
	}
	var pi *string
	if o.pathInterno != "" {
		pi = &o.pathInterno
	}
	var caricato *uuid.UUID
	if origine != db.OrigineAllegatoOutlook {
		caricato = &b.u.UtenteID
	}
	id := valore[uuid.UUID](b, `INSERT INTO allegato (messaggio_id, contenitore_id, indice, nome_file, path_interno, estensione, natura,
		origine, bytes, sha256, path_staging, stato, ricevuto_il, caricato_da)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::stato_allegato, now(), $13) RETURNING allegato_id`,
		msg, cont, b.n, nome, pi, ext, natura, origine, bytes, sha, percorso, stato, caricato)
	tipo := o.tipo
	if tipo == "" {
		switch ext {
		case "pdf":
			tipo = "disegno_2d"
		case "stp", "step":
			tipo = "cad_3d"
		default:
			tipo = "da_determinare"
		}
	}
	if tipo != "-" {
		var comp *uuid.UUID
		if o.componente != uuid.Nil {
			comp = &o.componente
		}
		var codice *string
		if o.codice != "" {
			codice = &o.codice
		}
		b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte)
			VALUES ($1, (SELECT thread_id FROM messaggio WHERE messaggio_id = $2), $3::tipo_documento, $4, $5, 60, 'estensione')`,
			id, msg, tipo, codice, comp)
	}
	sh := ""
	if sha != nil {
		sh = *sha
	}
	return id, sh
}

// pdf: un PDF di posta della RFQ del banco, in un messaggio suo.
func (b *bancoFile) pdf(nome string) (uuid.UUID, string) {
	b.t.Helper()
	b.n++
	return b.file(b.mail(b.thread), nome, contenuto(nome, int64(1000+b.n), 4_000), opzFile{})
}

// componente: un componente della RFQ.
func (b *bancoFile) componente(thread uuid.UUID, codice, tipo string) uuid.UUID {
	b.t.Helper()
	return valore[uuid.UUID](b, `INSERT INTO componente (thread_id, codice, tipo, confermato_da) VALUES ($1, $2, $3::tipo_componente, $4)
		RETURNING componente_id`, thread, codice, tipo, b.u.UtenteID)
}

// archivio: uno zip della RFQ (con la sua proposta, come ogni allegato) e le sue voci (gli allegati estratti,
// contenitore_id = lo zip).
func (b *bancoFile) archivio(thread uuid.UUID, nome string, voci []string, o opzFile) (zip uuid.UUID, ids []uuid.UUID) {
	b.t.Helper()
	msg := b.mail(thread)
	b.n++
	zip, _ = b.file(msg, nome, contenuto(nome, int64(5000+b.n), 2_000), opzFile{origine: o.origine})
	for i, v := range voci {
		vo := o
		vo.contenitore, vo.pathInterno = zip, "cartella/"+v
		b.n++
		id, _ := b.file(msg, v, contenuto(v, int64(7000+b.n+i), 3_000), vo)
		ids = append(ids, id)
	}
	return zip, ids
}

// ---------------------------------------------------------------- i corpi dei gesti, come li manderebbe la pagina

// modello: S1 della RFQ, con il dettaglio del file.
func (b *bancoFile) modello(thread, allegato uuid.UUID) *ModelloSorgente {
	b.t.Helper()
	m, err := b.s.Modello(b.ctx, thread, OpzioniModello{File: &allegato, Scartati: true})
	if err != nil {
		b.t.Fatalf("S1: %v", err)
	}
	return m
}

func fileDi(m *ModelloSorgente, allegato uuid.UUID) *FileSorgente {
	for i := range m.File {
		if m.File[i].AllegatoID == allegato {
			return &m.File[i]
		}
	}
	return nil
}

func chiave() *uuid.UUID {
	k := uuid.New()
	return &k
}

func vero() *bool {
	v := true
	return &v
}

// corpo: il corpo di un gesto dal modello letto adesso (versione, impronta delle dipendenze, perimetro dell'archivio).
func (b *bancoFile) corpo(gesto string, thread, allegato uuid.UUID, motivo string, mod ...func(*CorpoGesto)) CorpoGesto {
	b.t.Helper()
	m := b.modello(thread, allegato)
	f := fileDi(m, allegato)
	var ver int32
	if f != nil {
		ver = f.Versione
	}
	c := CorpoGesto{VersioneAttesa: &ver, ChiaveIdempotenza: chiave()}
	if motivo != "" {
		c.Motivo = &motivo
	}
	if gesto != GestoRiattiva && m.Dettaglio != nil {
		imp := m.Dettaglio.ImprontaDipendenze
		c.ImprontaDipendenze = &imp
	}
	if m.Dettaglio != nil && m.Dettaglio.NVoci != nil {
		n, iv := *m.Dettaglio.NVoci, m.Dettaglio.ImprontaVoci
		c.NVoci, c.ImprontaVoci = &n, &iv
	}
	for _, fn := range mod {
		fn(&c)
	}
	return c
}

func conScollega(c *CorpoGesto)   { c.Scollega = vero() }
func conUnicaCopia(c *CorpoGesto) { c.ConfermaUnicaCopia = vero() }

// soloZip: il gesto sul solo archivio (U2 §3.10: perimetro contenitore, senza il perimetro delle voci).
func soloZip(c *CorpoGesto) {
	p := PerimetroContenitore
	c.Perimetro, c.NVoci, c.ImprontaVoci = &p, nil, nil
}

// zipEContenuti: il gesto sull'archivio e sulle voci (perimetro contenitore_e_voci, con il perimetro letto).
func zipEContenuti(c *CorpoGesto) {
	p := PerimetroContenitoreEVoci
	c.Perimetro = &p
}

func (b *bancoFile) scarta(thread, allegato uuid.UUID, mod ...func(*CorpoGesto)) (*EsitoGesto, error) {
	b.t.Helper()
	return b.s.Scarta(b.ctx, thread, allegato, b.u.UtenteID, b.corpo(GestoScarta, thread, allegato, "non serve alla RFQ", mod...))
}

func (b *bancoFile) altriFile(thread, allegato uuid.UUID, mod ...func(*CorpoGesto)) (*EsitoGesto, error) {
	b.t.Helper()
	return b.s.MettiInAltriFile(b.ctx, thread, allegato, b.u.UtenteID, b.corpo(GestoAltriFile, thread, allegato, "da conservare", mod...))
}

func (b *bancoFile) riattiva(thread, allegato uuid.UUID, mod ...func(*CorpoGesto)) (*EsitoGesto, error) {
	b.t.Helper()
	return b.s.Riattiva(b.ctx, thread, allegato, b.u.UtenteID, b.corpo(GestoRiattiva, thread, allegato, "", mod...))
}

// deveRiuscire: il gesto e' riuscito.
func (b *bancoFile) deveRiuscire(e *EsitoGesto, err error) *EsitoGesto {
	b.t.Helper()
	if err != nil {
		if rf, ok := ComeRifiuto(err); ok {
			b.t.Fatalf("gesto rifiutato: %d %s %q %v", rf.Stato, rf.Codice, rf.Testo, rf.Extra)
		}
		b.t.Fatalf("gesto non riuscito: %v", err)
	}
	return e
}

// rifiuto: il gesto e' rifiutato con quello stato e quel codice.
func rifiutoAtteso(t *testing.T, err error, stato int, codice string) *Rifiuto {
	t.Helper()
	if err == nil {
		t.Fatalf("atteso il rifiuto %d %s, il gesto e' riuscito", stato, codice)
	}
	rf, ok := ComeRifiuto(err)
	if !ok {
		t.Fatalf("atteso il rifiuto %d %s, errore: %v", stato, codice, err)
	}
	if rf.Stato != stato || rf.Codice != codice {
		t.Fatalf("atteso il rifiuto %d %s, avuto %d %s (%q, %v)", stato, codice, rf.Stato, rf.Codice, rf.Testo, rf.Extra)
	}
	return rf
}

// ---------------------------------------------------------------- le letture per le asserzioni

func (b *bancoFile) stato(thread, allegato uuid.UUID) string {
	b.t.Helper()
	return valore[string](b, `SELECT coalesce((SELECT stato FROM file_rfq WHERE thread_id = $1 AND allegato_id = $2), '')`, thread, allegato)
}

func (b *bancoFile) rimozione(thread, allegato uuid.UUID) string {
	b.t.Helper()
	return valore[string](b, `SELECT coalesce((SELECT stato || '/' || motivo FROM file_rfq_rimozione WHERE thread_id = $1 AND allegato_id = $2), '')`,
		thread, allegato)
}

func (b *bancoFile) rimozioneID(thread, allegato uuid.UUID) int64 {
	b.t.Helper()
	return valore[int64](b, `SELECT rimozione_id FROM file_rfq_rimozione WHERE thread_id = $1 AND allegato_id = $2`, thread, allegato)
}

// copie: le copie di conservazione dell'occorrenza, «stato/causa» in ordine di nascita.
func (b *bancoFile) copie(thread, allegato uuid.UUID) []string {
	b.t.Helper()
	return valore[[]string](b, `SELECT coalesce(array_agg(stato || '/' || coalesce(causa, '-') ORDER BY conservazione_id), '{}')
		FROM file_rfq_conservazione WHERE thread_id = $1 AND allegato_id = $2`, thread, allegato)
}

// azioni: gli eventi dell'occorrenza, in ordine.
func (b *bancoFile) azioni(thread, allegato uuid.UUID) []string {
	b.t.Helper()
	return valore[[]string](b, `SELECT coalesce(array_agg(azione ORDER BY evento_id), '{}') FROM file_rfq_evento
		WHERE thread_id = $1 AND allegato_id = $2`, thread, allegato)
}

func (b *bancoFile) pathStaging(allegato uuid.UUID) string {
	b.t.Helper()
	return valore[string](b, `SELECT coalesce(path_staging, '') FROM allegato WHERE allegato_id = $1`, allegato)
}

func (b *bancoFile) statoProposta(allegato uuid.UUID) string {
	b.t.Helper()
	return valore[string](b, `SELECT coalesce((SELECT stato::text || '/' || coalesce(componente_id::text, '-') FROM documento_proposta
		WHERE allegato_id = $1), '')`, allegato)
}

// percorsoContenuto: il posto del contenuto nello staging del banco.
func (b *bancoFile) percorsoContenuto(sha, nome string) string {
	b.t.Helper()
	p, err := staging.PercorsoContenuto(b.staging, sha, nome)
	if err != nil {
		b.t.Fatal(err)
	}
	return p
}

// fileDelDeposito: i file definitivi del conservatore delle prove, relativi alla radice.
func (b *bancoFile) fileDelDeposito() []string { return b.cons.File() }

// partiDelDeposito: i parziali rimasti sotto la radice del conservatore delle prove.
func (b *bancoFile) partiDelDeposito() []string { return b.cons.Parti() }

// copiaVerificata: il file della copia copiata dell'occorrenza, con il suo sha.
func (b *bancoFile) copiaVerificata(thread, allegato uuid.UUID) (string, string) {
	b.t.Helper()
	var cartella, rel string
	b.rigaCon(`SELECT coalesce(cartella_thread, ''), percorso_relativo FROM file_rfq_conservazione
		WHERE thread_id = $1 AND allegato_id = $2 AND stato = 'copiata'`, []any{thread, allegato}, &cartella, &rel)
	p := b.cons.Percorso(cartella, rel)
	h, err := shaDelFile(p)
	if err != nil {
		b.t.Fatalf("la copia verificata non si legge: %v", err)
	}
	return p, h
}

// foto: il contenuto del database, per dire che un rifiuto non ha scritto niente.
func (b *bancoFile) foto() map[string]string {
	b.t.Helper()
	return testutil.FotoDelDatabase(b.t, b.p)
}

func (b *bancoFile) nienteScritto(prima map[string]string, cosa string) {
	b.t.Helper()
	if d := testutil.Differenze(prima, b.foto()); len(d) > 0 {
		b.t.Errorf("%s: tabelle cambiate %v", cosa, d)
	}
}

// avanti: l'orologio del servizio spostato avanti di d (il custode e il tempo 2 lo leggono).
func (b *bancoFile) avanti(d time.Duration) {
	b.s.Adesso = func() time.Time { return time.Now().Add(d) }
}

// cache: il custode della cache dello staging, con l'orologio avanti di giorni e la retention di 30 giorni, che conosce
// il deposito del servizio dato.
func (b *bancoFile) cache(s *Servizio, giorni int) *staging.Cache {
	return &staging.Cache{Pool: b.p, Staging: b.staging, Log: testutil.LogSilenzioso(), Retention: 30 * 24 * time.Hour,
		Adesso: func() time.Time { return time.Now().Add(time.Duration(giorni) * 24 * time.Hour) }, CopiaNelDeposito: s.CopiaNelDeposito()}
}

// albero: i file sotto una cartella con il loro sha (per dire che il NAS non e' stato toccato).
func albero(t *testing.T, radice string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.WalkDir(radice, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(radice, p)
		if d.IsDir() {
			out[rel+string(filepath.Separator)] = "dir"
			return nil
		}
		h, _ := shaDelFile(p)
		out[rel] = h
		return nil
	})
	return out
}

// arrivato: un file della richiesta scritto in un parziale dello staging, come lo legge E1′.
func (b *bancoFile) arrivato(c []byte, nome string) FileArrivato {
	b.t.Helper()
	f, err := ScriviParziale(b.staging, bytesReader(c), nome, "application/octet-stream", 1<<30)
	if err != nil {
		b.t.Fatal(err)
	}
	return f
}

// attendiLucchetto: aspetta (al piu' 10 secondi) che una transazione sia ferma su un lucchetto advisory (BloccaContenuto).
func (b *bancoFile) attendiLucchetto() {
	b.t.Helper()
	fine := time.Now().Add(10 * time.Second)
	for time.Now().Before(fine) {
		var n int
		if err := b.p.QueryRow(b.ctx, `SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted`).Scan(&n); err != nil {
			b.t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	b.t.Fatal("nessuna transazione si e' fermata sul lucchetto per contenuto")
}

// documentoNAS: un documento confermato della RFQ con quel contenuto, con lo stato della copia sul NAS dato.
func (b *bancoFile) documentoNAS(thread uuid.UUID, codice, sha, statoNas string) uuid.UUID {
	b.t.Helper()
	return valore[uuid.UUID](b, `INSERT INTO documento (thread_id, tipo, codice, nome_file, estensione, sha256, bytes, path_relativo, stato_nas, confermato_da)
		VALUES ($1, 'disegno_2d', $2, $3, 'pdf', $4, 10, $5, $6::stato_nas, $7) RETURNING documento_id`,
		thread, codice, codice+".pdf", sha, `ELENCO DISEGNI\`+codice+`\`+codice+".pdf", statoNas, b.u.UtenteID)
}

// uguali: due elenchi uguali, nello stesso ordine.
func uguali(a []string, b ...string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stessoAlbero(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
