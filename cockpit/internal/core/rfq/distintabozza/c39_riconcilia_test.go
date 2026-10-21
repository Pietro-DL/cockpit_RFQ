package distintabozza

// Le prove L1 di cambio-fonte-2 (direttiva 39 §3 [U + T-39]; EM App. I.7; riconciliazione3.go): la bozza manuale senza
// STEP e lo STEP nuovo. Che cosa si prova, su Cambia (puro):
//   - il nodo manuale con l'identita' sostenuta dalla voce tiene la chiave, le PB e le decisioni e acquisisce la casella
//     (modo proposta con la stessa identita'); la voce non entra (niente doppioni);
//   - i casi non certi (evidenze discordanti, ambiguo, incompatibile) in abbinamenti_proposti[], fuori da da_rivedere[],
//     con le voci trattenute; gli archi del registro verso le voci trattenute e la quantita' diversa fra le differenze;
//     le decisioni che citano un nodo abbinato e mono_pezzo da rivedere, mai tolte;
//   - il nodo registrato senza casella e la decisione_esistente della voce: abbinato, identita' registrata; le altre voci
//     entrano con i loro archi; P-6 non aggiunge i confermati senza STEP;
//   - l'arco da una voce che entra verso un nodo abbinato senza padre entra; la sorgente solo_prodotto vale come assenza;
//   - fra due sorgenti STEP l'algoritmo resta cambio-fonte-1.
// Dati sintetici (gli aiuti di formato3_aiuti_test.go, c3_riallinea_test.go, c3_step_test.go).

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// nodoManuale: un nodo scritto a mano nella bozza senza STEP (modo manuale, op crea, origine utente, senza casella).
func nodoManuale(id idProva3, chiave, codice, rev, tipo string) Nodo3 {
	return Nodo3{ChiaveBozza: chiave, Op: OpCrea, Tipo: tipo, Origine: OrigineUtente,
		Identita: Identita{Emittente: id.clienteE(), EmittentePresunto: true, Codice: codice, Rev: rev, Modo: ModoManuale}}
}

// vocePropone: la voce del registro di prova (per chiave PRODUCT) propone codice e rev con la lettura del cliente.
func vocePropone(t *testing.T, id idProva3, c *importazione.Contenuto, chiave, codice, rev string, alternative bool) {
	t.Helper()
	for i := range c.Proposta.Nodi {
		v := &c.Proposta.Nodi[i]
		if v.Origine.Chiave != chiave {
			continue
		}
		v.CodiceProposto, v.RevProposta = codice, rev
		v.IdentitaProposta = rawDi(t, letturaProva3(id.allegato, "l:u:step:"+chiave+":id:acme-num/sei:0-8", id.clienteE(), "acme", codice, map[string]any{}, nil))
		v.MotivoProposto, v.MotivoLettura = nil, nil
		if !alternative {
			v.Alternative = nil
		}
		return
	}
	t.Fatalf("voce %s assente", chiave)
}

func bozzaManualeProva(t *testing.T, in IngressoPartenza) Snapshot3 {
	t.Helper()
	s, _, err := PartenzaRegistrata(PartenzaVuota, in)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func proposto(xx []AbbinamentoProposto3, chiave string) (AbbinamentoProposto3, bool) {
	for _, x := range xx {
		if x.ChiaveBozza == chiave {
			return x, true
		}
	}
	return AbbinamentoProposto3{}, false
}

func diffArco(d propostabom.Differenze, padre, figlio, tipo string) bool {
	for _, a := range d.Archi {
		if a.Padre == padre && a.Figlio == figlio && a.Tipo == tipo {
			return true
		}
	}
	return false
}

func rivedere(xx []propostabom.DaRivedere, decisione string) bool {
	for _, x := range xx {
		if x.DecisioneID == decisione && x.Motivo == RivedereFonteCambiata {
			return true
		}
	}
	return false
}

func TestV1D39LaBozzaManualeIncontraLoStep(t *testing.T) {
	id := idSintetici3()
	in, _ := ingressoStepProva(id)
	prima := bozzaManualeProva(t, in)
	root := ChiaveDelComponente(id.prodotto)
	prima.Nodi = append(prima.Nodi,
		nodoManuale(id, "m-telaio", "ACM-1100", "A", "sottoassieme"),
		nodoManuale(id, "m-pezzo", "ACM-1109", "", "sciolto"),
		nodoManuale(id, "m-d1", "ACM-1111", "", "sciolto"),
		nodoManuale(id, "m-d2", "ACM-1111", "", "sciolto"),
		nodoManuale(id, "m-cordone", "ACM-1131", "B", "sciolto"),
		nodoManuale(id, "m-libero", "ACM-1999", "", "commerciale"))
	prima.Archi = append(prima.Archi,
		Arco3{Padre: root, Figlio: "m-telaio", Qta: 2, Origine: OrigineUtente},
		Arco3{Padre: "m-telaio", Figlio: "m-pezzo", Qta: 1, Origine: OrigineUtente},
		Arco3{Padre: "m-telaio", Figlio: "m-d1", Qta: 1, Origine: OrigineUtente},
		Arco3{Padre: root, Figlio: "m-d2", Qta: 1, Origine: OrigineUtente},
		Arco3{Padre: "m-telaio", Figlio: "m-cordone", Qta: 1, Origine: OrigineUtente},
		Arco3{Padre: root, Figlio: "m-libero", Qta: 4, Origine: OrigineUtente})
	prima.Decisioni = []Decisione3{{ID: "dv-1", Tipo: DecisioneDaValutare, ChiaveBozza: "m-telaio"}, {ID: "mp-1", Tipo: DecisioneMonoPezzo, Motivo: "pezzo unico"}}
	prima.DocumentiPreassociati = []Preassociazione3{{AllegatoID: id.pdf, ChiaveBozza: "m-telaio"}, {AllegatoID: idDi("d39-pdf2"), ChiaveBozza: "m-pezzo"}}
	if !SenzaStruttura(prima) {
		t.Fatal("la bozza vuota non ha struttura")
	}
	c := registroProva3(t, id)
	vocePropone(t, id, &c, "#9", "ACM-1109", "", true) // con le alternative: evidenze discordanti
	vocePropone(t, id, &c, "#11", "ACM-1111", "", false)
	vocePropone(t, id, &c, "#31", "ACM-1131", "A", false)
	nuovo := componi(t, c)
	es, err := Cambia(prima, nil, nuovo, nil, "arriva lo STEP", in)
	if err != nil {
		t.Fatal(err)
	}
	s := es.Snapshot
	r := es.Riconciliazione
	if r == nil {
		t.Fatal("senza struttura vale cambio-fonte-2")
	}
	// il telaio manuale: stessa chiave, casella acquisita, identita' della proposta (stesso codice, rev ed emittente)
	tel, ok := nodoC3(s, "m-telaio")
	if !ok || len(tel.Caselle) != 1 || tel.Caselle[0].AllegatoID != id.allegato || tel.Caselle[0].Rif != id.rif("#7") || tel.Origine != OrigineUtente ||
		tel.Identita.Modo != ModoProposta || tel.Identita.Codice != "ACM-1100" || tel.Identita.Rev != "A" || tel.Identita.Namespace != "acme" ||
		tel.Identita.EmittentePresunto || tel.Tipo != "sottoassieme" {
		t.Fatalf("telaio: %+v", tel)
	}
	if _, doppio := nodoC3(s, ChiaveDellaVoce(id.allegato, id.rif("#7"))); doppio {
		t.Fatal("la voce abbinata e' entrata come doppione")
	}
	if len(es.Aggiornate) != 1 || es.Aggiornate[0].ChiaveBozza != "m-telaio" || es.Aggiornate[0].Prima.Modo != ModoManuale {
		t.Fatalf("identita aggiornate: %+v", es.Aggiornate)
	}
	if len(r.Abbinati) != 1 || len(r.Acquisite) != 2 || r.Acquisite[0].Prima != nil || r.Acquisite[0].Dopo != id.allegato || r.Acquisite[1].Prima != nil {
		t.Fatalf("abbinati: %+v %+v", r.Abbinati, r.Acquisite)
	}
	// i casi non certi: fuori da da_rivedere, le voci trattenute
	for k, motivo := range map[string]string{"m-pezzo": AbbinamentoEvidenzeDiscordanti, "m-d1": AbbinamentoAmbiguo, "m-d2": AbbinamentoAmbiguo,
		"m-cordone": AbbinamentoIncompatibile} {
		p, ok := proposto(r.Proposti, k)
		if !ok || p.Motivo != motivo || len(p.Voci) != 1 {
			t.Fatalf("abbinamento proposto di %s: %+v (%+v)", k, p, r.Proposti)
		}
		if n, ok := nodoC3(s, k); !ok || len(n.Caselle) != 0 || n.Identita.Modo != ModoManuale {
			t.Fatalf("il nodo %s non abbinato e' cambiato: %+v", k, n)
		}
	}
	if _, ok := proposto(r.Proposti, "m-libero"); ok {
		t.Fatal("un nodo senza candidati fra gli abbinamenti proposti")
	}
	if len(es.Entrati) != 0 || len(es.Effetto.NodiCheEntrano) != 0 || len(s.Nodi) != len(prima.Nodi) {
		t.Fatalf("le voci trattenute sono entrate: %v %v", es.Entrati, es.Effetto.NodiCheEntrano)
	}
	for _, x := range es.Effetto.DaRivedere {
		if strings.HasPrefix(x.ChiaveBozza, "m-") && x.DecisioneID == "" {
			t.Fatalf("un abbinamento proposto in da_rivedere: %+v", x)
		}
	}
	// gli archi: tutti quelli della bozza restano con le quantita' dell'ingegnere; le differenze
	if len(s.Archi) != len(prima.Archi) {
		t.Fatalf("archi: %d, prima %d", len(s.Archi), len(prima.Archi))
	}
	for _, a := range s.Archi {
		if a.Padre == root && a.Figlio == "m-telaio" && (a.Qta != 2 || a.Origine != OrigineUtente) {
			t.Fatalf("l'arco dell'ingegnere e' cambiato: %+v", a)
		}
	}
	d := es.Effetto.Differenze
	if !diffArco(d, id.rif("#1"), id.rif("#7"), propostabom.DifferenzaCambiato) || !diffArco(d, id.rif("#7"), id.rif("#9"), propostabom.DifferenzaAggiunto) ||
		!diffArco(d, id.rif("#7"), id.rif("#31"), propostabom.DifferenzaAggiunto) {
		t.Fatalf("differenze: %+v", d)
	}
	// le decisioni e le PB restano; quelle incerte da rivedere
	if len(s.Decisioni) != 2 || len(s.DocumentiPreassociati) != 2 || !rivedere(es.Effetto.DaRivedere, "dv-1") || !rivedere(es.Effetto.DaRivedere, "mp-1") {
		t.Fatalf("decisioni %+v PB %+v da rivedere %+v", s.Decisioni, s.DocumentiPreassociati, es.Effetto.DaRivedere)
	}
	if len(es.Vassoio) != 0 || len(es.eventi) != 0 {
		t.Fatalf("niente nel vassoio e nessun evento di rimozione: %v %+v", es.Vassoio, es.eventi)
	}
	if *s.Sorgente.AllegatoID != id.allegato || s.Sorgente.Importazione != nuovo.Impronta {
		t.Fatalf("sorgente: %+v", s.Sorgente)
	}
	if _, err := s.Canonico(); err != nil {
		t.Fatalf("forma: %v", err)
	}
	// «Accetta» dell'evidenza discordante e' un salvataggio R2 dopo; senza la voce con le alternative il pezzo si abbina da solo
	c2 := registroProva3(t, id)
	vocePropone(t, id, &c2, "#9", "ACM-1109", "", false)
	es2, err := Cambia(prima, nil, componi(t, c2), nil, "arriva lo STEP", in)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := nodoC3(es2.Snapshot, "m-pezzo"); len(n.Caselle) != 1 || n.Caselle[0].Rif != id.rif("#9") {
		t.Fatalf("pezzo senza evidenze discordanti: %+v", n)
	}
	// un nodo manuale con le sue evidenze documentali non si abbina da solo
	conEvidenze := prima.copia()
	for i := range conEvidenze.Nodi {
		if conEvidenze.Nodi[i].ChiaveBozza == "m-pezzo" {
			conEvidenze.Nodi[i].EvidenzeIdentita = []EvidenzaDocumentale{{AllegatoID: id.pdf, Sha256: id.shaPdf, Nota: "cartiglio"}}
		}
	}
	es3, err := Cambia(conEvidenze, nil, componi(t, c2), nil, "arriva lo STEP", in)
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := proposto(es3.Riconciliazione.Proposti, "m-pezzo"); !ok || p.Motivo != AbbinamentoEvidenzeDiscordanti {
		t.Fatalf("pezzo con le evidenze manuali: %+v", es3.Riconciliazione.Proposti)
	}
}

func TestV1D39IlRegistratoSenzaCasellaELeVociCheEntrano(t *testing.T) {
	id := idSintetici3()
	in, comp := ingressoStepProva(id)
	prima := bozzaManualeProva(t, in)
	root := ChiaveDelComponente(id.prodotto)
	f, ok := in.Stato.Componente(comp["f"])
	if !ok {
		t.Fatal("componente f")
	}
	var tr TracciaPartenza
	kf := ChiaveDelComponente(comp["f"])
	prima.Nodi = append(prima.Nodi, in.nodoRegistrato(f, kf, OrigineRegistrato, &tr), nodoManuale(id, "m-orfano", "ACM-1109", "", "sciolto"))
	prima.Archi = append(prima.Archi, Arco3{Padre: root, Figlio: kf, Qta: 2, Origine: OrigineRegistrato})
	c := registroProva3(t, id)
	c.Proposta.Nodi[1].DecisioneEsistente = rawDi(t, map[string]any{"componente_id": comp["f"].String(), "codice": "ACM-1500"})
	vocePropone(t, id, &c, "#9", "ACM-1109", "", false)
	es, err := Cambia(prima, nil, componi(t, c), nil, "arriva lo STEP", in)
	if err != nil {
		t.Fatal(err)
	}
	s := es.Snapshot
	nf, _ := nodoC3(s, kf)
	if len(nf.Caselle) != 1 || nf.Caselle[0].Rif != id.rif("#7") || nf.Identita.Modo != ModoRegistrato || nf.Identita.Codice != "ACM-1500" {
		t.Fatalf("registrato abbinato: %+v", nf)
	}
	for _, n := range s.Nodi {
		if n.ComponenteID != nil && *n.ComponenteID == comp["f"] && n.ChiaveBozza != kf {
			t.Fatal("doppione del registrato")
		}
		if n.ComponenteID != nil && *n.ComponenteID == comp["n"] {
			t.Fatal("P-6 ha aggiunto un confermato senza STEP alla bozza manuale")
		}
	}
	// #9 abbinato al nodo orfano: l'arco #7 → #9 e' fra due nodi che restano, non entra da solo e si elenca
	no, _ := nodoC3(s, "m-orfano")
	if len(no.Caselle) != 1 || no.Caselle[0].Rif != id.rif("#9") || !diffArco(es.Effetto.Differenze, id.rif("#7"), id.rif("#9"), propostabom.DifferenzaAggiunto) {
		t.Fatalf("orfano abbinato: %+v %+v", no, es.Effetto.Differenze)
	}
	// #11 e #31 entrano sotto il registrato abbinato (P-5)
	k11, k31 := ChiaveDellaVoce(id.allegato, id.rif("#11")), ChiaveDellaVoce(id.allegato, id.rif("#31"))
	for _, k := range []string{k11, k31} {
		if _, ok := nodoC3(s, k); !ok {
			t.Fatalf("la voce %s non e' entrata", k)
		}
		trovato := false
		for _, a := range s.Archi {
			trovato = trovato || (a.Padre == kf && a.Figlio == k && a.Origine == OrigineStep)
		}
		if !trovato {
			t.Fatalf("l'arco verso %s non e' entrato: %+v", k, s.Archi)
		}
	}
	if !diffArco(es.Effetto.Differenze, id.rif("#1"), id.rif("#7"), propostabom.DifferenzaCambiato) {
		t.Fatalf("la quantita' diversa: %+v", es.Effetto.Differenze)
	}
	if _, err := s.Canonico(); err != nil {
		t.Fatalf("forma: %v", err)
	}
	// l'arco da una voce che entra verso un nodo abbinato senza padre entra
	prima2 := bozzaManualeProva(t, in)
	prima2.Nodi = append(prima2.Nodi, nodoManuale(id, "m-orfano", "ACM-1109", "", "sciolto"))
	c2 := registroProva3(t, id)
	vocePropone(t, id, &c2, "#9", "ACM-1109", "", false)
	es2, err := Cambia(prima2, nil, componi(t, c2), nil, "arriva lo STEP", in)
	if err != nil {
		t.Fatal(err)
	}
	k7 := ChiaveDellaVoce(id.allegato, id.rif("#7"))
	entrato := false
	for _, a := range es2.Snapshot.Archi {
		entrato = entrato || (a.Padre == k7 && a.Figlio == "m-orfano" && a.Qta == 2)
	}
	if !entrato {
		t.Fatalf("l'arco verso l'orfano abbinato: %+v", es2.Snapshot.Archi)
	}
}

func TestV1D39SoloProdottoESorgenteSTEP(t *testing.T) {
	id := idSintetici3()
	in, _ := ingressoStepProva(id)
	// solo_prodotto sullo stesso STEP: vale come l'assenza, le voci entrano (non e' il caso delle due occorrenze)
	prima := bozzaManualeProva(t, in)
	all := id.allegato
	prima.Sorgente = &Sorgente3{AllegatoID: &all, Sha256: id.sha, Gruppo: GruppoSoloProdotto}
	if !SenzaStruttura(prima) {
		t.Fatal("solo_prodotto e' senza struttura")
	}
	es, err := Cambia(prima, nil, componiProva3(t, id), nil, "analisi finita", in)
	if err != nil {
		t.Fatal(err)
	}
	if es.Riconciliazione == nil || len(es.Entrati) == 0 || len(es.Effetto.NuoviNodiProposti) != 0 {
		t.Fatalf("solo_prodotto: entrati %v proposti %v", es.Entrati, es.Effetto.NuoviNodiProposti)
	}
	// fra due sorgenti STEP: cambio-fonte-1
	vecchio := componiProva3(t, id)
	v := versioneProva(t, id, vecchio)
	if SenzaStruttura(v) {
		t.Fatal("la versione step ha struttura")
	}
	altro := idDi("d39-altro")
	es, err = Cambia(v, &vecchio, componi(t, spostaRegistro(t, registroProva3(t, id), id.allegato, altro, id.sha, shaProva('e'))), nil, "revisione", in)
	if err != nil {
		t.Fatal(err)
	}
	if es.Riconciliazione != nil {
		t.Fatal("fra due sorgenti STEP vale cambio-fonte-1")
	}
	_ = uuid.Nil
}
