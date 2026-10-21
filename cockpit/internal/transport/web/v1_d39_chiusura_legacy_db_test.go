//go:build integrazione

// L4 — chiusura del legacy (direttiva 39, corsia persistenza, dopo C-6b), con il banco web vero e la scena di C-6b della
// corsia file (scenaC6b: helper riusati, il file resta della corsia file):
//   - Prova 11 della direttiva 35 sull'insieme: con trg_proposta_copia_derivata acceso (variante 20261019d) la copia
//     documento_proposta.componente_id resta uguale alla fonte dopo ogni ingresso (gesti legacy, pre-assegnazione P1,
//     «Usa come STEP», vecchio «Scarta», worker, presa in carico con la conversione P1, gesti legacy nella RFQ formato 3,
//     S8, R5, «Scollega e metti in Altri file»); una scrittura della copia fuori dal servizio unico e' rifiutata al COMMIT;
//   - Prova 9 con svincolo_gia_chiuso: lo svincolo R2 (da «Togli» legacy nella RFQ formato 3), poi «Scollega e scarta»
//     chiude la AR; R5 riconosce lo svincolo gia' chiuso (nessuna scrittura sulla AR, nessun evento nuovo), la firma di
//     prima dello scarto e' superata;
//   - INC-G25: lo scarto e R5 in concorrenza, nei due ordini (lo scarto tiene la RFQ mentre R5 aspetta; R5 tiene la RFQ
//     mentre lo scarto aspetta): nessuno stallo, nessuna scrittura a meta', lo stato finale coerente.
// Solo dati sintetici (ACME).

package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/testutil"
)

// copiaFuoriDallaFonte (Prova 11 sull'insieme): le proposte aperte la cui copia non e' quella che la fonte dice. La copia
// attesa e' il componente della AR attiva dell'allegato; senza AR attiva, il componente dopo l'ultimo evento del giornale
// delle associazioni dell'allegato (il legame della sorgente, uno sgancio, la conversione P1); senza AR ne' eventi la
// copia e' un dato legacy scritto prima del trigger (pre-assegnazione P1), contata a parte da preassegnazioniP1.
func copiaFuoriDallaFonte(t *testing.T, b *bancoWeb) int {
	t.Helper()
	var n int
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FROM documento_proposta p
		LEFT JOIN LATERAL (SELECT a.componente_id FROM file_rfq_associazione a
		                    WHERE a.allegato_id = p.allegato_id AND a.stato = 'attiva' LIMIT 1) ar ON true
		LEFT JOIN LATERAL (SELECT e.componente_dopo FROM associazione_evento e
		                    WHERE e.allegato_id = p.allegato_id ORDER BY e.evento_id DESC LIMIT 1) ev ON true
		WHERE p.stato = 'aperta'
		  AND (ar.componente_id IS NOT NULL OR EXISTS (SELECT 1 FROM associazione_evento e WHERE e.allegato_id = p.allegato_id))
		  AND p.componente_id IS DISTINCT FROM COALESCE(ar.componente_id, ev.componente_dopo)`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// preassegnazioniP1: le proposte aperte con una copia senza AR attiva e senza eventi (il dato legacy di prima del trigger).
func preassegnazioniP1(t *testing.T, b *bancoWeb) int {
	t.Helper()
	var n int
	if err := b.pool.QueryRow(b.ctx, `SELECT count(*) FROM documento_proposta p WHERE p.stato = 'aperta' AND p.componente_id IS NOT NULL
		AND NOT EXISTS (SELECT 1 FROM file_rfq_associazione a WHERE a.allegato_id = p.allegato_id AND a.stato = 'attiva')
		AND NOT EXISTS (SELECT 1 FROM associazione_evento e WHERE e.allegato_id = p.allegato_id)`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// controllaCopia: nessuna copia fuori dalla fonte, il numero atteso di P1, il trigger acceso.
func controllaCopia(t *testing.T, b *bancoWeb, passo string, p1 int) {
	t.Helper()
	if n := copiaFuoriDallaFonte(t, b); n != 0 {
		t.Errorf("Prova 11, %s: %d copie fuori dalla fonte", passo, n)
	}
	if n := preassegnazioniP1(t, b); n != p1 {
		t.Errorf("Prova 11, %s: %d pre-assegnazioni P1, attese %d", passo, n, p1)
	}
	if g := valoreDi(t, b, `SELECT tgenabled::text FROM pg_trigger WHERE tgname = 'trg_proposta_copia_derivata'`); g != "O" {
		t.Errorf("%s: trg_proposta_copia_derivata %s, atteso acceso", passo, g)
	}
}

// s8C6b: lo STEP del prodotto P della scena confermato come sorgente (il documento cad_3d e la conferma di S8 passo 2,
// scritti come li scrive S8), poi S8 passo 3 sulla bozza aperta (solo_prodotto, versione +1). Restituisce la versione.
func (s *scenaC6b) s8C6b(bozza uuid.UUID, versione int) int {
	s.t.Helper()
	sha := valoreDi(s.t, s.bancoWeb, `SELECT sha256 FROM allegato WHERE allegato_id = $1`, s.step)
	var doc uuid.UUID
	if err := s.pool.QueryRow(s.ctx, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, $2, 'cad_3d', 'ACME-P1', 'ACME-P1.stp', 'stp', $3, '3D/ACME-P1.stp', $4) RETURNING documento_id`, s.thread, s.p, sha, s.utente).Scan(&doc); err != nil {
		s.t.Fatal(err)
	}
	if _, err := s.pool.Exec(s.ctx, `INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, documento_id, chiave_idempotenza, autore_id)
		VALUES ($1, $2, 'prodotto', 'conferma_documento', $3, $4, $5, gen_random_uuid(), $6)`, s.thread, s.p, s.step, sha, doc, s.utente); err != nil {
		s.t.Fatal(err)
	}
	v, err := distintabozza.SoloProdottoSuBozzaFormato3(s.ctx, s.pool, distintabozza.SoloProdottoSuBozza3{ThreadID: s.thread, UtenteID: s.utente,
		Comune: distintabozza.ComuneR2{ProdottoID: s.p, BozzaID: bozza, VersioneAttesa: versione, ChiaveSalvataggio: uuid.New(),
			ImprontaRichiesta: strings.Repeat("a", 64)},
		Sorgente: distintabozza.SorgenteDichiarata{AllegatoID: s.step, Gruppo: distintabozza.GruppoSoloProdotto},
		Motivo:   "STEP senza struttura importabile", PresaDAttoAnalisi: "pronta"})
	if err != nil {
		s.t.Fatalf("S8 passo 3: %v", err)
	}
	return v.Versione
}

// corpoBozzaC6b: lo snapshot salvato della versione con bozza_id e bozza_versione (il corpo di R4 e la bozza di R5).
func (s *scenaC6b) corpoBozzaC6b(bozza uuid.UUID, versione int) map[string]any {
	s.t.Helper()
	var raw json.RawMessage
	if err := s.pool.QueryRow(s.ctx, `SELECT snapshot FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = $2`, bozza, versione).Scan(&raw); err != nil {
		s.t.Fatal(err)
	}
	m := map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		s.t.Fatal(err)
	}
	m["bozza_id"], m["bozza_versione"] = bozza, versione
	return m
}

// r4C6b: R4 via HTTP sulla versione salvata.
func (s *scenaC6b) r4C6b(bozza uuid.UUID, versione int) distintabozza.RiepilogoR4 {
	s.t.Helper()
	raw, _ := json.Marshal(s.corpoBozzaC6b(bozza, versione))
	// il riepilogo con il motore (la firma che R5 ricalcola): le regole ACME pronte per il cliente della scena
	accendiMotoreV1(s.t, s.bancoWeb, s.thread)
	resp, testo := s.ad.chiediJSON(http.MethodPost, "/thread/"+s.thread.String()+"/motore-a/bom/riepilogo?avvisi_motore=1", string(raw))
	var r distintabozza.RiepilogoR4
	if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &r) != nil {
		s.t.Fatalf("R4: %d %s", resp.StatusCode, testo)
	}
	return r
}

// corpoR5C6b: il corpo di R5 dalla versione salvata, con la firma e una chiave nuova.
func (s *scenaC6b) corpoR5C6b(bozza uuid.UUID, versione int, firma string) string {
	s.t.Helper()
	m := s.corpoBozzaC6b(bozza, versione)
	m["chiave_idempotenza"] = uuid.New()
	raw, _ := json.Marshal(map[string]any{"firma_anteprima": firma, "bozza": m})
	return string(raw)
}

// svincoloDi: l'esito dello svincolo del file nel riepilogo.
func svincoloDi(r distintabozza.RiepilogoR4, allegato uuid.UUID) string {
	var out []string
	for _, sv := range r.Svincoli {
		if sv.Allegato == allegato {
			out = append(out, sv.Esito)
		}
	}
	return strings.Join(out, " ")
}

// arRiga: la riga della AR (stato, motivo, conferma di chiusura) e le azioni del giornale dell'allegato (azione:origine).
func (s *scenaC6b) arRiga(allegato uuid.UUID) string {
	s.t.Helper()
	return valoreDi(s.t, s.bancoWeb, `SELECT string_agg(a.stato || ':' || coalesce(a.motivo_chiusura, '-') || ':' ||
		CASE WHEN a.chiusa_conferma_id IS NULL THEN 'senza_conferma' ELSE 'con_conferma' END, ' ' ORDER BY a.creata_il) || ' #' ||
		(SELECT string_agg(e.azione || ':' || e.origine, ' ' ORDER BY e.evento_id) FROM associazione_evento e WHERE e.allegato_id = $1)
		FROM file_rfq_associazione a WHERE a.allegato_id = $1`, allegato)
}

// Prova 11 (direttiva 35) sull'insieme degli ingressi che toccano la copia, con il trigger acceso.
func TestV1D39Prova11CopiaUgualeAllaFonte(t *testing.T) {
	s := nuovaScenaC6b(t)
	controllaCopia(t, s.bancoWeb, "scena", 0)

	// RFQ legacy: «Assegna», lo spostamento, «Togli» (D-AR03 D punto 6 [U]: dal servizio unico)
	if a := s.assegna(s.prop1, &s.c); !strings.Contains(a, "assegnato al componente ACME-C1") {
		t.Fatalf("assegna: %q", a)
	}
	controllaCopia(t, s.bancoWeb, "assegna", 0)
	if a := s.assegnaCorreggi(s.prop1, &s.c2); !strings.Contains(a, "assegnato al componente ACME-C2") {
		t.Fatalf("sposta: %q", a)
	}
	controllaCopia(t, s.bancoWeb, "spostamento", 0)
	s.assegna(s.prop1, nil)
	if got := s.copia(s.pdf1); !strings.HasPrefix(got, "-/") {
		t.Errorf("copia dopo Togli: %q", got)
	}
	controllaCopia(t, s.bancoWeb, "togli", 0)

	// la copia scritta fuori dal servizio unico (senza evento) e' rifiutata al COMMIT (P4X10), anche verso nessuno
	for _, x := range []struct{ nome, sql string }{
		{"verso un componente", `UPDATE documento_proposta SET componente_id = $2, codice = 'ACME-C1' WHERE allegato_id = $1`},
		{"tolta", `UPDATE documento_proposta SET componente_id = NULL WHERE allegato_id = $1 AND $2::uuid IS NOT NULL`},
	} {
		if x.nome == "tolta" {
			s.assegna(s.prop1, &s.c)
		}
		prima := s.fotoAR()
		_, err := s.pool.Exec(s.ctx, x.sql, s.pdf1, s.c)
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != "P4X10" || !strings.Contains(pg.Message, "copia derivata") {
			t.Errorf("copia %s fuori dal servizio: %v", x.nome, err)
		}
		if s.fotoAR() != prima {
			t.Errorf("copia %s fuori dal servizio: qualcosa e' rimasto scritto", x.nome)
		}
	}
	s.assegna(s.prop1, nil)
	controllaCopia(t, s.bancoWeb, "scritture rifiutate", 0)

	// la pre-assegnazione legacy (P1, dato di prima del trigger) su pdf2 verso C, poi «Usa come STEP» (il legame della
	// sorgente) e il vecchio «Scarta il file» su pdf1
	testutil.CopiaLegacy(t, s.pool, `UPDATE documento_proposta SET componente_id = $2, codice = 'ACME-C1' WHERE allegato_id = $1`, s.pdf2, s.c)
	controllaCopia(t, s.bancoWeb, "pre-assegnazione P1", 1)
	if e, testo := s.op.gestoPonte(t, s.base()+"/assegna", url3(map[string]string{"proposta": s.propSt.String(), "componente": s.p.String(),
		"correggi_codice": "1"}), s.thread); e != "ok" {
		t.Fatalf("Usa come STEP: %s %q", e, testo)
	}
	controllaCopia(t, s.bancoWeb, "usa come STEP", 1)
	s.assegna(s.prop1, &s.c)
	if e, testo := s.op.gestoPonte(t, "/proposta/"+s.prop1.String()+"/scarta", url3(map[string]string{"ritorna_thread": s.thread.String()}), s.thread); e != "ok" {
		t.Fatalf("vecchio Scarta: %s %q", e, testo)
	}
	controllaCopia(t, s.bancoWeb, "vecchio scarta", 1)

	// il worker (una lettura nuova di pdf2) non tocca la copia
	if _, err := db.New(s.pool).UpsertProposta(s.ctx, db.UpsertPropostaParams{AllegatoID: s.pdf2, ThreadID: uuid.NullUUID{UUID: s.thread, Valid: true},
		TipoProposto: db.TipoDocumentoDisegno2d, Codice: pgtextC6b("ZZ-ALTRO"), Confidenza: 90, Fonte: db.FontePropostaCartiglio,
		Dettagli: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("UpsertProposta: %v", err)
	}
	if got := s.copia(s.pdf2); got != "ACME-C1/ACME-C1" {
		t.Errorf("il worker ha toccato la copia della P1: %q", got)
	}
	controllaCopia(t, s.bancoWeb, "worker", 1)

	// la presa in carico (R2 crea, partenza registrato): la P1 di pdf2 (C e' un nodo della versione 1) diventa una PB
	// legacy con l'evento conversione_legacy e la copia tolta; il legame della sorgente dello STEP resta
	bozza, v := s.creaBozza(distintabozza.PartenzaRegistrato)
	if got := s.copia(s.pdf2); !strings.HasPrefix(got, "-/") {
		t.Errorf("la copia della P1 dopo la presa in carico: %q", got)
	}
	if got := s.eventi(s.pdf2); got != "sgancia:conversione_legacy" {
		t.Errorf("il giornale della P1 convertita: %q", got)
	}
	if got := s.copia(s.step); got != "ACME-P1/ACME-P1" {
		t.Errorf("il legame della sorgente dopo la presa in carico: %q", got)
	}
	controllaCopia(t, s.bancoWeb, "presa in carico", 0)
	sn, _ := s.snapshot()
	var prov string
	for _, pb := range sn.DocumentiPreassociati {
		if pb.AllegatoID == s.pdf2 {
			prov = pb.Provenienza
		}
	}
	if prov != "legacy" {
		t.Errorf("la PB della P1 convertita: provenienza %q", prov)
	}

	// RFQ formato 3 con la bozza aperta: «Assegna» legacy di un file nuovo diventa una PB di R2, con la provenienza decisa
	// dal server (manuale, richiesta 4 della corsia file); la copia e la AR non cambiano
	pdf4, _ := s.file(s.thread, "ACME-D4.pdf", byteCasuali("x.pdf", 611, 1_500), db.OrigineAllegatoOutlook)
	prima := s.fotoAR()
	if a := s.assegna(s.proposta(pdf4), &s.c2); !strings.Contains(a, "bozza") {
		t.Fatalf("Assegna nel formato 3: %q", a)
	}
	if s.fotoAR() != prima {
		t.Error("Assegna nel formato 3 ha cambiato AR, giornale o copie")
	}
	sn, v2 := s.snapshot()
	prov = ""
	for _, pb := range sn.DocumentiPreassociati {
		if pb.AllegatoID == pdf4 {
			prov = pb.Provenienza
		}
	}
	if prov != "manuale" || v2 != v+1 {
		t.Errorf("la PB del gesto legacy nel formato 3: provenienza %q, versione %d → %d", prov, v, v2)
	}
	controllaCopia(t, s.bancoWeb, "assegna nel formato 3", 0)
	_ = bozza
}

// url3: i campi di un modulo.
func url3(m map[string]string) map[string][]string {
	out := map[string][]string{}
	for k, v := range m {
		out[k] = []string{v}
	}
	return out
}

// preparaSvincolo: la AR legacy di pdf2 su C (RFQ legacy), la presa in carico (partenza registrato), S8 passo 3 sullo STEP
// del prodotto, poi «Togli» della pagina legacy che nella RFQ formato 3 salva lo svincolo nella bozza (R2). Restituisce la
// bozza e la versione con lo svincolo.
func (s *scenaC6b) preparaSvincolo() (uuid.UUID, int) {
	s.t.Helper()
	if a := s.assegna(s.prop2, &s.c); !strings.Contains(a, "assegnato al componente ACME-C1") {
		s.t.Fatalf("AR legacy: %q", a)
	}
	bozza, v := s.creaBozza(distintabozza.PartenzaRegistrato)
	v = s.s8C6b(bozza, v)
	if a := s.assegna(s.prop2, nil); !strings.Contains(a, "preparato nella bozza") {
		s.t.Fatalf("svincolo nella bozza: %q", a)
	}
	_, v2 := s.snapshot()
	if v2 != v+1 {
		s.t.Fatalf("lo svincolo non ha scritto una versione: %d → %d", v, v2)
	}
	return bozza, v2
}

// Prova 9 (direttiva 35) con svincolo_gia_chiuso: lo svincolo R2, poi lo scarto con «scollega» (il gesto composto di
// D-ASS-03 [U]), poi R5: la AR resta come l'ha chiusa lo scarto, nessun evento nuovo, la conferma passa.
func TestV1D39Prova9SvincoloGiaChiusoAllaR5(t *testing.T) {
	s := nuovaScenaC6b(t)
	bozza, v := s.preparaSvincolo()
	r := s.r4C6b(bozza, v)
	if len(r.Blocchi) != 0 || svincoloDi(r, s.pdf2) != associazioni.EsitoApplicabile {
		t.Fatalf("R4 con lo svincolo: %+v %+v", r.Blocchi, r.Svincoli)
	}
	firmaPrima := r.FirmaAnteprima

	// «Scollega e scarta»: la AR si chiude con scollega_file, il file e' scartato, la bozza non cambia
	resp, _ := s.gesto(s.thread, s.pdf2, "scarta", "doppione", map[string]any{"scollega": true})
	if resp.stato != http.StatusOK {
		t.Fatalf("Scollega e scarta: %d %s", resp.stato, resp.corpo)
	}
	if got := s.arRiga(s.pdf2); got != "chiusa:scollega_file:senza_conferma #assegna:legacy sgancia:ponte scarta:ponte" {
		t.Errorf("AR dopo lo scarto: %q", got)
	}
	if got := s.statoDB(s.thread, s.pdf2); got != "scartato" {
		t.Errorf("occorrenza: %q", got)
	}
	if _, v2 := s.snapshot(); v2 != v {
		t.Errorf("lo scarto ha cambiato la bozza: v%d → v%d", v, v2)
	}
	controllaCopia(t, s.bancoWeb, "scarto", 0)

	// R5 con la firma di prima dello scarto: superata, niente scritto
	prima := fotoDelDatabase(t, s.bancoWeb)
	respR5, testo := s.ad.chiediJSON(http.MethodPost, "/thread/"+s.thread.String()+"/motore-a/bom/conferma", s.corpoR5C6b(bozza, v, firmaPrima))
	if respR5.StatusCode != http.StatusConflict || !strings.Contains(testo, `"codice":"ANTEPRIMA_SUPERATA"`) {
		t.Errorf("R5 con la firma di prima dello scarto: %d %s", respR5.StatusCode, testo)
	}
	if d := differenze(prima, fotoDelDatabase(t, s.bancoWeb)); d != "" {
		t.Errorf("R5 rifiutata ha scritto in: %s", d)
	}

	// R4 di adesso: lo svincolo e' gia' chiuso; R5 conferma senza toccare la AR
	r = s.r4C6b(bozza, v)
	if len(r.Blocchi) != 0 || svincoloDi(r, s.pdf2) != associazioni.EsitoSvincoloGiaChiuso {
		t.Fatalf("R4 dopo lo scarto: %+v %+v", r.Blocchi, r.Svincoli)
	}
	respR5, testo = s.ad.chiediJSON(http.MethodPost, "/thread/"+s.thread.String()+"/motore-a/bom/conferma", s.corpoR5C6b(bozza, v, r.FirmaAnteprima))
	var out distintabozza.ConfermaR5
	if respR5.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &out) != nil {
		t.Fatalf("R5 dopo lo scarto: %d %s", respR5.StatusCode, testo)
	}
	if out.SvincoliGiaChiusi != 1 || out.SvincoliApplicati != 0 || out.AssociazioniApplicate != 0 {
		t.Errorf("R5: svincoli gia' chiusi %d, applicati %d, AR applicate %d", out.SvincoliGiaChiusi, out.SvincoliApplicati, out.AssociazioniApplicate)
	}
	if got := s.arRiga(s.pdf2); got != "chiusa:scollega_file:senza_conferma #assegna:legacy sgancia:ponte scarta:ponte" {
		t.Errorf("AR dopo R5: %q", got)
	}
	if n := s.conteggio(`SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1 AND stato = 'attiva'`, s.pdf2); n != 0 {
		t.Errorf("AR attive di pdf2 dopo R5: %d", n)
	}
	controllaCopia(t, s.bancoWeb, "R5", 0)
}

// INC-G25: lo scarto e R5 in concorrenza. Le due strade prendono la riga della RFQ (BloccaThread) prima di leggere le
// AR: chi arriva secondo aspetta e poi rilegge.
func TestV1D39ScartoEConfermaInConcorrenza(t *testing.T) {
	t.Run("lo scarto tiene la RFQ, R5 aspetta", func(t *testing.T) {
		s := nuovaScenaC6b(t)
		bozza, v := s.preparaSvincolo()
		r := s.r4C6b(bozza, v)
		if len(r.Blocchi) != 0 {
			t.Fatalf("R4: %+v", r.Blocchi)
		}
		// la parte sul database dello scarto con «scollega» (filerfq: BloccaThread, poi la chiusura della AR dal servizio
		// unico con scollega_file), tenuta aperta mentre R5 parte
		tx, err := s.pool.Begin(s.ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(s.ctx) //nolint:errcheck
		q := db.New(tx)
		if _, err := q.BloccaThread(s.ctx, s.thread); err != nil {
			t.Fatal(err)
		}
		if _, err := associazioni.Scollega(s.ctx, q, associazioni.Atto{Thread: s.thread, Autore: s.utente, Chiave: uuid.New(), Nota: "doppione"},
			associazioni.Scollegamento{Allegato: s.pdf2, Motivo: associazioni.MotivoScollegaFile}); err != nil {
			t.Fatalf("scollega: %v", err)
		}
		corpo := s.corpoR5C6b(bozza, v, r.FirmaAnteprima)
		type esito struct {
			stato int
			testo string
		}
		fatto := make(chan esito, 1)
		go func() {
			resp, testo := s.ad.chiediJSON(http.MethodPost, "/thread/"+s.thread.String()+"/motore-a/bom/conferma", corpo)
			fatto <- esito{resp.StatusCode, testo}
		}()
		time.Sleep(700 * time.Millisecond)
		select {
		case e := <-fatto:
			t.Fatalf("R5 non ha aspettato la RFQ tenuta dallo scarto: %d %s", e.stato, e.testo)
		default:
		}
		if err := tx.Commit(s.ctx); err != nil {
			t.Fatal(err)
		}
		dopoScarto := fotoDelDatabase(t, s.bancoWeb)
		e := <-fatto
		if e.stato != http.StatusConflict || !strings.Contains(e.testo, `"codice":"ANTEPRIMA_SUPERATA"`) {
			t.Errorf("R5 dopo lo scarto concorrente: %d %s", e.stato, e.testo)
		}
		if d := differenze(dopoScarto, fotoDelDatabase(t, s.bancoWeb)); d != "" {
			t.Errorf("R5 superata ha scritto in: %s", d)
		}
		// la nuova anteprima: svincolo gia' chiuso, R5 passa
		r = s.r4C6b(bozza, v)
		if svincoloDi(r, s.pdf2) != associazioni.EsitoSvincoloGiaChiuso {
			t.Fatalf("R4 dopo lo scarto: %+v", r.Svincoli)
		}
		resp, testo := s.ad.chiediJSON(http.MethodPost, "/thread/"+s.thread.String()+"/motore-a/bom/conferma", s.corpoR5C6b(bozza, v, r.FirmaAnteprima))
		var out distintabozza.ConfermaR5
		if resp.StatusCode != http.StatusOK || json.Unmarshal([]byte(testo), &out) != nil || out.SvincoliGiaChiusi != 1 {
			t.Fatalf("R5 con l'anteprima nuova: %d %s", resp.StatusCode, testo)
		}
		if got := s.arRiga(s.pdf2); got != "chiusa:scollega_file:senza_conferma #assegna:legacy sgancia:ponte" {
			t.Errorf("AR alla fine: %q", got)
		}
		controllaCopia(t, s.bancoWeb, "scarto poi R5", 0)
	})

	t.Run("R5 tiene la RFQ, lo scarto aspetta", func(t *testing.T) {
		s := nuovaScenaC6b(t)
		bozza, v := s.preparaSvincolo()
		r := s.r4C6b(bozza, v)
		if len(r.Blocchi) != 0 {
			t.Fatalf("R4: %+v", r.Blocchi)
		}
		// il corpo dello scarto composto dalla pagina prima di R5 (con la D-7 della AR ancora attiva)
		corpoScarto := s.corpo(s.thread, s.pdf2, "scarta", "doppione", map[string]any{"scollega": true, "conferma_unica_copia": true})
		type esito struct {
			stato int
			corpo string
		}
		fatto := make(chan esito, 1)
		attese := 0
		raw, _ := json.Marshal(map[string]any{"firma_anteprima": r.FirmaAnteprima, "bozza": func() map[string]any {
			m := s.corpoBozzaC6b(bozza, v)
			m["chiave_idempotenza"] = uuid.New()
			return m
		}()})
		req, err := distintabozza.LeggiRichiestaR5(raw)
		if err != nil {
			t.Fatal(err)
		}
		out, err := distintabozza.Conferma3(context.Background(), s.pool, s.thread, req, distintabozza.OpzioniR5{UtenteID: s.utente, Motore: s.ws.motoreR5,
			Guasto: func(passo string) error {
				if passo != "associazioni" {
					return nil
				}
				// R5 ha chiuso la AR (svincolo_conferma) e tiene la RFQ: lo scarto parte e aspetta
				go func() {
					rs := s.chiama(s.ad, http.MethodPost, rottaFile(s.thread, s.pdf2, "scarta"), corpoScarto, nil)
					fatto <- esito{rs.stato, string(rs.corpo)}
				}()
				time.Sleep(700 * time.Millisecond)
				select {
				case e := <-fatto:
					fatto <- e
				default:
					attese++
				}
				return nil
			}})
		if err != nil {
			t.Fatalf("R5: %v", err)
		}
		if attese != 1 {
			t.Errorf("lo scarto non ha aspettato la RFQ tenuta da R5")
		}
		if out.SvincoliApplicati != 1 {
			t.Errorf("R5: svincoli applicati %d", out.SvincoliApplicati)
		}
		e := <-fatto
		// lo scarto rilegge dopo R5: le dipendenze del corpo sono superate (la D-7 non c'e' piu'), niente scritto
		if e.stato != http.StatusConflict {
			t.Errorf("lo scarto dopo R5: %d %s", e.stato, e.corpo)
		}
		if got := s.arRiga(s.pdf2); got != "chiusa:svincolo_conferma:con_conferma #assegna:legacy sgancia:conferma_bom" {
			t.Errorf("AR dopo R5: %q", got)
		}
		if got := s.statoDB(s.thread, s.pdf2); got != "" {
			t.Errorf("occorrenza dopo lo scarto superato: %q", got)
		}
		// lo scarto rifatto dalla pagina: senza AR attiva, niente da scollegare
		resp, _ := s.gesto(s.thread, s.pdf2, "scarta", "doppione", map[string]any{"conferma_unica_copia": true})
		if resp.stato != http.StatusOK {
			t.Fatalf("scarto rifatto: %d %s", resp.stato, resp.corpo)
		}
		if got := s.arRiga(s.pdf2); got != "chiusa:svincolo_conferma:con_conferma #assegna:legacy sgancia:conferma_bom scarta:ponte" {
			t.Errorf("AR dopo lo scarto rifatto: %q", got)
		}
		controllaCopia(t, s.bancoWeb, "R5 poi scarto", 0)
	})
}
