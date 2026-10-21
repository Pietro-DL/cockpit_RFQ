package distintabozza

// Le prove L1 della partenza step (direttiva 35, blocco C-3, R2 passo 6b; EM §6.1 P-1…P-8, §6.2 V-7, §6.3): la versione 1
// dal registro di prova (la radice al prodotto, le voci proposte e da decidere, il tipo proposto e la regola di sempre, gli
// archi con la quantita' dei fatti, i confermati senza STEP del gruppo bom, il riuso per decisione_esistente, il
// raggruppamento, i fuori per decisione che non entrano, nessuna preassociazione), la quantita' mancante, la scelta del
// gruppo dichiarato (GruppoDaImportare), V-7 e la traduzione dei rifiuti dell'adattatore.

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// ingressoStepProva: la working del prodotto della prova (ACM-1000, con un figlio confermato senza STEP e un nipote, e un
// componente registrato che una voce riusa) e il cliente della RFQ.
func ingressoStepProva(id idProva3) (IngressoPartenza, map[string]uuid.UUID) {
	c := map[string]uuid.UUID{"f": idDi("c3s-figlio"), "n": idDi("c3s-nipote"), "r": idDi("c3s-riuso")}
	comp := []confermabom.Componente{
		{ID: id.prodotto, CodiceLetterale: "ACM-1000", Tipo: "finito", Rev: "B", Descrizione: "Gruppo base"},
		{ID: c["f"], CodiceLetterale: "ACM-1500", Tipo: "sottoassieme"},
		{ID: c["n"], CodiceLetterale: "ACM-1510", Tipo: "commerciale"},
		{ID: c["r"], CodiceLetterale: "ACM-1900", Tipo: "sciolto", Rev: "a"},
	}
	archi := []confermabom.Arco{{Padre: id.prodotto, Figlio: c["f"], Qta: 2}, {Padre: c["f"], Figlio: c["n"], Qta: 5, Posizione: "10"}}
	st := confermabom.NuovoStatoRFQ(id.thread, 4, comp, archi, nil, nil)
	return IngressoPartenza{Prodotto: id.prodotto, Stato: st, Certificazioni: map[uuid.UUID]Certificazione{}, Cliente: id.clienteE()}, c
}

func TestV1C3LaVersioneDalRegistro(t *testing.T) {
	id := idSintetici3()
	reg := componiProva3(t, id)
	in, c := ingressoStepProva(id)
	s, tr, err := VersioneDaRegistro(reg, in)
	if err != nil {
		t.Fatal(err)
	}
	if s.Sorgente == nil || *s.Sorgente.AllegatoID != id.allegato || s.Sorgente.Sha256 != id.sha || s.Sorgente.Gruppo != "bom" ||
		s.Sorgente.Importazione != reg.Impronta {
		t.Fatalf("sorgente: %+v", s.Sorgente)
	}
	nodi := map[string]Nodo3{}
	for _, n := range s.Nodi {
		nodi[n.ChiaveBozza] = n
	}
	// P-2: la radice e' il nodo del prodotto, registrato, con la casella della radice
	root := nodi[ChiaveDelComponente(id.prodotto)]
	if root.ComponenteID == nil || *root.ComponenteID != id.prodotto || root.Op != OpRiusa || root.Origine != OrigineRegistrato ||
		len(root.Caselle) != 1 || root.Caselle[0].Rif != id.rif("#1") || root.Identita.Modo != ModoRegistrato || root.Tipo != "finito" {
		t.Fatalf("radice: %+v", root)
	}
	// P-4: il telaio proposto (codice, rev e identita' della lettura), il tipo proposto
	tel := nodi[ChiaveDellaVoce(id.allegato, id.rif("#7"))]
	if tel.Op != OpCrea || tel.Origine != OrigineStep || tel.Identita.Modo != ModoProposta || tel.Identita.Codice != "ACM-1100" ||
		tel.Identita.Rev != "A" || tel.Identita.Namespace != "acme" || tel.Tipo != "sottoassieme" || tel.Descrizione != "" {
		t.Fatalf("telaio: %+v", tel)
	}
	// P-4: le voci senza codice proposto sono da decidere, del cliente presunto; foglie → sciolto
	for _, k := range []string{"#9", "#11", "#31"} {
		n := nodi[ChiaveDellaVoce(id.allegato, id.rif(k))]
		if n.Identita.Modo != ModoDaDecidere || n.Identita.Codice != "" || !n.Identita.EmittentePresunto || n.Identita.Emittente != id.clienteE() ||
			n.Tipo != "sciolto" || len(n.Caselle) != 1 {
			t.Fatalf("voce %s: %+v", k, n)
		}
	}
	// P-5: gli archi del registro, con la quantita' e lo step
	step := 0
	for _, a := range s.Archi {
		if a.Origine == OrigineStep {
			step++
			if a.Step == nil || a.Step.AllegatoID != id.allegato || a.Step.Gruppo != "bom" {
				t.Fatalf("arco step: %+v", a)
			}
		}
	}
	if step != 4 {
		t.Fatalf("archi step: %d", step)
	}
	// P-6: il gruppo bom porta i confermati senza STEP sotto il prodotto (figlio e nipote), con i loro archi registrati
	f, n := nodi[ChiaveDelComponente(c["f"])], nodi[ChiaveDelComponente(c["n"])]
	if f.Origine != OrigineRegistrato || len(f.Caselle) != 0 || n.Origine != OrigineRegistrato || f.Identita.Modo != ModoRegistrato {
		t.Fatalf("P-6: %+v %+v", f, n)
	}
	registrati := 0
	for _, a := range s.Archi {
		if a.Origine == OrigineRegistrato {
			registrati++
			if a.Figlio == n.ChiaveBozza && (a.Qta != 5 || a.Posizione != "10") {
				t.Fatalf("arco confermato: %+v", a)
			}
		}
	}
	if registrati != 2 || len(s.Nodi) != 7 {
		t.Fatalf("P-6: %d archi registrati, %d nodi", registrati, len(s.Nodi))
	}
	// P-7, P-8: nessuna decisione, nessuna preassociazione; la voce fuori per decisione non entra
	if len(s.Decisioni) != 0 || len(s.DocumentiPreassociati) != 0 {
		t.Fatalf("decisioni o preassociazioni nella versione 1: %+v %+v", s.Decisioni, s.DocumentiPreassociati)
	}
	if _, ok := nodi[ChiaveDellaVoce(id.allegato, id.rif("#12"))]; ok {
		t.Fatal("la voce fuori per decisione e' entrata")
	}
	if f := FuoriDelRegistro(reg.Contenuto); len(f) != 1 || f[0] != id.voce("#12") {
		t.Fatalf("fuori per decisione: %v", f)
	}
	if len(tr.DescrizioniOltreLimite) != 0 {
		t.Fatalf("traccia: %+v", tr)
	}
	if _, err := s.Canonico(); err != nil {
		t.Fatalf("la versione 1 non ha la forma del formato 3: %v", err)
	}
}

// P-3 (il riuso per decisione_esistente) e il raggruppamento (decisione_esistente = il prodotto, su una voce che non e' la
// radice: P-4 con il tipo sottoassieme); la quantita' mancante rifiuta la partenza.
func TestV1C3LaVersioneDalRegistroRiusoERaggruppamento(t *testing.T) {
	id := idSintetici3()
	in, c := ingressoStepProva(id)
	cont := registroProva3(t, id)
	for i := range cont.Proposta.Nodi {
		switch cont.Proposta.Nodi[i].Origine.Rif {
		case id.rif("#9"):
			cont.Proposta.Nodi[i].DecisioneEsistente = rawDi(t, map[string]any{"componente_id": c["r"].String(), "codice": "ACM-1900"})
		case id.rif("#11"):
			cont.Proposta.Nodi[i].DecisioneEsistente = rawDi(t, map[string]any{"componente_id": id.prodotto.String(), "codice": "ACM-1000"})
		}
	}
	reg, err := importazione.Componi(cont)
	if err != nil {
		t.Fatal(err)
	}
	s, _, err := VersioneDaRegistro(reg, in)
	if err != nil {
		t.Fatal(err)
	}
	nodi := map[string]Nodo3{}
	for _, n := range s.Nodi {
		nodi[n.ChiaveBozza] = n
	}
	riuso := nodi[ChiaveDellaVoce(id.allegato, id.rif("#9"))]
	if riuso.Op != OpRiusa || riuso.ComponenteID == nil || *riuso.ComponenteID != c["r"] || riuso.Origine != OrigineStep || len(riuso.Caselle) != 1 ||
		riuso.Identita.Modo != ModoRegistrato || riuso.Identita.Codice != "ACM-1900" || riuso.Identita.Rev != "a" {
		t.Fatalf("P-3: %+v", riuso)
	}
	rag := nodi[ChiaveDellaVoce(id.allegato, id.rif("#11"))]
	if rag.Op != OpCrea || rag.Tipo != "sottoassieme" || rag.ComponenteID != nil {
		t.Fatalf("raggruppamento: %+v", rag)
	}
	if _, err := s.Canonico(); err != nil {
		t.Fatalf("forma: %v", err)
	}
	// una quantita' assente: arco_senza_quantita
	cont = registroProva3(t, id)
	cont.Proposta.Archi[1].Quantita = nil
	reg, err = importazione.Componi(cont)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = VersioneDaRegistro(reg, in)
	if e := errore3(t, err); e.Motivo != MotivoArcoSenzaQuantita {
		t.Fatalf("quantita' assente: %+v", e)
	}
}

func TestV1C3IlGruppoDichiarato(t *testing.T) {
	a, b, altro := idDi("c3g-a"), idDi("c3g-b"), idDi("c3g-altro")
	sha := shaProva('b')
	gruppi := []propostabom.Gruppo{
		{Chiave: "bom", Tipo: propostabom.GruppoBOM, AllegatoID: &b, Sha256: sha, Radice: "nodo:" + sha + ":#1"},
		{Chiave: "cand:a:#1", Tipo: propostabom.GruppoCandidata, AllegatoID: &a, Sha256: sha, Radice: "nodo:" + sha + ":#1"},
		{Chiave: "cand:a:#5", Tipo: propostabom.GruppoCandidata, AllegatoID: &a, Sha256: sha, Radice: "nodo:" + sha + ":#5"},
		{Chiave: "cand:altro:#1", Tipo: propostabom.GruppoCandidata, AllegatoID: &altro, Sha256: shaProva('e'), Radice: "nodo:" + shaProva('e') + ":#1"},
		{Chiave: "senza", Tipo: propostabom.GruppoSenzaSTEP},
	}
	impronte := map[string]string{"cand:a:#1": shaProva('1'), "cand:a:#5": shaProva('5'), "bom": shaProva('9')}
	improntaDi := func(k string) (string, error) {
		if x, ok := impronte[k]; ok {
			return x, nil
		}
		return "", errors.New("nessun gruppo")
	}
	nessuno := propostabom.NonAttivi{}
	// il gruppo c'e', sull'occorrenza dichiarata
	g, err := GruppoDaImportare("crea.sorgente", gruppi, nessuno, SorgenteDichiarata{AllegatoID: a, Gruppo: "cand:a:#1"}, shaProva('1'), nil, improntaDi)
	if err != nil || g.Chiave != "cand:a:#1" {
		t.Fatalf("gruppo dichiarato: %v %+v", err, g)
	}
	// la BOM di lavoro sta su un'altra occorrenza (b): gruppo_su_altra_occorrenza con l'allegato del gruppo
	_, err = GruppoDaImportare("crea.sorgente", gruppi, nessuno, SorgenteDichiarata{AllegatoID: a, Gruppo: "bom"}, shaProva('9'), nil, improntaDi)
	if e := errore3(t, err); e.Motivo != MotivoGruppoSuAltraOccorrenza || e.AllegatoID == nil || *e.AllegatoID != b {
		t.Fatalf("altra occorrenza: %+v", e)
	}
	// cambia_sorgente verso la stessa struttura sulla stessa occorrenza: stessa_sorgente (la radice della sorgente c'e')
	src := &propostabom.SorgenteBozza{AllegatoID: a, Sha256: sha, Radice: "nodo:" + sha + ":#1", Gruppo: "bom"}
	_, err = GruppoDaImportare("cambia_sorgente.sorgente", gruppi, nessuno, SorgenteDichiarata{AllegatoID: a, Gruppo: "cand:a:#1"}, shaProva('1'), src, improntaDi)
	if e := errore3(t, err); e.Motivo != MotivoStessaSorgente {
		t.Fatalf("stessa sorgente: %+v", e)
	}
	// un'altra radice dello stesso file non e' la stessa sorgente
	if _, err := GruppoDaImportare("cambia_sorgente.sorgente", gruppi, nessuno, SorgenteDichiarata{AllegatoID: a, Gruppo: "cand:a:#5"}, shaProva('5'), src,
		improntaDi); err != nil {
		t.Fatalf("un'altra radice: %v", err)
	}
	// il gruppo dichiarato non c'e' piu', la stessa struttura c'e' sotto un'altra chiave: 409 con il gruppo ritrovato
	_, err = GruppoDaImportare("crea.sorgente", gruppi, nessuno, SorgenteDichiarata{AllegatoID: a, Gruppo: "cand:a:#vecchia"}, shaProva('5'), nil, improntaDi)
	if e := errore3(t, err); e.Codice != CodiceAnteprimaSuperata || e.Anteprima != MotivoPropostaStep || e.GruppoCorrente == nil ||
		e.GruppoCorrente.Chiave != "cand:a:#5" || e.GruppoCorrente.Come != propostabom.ComeStessaOccorrenza {
		t.Fatalf("gruppo ritrovato: %+v", e)
	}
	// non c'e' e la struttura e' cambiata: lo stesso 409 senza il gruppo
	_, err = GruppoDaImportare("crea.sorgente", gruppi, nessuno, SorgenteDichiarata{AllegatoID: a, Gruppo: "cand:a:#vecchia"}, shaProva('7'), nil, improntaDi)
	if e := errore3(t, err); e.Codice != CodiceAnteprimaSuperata || e.GruppoCorrente != nil {
		t.Fatalf("struttura cambiata: %+v", e)
	}
	// l'allegato dichiarato non ha gruppi del prodotto: gruppo_non_importabile; il gruppo senza STEP non si importa
	_, err = GruppoDaImportare("crea.sorgente", gruppi, nessuno, SorgenteDichiarata{AllegatoID: idDi("c3g-x"), Gruppo: "cand:x:#1"}, shaProva('1'), nil, improntaDi)
	if e := errore3(t, err); e.Motivo != MotivoGruppoNonImportabile {
		t.Fatalf("senza gruppi: %+v", e)
	}
	// una candidata su un'occorrenza non attiva non c'e' (come in /dati)
	_, err = GruppoDaImportare("crea.sorgente", gruppi, propostabom.NonAttivi{altro: "altri_file"}, SorgenteDichiarata{AllegatoID: altro, Gruppo: "cand:altro:#1"},
		shaProva('1'), nil, improntaDi)
	if e := errore3(t, err); e.Motivo != MotivoGruppoNonImportabile {
		t.Fatalf("candidata non attiva: %+v", e)
	}
}

func TestV1C3V7ELaTraduzioneDeiRifiuti(t *testing.T) {
	id := idSintetici3()
	reg := componiProva3(t, id)
	if err := VerificaImprontaDichiarata("crea.impronta_contenuto", reg, reg.Contenuto.ImprontaContenuto); err != nil {
		t.Fatal(err)
	}
	err := VerificaImprontaDichiarata("crea.impronta_contenuto", reg, shaProva('0'))
	if e := errore3(t, err); e.Codice != CodiceAnteprimaSuperata || e.ImprontaContenuto != reg.Contenuto.ImprontaContenuto {
		t.Fatalf("V-7: %+v", e)
	}
	ad := &importazione.Errore{Codice: "FILE_NON_ATTIVO", Messaggio: "lo STEP e' in Altri file"}
	if e := errore3(t, ErroreDellAdattatore(ad, "crea.sorgente.gruppo")); e.Codice != CodiceFileNonAttivo || e.Campo != "crea.sorgente.gruppo" {
		t.Fatalf("rifiuto dell'adattatore: %+v", e)
	}
	difetto := &importazione.Errore{Messaggio: "un difetto"}
	var x *Errore
	if errors.As(ErroreDellAdattatore(difetto, "c"), &x) {
		t.Fatal("un difetto dell'adattatore diventa un rifiuto")
	}
	// la risposta porta il gruppo ritrovato nella forma del contratto
	b, _ := json.Marshal(GruppoRitrovato{Chiave: "bom", Come: "stessa_occorrenza", ImprontaContenuto: shaProva('1')})
	if string(b) != `{"chiave":"bom","come":"stessa_occorrenza","impronta_contenuto":"`+shaProva('1')+`"}` {
		t.Fatalf("gruppo ritrovato: %s", b)
	}
}
