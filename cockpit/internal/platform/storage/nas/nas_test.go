package nas

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCopiaIdempotente(t *testing.T) {
	radice := t.TempDir()
	src := filepath.Join(t.TempDir(), "6674611A_4.pdf")
	os.WriteFile(src, []byte("contenuto pdf"), 0o644)
	sha, _, _ := Sha256File(src)
	s := &Scrittore{Radice: radice}

	token := uuid.New()
	dst, creato, err := s.Copia(src, `ACME\WIP\2026 09 08 Rossi X`, `ELENCO DISEGNI\6674611A\6674611A_4.pdf`, sha, token)
	if err != nil {
		t.Fatal(err)
	}
	if !creato {
		t.Error("la prima copia non dice di aver creato il file")
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("destinazione mancante: %v", err)
	}
	if _, err := os.Stat(dst + ".parte." + token.String()); !os.IsNotExist(err) {
		t.Fatalf(".parte non rimosso")
	}
	if parti, _ := s.PartiNellaCartella(`ACME\WIP\2026 09 08 Rossi X\ELENCO DISEGNI\6674611A`); len(parti) != 0 {
		t.Fatalf("file intermedi rimasti: %v", parti)
	}
	// ripetere non riscrive e non fallisce, e il file non e' «creato» da questa chiamata
	if _, creato, err := s.Copia(src, `ACME\WIP\2026 09 08 Rossi X`, `ELENCO DISEGNI\6674611A\6674611A_4.pdf`, sha, uuid.New()); err != nil || creato {
		t.Fatalf("seconda copia: creato %v, %v", creato, err)
	}
	// hash diverso già presente → conflitto, mai sovrascritto
	os.WriteFile(src, []byte("altro contenuto"), 0o644)
	sha2, _, _ := Sha256File(src)
	if _, _, err := s.Copia(src, `ACME\WIP\2026 09 08 Rossi X`, `ELENCO DISEGNI\6674611A\6674611A_4.pdf`, sha2, uuid.New()); !errors.Is(err, ErrConflitto) {
		t.Fatalf("atteso ErrConflitto, ottenuto %v", err)
	}
	b, _ := os.ReadFile(dst)
	if string(b) != "contenuto pdf" {
		t.Fatalf("file sovrascritto")
	}
	// hash atteso sbagliato → nessun file lasciato
	if _, _, err := s.Copia(src, `ACME\WIP\y`, `a.pdf`, "deadbeef", uuid.New()); err == nil {
		t.Fatalf("atteso errore hash")
	}
	if _, err := os.Stat(filepath.Join(radice, "ACME", "WIP", "y", "a.pdf")); !os.IsNotExist(err) {
		t.Fatalf("file con hash errato non deve esistere")
	}
	if parti, _ := s.PartiNellaCartella(`ACME\WIP\y`); len(parti) != 0 {
		t.Fatalf("il .parte con l'hash sbagliato e' rimasto: %v", parti)
	}
}

// Addendum A4.1: fra il controllo iniziale e la promozione un file puo' comparire alla destinazione.
// os.Rename lo sostituirebbe; la promozione esclusiva no, ne' con il link ne' con il ripiego.
func TestLaPromozioneDelParteNonSostituisceUnFileComparso(t *testing.T) {
	cartella := t.TempDir()
	scrivi := func(nome, contenuto string) string {
		t.Helper()
		p := filepath.Join(cartella, nome)
		if err := os.WriteFile(p, []byte(contenuto), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	leggi := func(p string) string {
		t.Helper()
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	sha := func(p string) string {
		t.Helper()
		h, _, err := Sha256File(p)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	casi := []struct {
		nome     string
		promuovi func(parte, dst, sha string) (bool, error)
	}{
		{"con il link", promuovi},
		{"con la creazione esclusiva (ripiego)", copiaEsclusiva},
	}
	for i, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			// un file diverso comparso: conflitto, e il file resta com'era
			parte := scrivi("a.pdf.parte."+uuid.NewString(), "il nostro contenuto")
			atteso := sha(parte)
			dst := scrivi(filepath.Base(parte)+".dst", "un file comparso nel frattempo")
			creato, err := c.promuovi(parte, dst, atteso)
			if !errors.Is(err, ErrConflitto) || creato {
				t.Fatalf("atteso ErrConflitto senza creazione, ottenuto creato=%v %v", creato, err)
			}
			if got := leggi(dst); got != "un file comparso nel frattempo" {
				t.Fatalf("la destinazione e' stata sostituita: %q", got)
			}
			// lo stesso contenuto comparso: niente da fare, ma non e' nostro
			parte = scrivi("b.pdf.parte."+uuid.NewString(), "stesso contenuto")
			dst = scrivi(filepath.Base(parte)+".dst", "stesso contenuto")
			if creato, err := c.promuovi(parte, dst, sha(parte)); err != nil || creato {
				t.Fatalf("stesso contenuto: creato=%v %v", creato, err)
			}
			// destinazione libera: creata, con i byte giusti
			parte = scrivi("c.pdf.parte."+uuid.NewString(), "contenuto nuovo")
			dst = filepath.Join(cartella, "c"+string(rune('0'+i))+".pdf")
			if creato, err := c.promuovi(parte, dst, sha(parte)); err != nil || !creato {
				t.Fatalf("destinazione libera: creato=%v %v", creato, err)
			}
			if got := leggi(dst); got != "contenuto nuovo" {
				t.Fatalf("destinazione: %q", got)
			}
		})
	}
}

// Prova 61: due tentativi dello stesso job non condividono il .parte, e un .parte riaperto dopo la
// promozione non tronca la destinazione.
func TestDueTentativiNonCondividonoIlParte(t *testing.T) {
	radice := t.TempDir()
	src := filepath.Join(t.TempDir(), "d.pdf")
	os.WriteFile(src, []byte("disegno completo"), 0o644)
	sha, _, _ := Sha256File(src)
	s := &Scrittore{Radice: radice}
	cartella := filepath.Join(radice, "R", "ELENCO DISEGNI")
	if err := os.MkdirAll(cartella, 0o755); err != nil {
		t.Fatal(err)
	}
	// il primo tentativo e' morto a meta' e ha lasciato il suo .parte
	morto := uuid.New()
	vecchio := filepath.Join(cartella, "d.pdf.parte."+morto.String())
	os.WriteFile(vecchio, []byte("disegno a me"), 0o644)

	vivo := uuid.New()
	dst, creato, err := s.Copia(src, `R`, `ELENCO DISEGNI\d.pdf`, sha, vivo)
	if err != nil || !creato {
		t.Fatalf("secondo tentativo: creato=%v %v", creato, err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "disegno completo" {
		t.Fatalf("destinazione: %q", b)
	}
	if b, err := os.ReadFile(vecchio); err != nil || string(b) != "disegno a me" {
		t.Fatalf("il .parte del primo tentativo e' stato toccato: %q %v", b, err)
	}

	// Il .parte del tentativo riuscito, riaperto dopo la promozione: e' un file nuovo, non la
	// destinazione, e scriverci non la tronca.
	if err := creaParte(src, dst+".parte."+vivo.String()); err != nil {
		t.Fatalf("il .parte promosso doveva essere gia' stato tolto: %v", err)
	}
	os.WriteFile(dst+".parte."+vivo.String(), []byte("x"), 0o644)
	if b, _ := os.ReadFile(dst); string(b) != "disegno completo" {
		t.Fatalf("riscrivere il .parte ha cambiato la destinazione: %q", b)
	}
	// E se il .parte fosse ancora li', legato alla destinazione: O_EXCL lo rifiuta invece di troncarlo.
	legato := filepath.Join(cartella, "e.pdf.parte."+uuid.NewString())
	os.WriteFile(legato, []byte("byte veri"), 0o644)
	eDst := filepath.Join(cartella, "e.pdf")
	if err := os.Link(legato, eDst); err != nil {
		t.Skipf("hard link non disponibile su questo disco: %v", err)
	}
	if err := creaParte(src, legato); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("riaprire un .parte esistente doveva essere rifiutato, ottenuto %v", err)
	}
	if b, _ := os.ReadFile(eDst); string(b) != "byte veri" {
		t.Fatalf("la destinazione legata al .parte e' stata troncata: %q", b)
	}
}

// Prova 74, la parte L1: si tolgono solo i .parte.<token> con token morto E vecchi.
func TestIParteScadutiSiTolgonoQuelliViviNo(t *testing.T) {
	radice := t.TempDir()
	s := &Scrittore{Radice: radice}
	cartella := filepath.Join(radice, "R", "2D")
	if err := os.MkdirAll(cartella, 0o755); err != nil {
		t.Fatal(err)
	}
	vecchio := time.Now().Add(-2 * time.Hour)
	morto, vivo, recente := uuid.New(), uuid.New(), uuid.New()
	file := map[string]time.Time{
		"a.pdf.parte." + morto.String():   vecchio,    // morto e vecchio: si toglie
		"a.pdf.parte." + vivo.String():    vecchio,    // vivo: resta, qualunque eta' abbia
		"a.pdf.parte." + recente.String(): time.Now(), // morto ma recente: resta
		"a.pdf.parte":                     vecchio,    // senza token (prima di A4): non e' un file intermedio
		"b.pdf.parte.non-un-token":        vecchio,    // non ha la forma: e' dell'utente
		"c.pdf":                           vecchio,    // un file vero
	}
	for nome, quando := range file {
		p := filepath.Join(cartella, nome)
		os.WriteFile(p, []byte(nome), 0o644)
		os.Chtimes(p, quando, quando)
	}
	tolti, err := s.PulisciParti(`R\2D`, map[uuid.UUID]bool{vivo: true}, time.Now().Add(-time.Hour))
	if err != nil || tolti != 1 {
		t.Fatalf("tolti %d, %v: atteso 1", tolti, err)
	}
	for nome := range file {
		_, err := os.Stat(filepath.Join(cartella, nome))
		dovevaSparire := strings.HasSuffix(nome, morto.String())
		if dovevaSparire != os.IsNotExist(err) {
			t.Errorf("%s: doveva sparire %v, errore %v", nome, dovevaSparire, err)
		}
	}
	// la cartella non e' vuota: non si toglie
	if err := s.RimuoviCartellaVuota(`R\2D`); err == nil {
		t.Error("una cartella con dei file e' stata tolta")
	}
	if _, ok := TokenDiParte("x.pdf.parte." + vivo.String()); !ok {
		t.Error("TokenDiParte non riconosce un .parte")
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
