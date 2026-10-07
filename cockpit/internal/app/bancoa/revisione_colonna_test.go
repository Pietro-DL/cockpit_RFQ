package bancoa

import (
	"strings"
	"testing"

	"promatec/cockpit/internal/core/confronto"
	valut "promatec/cockpit/internal/core/valutazione"
)

// L1 — R113 B ratificata, la parte del banco (E2 §2.6 e §3.4; revisione d'impatto, R113 §3 b): la revisione del vecchio
// motore interpretata da valutazione arriva al banco con la sua provenienza (il gemello RevisioneDa, copiato da
// passaggio.go), e il banco
//   - conta la revisione vecchia solo nella colonna rev divisa per lo stato della lettura della colonna (letta, nessuna
//     regola, non interpretabile, ambigua; e a parte i file il cui stato non è nel record);
//   - conta, nella voce del gate delle decisioni preservate, le differenze di revisione diagnosticate sui file della
//     baseline (le revisioni diverse e le revisioni vecchie discordi), senza togliere la preservazione: l'indicatore di
//     revisione resta separato dalla correttezza dell'associazione.
//
// I clienti di questi test sono inventati: vedi scena_banco_test.go.

// fileColonnaACME: una riga di confronto di un file valutato, con la provenienza della revisione vecchia, la colonna rev
// e il motivo delle revisioni.
func fileColonnaACME(revisioneDa, rev, motivo string, valutato bool) FileBanco {
	v := confronto.Vecchio{Fonte: "nome_file", Codice: "9123456A", Rev: rev, Base: "9123456", Leggibile: true, RevisioneDa: revisioneDa}
	if revisioneDa != "" {
		v.Revisione = "2"
	}
	return FileBanco{Riga: confronto.EsitoFile{File: confronto.File{Vecchio: v, Nuovo: confronto.Nuovo{Valutato: valutato, MotivoRevisioni: motivo}}}}
}

// TestRispostaR113LaColonnaPerStato: il contatore della colonna si divide per stato, con i motivi di valutazione
// (T-B6-202) e la provenienza «colonna»; un file non valutato, con il codice leggibile, la colonna piena e nessuna
// revisione interpretata ha lo stato fuori dal record piatto, e si conta a parte invece di sparire. Non contano: la
// revisione dal codice, le revisioni vecchie discordi, la colonna vuota, il codice che non si legge.
func TestRispostaR113LaColonnaPerStato(t *testing.T) {
	nonLeggibile := fileColonnaACME("", "2", valut.MotivoRevisioniVecchiaNonLetta, true)
	nonLeggibile.Riga.File.Vecchio.Leggibile = false
	nonLeggibileNonValutato := fileColonnaACME("", "2", valut.MotivoRevisioniNuovoNonValutato, false)
	nonLeggibileNonValutato.Riga.File.Vecchio.Leggibile = false
	thread := []ThreadBanco{{ClienteID: clienteACME, Valutato: true, File: []FileBanco{
		fileColonnaACME(valut.RevisioneDaColonna, "2", "", true),
		fileColonnaACME(valut.RevisioneDaColonna, "2", valut.MotivoRevisioniNuovoNonValutato, false),
		fileColonnaACME("", "2", valut.MotivoRevisioniSoloInColonna, true),
		fileColonnaACME("", "2", valut.MotivoRevisioniColonnaNonInterpretabile, true),
		fileColonnaACME("", "2", valut.MotivoRevisioniColonnaNonInterpretabile, true),
		fileColonnaACME("", "2", valut.MotivoRevisioniColonnaAmbigua, true),
		fileColonnaACME("", "2", valut.MotivoRevisioniNuovoNonValutato, false),
		// non contano
		fileColonnaACME(valut.RevisioneDaCodice, "2", "", true),
		fileColonnaACME(valut.RevisioneDaCodice, "3", valut.MotivoRevisioniVecchiaDiscorde, true),
		fileColonnaACME("", " ", valut.MotivoRevisioniNuovoNonValutato, false),
		fileColonnaACME("", "", valut.MotivoRevisioniVecchiaNonLetta, true),
		nonLeggibile, nonLeggibileNonValutato,
	}}}
	c := correzioniPerCliente(thread)
	if len(c) != 1 {
		t.Fatalf("un cliente: %+v", c)
	}
	atteso := RevisioneInColonna{Letta: 2, NessunaRegola: 1, NonInterpretabile: 2, Ambigua: 1, StatoNonNelRecord: 1}
	if c[0].RevisioneSoloInColonna != atteso {
		t.Errorf("la colonna per stato: %+v, attesa %+v", c[0].RevisioneSoloInColonna, atteso)
	}
	if s := testoCorrezioni(c[0]); !strings.Contains(s, "revisione solo in colonna 7 (letta 2, nessuna regola 1, non interpretabile 2, ambigua 1, stato non nel record 1)") {
		t.Errorf("il riepilogo:\n%s", s)
	}
}

// TestRispostaR113LeRevisioniDellaBaseline: la voce del gate delle decisioni preservate conta le differenze di
// revisione diagnosticate sui file della baseline (diverse e revisioni vecchie discordi), e a parte le uguali, le altre
// non determinabili e le voci senza riga; nessuna toglie la preservazione, con o senza differenze di revisione.
func TestRispostaR113LeRevisioniDellaBaseline(t *testing.T) {
	ind := func(valore, motivo string) *confronto.IndicatoreRevisione {
		return &confronto.IndicatoreRevisione{Valore: valore, Motivo: motivo}
	}
	in := ingressoGateACME()
	in.Baseline = []esitoBaseline{
		{Percorso: "b[0]", Righe: true, Base: true, Revisione: ind(confronto.RevisioneDiversa, "")},
		{Percorso: "b[1]", Righe: true, Base: true, Revisione: ind(confronto.RevisioneNonDeterminabile, valut.MotivoRevisioniVecchiaDiscorde)},
		{Percorso: "b[2]", Righe: true, Base: true, Revisione: ind(confronto.RevisioneUguale, "")},
		{Percorso: "b[3]", Righe: true, Base: true, Revisione: ind(confronto.RevisioneUguale, valut.MotivoRevisioniEquivalente)},
		{Percorso: "b[4]", Righe: true, Base: true, Revisione: ind(confronto.RevisioneNonDeterminabile, valut.MotivoRevisioniSoloInColonna)},
		{Percorso: "b[5]", Righe: true, Base: true},
	}
	g := calcolaGate(in)
	atteso := RevisioniDellaBaseline{Diagnosticate: 2, Diverse: 1, VecchieDiscordi: 1, Uguali: 2, NonDeterminabili: 1, SenzaRiga: 1}
	if g.RevisioniBaseline != atteso {
		t.Errorf("le revisioni della baseline: %+v, attese %+v", g.RevisioniBaseline, atteso)
	}
	v := voce(g, VoceGateDecisioniPreservate)
	if v.Stato != VoceSuperata || g.DecisioniPreservate != 6 ||
		!strings.Contains(v.Motivo, "6 su 6; differenze di revisione diagnosticate 2 (diverse 1, revisioni vecchie discordi 1), uguali 2, non determinabili 1, senza riga 1") {
		t.Errorf("le differenze di revisione non tolgono la preservazione, e la voce le dice: %+v, %d", v, g.DecisioniPreservate)
	}
	// Con una decisione non preservata la voce non è superata, e le revisioni restano nel motivo.
	in.Baseline = append(in.Baseline, esitoBaseline{Percorso: "b[6]", Righe: false, Base: true})
	if v := voce(calcolaGate(in), VoceGateDecisioniPreservate); v.Stato != VoceNonSuperata || !strings.Contains(v.Motivo, "differenze di revisione diagnosticate 2") {
		t.Errorf("con una decisione non preservata: %+v", v)
	}
}

// TestRispostaR113LaProvenienzaArrivaAllaBaseline: la provenienza della revisione vecchia passa da valutazione a
// confronto (passaggio.go) e arriva all'indicatore di revisione che il banco mette nella baseline: una revisione
// vecchia letta nella colonna e diversa da quella del file è una differenza diagnosticata, con la provenienza
// «colonna», e conta nel contatore della colonna come «letta». Senza la copia del campo, la provenienza sparirebbe e il
// file conterebbe come «nessuna regola» o non conterebbe.
func TestRispostaR113LaProvenienzaArrivaAllaBaseline(t *testing.T) {
	doc := uidACME(0x221)
	f := valut.FileConfrontabile{AllegatoID: allBaseline,
		Vecchio: valut.VecchioPiatto{Stato: "confermata", Fonte: "nome_file", Codice: "9123456A", Rev: "2", Base: "9123456", Leggibile: true,
			Revisione: "2", RevisioneDa: valut.RevisioneDaColonna, Documento: &doc},
		Nuovo: valut.NuovoPiatto{Valutato: true, Basi: []string{"9123456"}, Revisione: "3", Revisioni: valut.RevisioniDiverse}}
	e := confronto.Confronta(inFiles([]valut.FileConfrontabile{f}), nil, nil)
	r := e.File[0].Revisione
	if r.Valore != confronto.RevisioneDiversa || r.ProvenienzaVecchia != valut.RevisioneDaColonna || r.Vecchia != "2" || r.Nuova != "3" {
		t.Fatalf("l'indicatore: %+v", r)
	}
	if got := revisioniDellaBaseline([]esitoBaseline{{Revisione: &r}}); got.Diagnosticate != 1 || got.Diverse != 1 {
		t.Errorf("la differenza diagnosticata: %+v", got)
	}
	fb := FileBanco{AllegatoID: allBaseline, Riga: e.File[0]}
	c := correzioniPerCliente([]ThreadBanco{{ClienteID: clienteACME, Valutato: true, File: []FileBanco{fb}}})
	if c[0].RevisioneSoloInColonna != (RevisioneInColonna{Letta: 1}) {
		t.Errorf("la colonna letta: %+v", c[0].RevisioneSoloInColonna)
	}
}

// TestRispostaR113SullaScena: sulla scena ACME la famiglia del codice deciso della baseline non ha una regola in campo
// separato (nessuna_regola): la baseline porta l'indicatore di revisione del suo file, il gate lo conta fra i non
// determinabili e nessuna differenza è diagnosticata; il contatore della colonna dice «nessuna regola».
func TestRispostaR113SullaScena(t *testing.T) {
	r := corsaBanco(t, mutaBanco{}, true, false)
	if r.Attesi == nil || len(r.Attesi.Baseline) != 1 || r.Attesi.Baseline[0].Revisione == nil ||
		r.Attesi.Baseline[0].Revisione.Motivo != valut.MotivoRevisioniSoloInColonna {
		t.Fatalf("la baseline con l'indicatore di revisione: %+v", r.Attesi)
	}
	if r.Gate.RevisioniBaseline != (RevisioniDellaBaseline{NonDeterminabili: 1}) ||
		!strings.Contains(voce(*r.Gate, VoceGateDecisioniPreservate).Motivo, "differenze di revisione diagnosticate 0") {
		t.Errorf("il gate: %+v, %+v", r.Gate.RevisioniBaseline, voce(*r.Gate, VoceGateDecisioniPreservate))
	}
	if len(r.CorrezioniManuali) != 1 || r.CorrezioniManuali[0].RevisioneSoloInColonna.NessunaRegola == 0 ||
		r.CorrezioniManuali[0].RevisioneSoloInColonna.Letta != 0 {
		t.Errorf("la colonna sulla scena: %+v", r.CorrezioniManuali)
	}
	if r.Versioni.RevisioneRegistrata == "" || !strings.Contains(r.Testo(), ", "+r.Versioni.RevisioneRegistrata+", ") {
		t.Errorf("la versione della lettura della colonna fra le versioni: %+v", r.Versioni)
	}
}
