package classificazione

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"promatec/cockpit/internal/core/registro/regole"
)

// L1 — le minuterie dei codici (revisione del 25/09).

// CodiceRev rilegge i NOSTRI nomi sul NAS (documenti.NomeTecnico, D21/D22): un file che torna indietro
// dal NAS — un documento ricaricato, un allegato di una nostra mail — deve dare il codice e la
// revisione che l'hanno fatto nascere, non «X_REV» con revisione «B».
func TestCodiceRevRileggeINostriNomiSulNas(t *testing.T) {
	casi := []struct{ nome, codice, rev string }{
		{"77720517_REV_B", "77720517", "B"},
		{"77720517_REV_B_2", "77720517", "B"},
		{"77720517_REV_ND", "77720517", ""},
		{"77720517_REV_ND_3", "77720517", ""},
		{"PZ-001_REV_04", "PZ-001", "04"},
		{"pz-001_rev_c", "PZ-001", "C"},
		// le convenzioni di sempre non cambiano
		{"1234567A_4", "1234567A", "4"},
		{"1234567A-B", "1234567A", "B"},
		{"1234567A_REV4", "1234567A", "4"},
		{"1234567A_R3", "1234567A", "3"},
		{"1234567A", "1234567A", ""},
	}
	for _, c := range casi {
		if k, r := CodiceRev(c.nome); k != c.codice || r != c.rev {
			t.Errorf("CodiceRev(%q) = (%q, %q), atteso (%q, %q)", c.nome, k, r, c.codice, c.rev)
		}
	}
	// e il giro completo: il nome del file diventa la proposta giusta
	if p := PropostaDaNome("77720517_REV_B_2.pdf", 100_000, "entrata"); p.Codice != "77720517" || p.Rev != "B" {
		t.Errorf("proposta dal nome sul NAS: codice %q rev %q", p.Codice, p.Rev)
	}
}

// Un codice non contiene spazi, di nessun tipo. Lo spazio unificatore, quello stretto e quello a
// larghezza zero arrivano dai cartigli, dai nomi dei file e dal corpo HTML, e a occhio non si vedono:
// «77720517», spazio unificatore (U+00A0), «B» passava come un codice solo.
func TestUnCodiceConUnoSpazioUnicodeNonEAmmissibile(t *testing.T) {
	// i caratteri si scrivono per numero: nel sorgente, scritti per esteso, non si vedrebbero
	spazi := []rune{0x00A0, 0x202F, 0x2007, 0x200B, 0x2028, 0x0085, 0xFEFF}
	var codici []string
	for _, r := range spazi {
		codici = append(codici, "77720517"+string(r)+"B")
	}
	for _, s := range codici {
		if CodiceAmmissibile(s) {
			t.Errorf("CodiceAmmissibile(%q) = true", s)
		}
		if sembraCodice(s) {
			t.Errorf("sembraCodice(%q) = true", s)
		}
	}
	if RevAmmissibile("B" + string(rune(0x00A0)) + "1") {
		t.Error("RevAmmissibile con lo spazio unificatore = true")
	}
	// quelli buoni restano buoni, anche con una lettera accentata
	for _, s := range []string{"77720517", "PZ-001_B", "ÀBC12345"} {
		if !CodiceAmmissibile(s) {
			t.Errorf("CodiceAmmissibile(%q) = false", s)
		}
	}
}

// «richiesta d’offerta» con l'apostrofo tipografico, che Outlook e Word mettono da soli, è una
// richiesta d'offerta: gli stessi trentacinque punti dell'apostrofo dritto.
func TestLApostrofoTipograficoELaStessaRichiestaDOfferta(t *testing.T) {
	dritto := Triage(IngressoTriage{Direzione: "entrata", Oggetto: "Richiesta d'offerta", Corpo: "in allegato"})
	curvo := Triage(IngressoTriage{Direzione: "entrata", Oggetto: "Richiesta d’offerta", Corpo: "in allegato"})
	if !contieneMotivo(curvo.Motivi, "richiesta d’offerta") {
		t.Errorf("l'apostrofo tipografico non è stato riconosciuto: %v", curvo.Motivi)
	}
	if curvo.Confidenza != dritto.Confidenza {
		t.Errorf("confidenza %d con l'apostrofo tipografico, %d con quello dritto", curvo.Confidenza, dritto.Confidenza)
	}
}

// spezzaFrasi con l'espressione compilata una volta sola divide come prima: sul punto, sul punto e
// virgola, sul punto esclamativo e interrogativo seguiti da spazio, e a ogni riga.
func TestSpezzaFrasiDivideComePrima(t *testing.T) {
	got := spezzaFrasi("Buongiorno. Vi abbiamo caricato sul portale i CAD; grazie!\r\nCordiali saluti? Sì\n\n1.5 mm")
	atteso := []string{"Buongiorno", "Vi abbiamo caricato sul portale i CAD", "grazie!", "Cordiali saluti", "Sì", "1.5 mm"}
	if !reflect.DeepEqual(got, atteso) {
		t.Errorf("spezzaFrasi = %q, atteso %q", got, atteso)
	}
}

// Una finestra di aggancio fuori da 1..3650 vale come non dichiarata (0 = il default dell'aggancio),
// come ogni altra regola ✗: le regole lette dal database non sono passate per forza dal convalidatore.
func TestUnaFinestraImpossibileValeComeNonDichiarata(t *testing.T) {
	for g, atteso := range map[int]int{45: 45, 1: 1, 3650: 3650, 0: 0, -5: 0, 3651: 0, 90000: 0} {
		if got := Compila("ACME", regole.Regole{FinestraAggancioGG: g}).Finestra(); got != atteso {
			t.Errorf("finestra_aggancio_gg = %d: Finestra() = %d, atteso %d", g, got, atteso)
		}
	}
	var nessuno *Motore
	if nessuno.Finestra() != 0 {
		t.Error("un motore nil non ha finestra")
	}
}

// Smistamento 4.13 — i numeri della firma non sono codici. Il CAP dell'indirizzo, la legge sulla
// privacy, i segnaposto ATT0000n degli allegati incorporati e il numero di una tabella «COD-nn-nnn»
// uscivano dall'estrattore generico a ogni mail del portale di un cliente. Firma e numeri inventati.
func TestINumeriDellaFirmaNonSonoCodici(t *testing.T) {
	corpo := "Buongiorno,\nvi chiediamo un preventivo per 7120001 e 7120010 secondo la tabella COD-99-123 REV.11.\n" +
		"Resto in attesa di Vs. riscontro.\n\n" +
		"Mario Rossi\nACME S.p.A.\nVia dell'Esempio 12 - 40999 Città Esempio (BO)\nSede operativa: CAP 40998, I-40997 Borgo Esempio\n" +
		"Informativa ai sensi del D.Lgs. 999/2099 e del Regolamento (UE) 2099/679."
	allegati := []string{"ATT00001.bin", "ATT00002.htm", "7120011.pdf"}
	rumore := []string{"40999", "40998", "40997", "999/2099", "2099/679", "ATT00001", "ATT00002", "COD-99-123"}

	// l'estrattore generico: restano i codici, non la firma
	nomi := make([]string, 0, len(allegati))
	for _, n := range allegati {
		nomi = append(nomi, strings.TrimSuffix(n, n[strings.LastIndex(n, "."):]))
	}
	trovati := EstraiCodici(append([]string{corpo}, nomi...)...)
	if !reflect.DeepEqual(trovati, []string{"7120001", "7120010", "7120011"}) {
		t.Errorf("EstraiCodici = %q, attesi i soli 7120001, 7120010 e 7120011", trovati)
	}

	// il triage senza regole del cliente (tutto è proponibile) e con le famiglie (il generico va fra
	// gli «altri numeri»): la firma non c'è né fra i codici né fra gli altri
	in := dalCliente("SI CTE 12345 26999999 - codici nuovi", corpo, allegati...)
	senza := Triage(in)
	in.Motore = motoreDiProva(t)
	con := Triage(in)
	for _, tr := range []EsitoTriage{senza, con} {
		for _, c := range tr.Estrazione.Codici {
			for _, r := range rumore {
				if strings.EqualFold(c.Codice, r) {
					t.Errorf("un numero della firma è finito fra i codici trovati: %+v", c)
				}
			}
		}
	}
	if !slices.Contains(senza.Codici, "7120001") {
		t.Errorf("il codice vero non c'è più: %q", senza.Codici)
	}

	// la forma intorno decide, non il numero: gli stessi numeri fuori da una firma restano candidati
	// (cinque cifre da sole possono essere un codice vero)
	fuori := EstraiCodici("Particolari 40999 e 2099/679 da quotare")
	for _, atteso := range []string{"40999", "2099/679"} {
		if !slices.Contains(fuori, atteso) {
			t.Errorf("%s fuori da una firma non è più un candidato: %q", atteso, fuori)
		}
	}
	// e una riga d'indirizzo toglie il CAP, non il codice che la precede nella stessa mail
	if got := EstraiCodici("Codice 7120012.\nVia dell'Esempio 3, 40996 Città"); !reflect.DeepEqual(got, []string{"7120012"}) {
		t.Errorf("riga d'indirizzo: %q", got)
	}
	// e le altre forme della firma, una per una (luogo a capo, civico con la lettera, luogo con la
	// provincia a capo o dopo un trattino, legge con «n.»). Il luogo con la provincia vale vicino a un
	// indirizzo o dopo i saluti: il caso «Codice 7120012\n40993 San Paolo di Esempio (BO)», senza né
	// l'uno né gli altri, è stato riscritto (giro di correzione 2 della 4.13) nelle tre forme con la
	// provincia qui sotto, perché senza un indirizzo ha la forma di un elenco di codici (la prova dopo).
	for _, firma := range []string{
		"Codice 7120012\nVia Esempio 3/A\n40995 Borgo Esempio",
		"Codice 7120012\nViale del Lavoro 12b, 40994 Città",
		"Codice 7120012\nVia Esempio, 5 int. 2\n40993 San Paolo di Esempio (BO)",
		"Codice 7120012\nCordiali saluti\nMario Rossi\nACME S.r.l.\n40993 San Paolo di Esempio (BO)",
		"Codice 7120012\r\nDistinti saluti\r\nACME S.r.l.\r\n40993 San Paolo di Esempio (BO)\r\n",
		"Codice 7120012, Piazza Esempio 1 - 40992 Città (MI)",
		"Codice 7120012 - Legge n. 123/2099",
		"Codice 7120012\r\nVia dell'Esempio 12\r\n40991 Città Esempio",
	} {
		if got := EstraiCodici(firma); !reflect.DeepEqual(got, []string{"7120012"}) {
			t.Errorf("%q: %q, atteso il solo 7120012", firma, got)
		}
	}

	// cinque cifre con una parola maiuscola dopo, fuori da un indirizzo, restano un codice: la sigla del
	// lato non è una provincia; la sigla del materiale lo è (PA, PC, AL, TO…), ma senza un indirizzo
	// vicino e fuori dalla firma non conta; e «via» come preposizione (via mail, via PEC, via DHL, anche
	// seguita da una frase con un numero piccolo e una virgola) non apre una via
	for _, testo := range []string{
		"Particolare 71200 Staffa (SX) da quotare",
		"71200 Staffa (SX)\n71201 Staffa (DX)",
		"- 71200 Supporto (LH)",
		"Vi inviamo via mail il disegno 71200 Rev.B da quotare",
		"Spedizione via DHL del particolare 71200 Supporto",
		"Vi mandiamo via PEC 2 disegni: 71200 Supporto",
		"Vi inviamo via PEC i disegni in rev. 2, 71200 Supporto e 71201 Staffa",
		"Spedito via DHL, colli 2\n71200 Supporto",
		"Consegna via Bartolini entro 3, 71200 Staffa",
		"Spedito Via Corriere, colli 2 - 71200 Supporto",
		"Codici da quotare:\n71200 Boccola (PA)\n71201 Staffa (AL)",
		"- 71200 Supporto (PC)",
		"quantita 20, 71200 Staffa (TO)",
		"Codice da quotare:\r\n71200 Boccola (PE)\r\n",
	} {
		if got := EstraiCodici(testo); !slices.Contains(got, "71200") {
			t.Errorf("%q: il codice 71200 non è più un candidato: %q", testo, got)
		}
		if strings.Contains(testo, "71201") && !slices.Contains(EstraiCodici(testo), "71201") {
			t.Errorf("%q: il codice 71201 non è più un candidato: %q", testo, EstraiCodici(testo))
		}
	}
	// e la firma si legge per poche righe dopo i saluti: più sotto (il messaggio citato) un codice con la
	// sigla del materiale resta un codice
	citato := "Cordiali saluti\nMario Rossi\n\n" + strings.Repeat("-\n", righeDiFirma) +
		"Da: Fornitore Esempio\nCodici da quotare:\n71200 Boccola (PA)"
	if got := EstraiCodici(citato); !slices.Contains(got, "71200") {
		t.Errorf("il codice sotto la firma, nel messaggio citato, non è più un candidato: %q", got)
	}
}

// Ritocco della 4.13 — il filtro dei numeri della firma legge il testo una volta sola. La prima versione
// rileggeva, per ogni CAP con la provincia, i saluti di tutto il testo che lo precede e le vie delle sue
// due righe: 5000 righe «71200 Boccola (PA) …» costavano mezzo minuto, 5000 CAP su una riga sola (un
// corpo senza a capo) altrettanto, e il triage gira nell'ingest. La lettura nuova deve dare gli STESSI
// esiti: si confronta con quella di prima (senzaNumeriDiFirmaDiPrima, qui sotto com'era) su corpi
// piccoli, poi si misura su quelli lunghi.
func TestFirmaInUnaLettura(t *testing.T) {
	// 1. gli esiti. Prima i confini scritti a mano: il saluto a righeDiFirma righe e una dopo, sulla stessa
	// riga del CAP, la via sulla riga prima, due righe sopra, fra due CAP della stessa riga, e il CRLF.
	corpi := []string{
		"Codice 7120012\nCordiali saluti\nMario Rossi\nACME S.r.l.\n40993 San Paolo di Esempio (BO)",
		"Codice 7120012\r\nDistinti saluti\r\nACME S.r.l.\r\n40993 San Paolo di Esempio (BO)\r\n",
		"Codice 7120012\nVia Esempio, 5 int. 2\n40993 San Paolo di Esempio (BO)",
		"Codice 7120012, Piazza Esempio 1 - 40992 Città (MI)",
		"Codici da quotare:\n71200 Boccola (PA)\n71201 Staffa (AL)",
		"71200 Boccola (PA)",
		"Cordiali saluti - 71200 Boccola (PA)\n71201 Staffa (AL)",
		"71200 Boccola (PA), Via Esempio 3, 71201 Staffa (AL)",
		"Via Esempio 3\n\n71200 Boccola (PA)\nVia Esempio 4\n71201 Staffa (AL)",
		"Spedito via DHL, colli 2 - 71200 Supporto (PA)",
	}
	for d := 0; d <= righeDiFirma+2; d++ {
		corpi = append(corpi, "Cordiali saluti\n"+strings.Repeat("-\n", d)+"71200 Boccola (PA)",
			"Grazie e cordiali saluti\r\nACME\r\n"+strings.Repeat("\r\n", d)+"- 71200 Boccola (PA) - 71201 Staffa (AL)")
	}
	// Poi corpi composti a caso (con un seme fisso: sono sempre gli stessi) da righe di firma, di
	// indirizzo e di elenco, unite da a capo o dagli stessi separatori su una riga sola.
	righe := []string{
		"Cordiali saluti", "Distinti saluti,", "Grazie e cordiali saluti", "Best regards", "Mario Rossi",
		"ACME S.r.l.", "Da: Fornitore Esempio", "Codice 7120012", "-", "",
		"Via dell'Esempio 12", "Via Esempio, 5 int. 2", "Corso Esempio 3", "Viale del Lavoro 12b, 40994 Città",
		"Via dell'Esempio 12 - 40999 Città Esempio (BO)", "40993 San Paolo di Esempio (BO)",
		"Piazza Esempio 1 - 40992 Città (MI), 71201 Staffa (AL)", "71200 Boccola (PA)",
		"71201 Staffa (AL) - 71202 Supporto (TO)", "- 71200 Supporto (PC)",
		"Codici: 71200 Boccola (PA), 71201 Staffa (AL); 71202 Perno (TO)",
		"Spedito via DHL, colli 2 - 71200 Supporto (PA)", "saluti a tutti - 71203 Boccola (PE)",
		"   Cordialmente, 71204 Staffa (CR)", "Corso Esempio 3 | 71205 Boccola (PA)",
	}
	separatori := []string{"\n", "\n", "\r\n", "\r\n", ", ", " - ", " | "}
	r := rand.New(rand.NewPCG(4, 13))
	for range 400 {
		var b strings.Builder
		for k := range 1 + r.IntN(25) {
			if k > 0 {
				b.WriteString(separatori[r.IntN(len(separatori))])
			}
			b.WriteString(righe[r.IntN(len(righe))])
		}
		corpi = append(corpi, b.String())
	}
	tolti, lasciati := 0, 0
	for _, c := range corpi {
		prima, dentro, fuori := senzaNumeriDiFirmaDiPrima(c)
		if got := senzaNumeriDiFirma(c); got != prima {
			t.Errorf("%q:\n  lettura nuova:    %q\n  lettura di prima: %q", c, got, prima)
		}
		tolti, lasciati = tolti+dentro, lasciati+fuori
	}
	t.Logf("%d corpi: %d CAP tolti e %d lasciati, dalle due letture", len(corpi), tolti, lasciati)
	// la prova vale se i corpi passano da tutte e due le strade: CAP tolti e CAP lasciati
	if tolti < 100 || lasciati < 100 {
		t.Errorf("i corpi di prova non bastano: %d CAP tolti e %d lasciati dalla lettura di prima", tolti, lasciati)
	}

	// 2. i tempi: 5000 righe d'elenco sotto una firma, e 5000 CAP su una riga sola. La soglia è larga
	// apposta: la prova non misura la velocità, misura che il costo non cresca col quadrato (prima:
	// mezzo minuto ciascuno). E sul corpo piccolo della stessa forma l'esito è quello di prima.
	elenco := func(n int) string {
		var b strings.Builder
		b.WriteString("Cordiali saluti\nMario Rossi\n")
		for k := range n {
			fmt.Fprintf(&b, "71200 Boccola (PA) descrizione della riga %d\n", k)
		}
		return b.String()
	}
	suUnaRiga := func(n int) string { return "Codici:" + strings.Repeat(", 71200 Boccola (PA)", n) }
	for _, caso := range []struct {
		nome  string
		corpo func(int) string
	}{{"5000 righe", elenco}, {"5000 CAP su una riga", suUnaRiga}} {
		const piccolo, grande = 50, 5000
		prima, dentro, _ := senzaNumeriDiFirmaDiPrima(caso.corpo(piccolo))
		if got := senzaNumeriDiFirma(caso.corpo(piccolo)); got != prima {
			t.Errorf("%s, corpo piccolo:\n  lettura nuova:    %q\n  lettura di prima: %q", caso.nome, got, prima)
		}
		lungo := caso.corpo(grande)
		inizio := time.Now()
		codici := EstraiCodici(lungo)
		if d := time.Since(inizio); d > time.Second {
			t.Errorf("%s (%d byte): EstraiCodici ha impiegato %v, la soglia è 1s", caso.nome, len(lungo), d)
		}
		if !reflect.DeepEqual(codici, []string{"71200"}) {
			t.Errorf("%s: EstraiCodici = %q, atteso il solo 71200", caso.nome, codici)
		}
		// la firma in testa toglie le stesse righe che toglie sul corpo piccolo, e nessun'altra
		if n := strings.Count(senzaNumeriDiFirma(lungo), "71200"); n != grande-dentro {
			t.Errorf("%s: restano %d CAP su %d, attesi %d (la lettura di prima ne toglie %d)", caso.nome, n, grande, grande-dentro, dentro)
		}
	}
	if _, dentro, _ := senzaNumeriDiFirmaDiPrima(elenco(50)); dentro == 0 {
		t.Error("la firma in testa all'elenco non toglie niente: la prova dei tempi non passa dai saluti")
	}
}

// senzaNumeriDiFirmaDiPrima è la lettura della 4.13 prima del ritocco, quadratica, copiata com'era (con in
// più il conto dei CAP tolti e lasciati): è il riferimento di TestFirmaInUnaLettura.
func senzaNumeriDiFirmaDiPrima(t string) (testo string, tolti, lasciati int) {
	for _, re := range reNumeriDiFirma {
		t = togli(t, re.FindAllStringSubmatchIndex(t, -1))
	}
	var inFirma [][]int
	for _, m := range reCAPConProvincia.FindAllStringSubmatchIndex(t, -1) {
		if capInFirmaDiPrima(t, m[2]) {
			inFirma = append(inFirma, m)
		} else {
			lasciati++
		}
	}
	return togli(t, inFirma), len(inFirma), lasciati
}

func capInFirmaDiPrima(t string, i int) bool {
	inizioRiga := strings.LastIndexByte(t[:i], '\n') + 1
	inizioPrima := 0
	if inizioRiga > 0 {
		inizioPrima = strings.LastIndexByte(t[:inizioRiga-1], '\n') + 1
	}
	if reViaConCivico.MatchString(t[inizioPrima:i]) {
		return true
	}
	for _, s := range reSaluto.FindAllStringIndex(t[:inizioRiga], -1) {
		if strings.Count(t[s[1]:inizioRiga], "\n") <= righeDiFirma {
			return true
		}
	}
	return false
}
