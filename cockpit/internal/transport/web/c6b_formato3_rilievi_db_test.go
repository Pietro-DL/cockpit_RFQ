//go:build integrazione

// L4 — C-6b, corsia file, rilievi della revisione (direttiva 39 §2, §5 [U + T-39]; INC-G9, D-AR07 [T-39]): nelle RFQ
// formato 3 i gesti documentali delle pagine legacy non toccano uno STEP del prodotto finito fuori da S8 e R5, e un
// documento non lascia il componente della sua AR. Ogni ingresso dalla rotta vera, con la fotografia di tutte le tabelle
// prima e dopo:
//   - dopo R5 vero (lo STEP sorgente S8 promosso: lo STEP strutturale della working): la conferma legacy di un secondo
//     STEP con «sostituisce» lo STEP della working, «Sostituisci» dalla rotta del Fascicolo, «Assegna» di un documento
//     con «sostituisce», «Assegna» di uno STEP del prodotto verso un altro componente, «Annulla sostituzione» di una
//     sostituzione verso lo STEP della working, la «Revoca» legacy: tutti rifiutano, niente cambia;
//   - la AR (TestC6bRilieviAssegnaDiUnDocumentoConLaAR): «Assegna» e «Togli» di un documento confermato con la AR attiva
//     su un altro componente rifiutano nel formato 3; un documento senza AR si sposta ancora.
// Solo dati sintetici.

package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/db"
)

// confermaLegacy: «Conferma il documento» della pagina della RFQ per la proposta; la risposta.
func (s *scenaC6b) confermaLegacy(prop uuid.UUID, f url.Values) string {
	s.t.Helper()
	_, html := s.op.fai(http.MethodPost, "/proposta/"+prop.String()+"/conferma", f, true)
	return html
}

// documentoDi: il documento della RFQ con il contenuto dell'allegato (uuid.Nil se non c'e').
func (s *scenaC6b) documentoDi(allegato uuid.UUID) uuid.UUID {
	s.t.Helper()
	var d uuid.UUID
	_ = s.pool.QueryRow(s.ctx, `SELECT d.documento_id FROM documento d JOIN allegato a ON a.sha256 = d.sha256
		WHERE a.allegato_id = $1 AND d.thread_id = $2`, allegato, s.thread).Scan(&d)
	return d
}

// stepNuovo: un altro STEP della RFQ con la proposta cad_3d.
func (s *scenaC6b) stepNuovo(nome string, seme int64) (allegato, proposta uuid.UUID) {
	s.t.Helper()
	allegato, _ = s.file(s.thread, nome, byteCasuali("x.stp", seme, 1_500), db.OrigineAllegatoOutlook)
	if _, err := s.pool.Exec(s.ctx, `UPDATE documento_proposta SET tipo_proposto = 'cad_3d' WHERE allegato_id = $1`, allegato); err != nil {
		s.t.Fatal(err)
	}
	return allegato, s.proposta(allegato)
}

// rifiutoF3Web: la risposta dice il rifiuto del formato 3 con la frase attesa, e nessuna tabella e' cambiata.
func (s *scenaC6b) rifiutoF3Web(nome, risposta string, prima map[string]string, frase string) {
	s.t.Helper()
	// l'avviso (testo senza le entita' HTML), o la risposta intera
	dice := func(testo string) bool { return strings.Contains(testo, "formato 3") && strings.Contains(testo, frase) }
	if !dice(avvisoC6b(risposta)) && !dice(risposta) {
		s.t.Errorf("%s: atteso il rifiuto del formato 3 («%s»), risposta %.600s", nome, frase, avvisoC6b(risposta)+" | "+risposta)
	}
	if d := differenze(prima, fotoDelDatabase(s.t, s.bancoWeb)); d != "" {
		s.t.Errorf("%s: il rifiuto ha scritto in %s", nome, d)
	}
}

func TestC6bRilieviGestiDocumentaliSulloStepDopoR5(t *testing.T) {
	s := nuovaScenaR5C6b(t)
	rp, v := s.r4()
	if len(rp.Blocchi) != 0 || !rp.Confermabile {
		t.Fatalf("R4 della scena: %+v", rp.Blocchi)
	}
	out, err := s.r5(s.richiestaR5(rp.FirmaAnteprima, v), nil)
	if err != nil || out.Esito != "confermata" || !out.Fonte.Promossa {
		t.Fatalf("R5: %+v %v", out, err)
	}
	stepA := s.documentoDi(s.step)
	if stepA == uuid.Nil || valoreDi(t, s.bancoWeb, `SELECT coalesce(step_strutturale_id::text, '-') FROM componente WHERE componente_id = $1`, s.p) != stepA.String() {
		t.Fatal("dopo R5 lo STEP sorgente e' lo STEP strutturale della working")
	}
	base := s.base()
	stepStrutturale := func() string {
		return valoreDi(t, s.bancoWeb, `SELECT coalesce(step_strutturale_id::text, '-') FROM componente WHERE componente_id = $1`, s.p)
	}

	// 1. la conferma legacy di un secondo STEP del prodotto con «sostituisce» lo STEP della working (e «no» al riferimento)
	allX, propX := s.stepNuovo("ACME-P1-REV.stp", 631)
	// la rotta della conferma rismista la RFQ dopo ogni risposta, anche dopo un rifiuto (dopoLaProposta): una conferma
	// rifiutata per un'altra ragione prima della fotografia, cosi' il rismistamento del file nuovo e' gia' fatto
	s.confermaLegacy(propX, url.Values{"tipo": {"cad_3d"}, "codice": {"ACME-P1"}, "componente_id": {s.p.String()},
		"scelta": {"sostituisci:" + uuid.NewString()}, "nuovo_riferimento": {"0"}})
	if s.documentoDi(allX) != uuid.Nil {
		t.Fatal("la conferma di assestamento doveva rifiutare")
	}
	prima := fotoDelDatabase(t, s.bancoWeb)
	risposta := s.confermaLegacy(propX, url.Values{"tipo": {"cad_3d"}, "codice": {"ACME-P1"}, "componente_id": {s.p.String()},
		"scelta": {"sostituisci:" + stepA.String()}, "nuovo_riferimento": {"0"}})
	s.rifiutoF3Web("conferma legacy con «sostituisce»", risposta, prima, "non si sostituisce da qui")

	// il secondo STEP entra accanto (come dopo S8 in una revisione: A e B documenti correnti del prodotto)
	risposta = s.confermaLegacy(propX, url.Values{"tipo": {"cad_3d"}, "codice": {"ACME-P1"}, "componente_id": {s.p.String()}, "scelta": {"aggiungi"}})
	docX := s.documentoDi(allX)
	if docX == uuid.Nil {
		t.Fatalf("il secondo STEP non e' entrato: %.400s", avvisoC6b(risposta))
	}

	// 2. «Sostituisci» dalla rotta del Fascicolo, con «no» al riferimento (la strada che non passava da DichiaraStrutturale)
	prima = fotoDelDatabase(t, s.bancoWeb)
	_, risposta = s.op.fai(http.MethodPost, base+"/documento/"+docX.String()+"/sostituisci",
		url.Values{"vecchio": {stepA.String()}, "nuovo_riferimento": {"0"}}, true)
	s.rifiutoF3Web("«Sostituisci» dal Fascicolo, «no»", risposta, prima, "non si sostituisce da qui")
	_, risposta = s.op.fai(http.MethodPost, base+"/documento/"+docX.String()+"/sostituisci",
		url.Values{"vecchio": {stepA.String()}, "nuovo_riferimento": {"1"}}, true)
	s.rifiutoF3Web("«Sostituisci» dal Fascicolo, «sì»", risposta, prima, "non si sostituisce da qui")

	// 3. «Assegna» di un documento senza componente al prodotto, con «sostituisce» lo STEP della working
	allY, propY := s.stepNuovo("ACME-P1-ALT.stp", 632)
	s.confermaLegacy(propY, url.Values{"tipo": {"cad_3d"}, "codice": {"ACME-P1"}})
	docY := s.documentoDi(allY)
	if docY == uuid.Nil || valoreDi(t, s.bancoWeb, `SELECT coalesce(componente_id::text, '-') FROM documento WHERE documento_id = $1`, docY) != "-" {
		t.Fatal("il terzo STEP, senza componente")
	}
	prima = fotoDelDatabase(t, s.bancoWeb)
	_, risposta = s.op.fai(http.MethodPost, base+"/assegna", url.Values{"documento": {docY.String()}, "componente": {s.p.String()},
		"scelta": {"sostituisci:" + stepA.String()}, "nuovo_riferimento": {"0"}}, true)
	s.rifiutoF3Web("«Assegna» con «sostituisce»", risposta, prima, "non si sostituisce da qui")

	// 4. «Assegna» di uno STEP del prodotto (il secondo, confermato come documento del finito) verso un altro componente
	_, risposta = s.op.fai(http.MethodPost, base+"/assegna", url.Values{"documento": {docX.String()}, "componente": {s.c.String()}, "correggi_codice": {"1"}}, true)
	s.rifiutoF3Web("«Assegna» di uno STEP del prodotto", risposta, prima, "non si sposta con «Assegna»")

	// 5. la «Revoca» legacy dello STEP strutturale dopo R5
	_, risposta = s.op.fai(http.MethodPost, base+"/componente/"+s.p.String()+"/step-strutturale/revoca",
		url.Values{"sha": {s.shaStep}, "motivo": {"prova"}}, true)
	s.rifiutoF3Web("«Revoca» legacy dopo R5", risposta, prima, "non si revoca da qui")

	// 6. «Annulla sostituzione» di una sostituzione verso lo STEP della working (il secondo STEP sostituito da A, scritto qui
	// a mano come una sostituzione legacy precedente: la catena la tiene il database)
	if _, err := s.pool.Exec(s.ctx, `UPDATE documento SET sostituito_da = $2 WHERE documento_id = $1`, docX, stepA); err != nil {
		t.Fatalf("la sostituzione di prova: %v", err)
	}
	prima = fotoDelDatabase(t, s.bancoWeb)
	_, risposta = s.op.fai(http.MethodPost, base+"/documento/"+docX.String()+"/annulla-sostituzione", url.Values{}, true)
	s.rifiutoF3Web("«Annulla sostituzione»", risposta, prima, "non si annulla da qui")

	if got := stepStrutturale(); got != stepA.String() {
		t.Errorf("lo STEP strutturale della working: %s, atteso %s", got, stepA)
	}
	if got := valoreDi(t, s.bancoWeb, `SELECT coalesce(sostituito_da::text, '-') FROM documento WHERE documento_id = $1`, stepA); got != "-" {
		t.Errorf("lo STEP della working e' sostituito: %s", got)
	}
}

// La AR (INC-G9, D-AR07 [T-39]): nella RFQ formato 3 un documento confermato non lascia con «Assegna» o «Togli» il
// componente della sua AR attiva (la AR nata quando la RFQ era legacy); un documento senza AR si sposta ancora.
func TestC6bRilieviAssegnaDiUnDocumentoConLaAR(t *testing.T) {
	s := nuovaScenaC6b(t)
	// RFQ legacy: la AR di D1 su C1, poi D1 confermato su C1 (la conferma vuole il componente della AR); D2 confermato su C1
	// senza AR
	if a := s.assegna(s.prop1, &s.c); !strings.Contains(a, "assegnato al componente ACME-C1") {
		t.Fatalf("AR legacy: %q", a)
	}
	s.confermaLegacy(s.prop1, url.Values{"tipo": {"disegno_2d"}, "codice": {"ACME-C1"}, "componente_id": {s.c.String()}})
	s.confermaLegacy(s.prop2, url.Values{"tipo": {"disegno_2d"}, "codice": {"ACME-C1"}, "componente_id": {s.c.String()}, "scelta": {"aggiungi"}})
	d1, d2 := s.documentoDi(s.pdf1), s.documentoDi(s.pdf2)
	if d1 == uuid.Nil || d2 == uuid.Nil {
		t.Fatal("i due documenti su ACME-C1")
	}
	// la RFQ diventa formato 3
	s.creaBozza(distintabozza.PartenzaVuota)
	base := s.base()

	prima := fotoDelDatabase(t, s.bancoWeb)
	_, risposta := s.op.fai(http.MethodPost, base+"/assegna", url.Values{"documento": {d1.String()}, "componente": {s.c2.String()},
		"correggi_codice": {"1"}, "scelta": {"aggiungi"}}, true)
	s.rifiutoF3Web("«Assegna» di D1 su ACME-C2", risposta, prima, "ha un'associazione registrata con ACME-C1")
	_, risposta = s.op.fai(http.MethodPost, base+"/assegna", url.Values{"documento": {d1.String()}, "componente": {""}}, true)
	s.rifiutoF3Web("«Togli» di D1", risposta, prima, "ha un'associazione registrata con ACME-C1")
	if got := valoreDi(t, s.bancoWeb, `SELECT c.codice FROM documento d JOIN componente c ON c.componente_id = d.componente_id WHERE d.documento_id = $1`, d1); got != "ACME-C1" {
		t.Errorf("D1 sta su %s", got)
	}

	// D2, senza AR: si sposta ancora
	_, risposta = s.op.fai(http.MethodPost, base+"/assegna", url.Values{"documento": {d2.String()}, "componente": {s.c2.String()},
		"correggi_codice": {"1"}, "scelta": {"aggiungi"}}, true)
	if got := valoreDi(t, s.bancoWeb, `SELECT c.codice FROM documento d JOIN componente c ON c.componente_id = d.componente_id WHERE d.documento_id = $1`, d2); got != "ACME-C2" {
		t.Errorf("D2 senza AR non si e' spostato: %s (%.300s)", got, avvisoC6b(risposta))
	}
	if n := s.conteggio(`SELECT count(*) FROM file_rfq_associazione WHERE stato = 'attiva' AND allegato_id = $1 AND componente_id = $2`, s.pdf1, s.c); n != 1 {
		t.Errorf("la AR di D1 su ACME-C1: %d", n)
	}
}
