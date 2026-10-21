//go:build integrazione

// L4 — giro G39, corsia motore, G39-01 [U] (20/10 sera, con la conferma di «corretta»; D-PB02 A, D-PB03 A) sulla strada
// vera, con il calcolo vero del Motore A e le regole ACME di prova (scenaG39: il prodotto finito 9110001A), rilievo della
// revisione del giro G39 («l'L1 usa un suggerimento finto; serve la strada vera»):
//   - S8 sullo STEP 9110001A.stp con i figli 9120101A e 9130202A (rotta vera): la bozza aperta con il registro;
//   - DOPO l'apertura arrivano tre PDF: «9120101A_1.pdf» e «9130202A_1.pdf» (il motore li legge con il codice di un figlio:
//     destinazione univoca) e «offerta_generale.pdf» (nessun codice: nessuna destinazione);
//   - S9 (GET /dati?sorgente=1) e R1 con ?suggerimenti_motore=1 danno gli stessi preassociazioni_proposte[]: i due PDF con
//     una destinazione, motivo dopo_apertura, provenienza motore e importazione = l'impronta del registro del calcolo di
//     adesso (il riferimento stabile), non quello della versione; il PDF senza destinazione no; le GET non scrivono;
//   - R2 (POST /motore-a/bozza, la rotta vera) con il primo accettato senza modifiche: PB «motore» con quel registro, che
//     R2 scrive in distinta_importazione; con il secondo spostato dall'ingegnere sul nodo dell'altro figlio: PB «corretta»
//     con lo stesso riferimento; il PDF senza destinazione a mano: PB «manuale».
//
// Sul codice di prima (S9 con i soli suggerimenti del registro della versione) la prova fallisce: S9 non dà i due PDF.
//
// I codici 91xxxxxA sono quelli della grammatica ACME di prova; il resto è inventato.
package web

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
)

func TestL4V1G39_01FileDopoLAperturaS9R1R2(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaG39(t, "G39D")
	step, _ := s.stepG39("9110001A.stp", "9110001A", fattiDi("#1", []string{"#1=9110001A", "#2=9120101A", "#3=9130202A"}, []string{"#1>#2*2", "#1>#3*1"}))
	base := "/thread/" + s.thread.String()
	get := func(percorso string, x any) {
		t.Helper()
		resp, corpo := s.ad.fai(http.MethodGet, percorso, nil, false)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %d %.800s", percorso, resp.StatusCode, corpo)
		}
		if err := json.Unmarshal([]byte(corpo), x); err != nil {
			t.Fatalf("%s: %v %.300s", percorso, err, corpo)
		}
	}

	// S8 sullo STEP del prodotto (la conferma dell'utente): nasce la bozza con il registro
	var gruppi struct {
		Gruppi []struct {
			Gruppo            string `json:"gruppo"`
			ImprontaContenuto string `json:"impronta_contenuto"`
		} `json:"gruppi"`
	}
	get(base+"/sorgente/step/"+step.String()+"/anteprima?prodotto="+s.prodotto.String(), &gruppi)
	gruppo, impronta := "", ""
	for _, g := range gruppi.Gruppi {
		if gruppo == "" && g.ImprontaContenuto != "" {
			gruppo, impronta = g.Gruppo, g.ImprontaContenuto
		}
	}
	corpo, _ := json.Marshal(map[string]any{"chiave_idempotenza": uuid.New(), "prodotto_id": s.prodotto, "partenza": "step", "gruppo": gruppo,
		"impronta_contenuto": impronta, "motivo": "sorgente della prova"})
	resp, testo := s.ad.chiediJSON(http.MethodPost, base+"/sorgente/step/"+step.String()+"/conferma-sorgente", string(corpo))
	if resp.StatusCode != http.StatusOK || !strings.Contains(testo, `"esito":"completata"`) {
		t.Fatalf("S8: %d %.1500s", resp.StatusCode, testo)
	}
	bozza := uuidSQL(t, b, `SELECT bozza_id FROM distinta_bozza WHERE thread_id = $1 AND prodotto_id = $2 AND stato = 'aperta'`, s.thread, s.prodotto)
	var regVersione string
	if err := b.pool.QueryRow(b.ctx, `SELECT i.impronta FROM distinta_bozza_versione v JOIN distinta_importazione i ON i.importazione_id = v.importazione_id
		WHERE v.bozza_id = $1 AND v.versione = 1`, bozza).Scan(&regVersione); err != nil {
		t.Fatalf("il registro della versione 1: %v", err)
	}

	// tre PDF arrivano dopo l'apertura della bozza
	pdf := func(nome, testoPdf string) uuid.UUID {
		a, sha := s.allegatoExt(nome, "pdf")
		s.analisiCorrente(sha, `{"testo_pdf": {"pagine": [{"testo": "`+testoPdf+`"}]}}`)
		return a
	}
	f1 := pdf("9120101A_1.pdf", "9120101A")
	f2 := pdf("9130202A_1.pdf", "9130202A")
	f3 := pdf("offerta_generale.pdf", "Offerta generale")

	type sorgente struct {
		Prodotti []struct {
			ProdottoID              *uuid.UUID                               `json:"prodotto_id"`
			PreassociazioniProposte []distintabozza.PreassociazioneProposta3 `json:"preassociazioni_proposte"`
			Righe                   []struct {
				AllegatoID uuid.UUID `json:"allegato_id"`
				Stato      string    `json:"stato"`
				Motivo     string    `json:"motivo"`
			} `json:"righe"`
		} `json:"prodotti"`
	}
	ordina := func(xx []distintabozza.PreassociazioneProposta3) []distintabozza.PreassociazioneProposta3 {
		out := append([]distintabozza.PreassociazioneProposta3{}, xx...)
		sort.Slice(out, func(i, j int) bool { return out[i].AllegatoID.String() < out[j].AllegatoID.String() })
		return out
	}
	prima := fotoDelDatabase(t, b)
	var x sorgente
	get(base+"/motore-a/dati?sorgente=1", &x)
	var s9 []distintabozza.PreassociazioneProposta3
	var righe []string
	for _, p := range x.Prodotti {
		if p.ProdottoID != nil && *p.ProdottoID == s.prodotto {
			s9 = p.PreassociazioniProposte
			for _, r := range p.Righe {
				if r.AllegatoID == f1 || r.AllegatoID == f2 || r.AllegatoID == f3 {
					righe = append(righe, r.AllegatoID.String()[:8]+":"+r.Stato+"/"+r.Motivo)
				}
			}
		}
	}
	t.Logf("S9, righe dei PDF: %v; suggerimenti: %+v", righe, s9)
	var r1 distintabozza.LetturaR1
	get(base+"/motore-a/bozza?prodotto="+s.prodotto.String()+"&suggerimenti_motore=1", &r1)
	if d := differenze(prima, fotoDelDatabase(t, b)); d != "" {
		t.Errorf("S9 o R1 hanno scritto: %s", d)
	}
	if r1.SuggerimentiMotore == nil || !r1.SuggerimentiMotore.Calcolati || r1.PreassociazioniProposte == nil {
		t.Fatalf("R1 con i suggerimenti del motore: %+v", r1.SuggerimentiMotore)
	}
	if !reflect.DeepEqual(ordina(s9), ordina(*r1.PreassociazioniProposte)) {
		t.Errorf("S9 e R1 (?suggerimenti_motore=1) diversi:\nS9 %+v\nR1 %+v", s9, *r1.PreassociazioniProposte)
	}
	sugg := map[uuid.UUID]distintabozza.PreassociazioneProposta3{}
	for _, p := range s9 {
		sugg[p.AllegatoID] = p
	}
	regAdesso := ""
	for _, a := range []uuid.UUID{f1, f2} {
		p, ok := sugg[a]
		if !ok || p.Motivo != distintabozza.PropostaDopoApertura || p.Provenienza != distintabozza.ProvenienzaMotore || p.ChiaveBozza == "" ||
			len(p.Importazione) != 64 || p.Importazione == regVersione {
			t.Fatalf("il PDF %s arrivato dopo l'apertura in S9: %+v (%v); tutti %+v", a, p, ok, s9)
		}
		if regAdesso == "" {
			regAdesso = p.Importazione
		} else if p.Importazione != regAdesso {
			t.Fatalf("due registri di adesso diversi: %s, %s", regAdesso, p.Importazione)
		}
	}
	if sugg[f1].ChiaveBozza == sugg[f2].ChiaveBozza {
		t.Fatalf("i due PDF sullo stesso nodo: %+v", s9)
	}
	if p, ok := sugg[f3]; ok {
		t.Fatalf("il PDF senza destinazione è un suggerimento: %+v", p)
	}

	// R2, la rotta vera: una PB in più sulla versione data
	salva := func(versione int, a uuid.UUID, chiave, registro string) {
		t.Helper()
		var raw json.RawMessage
		if err := b.pool.QueryRow(b.ctx, `SELECT snapshot FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = $2`, bozza, versione).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		m := map[string]any{}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		pb := map[string]any{"allegato_id": a, "chiave_bozza": chiave}
		if registro != "" {
			pb["importazione"] = registro
		}
		pp, _ := m["documenti_preassociati"].([]any)
		m["documenti_preassociati"] = append(pp, pb)
		corpo, _ := json.Marshal(map[string]any{"bozza_id": bozza, "versione_attesa": versione, "prodotto_id": s.prodotto, "vocabolario": 2,
			"chiave_salvataggio": uuid.New(), "snapshot": m,
			"azioni": []any{map[string]any{"azione": "file_preassociato", "chiave_bozza": chiave, "dopo": map[string]any{"allegato_id": a, "chiave_bozza": chiave},
				"origine": "utente"}}})
		resp, testo := s.ad.chiediJSON(http.MethodPost, base+"/motore-a/bozza", string(corpo))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("R2 versione %d, PB %s su %s: %d %.1500s", versione, a, chiave, resp.StatusCode, testo)
		}
	}
	pbDi := func(versione int, a uuid.UUID) distintabozza.Preassociazione3 {
		t.Helper()
		var raw json.RawMessage
		if err := b.pool.QueryRow(b.ctx, `SELECT snapshot FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = $2`, bozza, versione).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var sn struct {
			Documenti []distintabozza.Preassociazione3 `json:"documenti_preassociati"`
		}
		if err := json.Unmarshal(raw, &sn); err != nil {
			t.Fatal(err)
		}
		for _, p := range sn.Documenti {
			if p.AllegatoID == a {
				return p
			}
		}
		t.Fatalf("versione %d: nessuna PB per %s (%+v)", versione, a, sn.Documenti)
		return distintabozza.Preassociazione3{}
	}

	// 1. il primo accettato senza modifiche: «motore», con il riferimento al calcolo di adesso, che R2 scrive
	salva(1, f1, sugg[f1].ChiaveBozza, sugg[f1].Importazione)
	if p := pbDi(2, f1); p.Provenienza != distintabozza.ProvenienzaMotore || p.Importazione != regAdesso || p.ChiaveBozza != sugg[f1].ChiaveBozza {
		t.Errorf("il suggerimento accettato: %+v", p)
	}
	if n := uuidConta(t, b, `SELECT count(*) FROM distinta_importazione WHERE thread_id = $1 AND prodotto_id = $2 AND impronta = $3`,
		s.thread, s.prodotto, regAdesso); n != 1 {
		t.Errorf("il registro di adesso in distinta_importazione: %d", n)
	}
	// 2. il secondo spostato dall'ingegnere sul nodo dell'altro figlio: «corretta», con lo stesso riferimento
	salva(2, f2, sugg[f1].ChiaveBozza, sugg[f2].Importazione)
	if p := pbDi(3, f2); p.Provenienza != distintabozza.ProvenienzaCorretta || p.Importazione != regAdesso || p.ChiaveBozza != sugg[f1].ChiaveBozza {
		t.Errorf("il suggerimento spostato: %+v", p)
	}
	// 3. il PDF senza destinazione, a mano: «manuale»
	salva(3, f3, sugg[f2].ChiaveBozza, "")
	if p := pbDi(4, f3); p.Provenienza != distintabozza.ProvenienzaManuale || p.Importazione != "" {
		t.Errorf("il PDF a mano: %+v", p)
	}
	if n := uuidConta(t, b, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = ANY($1)`, []uuid.UUID{f1, f2, f3}); n != 0 {
		t.Errorf("AR prima di R5: %d", n)
	}
}
