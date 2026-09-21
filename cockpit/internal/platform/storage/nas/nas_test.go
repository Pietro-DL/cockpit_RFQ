package nas

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopiaIdempotente(t *testing.T) {
	radice := t.TempDir()
	src := filepath.Join(t.TempDir(), "6674611A_4.pdf")
	os.WriteFile(src, []byte("contenuto pdf"), 0o644)
	sha, _, _ := Sha256File(src)
	s := &Scrittore{Radice: radice}

	dst, err := s.Copia(src, `ACME\WIP\2026 09 08 Rossi X`, `ELENCO DISEGNI\6674611A\6674611A_4.pdf`, sha)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("destinazione mancante: %v", err)
	}
	if _, err := os.Stat(dst + ".parte"); !os.IsNotExist(err) {
		t.Fatalf(".parte non rimosso")
	}
	// ripetere non riscrive e non fallisce
	if _, err := s.Copia(src, `ACME\WIP\2026 09 08 Rossi X`, `ELENCO DISEGNI\6674611A\6674611A_4.pdf`, sha); err != nil {
		t.Fatalf("seconda copia: %v", err)
	}
	// hash diverso già presente → conflitto, mai sovrascritto
	os.WriteFile(src, []byte("altro contenuto"), 0o644)
	sha2, _, _ := Sha256File(src)
	if _, err := s.Copia(src, `ACME\WIP\2026 09 08 Rossi X`, `ELENCO DISEGNI\6674611A\6674611A_4.pdf`, sha2); !errors.Is(err, ErrConflitto) {
		t.Fatalf("atteso ErrConflitto, ottenuto %v", err)
	}
	b, _ := os.ReadFile(dst)
	if string(b) != "contenuto pdf" {
		t.Fatalf("file sovrascritto")
	}
	// hash atteso sbagliato → nessun file lasciato
	if _, err := s.Copia(src, `ACME\WIP\y`, `a.pdf`, "deadbeef"); err == nil {
		t.Fatalf("atteso errore hash")
	}
	if _, err := os.Stat(filepath.Join(radice, "ACME", "WIP", "y", "a.pdf")); !os.IsNotExist(err) {
		t.Fatalf("file con hash errato non deve esistere")
	}
}

func TestDryRun(t *testing.T) {
	s := &Scrittore{Radice: t.TempDir(), DryRun: true}
	if _, err := s.CreaCartella(`ACME\WIP\x`, []string{"ELENCO DISEGNI"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Radice, "ACME")); !os.IsNotExist(err) {
		t.Fatalf("dry run ha creato cartelle")
	}
}

func TestUNC(t *testing.T) {
	if got := UNC(`\\nas01\TECNICO - PREVENTIVI\PREVENTIVI DA FARE`, `ACME\WIP\x`); got != `\\nas01\TECNICO - PREVENTIVI\PREVENTIVI DA FARE\ACME\WIP\x` {
		t.Errorf("UNC corto: %q", got)
	}
	if got := UNC(`C:\promatec\_nas_test\PREVENTIVI DA FARE\`, `\ACME\WIP\x`); got != `C:\promatec\_nas_test\PREVENTIVI DA FARE\ACME\WIP\x` {
		t.Errorf("UNC locale: %q", got)
	}
	lungo := UNC(`\\nas01\radice`, strings.Repeat(`cartella lunga\`, 20)+"file.pdf")
	if !strings.HasPrefix(lungo, `\\?\UNC\nas01\radice\`) {
		t.Errorf("UNC lungo di rete senza prefisso: %q", lungo)
	}
	lungoLocale := UNC(`C:\radice`, strings.Repeat(`cartella lunga\`, 20)+"file.pdf")
	if !strings.HasPrefix(lungoLocale, `\\?\C:\radice\`) {
		t.Errorf("UNC lungo locale senza prefisso: %q", lungoLocale)
	}
	if strings.Count(lungo, `\\?\`) != 1 || strings.HasPrefix(UNC(lungo, "y"), `\\?\\\?\`) {
		t.Errorf("prefisso duplicato: %q", UNC(lungo, "y"))
	}
}
