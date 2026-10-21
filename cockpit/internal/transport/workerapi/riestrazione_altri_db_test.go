//go:build integrazione

package workerapi

// L4 — V1, direttiva 35, blocco C-6a (corsia file): la riestrazione di un archivio in Altri file (U2 §3.10, U2-P45;
// G-7 e G-12 cambiati). Per un contenitore in Altri file, non scartato, la riestrazione rimette i byte delle voci
// attive che ci sono gia', senza scrivere voci nuove, leggendo il contenitore dallo staging o dalla sua copia nella
// cartella ALTRO (il conservatore delle prove: una cartella temporanea). Per un contenitore scartato resta vietata.

import (
	"os"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/filerfq"
	"promatec/cockpit/internal/platform/storage/staging"
)

func TestU2P45RiestrazioneDiUnArchivioInAltriFile(t *testing.T) {
	b := preparaBancoCiclo(t)
	b.s.ArchiviInAltriFile = b.svc
	th := b.rfq("RFQ-A")
	z := zipOrdinato(t, [2]string{"V1.pdf", "%PDF-1.4 prima voce"}, [2]string{"V2.pdf", "%PDF-1.4 seconda voce"})
	arch := b.sceso(th, "ACME-P45.zip", z)
	if _, err := b.s.EstraiArchivio(b.ctx, arch.AllegatoID, uuid.New()); err != nil {
		t.Fatal(err)
	}
	var v1 uuid.UUID
	b.riga(`SELECT allegato_id FROM allegato WHERE contenitore_id = $1 AND indice = 1`, &v1, arch.AllegatoID)
	b.esegui(`UPDATE job SET stato = 'fatto', chiuso_il = now() WHERE tipo = 'analizza_allegato' AND stato IN ('pronto', 'in_corso')`)
	// il solo ZIP in Altri file (le voci restano attive)
	solo := filerfq.PerimetroContenitore
	b.gesto(b.svc.MettiInAltriFile(b.ctx, th, arch.AllegatoID, b.utente.UtenteID, b.corpo(filerfq.GestoAltriFile, th, arch.AllegatoID, "",
		func(c *filerfq.CorpoGesto) { c.Perimetro, c.NVoci, c.ImprontaVoci = &solo, nil, nil })))
	// la cache ha tolto dallo staging lo ZIP e la prima voce
	a1, err := b.q.GetAllegato(b.ctx, v1)
	if err != nil {
		t.Fatal(err)
	}
	pV1 := a1.PathStaging.String
	for _, p := range []string{arch.PathStaging.String, pV1} {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	var prima int
	b.riga(`SELECT count(*) FROM allegato WHERE contenitore_id = $1`, &prima, arch.AllegatoID)
	if _, err := b.s.EstraiArchivio(b.ctx, arch.AllegatoID, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if !staging.ContenutoGiaPresente(pV1, a1.Sha256.String) {
		t.Error("la riestrazione dalla copia in ALTRO non ha rimesso i byte della voce attiva")
	}
	var dopo int
	b.riga(`SELECT count(*) FROM allegato WHERE contenitore_id = $1`, &dopo, arch.AllegatoID)
	if dopo != prima {
		t.Errorf("voci %d → %d: la riestrazione di un archivio in Altri file non scrive voci nuove", prima, dopo)
	}
	var stato string
	b.riga(`SELECT coalesce((SELECT stato FROM file_rfq WHERE thread_id = $1 AND allegato_id = $2), 'attivo')`, &stato, th, arch.AllegatoID)
	if stato != filerfq.StatoAltriFile {
		t.Errorf("lo ZIP e' %q dopo la riestrazione", stato)
	}
	// uno ZIP scartato: niente
	b.esegui(`UPDATE job SET stato = 'fatto', chiuso_il = now() WHERE tipo = 'analizza_allegato' AND stato IN ('pronto', 'in_corso')`)
	z2 := zipOrdinato(t, [2]string{"W1.pdf", "%PDF-1.4 voce scartata"})
	arch2 := b.sceso(th, "ACME-P45b.zip", z2)
	b.scarta(th, arch2.AllegatoID)
	n, err := b.s.EstraiArchivio(b.ctx, arch2.AllegatoID, uuid.New())
	var voci int
	b.riga(`SELECT count(*) FROM allegato WHERE contenitore_id = $1`, &voci, arch2.AllegatoID)
	if err != nil || n != 0 || voci != 0 {
		t.Errorf("uno ZIP scartato si e' estratto: %d voci, %d righe, %v", n, voci, err)
	}
}
