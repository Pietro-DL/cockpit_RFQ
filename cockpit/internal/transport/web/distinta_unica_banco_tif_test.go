package web

// Le TIF del banco finto della Distinta unica (richiesta del frontend del 22/10, corsia TIF: le anteprime dei 2D TIFF).
// Quattro TIF sintetiche di testdata/distinta_unica_tif (genera.py, Pillow; disegni inventati): una CCITT G4 a 1 bit, una
// LZW a 8 bit, una Deflate (che la pagina dice non supportata) e una rotta. aggiungiTif le mette fra i file della RFQ del
// banco, attive, con i loro byte veri che S5 dà come allegato (come il server per un formato che non è un PDF). Solo le
// prove delle TIF le aggiungono: gli altri banchi restano con i loro file.

import (
	"os"
	"path/filepath"
)

const (
	bdT1 = "00000000-0000-4000-8000-0000000a0101" // ACM-1400_ASSIEME_G4.tif
	bdT2 = "00000000-0000-4000-8000-0000000a0102" // ACM-1400_A_LZW.tif
	bdT3 = "00000000-0000-4000-8000-0000000a0103" // ACM-1400_DEFLATE.tif
	bdT4 = "00000000-0000-4000-8000-0000000a0104" // ACM-1400_ROTTA.tif
)

func (b *bancoDU) aggiungiTif() {
	b.t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.byteFile == nil {
		b.byteFile = map[string][]byte{}
	}
	for _, x := range []struct{ id, nome string }{
		{bdT1, "ACM-1400_ASSIEME_G4.tif"}, {bdT2, "ACM-1400_A_LZW.tif"}, {bdT3, "ACM-1400_DEFLATE.tif"}, {bdT4, "ACM-1400_ROTTA.tif"},
	} {
		dati, err := os.ReadFile(filepath.Join("testdata", "distinta_unica_tif", x.nome))
		if err != nil {
			b.t.Fatalf("la TIF di prova %s non si legge: %v", x.nome, err)
		}
		b.file = append(b.file, &fileBanco{ID: x.id, Proposta: bdProposta(x.id), Nome: x.nome, Est: "tif", Sha: bdSha(x.id), Origine: "portale",
			Canale: "posta", Stato: "attivo", Analisi: "letto"})
		b.byteFile[x.id] = dati
	}
}
