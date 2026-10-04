package motorea

import (
	"strings"
	"testing"

	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L1 — i confini del codice (A1a-CNF pubblica; piano A, par.4.4.5 e 4.7.2; v3 §1.5, §7.4; T2-T5, T13-T15;
// D-05, R25 f): codici accanto ad altri codici e a separatori, codice dopo le cifre di un prefisso, due basi
// in un nome di pacchetto, offset in byte con accenti, emoji e spazio non separabile accanto, nessuna lettura
// dentro una parola, e le quattro classi di confine, ognuna con il caso che la giustifica e con la lettura in
// più che darebbe la grammatica senza la classe.
//
// I clienti di questi test sono inventati: il repository è pubblico, e le famiglie di codice dei clienti veri
// sono un dato dell'azienda. Le grammatiche sono quelle ACME di sintetiche_test.go.

// TestConfiniDueCodiciSeparati: due codici separati da «_», da uno spazio o da una virgola danno due
// letture; attaccati non ne danno nessuna. Dopo un inizio la scansione riparte dalla runa successiva, mai
// dalla fine del match: il secondo codice non si perde (T2, T5, T13).
func TestConfiniDueCodiciSeparati(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPrefisso()))
	for _, sep := range []string{"_", " ", ",", ", ", ";", "/", "\n"} {
		testo := "P7120100" + sep + "P7120101"
		letture, _ := riconosci(t, m, "corpo", testo)
		if len(letture) != 2 {
			t.Errorf("separatore %q: attese 2 letture, trovate %s", sep, riassunto(letture))
			continue
		}
		if letture[0].Base.Normalizzata != "7120100" || letture[1].Base.Normalizzata != "7120101" {
			t.Errorf("separatore %q: basi %q e %q", sep, letture[0].Base.Normalizzata, letture[1].Base.Normalizzata)
		}
		if letture[1].Intervallo.Inizio != 8+len(sep) || letture[1].Intervallo.Fine != len(testo) {
			t.Errorf("separatore %q: seconda lettura in %v", sep, letture[1].Intervallo)
		}
	}
	if letture, _ := riconosci(t, m, "corpo", "P7120100P7120101"); len(letture) != 0 {
		t.Errorf("due codici attaccati: nessuna lettura attesa, trovate %s", riassunto(letture))
	}
}

// TestConfiniCodiceDopoLeCifreDiUnPrefisso: nel nome «ACME-030P7120100.pdf» il codice segue le cifre
// dell'involucro: la forma del nome lo legge con involucro, P e base, la base sta dopo la P, e nessun'altra
// lettura nasce dentro il nome (v3 §1.5; D2).
func TestConfiniCodiceDopoLeCifreDiUnPrefisso(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(tutteLeFamiglie()...))
	testo := "ACME-030P7120100.pdf"
	letture, _ := riconosci(t, m, "nome_file", testo)
	l := unaLettura(t, letture, "acme-prefisso", "nome-pdf")
	if l.Intervallo.Inizio != 0 || l.Intervallo.Fine != len("ACME-030P7120100") {
		t.Errorf("lettura in %v, attesa [0,%d)", l.Intervallo, len("ACME-030P7120100"))
	}
	if l.Base.Normalizzata != "7120100" {
		t.Errorf("base %q", l.Base.Normalizzata)
	}
	if len(l.Base.Segmenti) != 1 || l.Base.Segmenti[0].Intervallo.Inizio != strings.Index(testo, "7120100") {
		t.Errorf("segmenti della base %+v: la base comincia dopo la P", l.Base.Segmenti)
	}
	if len(l.Affissi) != 1 || l.Affissi[0].Originale != "P" {
		t.Errorf("affissi %+v", l.Affissi)
	}
	if len(l.Decorazioni) != 1 || l.Decorazioni[0].Valore != "ACME-030" || l.Decorazioni[0].Tipo != grammatica.TipoDecorazioneInvolucro {
		t.Errorf("decorazioni %+v", l.Decorazioni)
	}
}

// TestConfiniDueBasiNelNomeDelPacchetto: «9123456_9123457.zip» dà le due basi, la seconda non si perde
// (v3 §1.4, §1.5).
func TestConfiniDueBasiNelNomeDelPacchetto(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(tutteLeFamiglie()...))
	letture, _ := riconosci(t, m, "nome_file", "9123456_9123457.zip")
	if len(letture) != 2 {
		t.Fatalf("attese 2 letture, trovate %s", riassunto(letture))
	}
	for i, atteso := range []struct {
		base         string
		inizio, fine int
	}{{"9123456", 0, 7}, {"9123457", 8, 15}} {
		l := letture[i]
		if l.Famiglia != "acme-marcatore" || l.Forma != "pacchetto" || l.Base.Normalizzata != atteso.base ||
			l.Intervallo.Inizio != atteso.inizio || l.Intervallo.Fine != atteso.fine {
			t.Errorf("lettura %d: %s/%s base %q in %v, attesa pacchetto %q in [%d,%d)", i, l.Famiglia, l.Forma,
				l.Base.Normalizzata, l.Intervallo, atteso.base, atteso.inizio, atteso.fine)
		}
	}
}

// TestConfiniOffsetInByte: gli intervalli sono in byte UTF-8 sul testo dato, anche con accenti, emoji e
// spazio non separabile accanto al codice (par.3.4.4). Le classi di confine sono solo ASCII: una lettera
// accentata o un'emoji accanto al codice non sono lettere o cifre ASCII, quindi il codice si legge (T3, T10).
func TestConfiniOffsetInByte(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPrefisso()))
	casi := []struct {
		nome, testo string
	}{
		{"accento prima, staccato", "Codice è P7120100, grazie"},
		{"accento attaccato prima", "èP7120100"},
		{"accento attaccato dopo", "P7120100è"},
		{"emoji ai due lati", "😀P7120100😀"},
		{"spazio non separabile", " P7120100 "},
		{"spazio stretto non separabile", "Rif. P7120100"},
		{"testo misto", "Città: «P7120100» — però"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			letture, _ := riconosci(t, m, "corpo", c.testo)
			l := unaLettura(t, letture, "acme-prefisso", "mail")
			inizio := strings.Index(c.testo, "P7120100")
			if l.Intervallo.Inizio != inizio || l.Intervallo.Fine != inizio+len("P7120100") {
				t.Errorf("intervallo %v, atteso [%d,%d) in byte", l.Intervallo, inizio, inizio+len("P7120100"))
			}
			if len(l.Base.Segmenti) != 1 || l.Base.Segmenti[0].Intervallo.Inizio != inizio+1 {
				t.Errorf("segmenti %+v: la base comincia al byte %d", l.Base.Segmenti, inizio+1)
			}
		})
	}
}

// TestConfiniNessunaLetturaDentroUnaParola: una lettera o una cifra ASCII accanto al codice, a sinistra o a
// destra, toglie la lettura. Il confine sinistro si guarda sul testo intero: cercando su testo[i:] la runa
// prima non si vedrebbe (T4).
func TestConfiniNessunaLetturaDentroUnaParola(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPrefisso(), famPunti()))
	for _, testo := range []string{"XP7120100", "AP7120100", "1P7120100", "P7120100X", "P71201001", "zP7120100z",
		"a9.123.4567.3", "9.123.4567.3b", "_P7120100x"} {
		if letture, _ := riconosci(t, m, "corpo", testo); len(letture) != 0 {
			t.Errorf("%q: nessuna lettura attesa, trovate %s", testo, riassunto(letture))
		}
	}
}

// TestConfiniClasseAlnumASCII: la classe predefinita vieta solo lettere e cifre ASCII; «_», «-», «.» e la
// fine del testo sono confini.
func TestConfiniClasseAlnumASCII(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPrefisso()))
	for _, testo := range []string{"P7120100_x", "x_P7120100", "-P7120100-", "P7120100.", "(P7120100)"} {
		letture, _ := riconosci(t, m, "corpo", testo)
		unaLettura(t, letture, "acme-prefisso", "mail")
	}
}

// TestConfiniClasseParolaASCII: con il confine destro parola_ascii la forma STEP non legge la base dentro
// «<base>A_PRT»: sulla radice la legge solo la forma con il token, sul nodo nessuna. Senza la classe la
// forma STEP darebbe una lettura in più, annidata nella prima (R25 f).
func TestConfiniClasseParolaASCII(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famMarcatore(), famMarcatoreX()))
	letture, _ := riconosci(t, m, "radice_step.id", "9123456A_PRT")
	l := unaLettura(t, letture, "acme-marcatore", "step-prt")
	if l.Token == nil || l.Token.Valore != "PRT" {
		t.Errorf("token %+v, atteso «PRT» conservato (D10)", l.Token)
	}
	if letture, _ := riconosci(t, m, "nodo_step.id", "9123456A_PRT"); len(letture) != 0 {
		t.Errorf("nodo «9123456A_PRT»: nessuna lettura attesa, trovate %s", riassunto(letture))
	}

	// Senza la classe: la forma STEP con il confine predefinito, la forma con il token riservata perché il suo
	// esempio non collida.
	f := famMarcatore()
	for i := range f.Forme {
		switch f.Forme[i].ID {
		case "step":
			f.Forme[i].ConfineDopo = nil
		case "step-prt":
			f.Forme[i].Stato = grammatica.StatoRiservata
		}
	}
	// Il negativo del nodo prende la lettura in più: la grammatica non si attiva.
	compilaErrore(t, grammaticaACME(f), CodiceEsempioInEccesso)
	var esempi []grammatica.EsempioCodice
	for _, e := range f.Esempi {
		if e.ID != "n-nodo-prt" {
			esempi = append(esempi, e)
		}
	}
	f.Esempi = esempi
	senza, _ := compilaBene(t, grammaticaACME(f))
	letture, _ = riconosci(t, senza, "nodo_step.id", "9123456A_PRT")
	l = unaLettura(t, letture, "acme-marcatore", "step")
	if l.Originale != "9123456A" {
		t.Errorf("senza la classe la forma STEP legge %q, attesa la lettura annidata «9123456A»", l.Originale)
	}
}

// TestConfiniClasseAlnumASCIIOSpazio: con il confine destro alnum_ascii_o_spazio la forma corta non legge il
// codice dentro il nome con il token, che legge solo la forma lunga; lo stesso per la base con il «+»
// seguita da due spazi e dal token. Senza la classe la forma corta collide con quella lunga sull'esempio.
func TestConfiniClasseAlnumASCIIOSpazio(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(tutteLeFamiglie()...))
	letture, _ := riconosci(t, m, "nome_file", "ACME-030P7120100 00 IN_WORK.stp")
	l := unaLettura(t, letture, "acme-prefisso", "nome-step")
	if l.Originale != "ACME-030P7120100 00 IN_WORK" {
		t.Errorf("lettura %q", l.Originale)
	}
	letture, _ = riconosci(t, m, "nome_file", "T+300.012345.010  00.zip")
	l = unaLettura(t, letture, "acme-campo-separato", "nome-token")
	if l.Token == nil || l.Token.Valore != "00" {
		t.Errorf("token %+v, atteso «00» conservato (Q1)", l.Token)
	}
	// Uno spazio dopo la base che non comincia la forma lunga: la forma corta non legge nemmeno lì.
	if letture, _ := riconosci(t, m, "nome_file", "ACME-030P7120100 copia.pdf"); len(letture) != 0 {
		t.Errorf("spazio dopo la base: la forma corta non legge, trovate %s", riassunto(letture))
	}

	f := famPrefisso()
	for i := range f.Forme {
		if f.Forme[i].ID == "nome-pdf" {
			f.Forme[i].ConfineDopo = nil
		}
	}
	compilaErrore(t, grammaticaACME(f), CodiceFormaCollisioneEsempi)
}

// TestConfiniClasseParolaASCIIOPuntoCifra: la forma parziale non legge dentro la forma completa, perché a
// destra non ammette «.» seguito da una cifra (A-C07); un punto a fine frase resta un confine. Senza la
// classe la parziale collide con la completa sull'esempio. A sinistra la classe non è ammessa.
func TestConfiniClasseParolaASCIIOPuntoCifra(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPunti()))
	letture, _ := riconosci(t, m, "corpo", "9.123.4567.3")
	unaLettura(t, letture, "acme-punti", "completa")
	for _, testo := range []string{"9.123.4567.", "Codice 9.123.4567. Grazie", "9.123.4567, poi"} {
		letture, _ := riconosci(t, m, "corpo", testo)
		unaLettura(t, letture, "acme-punti", "parziale")
	}

	f := famPunti()
	for i := range f.Forme {
		if f.Forme[i].ID == "parziale" {
			f.Forme[i].ConfineDopo = classe(grammatica.ConfineParolaASCII)
		}
	}
	f.Esempi = append(f.Esempi, positivo("e-solo-completa", "corpo", "9.123.4567.5",
		grammatica.LetturaAttesa{Forma: "completa", Base: "9.123.4567.5"}))
	compilaErrore(t, grammaticaACME(f), CodiceFormaCollisioneEsempi)

	g := famPunti()
	for i := range g.Forme {
		if g.Forme[i].ID == "parziale" {
			g.Forme[i].ConfinePrima = classe(grammatica.ConfineParolaASCIIOPuntoCifra)
		}
	}
	compilaErrore(t, grammaticaACME(g), grammatica.CodiceConfineNonAmmesso)
}

// TestConfiniLetturePerPianoAInizidiversi: le letture dello stesso piano che cominciano in punti diversi
// restano tutte (P1 §7.3), anche molte nello stesso testo.
func TestConfiniLetturePerPianoAInizidiversi(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPrefisso()))
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString("P712010")
		b.WriteByte(byte('0' + i%10))
		b.WriteString(", ")
	}
	letture, _ := riconosci(t, m, "corpo", b.String())
	if len(letture) != 50 {
		t.Fatalf("attese 50 letture, trovate %d", len(letture))
	}
}

// TestConfiniLettureDelloStessoPianoAnnidate: con una decorazione facoltativa davanti alla base e il confine
// sinistro alnum_ascii, lo stesso piano legge il nome dall'inizio e di nuovo dalla base: due inizi diversi,
// due letture, tutte e due conservate (P1 §7.3). Nessuna è una collisione, che vale fra forme diverse della
// stessa famiglia; è la grammatica a doverle evitare con confini e parti dichiarati (R25 f).
func TestConfiniLettureDelloStessoPianoAnnidate(t *testing.T) {
	f := famNuova()
	f.Forme[0].Parti[0].Min = 0
	f.Esempi = []grammatica.EsempioCodice{
		positivo("e-nome", "nome_file", "LOTTO7_AB12345K-r03.step",
			grammatica.LetturaAttesa{Forma: "nome", Base: "AB12345", Decorazioni: []string{"LOTTO7_"}},
			grammatica.LetturaAttesa{Forma: "nome", Base: "AB12345"}),
		positivo("e-corpo", "corpo", "AB12345K-TMP", grammatica.LetturaAttesa{Forma: "corpo", Base: "AB12345"}),
	}
	m, d := compilaBene(t, grammaticaACME(f))
	if len(conCodice(d, CodiceFormaCollisioneEsempi)) != 0 {
		t.Errorf("letture dello stesso piano trattate come collisione:\n%s", elenco(d))
	}
	letture, _ := riconosci(t, m, "nome_file", "LOTTO7_AB12345K-r03.step")
	if len(letture) != 2 || letture[0].Intervallo.Inizio != 0 || letture[1].Intervallo.Inizio != 7 {
		t.Fatalf("attese due letture, da 0 e da 7: %s", riassunto(letture))
	}
	if len(letture[0].Decorazioni) != 1 || len(letture[1].Decorazioni) != 0 {
		t.Errorf("decorazioni %+v e %+v", letture[0].Decorazioni, letture[1].Decorazioni)
	}
}
