// L1 — i nodi della BOM come li vedrà la UI (B6, V3; R91, T-B0-39; contratto §1.3, §2.3, §2.5; T-E1-19; T-B6-11; PO-20
// nella parte di B6): solo i nodi della BOM di lavoro, con la parentela, i 2D (del componente deciso o candidati del
// nodo), l'origine dell'associazione del primario, i motivi dei file del nodo, la classificazione, gli altri prodotti con
// lo stesso componente; la raggiungibilità senza gli archi tolti da una persona.
package valutazione_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

// I clienti di questi test sono inventati (ACME, acme.example; codici 712xxxx; UUID 00000000-0000-4000-8000-0000000000nn):
// vedi scena_prodotti_test.go.

// Gli ID delle prove dei nodi: i file candidati dei nodi senza decisione, il messaggio in uscita.
var (
	fPDF3     = uid(0xf61)
	fTIFF4    = uid(0xf62)
	fUscita   = uid(0xf63)
	mUscitaV3 = uid(0xf64)
	fScartato = uid(0xf65)
)

// fattiStepConDescrizione: i fatti di uno STEP come fattiStepF, con la descrizione del PRODUCT (descrizione_grezza) dei
// nodi dati.
func fattiStepConDescrizione(t *testing.T, sha string, nodi []nodoF, archi []arcoF, descrizioni map[string]string) fotorfq.Fatti {
	t.Helper()
	n := []map[string]any{}
	for _, x := range nodi {
		n = append(n, map[string]any{"chiave": x.chiave, "id_grezzo": x.id, "nome_grezzo": "", "descrizione_grezza": descrizioni[x.chiave],
			"rev_grezza": x.rev, "evidenza": map[string]any{}})
	}
	r := []map[string]any{}
	for _, a := range archi {
		r = append(r, map[string]any{"padre": a.padre, "figlio": a.figlio, "qta": a.qta, "evidenza": map[string]any{}})
	}
	raw, err := json.Marshal(map[string]any{"struttura": map[string]any{
		"versione": 3, "schema": "AP214", "radici": []string{nodi[0].chiave}, "avvisi": []string{}, "nodi": n, "relazioni": r,
		"limiti": map[string]any{"troncato": false},
		"scarti": map[string]any{"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return fatti(t, sha, string(raw), testo(""))
}

// rifNodi: i Rif dei nodi del prodotto, in ordine.
func rifNodi(pv valutazione.ProdottoValutato) []string {
	var out []string
	for _, n := range pv.Nodi {
		out = append(out, n.Nodo.Rif)
	}
	return out
}

// motiviNodo: i motivi di un nodo come testo, per i confronti.
func motiviNodo(n valutazione.NodoBOM) string {
	var out []string
	for _, m := range n.Motivi {
		out = append(out, string(m))
	}
	return strings.Join(out, " ")
}

// TestNodiSoloDellaBOMDiLavoro (T-B6-11; R91, R76 A; contratto §1.3): il prodotto con la BOM di lavoro ha i suoi nodi, la
// radice prima, ognuno con la parentela, i 2D del componente deciso, l'origine del primario e la classificazione; il
// secondo prodotto, con la sola struttura candidata, non ha nodi, e nemmeno il prodotto senza la fonte confermata.
func TestNodiSoloDellaBOMDiLavoro(t *testing.T) {
	th := scenaSmistamento(t)
	secondoProdotto(t, &th)
	et := esitoSmistamento(t, th)
	p := prodottoDi(t, et, rifProdB)
	if !reflect.DeepEqual(rifNodi(p), []string{nodoB("#1"), nodoB("#2")}) {
		t.Fatalf("nodi %v", rifNodi(p))
	}
	radice, figlio := p.Nodi[0], p.Nodi[1]
	conf := ancoraggio.OrigineConfermato
	if len(radice.Parentela) != 0 || radice.Disegni.Primario == nil || radice.Disegni.Primario.DocumentoID == nil || *radice.Disegni.Primario.DocumentoID != dPDF1 ||
		radice.Associazione == nil || *radice.Associazione != conf || radice.Classificazione.Ruolo != valutazione.RuoloProdotto ||
		radice.Classificazione.Categoria != valutazione.CategoriaFabbricato || len(radice.AncheIn) != 0 || len(radice.Motivi) != 0 || radice.Descrizione != nil {
		t.Errorf("la radice %+v", radice)
	}
	q := 2
	if !reflect.DeepEqual(figlio.Parentela, []valutazione.Parentela{{Padre: nodoB("#1"), Quantita: &q, Decisa: true}}) || figlio.Disegni.Primario == nil ||
		*figlio.Disegni.Primario.DocumentoID != dPDF2 || *figlio.Associazione != conf || figlio.Classificazione.Ruolo != valutazione.RuoloComponente ||
		figlio.Nodo.Decisione == nil || figlio.Nodo.Decisione.ComponenteID != cSciolto {
		t.Errorf("il figlio %+v", figlio)
	}
	if q := prodottoDi(t, et, rifP2); len(q.Nodi) != 0 || q.Struttura != ancoraggio.StatoStrutturaCandidata {
		t.Errorf("il secondo prodotto, con la struttura candidata: %d nodi (T-B6-11)", len(q.Nodi))
	}
	senza := scenaSmistamento(t)
	senzaFonte(&senza)
	if p := prodottoDi(t, esitoSmistamento(t, senza), rifProdB); len(p.Nodi) != 0 {
		t.Errorf("senza la fonte confermata: nodi %v", rifNodi(p))
	}
}

// TestPO20IlNodoSenzaDecisione (PO-20 nella parte di B6; R91, R85; LD-03): i nodi senza decisione della BOM di lavoro
// mostrano i 2D candidati (gruppoDelNodo): il PDF che si apre ha l'anteprima disponibile, il TIFF no, e la proposta di
// associazione resta anche senza anteprima; l'origine del primario è proposto; la classificazione non è determinata (nessun
// tipo deciso); i motivi dicono la pre-associazione da confermare; l'identità parziale del nodo resta visibile.
func TestPO20IlNodoSenzaDecisione(t *testing.T) {
	th := scenaTreNodi(t, true)
	f := fattiPDF(t, shaN(340), "7120300A1")
	fileNelMessaggio(&th, fPDF3, idM1, "7120300A_1.pdf", "pdf", shaN(340), &f)
	conProposta(&th, uid(0xf66), fPDF3, "disegno_2d", "nome_file", "aperta", nil, nil)
	fileNelMessaggio(&th, fTIFF4, idM1, "7120310A_1.tif", "tif", shaN(341), nil)
	conProposta(&th, uid(0xf67), fTIFF4, "disegno_2d", "estensione", "aperta", nil, nil)
	et := esitoSmistamento(t, th)
	p := prodottoDi(t, et, rifProdB)
	if !reflect.DeepEqual(rifNodi(p), []string{nodoB("#1"), nodoB("#2"), nodoB("#3"), nodoB("#4")}) {
		t.Fatalf("nodi %v", rifNodi(p))
	}
	proposto := ancoraggio.OrigineProposto
	tre, quattro := nodoBOMDi(t, p, nodoB("#3")), nodoBOMDi(t, p, nodoB("#4"))
	uno := 1
	for _, c := range []struct {
		nome        string
		n           valutazione.NodoBOM
		allegato    string
		padre       string
		disponibile bool
	}{{"#3, il PDF", tre, fPDF3.String(), nodoB("#1"), true}, {"#4, il TIFF", quattro, fTIFF4.String(), nodoB("#3"), false}} {
		pr := c.n.Disegni.Primario
		if pr == nil || pr.AllegatoID == nil || pr.AllegatoID.String() != c.allegato || pr.Provenienza != proposto || pr.Anteprima.Disponibile != c.disponibile {
			t.Errorf("%s: il primario %+v", c.nome, pr)
			continue
		}
		if c.n.Associazione == nil || *c.n.Associazione != proposto || c.n.Nodo.Decisione != nil || len(c.n.AncheIn) != 0 ||
			c.n.Classificazione.Categoria != valutazione.CategoriaNonDeterminata || c.n.Classificazione.Origine != valutazione.OrigineCategoriaProposta ||
			c.n.Classificazione.Ruolo != valutazione.RuoloComponente || motiviNodo(c.n) != "associazione_non_confermata" ||
			!reflect.DeepEqual(c.n.Parentela, []valutazione.Parentela{{Padre: c.padre, Quantita: &uno}}) {
			t.Errorf("%s: %+v", c.nome, c.n)
		}
	}
	if a := associazioneDi(t, et, fTIFF4); a.Associazione != ancoraggio.AssociazioneCandidatoUnico || len(a.Proposta) != 1 {
		t.Errorf("la proposta del TIFF senza anteprima: %+v", a)
	}
	if id := tre.Nodo.Codice.Identita; !id.Parziale || id.Motivo == "" {
		t.Errorf("l'identità del nodo %+v: l'incertezza resta visibile", id)
	}
	if n := nodoBOMDi(t, p, nodoB("#2")); n.Classificazione.Categoria != valutazione.CategoriaFabbricato || n.Nodo.Decisione == nil {
		t.Errorf("il nodo deciso %+v", n)
	}
}

// TestAncheInIlComponenteCondiviso (T-E1-19): lo sciolto sta anche nella BOM confermata del secondo prodotto; il suo nodo
// nella BOM di lavoro del primo lo dice (AncheIn), la radice no.
func TestAncheInIlComponenteCondiviso(t *testing.T) {
	th := scenaSmistamento(t)
	secondoProdotto(t, &th, "7120200A")
	th.Relazioni = append(th.Relazioni, fotorfq.Relazione{PadreID: cP2, FiglioID: cSciolto, Qta: 1, Origine: "step", ConfermatoDa: operatore, CreatoIl: dataACME})
	p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
	if n := nodoBOMDi(t, p, nodoB("#2")); !reflect.DeepEqual(n.AncheIn, []string{rifP2}) {
		t.Errorf("AncheIn dello sciolto %v", n.AncheIn)
	}
	if n := nodoBOMDi(t, p, nodoB("#1")); len(n.AncheIn) != 0 {
		t.Errorf("AncheIn della radice %v", n.AncheIn)
	}
	// Un terzo prodotto con un codice che viene prima nell'ordine dei prodotti e un Rif che viene dopo: AncheIn è in ordine
	// di Rif, non dei prodotti.
	cP3 := uid(0xf81)
	th.Componenti = append(th.Componenti, componente(cP3, "7120050A", "finito", "manuale"))
	th.Relazioni = append(th.Relazioni, fotorfq.Relazione{PadreID: cP3, FiglioID: cSciolto, Qta: 1, Origine: "step", ConfermatoDa: operatore, CreatoIl: dataACME})
	p = prodottoDi(t, esitoSmistamento(t, th), rifProdB)
	if n := nodoBOMDi(t, p, nodoB("#2")); !reflect.DeepEqual(n.AncheIn, []string{rifP2, "componente:" + cP3.String()}) {
		t.Errorf("AncheIn con tre prodotti %v", n.AncheIn)
	}
}

// TestLaClassificazioneDelNodoSenzaDecisione (T-E1R-01; R103 C): il nodo senza decisione letto dalla famiglia della
// minuteria ha la minuteria solo proposta (Proposta), con la categoria non determinata: una proposta non esenta mai, e
// l'esenzione la dice solo EsenteDal2D.
func TestLaClassificazioneDelNodoSenzaDecisione(t *testing.T) {
	th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7129001", ""}}, []arcoF{{"#1", "#2", 2}, {"#1", "#3", 4}})
	confermaLAlbero(t, &th, "#2", cSciolto, "7120200A", 2)
	th.Fabbisogni = fabbisogniDefault()
	r := insiemeDi(t, voceRegole{cliente: clienteACME, file: "acme.v1.json", byte: grammaticaDi(t, clienteACME, "ACME S.p.A.", famigliaCatena(), famigliaMinuteria())})
	p := prodottoDi(t, esitoSmistamentoCon(t, th, r), rifProdB)
	c := nodoBOMDi(t, p, nodoB("#3")).Classificazione
	if c.Proposta != valutazione.CategoriaMinuteria || c.Categoria != valutazione.CategoriaNonDeterminata || c.Confermata || valutazione.EsenteDal2D(c) {
		t.Errorf("la classificazione del nodo della minuteria %+v", c)
	}
}

// TestINodiSenzaGliArchiTolti (T-B5-67): i nodi che la radice raggiunge solo attraverso un arco tolto da una persona (la
// riga di relazione_proposta scartata da chi l'ha decisa) escono dalla BOM di lavoro; un nodo scartato ma ancora raggiunto
// resta, con la sua riga decisa accanto, e così la sua parentela.
func TestINodiSenzaGliArchiTolti(t *testing.T) {
	op := operatore
	th := scenaTreNodi(t, true)
	a := arcoDi(t, &th, "#1", "#3")
	a.Stato, a.DecisoDa = "scartata", &op
	if p := prodottoDi(t, esitoSmistamento(t, th), rifProdB); !reflect.DeepEqual(rifNodi(p), []string{nodoB("#1"), nodoB("#2")}) {
		t.Errorf("con l'arco tolto: nodi %v", rifNodi(p))
	}
	// Il figlio condiviso (#4 sotto #2 e sotto #3): con l'arco #3→#4 tolto resta il solo padre #2; con l'arco #1→#3 tolto
	// #3 esce dalla BOM, e #4 resta sotto #2, senza il padre che non è raggiunto.
	rombo := func() fotorfq.Thread {
		th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}, {"#4", "7120310A", ""}},
			[]arcoF{{"#1", "#2", 1}, {"#1", "#3", 1}, {"#2", "#4", 1}, {"#3", "#4", 1}})
		th.Fabbisogni = fabbisogniDefault()
		return th
	}
	for _, tolto := range [][2]string{{"#3", "#4"}, {"#1", "#3"}} {
		th := rombo()
		a := arcoDi(t, &th, tolto[0], tolto[1])
		a.Stato, a.DecisoDa = "scartata", &op
		p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
		if n := nodoBOMDi(t, p, nodoB("#4")); len(n.Parentela) != 1 || n.Parentela[0].Padre != nodoB("#2") {
			t.Errorf("con l'arco %s→%s tolto: la parentela del figlio condiviso %+v", tolto[0], tolto[1], n.Parentela)
		}
	}
	th = scenaTreNodi(t, true)
	r := rigaDi(t, &th, "#4")
	r.Stato, r.DecisoDa = "scartata", &op
	p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
	n := nodoBOMDi(t, p, nodoB("#4"))
	if n.Nodo.RigaDecisa == nil || n.Nodo.RigaDecisa.Stato != "scartata" || len(n.Parentela) != 1 || n.Parentela[0].Padre != nodoB("#3") {
		t.Errorf("il nodo scartato e raggiunto %+v", n)
	}
}

// TestLaDescrizioneDelNodo: la descrizione del componente deciso; senza, quella del PRODUCT dello STEP del nodo; una
// descrizione vuota non c'è.
func TestLaDescrizioneDelNodo(t *testing.T) {
	th := scenaTreNodi(t, true)
	nodi := []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}, {"#4", "7120310A", ""}}
	archi := []arcoF{{"#1", "#2", 2}, {"#1", "#3", 1}, {"#3", "#4", 1}}
	th.Fatti[shaStepB] = fattiStepConDescrizione(t, shaStepB, nodi, archi, map[string]string{"#1": "Assieme ACME", "#2": "Staffa dallo STEP", "#4": "   "})
	for i := range th.Componenti {
		switch th.Componenti[i].ID {
		case cSciolto:
			th.Componenti[i].Descrizione = testo("Staffa ACME")
		case cProdotto:
			th.Componenti[i].Descrizione = testo("  ")
		}
	}
	p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
	for rif, attesa := range map[string]string{nodoB("#1"): "Assieme ACME", nodoB("#2"): "Staffa ACME", nodoB("#3"): "", nodoB("#4"): ""} {
		d := ""
		if x := nodoBOMDi(t, p, rif).Descrizione; x != nil {
			d = *x
		}
		if d != attesa {
			t.Errorf("%s: descrizione %q, attesa %q", rif, d, attesa)
		}
	}
	if x := nodoBOMDi(t, p, nodoB("#4")).Descrizione; x != nil {
		t.Errorf("una descrizione di soli spazi: %q", *x)
	}
}

// TestIMotiviDelNodoSullaRegola (contratto §2.3): i motivi di un nodo dagli ingressi sintetici: un file candidato con una
// posizione sul nodo; un conflitto nuovo_file sul componente del nodo deciso, il cui file non ha una posizione sul nodo
// (per esempio una posizione su un altro STEP del prodotto); un conflitto di un altro prodotto, che non conta; un file con
// la posizione sul nodo di un altro prodotto, che non conta.
func TestIMotiviDelNodoSullaRegola(t *testing.T) {
	k := cSciolto
	n := ancoraggio.NodoProposto{Rif: nodoB("#2")}
	fA, fB, fC := uid(0xf91), uid(0xf92), uid(0xf93)
	ancoraggi := []ancoraggio.AncoraggioFile{
		{AllegatoID: fA, Candidati: []ancoraggio.CandidatoAncoraggio{{Posizioni: []ancoraggio.PosizioneCandidato{{Target: rifProdB, Nodo: nodoB("#2")}}}}},
		{AllegatoID: fB, Candidati: []ancoraggio.CandidatoAncoraggio{{Posizioni: []ancoraggio.PosizioneCandidato{{Target: rifProdB, Nodo: nodoB("#9")}}}}},
		{AllegatoID: fC, Candidati: []ancoraggio.CandidatoAncoraggio{{Posizioni: []ancoraggio.PosizioneCandidato{{Target: rifP2, Nodo: nodoB("#2")}}}}},
	}
	associazioni := []valutazione.AssociazioneFile{
		{AllegatoID: fA, Associazione: ancoraggio.AssociazioneCandidatoUnico, Perimetro: valutazione.PerimetroDentro},
		{AllegatoID: fB, Associazione: ancoraggio.AssociazioneCandidatoUnico, Perimetro: valutazione.PerimetroDentro},
		{AllegatoID: fC, Associazione: ancoraggio.AssociazioneAmbiguo, Perimetro: valutazione.PerimetroDentro},
	}
	nuovoFile := valutazione.Conflitto{Tipo: valutazione.ConflittoNuovoFile, Asse: valutazione.AsseSmistamento, Rif: ancoraggio.RifComponente(k),
		Prodotto: rifProdB, EvidenzaProposta: valutazione.EvidenzaProposta{AllegatoID: &fB}}
	altroProdotto := valutazione.Conflitto{Tipo: valutazione.ConflittoAssociazione, Asse: valutazione.AsseSmistamento, Rif: "documento:x",
		Prodotto: rifP2, EvidenzaProposta: valutazione.EvidenzaProposta{AllegatoID: &fA}}
	testoDi := func(m []valutazione.MotivoSmistamento) string {
		var out []string
		for _, x := range m {
			out = append(out, string(x))
		}
		return strings.Join(out, " ")
	}
	if got := testoDi(valutazione.MotiviDelNodoPerProva(rifProdB, n, &k, ancoraggi, associazioni, []valutazione.Conflitto{nuovoFile, altroProdotto})); got !=
		"associazione_non_confermata nuovo_file_su_componente_deciso" {
		t.Errorf("il nodo deciso: motivi %q", got)
	}
	if got := testoDi(valutazione.MotiviDelNodoPerProva(rifProdB, n, nil, ancoraggi, associazioni, []valutazione.Conflitto{nuovoFile})); got !=
		"associazione_non_confermata" {
		t.Errorf("il nodo senza decisione: motivi %q (il nuovo_file è di un componente)", got)
	}
}

// TestIMotiviDelNodo (contratto §2.3: «ambiguità e conflitti dei file candidati del nodo»): il conflitto di associazione
// sul componente del nodo deciso (PO-04), un file ambiguo con un candidato sul nodo (PO-13); un file fuori perimetro
// (in un messaggio in uscita) e un file scartato con il gesto non danno motivi.
func TestIMotiviDelNodo(t *testing.T) {
	t.Run("il conflitto sul componente del nodo (PO-04)", func(t *testing.T) {
		th := scenaSmistamento(t)
		for i := range th.Allegati {
			if th.Allegati[i].ID == aPDF2 {
				th.Allegati[i].NomeFile = "7120100A_1.pdf"
			}
		}
		p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
		if n := nodoBOMDi(t, p, nodoB("#2")); motiviNodo(n) != "associazione_in_conflitto" {
			t.Errorf("il nodo dello sciolto: motivi %q", motiviNodo(n))
		}
	})
	t.Run("un file con l'«assegna» sul componente del nodo, senza candidati", func(t *testing.T) {
		th := scenaSmistamento(t)
		fileNelMessaggio(&th, uid(0xf69), idM1, "allegato.pdf", "pdf", shaN(344), scansione(t, shaN(344)))
		c := cSciolto
		conProposta(&th, uid(0xf6a), uid(0xf69), "disegno_2d", "estensione", "aperta", &c, nil)
		p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
		if n := nodoBOMDi(t, p, nodoB("#2")); motiviNodo(n) != "nessun_candidato collocazione_non_determinabile associazione_non_confermata" {
			t.Errorf("il nodo dello sciolto: motivi %q", motiviNodo(n))
		}
	})
	t.Run("un file ambiguo (PO-13)", func(t *testing.T) {
		th := scenaSmistamento(t)
		th.Identificativi = []fotorfq.Identificativo{confermato("7120200A")}
		fileNelMessaggio(&th, fS, idM1, "7120200A_1.pdf", "pdf", shaN(51), scansione(t, shaN(51)))
		p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
		if n := nodoBOMDi(t, p, nodoB("#2")); !strings.Contains(motiviNodo(n), "associazione_ambigua") {
			t.Errorf("il nodo dello sciolto: motivi %q", motiviNodo(n))
		}
	})
	t.Run("fuori perimetro e scartato: niente", func(t *testing.T) {
		th := scenaSmistamento(t)
		m := conMessaggio(&th, mUscitaV3, 20, "Offerta ACME26-030", "In allegato il disegno.", true)
		m.Direzione = "uscita"
		fileNelMessaggio(&th, fUscita, mUscitaV3, "7120200A_1.pdf", "pdf", shaN(342), scansione(t, shaN(342)))
		fileNelMessaggio(&th, fScartato, idM1, "7120200A_2.pdf", "pdf", shaN(343), scansione(t, shaN(343)))
		op := operatore
		conProposta(&th, uid(0xf68), fScartato, "disegno_2d", "nome_file", "scartata", nil, &op)
		et := esitoSmistamento(t, th)
		if a := associazioneDi(t, et, fUscita); a.Perimetro != valutazione.PerimetroMessaggioInUscita || len(a.Proposta) == 0 {
			t.Fatalf("il file in uscita %+v", a)
		}
		if a := associazioneDi(t, et, fScartato); !a.Terminale || len(a.Proposta) == 0 {
			t.Fatalf("il file scartato %+v", a)
		}
		p := prodottoDi(t, et, rifProdB)
		if n := nodoBOMDi(t, p, nodoB("#2")); len(n.Motivi) != 0 {
			t.Errorf("il nodo dello sciolto: motivi %q", motiviNodo(n))
		}
		if statoProdotto(p) != "sì/pronto_fattibilita/" {
			t.Errorf("il prodotto: %s", statoProdotto(p))
		}
	})
}
