//go:build integrazione

// L4 — giro G39-bis (21/10), corsia motore, con il Motore A vero e le regole ACME di prova (sintetiche), sulla regola
// unica dello STEP candidato del finito (propostabom.CandidatureSorgenteDelFinito, distintabozza.StepCandidatiDalMotore):
//   - TestV1G39bisD2cSottoassiemeConLaBaseDelFinitoDB (D-2 (c) [U], D-AR04 (iii) [U]; rilievo bloccante del gate della
//     direttiva 39, P39-08): il finito 9123456A e il sottoassieme 9123456-100 della stessa RFQ; lo STEP
//     «9123456-100.stp», di cui la grammatica di prova legge solo la base 9123456 (quella del finito), è lo STEP del
//     sottoassieme: il nome è il codice di un altro componente della RFQ, quindi non è una candidatura del finito. R2
//     salva la sua PB (200), R4 con il motore non gli dà step_candidato_del_finito, S9 non lo propone come sorgente del
//     finito. Il riscontro: lo STEP «9123456-001.stp» (nessun componente con quel codice) resta il candidato univoco
//     del finito, rifiutato da R2, bloccato da R4 e proposto da S9;
//   - TestV1G39bisNomeUgualeAlCodiceDelFinitoDB (G39-02 [U], la rete di sicurezza [T-39]): lo STEP «9123456A.stp», il
//     cui nome la grammatica di prova non legge (nessun candidato nell'ancoraggio), è il candidato univoco del finito
//     9123456A, con l'evidenza nome_file.codice_del_prodotto: R2 lo rifiuta come PB (422 STEP_CANDIDATO_DEL_FINITO),
//     R4 lo blocca, S9 lo propone. Il riscontro: il PDF «9123456A.pdf» si preassocia (200).
// Tutte e due falliscono sul codice di prima del giro G39-bis (98db3d7).
// Dopo la revisione indipendente del giro G39-bis (punta 1befe91), le tre prove in fondo: i nomi con un suffisso dopo il
// codice del sottoassieme (D-2 (c) [U]), la candidatura non univoca esclusa da R2 e R4 (G39-02 [U]) e il nome esatto
// letto da una grammatica sintetica che legge base e marcatore. Codici della grammatica ACME di prova; il resto è
// inventato.

package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/jsoncanonico"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// g39bis: la scena di bancoBozzaV1 con i componenti e gli allegati della prova, e gli aiuti per R2, R4 e S9.
type g39bis struct {
	t   *testing.T
	b   *bancoWeb
	s   *scenaBozzaV1
	ad  *browser
	msg uuid.UUID
}

func nuovaG39bis(t *testing.T) *g39bis {
	b, s := bancoBozzaV1(t)
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	return &g39bis{t: t, b: b, s: s, ad: ad, msg: uuidSQL(t, b, `SELECT messaggio_id FROM messaggio WHERE thread_id = '`+s.thread.String()+`'`)}
}

func (g *g39bis) componente(codice, tipo string) uuid.UUID {
	g.t.Helper()
	id := uuid.New()
	if _, err := g.b.pool.Exec(g.b.ctx, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da)
		VALUES ($1, $2, $3, $4, 'manuale', $5)`, id, g.s.thread, codice, tipo, g.s.admin); err != nil {
		g.t.Fatal(err)
	}
	return id
}

func (g *g39bis) allegato(nome, estensione string, sha byte) uuid.UUID {
	g.t.Helper()
	id := uuid.New()
	if _, err := g.b.pool.Exec(g.b.ctx, `INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il, natura)
		VALUES ($1, $2, (SELECT COALESCE(max(indice), 0) + 1 FROM allegato WHERE messaggio_id = $2), $3, $4, repeat($5, 64), now(), 'file')`,
		id, g.msg, nome, estensione, string(sha)); err != nil {
		g.t.Fatal(err)
	}
	return id
}

// candidature: le candidature del calcolo vero (la stessa funzione di S9 e, per le univoche, di R2/R4), per allegato.
func (g *g39bis) candidature() map[uuid.UUID][]propostabom.CandidaturaSorgente {
	g.t.Helper()
	ris, err := g.b.ws.servizioMotoreA().Calcola(g.b.ctx, g.s.thread)
	if err != nil {
		g.t.Fatalf("il calcolo del motore: %v", err)
	}
	out := map[uuid.UUID][]propostabom.CandidaturaSorgente{}
	for _, c := range ris.CandidatureSorgente() {
		out[c.AllegatoID] = append(out[c.AllegatoID], c)
	}
	if len(out) == 0 {
		var diag []string
		for _, f := range ris.EsitoThread.Ancoraggi.File {
			raw, _ := json.Marshal(f)
			diag = append(diag, string(raw))
		}
		g.t.Logf("nessuna candidatura; ancoraggi %v", diag)
	}
	return out
}

// r2: il salvataggio R2 della bozza della scena con la sola PB sull'allegato dato (versione attesa 1).
func (g *g39bis) r2(a uuid.UUID) (int, string) {
	m := map[string]any{}
	for k, v := range g.s.snapshot {
		m[k] = v
	}
	pb := map[string]any{"allegato_id": a, "chiave_bozza": "k-root"}
	m["documenti_preassociati"] = []any{pb}
	raw, _ := json.Marshal(map[string]any{"bozza_id": g.s.bozza, "versione_attesa": 1, "prodotto_id": g.s.prodotto, "vocabolario": 2,
		"chiave_salvataggio": uuid.New(), "snapshot": m,
		"azioni": []any{map[string]any{"azione": "file_preassociato", "chiave_bozza": "k-root", "dopo": pb, "origine": "utente"}}})
	resp, testo := g.ad.chiediJSON(http.MethodPost, "/thread/"+g.s.thread.String()+"/motore-a/bozza", string(raw))
	return resp.StatusCode, testo
}

// r4: R4 con ?avvisi_motore=1 sulle PB date (bozza alla versione 1); gli allegati con il blocco step_candidato_del_finito.
func (g *g39bis) r4(aa ...uuid.UUID) map[uuid.UUID]bool {
	g.t.Helper()
	percorso := "/thread/" + g.s.thread.String() + "/motore-a/bom/riepilogo?avvisi_motore=1"
	corpo := func(vb map[string]any, firma string) string {
		m := map[string]any{}
		for k, v := range g.s.snapshot {
			m[k] = v
		}
		var pb []any
		for _, a := range aa {
			pb = append(pb, map[string]any{"allegato_id": a, "chiave_bozza": "k-root"})
		}
		m["documenti_preassociati"] = pb
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
	out := map[uuid.UUID]bool{}
	for _, bl := range r.Blocchi {
		if bl.Codice == distintabozza.BloccoPreassociazioneNonApplicab && bl.Motivo == "step_candidato_del_finito" && bl.AllegatoID != nil {
			out[*bl.AllegatoID] = true
		}
	}
	return out
}

// proposteS9: gli allegati proposti come sorgente del prodotto dato in S9 (fonte.proposte_sorgente[]), con le evidenze.
func (g *g39bis) proposteS9(prodotto uuid.UUID) map[uuid.UUID][]propostabom.EvidenzaCandidatura {
	g.t.Helper()
	resp, corpo := g.ad.chiediJSON(http.MethodGet, "/thread/"+g.s.thread.String()+"/motore-a/dati?sorgente=1", "")
	if resp.StatusCode != http.StatusOK {
		g.t.Fatalf("S9: %d %.800s", resp.StatusCode, corpo)
	}
	var x struct {
		Prodotti []struct {
			ProdottoID *uuid.UUID `json:"prodotto_id"`
			Fonte      struct {
				ProposteSorgente []struct {
					AllegatoID uuid.UUID                         `json:"allegato_id"`
					Evidenze   []propostabom.EvidenzaCandidatura `json:"evidenze"`
				} `json:"proposte_sorgente"`
			} `json:"fonte"`
		} `json:"prodotti"`
	}
	if err := json.Unmarshal([]byte(corpo), &x); err != nil {
		g.t.Fatalf("S9: %v", err)
	}
	out := map[uuid.UUID][]propostabom.EvidenzaCandidatura{}
	trovato := false
	for _, p := range x.Prodotti {
		if p.ProdottoID == nil || *p.ProdottoID != prodotto {
			continue
		}
		trovato = true
		for _, ps := range p.Fonte.ProposteSorgente {
			out[ps.AllegatoID] = ps.Evidenze
		}
	}
	if !trovato {
		g.t.Fatalf("S9 senza la sezione del prodotto %s: %.800s", prodotto, corpo)
	}
	return out
}

func TestV1G39bisD2cSottoassiemeConLaBaseDelFinitoDB(t *testing.T) {
	g := nuovaG39bis(t)
	fin := g.componente("9123456A", "finito")
	g.componente("9123456-100", "sottoassieme")
	stepSotto := g.allegato("9123456-100.stp", "stp", 'c')
	stepFinito := g.allegato("9123456-001.stp", "stp", 'd')
	accendiMotoreV1(t, g.b, g.s.thread)

	cc := g.candidature()
	if len(cc[stepSotto]) != 0 {
		t.Errorf("lo STEP 9123456-100.stp (il sottoassieme 9123456-100) e' una candidatura: %+v", cc[stepSotto])
	}
	if c := cc[stepFinito]; len(c) != 1 || c[0].ProdottoID != fin || !c[0].Univoca {
		t.Fatalf("lo STEP 9123456-001.stp non e' il candidato univoco del finito 9123456A (riscontro): %+v", c)
	}

	// R2: la PB sullo STEP del sottoassieme si salva; quella sullo STEP del finito e' rifiutata
	prima := fotoDelDatabase(t, g.b)
	if st, testo := g.r2(stepFinito); st != http.StatusUnprocessableEntity || !strings.Contains(testo, `"codice":"`+distintabozza.CodiceStepCandidatoDelFinito+`"`) {
		t.Fatalf("R2 con la PB sullo STEP del finito (riscontro): %d %.800s", st, testo)
	}
	if d := differenze(prima, fotoDelDatabase(t, g.b)); d != "" {
		t.Errorf("il rifiuto ha scritto in: %s", d)
	}
	// R4 prima del salvataggio (bozza alla versione 1): il blocco solo per lo STEP del finito
	bloccati := g.r4(stepSotto, stepFinito)
	if bloccati[stepSotto] {
		t.Errorf("R4 da' step_candidato_del_finito allo STEP del sottoassieme")
	}
	if !bloccati[stepFinito] {
		t.Errorf("R4 non da' step_candidato_del_finito allo STEP del finito (riscontro)")
	}
	// S9: la proposta di sorgente del finito e' lo STEP del finito, non quello del sottoassieme
	proposte := g.proposteS9(fin)
	if _, ok := proposte[stepSotto]; ok {
		t.Errorf("S9 propone lo STEP del sottoassieme come sorgente del finito: %+v", proposte)
	}
	if _, ok := proposte[stepFinito]; !ok {
		t.Errorf("S9 non propone lo STEP del finito (riscontro): %+v", proposte)
	}
	if st, testo := g.r2(stepSotto); st != http.StatusOK || strings.Contains(testo, distintabozza.CodiceStepCandidatoDelFinito) {
		t.Fatalf("R2 con la PB sullo STEP del sottoassieme: %d %.800s", st, testo)
	}
}

func TestV1G39bisNomeUgualeAlCodiceDelFinitoDB(t *testing.T) {
	g := nuovaG39bis(t)
	fin := g.componente("9123456A", "finito")
	stepNome := g.allegato("9123456A.stp", "stp", 'e')
	pdfNome := g.allegato("9123456A.pdf", "pdf", 'f')
	accendiMotoreV1(t, g.b, g.s.thread)

	cc := g.candidature()
	c := cc[stepNome]
	if len(c) != 1 || c[0].ProdottoID != fin || !c[0].Univoca || len(c[0].Evidenze) != 1 ||
		c[0].Evidenze[0].Selettore != "nome_file.codice_del_prodotto" || c[0].Evidenze[0].CodiceLetto != "9123456A" {
		t.Fatalf("lo STEP 9123456A.stp non e' il candidato univoco del finito 9123456A con l'evidenza del nome: %+v", c)
	}
	if len(cc[pdfNome]) != 0 {
		t.Errorf("il PDF 9123456A.pdf e' una candidatura: %+v", cc[pdfNome])
	}

	prima := fotoDelDatabase(t, g.b)
	if st, testo := g.r2(stepNome); st != http.StatusUnprocessableEntity || !strings.Contains(testo, `"codice":"`+distintabozza.CodiceStepCandidatoDelFinito+`"`) ||
		!strings.Contains(testo, stepNome.String()) {
		t.Fatalf("R2 con la PB sullo STEP 9123456A.stp: %d %.800s", st, testo)
	}
	if d := differenze(prima, fotoDelDatabase(t, g.b)); d != "" {
		t.Errorf("il rifiuto ha scritto in: %s", d)
	}
	if !g.r4(stepNome)[stepNome] {
		t.Errorf("R4 non da' step_candidato_del_finito allo STEP 9123456A.stp")
	}
	ev, ok := g.proposteS9(fin)[stepNome]
	if !ok || len(ev) != 1 || ev[0].Selettore != "nome_file.codice_del_prodotto" {
		t.Errorf("S9 non propone lo STEP 9123456A.stp con l'evidenza del nome: %v %+v", ok, ev)
	}
	if st, testo := g.r2(pdfNome); st != http.StatusOK {
		t.Fatalf("R2 con la PB sul PDF 9123456A.pdf (riscontro): %d %.800s", st, testo)
	}
}

// L4 — giro G39-bis, revisione indipendente (rilievo bloccante su D-2 (c) [U]): gli STEP del sottoassieme 9123456-100
// con un suffisso dopo il codice («9123456-100_rev2.stp», «9123456-100 A.stp», «9123456-100-B.stp»), di cui la
// grammatica di prova legge solo la base 9123456 (quella del finito 9123456A), non sono candidature del finito: R2 salva
// la loro PB (200), R4 con il motore non dà step_candidato_del_finito, S9 non li propone. Fallisce su 1befe91, dove il
// nome contava solo se era esattamente il codice del sottoassieme.
func TestV1G39bisD2cNomiConSuffissoDelSottoassiemeDB(t *testing.T) {
	for i, nome := range []string{"9123456-100_rev2.stp", "9123456-100 A.stp", "9123456-100-B.stp"} {
		t.Run(nome, func(t *testing.T) {
			g := nuovaG39bis(t)
			fin := g.componente("9123456A", "finito")
			g.componente("9123456-100", "sottoassieme")
			a := g.allegato(nome, "stp", byte('g'+i))
			accendiMotoreV1(t, g.b, g.s.thread)
			if cc := g.candidature(); len(cc[a]) != 0 {
				t.Errorf("lo STEP %s (il sottoassieme 9123456-100) e' una candidatura: %+v", nome, cc[a])
			}
			if g.r4(a)[a] {
				t.Errorf("R4 da' step_candidato_del_finito allo STEP %s del sottoassieme", nome)
			}
			if _, ok := g.proposteS9(fin)[a]; ok {
				t.Errorf("S9 propone lo STEP %s del sottoassieme come sorgente del finito", nome)
			}
			if st, testo := g.r2(a); st != http.StatusOK || strings.Contains(testo, distintabozza.CodiceStepCandidatoDelFinito) {
				t.Fatalf("R2 con la PB sullo STEP %s del sottoassieme: %d %.800s", nome, st, testo)
			}
		})
	}
}

// L4 — giro G39-bis, revisione indipendente (rilievo importante su G39-02 [U]: mai PB né AR ordinaria): lo STEP
// «9123456-001.stp», che la grammatica di prova ancora al finito 9123456A con la sola base, quando la RFQ ha anche il
// finito 9123456-001 (il nome è il suo codice): un altro finito non toglie la candidatura, la rende non univoca. R2
// rifiuta la PB (422 STEP_CANDIDATO_DEL_FINITO) e R4 la blocca come per la candidatura univoca; S9 non la propone come
// sorgente (si conferma con S8, a mano). Fallisce su 1befe91, dove il nome di un altro componente, anche finito, toglieva
// la candidatura e la PB si salvava (200).
func TestV1G39bisNonUnivocaEsclusaDaR2R4DB(t *testing.T) {
	g := nuovaG39bis(t)
	fin := g.componente("9123456A", "finito")
	altro := g.componente("9123456-001", "finito")
	a := g.allegato("9123456-001.stp", "stp", 'j')
	accendiMotoreV1(t, g.b, g.s.thread)

	cc := g.candidature()
	var delFinito *propostabom.CandidaturaSorgente
	for i := range cc[a] {
		if cc[a][i].ProdottoID == fin {
			delFinito = &cc[a][i]
		}
	}
	if delFinito == nil || delFinito.Univoca {
		t.Fatalf("lo STEP 9123456-001.stp non e' una candidatura non univoca del finito 9123456A: %+v", cc[a])
	}
	prima := fotoDelDatabase(t, g.b)
	if st, testo := g.r2(a); st != http.StatusUnprocessableEntity || !strings.Contains(testo, `"codice":"`+distintabozza.CodiceStepCandidatoDelFinito+`"`) {
		t.Fatalf("R2 con la PB sullo STEP della candidatura non univoca: %d %.800s", st, testo)
	}
	if d := differenze(prima, fotoDelDatabase(t, g.b)); d != "" {
		t.Errorf("il rifiuto ha scritto in: %s", d)
	}
	if !g.r4(a)[a] {
		t.Errorf("R4 non da' step_candidato_del_finito allo STEP della candidatura non univoca")
	}
	for _, p := range []uuid.UUID{fin, altro} {
		if _, ok := g.proposteS9(p)[a]; ok {
			t.Errorf("S9 propone lo STEP della candidatura non univoca come sorgente di %s", p)
		}
	}
	// l'esenzione dal motivo di S8 (integrazione, 57ed735) deve guardare solo le univoche (UnivocaDel)
	ris, err := g.b.ws.servizioMotoreA().Calcola(g.b.ctx, g.s.thread)
	if err != nil {
		t.Fatal(err)
	}
	s := distintabozza.StepCandidatiDalMotore(ris.EsitoThread, ris.Thread)
	if !s.Di(a) || s.UnivocaDel(a, fin) {
		t.Errorf("StepCandidatiDalMotore: candidato %v, univoco del finito %v (attesi vero, falso)", s.Di(a), s.UnivocaDel(a, fin))
	}
}

// L4 — giro G39-bis, revisione indipendente (rilievo minore sulla rete del nome): con una grammatica sintetica che legge
// nel nome del file la base e il marcatore insieme (la grammatica ACME di prova con la forma «nome-marcatore» al posto di
// «nome-sottolineato», che leggerebbe la stessa occorrenza: il compilatore rifiuta le due insieme, R25 f), lo
// STEP «9123456A.stp» è il candidato univoco del finito 9123456A per l'ancoraggio (evidenza nome_file con il marcatore
// A), non per la rete del nome esatto (nessuna evidenza nome_file.codice_del_prodotto); R2 lo rifiuta (422), R4 lo
// blocca, S9 lo propone con quell'evidenza.
func TestV1G39bisNomeEsattoLettoDallaGrammaticaDB(t *testing.T) {
	g := nuovaG39bis(t)
	fin := g.componente("9123456A", "finito")
	a := g.allegato("9123456A.stp", "stp", 'k')
	accendiMotoreV1ConForma(t, g.b, g.s.thread, "nome-sottolineato", map[string]any{"id": "nome-marcatore", "selettori": []any{"nome_file"},
		"stato": "attiva", "completa": true, "parti": []any{map[string]any{"tipo": "base", "min": 1, "max": 1},
			map[string]any{"tipo": "marcatore", "letterali": []any{"A"}, "min": 1, "max": 1}}},
		map[string]any{"id": "e-nome-marcatore", "origine": "sintetico", "selettore": "nome_file", "testo": "9123456A.stp",
			"atteso": map[string]any{"letture": []any{map[string]any{"forma": "nome-marcatore", "base": "9123456", "marcatore": "A"}}}})

	c := g.candidature()[a]
	if len(c) != 1 || c[0].ProdottoID != fin || !c[0].Univoca {
		t.Fatalf("lo STEP 9123456A.stp non e' il candidato univoco del finito 9123456A: %+v", c)
	}
	dalNome := false
	for _, e := range c[0].Evidenze {
		if e.Selettore == propostabom.SelettoreNomeFileCodiceProdotto {
			t.Errorf("l'evidenza della rete del nome con la grammatica che legge il nome: %+v", c[0].Evidenze)
		}
		dalNome = dalNome || (e.Selettore == "nome_file" && e.BaseLetta == "9123456" && e.MarcatoreLetto == "A")
	}
	if !dalNome {
		t.Errorf("manca l'evidenza nome_file con la base 9123456 e il marcatore A: %+v", c[0].Evidenze)
	}
	if st, testo := g.r2(a); st != http.StatusUnprocessableEntity || !strings.Contains(testo, `"codice":"`+distintabozza.CodiceStepCandidatoDelFinito+`"`) {
		t.Fatalf("R2 con la PB sullo STEP 9123456A.stp: %d %.800s", st, testo)
	}
	if !g.r4(a)[a] {
		t.Errorf("R4 non da' step_candidato_del_finito allo STEP 9123456A.stp")
	}
	ev, ok := g.proposteS9(fin)[a]
	if !ok || len(ev) == 0 || ev[0].Selettore != "nome_file" {
		t.Errorf("S9 non propone lo STEP 9123456A.stp con l'evidenza del nome letto: %v %+v", ok, ev)
	}
}

// accendiMotoreV1ConForma: come accendiMotoreV1, con la forma data al posto della forma «via» (e dei suoi esempi) nella
// prima famiglia della grammatica ACME di prova, con il suo esempio positivo (grammatica sintetica della prova).
func accendiMotoreV1ConForma(t *testing.T, b *bancoWeb, thread uuid.UUID, via string, forma, esempio map[string]any) {
	t.Helper()
	var cliente uuid.UUID
	var ragione string
	if err := b.pool.QueryRow(b.ctx, `SELECT c.cliente_id, c.ragione_sociale FROM thread_offerta t JOIN cliente c ON c.cliente_id = t.cliente_id
		WHERE t.thread_id = $1`, thread).Scan(&cliente, &ragione); err != nil {
		t.Fatal(err)
	}
	leggi := func(nome string) []byte {
		t.Helper()
		x, err := os.ReadFile(filepath.Join("..", "..", "app", "runtime", "testdata", "motore_a", nome))
		if err != nil {
			t.Fatalf("regole ACME: %v", err)
		}
		return bytes.ReplaceAll(x, []byte("\r\n"), []byte("\n"))
	}
	ind, err := grammatica.LeggiIndice(leggi("indice_acme.v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gr map[string]any
	dec := json.NewDecoder(bytes.NewReader(leggi("acme.v1.json")))
	dec.UseNumber()
	if err := dec.Decode(&gr); err != nil {
		t.Fatal(err)
	}
	cl := gr["cliente"].(map[string]any)
	cl["id"], cl["ragione_sociale"] = cliente.String(), ragione
	fam := gr["famiglie_codice"].([]any)[0].(map[string]any)
	forme := []any{forma}
	for _, f := range fam["forme"].([]any) {
		if f.(map[string]any)["id"] != via {
			forme = append(forme, f)
		}
	}
	esempi := []any{esempio}
	for _, e := range fam["esempi"].([]any) {
		lette, _ := e.(map[string]any)["atteso"].(map[string]any)["letture"].([]any)
		if len(lette) == 0 || lette[0].(map[string]any)["forma"] != via {
			esempi = append(esempi, e)
		}
	}
	fam["forme"], fam["esempi"] = forme, esempi
	raw, err := json.MarshalIndent(gr, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	ind.Grammatiche = []grammatica.VoceIndice{{ClienteID: cliente, File: "acme.v1.json", Sha256: jsoncanonico.Impronta(raw)}}
	ind.Casi = nil
	ins, diag, err := motorea.CompilaInsieme(ind, map[string][]byte{"acme.v1.json": raw})
	if err != nil || ins.Motori[cliente] == nil {
		t.Fatalf("la grammatica sintetica non si attiva: %v %v", err, diag)
	}
	mwAccendi(t, b, motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoPronta, Insieme: &ins, Limite: 60 * time.Second, Attivi: []uuid.UUID{cliente}})
}
