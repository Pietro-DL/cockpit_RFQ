package bancoa

import (
	"context"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/confronto"
)

// L1 — A1c-L1-25: il gate (piano 6.4.9, «Il gate nel rapporto»; v3 §6; R44, R58 B) su esiti sintetici: le false
// associazioni sui file dello scenario e della baseline; i conteggi dichiarati contro gli esiti; le decisioni
// preservate; i riservati mai contati come passati; la C5 e i non coperti fuori dal gate; un file in due sezioni contato
// una volta sola (D7); le voci che il runner non verifica da sé restano NON ESEGUITE. Una voce non superata dà
// «non_superato» e, con -gate, l'uscita 1; una voce non eseguita e nessuna non superata dà «incompleto» e l'uscita 3.
// Gli assi del prodotto, lo stato e il fascicolo non sono voci del gate (T-B0-16).
//
// I clienti di questi test sono inventati: vedi scena_banco_test.go.

// ingressoGateACME: un ingresso del gate con tutto in ordine, salvo le voci che il runner non verifica da sé.
func ingressoGateACME() ingressoGate {
	sc := &ScenarioAtteso{Conteggi: []ConteggioAtteso{{Nome: "radice", Valore: 1}, {Nome: "figlio", Valore: 1},
		{Nome: "fuori_scenario", Valore: 1}, {Nome: "prodotti", Valore: 1}, {Nome: "file", Valore: 3}}}
	return ingressoGate{
		Voci: []esitoVoce{
			{Percorso: "s[0]", Sezione: confronto.SezioneScenario, AllegatoID: allRadice, Cliente: clienteACME, Atteso: "radice", Esito: confronto.EsitoCorretto},
			{Percorso: "s[1]", Sezione: confronto.SezioneScenario, AllegatoID: allFiglio, Cliente: clienteACME, Atteso: "figlio", Esito: confronto.EsitoCorretto},
			{Percorso: "s[2]", Sezione: confronto.SezioneScenario, AllegatoID: allFuori, Cliente: clienteACME, Atteso: "fuori", Esito: confronto.EsitoFuoriRichiesta},
			{Percorso: "b[0]", Sezione: confronto.SezioneBaseline, AllegatoID: allBaseline, Cliente: clienteACME, Esito: confronto.EsitoCorretto},
			{Percorso: "r[0]", Sezione: confronto.SezioneReali, AllegatoID: allArchivio, Cliente: clienteACME, Esito: confronto.EsitoRiservato, Riservata: true},
			{Percorso: "c[0]", Sezione: confronto.SezioneDaRivedere, AllegatoID: allDaRivedere, Cliente: clienteACME, Esito: confronto.EsitoDaRivedere},
		},
		NonCoperti:       2,
		Scenario:         sc,
		ProdottiScenario: []confronto.EsitoProdottoAtteso{{Base: "9123456", Esito: confronto.EsitoCorretto}},
		Baseline:         []esitoBaseline{{Percorso: "b[0]", Righe: true, Base: true}},
		SolaLetturaOk:    true, SolaLetturaMotivo: "nessun database aperto",
		NonValutati: []string{"thread senza regole"},
		Censimento:  2,
	}
}

func voce(g Gate, nome string) VoceGate {
	for _, v := range g.Voci {
		if v.Nome == nome {
			return v
		}
	}
	return VoceGate{}
}

func TestGateIncompletoSenzaLeVociEsterne(t *testing.T) {
	g := calcolaGate(ingressoGateACME())
	for nome, stato := range map[string]string{
		VoceGateFalseAssociazioni: VoceSuperata, VoceGateConteggiScenario: VoceSuperata, VoceGateDecisioniPreservate: VoceSuperata,
		VoceGateRiservati: VoceSuperata, VoceGateZeroScritture: VoceNonEseguita, VoceGateMotoreSenzaLLM: VoceNonEseguita,
		VoceGateAltriProfili: VoceNonEseguita,
	} {
		if v := voce(g, nome); v.Stato != stato {
			t.Errorf("voce %s: %q (%s), attesa %q", nome, v.Stato, v.Motivo, stato)
		}
	}
	if g.Esito != GateIncompleto || g.FalseAssociazioni != 0 || g.DecisioniPreservate != 1 || g.DaRivedere != 1 || g.NonCoperti != 2 ||
		len(g.NonValutati) != 1 || len(g.Riservati) != 1 {
		t.Fatalf("gate: %+v", g)
	}
	if len(g.PerCliente) != 1 || g.PerCliente[0].Copertura != 4 || g.PerCliente[0].Astensioni != 0 {
		t.Errorf("copertura per cliente, in numeri: %+v (la C5 e i riservati stanno fuori)", g.PerCliente)
	}
	e := esitoUscitaGate(g).controllo()
	if e.Stato != ControlloNonEseguito || e.Differenze != 0 {
		t.Errorf("con -gate un gate incompleto è NON ESEGUITO (uscita 3): %+v", e)
	}
}

func TestGateNonSuperato(t *testing.T) {
	casi := map[string]struct {
		muta func(*ingressoGate)
		voce string
	}{
		"una falsa associazione":   {func(in *ingressoGate) { in.Voci[0].Esito = confronto.EsitoFalsaAssociazione }, VoceGateFalseAssociazioni},
		"un errato nella baseline": {func(in *ingressoGate) { in.Voci[3].Esito = confronto.EsitoErrato }, VoceGateFalseAssociazioni},
		"un conteggio diverso":     {func(in *ingressoGate) { in.Voci[1].Esito = confronto.EsitoMancante }, VoceGateConteggiScenario},
		"un prodotto errato":       {func(in *ingressoGate) { in.ProdottiScenario[0].Esito = confronto.EsitoErrato }, VoceGateConteggiScenario},
		"una decisione non preservata": {func(in *ingressoGate) {
			in.Baseline = append(in.Baseline, esitoBaseline{Percorso: "b[1]", Righe: false, Base: true})
		}, VoceGateDecisioniPreservate},
		"un riservato passato":      {func(in *ingressoGate) { in.Voci[4].Esito = confronto.EsitoCorretto }, VoceGateRiservati},
		"un caso riservato passato": {func(in *ingressoGate) { in.CasiRiservatiNonRiservati = 1 }, VoceGateRiservati},
		"il collegamento può scrivere": {func(in *ingressoGate) {
			in.SolaLetturaOk, in.SolaLetturaMotivo = false, "il collegamento può scrivere"
		}, VoceGateZeroScritture},
	}
	for nome, c := range casi {
		t.Run(nome, func(t *testing.T) {
			in := ingressoGateACME()
			in.Voci = append([]esitoVoce(nil), in.Voci...)
			in.ProdottiScenario = append([]confronto.EsitoProdottoAtteso(nil), in.ProdottiScenario...)
			c.muta(&in)
			g := calcolaGate(in)
			if v := voce(g, c.voce); v.Stato != VoceNonSuperata {
				t.Fatalf("voce %s: %+v", c.voce, v)
			}
			if g.Esito != GateNonSuperato {
				t.Errorf("esito %q", g.Esito)
			}
			if e := esitoUscitaGate(g).controllo(); e.Differenze == 0 {
				t.Errorf("con -gate una voce non superata è una differenza (uscita 1): %+v", e)
			}
		})
	}
}

// TestGateFuoriDalGate: la C5, i non coperti e i riservati non entrano nel gate; un file in due sezioni conta una volta,
// con la prima sezione (D7); senza voci dello scenario e della baseline la voce delle false associazioni non è eseguita
// (mai un «superato» per vuoto).
func TestGateFuoriDalGate(t *testing.T) {
	in := ingressoGateACME()
	in.Voci = append(in.Voci,
		esitoVoce{Percorso: "c[1]", Sezione: confronto.SezioneDaRivedere, AllegatoID: allFuoriRFQ, Cliente: clienteACME, Esito: confronto.EsitoErrato},
		esitoVoce{Percorso: "b[9]", Sezione: confronto.SezioneBaseline, AllegatoID: allRadice, Cliente: clienteACME, Esito: confronto.EsitoErrato})
	g := calcolaGate(in)
	if g.FalseAssociazioni != 0 || voce(g, VoceGateFalseAssociazioni).Stato != VoceSuperata {
		t.Errorf("un errato nella C5, o su un file già contato nello scenario, non entra: %+v", g)
	}
	vuoto := calcolaGate(ingressoGate{SolaLetturaOk: true})
	if v := voce(vuoto, VoceGateFalseAssociazioni); v.Stato != VoceNonEseguita {
		t.Errorf("senza voci: %+v", v)
	}
	if v := voce(vuoto, VoceGateDecisioniPreservate); v.Stato != VoceNonEseguita {
		t.Errorf("senza baseline: %+v", v)
	}
	if v := voce(vuoto, VoceGateConteggiScenario); v.Stato != VoceNonEseguita {
		t.Errorf("senza conteggi: %+v", v)
	}
	if vuoto.Esito != GateIncompleto {
		t.Errorf("esito %q", vuoto.Esito)
	}
}

// TestGateNelBanco: con -attesi il gate è nel rapporto; senza -gate non decide l'uscita; con -gate un gate incompleto
// dà 3 e uno non superato 1 (R44). Sulla scena ACME il motore non dà i prodotti dello scenario (la grammatica non legge
// la storia), quindi i conteggi dichiarati non tornano: senza i conteggi la voce non è eseguita.
func TestGateNelBanco(t *testing.T) {
	s := preparaBanco(t, mutaBanco{})
	r, err := EseguiBanco(context.Background(), s.opzioniExport(true), nil)
	if err != nil || r.Gate == nil || r.Gate.Esito != GateNonSuperato || r.Esito.CodiceUscita() != UscitaNonEseguito || r.Differenze != 0 ||
		!strings.HasSuffix(r.PrimaRiga(), "; gate non_superato (senza -gate non decide l'uscita)") {
		t.Fatalf("senza -gate il gate non decide l'uscita, e la prima riga lo dice: %s, gate %+v (%v)", r.PrimaRiga(), r.Gate, err)
	}
	o := s.opzioniExport(true)
	o.Gate = true
	r, err = EseguiBanco(context.Background(), o, nil)
	if err != nil || r.Esito.CodiceUscita() != UscitaConDifferenze {
		t.Fatalf("con -gate un gate non superato dà 1: %s (%v)", r.PrimaRiga(), err)
	}
	senzaConteggi := preparaBanco(t, mutaBanco{testi: map[string]func(string) string{"attesi": func(a string) string {
		return strings.Replace(a, "  conteggi:\n    radice: 1\n    figlio: 1\n    fuori_scenario: 1\n    file: 3\n    prodotti: 1\n", "", 1)
	}}})
	o = senzaConteggi.opzioniExport(true)
	o.Gate = true
	r, err = EseguiBanco(context.Background(), o, nil)
	if err != nil || r.Gate.Esito != GateIncompleto || r.Esito.CodiceUscita() != UscitaNonEseguito ||
		!strings.HasPrefix(r.PrimaRiga(), "ESITO: NON ESEGUITO — ") || !strings.Contains(r.PrimaRiga(), "; gate: incompleto") {
		t.Fatalf("con -gate un gate incompleto dà 3: %s, %+v (%v)", r.PrimaRiga(), r.Gate, err)
	}
}
