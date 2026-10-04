package motorea

import (
	"reflect"
	"regexp/syntax"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L1 — il contratto di Riconosci (piano A, par.3.3.4, 4.4.5; v3 §4.3; P1 §7.3; T9): ogni piano si applica da
// solo, mai in alternanza con gli altri, quindi famiglie diverse sulla stessa occorrenza danno tutte la loro
// lettura; le letture escono in ordine di (inizio, fine, famiglia, forma), con famiglia, namespace, forma,
// selettore e originale; un selettore senza piani non legge niente e non dà diagnostiche; le maiuscole
// indifferenti valgono solo per le lettere ASCII; il motore è immutabile e conserva il suo snapshot.
//
// I clienti di questi test sono inventati: il repository è pubblico, e le famiglie di codice dei clienti veri
// sono un dato dell'azienda. Oltre alle grammatiche di sintetiche_test.go, qui ci sono tre famiglie ACME
// minime con la stessa base, per l'ordine.

// famStessaBase: una famiglia con la base di acme-prefisso e le forme date, tutte sul corpo.
func famStessaBase(id string, forme ...grammatica.FormaCodice) grammatica.FamigliaCodice {
	f := grammatica.FamigliaCodice{
		ID: id, Namespace: id,
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto},
		Base:  base(segPattern("codice", "712[0-9]{4}", "")),
		Forme: forme,
	}
	for _, fo := range forme {
		f.Esempi = append(f.Esempi, grammatica.EsempioCodice{ID: "e-" + fo.ID, Origine: grammatica.OrigineSintetico,
			Selettore: "corpo", Testo: "7120100-X1",
			Atteso: grammatica.AttesoEsempio{Letture: []grammatica.LetturaAttesa{{Forma: fo.ID}}, AltreAmmesse: true}})
	}
	return f
}

func TestRiconosciOgniPianoDaSoloEInOrdine(t *testing.T) {
	g := grammaticaACME(
		famStessaBase("acme-c", forma("corta", sel("corpo"), pBase())),
		famStessaBase("acme-b", forma("lunga", sel("corpo"), pBase(), pSep("-"), pToken("Q1", "X1"))),
		famStessaBase("acme-a", forma("corta", sel("corpo"), pBase())),
	)
	m, _ := compilaBene(t, g)
	letture, d := riconosci(t, m, "corpo", "7120100-X1")
	if len(d) != 0 {
		t.Errorf("diagnostiche:\n%s", elenco(d))
	}
	attese := []struct {
		famiglia, forma string
		fine            int
	}{{"acme-a", "corta", 7}, {"acme-c", "corta", 7}, {"acme-b", "lunga", 10}}
	if len(letture) != len(attese) {
		t.Fatalf("letture %s, attese tre: un piano non nasconde gli altri", riassunto(letture))
	}
	for i, a := range attese {
		l := letture[i]
		if l.Famiglia != a.famiglia || l.Forma != a.forma || l.Intervallo.Inizio != 0 || l.Intervallo.Fine != a.fine {
			t.Errorf("lettura %d: %s/%s %v, attesa %s/%s [0,%d)", i, l.Famiglia, l.Forma, l.Intervallo, a.famiglia, a.forma, a.fine)
		}
		if l.Namespace != a.famiglia {
			t.Errorf("lettura %d: namespace %q", i, l.Namespace)
		}
	}
}

func TestRiconosciSelettoreSenzaPiani(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(tutteLeFamiglie()...))
	for _, s := range []string{"testo_pdf", "metadati_pdf", "voce_archivio", "cartiglio.revisione"} {
		letture, d := m.Riconosci(selettore(t, s), "P7120100 9123456A2 T+300.012345.010 01")
		if len(letture) != 0 || len(d) != 0 {
			t.Errorf("%s: letture %s, diagnostiche:\n%s", s, riassunto(letture), elenco(d))
		}
	}
	if letture, d := riconosci(t, m, "corpo", ""); len(letture) != 0 || len(d) != 0 {
		t.Errorf("testo vuoto: %s\n%s", riassunto(letture), elenco(d))
	}
}

func TestRiconosciSnapshotEImmutabilita(t *testing.T) {
	g := grammaticaACME(famPrefisso())
	s, err := grammatica.NuovoSnapshot(fileJSON(t, g), limitiACME())
	if err != nil {
		t.Fatal(err)
	}
	m, d, err := CompilaVerificato(s, limitiACME())
	if err != nil {
		t.Fatalf("%v\n%s", err, elenco(d))
	}
	if m.Snapshot().Hash != s.Hash || m.Snapshot().ClienteID != s.ClienteID || m.Snapshot().VersioneLimiti != s.VersioneLimiti {
		t.Errorf("Snapshot() non è lo snapshot ricevuto")
	}
	prima, _ := riconosci(t, m, "corpo", "P7120100")
	prima[0].Affissi[0].Valore.Fase = "alterata"
	prima[0].Base.Segmenti[0].Normalizzato = "alterato"
	dopo, _ := riconosci(t, m, "corpo", "P7120100")
	if dopo[0].Affissi[0].Valore.Fase != grammatica.FasePrototipo || dopo[0].Base.Segmenti[0].Normalizzato != "7120100" {
		t.Errorf("alterare una lettura cambia il motore: %+v", dopo[0])
	}
}

// TestRiconosciMaiuscoleEsatte: con maiuscole esatte una lettera di un'altra maiuscola non si legge.
func TestRiconosciMaiuscoleEsatte(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPrefisso(), famMarcatore()))
	if letture, _ := riconosci(t, m, "corpo", "p7120100"); len(letture) != 0 {
		t.Errorf("«p» minuscola letta come la P: %s", riassunto(letture))
	}
	if letture, _ := riconosci(t, m, "nome_file", "9123456a_2.pdf"); len(letture) != 0 {
		t.Errorf("marcatore minuscolo letto come «A»: %s", riassunto(letture))
	}
}

// TestRiconosciMaiuscoleIndifferentiASCII: con maiuscole indifferenti_ascii e la normalizzazione in
// maiuscolo la base si legge con tutte e due le maiuscole e si normalizza; l'originale resta com'è. Le
// lettere fuori dall'ASCII non si piegano: niente (?i), che leggerebbe il segno del kelvin come una K (T9).
func TestRiconosciMaiuscoleIndifferentiASCII(t *testing.T) {
	f := grammatica.FamigliaCodice{
		ID: "acme-maiuscole", Namespace: "acme-maiuscole",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloComponente},
		Base:  base(segLetterale("serie", "KB", ""), segPattern("numero", "[A-C][0-9]{4}", "")),
		Forme: []grammatica.FormaCodice{forma("corpo", sel("corpo"), pBase())},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-minuscole", "corpo", "kb-a1234 kbA1234", grammatica.LetturaAttesa{Forma: "corpo", Base: "KBA1234"}),
		},
	}
	f.Base.Maiuscole = grammatica.MaiuscoleIndifferentiASCII
	f.Base.Normalizza = grammatica.NormalizzaMaiuscolo
	m, _ := compilaBene(t, grammaticaACME(f))
	for _, testo := range []string{"KBA1234", "kba1234", "kBa1234", "KbC9999"} {
		letture, _ := riconosci(t, m, "corpo", testo)
		l := unaLettura(t, letture, "acme-maiuscole", "corpo")
		want := ""
		for _, c := range []byte(testo) {
			if 'a' <= c && c <= 'z' {
				c -= 'a' - 'A'
			}
			want += string(c)
		}
		if l.Base.Normalizzata != want || l.Base.Originale != testo || l.Originale != testo {
			t.Errorf("%q: normalizzata %q, originale %q", testo, l.Base.Normalizzata, l.Base.Originale)
		}
	}
	for _, testo := range []string{"KB" + "A1234", "KBD1234", "ＫＢA1234"} {
		if letture, _ := riconosci(t, m, "corpo", testo); len(letture) != 0 {
			t.Errorf("%q: nessuna lettura attesa, trovate %s", testo, riassunto(letture))
		}
	}
}

// TestRiconosciVinceLaLetturaPiuLunga: a parità d'inizio vince la lettura più lunga che rispetta il confine,
// qualunque sia l'ordine dei letterali e delle parti facoltative (T6), anche quando la parte facoltativa
// finale è un solo segno e il testo finisce lì.
func TestRiconosciVinceLaLetturaPiuLunga(t *testing.T) {
	f := famMarcatore()
	for i := range f.Forme {
		if f.Forme[i].ID == "dxf" {
			f.Forme[i].Parti[len(f.Forme[i].Parti)-1] = pToken("D10", "1", "12")
		}
	}
	m, _ := compilaBene(t, grammaticaACME(f))
	letture, _ := riconosci(t, m, "nome_file", "dxf_9123456a_drw_12.dxf")
	l := unaLettura(t, letture, "acme-marcatore", "dxf")
	if l.Token == nil || l.Token.Valore != "12" {
		t.Errorf("token %+v, atteso il letterale più lungo «12»", l.Token)
	}

	segno := grammatica.FamigliaCodice{
		ID: "acme-segno", Namespace: "acme-segno",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto},
		Base:  base(segPattern("codice", "712[0-9]{4}", "")),
		Forme: []grammatica.FormaCodice{forma("corpo", sel("corpo"), pBase(), pRif(grammatica.TipoParteAffisso, "piu", 0))},
		Affissi: []grammatica.Affisso{{ID: "piu", Letterali: []string{"+"}, Posizione: grammatica.PosizioneSuffisso,
			Riconoscimento: sel("corpo")}},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-segno", "corpo", "7120100+ altro", grammatica.LetturaAttesa{Forma: "corpo", Base: "7120100", Affissi: []string{"+"}}),
		},
	}
	ms, _ := compilaBene(t, grammaticaACME(segno))
	for _, testo := range []string{"7120100+", "7120100+ altro", "(7120100+)"} {
		letture, _ := riconosci(t, ms, "corpo", testo)
		l := unaLettura(t, letture, "acme-segno", "corpo")
		if l.Originale != "7120100+" || len(l.Affissi) != 1 || l.CodiceRichiesto != "7120100+" {
			t.Errorf("%q: originale %q, affissi %+v: attesa la lettura più lunga, con il segno", testo, l.Originale, l.Affissi)
		}
	}
	letture, _ = riconosci(t, ms, "corpo", "7120100-")
	if l := unaLettura(t, letture, "acme-segno", "corpo"); l.Originale != "7120100" || len(l.Affissi) != 0 {
		t.Errorf("senza il segno: originale %q, affissi %+v", l.Originale, l.Affissi)
	}
}

// TestRiconosciParteFacoltativaIncompleta: una revisione facoltativa scritta a metà non entra nella lettura,
// e non la inventa: resta la base, con il confine rispettato.
func TestRiconosciParteFacoltativaIncompleta(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPunti()))
	letture, _ := riconosci(t, m, "corpo", "9.123.4567.3/0")
	l := unaLettura(t, letture, "acme-punti", "completa")
	if l.Revisione != nil || l.Originale != "9.123.4567.3" {
		t.Errorf("revisione %+v, originale %q: «/0» non è una revisione di due cifre", l.Revisione, l.Originale)
	}
}

// TestRiconosciTestoNonUTF8: un testo con byte non UTF-8 non arriva dal documento validato (ValidaDocumento lo
// rifiuta), ma Riconosci non deve fermarsi: legge il codice fra i byte sbagliati, senza panico.
func TestRiconosciTestoNonUTF8(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPrefisso()))
	letture, _ := m.Riconosci(selettore(t, "corpo"), "\xffP7120100\xfe\x80")
	if len(letture) != 1 || letture[0].Intervallo.Inizio != 1 || letture[0].Intervallo.Fine != 9 {
		t.Errorf("letture %s, attesa una in [1,9)", riassunto(letture))
	}
}

// TestRiconosciLetteraliConMetacaratteri: i letterali si confrontano come testo (QuoteMeta, T16): il «+» del
// prefisso letterale non è una ripetizione, e un «.» non è un carattere qualunque.
func TestRiconosciLetteraliConMetacaratteri(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famCampoSeparato()))
	for _, testo := range []string{"TT300.012345.010.pdf", "T300.012345.010.pdf", "T+300x012345.010.pdf", "T+300.012345x010.pdf"} {
		if letture, _ := riconosci(t, m, "nome_file", testo); len(letture) != 0 {
			t.Errorf("%q: nessuna lettura attesa, trovate %s", testo, riassunto(letture))
		}
	}
	letture, _ := riconosci(t, m, "nome_file", "T+300.012345.010.pdf")
	l := unaLettura(t, letture, "acme-campo-separato", "nome")
	if l.Base.Normalizzata != "T+300.012345.010" || len(l.Base.Segmenti) != 3 || l.Base.Segmenti[0].Originale != "T+300" {
		t.Errorf("base %+v: il «+» resta nella base originale e normalizzata", l.Base)
	}
}

// TestRiconosciFinestraDiRicerca: la regex di un piano vede da ogni inizio solo i byte della lettura più
// lunga più quelli del confine (scansione.go). Sugli esempi di tutte le grammatiche, anche dentro testi
// lunghi, le letture e le diagnostiche sono le stesse che con la ricerca su tutta la coda del testo.
func TestRiconosciFinestraDiRicerca(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(tutteLeFamiglie()...))
	type caso struct {
		sel   evidenze.Selettore
		testo string
	}
	var casi []caso
	riempitivo := strings.Repeat("riga di testo, ", 200)
	for _, f := range tutteLeFamiglie() {
		for _, e := range f.Esempi {
			s, err := evidenze.LeggiSelettore(e.Selettore)
			if err != nil {
				continue
			}
			casi = append(casi, caso{s, e.Testo}, caso{s, riempitivo + e.Testo}, caso{s, e.Testo + " " + riempitivo},
				caso{s, riempitivo + e.Testo + " " + e.Testo + " " + riempitivo})
		}
	}
	type esito struct {
		letture []LetturaForma
		diag    []evidenze.Diagnostica
	}
	prima := make([]esito, len(casi))
	for i, c := range casi {
		prima[i].letture, prima[i].diag = m.Riconosci(c.sel, c.testo)
	}
	limitati := 0
	for _, piani := range m.piani {
		for _, p := range piani {
			if p.maxByte >= 0 {
				limitati++
			}
			p.maxByte = -1 // solo nella prova: la ricerca su tutta la coda
		}
	}
	if limitati == 0 {
		t.Fatal("nessun piano con una lunghezza massima: la finestra non è provata")
	}
	for i, c := range casi {
		letture, diag := m.Riconosci(c.sel, c.testo)
		if !reflect.DeepEqual(letture, prima[i].letture) || !reflect.DeepEqual(diag, prima[i].diag) {
			t.Errorf("caso %d: con la finestra %s, senza %s", i, riassunto(prima[i].letture), riassunto(letture))
		}
	}
}

// TestLunghezzaMassima: i byte più lunghi di una lettura, contando ogni runa per i byte della più lunga che
// la classe ammette; -1 senza un limite.
func TestLunghezzaMassima(t *testing.T) {
	casi := map[string]int{
		`712[0-9]{4}`:       7,
		`(?:AB|CDE)?é`:      5,
		`[0-9]{1,3}[^0-9]`:  7,
		`x[0-9]+`:           -1,
		`(?:a|b)*`:          -1,
		`\.[0-9]{2}|_[A-Z]`: 3,
	}
	for _, p := range []string{`712[0-9]{4}`, `(?:AB|CDE)?é`, `[0-9]{1,3}[^0-9]`, `x[0-9]+`, `(?:a|b)*`, `\.[0-9]{2}|_[A-Z]`} {
		albero, err := syntax.Parse(p, syntax.Perl)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if n := lunghezzaMassima(albero); n != casi[p] {
			t.Errorf("%s: %d, attesa %d", p, n, casi[p])
		}
	}
}
