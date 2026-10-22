//go:build integrazione

// L4 — direttiva 39 §2, §3 (S8 e revoca), corsia persistenza, sul database di prova, con un registro di prova al posto
// del calcolo del motore (che la rotta fa prima della transazione). Dati sintetici. Che cosa si prova:
//   - i passi di S8 e i loro esiti; la chiave d'intento (riproposto, CHIAVE_RIUSATA); la ripresa dopo un passo 2 fatto e
//     un passo 3 mai arrivato; il rifiuto definitivo del passo 3 (intento parziale);
//   - S8 non scrive componente.step_strutturale_id, documento.sostituito_da, AR, working (conteggi prima/dopo);
//   - la partenza solo_prodotto senza registro (niente registro fittizio), con la presa d'atto dello stato dell'analisi;
//   - una working con lo STEP A (step_strutturale_id) e S8 sullo STEP B: A resta corrente, la bozza va su B
//     (da-conferma-1 e cambio della fonte);
//   - la revoca; la sorgente valida per R5; «RFQ formato 3» anche dopo l'abbandono.

package sorgente

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
	"promatec/cockpit/internal/platform/testutil"
)

// scenaS8: una RFQ con il cliente, la cartella, un messaggio; il prodotto ACM-1000 (finito) con un figlio ACM-1100 nella
// working, un secondo finito ACM-6000; tre STEP (A e B con una proposta di 3D, C senza struttura) e un PDF; una seconda
// RFQ con uno STEP.
type scenaS8 struct {
	utente, cliente, thread, thread2, msg uuid.UUID
	prodotto, figlio, altroFinito         uuid.UUID
	stepA, stepB, stepC, pdf, stepT2      uuid.UUID
	shaA, shaB, shaC                      string
	dir                                   string
}

func esegui(t *testing.T, p *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := p.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
}

func conta(t *testing.T, p *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := p.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return n
}

func preparaS8(t *testing.T) (*pgxpool.Pool, scenaS8) {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	s := scenaS8{utente: uuid.New(), cliente: uuid.New(), thread: uuid.New(), thread2: uuid.New(), msg: uuid.New(), prodotto: uuid.New(),
		figlio: uuid.New(), altroFinito: uuid.New(), stepA: uuid.New(), stepB: uuid.New(), stepC: uuid.New(), pdf: uuid.New(), stepT2: uuid.New(),
		dir: t.TempDir()}
	esegui(t, p, `INSERT INTO utente (utente_id, sigla, nome, ufficio, ruolo) VALUES ($1, 'S8P', 'Prova S8', 'UT', 'admin')`, s.utente)
	esegui(t, p, `INSERT INTO cliente (cliente_id, cartella_nas, ragione_sociale) VALUES ($1, 'ACMES8', 'ACME S.p.A.')`, s.cliente)
	esegui(t, p, `INSERT INTO thread_offerta (thread_id, cliente_id, canale, data_inizio, oggetto, cartella_relativa) VALUES
		($1, $3, 'outlook', now(), 'RFQ S8', 'ACMES8\WIP\RFQ S8'), ($2, $3, 'outlook', now(), 'RFQ S8 bis', 'ACMES8\WIP\RFQ S8 bis')`, s.thread, s.thread2, s.cliente)
	msg2 := uuid.New()
	for i, x := range []struct{ th, m uuid.UUID }{{s.thread, s.msg}, {s.thread2, msg2}} {
		cv := uuid.New()
		esegui(t, p, `INSERT INTO conversazione (conversazione_id, canale, chiave_esterna, thread_id, primo_messaggio_il) VALUES ($1, 'outlook', $2, $3, now())`,
			cv, fmt.Sprintf("conv-s8-%d-%s@example.com", i, x.th), x.th)
		esegui(t, p, `INSERT INTO messaggio (messaggio_id, canale, chiave_esterna, conversazione_id, thread_id, direzione, data_evento, oggetto)
			VALUES ($1, 'outlook', $2, $3, $4, 'entrata', now(), 'RFQ S8')`, x.m, fmt.Sprintf("msg-s8-%d-%s@example.com", i, x.th), cv, x.th)
	}
	allegato := func(id, msg uuid.UUID, indice int, nome, ext string) string {
		t.Helper()
		percorso := filepath.Join(s.dir, fmt.Sprintf("%d_%s", indice, nome))
		contenuto := []byte("contenuto sintetico " + nome + " " + id.String())
		if err := os.WriteFile(percorso, contenuto, 0o644); err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(contenuto)
		sha := hex.EncodeToString(h[:])
		esegui(t, p, `INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, path_staging, stato, ricevuto_il)
			VALUES ($1, $2, $3, $4, $5, 'file', 'outlook', $6, $7, $8, 'analizzato', now())`, id, msg, indice, nome, ext, len(contenuto), sha, percorso)
		return sha
	}
	s.shaA = allegato(s.stepA, s.msg, 1, "ACM-1000.stp", "stp")
	s.shaB = allegato(s.stepB, s.msg, 2, "ACM-1000_revB.stp", "stp")
	s.shaC = allegato(s.stepC, s.msg, 3, "ACM-1000_senza.stp", "stp")
	allegato(s.pdf, s.msg, 4, "ACM-1000.pdf", "pdf")
	allegato(s.stepT2, msg2, 1, "ACM-7000.stp", "stp")
	for _, a := range []uuid.UUID{s.stepA, s.stepB, s.stepC} {
		esegui(t, p, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1, $2, 'cad_3d', 'ACM-1000', 80, 'nome_file')`,
			a, s.thread)
	}
	esegui(t, p, `INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte) VALUES ($1, $2, 'disegno_2d', 'ACM-1000', 80, 'nome_file')`,
		s.pdf, s.thread)
	esegui(t, p, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES
		($1, $4, 'ACM-1000', 'finito', 'manuale', $5), ($2, $4, 'ACM-1100', 'sottoassieme', 'manuale', $5), ($3, $4, 'ACM-6000', 'finito', 'manuale', $5)`,
		s.prodotto, s.figlio, s.altroFinito, s.thread, s.utente)
	esegui(t, p, `INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ($1, $2, $3, 1, 'manuale', $4)`,
		s.thread, s.prodotto, s.figlio, s.utente)
	return p, s
}

func rawT(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := jsoncanonico.Codifica(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// registroRadice: il registro di prova del gruppo bom di uno STEP con la sola radice riconosciuta (zero figli estratti:
// il caso della direttiva 39 §1, ultimo capoverso), codice della radice dato.
func registroRadice(t *testing.T, s scenaS8, allegato uuid.UUID, sha, codice string) importazione.Registro {
	t.Helper()
	vero, falso := true, false
	rif := "nodo:" + sha + ":#1"
	cliente := distintabozza.EmittenteCliente(s.cliente)
	lettura := map[string]any{
		"chiave_lettura": allegato.String() + "/l:u:step:#1", "unita_id": "u:#1", "selettore": "nodo_step.id", "testo_unita": codice,
		"intervallo": map[string]any{"inizio": 0, "fine": len(codice)}, "letterale": codice, "emittente": cliente,
		"hash_grammatica": strings.Repeat("7", 64), "namespace": "acme", "famiglia": "num", "forma": "sei",
		"base":          map[string]any{"originale": codice, "normalizzata": codice, "segmenti": []any{}, "mancanti": []any{}, "completa": true},
		"qualificatori": map[string]any{}, "revisione": nil, "stato_forma": "letta", "funzione": "identita", "ruolo": "",
		"qualita": "piena", "motivi": []any{"forma_completa"},
	}
	radice := importazione.Voce{Voce: "n:" + allegato.String() + ":" + rif,
		Origine:  importazione.OrigineVoce{AllegatoID: allegato.String(), Sha256: sha, Chiave: "#1", Rif: rif},
		EntitaID: rawT(t, "e#1"), Foglia: rawT(t, true), RadiceDelFile: rawT(t, true), Leggibile: rawT(t, true),
		StatoMotore: importazione.StatoMotore{Raggiunto: &vero, Tolto: &falso, Deciso: &falso},
		Grezzi:      rawT(t, map[string]any{"id": "#1", "nome": codice, "formazioni": []any{}}),
		Padri:       rawT(t, []any{}), IdentitaProposta: rawT(t, lettura), CodiceProposto: codice,
		TipoProposto: rawT(t, map[string]any{"valore": "finito", "origine": "confermato"}),
	}
	c := importazione.Contenuto{
		FormatoRegistro: importazione.FormatoRegistro, VersioneImportazione: importazione.VersioneImportazione1,
		ThreadID: s.thread.String(), ProdottoID: s.prodotto.String(),
		Gruppo: importazione.Gruppo{Chiave: "bom", Tipo: rawT(t, "bom"), AllegatoID: allegato.String(), Sha256: sha, Radice: rif,
			NomeFile: rawT(t, "ACM-1000.stp"), StatoStruttura: rawT(t, "bom_di_lavoro_proposta"), RadiceDelFile: rawT(t, true),
			Compatibilita: rawT(t, "uguale"), GrafoCompleto: rawT(t, true)},
		Regole: importazione.Regole{
			Versioni: importazione.Versioni{Algoritmo: rawT(t, "a1-1"), Router: rawT(t, "r-2"), Risultato: rawT(t, "es-3"), Ancoraggio: "ancoraggio-3",
				Valutazione: rawT(t, "v-4"), Composizione: rawT(t, "gp-1"), Importazione: importazione.VersioneImportazione1,
				Canonico: jsoncanonico.Versione, Limiti: rawT(t, "lim-1")},
			GrammaticaCliente: &importazione.GrammaticaCliente{ClienteID: s.cliente.String(), Hash: strings.Repeat("7", 64)}},
		Calcolo: rawT(t, map[string]any{"voci": map[string]any{}, "impronta_indice": "i-1", "bundle_id": "b-1"}),
		Proposta: importazione.Proposta{
			Fonte: importazione.FonteProposta{AllegatoID: allegato.String(), Sha256: sha, NomeRicevuto: rawT(t, "ACM-1000.stp"),
				PercorsoRicevuto: rawT(t, "STEP/ACM-1000.stp")},
			Nodi: []importazione.Voce{radice}, Archi: []importazione.ArcoRegistro{}, File: []importazione.FileRegistro{},
			Relazioni: []importazione.Relazione{}, FuoriPerDecisione: []importazione.FuoriPerDecisione{}, Diagnostiche: rawT(t, []any{})},
		AllegatiCitati: []string{allegato.String()},
	}
	r, err := importazione.Componi(c)
	if err != nil {
		t.Fatalf("Componi: %v", err)
	}
	return r
}

// corpoStep: il corpo di S8 con la partenza step, come la pagina lo manda dopo l'anteprima.
func corpoStep(t *testing.T, s scenaS8, chiave uuid.UUID, reg importazione.Registro, extra map[string]any) []byte {
	t.Helper()
	c := map[string]any{"chiave_idempotenza": chiave, "prodotto_id": s.prodotto, "gruppo": "bom", "impronta_contenuto": reg.Contenuto.ImprontaContenuto,
		"codice_atteso": "ACM-1000"}
	for k, v := range extra {
		if v == nil {
			delete(c, k)
		} else {
			c[k] = v
		}
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// s8: la procedura come la chiama la rotta: la lettura del corpo, l'intento gia' scritto, il passo 0, la procedura.
func s8(t *testing.T, p *pgxpool.Pool, s scenaS8, allegato uuid.UUID, corpo []byte, prep Preparazione) (EsitoS8, error) {
	t.Helper()
	ctx := context.Background()
	r, imp, err := LeggiRichiestaS8(corpo)
	if err != nil {
		return EsitoS8{}, err
	}
	if gia, err := IntentoRegistrato(ctx, db.New(p), s.thread, r.ChiaveIdempotenza, imp, GestoConfermaSorgente); err != nil {
		return EsitoS8{}, err
	} else if gia != nil {
		return Riproposto(gia)
	}
	if _, err := contestoInLettura(ctx, p, s.thread, allegato, r); err != nil {
		return EsitoS8{}, err
	}
	return ConfermaS8(ctx, p, s.thread, allegato, s.utente, r, imp, prep)
}

func codiceDi(t *testing.T, err error, codice, motivo string) *distintabozza.Errore {
	t.Helper()
	var e *distintabozza.Errore
	if !errors.As(err, &e) || e.Codice != codice || (motivo != "" && e.Motivo != motivo) {
		t.Fatalf("atteso %s/%s, avuto %v (%#v)", codice, motivo, err, e)
	}
	return e
}

// fotoVietate: le righe che S8 e la revoca non toccano mai (direttiva 39 §2): lo STEP strutturale dei componenti, le
// sostituzioni dei documenti, la working, le conferme, le AR, le righe legacy delle proposte strutturali.
func fotoVietate(t *testing.T, p *pgxpool.Pool) string {
	t.Helper()
	var out []string
	for _, q := range []string{
		`SELECT count(*) FROM componente WHERE step_strutturale_id IS NOT NULL`,
		`SELECT COALESCE(sum(hashtext(componente_id::text || COALESCE(step_strutturale_id::text, '-'))), 0) FROM componente`,
		`SELECT count(*) FROM documento WHERE sostituito_da IS NOT NULL`,
		`SELECT count(*) FROM componente`, `SELECT count(*) FROM componente_relazione`,
		`SELECT count(*) FROM conferma_strutturale`, `SELECT count(*) FROM file_rfq_associazione`,
		`SELECT count(*) FROM componente_proposta`, `SELECT count(*) FROM relazione_proposta`, `SELECT count(*) FROM associazione_evento`,
	} {
		out = append(out, fmt.Sprint(conta(t, p, q)))
	}
	return strings.Join(out, "|")
}

func TestV1S8IPassiLaChiaveELaRipresa(t *testing.T) {
	p, s := preparaS8(t)
	ctx := context.Background()
	regA := registroRadice(t, s, s.stepA, s.shaA, "ACM-1000")
	prep := Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}}
	if f3, err := associazioni.RFQFormato3(ctx, db.New(p), s.thread); err != nil || f3 {
		t.Fatalf("RFQ formato 3 prima di tutto: %v %v", f3, err)
	}
	prima := fotoVietate(t, p)
	chiave := uuid.New()
	corpo := corpoStep(t, s, chiave, regA, nil)
	es, err := s8(t, p, s, s.stepA, corpo, prep)
	if err != nil {
		t.Fatalf("S8: %v", err)
	}
	if es.Esito != EsitoCompletata || es.Riproposto || len(es.Passi) != 3 || es.Passi[0].Esito != EsitoNonServe || es.Passi[1].Esito != EsitoFatto ||
		es.Passi[2].Esito != EsitoFatto || es.Bozza == nil || es.Bozza.Versione != 1 || es.Bozza.Importazione == nil || es.CopiaNas != "in_coda" {
		t.Fatalf("esito: %+v", es)
	}
	if dopo := fotoVietate(t, p); dopo != prima {
		t.Fatalf("S8 ha toccato working, STEP strutturale, sostituzioni o AR: %s -> %s", prima, dopo)
	}
	// il documento del prodotto, la riga della fonte di S8 con k2, la scelta della bozza, la copia in coda, l'intento
	if n := conta(t, p, `SELECT count(*) FROM documento WHERE thread_id = $1 AND componente_id = $2 AND tipo = 'cad_3d' AND sha256 = $3`, s.thread, s.prodotto, s.shaA); n != 1 {
		t.Fatalf("documento dello STEP A: %d", n)
	}
	k2 := ChiavePasso(chiave, "2")
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'conferma_documento' AND ruolo = 'prodotto' AND chiave_idempotenza = $1
		AND documento_id IS NOT NULL AND gruppo = 'bom' AND prodotto_id = $2`, k2, s.prodotto); n != 1 {
		t.Fatalf("riga conferma_documento di S8: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'scelta' AND bozza_id = $1 AND gruppo = 'bom'`, es.Bozza.BozzaID); n != 1 {
		t.Fatalf("riga scelta della bozza: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM job WHERE tipo = 'copia_nas'`); n != 1 {
		t.Fatalf("copie sul NAS in coda: %d (attesa 1, il solo STEP sorgente)", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM esito_intento WHERE chiave = $1 AND gesto = 'conferma_sorgente'`, chiave); n != 1 {
		t.Fatalf("intento: %d", n)
	}
	if st, err := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.prodotto, s.shaA); err != nil || !st.Valida || st.Gruppo != "bom" {
		t.Fatalf("sorgente valida per R5: %+v %v", st, err)
	}
	if f3, err := associazioni.RFQFormato3(ctx, db.New(p), s.thread); err != nil || !f3 {
		t.Fatalf("RFQ formato 3 dopo S8: %v %v", f3, err)
	}
	// una conferma di S8 piu' recente di un altro contenuto dello stesso prodotto supera A; una nuova conferma di A la
	// rende di nuovo valida (I.2 della corsia contratto, [T-39]); il caso vero passa da S8 su B, qui le righe a mano
	var docA uuid.UUID
	if err := p.QueryRow(ctx, `SELECT documento_id FROM documento WHERE sha256 = $1`, s.shaA).Scan(&docA); err != nil {
		t.Fatal(err)
	}
	riga := func(sha string) {
		esegui(t, p, `INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, documento_id, chiave_idempotenza, autore_id)
			VALUES ($1, $2, 'prodotto', 'conferma_documento', $3, $4, $5, gen_random_uuid(), $6)`, s.thread, s.prodotto, s.stepA, sha, docA, s.utente)
	}
	riga(s.shaB)
	if st, _ := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.prodotto, s.shaA); st.Valida || st.Motivo != confermabom.SorgenteSuperata {
		t.Fatalf("A superata: %+v", st)
	}
	riga(s.shaA)
	if st, _ := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.prodotto, s.shaA); !st.Valida {
		t.Fatalf("A riconfermata: %+v", st)
	}
	// il reinvio con la stessa chiave e lo stesso corpo: riproposto, niente scritto (anche chiamando la procedura)
	conti := func() string {
		return fmt.Sprint(conta(t, p, `SELECT count(*) FROM documento`), conta(t, p, `SELECT count(*) FROM fonte_step_evento`),
			conta(t, p, `SELECT count(*) FROM distinta_bozza_versione`), conta(t, p, `SELECT count(*) FROM esito_intento`), conta(t, p, `SELECT count(*) FROM job`))
	}
	c0 := conti()
	es2, err := s8(t, p, s, s.stepA, corpo, prep)
	if err != nil || !es2.Riproposto || es2.Esito != EsitoCompletata || es2.Bozza == nil || es2.Bozza.BozzaID != es.Bozza.BozzaID {
		t.Fatalf("reinvio: %+v %v", es2, err)
	}
	r, imp, _ := LeggiRichiestaS8(corpo)
	es3, err := ConfermaS8(ctx, p, s.thread, s.stepA, s.utente, r, imp, prep)
	if err != nil || !es3.Riproposto {
		t.Fatalf("la procedura rifatta con la stessa chiave: %+v %v", es3, err)
	}
	if c1 := conti(); c1 != c0 {
		t.Fatalf("il reinvio ha scritto: %s -> %s", c0, c1)
	}
	// la stessa chiave con un altro corpo: CHIAVE_RIUSATA
	_, err = s8(t, p, s, s.stepA, corpoStep(t, s, chiave, regA, map[string]any{"motivo": "un altro corpo"}), prep)
	codiceDi(t, err, distintabozza.CodiceChiaveRiusata, "")
	// una chiave nuova sullo stesso STEP con la bozza gia' su questa sorgente: passi 2 e 3 «gia' fatto»
	es4, err := s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, nil), prep)
	if err != nil || es4.Esito != EsitoCompletata || es4.Passi[1].Esito != EsitoGiaFatto || es4.Passi[2].Esito != EsitoGiaFatto || es4.Bozza.BozzaID != es.Bozza.BozzaID {
		t.Fatalf("caso (b): %+v %v", es4, err)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza WHERE prodotto_id = $1`, s.prodotto); n != 1 {
		t.Fatalf("bozze del prodotto: %d (mai una seconda bozza)", n)
	}
	if dopo := fotoVietate(t, p); dopo != prima {
		t.Fatalf("i reinvii hanno toccato la working: %s -> %s", prima, dopo)
	}
}

func TestV1S8LaRipresaEIlRifiutoDelPasso3(t *testing.T) {
	p, s := preparaS8(t)
	ctx := context.Background()
	regA := registroRadice(t, s, s.stepA, s.shaA, "ACM-1000")
	prep := Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}}
	// il passo 2 fatto, poi niente (la risposta persa, il processo fermato): lo stesso reinvio riprende dal passo 3
	chiave := uuid.New()
	corpo := corpoStep(t, s, chiave, regA, nil)
	r, imp, err := LeggiRichiestaS8(corpo)
	if err != nil {
		t.Fatal(err)
	}
	if esito, copia, sp, err := passo2(ctx, p, s.thread, s.stepA, s.utente, r); err != nil || esito != EsitoFatto || copia != "in_coda" || sp != nil {
		t.Fatalf("passo 2 da solo: %s %s %v", esito, copia, err)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza`); n != 0 {
		t.Fatalf("il passo 2 ha creato una bozza: %d", n)
	}
	es, err := s8(t, p, s, s.stepA, corpo, prep)
	if err != nil || es.Esito != EsitoCompletata || es.Passi[1].Esito != EsitoGiaFatto || es.Passi[2].Esito != EsitoFatto || es.Bozza == nil {
		t.Fatalf("ripresa: %+v %v", es, err)
	}
	if n := conta(t, p, `SELECT count(*) FROM documento WHERE sha256 = $1`, s.shaA); n != 1 {
		t.Fatalf("documenti dopo la ripresa: %d", n)
	}
	_ = imp

	// un errore temporaneo: parziale ritentabile, nessun intento
	r2, imp2, _ := LeggiRichiestaS8(corpoStep(t, s, uuid.New(), regA, nil))
	out := EsitoS8{ChiaveIdempotenza: r2.ChiaveIdempotenza}
	x, err := chiudiParziale(ctx, p, s.thread, s.utente, r2, imp2, out, PassoBozza, &pgconn.PgError{Code: "40P01"})
	if err != nil || x.Esito != EsitoParziale || !x.Ritentabile || x.Passi[0].Codice != "RIPROVA" {
		t.Fatalf("temporaneo: %+v %v", x, err)
	}
	if n := conta(t, p, `SELECT count(*) FROM esito_intento WHERE chiave = $1`, r2.ChiaveIdempotenza); n != 0 {
		t.Fatalf("intento dopo un errore temporaneo: %d", n)
	}

	// un rifiuto definitivo del passo 3 (fra il passo 2 e il passo 3 il prodotto ha una bozza manuale, che il corpo non
	// dichiara: BOZZA_VERSIONE; con la sorgente S8 valida la partenza vuota non c'e' piu', la bozza manuale e' registrato):
	// parziale, l'intento scritto, lo stesso reinvio riproposto
	p2, s2 := p, s
	abbandona(t, p2, s2)
	if _, err := distintabozza.CreaFormato3(ctx, p2, distintabozza.Creazione3{ThreadID: s2.thread, UtenteID: s2.utente,
		Comune: distintabozza.ComuneR2{ProdottoID: s2.prodotto, ChiaveSalvataggio: uuid.New(), ImprontaRichiesta: strings.Repeat("a", 64)},
		Crea:   distintabozza.CreaR2{Partenza: distintabozza.PartenzaRegistrato}}); err != nil {
		t.Fatalf("crea registrato: %v", err)
	}
	chiave3 := uuid.New()
	r3, imp3, _ := LeggiRichiestaS8(corpoStep(t, s2, chiave3, regA, nil))
	x3, err := passo3(ctx, p2, s2.thread, s2.stepA, s2.utente, r3, imp3, prep, EsitoS8{ChiaveIdempotenza: chiave3, Passi: []PassoS8{}})
	if err != nil || x3.Esito != EsitoParziale || x3.Ritentabile || len(x3.Passi) != 1 || x3.Passi[0].Codice != distintabozza.CodiceBozzaVersione {
		t.Fatalf("rifiuto del passo 3: %+v %v", x3, err)
	}
	gia, err := IntentoRegistrato(ctx, db.New(p2), s2.thread, chiave3, imp3, GestoConfermaSorgente)
	if err != nil || gia == nil {
		t.Fatalf("intento parziale: %v", err)
	}
	if y, _ := Riproposto(gia); !y.Riproposto || y.Esito != EsitoParziale {
		t.Fatalf("reinvio del parziale: %+v", y)
	}
}

// abbandona: R3 sulla bozza aperta del prodotto.
func abbandona(t *testing.T, p *pgxpool.Pool, s scenaS8) {
	t.Helper()
	ctx := context.Background()
	b, err := db.New(p).GetDistintaBozzaAperta(ctx, db.GetDistintaBozzaApertaParams{ThreadID: s.thread, ProdottoID: s.prodotto})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := distintabozza.Abbandona(ctx, p, distintabozza.Abbandono{ThreadID: s.thread, BozzaID: b.BozzaID, UtenteID: s.utente,
		VersioneAttesa: int(b.Versione), Motivo: "prova"}); err != nil {
		t.Fatalf("abbandona: %v", err)
	}
}

// creaVuota: una bozza manuale (partenza vuota, senza STEP) del prodotto, con R2.
func creaVuota(t *testing.T, p *pgxpool.Pool, s scenaS8) uuid.UUID {
	t.Helper()
	k := uuid.New()
	c, err := distintabozza.CreaFormato3(context.Background(), p, distintabozza.Creazione3{ThreadID: s.thread, UtenteID: s.utente,
		Comune: distintabozza.ComuneR2{ProdottoID: s.prodotto, ChiaveSalvataggio: k, ImprontaRichiesta: strings.Repeat("a", 64)},
		Crea:   distintabozza.CreaR2{Partenza: distintabozza.PartenzaVuota}})
	if err != nil {
		t.Fatalf("crea vuota: %v", err)
	}
	return c.BozzaID
}

func TestV1S8IRifiutiDelPasso0(t *testing.T) {
	p, s := preparaS8(t)
	ctx := context.Background()
	regA := registroRadice(t, s, s.stepA, s.shaA, "ACM-1000")
	prep := Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}}
	prima := fotoVietate(t, p)
	conti := func() string {
		return fmt.Sprint(conta(t, p, `SELECT count(*) FROM documento`), conta(t, p, `SELECT count(*) FROM fonte_step_evento`),
			conta(t, p, `SELECT count(*) FROM distinta_bozza`), conta(t, p, `SELECT count(*) FROM esito_intento`))
	}
	c0 := conti()
	// un PDF non e' uno STEP; uno STEP di un'altra RFQ; un prodotto che non e' un finito; il codice del prodotto cambiato
	_, err := s8(t, p, s, s.pdf, corpoStep(t, s, uuid.New(), regA, nil), prep)
	codiceDi(t, err, CodiceNonStep, "")
	_, err = s8(t, p, s, s.stepT2, corpoStep(t, s, uuid.New(), regA, nil), prep)
	codiceDi(t, err, distintabozza.CodiceFuoriRFQ, "")
	_, err = s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, map[string]any{"prodotto_id": s.figlio}), prep)
	codiceDi(t, err, distintabozza.CodiceCorpoNonValido, MotivoProdottoNonAmmesso)
	_, err = s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, map[string]any{"codice_atteso": "ACM-9999"}), prep)
	codiceDi(t, err, distintabozza.CodiceAnteprimaSuperata, "")
	// V-7: l'impronta del contenuto non e' quella del registro calcolato adesso
	_, err = s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, map[string]any{"impronta_contenuto": strings.Repeat("0", 64)}), prep)
	codiceDi(t, err, distintabozza.CodiceAnteprimaSuperata, "")
	// il codice della radice diverso da quello del prodotto non chiede piu' il motivo (messaggio 46 [U]):
	// TestV1M46S8LaRadiceDiversaSenzaMotivo
	// «sostituisci» e' solo una presa d'atto {conferma: true}; un altro valore e' 422
	_, err = s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, map[string]any{"sostituisci": map[string]any{"conferma": false}}), prep)
	codiceDi(t, err, distintabozza.CodiceCorpoNonValido, "")
	// bozza manuale aperta senza la bozza nel corpo: 409 BOZZA_VERSIONE altra_bozza_aperta con la bozza da dichiarare (la
	// riconciliazione vuole che l'ingegnere abbia visto l'effetto; direttiva 39 §3), niente scritto
	bm := creaVuota(t, p, s)
	c1 := conti()
	_, err = s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, nil), prep)
	if e := codiceDi(t, err, distintabozza.CodiceBozzaVersione, distintabozza.MotivoAltraBozzaAperta); e.Conflitto == nil || e.Conflitto.BozzaID != bm {
		t.Fatalf("il conflitto della bozza manuale: %+v", e)
	}
	if c2 := conti(); c2 != c1 {
		t.Fatalf("il rifiuto della bozza manuale ha scritto: %s -> %s", c1, c2)
	}
	// il caso (g): lo stesso contenuto e' il documento di un altro finito, senza S8: dal giro G39 (G39-03 [U], RV3-32 [U])
	// si riassocia con «Riassocia a ‹P›» (sposta_da; messaggio 46 [U]: senza motivo); senza, STEP_SORGENTE_DI_ALTRO_PRODOTTO
	// riassociazione con l'altro prodotto, niente scritto (le riassociazioni e gli spostamenti veri sono in
	// s8_sposta_db_test.go)
	esegui(t, p, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, $2, 'cad_3d', 'ACM-6000', 'ACM-1000_revB.stp', 'stp', $3, '3D/ACM-6000.stp', $4)`, s.thread, s.altroFinito, s.shaB, s.utente)
	regB := registroRadice(t, s, s.stepB, s.shaB, "ACM-1000")
	_, err = s8(t, p, s, s.stepB, corpoStep(t, s, uuid.New(), regB, nil), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regB}})
	if e := codiceDi(t, err, CodiceStepSorgenteDiAltroProdotto, MotivoRiassociazione); e.AltroProdotto == nil || *e.AltroProdotto != s.altroFinito {
		t.Fatalf("la riassociazione senza «Riassocia»: %+v", e)
	}
	if dopo := fotoVietate(t, p); dopo != prima {
		t.Fatalf("i rifiuti hanno toccato la working: %s -> %s", prima, dopo)
	}
	_ = c0
	_ = ctx
}

func TestV1S8SoloProdottoSenzaRegistro(t *testing.T) {
	p, s := preparaS8(t)
	corpo := func(chiave uuid.UUID, extra map[string]any) []byte {
		// messaggio 46 [U]: senza il motivo dell'utente (lo scrive il server, TracciaS8)
		c := map[string]any{"chiave_idempotenza": chiave, "prodotto_id": s.prodotto, "partenza": "solo_prodotto", "stato_analisi": "in_analisi",
			"presa_d_atto": true}
		for k, v := range extra {
			if v == nil {
				delete(c, k)
			} else {
				c[k] = v
			}
		}
		b, _ := json.Marshal(c)
		return b
	}
	prep := Preparazione{StatoAnalisi: AnalisiInCorso}
	prima := fotoVietate(t, p)
	// senza presa d'atto; con una struttura importabile adesso; con lo stato dell'analisi cambiato (senza motivo passa:
	// messaggio 46 [U])
	_, err := s8(t, p, s, s.stepC, corpo(uuid.New(), map[string]any{"presa_d_atto": nil}), prep)
	codiceDi(t, err, distintabozza.CodiceCorpoNonValido, MotivoPresaDAttoMancante)
	_, err = s8(t, p, s, s.stepC, corpo(uuid.New(), nil), Preparazione{StatoAnalisi: AnalisiPronta, Importabile: true})
	codiceDi(t, err, distintabozza.CodiceAnteprimaSuperata, "")
	_, err = s8(t, p, s, s.stepC, corpo(uuid.New(), nil), Preparazione{StatoAnalisi: AnalisiErrore})
	codiceDi(t, err, distintabozza.CodiceAnteprimaSuperata, "")
	if n := conta(t, p, `SELECT count(*) FROM documento`); n != 0 {
		t.Fatalf("i rifiuti del passo 0 hanno scritto un documento: %d", n)
	}
	// la partenza solo_prodotto
	es, err := s8(t, p, s, s.stepC, corpo(uuid.New(), nil), prep)
	if err != nil || es.Esito != EsitoCompletata || es.Bozza == nil || es.Bozza.Importazione != nil {
		t.Fatalf("solo_prodotto: %+v %v", es, err)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_importazione`); n != 0 {
		t.Fatalf("registri d'importazione: %d (niente registro fittizio)", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1 AND importazione_id IS NULL AND fonte_allegato_id = $2
		AND fonte_sha256 = $3 AND fonte_gruppo = 'solo_prodotto'`, es.Bozza.BozzaID, s.stepC, s.shaC); n != 1 {
		t.Fatalf("la versione 1 con la sorgente reale e il gruppo riservato: %d", n)
	}
	var snap []byte
	if err := p.QueryRow(context.Background(), `SELECT snapshot FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = 1`, es.Bozza.BozzaID).Scan(&snap); err != nil {
		t.Fatal(err)
	}
	s3, err := distintabozza.LeggiSnapshot3(snap)
	if err != nil || len(s3.Nodi) != 1 || s3.Sorgente == nil || s3.Sorgente.Importazione != "" || s3.Sorgente.Gruppo != "solo_prodotto" {
		t.Fatalf("snapshot solo_prodotto: %+v %v", s3, err)
	}
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'scelta' AND gruppo = 'solo_prodotto' AND allegato_id = $1`, s.stepC); n != 1 {
		t.Fatalf("la scelta con il gruppo riservato: %d", n)
	}
	// la traccia del gesto nella riga di S8 e nella scelta della fonte (messaggio 46 [U])
	tracciaSolo := "gesto di S8P da «Conferma la sorgente»: lo STEP «ACM-1000_senza.stp» è la sorgente di ACM-1000; partenza dal solo prodotto (analisi: in_analisi)"
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione IN ('conferma_documento', 'scelta') AND allegato_id = $1 AND motivo = $2`, s.stepC, tracciaSolo); n != 2 {
		t.Fatalf("la traccia del gesto nelle righe della fonte: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE gruppo = 'senza'`); n != 0 {
		t.Fatal("una riga con il gruppo «senza»")
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_evento WHERE bozza_id = $1 AND azione = 'bozza_aperta' AND dopo->>'algoritmo' = 'solo-prodotto-1'
		AND dopo->>'presa_d_atto_analisi' = 'in_analisi'`, es.Bozza.BozzaID); n != 1 {
		t.Fatalf("l'apertura con l'algoritmo solo-prodotto-1: %d", n)
	}
	if dopo := fotoVietate(t, p); dopo != prima {
		t.Fatalf("solo_prodotto ha toccato la working: %s -> %s", prima, dopo)
	}
	// la rotta di R2 non accetta la partenza solo_prodotto da sola
	raw := []byte(`{"prodotto_id":"` + s.altroFinito.String() + `","bozza_id":null,"versione_attesa":0,"vocabolario":2,"chiave_salvataggio":"` +
		uuid.New().String() + `","crea":{"partenza":"solo_prodotto"}}`)
	_, err = distintabozza.LeggiRichiestaR2(raw)
	codiceDi(t, err, distintabozza.CodiceCorpoNonValido, distintabozza.MotivoPartenzaNonAmmessa)
}

func TestV1S8LaWorkingConLoStepAELoStepB(t *testing.T) {
	p, s := preparaS8(t)
	ctx := context.Background()
	// la working confermata con lo STEP A: il documento A del prodotto (confermato prima di V1, senza riga di S8), lo
	// STEP strutturale del prodotto, e una conferma formato 1 valida (contatore uguale a quello della working)
	var docA uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, $2, 'cad_3d', 'ACM-1000', 'ACM-1000.stp', 'stp', $3, '3D/ACM-1000.stp', $4) RETURNING documento_id`,
		s.thread, s.prodotto, s.shaA, s.utente).Scan(&docA); err != nil {
		t.Fatal(err)
	}
	esegui(t, p, `UPDATE componente SET step_strutturale_id = $2 WHERE componente_id = $1`, s.prodotto, docA)
	confermaFormato1(t, p, s)
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	st, err := confermabom.LeggiStatoRFQ(ctx, db.New(tx), s.thread)
	_ = tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, v, ok, err := st.ValiditaUltima(s.prodotto); err != nil || !ok || !v.Valida {
		t.Fatalf("la conferma di prova non e' valida: %v %v %v", ok, v, err)
	}
	// lo STEP storico A non soddisfa il gate senza S8 (D-AR04 (iii))
	if sv, _ := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.prodotto, s.shaA); sv.Valida {
		t.Fatal("lo STEP storico vale come sorgente S8")
	}
	prima := fotoVietate(t, p)
	// S8 sullo STEP B: il documento B si aggiunge (A resta corrente), la bozza nasce da-conferma-1 e cambia la fonte su B
	regB := registroRadice(t, s, s.stepB, s.shaB, "ACM-1000")
	// con la presa d'atto «sostituisci»: nessun effetto sul documento A
	es, err := s8(t, p, s, s.stepB, corpoStep(t, s, uuid.New(), regB, map[string]any{"sostituisci": map[string]any{"conferma": true}}),
		Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regB}})
	if err != nil || es.Esito != EsitoCompletata || es.Bozza == nil || es.Bozza.Versione != 2 || es.Bozza.Importazione == nil {
		t.Fatalf("S8 su B: %+v %v", es, err)
	}
	if dopo := fotoVietate(t, p); dopo != prima {
		t.Fatalf("S8 su B ha toccato la working: %s -> %s", prima, dopo)
	}
	var stepCorrente uuid.UUID
	if err := p.QueryRow(ctx, `SELECT step_strutturale_id FROM componente WHERE componente_id = $1`, s.prodotto).Scan(&stepCorrente); err != nil || stepCorrente != docA {
		t.Fatalf("lo STEP strutturale del prodotto non e' piu' A: %v %v", stepCorrente, err)
	}
	if n := conta(t, p, `SELECT count(*) FROM documento WHERE componente_id = $1 AND tipo = 'cad_3d' AND sostituito_da IS NULL`, s.prodotto); n != 2 {
		t.Fatalf("documenti 3D correnti del prodotto: %d (A e B)", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_versione v JOIN distinta_bozza b USING (bozza_id) WHERE b.prodotto_id = $1 AND v.versione = 1
		AND v.importazione_id IS NULL`, s.prodotto); n != 1 {
		t.Fatalf("versione 1 da-conferma-1: %d", n)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza WHERE prodotto_id = $1 AND stato = 'aperta' AND fonte_sha256 = $2`, s.prodotto, s.shaB); n != 1 {
		t.Fatalf("la bozza aperta sulla sorgente B: %d", n)
	}
	// la conferma formato 1 non ha una fonte: il cambio della fonte la registra come scelta della bozza
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'scelta' AND sha256 = $1 AND bozza_id = $2`, s.shaB, es.Bozza.BozzaID); n != 1 {
		t.Fatalf("il cambio della fonte nella storia: %d", n)
	}
	if sv, _ := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.prodotto, s.shaB); !sv.Valida {
		t.Fatalf("B non e' valida: %+v", sv)
	}
	// S8 sullo STEP storico A: solo la riga di S8 (nessun documento nuovo), e la bozza non cambia senza la scelta della bozza
	regA := registroRadice(t, s, s.stepA, s.shaA, "ACM-1000")
	_, err = s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, nil), Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}})
	codiceDi(t, err, distintabozza.CodiceBozzaVersione, distintabozza.MotivoAltraBozzaAperta)
	if n := conta(t, p, `SELECT count(*) FROM documento WHERE sha256 = $1`, s.shaA); n != 1 {
		t.Fatalf("documenti di A: %d", n)
	}
}

// confermaFormato1: una conferma formato 1 minima e valida del prodotto (il contatore della working di adesso),
// scritta a mano con i trigger della testata e dei certificati spenti nella sua transazione.
func confermaFormato1(t *testing.T, p *pgxpool.Pool, s scenaS8) {
	t.Helper()
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	conferma := uuid.New()
	snap, err := jsoncanonico.Codifica(json.RawMessage(`{"formato":1,"perimetro":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []struct {
		sql  string
		args []any
	}{
		{`ALTER TABLE conferma_strutturale DISABLE TRIGGER USER`, nil},
		{`ALTER TABLE conferma_strutturale_componente DISABLE TRIGGER USER`, nil},
		{`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura,
				base_contatore, contatore, contatore_dati, origine, snapshot, impronta_snapshot, delta, mappa, criterio_verifica, confermato_da)
			VALUES ($1, $2, $3, 1, 1, true, repeat('3', 64), 0,
				COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = $2), 0), 0,
				'{"rotta":"prova"}', $5::jsonb, $6, '{}', '{}', 'nessuno', $4)`,
			[]any{conferma, s.thread, s.prodotto, s.utente, string(snap), jsoncanonico.Impronta(snap)}},
		{`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, codice, tipo)
			SELECT $1, c.thread_id, c.componente_id, c.codice, c.tipo FROM componente c WHERE c.componente_id = ANY($2::uuid[])`,
			[]any{conferma, []uuid.UUID{s.prodotto, s.figlio}}},
		{`SET CONSTRAINTS ALL IMMEDIATE`, nil},
		{`ALTER TABLE conferma_strutturale_componente ENABLE TRIGGER USER`, nil},
		{`ALTER TABLE conferma_strutturale ENABLE TRIGGER USER`, nil},
	} {
		if _, err := tx.Exec(ctx, x.sql, x.args...); err != nil {
			t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestV1S8LaRevoca(t *testing.T) {
	p, s := preparaS8(t)
	ctx := context.Background()
	regA := registroRadice(t, s, s.stepA, s.shaA, "ACM-1000")
	prep := Preparazione{Proposta: &distintabozza.PropostaCalcolata{Registro: regA}}
	es, err := s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, nil), prep)
	if err != nil || es.Esito != EsitoCompletata {
		t.Fatalf("S8: %+v %v", es, err)
	}
	prima := fotoVietate(t, p)
	versioni := conta(t, p, `SELECT count(*) FROM distinta_bozza_versione`)
	revoca := func(chiave uuid.UUID, motivo string) (EsitoRevoca, error) {
		c := map[string]any{"chiave_idempotenza": chiave, "prodotto_id": s.prodotto}
		if motivo != "" {
			c["motivo"] = motivo
		}
		raw, _ := json.Marshal(c)
		r, imp, err := LeggiRichiestaRevoca(raw)
		if err != nil {
			return EsitoRevoca{}, err
		}
		return Revoca(ctx, p, s.thread, s.stepA, s.utente, r, imp)
	}
	// messaggio 46 [U]: la revoca senza motivo; il motivo della riga e' la traccia del gesto
	chiave := uuid.New()
	rv, err := revoca(chiave, "")
	if err != nil || rv.Riproposto || rv.BozzaID == nil || *rv.BozzaID != es.Bozza.BozzaID || rv.DocumentoID == uuid.Nil ||
		rv.BozzaAbbandonata == nil || *rv.BozzaAbbandonata != es.Bozza.BozzaID || rv.UsataDaConferma {
		t.Fatalf("revoca: %+v %v", rv, err)
	}
	// G39-03 [U]: prima di R5 la bozza nata dalla sorgente revocata si annulla (abbandonata, la storia resta)
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza WHERE bozza_id = $1 AND stato = 'abbandonata' AND motivo_chiusura LIKE 'sorgente revocata: %'`,
		es.Bozza.BozzaID); n != 1 {
		t.Fatalf("la bozza della sorgente revocata non e' abbandonata: %d", n)
	}
	if sv, _ := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.prodotto, s.shaA); sv.Valida || sv.Motivo != confermabom.SorgenteRevocata {
		t.Fatalf("la sorgente revocata vale ancora: %+v", sv)
	}
	if dopo := fotoVietate(t, p); dopo != prima {
		t.Fatalf("la revoca ha toccato la working: %s -> %s", prima, dopo)
	}
	if n := conta(t, p, `SELECT count(*) FROM distinta_bozza_versione`); n != versioni {
		t.Fatalf("la revoca ha cambiato le versioni della bozza (la storia resta): %d -> %d", versioni, n)
	}
	tracciaRevoca := "gesto di S8P da «Revoca come sorgente»: lo STEP «ACM-1000.stp» non è più la sorgente confermata di ACM-1000"
	if n := conta(t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'revoca_sorgente' AND chiave_idempotenza = $1 AND motivo = $2`, chiave, tracciaRevoca); n != 1 || rv.Motivo != tracciaRevoca {
		t.Fatalf("la riga della revoca: %d %q", n, rv.Motivo)
	}
	// reinvio; la stessa chiave con un motivo (un altro corpo); una seconda revoca senza niente da revocare
	if rv2, err := revoca(chiave, ""); err != nil || !rv2.Riproposto || rv2.Motivo != tracciaRevoca {
		t.Fatalf("reinvio della revoca: %+v %v", rv2, err)
	}
	_, err = revoca(chiave, "un altro motivo")
	codiceDi(t, err, distintabozza.CodiceChiaveRiusata, "")
	_, err = revoca(uuid.New(), "di nuovo")
	codiceDi(t, err, distintabozza.CodiceCorpoNonValido, MotivoNessunaSorgente)
	// S8 di nuovo sullo stesso STEP: la riga di S8 si scrive di nuovo (nessun documento nuovo), la bozza nuova nasce da li'
	es2, err := s8(t, p, s, s.stepA, corpoStep(t, s, uuid.New(), regA, nil), prep)
	if err != nil || es2.Passi[1].Esito != EsitoFatto || es2.Passi[2].Esito != EsitoFatto || es2.Bozza == nil || es2.Bozza.BozzaID == es.Bozza.BozzaID {
		t.Fatalf("S8 dopo la revoca: %+v %v", es2, err)
	}
	if sv, _ := confermabom.SorgenteValida(ctx, db.New(p), s.thread, s.prodotto, s.shaA); !sv.Valida {
		t.Fatalf("la sorgente riconfermata non vale: %+v", sv)
	}
	if n := conta(t, p, `SELECT count(*) FROM documento WHERE sha256 = $1`, s.shaA); n != 1 {
		t.Fatalf("documenti dopo la riconferma: %d", n)
	}
	// «RFQ formato 3» resta vera anche con la bozza abbandonata
	abbandona(t, p, s)
	if f3, err := associazioni.RFQFormato3(ctx, db.New(p), s.thread); err != nil || !f3 {
		t.Fatalf("RFQ formato 3 dopo l'abbandono: %v %v", f3, err)
	}
	if f3, err := associazioni.RFQFormato3(ctx, db.New(p), s.thread2); err != nil || f3 {
		t.Fatalf("la seconda RFQ e' legacy: %v %v", f3, err)
	}
}
