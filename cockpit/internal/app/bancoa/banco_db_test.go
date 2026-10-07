//go:build integrazione

package bancoa

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"promatec/cockpit"
	"promatec/cockpit/internal/platform/testutil"
)

// L4 — la sequenza -dsn del banco sul DB di prova (piano 6.4.9, passi 1–3; R32 c, R44, R92): con le migrazioni
// incorporate del binario lo schema coincide e ApriInLettura apre; il ruolo del DB di prova può scrivere, quindi
// ControllaSolaLettura ferma la corsa prima di qualunque lettura di dominio: NON ESEGUITO (uscita 3), con il rapporto,
// la riga della sorgente senza password, il ruolo e «scrittura possibile: sì» nel rapporto, nessuna fotografia, e il
// database com'era (nessuna scrittura). Sul DB di prova il banco non arriva mai al caricatore: la parte che segue la
// provano le L1 sugli export e le L4 del caricatore e di confronto.
//
// I clienti di questi test sono inventati: vedi scena_banco_test.go. Il DB è quello di prova (COCKPIT_TEST_DSN), mai
// la copia del dump.
func TestL4BancoDSNFermoSulRuoloCheScrive(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	dsn := testutil.DSN(t)
	if err := dsnSenzaPassword(dsn); err != nil {
		t.Skipf("SALTATO-AMBIENTE: il DSN del DB di prova porta la password, e il banco la rifiuta (R32 c)")
	}
	if v, ok := os.LookupEnv("PGPASSWORD"); ok {
		os.Unsetenv("PGPASSWORD")
		t.Cleanup(func() { os.Setenv("PGPASSWORD", v) })
	}
	prima := testutil.FotoDelDatabase(t, p)

	s := preparaBanco(t, mutaBanco{})
	o := Opzioni{Modalita: ModalitaDSN, Dataset: s.manifest, Uscita: s.uscita, DSN: dsn, Tutti: true, Migrazioni: cockpit.FS}
	var out bytes.Buffer
	r, err := EseguiBanco(context.Background(), o, &out)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := controlloBanco(r, "sola_lettura")
	if !ok || c.Stato != ControlloNonEseguito || r.Esito.CodiceUscita() != UscitaNonEseguito {
		t.Fatalf("il ruolo che scrive ferma la corsa: %+v, %s\n%s", c, r.PrimaRiga(), out.String())
	}
	if r.SolaLettura == nil || r.SolaLettura.Ruolo == "" || !r.SolaLettura.ScritturaPossibile {
		t.Errorf("il rapporto dice il ruolo e che può scrivere: %+v", r.SolaLettura)
	}
	// I controlli si fermano ai privilegi di scrittura, prima delle tabelle escluse: non sono state controllate, e il
	// rapporto non dice che sono tutte illeggibili (R-107).
	if r.SolaLettura != nil && r.SolaLettura.EscluseNonLeggibili != nil {
		t.Errorf("tabelle escluse non controllate, ma contate: %d", *r.SolaLettura.EscluseNonLeggibili)
	}
	if !strings.Contains(r.Testo(), "tabelle escluse non leggibili: non controllate") {
		t.Errorf("il riepilogo: %s", r.Testo())
	}
	if r.Fotografia != nil || len(r.Thread) != 0 {
		t.Error("senza i controlli di sola lettura non c'è la fotografia")
	}
	if !strings.HasPrefix(out.String(), "sorgente: ") || strings.Contains(out.String(), "collegato in sola lettura") {
		t.Errorf("a video: %q", out.String())
	}
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Fatalf("il banco ha cambiato il database: %v", d)
	}
}
