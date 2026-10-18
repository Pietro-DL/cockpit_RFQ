//go:build integrazione

package filerfq

// L4 — V1, corsia file: le prove di «Scarta» (PM23 §10 riga 8, SC-01…SC-21; A2 §8.2; EM §10.3, §10.4). SC-05 apre il
// file con un handle di Windows senza FILE_SHARE_DELETE (os.Open): sugli altri sistemi la prova si dichiara
// SALTATA-AMBIENTE.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
)

// SC-01: un'occorrenza sola, contenuto non condiviso: il file esce da _contenuti, path_staging nullo solo per lei,
// eseguita / rimosso, la proposta scartata con l'autore, associazione_evento «scarta», l'evento con fotografia e motivo,
// le righe di messaggio e allegato restano, il NAS non si tocca.
func TestSC01OccorrenzaSola(t *testing.T) {
	b := nuovoBancoFile(t)
	nas := albero(t, b.nas)
	a, sha := b.pdf("ACME-S01.pdf")
	p := b.pathStaging(a)
	e := b.deveRiuscire(b.scarta(b.thread, a))
	if e.Esito != "scartato" || e.Stato != StatoScartato || e.Versione != 1 {
		t.Errorf("esito %+v", e)
	}
	if e.Testo != "File scartato ed eliminato dallo staging." {
		t.Errorf("testo %q", e.Testo)
	}
	if esiste(p) || b.pathStaging(a) != "" {
		t.Errorf("il file resta nello staging: %v, path %q", esiste(p), b.pathStaging(a))
	}
	if got := b.rimozione(b.thread, a); got != "eseguita/rimosso" {
		t.Errorf("rimozione %s", got)
	}
	if got := valore[string](b, `SELECT stato::text || '/' || (deciso_da = $2)::text FROM documento_proposta WHERE allegato_id = $1`, a, b.u.UtenteID); got != "scartata/true" {
		t.Errorf("proposta %s", got)
	}
	if n := valore[int](b, `SELECT count(*) FROM associazione_evento e JOIN documento_proposta p ON p.proposta_id = e.proposta_id
		WHERE p.allegato_id = $1 AND e.azione = 'scarta' AND e.autore_id = $2`, a, b.u.UtenteID); n != 1 {
		t.Errorf("associazione_evento «scarta»: %d", n)
	}
	var motivo, nome, shaFoto string
	b.rigaCon(`SELECT motivo, fotografia ->> 'nome_file', fotografia ->> 'sha256' FROM file_rfq_evento WHERE allegato_id = $1 AND azione = 'scarta'`,
		[]any{a}, &motivo, &nome, &shaFoto)
	if motivo != "non serve alla RFQ" || nome != "ACME-S01.pdf" || shaFoto != sha {
		t.Errorf("evento: motivo %q, fotografia %q %q", motivo, nome, shaFoto)
	}
	if got := b.azioni(b.thread, a); !uguali(got, "scarta", "rimozione_eseguita") {
		t.Errorf("eventi %v", got)
	}
	if n := valore[int](b, `SELECT count(*) FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE a.allegato_id = $1`, a); n != 1 {
		t.Error("allegato o messaggio spariti")
	}
	if !stessoAlbero(nas, albero(t, b.nas)) {
		t.Error("il NAS e' cambiato")
	}
}

// SC-02: due occorrenze della stessa RFQ con lo stesso contenuto (lo stesso file): B tiene percorso e file, A
// non_necessaria_condiviso / condiviso.
func TestSC02DueOccorrenzeStessoFile(t *testing.T) {
	b := nuovoBancoFile(t)
	msg := b.mail(b.thread)
	c := contenuto("ACME-S02.pdf", 202, 3_000)
	a1, _ := b.file(msg, "ACME-S02.pdf", c, opzFile{})
	a2, _ := b.file(msg, "ACME-S02 copia.pdf", c, opzFile{})
	p := b.pathStaging(a1)
	if p != b.pathStaging(a2) {
		t.Fatal("le due occorrenze non indicano lo stesso file")
	}
	e := b.deveRiuscire(b.scarta(b.thread, a1))
	if got := b.rimozione(b.thread, a1); got != "non_necessaria_condiviso/condiviso" {
		t.Errorf("rimozione %s", got)
	}
	if !esiste(p) || b.pathStaging(a2) != p || b.pathStaging(a1) != "" {
		t.Errorf("file %v, path di B %q, path di A %q", esiste(p), b.pathStaging(a2), b.pathStaging(a1))
	}
	if !strings.Contains(e.Testo, "la usano altri file") {
		t.Errorf("testo %q", e.Testo)
	}
}

// SC-03: due RFQ con lo stesso contenuto; nella B la copia sul NAS di un documento e' in coda: dopo lo scarto in A, la
// B ha percorso e file, e la copia sul NAS trova la sua sorgente nello staging.
func TestSC03DueRFQConLaCopiaNASInCoda(t *testing.T) {
	b := nuovoBancoFile(t)
	rfqB := b.rfq("RFQ-B")
	c := contenuto("ACME-S03.pdf", 303, 3_000)
	a, sha := b.file(b.mail(b.thread), "ACME-S03.pdf", c, opzFile{})
	bb, _ := b.file(b.mail(rfqB), "ACME-S03.pdf", c, opzFile{})
	doc := b.documentoNAS(rfqB, "ACME-S03", sha, "in_coda")
	b.esegui(`INSERT INTO documento_provenienza (documento_id, allegato_id, ricevuto_il) VALUES ($1, $2, now())`, doc, bb)
	p := b.pathStaging(bb)
	b.deveRiuscire(b.scarta(b.thread, a))
	if got := b.rimozione(b.thread, a); got != "non_necessaria_condiviso/condiviso" {
		t.Errorf("rimozione %s", got)
	}
	if !esiste(p) || b.pathStaging(bb) != p {
		t.Fatalf("la RFQ B ha perso il file: %v %q", esiste(p), b.pathStaging(bb))
	}
	d, err := b.q.GetDocumento(b.ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	src, err := documenti.SorgenteStaging(b.ctx, b.q, d)
	if err != nil || !strings.EqualFold(src, p) {
		t.Errorf("la copia sul NAS della RFQ B non trova la sua sorgente: %q %v", src, err)
	}
}

// SC-04: lo stesso contenuto con due estensioni (.stp, .step) sono due file: si considera solo il file dell'occorrenza;
// l'altro file e il percorso dell'altra occorrenza restano (la correzione di AzzeraPathStagingPerSha), e il pin
// «proposta aperta» dell'altro file non ferma la rimozione.
func TestSC04StessoContenutoDueEstensioni(t *testing.T) {
	b := nuovoBancoFile(t)
	msg := b.mail(b.thread)
	c := contenuto("ACME-S04.stp", 404, 3_000)
	a, _ := b.file(msg, "ACME-S04.stp", c, opzFile{})
	bb, _ := b.file(msg, "ACME-S04.step", c, opzFile{})
	pa, pb := b.pathStaging(a), b.pathStaging(bb)
	if strings.EqualFold(pa, pb) {
		t.Fatal("le due estensioni indicano lo stesso file")
	}
	b.deveRiuscire(b.scarta(b.thread, a))
	if got := b.rimozione(b.thread, a); got != "eseguita/rimosso" {
		t.Errorf("rimozione %s (il pin dell'altro file non conta)", got)
	}
	if esiste(pa) {
		t.Error("il file dell'occorrenza scartata resta")
	}
	if !esiste(pb) || b.pathStaging(bb) != pb {
		t.Errorf("l'altro file o il suo percorso sono stati toccati: %v %q", esiste(pb), b.pathStaging(bb))
	}
}

// SC-05 (Windows): il file aperto da un altro processo: errore / file_occupato, path_staging ancora valorizzato, evento
// rimozione_errore, il testo «ancora da completare», mai «eliminato»; chiuso il file, il custode porta a eseguita.
func TestSC05FileApertoDaUnAltroProcesso(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("SALTATA-AMBIENTE: il file aperto che non si cancella e' di Windows (SC-05)")
	}
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-S05.pdf")
	p := b.pathStaging(a)
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	e := b.deveRiuscire(b.scarta(b.thread, a))
	f.Close()
	if got := b.rimozione(b.thread, a); got != "errore/file_occupato" {
		t.Fatalf("rimozione %s", got)
	}
	if b.pathStaging(a) != p || !esiste(p) {
		t.Errorf("path_staging %q, file %v: la transazione del passo doveva annullarsi", b.pathStaging(a), esiste(p))
	}
	if got := b.azioni(b.thread, a); !uguali(got, "scarta", "rimozione_errore") {
		t.Errorf("eventi %v", got)
	}
	if !strings.HasPrefix(e.Testo, "Scarto registrato; rimozione dallo staging ancora da completare") || strings.Contains(e.Testo, "eliminato") {
		t.Errorf("testo %q", e.Testo)
	}
	b.avanti(2 * time.Minute)
	if _, err := (&Custode{S: b.s}).Giro(b.ctx); err != nil {
		t.Fatal(err)
	}
	if got := b.rimozione(b.thread, a); got != "eseguita/rimosso" || esiste(p) {
		t.Errorf("dopo il custode: %s, file %v", got, esiste(p))
	}
}

// SC-06: il COMMIT del passo che fallisce dopo os.Remove: il file non c'e', il DB resta in_attesa; il tentativo dopo
// chiude eseguita (gia_assente) senza errore.
func TestSC06CommitCheFallisceDopoLaCancellazione(t *testing.T) {
	b := nuovoBancoFile(t)
	b.s.Agganci = &Agganci{SaltaTentativi: true}
	a, _ := b.pdf("ACME-S06.pdf")
	p := b.pathStaging(a)
	b.deveRiuscire(b.scarta(b.thread, a))
	id := b.rimozioneID(b.thread, a)
	b.s.Agganci = &Agganci{DopoLaCancellazione: func(int64) {
		// la connessione della transazione del passo (l'unica ferma dentro una transazione) si chiude: il COMMIT fallisce
		b.esegui(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity
			WHERE datname = current_database() AND pid <> pg_backend_pid() AND state = 'idle in transaction'`)
	}}
	if _, err := b.s.EseguiRimozione(b.ctx, id, false); err == nil {
		t.Fatal("il COMMIT doveva fallire")
	}
	if esiste(p) {
		t.Error("il file c'e' ancora: os.Remove doveva venire prima del COMMIT")
	}
	if got := b.rimozione(b.thread, a); got != "in_attesa/da_eseguire" {
		t.Errorf("rimozione %s dopo il COMMIT fallito", got)
	}
	b.s.Agganci = nil
	ep, err := b.s.EseguiRimozione(b.ctx, id, false)
	if err != nil || ep.Stato != "eseguita" || ep.Motivo != "gia_assente" {
		t.Errorf("il tentativo dopo: %+v %v", ep, err)
	}
	if b.pathStaging(a) != "" {
		t.Error("path_staging resta su un file che non c'e'")
	}
}

// SC-07: il riavvio con una rimozione in_attesa: il custode del processo nuovo la completa.
func TestSC07RiavvioConUnaRimozioneInAttesa(t *testing.T) {
	b := nuovoBancoFile(t)
	b.s.Agganci = &Agganci{SaltaTentativi: true}
	a, _ := b.pdf("ACME-S07.pdf")
	p := b.pathStaging(a)
	b.deveRiuscire(b.scarta(b.thread, a))
	if got := b.rimozione(b.thread, a); got != "in_attesa/da_eseguire" {
		t.Fatalf("rimozione %s", got)
	}
	dep, err := PreparaDeposito(b.ctx, b.q, b.deposito)
	if err != nil {
		t.Fatal(err)
	}
	s2 := &Servizio{Pool: b.p, Staging: b.staging, Deposito: dep, Log: b.s.Log, EtaMinima: time.Nanosecond}
	e, err := (&Custode{S: s2}).Giro(b.ctx)
	if err != nil || e.Rimozioni != 1 {
		t.Fatalf("custode del processo nuovo: %+v %v", e, err)
	}
	if got := b.rimozione(b.thread, a); got != "eseguita/rimosso" || esiste(p) {
		t.Errorf("rimozione %s, file %v", got, esiste(p))
	}
}

// SC-08: path_staging nello staging vecchio per messaggio: nessuna cancellazione, errore / fuori_cache non ritentabile,
// percorso_trovato, path_staging nullo; il giorno dopo, con il file ancora li', resta errore e S1 chiede la bonifica;
// tolto il file a mano, il controllo chiude eseguita / bonificato. Un percorso fuori dalla radice resta errore; lo
// stesso percorso indicato da un'altra occorrenza attiva da' non_necessaria_condiviso / condiviso.
func TestSC08StagingVecchioPerMessaggio(t *testing.T) {
	b := nuovoBancoFile(t)
	vecchio := filepath.Join(b.staging, "MSG-0001", "ACME-S08.pdf")
	c := contenuto("ACME-S08.pdf", 808, 3_000)
	if err := os.MkdirAll(filepath.Dir(vecchio), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vecchio, c, 0o644); err != nil {
		t.Fatal(err)
	}
	a, _ := b.file(b.mail(b.thread), "ACME-S08.pdf", c, opzFile{fuoriStaging: vecchio})
	e := b.deveRiuscire(b.scarta(b.thread, a))
	if got := b.rimozione(b.thread, a); got != "errore/fuori_cache" {
		t.Fatalf("rimozione %s", got)
	}
	var ritentabile bool
	var trovato string
	b.rigaCon(`SELECT ritentabile, coalesce(percorso_trovato, '') FROM file_rfq_rimozione WHERE allegato_id = $1`, []any{a}, &ritentabile, &trovato)
	if ritentabile || !strings.EqualFold(trovato, vecchio) || b.pathStaging(a) != "" || !esiste(vecchio) {
		t.Errorf("ritentabile %v, percorso_trovato %q, path %q, file %v", ritentabile, trovato, b.pathStaging(a), esiste(vecchio))
	}
	if !strings.Contains(e.Testo, "Serve una bonifica dell'amministratore") {
		t.Errorf("testo %q", e.Testo)
	}
	c1 := &Custode{S: b.s}
	b.avanti(25 * time.Hour)
	if _, err := c1.Giro(b.ctx); err != nil {
		t.Fatal(err)
	}
	if got := b.rimozione(b.thread, a); got != "errore/fuori_cache" {
		t.Errorf("il giorno dopo, con il file ancora li': %s", got)
	}
	if f := fileDi(b.modello(b.thread, a), a); f == nil || f.Rimozione == nil || !strings.Contains(f.Rimozione.Testo, "bonifica") {
		t.Errorf("S1 non chiede la bonifica: %+v", f)
	}
	_, err := b.scarta(b.thread, a)
	rifiutoAtteso(t, err, http.StatusConflict, "FILE_VERSIONE")
	if got := b.rimozione(b.thread, a); got != "errore/fuori_cache" {
		t.Errorf("il reinvio ha toccato la rimozione: %s", got)
	}
	if err := os.Remove(vecchio); err != nil {
		t.Fatal(err)
	}
	b.avanti(50 * time.Hour)
	if e, err := c1.Giro(b.ctx); err != nil || e.Bonificate != 1 {
		t.Fatalf("la bonifica: %+v %v", e, err)
	}
	if got := b.rimozione(b.thread, a); got != "eseguita/bonificato" {
		t.Errorf("rimozione %s", got)
	}
	if got := b.azioni(b.thread, a); !uguali(got, "scarta", "rimozione_errore", "rimozione_eseguita") {
		t.Errorf("eventi %v", got)
	}
	if f := fileDi(b.modello(b.thread, a), a); f == nil || f.Rimozione == nil || !strings.Contains(f.Rimozione.Testo, "l'ha tolta la bonifica") {
		t.Errorf("S1 dopo la bonifica: %+v", f)
	}

	// un percorso fuori dalla radice: errore percorso_non_valido, e la bonifica non guarda fuori dalla radice
	fuori := filepath.Join(t.TempDir(), "ACME-S08b.pdf")
	c2 := contenuto("ACME-S08b.pdf", 809, 3_000)
	if err := os.WriteFile(fuori, c2, 0o644); err != nil {
		t.Fatal(err)
	}
	a2, _ := b.file(b.mail(b.thread), "ACME-S08b.pdf", c2, opzFile{fuoriStaging: fuori})
	b.deveRiuscire(b.scarta(b.thread, a2))
	if got := b.rimozione(b.thread, a2); got != "errore/percorso_non_valido" {
		t.Errorf("rimozione fuori dalla radice: %s", got)
	}
	if err := os.Remove(fuori); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Custode{S: b.s}).Bonifica(b.ctx); err != nil {
		t.Fatal(err)
	}
	if got := b.rimozione(b.thread, a2); got != "errore/percorso_non_valido" {
		t.Errorf("la bonifica ha chiuso un percorso fuori dalla radice: %s", got)
	}

	// lo stesso percorso vecchio indicato da un'altra occorrenza attiva: condiviso, senza errore
	condiviso := filepath.Join(b.staging, "MSG-0002", "ACME-S08c.pdf")
	c3 := contenuto("ACME-S08c.pdf", 810, 3_000)
	if err := os.MkdirAll(filepath.Dir(condiviso), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(condiviso, c3, 0o644); err != nil {
		t.Fatal(err)
	}
	msg := b.mail(b.thread)
	a3, _ := b.file(msg, "ACME-S08c.pdf", c3, opzFile{fuoriStaging: condiviso})
	a4, _ := b.file(msg, "ACME-S08c copia.pdf", c3, opzFile{fuoriStaging: condiviso})
	b.deveRiuscire(b.scarta(b.thread, a3))
	if got := b.rimozione(b.thread, a3); got != "non_necessaria_condiviso/condiviso" {
		t.Errorf("rimozione con il percorso vecchio condiviso: %s", got)
	}
	if b.pathStaging(a4) != condiviso || !esiste(condiviso) {
		t.Error("l'altra occorrenza ha perso il file")
	}
}

// SC-09: le dipendenze che bloccano: documento confermato (D-1), fonte autorizzata (D-3), fonte della bozza aperta e
// preassociazione nella versione salvata (D-6), archivio con una voce attiva non elencata (perimetro): il codice giusto
// e nessuna riga scritta.
func TestSC09DipendenzeCheBloccano(t *testing.T) {
	b := nuovoBancoFile(t)
	casi := []struct {
		nome    string
		prepara func() uuid.UUID
		codice  string
	}{
		{"documento confermato", func() uuid.UUID {
			a, sha := b.pdf("ACME-S09a.pdf")
			doc := b.documentoNAS(b.thread, "ACME-S09A", sha, "scritto")
			b.esegui(`INSERT INTO documento_provenienza (documento_id, allegato_id, ricevuto_il) VALUES ($1, $2, now())`, doc, a)
			return a
		}, "D-1"},
		{"fonte autorizzata", func() uuid.UUID {
			a, sha := b.file(b.mail(b.thread), "ACME-S09B.stp", contenuto("ACME-S09B.stp", 902, 3_000), opzFile{})
			comp := b.componente(b.thread, "ACME-S09B", "finito")
			doc := valore[uuid.UUID](b, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, bytes,
				path_relativo, stato_nas, confermato_da) VALUES ($1, $2, 'cad_3d', 'ACME-S09B', 'ACME-S09B.stp', 'stp', $3, 10, 'CAD\ACME-S09B.stp',
				'scritto', $4) RETURNING documento_id`, b.thread, comp, sha, b.u.UtenteID)
			b.esegui(`INSERT INTO documento_provenienza (documento_id, allegato_id, ricevuto_il) VALUES ($1, $2, now())`, doc, a)
			b.esegui(`UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, comp, doc)
			return a
		}, "D-3"},
		{"fonte della bozza aperta", func() uuid.UUID {
			a, _ := b.file(b.mail(b.thread), "ACME-S09C.stp", contenuto("ACME-S09C.stp", 903, 3_000), opzFile{})
			b.bozzaConFonte(a, nil)
			return a
		}, "D-6"},
		{"preassociazione salvata", func() uuid.UUID {
			a, _ := b.pdf("ACME-S09D.pdf")
			pid := valore[uuid.UUID](b, `SELECT proposta_id FROM documento_proposta WHERE allegato_id = $1`, a)
			b.bozzaConFonte(uuid.Nil, map[string]string{"documenti_preassociati": `[{"proposta_id": "` + pid.String() + `"}]`})
			return a
		}, "D-6"},
	}
	for _, c := range casi {
		a := c.prepara()
		prima := b.foto()
		_, err := b.scarta(b.thread, a)
		rf := rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "DIPENDENZE_ATTIVE")
		if !citaDipendenza(rf, a, c.codice) {
			t.Errorf("%s: il rifiuto non porta %s: %v", c.nome, c.codice, rf.Extra["dipendenze"])
		}
		b.nienteScritto(prima, c.nome)
	}
	// archivio con una voce attiva non elencata: il perimetro visto e' vecchio
	zip, _ := b.archivio(b.thread, "ACME-S09.zip", []string{"E1.pdf"}, opzFile{})
	corpo := b.corpo(GestoScarta, b.thread, zip, "non serve")
	msgZip := valore[uuid.UUID](b, `SELECT messaggio_id FROM allegato WHERE allegato_id = $1`, zip)
	b.file(msgZip, "E2.pdf", contenuto("E2.pdf", 904, 2_000), opzFile{contenitore: zip, pathInterno: "cartella/E2.pdf"})
	prima := b.foto()
	_, err := b.s.Scarta(b.ctx, b.thread, zip, b.u.UtenteID, corpo)
	rf := rifiutoAtteso(t, err, http.StatusConflict, "ANTEPRIMA_SUPERATA")
	if rf.Extra["anteprima"] != "voci_archivio" || rf.Extra["n_voci"] != 2 {
		t.Errorf("perimetro: %v", rf.Extra)
	}
	b.nienteScritto(prima, "perimetro")
}

// SC-10: le risoluzioni: un file associato senza «scollega», l'unica copia senza «conferma_unica_copia» → 422
// RISOLUZIONE_RICHIESTA; con i campi passano, con «sgancia» e «scarta» scritti.
func TestSC10Risoluzioni(t *testing.T) {
	b := nuovoBancoFile(t)
	comp := b.componente(b.thread, "ACME-S10", "sciolto")
	a, _ := b.file(b.mail(b.thread), "ACME-S10.pdf", contenuto("ACME-S10.pdf", 1010, 3_000), opzFile{componente: comp, codice: "ACME-S10"})
	prima := b.foto()
	_, err := b.scarta(b.thread, a)
	rf := rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "RISOLUZIONE_RICHIESTA")
	if cc, _ := rf.Extra["campi"].([]string); !uguali(cc, RisoluzioneScollega) {
		t.Errorf("campi %v", rf.Extra["campi"])
	}
	b.nienteScritto(prima, "senza scollega")
	b.deveRiuscire(b.scarta(b.thread, a, conScollega))
	if got := valore[[]string](b, `SELECT array_agg(e.azione ORDER BY e.evento_id) FROM associazione_evento e JOIN documento_proposta p
		ON p.proposta_id = e.proposta_id WHERE p.allegato_id = $1`, a); !uguali(got, "sgancia", "scarta") {
		t.Errorf("associazione_evento %v", got)
	}
	if got := b.statoProposta(a); got != "scartata/-" {
		t.Errorf("proposta %s", got)
	}
	u, _ := b.file(b.mail(b.thread), "ACME-S10U.pdf", contenuto("ACME-S10U.pdf", 1011, 3_000), opzFile{origine: db.OrigineAllegatoManuale})
	prima = b.foto()
	_, err = b.scarta(b.thread, u)
	rf = rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "RISOLUZIONE_RICHIESTA")
	if !citaDipendenza(rf, u, "D-14") {
		t.Errorf("l'unica copia: %v", rf.Extra["occorrenze"])
	}
	b.nienteScritto(prima, "senza conferma dell'unica copia")
	b.deveRiuscire(b.scarta(b.thread, u, conUnicaCopia))
	if got := b.stato(b.thread, u); got != StatoScartato {
		t.Errorf("stato %q", got)
	}
}

// SC-11: l'impronta delle dipendenze vecchia (nel frattempo e' nata una dipendenza) → 409 ANTEPRIMA_SUPERATA con
// l'elenco nuovo. [T] della corsia file: i blocchi vengono prima dell'impronta, quindi la dipendenza nata qui e' una
// che non blocca (D-2, la copia sul NAS di un documento con lo stesso contenuto); una preassociazione salvata (D-6)
// risponde 422 DIPENDENZE_ATTIVE, come in SC-09.
func TestSC11ImprontaDelleDipendenzeVecchia(t *testing.T) {
	b := nuovoBancoFile(t)
	a, sha := b.pdf("ACME-S11.pdf")
	corpo := b.corpo(GestoScarta, b.thread, a, "non serve")
	b.documentoNAS(b.thread, "ACME-S11", sha, "in_coda")
	nuova := b.modello(b.thread, a).Dettaglio.ImprontaDipendenze
	prima := b.foto()
	_, err := b.s.Scarta(b.ctx, b.thread, a, b.u.UtenteID, corpo)
	rf := rifiutoAtteso(t, err, http.StatusConflict, "ANTEPRIMA_SUPERATA")
	if rf.Extra["anteprima"] != "dipendenze_file" || rf.Extra["impronta_dipendenze"] != nuova || !citaDipendenza(rf, a, "D-2") {
		t.Errorf("409: %v", rf.Extra)
	}
	b.nienteScritto(prima, "impronta vecchia")
	b.deveRiuscire(b.scarta(b.thread, a)) // con l'impronta nuova passa
}

// SC-12: il ricaricamento dello stesso file dopo lo scarto: un'occorrenza nuova, attiva; la vecchia resta scartata con
// l'evento ricaricato_dopo_scarto; nessun «gia' caricato».
func TestSC12RicaricamentoDopoLoScarto(t *testing.T) {
	b := nuovoBancoFile(t)
	c := contenuto("ACME-S12.pdf", 1212, 3_000)
	es, err := b.s.RegistraFile(b.ctx, b.thread, b.u, true, b.arrivato(c, "ACME-S12.pdf"))
	if err != nil || es.Esito != "caricato" {
		t.Fatalf("primo caricamento: %+v %v", es, err)
	}
	a1 := *es.AllegatoID
	b.deveRiuscire(b.scarta(b.thread, a1, conUnicaCopia))
	es2, err := b.s.RegistraFile(b.ctx, b.thread, b.u, true, b.arrivato(c, "ACME-S12.pdf"))
	if err != nil || es2.Esito != "caricato" || !es2.RicaricatoDopoScarto || es2.StatoFile != StatoAttivo {
		t.Fatalf("ricaricamento: %+v %v", es2, err)
	}
	a2 := *es2.AllegatoID
	if a2 == a1 {
		t.Fatal("il ricaricamento ha riusato l'occorrenza scartata")
	}
	if b.stato(b.thread, a1) != StatoScartato || b.stato(b.thread, a2) != "" {
		t.Errorf("stati: vecchia %q, nuova %q", b.stato(b.thread, a1), b.stato(b.thread, a2))
	}
	if n := valore[int](b, `SELECT count(*) FROM file_rfq_evento WHERE allegato_id = $1 AND azione = 'ricaricato_dopo_scarto' AND altro_allegato_id = $2
		AND origine = 'ponte'`, a1, a2); n != 1 {
		t.Errorf("evento ricaricato_dopo_scarto: %d", n)
	}
	if p := b.pathStaging(a2); !staging.ContenutoGiaPresente(p, shaDi(c)) {
		t.Errorf("il file ricaricato non e' nello staging: %q", p)
	}
}

// SC-13: «Riattiva» su uno scartato → 422 FILE_SCARTATO.
func TestSC13RiattivaSuUnoScartato(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-S13.pdf")
	b.deveRiuscire(b.scarta(b.thread, a))
	prima := b.foto()
	_, err := b.riattiva(b.thread, a)
	rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "FILE_SCARTATO")
	b.nienteScritto(prima, "Riattiva su uno scartato")
}

// SC-14: un archivio con il perimetro completo: archivio e voci scartati, una rimozione per occorrenza, i file tolti.
func TestSC14ArchivioConIlPerimetroCompleto(t *testing.T) {
	b := nuovoBancoFile(t)
	zip, voci := b.archivio(b.thread, "ACME-S14.zip", []string{"A.pdf", "B.pdf", "C.stp"}, opzFile{})
	percorsi := []string{b.pathStaging(zip)}
	for _, v := range voci {
		percorsi = append(percorsi, b.pathStaging(v))
	}
	e := b.deveRiuscire(b.scarta(b.thread, zip))
	if len(e.Occorrenze) != 4 {
		t.Errorf("occorrenze del gesto: %d", len(e.Occorrenze))
	}
	for _, id := range append([]uuid.UUID{zip}, voci...) {
		if b.stato(b.thread, id) != StatoScartato || b.rimozione(b.thread, id) != "eseguita/rimosso" {
			t.Errorf("%s: stato %q, rimozione %s", id, b.stato(b.thread, id), b.rimozione(b.thread, id))
		}
	}
	if n := valore[int](b, `SELECT count(*) FROM file_rfq_rimozione`); n != 4 {
		t.Errorf("rimozioni %d, attese 4", n)
	}
	for _, p := range percorsi {
		if esiste(p) {
			t.Errorf("file rimasto: %s", filepath.Base(p))
		}
	}
	// l'evento dell'archivio porta il perimetro (conteggio e impronta), ogni voce ha la sua riga
	var nVoci int
	b.rigaCon(`SELECT (dipendenze ->> 'n_voci')::int FROM file_rfq_evento WHERE allegato_id = $1 AND azione = 'scarta'`, []any{zip}, &nVoci)
	if nVoci != 3 {
		t.Errorf("n_voci nell'evento dell'archivio: %d", nVoci)
	}
}

// SC-15: l'unica copia da Altri file: un caricamento a mano in Altri file con lo staging gia' tolto dalla cache;
// «Scarta» senza conferma → 422 RISOLUZIONE_RICHIESTA, deposito intatto; con la conferma → rimozione e rilascio. (Il
// testo del dialogo «Ho capito: è l'unica copia» e' della pagina.)
func TestSC15UnicaCopiaDaAltriFile(t *testing.T) {
	b := nuovoBancoFile(t)
	u, sha := b.file(b.mail(b.thread), "ACME-S15.pdf", contenuto("ACME-S15.pdf", 1515, 3_000), opzFile{origine: db.OrigineAllegatoManuale})
	b.deveRiuscire(b.altriFile(b.thread, u))
	if giro, err := b.cache(b.s, 40).Giro(b.ctx); err != nil || giro.Rimossi != 1 {
		t.Fatalf("la cache non ha tolto lo staging: %+v %v", giro, err)
	}
	prima := b.foto()
	_, err := b.scarta(b.thread, u)
	rf := rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "RISOLUZIONE_RICHIESTA")
	if !citaDipendenza(rf, u, "D-14") {
		t.Errorf("l'unica copia: %v", rf.Extra["occorrenze"])
	}
	b.nienteScritto(prima, "senza conferma")
	if _, h := b.copiaVerificata(b.thread, u); h != sha {
		t.Fatal("il deposito non e' intatto")
	}
	e := b.deveRiuscire(b.scarta(b.thread, u, conUnicaCopia))
	if got := b.rimozione(b.thread, u); got != "eseguita/gia_assente" {
		t.Errorf("rimozione %s", got)
	}
	if got := b.copie(b.thread, u); !uguali(got, "rilasciata/scartato") {
		t.Errorf("conservazione %v", got)
	}
	if got := b.fileDelDeposito(); len(got) != 0 {
		t.Errorf("il deposito tiene ancora la copia: %v", got)
	}
	if !strings.Contains(e.Testo, "La copia di conservazione è stata eliminata.") {
		t.Errorf("testo %q", e.Testo)
	}
}

// SC-16: il tempo 2 con un caricamento dello stesso contenuto in un'altra RFQ in mezzo (fra il passo 9 e il passo 10):
// il caricamento aspetta il lucchetto, e alla fine il file c'e' ed e' indicato dalla nuova occorrenza; un file gia'
// assente chiude eseguita / gia_assente; una radice con maiuscole diverse non nasconde l'altro allegato.
func TestSC16Tempo2ConUnCaricamentoInMezzo(t *testing.T) {
	b := nuovoBancoFile(t)
	b.s.Agganci = &Agganci{SaltaTentativi: true}
	c := contenuto("ACME-S16.pdf", 1616, 3_000)
	a, sha := b.file(b.mail(b.thread), "ACME-S16.pdf", c, opzFile{})
	p := b.pathStaging(a)
	b.deveRiuscire(b.scarta(b.thread, a))
	id := b.rimozioneID(b.thread, a)
	rfqB := b.rfq("RFQ-B")
	f := b.arrivato(c, "ACME-S16.pdf")
	type esitoCar struct {
		es  EsitoFile
		err error
	}
	fatto := make(chan esitoCar, 1)
	b.s.Agganci = &Agganci{SaltaTentativi: true, PrimaDiCancellare: func(int64) {
		go func() {
			es, err := b.s.RegistraFile(context.Background(), rfqB, b.u, true, f)
			fatto <- esitoCar{es, err}
		}()
		b.attendiLucchetto()
	}}
	ep, err := b.s.EseguiRimozione(b.ctx, id, false)
	if err != nil || ep.Stato != "eseguita" {
		t.Fatalf("tempo 2: %+v %v", ep, err)
	}
	r := <-fatto
	if r.err != nil || r.es.Esito != "caricato" {
		t.Fatalf("il caricamento in mezzo: %+v %v", r.es, r.err)
	}
	if pb := b.pathStaging(*r.es.AllegatoID); !strings.EqualFold(pb, p) || !staging.ContenutoGiaPresente(pb, sha) {
		t.Errorf("la nuova occorrenza indica un file che non c'e': %q", pb)
	}

	// un file gia' assente: eseguita / gia_assente
	a2, _ := b.pdf("ACME-S16b.pdf")
	b.deveRiuscire(b.scarta(b.thread, a2))
	if err := os.Remove(b.pathStaging(a2)); err != nil {
		t.Fatal(err)
	}
	b.s.Agganci = nil
	if ep, err := b.s.EseguiRimozione(b.ctx, b.rimozioneID(b.thread, a2), false); err != nil || ep.Motivo != "gia_assente" {
		t.Errorf("file gia' assente: %+v %v", ep, err)
	}

	// la radice scritta con le maiuscole diverse non nasconde l'altro allegato che indica lo stesso file
	b.s.Staging = strings.ToUpper(b.staging)
	msg := b.mail(b.thread)
	c3 := contenuto("ACME-S16c.pdf", 1617, 3_000)
	a3, _ := b.file(msg, "ACME-S16c.pdf", c3, opzFile{})
	a4, _ := b.file(msg, "ACME-S16c copia.pdf", c3, opzFile{})
	b.deveRiuscire(b.scarta(b.thread, a3))
	if got := b.rimozione(b.thread, a3); got != "non_necessaria_condiviso/condiviso" {
		t.Errorf("con la radice in maiuscolo: %s", got)
	}
	if !esiste(b.pathStaging(a4)) {
		t.Error("il file dell'altro allegato e' stato tolto")
	}
}

// SC-17: il pin transitorio: lo sha del file scartato e' in un job pendente di un'altra RFQ → in_attesa /
// pin_transitorio; finito il job, il ritentativo chiude eseguita / rimosso (senza la regola della cache); lo stesso
// contenuto con un'altra estensione in un'altra RFQ, con la sua proposta aperta, non ferma la rimozione.
func TestSC17PinTransitorio(t *testing.T) {
	b := nuovoBancoFile(t)
	rfqB := b.rfq("RFQ-B")
	a, sha := b.pdf("ACME-S17.pdf")
	p := b.pathStaging(a)
	job := valore[int64](b, `INSERT INTO job (tipo, worker_tipo, payload, stato) VALUES ('analizza_allegato', 'analisi',
		jsonb_build_object('sha256', $1::text, 'thread_id', $2::text), 'pronto') RETURNING job_id`, sha, rfqB.String())
	b.deveRiuscire(b.scarta(b.thread, a))
	if got := b.rimozione(b.thread, a); got != "in_attesa/pin_transitorio" || !esiste(p) {
		t.Fatalf("rimozione %s, file %v", got, esiste(p))
	}
	b.esegui(`UPDATE job SET stato = 'fatto', chiuso_il = now() WHERE job_id = $1`, job)
	b.avanti(2 * time.Minute)
	if ep, err := b.s.EseguiRimozione(b.ctx, b.rimozioneID(b.thread, a), false); err != nil || ep.Stato != "eseguita" || ep.Motivo != "rimosso" {
		t.Errorf("il ritentativo: %+v %v", ep, err)
	}
	if esiste(p) {
		t.Error("il file resta")
	}
	// un'altra estensione e' un altro file: il suo pin non conta
	c := contenuto("ACME-S17.stp", 1718, 3_000)
	a2, _ := b.file(b.mail(b.thread), "ACME-S17.stp", c, opzFile{})
	b.file(b.mail(rfqB), "ACME-S17.step", c, opzFile{})
	b.deveRiuscire(b.scarta(b.thread, a2))
	if got := b.rimozione(b.thread, a2); got != "eseguita/rimosso" {
		t.Errorf("lo stesso contenuto con un'altra estensione ha fermato la rimozione: %s", got)
	}
}

// SC-18: uno zip caricato a mano (origine portale) di 900 file, uno dei quali e' uno zip di 900 voci: 1801 occorrenze,
// tutte con D-14. «Scarta» con il perimetro, l'impronta e la conferma sta sotto i 64 KiB e passa; senza la conferma →
// 422 RISOLUZIONE_RICHIESTA con le occorrenze; con l'elenco cambiato sotto il lucchetto → 409 dipendenze_file; con il
// perimetro cambiato → 409 voci_archivio; un evento per occorrenza, nessuno oltre 256 KiB. «Metti in Altri file» dello
// stesso zip (un altro, con i byte), con voci associate a componenti, passa con scollega.
func TestSC18ArchivioGrandeAnnidato(t *testing.T) {
	b := nuovoBancoFile(t)
	zip := b.archivioGrande(b.thread, false)
	m := b.modello(b.thread, zip)
	if m.Dettaglio == nil || m.Dettaglio.NVoci == nil || *m.Dettaglio.NVoci != 1800 {
		t.Fatalf("perimetro dell'archivio: %+v", m.Dettaglio)
	}
	d14 := 0
	for _, d := range m.Dettaglio.Dipendenze {
		if d.Codice == "D-14" {
			d14++
		}
	}
	if d14 != 1801 {
		t.Errorf("occorrenze con D-14: %d, attese 1801", d14)
	}
	corpo := b.corpo(GestoScarta, b.thread, zip, "pacchetto sbagliato")
	senza := corpo
	senza.ChiaveIdempotenza = chiave()
	_, err := b.s.Scarta(b.ctx, b.thread, zip, b.u.UtenteID, senza)
	rf := rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "RISOLUZIONE_RICHIESTA")
	if l, _ := rf.Extra["occorrenze"].([]Dipendenza); len(l) != 1801 {
		t.Errorf("occorrenze che chiedono la conferma: %d", len(l))
	}
	conferma := corpo
	conferma.ConfermaUnicaCopia = vero()
	if j, _ := json.Marshal(conferma); len(j) >= MaxCorpo {
		t.Fatalf("il corpo pesa %d byte", len(j))
	}
	// una dipendenza in piu' (una nota su una voce) sotto il lucchetto: 409 dipendenze_file
	voce := valore[uuid.UUID](b, `SELECT allegato_id FROM allegato WHERE contenitore_id = $1 ORDER BY indice LIMIT 1`, zip)
	nota := valore[uuid.UUID](b, `INSERT INTO annotazione_pdf (thread_id, allegato_id, pagina, x_norm, y_norm, testo, creata_da)
		VALUES ($1, $2, 1, 0.5, 0.5, 'nota', $3) RETURNING annotazione_id`, b.thread, voce, b.u.UtenteID)
	c1 := conferma
	c1.ChiaveIdempotenza = chiave()
	_, err = b.s.Scarta(b.ctx, b.thread, zip, b.u.UtenteID, c1)
	rf = rifiutoAtteso(t, err, http.StatusConflict, "ANTEPRIMA_SUPERATA")
	if rf.Extra["anteprima"] != "dipendenze_file" {
		t.Errorf("anteprima %v", rf.Extra["anteprima"])
	}
	b.esegui(`DELETE FROM annotazione_pdf WHERE annotazione_id = $1`, nota)
	// il perimetro cambiato (una voce in piu' nello zip interno): 409 voci_archivio
	interno := valore[uuid.UUID](b, `SELECT allegato_id FROM allegato WHERE contenitore_id = $1 AND lower(estensione) = 'zip'`, zip)
	msg := valore[uuid.UUID](b, `SELECT messaggio_id FROM allegato WHERE allegato_id = $1`, zip)
	extra := valore[uuid.UUID](b, `INSERT INTO allegato (messaggio_id, contenitore_id, indice, nome_file, estensione, natura, origine, bytes, sha256,
		stato, ricevuto_il, caricato_da) VALUES ($1, $2, 9999, 'X.pdf', 'pdf', 'file', 'portale', 10, $3, 'analizzato', now(), $4) RETURNING allegato_id`,
		msg, interno, shaDi([]byte("voce in piu'")), b.u.UtenteID)
	c2 := conferma
	c2.ChiaveIdempotenza = chiave()
	_, err = b.s.Scarta(b.ctx, b.thread, zip, b.u.UtenteID, c2)
	rf = rifiutoAtteso(t, err, http.StatusConflict, "ANTEPRIMA_SUPERATA")
	if rf.Extra["anteprima"] != "voci_archivio" {
		t.Errorf("anteprima %v", rf.Extra["anteprima"])
	}
	b.esegui(`DELETE FROM allegato WHERE allegato_id = $1`, extra)
	// con il perimetro, l'impronta e la conferma: passa
	conferma.ChiaveIdempotenza = chiave()
	inizio := time.Now()
	e := b.deveRiuscire(b.s.Scarta(b.ctx, b.thread, zip, b.u.UtenteID, conferma))
	t.Logf("SC-18: «Scarta» di 1801 occorrenze in %s", time.Since(inizio).Round(time.Millisecond))
	if len(e.Occorrenze) != 1801 {
		t.Errorf("occorrenze %d", len(e.Occorrenze))
	}
	var nEventi, massimo int
	b.rigaCon(`SELECT count(*), coalesce(max(greatest(pg_column_size(dipendenze), pg_column_size(fotografia))), 0)
		FROM file_rfq_evento WHERE azione = 'scarta'`, nil, &nEventi, &massimo)
	if nEventi != 1801 || massimo > 256<<10 {
		t.Errorf("eventi %d, il piu' grande %d byte", nEventi, massimo)
	}

	// «Metti in Altri file» di uno zip uguale con i byte presenti e voci associate a componenti: passa con scollega
	altro := b.archivioGrande(b.rfq("RFQ-C"), true)
	thC := valore[uuid.UUID](b, `SELECT m.thread_id FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE a.allegato_id = $1`, altro)
	_, err = b.altriFile(thC, altro)
	rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "RISOLUZIONE_RICHIESTA")
	inizio = time.Now()
	e = b.deveRiuscire(b.altriFile(thC, altro, conScollega))
	t.Logf("SC-18: «Metti in Altri file» di 1801 occorrenze in %s", time.Since(inizio).Round(time.Millisecond))
	if n := valore[int](b, `SELECT count(*) FROM file_rfq_conservazione WHERE thread_id = $1 AND stato = 'copiata'`, thC); n != 1801 {
		t.Errorf("copie verificate %d", n)
	}
	if n := valore[int](b, `SELECT count(*) FROM documento_proposta WHERE thread_id = $1 AND componente_id IS NOT NULL`, thC); n != 0 {
		t.Errorf("%d proposte ancora associate", n)
	}
}

// archivioGrande: lo zip di SC-18 caricato a mano: 900 voci dirette (la 900a e' uno zip) e 900 voci dello zip interno.
// conByte: i byte di ogni occorrenza nello staging, e dieci voci associate a un componente (per «Altri file»).
func (b *bancoFile) archivioGrande(thread uuid.UUID, conByte bool) uuid.UUID {
	b.t.Helper()
	msg := b.mail(thread)
	b.n++
	zip, _ := b.file(msg, fmt.Sprintf("PORTALE-%d.zip", b.n), contenuto("x.zip", int64(18000+b.n), 4_000), opzFile{origine: db.OrigineAllegatoPortale})
	seme := fmt.Sprintf("sc18-%d", b.n)
	inserisci := func(contenitore uuid.UUID, livello string, n int, conZip bool) {
		b.esegui(`INSERT INTO allegato (messaggio_id, contenitore_id, indice, nome_file, path_interno, estensione, natura, origine, bytes,
			sha256, stato, ricevuto_il, caricato_da)
			SELECT $1, $2, g, CASE WHEN $6 AND g = $4 THEN 'INTERNO.zip' ELSE $3 || g || '.pdf' END, $3 || g,
			       CASE WHEN $6 AND g = $4 THEN 'zip' ELSE 'pdf' END, 'file', 'portale', 100,
			       encode(sha256(convert_to($5 || $3 || g, 'UTF8')), 'hex'), 'analizzato', now(), $7
			  FROM generate_series(1, $4) g`, msg, contenitore, livello, n, seme, conZip, b.u.UtenteID)
	}
	inserisci(zip, "V", 900, true)
	interno := valore[uuid.UUID](b, `SELECT allegato_id FROM allegato WHERE contenitore_id = $1 AND indice = 900`, zip)
	inserisci(interno, "W", 900, false)
	if conByte {
		comp := b.componente(thread, fmt.Sprintf("ACME-SC18-%d", b.n), "sciolto")
		righe, err := b.p.Query(b.ctx, `WITH RECURSIVE giu AS (SELECT allegato_id FROM allegato WHERE contenitore_id = $1
			UNION ALL SELECT a.allegato_id FROM allegato a JOIN giu g ON a.contenitore_id = g.allegato_id)
			SELECT a.allegato_id, a.nome_file, a.path_interno FROM allegato a JOIN giu g ON g.allegato_id = a.allegato_id`, zip)
		if err != nil {
			b.t.Fatal(err)
		}
		type voce struct {
			id         uuid.UUID
			nome, path string
		}
		var voci []voce
		for righe.Next() {
			var v voce
			if err := righe.Scan(&v.id, &v.nome, &v.path); err != nil {
				b.t.Fatal(err)
			}
			voci = append(voci, v)
		}
		righe.Close()
		for i, v := range voci {
			c := []byte("%PDF-1.4 " + seme + v.path)
			h := shaDi(c)
			p := b.percorsoContenuto(h, v.nome)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				b.t.Fatal(err)
			}
			if err := os.WriteFile(p, c, 0o644); err != nil {
				b.t.Fatal(err)
			}
			b.esegui(`UPDATE allegato SET sha256 = $2, bytes = $3, path_staging = $4 WHERE allegato_id = $1`, v.id, h, len(c), p)
			if i < 10 {
				b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte)
					VALUES ($1, $2, 'disegno_2d', $3, $4, 60, 'estensione')`, v.id, thread, fmt.Sprintf("ACME-SC18-%d", b.n), comp)
			}
		}
	}
	return zip
}

// SC-19: «Scarta» su uno zip la cui voce STEP e' la fonte della bozza → 422 DIPENDENZE_ATTIVE con l'allegato_id della
// voce; una voce con D-1 o D-3 blocca allo stesso modo; una voce associata senza «scollega» → RISOLUZIONE_RICHIESTA.
func TestSC19ZipConUnaVoceCheBlocca(t *testing.T) {
	b := nuovoBancoFile(t)
	zip, voci := b.archivio(b.thread, "ACME-S19.zip", []string{"P1.stp", "P1.pdf"}, opzFile{})
	bozza := b.bozzaConFonte(voci[0], nil)
	prima := b.foto()
	_, err := b.scarta(b.thread, zip)
	rf := rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "DIPENDENZE_ATTIVE")
	if !citaDipendenza(rf, voci[0], "D-6") {
		t.Errorf("la voce fonte: %v", rf.Extra["dipendenze"])
	}
	b.nienteScritto(prima, "voce fonte della bozza")
	// la bozza chiusa: ora la voce PDF confermata (D-1)
	b.esegui(`UPDATE distinta_bozza SET stato = 'abbandonata', chiusa_da = $2, motivo_chiusura = 'prova' WHERE bozza_id = $1`, bozza, b.u.UtenteID)
	b.esegui(`UPDATE documento_proposta SET stato = 'confermata', deciso_da = $2, deciso_il = now() WHERE allegato_id = $1`, voci[1], b.u.UtenteID)
	prima = b.foto()
	_, err = b.scarta(b.thread, zip)
	rf = rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "DIPENDENZE_ATTIVE")
	if !citaDipendenza(rf, voci[1], "D-1") {
		t.Errorf("la voce confermata: %v", rf.Extra["dipendenze"])
	}
	b.nienteScritto(prima, "voce confermata")
	// la voce associata senza «scollega»
	comp := b.componente(b.thread, "ACME-S19", "sciolto")
	b.esegui(`UPDATE documento_proposta SET stato = 'aperta', deciso_da = NULL, deciso_il = NULL, componente_id = $2, codice = 'ACME-S19'
		WHERE allegato_id = $1`, voci[1], comp)
	prima = b.foto()
	_, err = b.scarta(b.thread, zip)
	rf = rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "RISOLUZIONE_RICHIESTA")
	if !citaDipendenza(rf, voci[1], "D-7") {
		t.Errorf("la voce associata: %v", rf.Extra["occorrenze"])
	}
	b.nienteScritto(prima, "voce associata")
	b.deveRiuscire(b.scarta(b.thread, zip, conScollega))
}

// SC-20 e CO-10: «Scarta» da attivo mentre un «Metti in Altri file» di un'altra scheda e' in copia (fra la copia e la
// fase C): la riga in_copia passa a da_rilasciare, la fase C risponde 409 tentativo_chiuso, e il file copiato si toglie
// con il rilascio.
func TestSC20ScartaDuranteLaCopia(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-S20.pdf")
	var es *EsitoGesto
	var errScarta error
	b.s.Agganci = &Agganci{DopoLaCopia: func(uuid.UUID) {
		es, errScarta = b.scarta(b.thread, a)
	}}
	_, err := b.altriFile(b.thread, a)
	rf := rifiutoAtteso(t, err, http.StatusConflict, "COPIA_IN_CORSO")
	if rf.Extra["motivo"] != "tentativo_chiuso" {
		t.Errorf("motivo %v", rf.Extra["motivo"])
	}
	if errScarta != nil || es == nil || es.Esito != "scartato" {
		t.Fatalf("lo «Scarta» dell'altra scheda: %+v %v", es, errScarta)
	}
	if got := b.stato(b.thread, a); got != StatoScartato {
		t.Errorf("stato %q", got)
	}
	if got := b.copie(b.thread, a); !uguali(got, "rilasciata/scartato") {
		t.Errorf("conservazione %v", got)
	}
	if got := b.fileDelDeposito(); len(got) != 0 {
		t.Errorf("il file copiato e' rimasto nel deposito: %v", got)
	}
}

// SC-21: un PDF prova di una decisione «Adotta come codice» (identita) nella bozza aperta: «Scarta» e «Metti in Altri
// file» → 422 DIPENDENZE_ATTIVE; tolta la decisione e salvata la bozza, passano.
func TestSC21ProvaDiUnaDecisioneIdentita(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-S21.pdf")
	bozza := b.bozzaConFonte(uuid.Nil, map[string]string{"decisioni": `[{"tipo": "identita", "chiave_lettura": "` + a.String() + `/cartiglio"}]`})
	for _, gesto := range []func() (*EsitoGesto, error){
		func() (*EsitoGesto, error) { return b.scarta(b.thread, a) },
		func() (*EsitoGesto, error) { return b.altriFile(b.thread, a) },
	} {
		prima := b.foto()
		_, err := gesto()
		rf := rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "DIPENDENZE_ATTIVE")
		if !citaDipendenza(rf, a, "D-6") {
			t.Errorf("la prova dell'identita': %v", rf.Extra["dipendenze"])
		}
		b.nienteScritto(prima, "prova dell'identita'")
	}
	tx, err := b.p.Begin(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	b.versioneBozza(tx, bozza, 2, uuid.Nil, map[string]string{"decisioni": `[]`})
	if err := tx.Commit(b.ctx); err != nil {
		t.Fatal(err)
	}
	b.deveRiuscire(b.altriFile(b.thread, a))
	b.deveRiuscire(b.scarta(b.thread, a))
}
