package fascicolo

// L1 — Smistamento F8: il flusso ancorato al prodotto come regole pure (addendum A5.13, A5.14.3-A5.14.4;
// decisioni dell'utente del 27/09, «bis» e «ter»). Prove 156-162, 165-173 dell'addendum, e quelle chieste per
// questa fase: il capitolato con il «documento generale» senza riferimento strutturale, la discordanza fra il
// nome e lo STEP, il PDF senza evidenze dal contenuto. Le prove con il database (181-189, 191, 192, 233) stanno
// in smistamento_db_test.go, in workerapi e in web.

import (
	"context"
	"math/rand"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/db"
)

// casoProdotto: il prodotto 7120001, con lo STEP del prodotto (radice 7120001; figli diretti 7120010 ×1 e
// 7120012 ×2; 7120011 ×2 sotto 7120010).
func casoProdotto(sc *scena, nomeStep string) db.Componente {
	p := sc.prodotto("7120001")
	sc.step(nomeStep, []string{"#1=7120001", "#2=7120010", "#3=7120012", "#4=7120011"}, []string{"#1>#2", "#1>#3*2", "#2>#4*2"})
	return p
}

// Prova 156 (A5.13.3, A-N1…A-N5; P4): il nome trova e ordina. Negativi: 7120010 non e' 7120001, 7120011 non e'
// 7120012.
func TestLaCompatibilitaDelNome(t *testing.T) {
	m := classificazione.Compila("ACME", regole.Regole{})
	casi := []struct {
		nome, path, zip, prodotto string
		peso                      int
		regole                    string
		disc                      string
	}{
		{nome: "7120001.stp", prodotto: "7120001", peso: 60, regole: "A-N1"},
		{nome: "7120001A_1.stp", prodotto: "7120001", peso: 40, regole: "A-N2", disc: DiscLetteraFinale},
		{nome: "7120001_STAFFA_SX.pdf", prodotto: "7120001", peso: 25, regole: "A-N3"},
		{nome: "Offerta 7120001 staffe.pdf", prodotto: "7120001", peso: 25, regole: "A-N3"},
		{nome: "vista.pdf", path: "7120001/disegni/vista.pdf", prodotto: "7120001", peso: 15, regole: "A-N4"},
		{nome: "7120001.stp", path: "7120001/7120001.stp", zip: "7120001_STEP.zip", prodotto: "7120001", peso: 85, regole: "A-N1 A-N4 A-N5"},
		{nome: "vista.pdf", zip: "7120001.zip", prodotto: "7120001", peso: 10, regole: "A-N5"},
		// negativi (P4)
		{nome: "7120010.pdf", prodotto: "7120001"},
		{nome: "7120011.stp", prodotto: "7120012"},
		{nome: "7120012.stp", prodotto: "7120011"},
		{nome: "vista.pdf", zip: "7120010.zip", prodotto: "7120001"},
	}
	for _, c := range casi {
		got := CompatibilitaNome(c.nome, c.path, c.zip, c.prodotto, m)
		if got.Peso != c.peso || strings.Join(got.Regole, " ") != c.regole || strings.Join(got.Discordanze, " ") != c.disc {
			t.Errorf("%s (%s, %s) per %s: peso %d, regole %v, discordanze %v; attesi %d, %q, %q", c.nome, c.path, c.zip, c.prodotto,
				got.Peso, got.Regole, got.Discordanze, c.peso, c.regole, c.disc)
		}
		if got.Compatibile() != (c.peso > 0) {
			t.Errorf("%s per %s: Compatibile() = %v", c.nome, c.prodotto, got.Compatibile())
		}
	}
}

// Prova 157 (P32): il solo nome non ancora. 7120001.stp con la radice 7120555: nessuna ancora per 7120001, e il
// motivo dice fonti_diverse. Il disegno del prodotto resta «da verificare».
func TestIlSoloNomeNonAncora(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120555", "#2=7120556"}, []string{"#1>#2"})
	sc.disegno("7120001.pdf")
	c := sc.calcola()
	a := c.Ancore["7120001"]
	if a.Livello != LivelloAssente || len(a.Portatori) != 0 {
		t.Fatalf("il nome dice il prodotto, la radice no: %+v", a)
	}
	if m := strings.Join(a.Motivi, " | "); !strings.Contains(m, "7120555") || !strings.Contains(m, DiscFontiDiverse) {
		t.Errorf("i motivi dell'ancora non dicono la radice e fonti_diverse: %s", m)
	}
	d := sc.dest(c, "7120001.pdf")
	if d.Esito != EsitoSospesa || d.Motivo != MotivoManca || len(d.Candidati) != 0 {
		t.Errorf("il disegno del prodotto senza ancora: %s %s %s", d.Esito, d.Motivo, candidati(d))
	}
}

// Prova 158 (A5.13.3, A-S1, A-S2, A-S3): l'ancora dalla radice dello STEP. Radice uguale: piena. Radice che la
// famiglia del cliente riconosce come il prodotto («7120001-01»): piena. Radice vicina («7120001A»): da
// confermare, con radice_diversa e lettera_finale.
func TestLAncoraDallaRadice(t *testing.T) {
	casi := []struct {
		nome, radice, livello, regola string
		disc                          []string
	}{
		{"7120001A_1.stp", "7120001", LivelloPiena, "A-S1", nil},
		{"assieme.stp", "7120001-01/7120001", LivelloPiena, "A-S2", nil},
		{"variante.stp", "7120001A", LivelloDaConfermare, "A-S3", []string{DiscRadiceDiversa, DiscLetteraFinale}},
	}
	for _, x := range casi {
		sc := nuovaScena(t)
		sc.prodotto("7120001")
		sc.step(x.nome, []string{"#1=" + x.radice, "#2=7120010"}, []string{"#1>#2"})
		a := sc.calcola().Ancore["7120001"]
		if a.Livello != x.livello || len(a.Portatori) != 1 || strings.Join(a.Portatori[0].Regole, " ") != x.regola ||
			strings.Join(a.Portatori[0].Discordanze, " ") != strings.Join(x.disc, " ") || a.Portatori[0].Nome != x.nome {
			t.Errorf("%s con la radice %s: %+v; atteso %s %s %v", x.nome, x.radice, a, x.livello, x.regola, x.disc)
		}
	}
}

// Prova 159: piu' radici, una delle quali e' il prodotto, non fanno un'ancora piena (A-S5): da confermare, e
// la destinazione dello STEP non si preseleziona (piu_radici).
func TestPiuRadiciNonFannoUnAncoraPiena(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("insieme.stp", []string{"#1=7120001", "#2=7120010", "#5=7120099"}, []string{"#1>#2"})
	c := sc.calcola()
	a := c.Ancore["7120001"]
	if a.Livello != LivelloDaConfermare || strings.Join(a.Portatori[0].Regole, " ") != "A-S5" || !contiene(a.Portatori[0].Discordanze, DiscPiuRadici) {
		t.Fatalf("piu' radici: %+v", a)
	}
	d := sc.dest(c, "insieme.stp")
	if d.Preselezionabile || !contiene(d.Discordanze, DiscPiuRadici) || len(d.Candidati) == 0 {
		t.Errorf("lo STEP con piu' radici: preselezionabile %v, discordanze %v, %s", d.Preselezionabile, d.Discordanze, candidati(d))
	}
	if !contiene(d.Candidati[0].Discordanze, DiscAncoraDaConfermare) {
		t.Errorf("il candidato dipende da un'ancora da confermare: %+v", d.Candidati[0])
	}
}

// Prova 160 (U7): due STEP diversi con la radice del prodotto sono ancore concorrenti: tutte e due nell'indice,
// e nessuna destinazione che ne dipende si preseleziona. Con uno solo, lo stesso disegno si preseleziona.
func TestDueAncoreConcorrentiNonSiPreselezionano(t *testing.T) {
	costruisci := func(due bool) (*scena, Calcolo) {
		sc := nuovaScena(t)
		p := casoProdotto(sc, "7120001.stp")
		if due {
			sc.step("7120001_v2.stp", []string{"#1=7120001", "#2=7120010"}, []string{"#1>#2"})
		}
		// 7120010 e' un componente deciso da una persona sotto il prodotto
		sc.arco(p, sc.componente("7120010", db.TipoComponenteSottoassieme, db.OrigineComponenteManuale), 1)
		sc.disegno("7120010.pdf")
		return sc, sc.calcola()
	}
	sc, c := costruisci(false)
	if d := sc.dest(c, "7120010.pdf"); !d.Preselezionabile || d.Candidati[0].Regola != "dest_componente_nome" {
		t.Fatalf("con un'ancora sola il componente deciso si preseleziona: %v %s %v", d.Preselezionabile, candidati(d), d.Discordanze)
	}
	sc, c = costruisci(true)
	a := c.Ancore["7120001"]
	if a.Livello != LivelloPiena || len(a.Portatori) != 2 || !contiene(a.Discordanze, DiscAncoreConcorrenti) {
		t.Fatalf("due ancore piene: %+v", a)
	}
	d := sc.dest(c, "7120010.pdf")
	if d.Preselezionabile || !contiene(d.Candidati[0].Discordanze, DiscAncoreConcorrenti) {
		t.Errorf("con due ancore concorrenti niente si preseleziona: %v %+v", d.Preselezionabile, d.Candidati[0])
	}
	// i due file che ancorano: il loro 3D e' un candidato, mai preselezionato
	for _, nome := range []string{"7120001.stp", "7120001_v2.stp"} {
		if x := sc.dest(c, nome); x.Preselezionabile || x.Candidati[0].Regola != "dest_radice_uguale" {
			t.Errorf("%s: %v %s", nome, x.Preselezionabile, candidati(x))
		}
	}
}

// Prova 161 (U3, A5.13.4): lo STEP di un sottoassieme propone i SUOI figli diretti. P → A, X nello STEP di P;
// A → Y, Z nello STEP di A (anche se lo STEP di P non li ha); B non si raggiunge da nessun prodotto:
// scollegato, e il suo file e' sospeso con radice_non_raggiungibile.
func TestLoStepDelSottoassiemeProponeISuoiFigliDiretti(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("prodotto.stp", []string{"#1=7120001", "#2=7120010", "#3=7120099"}, []string{"#1>#2", "#1>#3"})
	sc.step("7120010.stp", []string{"#1=7120010", "#2=7120011", "#3=7120012"}, []string{"#1>#2*2", "#1>#3"})
	sc.step("scollegato.stp", []string{"#1=7120555", "#2=7120556"}, []string{"#1>#2"})
	c := sc.calcola()
	ix := c.Indice
	attese := map[string]string{"7120001": AutoritaDecisa, "7120010": AutoritaDiretta, "7120099": AutoritaDiretta,
		"7120011": AutoritaDiretta, "7120012": AutoritaDiretta, "7120555": AutoritaScollegata, "7120556": AutoritaScollegata}
	for k, a := range attese {
		if v, ok := ix.Voci[k]; !ok || v.Autorita != a {
			t.Errorf("voce %s: %+v, attesa autorita' %s", k, v, a)
		}
	}
	if y := ix.Voci["7120011"]; len(y.Posizioni) != 1 || y.Posizioni[0].Padre != "7120010" || y.Posizioni[0].Qta != 2 || y.Posizioni[0].File != "7120010.stp" {
		t.Errorf("7120011 sta sotto 7120010 ×2 nello STEP di 7120010: %+v", y.Posizioni)
	}
	if sa, ok := ix.Sottoassiemi["7120010"]; !ok || sa.Livello != LivelloPiena || sa.Portatori[0].Nome != "7120010.stp" {
		t.Errorf("l'ancora del sottoassieme 7120010: %+v", sa)
	}
	if ix.UsatoDa[shaDel("prodotto.stp")] != "7120001" || ix.UsatoDa[shaDel("7120010.stp")] != "7120010" || !ix.Scollegati[shaDel("scollegato.stp")] {
		t.Errorf("chi usa quale STEP: %v, scollegati %v", ix.UsatoDa, ix.Scollegati)
	}
	d := sc.dest(c, "scollegato.stp")
	if d.Esito != EsitoSospesa || d.Motivo != MotivoRadiceLontana || len(d.Candidati) != 0 {
		t.Errorf("lo STEP scollegato: %s %s %s", d.Esito, d.Motivo, candidati(d))
	}
	// lo STEP del sottoassieme e' il 3D di 7120010 (un nodo, finche' nessuno l'accetta): bloccato no, ma mai
	// preselezionato (scelta 1)
	d = sc.dest(c, "7120010.stp")
	if len(d.Candidati) == 0 || d.Candidati[0].Regola != "dest_3d_sottoassieme" || d.Candidati[0].Ruolo != "3d_del_sottoassieme" ||
		!strings.HasPrefix(d.Candidati[0].Chiave, "nodo:") || d.Preselezionabile {
		t.Errorf("lo STEP del sottoassieme: %s, %+v", candidati(d), d.Candidati)
	}
}

// Prova 162 (A5.13.4, ter): i nodi profondi sono evidenza, non autorita'. Con lo STEP del prodotto autorizzato,
// 7120010 e' un figlio diretto (proposta vera, da accettare), 7120011 sotto di lui e' guida profonda: il suo
// disegno ha un candidato bloccato nessuna_autorita. Quando arriva lo STEP di 7120010 con altri figli, i due
// file dicono diversamente i figli dello stesso pezzo: struttura_diversa.
func TestINodiProfondiSonoEvidenzaNonAutorita(t *testing.T) {
	sc := nuovaScena(t)
	p := sc.prodotto("7120001")
	sc.step("prodotto.stp", []string{"#1=7120001", "#2=7120010", "#3=7120011"}, []string{"#1>#2", "#2>#3*2"})
	sc.autorizza(p, "prodotto.stp", "#1")
	sc.disegno("7120010.pdf")
	sc.disegno("7120011.pdf")
	c := sc.calcola()
	if v := c.Indice.Voci["7120011"]; v.Autorita != AutoritaProfonda {
		t.Fatalf("7120011 sotto 7120010: %+v", v)
	}
	if v := c.Indice.Voci["7120010"]; v.Autorita != AutoritaDiretta || !v.Nodi[0].NellAutorita {
		t.Fatalf("7120010 figlio diretto nell'autorita': %+v", v)
	}
	if d := sc.dest(c, "7120011.pdf"); candidati(d) != "1 7120011 dest_nodo_profondo [nessuna_autorita]" || d.Preselezionabile {
		t.Errorf("il disegno di un nodo profondo: %s", candidati(d))
	}
	if d := sc.dest(c, "7120010.pdf"); candidati(d) != "1 7120010 dest_nodo_diretto_nome" || d.Preselezionabile {
		t.Errorf("il disegno di un figlio diretto da accettare: %s, preselezionabile %v", candidati(d), d.Preselezionabile)
	}

	sc.step("7120010.stp", []string{"#1=7120010", "#2=7120011", "#3=7120013"}, []string{"#1>#2", "#1>#3"})
	c = sc.calcola()
	for _, k := range []string{"7120010", "7120011", "7120013"} {
		if v := c.Indice.Voci[k]; !contiene(v.Discordanze, DiscStrutturaDiversa) {
			t.Errorf("%s: i due STEP dicono diversamente i figli di 7120010: %+v", k, v)
		}
	}
	if d := sc.dest(c, "7120011.pdf"); !contiene(d.Candidati[0].Discordanze, DiscStrutturaDiversa) || d.Preselezionabile {
		t.Errorf("il disegno di 7120011 con struttura_diversa: %+v", d.Candidati)
	}
}

// Prova 165 (Domanda 4 = B): senza un riferimento strutturale l'associazione tecnica si ferma, anche con il nome
// del disegno uguale al prodotto. La valutazione c'e' (tipo e codice letti); nessun candidato; il gruppo «manca
// un riferimento strutturale» con il prodotto che non ha l'ancora.
func TestSenzaRiferimentoLAssociazioneSiFerma(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120002")
	sc.disegno("7120002.pdf")
	sc.step("7120002_vista.pdf.stp", nil, nil) // uno STEP senza struttura letta e senza lavoro in coda
	sc.f("7120002_vista.pdf.stp").Analisi = AnalisiAssente
	c := sc.calcola()
	d := sc.dest(c, "7120002.pdf")
	if d.Esito != EsitoSospesa || d.Motivo != MotivoManca || len(d.Candidati) != 0 || strings.Join(d.ProdottiSenzaAncora, " ") != "7120002" {
		t.Fatalf("senza riferimento: %+v", d)
	}
	if d.Lettura.Tipo != "disegno_2d" || d.Lettura.Codice != "7120002" {
		t.Errorf("la lettura del file resta: %+v", d.Lettura)
	}
	if e := strings.Join(d.Evidenze, " | "); !strings.Contains(e, "il nome dice 7120002, prodotto della RFQ") {
		t.Errorf("le evidenze dicono il nome e il prodotto senza ancora: %s", e)
	}
}

// Richiesta per F8 (Domanda 4 = B): una RFQ senza riferimento strutturale. Il capitolato riconosciuto dal
// contenuto riceve il «documento generale», preselezionabile; i disegni nessuna destinazione tecnica.
func TestSenzaRiferimentoIlCapitolatoHaIlGenerale(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120002")
	sc.capitolato("Capitolato fornitura.pdf")
	sc.disegno("7120002.pdf")
	sc.disegno("7120010.pdf")
	c := sc.calcola()
	if a := c.Ancore["7120002"]; a.Livello != LivelloAssente {
		t.Fatalf("nessun riferimento strutturale: %+v", a)
	}
	cap := sc.dest(c, "Capitolato fornitura.pdf")
	if cap.Esito != EsitoProposta || candidati(cap) != "1 generale dest_generale_contenuto" || !cap.Preselezionabile {
		t.Errorf("il capitolato dal contenuto: %s %s preselezionabile %v", cap.Esito, candidati(cap), cap.Preselezionabile)
	}
	for _, nome := range []string{"7120002.pdf", "7120010.pdf"} {
		if d := sc.dest(c, nome); d.Esito != EsitoSospesa || d.Motivo != MotivoManca || len(d.Candidati) != 0 {
			t.Errorf("%s: nessuna destinazione tecnica senza riferimento: %s %s %s", nome, d.Esito, d.Motivo, candidati(d))
		}
	}
}

// Prova 166: in attesa non e' mancanza. Lo STEP compatibile per nome si sta analizzando: l'ancora e' in
// attesa e il disegno e' «in elaborazione». Se l'analisi e' fallita non si aspetta: lo si dice, e il disegno
// e' «da verificare».
func TestInAttesaNonEMancanza(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.file("7120001.stp", shaDel("7120001.stp"), sc.valuta("7120001.stp", nil, "", nil))
	sc.f("7120001.stp").Analisi = AnalisiInCorso
	sc.disegno("7120010.pdf")
	c := sc.calcola()
	if a := c.Ancore["7120001"]; a.Livello != LivelloInAttesa || a.Portatori[0].Nome != "7120001.stp" {
		t.Fatalf("lo STEP in analisi: %+v", a)
	}
	if d := sc.dest(c, "7120010.pdf"); d.Esito != EsitoInAttesa || len(d.Candidati) != 0 {
		t.Errorf("il disegno aspetta lo STEP: %+v", d)
	}
	sc.f("7120001.stp").Analisi = AnalisiFallita
	c = sc.calcola()
	a := c.Ancore["7120001"]
	if a.Livello != LivelloAssente || !strings.Contains(strings.Join(a.Motivi, " | "), "7120001.stp non si è potuto leggere") {
		t.Fatalf("lo STEP la cui analisi e' fallita: %+v", a)
	}
	if d := sc.dest(c, "7120010.pdf"); d.Esito != EsitoSospesa || d.Motivo != MotivoManca {
		t.Errorf("il disegno senza lo STEP illeggibile: %s %s", d.Esito, d.Motivo)
	}
}

// Prova 167 (P15): il secondo giro legge il nome del PROPRIO file. Due copie dello stesso disegno: «7120010.pdf»
// ha il suo candidato, «copia.pdf» no, anche se la sua lettura porta il codice dell'altra copia.
func TestIlSecondoGiroLeggeIlNomeDelProprioFile(t *testing.T) {
	sc := nuovaScena(t)
	p := casoProdotto(sc, "7120001.stp")
	sc.autorizza(p, "7120001.stp", "#1")
	sha := shaDel("disegno 7120010")
	sc.file("7120010.pdf", sha, sc.valuta("7120010.pdf", &classificazione.Esito{Tipo: "disegno_2d", Fonte: "cartiglio"}, `{"termini_trovati": ["SCALA"]}`, nil))
	// la lettura della copia con il codice dell'altra: il difetto E03, che il secondo giro non deve ripetere
	i := sc.file("copia.pdf", sha, sc.valuta("7120010.pdf", &classificazione.Esito{Tipo: "disegno_2d", Fonte: "cartiglio"}, `{"termini_trovati": ["SCALA"]}`, nil))
	if sc.s.File[i].Proposta.Valutazione.Codice.Valore != "7120010" {
		t.Fatal("la scena: la lettura della copia deve portare il codice dell'altra")
	}
	c := sc.calcola()
	if d := sc.dest(c, "7120010.pdf"); len(d.Candidati) != 1 || d.Candidati[0].Codice != "7120010" {
		t.Errorf("7120010.pdf: %s", candidati(d))
	}
	if d := sc.dest(c, "copia.pdf"); len(d.Candidati) != 0 || d.Esito != EsitoNessuna || len(d.Codici) != 0 {
		t.Errorf("copia.pdf non prende il codice dell'altra copia: %s %s %+v", d.Esito, candidati(d), d.Codici)
	}
}

// Prova 168 (P33): un token del testo del PDF vale solo se e' nell'indice. Una norma o una quota nel corpo non
// e' un codice del file, non fa discordanza e non si dice; un codice del corpo che e' un pezzo della RFQ e' una
// chiave di ricerca: un candidato, ma non «dal contenuto».
func TestUnTokenDelTestoValeSoloSeEInIndice(t *testing.T) {
	sc := nuovaScena(t)
	p := casoProdotto(sc, "7120001.stp")
	sc.autorizza(p, "7120001.stp", "#1")
	sc.disegno("vista generale.pdf")
	sc.f("vista generale.pdf").EvidenzePDF = []EvidenzaContenutoPDF{
		{Fonte: FontePDFTesto, Codice: "UNI5739", Pagina: 1}, {Fonte: FontePDFTesto, Codice: "7120012", Pagina: 2}}
	d := sc.dest(sc.calcola(), "vista generale.pdf")
	if candidati(d) != "1 7120012 dest_nodo_diretto_nome" || d.Candidati[0].DalContenuto {
		t.Errorf("il token del corpo che e' nell'indice: %s %+v", candidati(d), d.Candidati)
	}
	if len(d.Codici) != 1 || d.Codici[0].Codice != "7120012" || contiene(d.Discordanze, DiscFontiDiverse) {
		t.Errorf("la norma non e' un codice del file: %+v %v", d.Codici, d.Discordanze)
	}
}

// Prova 169: lo stesso pezzo sotto due padri e' una destinazione sola, con le due posizioni (l'identita' della
// RFQ e' il pezzo, non il percorso).
func TestLoStessoPezzoSottoDuePadriEUnaDestinazione(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.prodotto("7120002")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=7120010"}, []string{"#1>#2*2"})
	sc.step("7120002.stp", []string{"#1=7120002", "#2=7120010"}, []string{"#1>#2"})
	sc.disegno("7120010.pdf")
	c := sc.calcola()
	d := sc.dest(c, "7120010.pdf")
	if len(d.Candidati) != 1 {
		t.Fatalf("un pezzo, una destinazione: %s", candidati(d))
	}
	pos := d.Candidati[0].Posizioni
	if len(pos) != 2 || pos[0].Padre != "7120001" || pos[0].Qta != 2 || pos[1].Padre != "7120002" || pos[1].Qta != 1 {
		t.Errorf("le due posizioni: %+v", pos)
	}
	if v := c.Indice.Voci["7120010"]; strings.Join(v.Prodotti, " ") != "7120001 7120002" || len(v.Nodi) != 2 {
		t.Errorf("la voce: %+v", v)
	}
}

// scenaPreselezionabile: il prodotto con lo STEP autorizzato, 7120010 figlio diretto accettato da una persona,
// e il suo disegno: la destinazione si preseleziona. Ogni caso della prova 170 le aggiunge una discordanza.
func scenaPreselezionabile(t *testing.T) (*scena, db.Componente) {
	sc := nuovaScena(t)
	p := casoProdotto(sc, "7120001.stp")
	sc.autorizza(p, "7120001.stp", "#1")
	a := sc.componente("7120010", db.TipoComponenteSottoassieme, db.OrigineComponenteManuale)
	sc.accetta("7120001.stp", "#2", a)
	sc.disegno("7120010.pdf")
	return sc, p
}

// Prova 170 (U7): con una discordanza niente e' preselezionabile, per ogni chiave di discordanza del flusso.
func TestConUnaDiscordanzaNienteEPreselezionabile(t *testing.T) {
	sc, _ := scenaPreselezionabile(t)
	d := sc.dest(sc.calcola(), "7120010.pdf")
	if !d.Preselezionabile || d.Stato != "preselezionabile" || candidati(d) != "1 7120010 dest_nodo_diretto_nome" ||
		!strings.HasPrefix(d.Candidati[0].Chiave, "componente:") {
		t.Fatalf("la scena di partenza deve essere preselezionabile: %v %s %v %+v", d.Preselezionabile, candidati(d), d.Discordanze, d.Candidati)
	}
	casi := []struct {
		chiave string
		guasta func(sc *scena, p db.Componente)
	}{
		{DiscFontiDiverse, func(sc *scena, _ db.Componente) {
			sc.f("7120010.pdf").EvidenzePDF = []EvidenzaContenutoPDF{{Fonte: FontePDFCartiglio, Codice: "7120012", Pagina: 1}}
		}},
		{DiscTipoDiverso, func(sc *scena, _ db.Componente) {
			sc.f("7120010.pdf").Proposta.Valutazione.Tipo.Stato = classificazione.StatoDiscorde
		}},
		{DiscRevFonti, func(sc *scena, _ db.Componente) {
			sc.f("7120010.pdf").Proposta.Valutazione.Rev.Stato = classificazione.StatoDiscorde
		}},
		{DiscBomCongelata, func(sc *scena, _ db.Componente) { sc.s.BomCongelata = true }},
		{DiscPreassegnato, func(sc *scena, _ db.Componente) {
			x := sc.componente("7120099", db.TipoComponenteSciolto, db.OrigineComponenteManuale)
			sc.f("7120010.pdf").Proposta.ComponenteID = uuid.NullUUID{UUID: x.ComponenteID, Valid: true}
		}},
		{DiscStrutturaDiversa, func(sc *scena, _ db.Componente) {
			sc.step("7120010_sotto.stp", []string{"#1=7120010", "#2=7120013"}, []string{"#1>#2"})
		}},
		{DiscAncoreConcorrenti, func(sc *scena, _ db.Componente) {
			sc.dich = nil // nessuna autorizzazione: due STEP pieni per il prodotto
			sc.step("7120001_v2.stp", []string{"#1=7120001", "#2=7120010"}, []string{"#1>#2"})
		}},
		{DiscAncoraDaConfermare, func(sc *scena, _ db.Componente) {
			// il prodotto ancorato solo da una radice vicina
			sc.dich = nil
			sc.s.Nodi, sc.s.Archi = nil, nil
			sc.s.File = sc.s.File[1:]
			sc.step("variante.stp", []string{"#1=7120001A", "#2=7120010"}, []string{"#1>#2"})
		}},
		{DiscQuasiUguale, func(sc *scena, _ db.Componente) {
			sc.s.File = sc.s.File[:len(sc.s.File)-1]
			sc.disegno("7120010A.pdf")
		}},
		{DiscStepNonAutorizzato, func(sc *scena, _ db.Componente) {
			sc.dich = nil
			for i := range sc.s.Nodi {
				sc.s.Nodi[i].Stato, sc.s.Nodi[i].ComponenteID, sc.s.Nodi[i].DecisoDa = db.StatoPropostaAperta, uuid.NullUUID{}, uuid.NullUUID{}
			}
		}},
		{DiscNessunaAutorita, func(sc *scena, _ db.Componente) {
			sc.s.File = sc.s.File[:len(sc.s.File)-1]
			sc.disegno("7120011.pdf")
		}},
	}
	for _, x := range casi {
		sc, p := scenaPreselezionabile(t)
		x.guasta(sc, p)
		c := sc.calcola()
		nome := sc.s.File[len(sc.s.File)-1].Nome
		if strings.HasSuffix(nome, ".stp") {
			nome = "7120010.pdf"
		}
		d := sc.dest(c, nome)
		tutte := append(append([]string(nil), d.Discordanze...), d.Candidati[0].Discordanze...)
		if d.Preselezionabile || !contiene(tutte, x.chiave) {
			t.Errorf("%s: preselezionabile %v, discordanze %v, %s", x.chiave, d.Preselezionabile, tutte, candidati(d))
		}
	}
}

// Prova 171 (FP7): le destinazioni non dipendono dall'ordine degli eventi. Lo stesso stato, con le righe e i
// file arrivati in un altro ordine, da' le stesse firme; il disegno arrivato prima dello STEP e' in attesa, e
// quando lo STEP e' letto e' lo stesso di quello arrivato dopo.
func TestLeDestinazioniNonDipendonoDallOrdineDegliEventi(t *testing.T) {
	costruisci := func(stepPrima bool) *scena {
		sc := nuovaScena(t)
		p := sc.prodotto("7120001")
		sc.prodotto("7120002")
		if stepPrima {
			sc.step("7120001.stp", []string{"#1=7120001", "#2=7120010", "#3=7120011"}, []string{"#1>#2", "#2>#3"})
			sc.disegno("7120010.pdf")
		} else {
			sc.disegno("7120010.pdf")
			sc.step("7120001.stp", []string{"#1=7120001", "#2=7120010", "#3=7120011"}, []string{"#1>#2", "#2>#3"})
		}
		sc.capitolato("Capitolato.pdf")
		sc.disegno("7120002.pdf")
		sc.autorizza(p, "7120001.stp", "#1")
		return sc
	}
	a, b := costruisci(true), costruisci(false)
	// gli stessi file con gli stessi istanti di arrivo: l'ordine degli eventi e' solo l'ordine delle righe
	for i := range b.s.File {
		b.s.File[i].RicevutoIl = a.f(b.s.File[i].Nome).RicevutoIl
	}
	rnd := rand.New(rand.NewSource(7))
	for giro := 0; giro < 5; giro++ {
		rnd.Shuffle(len(b.s.File), func(i, j int) { b.s.File[i], b.s.File[j] = b.s.File[j], b.s.File[i] })
		rnd.Shuffle(len(b.s.Nodi), func(i, j int) { b.s.Nodi[i], b.s.Nodi[j] = b.s.Nodi[j], b.s.Nodi[i] })
		rnd.Shuffle(len(b.s.Archi), func(i, j int) { b.s.Archi[i], b.s.Archi[j] = b.s.Archi[j], b.s.Archi[i] })
		rnd.Shuffle(len(b.s.Identificativi), func(i, j int) {
			b.s.Identificativi[i], b.s.Identificativi[j] = b.s.Identificativi[j], b.s.Identificativi[i]
		})
		ca, cb := a.calcola(), b.calcola()
		if ca.Indice.Firma != cb.Indice.Firma {
			t.Fatalf("giro %d: la firma dell'indice dipende dall'ordine", giro)
		}
		for id, x := range ca.Destinazioni {
			if y := cb.Destinazioni[id]; y.Firma != x.Firma {
				t.Errorf("giro %d: la destinazione di %v dipende dall'ordine:\n%+v\n%+v", giro, id, x, y)
			}
		}
	}
	// il disegno arrivato prima dello STEP: in attesa finche' lo STEP si analizza, poi come gli altri
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.disegno("7120010.pdf")
	sc.file("7120001.stp", shaDel("7120001.stp"), sc.valuta("7120001.stp", nil, "", nil))
	sc.f("7120001.stp").Analisi = AnalisiInCorso
	if d := sc.dest(sc.calcola(), "7120010.pdf"); d.Esito != EsitoInAttesa {
		t.Errorf("il disegno prima dello STEP: %s", d.Esito)
	}
}

// Prova 171: la firma non cambia se niente cambia (e le destinazioni gia' scritte non entrano nel calcolo);
// cambia con lo stato (un codice della richiesta in piu', le regole del cliente).
func TestLaFirmaNonCambiaSeNienteCambia(t *testing.T) {
	sc, _ := scenaPreselezionabile(t)
	c1 := sc.calcola()
	d1 := sc.dest(c1, "7120010.pdf")
	sc.f("7120010.pdf").Proposta.IndiceFirma, sc.f("7120010.pdf").Proposta.Firma = c1.Indice.Firma, d1.Firma
	c2 := sc.calcola()
	if c2.Indice.Firma != c1.Indice.Firma || sc.dest(c2, "7120010.pdf").Firma != d1.Firma {
		t.Fatal("stesso stato, firme diverse")
	}
	sc.s.ImprontaRegole = "regole-acme-cambiate"
	if c3 := sc.calcola(); c3.Indice.Firma == c1.Indice.Firma {
		t.Error("le regole del cliente cambiate non cambiano la firma dell'indice")
	}
	sc.s.ImprontaRegole = "regole-acme"
	sc.prodotto("7120002")
	if c4 := sc.calcola(); c4.Indice.Firma == c1.Indice.Firma {
		t.Error("un prodotto in piu' non cambia la firma dell'indice")
	}
}

// Prova 172 (P27): le fonti che non sono del cliente restano fuori dal flusso: lo STEP di un fornitore con la
// radice del prodotto non ancora niente, e i suoi file sono fuori_flusso con il motivo.
func TestLeFontiNonDelClienteRestanoFuoriDalFlusso(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=7120010"}, []string{"#1>#2"})
	sc.f("7120001.stp").DelCliente = false
	sc.disegno("7120010.pdf")
	sc.f("7120010.pdf").DelCliente = false
	sc.disegno("7120001.pdf")
	c := sc.calcola()
	if a := c.Ancore["7120001"]; a.Livello != LivelloAssente {
		t.Fatalf("lo STEP del fornitore ancora: %+v", a)
	}
	if len(c.Indice.Voci) != 0 {
		t.Errorf("i nodi di un fornitore nell'indice: %v", c.Indice.Voci)
	}
	for _, nome := range []string{"7120001.stp", "7120010.pdf"} {
		if d := sc.dest(c, nome); d.Esito != EsitoFuoriFlusso || d.Motivo != MotivoNonDelCliente || len(d.Candidati) != 0 {
			t.Errorf("%s: %s %s", nome, d.Esito, d.Motivo)
		}
	}
	if d := sc.dest(c, "7120001.pdf"); d.Esito != EsitoSospesa {
		t.Errorf("il disegno del cliente senza ancora: %s", d.Esito)
	}
}

// Prova 173 (Domanda 4 = B): i documenti non tecnici hanno la destinazione «documento generale», anche senza
// ancora; preselezionabile solo con il tipo dal contenuto. Un tipo dal solo formato (un foglio, una mail) e'
// proposto e non preselezionato.
func TestINonTecniciHannoLaDestinazioneGenerale(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.capitolato("Capitolato.pdf")
	sc.file("Offerta ricevuta.pdf", shaDel("offerta"), sc.valuta("Offerta ricevuta.pdf", &classificazione.Esito{Tipo: "offerta_promatec", Fonte: "cartiglio"},
		`{"termini_trovati": ["OFFERTA", "PREZZO"]}`, nil))
	sc.dalFormato("listino.xlsx")
	sc.dalFormato("inoltro.msg")
	c := sc.calcola()
	casi := []struct {
		nome, regola string
		presel       bool
	}{
		{"Capitolato.pdf", "dest_generale_contenuto", true},
		{"Offerta ricevuta.pdf", "dest_generale_contenuto", true},
		{"listino.xlsx", "dest_generale_estensione", false},
		{"inoltro.msg", "dest_generale_estensione", false},
	}
	for _, x := range casi {
		d := sc.dest(c, x.nome)
		if d.Esito != EsitoProposta || candidati(d) != "1 generale "+x.regola || d.Preselezionabile != x.presel {
			t.Errorf("%s: %s %s preselezionabile %v; attesi %s %v", x.nome, d.Esito, candidati(d), d.Preselezionabile, x.regola, x.presel)
		}
		if d.Candidati[0].Score != classificazione.Punteggi[x.regola].Score {
			t.Errorf("%s: lo score non e' quello della tabella S1", x.nome)
		}
	}
}

// Richiesta per F8 (decisioni del 27/09 ter): nome del file e STEP discordano. «7120012.stp» ha la radice
// 7120010: la struttura dello STEP e' un'evidenza indipendente dal nome, e la destinazione coerente con lo STEP
// viene PRIMA, anche se la regola del nome ha uno score piu' alto; ma resta una decisione dell'utente: nessuna
// preselezione, e la discordanza si mostra con le fonti di ciascun codice. Il nome del file resta com'e'.
func TestLaDiscordanzaNomeStepMetteLoStepPrimoSenzaPreselezione(t *testing.T) {
	sc := nuovaScena(t)
	p := casoProdotto(sc, "7120001.stp")
	sc.autorizza(p, "7120001.stp", "#1")
	sc.accetta("7120001.stp", "#2", sc.componente("7120010", db.TipoComponenteSottoassieme, db.OrigineComponenteManuale))
	sc.accetta("7120001.stp", "#4", sc.componente("7120011", db.TipoComponenteSciolto, db.OrigineComponenteManuale))
	sc.accetta("7120001.stp", "#3", sc.componente("7120012", db.TipoComponenteSciolto, db.OrigineComponenteManuale))
	sc.step("7120012.stp", []string{"#1=7120010", "#2=7120011"}, []string{"#1>#2*2"})
	d := sc.dest(sc.calcola(), "7120012.stp")
	if len(d.Candidati) < 2 || d.Candidati[0].Codice != "7120010" || !d.Candidati[0].DalContenuto ||
		d.Candidati[0].Regola != "dest_3d_sottoassieme" || d.Candidati[1].Codice != "7120012" || d.Candidati[1].Regola != "dest_nodo_diretto_nome" || d.Candidati[1].DalContenuto {
		t.Fatalf("la destinazione coerente con lo STEP prima: %s %+v", candidati(d), d.Candidati)
	}
	if d.Candidati[1].Score <= d.Candidati[0].Score {
		t.Errorf("la prova vuole il nome con lo score piu' alto (%d contro %d): l'ordine lo decide il contenuto", d.Candidati[1].Score, d.Candidati[0].Score)
	}
	if d.Preselezionabile || !contiene(d.Discordanze, DiscFontiDiverse) {
		t.Errorf("nome e STEP discordano: preselezionabile %v, discordanze %v", d.Preselezionabile, d.Discordanze)
	}
	fonti := map[string]string{}
	for _, c := range d.Codici {
		fonti[c.Codice] = strings.Join(c.Fonti, " ")
	}
	if fonti["7120010"] != FonteStepRadice || fonti["7120012"] != FonteNome {
		t.Errorf("le fonti di ciascun codice: %v", fonti)
	}
	if strings.Join(d.Candidati[0].Fonti, " ") != FonteStepRadice || strings.Join(d.Candidati[1].Fonti, " ") != FonteNome {
		t.Errorf("le fonti dei candidati: %v, %v", d.Candidati[0].Fonti, d.Candidati[1].Fonti)
	}
	if sc.f("7120012.stp").Nome != "7120012.stp" || d.Lettura.Codice == "" {
		t.Error("il nome del file e la sua lettura restano quelli che sono")
	}
}

// Il punto d'aggancio della lettura dei PDF (F9): oggi EvidenzeContenutoPDF non da' niente, e il flusso tratta
// il PDF come un PDF senza testo: il codice viene dal nome, il candidato non e' «dal contenuto», e lo si dice.
// Quando la normalizzazione di F9 dara' un codice del cartiglio, sara' un'evidenza indipendente: il candidato
// coerente con il cartiglio viene primo, e la discordanza con il nome non si preseleziona.
func TestSenzaEvidenzeIlPdfSiComportaComeUnPdfSenzaTesto(t *testing.T) {
	if ev, err := EvidenzeContenutoPDF(context.Background(), nil, shaDel("x"), nil); err != nil || len(ev) != 0 {
		t.Fatalf("il punto d'aggancio deve essere vuoto finche' F9 non c'e': %v %v", ev, err)
	}
	costruisci := func() *scena {
		sc, _ := scenaPreselezionabile(t)
		sc.accetta("7120001.stp", "#3", sc.componente("7120012", db.TipoComponenteSciolto, db.OrigineComponenteManuale))
		sc.disegno("7120012.pdf")
		return sc
	}
	sc := costruisci()
	d := sc.dest(sc.calcola(), "7120012.pdf")
	if candidati(d) != "1 7120012 dest_nodo_diretto_nome" || d.Candidati[0].DalContenuto || strings.Join(d.Candidati[0].Fonti, " ") != FonteNome {
		t.Errorf("il PDF senza evidenze: %s %+v", candidati(d), d.Candidati)
	}
	if !strings.Contains(strings.Join(d.Evidenze, " | "), "non ha evidenze dal contenuto") {
		t.Errorf("la destinazione dice che il PDF non ha evidenze dal contenuto: %v", d.Evidenze)
	}
	// con un codice nel cartiglio (quello che F9 portera'), diverso dal nome
	sc = costruisci()
	sc.f("7120012.pdf").EvidenzePDF = []EvidenzaContenutoPDF{{Fonte: FontePDFCartiglio, Codice: "7120010", Pagina: 1}}
	d = sc.dest(sc.calcola(), "7120012.pdf")
	if len(d.Candidati) != 2 || d.Candidati[0].Codice != "7120010" || d.Candidati[0].Regola != "dest_nodo_diretto_contenuto" ||
		d.Candidati[1].Codice != "7120012" || d.Preselezionabile || !contiene(d.Discordanze, DiscFontiDiverse) {
		t.Errorf("il cartiglio contro il nome: %s, preselezionabile %v, %v", candidati(d), d.Preselezionabile, d.Discordanze)
	}
}

// Il flusso parte solo dai prodotti confermati (scelta 3): un finito nato da un codice trovato, che nessuno ha
// confermato, non e' un prodotto; un finito creato a mano si'; un codice della richiesta il cui prodotto e'
// stato tolto dalla BOM resta fuori.
func TestIlFlussoParteSoloDaiProdottiConfermati(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.identificativo("7120002", false)
	sc.componente("7120002", db.TipoComponenteFinito, db.OrigineComponenteCodiceRilevato)
	sc.componente("7120003", db.TipoComponenteFinito, db.OrigineComponenteStep)
	sc.componente("7120004", db.TipoComponenteFinito, db.OrigineComponenteManuale)
	sc.identificativo("7120005", true)
	x := sc.componente("7120005", db.TipoComponenteFinito, db.OrigineComponenteCodiceRilevato)
	ieri := sc.t0.Add(-24 * 3600e9)
	for i := range sc.s.Componenti {
		if sc.s.Componenti[i].ComponenteID == x.ComponenteID {
			sc.s.Componenti[i].ArchiviatoIl = &ieri
		}
	}
	sc.identificativo("7120006", true) // confermato, senza componente
	var got []string
	for _, p := range ProdottiDellaRfq(&sc.s) {
		got = append(got, p.Codice)
	}
	if strings.Join(got, " ") != "7120001 7120004 7120006" {
		t.Errorf("i prodotti del flusso: %v", got)
	}
}

// Correzione di F8 (scelta 2; decisioni del 27/09): il solo «componente deciso» non basta alla preselezione.
// Una persona aggiunge a mano 7120020 sotto il prodotto ancorato dallo STEP, che non lo contiene: il disegno
// 7120020.pdf ha il candidato verso il componente, ma il legame fra il file e il pezzo viene solo dal nome
// del file, e niente e' preselezionato. Con il codice letto nel cartiglio (il contenuto) si'; e 7120010, nodo
// dello STEP che ancora il prodotto, resta preselezionabile (prova 160).
func TestIlSoloComponenteDecisoNonBastaAllaPreselezione(t *testing.T) {
	costruisci := func() *scena {
		sc := nuovaScena(t)
		p := casoProdotto(sc, "7120001.stp")
		sc.arco(p, sc.componente("7120020", db.TipoComponenteSciolto, db.OrigineComponenteManuale), 1)
		sc.disegno("7120020.pdf")
		return sc
	}
	sc := costruisci()
	c := sc.calcola()
	if a := c.Ancore["7120001"]; a.Livello != LivelloPiena {
		t.Fatalf("la scena: il prodotto ancorato dallo STEP: %+v", a)
	}
	if v := c.Indice.Voci["7120020"]; v.Autorita != AutoritaDecisa || len(v.Nodi) != 0 {
		t.Fatalf("7120020 e' un componente deciso, assente dallo STEP: %+v", v)
	}
	d := sc.dest(c, "7120020.pdf")
	if candidati(d) != "1 7120020 dest_componente_nome" || d.Preselezionabile || d.Stato != "da_scegliere" {
		t.Errorf("il disegno col solo nome verso un componente aggiunto a mano: %s, preselezionabile %v, stato %s", candidati(d), d.Preselezionabile, d.Stato)
	}
	if s := d.Candidati[0].Sostegno; !contiene(s, SostegnoComponente) || sostegnoBasta(s) {
		t.Errorf("il sostegno del candidato: %v", s)
	}
	// il codice nel cartiglio del PDF: un'evidenza dal contenuto, che basta
	sc = costruisci()
	sc.f("7120020.pdf").EvidenzePDF = []EvidenzaContenutoPDF{{Fonte: FontePDFCartiglio, Codice: "7120020", Pagina: 1}}
	if d := sc.dest(sc.calcola(), "7120020.pdf"); candidati(d) != "1 7120020 dest_componente_contenuto" || !d.Preselezionabile {
		t.Errorf("con il codice nel cartiglio: %s, preselezionabile %v, %v", candidati(d), d.Preselezionabile, d.Candidati[0].Sostegno)
	}
}

// Correzione di F8 (scelta 3, F7): un componente o un arco nati da un'evidenza non sono «decisi». Il pezzo entra
// nell'indice con i motivi (da_rivedere), il disegno ha il candidato verso il componente, ma niente e'
// preselezionato, anche quando il codice e' un nodo dello STEP che ancora il prodotto. Con il componente e
// l'arco decisi da una persona, lo stesso disegno si preseleziona.
func TestUnPezzoNatoDaUnEvidenzaNonEDeciso(t *testing.T) {
	casi := []struct {
		nome              string
		componente, arco  db.OrigineComponente
		codice, autorita  string
		presel, conMotivi bool
	}{
		{"deciso", db.OrigineComponenteManuale, db.OrigineComponenteManuale, "7120010", AutoritaDecisa, true, false},
		{"codice trovato, nello STEP", db.OrigineComponenteCodiceRilevato, db.OrigineComponenteStep, "7120010", AutoritaDiretta, false, true},
		{"codice trovato, arco a mano", db.OrigineComponenteCodiceRilevato, db.OrigineComponenteManuale, "7120010", AutoritaDiretta, false, true},
		{"arco da uno STEP non autorizzato", db.OrigineComponenteManuale, db.OrigineComponenteStep, "7120010", AutoritaDiretta, false, true},
		{"codice trovato, fuori dallo STEP", db.OrigineComponenteCodiceRilevato, db.OrigineComponenteStep, "7120020", AutoritaDaRivedere, false, true},
	}
	for _, x := range casi {
		sc := nuovaScena(t)
		p := casoProdotto(sc, "7120001.stp")
		sc.arcoDa(p, sc.componente(x.codice, db.TipoComponenteSciolto, x.componente), 1, x.arco)
		sc.disegno(x.codice + ".pdf")
		c := sc.calcola()
		v := c.Indice.Voci[x.codice]
		if v == nil || v.Autorita != x.autorita || (len(v.DaRivedere) > 0) != x.conMotivi || !v.ComponenteID.Valid {
			t.Errorf("%s: la voce %+v; attese autorita' %s e motivi %v", x.nome, v, x.autorita, x.conMotivi)
			continue
		}
		d := sc.dest(c, x.codice+".pdf")
		if len(d.Candidati) == 0 || !strings.HasPrefix(d.Candidati[0].Chiave, "componente:") || d.Preselezionabile != x.presel ||
			d.Candidati[0].DaRivedere == x.presel {
			t.Errorf("%s: %s preselezionabile %v; %+v", x.nome, candidati(d), d.Preselezionabile, d.Candidati)
			continue
		}
		if e := strings.Join(d.Candidati[0].Evidenze, " | "); x.conMotivi && !strings.Contains(e, "è da rivedere") {
			t.Errorf("%s: le evidenze non dicono perche' e' da rivedere: %s", x.nome, e)
		}
	}
	// il nodo dello STEP (non autorizzato) accettato da una persona: il componente nasce «accettato da uno STEP
	// non autorizzato», e per la Struttura BOM e' da rivedere; qui pure
	sc := nuovaScena(t)
	casoProdotto(sc, "7120001.stp")
	sc.accetta("7120001.stp", "#2", sc.componente("7120010", db.TipoComponenteSottoassieme, db.OrigineComponenteStep))
	sc.disegno("7120010.pdf")
	c := sc.calcola()
	if v := c.Indice.Voci["7120010"]; !v.ComponenteID.Valid || !contiene(v.DaRivedere, "7120010: "+ProvenienzaStepNonAutorizzato) {
		t.Fatalf("il nodo accettato da uno STEP non autorizzato: %+v", v)
	}
	if d := sc.dest(c, "7120010.pdf"); d.Preselezionabile || !d.Candidati[0].DaRivedere || contiene(d.Candidati[0].Sostegno, SostegnoComponente) {
		t.Errorf("il disegno del componente da rivedere: %s preselezionabile %v %+v", candidati(d), d.Preselezionabile, d.Candidati[0])
	}
}

// Correzione di F8 (Domanda 4 = B), variante della prova 165: la pre-assegnazione di prima non fa ripartire
// l'automatismo. Il prodotto non ha un riferimento strutturale: il disegno con un componente_id di prima resta
// sospeso con manca_riferimento_strutturale, senza candidati, e la pre-assegnazione si dice fra le evidenze.
// Con tutti i prodotti ancorati e nessun codice del file nell'indice, la pre-assegnazione resta l'unico
// candidato, mai preselezionato.
func TestSenzaRiferimentoLaPreassegnazioneNonEUnaDestinazione(t *testing.T) {
	sc := nuovaScena(t)
	x := sc.prodotto("7120002")
	sc.disegno("7120002.pdf")
	sc.f("7120002.pdf").Proposta.ComponenteID = uuid.NullUUID{UUID: x.ComponenteID, Valid: true}
	d := sc.dest(sc.calcola(), "7120002.pdf")
	if d.Esito != EsitoSospesa || d.Motivo != MotivoManca || len(d.Candidati) != 0 || strings.Join(d.ProdottiSenzaAncora, " ") != "7120002" {
		t.Fatalf("la pre-assegnazione senza riferimento: %s %s %s %v", d.Esito, d.Motivo, candidati(d), d.ProdottiSenzaAncora)
	}
	if e := strings.Join(d.Evidenze, " | "); !strings.Contains(e, "assegnato a 7120002 da un gesto di prima") {
		t.Errorf("le evidenze non dicono la pre-assegnazione: %s", e)
	}

	sc = nuovaScena(t)
	casoProdotto(sc, "7120001.stp")
	y := sc.componente("7120099", db.TipoComponenteSciolto, db.OrigineComponenteManuale)
	sc.disegno("schizzo.pdf")
	sc.f("schizzo.pdf").Proposta.ComponenteID = uuid.NullUUID{UUID: y.ComponenteID, Valid: true}
	d = sc.dest(sc.calcola(), "schizzo.pdf")
	if d.Esito != EsitoProposta || candidati(d) != "1 7120099 dest_assegnazione_precedente" || d.Preselezionabile || len(d.Candidati[0].Sostegno) != 0 {
		t.Errorf("la sola pre-assegnazione, con i prodotti ancorati: %s %s preselezionabile %v %+v", d.Esito, candidati(d), d.Preselezionabile, d.Candidati)
	}
}

// Correzione di F8 (Domanda 5 = B, ter): lo STEP di un commerciale e' solo guida. 7120010 e' un commerciale
// accettato sotto il prodotto; il suo STEP «7120010.stp» si legge e si mostra, ma i suoi nodi sono guida: il
// disegno di 7120013 ha un candidato bloccato nessuna_autorita, con «cambia prima il tipo», e mai «autorizza
// prima lo STEP» (un gesto impossibile per un commerciale). Guida anche 7120011, sotto il commerciale nello
// STEP del prodotto. Tornato sottoassieme, lo STEP di 7120010 propone di nuovo i suoi figli diretti.
func TestLoStepDiUnCommercialeESoloGuida(t *testing.T) {
	sc := nuovaScena(t)
	p := casoProdotto(sc, "7120001.stp")
	sc.autorizza(p, "7120001.stp", "#1")
	comm := sc.componente("7120010", db.TipoComponenteCommerciale, db.OrigineComponenteManuale)
	sc.accetta("7120001.stp", "#2", comm)
	sc.step("7120010.stp", []string{"#1=7120010", "#2=7120013"}, []string{"#1>#2"})
	sc.disegno("7120013.pdf")
	c := sc.calcola()
	ix := c.Indice
	if sa, ok := ix.Sottoassiemi["7120010"]; !ok || !sa.SoloGuida || !strings.Contains(strings.Join(sa.Motivi, " | "), "commerciale") {
		t.Fatalf("lo STEP del commerciale: %+v", sa)
	}
	if v := ix.Voci["7120010"]; !v.Commerciale {
		t.Errorf("la voce del commerciale: %+v", v)
	}
	for _, k := range []string{"7120013", "7120011"} {
		v := ix.Voci[k]
		if v == nil || v.Autorita != AutoritaProfonda || len(v.Nodi) == 0 || v.Nodi[0].SottoCommerciale != "7120010" {
			t.Errorf("%s sotto il commerciale: %+v", k, v)
		}
	}
	d := sc.dest(c, "7120013.pdf")
	e := strings.Join(d.Candidati[0].Evidenze, " | ")
	if candidati(d) != "1 7120013 dest_nodo_profondo [nessuna_autorita]" || d.Preselezionabile ||
		!strings.Contains(e, "cambia prima il tipo di 7120010") || strings.Contains(e, "autorizza prima lo STEP") {
		t.Errorf("il disegno sotto il commerciale: %s, %s", candidati(d), e)
	}
	if d := sc.dest(c, "7120010.stp"); len(d.Candidati) == 0 || !strings.Contains(strings.Join(d.Candidati[0].Evidenze, " | "), "un commerciale") {
		t.Errorf("il 3D del commerciale: %s %+v", candidati(d), d.Candidati)
	}
	// tornato sottoassieme: lo STEP di 7120010 e' il candidato che ne proporrebbe i figli diretti
	for i := range sc.s.Componenti {
		if sc.s.Componenti[i].ComponenteID == comm.ComponenteID {
			sc.s.Componenti[i].Tipo = db.TipoComponenteSottoassieme
		}
	}
	c2 := sc.calcola()
	if d := sc.dest(c2, "7120013.pdf"); candidati(d) != "1 7120013 dest_nodo_non_autorizzato [step_non_autorizzato]" {
		t.Errorf("tornato sottoassieme: %s", candidati(d))
	}
	if c2.Indice.Firma == c.Indice.Firma {
		t.Error("il cambio del tipo non cambia la firma dell'indice")
	}
}

// Correzione di F8 (A5.13.3: le alternative si mostrano, non sono autorita'): uno STEP con la radice del
// prodotto accanto a quello autorizzato, o con una radice vicina accanto a un'ancora piena, e' un'alternativa,
// non uno STEP scollegato: i suoi nodi sono guida con il file in uso, e nessuna evidenza dice «non collegato».
func TestUnoStepAlternativoNonEScollegato(t *testing.T) {
	casi := []struct {
		nome, alternativa, radice, uso string
		autorizza                      bool
	}{
		{"seconda radice uguale", "7120001_alt.stp", "7120001", "7120001.stp", true},
		{"radice vicina", "7120001A.stp", "7120001A", "7120001.stp", false},
	}
	for _, x := range casi {
		sc := nuovaScena(t)
		p := casoProdotto(sc, "7120001.stp")
		if x.autorizza {
			sc.autorizza(p, "7120001.stp", "#1")
		}
		sc.step(x.alternativa, []string{"#1=" + x.radice, "#2=7120015"}, []string{"#1>#2"})
		sc.disegno("7120015.pdf")
		c := sc.calcola()
		ix := c.Indice
		sha := shaDel(x.alternativa)
		if ix.Scollegati[sha] || ix.Alternative[sha] != "7120001" {
			t.Errorf("%s: scollegato %v, alternativa di %q", x.nome, ix.Scollegati[sha], ix.Alternative[sha])
		}
		v := ix.Voci["7120015"]
		if v == nil || v.Autorita != AutoritaProfonda || v.Nodi[0].Alternativa != x.uso || v.Nodi[0].AlternativaDi != "7120001" {
			t.Errorf("%s: la voce del nodo dell'alternativa: %+v", x.nome, v)
			continue
		}
		d := sc.dest(c, "7120015.pdf")
		e := strings.Join(append(append([]string(nil), d.Evidenze...), d.Candidati[0].Evidenze...), " | ")
		if candidati(d) != "1 7120015 dest_nodo_profondo [nessuna_autorita]" || d.Preselezionabile ||
			!strings.Contains(e, "alternativa a "+x.uso) || strings.Contains(e, "non collegato") {
			t.Errorf("%s: il disegno di un nodo dell'alternativa: %s, %s", x.nome, candidati(d), e)
		}
		if d := sc.dest(c, x.alternativa); d.Motivo == MotivoRadiceLontana || d.Esito != EsitoProposta {
			t.Errorf("%s: lo STEP alternativo: %s %s %s", x.nome, d.Esito, d.Motivo, candidati(d))
		}
	}
}

// Correzione di F8, secondo giro (U7, scelta 2): il nodo di un'ancora da confermare non sostiene una
// destinazione. 7120020 e' aggiunto a mano sotto 7120001, la cui ancora piena non lo contiene; il secondo
// prodotto 7120002 ha solo variante.stp, con la radice vicina 7120002A (A-S3, da confermare), che contiene
// 7120020. Il disegno 7120020.pdf col solo nome ha il candidato verso il componente, non preselezionabile:
// il livello migliore della voce (piena, da 7120001) non presta il sostegno al nodo dell'altra ancora, e
// l'evidenza dice il componente deciso sotto 7120001 soltanto. E un sottoassieme raggiunto solo da
// un'ancora da confermare (7120014, con il suo STEP pieno) non ha figli piu' affidabili di lui. Terzo giro:
// lo stesso per i nodi di due STEP pieni concorrenti di 7120002, che contengono tutti e due 7120020.
func TestIlNodoDiUnAncoraDaConfermareNonSostiene(t *testing.T) {
	sc := nuovaScena(t)
	p := casoProdotto(sc, "7120001.stp")
	sc.arco(p, sc.componente("7120020", db.TipoComponenteSciolto, db.OrigineComponenteManuale), 1)
	sc.prodotto("7120002")
	sc.step("variante.stp", []string{"#1=7120002A", "#2=7120020", "#3=7120014"}, []string{"#1>#2", "#1>#3"})
	sc.step("7120014.stp", []string{"#1=7120014", "#2=7120015"}, []string{"#1>#2"})
	sc.disegno("7120020.pdf")
	c := sc.calcola()
	if a := c.Ancore["7120002"]; a.Livello != LivelloDaConfermare {
		t.Fatalf("la scena: 7120002 ancorato solo da confermare: %+v", a)
	}
	v := c.Indice.Voci["7120020"]
	if v.Autorita != AutoritaDecisa || v.Livello != LivelloPiena || len(v.Nodi) != 1 || v.Nodi[0].Livello != LivelloDaConfermare || v.Nodi[0].affidabile() {
		t.Fatalf("7120020, deciso sotto 7120001 e nodo del solo STEP da confermare: %+v", v)
	}
	if strings.Join(v.ProdottiDecisi, " ") != "7120001" || strings.Join(v.ProdottiWorking, " ") != "7120001" || strings.Join(v.Prodotti, " ") != "7120001 7120002" {
		t.Errorf("i prodotti di 7120020: decisi %v, working %v, tutti %v", v.ProdottiDecisi, v.ProdottiWorking, v.Prodotti)
	}
	d := sc.dest(c, "7120020.pdf")
	if candidati(d) != "1 7120020 dest_componente_nome" || d.Preselezionabile || d.Stato != "da_scegliere" {
		t.Fatalf("il disegno col solo nome e un nodo da confermare: %s, preselezionabile %v, stato %s", candidati(d), d.Preselezionabile, d.Stato)
	}
	e := strings.Join(d.Candidati[0].Evidenze, " | ")
	if contiene(d.Candidati[0].Sostegno, SostegnoNodoAncora) || sostegnoBasta(d.Candidati[0].Sostegno) ||
		!strings.Contains(e, "7120020 è un componente deciso sotto 7120001 |") || strings.Contains(e, "7120001, 7120002") ||
		!strings.Contains(e, "7120020 compare nello STEP variante.stp, che ancora 7120002 con un'ancora da confermare") {
		t.Errorf("il sostegno %v e le evidenze: %s", d.Candidati[0].Sostegno, e)
	}
	if w := c.Indice.Voci["7120015"]; w == nil || w.Livello != LivelloDaConfermare || w.Nodi[0].Livello != LivelloDaConfermare {
		t.Errorf("7120015, figlio del sottoassieme raggiunto solo da un'ancora da confermare: %+v", w)
	}

	// le ancore concorrenti: 7120002 ha due STEP pieni con la sua radice, e tutti e due contengono 7120020
	sc = nuovaScena(t)
	p = casoProdotto(sc, "7120001.stp")
	sc.arco(p, sc.componente("7120020", db.TipoComponenteSciolto, db.OrigineComponenteManuale), 1)
	sc.prodotto("7120002")
	sc.step("7120002_a.stp", []string{"#1=7120002", "#2=7120020"}, []string{"#1>#2"})
	sc.step("7120002_b.stp", []string{"#1=7120002", "#2=7120020", "#3=7120021"}, []string{"#1>#2", "#1>#3"})
	sc.disegno("7120020.pdf")
	c = sc.calcola()
	if a := c.Ancore["7120002"]; a.Livello != LivelloPiena || !contiene(a.Discordanze, DiscAncoreConcorrenti) {
		t.Fatalf("la scena: 7120002 con due ancore piene concorrenti: %+v", a)
	}
	v = c.Indice.Voci["7120020"]
	if v.Autorita != AutoritaDecisa || len(v.Nodi) != 2 {
		t.Fatalf("7120020, deciso sotto 7120001 e nodo dei due STEP concorrenti: %+v", v)
	}
	for _, n := range v.Nodi {
		if !n.Concorrenti || n.Livello != LivelloPiena || n.affidabile() {
			t.Errorf("il nodo di %s: %+v", n.File, n)
		}
	}
	d = sc.dest(c, "7120020.pdf")
	e = strings.Join(d.Candidati[0].Evidenze, " | ")
	if candidati(d) != "1 7120020 dest_componente_nome" || d.Preselezionabile || contiene(d.Candidati[0].Sostegno, SostegnoNodoAncora) ||
		!strings.Contains(e, "7120020 compare nello STEP 7120002_a.stp, che ancora 7120002 con due file concorrenti: non sostiene la destinazione") {
		t.Errorf("il disegno col solo nome e i nodi di due ancore concorrenti: %s, preselezionabile %v, %v, %s", candidati(d), d.Preselezionabile,
			d.Candidati[0].Sostegno, e)
	}
}

// Correzione di F8, secondo giro (Domanda 5 = B): un criterio solo per il commerciale. 7120010 e' un
// commerciale sotto il prodotto 7120002, che non ha un'ancora; nello STEP pieno di 7120001 il nodo 7120010
// nessuno l'ha accettato. Il pezzo e' commerciale anche dal codice: il suo STEP «7120010.stp» e' solo guida
// (come i discendenti che lo STEP di 7120001 gli mette sotto), e il disegno di 7120013 ha un candidato
// bloccato nessuna_autorita con «cambia prima il tipo», mai «autorizza prima lo STEP».
func TestIlCommercialeSiRiconosceAncheDalCodice(t *testing.T) {
	sc := nuovaScena(t)
	casoProdotto(sc, "7120001.stp")
	q := sc.prodotto("7120002")
	sc.arco(q, sc.componente("7120010", db.TipoComponenteCommerciale, db.OrigineComponenteManuale), 1)
	sc.step("7120010.stp", []string{"#1=7120010", "#2=7120013"}, []string{"#1>#2"})
	sc.disegno("7120013.pdf")
	c := sc.calcola()
	ix := c.Indice
	if v := ix.Voci["7120010"]; v.ComponenteID.Valid || !v.Commerciale {
		t.Fatalf("la scena: 7120010 senza componente nell'indice, commerciale dal codice: %+v", v)
	}
	if sa, ok := ix.Sottoassiemi["7120010"]; !ok || !sa.SoloGuida {
		t.Errorf("lo STEP del commerciale riconosciuto dal codice: %+v", sa)
	}
	for _, k := range []string{"7120011", "7120013"} {
		if v := ix.Voci[k]; v == nil || v.Autorita != AutoritaProfonda || v.Nodi[0].SottoCommerciale != "7120010" || contiene(v.Discordanze, DiscStrutturaDiversa) {
			t.Errorf("%s sotto il commerciale: %+v", k, v)
		}
	}
	d := sc.dest(c, "7120013.pdf")
	e := strings.Join(d.Candidati[0].Evidenze, " | ")
	if candidati(d) != "1 7120013 dest_nodo_profondo [nessuna_autorita]" || d.Preselezionabile ||
		!strings.Contains(e, "cambia prima il tipo di 7120010") || strings.Contains(e, "autorizza prima lo STEP") {
		t.Errorf("il disegno sotto il commerciale: %s, %s", candidati(d), e)
	}
}

// Correzione di F8, secondo giro (Domanda 5 = B): lo STEP di un commerciale non porta struttura_diversa. Lo
// STEP del prodotto e' autorizzato e 7120010 e' un commerciale accettato: il suo disegno si preseleziona. Lo
// STEP di 7120010 (7120010 → 7120013, dove lo STEP del prodotto dice 7120010 → 7120011) e' solo guida:
// nessuno dei due comanda i figli di un commerciale, non c'e' niente da autorizzare, e il disegno resta
// preselezionabile.
func TestLoStepDelCommercialeNonEStrutturaDiversa(t *testing.T) {
	for _, conStep := range []bool{false, true} {
		sc := nuovaScena(t)
		p := casoProdotto(sc, "7120001.stp")
		sc.autorizza(p, "7120001.stp", "#1")
		sc.accetta("7120001.stp", "#2", sc.componente("7120010", db.TipoComponenteCommerciale, db.OrigineComponenteManuale))
		if conStep {
			sc.step("7120010.stp", []string{"#1=7120010", "#2=7120013"}, []string{"#1>#2"})
		}
		sc.disegno("7120010.pdf")
		c := sc.calcola()
		for _, k := range []string{"7120010", "7120011", "7120013"} {
			if v := c.Indice.Voci[k]; v != nil && contiene(v.Discordanze, DiscStrutturaDiversa) {
				t.Errorf("con lo STEP del commerciale %v: %s ha struttura_diversa", conStep, k)
			}
		}
		d := sc.dest(c, "7120010.pdf")
		if candidati(d) != "1 7120010 dest_nodo_diretto_nome" || !d.Preselezionabile || !contiene(d.Candidati[0].Sostegno, SostegnoStepAutorizzato) {
			t.Errorf("con lo STEP del commerciale %v: il disegno del commerciale %s, preselezionabile %v, %+v", conStep, candidati(d), d.Preselezionabile, d.Candidati)
		}
	}
	// i figli che lo STEP del prodotto mette sotto un pezzo che vi sta sotto un commerciale sono guida: non
	// discordano con lo STEP del pezzo. 7120011, deciso a mano sotto il prodotto, ha il suo STEP (7120011 →
	// 7120014); nello STEP del prodotto sta sotto il commerciale 7120010, con il figlio 7120013
	sc := nuovaScena(t)
	p := sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=7120010", "#3=7120011", "#4=7120013"}, []string{"#1>#2", "#2>#3", "#3>#4"})
	sc.arco(p, sc.componente("7120010", db.TipoComponenteCommerciale, db.OrigineComponenteManuale), 1)
	sc.arco(p, sc.componente("7120011", db.TipoComponenteSottoassieme, db.OrigineComponenteManuale), 1)
	sc.step("7120011.stp", []string{"#1=7120011", "#2=7120014"}, []string{"#1>#2"})
	c := sc.calcola()
	if sa, ok := c.Indice.Sottoassiemi["7120011"]; !ok || sa.SoloGuida {
		t.Fatalf("la scena: 7120011 ha il suo STEP come sorgente: %+v", sa)
	}
	for _, k := range []string{"7120011", "7120013", "7120014"} {
		if v := c.Indice.Voci[k]; v == nil || contiene(v.Discordanze, DiscStrutturaDiversa) {
			t.Errorf("i figli guida sotto il commerciale non discordano: %s %+v", k, v)
		}
	}
}

// Correzione di F8, secondo giro (Domanda 5 = B; A5.13.4): lo STEP di un pezzo che c'e' solo come guida
// sotto un commerciale si raggiunge, e non e' scollegato. 7120010 e' un commerciale sotto il prodotto; nella
// RFQ c'e' 7120011.stp (7120011 → 7120014), e 7120011 sta sotto 7120010 nello STEP del prodotto. Lo STEP e'
// solo guida: 7120014 e' guida sotto il commerciale, il suo disegno ha il candidato bloccato con «cambia prima
// il tipo di 7120010», e nessuna evidenza dice «non collegato» ne' «il file che ne proporrebbe i figli diretti».
func TestLoStepSottoUnCommercialeNonEScollegato(t *testing.T) {
	sc := nuovaScena(t)
	p := casoProdotto(sc, "7120001.stp")
	sc.arco(p, sc.componente("7120010", db.TipoComponenteCommerciale, db.OrigineComponenteManuale), 1)
	sc.step("7120011.stp", []string{"#1=7120011", "#2=7120014"}, []string{"#1>#2"})
	sc.disegno("7120014.pdf")
	c := sc.calcola()
	ix := c.Indice
	sha := shaDel("7120011.stp")
	if ix.Scollegati[sha] || ix.UsatoDa[sha] != "7120011" {
		t.Fatalf("lo STEP di 7120011: scollegato %v, usato da %q", ix.Scollegati[sha], ix.UsatoDa[sha])
	}
	if sa, ok := ix.Sottoassiemi["7120011"]; !ok || !sa.SoloGuida || !strings.Contains(strings.Join(sa.Motivi, " | "), "cambia prima il tipo di 7120010") {
		t.Errorf("l'ancora di 7120011, sotto il commerciale: %+v", sa)
	}
	if v := ix.Voci["7120011"]; v.SottoCommerciale != "7120010" || v.Commerciale {
		t.Errorf("la voce 7120011: %+v", v)
	}
	v := ix.Voci["7120014"]
	if v == nil || v.Autorita != AutoritaProfonda || len(v.Nodi) != 1 || v.Nodi[0].SottoCommerciale != "7120010" || v.Nodi[0].affidabile() {
		t.Fatalf("7120014, guida sotto il commerciale: %+v", v)
	}
	d := sc.dest(c, "7120014.pdf")
	if d.Esito != EsitoProposta || len(d.Candidati) == 0 {
		t.Fatalf("il disegno di 7120014: %s %s %v", d.Esito, d.Motivo, d.Evidenze)
	}
	e := strings.Join(append(append([]string(nil), d.Evidenze...), d.Candidati[0].Evidenze...), " | ")
	if candidati(d) != "1 7120014 dest_nodo_profondo [nessuna_autorita]" || d.Preselezionabile ||
		!strings.Contains(e, "cambia prima il tipo di 7120010") || strings.Contains(e, "non collegato") || strings.Contains(e, "autorizza prima lo STEP") {
		t.Errorf("il disegno di 7120014: %s, %s", candidati(d), e)
	}
	d = sc.dest(c, "7120011.stp")
	e = strings.Join(append(append([]string(nil), d.Evidenze...), d.Candidati[0].Evidenze...), " | ")
	if d.Esito != EsitoProposta || d.Motivo == MotivoRadiceLontana || d.Preselezionabile || d.Candidati[0].Regola != "dest_3d_sottoassieme" ||
		strings.Contains(e, "il file che ne proporrebbe i figli diretti") || !strings.Contains(e, "sta sotto 7120010, un commerciale; il suo STEP è solo guida") {
		t.Errorf("lo STEP di 7120011: %s %s %s, %s", d.Esito, d.Motivo, candidati(d), e)
	}
}

// Correzione di F8, terzo giro (U7, scelta 2, «bis»): lo stato «da rivedere» passa lungo il cammino. 7120030
// sta sotto il prodotto solo per un arco nato da un'evidenza, oppure e' esso stesso nato da un codice trovato;
// 7120020 e' aggiunto a mano sotto 7120001 e non e' nello STEP del prodotto; 7120030.stp dice 7120030 →
// 7120020. Il disegno 7120020.pdf col solo nome non e' preselezionabile: l'unica evidenza oltre al nome
// verrebbe da una struttura legata al prodotto da un pezzo che nessuno ha deciso, e l'evidenza lo dice. Lo
// stesso un anello piu' in basso (7120030 → 7120031 → 7120020, con lo STEP di 7120031). Con 7120030 e il suo
// arco decisi da una persona, lo stesso disegno si preseleziona, e l'evidenza dice quale STEP lo sostiene.
func TestIlSottoassiemeDaRivedereNonSostieneIlSuoStep(t *testing.T) {
	casi := []struct {
		nome             string
		componente, arco db.OrigineComponente
		sotto            bool // 7120020 sta un anello piu' in basso, sotto 7120031
		presel           bool
		evidenza         string
	}{
		{"arco da uno STEP non autorizzato", db.OrigineComponenteManuale, db.OrigineComponenteStep, false, false,
			"7120020 compare nello STEP 7120030.stp, che ancora 7120030, a sua volta da rivedere: non sostiene la destinazione"},
		{"codice trovato", db.OrigineComponenteCodiceRilevato, db.OrigineComponenteManuale, false, false,
			"7120020 compare nello STEP 7120030.stp, che ancora 7120030, a sua volta da rivedere: non sostiene la destinazione"},
		{"un anello piu' in basso", db.OrigineComponenteManuale, db.OrigineComponenteStep, true, false,
			"7120020 compare nello STEP 7120031.stp, che ancora 7120031, raggiunto solo attraverso 7120030, da rivedere: non sostiene la destinazione"},
		{"deciso", db.OrigineComponenteManuale, db.OrigineComponenteManuale, false, true,
			"7120020 è un nodo dello STEP 7120030.stp, che ancora 7120030: un'evidenza strutturale oltre al nome del file"},
	}
	for _, x := range casi {
		sc := nuovaScena(t)
		p := casoProdotto(sc, "7120001.stp")
		sc.arcoDa(p, sc.componente("7120030", db.TipoComponenteSottoassieme, x.componente), 1, x.arco)
		sc.arco(p, sc.componente("7120020", db.TipoComponenteSciolto, db.OrigineComponenteManuale), 1)
		if x.sotto {
			sc.step("7120030.stp", []string{"#1=7120030", "#2=7120031"}, []string{"#1>#2"})
			sc.step("7120031.stp", []string{"#1=7120031", "#2=7120020"}, []string{"#1>#2"})
		} else {
			sc.step("7120030.stp", []string{"#1=7120030", "#2=7120020"}, []string{"#1>#2"})
		}
		sc.disegno("7120020.pdf")
		c := sc.calcola()
		if w := c.Indice.Voci["7120030"]; (w.Autorita == AutoritaDecisa) != x.presel || w.Livello != LivelloPiena {
			t.Fatalf("%s: la scena, 7120030: %+v", x.nome, w)
		}
		v := c.Indice.Voci["7120020"]
		if v.Autorita != AutoritaDecisa || len(v.Nodi) != 1 || v.Nodi[0].Livello != LivelloPiena || v.Nodi[0].affidabile() != x.presel {
			t.Errorf("%s: 7120020, deciso sotto 7120001 e nodo dello STEP del sottoassieme: %+v", x.nome, v)
			continue
		}
		if t30 := v.Nodi[0].TramiteDaRivedere; (t30 == "7120030") == x.presel {
			t.Errorf("%s: il nodo di 7120020 dice il pezzo da rivedere %q", x.nome, t30)
		}
		d := sc.dest(c, "7120020.pdf")
		e := strings.Join(d.Candidati[0].Evidenze, " | ")
		if candidati(d) != "1 7120020 dest_componente_nome" || d.Preselezionabile != x.presel ||
			contiene(d.Candidati[0].Sostegno, SostegnoNodoAncora) != x.presel || !strings.Contains(e, x.evidenza) {
			t.Errorf("%s: il disegno col solo nome: %s, preselezionabile %v, %v, %s", x.nome, candidati(d), d.Preselezionabile, d.Candidati[0].Sostegno, e)
		}
	}
}

// Correzione di F8, terzo giro (FP7, U7): il punto fisso dei sottoassiemi non dipende dai nomi dei codici. Lo
// STEP del prodotto contiene il sottoassieme S, lo STEP di S contiene il pezzo X, lo STEP di X contiene
// 7120021; la variante da confermare del secondo prodotto contiene anche X. Tutto il cammino da 7120001 e'
// pieno: 7120021 e' pieno e il suo nodo sostiene una destinazione, con S = 7120030 e X = 7120020 come con i
// due codici scambiati. Prima una voce visitata presto (in ordine alfabetico) restava con il livello di allora.
func TestIlPuntoFissoNonDipendeDaiNomiDeiCodici(t *testing.T) {
	for _, nomi := range [][2]string{{"7120030", "7120020"}, {"7120020", "7120030"}} {
		sub, pezzo := nomi[0], nomi[1]
		sc := nuovaScena(t)
		sc.prodotto("7120001")
		sc.step("7120001.stp", []string{"#1=7120001", "#2=" + sub}, []string{"#1>#2"})
		sc.step(sub+".stp", []string{"#1=" + sub, "#2=" + pezzo}, []string{"#1>#2"})
		sc.prodotto("7120002")
		sc.step("variante.stp", []string{"#1=7120002A", "#2=" + pezzo}, []string{"#1>#2"})
		sc.step(pezzo+".stp", []string{"#1=" + pezzo, "#2=7120021"}, []string{"#1>#2"})
		ix := sc.calcola().Indice
		if w := ix.Voci[pezzo]; w.Livello != LivelloPiena {
			t.Errorf("S=%s X=%s: la voce di X: %+v", sub, pezzo, w)
		}
		v := ix.Voci["7120021"]
		if v == nil || v.Livello != LivelloPiena || len(v.Nodi) != 1 || v.Nodi[0].Livello != LivelloPiena || !v.Nodi[0].affidabile() ||
			v.Nodi[0].Ancora != pezzo || v.Nodi[0].Padre != pezzo {
			t.Errorf("S=%s X=%s: 7120021, sotto un cammino tutto pieno: %+v", sub, pezzo, v)
		}
	}
}

// Correzione di F8, terzo giro (Domanda 5 = B; A5.13.4): una voce che c'era solo come guida sotto un
// commerciale, e che poi un altro cammino raggiunge fuori dalla guida, usa il suo STEP come le altre voci.
// 7120011 e 7120013 stanno sotto il commerciale 7120010 nello STEP del prodotto; 7120013.stp (solo guida)
// porta 7120013 → 7120020 → 7120011, ed e' autorizzato per il componente 7120020 con la sorgente 7120020:
// 7120011 e' un figlio diretto nell'autorita' di quel file. Il suo STEP 7120011.stp non e' piu' solo guida:
// 7120014 e' un suo figlio diretto (da autorizzare), e nessuna evidenza dice insieme «cambia prima il tipo di
// 7120010» e «il file che ne proporrebbe i figli diretti».
func TestUnaVoceUscitaDallaGuidaUsaIlSuoStep(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=7120010", "#3=7120013", "#4=7120011"}, []string{"#1>#2", "#2>#3", "#2>#4"})
	sc.accetta("7120001.stp", "#2", sc.componente("7120010", db.TipoComponenteCommerciale, db.OrigineComponenteManuale))
	sc.step("7120011.stp", []string{"#1=7120011", "#2=7120014"}, []string{"#1>#2"})
	sc.step("7120013.stp", []string{"#1=7120013", "#2=7120020", "#3=7120011"}, []string{"#1>#2", "#2>#3"})
	sc.autorizza(sc.componente("7120020", db.TipoComponenteSottoassieme, db.OrigineComponenteManuale), "7120013.stp", "#2")
	sc.disegno("7120014.pdf")
	c := sc.calcola()
	ix := c.Indice
	if sa := ix.Sottoassiemi["7120013"]; !sa.SoloGuida {
		t.Fatalf("la scena: lo STEP di 7120013, sotto il commerciale, e' solo guida: %+v", sa)
	}
	if v := ix.Voci["7120011"]; v.SottoCommerciale != "" || v.Autorita != AutoritaDiretta || !v.Nodi[0].NellAutorita {
		t.Fatalf("la scena: 7120011 e' un figlio diretto nell'autorita' di 7120013.stp: %+v", v)
	}
	sa, ok := ix.Sottoassiemi["7120011"]
	if !ok || sa.SoloGuida || len(sa.Motivi) != 0 || ix.UsatoDa[shaDel("7120011.stp")] != "7120011" {
		t.Errorf("l'ancora di 7120011, uscita dalla guida: %+v", sa)
	}
	v := ix.Voci["7120014"]
	if v == nil || v.Autorita != AutoritaDiretta || len(v.Nodi) != 1 || v.Nodi[0].guida() || len(v.Posizioni) != 1 || v.Posizioni[0].Padre != "7120011" {
		t.Errorf("7120014, figlio diretto di 7120011 nel suo STEP: %+v", v)
	}
	d := sc.dest(c, "7120014.pdf")
	e := strings.Join(d.Candidati[0].Evidenze, " | ")
	if candidati(d) != "1 7120014 dest_nodo_non_autorizzato [step_non_autorizzato]" || d.Preselezionabile || strings.Contains(e, "cambia prima il tipo") {
		t.Errorf("il disegno di 7120014: %s, %s", candidati(d), e)
	}
	d = sc.dest(c, "7120011.stp")
	e = strings.Join(d.Candidati[0].Evidenze, " | ")
	if d.Candidati[0].Regola != "dest_3d_sottoassieme" || !strings.Contains(e, "il file che ne proporrebbe i figli diretti") || strings.Contains(e, "solo guida") {
		t.Errorf("lo STEP di 7120011: %s, %s", candidati(d), e)
	}
}

// Correzione di F8, terzo giro (A5.13.3-A5.13.4): una radice vicina non prende lo STEP di un'altra voce. Lo
// STEP del prodotto contiene 7120020 e 7120020A; nella RFQ c'e' 7120020A.stp. E' lo STEP di 7120020A (A-S1,
// piena), non una variante da confermare di 7120020, qualunque sia l'ordine delle visite: 7120021 sta sotto
// 7120020A. Lo stesso quando 7120020A c'e' solo come guida sotto un commerciale (il suo STEP e' solo guida), e
// quando 7120020A si raggiunge solo dopo 7120020, dallo STEP di un altro sottoassieme. Senza la voce 7120020A
// la radice vicina resta un'ancora da confermare di 7120020.
func TestLaRadiceVicinaNonPrendeLoStepDiUnAltraVoce(t *testing.T) {
	sc := nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=7120020", "#3=7120020A"}, []string{"#1>#2", "#1>#3"})
	sc.step("7120020A.stp", []string{"#1=7120020A", "#2=7120021"}, []string{"#1>#2"})
	ix := sc.calcola().Indice
	if _, ok := ix.Sottoassiemi["7120020"]; ok {
		t.Errorf("7120020 non ha uno STEP suo: %+v", ix.Sottoassiemi["7120020"])
	}
	if sa := ix.Sottoassiemi["7120020A"]; sa.Livello != LivelloPiena || len(sa.Portatori) != 1 || strings.Join(sa.Portatori[0].Regole, " ") != "A-S1" {
		t.Errorf("lo STEP di 7120020A: %+v", sa)
	}
	if v := ix.Voci["7120021"]; v.Nodi[0].Ancora != "7120020A" || v.Nodi[0].Padre != "7120020A" || v.Livello != LivelloPiena {
		t.Errorf("7120021 sotto 7120020A: %+v", v)
	}

	// 7120020A solo guida sotto il commerciale 7120010
	sc = nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=7120020", "#3=7120010", "#4=7120020A"}, []string{"#1>#2", "#1>#3", "#3>#4"})
	sc.accetta("7120001.stp", "#3", sc.componente("7120010", db.TipoComponenteCommerciale, db.OrigineComponenteManuale))
	sc.step("7120020A.stp", []string{"#1=7120020A", "#2=7120021"}, []string{"#1>#2"})
	ix = sc.calcola().Indice
	if _, ok := ix.Sottoassiemi["7120020"]; ok {
		t.Errorf("sotto il commerciale: 7120020 non ha uno STEP suo: %+v", ix.Sottoassiemi["7120020"])
	}
	if sa := ix.Sottoassiemi["7120020A"]; !sa.SoloGuida || ix.UsatoDa[shaDel("7120020A.stp")] != "7120020A" {
		t.Errorf("sotto il commerciale: lo STEP di 7120020A e' solo guida: %+v", sa)
	}
	if v := ix.Voci["7120021"]; v.Nodi[0].Ancora != "7120020A" || v.Nodi[0].SottoCommerciale != "7120010" {
		t.Errorf("sotto il commerciale: 7120021 e' guida sotto 7120020A: %+v", v)
	}

	// 7120020A si raggiunge solo dopo 7120020 (in ordine alfabetico), dallo STEP di 7120030: le radici vicine
	// si guardano solo quando le voci con una radice uguale sono ferme
	sc = nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=7120020", "#3=7120030"}, []string{"#1>#2", "#1>#3"})
	sc.step("7120030.stp", []string{"#1=7120030", "#2=7120020A"}, []string{"#1>#2"})
	sc.step("7120020A.stp", []string{"#1=7120020A", "#2=7120021"}, []string{"#1>#2"})
	ix = sc.calcola().Indice
	if _, ok := ix.Sottoassiemi["7120020"]; ok {
		t.Errorf("raggiunta dopo: 7120020 non ha uno STEP suo: %+v", ix.Sottoassiemi["7120020"])
	}
	if v := ix.Voci["7120021"]; v.Nodi[0].Ancora != "7120020A" || v.Livello != LivelloPiena {
		t.Errorf("raggiunta dopo: 7120021 sotto 7120020A: %+v", v)
	}

	// senza la voce 7120020A: la radice vicina e' un'ancora da confermare di 7120020
	sc = nuovaScena(t)
	sc.prodotto("7120001")
	sc.step("7120001.stp", []string{"#1=7120001", "#2=7120020"}, []string{"#1>#2"})
	sc.step("7120020A.stp", []string{"#1=7120020A", "#2=7120021"}, []string{"#1>#2"})
	ix = sc.calcola().Indice
	if sa := ix.Sottoassiemi["7120020"]; sa.Livello != LivelloDaConfermare || strings.Join(sa.Portatori[0].Regole, " ") != "A-S3" {
		t.Errorf("senza la voce 7120020A: %+v", sa)
	}
}

// Correzione di F8, quarto giro (U7, scelta 2, «bis»): l'autorizzazione di uno STEP non decide il pezzo che
// lo lega al prodotto. E' la scena di TestIlSottoassiemeDaRivedereNonSostieneIlSuoStep, con in piu'
// 7120030.stp autorizzato da una persona per il componente 7120030 (sorgente #1): 7120020 e' un figlio diretto
// nell'autorita' di quel file, ma 7120030 sta sotto il prodotto solo per un arco accettato da uno STEP non
// autorizzato. Il disegno 7120020.pdf non e' preselezionabile, non porta step_autorizzato, e nessuna evidenza
// dice insieme che il file sostiene e non sostiene la destinazione. Con l'arco deciso da una persona lo stesso
// disegno si preseleziona, sostenuto dallo STEP autorizzato. Prima usciva preselezionabile con
// [componente_deciso step_autorizzato] e l'evidenza «a sua volta da rivedere: non sostiene la destinazione».
func TestLoStepAutorizzatoDiUnPezzoDaRivedereNonSostiene(t *testing.T) {
	for _, x := range []struct {
		nome   string
		arco   db.OrigineComponente
		presel bool
	}{{"arco da uno STEP non autorizzato", db.OrigineComponenteStep, false}, {"arco deciso", db.OrigineComponenteManuale, true}} {
		sc := nuovaScena(t)
		p := casoProdotto(sc, "7120001.stp")
		s := sc.componente("7120030", db.TipoComponenteSottoassieme, db.OrigineComponenteManuale)
		sc.arcoDa(p, s, 1, x.arco)
		sc.arco(p, sc.componente("7120020", db.TipoComponenteSciolto, db.OrigineComponenteManuale), 1)
		sc.step("7120030.stp", []string{"#1=7120030", "#2=7120020"}, []string{"#1>#2"})
		sc.autorizza(s, "7120030.stp", "#1")
		sc.disegno("7120020.pdf")
		c := sc.calcola()
		v := c.Indice.Voci["7120020"]
		if len(v.Nodi) != 1 || !v.Nodi[0].NellAutorita || (v.Nodi[0].TramiteDaRivedere == "7120030") == x.presel {
			t.Fatalf("%s: la scena, 7120020 nell'autorita' di 7120030.stp: %+v", x.nome, v)
		}
		d := sc.dest(c, "7120020.pdf")
		top := d.Candidati[0]
		e := strings.Join(top.Evidenze, " | ")
		nonSostiene := strings.Contains(e, "non sostiene la destinazione")
		if candidati(d) != "1 7120020 dest_nodo_diretto_nome" || d.Preselezionabile != x.presel ||
			contiene(top.Sostegno, SostegnoStepAutorizzato) != x.presel || nonSostiene == x.presel ||
			!strings.Contains(e, "7120020 è un figlio diretto di 7120030 nello STEP autorizzato 7120030.stp") {
			t.Errorf("%s: il disegno col solo nome: %s, preselezionabile %v, %v, %s", x.nome, candidati(d), d.Preselezionabile, top.Sostegno, e)
		}
		if !x.presel && !strings.Contains(e, "7120020 compare nello STEP 7120030.stp, che ancora 7120030, a sua volta da rivedere: non sostiene la destinazione") {
			t.Errorf("%s: l'evidenza non dice perche' lo STEP non sostiene: %s", x.nome, e)
		}
	}
}

// Correzione di F8, quarto giro (FP7; A5.13.3-A5.13.4): anche con le radici vicine (A-S3) il punto fisso non
// dipende dai nomi dei codici. Lo STEP del prodotto contiene 7120020 e il sottoassieme S; l'unico STEP di S e'
// SA.stp, una radice vicina, che contiene 7120020A; nella RFQ c'e' 7120020A.stp → 7120021. 7120020A.stp e' lo
// STEP di 7120020A (A-S1), non una variante di 7120020, sia con S = 7120030 (visitato dopo 7120020) sia con
// S = 7120010 (visitato prima): 7120021 sta sotto 7120020A, e 7120020 non ha uno STEP suo. Prima, con S =
// 7120030, 7120020 prendeva 7120020A.stp come radice vicina prima che 7120020A nascesse, e lo teneva.
func TestLaRadiceVicinaNonDipendeDaiNomiDeiCodici(t *testing.T) {
	for _, sub := range []string{"7120030", "7120010"} {
		sc := nuovaScena(t)
		sc.prodotto("7120001")
		sc.step("7120001.stp", []string{"#1=7120001", "#2=7120020", "#3=" + sub}, []string{"#1>#2", "#1>#3"})
		sc.step(sub+"A.stp", []string{"#1=" + sub + "A", "#2=7120020A"}, []string{"#1>#2"})
		sc.step("7120020A.stp", []string{"#1=7120020A", "#2=7120021"}, []string{"#1>#2"})
		ix := sc.calcola().Indice
		if sa := ix.Sottoassiemi[sub]; sa.Livello != LivelloDaConfermare || len(sa.Portatori) != 1 || strings.Join(sa.Portatori[0].Regole, " ") != "A-S3" {
			t.Fatalf("S=%s: la scena, lo STEP di S e' una radice vicina: %+v", sub, sa)
		}
		if sa, ok := ix.Sottoassiemi["7120020"]; ok {
			t.Errorf("S=%s: 7120020 non ha uno STEP suo: %+v", sub, sa)
		}
		if sa := ix.Sottoassiemi["7120020A"]; len(sa.Portatori) != 1 || strings.Join(sa.Portatori[0].Regole, " ") != "A-S1" ||
			ix.UsatoDa[shaDel("7120020A.stp")] != "7120020A" {
			t.Errorf("S=%s: lo STEP di 7120020A: %+v", sub, sa)
		}
		v := ix.Voci["7120021"]
		if v == nil || len(v.Nodi) != 1 || v.Nodi[0].Ancora != "7120020A" || v.Nodi[0].Padre != "7120020A" ||
			len(v.Posizioni) != 1 || v.Posizioni[0].Padre != "7120020A" {
			t.Errorf("S=%s: 7120021 sotto 7120020A: %+v", sub, v)
		}
	}
}
