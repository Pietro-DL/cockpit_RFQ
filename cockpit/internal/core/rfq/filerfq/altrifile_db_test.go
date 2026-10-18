//go:build integrazione

package filerfq

// L4 — V1, corsia file: le prove di «Metti in Altri file» e di «Riattiva» (PM23 §10 riga 7, AF-01…AF-16; A2 §8.1;
// EM §10.5, §10.6). AF-06 e' la L1 della configurazione (platform/config), AF-08 e AF-10 stanno con le rotte
// (transport/web), AF-07 con il worker (transport/workerapi).

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
)

// AF-01: «Altri file» su un PDF attivo: la copia nel deposito con lo sha giusto, la conservazione copiata, lo stato
// altri_file, la proposta ancora aperta; il file esce dal fascicolo (v_fascicolo), dalla distribuzione delle letture
// (ListProposteAperteStessoFile) e dal pin 3 della cache; uno STEP in Altri file esce dalle query della struttura
// (ListStepDellaRfq, ListStepDaAnalizzareDellaRfq).
func TestAF01AltriFileSuUnPDFAttivo(t *testing.T) {
	b := nuovoBancoFile(t)
	comp := b.componente(b.thread, "ACME-100", "sciolto")
	msg := b.mail(b.thread)
	a, sha := b.file(msg, "ACME-100.pdf", contenuto("ACME-100.pdf", 11, 5_000), opzFile{codice: "ACME-100"})
	st, _ := b.file(msg, "ACME-100.stp", contenuto("ACME-100.stp", 12, 5_000), opzFile{})
	proposta := valore[uuid.UUID](b, `SELECT proposta_id FROM documento_proposta WHERE allegato_id = $1`, a)
	fascicolo := func() string {
		return valore[string](b, `SELECT coalesce(proposta_aperta::text, '-') FROM v_fascicolo WHERE componente_id = $1 AND tipo_documento = 'disegno_2d'`, comp)
	}
	conLaProposta := func(sha string, id uuid.UUID) bool {
		righe, err := b.q.ListProposteAperteStessoFile(b.ctx, pgtype.Text{String: sha, Valid: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range righe {
			if r.AllegatoID == id {
				return true
			}
		}
		return false
	}
	pin := func(sha string) string {
		righe, err := b.q.ListContenutiPinnati(b.ctx, []string{sha})
		if err != nil {
			t.Fatal(err)
		}
		if len(righe) == 0 {
			return ""
		}
		return righe[0].Motivo
	}
	nellaStruttura := func() (bool, bool) {
		th := uuid.NullUUID{UUID: b.thread, Valid: true}
		uno, err := b.q.ListStepDellaRfq(b.ctx, th)
		if err != nil {
			t.Fatal(err)
		}
		due, err := b.q.ListStepDaAnalizzareDellaRfq(b.ctx, th)
		if err != nil {
			t.Fatal(err)
		}
		in := func(l []db.Allegato) bool {
			for _, x := range l {
				if x.AllegatoID == st {
					return true
				}
			}
			return false
		}
		return in(uno), in(due)
	}
	// prima del gesto il file c'e' dappertutto: la prova guarda dove deve
	if got := fascicolo(); got != proposta.String() {
		t.Fatalf("prima del gesto v_fascicolo non porta la proposta: %s", got)
	}
	if !conLaProposta(sha, a) || pin(sha) != "proposta ancora aperta" {
		t.Fatalf("prima del gesto: distribuzione %v, pin %q", conLaProposta(sha, a), pin(sha))
	}
	if s1, s2 := nellaStruttura(); !s1 || !s2 {
		t.Fatalf("prima del gesto lo STEP manca dalle query della struttura: %v %v", s1, s2)
	}

	e := b.deveRiuscire(b.altriFile(b.thread, a))
	if e.Esito != "in_altri_file" || e.Stato != StatoAltriFile || e.Versione != 1 || e.Riproposto {
		t.Errorf("esito %+v", e)
	}
	if _, h := b.copiaVerificata(b.thread, a); h != sha {
		t.Errorf("la copia nel deposito ha lo sha %s, il file %s", h, sha)
	}
	if got := b.copie(b.thread, a); !uguali(got, "copiata/-") {
		t.Errorf("conservazione %v", got)
	}
	if got := b.stato(b.thread, a); got != StatoAltriFile {
		t.Errorf("stato %q", got)
	}
	if got := b.statoProposta(a); got != "aperta/-" {
		t.Errorf("la proposta non resta aperta: %s", got)
	}
	if got := b.azioni(b.thread, a); !uguali(got, "conservazione_copiata", "altri_file") {
		t.Errorf("eventi %v", got)
	}
	if got := fascicolo(); got != "-" {
		t.Errorf("v_fascicolo porta ancora la proposta di un file in Altri file: %s", got)
	}
	if conLaProposta(sha, a) {
		t.Error("la distribuzione delle letture porta ancora la proposta di un file in Altri file")
	}
	if got := pin(sha); got != staging.MotivoPropostaAltriFile {
		t.Errorf("pin del contenuto %q, atteso %q (il pin 3 non vale per un file in Altri file)", got, staging.MotivoPropostaAltriFile)
	}
	b.deveRiuscire(b.altriFile(b.thread, st))
	if s1, s2 := nellaStruttura(); s1 || s2 {
		t.Errorf("lo STEP in Altri file resta nelle query della struttura: %v %v", s1, s2)
	}
}

// AF-02: riavvio e pulizia dello staging: un servizio nuovo sullo stesso DB e sullo stesso deposito (stesso
// segnaposto), il custode della cache con l'orologio avanti di 40 giorni toglie lo staging; i byte si leggono dal
// deposito con lo sha giusto (la rotta S5 la prova transport/web).
func TestAF02RiavvioEPuliziaDelloStaging(t *testing.T) {
	b := nuovoBancoFile(t)
	a, sha := b.pdf("ACME-200.pdf")
	b.deveRiuscire(b.altriFile(b.thread, a))
	vecchio := b.pathStaging(a)
	dep, err := PreparaDeposito(b.ctx, b.q, b.deposito)
	if err != nil {
		t.Fatalf("il riavvio non riparte con le copie del suo deposito: %v", err)
	}
	if dep.ID != b.s.Deposito.ID {
		t.Fatalf("il segnaposto e' cambiato al riavvio: %s → %s", b.s.Deposito.ID, dep.ID)
	}
	s2 := &Servizio{Pool: b.p, Staging: b.staging, Deposito: dep, Log: b.s.Log}
	giro, err := b.cache(s2, 40).Giro(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if giro.Rimossi != 1 || esiste(vecchio) || b.pathStaging(a) != "" {
		t.Fatalf("la cache non ha tolto lo staging del file in Altri file: %+v, file %v, path %q", giro, esiste(vecchio), b.pathStaging(a))
	}
	f, _, err := s2.ApriCopia(b.ctx, b.thread, a, true)
	if err != nil || f == nil {
		t.Fatalf("la copia di conservazione non si apre: %v", err)
	}
	defer f.Close()
	letti, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if shaDi(letti) != sha {
		t.Error("i byte dal deposito non hanno lo sha del file")
	}
}

// AF-03: la copia che fallisce (deposito che non accetta i file, sorgente assente, sha alterato) → 422, stato attivo,
// nessun parziale; il trigger rifiuta un altri_file scritto a mano senza una copia copiata.
func TestAF03CopiaCheFallisce(t *testing.T) {
	t.Run("deposito che non accetta i file", func(t *testing.T) {
		b := nuovoBancoFile(t)
		a, _ := b.pdf("ACME-301.pdf")
		// al posto della cartella dei parziali c'e' un file: nel deposito non si scrive (come un deposito in sola
		// lettura, che su Windows non si simula con i permessi della cartella)
		parti := filepath.Join(b.deposito, CartellaPartiDeposito)
		if err := os.RemoveAll(parti); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(parti, []byte("non una cartella"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := b.altriFile(b.thread, a)
		rf := rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "CONSERVAZIONE_NON_RIUSCITA")
		if rf.Extra["causa"] != "permessi" {
			t.Errorf("causa %v", rf.Extra["causa"])
		}
		if got := b.stato(b.thread, a); got != "" {
			t.Errorf("stato %q dopo una copia non riuscita", got)
		}
		if got := b.copie(b.thread, a); !uguali(got, "errore_copia/permessi") {
			t.Errorf("conservazione %v", got)
		}
		if got := b.fileDelDeposito(); len(got) != 0 {
			t.Errorf("file nel deposito: %v", got)
		}
	})
	t.Run("sorgente assente", func(t *testing.T) {
		b := nuovoBancoFile(t)
		a, _ := b.pdf("ACME-302.pdf")
		if err := os.Remove(b.pathStaging(a)); err != nil {
			t.Fatal(err)
		}
		prima := b.foto()
		_, err := b.altriFile(b.thread, a)
		rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "FILE_SENZA_BYTE")
		b.nienteScritto(prima, "FILE_SENZA_BYTE")
	})
	t.Run("sha alterato", func(t *testing.T) {
		b := nuovoBancoFile(t)
		a, _ := b.pdf("ACME-303.pdf")
		if err := os.WriteFile(b.pathStaging(a), []byte("%PDF-1.4 altri byte con lo stesso nome"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := b.altriFile(b.thread, a)
		rf := rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "CONSERVAZIONE_NON_RIUSCITA")
		if rf.Extra["causa"] != "verifica_sha" {
			t.Errorf("causa %v", rf.Extra["causa"])
		}
		if got := b.copie(b.thread, a); !uguali(got, "errore_copia/verifica_sha") {
			t.Errorf("conservazione %v", got)
		}
		if got := b.stato(b.thread, a); got != "" {
			t.Errorf("stato %q", got)
		}
		if p := b.partiDelDeposito(); len(p) != 0 {
			t.Errorf("parziali rimasti: %v", p)
		}
		if got := b.fileDelDeposito(); len(got) != 0 {
			t.Errorf("file nel deposito: %v", got)
		}
	})
	t.Run("altri_file a mano senza copia", func(t *testing.T) {
		b := nuovoBancoFile(t)
		a, _ := b.pdf("ACME-304.pdf")
		tx, err := b.p.Begin(b.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(b.ctx)
		if _, err := tx.Exec(b.ctx, `INSERT INTO file_rfq (thread_id, allegato_id, stato, versione, messaggio_id, canale, contenitore_id, indice,
				nome_file, path_interno, estensione, sha256, bytes, origine, ricevuto_il, caricato_da, creato_da, aggiornato_da)
			SELECT m.thread_id, a.allegato_id, 'altri_file', 1, a.messaggio_id, m.canale, a.contenitore_id, a.indice, a.nome_file,
			       a.path_interno, a.estensione, a.sha256, a.bytes, a.origine, a.ricevuto_il, a.caricato_da, $2, $2
			  FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE a.allegato_id = $1`, a, b.u.UtenteID); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(b.ctx, `INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, stato_prima, stato_dopo, fotografia,
				dipendenze, origine, autore_id) VALUES ($1, $2, 'altri_file', 'attivo', 'altri_file', '{}', '{}', 'ponte', $3)`,
			b.thread, a, b.u.UtenteID); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(b.ctx); err == nil || !strings.Contains(err.Error(), "senza una copia di conservazione verificata") {
			t.Fatalf("il trigger lascia un file in Altri file senza copia: %v", err)
		}
		if got := b.stato(b.thread, a); got != "" {
			t.Errorf("stato %q", got)
		}
	})
}

// AF-04: il definitivo della riga c'e' gia' con un altro contenuto: errore_copia / conflitto, e il file non si
// sovrascrive (ne' si rilascia: non e' della riga).
func TestAF04DefinitivoGiaPresenteConUnAltroContenuto(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-400.pdf")
	prossimo := valore[int64](b, `SELECT coalesce(pg_sequence_last_value(pg_get_serial_sequence('file_rfq_conservazione', 'conservazione_id')::regclass), 0) + 1`)
	estraneo := filepath.Join(b.deposito, b.thread.String(), fmt.Sprintf("%s.%d.pdf", a, prossimo))
	if err := os.MkdirAll(filepath.Dir(estraneo), 0o755); err != nil {
		t.Fatal(err)
	}
	altro := []byte("un altro contenuto, scritto da chi sa chi")
	if err := os.WriteFile(estraneo, altro, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := b.altriFile(b.thread, a)
	rf := rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "CONSERVAZIONE_NON_RIUSCITA")
	if rf.Extra["causa"] != "conflitto" {
		t.Errorf("causa %v", rf.Extra["causa"])
	}
	if got := b.copie(b.thread, a); !uguali(got, "errore_copia/conflitto") {
		t.Errorf("conservazione %v", got)
	}
	if c, err := os.ReadFile(estraneo); err != nil || string(c) != string(altro) {
		t.Errorf("il file gia' presente e' stato toccato: %q %v", c, err)
	}
	if got := b.stato(b.thread, a); got != "" {
		t.Errorf("stato %q", got)
	}
}

// AF-05: «Riattiva» dopo che la cache ha tolto i byte: i byte tornano nello staging, verificati; stato attivo; la strada
// d'ingresso rifatta; la copia rilasciata; nessun documento nuovo, nessuno step_strutturale_id, nessun
// fonte_step_evento, nessuna associazione rimessa.
func TestAF05RiattivaDopoCheLaCacheHaToltoIByte(t *testing.T) {
	b := nuovoBancoFile(t)
	comp := b.componente(b.thread, "ACME-500", "sciolto")
	a, sha := b.file(b.mail(b.thread), "ACME-500.pdf", contenuto("ACME-500.pdf", 50, 4_000), opzFile{componente: comp, codice: "ACME-500"})
	_, err := b.altriFile(b.thread, a)
	rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "RISOLUZIONE_RICHIESTA") // D-7 senza «scollega»
	b.deveRiuscire(b.altriFile(b.thread, a, conScollega))
	if got := b.statoProposta(a); got != "aperta/-" {
		t.Fatalf("la proposta associata non e' stata scollegata: %s", got)
	}
	vecchio := b.pathStaging(a)
	if giro, err := b.cache(b.s, 40).Giro(b.ctx); err != nil || giro.Rimossi != 1 || esiste(vecchio) {
		t.Fatalf("la cache non ha tolto lo staging: %+v %v", giro, err)
	}
	conta := func() string {
		return valore[string](b, `SELECT (SELECT count(*) FROM documento) || '/' || (SELECT count(*) FROM fonte_step_evento) || '/' ||
			(SELECT count(*) FROM componente WHERE step_strutturale_id IS NOT NULL) || '/' || (SELECT count(*) FROM associazione_evento)`)
	}
	prima := conta()
	e := b.deveRiuscire(b.riattiva(b.thread, a))
	if e.Esito != "riattivato" || e.Stato != StatoAttivo || e.ByteRipristinati == nil || !*e.ByteRipristinati {
		t.Errorf("esito %+v", e)
	}
	p := b.pathStaging(a)
	if p != b.percorsoContenuto(sha, "ACME-500.pdf") || !staging.ContenutoGiaPresente(p, sha) {
		t.Errorf("i byte non sono tornati nello staging, verificati: %q", p)
	}
	if got := b.stato(b.thread, a); got != StatoAttivo {
		t.Errorf("stato %q", got)
	}
	if b.pipe.quante(a) != 1 {
		t.Errorf("la strada d'ingresso e' stata rifatta %d volte", b.pipe.quante(a))
	}
	if got := b.copie(b.thread, a); !uguali(got, "rilasciata/riattivato") {
		t.Errorf("conservazione %v", got)
	}
	if got := b.fileDelDeposito(); len(got) != 0 {
		t.Errorf("la copia rilasciata e' ancora nel deposito: %v", got)
	}
	if dopo := conta(); dopo != prima {
		t.Errorf("documenti/fonte_step_evento/step_strutturale_id/associazioni: %s → %s", prima, dopo)
	}
	if got := b.statoProposta(a); got != "aperta/-" {
		t.Errorf("l'associazione e' stata rimessa: %s", got)
	}
	if got := b.azioni(b.thread, a); !uguali(got, "conservazione_copiata", "altri_file", "riattiva", "conservazione_rilasciata") {
		t.Errorf("eventi %v", got)
	}
}

// AF-09: il NAS finto: nessuna scrittura sul NAS, nessun documento creato, la cartella ALTRO non si tocca.
func TestAF09NessunEffettoSulNAS(t *testing.T) {
	b := nuovoBancoFile(t)
	altro := filepath.Join(b.nas, "ACME", "WIP", "2026 10 18 RFQ-A", "ALTRO")
	if err := os.MkdirAll(altro, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(altro, "nota.txt"), []byte("del cliente"), 0o644); err != nil {
		t.Fatal(err)
	}
	prima := albero(t, b.nas)
	conta := func() string {
		return valore[string](b, `SELECT (SELECT count(*) FROM documento) || '/' || (SELECT count(*) FROM job)`)
	}
	c0 := conta()
	a, _ := b.pdf("ACME-900.pdf")
	b.deveRiuscire(b.altriFile(b.thread, a))
	b.deveRiuscire(b.riattiva(b.thread, a))
	b.deveRiuscire(b.altriFile(b.thread, a))
	b.deveRiuscire(b.scarta(b.thread, a))
	if !stessoAlbero(prima, albero(t, b.nas)) {
		t.Errorf("il NAS e' cambiato: %v → %v", prima, albero(t, b.nas))
	}
	if c := conta(); c != c0 {
		t.Errorf("documenti/job: %s → %s", c0, c)
	}
}

// AF-11: «Riattiva» con il custode della cache in mezzo (fra il controllo dello staging e la fase C): la fase C, sotto
// BloccaContenuto, ripristina dal deposito; il rilascio della copia aspetta lo staging verificato; il file resta
// apribile con lo sha giusto.
func TestAF11RiattivaConLaCacheInMezzo(t *testing.T) {
	b := nuovoBancoFile(t)
	a, sha := b.pdf("ACME-1100.pdf")
	b.deveRiuscire(b.altriFile(b.thread, a))
	p := b.pathStaging(a)
	var giro staging.EsitoCache
	var errGiro error
	b.s.Agganci = &Agganci{PrimaDellaFaseC: func(uuid.UUID) {
		giro, errGiro = b.cache(b.s, 40).Giro(b.ctx)
	}}
	e := b.deveRiuscire(b.riattiva(b.thread, a))
	if errGiro != nil || giro.Rimossi != 1 {
		t.Fatalf("la cache non ha tolto lo staging in mezzo: %+v %v (la prova non prova niente)", giro, errGiro)
	}
	if e.ByteRipristinati == nil || !*e.ByteRipristinati {
		t.Errorf("la fase C non ha ripristinato i byte: %+v", e)
	}
	if h, err := shaDelFile(p); err != nil || h != sha {
		t.Errorf("lo staging dopo la «Riattiva»: %s %v", h, err)
	}
	if b.pathStaging(a) != p {
		t.Errorf("path_staging %q, atteso %q", b.pathStaging(a), p)
	}
	if got := b.copie(b.thread, a); !uguali(got, "rilasciata/riattivato") {
		t.Errorf("conservazione %v", got)
	}
}

// AF-12: «Metti in Altri file» di un allegato di posta mai scaricato → 422 FILE_SENZA_BYTE, nessuna riga di
// conservazione; scaricato il file, un tentativo nuovo con una chiave nuova riesce subito.
func TestAF12AllegatoMaiScaricato(t *testing.T) {
	b := nuovoBancoFile(t)
	nome := "ACME-1200.pdf"
	a, _ := b.file(b.mail(b.thread), nome, nil, opzFile{senzaByte: true})
	prima := b.foto()
	_, err := b.altriFile(b.thread, a)
	rf := rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "FILE_SENZA_BYTE")
	if !strings.Contains(rf.Testo, "Scarica") {
		t.Errorf("un allegato di posta si scarica: %q", rf.Testo)
	}
	b.nienteScritto(prima, "FILE_SENZA_BYTE")
	c := contenuto(nome, 1200, 3_000)
	sha := shaDi(c)
	p := b.percorsoContenuto(sha, nome)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, c, 0o644); err != nil {
		t.Fatal(err)
	}
	b.esegui(`UPDATE allegato SET sha256 = $2, bytes = $3, path_staging = $4, stato = 'in_staging' WHERE allegato_id = $1`, a, sha, len(c), p)
	b.deveRiuscire(b.altriFile(b.thread, a))
	if got := b.copie(b.thread, a); !uguali(got, "copiata/-") {
		t.Errorf("conservazione %v", got)
	}
}

// AF-13: esiti diversi dal successo su un archivio: la copia di una voce fallisce → tutte le righe del gesto in uno
// stato finale (errore_copia o rilasciata), il rilascio toglie i file gia' copiati: nessun file orfano nel deposito.
func TestAF13ArchivioConUnaVoceCheFallisce(t *testing.T) {
	b := nuovoBancoFile(t)
	zip, voci := b.archivio(b.thread, "ACME.zip", []string{"V1.pdf", "V2.pdf", "V3.pdf", "V4.pdf"}, opzFile{})
	alterata := voci[2]
	if err := os.WriteFile(b.pathStaging(alterata), []byte("%PDF-1.4 byte alterati"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := b.altriFile(b.thread, zip)
	rf := rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "CONSERVAZIONE_NON_RIUSCITA")
	if rf.Extra["allegato_id"] != alterata {
		t.Errorf("il rifiuto non nomina la voce che non si copia: %v", rf.Extra["allegato_id"])
	}
	if n := valore[int](b, `SELECT count(*) FROM file_rfq_conservazione WHERE stato IN ('in_copia', 'copiata', 'da_rilasciare')`); n != 0 {
		t.Errorf("%d righe del gesto non in uno stato finale", n)
	}
	if got := b.copie(b.thread, alterata); !uguali(got, "errore_copia/verifica_sha") {
		t.Errorf("la voce alterata: %v", got)
	}
	if got := b.fileDelDeposito(); len(got) != 0 {
		t.Errorf("file orfani nel deposito: %v", got)
	}
	if p := b.partiDelDeposito(); len(p) != 0 {
		t.Errorf("parziali rimasti: %v", p)
	}
	for _, id := range append([]uuid.UUID{zip}, voci...) {
		if got := b.stato(b.thread, id); got != "" {
			t.Errorf("%s: stato %q dopo un gesto non riuscito", id, got)
		}
	}
}

// AF-14: «Altri file» → «Riattiva» con il rilascio bloccato (file aperto) → di nuovo «Altri file» → custode: la seconda
// copia ha un altro file, il rilascio della prima non la tocca, il contenuto resta apribile con lo sha giusto.
func TestAF14DueCicliDueFile(t *testing.T) {
	b := nuovoBancoFile(t)
	a, sha := b.pdf("ACME-1400.pdf")
	b.deveRiuscire(b.altriFile(b.thread, a))
	primo, _ := b.copiaVerificata(b.thread, a)
	aperto, err := os.Open(primo) // senza FILE_SHARE_DELETE: il rilascio non lo cancella
	if err != nil {
		t.Fatal(err)
	}
	chiuso := false
	defer func() {
		if !chiuso {
			aperto.Close()
		}
	}()
	b.deveRiuscire(b.riattiva(b.thread, a))
	if got := b.copie(b.thread, a); !uguali(got, "da_rilasciare/riattivato") {
		t.Fatalf("il rilascio con il file aperto: %v", got)
	}
	b.deveRiuscire(b.altriFile(b.thread, a))
	secondo, h := b.copiaVerificata(b.thread, a)
	if strings.EqualFold(primo, secondo) || h != sha {
		t.Fatalf("la seconda copia: %s (%s), la prima %s", secondo, h, primo)
	}
	aperto.Close()
	chiuso = true
	b.avanti(2 * time.Hour)
	if _, err := (&Custode{S: b.s}).Giro(b.ctx); err != nil {
		t.Fatal(err)
	}
	if got := b.copie(b.thread, a); !uguali(got, "rilasciata/riattivato", "copiata/-") {
		t.Errorf("conservazione %v", got)
	}
	if esiste(primo) {
		t.Error("il file della prima copia e' ancora nel deposito")
	}
	if h, err := shaDelFile(secondo); err != nil || h != sha {
		t.Errorf("la seconda copia dopo il rilascio della prima: %s %v", h, err)
	}
}

// AF-15: la radice del deposito cambiata senza spostare i file, o il segnaposto sparito: con righe copiate il server
// non parte e lo dice; la cache non toglie lo staging di un file in Altri file il cui file del deposito manca.
func TestAF15DepositoCambiatoOSegnapostoSparito(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-1500.pdf")
	b.deveRiuscire(b.altriFile(b.thread, a))
	if _, err := PreparaDeposito(b.ctx, b.q, t.TempDir()); err == nil {
		t.Error("con una radice nuova e righe copiate il server parte")
	}
	if err := os.Remove(filepath.Join(b.deposito, NomeSegnaposto)); err != nil {
		t.Fatal(err)
	}
	if _, err := PreparaDeposito(b.ctx, b.q, b.deposito); err == nil {
		t.Error("senza il segnaposto e con righe copiate il server parte")
	}
	// il file della copia sparito: la cache non toglie lo staging
	p, _ := b.copiaVerificata(b.thread, a)
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	staging := b.pathStaging(a)
	giro, err := b.cache(b.s, 40).Giro(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if giro.Rimossi != 0 || !esiste(staging) || b.pathStaging(a) == "" {
		t.Errorf("la cache ha tolto lo staging di un file in Altri file senza la sua copia: %+v", giro)
	}
}

// AF-16: «Metti in Altri file» su uno zip la cui voce e' un documento confermato (D-1) o la fonte della bozza (D-6) →
// 422 DIPENDENZE_ATTIVE con l'allegato_id della voce, niente scritto.
func TestAF16ZipConUnaVoceCheBlocca(t *testing.T) {
	b := nuovoBancoFile(t)
	zip, voci := b.archivio(b.thread, "ACME.zip", []string{"P1.stp", "P1.pdf"}, opzFile{})
	b.esegui(`UPDATE documento_proposta SET stato = 'confermata', deciso_da = $2, deciso_il = now() WHERE allegato_id = $1`, voci[1], b.u.UtenteID)
	prima := b.foto()
	_, err := b.altriFile(b.thread, zip)
	rf := rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "DIPENDENZE_ATTIVE")
	if !citaDipendenza(rf, voci[1], "D-1") {
		t.Errorf("il rifiuto non porta la D-1 della voce: %v", rf.Extra["dipendenze"])
	}
	b.nienteScritto(prima, "DIPENDENZE_ATTIVE (D-1)")
	// la fonte della bozza aperta (D-6)
	b.esegui(`UPDATE documento_proposta SET stato = 'aperta', deciso_da = NULL, deciso_il = NULL WHERE allegato_id = $1`, voci[1])
	b.bozzaConFonte(voci[0], nil)
	prima = b.foto()
	_, err = b.altriFile(b.thread, zip)
	rf = rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "DIPENDENZE_ATTIVE")
	if !citaDipendenza(rf, voci[0], "D-6") {
		t.Errorf("il rifiuto non porta la D-6 della voce fonte: %v", rf.Extra["dipendenze"])
	}
	b.nienteScritto(prima, "DIPENDENZE_ATTIVE (D-6)")
}

// citaDipendenza: il rifiuto porta una dipendenza con quel codice per quell'occorrenza.
func citaDipendenza(rf *Rifiuto, allegato uuid.UUID, codice string) bool {
	for _, chiave := range []string{"dipendenze", "occorrenze"} {
		l, _ := rf.Extra[chiave].([]Dipendenza)
		for _, d := range l {
			if d.AllegatoID == allegato && d.Codice == codice {
				return true
			}
		}
	}
	return false
}

// bozzaConFonte: una bozza aperta (formato 2) di un prodotto finito della RFQ con la fonte data; snapshot: le chiavi
// in piu' dello snapshot (documenti_preassociati, decisioni). Restituisce la bozza.
func (b *bancoFile) bozzaConFonte(fonte uuid.UUID, snapshot map[string]string) uuid.UUID {
	b.t.Helper()
	b.n++
	prodotto := b.componente(b.thread, fmt.Sprintf("ACME-P%d", b.n), "finito")
	bozza := uuid.New()
	tx, err := b.p.Begin(b.ctx)
	if err != nil {
		b.t.Fatal(err)
	}
	defer tx.Rollback(b.ctx)
	firma := strings.Repeat("a", 64)
	if _, err := tx.Exec(b.ctx, `INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
		VALUES ($1, $2, $3, 0, 0, $4, $5, $5)`, bozza, b.thread, prodotto, firma, b.u.UtenteID); err != nil {
		b.t.Fatal(err)
	}
	b.versioneBozza(tx, bozza, 1, fonte, snapshot)
	if err := tx.Commit(b.ctx); err != nil {
		b.t.Fatal(err)
	}
	return bozza
}

// versioneBozza: una versione salvata della bozza (formato 2) con la fonte e le chiavi date.
func (b *bancoFile) versioneBozza(tx interface {
	Exec(ctx context.Context, sql string, arg ...any) (pgconn.CommandTag, error)
}, bozza uuid.UUID, versione int, fonte uuid.UUID, snapshot map[string]string) {
	b.t.Helper()
	snap := `{"formato": "2"`
	for k, v := range snapshot {
		snap += `, "` + k + `": ` + v
	}
	snap += `}`
	var fid *uuid.UUID
	var fsha *string
	if fonte != uuid.Nil {
		fid = &fonte
		s := valore[string](b, `SELECT sha256 FROM allegato WHERE allegato_id = $1`, fonte)
		fsha = &s
	}
	if _, err := tx.Exec(b.ctx, `INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore,
		firma_base, fonte_allegato_id, fonte_sha256, salvata_da) VALUES ($1, $2, $3::jsonb, $4, 0, 0, $5, $6, $7, $8)`,
		bozza, versione, snap, fmt.Sprintf("%064x", versione), strings.Repeat("a", 64), fid, fsha, b.u.UtenteID); err != nil {
		b.t.Fatal(err)
	}
}
