//go:build integrazione

// L4 — direttiva 39 §3 e §8 (punto 2), corsia motore, rilievo della revisione: S6 `?bozza=` su una bozza manuale (senza
// sorgente) mostra l'effetto di cambio-fonte-2, lo stesso che S8 scrive poi sulla stessa bozza, sulla stessa scena (la
// scena B87 di motore_a_db_test.go con il calcolo vero del Motore A e le regole ACME, le sessioni vere):
//   - lo STEP del prodotto è riletto con due figli dai codici della grammatica ACME (le voci hanno il codice proposto);
//   - la bozza manuale (R2 crea vuota, poi un salvataggio R2) ha il prodotto, l'assieme riusato senza casella (registrato) e
//     due nodi manuali senza casella: uno
//     con l'identità di una voce dello STEP (codice, rev, emittente), uno con lo stesso codice di un'altra voce ma un'altra
//     rev (identità non sostenuta);
//   - S6 dice algoritmo cambio-fonte-2, gli abbinati, gli abbinamenti_proposti[] (fuori da da_rivedere[]), le caselle
//     acquisite (allegato_prima null), le voci che entrano senza i doppioni degli abbinati;
//   - S8 (la rotta vera, partenza step, con la bozza dichiarata) scrive la versione 3 della stessa bozza: l'evento
//     sorgente_cambiata ha lo stesso algoritmo, gli stessi abbinamenti_proposti[], lo stesso numero di abbinati, le stesse
//     caselle riscritte e le stesse voci nuove; nella versione i nodi abbinati tengono la chiave e hanno la casella dello
//     STEP, nessuna casella è di due nodi (niente doppioni), il nodo proposto non entra come doppione;
//   - S6 non scrive.
//
// I codici 777xxxxx sono quelli della scena B87; il resto è inventato.
package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
)

func TestL4V1D39S6RiconciliaComeS8(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.mwScena(t, "MS6R")
	mwAccendi(t, b, s.regole)
	// lo STEP del prodotto riletto con due figli che la grammatica ACME della scena legge come codici (le voci hanno un
	// codice proposto, come in una RFQ vera): la radice resta il prodotto
	s.analisiCorrente(s.shaDoc(s.step), fattiDi("#1", []string{"#1=77722757", "#2=9120101A", "#3=9130202A"}, []string{"#1>#2*2", "#1>#3*1"}))
	s6 := func(extra string) string {
		return "/thread/" + s.thread.String() + "/sorgente/step/" + s.allStep.String() + "/anteprima?prodotto=" + s.prodotto.String() + extra
	}
	get := func(percorso string) map[string]json.RawMessage {
		t.Helper()
		resp, corpo := s.ad.fai(http.MethodGet, percorso, nil, false)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %d %.800s", percorso, resp.StatusCode, corpo)
		}
		var x map[string]json.RawMessage
		if err := json.Unmarshal([]byte(corpo), &x); err != nil {
			t.Fatalf("%v: %.300s", err, corpo)
		}
		return x
	}

	// la proposta dello STEP senza bozza: il primo gruppo che si importa e le voci del suo registro
	x := get(s6(""))
	var gruppi []struct {
		Gruppo            string `json:"gruppo"`
		ImprontaContenuto string `json:"impronta_contenuto"`
	}
	if err := json.Unmarshal(x["gruppi"], &gruppi); err != nil {
		t.Fatal(err)
	}
	gruppo := ""
	for _, g := range gruppi {
		if gruppo == "" && g.ImprontaContenuto != "" {
			gruppo = g.Gruppo
		}
	}
	x = get(s6("&gruppo=" + url.QueryEscape(gruppo)))
	var proposta struct {
		Gruppo            string `json:"gruppo"`
		ImprontaContenuto string `json:"impronta_contenuto"`
		Contenuto         struct {
			Proposta struct {
				Nodi []struct {
					Voce             string `json:"voce"`
					CodiceProposto   string `json:"codice_proposto"`
					RevProposta      string `json:"rev_proposta"`
					IdentitaProposta struct {
						Emittente string `json:"emittente"`
						Namespace string `json:"namespace"`
					} `json:"identita_proposta"`
					Origine struct {
						Rif string `json:"rif"`
					} `json:"origine"`
				} `json:"nodi"`
			} `json:"proposta"`
			Gruppo struct {
				Radice string `json:"radice"`
			} `json:"gruppo"`
		} `json:"contenuto"`
	}
	if err := json.Unmarshal(x["proposta"], &proposta); err != nil || proposta.Gruppo == "" {
		t.Fatalf("la proposta di S6: %v %s", err, x["proposta"])
	}
	// due voci dello STEP che non sono la radice, con il codice proposto
	type voce struct{ codice, rev, emittente, namespace, rif string }
	var voci []voce
	for _, v := range proposta.Contenuto.Proposta.Nodi {
		if v.CodiceProposto == "" || v.Origine.Rif == proposta.Contenuto.Gruppo.Radice {
			continue
		}
		voci = append(voci, voce{v.CodiceProposto, v.RevProposta, v.IdentitaProposta.Emittente, v.IdentitaProposta.Namespace, v.Origine.Rif})
	}
	sort.Slice(voci, func(i, j int) bool { return voci[i].rif < voci[j].rif })
	t.Logf("voci dello STEP: %+v", voci)
	if len(voci) < 2 {
		t.Logf("i nodi della proposta: %+v, radice %q", proposta.Contenuto.Proposta.Nodi, proposta.Contenuto.Gruppo.Radice)
	}
	if len(voci) < 2 {
		t.Fatalf("la scena vuole almeno due voci con il codice proposto: %+v", voci)
	}
	manuale := func(chiave string, v voce, rev string) map[string]any {
		// il namespace e la presunzione dell'emittente li calcola il server (il modo manuale: presunto)
		id := map[string]any{"emittente": v.emittente, "emittente_presunto": true, "codice": v.codice, "modo": "manuale"}
		if rev != "" {
			id["rev"] = rev
		}
		return map[string]any{"chiave_bozza": chiave, "op": "crea", "tipo": "sciolto", "origine": "utente", "identita": id}
	}
	altraRev := voci[1].rev + "Z"
	em := distintabozza.EmittenteCliente(uuidSQL(t, b, `SELECT cliente_id FROM thread_offerta WHERE thread_id = $1`, s.thread))
	// la bozza manuale come la fa la pagina: R2 crea con la partenza vuota (il solo prodotto), poi un salvataggio R2 con
	// l'assieme della working riusato, i due nodi manuali e gli archi dell'ingegnere
	kp, ka := distintabozza.ChiaveDelComponente(s.prodotto), distintabozza.ChiaveDelComponente(s.assieme)
	c, err := distintabozza.CreaFormato3(b.ctx, b.pool, distintabozza.Creazione3{ThreadID: s.thread, UtenteID: s.utente,
		Comune: distintabozza.ComuneR2{ProdottoID: s.prodotto, ChiaveSalvataggio: uuid.New(), ImprontaRichiesta: strings.Repeat("a", 64)},
		Crea:   distintabozza.CreaR2{Partenza: distintabozza.PartenzaVuota}})
	if err != nil {
		t.Fatalf("crea vuota: %v", err)
	}
	bozza := c.BozzaID
	var snap1 string
	if err := b.pool.QueryRow(b.ctx, `SELECT snapshot::text FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = 1`, bozza).Scan(&snap1); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(snap1), &m); err != nil {
		t.Fatal(err)
	}
	assieme := s9Nodo(ka, &s.assieme, "sottoassieme", "77720517", em)
	mOk, mRev := manuale("m-ok", voci[0], voci[0].rev), manuale("m-rev", voci[1], altraRev)
	arco := func(figlio string, q int) map[string]any {
		return map[string]any{"padre": kp, "figlio": figlio, "qta": q, "origine": "utente"}
	}
	aa, aOk, aRev := arco(ka, 2), arco("m-ok", 1), arco("m-rev", 1)
	m["nodi"] = append(m["nodi"].([]any), assieme, mOk, mRev)
	m["archi"] = append(m["archi"].([]any), aa, aOk, aRev)
	az := func(verbo, chiave string, dopo any) map[string]any {
		return map[string]any{"azione": verbo, "chiave_bozza": chiave, "prima": nil, "dopo": dopo, "origine": "utente"}
	}
	raw, err := json.Marshal(map[string]any{"bozza_id": bozza, "versione_attesa": 1, "prodotto_id": s.prodotto, "vocabolario": 2,
		"chiave_salvataggio": uuid.New(), "snapshot": m, "azioni": []any{az("nodo_aggiunto", ka, assieme), az("nodo_aggiunto", "m-ok", mOk),
			az("nodo_aggiunto", "m-rev", mRev), az("arco_aggiunto", ka, aa), az("arco_aggiunto", "m-ok", aOk), az("arco_aggiunto", "m-rev", aRev)}})
	if err != nil {
		t.Fatal(err)
	}
	r2, err := distintabozza.LeggiRichiestaR2(raw)
	if err != nil {
		t.Fatalf("corpo di R2: %v", err)
	}
	sv := r2.Salvataggio
	sv.ThreadID, sv.UtenteID = s.thread, s.utente
	if _, err := distintabozza.SalvaFormato3(b.ctx, b.pool, sv); err != nil {
		t.Fatalf("il salvataggio della bozza manuale: %v", err)
	}

	// S6 sulla bozza manuale: cambio-fonte-2, senza scrivere
	prima := fotoDelDatabase(t, b)
	x = get(s6("&gruppo=" + url.QueryEscape(proposta.Gruppo) + "&bozza=" + bozza.String()))
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("S6 ha scritto: %s", d)
	}
	var eff struct {
		Algoritmo        string   `json:"algoritmo"`
		NodiCheEntrano   []string `json:"nodi_che_entrano"`
		NuoviNodi        []string `json:"nuovi_nodi_proposti"`
		CaselleRiscritte []struct {
			ChiaveBozza string     `json:"chiave_bozza"`
			DecisioneID string     `json:"decisione_id"`
			Rif         string     `json:"rif"`
			Prima       *uuid.UUID `json:"allegato_prima"`
		} `json:"caselle_riscritte"`
		Abbinati []struct {
			ChiaveBozza string `json:"chiave_bozza"`
			Voce        string `json:"voce"`
			Rif         string `json:"rif"`
		} `json:"abbinati"`
		AbbinamentiProposti json.RawMessage `json:"abbinamenti_proposti"`
		DaRivedere          json.RawMessage `json:"da_rivedere"`
		IdentitaAggiornate  []struct {
			ChiaveBozza string `json:"chiave_bozza"`
		} `json:"identita_aggiornate"`
	}
	if err := json.Unmarshal(x["effetto_sulla_bozza"], &eff); err != nil {
		t.Fatalf("effetto_sulla_bozza: %v %s", err, x["effetto_sulla_bozza"])
	}
	t.Logf("S6: %s", x["effetto_sulla_bozza"])
	abbinato := map[string]string{}
	for _, a := range eff.Abbinati {
		abbinato[a.ChiaveBozza] = a.Rif
	}
	if eff.Algoritmo != distintabozza.AlgoritmoCambioFonte2 || abbinato["m-ok"] != voci[0].rif || abbinato["m-rev"] != "" {
		t.Errorf("S6 sulla bozza manuale: algoritmo %q, abbinati %+v (attesi m-ok su %s, non m-rev)", eff.Algoritmo, eff.Abbinati, voci[0].rif)
	}
	var proposti []distintabozza.AbbinamentoProposto3
	if err := json.Unmarshal(eff.AbbinamentiProposti, &proposti); err != nil || len(proposti) == 0 {
		t.Errorf("S6: abbinamenti_proposti %s %v", eff.AbbinamentiProposti, err)
	}
	for _, p := range proposti {
		if p.ChiaveBozza == "m-ok" {
			t.Errorf("il nodo abbinato è anche proposto: %+v", p)
		}
	}
	if !strings.Contains(string(eff.AbbinamentiProposti), `"m-rev"`) || strings.Contains(string(eff.DaRivedere), "m-rev") {
		t.Errorf("il nodo con l'altra rev è un abbinamento proposto, fuori da da_rivedere: %s / %s", eff.AbbinamentiProposti, eff.DaRivedere)
	}
	acquisite := map[string]bool{}
	for _, c := range eff.CaselleRiscritte {
		if c.Prima == nil {
			acquisite[c.ChiaveBozza] = true
		}
	}
	if !acquisite["m-ok"] || !acquisite[kp] {
		t.Errorf("le caselle acquisite (allegato_prima null) del nodo abbinato e del prodotto: %+v", eff.CaselleRiscritte)
	}

	// S8 sulla stessa bozza, con la proposta vista in S6
	corpo, _ := json.Marshal(map[string]any{"chiave_idempotenza": uuid.New(), "prodotto_id": s.prodotto, "partenza": "step", "gruppo": proposta.Gruppo,
		"impronta_contenuto": proposta.ImprontaContenuto, "motivo": "la radice dello STEP di prova non ha un codice letto", "bozza": map[string]any{"bozza_id": bozza, "versione_attesa": 2}})
	resp, testo := s.ad.chiediJSON(http.MethodPost, "/thread/"+s.thread.String()+"/sorgente/step/"+s.allStep.String()+"/conferma-sorgente", string(corpo))
	if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"esito":"completata"`) {
		t.Fatalf("S8: %d %.1500s", resp.StatusCode, testo)
	}
	if n := uuidConta(t, b, `SELECT count(*) FROM distinta_bozza WHERE thread_id = $1 AND prodotto_id = $2 AND stato = 'aperta'`, s.thread, s.prodotto); n != 1 {
		t.Fatalf("bozze aperte del prodotto dopo S8: %d", n)
	}
	var dopoRaw string
	if err := b.pool.QueryRow(b.ctx, `SELECT dopo::text FROM distinta_bozza_evento WHERE bozza_id = $1 AND azione = 'sorgente_cambiata' AND versione = 3`,
		bozza).Scan(&dopoRaw); err != nil {
		t.Fatalf("l'evento sorgente_cambiata della versione 3: %v", err)
	}
	var dopo struct {
		Algoritmo           string          `json:"algoritmo"`
		AbbinamentiProposti json.RawMessage `json:"abbinamenti_proposti"`
		Conteggi            map[string]int  `json:"conteggi"`
		CaselleRiscritte    []string        `json:"caselle_riscritte"`
		NuoviNodi           []string        `json:"nuovi_nodi_proposti"`
		IdentitaAggiornate  []string        `json:"identita_aggiornate"`
	}
	if err := json.Unmarshal([]byte(dopoRaw), &dopo); err != nil {
		t.Fatal(err)
	}
	t.Logf("S8, sorgente_cambiata: %s", dopoRaw)
	var propostiS8 []distintabozza.AbbinamentoProposto3
	_ = json.Unmarshal(dopo.AbbinamentiProposti, &propostiS8)
	if dopo.Algoritmo != eff.Algoritmo || !reflect.DeepEqual(propostiS8, proposti) || dopo.Conteggi["abbinati"] != len(eff.Abbinati) {
		t.Errorf("S6 e la versione scritta da S8: algoritmo %q / %q, proposti %+v / %+v, abbinati %d / %d", eff.Algoritmo, dopo.Algoritmo, proposti,
			propostiS8, len(eff.Abbinati), dopo.Conteggi["abbinati"])
	}
	var chiaviS6 []string
	for _, c := range eff.CaselleRiscritte {
		if c.ChiaveBozza != "" {
			chiaviS6 = append(chiaviS6, c.ChiaveBozza)
		} else {
			chiaviS6 = append(chiaviS6, c.DecisioneID)
		}
	}
	if !reflect.DeepEqual(chiaviS6, dopo.CaselleRiscritte) || !reflect.DeepEqual(nonNil(eff.NuoviNodi), nonNil(dopo.NuoviNodi)) {
		t.Errorf("caselle riscritte %v / %v, voci nuove %v / %v", chiaviS6, dopo.CaselleRiscritte, eff.NuoviNodi, dopo.NuoviNodi)
	}
	var aggS6 []string
	for _, a := range eff.IdentitaAggiornate {
		aggS6 = append(aggS6, a.ChiaveBozza)
	}
	sort.Strings(aggS6)
	aggS8 := append([]string(nil), dopo.IdentitaAggiornate...)
	sort.Strings(aggS8)
	if !reflect.DeepEqual(nonNil(aggS6), nonNil(aggS8)) {
		t.Errorf("identità aggiornate %v / %v", aggS6, aggS8)
	}

	// la versione 3: i nodi abbinati con la loro chiave e la casella dello STEP; nessuna casella di due nodi; m-rev senza casella
	var snapRaw string
	if err := b.pool.QueryRow(b.ctx, `SELECT snapshot::text FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = 3`, bozza).Scan(&snapRaw); err != nil {
		t.Fatal(err)
	}
	v2, err := distintabozza.LeggiSnapshot3([]byte(snapRaw))
	if err != nil {
		t.Fatal(err)
	}
	perCasella := map[string][]string{}
	caselleDi := map[string][]string{}
	for _, n := range v2.Nodi {
		for _, c := range n.Caselle {
			perCasella[c.AllegatoID.String()+"|"+c.Rif] = append(perCasella[c.AllegatoID.String()+"|"+c.Rif], n.ChiaveBozza)
			caselleDi[n.ChiaveBozza] = append(caselleDi[n.ChiaveBozza], c.Rif)
		}
	}
	for k, v := range perCasella {
		if len(v) > 1 {
			t.Errorf("la casella %s è di più nodi (doppione): %v", k, v)
		}
	}
	for k, rif := range abbinato {
		if len(caselleDi[k]) != 1 || caselleDi[k][0] != rif {
			t.Errorf("il nodo abbinato %s nella versione 3: caselle %v, attesa %s", k, caselleDi[k], rif)
		}
	}
	if len(caselleDi["m-rev"]) != 0 {
		t.Errorf("il nodo proposto (non abbinato) ha preso una casella: %v", caselleDi["m-rev"])
	}
	nuovi := 0
	for _, n := range v2.Nodi {
		switch n.ChiaveBozza {
		case kp, ka, "m-ok", "m-rev":
		default:
			nuovi++
		}
	}
	if nuovi != len(eff.NodiCheEntrano) {
		t.Errorf("le voci entrate nella versione 3: %d, S6 ne diceva %d (%v)", nuovi, len(eff.NodiCheEntrano), eff.NodiCheEntrano)
	}
}

func nonNil(x []string) []string {
	if x == nil {
		return []string{}
	}
	return x
}

func uuidConta(t *testing.T, b *bancoWeb, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := b.pool.QueryRow(b.ctx, sql, args...).Scan(&n); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return n
}
