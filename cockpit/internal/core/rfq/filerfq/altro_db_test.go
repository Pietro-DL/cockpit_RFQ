//go:build integrazione

package filerfq

// L4 — V1, direttiva 35, blocco C-6a (corsia file): la cartella ALTRO della RFQ (U2 §3.2, §3.6). Le copie degli Altri
// file e i documenti confermati di tipo «altro» stanno nella stessa cartella: si riservano i nomi sotto lo stesso
// lucchetto e con la stessa query (PercorsoOccupato), e due righe non nominano mai lo stesso file. Prove AF-30…AF-32 di
// PM23. DB di prova, cartelle temporanee, nomi sintetici (ACME).

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/db"
)

// documentoAltro: un documento confermato di tipo «altro» della RFQ, scritto sul NAS al percorso dato (relativo alla
// cartella della RFQ, come path_relativo).
func (b *bancoFile) documentoAltro(percorso, sha string) {
	b.t.Helper()
	b.esegui(`INSERT INTO documento (thread_id, tipo, codice, nome_file, estensione, sha256, bytes, path_relativo, stato_nas, confermato_da)
		VALUES ($1, 'altro', NULL, $2, 'pdf', $3, 10, $2, 'scritto', $4)`, b.thread, percorso, sha, b.u.UtenteID)
}

// AF-30: un documento confermato di tipo «altro» nomina gia' ALTRO\<nome>: la copia degli Altri file con lo stesso
// nome riceve il progressivo, anche con un'altra combinazione di maiuscole (la condivisione e' Windows); il file del
// documento non si tocca.
func TestAF30NomeDiUnDocumentoNellaCartellaAltro(t *testing.T) {
	b := nuovoBancoFile(t)
	a, sha := b.pdf("ACME-3000.pdf")
	b.documentoAltro(`ALTRO\acme-3000.PDF`, "0000000000000000000000000000000000000000000000000000000000000000")
	b.deveRiuscire(b.altriFile(b.thread, a))
	if got := valore[string](b, `SELECT percorso_relativo FROM file_rfq_conservazione WHERE allegato_id = $1 AND stato = 'copiata'`, a); got != `ALTRO\ACME-3000_2.pdf` {
		t.Errorf("nome riservato %q, atteso ALTRO\\ACME-3000_2.pdf", got)
	}
	if _, h := b.copiaVerificata(b.thread, a); h != sha {
		t.Errorf("la copia ha lo sha %s", h)
	}
}

// AF-31: il contrario: una copia viva degli Altri file nomina ALTRO\<nome>; un documento che sceglie un nome nella
// stessa cartella (documenti.ScegliPercorso, sotto lo stesso lucchetto) riceve il progressivo. Lo stesso per una copia
// trattenuta (viva per il nome); una copia rilasciata non tiene piu' il nome.
func TestAF31NomeDiUnaCopiaPerIDocumenti(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-3100.pdf")
	b.deveRiuscire(b.altriFile(b.thread, a))
	scegli := func(nome string) string {
		t.Helper()
		var p string
		err := b.s.inTx(context.Background(), func(q *db.Queries) error {
			var err error
			p, err = documenti.ScegliPercorso(context.Background(), q, b.thread, "ALTRO", nome)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	if got := scegli("ACME-3100.pdf"); got != `ALTRO\ACME-3100_2.pdf` {
		t.Errorf("con la copia copiata il documento ha scelto %q", got)
	}
	b.deveRiuscire(b.riattiva(b.thread, a))
	if got := b.copie(b.thread, a); !uguali(got, "trattenuta/riattivato") {
		t.Fatalf("conservazione %v", got)
	}
	if got := scegli("ACME-3100.pdf"); got != `ALTRO\ACME-3100_2.pdf` {
		t.Errorf("con la copia trattenuta il documento ha scelto %q", got)
	}
	// la copia trattenuta torna in Altri file (eredita) e poi si scarta: il file e la riga viva non ci sono piu'
	b.deveRiuscire(b.altriFile(b.thread, a))
	b.deveRiuscire(b.scarta(b.thread, a, conUnicaCopia))
	if got := scegli("ACME-3100.pdf"); got != `ALTRO\ACME-3100.pdf` {
		t.Errorf("dopo lo scarto il documento ha scelto %q", got)
	}
}

// AF-32: due file con lo stesso nome nello stesso gesto (le voci di un archivio) e un file messo a mano da una persona
// nella cartella ALTRO: ogni copia ha il suo nome, nessuna sovrascrive l'altra ne' il file della persona.
func TestAF32NomiNelloStessoGestoEFileDiUnaPersona(t *testing.T) {
	b := nuovoBancoFile(t)
	rfq := b.cons.Percorso(b.cartellaRFQ(b.thread), "ALTRO")
	if err := os.MkdirAll(rfq, 0o755); err != nil {
		t.Fatal(err)
	}
	aMano := filepath.Join(rfq, "V.pdf")
	if err := os.WriteFile(aMano, []byte("messo a mano"), 0o644); err != nil {
		t.Fatal(err)
	}
	msg := b.mail(b.thread)
	zip, _ := b.file(msg, "ACME-32.zip", contenuto("ACME-32.zip", 3200, 2_000), opzFile{})
	v1, _ := b.file(msg, "V.pdf", contenuto("V.pdf", 3201, 3_000), opzFile{contenitore: zip, pathInterno: "A/V.pdf"})
	v2, _ := b.file(msg, "V.pdf", contenuto("V.pdf", 3202, 3_000), opzFile{contenitore: zip, pathInterno: "B/V.pdf"})
	voci := []uuid.UUID{v1, v2}
	// S1: i due gesti dell'archivio, con il perimetro (U2 §3.10)
	if d := b.modello(b.thread, zip).Dettaglio; d == nil || len(d.AzioniArchivio) != 2 ||
		d.AzioniArchivio[0] != (AzioneArchivio{Perimetro: PerimetroContenitore, Disponibile: true}) ||
		d.AzioniArchivio[1] != (AzioneArchivio{Perimetro: PerimetroContenitoreEVoci, Disponibile: true, NVoci: 2}) {
		t.Errorf("azioni dell'archivio in S1: %+v", d)
	}
	b.deveRiuscire(b.altriFile(b.thread, zip, zipEContenuti))
	nomi := valore[[]string](b, `SELECT array_agg(percorso_relativo ORDER BY percorso_relativo) FROM file_rfq_conservazione
		WHERE allegato_id = ANY($1) AND stato = 'copiata'`, voci)
	if !uguali(nomi, `ALTRO\V_2.pdf`, `ALTRO\V_3.pdf`) {
		t.Errorf("nomi delle voci %v", nomi)
	}
	if c, err := os.ReadFile(aMano); err != nil || string(c) != "messo a mano" {
		t.Errorf("il file messo a mano e' stato toccato: %q %v", c, err)
	}
}
