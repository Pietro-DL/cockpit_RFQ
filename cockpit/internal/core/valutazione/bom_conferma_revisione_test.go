// L1 — la regola della conferma dopo la revisione indipendente dell'addendum ad A1c rev. 1 e della corsia C (13/10/2026,
// `a1d/01-analisi/2026-10-13-P4-dir12-revisione-addendum-C.md`; recepita nell'addendum rev. 2). I rilievi alti e medi
// che toccano il codice o le prove: A-1 (il perimetro certificato scatta sulla fonte, non sulla struttura vuota), M-1
// (la fonte confermata senza Fonte è un ingresso incoerente), M-2 (il ramo R111 dell'adattatore), M-3 (nessun innesto dei
// campi provvisori prima delle risposte), M-4 (la rianalisi della fonte principale, limite dichiarato), M-6 (lo scenario D,
// la parte che non aspetta D-ADD-04), più il basso B-9. Ingressi sintetici ACME, codici inventati, nessun DB. Le prove L4
// e nel browser degli stessi scenari sono dei passi B3.1, B3.5, B3.6, B5 (addendum §9) e qui non ci sono.
package valutazione_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

// ---- A-1: il perimetro certificato scatta sulla fonte ----

// TestPerimetroSullaFonteNonSullaStrutturaVuota (A-1 della revisione; direttiva 12 §2.1 [U], caso A e caso B; addendum
// rev. 2 AD-1, T-24, T-25): con la fonte confermata il perimetro certificato prende il posto della struttura quando lo
// STEP della fonte non dà nodi, anche se la RFQ ha strutture candidate di altri file con il codice del prodotto: quelle
// escono, e lo scenario A resta positivo. Uno STEP della fonte che dà un nodo fuori dalla BOM di lavoro resta alla regola
// di A1c (il nodo è da decidere). Senza la fonte confermata (caso B) il perimetro vale solo senza nessuna struttura: una
// candidata resta da decidere (R71 A, R76 A; le voci di un nuovo STEP sono quelle che R111 A conta, TestR111…).
func TestPerimetroSullaFonteNonSullaStrutturaVuota(t *testing.T) {
	altra, altraFiglio := ancoraggio.RifNodo(sha("9"), "#1"), ancoraggio.RifNodo(sha("9"), "#2")
	candidata := func(in *valutazione.IngressoVerificaConferma) {
		in.Struttura.Radici, in.Struttura.Nodi = []string{altra}, []string{altra, altraFiglio}
		in.Struttura.Archi = []string{valutazione.RifArco(altra, altraFiglio)}
	}

	// scenario A con una struttura candidata di un altro STEP (sonda R3 della revisione): verificata
	in := scenarioAACM()
	candidata(&in)
	v := valutazione.VerificaDallaConferma(in)
	if !v.Verificata || asse(v.Nomenclatura) != "verificata/" || asse(v.Gerarchia) != "verificata/" || v.Nomenclatura.DaDecidere != 0 ||
		v.Gerarchia.DaDecidere != 0 || v.Conferma.Deroga == nil || v.Conferma.Deroga.Stato != valutazione.DerogaApplicata ||
		v.Conferma.BaseGerarchia != valutazione.BaseGerarchiaRicostruitaAMano {
		t.Errorf("scenario A con una candidata estranea: %s (%d, %d), %+v", esitoConferma(v), v.Nomenclatura.DaDecidere, v.Gerarchia.DaDecidere, v.Conferma)
	}
	diagnosticaNonAnalizzabile(t, "scenario A con una candidata estranea", v, motivoNonAnalizzatoACM)
	// la candidata non cambia l'esito: lo stesso JSON della verifica senza di lei
	if a, b := jsonDi(t, v), jsonDi(t, valutazione.VerificaDallaConferma(scenarioAACM())); a != b {
		t.Errorf("scenario A: la candidata estranea cambia la verifica:\n%s\n%s", a, b)
	}

	// T-25: lo STEP della fonte dà un nodo fuori dalla BOM di lavoro: niente perimetro, il nodo resta da decidere
	in = scenarioAACM()
	in.Struttura.Radici, in.Struttura.Nodi = []string{radiceStepAC}, []string{radiceStepAC}
	if v := valutazione.VerificaDallaConferma(in); v.Verificata || asse(v.Nomenclatura) != "da_verificare/"+valutazione.MotivoAsseDaDecidere {
		t.Errorf("nodo della fonte fuori dalla BOM di lavoro: %s", esitoConferma(v))
	}

	// caso B: senza strutture, nomenclatura verificata dal perimetro e K-02 sulla gerarchia; con una candidata, la
	// candidata è da decidere e K-02 resta (T-24). Gli esiti sono asseriti tutti e due
	// (l'arco della candidata è da decidere sulla gerarchia, e una mancanza indipendente dalla fonte esce prima di K-02:
	// AD-4b; senza l'arco la gerarchia resta ferma solo per K-02)
	soloNodo := func(in *valutazione.IngressoVerificaConferma) {
		in.Struttura.Radici, in.Struttura.Nodi = []string{altra}, []string{altra}
	}
	k02, daDecidere := "da_verificare/"+valutazione.MotivoAsseFonteNonConfermata, "da_verificare/"+valutazione.MotivoAsseDaDecidere
	for _, c := range []struct {
		nome         string
		cambia       func(*valutazione.IngressoVerificaConferma)
		nomenclatura string
		daDecidere   int
		gerarchia    string
	}{
		{"caso B senza strutture", func(*valutazione.IngressoVerificaConferma) {}, "verificata/", 0, k02},
		{"caso B con una candidata estranea di un solo nodo", soloNodo, daDecidere, 1, k02},
		{"caso B con una candidata estranea con un arco", candidata, daDecidere, 2, daDecidere},
	} {
		in := scenarioAACM()
		in.Struttura, in.Fonte = strutturaSenzaNodiACM(false), nil
		c.cambia(&in)
		v := valutazione.VerificaDallaConferma(in)
		if v.Verificata || asse(v.Nomenclatura) != c.nomenclatura || v.Nomenclatura.DaDecidere != c.daDecidere ||
			asse(v.Gerarchia) != c.gerarchia || v.Conferma.Deroga != nil || v.Conferma.BaseGerarchia != "" {
			t.Errorf("%s: %s (%d da decidere)", c.nome, esitoConferma(v), v.Nomenclatura.DaDecidere)
		}
	}
}

// ---- M-1: la fonte confermata senza Fonte ----

// TestFonteConfermataSenzaFonte (M-1 della revisione; direttiva 12 §2.1 [U]: il mono_pezzo vale «purché la fonte STEP
// richiesta sia presente, corrente e autorizzata»): StrutturaDaVerificare dice la fonte confermata, ma l'ingresso non
// porta la Fonte. La regola non crede a due ingressi che si contraddicono: ingresso_incoerente sui due assi, nessun gesto,
// criterio nessuno; prima un mono_pezzo passava anche con la radice certificata di un altro sha (sonde R2, R2b).
func TestFonteConfermataSenzaFonte(t *testing.T) {
	mono := func() valutazione.IngressoVerificaConferma {
		return valutazione.IngressoVerificaConferma{Conferma: monoACM(), Struttura: strutturaSenzaNodiACM(true),
			Analisi: valutazione.AnalisiStruttura{MotivoParziale: motivoNonAnalizzatoACM}}
	}
	altroSha := mono()
	altroSha.Conferma.Componenti[0].StepSha256 = sha("b")
	ricostruita := scenarioAACM()
	ricostruita.Fonte = nil
	m := "da_verificare/" + valutazione.MotivoAsseIngressoIncoerente
	for nome, in := range map[string]valutazione.IngressoVerificaConferma{
		"mono_pezzo": mono(),
		"mono_pezzo con la radice di un altro sha": altroSha,
		"ricostruzione a mano":                     ricostruita,
	} {
		v := valutazione.VerificaDallaConferma(in)
		if v.Verificata || asse(v.Nomenclatura) != m || asse(v.Gerarchia) != m || v.Criterio != valutazione.CriterioNessuno || v.Conferma.BaseGerarchia != "" {
			t.Errorf("%s, fonte confermata senza Fonte: %s, base %q", nome, esitoConferma(v), v.Conferma.BaseGerarchia)
		}
	}
}

// ---- M-2: il ramo R111 dell'adattatore ----

// TestR111DopoLaFonteSuperataConUnaConferma (M-2 della revisione, sonda R14; A1c R111 A [U], precisata il 07/10;
// addendum AD-4c, riga «voci da decidere del nuovo STEP dopo una fonte superata»): lo STEP della fonte è sostituito da
// uno nuovo, non ancora autorizzato; la fotografia porta le due versioni. Le voci da decidere che vengono solo dal nuovo
// STEP sono condizioni nuove: il prodotto con una conferma è da_riesaminare. Una sola voce vecchia accanto (dello STEP
// di prima, o di un terzo STEP) tiene il prodotto non_pronto (T-E1-14: le mancanze già presenti non si nascondono).
func TestR111DopoLaFonteSuperataConUnaConferma(t *testing.T) {
	nuovo := docNuovoACM
	th := fotorfq.Thread{Documenti: []fotorfq.DocumentoConfermato{{ID: docFonteACM, Sha256: shaFonteACM, SostituitoDa: &nuovo}, {ID: docNuovoACM, Sha256: shaNuovoACM}}}
	fonte := valutazione.FonteProdotto{Calcolata: true, Motivo: valutazione.MotivoFonteRiferimentoSuperato,
		Riferimento: &ancoraggio.RiferimentoFonte{DocumentoID: docFonteACM, Sha256: shaFonteACM}}
	r1, r2 := ancoraggio.RifNodo(shaNuovoACM, "#1"), ancoraggio.RifNodo(shaNuovoACM, "#2")
	vecchio, terzo := ancoraggio.RifNodo(shaFonteACM, "#4"), ancoraggio.RifNodo(sha("9"), "#1")
	strutture := []ancoraggio.StrutturaProdotto{
		{Target: "ACM-9000", Sha256: shaFonteACM, Nodi: []ancoraggio.NodoProposto{{Rif: radiceStepAC}, {Rif: vecchio}}},
		{Target: "ACM-9000", Sha256: shaNuovoACM, Nodi: []ancoraggio.NodoProposto{{Rif: r1}, {Rif: r2}}},
	}
	for _, c := range []struct {
		nome   string
		altre  []string
		cond   valutazione.CondizioniNuove
		atteso valutazione.StatoProdotto
	}{
		{"solo voci del nuovo STEP", nil, valutazione.CondizioniNuove{Nomenclatura: true, Gerarchia: true}, valutazione.ProdottoDaRiesaminare},
		{"una voce dello STEP di prima accanto", []string{vecchio}, valutazione.CondizioniNuove{Gerarchia: true}, valutazione.ProdottoNonPronto},
		{"una voce di un terzo STEP accanto", []string{terzo}, valutazione.CondizioniNuove{Gerarchia: true}, valutazione.ProdottoNonPronto},
	} {
		in := ricostruzioneACM()
		in.Struttura = strutturaSoloPartiACM(false)
		in.Struttura.Nodi = append(append(in.Struttura.Nodi, r1, r2), c.altre...)
		in.Struttura.Radici = append(in.Struttura.Radici, r1)
		in.DecisioniStep.Nodi = map[string]uuid.UUID{radiceStepAC: finitoACM}
		v := valutazione.VerificaDallaConferma(in)
		if v.Verificata || asse(v.Nomenclatura) != "da_verificare/"+valutazione.MotivoAsseDaDecidere ||
			asse(v.Gerarchia) != "da_verificare/"+valutazione.MotivoAsseFonteNonConfermata {
			t.Errorf("%s: verifica %s", c.nome, esitoConferma(v))
		}
		pv := pvACM(v, fonte)
		cond := valutazione.CondizioniNuoveDallaConfermaPerProva(pv, in, strutture, th)
		if cond != c.cond {
			t.Errorf("%s: condizioni nuove %+v, attese %+v", c.nome, cond, c.cond)
		}
		if _, stato, motivi := valutazione.StatoDelProdotto(pv, cond); stato != c.atteso {
			t.Errorf("%s: stato %s, atteso %s (motivi %v)", c.nome, stato, c.atteso, motivi)
		}
	}
}

// ---- M-4: la rianalisi della fonte principale ----

// TestRianalisiDellaFontePrincipaleResta (M-4 della revisione, sonda R8; addendum rev. 2 AD-4c, limite dichiarato [T]
// T-26; domanda D-ADD-05 parte 3, senza risposta): lo STEP della fonte, riconosciuto e non superato, ha un nodo e un arco
// nuovi comparsi dopo la conferma per una rianalisi (analizzatore o configurazione nuovi). L'ingresso non porta il segno
// «sopravvenuta» per le voci della fonte principale: restano un requisito, quindi non_pronto (il verso prudente). La
// prova fissa il comportamento di oggi; con la A della parte 3 diventa da_riesaminare e va riscritta.
func TestRianalisiDellaFontePrincipaleResta(t *testing.T) {
	in := stepConRelazioniACM()
	n9 := ancoraggio.RifNodo(shaFonteACM, "#9")
	in.Struttura.Nodi = append(in.Struttura.Nodi, n9)
	in.Struttura.Archi = append(in.Struttura.Archi, valutazione.RifArco(radiceStepAC, n9))
	v := valutazione.VerificaDallaConferma(in)
	if v.Verificata || asse(v.Nomenclatura) != "da_verificare/da_decidere" || asse(v.Gerarchia) != "da_verificare/da_decidere" {
		t.Errorf("rianalisi della fonte principale: %s", esitoConferma(v))
	}
	pv := pvACM(v, fonteConfermataACM())
	cond := valutazione.CondizioniNuoveDallaConfermaPerProva(pv, in, nil, fotorfq.Thread{})
	if _, stato, _ := valutazione.StatoDelProdotto(pv, cond); cond != (valutazione.CondizioniNuove{}) || stato != valutazione.ProdottoNonPronto {
		t.Errorf("D-ADD-05 parte 3 (T-26): condizioni nuove %+v, stato %s; attesi {}, non_pronto", cond, stato)
	}
}

// ---- M-6: lo scenario D, la parte di oggi ----

// TestScenarioDRevisioneDocumentale (direttiva 12 §10, scenario D [U]: «Arriva una nuova revisione PDF/TIFF senza STEP.
// Non inventare nuove relazioni»; §3.2: «Distinguere tuttavia una revisione puramente documentale da una modifica
// strutturale dimostrata»; addendum AD-4c, AD-7; M-6 della revisione). La parte che non aspetta D-ADD-04: il PDF non
// entra nell'ingresso della verifica della BOM, che resta verificata, con gli stessi archi certificati e nessun
// cambiamento. Oggi (nessun percorso persistito, LD-18) il PDF è un file pertinente non terminale: lo smistamento non è
// verificato e il prodotto è da_riesaminare (T-E1-14). Con il gesto futuro, un percorso sintetico inbox_nuovo_cad aperto
// sul prodotto: smistamento in_revisione, motivo revisione_tecnica_aperta, prodotto da_riesaminare, la BOM sempre
// verificata. Chiuso il percorso e confermato il file, il prodotto torna pronto. Situazione e Riesame (P-07) non ci sono:
// aspettano D-ADD-04.
func TestScenarioDRevisioneDocumentale(t *testing.T) {
	in := ricostruzioneACM()
	prima := jsonDi(t, in)
	v := valutazione.VerificaDallaConferma(in)
	if !v.Verificata || len(v.Conferma.Cambiamenti) != 0 || len(v.Conferma.Archi) != len(qtaACM) || v.Conferma.SecondarieAperte != 0 {
		t.Fatalf("scenario D: la BOM confermata non è verificata: %s, %+v", esitoConferma(v), v.Conferma)
	}
	if jsonDi(t, in) != prima {
		t.Errorf("scenario D: la regola ha cambiato l'ingresso")
	}
	pdf := uid(0x9c60)
	ingresso := func(terminale bool) valutazione.IngressoSmistamento {
		return valutazione.IngressoSmistamento{Prodotto: "ACM-9000", Elementi: []string{rifC(finitoACM)}, Calcolata: true,
			File: []valutazione.FileDelloSmistamento{fileS(pdf, terminale, ancoraggio.AssociazioneCandidatoUnico, ancoraggio.CollocazioneRadice, false)}}
	}
	aperto := percorso("rev-acm-1", valutazione.OriginePercorsoInboxNuovoCAD, true, "ACM-9000")
	chiuso := percorso("rev-acm-1", valutazione.OriginePercorsoInboxNuovoCAD, false, "ACM-9000")
	for _, c := range []struct {
		nome        string
		smistamento valutazione.VerificaSmistamento
		stato       valutazione.StatoSmistamento
		atteso      valutazione.StatoProdotto
		revisione   bool
	}{
		{"oggi: il PDF pertinente non terminale, nessun percorso", valutazione.Smistamento(ingresso(false), nil),
			valutazione.SmistamentoDaVerificare, valutazione.ProdottoDaRiesaminare, false},
		{"gesto futuro: percorso inbox_nuovo_cad aperto sul prodotto", valutazione.Smistamento(ingresso(false), []valutazione.PercorsoRevisione{aperto}),
			valutazione.SmistamentoInRevisione, valutazione.ProdottoDaRiesaminare, true},
		{"percorso chiuso e PDF confermato", valutazione.Smistamento(ingresso(true), []valutazione.PercorsoRevisione{chiuso}),
			valutazione.SmistamentoVerificato, valutazione.ProdottoProntoFattibilita, false},
	} {
		if c.smistamento.Stato != c.stato {
			t.Errorf("%s: smistamento %s, atteso %s", c.nome, c.smistamento.Stato, c.stato)
		}
		pv := pvACM(v, fonteConfermataACM())
		pv.Smistamento = c.smistamento
		cond := valutazione.CondizioniNuoveDallaConfermaPerProva(pv, in, nil, fotorfq.Thread{})
		_, stato, motivi := valutazione.StatoDelProdotto(pv, cond)
		if stato != c.atteso || slices.Contains(motivi, valutazione.MotivoProdottoNomenclatura) || slices.Contains(motivi, valutazione.MotivoProdottoGerarchia) ||
			slices.Contains(motivi, valutazione.MotivoProdottoRevisioneTecnicaAperta) != c.revisione {
			t.Errorf("%s: stato %s, atteso %s, motivi %v", c.nome, stato, c.atteso, motivi)
		}
	}
}

// ---- B-9: il criterio esposto di una conferma senza archi che ora ne ha ----

// TestConfermaSenzaArchiCheOraNeHa (B-9 della revisione, mutazione RM22): una conferma con il criterio step e senza archi,
// e ora un arco corrente. Il motivo è struttura_cambiata_dopo_conferma; il prodotto non è «senza figli» (gli archi correnti
// contano), quindi il criterio esposto è quello della conferma, step, e non nessuno.
func TestConfermaSenzaArchiCheOraNeHa(t *testing.T) {
	in := ricostruzioneACM()
	in.Conferma = confermaManualeACM(nil)
	in.ArchiCorrenti = []valutazione.ArcoCorrente{{PadreID: finitoACM, FiglioID: figlioACM(0), Qta: 1}}
	v := valutazione.VerificaDallaConferma(in)
	m := "da_verificare/" + valutazione.MotivoAsseStrutturaCambiata
	if v.Verificata || asse(v.Nomenclatura) != m || asse(v.Gerarchia) != m || v.Criterio != valutazione.CriterioStep || v.SenzaFigli {
		t.Errorf("conferma senza archi, un arco corrente: %s, senza figli %v", esitoConferma(v), v.SenzaFigli)
	}
}

// ---- M-3: nessun innesto dei campi provvisori prima delle risposte ----

// campiP4Provvisori: vero finché le domande D-ADD-03 (provenienza: il valore indeterminata, BaseGerarchia) e D-ADD-04
// (riesame pendente) dell'addendum non hanno una risposta [U]. Lo cambia a falso chi innesta la regola in Calcola (B3.5),
// nello stesso commit che aggiorna il contratto e le prove (direttiva 12 §4 [U]: «Non inventare un nuovo valore pubblico
// senza aggiornare il contratto e le prove»).
const campiP4Provvisori = true

// TestNessunInnestoPrimaDelleRisposte (M-3 della revisione; addendum rev. 2 §8, consegne a B3.5): finché i campi [P]
// sono provvisori, nessun file di produzione di internal/ fuori da bom_conferma.go chiama la regola della conferma o
// nomina i suoi valori provvisori. Così indeterminata, BaseGerarchia e la provenienza per arco non arrivano al JSON
// pubblico prima della risposta dell'utente. La prova legge i sorgenti, non il DB.
func TestNessunInnestoPrimaDelleRisposte(t *testing.T) {
	if !campiP4Provvisori {
		return
	}
	nomi := []string{"VerificaDallaConferma", "verificaDallaConferma", "condizioniNuoveDallaConferma", "OrigineGerarchiaIndeterminata",
		"BaseGerarchia", "ProvenienzaArchi", "ProvenienzaIndeterminata"}
	proprio, err := filepath.Abs("bom_conferma.go")
	if err != nil {
		t.Fatal(err)
	}
	letti := 0
	err = filepath.WalkDir(filepath.Join("..", ".."), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		if a, err := filepath.Abs(p); err == nil && a == proprio {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		letti++
		for i, riga := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(riga), "//") {
				continue // un commento che nomina la regola non la innesta
			}
			for _, n := range nomi {
				if strings.Contains(riga, n) {
					t.Errorf("%s:%d nomina %s mentre i campi [P] sono provvisori (D-ADD-03, D-ADD-04 senza risposta)", p, i+1, n)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if letti < 50 {
		t.Fatalf("letti solo %d sorgenti di internal/: la prova non guarda dove deve", letti)
	}
}

// jsonDi: il JSON di un valore, per confrontare due uscite o un ingresso prima e dopo.
func jsonDi(t *testing.T, x any) string {
	t.Helper()
	b, err := json.Marshal(x)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
