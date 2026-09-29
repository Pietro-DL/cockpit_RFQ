package fascicolo

// L1 — Giro 4, fase 4.6 nel flusso ancorato (F8): il cartiglio letto meglio, il bloccante dello scenario del 28/09
// («il PDF d'assieme prende il codice del primo figlio»). Nella zona del cartiglio vota soltanto il campo del
// codice: le righe dell'elenco particolari di un disegno d'assieme non sono letture ne' chiavi, e quindi nemmeno
// candidati; «SPECULARE DI X» e' una nota della destinazione, non un candidato verso l'altro pezzo. Il cartiglio di
// un testo letto con il worker di prima (la sottoversione 1) non e' contenuto finche' non si rianalizza (fase
// 4.6r). Dati inventati.

import (
	"fmt"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// Il disegno d'assieme ACME del prodotto 7120002 ha nella zona del cartiglio l'elenco particolari (dodici righe, i
// figli 7120020…7120031, che sono anche i figli diretti del suo STEP autorizzato), «SPECULARE DI 7120003» e il
// campo del codice «Part Nr:» 7120002, come lo riporta il worker della sottoversione 2. Prima le righe erano letture
// del cartiglio a 85, dal contenuto: ogni figlio era un candidato del disegno d'assieme, e il primo nell'ordine del
// file vinceva. Adesso la destinazione ha un solo codice letto, 7120002, e un solo candidato, il prodotto; nessun
// figlio; la nota «speculare di 7120003». Il disegno di un figlio con «SPECULARE DI 7120021» scritto nella pagina
// (dove prima era una chiave dell'indice, e 7120021 e' un figlio dello STEP) va al suo pezzo, 7120020, con la nota, e
// nessun candidato verso 7120021.
//
// Le controprove (a mano): con soloDelCodice che lascia passare ogni codice della zona del cartiglio, il disegno
// d'assieme ha i figli fra i candidati; senza separaSpeculari, il disegno del figlio ha il candidato 7120021.
func TestIlPdfDAssiemeNonPortaIFigliNelFlusso(t *testing.T) {
	sc := nuovaScena(t)
	sc.s.Motore = motoreACME712(t)
	p := sc.prodotto("7120002")
	nodi, archi := []string{"#1=7120002"}, []string{}
	var figli []string
	var righe strings.Builder
	righe.WriteString("Pos. Part Number Descrizione Q.ty\n")
	for i := 0; i < 12; i++ {
		f := fmt.Sprint(7120020 + i)
		figli = append(figli, f)
		nodi = append(nodi, fmt.Sprintf("#%d=%s", i+2, f))
		archi = append(archi, fmt.Sprintf("#1>#%d", i+2))
		fmt.Fprintf(&righe, "%d %s PEZZO ACME 1\n", i+1, f)
	}
	sc.step("7120002.stp", nodi, archi)
	sc.autorizza(p, "7120002.stp", "#1")
	righe.WriteString("SPECULARE DI 7120003\nFamily: Part Nr: Descrizione:\n-- 7120002 STAFFA ASSIEME ACME")
	assieme := conCampo(testoFinto(righe.String(), "", worker.MetadatiPDF{}), worker.CampoCodice, "Part Nr:", "7120002")
	sc.disegnoLetto("tavola assieme.pdf", fattiPDF(t, assieme))
	figlio := conCampo(testoFinto("Part Nr: 7120020", "NOTA: SPECULARE DI 7120021", worker.MetadatiPDF{}), worker.CampoCodice, "Part Nr:", "7120020")
	sc.disegnoLetto("tavola figlio.pdf", fattiPDF(t, figlio))
	c := sc.calcola()

	d := sc.dest(c, "tavola assieme.pdf")
	var codici []string
	for _, k := range d.Codici {
		codici = append(codici, k.Codice)
	}
	if strings.Join(codici, " ") != "7120002" || len(d.Candidati) != 1 || d.Candidati[0].Codice != "7120002" {
		t.Errorf("il disegno d'assieme: codici %v, candidati %s", codici, candidati(d))
	}
	for _, f := range figli {
		if strings.Contains(candidati(d), f) {
			t.Errorf("il disegno d'assieme ha il figlio %s fra i candidati: %s", f, candidati(d))
		}
	}
	if strings.Join(d.Note, " | ") != "speculare di 7120003" {
		t.Errorf("lo speculare del disegno d'assieme e' una nota: %v", d.Note)
	}
	if e := strings.Join(d.Evidenze, " | "); strings.Contains(e, "7120020") || strings.Contains(e, "7120003") {
		t.Errorf("le righe dell'elenco o lo speculare fra le evidenze del disegno d'assieme: %s", e)
	}

	df := sc.dest(c, "tavola figlio.pdf")
	if len(df.Candidati) == 0 || df.Candidati[0].Codice != "7120020" || strings.Contains(candidati(df), "7120021") {
		t.Errorf("il disegno del figlio va al suo pezzo, non allo speculare: %s", candidati(df))
	}
	if strings.Join(df.Note, " | ") != "speculare di 7120021" {
		t.Errorf("lo speculare del disegno del figlio e' una nota: %v", df.Note)
	}
	for _, k := range df.Codici {
		if k.Codice == "7120021" {
			t.Errorf("lo speculare e' un codice del file: %+v", df.Codici)
		}
	}
}

// assiemeACME e' il testo del disegno d'assieme ACME del prodotto 7120001 (i figli 7120010 e 7120012 nell'elenco
// particolari dentro la zona del cartiglio, poi il cartiglio con «Part Nr:» 7120001), come lo riporta il worker della
// sottoversione del testo data: la 1 prendeva l'intestazione dell'elenco («Part Number») per il campo del codice,
// con il valore della riga sotto, cioe' il primo figlio, e non conosceva «Part Nr:»; dalla 2 (fase 4.6) il campo del
// codice e' «Part Nr:».
func assiemeACME(versione int) worker.TestoPDF {
	testo := "Pos. Part Number Descrizione Q.ty\n1 7120010 SOTTOASSIEME ACME 1\n2 7120012 PEZZO ACME 2\n" +
		"Family: Part Nr: Descrizione:\n-- 7120001 ASSIEME ACME"
	if versione < 2 {
		tp := conCampo(testoFinto(testo, "", worker.MetadatiPDF{}), worker.CampoCodice, "Part Number", "7120010")
		tp.Versione = versione
		return tp
	}
	tp := conCampo(testoFinto(testo, "", worker.MetadatiPDF{}), worker.CampoCodice, "Part Nr:", "7120001")
	tp.Versione = versione
	return tp
}

// diPrima e' il testo come l'avrebbe riportato il worker della sottoversione 1 (prima della fase 4.6).
func diPrima(tp worker.TestoPDF) worker.TestoPDF {
	tp.Versione = 1
	return tp
}

// TestIlCartiglioLettoConIlWorkerDiPrimaNonPreseleziona (giro 4, fase 4.6r: il bug 3 della Distinta che rientrava dai
// PDF gia' letti dal worker installato sulle postazioni). Il prodotto ACME 7120001 ha lo STEP autorizzato, con i figli
// diretti 7120010 e 7120012 accettati come componenti decisi; il suo disegno d'assieme «tavola assieme.pdf» (un nome
// senza codice) e' stato letto dal worker di prima, che dava il campo del codice all'intestazione dell'elenco
// particolari, con il primo figlio come valore. Letto con la regola della 4.6 (vota il solo campo del codice) quel
// campo era l'unica lettura: «unica 85» sul figlio 7120010 nella valutazione, e nel flusso un candidato dal contenuto
// verso il figlio diretto dello STEP autorizzato, preselezionabile. Adesso, finche' il testo e' della sottoversione 1:
//   - la valutazione non ha un voto dal contenuto;
//   - nel flusso i codici della zona del cartiglio sono chiavi di ricerca (fonte testo_pdf): nessun candidato dal
//     contenuto, niente preselezionabile, e la destinazione dice «testo letto con il worker di prima: da
//     rianalizzare»;
//   - dopo «Rianalizza» (lo stesso disegno letto dal worker di oggi) il disegno va al prodotto 7120001 dal contenuto,
//     senza candidati verso i figli.
//
// Senza STEP il disegno «7120002.pdf» del prodotto 7120002 letto con il worker di prima ancora A-P3 (da confermare,
// con il motivo), non A-P1 come quello letto dal worker di oggi; e un disegno del prodotto il cui testo di prima non
// dice il prodotto lo dice con la frase, mentre la firma dell'indice cambia con la rianalisi anche se l'ancora resta
// assente (testiPdf).
//
// Le controprove (a mano): con soloDelCodice che tratta il testo di prima come quello di oggi, il disegno d'assieme
// ha «1 7120010 dest_nodo_diretto_contenuto», preselezionabile; con il cartiglio da rileggere contato come
// cartiglio in codiciDelFile, il candidato e' dal contenuto; in dicePdf, l'ancora e' A-P1; senza il segno in
// testiPdf la firma non cambia.
func TestIlCartiglioLettoConIlWorkerDiPrimaNonPreseleziona(t *testing.T) {
	costruisci := func(tp worker.TestoPDF) *scena {
		sc, _ := scenaPreselezionabile(t)
		sc.s.Motore = motoreACME712(t)
		sc.accetta("7120001.stp", "#3", sc.componente("7120012", db.TipoComponenteSciolto, db.OrigineComponenteManuale))
		sc.disegnoLetto("tavola assieme.pdf", fattiPDF(t, tp))
		return sc
	}
	sc := costruisci(assiemeACME(1))
	if v := sc.f("tavola assieme.pdf").Proposta.Valutazione.Codice; v.Valore != "" || v.Stato != classificazione.StatoNessuna {
		t.Errorf("la valutazione del disegno letto con il worker di prima ha un voto dal contenuto: %+v", v)
	}
	if tp := sc.f("tavola assieme.pdf").TestoPDF; !tp.DaRileggere || !tp.Letto() || tp.Frase != classificazione.FraseTestoDiPrima {
		t.Errorf("il testo di prima: %+v", tp)
	}
	d := sc.dest(sc.calcola(), "tavola assieme.pdf")
	for _, c := range d.Candidati {
		if c.DalContenuto || strings.Join(c.Fonti, " ") != FonteTestoPDF || contiene(c.Sostegno, SostegnoContenuto) {
			t.Errorf("un candidato dal cartiglio di prima e' piu' di una chiave: %+v", c)
		}
	}
	// i codici della zona, chiavi dell'indice: i due figli e il prodotto, dal nome della regola e mai scelti
	if d.Preselezionabile || d.Stato == "preselezionabile" ||
		candidati(d) != "1 7120010 dest_nodo_diretto_nome | 2 7120012 dest_nodo_diretto_nome | 3 7120001 dest_componente_nome" {
		t.Errorf("il disegno letto con il worker di prima: %s, preselezionabile %v", candidati(d), d.Preselezionabile)
	}
	for _, k := range d.Codici {
		if k.DalContenuto {
			t.Errorf("un codice del cartiglio di prima e' dal contenuto: %+v", k)
		}
	}
	if e := evidenzeIn(d); !strings.Contains(e, classificazione.FraseTestoDiPrima) ||
		!strings.Contains(e, "il cartiglio del PDF cita 7120010 (pagina 1), ma il testo è stato letto con il worker di prima") {
		t.Errorf("la destinazione dice il testo di prima: %s", e)
	}

	// dopo «Rianalizza»: il campo del codice del worker di oggi, il prodotto dal contenuto, nessun figlio
	sc = costruisci(assiemeACME(worker.VersioneTestoPDF))
	d = sc.dest(sc.calcola(), "tavola assieme.pdf")
	if candidati(d) != "1 7120001 dest_componente_contenuto" || !d.Candidati[0].DalContenuto || sc.f("tavola assieme.pdf").TestoPDF.DaRileggere ||
		strings.Contains(evidenzeIn(d), classificazione.FraseTestoDiPrima) {
		t.Errorf("il disegno riletto dal worker di oggi: %s %+v, %s", candidati(d), d.Candidati, evidenzeIn(d))
	}
	if v := sc.f("tavola assieme.pdf").Proposta.Valutazione.Codice; v.Valore != "7120001" || v.Score != 85 {
		t.Errorf("la valutazione del disegno riletto: %+v", v)
	}

	// senza STEP: il disegno del prodotto letto con il worker di prima e' un'ancora da confermare, non A-P1
	prodotto := func(tp worker.TestoPDF) Calcolo {
		sc := nuovaScena(t)
		sc.s.Motore = motoreACME712(t)
		sc.prodotto("7120002")
		sc.disegnoLetto("7120002.pdf", fattiPDF(t, tp))
		return sc.calcola()
	}
	regole := func(a Ancora) string {
		var out []string
		for _, p := range a.Portatori {
			out = append(out, strings.Join(p.Regole, "+")+":"+strings.Join(p.Discordanze, "+"))
		}
		return strings.Join(out, " ")
	}
	vecchia, nuova := prodotto(diPrima(disegnoN("7120002"))).Ancore["7120002"], prodotto(disegnoN("7120002")).Ancore["7120002"]
	if vecchia.Livello != LivelloPiatta || regole(vecchia) != "A-P3:"+DiscAncoraDaConfermare ||
		!strings.Contains(strings.Join(vecchia.Motivi, " | "), "il testo di 7120002.pdf cita 7120002, ma è stato letto con il worker di prima (A-P3)") {
		t.Errorf("l'ancora dal disegno letto con il worker di prima: %s %s %v", vecchia.Livello, regole(vecchia), vecchia.Motivi)
	}
	if nuova.Livello != LivelloPiatta || regole(nuova) != "A-P1+A-N1:" {
		t.Errorf("l'ancora dal disegno letto dal worker di oggi: %s %s", nuova.Livello, regole(nuova))
	}

	// un disegno del prodotto il cui testo non dice il prodotto: l'ancora e' assente prima e dopo, il motivo dice il
	// testo di prima, e la firma dell'indice cambia con la rianalisi
	muto := testoFinto("SCALA 1:1", "", worker.MetadatiPDF{})
	cv, cn := prodotto(diPrima(muto)), prodotto(muto)
	if cv.Ancore["7120002"].Livello != LivelloAssente || cn.Ancore["7120002"].Livello != LivelloAssente {
		t.Errorf("il testo che non dice il prodotto non ancora: %s, %s", cv.Ancore["7120002"].Livello, cn.Ancore["7120002"].Livello)
	}
	if m := strings.Join(cv.Ancore["7120002"].Motivi, " | "); !strings.Contains(m, "il testo di 7120002.pdf non dice 7120002 ("+classificazione.FraseTestoDiPrima+")") {
		t.Errorf("il motivo dice il testo di prima: %s", m)
	}
	if m := strings.Join(cn.Ancore["7120002"].Motivi, " | "); strings.Contains(m, classificazione.FraseTestoDiPrima) {
		t.Errorf("il testo di oggi non e' «di prima»: %s", m)
	}
	if cv.Indice.Firma == cn.Indice.Firma {
		t.Error("la firma dell'indice non cambia con la rianalisi del testo di prima")
	}
}

// TestIlCartiglioDiPrimaDiversoNonPreseleziona (giro 4, fase 4.6r2; lo scenario del 28/09, il PDF d'assieme letto dal
// worker vero; U7, prova 170): nel flusso la dimensione del codice discorde e' la discordanza «fonti diverse». Il
// cartiglio di un testo letto con il worker di prima non vota, ma se dice un codice diverso da quello del nome la
// valutazione e' discorde (classificazione.Componi): nessuna preselezione nuova, e quella dal nome si ferma finche'
// non si rianalizza. Lo stesso codice non cambia niente.
//   - il disegno d'assieme ACME «ACME-030P7120001.pdf» (un nome con un codice generico, che non e' quello del
//     cartiglio) letto con il worker di prima: valutazione discorde dal nome, destinazione con «fonti diverse», niente
//     di preselezionabile e nessun candidato dal contenuto;
//   - il disegno «7120010.pdf» del figlio accettato (la scena preselezionabile della prova 170) con il testo di prima
//     che nella zona del cartiglio dice anche 7120099 (un codice che l'indice non conosce: nessun secondo candidato):
//     «fonti diverse», e la preselezione dal nome si ferma;
//   - lo stesso disegno con il testo di prima che dice solo 7120010: preselezionabile dal nome, come senza la regola.
//
// La controprova (a mano): senza il caso contraddiceDiPrima in classificazione.Componi i primi due casi non hanno
// «fonti diverse», e il secondo e' preselezionabile dal nome.
func TestIlCartiglioDiPrimaDiversoNonPreseleziona(t *testing.T) {
	sc, _ := scenaPreselezionabile(t)
	sc.s.Motore = motoreACME712(t)
	sc.accetta("7120001.stp", "#3", sc.componente("7120012", db.TipoComponenteSciolto, db.OrigineComponenteManuale))
	sc.disegnoLetto("ACME-030P7120001.pdf", fattiPDF(t, assiemeACME(1)))
	if v := sc.f("ACME-030P7120001.pdf").Proposta.Valutazione.Codice; v.Valore != "ACME-030P7120001" || v.Regola != "nome_codice_generico" ||
		v.Stato != classificazione.StatoDiscorde {
		t.Errorf("la valutazione del disegno d'assieme letto con il worker di prima: %+v", v)
	}
	d := sc.dest(sc.calcola(), "ACME-030P7120001.pdf")
	if d.Preselezionabile || d.Stato == "preselezionabile" || !contiene(d.Discordanze, DiscFontiDiverse) {
		t.Errorf("il disegno d'assieme letto con il worker di prima: %s, preselezionabile %v, discordanze %v", candidati(d), d.Preselezionabile, d.Discordanze)
	}
	for _, c := range d.Candidati {
		if c.DalContenuto || contiene(c.Sostegno, SostegnoContenuto) {
			t.Errorf("un candidato dal cartiglio di prima: %+v", c)
		}
	}

	// il disegno del figlio accettato, letto con il worker di prima
	figlio := func(zona string) Destinazione {
		sc := nuovaScena(t)
		sc.s.Motore = motoreACME712(t)
		p := casoProdotto(sc, "7120001.stp")
		sc.autorizza(p, "7120001.stp", "#1")
		sc.accetta("7120001.stp", "#2", sc.componente("7120010", db.TipoComponenteSottoassieme, db.OrigineComponenteManuale))
		sc.disegnoLetto("7120010.pdf", fattiPDF(t, diPrima(conCampo(testoFinto(zona, "", worker.MetadatiPDF{}), worker.CampoNumeroDisegno,
			"DISEGNO N.", "7120010"))))
		return sc.dest(sc.calcola(), "7120010.pdf")
	}
	if d := figlio("DISEGNO N. 7120010\n1 7120099 PEZZO ACME 2\nSCALA 1:1"); d.Preselezionabile || !contiene(d.Discordanze, DiscFontiDiverse) ||
		candidati(d) != "1 7120010 dest_nodo_diretto_nome" {
		t.Errorf("il disegno del figlio con un altro codice nel cartiglio di prima: %s, preselezionabile %v, discordanze %v", candidati(d),
			d.Preselezionabile, d.Discordanze)
	}
	if d := figlio("DISEGNO N. 7120010\nSCALA 1:1"); !d.Preselezionabile || contiene(d.Discordanze, DiscFontiDiverse) ||
		candidati(d) != "1 7120010 dest_nodo_diretto_nome" {
		t.Errorf("il disegno del figlio con lo stesso codice nel cartiglio di prima: %s, preselezionabile %v, discordanze %v", candidati(d),
			d.Preselezionabile, d.Discordanze)
	}
}
