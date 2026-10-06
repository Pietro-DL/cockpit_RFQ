package testutil

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/dataset"
)

// L1 — la guardia sul database di test si prova senza database: dice di no PRIMA di connettersi, ed
// e' proprio quello che deve fare. Un DSN che porta a un database di sviluppo non deve mai arrivare a
// SchemaVuoto, che distrugge lo schema.
func TestLaGuardiaLeggeIlNomeDelDatabaseComeLoLeggePgx(t *testing.T) {
	// PGDATABASE fissato qui, perche' il caso «senza database» sotto dipenda solo da questa prova e
	// non dall'ambiente di chi la lancia
	t.Setenv("PGDATABASE", "cockpit_dev")
	casi := []struct {
		nome   string
		dsn    string
		valido bool
	}{
		{"URL verso un database di test", "postgres://cockpit_test:cockpit_test@127.0.0.1:5433/cockpit_test_fx", true},
		{"chiave=valore verso un database di test", "host=127.0.0.1 port=5433 user=cockpit dbname=cockpit_test", true},
		{"URL verso lo sviluppo", "postgres://cockpit:x@127.0.0.1:5432/cockpit_dev", false},
		// il caso che passava: nessun percorso, e «tester» nel nome dell'utente sembrava un test
		{"chiave=valore verso lo sviluppo, utente «tester»", "host=x user=tester dbname=cockpit_dev", false},
		// il percorso dice test, il parametro dice altro: vince il parametro, come in pgx
		{"URL con ?dbname= che porta altrove", "postgres://u:p@127.0.0.1:5433/cockpit_test?dbname=cockpit_dev", false},
		// nessun nome nel DSN: decide l'ambiente (qui PGDATABASE=cockpit_dev), non il testo
		{"URL senza database", "postgres://tester:p@127.0.0.1:5433/", false},
		{"DSN illeggibile", "postgres://%zz", false},
	}
	for _, c := range casi {
		err := DatabaseDiTest(c.dsn)
		if c.valido && err != nil {
			t.Errorf("%s: rifiutato (%v), doveva passare", c.nome, err)
		}
		if !c.valido && err == nil {
			t.Errorf("%s: accettato %q — i test distruggerebbero lo schema di un database che non e' di test", c.nome, c.dsn)
		}
	}
}

// L1 — la guardia di SchemaVuoto sul collegamento (A1c-L1-02, la parte della guardia; piano A, 6.4.7): prima del
// DROP il database collegato deve avere «test» nel nome e l'utente non deve essere il ruolo del banco. I nomi di
// database e ruoli sono inventati.
func TestLaGuardiaDiSchemaVuotoGuardaIlCollegamento(t *testing.T) {
	for _, c := range []struct {
		nome                         string
		database, utente, ruoloBanco string
		ammesso                      bool
	}{
		{"database di prova, nessun ruolo del banco dichiarato", "acme_prova_test", "prove_acme", "", true},
		{"database di prova, utente diverso dal ruolo del banco", "acme_prova_test", "prove_acme", "lettore_acme", true},
		{"maiuscole nel nome", "ACME_Prova_TEST", "prove_acme", "", true},
		{"database senza «test»", "acme_copia_intatta", "prove_acme", "", false},
		{"database di sviluppo", "acme_dev", "prove_acme", "lettore_acme", false},
		{"il ruolo del banco su un database di prova", "acme_prova_test", "lettore_acme", "lettore_acme", false},
	} {
		err := schemaVuotoAmmesso(c.database, c.utente, c.ruoloBanco)
		if c.ammesso && err != nil {
			t.Errorf("%s: rifiutato (%v)", c.nome, err)
		}
		if !c.ammesso && err == nil {
			t.Errorf("%s: ammesso, e lo schema di %q sarebbe distrutto", c.nome, c.database)
		}
	}
}

// L1 — il ruolo del banco viene dalla variabile della copia del dump, letto come lo legge pgx; senza la variabile,
// o con un DSN che non si legge, non c'è (resta la guardia sul nome).
func TestIlRuoloDelBancoVieneDallaVariabileDelDump(t *testing.T) {
	t.Setenv("COCKPIT_DUMP_DSN", "postgres://lettore_acme@127.0.0.1:5432/acme_copia")
	if r := ruoloDelBanco(); r != "lettore_acme" {
		t.Errorf("ruolo del banco: %q", r)
	}
	t.Setenv("COCKPIT_DUMP_DSN", "host=127.0.0.1 user=lettore_kv dbname=acme_copia")
	if r := ruoloDelBanco(); r != "lettore_kv" {
		t.Errorf("ruolo del banco da un DSN chiave=valore: %q", r)
	}
	t.Setenv("COCKPIT_DUMP_DSN", "")
	if r := ruoloDelBanco(); r != "" {
		t.Errorf("senza la variabile il ruolo del banco è %q", r)
	}
	t.Setenv("COCKPIT_DUMP_DSN", "postgres://%zz")
	if r := ruoloDelBanco(); r != "" {
		t.Errorf("con un DSN che non si legge il ruolo del banco è %q", r)
	}
}

// L1 — dove le prove possono scrivere i suggerimenti dell'agente (A1c-L4S-11, la parte pura; R10): il database
// di prova o una copia usa e getta con il marcatore; mai un database senza l'uno e senza l'altro.
func TestDoveSiScrivonoLeRigheDiProva(t *testing.T) {
	for _, c := range []struct {
		nome, database, commento string
		ammesso                  bool
	}{
		{"database di prova", "acme_prova_test", "", true},
		{"copia usa e getta", "acme_copia_run", marcatoreCopiaUsaEGetta + "acme_copia (2026-10-06 08:00)", true},
		{"copia intatta: niente «test», niente marcatore", "acme_copia", "", false},
		{"commento qualunque", "acme_copia", "copia di lavoro", false},
		{"marcatore non all'inizio", "acme_copia", "nota: " + marcatoreCopiaUsaEGetta + "acme_copia", false},
	} {
		err := scritturaDiProvaAmmessa(c.database, c.commento)
		if c.ammesso != (err == nil) {
			t.Errorf("%s: ammesso=%v, errore %v", c.nome, c.ammesso, err)
		}
	}
}

// L1 — i testi SQL vietati a una lettura del motore A (A1c-L4S-02, -08: il registro SQL): le tabelle escluse, le
// scritture, i lucchetti; le letture passano, anche quando una colonna contiene una parola simile.
func TestIlRegistroSQLRiconosceITestiVietati(t *testing.T) {
	r := &RegistroSQL{}
	vietati := []string{
		"INSERT INTO utente (sigla) VALUES ($1)",
		"update documento_proposta set stato = stato where false",
		"DELETE FROM job",
		"TRUNCATE componente",
		"COPY allegato TO STDOUT",
		"LOCK TABLE documento",
		"SELECT pg_advisory_xact_lock(1)",
		"SELECT * FROM documento WHERE thread_id = $1 FOR UPDATE",
		"SELECT * FROM documento FOR NO KEY UPDATE",
		"SELECT * FROM documento FOR SHARE",
		"SELECT * FROM documento FOR KEY SHARE",
		"SELECT * FROM " + "analisi" + "_messaggio",
		"SELECT * FROM worker_credenziale",
		"SELECT token FROM sessione",
	}
	ammessi := []string{
		"begin isolation level repeatable read read only",
		"SHOW transaction_read_only",
		"SELECT now()",
		"SELECT aggiornato_il, ultimo_aggiornamento FROM thread_offerta",
		"SELECT * FROM v_fascicolo WHERE thread_id = $1 ORDER BY codice, tipo_documento",
		"rollback",
	}
	for _, s := range append(append([]string(nil), vietati...), ammessi...) {
		r.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: s})
	}
	r.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: vietati[0]}) // ripetuto: una volta sola
	got := r.Vietati()
	if strings.Join(got, "\n") != strings.Join(vietati, "\n") {
		t.Errorf("vietati:\n%s\natteso:\n%s", strings.Join(got, "\n"), strings.Join(vietati, "\n"))
	}
	if n := len(r.Voci()); n != len(vietati)+len(ammessi)+1 {
		t.Errorf("voci registrate: %d", n)
	}
	r.Azzera()
	if len(r.Voci()) != 0 || len(r.Vietati()) != 0 {
		t.Error("Azzera non ha dimenticato le voci")
	}
}

// L1 — le differenze fra due foto del database: valori diversi, tabelle in una sola foto, in ordine.
func TestDifferenzeFraDueFoto(t *testing.T) {
	prima := map[string]string{"allegato": "2 aa", "componente": "1 bb", "job": "0 -"}
	dopo := map[string]string{"allegato": "2 aa", "componente": "2 cc", "sessione": "1"}
	if got := strings.Join(Differenze(prima, dopo), ","); got != "componente,job,sessione" {
		t.Errorf("differenze: %q", got)
	}
	if d := Differenze(prima, prima); len(d) != 0 {
		t.Errorf("la stessa foto ha differenze: %v", d)
	}
}

// tbFinto: un testing.TB che registra Fatalf e Skipf invece di fermare la prova vera, e chiude la goroutine come
// farebbero FailNow e SkipNow. Il resto (Cleanup, TempDir, Setenv) va alla prova vera.
type tbFinto struct {
	testing.TB
	fatale, saltato string
}

func (f *tbFinto) Helper() {}

func (f *tbFinto) Fatalf(format string, args ...any) {
	f.fatale = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

func (f *tbFinto) Fatal(args ...any) {
	f.fatale = fmt.Sprint(args...)
	runtime.Goexit()
}

func (f *tbFinto) FailNow() {
	f.fatale = "FailNow"
	runtime.Goexit()
}

func (f *tbFinto) Skipf(format string, args ...any) {
	f.saltato = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

func (f *tbFinto) Skip(args ...any) {
	f.saltato = fmt.Sprint(args...)
	runtime.Goexit()
}

func (f *tbFinto) SkipNow() {
	f.saltato = "SkipNow"
	runtime.Goexit()
}

// conTBFinto esegue corpo con un tbFinto, in una goroutine sua, e dice come è finita.
func conTBFinto(t *testing.T, corpo func(tb testing.TB)) (fatale, saltato string) {
	t.Helper()
	f := &tbFinto{TB: t}
	fatto := make(chan struct{})
	go func() {
		defer close(fatto)
		corpo(f)
	}()
	<-fatto
	return f.fatale, f.saltato
}

// L1 — NON ESEGUITA (A1c-L1-02; R44): ferma la prova con il prefisso «NON ESEGUITA:» e il motivo, con un FAIL,
// mai con uno SKIP.
func TestNonEseguitaFallisceEMaiSalta(t *testing.T) {
	fatale, saltato := conTBFinto(t, func(tb testing.TB) {
		NonEseguita(tb, "manca la copia inventata: darla con la variabile")
		tb.Errorf("dopo NonEseguita la prova è andata avanti")
	})
	if fatale != "NON ESEGUITA: manca la copia inventata: darla con la variabile" {
		t.Errorf("messaggio: %q", fatale)
	}
	if saltato != "" {
		t.Errorf("NonEseguita ha saltato la prova: %q", saltato)
	}
}

// L1 — le risorse d'ambiente (piano A, 3.7.3; R44): una risorsa mancante salta con «SALTATO-AMBIENTE» nelle corse
// di sviluppo, ed è NON ESEGUITA quando COCKPIT_PROVE_OBBLIGATORIE la dichiara (spazi e maiuscole non contano).
func TestRichiestoSaltaONonEsegue(t *testing.T) {
	for _, c := range []struct {
		nome, obbligatorie, risorsa string
		nonEseguita                 bool
	}{
		{"corsa di sviluppo", "", "L4", false},
		{"un'altra risorsa obbligatoria", "PYTHON", "L4", false},
		{"obbligatoria", "L4", "L4", true},
		{"obbligatoria, con spazi e maiuscole", " python , l4 ", "L4", true},
	} {
		t.Setenv(variabileObbligatorie, c.obbligatorie)
		fatale, saltato := conTBFinto(t, func(tb testing.TB) { Richiesto(tb, c.risorsa, "il DB di prova inventato non c'è") })
		if c.nonEseguita {
			if !strings.HasPrefix(fatale, "NON ESEGUITA: "+c.risorsa+": il DB di prova inventato non c'è") || saltato != "" {
				t.Errorf("%s: fatale %q, saltato %q", c.nome, fatale, saltato)
			}
			continue
		}
		if saltato != "SALTATO-AMBIENTE: "+c.risorsa+": il DB di prova inventato non c'è" || fatale != "" {
			t.Errorf("%s: fatale %q, saltato %q", c.nome, fatale, saltato)
		}
	}
}

// manifestDiProva scrive in una cartella temporanea (fuori dal modulo) un manifest sintetico con una voce degli
// attesi e una delle regole, e ne restituisce il percorso. Nomi di file inventati, con «_acme».
func manifestDiProva(t *testing.T) (string, []byte) {
	t.Helper()
	dir := t.TempDir()
	attesi := []byte("versione_attesi: 1\n")
	regole := []byte(`{"versione": 1}`)
	for nome, b := range map[string][]byte{"attesi_acme.yaml": attesi, "indice_acme.v1.json": regole} {
		if err := os.WriteFile(filepath.Join(dir, nome), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sha := func(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
	m := fmt.Sprintf(`{"versione_manifest": 1, "voci": [
  {"nome": "attesi", "percorso": "attesi_acme.yaml", "ruolo": "attesi", "sha256": %q, "byte": %d},
  {"nome": "regole.indice", "percorso": "indice_acme.v1.json", "ruolo": "regole", "sha256": %q, "byte": %d}
], "copia": {}}`, sha(attesi), len(attesi), sha(regole), len(regole))
	percorso := filepath.Join(dir, "manifest_acme.json")
	if err := os.WriteFile(percorso, []byte(m), 0o644); err != nil {
		t.Fatal(err)
	}
	return percorso, regole
}

// L1 — il dataset privato nelle prove (A1c-L1-02; P-11, R44): FileDelDataset dà i byte di una voce controllata,
// rifiuta la voce degli attesi con un fallimento della prova (non manca niente: la prova chiede ciò che non può
// avere), e senza la variabile, con una voce che manca, con un file cambiato o con un manifest dentro il modulo la
// prova è NON ESEGUITA.
func TestFileDelDatasetRifiutaGliAttesi(t *testing.T) {
	percorso, regole := manifestDiProva(t)
	t.Setenv(variabileDataset, percorso)

	var letto []byte
	fatale, _ := conTBFinto(t, func(tb testing.TB) { letto = FileDelDataset(tb, "regole.indice") })
	if fatale != "" || string(letto) != string(regole) {
		t.Errorf("voce delle regole: %q, %q", letto, fatale)
	}

	fatale, saltato := conTBFinto(t, func(tb testing.TB) { FileDelDataset(tb, "attesi") })
	if !strings.Contains(fatale, "gli attesi li legge solo il runner") || strings.HasPrefix(fatale, "NON ESEGUITA") || saltato != "" {
		t.Errorf("voce degli attesi: fatale %q, saltato %q: atteso un fallimento della prova, non una NON ESEGUITA", fatale, saltato)
	}
	if _, err := fileDelManifest(dataset.Manifest{Voci: []dataset.Voce{{Nome: "attesi", Ruolo: dataset.RuoloAttesi}}}, "attesi"); err != errAttesiSoloAlRunner {
		t.Errorf("parte pura, voce degli attesi: %v", err)
	}

	for _, c := range []struct {
		nome, voce, frase string
		prepara           func(t *testing.T)
	}{
		{"voce che il manifest non ha", "regole.altre", "regole.altre", nil},
		{"file cambiato", "regole.indice", "sha256", func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(filepath.Dir(percorso), "indice_acme.v1.json"), []byte(`{"versione": 2}`), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"senza la variabile", "regole.indice", variabileDataset + " non impostata", func(t *testing.T) { t.Setenv(variabileDataset, "") }},
		{"manifest dentro il modulo", "regole.indice", "dentro il modulo", func(t *testing.T) { t.Setenv(variabileDataset, "manifest_acme.json") }},
	} {
		t.Run(c.nome, func(t *testing.T) {
			if c.prepara != nil {
				c.prepara(t)
			}
			fatale, saltato := conTBFinto(t, func(tb testing.TB) { FileDelDataset(tb, c.voce) })
			if !strings.HasPrefix(fatale, "NON ESEGUITA: ") || !strings.Contains(fatale, c.frase) || saltato != "" {
				t.Errorf("fatale %q, saltato %q; attesa una NON ESEGUITA che dica %q", fatale, saltato, c.frase)
			}
		})
	}
}

// L1 — la cartella delle uscite delle prove private (piano A, 3.7.3): senza la variabile una cartella
// temporanea; con la variabile, fuori dal modulo, quella cartella, creata; dentro il modulo, NON ESEGUITA.
func TestCartellaRapportiFuoriDalModulo(t *testing.T) {
	t.Setenv(variabileRapporti, "")
	var c string
	if fatale, _ := conTBFinto(t, func(tb testing.TB) { c = CartellaRapporti(tb) }); fatale != "" || c == "" {
		t.Errorf("senza la variabile: %q, %q", c, fatale)
	}
	fuori := filepath.Join(t.TempDir(), "rapporti_acme")
	t.Setenv(variabileRapporti, fuori)
	if fatale, _ := conTBFinto(t, func(tb testing.TB) { c = CartellaRapporti(tb) }); fatale != "" || c != fuori {
		t.Errorf("fuori dal modulo: %q, %q", c, fatale)
	}
	if fi, err := os.Stat(fuori); err != nil || !fi.IsDir() {
		t.Errorf("la cartella non è stata creata: %v", err)
	}
	t.Setenv(variabileRapporti, "cartella_acme_rapporti")
	if fatale, _ := conTBFinto(t, func(tb testing.TB) { CartellaRapporti(tb) }); !strings.HasPrefix(fatale, "NON ESEGUITA: ") {
		t.Errorf("dentro il modulo: %q", fatale)
	}
	if _, err := os.Stat("cartella_acme_rapporti"); err == nil {
		t.Error("una cartella dentro il modulo è stata creata")
	}
}

// L1 — il DB di prova come risorsa d'ambiente (piano A, 3.7.3, 6.7.0 C-L4S e 6.7.3; R44): senza COCKPIT_TEST_DSN, DSN
// passa da Richiesto; nelle corse di sviluppo la prova salta con «SALTATO-AMBIENTE: L4: …», con
// COCKPIT_PROVE_OBBLIGATORIE=L4 è NON ESEGUITA, mai verde. Con la variabile, DSN la restituisce, e un database
// senza «test» nel nome resta un fallimento della prova, non una NON ESEGUITA. I nomi sono inventati.
func TestDSNPassaDaRichiesto(t *testing.T) {
	const motivo = "L4: COCKPIT_TEST_DSN non impostata: test d'integrazione saltato"
	t.Setenv("COCKPIT_TEST_DSN", "")

	t.Setenv(variabileObbligatorie, "")
	fatale, saltato := conTBFinto(t, func(tb testing.TB) {
		DSN(tb)
		tb.Errorf("dopo il salto la prova è andata avanti")
	})
	if saltato != "SALTATO-AMBIENTE: "+motivo || fatale != "" {
		t.Errorf("corsa di sviluppo: saltato %q, fatale %q", saltato, fatale)
	}

	t.Setenv(variabileObbligatorie, "L4")
	fatale, saltato = conTBFinto(t, func(tb testing.TB) { DSN(tb) })
	if !strings.HasPrefix(fatale, "NON ESEGUITA: "+motivo) || saltato != "" {
		t.Errorf("con L4 obbligatoria: fatale %q, saltato %q", fatale, saltato)
	}

	t.Setenv("COCKPIT_TEST_DSN", "postgres://prove_acme@127.0.0.1:5432/acme_prova_test")
	var dsn string
	if fatale, saltato = conTBFinto(t, func(tb testing.TB) { dsn = DSN(tb) }); fatale != "" || saltato != "" ||
		dsn != "postgres://prove_acme@127.0.0.1:5432/acme_prova_test" {
		t.Errorf("con la variabile: %q, fatale %q, saltato %q", dsn, fatale, saltato)
	}
	t.Setenv("COCKPIT_TEST_DSN", "postgres://prove_acme@127.0.0.1:5432/acme_dev")
	if fatale, _ = conTBFinto(t, func(tb testing.TB) { DSN(tb) }); !strings.HasPrefix(fatale, "COCKPIT_TEST_DSN: ") {
		t.Errorf("un database senza «test»: %q, atteso il fallimento della guardia", fatale)
	}
}
