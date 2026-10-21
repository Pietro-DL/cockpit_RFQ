//go:build integrazione

// L4 — Prova 9 della direttiva 35 per C-6b («Scarto e Altri file»: svincolo, dipendenze, guardie, eventi, stato
// dell'occorrenza, comportamento dopo errori parziali; niente cancellazioni di file storici o documenti confermati),
// con i gesti composti di D-ASS-03 [U] («Scollega e scarta», «Scollega e metti in Altri file»: R2 svincolo persistito,
// poi S2 o S3; stato parziale leggibile; alla R5 svincolo_gia_chiuso) e INC-G10, INC-G11. Solo dati sintetici.

package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/db"
)

// dipendenzaDi: la voce dell'elenco delle dipendenze di S1 con quel codice per il file, o nil.
func (s *scenaC6b) dipendenzaDi(allegato uuid.UUID, codice string) map[string]any {
	s.t.Helper()
	m := s.modello(s.thread, allegato)
	d, _ := m["dettaglio"].(map[string]any)
	l, _ := d["dipendenze"].([]any)
	for _, x := range l {
		v, _ := x.(map[string]any)
		if v["codice"] == codice && v["allegato_id"] == allegato.String() {
			return v
		}
	}
	return nil
}

// svincoliPendenti: gli svincoli delle bozze aperte (lettura di S9) per il file, «componente:AR attiva».
func (s *scenaC6b) svincoliPendenti(allegato uuid.UUID) string {
	s.t.Helper()
	tx, err := s.pool.Begin(s.ctx)
	if err != nil {
		s.t.Fatal(err)
	}
	defer tx.Rollback(s.ctx) //nolint:errcheck
	l, err := distintabozza.LeggiBozzeDellaRfq(s.ctx, db.New(tx), s.thread)
	if err != nil {
		s.t.Fatal(err)
	}
	var out []string
	for _, b := range l.Bozze {
		for _, sv := range b.Svincoli {
			if sv.AllegatoID == allegato {
				stato := "attiva"
				if !sv.AssociazioneAttiva {
					stato = "gia_chiusa"
				}
				out = append(out, s.codiceComp(sv.ComponenteID)+":"+stato)
			}
		}
	}
	return strings.Join(out, " ")
}

func (s *scenaC6b) codiceComp(c uuid.UUID) string {
	return valoreDi(s.t, s.bancoWeb, `SELECT codice FROM componente WHERE componente_id = $1`, c)
}

// Prova 9, RFQ interamente legacy: «Scollega e scarta» di un file con una AR gesto_legacy. D-7 dalla AR chiede la
// risoluzione «scollega» (senza: 422, niente cambia); con «scollega» la AR si chiude dal servizio (scollega_file, con la
// chiave del gesto), il giornale ha lo «sgancia» di origine ponte, l'occorrenza e' scartata, la proposta non tiene il
// componente; il reinvio non scrive di nuovo. Un documento confermato non si scarta (D-1) e resta.
func TestC6bProva9ScollegaEScartaNellaRFQLegacy(t *testing.T) {
	s := nuovaScenaC6b(t)
	if a := s.assegna(s.prop1, &s.c); !strings.Contains(a, "assegnato al componente ACME-C1") {
		t.Fatalf("assegna: %q", a)
	}
	d := s.dipendenzaDi(s.pdf1, "D-7")
	if d == nil || d["esito"] != "risoluzione" || d["risoluzione"] != "scollega" || !strings.Contains(d["testo"].(string), "associazione registrata con ACME-C1") {
		t.Fatalf("D-7 dalla AR: %v", d)
	}
	if s.dipendenzaDi(s.pdf1, "D-7b") != nil {
		t.Error("D-7b con una AR attiva")
	}
	prima := fotoDominio(t, s.bancoWeb)
	if r, _ := s.gesto(s.thread, s.pdf1, "scarta", "doppione", nil); r.stato != http.StatusUnprocessableEntity || r.mappa(t)["codice"] != "RISOLUZIONE_RICHIESTA" {
		t.Fatalf("Scarta senza scollega: %d %s", r.stato, r.corpo)
	}
	if df := differenze(prima, fotoDominio(t, s.bancoWeb)); df != "" {
		t.Errorf("il rifiuto ha scritto in %s", df)
	}
	r, c := s.gesto(s.thread, s.pdf1, "scarta", "doppione", map[string]any{"scollega": true})
	if r.stato != http.StatusOK {
		t.Fatalf("Scollega e scarta: %d %s", r.stato, r.corpo)
	}
	if got := s.ar(s.pdf1); got != "ACME-C1:gesto_legacy:chiusa:scollega_file" {
		t.Errorf("AR dopo Scollega e scarta: %q", got)
	}
	if got := valoreDi(t, s.bancoWeb, `SELECT chiusa_chiave::text FROM file_rfq_associazione WHERE allegato_id = $1`, s.pdf1); got != c["chiave_idempotenza"] {
		t.Errorf("la chiusura porta la chiave del gesto: %s / %v", got, c["chiave_idempotenza"])
	}
	if got := s.eventi(s.pdf1); got != "assegna:legacy sgancia:ponte scarta:ponte" {
		t.Errorf("giornale: %q", got)
	}
	if got := s.statoDB(s.thread, s.pdf1); got != "scartato" {
		t.Errorf("occorrenza: %q", got)
	}
	if got := valoreDi(t, s.bancoWeb, `SELECT stato::text || ':' || coalesce(componente_id::text, '-') FROM documento_proposta WHERE allegato_id = $1`, s.pdf1); got != "scartata:-" {
		t.Errorf("proposta: %q", got)
	}
	dopo := fotoDominio(t, s.bancoWeb)
	if r2 := s.chiama(s.ad, http.MethodPost, rottaFile(s.thread, s.pdf1, "scarta"), c, nil); r2.stato != http.StatusOK {
		t.Fatalf("reinvio: %d %s", r2.stato, r2.corpo)
	}
	if df := differenze(dopo, fotoDominio(t, s.bancoWeb)); df != "" {
		t.Errorf("il reinvio ha scritto in %s", df)
	}

	// un documento confermato (D-1) non si scarta, anche con «scollega»: il documento e la sua riga restano
	if e, testo := s.op.gestoPonte(t, s.base()+"/file/"+s.prop2.String()+"/generale", map[string][]string{"tipo": {"altro"}}, s.thread); e != "ok" {
		t.Fatalf("Documentazione generale: %s %q", e, testo)
	}
	doc := valoreDi(t, s.bancoWeb, `SELECT count(*)::text FROM documento`)
	prima = fotoDominio(t, s.bancoWeb)
	if r, _ := s.gesto(s.thread, s.pdf2, "scarta", "doppione", map[string]any{"scollega": true}); r.stato != http.StatusUnprocessableEntity || r.mappa(t)["codice"] != "DIPENDENZE_ATTIVE" {
		t.Errorf("Scarta di un documento confermato: %d %s", r.stato, r.corpo)
	}
	if df := differenze(prima, fotoDominio(t, s.bancoWeb)); df != "" || valoreDi(t, s.bancoWeb, `SELECT count(*)::text FROM documento`) != doc {
		t.Errorf("lo scarto rifiutato ha toccato %s", df)
	}
}

// Prova 9, RFQ formato 3, con D-ASS-03 [U]: la AR (nata quando la RFQ era legacy) non si chiude da S3 senza lo svincolo
// salvato nella bozza (D-7 blocca, niente cambia). Lo svincolo salvato (qui da «Togli» della pagina legacy, che diventa
// R2): stato parziale leggibile (bozza con lo svincolo, AR ancora attiva, D-6 «svincolo» e D-7 chiedono «scollega»). Un
// errore a meta' (l'anteprima delle dipendenze superata) non applica niente. Poi «Scollega e metti in Altri file»: la AR
// si chiude, il file va in Altri file, la bozza non cambia e la lettura di S9 dice lo svincolo «gia' chiuso» (alla R5:
// svincolo_gia_chiuso). INC-G11: un'occorrenza non attiva con una AR attiva la ferma il database al COMMIT.
func TestC6bProva9GestoCompostoNellaRFQFormato3(t *testing.T) {
	s := nuovaScenaC6b(t)
	s.assegna(s.prop2, &s.c) // AR legacy
	s.creaBozza(distintabozza.PartenzaRegistrato)
	if d := s.dipendenzaDi(s.pdf2, "D-7"); d == nil || d["esito"] != "blocca" || !strings.Contains(d["testo"].(string), "prima si salva lo svincolo") {
		t.Fatalf("D-7 nel formato 3 senza svincolo: %v", d)
	}
	prima := fotoDominio(t, s.bancoWeb)
	if r, _ := s.gesto(s.thread, s.pdf2, "altri-file", "", map[string]any{"scollega": true}); r.stato != http.StatusUnprocessableEntity || r.mappa(t)["codice"] != "DIPENDENZE_ATTIVE" {
		t.Fatalf("Altri file senza svincolo nella bozza: %d %s", r.stato, r.corpo)
	}
	if df := differenze(prima, fotoDominio(t, s.bancoWeb)); df != "" {
		t.Errorf("il rifiuto ha scritto in %s", df)
	}
	vecchio := s.corpo(s.thread, s.pdf2, "altri-file", "", map[string]any{"scollega": true})

	// R2: lo svincolo nella bozza (primo effetto del gesto composto)
	if a := s.assegna(s.prop2, nil); !strings.Contains(a, "preparato nella bozza") {
		t.Fatalf("svincolo nella bozza: %q", a)
	}
	_, v := s.snapshot()
	if got := s.svincoliPendenti(s.pdf2); got != "ACME-C1:attiva" {
		t.Errorf("stato parziale (S9): %q", got)
	}
	if got := s.ar(s.pdf2); got != "ACME-C1:gesto_legacy:attiva:-" {
		t.Errorf("stato parziale (AR): %q", got)
	}
	if d := s.dipendenzaDi(s.pdf2, "D-6"); d == nil || d["risoluzione"] != "scollega" {
		t.Errorf("D-6 dello svincolo: %v", d)
	}
	if d := s.dipendenzaDi(s.pdf2, "D-7"); d == nil || d["esito"] != "risoluzione" {
		t.Errorf("D-7 con lo svincolo salvato: %v", d)
	}
	// l'errore a meta': il corpo composto prima dello svincolo (impronta delle dipendenze superata)
	prima = fotoDominio(t, s.bancoWeb)
	if r := s.chiama(s.ad, http.MethodPost, rottaFile(s.thread, s.pdf2, "altri-file"), vecchio, nil); r.stato != http.StatusConflict {
		t.Fatalf("anteprima superata: %d %s", r.stato, r.corpo)
	}
	if df := differenze(prima, fotoDominio(t, s.bancoWeb)); df != "" {
		t.Errorf("l'errore a meta' ha scritto in %s", df)
	}
	// il secondo effetto: S3 con «scollega»
	if r, _ := s.gesto(s.thread, s.pdf2, "altri-file", "", map[string]any{"scollega": true}); r.stato != http.StatusOK {
		t.Fatalf("Scollega e metti in Altri file: %d %s", r.stato, r.corpo)
	}
	if got := s.ar(s.pdf2); got != "ACME-C1:gesto_legacy:chiusa:scollega_file" {
		t.Errorf("AR dopo il gesto composto: %q", got)
	}
	if got := s.statoDB(s.thread, s.pdf2); got != "altri_file" {
		t.Errorf("occorrenza: %q", got)
	}
	if got := s.svincoliPendenti(s.pdf2); got != "ACME-C1:gia_chiusa" {
		t.Errorf("S9 dopo il gesto composto (svincolo_gia_chiuso alla R5): %q", got)
	}
	if _, v2 := s.snapshot(); v2 != v {
		t.Errorf("il gesto composto ha cambiato la bozza: v%d → v%d", v, v2)
	}

	// INC-G11: il database non lascia un'occorrenza non attiva con una AR attiva (difesa al COMMIT, oltre a D-7). Un file
	// con la riga dell'occorrenza (Altri file e Riattiva) e una AR attiva: portarlo a scartato a mano si ferma
	pdf4, _ := s.file(s.thread, "ACME-D4.pdf", byteCasuali("x.pdf", 605, 1_500), db.OrigineAllegatoOutlook)
	if r, _ := s.gesto(s.thread, pdf4, "altri-file", "", nil); r.stato != http.StatusOK {
		t.Fatalf("Altri file: %d %s", r.stato, r.corpo)
	}
	if r, _ := s.gesto(s.thread, pdf4, "riattiva", "", nil); r.stato != http.StatusOK {
		t.Fatalf("Riattiva: %d %s", r.stato, r.corpo)
	}
	ar := uuidSQL(t, s.bancoWeb, `SELECT associazione_id FROM file_rfq_associazione WHERE allegato_id = $1`, s.pdf2)
	tx, err := s.pool.Begin(s.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(s.ctx) //nolint:errcheck
	// una AR attiva per pdf4, scritta a mano con i trigger delle AR spenti in questa transazione (la riga copiata da quella
	// chiusa di pdf2, gesto_legacy, che nella RFQ formato 3 il servizio non scriverebbe)
	for _, q := range []string{`ALTER TABLE file_rfq_associazione DISABLE TRIGGER USER`,
		`INSERT INTO file_rfq_associazione (thread_id, allegato_id, componente_id, origine, chiave_idempotenza, creata_da)
			SELECT thread_id, '` + pdf4.String() + `', componente_id, origine, gen_random_uuid(), creata_da FROM file_rfq_associazione WHERE associazione_id = '` + ar.String() + `'`,
		`SET CONSTRAINTS ALL IMMEDIATE`, `ALTER TABLE file_rfq_associazione ENABLE TRIGGER USER`, `SET CONSTRAINTS ALL DEFERRED`} {
		if _, err := tx.Exec(s.ctx, q); err != nil {
			t.Fatalf("%v: %s", err, q)
		}
	}
	if _, err := tx.Exec(s.ctx, `UPDATE file_rfq SET stato = 'scartato', versione = versione + 1 WHERE thread_id = $1 AND allegato_id = $2`, s.thread, pdf4); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := tx.Exec(s.ctx, `SET CONSTRAINTS trg_file_rfq_senza_ar IMMEDIATE`); err == nil || !strings.Contains(err.Error(), "associazione registrata ancora attiva") {
		t.Errorf("INC-G11: un'occorrenza scartata con una AR attiva: %v", err)
	}
}
