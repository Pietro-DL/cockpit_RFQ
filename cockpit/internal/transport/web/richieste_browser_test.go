//go:build integrazione && browser

// L7 — la pagina Richieste in un BROWSER VERO.
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags "integrazione browser" -count=1 -run TestL7Richieste ./internal/transport/web/
//
// Le RFQ sono quelle di L4 (scenaRichieste), con PDF veri nello staging perche' le anteprime si
// disegnino, e il poll a due secondi perche' se ne vedano due giri. Con COCKPIT_FOTO_DIR lo script salva
// le fotografie della pagina.
package web

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestL7Richieste(t *testing.T) {
	s := preparaRichieste(t)
	b := s.b
	// ogni RFQ di prova ha la sua cartella temporanea: la radice dello staging e' quella che le contiene
	b.ws.Staging = filepath.Dir(s.t1.staging)
	b.ws.PollRichieste = 2 * time.Second
	pdf := func(allegato uuid.UUID, codice string) {
		t.Helper()
		contenuto := pdfTavola(codice)
		var percorso string
		if err := b.pool.QueryRow(b.ctx, `SELECT path_staging FROM allegato WHERE allegato_id = $1`, allegato).Scan(&percorso); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(percorso, contenuto, 0o644); err != nil {
			t.Fatal(err)
		}
		s.esegui(`UPDATE allegato SET sha256 = $2, bytes = $3 WHERE allegato_id = $1`, allegato, shaDi(contenuto), len(contenuto))
	}
	pdf(s.docCorrente, "7120001")
	pdf(s.propostaP2, "7120002")
	for i := 1; i <= 4; i++ {
		codice := fmt.Sprintf("713000%d", i)
		pdf(s.allegatoDi(s.t3.proposta(codice+".pdf", codice, uuid.Nil)), codice)
	}

	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	arg := []string{filepath.Join("e2e", "richieste.py"), "--url", b.srv.URL, "--ip", "10.0.0.5:51000", "--sigla", "FP", "--password", "prova-fp"}
	if foto := os.Getenv("COCKPIT_FOTO_DIR"); foto != "" {
		arg = append(arg, "--foto", foto)
	}
	if canale := os.Getenv("COCKPIT_BROWSER_CANALE"); canale != "" {
		arg = append(arg, "--canale", canale)
	}
	if prove := os.Getenv("COCKPIT_PROVE"); prove != "" {
		arg = append(arg, "--prove", prove)
	}
	uscita, err := exec.Command(python, arg...).CombinedOutput()
	testo := strings.TrimRight(string(uscita), "\r\n")
	if err != nil {
		if strings.Contains(testo, "ModuleNotFoundError") || strings.Contains(testo, "Executable doesn't exist") {
			t.Skipf("Playwright o il browser non sono installati su questa macchina:\n%s", testo)
		}
		t.Fatalf("le prove nel browser sono fallite (%v):\n%s", err, testo)
	}
	t.Logf("prove nel browser:\n%s", testo)
}

// pdfTavola e' un PDF VERO, che il viewer del browser sa disegnare: una tavola A4 orizzontale con la
// cornice, un cartiglio con il codice e qualche vista. pdfFinto basta alla rotta (guarda solo i primi
// byte), ma nel viewer e' un file rotto: le miniature di questa pagina vanno viste davvero.
func pdfTavola(codice string) []byte {
	h := 0
	for _, c := range codice {
		h = h*31 + int(c)
	}
	dx, dy := 40+h%60, 30+(h/7)%50
	contenuto := fmt.Sprintf(`2 w 20 20 802 555 re S
1 w 520 20 302 100 re S 520 70 m 822 70 l S 700 20 m 700 70 l S
BT /F1 26 Tf 535 82 Td (%s) Tj ET
BT /F1 11 Tf 535 45 Td (ACME - disegno di prova) Tj ET
BT /F1 11 Tf 712 45 Td (rev A  1:2) Tj ET
3 w %d %d 300 170 re S
1 w %d %d m %d %d l S
2 w %d %d 60 170 re S
1 w %d %d 40 40 re S %d %d 40 40 re S
`, codice, 80+dx, 300+dy, 80+dx, 280+dy, 380+dx, 280+dy, 420+dx, 300+dy, 130+dx, 360+dy, 290+dx, 360+dy)
	oggetti := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 842 595] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(contenuto), contenuto),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	pos := make([]int, len(oggetti))
	for i, o := range oggetti {
		pos[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(oggetti)+1)
	for _, p := range pos {
		fmt.Fprintf(&b, "%010d 00000 n \n", p)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(oggetti)+1, xref)
	return []byte(b.String())
}
