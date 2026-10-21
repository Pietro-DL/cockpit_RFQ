//go:build integrazione

// L4 — giro G39-ter, G39-03 [U] prima di R5 («la bozza appena creata deve essere annullata siccome la sorgente step
// cambia») e messaggio 43, caso 1 [U] (la bozza sbagliata si cancella, cioe' passa ad abbandonata; in fonte_step_evento
// la revoca e poi la conferma giusta; i registri delle bozze annotano revoca e riassegnamento), sul database di prova con
// registri di prova al posto del calcolo del motore. Dati sintetici: la scena di s8_db_test.go (ACM-1000 e ACM-6000 sono
// due prodotti finiti della stessa RFQ). Che cosa si prova, con i conteggi prima e dopo:
//   - S8 di un altro STEP su un prodotto la cui bozza aperta e' nata da una sorgente S8 (partenza step), prima di
//     qualunque R5 del prodotto: la bozza vecchia e' abbandonata (non riadattata con il cambio della fonte), la revoca
//     della sorgente vecchia e' registrata, la bozza nuova nasce dalla proposta (con le PB iniziali del motore), l'evento
//     riassegnazione_sorgente sta sulla versione 1 della bozza nuova, il registro ha abbandonata + aperta;
//   - la bozza vecchia con modifiche dell'utente: senza la presa d'atto (sostituisci) il rifiuto non scrive niente; con
//     la presa d'atto la bozza e' sostituita e la sua storia resta;
//   - il reinvio con la stessa chiave: riproposto, niente di nuovo; una chiave nuova sulla stessa sorgente: «gia' fatto»;
//   - lo scambio incrociato prima di R5 (ACM-1000 con lo STEP di ACM-6000, ACM-6000 con quello di ACM-1000), poi le due
//     S8 giuste: ogni prodotto finisce con la sua sorgente e la sua bozza, la storia delle due bozze sbagliate resta.
// Sul codice di 9eb44e3 queste prove falliscono: S8 faceva il cambio della fonte sulla stessa bozza (s8.go, caso (d)).

package sorgente

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/db"
)

// sostituzioneDi: il campo «sostituzione» della risposta di S8 (letto dal JSON: la prova compila anche sul codice di prima).
func sostituzioneDi(t *testing.T, es EsitoS8) map[string]any {
	t.Helper()
	b, err := json.Marshal(es)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	x, _ := m["sostituzione"].(map[string]any)
	return x
}

// documentoDello: il documento della RFQ con quello sha256.
func documentoDello(t *testing.T, p *pgxpool.Pool, sha string) uuid.UUID {
	t.Helper()
	var d uuid.UUID
	if err := p.QueryRow(context.Background(), `SELECT documento_id FROM documento WHERE sha256 = $1`, sha).Scan(&d); err != nil {
		t.Fatal(err)
	}
	return d
}

// radiceDellaBozza: la chiave del nodo del prodotto nella versione data.
func radiceDellaBozza(t *testing.T, p *pgxpool.Pool, bozza uuid.UUID, versione int, prodotto uuid.UUID) string {
	t.Helper()
	sn, _ := snapshotVersione(t, p, bozza, versione)
	for _, n := range sn.Nodi {
		if n.ComponenteID != nil && *n.ComponenteID == prodotto {
			return n.ChiaveBozza
		}
	}
	t.Fatalf("la bozza %s non ha il nodo del prodotto", bozza)
	return ""
}

// verificaSostituzione: i fatti della sostituzione della bozza vecchia (da shaVecchio) con la nuova (da shaNuovo) sul
// prodotto, con la chiave d'intento della S8 che l'ha fatta.
func verificaSostituzione(t *testing.T, p *pgxpool.Pool, s scenaS8, prodotto, vecchia uuid.UUID, versioneVecchia int, shaVecchio, shaNuovo string,
	chiave uuid.UUID, es EsitoS8) {
	t.Helper()
	ctx := context.Background()
	if es.Esito != EsitoCompletata || es.Bozza == nil || es.Bozza.BozzaID == vecchia || es.Bozza.Versione != 1 {
		t.Fatalf("la bozza nuova: %+v", es)
	}
	nuova := es.Bozza.BozzaID
	// la bozza vecchia abbandonata, non riadattata: nessuna versione dopo quella dichiarata, il motivo della chiusura dice
	// la sorgente errata e la bozza nuova
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza WHERE bozza_id = $1 AND stato = 'abbandonata' AND versione = $2
		AND motivo_chiusura LIKE 'sorgente_errata: sostituita dalla conferma S8 di ' || left($3, 12) || '%'
		AND motivo_chiusura LIKE '%' || $4::text || '%'`, vecchia, versioneVecchia, shaNuovo, nuova); n != 1 {
		t.Fatal("la bozza vecchia non e' abbandonata con il motivo della sostituzione (o e' stata riadattata)")
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione > $2`, vecchia, versioneVecchia); n != 0 {
		t.Fatalf("la bozza vecchia ha %d versioni nuove: e' stata riadattata", n)
	}
	// la revoca della sorgente vecchia, con il documento della sua conferma S8, la bozza vecchia e la chiave del passo
	docVecchio := documentoDello(t, p, shaVecchio)
	var revoca int64
	if err := p.QueryRow(ctx, `SELECT evento_id FROM fonte_step_evento WHERE prodotto_id = $1 AND ruolo = 'prodotto' AND azione = 'revoca_sorgente'
		AND sha256 = $2 AND documento_id = $3 AND bozza_id = $4 AND chiave_idempotenza = $5
		AND motivo LIKE 'sorgente_errata: sostituita dalla conferma S8 di ' || left($6, 12) || '%'`,
		prodotto, shaVecchio, docVecchio, vecchia, ChiavePasso(chiave, "2r"), shaNuovo).Scan(&revoca); err != nil {
		t.Fatalf("la revoca della sorgente vecchia: %v", err)
	}
	// l'ordine nella storia della fonte (messaggio 43 [U]): la revoca della sorgente vecchia, poi la conferma S8 della nuova,
	// poi la scelta della bozza nuova
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento c, fonte_step_evento sc
		WHERE c.prodotto_id = $1 AND c.azione = 'conferma_documento' AND c.chiave_idempotenza = $2 AND c.sha256 = $3
		  AND sc.prodotto_id = $1 AND sc.azione = 'scelta' AND sc.bozza_id = $4 AND sc.sha256 = $3
		  AND $5 < c.evento_id AND c.evento_id < sc.evento_id`, prodotto, ChiavePasso(chiave, "2"), shaNuovo, nuova, revoca); n != 1 {
		t.Fatal("la storia della fonte non e' revoca, conferma S8, scelta della bozza nuova")
	}
	st, err := confermabom.SorgenteValida(ctx, db.New(p), s.thread, prodotto, shaVecchio)
	if err != nil || st.Valida {
		t.Fatalf("la sorgente vecchia vale ancora: %+v %v", st, err)
	}
	if st, err := confermabom.SorgenteValida(ctx, db.New(p), s.thread, prodotto, shaNuovo); err != nil || !st.Valida || st.Correzione {
		t.Fatalf("la sorgente nuova: %+v %v", st, err)
	}
	// la bozza nuova: aperta, dalla sorgente nuova, nata dalla proposta (partenza step) con la ripartenza dalla vecchia
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza WHERE bozza_id = $1 AND prodotto_id = $2 AND stato = 'aperta' AND fonte_sha256 = $3 AND versione = 1`,
		nuova, prodotto, shaNuovo); n != 1 {
		t.Fatal("la bozza nuova non nasce dalla sorgente nuova")
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_evento WHERE bozza_id = $1 AND n = 1 AND azione = 'bozza_aperta' AND origine = 'server'
		AND dopo->>'partenza' = 'step' AND dopo->'da_bozza'->>'bozza_id' = $2::text`, nuova, vecchia); n != 1 {
		t.Fatal("la bozza nuova non e' la ripartenza dalla proposta della bozza vecchia")
	}
	// l'evento riassegnazione_sorgente sulla versione 1 della bozza nuova: prima la bozza abbandonata e la sorgente vecchia,
	// dopo la sorgente nuova e l'evento della revoca
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_evento WHERE bozza_id = $1 AND versione = 1 AND azione = 'riassegnazione_sorgente'
		AND origine = 'server' AND prima->>'bozza_id' = $2::text AND (prima->>'versione')::int = $3 AND prima->'sorgente'->>'sha256' = $4
		AND dopo->'sorgente'->>'sha256' = $5 AND (dopo->>'revoca_evento')::bigint = $6 AND motivo LIKE 'sorgente_errata:%'`,
		nuova, vecchia, versioneVecchia, shaVecchio, shaNuovo, revoca); n != 1 {
		t.Fatal("l'evento riassegnazione_sorgente della bozza nuova")
	}
	// i registri delle due bozze: abbandonata (con il motivo) e aperta
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_registro WHERE bozza_id = $1 AND passo = 'abbandonata' AND motivo LIKE 'sorgente_errata:%'`, vecchia); n != 1 {
		t.Fatal("il registro della bozza abbandonata")
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_registro WHERE bozza_id = $1 AND passo = 'aperta'`, nuova); n != 1 {
		t.Fatal("il registro della bozza nuova")
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza WHERE prodotto_id = $1 AND stato = 'aperta'`, prodotto); n != 1 {
		t.Fatalf("bozze aperte del prodotto: %d", n)
	}
	// la risposta lo dice
	sx := sostituzioneDi(t, es)
	if sx == nil || sx["bozza_abbandonata"] != vecchia.String() || sx["sha256_precedente"] != shaVecchio || sx["revoca_evento"] != float64(revoca) ||
		!strings.Contains(es.Testo, "sostituita") {
		t.Fatalf("la risposta non dice la sostituzione: %+v / %q", sx, es.Testo)
	}
}

func TestV1G39terS8SuUnaBozzaNataDaUnAltroStep(t *testing.T) {
	p, s := preparaS8(t)
	esegui(t, p, `UPDATE componente SET tipo = 'sciolto' WHERE componente_id = $1`, s.figlio)
	// lo STEP B confermato per errore come sorgente di ACM-1000: la bozza nasce da li' (prima di ogni R5)
	regB := registroRadice(t, s, s.stepB, s.shaB, "ACM-1000")
	esB, err := s8(t, p, s, s.stepB, corpoStep(t, s, uuid.New(), regB, nil), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regB}})
	if err != nil || esB.Bozza == nil || esB.Bozza.Versione != 1 {
		t.Fatalf("S8 di B: %+v %v", esB, err)
	}
	// lo STEP A giusto, con un figlio e la proposta del PDF del figlio (la PB iniziale del motore)
	pdf := pdfDi(t, p, s, "ACM-1150", '6')
	regA := registroConFigli(t, s, s.stepA, s.shaA, []figlioProva{{chiave: "#3", codice: "ACM-1150", qta: 4}})
	regA = conProposteG39(t, regA, map[string][]uuid.UUID{"#3": {pdf}})
	prepA := Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}}
	prima := fotoVietate(t, p)
	// senza la bozza dichiarata: 409 come oggi, niente scritto
	c0 := contiG39(t, p)
	_, err = s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, nil), prepA)
	codiceDi(t, err, distintabozza.CodiceBozzaVersione, distintabozza.MotivoAltraBozzaAperta)
	if c1 := contiG39(t, p); c1 != c0 {
		t.Fatalf("il rifiuto ha scritto: %s -> %s", c0, c1)
	}
	chiave := uuid.New()
	corpo := corpoStep(t, s, chiave, regA, map[string]any{"bozza": map[string]any{"bozza_id": esB.Bozza.BozzaID, "versione_attesa": 1},
		"sostituisci": map[string]any{"conferma": true}})
	es, err := s8(t, p, s, s.stepA, corpo, prepA)
	if err != nil {
		t.Fatalf("S8 di A: %v", err)
	}
	verificaSostituzione(t, p, s, s.prodotto, esB.Bozza.BozzaID, 1, s.shaB, s.shaA, chiave, es)
	// la bozza nuova ha le PB iniziali del motore della sorgente nuova
	sn, _ := snapshotVersione(t, p, es.Bozza.BozzaID, 1)
	if pb := pbDiG39(sn, pdf); pb.AllegatoID != pdf || pb.Provenienza != distintabozza.ProvenienzaMotore {
		t.Fatalf("la PB iniziale del motore nella bozza nuova: %+v", sn.DocumentiPreassociati)
	}
	if dopo := fotoVietate(t, p); dopo != prima {
		t.Fatalf("la sostituzione ha toccato working, STEP strutturale, sostituzioni o AR: %s -> %s", prima, dopo)
	}
	// il reinvio con la stessa chiave: riproposto, niente di nuovo (anche chiamando la procedura)
	c2 := contiG39(t, p)
	if es2, err := s8(t, p, s, s.stepA, corpo, prepA); err != nil || !es2.Riproposto || es2.Bozza == nil || es2.Bozza.BozzaID != es.Bozza.BozzaID ||
		sostituzioneDi(t, es2) == nil {
		t.Fatalf("reinvio: %+v %v", es2, err)
	}
	r, imp, _ := LeggiRichiestaS8(corpo)
	if es3, err := ConfermaS8(context.Background(), p, s.thread, s.stepA, s.utente, r, imp, prepA); err != nil || !es3.Riproposto {
		t.Fatalf("la procedura rifatta con la stessa chiave: %+v %v", es3, err)
	}
	if c3 := contiG39(t, p); c3 != c2 {
		t.Fatalf("il reinvio ha scritto: %s -> %s", c2, c3)
	}
	// una chiave nuova sulla stessa sorgente: passi 2 e 3 «gia' fatto», nessuna revoca in piu'
	es4, err := s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, map[string]any{"bozza": map[string]any{"bozza_id": es.Bozza.BozzaID, "versione_attesa": 1}}), prepA)
	if err != nil || es4.Passi[1].Esito != EsitoGiaFatto || es4.Passi[2].Esito != EsitoGiaFatto || es4.Bozza.BozzaID != es.Bozza.BozzaID || sostituzioneDi(t, es4) != nil {
		t.Fatalf("la stessa sorgente con una chiave nuova: %+v %v", es4, err)
	}
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'revoca_sorgente'`); n != 1 {
		t.Fatalf("revoche: %d", n)
	}
}

func TestV1G39terS8SostituisceLaBozzaModificataConLaPresaDAtto(t *testing.T) {
	p, s := preparaS8(t)
	regB := registroRadice(t, s, s.stepB, s.shaB, "ACM-1000")
	esB, err := s8(t, p, s, s.stepB, corpoStep(t, s, uuid.New(), regB, nil), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regB}})
	if err != nil || esB.Bozza == nil {
		t.Fatalf("S8 di B: %+v %v", esB, err)
	}
	// l'utente modifica la bozza nata da B: una PB manuale del PDF sul prodotto (versione 2)
	radice := radiceDellaBozza(t, p, esB.Bozza.BozzaID, 1, s.prodotto)
	if _, err := aggiungiPBG39(t, p, s, esB.Bozza.BozzaID, 1, s.pdf, radice, "", nil); err != nil {
		t.Fatalf("la modifica dell'utente: %v", err)
	}
	regA := registroRadice(t, s, s.stepA, s.shaA, "ACM-1000")
	prepA := Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}}
	bozza := map[string]any{"bozza_id": esB.Bozza.BozzaID, "versione_attesa": 2}
	// senza la presa d'atto: rifiuto prima di ogni scrittura
	c0 := contiG39(t, p)
	_, err = s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, map[string]any{"bozza": bozza}), prepA)
	if e := codiceDi(t, err, distintabozza.CodiceCorpoNonValido, MotivoPresaDAttoMancante); e.Campo != "sostituisci" {
		t.Fatalf("il rifiuto senza la presa d'atto: %+v", e)
	}
	if c1 := contiG39(t, p); c1 != c0 {
		t.Fatalf("il rifiuto ha scritto: %s -> %s", c0, c1)
	}
	// con la presa d'atto: la bozza modificata e' sostituita, la sua storia (le due versioni) resta
	chiave := uuid.New()
	es, err := s8(t, p, s, s.stepA, corpoStep(t, s, chiave, regA, map[string]any{"bozza": bozza, "sostituisci": map[string]any{"conferma": true}}), prepA)
	if err != nil {
		t.Fatalf("S8 di A con la presa d'atto: %v", err)
	}
	verificaSostituzione(t, p, s, s.prodotto, esB.Bozza.BozzaID, 2, s.shaB, s.shaA, chiave, es)
	if sx := sostituzioneDi(t, es); sx["modifiche_utente"] != true {
		t.Fatalf("la risposta non dice le modifiche dell'utente: %+v", sx)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1`, esB.Bozza.BozzaID); n != 2 {
		t.Fatalf("la storia della bozza sostituita: %d versioni", n)
	}
	sn, _ := snapshotVersione(t, p, es.Bozza.BozzaID, 1)
	if pb := pbDiG39(sn, s.pdf); pb.AllegatoID == s.pdf {
		t.Fatalf("la PB manuale della bozza sbagliata e' passata alla bozza nuova: %+v", sn.DocumentiPreassociati)
	}
}

func TestV1G39terScambioIncrociatoPrimaDiR5(t *testing.T) {
	p, s := preparaS8(t)
	// lo STEP A e' di ACM-1000, lo STEP B di ACM-6000: scambiati per errore
	regB1000 := registroRadice(t, s, s.stepB, s.shaB, "ACM-1000")
	regA1000 := registroRadice(t, s, s.stepA, s.shaA, "ACM-1000")
	regA6000, regB6000 := registroPer(t, regA1000, s.altroFinito), registroPer(t, regB1000, s.altroFinito)
	es1, err := s8(t, p, s, s.stepB, corpoStep(t, s, uuid.New(), regB1000, nil), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regB1000}})
	if err != nil || es1.Bozza == nil {
		t.Fatalf("S8 di B su ACM-1000: %+v %v", es1, err)
	}
	es2, err := s8(t, p, s, s.stepA, corpoPer(t, s, s.altroFinito, "ACM-6000", uuid.New(), regA6000, map[string]any{"motivo": "STEP del cliente per ACM-6000"}),
		Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA6000}})
	if err != nil || es2.Bozza == nil {
		t.Fatalf("S8 di A su ACM-6000: %+v %v", es2, err)
	}
	prima := fotoVietate(t, p)
	// la S8 giusta su ACM-1000: lo STEP A passa da ACM-6000 (sposta la sorgente) e la bozza di ACM-1000 nata da B e' sostituita
	k3 := uuid.New()
	es3, err := s8(t, p, s, s.stepA, corpoStep(t, s, k3, regA1000, map[string]any{"motivo": "STEP scambiati", "sposta_da": s.altroFinito,
		"bozza": map[string]any{"bozza_id": es1.Bozza.BozzaID, "versione_attesa": 1}, "sostituisci": map[string]any{"conferma": true}}),
		Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA1000}})
	if err != nil {
		t.Fatalf("S8 di A su ACM-1000: %v", err)
	}
	if es3.Spostamento == nil || es3.Spostamento.BozzaAbbandonata == nil || *es3.Spostamento.BozzaAbbandonata != es2.Bozza.BozzaID {
		t.Fatalf("lo spostamento da ACM-6000: %+v", es3.Spostamento)
	}
	verificaSostituzione(t, p, s, s.prodotto, es1.Bozza.BozzaID, 1, s.shaB, s.shaA, k3, es3)
	// la S8 giusta su ACM-6000: lo STEP B e' ancora un documento di ACM-1000, senza sorgente valida (revocata): si
	// riassocia con il motivo; ACM-6000 non ha piu' una bozza aperta, la bozza nuova nasce da B
	es4, err := s8(t, p, s, s.stepB, corpoPer(t, s, s.altroFinito, "ACM-6000", uuid.New(), regB6000, map[string]any{"motivo": "STEP scambiati",
		"sposta_da": s.prodotto}), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regB6000}})
	if err != nil || es4.Esito != EsitoCompletata || es4.Bozza == nil || es4.Spostamento == nil || es4.Spostamento.Caso != CasoDocumento {
		t.Fatalf("S8 di B su ACM-6000: %+v %v", es4, err)
	}
	// ogni prodotto con la sua sorgente valida e la sua bozza; mai la stessa sorgente su due prodotti
	for _, x := range []struct {
		prodotto uuid.UUID
		sha      string
	}{{s.prodotto, s.shaA}, {s.altroFinito, s.shaB}} {
		if n := sorgentiValide(t, p, s, x.sha); n != 1 {
			t.Fatalf("sorgenti valide di %s: %d", x.sha[:8], n)
		}
		if st, _ := confermabom.SorgenteValida(context.Background(), db.New(p), s.thread, x.prodotto, x.sha); !st.Valida {
			t.Fatalf("la sorgente giusta di %s: %+v", x.prodotto, st)
		}
		if n := conta(t, p, `SELECT count(*) FROM distinta_bozza WHERE prodotto_id = $1 AND stato = 'aperta' AND fonte_sha256 = $2`, x.prodotto, x.sha); n != 1 {
			t.Fatalf("la bozza aperta di %s dalla sua sorgente: %d", x.prodotto, n)
		}
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza WHERE stato = 'abbandonata' AND bozza_id IN ($1, $2)`, es1.Bozza.BozzaID, es2.Bozza.BozzaID); n != 2 {
		t.Fatalf("le due bozze sbagliate abbandonate: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_registro WHERE passo = 'abbandonata' AND bozza_id IN ($1, $2)`, es1.Bozza.BozzaID, es2.Bozza.BozzaID); n != 2 {
		t.Fatalf("i registri delle due bozze sbagliate: %d", n)
	}
	// la storia della fonte: le due revoche della S8 giusta su ACM-1000 (A su ACM-6000 con lo spostamento, B su ACM-1000
	// con la sostituzione), poi la traccia della riassociazione di B (il documento lascia ACM-1000: sposta.go, com'era)
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'revoca_sorgente' AND ((prodotto_id = $1 AND sha256 = $2 AND chiave_idempotenza = $5)
		OR (prodotto_id = $3 AND sha256 = $4 AND chiave_idempotenza = $6))`, s.prodotto, s.shaB, s.altroFinito, s.shaA, ChiavePasso(k3, "2r"), ChiavePasso(k3, "2s")); n != 2 {
		t.Fatalf("le revoche delle sorgenti scambiate: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'revoca_sorgente'`); n != 3 {
		t.Fatalf("revoche in tutto: %d (attese 3: le due della S8 su ACM-1000 e la traccia della riassociazione di B)", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM documento WHERE (sha256 = $1 AND componente_id = $2) OR (sha256 = $3 AND componente_id = $4)`,
		s.shaA, s.prodotto, s.shaB, s.altroFinito); n != 2 {
		t.Fatalf("i documenti con il prodotto giusto: %d", n)
	}
	if dopo := fotoVietate(t, p); dopo != prima {
		t.Fatalf("lo scambio ha toccato working, STEP strutturale, sostituzioni o AR: %s -> %s", prima, dopo)
	}
}
