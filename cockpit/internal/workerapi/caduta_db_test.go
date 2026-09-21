//go:build integrazione

// L4 — 7C.1, P1: una connessione che cade DOPO l'upload o DOPO il result non fa danni.
//
// Al banco a due macchine del 20/09/2026 una connessione TLS e' caduta (RemoteDisconnected). Il
// worker, che ora ripete il result subito dopo un buco di rete, puo' presentare due volte lo
// stesso upload e due volte lo stesso result: il server deve restare con UN contenuto, UN job
// fatto, UN tentativo, e rispondere al secondo result con 409 — che il worker tace, come per un
// lease perso — senza toccare cio' che il primo ha scritto.
package workerapi

import (
	"bytes"
	"strings"
	"testing"

	"promatec/cockpit/internal/api"
	"promatec/cockpit/internal/jobs"
	"promatec/cockpit/internal/platform/db"
)

func TestUnaCadutaDopoLUploadOIlResultNonDuplicaNiente(t *testing.T) {
	b := preparaBanco(t, 0)
	tent := b.claim("outlook@PC-A")
	contenuto, sha := contenutoCasuale(8192, 3)

	// upload, «caduta», stesso upload ripetuto tale e quale dallo stesso tentativo
	for i := 0; i < 2; i++ {
		resp := b.put(tent, b.allegato.AllegatoID, bytes.NewReader(contenuto), int64(len(contenuto)))
		if s := stato(t, resp, 204); s != "" {
			t.Fatalf("upload %d: %s", i+1, s)
		}
	}
	// un solo .parte per questo tentativo, non due
	parti := 0
	for _, f := range b.fileDiStaging() {
		if strings.Contains(f, ".parte.") {
			parti++
		}
	}
	if parti != 1 {
		t.Fatalf("file di tentativo dopo due upload uguali: %d, atteso 1 (%v)", parti, b.fileDiStaging())
	}

	// result, «caduta», stesso result ripetuto
	r := api.RisultatoStage{AllegatoID: b.allegato.AllegatoID, Sha256: sha, Bytes: int64(len(contenuto)),
		RisultatoElemento: api.RisultatoElemento{EntryID: "ENTRY-disegno.pdf", Cartella: "Posta in arrivo"}}
	primo := b.result(tent, r)
	primo.Body.Close()
	if primo.StatusCode != 204 {
		t.Fatalf("primo result: %d", primo.StatusCode)
	}
	secondo := b.result(tent, r)
	secondo.Body.Close()
	if secondo.StatusCode != 409 {
		t.Fatalf("secondo result (ripetuto dopo una caduta): %d, atteso 409", secondo.StatusCode)
	}

	j := b.jobOra()
	if j.Stato != db.StatoJobFatto || j.Tentativi != 1 {
		t.Errorf("job: stato=%s tentativi=%d, atteso fatto al primo tentativo", j.Stato, j.Tentativi)
	}
	a := b.allegatoOra()
	if a.Sha256.String != sha || !esiste(a.PathStaging.String) || hashDi(t, a.PathStaging.String) != sha {
		t.Errorf("il contenuto non e' quello consegnato: %+v", a)
	}
	contenuti := 0
	for _, f := range b.fileDiStaging() {
		if strings.HasPrefix(f, jobs.CartellaContenuti) {
			contenuti++
		}
		if strings.Contains(f, ".parte.") {
			t.Errorf("e' rimasto un file di tentativo: %s", f)
		}
	}
	if contenuti != 1 {
		t.Errorf("contenuti in staging: %d, atteso 1 (%v)", contenuti, b.fileDiStaging())
	}
	// una sola proposta per l'allegato, e una sola analisi accodata
	if n := conta(t, b, `SELECT count(*) FROM documento_proposta WHERE allegato_id = $1`, b.allegato.AllegatoID); n != 1 {
		t.Errorf("proposte per l'allegato: %d", n)
	}
	if n := conta(t, b, `SELECT count(*) FROM job WHERE tipo = 'analizza_allegato'`); n != 1 {
		t.Errorf("job di analisi accodati: %d, atteso 1", n)
	}
}

func conta(t *testing.T, b *banco, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := b.pool.QueryRow(b.ctx, sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
