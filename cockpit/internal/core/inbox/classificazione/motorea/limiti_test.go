package motorea

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L1 — i limiti nel motore (A1a-LIM, parte di motorea; piano A, par.4.4.5, 4.6.1, 4.7.2; R43 B; E-23; v3 §2):
// i limiti vengono da chi chiama, cioè dall'indice, mai da valori del motore. Un limite di validazione
// superato è un errore e la grammatica non si attiva; un limite di riconoscimento superato è un avviso
// limite.superato, con un risultato parziale, mai un successo vuoto e mai una lettura inventata. Un indice
// senza limiti o oltre un tetto non attiva nessuna grammatica; la versione dei limiti arriva nello snapshot;
// cambiare un limite cambia l'impronta dell'insieme, non l'hash della grammatica.
//
// I clienti di questi test sono inventati: il repository è pubblico, e le famiglie di codice dei clienti veri
// sono un dato dell'azienda. Le grammatiche sono quelle ACME di sintetiche_test.go.

// conRiconoscimento: i limiti ACME con altri limiti di riconoscimento.
func conRiconoscimento(maxByte, maxLetture int) grammatica.Limiti {
	lim := limitiACME()
	lim.Riconoscimento.MaxByteUnita = maxByte
	lim.Riconoscimento.MaxLetturePerUnita = maxLetture
	return lim
}

// motoreCon: acme-prefisso compilato con quei limiti.
func motoreCon(t *testing.T, lim grammatica.Limiti) *Motore {
	t.Helper()
	m, d, err := compila(t, grammaticaACME(famPrefisso()), lim)
	if err != nil {
		t.Fatalf("la grammatica non compila con i limiti dati: %v\n%s", err, elenco(d))
	}
	return m
}

// limiteSuperato: le diagnostiche limite.superato, che devono essere avvisi di natura limite.
func limiteSuperato(t *testing.T, d []evidenze.Diagnostica) []evidenze.Diagnostica {
	t.Helper()
	out := conCodice(d, grammatica.CodiceLimiteSuperato)
	for _, x := range out {
		if x.Gravita != evidenze.GravitaAvviso || x.Natura != evidenze.NaturaLimite {
			t.Errorf("limite.superato in Riconosci con gravità %s e natura %s: attesi avviso e limite", x.Gravita, x.Natura)
		}
	}
	return out
}

// sottoinsieme: ogni lettura del risultato parziale è una lettura del risultato senza limiti, identica.
func sottoinsieme(t *testing.T, parziale, intero []LetturaForma) {
	t.Helper()
	for _, p := range parziale {
		trovata := false
		for _, l := range intero {
			if reflect.DeepEqual(p, l) {
				trovata = true
				break
			}
		}
		if !trovata {
			t.Errorf("lettura inventata dal taglio: %s/%s %v %q", p.Famiglia, p.Forma, p.Intervallo, p.Originale)
		}
	}
}

func TestLimitiLetturePerUnita(t *testing.T) {
	testo := "P7120100 P7120101 P7120102 P7120103"
	intero, d := riconosci(t, motoreCon(t, limitiACME()), "corpo", testo)
	if len(intero) != 4 || len(d) != 0 {
		t.Fatalf("senza limiti stretti: %s\n%s", riassunto(intero), elenco(d))
	}
	m := motoreCon(t, conRiconoscimento(1048576, 2))
	letture, d := riconosci(t, m, "corpo", testo)
	if len(letture) != 2 {
		t.Fatalf("attese le prime 2 letture, trovate %s", riassunto(letture))
	}
	if letture[0].Base.Normalizzata != "7120100" || letture[1].Base.Normalizzata != "7120101" {
		t.Errorf("tenute %s: attese le prime nell'ordine del testo", riassunto(letture))
	}
	if len(limiteSuperato(t, d)) != 1 {
		t.Errorf("attesa una limite.superato, trovate:\n%s", elenco(d))
	}
	sottoinsieme(t, letture, intero)

	// Esattamente al limite: nessun avviso.
	letture, d = riconosci(t, m, "corpo", "P7120100 P7120101")
	if len(letture) != 2 || len(conCodice(d, grammatica.CodiceLimiteSuperato)) != 0 {
		t.Errorf("al limite: %s\n%s", riassunto(letture), elenco(d))
	}
}

// TestLimitiLetturePerUnitaSuPiuForme: il taglio vale per l'unità, non per ogni forma: con più famiglie sullo
// stesso testo restano al più max_letture_per_unita letture in tutto, le prime nell'ordine (inizio, fine,
// famiglia, forma).
func TestLimitiLetturePerUnitaSuPiuForme(t *testing.T) {
	g := grammaticaACME(famPrefisso(), famPunti(), famEtichetta(), famEtichettaPN())
	testo := "PN 7654321 P7120100 12.3456.7890 9.123.4567.3 P7120101"
	pieno, _, err := compila(t, g, limitiACME())
	if err != nil {
		t.Fatal(err)
	}
	intero, _ := riconosci(t, pieno, "corpo", testo)
	if len(intero) != 5 {
		t.Fatalf("senza limiti stretti: %s", riassunto(intero))
	}
	m, d, err := compila(t, g, conRiconoscimento(1048576, 3))
	if err != nil {
		t.Fatalf("%v\n%s", err, elenco(d))
	}
	letture, diag := riconosci(t, m, "corpo", testo)
	if !reflect.DeepEqual(letture, intero[:3]) {
		t.Errorf("tenute %s, attese le prime tre %s", riassunto(letture), riassunto(intero[:3]))
	}
	if len(limiteSuperato(t, diag)) == 0 {
		t.Errorf("manca limite.superato:\n%s", elenco(diag))
	}
}

func TestLimitiByteUnita(t *testing.T) {
	testo := "P7120100" + strings.Repeat(" ", 40) + "P7120101"
	intero, _ := riconosci(t, motoreCon(t, limitiACME()), "corpo", testo)
	m := motoreCon(t, conRiconoscimento(20, 1000))
	letture, d := riconosci(t, m, "corpo", testo)
	if len(letture) != 1 || letture[0].Base.Normalizzata != "7120100" {
		t.Errorf("attesa la sola lettura nei primi byte, trovate %s", riassunto(letture))
	}
	if len(limiteSuperato(t, d)) != 1 {
		t.Errorf("attesa una limite.superato, trovate:\n%s", elenco(d))
	}
	sottoinsieme(t, letture, intero)

	// Mai un successo vuoto: senza letture nei primi byte, l'avviso c'è comunque.
	letture, d = riconosci(t, m, "corpo", strings.Repeat(" ", 30)+"P7120100")
	if len(letture) != 0 {
		t.Errorf("letture oltre il limite dei byte: %s", riassunto(letture))
	}
	if len(limiteSuperato(t, d)) == 0 {
		t.Errorf("zero letture senza dire che il risultato è parziale")
	}

	// Un testo multibyte tagliato a metà di una runa non dà intervalli a metà runa (riconosci li controlla).
	riconosci(t, motoreCon(t, conRiconoscimento(21, 1000)), "corpo", "P7120100"+strings.Repeat("è", 20)+"P7120101")

	// Al limite esatto: nessun avviso.
	letture, d = riconosci(t, motoreCon(t, conRiconoscimento(8, 1000)), "corpo", "P7120100")
	if len(letture) != 1 || len(conCodice(d, grammatica.CodiceLimiteSuperato)) != 0 {
		t.Errorf("testo lungo quanto il limite: %s\n%s", riassunto(letture), elenco(d))
	}
}

// TestLimitiDiValidazione: un limite di validazione superato è un errore (limite.superato, gravità errore)
// e la grammatica non si attiva, anche se lo snapshot è nato con limiti più larghi: CompilaVerificato usa i
// limiti che riceve.
func TestLimitiDiValidazione(t *testing.T) {
	s, err := grammatica.NuovoSnapshot(fileJSON(t, grammaticaACME(famPrefisso())), limitiACME())
	if err != nil {
		t.Fatal(err)
	}
	stretti := limitiACME()
	stretti.Grammatica.MaxFormePerFamiglia = 2
	m, d, err := CompilaVerificato(s, stretti)
	if m != nil || err == nil {
		t.Fatalf("motore attivato oltre il limite delle forme")
	}
	sup := conCodice(d, grammatica.CodiceLimiteSuperato)
	if len(sup) == 0 || sup[0].Gravita != evidenze.GravitaErrore {
		t.Errorf("atteso limite.superato come errore:\n%s", elenco(d))
	}
}

// TestLimitiNessunValoreDelMotore: senza limiti il motore non ne inventa: la grammatica non si attiva.
func TestLimitiNessunValoreDelMotore(t *testing.T) {
	s, err := grammatica.NuovoSnapshot(fileJSON(t, grammaticaACME(famPrefisso())), limitiACME())
	if err != nil {
		t.Fatal(err)
	}
	if m, d, err := CompilaVerificato(s, grammatica.Limiti{}); m != nil || err == nil {
		t.Errorf("motore attivato senza limiti:\n%s", elenco(d))
	}
}

// indiceACME: un indice con la grammatica ACME, il suo sha256 e i limiti dati.
func indiceACME(t *testing.T, lim grammatica.Limiti) (grammatica.IndiceRegole, map[string][]byte) {
	t.Helper()
	raw := fileJSON(t, grammaticaACME(famPrefisso()))
	return grammatica.IndiceRegole{
		VersioneIndice: grammatica.VersioneIndice,
		Limiti:         lim,
		Grammatiche:    []grammatica.VoceIndice{{ClienteID: clienteACME, File: "acme.v1.json", Sha256: sha256Esadecimale(raw)}},
	}, map[string][]byte{"acme.v1.json": raw}
}

// TestLimitiDellIndice: un indice senza limiti o con un valore oltre il tetto non attiva nessuna grammatica,
// con l'errore e il codice giusto.
func TestLimitiDellIndice(t *testing.T) {
	oltre := limitiACME()
	oltre.Riconoscimento.MaxLetturePerUnita = grammatica.TettiLimiti().Riconoscimento.MaxLetturePerUnita + 1
	senzaVersione := limitiACME()
	senzaVersione.Versione = ""
	casi := []struct {
		nome   string
		lim    grammatica.Limiti
		codice string
	}{
		{"senza limiti", grammatica.Limiti{}, grammatica.CodiceCampoObbligatorio},
		{"senza versione", senzaVersione, grammatica.CodiceCampoObbligatorio},
		{"oltre il tetto", oltre, grammatica.CodiceLimiteOltreTetto},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			ind, contenuti := indiceACME(t, c.lim)
			ins, d, err := CompilaInsieme(ind, contenuti)
			var ec *evidenze.ErroreContratto
			if !errors.As(err, &ec) {
				t.Fatalf("errore %v, atteso un *evidenze.ErroreContratto", err)
			}
			if len(conCodice(d, c.codice)) == 0 {
				t.Errorf("manca %s:\n%s", c.codice, elenco(d))
			}
			if len(ins.Motori) != 0 {
				t.Errorf("%d grammatiche attive con un indice non valido", len(ins.Motori))
			}
			if m, _ := ins.MotoreDi(clienteACME, ragioneSocialeACME); m != nil {
				t.Errorf("MotoreDi dà un motore con un indice non valido")
			}
		})
	}
}

// TestLimitiVersioneNelloSnapshotEImpronta: la versione dei limiti dell'indice arriva nello snapshot del
// motore; cambiare un valore cambia l'impronta dell'insieme, non l'hash della grammatica; i limiti di
// riconoscimento dell'indice sono quelli che Riconosci applica.
func TestLimitiVersioneNelloSnapshotEImpronta(t *testing.T) {
	ind, contenuti := indiceACME(t, limitiACME())
	a, d, err := CompilaInsieme(ind, contenuti)
	if err != nil {
		t.Fatalf("%v\n%s", err, elenco(d))
	}
	ma := a.Motori[clienteACME]
	if ma == nil {
		t.Fatalf("cliente non attivo:\n%s", elenco(d))
	}
	if ma.Snapshot().VersioneLimiti != "limiti-acme-1" {
		t.Errorf("VersioneLimiti %q, attesa la versione_limiti dell'indice", ma.Snapshot().VersioneLimiti)
	}

	altri := conRiconoscimento(1048576, 1)
	altri.Versione = "limiti-acme-2"
	ind2, _ := indiceACME(t, altri)
	b, d, err := CompilaInsieme(ind2, contenuti)
	if err != nil {
		t.Fatalf("%v\n%s", err, elenco(d))
	}
	mb := b.Motori[clienteACME]
	if mb == nil {
		t.Fatalf("cliente non attivo con i limiti nuovi:\n%s", elenco(d))
	}
	if a.ImprontaIndice == "" || a.ImprontaIndice == b.ImprontaIndice {
		t.Errorf("impronte dell'insieme %q e %q: un limite cambiato deve cambiarla", a.ImprontaIndice, b.ImprontaIndice)
	}
	if ma.Snapshot().Hash != mb.Snapshot().Hash {
		t.Errorf("hash della grammatica cambiato con i limiti: %s, %s", ma.Snapshot().Hash, mb.Snapshot().Hash)
	}
	if mb.Snapshot().VersioneLimiti != "limiti-acme-2" {
		t.Errorf("VersioneLimiti %q", mb.Snapshot().VersioneLimiti)
	}
	letture, diag := riconosci(t, mb, "corpo", "P7120100 P7120101")
	if len(letture) != 1 || len(limiteSuperato(t, diag)) != 1 {
		t.Errorf("i limiti di riconoscimento dell'indice non sono applicati: %s\n%s", riassunto(letture), elenco(diag))
	}
}

// TestLimitiByteUnitaLetturaACavallo: una lettura che comincia nei primi max_byte_unita byte e finisce oltre
// non diventa un codice più corto: il risultato è parziale (con l'avviso) ed è fatto solo di letture che il
// motore darebbe senza il limite. Il piano non dice se la lettura a cavallo resti: si prova solo che non se
// ne inventa una.
func TestLimitiByteUnitaLetturaACavallo(t *testing.T) {
	g := grammaticaACME(famMarcatoreX())
	pieno, _, err := compila(t, g, limitiACME())
	if err != nil {
		t.Fatal(err)
	}
	testo := "9123456X1_PRT"
	intero, _ := riconosci(t, pieno, "radice_step.id", testo)
	m, d, err := compila(t, g, conRiconoscimento(6, 1000))
	if err != nil {
		t.Fatalf("%v\n%s", err, elenco(d))
	}
	letture, diag := riconosci(t, m, "radice_step.id", testo)
	sottoinsieme(t, letture, intero)
	if len(limiteSuperato(t, diag)) == 0 {
		t.Errorf("testo oltre il limite senza avviso")
	}
}
