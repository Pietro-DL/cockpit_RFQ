//go:build integrazione

// L4 — C-6b, corsia file (direttiva 39 §6 [U]; D-AR03 D, messaggio 38 [U]; piano della direttiva 35 §7.2 C-6): i gesti
// atomici delle pagine legacy e della vecchia pagina del Motore A rispetto alle associazioni registrate (AR).
//   - RFQ interamente legacy: ogni ingresso passa dal servizio unico (AR gesto_legacy con l'evento e la copia, la
//     sostituzione esplicita, la copia senza AR tolta con l'evento, la chiusura con sgancio_legacy, scarto_legacy,
//     file_generale); lo STEP del finito e' il legame della sorgente, mai una AR; la correzione del codice allinea la copia
//     dal servizio;
//   - RFQ formato 3 senza bozza aperta: ogni ingresso atomico rifiuta e non cambia niente (prova d'insieme sul dominio);
//   - RFQ formato 3 con la bozza aperta: «Assegna» e «Togli» diventano salvataggi R2 della bozza (PB, svincolo), mai una
//     AR o la copia (prova d'insieme su AR, giornale e copia); lo STEP del finito rimanda a S8;
//   - Prova 11 (compatibilita'): nessuna copia diverge dalla AR attiva; il worker non tocca la copia.
// Solo dati sintetici.

package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// scenaC6b: la RFQ della Sorgente con un prodotto finito P, due componenti C e C2 sotto P, due PDF e uno STEP del
// prodotto, ognuno con la sua proposta aperta.
type scenaC6b struct {
	*bancoSorgente
	t                    *testing.T
	utente, p, c, c2     uuid.UUID
	pdf1, pdf2, step     uuid.UUID
	prop1, prop2, propSt uuid.UUID
}

func nuovaScenaC6b(t *testing.T) *scenaC6b {
	t.Helper()
	b := preparaBancoWeb(t)
	s := &scenaC6b{bancoSorgente: b.sorgente(t, true), t: t}
	// la vecchia pagina del Motore A: i suoi gesti passano dal ponte (esito ok/no), a flag acceso
	mwAccendi(t, b, motorea_web.Regole{Accesa: true, Stato: motorea_web.StatoIndiceAssente})
	s.utente = uuidSQL(t, b, `SELECT utente_id FROM utente WHERE sigla = 'FP'`)
	ins := func(codice, tipo string) uuid.UUID {
		return uuidSQL(t, b, `INSERT INTO componente (thread_id, codice, tipo, confermato_da) VALUES ($1, $2, $3::tipo_componente, $4) RETURNING componente_id`,
			s.thread, codice, tipo, s.utente)
	}
	s.p, s.c, s.c2 = ins("ACME-P1", "finito"), ins("ACME-C1", "sciolto"), ins("ACME-C2", "sciolto")
	for _, f := range []uuid.UUID{s.c, s.c2} {
		if _, err := s.pool.Exec(s.ctx, `INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da)
			VALUES ($1, $2, $3, 1, 'manuale', $4)`, s.thread, s.p, f, s.utente); err != nil {
			t.Fatal(err)
		}
	}
	s.pdf1, _ = s.file(s.thread, "ACME-D1.pdf", byteCasuali("x.pdf", 601, 1_500), db.OrigineAllegatoOutlook)
	s.pdf2, _ = s.file(s.thread, "ACME-D2.pdf", byteCasuali("x.pdf", 602, 1_500), db.OrigineAllegatoOutlook)
	s.step, _ = s.file(s.thread, "ACME-P1.stp", byteCasuali("x.stp", 603, 1_500), db.OrigineAllegatoOutlook)
	if _, err := s.pool.Exec(s.ctx, `UPDATE documento_proposta SET tipo_proposto = 'cad_3d' WHERE allegato_id = $1`, s.step); err != nil {
		t.Fatal(err)
	}
	s.prop1, s.prop2, s.propSt = s.proposta(s.pdf1), s.proposta(s.pdf2), s.proposta(s.step)
	return s
}

// avvisoC6b: l'avviso della pagina della RFQ, o quello del Fascicolo.
func avvisoC6b(html string) string {
	if a := avvisoDi(html); a != "" {
		return a
	}
	return avvisoF(html)
}

func (s *scenaC6b) proposta(allegato uuid.UUID) uuid.UUID {
	return uuidSQL(s.t, s.bancoWeb, `SELECT proposta_id FROM documento_proposta WHERE allegato_id = $1`, allegato)
}

func (s *scenaC6b) base() string { return "/thread/" + s.thread.String() + "/fascicolo" }

// assegna: «Assegna» (o «Togli» con comp nil) dalla pagina del Fascicolo; l'avviso.
func (s *scenaC6b) assegna(prop uuid.UUID, comp *uuid.UUID) string {
	s.t.Helper()
	return s.assegnaCon(prop, comp, url.Values{})
}

// assegnaCorreggi: «Assegna» con correggi_codice (lo spostamento di una proposta che ha gia' il codice di un altro
// componente, come prima della direttiva).
func (s *scenaC6b) assegnaCorreggi(prop uuid.UUID, comp *uuid.UUID) string {
	s.t.Helper()
	return s.assegnaCon(prop, comp, url.Values{"correggi_codice": {"1"}})
}

func (s *scenaC6b) assegnaCon(prop uuid.UUID, comp *uuid.UUID, f url.Values) string {
	s.t.Helper()
	f.Set("proposta", prop.String())
	f.Set("componente", "")
	if comp != nil {
		f.Set("componente", comp.String())
	}
	_, html := s.op.fai(http.MethodPost, s.base()+"/assegna", f, true)
	return avvisoC6b(html)
}

// ar: lo stato delle AR dell'allegato, «componente:origine:stato:motivo» in ordine di nascita.
func (s *scenaC6b) ar(allegato uuid.UUID) string {
	s.t.Helper()
	return valoreDi(s.t, s.bancoWeb, `SELECT coalesce(string_agg(c.codice || ':' || a.origine || ':' || a.stato || ':' || coalesce(a.motivo_chiusura, '-'), ' '
		ORDER BY a.creata_il, a.associazione_id), '') FROM file_rfq_associazione a JOIN componente c ON c.componente_id = a.componente_id
		WHERE a.allegato_id = $1`, allegato)
}

// copia: il componente della copia della proposta dell'allegato (codice, o «-»).
func (s *scenaC6b) copia(allegato uuid.UUID) string {
	s.t.Helper()
	return valoreDi(s.t, s.bancoWeb, `SELECT coalesce(c.codice, '-') || '/' || coalesce(p.codice, '-') FROM documento_proposta p
		LEFT JOIN componente c ON c.componente_id = p.componente_id WHERE p.allegato_id = $1`, allegato)
}

// eventi: le azioni del giornale delle associazioni dell'allegato, «azione:origine».
func (s *scenaC6b) eventi(allegato uuid.UUID) string {
	s.t.Helper()
	return valoreDi(s.t, s.bancoWeb, `SELECT coalesce(string_agg(azione || ':' || origine, ' ' ORDER BY evento_id), '') FROM associazione_evento
		WHERE allegato_id = $1 OR proposta_id IN (SELECT proposta_id FROM documento_proposta WHERE allegato_id = $1)`, allegato)
}

// fotoAR: le AR, il loro giornale e le copie (componente e codice delle proposte): quello che un gesto atomico di una
// RFQ formato 3 non deve mai cambiare.
func (s *scenaC6b) fotoAR() string {
	s.t.Helper()
	return valoreDi(s.t, s.bancoWeb, `SELECT
		(SELECT count(*)::text || ' ' || coalesce(md5(string_agg(x::text, '|' ORDER BY x::text)), '-') FROM file_rfq_associazione x) || ' # ' ||
		(SELECT count(*)::text || ' ' || coalesce(md5(string_agg(x::text, '|' ORDER BY x::text)), '-') FROM associazione_evento x) || ' # ' ||
		(SELECT coalesce(md5(string_agg(proposta_id::text || ':' || coalesce(componente_id::text, '-') || ':' || coalesce(codice, '-'), '|'
			ORDER BY proposta_id)), '-') FROM documento_proposta)`)
}

// creaBozza: una bozza formato 3 del prodotto, creata da R2 con la partenza data (vuota, registrato); rende la RFQ
// formato 3.
func (s *scenaC6b) creaBozza(partenza string) (uuid.UUID, int) {
	s.t.Helper()
	raw, _ := json.Marshal(map[string]any{"bozza_id": nil, "versione_attesa": 0, "prodotto_id": s.p, "vocabolario": 2,
		"chiave_salvataggio": uuid.New(), "crea": map[string]any{"partenza": partenza}})
	req, err := distintabozza.LeggiRichiestaR2(raw)
	if err != nil {
		s.t.Fatalf("crea %s: %v", partenza, err)
	}
	c, err := distintabozza.CreaFormato3(s.ctx, s.pool, req.CreaDaRichiesta(s.thread, s.utente, nil))
	if err != nil {
		s.t.Fatalf("crea %s: %v", partenza, err)
	}
	return c.BozzaID, c.Versione
}

func (s *scenaC6b) abbandona(bozza uuid.UUID, versione int) {
	s.t.Helper()
	if _, err := distintabozza.Abbandona(s.ctx, s.pool, distintabozza.Abbandono{ThreadID: s.thread, BozzaID: bozza, UtenteID: s.utente,
		VersioneAttesa: versione, Motivo: "prova C-6b"}); err != nil {
		s.t.Fatalf("abbandona: %v", err)
	}
}

// snapshot: lo snapshot della versione corrente della bozza aperta del prodotto, e la versione.
func (s *scenaC6b) snapshot() (distintabozza.Snapshot3, int) {
	s.t.Helper()
	var raw []byte
	var imp string
	var v int
	if err := s.pool.QueryRow(s.ctx, `SELECT v.snapshot, v.impronta, b.versione FROM distinta_bozza b
		JOIN distinta_bozza_versione v ON v.bozza_id = b.bozza_id AND v.versione = b.versione
		WHERE b.thread_id = $1 AND b.prodotto_id = $2 AND b.stato = 'aperta'`, s.thread, s.p).Scan(&raw, &imp, &v); err != nil {
		s.t.Fatal(err)
	}
	l, err := distintabozza.RileggiVersione(3, raw, imp)
	if err != nil || l.Formato3 == nil {
		s.t.Fatalf("snapshot: %v", err)
	}
	return *l.Formato3, v
}

// divergenze (Prova 11): le proposte aperte la cui copia non dice il componente della AR attiva.
func (s *scenaC6b) divergenze() int {
	return s.conteggio(`SELECT count(*) FROM documento_proposta p JOIN file_rfq_associazione a ON a.allegato_id = p.allegato_id AND a.stato = 'attiva'
		WHERE p.stato = 'aperta' AND p.componente_id IS DISTINCT FROM a.componente_id`)
}

// RFQ interamente legacy (D-AR03 D punto 6 [U]): ogni gesto atomico passa dal servizio unico.
func TestC6bGestiLegacyDalServizioUnico(t *testing.T) {
	s := nuovaScenaC6b(t)

	// «Assegna» dal Fascicolo: AR gesto_legacy, evento «assegna» di origine legacy, la copia
	if a := s.assegna(s.prop1, &s.c); !strings.Contains(a, "assegnato al componente ACME-C1") {
		t.Fatalf("assegna: %q", a)
	}
	if got := s.ar(s.pdf1); got != "ACME-C1:gesto_legacy:attiva:-" {
		t.Errorf("AR dopo assegna: %q", got)
	}
	if got := s.copia(s.pdf1); got != "ACME-C1/ACME-C1" {
		t.Errorf("copia dopo assegna: %q", got)
	}
	// lo stesso gesto ripetuto: invariato, nessuna riga nuova
	s.assegna(s.prop1, &s.c)
	if got := s.ar(s.pdf1); got != "ACME-C1:gesto_legacy:attiva:-" {
		t.Errorf("AR dopo il gesto ripetuto: %q", got)
	}
	// verso un altro componente: la sostituzione esplicita
	if a := s.assegna(s.prop1, &s.c2); !strings.Contains(a, "codice diverso") {
		t.Fatalf("assegna verso C2 senza correggi_codice (la regola di sempre): %q", a)
	}
	if a := s.assegnaCorreggi(s.prop1, &s.c2); !strings.Contains(a, "assegnato al componente ACME-C2") {
		t.Fatalf("assegna verso C2: %q", a)
	}
	if got := s.ar(s.pdf1); got != "ACME-C1:gesto_legacy:chiusa:sostituita ACME-C2:gesto_legacy:attiva:-" {
		t.Errorf("AR dopo lo spostamento: %q", got)
	}
	// «Togli»: sgancio_legacy, la copia tolta
	s.assegna(s.prop1, nil)
	if got := s.ar(s.pdf1); !strings.HasSuffix(got, "ACME-C2:gesto_legacy:chiusa:sgancio_legacy") {
		t.Errorf("AR dopo Togli: %q", got)
	}
	if got := s.copia(s.pdf1); !strings.HasPrefix(got, "-/") {
		t.Errorf("copia dopo Togli: %q", got)
	}
	if got := s.eventi(s.pdf1); got != "assegna:legacy sostituisci:legacy sgancia:legacy" {
		t.Errorf("giornale di pdf1: %q", got)
	}

	// la pre-assegnazione legacy (copia senza AR) su un altro componente: tolta con l'evento, poi la AR
	testutil.CopiaLegacy(t, s.pool, `UPDATE documento_proposta SET componente_id = $2, codice = 'ACME-C1' WHERE allegato_id = $1`, s.pdf2, s.c)
	if a := s.assegnaCorreggi(s.prop2, &s.c2); !strings.Contains(a, "assegnato al componente ACME-C2") {
		t.Fatalf("assegna della pre-assegnazione: %q", a)
	}
	if got := s.ar(s.pdf2); got != "ACME-C2:gesto_legacy:attiva:-" {
		t.Errorf("AR dalla pre-assegnazione: %q", got)
	}
	if got := s.eventi(s.pdf2); got != "sgancia:legacy assegna:legacy" {
		t.Errorf("giornale della pre-assegnazione: %q", got)
	}

	// «Usa come STEP»: lo STEP del prodotto finito e' il legame della sorgente, mai una AR (dal ponte della vecchia
	// pagina del Motore A, come lo manda apriUsaStep)
	if e, testo := s.op.gestoPonte(t, s.base()+"/assegna", url.Values{"proposta": {s.propSt.String()}, "componente": {s.p.String()},
		"correggi_codice": {"1"}}, s.thread); e != "ok" {
		t.Fatalf("Usa come STEP: %s %q", e, testo)
	}
	if got := s.ar(s.step); got != "" {
		t.Errorf("lo STEP del finito non e' una AR: %q", got)
	}
	if got := s.copia(s.step); got != "ACME-P1/ACME-P1" {
		t.Errorf("la copia del legame della sorgente: %q", got)
	}
	if got := s.conteggio(`SELECT count(*) FROM fonte_step_evento WHERE allegato_id = $1 AND azione = 'caricamento' AND ruolo = 'prodotto'`, s.step); got != 1 {
		t.Errorf("il legame della sorgente in fonte_step_evento: %d", got)
	}

	// il vecchio «Scarta il file» (INC-G10): la AR si chiude dal servizio prima della decisione, la proposta scartata non
	// tiene il componente
	s.assegna(s.prop1, &s.c)
	if e, testo := s.op.gestoPonte(t, "/proposta/"+s.prop1.String()+"/scarta", url.Values{"ritorna_thread": {s.thread.String()}}, s.thread); e != "ok" {
		t.Fatalf("vecchio Scarta: %s %q", e, testo)
	}
	if got := s.ar(s.pdf1); !strings.HasSuffix(got, "ACME-C1:gesto_legacy:chiusa:scarto_legacy") {
		t.Errorf("AR dopo il vecchio Scarta: %q", got)
	}
	if got := valoreDi(t, s.bancoWeb, `SELECT stato::text || ':' || coalesce(componente_id::text, '-') FROM documento_proposta WHERE allegato_id = $1`, s.pdf1); got != "scartata:-" {
		t.Errorf("la proposta scartata: %q", got)
	}

	// «Documentazione generale»: la AR si chiude con file_generale, il documento nasce senza componente
	if e, testo := s.op.gestoPonte(t, s.base()+"/file/"+s.prop2.String()+"/generale", url.Values{"tipo": {"altro"}}, s.thread); e != "ok" {
		t.Fatalf("Documentazione generale: %s %q", e, testo)
	}
	if got := s.ar(s.pdf2); got != "ACME-C2:gesto_legacy:chiusa:file_generale" {
		t.Errorf("AR dopo Documentazione generale: %q", got)
	}
	if got := valoreDi(t, s.bancoWeb, `SELECT coalesce(d.componente_id::text, '-') FROM documento d JOIN documento_provenienza dp ON dp.documento_id = d.documento_id
		WHERE dp.allegato_id = $1`, s.pdf2); got != "-" {
		t.Errorf("il documento generale: %q", got)
	}

	// la correzione del codice: la copia prende il codice nuovo dal servizio, la AR non cambia
	pdf3, _ := s.file(s.thread, "ACME-D3.pdf", byteCasuali("x.pdf", 604, 1_500), db.OrigineAllegatoOutlook)
	s.assegna(s.proposta(pdf3), &s.c2)
	prima := s.ar(pdf3)
	_, html := s.op.fai(http.MethodPost, s.base()+"/componente/"+s.c2.String()+"/codice", url.Values{"codice": {"ACME-C2B"}}, true)
	if a := avvisoC6b(html); !strings.Contains(a, "Codice corretto") {
		t.Fatalf("correzione del codice: %q", a)
	}
	if got := s.copia(pdf3); got != "ACME-C2B/ACME-C2B" {
		t.Errorf("la copia dopo la correzione: %q", got)
	}
	if got := s.ar(pdf3); got != strings.ReplaceAll(prima, "ACME-C2:", "ACME-C2B:") {
		t.Errorf("la AR dopo la correzione: %q (prima %q)", got, prima)
	}

	// Prova 11: il worker (UpsertProposta, una lettura nuova del file) non sovrascrive la decisione: la copia tiene
	// componente e codice, la lettura va nei dettagli
	if _, err := db.New(s.pool).UpsertProposta(s.ctx, db.UpsertPropostaParams{AllegatoID: pdf3, ThreadID: uuid.NullUUID{UUID: s.thread, Valid: true},
		TipoProposto: db.TipoDocumentoDisegno2d, Codice: pgtextC6b("ZZ-ALTRO"), Confidenza: 90, Fonte: db.FontePropostaCartiglio,
		Dettagli: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("UpsertProposta del worker: %v", err)
	}
	if got := s.copia(pdf3); got != "ACME-C2B/ACME-C2B" {
		t.Errorf("Prova 11: il worker ha riscritto la copia: %q", got)
	}
	// Prova 11: nessuna copia diverge dalla AR attiva; la RFQ non e' diventata formato 3
	if n := s.divergenze(); n != 0 {
		t.Errorf("Prova 11: %d copie divergono dalla AR attiva", n)
	}
	if got := valoreDi(t, s.bancoWeb, `SELECT rfq_e_formato3($1)::text`, s.thread); got != "false" {
		t.Errorf("la RFQ e' ancora legacy: %s", got)
	}
}

// RFQ formato 3 senza bozza aperta (D-AR03 D punto 5 [U]): ogni ingresso atomico rifiuta, invita al percorso di
// revisione e non cambia niente nel dominio (prova d'insieme prima e dopo, per ingresso). La AR nata quando la RFQ era
// legacy resta com'era. Lo STEP del finito rimanda a S8. L'abbandono della bozza non riporta la RFQ a legacy.
func TestC6bFormato3SenzaBozzaNessunGestoAtomicoScriveAR(t *testing.T) {
	s := nuovaScenaC6b(t)
	s.assegna(s.prop2, &s.c) // la AR nata nella RFQ legacy
	if got := s.ar(s.pdf2); got != "ACME-C1:gesto_legacy:attiva:-" {
		t.Fatalf("AR legacy: %q", got)
	}
	bz, v := s.creaBozza(distintabozza.PartenzaVuota)
	s.abbandona(bz, v)
	if got := valoreDi(t, s.bancoWeb, `SELECT rfq_e_formato3($1)::text`, s.thread); got != "true" {
		t.Fatalf("dopo l'abbandono la RFQ resta formato 3: %s", got)
	}
	casi := []struct {
		nome, percorso string
		form           url.Values
		frase          string
	}{
		{"assegna dal Fascicolo", s.base() + "/assegna", url.Values{"proposta": {s.prop1.String()}, "componente": {s.c.String()}}, "apri la revisione"},
		{"togli dal Fascicolo (AR legacy)", s.base() + "/assegna", url.Values{"proposta": {s.prop2.String()}, "componente": {""}}, "apri la revisione"},
		{"sposta su un altro componente (AR legacy)", s.base() + "/assegna", url.Values{"proposta": {s.prop2.String()}, "componente": {s.c2.String()}}, "apri la revisione"},
		{"Usa come STEP", s.base() + "/assegna", url.Values{"proposta": {s.propSt.String()}, "componente": {s.p.String()}, "correggi_codice": {"1"}}, "Conferma la sorgente"},
		{"vecchio Scarta il file (AR legacy)", "/proposta/" + s.prop2.String() + "/scarta", url.Values{"ritorna_thread": {s.thread.String()}}, "Scollega e scarta"},
		{"Documentazione generale (AR legacy)", s.base() + "/file/" + s.prop2.String() + "/generale", url.Values{"tipo": {"altro"}}, "svincola nella bozza"},
		{"due proposte insieme", s.base() + "/assegna", url.Values{"proposta": {s.prop1.String(), s.propSt.String()}, "componente": {s.c.String()}}, "un file alla volta"},
	}
	for _, c := range casi {
		for _, ponte := range []bool{false, true} {
			prima := fotoDominio(t, s.bancoWeb)
			var testo string
			if ponte {
				var e string
				e, testo = s.op.gestoPonte(t, c.percorso, c.form, s.thread)
				if e != "no" {
					t.Errorf("%s dal ponte: esito %q (%q)", c.nome, e, testo)
				}
			} else {
				_, html := s.op.fai(http.MethodPost, c.percorso, c.form, true)
				testo = avvisoC6b(html)
			}
			if !strings.Contains(testo, c.frase) {
				t.Errorf("%s (ponte %v): %q, manca %q", c.nome, ponte, testo, c.frase)
			}
			if d := differenze(prima, fotoDominio(t, s.bancoWeb)); d != "" {
				t.Errorf("%s (ponte %v): tabelle cambiate %s", c.nome, ponte, d)
			}
		}
	}
	if got := s.ar(s.pdf2); got != "ACME-C1:gesto_legacy:attiva:-" {
		t.Errorf("la AR legacy dopo i rifiuti: %q", got)
	}
	// il vecchio «Scarta il file» di un file senza legami resta: decide la proposta, senza AR ne' copia
	foto := s.fotoAR()
	if e, testo := s.op.gestoPonte(t, "/proposta/"+s.prop1.String()+"/scarta", url.Values{"ritorna_thread": {s.thread.String()}}, s.thread); e != "ok" {
		t.Fatalf("vecchio Scarta senza legami: %s %q", e, testo)
	}
	if got := s.fotoAR(); got != foto {
		t.Error("il vecchio Scarta senza legami ha cambiato AR, giornale o copia")
	}
	// la correzione del codice: AR e componente della copia non cambiano (il codice della copia segue il componente, FK)
	arPrima := valoreDi(t, s.bancoWeb, `SELECT coalesce(md5(string_agg(x::text, '|' ORDER BY x::text)), '-') FROM file_rfq_associazione x`)
	_, html := s.op.fai(http.MethodPost, s.base()+"/componente/"+s.c.String()+"/codice", url.Values{"codice": {"ACME-C1B"}}, true)
	if a := avvisoC6b(html); !strings.Contains(a, "Codice corretto") {
		t.Fatalf("correzione del codice nel formato 3: %q", a)
	}
	if got := valoreDi(t, s.bancoWeb, `SELECT coalesce(md5(string_agg(x::text, '|' ORDER BY x::text)), '-') FROM file_rfq_associazione x`); got != arPrima {
		t.Error("la correzione del codice ha cambiato le AR")
	}
	if got := s.copia(s.pdf2); got != "ACME-C1B/ACME-C1B" {
		t.Errorf("la copia segue il codice del componente: %q", got)
	}
	if n := s.divergenze(); n != 0 {
		t.Errorf("Prova 11: %d copie divergono dalla AR attiva", n)
	}
}

// RFQ formato 3 con la bozza aperta (D-AR03 D punti 2, 3, 5 [U]): «Assegna» e «Togli» delle pagine legacy e della
// vecchia pagina del Motore A diventano salvataggi R2 della bozza (PB, svincolo dell'AR legacy), con il giornale della
// bozza; AR, giornale delle AR e copie non cambiano mai. La pagina nuova, ferma alla versione di prima, riceve 409.
func TestC6bFormato3ConBozzaIlGestoDiventaR2(t *testing.T) {
	s := nuovaScenaC6b(t)
	s.assegna(s.prop2, &s.c) // la AR nata nella RFQ legacy
	s.creaBozza(distintabozza.PartenzaRegistrato)
	snap, v0 := s.snapshot()
	chiaveDi := func(comp uuid.UUID) string {
		for _, n := range snap.Nodi {
			if n.ComponenteID != nil && *n.ComponenteID == comp {
				return n.ChiaveBozza
			}
		}
		t.Fatalf("il componente %s non e' nella bozza registrata", comp)
		return ""
	}
	kC, kC2 := chiaveDi(s.c), chiaveDi(s.c2)
	foto := s.fotoAR()

	// «Assegna» pdf1 → C: la PB nella bozza, versione nuova; la provenienza la decide il server di R2 (manuale: direttiva 39
	// §4, richiesta 4 della corsia file, recepita dalla persistenza in 1ebc110)
	if a := s.assegna(s.prop1, &s.c); !strings.Contains(a, "preassociato a ACME-C1 nella bozza") {
		t.Fatalf("assegna nel formato 3: %q", a)
	}
	snap1, v1 := s.snapshot()
	// la provenienza della PB la decide il server di R2 (manuale: la PB arriva senza, dalla pagina legacy; R39-24)
	if v1 != v0+1 || len(snap1.DocumentiPreassociati) != 1 || snap1.DocumentiPreassociati[0].AllegatoID != s.pdf1 ||
		snap1.DocumentiPreassociati[0].ChiaveBozza != kC || snap1.DocumentiPreassociati[0].Provenienza != "manuale" {
		t.Fatalf("PB dopo assegna: v%d %+v", v1, snap1.DocumentiPreassociati)
	}
	// dal ponte, verso C2: la PB si sposta (file_svincolato + file_preassociato nello stesso gesto)
	if e, testo := s.op.gestoPonte(t, s.base()+"/assegna", url.Values{"proposta": {s.prop1.String()}, "componente": {s.c2.String()}}, s.thread); e != "ok" {
		t.Fatalf("assegna dal ponte nel formato 3: %s %q", e, testo)
	}
	snap2, v2 := s.snapshot()
	if v2 != v1+1 || len(snap2.DocumentiPreassociati) != 1 || snap2.DocumentiPreassociati[0].ChiaveBozza != kC2 {
		t.Fatalf("PB dopo lo spostamento: v%d %+v", v2, snap2.DocumentiPreassociati)
	}
	if got := valoreDi(t, s.bancoWeb, `SELECT string_agg(e.azione, ' ' ORDER BY e.n) FROM distinta_bozza_evento e JOIN distinta_bozza b ON b.bozza_id = e.bozza_id
		WHERE b.thread_id = $1 AND b.stato = 'aperta' AND e.versione > $2`, s.thread, v0); got != "file_preassociato file_svincolato file_preassociato" {
		t.Errorf("giornale della bozza: %q", got)
	}
	// «Togli»: la PB esce
	if a := s.assegna(s.prop1, nil); !strings.Contains(a, "tolto dalla bozza") {
		t.Fatalf("togli nel formato 3: %q", a)
	}
	// «Togli» di un file con la AR legacy e senza PB: lo svincolo si prepara nella bozza
	if a := s.assegna(s.prop2, nil); !strings.Contains(a, "Svincolo di ACME-D2.pdf da ACME-C1 preparato") {
		t.Fatalf("togli della AR nel formato 3: %q", a)
	}
	snap4, v4 := s.snapshot()
	trovato := false
	for _, d := range snap4.Decisioni {
		if d.Tipo == distintabozza.DecisioneScollegaDocumento && d.AllegatoID != nil && *d.AllegatoID == s.pdf2 && d.ComponenteID != nil && *d.ComponenteID == s.c {
			trovato = true
		}
	}
	if !trovato || len(snap4.DocumentiPreassociati) != 0 {
		t.Errorf("lo svincolo nella bozza: %+v / %+v", snap4.Decisioni, snap4.DocumentiPreassociati)
	}
	// ripetuto: niente di nuovo
	if a := s.assegna(s.prop2, nil); !strings.Contains(a, "è già nella bozza") {
		t.Errorf("togli ripetuto: %q", a)
	}
	if _, v := s.snapshot(); v != v4 {
		t.Errorf("il gesto ripetuto ha salvato una versione: %d", v)
	}
	// lo STEP del finito rimanda a S8; un componente fuori dalla bozza rifiuta
	altro := uuidSQL(t, s.bancoWeb, `INSERT INTO componente (thread_id, codice, tipo, confermato_da) VALUES ($1, 'ACME-X9', 'sciolto', $2) RETURNING componente_id`,
		s.thread, s.utente)
	foto2 := fotoDominio(t, s.bancoWeb)
	if a := s.assegna(s.propSt, &s.p); !strings.Contains(a, "Conferma la sorgente") {
		t.Errorf("Usa come STEP con la bozza: %q", a)
	}
	if a := s.assegna(s.prop1, &altro); !strings.Contains(a, "non è in nessuna bozza aperta") {
		t.Errorf("componente fuori dalla bozza: %q", a)
	}
	// il vecchio «Scarta il file» e «Documentazione generale» di un file nello svincolo della bozza: rifiuto
	if e, testo := s.op.gestoPonte(t, "/proposta/"+s.prop2.String()+"/scarta", url.Values{"ritorna_thread": {s.thread.String()}}, s.thread); e != "no" || !strings.Contains(testo, "Scollega e scarta") {
		t.Errorf("vecchio Scarta con la AR: %s %q", e, testo)
	}
	if d := differenze(foto2, fotoDominio(t, s.bancoWeb)); d != "" {
		t.Errorf("i rifiuti hanno scritto in %s", d)
	}
	// la pagina nuova ferma a v0 riceve 409 al salvataggio
	raw, _ := json.Marshal(map[string]any{"bozza_id": valoreDi(t, s.bancoWeb, `SELECT bozza_id::text FROM distinta_bozza WHERE thread_id = $1 AND stato = 'aperta'`, s.thread),
		"versione_attesa": v0, "prodotto_id": s.p, "vocabolario": 2, "chiave_salvataggio": uuid.New(), "snapshot": snap, "azioni": []any{}})
	req, err := distintabozza.LeggiRichiestaR2(raw)
	if err != nil {
		t.Fatal(err)
	}
	sv := req.Salvataggio
	sv.ThreadID, sv.UtenteID = s.thread, s.utente
	_, err = distintabozza.SalvaFormato3(s.ctx, s.pool, sv)
	var e *distintabozza.Errore
	if !asErrore(err, &e) || e.Codice != distintabozza.CodiceBozzaVersione {
		t.Errorf("la pagina ferma alla versione di prima: %v", err)
	}
	// prova d'insieme: nessun gesto ha cambiato AR, giornale delle AR o copie
	if got := s.fotoAR(); got != foto {
		t.Error("un gesto atomico nel formato 3 ha cambiato AR, giornale o copia")
	}
	if got := s.ar(s.pdf2); got != "ACME-C1:gesto_legacy:attiva:-" {
		t.Errorf("la AR legacy: %q", got)
	}
}

func pgtextC6b(v string) pgtype.Text { return pgtype.Text{String: v, Valid: true} }

func asErrore(err error, e **distintabozza.Errore) bool { return errors.As(err, e) }
