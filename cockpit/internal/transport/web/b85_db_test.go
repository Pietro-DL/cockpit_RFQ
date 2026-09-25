//go:build integrazione

// L4 — B8.5 nelle rotte: la domanda «aggiungi o sostituisci» quando un file entra in un componente che
// ha gia' un documento corrente dello stesso tipo (decisione del 24/09/2026 sulla condizione aperta di
// A4.10), la rilettura degli STEP all'apertura della RFQ, i gesti sulle proposte di struttura.

package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

func (b *bancoWeb) sostituitoDa(doc uuid.UUID) string {
	b.t.Helper()
	d, err := b.q.GetDocumento(b.ctx, doc)
	if err != nil {
		b.t.Fatal(err)
	}
	if !d.SostituitoDa.Valid {
		return "-"
	}
	return d.SostituitoDa.UUID.String()
}

// Un file che entra in un componente con un disegno corrente vuole la risposta: senza, niente cambia.
// «aggiungi» lascia correnti tutti e due; un predecessore preciso diventa sostituito dal file nuovo.
func TestAssegnareAUnPezzoConUnDisegnoCorrenteVuoleLaScelta(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "SCELTA")
	comp := r.componente("P1")
	altro := r.componente("Q2")
	foglio1 := r.documento("foglio1.pdf", "P1", comp, db.StatoNasScritto)
	diQ2 := r.documento("q2.pdf", "Q2", altro, db.StatoNasScritto)
	foglio2 := r.documento("foglio2.pdf", "P1", uuid.Nil, db.StatoNasInCoda)
	rev := r.documento("foglio1 rev B.pdf", "P1", uuid.Nil, db.StatoNasInCoda)
	w := operatore(b)

	prima := b.foto(foglio2)
	a := r.assegna(w, url.Values{"componente": {comp.String()}, "documento": {foglio2.String()}})
	if !strings.Contains(a, "Assegnazione non riuscita") || !strings.Contains(a, "foglio1.pdf") || !strings.Contains(a, "«aggiungi»") {
		t.Fatalf("senza scelta: %q", a)
	}
	if b.foto(foglio2) != prima {
		t.Fatalf("senza scelta il documento e' cambiato: %s", b.foto(foglio2))
	}
	for _, sbagliata := range []string{"boh", diQ2.String(), foglio2.String()} {
		a = r.assegna(w, url.Values{"componente": {comp.String()}, "documento": {foglio2.String()}, "scelta": {sbagliata}})
		if !strings.Contains(a, "Assegnazione non riuscita") || b.foto(foglio2) != prima {
			t.Errorf("scelta %q: doveva rifiutare senza cambiare niente: %q", sbagliata, a)
		}
	}

	a = r.assegna(w, url.Values{"componente": {comp.String()}, "documento": {foglio2.String()}, "scelta": {"aggiungi"}})
	if !strings.Contains(a, "assegnato al componente P1") || !strings.Contains(a, "Si aggiunge") {
		t.Fatalf("aggiungi: %q", a)
	}
	if b.sostituitoDa(foglio1) != "-" || b.sostituitoDa(foglio2) != "-" {
		t.Errorf("con «aggiungi» restano correnti tutti e due")
	}

	// adesso i correnti sono due: la revisione nuova nomina quello che sostituisce
	a = r.assegna(w, url.Values{"componente": {comp.String()}, "documento": {rev.String()},
		"scelta_" + rev.String(): {foglio1.String()}})
	if !strings.Contains(a, "foglio1.pdf sostituito da foglio1 rev B.pdf") {
		t.Fatalf("sostituisci: %q", a)
	}
	if b.sostituitoDa(foglio1) != rev.String() || b.sostituitoDa(foglio2) != "-" {
		t.Errorf("la sostituzione riguarda un predecessore preciso: foglio1 → %s, foglio2 → %s", b.sostituitoDa(foglio1), b.sostituitoDa(foglio2))
	}
}

// La stessa domanda la fa la conferma, quando il file diventa documento gia' assegnato.
func TestLaConfermaConComponenteVuoleLaScelta(t *testing.T) {
	b := preparaBancoWeb(t)
	ImpostaCapacitaProva(t, tutteAccese)
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "SCELTA2")
	comp := r.componente("P1")
	vecchio := r.documento("P1 rev A.pdf", "P1", comp, db.StatoNasScritto)
	p := r.proposta("P1 rev B.pdf", "P1", uuid.Nil)
	w := operatore(b)

	avviso := confermaProposta(w, p, url.Values{"componente_id": {comp.String()}})
	if !strings.Contains(avviso, "Conferma non riuscita") || !strings.Contains(avviso, "P1 rev A.pdf") {
		t.Fatalf("senza scelta: %s", estrai(avviso, "avviso"))
	}
	if n := b.contaNelThread("documento", r.thread); n != 1 {
		t.Fatalf("%d documenti dopo una conferma rifiutata", n)
	}
	if b.fotoProposta(p) != "componente=- codice=P1 stato=aperta" {
		t.Errorf("la proposta deve restare aperta: %s", b.fotoProposta(p))
	}
	avviso = confermaProposta(w, p, url.Values{"componente_id": {comp.String()}, "scelta": {vecchio.String()}, "rev": {"B"}, "codice": {"P1"}})
	if !strings.Contains(avviso, "Assegnato al componente P1") || !strings.Contains(avviso, "P1 rev A.pdf sostituito da P1 rev B.pdf") {
		t.Fatalf("con la scelta: %s", estrai(avviso, "avviso"))
	}
	var nuovo uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT documento_id FROM documento WHERE thread_id = $1 AND nome_file = 'P1 rev B.pdf'`, r.thread).Scan(&nuovo); err != nil {
		t.Fatal(err)
	}
	if b.sostituitoDa(vecchio) != nuovo.String() {
		t.Errorf("il vecchio doveva essere sostituito dal nuovo: %s", b.sostituitoDa(vecchio))
	}
}

// stepNellaRfq e' uno STEP arrivato nella RFQ con i suoi fatti per l'analizzatore an (o senza, se
// fatti e' vuoto).
func (r *rfqFascicolo) stepNellaRfq(nome, fatti string, an coda.Analizzatore) uuid.UUID {
	r.b.t.Helper()
	id, sha := r.allegato(nome)
	if _, err := r.b.pool.Exec(r.b.ctx, `UPDATE allegato SET estensione = 'stp' WHERE allegato_id = $1`, id); err != nil {
		r.b.t.Fatal(err)
	}
	if fatti != "" {
		if _, err := r.b.pool.Exec(r.b.ctx, `INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, $2, $3, $4)`,
			sha, an.Versione, an.Hash(), fatti); err != nil {
			r.b.t.Fatal(err)
		}
	}
	return id
}

const fattiAssieme = `{"struttura": {"versione": 3, "schema": "AP214", "radici": ["#1"], "avvisi": [],
	"nodi": [{"chiave": "#1", "id_grezzo": "52922757", "nome_grezzo": "52922757", "evidenza": {}},
	         {"chiave": "#2", "id_grezzo": "52920517", "nome_grezzo": "52920517", "evidenza": {}}],
	"relazioni": [{"padre": "#1", "figlio": "#2", "qta": 4, "evidenza": {}}],
	"limiti": {"troncato": false}, "scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0,
	"occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`

// Aprire la RFQ rilegge i suoi STEP: quello con i fatti correnti diventa proposte, quello senza si
// accoda al worker. Poi i gesti: accettare tutto il file porta nodi e arco nella BOM, e un secondo
// «accetta» sulla stessa proposta risponde con il rifiuto, non con un errore.
func TestAprireLaRfqRileggeGliStepEIGestiRispondonoConLaPagina(t *testing.T) {
	b := preparaBancoWeb(t)
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "STEP85")
	letto := r.stepNellaRfq("assieme.stp", fattiAssieme, an)
	r.stepNellaRfq("nuovo.stp", "", an)
	w := operatore(b)

	if risp, _ := w.fai(http.MethodGet, "/thread/"+r.thread.String(), nil, false); risp.StatusCode != 200 {
		t.Fatalf("apertura: %d", risp.StatusCode)
	}
	if n := r.conta(`SELECT count(*) FROM componente_proposta WHERE thread_id = $1 AND stato = 'aperta'`, r.thread); n != 2 {
		t.Errorf("proposte di nodo aperte dopo l'apertura: %d, attese 2", n)
	}
	if n := r.conta(`SELECT count(*) FROM job WHERE tipo = 'analizza_allegato'`); n != 1 {
		t.Errorf("analisi accodate all'apertura: %d, attesa 1 (lo STEP senza fatti correnti)", n)
	}
	if n := r.conta(`SELECT count(*) FROM componente WHERE thread_id = $1`, r.thread); n != 0 {
		t.Errorf("aprire la RFQ ha creato %d componenti", n)
	}
	// una seconda apertura non accoda di nuovo e non riscrive
	w.fai(http.MethodGet, "/thread/"+r.thread.String(), nil, false)
	if n := r.conta(`SELECT count(*) FROM job WHERE tipo = 'analizza_allegato'`); n != 1 {
		t.Errorf("la seconda apertura ha accodato di nuovo: %d", n)
	}

	_, html := w.fai(http.MethodPost, "/thread/"+r.thread.String()+"/fascicolo/rianalizza", url.Values{}, true)
	if a := avvisoDi(html); !strings.Contains(a, "1 STEP riletto") || !strings.Contains(a, "1 analisi già in coda") {
		t.Errorf("rianalizza: %q", a)
	}

	_, html = w.fai(http.MethodPost, fmt.Sprintf("/thread/%s/fascicolo/file/%s/accetta", r.thread, letto), url.Values{}, true)
	if a := avvisoDi(html); a != "Accettati 2 nodi e 1 relazione." {
		t.Fatalf("accetta il file: %q", a)
	}
	if n := r.conta(`SELECT count(*) FROM componente_relazione WHERE thread_id = $1 AND qta = 4`, r.thread); n != 1 {
		t.Errorf("l'arco ×4 non e' nella BOM")
	}
	var pid uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT proposta_id FROM componente_proposta WHERE thread_id = $1 AND chiave = '#2'`, r.thread).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	_, html = w.fai(http.MethodPost, fmt.Sprintf("/thread/%s/fascicolo/nodo/%s/accetta", r.thread, pid), url.Values{}, true)
	if a := avvisoDi(html); !strings.Contains(a, "Niente è cambiato") || !strings.Contains(a, "già decisa") {
		t.Errorf("una proposta decisa due volte: %q", a)
	}
	_, html = w.fai(http.MethodPost, fmt.Sprintf("/thread/%s/fascicolo/nodo/%s/accetta", r.thread, "non-un-id"), url.Values{}, true)
	if a := avvisoDi(html); !strings.Contains(a, "proposta non valido") {
		t.Errorf("id non valido: %q", a)
	}
}
