package motorea

import (
	"regexp"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L1 — i confini del codice (A1a-CNF pubblica; piano A, par.4.4.5 e 4.7.2; v3 §1.5, §7.4; T2-T5, T13-T15;
// D-05, R25 f): codici accanto ad altri codici e a separatori, codice dopo le cifre di un prefisso, due basi
// in un nome di pacchetto, offset in byte con accenti, emoji e spazio non separabile accanto, nessuna lettura
// dentro una parola, e le sei classi di confine (la quinta, alnum_ascii_o_trattino, dal 17/10: LS-03 A; la
// sesta, parola_ascii_spazio_o_trattino, dal 21/10: D-G48-01 B), ognuna con il caso che la giustifica e con la
// lettura in più che darebbe la grammatica senza la classe.
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

// famTrattino: acme-trattino (LS-03 A, 17/10). Base di otto cifre; la forma «documento» con il suffisso
// «---7-ARC_<base>--» (token «7» conservato, Q1; la base ripetuta, D2) e la forma «breve» «<base>--», con il
// confine sinistro parola_ascii e il destro alnum_ascii_o_trattino. Senza i due confini la forma breve
// leggerebbe la base all'inizio del nome della forma documento e dentro «ARC_<base>--» (R25 f). Codici, sigle e
// nomi sono inventati: riproducono il meccanismo (un «-» dopo «<base>--», la base ripetuta dopo «_»), non la
// convenzione di un cliente.
func famTrattino() grammatica.FamigliaCodice {
	breve := forma("breve", sel("nome_file"), pBase(), pSep("--"))
	breve.ConfinePrima = classe(grammatica.ConfineParolaASCII)
	breve.ConfineDopo = classe(grammatica.ConfineAlnumASCIIOTrattino)
	return grammatica.FamigliaCodice{
		ID: "acme-trattino", Namespace: "acme-trattino",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(segPattern("codice", "[0-9]{8}", "")),
		Forme: []grammatica.FormaCodice{
			forma("documento", sel("nome_file"), pBase(), pRif(grammatica.TipoParteDecorazione, "doc", 1)),
			breve,
		},
		Decorazioni: []grammatica.Decorazione{{ID: "doc", Tipo: grammatica.TipoDecorazioneSuffissoDocumento, Selettori: sel("nome_file"),
			Parti: []grammatica.Parte{pSep("---"), pToken("Q1", "7"), pSep("-ARC_"), pRipetizione(), pSep("--")}}},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-documento", "nome_file", "12345678---7-ARC_12345678--.pdf",
				grammatica.LetturaAttesa{Forma: "documento", Base: "12345678"}),
			positivo("e-breve", "nome_file", "12345678--.pdf", grammatica.LetturaAttesa{Forma: "breve", Base: "12345678"}),
			negativo("n-breve-tre-trattini", "nome_file", "12345678---.pdf"),
		},
	}
}

// TestConfiniClasseAlnumASCIIOTrattino: con il confine destro alnum_ascii_o_trattino (e parola_ascii a
// sinistra) la forma breve legge «<base>--» quando dopo non c'è una lettera, una cifra o un «-» ASCII, e non
// legge niente dentro il nome della forma documento, che legge solo la sua forma. «_», gli spazi, la
// punteggiatura, i trattini fuori dall'ASCII e la fine del testo sono confini. Senza una delle due classi la
// forma breve collide con la documento sull'esempio e la grammatica non si attiva (R25 f).
func TestConfiniClasseAlnumASCIIOTrattino(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famTrattino()))
	casi := []struct {
		testo, forma, letta string // forma vuota: nessuna lettura
	}{
		{"12345678--", "breve", "12345678--"}, // la fine del testo è un confine
		{"12345678--.pdf", "breve", "12345678--"},
		{"12345678-- (copia)", "breve", "12345678--"},
		{"12345678-- (copia).pdf", "breve", "12345678--"},
		{"12345678--(1).pdf", "breve", "12345678--"},
		{"(12345678--).pdf", "breve", "12345678--"},
		{"12345678--_rev.pdf", "breve", "12345678--"}, // «_» non è vietato da questa classe
		{"12345678--—x.pdf", "breve", "12345678--"},   // trattino lungo U+2014: fuori dall'ASCII
		{"12345678--‐x.pdf", "breve", "12345678--"},   // trattino U+2010: fuori dall'ASCII
		{"12345678--èx.pdf", "breve", "12345678--"},   // lettera fuori dall'ASCII (T3, T10)
		{"12345678-- x.pdf", "breve", "12345678--"},
		{"12345678---7-ARC_12345678--", "documento", "12345678---7-ARC_12345678--"},
		{"12345678---7-ARC_12345678--.pdf", "documento", "12345678---7-ARC_12345678--"},
		{"12345678---.pdf", "", ""}, // un terzo trattino: la forma breve non legge l'inizio di una più lunga
		{"12345678----.pdf", "", ""},
		{"12345678--A", "", ""}, // lettera ASCII dopo
		{"12345678--a.pdf", "", ""},
		{"12345678--7.pdf", "", ""}, // cifra ASCII dopo
		{"X12345678--", "", ""},     // lettera ASCII prima (parola_ascii)
		{"_12345678--.pdf", "", ""}, // «_» prima (parola_ascii)
		{"012345678--", "", ""},     // nove cifre: nessuna lettura dentro il numero
		{"123456789--", "", ""},
		{"12345678-.pdf", "", ""}, // un trattino solo: non è la forma
	}
	for _, c := range casi {
		letture, _ := riconosci(t, m, "nome_file", c.testo)
		if c.forma == "" {
			if len(letture) != 0 {
				t.Errorf("%q: nessuna lettura attesa, trovate %s", c.testo, riassunto(letture))
			}
			continue
		}
		if len(letture) != 1 || letture[0].Famiglia != "acme-trattino" || letture[0].Forma != c.forma ||
			letture[0].Originale != c.letta || letture[0].Base.Normalizzata != "12345678" {
			t.Errorf("%q: attesa una lettura acme-trattino/%s %q, trovate %s", c.testo, c.forma, c.letta, riassunto(letture))
		}
	}

	// Senza la classe a destra (o con parola_ascii) la forma breve legge «<base>--» all'inizio del nome della
	// forma documento; senza parola_ascii a sinistra legge «ARC_<base>--». Tutti e due i casi sono una
	// collisione sull'esempio della forma documento.
	for _, v := range []struct {
		nome   string
		cambia func(*grammatica.FormaCodice)
	}{
		{"senza la classe a destra", func(f *grammatica.FormaCodice) { f.ConfineDopo = nil }},
		{"parola_ascii a destra", func(f *grammatica.FormaCodice) { f.ConfineDopo = classe(grammatica.ConfineParolaASCII) }},
		{"senza parola_ascii a sinistra", func(f *grammatica.FormaCodice) { f.ConfinePrima = nil }},
	} {
		t.Run(v.nome, func(t *testing.T) {
			f := famTrattino()
			for i := range f.Forme {
				if f.Forme[i].ID == "breve" {
					v.cambia(&f.Forme[i])
				}
			}
			compilaErrore(t, grammaticaACME(f), CodiceFormaCollisioneEsempi)
		})
	}
}

// TestConfiniClasseAlnumASCIIOTrattinoASinistra: la classe vale anche come confine sinistro (non è fra le
// classi solo a destra): vieta lettere, cifre e «-» ASCII prima del codice e ammette «_», gli spazi, la
// punteggiatura e i trattini fuori dall'ASCII; l'inizio del testo è un confine.
func TestConfiniClasseAlnumASCIIOTrattinoASinistra(t *testing.T) {
	breve := forma("breve", sel("nome_file"), pBase(), pSep("--"))
	breve.ConfinePrima = classe(grammatica.ConfineAlnumASCIIOTrattino)
	breve.ConfineDopo = classe(grammatica.ConfineAlnumASCIIOTrattino)
	f := grammatica.FamigliaCodice{
		ID: "acme-trattino-sinistro", Namespace: "acme-trattino",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(segPattern("codice", "[0-9]{8}", "")),
		Forme: []grammatica.FormaCodice{breve},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-breve", "nome_file", "12345678--.pdf", grammatica.LetturaAttesa{Forma: "breve", Base: "12345678"}),
			negativo("n-trattino-prima", "nome_file", "-12345678--.pdf"),
		},
	}
	m, _ := compilaBene(t, grammaticaACME(f))
	for _, c := range []struct {
		testo  string
		inizio int // -1: nessuna lettura
	}{
		{"12345678--.pdf", 0},
		{"_12345678--.pdf", 1},
		{" 12345678--.pdf", 1},
		{"(12345678--).pdf", 1},
		{"x.12345678--.pdf", 2},
		{"—12345678--.pdf", len("—")},
		{"‐12345678--.pdf", len("‐")},
		{"-12345678--.pdf", -1},
		{"--12345678--.pdf", -1},
		{"A12345678--.pdf", -1},
		{"012345678--.pdf", -1},
	} {
		letture, _ := riconosci(t, m, "nome_file", c.testo)
		if c.inizio < 0 {
			if len(letture) != 0 {
				t.Errorf("%q: nessuna lettura attesa, trovate %s", c.testo, riassunto(letture))
			}
			continue
		}
		if len(letture) != 1 || letture[0].Forma != "breve" || letture[0].Intervallo.Inizio != c.inizio ||
			letture[0].Intervallo.Fine != c.inizio+len("12345678--") {
			t.Errorf("%q: attesa la lettura breve in [%d,%d), trovate %s", c.testo, c.inizio, c.inizio+len("12345678--"), riassunto(letture))
		}
	}
	// I due controlli in Go, ai bordi: l'inizio del testo è un confine, il «-» ASCII no, «—» sì.
	tr := grammatica.ConfineAlnumASCIIOTrattino
	if !confineSinistro(tr, "12345678", 0) || confineSinistro(tr, "-1", 1) || !confineSinistro(tr, "—1", len("—")) {
		t.Error("confine sinistro della classe alnum_ascii_o_trattino sbagliato")
	}
	if !confineDestro(tr, "1", 1) || confineDestro(tr, "1-", 1) || !confineDestro(tr, "1—", 1) {
		t.Error("confine destro della classe alnum_ascii_o_trattino sbagliato")
	}
}

// TestConfiniRegexEGoConcordano: per ogni classe la regex del confine destro (frammentoDestro, in fondo alla
// regex del piano) e il controllo in Go (confineDestro, che allunga le letture) dicono la stessa cosa sugli
// stessi caratteri, anche fuori dall'ASCII, alla fine del testo e su un byte UTF-8 non valido. Una classe
// fuori dal contratto non è nota al motore.
func TestConfiniRegexEGoConcordano(t *testing.T) {
	classi := []grammatica.ClasseConfine{grammatica.ConfineAlnumASCII, grammatica.ConfineParolaASCII,
		grammatica.ConfineAlnumASCIIOSpazio, grammatica.ConfineParolaASCIIOPuntoCifra, grammatica.ConfineAlnumASCIIOTrattino,
		grammatica.ConfineParolaASCIISpazioOTrattino}
	dopo := []string{"", "-", "--", "-x", "—", "–", "‐", "−", " ", " ", "\t", "\n", ".", ".5", ".x", "(", ")", "_",
		"$", "/", "A", "z", "0", "9", "è", "😀", "\xff"}
	for _, c := range classi {
		if !classeNota(c) {
			t.Errorf("classe %s non nota al motore", c)
		}
		fr, ok := frammentoDestro(c)
		if !ok {
			t.Fatalf("classe %s senza regex del confine destro", c)
		}
		re := regexp.MustCompile(`\A` + fr)
		for _, d := range dopo {
			if r, g := re.MatchString(d), confineDestro(c, d, 0); r != g {
				t.Errorf("classe %s, dopo il codice %q: la regex dice %v, il controllo in Go %v", c, d, r, g)
			}
		}
	}
	// La tabella della classe nuova, scritta una seconda volta per non fidarsi della sola concordanza.
	vietati := map[string]bool{"-": true, "--": true, "-x": true, "A": true, "z": true, "0": true, "9": true}
	for _, d := range dopo {
		if confineDestro(grammatica.ConfineAlnumASCIIOTrattino, d, 0) == vietati[d] {
			t.Errorf("alnum_ascii_o_trattino, dopo il codice %q: vietato %v", d, vietati[d])
		}
	}
	// La tabella della sesta classe (D-G48-01 B), ai due lati: a destra con la regex e con il Go, a sinistra con
	// il controllo in Go sulla runa prima del codice.
	vietatiSesta := map[string]bool{"-": true, "--": true, "-x": true, " ": true, "_": true, "A": true, "z": true,
		"0": true, "9": true}
	sesta := grammatica.ConfineParolaASCIISpazioOTrattino
	for _, d := range dopo {
		if confineDestro(sesta, d, 0) == vietatiSesta[d] {
			t.Errorf("parola_ascii_spazio_o_trattino, dopo il codice %q: vietato %v", d, vietatiSesta[d])
		}
		if d == "" {
			continue
		}
		// a sinistra conta l'ultimo carattere prima del codice: vietato se è una lettera, una cifra, «_», uno
		// spazio o un «-» ASCII
		u := d[len(d)-1]
		vietato := u < 0x80 && (('0' <= u && u <= '9') || ('A' <= u && u <= 'Z') || ('a' <= u && u <= 'z') ||
			strings.IndexByte("_ -", u) >= 0)
		if confineSinistro(sesta, d+"1", len(d)) == vietato {
			t.Errorf("parola_ascii_spazio_o_trattino, prima del codice %q: vietato %v", d, vietato)
		}
	}
	for _, nome := range []grammatica.ClasseConfine{"alnum_ascii_o_trattini", "parola_ascii_spazio_o_trattini", "parola_ascii_o_spazio_o_trattino"} {
		if _, ok := frammentoDestro(nome); ok || classeNota(nome) {
			t.Errorf("la classe %s, fuori dal contratto, è nota al motore", nome)
		}
	}
}

// famNomeNudo: acme-nudo (D-G48-01 B, 21/10). Base di otto cifre; la forma «nome» legge il nome del file che è
// solo il codice seguito subito dall'estensione («<base>.pdf», «<base>.stp»), con il confine sinistro
// parola_ascii_spazio_o_trattino. Senza la classe la forma leggerebbe il codice anche dopo un prefisso separato
// da «_», da «-» o da uno spazio. Codici, prefissi ed estensioni sono inventati: riproducono il meccanismo (un
// nome che è solo il codice), non la convenzione di un cliente.
func famNomeNudo() grammatica.FamigliaCodice {
	nome := forma("nome", sel("nome_file"), pBase(), pSep(".pdf", ".PDF", ".stp"))
	nome.ConfinePrima = classe(grammatica.ConfineParolaASCIISpazioOTrattino)
	return grammatica.FamigliaCodice{
		ID: "acme-nudo", Namespace: "acme-nudo",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(segPattern("codice", "[0-9]{8}", "")),
		Forme: []grammatica.FormaCodice{nome},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-nome", "nome_file", "12345678.pdf", grammatica.LetturaAttesa{Forma: "nome", Base: "12345678"}),
			negativo("n-sottolineato", "nome_file", "Offerta_12345678.pdf"),
			negativo("n-trattino", "nome_file", "DDT-12345678.pdf"),
			negativo("n-anno", "nome_file", "2026-12345678.pdf"),
			negativo("n-spazio", "nome_file", "report 12345678.pdf"),
		},
	}
}

// TestConfiniClasseParolaASCIISpazioOTrattino: con il confine sinistro parola_ascii_spazio_o_trattino la forma
// legge «<base>.pdf» all'inizio del testo e dopo i caratteri che la classe non vieta, e non legge niente dopo
// una lettera, una cifra, «_», uno spazio o «-» ASCII. Il «.», «#», le parentesi, «/» e i caratteri fuori
// dall'ASCII (anche i trattini lunghi e lo spazio non separabile) restano confini, come dice il contratto. Con
// una delle classi di prima a sinistra un esempio negativo si legge e la grammatica non si attiva.
func TestConfiniClasseParolaASCIISpazioOTrattino(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famNomeNudo()))
	for _, c := range []struct {
		testo  string
		inizio int // -1: nessuna lettura
	}{
		{"12345678.pdf", 0},
		{"12345678.PDF", 0},
		{"12345678.stp", 0},
		{"Offerta_12345678.pdf", -1},
		{"DDT-12345678.pdf", -1},
		{"2026-12345678.pdf", -1},
		{"report 12345678.pdf", -1},
		{"_12345678.pdf", -1},
		{"-12345678.pdf", -1},
		{" 12345678.pdf", -1},
		{"x12345678.pdf", -1},
		{"012345678.pdf", -1}, // nove cifre: nessuna lettura dentro il numero
		{"fattura.12345678.pdf", len("fattura.")},
		{"#12345678.pdf", 1},
		{"(12345678.pdf", 1},
		{"cartella/12345678.pdf", len("cartella/")},
		{"—12345678.pdf", len("—")},
		{"‐12345678.pdf", len("‐")},
		{" 12345678.pdf", len(" ")}, // spazio non separabile: fuori dall'ASCII
		{"è12345678.pdf", len("è")},
		{"12345678_A.pdf", -1}, // a destra il separatore dell'estensione viene subito dopo il codice
		{"12345678.pdfx", -1},  // confine destro della forma: alnum_ascii
	} {
		letture, _ := riconosci(t, m, "nome_file", c.testo)
		if c.inizio < 0 {
			if len(letture) != 0 {
				t.Errorf("%q: nessuna lettura attesa, trovate %s", c.testo, riassunto(letture))
			}
			continue
		}
		if len(letture) != 1 || letture[0].Famiglia != "acme-nudo" || letture[0].Forma != "nome" ||
			letture[0].Intervallo.Inizio != c.inizio || letture[0].Base.Normalizzata != "12345678" {
			t.Errorf("%q: attesa la lettura acme-nudo/nome da %d, trovate %s", c.testo, c.inizio, riassunto(letture))
		}
	}

	// Con le classi di prima a sinistra almeno un esempio negativo si legge: la grammatica non si attiva.
	for _, v := range []struct {
		nome string
		c    *grammatica.ClasseConfine
	}{
		{"senza classe a sinistra", nil},
		{"alnum_ascii", classe(grammatica.ConfineAlnumASCII)},
		{"parola_ascii", classe(grammatica.ConfineParolaASCII)},
		{"alnum_ascii_o_spazio", classe(grammatica.ConfineAlnumASCIIOSpazio)},
		{"alnum_ascii_o_trattino", classe(grammatica.ConfineAlnumASCIIOTrattino)},
	} {
		t.Run(v.nome, func(t *testing.T) {
			f := famNomeNudo()
			f.Forme[0].ConfinePrima = v.c
			compilaErrore(t, grammaticaACME(f), CodiceEsempioInEccesso)
		})
	}
}

// TestConfiniClasseParolaASCIISpazioOTrattinoADestra: la classe vale anche come confine destro (non è fra le
// classi solo a destra, e nemmeno solo a sinistra): dopo il codice vieta lettere, cifre, «_», spazio e «-»
// ASCII, e ammette il «.», la punteggiatura, i caratteri fuori dall'ASCII e la fine del testo.
func TestConfiniClasseParolaASCIISpazioOTrattinoADestra(t *testing.T) {
	cod := forma("codice", sel("nome_file"), pBase())
	cod.ConfineDopo = classe(grammatica.ConfineParolaASCIISpazioOTrattino)
	f := grammatica.FamigliaCodice{
		ID: "acme-nudo-destro", Namespace: "acme-nudo",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(segPattern("codice", "[0-9]{8}", "")),
		Forme: []grammatica.FormaCodice{cod},
		Esempi: []grammatica.EsempioCodice{
			positivo("e-codice", "nome_file", "12345678.pdf", grammatica.LetturaAttesa{Forma: "codice", Base: "12345678"}),
			negativo("n-sottolineato-dopo", "nome_file", "12345678_A.pdf"),
		},
	}
	m, _ := compilaBene(t, grammaticaACME(f))
	for _, c := range []struct {
		testo string
		letta bool
	}{
		{"12345678", true}, {"12345678.pdf", true}, {"12345678(1).pdf", true}, {"12345678#2.pdf", true},
		{"12345678—x.pdf", true}, {"12345678 x.pdf", true}, {"12345678èx.pdf", true},
		{"12345678_A.pdf", false}, {"12345678-A.pdf", false}, {"12345678 A.pdf", false}, {"12345678A.pdf", false},
		{"123456789.pdf", false},
	} {
		letture, _ := riconosci(t, m, "nome_file", c.testo)
		if !c.letta {
			if len(letture) != 0 {
				t.Errorf("%q: nessuna lettura attesa, trovate %s", c.testo, riassunto(letture))
			}
			continue
		}
		if len(letture) != 1 || letture[0].Forma != "codice" || letture[0].Intervallo.Inizio != 0 ||
			letture[0].Intervallo.Fine != len("12345678") {
			t.Errorf("%q: attesa la lettura codice in [0,8), trovate %s", c.testo, riassunto(letture))
		}
	}
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
