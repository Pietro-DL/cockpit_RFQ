// L1 — i prodotti target della RFQ e la loro identità (contratto di A1c, §1.0 riga 1, §1.1; R60 A, R70 A, R75 A, R78;
// T-B0-07, T-B0-21, T-B0-23; T-E1-02, T-E1-24; PO-19 nella parte di B1): il gesto 2 in tutte le sue forme, lo
// scenario, i candidati della mail che non sono mai target, la diagnosi della possibile rinomina; il determinismo
// dell'esito con gli elenchi della fotografia permutati.
package valutazione_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// valuta chiama ValutaProdotti e controlla gli invarianti di ogni esito: i target sono confermati con l'identità
// confermata o di scenario con l'identità da confermare, mai altro; TargetConfermato vale solo per i primi; i
// candidati della mail sono tutti proposti e da confermare (R60 A); le diagnostiche sono in ordine.
func valuta(t *testing.T, th fotorfq.Thread, m *motorea.Motore, caso *valutazione.IngressoCaso) valutazione.ValutazioneProdotti {
	t.Helper()
	v, err := valutazione.ValutaProdotti(fotografia(th), th, m, caso)
	if err != nil {
		t.Fatalf("ValutaProdotti: %v", err)
	}
	for _, p := range v.ProdottiValutati {
		switch {
		case p.Autorita == ancoraggio.AutoritaConfermata && p.Identita == valutazione.IdentitaConfermata:
			if !valutazione.TargetConfermato(p) {
				t.Errorf("%s: confermato ma non TargetConfermato", p.Rif)
			}
		case p.Autorita == ancoraggio.AutoritaScenario && p.Identita == valutazione.IdentitaDaConfermare:
			if valutazione.TargetConfermato(p) || p.ComponenteID != nil || !strings.HasPrefix(p.Rif, "scenario:") {
				t.Errorf("%s: un prodotto dello scenario è confermato o ha un componente (R75 A)", p.Rif)
			}
		default:
			t.Errorf("%s: autorità %q e identità %q fuori dalle due forme di R60 A", p.Rif, p.Autorita, p.Identita)
		}
	}
	for _, c := range v.Prodotti.Candidati {
		if c.Origine != ancoraggio.OrigineProposto {
			t.Errorf("candidato %s: origine %q", c.CodiceRichiesto, c.Origine)
		}
	}
	for i := 1; i < len(v.Diagnostiche); i++ {
		a, b := v.Diagnostiche[i-1], v.Diagnostiche[i]
		if a.Codice > b.Codice {
			t.Errorf("diagnostiche fuori ordine: %s prima di %s", a.Codice, b.Codice)
		}
	}
	return v
}

func rifDi(v valutazione.ValutazioneProdotti) []string {
	var out []string
	for _, p := range v.ProdottiValutati {
		out = append(out, p.Rif)
	}
	return out
}

func prodotto(t *testing.T, v valutazione.ValutazioneProdotti, rif string) valutazione.ProdottoValutato {
	t.Helper()
	for _, p := range v.ProdottiValutati {
		if p.Rif == rif {
			return p
		}
	}
	t.Fatalf("il prodotto %s non c'è fra %v", rif, rifDi(v))
	return valutazione.ProdottoValutato{}
}

func conCodice(d []evidenze.Diagnostica, codice string) []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	for _, x := range d {
		if x.Codice == codice {
			out = append(out, x)
		}
	}
	return out
}

func componente(id uuid.UUID, codice, tipo, origine string) fotorfq.Componente {
	return fotorfq.Componente{ID: id, Codice: codice, Tipo: tipo, Origine: origine, ConfermatoDa: operatore, CreatoIl: dataACME}
}

func confermato(codice string) fotorfq.Identificativo {
	op := operatore
	return fotorfq.Identificativo{Codice: codice, Origine: "proposta_famiglia", ConfermatoDa: &op, CreatoIl: dataACME}
}

func proposto(codice string) fotorfq.Identificativo {
	return fotorfq.Identificativo{Codice: codice, Origine: "proposta_corpo", CreatoIl: dataACME}
}

// TestPO19IlCodiceAccettatoAllaCreazioneEIlTarget (PO-19, parte B1; R70 A, R78; T-E1-02): la mail chiede tre codici;
// alla creazione della RFQ l'operatore ne conferma uno (identificativo con confermato_da). Quello è il target, con
// l'identità confermata e la base letta; gli altri due restano candidati della mail, da confermare, e non sono
// target. Un identificativo non confermato non è target.
func TestPO19IlCodiceAccettatoAllaCreazioneEIlTarget(t *testing.T) {
	m := motoreACME(t)
	th := threadBase("Buongiorno,\r\nvi chiediamo l'offerta per P7120100, P7120200 e P7120300.\r\nGrazie")
	th.Identificativi = []fotorfq.Identificativo{confermato("P7120100"), proposto("P7120200")}
	v := valuta(t, th, m, nil)

	if got := strings.Join(rifDi(v), ","); got != "identificativo:P7120100" {
		t.Fatalf("target %s: atteso il solo codice confermato", got)
	}
	p := v.ProdottiValutati[0]
	if p.CodiceRichiesto != "P7120100" || p.Base.Normalizzata != "7120100" || p.ComponenteID != nil || !valutazione.TargetConfermato(p) {
		t.Errorf("target %+v", p)
	}
	var candidati []string
	for _, c := range v.Prodotti.Candidati {
		candidati = append(candidati, c.CodiceRichiesto)
		if c.Motivo != ancoraggio.MotivoCandidatoDaConfermare {
			t.Errorf("candidato %s: motivo %q, atteso da confermare", c.CodiceRichiesto, c.Motivo)
		}
	}
	if strings.Join(candidati, ",") != "P7120100,P7120200,P7120300" {
		t.Errorf("candidati %v: i tre codici della mail restano candidati, anche quello confermato", candidati)
	}
	if v.Richiesta.Stato != ancoraggio.StatoRichiestaValutata {
		t.Errorf("richiesta %q, attesa valutata (gesto 1)", v.Richiesta.Stato)
	}
}

// TestR70LeFormeDelTargetConfermato (R70 A; T-B0-07, T-B0-21; emendamento E1 §4.2): l'identificativo confermato con
// il suo componente (Rif componente:<uuid>, legato per codice senza maiuscole), senza componente (identificativo:
// <codice>, fonte dai soli candidati), con il componente archiviato (fuori); il finito attivo di origine «manuale»;
// un finito di origine «codice_rilevato» o «step», con confermato_da pieno, non è target; un finito manuale
// archiviato nemmeno; un identificativo e il suo finito manuale non si ripetono.
func TestR70LeFormeDelTargetConfermato(t *testing.T) {
	m := motoreACME(t)
	th := threadBase("Buongiorno,\r\nRichiesta di offerta.")
	archiviato := dataACME
	cConIdent, cArchiviato, cManuale, cRilevato, cStep, cManualeArch := uid(0x101), uid(0x102), uid(0x103), uid(0x104), uid(0x105), uid(0x106)
	th.Componenti = []fotorfq.Componente{
		componente(cConIdent, "7120100", "finito", "codice_rilevato"),
		func() fotorfq.Componente {
			c := componente(cArchiviato, "7120200", "finito", "codice_rilevato")
			c.ArchiviatoIl = &archiviato
			return c
		}(),
		componente(cManuale, "7120300", "finito", "manuale"),
		componente(cRilevato, "7120400", "finito", "codice_rilevato"),
		componente(cStep, "7120500", "finito", "step"),
		func() fotorfq.Componente {
			c := componente(cManualeArch, "7120600", "finito", "manuale")
			c.ArchiviatoIl = &archiviato
			return c
		}(),
	}
	th.Identificativi = []fotorfq.Identificativo{confermato("7120100"), confermato("7120200"), confermato("7120700"), confermato("7120300"), proposto("7120400")}
	v := valuta(t, th, m, nil)

	want := []string{"componente:" + cConIdent.String(), "componente:" + cManuale.String(), "identificativo:7120700"}
	if !reflect.DeepEqual(rifDi(v), want) {
		t.Fatalf("target %v, attesi %v", rifDi(v), want)
	}
	if p := prodotto(t, v, "componente:"+cConIdent.String()); p.ComponenteID == nil || *p.ComponenteID != cConIdent || p.Base.Normalizzata != "7120100" {
		t.Errorf("identificativo con il componente: %+v", p)
	}
	if p := prodotto(t, v, "identificativo:7120700"); p.ComponenteID != nil || p.Fonte.Stato != valutazione.FonteAssente ||
		p.Fonte.Motivo != valutazione.MotivoFonteProdottoSenzaComponente {
		t.Errorf("identificativo senza componente: %+v (T-B0-07)", p)
	}
	// T-E1-24: l'identificativo senza componente e i due finiti attivi che non sono target.
	d := conCodice(v.Diagnostiche, valutazione.CodiceTargetPossibileRinomina)
	if len(d) != 1 || d[0].Rif[0] != "identificativo:7120700" || len(d[0].Rif) != 3 || d[0].Gravita != evidenze.GravitaAvviso {
		t.Errorf("diagnostiche %+v: attesa target.possibile_rinomina con i finiti non target", v.Diagnostiche)
	}

	t.Run("il legame per codice è senza maiuscole e senza spazi ai bordi", func(t *testing.T) {
		th := threadBase("Richiesta.")
		th.Componenti = []fotorfq.Componente{componente(cConIdent, "acme7120100", "finito", "codice_rilevato")}
		th.Identificativi = []fotorfq.Identificativo{confermato(" ACME7120100 ")}
		v := valuta(t, th, m, nil)
		if got := strings.Join(rifDi(v), ","); got != "componente:"+cConIdent.String() {
			t.Errorf("target %s", got)
		}
	})

	t.Run("due identificativi senza componente con la stessa chiave del codice: un target solo", func(t *testing.T) {
		th := threadBase("Richiesta.")
		th.Identificativi = []fotorfq.Identificativo{confermato("p7120700 "), confermato("P7120700"), confermato("7120800")}
		v := valuta(t, th, m, nil)
		want := []string{"identificativo:7120800", "identificativo:P7120700"}
		if !reflect.DeepEqual(rifDi(v), want) {
			t.Errorf("target %v, attesi %v (la chiave del codice, come in ProdottiDellaRfq)", rifDi(v), want)
		}
		p := th
		p.Identificativi = []fotorfq.Identificativo{th.Identificativi[2], th.Identificativi[1], th.Identificativi[0]}
		if got := rifDi(valuta(t, p, m, nil)); !reflect.DeepEqual(got, want) {
			t.Errorf("con gli identificativi permutati: %v", got)
		}
	})

	t.Run("senza finiti non target nessuna rinomina", func(t *testing.T) {
		th := threadBase("Richiesta.")
		th.Identificativi = []fotorfq.Identificativo{confermato("7120700")}
		if v := valuta(t, th, m, nil); len(conCodice(v.Diagnostiche, valutazione.CodiceTargetPossibileRinomina)) != 0 {
			t.Errorf("diagnostiche %+v", v.Diagnostiche)
		}
	})

	t.Run("un codice che la grammatica non legge: la base resta vuota, con la diagnosi", func(t *testing.T) {
		th := threadBase("Richiesta.")
		th.Identificativi = []fotorfq.Identificativo{confermato("ACME-SENZA-FORMA")}
		v := valuta(t, th, m, nil)
		p := prodotto(t, v, "identificativo:ACME-SENZA-FORMA")
		if p.Base.Normalizzata != "" || len(conCodice(v.Diagnostiche, valutazione.CodiceTargetNonLeggibile)) != 1 {
			t.Errorf("prodotto %+v, diagnostiche %+v", p, v.Diagnostiche)
		}
	})

	t.Run("senza grammatica i target restano, senza base e senza candidati", func(t *testing.T) {
		v := valuta(t, th, nil, nil)
		if !reflect.DeepEqual(rifDi(v), want) || len(v.Prodotti.Candidati) != 0 {
			t.Errorf("target %v, candidati %d", rifDi(v), len(v.Prodotti.Candidati))
		}
		for _, p := range v.ProdottiValutati {
			if p.Base.Normalizzata != "" {
				t.Errorf("%s: una base senza grammatica", p.Rif)
			}
		}
	})
}

// mailInoltrata: la mail con la tabella a quattro righe e l'oggetto «I: …», senza confine nel corpo: la storia non
// è separabile e tutto il corpo sta in s:storia:1 (R48 A).
func mailInoltrata() string {
	return "Buongiorno,\r\nvi giro la richiesta ACME26-030.\r\n\r\nCodice\r\nQ.TA\r\nP7120100\r\n5\r\nP7120200\r\n5\r\nP7120300\r\n5\r\nP7120400\r\n5\r\n\r\nGrazie"
}

// TestR75LoScenarioETargetDaConfermare (R75 A; T-B0-23; R29 b C; R48 A): con il caso di autorità «scenario» che
// sceglie s:storia:1, i prodotti letti dalla mail in quel segmento sono target di scenario, con l'identità da
// confermare, mai TargetConfermato; senza l'autorità, o senza il caso, restano candidati. Lo scenario è target
// qualunque sia il gesto 1. Un prodotto dello scenario con la stessa base di un target confermato non si ripete.
func TestR75LoScenarioETargetDaConfermare(t *testing.T) {
	m := motoreACME(t)
	th := threadBase(mailInoltrata())
	th.Messaggi[0].Oggetto = testo("I: Richiesta ACME26-030")
	th.Agganci = []fotorfq.AggancioMessaggio{{MessaggioID: idM1, Aggancio: "auto_conversazione"}} // senza il gesto 1
	tid := threadACME
	caso := &valutazione.IngressoCaso{ID: "ACME-SCENARIO", ThreadID: &tid, ClienteID: clienteACME, Autorita: ancoraggio.AutoritaScenario,
		Segmenti: []valutazione.SegmentoDichiarato{{MessaggioID: idM1, SegmentoID: "s:storia:1", Uso: "pertinente", Origine: "scenario", Motivo: "storia non separabile"}}}

	v := valuta(t, th, m, caso)
	want := []string{"scenario:ACME-SCENARIO:1", "scenario:ACME-SCENARIO:2", "scenario:ACME-SCENARIO:3", "scenario:ACME-SCENARIO:4"}
	if !reflect.DeepEqual(rifDi(v), want) {
		t.Fatalf("target %v, attesi i quattro prodotti dello scenario", rifDi(v))
	}
	for i, p := range v.ProdottiValutati {
		if p.CodiceRichiesto != "P712"+[]string{"0100", "0200", "0300", "0400"}[i] || p.Base.Normalizzata == "" {
			t.Errorf("prodotto %d: %+v", i, p)
		}
	}
	if v.Richiesta.Stato != ancoraggio.StatoRichiestaDaValutare {
		t.Errorf("richiesta %q: lo stato del gesto 1 resta com'è", v.Richiesta.Stato)
	}

	t.Run("senza l'autorità o senza il caso: candidati, non target", func(t *testing.T) {
		senza := *caso
		senza.Autorita = ""
		for _, c := range []*valutazione.IngressoCaso{&senza, nil} {
			v := valuta(t, th, m, c)
			if len(v.ProdottiValutati) != 0 {
				t.Errorf("target %v senza lo scenario", rifDi(v))
			}
		}
	})

	t.Run("un prodotto dello scenario e un target confermato con la stessa base: vince il confermato", func(t *testing.T) {
		th := th
		th.Identificativi = []fotorfq.Identificativo{confermato("7120200")}
		v := valuta(t, th, m, caso)
		want := []string{"identificativo:7120200", "scenario:ACME-SCENARIO:1", "scenario:ACME-SCENARIO:2", "scenario:ACME-SCENARIO:3"}
		if !reflect.DeepEqual(rifDi(v), want) {
			t.Errorf("target %v, attesi %v", rifDi(v), want)
		}
	})

	t.Run("un caso di un altro thread è un errore", func(t *testing.T) {
		altro := *caso
		x := uid(0x999)
		altro.ThreadID = &x
		if _, err := valutazione.ValutaProdotti(fotografia(th), th, m, &altro); err == nil {
			t.Error("un caso di un altro thread accettato")
		}
	})
}

// TestValutaProdottiDeterministico (par.3.4.5; MOTORE-SENZA-LLM): lo stesso thread con gli elenchi della fotografia
// permutati dà gli stessi byte canonici, e la fotografia di chi chiama non cambia.
func TestValutaProdottiDeterministico(t *testing.T) {
	m := motoreACME(t)
	th := threadBase("Buongiorno,\r\nvi chiediamo l'offerta per P7120100 e P7120200.")
	th.Messaggi = append(th.Messaggi, messaggio(idM2, 30, "Re: Richiesta", "Aggiungete anche P7120300."))
	th.Agganci = append(th.Agganci, gesto(idM2))
	c1, c2 := uid(0x201), uid(0x202)
	th.Componenti = []fotorfq.Componente{componente(c1, "7120100", "finito", "manuale"), componente(c2, "7120200", "finito", "codice_rilevato")}
	th.Identificativi = []fotorfq.Identificativo{confermato("7120200"), confermato("7120900")}
	conAllegato(&th, allegato(uid(0x301), 1, "7120900.stp", "stp", sha("a")), ptr(fattiSTEP(t, sha("a"), []string{"#1"}, []nodoSTEP{{"#1", "7120900"}}, nil, "")))
	conAllegato(&th, allegato(uid(0x302), 2, "7129999.stp", "stp", sha("b")), nil)
	th.Triage = []fotorfq.Triage{{ID: uid(0x401), MessaggioID: idM1, Esito: "nuova_rfq", Atto: "richiesta_offerta", Stato: "accettata", CreatoIl: dataACME}}

	p := th
	p.Messaggi = []fotorfq.Messaggio{th.Messaggi[1], th.Messaggi[0]}
	p.Agganci = []fotorfq.AggancioMessaggio{th.Agganci[1], th.Agganci[0]}
	p.Componenti = []fotorfq.Componente{th.Componenti[1], th.Componenti[0]}
	p.Identificativi = []fotorfq.Identificativo{th.Identificativi[1], th.Identificativi[0]}
	p.Allegati = []fotorfq.Allegato{th.Allegati[1], th.Allegati[0]}
	prima, _ := jsoncanonico.Codifica(th)

	b1, err1 := jsoncanonico.Codifica(valuta(t, th, m, nil))
	b2, err2 := jsoncanonico.Codifica(valuta(t, p, m, nil))
	if err1 != nil || err2 != nil || string(b1) != string(b2) {
		t.Fatalf("elenchi permutati, esiti diversi:\n%s\n%s", b1, b2)
	}
	if dopo, _ := jsoncanonico.Codifica(th); string(dopo) != string(prima) {
		t.Error("ValutaProdotti ha cambiato la fotografia di chi chiama")
	}
	v := valuta(t, th, m, nil)
	if len(v.ProdottiValutati) != 3 || v.ProdottiValutati[2].Fonte.Stato != valutazione.FonteInAttesaDiConferma {
		t.Errorf("prodotti %+v", v.ProdottiValutati)
	}

}

func ptr[T any](v T) *T { return &v }
