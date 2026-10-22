//go:build integrazione

// L4 — R5 formato 3, la conferma atomica (conferma3*.go; EM §9.5, App. I.6; direttiva 39 §1, §2, §7, §8 [U + T-39]),
// corsia persistenza, sul database di prova:
//   - la bozza senza STEP (partenza vuota, PB manuali salvate con R2): 422 BLOCCHI con STEP_SORGENTE_NON_CONFERMATA,
//     nessuna riga cambia (working, conferme, AR, documenti, coda del NAS); lo STEP storico senza S8 non basta;
//   - la prima conferma: S8 non scrive step_strutturale_id, R5 sì (con l'evento autorizzazione); il componente nuovo e
//     l'arco nella working; testata, righe e archi certificati; le AR delle PB (anche lo STEP documentale .stp) con
//     provenienza_pb, la copia derivata uguale alla AR; lo STEP sorgente mai PB ne' AR; il reinvio riproposto senza doppie
//     conferme o AR, CHIAVE_RIUSATA, BOZZA_VERSIONE dopo la chiusura;
//   - la revisione: la working con lo STEP A resta intatta dopo S8 di B; R5 promuove B e sostituisce A dopo, la storia
//     resta (la conferma 1 tiene A); un errore iniettato fra promozione, sostituzione, testata, AR e conferma non lascia
//     niente; l'ordine rovesciato (testata prima della working) si ferma con P4X01;
//   - la revoca di B prima di R5 (anche in concorrenza: la revoca tiene il lucchetto mentre R5 aspetta): 422 senza effetti;
//   - il caso a zero figli: S8 valido con zero figli consente R5, criterio nessuno, niente mono_pezzo dedotto; con la dichiarazione
//     mono_pezzo la conferma e' mono_pezzo sullo STEP promosso;
//   - Prova 7 della direttiva 35: una AR legacy verso un altro componente con lo svincolo nella bozza: sostituita, mai in
//     silenzio; Prove 5 e 10: lo spostamento di un sottoassieme in una seconda conferma e la storia delle due conferme
//     ricostruita dal database;
//   - la ricodifica (Prova 4) e le altre parti del blocco «R5 completo» stanno in c39_r5_ricodifica_db_test.go e
//     c39_r5_completo_db_test.go.
// Dati sintetici (ACME, codici ACM-xxxx).

package distintabozza

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/platform/db"
)

// tabelleR5: le tabelle che R5 scrive o non deve scrivere: una fotografia del contenuto (md5 di tutte le righe).
var tabelleR5 = []string{"componente", "componente_relazione", "conferma_strutturale", "conferma_strutturale_componente",
	"conferma_strutturale_arco", "conferma_strutturale_non_incluso", "file_rfq_associazione", "associazione_evento", "documento",
	"documento_proposta", "componente_proposta", "relazione_proposta", "fonte_step_evento", "distinta_bozza", "esito_intento",
	"f8_stato", "job", "working_contatore"}

func fotoR5(t *testing.T, p *pgxpool.Pool) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, tab := range tabelleR5 {
		var h *string
		if err := p.QueryRow(context.Background(), `SELECT md5(string_agg(x::text, '|' ORDER BY x::text)) FROM `+tab+` x`).Scan(&h); err != nil {
			t.Fatalf("%s: %v", tab, err)
		}
		if h == nil {
			out[tab] = "-"
		} else {
			out[tab] = *h
		}
	}
	return out
}

func diffFoto(a, b map[string]string) []string {
	var out []string
	for _, tab := range tabelleR5 {
		if a[tab] != b[tab] {
			out = append(out, tab)
		}
	}
	return out
}

// s8DB: il documento cad_3d dello STEP del prodotto e, con s8, la conferma della sorgente di S8 (conferma_documento con la
// chiave del passo 2, k2); senza, solo il documento (lo STEP storico, D-AR04 (iii)).
func s8DB(t *testing.T, p *pgxpool.Pool, s scenaDB, prodotto, allegato uuid.UUID, sha, codice string, s8 bool) uuid.UUID {
	t.Helper()
	var doc uuid.UUID
	if err := p.QueryRow(context.Background(), `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256,
		path_relativo, confermato_da) VALUES ($1, $2, 'cad_3d', $3::text, $3::text || '_' || left($4::text, 4) || '.stp', 'stp', $4::text,
		'3D/' || $3::text || '_' || left($4::text, 4) || '.stp', $5)
		RETURNING documento_id`, s.thread, prodotto, codice, sha, s.utente).Scan(&doc); err != nil {
		t.Fatal(err)
	}
	if s8 {
		esegui3(t, p, `INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, documento_id, chiave_idempotenza, autore_id)
			VALUES ($1, $2, 'prodotto', 'conferma_documento', $3, $4, $5, gen_random_uuid(), $6)`, s.thread, prodotto, allegato, sha, doc, s.utente)
	}
	return doc
}

// stepDB: un'occorrenza STEP della RFQ (per la sorgente o per un documento), senza proposta.
func stepDB(t *testing.T, p *pgxpool.Pool, thread uuid.UUID, nome string, sha byte) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var msg uuid.UUID
	if err := p.QueryRow(ctx, `SELECT messaggio_id FROM messaggio WHERE thread_id = $1 ORDER BY data_evento LIMIT 1`, thread).Scan(&msg); err != nil {
		t.Fatal(err)
	}
	a := uuid.New()
	esegui3(t, p, `INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il, natura)
		VALUES ($1, $2, (SELECT COALESCE(max(indice), 0) + 1 FROM allegato WHERE messaggio_id = $2), $3, 'stp', repeat($4, 64), now(), 'file')`,
		a, msg, nome, string(sha))
	return a
}

// stpDocumentaleDB: uno STEP di un particolare (documento cad_3d proposto con il suo codice), da preassociare come un PDF.
func stpDocumentaleDB(t *testing.T, p *pgxpool.Pool, thread uuid.UUID, nome, codice string, sha byte) uuid.UUID {
	t.Helper()
	a := stepDB(t, p, thread, nome, sha)
	esegui3(t, p, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte)
		VALUES ($1, $2, 'cad_3d', $3, 80, 'nome_file')`, a, thread, codice)
	return a
}

// richiestaR5: il corpo di R5 dalla versione salvata, con la firma e la chiave.
func richiestaR5(t *testing.T, p *pgxpool.Pool, bozza uuid.UUID, versione int, firma string, chiave uuid.UUID, presa []uuid.UUID) RichiestaR5 {
	t.Helper()
	m := corpoDellaVersione(t, p, bozza, versione)
	m["chiave_idempotenza"] = chiave
	corpo := map[string]any{"firma_anteprima": firma, "bozza": m}
	if presa != nil {
		corpo["presa_d_atto_in_analisi"] = presa
	}
	raw, err := json.Marshal(corpo)
	if err != nil {
		t.Fatal(err)
	}
	r, err := LeggiRichiestaR5(raw)
	if err != nil {
		t.Fatalf("corpo di R5: %v", err)
	}
	return r
}

func confermaR5DB(t *testing.T, p *pgxpool.Pool, s scenaDB, r RichiestaR5, op OpzioniR5) (ConfermaR5, error) {
	t.Helper()
	op.UtenteID = s.utente
	if op.Motore == nil {
		op.Motore = motoreFintoDB
	}
	return Conferma3(context.Background(), p, s.thread, r, op)
}

func rifiutoR5Di(t *testing.T, err error, codice string) *RifiutoR5 {
	t.Helper()
	var r *RifiutoR5
	if !errors.As(err, &r) || r.Err.Codice != codice {
		t.Fatalf("atteso il rifiuto %s, avuto %v", codice, err)
	}
	return r
}

// bozzaManualeDB: la partenza vuota del prodotto con un pezzo nuovo (ACM-6500, sciolto, ×2 sotto la radice) e le sue PB
// manuali salvate con R2 (versione 2). Restituisce la bozza.
func bozzaManualeDB(t *testing.T, p *pgxpool.Pool, s scenaDB, prodotto uuid.UUID, codice string, pb ...uuid.UUID) uuid.UUID {
	t.Helper()
	c, err := creaDB(t, p, s, prodotto, uuid.New(), map[string]any{"partenza": PartenzaVuota})
	if err != nil {
		t.Fatalf("crea vuota: %v", err)
	}
	m, _ := snapshotDellaVersione(t, p, c.BozzaID, 1)
	radice := m["nodi"].([]any)[0].(map[string]any)
	id := map[string]any{"emittente": radice["identita"].(map[string]any)["emittente"], "emittente_presunto": true, "codice": codice, "modo": "manuale"}
	nodo := map[string]any{"chiave_bozza": "k-man", "op": "crea", "tipo": "sciolto", "origine": "utente", "identita": id}
	arco := map[string]any{"padre": radice["chiave_bozza"], "figlio": "k-man", "qta": 2, "origine": "utente"}
	m["nodi"] = append(m["nodi"].([]any), nodo)
	m["archi"] = []any{arco}
	azioni := []Azione{{Azione: AzNodoAggiunto, ChiaveBozza: "k-man", Dopo: mustJSON(nodo), Origine: "utente"},
		{Azione: AzArcoAggiunto, ChiaveBozza: "k-man", Dopo: mustJSON(arco), Origine: "utente"}}
	var pp []any
	for _, a := range pb {
		x := map[string]any{"allegato_id": a, "chiave_bozza": "k-man"}
		pp = append(pp, x)
		azioni = append(azioni, Azione{Azione: AzFilePreassociato, ChiaveBozza: "k-man", Dopo: mustJSON(x), Origine: "utente"})
	}
	if pp == nil {
		pp = []any{}
	}
	m["documenti_preassociati"] = pp
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, c.BozzaID, prodotto, 1, uuid.New(), m, azioni); err != nil {
		t.Fatalf("R2 con il pezzo e le PB manuali: %v", err)
	}
	return c.BozzaID
}

// soloProdottoSuDB: S8 passo 3 sulla bozza manuale aperta (versione attesa): la stessa bozza con la sorgente STEP reale e il
// gruppo riservato solo_prodotto (versione +1).
func soloProdottoSuDB(t *testing.T, p *pgxpool.Pool, s scenaDB, prodotto, bozza uuid.UUID, versione int, allegato uuid.UUID) int {
	t.Helper()
	v, err := SoloProdottoSuBozzaFormato3(context.Background(), p, SoloProdottoSuBozza3{ThreadID: s.thread, UtenteID: s.utente,
		Comune:   ComuneR2{ProdottoID: prodotto, BozzaID: bozza, VersioneAttesa: versione, ChiaveSalvataggio: uuid.New(), ImprontaRichiesta: shaProva('a')},
		Sorgente: SorgenteDichiarata{AllegatoID: allegato, Gruppo: GruppoSoloProdotto}, Motivo: "lo STEP del prodotto senza struttura importabile",
		PresaDAttoAnalisi: "pronta"})
	if err != nil {
		t.Fatalf("S8 passo 3 (solo_prodotto sulla bozza manuale): %v", err)
	}
	return v.Versione
}

func stepDi(t *testing.T, p *pgxpool.Pool, componente uuid.UUID) uuid.NullUUID {
	t.Helper()
	var x uuid.NullUUID
	if err := p.QueryRow(context.Background(), `SELECT step_strutturale_id FROM componente WHERE componente_id = $1`, componente).Scan(&x); err != nil {
		t.Fatal(err)
	}
	return x
}

func sostituitoDa(t *testing.T, p *pgxpool.Pool, doc uuid.UUID) uuid.NullUUID {
	t.Helper()
	var x uuid.NullUUID
	if err := p.QueryRow(context.Background(), `SELECT sostituito_da FROM documento WHERE documento_id = $1`, doc).Scan(&x); err != nil {
		t.Fatal(err)
	}
	return x
}

// D39 §1, §8 [U]: crea vuota senza STEP → R2 salva PB manuali → R4 mostra il riepilogo e il blocco → R5 422, nessuna
// modifica a working, AR, documento o NAS. Poi lo STEP storico senza S8 (documento del prodotto senza conferma S8): la
// bozza con quella sorgente resta bloccata.
func TestV1D39R5BozzaSenzaStepDB(t *testing.T) {
	p, s := poolPulito(t)
	a := allegatoFileDB(t, p, s.thread, "ACM-6500.pdf", "ACM-6500", '5')
	bozza := bozzaManualeDB(t, p, s, s.altroFinito, "ACM-6500", a)
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, bozza, 2), s.utente)
	if g := trova(rp.Blocchi, BloccoStepSorgenteNonConfermata); len(g) != 1 || g[0].Motivo != SorgenteMotivoSenzaSorgente || len(rp.Blocchi) != 1 {
		t.Fatalf("R4: %v", codici(rp.Blocchi))
	}
	prima := fotoR5(t, p)
	_, err := confermaR5DB(t, p, s, richiestaR5(t, p, bozza, 2, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	r := rifiutoR5Di(t, err, CodiceBlocchi)
	if g := trova(r.Blocchi, BloccoStepSorgenteNonConfermata); len(g) != 1 || g[0].Motivo != SorgenteMotivoSenzaSorgente {
		t.Fatalf("422 BLOCCHI con il gate: %+v", r.Blocchi)
	}
	// anche con una firma qualunque (una chiamata HTTP diretta): il gate, mai una scorciatoia
	_, err = confermaR5DB(t, p, s, richiestaR5(t, p, bozza, 2, shaProva('7'), uuid.New(), nil), OpzioniR5{})
	rifiutoR5Di(t, err, CodiceBlocchi)
	if d := diffFoto(prima, fotoR5(t, p)); len(d) > 0 {
		t.Fatalf("R5 rifiutata ha scritto in %v", d)
	}
	if stepDi(t, p, s.altroFinito).Valid {
		t.Fatal("step_strutturale_id scritto")
	}
	// lo STEP storico (documento del prodotto, senza conferma S8): la bozza con quella sorgente resta bloccata
	doc := s8DB(t, p, s, s.altroFinito, s.allegato2, shaProva('c'), "ACM-6000", false)
	v := soloProdottoSuDB(t, p, s, s.altroFinito, bozza, 2, s.allegato2)
	rp = mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, bozza, v), s.utente)
	prima = fotoR5(t, p)
	_, err = confermaR5DB(t, p, s, richiestaR5(t, p, bozza, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	r = rifiutoR5Di(t, err, CodiceBlocchi)
	if g := trova(r.Blocchi, BloccoStepSorgenteNonConfermata); len(g) != 1 || g[0].Motivo != SorgenteMotivoNonConfermata {
		t.Fatalf("lo STEP storico non basta: %+v", r.Blocchi)
	}
	if d := diffFoto(prima, fotoR5(t, p)); len(d) > 0 || stepDi(t, p, s.altroFinito).Valid || sostituitoDa(t, p, doc).Valid {
		t.Fatalf("R5 rifiutata ha scritto in %v", d)
	}
}

// D39 §2, §8 [U + T-39]: la prima conferma. La bozza manuale riconciliata con lo STEP confermato da S8 (solo_prodotto):
// S8 non scrive step_strutturale_id, R5 sì; working, conferma e AR (anche dello STEP documentale) nella stessa transazione;
// la provenienza delle PB sulla AR; la copia derivata uguale alla AR; lo STEP sorgente mai PB ne' AR; il reinvio.
func TestV1D39R5PrimaConfermaDB(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	pdf := allegatoFileDB(t, p, s.thread, "ACM-6500.pdf", "ACM-6500", '5')
	stp := stpDocumentaleDB(t, p, s.thread, "ACM-6500.stp", "ACM-6500", '6')
	bozza := bozzaManualeDB(t, p, s, s.altroFinito, "ACM-6500", pdf, stp)
	doc := s8DB(t, p, s, s.altroFinito, s.allegato2, shaProva('c'), "ACM-6000", true)
	v := soloProdottoSuDB(t, p, s, s.altroFinito, bozza, 2, s.allegato2)
	if stepDi(t, p, s.altroFinito).Valid {
		t.Fatal("S8 non scrive step_strutturale_id")
	}
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, bozza, v), s.utente)
	if len(rp.Blocchi) != 0 || !rp.Confermabile || rp.CriterioPrevisto.Criterio != confermabom.CriterioStep {
		t.Fatalf("R4: %v %+v %+v", codici(rp.Blocchi), rp.Blocchi, rp.CriterioPrevisto)
	}
	chiave := uuid.New()
	req := richiestaR5(t, p, bozza, v, rp.FirmaAnteprima, chiave, nil)
	fPrima := fotoR5(t, p)
	out, err := confermaR5DB(t, p, s, req, OpzioniR5{})
	if err != nil {
		t.Fatalf("R5: %v", err)
	}
	// R5 non scrive documenti (una AR non e' un documento confermato) ne' lavori sul NAS (lo STEP sorgente e' gia' in coda da S8)
	for _, tab := range diffFoto(fPrima, fotoR5(t, p)) {
		if tab == "documento" || tab == "job" {
			t.Fatalf("R5 ha scritto in %s", tab)
		}
	}
	if out.NumeroConferma != 1 || !out.Fonte.Promossa || out.Fonte.StepDopo == nil || *out.Fonte.StepDopo != doc || out.Fonte.SostituzioneDi != nil ||
		len(out.IdentitaCreate) != 1 || len(out.ArchiAggiunti) != 1 || out.AssociazioniApplicate != 2 || out.CriterioVerifica != confermabom.CriterioStep ||
		out.F8Stato != "in_attesa" || out.Riproposto {
		t.Fatalf("risposta: %+v", out)
	}
	nuovo := out.IdentitaCreate[0]
	if x := stepDi(t, p, s.altroFinito); !x.Valid || x.UUID != doc {
		t.Fatalf("R5 promuove la sorgente: %v", x)
	}
	if n := conta3(t, p, `SELECT count(*) FROM fonte_step_evento WHERE prodotto_id = $1 AND azione = 'autorizzazione' AND documento_id = $2
		AND chiave_idempotenza = $3`, s.altroFinito, doc, chiave); n != 1 {
		t.Fatalf("evento autorizzazione: %d", n)
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente WHERE componente_id = $1 AND codice = 'ACM-6500' AND tipo = 'sciolto' AND archiviato_il IS NULL`, nuovo); n != 1 {
		t.Fatal("il componente nuovo")
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2 AND qta = 2`, s.altroFinito, nuovo); n != 1 {
		t.Fatal("l'arco nuovo della working")
	}
	if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale c JOIN conferma_strutturale_componente k ON k.conferma_id = c.conferma_id
		WHERE c.conferma_id = $1 AND c.formato = 3 AND c.criterio_verifica = 'step' AND c.bozza_id = $2 AND c.bozza_versione = $3
		  AND k.radice AND k.step_strutturale_id = $4 AND k.step_sha256 = $5`, out.ConfermaID, bozza, v, doc, shaProva('c')); n != 1 {
		t.Fatal("testata e radice certificata con lo STEP promosso")
	}
	if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale_arco WHERE conferma_id = $1 AND padre_id = $2 AND figlio_id = $3 AND qta = 2 AND origine = 'utente'`,
		out.ConfermaID, s.altroFinito, nuovo); n != 1 {
		t.Fatal("arco certificato")
	}
	// le AR: provenienza manuale (copiata dalla PB), conferma, nodo; la copia derivata uguale
	for _, a := range []uuid.UUID{pdf, stp} {
		if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1 AND componente_id = $2 AND stato = 'attiva'
			AND origine = 'conferma_bom' AND conferma_id = $3 AND chiave_bozza = 'k-man' AND provenienza_pb = 'manuale' AND importazione_id IS NULL`,
			a, nuovo, out.ConfermaID); n != 1 {
			t.Fatalf("AR di %s", a)
		}
		if n := conta3(t, p, `SELECT count(*) FROM documento_proposta WHERE allegato_id = $1 AND componente_id = $2`, a, nuovo); n != 1 {
			t.Fatalf("copia derivata di %s", a)
		}
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1`, s.allegato2); n != 0 {
		t.Fatal("lo STEP sorgente non e' mai una AR")
	}
	if n := conta3(t, p, `SELECT count(*) FROM distinta_bozza WHERE bozza_id = $1 AND stato = 'confermata'`, bozza); n != 1 {
		t.Fatal("la bozza confermata")
	}
	if n := conta3(t, p, `SELECT count(*) FROM esito_intento WHERE chiave = $1 AND gesto = 'conferma_bom_v3' AND conferma_id = $2`, chiave, out.ConfermaID); n != 1 {
		t.Fatal("intento")
	}
	st, err := confermabom.LeggiStatoRFQ(ctx, db.New(p), s.thread)
	if err != nil {
		t.Fatal(err)
	}
	if _, val, ok, err := st.ValiditaUltima(s.altroFinito); err != nil || !ok || !val.Valida {
		t.Fatalf("la conferma e' valida: %v %+v %v", ok, val, err)
	}
	// il reinvio: riproposto, nessuna doppia conferma ne' AR
	prima := fotoR5(t, p)
	out2, err := confermaR5DB(t, p, s, req, OpzioniR5{})
	if err != nil || !out2.Riproposto || out2.ConfermaID != out.ConfermaID || out2.AssociazioniApplicate != 2 {
		t.Fatalf("reinvio: %+v %v", out2, err)
	}
	if d := diffFoto(prima, fotoR5(t, p)); len(d) > 0 {
		t.Fatalf("il reinvio ha scritto in %v", d)
	}
	// la stessa chiave con un altro corpo; una chiave nuova sulla bozza chiusa
	altra := richiestaR5(t, p, bozza, v, shaProva('8'), chiave, nil)
	if _, err := confermaR5DB(t, p, s, altra, OpzioniR5{}); err == nil || err.(*Errore).Codice != CodiceChiaveRiusata {
		t.Fatalf("CHIAVE_RIUSATA: %v", err)
	}
	if _, err := confermaR5DB(t, p, s, richiestaR5(t, p, bozza, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{}); err == nil ||
		err.(*Errore).Codice != CodiceBozzaVersione || err.(*Errore).Motivo != MotivoBozzaChiusa {
		t.Fatalf("bozza chiusa: %v", err)
	}
	if d := diffFoto(prima, fotoR5(t, p)); len(d) > 0 {
		t.Fatalf("i rifiuti hanno scritto in %v", d)
	}
}

// primaConfermaDB: la prima conferma dell'altro finito con lo STEP A (allegato2, sha c…) e il pezzo ACM-6500 ×2 con un
// PDF preassociato; restituisce il documento A, il componente nuovo e la conferma.
func primaConfermaDB(t *testing.T, p *pgxpool.Pool, s scenaDB) (uuid.UUID, uuid.UUID, ConfermaR5) {
	t.Helper()
	pdf := allegatoFileDB(t, p, s.thread, "ACM-6500.pdf", "ACM-6500", '5')
	bozza := bozzaManualeDB(t, p, s, s.altroFinito, "ACM-6500", pdf)
	docA := s8DB(t, p, s, s.altroFinito, s.allegato2, shaProva('c'), "ACM-6000", true)
	v := soloProdottoSuDB(t, p, s, s.altroFinito, bozza, 2, s.allegato2)
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, bozza, v), s.utente)
	out, err := confermaR5DB(t, p, s, richiestaR5(t, p, bozza, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("prima conferma: %v", err)
	}
	return docA, out.IdentitaCreate[0], out
}

// revisioneDB: lo STEP B del prodotto confermato da S8 e la bozza della revisione: solo_prodotto su B (versione 1, sola
// radice), poi R2 rimette il pezzo registrato con il suo arco (versione 2). Restituisce B, la bozza e la versione.
func revisioneDB(t *testing.T, p *pgxpool.Pool, s scenaDB, pezzo uuid.UUID) (uuid.UUID, uuid.UUID, int) {
	t.Helper()
	allB := stepDB(t, p, s.thread, "ACM-6000_B.stp", '9')
	docB := s8DB(t, p, s, s.altroFinito, allB, shaProva('9'), "ACM-6000", true)
	c, err := CreaFormato3(context.Background(), p, Creazione3{ThreadID: s.thread, UtenteID: s.utente,
		Comune: ComuneR2{ProdottoID: s.altroFinito, ChiaveSalvataggio: uuid.New(), ImprontaRichiesta: shaProva('a')},
		Crea: CreaR2{Partenza: PartenzaSoloProdotto, Sorgente: &SorgenteDichiarata{AllegatoID: allB, Gruppo: GruppoSoloProdotto},
			Motivo: "revisione dello STEP del prodotto", PresaDAttoAnalisi: "pronta"}})
	if err != nil {
		t.Fatalf("bozza della revisione: %v", err)
	}
	m, _ := snapshotDellaVersione(t, p, c.BozzaID, 1)
	radice := m["nodi"].([]any)[0].(map[string]any)
	em := radice["identita"].(map[string]any)["emittente"]
	k := ChiaveDelComponente(pezzo)
	nodo := map[string]any{"chiave_bozza": k, "componente_id": pezzo, "op": "riusa", "tipo": "sciolto", "origine": "registrato",
		"identita": map[string]any{"emittente": em, "emittente_presunto": true, "codice": "ACM-6500", "modo": "registrato"}}
	arco := map[string]any{"padre": radice["chiave_bozza"], "figlio": k, "qta": 2, "origine": "registrato"}
	m["nodi"] = append(m["nodi"].([]any), nodo)
	m["archi"] = []any{arco}
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, c.BozzaID, s.altroFinito, 1, uuid.New(), m, []Azione{
		{Azione: AzNodoAggiunto, ChiaveBozza: k, Dopo: mustJSON(nodo), Origine: "utente"},
		{Azione: AzArcoAggiunto, ChiaveBozza: k, Dopo: mustJSON(arco), Origine: "utente"}}); err != nil {
		t.Fatalf("R2 della revisione: %v", err)
	}
	return docB, c.BozzaID, 2
}

// D39 §2, §7, §8 [U + T-39]: la revisione. S8 di B non tocca la working (A resta lo STEP strutturale e non sostituito); un
// errore iniettato fra promozione, sostituzione, testata, AR e conferma (e dopo) non lascia niente; l'ordine rovesciato si
// ferma; R5 promuove B e sostituisce A dopo, la storia di A resta nella conferma 1.
func TestV1D39R5RevisioneDB(t *testing.T) {
	p, s := poolPulito(t)
	docA, pezzo, c1 := primaConfermaDB(t, p, s)
	docB, bozza, v := revisioneDB(t, p, s, pezzo)
	if x := stepDi(t, p, s.altroFinito); !x.Valid || x.UUID != docA || sostituitoDa(t, p, docA).Valid {
		t.Fatalf("dopo S8 di B la working tiene A: %v", x)
	}
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, bozza, v), s.utente)
	if len(rp.Blocchi) != 0 || !rp.EffettoSullaFonte.Promozione || rp.EffettoSullaFonte.SostituzioneDi == nil || *rp.EffettoSullaFonte.SostituzioneDi != docA {
		t.Fatalf("R4 della revisione: %v %+v", codici(rp.Blocchi), rp.EffettoSullaFonte)
	}
	prima := fotoR5(t, p)
	guasto := errors.New("guasto iniettato")
	for _, passo := range []string{"promozione", "sostituzione", "testata", "certificati", "associazioni", "segni", "chiusura"} {
		_, err := confermaR5DB(t, p, s, richiestaR5(t, p, bozza, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{Guasto: func(x string) error {
			if x == passo {
				return guasto
			}
			return nil
		}})
		if !errors.Is(err, guasto) {
			t.Fatalf("guasto dopo %s: %v", passo, err)
		}
		if d := diffFoto(prima, fotoR5(t, p)); len(d) > 0 {
			t.Fatalf("guasto dopo %s: effetti parziali in %v", passo, d)
		}
	}
	out, err := confermaR5DB(t, p, s, richiestaR5(t, p, bozza, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("R5 della revisione: %v", err)
	}
	for _, tab := range diffFoto(prima, fotoR5(t, p)) {
		if tab == "job" {
			t.Fatal("R5 ha scritto lavori sul NAS: i byte di A e B restano")
		}
	}
	if out.NumeroConferma != 2 || !out.Fonte.Promossa || out.Fonte.SostituzioneDi == nil || *out.Fonte.SostituzioneDi != docA ||
		len(out.IdentitaCreate) != 0 || len(out.ArchiAggiunti) != 0 || len(out.ArchiTolti) != 0 {
		t.Fatalf("risposta: %+v", out)
	}
	if x := stepDi(t, p, s.altroFinito); !x.Valid || x.UUID != docB {
		t.Fatalf("R5 promuove B: %v", x)
	}
	if x := sostituitoDa(t, p, docA); !x.Valid || x.UUID != docB {
		t.Fatalf("A sostituito da B: %v", x)
	}
	// la storia: la conferma 1 tiene A, la 2 B; tutte e due restano
	for _, c := range []struct {
		conferma uuid.UUID
		doc      uuid.UUID
		sha      string
	}{{c1.ConfermaID, docA, shaProva('c')}, {out.ConfermaID, docB, shaProva('9')}} {
		if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale_componente WHERE conferma_id = $1 AND radice AND step_strutturale_id = $2 AND step_sha256 = $3`,
			c.conferma, c.doc, c.sha); n != 1 {
			t.Fatalf("radice certificata della conferma %s", c.conferma)
		}
	}
	if n := conta3(t, p, `SELECT count(*) FROM documento WHERE documento_id IN ($1, $2)`, docA, docB); n != 2 {
		t.Fatal("tutti e due i documenti restano")
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE componente_id = $1 AND stato = 'attiva' AND conferma_id = $2`, pezzo, c1.ConfermaID); n != 1 {
		t.Fatal("la AR della conferma 1 resta attiva con la sua conferma d'origine")
	}
}

// L'ordine (D39 §7): la testata prima della working si ferma con P4X01 (working_conta) e non lascia niente. Si prova con
// l'interno di R5 (OpzioniR5.invertiTestata, solo per le prove) sul caso a zero figli, dove l'unica scrittura della working
// e' la promozione.
func TestV1D39R5OrdineRovesciatoDB(t *testing.T) {
	p, s := poolPulito(t)
	c, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaVuota})
	if err != nil {
		t.Fatal(err)
	}
	s8DB(t, p, s, s.altroFinito, s.allegato2, shaProva('c'), "ACM-6000", true)
	v := soloProdottoSuDB(t, p, s, s.altroFinito, c.BozzaID, 1, s.allegato2)
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, v), s.utente)
	prima := fotoR5(t, p)
	_, err = confermaR5DB(t, p, s, richiestaR5(t, p, c.BozzaID, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{invertiTestata: true})
	r := rifiutoR5Di(t, err, CodiceControlloFinale)
	if r.Err.Motivo != "P4X01" || !r.NonRegistrato {
		t.Fatalf("ordine rovesciato: %+v", r.Err)
	}
	if d := diffFoto(prima, fotoR5(t, p)); len(d) > 0 {
		t.Fatalf("ordine rovesciato: effetti in %v", d)
	}
}

// D39 §1, §8 (il caso a zero figli): S8 valido con zero figli consente R5 se gli altri controlli passano, con il criterio nessuno e
// niente mono_pezzo dedotto; con la dichiarazione mono_pezzo la conferma e' mono_pezzo sullo STEP promosso da R5 (la 0022
// vuole lo STEP sulla radice certificata al COMMIT).
func TestV1D39R5ZeroFigliDB(t *testing.T) {
	p, s := poolPulito(t)
	c, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaVuota})
	if err != nil {
		t.Fatal(err)
	}
	doc := s8DB(t, p, s, s.altroFinito, s.allegato2, shaProva('c'), "ACM-6000", true)
	v := soloProdottoSuDB(t, p, s, s.altroFinito, c.BozzaID, 1, s.allegato2)
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, v), s.utente)
	if len(rp.Blocchi) != 0 || rp.CriterioPrevisto.Criterio != "nessuno" {
		t.Fatalf("R4: %v %+v", codici(rp.Blocchi), rp.CriterioPrevisto)
	}
	out, err := confermaR5DB(t, p, s, richiestaR5(t, p, c.BozzaID, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("R5 a zero figli: %v", err)
	}
	if out.CriterioVerifica != "nessuno" || out.MonoPezzo || !out.Fonte.Promossa {
		t.Fatalf("niente dedotto: %+v", out)
	}
	if x := stepDi(t, p, s.altroFinito); !x.Valid || x.UUID != doc {
		t.Fatal("promozione")
	}
	// la dichiarazione mono_pezzo in una seconda conferma (da-conferma-1), senza il motivo dell'utente (messaggio 46 [U])
	c2, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaConferma})
	if err != nil {
		t.Fatalf("da-conferma-1: %v", err)
	}
	m, _ := snapshotDellaVersione(t, p, c2.BozzaID, 1)
	d := map[string]any{"id": "d-mono", "tipo": "mono_pezzo"}
	m["decisioni"] = append(m["decisioni"].([]any), d)
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, c2.BozzaID, s.altroFinito, 1, uuid.New(), m, []Azione{{Azione: AzDecisioneAggiunta, Dopo: mustJSON(d), Origine: "utente"}}); err != nil {
		t.Fatalf("R2 mono_pezzo: %v", err)
	}
	rp = mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c2.BozzaID, 2), s.utente)
	if len(rp.Blocchi) != 0 || rp.CriterioPrevisto.Criterio != confermabom.CriterioMonoPezzo {
		t.Fatalf("R4 mono_pezzo: %v %+v", codici(rp.Blocchi), rp.CriterioPrevisto)
	}
	out, err = confermaR5DB(t, p, s, richiestaR5(t, p, c2.BozzaID, 2, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("R5 mono_pezzo: %v", err)
	}
	if !out.MonoPezzo || out.CriterioVerifica != confermabom.CriterioMonoPezzo || out.NumeroConferma != 2 || out.Fonte.Promossa {
		t.Fatalf("mono_pezzo: %+v", out)
	}
	// ck_conferma_mono_pezzo vuole il motivo: la traccia del gesto scritta dal server
	if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale WHERE prodotto_id = $1 AND numero = 2 AND mono_pezzo AND mono_pezzo_motivo = $2`, s.altroFinito,
		"gesto di DBZ da «Dichiara mono_pezzo»: il prodotto è un pezzo unico: dichiarazione mono_pezzo della bozza, registrata con la conferma n. 2"); n != 1 {
		t.Fatalf("il motivo della dichiarazione mono_pezzo senza il motivo dell'utente: %d", n)
	}
	for _, x := range rp.VerificaPrevista.Motivi {
		if x == confermabom.MotivoMonoPezzoSenzaMotivo {
			t.Fatalf("R4: la dichiarazione della bozza trattata come senza motivo: %+v", rp.VerificaPrevista)
		}
	}
}

// D39 §2, §8: la revoca di B dopo R4 → R5 rifiuta senza effetti; e in concorrenza: la revoca tiene il lucchetto della RFQ
// mentre R5 aspetta, poi R5 la vede (S si apre dopo il lucchetto) e rifiuta.
func TestV1D39R5RevocaDB(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	_, pezzo, _ := primaConfermaDB(t, p, s)
	docB, bozza, v := revisioneDB(t, p, s, pezzo)
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, bozza, v), s.utente)
	if len(rp.Blocchi) != 0 {
		t.Fatalf("R4: %v", codici(rp.Blocchi))
	}
	revoca := func(q *db.Queries) {
		allB := uuidDaSQL(t, p, `SELECT allegato_id FROM fonte_step_evento WHERE documento_id = '`+docB.String()+`' LIMIT 1`)
		if _, err := q.InsertFonteStepEvento(ctx, db.InsertFonteStepEventoParams{ThreadID: s.thread, ProdottoID: s.altroFinito, Ruolo: "prodotto",
			Azione: "revoca_sorgente", AllegatoID: allB, Sha256: shaProva('9'), DocumentoID: uuid.NullUUID{UUID: docB, Valid: true},
			Motivo: testoNullo("STEP scelto per errore"), ChiaveIdempotenza: uuid.NullUUID{UUID: uuid.New(), Valid: true}, AutoreID: s.utente}); err != nil {
			t.Fatal(err)
		}
	}
	// in concorrenza: la revoca prende il lucchetto e scrive, R5 parte e aspetta, la revoca fa COMMIT
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	q := db.New(tx)
	if _, err := q.BloccaThread(ctx, s.thread); err != nil {
		t.Fatal(err)
	}
	revoca(q)
	prima := fotoR5(t, p) // la revoca non e' ancora visibile
	fatto := make(chan error, 1)
	go func() {
		_, err := confermaR5DB(t, p, s, richiestaR5(t, p, bozza, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
		fatto <- err
	}()
	time.Sleep(500 * time.Millisecond)
	select {
	case err := <-fatto:
		t.Fatalf("R5 non ha aspettato il lucchetto della revoca: %v", err)
	default:
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	err = <-fatto
	r := rifiutoR5Di(t, err, CodiceBlocchi)
	if g := trova(r.Blocchi, BloccoStepSorgenteNonConfermata); len(g) != 1 || g[0].Motivo != SorgenteMotivoRevocata {
		t.Fatalf("revoca in concorrenza: %+v", r.Blocchi)
	}
	dopo := fotoR5(t, p)
	if d := diffFoto(prima, dopo); len(d) != 1 || d[0] != "fonte_step_evento" {
		t.Fatalf("solo la revoca e' scritta: %v", d)
	}
	if x := stepDi(t, p, s.altroFinito); x.UUID == docB {
		t.Fatal("B promosso dopo la revoca")
	}
	// la revoca dopo R4 (senza concorrenza): lo stesso rifiuto, niente scritto
	_, err = confermaR5DB(t, p, s, richiestaR5(t, p, bozza, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	rifiutoR5Di(t, err, CodiceBlocchi)
	if d := diffFoto(dopo, fotoR5(t, p)); len(d) > 0 {
		t.Fatalf("R5 dopo la revoca ha scritto in %v", d)
	}
}

func uuidDaSQL(t *testing.T, p *pgxpool.Pool, sql string) uuid.UUID {
	t.Helper()
	var u uuid.UUID
	if err := p.QueryRow(context.Background(), sql).Scan(&u); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return u
}

// Prova 7 (dir. 35) nella parte di R5: una AR legacy (gesto legacy, prima che la RFQ diventi formato 3) del PDF sul prodotto;
// la bozza mette il PDF sul pezzo nuovo con lo svincolo della AR legacy: R5 sostituisce, mai in silenzio (la riga legacy
// chiusa come sostituita, una sola AR attiva, la copia segue).
func TestV1D39R5Prova7DB(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	pdf := allegatoFileDB(t, p, s.thread, "ACM-6500.pdf", "ACM-6500", '5')
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.New(tx).BloccaThread(ctx, s.thread); err != nil {
		t.Fatal(err)
	}
	if _, err := associazioni.RegistraLegacy(ctx, db.New(tx), associazioni.Atto{Thread: s.thread, Autore: s.utente, Chiave: uuid.New()}, pdf, s.altroFinito); err != nil {
		t.Fatalf("AR legacy: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	bozza := bozzaManualeDB(t, p, s, s.altroFinito, "ACM-6500", pdf)
	s8DB(t, p, s, s.altroFinito, s.allegato2, shaProva('c'), "ACM-6000", true)
	v := soloProdottoSuDB(t, p, s, s.altroFinito, bozza, 2, s.allegato2)
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, bozza, v), s.utente)
	if b := trova(rp.Blocchi, BloccoPreassociazioneNonApplicab); len(b) != 1 || b[0].Motivo != associazioni.MotivoGiaAssociataAltrove {
		t.Fatalf("R4 senza lo svincolo: %v", codici(rp.Blocchi))
	}
	prima := fotoR5(t, p)
	_, err = confermaR5DB(t, p, s, richiestaR5(t, p, bozza, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	rifiutoR5Di(t, err, CodiceBlocchi)
	if d := diffFoto(prima, fotoR5(t, p)); len(d) > 0 {
		t.Fatalf("R5 senza lo svincolo ha scritto in %v", d)
	}
	// lo svincolo della AR legacy nella bozza (versione v+1)
	m, _ := snapshotDellaVersione(t, p, bozza, v)
	d := map[string]any{"id": "d-sv", "tipo": "scollega_documento", "allegato_id": pdf, "componente_id": s.altroFinito}
	m["decisioni"] = append(m["decisioni"].([]any), d)
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, bozza, s.altroFinito, v, uuid.New(), m, []Azione{{Azione: AzDecisioneAggiunta, Dopo: mustJSON(d), Origine: "utente"}}); err != nil {
		t.Fatalf("R2 svincolo: %v", err)
	}
	rp = mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, bozza, v+1), s.utente)
	if len(rp.Blocchi) != 0 || rp.DocumentiPreassociati[0].Esito != associazioni.EsitoSostituzione {
		t.Fatalf("R4 con lo svincolo: %v %+v", codici(rp.Blocchi), rp.DocumentiPreassociati)
	}
	out, err := confermaR5DB(t, p, s, richiestaR5(t, p, bozza, v+1, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("R5 con la sostituzione: %v", err)
	}
	nuovo := out.IdentitaCreate[0]
	if out.AssociazioniApplicate != 1 || out.SvincoliApplicati != 1 {
		t.Fatalf("risposta: %+v", out)
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1 AND stato = 'attiva'`, pdf); n != 1 {
		t.Fatal("una sola AR attiva")
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1 AND componente_id = $2 AND origine = 'gesto_legacy'
		AND stato = 'chiusa' AND motivo_chiusura = 'sostituita'`, pdf, s.altroFinito); n != 1 {
		t.Fatal("la AR legacy chiusa come sostituita")
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1 AND componente_id = $2 AND stato = 'attiva' AND sostituisce_id IS NOT NULL`,
		pdf, nuovo); n != 1 {
		t.Fatal("la AR nuova sostituisce la legacy")
	}
	if n := conta3(t, p, `SELECT count(*) FROM documento_proposta WHERE allegato_id = $1 AND componente_id = $2`, pdf, nuovo); n != 1 {
		t.Fatal("la copia segue la AR")
	}
}

// Prove 5 e 10 (dir. 35) nella parte di R5: una seconda conferma (da-conferma-1, stessa sorgente) sposta un particolare da un
// sottoassieme a un altro; la storia delle due conferme si ricostruisce dal database (struttura e AR di ciascuna).
func TestV1D39R5SpostamentoEStoriaDB(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	pdf := allegatoFileDB(t, p, s.thread, "ACM-6511.pdf", "ACM-6511", '5')
	c, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaVuota})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := snapshotDellaVersione(t, p, c.BozzaID, 1)
	radice := m["nodi"].([]any)[0].(map[string]any)
	em := radice["identita"].(map[string]any)["emittente"]
	r := radice["chiave_bozza"].(string)
	nodo := func(k, codice, tipo string) map[string]any {
		return map[string]any{"chiave_bozza": k, "op": "crea", "tipo": tipo, "origine": "utente",
			"identita": map[string]any{"emittente": em, "emittente_presunto": true, "codice": codice, "modo": "manuale"}}
	}
	arco := func(pa, fi string, q int) map[string]any {
		return map[string]any{"padre": pa, "figlio": fi, "qta": q, "origine": "utente"}
	}
	nn := []any{nodo("k-sa", "ACM-8100", "sottoassieme"), nodo("k-sb", "ACM-8200", "sottoassieme"), nodo("k-p1", "ACM-6511", "sciolto"),
		nodo("k-p2", "ACM-6512", "sciolto"), nodo("k-p3", "ACM-6513", "sciolto")}
	aa := []any{arco(r, "k-sa", 1), arco(r, "k-sb", 1), arco("k-sa", "k-p1", 2), arco("k-sa", "k-p2", 1), arco("k-sb", "k-p3", 4)}
	m["nodi"] = append(m["nodi"].([]any), nn...)
	m["archi"] = aa
	pb := map[string]any{"allegato_id": pdf, "chiave_bozza": "k-p1"}
	m["documenti_preassociati"] = []any{pb}
	var az []Azione
	for _, x := range nn {
		az = append(az, Azione{Azione: AzNodoAggiunto, ChiaveBozza: x.(map[string]any)["chiave_bozza"].(string), Dopo: mustJSON(x), Origine: "utente"})
	}
	for _, x := range aa {
		az = append(az, Azione{Azione: AzArcoAggiunto, ChiaveBozza: x.(map[string]any)["figlio"].(string), Dopo: mustJSON(x), Origine: "utente"})
	}
	az = append(az, Azione{Azione: AzFilePreassociato, ChiaveBozza: "k-p1", Dopo: mustJSON(pb), Origine: "utente"})
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, c.BozzaID, s.altroFinito, 1, uuid.New(), m, az); err != nil {
		t.Fatalf("R2 della struttura: %v", err)
	}
	s8DB(t, p, s, s.altroFinito, s.allegato2, shaProva('c'), "ACM-6000", true)
	v := soloProdottoSuDB(t, p, s, s.altroFinito, c.BozzaID, 2, s.allegato2)
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, v), s.utente)
	c1, err := confermaR5DB(t, p, s, richiestaR5(t, p, c.BozzaID, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("conferma 1: %v (%v)", err, codici(rp.Blocchi))
	}
	sa, sb, p1 := c1.Mappa["k-sa"], c1.Mappa["k-sb"], c1.Mappa["k-p1"]
	// la seconda conferma: P1 passa da SA a SB (lo spostamento: niente togli_arco, il figlio ha un arco nuovo)
	c2, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaConferma})
	if err != nil {
		t.Fatalf("da-conferma-1: %v", err)
	}
	m2, _ := snapshotDellaVersione(t, p, c2.BozzaID, 1)
	kp1, ksa, ksb := ChiaveDelComponente(p1), ChiaveDelComponente(sa), ChiaveDelComponente(sb)
	chiaviDi := map[string]string{}
	for _, x := range m2["nodi"].([]any) {
		n := x.(map[string]any)
		if id, ok := n["componente_id"].(string); ok {
			chiaviDi[id] = n["chiave_bozza"].(string)
		}
	}
	kp1, ksa, ksb = valoreO(chiaviDi[p1.String()], kp1), valoreO(chiaviDi[sa.String()], ksa), valoreO(chiaviDi[sb.String()], ksb)
	var archi2 []any
	var tolto, nuovoArco map[string]any
	for _, x := range m2["archi"].([]any) {
		a := x.(map[string]any)
		if a["padre"] == ksa && a["figlio"] == kp1 {
			tolto = a
			nuovoArco = map[string]any{"padre": ksb, "figlio": kp1, "qta": a["qta"], "origine": "utente"}
			archi2 = append(archi2, nuovoArco)
			continue
		}
		archi2 = append(archi2, a)
	}
	if tolto == nil {
		t.Fatalf("l'arco SA → P1 non c'e' nella bozza da-conferma-1: %v", m2["archi"])
	}
	m2["archi"] = archi2
	if _, err := sc.salva3Su(t, c2.BozzaID, s.altroFinito, 1, uuid.New(), m2, []Azione{
		{Azione: AzArcoSpostato, ChiaveBozza: kp1, Prima: mustJSON(tolto), Dopo: mustJSON(nuovoArco), Origine: "utente"}}); err != nil {
		t.Fatalf("R2 dello spostamento: %v", err)
	}
	rp = mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c2.BozzaID, 2), s.utente)
	if len(rp.Blocchi) != 0 {
		t.Fatalf("R4 dello spostamento: %v %+v", codici(rp.Blocchi), rp.Blocchi)
	}
	c2r, err := confermaR5DB(t, p, s, richiestaR5(t, p, c2.BozzaID, 2, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("conferma 2: %v", err)
	}
	if c2r.NumeroConferma != 2 || len(c2r.ArchiTolti) != 1 || len(c2r.ArchiAggiunti) != 1 || c2r.Fonte.Promossa || c2r.AssociazioniApplicate != 0 {
		t.Fatalf("risposta della seconda: %+v", c2r)
	}
	// la storia dal database: la struttura di ciascuna conferma e le sue AR
	padreDi := func(conferma uuid.UUID) uuid.UUID {
		return uuidDaSQL(t, p, `SELECT padre_id FROM conferma_strutturale_arco WHERE conferma_id = '`+conferma.String()+`' AND figlio_id = '`+p1.String()+`'`)
	}
	if padreDi(c1.ConfermaID) != sa || padreDi(c2r.ConfermaID) != sb {
		t.Fatal("la struttura delle due conferme")
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = $1 AND componente_id = $2 AND conferma_id = $3 AND stato = 'attiva'`,
		pdf, p1, c1.ConfermaID); n != 1 {
		t.Fatal("la AR nata con la conferma 1 resta, con la sua conferma d'origine")
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2`, sb, p1); n != 1 {
		t.Fatal("lo stato corrente della working")
	}
	var dec json.RawMessage
	if err := p.QueryRow(ctx, `SELECT decisioni FROM conferma_strutturale WHERE conferma_id = $1`, c2r.ConfermaID).Scan(&dec); err != nil ||
		!strings.Contains(string(dec), `"spostato"`) {
		t.Fatalf("la provenienza della modifica (registro delle decisioni): %s %v", dec, err)
	}
}

func valoreO(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Strategia A (EM §9.5 passi 9-10): un cambiamento esterno nella parte proposta fra S e il COMMIT (una proposta di
// documento riletta dal worker, che non prende il lucchetto della RFQ) rende H_T diverso da H_S: ROLLBACK e un secondo
// controllo; se il secondo passa la conferma c'e', se gli ingressi si muovono ancora 409 ingressi_in_movimento senza effetti.
func TestV1D39R5IngressiInMovimentoDB(t *testing.T) {
	p, s := poolPulito(t)
	altro := allegatoFileDB(t, p, s.thread, "ACM-9900.pdf", "ACM-9900", '4')
	c, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaVuota})
	if err != nil {
		t.Fatal(err)
	}
	s8DB(t, p, s, s.altroFinito, s.allegato2, shaProva('c'), "ACM-6000", true)
	v := soloProdottoSuDB(t, p, s, s.altroFinito, c.BozzaID, 1, s.allegato2)
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, v), s.utente)
	giri := 0
	muovi := func(sempre bool) func() {
		return func() {
			giri++
			if sempre || giri == 1 {
				esegui3(t, p, `UPDATE documento_proposta SET codice = codice || 'X' WHERE allegato_id = $1`, altro)
			}
		}
	}
	prima := fotoR5(t, p)
	_, err = confermaR5DB(t, p, s, richiestaR5(t, p, c.BozzaID, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{DopoS: muovi(true)})
	r := rifiutoR5Di(t, err, CodiceAnteprimaSuperata)
	if r.Err.Motivo != MotivoIngressiInMovimento || giri != 2 {
		t.Fatalf("ingressi in movimento: %+v dopo %d giri", r.Err, giri)
	}
	for _, tab := range diffFoto(prima, fotoR5(t, p)) {
		if tab != "documento_proposta" {
			t.Fatalf("effetti di una conferma non riuscita in %s", tab)
		}
	}
	// il secondo controllo passa: la conferma c'e', una sola
	giri = 0
	out, err := confermaR5DB(t, p, s, richiestaR5(t, p, c.BozzaID, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{DopoS: muovi(false)})
	if err != nil || giri != 2 || out.NumeroConferma != 1 {
		t.Fatalf("secondo controllo: %+v %v (%d giri)", out, err, giri)
	}
	if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale WHERE prodotto_id = $1`, s.altroFinito); n != 1 {
		t.Fatalf("conferme: %d", n)
	}
}
