//go:build integrazione

// L4 — Smistamento, rilievi della verifica di F1 chiusi con la fase F3 (prove 99 e 100, A5.4.5): il triage non
// completa i file fermi, il limite MaxPreparatiPerApertura vale per tutta la preparazione, e il bottone di
// riserva della preparazione, senza htmx, dice quando non e' riuscita.

package web

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/storage/nas"
)

// Il triage chiama PreparaFile SENZA la rilettura dei file fermi (prova 100, A5.4.5). Uno STEP fermo nello
// staging i cui fatti ci sono gia' (lo stage riusato) resta fermo dopo l'aggancio, anche con la strada del
// workerapi accesa: lo completa la preparazione della pagina (POST …/prepara), che qui fa da controprova (con
// la strada spenta la prova non proverebbe niente). Rimettere s.rileggiFatti() nel triage la fa fallire.
func TestIlTriageNonCompletaIFileFermi(t *testing.T) {
	b := preparaBancoWeb(t)
	wa := b.caricamentoAcceso(t)
	b.ws.Analizzatore = wa.Analizzatore
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	if b.ws.rileggiFatti() == nil {
		t.Fatal("la strada del workerapi per i file fermi e' spenta: la prova non proverebbe niente")
	}
	acme := b.unCliente("Acme S.p.A.", "ACME", "acme.example")
	thread := b.rfqDa(uuid.Nil, acme, `ACME\WIP\2026 09 27 RFQ 7120001 FERMI`, "7120001")
	if _, err := b.pool.Exec(b.ctx, `UPDATE identificativo_thread SET confermato_da = (SELECT utente_id FROM utente WHERE sigla = 'FP') WHERE thread_id = $1`, thread); err != nil {
		t.Fatal(err)
	}
	msg := b.mailConAllegati("RFQ 7120001", "In allegato lo STEP di 7120012.",
		worker.AllegatoIn{Indice: 1, NomeFile: "7120012.stp", Estensione: "stp", Natura: "file", Bytes: 9000})
	stp := uuidSQL(t, b, `SELECT allegato_id FROM allegato WHERE messaggio_id = $1`, msg)
	percorso := filepath.Join(wa.Staging, "7120012.stp")
	if err := os.WriteFile(percorso, []byte("ISO-10303-21; 7120012"), 0o644); err != nil {
		t.Fatal(err)
	}
	sha, _, err := nas.Sha256File(percorso)
	if err != nil {
		t.Fatal(err)
	}
	// i fatti di un'analisi vera portano l'esito: senza, la strada del workerapi lascerebbe il file fermo
	fatti := fattiDi("#1", []string{"#1=7120012"}, nil)
	fatti = `{"esito": {"tipo_proposto": "cad_3d", "codice": "7120012", "confidenza": 90, "fonte": "step"}, ` + fatti[1:]
	if _, err := b.pool.Exec(b.ctx, `UPDATE allegato SET sha256 = $2, path_staging = $3, stato = 'in_staging' WHERE allegato_id = $1`,
		stp, sha, percorso); err != nil {
		t.Fatal(err)
	}
	if _, err := b.pool.Exec(b.ctx, `INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, $2, $3, $4)`,
		sha, wa.Analizzatore.Versione, wa.Analizzatore.Hash(), fatti); err != nil {
		t.Fatal(err)
	}
	stato := func() string {
		t.Helper()
		var s string
		if err := b.pool.QueryRow(b.ctx, `SELECT stato::text FROM allegato WHERE allegato_id = $1`, stp).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}

	w := operatore(b)
	_, html := w.fai(http.MethodPost, "/messaggio/"+msg.String()+"/aggancia", url.Values{"thread_id": {thread.String()}}, true)
	if !strings.Contains(leggibile(html), "Agganciato") {
		t.Fatalf("aggancio: %q", estrai(html, "avviso"))
	}
	if s := stato(); s != "in_staging" {
		t.Fatalf("dopo l'aggancio lo STEP fermo e' %q: il triage l'ha completato (rilettura dei file fermi nel triage)", s)
	}

	_, html = w.daFascicolo(http.MethodPost, "/thread/"+thread.String()+"/fascicolo/prepara", url.Values{}, thread, "")
	if a := avvisoF(html); !strings.Contains(a, "1 file fermo completato con i fatti già calcolati") {
		t.Errorf("la preparazione della pagina completa lo STEP fermo: %q", a)
	}
	if s := stato(); s == "in_staging" {
		t.Error("la preparazione della pagina doveva completare lo STEP fermo")
	}
}

// Il limite MaxPreparatiPerApertura vale per TUTTA la preparazione (prova 99): i file fermi di PreparaFile e
// le analisi degli STEP di AccodaAnalisiMancantiDaSola insieme. Con piu' lavoro del limite un giro ne accoda
// esattamente il limite (prima i file fermi) e rimanda il resto, e lo dice; il giro dopo accoda quello che era
// rimasto. Contare il limite per ciascuna delle due strade la fa fallire.
func TestIlLimiteValePerTuttaLaPreparazione(t *testing.T) {
	b := preparaBancoWeb(t)
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "LIMITE")
	// meta' del limite piu' due per parte: nessuna delle due strade da sola lo supera, insieme si'
	per := MaxPreparatiPerApertura/2 + 2
	var pdf []uuid.UUID
	for i := 0; i < per; i++ {
		id, _ := r.allegato(fmt.Sprintf("71200%02d.pdf", i))
		r.esegui(`UPDATE allegato SET stato = 'in_staging' WHERE allegato_id = $1`, id)
		pdf = append(pdf, id)
		r.stepNellaRfq(fmt.Sprintf("71201%02d.stp", i), "", an)
	}
	analisi := func() int { return r.conta(`SELECT count(*) FROM job WHERE tipo = 'analizza_allegato'`) }

	w := operatore(b)
	_, html := w.daFascicolo(http.MethodPost, r.base()+"/prepara", url.Values{}, r.thread, "")
	atteso := fmt.Sprintf("Preparazione dei file: %d analisi accodate, %d file rimandati al prossimo giro.",
		MaxPreparatiPerApertura, 2*per-MaxPreparatiPerApertura)
	if a := avvisoF(html); a != atteso {
		t.Errorf("avviso: %q, atteso %q", a, atteso)
	}
	if n := analisi(); n != MaxPreparatiPerApertura {
		t.Errorf("analisi accodate in un giro: %d, al piu' %d", n, MaxPreparatiPerApertura)
	}
	if n := r.conta(`SELECT count(*) FROM job j JOIN allegato a ON a.sha256 = j.payload ->> 'sha256'
		WHERE j.tipo = 'analizza_allegato' AND a.allegato_id = ANY($1)`, pdf); n != per {
		t.Errorf("i file fermi vengono prima: %d accodati su %d", n, per)
	}
	w.daFascicolo(http.MethodPost, r.base()+"/prepara", url.Values{}, r.thread, "")
	if n := analisi(); n != 2*per {
		t.Errorf("il giro dopo accoda il resto: %d analisi, attese %d", n, 2*per)
	}
}

// La preparazione chiesta con il bottone di riserva, senza htmx, che non riesce. Prima tornava alla pagina con
// un 303 come se fosse partita, e l'operatore non sapeva niente; ora risponde con la pagina da cui si era
// partiti, intera, e l'avviso al suo posto (niente e' cambiato, e perche'), e il database non ha il lavoro a
// meta'.
func TestLaPreparazioneSenzaHtmxDiceSeNonERiuscita(t *testing.T) {
	b := preparaBancoWeb(t)
	an := coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}
	b.ws.Analizzatore = an
	t.Cleanup(func() { b.ws.Analizzatore = coda.Analizzatore{} })
	r := b.rfqFascicolo(b.clienteDiProva("ACME", "Acme S.p.A.", "acme.example"), "GUASTO")
	fermo, _ := r.allegato("7120010.pdf")
	r.esegui(`UPDATE allegato SET stato = 'in_staging' WHERE allegato_id = $1`, fermo)
	r.stepNellaRfq("7120011.stp", "", an)
	r.esegui(`CREATE OR REPLACE FUNCTION prova_rifiuta_analisi() RETURNS trigger LANGUAGE plpgsql AS $F$
		BEGIN
			IF NEW.tipo = 'analizza_allegato' THEN RAISE EXCEPTION 'prova: questa analisi non si accoda'; END IF;
			RETURN NEW;
		END $F$`)
	r.esegui(`CREATE TRIGGER prova_rifiuta_analisi BEFORE INSERT ON job FOR EACH ROW EXECUTE FUNCTION prova_rifiuta_analisi()`)
	t.Cleanup(func() { _, _ = b.pool.Exec(b.ctx, `DROP TRIGGER IF EXISTS prova_rifiuta_analisi ON job`) })

	w := operatore(b)
	for _, c := range []struct {
		form           url.Values
		avviso, pagina string // l'avviso al suo posto, e un pezzo che c'e' solo nella pagina intera da cui si era partiti
	}{
		{url.Values{"da": {"fascicolo"}}, `<div class="avviso-f no" role="status">Preparazione dei file non riuscita. Niente è cambiato:`,
			`data-base="` + r.base() + `"`},
		{url.Values{}, `<div class="avviso">Preparazione dei file non riuscita. Niente è cambiato:`,
			`<a class="bottone primario" href="` + r.base() + `">Apri il Fascicolo</a>`},
	} {
		resp, html := w.fai(http.MethodPost, r.base()+"/prepara", c.form, false)
		if resp.StatusCode == http.StatusSeeOther {
			t.Fatalf("senza htmx %v: la preparazione non riuscita torna alla pagina in silenzio (303 → %q)", c.form, resp.Header.Get("Location"))
		}
		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("senza htmx %v: %d, atteso 500", c.form, resp.StatusCode)
		}
		// la pagina intera, con il layout e il foglio di stile: prima (giro di correzione di F3) era un frammento
		// nudo, «<div class="avviso errore-box">…</div>» con il collegamento «Torna alla pagina», senza stile
		for _, atteso := range []string{"<!doctype html>", `<link rel="stylesheet" href="/static/style.css`, c.avviso, "questa analisi non si accoda",
			c.pagina} {
			if !strings.Contains(html, atteso) {
				t.Errorf("senza htmx %v: manca %q in %q", c.form, atteso, html)
			}
		}
	}
	// da htmx lo diceva gia': l'avviso nel Fascicolo
	_, html := w.daFascicolo(http.MethodPost, r.base()+"/prepara", url.Values{}, r.thread, "")
	if a := avvisoF(html); !strings.HasPrefix(a, "Niente è cambiato:") {
		t.Errorf("da htmx: %q", a)
	}
	if n := r.conta(`SELECT count(*) FROM job`); n != 0 {
		t.Errorf("la preparazione non riuscita ha lasciato %d job", n)
	}
}
