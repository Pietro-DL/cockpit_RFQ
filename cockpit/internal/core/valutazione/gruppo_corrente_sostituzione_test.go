// L1 — T-09, la sostituzione dello STEP con una radice di riserva (V1: emendamento di V1 §6.4.1, «Perché serve» e «Il
// risultato», con il quarto giro su O-01, OQ-02; piano della 0023, T-09). Le strutture si calcolano con Calcola, quindi con
// ProponiStrutture e con quello che la valutazione le passa dalla fotografia (la fonte confermata solo con la vista che lo
// dice, le associazioni decise senza i documenti sostituiti), e poi passano per propostabom.GruppoCorrenteDelProdotto con
// la sorgente di una bozza che ha il registro sul gruppo «bom» dello STEP A. È una prova di propostabom che sta qui perché
// usa le scene della valutazione (le prove di propostabom non possono importare gli aiuti di queste). Tutto è inventato:
// il finito 210000001 della grammatica delle radici di riserva (insiemeRiserve), gli STEP A e B, le loro righe.
package valutazione_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/propostabom"
)

var (
	cSost    = uid(0x5a01)
	aSostA   = uid(0x5a11)
	aSostB   = uid(0x5a12)
	aSostC   = uid(0x5a13) // un altro STEP con una struttura compatibile del prodotto (il cancello del passo 3 bis)
	dSostA   = uid(0x5a21)
	dSostB   = uid(0x5a22)
	shaSostA = strings.Repeat("5a", 32)
	shaSostB = strings.Repeat("5b", 32)
	shaSostC = strings.Repeat("5c", 32)
)

// I momenti della scena della sostituzione.
//   - candidata: prima di «Autorizza»: A è un documento confermato sul finito (l'associazione), nessuno STEP autorizzato;
//   - autorizzato: dopo «Autorizza» su A, la BOM di lavoro, dove la bozza prende il registro sul gruppo «bom»;
//   - si, no: A sostituito da B (fascicolo.Sostituisci), con la risposta alla domanda «il nuovo file diventa lo STEP
//     autorizzato?».
const (
	momentoCandidata   = "candidata"
	momentoAutorizzato = "autorizzato"
	momentoSi          = "si"
	momentoNo          = "no"
)

// Le radici e i nomi degli STEP della scena.
const (
	radiceRiserva     = "101000001_01" // letta solo come documento (acme-doc): nessuna base del finito
	radiceCompatibile = "210000001"    // la base del finito (acme)
	radiceAltroDoc    = "101000009_01" // B senza una struttura compatibile
	nomeDiceIlFinito  = "210000001_101000001_01.stp"
	nomeMuto          = "assieme-a.stp"
	nomeB             = "nuovo-b.stp"
)

// sostituzione: un momento della scena, con la radice e il nome di A e la radice di B (B non c'è prima della sostituzione).
type sostituzione struct {
	momento        string
	radiceA, nomeA string
	radiceB        string
}

// scenaSostituzione: la RFQ ACME con il finito manuale 210000001 e lo STEP A (radice #1 e un figlio), poi B, come le
// scrive il legacy:
//   - «Autorizza» (marcaSorgente): step_strutturale_id, la vista presente_analizzato, la riga della radice decisa come il
//     prodotto (duplicato, da una persona) con la marcatura;
//   - la sostituzione: documento.sostituito_da su A; le righe ancora aperte di A chiuse dall'automatismo (chiudiProposteDi:
//     scartate senza chi le ha decise, con la nota); B con i suoi fatti, il suo documento sul finito e le sue righe;
//   - «no»: l'autorizzazione resta su A ed è superata (la vista riferimento_superato), con la marcatura di A;
//   - «sì»: l'autorizzazione del file nuovo (DichiaraStrutturale): la marcatura di A va nella storia, la radice di B è
//     decisa come il prodotto con la sua marcatura, la vista presente_analizzato su B.
func scenaSostituzione(t *testing.T, s sostituzione) fotorfq.Thread {
	t.Helper()
	th := threadBase("Buongiorno,\r\nRichiesta di offerta.")
	th.Componenti = []fotorfq.Componente{componente(cSost, "210000001", "finito", "manuale")}
	cp, op := cSost, operatore
	il := dataACME.Add(2 * time.Hour)
	vista := func(step *uuid.UUID, esito string) {
		th.StepProdotto = []fotorfq.RigaStepProdotto{{ComponenteID: cSost, Codice: "210000001", StepStrutturaleID: step, NStepCorrenti: 1,
			AnalisiCompleta: true, Esito: esito}}
	}
	// righe: una riga aperta per nodo e l'arco aperto di uno STEP (radice #1, figlio #2); dà l'indice della riga della radice.
	righe := func(a uuid.UUID, nome, sha string, base int, radice, figlio string) int {
		i := len(th.RigheComponenteProposta)
		for k, id := range []string{radice, figlio} {
			th.RigheComponenteProposta = append(th.RigheComponenteProposta, fotorfq.RigaComponenteProposta{ID: uid(base + k), AllegatoID: a, NomeFile: nome,
				Sha256: sha, Chiave: fmt.Sprintf("#%d", k+1), IDGrezzo: id, Famiglia: "acme", Fonte: "step", Stato: "aperta"})
		}
		th.RigheRelazioneProposta = append(th.RigheRelazioneProposta, fotorfq.RigaRelazioneProposta{AllegatoID: a, NomeFile: nome, PadreChiave: "#1",
			FiglioChiave: "#2", Qta: 1, Stato: "aperta"})
		return i
	}
	// autorizza: la riga della radice decisa come il prodotto, da chi autorizza, con la marcatura del documento.
	autorizza := func(i int, doc uuid.UUID) {
		d := doc
		r := &th.RigheComponenteProposta[i]
		r.Stato, r.ComponenteID, r.DecisoDa, r.DecisoIl = "duplicato", &cp, &op, &il
		r.Marcatura = &fotorfq.MarcaturaStrutturale{Versione: 1, Ruolo: "radice", ComponenteID: &cp, DocumentoID: &d, DichiaratoDa: &op,
			DichiaratoIl: "2026-10-01T08:00:00Z"}
	}

	fa := fattiSTEP(t, shaSostA, []string{"#1"}, []nodoSTEP{{"#1", s.radiceA}, {"#2", "101000002_01"}}, [][2]string{{"#1", "#2"}}, "")
	conAllegato(&th, allegato(aSostA, 1, s.nomeA, "stp", shaSostA), &fa)
	docA := fotorfq.DocumentoConfermato{ID: dSostA, ThreadID: threadACME, ComponenteID: &cp, Tipo: "cad_3d", NomeFile: s.nomeA, Estensione: "stp",
		Sha256: shaSostA, StatoNas: "scritto", ConfermatoDa: operatore, ConfermatoIl: dataACME, Allegati: []uuid.UUID{aSostA}}
	radA := righe(aSostA, s.nomeA, shaSostA, 0x5a31, s.radiceA, "101000002_01")
	if s.momento == momentoCandidata {
		th.Documenti = []fotorfq.DocumentoConfermato{docA}
		vista(nil, "da_scegliere")
		return th
	}
	dA := dSostA
	autorizza(radA, dSostA)
	th.Componenti[0].StepStrutturaleID = &dA
	vista(&dA, "presente_analizzato")
	if s.momento == momentoAutorizzato {
		th.Documenti = []fotorfq.DocumentoConfermato{docA}
		return th
	}

	fb := fattiSTEP(t, shaSostB, []string{"#1"}, []nodoSTEP{{"#1", s.radiceB}, {"#2", "101000003_01"}}, [][2]string{{"#1", "#2"}}, "")
	conAllegato(&th, allegato(aSostB, 2, nomeB, "stp", shaSostB), &fb)
	dB := dSostB
	docA.SostituitoDa = &dB
	docB := fotorfq.DocumentoConfermato{ID: dSostB, ThreadID: threadACME, ComponenteID: &cp, Tipo: "cad_3d", NomeFile: nomeB, Estensione: "stp",
		Sha256: shaSostB, StatoNas: "scritto", ConfermatoDa: operatore, ConfermatoIl: dataACME.Add(time.Hour), Allegati: []uuid.UUID{aSostB}}
	th.Documenti = []fotorfq.DocumentoConfermato{docA, docB}
	nota := "superata da " + nomeB
	for i := range th.RigheComponenteProposta {
		if r := &th.RigheComponenteProposta[i]; r.Stato == "aperta" {
			r.Stato, r.Nota = "scartata", &nota
		}
	}
	for i := range th.RigheRelazioneProposta {
		if r := &th.RigheRelazioneProposta[i]; r.Stato == "aperta" {
			r.Stato, r.Nota = "scartata", &nota
		}
	}
	radB := righe(aSostB, nomeB, shaSostB, 0x5a41, s.radiceB, "101000003_01")
	switch s.momento {
	case momentoNo:
		vista(&dA, "riferimento_superato")
	case momentoSi:
		th.RigheComponenteProposta[radA].Marcatura = nil
		autorizza(radB, dSostB)
		th.Componenti[0].StepStrutturaleID = &dB
		vista(&dB, "presente_analizzato")
	default:
		t.Fatalf("momento %q", s.momento)
	}
	return th
}

// TestT09LaSostituzioneConUnaRadiceDiRiserva (T-09, quarto giro su O-01, OQ-02): un registro sul «bom» dello STEP A, con una
// radice di riserva nata dall'associazione (origine associazione_step), e A sostituito dal file B dello stesso prodotto.
// La bozza ha la radice (fonte_radice del registro): la sorgente è (A, sha256 di A, #1 di A, «bom»).
//   - prima: la candidata con l'origine associazione_step, poi, dopo «Autorizza», la BOM di lavoro su A: «bom»,
//     stessa_occorrenza;
//   - (i) «sì», con B autorizzato (la sua radice si chiama come il prodotto): la struttura di A non è più fra i gruppi del
//     prodotto: assente, nessun_gruppo, con o senza il nome di A che dice il prodotto;
//   - (ii) «no», con B compatibile: lo stesso;
//   - (iii) «no», B senza una struttura compatibile e il nome di A che dice il prodotto: cand:A:#1, stessa_occorrenza, e
//     l'origine passa a nome_file_step;
//   - (iv) «no», B senza una struttura compatibile e, come sola prova di A, l'associazione del documento sostituito:
//     assente, nessun_gruppo;
//   - (v) gli stessi passaggi con una radice compatibile, con «sì» e con «no»: cand:A:#1, stessa_occorrenza.
//
// Mai altra_occorrenza: B è un altro contenuto.
func TestT09LaSostituzioneConUnaRadiceDiRiserva(t *testing.T) {
	r := insiemeRiserve(t)
	rif := ancoraggio.RifComponente(cSost)
	radiceDiA := ancoraggio.RifNodo(shaSostA, "#1")
	sorgente := propostabom.SorgenteBozza{AllegatoID: aSostA, Sha256: shaSostA, Radice: radiceDiA, Gruppo: propostabom.GruppoBOM}
	candA := propostabom.ChiaveCandidata(aSostA, radiceDiA)
	assente := propostabom.GruppoCorrente{Come: propostabom.ComeAssente, Motivo: propostabom.MotivoNessunGruppo}
	candidataDiA := propostabom.GruppoCorrente{Chiave: candA, Come: propostabom.ComeStessaOccorrenza}

	// corrente: il gruppo corrente della sorgente sull'esito di Calcola, e l'origine della struttura candidata di A ("-" se
	// A non ha una struttura candidata).
	corrente := func(t *testing.T, s sostituzione) (propostabom.GruppoCorrente, string) {
		t.Helper()
		et := esitoSmistamentoCon(t, scenaSostituzione(t, s), r)
		pv := prodottoDi(t, et, rif)
		origine := "-"
		for _, sc := range pv.StruttureCandidate {
			if sc.AllegatoID == aSostA && sc.Radice == radiceDiA {
				origine = sc.Origine
			}
		}
		return propostabom.GruppoCorrenteDelProdotto(pv, et.Ancoraggi.Strutture, nil, sorgente), origine
	}
	uguale := func(a, b propostabom.GruppoCorrente) bool {
		return a.Chiave == b.Chiave && a.Come == b.Come && a.Motivo == b.Motivo && (a.AllegatoID == nil) == (b.AllegatoID == nil)
	}

	t.Run("prima della sostituzione", func(t *testing.T) {
		for _, nomeA := range []string{nomeMuto, nomeDiceIlFinito} {
			got, origine := corrente(t, sostituzione{momento: momentoCandidata, radiceA: radiceRiserva, nomeA: nomeA})
			if !uguale(got, candidataDiA) || origine != ancoraggio.OrigineStrutturaAssociazioneSTEP {
				t.Errorf("%s, prima di «Autorizza»: %+v con l'origine %q; attesa la candidata con l'origine associazione_step", nomeA, got, origine)
			}
			got, _ = corrente(t, sostituzione{momento: momentoAutorizzato, radiceA: radiceRiserva, nomeA: nomeA})
			if want := (propostabom.GruppoCorrente{Chiave: propostabom.GruppoBOM, Come: propostabom.ComeStessaOccorrenza}); !uguale(got, want) {
				t.Errorf("%s, dopo «Autorizza»: %+v, atteso %+v", nomeA, got, want)
			}
		}
	})

	casi := []struct {
		nome   string
		s      sostituzione
		want   propostabom.GruppoCorrente
		origin string // l'origine della candidata di A: "-" senza
	}{
		{"(i) «sì», B autorizzato", sostituzione{momentoSi, radiceRiserva, nomeMuto, radiceCompatibile}, assente, "-"},
		{"(i) «sì», B autorizzato, il nome di A dice il prodotto", sostituzione{momentoSi, radiceRiserva, nomeDiceIlFinito, radiceCompatibile}, assente, "-"},
		{"(ii) «no», B compatibile", sostituzione{momentoNo, radiceRiserva, nomeMuto, radiceCompatibile}, assente, "-"},
		{"(ii) «no», B compatibile, il nome di A dice il prodotto", sostituzione{momentoNo, radiceRiserva, nomeDiceIlFinito, radiceCompatibile}, assente, "-"},
		{"(iii) «no», B senza struttura compatibile, il nome di A dice il prodotto", sostituzione{momentoNo, radiceRiserva, nomeDiceIlFinito, radiceAltroDoc},
			candidataDiA, ancoraggio.OrigineStrutturaNomeFileSTEP},
		{"(iv) «no», B senza struttura compatibile, la sola associazione del documento sostituito", sostituzione{momentoNo, radiceRiserva, nomeMuto, radiceAltroDoc},
			assente, "-"},
		{"(v) radice compatibile, «sì»", sostituzione{momentoSi, radiceCompatibile, nomeMuto, radiceCompatibile}, candidataDiA, ""},
		{"(v) radice compatibile, «no»", sostituzione{momentoNo, radiceCompatibile, nomeMuto, radiceCompatibile}, candidataDiA, ""},
		{"(v) radice compatibile, «no», B senza struttura compatibile", sostituzione{momentoNo, radiceCompatibile, nomeMuto, radiceAltroDoc}, candidataDiA, ""},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			// la stessa scena prima della sostituzione: la bozza seguiva la BOM di lavoro su A
			prima, _ := corrente(t, sostituzione{momento: momentoAutorizzato, radiceA: c.s.radiceA, nomeA: c.s.nomeA})
			if prima.Chiave != propostabom.GruppoBOM || prima.Come != propostabom.ComeStessaOccorrenza {
				t.Fatalf("prima della sostituzione: %+v, atteso «bom» sulla stessa occorrenza", prima)
			}
			got, origine := corrente(t, c.s)
			if !uguale(got, c.want) || origine != c.origin {
				t.Errorf("gruppo corrente %+v (allegato %v), origine della candidata di A %q; attesi %+v e %q", got, got.AllegatoID, origine, c.want, c.origin)
			}
			if got.Come == propostabom.ComeAltraOccorrenza {
				t.Errorf("altra_occorrenza con un altro contenuto: %+v", got)
			}
		})
	}
}

// revocaDiA: «Revoca» dell'autorizzazione di A come la scrive il legacy (F/autorizzazione.go:1117): step_strutturale_id
// vuoto, la vista da_scegliere, la riga della radice di A riaperta, con la marcatura nella storia. Il documento di A resta
// confermato sul prodotto: l'associazione decisa resta una prova del passo 3 bis.
func revocaDiA(th *fotorfq.Thread) {
	th.Componenti[0].StepStrutturaleID = nil
	th.StepProdotto[0].StepStrutturaleID, th.StepProdotto[0].Esito = nil, "da_scegliere"
	for i := range th.RigheComponenteProposta {
		if r := &th.RigheComponenteProposta[i]; r.AllegatoID == aSostA && r.Chiave == "#1" {
			r.Stato, r.ComponenteID, r.DecisoDa, r.DecisoIl, r.Marcatura = "aperta", nil, nil, nil, nil
		}
	}
}

// conFileCompatibile: lo STEP C del prodotto, con la radice che ha la base del finito (la regola 1 di ProponiStrutture),
// senza documento, con le sue righe aperte.
func conFileCompatibile(t *testing.T, th *fotorfq.Thread) {
	t.Helper()
	fc := fattiSTEP(t, shaSostC, []string{"#1"}, []nodoSTEP{{"#1", radiceCompatibile}, {"#2", "101000004_01"}}, [][2]string{{"#1", "#2"}}, "")
	conAllegato(th, allegato(aSostC, 3, "compatibile-c.stp", "stp", shaSostC), &fc)
	for k, id := range []string{radiceCompatibile, "101000004_01"} {
		th.RigheComponenteProposta = append(th.RigheComponenteProposta, fotorfq.RigaComponenteProposta{ID: uid(0x5a51 + k), AllegatoID: aSostC,
			NomeFile: "compatibile-c.stp", Sha256: shaSostC, Chiave: fmt.Sprintf("#%d", k+1), IDGrezzo: id, Famiglia: "acme", Fonte: "step", Stato: "aperta"})
	}
}

// TestT09IlCancelloDelPasso3Bis (T-09, il cancello del passo 3 bis anche per «Revoca» e per la sostituzione di uno STEP
// non autorizzato; EM §6.4.1, «Lo stesso cancello vale per "Revoca", per il cambio di tipo e per la sostituzione di uno
// STEP non autorizzato»; recepimento dell'ondata A, residuo di O-01), con le strutture calcolate dalla fotografia con
// Calcola. Una radice di riserva torna fra le candidate solo se il passo 3 bis la dà ancora: una radice sola nello STEP,
// nessuna struttura compatibile del prodotto da nessun file, nessuna fonte confermata, con la prova del nome del file o
// dell'associazione decisa.
//   - una bozza sul «bom» di A (radice di riserva), poi «Revoca», senza un file compatibile: cand:A:#1, stessa_occorrenza;
//   - la stessa «Revoca» con un altro file con una struttura compatibile del prodotto: assente, nessun_gruppo;
//   - A con due radici, la giusta scelta da «Autorizza», poi «Revoca»: assente, nessun_gruppo;
//   - una bozza sulla candidata di A (non autorizzato), poi la sostituzione di A con un file compatibile, senza la
//     domanda: assente, nessun_gruppo.
//
// I casi P-4 (a) e (b) con i fatti calcolati (l'occorrenza della bozza in «Altri file») stanno in TestA28LoSTEPNonAttivo.
// Il cambio di tipo del prodotto non è qui: con il tipo commerciale il componente non è più un prodotto della valutazione
// (i target sono i finiti), e il gruppo corrente non si calcola per lui (resoconto della corsia, ondata B).
func TestT09IlCancelloDelPasso3Bis(t *testing.T) {
	r := insiemeRiserve(t)
	rif := ancoraggio.RifComponente(cSost)
	radiceDiA := ancoraggio.RifNodo(shaSostA, "#1")
	candA := propostabom.ChiaveCandidata(aSostA, radiceDiA)
	sulBOM := propostabom.SorgenteBozza{AllegatoID: aSostA, Sha256: shaSostA, Radice: radiceDiA, Gruppo: propostabom.GruppoBOM}
	sullaCandidata := propostabom.SorgenteBozza{AllegatoID: aSostA, Sha256: shaSostA, Radice: radiceDiA, Gruppo: candA}
	corrente := func(t *testing.T, th fotorfq.Thread, s propostabom.SorgenteBozza) propostabom.GruppoCorrente {
		t.Helper()
		et := esitoSmistamentoCon(t, th, r)
		return propostabom.GruppoCorrenteDelProdotto(prodottoDi(t, et, rif), et.Ancoraggi.Strutture, nil, s)
	}
	assente := propostabom.GruppoCorrente{Come: propostabom.ComeAssente, Motivo: propostabom.MotivoNessunGruppo}
	candidataDiA := propostabom.GruppoCorrente{Chiave: candA, Come: propostabom.ComeStessaOccorrenza}
	bomDiA := propostabom.GruppoCorrente{Chiave: propostabom.GruppoBOM, Come: propostabom.ComeStessaOccorrenza}
	uguale := func(a, b propostabom.GruppoCorrente) bool {
		return a.Chiave == b.Chiave && a.Come == b.Come && a.Motivo == b.Motivo && (a.AllegatoID == nil) == (b.AllegatoID == nil)
	}
	autorizzato := func(t *testing.T) fotorfq.Thread {
		return scenaSostituzione(t, sostituzione{momento: momentoAutorizzato, radiceA: radiceRiserva, nomeA: nomeMuto})
	}

	t.Run("«Revoca» senza un file compatibile", func(t *testing.T) {
		th := autorizzato(t)
		if got := corrente(t, th, sulBOM); !uguale(got, bomDiA) {
			t.Fatalf("prima di «Revoca»: %+v", got)
		}
		revocaDiA(&th)
		if got := corrente(t, th, sulBOM); !uguale(got, candidataDiA) {
			t.Errorf("dopo «Revoca», senza un file compatibile: %+v, attesa la candidata di A", got)
		}
	})
	t.Run("«Revoca» con un file compatibile", func(t *testing.T) {
		th := autorizzato(t)
		conFileCompatibile(t, &th)
		if got := corrente(t, th, sulBOM); !uguale(got, bomDiA) {
			t.Fatalf("prima di «Revoca», con il file compatibile: %+v", got)
		}
		revocaDiA(&th)
		if got := corrente(t, th, sulBOM); !uguale(got, assente) {
			t.Errorf("dopo «Revoca», con un file compatibile: %+v, atteso assente / nessun_gruppo", got)
		}
	})
	t.Run("A con due radici, «Revoca»", func(t *testing.T) {
		th := autorizzato(t)
		th.Fatti[shaSostA] = fattiSTEP(t, shaSostA, []string{"#1", "#3"}, []nodoSTEP{{"#1", radiceRiserva}, {"#2", "101000002_01"}, {"#3", "101000007_01"}},
			[][2]string{{"#1", "#2"}}, "")
		th.RigheComponenteProposta = append(th.RigheComponenteProposta, fotorfq.RigaComponenteProposta{ID: uid(0x5a61), AllegatoID: aSostA, NomeFile: nomeMuto,
			Sha256: shaSostA, Chiave: "#3", IDGrezzo: "101000007_01", Famiglia: "acme", Fonte: "step", Stato: "aperta"})
		if got := corrente(t, th, sulBOM); !uguale(got, bomDiA) {
			t.Fatalf("prima di «Revoca», con la radice scelta da «Autorizza»: %+v", got)
		}
		revocaDiA(&th)
		if got := corrente(t, th, sulBOM); !uguale(got, assente) {
			t.Errorf("dopo «Revoca», con due radici: %+v, atteso assente / nessun_gruppo", got)
		}
	})
	t.Run("la sostituzione di uno STEP non autorizzato", func(t *testing.T) {
		th := scenaSostituzione(t, sostituzione{momento: momentoCandidata, radiceA: radiceRiserva, nomeA: nomeMuto})
		if got := corrente(t, th, sullaCandidata); !uguale(got, candidataDiA) {
			t.Fatalf("prima della sostituzione: %+v", got)
		}
		// la sostituzione senza la domanda (F/struttura.go:327-330): sostituito_da su A, le righe aperte di A chiuse
		// dall'automatismo, B con il suo documento sul finito e le sue righe; nessuna autorizzazione
		fb := fattiSTEP(t, shaSostB, []string{"#1"}, []nodoSTEP{{"#1", radiceCompatibile}, {"#2", "101000003_01"}}, [][2]string{{"#1", "#2"}}, "")
		conAllegato(&th, allegato(aSostB, 2, nomeB, "stp", shaSostB), &fb)
		dB, cp := dSostB, cSost
		th.Documenti[0].SostituitoDa = &dB
		th.Documenti = append(th.Documenti, fotorfq.DocumentoConfermato{ID: dSostB, ThreadID: threadACME, ComponenteID: &cp, Tipo: "cad_3d", NomeFile: nomeB,
			Estensione: "stp", Sha256: shaSostB, StatoNas: "scritto", ConfermatoDa: operatore, ConfermatoIl: dataACME.Add(time.Hour), Allegati: []uuid.UUID{aSostB}})
		nota := "superata da " + nomeB
		for i := range th.RigheComponenteProposta {
			if r := &th.RigheComponenteProposta[i]; r.Stato == "aperta" {
				r.Stato, r.Nota = "scartata", &nota
			}
		}
		for k, id := range []string{radiceCompatibile, "101000003_01"} {
			th.RigheComponenteProposta = append(th.RigheComponenteProposta, fotorfq.RigaComponenteProposta{ID: uid(0x5a71 + k), AllegatoID: aSostB, NomeFile: nomeB,
				Sha256: shaSostB, Chiave: fmt.Sprintf("#%d", k+1), IDGrezzo: id, Famiglia: "acme", Fonte: "step", Stato: "aperta"})
		}
		if got := corrente(t, th, sullaCandidata); !uguale(got, assente) {
			t.Errorf("dopo la sostituzione non autorizzata con un file compatibile: %+v, atteso assente / nessun_gruppo", got)
		}
	})
}
