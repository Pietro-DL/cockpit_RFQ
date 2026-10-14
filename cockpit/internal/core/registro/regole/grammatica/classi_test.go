package grammatica

import (
	"bytes"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// L1 — la tabella delle classi di una famiglia (classi.go; decisione dell'utente del 14/10, [U]): serie del codice →
// (categoria, descrizione, fonte). Una tabella valida passa e ha un hash che non dipende dall'ordine delle voci; ogni
// violazione dà il suo codice con il percorso; una grammatica senza tabella e un indice senza max_voci_classi restano
// quelli di prima, byte per byte.
//
// I clienti di questi test sono inventati: la grammatica è quella ACME di valida_test.go, con la famiglia acme-punti
// (8.BBB.T) che al posto della categoria minuteria porta una tabella sul segmento «b». Valori, descrizioni e fonti
// sono inventati.

// tabellaACME: la tabella delle classi della variante, al posto delle categorie di acme-punti.
const tabellaACME = `"classi": {"segmento": "b", "voci": [
        {"valore": "456", "categoria": "commerciale", "descrizione": "Boccola inventata", "fonte": "norma ACME 8456"},
        {"valore": "123", "categoria": "minuteria", "descrizione": "Vite inventata"}]},`

// conTabella: la grammatica ACME con la tabella delle classi su acme-punti, più le sostituzioni date.
func conTabella(t *testing.T, sostituzioni ...string) []byte {
	t.Helper()
	return variante(t, append([]string{`"categorie": ["minuteria"],`, tabellaACME}, sostituzioni...)...)
}

// limitiConClassi: i limiti ACME con max_voci_classi dichiarato.
func limitiConClassi(n int) Limiti {
	l := limitiACME()
	l.Grammatica.MaxVociClassi = n
	return l
}

func TestClassiTabellaValida(t *testing.T) {
	ds := diagnosiDi(t, conTabella(t), limitiConClassi(8))
	if e := errori(ds); len(e) > 0 {
		t.Fatalf("la tabella ACME ha errori:%s", elenca(e))
	}
	s := snapshotDi(t, conTabella(t), limitiConClassi(8))
	var f *FamigliaCodice
	for i := range s.Grammatica.Famiglie {
		if s.Grammatica.Famiglie[i].ID == "acme-punti" {
			f = &s.Grammatica.Famiglie[i]
		}
	}
	if f == nil || f.Classi == nil || len(f.Classi.Voci) != 2 || f.Classi.Voci[0].Valore != "123" || f.Classi.Voci[1].Valore != "456" {
		t.Fatalf("la tabella normalizzata non ha le voci in ordine di valore: %+v", f)
	}
	if f.Categorie != nil {
		t.Fatalf("categorie inventate dalla normalizzazione: %v", f.Categorie)
	}
	// L'ordine delle voci nel file non conta: stesso canonico, stesso hash.
	rovesciata := conTabella(t, `{"valore": "456", "categoria": "commerciale", "descrizione": "Boccola inventata", "fonte": "norma ACME 8456"},
        {"valore": "123", "categoria": "minuteria", "descrizione": "Vite inventata"}]`,
		`{"valore": "123", "categoria": "minuteria", "descrizione": "Vite inventata"},
        {"valore": "456", "categoria": "commerciale", "descrizione": "Boccola inventata", "fonte": "norma ACME 8456"}]`)
	if r := snapshotDi(t, rovesciata, limitiConClassi(8)); r.Hash != s.Hash || !bytes.Equal(r.Canonico, s.Canonico) {
		t.Fatalf("l'ordine delle voci cambia l'hash: %s, %s", r.Hash, s.Hash)
	}
	// La descrizione e la fonte entrano nell'hash, come ogni testo della grammatica (parte 1 §9.2).
	if d := snapshotDi(t, conTabella(t, `"Vite inventata"`, `"Vite inventata M6"`), limitiConClassi(8)); d.Hash == s.Hash {
		t.Fatal("una descrizione diversa non cambia l'hash")
	}
	// Il canonico si rilegge con la porta stretta e dà lo stesso hash.
	if r := snapshotDi(t, s.Canonico, limitiConClassi(8)); r.Hash != s.Hash {
		t.Fatalf("il canonico riletto cambia hash: %s, %s", r.Hash, s.Hash)
	}
	// Il limite vale anche al suo valore esatto.
	if e := errori(diagnosiDi(t, conTabella(t), limitiConClassi(2))); len(e) > 0 {
		t.Fatalf("due voci con max_voci_classi 2:%s", elenca(e))
	}
}

// TestClassiSenzaTabellaComePrima: la grammatica ACME senza tabella e i limiti ACME senza max_voci_classi hanno gli
// stessi byte canonici e le stesse impronte del codice di prima della tabella (aabe52c, calcolati lì con la stessa
// funzione): il campo è facoltativo e inerte, e nessuna versione cambia.
func TestClassiSenzaTabellaComePrima(t *testing.T) {
	const (
		hashPrima   = "929397515c5af8feb2b825d0a115ad32940b9819dec13e15455c76c96c0b53c7"
		limitiPrima = "a47ea7493fa3ea63719866f8b3b6461903442c386157d949087aadcb24c9c776"
	)
	s := snapshotDi(t, []byte(grammaticaACME), limitiACME())
	if s.Hash != hashPrima || bytes.Contains(s.Canonico, []byte(`"classi"`)) {
		t.Fatalf("la grammatica senza tabella ha cambiato canonico: %s", s.Hash)
	}
	l, err := jsoncanonico.ImprontaDi(limitiACME())
	if err != nil {
		t.Fatal(err)
	}
	if l != limitiPrima {
		t.Fatalf("i limiti senza max_voci_classi hanno cambiato impronta: %s", l)
	}
	if ds := limitiACME().Valida(); ds != nil {
		t.Fatalf("i limiti senza max_voci_classi non sono più validi:%s", elenca(ds))
	}
}

// TestClassiIndiceSenzaLimite: con un indice che non dichiara max_voci_classi (lo zero) nessuna tabella si attiva:
// limite.superato, errore di limite, con il messaggio che nomina la chiave (R43 B).
func TestClassiIndiceSenzaLimite(t *testing.T) {
	d := richiedi(t, diagnosiDi(t, conTabella(t), limitiACME()), CodiceLimiteSuperato, "famiglie_codice[acme-punti].classi.voci")
	if d.Gravita != evidenze.GravitaErrore || d.Natura != evidenze.NaturaLimite || !strings.Contains(d.Messaggio, "max_voci_classi") {
		t.Fatalf("tabella senza limite: %s %s %q", d.Gravita, d.Natura, d.Messaggio)
	}
}

func TestClassiErrori(t *testing.T) {
	const p = "famiglie_codice[acme-punti]"
	lungo := strings.Repeat("x", MaxByteDescrizioneClasse+1)
	casi := []struct {
		nome     string
		raw      []byte
		lim      int
		codice   string
		percorso string
	}{
		{"con le categorie", variante(t, `"categorie": ["minuteria"],`, `"categorie": ["minuteria"], `+tabellaACME), 8,
			CodiceClassiConCategorie, p + ".categorie"},
		{"senza segmento", conTabella(t, `"segmento": "b"`, `"segmento": ""`), 8, CodiceCampoObbligatorio, p + ".classi.segmento"},
		{"segmento fuori dalla base", conTabella(t, `"segmento": "b"`, `"segmento": "z"`), 8, CodiceClassiSegmentoNonIdentitario, p + ".classi.segmento"},
		{"valore non riconosciuto dal segmento", conTabella(t, `"valore": "123"`, `"valore": "12"`), 8, CodiceClassiValoreNonConforme, p + ".classi.voci[12].valore"},
		{"valore con il separatore", conTabella(t, `"valore": "123"`, `"valore": ".123"`), 8, CodiceClassiValoreNonConforme, p + ".classi.voci[.123].valore"},
		{"valore vuoto", conTabella(t, `"valore": "123"`, `"valore": ""`), 8, CodiceCampoObbligatorio, p + ".classi.voci[1].valore"},
		{"valore ripetuto", conTabella(t, `"valore": "123"`, `"valore": "456"`), 8, CodiceInsiemeConDuplicati, p + ".classi.voci[1]"},
		{"categoria fuori elenco", conTabella(t, `"categoria": "minuteria", "descrizione": "Vite`, `"categoria": "fabbricato", "descrizione": "Vite`), 8,
			CodiceEnumIgnoto, p + ".classi.voci[123].categoria"},
		{"senza descrizione", conTabella(t, `"descrizione": "Vite inventata"`, `"descrizione": "  "`), 8, CodiceCampoObbligatorio, p + ".classi.voci[123].descrizione"},
		{"descrizione con un controllo", conTabella(t, `"descrizione": "Vite inventata"`, `"descrizione": "Vite\tinventata"`), 8,
			CodiceCarattereDiControllo, p + ".classi.voci[123].descrizione"},
		{"descrizione troppo lunga", conTabella(t, `"descrizione": "Vite inventata"`, `"descrizione": "`+lungo+`"`), 8,
			CodiceLimiteSuperato, p + ".classi.voci[123].descrizione"},
		{"fonte troppo lunga", conTabella(t, `"fonte": "norma ACME 8456"`, `"fonte": "`+strings.Repeat("f", MaxByteFonteClasse+1)+`"`), 8,
			CodiceLimiteSuperato, p + ".classi.voci[456].fonte"},
		{"fonte con un controllo", conTabella(t, `"fonte": "norma ACME 8456"`, `"fonte": "norma ACME"`), 8,
			CodiceCarattereDiControllo, p + ".classi.voci[456].fonte"},
		{"senza voci", variante(t, `"categorie": ["minuteria"],`, `"classi": {"segmento": "b", "voci": []},`), 8, CodiceCampoObbligatorio, p + ".classi.voci"},
		{"troppe voci", conTabella(t), 1, CodiceLimiteSuperato, p + ".classi.voci"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			d := richiedi(t, diagnosiDi(t, c.raw, limitiConClassi(c.lim)), c.codice, c.percorso)
			if d.Gravita != evidenze.GravitaErrore {
				t.Fatalf("%s: gravità %s, atteso errore", c.codice, d.Gravita)
			}
		})
	}
	// Una chiave sconosciuta dentro la tabella: la porta stretta la rifiuta, come ovunque.
	richiedi(t, diagnosiDi(t, conTabella(t, `"valore": "123", "categoria"`, `"valore": "123", "classe": "x", "categoria"`), limitiConClassi(8)),
		CodiceChiaveSconosciuta, "")
}

// TestClassiSegmentoNonIdentitario: il segmento della tabella deve essere identitario.
func TestClassiSegmentoNonIdentitario(t *testing.T) {
	g, err := Decodifica(conTabella(t))
	if err != nil {
		t.Fatal(err)
	}
	for i := range g.Famiglie {
		if g.Famiglie[i].ID == "acme-punti" {
			g.Famiglie[i].Base.Segmenti[1].Identitario = false
		}
	}
	richiedi(t, g.Valida(limitiConClassi(8)), CodiceClassiSegmentoNonIdentitario, "famiglie_codice[acme-punti].classi.segmento")
}

// TestClassiMaiuscole: il valore si controlla con il segmento compilato come lo compila il motore. Con le maiuscole
// indifferenti «a12» è riconosciuto da [A-C][0-9]{2}; con normalizza «maiuscolo» la lettura dà «A12», quindi la voce
// si scrive così; con le maiuscole esatte «a12» non è riconosciuto. Regressione del 14/10: con le maiuscole esatte,
// normalizza «maiuscolo» e un pattern con lettere minuscole ([a-c]) la lettura di «a12» dà «A12», e la voce «A12» si
// accetta (prima ogni voce si rifiutava: il segmento si controllava senza piega).
func TestClassiMaiuscole(t *testing.T) {
	casi := []struct {
		nome                  string
		maiuscole, normalizza string
		valore                string
		errore                bool
		pattern               string // vuoto: [A-C][0-9]{2}
	}{
		{"indifferenti, nessuna normalizzazione, minuscola", MaiuscoleIndifferentiASCII, NormalizzaNessuna, "a12", false, ""},
		{"indifferenti, normalizza in maiuscolo, minuscola", MaiuscoleIndifferentiASCII, NormalizzaMaiuscolo, "a12", true, ""},
		{"indifferenti, normalizza in maiuscolo, maiuscola", MaiuscoleIndifferentiASCII, NormalizzaMaiuscolo, "A12", false, ""},
		{"esatte, minuscola", MaiuscoleEsatte, NormalizzaNessuna, "a12", true, ""},
		{"esatte, maiuscola", MaiuscoleEsatte, NormalizzaNessuna, "B12", false, ""},
		{"esatte, normalizza in maiuscolo, pattern minuscolo, maiuscola", MaiuscoleEsatte, NormalizzaMaiuscolo, "A12", false, "[a-c][0-9]{2}"},
		{"esatte, normalizza in maiuscolo, pattern minuscolo, minuscola", MaiuscoleEsatte, NormalizzaMaiuscolo, "a12", true, "[a-c][0-9]{2}"},
		{"esatte, normalizza in maiuscolo, pattern minuscolo, fuori", MaiuscoleEsatte, NormalizzaMaiuscolo, "D12", true, "[a-c][0-9]{2}"},
		{"esatte, normalizza in maiuscolo, pattern maiuscolo, maiuscola", MaiuscoleEsatte, NormalizzaMaiuscolo, "C12", false, ""},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			g, err := Decodifica(conTabella(t))
			if err != nil {
				t.Fatal(err)
			}
			for i := range g.Famiglie {
				f := &g.Famiglie[i]
				if f.ID != "acme-punti" {
					continue
				}
				f.Base.Segmenti[1].Pattern = "[A-C][0-9]{2}"
				if c.pattern != "" {
					f.Base.Segmenti[1].Pattern = c.pattern
				}
				f.Base.Maiuscole, f.Base.Normalizza = c.maiuscole, c.normalizza
				f.Classi.Voci = []VoceClasse{{Valore: c.valore, Categoria: CategoriaMinuteria, Descrizione: "Vite inventata"}}
			}
			var nostri []evidenze.Diagnostica
			for _, d := range g.Valida(limitiConClassi(8)) {
				if d.Codice == CodiceClassiValoreNonConforme {
					nostri = append(nostri, d)
				}
			}
			if (len(nostri) > 0) != c.errore {
				t.Fatalf("valore %q: errore atteso %v, diagnostiche:%s", c.valore, c.errore, elenca(nostri))
			}
		})
	}
}

// TestClassiLimiteNellIndice: max_voci_classi è facoltativo (lo zero vale «non dichiarato») ma, se c'è, sta sotto il
// tetto e non è negativo.
func TestClassiLimiteNellIndice(t *testing.T) {
	if ds := limitiConClassi(TettiLimiti().Grammatica.MaxVociClassi).Valida(); ds != nil {
		t.Fatalf("max_voci_classi al tetto rifiutato:%s", elenca(ds))
	}
	richiedi(t, limitiConClassi(TettiLimiti().Grammatica.MaxVociClassi+1).Valida(), CodiceLimiteOltreTetto, "limiti.grammatica.max_voci_classi")
	richiedi(t, limitiConClassi(-1).Valida(), CodiceCampoObbligatorio, "limiti.grammatica.max_voci_classi")
}

// TestCategoriaCommerciale: «commerciale» è nell'enum delle categorie (N-CATEGORIA, 14/10, [U]): una famiglia senza
// tabella la può dichiarare come la minuteria.
func TestCategoriaCommerciale(t *testing.T) {
	ds := diagnosiDi(t, variante(t, `"categorie": ["minuteria"]`, `"categorie": ["commerciale"]`), limitiACME())
	if e := errori(ds); len(e) > 0 {
		t.Fatalf("la categoria commerciale rifiutata:%s", elenca(e))
	}
}
