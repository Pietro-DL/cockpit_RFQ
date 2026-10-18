// L1 — la chiave dei gruppi e il gruppo corrente della sorgente (V1: emendamento di V1 §6.1, §6.4.1 con il secondo e il
// terzo giro su O-01; piano della 0023 T-09, la parte del gruppo corrente; A-09, la parità con la pagina, sta in
// transport/web/motore_a). Gli esiti sono sintetici: due copie A e B dello stesso STEP (A la copia della bozza, B il
// portatore delle righe legacy), un altro STEP C, due prodotti P1 e P2. Tutto è inventato.
package propostabom_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/valutazione"
)

var (
	allA = uuid.MustParse("00000000-0000-4000-8000-0000000a0001")
	allB = uuid.MustParse("00000000-0000-4000-8000-0000000a0002")
	allC = uuid.MustParse("00000000-0000-4000-8000-0000000a0003")
	shaX = strings.Repeat("ab", 32)
	shaY = strings.Repeat("cd", 32)
	r1   = ancoraggio.RifNodo(shaX, "#1")
	r7   = ancoraggio.RifNodo(shaX, "#7")
	rY   = ancoraggio.RifNodo(shaY, "#1")
	p1   = "componente:00000000-0000-4000-8000-0000000c0001"
	p2   = "componente:00000000-0000-4000-8000-0000000c0002"
)

func candidata(a uuid.UUID, sha, radice string) valutazione.StrutturaCandidata {
	return valutazione.StrutturaCandidata{AllegatoID: a, Sha256: sha, Radice: radice}
}

func bomDi(target string, a uuid.UUID, sha, radice string) ancoraggio.StrutturaProdotto {
	return ancoraggio.StrutturaProdotto{Target: target, AllegatoID: a, Sha256: sha, Radice: radice, Stato: ancoraggio.StatoBOMDiLavoroProposta}
}

// prodotto: un prodotto valutato, con la BOM di lavoro (un nodo) o senza, e le candidate date.
func prodotto(rif string, conBOM bool, candidate ...valutazione.StrutturaCandidata) valutazione.ProdottoValutato {
	pv := valutazione.ProdottoValutato{Rif: rif, StruttureCandidate: candidate}
	if conBOM {
		pv.Nodi = []valutazione.NodoBOM{{Nodo: ancoraggio.NodoProposto{Rif: r1}}}
	}
	return pv
}

func uidP(u uuid.UUID) *uuid.UUID { return &u }

// TestChiaveDelGruppo: le costanti e la forma della chiave di una candidata, «cand:<allegato_id>:<radice>».
func TestChiaveDelGruppo(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{propostabom.GruppoBOM, "bom"},
		{propostabom.GruppoCandidata, "cand"},
		{propostabom.GruppoSenzaSTEP, "senza"},
		{propostabom.ChiaveCandidata(allA, r1), "cand:" + allA.String() + ":" + r1},
		{propostabom.ComeStessaOccorrenza, "stessa_occorrenza"},
		{propostabom.ComeAltraOccorrenza, "altra_occorrenza"},
		{propostabom.ComeAssente, "assente"},
		{propostabom.MotivoNessunGruppo, "nessun_gruppo"},
		{propostabom.MotivoOccorrenzaNonAttiva, "occorrenza_non_attiva"},
		{propostabom.MotivoSenzaRadice, "senza_radice"},
	} {
		if c.got != c.want {
			t.Errorf("%q, atteso %q", c.got, c.want)
		}
	}
}

// TestGruppiDelProdotto: i gruppi nell'ordine della pagina (bom, le candidate nell'ordine dell'esito, senza), con i dati
// della struttura; la BOM di un altro prodotto o una candidata non danno i dati del gruppo «bom»; con due BOM di lavoro
// dello stesso prodotto vale la prima; senza BOM di lavoro niente «bom», senza confermati senza STEP niente «senza»; gli
// ingressi non cambiano.
func TestGruppiDelProdotto(t *testing.T) {
	pv := prodotto(p1, true, candidata(allC, shaY, rY), candidata(allA, shaX, r7))
	pv.ConfermatiSenzaSTEP = []valutazione.NodoBOM{{Nodo: ancoraggio.NodoProposto{Rif: "componente:00000000-0000-4000-8000-0000000c0009"}}}
	cand := ancoraggio.StrutturaProdotto{Target: p1, AllegatoID: allC, Sha256: shaY, Radice: rY, Stato: ancoraggio.StatoStrutturaCandidata}
	strutture := []ancoraggio.StrutturaProdotto{bomDi(p2, allC, shaY, rY), cand, bomDi(p1, allB, shaX, r1)}
	prima, _ := json.Marshal([]any{pv, strutture})
	got := propostabom.GruppiDelProdotto(pv, strutture)
	want := []propostabom.Gruppo{
		{Chiave: "bom", Tipo: "bom", AllegatoID: uidP(allB), Sha256: shaX, Radice: r1},
		{Chiave: propostabom.ChiaveCandidata(allC, rY), Tipo: "cand", AllegatoID: uidP(allC), Sha256: shaY, Radice: rY},
		{Chiave: propostabom.ChiaveCandidata(allA, r7), Tipo: "cand", AllegatoID: uidP(allA), Sha256: shaX, Radice: r7},
		{Chiave: "senza", Tipo: "senza"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("gruppi %+v\natteso %+v", got, want)
	}
	if dopo, _ := json.Marshal([]any{pv, strutture}); string(dopo) != string(prima) {
		t.Error("GruppiDelProdotto ha cambiato gli ingressi")
	}
	got[0].AllegatoID = uidP(allC)
	if pv2 := propostabom.GruppiDelProdotto(pv, strutture); *pv2[0].AllegatoID != allB {
		t.Error("i gruppi condividono l'allegato con chi chiama")
	}
	// Con la BOM di lavoro ma senza la sua struttura (una struttura che non è della BOM, o di un altro prodotto): il gruppo
	// «bom» c'è, senza i dati.
	if g := propostabom.GruppiDelProdotto(prodotto(p1, true), []ancoraggio.StrutturaProdotto{bomDi(p2, allA, shaX, r1), cand}); len(g) != 1 ||
		g[0].Chiave != "bom" || g[0].AllegatoID != nil || g[0].Sha256 != "" || g[0].Radice != "" {
		t.Errorf("«bom» senza la struttura della BOM di lavoro: %+v", g)
	}
	if g := propostabom.GruppiDelProdotto(prodotto(p1, false), []ancoraggio.StrutturaProdotto{bomDi(p1, allA, shaX, r1)}); len(g) != 0 {
		t.Errorf("senza BOM di lavoro né candidate né confermati: %+v", g)
	}
	// Due BOM di lavoro dello stesso prodotto (fuori da R76 A, che ne ammette una): vale la prima, come Indice.bomDi.
	if g := propostabom.GruppiDelProdotto(prodotto(p1, true), []ancoraggio.StrutturaProdotto{bomDi(p1, allB, shaX, r1), bomDi(p1, allC, shaY, rY)}); len(g) != 1 ||
		g[0].AllegatoID == nil || *g[0].AllegatoID != allB || g[0].Sha256 != shaX || g[0].Radice != r1 {
		t.Errorf("due BOM di lavoro dello stesso prodotto: %+v", g)
	}
}

// TestT09GruppoCorrenteDellaSorgente (T-09; emendamento di V1 §6.4.1): i quattro esiti e i tre motivi di assente, con due
// copie dello stesso STEP, due prodotti con la stessa radice candidata, le occorrenze non attive, il ripiego per chiave
// (anche con la chiave «bom», che non lo ammette), il passaggio inverso da «bom» alla candidata e la BOM di lavoro con
// un'altra struttura (un'altra radice, un altro file) per una bozza con la radice. La sostituzione dello STEP con le
// strutture calcolate da Calcola sta in valutazione (gruppo_corrente_sostituzione_test.go).
func TestT09GruppoCorrenteDellaSorgente(t *testing.T) {
	sorgenteA := propostabom.SorgenteBozza{AllegatoID: allA, Sha256: shaX, Radice: r1, Gruppo: propostabom.ChiaveCandidata(allA, r1)}
	conGruppo := func(s propostabom.SorgenteBozza, gruppo, radice string) propostabom.SorgenteBozza {
		s.Gruppo, s.Radice = gruppo, radice
		return s
	}
	casi := []struct {
		nome      string
		pv        valutazione.ProdottoValutato
		strutture []ancoraggio.StrutturaProdotto
		nonAttivi propostabom.NonAttivi
		s         propostabom.SorgenteBozza
		want      propostabom.GruppoCorrente
	}{
		{"la candidata della stessa occorrenza", prodotto(p1, false, candidata(allA, shaX, r1)), nil, nil, sorgenteA,
			propostabom.GruppoCorrente{Chiave: propostabom.ChiaveCandidata(allA, r1), Come: "stessa_occorrenza"}},
		{"la BOM di lavoro della stessa occorrenza (dopo «Autorizza»)", prodotto(p1, true), []ancoraggio.StrutturaProdotto{bomDi(p1, allA, shaX, r1)}, nil,
			sorgenteA, propostabom.GruppoCorrente{Chiave: "bom", Come: "stessa_occorrenza"}},
		{"la BOM di lavoro sulla copia portatrice", prodotto(p1, true), []ancoraggio.StrutturaProdotto{bomDi(p1, allB, shaX, r1)}, nil, sorgenteA,
			propostabom.GruppoCorrente{Chiave: "bom", Come: "altra_occorrenza", AllegatoID: uidP(allB)}},
		{"nessun gruppo: un altro STEP", prodotto(p1, false, candidata(allC, shaY, rY)), nil, nil, sorgenteA,
			propostabom.GruppoCorrente{Come: "assente", Motivo: "nessun_gruppo"}},
		{"nessun gruppo: un'altra radice dello stesso file", prodotto(p1, false, candidata(allA, shaX, r7)), nil, nil, sorgenteA,
			propostabom.GruppoCorrente{Come: "assente", Motivo: "nessun_gruppo"}},
		{"una candidata dell'altra copia non vale mai", prodotto(p1, false, candidata(allB, shaX, r1)), nil, nil, sorgenteA,
			propostabom.GruppoCorrente{Come: "assente", Motivo: "nessun_gruppo"}},
		{"la candidata della propria copia prima della BOM sull'altra", prodotto(p1, true, candidata(allA, shaX, r1)),
			[]ancoraggio.StrutturaProdotto{bomDi(p1, allB, shaX, r1)}, nil, sorgenteA,
			propostabom.GruppoCorrente{Chiave: propostabom.ChiaveCandidata(allA, r1), Come: "stessa_occorrenza"}},
		{"il passaggio inverso: un registro su «bom», la struttura di nuovo candidata (dopo «Revoca»)", prodotto(p1, false, candidata(allA, shaX, r1)), nil, nil,
			conGruppo(sorgenteA, "bom", r1), propostabom.GruppoCorrente{Chiave: propostabom.ChiaveCandidata(allA, r1), Come: "stessa_occorrenza"}},
		{"la BOM sul portatore in Altri file", prodotto(p1, true), []ancoraggio.StrutturaProdotto{bomDi(p1, allB, shaX, r1)},
			propostabom.NonAttivi{allB: "altri_file"}, sorgenteA, propostabom.GruppoCorrente{Come: "assente", AllegatoID: uidP(allB), Motivo: "occorrenza_non_attiva"}},
		{"la BOM sulla stessa occorrenza scartata", prodotto(p1, true), []ancoraggio.StrutturaProdotto{bomDi(p1, allA, shaX, r1)},
			propostabom.NonAttivi{allA: "scartato"}, sorgenteA, propostabom.GruppoCorrente{Come: "assente", AllegatoID: uidP(allA), Motivo: "occorrenza_non_attiva"}},
		// P-4 (b) dell'ondata A di V1: la candidata della propria occorrenza non attiva dà occorrenza_non_attiva, con
		// l'allegato; una candidata non attiva di un'altra occorrenza no.
		{"la candidata della propria occorrenza scartata (P-4 b)", prodotto(p1, false, candidata(allA, shaX, r1)), nil, propostabom.NonAttivi{allA: "scartato"},
			sorgenteA, propostabom.GruppoCorrente{Come: "assente", AllegatoID: uidP(allA), Motivo: "occorrenza_non_attiva"}},
		{"la candidata della propria occorrenza in Altri file (P-4 b)", prodotto(p1, false, candidata(allA, shaX, r1)), nil,
			propostabom.NonAttivi{allA: "altri_file"}, sorgenteA, propostabom.GruppoCorrente{Come: "assente", AllegatoID: uidP(allA), Motivo: "occorrenza_non_attiva"}},
		{"la candidata non attiva dell'altra copia non vale", prodotto(p1, false, candidata(allB, shaX, r1)), nil, propostabom.NonAttivi{allB: "scartato"},
			sorgenteA, propostabom.GruppoCorrente{Come: "assente", Motivo: "nessun_gruppo"}},
		{"la candidata della propria occorrenza scartata con un'altra radice non vale", prodotto(p1, false, candidata(allA, shaX, r7)), nil,
			propostabom.NonAttivi{allA: "scartato"}, sorgenteA, propostabom.GruppoCorrente{Come: "assente", Motivo: "nessun_gruppo"}},
		{"la candidata scartata e la BOM attiva sul portatore", prodotto(p1, true, candidata(allA, shaX, r1)),
			[]ancoraggio.StrutturaProdotto{bomDi(p1, allB, shaX, r1)}, propostabom.NonAttivi{allA: "scartato"}, sorgenteA,
			propostabom.GruppoCorrente{Chiave: "bom", Come: "altra_occorrenza", AllegatoID: uidP(allB)}},
		{"il ripiego per chiave: la chiave della candidata, con lo sha256 della sorgente", prodotto(p1, false, candidata(allA, shaX, r1)), nil, nil,
			conGruppo(sorgenteA, propostabom.ChiaveCandidata(allA, r1), ""),
			propostabom.GruppoCorrente{Chiave: propostabom.ChiaveCandidata(allA, r1), Come: "stessa_occorrenza"}},
		{"il ripiego per chiave, con la candidata e la BOM sul portatore: vale la candidata", prodotto(p1, true, candidata(allA, shaX, r1)),
			[]ancoraggio.StrutturaProdotto{bomDi(p1, allB, shaX, r1)}, propostabom.NonAttivi{}, conGruppo(sorgenteA, propostabom.ChiaveCandidata(allA, r1), ""),
			propostabom.GruppoCorrente{Chiave: propostabom.ChiaveCandidata(allA, r1), Come: "stessa_occorrenza"}},
		{"il ripiego per chiave: la chiave «bom» non lo ammette, anche con un «bom» dello stesso sha256", prodotto(p1, true),
			[]ancoraggio.StrutturaProdotto{bomDi(p1, allA, shaX, r7)}, nil, conGruppo(sorgenteA, "bom", ""),
			propostabom.GruppoCorrente{Come: "assente", Motivo: "senza_radice"}},
		{"il ripiego per chiave: una chiave che non c'è più", prodotto(p1, true), []ancoraggio.StrutturaProdotto{bomDi(p1, allA, shaX, r1)}, nil,
			conGruppo(sorgenteA, propostabom.ChiaveCandidata(allA, r1), ""), propostabom.GruppoCorrente{Come: "assente", Motivo: "senza_radice"}},
		{"il ripiego per chiave: la candidata con un altro sha256", prodotto(p1, false, candidata(allA, shaY, r1)), nil, nil,
			conGruppo(sorgenteA, propostabom.ChiaveCandidata(allA, r1), ""), propostabom.GruppoCorrente{Come: "assente", Motivo: "senza_radice"}},
		{"il ripiego per chiave: la candidata su un'occorrenza non attiva non c'è", prodotto(p1, false, candidata(allA, shaX, r1)), nil,
			propostabom.NonAttivi{allA: "altri_file"}, conGruppo(sorgenteA, propostabom.ChiaveCandidata(allA, r1), ""),
			propostabom.GruppoCorrente{Come: "assente", Motivo: "senza_radice"}},
		{"il ripiego per chiave: senza la chiave", prodotto(p1, false, candidata(allA, shaX, r1)), nil, nil, conGruppo(sorgenteA, "", ""),
			propostabom.GruppoCorrente{Come: "assente", Motivo: "senza_radice"}},
		{"con la radice (un registro) mai senza_radice, anche con la chiave «bom»", prodotto(p1, false), nil, nil, conGruppo(sorgenteA, "bom", r1),
			propostabom.GruppoCorrente{Come: "assente", Motivo: "nessun_gruppo"}},
		// La BOM di lavoro con un'altra struttura, per una bozza con la radice (un registro, il percorso principale di V1):
		// la regola confronta sha256 e radice, mai solo lo sha256 o solo la presenza di un «bom» (EM §6.4.1, passi 2-3; il
		// rischio che il paragrafo del ripiego nomina). Sempre assente con nessun_gruppo, mai altra_occorrenza.
		{"una nuova autorizzazione dello stesso STEP con un'altra radice, sulla stessa occorrenza", prodotto(p1, true),
			[]ancoraggio.StrutturaProdotto{bomDi(p1, allA, shaX, r7)}, nil, conGruppo(sorgenteA, "bom", r1),
			propostabom.GruppoCorrente{Come: "assente", Motivo: "nessun_gruppo"}},
		{"una nuova autorizzazione dello stesso STEP con un'altra radice, sul portatore", prodotto(p1, true),
			[]ancoraggio.StrutturaProdotto{bomDi(p1, allB, shaX, r7)}, nil, conGruppo(sorgenteA, "bom", r1),
			propostabom.GruppoCorrente{Come: "assente", Motivo: "nessun_gruppo"}},
		{"la BOM del file nuovo dopo la sostituzione (risposta «sì»)", prodotto(p1, true),
			[]ancoraggio.StrutturaProdotto{bomDi(p1, allC, shaY, rY)}, nil, conGruppo(sorgenteA, "bom", r1),
			propostabom.GruppoCorrente{Come: "assente", Motivo: "nessun_gruppo"}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			prima, _ := json.Marshal([]any{c.pv, c.strutture, c.nonAttivi, c.s})
			got := propostabom.GruppoCorrenteDelProdotto(c.pv, c.strutture, c.nonAttivi, c.s)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("gruppo corrente %+v (allegato %v), atteso %+v (allegato %v)", got, got.AllegatoID, c.want, c.want.AllegatoID)
			}
			if di := propostabom.GruppoCorrenteDelProdotto(c.pv, c.strutture, c.nonAttivi, c.s); !reflect.DeepEqual(di, got) {
				t.Error("due chiamate danno esiti diversi")
			}
			if dopo, _ := json.Marshal([]any{c.pv, c.strutture, c.nonAttivi, c.s}); string(dopo) != string(prima) {
				t.Error("la funzione ha cambiato gli ingressi")
			}
		})
	}
}

// TestT09DueProdottiConLaStessaRadice (OV-10): la chiave non contiene il prodotto, e la stessa radice candidata per due
// prodotti dà la stessa stringa. Per la bozza di P1 conta solo P1: la sua candidata se c'è; con la sola BOM di P2
// sull'allegato, assente.
func TestT09DueProdottiConLaStessaRadice(t *testing.T) {
	s := propostabom.SorgenteBozza{AllegatoID: allA, Sha256: shaX, Radice: r1, Gruppo: propostabom.ChiaveCandidata(allA, r1)}
	strutture := []ancoraggio.StrutturaProdotto{bomDi(p2, allA, shaX, r1)}
	pv1, pv2 := prodotto(p1, false, candidata(allA, shaX, r1)), prodotto(p2, true, candidata(allA, shaX, r1))
	if propostabom.GruppiDelProdotto(pv1, strutture)[0].Chiave != propostabom.GruppiDelProdotto(pv2, strutture)[1].Chiave {
		t.Fatal("la stessa radice candidata per due prodotti deve dare la stessa chiave")
	}
	if got := propostabom.GruppoCorrenteDelProdotto(pv1, strutture, nil, s); got.Come != "stessa_occorrenza" || got.Chiave != s.Gruppo {
		t.Errorf("P1 con la sua candidata: %+v", got)
	}
	if got := propostabom.GruppoCorrenteDelProdotto(prodotto(p1, false), strutture, nil, s); got.Come != "assente" || got.Motivo != "nessun_gruppo" || got.Chiave != "" {
		t.Errorf("P1 senza gruppi, con la sola BOM di P2 sull'allegato: %+v", got)
	}
	if got := propostabom.GruppoCorrenteDelProdotto(prodotto(p1, true), strutture, nil, s); got.Come != "assente" {
		t.Errorf("P1 con la sua BOM di lavoro senza struttura e la BOM di P2: %+v", got)
	}
}

// TestT09NessunaChiaveSiScompone: la regola guarda allegato, sha256 e radice dei gruppi, mai la loro chiave. Una
// candidata con una chiave opaca si ritrova, con quella chiave; un gruppo la cui chiave «dice» l'occorrenza e la radice
// della sorgente, ma con altri dati, non vale.
func TestT09NessunaChiaveSiScompone(t *testing.T) {
	s := propostabom.SorgenteBozza{AllegatoID: allA, Sha256: shaX, Radice: r1}
	opaca := []propostabom.Gruppo{{Chiave: "opaca-1", Tipo: propostabom.GruppoCandidata, AllegatoID: uidP(allA), Sha256: shaX, Radice: r1}}
	if got := propostabom.GruppoCorrenteDellaSorgente(opaca, nil, s); got.Chiave != "opaca-1" || got.Come != "stessa_occorrenza" {
		t.Errorf("la candidata con una chiave opaca: %+v", got)
	}
	ingannevole := []propostabom.Gruppo{{Chiave: propostabom.ChiaveCandidata(allA, r1), Tipo: propostabom.GruppoCandidata, AllegatoID: uidP(allC),
		Sha256: shaY, Radice: rY}}
	if got := propostabom.GruppoCorrenteDellaSorgente(ingannevole, nil, s); got.Come != "assente" || got.Motivo != "nessun_gruppo" {
		t.Errorf("una chiave che dice la sorgente con altri dati: %+v", got)
	}
}

// TestGruppoCorrenteIlJSON: la forma {chiave?, come, allegato_id?, motivo?}, con i soli campi di ogni esito.
func TestGruppoCorrenteIlJSON(t *testing.T) {
	for _, c := range []struct {
		g    propostabom.GruppoCorrente
		want string
	}{
		{propostabom.GruppoCorrente{Chiave: "bom", Come: "stessa_occorrenza"}, `{"chiave":"bom","come":"stessa_occorrenza"}`},
		{propostabom.GruppoCorrente{Chiave: "bom", Come: "altra_occorrenza", AllegatoID: uidP(allB)},
			`{"chiave":"bom","come":"altra_occorrenza","allegato_id":"` + allB.String() + `"}`},
		{propostabom.GruppoCorrente{Come: "assente", AllegatoID: uidP(allB), Motivo: "occorrenza_non_attiva"},
			`{"come":"assente","allegato_id":"` + allB.String() + `","motivo":"occorrenza_non_attiva"}`},
		{propostabom.GruppoCorrente{Come: "assente", Motivo: "senza_radice"}, `{"come":"assente","motivo":"senza_radice"}`},
	} {
		if b, err := json.Marshal(c.g); err != nil || string(b) != c.want {
			t.Errorf("%s, atteso %s (%v)", b, c.want, err)
		}
	}
}
