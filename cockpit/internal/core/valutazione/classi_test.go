package valutazione_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
)

// L1 — la tabella delle classi del cliente nella valutazione (decisione dell'utente del 14/10, [U]): la proposta della
// categoria (minuteria prima, poi commerciale), mai un'esenzione dal 2D (R103 C, R99 A); il tipo proposto commerciale
// con l'origine proposto_dalla_grammatica solo per un nodo senza decisione e senza figli («il commerciale è SEMPRE una
// foglia»); NodoBOM.Classe con la voce della lettura scelta.
//
// I clienti di questi test sono inventati: ACME, con la famiglia acme-catena (712xxxx con il marcatore) e una famiglia
// acme-classi (712 più una cifra di serie e tre di articolo, senza marcatore) con la tabella sul segmento «serie».
// Valori, descrizioni e fonti sono inventati.

// famigliaClassi: acme-classi, 712 + serie (una cifra) + articolo (tre cifre), con la tabella delle classi sulla serie:
// 9 → minuteria, 8 → commerciale. Come acme-minuteria, non legge i codici con il marcatore (il confine dopo la base).
func famigliaClassi() grammatica.FamigliaCodice {
	base := grammatica.Base{
		Segmenti: []grammatica.SegmentoBase{
			{Nome: "radice", Letterale: "712", Identitario: true},
			{Nome: "serie", Pattern: "[0-9]", Identitario: true},
			{Nome: "articolo", Pattern: "[0-9]{3}", Identitario: true},
		},
		Maiuscole: grammatica.MaiuscoleEsatte, Normalizza: grammatica.NormalizzaNessuna,
		ConfinePrima: grammatica.ConfineAlnumASCII, ConfineDopo: grammatica.ConfineAlnumASCII,
	}
	return grammatica.FamigliaCodice{
		ID: "acme-classi", Namespace: "acme-classi",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloComponente},
		Classi: &grammatica.TabellaClassi{Segmento: "serie", Voci: []grammatica.VoceClasse{
			{Valore: "9", Categoria: grammatica.CategoriaMinuteria, Descrizione: "Vite inventata", Fonte: "norma ACME 9"},
			{Valore: "8", Categoria: grammatica.CategoriaCommerciale, Descrizione: "Boccola inventata", Fonte: "norma ACME 8"},
		}},
		Base: base,
		Forme: []grammatica.FormaCodice{formaCatena("codice", []string{"nome_file", "cartiglio.codice", "radice_step.id", "nodo_step.id"},
			parteC(grammatica.TipoParteBase))},
		Esempi: []grammatica.EsempioCodice{{ID: "e-classi", Origine: grammatica.OrigineSintetico, Selettore: "nodo_step.id", Testo: "7129001",
			Atteso: grammatica.AttesoEsempio{Letture: []grammatica.LetturaAttesa{{Forma: "codice", Base: "7129001"}}}}},
	}
}

// insiemeConClassi: le regole ACME con acme-catena e acme-classi, dall'indice con max_voci_classi dichiarato.
func insiemeConClassi(t *testing.T) *motorea.InsiemeRegole {
	t.Helper()
	raw := grammaticaDi(t, clienteACME, "ACME S.p.A.", famigliaCatena(), famigliaClassi())
	lim := limitiACME()
	lim.Grammatica.MaxVociClassi = 8
	h := sha256.Sum256(raw)
	ind := grammatica.IndiceRegole{VersioneIndice: grammatica.VersioneIndice, Limiti: lim,
		Grammatiche: []grammatica.VoceIndice{{ClienteID: clienteACME, File: "acme.v1.json", Sha256: hex.EncodeToString(h[:])}}}
	r, _, err := motorea.CompilaInsieme(ind, map[string][]byte{"acme.v1.json": raw})
	if err != nil || r.Motori[clienteACME] == nil {
		t.Fatalf("CompilaInsieme con la tabella: %v, scartati %v", err, r.Scartati)
	}
	return &r
}

// scenaClassi: lo STEP dell'assieme senza la fonte confermata (una struttura candidata): la radice #1 7120100A, sotto
// #2 7128002 (serie 8, commerciale nella tabella) che ha a sua volta il figlio #3 7129003 (serie 9, minuteria), e la
// foglia #4 7128004 (serie 8). Nessun nodo è deciso.
func scenaClassi(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7128002", ""}, {"#3", "7129003", ""}, {"#4", "7128004", ""}},
		[]arcoF{{"#1", "#2", 1}, {"#2", "#3", 4}, {"#1", "#4", 2}})
	senzaFonte(&th)
	th.Fabbisogni = fabbisogniDefault()
	return th
}

func TestClassiNeiNodiDellaStruttura(t *testing.T) {
	et := esitoSmistamentoCon(t, scenaClassi(t), insiemeConClassi(t))
	p := prodottoDi(t, et, rifProdB)
	if len(p.StruttureCandidate) != 1 {
		t.Fatalf("strutture candidate %v", chiaviCandidate(p))
	}
	nodi := map[string]valutazione.NodoBOM{}
	for _, n := range p.StruttureCandidate[0].Nodi {
		nodi[n.Nodo.Rif] = n
	}
	radice, conFigli, minuteria, foglia := nodi[nodoB("#1")], nodi[nodoB("#2")], nodi[nodoB("#3")], nodi[nodoB("#4")]

	// La radice è della famiglia della catena: nessuna voce, la catena di prima.
	if radice.Classe != nil || tipoDi(radice) != "sottoassieme/proposto_dalla_regola" || radice.Classificazione.Proposta != "" {
		t.Errorf("la radice senza tabella: %s, Classe %+v, proposta %q", tipoDi(radice), radice.Classe, radice.Classificazione.Proposta)
	}
	// Un nodo con dei figli e una voce commerciale: il tipo resta quello della regola, la voce si vede.
	if tipoDi(conFigli) != "sottoassieme/proposto_dalla_regola" ||
		!reflect.DeepEqual(conFigli.Classe, &valutazione.ClasseNodo{Categoria: "commerciale", Descrizione: "Boccola inventata", Fonte: "norma ACME 8", Valore: "8"}) ||
		conFigli.Classificazione.Proposta != valutazione.CategoriaCommerciale {
		t.Errorf("il nodo con i figli: %s, Classe %+v, classificazione %+v", tipoDi(conFigli), conFigli.Classe, conFigli.Classificazione)
	}
	// Le foglie con una voce: commerciale proposto dalla grammatica; la minuteria resta la proposta della categoria.
	if tipoDi(minuteria) != "commerciale/proposto_dalla_grammatica" ||
		!reflect.DeepEqual(minuteria.Classe, &valutazione.ClasseNodo{Categoria: "minuteria", Descrizione: "Vite inventata", Fonte: "norma ACME 9", Valore: "9"}) {
		t.Errorf("la foglia minuteria: %s, Classe %+v", tipoDi(minuteria), minuteria.Classe)
	}
	if c := minuteria.Classificazione; c.Proposta != valutazione.CategoriaMinuteria || c.Categoria != valutazione.CategoriaNonDeterminata || c.Confermata ||
		valutazione.EsenteDal2D(c) {
		t.Errorf("la classificazione della foglia minuteria: %+v", c)
	}
	if tipoDi(foglia) != "commerciale/proposto_dalla_grammatica" || foglia.Classe == nil || foglia.Classe.Categoria != "commerciale" {
		t.Errorf("la foglia commerciale: %s, Classe %+v", tipoDi(foglia), foglia.Classe)
	}
	if c := foglia.Classificazione; c.Proposta != valutazione.CategoriaCommerciale || valutazione.EsenteDal2D(c) {
		t.Errorf("la classificazione della foglia commerciale: %+v", c)
	}
	// Il 2D resta previsto per le due foglie: il tipo proposto non esenta (FabbisogniDelComponente).
	for _, rif := range []string{nodoB("#3"), nodoB("#4")} {
		trovato := false
		for _, pr := range p.Documenti.Previsti {
			if pr.Nodo == rif && pr.TipoDocumento == "disegno_2d" {
				trovato = true
			}
		}
		if !trovato {
			t.Errorf("%s: il 2D non è previsto (previsti %+v)", rif, p.Documenti.Previsti)
		}
	}
	// Il JSON del nodo porta la voce; quello di un nodo senza voce non ha la chiave.
	b, _ := json.Marshal(foglia)
	if !strings.Contains(string(b), `"classe":{"categoria":"commerciale","descrizione":"Boccola inventata","fonte":"norma ACME 8","valore":"8"}`) {
		t.Errorf("JSON della foglia: %s", b)
	}
	if b, _ := json.Marshal(radice); strings.Contains(string(b), `"classe"`) {
		t.Errorf("JSON della radice con la chiave classe: %s", b)
	}
}

// TestClassiLaPropostaDellaCategoria: la regola pura (Classifica) con le categorie della grammatica.
func TestClassiLaPropostaDellaCategoria(t *testing.T) {
	for _, c := range []struct {
		nome      string
		tipo      string
		categorie []string
		proposta  string
	}{
		{"commerciale su un nodo senza tipo", "", []string{"commerciale"}, valutazione.CategoriaCommerciale},
		{"commerciale su uno sciolto", "sciolto", []string{"commerciale"}, valutazione.CategoriaCommerciale},
		{"commerciale su un commerciale: niente da proporre", "commerciale", []string{"commerciale"}, ""},
		{"minuteria e commerciale: la minuteria prima", "", []string{"commerciale", "minuteria"}, valutazione.CategoriaMinuteria},
		{"minuteria su un commerciale", "commerciale", []string{"minuteria"}, valutazione.CategoriaMinuteria},
		{"nessuna categoria", "sciolto", nil, ""},
	} {
		t.Run(c.nome, func(t *testing.T) {
			cl := valutazione.Classifica(valutazione.ComponenteDaClassificare{Tipo: c.tipo, Categorie: c.categorie}, nil)
			if cl.Proposta != c.proposta || valutazione.EsenteDal2D(cl) {
				t.Errorf("proposta %q, attesa %q; classificazione %+v", cl.Proposta, c.proposta, cl)
			}
		})
	}
}
