//go:build integrazione

package filerfq

// L4 — V1, corsia file: la concorrenza dei file (PM23 §10 riga 9, CO-01…CO-17, CO-07b, CO-08b, CO-14b; A2 §8.3; EM
// §10.4-§10.7) e M-33 per le strade del Go che scrivono proposte (DECA P-5 (persistenza)). CO-12 e CO-13 sono le L1
// statiche (statiche_test.go); CO-04 sta con le rotte (transport/web); i risultati del worker di CO-01, CO-02 e CO-03
// stanno con il worker (transport/workerapi).

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/documenti"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/storage/staging"
)

// lavoro: un job pendente di analisi con l'occorrenza (e il contenuto) nel payload.
func (b *bancoFile) lavoro(allegato uuid.UUID, sha string) int64 {
	b.t.Helper()
	return valore[int64](b, `INSERT INTO job (tipo, worker_tipo, payload, stato) VALUES ('analizza_allegato', 'analisi',
		jsonb_build_object('allegato_id', $1::text, 'sha256', $2::text), 'pronto') RETURNING job_id`, allegato.String(), sha)
}

func (b *bancoFile) lavoroFatto(job int64) {
	b.t.Helper()
	b.esegui(`UPDATE job SET stato = 'fatto', chiuso_il = now() WHERE job_id = $1`, job)
}

// CO-01: un'analisi in coda con A nel payload, poi lo scarto di A: la decisione e' registrata, la rimozione aspetta
// (in_attesa / lavoro_in_corso), il job non si annulla (il worker riceve ancora i byte); finito il lavoro, la rimozione
// si completa. (I fatti che arrivano a B e non ad A: la prova del worker.)
func TestCO01AnalisiInCodaConLOccorrenzaNelPayload(t *testing.T) {
	b := nuovoBancoFile(t)
	a, sha := b.pdf("ACME-C01.pdf")
	p := b.pathStaging(a)
	job := b.lavoro(a, sha)
	e := b.deveRiuscire(b.scarta(b.thread, a))
	if got := b.stato(b.thread, a); got != StatoScartato {
		t.Errorf("la decisione non e' registrata: %q", got)
	}
	if got := b.rimozione(b.thread, a); got != "in_attesa/lavoro_in_corso" || !esiste(p) {
		t.Fatalf("rimozione %s, file %v", got, esiste(p))
	}
	if !strings.Contains(e.Testo, "analisi in corso") {
		t.Errorf("testo %q", e.Testo)
	}
	if got := valore[string](b, `SELECT stato::text FROM job WHERE job_id = $1`, job); got != "pronto" {
		t.Errorf("il job di analisi e' stato toccato: %s", got)
	}
	b.lavoroFatto(job)
	b.avanti(2 * time.Minute)
	if _, err := (&Custode{S: b.s}).Giro(b.ctx); err != nil {
		t.Fatal(err)
	}
	if got := b.rimozione(b.thread, a); got != "eseguita/rimosso" || esiste(p) {
		t.Errorf("dopo il lavoro: %s, file %v", got, esiste(p))
	}
}

// CO-02: un download pendente di A, poi lo scarto: il job e' annullato, nessun path_staging. (Il risultato tardivo che
// riceve 409 senza promozione: la prova del worker.)
func TestCO02DownloadPendenteAnnullato(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.file(b.mail(b.thread), "ACME-C02.pdf", nil, opzFile{senzaByte: true})
	job := valore[int64](b, `INSERT INTO job (tipo, worker_tipo, payload, chiave_idempotenza, stato) VALUES ('stage_allegato', 'outlook',
		jsonb_build_object('allegato_id', $1::text), 'stage:' || $1::text, 'pronto') RETURNING job_id`, a.String())
	b.deveRiuscire(b.scarta(b.thread, a))
	if got := valore[string](b, `SELECT stato::text FROM job WHERE job_id = $1`, job); got != "annullato" {
		t.Errorf("il download pendente: %s", got)
	}
	if b.pathStaging(a) != "" {
		t.Error("path_staging su un file scartato mai scaricato")
	}
	if got := b.rimozione(b.thread, a); got != "eseguita/gia_assente" {
		t.Errorf("rimozione %s", got)
	}
}

// CO-05: preparazione, ripresa della copia sul NAS, analisi mancanti: niente download, estrazione o analisi per le
// occorrenze non attive.
func TestCO05NienteLavoriPerLeOccorrenzeNonAttive(t *testing.T) {
	b := nuovoBancoFile(t)
	th := uuid.NullUUID{UUID: b.thread, Valid: true}
	g, _ := b.file(b.mail(b.thread), "ACME-C05.pdf", nil, opzFile{senzaByte: true})
	b.esegui(`UPDATE documento_proposta SET dettagli = '{"pre_spunta": true}' WHERE allegato_id = $1`, g)
	daPreparare := func() bool {
		righe, err := b.q.ListAllegatiDaPreparare(b.ctx, db.ListAllegatiDaPreparareParams{ThreadID: th, MaxBytes: 1 << 30})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range righe {
			if r.AllegatoID == g {
				return true
			}
		}
		return false
	}
	if !daPreparare() {
		t.Fatal("prima dello scarto il file non e' da preparare: la prova non guarda dove deve")
	}
	b.deveRiuscire(b.scarta(b.thread, g))
	if daPreparare() {
		t.Error("un file scartato resta da scaricare")
	}
	f, _ := b.pdf("ACME-C05b.pdf")
	b.esegui(`UPDATE allegato SET stato = 'in_staging' WHERE allegato_id = $1`, f)
	fermo := func() bool {
		righe, err := b.q.ListAllegatiFermiInStaging(b.ctx, th)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range righe {
			if r.AllegatoID == f {
				return true
			}
		}
		return false
	}
	if !fermo() {
		t.Fatal("prima del gesto il file non e' fermo nello staging")
	}
	b.deveRiuscire(b.altriFile(b.thread, f))
	if fermo() {
		t.Error("un file in Altri file resta da analizzare")
	}
	a, err := b.q.GetAllegato(b.ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	jobPrima := valore[int](b, `SELECT count(*) FROM job`)
	originale := errors.New("contenuto mancante")
	if err := documenti.RiprendiContenuto(b.ctx, b.q, nil, a, originale); err == nil || !errors.Is(err, originale) || !strings.Contains(err.Error(), "Altri file") {
		t.Errorf("la ripresa della copia per un file in Altri file: %v", err)
	}
	if n := valore[int](b, `SELECT count(*) FROM job`); n != jobPrima {
		t.Errorf("la ripresa ha accodato %d lavori per un file non attivo", n-jobPrima)
	}
}

// CO-06: il riuso del gemello (un'altra transazione) contro il tempo 2 di A, con l'aggancio fra il controllo e la
// cancellazione: il riuso aspetta BloccaContenuto e, dopo la cancellazione, accoda il download invece di indicare un
// file che non c'e'.
func TestCO06RiusoDelGemelloControIlTempo2(t *testing.T) {
	b := nuovoBancoFile(t)
	b.s.Agganci = &Agganci{SaltaTentativi: true}
	c := contenuto("ACME-C06.pdf", 606, 3_000)
	a, sha := b.file(b.mail(b.thread), "ACME-C06.pdf", c, opzFile{})
	b.deveRiuscire(b.scarta(b.thread, a))
	rfqB := b.rfq("RFQ-B")
	msgB := b.mail(rfqB)
	casella := valore[uuid.UUID](b, `INSERT INTO casella (indirizzo, nome, condivisa) VALUES ('commerciale@azienda.example', 'Commerciale', true)
		RETURNING casella_id`)
	// B: lo stesso contenuto, non ancora sceso (lo sha lo conosce dall'ingest)
	bb := valore[uuid.UUID](b, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, stato, ricevuto_il)
		VALUES ($1, 1, 'ACME-C06.pdf', 'pdf', 'file', 'outlook', $2, $3, 'grezzo', now()) RETURNING allegato_id`, msgB, len(c), sha)
	type esitoStage struct {
		e   coda.EsitoStage
		err error
	}
	fatto := make(chan esitoStage, 1)
	b.s.Agganci = &Agganci{SaltaTentativi: true, PrimaDiCancellare: func(int64) {
		go func() {
			var r esitoStage
			r.err = b.s.inTx(b.ctx, func(q *db.Queries) error {
				al, err := q.GetAllegato(b.ctx, bb)
				if err != nil {
					return err
				}
				m, err := q.GetMessaggio(b.ctx, msgB)
				if err != nil {
					return err
				}
				r.e, _, err = coda.AccodaStage(b.ctx, q, staging.FileStaging{}, al, m, coda.Copia{CasellaID: casella, EntryID: "E-C06"}, 2)
				return err
			})
			fatto <- r
		}()
		b.attendiLucchetto()
	}}
	if ep, err := b.s.EseguiRimozione(b.ctx, b.rimozioneID(b.thread, a), false); err != nil || ep.Stato != "eseguita" {
		t.Fatalf("tempo 2: %+v %v", ep, err)
	}
	r := <-fatto
	if r.err != nil || r.e != coda.StageAccodato {
		t.Fatalf("il riuso del gemello dopo la cancellazione: %v %v (atteso il download accodato)", r.e, r.err)
	}
	if p := b.pathStaging(bb); p != "" {
		t.Errorf("B indica un file che non c'e': %q", p)
	}
}

// CO-07: la regola dei 10 minuti: un file appena scritto non si cancella (in_attesa / file_giovane); il caricamento
// dello stesso contenuto in un'altra RFQ lo adotta e il suo percorso resta valido; il ritentativo chiude condiviso.
func TestCO07FileGiovaneECaricamentoDelloStessoContenuto(t *testing.T) {
	b := nuovoBancoFile(t)
	b.s.EtaMinima = 0 // la regola di produzione: 10 minuti
	c := contenuto("ACME-C07.pdf", 707, 3_000)
	a, sha := b.file(b.mail(b.thread), "ACME-C07.pdf", c, opzFile{})
	b.deveRiuscire(b.scarta(b.thread, a))
	if got := b.rimozione(b.thread, a); got != "in_attesa/file_giovane" {
		t.Fatalf("rimozione %s", got)
	}
	es, err := b.s.RegistraFile(b.ctx, b.rfq("RFQ-B"), b.u, true, b.arrivato(c, "ACME-C07.pdf"))
	if err != nil || es.Esito != "caricato" {
		t.Fatalf("caricamento: %+v %v", es, err)
	}
	pb := b.pathStaging(*es.AllegatoID)
	b.avanti(time.Hour)
	if _, err := b.s.EseguiRimozione(b.ctx, b.rimozioneID(b.thread, a), false); err != nil {
		t.Fatal(err)
	}
	if got := b.rimozione(b.thread, a); got != "non_necessaria_condiviso/condiviso" {
		t.Errorf("il ritentativo: %s", got)
	}
	if !staging.ContenutoGiaPresente(pb, sha) {
		t.Error("il percorso del caricamento non porta al contenuto")
	}
}

// CO-07b: l'adozione di un contenuto gia' presente con il tempo 2 in mezzo: il caricamento tiene il parziale fino al
// controllo sotto il lucchetto e lo promuove se il contenuto e' sparito; la risposta «caricato» corrisponde a un file
// presente, e il parziale non resta.
func TestCO07bAdozioneConIlTempo2InMezzo(t *testing.T) {
	b := nuovoBancoFile(t)
	b.s.Agganci = &Agganci{SaltaTentativi: true}
	c := contenuto("ACME-C07b.pdf", 708, 3_000)
	a, sha := b.file(b.mail(b.thread), "ACME-C07b.pdf", c, opzFile{})
	b.deveRiuscire(b.scarta(b.thread, a))
	f := b.arrivato(c, "ACME-C07b.pdf")
	rfqB := b.rfq("RFQ-B")
	type esitoCar struct {
		es  EsitoFile
		err error
	}
	fatto := make(chan esitoCar, 1)
	b.s.Agganci = &Agganci{SaltaTentativi: true, PrimaDiCancellare: func(int64) {
		go func() {
			es, err := b.s.RegistraFile(b.ctx, rfqB, b.u, false, f)
			fatto <- esitoCar{es, err}
		}()
		b.attendiLucchetto()
		if !esiste(f.Parte) {
			b.t.Error("il caricamento ha tolto il parziale prima del controllo sotto il lucchetto")
		}
	}}
	if _, err := b.s.EseguiRimozione(b.ctx, b.rimozioneID(b.thread, a), false); err != nil {
		t.Fatal(err)
	}
	r := <-fatto
	if r.err != nil || r.es.Esito != "caricato" {
		t.Fatalf("caricamento: %+v %v", r.es, r.err)
	}
	if p := b.pathStaging(*r.es.AllegatoID); !staging.ContenutoGiaPresente(p, sha) {
		t.Errorf("«caricato» senza il file: %q", p)
	}
	if esiste(f.Parte) {
		t.Error("il parziale e' rimasto")
	}
}

// CO-08: il reinvio con la stessa chiave e lo stesso corpo → riproposto, nessun secondo evento ne' seconda rimozione;
// con un corpo diverso → 409 CHIAVE_RIUSATA; l'esito dell'intento e' registrato con il COMMIT (R6, «registrato», e'
// la rotta della corsia persistenza).
func TestCO08ReinvioDelGesto(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-C08.pdf")
	corpo := b.corpo(GestoScarta, b.thread, a, "non serve")
	e1 := b.deveRiuscire(b.s.Scarta(b.ctx, b.thread, a, b.u.UtenteID, corpo))
	conta := func() string {
		return valore[string](b, `SELECT (SELECT count(*) FROM file_rfq_evento) || '/' || (SELECT count(*) FROM file_rfq_rimozione)
			|| '/' || (SELECT count(*) FROM esito_intento)`)
	}
	prima := conta()
	e2 := b.deveRiuscire(b.s.Scarta(b.ctx, b.thread, a, b.u.UtenteID, corpo))
	if !e2.Riproposto || e2.Versione != e1.Versione || e2.ChiaveIdempotenza != e1.ChiaveIdempotenza {
		t.Errorf("reinvio: %+v", e2)
	}
	if dopo := conta(); dopo != prima {
		t.Errorf("il reinvio ha scritto: %s → %s", prima, dopo)
	}
	diverso := corpo
	m := "un altro motivo"
	diverso.Motivo = &m
	_, err := b.s.Scarta(b.ctx, b.thread, a, b.u.UtenteID, diverso)
	rifiutoAtteso(t, err, http.StatusConflict, "CHIAVE_RIUSATA")
	if n := valore[int](b, `SELECT count(*) FROM esito_intento WHERE chiave = $1 AND gesto = 'scarta_file' AND thread_id = $2`,
		*corpo.ChiaveIdempotenza, b.thread); n != 1 {
		t.Errorf("esito_intento registrato: %d righe", n)
	}
}

// CO-08b: una chiave riusata per lo scarto di un altro file con lo stesso corpo → 409 CHIAVE_RIUSATA e il file B non e'
// scartato; la stessa chiave in un'altra RFQ → 422 FUORI_RFQ.
func TestCO08bChiaveRiusataPerUnAltroFile(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-C08A.pdf")
	bb, _ := b.pdf("ACME-C08B.pdf")
	corpoA := b.corpo(GestoScarta, b.thread, a, "stesso motivo")
	b.deveRiuscire(b.s.Scarta(b.ctx, b.thread, a, b.u.UtenteID, corpoA))
	corpoB := b.corpo(GestoScarta, b.thread, bb, "stesso motivo")
	corpoB.ChiaveIdempotenza = corpoA.ChiaveIdempotenza
	_, err := b.s.Scarta(b.ctx, b.thread, bb, b.u.UtenteID, corpoB)
	rifiutoAtteso(t, err, http.StatusConflict, "CHIAVE_RIUSATA")
	if got := b.stato(b.thread, bb); got != "" {
		t.Errorf("il file B risulta %q", got)
	}
	rfqB := b.rfq("RFQ-B")
	cc, _ := b.file(b.mail(rfqB), "ACME-C08C.pdf", contenuto("ACME-C08C.pdf", 809, 2_000), opzFile{})
	corpoC := b.corpo(GestoScarta, rfqB, cc, "stesso motivo")
	corpoC.ChiaveIdempotenza = corpoA.ChiaveIdempotenza
	_, err = b.s.Scarta(b.ctx, rfqB, cc, b.u.UtenteID, corpoC)
	rifiutoAtteso(t, err, http.StatusUnprocessableEntity, "FUORI_RFQ")
}

// CO-09: due «Altri file» concorrenti sulla stessa occorrenza: uno 200, l'altro 409 COPIA_IN_CORSO; una sola copia viva.
func TestCO09DueAltriFileConcorrenti(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-C09.pdf")
	var err2 error
	secondo := false
	b.s.Agganci = &Agganci{DopoLaCopia: func(uuid.UUID) {
		if secondo {
			return
		}
		secondo = true
		_, err2 = b.altriFile(b.thread, a)
	}}
	b.deveRiuscire(b.altriFile(b.thread, a))
	rf := rifiutoAtteso(t, err2, http.StatusConflict, "COPIA_IN_CORSO")
	if rf.Extra["motivo"] != "copia_viva" {
		t.Errorf("motivo %v", rf.Extra["motivo"])
	}
	if got := b.copie(b.thread, a); !uguali(got, "copiata/-") {
		t.Errorf("conservazione %v", got)
	}
}

// CO-11: il custode della cache durante la copia di conservazione: il pin «conservazione in corso» tiene il file (il
// file non ha proposte aperte: e' il solo pin). In piu' (TR-09): con l'aggancio fra il calcolo dei pin e la
// cancellazione e un caricamento dello stesso contenuto in mezzo, la cache ricalcola sotto BloccaContenuto e non
// cancella.
func TestCO11CacheDuranteLaCopiaEConUnCaricamentoInMezzo(t *testing.T) {
	b := nuovoBancoFile(t)
	a, sha := b.file(b.mail(b.thread), "ACME-C11.pdf", contenuto("ACME-C11.pdf", 1111, 3_000), opzFile{tipo: "-"})
	p := b.pathStaging(a)
	var giro staging.EsitoCache
	var motivo string
	b.s.Agganci = &Agganci{DopoLaCopia: func(uuid.UUID) {
		if righe, err := b.q.ListContenutiPinnati(b.ctx, []string{sha}); err == nil && len(righe) == 1 {
			motivo = righe[0].Motivo
		}
		giro, _ = b.cache(b.s, 40).Giro(b.ctx)
	}}
	b.deveRiuscire(b.altriFile(b.thread, a))
	if motivo != "conservazione in corso" || giro.Rimossi != 0 || !esiste(p) {
		t.Errorf("durante la copia: pin %q, rimossi %d, file %v", motivo, giro.Rimossi, esiste(p))
	}

	// TR-09: un contenuto fermo da 40 giorni (il file e la riga), la cache con l'orologio di adesso; fra la passata che
	// decide e la cancellazione un caricamento lo adotta in un'altra RFQ: sotto BloccaContenuto la cache vede l'uso nuovo
	c := contenuto("ACME-C11b.pdf", 1112, 3_000)
	a2, sha2 := b.file(b.mail(b.thread), "ACME-C11b.pdf", c, opzFile{tipo: "-"})
	p2 := b.pathStaging(a2)
	vecchio := time.Now().Add(-40 * 24 * time.Hour)
	if err := os.Chtimes(p2, vecchio, vecchio); err != nil {
		t.Fatal(err)
	}
	b.esegui(`UPDATE allegato SET ricevuto_il = now() - interval '40 days' WHERE allegato_id = $1`, a2)
	rfqB := b.rfq("RFQ-B")
	cache := &staging.Cache{Pool: b.p, Staging: b.staging, Log: b.s.Log, Retention: 30 * 24 * time.Hour, CopiaNelDeposito: b.s.CopiaNelDeposito()}
	var es EsitoFile
	var errCar error
	chiamato := false
	cache.PrimaDiRimuovere = func(s string) {
		if s == sha2 {
			chiamato = true
			es, errCar = b.s.RegistraFile(b.ctx, rfqB, b.u, true, b.arrivato(c, "ACME-C11b.pdf"))
		}
	}
	g, err := cache.Giro(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !chiamato {
		t.Fatalf("la cache non ha scelto il contenuto fermo: %+v (la prova non prova niente)", g)
	}
	if errCar != nil || es.Esito != "caricato" {
		t.Fatalf("il caricamento in mezzo: %+v %v", es, errCar)
	}
	if !esiste(p2) {
		t.Errorf("la cache ha cancellato un contenuto appena adottato: %+v", g)
	}
	if pb := b.pathStaging(*es.AllegatoID); !staging.ContenutoGiaPresente(pb, sha2) {
		t.Errorf("il percorso del caricamento non porta al contenuto: %q", pb)
	}
}

// CO-14: il messaggio spostato in un'altra RFQ dopo lo scarto: nella RFQ nuova l'occorrenza e' attiva (nessuna riga).
// (I byte si riprendono con «Riscarica»: la prova delle rotte, CO-04.)
func TestCO14MessaggioSpostatoDopoLoScarto(t *testing.T) {
	b := nuovoBancoFile(t)
	a, _ := b.pdf("ACME-C14.pdf")
	b.deveRiuscire(b.scarta(b.thread, a))
	rfqB := b.rfq("RFQ-B")
	b.esegui(`UPDATE messaggio SET thread_id = $2 WHERE messaggio_id = (SELECT messaggio_id FROM allegato WHERE allegato_id = $1)`, a, rfqB)
	f := fileDi(b.modello(rfqB, a), a)
	if f == nil || f.Stato != StatoAttivo || f.Versione != 0 {
		t.Fatalf("nella RFQ nuova: %+v", f)
	}
	if f.Byte.Staging != "assente" {
		t.Errorf("i byte dello scarto nella RFQ vecchia: staging %q (si riprendono con «Riscarica»)", f.Byte.Staging)
	}
	if b.stato(b.thread, a) != StatoScartato {
		t.Error("la storia nella RFQ vecchia e' cambiata")
	}
}

// CO-14b: una rimozione in_attesa (lavoro in corso), poi il messaggio passa a un'altra RFQ: il ritentativo chiude
// non_necessaria_condiviso / rfq_cambiata senza cancellare.
func TestCO14bRimozioneInAttesaEMessaggioSpostato(t *testing.T) {
	b := nuovoBancoFile(t)
	a, sha := b.pdf("ACME-C14b.pdf")
	p := b.pathStaging(a)
	job := b.lavoro(a, sha)
	b.deveRiuscire(b.scarta(b.thread, a))
	if got := b.rimozione(b.thread, a); got != "in_attesa/lavoro_in_corso" {
		t.Fatalf("rimozione %s", got)
	}
	b.esegui(`UPDATE messaggio SET thread_id = $2 WHERE messaggio_id = (SELECT messaggio_id FROM allegato WHERE allegato_id = $1)`, a, b.rfq("RFQ-B"))
	b.lavoroFatto(job)
	if ep, err := b.s.EseguiRimozione(b.ctx, b.rimozioneID(b.thread, a), false); err != nil || ep.Motivo != "rfq_cambiata" {
		t.Fatalf("il ritentativo: %+v %v", ep, err)
	}
	if got := b.rimozione(b.thread, a); got != "non_necessaria_condiviso/rfq_cambiata" || !esiste(p) || b.pathStaging(a) != p {
		t.Errorf("rimozione %s, file %v, path %q", got, esiste(p), b.pathStaging(a))
	}
}

// CO-15: il custode della cache con l'aggancio fra la passata che decide e la cancellazione, e una «Riattiva» in mezzo:
// lo staging non si cancella, la copia si rilascia solo con lo staging verificato; nessun file perso.
func TestCO15CacheConUnaRiattivaInMezzo(t *testing.T) {
	b := nuovoBancoFile(t)
	a, sha := b.pdf("ACME-C15.pdf")
	b.deveRiuscire(b.altriFile(b.thread, a))
	p := b.pathStaging(a)
	cache := b.cache(b.s, 40)
	var e *EsitoGesto
	var errR error
	cache.PrimaDiRimuovere = func(s string) {
		if s == sha {
			e, errR = b.riattiva(b.thread, a)
		}
	}
	g, err := cache.Giro(b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if errR != nil || e == nil {
		t.Fatalf("la «Riattiva» in mezzo: %v", errR)
	}
	if g.Rimossi != 0 || !staging.ContenutoGiaPresente(p, sha) {
		t.Errorf("lo staging e' stato cancellato: rimossi %d", g.Rimossi)
	}
	if e.ByteRipristinati == nil || *e.ByteRipristinati {
		t.Errorf("i byte erano nello staging: %+v", e)
	}
	if got := b.copie(b.thread, a); !uguali(got, "trattenuta/riattivato") {
		t.Errorf("conservazione %v", got)
	}
}

// CO-16: la ripresa con un'altra chiave dopo 15 minuti di battito fermo: la riga vecchia va in rilascio, la fase C del
// tentativo vecchio risponde 409 tentativo_chiuso senza toccare la copia nuova; il custode non chiude una copia con il
// battito vivo anche se iniziata da oltre 15 minuti.
func TestCO16RipresaDopoIlBattitoFermo(t *testing.T) {
	b := nuovoBancoFile(t)
	a, sha := b.pdf("ACME-C16.pdf")
	var e2 *EsitoGesto
	var err2 error
	ripreso := false
	b.s.Agganci = &Agganci{DopoLaCopia: func(uuid.UUID) {
		if ripreso {
			return
		}
		ripreso = true
		b.avanti(16 * time.Minute) // nessun battito da 16 minuti
		e2, err2 = b.altriFile(b.thread, a)
	}}
	_, err1 := b.altriFile(b.thread, a)
	if err2 != nil || e2 == nil || e2.Esito != "in_altri_file" {
		t.Fatalf("la ripresa con un'altra chiave: %+v %v", e2, err2)
	}
	rf := rifiutoAtteso(t, err1, http.StatusConflict, "COPIA_IN_CORSO")
	if rf.Extra["motivo"] != "tentativo_chiuso" {
		t.Errorf("motivo %v", rf.Extra["motivo"])
	}
	if got := b.copie(b.thread, a); !uguali(got, "da_rilasciare/tempo_scaduto", "copiata/-") {
		t.Fatalf("conservazione %v", got)
	}
	if _, err := (&Custode{S: b.s}).Giro(b.ctx); err != nil {
		t.Fatal(err)
	}
	// la copia del tentativo scaduto ha la prova (il suo file l'ha creato il cockpit): con la base B di U2-Q01
	// (provvisoria [T]) resta, trattenuta
	if got := b.copie(b.thread, a); !uguali(got, "trattenuta/tempo_scaduto", "copiata/-") {
		t.Errorf("dopo il custode: %v", got)
	}
	if _, h := b.copiaVerificata(b.thread, a); h != sha {
		t.Error("la copia nuova non e' intatta")
	}

	// il battito vivo, l'inizio vecchio: il custode non chiude
	b.s.Adesso = nil
	a2, _ := b.pdf("ACME-C16b.pdf")
	var chiusa bool
	b.s.Agganci = &Agganci{DopoLaCopia: func(uuid.UUID) {
		// la copia e' iniziata 20 minuti fa (si sposta l'inizio, che il trigger non lascia cambiare), il battito (la
		// colonna ultimo_battito, U2 §3.3 passo 2) e' di adesso
		b.esegui(`ALTER TABLE file_rfq_conservazione DISABLE TRIGGER trg_cons_cambia`)
		b.esegui(`UPDATE file_rfq_conservazione SET iniziata_il = now() - interval '20 minutes', ultimo_battito = now()
			WHERE allegato_id = $1 AND stato = 'in_copia'`, a2)
		b.esegui(`ALTER TABLE file_rfq_conservazione ENABLE TRIGGER trg_cons_cambia`)
		e, err := (&Custode{S: b.s}).Giro(b.ctx)
		chiusa = err != nil || e.TempiScaduti > 0
	}}
	b.deveRiuscire(b.altriFile(b.thread, a2))
	if chiusa {
		t.Error("il custode ha chiuso una copia con il battito vivo")
	}
	if got := b.copie(b.thread, a2); !uguali(got, "copiata/-") {
		t.Errorf("conservazione %v", got)
	}
}

// CO-17: il messaggio di un file in Altri file passa a un'altra RFQ (UPDATE diretto): i byte si trovano per
// allegato_id (S5 per la RFQ nuova, S3 e «Riattiva» nella RFQ nuova), e la copia della RFQ vecchia non si rilascia.
func TestCO17MessaggioSpostatoConIlFileInAltriFile(t *testing.T) {
	b := nuovoBancoFile(t)
	a, sha := b.pdf("ACME-C17.pdf")
	b.deveRiuscire(b.altriFile(b.thread, a))
	if giro, err := b.cache(b.s, 40).Giro(b.ctx); err != nil || giro.Rimossi != 1 {
		t.Fatalf("la cache non ha tolto lo staging: %+v %v", giro, err)
	}
	rfqB := b.rfq("RFQ-B")
	b.esegui(`UPDATE messaggio SET thread_id = $2 WHERE messaggio_id = (SELECT messaggio_id FROM allegato WHERE allegato_id = $1)`, a, rfqB)
	f, _, _, err := b.s.ApriCopia(b.ctx, rfqB, a, false)
	if err != nil || f == nil {
		t.Fatalf("S5 nella RFQ nuova non trova i byte: %v", err)
	}
	h, err := shaDelFile(f.Name())
	f.Close()
	if err != nil || h != sha {
		t.Errorf("i byte trovati: %s %v", h, err)
	}
	b.deveRiuscire(b.altriFile(rfqB, a))
	if got := valore[string](b, `SELECT sorgente_byte FROM file_rfq_conservazione WHERE thread_id = $1 AND allegato_id = $2`, rfqB, a); got != "conservazione" {
		t.Errorf("la copia nella RFQ nuova viene da %q", got)
	}
	b.deveRiuscire(b.riattiva(rfqB, a))
	if p := b.pathStaging(a); !staging.ContenutoGiaPresente(p, sha) {
		t.Errorf("la «Riattiva» nella RFQ nuova non ha rimesso i byte: %q", p)
	}
	if got := b.copie(b.thread, a); !uguali(got, "copiata/-") {
		t.Errorf("la copia della RFQ vecchia: %v", got)
	}
}
