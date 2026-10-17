//go:build integrazione

package web

// L4 — LS-01 (17/10/2026; correzioni dei verificatori, punto 8): le frasi VERE del gesto «Rianalizza» su uno STEP letto
// nella forma 4 del worker di GP-3, attraverso la rotta (POST /thread/{id}/fascicolo/rianalizza) e l'avviso della
// pagina, come le legge l'operatore. Sono le frasi del pacchetto di LS-01 (LEGGIMI-PACCHETTO.md §4), che prima di questa
// prova erano scritte a memoria: uno STEP della RFQ con i fatti correnti si rilegge sempre con le regole del cliente
// (RileggiStepDellaRfq), quindi su uno STEP v4 il gesto non dice mai «Niente da rileggere…», né prima né dopo LS-01.
//
//   - Uno STEP v4 nello staging: «1 STEP riletto con le regole del cliente, 1 STEP accodato per rileggerne la
//     struttura.»; ripremuto mentre l'analisi è in coda: «…, 1 STEP già in coda per rileggerne la struttura.»; dopo il
//     risultato del worker corretto (la forma 3): «1 STEP riletto con le regole del cliente.», la stessa frase che il
//     gesto dava su uno STEP v4 prima di LS-01, quando non lo riaccodava.
//   - Uno STEP v4 non più nello staging: «1 STEP riletto con le regole del cliente, 1 STEP da rileggere non più in
//     staging (Riscarica, poi di nuovo Rianalizza).»; quando il file è di nuovo nello staging, il gesto lo riaccoda.
//     Che «Riscarica» da solo non lo rilegga lo prova la L4 del workerapi (TestLS01Rianalizza…NonAccettata, che passa
//     per dopoStaging).
//
// Il risultato del worker qui si scrive con l'SQL (i fatti nella forma 3 sotto la stessa chiave, il job chiuso): la
// frase dipende solo da quello che c'è nel database, e la strada vera del risultato la provano le L4 del workerapi.
// Gli STEP e i codici sono inventati (ACME, 712xxxx): il repository è pubblico.

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// fattiStepGP3: i fatti di uno STEP come li scriveva il worker di GP-3, la struttura `versione: 4` con i campi della
// diagnostica ma senza `versione_diagnostica`; per il resto una lettura completa (una radice, un'occorrenza d'assieme,
// niente scartato).
const fattiStepGP3 = `{"struttura": {"versione": 4, "schema": "AP214", "radici": ["#1"], "avvisi": [],
	"nodi": [{"chiave": "#1", "id_grezzo": "7120600", "nome_grezzo": "7120600", "evidenza": {},
	          "geometria": {"solidi": 0, "rappresentazioni": 1, "vuota": true}},
	         {"chiave": "#2", "id_grezzo": "7120601", "nome_grezzo": "7120601", "evidenza": {},
	          "geometria": {"solidi": 1, "rappresentazioni": 1, "vuota": false}}],
	"relazioni": [{"padre": "#1", "figlio": "#2", "qta": 2, "evidenza": {},
	               "istanze": [{"rif": "#9", "id": "NAUO1", "nome": "7120601:1"}, {"rif": "#10", "id": "NAUO2", "nome": "7120601:2"}]}],
	"limiti": {"troncato": false}, "scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0,
	"occorrenze_su_se_stesse": 0, "testi_troncati": 0}, "stato": "valida", "motivo_stato": ""}}`

func TestLS01IlGestoRianalizzaDiceLeFrasiVereSuUnoStepV4(t *testing.T) {
	b := preparaBancoWeb(t)
	an := coda.Analizzatore{Versione: 4, Parametri: map[string]any{}}
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	cliente := b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example")
	w := operatore(b)
	rianalizza := func(r *rfqFascicolo) string {
		t.Helper()
		resp, html := w.fai(http.MethodPost, "/thread/"+r.thread.String()+"/fascicolo/rianalizza", url.Values{}, true)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("rianalizza: stato %d", resp.StatusCode)
		}
		return avvisoDi(html)
	}
	// stepV4: uno STEP della RFQ con i fatti di GP-3 sotto la chiave corrente; restituisce il suo sha e il percorso nello
	// staging (rfqFascicolo.allegato ce lo scrive davvero)
	stepV4 := func(r *rfqFascicolo, nome string) (sha, percorso string) {
		t.Helper()
		id := r.stepNellaRfq(nome, fattiStepGP3, an)
		if err := b.pool.QueryRow(b.ctx, `SELECT sha256, path_staging FROM allegato WHERE allegato_id = $1`, id).Scan(&sha, &percorso); err != nil {
			t.Fatal(err)
		}
		f, err := b.q.GetAnalisiFatti(b.ctx, db.GetAnalisiFattiParams{Sha256: sha, VersioneAnalizzatore: int16(an.Versione),
			HashConfigurazione: an.Hash()})
		if err != nil || !worker.StrutturaDaRileggere(f.Fatti) {
			t.Fatalf("lo STEP v4 di prova: i fatti devono essere da rileggere (%v)", err)
		}
		return sha, percorso
	}
	frase := func(quando, ottenuta, attesa string) {
		t.Helper()
		if ottenuta != attesa {
			t.Errorf("%s:\n  %q\natteso\n  %q", quando, ottenuta, attesa)
		}
	}

	// Uno STEP v4 nello staging.
	r := b.rfqFascicolo(cliente, "LS01A")
	sha, _ := stepV4(r, "7120600.stp")
	frase("prima pressione", rianalizza(r), "1 STEP riletto con le regole del cliente, 1 STEP accodato per rileggerne la struttura.")
	frase("ripremuto con l'analisi in coda", rianalizza(r),
		"1 STEP riletto con le regole del cliente, 1 STEP già in coda per rileggerne la struttura.")
	// il worker corretto risponde: i fatti nella forma 3, con la diagnostica 1, sotto la stessa chiave; il job si chiude
	v3 := strings.Replace(fattiStepGP3, `"versione": 4,`, `"versione": 3, "versione_diagnostica": 1,`, 1)
	if _, err := b.pool.Exec(b.ctx, `UPDATE analisi_fatti SET fatti = $4, calcolato_il = now()
		WHERE sha256 = $1 AND versione_analizzatore = $2 AND hash_configurazione = $3`, sha, an.Versione, an.Hash(), v3); err != nil {
		t.Fatal(err)
	}
	chiusi, err := b.pool.Exec(b.ctx, `UPDATE job SET stato = 'fatto', chiuso_il = now() WHERE chiave_idempotenza = $1 AND stato = 'pronto'`,
		coda.ChiaveAnalisi(sha, an))
	if err != nil {
		t.Fatal(err)
	}
	if n := chiusi.RowsAffected(); n != 1 {
		t.Fatalf("job dell'analisi riaccodata chiusi: %d, atteso 1", n)
	}
	frase("dopo il risultato del worker corretto", rianalizza(r), "1 STEP riletto con le regole del cliente.")

	// Uno STEP v4 il cui contenuto non è più nello staging (la cache lo ha tolto).
	r2 := b.rfqFascicolo(cliente, "LS01B")
	_, percorso := stepV4(r2, "7120700.stp")
	contenuto, err := os.ReadFile(percorso)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(percorso); err != nil {
		t.Fatal(err)
	}
	frase("STEP v4 non più nello staging", rianalizza(r2),
		"1 STEP riletto con le regole del cliente, 1 STEP da rileggere non più in staging (Riscarica, poi di nuovo Rianalizza).")
	// di nuovo nello staging (dopo «Riscarica»): il gesto lo riaccoda
	if err := os.WriteFile(percorso, contenuto, 0o644); err != nil {
		t.Fatal(err)
	}
	frase("di nuovo nello staging", rianalizza(r2), "1 STEP riletto con le regole del cliente, 1 STEP accodato per rileggerne la struttura.")
}
