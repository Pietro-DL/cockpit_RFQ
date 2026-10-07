package bancoa

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	valut "promatec/cockpit/internal/core/valutazione"
)

// L1 — R109, precisata dall'utente il 07/10 senza scegliere una lettera (E2 §2.3; revisione d'impatto, R109 §3 b e §4 b):
// PO-29 con la derivazione dagli attesi esplicita e indipendente dal motore (fontiDelloScenario):
//   - si scorrono i prodotti attesi dello scenario, non quelli del motore: un prodotto atteso senza calcolato è una
//     differenza, anche quando il motore legge la base in un altro modo (prima la riga non c'era, oppure dava «assente»
//     senza differenze);
//   - un «assente» senza motivo negli attesi: il rapporto lo scrive, e il motivo calcolato non si confronta;
//   - la derivazione elenca la regola, i prodotti attesi e le sole voci con atteso radice del target; una voce radice
//     non risolta rende la derivazione incompleta, e lo dice;
//   - i livelli distinti: presente, estrazione riuscita, associazione, autorizzazione come fonte, verifica operativa (mai
//     in A1c);
//   - lo scenario che la corsa non verifica: nessuna differenza, la parte calcolata fra le non verificate.
//
// I clienti di questi test sono inventati: vedi scena_banco_test.go.

// indiceFontiACME: tre allegati del thread dello scenario (lo STEP, il PDF radice, il PDF fuori) e il thread.
func indiceFontiACME() indiceFoto {
	stp, pdf := "stp", "pdf"
	shaStep, shaPDF := shaACME(0x102), shaACME(0x101)
	return indiceFoto{allegati: map[uuid.UUID]rifAllegato{
		allFiglio: {allegato: fotorfq.Allegato{ID: allFiglio, NomeFile: "9123456A_1.stp", Estensione: &stp, Sha256: &shaStep}, thread: threadScenario},
		allRadice: {allegato: fotorfq.Allegato{ID: allRadice, NomeFile: "9123456A_2.pdf", Estensione: &pdf, Sha256: &shaPDF}, thread: threadScenario},
		allFuori:  {allegato: fotorfq.Allegato{ID: allFuori, NomeFile: "9123458.pdf", Estensione: &pdf}, thread: threadScenario},
	}, thread: map[uuid.UUID]int{threadScenario: 0}}
}

// voceScenarioACME: una voce di file dello scenario, risolta, con il suo percorso.
func voceScenarioACME(i int, id uuid.UUID, atteso, base string) voceTradotta {
	return voceTradotta{voce: VoceAttesa{Sezione: confronto.SezioneScenario, Percorso: fmt.Sprintf("scenario_inoltro.file[%d]", i),
		Atteso: atteso, TargetBase: base}, thread: threadScenario, risolta: true,
		file: confronto.FileAtteso{AllegatoID: id, Sezione: confronto.SezioneScenario}}
}

// fotoFontiACME: la fotografia del thread dello scenario, senza fatti.
func fotoFontiACME() fotorfq.Fotografia {
	return fotorfq.Fotografia{Thread: []fotorfq.Thread{{ID: threadScenario, ClienteID: clienteACME}}}
}

// esitoFontiACME: l'esito con il thread dello scenario valutato e i suoi prodotti calcolati.
func esitoFontiACME(calcolati ...valut.ProdottoValutato) valut.Esito {
	return valut.Esito{Thread: []valut.EsitoThread{{ThreadID: threadScenario, ClienteID: clienteACME, Valutato: true, ProdottiValutati: calcolati}}}
}

// fontiACME: la fonte contro l'atteso dello scenario, con i prodotti attesi e i prodotti calcolati del thread.
func fontiACME(tr *traduzione, ix indiceFoto, attesi []ProdottoScenario, calcolati ...valut.ProdottoValutato) *FontiDelloScenario {
	th := threadScenario
	return fontiDelloScenario(fotoFontiACME(), esitoFontiACME(calcolati...), &ScenarioAtteso{ThreadID: &th, Prodotti: attesi}, tr, ix, "")
}

// fonteUnicaACME: la sola fonte dello scenario, con un prodotto calcolato e la base attesa dalle voci.
func fonteUnicaACME(t *testing.T, tr *traduzione, ix indiceFoto, pv valut.ProdottoValutato) ConfrontoFonte {
	t.Helper()
	fs := fontiACME(tr, ix, nil, pv)
	if fs == nil || len(fs.Prodotti) != 1 {
		t.Fatalf("una fonte attesa: %+v", fs)
	}
	return fs.Prodotti[0]
}

// prodottoACME: un prodotto calcolato con la base e la fonte; i candidati della fonte dal JSON (il banco non nomina
// ancoraggio, nemmeno nelle prove fuori dalla prova di parità).
func prodottoACME(t *testing.T, base string, stato valut.StatoFonte, motivo valut.MotivoFonte, candidati ...uuid.UUID) valut.ProdottoValutato {
	t.Helper()
	pv := valut.ProdottoValutato{Rif: "identificativo:" + base, Base: motorea.BaseLetta{Normalizzata: base},
		Fonte: valut.FonteProdotto{Stato: stato, Motivo: motivo, Calcolata: true}}
	if len(candidati) > 0 {
		var cc []string
		for _, id := range candidati {
			cc = append(cc, fmt.Sprintf(`{"origine":"motore_a","allegato_id":%q,"estrazione":"riuscita","grafo_completo":true}`, id))
		}
		if err := json.Unmarshal([]byte(`{"candidati":[`+strings.Join(cc, ",")+`]}`), &pv.Fonte); err != nil {
			t.Fatal(err)
		}
		pv.Fonte.Stato, pv.Fonte.Motivo, pv.Fonte.Calcolata = stato, motivo, true
	}
	return pv
}

// TestRispostaR109SiScorronoIProdottiAttesi (R109 §4 b, prova 1): gli attesi dicono la base 9123456, il motore la legge
// in un altro modo. Prima la riga del prodotto atteso non c'era (si scorrevano i prodotti del motore, e per quello del
// motore la fonte attesa era «assente» senza differenze); ora il prodotto atteso è senza calcolato, una differenza
// esplicita, e il prodotto del motore sta fra i calcolati senza atteso. Le basi sono quelle degli attesi: anche un
// prodotto atteso senza voci radice c'è, in ordine di base.
func TestRispostaR109SiScorronoIProdottiAttesi(t *testing.T) {
	ix := indiceFontiACME()
	tr := &traduzione{voci: []voceTradotta{voceScenarioACME(0, allFiglio, "radice", "9123456")}}
	altrove := prodottoACME(t, "9.123.456", valut.FonteAssente, valut.MotivoFonteNessunaFonte)
	fs := fontiACME(tr, ix, []ProdottoScenario{{Percorso: "scenario_inoltro.prodotti_attesi[0]", Base: "9123456"}}, altrove)
	if fs == nil || len(fs.Prodotti) != 1 {
		t.Fatalf("un prodotto atteso: %+v", fs)
	}
	c := fs.Prodotti[0]
	if c.Base != "9123456" || !c.SenzaCalcolato || c.Calcolato != nil ||
		strings.Join(c.Differenze, ",") != "prodotto atteso senza calcolato: nessun prodotto del thread dello scenario ha la base attesa" {
		t.Errorf("il prodotto atteso con la base letta in un altro modo: %+v", c)
	}
	if strings.Join(fs.CalcolatiSenzaAtteso, ",") != "9.123.456 (identificativo:9.123.456)" {
		t.Errorf("il prodotto del motore senza atteso: %v", fs.CalcolatiSenzaAtteso)
	}

	// Due prodotti attesi, uno solo con le voci, nessuno calcolato: tutti e due, in ordine di base.
	fs = fontiACME(tr, ix, []ProdottoScenario{{Percorso: "scenario_inoltro.prodotti_attesi[0]", Base: "9123457"},
		{Percorso: "scenario_inoltro.prodotti_attesi[1]", Base: "9123456"}})
	if len(fs.Prodotti) != 2 || fs.Prodotti[0].Base != "9123456" || fs.Prodotti[1].Base != "9123457" ||
		!fs.Prodotti[0].SenzaCalcolato || !fs.Prodotti[1].SenzaCalcolato || len(fs.Prodotti[1].Derivazione.Voci) != 0 {
		t.Errorf("i prodotti attesi senza calcolato: %+v", fs.Prodotti)
	}

	// Il thread dello scenario non valutato, o assente dall'esito: senza calcolato, con il perché.
	th := threadScenario
	e := esitoFontiACME()
	e.Thread[0].Valutato, e.Thread[0].Motivo = false, valut.MotivoThreadErroreValutazione
	c = fontiDelloScenario(fotoFontiACME(), e, &ScenarioAtteso{ThreadID: &th}, tr, ix, "").Prodotti[0]
	if !c.SenzaCalcolato || !conPrefisso(c.Differenze, "prodotto atteso senza calcolato: il thread dello scenario non è valutato (") {
		t.Errorf("il thread non valutato: %+v", c)
	}
	c = fontiDelloScenario(fotoFontiACME(), valut.Esito{}, &ScenarioAtteso{ThreadID: &th}, tr, ix, "").Prodotti[0]
	if !c.SenzaCalcolato || !conPrefisso(c.Differenze, "prodotto atteso senza calcolato: il thread dello scenario non è nell'esito") {
		t.Errorf("il thread assente dall'esito: %+v", c)
	}

	// Due prodotti calcolati con la base attesa: una differenza, e il confronto non sceglie.
	doppio := fontiACME(tr, ix, nil, prodottoACME(t, "9123456", valut.FonteAssente, ""), prodottoACME(t, "9123456", valut.FonteAssente, ""))
	if c := doppio.Prodotti[0]; c.Calcolato != nil || c.SenzaCalcolato || !conPrefisso(c.Differenze, "2 prodotti calcolati con la base attesa") {
		t.Errorf("due calcolati con la stessa base: %+v", c)
	}
}

// TestRispostaR109AssenteSenzaMotivoFissato (R109 §4 b, prova 2): nessuno STEP fra le radici attese. La fonte attesa è
// assente e gli attesi non ne fissano il motivo: il rapporto lo scrive fra le parti non verificate e nel riepilogo, non
// tace, e il motivo calcolato non diventa una differenza.
func TestRispostaR109AssenteSenzaMotivoFissato(t *testing.T) {
	ix := indiceFontiACME()
	tr := &traduzione{voci: []voceTradotta{voceScenarioACME(0, allRadice, "radice", "9123456"), voceScenarioACME(1, allFuori, "fuori", "")}}
	c := fonteUnicaACME(t, tr, ix, prodottoACME(t, "9123456", valut.FonteAssente, valut.MotivoFonteStepPresenteNonAnalizzato))
	if c.Atteso.Stato != string(valut.FonteAssente) || c.Atteso.Motivo != "" || len(c.Differenze) != 0 ||
		len(c.NonVerificate) != 1 || !strings.HasPrefix(c.NonVerificate[0], "motivo: non fissato dagli attesi") {
		t.Errorf("assente senza motivo fissato: %+v", c)
	}
	var b strings.Builder
	testoFonti(func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }, &FontiDelloScenario{Prodotti: []ConfrontoFonte{c}})
	if !strings.Contains(b.String(), "prodotto atteso 9123456 (regola "+RegolaFonteAttesa+", 1 voci radice): atteso assente, motivo non fissato dagli attesi; calcolato assente, step_presente_non_analizzato") ||
		!strings.Contains(b.String(), "non verificate: motivo: non fissato dagli attesi") {
		t.Errorf("il riepilogo:\n%s", b.String())
	}
}

// TestRispostaR109LaDerivazione (R109 §4 b, prova 3): la derivazione dice la regola, i prodotti attesi e solo le voci
// con atteso radice del target (né i figli con la stessa base, né i fuori). Una voce radice che non si risolve: senza
// STEP risolti la fonte attesa non si deriva, e lo dice; con uno STEP risolto lo stato si deriva, ma i candidati attesi
// possono essere di più e non si confrontano.
func TestRispostaR109LaDerivazione(t *testing.T) {
	ix := indiceFontiACME()
	pv := prodottoACME(t, "9123456", valut.FonteAssente, valut.MotivoFonteNessunaFonte)
	tr := &traduzione{voci: []voceTradotta{
		voceScenarioACME(0, allRadice, "radice", "9123456"),
		voceScenarioACME(1, allFiglio, "figlio", "9123456"),
		voceScenarioACME(2, allFuori, "fuori", ""),
		voceScenarioACME(3, allFiglio, "radice", "9123456"),
		voceScenarioACME(4, allFuori, "radice", "9123499"),
	}}
	fs := fontiACME(tr, ix, []ProdottoScenario{{Percorso: "scenario_inoltro.prodotti_attesi[0]", Base: "9123456"}}, pv)
	d := fs.Prodotti[0].Derivazione
	if d.Regola != RegolaFonteAttesa || strings.Join(d.Voci, ",") != "scenario_inoltro.file[0],scenario_inoltro.file[3]" ||
		strings.Join(d.Prodotti, ",") != "scenario_inoltro.prodotti_attesi[0]" || len(d.NonRisolte) != 0 {
		t.Errorf("la derivazione: %+v", d)
	}
	if len(fs.Prodotti) != 2 || fs.Prodotti[1].Base != "9123499" || strings.Join(fs.Prodotti[1].Derivazione.Voci, ",") != "scenario_inoltro.file[4]" {
		t.Errorf("la base del target di una radice è un prodotto atteso: %+v", fs.Prodotti)
	}

	nr := voceScenarioACME(5, uidACME(0x1ff), "radice", "9123456")
	nr.risolta, nr.motivoNR = false, "l'allegato non è nella fotografia"
	c := fonteUnicaACME(t, &traduzione{voci: []voceTradotta{voceScenarioACME(0, allRadice, "radice", "9123456"), nr}}, ix, pv)
	if c.Atteso.Stato != "" || len(c.Differenze) != 0 || strings.Join(c.Derivazione.NonRisolte, ",") != "scenario_inoltro.file[5]: l'allegato non è nella fotografia" ||
		!conPrefisso(c.NonVerificate, "stato e motivo: 1 voci radice del target non risolte") {
		t.Errorf("una radice non risolta senza STEP: %+v", c)
	}
	c = fonteUnicaACME(t, &traduzione{voci: []voceTradotta{voceScenarioACME(3, allFiglio, "radice", "9123456"), nr}}, ix, pv)
	if c.Atteso.Stato != string(valut.FonteInAttesaDiConferma) || strings.Join(c.Differenze, ",") != "stato,motivo" ||
		!conPrefisso(c.NonVerificate, "candidati: 1 voci radice del target non risolte") {
		t.Errorf("una radice non risolta con uno STEP: %+v", c)
	}
}

// TestRispostaR109ILivelliDistinti (R109: «Mantieni però distinti estrazione riuscita, struttura corretta, associazione
// al prodotto, autorizzazione come fonte e verifica operativa»): sei livelli, in ordine, ognuno con la sua sorgente;
// l'estrazione dai fatti della fotografia (il grafo dal motivo_parziale del worker, il lavoro pendente), la struttura
// dall'asse della gerarchia del prodotto calcolato, «calcolato, non confrontato» (R-144), l'associazione dai candidati
// del motore, la verifica operativa mai in A1c.
func TestRispostaR109ILivelliDistinti(t *testing.T) {
	ix := indiceFontiACME()
	tr := &traduzione{voci: []voceTradotta{voceScenarioACME(0, allRadice, "radice", "9123456"), voceScenarioACME(3, allFiglio, "radice", "9123456")}}
	f := fotoFontiACME()
	completo := ""
	f.Thread[0].Fatti = map[string]fotorfq.Fatti{shaACME(0x102): {Sha256: shaACME(0x102), MotivoParziale: &completo}}
	f.Thread[0].InAttesa = []uuid.UUID{allFiglio}
	th := threadScenario
	pv := prodottoACME(t, "9123456", valut.FonteInAttesaDiConferma, valut.MotivoFonteDocumentoCandidato, allFiglio, allFuori)
	pv.BOM.Gerarchia = valut.VerificaAsse{Stato: "da_verificare", Motivo: "motivo-gerarchia-acme"}
	pv.BOM.Nomenclatura = valut.VerificaAsse{Stato: "stato-nomenclatura-acme"}
	c := fontiDelloScenario(f, esitoFontiACME(pv), &ScenarioAtteso{ThreadID: &th}, tr, ix, "").Prodotti[0]
	if strings.Join(c.Differenze, ",") != "candidati" {
		t.Errorf("differenze: %v", c.Differenze)
	}
	var nomi []string
	liv := map[string]LivelloFonte{}
	for _, l := range c.Livelli {
		nomi = append(nomi, l.Livello)
		liv[l.Livello] = l
	}
	if strings.Join(nomi, ",") != "presente,estrazione_riuscita,struttura,associazione,autorizzazione_come_fonte,verifica_operativa" {
		t.Fatalf("i livelli: %v", nomi)
	}
	for nome, attesi := range map[string][2]string{
		LivelloPresente:   {"2 voci radice del target, 1 STEP fra quelle risolte", "fotografia: 2 voci radice con il file, 0 senza (non risolte)"},
		LivelloEstrazione: {"non fissata dagli attesi", "1 STEP attesi su 1 con i fatti alla terna (grafo completo 1, parziale 0, non determinabile 0), 1 con il lavoro pendente"},
		LivelloStruttura: {"non fissata dagli attesi: la «struttura corretta» la misura l'asse della gerarchia del prodotto (R-144)",
			"asse della gerarchia del prodotto calcolato: da_verificare (motivo-gerarchia-acme); calcolato, non confrontato"},
		LivelloAssociazione:   {"1 STEP attesi come candidati", "motore: 2 candidati per il prodotto, 1 fra gli STEP attesi"},
		LivelloAutorizzazione: {"in_attesa_di_conferma, motivo documento_candidato; mai confermata", "motore: in_attesa_di_conferma, motivo documento_candidato"},
		LivelloVerifica:       {"mai in A1c", "non eseguita: mai in A1c"},
	} {
		if l := liv[nome]; l.Atteso != attesi[0] || !strings.Contains(l.Calcolato, attesi[1]) {
			t.Errorf("%s: %+v, atteso %q / %q", nome, l, attesi[0], attesi[1])
		}
	}
	// Con una sezione della gerarchia assente (gli export) la struttura lo dice, come l'asse nella sezione dei prodotti
	// (T-12).
	fx := f
	fx.Sezioni = map[string]fotorfq.StatoSezione{fotorfq.SezioneRelazioni: {Stato: fotorfq.StatoSezioneAssente}}
	cx := fontiDelloScenario(fx, esitoFontiACME(pv), &ScenarioAtteso{ThreadID: &th}, tr, ix, "").Prodotti[0]
	if l := cx.Livelli[2]; l.Livello != LivelloStruttura || !strings.Contains(l.Calcolato, "non calcolato: relazioni assenti nella fotografia (T-12)") ||
		!strings.HasSuffix(l.Calcolato, "; calcolato, non confrontato") {
		t.Errorf("la struttura con la sezione delle relazioni assente: %+v", l)
	}
	// Senza calcolato i livelli del motore lo dicono; l'estrazione resta quella della fotografia.
	c = fontiDelloScenario(f, esitoFontiACME(), &ScenarioAtteso{ThreadID: &th}, tr, ix, "").Prodotti[0]
	for _, l := range c.Livelli {
		if (l.Livello == LivelloStruttura || l.Livello == LivelloAssociazione || l.Livello == LivelloAutorizzazione) &&
			l.Calcolato != "nessun prodotto calcolato con la base attesa" {
			t.Errorf("%s senza calcolato: %+v", l.Livello, l)
		}
	}
}

// TestRispostaR109LoScenarioNonVerificabile: il thread dello scenario non è fra quelli scelti, o non si valuta per un
// limite degli ingressi, o lo scenario non dice il thread: i prodotti attesi ci sono, con la derivazione, ma la parte
// calcolata è fra le non verificate, mai una differenza (R-104, T-B6-104).
func TestRispostaR109LoScenarioNonVerificabile(t *testing.T) {
	ix := indiceFontiACME()
	tr := &traduzione{voci: []voceTradotta{voceScenarioACME(3, allFiglio, "radice", "9123456")}}
	th := threadScenario
	fs := fontiDelloScenario(fotoFontiACME(), esitoFontiACME(), &ScenarioAtteso{ThreadID: &th}, tr, ix, "il thread dello scenario non è fra quelli scelti")
	c := fs.Prodotti[0]
	if fs.NonVerificabile == "" || c.SenzaCalcolato || len(c.Differenze) != 0 || c.Calcolato != nil ||
		!conPrefisso(c.NonVerificate, "calcolato: il thread dello scenario non è fra quelli scelti") || len(c.Derivazione.Voci) != 1 {
		t.Errorf("lo scenario non verificabile: %+v", fs)
	}
	fs = fontiDelloScenario(fotoFontiACME(), esitoFontiACME(), &ScenarioAtteso{}, tr, ix, "")
	if fs.NonVerificabile != "lo scenario degli attesi non dice il thread" || len(fs.Prodotti[0].Differenze) != 0 {
		t.Errorf("lo scenario senza thread: %+v", fs)
	}
	if fontiDelloScenario(fotoFontiACME(), esitoFontiACME(), nil, tr, ix, "") != nil {
		t.Error("senza lo scenario non c'è la fonte contro l'atteso")
	}
}

// TestRispostaR109SullaScena: sulla scena ACME il motore non dà i prodotti dello scenario (la grammatica non legge la
// storia). Prima la fonte contro l'atteso non c'era; ora il prodotto atteso c'è, senza calcolato, con la derivazione
// dagli attesi (la sola radice è un PDF: assente, motivo non fissato), nel JSON, nel riepilogo e fra gli informativi
// incompleti. Con il thread dello scenario non scelto, la fonte non si verifica.
func TestRispostaR109SullaScena(t *testing.T) {
	r := corsaBanco(t, mutaBanco{}, true, false)
	if r.Prodotti == nil || r.Prodotti.FontiScenario == nil || len(r.Prodotti.FontiScenario.Prodotti) != 1 {
		t.Fatalf("la fonte contro l'atteso dello scenario: %+v", r.Prodotti)
	}
	fs := r.Prodotti.FontiScenario
	c := fs.Prodotti[0]
	if fs.ThreadID == nil || *fs.ThreadID != threadScenario || c.Base != "9123456" || !c.SenzaCalcolato ||
		strings.Join(c.Derivazione.Voci, ",") != "scenario_inoltro.file[0]" || strings.Join(c.Derivazione.Prodotti, ",") != "scenario_inoltro.prodotti_attesi[0]" ||
		c.Atteso.Stato != string(valut.FonteAssente) || !conPrefisso(c.NonVerificate, "motivo: non fissato dagli attesi") {
		t.Errorf("il prodotto atteso della scena: %+v", c)
	}
	if txt := r.Testo(); !strings.Contains(txt, "prodotto atteso 9123456 (regola "+RegolaFonteAttesa+", 1 voci radice): atteso assente, motivo non fissato dagli attesi; calcolato nessuno; differenze: prodotto atteso senza calcolato") {
		t.Errorf("il riepilogo:\n%s", txt)
	}
	if !strings.Contains(fmt.Sprint(r.Chiusura.Informativi), "1 prodotti attesi dello scenario su 1 con parti della fonte contro l'atteso non verificate") {
		t.Errorf("gli informativi incompleti: %+v", r.Chiusura.Informativi)
	}
	r = corsaBanco(t, mutaBanco{}, true, false, threadDecisioni)
	if fs := r.Prodotti.FontiScenario; fs == nil || fs.NonVerificabile != "il thread dello scenario non è fra quelli scelti" ||
		len(fs.Prodotti) != 1 || len(fs.Prodotti[0].Differenze) != 0 {
		t.Errorf("il thread dello scenario non scelto: %+v", fs)
	}
}
