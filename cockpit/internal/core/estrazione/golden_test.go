// L1 — i golden degli adattatori (piano A, 5.6.3): l'aiuto che li legge, li confronta e li riscrive solo
// dichiarandolo (A1b-18), la versione degli adattatori fissata (5.6.1), e i documenti degli adattatori contro i
// loro golden, sugli ingressi sintetici di testdata/ingressi (A1b-14 per lo STEP).
//
// Tutti i dati sono sintetici: ACME, codici di fantasia (ACME7000100, 9999999A, CORDONE_ID_0001…), UUID della
// forma 00000000-0000-4000-8000-0000000000nn. Le forme sono quelle dei fatti veri del worker, il contenuto no:
// il repository è pubblico, e un golden non nasce mai da dati reali.
package estrazione

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// La disciplina dei golden (5.6.3; par.3.7.3).
const (
	varAggiornaGolden  = "COCKPIT_AGGIORNA_GOLDEN" // "1" riscrive i golden, e la corsa fallisce
	varDatasetPrivato  = "COCKPIT_DATASET_A"       // con il dataset privato impostato la riscrittura si rifiuta
	msgGoldenRiscritti = "golden riscritti: rivedere il diff e rilanciare senza la variabile"
)

// conTagPrivato è vero solo nelle corse con il tag privato (golden_privato_test.go): allora la riscrittura dei
// golden si rifiuta, perché un golden non nasce mai da una corsa con dati privati (5.6.3).
var conTagPrivato = false

// errGoldenRiscritti: l'esito di una riscrittura. È un errore di proposito: la corsa che riscrive fallisce, e
// i golden si tengono solo dopo averne letto il diff e rilanciato senza la variabile.
var errGoldenRiscritti = errors.New(msgGoldenRiscritti)

// aggiornamentoGolden dice se la corsa riscrive i golden: solo con COCKPIT_AGGIORNA_GOLDEN=1. La riscrittura si
// rifiuta, con un errore, con il tag privato o con il dataset privato impostato. getenv è os.Getenv nelle
// corse, una mappa nella prova della disciplina.
func aggiornamentoGolden(getenv func(string) string, privato bool) (bool, error) {
	if getenv(varAggiornaGolden) != "1" {
		return false, nil
	}
	if privato {
		return false, fmt.Errorf("%s=1 rifiutata: la corsa ha il tag privato, e un golden non nasce da dati privati", varAggiornaGolden)
	}
	if getenv(varDatasetPrivato) != "" {
		return false, fmt.Errorf("%s=1 rifiutata: il dataset privato è impostato, e un golden non nasce da dati privati", varAggiornaGolden)
	}
	return true, nil
}

// verificaGolden confronta v con il golden nel percorso. Si confrontano i byte del JSON canonico: il golden si
// rilegge con jsoncanonico, quindi indentazione e fine riga del file non contano. Con aggiorna il golden si
// riscrive indentato, con fine riga LF, e il risultato è errGoldenRiscritti.
func verificaGolden(percorso string, v any, aggiorna bool) error {
	ottenuto, err := jsoncanonico.Codifica(v)
	if err != nil {
		return fmt.Errorf("canonico del risultato: %w", err)
	}
	if aggiorna {
		var b bytes.Buffer
		if err := json.Indent(&b, ottenuto, "", "  "); err != nil {
			return err
		}
		b.WriteByte('\n')
		if err := os.MkdirAll(filepath.Dir(percorso), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(percorso, b.Bytes(), 0o644); err != nil {
			return err
		}
		return errGoldenRiscritti
	}
	grezzo, err := os.ReadFile(percorso)
	if err != nil {
		return fmt.Errorf("golden %s non letto (si genera con %s=1, poi si rivede il diff): %w", percorso, varAggiornaGolden, err)
	}
	atteso, err := jsoncanonico.Codifica(json.RawMessage(grezzo))
	if err != nil {
		return fmt.Errorf("golden %s: non è un JSON valido per jsoncanonico: %w", percorso, err)
	}
	if !bytes.Equal(atteso, ottenuto) {
		return fmt.Errorf("diverso dal golden %s:\n%s", percorso, primaDifferenza(atteso, ottenuto))
	}
	return nil
}

// primaDifferenza: il punto in cui due canonici si separano, con un po' di contesto, per leggere l'errore
// senza un diff esterno.
func primaDifferenza(atteso, ottenuto []byte) string {
	i := 0
	for i < len(atteso) && i < len(ottenuto) && atteso[i] == ottenuto[i] {
		i++
	}
	da := max(0, i-80)
	taglio := func(b []byte) string { return string(b[da:min(len(b), i+80)]) }
	return fmt.Sprintf("al byte %d\n  golden:    …%s…\n  risultato: …%s…", i, taglio(atteso), taglio(ottenuto))
}

// confrontaGolden: l'aiuto delle prove. La riscrittura fa fallire la corsa con il messaggio della disciplina.
func confrontaGolden(t *testing.T, percorso string, v any) {
	t.Helper()
	aggiorna, err := aggiornamentoGolden(os.Getenv, conTagPrivato)
	if err != nil {
		t.Fatal(err)
	}
	if err := verificaGolden(percorso, v, aggiorna); err != nil {
		t.Error(err)
	}
}

// TestLaVersioneDegliAdattatoriEFissa (5.6.1): «adattatori-1» e le sue mappature. Se un valore cambia, questa
// prova si riscrive con il titolo «Riscritta per …», insieme ai golden.
func TestLaVersioneDegliAdattatoriEFissa(t *testing.T) {
	for _, c := range [][2]string{
		{VersioneAdattatore, "adattatori-1"},
		{mappaturaSTEP, "mappatura-step-1"},
		{mappaturaPDF, "mappatura-pdf-1"},
		{mappaturaNome, "mappatura-nome-1"},
		{mappaturaEmail, "mappatura-email-1"},
	} {
		if c[0] != c[1] {
			t.Errorf("versione %q, attesa %q: un cambio di versione si dichiara nel commit", c[0], c[1])
		}
	}
}

// TestIGoldenSiRiscrivonoSoloDichiarandolo (A1b-18): con la variabile la riscrittura fallisce di proposito;
// senza, il golden riscritto passa, anche con i fine riga CRLF; un risultato diverso non passa; con il tag
// privato o con il dataset privato la variabile si rifiuta. Il golden sta in t.TempDir(): i golden veri non si
// toccano.
func TestIGoldenSiRiscrivonoSoloDichiarandolo(t *testing.T) {
	ambiente := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	for _, c := range []struct {
		nome    string
		env     map[string]string
		privato bool
		vuole   bool
		errore  bool
	}{
		{"senza la variabile", nil, false, false, false},
		{"con un valore diverso da 1", map[string]string{varAggiornaGolden: "si"}, false, false, false},
		{"con la variabile", map[string]string{varAggiornaGolden: "1"}, false, true, false},
		{"con il tag privato", map[string]string{varAggiornaGolden: "1"}, true, false, true},
		{"con il dataset privato", map[string]string{varAggiornaGolden: "1", varDatasetPrivato: "manifest"}, false, false, true},
		{"il dataset da solo non riscrive", map[string]string{varDatasetPrivato: "manifest"}, false, false, false},
	} {
		vuole, err := aggiornamentoGolden(ambiente(c.env), c.privato)
		if vuole != c.vuole || (err != nil) != c.errore {
			t.Errorf("%s: riscrive %v, errore %v; attesi %v e errore %v", c.nome, vuole, err, c.vuole, c.errore)
		}
	}

	doc, err := DaTesto(selettoreDi("nome_file"), "ACME-030PB07XX0001.pdf")
	if err != nil {
		t.Fatal(err)
	}
	percorso := filepath.Join(t.TempDir(), "golden", "nome_acme.json")
	if err := verificaGolden(percorso, doc, false); err == nil {
		t.Fatal("un golden che non c'è passa")
	}
	if err := verificaGolden(percorso, doc, true); !errors.Is(err, errGoldenRiscritti) || err.Error() != msgGoldenRiscritti {
		t.Fatalf("la riscrittura dà %v, attesa la corsa fallita con «%s»", err, msgGoldenRiscritti)
	}
	scritto, err := os.ReadFile(percorso)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(scritto, []byte("\r")) || !bytes.HasSuffix(scritto, []byte("}\n")) || !bytes.Contains(scritto, []byte("\n  \"")) {
		t.Errorf("il golden va scritto indentato, con fine riga LF:\n%s", scritto)
	}
	if err := verificaGolden(percorso, doc, false); err != nil {
		t.Errorf("il golden appena riscritto non passa: %v", err)
	}
	crlf := bytes.ReplaceAll(scritto, []byte("\n"), []byte("\r\n"))
	if err := os.WriteFile(percorso, crlf, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verificaGolden(percorso, doc, false); err != nil {
		t.Errorf("un golden con CRLF dà un altro canonico: %v", err)
	}
	altro, err := DaTesto(selettoreDi("nome_file"), "ACME-030PB07XX0002.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if err := verificaGolden(percorso, altro, false); err == nil || !strings.Contains(err.Error(), "al byte") {
		t.Errorf("un risultato diverso passa, o l'errore non dice dove: %v", err)
	}
}

// ---- i golden degli adattatori ----

// ingressoAllegato: la forma degli ingressi di un file (5.6.3): il record dell'allegato, il contenitore e i
// fatti, senza digest (lo calcola la prova, con fotorfq.ImprontaPayload, come chiunque riempia un Fatti).
type ingressoAllegato struct {
	Allegato    fotorfq.Allegato  `json:"allegato"`
	Contenitore *fotorfq.Allegato `json:"contenitore"`
	Fatti       *fotorfq.Fatti    `json:"fatti"`
}

// casoGolden: un ingresso e il suo golden. I nomi stanno qui, in una tabella, e non si cercano nelle cartelle.
// struttura è ciò che worker.DecodificaStruttura deve dire del payload: un ingresso che non si decodifica come
// la tabella dichiara è un errore della prova, non un caso dell'adattatore.
type casoGolden struct {
	tipo      string // step
	nome      string // il file in testdata/ingressi/<tipo>/ e in testdata/golden/
	struttura bool
}

// casiGolden: F-STEP-1…10 (5.7.2). F-STEP-6 ha due ingressi: il file che non è Part 21 e la struttura v1.
var casiGolden = []casoGolden{
	{"step", "step_01_assieme", true},
	{"step", "step_02_due_radici", true},
	{"step", "step_03_figlio_con_due_padri", true},
	{"step", "step_04_formazioni_alternative", true},
	{"step", "step_05_troncata_con_scarti", true},
	{"step", "step_06_non_step21", true},
	{"step", "step_06b_struttura_v1", false},
	{"step", "step_07_caratteri", true},
	{"step", "step_08_formazioni", true},
	{"step", "step_09_cordoni", true},
	{"step", "step_10_v2_senza_scarti", true},
}

// leggiIngressoAllegato decodifica un ingresso in modo stretto (chiavi sconosciute rifiutate), controlla che
// il payload si decodifichi come la tabella dichiara, e calcola il Digest dei fatti.
func leggiIngressoAllegato(t *testing.T, c casoGolden) ingressoAllegato {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "ingressi", c.tipo, c.nome+".json"))
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var in ingressoAllegato
	if err := dec.Decode(&in); err != nil {
		t.Fatalf("%s: ingresso non valido: %v", c.nome, err)
	}
	if in.Fatti == nil {
		return in
	}
	if in.Fatti.Digest != "" {
		t.Fatalf("%s: il digest non sta negli ingressi: lo calcola la prova", c.nome)
	}
	if _, ok := worker.DecodificaStruttura(in.Fatti.Payload); ok != c.struttura {
		t.Fatalf("%s: DecodificaStruttura dice %v, la tabella dichiara %v", c.nome, ok, c.struttura)
	}
	if in.Fatti.Digest, err = fotorfq.ImprontaPayload(in.Fatti.Payload); err != nil {
		t.Fatalf("%s: %v", c.nome, err)
	}
	return in
}

// TestIGoldenDegliAdattatori (A1b-14): ogni ingresso dà il documento del suo golden, byte per byte nel JSON
// canonico. Prima di tenere un golden lo si legge per intero: ogni unità, entità, legame, capacità e
// diagnostica deve venire dall'ingresso e dalla mappatura (5.4.5). Un golden sbagliato si corregge nel prodotto,
// mai a mano.
func TestIGoldenDegliAdattatori(t *testing.T) {
	perTipo := map[string][]casoGolden{}
	var tipi []string
	for _, c := range casiGolden {
		if perTipo[c.tipo] == nil {
			tipi = append(tipi, c.tipo)
		}
		perTipo[c.tipo] = append(perTipo[c.tipo], c)
	}
	for _, tipo := range tipi {
		t.Run(tipo, func(t *testing.T) {
			for _, c := range perTipo[tipo] {
				t.Run(c.nome, func(t *testing.T) {
					in := leggiIngressoAllegato(t, c)
					doc, err := DaAllegato(in.Allegato, in.Fatti, in.Contenitore)
					if err != nil {
						t.Fatal(err)
					}
					confrontaGolden(t, filepath.Join("testdata", "golden", c.nome+".json"), doc)
				})
			}
		})
	}
}
