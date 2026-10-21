// L1 — le correzioni dell'adattatore dell'ondata C di V1 (emendamento U2 §4.2 punto 2, §4.4, §4.7; U2-P22, U2-P49;
// decisioni dell'ondata B, PD-03): il motivo della rev sempre con la rev vuota (A-32), la traccia dei nodi fuori per
// decisione con l'identità proposta (A-11 esteso, la parte del registro di R-43), le diagnostiche del contesto non
// applicato nella testata (A-30), la fotografia obbligatoria nell'ingresso. Tutto è inventato: ACME, BETA, i codici
// 712xxxx, gli UUID di prova.
package valutazione_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// TestA32IlMotivoDellaRev (A-32; U2 §4.2 punto 2): ogni voce con rev_proposta vuota porta motivo_rev, in un elenco
// chiuso; con un candidato di revisione fra i riferimenti il motivo è revisione_fra_i_riferimenti (base B di V1-Q26: la
// rev si propone solo dalla lettura dello STEP); senza nessuna revisione revisione_assente. Una voce con la rev
// proposta non ha il motivo, e la rev è in maiuscole ASCII.
func TestA32IlMotivoDellaRev(t *testing.T) {
	ammessi := map[string]bool{propostabom.MotivoRevAssente: true, propostabom.MotivoRevNonLetta: true,
		propostabom.MotivoRevNonAmmissibile: true, propostabom.MotivoRevFraIRiferimenti: true}
	th := scenaCollegamento(t)
	th.Componenti[0].Codice = "7120100A1" // la revisione 1 del codice del target: un candidato di revisione della radice
	c := calcolaPerIlRegistro(t, th, insiemeACME(t))
	k := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	conti := map[string]int{}
	for _, v := range k.Proposta.Nodi {
		mr := testoJSON(t, v.MotivoRev)
		if v.RevProposta != "" {
			if mr != "" || strings.ToUpper(v.RevProposta) != v.RevProposta {
				t.Errorf("%s: rev %q con il motivo %q", v.Voce, v.RevProposta, mr)
			}
			continue
		}
		if !ammessi[mr] {
			t.Errorf("%s: rev vuota con il motivo %q (atteso uno di %v)", v.Voce, mr, ammessi)
		}
		conti[mr]++
		candidato := strings.Contains(string(v.Riferimenti), `"candidato_revisione"`)
		if candidato && mr != propostabom.MotivoRevFraIRiferimenti {
			t.Errorf("%s: un candidato di revisione fra i riferimenti e il motivo %q", v.Voce, mr)
		}
		if !candidato && !strings.Contains(string(v.Riferimenti), `"revisione"`) && mr != propostabom.MotivoRevAssente {
			t.Errorf("%s: nessuna revisione e il motivo %q", v.Voce, mr)
		}
	}
	if conti[propostabom.MotivoRevFraIRiferimenti] == 0 || conti[propostabom.MotivoRevAssente] == 0 {
		t.Errorf("la scena non prova i due motivi: %v", conti)
	}
	if r := voceDelRegistro(t, k, nodoB("#1")); testoJSON(t, r.MotivoRev) != propostabom.MotivoRevFraIRiferimenti {
		t.Errorf("la radice con la revisione del codice del target: %s", r.MotivoRev)
	}
}

// TestA11LaTracciaDeiFuori (A-11 esteso, la parte del registro di R-43; U2 §4.4, Q13 [U] e [UR], U2-P22): un nodo tolto
// da una persona porta chi, quando, la nota della riga come motivo e il gesto d'origine; un nodo tolto con «Conferma
// l'albero» porta il segno; un nodo non raggiunto porta gli archi tolti che lo staccano, ognuno con chi e i campi che lo
// storico non ha in mancanti, mai ricostruiti. Ogni nodo fuori ha l'identità proposta con le due proiezioni, che
// TrovaLettura ritrova. La traccia sta fra le evidenze: cambiare la nota non cambia impronta_contenuto.
func TestA11LaTracciaDeiFuori(t *testing.T) {
	nodi := []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}, {"#4", "7120400A", ""}, {"#5", "7120500A", ""},
		{"#6", "7120600A", ""}}
	archi := []arcoF{{"#1", "#2", 1}, {"#1", "#3", 1}, {"#3", "#4", 3}, {"#1", "#5", 1}, {"#1", "#6", 1}}
	scena := func(nota string) fotorfq.Thread {
		th := scenaBOM(t, nodi, archi)
		op := operatore
		decidi(t, &th, "#5", "scartata", uuid.Nil, &op)
		r5 := rigaDi(t, &th, "#5")
		r5.ComponenteID = nil
		if nota != "" {
			r5.Nota = &nota
		}
		decidi(t, &th, "#6", "scartata", uuid.Nil, &op)
		r6 := rigaDi(t, &th, "#6")
		r6.ComponenteID, r6.DecisoIl = nil, nil
		r6.Albero = &fotorfq.SegnoAlbero{Da: operatore, Il: "2026-10-18T10:00:00Z", Firma: strings.Repeat("f", 64), Nodo: "#6"}
		a := arcoDi(t, &th, "#1", "#3")
		a.Stato, a.DecisoDa = "scartata", &op
		return th
	}
	c := calcolaPerIlRegistro(t, scena("doppione nel file"), insiemeACME(t))
	reg := c.importa(t, rifProdB, propostabom.GruppoBOM, nil)
	k := reg.Contenuto
	fuori := map[string]importazione.FuoriPerDecisione{}
	for _, x := range k.Proposta.FuoriPerDecisione {
		_, rif, _ := importazione.PartiDellaVoce(x.Voce)
		fuori[rif] = x
	}
	if len(fuori) != 4 {
		t.Fatalf("fuori per decisione: %d, attesi #3, #4, #5, #6", len(fuori))
	}
	t5 := fuori[nodoB("#5")]
	if t5.Segno != importazione.SegnoTolto || testoJSON(t, t5.Chi) != operatore.String() || testoJSON(t, t5.Motivo) != "doppione nel file" ||
		testoJSON(t, t5.OrigineLegacy) != propostabom.OrigineLegacyScartaNodo || len(t5.Mancanti) != 0 || len(t5.SegnoAlbero) != 0 {
		t.Errorf("la traccia di #5: %+v", t5)
	}
	quando, err := time.Parse(time.RFC3339Nano, testoJSON(t, t5.Quando))
	if err != nil || !quando.Equal(dataACME.Add(2*time.Hour)) {
		t.Errorf("quando di #5: %s (%v)", t5.Quando, err)
	}
	t6 := fuori[nodoB("#6")]
	var mancanti6 []string
	_ = json.Unmarshal(t6.Mancanti, &mancanti6)
	if testoJSON(t, t6.OrigineLegacy) != propostabom.OrigineLegacyConfermaAlb || testoJSON(t, campoDi(t, t6.SegnoAlbero, "nodo")) != "#6" ||
		strings.Join(mancanti6, ",") != "quando,motivo" || len(t6.Quando) != 0 || len(t6.Motivo) != 0 {
		t.Errorf("la traccia di #6 (Conferma l'albero, senza data né nota): %+v", t6)
	}
	for _, rif := range []string{nodoB("#3"), nodoB("#4")} {
		x := fuori[rif]
		var aa []struct {
			Padre, Figlio, Chi string
			Mancanti           []string
		}
		if err := json.Unmarshal(x.ArchiTolti, &aa); err != nil || len(aa) != 1 || aa[0].Padre != nodoB("#1") || aa[0].Figlio != nodoB("#3") ||
			aa[0].Chi != operatore.String() || strings.Join(aa[0].Mancanti, ",") != "quando,motivo" {
			t.Errorf("%s: archi tolti %s", rif, x.ArchiTolti)
		}
		if x.Segno != importazione.SegnoNonRaggiunto || testoJSON(t, x.OrigineLegacy) != propostabom.OrigineLegacyArcoTolto || len(x.Mancanti) != 0 {
			t.Errorf("%s: %+v", rif, x)
		}
	}
	attesi := map[string]string{nodoB("#3"): "7120300A", nodoB("#4"): "7120400A", nodoB("#5"): "7120500A", nodoB("#6"): "7120600A"}
	for rif, x := range fuori {
		if x.CodiceProposto != attesi[rif] || testoJSON(t, x.MotivoRev) != propostabom.MotivoRevAssente || len(x.IdentitaProposta) == 0 {
			t.Errorf("%s: identità proposta %q / %s / %s", rif, x.CodiceProposto, x.MotivoRev, x.IdentitaProposta)
			continue
		}
		chiave := testoJSON(t, campoDi(t, x.IdentitaProposta, "chiave_lettura"))
		if l, ok := k.TrovaLettura(chiave); !ok || l.Letterale != x.CodiceProposto {
			t.Errorf("%s: TrovaLettura(%s) = %+v, %v", rif, chiave, l, ok)
		}
	}
	// la traccia sta fra le evidenze
	altro := calcolaPerIlRegistro(t, scena("un'altra nota"), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	if altro.ImprontaContenuto != k.ImprontaContenuto || altro.ImprontaEvidenze == k.ImprontaEvidenze {
		t.Errorf("una nota diversa: contenuto %v, evidenze %v (attesi uguale e diverse)", altro.ImprontaContenuto == k.ImprontaContenuto,
			altro.ImprontaEvidenze == k.ImprontaEvidenze)
	}
}

// TestA30IlContestoNonApplicato (A-30; U2 §4.7, Q24 [U], U2-P49): un contesto dichiarato che non si applica (eredita da
// un padre che V1 non cerca) dà contesto.non_valido nella testata del registro, fra le evidenze: impronta_contenuto è
// quella del thread senza contesto, il registro non porta il contesto come applicato (niente regole.contesto).
func TestA30IlContestoNonApplicato(t *testing.T) {
	th := scenaCollegamento(t)
	_, hash := insiemeConPartner(t)
	figlio := contestoACME(th.ID, hash, motorea.OccorrenzaContesto{AllegatoID: aStepB.String()})
	figlio.EreditaDa = "ctx-acme-progetto"
	r, _ := insiemeConPartner(t, figlio)
	senza, _ := insiemeConPartner(t)
	k := calcolaPerIlRegistro(t, th, r).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	base := calcolaPerIlRegistro(t, scenaCollegamento(t), senza).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	if !strings.Contains(string(k.Proposta.Diagnostiche), `"`+motorea.CodiceContestoNonValido+`"`) {
		t.Errorf("la testata non dice il contesto non applicato: %s", k.Proposta.Diagnostiche)
	}
	if strings.Contains(string(base.Proposta.Diagnostiche), motorea.CodiceContestoNonValido) {
		t.Errorf("senza contesto la testata dice contesto.non_valido: %s", base.Proposta.Diagnostiche)
	}
	if k.Regole.Contesto != nil || len(k.Contesto) != 0 {
		t.Errorf("il contesto non applicato sembra applicato: %+v %s", k.Regole.Contesto, k.Contesto)
	}
	if k.ImprontaContenuto != base.ImprontaContenuto {
		t.Error("la diagnostica del contesto cambia impronta_contenuto")
	}
}

// TestLaFotografiaNellIngresso (U2 §4.4): senza la fotografia l'adattatore non compone il registro (la traccia dei nodi
// fuori per decisione non si ricostruisce): un difetto, senza codice.
func TestLaFotografiaNellIngresso(t *testing.T) {
	c := calcolaPerIlRegistro(t, scenaCollegamento(t), insiemeACME(t))
	in := c.ingresso(rifProdB, propostabom.GruppoBOM, nil)
	in.Foto = nil
	_, err := propostabom.ImportaGruppo(in)
	var e *importazione.Errore
	if err == nil || !errors.As(err, &e) || e.Codice != "" {
		t.Errorf("senza la fotografia: %v", err)
	}
}

// TestLeDifferenzeDelleEvidenze (U2 §5.2; DEC-B PD-05, OB-03 la parte del motore): Confronta dice dove cambiano le
// evidenze, in differenze.evidenze[], senza cambiare l'esito né le differenze del contenuto: la nota di un nodo tolto
// (un campo di evidenza di un nodo fuori), un file candidato nuovo (il file aggiunto e le associazioni proposte della
// voce). Con lo stesso registro nessuna differenza, nemmeno fra le evidenze.
func TestLeDifferenzeDelleEvidenze(t *testing.T) {
	nodi := []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}}
	archi := []arcoF{{"#1", "#2", 1}, {"#1", "#3", 1}}
	conNota := func(nota string) importazione.Contenuto {
		th := scenaBOM(t, nodi, archi)
		op := operatore
		decidi(t, &th, "#2", "scartata", uuid.Nil, &op)
		r := rigaDi(t, &th, "#2")
		r.ComponenteID, r.Nota = nil, &nota
		return calcolaPerIlRegistro(t, th, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	}
	a, b := conNota("prima"), conNota("dopo")
	if x := propostabom.Confronta(a, a); !x.Differenze.IsZero() || x.Esito != propostabom.EsitoUguale {
		t.Errorf("lo stesso registro: %+v", x)
	}
	x := propostabom.Confronta(a, b)
	if x.Esito != propostabom.EsitoUguale || !x.Differenze.Vuote() || len(x.Differenze.Evidenze) != 1 ||
		x.Differenze.Evidenze[0].Voce != "n:"+aStepB.String()+":"+nodoB("#2") || x.Differenze.Evidenze[0].Tipo != propostabom.DifferenzaCambiato ||
		strings.Join(x.Differenze.Evidenze[0].Campi, ",") != "motivo" {
		t.Errorf("la nota di un nodo tolto: %+v", x)
	}
	// un file candidato nuovo
	senza := scenaBOM(t, nodi, archi)
	con := scenaBOM(t, nodi, archi)
	dxf := fattiDXF(t, shaDxfK4, [2]string{"riferimento_3d", "7120300A"}, [2]string{"codice", "PN93000"})
	conAllegato(&con, allegato(aDxfK4, 2, "disegno-k4.dxf", "dxf", shaDxfK4), &dxf)
	v := calcolaPerIlRegistro(t, senza, insiemeK4(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	n := calcolaPerIlRegistro(t, con, insiemeK4(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	y := propostabom.Confronta(v, n)
	trovati := map[string]propostabom.DifferenzaEvidenza{}
	for _, e := range y.Differenze.Evidenze {
		trovati[e.Voce] = e
	}
	if e, ok := trovati["f:"+aDxfK4.String()]; !ok || e.Tipo != propostabom.DifferenzaAggiunto {
		t.Errorf("il file candidato nuovo fra le evidenze: %+v", y.Differenze.Evidenze)
	}
	if e, ok := trovati["n:"+aStepB.String()+":"+nodoB("#3")]; !ok || !strings.Contains(strings.Join(e.Campi, ","), "associazioni_proposte") {
		t.Errorf("le associazioni proposte di #3 fra le evidenze: %+v", y.Differenze.Evidenze)
	}
	j, _ := json.Marshal(y)
	if strings.Contains(string(j), "evidenze_cambiate") || !strings.Contains(string(j), `"evidenze":[`) {
		t.Errorf("il JSON del confronto: %s", j)
	}
}

// TestOB17LIdentitaDellaPropostaComeLAdattatore (OB-17; RP RQ-5): per ogni voce con codice_proposto dei registri veri
// dell'adattatore (il cliente, e un partner nel perimetro del contesto), distintabozza.IdentitaDellaProposta (il nodo
// «proposta» che la bozza costruisce, V-1) è byte per byte l'identità che l'adattatore scrive: l'emittente e il
// namespace della lettura scelta, codice_proposto e rev_proposta della voce, il modo proposta; senza codice_proposto
// nessun nodo «proposta».
func TestOB17LIdentitaDellaPropostaComeLAdattatore(t *testing.T) {
	_, hash := insiemeConLegami(t, false)
	r, _ := insiemeConLegami(t, false, contestoDeiLegami(hash, motorea.OccorrenzaContesto{AllegatoID: aStepB.String()}))
	registri := []importazione.Contenuto{
		calcolaPerIlRegistro(t, scenaCollegamento(t), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto,
		calcolaPerIlRegistro(t, scenaLegame(t), r).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto,
	}
	viste := 0
	for _, k := range registri {
		for _, v := range k.Proposta.Nodi {
			id, ok, err := distintabozza.IdentitaDellaProposta(v)
			if err != nil {
				t.Fatalf("%s: %v", v.Voce, err)
			}
			if v.CodiceProposto == "" {
				if ok {
					t.Errorf("%s: un nodo proposta senza codice_proposto", v.Voce)
				}
				continue
			}
			var l struct{ Emittente, Namespace string }
			if err := json.Unmarshal(v.IdentitaProposta, &l); err != nil || l.Emittente == "" {
				t.Fatalf("%s: l'identità proposta %s", v.Voce, v.IdentitaProposta)
			}
			atteso := distintabozza.Identita{Emittente: l.Emittente, Namespace: l.Namespace, Codice: v.CodiceProposto, Rev: v.RevProposta,
				Modo: distintabozza.ModoProposta}
			a, _ := json.Marshal(atteso)
			b, _ := json.Marshal(id)
			if !ok || string(a) != string(b) {
				t.Errorf("%s: la bozza costruisce %s, l'adattatore dice %s", v.Voce, b, a)
			}
			if !strings.HasPrefix(l.Emittente, "cliente:") && !strings.HasPrefix(l.Emittente, "partner:") {
				t.Errorf("%s: emittente %q", v.Voce, l.Emittente)
			}
			viste++
		}
	}
	if viste < 4 {
		t.Errorf("solo %d voci con il codice proposto: la prova non dice niente", viste)
	}
}

// TestA33LIdentificativoNelModello (A-33, la parte di /dati; U2 §4.2 punto 4): nel modello del prodotto ogni nodo con il
// codice proposto ha l'identificativo verso il cliente composto con la grammatica del cliente (la forma del cartiglio
// ACME chiede la revisione: senza rev proposta il motivo, mai una stringa inventata), e la decisione esistente ha quello
// dell'identità registrata (codice e rev della working: la concatenazione della forma ACME). Senza le regole: il motivo
// grammatica_assente. Un emittente partner usa la grammatica del partner.
func TestA33LIdentificativoNelModello(t *testing.T) {
	r := insiemeACME(t)
	c := calcolaPerIlRegistro(t, scenaCollegamento(t), r)
	e, et := c.e, c.et
	w := map[uuid.UUID]propostabom.ComponenteDellaWorking{cSciolto: {Codice: "7120200A", Rev: "1"}}
	m := propostabom.Modello(propostabom.IngressoModello{Foto: &c.f, Esito: &e, Thread: &et, Regole: c.reg, Prodotto: rifProdB, Componenti: w,
		Insieme: r})
	n := nodoDelModello(t, gruppoDelModello(t, m, propostabom.GruppoBOM), nodoB("#2"))
	if n.IdentificativoCliente == nil || n.IdentificativoCliente.Testo != "" || n.IdentificativoCliente.Motivo != motorea.MotivoComposizioneRevisioneNonDeterminata {
		t.Errorf("l'identificativo della proposta senza rev: %+v", n.IdentificativoCliente)
	}
	if n.DecisioneEsistente == nil || n.DecisioneEsistente.IdentificativoCliente != (propostabom.IdentificativoCliente{Testo: "7120200A1"}) {
		t.Errorf("l'identificativo dell'identità registrata: %+v", n.DecisioneEsistente)
	}
	senza := propostabom.Modello(propostabom.IngressoModello{Foto: &c.f, Esito: &e, Thread: &et, Regole: c.reg, Prodotto: rifProdB, Componenti: w})
	n = nodoDelModello(t, gruppoDelModello(t, senza, propostabom.GruppoBOM), nodoB("#2"))
	if n.IdentificativoCliente == nil || n.IdentificativoCliente.Motivo != propostabom.MotivoIdentificativoGrammaticaAssente ||
		n.DecisioneEsistente.IdentificativoCliente.Motivo != propostabom.MotivoIdentificativoGrammaticaAssente {
		t.Errorf("senza le regole: %+v / %+v", n.IdentificativoCliente, n.DecisioneEsistente)
	}
	cliente := "cliente:" + clienteACME.String()
	for _, x := range []struct {
		emittente, namespace, codice, rev string
		atteso                            propostabom.IdentificativoCliente
	}{
		{cliente, "acme-catena", "7120300A", "2", propostabom.IdentificativoCliente{Testo: "7120300A2"}},
		{cliente, "", "7120300A", "2", propostabom.IdentificativoCliente{Testo: "7120300A2"}},
		{cliente, "acme-catena", "ACM-1200", "2", propostabom.IdentificativoCliente{Motivo: motorea.MotivoIdentificativoCodiceNonLetto}},
		{"partner:nessuno", "", "7120300A", "2", propostabom.IdentificativoCliente{Motivo: propostabom.MotivoIdentificativoGrammaticaAssente}},
		{"", "", "7120300A", "2", propostabom.IdentificativoCliente{Motivo: propostabom.MotivoIdentificativoGrammaticaAssente}},
	} {
		if got := propostabom.IdentificativoDelCliente(r, x.emittente, x.namespace, x.codice, x.rev); got != x.atteso {
			t.Errorf("%+v: %+v", x, got)
		}
	}
}

// TestLaPropostaAggiornataNelleSeiClassi (U2 §5.3; C-04 C [U]): il registro della conferma confrontato con la proposta di
// adesso: un'identità cambiata (identità), un nodo e il suo arco spariti (gerarchia), con le classi di C-04 contate; lo
// stesso registro non dà nessuna proposta aggiornata; una quantità cambiata è quantità.
func TestLaPropostaAggiornataNelleSeiClassi(t *testing.T) {
	nodi := []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}, {"#4", "7120400A", ""}}
	archi := []arcoF{{"#1", "#2", 1}, {"#1", "#3", 1}, {"#1", "#4", 1}}
	conferma := calcolaPerIlRegistro(t, scenaBOM(t, nodi, archi), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	if x := propostabom.PropostaAggiornataDi(conferma, conferma); x != nil {
		t.Errorf("lo stesso registro: %+v", x)
	}
	nodi2 := []nodoF{{"#1", "7120100A", ""}, {"#2", "7120250A", ""}, {"#3", "7120300A", ""}}
	adesso := calcolaPerIlRegistro(t, scenaBOM(t, nodi2, archi[:2]), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	x := propostabom.PropostaAggiornataDi(conferma, adesso)
	if x == nil || x.ImprontaContenuto != adesso.ImprontaContenuto || x.Gruppo != propostabom.GruppoBOM {
		t.Fatalf("la proposta aggiornata: %+v", x)
	}
	if x.PerTipo != (propostabom.PerTipo{Identita: 1, Gerarchia: 2}) {
		t.Errorf("le classi: %+v (differenze %+v)", x.PerTipo, x.Differenze)
	}
	archi3 := []arcoF{{"#1", "#2", 1}, {"#1", "#3", 2}, {"#1", "#4", 1}}
	qta := calcolaPerIlRegistro(t, scenaBOM(t, nodi, archi3), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	if y := propostabom.PropostaAggiornataDi(conferma, qta); y == nil || y.PerTipo.Quantita != 1 || y.PerTipo.Identita != 0 {
		t.Errorf("una quantità cambiata: %+v", y)
	}
}

// impronteOroImportazione1: le impronte d'oro di importazione-1 sul registro sintetico di T-13 (DEC-B PD-03, OB-02), fissate
// alla chiusura del codice dell'ondata C (corsia motore, 18/10). Un campo cambiato fuori dall'elenco chiuso del registro
// (contenuto, evidenze, regole) le rompe: da qui ogni campo nuovo vuole importazione-2. Si riscrivono solo con un commit
// che dichiara il cambio di versione (o, prima del primo registro fuori dai DB di prova, con la ratifica della revisione).
var impronteOroImportazione1 = [4]string{
	"db48d01dbbda6819b31d6d89e75345fe8df3fd4a3b395bad2caa91620377fe32", // impronta
	"fa9c7284e1bbb471433a608f605fd75b5e518fbe10ece5fc35e5334faf06ffab", // impronta_contenuto
	"f8d135ec1dd04b87775f7e478dd1852eb30c3bef394ceac65e2d02d2386f90e9", // impronta_evidenze
	"fc1d1e475326a3daa74cbc26cbe33b1482f3063599fe59a354f155164a4bc131", // impronta_regole
}

// TestOB02LImprontaDOroDiImportazione1 (OB-02): le quattro impronte del registro della scena di T-13 sono quelle d'oro, e
// la versione è importazione-1 con il formato del registro di oggi.
func TestOB02LImprontaDOroDiImportazione1(t *testing.T) {
	c := calcolaPerIlRegistro(t, scenaBOM(t, nodiT13, archiT13), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil)
	got := [4]string{c.Impronta, c.Contenuto.ImprontaContenuto, c.Contenuto.ImprontaEvidenze, c.Contenuto.ImprontaRegole}
	if c.Contenuto.VersioneImportazione != "importazione-1" {
		t.Errorf("la versione dell'importazione %q", c.Contenuto.VersioneImportazione)
	}
	if got != impronteOroImportazione1 {
		t.Errorf("le impronte di importazione-1 sono cambiate:\n %q\natteso:\n %q", got, impronteOroImportazione1)
	}
}
