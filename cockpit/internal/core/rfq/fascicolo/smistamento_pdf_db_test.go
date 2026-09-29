//go:build integrazione

// L4 — Giro 4, fase 4.2: F9 → F8, una porta sola per il PDF, contro PostgreSQL vero. Le evidenze del PDF che il
// flusso consuma vengono dai fatti CORRENTI, normalizzate come le legge TestoCorrenteDelPDF; il disegno del
// prodotto senza STEP e' un'ancora piatta nelle destinazioni scritte; «Rianalizza» passa prima il disegno del
// prodotto; e il gate integrato (precisazione dell'utente del 27/09 sera) su una scena con lo STEP autorizzato,
// il disegno del prodotto con il cartiglio, un PDF senza testo e un file tecnico senza destinazione.

package fascicolo_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// fattiTesto sono i fatti dell'analizzatore 4 di un disegno finto: il testo nativo in basso a destra della pagina
// 1 (il probabile cartiglio), quello nel resto della pagina e il titolo. Tutto vuoto: un PDF senza testo nel
// file, con l'OCR che sulla postazione non c'e'.
func fattiTesto(t *testing.T, bassoDestra, pagina, titolo string) string {
	t.Helper()
	tp := worker.TestoPDF{Versione: 1, Pagine: 1, PagineLette: 1, FormatoPagina1: []float64{842, 595}, Metadati: worker.MetadatiPDF{Titolo: titolo},
		OCR:    worker.OCRPDF{Stato: worker.OCRNonNecessario},
		Limiti: worker.LimitiTestoPDF{PagineMax: 11, FrammentiMax: 200, FrammentoMax: 512, CaratteriMax: 16384}}
	for _, f := range []struct{ zona, testo string }{{worker.ZonaBassoDestra, bassoDestra}, {worker.ZonaPagina, pagina}} {
		if f.testo != "" {
			tp.Frammenti = append(tp.Frammenti, worker.FrammentoPDF{Pagina: 1, Zona: f.zona, Fonte: worker.FonteTestoNativo, Testo: f.testo,
				Riquadro: []float64{560, 490, 724, 543}})
			tp.Caratteri += len(f.testo)
		}
	}
	tp.Estraibile = tp.Caratteri > 0
	if !tp.Estraibile {
		tp.OCR = worker.OCRPDF{Stato: worker.OCRNonDisponibile, Motivo: "motore assente"}
	}
	b, err := json.Marshal(map[string]any{"cartiglio": true, "termini_trovati": []string{"SCALA"}, "testo_pdf": tp})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// disegnoConFatti e' un PDF della RFQ analizzato, con i fatti dati sotto la chiave dell'analizzatore di prova (i
// fatti correnti) e la lettura che l'analisi ne trae; fatti "" = nessun fatto corrente.
func (b *banco) disegnoConFatti(msg uuid.UUID, nome, fatti string) uuid.UUID {
	b.t.Helper()
	b.n++
	sha := fmt.Sprintf("%064x", 90000+b.n)
	a := uno[uuid.UUID](b, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, ricevuto_il, stato)
		VALUES ($1, $2, $3, 'pdf', 'file', 'outlook', 100, $4, now(), 'analizzato') RETURNING allegato_id`, msg, b.n, nome, sha)
	if fatti != "" {
		b.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, $2, $3, $4)`,
			sha, analizzatoreProva.Versione, analizzatoreProva.Hash(), fatti)
	}
	lettura := fatti
	if lettura == "" {
		lettura = `{"cartiglio": true, "termini_trovati": ["SCALA"]}`
	}
	b.letto(a, nome, &classificazione.Esito{Tipo: "disegno_2d", Fonte: "cartiglio"}, lettura)
	return a
}

// statoFlusso e' lo stato del flusso della RFQ, come lo legge un giro.
func (b *banco) statoFlusso() fascicolo.StatoFlusso {
	b.t.Helper()
	var s fascicolo.StatoFlusso
	ok(b.t, b.tx(func(q *db.Queries) (err error) {
		s, err = fascicolo.LeggiStatoFlusso(b.ctx, q, b.thread, analizzatoreProva)
		return err
	}))
	return s
}

func fileDi(s fascicolo.StatoFlusso, id uuid.UUID) fascicolo.FileFlusso {
	for _, f := range s.File {
		if f.AllegatoID == id {
			return f
		}
	}
	return fascicolo.FileFlusso{}
}

func codiciEvidenze(ev []fascicolo.EvidenzaContenutoPDF) string {
	var out []string
	for _, e := range ev {
		out = append(out, e.Fonte+":"+e.Codice)
	}
	return strings.Join(out, " ")
}

// Giro 4, fase 4.2: le evidenze del PDF vengono dai fatti CORRENTI, dalla stessa porta di TestoCorrenteDelPDF.
// «7120002.pdf» ha i fatti di un analizzatore di prima, che nel cartiglio dicevano 7120099, e quelli correnti,
// con 7120002: il flusso vede 7120002, lo stato «letto», e l'ancora piatta del prodotto. Un PDF con i soli fatti
// di prima e' «non letto, da rianalizzare», uno con i fatti correnti senza il testo lo stesso, uno mai analizzato
// «da analizzare»: nessuno ha evidenze. Tolti i fatti correnti, il disegno non ancora piu', la firma dell'indice
// cambia e il giro dopo lo dice.
func TestLeEvidenzeDelPdfVengonoDaiFattiCorrenti(t *testing.T) {
	b := nuovoBanco(t)
	b.acme()
	b.prodottoConfermato("7120002")
	msg := b.mail()
	disegno := b.disegnoConFatti(msg, "7120002.pdf", fattiTesto(t, "DISEGNO N. 7120002\nSCALA 1:1", "", ""))
	shaDisegno := uno[string](b, `SELECT sha256 FROM allegato WHERE allegato_id = $1`, disegno)
	b.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 2, $2, $3)`,
		shaDisegno, hashCfg, fattiTesto(t, "DISEGNO N. 7120099", "", ""))
	vecchio := b.disegnoConFatti(msg, "7120003.pdf", "")
	b.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti)
		SELECT sha256, 2, $2, $3 FROM allegato WHERE allegato_id = $1`, vecchio, hashCfg, fattiTesto(t, "DISEGNO N. 7120003", "", ""))
	senzaTesto := b.disegnoConFatti(msg, "7120004.pdf", `{"cartiglio": true, "termini_trovati": ["SCALA"]}`)
	mai := b.disegnoConFatti(msg, "7120005.pdf", "")
	b.esegui(`UPDATE allegato SET stato = 'in_staging' WHERE allegato_id = $1`, mai)

	s := b.statoFlusso()
	f := fileDi(s, disegno)
	if f.TestoPDF.Stato != classificazione.TestoLetto || f.TestoPDF.Frase != "" || codiciEvidenze(f.EvidenzePDF) != fascicolo.FontePDFCartiglio+":7120002" ||
		f.EvidenzePDF[0].Origine != fascicolo.OrigineFamiglia || f.EvidenzePDF[0].DipendeDaNome {
		t.Errorf("il disegno con i fatti correnti: %+v %+v", f.TestoPDF, f.EvidenzePDF)
	}
	// la stessa porta di TestoCorrenteDelPDF
	var l classificazione.LettureTestoPDF
	ok(t, b.tx(func(q *db.Queries) error {
		a, err := q.GetAllegato(b.ctx, disegno)
		if err != nil {
			return err
		}
		m, err := fascicolo.MotoreDellaRfq(b.ctx, q, b.thread)
		if err != nil {
			return err
		}
		l, err = fascicolo.TestoCorrenteDelPDF(b.ctx, q, a, analizzatoreProva, m)
		return err
	}))
	if l.Stato != f.TestoPDF.Stato || len(l.Letture) != len(f.EvidenzePDF) || l.Letture[0].Codice != f.EvidenzePDF[0].Codice {
		t.Errorf("le evidenze del flusso non sono il testo corrente del PDF: %+v contro %+v", l, f.EvidenzePDF)
	}
	for _, x := range []struct {
		id    uuid.UUID
		stato string
	}{{vecchio, classificazione.TestoNonLetto}, {senzaTesto, classificazione.TestoNonLetto}, {mai, classificazione.TestoDaAnalizzare}} {
		f := fileDi(s, x.id)
		if f.TestoPDF.Stato != x.stato || f.TestoPDF.Frase != classificazione.FraseTestoPDF(x.stato, "") || len(f.EvidenzePDF) != 0 {
			t.Errorf("%s: %+v %+v", f.Nome, f.TestoPDF, f.EvidenzePDF)
		}
	}

	// il giro: l'ancora piatta dal cartiglio corrente
	b.rismista()
	d := b.dest(disegno)
	if righeDest(d) != "1 7120002 dest_componente_contenuto" || d.Preselezionabile || d.Ancore[0].Livello != fascicolo.LivelloPiatta {
		t.Errorf("il disegno del prodotto: %s, preselezionabile %v, %+v", righeDest(d), d.Preselezionabile, d.Ancore)
	}
	firma := d.IndiceFirma

	// senza i fatti correnti il disegno non ancora piu', e il giro lo riscrive
	b.esegui(`DELETE FROM analisi_fatti WHERE sha256 = $1 AND versione_analizzatore = $2`, shaDisegno, analizzatoreProva.Versione)
	if f := fileDi(b.statoFlusso(), disegno); f.TestoPDF.Stato != classificazione.TestoNonLetto || len(f.EvidenzePDF) != 0 {
		t.Errorf("il disegno con i soli fatti di prima: %+v %+v", f.TestoPDF, f.EvidenzePDF)
	}
	b.rismista()
	d = b.dest(disegno)
	if d.Esito != fascicolo.EsitoSospesa || d.IndiceFirma == firma || !strings.Contains(strings.Join(d.Evidenze, " | "), "da rianalizzare") {
		t.Errorf("il disegno senza i fatti correnti: %s, firma cambiata %v, %v", d.Esito, d.IndiceFirma != firma, d.Evidenze)
	}
}

// pdfDaRileggere e' un PDF della RFQ in staging, gia' analizzato da un analizzatore di prima (fatti della
// versione 2 soltanto): «Rianalizza» lo riaccoda.
func (b *banco) pdfDaRileggere(msg uuid.UUID, nome string) uuid.UUID {
	b.t.Helper()
	b.n++
	sha := fmt.Sprintf("%064x", 95000+b.n)
	p := filepath.Join(b.t.TempDir(), sha+".pdf")
	if err := os.WriteFile(p, []byte("%PDF-1.4 "+nome), 0o644); err != nil {
		b.t.Fatal(err)
	}
	id := uno[uuid.UUID](b, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1, $2, $3, 'pdf', 'file', 'outlook', 20, $4, $5, 'analizzato', now()) RETURNING allegato_id`, msg, b.n, nome, sha, p)
	b.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 2, $2, '{"testo_letto": 3}')`, sha, hashCfg)
	b.letto(id, nome, &classificazione.Esito{Tipo: "disegno_2d", Fonte: "cartiglio"}, `{"cartiglio": true, "termini_trovati": ["SCALA"]}`)
	return id
}

// Giro 4, fase 4.2 (A5.13.6): «Rianalizza» passa prima il disegno del prodotto. Tre PDF letti da un analizzatore
// di prima, arrivati in quest'ordine: «vista.pdf», «tavola.pdf» e «7120002.pdf», compatibile per nome con il
// prodotto confermato. Con il limite di una analisi si accoda il disegno del prodotto, con la priorita' 5; gli
// altri due aspettano. Con il limite largo si accodano anche gli altri, alla priorita' di sempre (6), e il disegno
// del prodotto e' gia' in coda. Un'analisi del disegno del prodotto gia' in coda alla 6 passa alla 5 con il gesto
// ripetuto; le altre restano alla 6. La controprova (a mano): senza l'ordine dei compatibili, con il limite di
// uno si accoda «vista.pdf».
func TestRianalizzaPrimaIlDisegnoDelProdotto(t *testing.T) {
	b := nuovoBanco(t)
	b.prodottoConfermato("7120002")
	msg := b.mail()
	vista := b.pdfDaRileggere(msg, "vista.pdf")
	tavola := b.pdfDaRileggere(msg, "tavola.pdf")
	prodotto := b.pdfDaRileggere(msg, "7120002.pdf")
	rianalizza := func(max int) fascicolo.Rianalisi {
		t.Helper()
		var ri fascicolo.Rianalisi
		ok(t, b.tx(func(q *db.Queries) (err error) {
			ri, err = fascicolo.RianalizzaRfq(b.ctx, q, b.thread, analizzatoreProva, max)
			return err
		}))
		return ri
	}
	priorita := func(a uuid.UUID) string {
		return uno[string](b, `SELECT coalesce((SELECT j.priorita::text FROM job j JOIN allegato a ON j.chiave_idempotenza = 'analizza:' || a.sha256 || $2
			WHERE a.allegato_id = $1 AND j.stato = 'pronto'), '-')`, a, fmt.Sprintf(":%d:%s", analizzatoreProva.Versione, analizzatoreProva.Hash()))
	}
	if ri := rianalizza(1); ri.PdfAccodati != 1 || ri.PdfRimandati != 2 || ri.PdfGiaInCoda != 0 {
		t.Errorf("con il limite di uno: %+v", ri)
	}
	if got := priorita(prodotto) + " " + priorita(vista) + " " + priorita(tavola); got != fmt.Sprintf("%d - -", coda.PrioritaAnalisiCompatibile) {
		t.Errorf("le priorita' (disegno del prodotto, vista, tavola): %s, attese «5 - -»", got)
	}
	if ri := rianalizza(10); ri.PdfAccodati != 2 || ri.PdfGiaInCoda != 1 || ri.PdfRimandati != 0 {
		t.Errorf("con il limite largo: %+v", ri)
	}
	if got := priorita(prodotto) + " " + priorita(vista) + " " + priorita(tavola); got != "5 6 6" {
		t.Errorf("le priorita' dopo il limite largo: %s, attese «5 6 6»", got)
	}
	// un'analisi del disegno del prodotto gia' in coda, alla priorita' di sempre, passa avanti
	b.esegui(`UPDATE job SET priorita = 6 WHERE stato = 'pronto'`)
	if ri := rianalizza(10); ri.PdfAccodati != 0 || ri.PdfGiaInCoda != 3 {
		t.Errorf("il gesto ripetuto: %+v", ri)
	}
	if got := priorita(prodotto) + " " + priorita(vista) + " " + priorita(tavola); got != "5 6 6" {
		t.Errorf("le priorita' dopo il gesto ripetuto: %s, attese «5 6 6»", got)
	}
	if n := uno[int](b, `SELECT count(*) FROM job WHERE tipo = 'analizza_allegato'`); n != 3 {
		t.Errorf("un job per contenuto: %d", n)
	}
}

// Il gate integrato (giro 4, fase 4.2; precisazione dell'utente del 27/09 sera: «dopo merge F8+F9, rieseguire il
// gate integrato includendo file tecnici senza destinazione, oltre a guida/autorita' e NAS»). La scena: il
// prodotto ACME 7120001 con lo STEP completo autorizzato (figli diretti 7120010 e 7120012 decisi da una persona,
// guida profonda aperta) e i documenti richiesti gia' sul NAS; poi arrivano il disegno del prodotto con il
// cartiglio («tavola assieme.pdf», che nel nome non ha codici), il disegno di 7120010 senza testo (curve, OCR
// assente) e un DXF 7120099 che non e' un pezzo della RFQ. Un giro del flusso:
//   - le destinazioni: la tavola va al prodotto per il suo cartiglio (dal contenuto, con lo STEP autorizzato), il
//     disegno senza testo va a 7120010 per il solo nome con la frase del suo stato, il DXF non ha destinazione
//     («nessun pezzo corrispondente»), lo STEP e' il 3D del prodotto;
//   - nessuna identita' nasce: componenti, relazioni, documenti, provenienze, proposte di struttura e colonne
//     delle proposte di documento non cambiano;
//   - il NAS non si tocca: i documenti restano come sono, nessun lavoro nuovo in coda;
//   - il gate guarda l'autorita' e non la guida (GateStrutturale a zero, l'avviso dell'assieme con il figlio in
//     guida) e, fino a F11, non si ferma sui file tecnici senza destinazione (A5.4.8: la condizione L1 entra con
//     la decisione): passa, e la V1 si congela.
func TestIlGateIntegratoConIlTestoDeiPdf(t *testing.T) {
	b := nuovoBanco(t)
	sc := b.scenaGate(true)
	msg := b.mail()
	tavola := b.disegnoConFatti(msg, "tavola assieme.pdf", fattiTesto(t, "DISEGNO N. 7120001\nSCALA 1:5", "", ""))
	muto := b.disegnoConFatti(msg, "7120010.pdf", fattiTesto(t, "", "", ""))
	b.n++
	dxf := uno[uuid.UUID](b, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, ricevuto_il, stato)
		VALUES ($1, $2, '7120099.dxf', 'dxf', 'file', 'outlook', 100, $3, now(), 'analizzato') RETURNING allegato_id`, msg, b.n, fmt.Sprintf("%064x", 99000+b.n))
	b.letto(dxf, "7120099.dxf", nil, "")
	b.letto(sc.a.AllegatoID, sc.a.NomeFile, &classificazione.Esito{Tipo: "cad_3d", Fonte: "step"}, "")

	prima, lavori := b.fotoNonFlusso(), uno[int](b, `SELECT count(*) FROM job`)
	nas := uno[string](b, `SELECT string_agg(documento_id::text || ':' || stato_nas::text, ' ' ORDER BY documento_id) FROM documento`)
	es := b.rismista()
	if es.File < 4 || es.Scritte < 4 {
		t.Fatalf("il giro non ha scritto le destinazioni: %+v", es)
	}

	// le destinazioni
	if d := b.dest(tavola); righeDest(d) != "1 7120001 dest_componente_contenuto" || !d.Candidati[0].DalContenuto ||
		!strings.Contains(strings.Join(d.Candidati[0].Evidenze, " | "), "il cartiglio del PDF dice 7120001 (pagina 1)") {
		t.Errorf("la tavola del prodotto: %s %+v", righeDest(d), d.Candidati)
	}
	d := b.dest(muto)
	if righeDest(d) != "1 7120010 dest_nodo_diretto_nome" || d.Candidati[0].DalContenuto ||
		!strings.Contains(strings.Join(d.Evidenze, " | "), "serve l'OCR, che sul worker non c'e': il suo codice viene solo dal nome") {
		t.Errorf("il disegno senza testo: %s %v", righeDest(d), d.Evidenze)
	}
	if d := b.dest(dxf); d.Esito != fascicolo.EsitoNessuna || len(d.Candidati) != 0 {
		t.Errorf("il file tecnico senza destinazione: %s %s", d.Esito, righeDest(d))
	}
	if d := b.dest(sc.a.AllegatoID); righeDest(d) != "1 7120001 dest_radice_uguale" {
		t.Errorf("lo STEP del prodotto: %s", righeDest(d))
	}

	// nessuna identita', il NAS non si tocca
	if dopo := b.fotoNonFlusso(); dopo != prima {
		t.Errorf("il giro ha toccato altro che le destinazioni:\nprima %s\ndopo  %s", prima, dopo)
	}
	if n := uno[int](b, `SELECT count(*) FROM job`); n != lavori {
		t.Errorf("il giro ha messo in coda del lavoro: %d → %d", lavori, n)
	}
	if got := uno[string](b, `SELECT string_agg(documento_id::text || ':' || stato_nas::text, ' ' ORDER BY documento_id) FROM documento`); got != nas {
		t.Errorf("i documenti sul NAS sono cambiati:\n%s\n%s", nas, got)
	}

	// il gate: l'autorita', non la guida; i file senza destinazione non lo fermano fino a F11
	if s := b.strutturali(); s != (db.GateStrutturaleRow{}) {
		t.Errorf("la guida non conta: %+v", s)
	}
	g := b.passa("con i PDF e il file tecnico senza destinazione")
	if !strings.Contains(avvisiDi(g), "7120010 ha 1 figlio che resta guida") {
		t.Errorf("l'avviso dell'assieme con il figlio in guida: %s", avvisiDi(g))
	}
	b.congelaOk("con i PDF e il file tecnico senza destinazione")
}
