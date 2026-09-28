//go:build integrazione

// L4 — Smistamento F9: la lettura strutturata del testo dei PDF (`dettagli.testo_pdf`, analizzatore 4) dal
// risultato del worker alla lettura di ogni copia, e la rianalisi dei PDF letti dall'analizzatore 3, contro
// PostgreSQL vero. Il testo e' del CONTENUTO e passa a tutte le copie (P15: il nome no); il codice di famiglia
// del cartiglio conferma il nome, lo contraddice o da' il codice a un file che nel nome non ce l'ha, con le
// regole del cliente della RFQ; un fatto senza testo (worker vecchio) non inventa niente (A5.13.8, A5.14.3,
// Domanda 7 = B). Un PDF con i soli fatti v3 e' «testo non letto» finche' una persona non chiede «Rianalizza»;
// la rianalisi porta i fatti v4 e non tocca le decisioni. Lo stesso vale per un PDF con i fatti v4 ma senza il
// testo (un worker non aggiornato ha risposto a un job della 4): «Rianalizza» lo riaccoda con la stessa chiave.

package workerapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

const regoleACME712 = `{"famiglie_codice": [{"regex": "(?P<codice>712\\d{4})", "descrizione": "ACME 712", "esempio": "7120001"}]}`

// testoDisegno e' il fatto `testo_pdf` di un disegno finto: il cartiglio in basso a destra della pagina 1 (con
// il campo «DISEGNO N.») e una nota con un altro codice nel resto della pagina.
func testoDisegno() worker.TestoPDF {
	return worker.TestoPDF{Versione: 1, Estraibile: true, Pagine: 1, PagineLette: 1, Caratteri: 90, FormatoPagina1: []float64{842, 595},
		Frammenti: []worker.FrammentoPDF{
			{Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Testo: "TOLLERANZE GENERALI\nDISEGNO N. 7120010\nSCALA 1:2",
				Riquadro: []float64{560, 490, 724, 543}},
			{Pagina: 1, Zona: worker.ZonaPagina, Fonte: worker.FonteTestoNativo, Testo: "NOTA 7120012", Riquadro: []float64{40, 50, 120, 63}}},
		Cartiglio: []worker.CampoCartiglio{{Etichetta: worker.CampoNumeroDisegno, Letta: "DISEGNO N.", Valore: "7120010", Pagina: 1,
			Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Riquadro: []float64{620, 530, 655, 543}}},
		Metadati: worker.MetadatiPDF{Creatore: "CAD"},
		OCR:      worker.OCRPDF{Stato: worker.OCRNonNecessario},
		Limiti:   worker.LimitiTestoPDF{PagineMax: 11, FrammentiMax: 200, FrammentoMax: 512, CaratteriMax: 16384, CampiMax: 24}}
}

// dettagliDisegno sono i dettagli dell'analizzatore 4 per il disegno finto.
func dettagliDisegno(t *testing.T) json.RawMessage {
	t.Helper()
	dett, err := json.Marshal(map[string]any{"cartiglio": true, "termini_trovati": []string{"SCALA", "TOLLERANZE GENERALI"},
		"fonti": map[string]string{"tipo": "termini_pdf", "codice": "nome_file", "rev": "nome_file"}, "testo_pdf": testoDisegno()})
	if err != nil {
		t.Fatal(err)
	}
	return dett
}

// TestIlTestoDelPdfEntraNellaLetturaDiOgniCopia (F9): lo stesso disegno con tre nomi nella RFQ, e il worker
// che risponde con la lettura del testo. Il cartiglio dice 7120010: conferma «7120010_1.pdf» (due fonti, score
// 85 dal contenuto), da' il codice a «tavola.pdf» (che nel nome non ne ha) e contraddice «7120011.pdf»
// (discorde, e in colonna resta il nome: il nome non si corregge). Una copia che scende dopo riceve gli stessi
// fatti. Il testo resta nei fatti conservati; un PDF analizzato da un worker vecchio (senza testo) resta con il
// suo nome, e il suo testo e' «non letto».
func TestIlTestoDelPdfEntraNellaLetturaDiOgniCopia(t *testing.T) {
	b := nuovoBancoLetture(t)
	thread := b.rfq(regoleACME712)
	contenuto := []byte("%PDF-1.4 disegno 7120010")
	sha := shaDi(contenuto)
	nomi := []string{"7120010_1.pdf", "tavola.pdf", "7120011.pdf"}
	var copie []db.Allegato
	for _, n := range nomi {
		a := b.allegato(b.mail(thread), n, sha, int64(len(contenuto)))
		b.esegui(`INSERT INTO documento_proposta (allegato_id, tipo_proposto, confidenza, fonte) VALUES ($1, 'da_determinare', 30, 'estensione')`, a.AllegatoID)
		copie = append(copie, a)
	}
	// il worker ha analizzato la prima copia: codice e rev in testa sono il SUO nome
	b.analisi(copie[0], worker.RisultatoAnalisi{TipoProposto: "disegno_2d", Codice: "7120010", Rev: "1", Confidenza: 95,
		Fonte: "cartiglio", Dettagli: dettagliDisegno(t)})

	attese := []struct {
		colonne string
		codice  string
		stato   string
	}{
		{"disegno_2d:7120010:1:85:cartiglio", "7120010", classificazione.StatoConcorde},
		{"disegno_2d:7120010::85:cartiglio", "7120010", classificazione.StatoUnica},
		{"disegno_2d:7120011::70:nome_file", "7120010", classificazione.StatoDiscorde},
	}
	for i, a := range copie {
		l := b.lettura(a.AllegatoID)
		att := attese[i]
		if l.colonne() != att.colonne || !l.laRiassume() || l.v.Codice.Valore != att.codice || l.v.Codice.Stato != att.stato {
			t.Errorf("%s: colonne %s (attese %s), codice %q %s (atteso %q %s), riassume %v", a.NomeFile, l.colonne(), att.colonne,
				l.v.Codice.Valore, l.v.Codice.Stato, att.codice, att.stato, l.laRiassume())
		}
		if !l.v.HaEvidenza("pdf_testo_famiglia") {
			t.Errorf("%s: il cartiglio e' fra le evidenze del codice: %+v", a.NomeFile, l.v.Codice.Evidenze)
		}
		for _, e := range l.v.Codice.Evidenze {
			if e.Valore == "7120012" {
				t.Errorf("%s: un codice del resto del testo non e' una lettura del codice (P33): %+v", a.NomeFile, e)
			}
		}
	}
	if s := b.testo(`SELECT coalesce(fatti -> 'testo_pdf' ->> 'versione', '') FROM analisi_fatti WHERE sha256 = $1`, sha); s != "1" {
		t.Errorf("la lettura del testo sta nei fatti conservati del contenuto: versione %q", s)
	}

	// una copia che scende dopo riceve i fatti che ci sono gia': il testo passa, il nome e' il suo
	dopo := b.allegato(b.mail(thread), "7120010.pdf", sha, int64(len(contenuto)))
	b.scende(dopo)
	if l := b.lettura(dopo.AllegatoID); l.colonne() != "disegno_2d:7120010::85:cartiglio" || l.v.Codice.Stato != classificazione.StatoConcorde ||
		l.v.Da != classificazione.DaFattiEsistenti {
		t.Errorf("la copia arrivata dopo: %s, codice %s (da %s)", l.colonne(), l.v.Codice.Stato, l.v.Da)
	}

	// un worker vecchio risponde senza testo_pdf: nessuna lettura dal testo, il nome resta com'e'
	vecchio := b.allegato(b.mail(thread), "7120012.pdf", shaDi([]byte("%PDF-1.4 altro")), 14)
	b.esegui(`INSERT INTO documento_proposta (allegato_id, tipo_proposto, confidenza, fonte) VALUES ($1, 'da_determinare', 30, 'estensione')`, vecchio.AllegatoID)
	b.analisi(vecchio, worker.RisultatoAnalisi{TipoProposto: "disegno_2d", Codice: "7120012", Confidenza: 95, Fonte: "cartiglio",
		Dettagli: json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA"]}`)})
	if l := b.lettura(vecchio.AllegatoID); l.colonne() != "disegno_2d:7120012::70:nome_file" || l.v.HaEvidenza("pdf_testo_famiglia") {
		t.Errorf("il fatto senza testo: %s, evidenze %+v", l.colonne(), l.v.Codice.Evidenze)
	}
	if stato := classificazione.StatoDelTestoPDF(json.RawMessage(b.testo(`SELECT fatti::text FROM analisi_fatti WHERE sha256 = $1`, vecchio.Sha256.String))); stato != classificazione.TestoNonLetto {
		t.Errorf("il fatto del worker vecchio e' «testo non letto», non «senza testo»: %q", stato)
	}
}

// TestIPdfConISoliFattiV3SiRianalizzanoSoloAChiederlo (F9, analizzatore 3 → 4; precisazioni del 27/09): tre copie
// dello stesso disegno analizzate con la 3 (fatti v3, senza testo): una aperta con la lettura dal nome, una
// confermata, una aperta ma decisa da una persona (B8.7b); e un altro PDF v3 il cui contenuto non e' piu' in
// staging. Con il server alla 4:
//   - il testo delle copie e' «non letto, da rianalizzare», e la preparazione della pagina non accoda niente
//     (nessuna rianalisi in massa che nessuno ha chiesto);
//   - «Rianalizza» accoda UNA analisi v4 per il contenuto (non tre), conta il PDF senza staging, e ripetuto dice
//     «gia' in coda» senza un secondo job;
//   - il risultato v4 porta il testo: i fatti v4 si aggiungono (i v3 restano), la copia aperta prende la
//     lettura nuova (cartiglio concorde, score 85), la confermata non cambia di un byte, quella decisa da una
//     persona tiene le sue colonne e riceve la lettura solo in `lettura_dopo`; nessuna identita' tecnica nasce;
//   - dopo, «Rianalizza» non ha piu' niente da fare per quel contenuto.
func TestIPdfConISoliFattiV3SiRianalizzanoSoloAChiederlo(t *testing.T) {
	b := nuovoBancoLetture(t)
	v3 := coda.Analizzatore{Versione: 3, Parametri: map[string]any{}}
	v4 := coda.Analizzatore{Versione: 4, Parametri: map[string]any{}}
	b.s.Analizzatore = v4
	thread := b.rfq(regoleACME712)
	contenuto := []byte("%PDF-1.4 disegno v3 7120010")
	sha := shaDi(contenuto)
	file := filepath.Join(t.TempDir(), sha+".pdf")
	if err := os.WriteFile(file, contenuto, 0o600); err != nil {
		t.Fatal(err)
	}
	copia := func(nome, stato, codice, rev, fonte string, conf int) db.Allegato {
		t.Helper()
		a := b.allegato(b.mail(thread), nome, sha, int64(len(contenuto)))
		b.esegui(`UPDATE allegato SET stato = 'analizzato', path_staging = $2 WHERE allegato_id = $1`, a.AllegatoID, file)
		b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte, stato, dettagli)
			VALUES ($1, $2, 'disegno_2d', $3, nullif($4, ''), $5, $6::fonte_proposta, $7::stato_proposta, '{"cartiglio": true, "termini_trovati": ["SCALA"]}')`,
			a.AllegatoID, thread, codice, rev, conf, fonte, stato)
		a, err := b.q.GetAllegato(b.ctx, a.AllegatoID)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	aperta := copia("7120010_1.pdf", "aperta", "7120010", "1", "nome_file", 70)
	confermata := copia("7120010.pdf", "confermata", "7120010", "", "operatore", 100)
	decisa := copia("tavola.pdf", "aperta", "7120011", "A", "operatore", 100)
	fattiV3 := `{"cartiglio": true, "termini_trovati": ["SCALA"], "esito": {"tipo_proposto": "disegno_2d", "codice": "7120010", "rev": "1", "confidenza": 95, "fonte": "cartiglio"}}`
	b.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 3, $2, $3)`, sha, v3.Hash(), fattiV3)
	// un altro PDF v3, il cui contenuto la cache ha tolto
	altro := shaDi([]byte("%PDF-1.4 sparito"))
	sparito := b.allegato(b.mail(thread), "7120012.pdf", altro, 16)
	b.esegui(`UPDATE allegato SET stato = 'analizzato', path_staging = NULL WHERE allegato_id = $1`, sparito.AllegatoID)
	b.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 3, $2, '{"testo_letto": 3}')`, altro, v3.Hash())
	if sparito, err := b.q.GetAllegato(b.ctx, sparito.AllegatoID); err == nil {
		defer func() {
			// il PDF che non si e' potuto riaccodare resta «testo non letto»
			m, _ := fascicolo.MotoreDellaRfq(b.ctx, b.q, thread)
			if l, _ := fascicolo.TestoCorrenteDelPDF(b.ctx, b.q, sparito, v4, m); l.Stato != classificazione.TestoNonLetto {
				t.Errorf("il PDF v3 senza staging: %+v", l)
			}
		}()
	}

	riga := func(a db.Allegato) string {
		return b.testo(`SELECT row_to_json(p)::text FROM documento_proposta p WHERE allegato_id = $1`, a.AllegatoID)
	}
	colonne := func(a db.Allegato) string {
		return b.testo(`SELECT concat_ws(':', tipo_proposto, codice, rev, confidenza, fonte, stato) FROM documento_proposta WHERE allegato_id = $1`, a.AllegatoID)
	}
	identita := func() string {
		return b.testo(`SELECT concat_ws(':', (SELECT count(*) FROM componente), (SELECT count(*) FROM componente_relazione),
			(SELECT count(*) FROM documento), (SELECT count(*) FROM documento_provenienza))`)
	}
	primaConfermata, primaDecisa, primaIdentita := riga(confermata), colonne(decisa), identita()
	lavori := func() int {
		n, _ := strconv.Atoi(b.testo(`SELECT count(*)::text FROM job WHERE tipo = 'analizza_allegato'`))
		return n
	}
	motore, err := fascicolo.MotoreDellaRfq(b.ctx, b.q, thread)
	if err != nil {
		t.Fatal(err)
	}

	// prima: «testo non letto», e aprire la pagina non accoda
	for _, a := range []db.Allegato{aperta, confermata, decisa} {
		l, err := fascicolo.TestoCorrenteDelPDF(b.ctx, b.q, a, v4, motore)
		if err != nil || l.Stato != classificazione.TestoNonLetto || l.Frase == "" || len(l.Letture) != 0 {
			t.Errorf("%s con i soli fatti v3: %+v (%v)", a.NomeFile, l, err)
		}
	}
	if ri, err := fascicolo.AccodaAnalisiMancantiDaSola(b.ctx, b.q, thread, v4, 10); err != nil || ri.Accodati+ri.PdfAccodati != 0 || lavori() != 0 {
		t.Errorf("la preparazione accoda da sola i PDF v3: %+v, job %d (%v)", ri, lavori(), err)
	}

	// «Rianalizza»: una analisi v4 per il contenuto, il PDF senza staging contato
	rianalizza := func() fascicolo.Rianalisi {
		t.Helper()
		ri, err := fascicolo.RianalizzaRfq(b.ctx, b.q, thread, v4, 10)
		if err != nil {
			t.Fatal(err)
		}
		return ri
	}
	if ri := rianalizza(); ri.PdfAccodati != 1 || ri.PdfGiaInCoda != 0 || ri.PdfSenzaStaging != 1 || ri.Accodati != 0 {
		t.Errorf("prima rianalisi: %+v", ri)
	}
	chiave := coda.ChiaveAnalisi(sha, v4)
	if n := b.testo(`SELECT count(*)::text FROM job WHERE chiave_idempotenza = $1 AND stato = 'pronto'
		AND (payload ->> 'versione_analizzatore')::int = 4`, chiave); n != "1" || lavori() != 1 {
		t.Errorf("job v4 per il contenuto: %s (in tutto %d), atteso uno", n, lavori())
	}
	if ri := rianalizza(); ri.PdfAccodati != 0 || ri.PdfGiaInCoda != 1 || lavori() != 1 {
		t.Errorf("la rianalisi ripetuta raddoppia: %+v, job %d", ri, lavori())
	}

	// il worker nuovo risponde al job accodato
	var jobID int64
	if err := b.pool.QueryRow(b.ctx, `UPDATE job SET stato = 'in_corso' WHERE chiave_idempotenza = $1 RETURNING job_id`, chiave).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	j, err := b.q.GetJob(b.ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	var p worker.PayloadAnalizzaAllegato
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		t.Fatal(err)
	}
	dati, _ := json.Marshal(worker.RisultatoAnalisi{AllegatoID: p.AllegatoID, TipoProposto: "disegno_2d", Codice: "7120010", Rev: "1",
		Confidenza: 95, Fonte: "cartiglio", Dettagli: dettagliDisegno(t), VersioneAnalizzatore: 4, HashConfigurazione: v4.Hash()})
	tx, err := b.pool.Begin(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.s.applicaRisultato(b.ctx, db.New(tx), &j, dati, nil); err != nil {
		_ = tx.Rollback(b.ctx)
		t.Fatalf("il risultato v4 non si applica: %v", err)
	}
	if err := tx.Commit(b.ctx); err != nil {
		t.Fatal(err)
	}

	// i fatti: v3 intatti, v4 con il testo
	if s := b.testo(`SELECT fatti::text FROM analisi_fatti WHERE sha256 = $1 AND versione_analizzatore = 3`, sha); !jsonUguali(s, fattiV3) {
		t.Errorf("i fatti v3 restano sotto la loro chiave: %s", s)
	}
	if s := b.testo(`SELECT coalesce(fatti -> 'testo_pdf' ->> 'versione', '') FROM analisi_fatti WHERE sha256 = $1 AND versione_analizzatore = 4`, sha); s != "1" {
		t.Errorf("i fatti v4 portano il testo: %q", s)
	}
	// le decisioni: la confermata non cambia di un byte, quella decisa da una persona tiene le colonne
	if s := riga(confermata); s != primaConfermata {
		t.Errorf("la proposta confermata e' stata riscritta:\n prima %s\n dopo  %s", primaConfermata, s)
	}
	if s := colonne(decisa); s != primaDecisa {
		t.Errorf("la proposta decisa da una persona ha cambiato colonne: %s (prima %s)", s, primaDecisa)
	}
	if s := b.testo(`SELECT coalesce(dettagli -> 'lettura_dopo' ->> 'codice', '') FROM documento_proposta WHERE allegato_id = $1`, decisa.AllegatoID); s != "7120010" {
		t.Errorf("la lettura nuova accanto alla decisione, in lettura_dopo: %q", s)
	}
	// la copia aperta prende la lettura nuova, con il cartiglio che conferma il nome
	if l := b.lettura(aperta.AllegatoID); l.colonne() != "disegno_2d:7120010:1:85:cartiglio" || l.v.Codice.Stato != classificazione.StatoConcorde || !l.laRiassume() {
		t.Errorf("la copia aperta dopo la rianalisi: %s, %s", l.colonne(), l.v.Codice.Stato)
	}
	if s := identita(); s != primaIdentita {
		t.Errorf("la rianalisi ha creato identita' tecniche: %s (prima %s)", s, primaIdentita)
	}
	l, err := fascicolo.TestoCorrenteDelPDF(b.ctx, b.q, aperta, v4, motore)
	if err != nil || l.Stato != classificazione.TestoLetto || len(l.Letture) == 0 || l.Letture[0].Codice != "7120010" ||
		l.Letture[0].Fonte != classificazione.FonteTestoCartiglio || l.Letture[0].DipendeDaNome || l.Letture[0].Etichetta != worker.CampoNumeroDisegno {
		t.Errorf("il testo corrente dopo la rianalisi: %+v (%v)", l, err)
	}

	// dopo: niente da rianalizzare per quel contenuto (il PDF senza staging resta contato)
	if ri := rianalizza(); ri.PdfAccodati != 0 || ri.PdfGiaInCoda != 0 || ri.PdfSenzaStaging != 1 || lavori() != 1 {
		t.Errorf("dopo i fatti v4: %+v, job %d", ri, lavori())
	}
	// un allegato senza analisi e' «da analizzare», non «non letto»
	nuovo := b.allegato(b.mail(thread), "nuovo.pdf", shaDi([]byte("%PDF-1.4 nuovo")), 14)
	if l, _ := fascicolo.TestoCorrenteDelPDF(b.ctx, b.q, nuovo, v4, motore); l.Stato != classificazione.TestoDaAnalizzare {
		t.Errorf("un PDF mai analizzato: %+v", l)
	}
}

// TestUnPdfLettoDaUnWorkerVecchioSiRianalizzaAChiederlo (F9, analizzatore 3 → 4, giro di correzione 2; precisazioni
// del 27/09): i fatti con la chiave CORRENTE ci sono, ma senza `testo_pdf`, perche' un worker non aggiornato ha
// risposto a un job della 4 (il passo 1 della procedura in cockpit.toml.example). Prima il testo era «non letto,
// da rianalizzare» e nessun gesto poteva rianalizzarlo: AccodaPdfDaRileggere e coda.AccodaAnalisi si fermavano ai
// fatti correnti, e «Rianalizza» rispondeva «niente da rileggere» fino al prossimo cambio di versione. Adesso:
//   - il testo e' «non letto» con una frase che nomina il worker non aggiornato, e aprire la pagina non accoda;
//   - «Rianalizza» accoda UNA analisi con la chiave corrente (coda.RiaccodaAnalisi) e ripetuto dice «gia' in coda»;
//   - il risultato del worker aggiornato riscrive i fatti sotto la stessa chiave (una riga, ora con il testo), la
//     copia aperta prende la lettura nuova, la confermata non cambia di un byte, quella decisa da una persona
//     tiene le colonne e riceve la lettura solo in `lettura_dopo`; nessuna identita' tecnica nasce;
//   - dopo, «Rianalizza» non ha piu' niente da fare; un PDF illeggibile con i fatti correnti non si riaccoda.
func TestUnPdfLettoDaUnWorkerVecchioSiRianalizzaAChiederlo(t *testing.T) {
	b := nuovoBancoLetture(t)
	v4 := coda.Analizzatore{Versione: 4, Parametri: map[string]any{}}
	b.s.Analizzatore = v4
	thread := b.rfq(regoleACME712)
	contenuto := []byte("%PDF-1.4 disegno v4 senza testo 7120010")
	sha := shaDi(contenuto)
	file := filepath.Join(t.TempDir(), sha+".pdf")
	if err := os.WriteFile(file, contenuto, 0o600); err != nil {
		t.Fatal(err)
	}
	copia := func(nome, sha, stato, codice, rev, fonte string, conf int) db.Allegato {
		t.Helper()
		a := b.allegato(b.mail(thread), nome, sha, int64(len(contenuto)))
		b.esegui(`UPDATE allegato SET stato = 'analizzato', path_staging = $2 WHERE allegato_id = $1`, a.AllegatoID, file)
		b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, rev, confidenza, fonte, stato, dettagli)
			VALUES ($1, $2, 'disegno_2d', $3, nullif($4, ''), $5, $6::fonte_proposta, $7::stato_proposta, '{"cartiglio": true, "termini_trovati": ["SCALA"]}')`,
			a.AllegatoID, thread, codice, rev, conf, fonte, stato)
		a, err := b.q.GetAllegato(b.ctx, a.AllegatoID)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	aperta := copia("7120010_1.pdf", sha, "aperta", "7120010", "1", "nome_file", 70)
	confermata := copia("7120010.pdf", sha, "confermata", "7120010", "", "operatore", 100)
	decisa := copia("tavola.pdf", sha, "aperta", "7120011", "A", "operatore", 100)
	// i fatti della 4 scritti da un worker vecchio: l'esito c'e', il testo no
	b.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 4, $2, $3)`, sha, v4.Hash(),
		`{"cartiglio": true, "termini_trovati": ["SCALA"], "testo_letto": 1, "esito": {"tipo_proposto": "disegno_2d", "codice": "7120010", "rev": "1", "confidenza": 95, "fonte": "cartiglio"}}`)
	// un altro PDF con i fatti correnti che dicono «non si apre»: e' una risposta, non si riaccoda
	rotto := shaDi([]byte("%PDF-1.4 rotto"))
	copia("7120012.pdf", rotto, "aperta", "7120012", "", "nome_file", 70)
	b.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 4, $2, '{"errore_pdf": "cannot open broken document"}')`,
		rotto, v4.Hash())

	riga := func(a db.Allegato) string {
		return b.testo(`SELECT row_to_json(p)::text FROM documento_proposta p WHERE allegato_id = $1`, a.AllegatoID)
	}
	colonne := func(a db.Allegato) string {
		return b.testo(`SELECT concat_ws(':', tipo_proposto, codice, rev, confidenza, fonte, stato) FROM documento_proposta WHERE allegato_id = $1`, a.AllegatoID)
	}
	identita := func() string {
		return b.testo(`SELECT concat_ws(':', (SELECT count(*) FROM componente), (SELECT count(*) FROM componente_relazione),
			(SELECT count(*) FROM documento), (SELECT count(*) FROM documento_provenienza))`)
	}
	lavori := func() int {
		n, _ := strconv.Atoi(b.testo(`SELECT count(*)::text FROM job WHERE tipo = 'analizza_allegato'`))
		return n
	}
	primaConfermata, primaDecisa, primaIdentita := riga(confermata), colonne(decisa), identita()
	motore, err := fascicolo.MotoreDellaRfq(b.ctx, b.q, thread)
	if err != nil {
		t.Fatal(err)
	}

	// prima: «testo non letto», con la frase vera, e aprire la pagina non accoda
	for _, a := range []db.Allegato{aperta, confermata, decisa} {
		l, err := fascicolo.TestoCorrenteDelPDF(b.ctx, b.q, a, v4, motore)
		if err != nil || l.Stato != classificazione.TestoNonLetto || !strings.Contains(l.Frase, "worker non aggiornato") || len(l.Letture) != 0 {
			t.Errorf("%s con i fatti v4 senza testo: %+v (%v)", a.NomeFile, l, err)
		}
	}
	if ri, err := fascicolo.AccodaAnalisiMancantiDaSola(b.ctx, b.q, thread, v4, 10); err != nil || ri.Accodati+ri.PdfAccodati != 0 || lavori() != 0 {
		t.Errorf("la preparazione accoda da sola: %+v, job %d (%v)", ri, lavori(), err)
	}

	rianalizza := func() fascicolo.Rianalisi {
		t.Helper()
		ri, err := fascicolo.RianalizzaRfq(b.ctx, b.q, thread, v4, 10)
		if err != nil {
			t.Fatal(err)
		}
		return ri
	}
	// «Rianalizza»: una analisi con la chiave corrente per il contenuto, non per copia; il PDF illeggibile no
	if ri := rianalizza(); ri.PdfAccodati != 1 || ri.PdfGiaInCoda != 0 || ri.PdfSenzaStaging != 0 || ri.Accodati != 0 {
		t.Errorf("prima rianalisi: %+v", ri)
	}
	chiave := coda.ChiaveAnalisi(sha, v4)
	if n := b.testo(`SELECT count(*)::text FROM job WHERE chiave_idempotenza = $1 AND stato = 'pronto'
		AND (payload ->> 'versione_analizzatore')::int = 4`, chiave); n != "1" || lavori() != 1 {
		t.Errorf("job per il contenuto: %s (in tutto %d), atteso uno", n, lavori())
	}
	if ri := rianalizza(); ri.PdfAccodati != 0 || ri.PdfGiaInCoda != 1 || lavori() != 1 {
		t.Errorf("la rianalisi ripetuta raddoppia: %+v, job %d", ri, lavori())
	}

	// il worker aggiornato risponde al job accodato
	var jobID int64
	if err := b.pool.QueryRow(b.ctx, `UPDATE job SET stato = 'in_corso' WHERE chiave_idempotenza = $1 RETURNING job_id`, chiave).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	j, err := b.q.GetJob(b.ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	var p worker.PayloadAnalizzaAllegato
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		t.Fatal(err)
	}
	dati, _ := json.Marshal(worker.RisultatoAnalisi{AllegatoID: p.AllegatoID, TipoProposto: "disegno_2d", Codice: "7120010", Rev: "1",
		Confidenza: 95, Fonte: "cartiglio", Dettagli: dettagliDisegno(t), VersioneAnalizzatore: 4, HashConfigurazione: v4.Hash()})
	tx, err := b.pool.Begin(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.s.applicaRisultato(b.ctx, db.New(tx), &j, dati, nil); err != nil {
		_ = tx.Rollback(b.ctx)
		t.Fatalf("il risultato del worker aggiornato non si applica: %v", err)
	}
	if err := tx.Commit(b.ctx); err != nil {
		t.Fatal(err)
	}

	// i fatti: la stessa chiave, una riga sola, ora con il testo
	if s := b.testo(`SELECT count(*)::text || ':' || coalesce(max(fatti -> 'testo_pdf' ->> 'versione'), '') FROM analisi_fatti WHERE sha256 = $1`, sha); s != "1:1" {
		t.Errorf("i fatti correnti si riscrivono sotto la loro chiave, con il testo: %q", s)
	}
	if s := riga(confermata); s != primaConfermata {
		t.Errorf("la proposta confermata e' stata riscritta:\n prima %s\n dopo  %s", primaConfermata, s)
	}
	if s := colonne(decisa); s != primaDecisa {
		t.Errorf("la proposta decisa da una persona ha cambiato colonne: %s (prima %s)", s, primaDecisa)
	}
	if s := b.testo(`SELECT coalesce(dettagli -> 'lettura_dopo' ->> 'codice', '') FROM documento_proposta WHERE allegato_id = $1`, decisa.AllegatoID); s != "7120010" {
		t.Errorf("la lettura nuova accanto alla decisione, in lettura_dopo: %q", s)
	}
	if l := b.lettura(aperta.AllegatoID); l.colonne() != "disegno_2d:7120010:1:85:cartiglio" || l.v.Codice.Stato != classificazione.StatoConcorde || !l.laRiassume() {
		t.Errorf("la copia aperta dopo la rianalisi: %s, %s", l.colonne(), l.v.Codice.Stato)
	}
	if s := identita(); s != primaIdentita {
		t.Errorf("la rianalisi ha creato identita' tecniche: %s (prima %s)", s, primaIdentita)
	}
	if l, err := fascicolo.TestoCorrenteDelPDF(b.ctx, b.q, aperta, v4, motore); err != nil || l.Stato != classificazione.TestoLetto || len(l.Letture) == 0 {
		t.Errorf("il testo corrente dopo la rianalisi: %+v (%v)", l, err)
	}

	// dopo: niente da rianalizzare
	if ri := rianalizza(); ri.PdfAccodati != 0 || ri.PdfGiaInCoda != 0 || lavori() != 1 {
		t.Errorf("dopo i fatti con il testo: %+v, job %d", ri, lavori())
	}
}

// jsonUguali confronta due documenti JSON per contenuto (jsonb riordina le chiavi).
func jsonUguali(a, b string) bool {
	var x, y any
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return false
	}
	xa, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return string(xa) == string(yb)
}
