//go:build integrazione && browser

// L7 — il Fascicolo v3 in un BROWSER VERO: la vista Documenti (pdf.js, filmstrip, scelta senza ricaricare),
// le note puntate sul disegno, l'editor della struttura, l'associazione dal pannello.
//
//	COCKPIT_TEST_DSN=postgres://…/cockpit_test go test -tags "integrazione browser" -count=1 -run TestL7V3 ./internal/transport/web/
//
// Il banco e' quello di fascicolo_browser_test.go (scenaL7), con PDF veri: pdf.js li disegna davvero, e un
// file rotto sarebbe un errore nella pagina.
package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func lanciaL7V3(t *testing.T, s *scenaL7, prove string) {
	t.Helper()
	python, err := exec.LookPath("python")
	if err != nil {
		t.Skipf("python non trovato: %v", err)
	}
	script := filepath.Join("e2e", "fascicolo_v3.py")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("lo script del browser non c'è: %v", err)
	}
	arg := []string{script, "--url", s.b.srv.URL, "--ip", "10.0.0.5:51000", "--sigla", "FP", "--password", "prova-fp",
		"--thread", s.thread.String(), "--prodotto", s.prodotto.String(), "--assieme", s.assieme.String(), "--particolare", s.particolare.String(),
		"--pdf", s.pdf.String(), "--prove", prove}
	if foto := os.Getenv("COCKPIT_FOTO_DIR"); foto != "" {
		arg = append(arg, "--foto", foto)
	}
	if canale := os.Getenv("COCKPIT_BROWSER_CANALE"); canale != "" {
		arg = append(arg, "--canale", canale)
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

// La vista Documenti e le note sul disegno.
func TestL7V3DocumentiENote(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaL7(t, "V3J", 0)
	lanciaL7V3(t, s, "JK")
	if n := s.conta(`SELECT count(*) FROM annotazione_pdf WHERE thread_id = $1`, s.thread); n != 0 {
		t.Errorf("la nota tolta nel browser e' ancora nel database: %d", n)
	}
}

// L'editor della struttura: il nodo proposto spostato sotto il prodotto entra con la conferma, con la
// quantita' che aveva, e la proposta dell'arco sotto l'assieme si chiude con il motivo.
func TestL7V3EditorDellaStruttura(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaL7(t, "V3L", 0)
	lanciaL7V3(t, s, "L")
	if got := s.valore(`SELECT r.qta::text FROM componente_relazione r JOIN componente c ON c.componente_id = r.figlio_id
		WHERE r.padre_id = $1 AND c.codice = '53011111'`, s.prodotto); got != "2" {
		t.Errorf("53011111 sotto il prodotto: %s", got)
	}
	if n := s.conta(`SELECT count(*) FROM componente_relazione r JOIN componente c ON c.componente_id = r.figlio_id
		WHERE r.padre_id = $1 AND c.codice = '53011111'`, s.assieme); n != 0 {
		t.Errorf("53011111 e' rimasto sotto l'assieme: %d", n)
	}
	if got := s.valore(`SELECT stato::text || ' ' || coalesce(nota, '') FROM relazione_proposta WHERE thread_id = $1 AND figlio_chiave = '#3'`, s.thread); got != "scartata tolta nella struttura confermata" {
		t.Errorf("la proposta dell'arco sotto l'assieme: %q", got)
	}
}

// L'associazione dal pannello della vista Documenti.
func TestL7V3AssociazioneDalPannello(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaL7(t, "V3M", 0)
	lanciaL7V3(t, s, "M")
	if n := s.conta(`SELECT count(*) FROM documento WHERE componente_id = $1 AND tipo = 'disegno_2d'`, s.assieme); n != 1 {
		t.Errorf("il PDF confermato dal pannello non e' un documento dell'assieme: %d", n)
	}
}

// L'editor senza STEP: un componente dei non posizionati trascinato nella struttura, spostato trascinandolo,
// condiviso sotto un secondo padre con il gesto esplicito. La conferma porta i due archi.
func TestL7V3EditorAMano(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaL7(t, "V3N", 0)
	vite := s.componenteTipo("54000000", "sciolto")
	lanciaL7V3(t, s, "N")
	if got := s.valore(`SELECT string_agg(p.codice || '>' || f.codice || 'x' || r.qta, ' ' ORDER BY p.codice) FROM componente_relazione r
		JOIN componente p ON p.componente_id = r.padre_id JOIN componente f ON f.componente_id = r.figlio_id WHERE r.figlio_id = $1`, vite); got != "52920517>54000000x1 52922757>54000000x1" {
		t.Errorf("54000000 nella BOM: %s", got)
	}
}
