// L1 — la regola della conferma dopo la direttiva 12 (13/10/2026: D-ADD-02 = B [U], ratifica con correzioni
// dell'addendum ad A1c; addendum rev. 1 §8, P-01…P-11; direttiva 12 §10, le prove minime, parte pura). Scenari A
// (STEP non analizzabile), B (mono_pezzo senza gerarchia), C (STEP secondario revisionato), E (provenienza degli
// archi), più le condizioni nuove di AD-4c (da_riesaminare per una BOM già confermata, non_pronto per una BOM senza i
// requisiti della prima conferma) e la base della gerarchia. Ingressi sintetici ACME, codici inventati, nessun DB. Le
// prove L4 e nel browser degli stessi scenari sono dei passi B3.1, B3.5, B3.6, B5 (addendum §9) e qui non ci sono.
package valutazione_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

// ---- la scena dello STEP che non dà nodi ----

// motivoNonAnalizzatoACM: il motivo parziale che v_step_prodotto (0020) dà a uno STEP senza un'analisi corrente.
const motivoNonAnalizzatoACM = "nessuna analisi corrente dello STEP"

var (
	docNuovoACM = uid(0x9c06)
	shaNuovoACM = sha("e")
	derogaNuova = uid(0x9c07)
)

// fonteSenzaAnalisiACM: la fonte confermata con lo STEP autorizzato senza un'analisi corrente: la chiave d'analisi è
// vuota, come la rappresenta la 0020 (le tre colonne NULL).
func fonteSenzaAnalisiACM() *valutazione.FonteDellaVerifica {
	return &valutazione.FonteDellaVerifica{DocumentoID: docFonteACM, Sha256: shaFonteACM}
}

// strutturaSenzaNodiACM: la struttura di un prodotto il cui STEP non dà nodi: nessuna BOM di lavoro, nessun nodo.
func strutturaSenzaNodiACM(fonte bool) valutazione.StrutturaDaVerificare {
	return valutazione.StrutturaDaVerificare{ConComponente: true, FonteConfermata: fonte}
}

// derogaNonAnalizzatoACM: la deroga 0020 dello STEP non analizzato: la chiave d'analisi vuota, il motivo parziale com'era,
// chi e quando.
func derogaNonAnalizzatoACM() valutazione.DerogaStruttura {
	return valutazione.DerogaStruttura{ID: derogaACM, StepDocumentoID: docFonteACM, StepSha256: shaFonteACM,
		Motivo: "lo STEP non si analizza: la distinta viene dal disegno d'assieme ACM-9000", Stato: valutazione.StatoDerogaValida,
		MotivoParziale: motivoNonAnalizzatoACM, ConcessaDa: operatore, ConcessaIl: "2026-10-12T20:30:00Z"}
}

// scenarioAACM: lo scenario A della direttiva 12 §10: lo STEP corrente autorizzato e senza analisi, la gerarchia
// ricostruita a mano (13 figli con le loro qta, archi decisi da una persona), la deroga valida, la conferma coerente.
func scenarioAACM() valutazione.IngressoVerificaConferma {
	c := confermaManualeACM(qtaACM)
	return valutazione.IngressoVerificaConferma{Conferma: c, Struttura: strutturaSenzaNodiACM(true), ArchiCorrenti: correntiDi(c),
		Fonte: fonteSenzaAnalisiACM(), Analisi: valutazione.AnalisiStruttura{MotivoParziale: motivoNonAnalizzatoACM},
		Deroghe: []valutazione.DerogaStruttura{derogaNonAnalizzatoACM()}}
}

// diagnosticaNonAnalizzabile: il fatto che lo STEP non sia analizzabile resta visibile (direttiva 12 §2.1 [U]: «Non
// eliminare la diagnostica»): gerarchia_step e il motivo parziale, intatto.
func diagnosticaNonAnalizzabile(t *testing.T, nome string, v valutazione.VerificaBOM, motivo string) {
	t.Helper()
	if v.GerarchiaStep != valutazione.GerarchiaStepNonAnalizzabile || v.Conferma == nil || v.Conferma.MotivoParzialeStep != motivo {
		t.Errorf("%s: la diagnostica dello STEP non analizzabile non è visibile: gerarchia_step %s, dettaglio %+v", nome, v.GerarchiaStep, v.Conferma)
	}
}

// ---- scenario A: STEP non analizzabile ----

// TestScenarioAStepNonAnalizzabile (direttiva 12 §2.1 e §10, scenario A [U]; addendum ad A1c AD-1, AD-4, §8 P-01, P-02,
// P-03): con lo STEP autorizzato che non si analizza, l'ingegnere ricostruisce la gerarchia a mano e registra una deroga
// motivata: la verifica positiva è ammessa, e dichiara che la gerarchia è ricostruita a mano (mai «dallo STEP»). Senza
// deroga, con la deroga di un altro STEP, scaduta, di un'altra analisi o senza motivo, con un conflitto aperto, con la
// struttura cambiata o con una riga secondaria aperta non è positiva. La diagnostica resta sempre visibile.
func TestScenarioAStepNonAnalizzabile(t *testing.T) {
	v := valutazione.VerificaDallaConferma(scenarioAACM())
	if !v.Verificata || asse(v.Nomenclatura) != "verificata/" || asse(v.Gerarchia) != "verificata/" || v.Criterio != valutazione.CriterioStep ||
		!v.FonteRegistrata || v.Nomenclatura.Legacy || v.Gerarchia.Legacy || v.Nomenclatura.DaDecidere != 0 || v.Gerarchia.DaDecidere != 0 {
		t.Fatalf("scenario A, deroga valida: %s, BOM %+v", esitoConferma(v), v)
	}
	diagnosticaNonAnalizzabile(t, "scenario A, deroga valida", v, motivoNonAnalizzatoACM)
	d := v.Conferma
	if d.BaseGerarchia != valutazione.BaseGerarchiaRicostruitaAMano || v.OrigineGerarchia != valutazione.OrigineGerarchiaManuale ||
		len(d.Cambiamenti) != 0 || d.RadiceMappata {
		t.Errorf("scenario A: base %q, origine %q, dettaglio %+v", d.BaseGerarchia, v.OrigineGerarchia, d)
	}
	// la deroga si vede anche con la verifica positiva, con lo stato dell'analisi derogata e chi e quando ([P] P-03)
	if d.Deroga == nil || d.Deroga.Stato != valutazione.DerogaApplicata || d.Deroga.ID == nil || *d.Deroga.ID != derogaACM ||
		d.Deroga.MotivoParziale != motivoNonAnalizzatoACM || d.Deroga.ConcessaDa == nil || *d.Deroga.ConcessaDa != operatore || d.Deroga.ConcessaIl == "" {
		t.Errorf("scenario A: deroga esposta %+v", d.Deroga)
	}

	// con un'analisi corrente la cui struttura non si legge (presente_parziale della 0020) la deroga porta la chiave vera
	illeggibile := scenarioAACM()
	illeggibile.Fonte.Analisi, illeggibile.Analisi.MotivoParziale = analisiACM, "struttura v2: versione non letta"
	illeggibile.Deroghe[0].Analisi, illeggibile.Deroghe[0].MotivoParziale = analisiACM, "struttura v2: versione non letta"
	if v := valutazione.VerificaDallaConferma(illeggibile); !v.Verificata || v.Conferma.Deroga.Stato != valutazione.DerogaApplicata {
		t.Errorf("scenario A, struttura illeggibile con la deroga di quell'analisi: %s, %+v", esitoConferma(v), v.Conferma.Deroga)
	} else {
		diagnosticaNonAnalizzabile(t, "scenario A, struttura illeggibile", v, "struttura v2: versione non letta")
	}

	scaduta := derogaNonAnalizzatoACM()
	scaduta.Stato = valutazione.StatoDerogaSuperata
	altroDoc := derogaNonAnalizzatoACM()
	altroDoc.StepDocumentoID = docAltroACM
	senzaMotivo := derogaNonAnalizzatoACM()
	senzaMotivo.Motivo = " "
	conflitto := []valutazione.Conflitto{{Tipo: valutazione.ConflittoArco, Asse: valutazione.AsseGerarchia,
		Rif: valutazione.RifArco(rifC(finitoACM), rifC(figlioACM(2))), Motivo: "qta decisa contro il disegno"}}
	m := func(s string) string { return "da_verificare/" + s }
	for _, c := range []struct {
		nome         string
		cambia       func(*valutazione.IngressoVerificaConferma)
		nomenclatura string
		gerarchia    string
		deroga       string // "" = deroga non valutata
	}{
		{"senza deroga", func(in *valutazione.IngressoVerificaConferma) { in.Deroghe = nil },
			"verificata/", m(valutazione.MotivoAsseDerogaAssente), valutazione.DerogaAssente},
		{"deroga senza motivazione", func(in *valutazione.IngressoVerificaConferma) {
			in.Deroghe = []valutazione.DerogaStruttura{senzaMotivo}
		}, "verificata/", m(valutazione.MotivoAsseDerogaSenzaMotivo), valutazione.DerogaSenzaMotivo},
		{"deroga di un altro STEP", func(in *valutazione.IngressoVerificaConferma) {
			in.Deroghe = []valutazione.DerogaStruttura{altroDoc}
		}, "verificata/", m(valutazione.MotivoAsseDerogaAltroStep), valutazione.DerogaAltroStep},
		{"deroga superata secondo la 0020", func(in *valutazione.IngressoVerificaConferma) {
			in.Deroghe = []valutazione.DerogaStruttura{scaduta}
		}, "verificata/", m(valutazione.MotivoAsseDerogaScaduta), valutazione.DerogaScaduta},
		// la prima analisi fa decadere la deroga dello STEP non analizzato (0020): qui l'analisi c'è ma la struttura non si
		// legge, quindi gerarchia_step resta non_analizzabile e la deroga con la chiave vuota non vale più
		{"prima analisi dopo la deroga dello STEP non analizzato", func(in *valutazione.IngressoVerificaConferma) {
			in.Fonte.Analisi, in.Analisi.MotivoParziale = analisiACM, "struttura non letta"
		}, "verificata/", m(valutazione.MotivoAsseDerogaAltraAnalisi), valutazione.DerogaAltraAnalisi},
		{"conflitto strutturale aperto, anche con la deroga", func(in *valutazione.IngressoVerificaConferma) { in.Struttura.Conflitti = conflitto },
			"verificata/", "conflitto/conflitto", valutazione.DerogaApplicata},
		{"un figlio aggiunto dopo la conferma", func(in *valutazione.IngressoVerificaConferma) {
			in.ArchiCorrenti = append(in.ArchiCorrenti, valutazione.ArcoCorrente{PadreID: finitoACM, FiglioID: figlioACM(20), Qta: 1})
		}, m(valutazione.MotivoAsseStrutturaCambiata), m(valutazione.MotivoAsseStrutturaCambiata), valutazione.DerogaApplicata},
		{"una riga aperta di uno STEP secondario", func(in *valutazione.IngressoVerificaConferma) {
			in.SecondarieAperte = []valutazione.RigaSecondariaAperta{{Rif: ancoraggio.RifNodo(sha("d"), "#7")}}
		}, m(valutazione.MotivoAsseDaDecidere), m(valutazione.MotivoAsseDaDecidere), valutazione.DerogaApplicata},
	} {
		in := scenarioAACM()
		c.cambia(&in)
		v := valutazione.VerificaDallaConferma(in)
		if v.Verificata || asse(v.Nomenclatura) != c.nomenclatura || asse(v.Gerarchia) != c.gerarchia {
			t.Errorf("scenario A, %s: %s", c.nome, esitoConferma(v))
		}
		switch {
		case v.Conferma.Deroga != nil && v.Conferma.Deroga.Stato != c.deroga:
			t.Errorf("scenario A, %s: deroga %q, attesa %q", c.nome, v.Conferma.Deroga.Stato, c.deroga)
		case v.Conferma.Deroga == nil && c.deroga != "":
			t.Errorf("scenario A, %s: deroga non valutata, attesa %q", c.nome, c.deroga)
		}
		diagnosticaNonAnalizzabile(t, "scenario A, "+c.nome, v, in.Analisi.MotivoParziale)
		if v.Conferma.BaseGerarchia != valutazione.BaseGerarchiaRicostruitaAMano {
			t.Errorf("scenario A, %s: base %q", c.nome, v.Conferma.BaseGerarchia)
		}
	}

	// uno STEP che riconosce la gerarchia ma arriva senza nodi: l'ingresso non torna, il perimetro certificato non lo
	// sostituisce e resta nessuna_struttura (guardia prudente [T] di P-02)
	in := scenarioAACM()
	in.Fonte.Analisi = analisiACM
	in.Analisi = valutazione.AnalisiStruttura{Letta: true, Versione: 3, Radici: 1, Relazioni: 13}
	in.Deroghe = nil
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Gerarchia) != "da_verificare/"+valutazione.MotivoAsseNessunaStruttura ||
		asse(v.Nomenclatura) != "da_verificare/"+valutazione.MotivoAsseNessunaStruttura {
		t.Errorf("STEP riconosciuto senza nodi: %s", esitoConferma(v))
	}
}

// TestScenarioAShaCambiato (direttiva 12 §2.1 e §10 [U]: «Con SHA cambiato: la vecchia deroga non autorizza
// automaticamente la nuova situazione»; addendum AD-4, tabella della deroga): il nuovo STEP autorizzato è un altro
// documento con un altro sha. La conferma n, che certificava lo STEP di prima, non vale più (la firma comprende lo STEP
// della radice: lo dice il chiamante); se il chiamante la dicesse valida, la radice certificata non porta lo STEP della
// fonte. Confermato di nuovo sullo STEP nuovo (n+1), la deroga vecchia è di un altro STEP: serve la sua. Prima del gesto 3
// sullo STEP nuovo vale K-02 (caso B).
func TestScenarioAShaCambiato(t *testing.T) {
	nuovaFonte := func(in *valutazione.IngressoVerificaConferma) {
		in.Fonte = &valutazione.FonteDellaVerifica{DocumentoID: docNuovoACM, Sha256: shaNuovoACM}
	}
	m := func(s string) string { return "da_verificare/" + s }

	in := scenarioAACM()
	nuovaFonte(&in)
	in.Conferma.Valida = false
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Nomenclatura) != m(valutazione.MotivoAsseConfermaNonValida) ||
		asse(v.Gerarchia) != m(valutazione.MotivoAsseConfermaNonValida) || v.Criterio != valutazione.CriterioNessuno || v.Conferma.BaseGerarchia != "" {
		t.Errorf("sha cambiato, conferma n non più valida: %s, base %q", esitoConferma(v), v.Conferma.BaseGerarchia)
	}

	in = scenarioAACM()
	nuovaFonte(&in)
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Nomenclatura) != m(valutazione.MotivoAsseRadiceNonMappata) ||
		asse(v.Gerarchia) != m(valutazione.MotivoAsseDerogaAltroStep) {
		t.Errorf("sha cambiato, conferma n detta valida con la radice dello STEP di prima: %s", esitoConferma(v))
	}

	// la conferma n+1 sullo STEP nuovo: la radice certificata porta il documento e lo sha nuovi
	in = scenarioAACM()
	nuovaFonte(&in)
	d := docNuovoACM
	in.Conferma.Numero, in.Conferma.Componenti[0].StepStrutturaleID, in.Conferma.Componenti[0].StepSha256 = 2, &d, shaNuovoACM
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Nomenclatura) != "verificata/" ||
		asse(v.Gerarchia) != m(valutazione.MotivoAsseDerogaAltroStep) || v.Conferma.Deroga.Stato != valutazione.DerogaAltroStep {
		t.Errorf("conferma n+1 sullo STEP nuovo con la deroga vecchia: %s, %+v", esitoConferma(v), v.Conferma.Deroga)
	}
	// con i dati veri la deroga vecchia arriva superata (B3.1 dà valida solo alla deroga della vista: addendum AD-4,
	// «Lo stato della deroga e il motivo che esce»): lo STEP si controlla prima dello stato, ed esce altro_step, mai
	// scaduta (revisione rev. 2, M-1)
	for nome, cambia := range map[string]func(*valutazione.DerogaStruttura){
		"documento e sha di prima":       func(*valutazione.DerogaStruttura) {},
		"stesso documento, sha di prima": func(d *valutazione.DerogaStruttura) { d.StepDocumentoID = docNuovoACM },
		"altro documento, stesso sha":    func(d *valutazione.DerogaStruttura) { d.StepSha256 = shaNuovoACM },
	} {
		vecchia := derogaNonAnalizzatoACM()
		vecchia.Stato = valutazione.StatoDerogaSuperata
		cambia(&vecchia)
		n1 := in
		n1.Deroghe = []valutazione.DerogaStruttura{vecchia}
		if v := valutazione.VerificaDallaConferma(n1); v.Verificata || asse(v.Nomenclatura) != "verificata/" ||
			asse(v.Gerarchia) != m(valutazione.MotivoAsseDerogaAltroStep) || v.Conferma.Deroga == nil ||
			v.Conferma.Deroga.Stato != valutazione.DerogaAltroStep {
			t.Errorf("conferma n+1 sullo STEP nuovo con la deroga vecchia superata, %s: %s, %+v", nome, esitoConferma(v), v.Conferma.Deroga)
		}
	}
	nuova := derogaNonAnalizzatoACM()
	nuova.ID, nuova.StepDocumentoID, nuova.StepSha256 = derogaNuova, docNuovoACM, shaNuovoACM
	in.Deroghe = append([]valutazione.DerogaStruttura{nuova}, in.Deroghe...)
	if v := valutazione.VerificaDallaConferma(in); !v.Verificata || *v.Conferma.Deroga.ID != derogaNuova {
		t.Errorf("conferma n+1 con la deroga dello STEP nuovo: %s, %+v", esitoConferma(v), v.Conferma.Deroga)
	}
}

// TestScenarioACasoBFonteNonAutorizzata (direttiva 12 §2.1, caso B [U]: «La conferma umana non elimina il vincolo K-02
// sulla fonte»): con lo STEP assente, non identificabile o non autorizzato come fonte corrente la BOM ricostruita a mano
// non è positiva, la deroga non si valuta (è una proprietà della fonte confermata, T-13) e la base della gerarchia è
// vuota: il file non diventa una fonte perché l'operatore ha costruito una BOM.
func TestScenarioACasoBFonteNonAutorizzata(t *testing.T) {
	for nome, cambia := range map[string]func(*valutazione.IngressoVerificaConferma){
		"fonte superata o revocata": func(in *valutazione.IngressoVerificaConferma) { in.Struttura = strutturaSenzaNodiACM(false) },
		"nessuno STEP autorizzato": func(in *valutazione.IngressoVerificaConferma) {
			in.Struttura, in.Fonte = strutturaSenzaNodiACM(false), nil
		},
	} {
		in := scenarioAACM()
		cambia(&in)
		v := valutazione.VerificaDallaConferma(in)
		if v.Verificata || asse(v.Gerarchia) != "da_verificare/"+valutazione.MotivoAsseFonteNonConfermata || v.FonteRegistrata ||
			v.Criterio != valutazione.CriterioNessuno || v.Conferma.Deroga != nil || v.Conferma.BaseGerarchia != "" {
			t.Errorf("caso B, %s: %s, deroga %+v, base %q", nome, esitoConferma(v), v.Conferma.Deroga, v.Conferma.BaseGerarchia)
		}
	}
}

// ---- scenario B: mono_pezzo senza gerarchia analizzabile ----

// TestScenarioBMonoPezzoSenzaGerarchia (direttiva 12 §2.1 «Caso mono_pezzo» e §10, scenario B [U]; addendum AD-3, §8
// P-02): con lo STEP autorizzato registrato sulla radice finito, la dichiarazione motivata e nessun figlio, la BOM è
// verificata anche quando lo STEP non dà una gerarchia (senza analisi, o «solo parti»), senza obbligo di deroga. Un
// figlio aggiunto dopo toglie la verifica finché la struttura non viene riesaminata. La dichiarazione non si deduce
// dall'assenza di figli; K-02 resta.
func TestScenarioBMonoPezzoSenzaGerarchia(t *testing.T) {
	ingresso := func() valutazione.IngressoVerificaConferma {
		return valutazione.IngressoVerificaConferma{Conferma: monoACM(), Struttura: strutturaSenzaNodiACM(true), Fonte: fonteSenzaAnalisiACM(),
			Analisi: valutazione.AnalisiStruttura{MotivoParziale: motivoNonAnalizzatoACM}}
	}
	v := valutazione.VerificaDallaConferma(ingresso())
	if !v.Verificata || v.Criterio != valutazione.CriterioMonoPezzo || v.Conferma.Deroga != nil || v.Conferma.BaseGerarchia != valutazione.BaseGerarchiaMonoPezzo ||
		v.OrigineGerarchia != "" || !v.SenzaFigli || !v.FonteRegistrata || v.Conferma.MonoPezzoMotivo == "" {
		t.Errorf("scenario B, STEP non analizzabile: %s, %+v", esitoConferma(v), v.Conferma)
	}
	diagnosticaNonAnalizzabile(t, "scenario B", v, motivoNonAnalizzatoACM)

	// lo STEP letto «solo parti», senza nodi nella struttura: verificata lo stesso, nessuna deroga
	in := ingresso()
	in.Fonte, in.Analisi = fonteACM(), analisiSoloPartiACM()
	if v := valutazione.VerificaDallaConferma(in); !v.Verificata || v.Conferma.Deroga != nil || v.GerarchiaStep != valutazione.GerarchiaStepNonRiconosciuta {
		t.Errorf("scenario B, STEP solo parti senza nodi: %s, %s", esitoConferma(v), v.GerarchiaStep)
	}

	figlio := []valutazione.ArcoCorrente{{PadreID: finitoACM, FiglioID: figlioACM(0), Qta: 1}}
	aggiunto := []string{"aggiunto:" + valutazione.RifArco(rifC(finitoACM), rifC(figlioACM(0)))}
	in = ingresso()
	in.ArchiCorrenti = figlio
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Gerarchia) != "da_verificare/"+valutazione.MotivoAsseMonoPezzoDecaduto ||
		asse(v.Nomenclatura) != "da_verificare/"+valutazione.MotivoAsseMonoPezzoDecaduto || !reflect.DeepEqual(v.Conferma.Cambiamenti, aggiunto) {
		t.Errorf("scenario B, figlio aggiunto con la conferma valida: %s, %v", esitoConferma(v), v.Conferma.Cambiamenti)
	}
	// con i dati veri il primo arco aggiunto cambia contatore e firma: la conferma non vale, e il cambiamento si vede
	in.Conferma.Valida = false
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Gerarchia) != "da_verificare/"+valutazione.MotivoAsseConfermaNonValida ||
		!reflect.DeepEqual(v.Conferma.Cambiamenti, aggiunto) {
		t.Errorf("scenario B, figlio aggiunto con la conferma non più valida: %s, %v", esitoConferma(v), v.Conferma.Cambiamenti)
	}

	in = ingresso()
	in.Struttura = strutturaSenzaNodiACM(false)
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Gerarchia) != "da_verificare/"+valutazione.MotivoAsseFonteNonConfermata {
		t.Errorf("scenario B, senza la fonte confermata: %s", esitoConferma(v))
	}

	// la radice certificata con lo STEP di prima, la conferma detta valida dal chiamante
	in = ingresso()
	in.Conferma.Componenti[0].StepSha256 = sha("b")
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Nomenclatura) != "da_verificare/"+valutazione.MotivoAsseRadiceNonMappata {
		t.Errorf("scenario B, radice certificata con un altro STEP: %s", esitoConferma(v))
	}

	// senza dichiarazione: mai un mono_pezzo dall'assenza di figli, e il motivo non è più nessuna_struttura
	senza := confermaManualeACM(nil)
	senza.Criterio = valutazione.CriterioNessuno
	in = ingresso()
	in.Conferma = senza
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Gerarchia) != "da_verificare/"+valutazione.MotivoAsseSenzaFigliNonDichiarato ||
		asse(v.Nomenclatura) != "verificata/" || v.Criterio != valutazione.CriterioNessuno || v.Conferma.BaseGerarchia != "" {
		t.Errorf("scenario B, senza dichiarazione: %s", esitoConferma(v))
	}
}

// ---- scenario C: STEP secondario revisionato ----

// TestScenarioCSecondarioRevisionato (direttiva 12 §3.3 e §10, scenario C [U]; addendum AD-6, AD-4c, §8 P-06, P-08):
// conferma n valida; una revisione dello STEP di un sottoassieme (STEP secondario autorizzato) sostituisce la vite M6
// (figlio 07 della conferma, qta 4) con una M8, propone la qta di un altro figlio diversa e non nomina più la M6. La
// regola non cambia niente da sola: la M6 resta certificata, nessun arco tolto, nessuna differenza inventata per nome; le
// righe della revisione sono proposte esplicite, da decidere, esposte con il segno «sopravvenuta»; il prodotto è da
// riesaminare. Nessuna scrittura: l'ingresso esce com'era. Quando l'ingegnere cambia la working senza riconfermare, la
// BOM è modificata dopo la conferma (non_pronto); con la conferma n+1 torna verificata.
func TestScenarioCSecondarioRevisionato(t *testing.T) {
	shaRev := sha("f")
	radiceRev := ancoraggio.RifNodo(shaRev, "#1")
	m8 := ancoraggio.RifNodo(shaRev, "#8")
	righe := []valutazione.RigaSecondariaAperta{
		{Rif: m8, Sopravvenuta: true},
		{Rif: valutazione.RifArco(radiceRev, m8), Arco: true, Sopravvenuta: true},
		{Rif: valutazione.RifArco(radiceRev, ancoraggio.RifNodo(shaRev, "#3")), Arco: true, Sopravvenuta: true},
	}
	in := ricostruzioneACM()
	in.SecondarieAperte = righe
	prima, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	v := valutazione.VerificaDallaConferma(in)
	if v.Verificata || asse(v.Nomenclatura) != "da_verificare/da_decidere" || asse(v.Gerarchia) != "da_verificare/da_decidere" ||
		v.Nomenclatura.DaDecidere != 1 || v.Gerarchia.DaDecidere != 3 || !v.Conferma.Valida || len(v.Conferma.Cambiamenti) != 0 {
		t.Errorf("scenario C: %s (%d, %d), %+v", esitoConferma(v), v.Nomenclatura.DaDecidere, v.Gerarchia.DaDecidere, v.Conferma)
	}
	if !reflect.DeepEqual(v.Conferma.Secondarie, righe) || v.Conferma.SecondarieAperte != 3 {
		t.Errorf("scenario C: righe esposte %+v", v.Conferma.Secondarie)
	}
	// la M6 resta fra gli archi certificati, con la sua qta
	vite := valutazione.RifArco(rifC(finitoACM), rifC(figlioACM(6)))
	trovata := false
	for _, a := range v.Conferma.Archi {
		trovata = trovata || a.Arco == vite
	}
	if !trovata {
		t.Errorf("scenario C: la M6 non è più fra gli archi certificati: %+v", v.Conferma.Archi)
	}
	cond := valutazione.CondizioniNuoveDallaConfermaPerProva(pvACM(v, fonteConfermataACM()), in, nil, fotorfq.Thread{})
	if cond != (valutazione.CondizioniNuove{Nomenclatura: true, Gerarchia: true}) {
		t.Errorf("scenario C: condizioni nuove %+v", cond)
	}
	if _, stato, _ := valutazione.StatoDelProdotto(pvACM(v, fonteConfermataACM()), cond); stato != valutazione.ProdottoDaRiesaminare {
		t.Errorf("scenario C: stato del prodotto %s, atteso da_riesaminare", stato)
	}
	dopo, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(prima) != string(dopo) {
		t.Errorf("scenario C: la regola ha cambiato l'ingresso")
	}

	// l'ingegnere accetta la M8 nella working (via la M6, dentro la M8) senza riconfermare: modificata dopo la conferma
	modificata := ricostruzioneACM()
	modificata.ArchiCorrenti[6].FiglioID = figlioACM(21)
	v = valutazione.VerificaDallaConferma(modificata)
	attesi := []string{"aggiunto:" + valutazione.RifArco(rifC(finitoACM), rifC(figlioACM(21))), "tolto:" + vite}
	if v.Verificata || asse(v.Gerarchia) != "da_verificare/"+valutazione.MotivoAsseStrutturaCambiata || !reflect.DeepEqual(v.Conferma.Cambiamenti, attesi) {
		t.Errorf("scenario C, working cambiata senza conferma: %s, %v", esitoConferma(v), v.Conferma.Cambiamenti)
	}
	cond = valutazione.CondizioniNuoveDallaConfermaPerProva(pvACM(v, fonteConfermataACM()), modificata, nil, fotorfq.Thread{})
	if _, stato, _ := valutazione.StatoDelProdotto(pvACM(v, fonteConfermataACM()), cond); stato != valutazione.ProdottoNonPronto {
		t.Errorf("scenario C, working cambiata senza conferma: stato %s, atteso non_pronto", stato)
	}
	// la conferma n+1 con la M8
	n1 := ricostruzioneACM()
	n1.Conferma.Numero = 2
	n1.Conferma.Componenti[7].ComponenteID, n1.Conferma.Componenti[7].Codice = figlioACM(21), "ACM-9000-21"
	n1.Conferma.Archi[6].FiglioID = figlioACM(21)
	n1.ArchiCorrenti = correntiDi(n1.Conferma)
	if v := valutazione.VerificaDallaConferma(n1); !v.Verificata || v.Conferma.Numero != 2 {
		t.Errorf("scenario C, conferma n+1 con la M8: %s", esitoConferma(v))
	}
}

// ---- scenario E: la provenienza degli archi ----

// TestProvenienzaDegliArchi (direttiva 12 §4 e §10, scenario E [U]; addendum AD-5b, §8 P-05): archi dallo STEP, archi di
// una persona, archi riportati da una conferma precedente, archi legacy con l'origine NULL. Nessuna origine si
// attribuisce falsamente allo STEP o a una persona: conferma, registrato e NULL contano solo con la provenienza
// ricostruita dalla lettura, altrimenti sono indeterminata; origine_gerarchia resta coerente con gli archi; la verifica
// non cambia (certezza della struttura ≠ certezza della provenienza). Il valore aggregato indeterminata è PROVVISORIO
// (D-ADD-03 a, senza risposta): i casi che lo attendono sono marcati «D-ADD-03».
func TestProvenienzaDegliArchi(t *testing.T) {
	type arco struct{ origine, provenienza, da string }
	for _, c := range []struct {
		nome     string
		archi    []arco
		atteso   valutazione.OrigineGerarchia
		attese   []string // la provenienza di ogni arco, nell'ordine dei figli
		conteggi map[string]int
	}{
		{"le quattro origini dello scenario E, tutte ricostruite", []arco{{"step", "", ""}, {"utente", "", ""}, {"conferma", "step", "conferma:1"}, {"", "utente", "conferma:1"}},
			valutazione.OrigineGerarchiaMista, []string{"step", "utente", "step", "utente"}, map[string]int{"step": 2, "utente": 2}},
		{"D-ADD-03: un arco legacy NULL non ricostruibile", []arco{{"step", "", ""}, {"utente", "", ""}, {"conferma", "step", "conferma:1"}, {"", "", ""}},
			valutazione.OrigineGerarchiaIndeterminata, []string{"step", "utente", "step", "indeterminata"}, map[string]int{"step": 2, "utente": 1, "indeterminata": 1}},
		{"D-ADD-03: conferma, registrato e NULL senza ricostruzione non sono manuale (sonda S-E1 rovesciata)", []arco{{"conferma", "", ""}, {"registrato", "", ""}, {"", "", ""}},
			valutazione.OrigineGerarchiaIndeterminata, []string{"indeterminata", "indeterminata", "indeterminata"}, map[string]int{"indeterminata": 3}},
		{"tutti dallo STEP, anche quello riportato da una conferma precedente", []arco{{"step", "", ""}, {"conferma", "step", "conferma:2"}},
			valutazione.OrigineGerarchiaStep, []string{"step", "step"}, map[string]int{"step": 2}},
		{"tutti di una persona, anche quello registrato", []arco{{"utente", "", ""}, {"registrato", "utente", "conferma:1"}},
			valutazione.OrigineGerarchiaManuale, []string{"utente", "utente"}, map[string]int{"utente": 2}},
		{"D-ADD-03: un'origine registrata e una provenienza ricostruita che si contraddicono", []arco{{"step", "utente", "conferma:1"}, {"step", "", ""}},
			valutazione.OrigineGerarchiaIndeterminata, []string{"indeterminata", "step"}, map[string]int{"step": 1, "indeterminata": 1}},
		{"D-ADD-03: «manuale», il default di componente.origine, non è una provenienza", []arco{{"", "manuale", "componente_relazione"}, {"utente", "", ""}},
			valutazione.OrigineGerarchiaIndeterminata, []string{"indeterminata", "utente"}, map[string]int{"utente": 1, "indeterminata": 1}},
		// Riscritta per la revisione della rev. 1 (M-3): prima STEP più codice_rilevato dava mista, cioè attribuiva in
		// parte a una persona archi che non sono di nessuna persona (direttiva 12 §4 [U]); mista vale solo fra STEP e persona
		{"D-ADD-03 a2: un terzo produttore accanto allo STEP non fa mista", []arco{{"registrato", "codice_rilevato", "componente_relazione"}, {"step", "", ""}},
			valutazione.OrigineGerarchiaIndeterminata, []string{"codice_rilevato", "step"}, map[string]int{"codice_rilevato": 1, "step": 1}},
		{"D-ADD-03 a2: STEP, persona e un terzo produttore", []arco{{"step", "", ""}, {"utente", "", ""}, {"registrato", "codice_rilevato", "componente_relazione"}},
			valutazione.OrigineGerarchiaIndeterminata, []string{"step", "utente", "codice_rilevato"}, map[string]int{"step": 1, "utente": 1, "codice_rilevato": 1}},
		{"D-ADD-03 a2: un solo produttore che non è né lo STEP né una persona", []arco{{"registrato", "codice_rilevato", "componente_relazione"}},
			valutazione.OrigineGerarchiaIndeterminata, []string{"codice_rilevato"}, map[string]int{"codice_rilevato": 1}},
	} {
		qta := make([]int, len(c.archi))
		for i := range qta {
			qta[i] = 1
		}
		in := scenarioAACM()
		in.Conferma = confermaManualeACM(qta)
		for i, a := range c.archi {
			in.Conferma.Archi[i].Origine, in.Conferma.Archi[i].Provenienza, in.Conferma.Archi[i].ProvenienzaDa = a.origine, a.provenienza, a.da
		}
		in.ArchiCorrenti = correntiDi(in.Conferma)
		v := valutazione.VerificaDallaConferma(in)
		if !v.Verificata {
			t.Errorf("%s: la provenienza ha tolto la verifica: %s", c.nome, esitoConferma(v))
		}
		if v.OrigineGerarchia != c.atteso || !reflect.DeepEqual(v.Conferma.ProvenienzaArchi, c.conteggi) {
			t.Errorf("%s: origine %q, conteggi %v; attesi %q, %v", c.nome, v.OrigineGerarchia, v.Conferma.ProvenienzaArchi, c.atteso, c.conteggi)
		}
		per := map[string]valutazione.ProvenienzaArco{}
		for _, p := range v.Conferma.Archi {
			per[p.Arco] = p
		}
		for i, a := range c.archi {
			p := per[valutazione.RifArco(rifC(finitoACM), rifC(figlioACM(i)))]
			if p.Provenienza != c.attese[i] || p.Origine != a.origine {
				t.Errorf("%s, arco %d: %+v, attesa %s", c.nome, i, p, c.attese[i])
			}
			// nessun arco di trasporto o NULL è step o utente senza una provenienza ricostruita
			if (a.origine != "step" && a.origine != "utente") && a.provenienza == "" && p.Provenienza != valutazione.ProvenienzaIndeterminata {
				t.Errorf("%s, arco %d: provenienza attribuita senza evidenza: %+v", c.nome, i, p)
			}
			if p.Provenienza != valutazione.ProvenienzaIndeterminata && p.Da == "" {
				t.Errorf("%s, arco %d: provenienza senza da dove viene: %+v", c.nome, i, p)
			}
		}
	}
}

// ---- la base della gerarchia ----

// TestBaseDellaGerarchia (direttiva 12 §2.1 [U]: «il sistema deve dichiarare che la gerarchia è stata ricostruita
// manualmente»; addendum AD-4, §8 P-04; campo PROVVISORIO [P], D-ADD-03 b): ricostruita_a_mano con lo STEP che non
// determina la gerarchia (non riconosciuta, parziale, non analizzabile), step con lo STEP riconosciuto, mono_pezzo per la
// dichiarazione, vuota senza la fonte confermata, con una conferma che non conta o con il criterio nessuno. Il criterio
// esposto resta step anche per la ricostruzione a mano: non è lui a dire da dove viene la gerarchia (P-09).
func TestBaseDellaGerarchia(t *testing.T) {
	parziale := ricostruzioneACM()
	parziale.Analisi = valutazione.AnalisiStruttura{Letta: true, Versione: 3, Radici: 1, Troncato: true, MotivoParziale: "lettura troncata"}
	senzaFonte := ricostruzioneACM()
	senzaFonte.Struttura = strutturaSoloPartiACM(false)
	nonValida := ricostruzioneACM()
	nonValida.Conferma.Valida = false
	nessuno := ricostruzioneACM()
	nessuno.Conferma.Criterio = valutazione.CriterioNessuno
	mono := valutazione.IngressoVerificaConferma{Conferma: monoACM(), Struttura: strutturaSoloPartiACM(true), Fonte: fonteACM(), Analisi: analisiSoloPartiACM()}
	for _, c := range []struct {
		nome     string
		in       valutazione.IngressoVerificaConferma
		base     valutazione.BaseGerarchia
		criterio valutazione.CriterioVerifica
	}{
		{"STEP non riconosciuto, deroga valida", ricostruzioneACM(), valutazione.BaseGerarchiaRicostruitaAMano, valutazione.CriterioStep},
		{"STEP parziale, deroga valida", parziale, valutazione.BaseGerarchiaRicostruitaAMano, valutazione.CriterioStep},
		{"STEP non analizzabile, deroga valida", scenarioAACM(), valutazione.BaseGerarchiaRicostruitaAMano, valutazione.CriterioStep},
		{"STEP riconosciuto", stepConRelazioniACM(), valutazione.BaseGerarchiaStep, valutazione.CriterioStep},
		{"mono_pezzo", mono, valutazione.BaseGerarchiaMonoPezzo, valutazione.CriterioMonoPezzo},
		{"senza la fonte confermata", senzaFonte, "", valutazione.CriterioNessuno},
		{"conferma non valida", nonValida, "", valutazione.CriterioNessuno},
		{"criterio nessuno della conferma", nessuno, "", valutazione.CriterioNessuno},
	} {
		v := valutazione.VerificaDallaConferma(c.in)
		if v.Conferma.BaseGerarchia != c.base || v.Criterio != c.criterio {
			t.Errorf("%s: base %q, criterio %s; attesi %q, %s", c.nome, v.Conferma.BaseGerarchia, v.Criterio, c.base, c.criterio)
		}
	}
}

// ---- le condizioni nuove: riesame o non pronto ----

// fonteConfermataACM, fonteSuperataACM: lo stato della fonte del prodotto (asse 2).
func fonteConfermataACM() valutazione.FonteProdotto {
	return valutazione.FonteProdotto{Calcolata: true, Stato: valutazione.FonteConfermata, Motivo: valutazione.MotivoFonteEstrazioneRiuscita}
}

func fonteSuperataACM() valutazione.FonteProdotto {
	return valutazione.FonteProdotto{Calcolata: true, Motivo: valutazione.MotivoFonteRiferimentoSuperato}
}

// pvACM: il prodotto ACM-9000 con il target confermato, la fonte data, la verifica della BOM data, lo smistamento
// verificato e la completezza completa: solo la BOM e la fonte decidono lo stato.
func pvACM(v valutazione.VerificaBOM, fonte valutazione.FonteProdotto) valutazione.ProdottoValutato {
	c := finitoACM
	return valutazione.ProdottoValutato{Rif: "ACM-9000", Autorita: ancoraggio.AutoritaConfermata, ComponenteID: &c, CodiceRichiesto: "ACM-9000",
		Identita: valutazione.IdentitaConfermata, Fonte: fonte, BOM: v,
		Smistamento: valutazione.VerificaSmistamento{Stato: valutazione.SmistamentoVerificato, Calcolata: true},
		Documenti:   valutazione.CompletezzaDocumentale{Stato: valutazione.DocumentiCompleta, PerimetroChiuso: true}}
}

// TestCondizioniNuoveRiesameONonPronto (direttiva 12 §3.5 [U]; addendum AD-4c, AD-4d, §8 P-06; A1c T-E1-14): una BOM già
// confermata che riceve evidenze nuove e irrisolte è da riesaminare; una BOM senza i requisiti della prima conferma, una
// BOM modificata dopo la conferma, una deroga invalida e un errore d'integrità restano non_pronto, anche quando accanto
// c'è un'evidenza nuova (che non li maschera). Lo stato si calcola con StatoDelProdotto (A1c, invariato) e l'adattatore
// delle condizioni nuove dei prodotti con una conferma.
func TestCondizioniNuoveRiesameONonPronto(t *testing.T) {
	shaRev := sha("f")
	nuovaNodo := valutazione.RigaSecondariaAperta{Rif: ancoraggio.RifNodo(shaRev, "#8"), Sopravvenuta: true}
	nuovaArco := valutazione.RigaSecondariaAperta{Rif: valutazione.RifArco(ancoraggio.RifNodo(shaRev, "#1"), ancoraggio.RifNodo(shaRev, "#8")), Arco: true, Sopravvenuta: true}
	vecchia := valutazione.RigaSecondariaAperta{Rif: ancoraggio.RifNodo(shaRev, "#9")}
	sopravvenute := func(in *valutazione.IngressoVerificaConferma) {
		in.SecondarieAperte = append(in.SecondarieAperte, nuovaNodo, nuovaArco)
	}
	conflitto := []valutazione.Conflitto{{Tipo: valutazione.ConflittoArco, Asse: valutazione.AsseGerarchia,
		Rif: valutazione.RifArco(rifC(finitoACM), rifC(figlioACM(2))), Motivo: "qta decisa contro lo STEP"}}
	n4 := ancoraggio.RifNodo(shaFonteACM, "#4")
	type passo = func(*valutazione.IngressoVerificaConferma)
	for _, c := range []struct {
		nome   string
		base   func() valutazione.IngressoVerificaConferma
		fonte  valutazione.FonteProdotto
		passi  []passo
		atteso valutazione.StatoProdotto
	}{
		{"tutto a posto", ricostruzioneACM, fonteConfermataACM(), nil, valutazione.ProdottoProntoFattibilita},
		// evidenze nuove: da_riesaminare
		{"evidenza nuova: righe di uno STEP secondario sopravvenute", ricostruzioneACM, fonteConfermataACM(), []passo{sopravvenute}, valutazione.ProdottoDaRiesaminare},
		{"evidenza nuova: una sola riga d'arco sopravvenuta, nessuna voce principale", ricostruzioneACM, fonteConfermataACM(), []passo{func(in *valutazione.IngressoVerificaConferma) {
			in.SecondarieAperte = []valutazione.RigaSecondariaAperta{nuovaArco}
		}}, valutazione.ProdottoDaRiesaminare},
		{"evidenza nuova sullo STEP non analizzabile ricostruito a mano", scenarioAACM, fonteConfermataACM(), []passo{sopravvenute}, valutazione.ProdottoDaRiesaminare},
		{"condizione nuova: conflitto aperto", ricostruzioneACM, fonteConfermataACM(), []passo{func(in *valutazione.IngressoVerificaConferma) {
			in.Struttura.Conflitti = conflitto
		}}, valutazione.ProdottoDaRiesaminare},
		{"condizione nuova: fonte superata", func() valutazione.IngressoVerificaConferma {
			in := ricostruzioneACM()
			in.Struttura = strutturaSoloPartiACM(false)
			in.DecisioniStep.Nodi = map[string]uuid.UUID{radiceStepAC: finitoACM}
			return in
		}, fonteSuperataACM(), nil, valutazione.ProdottoDaRiesaminare},
		// requisiti della prima conferma: non_pronto, anche con l'evidenza nuova accanto
		{"requisito: riga secondaria già presente alla conferma", ricostruzioneACM, fonteConfermataACM(), []passo{func(in *valutazione.IngressoVerificaConferma) {
			in.SecondarieAperte = []valutazione.RigaSecondariaAperta{vecchia}
		}}, valutazione.ProdottoNonPronto},
		{"requisito: riga vecchia accanto alle sopravvenute", ricostruzioneACM, fonteConfermataACM(), []passo{sopravvenute, func(in *valutazione.IngressoVerificaConferma) {
			in.SecondarieAperte = append(in.SecondarieAperte, vecchia)
		}}, valutazione.ProdottoNonPronto},
		{"requisito: deroga assente", ricostruzioneACM, fonteConfermataACM(), []passo{sopravvenute, func(in *valutazione.IngressoVerificaConferma) { in.Deroghe = nil }},
			valutazione.ProdottoNonPronto},
		{"requisito: deroga assente sullo STEP non analizzabile", scenarioAACM, fonteConfermataACM(), []passo{sopravvenute, func(in *valutazione.IngressoVerificaConferma) {
			in.Deroghe = nil
		}}, valutazione.ProdottoNonPronto},
		{"requisito: senza figli e senza dichiarazione", ricostruzioneACM, fonteConfermataACM(), []passo{sopravvenute, func(in *valutazione.IngressoVerificaConferma) {
			in.Conferma = confermaManualeACM(nil)
			in.Conferma.Criterio = valutazione.CriterioNessuno
			in.ArchiCorrenti = nil
		}}, valutazione.ProdottoNonPronto},
		{"requisito: criterio nessuno con figli", ricostruzioneACM, fonteConfermataACM(), []passo{sopravvenute, func(in *valutazione.IngressoVerificaConferma) {
			in.Conferma.Criterio = valutazione.CriterioNessuno
		}}, valutazione.ProdottoNonPronto},
		{"requisito: arco STEP aperto non recepito dalla conferma", stepConRelazioniACM, fonteConfermataACM(), []passo{sopravvenute, func(in *valutazione.IngressoVerificaConferma) {
			in.Struttura.Nodi = append(in.Struttura.Nodi, n4)
			in.Struttura.Archi = append(in.Struttura.Archi, valutazione.RifArco(radiceStepAC, n4))
		}}, valutazione.ProdottoNonPronto},
		{"requisito: arco STEP fra due nodi decisi, non recepito (voce vecchia della sola gerarchia)", stepConRelazioniACM, fonteConfermataACM(), []passo{sopravvenute, func(in *valutazione.IngressoVerificaConferma) {
			in.Struttura.Archi = append(in.Struttura.Archi, valutazione.RifArco(ancoraggio.RifNodo(shaFonteACM, "#2"), ancoraggio.RifNodo(shaFonteACM, "#3")))
		}}, valutazione.ProdottoNonPronto},
		{"requisito: nodo STEP aperto non deciso (voce vecchia della sola nomenclatura)", stepConRelazioniACM, fonteConfermataACM(), []passo{sopravvenute, func(in *valutazione.IngressoVerificaConferma) {
			in.Struttura.Nodi = append(in.Struttura.Nodi, n4)
		}}, valutazione.ProdottoNonPronto},
		// modificata dopo la conferma, deroga invalida: non_pronto
		{"modificata: arco tolto dopo la conferma", ricostruzioneACM, fonteConfermataACM(), []passo{sopravvenute, func(in *valutazione.IngressoVerificaConferma) {
			in.ArchiCorrenti = in.ArchiCorrenti[1:]
		}}, valutazione.ProdottoNonPronto},
		{"modificata: conferma non più valida", ricostruzioneACM, fonteConfermataACM(), []passo{sopravvenute, func(in *valutazione.IngressoVerificaConferma) {
			in.Conferma.Valida = false
		}}, valutazione.ProdottoNonPronto},
		{"deroga invalida: scaduta", ricostruzioneACM, fonteConfermataACM(), []passo{sopravvenute, func(in *valutazione.IngressoVerificaConferma) {
			in.Deroghe[0].Stato = valutazione.StatoDerogaSuperata
		}}, valutazione.ProdottoNonPronto},
		{"conflitto e deroga assente", ricostruzioneACM, fonteConfermataACM(), []passo{func(in *valutazione.IngressoVerificaConferma) {
			in.Struttura.Conflitti, in.Deroghe = conflitto, nil
		}}, valutazione.ProdottoNonPronto},
		// errori d'integrità: mai un riesame
		{"integrità: conferma incoerente", ricostruzioneACM, fonteConfermataACM(), []passo{sopravvenute, func(in *valutazione.IngressoVerificaConferma) {
			in.Conferma.Componenti[1].Radice = true
		}}, valutazione.ProdottoNonPronto},
		{"integrità: ingressi incoerenti", ricostruzioneACM, fonteConfermataACM(), []passo{sopravvenute, func(in *valutazione.IngressoVerificaConferma) {
			in.Struttura.ArchiConfermati = []string{valutazione.RifArco(rifC(finitoACM), rifC(figlioACM(0)))}
		}}, valutazione.ProdottoNonPronto},
		{"integrità: radice STEP non mappata", ricostruzioneACM, fonteConfermataACM(), []passo{sopravvenute, func(in *valutazione.IngressoVerificaConferma) {
			in.Conferma.Componenti[0].StepSha256 = sha("b")
		}}, valutazione.ProdottoNonPronto},
		{"integrità: mono_pezzo non applicabile", func() valutazione.IngressoVerificaConferma {
			in := valutazione.IngressoVerificaConferma{Conferma: monoACM(), Struttura: strutturaSoloPartiACM(true), Fonte: fonteACM(), Analisi: analisiSoloPartiACM()}
			in.Conferma.MonoPezzoMotivo = " "
			return in
		}, fonteConfermataACM(), []passo{sopravvenute}, valutazione.ProdottoNonPronto},
	} {
		in := c.base()
		for _, p := range c.passi {
			p(&in)
		}
		v := valutazione.VerificaDallaConferma(in)
		pv := pvACM(v, c.fonte)
		cond := valutazione.CondizioniNuoveDallaConfermaPerProva(pv, in, nil, fotorfq.Thread{})
		if _, stato, motivi := valutazione.StatoDelProdotto(pv, cond); stato != c.atteso {
			t.Errorf("%s: stato %s, atteso %s (verifica %s, condizioni nuove %+v, motivi %v)", c.nome, stato, c.atteso, esitoConferma(v), cond, motivi)
		}
	}
}
