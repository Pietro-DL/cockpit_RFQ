// L1 — blocco 5B: come si classifica un documento confrontandolo con il file vero.
//
// Sono i cinque casi del checkpoint piu' i due che escono dal disegno: il file gia' presente e
// giusto (che non va copiato di nuovo) e il file che c'e' ma non si legge (che non e' un conflitto:
// dire «qualcuno l'ha sostituito» sulla base di un permesso negato sarebbe un'accusa inventata).
//
// Sta a L1 perche' bastano una cartella temporanea e un orologio finto: non serve un database per
// sapere che un file che non c'e' non c'e'.
package jobs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/nas"
	"promatec/cockpit/internal/platform/db"
)

const cartellaProva = `ACME\WIP\2026 09 17 prova`

// bancoEsame prepara una radice con dentro, se contenuto non e' vuoto, il file del documento.
func bancoEsame(t *testing.T, contenuto string) (string, db.Documento) {
	t.Helper()
	radice := t.TempDir()
	testo := "questo e' il disegno"
	sha, _, err := shaDi(t, testo)
	if err != nil {
		t.Fatal(err)
	}
	d := db.Documento{
		DocumentoID: uuid.New(), ThreadID: uuid.New(), Tipo: db.TipoDocumentoDisegno2d,
		NomeFile: "6674611A_4.pdf", Estensione: "pdf", Sha256: sha,
		PathRelativo: `ELENCO DISEGNI\6674611A\6674611A_4.pdf`,
		StatoNas:     db.StatoNasInCoda, ConfermatoIl: time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC),
	}
	if contenuto != "" {
		p := filepath.Join(radice, "ACME", "WIP", "2026 09 17 prova", "ELENCO DISEGNI", "6674611A")
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "6674611A_4.pdf"), []byte(contenuto), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return radice, d
}

func shaDi(t *testing.T, testo string) (string, int64, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(p, []byte(testo), 0o644); err != nil {
		return "", 0, err
	}
	return nas.Sha256File(p)
}

func cartella(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }

// adesso e' molto dopo la conferma: le attese sono tutte «vecchie» se non si dice il contrario.
var adessoProva = time.Date(2026, 9, 17, 8, 0, 0, 0, time.UTC)

func TestUnDocumentoScrittoSenzaFileEeMancante(t *testing.T) {
	radice, d := bancoEsame(t, "")
	d.StatoNas = db.StatoNasScritto

	c := controllo{radice: radice, attesa: 30 * time.Minute, adesso: adessoProva, scrittura: true}
	e := c.esamina(d, cartella(cartellaProva))

	if e.Problema != db.ProblemaNasMancante {
		t.Fatalf("problema = %q, atteso mancante: il database dice che il file c'e' e il file non c'e'", e.Problema)
	}
	if !strings.Contains(e.Percorso, "6674611A_4.pdf") {
		t.Errorf("il percorso non dice dove si e' guardato: %q", e.Percorso)
	}
	if strings.Contains(e.Percorso, radice) {
		t.Errorf("il percorso contiene la radice di QUESTO server: una riga di anomalia deve poter essere "+
			"letta anche da un'altra postazione\n  %s", e.Percorso)
	}
}

func TestUnFileConUnAltroContenutoEeUnConflitto(t *testing.T) {
	radice, d := bancoEsame(t, "tutt'altra cosa")
	d.StatoNas = db.StatoNasScritto

	c := controllo{radice: radice, attesa: 30 * time.Minute, adesso: adessoProva, scrittura: true}
	e := c.esamina(d, cartella(cartellaProva))

	if e.Problema != db.ProblemaNasConflitto {
		t.Fatalf("problema = %q, atteso conflitto", e.Problema)
	}
	if e.ShaTrovato == "" || e.ShaTrovato == d.Sha256 {
		t.Errorf("l'hash trovato non e' registrato: senza, il conflitto non e' verificabile da nessuno (%q)", e.ShaTrovato)
	}
	if !strings.Contains(e.Dettaglio, "Non viene sovrascritto") {
		t.Errorf("il dettaglio non dice che il file NON viene toccato:\n  %s", e.Dettaglio)
	}
	// e il file sul disco non deve essere stato toccato dal solo fatto di averlo guardato
	b, err := os.ReadFile(filepath.Join(radice, "ACME", "WIP", "2026 09 17 prova", "ELENCO DISEGNI", "6674611A", "6674611A_4.pdf"))
	if err != nil || string(b) != "tutt'altra cosa" {
		t.Fatalf("il ricognitore ha toccato il file: %q (%v)", string(b), err)
	}
}

func TestUnFileGiaGiustoNonEeDaCopiare(t *testing.T) {
	radice, d := bancoEsame(t, "questo e' il disegno")

	c := controllo{radice: radice, attesa: 30 * time.Minute, adesso: adessoProva, scrittura: true}
	e := c.esamina(d, cartella(cartellaProva))

	if e.Problema != db.ProblemaNasGiaPresente {
		t.Fatalf("problema = %q, atteso gia_presente: il file e' li' ed e' byte per byte quello giusto", e.Problema)
	}
	if e.ShaTrovato != d.Sha256 {
		t.Errorf("hash trovato %q, atteso %q", e.ShaTrovato, d.Sha256)
	}
}

func TestUnDocumentoScrittoConIlSuoFileNonEeUnProblema(t *testing.T) {
	radice, d := bancoEsame(t, "questo e' il disegno")
	d.StatoNas = db.StatoNasScritto

	c := controllo{radice: radice, attesa: 30 * time.Minute, adesso: adessoProva, scrittura: true}
	if e := c.esamina(d, cartella(cartellaProva)); e.Problema != "" {
		t.Fatalf("un documento a posto viene segnalato come %q: un elenco pieno di righe giuste e' un elenco che nessuno guarda", e.Problema)
	}
}

func TestUnaCopiaAppenaConfermataNonVieneSegnalata(t *testing.T) {
	radice, d := bancoEsame(t, "")
	d.ConfermatoIl = adessoProva.Add(-5 * time.Minute)

	c := controllo{radice: radice, attesa: 30 * time.Minute, adesso: adessoProva, scrittura: true}
	if e := c.esamina(d, cartella(cartellaProva)); e.Problema != "" {
		t.Fatalf("una conferma di cinque minuti fa e' gia' un'anomalia (%q): la copia non ha ancora avuto il tempo di partire", e.Problema)
	}
}

func TestUnaCopiaGiaInCodaNonEeUnLavoroPerUnaPersona(t *testing.T) {
	radice, d := bancoEsame(t, "")

	c := controllo{radice: radice, attesa: 30 * time.Minute, adesso: adessoProva, scrittura: true, inCoda: true}
	if e := c.esamina(d, cartella(cartellaProva)); e.Problema != "" {
		t.Fatalf("segnalato come %q un documento la cui copia e' gia' in coda: si chiederebbe a qualcuno "+
			"di occuparsi di una cosa che il sistema sta gia' facendo", e.Problema)
	}
}

func TestUnAttesaVecchiaDiceDaQuantoEPerche(t *testing.T) {
	radice, d := bancoEsame(t, "")

	// scrittura spenta: il motivo dell'attesa e' noto, e va detto con il nome della riga da cambiare
	c := controllo{radice: radice, attesa: 30 * time.Minute, adesso: adessoProva, scrittura: false}
	e := c.esamina(d, cartella(cartellaProva))
	if e.Problema != db.ProblemaNasInAttesa {
		t.Fatalf("problema = %q, atteso in_attesa", e.Problema)
	}
	if !strings.Contains(e.Dettaglio, "16 giorni") {
		t.Errorf("il dettaglio non dice da quanto aspetta:\n  %s", e.Dettaglio)
	}
	if !strings.Contains(e.Dettaglio, "nas_scrittura") {
		t.Errorf("con la scrittura spenta il dettaglio non nomina la capacita':\n  %s", e.Dettaglio)
	}

	// scrittura accesa e nessuna copia in coda: e' fermo davvero, e va detto in un altro modo
	c.scrittura = true
	e = c.esamina(d, cartella(cartellaProva))
	if !strings.Contains(e.Dettaglio, "nessuna copia in coda") {
		t.Errorf("con la scrittura accesa il dettaglio non dice che nessuno lo sta copiando:\n  %s", e.Dettaglio)
	}
}

func TestUnaCopiaFallitaPortaConSeIlMotivo(t *testing.T) {
	radice, d := bancoEsame(t, "")
	d.StatoNas = db.StatoNasErrore
	d.ErroreNas = pgtype.Text{String: `contenuto non piu' in staging: il contenuto di "6674611A_4.pdf" non e' piu' nello staging`, Valid: true}

	c := controllo{radice: radice, attesa: 30 * time.Minute, adesso: adessoProva, scrittura: true}
	e := c.esamina(d, cartella(cartellaProva))

	if e.Problema != db.ProblemaNasErrore {
		t.Fatalf("problema = %q, atteso errore", e.Problema)
	}
	if !strings.Contains(e.Dettaglio, "staging") {
		t.Errorf("il motivo del fallimento non arriva nell'anomalia: andrebbe cercato nel log\n  %s", e.Dettaglio)
	}
}

// Una cartella con il nome del file non e' «manca il file»: il nome e' occupato, e la copia non
// potrebbe scriverci nemmeno volendo. Dirlo «mancante» manderebbe l'operatore a premere «Riaccoda»
// per una copia che fallirebbe ogni volta.
func TestUnaCartellaAlPostoDelFileSiDice(t *testing.T) {
	radice, d := bancoEsame(t, "")
	occupato := filepath.Join(radice, "ACME", "WIP", "2026 09 17 prova", "ELENCO DISEGNI", "6674611A", "6674611A_4.pdf")
	if err := os.MkdirAll(occupato, 0o755); err != nil {
		t.Fatal(err)
	}

	c := controllo{radice: radice, attesa: 30 * time.Minute, adesso: adessoProva, scrittura: true}
	e := c.esamina(d, cartella(cartellaProva))

	if e.Problema != db.ProblemaNasIlleggibile {
		t.Fatalf("problema = %q, atteso illeggibile", e.Problema)
	}
	if !strings.Contains(e.Dettaglio, "CARTELLA") {
		t.Errorf("il dettaglio non dice che cosa c'e' al posto del file:\n  %s", e.Dettaglio)
	}
}

func TestUnaRfqSenzaCartellaNonHaUnPostoInCuiGuardare(t *testing.T) {
	radice, d := bancoEsame(t, "")

	c := controllo{radice: radice, attesa: 30 * time.Minute, adesso: adessoProva, scrittura: true}
	e := c.esamina(d, cartella(""))

	if e.Problema != db.ProblemaNasInAttesa {
		t.Fatalf("problema = %q, atteso in_attesa", e.Problema)
	}
	if !strings.Contains(e.Dettaglio, "cartella") {
		t.Errorf("il dettaglio non dice che manca la cartella della RFQ:\n  %s", e.Dettaglio)
	}
}

// «3 giorni» e' una risposta alla domanda «e' tanto?»; «72h13m0s» non lo e'.
func TestLaDurataSiLegge(t *testing.T) {
	casi := map[time.Duration]string{
		90 * time.Second:    "meno di due minuti",
		40 * time.Minute:    "40 minuti",
		5 * time.Hour:       "5 ore",
		74 * time.Hour:      "3 giorni",
		30 * 24 * time.Hour: "30 giorni",
	}
	for d, atteso := range casi {
		if got := durata(d); got != atteso {
			t.Errorf("durata(%s) = %q, atteso %q", d, got, atteso)
		}
	}
}
