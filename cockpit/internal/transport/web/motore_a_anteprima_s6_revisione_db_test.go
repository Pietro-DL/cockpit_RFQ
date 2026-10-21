//go:build integrazione

// L4 — giro G39, corsia motore, aperto (a) del verificatore del 20/10 [T-39]: S6 `?bozza=` su una REVISIONE con un
// secondo STEP mostra lo stesso effetto che S8 scrive poi sulla stessa bozza. Prima S6 non chiamava
// distintabozza.PreparaRevisione e calcolava cambio-fonte-1 sulla versione da-conferma-1: i nodi registrati con la casella
// dello STEP di prima uscivano (fuori_per_cambio_fonte) e le voci dello STEP nuovo con gli stessi codici rientravano come
// doppioni, mentre S8 (Cambia) li riconcilia per identità. Scena (B87 con il calcolo vero del Motore A e le regole ACME,
// le sessioni vere; la scena scenaG39: il prodotto finito 9110001A con i codici della grammatica ACME di prova):
//   - lo STEP A del prodotto letto con due figli dai codici della grammatica ACME; S8 su A (rotta vera, partenza step),
//     R4 e R5 dalle rotte vere: la conferma 1 crea i due componenti;
//   - lo STEP B (rev 2) con la stessa struttura e gli stessi codici, uno sha256 diverso (i rif di B non sono quelli di A)
//     e un pezzo in più; la revisione aperta da R2 (partenza conferma: da-conferma-1);
//   - S6 sulla revisione verso B: algoritmo cambio-fonte-2, i due nodi registrati ricollegati, nessuna uscita, le
//     caselle acquisite con l'occorrenza di prima, una sola voce che entra (il pezzo nuovo);
//   - S8 su B con la revisione dichiarata scrive la versione 2: l'evento sorgente_cambiata ha lo stesso algoritmo, le
//     stesse uscite (nessuna), le stesse caselle riscritte, le stesse voci nuove, lo stesso numero di abbinati e di
//     registrati della fonte precedente; nella versione nessuna casella è di due nodi; S6 non scrive.
//
// I codici 91xxxxxA sono quelli della grammatica ACME di prova; il resto è inventato.
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

func TestL4V1G39S6RevisioneComeS8(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaG39(t, "G39R")
	allA, _ := s.stepG39("9110001A.stp", "9110001A", fattiDi("#1", []string{"#1=9110001A", "#2=9120101A", "#3=9130202A"}, []string{"#1>#2*2", "#1>#3*1"}))
	base := "/thread/" + s.thread.String()
	s6 := func(allegato uuid.UUID, extra string) string {
		return base + "/sorgente/step/" + allegato.String() + "/anteprima?prodotto=" + s.prodotto.String() + extra
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
	// propostaDi: il primo gruppo che si importa dello STEP e la sua impronta
	propostaDi := func(allegato uuid.UUID) (string, string) {
		t.Helper()
		var gruppi []struct {
			Gruppo            string `json:"gruppo"`
			ImprontaContenuto string `json:"impronta_contenuto"`
		}
		if err := json.Unmarshal(get(s6(allegato, ""))["gruppi"], &gruppi); err != nil {
			t.Fatal(err)
		}
		for _, g := range gruppi {
			if g.ImprontaContenuto != "" {
				return g.Gruppo, g.ImprontaContenuto
			}
		}
		t.Fatalf("lo STEP %s non ha un gruppo che si importa: %+v", allegato, gruppi)
		return "", ""
	}
	s8 := func(allegato uuid.UUID, gruppo, impronta string, bozza map[string]any, extra map[string]any) string {
		t.Helper()
		m := map[string]any{"chiave_idempotenza": uuid.New(), "prodotto_id": s.prodotto, "partenza": "step", "gruppo": gruppo,
			"impronta_contenuto": impronta, "motivo": "prima sorgente della prova"}
		if bozza != nil {
			m["bozza"] = bozza
		}
		for k, v := range extra {
			m[k] = v
		}
		corpo, _ := json.Marshal(m)
		resp, testo := s.ad.chiediJSON(http.MethodPost, base+"/sorgente/step/"+allegato.String()+"/conferma-sorgente", string(corpo))
		if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"esito":"completata"`) {
			t.Fatalf("S8 su %s: %d %.1500s", allegato, resp.StatusCode, testo)
		}
		return testo
	}
	bozzaAperta := func() uuid.UUID {
		t.Helper()
		return uuidSQL(t, b, `SELECT bozza_id FROM distinta_bozza WHERE thread_id = $1 AND prodotto_id = $2 AND stato = 'aperta'`, s.thread, s.prodotto)
	}
	corpoBozza := func(bozza uuid.UUID, versione int) map[string]any {
		t.Helper()
		var raw json.RawMessage
		if err := b.pool.QueryRow(b.ctx, `SELECT snapshot FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = $2`, bozza, versione).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		m := map[string]any{}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		m["bozza_id"], m["bozza_versione"] = bozza, versione
		return m
	}

	// --- la prima conferma, con lo STEP A: S8, R4, R5
	gA, iA := propostaDi(allA)
	s8(allA, gA, iA, nil, nil)
	bozzaA := bozzaAperta()
	raw, _ := json.Marshal(corpoBozza(bozzaA, 1))
	resp, testo := s.ad.chiediJSON(http.MethodPost, base+"/motore-a/bom/riepilogo?avvisi_motore=1", string(raw))
	var r4 distintabozza.RiepilogoR4
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &r4) != nil || !r4.Confermabile {
		t.Fatalf("R4 della prima conferma: %d %.2000s", resp.StatusCode, testo)
	}
	m := corpoBozza(bozzaA, 1)
	m["chiave_idempotenza"] = uuid.New()
	raw, _ = json.Marshal(map[string]any{"firma_anteprima": r4.FirmaAnteprima, "bozza": m})
	resp, testo = s.ad.chiediJSON(http.MethodPost, base+"/motore-a/bom/conferma", string(raw))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("R5 della prima conferma: %d %.2000s", resp.StatusCode, testo)
	}
	if n := uuidConta(t, b, `SELECT count(*) FROM componente WHERE thread_id = $1 AND codice IN ('9120101A', '9130202A')`, s.thread); n != 2 {
		t.Fatalf("i componenti della conferma 1: %d", n)
	}

	// --- lo STEP B (rev 2): stessa struttura, stessi codici, chiavi e sha256 diversi, un pezzo in più
	allB, _ := s.stepG39("9110001A_rev2.stp", "9110001A", fattiDi("#1", []string{"#1=9110001A", "#5=9120101A", "#6=9130202A", "#7=9130303A"},
		[]string{"#1>#5*2", "#1>#6*1", "#1>#7*3"}))
	// la revisione aperta come la apre la pagina: R2 crea con la partenza conferma (da-conferma-1)
	c, err := distintabozza.CreaFormato3(b.ctx, b.pool, distintabozza.Creazione3{ThreadID: s.thread, UtenteID: s.utente,
		Comune: distintabozza.ComuneR2{ProdottoID: s.prodotto, ChiaveSalvataggio: uuid.New(), ImprontaRichiesta: strings.Repeat("a", 64)},
		Crea:   distintabozza.CreaR2{Partenza: distintabozza.PartenzaConferma}})
	if err != nil {
		t.Fatalf("la revisione (da-conferma-1): %v", err)
	}
	rev := c.BozzaID
	gB, iB := propostaDi(allB)

	// S6 sulla revisione verso B, senza scrivere
	prima := fotoDelDatabase(t, b)
	x := get(s6(allB, "&gruppo="+url.QueryEscape(gB)+"&bozza="+rev.String()))
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("S6 ha scritto: %s", d)
	}
	t.Logf("S6: %s", x["effetto_sulla_bozza"])
	var eff struct {
		Algoritmo        string   `json:"algoritmo"`
		Fuori            []string `json:"fuori_per_cambio_fonte"`
		NodiCheEntrano   []string `json:"nodi_che_entrano"`
		NuoviNodi        []string `json:"nuovi_nodi_proposti"`
		CaselleRiscritte []struct {
			ChiaveBozza string     `json:"chiave_bozza"`
			DecisioneID string     `json:"decisione_id"`
			Prima       *uuid.UUID `json:"allegato_prima"`
		} `json:"caselle_riscritte"`
		Abbinati []struct {
			ChiaveBozza string `json:"chiave_bozza"`
		} `json:"abbinati"`
		AbbinamentiProposti json.RawMessage `json:"abbinamenti_proposti"`
		Ricollegati         []string        `json:"ricollegati"`
		IdentitaAggiornate  []struct {
			ChiaveBozza string `json:"chiave_bozza"`
		} `json:"identita_aggiornate"`
	}
	if err := json.Unmarshal(x["effetto_sulla_bozza"], &eff); err != nil {
		t.Fatalf("effetto_sulla_bozza: %v %s", err, x["effetto_sulla_bozza"])
	}
	if eff.Algoritmo != distintabozza.AlgoritmoCambioFonte2 || len(eff.Fuori) != 0 || len(eff.Ricollegati) != 2 || len(eff.Abbinati) != 2 ||
		len(eff.NodiCheEntrano) != 1 {
		t.Errorf("S6 sulla revisione: algoritmo %q, fuori %v, ricollegati %v, abbinati %+v, entrano %v (attesi cambio-fonte-2, nessuna uscita, "+
			"due ricollegati e abbinati, una voce nuova)", eff.Algoritmo, eff.Fuori, eff.Ricollegati, eff.Abbinati, eff.NodiCheEntrano)
	}
	ricollegato := map[string]bool{}
	for _, k := range eff.Ricollegati {
		ricollegato[k] = true
	}
	for _, c := range eff.CaselleRiscritte {
		if ricollegato[c.ChiaveBozza] && (c.Prima == nil || *c.Prima != allA) {
			t.Errorf("la casella acquisita dal nodo ricollegato %s dice allegato_prima %v, attesa l'occorrenza di A", c.ChiaveBozza, c.Prima)
		}
	}

	// S8 su B con la revisione dichiarata: la versione 2
	s8(allB, gB, iB, map[string]any{"bozza_id": rev, "versione_attesa": 1}, map[string]any{"sostituisci": map[string]any{"conferma": true},
		"motivo": "revisione 2 dello STEP"})
	if bozzaAperta() != rev {
		t.Fatal("S8 ha aperto un'altra bozza")
	}
	var dopoRaw string
	if err := b.pool.QueryRow(b.ctx, `SELECT dopo::text FROM distinta_bozza_evento WHERE bozza_id = $1 AND azione = 'sorgente_cambiata' AND versione = 2`,
		rev).Scan(&dopoRaw); err != nil {
		t.Fatalf("l'evento sorgente_cambiata della versione 2: %v", err)
	}
	t.Logf("S8, sorgente_cambiata: %s", dopoRaw)
	var dopo struct {
		Algoritmo           string          `json:"algoritmo"`
		AbbinamentiProposti json.RawMessage `json:"abbinamenti_proposti"`
		Conteggi            map[string]int  `json:"conteggi"`
		CaselleRiscritte    []string        `json:"caselle_riscritte"`
		Fuori               []string        `json:"fuori_per_cambio_fonte"`
		NuoviNodi           []string        `json:"nuovi_nodi_proposti"`
		IdentitaAggiornate  []string        `json:"identita_aggiornate"`
	}
	if err := json.Unmarshal([]byte(dopoRaw), &dopo); err != nil {
		t.Fatal(err)
	}
	var proposti, propostiS8 []distintabozza.AbbinamentoProposto3
	_ = json.Unmarshal(eff.AbbinamentiProposti, &proposti)
	_ = json.Unmarshal(dopo.AbbinamentiProposti, &propostiS8)
	if dopo.Algoritmo != eff.Algoritmo || !reflect.DeepEqual(nonNil(dopo.Fuori), nonNil(eff.Fuori)) || dopo.Conteggi["abbinati"] != len(eff.Abbinati) ||
		dopo.Conteggi["registrati_della_fonte_precedente"] != len(eff.Ricollegati) || dopo.Conteggi["entrati"] != len(eff.NodiCheEntrano) ||
		len(propostiS8) != len(proposti) || (len(proposti) > 0 && !reflect.DeepEqual(propostiS8, proposti)) {
		t.Errorf("S6 e la versione scritta da S8: algoritmo %q / %q, fuori %v / %v, abbinati %d / %d, ricollegati %d / %d, entrati %d / %d, proposti %+v / %+v",
			eff.Algoritmo, dopo.Algoritmo, eff.Fuori, dopo.Fuori, len(eff.Abbinati), dopo.Conteggi["abbinati"], len(eff.Ricollegati),
			dopo.Conteggi["registrati_della_fonte_precedente"], len(eff.NodiCheEntrano), dopo.Conteggi["entrati"], proposti, propostiS8)
	}
	var chiaviS6 []string
	for _, c := range eff.CaselleRiscritte {
		if c.ChiaveBozza != "" {
			chiaviS6 = append(chiaviS6, c.ChiaveBozza)
		} else {
			chiaviS6 = append(chiaviS6, c.DecisioneID)
		}
	}
	sort.Strings(chiaviS6)
	chiaviS8 := append([]string(nil), dopo.CaselleRiscritte...)
	sort.Strings(chiaviS8)
	if !reflect.DeepEqual(nonNil(chiaviS6), nonNil(chiaviS8)) || !reflect.DeepEqual(nonNil(eff.NuoviNodi), nonNil(dopo.NuoviNodi)) {
		t.Errorf("caselle riscritte %v / %v, voci nuove %v / %v", chiaviS6, chiaviS8, eff.NuoviNodi, dopo.NuoviNodi)
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
	// la versione 2: i ricollegati con la loro chiave e la casella di B, nessuna casella di due nodi
	var snapRaw string
	if err := b.pool.QueryRow(b.ctx, `SELECT snapshot::text FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = 2`, rev).Scan(&snapRaw); err != nil {
		t.Fatal(err)
	}
	v2, err := distintabozza.LeggiSnapshot3([]byte(snapRaw))
	if err != nil {
		t.Fatal(err)
	}
	perCasella := map[string][]string{}
	caselleDi := map[string][]uuid.UUID{}
	for _, n := range v2.Nodi {
		for _, c := range n.Caselle {
			perCasella[c.AllegatoID.String()+"|"+c.Rif] = append(perCasella[c.AllegatoID.String()+"|"+c.Rif], n.ChiaveBozza)
			caselleDi[n.ChiaveBozza] = append(caselleDi[n.ChiaveBozza], c.AllegatoID)
		}
	}
	for k, v := range perCasella {
		if len(v) > 1 {
			t.Errorf("la casella %s è di più nodi (doppione): %v", k, v)
		}
	}
	for _, k := range eff.Ricollegati {
		if len(caselleDi[k]) != 1 || caselleDi[k][0] != allB {
			t.Errorf("il nodo ricollegato %s nella versione 2: caselle %v, attesa una su B", k, caselleDi[k])
		}
	}
}
