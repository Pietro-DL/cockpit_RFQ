//go:build integrazione

package filerfq

// L4 — V1, direttiva 35, blocco C-6a (corsia file): i parziali nella cartella ALTRO della RFQ (U2 §3.6 regola 6, §3.9;
// prova AF-33 di PM23). Nella stessa cartella scrivono i lavori di copia dei documenti (il token e' il lease del job) e
// le copie degli Altri file (il token della riga): chi pulisce, i documenti come il custode, tiene vivi i token di tutti
// e due. Cartelle temporanee, nomi sintetici (ACME).

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/storage/nas"
)

func TestAF33ParzialiConITokenUniti(t *testing.T) {
	b := nuovoBancoFile(t)
	a, sha := b.pdf("ACME-3300.pdf")
	cartella := b.cartellaRFQ(b.thread)
	// una copia degli Altri file in corso, con il suo token (come la lascia la fase A)
	vivo := uuid.New()
	b.esegui(`INSERT INTO file_rfq_conservazione (thread_id, allegato_id, sha256, percorso_relativo, radice_id, cartella_thread, token)
		VALUES ($1, $2, $3, 'ALTRO\ACME-3300.pdf', $4, $5, $6)`, b.thread, a, sha, b.cons.RadiceID(), cartella, vivo)
	altro := b.cons.Percorso(cartella, "ALTRO")
	if err := os.MkdirAll(altro, 0o755); err != nil {
		t.Fatal(err)
	}
	vecchio := time.Now().Add(-48 * time.Hour)
	parte := func(nome string, tok uuid.UUID) string {
		p := filepath.Join(altro, nome+".parte."+tok.String())
		if err := os.WriteFile(p, []byte("parziale"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, vecchio, vecchio); err != nil {
			t.Fatal(err)
		}
		return p
	}
	delVivo := parte("ACME-3300.pdf", vivo)
	morto1 := parte("ACME-3301.pdf", uuid.New())
	// la pulizia dei documenti nella cartella ALTRO: tiene il parziale della copia in corso
	scr := &nas.Scrittore{Radice: b.deposito}
	n, err := documenti.PulisciPartiScadute(b.ctx, b.q, scr, cartella+`\ALTRO`)
	if err != nil || n != 1 {
		t.Fatalf("pulizia dei documenti: %d %v", n, err)
	}
	if !esiste(delVivo) || esiste(morto1) {
		t.Errorf("dopo la pulizia dei documenti: vivo %v, morto %v", esiste(delVivo), esiste(morto1))
	}
	// il custode del ciclo dei file: lo stesso
	morto2 := parte("ACME-3302.pdf", uuid.New())
	if tolti := (&Custode{S: b.s}).pulisciParti(b.ctx, b.q, time.Now()); tolti != 1 {
		t.Errorf("il custode ha tolto %d parziali, atteso 1", tolti)
	}
	if !esiste(delVivo) || esiste(morto2) {
		t.Errorf("dopo il custode: vivo %v, morto %v", esiste(delVivo), esiste(morto2))
	}
	// finita la copia (la riga non e' piu' in_copia) il suo parziale vecchio si toglie
	b.esegui(`UPDATE file_rfq_conservazione SET stato = 'errore_copia', causa = 'rifiutata' WHERE token = $1`, vivo)
	if tolti := (&Custode{S: b.s}).pulisciParti(b.ctx, b.q, time.Now()); tolti != 1 || esiste(delVivo) {
		t.Errorf("il parziale di una copia finita: tolti %d, ancora presente %v", tolti, esiste(delVivo))
	}
}
