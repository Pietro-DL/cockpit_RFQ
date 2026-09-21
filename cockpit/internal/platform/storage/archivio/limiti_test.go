// L1 — voce 1.8 del piano: budget di byte dello zip (A8, N10) e percorso davvero dentro la
// destinazione (N9, N11).
package archivio

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A8 — il budget vale sui byte scritti e si consuma fra una voce e l'altra.
//
// Prima il conto si faceva sulla dimensione dichiarata nell'header dello zip, e il tetto per voce era
// l'intero budget: nessuna somma dei byte davvero scritti. Bastava un archivio con più voci perché il
// totale su disco superasse il limite senza che nessun singolo controllo scattasse.
func TestBudgetSuiByteScritti(t *testing.T) {
	dieci := strings.Repeat("x", 10)
	zip := creaZip(t, map[string]string{"a.txt": dieci, "b.txt": dieci, "c.txt": dieci})
	dest := filepath.Join(t.TempDir(), "fuori")

	// budget di 25 byte: due voci da 10 entrano, la terza no
	voci, err := EstraiCon(zip, dest, 100, 25)
	if !errors.Is(err, ErrLimite) {
		t.Fatalf("errore = %v, atteso ErrLimite", err)
	}
	if len(voci) != 2 {
		t.Fatalf("voci estratte = %d, attese 2", len(voci))
	}
	var scritti int64
	for _, v := range voci {
		st, err := os.Stat(v.Path)
		if err != nil {
			t.Fatalf("la voce dichiarata non è sul disco: %v", err)
		}
		scritti += st.Size()
		if st.Size() != v.Bytes {
			t.Errorf("%s: dichiarati %d byte, sul disco %d", v.NomeFile, v.Bytes, st.Size())
		}
	}
	if scritti > 25 {
		t.Errorf("sul disco sono finiti %d byte con un budget di 25", scritti)
	}

	// la voce che ha sforato non deve lasciare un file a metà: un file troncato con l'hash di un altro
	// contenuto sarebbe peggio di nessun file.
	entri, _ := os.ReadDir(dest)
	if len(entri) != 2 {
		nomi := make([]string, 0, len(entri))
		for _, e := range entri {
			nomi = append(nomi, e.Name())
		}
		t.Errorf("file in destinazione = %v, attesi 2: la voce oltre il budget ha lasciato un residuo", nomi)
	}
}

// il budget esatto non è un errore: 30 byte di contenuto in un budget di 30 entrano tutti
func TestBudgetEsattoNonTronca(t *testing.T) {
	dieci := strings.Repeat("x", 10)
	zip := creaZip(t, map[string]string{"a.txt": dieci, "b.txt": dieci, "c.txt": dieci})
	voci, err := EstraiCon(zip, filepath.Join(t.TempDir(), "fuori"), 100, 30)
	if err != nil {
		t.Fatalf("errore con il budget esatto: %v", err)
	}
	if len(voci) != 3 {
		t.Errorf("voci = %d, attese 3", len(voci))
	}
}

// il limite sul numero di voci resta e si vede
func TestLimiteNumeroVoci(t *testing.T) {
	zip := creaZip(t, map[string]string{"a.txt": "a", "b.txt": "b", "c.txt": "c"})
	voci, err := EstraiCon(zip, filepath.Join(t.TempDir(), "fuori"), 2, 1<<20)
	if !errors.Is(err, ErrLimite) || len(voci) != 2 {
		t.Errorf("voci = %d, err = %v (attese 2 e ErrLimite)", len(voci), err)
	}
}

// N9/N11 — «dentro» risponde alla domanda giusta. Il controllo per prefisso di stringa sbagliava sia
// per eccesso (una cartella sorella con lo stesso inizio di nome) sia per difetto (maiuscole diverse
// sullo stesso percorso di Windows).
func TestDentroNonSiFaIngannareDalPrefisso(t *testing.T) {
	radice := filepath.FromSlash("C:/staging")
	casi := []struct {
		percorso string
		atteso   bool
		perche   string
	}{
		{filepath.FromSlash("C:/staging/allegato/1.pdf"), true, "figlio diretto"},
		{filepath.FromSlash("C:/staging"), true, "la radice stessa"},
		{filepath.FromSlash("C:/staging-vecchio/1.pdf"), false, "cartella sorella con lo stesso prefisso di nome"},
		{filepath.FromSlash("C:/staging/../altro/1.pdf"), false, "risalita che esce dalla radice"},
		{filepath.FromSlash("C:/altro/1.pdf"), false, "tutt'altro ramo"},
	}
	for _, c := range casi {
		if got := dentro(radice, c.percorso); got != c.atteso {
			t.Errorf("dentro(%q, %q) = %v, atteso %v — %s", radice, c.percorso, got, c.atteso, c.perche)
		}
	}
}

// zip-slip: una voce che prova a uscire dalla destinazione viene saltata, non scritta altrove
func TestZipSlipNonEsceDallaDestinazione(t *testing.T) {
	zip := creaZip(t, map[string]string{
		"../../fuori.txt":         "non deve uscire",
		"buona.txt":               "questa sì",
		`..\..\anche-windows.txt`: "nemmeno con le barre rovesce",
	})
	base := t.TempDir()
	dest := filepath.Join(base, "staging", "zip")
	voci, err := EstraiCon(zip, dest, 100, 1<<20)
	if err != nil {
		t.Fatalf("estrai: %v", err)
	}
	for _, v := range voci {
		if !dentro(dest, v.Path) {
			t.Errorf("voce scritta fuori dalla destinazione: %s", v.Path)
		}
	}
	// nessun file deve essere comparso sopra la destinazione
	for _, sopra := range []string{filepath.Join(base, "fuori.txt"), filepath.Join(base, "staging", "fuori.txt"),
		filepath.Join(base, "anche-windows.txt"), filepath.Join(base, "staging", "anche-windows.txt")} {
		if _, err := os.Stat(sopra); err == nil {
			t.Errorf("lo zip ha scritto fuori dalla destinazione: %s", sopra)
		}
	}
}
