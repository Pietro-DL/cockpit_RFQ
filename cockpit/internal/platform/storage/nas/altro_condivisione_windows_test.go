//go:build windows

package nas

// AF-34 (U2 §10.1, Windows) — il protocollo della copia e della rimozione su una condivisione di prova letta per
// percorso di rete: promozione senza sostituzione, rinomina esclusiva, verifica, cancellazione; il file aperto da un
// altro programma; la ripresa dopo la rinomina. La condivisione la indica COCKPIT_PROVA_CONDIVISIONE (un percorso UNC
// verso una cartella temporanea creata apposta, mai il NAS vero ne' una cartella sotto C:\promatec): senza, la prova e'
// SALTATA-AMBIENTE. La prova scrive solo in una sottocartella nuova e la toglie alla fine.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/google/uuid"
)

func TestAF34AltroSuCondivisioneDiProva(t *testing.T) {
	cond := strings.TrimSpace(os.Getenv("COCKPIT_PROVA_CONDIVISIONE"))
	if cond == "" || !strings.HasPrefix(cond, `\\`) {
		t.Skip("SALTATO-AMBIENTE: CONDIVISIONE: COCKPIT_PROVA_CONDIVISIONE non indica un percorso UNC di prova")
	}
	if strings.Contains(strings.ToLower(cond), "promatec") {
		t.Fatalf("la condivisione di prova %q sembra una cartella del server dell'utente: rifiutata", cond)
	}
	radice := filepath.Join(cond, "cockpit-af34-"+uuid.NewString())
	cartella := `ACME\RFQ prova`
	if err := os.MkdirAll(filepath.Join(radice, cartella), 0o755); err != nil {
		t.Skipf("SALTATO-AMBIENTE: CONDIVISIONE: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(radice) })
	dati := []byte("%PDF-1.4 ACME su condivisione")
	c := &Conservatore{Radice: radice}
	p := Posto{RadiceID: c.RadiceID(), Cartella: cartella, Percorso: `ALTRO\offerta.pdf`, Sha256: shaDi(dati), Bytes: int64(len(dati))}
	dst := filepath.Join(radice, cartella, "ALTRO", "offerta.pdf")

	if e, err := c.Copia(p, uuid.New(), bytes.NewReader(dati)); err != nil || !e.Creato {
		t.Fatalf("copia: %+v %v", e, err)
	}
	if e, err := c.Copia(p, uuid.New(), bytes.NewReader(dati)); err == nil || e.Creato {
		t.Fatalf("la seconda copia sullo stesso nome: %+v %v", e, err)
	}
	// aperto senza la condivisione in cancellazione, poi con: il file resta al suo nome
	for _, cond := range []uint32{syscall.FILE_SHARE_READ, syscall.FILE_SHARE_READ | syscall.FILE_SHARE_DELETE} {
		h := apriCon(t, dst, cond)
		e := c.Rimuovi(p, uuid.New(), false)
		syscall.CloseHandle(h)
		if e.Motivo != MotivoFileOccupato {
			t.Errorf("condivisione %x: %+v", cond, e)
		}
		if _, err := os.Stat(dst); err != nil {
			t.Fatalf("condivisione %x: il file non e' al suo nome: %v", cond, err)
		}
	}
	// ripresa dopo la rinomina (a): il nome di servizio c'e', si riparte dalla verifica
	tok := uuid.New()
	if err := rinominaSenzaSostituire(dst, filepath.Join(filepath.Dir(dst), NomeServizio("offerta.pdf", tok))); err != nil {
		t.Fatal(err)
	}
	if e := c.Rimuovi(p, tok, true); e.Motivo != MotivoRimosso {
		t.Fatalf("ripresa: %+v", e)
	}
	if v := vociDi(t, filepath.Dir(dst)); len(v) != 0 {
		t.Errorf("resta qualcosa: %v", v)
	}
}
