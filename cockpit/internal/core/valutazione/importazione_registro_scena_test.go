// L1 — l'adattatore del registro d'importazione sulle scene della valutazione (V1: emendamento di V1 §3.8, §3.8.1,
// §3.8.2, §6.1, §11.2; piano della 0023 §14 passo 3, prove A-01…A-19 e A-23 di A3 §1.9 e PM23 §10). Le strutture si
// calcolano con Calcola sulle fotografie sintetiche, le regole di lettura con valutazione.RegoleDiLettura, poi
// propostabom.ImportaGruppo compone il registro con importazione.Componi, e ogni registro si rilegge con
// importazione.Leggi. È una prova di propostabom che sta qui perché usa le scene della valutazione (come T-09 della
// sostituzione). Tutto è inventato: ACME, BETA, i codici 712xxxx, gli UUID di prova.
package valutazione_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// calcoloPerIlRegistro: un calcolo con tutto ciò che serve all'adattatore.
type calcoloPerIlRegistro struct {
	f   fotorfq.Fotografia
	e   valutazione.Esito
	et  valutazione.EsitoThread
	reg valutazione.RegoleDelThread
}

// calcolaPerIlRegistro: Calcola sulla fotografia del thread (la vista calcolata come la dà v_fascicolo), con le regole
// date, e le regole di lettura del thread sulla stessa fotografia.
func calcolaPerIlRegistro(t *testing.T, th fotorfq.Thread, r *motorea.InsiemeRegole) calcoloPerIlRegistro {
	t.Helper()
	vistaCome(&th)
	f := fotografia(th)
	e := calcola(t, f, r, valutazione.Ingressi{})
	reg, ok := valutazione.RegoleDiLettura(f, r, th.ID)
	if !ok {
		t.Fatal("RegoleDiLettura: il thread non c'è")
	}
	return calcoloPerIlRegistro{f: f, e: e, et: esitoThread(t, e, th.ID), reg: reg}
}

func (c calcoloPerIlRegistro) ingresso(prodotto, gruppo string, na propostabom.NonAttivi) propostabom.Ingresso {
	e, et := c.e, c.et
	return propostabom.Ingresso{Esito: &e, Thread: &et, Regole: c.reg, Prodotto: prodotto, Gruppo: gruppo, NonAttivi: na}
}

// importa: l'adattatore sul gruppo, che deve riuscire, e le regole della foglia sull'uscita vera (A-29, la parte della
// corsia motore; R-2 e R-3 della corsia persistenza): la rilettura stretta dà gli stessi byte e la stessa impronta,
// Componi(Leggi(x).Contenuto) gli stessi byte, e le colonne della testata ricavate dal registro lo rileggono come fa il
// database (Rileggi).
func (c calcoloPerIlRegistro) importa(t *testing.T, prodotto, gruppo string, na propostabom.NonAttivi) importazione.Registro {
	t.Helper()
	r, err := propostabom.ImportaGruppo(c.ingresso(prodotto, gruppo, na))
	if err != nil {
		t.Fatalf("ImportaGruppo(%s, %s): %v", prodotto, gruppo, err)
	}
	l, err := importazione.Leggi(r.Canonico)
	if err != nil {
		t.Fatalf("il registro dell'adattatore non si rilegge: %v", err)
	}
	if !bytes.Equal(l.Canonico, r.Canonico) || l.Impronta != r.Impronta {
		t.Fatal("la rilettura del registro dell'adattatore cambia i byte")
	}
	k, err := importazione.Componi(l.Contenuto)
	if err != nil || !bytes.Equal(k.Canonico, r.Canonico) {
		t.Fatalf("Componi(Leggi(x).Contenuto) non dà gli stessi byte: %v", err)
	}
	if _, err := importazione.Rileggi(r.Colonne(), r.Canonico); err != nil {
		t.Fatalf("il registro non si rilegge con le sue colonne: %v", err)
	}
	return r
}

// voceDelRegistro: la voce del registro con quel rif; fallisce se non c'è.
func voceDelRegistro(t *testing.T, c importazione.Contenuto, rif string) importazione.Voce {
	t.Helper()
	for _, v := range c.Proposta.Nodi {
		if v.Origine.Rif == rif {
			return v
		}
	}
	t.Fatalf("il registro non ha la voce %s", rif)
	return importazione.Voce{}
}

// campoDi: un campo di un oggetto JSON.
func campoDi(t *testing.T, raw json.RawMessage, campo string) json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("non è un oggetto JSON: %s", raw)
	}
	return m[campo]
}

func testoJSON(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var s string
	if len(raw) == 0 {
		return ""
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("non è un testo JSON: %s", raw)
	}
	return s
}

// TestA01IlRegistroDeterministico (A-01; PM23 T-09, la parte del determinismo): lo stesso ingresso dà gli stessi byte e
// le stesse impronte; l'ordine degli elenchi della fotografia non conta (permuta, l'aiuto delle prove di Calcola);
// nessun orario e nessun ID inventato nel contenuto confrontato.
func TestA01IlRegistroDeterministico(t *testing.T) {
	r := insiemeACME(t)
	c := calcolaPerIlRegistro(t, scenaCollegamento(t), r)
	a := c.importa(t, rifProdB, propostabom.GruppoBOM, nil)
	b := c.importa(t, rifProdB, propostabom.GruppoBOM, nil)
	if !bytes.Equal(a.Canonico, b.Canonico) || a.Impronta != b.Impronta {
		t.Fatal("due importazioni dello stesso ingresso danno byte diversi")
	}
	th := scenaCollegamento(t)
	vistaCome(&th)
	f, in := permuta(fotografia(th), valutazione.Ingressi{})
	e := calcola(t, f, r, in)
	et := esitoThread(t, e, th.ID)
	reg, _ := valutazione.RegoleDiLettura(f, r, th.ID)
	p, err := propostabom.ImportaGruppo(propostabom.Ingresso{Esito: &e, Thread: &et, Regole: reg, Prodotto: rifProdB, Gruppo: propostabom.GruppoBOM})
	if err != nil {
		t.Fatal(err)
	}
	if p.Contenuto.ImprontaContenuto != a.Contenuto.ImprontaContenuto || p.Contenuto.ImprontaEvidenze != a.Contenuto.ImprontaEvidenze ||
		p.Contenuto.ImprontaRegole != a.Contenuto.ImprontaRegole {
		t.Fatalf("con la fotografia permutata le impronte parziali cambiano:\n%s\n%s", p.Canonico, a.Canonico)
	}
}

// TestA02A03A12LIdentitaDaForma (A-02, A-03, A-12; EM §3.8, §6.1): sulla scena del collegamento (il finito 7120100A con
// lo STEP autorizzato, la BOM di lavoro), l'identità proposta di ogni voce viene da CatenaCodice.Forma con l'emittente
// esplicito (il cliente con il suo ID e l'hash della sua grammatica, mai vuoto), il letterale byte per byte
// (testo_unita[inizio:fine]), base, marcatore e forma della lettura; la radice è la voce della radice del gruppo, una
// radice del file; codice e rev proposti sono le proiezioni di importazione-1.
func TestA02A03A12LIdentitaDaForma(t *testing.T) {
	c := calcolaPerIlRegistro(t, scenaCollegamento(t), insiemeACME(t))
	reg := c.importa(t, rifProdB, propostabom.GruppoBOM, nil)
	k := reg.Contenuto
	if k.Gruppo.Chiave != propostabom.GruppoBOM || k.Gruppo.Radice != nodoB("#1") || k.Gruppo.AllegatoID != aStepB.String() || k.Gruppo.Sha256 != shaStepB ||
		k.Proposta.Fonte.AllegatoID != aStepB.String() || k.ProdottoID != cProdotto.String() || k.ThreadID != threadACME.String() {
		t.Fatalf("testata e gruppo: %+v %+v", k.Gruppo, k.Proposta.Fonte)
	}
	if len(k.Proposta.Nodi) != 4 || len(k.Proposta.FuoriPerDecisione) != 0 {
		t.Fatalf("voci %d, fuori %d: attese 4 e 0", len(k.Proposta.Nodi), len(k.Proposta.FuoriPerDecisione))
	}
	radice := voceDelRegistro(t, k, nodoB("#1"))
	if !k.EVoceDellaRadice(radice) || string(radice.RadiceDelFile) != "true" || string(radice.Foglia) != "false" || len(radice.Padri) != 0 {
		t.Errorf("la voce della radice: %+v", radice)
	}
	cliente := "cliente:" + clienteACME.String()
	for _, x := range []struct{ chiave, letterale, codice string }{{"#1", "7120100A", "7120100A"}, {"#2", "7120200A", "7120200A"},
		{"#3", "7120300A", "7120300A"}, {"#4", "7120400A", "7120400A"}} {
		v := voceDelRegistro(t, k, nodoB(x.chiave))
		var l struct {
			Emittente, HashGrammatica, Namespace, Famiglia, Forma, Letterale, TestoUnita string
			Intervallo                                                                   intervalloJSON
			Base                                                                         struct{ Originale, Normalizzata string }
			Qualificatori                                                                struct{ Marcatore *struct{ Valore string } }
		}
		if err := json.Unmarshal(v.IdentitaProposta, &l); err != nil {
			t.Fatalf("%s: %v", x.chiave, err)
		}
		var raw map[string]json.RawMessage
		_ = json.Unmarshal(v.IdentitaProposta, &raw)
		l.HashGrammatica, l.TestoUnita = testoJSON(t, raw["hash_grammatica"]), testoJSON(t, raw["testo_unita"])
		if l.Emittente != cliente || l.HashGrammatica == "" || l.HashGrammatica != c.et.HashSnapshot || l.Namespace != "acme-catena" ||
			l.Famiglia != "acme-catena" || l.Forma != "step" || l.Letterale != x.letterale || l.Base.Normalizzata != x.letterale[:7] ||
			l.Qualificatori.Marcatore == nil || l.Qualificatori.Marcatore.Valore != "A" {
			t.Errorf("%s: identità proposta %s", x.chiave, v.IdentitaProposta)
		}
		if l.TestoUnita[l.Intervallo.Inizio:l.Intervallo.Fine] != l.Letterale {
			t.Errorf("%s: il letterale non è testo_unita[inizio:fine]", x.chiave)
		}
		if v.CodiceProposto != x.codice || v.RevProposta != "" || len(v.MotivoProposto) != 0 {
			t.Errorf("%s: codice proposto %q, rev %q, motivo %s", x.chiave, v.CodiceProposto, v.RevProposta, v.MotivoProposto)
		}
		if testoJSON(t, campoDi(t, v.IdentitaProposta, "chiave_lettura")) == "" || !strings.HasPrefix(testoJSON(t, campoDi(t, v.IdentitaProposta, "chiave_lettura")), aStepB.String()+"/") {
			t.Errorf("%s: la chiave della lettura non è dell'allegato della voce", x.chiave)
		}
	}
	// il codice composto del cartiglio resta un'evidenza, accanto (la forma «step» non legge la revisione: R87)
	if cc := voceDelRegistro(t, k, nodoB("#2")).CodiceComposto; testoJSON(t, campoDi(t, cc, "motivo")) != motorea.MotivoComposizioneRevisioneNonDeterminata {
		t.Errorf("codice composto di #2: %s", cc)
	}
}

// intervalloJSON: un intervallo {inizio, fine} letto dal JSON del registro.
type intervalloJSON struct {
	Inizio int `json:"inizio"`
	Fine   int `json:"fine"`
}

// TestA10NessunCodiceLegacy (A-10, A-10b la parte del registro; EM §3.8, §6.1): con una riga legacy di un altro codice
// (il classificatore) il codice proposto è quello del motore; il codice corretto a mano su una riga aperta è
// codice_manuale_legacy, con la rev e la riga, mai il codice di partenza; il componente della riga decisa è
// decisione_esistente.
func TestA10NessunCodiceLegacy(t *testing.T) {
	c := calcolaPerIlRegistro(t, scenaCollegamento(t), insiemeACME(t))
	k := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	v2, v3, v4 := voceDelRegistro(t, k, nodoB("#2")), voceDelRegistro(t, k, nodoB("#3")), voceDelRegistro(t, k, nodoB("#4"))
	if v2.CodiceProposto != "7120200A" || v4.CodiceProposto != "7120400A" {
		t.Errorf("il codice del classificatore legacy diventa codice di partenza: %q %q", v2.CodiceProposto, v4.CodiceProposto)
	}
	if v3.CodiceProposto != "7120300A" {
		t.Errorf("il codice corretto a mano diventa codice di partenza: %q", v3.CodiceProposto)
	}
	var cm struct{ Codice, Rev, RigaLegacy string }
	if err := json.Unmarshal(v3.CodiceManualeLegacy, &cm); err != nil || cm.Codice != "7120300B" || cm.Rev != "2" {
		t.Errorf("codice_manuale_legacy di #3: %s", v3.CodiceManualeLegacy)
	}
	if testoJSON(t, campoDi(t, v3.CodiceManualeLegacy, "riga_legacy")) != rigaB3.String() {
		t.Errorf("la riga legacy del codice manuale: %s", v3.CodiceManualeLegacy)
	}
	if len(v2.CodiceManualeLegacy) != 0 || len(v4.CodiceManualeLegacy) != 0 {
		t.Errorf("un codice del classificatore (origine famiglia) è un codice manuale: %s %s", v2.CodiceManualeLegacy, v4.CodiceManualeLegacy)
	}
	var de struct {
		ComponenteID uuid.UUID `json:"componente_id"`
		Codice, Rev  string
	}
	if err := json.Unmarshal(v2.DecisioneEsistente, &de); err != nil || de.ComponenteID != cSciolto || de.Codice != "7120200A" || de.Rev != "1" {
		t.Errorf("decisione_esistente di #2: %s", v2.DecisioneEsistente)
	}
	if len(v3.DecisioneEsistente) != 0 {
		t.Errorf("una riga aperta dà decisione_esistente: %s", v3.DecisioneEsistente)
	}
}

// TestA11ToltiENonRaggiunti (A-11; EM §3.8, P-8): un nodo tolto da una persona (la sua riga scartata con chi l'ha
// decisa) e i nodi raggiunti solo da un arco tolto da una persona non sono voci: stanno fra i nodi fuori per decisione,
// con il segno e la traccia che l'esito ha; gli archi verso di loro restano nel registro. La radice non è mai fuori.
func TestA11ToltiENonRaggiunti(t *testing.T) {
	nodi := []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}, {"#4", "7120400A", ""}, {"#5", "7120500A", ""}}
	archi := []arcoF{{"#1", "#2", 1}, {"#1", "#3", 1}, {"#3", "#4", 3}, {"#1", "#5", 1}}
	th := scenaBOM(t, nodi, archi)
	op := operatore
	decidi(t, &th, "#5", "scartata", uuid.Nil, &op)
	a := arcoDi(t, &th, "#1", "#3")
	a.Stato, a.DecisoDa = "scartata", &op
	c := calcolaPerIlRegistro(t, th, insiemeACME(t))
	k := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	segni := map[string]importazione.FuoriPerDecisione{}
	for _, x := range k.Proposta.FuoriPerDecisione {
		_, rif, _ := importazione.PartiDellaVoce(x.Voce)
		segni[rif] = x
	}
	if len(segni) != 3 || segni[nodoB("#5")].Segno != importazione.SegnoTolto || segni[nodoB("#3")].Segno != importazione.SegnoNonRaggiunto ||
		segni[nodoB("#4")].Segno != importazione.SegnoNonRaggiunto {
		t.Fatalf("fuori per decisione: %+v", k.Proposta.FuoriPerDecisione)
	}
	if testoJSON(t, segni[nodoB("#5")].Chi) != operatore.String() {
		t.Errorf("chi ha tolto #5: %s", segni[nodoB("#5")].Chi)
	}
	if len(k.Proposta.Nodi) != 2 {
		t.Errorf("voci %d: attese la radice e #2", len(k.Proposta.Nodi))
	}
	if len(k.Proposta.Archi) != 4 {
		t.Errorf("archi %d: attesi i quattro dei fatti, anche verso i nodi fuori", len(k.Proposta.Archi))
	}
}

// TestA13ISegnaliCopiati (A-13; EM §3.8, §11.1): i segnali LS-02 dei cordoni arrivano nelle voci byte per byte come
// NodoProposto.Segnali; le altre voci non ne hanno.
func TestA13ISegnaliCopiati(t *testing.T) {
	c := calcolaPerIlRegistro(t, scenaCordoni(t, "Saldatura"), insiemeACME(t))
	k := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	st := bomDellEsito(t, c.et, rifProdB)
	segnalati := 0
	for _, n := range st.Nodi {
		v := voceDelRegistro(t, k, n.Rif)
		if len(n.Segnali) == 0 {
			if len(v.Segnali) != 0 {
				t.Errorf("%s: segnali nel registro senza segnali nel nodo", n.Rif)
			}
			continue
		}
		segnalati++
		atteso, _ := jsoncanonico.Codifica(n.Segnali)
		avuto, _ := jsoncanonico.Codifica(json.RawMessage(v.Segnali))
		if !bytes.Equal(atteso, avuto) {
			t.Errorf("%s: segnali\n%s\nattesi\n%s", n.Rif, avuto, atteso)
		}
	}
	if segnalati != 3 {
		t.Errorf("voci segnalate %d, attesi i tre cordoni", segnalati)
	}
}

// TestA14GliErroriDellImportazione (A-14; EM §6.1, «Errori dell'importazione»): il gruppo «senza», un gruppo sconosciuto,
// il gruppo di un altro prodotto, un prodotto che non c'è e lo STEP del gruppo non attivo sono rifiuti con il codice e il
// motivo; un thread con un errore della valutazione non si importa.
func TestA14GliErroriDellImportazione(t *testing.T) {
	th := scenaCollegamento(t)
	secondoProdotto(t, &th)
	c := calcolaPerIlRegistro(t, th, insiemeACME(t))
	rifiuto := func(t *testing.T, in propostabom.Ingresso, codice, motivo string) {
		t.Helper()
		_, err := propostabom.ImportaGruppo(in)
		var e *importazione.Errore
		if !errors.As(err, &e) || e.Codice != codice || e.Motivo != motivo {
			t.Errorf("atteso %s/%s, avuto %v", codice, motivo, err)
		}
	}
	rifiuto(t, c.ingresso(rifProdB, "inventato", nil), propostabom.CodiceCorpoNonValido, propostabom.MotivoGruppoNonImportabile)
	rifiuto(t, c.ingresso(rifProdB, propostabom.GruppoSenzaSTEP, nil), propostabom.CodiceCorpoNonValido, propostabom.MotivoGruppoNonImportabile)
	rifiuto(t, c.ingresso("componente:"+uid(0x7ff).String(), propostabom.GruppoBOM, nil), propostabom.CodiceCorpoNonValido, propostabom.MotivoGruppoNonImportabile)
	rifiuto(t, c.ingresso(rifProdB, propostabom.GruppoBOM, propostabom.NonAttivi{aStepB: "altri_file"}), propostabom.CodiceFileNonAttivo, "")
	// il gruppo di un altro prodotto: una chiave che il secondo prodotto ha e il primo no
	altri := map[string]bool{}
	for _, pv := range c.et.ProdottiValutati {
		if pv.Rif == rifProdB {
			continue
		}
		for _, g := range propostabom.GruppiDelProdotto(pv, c.et.Ancoraggi.Strutture) {
			altri[g.Chiave] = true
		}
	}
	pv := prodottoDi(t, c.et, rifProdB)
	for _, g := range propostabom.GruppiDelProdotto(pv, c.et.Ancoraggi.Strutture) {
		delete(altri, g.Chiave)
	}
	for k := range altri {
		if k != propostabom.GruppoSenzaSTEP {
			rifiuto(t, c.ingresso(rifProdB, k, nil), propostabom.CodiceCorpoNonValido, propostabom.MotivoGruppoNonImportabile)
		}
	}
	errato := c.et
	errato.Motivo = valutazione.MotivoThreadErroreValutazione
	e := c.e
	rifiuto(t, propostabom.Ingresso{Esito: &e, Thread: &errato, Regole: c.reg, Prodotto: rifProdB, Gruppo: propostabom.GruppoBOM},
		importazione.CodicePropostaNonDisponibile, propostabom.MotivoErroreValutazione)
}

// TestA15GliArchi (A-15; EM §3.8, «proposta.archi[]»): un arco per ogni arco della struttura, con la quantità dei fatti;
// un figlio condiviso è una voce sola con due archi e i suoi due padri.
func TestA15GliArchi(t *testing.T) {
	nodi := []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}, {"#4", "7120400A", ""}}
	archi := []arcoF{{"#1", "#2", 2}, {"#1", "#3", 1}, {"#2", "#4", 3}, {"#3", "#4", 1}}
	c := calcolaPerIlRegistro(t, scenaBOM(t, nodi, archi), insiemeACME(t))
	k := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	if len(k.Proposta.Archi) != 4 {
		t.Fatalf("archi %d: %+v", len(k.Proposta.Archi), k.Proposta.Archi)
	}
	qta := map[[2]string]int{}
	for _, a := range k.Proposta.Archi {
		if a.Quantita == nil || a.AllegatoID != aStepB.String() {
			t.Fatalf("arco senza quantità o di un altro allegato: %+v", a)
		}
		qta[[2]string{a.Padre, a.Figlio}] = *a.Quantita
	}
	if qta[[2]string{nodoB("#1"), nodoB("#2")}] != 2 || qta[[2]string{nodoB("#2"), nodoB("#4")}] != 3 || qta[[2]string{nodoB("#3"), nodoB("#4")}] != 1 {
		t.Errorf("quantità %v", qta)
	}
	v4 := voceDelRegistro(t, k, nodoB("#4"))
	var padri []string
	_ = json.Unmarshal(v4.Padri, &padri)
	if len(padri) != 2 || padri[0] != nodoB("#2") || padri[1] != nodoB("#3") || string(v4.Foglia) != "true" {
		t.Errorf("il figlio condiviso: padri %v, foglia %s", padri, v4.Foglia)
	}
}

// TestA16LeDiagnosticheNonSiPerdono (A-16; EM §3.8): le diagnostiche dell'esito per i file del registro (il documento e
// l'interpretazione dello STEP e degli altri file, l'ancoraggio sul prodotto e sullo STEP del gruppo) stanno tutte
// nelle voci o nella testata del registro, e nessuna diagnostica del registro viene da altrove, salvo quelle
// dell'adattatore (importazione.*); una diagnostica che cita un nodo solo sta nella sua voce, una che ne cita più d'uno
// nella testata, una volta.
func TestA16LeDiagnosticheNonSiPerdono(t *testing.T) {
	nodi := []nodoF{{"#1", "7120100A", "1"}, {"#2", "7120200A", "1"}, {"#3", "7120300A", ""}}
	archi := []arcoF{{"#1", "#2", 1}, {"#1", "#3", 1}}
	th := scenaBOM(t, nodi, archi)
	f := th.Fatti[shaStepB]
	f.MotivoParziale = testo("2 radici nel file: una distinta ne ha una")
	th.Fatti[shaStepB] = f
	c := calcolaPerIlRegistro(t, th, insiemeACME(t))
	k := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	nelRegistro := map[string]bool{}
	chiaveDi := func(d map[string]any) string { b, _ := json.Marshal(d); return string(b) }
	raccogli := func(raw json.RawMessage) []map[string]any {
		var dd []map[string]any
		_ = json.Unmarshal(raw, &dd)
		return dd
	}
	perNodo := map[string][]map[string]any{}
	for _, v := range k.Proposta.Nodi {
		for _, d := range raccogli(v.Diagnostiche) {
			nelRegistro[chiaveDi(d)] = true
			perNodo[v.Origine.Rif] = append(perNodo[v.Origine.Rif], d)
		}
	}
	for _, d := range raccogli(k.Proposta.Diagnostiche) {
		nelRegistro[chiaveDi(d)] = true
	}
	attese := map[string]bool{}
	norm := func(d diag) string {
		rif := []any{}
		for _, r := range d.Rif {
			rif = append(rif, r)
		}
		m := map[string]any{"codice": d.Codice, "gravita": d.Gravita, "natura": d.Natura, "messaggio": d.Messaggio}
		if d.Percorso != "" {
			m["percorso"] = d.Percorso
		}
		if len(rif) > 0 {
			m["rif"] = rif
		}
		return chiaveDi(m)
	}
	step := fileInterpretatoDi(t, c.et, aStepB)
	for _, d := range step.Documento.Qualita.Diagnostiche {
		attese[norm(diag{d.Codice, string(d.Gravita), string(d.Natura), d.Percorso, d.Messaggio, d.Rif})] = true
	}
	for _, d := range step.Interpretazione.Diagnostiche {
		attese[norm(diag{d.Codice, string(d.Gravita), string(d.Natura), d.Percorso, d.Messaggio, d.Rif})] = true
	}
	for _, d := range c.et.Ancoraggi.Diagnostiche {
		target, gruppo := false, false
		for _, r := range d.Rif {
			target = target || r == rifProdB
			gruppo = gruppo || r == "allegato:"+aStepB.String() || strings.HasPrefix(r, "nodo:"+shaStepB)
		}
		if target && gruppo {
			attese[norm(diag{d.Codice, string(d.Gravita), string(d.Natura), d.Percorso, d.Messaggio, d.Rif})] = true
		}
	}
	if len(attese) == 0 {
		t.Fatal("la scena non ha diagnostiche: la prova non prova niente")
	}
	for d := range attese {
		if !nelRegistro[d] {
			t.Errorf("una diagnostica dell'esito si perde: %s", d)
		}
	}
	for d := range nelRegistro {
		if !attese[d] && !strings.Contains(d, `"codice":"importazione.`) {
			t.Errorf("una diagnostica del registro non viene dall'esito: %s", d)
		}
	}
	// ancoraggio.grafo_incompleto cita la radice: sta nella sua voce
	trovata := false
	for _, d := range perNodo[nodoB("#1")] {
		trovata = trovata || d["codice"] == ancoraggio.CodiceGrafoIncompleto
	}
	if !trovata {
		t.Errorf("ancoraggio.grafo_incompleto non sta nella voce della radice: %+v", perNodo[nodoB("#1")])
	}
}

// diag: i campi di una diagnostica, per i confronti.
type diag struct {
	Codice, Gravita, Natura, Percorso, Messaggio string
	Rif                                          []string
}

// TestA17IlPesoDiDuemilaNodi (A-17; EM §3.10; PM23 T-07, la parte del registro): una struttura sintetica di 2000 nodi
// (il tetto del worker) dà un registro sotto il limite della foglia; peso e tempo vanno nel log.
func TestA17IlPesoDiDuemilaNodi(t *testing.T) {
	if testing.Short() {
		t.Skip("prova lunga")
	}
	nodi := []nodoF{{"#1", "7120100A", ""}}
	var archi []arcoF
	for i := 2; i <= 2000; i++ {
		nodi = append(nodi, nodoF{fmt.Sprintf("#%d", i), fmt.Sprintf("712%04dA", i), "1"})
		archi = append(archi, arcoF{"#1", fmt.Sprintf("#%d", i), 1})
	}
	c := calcolaPerIlRegistro(t, scenaBOM(t, nodi, archi), insiemeACME(t))
	inizio := time.Now()
	if _, err := propostabom.ImportaGruppo(c.ingresso(rifProdB, propostabom.GruppoBOM, nil)); err != nil {
		t.Fatal(err)
	}
	durata := time.Since(inizio) // l'adattatore con Componi, come in R2 e in S6
	r := c.importa(t, rifProdB, propostabom.GruppoBOM, nil)
	if len(r.Contenuto.Proposta.Nodi) != 2000 {
		t.Fatalf("voci %d, attese 2000", len(r.Contenuto.Proposta.Nodi))
	}
	if len(r.Canonico) > importazione.MaxContenuto {
		t.Fatalf("il registro di 2000 nodi pesa %d byte, oltre il limite di %d", len(r.Canonico), importazione.MaxContenuto)
	}
	// una diagnostica che cita le unità di più nodi (qui l'avviso di capacità sulle revisioni lette di tutti i nodi) sta
	// una volta nella testata: nessuna diagnostica è ripetuta in due voci
	viste := map[string]string{}
	for _, v := range r.Contenuto.Proposta.Nodi {
		var dd []json.RawMessage
		_ = json.Unmarshal(v.Diagnostiche, &dd)
		for _, d := range dd {
			if prima, ok := viste[string(d)]; ok {
				t.Fatalf("la stessa diagnostica sta in due voci (%s e %s): %s", prima, v.Voce, d)
			}
			viste[string(d)] = v.Voce
		}
	}
	if !strings.Contains(string(r.Contenuto.Proposta.Diagnostiche), `"codice":"capacita.non_supportata"`) {
		t.Errorf("l'avviso di capacità su tutti i nodi non sta nella testata: %.300s", r.Contenuto.Proposta.Diagnostiche)
	}
	t.Logf("A-17: registro di 2000 nodi = %d byte di testo canonico (%.1f MiB; limite %d MiB), composto (ImportaGruppo) in %s",
		len(r.Canonico), float64(len(r.Canonico))/(1<<20), importazione.MaxContenuto>>20, durata)
}

// TestA19IlGrafoDaRileggere (A-19; EM §11.2): una struttura letta da un worker non aggiornato (il motivo della
// regressione della 0020) dà la diagnostica importazione.grafo_da_rileggere con il suo testo; il registro tiene
// grafo_completo falso e il motivo come sono, e il segnale LS-02 il contro-indizio del grafo.
func TestA19IlGrafoDaRileggere(t *testing.T) {
	th := scenaCordoni(t, "Saldatura")
	f := th.Fatti[shaStepB]
	motivo := "struttura v4: gli scarti non sono numerati, serve la rianalisi v3"
	f.MotivoParziale = &motivo
	th.Fatti[shaStepB] = f
	c := calcolaPerIlRegistro(t, th, insiemeACME(t))
	k := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	if string(k.Gruppo.GrafoCompleto) != "false" || testoJSON(t, k.Gruppo.MotivoGrafo) != motivo {
		t.Errorf("il grafo nel registro: %s %s", k.Gruppo.GrafoCompleto, k.Gruppo.MotivoGrafo)
	}
	if !strings.Contains(string(k.Proposta.Diagnostiche), `"codice":"`+propostabom.CodiceImportazioneGrafoDaRileggere+`"`) ||
		!strings.Contains(string(k.Proposta.Diagnostiche), "Rifai l'analisi") {
		t.Errorf("la diagnostica della struttura da rileggere: %s", k.Proposta.Diagnostiche)
	}
	conControIndizio := 0
	for _, v := range k.Proposta.Nodi {
		if strings.Contains(string(v.Segnali), ancoraggio.ControIndizioGrafoNonCompleto) {
			conControIndizio++
		}
	}
	if conControIndizio != 3 {
		t.Errorf("i cordoni con il contro-indizio del grafo: %d", conControIndizio)
	}
	// con un altro motivo (un grafo non completo per un'altra ragione) la diagnostica non c'è
	f.MotivoParziale = testo("2 radici nel file: una distinta ne ha una")
	th.Fatti[shaStepB] = f
	k = calcolaPerIlRegistro(t, th, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	if strings.Contains(string(k.Proposta.Diagnostiche), propostabom.CodiceImportazioneGrafoDaRileggere) {
		t.Errorf("la diagnostica della regressione con un altro motivo: %s", k.Proposta.Diagnostiche)
	}
}

// TestA05LettureDiscordanti (A-05; EM §3.8; T-E1-07): un nodo il cui id dà due identità che non concordano (due famiglie
// della stessa grammatica lo leggono con basi diverse) non ha l'identità proposta; tutte e due le letture stanno fra le
// alternative, il codice di partenza è vuoto con il motivo. I nomi dei nodi non bastano: nodo_step.nome è riservato in
// A1 (capacitaRiservate), e una forma che lo nomina non entra nel motore.
func TestA05LettureDiscordanti(t *testing.T) {
	th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120300A", ""}}, []arcoF{{"#1", "#2", 1}})
	c := calcolaPerIlRegistro(t, th, insiemeConLaFamigliaAlternativa(t))
	k := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	v := voceDelRegistro(t, k, nodoB("#2"))
	if len(v.IdentitaProposta) != 0 || v.CodiceProposto != "" || testoJSON(t, v.MotivoProposto) != ancoraggio.MotivoLettureDiscordanti ||
		testoJSON(t, v.MotivoLettura) != ancoraggio.MotivoLettureDiscordanti || v.RevProposta != "" {
		t.Fatalf("la voce discordante: identità %s, codice %q, rev %q, motivi %s %s", v.IdentitaProposta, v.CodiceProposto, v.RevProposta,
			v.MotivoProposto, v.MotivoLettura)
	}
	var alt []map[string]any
	if err := json.Unmarshal(v.Alternative, &alt); err != nil || len(alt) != 2 {
		t.Fatalf("le alternative: %s", v.Alternative)
	}
	famiglie := map[string]bool{}
	for _, a := range alt {
		famiglie[fmt.Sprint(a["famiglia"])] = true
		if a["letterale"] != "7120300A" || a["emittente"] != "cliente:"+clienteACME.String() {
			t.Errorf("un'alternativa: %v", a)
		}
	}
	if !famiglie["acme-catena"] || !famiglie["acme-alt"] {
		t.Errorf("le alternative non sono le due letture: %v", famiglie)
	}
	// la radice, letta da una famiglia sola, ha la sua identità
	if r := voceDelRegistro(t, k, nodoB("#1")); len(r.IdentitaProposta) == 0 || r.CodiceProposto != "7120100A" || len(r.Alternative) != 0 {
		t.Errorf("la radice: identità %s, codice %q, alternative %s", r.IdentitaProposta, r.CodiceProposto, r.Alternative)
	}
}

// insiemeConLaFamigliaAlternativa: ACME con la famiglia della catena e una seconda famiglia (acme-alt) che legge gli id
// 71203xxA dei nodi come base intera, senza marcatore: per quegli id le due letture non concordano (T-E1-07).
func insiemeConLaFamigliaAlternativa(t *testing.T) *motorea.InsiemeRegole {
	t.Helper()
	alt := grammatica.FamigliaCodice{
		ID: "acme-alt", Namespace: "acme-alt",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloComponente},
		Base: grammatica.Base{
			Segmenti:  []grammatica.SegmentoBase{{Nome: "codice", Pattern: "71203[0-9]{2}A", Identitario: true}},
			Maiuscole: grammatica.MaiuscoleEsatte, Normalizza: grammatica.NormalizzaNessuna,
			ConfinePrima: grammatica.ConfineAlnumASCII, ConfineDopo: grammatica.ConfineAlnumASCII,
		},
		Forme: []grammatica.FormaCodice{formaCatena("alt", []string{"nodo_step.id"}, parteC(grammatica.TipoParteBase))},
		Esempi: []grammatica.EsempioCodice{{ID: "e-alt", Origine: grammatica.OrigineSintetico, Selettore: "nodo_step.id", Testo: "7120300A",
			Atteso: grammatica.AttesoEsempio{Letture: []grammatica.LetturaAttesa{{Forma: "alt", Base: "7120300A"}}, AltreAmmesse: true}}},
	}
	r := insiemeDi(t, voceRegole{cliente: clienteACME, file: "acme.v1.json", byte: grammaticaDi(t, clienteACME, "ACME S.p.A.", famigliaCatena(), alt)})
	if r.Motori[clienteACME] == nil {
		t.Fatalf("la grammatica con la famiglia alternativa non compila: %+v", r.Scartati)
	}
	return r
}

// TestA23IlContestoNelRegistro (A-23, C-17, C-18 la parte del registro; EM §3.8.2; R-2 della corsia motore, con le
// avvertenze di P-2: nessun eredita_da nella fixture, il contesto confrontato per impronta): con un contesto dichiarato
// per il thread il registro ha i partner attivi, il contesto delle regole, la copia del contesto risolto (con l'impronta
// del contesto) e l'impronta dell'indice; un file nel perimetro ha contesto_id e impronta_contesto, un file fuori dal
// perimetro no. Senza contesto quei campi sono assenti. Un file nel perimetro senza i due campi, o un motore senza
// l'impronta dell'indice, è un registro incompleto: 422, niente registro.
func TestA23IlContestoNelRegistro(t *testing.T) {
	th := scenaCollegamento(t)
	_, hash := insiemeConPartner(t)
	r, _ := insiemeConPartner(t, contestoACME(th.ID, hash, motorea.OccorrenzaContesto{AllegatoID: aStepB.String()}))
	c := calcolaPerIlRegistro(t, th, r)
	k := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	if k.Regole.Partner == nil || len(*k.Regole.Partner) != 1 || (*k.Regole.Partner)[0] != (importazione.PartnerAttivo{Emittente: "partner:beta", Hash: hash}) {
		t.Fatalf("partner attivi: %+v", k.Regole.Partner)
	}
	if k.Regole.Contesto == nil || testoJSON(t, k.Regole.Contesto.ID) != "ctx-acme-v1" || k.Regole.Contesto.Impronta != c.reg.Contesto.Impronta {
		t.Fatalf("contesto delle regole: %+v", k.Regole.Contesto)
	}
	var copia motorea.ContestoCanonico
	if err := json.Unmarshal(k.Contesto, &copia); err != nil {
		t.Fatalf("la copia del contesto: %v", err)
	}
	if imp, err := jsoncanonico.ImprontaDi(copia); err != nil || imp != k.Regole.Contesto.Impronta || copia.Padre != nil {
		t.Errorf("la copia del contesto per impronta: %s (%v), attesa %s; padre %v", imp, err, k.Regole.Contesto.Impronta, copia.Padre)
	}
	if testoJSON(t, campoDi(t, k.Calcolo, "impronta_indice")) != c.e.ImprontaIndice {
		t.Errorf("calcolo.impronta_indice: %s", k.Calcolo)
	}
	dentro, fuori := 0, 0
	for _, x := range k.Proposta.File {
		ctx := testoJSON(t, campoDi(t, x.Interpretazione, "contesto_id"))
		imp := testoJSON(t, campoDi(t, x.Interpretazione, "impronta_contesto"))
		if x.AllegatoID == aStepB.String() {
			dentro++
			if ctx != "ctx-acme-v1" || imp != c.reg.Contesto.Impronta {
				t.Errorf("lo STEP nel perimetro: %s", x.Interpretazione)
			}
			continue
		}
		fuori++
		if ctx != "" || imp != "" {
			t.Errorf("il file %s fuori dal perimetro ha il contesto: %s", x.AllegatoID, x.Interpretazione)
		}
	}
	if dentro != 1 || fuori == 0 {
		t.Fatalf("file nel perimetro %d, fuori %d: la prova vuole lo STEP dentro e un file fuori", dentro, fuori)
	}
	// senza contesto: tutto assente
	s := calcolaPerIlRegistro(t, scenaCollegamento(t), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	if s.Regole.Partner != nil || s.Regole.Contesto != nil || len(s.Contesto) != 0 {
		t.Errorf("senza contesto: partner %+v, contesto %+v, copia %s", s.Regole.Partner, s.Regole.Contesto, s.Contesto)
	}
	// un file nel perimetro senza i due campi: registro incompleto
	senza := c.et
	senza.File = append([]valutazione.FileInterpretato(nil), c.et.File...)
	for i := range senza.File {
		if senza.File[i].AllegatoID == aStepB {
			senza.File[i].Interpretazione.ContestoID, senza.File[i].Interpretazione.ImprontaContesto = "", ""
		}
	}
	e := c.e
	_, err := propostabom.ImportaGruppo(propostabom.Ingresso{Esito: &e, Thread: &senza, Regole: c.reg, Prodotto: rifProdB, Gruppo: propostabom.GruppoBOM})
	var errReg *importazione.Errore
	if !errors.As(err, &errReg) || errReg.Codice != importazione.CodicePropostaNonDisponibile || errReg.Motivo != importazione.MotivoRegistroIncompleto {
		t.Errorf("un file nel perimetro senza contesto_id: atteso registro_incompleto, %v", err)
	}
	// un motore che non espone l'impronta dell'indice
	e.ImprontaIndice = ""
	et := c.et
	_, err = propostabom.ImportaGruppo(propostabom.Ingresso{Esito: &e, Thread: &et, Regole: c.reg, Prodotto: rifProdB, Gruppo: propostabom.GruppoBOM})
	if !errors.As(err, &errReg) || errReg.Motivo != importazione.MotivoRegistroIncompleto {
		t.Errorf("senza l'impronta dell'indice: atteso registro_incompleto, %v", err)
	}
}

// TestT13NienteOrariNelConfronto (T-13, la parte sui campi; EM §3.8.1): nella parte del registro che entra nelle impronte
// parziali (tutto, salvo calcolo, contesto e la testata ripetuta) non c'è un orario, un bundle_id, un ID o un'impronta
// d'interpretazione dell'esito; un orario o un ID di calcolo cambiati in calcolo non cambiano le impronte parziali.
func TestT13NienteOrariNelConfronto(t *testing.T) {
	c := calcolaPerIlRegistro(t, scenaCollegamento(t), insiemeACME(t))
	reg := c.importa(t, rifProdB, propostabom.GruppoBOM, nil)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(reg.Canonico, &m); err != nil {
		t.Fatal(err)
	}
	parti := string(m["gruppo"]) + string(m["proposta"]) + string(m["regole"])
	vietati := map[string]bool{}
	for _, f := range c.et.File {
		for _, x := range []string{f.Documento.BundleID, f.Interpretazione.ID, f.Interpretazione.Impronta, f.Interpretazione.BundleID} {
			if x != "" {
				vietati[x] = true
			}
		}
	}
	vietati[c.e.ImprontaFotografia], vietati[c.e.ImprontaIndice], vietati[c.et.Ancoraggi.Impronta] = true, true, true
	for x := range vietati {
		if strings.Contains(parti, x) {
			t.Errorf("un ID o un'impronta del calcolo (%s) sta nella parte confrontata", x)
		}
	}
	if regexp.MustCompile(`20[0-9]{2}-[01][0-9]-[0-3][0-9]T[0-2][0-9]:`).MatchString(parti) {
		t.Error("un orario sta nella parte confrontata del registro")
	}
	if !strings.Contains(string(m["calcolo"]), "calcolato_il") || !strings.Contains(string(m["calcolo"]), c.et.Ancoraggi.Impronta) {
		t.Errorf("il calcolo non porta i fatti e le impronte: %s", m["calcolo"])
	}
	// un altro momento del calcolo dei fatti (la rianalisi degli stessi byte, RA-04 (a)): le impronte parziali uguali
	th := scenaCollegamento(t)
	f := th.Fatti[shaStepB]
	f.CalcolatoIl = f.CalcolatoIl.Add(48 * time.Hour)
	th.Fatti[shaStepB] = f
	b := calcolaPerIlRegistro(t, th, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil)
	if b.Contenuto.ImprontaContenuto != reg.Contenuto.ImprontaContenuto || b.Contenuto.ImprontaEvidenze != reg.Contenuto.ImprontaEvidenze ||
		b.Contenuto.ImprontaRegole != reg.Contenuto.ImprontaRegole {
		t.Error("un altro momento del calcolo dei fatti cambia le impronte parziali")
	}
	if b.Impronta == reg.Impronta {
		t.Error("un altro momento del calcolo dei fatti non cambia l'impronta intera (il calcolo ci entra)")
	}
	if !utf8.Valid(reg.Canonico) {
		t.Error("il registro non è UTF-8")
	}
}

// TestT13LeProiezioniSullUscitaVera (T-13, la parte sulle proiezioni; PM23 §10; EM §3.8.1): sul registro vero
// dell'adattatore (la scena del collegamento), ogni campo presente di una voce cambiato da solo cambia l'impronta della
// sua classe e solo quella: i campi «contenuto» impronta_contenuto, i campi «evidenze» impronta_evidenze; per la voce
// della radice i campi che vengono dallo stato della RFQ (e stato_motore) stanno fra le evidenze (O-01). La classe la dà
// l'elenco chiuso della foglia (importazione.Classificazione), mai una copia.
func TestT13LeProiezioniSullUscitaVera(t *testing.T) {
	reg := calcolaPerIlRegistro(t, scenaCollegamento(t), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil)
	base, err := importazione.ImpronteParziali(reg.Canonico)
	if err != nil {
		t.Fatal(err)
	}
	classi := importazione.Classificazione()
	radiceEvidenze := map[string]bool{}
	for _, c := range importazione.CampiDellaRadiceFraLeEvidenze {
		radiceEvidenze[c] = true
	}
	var m map[string]any
	if err := json.Unmarshal(reg.Canonico, &m); err != nil {
		t.Fatal(err)
	}
	nodi := m["proposta"].(map[string]any)["nodi"].([]any)
	radice := m["gruppo"].(map[string]any)["radice"].(string)
	altro := func(v any) any {
		switch x := v.(type) {
		case string:
			return x + "x"
		case bool:
			return !x
		case float64:
			return x + 1
		case []any:
			return append(append([]any(nil), x...), "x")
		case map[string]any:
			y := map[string]any{"x_prova": "x"}
			for k, w := range x {
				y[k] = w
			}
			return y
		}
		return "x"
	}
	provati := 0
	for i := range nodi {
		v := nodi[i].(map[string]any)
		dellaRadice := v["origine"].(map[string]any)["rif"] == radice
		campi := map[string]any{}
		for k, w := range v {
			if k == "stato_motore" {
				for s, z := range w.(map[string]any) {
					campi["stato_motore."+s] = z
				}
				continue
			}
			campi[k] = w
		}
		for campo, valore := range campi {
			if campo == "voce" || campo == "origine" {
				continue // cambiarli sposta la voce: non è il cambio di un campo
			}
			classe := classi["proposta.nodi[]."+campo]
			if dellaRadice && (radiceEvidenze[campo] || strings.HasPrefix(campo, "stato_motore.")) {
				classe = importazione.InEvidenze
			}
			if classe != importazione.InContenuto && classe != importazione.InEvidenze {
				t.Errorf("%s: il campo %s non ha una classe della voce (%q)", v["voce"], campo, classe)
				continue
			}
			copia := map[string]any{}
			b, _ := json.Marshal(m)
			_ = json.Unmarshal(b, &copia)
			cv := copia["proposta"].(map[string]any)["nodi"].([]any)[i].(map[string]any)
			if s, ok := strings.CutPrefix(campo, "stato_motore."); ok {
				cv["stato_motore"].(map[string]any)[s] = altro(valore)
			} else {
				cv[campo] = altro(valore)
			}
			b, _ = json.Marshal(copia)
			imp, err := importazione.ImpronteParziali(b)
			if err != nil {
				t.Fatalf("%s.%s: %v", v["voce"], campo, err)
			}
			cambiaC, cambiaE := imp.Contenuto != base.Contenuto, imp.Evidenze != base.Evidenze
			if (classe == importazione.InContenuto) != cambiaC || (classe == importazione.InEvidenze) != cambiaE || imp.Regole != base.Regole {
				t.Errorf("%s.%s (classe %s): contenuto cambiato %v, evidenze cambiate %v", v["voce"], campo, classe, cambiaC, cambiaE)
			}
			provati++
		}
	}
	if provati < 40 {
		t.Errorf("campi provati %d: la scena ne ha troppo pochi", provati)
	}
	t.Logf("T-13: %d campi delle voci provati sul registro vero", provati)
}
