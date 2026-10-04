package motorea

import (
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L1 — le revisioni (A1a-REV; piano A, par.4.4.5 «Come si costruisce una lettura», 4.7.2; D4, D5, D9; P1
// §5.2): forte e debole su «/00», «/01», «/10», «/20», «/30», con gli zeri; «/xx» è un token sospeso, senza
// valore, con revisione.da_verificare e la lettura da verificare; «_30» è solo un token, mai uguale né
// equivalente a «/30»; la revisione in campo separato a due cifre, senza forte e debole, si verifica con D-07;
// ConfrontaRevisioni confronta solo stringhe ed equivalenze dichiarate, senza ordinamento.
//
// I clienti di questi test sono inventati: il repository è pubblico, e le famiglie di codice dei clienti veri
// sono un dato dell'azienda. Le grammatiche sono acme-punti, acme-marcatore e acme-campo-separato di
// sintetiche_test.go.

// revisioneDi: la revisione dell'unica lettura di quella forma.
func revisioneDi(t *testing.T, m *Motore, s, testo, famiglia, nomeForma string) (LetturaForma, *RevisioneLetta, []evidenze.Diagnostica) {
	t.Helper()
	letture, diag := riconosci(t, m, s, testo)
	l := unaLettura(t, letture, famiglia, nomeForma)
	if l.Revisione == nil {
		t.Fatalf("%q: nessuna revisione letta", testo)
	}
	return l, l.Revisione, diag
}

func TestRevisioneForteEDeboleConGliZeri(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPunti()))
	for _, r := range []string{"00", "01", "10", "20", "30"} {
		testo := "9.123.4567.3/" + r
		l, rev, diag := revisioneDi(t, m, "corpo", testo, "acme-punti", "completa")
		if rev.Stato != StatoRevisioneLetta || rev.Normalizzata != r {
			t.Errorf("%q: stato %q, normalizzata %q: attesa letta, %q con gli zeri (D4)", testo, rev.Stato, rev.Normalizzata, r)
		}
		if rev.Token != nil || rev.RichiedeVerifica {
			t.Errorf("%q: token %+v, da verificare %v", testo, rev.Token, rev.RichiedeVerifica)
		}
		if len(rev.Segmenti) != 2 || rev.Segmenti[0].Nome != "forte" || rev.Segmenti[0].Normalizzato != r[:1] ||
			rev.Segmenti[1].Nome != "debole" || rev.Segmenti[1].Normalizzato != r[1:] {
			t.Errorf("%q: segmenti %+v, attesi forte %q e debole %q", testo, rev.Segmenti, r[:1], r[1:])
		}
		if rev.Regola != "rev-barra" || rev.Sorgente != grammatica.SorgenteInline {
			t.Errorf("%q: regola %q, sorgente %q", testo, rev.Regola, rev.Sorgente)
		}
		if rev.Originale != testo[rev.Intervallo.Inizio:rev.Intervallo.Fine] || !strings.HasSuffix(rev.Originale, r) {
			t.Errorf("%q: originale %q in %v", testo, rev.Originale, rev.Intervallo)
		}
		if l.Base.Normalizzata != "9.123.4567.3" || l.CodiceRichiesto != "9.123.4567.3" {
			t.Errorf("%q: base %q, codice richiesto %q: la revisione resta fuori", testo, l.Base.Normalizzata, l.CodiceRichiesto)
		}
		if l.Stato != StatoCompleta {
			t.Errorf("%q: stato %q", testo, l.Stato)
		}
		if len(conCodice(diag, CodiceRevisioneDaVerificare)) != 0 {
			t.Errorf("%q: revisione letta con revisione.da_verificare", testo)
		}
	}
}

func TestRevisioneTokenSospesoXX(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPunti()))
	l, rev, diag := revisioneDi(t, m, "corpo", "9.123.4567.3/xx", "acme-punti", "completa")
	if rev.Stato != StatoRevisioneNonInterpretabile || rev.Normalizzata != "" || len(rev.Segmenti) != 0 {
		t.Errorf("stato %q, normalizzata %q, segmenti %+v: atteso un token senza valore", rev.Stato, rev.Normalizzata, rev.Segmenti)
	}
	if rev.Token == nil || rev.Token.Valore != "xx" || !rev.RichiedeVerifica {
		t.Errorf("token %+v, da verificare %v", rev.Token, rev.RichiedeVerifica)
	}
	if l.Stato != StatoDaVerificare {
		t.Errorf("stato della lettura %q, atteso da_verificare (base candidata)", l.Stato)
	}
	if l.Base.Normalizzata != "9.123.4567.3" {
		t.Errorf("base %q", l.Base.Normalizzata)
	}
	dv := conCodice(diag, CodiceRevisioneDaVerificare)
	if len(dv) != 1 || dv[0].Gravita != evidenze.GravitaAvviso || dv[0].Natura != evidenze.NaturaDati {
		t.Fatalf("attesa una revisione.da_verificare (avviso, dati), trovate:\n%s", elenco(diag))
	}
	if strings.Contains(dv[0].Messaggio, "9.123.4567") {
		t.Errorf("il messaggio riporta il testo letto: %q", dv[0].Messaggio)
	}
	// Una per lettura: due codici con il token, due diagnostiche.
	_, diag = riconosci(t, m, "corpo", "9.123.4567.3/xx e 9.123.4567.5/xx")
	if n := len(conCodice(diag, CodiceRevisioneDaVerificare)); n != 2 {
		t.Errorf("due letture con il token: %d revisione.da_verificare, attese 2", n)
	}
	// «XX» maiuscolo non è il token dichiarato (maiuscole esatte): nessuna revisione letta.
	letture, _ := riconosci(t, m, "corpo", "9.123.4567.3/XX")
	for _, x := range letture {
		if x.Revisione != nil {
			t.Errorf("«/XX»: revisione letta %+v, il token dichiarato è «xx»", x.Revisione)
		}
	}
}

func TestRevisioneTokenSottolineatoSenzaEquivalenza(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPunti()))
	l, rev, diag := revisioneDi(t, m, "oggetto", "9.123.4567.3_30", "acme-punti", "completa-sottolineato")
	if rev.Stato != StatoRevisioneNonInterpretabile || rev.Token == nil || rev.Token.Valore != "30" || rev.Normalizzata != "" {
		t.Errorf("revisione %+v: atteso il token «30» senza valore (D5, D-06)", rev)
	}
	if l.Stato != StatoDaVerificare || len(conCodice(diag, CodiceRevisioneDaVerificare)) != 1 {
		t.Errorf("stato %q, diagnostiche:\n%s", l.Stato, elenco(diag))
	}
	_, barra, _ := revisioneDi(t, m, "oggetto", "9.123.4567.3/30", "acme-punti", "completa")
	for _, c := range []Compatibilita{ConfrontaRevisioni(*rev, *barra), ConfrontaRevisioni(*barra, *rev)} {
		if c == CompatibilitaUguale || c == CompatibilitaEquivalente {
			t.Errorf("«_30» e «/30»: %q, nessuna equivalenza dichiarata", c)
		}
		if c != CompatibilitaNonDeterminabile {
			t.Errorf("«_30» e «/30»: %q, atteso non_determinabile (una delle due non è letta)", c)
		}
	}
}

func TestRevisioneInCampoSeparatoD07(t *testing.T) {
	// L'esempio della grammatica con la revisione attesa e la forma vuota passa (D-07).
	compilaBene(t, grammaticaACME(famCampoSeparato()))

	// Un valore diverso dall'atteso: esempio_diverso.
	f := famCampoSeparato()
	f.Esempi = append(f.Esempi, positivo("e-rev-diversa", "cartiglio.revisione", "01", grammatica.LetturaAttesa{Revisione: "02"}))
	compilaErrore(t, grammaticaACME(f), CodiceEsempioDiverso)

	// Una cifra sola: la regola vuole due cifre, la revisione attesa manca.
	g := famCampoSeparato()
	g.Esempi = append(g.Esempi, positivo("e-rev-corta", "cartiglio.revisione", "1", grammatica.LetturaAttesa{Revisione: "1"}))
	compilaErrore(t, grammaticaACME(g), CodiceEsempioMancante)

	// Il campo intero, non un pezzo: «01 bis» non è una revisione a due cifre.
	h := famCampoSeparato()
	h.Esempi = append(h.Esempi, positivo("e-rev-con-testo", "cartiglio.revisione", "01 bis", grammatica.LetturaAttesa{Revisione: "01"}))
	compilaErrore(t, grammaticaACME(h), CodiceEsempioMancante)

	// Un negativo sul campo che la regola legge: la revisione è in eccesso.
	k := famCampoSeparato()
	k.Esempi = append(k.Esempi, negativo("n-rev", "cartiglio.revisione", "07"))
	compilaErrore(t, grammaticaACME(k), CodiceEsempioInEccesso)
}

// TestRevisioneInCampoSeparatoNonEUnaForma: la regola in campo separato non dà letture di forma: su quel
// selettore Riconosci non legge niente, il valore lo darà l'attributo in A1b (D-07).
func TestRevisioneInCampoSeparatoNonEUnaForma(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famCampoSeparato()))
	if letture, _ := riconosci(t, m, "cartiglio.revisione", "01"); len(letture) != 0 {
		t.Errorf("letture di forma sul campo della revisione: %s", riassunto(letture))
	}
}

func TestConfrontaRevisioni(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPunti()))
	rev := func(testo string) RevisioneLetta {
		_, r, _ := revisioneDi(t, m, "corpo", "9.123.4567.3"+testo, "acme-punti", "completa")
		return *r
	}
	casi := []struct {
		nome   string
		a, b   RevisioneLetta
		atteso Compatibilita
	}{
		{"stessa stringa", rev("/01"), rev("/01"), CompatibilitaUguale},
		{"zeri diversi", rev("/00"), rev("/01"), CompatibilitaDiscordante},
		{"cifre invertite", rev("/01"), rev("/10"), CompatibilitaDiscordante},
		{"token e letta", rev("/xx"), rev("/00"), CompatibilitaNonDeterminabile},
		{"due token", rev("/xx"), rev("/xx"), CompatibilitaNonDeterminabile},
		{"assente e letta", RevisioneLetta{}, rev("/00"), CompatibilitaNonDeterminabile},
	}
	for _, c := range casi {
		if got := ConfrontaRevisioni(c.a, c.b); got != c.atteso {
			t.Errorf("%s: %q, atteso %q", c.nome, got, c.atteso)
		}
	}
	// Nessun ordinamento: il confronto non dipende dall'ordine dei due argomenti.
	for _, c := range casi {
		if ConfrontaRevisioni(c.a, c.b) != ConfrontaRevisioni(c.b, c.a) {
			t.Errorf("%s: il confronto cambia con l'ordine degli argomenti", c.nome)
		}
	}
}

// TestConfrontaRevisioniEquivalenzaDichiarata: solo una coppia che la regola dichiara è equivalente (P1
// §5.2: solo equivalenze dichiarate); due valori fuori dalla coppia restano discordanti.
func TestConfrontaRevisioniEquivalenzaDichiarata(t *testing.T) {
	f := famMarcatore()
	f.Revisioni[0].Segmenti[0].Pattern = "[0-9]{1,2}"
	f.Revisioni[0].Equivalenze = [][2]string{{"2", "02"}}
	m, _ := compilaBene(t, grammaticaACME(f))
	_, due, _ := revisioneDi(t, m, "cartiglio.codice", "9123456A2", "acme-marcatore", "cartiglio")
	_, zerodue, _ := revisioneDi(t, m, "cartiglio.codice", "9123456A02", "acme-marcatore", "cartiglio")
	_, tre, _ := revisioneDi(t, m, "cartiglio.codice", "9123456A3", "acme-marcatore", "cartiglio")
	if due.Normalizzata != "2" || zerodue.Normalizzata != "02" {
		t.Fatalf("revisioni %q e %q", due.Normalizzata, zerodue.Normalizzata)
	}
	if got := ConfrontaRevisioni(*due, *zerodue); got != CompatibilitaEquivalente {
		t.Errorf("coppia dichiarata: %q, atteso equivalente", got)
	}
	if got := ConfrontaRevisioni(*zerodue, *due); got != CompatibilitaEquivalente {
		t.Errorf("coppia dichiarata, rovesciata: %q, atteso equivalente", got)
	}
	if got := ConfrontaRevisioni(*due, *tre); got != CompatibilitaDiscordante {
		t.Errorf("fuori dalla coppia: %q, atteso discordante", got)
	}

	// Senza la dichiarazione la stessa coppia è discordante.
	g := famMarcatore()
	g.Revisioni[0].Segmenti[0].Pattern = "[0-9]{1,2}"
	n, _ := compilaBene(t, grammaticaACME(g))
	_, a, _ := revisioneDi(t, n, "cartiglio.codice", "9123456A2", "acme-marcatore", "cartiglio")
	_, b, _ := revisioneDi(t, n, "cartiglio.codice", "9123456A02", "acme-marcatore", "cartiglio")
	if got := ConfrontaRevisioni(*a, *b); got != CompatibilitaDiscordante {
		t.Errorf("senza equivalenza dichiarata: %q, atteso discordante", got)
	}
}
