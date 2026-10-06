// L1 — RichiestaDelThread, riscritta come dice il contratto di A1c (§7: A1c-L1-17; 6.0.5, 6.4.4, 6.4.6; R29 b C e c;
// R48 A; T-B0-06; T-16): lo stato di ogni messaggio dal gesto 1, dalla direzione e dalla controparte, mai dal triage
// («ignora» non rende non valutabile, «nuova_rfq» non rende valutata); lo stato del thread; l'uso dei segmenti con il
// caso (origine «scenario», solo i segmenti dichiarati) o con il riconoscimento tracciato (solo la parte corrente di
// un messaggio valutato, mai la storia, nemmeno non separabile); l'impronta dei gesti; lo stesso file dei casi dà la
// stessa richiesta nel banco e nell'anteprima.
package valutazione_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// documenti: i documenti dei messaggi del thread, per messaggio, come li fa valutazione.
func documenti(t *testing.T, th fotorfq.Thread) map[uuid.UUID]evidenze.DocumentoEvidenze {
	t.Helper()
	out := map[uuid.UUID]evidenze.DocumentoEvidenze{}
	for _, m := range th.Messaggi {
		d, err := estrazione.DaMessaggio(m)
		if err != nil {
			t.Fatal(err)
		}
		out[m.ID] = d
	}
	return out
}

func gestoDi(t *testing.T, r ancoraggio.RichiestaValutata, id uuid.UUID) ancoraggio.GestoMessaggio {
	t.Helper()
	for _, g := range r.Gesti {
		if g.MessaggioID == id {
			return g
		}
	}
	t.Fatalf("il messaggio %s non ha il suo gesto nella richiesta", id)
	return ancoraggio.GestoMessaggio{}
}

func triage(id uuid.UUID, messaggio uuid.UUID, esito, atto string) fotorfq.Triage {
	return fotorfq.Triage{ID: id, MessaggioID: messaggio, Esito: esito, Atto: atto, Stato: "accettata", CreatoIl: dataACME}
}

// TestL117LoStatoDiOgniMessaggio (A1c-L1-17 riscritta; 6.0.5; R29 c; contratto §1.1, nota): valutata solo con il
// gesto 1 dell'operatore, il messaggio in entrata e la controparte il cliente del thread; il triage resta in Evento e
// non decide niente.
func TestL117LoStatoDiOgniMessaggio(t *testing.T) {
	base := func(mod func(m *fotorfq.Messaggio, a *fotorfq.AggancioMessaggio)) fotorfq.Thread {
		th := threadBase("Buongiorno,\r\nrichiesta per P7120100.")
		mod(&th.Messaggi[0], &th.Agganci[0])
		return th
	}
	for _, c := range []struct {
		nome   string
		mod    func(m *fotorfq.Messaggio, a *fotorfq.AggancioMessaggio)
		triage string
		atteso ancoraggio.StatoRichiesta
	}{
		{"gesto 1, entrata, cliente del thread", func(*fotorfq.Messaggio, *fotorfq.AggancioMessaggio) {}, "", ancoraggio.StatoRichiestaValutata},
		{"gesto 1 e triage «ignora»: il triage non conta", func(*fotorfq.Messaggio, *fotorfq.AggancioMessaggio) {}, "ignora", ancoraggio.StatoRichiestaValutata},
		{"triage «nuova_rfq» senza gesto: da valutare", func(_ *fotorfq.Messaggio, a *fotorfq.AggancioMessaggio) {
			*a = fotorfq.AggancioMessaggio{MessaggioID: a.MessaggioID, Aggancio: "auto_conversazione"}
		}, "nuova_rfq", ancoraggio.StatoRichiestaDaValutare},
		{"aggancio dell'operatore senza la data", func(_ *fotorfq.Messaggio, a *fotorfq.AggancioMessaggio) { a.AgganciatoIl = nil }, "", ancoraggio.StatoRichiestaDaValutare},
		{"in uscita", func(m *fotorfq.Messaggio, _ *fotorfq.AggancioMessaggio) { m.Direzione = "uscita" }, "", ancoraggio.StatoRichiestaNonValutabile},
		{"un fornitore", func(m *fotorfq.Messaggio, _ *fotorfq.AggancioMessaggio) {
			m.ControparteTipo, m.ControparteClienteID = "fornitore", nil
		}, "", ancoraggio.StatoRichiestaNonValutabile},
		{"un altro cliente", func(m *fotorfq.Messaggio, _ *fotorfq.AggancioMessaggio) {
			c := altroCliente
			m.ControparteClienteID = &c
		}, "", ancoraggio.StatoRichiestaNonValutabile},
		{"interno", func(m *fotorfq.Messaggio, _ *fotorfq.AggancioMessaggio) {
			m.ControparteTipo, m.ControparteClienteID = "interno", nil
		}, "", ancoraggio.StatoRichiestaDaValutare},
		{"sconosciuto", func(m *fotorfq.Messaggio, _ *fotorfq.AggancioMessaggio) {
			m.ControparteTipo, m.ControparteClienteID = "sconosciuto", nil
		}, "nuova_rfq", ancoraggio.StatoRichiestaDaValutare},
		{"direzione non nota", func(m *fotorfq.Messaggio, _ *fotorfq.AggancioMessaggio) { m.Direzione = "" }, "", ancoraggio.StatoRichiestaDaValutare},
	} {
		t.Run(c.nome, func(t *testing.T) {
			th := base(c.mod)
			if c.triage != "" {
				th.Triage = []fotorfq.Triage{triage(uid(0x701), idM1, c.triage, "richiesta_offerta")}
			}
			r, usi := valutazione.RichiestaDelThread(th, nil, documenti(t, th))
			g := gestoDi(t, r, idM1)
			if g.Stato != c.atteso || r.Stato != c.atteso || g.Motivo == "" {
				t.Fatalf("gesto %+v, richiesta %q: atteso %q", g, r.Stato, c.atteso)
			}
			if c.triage != "" && g.Evento != c.triage+"/richiesta_offerta" {
				t.Errorf("evento %q: il triage resta come evidenza", g.Evento)
			}
			uso := usi[documenti(t, th)[idM1].BundleID]
			if c.atteso == ancoraggio.StatoRichiestaValutata {
				if len(uso.Selezioni) != 1 || uso.Selezioni[0].SegmentoID != "s:corrente" || uso.Selezioni[0].Origine != "riconoscimento" ||
					uso.Selezioni[0].Uso != "pertinente" || len(r.Segmenti) != 1 || r.Segmenti[0].Origine != "riconoscimento" || r.Decisioni == "" {
					t.Errorf("uso %+v, segmenti %+v, decisioni %q: attesa la parte corrente per riconoscimento", uso, r.Segmenti, r.Decisioni)
				}
				if c.triage != "" && !strings.Contains(uso.Selezioni[0].Motivo, "ignora/richiesta_offerta") {
					t.Errorf("motivo %q: il triage sta nel motivo, come evidenza", uso.Selezioni[0].Motivo)
				}
			} else if uso.Stato != "sconosciuto" || len(uso.Selezioni) != 0 || len(r.Segmenti) != 0 || r.Decisioni != "" {
				t.Errorf("uso %+v, segmenti %+v, decisioni %q: senza gesto nessun riconoscimento e nessuna decisione", uso, r.Segmenti, r.Decisioni)
			}
		})
	}
}

// TestL117LoStatoDelThreadELeDecisioni (T-B0-06; T-16): valutata se un messaggio lo è; non_valutabile solo se lo
// sono tutti; le decisioni sono l'impronta dei gesti dei messaggi valutati, in ordine di messaggio, uguale con gli
// elenchi permutati e diversa con un altro operatore.
func TestL117LoStatoDelThreadELeDecisioni(t *testing.T) {
	th := threadBase("Richiesta.")
	m2 := messaggio(idM2, 30, "Re: Richiesta", "Altro.")
	m2.Direzione = "uscita"
	th.Messaggi = append(th.Messaggi, m2)
	th.Agganci = append(th.Agganci, gesto(idM2))
	r, _ := valutazione.RichiestaDelThread(th, nil, documenti(t, th))
	if r.Stato != ancoraggio.StatoRichiestaValutata || !reflect.DeepEqual(r.Messaggi, []uuid.UUID{idM1, idM2}) {
		t.Errorf("richiesta %+v", r)
	}
	il := dataACME.Add(time.Hour)
	attese, err := jsoncanonico.ImprontaDi([]map[string]any{{"messaggio_id": idM1.String(), "agganciato_da": operatore.String(), "agganciato_il": il}})
	if err != nil || r.Decisioni != attese {
		t.Errorf("decisioni %q, attese %q (T-16: solo i gesti dei messaggi valutati)", r.Decisioni, attese)
	}

	p := th
	p.Messaggi = []fotorfq.Messaggio{th.Messaggi[1], th.Messaggi[0]}
	p.Agganci = []fotorfq.AggancioMessaggio{th.Agganci[1], th.Agganci[0]}
	rp, _ := valutazione.RichiestaDelThread(p, nil, documenti(t, p))
	if !reflect.DeepEqual(r, rp) {
		t.Errorf("elenchi permutati, richieste diverse:\n%+v\n%+v", r, rp)
	}
	altro := th
	altro.Agganci = append([]fotorfq.AggancioMessaggio(nil), th.Agganci...)
	da := uid(0xa2)
	altro.Agganci[0].AgganciatoDa = &da
	if ra, _ := valutazione.RichiestaDelThread(altro, nil, documenti(t, altro)); ra.Decisioni == r.Decisioni {
		t.Error("un altro operatore, la stessa impronta delle decisioni")
	}

	tutti := th
	tutti.Messaggi = append([]fotorfq.Messaggio(nil), th.Messaggi...)
	tutti.Messaggi[0].Direzione = "uscita"
	if rt, _ := valutazione.RichiestaDelThread(tutti, nil, documenti(t, tutti)); rt.Stato != ancoraggio.StatoRichiestaNonValutabile || rt.Decisioni != "" {
		t.Errorf("tutti in uscita: %q, %q", rt.Stato, rt.Decisioni)
	}
	misto := tutti
	misto.Agganci = []fotorfq.AggancioMessaggio{{MessaggioID: idM1, Aggancio: "nessuno"}, gesto(idM2)}
	misto.Messaggi = append([]fotorfq.Messaggio(nil), th.Messaggi...)
	if rm, _ := valutazione.RichiestaDelThread(misto, nil, documenti(t, misto)); rm.Stato != ancoraggio.StatoRichiestaDaValutare {
		t.Errorf("uno da valutare e uno non valutabile: %q", rm.Stato)
	}
}

// TestL117IlCasoEIlRiconoscimento (R29 b C; R48 A; R75 A; 6.4.6): con il caso, solo i segmenti dichiarati, con
// l'origine «scenario» e il caso nel riferimento, e gli altri messaggi con l'uso sconosciuto; i messaggi del caso, se
// li elenca. Senza caso, la mail inoltrata senza confine non dà prodotti: il riconoscimento tocca solo la parte
// corrente, vuota, e la storia resta menzione.
func TestL117IlCasoEIlRiconoscimento(t *testing.T) {
	m := motoreACME(t)
	th := threadBase(mailInoltrata())
	th.Messaggi[0].Oggetto = testo("I: Richiesta ACME26-030")
	th.Messaggi = append(th.Messaggi, messaggio(idM2, 30, "Re: Richiesta", "Aggiungete P7120900."))
	th.Agganci = append(th.Agganci, gesto(idM2))
	docs := documenti(t, th)
	tid := threadACME
	caso := &valutazione.IngressoCaso{ID: "ACME-SCENARIO", ThreadID: &tid, ClienteID: clienteACME, Autorita: ancoraggio.AutoritaScenario,
		Segmenti: []valutazione.SegmentoDichiarato{{MessaggioID: idM1, SegmentoID: "s:storia:1", Uso: "pertinente", Origine: "scenario", Motivo: "storia non separabile"}}}

	r, usi := valutazione.RichiestaDelThread(th, caso, docs)
	u1, u2 := usi[docs[idM1].BundleID], usi[docs[idM2].BundleID]
	if len(u1.Selezioni) != 1 || u1.Selezioni[0].SegmentoID != "s:storia:1" || u1.Selezioni[0].Origine != "scenario" || u1.Selezioni[0].Rif != "caso:ACME-SCENARIO" {
		t.Errorf("uso del messaggio del caso %+v", u1)
	}
	if u2.Stato != "sconosciuto" || len(r.Segmenti) != 1 || r.Segmenti[0].Origine != "scenario" {
		t.Errorf("uso dell'altro messaggio %+v, segmenti %+v: con il caso niente riconoscimento", u2, r.Segmenti)
	}

	soloIlPrimo := *caso
	soloIlPrimo.Messaggi = []uuid.UUID{idM1}
	if r, _ := valutazione.RichiestaDelThread(th, &soloIlPrimo, docs); !reflect.DeepEqual(r.Messaggi, []uuid.UUID{idM1}) {
		t.Errorf("messaggi della richiesta %v: attesi quelli del caso", r.Messaggi)
	}

	v := valuta(t, th, m, nil)
	for _, c := range v.Prodotti.Candidati {
		for _, e := range c.Evidenze {
			if e.Segmento != "s:corrente" || e.MessaggioID != idM2 {
				t.Errorf("candidato %s dalla storia o dalla parte vuota del primo messaggio: %+v (R48 A)", c.CodiceRichiesto, e)
			}
		}
	}
	if len(v.Prodotti.Candidati) != 1 || v.Prodotti.Candidati[0].OrigineUso != "riconoscimento" {
		t.Errorf("candidati %+v: atteso il solo codice del secondo messaggio, per riconoscimento", v.Prodotti.Candidati)
	}

	t.Run("banco e anteprima: lo stesso file dei casi, la stessa richiesta", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join("testdata", "casi_acme.v1.json"))
		if err != nil {
			t.Fatal(err)
		}
		banco, err1 := valutazione.LeggiIngressi(raw)
		anteprima, err2 := valutazione.LeggiIngressi(append([]byte(nil), raw...))
		if err1 != nil || err2 != nil {
			t.Fatal(err1, err2)
		}
		rb, ub := valutazione.RichiestaDelThread(th, banco.CasoDelThread(threadACME), docs)
		ra, ua := valutazione.RichiestaDelThread(th, anteprima.CasoDelThread(threadACME), docs)
		if !reflect.DeepEqual(rb, ra) || !reflect.DeepEqual(ub, ua) || len(rb.Segmenti) != 1 || rb.Segmenti[0].Origine != "scenario" {
			t.Errorf("banco %+v\nanteprima %+v", rb, ra)
		}
	})
}
