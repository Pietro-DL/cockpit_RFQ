// L1 — Calcola, il punto d'ingresso del percorso puro (piano A, 6.4.6, la sequenza di Calcola; par.3.3.8; contratto
// §2.3, «Campi nuovi di EsitoThread»; fase 0 di B6, F.2, F.3, IM.1; T-B6-02, T-B6-09, T-B6-10, F0-03, F0-04, F0-05,
// F0-13): A1c-L1-16 (il percorso completo, i thread senza grammatica, con la grammatica scartata e con la ragione sociale
// discorde, un adattatore in errore che non ferma il thread, l'atteso che non compare mai: R2), A1c-L1-14 riscritta sulla
// disponibilità del contratto (senza_testo per una scansione: T-B0-31, T-B6-02, con la deviazione dal piano nominata), gli
// errori della valutazione, i messaggi fuori RFQ dei casi di censimento (R34), i conflitti composti, il determinismo.
//
// I clienti, i codici, i nomi dei file e gli ID sono inventati (ACME, 712xxxx, acme.example): il repository è pubblico.
package valutazione_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

func contieneCodice(d []evidenze.Diagnostica, codice string) bool {
	return len(conCodice(d, codice)) > 0
}

// TestL116IlPercorsoCompleto (A1c-L1-16; 6.4.6 passi 1–11; T-B6-09; F0-03, F0-04): quattro RFQ in una fotografia. Quella
// di ACME è valutata con il suo motore, e l'esito porta il nuovo di ValutaProdotti composto nel thread, un record per
// allegato e i record piatti; le altre tre non sono valutate, con il motivo della grammatica (senza voce nell'indice,
// scartata, ragione sociale discorde) e le diagnostiche di MotoreDi, ma i loro prodotti ci sono, come informazione, e i
// loro allegati hanno il record piatto con il nuovo non valutato.
func TestL116IlPercorsoCompleto(t *testing.T) {
	r := insiemeACME(t)
	thread := quattroThread(t)
	for i := 1; i < len(thread); i++ {
		thread[i].Identificativi = []fotorfq.Identificativo{confermato("7120100A2")}
	}
	a := allegato(uid(0x6c1), 1, "7120100A_1.stp", "stp", sha("e"))
	a.MessaggioID = mSenzaRegole
	thread[1].Allegati = []fotorfq.Allegato{a}
	thread[1].Proposte = []fotorfq.PropostaAttuale{{ID: uid(0x6c2), AllegatoID: a.ID, Tipo: "cad_3d", Codice: testo("7120100A"), Fonte: "step", Stato: "aperta"}}
	f := fotografiaDi(thread...)
	e := calcola(t, f, r, valutazione.Ingressi{Versione: valutazione.VersioneCasi})

	if len(e.Thread) != 4 || e.ImprontaIndice != r.ImprontaIndice || e.VersioneLimiti != "limiti-acme-1" || len(e.ImprontaFotografia) != 64 {
		t.Fatalf("esito: %d thread, indice %q, limiti %q, fotografia %q", len(e.Thread), e.ImprontaIndice, e.VersioneLimiti, e.ImprontaFotografia)
	}
	if h, _ := fotorfq.ImprontaFotografia(f); h != e.ImprontaFotografia {
		t.Errorf("impronta della fotografia %s, attesa %s", e.ImprontaFotografia, h)
	}

	t.Run("il thread valutato", func(t *testing.T) {
		m := r.Motori[clienteACME]
		et := esitoThread(t, e, threadACME)
		if !et.Valutato || et.Motivo != "" || et.HashSnapshot != m.Snapshot().Hash || et.ClienteID != clienteACME {
			t.Fatalf("thread ACME: valutato %v, motivo %q, snapshot %q", et.Valutato, et.Motivo, et.HashSnapshot)
		}
		v, err := valutazione.ValutaProdotti(f, threadDiID(t, f, threadACME), m, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct {
			nome      string
			esito, vp any
		}{{"richiesta", et.Richiesta, v.Richiesta}, {"prodotti", et.Prodotti, v.Prodotti}, {"ancoraggi", et.Ancoraggi, v.Ancoraggi},
			{"prodotti valutati", et.ProdottiValutati, v.ProdottiValutati}} {
			if canonicoDi(t, c.esito) != canonicoDi(t, c.vp) {
				t.Errorf("%s: l'esito non porta quelli di ValutaProdotti", c.nome)
			}
		}
		for _, d := range v.Diagnostiche {
			if !contieneCodice(et.Diagnostiche, d.Codice) {
				t.Errorf("la diagnostica %s di ValutaProdotti non è fra quelle del thread", d.Codice)
			}
		}
		for _, d := range et.Diagnostiche { // F0-04: quelle di ancoraggio restano in Ancoraggi
			for _, da := range et.Ancoraggi.Diagnostiche {
				if reflect.DeepEqual(d, da) {
					t.Errorf("la diagnostica %s di ancoraggio è copiata fra quelle del thread (F0-04)", d.Codice)
				}
			}
		}
		if len(et.ProdottiValutati) == 0 || len(et.ProdottiConfrontabili) != len(v.Prodotti.Candidati) || len(v.Prodotti.Candidati) == 0 {
			t.Fatalf("prodotti %d, confrontabili %d, candidati %d", len(et.ProdottiValutati), len(et.ProdottiConfrontabili), len(v.Prodotti.Candidati))
		}
		for i, c := range v.Prodotti.Candidati {
			p := et.ProdottiConfrontabili[i]
			if p.CodiceRichiesto != c.CodiceRichiesto || p.Base != c.Base.Normalizzata || p.QuantitaDaCella != (len(c.EvidenzaQuantita) > 0) {
				t.Errorf("prodotto confrontabile %d: %+v dal candidato %+v", i, p, c)
			}
		}
		for _, fi := range et.File {
			if fi.Motivo != "" {
				continue
			}
			if fi.Documento.BundleID == "" || fi.Interpretazione.BundleID != fi.Documento.BundleID {
				t.Errorf("file %s: documento %q, interpretazione %q", fi.AllegatoID, fi.Documento.BundleID, fi.Interpretazione.BundleID)
			}
			if a := ancoraggioDel(t, v, fi.AllegatoID); a.Disponibilita != fi.Disponibilita {
				t.Errorf("file %s: disponibilità %s, l'ancoraggio dice %s", fi.AllegatoID, fi.Disponibilita, a.Disponibilita)
			}
		}
		// le evidenze del nuovo: il cartiglio del 2D dello sciolto, con il testo dell'unità e l'intervallo della lettura
		trovata := false
		for _, ev := range et.Evidenze {
			if ev.AllegatoID == aDisegno && strings.HasPrefix(ev.UnitaID, "u:pdf:cartiglio") {
				trovata = true
				if ev.Testo[ev.Intervallo.Inizio:ev.Intervallo.Fine] != "7120200A1" || ev.LetturaID == "" {
					t.Errorf("evidenza %+v", ev)
				}
			}
		}
		if !trovata {
			t.Errorf("nessuna evidenza dal cartiglio del 2D fra %d", len(et.Evidenze))
		}
	})

	t.Run("i thread non valutati", func(t *testing.T) {
		for _, c := range []struct {
			thread uuid.UUID
			motivo valutazione.MotivoThread
			codice string
		}{
			{threadSenzaRegole, valutazione.MotivoThreadSenzaGrammatica, "regole.assenti"},
			{threadScartato, valutazione.MotivoThreadGrammaticaScartata, "regole.sha256_discorde"},
			{threadDiscorde, valutazione.MotivoThreadRagioneSocialeDiscorde, "regole.ragione_sociale_discorde"},
		} {
			et := esitoThread(t, e, c.thread)
			if et.Valutato || et.Motivo != c.motivo || et.HashSnapshot != "" || !contieneCodice(et.Diagnostiche, c.codice) {
				t.Errorf("thread %s: valutato %v, motivo %q, snapshot %q, diagnostiche %v", c.thread, et.Valutato, et.Motivo, et.HashSnapshot, codiciDi(et.Diagnostiche))
			}
			// T-B6-09: senza grammatica i prodotti si calcolano lo stesso, come informazione, senza la base letta
			if len(et.ProdottiValutati) != 1 || et.ProdottiValutati[0].Rif != "identificativo:7120100A2" || et.ProdottiValutati[0].Base.Normalizzata != "" {
				t.Errorf("thread %s: prodotti %+v", c.thread, et.ProdottiValutati)
			}
			if len(et.ProdottiConfrontabili) != 0 || len(et.Evidenze) != 0 {
				t.Errorf("thread %s: senza grammatica nessun candidato e nessuna evidenza", c.thread)
			}
		}
		et := esitoThread(t, e, threadSenzaRegole)
		c := confrontabileDi(t, et, uid(0x6c1))
		n := c.Nuovo
		if n.Valutato || n.Motivo != valutazione.MotivoFileThreadNonValutato || n.Associazione != string(ancoraggio.AssociazioneNonValutata) ||
			n.Collocazione != "" || len(n.Basi) != 0 || len(n.Candidati) != 0 || n.Revisioni != valutazione.RevisioniNonConfrontabili ||
			n.MotivoRevisioni != valutazione.MotivoRevisioniNuovoNonValutato {
			t.Errorf("nuovo di un thread non valutato: %+v", n)
		}
		if c.Vecchio.Codice != "7120100A" || c.Vecchio.Leggibile || c.Vecchio.MotivoLettura != valutazione.MotivoLetturaSenzaGrammatica ||
			contieneCodice(et.Diagnostiche, valutazione.CodiceCodiceRegistratoNonLeggibile) {
			t.Errorf("vecchio senza grammatica: %+v (la diagnostica del codice non leggibile solo con la grammatica)", c.Vecchio)
		}
		if fi := fileInterpretatoDi(t, et, uid(0x6c1)); fi.Motivo != "" || fi.Documento.BundleID == "" || len(fi.Interpretazione.Letture) != 0 {
			t.Errorf("file di un thread senza grammatica: motivo %q, documento %q, %d letture", fi.Motivo, fi.Documento.BundleID, len(fi.Interpretazione.Letture))
		}
	})
}

// TestL116UnAdattatoreInErroreNonFermaIlThread (A1c-L1-16; 6.4.6 passo 3; F0-03): un file i cui fatti non tornano con
// il loro payload (l'adattatore dà errore) resta fuori, con il suo record e il motivo; il thread resta valutato, e gli
// altri file pure.
func TestL116UnAdattatoreInErroreNonFermaIlThread(t *testing.T) {
	th := scenaCollegamento(t)
	rotto := uid(0x6d1)
	fr := fattiPDF(t, sha("7"), "7120200A1")
	fr.Digest = strings.Repeat("9", 64)
	conAllegato(&th, allegato(rotto, 9, "rotto-acme.pdf", "pdf", sha("7")), &fr)
	e := calcola(t, fotografiaDi(th), insiemeACME(t), valutazione.Ingressi{})
	et := esitoThread(t, e, threadACME)
	if !et.Valutato {
		t.Fatalf("il thread non è valutato: %q %v", et.Motivo, codiciDi(et.Diagnostiche))
	}
	if fi := fileInterpretatoDi(t, et, rotto); fi.Motivo != valutazione.MotivoFileDocumentoNonLeggibile || fi.Documento.BundleID != "" {
		t.Errorf("record del file rotto: %+v", fi)
	}
	if n := confrontabileDi(t, et, rotto).Nuovo; n.Valutato || n.Motivo != valutazione.MotivoFileDocumentoNonLeggibile || n.Associazione != string(ancoraggio.AssociazioneNonValutata) {
		t.Errorf("nuovo del file rotto: %+v", n)
	}
	if n := confrontabileDi(t, et, aDisegno).Nuovo; !n.Valutato || n.Motivo != "" {
		t.Errorf("gli altri file restano valutati: %+v", n)
	}
}

// TestIRecordPiattiDelNuovo (fase 0, CP.2, regole di popolamento): il nuovo piatto di ogni file ancorato, con le scelte
// fatte qui una volta sola. Le basi delle letture d'identità; i candidati nell'ordine e con il target, il livello e
// l'autorità di ancoraggio; la base del target per il livello prodotto, del codice del componente deciso, della forma del
// nodo senza decisione; le radici come basi dei target; la collocazione, l'associazione, la disponibilità, la revisione.
func TestIRecordPiattiDelNuovo(t *testing.T) {
	th := scenaCollegamento(t)
	nome := uid(0x6c3)
	conAllegato(&th, allegato(nome, 16, "7120400A_1.pdf", "pdf", sha("e")), nil)
	e := calcola(t, fotografiaDi(th), insiemeACME(t), valutazione.Ingressi{})
	et := esitoThread(t, e, threadACME)
	for _, c := range []struct {
		allegato     uuid.UUID
		basi         []string
		candidato    valutazione.CandidatoPiatto
		collocazione ancoraggio.Collocazione
		revisione    string
	}{
		{aStepB, []string{"7120100"}, valutazione.CandidatoPiatto{Target: rifProdB, Livello: ancoraggio.LivelloProdotto, Base: "7120100", Radici: []string{"7120100"}},
			ancoraggio.CollocazioneRadice, "1"},
		{aDisegno, []string{"7120200"}, valutazione.CandidatoPiatto{Target: "componente:" + cSciolto.String(), Livello: ancoraggio.LivelloComponente, Base: "7120200",
			Radici: []string{"7120100"}}, ancoraggio.CollocazioneFiglio, "1"},
		{nome, []string{"7120400"}, valutazione.CandidatoPiatto{Target: nodoB("#4"), Livello: ancoraggio.LivelloComponente, Base: "7120400",
			Radici: []string{"7120100"}}, ancoraggio.CollocazioneFiglio, "1"},
	} {
		n := confrontabileDi(t, et, c.allegato).Nuovo
		var af ancoraggio.AncoraggioFile
		for _, a := range et.Ancoraggi.File {
			if a.AllegatoID == c.allegato {
				af = a
			}
		}
		if len(af.Candidati) != 1 || len(n.Candidati) != 1 {
			t.Fatalf("allegato %s: candidati %d nel piatto, %d in ancoraggio", c.allegato, len(n.Candidati), len(af.Candidati))
		}
		c.candidato.Autorita = string(af.Candidati[0].Autorita)
		if !n.Valutato || !reflect.DeepEqual(n.Basi, c.basi) || !reflect.DeepEqual(n.Candidati[0], c.candidato) || n.Collocazione != string(c.collocazione) ||
			n.Associazione != string(af.Associazione) || n.Disponibilita != string(af.Disponibilita) || n.Revisione != c.revisione {
			t.Errorf("allegato %s:\nnuovo %+v\natteso basi %v, candidato %+v, %s, revisione %q", c.allegato, n, c.basi, c.candidato, c.collocazione, c.revisione)
		}
	}
}

// TestL116GliErroriDellaValutazione (T-B6-09, F0-05): un errore della valutazione rende il thread non valutato, senza
// fermare gli altri. Un errore senza diagnostiche di contratto dà valutazione.errore_valutazione; un errore di contratto
// (un segmento del caso che il documento non ha) porta le sue diagnostiche, e il codice nuovo non c'è. Senza motore vale il
// motivo della grammatica, e l'errore va fra le diagnostiche. I record dei file e il vecchio ci sono lo stesso.
func TestL116GliErroriDellaValutazione(t *testing.T) {
	r := insiemeACME(t)
	f := fotografiaDi(quattroThread(t)...)
	thACME, thTre := threadACME, threadSenzaRegole
	n := len(scenaCollegamento(t).Allegati)

	t.Run("il caso di un altro cliente", func(t *testing.T) {
		in := valutazione.Ingressi{Versione: valutazione.VersioneCasi, Casi: []valutazione.IngressoCaso{
			{ID: "caso-acme", ThreadID: &thACME, ClienteID: altroCliente},
			{ID: "caso-tre", ThreadID: &thTre, ClienteID: altroCliente},
		}}
		e := calcola(t, f, r, in)
		et := esitoThread(t, e, threadACME)
		if et.Valutato || et.Motivo != valutazione.MotivoThreadErroreValutazione || !contieneCodice(et.Diagnostiche, valutazione.CodiceErroreValutazione) {
			t.Fatalf("thread ACME: valutato %v, motivo %q, diagnostiche %v", et.Valutato, et.Motivo, codiciDi(et.Diagnostiche))
		}
		if len(et.ProdottiValutati) != 0 || len(et.Ancoraggi.File) != 0 || len(et.Prodotti.Candidati) != 0 || et.Richiesta.Stato != "" {
			t.Errorf("con un errore niente del nuovo: %d prodotti, %d ancoraggi", len(et.ProdottiValutati), len(et.Ancoraggi.File))
		}
		if len(et.File) != n || len(et.Confrontabili) != n {
			t.Errorf("record dei file %d e piatti %d, attesi %d", len(et.File), len(et.Confrontabili), n)
		}
		if c := confrontabileDi(t, et, aDisegno); c.Nuovo.Motivo != valutazione.MotivoFileThreadNonValutato || c.Vecchio.Documento == nil || *c.Vecchio.Documento != dDisegno {
			t.Errorf("record piatto del 2D: %+v", c)
		}
		tre := esitoThread(t, e, threadSenzaRegole)
		if tre.Motivo != valutazione.MotivoThreadSenzaGrammatica || !contieneCodice(tre.Diagnostiche, valutazione.CodiceErroreValutazione) ||
			!contieneCodice(tre.Diagnostiche, "regole.assenti") {
			t.Errorf("senza motore e con un errore: motivo %q, diagnostiche %v", tre.Motivo, codiciDi(tre.Diagnostiche))
		}
		if d := esitoThread(t, e, threadDiscorde); d.Motivo != valutazione.MotivoThreadRagioneSocialeDiscorde {
			t.Errorf("gli altri thread proseguono: %q", d.Motivo)
		}
	})
	t.Run("un errore di contratto porta le sue diagnostiche", func(t *testing.T) {
		in := valutazione.Ingressi{Versione: valutazione.VersioneCasi, Casi: []valutazione.IngressoCaso{{ID: "caso-acme", ThreadID: &thACME,
			ClienteID: clienteACME, Segmenti: []valutazione.SegmentoDichiarato{{MessaggioID: idM1, SegmentoID: "s:storia:9", Uso: "pertinente", Origine: "scenario"}}}}}
		e := calcola(t, f, r, in)
		et := esitoThread(t, e, threadACME)
		if et.Valutato || et.Motivo != valutazione.MotivoThreadErroreValutazione || contieneCodice(et.Diagnostiche, valutazione.CodiceErroreValutazione) ||
			!contieneCodice(et.Diagnostiche, evidenze.CodiceDocumentoUsoNonValido) {
			t.Errorf("thread ACME: valutato %v, motivo %q, diagnostiche %v", et.Valutato, et.Motivo, codiciDi(et.Diagnostiche))
		}
	})
}

// TestL116SenzaInsiemeDiRegole (6.4.6 passo 2; fase 0, F.2): senza insieme di regole nessun thread è valutato, con
// senza_grammatica_a e la nota regole.assenti; l'impronta dell'indice e la versione dei limiti restano vuote.
func TestL116SenzaInsiemeDiRegole(t *testing.T) {
	e := calcola(t, fotografiaDi(quattroThread(t)...), nil, valutazione.Ingressi{})
	if e.ImprontaIndice != "" || e.VersioneLimiti != "" {
		t.Errorf("indice %q, limiti %q", e.ImprontaIndice, e.VersioneLimiti)
	}
	for _, et := range e.Thread {
		if et.Valutato || et.Motivo != valutazione.MotivoThreadSenzaGrammatica || !contieneCodice(et.Diagnostiche, "regole.assenti") {
			t.Errorf("thread %s: valutato %v, motivo %q", et.ThreadID, et.Valutato, et.Motivo)
		}
	}
}

// TestL116LaFotografiaNonValida (6.4.6 passo 1): con una diagnostica di contratto di ValidaFotografia, Calcola dà
// l'errore di contratto e un esito vuoto.
func TestL116LaFotografiaNonValida(t *testing.T) {
	th := scenaCollegamento(t)
	a := allegato(uid(0x6e1), 20, "orfano-acme.pdf", "pdf", sha("6"))
	a.MessaggioID = uid(0x6e2)
	conAllegato(&th, a, nil)
	e, err := valutazione.Calcola(fotografiaDi(th), insiemeACME(t), valutazione.Ingressi{})
	var ec *evidenze.ErroreContratto
	if !errors.As(err, &ec) || !contieneCodice(ec.Diagnostiche, fotorfq.CodiceRiferimentoNonRisolto) || !reflect.DeepEqual(e, valutazione.Esito{}) {
		t.Errorf("errore %v, esito %+v", err, e.Impronta)
	}
}

// TestL116IMessaggiFuoriRFQ (R34; 6.4.6, «FuoriRFQ»): i messaggi senza thread dei casi di censimento, valutati con la
// grammatica del cliente del caso, con il documento, l'interpretazione, i file e i prodotti; un messaggio del caso che la
// fotografia non ha è un record non valutato; un cliente senza grammatica dà il suo motivo; un messaggio fuori RFQ che
// nessun caso elenca lo dice un avviso fra le diagnostiche dell'esito. In ordine di (caso, messaggio).
func TestL116IMessaggiFuoriRFQ(t *testing.T) {
	mUno, mDue, mTre, mManca := uid(0x6f1), uid(0x6f2), uid(0x6f3), uid(0x6f4)
	fs := fattiPDF(t, sha("5"), "7120100A2")
	f := fotografiaDi(scenaCollegamento(t))
	f.FuoriRFQ = []fotorfq.MessaggioFuoriRFQ{
		messaggioFuori(mUno, clienteACME, "Buongiorno,\r\nvi ordiniamo 7120100A2.\r\nGrazie", []fotorfq.Allegato{allegato(uid(0x6f5), 1, "ordine-acme.pdf", "pdf", sha("5"))}, fs),
		messaggioFuori(mDue, clienteACME, "Buongiorno,\r\nun messaggio che nessun caso elenca.", nil),
		messaggioFuori(mTre, clienteSenzaRegole, "Buongiorno,\r\nvi ordiniamo 7120100A2.", nil),
	}
	in := valutazione.Ingressi{Versione: valutazione.VersioneCasi, Casi: []valutazione.IngressoCaso{
		{ID: "censimento-tre", ClienteID: clienteSenzaRegole, Messaggi: []uuid.UUID{mTre}},
		{ID: "censimento-acme", ClienteID: clienteACME, Messaggi: []uuid.UUID{mManca, mUno, mUno},
			Segmenti: []valutazione.SegmentoDichiarato{{MessaggioID: mUno, SegmentoID: "s:corrente", Uso: "pertinente", Origine: "scenario"}}},
	}}
	e := calcola(t, f, insiemeACME(t), in)
	if len(e.FuoriRFQ) != 3 {
		t.Fatalf("fuori RFQ %d: %+v", len(e.FuoriRFQ), e.FuoriRFQ)
	}
	ordine := []struct {
		caso string
		id   uuid.UUID
	}{{"censimento-acme", mUno}, {"censimento-acme", mManca}, {"censimento-tre", mTre}}
	if mManca.String() < mUno.String() {
		ordine[0], ordine[1] = ordine[1], ordine[0]
	}
	for i, o := range ordine {
		if e.FuoriRFQ[i].Caso != o.caso || e.FuoriRFQ[i].MessaggioID != o.id {
			t.Errorf("fuori RFQ %d: %s %s, atteso %s %s", i, e.FuoriRFQ[i].Caso, e.FuoriRFQ[i].MessaggioID, o.caso, o.id)
		}
	}
	for _, ef := range e.FuoriRFQ {
		switch ef.MessaggioID {
		case mUno:
			if !ef.Valutato || ef.Motivo != "" || ef.HashSnapshot == "" || ef.ClienteID != clienteACME || ef.Messaggio.MessaggioID != mUno ||
				ef.Messaggio.Documento.BundleID == "" || len(ef.Messaggio.Interpretazione.Letture) == 0 {
				t.Errorf("messaggio valutato: %+v", ef)
			}
			if len(ef.Prodotti.Candidati) != 1 || ef.Prodotti.Candidati[0].Base.Normalizzata != "7120100" {
				t.Errorf("prodotti del messaggio: %+v", ef.Prodotti.Candidati)
			}
			// l'interpretazione usa i segmenti del caso: la parte corrente è pertinente, con l'origine «scenario»
			pertinenti := 0
			for _, l := range ef.Messaggio.Interpretazione.Letture {
				if l.Uso == "pertinente" && l.OrigineUso == "scenario" {
					pertinenti++
				}
			}
			if pertinenti == 0 {
				t.Errorf("nessuna lettura con l'uso del caso: %+v", ef.Messaggio.Interpretazione.Letture)
			}
			if len(ef.File) != 1 || ef.File[0].Motivo != "" || len(ef.File[0].Interpretazione.Letture) == 0 {
				t.Errorf("file del messaggio: %+v", ef.File)
			}
		case mManca:
			if ef.Valutato || ef.Motivo != valutazione.MotivoThreadErroreValutazione || !contieneCodice(ef.Diagnostiche, valutazione.CodiceErroreValutazione) {
				t.Errorf("messaggio che la fotografia non ha: %+v", ef)
			}
		case mTre:
			if ef.Valutato || ef.Motivo != valutazione.MotivoThreadSenzaGrammatica || len(ef.Prodotti.Candidati) != 0 || ef.Messaggio.Documento.BundleID == "" {
				t.Errorf("messaggio senza grammatica: %+v", ef)
			}
		}
	}
	// R-63 della revisione di V1: il messaggio che nessun caso elenca ha il suo codice, un avviso; errore_valutazione resta
	// per l'errore vero (il messaggio del caso che manca è nel suo record).
	avvisi := conCodice(e.Diagnostiche, valutazione.CodiceMessaggioFuoriRFQSenzaCaso)
	if len(avvisi) != 1 || avvisi[0].Gravita != evidenze.GravitaAvviso || !reflect.DeepEqual(avvisi[0].Rif, []string{mDue.String()}) ||
		contieneCodice(e.Diagnostiche, valutazione.CodiceErroreValutazione) {
		t.Errorf("il messaggio che nessun caso elenca: %+v", e.Diagnostiche)
	}
}

// TestL116IlDeterminismo (A1c-L1-16; IM.1): due esecuzioni, e la stessa fotografia con ogni elenco permutato (i thread,
// i messaggi, gli allegati, le decisioni, i messaggi fuori RFQ, i casi), danno gli stessi byte canonici e la stessa
// Esito.Impronta; la fotografia di chi chiama non cambia; l'impronta copre l'indice delle regole.
func TestL116IlDeterminismo(t *testing.T) {
	r := insiemeACME(t)
	f := fotografiaDi(quattroThread(t)...)
	mUno := uid(0x6f1)
	f.FuoriRFQ = []fotorfq.MessaggioFuoriRFQ{messaggioFuori(mUno, clienteACME, "Buongiorno,\r\nvi ordiniamo 7120100A2.", nil)}
	in := valutazione.Ingressi{Versione: valutazione.VersioneCasi, Casi: []valutazione.IngressoCaso{
		{ID: "censimento-acme", ClienteID: clienteACME, Messaggi: []uuid.UUID{mUno}},
		{ID: "caso-tre", ThreadID: &threadSenzaRegole, ClienteID: clienteSenzaRegole},
	}}
	prima := canonicoDi(t, f)
	e1 := calcola(t, f, r, in)
	e2 := calcola(t, f, r, in)
	if e1.Impronta != e2.Impronta || canonicoDi(t, e1) != canonicoDi(t, e2) {
		t.Fatal("due esecuzioni danno esiti diversi")
	}
	if canonicoDi(t, f) != prima {
		t.Error("Calcola ha cambiato la fotografia di chi chiama")
	}
	fp, inp := permuta(f, in)
	if canonicoDi(t, fp) == prima {
		t.Fatal("la permutazione non cambia niente: la prova non prova")
	}
	e3 := calcola(t, fp, r, inp)
	if e3.Impronta != e1.Impronta || canonicoDi(t, e3) != canonicoDi(t, e1) {
		t.Errorf("con gli ingressi permutati l'impronta cambia: %s e %s", e1.Impronta, e3.Impronta)
	}
	altro := *r
	altro.ImprontaIndice = strings.Repeat("1", 64)
	if e4 := calcola(t, f, &altro, in); e4.Impronta == e1.Impronta {
		t.Error("l'impronta non copre l'impronta dell'indice delle regole (R43 B)")
	}
}

// TestIConflittiComposti (T-B6-10, F0-13; dubbio T-B6-23): i pezzi di B5 entrano nell'esito con il prodotto, senza
// doppioni; sulla regola, lo stesso conflitto di due prodotti resta due voci, due rimozioni dello stesso arco da due
// documenti restano due, un doppione esatto se ne va, e l'ordine non dipende da quello dei pezzi.
func TestIConflittiComposti(t *testing.T) {
	t.Run("i pezzi di B5 nell'esito", func(t *testing.T) {
		th := scenaAlbero(t)
		th.Relazioni[0].Qta = 3
		f := fotografiaDi(th)
		r := insiemeACME(t)
		e := calcola(t, f, r, valutazione.Ingressi{})
		v, err := valutazione.ValutaProdotti(f, th, r.Motori[clienteACME], nil)
		if err != nil {
			t.Fatal(err)
		}
		et := esitoThread(t, e, threadACME)
		if len(et.Conflitti) != 1 || canonicoDi(t, et.Conflitti) != canonicoDi(t, v.Conflitti) || et.Conflitti[0].Prodotto != rifProdB {
			t.Errorf("conflitti dell'esito %+v, pezzi %+v", et.Conflitti, v.Conflitti)
		}
	})
	t.Run("la regola", func(t *testing.T) {
		d1, d2, a := uid(0x6a8), uid(0x6a9), uid(0x6aa)
		arco := valutazione.Conflitto{Tipo: valutazione.ConflittoArco, Asse: valutazione.AsseGerarchia, Rif: "arco:componente:a>componente:b",
			Prodotto: "componente:p1", Decisione: "qta 1", OrigineDecisione: ancoraggio.OrigineConfermato, Proposta: "rimozione", Motivo: valutazione.MotivoConflittoRimozione,
			EvidenzaProposta: valutazione.EvidenzaProposta{DocumentoID: &d1}}
		altroDoc := arco
		altroDoc.EvidenzaProposta = valutazione.EvidenzaProposta{DocumentoID: &d2}
		altroProdotto := arco
		altroProdotto.Prodotto = "componente:p2"
		codice := valutazione.Conflitto{Tipo: valutazione.ConflittoCodice, Asse: valutazione.AsseNomenclatura, Rif: "componente:c", Prodotto: "componente:p1",
			Decisione: "7120200A", OrigineDecisione: ancoraggio.OrigineConfermato, Proposta: "7120200B1", Motivo: "codice",
			EvidenzaProposta: valutazione.EvidenzaProposta{AllegatoID: &a, Riferimenti: []string{"nodo:x"}}}
		doppione := codice
		doppione.EvidenzaProposta.Riferimenti = []string{"nodo:y"}
		pezzi1 := []valutazione.Conflitto{arco, altroDoc, codice}
		pezzi2 := []valutazione.Conflitto{altroProdotto, doppione}
		out := valutazione.ComponiConflittiPerProva(pezzi1, pezzi2)
		if len(out) != 4 {
			t.Fatalf("composti %d: %+v", len(out), out)
		}
		for _, c := range out {
			if c.Tipo == valutazione.ConflittoCodice && !reflect.DeepEqual(c.EvidenzaProposta.Riferimenti, []string{"nodo:x"}) {
				t.Errorf("dei doppioni resta il primo, nell'ordine dei pezzi: %+v", c.EvidenzaProposta.Riferimenti)
			}
		}
		rov := valutazione.ComponiConflittiPerProva(rovescia(pezzi2), rovescia(pezzi1))
		chiave := func(c []valutazione.Conflitto) string {
			var s []string
			for _, x := range c {
				s = append(s, string(x.Tipo)+"|"+x.Prodotto+"|"+x.Rif+"|"+val(nilSeVuoto(x.EvidenzaProposta.DocumentoID)))
			}
			return strings.Join(s, ";")
		}
		if chiave(out) != chiave(rov) {
			t.Errorf("l'ordine dipende dai pezzi:\n%s\n%s", chiave(out), chiave(rov))
		}
		prodotti := map[string]int{}
		for _, c := range out {
			if c.Tipo == valutazione.ConflittoArco {
				prodotti[c.Prodotto]++
			}
		}
		if prodotti["componente:p1"] != 2 || prodotti["componente:p2"] != 1 {
			t.Errorf("conflitti d'arco per prodotto %v: due documenti restano due, un altro prodotto è una voce in più", prodotti)
		}
	})
}

// nilSeVuoto: il testo di un UUID facoltativo, come puntatore a stringa, per i confronti.
func nilSeVuoto(p *uuid.UUID) *string {
	if p == nil {
		return nil
	}
	s := p.String()
	return &s
}

// TestL114LaDisponibilitaEIFileNonValutati (A1c-L1-14 riscritta; 6.4.6 passo 8; T-B0-31, T-B6-02; F0-03): ogni
// disponibilità del contratto, e i file che l'adattatore lascia fuori, nei record dell'esito.
//
// Deviazione dal piano, nominata (T-B6-02): il 6.4.6, passo 8, e A1c-L1-14 davano illeggibile alla scansione (un PDF senza
// testo e senza OCR); vale il contratto (T-B0-31, LD-05): senza_testo, e illeggibile resta per un PDF che non si apre.
//   - mancante, pendente, illeggibile, errore, senza_testo, parziale: la disponibilità del file, un asse distinto
//     dall'associazione; un file assente o che non si legge non è mai «fuori richiesta» (nessuna assenza verificata);
//   - un archivio non estraibile resta un file, di cui si legge solo il nome (parziale, con la lettura del nome);
//   - un contenitore e una natura che non è un file: non_valutata, con il loro record e il motivo; la voce
//     dell'archivio è un file a sé.
func TestL114LaDisponibilitaEIFileNonValutati(t *testing.T) {
	th := scenaCollegamento(t)
	rotto, stepRotto, archivio, zip, voce, inline := uid(0x619), uid(0x61a), uid(0x61b), uid(0x61c), uid(0x61d), uid(0x61e)
	fr := fattiPDFErrore(t, sha("8"))
	conAllegato(&th, allegato(rotto, 9, "rotto-acme.pdf", "pdf", sha("8")), &fr)
	fs := fattiSTEPIllegibili(t, sha("9"))
	conAllegato(&th, allegato(stepRotto, 10, "rotto-acme.stp", "stp", sha("9")), &fs)
	conAllegato(&th, allegato(archivio, 11, "7120500A_1.zip", "zip", sha("0")), nil)
	conAllegato(&th, allegato(zip, 12, "pacco-acme.zip", "zip", sha("f")), nil)
	v := allegato(voce, 1, "voce-acme.pdf", "pdf", sha("1"))
	v.ContenitoreID = &zip
	conAllegato(&th, v, nil)
	in := allegato(inline, 13, "immagine-acme.png", "png", sha("2"))
	in.Natura = "inline"
	conAllegato(&th, in, nil)

	e := calcola(t, fotografiaDi(th), insiemeACME(t), valutazione.Ingressi{})
	et := esitoThread(t, e, threadACME)
	for _, c := range []struct {
		allegato uuid.UUID
		atteso   ancoraggio.Disponibilita
	}{
		{aVuoto, ancoraggio.DisponibilitaMancante}, {aInAttesa, ancoraggio.DisponibilitaPendente}, {rotto, ancoraggio.DisponibilitaIlleggibile},
		{stepRotto, ancoraggio.DisponibilitaErrore}, {aScansione, ancoraggio.DisponibilitaSenzaTesto}, {archivio, ancoraggio.DisponibilitaParziale},
	} {
		n := confrontabileDi(t, et, c.allegato).Nuovo
		if !n.Valutato || n.Disponibilita != string(c.atteso) || fileInterpretatoDi(t, et, c.allegato).Disponibilita != c.atteso {
			t.Errorf("allegato %s: valutato %v, disponibilità %q, attesa %q", c.allegato, n.Valutato, n.Disponibilita, c.atteso)
		}
		if n.Associazione == string(ancoraggio.AssociazioneNonValutata) || n.Associazione == "" {
			t.Errorf("allegato %s: l'associazione %q è un asse a sé, valutato", c.allegato, n.Associazione)
		}
		if c.atteso != ancoraggio.DisponibilitaSenzaTesto && c.atteso != ancoraggio.DisponibilitaParziale && n.Collocazione == string(ancoraggio.CollocazioneFuoriRichiesta) {
			t.Errorf("allegato %s (%s): un file che non si legge non è fuori richiesta", c.allegato, c.atteso)
		}
	}
	if n := confrontabileDi(t, et, archivio).Nuovo; !reflect.DeepEqual(n.Basi, []string{"7120500"}) {
		t.Errorf("archivio non estraibile: basi %v (la lettura del nome)", n.Basi)
	}
	for _, c := range []struct {
		allegato uuid.UUID
		motivo   string
	}{{zip, valutazione.MotivoFileContenitore}, {inline, valutazione.MotivoFileNaturaNonFile}} {
		n := confrontabileDi(t, et, c.allegato).Nuovo
		if n.Valutato || n.Motivo != c.motivo || n.Associazione != string(ancoraggio.AssociazioneNonValutata) || n.Disponibilita != "" {
			t.Errorf("allegato %s: nuovo %+v", c.allegato, n)
		}
		if fi := fileInterpretatoDi(t, et, c.allegato); fi.Motivo != c.motivo || fi.Documento.BundleID != "" || fi.Disponibilita != "" {
			t.Errorf("allegato %s: record %+v", c.allegato, fi.Motivo)
		}
	}
	if n := confrontabileDi(t, et, voce).Nuovo; !n.Valutato || n.Motivo != "" {
		t.Errorf("la voce dell'archivio è un file a sé: %+v", n)
	}
}
