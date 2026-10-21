//go:build integrazione

// L4 — giro G39-ter (21/10), corsia motore, con il Motore A vero e le regole ACME di prova (sintetiche), sui rilievi del
// gate della direttiva 39 (P39-08) e su G39-06 [U]:
//   - TestV1G39terD2cNomiSenzaSeparatoreDB (D-2 (c) [U]): con il finito 9123456A e il sottoassieme 9123456-100, gli
//     STEP «9123456-100A.stp», «9123456-100(B).stp», «9123456-100rev2.stp» e «9123456_100.stp» (la grammatica di prova
//     ne legge solo la base 9123456, quella del finito) non sono candidati univoci del finito: R2 salva la PB verso il
//     nodo del sottoassieme 9123456-100 (200), R4 con il motore non la blocca, S9 non li propone come sorgente del finito
//     e la candidatura non vale l'esenzione dal motivo in S8 (UnivocaDel falso);
//   - TestV1G39terS8VuoleIlMotivoSuiNomiDelSottoassiemeDB (D-2 (c) [U], G39-02 [U]): gli stessi quattro nomi, con i fatti
//     di uno STEP importabile con la radice 9123456A (l'anteprima di S8 vuole una struttura del prodotto: è anche la
//     struttura propria, che con il nome del sottoassieme dà una candidatura non univoca) e la vecchia lettura del nome
//     (documento_proposta.codice 9123456-100): S8 verso il finito 9123456A senza il motivo non si completa, il passo 2
//     vuole il motivo (MOTIVO_OBBLIGATORIO); il riscontro: lo STEP «9123456-001.stp», candidato univoco, si conferma
//     senza il motivo;
//   - TestV1G39terStrutturaPropriaColNomeDelFiglioDB (G39-02 [U]): lo STEP «9110001-100.stp» con una sua struttura con
//     la radice del finito 9110001A e il sottoassieme 9110001-100 nella RFQ resta una candidatura del finito, non
//     univoca (non un file ordinario);
//   - TestV1G39terG39_06PBVersoUnaParteDB (G39-06 [U], lettura [T] dell'orchestratore): lo STEP «9123456-001.stp»,
//     candidatura non univoca (il nome è il codice di un altro finito): R2 salva la PB verso la parte 9123456-200 (200) e
//     rifiuta quella verso il prodotto finito della bozza (422 STEP_CANDIDATO_DEL_FINITO, il rimedio S8); R4 non blocca la
//     prima e blocca la seconda; in S9 la PB salvata verso la parte non è in_conflitto e il motore non propone lo STEP a
//     nessuno; il riscontro: la candidatura univoca resta esclusa anche verso la parte (422, R4 blocca).
// Tutte falliscono su 9eb44e3. Codici della grammatica ACME di prova; il resto è inventato.

package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
)

// nodoG39ter: il nodo che riusa il componente dato, con l'identità della scena (l'emittente del nodo radice).
func (g *g39bis) nodoG39ter(chiave string, componente uuid.UUID, codice, tipo string) map[string]any {
	identita := map[string]any{}
	for k, v := range g.s.snapshot["nodi"].([]any)[0].(map[string]any)["identita"].(map[string]any) {
		identita[k] = v
	}
	identita["codice"], identita["modo"] = codice, "registrato"
	return map[string]any{"chiave_bozza": chiave, "componente_id": componente.String(), "op": "riusa", "tipo": tipo, "origine": "utente",
		"identita": identita}
}

// snapshotG39ter: lo snapshot della scena con il nodo dato (figlio della radice) e la PB sull'allegato verso la chiave data.
func (g *g39bis) snapshotG39ter(nodo map[string]any, a uuid.UUID, chiave string) (map[string]any, map[string]any, map[string]any) {
	m := map[string]any{}
	for k, v := range g.s.snapshot {
		m[k] = v
	}
	arco := map[string]any{"padre": "k-root", "figlio": nodo["chiave_bozza"], "qta": 1, "origine": "utente"}
	m["nodi"] = append(append([]any{}, g.s.snapshot["nodi"].([]any)...), nodo)
	m["archi"] = []any{arco}
	pb := map[string]any{"allegato_id": a, "chiave_bozza": chiave}
	m["documenti_preassociati"] = []any{pb}
	return m, arco, pb
}

// r2Nodo: R2 (versione attesa 1) con il nodo dato, il suo arco e la PB sull'allegato verso la chiave data.
func (g *g39bis) r2Nodo(nodo map[string]any, a uuid.UUID, chiave string) (int, string) {
	m, arco, pb := g.snapshotG39ter(nodo, a, chiave)
	k := nodo["chiave_bozza"]
	raw, _ := json.Marshal(map[string]any{"bozza_id": g.s.bozza, "versione_attesa": 1, "prodotto_id": g.s.prodotto, "vocabolario": 2,
		"chiave_salvataggio": uuid.New(), "snapshot": m,
		"azioni": []any{
			map[string]any{"azione": "nodo_aggiunto", "chiave_bozza": k, "dopo": nodo, "origine": "utente"},
			map[string]any{"azione": "arco_aggiunto", "chiave_bozza": k, "dopo": arco, "origine": "utente"},
			map[string]any{"azione": "file_preassociato", "chiave_bozza": chiave, "dopo": pb, "origine": "utente"},
		}})
	resp, testo := g.ad.chiediJSON(http.MethodPost, "/thread/"+g.s.thread.String()+"/motore-a/bozza", string(raw))
	return resp.StatusCode, testo
}

// r4Nodo: R4 con ?avvisi_motore=1 sullo snapshot con il nodo dato e la PB sull'allegato verso la chiave data (bozza alla
// versione 1): vero se l'allegato ha il blocco step_candidato_del_finito.
func (g *g39bis) r4Nodo(nodo map[string]any, a uuid.UUID, chiave string) bool {
	g.t.Helper()
	percorso := "/thread/" + g.s.thread.String() + "/motore-a/bom/riepilogo?avvisi_motore=1"
	corpo := func(vb map[string]any, firma string) string {
		m, _, _ := g.snapshotG39ter(nodo, a, chiave)
		if vb != nil {
			m["versione_bom_base"], m["firma_base"] = vb, firma
		}
		m["bozza_id"], m["bozza_versione"] = g.s.bozza, 1
		raw, _ := json.Marshal(m)
		return string(raw)
	}
	resp, testo := g.ad.chiediJSON(http.MethodPost, percorso, corpo(nil, ""))
	var r0 distintabozza.RiepilogoR4
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &r0) != nil {
		g.t.Fatalf("R4: %d %.1500s", resp.StatusCode, testo)
	}
	if r0.Base.Stato == distintabozza.BaseSuperata {
		resp, testo = g.ad.chiediJSON(http.MethodPost, percorso, corpo(map[string]any{"conferma_id": nil, "numero": 0,
			"contatore": r0.VersioneCorrente.Contatore}, r0.VersioneCorrente.FirmaStruttura))
	}
	var r distintabozza.RiepilogoR4
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &r) != nil || !r.AvvisiMotoreCalcolati {
		g.t.Fatalf("R4 con il motore: %d %.1500s", resp.StatusCode, testo)
	}
	for _, bl := range r.Blocchi {
		if bl.Codice == distintabozza.BloccoPreassociazioneNonApplicab && bl.Motivo == "step_candidato_del_finito" && bl.AllegatoID != nil && *bl.AllegatoID == a {
			return true
		}
	}
	return false
}

// righeS9: le righe (stato/motivo) dell'allegato nella sezione del prodotto della bozza della scena, in S9.
func (g *g39bis) righeS9(a uuid.UUID) []string {
	g.t.Helper()
	resp, corpo := g.ad.chiediJSON(http.MethodGet, "/thread/"+g.s.thread.String()+"/motore-a/dati?sorgente=1", "")
	if resp.StatusCode != http.StatusOK {
		g.t.Fatalf("S9: %d %.800s", resp.StatusCode, corpo)
	}
	var x struct {
		Prodotti []struct {
			ProdottoID *uuid.UUID `json:"prodotto_id"`
			Righe      []struct {
				AllegatoID uuid.UUID `json:"allegato_id"`
				Stato      string    `json:"stato"`
				Motivo     string    `json:"motivo"`
			} `json:"righe"`
		} `json:"prodotti"`
	}
	if err := json.Unmarshal([]byte(corpo), &x); err != nil {
		g.t.Fatalf("S9: %v", err)
	}
	var out []string
	for _, p := range x.Prodotti {
		if p.ProdottoID == nil || *p.ProdottoID != g.s.prodotto {
			continue
		}
		for _, r := range p.Righe {
			if r.AllegatoID == a {
				out = append(out, r.Stato+"/"+r.Motivo)
			}
		}
	}
	return out
}

// univocaDelFinito: la condizione dell'esenzione dal motivo in S8 sul calcolo vero (StepCandidati.UnivocaDel).
func (g *g39bis) univocaDelFinito(a, fin uuid.UUID) bool {
	g.t.Helper()
	ris, err := g.b.ws.servizioMotoreA().Calcola(g.b.ctx, g.s.thread)
	if err != nil {
		g.t.Fatal(err)
	}
	return distintabozza.StepCandidatiDalMotore(ris.EsitoThread, ris.Thread).UnivocaDel(a, fin)
}

var nomiG39terSottoassieme = []string{"9123456-100A.stp", "9123456-100(B).stp", "9123456-100rev2.stp", "9123456_100.stp"}

func TestV1G39terD2cNomiSenzaSeparatoreDB(t *testing.T) {
	for i, nome := range nomiG39terSottoassieme {
		t.Run(nome, func(t *testing.T) {
			g := nuovaG39bis(t)
			fin := g.componente("9123456A", "finito")
			sotto := g.componente("9123456-100", "sottoassieme")
			a := g.allegato(nome, "stp", byte('m'+i))
			accendiMotoreV1(t, g.b, g.s.thread)

			for _, c := range g.candidature()[a] {
				if c.Univoca {
					t.Errorf("lo STEP %s (il sottoassieme 9123456-100) e' il candidato univoco di %s: %+v", nome, c.CodiceProdotto, c)
				}
			}
			if g.univocaDelFinito(a, fin) {
				t.Errorf("lo STEP %s vale l'esenzione dal motivo in S8 per il finito (UnivocaDel)", nome)
			}
			if _, ok := g.proposteS9(fin)[a]; ok {
				t.Errorf("S9 propone lo STEP %s come sorgente del finito", nome)
			}
			nodo := g.nodoG39ter("k-sotto", sotto, "9123456-100", "sottoassieme")
			if g.r4Nodo(nodo, a, "k-sotto") {
				t.Errorf("R4 blocca la PB dello STEP %s verso il sottoassieme 9123456-100", nome)
			}
			if st, testo := g.r2Nodo(nodo, a, "k-sotto"); st != http.StatusOK || strings.Contains(testo, distintabozza.CodiceStepCandidatoDelFinito) {
				t.Fatalf("R2 con la PB dello STEP %s verso il sottoassieme 9123456-100: %d %.800s", nome, st, testo)
			}
		})
	}
}

func TestV1G39terS8VuoleIlMotivoSuiNomiDelSottoassiemeDB(t *testing.T) {
	type caso struct {
		nome, letto string
		motivo      bool
	}
	casi := []caso{{"9123456-001.stp", "9123456-001", false}} // il riscontro: candidato univoco, nessun motivo
	for _, n := range nomiG39terSottoassieme {
		casi = append(casi, caso{n, "9123456-100", true})
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			b := preparaBancoWeb(t)
			s := b.scenaG39(t, "G39T")
			fin := s.componenteTipo("9123456A", "finito")
			s.componenteTipo("9123456-100", "sottoassieme")
			step, _ := s.stepG39(c.nome, c.letto, fattiDi("#1", []string{"#1=9123456A", "#2=9120101A"}, []string{"#1>#2*1"}))
			base := "/thread/" + s.thread.String()
			var gruppi struct {
				Gruppi []struct {
					Gruppo            string `json:"gruppo"`
					ImprontaContenuto string `json:"impronta_contenuto"`
				} `json:"gruppi"`
			}
			resp, corpo := s.ad.fai(http.MethodGet, base+"/sorgente/step/"+step.String()+"/anteprima?prodotto="+fin.String(), nil, false)
			if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(corpo), &gruppi) != nil {
				t.Fatalf("anteprima: %d %.800s", resp.StatusCode, corpo)
			}
			gruppo, impronta := "", ""
			for _, g := range gruppi.Gruppi {
				if gruppo == "" && g.ImprontaContenuto != "" {
					gruppo, impronta = g.Gruppo, g.ImprontaContenuto
				}
			}
			if gruppo == "" {
				t.Fatalf("anteprima senza un gruppo con l'impronta: %.800s", corpo)
			}
			raw, _ := json.Marshal(map[string]any{"chiave_idempotenza": uuid.New(), "prodotto_id": fin, "partenza": "step", "gruppo": gruppo,
				"impronta_contenuto": impronta})
			resp, testo := s.ad.chiediJSON(http.MethodPost, base+"/sorgente/step/"+step.String()+"/conferma-sorgente", string(raw))
			completata := resp.StatusCode == http.StatusOK && strings.Contains(testo, `"esito":"completata"`)
			if c.motivo {
				if completata || !strings.Contains(testo, distintabozza.CodiceMotivoObbligatorio) {
					t.Errorf("S8 senza il motivo sullo STEP %s verso il finito 9123456A: atteso MOTIVO_OBBLIGATORIO, avuto %d %.1200s", c.nome, resp.StatusCode, testo)
				}
				return
			}
			if !completata {
				t.Errorf("S8 senza il motivo sullo STEP univoco %s (riscontro): %d %.1200s", c.nome, resp.StatusCode, testo)
			}
		})
	}
}

func TestV1G39terStrutturaPropriaColNomeDelFiglioDB(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.scenaG39(t, "G39S")
	s.componenteTipo("9110001-100", "sottoassieme")
	step, _ := s.stepG39("9110001-100.stp", "", fattiDi("#1", []string{"#1=9110001A", "#2=9110001-100"}, []string{"#1>#2*1"}))
	ris, err := b.ws.servizioMotoreA().Calcola(b.ctx, s.thread)
	if err != nil {
		t.Fatal(err)
	}
	var trovata bool
	for _, c := range ris.CandidatureSorgente() {
		if c.AllegatoID != step {
			continue
		}
		if c.ProdottoID != s.prodotto || c.Univoca {
			t.Errorf("lo STEP 9110001-100.stp con la struttura propria: candidatura inattesa %+v", c)
		}
		trovata = true
	}
	if !trovata {
		for _, f := range ris.EsitoThread.Ancoraggi.File {
			raw, _ := json.Marshal(f)
			t.Logf("ancoraggio %s", raw)
		}
		for _, pv := range ris.EsitoThread.ProdottiValutati {
			raw, _ := json.Marshal(pv.StruttureCandidate)
			t.Logf("strutture candidate di %s: %s", pv.CodiceRichiesto, raw)
		}
		t.Errorf("lo STEP 9110001-100.stp con una sua struttura con la radice del finito 9110001A non e' una candidatura del finito: diventa un file ordinario")
	}
}

func TestV1G39terG39_06PBVersoUnaParteDB(t *testing.T) {
	t.Run("non univoca", func(t *testing.T) {
		g := nuovaG39bis(t)
		fin := g.componente("9123456A", "finito")
		altro := g.componente("9123456-001", "finito")
		parte := g.componente("9123456-200", "sciolto")
		a := g.allegato("9123456-001.stp", "stp", 'j')
		accendiMotoreV1(t, g.b, g.s.thread)

		cc := g.candidature()[a]
		if len(cc) == 0 {
			t.Fatalf("lo STEP 9123456-001.stp non e' una candidatura (riscontro della scena)")
		}
		for _, c := range cc {
			if c.Univoca {
				t.Fatalf("lo STEP 9123456-001.stp e' una candidatura univoca (riscontro della scena): %+v", c)
			}
		}
		for _, p := range []uuid.UUID{fin, altro} {
			if _, ok := g.proposteS9(p)[a]; ok {
				t.Errorf("S9 propone lo STEP non univoco come sorgente di %s", p)
			}
		}
		for _, r := range g.righeS9(a) {
			if strings.HasPrefix(r, "proposto/") {
				t.Errorf("S9 propone da solo lo STEP non univoco come PB: %v", g.righeS9(a))
			}
		}
		nodo := g.nodoG39ter("k-parte", parte, "9123456-200", "sciolto")
		// verso il prodotto finito della bozza: R4 blocca, R2 rifiuta con il rimedio S8
		if !g.r4Nodo(nodo, a, "k-root") {
			t.Errorf("R4 non blocca la PB dello STEP non univoco verso il prodotto finito")
		}
		prima := fotoDelDatabase(t, g.b)
		if st, testo := g.r2Nodo(nodo, a, "k-root"); st != http.StatusUnprocessableEntity ||
			!strings.Contains(testo, `"codice":"`+distintabozza.CodiceStepCandidatoDelFinito+`"`) || !strings.Contains(testo, "S8") {
			t.Fatalf("R2 con la PB dello STEP non univoco verso il prodotto finito: %d %.800s", st, testo)
		}
		if d := differenze(prima, fotoDelDatabase(t, g.b)); d != "" {
			t.Errorf("il rifiuto ha scritto in: %s", d)
		}
		// verso la parte: la scelta dell'utente (G39-06 [U])
		if g.r4Nodo(nodo, a, "k-parte") {
			t.Errorf("R4 blocca la PB dello STEP non univoco verso la parte 9123456-200")
		}
		if st, testo := g.r2Nodo(nodo, a, "k-parte"); st != http.StatusOK || strings.Contains(testo, distintabozza.CodiceStepCandidatoDelFinito) {
			t.Fatalf("R2 con la PB dello STEP non univoco verso la parte 9123456-200: %d %.800s", st, testo)
		}
		righe := g.righeS9(a)
		salvata := false
		for _, r := range righe {
			if strings.HasPrefix(r, "in_conflitto/") {
				t.Errorf("S9: la PB salvata verso la parte e' in conflitto: %v", righe)
			}
			salvata = salvata || strings.HasPrefix(r, "salvato_nella_bozza/")
		}
		if !salvata {
			t.Errorf("S9: manca la riga della PB salvata verso la parte: %v", righe)
		}
	})
	t.Run("univoca invariata", func(t *testing.T) {
		g := nuovaG39bis(t)
		fin := g.componente("9123456A", "finito")
		parte := g.componente("9123456-200", "sciolto")
		a := g.allegato("9123456-001.stp", "stp", 'k')
		accendiMotoreV1(t, g.b, g.s.thread)
		if c := g.candidature()[a]; len(c) != 1 || c[0].ProdottoID != fin || !c[0].Univoca {
			t.Fatalf("lo STEP 9123456-001.stp non e' il candidato univoco del finito (riscontro): %+v", c)
		}
		if _, ok := g.proposteS9(fin)[a]; !ok {
			t.Errorf("S9 non propone lo STEP univoco come sorgente del finito")
		}
		nodo := g.nodoG39ter("k-parte", parte, "9123456-200", "sciolto")
		if !g.r4Nodo(nodo, a, "k-parte") {
			t.Errorf("R4 non blocca la PB dello STEP univoco verso la parte")
		}
		if st, testo := g.r2Nodo(nodo, a, "k-parte"); st != http.StatusUnprocessableEntity ||
			!strings.Contains(testo, `"codice":"`+distintabozza.CodiceStepCandidatoDelFinito+`"`) {
			t.Fatalf("R2 con la PB dello STEP univoco verso la parte: %d %.800s", st, testo)
		}
	})
}
