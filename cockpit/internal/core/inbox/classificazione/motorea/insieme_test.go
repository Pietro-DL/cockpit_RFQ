package motorea

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L1 — l'insieme delle regole (A1a-INS; piano A, par.3.6.3-3.6.5, 4.4.5, 4.7.2; par.12; v3 §10.5):
// CompilaInsieme scarta un cliente con lo sha256 diverso dall'indice, il cliente.id diverso dalla voce, il
// file assente o la grammatica non valida, e lascia attivi gli altri; MotoreDi cerca per UUID e controlla la
// ragione sociale; un cliente senza voce dà nil e regole.assenti. Un cliente ACME in più entra con la sola
// voce nell'indice e il suo file v1, che usa capacità già previste: è attivo e i suoi esempi passano, senza
// codice Go che lo nomini (par.12). Indice e grammatiche stanno in t.TempDir(): li legge la prova, non il
// motore, che non tocca il disco.
//
// I clienti di questi test sono inventati: il repository è pubblico, e le famiglie di codice dei clienti veri
// sono un dato dell'azienda. Gli UUID, le ragioni sociali e le famiglie sono inventati.

var (
	clienteNuovo = uuid.MustParse("00000000-0000-4000-8000-00000000ac02")
	clienteTerzo = uuid.MustParse("00000000-0000-4000-8000-00000000ac03")
	clienteMai   = uuid.MustParse("00000000-0000-4000-8000-00000000ac09")
)

const ragioneSocialeNuovo = "ACME Nuova S.r.l."

func sha256Esadecimale(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// famNuova: la famiglia del cliente aggiunto con i soli file. Usa capacità già previste (marcatore, revisione
// inline con un separatore, token, involucro, confine parola_ascii), nessuna nuova.
func famNuova() grammatica.FamigliaCodice {
	nome := forma("nome", sel("nome_file"), pRif(grammatica.TipoParteDecorazione, "lotto", 1), pBase(), pMarcatore("K"),
		pRif(grammatica.TipoParteRevisione, "rev", 0))
	nome.ConfineDopo = classe(grammatica.ConfineParolaASCII)
	return grammatica.FamigliaCodice{
		ID: "acme-nuova", Namespace: "acme-nuova",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloComponente},
		Base:  base(segLetterale("serie", "AB", ""), segPattern("numero", "[0-9]{5}", "")),
		Forme: []grammatica.FormaCodice{
			nome,
			forma("corpo", sel("corpo"), pBase(), pMarcatore("K"), pSep("-"), pToken("Q1", "TMP")),
		},
		Revisioni: []grammatica.RegolaRevisione{{ID: "rev", Selettori: sel("nome_file"), Stato: grammatica.StatoAttiva,
			Sorgente: grammatica.SorgenteInline, Separatori: []string{"-r"},
			Segmenti: []grammatica.SegmentoRevisione{{Nome: "valore", Pattern: "[0-9]{2}", Significato: grammatica.SignificatoNessuno}}}},
		Decorazioni: []grammatica.Decorazione{{ID: "lotto", Tipo: grammatica.TipoDecorazioneInvolucro,
			Sottotipo: grammatica.SottotipoRiferimentoPacchetto, Letterali: []string{"LOTTO7_"}, Selettori: sel("nome_file")}},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-nome", "nome_file", "LOTTO7_AB12345K-r03.step",
				grammatica.LetturaAttesa{Forma: "nome", Base: "AB12345", Marcatore: "K", Revisione: "03", Decorazioni: []string{"LOTTO7_"}}),
			positivo("e-corpo", "corpo", "AB12345K-TMP", grammatica.LetturaAttesa{Forma: "corpo", Base: "AB12345", Marcatore: "K"}),
			negativo("n-corpo", "corpo", "AB12345K_TMP"),
		},
	}
}

func grammaticaNuova() grammatica.Grammatica {
	g := grammaticaACME(famNuova())
	g.Cliente = grammatica.ClienteGrammatica{ID: clienteNuovo, RagioneSociale: ragioneSocialeNuovo}
	return g
}

// cartellaRegole scrive i file delle grammatiche e l'indice in una cartella temporanea, come li terrebbe il
// dataset privato. voci: nome del file → (cliente della voce, contenuto). Lo sha256 nell'indice è quello del
// contenuto, salvo quelli in shaDiversi.
type voceFile struct {
	file    string
	cliente uuid.UUID
	raw     []byte
}

func cartellaRegole(t *testing.T, voci []voceFile, shaDiversi map[string]string, assenti map[string]bool) string {
	t.Helper()
	dir := t.TempDir()
	ind := grammatica.IndiceRegole{VersioneIndice: grammatica.VersioneIndice, Limiti: limitiACME()}
	for _, v := range voci {
		sha := sha256Esadecimale(v.raw)
		if s, ok := shaDiversi[v.file]; ok {
			sha = s
		}
		ind.Grammatiche = append(ind.Grammatiche, grammatica.VoceIndice{ClienteID: v.cliente, File: v.file, Sha256: sha})
		if !assenti[v.file] {
			if err := os.WriteFile(filepath.Join(dir, v.file), v.raw, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	raw, err := json.Marshal(ind)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "indice_acme.v1.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// leggiInsieme: come farebbe chi chiama (il banco, il caricatore): l'indice con LeggiIndice, i file che
// l'indice nomina e che ci sono, poi CompilaInsieme.
func leggiInsieme(t *testing.T, dir string) (InsiemeRegole, []evidenze.Diagnostica, error) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "indice_acme.v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	ind, err := grammatica.LeggiIndice(raw)
	if err != nil {
		t.Fatalf("indice: %v\n%s", err, elenco(diagnosticheDi(err)))
	}
	contenuti := map[string][]byte{}
	for _, v := range ind.Grammatiche {
		if b, err := os.ReadFile(filepath.Join(dir, v.File)); err == nil {
			contenuti[v.File] = b
		}
	}
	return CompilaInsieme(ind, contenuti)
}

func vociBuone(t *testing.T) []voceFile {
	return []voceFile{
		{"acme.v1.json", clienteACME, fileJSON(t, grammaticaACME(tutteLeFamiglie()...))},
		{"acme-nuova.v1.json", clienteNuovo, fileJSON(t, grammaticaNuova())},
	}
}

func TestInsiemeTuttiAttivi(t *testing.T) {
	ins, d, err := leggiInsieme(t, cartellaRegole(t, vociBuone(t), nil, nil))
	if err != nil {
		t.Fatalf("%v\n%s", err, elenco(d))
	}
	if len(ins.Motori) != 2 || len(ins.Scartati) != 0 {
		t.Fatalf("motori %d, scartati %d:\n%s", len(ins.Motori), len(ins.Scartati), elenco(d))
	}
	if len(conGravita(d, evidenze.GravitaErrore)) != 0 {
		t.Errorf("errori con tutti i clienti attivi:\n%s", elenco(d))
	}
	if ins.ImprontaIndice == "" || len(ins.ImprontaIndice) != 64 {
		t.Errorf("impronta dell'indice %q", ins.ImprontaIndice)
	}
	if !reflect.DeepEqual(ins.Indice.Limiti, limitiACME()) {
		t.Errorf("limiti dell'insieme %+v: sono quelli dell'indice", ins.Indice.Limiti)
	}
}

// TestInsiemeClienteNuovoSoloConIFile (par.12): il cliente aggiunto con la voce nell'indice e il suo file v1
// è attivo, i suoi esempi sono verificati e Riconosci lo legge; nessun sorgente Go del motore lo nomina.
func TestInsiemeClienteNuovoSoloConIFile(t *testing.T) {
	ins, d, err := leggiInsieme(t, cartellaRegole(t, vociBuone(t), nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	m, dm := ins.MotoreDi(clienteNuovo, ragioneSocialeNuovo)
	if m == nil {
		t.Fatalf("cliente nuovo non attivo:\n%s\n%s", elenco(dm), elenco(d))
	}
	for _, x := range d {
		if x.Codice == CodiceEsempioNonVerificato && strings.Contains(x.Percorso, "acme-nuova") {
			t.Errorf("esempio del cliente nuovo non verificato: %s", x.Percorso)
		}
	}
	letture, _ := riconosci(t, m, "nome_file", "LOTTO7_AB12345K-r03.step")
	l := unaLettura(t, letture, "acme-nuova", "nome")
	if l.Base.Normalizzata != "AB12345" || l.Revisione == nil || l.Revisione.Normalizzata != "03" || l.Marcatore == nil {
		t.Errorf("lettura %+v", l)
	}
	// Il motore di un cliente non legge le forme di un altro.
	if letture, _ := riconosci(t, m, "corpo", "P7120100"); len(letture) != 0 {
		t.Errorf("il motore del cliente nuovo legge le forme di ACME: %s", riassunto(letture))
	}

	// Nessun codice Go del motore nomina il cliente nuovo, la sua famiglia o i suoi letterali.
	p, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range p {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, nome := range []string{"acme-nuova", clienteNuovo.String(), "LOTTO7_", ragioneSocialeNuovo} {
			if bytes.Contains(b, []byte(nome)) {
				t.Errorf("%s nomina %q: un cliente nuovo entra solo con i file", f, nome)
			}
		}
	}
}

func TestInsiemeMotoreDi(t *testing.T) {
	ins, _, err := leggiInsieme(t, cartellaRegole(t, vociBuone(t), nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, rs := range []string{ragioneSocialeACME, "  ACME S.P.A.  ", "acme s.p.a."} {
		if m, d := ins.MotoreDi(clienteACME, rs); m == nil || len(d) != 0 {
			t.Errorf("ragione sociale %q: motore %v\n%s", rs, m != nil, elenco(d))
		} else if m.Snapshot().ClienteID != clienteACME {
			t.Errorf("motore del cliente %v", m.Snapshot().ClienteID)
		}
	}
	// Un UUID giusto con la ragione sociale di un altro: nil e la diagnosi (par.3.6.3).
	m, d := ins.MotoreDi(clienteACME, ragioneSocialeNuovo)
	if m != nil || len(conCodice(d, grammatica.CodiceRegoleRagioneSocialeDiscorde)) != 1 {
		t.Errorf("ragione sociale discorde: motore %v\n%s", m != nil, elenco(d))
	}
	// Un cliente senza voce: nil e regole.assenti, una nota.
	m, d = ins.MotoreDi(clienteMai, "Chiunque S.p.A.")
	ra := conCodice(d, CodiceRegoleAssenti)
	if m != nil || len(ra) != 1 || ra[0].Gravita != evidenze.GravitaNota || ra[0].Natura != evidenze.NaturaDati {
		t.Errorf("cliente assente: motore %v\n%s", m != nil, elenco(d))
	}
}

// scartato: il cliente è fra gli scartati con il codice, l'altro resta attivo, e MotoreDi lo dice.
func scartato(t *testing.T, ins InsiemeRegole, d []evidenze.Diagnostica, err error, cliente uuid.UUID, codice string) {
	t.Helper()
	if err != nil {
		t.Fatalf("un cliente scartato non è un errore dell'insieme: %v", err)
	}
	if ins.Motori[cliente] != nil {
		t.Fatalf("cliente %v attivo, atteso scartato con %s", cliente, codice)
	}
	if len(conCodice(ins.Scartati[cliente], codice)) == 0 {
		t.Errorf("scartati[%v] senza %s:\n%s", cliente, codice, elenco(ins.Scartati[cliente]))
	}
	if len(conCodice(d, codice)) == 0 {
		t.Errorf("le diagnostiche dell'insieme non portano %s:\n%s", codice, elenco(d))
	}
	m, dm := ins.MotoreDi(cliente, ragioneSocialeACME)
	if m != nil || len(conCodice(dm, codice)) == 0 {
		t.Errorf("MotoreDi del cliente scartato: motore %v\n%s", m != nil, elenco(dm))
	}
	if len(conCodice(dm, CodiceRegoleAssenti)) != 0 {
		t.Errorf("un cliente scartato non è un cliente senza regole:\n%s", elenco(dm))
	}
	altri := 0
	for id, m := range ins.Motori {
		if id != cliente && m != nil {
			altri++
		}
	}
	if altri == 0 {
		t.Errorf("nessun altro cliente attivo: uno scarto non deve toccare gli altri")
	}
}

func TestInsiemeSha256Discorde(t *testing.T) {
	voci := vociBuone(t)
	dir := cartellaRegole(t, voci, nil, nil)
	// Il file cambia dopo l'indice.
	if err := os.WriteFile(filepath.Join(dir, "acme.v1.json"), append(voci[0].raw, ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	ins, d, err := leggiInsieme(t, dir)
	scartato(t, ins, d, err, clienteACME, CodiceRegoleSha256Discorde)

	// Lo stesso con uno sha256 dell'indice sbagliato.
	ins, d, err = leggiInsieme(t, cartellaRegole(t, voci, map[string]string{"acme.v1.json": strings.Repeat("0", 64)}, nil))
	scartato(t, ins, d, err, clienteACME, CodiceRegoleSha256Discorde)
}

func TestInsiemeClienteDiscorde(t *testing.T) {
	voci := vociBuone(t)
	voci = append(voci, voceFile{"acme-terzo.v1.json", clienteTerzo, fileJSON(t, grammaticaACME(famPunti()))})
	ins, d, err := leggiInsieme(t, cartellaRegole(t, voci, nil, nil))
	scartato(t, ins, d, err, clienteTerzo, CodiceRegoleClienteDiscorde)
	if ins.Motori[clienteACME] == nil || ins.Motori[clienteNuovo] == nil {
		t.Errorf("gli altri clienti devono restare attivi")
	}
}

func TestInsiemeFileAssente(t *testing.T) {
	ins, d, err := leggiInsieme(t, cartellaRegole(t, vociBuone(t), nil, map[string]bool{"acme.v1.json": true}))
	scartato(t, ins, d, err, clienteACME, CodiceRegoleFileAssente)
}

func TestInsiemeGrammaticaNonValida(t *testing.T) {
	voci := vociBuone(t)
	// Un esempio sbagliato: la grammatica valida non si attiva.
	f := famPrefisso()
	f.Esempi = append(f.Esempi, negativo("n-no", "corpo", "P7120100"))
	voci[0].raw = fileJSON(t, grammaticaACME(f))
	ins, d, err := leggiInsieme(t, cartellaRegole(t, voci, nil, nil))
	scartato(t, ins, d, err, clienteACME, CodiceRegoleNonValide)
	if len(conCodice(ins.Scartati[clienteACME], CodiceEsempioInEccesso)) == 0 {
		t.Errorf("lo scarto non dice perché:\n%s", elenco(ins.Scartati[clienteACME]))
	}

	// Un file che non si decodifica.
	voci = vociBuone(t)
	voci[0].raw = []byte(`{"versione_schema": 1, "sconosciuta": true}`)
	ins, d, err = leggiInsieme(t, cartellaRegole(t, voci, nil, nil))
	scartato(t, ins, d, err, clienteACME, CodiceRegoleNonValide)
}

// TestInsiemeClienteRipetuto: un indice costruito a mano con un cliente due volte non attiva nessuna
// grammatica: quale valga non si decide.
func TestInsiemeClienteRipetuto(t *testing.T) {
	ind, contenuti := indiceACME(t, limitiACME())
	ind.Grammatiche = append(ind.Grammatiche, ind.Grammatiche[0])
	ins, d, err := CompilaInsieme(ind, contenuti)
	if err == nil || len(ins.Motori) != 0 || len(conCodice(d, grammatica.CodiceRegoleClienteRipetuto)) == 0 {
		t.Errorf("cliente ripetuto: errore %v, motori %d\n%s", err, len(ins.Motori), elenco(d))
	}
}

// TestInsiemeDeterministico: due compilazioni dello stesso indice danno le stesse diagnostiche nello stesso
// ordine e la stessa impronta.
func TestInsiemeDeterministico(t *testing.T) {
	voci := append(vociBuone(t), voceFile{"acme-terzo.v1.json", clienteTerzo, fileJSON(t, grammaticaACME(famPunti()))})
	dir := cartellaRegole(t, voci, nil, map[string]bool{"acme-nuova.v1.json": true})
	a, da, _ := leggiInsieme(t, dir)
	b, db, _ := leggiInsieme(t, dir)
	if !reflect.DeepEqual(da, db) || a.ImprontaIndice != b.ImprontaIndice {
		t.Errorf("due compilazioni diverse:\n%s\n---\n%s", elenco(da), elenco(db))
	}
}
