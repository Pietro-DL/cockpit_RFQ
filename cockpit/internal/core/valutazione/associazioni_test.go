// L1 — lo smistamento dalla fotografia, attraverso Calcola (B6, V2; contratto §1.5, §1.7, §2.3, §2.5, §2.6; E1R §6;
// R81, R93, R95 A, R105; T-B0-12, T-B0-25, T-B0-29, T-B0-32; T-E1-05, T-E1-11, T-E1-12; T-E1R-10, T-E1R-11): il
// prodotto altrimenti verificato con la riga della radice aperta (PO-15, parte B6); il file confermato sul componente
// sbagliato (PO-04); il file ambiguo (PO-13); lo scarto con il gesto (PO-14, PO-39); la rinomina dopo la conferma
// (PO-21, parte B6); il nuovo CAD (PO-25, parte dello smistamento); gli orfani (PO-27); il perimetro (PO-32); il
// contesto del messaggio (PO-40); R106 B, R107 e R108 A (le risposte dell'utente del 07/10) sulla scena, in tutte e due
// le direzioni; la destinazione F8; lo smistamento non calcolato (T-12, senza grammatica, il target senza componente);
// il determinismo.
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è pubblico.
package valutazione_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/valutazione"
)

// I file delle scene dello smistamento.
var (
	fX = uid(0xa41)
	fY = uid(0xa42)
	fZ = uid(0xa43)
	fW = uid(0xa44)
	fF = uid(0xa45)
	fG = uid(0xa46)
	fS = uid(0xa47)
	fU = uid(0xa48)
	fV = uid(0xa49)
	fD = uid(0xa4a)
	cQ = uid(0xa4b)
)

// TestPO15SmistamentoVerificato (PO-15, parte B6; R81, T-B0-32): il prodotto altrimenti verificato ha lo smistamento
// verificato, calcolato: ogni file è terminale (lo STEP strutturale e i due 2D confermati), con l'origine confermata, nel
// perimetro, pertinente al prodotto; nessun file da smistare, nessun orfano. La riga della radice legacy resta aperta, e
// non blocca né la BOM né lo smistamento.
func TestPO15SmistamentoVerificato(t *testing.T) {
	th := scenaSmistamento(t)
	if r := rigaDi(t, &th, "#1"); r.Stato != "aperta" || r.ID != rigaRadB {
		t.Fatalf("la riga della radice %+v", r)
	}
	et := esitoSmistamento(t, th)
	p := prodottoDi(t, et, rifProdB)
	if p.Smistamento.Stato != valutazione.SmistamentoVerificato || !p.Smistamento.Calcolata || !p.BOM.Verificata || p.Documenti.Stato != valutazione.DocumentiCompleta {
		t.Fatalf("smistamento %+v, BOM verificata %v, documenti %s", p.Smistamento, p.BOM.Verificata, p.Documenti.Stato)
	}
	for _, a := range et.Associazioni {
		if !a.Terminale || a.Esclusa || a.Conflitto || a.Origine != ancoraggio.OrigineConfermato || a.Perimetro != valutazione.PerimetroDentro ||
			!reflect.DeepEqual(a.Pertinente, []string{rifProdB}) || a.DocumentoID == nil || a.Confermata == nil {
			t.Errorf("associazione %+v", a)
		}
	}
	step := associazioneDi(t, et, aStepB)
	if step.Associazione != ancoraggio.AssociazioneCandidatoUnico || !reflect.DeepEqual(step.Proposta, []string{"prodotto/" + rifProdB}) ||
		*step.DocumentoID != dStepB || *step.Confermata != cProdotto || step.DestinazioneF8 != nil || step.DestinazioneCoincide != nil {
		t.Errorf("lo STEP strutturale %+v", step)
	}
	if len(et.DaSmistare) != 0 || et.Fascicolo.Orfani != 0 || len(conflittiDi(et, valutazione.ConflittoAssociazione)) != 0 {
		t.Errorf("da smistare %+v, orfani %d, conflitti %+v", et.DaSmistare, et.Fascicolo.Orfani, et.Conflitti)
	}
}

// TestPO04IlFileSulComponenteSbagliato (PO-04; R95 A, R62 g A; T-B0-25): la BOM è verificata, ma il 2D confermato sullo
// sciolto ha nel nome il codice del prodotto: l'identità letta adesso porta a un altro componente. Il conflitto di
// associazione blocca lo smistamento (conflitto), con le evidenze dei due lati (il documento, chi e quando l'ha
// confermato; il candidato e la lettura del nome); la decisione resta il valore corrente, e il documento resta presente
// nella completezza. Senza il nome del prodotto nessun conflitto.
func TestPO04IlFileSulComponenteSbagliato(t *testing.T) {
	th := scenaSmistamento(t)
	for i := range th.Allegati {
		if th.Allegati[i].ID == aPDF2 {
			th.Allegati[i].NomeFile = "7120100A_1.pdf"
		}
	}
	et := esitoSmistamento(t, th)
	p := prodottoDi(t, et, rifProdB)
	rif := valutazione.RifDocumento(dPDF2)
	if p.Smistamento.Stato != valutazione.SmistamentoConflitto || motiviDi(p.Smistamento) != "associazione_in_conflitto" ||
		!reflect.DeepEqual(p.Smistamento.Conflitti, []string{rif}) || len(p.Smistamento.FileNonTerminali) != 0 {
		t.Fatalf("smistamento %+v", p.Smistamento)
	}
	if !p.BOM.Verificata || p.Documenti.Stato != valutazione.DocumentiCompleta || esitoDi(voceDi(t, p.Documenti, cSciolto, "disegno_2d")) != "presente/" {
		t.Errorf("BOM %v, documenti %s: il conflitto di associazione non toglie il documento dalla completezza (R62 g A)", p.BOM.Verificata, p.Documenti.Stato)
	}
	c := conflittiDi(et, valutazione.ConflittoAssociazione)
	if len(c) != 1 || c[0].Asse != valutazione.AsseSmistamento || c[0].Rif != rif || c[0].Prodotto != rifProdB ||
		c[0].Decisione != ancoraggio.RifComponente(cSciolto) || c[0].OrigineDecisione != ancoraggio.OrigineConfermato ||
		c[0].Motivo != valutazione.MotivoConflittoComponenteDiverso || c[0].Proposta != "prodotto/"+rifProdB ||
		c[0].EvidenzaDecisione.Da == nil || *c[0].EvidenzaDecisione.Da != operatore || c[0].EvidenzaDecisione.Il == nil ||
		*c[0].EvidenzaProposta.AllegatoID != aPDF2 || *c[0].EvidenzaProposta.DocumentoID != dPDF2 || c[0].EvidenzaProposta.UnitaID == "" {
		t.Fatalf("conflitti %+v", c)
	}
	a := associazioneDi(t, et, aPDF2)
	if !a.Conflitto || !a.Terminale || *a.Confermata != cSciolto || a.Origine != ancoraggio.OrigineConfermato {
		t.Errorf("associazione %+v: la decisione resta", a)
	}
}

// TestPO13IlFileAmbiguo (PO-13; R81, T-B0-25): con lo sciolto anche target (il suo codice è un identificativo confermato),
// un file con il nome dello sciolto ha due candidati a due livelli: ambiguo, collocazione non determinabile. Compare in «da
// smistare» con associazione_ambigua e blocca lo smistamento dei due prodotti a cui è pertinente. Quando una persona lo
// conferma sullo sciolto (il documento) è terminale: esce da «da smistare» e non blocca più, senza conflitto.
func TestPO13IlFileAmbiguo(t *testing.T) {
	th := scenaSmistamento(t)
	th.Identificativi = []fotorfq.Identificativo{confermato("7120200A")}
	fileNelMessaggio(&th, fS, idM1, "7120200A_1.pdf", "pdf", shaN(51), scansione(t, shaN(51)))
	rifSciolto := ancoraggio.RifComponente(cSciolto)
	et := esitoSmistamento(t, th)
	a := associazioneDi(t, et, fS)
	if a.Associazione != ancoraggio.AssociazioneAmbiguo || a.Collocazione != ancoraggio.CollocazioneNonDeterminabile || a.Terminale || a.Esclusa ||
		!reflect.DeepEqual(a.Pertinente, []string{rifProdB, rifSciolto}) || len(a.Proposta) != 2 {
		t.Fatalf("associazione %+v", a)
	}
	if d, ok := daSmistareDi(et, fS); !ok || d.Motivo != valutazione.MotivoSmistamentoAssociazioneAmbigua || d.Orfano || d.Proposta != "" {
		t.Errorf("da smistare %+v %v", d, ok)
	}
	for _, rif := range []string{rifProdB, rifSciolto} {
		s := prodottoDi(t, et, rif).Smistamento
		if s.Stato == valutazione.SmistamentoVerificato || !nonTerminale(s, fS) || !strings.Contains(motiviDi(s), "associazione_ambigua") {
			t.Errorf("%s: smistamento %+v", rif, s)
		}
	}

	c := cSciolto
	th.Documenti = append(th.Documenti, documento2D(uid(0xa52), c, fS, "7120200A_1.pdf", "pdf", shaN(51)))
	et = esitoSmistamento(t, th)
	if a := associazioneDi(t, et, fS); !a.Terminale || a.Conflitto {
		t.Errorf("confermato: %+v", a)
	}
	if _, ok := daSmistareDi(et, fS); ok || nonTerminale(prodottoDi(t, et, rifProdB).Smistamento, fS) || len(conflittiDi(et, valutazione.ConflittoAssociazione)) != 0 {
		t.Errorf("confermato, ancora da smistare o in conflitto: %+v", et.Conflitti)
	}
}

// TestPO14PO39LoScarto (PO-14, PO-39; R105 [U], E1R §6; T-E1R-10, T-B0-32): il 2D dello sciolto manca; un file con il
// nome dello sciolto e la proposta scartata da una persona (il gesto «scarta», scritto solo nel DB) è escluso e
// terminale: non è in «da smistare», non è orfano, non tiene aperto nessuno smistamento, e la voce del 2D che mancava
// resta manca. Mai scartati: una proposta scartata senza deciso_da, un orfano, un file senza candidati, un fuori_richiesta
// proposto, un rumore proposto o riproposto da hash_rumore, un PDF che non si apre.
func TestPO14PO39LoScarto(t *testing.T) {
	th := scenaCompletezza(t, false)
	conMessaggio(&th, mNessuno, 20, "File", "Buongiorno,\r\nin allegato i file.\r\nGrazie", true)
	op, cs := operatore, cSciolto
	fileNelMessaggio(&th, fX, idM1, "7120200A_1.pdf", "pdf", shaN(61), scansione(t, shaN(61)))
	conProposta(&th, uid(0xb61), fX, "disegno_2d", "nome_file", "scartata", &cs, &op)
	et := esitoSmistamento(t, th)
	a := associazioneDi(t, et, fX)
	if !a.Esclusa || !a.Terminale || a.Manuale != nil || !reflect.DeepEqual(a.Pertinente, []string{rifProdB}) {
		t.Fatalf("lo scartato %+v", a)
	}
	p := prodottoDi(t, et, rifProdB)
	if _, ok := daSmistareDi(et, fX); ok || nonTerminale(p.Smistamento, fX) || et.Fascicolo.Orfani != 0 {
		t.Errorf("lo scartato è ancora da smistare o tiene aperto lo smistamento: %+v", p.Smistamento)
	}
	if p.Smistamento.Stato != valutazione.SmistamentoVerificato || esitoDi(voceDi(t, p.Documenti, cSciolto, "disegno_2d")) != "manca/nessun_documento" {
		t.Errorf("smistamento %+v, voce del 2D %s: scartare non soddisfa la voce (T-B6-08: la voce che manca la dice la completezza)", p.Smistamento,
			esitoDi(voceDi(t, p.Documenti, cSciolto, "disegno_2d")))
	}

	t.Run("terminali: lo scartato senza codice e il duplicato, senza contesto", func(t *testing.T) {
		th := scenaCompletezza(t, false)
		op := operatore
		pdfSenzaCodice(t, &th, fV, idM1, "scartato-senza-codice.pdf", 68)
		conProposta(&th, uid(0xb68), fV, "altro", "nome_file", "scartata", nil, &op)
		pdfSenzaCodice(t, &th, fD, idM1, "copia.pdf", 69)
		conProposta(&th, uid(0xb69), fD, "disegno_2d", "nome_file", "duplicato", nil, &op)
		et := esitoSmistamento(t, th)
		v, d := associazioneDi(t, et, fV), associazioneDi(t, et, fD)
		if !v.Esclusa || !v.Terminale || v.Pertinente != nil || v.PertinenzaContesto != nil {
			t.Errorf("lo scartato senza codice nel messaggio che nomina il prodotto: %+v", v)
		}
		if d.Esclusa || !d.Terminale || d.PertinenzaContesto != nil {
			t.Errorf("il duplicato deciso da una persona: %+v", d)
		}
		if s := prodottoDi(t, et, rifProdB).Smistamento; s.Stato != valutazione.SmistamentoVerificato {
			t.Errorf("smistamento %+v", s)
		}
	})

	t.Run("mai scartati", func(t *testing.T) {
		th := scenaCompletezza(t, false)
		conMessaggio(&th, mNessuno, 20, "File", "Buongiorno,\r\nin allegato i file.\r\nGrazie", true)
		fileNelMessaggio(&th, fY, idM1, "7120200A_2.pdf", "pdf", shaN(62), scansione(t, shaN(62)))
		conProposta(&th, uid(0xb62), fY, "disegno_2d", "nome_file", "scartata", &cs, nil) // senza deciso_da: non è uno scarto
		pdfSenzaCodice(t, &th, fZ, mNessuno, "allegato-tre.pdf", 63)                      // un orfano
		pdfSenzaCodice(t, &th, fW, idM1, "allegato-uno.pdf", 64)                          // senza candidati
		fileNelMessaggio(&th, fF, mNessuno, "7120555A_1.pdf", "pdf", shaN(65), scansione(t, shaN(65)))
		conProposta(&th, uid(0xb65), fF, "disegno_2d", "nome_file", "aperta", nil, nil) // un fuori_richiesta proposto
		pdfSenzaCodice(t, &th, fG, mNessuno, "firma.pdf", 66)
		conProposta(&th, uid(0xb66), fG, "rumore", "rumore", "aperta", nil, nil) // riproposto come rumore da hash_rumore
		f := fattiPDFErrore(t, shaN(67))
		fileNelMessaggio(&th, fU, idM1, "illeggibile.pdf", "pdf", shaN(67), &f)
		et := esitoSmistamento(t, th)
		for _, id := range []uuid.UUID{fY, fZ, fW, fF, fG, fU} {
			if a := associazioneDi(t, et, id); a.Esclusa || a.Terminale || a.Perimetro != valutazione.PerimetroDentro {
				t.Errorf("file %s: %+v", id, a)
			}
		}
		if a := associazioneDi(t, et, fF); a.Collocazione != ancoraggio.CollocazioneFuoriRichiesta {
			t.Errorf("il fuori_richiesta %+v", a)
		}
		if !nonTerminale(prodottoDi(t, et, rifProdB).Smistamento, fY) {
			t.Error("la proposta scartata senza deciso_da non tiene aperto lo smistamento")
		}
	})
}

// TestPO21LaRinomina (PO-21, parte B6, caso b; T-E1-05, R95 A, R62 g A): il 2D confermato sullo sciolto ha il nome dello
// sciolto: nessun conflitto. Dopo la rinomina del componente confermato (stesso componente_id, un altro codice) il nodo e
// il componente restano gli stessi, la compatibilità si ricalcola, e il file non è più compatibile con il codice
// confermato: conflitto codice_confermato, la decisione conservata, il documento presente nella completezza.
func TestPO21LaRinomina(t *testing.T) {
	th := scenaSmistamento(t)
	for i := range th.Allegati {
		if th.Allegati[i].ID == aPDF2 {
			th.Allegati[i].NomeFile = "7120200A_1.pdf"
		}
	}
	et := esitoSmistamento(t, th)
	if s := prodottoDi(t, et, rifProdB).Smistamento; s.Stato != valutazione.SmistamentoVerificato || associazioneDi(t, et, aPDF2).Conflitto {
		t.Fatalf("prima della rinomina: %+v", s)
	}
	for i := range th.Componenti {
		if th.Componenti[i].ID == cSciolto {
			th.Componenti[i].Codice = "7120250A"
		}
	}
	et = esitoSmistamento(t, th)
	p := prodottoDi(t, et, rifProdB)
	c := conflittiDi(et, valutazione.ConflittoAssociazione)
	if p.Smistamento.Stato != valutazione.SmistamentoConflitto || len(c) != 1 || c[0].Motivo != valutazione.MotivoConflittoCodiceConfermato ||
		c[0].Decisione != ancoraggio.RifComponente(cSciolto) {
		t.Fatalf("dopo la rinomina: %+v, conflitti %+v", p.Smistamento, c)
	}
	if a := associazioneDi(t, et, aPDF2); *a.Confermata != cSciolto || !a.Conflitto || esitoDi(voceDi(t, p.Documenti, cSciolto, "disegno_2d")) != "presente/" {
		t.Errorf("la decisione e il documento restano: %+v", a)
	}
}

// TestPO25IlNuovoCAD (PO-25, parte dello smistamento sulla fotografia; T-B0-29, T-B0-07, T-E1-14, T-E1-08): arriva uno
// STEP nuovo con la radice del prodotto, che ha già la fonte confermata e il suo cad_3d. Lo STEP è candidato, la fonte
// resta confermata, il file è pertinente solo al prodotto, c'è il conflitto nuovo_file (il cad_3d deciso, il file nuovo)
// e lo smistamento va in conflitto; il secondo prodotto non cambia. Un secondo 2D dello sciolto non è un nuovo_file: è un
// file pertinente, che aspetta la conferma.
func TestPO25IlNuovoCAD(t *testing.T) {
	base := scenaSmistamento(t)
	secondoProdotto(t, &base)
	prima := prodottoDi(t, esitoSmistamento(t, base), rifP2).Smistamento
	th := base
	th.Allegati = append([]fotorfq.Allegato(nil), base.Allegati...)
	th.Fatti = map[string]fotorfq.Fatti{}
	for k, v := range base.Fatti {
		th.Fatti[k] = v
	}
	f := fattiStepF(t, shaN(71), []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 2}})
	fileNelMessaggio(&th, fX, idM1, "7120100A_2.stp", "stp", shaN(71), &f)
	conProposta(&th, uid(0xb71), fX, "cad_3d", "step", "aperta", nil, nil)
	et := esitoSmistamento(t, th)
	p := prodottoDi(t, et, rifProdB)
	if p.Fonte.Stato != valutazione.FonteConfermata || p.Smistamento.Stato != valutazione.SmistamentoConflitto ||
		!strings.Contains(motiviDi(p.Smistamento), "nuovo_file_su_componente_deciso") {
		t.Fatalf("fonte %s, smistamento %+v", p.Fonte.Stato, p.Smistamento)
	}
	if a := associazioneDi(t, et, fX); !reflect.DeepEqual(a.Pertinente, []string{rifProdB}) || !a.Conflitto || a.Terminale {
		t.Errorf("lo STEP nuovo %+v", a)
	}
	c := conflittiDi(et, valutazione.ConflittoNuovoFile)
	if len(c) != 1 || c[0].Asse != valutazione.AsseSmistamento || c[0].Rif != ancoraggio.RifComponente(cProdotto) || c[0].Prodotto != rifProdB ||
		c[0].Decisione != valutazione.RifDocumento(dStepB) || c[0].Proposta != "allegato:"+fX.String() || c[0].Motivo != "cad_3d" ||
		c[0].EvidenzaDecisione.Da == nil || *c[0].EvidenzaProposta.AllegatoID != fX {
		t.Fatalf("conflitti nuovo_file %+v", c)
	}
	if dopo := prodottoDi(t, et, rifP2).Smistamento; canonicoDi(t, dopo) != canonicoDi(t, prima) {
		t.Errorf("il secondo prodotto cambia: %+v → %+v", prima, dopo)
	}

	t.Run("un secondo 2D non è un nuovo_file", func(t *testing.T) {
		th := scenaSmistamento(t)
		fileNelMessaggio(&th, fY, idM1, "7120200A_1.pdf", "pdf", shaN(72), scansione(t, shaN(72)))
		conProposta(&th, uid(0xb72), fY, "disegno_2d", "nome_file", "aperta", nil, nil)
		et := esitoSmistamento(t, th)
		s := prodottoDi(t, et, rifProdB).Smistamento
		if len(conflittiDi(et, valutazione.ConflittoNuovoFile)) != 0 || s.Stato != valutazione.SmistamentoDaVerificare || !nonTerminale(s, fY) ||
			motiviDi(s) != "file_da_smistare associazione_non_confermata" {
			t.Errorf("smistamento %+v, conflitti %+v", s, et.Conflitti)
		}
	})
}

// TestPO27GliOrfani (PO-27; R93 [U][R], T-E1-11, T-E1-16; E1R): con due prodotti, un file senza nessuna evidenza in un
// messaggio che non nomina prodotti è orfano: in «da smistare» con i prodotti vuoti, un avviso nel fascicolo, nessun
// prodotto bloccato, il congelamento invariato. Accanto: un file con un candidato solo nel primo prodotto blocca solo il
// primo; un file con un candidato in tutti e due (il figlio condiviso) blocca tutti e due; un fuori_richiesta proposto
// senza altre evidenze è orfano, e con un «assegna» sul secondo prodotto è pertinente al secondo. Lo stesso orfano
// scartato con il gesto esce da «da smistare» e dagli avvisi.
func TestPO27GliOrfani(t *testing.T) {
	scena := func(scartaZ bool) fotorfq.Thread {
		th := scenaSmistamento(t)
		secondoProdotto(t, &th, "7120200A")
		conMessaggio(&th, mNessuno, 20, "File", "Buongiorno,\r\nin allegato i file.\r\nGrazie", true)
		pdfSenzaCodice(t, &th, fZ, mNessuno, "allegato-tre.pdf", 81)
		fileNelMessaggio(&th, fU, mNessuno, "7120100A_9.pdf", "pdf", shaN(82), scansione(t, shaN(82)))
		fileNelMessaggio(&th, fS, mNessuno, "7120200A_1.pdf", "pdf", shaN(83), scansione(t, shaN(83)))
		fileNelMessaggio(&th, fF, mNessuno, "7120555A_1.pdf", "pdf", shaN(84), scansione(t, shaN(84)))
		fileNelMessaggio(&th, fG, mNessuno, "7120555A_2.pdf", "pdf", shaN(85), scansione(t, shaN(85)))
		p2 := cP2
		conProposta(&th, uid(0xb85), fG, "disegno_2d", "nome_file", "aperta", &p2, nil)
		if scartaZ {
			op := operatore
			conProposta(&th, uid(0xb81), fZ, "altro", "nome_file", "scartata", nil, &op)
		}
		return th
	}
	et := esitoSmistamento(t, scena(false))
	p1, p2 := prodottoDi(t, et, rifProdB).Smistamento, prodottoDi(t, et, rifP2).Smistamento

	d, ok := daSmistareDi(et, fZ)
	if !ok || !d.Orfano || len(d.Prodotti) != 0 || d.Motivo != valutazione.MotivoSmistamentoNessunCandidato || nonTerminale(p1, fZ) || nonTerminale(p2, fZ) {
		t.Errorf("l'orfano %+v %v", d, ok)
	}
	if !contieneRif(et.Fascicolo.Avvisi, "orfano:"+fZ.String()) || et.Fascicolo.Congelabile {
		t.Errorf("avvisi %v, congelabile %v", et.Fascicolo.Avvisi, et.Fascicolo.Congelabile)
	}
	if !nonTerminale(p1, fU) || nonTerminale(p2, fU) {
		t.Errorf("il candidato solo nel primo prodotto: %+v / %+v", p1, p2)
	}
	if a := associazioneDi(t, et, fS); a.Associazione != ancoraggio.AssociazioneCandidatoUnico || !reflect.DeepEqual(a.Pertinente, []string{rifProdB, rifP2}) ||
		!nonTerminale(p1, fS) || !nonTerminale(p2, fS) {
		t.Errorf("il figlio condiviso %+v", a)
	}
	if d, ok := daSmistareDi(et, fF); !ok || !d.Orfano || d.Motivo != valutazione.MotivoSmistamentoFuoriRichiestaNonConfermato {
		t.Errorf("il fuori_richiesta senza altre evidenze %+v %v", d, ok)
	}
	if a := associazioneDi(t, et, fG); a.Collocazione != ancoraggio.CollocazioneFuoriRichiesta || !reflect.DeepEqual(a.Pertinente, []string{rifP2}) ||
		*a.Manuale != cP2 || a.Origine != ancoraggio.OrigineManuale || !nonTerminale(p2, fG) || nonTerminale(p1, fG) {
		t.Errorf("il fuori_richiesta con l'«assegna» %+v", a)
	}
	if d, ok := daSmistareDi(et, fG); !ok || d.Orfano || !reflect.DeepEqual(d.Prodotti, []string{rifP2}) {
		t.Errorf("il fuori_richiesta con l'«assegna» da smistare: %+v %v", d, ok)
	}

	t.Run("il figlio condiviso confermato sullo sciolto: il candidato ci sta per il nodo deciso", func(t *testing.T) {
		th := scena(false)
		c := cSciolto
		th.Documenti = append(th.Documenti, documento2D(uid(0xb83), c, fS, "7120200A_1.pdf", "pdf", shaN(83)))
		et := esitoSmistamento(t, th)
		a := associazioneDi(t, et, fS)
		if !a.Terminale || a.Conflitto || len(a.Proposta) != 1 || !strings.HasPrefix(a.Proposta[0], "componente/"+ancoraggio.PrefissoRifIdentita) ||
			len(conflittiDi(et, valutazione.ConflittoAssociazione)) != 0 {
			t.Errorf("il figlio condiviso confermato: %+v, conflitti %+v", a, et.Conflitti)
		}
	})

	scartato := esitoSmistamento(t, scena(true))
	if _, ok := daSmistareDi(scartato, fZ); ok || contieneRif(scartato.Fascicolo.Avvisi, "orfano:"+fZ.String()) ||
		scartato.Fascicolo.Orfani != et.Fascicolo.Orfani-1 || !associazioneDi(t, scartato, fZ).Esclusa {
		t.Errorf("l'orfano scartato: avvisi %v, orfani %d", scartato.Fascicolo.Avvisi, scartato.Fascicolo.Orfani)
	}
	if scartato.Fascicolo.Congelabile != et.Fascicolo.Congelabile {
		t.Error("gli orfani cambiano il congelamento")
	}
}

// TestPO32IlPerimetro (PO-32; R93 b A [R], T-E1-12): fuori controllo l'allegato inline, il .msg come contenitore, l'archivio
// estratto, il collegamento, i file di un messaggio in uscita e quelli di un fornitore o di un altro cliente; contano le
// voci di un .msg e di un archivio, l'inoltro interno e un file proposto come rumore. Uno STEP con la radice del prodotto
// in un messaggio in uscita verso un fornitore non è pertinente e non ferma il prodotto (nessun nuovo_file). Un file
// fuori perimetro non entra in «da smistare» né fra gli orfani, e non toglie nessun fabbisogno.
func TestPO32IlPerimetro(t *testing.T) {
	scena := func(t *testing.T, con2D bool) fotorfq.Thread {
		th := scenaCompletezza(t, con2D)
		m := conMessaggio(&th, mUscita, 30, "Richiesta al fornitore", "Buongiorno,\r\nvi giriamo il file 7120100A.\r\nSaluti", false)
		m.Direzione, m.ControparteTipo, m.ControparteClienteID = "uscita", "fornitore", nil
		m = conMessaggio(&th, mFornit, 31, "Offerta", "Buongiorno,\r\nla nostra offerta.\r\nSaluti", false)
		m.ControparteTipo, m.ControparteClienteID = "fornitore", nil
		m = conMessaggio(&th, mAltroCl, 32, "Altro", "Buongiorno,\r\nun file.\r\nSaluti", false)
		ac := altroCliente
		m.ControparteClienteID = &ac
		m = conMessaggio(&th, mInterno, 33, "I: file del cliente", "Ciao,\r\nti giro i file del cliente.\r\nCiao", false)
		m.Direzione, m.Interno, m.ControparteTipo, m.ControparteClienteID = "uscita", true, "interno", nil
		return th
	}
	conNatura := func(th *fotorfq.Thread, id, msg uuid.UUID, nome, est, natura string, contenitore *uuid.UUID, n int) {
		a := allegato(id, int16(40+len(th.Allegati)), nome, est, shaN(n))
		a.MessaggioID, a.Natura, a.ContenitoreID = msg, natura, contenitore
		f := scansione(t, shaN(n))
		conAllegato(th, a, f)
	}
	th := scena(t, true)
	inline, outlook, vMsg, zip, vZip, link := uid(0xc01), uid(0xc02), uid(0xc03), uid(0xc04), uid(0xc05), uid(0xc06)
	uscita, fornitore, altro, interno, rumore := uid(0xc07), uid(0xc08), uid(0xc09), uid(0xc0a), uid(0xc0b)
	conNatura(&th, inline, idM1, "logo.png", "png", "inline", nil, 101)
	conNatura(&th, outlook, idM1, "inoltro.msg", "msg", "elemento_outlook", nil, 102)
	conNatura(&th, vMsg, idM1, "voce-msg.pdf", "pdf", "file", &outlook, 103)
	conNatura(&th, zip, idM1, "pacco.zip", "zip", "file", nil, 104)
	conNatura(&th, vZip, idM1, "voce-zip.pdf", "pdf", "file", &zip, 105)
	conNatura(&th, link, idM1, "collegamento", "", "collegamento", nil, 106)
	f := fattiStepF(t, shaN(107), []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}}, []arcoF{{"#1", "#2", 2}})
	fileNelMessaggio(&th, uscita, mUscita, "7120100A_3.stp", "stp", shaN(107), &f)
	conProposta(&th, uid(0xc17), uscita, "cad_3d", "step", "aperta", nil, nil)
	pdfSenzaCodice(t, &th, fornitore, mFornit, "offerta.pdf", 108)
	pdfSenzaCodice(t, &th, altro, mAltroCl, "altro.pdf", 109)
	pdfSenzaCodice(t, &th, interno, mInterno, "inoltrato.pdf", 110)
	pdfSenzaCodice(t, &th, rumore, idM1, "firma.pdf", 111)
	conProposta(&th, uid(0xc1b), rumore, "rumore", "rumore", "aperta", nil, nil)
	et := esitoSmistamento(t, th)

	fuori := map[uuid.UUID]string{inline: valutazione.PerimetroInline, outlook: valutazione.PerimetroElementoOutlook,
		zip: valutazione.PerimetroContenitoreEstratto, link: valutazione.PerimetroCollegamento, uscita: valutazione.PerimetroMessaggioInUscita,
		fornitore: valutazione.PerimetroAltraControparte, altro: valutazione.PerimetroAltraControparte}
	p := prodottoDi(t, et, rifProdB)
	for id, per := range fuori {
		a := associazioneDi(t, et, id)
		_, daSmistare := daSmistareDi(et, id)
		if a.Perimetro != per || len(a.Pertinente) != 0 || daSmistare || nonTerminale(p.Smistamento, id) {
			t.Errorf("fuori perimetro %s: %+v (da smistare %v)", per, a, daSmistare)
		}
	}
	for _, id := range []uuid.UUID{vMsg, vZip, interno, rumore} {
		a := associazioneDi(t, et, id)
		_, daSmistare := daSmistareDi(et, id)
		if a.Perimetro != valutazione.PerimetroDentro || !daSmistare {
			t.Errorf("conta: %+v (da smistare %v)", a, daSmistare)
		}
	}
	if len(conflittiDi(et, valutazione.ConflittoNuovoFile)) != 0 || p.Fonte.Stato != valutazione.FonteConfermata {
		t.Errorf("lo STEP in uscita: conflitti %+v, fonte %s", et.Conflitti, p.Fonte.Stato)
	}
	if a := associazioneDi(t, et, inline); a.Associazione != ancoraggio.AssociazioneNonValutata || a.Collocazione != "" {
		t.Errorf("l'inline non è valutato: %+v", a)
	}

	t.Run("lo STEP in uscita non ferma il prodotto", func(t *testing.T) {
		th := scena(t, true)
		fileNelMessaggio(&th, uscita, mUscita, "7120100A_3.stp", "stp", shaN(107), &f)
		conProposta(&th, uid(0xc17), uscita, "cad_3d", "step", "aperta", nil, nil)
		if s := prodottoDi(t, esitoSmistamento(t, th), rifProdB).Smistamento; s.Stato != valutazione.SmistamentoVerificato {
			t.Errorf("smistamento %+v", s)
		}
	})
	t.Run("un file fuori perimetro non toglie nessun fabbisogno", func(t *testing.T) {
		th := scena(t, false)
		fileNelMessaggio(&th, uscita, mUscita, "7120200A_1.pdf", "pdf", shaN(112), scansione(t, shaN(112)))
		conProposta(&th, uid(0xc17), uscita, "disegno_2d", "nome_file", "aperta", nil, nil)
		p := prodottoDi(t, esitoSmistamento(t, th), rifProdB)
		if v := voceDi(t, p.Documenti, cSciolto, "disegno_2d"); v.Esito == valutazione.EsitoPresente || p.Documenti.Stato == valutazione.DocumentiCompleta {
			t.Errorf("voce %s, documenti %s", esitoDi(v), p.Documenti.Stato)
		}
	})
}

// scenaContesto: i due prodotti, con il 2D che manca, e i messaggi del contesto: il primo nomina solo il prodotto, mDue
// nomina tutti e due, mNessuno nessuno; X (senza codice) nel primo, Y in mDue, Z in mNessuno, W (con il codice del
// secondo prodotto) nel primo.
func scenaContesto(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaCompletezza(t, false)
	secondoProdotto(t, &th)
	conMessaggio(&th, mDue, 10, "Altri file", "Buongiorno,\r\nper 7120100A e 7120900A vi mandiamo i file.\r\nGrazie", true)
	conMessaggio(&th, mNessuno, 20, "File", "Buongiorno,\r\nin allegato i file.\r\nGrazie", true)
	pdfSenzaCodice(t, &th, fX, idM1, "allegato-uno.pdf", 91)
	pdfSenzaCodice(t, &th, fY, mDue, "allegato-due.pdf", 92)
	pdfSenzaCodice(t, &th, fZ, mNessuno, "allegato-tre.pdf", 93)
	fileNelMessaggio(&th, fW, idM1, "7120900A_1.pdf", "pdf", shaN(94), scansione(t, shaN(94)))
	return th
}

// TestPO40IlContesto (PO-40; R105 [U], la regola applicata; T-E1R-11; R106 B e R107, precisate il 07/10): un file senza
// evidenze d'identità, allegato a un messaggio che nomina solo il primo prodotto, è pertinente al primo
// (PertinenzaContesto) e ne tiene aperto lo smistamento; il secondo resta libero. Lo stesso file con un messaggio che
// nomina tutti e due è orfano, con i prodotti nominati anche nell'avviso del fascicolo, e non blocca nessuno. Con un
// messaggio senza prodotti è orfano, senza contesto. Un file con un candidato nel secondo prodotto, allegato al
// messaggio che nomina solo il primo, è pertinente solo al secondo. Il contesto non è mai un candidato e non soddisfa
// voci.
func TestPO40IlContesto(t *testing.T) {
	et := esitoSmistamento(t, scenaContesto(t))
	p1, p2 := prodottoDi(t, et, rifProdB), prodottoDi(t, et, rifP2)

	x := associazioneDi(t, et, fX)
	if !reflect.DeepEqual(x.Pertinente, []string{rifProdB}) || !reflect.DeepEqual(x.PertinenzaContesto, []string{rifProdB}) ||
		x.Associazione != ancoraggio.AssociazioneNessunCandidato || x.Proposta != nil || x.Origine != "" {
		t.Errorf("X: %+v", x)
	}
	if !nonTerminale(p1.Smistamento, fX) || nonTerminale(p2.Smistamento, fX) || p1.Smistamento.Stato == valutazione.SmistamentoVerificato {
		t.Errorf("X tiene aperto il primo, non il secondo: %+v / %+v", p1.Smistamento, p2.Smistamento)
	}
	if d, ok := daSmistareDi(et, fX); !ok || d.Orfano || !reflect.DeepEqual(d.Prodotti, []string{rifProdB}) || d.Motivo != valutazione.MotivoSmistamentoNessunCandidato {
		t.Errorf("X da smistare: %+v %v", d, ok)
	}
	if esitoDi(voceDi(t, p1.Documenti, cProdotto, "disegno_2d")) != "manca/nessun_documento" {
		t.Error("il contesto soddisfa una voce")
	}

	y, ok := daSmistareDi(et, fY)
	contesto := []string{rifProdB, rifP2}
	if !ok || !y.Orfano || len(y.Prodotti) != 0 || !reflect.DeepEqual(y.ProdottiContesto, contesto) || nonTerminale(p1.Smistamento, fY) || nonTerminale(p2.Smistamento, fY) {
		t.Errorf("Y: %+v %v", y, ok)
	}
	if !contieneRif(et.Fascicolo.Avvisi, "orfano:"+fY.String()+":prodotti_contesto:"+rifProdB+"|"+rifP2) {
		t.Errorf("l'avviso di Y fra %v", et.Fascicolo.Avvisi)
	}
	if z, ok := daSmistareDi(et, fZ); !ok || !z.Orfano || z.ProdottiContesto != nil || !contieneRif(et.Fascicolo.Avvisi, "orfano:"+fZ.String()) {
		t.Errorf("Z: %+v %v", z, ok)
	}
	if w := associazioneDi(t, et, fW); !reflect.DeepEqual(w.Pertinente, []string{rifP2}) || w.PertinenzaContesto != nil {
		t.Errorf("W: %+v", w)
	}

	t.Run("un codice con un altro marcatore non nomina il prodotto", func(t *testing.T) {
		th := scenaContesto(t)
		conMessaggio(&th, mSoloP2, 40, "Varianti", "Buongiorno,\r\nper la variante 7120100B vi mandiamo il file.\r\nGrazie", true)
		pdfSenzaCodice(t, &th, fG, mSoloP2, "allegato-variante.pdf", 95)
		et := esitoSmistamento(t, th)
		if d, ok := daSmistareDi(et, fG); !ok || !d.Orfano || d.ProdottiContesto != nil {
			t.Errorf("il file del messaggio con 7120100B: %+v %v (T-B4-30: un altro marcatore è un'altra identità)", d, ok)
		}
	})
}

// TestR106SullaScena (R106 B, precisata dall'utente il 07/10; la quinta prova della revisione d'impatto, che fissa il
// comportamento di oggi): un file con un'identità letta ma senza nessuna evidenza di pertinenza (un fuori_richiesta
// proposto: un codice letto senza candidati), allegato al messaggio che nomina solo il prodotto, è pertinente al prodotto
// per il contesto e ne tiene aperto lo smistamento. Un file con un'evidenza (un candidato) non riceve il contesto nella
// pertinenza; il contesto, con la provenienza, l'hanno tutti e due, e nessuno dei due è in contraddizione.
func TestR106SullaScena(t *testing.T) {
	th := scenaSmistamento(t)
	fileNelMessaggio(&th, fF, idM1, "7120555A_1.pdf", "pdf", shaN(121), scansione(t, shaN(121)))
	fileNelMessaggio(&th, fU, idM1, "7120200A_1.pdf", "pdf", shaN(122), scansione(t, shaN(122)))
	et := esitoSmistamento(t, th)
	a := associazioneDi(t, et, fF)
	if a.Collocazione != ancoraggio.CollocazioneFuoriRichiesta || !reflect.DeepEqual(a.PertinenzaContesto, []string{rifProdB}) {
		t.Fatalf("il fuori_richiesta: %+v", a)
	}
	s := prodottoDi(t, et, rifProdB).Smistamento
	if !nonTerminale(s, fF) || !strings.Contains(motiviDi(s), "fuori_richiesta_non_confermato") {
		t.Errorf("smistamento %+v", s)
	}
	if d, ok := daSmistareDi(et, fF); !ok || d.Orfano || d.Motivo != valutazione.MotivoSmistamentoFuoriRichiestaNonConfermato {
		t.Errorf("da smistare %+v %v", d, ok)
	}
	if u := associazioneDi(t, et, fU); u.PertinenzaContesto != nil || u.Associazione != ancoraggio.AssociazioneCandidatoUnico {
		t.Errorf("il file con un candidato riceve il contesto: %+v", u)
	}
	for _, id := range []uuid.UUID{fF, fU} {
		if a := associazioneDi(t, et, id); a.Contesto == nil || !reflect.DeepEqual(a.Contesto.Prodotti, []string{rifProdB}) || len(discordiDi(et, id)) != 0 {
			t.Errorf("file %s: contesto %+v, contraddizione %+v", id, a.Contesto, discordiDi(et, id))
		}
	}
}

// TestR107SullaScena (R107, precisata dall'utente il 07/10; in A1c la formula dell'opzione A, lettura [T] registrata in
// domande-a1c.md, alla lettera: R-71 della revisione di V2; R48 A): contano le letture del motore A con la funzione
// «richiesta» sul messaggio a cui il file è allegato, cioè il segmento corrente di ogni messaggio, anche senza il gesto
// 1, più i segmenti che un caso dichiara pertinenti. La storia di un inoltro senza confine non conta senza un caso, e
// conta quando il caso la dichiara pertinente; il caso aggiunge, non toglie il corrente degli altri messaggi; le righe
// legacy candidato_codice non contano mai; per la voce di un archivio vale il messaggio che porta l'archivio. La
// grammatica legge i codici anche nella storia (insiemeConStoria): la storia si legge, e il contesto la usa solo quando
// un caso la dichiara pertinente.
func TestR107SullaScena(t *testing.T) {
	r := insiemeConStoria(t)
	scena := func() fotorfq.Thread {
		th := scenaSmistamento(t)
		secondoProdotto(t, &th)
		conMessaggio(&th, mSenza, 10, "Altri file", "Buongiorno,\r\nper 7120900A i file.\r\nGrazie", false)
		conMessaggio(&th, mInoltro, 20, "I: Richiesta ACME26-031", "Buongiorno,\r\nvi giro la richiesta per 7120900A.\r\nGrazie", true)
		conMessaggio(&th, mNessuno, 30, "File", "Buongiorno,\r\nin allegato i file.\r\nGrazie", true)
		pdfSenzaCodice(t, &th, fX, idM1, "allegato-uno.pdf", 131)
		pdfSenzaCodice(t, &th, fY, mSenza, "allegato-senza.pdf", 132)
		pdfSenzaCodice(t, &th, fZ, mInoltro, "allegato-inoltro.pdf", 133)
		pdfSenzaCodice(t, &th, fW, mNessuno, "allegato-legacy.pdf", 134)
		th.CandidatiCodice = []fotorfq.CandidatoCodice{{MessaggioID: mNessuno, Codice: "7120900A", Ruolo: "prodotto", Origine: "regex", Punteggio: 90}}
		zip := uid(0xc21)
		z := allegato(zip, 60, "pacco.zip", "zip", shaN(135))
		conAllegato(&th, z, nil)
		v := allegato(fU, 61, "voce.pdf", "pdf", shaN(136))
		v.MessaggioID, v.ContenitoreID = mNessuno, &zip
		conAllegato(&th, v, scansione(t, shaN(136)))
		return th
	}
	et := esitoSmistamentoCon(t, scena(), r)
	for _, id := range []uuid.UUID{fZ, fW} {
		if d, ok := daSmistareDi(et, id); !ok || !d.Orfano || d.ProdottiContesto != nil {
			t.Errorf("file %s: %+v %v", id, d, ok)
		}
	}
	if x := associazioneDi(t, et, fX); !reflect.DeepEqual(x.PertinenzaContesto, []string{rifProdB}) {
		t.Errorf("il corrente del primo messaggio: %+v", x)
	}
	if y := associazioneDi(t, et, fY); !reflect.DeepEqual(y.PertinenzaContesto, []string{rifP2}) || !nonTerminale(prodottoDi(t, et, rifP2).Smistamento, fY) {
		t.Errorf("il corrente del messaggio senza il gesto 1 (R-71): %+v", y)
	}
	if u := associazioneDi(t, et, fU); !reflect.DeepEqual(u.PertinenzaContesto, []string{rifProdB}) || u.Perimetro != valutazione.PerimetroDentro {
		t.Errorf("la voce dell'archivio: %+v", u)
	}

	tid := threadACME
	caso := valutazione.IngressoCaso{ID: "ACME-STORIA", ThreadID: &tid, ClienteID: clienteACME,
		Segmenti: []valutazione.SegmentoDichiarato{{MessaggioID: mInoltro, SegmentoID: "s:storia:1", Uso: "pertinente", Origine: "scenario"}}}
	et = esitoSmistamentoCon(t, scena(), r, caso)
	if z := associazioneDi(t, et, fZ); !reflect.DeepEqual(z.PertinenzaContesto, []string{rifP2}) {
		t.Errorf("la storia dichiarata dal caso: %+v", z)
	}
	if x := associazioneDi(t, et, fX); !reflect.DeepEqual(x.PertinenzaContesto, []string{rifProdB}) {
		t.Errorf("con il caso il corrente del primo messaggio conta ancora (il caso aggiunge: R-71): %+v", x)
	}
	if y := associazioneDi(t, et, fY); !reflect.DeepEqual(y.PertinenzaContesto, []string{rifP2}) {
		t.Errorf("con il caso il corrente del messaggio senza il gesto 1 conta ancora: %+v", y)
	}
}

// TestR108SullaScena (R108 A, confermata dall'utente il 07/10 con il requisito operativo per lo spazio di verifica): la
// pre-associazione (un candidato_unico non confermato) e lo STEP del secondo prodotto non entrano in «da smistare», ma
// tengono aperto lo smistamento del loro prodotto, con il motivo associazione_non_confermata; un file pertinente con
// uno dei cinque motivi entra, con i suoi prodotti.
func TestR108SullaScena(t *testing.T) {
	th := scenaSmistamento(t)
	secondoProdotto(t, &th)
	fileNelMessaggio(&th, fU, idM1, "7120200A_1.pdf", "pdf", shaN(141), scansione(t, shaN(141)))
	pdfSenzaCodice(t, &th, fX, idM1, "allegato-uno.pdf", 142)
	et := esitoSmistamento(t, th)
	for _, c := range []struct {
		id  uuid.UUID
		rif string
	}{{fU, rifProdB}, {aStepP2, rifP2}} {
		s := prodottoDi(t, et, c.rif).Smistamento
		if _, ok := daSmistareDi(et, c.id); ok || !nonTerminale(s, c.id) || !strings.Contains(motiviDi(s), "associazione_non_confermata") {
			t.Errorf("%s: %+v", c.id, s)
		}
	}
	if d, ok := daSmistareDi(et, fX); !ok || d.Orfano || !reflect.DeepEqual(d.Prodotti, []string{rifProdB}) {
		t.Errorf("il file pertinente per il contesto: %+v %v", d, ok)
	}
}

// TestLaDestinazioneF8 (contratto §1.5, «Associazione proposta» e «Corretta a mano o proposta e accettata»): la
// destinazione F8 di una proposta aperta è un'evidenza di pertinenza, per ognuna delle sue chiavi (un componente del
// perimetro, il nodo di una riga, il codice di un target; «generale» no); DestinazioneCoincide confronta la prima chiave
// con il documento confermato: coincide, non coincide, non determinabile (senza destinazione).
func TestLaDestinazioneF8(t *testing.T) {
	th := scenaSmistamento(t)
	conMessaggio(&th, mNessuno, 20, "File", "Buongiorno,\r\nin allegato i file.\r\nGrazie", true)
	casi := []struct {
		id       uuid.UUID
		chiave   string
		rif      []string
		proposta uuid.UUID
	}{{fX, "componente:" + cSciolto.String(), []string{rifProdB}, uid(0xd01)}, {fY, "nodo:" + rigaB2.String(), []string{rifProdB}, uid(0xd02)},
		{fZ, "identificativo:7120100A", []string{rifProdB}, uid(0xd03)}, {fW, "generale", nil, uid(0xd04)}}
	for i, c := range casi {
		pdfSenzaCodice(t, &th, c.id, mNessuno, "destinazione.pdf", 150+i)
		conProposta(&th, c.proposta, c.id, "disegno_2d", "regola_cliente", "aperta", nil, nil).Dettagli = destinazione(t, c.chiave)
	}
	op := operatore
	conProposta(&th, uid(0xd05), aPDF2, "disegno_2d", "nome_file", "confermata", nil, &op).Dettagli = destinazione(t, "componente:"+cSciolto.String(), "generale")
	conProposta(&th, uid(0xd06), aPDF1, "disegno_2d", "nome_file", "confermata", nil, &op).Dettagli = destinazione(t, "componente:"+cSciolto.String())
	et := esitoSmistamento(t, th)
	for _, c := range casi {
		a := associazioneDi(t, et, c.id)
		if !reflect.DeepEqual(a.Pertinente, c.rif) || !reflect.DeepEqual(a.DestinazioneF8, []string{c.chiave}) || a.DestinazioneCoincide != nil {
			t.Errorf("%s: %+v", c.chiave, a)
		}
	}
	if d, ok := daSmistareDi(et, fW); !ok || !d.Orfano {
		t.Errorf("la destinazione generale non lega a un prodotto: %+v %v", d, ok)
	}
	if a := associazioneDi(t, et, aPDF2); a.DestinazioneCoincide == nil || !*a.DestinazioneCoincide {
		t.Errorf("proposta e accettata: %+v", a)
	}
	if a := associazioneDi(t, et, aPDF1); a.DestinazioneCoincide == nil || *a.DestinazioneCoincide {
		t.Errorf("corretta a mano: %+v", a)
	}
	if a := associazioneDi(t, et, aStepB); a.DestinazioneCoincide != nil {
		t.Errorf("senza destinazione: %+v", a)
	}
}

// TestSmistamentoNonCalcolatoSullaScena (T-12, T-B6-09, T-B6-07, F0-17; dubbio T-B6-61): il prodotto altrimenti verificato
// ha lo smistamento da_verificare, non calcolato, con una sezione della fotografia assente, senza l'insieme delle regole
// (il thread non è valutato, ma le associazioni ci sono, come informazione) e per un target senza componente.
func TestSmistamentoNonCalcolatoSullaScena(t *testing.T) {
	th := scenaSmistamento(t)
	vistaCome(&th)
	t.Run("una sezione assente (T-12)", func(t *testing.T) {
		for _, k := range []string{fotorfq.SezioneProposteDocumento, fotorfq.SezioneProvenienze} {
			f := fotografia(th)
			f.Sezioni[k] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneAssente}
			p := prodottoDi(t, esitoThread(t, calcola(t, f, insiemeACME(t), valutazione.Ingressi{}), threadACME), rifProdB)
			if s := p.Smistamento; s.Stato != valutazione.SmistamentoDaVerificare || s.Calcolata {
				t.Errorf("senza %s: smistamento %+v (documenti %s)", k, s, p.Documenti.Stato)
			}
		}
	})
	t.Run("senza regole: non valutato, mai verificato", func(t *testing.T) {
		et := esitoThread(t, calcola(t, fotografia(th), nil, valutazione.Ingressi{}), threadACME)
		s := prodottoDi(t, et, rifProdB).Smistamento
		if et.Valutato || s.Stato != valutazione.SmistamentoDaVerificare || s.Calcolata || len(et.Associazioni) != len(th.Allegati) {
			t.Errorf("valutato %v, smistamento %+v, %d associazioni", et.Valutato, s, len(et.Associazioni))
		}
	})
	t.Run("il target senza componente (T-B6-07, F0-17)", func(t *testing.T) {
		th := scenaSmistamento(t)
		th.Identificativi = []fotorfq.Identificativo{confermato("7120888A")}
		s := prodottoDi(t, esitoSmistamento(t, th), "identificativo:7120888A").Smistamento
		if s.Stato != valutazione.SmistamentoDaVerificare || s.Calcolata || len(s.Motivi) != 0 {
			t.Errorf("smistamento %+v", s)
		}
	})
}

// TestSmistamentoDeterministicoSullaScena (par.3.4.5; A1c-L1-16 per lo smistamento): la scena del contesto con gli elenchi
// della fotografia permutati dà lo stesso esito del thread, byte per byte: associazioni, file da smistare, conflitti,
// smistamento di ogni prodotto, avvisi.
func TestSmistamentoDeterministicoSullaScena(t *testing.T) {
	th := scenaContesto(t)
	conProposta(&th, uid(0xd11), fZ, "altro", "nome_file", "scartata", nil, nil)
	vistaCome(&th)
	f := fotografia(th)
	r := insiemeACME(t)
	e1 := calcola(t, f, r, valutazione.Ingressi{})
	g, _ := permuta(f, valutazione.Ingressi{})
	e2 := calcola(t, g, r, valutazione.Ingressi{})
	t1, t2 := esitoThread(t, e1, threadACME), esitoThread(t, e2, threadACME)
	for _, c := range []struct {
		nome string
		a, b any
	}{{"associazioni", t1.Associazioni, t2.Associazioni}, {"da smistare", t1.DaSmistare, t2.DaSmistare}, {"conflitti", t1.Conflitti, t2.Conflitti},
		{"prodotti", t1.ProdottiValutati, t2.ProdottiValutati}, {"fascicolo", t1.Fascicolo, t2.Fascicolo}} {
		if canonicoDi(t, c.a) != canonicoDi(t, c.b) {
			t.Errorf("%s: gli elenchi permutati cambiano l'esito", c.nome)
		}
	}
	if e1.Impronta != e2.Impronta {
		t.Error("l'impronta dell'esito cambia con gli elenchi permutati")
	}
}

// TestEvidenzeSenzaIlRuoloProdotto (contratto §1.5, «La pertinenza e gli orfani», seconda e quarta evidenza; R76 b; R107):
// con una famiglia che ha solo il ruolo «componente», un file con il codice del prodotto non ha candidati a livello
// prodotto. Senza strutture del prodotto, la collocazione non determinabile per quel target è un'evidenza: il file è
// pertinente al prodotto, e sta in «da smistare» con collocazione_non_determinabile. Con lo STEP la cui radice ha la base
// del prodotto, il candidato della fonte (motore_a) è un'evidenza dello STEP. Il messaggio che scrive il codice del
// prodotto con quella famiglia non lo nomina (il ruolo «prodotto» manca): un file senza codice resta orfano.
func TestEvidenzeSenzaIlRuoloProdotto(t *testing.T) {
	f := famigliaCatena()
	f.Ruoli = []grammatica.Ruolo{grammatica.RuoloComponente}
	r := insiemeDi(t, voceRegole{cliente: clienteACME, file: "acme.v1.json", byte: grammaticaDi(t, clienteACME, "ACME S.p.A.", f)})
	rifQ := ancoraggio.RifComponente(cQ)
	scena := func() fotorfq.Thread {
		th := threadBase("Buongiorno,\r\nper 7120777A vi mandiamo i file.\r\nGrazie")
		th.Componenti = []fotorfq.Componente{componente(cQ, "7120777A", "finito", "manuale")}
		th.StepProdotto = []fotorfq.RigaStepProdotto{{ComponenteID: cQ, Codice: "7120777A", Esito: "da_scegliere"}}
		th.Fabbisogni = fabbisogniDefault()
		fileNelMessaggio(&th, fX, idM1, "7120777A_1.pdf", "pdf", shaN(161), scansione(t, shaN(161)))
		pdfSenzaCodice(t, &th, fY, idM1, "allegato-uno.pdf", 162)
		return th
	}
	et := esitoSmistamentoCon(t, scena(), r)
	if prodottoDi(t, et, rifQ).Struttura != ancoraggio.StatoStrutturaNessuna {
		t.Fatalf("il prodotto ha una struttura")
	}
	x := associazioneDi(t, et, fX)
	if x.Collocazione != ancoraggio.CollocazioneNonDeterminabile || len(x.Proposta) != 0 || !reflect.DeepEqual(x.Pertinente, []string{rifQ}) || x.PertinenzaContesto != nil {
		t.Errorf("il file con il codice del prodotto: %+v", x)
	}
	if d, ok := daSmistareDi(et, fX); !ok || d.Orfano || d.Motivo != valutazione.MotivoSmistamentoCollocazioneNonDeterminabile {
		t.Errorf("da smistare %+v %v", d, ok)
	}
	if d, ok := daSmistareDi(et, fY); !ok || !d.Orfano || d.ProdottiContesto != nil {
		t.Errorf("il file senza codice, con un messaggio che non nomina prodotti per questa famiglia: %+v %v", d, ok)
	}

	th := scena()
	st := fattiStepF(t, shaN(163), []nodoF{{"#1", "7120777A", ""}, {"#2", "7120778A", ""}}, []arcoF{{"#1", "#2", 1}})
	fileNelMessaggio(&th, fZ, idM1, "7120777A_1.stp", "stp", shaN(163), &st)
	et = esitoSmistamentoCon(t, th, r)
	candidato := false
	for _, c := range prodottoDi(t, et, rifQ).Fonte.Candidati {
		candidato = candidato || (c.Origine == ancoraggio.CandidatoDaMotoreA && c.AllegatoID != nil && *c.AllegatoID == fZ)
	}
	if z := associazioneDi(t, et, fZ); !candidato || len(z.Proposta) != 0 || !reflect.DeepEqual(z.Pertinente, []string{rifQ}) || z.PertinenzaContesto != nil {
		t.Errorf("lo STEP candidato della fonte (%v): %+v", candidato, z)
	}
}
