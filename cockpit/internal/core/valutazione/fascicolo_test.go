// L1 — il fascicolo della RFQ (B6, V3; R88, R89, R96 (a) A, (b) B, (c) A con il gesto che resta [R]; contratto §0.3, §1.0,
// §1.7; T-B0-28, T-E1-16; F0-06, F0-07, F0-18; T-B6-09, T-B6-12; LD-23; PO-01, PO-02 in L1, PO-31 sulla regola): la regola
// con prodotti sintetici e un gesto di congelamento sintetico (nessun adattatore: LD-23), poi dalla fotografia, attraverso
// Calcola, con la bom_versione del legacy.
package valutazione_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

// I clienti di questi test sono inventati (ACME, acme.example; codici 712xxxx; UUID 00000000-0000-4000-8000-0000000000nn):
// vedi scena_prodotti_test.go.

// prodottoDelFascicolo: un prodotto sintetico per la regola del fascicolo: il Rif, se è verificato (con i motivi
// altrimenti) e l'impronta.
func prodottoDelFascicolo(rif string, verificato bool, impronta string) valutazione.ProdottoValutato {
	pv := valutazione.ProdottoValutato{Rif: rif, Verificato: verificato, Stato: valutazione.ProdottoProntoFattibilita, Impronta: impronta}
	if !verificato {
		pv.Stato, pv.Motivi = valutazione.ProdottoNonPronto, []valutazione.MotivoProdotto{valutazione.MotivoProdottoSmistamento}
	}
	return pv
}

// legacyCongelata: il congelamento legacy con la versione corrente nello stato dato e, se c'è, l'ultima congelata.
func legacyCongelata(corrente int32, stato string, congelata *int32) *valutazione.CongelamentoLegacy {
	l := &valutazione.CongelamentoLegacy{VersioneCorrente: corrente, StatoCorrente: stato}
	if congelata != nil {
		n := *congelata
		da, il := operatore, dataACME.Add(24*time.Hour)
		l.UltimaCongelata, l.CongelataDa, l.CongelataIl, l.Riaperta = &n, &da, &il, stato != "congelata"
	}
	return l
}

// fascicoloTesto: il fascicolo come testo, per i confronti: congelabile e motivo, congelato e motivo, conflitti, avvisi.
func fascicoloTesto(f valutazione.StatoFascicolo) string {
	c, g := "no", "no"
	if f.Congelabile {
		c = "sì"
	}
	if f.Congelato {
		g = "sì"
	}
	return "congelabile " + c + "/" + f.MotivoNonCongelabile + " congelato " + g + "/" + f.MotivoNonCongelato + " conflitti [" +
		strings.Join(f.ConflittiCongelamento, " ") + "] avvisi [" + strings.Join(f.Avvisi, " ") + "]"
}

// TestPO31IlFascicoloSullaRegola (PO-31 sulla regola; R96 (a) A, (b) B, (c) A, la lettura del gesto che resta [R]; F0-06,
// F0-07; T-E1-16, LD-23): zero target; la V1 congelata con un prodotto non verificato e con tutti verificati; la V2 in
// bozza; con un gesto sintetico e le impronte uguali, cambiate, un target non più verificato, uno nuovo, uno che non c'è
// più; il thread non valutato; il fascicolo non calcolato; gli orfani, che non cambiano Congelabile.
func TestPO31IlFascicoloSullaRegola(t *testing.T) {
	uno := int32(1)
	p1, p2 := "componente:"+uid(0xf41).String(), "componente:"+uid(0xf42).String()
	tutti := []valutazione.ProdottoValutato{prodottoDelFascicolo(p1, true, sha("1")), prodottoDelFascicolo(p2, true, sha("2"))}
	unoNo := []valutazione.ProdottoValutato{prodottoDelFascicolo(p1, true, sha("1")), prodottoDelFascicolo(p2, false, sha("2"))}
	gesto := func(impronte map[string]string) *valutazione.GestoCongelamento {
		return &valutazione.GestoCongelamento{Da: operatore, Il: dataACME.Add(48 * time.Hour), Impronte: impronte}
	}
	uguali := map[string]string{p1: sha("1"), p2: sha("2")}
	ingresso := func(prodotti []valutazione.ProdottoValutato) valutazione.IngressoFascicolo {
		return valutazione.IngressoFascicolo{Valutato: true, Calcolato: true, Prodotti: prodotti, FaseThread: "APERTA"}
	}
	casi := []struct {
		nome   string
		in     valutazione.IngressoFascicolo
		gesto  *valutazione.GestoCongelamento
		atteso string
	}{
		{"zero target: non congelabile (R96 a A)", ingresso(nil), nil, "congelabile no/nessun_target congelato no/gesto_non_registrato conflitti [] avvisi []"},
		{"tutti verificati: congelabile, non congelato senza il gesto (LD-23)", ingresso(tutti), nil,
			"congelabile sì/ congelato no/gesto_non_registrato conflitti [] avvisi []"},
		{"un prodotto non verificato", ingresso(unoNo), nil, "congelabile no/prodotti_non_pronti congelato no/gesto_non_registrato conflitti [] avvisi []"},
		{"la V1 congelata e un prodotto non verificato: si mostra com'è, con il motivo (T-B0-28)", func() valutazione.IngressoFascicolo {
			in := ingresso(unoNo)
			in.Legacy = legacyCongelata(1, "congelata", &uno)
			return in
		}(), nil, "congelabile no/bom_versione_con_prodotti_non_verificati congelato no/gesto_non_registrato conflitti [] avvisi [non_congelabile:prodotti_non_pronti]"},
		{"la V1 congelata e tutti verificati: il gesto legacy non vale come congelato (R96 b B)", func() valutazione.IngressoFascicolo {
			in := ingresso(tutti)
			in.Legacy = legacyCongelata(1, "congelata", &uno)
			return in
		}(), nil, "congelabile sì/ congelato no/gesto_non_registrato conflitti [] avvisi []"},
		{"la V2 in bozza dopo la V1 congelata: riaperto, il motivo della bom_versione non vale (R96 c A)", func() valutazione.IngressoFascicolo {
			in := ingresso(unoNo)
			in.Legacy = legacyCongelata(2, "bozza", &uno)
			return in
		}(), nil, "congelabile no/prodotti_non_pronti congelato no/gesto_non_registrato conflitti [] avvisi []"},
		{"con il gesto e le impronte uguali: congelato, senza conflitti", ingresso(tutti), gesto(uguali),
			"congelabile sì/ congelato sì/ conflitti [] avvisi []"},
		{"con un'impronta cambiata: il gesto resta, con il conflitto", ingresso([]valutazione.ProdottoValutato{prodottoDelFascicolo(p1, true, sha("1")),
			prodottoDelFascicolo(p2, true, sha("3"))}), gesto(uguali), "congelabile sì/ congelato sì/ conflitti [impronta_cambiata:" + p2 + "] avvisi []"},
		{"con un target non più verificato", ingresso(unoNo), gesto(uguali),
			"congelabile no/prodotti_non_pronti congelato sì/ conflitti [non_piu_congelabile] avvisi []"},
		{"con un target aggiunto dopo il gesto", ingresso(tutti), gesto(map[string]string{p1: sha("1")}),
			"congelabile sì/ congelato sì/ conflitti [target_nuovo:" + p2 + "] avvisi []"},
		{"con un target del gesto che oggi non c'è più (F0-07)", ingresso(tutti[:1]), gesto(uguali),
			"congelabile sì/ congelato sì/ conflitti [impronta_cambiata:" + p2 + "] avvisi []"},
		{"con un target del gesto che non c'è più, anche con l'impronta vuota (F0-07: mai silenzio)", ingresso(tutti[:1]),
			gesto(map[string]string{p1: sha("1"), p2: ""}), "congelabile sì/ congelato sì/ conflitti [impronta_cambiata:" + p2 + "] avvisi []"},
		{"tutti i conflitti insieme, nell'ordine", ingresso([]valutazione.ProdottoValutato{prodottoDelFascicolo(p2, false, sha("9")),
			prodottoDelFascicolo("componente:c", true, sha("c"))}), gesto(map[string]string{p2: sha("2"), "componente:b": sha("b"), "componente:a": sha("a")}),
			"congelabile no/prodotti_non_pronti congelato sì/ conflitti [non_piu_congelabile impronta_cambiata:" + p2 + " impronta_cambiata:componente:a " +
				"impronta_cambiata:componente:b target_nuovo:componente:c] avvisi []"},
		{"il thread non valutato, con un prodotto non verificato (T-B6-09)", func() valutazione.IngressoFascicolo {
			in := ingresso(unoNo)
			in.Valutato = false
			return in
		}(), nil, "congelabile no/thread_non_valutato congelato no/gesto_non_registrato conflitti [] avvisi [non_congelabile:prodotti_non_pronti]"},
		{"il thread non valutato senza target", func() valutazione.IngressoFascicolo {
			in := ingresso(nil)
			in.Valutato = false
			return in
		}(), nil, "congelabile no/thread_non_valutato congelato no/gesto_non_registrato conflitti [] avvisi [non_congelabile:nessun_target]"},
		{"il thread non valutato con tutti verificati", func() valutazione.IngressoFascicolo {
			in := ingresso(tutti)
			in.Valutato = false
			return in
		}(), nil, "congelabile no/thread_non_valutato congelato no/gesto_non_registrato conflitti [] avvisi []"},
		{"tutti i motivi insieme, con la precedenza di F0-06", func() valutazione.IngressoFascicolo {
			in := ingresso(unoNo)
			in.Valutato, in.Legacy = false, legacyCongelata(1, "congelata", &uno)
			return in
		}(), nil, "congelabile no/thread_non_valutato congelato no/gesto_non_registrato conflitti [] avvisi " +
			"[non_congelabile:bom_versione_con_prodotti_non_verificati non_congelabile:prodotti_non_pronti]"},
		{"il fascicolo non calcolato (T-12): niente sui target, mai congelabile", func() valutazione.IngressoFascicolo {
			in := ingresso(tutti)
			in.Calcolato = false
			return in
		}(), nil, "congelabile no/ congelato no/gesto_non_registrato conflitti [] avvisi []"},
		{"il fascicolo non calcolato e senza target", func() valutazione.IngressoFascicolo {
			in := ingresso(nil)
			in.Calcolato = false
			return in
		}(), nil, "congelabile no/ congelato no/gesto_non_registrato conflitti [] avvisi []"},
		{"gli orfani sono un avviso e non cambiano Congelabile (R93, T-E1-16)", func() valutazione.IngressoFascicolo {
			in := ingresso(tutti)
			in.DaSmistare = []valutazione.FileDaSmistare{{AllegatoID: uid(0xf43), Orfano: true}, {AllegatoID: uid(0xf44), Prodotti: []string{p1}},
				{AllegatoID: uid(0xf45), Orfano: true, ProdottiContesto: []string{p1, p2}}}
			return in
		}(), nil, "congelabile sì/ congelato no/gesto_non_registrato conflitti [] avvisi [orfano:" + uid(0xf43).String() + " orfano:" + uid(0xf45).String() +
			":prodotti_contesto:" + p1 + "|" + p2 + "]"},
		{"gli orfani prima dei motivi", func() valutazione.IngressoFascicolo {
			in := ingresso(nil)
			in.Valutato, in.DaSmistare = false, []valutazione.FileDaSmistare{{AllegatoID: uid(0xf43), Orfano: true}}
			return in
		}(), nil, "congelabile no/thread_non_valutato congelato no/gesto_non_registrato conflitti [] avvisi [orfano:" + uid(0xf43).String() +
			" non_congelabile:nessun_target]"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			prima := canonicoDi(t, c.in)
			f := valutazione.Fascicolo(c.in, c.gesto)
			if got := fascicoloTesto(f); got != c.atteso {
				t.Errorf("fascicolo\n%s\natteso\n%s", got, c.atteso)
			}
			if canonicoDi(t, c.in) != prima {
				t.Error("Fascicolo ha cambiato l'ingresso di chi chiama")
			}
			if f.FaseThread != "APERTA" || f.Calcolato != c.in.Calcolato || (c.gesto == nil) != (f.CongelatoDa == nil) || (c.gesto == nil) != (f.CongelatoIl == nil) {
				t.Errorf("fase %q, calcolato %v, chi e quando %v %v", f.FaseThread, f.Calcolato, f.CongelatoDa, f.CongelatoIl)
			}
			if c.gesto != nil && (*f.CongelatoDa != c.gesto.Da || !f.CongelatoIl.Equal(c.gesto.Il)) {
				t.Errorf("chi e quando del gesto: %v %v", f.CongelatoDa, f.CongelatoIl)
			}
		})
	}

	t.Run("i conteggi, i pronti e i bloccati in ordine di Rif, con i loro motivi", func(t *testing.T) {
		in := ingresso([]valutazione.ProdottoValutato{prodottoDelFascicolo("componente:z", false, ""), prodottoDelFascicolo("componente:b", true, ""),
			prodottoDelFascicolo("componente:a", true, ""), prodottoDelFascicolo("componente:c", false, "")})
		f := valutazione.Fascicolo(in, nil)
		if f.NumeroTarget != 4 || f.NumeroVerificati != 2 || !reflect.DeepEqual(f.Pronti, []string{"componente:a", "componente:b"}) || len(f.Bloccati) != 2 ||
			f.Bloccati[0].Rif != "componente:c" || f.Bloccati[1].Rif != "componente:z" || motiviProdotto(f.Bloccati[0].Motivi) != "smistamento_non_verificato" {
			t.Errorf("fascicolo %+v", f)
		}
		f.Bloccati[0].Motivi[0] = "cambiato"
		if in.Prodotti[3].Motivi[0] != valutazione.MotivoProdottoSmistamento {
			t.Error("i motivi dei bloccati condividono la memoria con i prodotti")
		}
	})
	t.Run("il congelamento legacy si mostra com'è, in una copia", func(t *testing.T) {
		in := ingresso(tutti)
		in.Legacy = legacyCongelata(2, "bozza", &uno)
		f := valutazione.Fascicolo(in, nil)
		if !reflect.DeepEqual(f.Legacy, in.Legacy) || f.Legacy == in.Legacy || f.Legacy.UltimaCongelata == in.Legacy.UltimaCongelata ||
			f.Legacy.CongelataDa == in.Legacy.CongelataDa || f.Legacy.CongelataIl == in.Legacy.CongelataIl {
			t.Errorf("legacy %+v", f.Legacy)
		}
		in.Legacy.UltimaCongelata, in.Legacy.CongelataDa, in.Legacy.CongelataIl = nil, nil, nil
		if g := valutazione.Fascicolo(in, nil); g.Legacy.UltimaCongelata != nil || !g.Legacy.Riaperta {
			t.Errorf("legacy senza la congelata %+v", g.Legacy)
		}
	})
	t.Run("il determinismo: i prodotti permutati e le impronte del gesto in ordine di chiave", func(t *testing.T) {
		impronte := map[string]string{}
		var prodotti []valutazione.ProdottoValutato
		for i := 0; i < 12; i++ {
			rif := "componente:" + uid(0xf50+i).String()
			impronte[rif] = sha("e")
			prodotti = append(prodotti, prodottoDelFascicolo(rif, i%3 != 0, sha("f")))
		}
		a := valutazione.Fascicolo(ingresso(prodotti), gesto(impronte))
		for k := 0; k < 5; k++ {
			b := valutazione.Fascicolo(ingresso(rovescia(prodotti)), gesto(impronte))
			if canonicoDi(t, a) != canonicoDi(t, b) {
				t.Fatal("il fascicolo cambia con l'ordine dei prodotti o delle impronte")
			}
		}
	})
}

// TestPO01DueProdottiUnoBloccato (PO-01 in L1; R88): la RFQ con due prodotti, uno verificato e pronto, l'altro (il secondo
// finito, con il suo STEP non autorizzato) bloccato con i suoi motivi: gli altri avanzano da soli, e il fascicolo non è
// congelabile, con prodotti_non_pronti.
func TestPO01DueProdottiUnoBloccato(t *testing.T) {
	th := scenaSmistamento(t)
	secondoProdotto(t, &th)
	et := esitoSmistamento(t, th)
	p, q := prodottoDi(t, et, rifProdB), prodottoDi(t, et, rifP2)
	if statoProdotto(p) != "sì/pronto_fattibilita/" || q.Verificato || q.Stato != valutazione.ProdottoNonPronto ||
		!strings.HasPrefix(motiviProdotto(q.Motivi), "fonte_strutturale_step_mancante_o_non_confermata") {
		t.Fatalf("il primo %s, il secondo %s", statoProdotto(p), statoProdotto(q))
	}
	fa := et.Fascicolo
	if fa.NumeroTarget != 2 || fa.NumeroVerificati != 1 || !reflect.DeepEqual(fa.Pronti, []string{rifProdB}) || len(fa.Bloccati) != 1 ||
		fa.Bloccati[0].Rif != rifP2 || !reflect.DeepEqual(fa.Bloccati[0].Motivi, q.Motivi) || fascicoloTesto(fa) !=
		"congelabile no/prodotti_non_pronti congelato no/gesto_non_registrato conflitti [] avvisi []" || fa.FaseThread != "APERTA" {
		t.Errorf("fascicolo %+v", fa)
	}
}

// TestPO02IlFascicoloCongelabileNonCongelato (PO-02 in L1; R89, R96 b B e c A; T-B0-28; LD-23): l'ultimo prodotto
// diventa verificato (il 2D nuovo confermato), e il fascicolo è congelabile ma non congelato senza il gesto del modello
// nuovo: senza bom_versione, con la V1 congelata (che si mostra in Legacy), con la V2 in bozza (riaperto).
func TestPO02IlFascicoloCongelabileNonCongelato(t *testing.T) {
	th := scenaSmistamento(t)
	fileNelMessaggio(&th, fPDFNuovo, idM1, "7120200A_1.pdf", "pdf", shaN(330), scansione(t, shaN(330)))
	conProposta(&th, uid(0xf46), fPDFNuovo, "disegno_2d", "nome_file", "aperta", nil, nil)
	if fa := esitoSmistamento(t, th).Fascicolo; fa.Congelabile || fa.MotivoNonCongelabile != valutazione.NonCongelabileProdottiNonPronti {
		t.Fatalf("prima: %s", fascicoloTesto(fa))
	}
	th.Documenti = append(th.Documenti, documento2D(uid(0xf47), cSciolto, fPDFNuovo, "7120200A_1.pdf", "pdf", shaN(330)))
	v1 := &fotorfq.VersioneBOM{ID: uid(0xf48), Numero: 1, Stato: "congelata", Contesto: "fattibilita", CongelataDa: &operatore,
		CongelataIl: func() *time.Time { x := dataACME.Add(24 * time.Hour); return &x }()}
	v2 := &fotorfq.VersioneBOM{ID: uid(0xf49), Numero: 2, Stato: "bozza", Contesto: "fattibilita"}
	casi := []struct {
		nome      string
		corrente  *fotorfq.VersioneBOM
		congelata *fotorfq.VersioneBOM
		legacy    string
	}{
		{"senza bom_versione", nil, nil, ""},
		{"con la V1 congelata", v1, v1, "1/congelata/1/no"},
		{"con la V2 in bozza dopo la V1 congelata", v2, v1, "2/bozza/1/sì"},
		{"con la sola V1 in bozza", &fotorfq.VersioneBOM{ID: uid(0xf4a), Numero: 1, Stato: "bozza"}, nil, "1/bozza/-/no"},
		{"con la sola congelata, senza la corrente", nil, v1, "1/congelata/1/no"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			x := th
			x.VersioneBOM, x.UltimaCongelata = c.corrente, c.congelata
			et := esitoSmistamento(t, x)
			if p := prodottoDi(t, et, rifProdB); statoProdotto(p) != "sì/pronto_fattibilita/" {
				t.Fatalf("l'ultimo prodotto: %s", statoProdotto(p))
			}
			fa := et.Fascicolo
			if fascicoloTesto(fa) != "congelabile sì/ congelato no/gesto_non_registrato conflitti [] avvisi []" {
				t.Errorf("fascicolo %s", fascicoloTesto(fa))
			}
			if got := legacyTesto(fa.Legacy); got != c.legacy {
				t.Errorf("legacy %q, atteso %q", got, c.legacy)
			}
			if c.congelata != nil && (fa.Legacy.CongelataDa == nil || *fa.Legacy.CongelataDa != operatore || fa.Legacy.CongelataIl == nil ||
				!fa.Legacy.CongelataIl.Equal(*c.congelata.CongelataIl)) {
				t.Errorf("chi e quando della congelata: %+v", fa.Legacy)
			}
		})
	}
}

// legacyTesto: il congelamento legacy come testo «corrente/stato/ultima congelata/riaperta»; "" senza.
func legacyTesto(l *valutazione.CongelamentoLegacy) string {
	if l == nil {
		return ""
	}
	u, r := "-", "no"
	if l.UltimaCongelata != nil {
		u = string(rune('0' + *l.UltimaCongelata))
	}
	if l.Riaperta {
		r = "sì"
	}
	return string(rune('0'+l.VersioneCorrente)) + "/" + l.StatoCorrente + "/" + u + "/" + r
}

// TestIlFascicoloDelThreadNonValutato (T-B6-09; F0-06; T-12; dubbio T-B6-86): senza l'insieme delle regole il thread non
// è valutato, i prodotti sono informazione, non_pronto, e il fascicolo non si congela (thread_non_valutato) né è calcolato,
// perché gli orfani non si calcolano (R-86 della controprova di V3: lo stesso criterio della pertinenza); con un errore
// della valutazione il fascicolo non è calcolato, con lo stesso motivo; con una sezione dei target, della versione della
// BOM o dello smistamento assente il fascicolo non è calcolato (le sezioni dello smistamento: la nota di V2, con R-75 della
// revisione di V2, accolta con la revisione di V3), e nessun orfano si mostra come calcolato.
func TestIlFascicoloDelThreadNonValutato(t *testing.T) {
	th := scenaSmistamento(t)
	vistaCome(&th)
	t.Run("senza regole", func(t *testing.T) {
		et := esitoThread(t, calcola(t, fotografia(th), nil, valutazione.Ingressi{}), threadACME)
		p := prodottoDi(t, et, rifProdB)
		if et.Valutato || p.Stato != valutazione.ProdottoNonPronto || p.Verificato || len(p.Impronta) != 64 ||
			fascicoloTesto(et.Fascicolo) != "congelabile no/thread_non_valutato congelato no/gesto_non_registrato conflitti [] avvisi []" ||
			et.Fascicolo.Calcolato || et.Fascicolo.NumeroTarget != 1 || et.Fascicolo.Orfani != 0 {
			t.Errorf("prodotto %s, fascicolo %+v", statoProdotto(p), et.Fascicolo)
		}
	})
	t.Run("con un errore della valutazione", func(t *testing.T) {
		tid := threadACME
		caso := valutazione.IngressoCaso{ID: "ACME-ERRORE", ThreadID: &tid, ClienteID: altroCliente}
		th := th
		th.VersioneBOM = &fotorfq.VersioneBOM{ID: uid(0xf4b), Numero: 1, Stato: "bozza"}
		et := esitoThread(t, calcola(t, fotografia(th), insiemeACME(t), valutazione.Ingressi{Versione: valutazione.VersioneCasi, Casi: []valutazione.IngressoCaso{caso}}),
			threadACME)
		fa := et.Fascicolo
		if et.Valutato || et.Motivo != valutazione.MotivoThreadErroreValutazione || fa.Calcolato || fa.NumeroTarget != 0 ||
			fascicoloTesto(fa) != "congelabile no/thread_non_valutato congelato no/gesto_non_registrato conflitti [] avvisi []" || legacyTesto(fa.Legacy) != "1/bozza/-/no" {
			t.Errorf("thread con l'errore: valutato %v %q, fascicolo %+v", et.Valutato, et.Motivo, fa)
		}
	})
	for _, k := range []string{fotorfq.SezioneIdentificativi, fotorfq.SezioneComponenti, fotorfq.SezioneVersioneBOM, fotorfq.SezioneAllegati,
		fotorfq.SezioneMessaggi, fotorfq.SezioneProposteDocumento, fotorfq.SezioneDocumenti, fotorfq.SezioneProvenienze, fotorfq.SezioneRelazioni,
		fotorfq.SezioneRigheComponenteProposta} {
		t.Run("senza la sezione "+k, func(t *testing.T) {
			f := fotografia(th)
			f.Sezioni[k] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneAssente}
			fa := esitoThread(t, calcola(t, f, insiemeACME(t), valutazione.Ingressi{}), threadACME).Fascicolo
			if fa.Calcolato || fa.Congelabile || fa.MotivoNonCongelabile != "" || fa.Orfani != 0 {
				t.Errorf("fascicolo %+v", fa)
			}
		})
	}
	for _, k := range []string{fotorfq.SezioneTriage, fotorfq.SezioneLavoroPendente, fotorfq.SezioneRigheRelazioneProposta} {
		t.Run("senza la sezione "+k+" il fascicolo si calcola", func(t *testing.T) {
			f := fotografia(th)
			f.Sezioni[k] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneAssente}
			if fa := esitoThread(t, calcola(t, f, insiemeACME(t), valutazione.Ingressi{}), threadACME).Fascicolo; !fa.Calcolato {
				t.Errorf("fascicolo %+v", fa)
			}
		})
	}
	t.Run("con le sezioni filtrate o parziali il fascicolo si calcola", func(t *testing.T) {
		f := fotografia(th)
		f.Sezioni[fotorfq.SezioneVersioneBOM] = fotorfq.StatoSezione{Stato: fotorfq.StatoSezioneParziale}
		if fa := esitoThread(t, calcola(t, f, insiemeACME(t), valutazione.Ingressi{}), threadACME).Fascicolo; !fa.Calcolato || !fa.Congelabile {
			t.Errorf("fascicolo %+v", fa)
		}
	})
}

// TestLaDerogaRipetutaPerIlComponenteCondiviso (T-B5-61, T-B6-31; dubbio T-B6-88): una deroga sul 2D di un componente
// condiviso da due prodotti dà documenti.deroga_non_sostituisce_2d una volta per prodotto, con il percorso della
// completezza di ognuno: come per i conflitti (F0-13), una voce per prodotto, e nessuna diagnostica si fonde.
func TestLaDerogaRipetutaPerIlComponenteCondiviso(t *testing.T) {
	th := scenaSmistamento(t)
	secondoProdotto(t, &th, "7120200A")
	th.Relazioni = append(th.Relazioni, fotorfq.Relazione{PadreID: cP2, FiglioID: cSciolto, Qta: 1, Origine: "step", ConfermatoDa: operatore, CreatoIl: dataACME})
	deroga := uid(0xf4c)
	th.Deroghe = append(th.Deroghe, fotorfq.DerogaFabbisogno{ID: deroga, ComponenteID: cSciolto, Tipo: "disegno_2d", Motivo: "prova", UtenteID: operatore,
		CreataIl: dataACME})
	et := esitoSmistamento(t, th)
	var percorsi []string
	for _, d := range et.Diagnostiche {
		if d.Codice == valutazione.CodiceDocumentiDerogaNonSostituisce2D {
			percorsi = append(percorsi, d.Percorso)
			if !contieneRif(d.Rif, ancoraggio.RifComponente(cSciolto)) || !contieneRif(d.Rif, "deroga_fabbisogno:"+deroga.String()) {
				t.Errorf("riferimenti %v", d.Rif)
			}
		}
	}
	atteso := []string{"prodotti[" + rifProdB + "].documenti", "prodotti[" + rifP2 + "].documenti"}
	if rifP2 < rifProdB {
		atteso[0], atteso[1] = atteso[1], atteso[0]
	}
	if !reflect.DeepEqual(percorsi, atteso) {
		t.Errorf("le diagnostiche della deroga: %v, attese %v", percorsi, atteso)
	}
}
