// L1 — T-13, i sottocasi a scena (piano della 0023 §10, T-13; EM §3.8.1, §6.4.1, §6.11; O-01 con il secondo, il terzo e il
// quarto giro), sull'uscita vera dell'adattatore: due esiti del motore che differiscono per un solo fatto della RFQ, e le
// impronte parziali dei due registri. La parte sui campi e sulle proiezioni sta in importazione_registro_scena_test.go
// (TestT13NienteOrariNelConfronto, TestT13LeProiezioniSullUscitaVera). Nati dai sondaggi della revisione della corsia
// motore (ondata B), portati nella suite con le attese di T-13. Tutto è inventato, come nelle scene.
package valutazione_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// nodiT13, archiT13: la struttura delle scene di T-13: la radice, un figlio, un sottoassieme #3 con il suo figlio #4.
var (
	nodiT13  = []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}, {"#4", "7120310A", ""}}
	archiT13 = []arcoF{{"#1", "#2", 2}, {"#1", "#3", 1}, {"#3", "#4", 1}}
)

// soloLaVoce: le differenze del confronto sono tutte della voce data (nessun arco, niente del gruppo), e la voce c'è;
// restituisce i suoi campi.
func soloLaVoce(t *testing.T, x propostabom.Confronto, voce string) []string {
	t.Helper()
	var campi []string
	trovata := false
	for _, d := range x.Differenze.Nodi {
		if d.Voce != voce {
			t.Errorf("una differenza fuori da %s: %+v", voce, d)
			continue
		}
		trovata, campi = true, d.Campi
	}
	if !trovata || len(x.Differenze.Archi) != 0 || len(x.Differenze.Gruppo) != 0 || x.Esito != propostabom.EsitoPropostaCambiata {
		t.Errorf("atteso proposta_cambiata solo per %s: %s %+v", voce, x.Esito, x.Differenze)
	}
	return campi
}

// TestT13LaCandidataELaBOMDopoAutorizza (T-13, O-01 e OV-01): la stessa struttura candidata (prima di «Autorizza») e
// BOM di lavoro (dopo, con la riga della radice decisa come il prodotto), sulla stessa occorrenza, danno la stessa
// impronta_contenuto; le evidenze cambiano (la chiave e lo stato della struttura, la riga della radice).
func TestT13LaCandidataELaBOMDopoAutorizza(t *testing.T) {
	dopo := scenaBOM(t, nodiT13, archiT13)
	op := operatore
	decidi(t, &dopo, "#1", "duplicato", cProdotto, &op)
	bom := calcolaPerIlRegistro(t, dopo, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	prima := scenaBOM(t, nodiT13, archiT13)
	senzaFonte(&prima)
	cand := calcolaPerIlRegistro(t, prima, insiemeACME(t)).importa(t, rifProdB, propostabom.ChiaveCandidata(aStepB, nodoB("#1")), nil).Contenuto
	x := propostabom.Confronta(cand, bom)
	if bom.ImprontaContenuto != cand.ImprontaContenuto || x.Esito != propostabom.EsitoUguale || !x.Differenze.Vuote() {
		t.Errorf("candidata e BOM dopo «Autorizza»: %s %+v", x.Esito, x.Differenze)
	}
	if bom.ImprontaEvidenze == cand.ImprontaEvidenze || len(x.Differenze.Evidenze) == 0 {
		t.Error("candidata e BOM: le evidenze devono cambiare (la chiave, lo stato della struttura, la riga della radice)")
	}
}

// TestT13LaRadiceDiRiserva (T-13, OV-09): con una radice di riserva la candidata (origine associazione_step, prima di
// «Autorizza») e la BOM di lavoro (dopo) hanno anche gruppo.tipo, gruppo.origine e gruppo.compatibilita diversi, che
// stanno fra le evidenze: stessa impronta_contenuto.
func TestT13LaRadiceDiRiserva(t *testing.T) {
	r := insiemeRiserve(t)
	rif := ancoraggio.RifComponente(cSost)
	radiceDiA := ancoraggio.RifNodo(shaSostA, "#1")
	cand := calcolaPerIlRegistro(t, scenaSostituzione(t, sostituzione{momento: momentoCandidata, radiceA: radiceRiserva, nomeA: nomeMuto}), r).
		importa(t, rif, propostabom.ChiaveCandidata(aSostA, radiceDiA), nil).Contenuto
	bom := calcolaPerIlRegistro(t, scenaSostituzione(t, sostituzione{momento: momentoAutorizzato, radiceA: radiceRiserva, nomeA: nomeMuto}), r).
		importa(t, rif, propostabom.GruppoBOM, nil).Contenuto
	if testoJSON(t, cand.Gruppo.Origine) != ancoraggio.OrigineStrutturaAssociazioneSTEP || testoJSON(t, cand.Gruppo.Tipo) == testoJSON(t, bom.Gruppo.Tipo) {
		t.Fatalf("i campi del gruppo della scena: candidata %+v, BOM %+v", cand.Gruppo, bom.Gruppo)
	}
	x := propostabom.Confronta(cand, bom)
	if cand.ImprontaContenuto != bom.ImprontaContenuto || x.Esito != propostabom.EsitoUguale || !x.Differenze.Vuote() || len(x.Differenze.Evidenze) == 0 {
		t.Errorf("la radice di riserva, candidata e BOM: %s %+v", x.Esito, x.Differenze)
	}
}

// TestT13RaggruppamentoDelegaERigaDecisa (T-13, giro 2, OV-21 e OT-01; EM §6.11, «Raggruppamenti e deleghe»): sulla BOM
// di lavoro dopo «Autorizza», la riga di #3 decisa come il prodotto (un raggruppamento), con un componente delegato (una
// delega: «duplicato» con quel componente), con un componente da un gesto legacy («confermata») o chiusa con un
// componente senza una persona (un aggancio automatico per codice) cambia impronta_contenuto solo per la voce di #3, con
// decisione_esistente fra i campi.
func TestT13RaggruppamentoDelegaERigaDecisa(t *testing.T) {
	op := operatore
	base := scenaBOM(t, nodiT13, archiT13)
	decidi(t, &base, "#1", "duplicato", cProdotto, &op)
	a := calcolaPerIlRegistro(t, base, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	for _, c := range []struct {
		nome       string
		stato      string
		componente uuid.UUID
		conferma   bool
		persona    bool
	}{
		{"raggruppamento", "duplicato", cProdotto, false, true},
		{"delega", "duplicato", cTerzo, true, true},
		{"riga decisa con un componente", "confermata", cTerzo, true, true},
		// terzo giro su O-01, OT-01: una riga chiusa con un componente senza una persona (un aggancio automatico per codice)
		{"riga chiusa con un componente senza una persona", "duplicato", cTerzo, true, false},
	} {
		t.Run(c.nome, func(t *testing.T) {
			th := scenaBOM(t, nodiT13, archiT13)
			decidi(t, &th, "#1", "duplicato", cProdotto, &op)
			if c.conferma {
				confermaComponente(&th, cTerzo, "7120300A", cProdotto, 1)
			}
			da := &op
			if !c.persona {
				da = nil
			}
			decidi(t, &th, "#3", c.stato, c.componente, da)
			b := calcolaPerIlRegistro(t, th, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
			campi := soloLaVoce(t, propostabom.Confronta(a, b), vocePerProva(aStepB, nodoB("#3")))
			if !contieneTesto(campi, "decisione_esistente") {
				t.Errorf("%s: i campi di #3 senza decisione_esistente: %v", c.nome, campi)
			}
		})
	}
}

// TestT13LeRigheLegacy (T-13, terzo giro su O-01, OT-01): la chiusura di una riga senza una persona («scartata» con
// deciso_da nullo, come dopo la sostituzione dello STEP) cambia impronta_contenuto solo per quella voce, senza toccare
// stato_motore; una riga riscritta e rimasta aperta (un altro id, la stessa proposta) cambia solo le evidenze.
func TestT13LeRigheLegacy(t *testing.T) {
	nodi := []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}}
	archi := []arcoF{{"#1", "#2", 1}, {"#1", "#3", 1}}
	base := calcolaPerIlRegistro(t, scenaBOM(t, nodi, archi), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	th := scenaBOM(t, nodi, archi)
	rigaDi(t, &th, "#3").Stato = "scartata"
	chiusa := calcolaPerIlRegistro(t, th, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	campi := soloLaVoce(t, propostabom.Confronta(base, chiusa), vocePerProva(aStepB, nodoB("#3")))
	if !contieneTesto(campi, "abbinamento_proposto") || contieneTesto(campi, "stato_motore.tolto") || contieneTesto(campi, "stato_motore.raggiunto") {
		t.Errorf("la riga di #3 chiusa senza una persona: campi %v", campi)
	}
	th2 := scenaBOM(t, nodi, archi)
	rigaDi(t, &th2, "#3").ID = uid(0x7aa)
	riscritta := calcolaPerIlRegistro(t, th2, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	if y := propostabom.Confronta(base, riscritta); y.Esito != propostabom.EsitoUguale || len(y.Differenze.Evidenze) == 0 || !y.Differenze.Vuote() {
		t.Errorf("la riga riscritta e aperta: %s, evidenze cambiate %v, %+v", y.Esito, len(y.Differenze.Evidenze) > 0, y.Differenze)
	}
}

// TestT13LaCopia (T-13, quarto giro su O-01, OQ-01 e OT-03): due copie dello stesso STEP, lo STEP della scena portatore
// delle righe legacy e la copia senza righe.
//   - Un arco tolto da una persona sul portatore (#1 → #3) toglie #3 anche nel registro della copia (non_raggiunto),
//     perché gli archi tolti si contano per contenuto: impronta_contenuto cambia per la copia come per il portatore.
//   - La riga di #3 sul portatore scartata da una persona, con il tipo proposto dalla riga: nel registro del portatore
//     #3 è tolto; in quello della copia #3 resta una voce, e cambia solo tipo_proposto (le righe aperte si contano per
//     contenuto), mai stato_motore.tolto, abbinamento_proposto o decisione_esistente (le righe si cercano per allegato).
func TestT13LaCopia(t *testing.T) {
	op := operatore
	// i registri della copia e del portatore sulla scena con la copia, dopo il cambio dato della fotografia (nil: nessuno)
	registri := func(t *testing.T, cambia func(*testing.T, *fotorfq.Thread)) (copia, bom importazione.Contenuto) {
		t.Helper()
		th := scenaCollegamento(t)
		conAllegato(&th, allegato(aStepGemello, 6, "copia-acme.stp", "stp", shaStepB), nil)
		rigaDi(t, &th, "#3").TipoProposto = testo("commerciale")
		if cambia != nil {
			cambia(t, &th)
		}
		c := calcolaPerIlRegistro(t, th, insiemeACME(t))
		var chiave string
		for _, g := range propostabom.GruppiDelProdotto(prodottoDi(t, c.et, rifProdB), c.et.Ancoraggi.Strutture) {
			if g.AllegatoID != nil && *g.AllegatoID == aStepGemello {
				chiave = g.Chiave
			}
		}
		if chiave == "" {
			t.Fatal("la copia non ha un gruppo del prodotto")
		}
		return c.importa(t, rifProdB, chiave, nil).Contenuto, c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	}
	copia0, bom0 := registri(t, nil)

	t.Run("l'arco tolto sul portatore", func(t *testing.T) {
		k, b := registri(t, func(t *testing.T, th *fotorfq.Thread) {
			a := arcoDi(t, th, "#1", "#3")
			a.Stato, a.DecisoDa = "scartata", &op
		})
		trovato := false
		for _, x := range k.Proposta.FuoriPerDecisione {
			trovato = trovato || (x.Voce == vocePerProva(aStepGemello, nodoB("#3")) && x.Segno == importazione.SegnoNonRaggiunto)
		}
		if !trovato {
			t.Errorf("nella copia #3 non è fuori come non_raggiunto: %+v", k.Proposta.FuoriPerDecisione)
		}
		if k.ImprontaContenuto == copia0.ImprontaContenuto {
			t.Error("l'arco tolto sul portatore non cambia il registro della copia")
		}
		if b.ImprontaContenuto == bom0.ImprontaContenuto {
			t.Error("l'arco tolto non cambia il registro del portatore")
		}
	})
	t.Run("la riga del portatore scartata da una persona", func(t *testing.T) {
		copia1, bom1 := registri(t, func(t *testing.T, th *fotorfq.Thread) { decidi(t, th, "#3", "scartata", uuid.Nil, &op) })
		tolto := false
		for _, x := range bom1.Proposta.FuoriPerDecisione {
			tolto = tolto || (x.Voce == vocePerProva(aStepB, nodoB("#3")) && x.Segno == importazione.SegnoTolto)
		}
		if !tolto {
			t.Errorf("nel registro del portatore #3 non è tolto: %+v", bom1.Proposta.FuoriPerDecisione)
		}
		campi := soloLaVoce(t, propostabom.Confronta(copia0, copia1), vocePerProva(aStepGemello, nodoB("#3")))
		if !contieneTesto(campi, "tipo_proposto") {
			t.Errorf("nella copia #3 deve cambiare tipo_proposto: %v", campi)
		}
		for _, c := range []string{"stato_motore.tolto", "stato_motore.raggiunto", "abbinamento_proposto", "decisione_esistente", "codice_manuale_legacy"} {
			if contieneTesto(campi, c) {
				t.Errorf("nella copia #3 cambia %s, che sta sul portatore: %v", c, campi)
			}
		}
	})
}

// TestT13IlDueDDelProdottoSullaRadice (T-13, OV-27): un 2D del prodotto associato alla radice, con il cartiglio discorde
// dal codice del prodotto, e un nodo sotto la radice (#2, deciso con lo sciolto) con un suo 2D: le discordanze della
// radice con il codice confermato ci sono solo sulla BOM di lavoro e stanno fra le evidenze, il riferimento documentale
// di #2 è lo stesso nei due esiti, quindi candidata e BOM dopo «Autorizza» danno la stessa impronta_contenuto.
func TestT13IlDueDDelProdottoSullaRadice(t *testing.T) {
	nodi := []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}}
	archi := []arcoF{{"#1", "#2", 1}, {"#1", "#3", 1}}
	op := operatore
	scena := func(t *testing.T) fotorfq.Thread {
		th := scenaBOM(t, nodi, archi)
		conDisegno(t, &th, cProdotto, "7120100B1", testo("1"))
		confermaComponente(&th, cSciolto, "7120200A", cProdotto, 1)
		decidi(t, &th, "#2", "confermata", cSciolto, &op)
		conDisegnoSu(t, &th, cSciolto, "7120200A1", uid(0xe8b1), uid(0xe8b2), strings.Repeat("9b", 32), "disegno-sciolto.pdf")
		return th
	}
	dopo := scena(t)
	decidi(t, &dopo, "#1", "duplicato", cProdotto, &op)
	bom := calcolaPerIlRegistro(t, dopo, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	prima := scena(t)
	senzaFonte(&prima)
	cand := calcolaPerIlRegistro(t, prima, insiemeACME(t)).importa(t, rifProdB, propostabom.ChiaveCandidata(aStepB, nodoB("#1")), nil).Contenuto
	if !contieneDocumentale(t, voceDelRegistro(t, bom, nodoB("#1"))) || !contieneDocumentale(t, voceDelRegistro(t, bom, nodoB("#2"))) {
		t.Fatal("la radice o #2 della BOM senza il riferimento documentale del loro 2D: la prova non prova niente")
	}
	if a, b := voceDelRegistro(t, cand, nodoB("#2")).Riferimenti, voceDelRegistro(t, bom, nodoB("#2")).Riferimenti; string(a) != string(b) {
		t.Errorf("i riferimenti di #2 cambiano fra candidata e BOM:\n%s\n%s", a, b)
	}
	if x := propostabom.Confronta(cand, bom); bom.ImprontaContenuto != cand.ImprontaContenuto || x.Esito != propostabom.EsitoUguale {
		t.Errorf("OV-27, candidata e BOM: %s %+v", x.Esito, x.Differenze)
	}
}

// conDisegnoSu: un 2D confermato di un componente su un allegato dato (come conDisegno, con gli ID e lo sha dati).
func conDisegnoSu(t *testing.T, th *fotorfq.Thread, comp uuid.UUID, cartiglio string, a, d uuid.UUID, shaFile, nome string) {
	t.Helper()
	f := fattiPDF(t, shaFile, cartiglio)
	conAllegato(th, allegato(a, 8, nome, "pdf", shaFile), &f)
	c := comp
	th.Documenti = append(th.Documenti, fotorfq.DocumentoConfermato{ID: d, ThreadID: threadACME, ComponenteID: &c, Tipo: "disegno_2d", NomeFile: nome,
		Estensione: "pdf", Sha256: shaFile, Rev: testo("1"), StatoNas: "scritto", ConfermatoDa: operatore, ConfermatoIl: dataACME, Allegati: []uuid.UUID{a}})
}

func contieneDocumentale(t *testing.T, v importazione.Voce) bool {
	t.Helper()
	for _, r := range riferimentiDi(t, v) {
		if r.Tipo == "documentale" {
			return true
		}
	}
	return false
}
