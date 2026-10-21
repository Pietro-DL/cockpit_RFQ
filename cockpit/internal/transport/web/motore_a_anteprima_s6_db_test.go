//go:build integrazione

// L4 — direttiva 39, corsia motore: S6 sulla rotta HTTP, GET /thread/{id}/sorgente/step/{allegato}/anteprima, con il
// calcolo vero del Motore A (regole ACME, scena B87 di motore_a_db_test.go) e le sessioni vere:
//   - i quattro stati di struttura_step: lo STEP del prodotto con la struttura (con_struttura, i figli, la partenza step,
//     la stessa impronta_contenuto dei gruppi di /dati), uno STEP con la sola radice riconosciuta (sola_radice, zero
//     figli, partenza step sul vero gruppo: mai mono_pezzo), uno STEP senza una struttura per il prodotto
//     (nessuna_struttura, partenza solo_prodotto) e uno STEP senza fatti (analisi non pronta: non_determinabile, presa
//     d'atto richiesta, mai una prova geometrica);
//   - la fonte di /dati sullo STEP del prodotto dice lo stesso caso (la stessa funzione) e la conferma ammessa è S8;
//   - ?bozza=: l'effetto del cambio di fonte sulla vista della bozza (distintabozza.VistaDellaBozza): una bozza manuale con
//     il solo prodotto, senza sorgente, riceve l'effetto verso lo STEP (nessuna «stessa sorgente»);
//   - i rifiuti: 400 (prodotto mancante), 422 NON_STEP (il PDF), FUORI_RFQ (un prodotto che non è un finito, uno STEP di
//     un'altra RFQ, una bozza che non è del prodotto), il permesso (solo l'amministratore), il flag spento (404);
//   - nessuna scrittura.
//
// I codici 777xxxxx sono quelli della scena B87; il resto è inventato.
package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

type rispostaS6Prova struct {
	AllegatoID string `json:"allegato_id"`
	ProdottoID string `json:"prodotto_id"`
	Stato      string `json:"stato"`
	Gruppi     []struct {
		Gruppo            string `json:"gruppo"`
		InVista           bool   `json:"in_vista"`
		Nodi              int    `json:"nodi"`
		ImprontaContenuto string `json:"impronta_contenuto"`
	} `json:"gruppi"`
	StrutturaSTEP     propostabom.StrutturaSTEP `json:"struttura_step"`
	EffettoSullaBozza *struct {
		StessaSorgente bool     `json:"stessa_sorgente"`
		NodiCheEntrano []string `json:"nodi_che_entrano"`
	} `json:"effetto_sulla_bozza"`
	Codice string `json:"codice"`
	Testo  string `json:"testo"`
}

func TestL4V1D39S6IQuattroStatiDelloSTEP(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.mwScena(t, "MS6")
	s6 := func(allegato, prodotto uuid.UUID, extra string) string {
		return "/thread/" + s.thread.String() + "/sorgente/step/" + allegato.String() + "/anteprima?prodotto=" + prodotto.String() + extra
	}
	leggi := func(w *browser, percorso string, atteso int) rispostaS6Prova {
		t.Helper()
		resp, corpo := w.fai(http.MethodGet, percorso, nil, false)
		if resp.StatusCode != atteso || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("GET %s: %d (atteso %d) %q: %.600s", percorso, resp.StatusCode, atteso, resp.Header.Get("Cache-Control"), corpo)
		}
		var x rispostaS6Prova
		if err := json.Unmarshal([]byte(corpo), &x); err != nil {
			t.Fatalf("%v: %.300s", err, corpo)
		}
		return x
	}

	// a flag spento: 404
	spente := s.regole
	spente.Accesa, spente.Stato = false, motorea_web.StatoSpenta
	b.ws.MotoreA = spente
	if resp, _ := s.ad.fai(http.MethodGet, s6(s.allStep, s.prodotto, ""), nil, false); resp.StatusCode != http.StatusNotFound {
		t.Errorf("flag spento: %d", resp.StatusCode)
	}
	mwAccendi(t, b, s.regole)

	// uno STEP della RFQ senza una struttura per il prodotto (la radice è un altro codice)
	altro, shaA := s.allegatoExt("ALTRO_99990001.stp", "stp")
	s.analisiCorrente(shaA, fattiDi("#1", []string{"#1=99990001", "#2=99990002"}, []string{"#1>#2*1"}))

	prima := fotoDelDatabase(t, b)

	// con la struttura: lo STEP del prodotto
	x := leggi(s.ad, s6(s.allStep, s.prodotto, ""), 200)
	if x.StrutturaSTEP.StatoAnalisi != propostabom.AnalisiPronta || x.StrutturaSTEP.Struttura != propostabom.StrutturaConFigli ||
		x.StrutturaSTEP.FigliEstratti < 1 || x.StrutturaSTEP.Partenza != propostabom.PartenzaStep || x.StrutturaSTEP.PresaDAttoRichiesta ||
		x.Stato != propostabom.StatoAnteprimaPronta || len(x.Gruppi) == 0 || x.ProdottoID != s.prodotto.String() {
		t.Errorf("STEP con la struttura: %+v", x)
	}
	impronte := map[string]string{}
	gruppo := ""
	for _, g := range x.Gruppi {
		impronte[g.Gruppo] = g.ImprontaContenuto
		if gruppo == "" && g.ImprontaContenuto != "" {
			gruppo = g.Gruppo
		}
	}
	// la stessa impronta dei gruppi di /dati e la stessa struttura nella fonte
	_, corpo := s.ad.fai(http.MethodGet, s.dati(), nil, false)
	var d struct {
		Gruppi []struct {
			Chiave            string `json:"chiave"`
			ImprontaContenuto string `json:"impronta_contenuto"`
		} `json:"gruppi"`
		Fonte *struct {
			ConfermaAmmessa bool                       `json:"conferma_ammessa"`
			Gesto           string                     `json:"gesto"`
			StrutturaSTEP   *propostabom.StrutturaSTEP `json:"struttura_step"`
		} `json:"fonte"`
	}
	if err := json.Unmarshal([]byte(corpo), &d); err != nil {
		t.Fatal(err)
	}
	uguali := 0
	for _, g := range d.Gruppi {
		if v, ok := impronte[g.Chiave]; ok {
			if v != g.ImprontaContenuto {
				t.Errorf("gruppo %s: impronta S6 %s, /dati %s", g.Chiave, v, g.ImprontaContenuto)
			}
			uguali++
		}
	}
	if uguali == 0 {
		t.Errorf("nessun gruppo di S6 fra quelli di /dati: %v / %+v", impronte, d.Gruppi)
	}
	if d.Fonte == nil || !d.Fonte.ConfermaAmmessa || d.Fonte.Gesto != motorea_web.GestoConfermaSorgente || d.Fonte.StrutturaSTEP == nil ||
		d.Fonte.StrutturaSTEP.Struttura != x.StrutturaSTEP.Struttura || d.Fonte.StrutturaSTEP.FigliEstratti != x.StrutturaSTEP.FigliEstratti {
		t.Errorf("la fonte di /dati: %+v (S6 %+v)", d.Fonte, x.StrutturaSTEP)
	}

	// nessuna struttura per il prodotto: la partenza solo_prodotto
	x = leggi(s.ad, s6(altro, s.prodotto, ""), 200)
	t.Logf("nessuna struttura: %+v", x)
	if x.StrutturaSTEP.StatoAnalisi != propostabom.AnalisiPronta || x.StrutturaSTEP.Struttura != propostabom.StrutturaNessuna ||
		x.StrutturaSTEP.Partenza != propostabom.PartenzaSoloProdotto || x.StrutturaSTEP.PresaDAttoRichiesta {
		t.Errorf("STEP senza struttura per il prodotto: %+v", x)
	}

	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("S6 ha scritto: %s", d)
	}

	// ?bozza=: una bozza manuale con il solo prodotto, senza sorgente
	em := distintabozza.EmittenteCliente(uuidSQL(t, b, `SELECT cliente_id FROM thread_offerta WHERE thread_id = $1`, s.thread))
	s.s9Bozza3(t, s.prodotto, map[string]any{"formato": 3, "prodotto_id": s.prodotto.String(),
		"nodi": []any{s9Nodo("k-p", &s.prodotto, "finito", "77722757", em)}, "archi": []any{}, "decisioni": []any{}, "documenti_preassociati": []any{}}, "")
	bozza := uuidSQL(t, b, `SELECT bozza_id FROM distinta_bozza WHERE thread_id = $1 AND prodotto_id = $2 AND stato = 'aperta'`, s.thread, s.prodotto)
	prima = fotoDelDatabase(t, b)
	x = leggi(s.ad, s6(s.allStep, s.prodotto, "&gruppo="+url.QueryEscape(gruppo)+"&bozza="+bozza.String()), 200)
	if x.EffettoSullaBozza == nil || x.EffettoSullaBozza.StessaSorgente || len(x.EffettoSullaBozza.NodiCheEntrano) == 0 {
		t.Errorf("l'effetto sulla bozza manuale: %+v", x.EffettoSullaBozza)
	}

	// i rifiuti
	if x := leggi(s.ad, s6(s.allDisegno, s.prodotto, ""), 422); x.Codice != "NON_STEP" {
		t.Errorf("il PDF: %+v", x)
	}
	if x := leggi(s.ad, s6(s.allStep, s.assieme, ""), 422); x.Codice != distintabozza.CodiceFuoriRFQ {
		t.Errorf("un prodotto che non è un finito: %+v", x)
	}
	if x := leggi(s.ad, s6(uuid.New(), s.prodotto, ""), 422); x.Codice != distintabozza.CodiceFuoriRFQ {
		t.Errorf("uno STEP che non è della RFQ: %+v", x)
	}
	if x := leggi(s.ad, s6(s.allStep, s.prodotto, "&bozza="+uuid.New().String()), 422); x.Codice != distintabozza.CodiceFuoriRFQ {
		t.Errorf("una bozza che non c'è: %+v", x)
	}
	if resp, _ := s.ad.fai(http.MethodGet, "/thread/"+s.thread.String()+"/sorgente/step/"+s.allStep.String()+"/anteprima", nil, false); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("senza prodotto: %d", resp.StatusCode)
	}
	for nome, w := range map[string]*browser{"operatore": s.op, "consultazione": s.co} {
		if resp, _ := w.fai(http.MethodGet, s6(s.allStep, s.prodotto, ""), nil, false); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: %d, atteso 403", nome, resp.StatusCode)
		}
	}
	if resp, _ := s.ad.fai(http.MethodPost, s6(s.allStep, s.prodotto, ""), nil, false); resp.StatusCode != http.StatusMethodNotAllowed &&
		resp.StatusCode != http.StatusForbidden {
		t.Errorf("POST: %d, atteso 405 (o 403 del CSRF)", resp.StatusCode)
	}
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("S6 ha scritto: %s", d)
	}

	// la sola radice riconosciuta (lo STEP del prodotto riletto con la sola radice): il vero gruppo, zero figli, mai
	// mono_pezzo
	s.analisiCorrente(s.shaDoc(s.step), fattiDi("#1", []string{"#1=77722757"}, nil))
	x = leggi(s.ad, s6(s.allStep, s.prodotto, ""), 200)
	t.Logf("sola radice: %+v", x)
	if x.StrutturaSTEP.Struttura != propostabom.StrutturaSolaRadice || x.StrutturaSTEP.FigliEstratti != 0 || x.StrutturaSTEP.Partenza != propostabom.PartenzaStep ||
		x.StrutturaSTEP.PresaDAttoRichiesta || !strings.Contains(x.StrutturaSTEP.Testo, "non ha estratto figli") {
		t.Errorf("STEP con la sola radice: %+v", x)
	}

	// senza fatti: l'analisi non è pronta, il caso non è determinabile e la partenza vuole la presa d'atto
	s.esegui(`DELETE FROM analisi_fatti WHERE sha256 = $1`, s.shaDoc(s.step))
	x = leggi(s.ad, s6(s.allStep, s.prodotto, ""), 200)
	t.Logf("senza fatti: %+v", x)
	if x.StrutturaSTEP.Struttura != propostabom.StrutturaNonDeterminabile || !x.StrutturaSTEP.PresaDAttoRichiesta ||
		x.StrutturaSTEP.Partenza != propostabom.PartenzaSoloProdotto || x.StrutturaSTEP.StatoAnalisi == propostabom.AnalisiPronta {
		t.Errorf("STEP senza fatti: %+v", x)
	}
	if x.StrutturaSTEP.StatoAnalisi == propostabom.AnalisiInCorso && x.Stato != propostabom.StatoAnteprimaInAnalisi {
		t.Errorf("con l'analisi in corso lo stato è in_analisi: %+v", x)
	}
}
