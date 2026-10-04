package bancoa

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"promatec/cockpit/internal/platform/jsoncanonico"
)

// L1 — il rapporto e gli esiti di Esegui (A1a-BA; R44): la versione del rapporto fissa; esito e codice
// d'uscita (0 conforme, 1 con differenze, 3 non eseguito; una differenza prevale su un non eseguito); la
// prima riga «ESITO: …»; la scrittura atomica fuori dal modulo, in JSON canonico e in testo; l'elenco dei
// controlli, ciascuno eseguito o non eseguito con il motivo; un errore d'uso non scrive nessun rapporto. La
// parte sui flag e sui codici d'uscita del comando sta in cmd/bancoa.
//
// I clienti di questi test sono inventati. Non è pigrizia: il dataset vero è privato, e questo repository è
// pubblico. Il dataset ACME nasce in t.TempDir(), fuori dal modulo.

// TestVersioneRapportoFissa: riscritta per A1b.11 (la versione passa a 2: nei casi le letture hanno la funzione
// del router e l'esito porta gli attributi di Interpreta).
func TestVersioneRapportoFissa(t *testing.T) {
	if VersioneRapporto != 2 {
		t.Fatalf("VersioneRapporto = %d: cambiarla vuol dire riscrivere questa prova («Riscritta per …»)", VersioneRapporto)
	}
}

func TestEsitoECodiceDUscita(t *testing.T) {
	for e, codice := range map[Esito]int{EsitoConforme: 0, EsitoConDifferenze: 1, EsitoNonEseguito: 3, "": 3, "inventato": 3} {
		if e.CodiceUscita() != codice {
			t.Errorf("%q → %d, atteso %d", e, e.CodiceUscita(), codice)
		}
	}
	righe := map[string]Rapporto{
		"ESITO: ESEGUITO — conforme":                  {Esito: EsitoConforme},
		"ESITO: ESEGUITO — con differenze (2)":        {Esito: EsitoConDifferenze, Differenze: 2},
		"ESITO: NON ESEGUITO — attesi: file mancante": {Esito: EsitoNonEseguito, Motivo: "attesi: file mancante"},
	}
	for riga, r := range righe {
		if r.PrimaRiga() != riga {
			t.Errorf("prima riga %q, attesa %q", r.PrimaRiga(), riga)
		}
	}
}

func TestScriviRapportoAtomicoFuoriDalModulo(t *testing.T) {
	r := Rapporto{VersioneRapporto: 1, Modalita: ModalitaCasi, Esito: EsitoConforme, Manifest: "manifest_acme.json",
		Controlli: []Controllo{{Nome: "manifest", Stato: ControlloEseguito}}}
	cartella := filepath.Join(t.TempDir(), "banco", "20261004-1200")
	for i := 0; i < 2; i++ { // la seconda volta riscrive gli stessi file
		if err := ScriviRapporto(cartella, r); err != nil {
			t.Fatal(err)
		}
	}
	voci, err := os.ReadDir(cartella)
	if err != nil {
		t.Fatal(err)
	}
	var nomi []string
	for _, v := range voci {
		nomi = append(nomi, v.Name())
	}
	if strings.Join(nomi, " ") != "casi.json casi.txt" {
		t.Fatalf("file nella cartella dei rapporti: %v (nessun .tmp deve restare)", nomi)
	}
	js, _ := os.ReadFile(filepath.Join(cartella, "casi.json"))
	can, err := jsoncanonico.Codifica(json.RawMessage(js))
	if err != nil || !bytes.Equal(can, js) {
		t.Fatalf("il JSON del rapporto non è canonico: %v", err)
	}
	txt, _ := os.ReadFile(filepath.Join(cartella, "casi.txt"))
	if !strings.HasPrefix(string(txt), "ESITO: ESEGUITO — conforme\n") {
		t.Fatalf("prima riga: %q", txt)
	}

	dentro := filepath.Join("testdata", "uscita-non-ammessa")
	if err := ScriviRapporto(dentro, r); err == nil {
		t.Fatal("cartella dentro il modulo accettata")
	}
	if _, err := os.Stat(dentro); !os.IsNotExist(err) {
		t.Fatalf("la cartella rifiutata è stata creata: %v", err)
	}
	r.Modalita = "dsn"
	if err := ScriviRapporto(cartella, r); err == nil {
		t.Fatal("modalità senza nome di rapporto accettata")
	}
}

// esegue: Esegui su un dataset ACME, con il riepilogo di testo.
func esegue(t *testing.T, d datasetACME, modalita string) (Rapporto, string) {
	t.Helper()
	var out bytes.Buffer
	r, err := Esegui(context.Background(), Opzioni{Modalita: modalita, Dataset: d.manifest, Uscita: filepath.Join(d.dir, "banco")}, &out)
	if err != nil {
		t.Fatal(err)
	}
	return r, out.String()
}

func controllo(r Rapporto, nome string) Controllo {
	for _, c := range r.Controlli {
		if c.Nome == nome {
			return c
		}
	}
	return Controllo{}
}

// TestEseguiConforme: le due modalità sul dataset ACME, tutti i controlli eseguiti.
func TestEseguiConforme(t *testing.T) {
	d := preparaDataset(t, nil)
	for _, modalita := range []string{ModalitaRegole, ModalitaCasi} {
		r, testo := esegue(t, d, modalita)
		if r.Esito != EsitoConforme || r.Differenze != 0 {
			t.Fatalf("%s: %s\n%s", modalita, r.PrimaRiga(), testo)
		}
		for _, c := range r.Controlli {
			if c.Stato != ControlloEseguito {
				t.Errorf("%s: controllo %s %s", modalita, c.Nome, c.Stato)
			}
		}
		if r.VersioneLimiti != "limiti-acme-1" || r.Sha256Attesi == "" || r.Sha256Indice == "" || r.VersioneAttesi != 1 {
			t.Errorf("%s: testata del rapporto %+v", modalita, r)
		}
		if !strings.HasPrefix(testo, "ESITO: ESEGUITO — conforme\nmodalità: "+modalita+"\n") ||
			!strings.Contains(testo, "versione_limiti: limiti-acme-1") {
			t.Errorf("%s: riepilogo\n%s", modalita, testo)
		}
		if _, err := os.Stat(filepath.Join(d.dir, "banco", modalita+".json")); err != nil {
			t.Errorf("%s: rapporto non scritto: %v", modalita, err)
		}
	}
	r, _ := esegue(t, d, ModalitaCasi)
	if r.Casi == nil || r.Casi.Conteggi.Passati != 10 || r.Casi.Conteggi.Totale != 14 {
		t.Fatalf("casi: %+v", r.Casi)
	}
}

// TestEseguiConDifferenzeENonEseguito: un caso fallito → con differenze; una voce cambiata o il manifest
// assente → non eseguito, con il motivo in testa; una differenza e una voce assente insieme → con differenze.
func TestEseguiConDifferenzeENonEseguito(t *testing.T) {
	fallito := preparaDataset(t, mutazioni{"attesi_acme.yaml": func(s string) string {
		return strings.Replace(s, "      etichetta: PN\n", "      etichetta: XPN\n", 1)
	}})
	r, testo := esegue(t, fallito, ModalitaCasi)
	if r.Esito != EsitoConDifferenze || r.Differenze != 1 || controllo(r, "casi_contratto").Differenze != 1 {
		t.Fatalf("caso fallito: %s\n%s", r.PrimaRiga(), testo)
	}
	if !strings.Contains(testo, "etichetta [A1a, fallita]: atteso XPN, ottenuto PN") || !strings.Contains(testo, "lettura acme-etichetta-pn/pn") {
		t.Errorf("il riepilogo deve dire atteso, ottenuto e regola del caso non passato:\n%s", testo)
	}

	cambiato := preparaDataset(t, nil)
	if err := os.WriteFile(cambiato.percorso("attesi_acme.yaml"), []byte("versione_attesi: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, testo = esegue(t, cambiato, ModalitaCasi)
	if r.Esito != EsitoNonEseguito || !strings.HasPrefix(testo, "ESITO: NON ESEGUITO — attesi: ") ||
		controllo(r, VoceAttesi).Stato != ControlloNonEseguito {
		t.Fatalf("voce cambiata: %s\n%s", r.PrimaRiga(), testo)
	}

	assente := preparaDataset(t, nil)
	if err := os.Remove(assente.manifest); err != nil {
		t.Fatal(err)
	}
	r, testo = esegue(t, assente, ModalitaRegole)
	if r.Esito != EsitoNonEseguito || !strings.HasPrefix(testo, "ESITO: NON ESEGUITO — manifest: ") {
		t.Fatalf("manifest assente: %s\n%s", r.PrimaRiga(), testo)
	}

	// La grammatica cambiata dopo l'indice (cliente scartato) e gli attesi tolti: 1 prevale su 3.
	misto := preparaDataset(t, nil)
	if err := os.WriteFile(misto.percorso("regole/acme.v1.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(misto.percorso("attesi_acme.yaml")); err != nil {
		t.Fatal(err)
	}
	r, testo = esegue(t, misto, ModalitaRegole)
	if r.Esito != EsitoConDifferenze || controllo(r, "coerenza_esempi_attesi").Stato != ControlloNonEseguito {
		t.Fatalf("differenza e voce assente: %s\n%s", r.PrimaRiga(), testo)
	}
}

// TestEseguiErroreDUso: opzioni sbagliate, uscita o dataset dentro il modulo: nessun rapporto (uscita 2).
func TestEseguiErroreDUso(t *testing.T) {
	d := preparaDataset(t, nil)
	fuori := filepath.Join(d.dir, "banco")
	casi := map[string]Opzioni{
		"modalità ignota":          {Modalita: "dsn", Dataset: d.manifest, Uscita: fuori},
		"modalità vuota":           {Dataset: d.manifest, Uscita: fuori},
		"senza dataset":            {Modalita: ModalitaCasi, Uscita: fuori},
		"senza uscita":             {Modalita: ModalitaCasi, Dataset: d.manifest},
		"uscita dentro il modulo":  {Modalita: ModalitaCasi, Dataset: d.manifest, Uscita: filepath.Join("testdata", "uscita-non-ammessa")},
		"dataset dentro il modulo": {Modalita: ModalitaCasi, Dataset: filepath.Join("testdata", "manifest_acme.json"), Uscita: fuori},
	}
	for nome, o := range casi {
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
	if _, err := os.Stat(filepath.Join("testdata", "uscita-non-ammessa")); !os.IsNotExist(err) {
		t.Fatal("cartella creata dentro il modulo")
	}
}
