// L1 — le radici di riserva (estensione di R76 b, notte 14/10; passo 3 bis di ProponiStrutture) e la guardia su
// fuori_richiesta (L8): un target senza strutture prende come struttura candidata la radice, l'unica, di uno STEP che non
// ne ha la base, quando lo dice il nome del file (nome_file_step) o un'associazione decisa del file al componente del
// target (associazione_step); un file letto solo in namespace che nessun target e nessun nodo usa non è fuori richiesta;
// uno STEP «solo parti» resta un grafo incompleto (D-SHOCK-S1 B, D-SHOCK-S2 B). Con un target che ha già una struttura
// non cambia niente.
package ancoraggio_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// I clienti di questi test sono inventati (vedi struttura_test.go). Due spazi di codici ACME: il materiale «acme»
// (21 più sette cifre, 210000001) e il documento «acme-doc» (101 più sei cifre, il foglio dopo il trattino basso,
// 101000001_01). Lo STEP porta il numero del documento nei suoi nodi, il nome del file anche il materiale: è il caso che
// la regola 1 non vede, perché le basi si confrontano solo nello stesso namespace.

// famMaterialeRis: il materiale ACME, sui campi d'identità del file e sugli id dello STEP. Ruoli {prodotto, componente}.
func famMaterialeRis() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme", Namespace: "acme",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(grammatica.SegmentoBase{Nome: "codice", Pattern: "21[0-9]{7}", Identitario: true}),
		Forme: []grammatica.FormaCodice{{ID: "codice", Selettori: selStrutture, Stato: grammatica.StatoAttiva, Completa: true,
			Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}}},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-step", "radice_step.id", "210000001", false, grammatica.LetturaAttesa{Forma: "codice", Base: "210000001"}),
		},
	}
}

// famDocumentoRis: il documento ACME, con il foglio, sugli stessi campi. Ruoli {prodotto, componente}.
func famDocumentoRis() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-doc", Namespace: "acme-doc",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base: base(grammatica.SegmentoBase{Nome: "documento", Pattern: "101[0-9]{6}", Identitario: true},
			grammatica.SegmentoBase{Nome: "foglio", Pattern: "[0-9]{2}", Separatore: "_", Identitario: true}),
		Forme: []grammatica.FormaCodice{{ID: "codice", Selettori: selStrutture, Stato: grammatica.StatoAttiva, Completa: true,
			Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}}},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-step", "radice_step.id", "101000001_01", false, grammatica.LetturaAttesa{Forma: "codice", Base: "101000001_01"}),
		},
	}
}

func motoreRis(t *testing.T) *motorea.Motore {
	t.Helper()
	return motore(t, famMaterialeRis(), famDocumentoRis())
}

// stepRis: uno STEP con il nome dato e la radice #1, i figli e gli archi dati, con il grafo completo o con il motivo.
func stepRis(t *testing.T, m *motorea.Motore, id uuid.UUID, nome, sha string, radici []string, nodi []nodoS, archi []arcoS, motivo string) (ancoraggio.FileInterpretato, ancoraggio.StrutturaFile) {
	t.Helper()
	f := conDisponibilita(fileS(t, m, id, nome, "stp", sha, fattiS(t, sha, radici, nodi, archi, &motivo)), ancoraggio.DisponibilitaDisponibile)
	return f, strutturaS(t, f)
}

// targetRis: il prodotto target confermato 210000001, letto nel namespace del materiale, con il suo componente.
func targetRis(t *testing.T, m *motorea.Motore, componente uuid.UUID) ancoraggio.ProdottoRichiesto {
	t.Helper()
	tg := targetS(t, m, ancoraggio.RifComponente(componente), ancoraggio.AutoritaConfermata, "210000001")
	if tg.Namespace != "acme" || tg.Base.Normalizzata != "210000001" {
		t.Fatalf("il target nel namespace del materiale: %+v", tg)
	}
	tg.ComponenteID = &componente
	return tg
}

var (
	cProdottoR = uidS(0x901)
	idStepR    = uidS(0x902)
	shaStepR   = shaS("e")
	nodiStepR  = []nodoS{{"#1", "101000001_01", "ASSIEME"}, {"#2", "101000002_01", "PIASTRA"}}
	archiStepR = []arcoS{{"#1", "#2", 2, []string{"#21", "#22"}}}
)

// TestRadiceDalNomeDelloSTEP (passo 3 bis, nome_file_step; L1 e L2 della diagnosi del 14/10): lo STEP si chiama con il
// materiale del target e il documento della radice; la radice, l'unica, è letta solo come documento. Il target prende la
// struttura candidata sotto quella radice, con l'origine, la compatibilità non determinabile, i figli, il file come
// documento candidato della fonte con il motivo e senza radice compatibile, e la nota. Niente si promuove (R59 A).
func TestRadiceDalNomeDelloSTEP(t *testing.T) {
	m := motoreRis(t)
	_, s := stepRis(t, m, idStepR, "210000001_101000001_01.stp", shaStepR, []string{"#1"}, nodiStepR, archiStepR, "")
	var radice ancoraggio.NodoStruttura
	for _, n := range s.Nodi {
		if n.Chiave == "#1" {
			radice = n
		}
	}
	if len(radice.Letture) != 1 || radice.Letture[0].Forma.Namespace != "acme-doc" {
		t.Fatalf("la radice deve essere letta solo come documento: %+v", radice.Letture)
	}
	nome := map[string]bool{}
	for _, l := range s.NomeFile {
		nome[l.Lettura.Forma.Namespace+":"+l.Lettura.Forma.Base.Normalizzata] = true
	}
	if !nome["acme:210000001"] || !nome["acme-doc:101000001_01"] {
		t.Fatalf("il nome del file deve dare il materiale e il documento: %+v", nome)
	}
	tg := targetRis(t, m, cProdottoR)
	strutture, diag := proponiS(t, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}})
	if len(strutture) != 1 {
		t.Fatalf("attesa una struttura candidata: %+v", strutture)
	}
	x := strutture[0]
	if x.Origine != ancoraggio.OrigineStrutturaNomeFileSTEP || x.Stato != ancoraggio.StatoStrutturaCandidata || x.Compatibilita != motorea.CompatibilitaNonDeterminabile ||
		x.Radice != ancoraggio.RifNodo(shaStepR, "#1") || !x.RadiceDelFile || !x.GrafoCompleto ||
		!reflect.DeepEqual(rifNodi(x), []string{ancoraggio.RifNodo(shaStepR, "#1"), ancoraggio.RifNodo(shaStepR, "#2")}) {
		t.Errorf("la struttura dal nome dello STEP: %+v", x)
	}
	if len(x.Fonti) != 1 || x.Fonti[0].Motivo != ancoraggio.MotivoFonteNomeFileSTEP || x.Fonti[0].RadiceCompatibile != "" ||
		!reflect.DeepEqual(x.Fonti[0].Radici, []string{"#1"}) || *x.Fonti[0].AllegatoID != idStepR || x.Fonti[0].Estrazione != ancoraggio.EstrazioneRiuscita {
		t.Errorf("il file come documento candidato della fonte: %+v", x.Fonti)
	}
	note := conCodice(diag, ancoraggio.CodiceRadiceDalNomeFile)
	if len(diag) != 1 || len(note) != 1 || !reflect.DeepEqual(note[0].Rif, []string{tg.Rif, "allegato:" + idStepR.String(), ancoraggio.RifNodo(shaStepR, "#1")}) {
		t.Errorf("la nota, sola, al posto di target_senza_struttura: %+v", diag)
	}
	if got := ancoraggio.StatoStrutturaDelTarget(strutture, tg.Rif); got != ancoraggio.StatoStrutturaCandidata {
		t.Errorf("lo stato della struttura del target: %s", got)
	}
}

// TestRadiceDalNomeLeCondizioni (passo 3 bis): senza tutte le condizioni la radice di riserva non c'è, e il target resta
// senza struttura (target_senza_struttura): la radice letta nel namespace del target con un'altra base (la regola 1 la
// dice discordante: R76 b resta la regola), due radici nel file (nessuna scelta: T-E1-07), il nome con un altro
// materiale, il nome che non dice il materiale, la fonte confermata con la radice non registrata (T-B0-08), il nome
// con un documento che non concorda con quello della radice (NomeContraddiceLaRadice, revisione della notte 14/10).
func TestRadiceDalNomeLeCondizioni(t *testing.T) {
	m := motoreRis(t)
	tg := targetRis(t, m, cProdottoR)
	for _, c := range []struct {
		nome, file string
		radici     []string
		nodi       []nodoS
		archi      []arcoS
		fonte      bool
	}{
		{"radice discordante nel namespace del target", "210000001_101000001_01.stp", []string{"#1"},
			[]nodoS{{"#1", "210000009", "ASSIEME"}, {"#2", "101000002_01", "PIASTRA"}}, archiStepR, false},
		{"due radici", "210000001_101000001_01.stp", []string{"#1", "#2"}, nodiStepR, nil, false},
		{"un altro materiale nel nome", "210000009_101000001_01.stp", []string{"#1"}, nodiStepR, archiStepR, false},
		{"il nome senza il materiale", "101000001_01.stp", []string{"#1"}, nodiStepR, archiStepR, false},
		{"due materiali nel nome", "210000001_210000009.stp", []string{"#1"}, nodiStepR, archiStepR, false},
		{"la fonte confermata senza la radice", "210000001_101000001_01.stp", []string{"#1"}, nodiStepR, archiStepR, true},
		// revisione della notte 14/10: il nome dice il target, ma anche un documento che non è quello della radice
		{"il nome contraddice la radice", "210000001_101000555_01.stp", []string{"#1"}, nodiStepR, archiStepR, false},
	} {
		t.Run(c.nome, func(t *testing.T) {
			_, s := stepRis(t, m, idStepR, c.file, shaStepR, c.radici, c.nodi, c.archi, "")
			x := tg
			if c.fonte {
				x.FonteConfermata = fonteR59(shaStepR, "", &idStepR)
			}
			strutture, diag := proponiS(t, []ancoraggio.ProdottoRichiesto{x}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}})
			if len(strutture) != 0 || len(conCodice(diag, ancoraggio.CodiceTargetSenzaStruttura)) != 1 || len(conCodice(diag, ancoraggio.CodiceRadiceDalNomeFile)) != 0 {
				t.Errorf("nessuna radice di riserva: %+v %+v", strutture, diag)
			}
		})
	}
}

// TestRadiceDiRiservaNonTocca (passo 3 bis; regressione della regola 1): con una struttura dalla regola 1 in un altro
// STEP (la radice con la base del target), lo STEP con il nome del target e la radice muta non dà niente: le radici di
// riserva sono solo per un target senza strutture. La struttura della regola 1 non ha l'origine.
func TestRadiceDiRiservaNonTocca(t *testing.T) {
	m := motoreRis(t)
	_, riserva := stepRis(t, m, idStepR, "210000001_101000001_01.stp", shaStepR, []string{"#1"}, nodiStepR, archiStepR, "")
	idClassico, shaClassico := uidS(0x903), shaS("f")
	_, classico := stepRis(t, m, idClassico, "assieme.stp", shaClassico, []string{"#1"}, []nodoS{{"#1", "210000001", "ASSIEME"}, {"#2", "210000002", "PIASTRA"}},
		[]arcoS{{"#1", "#2", 1, nil}}, "")
	tg := targetRis(t, m, cProdottoR)
	strutture, diag := proponiS(t, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{riserva, classico}})
	if len(strutture) != 1 || strutture[0].AllegatoID != idClassico || strutture[0].Origine != "" || strutture[0].Compatibilita != motorea.CompatibilitaUguale || len(diag) != 0 {
		t.Errorf("solo la struttura della regola 1, senza origine e senza note: %+v %+v", strutture, diag)
	}
}

// TestRadiceDalloSTEPAssociato (passo 3 bis, associazione_step; L4 della diagnosi del 14/10): uno STEP senza codici nel
// nome, con la radice letta solo come documento, associato da una decisione al componente del target. La struttura
// candidata ha l'origine associazione_step e nessuna fonte (il candidato lo dà valutazione: documento_del_prodotto).
// Con anche il nome che dice il target, una struttura sola, con l'origine dell'associazione (la decisione di una
// persona) e il file come documento candidato. Un'associazione a un altro componente non prova niente.
func TestRadiceDalloSTEPAssociato(t *testing.T) {
	m := motoreRis(t)
	tg := targetRis(t, m, cProdottoR)
	associa := func(componente uuid.UUID) []ancoraggio.AssociazioneDecisa {
		return []ancoraggio.AssociazioneDecisa{{AllegatoID: idStepR, ComponenteID: componente, Origine: ancoraggio.OrigineManuale}}
	}
	t.Run("solo l'associazione", func(t *testing.T) {
		_, s := stepRis(t, m, idStepR, "assieme.stp", shaStepR, []string{"#1"}, nodiStepR, archiStepR, "")
		strutture, diag := proponiS(t, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, AssociazioniDecise: associa(cProdottoR)})
		if len(strutture) != 1 || strutture[0].Origine != ancoraggio.OrigineStrutturaAssociazioneSTEP || len(strutture[0].Fonti) != 0 ||
			strutture[0].Radice != ancoraggio.RifNodo(shaStepR, "#1") || len(strutture[0].Nodi) != 2 {
			t.Errorf("la struttura dallo STEP associato: %+v", strutture)
		}
		if len(diag) != 1 || len(conCodice(diag, ancoraggio.CodiceRadiceDalloSTEPAssociato)) != 1 {
			t.Errorf("la nota dell'associazione: %+v", diag)
		}
	})
	t.Run("la radice muta", func(t *testing.T) {
		_, s := stepRis(t, m, idStepR, "assieme.stp", shaStepR, []string{"#1"}, []nodoS{{"#1", "SALDATO", ""}, {"#2", "101000002_01", "PIASTRA"}}, archiStepR, "")
		strutture, _ := proponiS(t, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, AssociazioniDecise: associa(cProdottoR)})
		if len(strutture) != 1 || strutture[0].Origine != ancoraggio.OrigineStrutturaAssociazioneSTEP || strutture[0].Nodi[0].Leggibile {
			t.Errorf("la struttura sotto la radice muta: %+v", strutture)
		}
	})
	t.Run("il nome e l'associazione", func(t *testing.T) {
		_, s := stepRis(t, m, idStepR, "210000001_101000001_01.stp", shaStepR, []string{"#1"}, nodiStepR, archiStepR, "")
		strutture, diag := proponiS(t, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, AssociazioniDecise: associa(cProdottoR)})
		if len(strutture) != 1 || strutture[0].Origine != ancoraggio.OrigineStrutturaAssociazioneSTEP || len(strutture[0].Fonti) != 1 ||
			strutture[0].Fonti[0].Motivo != ancoraggio.MotivoFonteNomeFileSTEP {
			t.Errorf("una struttura sola, con l'origine dell'associazione e la fonte dal nome: %+v", strutture)
		}
		if len(diag) != 1 || len(conCodice(diag, ancoraggio.CodiceRadiceDalloSTEPAssociato)) != 1 {
			t.Errorf("una nota sola, quella dell'associazione: %+v", diag)
		}
	})
	t.Run("un altro componente", func(t *testing.T) {
		_, s := stepRis(t, m, idStepR, "assieme.stp", shaStepR, []string{"#1"}, nodiStepR, archiStepR, "")
		strutture, diag := proponiS(t, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, AssociazioniDecise: associa(uidS(0x9ff))})
		if len(strutture) != 0 || len(conCodice(diag, ancoraggio.CodiceTargetSenzaStruttura)) != 1 {
			t.Errorf("l'associazione a un altro componente non prova niente: %+v %+v", strutture, diag)
		}
	})
}

// TestGliAncoraggiSottoLaRadiceDiRiserva (passo 3 bis con ProponiAncoraggi): con la struttura dal nome dello STEP, lo STEP
// è la radice (il suo nome ha la base del target), il 2D della radice, letto come documento, è un candidato a livello
// prodotto con il motivo radice_di_riserva (la radice rappresenta il prodotto, e non è un nodo da componente), il 2D del
// figlio è un figlio. Nessun file della scena è fuori richiesta, e il determinismo regge con gli ingressi rovesciati.
// Il 2D di un figlio letto solo come materiale (revisione della notte 14/10) non ha candidati e non si confronta con
// niente: nessun nodo della struttura di riserva è letto nel namespace del target, che quindi non conta. Non
// determinabile con namespace_non_confrontabile, non fuori richiesta.
func TestGliAncoraggiSottoLaRadiceDiRiserva(t *testing.T) {
	m := motoreRis(t)
	step, s := stepRis(t, m, idStepR, "210000001_101000001_01.stp", shaStepR, []string{"#1"}, nodiStepR, archiStepR, "")
	tg := targetRis(t, m, cProdottoR)
	pdfRadice, pdfFiglio := cartiglio(t, m, 0x911, "101000001_01"), cartiglio(t, m, 0x912, "101000002_01")
	pdfMateriale := cartiglio(t, m, 0x913, "210000002")
	file := []ancoraggio.FileInterpretato{step, pdfRadice, pdfFiglio, pdfMateriale}
	ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}}
	e := ancoraA(t, file, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	radice := ancoraggio.RifNodo(shaStepR, "#1")
	for _, c := range []struct {
		id     uuid.UUID
		motivo string
	}{{idStepR, ancoraggio.MotivoCandidatoBaseDelTarget}, {pdfRadice.AllegatoID, ancoraggio.MotivoCandidatoRadiceDiRiserva}} {
		a := ancoraggioDi(t, e, c.id)
		if a.Associazione != ancoraggio.AssociazioneCandidatoUnico || a.Collocazione != ancoraggio.CollocazioneRadice {
			t.Fatalf("%s: il file della radice è il prodotto: %+v", c.id, a)
		}
		x := a.Candidati[0]
		if x.Target != tg.Rif || x.Livello != ancoraggio.LivelloProdotto || x.Motivo != c.motivo || len(x.Posizioni) != 1 || x.Posizioni[0].Nodo != radice ||
			x.Posizioni[0].Stato != ancoraggio.StatoStrutturaCandidata {
			t.Errorf("%s: il candidato a livello prodotto: %+v", c.id, x)
		}
	}
	f := ancoraggioDi(t, e, pdfFiglio.AllegatoID)
	if f.Collocazione != ancoraggio.CollocazioneFiglio || len(f.Candidati) != 1 || f.Candidati[0].Posizioni[0].Nodo != ancoraggio.RifNodo(shaStepR, "#2") {
		t.Errorf("il 2D del figlio: %+v", f)
	}
	if a := ancoraggioDi(t, e, pdfMateriale.AllegatoID); a.Collocazione != ancoraggio.CollocazioneNonDeterminabile || len(a.Candidati) != 0 ||
		!reflect.DeepEqual(a.Motivi, []string{ancoraggio.MotivoAncoraggioNamespaceNonConfrontabile}) {
		t.Errorf("il 2D del figlio letto solo come materiale: %+v", a)
	}
	for _, a := range e.File {
		if a.Collocazione == ancoraggio.CollocazioneFuoriRichiesta {
			t.Errorf("%s: fuori richiesta sotto la radice di riserva: %+v", a.AllegatoID, a)
		}
	}
	rov := ctx
	rov.Strutture = rovescia(ctx.Strutture)
	for i := range rov.Strutture {
		rov.Strutture[i].Nodi = rovescia(rov.Strutture[i].Nodi)
		rov.Strutture[i].NomeFile = rovescia(rov.Strutture[i].NomeFile)
	}
	fileRov := rovescia(file)
	for i := range fileRov {
		fileRov[i].Interpretazione.Letture = rovescia(fileRov[i].Interpretazione.Letture)
	}
	if b := ancoraA(t, fileRov, []ancoraggio.ProdottoRichiesto{tg}, rov); canonico(t, e) != canonico(t, b) {
		t.Errorf("le radici di riserva dipendono dall'ordine degli ingressi")
	}
}

// TestLaRadiceDiRiservaConIlSoloRuoloComponente (revisione della notte 14/10): la famiglia del documento ha il solo ruolo
// componente, quindi la lettura del 2D della radice non ha il ruolo prodotto. La radice di riserva non è fra le identità
// raggiungibili (radiceProdotto), ma la sua identità sostiene lo stesso il candidato a livello prodotto, con il motivo
// radice_di_riserva: il 2D della radice non è fuori richiesta. Il 2D del figlio resta un figlio.
func TestLaRadiceDiRiservaConIlSoloRuoloComponente(t *testing.T) {
	doc := famDocumentoRis()
	doc.Ruoli = []grammatica.Ruolo{grammatica.RuoloComponente}
	m := motore(t, famMaterialeRis(), doc)
	step, s := stepRis(t, m, idStepR, "210000001_101000001_01.stp", shaStepR, []string{"#1"}, nodiStepR, archiStepR, "")
	tg := targetRis(t, m, cProdottoR)
	pdfRadice, pdfFiglio := cartiglio(t, m, 0x941, "101000001_01"), cartiglio(t, m, 0x942, "101000002_01")
	for _, l := range pdfRadice.Interpretazione.Letture {
		if l.Funzione == motorea.FunzIdentitaFile && haRuoloR(l.RuoliCandidati, grammatica.RuoloProdotto) {
			t.Fatalf("la lettura del 2D della radice ha il ruolo prodotto: %+v", l)
		}
	}
	ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}}
	strutture, _ := proponiS(t, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	if len(strutture) != 1 || strutture[0].Origine != ancoraggio.OrigineStrutturaNomeFileSTEP {
		t.Fatalf("la struttura dal nome dello STEP: %+v", strutture)
	}
	e := ancoraA(t, []ancoraggio.FileInterpretato{step, pdfRadice, pdfFiglio}, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	a := ancoraggioDi(t, e, pdfRadice.AllegatoID)
	if a.Collocazione != ancoraggio.CollocazioneRadice || a.Associazione != ancoraggio.AssociazioneCandidatoUnico ||
		a.Candidati[0].Livello != ancoraggio.LivelloProdotto || a.Candidati[0].Motivo != ancoraggio.MotivoCandidatoRadiceDiRiserva {
		t.Errorf("il 2D della radice con il solo ruolo componente: %+v", a)
	}
	if f := ancoraggioDi(t, e, pdfFiglio.AllegatoID); f.Collocazione != ancoraggio.CollocazioneFiglio {
		t.Errorf("il 2D del figlio: %+v", f)
	}
	for _, x := range e.File {
		if x.Collocazione == ancoraggio.CollocazioneFuoriRichiesta {
			t.Errorf("%s: fuori richiesta sotto la radice di riserva: %+v", x.AllegatoID, x)
		}
	}
}

// haRuoloR: il ruolo è fra quelli dati.
func haRuoloR(ruoli []grammatica.Ruolo, r grammatica.Ruolo) bool {
	for _, x := range ruoli {
		if x == r {
			return true
		}
	}
	return false
}

// TestFuoriRichiestaSoloConfrontabile (L8 della diagnosi del 14/10): con lo scenario completo (un target, la sua
// struttura con il grafo completo), un 2D letto come materiale ma senza candidati resta fuori richiesta, come prima; un
// 2D letto solo come documento, un namespace che nessun target e nessun nodo usa, è non determinabile con
// namespace_non_confrontabile. Quando un nodo della struttura è letto come documento, il namespace si confronta, e lo
// stesso 2D torna fuori richiesta.
func TestFuoriRichiestaSoloConfrontabile(t *testing.T) {
	m := motoreRis(t)
	tg := targetRis(t, m, cProdottoR)
	estraneo, documento := cartiglio(t, m, 0x921, "210000999"), cartiglio(t, m, 0x922, "101000777_01")
	collocazione := func(e ancoraggio.EsitoAncoraggi, id uuid.UUID) string {
		a := ancoraggioDi(t, e, id)
		return string(a.Collocazione) + "/" + strings.Join(a.Motivi, ",")
	}

	step, s := stepRis(t, m, idStepR, "assieme.stp", shaStepR, []string{"#1"}, []nodoS{{"#1", "210000001", "ASSIEME"}, {"#2", "210000002", "PIASTRA"}},
		[]arcoS{{"#1", "#2", 1, nil}}, "")
	e := ancoraA(t, []ancoraggio.FileInterpretato{step, estraneo, documento}, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}})
	if got := collocazione(e, estraneo.AllegatoID); got != "fuori_richiesta/" {
		t.Errorf("il materiale senza candidati: %s", got)
	}
	if got := collocazione(e, documento.AllegatoID); got != "non_determinabile/"+ancoraggio.MotivoAncoraggioNamespaceNonConfrontabile {
		t.Errorf("il documento che non si confronta: %s", got)
	}

	step, s = stepRis(t, m, idStepR, "assieme.stp", shaStepR, []string{"#1"}, []nodoS{{"#1", "210000001", "ASSIEME"}, {"#2", "101000002_01", "PIASTRA"}},
		[]arcoS{{"#1", "#2", 1, nil}}, "")
	e = ancoraA(t, []ancoraggio.FileInterpretato{step, estraneo, documento}, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}})
	if got := collocazione(e, documento.AllegatoID); got != "fuori_richiesta/" {
		t.Errorf("il documento in un namespace dei nodi: %s", got)
	}
}

// TestLoSTEPSoloPartiRestaIncompleto (D-SHOCK-S1 B, D-SHOCK-S2 B dell'utente; 0020, struttura_motivo_parziale): uno
// STEP con una radice e nessuna occorrenza di assieme non dimostra che i figli non esistano, quindi il grafo resta non
// completo con il motivo «solo parti» (R32 b), anche quando la lettura non è troncata: la struttura del target dà
// grafo_incompleto, e un 2D senza candidati non è fuori richiesta. È la decisione dell'utente, non un difetto da
// correggere: la proposta della notte del 14/10 (un pezzo senza figli come grafo completo di un nodo) non si applica.
func TestLoSTEPSoloPartiRestaIncompleto(t *testing.T) {
	m := motoreRis(t)
	tg := targetRis(t, m, cProdottoR)
	step, s := stepRis(t, m, idStepR, "pezzo.stp", shaStepR, []string{"#1"}, []nodoS{{"#1", "210000001", "PEZZO"}}, nil, motivoSoloParti)
	if s.GrafoCompleto || s.MotivoGrafo != motivoSoloParti {
		t.Fatalf("lo STEP «solo parti» ha il grafo non completo, con il motivo: %+v", s)
	}
	estraneo := cartiglio(t, m, 0x931, "210000999")
	e := ancoraA(t, []ancoraggio.FileInterpretato{step, estraneo}, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}})
	if len(conCodice(e.Diagnostiche, ancoraggio.CodiceGrafoIncompleto)) != 1 {
		t.Errorf("la struttura del target dice grafo_incompleto: %+v", e.Diagnostiche)
	}
	if a := ancoraggioDi(t, e, estraneo.AllegatoID); a.Collocazione != ancoraggio.CollocazioneNonDeterminabile ||
		!reflect.DeepEqual(a.Motivi, []string{ancoraggio.MotivoAncoraggioGrafoIncompleto}) {
		t.Errorf("un 2D senza candidati non è fuori richiesta accanto a uno STEP «solo parti»: %+v", a)
	}
}
