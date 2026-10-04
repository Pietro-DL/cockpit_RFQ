package motorea

import "testing"

// L1 — A-C07 pubblica (piano A, par.4.7.2; P1 §12, §10.2; D5): una forma parziale dà vincoli, non un
// articolo. «9.123.4567» si legge come parziale, con T fra i mancanti e nessun segmento T; il confronto con
// due completamenti diversi dà compatibile_parziale tutte e due le volte (l'ambiguità resta), fra i due
// completi dà discordante; una base completa non dà anche la lettura parziale.
//
// I clienti di questi test sono inventati: il repository è pubblico, e le famiglie di codice dei clienti veri
// sono un dato dell'azienda. La grammatica è acme-punti di sintetiche_test.go.

// leggiBase: la base dell'unica lettura di quella forma sul corpo.
func leggiBase(t *testing.T, m *Motore, testo, nomeForma string) BaseLetta {
	t.Helper()
	letture, _ := riconosci(t, m, "corpo", testo)
	return unaLettura(t, letture, "acme-punti", nomeForma).Base
}

func TestAC07FormaParzialeSenzaCompletamento(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPunti()))
	letture, _ := riconosci(t, m, "corpo", "9.123.4567")
	l := unaLettura(t, letture, "acme-punti", "parziale")
	if l.Base.Completa || l.Stato != StatoParziale {
		t.Errorf("completa %v, stato %q: attesa una base parziale", l.Base.Completa, l.Stato)
	}
	if len(l.Base.Mancanti) != 1 || l.Base.Mancanti[0] != "T" {
		t.Errorf("mancanti %v, atteso [T]", l.Base.Mancanti)
	}
	for _, s := range l.Base.Segmenti {
		if s.Nome == "T" {
			t.Errorf("segmento T letto in una forma parziale: %+v (nessun completamento inventato)", s)
		}
	}
	if len(l.Base.Segmenti) != 3 {
		t.Errorf("segmenti %+v, attesi prefisso, gruppo e numero", l.Base.Segmenti)
	}
	if l.Base.Normalizzata != "9.123.4567" || l.CodiceRichiesto != "9.123.4567" {
		t.Errorf("base %q, codice richiesto %q: niente T inventata", l.Base.Normalizzata, l.CodiceRichiesto)
	}
}

func TestAC07LaFormaCompletaNonDaLaParziale(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPunti()))
	for _, testo := range []string{"9.123.4567.3", "9.123.4567.3/01", "9.123.4567.3/xx", "9.123.4567.3_30", "S9.123.4567.3"} {
		letture, _ := riconosci(t, m, "corpo", testo)
		if len(letture) != 1 || letture[0].Forma == "parziale" {
			t.Errorf("%q: attesa una sola lettura, non parziale; trovate %s", testo, riassunto(letture))
		}
	}
}

func TestAC07ConfrontaBasi(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPunti()))
	parziale := leggiBase(t, m, "9.123.4567", "parziale")
	tre := leggiBase(t, m, "9.123.4567.3", "completa")
	cinque := leggiBase(t, m, "9.123.4567.5", "completa")
	altra := leggiBase(t, m, "9.123.4568", "parziale")

	casi := []struct {
		nome   string
		a, b   BaseLetta
		atteso Compatibilita
	}{
		{"parziale e completa .3", parziale, tre, CompatibilitaParziale},
		{"parziale e completa .5", parziale, cinque, CompatibilitaParziale},
		{"completa .3 e parziale", tre, parziale, CompatibilitaParziale},
		{"le due complete", tre, cinque, CompatibilitaDiscordante},
		{"la stessa completa", tre, leggiBase(t, m, "9.123.4567.3/01", "completa"), CompatibilitaUguale},
		{"due parziali uguali", parziale, leggiBase(t, m, "9.123.4567/01", "parziale"), CompatibilitaParziale},
		{"parziale con un segmento comune diverso", altra, tre, CompatibilitaDiscordante},
		{"due parziali diverse", altra, parziale, CompatibilitaDiscordante},
	}
	for _, c := range casi {
		if got := ConfrontaBasi(c.a, c.b); got != c.atteso {
			t.Errorf("%s: %q, atteso %q", c.nome, got, c.atteso)
		}
	}
}

// TestAC07ConfrontaBasiFraSpaziDiversi: basi di famiglie con una struttura diversa non si confrontano
// (non_determinabile), e una base vuota nemmeno. Il namespace della lettura lo controlla chi confronta due
// letture: ConfrontaBasi riceve solo le basi (par.3.3.4).
func TestAC07ConfrontaBasiFraSpaziDiversi(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPunti(), famEtichetta()))
	punti := leggiBase(t, m, "9.123.4567.3", "completa")
	letture, _ := riconosci(t, m, "corpo", "12.3456.7890")
	etichetta := unaLettura(t, letture, "acme-etichetta", "corpo").Base
	if got := ConfrontaBasi(punti, etichetta); got != CompatibilitaNonDeterminabile {
		t.Errorf("basi di struttura diversa: %q, atteso non_determinabile", got)
	}
	if got := ConfrontaBasi(BaseLetta{}, punti); got != CompatibilitaNonDeterminabile {
		t.Errorf("base vuota: %q, atteso non_determinabile", got)
	}
}

// TestAC07LaParzialeNonEPerdutaDentroUnTesto: la parziale accanto a una completa nello stesso testo resta
// una lettura a sé, e la completa non si spezza.
func TestAC07LaParzialeNonEPerdutaDentroUnTesto(t *testing.T) {
	m, _ := compilaBene(t, grammaticaACME(famPunti()))
	letture, _ := riconosci(t, m, "corpo", "9.123.4567 oppure 9.123.4567.3")
	if len(letture) != 2 || letture[0].Forma != "parziale" || letture[1].Forma != "completa" {
		t.Fatalf("attese la parziale e la completa, trovate %s", riassunto(letture))
	}
	if ConfrontaBasi(letture[0].Base, letture[1].Base) != CompatibilitaParziale {
		t.Errorf("le due basi devono restare compatibili, con l'ambiguità")
	}
}
