package migrazioni

// L1 — T-11 del piano della 0023 (§4, §11; giro 1 e giro 2 della revisione, MIG-09): le quattro funzioni della 0022 che
// la 0023 sostituisce hanno il corpo della 0022 byte per byte, salvo le righe marcate «(0023)», una per una; le cinque
// viste ridefinite hanno il testo di oggi con il solo predicato file_rfq_attivo aggiunto. In piu' lo script di ritorno
// (scripts/0023_indietro.sql, prova U-09) rimette i corpi, le viste e i commenti della 22 byte per byte.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	risorse "promatec/cockpit"
)

// Le quattro funzioni della 0022 che la 0023 sostituisce (PM23 §2, «Funzioni»).
var funzioniSostituite0023 = []string{
	"distinta_bozza_versione_nasce", "conferma_strutturale_nasce", "conferma_strutturale_contenuto", "conferma_strutturale_completa",
}

// Le cinque viste ridefinite, con la migrazione del testo di oggi e quante righe ricevono il predicato (PM23 §4.11).
var visteRidefinite0023 = []struct {
	nome, origine string
	predicati     int
}{
	{"v_fascicolo", "0022_p4_conferme_working.sql", 1},
	{"v_step_prodotto", "0020_bom_versioni.sql", 1},
	{"v_thread_da_riesaminare", "0020_bom_versioni.sql", 3},
	{"v_cruscotto", "0018_fascicolo.sql", 1},
	{"v_codici_candidati_thread", "0018_fascicolo.sql", 3},
}

// I commenti della 0022 che la 0023 aggiorna (e il ritorno rimette).
var commentiAggiornati0023 = []string{
	"COMMENT ON TABLE distinta_bozza_versione IS",
	"COMMENT ON TABLE conferma_strutturale IS",
	"COMMENT ON COLUMN conferma_strutturale_arco.origine IS",
}

var (
	reMarca0023     = regexp.MustCompile(`\s*--\s*\(0023[^)]*\)\s*$`)
	rePredicato0023 = regexp.MustCompile(` AND file_rfq_attivo\([a-z]+\.thread_id, [a-z]+\.allegato_id\)`)
)

func testoMigrazione(t *testing.T, nome string) string {
	t.Helper()
	b, err := risorse.FS.ReadFile("migrations/" + nome)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

func testoScript(t *testing.T, nome string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", nome))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// normalizzaTesta: CREATE OR REPLACE e CREATE sono la stessa testata (la 0023 usa la prima, la 0022 la seconda).
func normalizzaTesta(righe []string) []string {
	out := append([]string(nil), righe...)
	if len(out) > 0 {
		out[0] = strings.Replace(out[0], "CREATE OR REPLACE ", "CREATE ", 1)
	}
	return out
}

// estraiFunzione: le righe dalla testata (CREATE [OR REPLACE] FUNCTION nome()) a «END $$;». Esattamente una nel testo.
func estraiFunzione(testo, nome string) ([]string, error) {
	righe := strings.Split(testo, "\n")
	var trovate [][]string
	for i, r := range righe {
		if strings.HasPrefix(r, "CREATE FUNCTION "+nome+"()") || strings.HasPrefix(r, "CREATE OR REPLACE FUNCTION "+nome+"()") {
			j := i
			for j < len(righe) && righe[j] != "END $$;" {
				j++
			}
			if j == len(righe) {
				return nil, fmt.Errorf("funzione %s senza «END $$;»", nome)
			}
			trovate = append(trovate, righe[i:j+1])
		}
	}
	if len(trovate) != 1 {
		return nil, fmt.Errorf("funzione %s: %d definizioni nel testo", nome, len(trovate))
	}
	return normalizzaTesta(trovate[0]), nil
}

// estraiVista: le righe dell'ultima definizione della vista nel testo (CREATE [OR REPLACE] VIEW nome AS ... ;), con le
// marche tolte solo per riconoscere la riga finale.
func estraiVista(testo, nome string) ([]string, error) {
	righe := strings.Split(testo, "\n")
	inizio := -1
	for i, r := range righe {
		if r == "CREATE VIEW "+nome+" AS" || r == "CREATE OR REPLACE VIEW "+nome+" AS" {
			inizio = i
		}
	}
	if inizio < 0 {
		return nil, fmt.Errorf("vista %s non trovata", nome)
	}
	for j := inizio; j < len(righe); j++ {
		if strings.HasSuffix(strings.TrimRight(reMarca0023.ReplaceAllString(righe[j], ""), " "), ";") {
			return normalizzaTesta(righe[inizio : j+1]), nil
		}
	}
	return nil, fmt.Errorf("vista %s senza «;» finale", nome)
}

// estraiCommento: lo statement COMMENT che comincia con la riga data, fino alla riga che finisce con «';».
func estraiCommento(testo, prima string) ([]string, error) {
	righe := strings.Split(testo, "\n")
	var trovati [][]string
	for i, r := range righe {
		if r == prima {
			j := i
			for j < len(righe) && !strings.HasSuffix(righe[j], "';") {
				j++
			}
			if j == len(righe) {
				return nil, fmt.Errorf("commento %q senza fine", prima)
			}
			trovati = append(trovati, righe[i:j+1])
		}
	}
	if len(trovati) == 0 {
		return nil, fmt.Errorf("commento %q non trovato", prima)
	}
	return trovati[len(trovati)-1], nil
}

func marcata(r string) bool { return strings.Contains(r, "(0023") }

// opRiga e' un passo del confronto riga per riga: '=' uguale, '-' tolta dalla 0022, '+' aggiunta dalla 0023.
type opRiga struct {
	tipo byte
	riga string
}

// confrontoRighe: il confronto per sottosequenza comune piu' lunga (i corpi hanno al piu' qualche centinaio di righe).
func confrontoRighe(a, b []string) []opRiga {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	var out []opRiga
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, opRiga{'=', a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, opRiga{'-', a[i]})
			i++
		default:
			out = append(out, opRiga{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, opRiga{'-', a[i]})
	}
	for ; j < m; j++ {
		out = append(out, opRiga{'+', b[j]})
	}
	return out
}

// differenzeMarcate controlla la regola delle marche su un corpo: in ogni blocco di differenze la parte della 0023 ha
// solo righe marcate e almeno una riga (niente righe della 0022 tolte senza una riga nuova al loro posto). Restituisce
// i problemi trovati.
func differenzeMarcate(vecchio, nuovo []string) []string {
	var problemi []string
	ops := confrontoRighe(vecchio, nuovo)
	for k := 0; k < len(ops); {
		if ops[k].tipo == '=' {
			k++
			continue
		}
		var tolte, aggiunte []string
		for ; k < len(ops) && ops[k].tipo != '='; k++ {
			if ops[k].tipo == '-' {
				tolte = append(tolte, ops[k].riga)
			} else {
				aggiunte = append(aggiunte, ops[k].riga)
			}
		}
		if len(aggiunte) == 0 {
			problemi = append(problemi, fmt.Sprintf("righe della 0022 tolte senza righe marcate al loro posto: %q", tolte))
		}
		for _, r := range aggiunte {
			if !marcata(r) {
				problemi = append(problemi, fmt.Sprintf("riga nuova o cambiata senza la marca (0023): %q", r))
			}
		}
	}
	return problemi
}

// TestV1T11CorpiDelleFunzioniSostituite: per ogni funzione sostituita, il confronto fra il corpo della 0022 e quello
// della 0023 ha, in ogni blocco, dalla parte della 0023 solo righe marcate; togliendo dalla 0023 le righe marcate e
// dalla 0022 le righe che quei blocchi sostituiscono, i due testi coincidono byte per byte.
func TestV1T11CorpiDelleFunzioniSostituite(t *testing.T) {
	m22, m23 := testoMigrazione(t, "0022_p4_conferme_working.sql"), testoMigrazione(t, "0023_distinta_formato3.sql")
	for _, nome := range funzioniSostituite0023 {
		vecchio, err := estraiFunzione(m22, nome)
		if err != nil {
			t.Fatal(err)
		}
		nuovo, err := estraiFunzione(m23, nome)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(nuovo[0], "CREATE FUNCTION ") {
			t.Fatalf("%s: testata %q", nome, nuovo[0])
		}
		for _, p := range differenzeMarcate(vecchio, nuovo) {
			t.Errorf("%s: %s", nome, p)
		}
		// la controprova: senza le righe marcate, le righe della 0023 sono una sottosequenza della 0022
		var senza []string
		for _, r := range nuovo {
			if !marcata(r) {
				senza = append(senza, r)
			}
		}
		for _, op := range confrontoRighe(vecchio, senza) {
			if op.tipo == '+' {
				t.Errorf("%s: riga non marcata che non viene dalla 0022: %q", nome, op.riga)
			}
		}
	}
}

// TestV1T11VisteSoloIlPredicato: il testo di ogni vista ridefinita e' quello di oggi con il solo predicato
// file_rfq_attivo aggiunto, sulle righe marcate e solo su quelle.
func TestV1T11VisteSoloIlPredicato(t *testing.T) {
	m23 := testoMigrazione(t, "0023_distinta_formato3.sql")
	for _, v := range visteRidefinite0023 {
		oggi, err := estraiVista(testoMigrazione(t, v.origine), v.nome)
		if err != nil {
			t.Fatal(err)
		}
		nuova, err := estraiVista(m23, v.nome)
		if err != nil {
			t.Fatal(err)
		}
		predicati := 0
		pulita := make([]string, len(nuova))
		for i, r := range nuova {
			n := len(rePredicato0023.FindAllString(r, -1))
			predicati += n
			if (n > 0) != marcata(r) {
				t.Errorf("%s: riga con il predicato senza la marca, o marcata senza il predicato: %q", v.nome, r)
			}
			pulita[i] = rePredicato0023.ReplaceAllString(reMarca0023.ReplaceAllString(r, ""), "")
		}
		if predicati != v.predicati {
			t.Errorf("%s: %d predicati file_rfq_attivo, attesi %d", v.nome, predicati, v.predicati)
		}
		if strings.Join(pulita, "\n") != strings.Join(oggi, "\n") {
			for _, op := range confrontoRighe(oggi, pulita) {
				if op.tipo != '=' {
					t.Errorf("%s: %c %q", v.nome, op.tipo, op.riga)
				}
			}
		}
	}
}

// TestV1IlRitornoRimetteLa22 (U-09, parte L1): lo script di ritorno rimette le quattro funzioni, le cinque viste e i
// tre commenti della 22 byte per byte, cosi' il catalogo dopo 22 → 23 → 22 e' quello della 22 (la prova L4
// TestV1IlRitornoManualeDalla23Alla22 lo confronta sul database).
func TestV1IlRitornoRimetteLa22(t *testing.T) {
	ritorno := testoScript(t, "0023_indietro.sql")
	m22 := testoMigrazione(t, "0022_p4_conferme_working.sql")
	for _, nome := range funzioniSostituite0023 {
		vecchio, err := estraiFunzione(m22, nome)
		if err != nil {
			t.Fatal(err)
		}
		rimesso, err := estraiFunzione(ritorno, nome)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(rimesso, "\n") != strings.Join(vecchio, "\n") {
			t.Errorf("il ritorno non rimette %s byte per byte", nome)
		}
	}
	for _, v := range visteRidefinite0023 {
		oggi, err := estraiVista(testoMigrazione(t, v.origine), v.nome)
		if err != nil {
			t.Fatal(err)
		}
		rimessa, err := estraiVista(ritorno, v.nome)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(rimessa, "\n") != strings.Join(oggi, "\n") {
			t.Errorf("il ritorno non rimette la vista %s com'era", v.nome)
		}
	}
	for _, c := range commentiAggiornati0023 {
		vecchio, err := estraiCommento(m22, c)
		if err != nil {
			t.Fatal(err)
		}
		rimesso, err := estraiCommento(ritorno, c)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(rimesso, "\n") != strings.Join(vecchio, "\n") {
			t.Errorf("il ritorno non rimette il commento %q della 0022", c)
		}
	}
	// lo script non e' una migrazione e si ferma prima di toccare un database non alla 23
	if _, err := risorse.FS.ReadFile("migrations/0023_indietro.sql"); err == nil {
		t.Error("0023_indietro.sql sta fra le migrazioni")
	}
	for _, frase := range []string{"non alla 23", "righe formato 3", "le tabelle della 23", "colonne della 23", "gesto della 23",
		"DELETE FROM schema_versione WHERE versione = 23;", "DELETE FROM schema_variante WHERE versione = 23;"} {
		if !strings.Contains(ritorno, frase) {
			t.Errorf("lo script di ritorno non contiene %q", frase)
		}
	}
}

// La regola delle marche coglie i tre difetti che T-11 deve vedere: una riga cambiata senza la marca, una riga nuova
// senza la marca, una riga della 0022 tolta senza una riga nuova al suo posto. Una sostituzione marcata passa.
func TestV1T11LaRegolaDelleMarcheColgeIDifetti(t *testing.T) {
	vecchio := []string{"BEGIN", "    IF a THEN", "        x := 1;", "    END IF;", "END $$;"}
	casi := []struct {
		nome   string
		nuovo  []string
		difett bool
	}{
		{"sostituzione marcata", []string{"BEGIN", "    IF a OR b THEN  -- (0023)", "        x := 1;", "    END IF;", "END $$;"}, false},
		{"blocco nuovo marcato", []string{"BEGIN", "    y := 2;  -- (0023)", "    IF a THEN", "        x := 1;", "    END IF;", "END $$;"}, false},
		{"riga cambiata senza marca", []string{"BEGIN", "    IF a OR b THEN", "        x := 1;", "    END IF;", "END $$;"}, true},
		{"riga nuova senza marca", []string{"BEGIN", "    y := 2;", "    IF a THEN", "        x := 1;", "    END IF;", "END $$;"}, true},
		{"riga tolta senza sostituta", []string{"BEGIN", "    IF a THEN", "    END IF;", "END $$;"}, true},
	}
	for _, c := range casi {
		if got := len(differenzeMarcate(vecchio, c.nuovo)) > 0; got != c.difett {
			t.Errorf("%s: difetti trovati %v, attesi %v", c.nome, got, c.difett)
		}
	}
}
