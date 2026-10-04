package grammatica

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// L1 — la forma canonica e l'hash delle grammatiche (A1a-SNP; parte 1 §7.1, §9.2; par.3.4.2 del piano A):
// l'ordine senza significato non cambia l'hash, quello con significato e le descrizioni sì; le versioni
// stanno nello snapshot; i limiti e le capacità sono metadati, fuori dall'hash (R43 B, D-08); con un errore
// lo snapshot non nasce (A-C01). Qui sono fissate anche le versioni del pacchetto (par.4.6.1).
//
// I clienti di questi test sono inventati: la grammatica è quella ACME di valida_test.go, permutata o
// cambiata in un pezzo solo.

func snapshotDi(t *testing.T, raw []byte, lim Limiti) SnapshotRegole {
	t.Helper()
	s, err := NuovoSnapshot(raw, lim)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestA1aSNPVersioni fissa le versioni del pacchetto: cambiarne una vuol dire riscrivere questa prova con il
// titolo «Riscritta per …», in un commit che lo dichiara (par.3.4.1, 4.6.1).
func TestA1aSNPVersioni(t *testing.T) {
	if VersioneSchema != 1 || VersioneIndice != 1 || VersioneCapacita != "capacita-a1-1" {
		t.Fatalf("versioni: schema %d, indice %d, capacità %q", VersioneSchema, VersioneIndice, VersioneCapacita)
	}
	s := snapshotDi(t, []byte(grammaticaACME), limitiACME())
	if s.VersioneSchema != 1 || s.VersioneCanonicalizzazione != jsoncanonico.Versione ||
		s.VersioneLimiti != "limiti-acme-1" || s.VersioneCapacita != VersioneCapacita {
		t.Fatalf("versioni nello snapshot: %+v", s)
	}
	if s.ClienteID.String() != "00000000-0000-4000-8000-00000000ac01" {
		t.Fatalf("ClienteID = %s", s.ClienteID)
	}
}

// TestA1aSNPHashECanonico: Hash è lo sha256 del canonico, che è il JSON canonico della grammatica
// normalizzata; le diagnostiche dello snapshot sono avvisi e note, mai errori.
func TestA1aSNPHashECanonico(t *testing.T) {
	s := snapshotDi(t, []byte(grammaticaACME), limitiACME())
	can, err := jsoncanonico.Codifica(Normalizza(s.Grammatica))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(can, s.Canonico) || s.Hash != jsoncanonico.Impronta(s.Canonico) || len(s.Hash) != 64 {
		t.Fatalf("hash o canonico incoerenti: %s", s.Hash)
	}
	if len(s.Diagnostiche) != 1 || s.Diagnostiche[0].Codice != CodiceRiserva {
		t.Fatalf("diagnostiche dello snapshot:%s", elenca(s.Diagnostiche))
	}
	// Il canonico si rilegge con la porta stretta e dà lo stesso hash: è una grammatica come le altre.
	s2 := snapshotDi(t, s.Canonico, limitiACME())
	if s2.Hash != s.Hash {
		t.Fatalf("il canonico riletto cambia hash: %s, %s", s2.Hash, s.Hash)
	}
	// I confini vuoti della base diventano il valore predefinito: scritto o sottinteso, stesso hash.
	esplicito := snapshotDi(t, variante(t, `"maiuscole": "esatte", "normalizza": "nessuna"},`,
		`"maiuscole": "esatte", "normalizza": "nessuna", "confine_prima": "alnum_ascii", "confine_dopo": "alnum_ascii"},`), limitiACME())
	if esplicito.Hash != s.Hash {
		t.Fatal("il confine predefinito scritto per esteso cambia l'hash")
	}
}

// permuta rilegge la grammatica ACME in Go, la cambia e la riscrive in JSON (con encoding/json: l'ordine
// delle chiavi qui non conta, conta quello degli elenchi).
func permuta(t *testing.T, cambia func(*Grammatica)) []byte {
	t.Helper()
	g, err := Decodifica([]byte(grammaticaACME))
	if err != nil {
		t.Fatal(err)
	}
	cambia(&g)
	b, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func rovescia[T any](s []T) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func TestA1aSNPOrdineSenzaSignificato(t *testing.T) {
	base := snapshotDi(t, []byte(grammaticaACME), limitiACME()).Hash
	permutazioni := map[string]func(*Grammatica){
		"famiglie": func(g *Grammatica) { rovescia(g.Famiglie) },
		"forme":    func(g *Grammatica) { rovescia(g.Famiglie[0].Forme) },
		"esempi":   func(g *Grammatica) { rovescia(g.Famiglie[0].Esempi) },
		"decorazioni": func(g *Grammatica) {
			rovescia(g.Famiglie[3].Decorazioni)
		},
		"ruoli":          func(g *Grammatica) { rovescia(g.Famiglie[0].Ruoli) },
		"selettori":      func(g *Grammatica) { rovescia(g.Famiglie[0].Forme[0].Selettori) },
		"riconoscimento": func(g *Grammatica) { rovescia(g.Famiglie[0].Affissi[0].Riconoscimento) },
		"letterali":      func(g *Grammatica) { g.Famiglie[2].Etichette[0].Letterali = []string{"PN", "P/N"} },
		"spazi e chiavi": func(g *Grammatica) {},
	}
	// "letterali" cambia anche il contenuto: si confronta con la sua permutazione, non con la base.
	lettA := snapshotDi(t, permuta(t, permutazioni["letterali"]), limitiACME()).Hash
	lettB := snapshotDi(t, permuta(t, func(g *Grammatica) { g.Famiglie[2].Etichette[0].Letterali = []string{"P/N", "PN"} }), limitiACME()).Hash
	if lettA != lettB {
		t.Error("letterali permutati: hash diverso")
	}
	delete(permutazioni, "letterali")
	for _, nome := range []string{"famiglie", "forme", "esempi", "decorazioni", "ruoli", "selettori", "riconoscimento", "spazi e chiavi"} {
		if h := snapshotDi(t, permuta(t, permutazioni[nome]), limitiACME()).Hash; h != base {
			t.Errorf("%s permutati: hash diverso", nome)
		}
	}
}

func TestA1aSNPOrdineConSignificato(t *testing.T) {
	base := snapshotDi(t, []byte(grammaticaACME), limitiACME()).Hash
	cambi := map[string][]byte{
		"parti permutate":                    permuta(t, func(g *Grammatica) { rovescia(g.Famiglie[0].Forme[0].Parti) }),
		"segmenti della revisione permutati": permuta(t, func(g *Grammatica) { rovescia(g.Famiglie[1].Revisioni[0].Segmenti) }),
		"descrizione cambiata":               variante(t, `"descrizione": "codici inventati con il prefisso di fase"`, `"descrizione": "codici inventati con il prefisso"`),
		"rif_caso cambiato":                  variante(t, `"rif_caso": "ACME-CASO-1"`, `"rif_caso": "ACME-CASO-2"`),
	}
	for _, nome := range []string{"parti permutate", "segmenti della revisione permutati", "descrizione cambiata", "rif_caso cambiato"} {
		if h := snapshotDi(t, cambi[nome], limitiACME()).Hash; h == base {
			t.Errorf("%s: stesso hash", nome)
		}
	}
}

// TestA1aSNPLimitiFuoriDallHash: la stessa grammatica con limiti diversi ha lo stesso hash; lo snapshot porta
// la versione_limiti ricevuta (A1a-LIM, R43 B).
func TestA1aSNPLimitiFuoriDallHash(t *testing.T) {
	a := snapshotDi(t, []byte(grammaticaACME), limitiACME())
	lim := limitiACME()
	lim.Versione = "limiti-acme-2"
	lim.Riconoscimento.MaxLetturePerUnita = 500
	b := snapshotDi(t, []byte(grammaticaACME), lim)
	if a.Hash != b.Hash || !bytes.Equal(a.Canonico, b.Canonico) {
		t.Fatal("i limiti sono entrati nell'hash della grammatica")
	}
	if b.VersioneLimiti != "limiti-acme-2" {
		t.Fatalf("VersioneLimiti = %q, attesa la versione_limiti ricevuta", b.VersioneLimiti)
	}
}

// TestA1aSNPNormalizzaNonCambiaLIngresso: Normalizza lavora su una copia; due chiamate danno gli stessi byte.
func TestA1aSNPNormalizzaNonCambiaLIngresso(t *testing.T) {
	g, err := Decodifica(permuta(t, func(g *Grammatica) { rovescia(g.Famiglie) }))
	if err != nil {
		t.Fatal(err)
	}
	prima := g.Famiglie[0].ID
	n1, _ := jsoncanonico.Codifica(Normalizza(g))
	n2, _ := jsoncanonico.Codifica(Normalizza(g))
	if g.Famiglie[0].ID != prima || !bytes.Equal(n1, n2) {
		t.Fatal("Normalizza ha cambiato l'ingresso o non è deterministica")
	}
	if Normalizza(g).Famiglie[0].ID != "acme-documento" {
		t.Fatal("famiglie non in ordine di ID")
	}
	// Una slice vuota e una assente danno lo stesso canonico.
	g.Famiglie[0].Etichette = []Etichetta{}
	n3, _ := jsoncanonico.Codifica(Normalizza(g))
	if !bytes.Equal(n1, n3) {
		t.Fatal("slice vuota e slice assente danno canonici diversi")
	}
}

// TestAC01NessunoSnapshot: con un errore della decodifica o della validazione lo snapshot non nasce, e
// l'errore è un *evidenze.ErroreContratto con il codice del caso (A-C01).
func TestAC01NessunoSnapshot(t *testing.T) {
	for _, c := range casiAC01 {
		t.Run(c.nome, func(t *testing.T) {
			s, err := NuovoSnapshot(variante(t, c.sostituzioni...), limitiACME())
			var ec *evidenze.ErroreContratto
			if !errors.As(err, &ec) {
				t.Fatalf("errore %v, atteso *evidenze.ErroreContratto", err)
			}
			if s.Hash != "" || s.Canonico != nil || s.Grammatica.Famiglie != nil {
				t.Fatal("con un errore lo snapshot non esiste")
			}
			richiedi(t, ec.Diagnostiche, c.codice, c.percorso)
		})
	}
	// Anche limiti non validi: nessuna grammatica si attiva.
	if _, err := NuovoSnapshot([]byte(grammaticaACME), Limiti{}); err == nil || !strings.Contains(err.Error(), CodiceCampoObbligatorio) {
		t.Fatalf("limiti assenti: errore %v", err)
	}
}

// TestA1aSNPRiservatoNelloSnapshot: una capacità riservata non impedisce lo snapshot: l'avviso viaggia nelle
// sue diagnostiche (A1a-CAP, C-10).
func TestA1aSNPRiservatoNelloSnapshot(t *testing.T) {
	raw := variante(t, `{"id": "stato", "tipo": "stato_pdm", "letterali": ["IN_WORK"], "selettori": ["nome_file"]}`,
		`{"id": "stato", "tipo": "stato_pdm", "letterali": ["IN_WORK"], "selettori": ["nome_file"]},
        {"id": "foglio-a", "tipo": "formato", "letterali": ["A3"], "selettori": ["nome_file"]}`)
	s := snapshotDi(t, raw, limitiACME())
	richiedi(t, s.Diagnostiche, CodiceCapacitaNonSupportata, "famiglie_codice[acme-documento].decorazioni[foglio-a]")
	for _, d := range s.Diagnostiche {
		if d.Gravita == evidenze.GravitaErrore {
			t.Fatalf("errore nello snapshot:%s", elenca(s.Diagnostiche))
		}
	}
}
