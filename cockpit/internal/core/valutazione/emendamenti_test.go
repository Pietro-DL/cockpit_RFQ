// L1 — gli emendamenti EB7 (P7e; ratificati dall'utente l'08/10; bozza del contratto con il frontendista §1.3–§1.6): le
// strutture candidate con i loro nodi e i componenti confermati senza STEP (EB7-1 B), il tipo del nodo con l'origine
// (EB7-2 A), il documento e lo stato del NAS nella voce del 2D (EB7-4 A). Esporre più dati non cambia stato, assi e
// autorità [U]: gli assi di ogni prodotto delle scene sono quelli calcolati con il codice di prima di P7e (1cc6409), e la
// struttura candidata, la fonte confermata, la BOM di lavoro e la BOM verificata restano quattro cose distinte.
package valutazione_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/valutazione"
)

// I clienti di questi test sono inventati (ACME, acme.example; codici 712xxxx; UUID 00000000-0000-4000-8000-0000000000nn):
// vedi scena_emendamenti_test.go.

// Gli assi dei prodotti delle scene (assiDelProdotto), calcolati con il codice di prima di P7e (1cc6409) sulle stesse
// scene: i campi nuovi non li cambiano (vincolo dell'utente dell'08/10). Se una regola di un asse cambia per conto suo,
// si ricalcolano con il commit che la cambia.
const (
	assiCandidata        = "e9859b66c4e763ae074c6c031fa44cd2609f13e8b8e3d40971f23f4e76d80ba3"
	assiDueCandidate     = "430523c8cedbedb3da8a091505c0d2f6ef7049b61ea0f0a7601f6f7d203cbe6e"
	assiAlbero           = "0bb1fe5fc130723c64516632e1c63a91aec6234d667d2f217cc39d9f17f2ae64"
	assiAlberoESecondo   = "d08073820d28bf4462413944a868c4df7ac02c6758aed1f97c1fcbe627bcbbf9"
	assiSenzaSTEP        = "c3f4990d7f593d7c6cbf7f5ffbe74370bc8628cd05901e8203f83dfdd8417180"
	assiAccanto          = "245494abc323099121881d49acacaa2d36c4fd05d2283254289c0fec2d867895"
	assiDueProdottiPrimo = "de66a2f7203e674dab19510a69e164ebc1a6210543209858c70849c63709f944"
	assiDueProdottiAltro = "3231ece1491f8b0167132b5211bd31b2da4d37047d9c9360e5345fc176e4dbbc"
)

// rifNodiDi: i Rif di un elenco di nodi, in ordine.
func rifNodiDi(nodi []valutazione.NodoBOM) []string {
	var out []string
	for _, n := range nodi {
		out = append(out, n.Nodo.Rif)
	}
	return out
}

// nodoFra: il nodo con quel Rif fra quelli dati; se non c'è, la prova fallisce.
func nodoFra(t *testing.T, nodi []valutazione.NodoBOM, rif string) valutazione.NodoBOM {
	t.Helper()
	for _, n := range nodi {
		if n.Nodo.Rif == rif {
			return n
		}
	}
	t.Fatalf("il nodo %s non c'è fra %v", rif, rifNodiDi(nodi))
	return valutazione.NodoBOM{}
}

// tipoDi: il tipo e l'origine di un nodo, per i confronti.
func tipoDi(n valutazione.NodoBOM) string { return n.Tipo + "/" + string(n.OrigineTipo) }

// candidateDelleAncore: le strutture candidate del prodotto fra quelle di ancoraggio, come (allegato, radice), in ordine.
func candidateDelleAncore(et valutazione.EsitoThread, rif string) []string {
	var out []string
	for _, s := range et.Ancoraggi.Strutture {
		if s.Target == rif && s.Stato == ancoraggio.StatoStrutturaCandidata {
			out = append(out, s.AllegatoID.String()+" "+s.Radice)
		}
	}
	return out
}

// chiaviCandidate: le strutture candidate del prodotto valutato, come (allegato, radice), in ordine.
func chiaviCandidate(pv valutazione.ProdottoValutato) []string {
	var out []string
	for _, s := range pv.StruttureCandidate {
		out = append(out, s.AllegatoID.String()+" "+s.Radice)
	}
	return out
}

// TestEB71LaStrutturaCandidataConINodi (EB7-1 B; bozza §1.3): uno STEP in attesa di conferma dà una struttura candidata
// con i suoi nodi, come quelli della BOM di lavoro: la parentela dai fatti, il 2D candidato del figlio con l'origine
// proposto, la minuteria solo proposta dove la grammatica la legge, il tipo proposto. La radice ha lo stesso codice del
// target ma non ne prende il componente (T-B4-38 vale solo per la BOM di lavoro): niente 2D del finito, ruolo componente,
// nessuna descrizione del componente, nessun tipo confermato. Il prodotto non si promuove: PV.Nodi resta vuoto, e gli assi
// sono quelli di prima di P7e.
func TestEB71LaStrutturaCandidataConINodi(t *testing.T) {
	et := esitoSmistamentoCon(t, scenaCandidata(t), insiemeConMinuteria(t))
	p := prodottoDi(t, et, rifProdB)
	if p.Fonte.Stato != valutazione.FonteInAttesaDiConferma || p.Struttura != ancoraggio.StatoStrutturaCandidata || len(p.Nodi) != 0 || p.Verificato ||
		p.BOM.Verificata {
		t.Fatalf("il prodotto: fonte %s, struttura %s, %d nodi, verificato %v, BOM verificata %v", p.Fonte.Stato, p.Struttura, len(p.Nodi), p.Verificato,
			p.BOM.Verificata)
	}
	if a := assiDelProdotto(t, p); a != assiCandidata {
		t.Errorf("gli assi del prodotto sono cambiati: %s, prima di P7e %s", a, assiCandidata)
	}
	if len(p.StruttureCandidate) != 1 {
		t.Fatalf("strutture candidate %v", chiaviCandidate(p))
	}
	s := p.StruttureCandidate[0]
	anc := strutturaDelProdotto(t, valutazione.ValutazioneProdotti{Ancoraggi: et.Ancoraggi}, rifProdB, ancoraggio.StatoStrutturaCandidata)
	if s.AllegatoID != aStepB || s.Sha256 != shaStepB || s.Radice != nodoB("#1") || !s.RadiceDelFile || s.Compatibilita != motorea.CompatibilitaUguale ||
		!s.GrafoCompleto || s.MotivoGrafo != "" || s.AllegatoID != anc.AllegatoID || s.Radice != anc.Radice || s.Compatibilita != anc.Compatibilita {
		t.Errorf("la struttura candidata %+v (ancoraggio %s %s)", s, anc.AllegatoID, anc.Radice)
	}
	if !reflect.DeepEqual(rifNodiDi(s.Nodi), []string{nodoB("#1"), nodoB("#2"), nodoB("#3")}) {
		t.Fatalf("i nodi %v", rifNodiDi(s.Nodi))
	}
	radice, figlio, minuteria := s.Nodi[0], s.Nodi[1], s.Nodi[2]
	if radice.Nodo.Decisione != nil || radice.Disegni.Primario != nil || radice.Associazione != nil || radice.Descrizione != nil || len(radice.AncheIn) != 0 ||
		radice.Classificazione.Ruolo != valutazione.RuoloComponente || radice.OrigineTipo == valutazione.OrigineTipoConfermato || tipoDi(radice) != "sottoassieme/proposto_dalla_regola" {
		t.Errorf("la radice della candidata prende il componente del target: %+v", radice)
	}
	due, quattro := 2, 4
	pr := figlio.Disegni.Primario
	if pr == nil || pr.AllegatoID == nil || *pr.AllegatoID != aPDFCand || pr.Provenienza != ancoraggio.OrigineProposto || figlio.Associazione == nil ||
		*figlio.Associazione != ancoraggio.OrigineProposto || !strings.Contains(motiviNodo(figlio), "associazione_non_confermata") ||
		!reflect.DeepEqual(figlio.Parentela, []valutazione.Parentela{{Padre: nodoB("#1"), Quantita: &due}}) || tipoDi(figlio) != "sciolto/proposto_dalla_regola" {
		t.Errorf("il figlio con il 2D candidato %+v", figlio)
	}
	if c := minuteria.Classificazione; c.Proposta != valutazione.CategoriaMinuteria || c.Categoria != valutazione.CategoriaNonDeterminata || c.Confermata ||
		valutazione.EsenteDal2D(c) || !reflect.DeepEqual(minuteria.Parentela, []valutazione.Parentela{{Padre: nodoB("#1"), Quantita: &quattro}}) {
		t.Errorf("la minuteria proposta %+v", minuteria)
	}
	// Il componente del target, senza BOM di lavoro, sta fra i confermati senza STEP, con il suo 2D e la sua descrizione.
	if len(p.ConfermatiSenzaSTEP) != 1 {
		t.Fatalf("confermati senza STEP %v", rifNodiDi(p.ConfermatiSenzaSTEP))
	}
	if c := p.ConfermatiSenzaSTEP[0]; c.Nodo.Rif != rifProdB || c.Disegni.Primario == nil || c.Disegni.Primario.DocumentoID == nil ||
		*c.Disegni.Primario.DocumentoID != dPDF1 || c.Descrizione == nil || *c.Descrizione != "Assieme ACME" || tipoDi(c) != "finito/confermato" {
		t.Errorf("il componente del target %+v", c)
	}
}

// TestEB71DueSTEPCandidati (EB7-1 B; bozza §1.3): due STEP candidati dello stesso prodotto danno due elementi, nell'ordine
// delle strutture di ancoraggio, con la chiave (allegato, radice); gli assi non cambiano.
func TestEB71DueSTEPCandidati(t *testing.T) {
	th := scenaCandidata(t)
	conSecondoSTEP(t, &th)
	et := esitoSmistamentoCon(t, th, insiemeConMinuteria(t))
	p := prodottoDi(t, et, rifProdB)
	attese := []string{aStepB2.String() + " " + ancoraggio.RifNodo(shaStepB2, "#1"), aStepB.String() + " " + nodoB("#1")}
	if got := chiaviCandidate(p); !reflect.DeepEqual(got, attese) || !reflect.DeepEqual(got, candidateDelleAncore(et, rifProdB)) {
		t.Fatalf("strutture candidate %v, attese %v (ancoraggio %v)", got, attese, candidateDelleAncore(et, rifProdB))
	}
	if !reflect.DeepEqual(rifNodiDi(p.StruttureCandidate[0].Nodi), []string{ancoraggio.RifNodo(shaStepB2, "#1"), ancoraggio.RifNodo(shaStepB2, "#2")}) ||
		len(p.StruttureCandidate[1].Nodi) != 3 || len(p.Nodi) != 0 {
		t.Errorf("i nodi delle due candidate %v e %v; nodi della BOM %d", rifNodiDi(p.StruttureCandidate[0].Nodi), rifNodiDi(p.StruttureCandidate[1].Nodi), len(p.Nodi))
	}
	if a := assiDelProdotto(t, p); a != assiDueCandidate {
		t.Errorf("gli assi del prodotto sono cambiati: %s, prima di P7e %s", a, assiDueCandidate)
	}
}

// TestEB71DopoConfermaLAlbero (EB7-1 B; T-B6-11): con la fonte confermata e «Conferma l'albero» la BOM di lavoro sta in
// PV.Nodi e non fra le candidate; i componenti del perimetro sono tutti rappresentati dai nodi, quindi nessun confermato
// senza STEP. Un secondo STEP dello stesso prodotto resta una candidata, a parte.
func TestEB71DopoConfermaLAlbero(t *testing.T) {
	p := prodottoDi(t, esitoSmistamento(t, scenaCompletezza(t, true)), rifProdB)
	if !reflect.DeepEqual(rifNodi(p), []string{nodoB("#1"), nodoB("#2")}) || len(p.StruttureCandidate) != 0 || len(p.ConfermatiSenzaSTEP) != 0 ||
		p.Struttura != ancoraggio.StatoBOMDiLavoroProposta {
		t.Errorf("nodi %v, candidate %v, confermati senza STEP %v", rifNodi(p), chiaviCandidate(p), rifNodiDi(p.ConfermatiSenzaSTEP))
	}
	if a := assiDelProdotto(t, p); a != assiAlbero {
		t.Errorf("gli assi del prodotto sono cambiati: %s, prima di P7e %s", a, assiAlbero)
	}
	th := scenaCompletezza(t, true)
	conSecondoSTEP(t, &th)
	et := esitoSmistamento(t, th)
	p = prodottoDi(t, et, rifProdB)
	if got := chiaviCandidate(p); !reflect.DeepEqual(got, []string{aStepB2.String() + " " + ancoraggio.RifNodo(shaStepB2, "#1")}) ||
		!reflect.DeepEqual(rifNodi(p), []string{nodoB("#1"), nodoB("#2")}) {
		t.Errorf("con il secondo STEP: candidate %v, nodi %v", got, rifNodi(p))
	}
	if a := assiDelProdotto(t, p); a != assiAlberoESecondo {
		t.Errorf("gli assi del prodotto sono cambiati: %s, prima di P7e %s", a, assiAlberoESecondo)
	}
}

// TestEB71IConfermatiSenzaSTEP (EB7-1 B; bozza §1.4): una BOM confermata senza nessuno STEP: il componente del target per
// primo, poi lo sciolto, ognuno come un NodoBOM senza file (il Rif «componente:<uuid>», l'allegato nullo, la catena del
// codice vuota) con la decisione del contesto di ancoraggio; la parentela dalla relazione confermata, con la quantità
// confermata; il 2D, la classificazione e il tipo confermato del componente.
func TestEB71IConfermatiSenzaSTEP(t *testing.T) {
	p := prodottoDi(t, esitoSmistamento(t, scenaSenzaSTEP(t)), rifProdB)
	if p.Struttura != ancoraggio.StatoStrutturaNessuna || len(p.Nodi) != 0 || len(p.StruttureCandidate) != 0 {
		t.Fatalf("struttura %s, nodi %d, candidate %d", p.Struttura, len(p.Nodi), len(p.StruttureCandidate))
	}
	if a := assiDelProdotto(t, p); a != assiSenzaSTEP {
		t.Errorf("gli assi del prodotto sono cambiati: %s, prima di P7e %s", a, assiSenzaSTEP)
	}
	if !reflect.DeepEqual(rifNodiDi(p.ConfermatiSenzaSTEP), []string{rifProdB, ancoraggio.RifComponente(cSciolto)}) {
		t.Fatalf("confermati senza STEP %v", rifNodiDi(p.ConfermatiSenzaSTEP))
	}
	finito, sciolto := p.ConfermatiSenzaSTEP[0], p.ConfermatiSenzaSTEP[1]
	for _, c := range []struct {
		n      valutazione.NodoBOM
		id     uuid.UUID
		codice string
	}{{finito, cProdotto, "7120100A"}, {sciolto, cSciolto, "7120200A"}} {
		n := c.n.Nodo
		d := n.Decisione
		if n.AllegatoID != uuid.Nil || n.EntitaID != "" || !n.SenzaFile || !reflect.DeepEqual(n.Codice, ancoraggio.CatenaCodice{}) || len(n.Padri) != 0 ||
			n.RigaLegacy != nil || n.RigaDecisa != nil || n.Leggibile || d == nil || d.ComponenteID != c.id || d.Codice != c.codice ||
			d.Autorita != ancoraggio.AutoritaConfermata || d.Origine != ancoraggio.OrigineConfermato || d.Lettura == nil {
			t.Errorf("%s: il nodo senza file %+v", c.codice, n)
		}
	}
	if len(finito.Parentela) != 0 || tipoDi(finito) != "finito/confermato" || finito.Classificazione.Ruolo != valutazione.RuoloProdotto {
		t.Errorf("il finito %+v", finito)
	}
	tre := 3
	if !reflect.DeepEqual(sciolto.Parentela, []valutazione.Parentela{{Padre: rifProdB, Quantita: &tre, Decisa: true}}) || tipoDi(sciolto) != "sciolto/confermato" ||
		sciolto.Disegni.Primario == nil || sciolto.Disegni.Primario.DocumentoID == nil || *sciolto.Disegni.Primario.DocumentoID != dPDF2 ||
		sciolto.Associazione == nil || *sciolto.Associazione != ancoraggio.OrigineConfermato || sciolto.Classificazione.Categoria != valutazione.CategoriaFabbricato {
		t.Errorf("lo sciolto %+v", sciolto)
	}
}

// TestEB71IlComponenteAggiuntoAccantoAllaBOM (EB7-1 B; bozza §1.4): due componenti confermati accanto alla BOM di lavoro,
// senza un nodo: il Padre è il Rif con cui il padre compare nella pagina, cioè il nodo di PV.Nodi che lo rappresenta (la
// radice per il componente del target, il nodo deciso per lo sciolto). In ordine di componente; i componenti rappresentati
// dai nodi non ci sono.
func TestEB71IlComponenteAggiuntoAccantoAllaBOM(t *testing.T) {
	p := prodottoDi(t, esitoSmistamento(t, scenaAccanto(t)), rifProdB)
	if !reflect.DeepEqual(rifNodi(p), []string{nodoB("#1"), nodoB("#2")}) {
		t.Fatalf("nodi %v", rifNodi(p))
	}
	if a := assiDelProdotto(t, p); a != assiAccanto {
		t.Errorf("gli assi del prodotto sono cambiati: %s, prima di P7e %s", a, assiAccanto)
	}
	if !reflect.DeepEqual(rifNodiDi(p.ConfermatiSenzaSTEP), []string{ancoraggio.RifComponente(cSottoSciol), ancoraggio.RifComponente(cAMano)}) {
		t.Fatalf("confermati senza STEP %v", rifNodiDi(p.ConfermatiSenzaSTEP))
	}
	due, quattro := 2, 4
	if n := p.ConfermatiSenzaSTEP[0]; !reflect.DeepEqual(n.Parentela, []valutazione.Parentela{{Padre: nodoB("#2"), Quantita: &due, Decisa: true}}) {
		t.Errorf("sotto lo sciolto: parentela %+v", n.Parentela)
	}
	if n := p.ConfermatiSenzaSTEP[1]; !reflect.DeepEqual(n.Parentela, []valutazione.Parentela{{Padre: nodoB("#1"), Quantita: &quattro, Decisa: true}}) ||
		tipoDi(n) != "sciolto/confermato" {
		t.Errorf("aggiunto a mano sotto il finito: %+v", n)
	}
}

// TestEB71LaDecisioneDelContesto (EB7-1 B; bozza §1.4: «lo stesso del contesto di ancoraggio»): lo sciolto è un nodo
// deciso della BOM di lavoro del primo prodotto e un confermato senza STEP del secondo, che non ha STEP: la sua Decisione
// è la stessa, campo per campo (con l'autorità, l'origine, la revisione e la lettura del contesto), e AncheIn dice il primo
// prodotto. Il primo prodotto non ha confermati senza STEP.
func TestEB71LaDecisioneDelContesto(t *testing.T) {
	et := esitoSmistamento(t, scenaDueProdotti(t))
	p1, p2 := prodottoDi(t, et, rifProdB), prodottoDi(t, et, rifP2Senza)
	nodo := nodoBOMDi(t, p1, nodoB("#2"))
	if !reflect.DeepEqual(rifNodiDi(p2.ConfermatiSenzaSTEP), []string{rifP2Senza, ancoraggio.RifComponente(cSciolto)}) || len(p1.ConfermatiSenzaSTEP) != 0 {
		t.Fatalf("confermati senza STEP %v e %v", rifNodiDi(p2.ConfermatiSenzaSTEP), rifNodiDi(p1.ConfermatiSenzaSTEP))
	}
	c := p2.ConfermatiSenzaSTEP[1]
	if nodo.Nodo.Decisione == nil || c.Nodo.Decisione == nil || !reflect.DeepEqual(*c.Nodo.Decisione, *nodo.Nodo.Decisione) || c.Nodo.Decisione.Rev == nil ||
		*c.Nodo.Decisione.Rev != "2" {
		t.Errorf("la decisione del confermato senza STEP %+v, del nodo %+v", c.Nodo.Decisione, nodo.Nodo.Decisione)
	}
	cinque := 5
	if !reflect.DeepEqual(c.Parentela, []valutazione.Parentela{{Padre: rifP2Senza, Quantita: &cinque, Decisa: true}}) || !reflect.DeepEqual(c.AncheIn, []string{rifProdB}) {
		t.Errorf("parentela %+v, AncheIn %v", c.Parentela, c.AncheIn)
	}
	if a := assiDelProdotto(t, p1); a != assiDueProdottiPrimo {
		t.Errorf("gli assi del primo prodotto sono cambiati: %s, prima di P7e %s", a, assiDueProdottiPrimo)
	}
	if a := assiDelProdotto(t, p2); a != assiDueProdottiAltro {
		t.Errorf("gli assi del secondo prodotto sono cambiati: %s, prima di P7e %s", a, assiDueProdottiAltro)
	}
}

// TestEB72IlTipoDelNodo (EB7-2 A; bozza §1.6): il tipo del nodo con la sua origine.
//   - La radice della BOM di lavoro senza una decisione propria: il tipo del componente del target, confermato; lo sciolto
//     deciso: sciolto, confermato.
//   - Un nodo aperto con tipo_proposto = sottoassieme e senza figli: sottoassieme, proposto dalla riga (la riga vince
//     sulla regola, che direbbe sciolto).
//   - Un nodo senza riga, con figli: sottoassieme, proposto dalla regola; il suo figlio senza figli: sciolto.
//   - Con l'arco verso l'unico figlio tolto da una persona il nodo non ha più figli: sciolto (dubbio T-P7e-05).
func TestEB72IlTipoDelNodo(t *testing.T) {
	p := prodottoDi(t, esitoSmistamento(t, scenaCompletezza(t, true)), rifProdB)
	if r, f := nodoBOMDi(t, p, nodoB("#1")), nodoBOMDi(t, p, nodoB("#2")); tipoDi(r) != "finito/confermato" || r.Nodo.Decisione != nil ||
		tipoDi(f) != "sciolto/confermato" {
		t.Errorf("la radice %s, lo sciolto %s", tipoDi(r), tipoDi(f))
	}
	th := scenaTreNodi(t, true)
	rigaDi(t, &th, "#4").TipoProposto = testo("sottoassieme")
	p = prodottoDi(t, esitoSmistamento(t, th), rifProdB)
	if n := nodoBOMDi(t, p, nodoB("#4")); tipoDi(n) != "sottoassieme/proposto_dalla_riga" || n.Nodo.RigaLegacy == nil {
		t.Errorf("il nodo aperto con il tipo proposto: %s", tipoDi(n))
	}
	if n := nodoBOMDi(t, p, nodoB("#3")); tipoDi(n) != "sottoassieme/proposto_dalla_regola" {
		t.Errorf("il nodo con il figlio e la riga senza tipo: %s", tipoDi(n))
	}
	th = scenaTreNodi(t, true)
	var righe []fotorfq.RigaComponenteProposta
	for _, r := range th.RigheComponenteProposta {
		if r.Chiave != "#3" {
			righe = append(righe, r)
		}
	}
	th.RigheComponenteProposta = righe
	p = prodottoDi(t, esitoSmistamento(t, th), rifProdB)
	if n := nodoBOMDi(t, p, nodoB("#3")); tipoDi(n) != "sottoassieme/proposto_dalla_regola" || n.Nodo.RigaLegacy != nil || n.Nodo.RigaDecisa != nil {
		t.Errorf("il nodo senza riga con il figlio: %s, riga %+v", tipoDi(n), n.Nodo.RigaLegacy)
	}
	if n := nodoBOMDi(t, p, nodoB("#4")); tipoDi(n) != "sciolto/proposto_dalla_regola" {
		t.Errorf("il figlio senza figli: %s", tipoDi(n))
	}
	th = scenaTreNodi(t, true)
	op := operatore
	a := arcoDi(t, &th, "#3", "#4")
	a.Stato, a.DecisoDa = "scartata", &op
	p = prodottoDi(t, esitoSmistamento(t, th), rifProdB)
	if n := nodoBOMDi(t, p, nodoB("#3")); tipoDi(n) != "sciolto/proposto_dalla_regola" || len(rifNodi(p)) != 3 {
		t.Errorf("con l'arco tolto: %s, nodi %v", tipoDi(n), rifNodi(p))
	}
}

// TestEB72IlCommerciale (EB7-2 A; R103 C, T-E1R-01): un nodo deciso come un commerciale ha il tipo commerciale, confermato,
// e la sua classificazione non lo esenta dal 2D: la voce del 2D resta, bloccante e invariante.
func TestEB72IlCommerciale(t *testing.T) {
	th := scenaCompletezza(t, true)
	th.Componenti[1].Tipo = "commerciale"
	p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
	n := nodoBOMDi(t, p, nodoB("#2"))
	if tipoDi(n) != "commerciale/confermato" || n.Classificazione.Categoria != valutazione.CategoriaCommerciale || valutazione.EsenteDal2D(n.Classificazione) {
		t.Errorf("il commerciale: %s, %+v", tipoDi(n), n.Classificazione)
	}
	if v := voceDi(t, p.Documenti, cSciolto, "disegno_2d"); !v.Bloccante || !v.Invariante || v.TipoComponente != "commerciale" {
		t.Errorf("la voce del 2D del commerciale %+v", v)
	}
}

// TestEB74LoStatoDelNASDelDisegno (EB7-4 A; bozza §1.5): la voce del 2D porta il documento che la decide e il suo stato
// del NAS dalla fotografia; l'esito e il motivo della voce non cambiano. Una conferma non è una copia completata: con
// in_coda la voce è presente e lo stato dice in_coda. Senza un 2D confermato, tutti e due vuoti; un «assegna» non è un
// documento. Il documento è quello che decide la voce, non quello della vista (il più recente, qui un DWG).
func TestEB74LoStatoDelNASDelDisegno(t *testing.T) {
	m := motoreCatena(t)
	voce := func(t *testing.T, th fotorfq.Thread, comp uuid.UUID) valutazione.VoceFabbisogno {
		t.Helper()
		return voceDi(t, documentiDi(t, valutaConVista(t, th, m), rifProdB), comp, "disegno_2d")
	}
	th := scenaCompletezza(t, true)
	for _, c := range []struct {
		comp uuid.UUID
		doc  uuid.UUID
	}{{cProdotto, dPDF1}, {cSciolto, dPDF2}} {
		if v := voce(t, th, c.comp); esitoDi(v) != "presente/" || v.DocumentoID == nil || *v.DocumentoID != c.doc || v.StatoNas != "scritto" {
			t.Errorf("il 2D confermato e scritto: %+v", v)
		}
	}
	for _, stato := range []string{"in_coda", "errore"} {
		th := scenaCompletezza(t, true)
		for i := range th.Documenti {
			if th.Documenti[i].ID == dPDF2 {
				th.Documenti[i].StatoNas = stato
			}
		}
		if v := voce(t, th, cSciolto); esitoDi(v) != "presente/" || v.DocumentoID == nil || *v.DocumentoID != dPDF2 || v.StatoNas != stato {
			t.Errorf("il 2D confermato con %s: %+v", stato, v)
		}
	}
	if v := voce(t, scenaCompletezza(t, false), cSciolto); esitoDi(v) != "manca/nessun_documento" || v.DocumentoID != nil || v.StatoNas != "" {
		t.Errorf("senza 2D confermato: %+v", v)
	}
	th = scenaCompletezza(t, false)
	conFile2D(&th, allegato(aPDF2, 3, "disegno-acme.pdf", "pdf", sha2), scansione(t, sha2), nil, cSciolto)
	if v := voce(t, th, cSciolto); esitoDi(v) != "da_verificare/associazione_non_confermata" || v.DocumentoID != nil || v.StatoNas != "" {
		t.Errorf("con il solo «assegna»: %+v", v)
	}
	// Il PDF valido confermato prima e un DWG confermato dopo: la vista prende il più recente, la voce il PDF che la decide.
	th = scenaCompletezza(t, true)
	dwg := documento2D(dDWG, cSciolto, aDWG, "disegno-acme.dwg", "dwg", shaDWG)
	dwg.ConfermatoIl, dwg.StatoNas = dataACME.Add(4*time.Hour), "in_coda"
	conFile2D(&th, allegato(aDWG, 9, "disegno-acme.dwg", "dwg", shaDWG), nil, &dwg, uuid.Nil)
	vistaCome(&th)
	for _, r := range th.Fascicolo {
		if r.ComponenteID == cSciolto && r.TipoDocumento == "disegno_2d" && (r.DocumentoID == nil || *r.DocumentoID != dDWG) {
			t.Fatalf("la vista prende il documento più recente: %+v", r)
		}
	}
	if v := voce(t, th, cSciolto); esitoDi(v) != "presente/" || v.DocumentoID == nil || *v.DocumentoID != dPDF2 || v.StatoNas != "scritto" {
		t.Errorf("il PDF che decide la voce, non il DWG della vista: %+v", v)
	}
}

// TestEB74LaVoceDecisaDallAssociazioneConUnDWG (EB7-4 A, dubbio T-P7e-10): un DWG confermato e corrente, e un PDF con
// «assegna»: la voce è da verificare per l'associazione (VoceDelDisegno non cambia), e il documento confermato della voce
// è il DWG, nell'ordine della bozza (valido, da verificare, formato non configurato). La bozza dice «il documento che
// decide la voce», e qui non la decide: la scelta letterale resta visibile in questa prova.
func TestEB74LaVoceDecisaDallAssociazioneConUnDWG(t *testing.T) {
	th := scenaCompletezza(t, false)
	dwg := documento2D(dDWG, cSciolto, aDWG, "disegno-acme.dwg", "dwg", shaDWG)
	conFile2D(&th, allegato(aDWG, 9, "disegno-acme.dwg", "dwg", shaDWG), nil, &dwg, uuid.Nil)
	conFile2D(&th, allegato(aPDF2, 3, "disegno-acme.pdf", "pdf", sha2), scansione(t, sha2), nil, cSciolto)
	v := voceDi(t, documentiDi(t, valutaConVista(t, th, motoreCatena(t)), rifProdB), cSciolto, "disegno_2d")
	if esitoDi(v) != "da_verificare/associazione_non_confermata" || v.DocumentoID == nil || *v.DocumentoID != dDWG || v.StatoNas != "scritto" {
		t.Errorf("voce %+v", v)
	}
}

// TestEB74IlDocumentoCheDecideSullaRegola (EB7-4 A; bozza §1.5): sulla regola, con gruppi sintetici: il primo 2D corrente e
// confermato nell'ordine della validità (valido, da verificare, formato non configurato) e poi del gruppo (il primario,
// poi gli alternativi); un documento non corrente, un «assegna» o un candidato non decidono niente.
func TestEB74IlDocumentoCheDecideSullaRegola(t *testing.T) {
	d := func(n int) *uuid.UUID { id := uid(0xe790 + n); return &id }
	x := func(n int, prov ancoraggio.OrigineDato, corrente bool, val valutazione.ValiditaDisegno2D) valutazione.Disegno2D {
		out := valutazione.Disegno2D{Provenienza: prov, Corrente: corrente, Validita: val}
		if prov == ancoraggio.OrigineConfermato {
			out.DocumentoID = d(n)
		}
		return out
	}
	conf, man, prop := ancoraggio.OrigineConfermato, ancoraggio.OrigineManuale, ancoraggio.OrigineProposto
	valido, daVer, nonConf := valutazione.ValiditaValido, valutazione.ValiditaDaVerificare, valutazione.ValiditaFormatoNonConfigurato
	gruppo := func(p valutazione.Disegno2D, alt ...valutazione.Disegno2D) *valutazione.GruppoDisegni2D {
		return &valutazione.GruppoDisegni2D{Primario: &p, Alternativi: alt}
	}
	for _, c := range []struct {
		nome   string
		g      *valutazione.GruppoDisegni2D
		attesa *uuid.UUID
	}{
		{"nessun gruppo", nil, nil},
		{"il valido prima del da verificare primario", gruppo(x(1, conf, true, daVer), x(2, conf, true, valido)), d(2)},
		{"il formato non configurato per ultimo", gruppo(x(3, prop, false, valido), x(4, conf, true, nonConf), x(5, conf, true, daVer)), d(5)},
		{"solo il formato non configurato", gruppo(x(6, man, false, valido), x(7, conf, true, nonConf)), d(7)},
		{"il non corrente non decide", gruppo(x(8, conf, false, valido)), nil},
		{"«assegna» e candidati non decidono", gruppo(x(9, man, false, valido), x(10, prop, false, valido)), nil},
		{"fra due validi il primario", gruppo(x(11, conf, true, valido), x(12, conf, true, valido)), d(11)},
	} {
		got := valutazione.DocumentoDellaVoceDel2DPerProva(c.g)
		if (got == nil) != (c.attesa == nil) || (got != nil && *got != *c.attesa) {
			t.Errorf("%s: %v, atteso %v", c.nome, got, c.attesa)
		}
	}
	// L'uscita non condivide memoria con il gruppo.
	g := gruppo(x(13, conf, true, valido))
	if got := valutazione.DocumentoDellaVoceDel2DPerProva(g); got == nil || got == g.Primario.DocumentoID {
		t.Errorf("il documento è lo stesso puntatore del gruppo")
	}
}
