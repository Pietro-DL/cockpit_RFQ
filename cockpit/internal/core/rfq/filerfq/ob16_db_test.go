//go:build integrazione

package filerfq

// L4 — V1, direttiva 35, blocco C-6a (corsia file): DEC-B PF-27, prova OB-16. Un file ricaricato (E1′) uguale a un
// doppione presente che non ha piu' nessuna sorgente dei byte che S5 serve: i byte arrivati diventano i suoi
// (byte_ripristinati) e S5 lo serve; con una sorgente presente niente cambia. DB di prova, cartelle temporanee, nomi
// sintetici (ACME).

import (
	"os"
	"testing"

	"promatec/cockpit/internal/platform/storage/staging"
)

func TestOB16ByteRipristinatiAlRicaricamento(t *testing.T) {
	t.Run("nessuna sorgente: byte adottati", func(t *testing.T) {
		b := nuovoBancoFile(t)
		c := contenuto("ACME-OB16.pdf", 1616, 2_000)
		es, err := b.s.RegistraFile(b.ctx, b.thread, b.u, false, b.arrivato(c, "ACME-OB16.pdf"))
		if err != nil || es.Esito != "caricato" {
			t.Fatalf("caricamento: %+v %v", es, err)
		}
		a := *es.AllegatoID
		p := b.pathStaging(a)
		if err := os.Remove(p); err != nil { // la cache l'ha tolto: nessuna sorgente resta
			t.Fatal(err)
		}
		es2, err := b.s.RegistraFile(b.ctx, b.thread, b.u, false, b.arrivato(c, "ACME-OB16.pdf"))
		if err != nil || es2.Esito != "gia_presente" || es2.AllegatoID == nil || *es2.AllegatoID != a || !es2.ByteRipristinati {
			t.Fatalf("ricaricamento: %+v %v", es2, err)
		}
		if q := b.pathStaging(a); !staging.ContenutoGiaPresente(q, shaDi(c)) {
			t.Errorf("i byte non sono tornati al doppione: %q", q)
		}
		if f := fileDi(b.modello(b.thread, a), a); f == nil || f.Byte.Staging != "presente" || !f.Azioni.Contenuto.Disponibile {
			t.Errorf("S1 dopo i byte ripristinati (S5 deve servirlo): %+v", f)
		}
	})
	t.Run("una sorgente presente: niente cambia", func(t *testing.T) {
		b := nuovoBancoFile(t)
		c := contenuto("ACME-OB16b.pdf", 1617, 2_000)
		es, err := b.s.RegistraFile(b.ctx, b.thread, b.u, false, b.arrivato(c, "ACME-OB16b.pdf"))
		if err != nil || es.Esito != "caricato" {
			t.Fatalf("caricamento: %+v %v", es, err)
		}
		prima := b.pathStaging(*es.AllegatoID)
		es2, err := b.s.RegistraFile(b.ctx, b.thread, b.u, false, b.arrivato(c, "ACME-OB16b.pdf"))
		if err != nil || es2.Esito != "gia_presente" || es2.ByteRipristinati {
			t.Fatalf("ricaricamento: %+v %v", es2, err)
		}
		if dopo := b.pathStaging(*es.AllegatoID); dopo != prima {
			t.Errorf("path_staging cambiato: %q → %q", prima, dopo)
		}
	})
}
