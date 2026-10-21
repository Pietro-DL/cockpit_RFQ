//go:build integrazione

// L4 — D-2 (c) [U] con il Motore A vero e le regole ACME (sintetiche), rilievo importante 1 della revisione della corsia
// persistenza parte A (giro G39, 20-21/10): lo STEP di una parte o di un sottoassieme resta una PB, anche con il
// riconoscimento del motore dello STEP del finito (G39-02 [U]). La scena: il prodotto finito 9123456A (base 9123456,
// marcatore A), il sottoassieme 9234567A e la parte 9312345A della stessa RFQ. Gli STEP:
//   - «9234567A_1.stp»: il nome del sottoassieme con la revisione (forma nome-sottolineato: base 9234567, marcatore A);
//   - «9312345.stp»: il nome della parte con la sola base (forma pacchetto);
//   - «9123456-001.stp»: il nome con la base del prodotto (forma pacchetto, il suffisso -001 non e' un codice della
//     grammatica): e' lo STEP candidato del finito (PM-01, il caso del marcatore), il riscontro che il motore e' calcolato.
// Via HTTP: R2 salva le PB sugli STEP della parte e del sottoassieme (200, provenienza manuale); R4 con ?avvisi_motore=1
// e il motore calcolato non da' il motivo step_candidato_del_finito per loro, e lo da' per lo STEP del finito messo
// nel corpo (il riscontro).

package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
)

func TestV1G39_02D2cStepDiPartiESottoassiemiRestanoPBConIlMotoreVeroDB(t *testing.T) {
	b, s := bancoBozzaV1(t)
	ad := b.browser("10.0.0.8")
	ad.login("AD", "prova-ad")
	base := "/thread/" + s.thread.String() + "/motore-a"
	fin, sotto, parte := uuid.New(), uuid.New(), uuid.New()
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da)
		VALUES ($1, $4, '9123456A', 'finito', 'manuale', $5), ($2, $4, '9234567A', 'sottoassieme', 'manuale', $5),
		($3, $4, '9312345A', 'sciolto', 'manuale', $5)`, fin, sotto, parte, s.thread, s.admin); err != nil {
		t.Fatal(err)
	}
	msg := uuidSQL(t, b, `SELECT messaggio_id FROM messaggio WHERE thread_id = '`+s.thread.String()+`'`)
	allegato := func(nome string, sha byte) uuid.UUID {
		id := uuid.New()
		if _, err := b.pool.Exec(b.ctx, `INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il, natura)
			VALUES ($1, $2, (SELECT COALESCE(max(indice), 0) + 1 FROM allegato WHERE messaggio_id = $2), $3, 'stp', repeat($4, 64), now(), 'file')`,
			id, msg, nome, string(sha)); err != nil {
			t.Fatal(err)
		}
		return id
	}
	stepSotto := allegato("9234567A_1.stp", 'c')
	stepParte := allegato("9312345.stp", 'd')
	stepFinito := allegato("9123456-001.stp", 'e')
	accendiMotoreV1(t, b, s.thread)

	// il riconoscimento del motore sul calcolo vero: solo lo STEP del finito e' candidato, e solo del finito
	ris, err := b.ws.servizioMotoreA().Calcola(b.ctx, s.thread)
	if err != nil {
		t.Fatalf("il calcolo del motore: %v", err)
	}
	cand := distintabozza.StepCandidatiDalMotore(ris.EsitoThread, ris.Thread)
	if cand == nil || !cand.DalMotore {
		t.Fatalf("nessun riconoscimento del motore: %+v", cand)
	}
	diag := func(a uuid.UUID) string {
		for _, f := range ris.EsitoThread.Ancoraggi.File {
			if f.AllegatoID == a {
				raw, _ := json.Marshal(f)
				return string(raw)
			}
		}
		return "(nessun ancoraggio)"
	}
	for _, c := range []struct {
		nome string
		a    uuid.UUID
	}{{"9234567A_1.stp (sottoassieme)", stepSotto}, {"9312345.stp (parte)", stepParte}} {
		if len(cand.Prodotti[c.a]) != 0 {
			t.Errorf("lo STEP %s riconosciuto candidato del finito: %v; ancoraggio %s", c.nome, cand.Prodotti[c.a], diag(c.a))
		}
	}
	if pp := cand.Prodotti[stepFinito]; len(pp) != 1 || pp[0] != fin {
		t.Fatalf("lo STEP 9123456-001.stp non e' il candidato del solo finito 9123456A: %v; ancoraggio %s", pp, diag(stepFinito))
	}

	// R2 via HTTP: le PB sugli STEP del sottoassieme e della parte si salvano, come documenti (provenienza manuale)
	snapshotCon := func(pb []any) map[string]any {
		m := map[string]any{}
		for k, v := range s.snapshot {
			m[k] = v
		}
		m["documenti_preassociati"] = pb
		return m
	}
	pbSotto := map[string]any{"allegato_id": stepSotto, "chiave_bozza": "k-root"}
	pbParte := map[string]any{"allegato_id": stepParte, "chiave_bozza": "k-root"}
	raw, _ := json.Marshal(map[string]any{"bozza_id": s.bozza, "versione_attesa": 1, "prodotto_id": s.prodotto, "vocabolario": 2,
		"chiave_salvataggio": uuid.New(), "snapshot": snapshotCon([]any{pbSotto, pbParte}),
		"azioni": []any{
			map[string]any{"azione": "file_preassociato", "chiave_bozza": "k-root", "dopo": pbSotto, "origine": "utente"},
			map[string]any{"azione": "file_preassociato", "chiave_bozza": "k-root", "dopo": pbParte, "origine": "utente"},
		}})
	resp, testo := ad.chiediJSON(http.MethodPost, base+"/bozza", string(raw))
	if resp.StatusCode != http.StatusOK || strings.Contains(testo, distintabozza.CodiceStepCandidatoDelFinito) {
		t.Fatalf("R2 con le PB sugli STEP del sottoassieme e della parte: %d %s", resp.StatusCode, testo)
	}
	var salvate int
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FROM distinta_bozza_versione v, jsonb_array_elements(v.snapshot->'documenti_preassociati') pb
		WHERE v.bozza_id = $1 AND v.versione = 2 AND pb->>'allegato_id' IN ($2, $3) AND pb->>'provenienza' = 'manuale'`,
		s.bozza, stepSotto.String(), stepParte.String()).Scan(&salvate); err != nil {
		t.Fatal(err)
	}
	if salvate != 2 {
		t.Fatalf("le PB manuali salvate nella versione 2: %d, attese 2", salvate)
	}

	// R4 con il motore calcolato: nessun motivo step_candidato_del_finito per gli STEP del sottoassieme e della parte; il
	// riscontro: lo STEP del finito messo nel corpo lo ha
	percorso := base + "/bom/riepilogo?avvisi_motore=1"
	corpoR4 := func(vb map[string]any, firma string) string {
		m := snapshotCon([]any{pbSotto, pbParte, map[string]any{"allegato_id": stepFinito, "chiave_bozza": "k-root"}})
		if vb != nil {
			m["versione_bom_base"], m["firma_base"] = vb, firma
		}
		m["bozza_id"], m["bozza_versione"] = s.bozza, 2
		raw, _ := json.Marshal(m)
		return string(raw)
	}
	resp, testo = ad.chiediJSON(http.MethodPost, percorso, corpoR4(nil, ""))
	var r0 distintabozza.RiepilogoR4
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &r0) != nil {
		t.Fatalf("R4: %d %s", resp.StatusCode, testo)
	}
	if r0.Base.Stato == distintabozza.BaseSuperata {
		resp, testo = ad.chiediJSON(http.MethodPost, percorso, corpoR4(map[string]any{"conferma_id": nil, "numero": 0,
			"contatore": r0.VersioneCorrente.Contatore}, r0.VersioneCorrente.FirmaStruttura))
	}
	var r distintabozza.RiepilogoR4
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &r) != nil || !r.AvvisiMotoreCalcolati {
		t.Fatalf("R4 con il motore: %d %s", resp.StatusCode, testo)
	}
	conMotivo := map[uuid.UUID]bool{}
	for _, bl := range r.Blocchi {
		if bl.Codice == distintabozza.BloccoPreassociazioneNonApplicab && bl.Motivo == "step_candidato_del_finito" && bl.AllegatoID != nil {
			conMotivo[*bl.AllegatoID] = true
		}
	}
	if conMotivo[stepSotto] || conMotivo[stepParte] {
		t.Errorf("R4 da' step_candidato_del_finito a uno STEP di sottoassieme o parte: %s", testo)
	}
	if !conMotivo[stepFinito] {
		t.Errorf("R4 non da' step_candidato_del_finito allo STEP del finito (riscontro): %s", testo)
	}
}
