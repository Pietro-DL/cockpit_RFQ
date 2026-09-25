//go:build integrazione

// L4 — B8.7b, la preparazione del Fascicolo contro PostgreSQL vero: i codici della richiesta diventano
// prodotti finiti (una volta sola, solo quelli confermati, mai con la BOM congelata); i file utili scendono
// da soli e quelli fermi si rimettono in moto (senza insistere su quelli che hanno fallito); il lavoro in
// corso si conta per job; il piano letto dal database.

package fascicolo_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

var analizzatoreProva = coda.Analizzatore{Versione: 3, Parametri: map[string]any{"termini_cartiglio": []any{"scala"}}}

// messaggioOutlook e' una mail della RFQ del banco, presente nella casella commerciale: e' quello che serve
// perche' un suo allegato si possa scaricare (CopiaPerDownload).
func (b *banco) messaggioOutlook() uuid.UUID {
	b.t.Helper()
	b.n++
	conv := uno[uuid.UUID](b, `INSERT INTO conversazione (canale, chiave_esterna, primo_messaggio_il) VALUES ('outlook', $1, now()) RETURNING conversazione_id`,
		fmt.Sprintf("C-B87B-%d", b.n))
	msg := uno[uuid.UUID](b, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, thread_id)
		VALUES ('outlook', $1, $2, 'entrata', now(), $3) RETURNING messaggio_id`, fmt.Sprintf("<b87b-%d@prova>", b.n), conv, b.thread)
	cas := uno[uuid.UUID](b, `INSERT INTO casella (indirizzo, nome, condivisa) VALUES ('commerciale@azienda.example', 'Commerciale', true)
		ON CONFLICT (canale, indirizzo) DO UPDATE SET nome = EXCLUDED.nome RETURNING casella_id`)
	b.esegui(`INSERT INTO messaggio_casella (messaggio_id, casella_id, entry_id, ricevuto_il) VALUES ($1, $2, $3, now())`, msg, cas, fmt.Sprintf("ENTRY-%d", b.n))
	return msg
}

// allegatoGrezzo e' un allegato non ancora sceso, con la proposta a ingest (pre_spunta come la calcola il
// nome del file).
func (b *banco) allegatoGrezzo(msg uuid.UUID, indice int, nome string, bytes int64, preSpunta bool) uuid.UUID {
	b.t.Helper()
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(nome), "."))
	id := uno[uuid.UUID](b, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, ricevuto_il)
		VALUES ($1, $2, $3, $4, 'file', 'outlook', $5, now()) RETURNING allegato_id`, msg, indice, nome, ext, bytes)
	b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte, dettagli)
		VALUES ($1, $2, 'altro', 20, 'estensione', jsonb_build_object('pre_spunta', $3::boolean))`, id, b.thread, preSpunta)
	return id
}

// allegatoInStaging e' un allegato sceso e fermo: il file c'e' davvero, con il contenuto dato.
func (b *banco) allegatoInStaging(msg uuid.UUID, indice int, nome, contenuto string) (uuid.UUID, string) {
	b.t.Helper()
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(nome), "."))
	sha := fmt.Sprintf("%064x", 1000+indice+b.n*100)
	p := filepath.Join(b.t.TempDir(), sha+"."+ext)
	if err := os.WriteFile(p, []byte(contenuto), 0o644); err != nil {
		b.t.Fatal(err)
	}
	id := uno[uuid.UUID](b, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, stato, ricevuto_il)
		VALUES ($1, $2, $3, $4, 'file', 'outlook', $5, $6, $7, 'in_staging', now()) RETURNING allegato_id`, msg, indice, nome, ext, len(contenuto), sha, p)
	b.esegui(`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, confidenza, fonte) VALUES ($1, $2, 'da_determinare', 40, 'estensione')`, id, b.thread)
	return id, sha
}

func (b *banco) identificativo(codice, origine string, confermato bool) {
	b.t.Helper()
	var chi any
	if confermato {
		chi = b.utente
	}
	b.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confidenza, confermato_da) VALUES ($1, $2, $3, 80, $4)`,
		b.thread, codice, origine, chi)
}

func (b *banco) assicura() fascicolo.EsitoProdotti {
	b.t.Helper()
	var es fascicolo.EsitoProdotti
	ok(b.t, b.tx(func(q *db.Queries) (err error) {
		es, err = fascicolo.AssicuraProdottiDellaRichiesta(b.ctx, q, b.thread)
		return err
	}))
	return es
}

func (b *banco) prepara(max int, rileggi fascicolo.RiletturaFatti) fascicolo.Preparazione {
	b.t.Helper()
	var p fascicolo.Preparazione
	ok(b.t, b.tx(func(q *db.Queries) (err error) {
		p, err = fascicolo.PreparaFile(b.ctx, q, b.thread, analizzatoreProva, 64<<20, max, rileggi)
		return err
	}))
	return p
}

// I codici della richiesta confermati da una persona diventano componenti radice di tipo finito, una volta
// sola; un componente che c'e' gia' non si tocca; un codice non confermato, o che non puo' essere un codice,
// resta fuori. Una proposta STEP aperta con quel codice ritrova il componente.
func TestICodiciDellaRichiestaDiventanoProdottiUnaVoltaSola(t *testing.T) {
	b := nuovoBanco(t)
	b.identificativo("52922757", "proposta_famiglia", true)
	b.identificativo("52922758", "manuale", true)
	b.identificativo("52922759", "proposta_generico", false)
	b.identificativo(strings.Repeat("7", 45), "manuale", true)
	b.identificativo("52922760", "proposta_generico", true)
	assieme := b.componente("52922760", db.TipoComponenteSottoassieme)
	step := b.allegatoStep("assieme.stp", strings.Repeat("e", 64))
	b.esegui(`INSERT INTO componente_proposta (thread_id, allegato_id, sha256, chiave, nome_grezzo, codice, origine_codice, tipo_proposto, fonte, confidenza)
		VALUES ($1, $2, $3, '#1', '52922757', '52922757', 'famiglia', 'finito', 'step', 90)`, b.thread, step.AllegatoID, step.Sha256.String)

	es := b.assicura()
	if strings.Join(es.Creati, ",") != "52922757,52922758" {
		t.Fatalf("creati %v, attesi 52922757 e 52922758", es.Creati)
	}
	if len(es.Saltati) != 1 {
		t.Errorf("il codice troppo lungo resta fuori, e lo si dice: %v", es.Saltati)
	}
	for codice, origine := range map[string]string{"52922757": "codice_rilevato", "52922758": "manuale"} {
		tipo := uno[string](b, `SELECT tipo::text || '/' || origine::text || '/' || (confermato_da = $3)::text FROM componente WHERE thread_id = $1 AND codice = $2`, b.thread, codice, b.utente)
		if tipo != "finito/"+origine+"/true" {
			t.Errorf("%s: %s", codice, tipo)
		}
	}
	if n := uno[int](b, `SELECT count(*) FROM componente WHERE thread_id = $1 AND codice = '52922759'`, b.thread); n != 0 {
		t.Error("un codice non confermato non diventa un prodotto")
	}
	if tipo := uno[string](b, `SELECT tipo::text FROM componente WHERE componente_id = $1`, assieme); tipo != "sottoassieme" {
		t.Errorf("il componente che c'era resta com'era: %s", tipo)
	}
	stato := uno[string](b, `SELECT stato::text || '/' || (componente_id IS NOT NULL)::text FROM componente_proposta WHERE thread_id = $1 AND chiave = '#1'`, b.thread)
	if stato != "duplicato/true" {
		t.Errorf("la radice STEP ritrova il prodotto: %s", stato)
	}

	if es := b.assicura(); len(es.Creati) != 0 {
		t.Errorf("la seconda volta non nasce niente: %v", es.Creati)
	}
	if n := uno[int](b, `SELECT count(*) FROM componente WHERE thread_id = $1`, b.thread); n != 3 {
		t.Errorf("componenti: %d, attesi 3", n)
	}
}

// Con la BOM congelata un codice della richiesta resta codice: il prodotto entra aprendo una revisione (D26).
func TestConLaBomCongelataICodiciNonDiventanoProdotti(t *testing.T) {
	b := nuovoBanco(t)
	b.rfqCongelabile()
	if _, err := b.congela("prima baseline"); err != nil {
		t.Fatal(err)
	}
	b.identificativo("52922757", "manuale", true)
	es := b.assicura()
	if es.Bloccata != 1 || len(es.Creati) != 0 {
		t.Fatalf("con la BOM congelata: %+v", es)
	}
	if n := uno[int](b, `SELECT count(*) FROM componente WHERE thread_id = $1 AND codice = '52922757'`, b.thread); n != 0 {
		t.Error("nessun componente con la BOM congelata")
	}
}

// Scendono da soli i file utili del messaggio della RFQ, nel limite degli upload; non le immagini, non quelli
// gia' in coda, non quelli il cui download e' fallito. Rifarlo non accoda di nuovo. Una mail che non e' in
// nessuna casella attiva non si scarica, e lo si dice.
func TestLaPreparazioneScaricaIFileUtiliESoloQuelli(t *testing.T) {
	b := nuovoBanco(t)
	msg := b.messaggioOutlook()
	utile := b.allegatoGrezzo(msg, 1, "RFQ ACME.zip", 4000, true)
	b.allegatoGrezzo(msg, 2, "enorme.zip", 100<<20, true)
	b.allegatoGrezzo(msg, 3, "logo.png", 3000, false)
	coda := b.allegatoGrezzo(msg, 4, "in coda.pdf", 3000, true)
	b.esegui(`UPDATE allegato SET errore = 'in coda' WHERE allegato_id = $1`, coda)
	fallito := b.allegatoGrezzo(msg, 5, "fallito.pdf", 3000, true)
	b.esegui(`UPDATE allegato SET stato = 'errore', errore = 'elemento non trovato' WHERE allegato_id = $1`, fallito)
	senzaCasella := uno[uuid.UUID](b, `INSERT INTO messaggio (canale, chiave_esterna, conversazione_id, direzione, data_evento, thread_id)
		SELECT 'outlook', '<senza-casella@prova>', conversazione_id, 'entrata', now(), thread_id FROM messaggio WHERE messaggio_id = $1 RETURNING messaggio_id`, msg)
	b.allegatoGrezzo(senzaCasella, 1, "orfano.stp", 3000, true)

	p := b.prepara(10, nil)
	if p.Download != 1 || len(p.Saltati) != 1 || !strings.Contains(p.Saltati[0], "orfano.stp") {
		t.Fatalf("preparazione: %+v", p)
	}
	if n := uno[int](b, `SELECT count(*) FROM job WHERE tipo = 'stage_allegato' AND chiave_idempotenza = $1`, "stage:"+utile.String()); n != 1 {
		t.Errorf("il download dello zip: %d job", n)
	}
	if n := uno[int](b, `SELECT count(*) FROM job WHERE tipo = 'stage_allegato'`); n != 1 {
		t.Errorf("solo lo zip scende: %d job di download", n)
	}
	if p := b.prepara(10, nil); p.Download != 0 {
		t.Errorf("rifarlo non accoda di nuovo: %+v", p)
	}
}

// Un archivio sceso e mai estratto si estrae, un file sceso e mai analizzato si analizza, uno i cui fatti ci
// sono gia' li riceve; uno la cui analisi e' fallita non si riaccoda da solo, e un file sparito dalla cache
// non si tocca. Il limite del giro vale.
func TestLaPreparazioneRimetteInMotoIFileFermi(t *testing.T) {
	b := nuovoBanco(t)
	msg := b.messaggioOutlook()
	zip, _ := b.allegatoInStaging(msg, 1, "RFQ.zip", "PK...")
	b.esegui(`UPDATE documento_proposta SET tipo_proposto = 'altro' WHERE allegato_id = $1`, zip)
	pdf, shaPdf := b.allegatoInStaging(msg, 2, "disegno.pdf", "%PDF-1.7 uno")
	gia, shaGia := b.allegatoInStaging(msg, 3, "gia letto.pdf", "%PDF-1.7 due")
	b.esegui(`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 3, $2, '{}')`, shaGia, analizzatoreProva.Hash())
	sparito, _ := b.allegatoInStaging(msg, 4, "sparito.pdf", "%PDF-1.7 tre")
	b.esegui(`UPDATE allegato SET path_staging = path_staging || '.non-c-e' WHERE allegato_id = $1`, sparito)

	var riletti []uuid.UUID
	rileggi := func(ctx context.Context, q *db.Queries, a uuid.UUID) error { riletti = append(riletti, a); return nil }
	p := b.prepara(10, rileggi)
	if p.Estrazioni != 1 || p.Analisi != 1 || p.Riletti != 1 {
		t.Fatalf("preparazione: %+v", p)
	}
	if len(riletti) != 1 || riletti[0] != gia {
		t.Errorf("i fatti gia' calcolati si applicano al file fermo: %v", riletti)
	}
	if n := uno[int](b, `SELECT count(*) FROM job WHERE chiave_idempotenza = $1`, "estrai:"+zip.String()); n != 1 {
		t.Errorf("l'estrazione dello zip: %d", n)
	}
	chiave := coda.ChiaveAnalisi(shaPdf, analizzatoreProva)
	if n := uno[int](b, `SELECT count(*) FROM job WHERE chiave_idempotenza = $1`, chiave); n != 1 {
		t.Errorf("l'analisi del PDF: %d", n)
	}
	_ = pdf

	// rifatta: i lavori sono pendenti, non si accodano di nuovo
	riletti = nil
	if p := b.prepara(10, rileggi); p.Estrazioni+p.Analisi != 0 {
		t.Errorf("con i lavori pendenti non si accoda niente: %+v", p)
	}
	// l'analisi fallisce: non si insiste
	b.esegui(`UPDATE job SET stato = 'fallito', chiuso_il = now() WHERE chiave_idempotenza = $1`, chiave)
	if p := b.prepara(10, rileggi); p.Analisi != 0 {
		t.Errorf("un'analisi fallita non si riaccoda da sola: %+v", p)
	}
	// il limite del giro
	b.esegui(`UPDATE job SET stato = 'annullato', chiuso_il = now() WHERE stato IN ('pronto', 'in_corso')`)
	b.esegui(`DELETE FROM job WHERE stato = 'annullato'`)
	if p := b.prepara(1, rileggi); p.Estrazioni+p.Analisi+p.Riletti != 1 || p.Rimandati == 0 {
		t.Errorf("con il limite di uno: %+v", p)
	}
}

// Il lavoro in corso si conta per job: un'analisi dello stesso contenuto che la RFQ ha due volte e' una sola.
// Finiti i job, la pagina smette di aggiornarsi.
func TestIlLavoroInCorsoSiContaPerJob(t *testing.T) {
	b := nuovoBanco(t)
	msg := b.messaggioOutlook()
	grezzo := b.allegatoGrezzo(msg, 1, "da scaricare.stp", 1000, true)
	zip, _ := b.allegatoInStaging(msg, 2, "RFQ.zip", "PK")
	a1, sha := b.allegatoInStaging(msg, 3, "uno.pdf", "%PDF uno")
	a2, _ := b.allegatoInStaging(msg, 4, "due.pdf", "%PDF uno")
	b.esegui(`UPDATE allegato SET sha256 = $2 WHERE allegato_id = $1`, a2, sha)
	_ = a1
	for tipo, x := range map[string]any{"stage_allegato": map[string]any{"allegato_id": grezzo}, "estrai_archivio": map[string]any{"allegato_id": zip},
		"analizza_allegato": map[string]any{"allegato_id": a1, "sha256": sha}} {
		ok(t, b.tx(func(q *db.Queries) error {
			_, err := coda.Accoda(b.ctx, q, db.TipoJob(tipo), x, "prova:"+tipo, 5)
			return err
		}))
	}
	var l fascicolo.Lavoro
	ok(t, b.tx(func(q *db.Queries) (err error) { l, err = fascicolo.LavoroInCorso(b.ctx, q, b.thread); return err }))
	if l.Download != 1 || l.Estrazioni != 1 || l.Analisi != 1 || !l.InCorso() || len(l.File) != 4 {
		t.Fatalf("lavoro: %+v", l)
	}
	b.esegui(`UPDATE job SET stato = 'fatto', chiuso_il = now()`)
	ok(t, b.tx(func(q *db.Queries) (err error) { l, err = fascicolo.LavoroInCorso(b.ctx, q, b.thread); return err }))
	if l.InCorso() {
		t.Errorf("a job finiti non c'e' lavoro: %+v", l)
	}
}

// Il piano dal database: un disegno del prodotto e' pronto; con la sua analisi in corso aspetta.
func TestIlPianoDalDatabase(t *testing.T) {
	b := nuovoBanco(t)
	prodotto := b.componente("52922757", db.TipoComponenteFinito)
	msg := b.messaggioOutlook()
	pdf, sha := b.allegatoInStaging(msg, 1, "52922757.pdf", "%PDF disegno")
	b.esegui(`UPDATE allegato SET stato = 'analizzato' WHERE allegato_id = $1`, pdf)
	b.esegui(`UPDATE documento_proposta SET tipo_proposto = 'disegno_2d', codice = '52922757', fonte = 'cartiglio', confidenza = 95 WHERE allegato_id = $1`, pdf)

	leggi := func() fascicolo.PianoFascicolo {
		var p fascicolo.PianoFascicolo
		ok(t, b.tx(func(q *db.Queries) (err error) {
			p, err = fascicolo.LeggiPianoFascicolo(b.ctx, q, b.thread)
			return err
		}))
		return p
	}
	p := leggi()
	if len(p.File) != 1 || p.File[0].Stato != fascicolo.VocePronta || p.File[0].Componente == nil || p.File[0].Componente.ComponenteID != prodotto {
		t.Fatalf("piano: %+v", p.File)
	}
	ok(t, b.tx(func(q *db.Queries) error {
		_, err := coda.Accoda(b.ctx, q, db.TipoJobAnalizzaAllegato, map[string]any{"allegato_id": pdf, "sha256": sha}, "analizza:prova", 5)
		return err
	}))
	if p := leggi(); p.File[0].Stato != fascicolo.VoceAttesa {
		t.Errorf("con l'analisi in corso il file aspetta: %s", p.File[0].Stato)
	}
}
