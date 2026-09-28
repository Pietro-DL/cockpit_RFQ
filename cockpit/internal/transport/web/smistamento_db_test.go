//go:build integrazione

// L4 — Smistamento F8 nel web: gli inneschi del flusso ancorato al prodotto (addendum A5.13.6). L'aggancio
// della RFQ (IN1, prova 188), la correzione del codice di un nodo (IN7, prova 189), «Rianalizza» (IN8) e
// «Aggiorna le proposte» (IN9, POST, almeno operatore). Le GET non scrivono (prova 98, nessunaget_db_test.go):
// qui si controlla che la rotta nuova non si apra a chi consulta.

package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
)

// destinazioneDi e' dettagli.destinazione della proposta dell'allegato (nil se non c'e').
func destinazioneDi(t *testing.T, b *bancoWeb, allegato uuid.UUID) *fascicolo.Destinazione {
	t.Helper()
	var raw []byte
	if err := b.pool.QueryRow(b.ctx, `SELECT coalesce(dettagli -> 'destinazione', 'null'::jsonb) FROM documento_proposta WHERE allegato_id = $1`,
		allegato).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var d *fascicolo.Destinazione
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func righeDi(d *fascicolo.Destinazione) string {
	if d == nil {
		return "(nessuna destinazione)"
	}
	var parti []string
	for _, c := range d.Candidati {
		x := c.Codice + " " + c.Regola
		if c.Bloccato != "" {
			x += " [" + c.Bloccato + "]"
		}
		parti = append(parti, x)
	}
	return d.Esito + ": " + strings.Join(parti, " | ")
}

// rfqConStep: una RFQ di ACME con il prodotto 7120001 confermato e lo STEP del prodotto letto con «Rianalizza»
// (IN8: il gesto fa girare anche il flusso). nodo2 e' il codice del figlio #2 nel file.
func rfqConStep(t *testing.T, b *bancoWeb, chiave, nodo2 string) (*rfqFascicolo, *browser, uuid.UUID) {
	t.Helper()
	acme := b.clienteDiProva("ACME", "ACME S.p.A.", "acme.example")
	r := b.rfqFascicolo(acme, chiave)
	r.fase("FATTIBILITA")
	r.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confidenza, confermato_da) VALUES ($1, '7120001', 'proposta_famiglia', 80, $2)`,
		r.thread, r.utente)
	r.componenteTipo("7120001", "finito")
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{}}
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	step := r.stepNellaRfq("7120001.stp", fattiDi("#1", []string{"#1=7120001", "#2=" + nodo2}, []string{"#1>#2*1"}), an)
	w := operatore(b)
	_, html := w.daFascicolo(http.MethodPost, r.base()+"/rianalizza", url.Values{}, r.thread, "")
	if a := avvisoF(html); !strings.Contains(a, "STEP riletti") && !strings.Contains(a, "STEP riletto") {
		t.Fatalf("rianalizza: %q", a)
	}
	return r, w, step
}

// disegnoLetto e' un PDF gia' letto come disegno, nel messaggio msg, con la proposta nella RFQ thread (o in
// nessuna).
func disegnoLetto(t *testing.T, r *rfqFascicolo, msg uuid.UUID, nome string, thread uuid.NullUUID) uuid.UUID {
	t.Helper()
	a, _ := r.allegatoExt(nome, "pdf")
	r.esegui(`UPDATE allegato SET messaggio_id = $2 WHERE allegato_id = $1`, a, msg)
	r.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte, dettagli)
		VALUES ($1, $2, 'disegno_2d', $3, 75, 'cartiglio', '{"cartiglio": true, "termini_trovati": ["SCALA"]}')`,
		a, thread, strings.TrimSuffix(nome, ".pdf"))
	return a
}

// Prova 188 (IN1, critica C6): l'aggancio fa partire il flusso sui file gia' analizzati. Il disegno arriva in
// una mail non ancora agganciata, letto; agganciandola alla RFQ il suo codice trova il figlio dello STEP del
// prodotto: la destinazione c'e' subito, senza aspettare un'altra analisi.
func TestLAggancioFaPartireIlFlussoSuiFileGiaAnalizzati(t *testing.T) {
	b := preparaBancoWeb(t)
	r, w, _ := rfqConStep(t, b, "F8-AGGANCIO", "7120010")
	msg := b.messaggioIn("<f8-aggancio@acme.example>")
	disegno := disegnoLetto(t, r, msg, "7120010.pdf", uuid.NullUUID{})
	if d := destinazioneDi(t, b, disegno); d != nil {
		t.Fatalf("prima dell'aggancio il disegno non e' di nessuna RFQ: %s", righeDi(d))
	}
	resp, html := w.fai(http.MethodPost, "/messaggio/"+msg.String()+"/aggancia", url.Values{"thread_id": {r.thread.String()}}, true)
	if resp.StatusCode != 200 || !strings.Contains(leggibile(html), "Agganciato alla RFQ") {
		t.Fatalf("aggancio: %d %s", resp.StatusCode, estrai(html, "avviso"))
	}
	d := destinazioneDi(t, b, disegno)
	if got := righeDi(d); got != "proposta: 7120010 dest_nodo_non_autorizzato [step_non_autorizzato]" {
		t.Errorf("dopo l'aggancio: %s", got)
	}
}

// Prova 189 (IN7): correggere il codice di un nodo rifa' le destinazioni. Il figlio #2 dello STEP e' letto
// 7120555; il disegno 7120010 non trova il suo pezzo («nessuna»). Una persona scrive il codice giusto del nodo:
// il flusso gira dopo il gesto, e il disegno trova il figlio.
func TestCorreggereIlCodiceDiUnNodoRifaLeDestinazioni(t *testing.T) {
	b := preparaBancoWeb(t)
	r, w, step := rfqConStep(t, b, "F8-CODICE", "7120555")
	disegno := disegnoLetto(t, r, r.msg, "7120010.pdf", uuid.NullUUID{UUID: r.thread, Valid: true})
	_, html := w.daFascicolo(http.MethodPost, "/thread/"+r.thread.String()+"/smistamento/aggiorna", url.Values{}, r.thread, "")
	if a := avvisoF(html); !strings.Contains(a, "Proposte aggiornate") {
		t.Fatalf("aggiorna le proposte: %q", a)
	}
	if got := righeDi(destinazioneDi(t, b, disegno)); got != "nessuna: " {
		t.Fatalf("prima della correzione: %s", got)
	}
	var nodo uuid.UUID
	if err := b.pool.QueryRow(b.ctx, `SELECT proposta_id FROM componente_proposta WHERE allegato_id = $1 AND chiave = '#2'`, step).Scan(&nodo); err != nil {
		t.Fatal(err)
	}
	_, html = w.daFascicolo(http.MethodPost, r.base()+"/nodo/"+nodo.String()+"/codice", url.Values{"codice": {"7120010"}}, r.thread, "")
	if a := avvisoF(html); strings.Contains(a, "Niente è cambiato") {
		t.Fatalf("il codice del nodo: %q", a)
	}
	if got := righeDi(destinazioneDi(t, b, disegno)); got != "proposta: 7120010 dest_nodo_non_autorizzato [step_non_autorizzato]" {
		t.Errorf("dopo la correzione del nodo: %s", got)
	}
}

// IN9: «Aggiorna le proposte» e' un POST da operatore. Chi consulta non lo puo' usare, e il rifiuto non scrive
// niente; l'operatore riscrive le destinazioni, e la seconda volta non c'e' niente da riscrivere.
func TestAggiornaLeProposteEUnPostDaOperatore(t *testing.T) {
	b := preparaBancoWeb(t)
	r, w, _ := rfqConStep(t, b, "F8-AGGIORNA", "7120010")
	disegno := disegnoLetto(t, r, r.msg, "7120010.pdf", uuid.NullUUID{UUID: r.thread, Valid: true})
	co := b.browser("10.0.0.9")
	co.login("CO", "prova-co")
	resp, _ := co.fai(http.MethodPost, "/thread/"+r.thread.String()+"/smistamento/aggiorna", url.Values{}, true)
	if resp.StatusCode == http.StatusOK && destinazioneDi(t, b, disegno) != nil {
		t.Fatal("chi consulta ha aggiornato le proposte")
	}
	if d := destinazioneDi(t, b, disegno); d != nil {
		t.Errorf("il rifiuto ha scritto una destinazione: %s", righeDi(d))
	}
	_, html := w.daFascicolo(http.MethodPost, "/thread/"+r.thread.String()+"/smistamento/aggiorna", url.Values{}, r.thread, "")
	if a := avvisoF(html); !strings.Contains(a, "Proposte aggiornate") {
		t.Fatalf("l'operatore: %q", a)
	}
	if d := destinazioneDi(t, b, disegno); d == nil || d.Esito != fascicolo.EsitoProposta {
		t.Errorf("dopo «Aggiorna le proposte»: %s", righeDi(d))
	}
	_, html = w.daFascicolo(http.MethodPost, "/thread/"+r.thread.String()+"/smistamento/aggiorna", url.Values{}, r.thread, "")
	if a := avvisoF(html); !strings.Contains(a, "già aggiornate") {
		t.Errorf("la seconda volta: %q", a)
	}
}
