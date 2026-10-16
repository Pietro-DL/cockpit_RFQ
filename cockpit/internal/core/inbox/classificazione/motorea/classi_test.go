package motorea

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L1 — la tabella delle classi nel motore (decisione dell'utente del 14/10, [U]): una lettura di una famiglia con la
// tabella prende la voce del valore del suo segmento (LetturaForma.Classe) e le categorie diventano quelle della
// famiglia più quella della voce; senza voce la lettura non cambia; senza tabelle letture e interpretazioni hanno gli
// stessi byte del codice di prima.
//
// I clienti di questi test sono inventati: la famiglia è acme-punti delle grammatiche sintetiche (9.GGG.NNNN.T), con
// una tabella sul segmento «numero». Valori, descrizioni e fonti sono inventati.

// limitiConClassi: i limiti ACME con max_voci_classi dichiarato, come lo darebbe l'indice (R43 B).
func limitiConClassi() grammatica.Limiti {
	l := limitiACME()
	l.Grammatica.MaxVociClassi = 8
	return l
}

// famPuntiConClassi: acme-punti con la tabella delle classi sul segmento «numero».
func famPuntiConClassi() grammatica.FamigliaCodice {
	f := famPunti()
	f.Classi = &grammatica.TabellaClassi{Segmento: "numero", Voci: []grammatica.VoceClasse{
		{Valore: "4567", Categoria: grammatica.CategoriaCommerciale, Descrizione: "Boccola inventata", Fonte: "norma ACME 94567"},
		{Valore: "1111", Categoria: grammatica.CategoriaMinuteria, Descrizione: "Vite inventata"},
	}}
	return f
}

func motoreConClassi(t *testing.T, famiglie ...grammatica.FamigliaCodice) *Motore {
	t.Helper()
	m, d, err := compila(t, grammaticaACME(famiglie...), limitiConClassi())
	if err != nil || m == nil {
		t.Fatalf("la grammatica con la tabella non compila: %v\n%s", err, elenco(d))
	}
	return m
}

func TestClassiLetturaConLaVoce(t *testing.T) {
	m := motoreConClassi(t, famPuntiConClassi())
	casi := []struct {
		nome, selettore, testo string
		forma                  string
		classe                 *ClasseLetta
		categorie              []grammatica.Categoria
	}{
		{"completa con la voce commerciale", "corpo", "9.123.4567.3/01", "completa",
			&ClasseLetta{Segmento: "numero", Valore: "4567", Categoria: grammatica.CategoriaCommerciale, Descrizione: "Boccola inventata", Fonte: "norma ACME 94567"},
			[]grammatica.Categoria{grammatica.CategoriaCommerciale}},
		{"completa con la voce minuteria, senza fonte", "oggetto", "S9.123.1111.3", "completa",
			&ClasseLetta{Segmento: "numero", Valore: "1111", Categoria: grammatica.CategoriaMinuteria, Descrizione: "Vite inventata"},
			[]grammatica.Categoria{grammatica.CategoriaMinuteria}},
		// La forma parziale legge il segmento «numero» (manca solo T): la classe c'è anche lì.
		{"parziale che legge il segmento", "corpo", "9.123.4567", "parziale",
			&ClasseLetta{Segmento: "numero", Valore: "4567", Categoria: grammatica.CategoriaCommerciale, Descrizione: "Boccola inventata", Fonte: "norma ACME 94567"},
			[]grammatica.Categoria{grammatica.CategoriaCommerciale}},
		{"nessuna voce per il valore", "corpo", "9.123.2222.3", "completa", nil, nil},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			letture, d := riconosci(t, m, c.selettore, c.testo)
			if len(d) != 0 || len(letture) != 1 || letture[0].Forma != c.forma {
				t.Fatalf("letture %s, diagnostiche:\n%s", riassunto(letture), elenco(d))
			}
			l := letture[0]
			if !reflect.DeepEqual(l.Classe, c.classe) {
				t.Errorf("Classe %+v, attesa %+v", l.Classe, c.classe)
			}
			if !reflect.DeepEqual(l.Categorie, c.categorie) {
				t.Errorf("Categorie %v, attese %v", l.Categorie, c.categorie)
			}
		})
	}
	// La lettura è una copia: cambiarla non cambia il motore.
	prima, _ := riconosci(t, m, "corpo", "9.123.4567.3")
	prima[0].Classe.Descrizione = "alterata"
	prima[0].Categorie[0] = "alterata"
	dopo, _ := riconosci(t, m, "corpo", "9.123.4567.3")
	if dopo[0].Classe.Descrizione != "Boccola inventata" || dopo[0].Categorie[0] != grammatica.CategoriaCommerciale {
		t.Errorf("alterare una lettura cambia il motore: %+v", dopo[0])
	}
}

// TestClassiMaiuscoleEsatteNormalizzate: regressione del 14/10. Con le maiuscole esatte, normalizza «maiuscolo» e un
// segmento con lettere minuscole, «9.123.abcd.3» si legge con il segmento normalizzato «ABCD»: la voce «ABCD» passa la
// validazione (prima si rifiutava: il valore si controllava con il segmento non piegato) e la lettura la prende.
func TestClassiMaiuscoleEsatteNormalizzate(t *testing.T) {
	f := famPunti()
	f.Base.Segmenti[2].Pattern = "[0-9a-z]{4}"
	f.Base.Maiuscole, f.Base.Normalizza = grammatica.MaiuscoleEsatte, grammatica.NormalizzaMaiuscolo
	f.Classi = &grammatica.TabellaClassi{Segmento: "numero", Voci: []grammatica.VoceClasse{
		{Valore: "ABCD", Categoria: grammatica.CategoriaMinuteria, Descrizione: "Rondella inventata"},
	}}
	m := motoreConClassi(t, f)
	letture, d := riconosci(t, m, "corpo", "9.123.abcd.3")
	if len(d) != 0 || len(letture) != 1 {
		t.Fatalf("letture %s, diagnostiche:\n%s", riassunto(letture), elenco(d))
	}
	atteso := &ClasseLetta{Segmento: "numero", Valore: "ABCD", Categoria: grammatica.CategoriaMinuteria, Descrizione: "Rondella inventata"}
	if !reflect.DeepEqual(letture[0].Classe, atteso) {
		t.Errorf("Classe %+v, attesa %+v", letture[0].Classe, atteso)
	}
}

// TestClassiUnioneDelleCategorie: le categorie della lettura sono quelle della famiglia più quella della voce, in
// ordine e senza doppioni. Valida vieta categorie e tabella insieme, quindi l'unione si prova sul piano, in Go.
func TestClassiUnioneDelleCategorie(t *testing.T) {
	p := &pianoForma{classi: tabellaDi(famPuntiConClassi().Classi)}
	for _, c := range []struct {
		famiglia, attese []grammatica.Categoria
		valore           string
	}{
		{[]grammatica.Categoria{grammatica.CategoriaMinuteria}, []grammatica.Categoria{grammatica.CategoriaCommerciale, grammatica.CategoriaMinuteria}, "4567"},
		{[]grammatica.Categoria{grammatica.CategoriaMinuteria}, []grammatica.Categoria{grammatica.CategoriaMinuteria}, "1111"},
		{[]grammatica.Categoria{grammatica.CategoriaMinuteria}, []grammatica.Categoria{grammatica.CategoriaMinuteria}, "9999"},
	} {
		l := LetturaForma{Categorie: append([]grammatica.Categoria(nil), c.famiglia...),
			Base: BaseLetta{Segmenti: []SegmentoLetto{{Nome: "gruppo", Normalizzato: c.valore}, {Nome: "numero", Normalizzato: c.valore}}}}
		p.classifica(&l)
		if !reflect.DeepEqual(l.Categorie, c.attese) {
			t.Errorf("valore %s: categorie %v, attese %v", c.valore, l.Categorie, c.attese)
		}
		if (l.Classe != nil) != (c.valore != "9999") {
			t.Errorf("valore %s: Classe %+v", c.valore, l.Classe)
		}
	}
}

// TestClassiNellInterpretazione: la lettura del codice porta la Classe nella sua forma e le categorie unite (punto 6
// di Interpreta: le categorie vengono dalla lettura di forma, mai da un riconoscitore sul nome, R7).
func TestClassiNellInterpretazione(t *testing.T) {
	m := motoreConClassi(t, famPuntiConClassi())
	doc := docDSint([]evidenze.TestoOriginale{testoOrig(corpoDSint, "vedi 9.123.4567.3 e 9.123.2222.3")}, nil, nil,
		[]evidenze.UnitaEvidenza{uTesto(t, "u:corpo", "", "corpo", corpoDSint, "vedi 9.123.4567.3 e 9.123.2222.3", 0)})
	r := interpreta(t, m, doc, evidenze.UsoSconosciuto(bundleDSint))
	var con, senza int
	for _, l := range r.Letture {
		switch l.Forma.Base.Normalizzata {
		case "9.123.4567.3":
			con++
			if l.Forma.Classe == nil || l.Forma.Classe.Valore != "4567" || !reflect.DeepEqual(l.Categorie, []grammatica.Categoria{grammatica.CategoriaCommerciale}) {
				t.Errorf("lettura %s: Classe %+v, categorie %v", l.ID, l.Forma.Classe, l.Categorie)
			}
		case "9.123.2222.3":
			senza++
			if l.Forma.Classe != nil || l.Categorie != nil {
				t.Errorf("lettura %s senza voce: Classe %+v, categorie %v", l.ID, l.Forma.Classe, l.Categorie)
			}
		}
	}
	if con != 1 || senza != 1 {
		t.Fatalf("letture %d con la voce e %d senza, attese 1 e 1", con, senza)
	}
	for _, l := range r.Letture {
		b, err := json.Marshal(l.Forma)
		if err != nil {
			t.Fatal(err)
		}
		con := l.Forma.Base.Normalizzata == "9.123.4567.3"
		if con && !strings.Contains(string(b), `"Classe":{"Segmento":"numero","Valore":"4567","Categoria":"commerciale","Descrizione":"Boccola inventata","Fonte":"norma ACME 94567"}`) {
			t.Errorf("JSON della lettura con la Classe: %s", b)
		}
		if !con && strings.Contains(string(b), `"Classe"`) {
			t.Errorf("JSON della lettura senza voce con la chiave Classe: %s", b)
		}
	}
}

// impronteSenzaTabelle: lo sha256 del JSON delle letture delle grammatiche sintetiche su alcuni testi, e quello del
// JSON di un'interpretazione con la sua impronta. È scritta in modo da compilare anche sul codice di prima della
// tabella (aabe52c), dove sono stati calcolati i valori di TestClassiJSONComePrima.
func impronteSenzaTabelle(t *testing.T) (letture, interpretazione, impronta string) {
	t.Helper()
	m, _ := compilaBene(t, grammaticaACME(tutteLeFamiglie()...))
	var tutte []LetturaForma
	for _, c := range []struct{ s, testo string }{
		{"corpo", "P7120100 9123456A2 9.123.4567.3/01 9.123.4567 9.123.4567.3/xx"},
		{"oggetto", "S9.123.4567.3 9.123.4567.3_30"},
		{"nome_file", "6123456_6123456.pdf"},
	} {
		l, _ := m.Riconosci(selettore(t, c.s), c.testo)
		tutte = append(tutte, l...)
	}
	b, err := json.Marshal(tutte)
	if err != nil {
		t.Fatal(err)
	}
	doc := docDSint(nil, nil, []evidenze.EntitaLocale{entDisegno("e:pdf:p1")},
		[]evidenze.UnitaEvidenza{
			uCampo(t, "u:01", "e:pdf:p1", "cartiglio.codice", "ACME1111"),
			uCampo(t, "u:02", "e:pdf:p1", "cartiglio.revisione", "01"),
		})
	r := interpreta(t, motoreCodice(t), doc, evidenze.UsoSconosciuto(bundleDSint))
	b2, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	s1, s2 := sha256.Sum256(b), sha256.Sum256(b2)
	return hex.EncodeToString(s1[:]), hex.EncodeToString(s2[:]), r.Impronta
}

// TestClassiJSONComePrima: senza tabelle delle classi il JSON delle letture è byte per byte quello del codice di prima
// (aabe52c): Classe ha omitempty, e dal 16/10 anche Emittente, HashEmittente e Ruolo (GP-1). L'interpretazione e la
// sua impronta cambiano solo perché cambiano le versioni (motorea-2, risultato 2: GP-1, addendum §0): i valori di
// prima erano interpretazione 223ab91f… e impronta fb98a20e….
func TestClassiJSONComePrima(t *testing.T) {
	const (
		lettureprima         = "e488212178a72f18d794ac26cf3ab2a220095f149f761b601412a3dd84575799"
		interpretazioneprima = "b0c8ad6aecc5b6495d31b80281891d7165c3e11c0da9f263361d18b34cef9ea9"
		improntaprima        = "c22bf03f3f839f735ef4f822ff292fbc637bf60fa1a5017f07c78047c9e826ef"
	)
	l, i, imp := impronteSenzaTabelle(t)
	if l != lettureprima || i != interpretazioneprima || imp != improntaprima {
		t.Fatalf("senza tabelle il JSON è cambiato: letture %s, interpretazione %s, impronta %s", l, i, imp)
	}
	if VersioneAlgoritmo != "motorea-2" || VersioneRisultato != 2 {
		t.Fatalf("versioni del motore cambiate: %q %d", VersioneAlgoritmo, VersioneRisultato)
	}
}
