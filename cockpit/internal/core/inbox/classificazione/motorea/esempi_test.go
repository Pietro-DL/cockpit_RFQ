package motorea

import (
	"reflect"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L1 — la verifica degli esempi in CompilaVerificato (A1a-ESE; piano A, par.4.4.5 «Verifica degli esempi»,
// 4.7.2; P1 §5.4, §7.1; R25 f): un positivo non riconosciuto, un negativo riconosciuto, una lettura in più,
// campi diversi, due forme della stessa famiglia sulla stessa occorrenza sono errori, e l'attivazione è
// vietata; altre_ammesse ammette le letture in più, non le collisioni; gli esempi delle forme che non entrano
// nel motore restano non verificati, mai passati. La verifica usa il motore intero del cliente: anche le
// altre famiglie sullo stesso selettore.
//
// I clienti di questi test sono inventati: il repository è pubblico, e le famiglie di codice dei clienti veri
// sono un dato dell'azienda. Le grammatiche sono quelle ACME di sintetiche_test.go.

// conEsempio: acme-prefisso con un esempio in più.
func conEsempio(e grammatica.EsempioCodice) grammatica.Grammatica {
	f := famPrefisso()
	f.Esempi = append(f.Esempi, e)
	return grammaticaACME(f)
}

func TestEsempioPositivoNonRiconosciuto(t *testing.T) {
	d := compilaErrore(t, conEsempio(positivo("e-no", "corpo", "Q7120100",
		grammatica.LetturaAttesa{Forma: "mail", Base: "7120100"})), CodiceEsempioMancante)
	for _, x := range conCodice(d, CodiceEsempioMancante) {
		if x.Percorso == "" || !strings.Contains(x.Percorso, "e-no") {
			t.Errorf("percorso %q: deve dire quale esempio", x.Percorso)
		}
		if x.Natura != evidenze.NaturaContratto {
			t.Errorf("natura %q", x.Natura)
		}
	}
}

func TestEsempioNegativoRiconosciuto(t *testing.T) {
	compilaErrore(t, conEsempio(negativo("n-no", "corpo", "P7120100")), CodiceEsempioInEccesso)
}

func TestEsempioLetturaInPiu(t *testing.T) {
	compilaErrore(t, conEsempio(positivo("e-due", "corpo", "P7120100 P7120101",
		grammatica.LetturaAttesa{Forma: "mail", Base: "7120100"})), CodiceEsempioInEccesso)
}

func TestEsempioCampiDiversi(t *testing.T) {
	casi := map[string]grammatica.LetturaAttesa{
		"base":        {Forma: "mail", Base: "7120101"},
		"affissi":     {Forma: "mail", Base: "7120100", Affissi: []string{"Q"}},
		"decorazioni": {Forma: "mail", Base: "7120100", Decorazioni: []string{"ACME-030"}},
		"revisione":   {Forma: "mail", Base: "7120100", Revisione: "01"},
		"marcatore":   {Forma: "mail", Base: "7120100", Marcatore: "A"},
		"mancanti":    {Forma: "mail", Base: "7120100", Mancanti: []string{"codice"}},
	}
	for _, nome := range []string{"base", "affissi", "decorazioni", "revisione", "marcatore", "mancanti"} {
		t.Run(nome, func(t *testing.T) {
			compilaErrore(t, conEsempio(positivo("e-diverso", "corpo", "P7120100", casi[nome])), CodiceEsempioDiverso)
		})
	}
}

// TestEsempioFormaDiversa: la lettura c'è, ma di un'altra forma della famiglia: l'atteso manca e la lettura
// prodotta è in più.
func TestEsempioFormaDiversa(t *testing.T) {
	d := compilaErrore(t, conEsempio(positivo("e-forma", "corpo", "P7120100", grammatica.LetturaAttesa{Forma: "cartiglio"})),
		CodiceEsempioMancante)
	if len(conCodice(d, CodiceEsempioInEccesso)) == 0 {
		t.Errorf("la lettura della forma della mail non attesa: manca esempio_in_eccesso:\n%s", elenco(d))
	}
}

// TestEsempioDueFormeSullaStessaOccorrenza: con la forma STEP senza il confine parola_ascii, sulla radice
// «9123456A_PRT» leggono due forme della famiglia: forma.collisione_esempi, anche con altre_ammesse. Nessuna
// precedenza per ordine di array (R25 f).
func TestEsempioDueFormeSullaStessaOccorrenza(t *testing.T) {
	f := famMarcatore()
	for i := range f.Forme {
		if f.Forme[i].ID == "step" {
			f.Forme[i].ConfineDopo = nil
		}
	}
	var esempi []grammatica.EsempioCodice
	for _, e := range f.Esempi {
		if e.ID != "n-nodo-prt" {
			esempi = append(esempi, e)
		}
	}
	f.Esempi = esempi
	d := compilaErrore(t, grammaticaACME(f), CodiceFormaCollisioneEsempi)
	for _, x := range conCodice(d, CodiceFormaCollisioneEsempi) {
		if !strings.Contains(x.Percorso, "e-step-prt") {
			t.Errorf("collisione su %q, attesa sull'esempio della radice con il token", x.Percorso)
		}
	}

	for i := range f.Esempi {
		if f.Esempi[i].ID == "e-step-prt" {
			f.Esempi[i].Atteso.AltreAmmesse = true
		}
	}
	d = compilaErrore(t, grammaticaACME(f), CodiceFormaCollisioneEsempi)
	if len(conCodice(d, CodiceEsempioInEccesso)) != 0 {
		t.Errorf("con altre_ammesse la lettura in più non è un eccesso:\n%s", elenco(d))
	}
}

// TestEsempioAltreAmmesse: con altre_ammesse una lettura in più non è un errore; senza, sì.
func TestEsempioAltreAmmesse(t *testing.T) {
	e := positivo("e-altre", "corpo", "P7120100 P7120101", grammatica.LetturaAttesa{Forma: "mail", Base: "7120100"})
	e.Atteso.AltreAmmesse = true
	compilaBene(t, conEsempio(e))
	// L'atteso deve comunque esserci.
	e2 := positivo("e-altre-manca", "corpo", "P7120101", grammatica.LetturaAttesa{Forma: "mail", Base: "7120100"})
	e2.Atteso.AltreAmmesse = true
	compilaErrore(t, conEsempio(e2), CodiceEsempioDiverso)
}

// TestEsempioSulMotoreIntero: un negativo di una famiglia è letto da un'altra famiglia dello stesso cliente:
// la verifica usa il motore intero, quindi è una lettura in più.
func TestEsempioSulMotoreIntero(t *testing.T) {
	f := famMarcatore()
	f.Esempi = append(f.Esempi, negativo("n-x-sul-nodo", "nodo_step.id", "912345X1"))
	// Da sola la famiglia A non legge il codice con la X.
	compilaBene(t, grammaticaACME(f))
	// Con la famiglia X lo legge un'altra famiglia: errore.
	compilaErrore(t, grammaticaACME(f, famMarcatoreX()), CodiceEsempioInEccesso)
	// Un positivo con la famiglia esplicita di un'altra famiglia si verifica.
	g := famMarcatore()
	g.Esempi = append(g.Esempi, positivo("e-x-dalla-a", "nodo_step.id", "912345X1",
		grammatica.LetturaAttesa{Famiglia: "acme-marcatore-x", Forma: "x-nodo", Base: "912345"}))
	compilaBene(t, grammaticaACME(g, famMarcatoreX()))
}

// TestEsempioDellaFormaRiservata: gli esempi di una forma riservata non si verificano, nemmeno se sono
// sbagliati: grammatica.esempio_non_verificato, nota, e il motore nasce. La forma riservata non legge
// niente.
func TestEsempioDellaFormaRiservata(t *testing.T) {
	f := famPrefisso()
	for i := range f.Forme {
		if f.Forme[i].ID == "nome-step" {
			f.Forme[i].Stato = grammatica.StatoRiservata
		}
	}
	f.Esempi = append(f.Esempi, positivo("e-sbagliato", "nome_file", "niente da leggere",
		grammatica.LetturaAttesa{Forma: "nome-step", Base: "7120199"}))
	m, d := compilaBene(t, grammaticaACME(f))
	nv := conCodice(d, CodiceEsempioNonVerificato)
	if len(nv) != 2 {
		t.Fatalf("attesi 2 esempi non verificati, trovati:\n%s", elenco(d))
	}
	for _, x := range nv {
		if x.Gravita != evidenze.GravitaNota {
			t.Errorf("gravità %q, attesa nota", x.Gravita)
		}
	}
	if letture, _ := riconosci(t, m, "nome_file", "ACME-030P7120100 00 IN_WORK.stp"); len(letture) != 0 {
		t.Errorf("la forma riservata legge: %s", riassunto(letture))
	}
	// Le altre forme sullo stesso selettore restano attive.
	letture, _ := riconosci(t, m, "nome_file", "ACME-030P7120100.pdf")
	unaLettura(t, letture, "acme-prefisso", "nome-pdf")
}

// TestEsempioDiUnaFormaConUnaDecorazioneRiservata: una forma che usa una decorazione di un tipo riservato in
// A1 (formato) non entra nel motore, con capacita.non_supportata (avviso); i suoi esempi non si verificano, e
// il resto della grammatica resta attivo (R20 c; par.12).
func TestEsempioDiUnaFormaConUnaDecorazioneRiservata(t *testing.T) {
	f := famPrefisso()
	f.Decorazioni = append(f.Decorazioni, grammatica.Decorazione{ID: "estensione", Tipo: grammatica.TipoDecorazioneFormato,
		Letterali: []string{".pdf"}, Selettori: sel("nome_file")})
	f.Forme = append(f.Forme, forma("nome-con-formato", sel("nome_file"),
		pRif(grammatica.TipoParteDecorazione, "pacchetto", 1), pRif(grammatica.TipoParteAffisso, "P", 1), pBase(),
		pRif(grammatica.TipoParteDecorazione, "estensione", 1)))
	f.Esempi = append(f.Esempi, positivo("e-formato", "nome_file", "ACME-030P7120101.pdf",
		grammatica.LetturaAttesa{Forma: "nome-con-formato", Base: "7120101"}))
	m, d := compilaBene(t, grammaticaACME(f))
	if cap := conCodice(d, grammatica.CodiceCapacitaNonSupportata); len(cap) == 0 || conGravita(cap, evidenze.GravitaAvviso) == nil {
		t.Errorf("manca capacita.non_supportata (avviso):\n%s", elenco(d))
	}
	if len(conCodice(d, CodiceEsempioNonVerificato)) != 1 {
		t.Errorf("l'esempio della forma con il formato deve restare non verificato:\n%s", elenco(d))
	}
	letture, _ := riconosci(t, m, "nome_file", "ACME-030P7120101.pdf")
	l := unaLettura(t, letture, "acme-prefisso", "nome-pdf")
	if l.Originale != "ACME-030P7120101" {
		t.Errorf("originale %q: il formato non si legge", l.Originale)
	}
}

// TestEsempioSuSelettoreRiservatoONonAttivo: una forma su un selettore riservato in A1 non ha piani; resta
// attiva sui suoi selettori attivi. Un esempio su un selettore senza forme attive né revisioni in campo
// separato è un errore: grammatica.esempio_selettore_inattivo.
func TestEsempioSuSelettoreRiservatoONonAttivo(t *testing.T) {
	f := famPrefisso()
	for i := range f.Forme {
		if f.Forme[i].ID == "cartiglio" {
			f.Forme[i].Selettori = sel("cartiglio.codice", "elenco_pdf.codice")
		}
	}
	m, d := compilaBene(t, grammaticaACME(f))
	if len(conCodice(d, grammatica.CodiceCapacitaNonSupportata)) == 0 {
		t.Errorf("il selettore riservato non è diagnosticato:\n%s", elenco(d))
	}
	if letture, _ := m.Riconosci(selettore(t, "elenco_pdf.codice"), "7120100"); len(letture) != 0 {
		t.Errorf("letture su un selettore riservato: %s", riassunto(letture))
	}
	letture, _ := riconosci(t, m, "cartiglio.codice", "7120100")
	unaLettura(t, letture, "acme-prefisso", "cartiglio")

	compilaErrore(t, conEsempio(positivo("e-testo-pdf", "testo_pdf", "P7120100",
		grammatica.LetturaAttesa{Forma: "mail", Base: "7120100"})), CodiceEsempioSelettoreInattivo)
}

// TestEsempioErroreVietaLAttivazione: con un errore negli esempi non c'è motore, l'errore è un
// *evidenze.ErroreContratto con le stesse diagnostiche restituite, e le diagnostiche sono le stesse a ogni
// compilazione.
func TestEsempioErroreVietaLAttivazione(t *testing.T) {
	g := conEsempio(negativo("n-no", "corpo", "P7120100"))
	s, err := grammatica.NuovoSnapshot(fileJSON(t, g), limitiACME())
	if err != nil {
		t.Fatalf("lo snapshot nasce: gli esempi li verifica il motore, non la validazione: %v", err)
	}
	m1, d1, err1 := CompilaVerificato(s, limitiACME())
	m2, d2, _ := CompilaVerificato(s, limitiACME())
	if m1 != nil || m2 != nil || err1 == nil {
		t.Fatal("motore attivato con un esempio sbagliato")
	}
	if !reflect.DeepEqual(d1, d2) {
		t.Errorf("diagnostiche diverse fra due compilazioni:\n%s\n---\n%s", elenco(d1), elenco(d2))
	}
	if !reflect.DeepEqual(diagnosticheDi(err1), d1) {
		t.Errorf("l'errore non porta le diagnostiche restituite:\n%s\n---\n%s", elenco(diagnosticheDi(err1)), elenco(d1))
	}
}

// TestEsempioAbbinamentoIndipendenteDallOrdine: le letture attese si abbinano tutte insieme, non una alla
// volta: un'attesa senza base non toglie la lettura a quella con la base, in qualunque ordine dell'array
// (4.4.5: nessuna precedenza per ordine di array).
func TestEsempioAbbinamentoIndipendenteDallOrdine(t *testing.T) {
	generica := grammatica.LetturaAttesa{Forma: "mail"}
	precisa := grammatica.LetturaAttesa{Forma: "mail", Base: "7120100"}
	compilaBene(t, conEsempio(positivo("e-generica-prima", "corpo", "P7120100 P7120101", generica, precisa)))
	compilaBene(t, conEsempio(positivo("e-precisa-prima", "corpo", "P7120100 P7120101", precisa, generica)))
	// Due attese che chiedono la stessa lettura: una resta senza, ed è un errore.
	compilaErrore(t, conEsempio(positivo("e-doppia", "corpo", "P7120100 P7120101", precisa, precisa)), CodiceEsempioDiverso)
}

// TestEsempioAffissiEDecorazioniColTestoLetto: affissi e decorazioni attesi sono i testi letti, come
// nell'esempio del piano A (par.4.6.6), non gli ID delle regole: un solo significato.
func TestEsempioAffissiEDecorazioniColTestoLetto(t *testing.T) {
	compilaBene(t, conEsempio(positivo("e-testo", "nome_file", "ACME-030P7120100.pdf",
		grammatica.LetturaAttesa{Forma: "nome-pdf", Base: "7120100", Decorazioni: []string{"ACME-030"}})))
	compilaErrore(t, conEsempio(positivo("e-id-regola", "nome_file", "ACME-030P7120100.pdf",
		grammatica.LetturaAttesa{Forma: "nome-pdf", Base: "7120100", Decorazioni: []string{"pacchetto"}})), CodiceEsempioDiverso)
}

// TestEsempioDiUnAltraFamigliaConSoloFormeRiservate: una lettura attesa senza forma, di un'altra famiglia che
// sul selettore ha solo una forma riservata, non si verifica: grammatica.esempio_non_verificato, non
// esempio_mancante.
func TestEsempioDiUnAltraFamigliaConSoloFormeRiservate(t *testing.T) {
	f := famPrefisso()
	f.Esempi = append(f.Esempi, positivo("e-altra-riservata", "nome_file", "T300.012345.010.pdf",
		grammatica.LetturaAttesa{Famiglia: "acme-campo-separato-lavagna"}))
	_, d := compilaBene(t, grammaticaACME(f, famLavagna()))
	trovato := false
	for _, x := range conCodice(d, CodiceEsempioNonVerificato) {
		trovato = trovato || strings.Contains(x.Percorso, "e-altra-riservata")
	}
	if !trovato {
		t.Errorf("l'esempio che attende la famiglia con la sola forma riservata deve restare non verificato:\n%s", elenco(d))
	}
}

// TestEsempioFormaConDueToken: una LetturaForma ha un posto solo per il token. Una forma che ne dichiara due
// non entra nel motore, con capacita.non_supportata (avviso), invece di perderne uno in silenzio; il suo
// esempio non si verifica e il resto della grammatica resta attivo (R20 c).
func TestEsempioFormaConDueToken(t *testing.T) {
	f := famPrefisso()
	f.Forme = append(f.Forme, forma("due-token", sel("corpo"), pBase(), pSep("-"), pToken("Q1", "00"), pSep("-"), pToken("Q1", "01")))
	f.Esempi = append(f.Esempi, positivo("e-due-token", "corpo", "7120100-00-01", grammatica.LetturaAttesa{Forma: "due-token"}))
	m, d := compilaBene(t, grammaticaACME(f))
	cap := conCodice(d, grammatica.CodiceCapacitaNonSupportata)
	if len(cap) != 1 || cap[0].Gravita != evidenze.GravitaAvviso || !strings.Contains(cap[0].Percorso, "due-token") {
		t.Errorf("attesa una capacita.non_supportata (avviso) sulla forma con due token:\n%s", elenco(d))
	}
	if nv := conCodice(d, CodiceEsempioNonVerificato); len(nv) != 1 || !strings.Contains(nv[0].Percorso, "e-due-token") {
		t.Errorf("l'esempio della forma con due token deve restare non verificato:\n%s", elenco(d))
	}
	if letture, _ := riconosci(t, m, "corpo", "7120100-00-01"); len(letture) != 0 {
		t.Errorf("la forma con due token legge: %s", riassunto(letture))
	}
	letture, _ := riconosci(t, m, "corpo", "P7120100")
	unaLettura(t, letture, "acme-prefisso", "mail")
}
