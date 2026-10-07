package bancoa

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
)

// L1 — le modalità dsn ed exports dall'ingresso al rapporto (piano 6.4.9; T-B6-03, T-B6-04, F0-01, F0-16), senza DB:
//   - le cinque condizioni di F0-01: un'opzione dsn senza DSN è un errore d'uso; exports senza la cartella pure; -dsn con
//     la modalità casi pure; il rapporto dei modi nuovi lo scrive una funzione nuova; Esegui non esegue i modi nuovi;
//   - le altre regole d'uso: -dsn ed -exports si escludono, -tutti solo con -dsn, -thread o -tutti con -dsn, il DSN senza
//     password (URL e chiave=valore) e senza ripeterlo, PGPASSWORD rifiutata, -gate con -attesi, le migrazioni
//     incorporate con -dsn;
//   - la sequenza -exports sulla scena ACME: conforme senza gli attesi; i thread scelti; un thread scelto che non c'è,
//     un manifest assente o un file cambiato: NON ESEGUITO (3); un export che non ha la forma attesa: con differenze (1);
//   - la sequenza -dsn fino al collegamento: una copia che non si raggiunge dà NON ESEGUITO (3), con il rapporto.
//
// I clienti di questi test sono inventati: vedi scena_banco_test.go.

func TestEseguiBancoErroriDUso(t *testing.T) {
	s := preparaBanco(t, mutaBanco{})
	mig := fstest.MapFS{"migrations/0001_acme.sql": &fstest.MapFile{Data: []byte("select 1;")}}
	dsn := Opzioni{Modalita: ModalitaDSN, Dataset: s.manifest, Uscita: s.uscita, DSN: "postgres://acme_banco@127.0.0.1:1/acme_prova",
		Tutti: true, Migrazioni: mig}
	exp := s.opzioniExport(false)
	casi := map[string]Opzioni{
		"(a) dsn senza DSN":           func() Opzioni { o := dsn; o.DSN = ""; return o }(),
		"(b) exports senza cartella":  func() Opzioni { o := exp; o.Exports = ""; return o }(),
		"-dsn ed -exports":            func() Opzioni { o := dsn; o.Exports = s.export; return o }(),
		"-tutti con exports":          func() Opzioni { o := exp; o.Tutti = true; return o }(),
		"dsn senza -thread né -tutti": func() Opzioni { o := dsn; o.Tutti = false; return o }(),
		"-thread e -tutti":            func() Opzioni { o := dsn; o.Thread = []uuid.UUID{threadScenario}; return o }(),
		"DSN con la password (URL)": func() Opzioni {
			o := dsn
			o.DSN = "postgres://acme_banco:segreto-acme@127.0.0.1:1/acme_prova"
			return o
		}(),
		"DSN con la password (param)": func() Opzioni {
			o := dsn
			o.DSN = "postgres://acme_banco@127.0.0.1:1/acme_prova?password=segreto-acme"
			return o
		}(),
		"DSN con la password (chiave)": func() Opzioni {
			o := dsn
			o.DSN = "host=127.0.0.1 port=1 password = segreto-acme dbname=acme_prova"
			return o
		}(),
		"senza migrazioni incorporate":    func() Opzioni { o := dsn; o.Migrazioni = nil; return o }(),
		"-gate senza -attesi":             func() Opzioni { o := exp; o.Gate = true; return o }(),
		"senza dataset":                   func() Opzioni { o := exp; o.Dataset = ""; return o }(),
		"uscita dentro il modulo":         func() Opzioni { o := exp; o.Uscita = filepath.Join("testdata", "uscita-non-ammessa"); return o }(),
		"export dentro il modulo":         func() Opzioni { o := exp; o.Exports = "testdata"; return o }(),
		"modalità di A1a con EseguiBanco": func() Opzioni { o := exp; o.Modalita = ModalitaCasi; return o }(),
	}
	for nome, o := range casi {
		t.Run(nome, func(t *testing.T) {
			r, err := EseguiBanco(context.Background(), o, nil)
			var uso *ErroreUso
			if !errors.As(err, &uso) || r.Esito != "" {
				t.Fatalf("errore %v, esito %q", err, r.Esito)
			}
			if strings.Contains(err.Error(), "segreto-acme") {
				t.Errorf("l'errore ripete il DSN: %v", err)
			}
		})
	}
	if _, err := os.Stat(s.uscita); !os.IsNotExist(err) {
		t.Fatalf("un errore d'uso ha scritto la cartella dei rapporti: %v", err)
	}

	// PGPASSWORD impostata: il banco non parte (R32 c).
	t.Setenv("PGPASSWORD", "segreto-acme")
	if _, err := EseguiBanco(context.Background(), dsn, nil); err == nil || strings.Contains(err.Error(), "segreto-acme") {
		t.Errorf("PGPASSWORD impostata: %v", err)
	}
}

// TestEseguiNonEsegueIModiDiA1c (F0-01 c, e): Esegui resta quello di A1a: le modalità dsn ed exports e i flag di A1c con
// regole o casi sono errori d'uso, senza rapporto.
func TestEseguiNonEsegueIModiDiA1c(t *testing.T) {
	d := preparaDataset(t, nil)
	fuori := filepath.Join(d.dir, "banco")
	for nome, o := range map[string]Opzioni{
		"dsn con Esegui":     {Modalita: ModalitaDSN, Dataset: d.manifest, Uscita: fuori, DSN: "postgres://acme@127.0.0.1:1/acme_prova"},
		"exports con Esegui": {Modalita: ModalitaExports, Dataset: d.manifest, Uscita: fuori, Exports: d.dir},
		"(c) -dsn con casi":  {Modalita: ModalitaCasi, Dataset: d.manifest, Uscita: fuori, DSN: "postgres://acme@127.0.0.1:1/acme_prova"},
		"-attesi con regole": {Modalita: ModalitaRegole, Dataset: d.manifest, Uscita: fuori, Attesi: true},
		"-thread con casi":   {Modalita: ModalitaCasi, Dataset: d.manifest, Uscita: fuori, Thread: []uuid.UUID{threadScenario}},
	} {
		t.Run(nome, func(t *testing.T) {
			r, err := Esegui(context.Background(), o, nil)
			var uso *ErroreUso
			if !errors.As(err, &uso) || r.Esito != "" {
				t.Fatalf("errore %v, esito %q", err, r.Esito)
			}
		})
	}
	if _, err := os.Stat(fuori); !os.IsNotExist(err) {
		t.Fatalf("un errore d'uso ha scritto la cartella dei rapporti: %v", err)
	}
}

func TestEseguiBancoExportSenzaAttesi(t *testing.T) {
	s := preparaBanco(t, mutaBanco{})
	var out bytes.Buffer
	r, err := EseguiBanco(context.Background(), s.opzioniExport(false), &out)
	if err != nil {
		t.Fatal(err)
	}
	if r.Esito != EsitoConforme || r.Esito.CodiceUscita() != UscitaConforme || r.VersioneRapporto != VersioneRapportoBanco || r.Modalita != ModalitaExports {
		t.Fatalf("%s\n%s", r.PrimaRiga(), out.String())
	}
	if r.Gate != nil || r.Attesi != nil || r.Casi != nil {
		t.Error("senza -attesi niente esiti, attesi e gate: il rapporto ha il vecchio e il nuovo")
	}
	if _, ok := controlloBanco(r, ControlloScenario); ok {
		t.Error("senza -attesi i controlli del runner non ci sono")
	}
	if len(r.Thread) != 2 || r.Valutazione == nil || r.Valutazione.ThreadValutati != 2 || len(r.Valutazione.Impronta) != 64 {
		t.Fatalf("thread %d, valutazione %+v", len(r.Thread), r.Valutazione)
	}
	for _, th := range r.Thread {
		if len(th.ImprontaConfronto) != 64 || len(th.File) == 0 {
			t.Errorf("thread %s: impronta del confronto %q, file %d", th.ThreadID, th.ImprontaConfronto, len(th.File))
		}
		for _, f := range th.File {
			if f.NomeFile == "" || f.Sha256 == "" || len(f.Esiti) != 0 {
				t.Errorf("file %s: %+v", f.AllegatoID, f)
			}
		}
	}
	if r.SolaLettura != nil || !strings.HasSuffix(r.Scritture, "nessun database aperto") {
		t.Errorf("con gli export nessun database: %+v %q", r.SolaLettura, r.Scritture)
	}
}

func TestEseguiBancoExportThreadScelti(t *testing.T) {
	s := preparaBanco(t, mutaBanco{})
	o := s.opzioniExport(true)
	o.Thread = []uuid.UUID{threadDecisioni}
	r, err := EseguiBanco(context.Background(), o, nil)
	if err != nil || len(r.Thread) != 1 || r.Thread[0].ThreadID != threadDecisioni {
		t.Fatalf("thread scelti: %+v (%v)", r.Thread, err)
	}
	// Con i soli thread scelti le voci degli altri thread non si risolvono: non verificabili, mai differenze.
	if c, _ := controlloBanco(r, ControlloRisoluzione); c.Stato != ControlloNonEseguito || c.Differenze != 0 {
		t.Errorf("n.2 con i thread scelti: %+v", c)
	}
	o.Thread = []uuid.UUID{uidACME(0x79)}
	r, err = EseguiBanco(context.Background(), o, nil)
	if c, _ := controlloBanco(r, "thread"); err != nil || c.Stato != ControlloNonEseguito || r.Esito.CodiceUscita() != UscitaNonEseguito {
		t.Fatalf("un thread scelto che non è negli export: %+v, %s (%v)", c, r.PrimaRiga(), err)
	}
}

func TestEseguiBancoNonEseguitoEConDifferenze(t *testing.T) {
	// Il manifest che manca: il rapporto c'è, con il motivo in testa.
	s := preparaBanco(t, mutaBanco{})
	if err := os.Remove(s.manifest); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	r, err := EseguiBanco(context.Background(), s.opzioniExport(true), &out)
	if err != nil || r.Esito.CodiceUscita() != UscitaNonEseguito || !strings.HasPrefix(r.PrimaRiga(), "ESITO: NON ESEGUITO — manifest: ") {
		t.Fatalf("manifest assente: %s (%v)", r.PrimaRiga(), err)
	}
	if txt, _ := os.ReadFile(filepath.Join(s.uscita, "rapporto-exports.txt")); !strings.HasPrefix(string(txt), "ESITO: NON ESEGUITO —") {
		t.Fatalf("rapporto del non eseguito: %q", txt)
	}
	if !strings.HasPrefix(out.String(), "sorgente: export ") || !strings.Contains(out.String(), "rapporto: ") {
		t.Errorf("a video: %q", out.String())
	}

	// Un export cambiato dopo il manifest: NON ESEGUITO.
	s = preparaBanco(t, mutaBanco{})
	writeFile(t, filepath.Join(s.export, nomeFileExport(ExportTriage)), "{}")
	r, err = EseguiBanco(context.Background(), s.opzioniExport(false), nil)
	if c, _ := controlloBanco(r, "export"); err != nil || c.Stato != ControlloNonEseguito || r.Esito.CodiceUscita() != UscitaNonEseguito {
		t.Fatalf("export cambiato: %+v, %s (%v)", c, r.PrimaRiga(), err)
	}

	// Un messaggio di un caso di censimento che gli export non hanno: la fotografia c'è, il censimento è parziale, e la
	// parte che manca è NON ESEGUITA nel controllo dei casi (gli export hanno solo i messaggi in entrata: T-B6-112).
	s = preparaBanco(t, mutaBanco{testi: map[string]func(string) string{"casi": func(c string) string {
		return strings.Replace(c, msgFuori2.String(), msgNonEsportato.String(), 1)
	}}})
	r, err = EseguiBanco(context.Background(), s.opzioniExport(false), nil)
	if c, _ := controlloBanco(r, ControlloCasiFoto); err != nil || c.Stato != ControlloNonEseguito || c.Differenze != 0 || r.Esito.CodiceUscita() != UscitaNonEseguito {
		t.Fatalf("messaggio del caso non esportato: %+v, %s (%v)", c, r.PrimaRiga(), err)
	}
	// valutazione dà per il messaggio che la fotografia non ha un record non valutato (T-B6-26): il censimento lo mostra.
	if len(r.Censimento) != 2 || r.Censimento[1].MessaggioID != msgNonEsportato || r.Censimento[1].Valutato {
		t.Fatalf("censimento: %+v", r.Censimento)
	}

	// Un export con l'impronta giusta e una colonna che manca: una differenza (uscita 1).
	s = preparaBanco(t, mutaBanco{righe: func(r map[string][]riga) { delete(r[ExportThread][0], "creato_il") }})
	r, err = EseguiBanco(context.Background(), s.opzioniExport(false), nil)
	if c, _ := controlloBanco(r, "export"); err != nil || c.Differenze != 1 || r.Esito.CodiceUscita() != UscitaConDifferenze {
		t.Fatalf("export non valido: %+v, %s (%v)", c, r.PrimaRiga(), err)
	}
}

// TestEseguiBancoDSNNonRaggiungibile: la sequenza -dsn fino al collegamento, senza un DB: la riga della sorgente senza
// password prima di collegarsi, poi la copia che non si raggiunge: NON ESEGUITO (3), con il rapporto e senza la prima
// scrittura (la lettura non è cominciata). Nessun DB si apre: la porta 1 di 127.0.0.1 rifiuta il collegamento.
func TestEseguiBancoDSNNonRaggiungibile(t *testing.T) {
	if v, ok := os.LookupEnv("PGPASSWORD"); ok {
		os.Unsetenv("PGPASSWORD")
		t.Cleanup(func() { os.Setenv("PGPASSWORD", v) })
	}
	s := preparaBanco(t, mutaBanco{})
	mig := fstest.MapFS{"migrations/0001_acme.sql": &fstest.MapFile{Data: []byte("select 1;")}}
	o := Opzioni{Modalita: ModalitaDSN, Dataset: s.manifest, Uscita: s.uscita, Tutti: true, Migrazioni: mig,
		DSN: "postgres://acme_banco@127.0.0.1:1/acme_prova?connect_timeout=2"}
	var out bytes.Buffer
	r, err := EseguiBanco(context.Background(), o, &out)
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := controlloBanco(r, "copia"); c.Stato != ControlloNonEseguito || r.Esito.CodiceUscita() != UscitaNonEseguito {
		t.Fatalf("copia non raggiungibile: %+v, %s", c, r.PrimaRiga())
	}
	if !strings.HasPrefix(out.String(), "sorgente: 127.0.0.1:1/acme_prova come acme_banco (banco, sola lettura)\n") {
		t.Errorf("la prima riga a video: %q", out.String())
	}
	if strings.Contains(out.String(), "collegato in sola lettura") || r.Fotografia != nil {
		t.Error("senza collegamento non c'è la riga del collegamento, né la fotografia")
	}
	if !strings.HasSuffix(r.Scritture, "nessuna scrittura sul database") {
		t.Errorf("scritture: %q", r.Scritture)
	}
	if _, err := os.Stat(filepath.Join(s.uscita, "rapporto-dsn.json")); err != nil {
		t.Errorf("rapporto: %v", err)
	}
}
