//go:build integrazione

// L4 — la 0022 di P4 (giro 5, A1d, Q1; direttiva 2026-10-12-06, «Q1/L4 obbligatorie»): installazione da zero e
// due avvii identici del migratore, la 0022 che non si riapplica, l'upgrade 21 → 22 con righe sintetiche, tutti i
// vincoli FK e CHECK delle 18 tabelle nuove (con il controllo che nessuno resti senza prova; le tre della
// direttiva 11 sono distinta_bozza_registro, copertura_documento, schema_variante), i trigger del
// contatore su ogni scrittore legacy (le query sqlc vere) e sugli scrittori diretti, nessun contatore su un UPDATE
// identico, la conferma che si scrive per ultima (R1 della revisione DDL), la storia che non si modifica e non si
// cancella, i limiti dei campi. Solo dati sintetici (ACME, example.com).

package migrazioni_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/migrazioni"
	"promatec/cockpit/internal/platform/testutil"
)

// Le 19 tabelle della 0022 (15 della prima stesura, 3 dell'emendamento della direttiva 11, 1 delle revisioni della
// notte: deroga_struttura_storia, RSC-05).
var tabelle22 = []string{"working_contatore", "distinta_bozza", "distinta_bozza_versione", "distinta_bozza_evento",
	"conferma_strutturale", "conferma_strutturale_componente", "conferma_strutturale_arco", "esito_intento", "f8_stato",
	"correzione_documento", "associazione_evento", "fonte_step_evento", "attestazione_fascicolo",
	"attestazione_fascicolo_documento", "deroga_fabbisogno_storia",
	"distinta_bozza_registro", "copertura_documento", "schema_variante", "deroga_struttura_storia"}

// I quattro trigger che la 0022 mette su tabelle che c'erano gia'.
var triggerSuStoriche22 = []string{"trg_working_conta_componente", "trg_working_conta_relazione", "trg_deroga_fabbisogno_storia",
	"trg_deroga_struttura_storia"}

var sost22 = strings.NewReplacer(
	"{U}", "00000000-0000-4000-8000-0000000022c1",
	"{C}", "00000000-0000-4000-8000-0000000022b1",
	"{T}", "00000000-0000-4000-8000-0000000022a1",
	"{T2}", "00000000-0000-4000-8000-0000000022a2",
	"{CV}", "00000000-0000-4000-8000-0000000022e0",
	"{M}", "00000000-0000-4000-8000-0000000022e1",
	"{A}", "00000000-0000-4000-8000-0000000022f1",
	"{A2}", "00000000-0000-4000-8000-0000000022f2",
	"{P}", "00000000-0000-4000-8000-000000221000",
	"{S}", "00000000-0000-4000-8000-000000221100",
	"{P2}", "00000000-0000-4000-8000-000000222000",
	"{P3}", "00000000-0000-4000-8000-000000223000",
	"{Q}", "00000000-0000-4000-8000-000000229000",
	"{D}", "00000000-0000-4000-8000-0000002200d1",
	"{D2}", "00000000-0000-4000-8000-0000002200d2",
	"{DP}", "00000000-0000-4000-8000-0000002200e1",
	"{DF}", "00000000-0000-4000-8000-0000002200f1",
	"{C1}", "00000000-0000-4000-8000-000000220c01",
	"{K1}", "00000000-0000-4000-8000-000000220c11",
	"{B1}", "00000000-0000-4000-8000-000000220b01",
	"{AT1}", "00000000-0000-4000-8000-000000220a71",
)

func u22(nome string) uuid.UUID { return uuid.MustParse(sost22.Replace(nome)) }

// base21: righe sintetiche che stanno anche nello schema 21 (l'upgrade parte da qui). Due RFQ ACME; nella prima un
// finito P con un sottoassieme S (qta 2), due finiti senza figli P2 e P3, un documento STEP di P e un disegno di S,
// una proposta di documento, una deroga; nella seconda un finito Q.
const base21 = `
INSERT INTO utente (utente_id, sigla, nome, ufficio, ruolo) VALUES ('{U}', 'P22', 'Prova 0022', 'UT', 'admin');
INSERT INTO cliente (cliente_id, cartella_nas, ragione_sociale) VALUES ('{C}', 'ACME22', 'ACME S.p.A.');
INSERT INTO thread_offerta (thread_id, cliente_id, canale, data_inizio, oggetto) VALUES
  ('{T}', '{C}', 'outlook', now(), 'RFQ ACME-1000'), ('{T2}', '{C}', 'outlook', now(), 'RFQ ACME-9000');
INSERT INTO conversazione (conversazione_id, canale, chiave_esterna, thread_id, primo_messaggio_il)
  VALUES ('{CV}', 'outlook', 'conv-0022@example.com', '{T}', now());
INSERT INTO messaggio (messaggio_id, canale, chiave_esterna, conversazione_id, thread_id, direzione, data_evento, oggetto)
  VALUES ('{M}', 'outlook', 'msg-0022@example.com', '{CV}', '{T}', 'entrata', now(), 'RFQ ACME-1000');
INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il) VALUES
  ('{A}', '{M}', 1, 'ACM-1000.stp', 'stp', repeat('b', 64), now()),
  ('{A2}', '{M}', 2, 'ACM-1100.pdf', 'pdf', repeat('d', 64), now());
INSERT INTO componente (componente_id, thread_id, codice, rev, descrizione, tipo, origine, confermato_da) VALUES
  ('{P}', '{T}', 'ACM-1000', 'A', 'Gruppo base', 'finito', 'manuale', '{U}'),
  ('{S}', '{T}', 'ACM-1100', 'A', 'Telaio', 'sottoassieme', 'manuale', '{U}'),
  ('{P2}', '{T}', 'ACM-2000', NULL, 'Piastra', 'finito', 'manuale', '{U}'),
  ('{P3}', '{T}', 'ACM-3000', NULL, NULL, 'finito', 'manuale', '{U}'),
  ('{Q}', '{T2}', 'ACM-9000', NULL, NULL, 'finito', 'manuale', '{U}');
INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da)
  VALUES ('{T}', '{P}', '{S}', 2, 'manuale', '{U}');
INSERT INTO documento (documento_id, thread_id, componente_id, tipo, codice, rev, nome_file, estensione, sha256, path_relativo, confermato_da) VALUES
  ('{D}', '{T}', '{P}', 'cad_3d', 'ACM-1000', 'A', 'ACM-1000.stp', 'stp', repeat('c', 64), 'ACME22/ACM-1000.stp', '{U}'),
  ('{D2}', '{T}', '{S}', 'disegno_2d', 'ACM-1100', 'A', 'ACM-1100.pdf', 'pdf', repeat('e', 64), 'ACME22/ACM-1100.pdf', '{U}');
INSERT INTO documento_proposta (proposta_id, allegato_id, thread_id, tipo_proposto, confidenza, fonte)
  VALUES ('{DP}', '{A2}', '{T}', 'disegno_2d', 50, 'nome_file');
INSERT INTO deroga_fabbisogno (deroga_id, thread_id, componente_id, tipo, motivo, utente_id)
  VALUES ('{DF}', '{T}', '{S}', 'sviluppo_dxf', 'deroga nata alla 21', '{U}');`

func esegui22(t *testing.T, p *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := p.Exec(context.Background(), sost22.Replace(sql)); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
}

func uno22[T any](t *testing.T, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, sql string) T {
	t.Helper()
	var v T
	if err := q.QueryRow(context.Background(), sost22.Replace(sql)).Scan(&v); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return v
}

// conferma1 registra la conferma n. 1 di P (formato 2, criterio nessuno), con i contatori correnti e il
// perimetro di P (P radice, S, l'arco P → S qta 2). Tutto in una transazione, come fara' R5: la testata dopo le
// scritture (qui nessuna), componenti e archi nella stessa transazione.
const conferma1 = `
INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2,
       firma_struttura, firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati,
       origine, snapshot, impronta_snapshot, delta, mappa, criterio_verifica, chiave_idempotenza, confermato_da)
SELECT '{C1}', '{T}', '{P}', 1, NULL, 2, false, repeat('3', 64), repeat('2', 64), NULL, 0, repeat('1', 64),
       COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = '{T}'), 0),
       COALESCE((SELECT dati FROM working_contatore WHERE thread_id = '{T}'), 0),
       '{"rotta":"prova"}', jsonb_build_object('formato', 2, 'prodotto_id', '{P}'::text), repeat('a', 64), '{}', '{}',
       'nessuno', '{K1}', '{U}';
INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo)
VALUES ('{C1}', '{T}', '{P}', true, 'k-p', 'ACM-1000', 'A', 'Gruppo base', 'finito'),
       ('{C1}', '{T}', '{S}', false, 'k-s', 'ACM-1100', 'A', 'Telaio', 'sottoassieme');
INSERT INTO conferma_strutturale_arco (conferma_id, padre_id, figlio_id, qta, origine) VALUES ('{C1}', '{P}', '{S}', 2, 'conferma');`

// storia22: una bozza aperta di P2 (versione 1 con un evento), l'esito della conferma n. 1, l'attestazione n. 1 di
// P con il documento STEP, uno stato F8, una correzione, un'associazione, un evento della fonte STEP, una deroga
// strutturale.
var storia22 = []string{
	`INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
	 VALUES ('{B1}', '{T}', '{P2}', 0, 0, repeat('1', 64), '{U}', '{U}');
	 INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, salvata_da)
	 VALUES ('{B1}', 1, '{"formato":2}', repeat('a', 64), 0, 0, repeat('1', 64), '{U}');
	 INSERT INTO distinta_bozza_evento (bozza_id, n, versione, azione, origine, utente_id) VALUES ('{B1}', 1, 1, 'apri_bozza', 'step', '{U}');`,
	`INSERT INTO esito_intento (chiave, thread_id, gesto, utente_id, impronta_corpo, esito, conferma_id)
	 VALUES ('{K1}', '{T}', 'conferma_bom_v2', '{U}', repeat('a', 64), '{"esito":"confermata"}', '{C1}');
	 INSERT INTO f8_stato (thread_id, chiave_intento, conferma_id) VALUES ('{T}', '{K1}', '{C1}');`,
	`INSERT INTO attestazione_fascicolo (attestazione_id, thread_id, prodotto_id, numero, conferma_strutturale_id, firma_piano, attestato_da)
	 VALUES ('{AT1}', '{T}', '{P}', 1, '{C1}', repeat('f', 64), '{U}');
	 INSERT INTO attestazione_fascicolo_documento (attestazione_id, thread_id, documento_id, componente_id, tipo, rev, sha256)
	 VALUES ('{AT1}', '{T}', '{D}', '{P}', 'cad_3d', 'A', repeat('c', 64));`,
	`INSERT INTO correzione_documento (thread_id, proposta_id, allegato_id, nome_file, sha256, codice_prima, codice_dopo, motivo, origine, autore_id)
	 VALUES ('{T}', '{DP}', '{A2}', 'ACM-1100.pdf', repeat('d', 64), 'ACM-110', 'ACM-1100', 'refuso nel nome', 'motore_a', '{U}');
	 INSERT INTO associazione_evento (thread_id, proposta_id, azione, componente_dopo, origine, autore_id)
	 VALUES ('{T}', '{DP}', 'assegna', '{S}', 'ponte', '{U}');
	 INSERT INTO fonte_step_evento (thread_id, prodotto_id, azione, allegato_id, sha256, gruppo, autore_id)
	 VALUES ('{T}', '{P}', 'scelta', '{A}', repeat('b', 64), 'cand:a:root', '{U}');`,
	// una deroga strutturale sullo STEP di P, con la sua riga di storia (RSC-05)
	`INSERT INTO deroga_struttura (thread_id, componente_id, step_documento_id, step_sha256, motivo_parziale, motivo, concessa_da)
	 VALUES ('{T}', '{P}', '{D}', repeat('c', 64), 'nessuna occorrenza di assieme', 'la gerarchia e'' quella del PDF', '{U}');`,
}

// schema22ConStoria: schema pulito alla 22, le righe di base, la conferma n. 1 e la storia. Ogni blocco nella sua
// transazione (i trigger differiti controllano al COMMIT).
func schema22ConStoria(t *testing.T) *pgxpool.Pool {
	t.Helper()
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	esegui22(t, p, base21)
	esegui22(t, p, conferma1)
	for _, s := range storia22 {
		esegui22(t, p, s)
	}
	return p
}

// ------------------------------------------------------------------ installazione, avvii, riapplicazione

func TestP4La22DaZeroEDueAvviiIdentici(t *testing.T) {
	p := testutil.Pool(t)
	testutil.SchemaPulito(t, p)
	ctx := context.Background()
	if !applicata(t, p, 22) || uno22[int32](t, p, `SELECT max(versione) FROM schema_versione`) != 22 {
		t.Fatal("dopo SchemaPulito lo schema non e' alla 22")
	}
	for _, tab := range tabelle22 {
		if !uno22[bool](t, p, fmt.Sprintf(`SELECT to_regclass('public.%s') IS NOT NULL`, tab)) {
			t.Errorf("manca la tabella %s", tab)
		}
	}
	if n := uno22[int](t, p, `SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgname = ANY('{trg_working_conta_componente,trg_working_conta_relazione,trg_deroga_fabbisogno_storia,trg_deroga_struttura_storia}')`); n != 4 {
		t.Errorf("trigger sulle tabelle storiche: %d, attesi 4", n)
	}
	prima := impronta18(t, p)
	versioni := uno22[int](t, p, `SELECT count(*) FROM schema_versione`)
	// secondo avvio: niente da applicare, niente che cambia
	n, err := migrazioni.Applica(ctx, p, risorse.FS, testutil.LogSilenzioso())
	if err != nil || n != 0 {
		t.Fatalf("secondo avvio: %d migrazioni applicate, errore %v (attese 0, nil)", n, err)
	}
	if d := differenze(prima, impronta18(t, p)); d != "" {
		t.Errorf("il secondo avvio ha cambiato lo schema:\n%s", d)
	}
	if v := uno22[int](t, p, `SELECT count(*) FROM schema_versione`); v != versioni {
		t.Errorf("schema_versione: %d righe prima, %d dopo il secondo avvio", versioni, v)
	}
}

// Il testo della 0022 rieseguito su uno schema che ce l'ha gia' si ferma al primo oggetto e non cambia niente:
// il migratore non la rilancerebbe (versione gia' registrata), ma anche a mano non lascia mezzo schema.
func TestP4La0022NonSiRiapplica(t *testing.T) {
	p := schema22ConStoria(t)
	ctx := context.Background()
	migs, err := migrazioni.Elenca(risorse.FS)
	if err != nil {
		t.Fatal(err)
	}
	m := migs[len(migs)-1]
	if m.Versione != 22 {
		t.Fatalf("ultima migrazione %s", m.Nome)
	}
	prima := impronta18(t, p)
	righe := uno22[int](t, p, `SELECT (SELECT count(*) FROM conferma_strutturale) + (SELECT count(*) FROM distinta_bozza_evento) + (SELECT count(*) FROM working_contatore)`)
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, m.SQL)
	_ = tx.Rollback(ctx)
	if err == nil || !strings.Contains(err.Error(), "p4_rifiuta") {
		t.Fatalf("la 0022 rieseguita doveva fermarsi su p4_rifiuta gia' esistente: %v", err)
	}
	if d := differenze(prima, impronta18(t, p)); d != "" {
		t.Errorf("la riapplicazione fallita ha cambiato lo schema:\n%s", d)
	}
	if r := uno22[int](t, p, `SELECT (SELECT count(*) FROM conferma_strutturale) + (SELECT count(*) FROM distinta_bozza_evento) + (SELECT count(*) FROM working_contatore)`); r != righe {
		t.Errorf("righe %d prima, %d dopo", righe, r)
	}
}

// impronteDati: per ogni tabella dell'elenco, l'md5 delle righe in ordine di testo.
func impronteDati(t *testing.T, p *pgxpool.Pool, tabelle []string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, tab := range tabelle {
		out[tab] = uno22[string](t, p, fmt.Sprintf(`SELECT md5(coalesce(string_agg(x::text, '|' ORDER BY x::text), '')) FROM %s x`, pgx.Identifier{tab}.Sanitize()))
	}
	return out
}

func tabelleDelloSchema(t *testing.T, p *pgxpool.Pool) []string {
	t.Helper()
	righe, err := p.Query(context.Background(), `SELECT tablename::text FROM pg_tables WHERE schemaname = 'public' ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(righe, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// oggettoDelCatalogo: la tabella e il nome dell'oggetto di una riga di impronta18 (relazione, colonna, vincolo,
// indice, trigger); stringhe vuote per gli altri tipi di riga.
func oggettoDelCatalogo(r string) (tabella, nome string) {
	campi := strings.Fields(r)
	if len(campi) < 2 {
		return "", ""
	}
	dopoOn := func() string {
		for i, c := range campi {
			if c == "ON" && i+1 < len(campi) {
				return strings.TrimPrefix(campi[i+1], "public.")
			}
		}
		return ""
	}
	switch campi[0] {
	case "relazione":
		return campi[1], campi[1]
	case "colonna":
		tc := strings.SplitN(campi[1], ".", 2)
		if len(tc) == 2 {
			return tc[0], tc[1]
		}
	case "vincolo":
		if len(campi) >= 3 {
			return strings.TrimPrefix(campi[1], "public."), campi[2]
		}
	case "indice":
		return dopoOn(), campi[1]
	case "trigger":
		// trigger CREATE [CONSTRAINT] TRIGGER nome ...
		for i, c := range campi {
			if c == "TRIGGER" && i+1 < len(campi) {
				return dopoOn(), campi[i+1]
			}
		}
	}
	return "", ""
}

// 21 → 22 su un database con righe: niente di quello che c'era cambia (catalogo e dati), nascono solo gli oggetti
// nuovi e i tre trigger sulle tabelle storiche, nessuna riga nelle tabelle nuove, e lo schema che ne esce e'
// identico a quello installato da zero.
func TestP4Upgrade21a22ConRigheSintetiche(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	ctx := context.Background()
	testutil.SchemaFinoA(t, p, 21)
	esegui22(t, p, base21)
	storiche := tabelleDelloSchema(t, p)
	catPrima := impronta18(t, p)
	datiPrima := impronteDati(t, p, storiche)

	if _, err := migrazioni.ApplicaFinoA(ctx, p, risorse.FS, 22, testutil.LogSilenzioso()); err != nil {
		t.Fatalf("upgrade 21 → 22: %v", err)
	}
	if !applicata(t, p, 22) {
		t.Fatal("la 22 non risulta applicata")
	}
	catDopo := impronta18(t, p)
	inDopo := map[string]bool{}
	for _, r := range catDopo {
		inDopo[r] = true
	}
	// l'unica riga che puo' cambiare e' la definizione di v_fascicolo (deroga dichiarata, D-SHOCK-DOC-02 A): le sue
	// 19 colonne restano righe «colonna» identiche (nome, posizione, tipo), controllate dal ciclo stesso
	for _, r := range catPrima {
		if !inDopo[r] && !strings.HasPrefix(r, "vista v_fascicolo ") {
			t.Errorf("l'upgrade ha cambiato o tolto: %s", r)
		}
	}
	if n := uno22[int](t, p, `SELECT count(*) FROM pg_attribute WHERE attrelid = 'v_fascicolo'::regclass AND attnum > 0`); n != 20 {
		t.Errorf("v_fascicolo ha %d colonne, attese le 19 della 0020 piu' copertura_id", n)
	}
	if c := uno22[string](t, p, `SELECT attname::text FROM pg_attribute WHERE attrelid = 'v_fascicolo'::regclass AND attnum = 20`); c != "copertura_id" {
		t.Errorf("la colonna in coda di v_fascicolo e' %q, attesa copertura_id", c)
	}
	inPrima := map[string]bool{}
	for _, r := range catPrima {
		inPrima[r] = true
	}
	// sulle tabelle storiche la 0022 aggiunge SOLO i tre trigger AFTER: nessuna colonna, vincolo, indice o altro
	// trigger (le mutazioni S70 e S71 della revisione delle prove: un trigger trg_p4_* o una colonna
	// conferma_strutturale_* su una tabella storica passavano il filtro dei nomi)
	storicheSet := map[string]bool{}
	for _, tab := range storiche {
		storicheSet[tab] = true
	}
	permessiSuStoriche := map[string]bool{}
	for _, tg := range triggerSuStoriche22 {
		permessiSuStoriche[tg] = true
	}
	nuoviNomi := append(append([]string{}, tabelle22...), triggerSuStoriche22...)
	nuoviNomi = append(nuoviNomi, "p4_", "working_conta", "distinta_bozza", "conferma_strutturale", "esito_intento",
		"f8_stato", "attestazione_", "deroga_fabbisogno_registra", "deroga_fabbisogno_storia", "ux_csc", "ix_csc", "ix_csa",
		"ix_dbe", "ix_dfs", "ix_afd", "ux_conferma", "ix_conferma", "ux_esito", "ix_esito", "ix_f8", "ix_correzione",
		"ix_associazione", "ix_fonte_step", "ux_fonte_step", "fonte_step", "ux_distinta", "ix_distinta", "ix_attestazione",
		"schema_versione", "copertura", "ix_dbr", "ux_dbr", "trg_csc_risolve_fonte", "v_fascicolo",
		"deroga_struttura_registra", "ix_dss")
	for _, r := range catDopo {
		if inPrima[r] {
			continue
		}
		if tab, nome := oggettoDelCatalogo(r); storicheSet[tab] && !permessiSuStoriche[nome] {
			t.Errorf("riga nuova del catalogo su una tabella storica (%s): %s", tab, r)
			continue
		}
		ok := false
		for _, n := range nuoviNomi {
			if strings.Contains(r, n) {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("riga nuova del catalogo che non e' della 0022: %s", r)
		}
	}
	datiDopo := impronteDati(t, p, storiche)
	for _, tab := range storiche {
		if tab != "schema_versione" && datiPrima[tab] != datiDopo[tab] {
			t.Errorf("l'upgrade ha cambiato i dati di %s", tab)
		}
	}
	for _, tab := range tabelle22 {
		atteso := 0
		if tab == "schema_variante" {
			atteso = 1 // la riga della variante della 0022, e nessun'altra
		}
		if n := testutil.Conta(t, p, tab); n != atteso {
			t.Errorf("l'upgrade ha scritto %d righe in %s (attese %d)", n, tab, atteso)
		}
	}
	// lo schema che esce dall'upgrade e quello installato da zero sono lo stesso schema
	testutil.SchemaPulito(t, p)
	if d := differenze(catDopo, impronta18(t, p)); d != "" {
		t.Errorf("lo schema dopo 21 → 22 non e' quello da zero:\n%s", d)
	}
}

// ------------------------------------------------------------------ tutti i vincoli FK e CHECK

// riga e' una INSERT valida per una tabella nuova, colonna → espressione SQL; le prove ne cambiano qualche colonna.
type riga []struct{ col, expr string }

var righeValide22 = map[string]riga{
	"working_contatore": {{"thread_id", "gen_random_uuid()"}, {"struttura", "0"}, {"dati", "0"}},
	"distinta_bozza": {{"thread_id", "'{T}'"}, {"prodotto_id", "'{P3}'"}, {"stato", "'aperta'"}, {"formato", "2"}, {"versione", "0"},
		{"base_conferma_id", "NULL"}, {"base_numero", "0"}, {"base_contatore", "0"}, {"firma_base", "repeat('1', 64)"},
		{"fonte_allegato_id", "NULL"}, {"fonte_sha256", "NULL"}, {"creata_da", "'{U}'"}, {"aggiornata_da", "'{U}'"},
		{"chiusa_da", "NULL"}, {"chiusa_il", "NULL"}},
	"distinta_bozza_versione": {{"bozza_id", "'{B1}'"}, {"versione", "2"}, {"snapshot", `'{"formato":2}'`}, {"impronta", "repeat('a', 64)"},
		{"base_conferma_id", "NULL"}, {"base_numero", "0"}, {"base_contatore", "0"}, {"firma_base", "repeat('1', 64)"},
		{"fonte_allegato_id", "NULL"}, {"fonte_sha256", "NULL"}, {"salvata_da", "'{U}'"}},
	"distinta_bozza_evento": {{"bozza_id", "'{B1}'"}, {"n", "100"}, {"versione", "1"}, {"azione", "'aggiungi_nodo'"},
		{"chiave_bozza", "NULL"}, {"prima", "NULL"}, {"dopo", "NULL"}, {"origine", "'utente'"}, {"motivo", "NULL"}, {"utente_id", "'{U}'"}},
	"conferma_strutturale": {{"thread_id", "'{T}'"}, {"prodotto_id", "'{P}'"}, {"numero", "2"}, {"precedente_id", "'{C1}'"},
		{"formato", "2"}, {"senza_riepilogo_v2", "false"}, {"firma_struttura", "repeat('3', 64)"}, {"firma_anteprima", "repeat('2', 64)"},
		{"base_conferma_id", "'{C1}'"}, {"base_contatore", "0"}, {"firma_base", "repeat('1', 64)"}, {"contatore", "0"}, {"contatore_dati", "0"},
		{"bozza_id", "NULL"}, {"bozza_versione", "NULL"}, {"fonte_allegato_id", "NULL"}, {"fonte_sha256", "NULL"},
		{"origine", `'{"rotta":"prova"}'`}, {"snapshot", "jsonb_build_object('formato', 2, 'prodotto_id', '{P}'::text)"},
		{"impronta_snapshot", "repeat('a', 64)"}, {"delta", "'{}'"}, {"mappa", "'{}'"}, {"mono_pezzo", "false"}, {"mono_pezzo_motivo", "NULL"},
		{"criterio_verifica", "'nessuno'"}, {"chiave_idempotenza", "gen_random_uuid()"}, {"confermato_da", "'{U}'"}},
	"conferma_strutturale_componente": {{"conferma_id", "'{C1}'"}, {"thread_id", "'{T}'"}, {"componente_id", "'{P2}'"}, {"radice", "false"},
		{"chiave_bozza", "'k-p2'"}, {"codice", "'ACM-2000'"}, {"tipo", "'finito'"}, {"step_strutturale_id", "NULL"}, {"step_sha256", "NULL"}},
	"conferma_strutturale_arco": {{"conferma_id", "'{C1}'"}, {"padre_id", "'{S}'"}, {"figlio_id", "'{P}'"}, {"qta", "1"}, {"origine", "NULL"}},
	"esito_intento": {{"chiave", "gen_random_uuid()"}, {"thread_id", "'{T}'"}, {"gesto", "'lotto_fascicolo'"}, {"utente_id", "'{U}'"},
		{"impronta_corpo", "repeat('a', 64)"}, {"esito", "'{}'"}, {"conferma_id", "NULL"}},
	"f8_stato": {{"thread_id", "'{T}'"}, {"chiave_intento", "gen_random_uuid()"}, {"conferma_id", "NULL"}, {"stato", "'in_attesa'"},
		{"errore", "NULL"}, {"tentativi", "0"}, {"risolto_da", "NULL"}},
	"correzione_documento": {{"thread_id", "'{T}'"}, {"proposta_id", "'{DP}'"}, {"documento_id", "NULL"}, {"allegato_id", "'{A2}'"},
		{"nome_file", "'ACM-1100.pdf'"}, {"sha256", "repeat('d', 64)"}, {"motivo", "NULL"}, {"origine", "'assegna_legacy'"}, {"autore_id", "'{U}'"}},
	"associazione_evento": {{"thread_id", "'{T}'"}, {"proposta_id", "'{DP}'"}, {"documento_id", "NULL"}, {"azione", "'scarta'"},
		{"componente_dopo", "NULL"}, {"conferma_id", "NULL"}, {"motivo", "NULL"}, {"origine", "'legacy'"}, {"autore_id", "'{U}'"}},
	"fonte_step_evento": {{"thread_id", "'{T}'"}, {"prodotto_id", "'{P}'"}, {"azione", "'caricamento'"}, {"allegato_id", "'{A}'"},
		{"sha256", "repeat('b', 64)"}, {"gruppo", "NULL"}, {"proposta_id", "NULL"}, {"documento_id", "NULL"}, {"precedente_allegato_id", "NULL"},
		{"precedente_sha256", "NULL"}, {"motivo", "NULL"}, {"autore_id", "'{U}'"}},
	"attestazione_fascicolo": {{"thread_id", "'{T}'"}, {"prodotto_id", "'{P}'"}, {"numero", "2"}, {"precedente_id", "'{AT1}'"},
		{"conferma_strutturale_id", "'{C1}'"}, {"firma_piano", "repeat('f', 64)"}, {"requisiti", "'[]'"}, {"attestato_da", "'{U}'"}},
	"attestazione_fascicolo_documento": {{"attestazione_id", "'{AT1}'"}, {"thread_id", "'{T}'"}, {"documento_id", "'{D2}'"},
		{"componente_id", "'{S}'"}, {"tipo", "'disegno_2d'"}, {"rev", "'A'"}, {"sha256", "repeat('e', 64)"}},
	"deroga_fabbisogno_storia": {{"operazione", "'INSERT'"}, {"deroga_id", "gen_random_uuid()"}, {"thread_id", "'{T}'"},
		{"componente_id", "'{S}'"}, {"tipo", "'sviluppo_dxf'"}, {"prima", "NULL"}, {"dopo", "'{}'"}},
	// direttiva 11
	"distinta_bozza_registro": {{"bozza_id", "gen_random_uuid()"}, {"thread_id", "'{T}'"}, {"prodotto_id", "'{P2}'"},
		{"passo", "'aperta'"}, {"utente_id", "'{U}'"}, {"motivo", "NULL"}},
	"copertura_documento": {{"thread_id", "'{T}'"}, {"documento_id", "'{D2}'"}, {"sha256", "repeat('e', 64)"},
		{"documento_componente", "'{S}'"}, {"componente_id", "'{P2}'"}, {"tipo", "'disegno_2d'"}, {"azione", "'copre'"},
		{"precedente_id", "NULL"}, {"pagine", "'{1}'"}, {"pagine_totali", "2"}, {"rev_documento", "'A'"}, {"rev_componente", "NULL"},
		{"motivo", "'pagina 1 del disegno di S'"}, {"chiave_idempotenza", "gen_random_uuid()"}, {"autore_id", "'{U}'"}},
	"schema_variante": {{"versione", "99"}, {"variante", "'prova-99'"}},
	// revisioni della notte (RSC-05)
	"deroga_struttura_storia": {{"operazione", "'INSERT'"}, {"deroga_struttura_id", "gen_random_uuid()"}, {"thread_id", "'{T}'"},
		{"componente_id", "'{P}'"}, {"step_documento_id", "'{D}'"}, {"step_sha256", "repeat('c', 64)"}, {"riga", "'{}'"}},
}

func insertCon(tab string, cambi map[string]string) string {
	r := righeValide22[tab]
	var cols, vals []string
	for _, c := range r {
		v := c.expr
		if x, ok := cambi[c.col]; ok {
			v = x
		}
		cols = append(cols, c.col)
		vals = append(vals, v)
	}
	for c, v := range cambi { // colonne che la riga valida non nomina
		trovata := false
		for _, rc := range r {
			if rc.col == c {
				trovata = true
			}
		}
		if !trovata {
			cols = append(cols, c)
			vals = append(vals, v)
		}
	}
	return sost22.Replace(fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", tab, strings.Join(cols, ", "), strings.Join(vals, ", ")))
}

// casoVincolo: una riga valida con qualche colonna cambiata viola esattamente un vincolo. I trigger utente della
// tabella sono spenti nella transazione della prova (ALTER TABLE … DISABLE TRIGGER USER, annullato dal ROLLBACK),
// cosi' risponde il vincolo e non un trigger; senza sono i vincoli (o, con «indice:», gli indici unici) da togliere
// nella transazione per isolare un FK che un altro vincolo fermerebbe prima (un FK su thread_id coperto da una FK
// composita, l'indice unico dell'esito della conferma n. 1); immediato controlla subito i vincoli differiti.
type casoVincolo struct {
	tab       string
	cambi     map[string]string
	atteso    string
	senza     []string
	immediato bool
}

const nuovo = "gen_random_uuid()"

var casiVincoli22 = []casoVincolo{
	{"working_contatore", map[string]string{"struttura": "-1"}, "ck_working_contatore_positivi", nil, false},

	{"distinta_bozza", map[string]string{"base_numero": "1"}, "ck_distinta_bozza_base", nil, false},
	{"distinta_bozza", map[string]string{"stato": "'abbandonata'"}, "ck_distinta_bozza_chiusura", nil, false},
	{"distinta_bozza", map[string]string{"fonte_sha256": "repeat('a', 64)"}, "ck_distinta_bozza_fonte", nil, false},
	{"distinta_bozza", map[string]string{"formato": "1"}, "ck_distinta_bozza_formato", nil, false},
	{"distinta_bozza", map[string]string{"stato": "'chiusa'", "chiusa_il": "now()", "chiusa_da": "'{U}'"}, "ck_distinta_bozza_stato", nil, false},
	{"distinta_bozza", map[string]string{"versione": "-1"}, "ck_distinta_bozza_versione", nil, false},
	{"distinta_bozza", map[string]string{"aggiornata_da": nuovo}, "distinta_bozza_aggiornata_da_fkey", nil, false},
	{"distinta_bozza", map[string]string{"stato": "'abbandonata'", "chiusa_il": "now()", "chiusa_da": nuovo}, "distinta_bozza_chiusa_da_fkey", nil, false},
	{"distinta_bozza", map[string]string{"creata_da": nuovo}, "distinta_bozza_creata_da_fkey", nil, false},
	{"distinta_bozza", map[string]string{"fonte_allegato_id": nuovo, "fonte_sha256": "repeat('a', 64)"}, "distinta_bozza_fonte_allegato_id_fkey", nil, false},
	{"distinta_bozza", map[string]string{"thread_id": nuovo}, "distinta_bozza_thread_id_fkey", []string{"distinta_bozza.fk_distinta_bozza_prodotto"}, false},
	{"distinta_bozza", map[string]string{"base_conferma_id": nuovo, "base_numero": "1"}, "fk_distinta_bozza_base", nil, false},
	{"distinta_bozza", map[string]string{"prodotto_id": nuovo}, "fk_distinta_bozza_prodotto", nil, false},
	{"distinta_bozza", map[string]string{}, "fk_distinta_bozza_versione_corrente", nil, true},

	{"distinta_bozza_versione", map[string]string{"base_numero": "1"}, "ck_dbv_base", nil, false},
	{"distinta_bozza_versione", map[string]string{"fonte_sha256": "repeat('a', 64)"}, "ck_dbv_fonte", nil, false},
	{"distinta_bozza_versione", map[string]string{"impronta": "repeat('A', 64)"}, "ck_dbv_impronta", nil, false},
	{"distinta_bozza_versione", map[string]string{"snapshot": "'[]'"}, "ck_dbv_snapshot", nil, false},
	{"distinta_bozza_versione", map[string]string{"versione": "0"}, "ck_dbv_versione", nil, false},
	{"distinta_bozza_versione", map[string]string{"bozza_id": nuovo}, "distinta_bozza_versione_bozza_id_fkey", nil, false},
	{"distinta_bozza_versione", map[string]string{"fonte_allegato_id": nuovo, "fonte_sha256": "repeat('a', 64)"}, "distinta_bozza_versione_fonte_allegato_id_fkey", nil, false},
	{"distinta_bozza_versione", map[string]string{"salvata_da": nuovo}, "distinta_bozza_versione_salvata_da_fkey", nil, false},
	{"distinta_bozza_versione", map[string]string{"base_conferma_id": nuovo, "base_numero": "1"}, "fk_dbv_base", nil, false},

	{"distinta_bozza_evento", map[string]string{"azione": "'Aggiungi'"}, "ck_dbe_azione", nil, false},
	{"distinta_bozza_evento", map[string]string{"chiave_bozza": "'k x'"}, "ck_dbe_chiave", nil, false},
	{"distinta_bozza_evento", map[string]string{"dopo": "jsonb_build_object('x', repeat('y', 300000))"}, "ck_dbe_dopo", nil, false},
	{"distinta_bozza_evento", map[string]string{"motivo": "' '"}, "ck_dbe_motivo", nil, false},
	{"distinta_bozza_evento", map[string]string{"n": "0"}, "ck_dbe_n", nil, false},
	{"distinta_bozza_evento", map[string]string{"origine": "'browser'"}, "ck_dbe_origine", nil, false},
	{"distinta_bozza_evento", map[string]string{"prima": "jsonb_build_object('x', repeat('y', 300000))"}, "ck_dbe_prima", nil, false},
	{"distinta_bozza_evento", map[string]string{"bozza_id": nuovo}, "distinta_bozza_evento_bozza_id_fkey", []string{"distinta_bozza_evento.fk_dbe_versione"}, false},
	{"distinta_bozza_evento", map[string]string{"utente_id": nuovo}, "distinta_bozza_evento_utente_id_fkey", nil, false},
	{"distinta_bozza_evento", map[string]string{"versione": "99"}, "fk_dbe_versione", nil, false},

	{"conferma_strutturale", map[string]string{"base_conferma_id": "NULL"}, "ck_conferma_base", nil, false},
	{"conferma_strutturale", map[string]string{"bozza_id": "'{B1}'"}, "ck_conferma_bozza", nil, false},
	{"conferma_strutturale", map[string]string{"chiave_idempotenza": "NULL"}, "ck_conferma_chiave", nil, false},
	{"conferma_strutturale", map[string]string{"criterio_verifica": "'manuale'"}, "ck_conferma_criterio", nil, false},
	{"conferma_strutturale", map[string]string{"firma_anteprima": "NULL"}, "ck_conferma_firme", nil, false},
	{"conferma_strutturale", map[string]string{"fonte_sha256": "repeat('a', 64)"}, "ck_conferma_fonte", nil, false},
	{"conferma_strutturale", map[string]string{"senza_riepilogo_v2": "true"}, "ck_conferma_formato", nil, false},
	{"conferma_strutturale", map[string]string{"origine": "'{}'"}, "ck_conferma_json", nil, false},
	{"conferma_strutturale", map[string]string{"mono_pezzo_motivo": "'pezzo unico'"}, "ck_conferma_mono_pezzo", nil, false},
	// RD-02: in formato 2 mono_pezzo vuole la dichiarazione nello snapshot (e viceversa)
	{"conferma_strutturale", map[string]string{"mono_pezzo": "true", "criterio_verifica": "'mono_pezzo'", "mono_pezzo_motivo": "'pezzo unico'"},
		"ck_conferma_mono_pezzo_snapshot", nil, false},
	{"conferma_strutturale", map[string]string{"numero": "1"}, "ck_conferma_numero", nil, false},
	{"conferma_strutturale", map[string]string{"snapshot": `'{"formato":1}'`}, "ck_conferma_snapshot", nil, false},
	{"conferma_strutturale", map[string]string{"confermato_da": nuovo}, "conferma_strutturale_confermato_da_fkey", nil, false},
	{"conferma_strutturale", map[string]string{"fonte_allegato_id": nuovo, "fonte_sha256": "repeat('a', 64)"}, "conferma_strutturale_fonte_allegato_id_fkey", nil, false},
	{"conferma_strutturale", map[string]string{"thread_id": nuovo}, "conferma_strutturale_thread_id_fkey", []string{"conferma_strutturale.fk_conferma_prodotto"}, false},
	{"conferma_strutturale", map[string]string{"formato": "1", "senza_riepilogo_v2": "true", "chiave_idempotenza": "NULL", "base_conferma_id": nuovo}, "fk_conferma_base", nil, false},
	{"conferma_strutturale", map[string]string{"bozza_id": nuovo, "bozza_versione": "1"}, "fk_conferma_bozza", []string{"conferma_strutturale.fk_conferma_bozza_versione"}, false},
	{"conferma_strutturale", map[string]string{"bozza_id": "'{B1}'", "bozza_versione": "99"}, "fk_conferma_bozza_versione", []string{"conferma_strutturale.fk_conferma_bozza"}, false},
	{"conferma_strutturale", map[string]string{"formato": "1", "senza_riepilogo_v2": "true", "chiave_idempotenza": "NULL", "base_conferma_id": "NULL", "precedente_id": nuovo}, "fk_conferma_precedente", nil, false},
	{"conferma_strutturale", map[string]string{"prodotto_id": nuovo, "precedente_id": "NULL", "base_conferma_id": "NULL", "numero": "1",
		"formato": "1", "senza_riepilogo_v2": "true", "chiave_idempotenza": "NULL"}, "fk_conferma_prodotto", nil, false},

	{"conferma_strutturale_componente", map[string]string{"chiave_bozza": "'k x'"}, "ck_csc_chiave", nil, false},
	{"conferma_strutturale_componente", map[string]string{"step_strutturale_id": "'{D}'"}, "ck_csc_step", nil, false},
	{"conferma_strutturale_componente", map[string]string{"componente_id": nuovo}, "fk_csc_componente", nil, false},
	{"conferma_strutturale_componente", map[string]string{"conferma_id": nuovo}, "fk_csc_conferma", nil, false},

	{"conferma_strutturale_arco", map[string]string{"qta": "0"}, "ck_csa_arco", nil, false},
	{"conferma_strutturale_arco", map[string]string{"origine": "'browser'"}, "ck_csa_origine", nil, false},
	{"conferma_strutturale_arco", map[string]string{"figlio_id": nuovo}, "fk_csa_figlio", nil, false},
	{"conferma_strutturale_arco", map[string]string{"padre_id": nuovo}, "fk_csa_padre", nil, false},

	{"esito_intento", map[string]string{"gesto": "'conferma_bom_v2'"}, "ck_esito_intento_conferma", nil, false},
	{"esito_intento", map[string]string{"esito": "'[]'"}, "ck_esito_intento_esito", nil, false},
	{"esito_intento", map[string]string{"gesto": "'altro'"}, "ck_esito_intento_gesto", nil, false},
	{"esito_intento", map[string]string{"impronta_corpo": "'x'"}, "ck_esito_intento_impronta", nil, false},
	{"esito_intento", map[string]string{"thread_id": nuovo}, "esito_intento_thread_id_fkey", nil, false},
	{"esito_intento", map[string]string{"utente_id": nuovo}, "esito_intento_utente_id_fkey", nil, false},
	{"esito_intento", map[string]string{"gesto": "'conferma_bom_v2'", "conferma_id": "'{C1}'"}, "fk_esito_intento_chiave", []string{"indice:ux_esito_intento_conferma"}, false},
	{"esito_intento", map[string]string{"gesto": "'conferma_bom_v2'", "conferma_id": nuovo}, "fk_esito_intento_conferma", []string{"esito_intento.fk_esito_intento_chiave"}, false},

	{"f8_stato", map[string]string{"stato": "'errore'"}, "ck_f8_errore", nil, false},
	{"f8_stato", map[string]string{"chiave_intento": "NULL"}, "ck_f8_oggetto", nil, false},
	{"f8_stato", map[string]string{"stato": "'eseguito'"}, "ck_f8_risolto", nil, false},
	{"f8_stato", map[string]string{"stato": "'boh'"}, "ck_f8_stato", nil, false},
	{"f8_stato", map[string]string{"tentativi": "-1"}, "ck_f8_tentativi", nil, false},
	{"f8_stato", map[string]string{"thread_id": nuovo}, "f8_stato_thread_id_fkey", nil, false},
	{"f8_stato", map[string]string{"conferma_id": nuovo}, "fk_f8_conferma", nil, false},

	{"correzione_documento", map[string]string{"origine": "'motore_a'"}, "ck_correzione_motivo", nil, false},
	{"correzione_documento", map[string]string{"proposta_id": "NULL"}, "ck_correzione_oggetto", nil, false},
	{"correzione_documento", map[string]string{"origine": "'altro'"}, "ck_correzione_origine", nil, false},
	{"correzione_documento", map[string]string{"sha256": "repeat('D', 64)"}, "ck_correzione_sha", nil, false},
	{"correzione_documento", map[string]string{"allegato_id": nuovo}, "correzione_documento_allegato_id_fkey", nil, false},
	{"correzione_documento", map[string]string{"autore_id": nuovo}, "correzione_documento_autore_id_fkey", nil, false},
	{"correzione_documento", map[string]string{"proposta_id": nuovo}, "correzione_documento_proposta_id_fkey", nil, false},
	{"correzione_documento", map[string]string{"thread_id": nuovo}, "correzione_documento_thread_id_fkey", nil, false},
	{"correzione_documento", map[string]string{"documento_id": nuovo}, "fk_correzione_documento", nil, false},

	{"associazione_evento", map[string]string{"azione": "'fondi'"}, "ck_associazione_azione", nil, false},
	{"associazione_evento", map[string]string{"azione": "'assegna'"}, "ck_associazione_coerente", nil, false},
	{"associazione_evento", map[string]string{"motivo": "' '"}, "ck_associazione_motivo", nil, false},
	{"associazione_evento", map[string]string{"proposta_id": "NULL"}, "ck_associazione_oggetto", nil, false},
	{"associazione_evento", map[string]string{"origine": "'altro'"}, "ck_associazione_origine", nil, false},
	{"associazione_evento", map[string]string{"autore_id": nuovo}, "associazione_evento_autore_id_fkey", nil, false},
	{"associazione_evento", map[string]string{"proposta_id": nuovo}, "associazione_evento_proposta_id_fkey", nil, false},
	{"associazione_evento", map[string]string{"thread_id": nuovo}, "associazione_evento_thread_id_fkey", nil, false},
	{"associazione_evento", map[string]string{"conferma_id": nuovo}, "fk_associazione_conferma", nil, false},
	{"associazione_evento", map[string]string{"documento_id": nuovo}, "fk_associazione_documento", nil, false},

	{"fonte_step_evento", map[string]string{"azione": "'fondi'"}, "ck_fonte_azione", nil, false},
	{"fonte_step_evento", map[string]string{"azione": "'autorizzazione'"}, "ck_fonte_documento", nil, false},
	{"fonte_step_evento", map[string]string{"gruppo": "' '"}, "ck_fonte_gruppo", nil, false},
	{"fonte_step_evento", map[string]string{"azione": "'scelta'"}, "ck_fonte_gruppo_obbligatorio", nil, false},
	{"fonte_step_evento", map[string]string{"azione": "'rifiuto'", "gruppo": "'bom'"}, "ck_fonte_motivo", nil, false},
	{"fonte_step_evento", map[string]string{"sha256": "'x'"}, "ck_fonte_sha", nil, false},
	{"fonte_step_evento", map[string]string{"azione": "'sostituzione'", "gruppo": "'bom'", "motivo": "'fonte sbagliata'"}, "ck_fonte_sostituzione", nil, false},
	{"fonte_step_evento", map[string]string{"documento_id": nuovo}, "fk_fonte_documento", nil, false},
	{"fonte_step_evento", map[string]string{"prodotto_id": nuovo}, "fk_fonte_prodotto", nil, false},
	{"fonte_step_evento", map[string]string{"allegato_id": nuovo}, "fonte_step_evento_allegato_id_fkey", nil, false},
	{"fonte_step_evento", map[string]string{"autore_id": nuovo}, "fonte_step_evento_autore_id_fkey", nil, false},
	{"fonte_step_evento", map[string]string{"precedente_allegato_id": nuovo, "precedente_sha256": "repeat('b', 64)"}, "fonte_step_evento_precedente_allegato_id_fkey", nil, false},
	{"fonte_step_evento", map[string]string{"proposta_id": nuovo}, "fonte_step_evento_proposta_id_fkey", nil, false},
	{"fonte_step_evento", map[string]string{"thread_id": nuovo}, "fonte_step_evento_thread_id_fkey", []string{"fonte_step_evento.fk_fonte_prodotto"}, false},

	{"attestazione_fascicolo", map[string]string{"firma_piano": "'x'"}, "ck_attestazione_firma", nil, false},
	{"attestazione_fascicolo", map[string]string{"numero": "1"}, "ck_attestazione_numero", nil, false},
	{"attestazione_fascicolo", map[string]string{"requisiti": "'{}'"}, "ck_attestazione_requisiti", nil, false},
	{"attestazione_fascicolo", map[string]string{"attestato_da": nuovo}, "attestazione_fascicolo_attestato_da_fkey", nil, false},
	{"attestazione_fascicolo", map[string]string{"thread_id": nuovo}, "attestazione_fascicolo_thread_id_fkey", []string{"attestazione_fascicolo.fk_attestazione_prodotto"}, false},
	{"attestazione_fascicolo", map[string]string{"conferma_strutturale_id": nuovo}, "fk_attestazione_conferma", nil, false},
	{"attestazione_fascicolo", map[string]string{"precedente_id": nuovo}, "fk_attestazione_precedente", nil, false},
	{"attestazione_fascicolo", map[string]string{"prodotto_id": nuovo}, "fk_attestazione_prodotto",
		[]string{"attestazione_fascicolo.fk_attestazione_conferma", "attestazione_fascicolo.fk_attestazione_precedente"}, false},

	{"attestazione_fascicolo_documento", map[string]string{"sha256": "'x'"}, "ck_afd_sha", nil, false},
	{"attestazione_fascicolo_documento", map[string]string{"attestazione_id": nuovo}, "fk_afd_attestazione", nil, false},
	{"attestazione_fascicolo_documento", map[string]string{"documento_id": nuovo}, "fk_afd_documento", nil, false},

	{"deroga_fabbisogno_storia", map[string]string{"operazione": "'MERGE'"}, "ck_dfs_operazione", nil, false},
	{"deroga_fabbisogno_storia", map[string]string{"dopo": "'[]'"}, "ck_dfs_righe", nil, false},

	// direttiva 11: fonti secondarie (D-SHOCK-S2), registro delle bozze (P-04a), coperture (DOC-01), variante
	{"fonte_step_evento", map[string]string{"ruolo": "'sottoassieme'", "azione": "'scelta'", "gruppo": "'bom'", "componente_id": nuovo}, "fk_fonte_componente", nil, false},
	{"fonte_step_evento", map[string]string{"ruolo": "'sottoassieme'", "azione": "'risoluzione'", "componente_id": "'{S}'", "chiave_bozza": "'k-s'",
		"destinazione": `'{"versione": 1, "nodo": {}}'`, "bozza_id": nuovo, "risolve_evento_id": "999999"}, "fk_fonte_risolve", nil, false},
	// RD-01: la destinazione temporanea porta la copia del nodo
	{"fonte_step_evento", map[string]string{"ruolo": "'sottoassieme'", "azione": "'rifiuto'", "gruppo": "'bom'", "motivo": "'altro'",
		"chiave_bozza": "'k-n'", "bozza_id": nuovo}, "ck_fonte_destinazione", nil, false},
	{"fonte_step_evento", map[string]string{"ruolo": "'sottoassieme'", "azione": "'scelta'", "gruppo": "'bom'", "componente_id": "'{S}'",
		"chiave_bozza": "'k x'", "bozza_id": nuovo}, "ck_fonte_chiave", nil, false},
	{"fonte_step_evento", map[string]string{"ruolo": "'sottoassieme'"}, "ck_fonte_ruolo", nil, false},
	{"fonte_step_evento", map[string]string{"risolve_evento_id": "1"}, "ck_fonte_risoluzione", nil, false},

	{"distinta_bozza_registro", map[string]string{"passo": "'salvata'"}, "ck_dbr_passo", nil, false},
	{"distinta_bozza_registro", map[string]string{"utente_id": "NULL"}, "ck_dbr_utente", nil, false},
	{"distinta_bozza_registro", map[string]string{"motivo": "' '"}, "ck_dbr_motivo", nil, false},
	{"distinta_bozza_registro", map[string]string{"utente_id": nuovo}, "distinta_bozza_registro_utente_id_fkey", nil, false},
	{"distinta_bozza_registro", map[string]string{"prodotto_id": nuovo}, "fk_dbr_prodotto", nil, false},

	{"copertura_documento", map[string]string{"azione": "'sposta'"}, "ck_copertura_azione", nil, false},
	{"copertura_documento", map[string]string{"tipo": "'cad_3d'"}, "ck_copertura_tipo", nil, false},
	{"copertura_documento", map[string]string{"componente_id": "'{S}'"}, "ck_copertura_altro_componente", nil, false},
	{"copertura_documento", map[string]string{"pagine": "'{3}'"}, "ck_copertura_pagine", nil, false},
	{"copertura_documento", map[string]string{"azione": "'revoca'", "pagine": "NULL", "pagine_totali": "NULL", "motivo": "'non serve piu'''"}, "ck_copertura_catena", nil, false},
	{"copertura_documento", map[string]string{"motivo": "' '"}, "ck_copertura_motivo", nil, false},
	{"copertura_documento", map[string]string{"sha256": "'x'"}, "ck_copertura_sha", nil, false},
	{"copertura_documento", map[string]string{"thread_id": nuovo}, "copertura_documento_thread_id_fkey",
		[]string{"copertura_documento.fk_copertura_documento", "copertura_documento.fk_copertura_componente"}, false},
	{"copertura_documento", map[string]string{"autore_id": nuovo}, "copertura_documento_autore_id_fkey", nil, false},
	{"copertura_documento", map[string]string{"documento_id": nuovo}, "fk_copertura_documento", nil, false},
	{"copertura_documento", map[string]string{"componente_id": nuovo}, "fk_copertura_componente", nil, false},
	{"copertura_documento", map[string]string{"azione": "'rettifica'", "precedente_id": nuovo, "motivo": "'pagine sbagliate'"}, "fk_copertura_precedente", nil, false},

	{"schema_variante", map[string]string{"variante": "'Variante Due'"}, "ck_schema_variante", nil, false},

	{"deroga_struttura_storia", map[string]string{"operazione": "'UPDATE'"}, "ck_dss_operazione", nil, false},
	{"deroga_struttura_storia", map[string]string{"riga": "'[]'"}, "ck_dss_riga", nil, false},
}

func vincoloViolato(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.ConstraintName
	}
	return ""
}

// Ogni vincolo FK e CHECK delle 15 tabelle ha una prova che lo viola da solo; il controllo finale confronta con il
// catalogo, cosi' un vincolo nuovo senza prova fa fallire la prova.
func TestP4OgniVincoloFKECheckDella0022(t *testing.T) {
	p := schema22ConStoria(t)
	ctx := context.Background()
	// prima la riga valida di ogni tabella passa (trigger spenti): un caso fallisce per il suo vincolo, non per la base
	for tab := range righeValide22 {
		tx, err := p.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, "ALTER TABLE "+tab+" DISABLE TRIGGER USER"); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, insertCon(tab, nil)); err != nil {
			t.Errorf("la riga valida di %s non passa: %v", tab, err)
		}
		_ = tx.Rollback(ctx)
	}
	provati := map[string]bool{}
	for _, c := range casiVincoli22 {
		t.Run(c.atteso, func(t *testing.T) {
			tx, err := p.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx) //nolint:errcheck
			if _, err := tx.Exec(ctx, "ALTER TABLE "+c.tab+" DISABLE TRIGGER USER"); err != nil {
				t.Fatal(err)
			}
			for _, s := range c.senza {
				ddl := ""
				if nome, ok := strings.CutPrefix(s, "indice:"); ok {
					ddl = "DROP INDEX " + nome
				} else {
					tv := strings.SplitN(s, ".", 2)
					ddl = "ALTER TABLE " + tv[0] + " DROP CONSTRAINT " + tv[1]
				}
				if _, err := tx.Exec(ctx, ddl); err != nil {
					t.Fatal(err)
				}
			}
			if c.immediato {
				if _, err := tx.Exec(ctx, "SET CONSTRAINTS "+c.atteso+" IMMEDIATE"); err != nil {
					t.Fatal(err)
				}
			}
			_, err = tx.Exec(ctx, insertCon(c.tab, c.cambi))
			if got := vincoloViolato(err); got != c.atteso {
				t.Fatalf("%s: atteso il vincolo %s, ottenuto %q (%v)", c.tab, c.atteso, got, err)
			}
			provati[c.atteso] = true
		})
	}
	righe, err := p.Query(ctx, `SELECT conname::text FROM pg_constraint
		WHERE connamespace = 'public'::regnamespace AND contype IN ('f', 'c') AND conrelid::regclass::text = ANY($1) ORDER BY 1`, tabelle22)
	if err != nil {
		t.Fatal(err)
	}
	tutti, err := pgx.CollectRows(righe, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	var mancano []string
	for _, n := range tutti {
		if !provati[n] {
			mancano = append(mancano, n)
		}
	}
	if len(mancano) > 0 {
		t.Errorf("vincoli FK/CHECK della 0022 senza una prova passata (%d su %d): %s", len(mancano), len(tutti), strings.Join(mancano, ", "))
	}
	t.Logf("vincoli FK e CHECK della 0022 provati: %d su %d", len(tutti)-len(mancano), len(tutti))
}

// ------------------------------------------------------------------ il contatore e tutti gli scrittori

type contatori struct{ s, d int64 }

func leggiContatori(t *testing.T, q *db.Queries, thread uuid.UUID) contatori {
	t.Helper()
	r, err := q.GetWorkingContatore(context.Background(), thread)
	if err != nil {
		t.Fatal(err)
	}
	return contatori{r.Struttura, r.Dati}
}

func testo22(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

// Ogni scrittore di componente e componente_relazione (nota B1 §4.1-4.2: le query sqlc legacy, piu' l'SQL diretto)
// muove l'asse giusto, una volta sola per transazione; un UPDATE che non cambia niente non muove niente.
func TestP4IlContatoreVedeOgniScrittore(t *testing.T) {
	p := schema22ConStoria(t)
	ctx := context.Background()
	T, P, S, P2, P3, U, D := u22("{T}"), u22("{P}"), u22("{S}"), u22("{P2}"), u22("{P3}"), u22("{U}"), u22("{D}")
	casi := []struct {
		nome   string
		atteso contatori
		fai    func(q *db.Queries, tx pgx.Tx) error
	}{
		{"InsertComponente nuovo", contatori{1, 0}, func(q *db.Queries, _ pgx.Tx) error {
			_, err := q.InsertComponente(ctx, db.InsertComponenteParams{ThreadID: T, Codice: "ACM-5000", Qta: 1, Tipo: db.TipoComponenteSciolto,
				Origine: db.OrigineComponenteManuale, ConfermatoDa: U})
			return err
		}},
		{"InsertComponente gia' esistente, descrizione uguale", contatori{0, 0}, func(q *db.Queries, _ pgx.Tx) error {
			_, err := q.InsertComponente(ctx, db.InsertComponenteParams{ThreadID: T, Codice: "acm-1100", Descrizione: testo22("Telaio"), Qta: 1,
				Tipo: db.TipoComponenteSottoassieme, Origine: db.OrigineComponenteManuale, ConfermatoDa: U})
			return err
		}},
		{"InsertComponente gia' esistente, descrizione nuova", contatori{0, 1}, func(q *db.Queries, _ pgx.Tx) error {
			_, err := q.InsertComponente(ctx, db.InsertComponenteParams{ThreadID: T, Codice: "ACM-1100", Descrizione: testo22("Telaio saldato"), Qta: 1,
				Tipo: db.TipoComponenteSottoassieme, Origine: db.OrigineComponenteManuale, ConfermatoDa: U})
			return err
		}},
		{"SetCodiceComponente", contatori{1, 0}, func(q *db.Queries, _ pgx.Tx) error {
			return q.SetCodiceComponente(ctx, db.SetCodiceComponenteParams{ComponenteID: P3, Codice: "ACM-3001"})
		}},
		{"SetCodiceComponente solo maiuscole", contatori{1, 0}, func(q *db.Queries, _ pgx.Tx) error {
			return q.SetCodiceComponente(ctx, db.SetCodiceComponenteParams{ComponenteID: P3, Codice: "acm-3000"})
		}},
		{"SetCodiceComponente uguale", contatori{0, 0}, func(q *db.Queries, _ pgx.Tx) error {
			return q.SetCodiceComponente(ctx, db.SetCodiceComponenteParams{ComponenteID: P3, Codice: "ACM-3000"})
		}},
		{"SetTipoComponente", contatori{1, 0}, func(q *db.Queries, _ pgx.Tx) error {
			return q.SetTipoComponente(ctx, db.SetTipoComponenteParams{ComponenteID: S, Tipo: db.TipoComponenteSciolto, ConfermatoDa: U})
		}},
		{"SetTipoComponente uguale", contatori{0, 0}, func(q *db.Queries, _ pgx.Tx) error {
			return q.SetTipoComponente(ctx, db.SetTipoComponenteParams{ComponenteID: S, Tipo: db.TipoComponenteSottoassieme, ConfermatoDa: U})
		}},
		{"SetDatiComponente rev", contatori{0, 1}, func(q *db.Queries, _ pgx.Tx) error {
			_, err := q.SetDatiComponente(ctx, db.SetDatiComponenteParams{ComponenteID: S, Tipo: db.TipoComponenteSottoassieme, Rev: testo22("B"),
				Descrizione: testo22("Telaio"), ConfermatoDa: U})
			return err
		}},
		{"SetDatiComponente tipo e descrizione", contatori{1, 1}, func(q *db.Queries, _ pgx.Tx) error {
			_, err := q.SetDatiComponente(ctx, db.SetDatiComponenteParams{ComponenteID: S, Tipo: db.TipoComponenteSciolto, Rev: testo22("A"),
				Descrizione: testo22("Telaio saldato"), ConfermatoDa: U})
			return err
		}},
		{"SetDatiComponente uguale", contatori{0, 0}, func(q *db.Queries, _ pgx.Tx) error {
			_, err := q.SetDatiComponente(ctx, db.SetDatiComponenteParams{ComponenteID: S, Tipo: db.TipoComponenteSottoassieme, Rev: testo22("A"),
				Descrizione: testo22("Telaio"), ConfermatoDa: U})
			return err
		}},
		{"SetNoteComponente (fuori dal contatore)", contatori{0, 0}, func(q *db.Queries, _ pgx.Tx) error {
			return q.SetNoteComponente(ctx, db.SetNoteComponenteParams{ComponenteID: S, NoteFattibilita: testo22("nota"),
				EsitoFattibilita: db.NullEsitoFattibilita{}})
		}},
		{"ArchiviaComponente", contatori{1, 0}, func(q *db.Queries, _ pgx.Tx) error {
			_, err := q.ArchiviaComponente(ctx, db.ArchiviaComponenteParams{ArchiviatoDa: uuid.NullUUID{UUID: U, Valid: true}, Motivo: testo22("prova"), ComponenteID: P3})
			return err
		}},
		{"ArchiviaComponente e RipristinaComponente nella stessa transazione", contatori{1, 0}, func(q *db.Queries, _ pgx.Tx) error {
			if _, err := q.ArchiviaComponente(ctx, db.ArchiviaComponenteParams{ArchiviatoDa: uuid.NullUUID{UUID: U, Valid: true}, Motivo: testo22("prova"), ComponenteID: P3}); err != nil {
				return err
			}
			_, err := q.RipristinaComponente(ctx, P3)
			return err
		}},
		{"SetStepStrutturale", contatori{1, 0}, func(q *db.Queries, _ pgx.Tx) error {
			_, err := q.SetStepStrutturale(ctx, db.SetStepStrutturaleParams{StepStrutturaleID: uuid.NullUUID{UUID: D, Valid: true}, ComponenteID: P})
			return err
		}},
		{"DeleteComponente (con la cascata dei suoi archi)", contatori{1, 0}, func(q *db.Queries, _ pgx.Tx) error {
			_, err := q.DeleteComponente(ctx, P3)
			return err
		}},
		{"InsertRelazione", contatori{1, 0}, func(q *db.Queries, _ pgx.Tx) error {
			_, err := q.InsertRelazione(ctx, db.InsertRelazioneParams{ThreadID: T, PadreID: P2, FiglioID: P3, Qta: 3, Origine: db.OrigineComponenteManuale, ConfermatoDa: U})
			return err
		}},
		{"InsertRelazione gia' presente (DO NOTHING)", contatori{0, 0}, func(q *db.Queries, _ pgx.Tx) error {
			_, err := q.InsertRelazione(ctx, db.InsertRelazioneParams{ThreadID: T, PadreID: P, FiglioID: S, Qta: 9, Origine: db.OrigineComponenteManuale, ConfermatoDa: U})
			return err
		}},
		{"SetQtaRelazione", contatori{1, 0}, func(q *db.Queries, _ pgx.Tx) error {
			_, err := q.SetQtaRelazione(ctx, db.SetQtaRelazioneParams{Qta: 5, ConfermatoDa: U, PadreID: P, FiglioID: S})
			return err
		}},
		{"SetQtaRelazione uguale", contatori{0, 0}, func(q *db.Queries, _ pgx.Tx) error {
			_, err := q.SetQtaRelazione(ctx, db.SetQtaRelazioneParams{Qta: 2, ConfermatoDa: U, PadreID: P, FiglioID: S})
			return err
		}},
		{"DeleteRelazione", contatori{1, 0}, func(q *db.Queries, _ pgx.Tx) error {
			_, err := q.DeleteRelazione(ctx, db.DeleteRelazioneParams{PadreID: P, FiglioID: S})
			return err
		}},
		{"DeleteRelazioniComponente", contatori{1, 0}, func(q *db.Queries, _ pgx.Tx) error {
			_, err := q.DeleteRelazioniComponente(ctx, S)
			return err
		}},
		{"SQL diretto: posizione di un arco", contatori{0, 1}, func(_ *db.Queries, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sost22.Replace(`UPDATE componente_relazione SET posizione = '10' WHERE padre_id = '{P}' AND figlio_id = '{S}'`))
			return err
		}},
		{"SQL diretto: UPDATE identico su componente e arco", contatori{0, 0}, func(_ *db.Queries, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sost22.Replace(`UPDATE componente SET codice = codice, rev = rev, descrizione = descrizione, tipo = tipo WHERE thread_id = '{T}';
				UPDATE componente_relazione SET qta = qta, posizione = posizione WHERE thread_id = '{T}'`))
			return err
		}},
		// mutazione S05: una scrittura solo di struttura, poi una di struttura e dati nella stessa transazione; la
		// seconda passa la WHERE dell'UPSERT per l'asse dati, e la struttura non deve contare di nuovo
		{"SQL diretto: struttura, poi struttura e dati, una transazione", contatori{1, 1}, func(_ *db.Queries, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sost22.Replace(`UPDATE componente SET codice = 'ACM-3001' WHERE componente_id = '{P3}';
				UPDATE componente SET tipo = 'sciolto', rev = 'Z' WHERE componente_id = '{S}'`))
			return err
		}},
		// mutazione S07: un componente che cambia RFQ muove anche il contatore della RFQ che lascia
		{"SQL diretto: un componente cambia RFQ (conta anche la RFQ di partenza)", contatori{1, 0}, func(_ *db.Queries, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sost22.Replace(`UPDATE componente SET thread_id = '{T2}' WHERE componente_id = '{P3}'`))
			return err
		}},
		{"SQL diretto: tante scritture strutturali e di dati, una transazione", contatori{1, 1}, func(_ *db.Queries, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sost22.Replace(`UPDATE componente SET tipo = 'sciolto' WHERE componente_id = '{S}';
				UPDATE componente SET codice = 'ACM-2001' WHERE componente_id = '{P2}';
				UPDATE componente_relazione SET qta = 7 WHERE padre_id = '{P}' AND figlio_id = '{S}';
				UPDATE componente SET rev = 'C', descrizione = 'x' WHERE thread_id = '{T}';
				INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ('{T}', '{P2}', '{P3}', 1, 'manuale', '{U}')`))
			return err
		}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			tx, err := p.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx) //nolint:errcheck // ogni caso riparte dalle stesse righe
			q := db.New(tx)
			prima := leggiContatori(t, q, T)
			righePrima := uno22[int](t, tx, `SELECT count(*) FROM working_contatore`)
			if err := c.fai(q, tx); err != nil {
				t.Fatalf("scrittura: %v", err)
			}
			dopo := leggiContatori(t, q, T)
			if d := (contatori{dopo.s - prima.s, dopo.d - prima.d}); d != c.atteso {
				t.Errorf("contatori mossi di %+v, attesi %+v", d, c.atteso)
			}
			if c.atteso == (contatori{}) && uno22[int](t, tx, `SELECT count(*) FROM working_contatore`) != righePrima {
				t.Error("una scrittura che non conta ha creato una riga di working_contatore")
			}
		})
	}
	// un'altra RFQ non si muove
	q := db.New(p)
	if c := leggiContatori(t, q, u22("{T2}")); c.s != 1 || c.d != 0 {
		t.Errorf("la RFQ T2 ha contatori %+v: le prove sopra non l'hanno toccata e la sua nascita conta 1", c)
	}
}

// La storia delle deroghe la scrive il trigger, con l'autore dichiarato nella transazione o, per un INSERT, chi
// concede; un INSERT diretto nella storia si rifiuta.
func TestP4LaStoriaDelleDeroghe(t *testing.T) {
	p := schema22ConStoria(t)
	ctx := context.Background()
	T, S, U := u22("{T}"), u22("{S}"), u22("{U}")
	prima := testutil.Conta(t, p, "deroga_fabbisogno_storia")
	q := db.New(p)
	if _, err := q.InsertDeroga(ctx, db.InsertDerogaParams{ThreadID: T, ComponenteID: S, Tipo: db.TipoDocumentoDisegno2d, Motivo: "disegno non richiesto", UtenteID: U}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.InsertDeroga(ctx, db.InsertDerogaParams{ThreadID: T, ComponenteID: S, Tipo: db.TipoDocumentoDisegno2d, Motivo: "motivo corretto", UtenteID: U}); err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('cockpit.autore_id', $1, true)`, U.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.New(tx).DeleteDeroga(ctx, db.DeleteDerogaParams{DerogaID: u22("{DF}"), ThreadID: T}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	righe, err := p.Query(ctx, `SELECT operazione, coalesce(autore_id::text, '') FROM deroga_fabbisogno_storia ORDER BY storia_id`)
	if err != nil {
		t.Fatal(err)
	}
	var visti []string
	for righe.Next() {
		var op, autore string
		if err := righe.Scan(&op, &autore); err != nil {
			t.Fatal(err)
		}
		visti = append(visti, op+":"+autore)
	}
	righe.Close()
	atteso := []string{"INSERT:" + U.String(), "INSERT:" + U.String(), "UPDATE:", "DELETE:" + U.String()}
	if fmt.Sprint(visti[prima:]) != fmt.Sprint(atteso[1:]) || len(visti) != prima+3 {
		t.Errorf("storia delle deroghe %v (prima %d righe), attese in coda %v", visti, prima, atteso[1:])
	}
	// la dichiarazione era della sola transazione: non resta sulla connessione
	if v := uno22[string](t, p, `SELECT coalesce(current_setting('cockpit.autore_id', true), '')`); v != "" {
		t.Errorf("cockpit.autore_id resta sulla connessione: %q", v)
	}
	_, err = p.Exec(ctx, sost22.Replace(`INSERT INTO deroga_fabbisogno_storia (operazione, deroga_id, thread_id, componente_id, tipo, dopo) VALUES ('INSERT', gen_random_uuid(), '{T}', '{S}', 'sviluppo_dxf', '{}')`))
	if codice(err) != "P4X02" {
		t.Errorf("INSERT diretto nella storia delle deroghe: %v", err)
	}
}

// R1 della revisione DDL (il rilievo piu' importante): il contatore conta una volta per transazione, quindi dopo
// la conferma, nella stessa transazione, la working della RFQ non si scrive piu'; e la conferma porta i contatori
// correnti. In una transazione successiva la scrittura passa e muove il contatore (la conferma non e' piu' valida
// per contatore).
func TestP4LaConfermaSiScrivePerUltima(t *testing.T) {
	p := schema22ConStoria(t)
	ctx := context.Background()
	T := u22("{T}")
	numero2 := func(contatore string) string {
		return sost22.Replace(`INSERT INTO conferma_strutturale (thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2,
			firma_struttura, firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati, origine, snapshot,
			impronta_snapshot, delta, mappa, criterio_verifica, chiave_idempotenza, confermato_da)
			SELECT '{T}', '{P}', 2, '{C1}', 2, false, repeat('3', 64), repeat('2', 64), '{C1}', 0, repeat('1', 64), ` + contatore + `,
			COALESCE((SELECT dati FROM working_contatore WHERE thread_id = '{T}'), 0), '{"rotta":"prova"}',
			jsonb_build_object('formato', 2, 'prodotto_id', '{P}'::text), repeat('a', 64), '{}', '{}', 'nessuno', gen_random_uuid(), '{U}'`)
	}
	corrente := `COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = '{T}'), 0)`

	// contatore registrato diverso da quello corrente: rifiutata
	_, err := p.Exec(ctx, numero2(corrente+" + 1"))
	if codice(err) != "P4X01" || !strings.Contains(err.Error(), "contatori") {
		t.Errorf("conferma con il contatore sbagliato: %v", err)
	}

	// nella stessa transazione: scrittura, conferma con i contatori correnti, poi ancora una scrittura → P4X01
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, sost22.Replace(`UPDATE componente_relazione SET qta = 3 WHERE padre_id = '{P}' AND figlio_id = '{S}'`)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, numero2(corrente)); err != nil {
		t.Fatalf("conferma con i contatori correnti: %v", err)
	}
	_, err = tx.Exec(ctx, sost22.Replace(`UPDATE componente SET rev = 'Z' WHERE componente_id = '{S}'`))
	_ = tx.Rollback(ctx)
	if codice(err) != "P4X01" || !strings.Contains(err.Error(), "dopo una conferma strutturale") {
		t.Fatalf("scrittura della working dopo la conferma nella stessa transazione: %v", err)
	}

	// conferma completa (componenti e archi uguali alla working) nella sua transazione, poi una scrittura in una
	// transazione nuova: passa e muove il contatore
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, numero2(corrente)); err != nil {
		t.Fatal(err)
	}
	var c2 uuid.UUID
	if err := tx.QueryRow(ctx, sost22.Replace(`SELECT conferma_id FROM conferma_strutturale WHERE prodotto_id = '{P}' AND numero = 2`)).Scan(&c2); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, sost22.Replace(`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo)
		VALUES ($1, '{T}', '{P}', true, 'k-p', 'ACM-1000', 'A', 'Gruppo base', 'finito'), ($1, '{T}', '{S}', false, 'k-s', 'ACM-1100', 'A', 'Telaio', 'sottoassieme');
		`), c2); err != nil {
		t.Fatal(err)
	}
	// manca l'arco P → S: al COMMIT il perimetro non e' chiuso
	err = tx.Commit(ctx)
	if codice(err) != "P4X01" || !strings.Contains(err.Error(), "fra gli archi certificati") {
		t.Fatalf("conferma senza l'arco della working: al COMMIT atteso P4X01, ottenuto %v", err)
	}
	tx, err = p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, numero2(corrente)); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, sost22.Replace(`SELECT conferma_id FROM conferma_strutturale WHERE prodotto_id = '{P}' AND numero = 2`)).Scan(&c2); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, sost22.Replace(`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo)
		VALUES ($1, '{T}', '{P}', true, 'k-p', 'ACM-1000', 'A', 'Gruppo base', 'finito'), ($1, '{T}', '{S}', false, 'k-s', 'ACM-1100', 'A', 'Telaio', 'sottoassieme')`), c2); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, sost22.Replace(`INSERT INTO conferma_strutturale_arco (conferma_id, padre_id, figlio_id, qta, origine) VALUES ($1, '{P}', '{S}', 2, 'conferma')`), c2); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("conferma n. 2 completa: %v", err)
	}
	q := db.New(p)
	ultima, err := q.GetUltimaConfermaStrutturale(ctx, db.GetUltimaConfermaStrutturaleParams{ThreadID: T, ProdottoID: u22("{P}")})
	if err != nil || ultima.Numero != 2 || ultima.ConfermaID != c2 {
		t.Fatalf("ultima conferma: %+v %v", ultima, err)
	}
	prima := leggiContatori(t, q, T)
	if prima.s != ultima.Contatore {
		t.Fatalf("contatore corrente %d, registrato %d", prima.s, ultima.Contatore)
	}
	esegui22(t, p, `UPDATE componente_relazione SET qta = 4 WHERE padre_id = '{P}' AND figlio_id = '{S}'`)
	if dopo := leggiContatori(t, q, T); dopo.s != prima.s+1 {
		t.Errorf("dopo la conferma, in una transazione nuova, la scrittura deve muovere il contatore: %d → %d", prima.s, dopo.s)
	}
	// le letture di B1 sulla conferma: componenti, archi, storico
	comp, err := q.ListConfermaStrutturaleComponenti(ctx, db.ListConfermaStrutturaleComponentiParams{ConfermaID: c2, ThreadID: T})
	if err != nil || len(comp) != 2 || !comp[0].Radice || comp[0].ComponenteID != u22("{P}") {
		t.Errorf("componenti della conferma: %+v %v", comp, err)
	}
	ar, err := q.ListConfermaStrutturaleArchi(ctx, db.ListConfermaStrutturaleArchiParams{ConfermaID: c2, ThreadID: T})
	if err != nil || len(ar) != 1 || ar[0].Qta != 2 {
		t.Errorf("archi della conferma: %+v %v", ar, err)
	}
	if ar, _ := q.ListConfermaStrutturaleArchi(ctx, db.ListConfermaStrutturaleArchiParams{ConfermaID: c2, ThreadID: u22("{T2}")}); len(ar) != 0 {
		t.Error("gli archi di una conferma si leggono da un'altra RFQ")
	}
	storico, err := q.ListConfermeStrutturali(ctx, db.ListConfermeStrutturaliParams{ThreadID: T, ProdottoID: u22("{P}")})
	if err != nil || len(storico) != 2 || storico[0].Numero != 1 || storico[1].Numero != 2 {
		t.Errorf("storico delle conferme: %d righe, %v", len(storico), err)
	}
	piena, err := q.GetConfermaStrutturale(ctx, db.GetConfermaStrutturaleParams{ThreadID: T, ConfermaID: u22("{C1}")})
	if err != nil || piena.Numero != 1 || string(piena.Snapshot) == "" || piena.CriterioVerifica != "nessuno" {
		t.Errorf("conferma intera: %+v %v", piena, err)
	}
	if _, err := q.GetConfermaStrutturale(ctx, db.GetConfermaStrutturaleParams{ThreadID: u22("{T2}"), ConfermaID: u22("{C1}")}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("una conferma si legge da un'altra RFQ: %v", err)
	}
}

// D-P4B1-01 B [U] (messaggio 10): mono_pezzo si dichiara anche dal formato 1. D-P4A-02 A [U] (direttiva 11): al
// COMMIT il database vuole la radice certificata di tipo finito, con lo STEP autorizzato, senza figli ne' archi.
// Positivo: P2 con il suo STEP autorizzato, formato 1. Negativi: senza STEP (al COMMIT); con un figlio nella working
// (alla nascita); con un altro componente certificato (al COMMIT); radice non finita e figlio nella working arrivati
// al COMMIT con i trigger di nascita spenti nella transazione (il controllo al COMMIT e' una rete in piu': qui si
// prova che c'e' davvero).
func TestP4MonoPezzoAlCommit(t *testing.T) {
	p := schema22ConStoria(t)
	ctx := context.Background()
	// lo STEP di P2, confermato e autorizzato
	esegui22(t, p, `INSERT INTO documento (documento_id, thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ('00000000-0000-4000-8000-0000002200d3', '{T}', '{P2}', 'cad_3d', 'ACM-2000', 'ACM-2000.stp', 'stp', repeat('7', 64), 'ACME22/ACM-2000.stp', '{U}');
		UPDATE componente SET step_strutturale_id = '00000000-0000-4000-8000-0000002200d3' WHERE componente_id = '{P2}'`)
	testata := func(prodotto string) string {
		return sost22.Replace(`INSERT INTO conferma_strutturale (thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura,
				base_contatore, contatore, contatore_dati, origine, snapshot, impronta_snapshot, delta, mappa, mono_pezzo,
				mono_pezzo_motivo, criterio_verifica, confermato_da)
			 SELECT '{T}', '` + prodotto + `', 1, 1, true, repeat('3', 64), 0,
				COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = '{T}'), 0),
				COALESCE((SELECT dati FROM working_contatore WHERE thread_id = '{T}'), 0),
				'{"rotta":"distinta/albero/conferma"}', '{}', repeat('a', 64), '{}', '{}', true, 'piastra tagliata al laser, pezzo unico',
				'mono_pezzo', '{U}'`)
	}
	radice := func(prodotto, codice, rev, descr, tipo, step, sha string) string {
		return sost22.Replace(`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, codice, rev, descrizione, tipo, step_strutturale_id, step_sha256)
			 SELECT conferma_id, '{T}', prodotto_id, true, '` + codice + `', ` + rev + `, ` + descr + `, '` + tipo + `', ` + step + `, ` + sha + `
			   FROM conferma_strutturale WHERE prodotto_id = '` + prodotto + `'`)
	}
	conStep := radice("{P2}", "ACM-2000", "NULL", "'Piastra'", "finito", "'00000000-0000-4000-8000-0000002200d3'", "repeat('7', 64)")
	// i trigger spenti in una prova restano spenti fino alla fine della sua transazione (con un trigger differito in
	// coda la tabella non si puo' riaccendere): il ROLLBACK, o il COMMIT rifiutato, li riaccende; dopo ogni prova li
	// si riaccende comunque, per non lasciarli spenti se una prova fallisce passando
	prova := func(nome string, istruzioni []string, codiceAtteso, frase string) {
		t.Helper()
		defer esegui22(t, p, `ALTER TABLE conferma_strutturale ENABLE TRIGGER USER`)
		tx, err := p.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		for _, s := range istruzioni {
			if _, err := tx.Exec(ctx, s); err != nil {
				if codice(err) == codiceAtteso && strings.Contains(err.Error(), frase) && codiceAtteso != "" {
					return
				}
				t.Fatalf("%s: %v\n%s", nome, err, s)
			}
		}
		err = tx.Commit(ctx)
		if codiceAtteso == "" {
			if err != nil {
				t.Fatalf("%s: al COMMIT %v", nome, err)
			}
			return
		}
		if codice(err) != codiceAtteso || !strings.Contains(err.Error(), frase) {
			t.Fatalf("%s: atteso %s («%s»), ottenuto %v", nome, codiceAtteso, frase, err)
		}
	}

	// negativi prima del positivo (dopo il positivo P2 ha gia' la conferma n. 1)
	prova("senza lo STEP sulla radice certificata", []string{testata("{P3}"), radice("{P3}", "ACM-3000", "NULL", "NULL", "finito", "NULL", "NULL")},
		"P4X01", "mono_pezzo senza lo STEP autorizzato")
	prova("con un altro componente certificato", []string{testata("{P2}"), conStep,
		sost22.Replace(`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, codice, descrizione, tipo)
			SELECT conferma_id, '{T}', '{P3}', false, 'ACM-3000', NULL, 'finito' FROM conferma_strutturale WHERE prodotto_id = '{P2}'`)},
		"P4X01", "mono_pezzo, ma al COMMIT")
	// radice non finita: la nascita la rifiuta gia' (S12); con il trigger di nascita spento arriva al COMMIT
	prova("radice non finita (nascita)", []string{testata("{S}")},
		"P4X01", "non un prodotto finito")
	prova("radice non finita (al COMMIT)", []string{
		`ALTER TABLE conferma_strutturale DISABLE TRIGGER trg_conferma_strutturale_nasce`,
		testata("{S}"),
		radice("{S}", "ACM-1100", "'A'", "'Telaio'", "sottoassieme", "NULL", "NULL")},
		"P4X01", "non un finito")
	// un figlio nella working: alla nascita (P3 riceve un figlio), e al COMMIT con la nascita spenta
	esegui22(t, p, `INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ('{T}', '{P3}', '{S}', 1, 'manuale', '{U}')`)
	prova("con un figlio nella working (nascita)", []string{testata("{P3}")}, "P4X01", "mono_pezzo dichiarato ma il prodotto ha figli")
	esegui22(t, p, `INSERT INTO documento (documento_id, thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ('00000000-0000-4000-8000-0000002200d4', '{T}', '{P3}', 'cad_3d', 'ACM-3000', 'ACM-3000.stp', 'stp', repeat('8', 64), 'ACME22/ACM-3000.stp', '{U}');
		UPDATE componente SET step_strutturale_id = '00000000-0000-4000-8000-0000002200d4' WHERE componente_id = '{P3}'`)
	prova("con un figlio nella working (al COMMIT)", []string{
		`ALTER TABLE conferma_strutturale DISABLE TRIGGER trg_conferma_strutturale_nasce`,
		testata("{P3}"),
		radice("{P3}", "ACM-3000", "NULL", "NULL", "finito", "'00000000-0000-4000-8000-0000002200d4'", "repeat('8', 64)")},
		"P4X01", "mono_pezzo, ma al COMMIT")

	// positivo: P2 finito, senza figli, con lo STEP autorizzato sulla radice certificata, formato 1
	prova("positivo", []string{testata("{P2}"), conStep}, "", "")
	q := db.New(p)
	c, err := q.GetUltimaConfermaStrutturale(ctx, db.GetUltimaConfermaStrutturaleParams{ThreadID: u22("{T}"), ProdottoID: u22("{P2}")})
	if err != nil || !c.MonoPezzo || c.CriterioVerifica != "mono_pezzo" || c.Formato != 1 || !c.SenzaRiepilogoV2 || c.FonteAllegatoID.Valid {
		t.Errorf("conferma mono_pezzo formato 1 riletta: %+v %v", c, err)
	}
	// il tipo del componente resta finito: nessun quinto valore di tipo_componente
	if tp := uno22[string](t, p, `SELECT tipo::text FROM componente WHERE componente_id = '{P2}'`); tp != "finito" {
		t.Errorf("dopo la conferma mono_pezzo il tipo di P2 e' %q, atteso finito", tp)
	}
	if n := uno22[int](t, p, `SELECT count(*) FROM pg_enum WHERE enumtypid = 'tipo_componente'::regtype`); n != 4 {
		t.Errorf("tipo_componente ha %d valori: la 0022 non ne aggiunge", n)
	}
}

// esito_intento: scrittura e lettura con le query di B1, perimetro della RFQ, chiave unica.
func TestP4LEsitoDegliIntenti(t *testing.T) {
	p := schema22ConStoria(t)
	ctx := context.Background()
	q := db.New(p)
	T, U := u22("{T}"), u22("{U}")
	k := uuid.New()
	creato, err := q.InsertEsitoIntento(ctx, db.InsertEsitoIntentoParams{Chiave: k, ThreadID: T, Gesto: "lotto_fascicolo", UtenteID: U,
		ImprontaCorpo: strings.Repeat("9", 64), Esito: []byte(`{"stato":"registrato","voci":3}`)})
	if err != nil || creato.IsZero() {
		t.Fatalf("InsertEsitoIntento: %v", err)
	}
	e, err := q.GetEsitoIntento(ctx, db.GetEsitoIntentoParams{Chiave: k, ThreadID: T})
	if err != nil || e.Gesto != "lotto_fascicolo" || e.ConfermaID.Valid || !strings.Contains(string(e.Esito), `"voci": 3`) {
		t.Errorf("GetEsitoIntento: %+v %v", e, err)
	}
	if _, err := q.GetEsitoIntento(ctx, db.GetEsitoIntentoParams{Chiave: k, ThreadID: u22("{T2}")}); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("un esito si legge da un'altra RFQ: %v", err)
	}
	_, err = q.InsertEsitoIntento(ctx, db.InsertEsitoIntentoParams{Chiave: k, ThreadID: T, Gesto: "lotto_fascicolo", UtenteID: U,
		ImprontaCorpo: strings.Repeat("9", 64), Esito: []byte(`{}`)})
	if vincoloViolato(err) != "esito_intento_pkey" {
		t.Errorf("due esiti con la stessa chiave: %v", err)
	}
	conf, err := q.GetEsitoIntento(ctx, db.GetEsitoIntentoParams{Chiave: u22("{K1}"), ThreadID: T})
	if err != nil || conf.ConfermaID.UUID != u22("{C1}") {
		t.Errorf("esito della conferma n. 1: %+v %v", conf, err)
	}
}

// ------------------------------------------------------------------ la storia non si modifica e non si cancella

func TestP4LaStoriaNonSiModificaENonSiCancella(t *testing.T) {
	p := schema22ConStoria(t)
	ctx := context.Background()
	casi := []struct {
		nome, sql, codice string
	}{
		{"UPDATE di una conferma", `UPDATE conferma_strutturale SET mono_pezzo = mono_pezzo`, "P4X01"},
		{"DELETE di una conferma", `DELETE FROM conferma_strutturale`, "P4X01"},
		{"TRUNCATE delle conferme", `TRUNCATE conferma_strutturale CASCADE`, "P4X01"},
		{"UPDATE di un componente certificato", `UPDATE conferma_strutturale_componente SET codice = 'X'`, "P4X01"},
		{"DELETE di un componente certificato", `DELETE FROM conferma_strutturale_componente`, "P4X01"},
		{"UPDATE di un arco certificato", `UPDATE conferma_strutturale_arco SET qta = 9`, "P4X01"},
		{"DELETE di un arco certificato", `DELETE FROM conferma_strutturale_arco`, "P4X01"},
		{"TRUNCATE degli archi certificati", `TRUNCATE conferma_strutturale_arco`, "P4X01"},
		// la riga e' identica alla working (codice, rev, descrizione, tipo): risponde solo il controllo della
		// transazione (mutazione S25)
		{"un componente nuovo in una conferma gia' registrata", `INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, chiave_bozza, codice, descrizione, tipo)
			VALUES ('{C1}', '{T}', '{P2}', 'k-p2', 'ACM-2000', 'Piastra', 'finito')`, "P4X01"},
		{"UPDATE di una versione della bozza", `UPDATE distinta_bozza_versione SET impronta = repeat('c', 64)`, "P4X02"},
		{"DELETE di una versione della bozza", `DELETE FROM distinta_bozza_versione`, "P4X02"},
		{"TRUNCATE delle versioni", `TRUNCATE distinta_bozza_versione CASCADE`, "P4X02"},
		{"UPDATE di un evento della bozza", `UPDATE distinta_bozza_evento SET azione = 'altro'`, "P4X02"},
		{"DELETE di un evento della bozza", `DELETE FROM distinta_bozza_evento`, "P4X02"},
		{"TRUNCATE del giornale", `TRUNCATE distinta_bozza_evento`, "P4X02"},
		{"un evento fuori dalla transazione del salvataggio", `INSERT INTO distinta_bozza_evento (bozza_id, n, versione, azione, origine, utente_id)
			VALUES ('{B1}', 2, 1, 'togli_nodo', 'utente', '{U}')`, "P4X04"},
		{"DELETE di una bozza aperta", `DELETE FROM distinta_bozza`, "P4X04"},
		// mutazioni S32-S36, S42, S59: il ciclo della bozza
		{"una bozza aperta che cambia la creazione", `UPDATE distinta_bozza SET creata_il = creata_il - interval '1 day', versione = versione + 1`, "P4X04"},
		{"una chiusura che cambia anche la base", `UPDATE distinta_bozza SET stato = 'abbandonata', chiusa_da = '{U}', firma_base = repeat('9', 64)`, "P4X04"},
		{"una bozza chiusa che cambia ancora", `UPDATE distinta_bozza SET stato = 'abbandonata', chiusa_da = '{U}';
			UPDATE distinta_bozza SET motivo_chiusura = 'ripensamento'`, "P4X04"},
		{"una bozza che nasce alla versione 1", `INSERT INTO distinta_bozza (thread_id, prodotto_id, versione, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
			VALUES ('{T}', '{P3}', 1, 0, 0, repeat('1', 64), '{U}', '{U}')`, "P4X04"},
		{"una bozza di un sottoassieme", `INSERT INTO distinta_bozza (thread_id, prodotto_id, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
			VALUES ('{T}', '{S}', 0, 0, repeat('1', 64), '{U}', '{U}')`, "P4X04"},
		{"una seconda bozza aperta dello stesso prodotto", `INSERT INTO distinta_bozza (thread_id, prodotto_id, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
			VALUES ('{T}', '{P2}', 0, 0, repeat('1', 64), '{U}', '{U}')`, "23505"},
		{"TRUNCATE delle bozze", `TRUNCATE distinta_bozza CASCADE`, "P4X04"},
		{"una bozza chiusa come confermata senza conferma", `UPDATE distinta_bozza SET stato = 'confermata', chiusa_da = '{U}', chiusa_il = now()`, "P4X04"},
		{"la versione della bozza senza la sua riga", `UPDATE distinta_bozza SET versione = 5`, "P4X04"},
		{"UPDATE di un esito", `UPDATE esito_intento SET esito = '{}'`, "P4X03"},
		{"DELETE di un esito recente", `DELETE FROM esito_intento`, "P4X03"},
		{"TRUNCATE degli esiti", `TRUNCATE esito_intento`, "P4X03"},
		{"DELETE di uno stato F8", `DELETE FROM f8_stato`, "P4X03"},
		{"F8 che cambia conferma", `UPDATE f8_stato SET conferma_id = NULL`, "P4X03"},
		// mutazioni S51, S52
		{"F8 eseguito che torna in attesa", `UPDATE f8_stato SET stato = 'eseguito', risolto_da = 'stessa_richiesta';
			UPDATE f8_stato SET stato = 'in_attesa', risolto_da = NULL`, "P4X03"},
		{"F8 con i tentativi che calano", `UPDATE f8_stato SET tentativi = 2; UPDATE f8_stato SET tentativi = 1`, "P4X03"},
		{"UPDATE di una correzione", `UPDATE correzione_documento SET motivo = 'altro'`, "P4X02"},
		{"DELETE di una correzione", `DELETE FROM correzione_documento`, "P4X02"},
		{"UPDATE di un'associazione", `UPDATE associazione_evento SET motivo = 'altro'`, "P4X02"},
		{"DELETE di un'associazione", `DELETE FROM associazione_evento`, "P4X02"},
		{"UPDATE della storia della fonte", `UPDATE fonte_step_evento SET gruppo = 'bom'`, "P4X02"},
		{"DELETE della storia della fonte", `DELETE FROM fonte_step_evento`, "P4X02"},
		{"UPDATE di un'attestazione", `UPDATE attestazione_fascicolo SET requisiti = '[]'`, "P4X02"},
		{"DELETE di un documento attestato", `DELETE FROM attestazione_fascicolo_documento`, "P4X02"},
		{"UPDATE della storia delle deroghe", `UPDATE deroga_fabbisogno_storia SET autore_id = NULL`, "P4X02"},
		{"DELETE della storia delle deroghe", `DELETE FROM deroga_fabbisogno_storia`, "P4X02"},
		{"il contatore che torna indietro", `UPDATE working_contatore SET struttura = 0`, "P4X05"},
		// R-01: la riga la scrivono solo i trigger, con le marche della transazione corrente
		{"un contatore scritto a mano", `INSERT INTO working_contatore (thread_id, tx_struttura, tx_dati) VALUES (gen_random_uuid(), pg_current_xact_id(), pg_current_xact_id())`, "P4X05"},
		{"una marca di transazione scritta a mano", `UPDATE working_contatore SET tx_struttura = pg_current_xact_id() WHERE thread_id = '{T}'`, "P4X05"},
		{"il muro della conferma tolto a mano", `UPDATE working_contatore SET tx_conferma = NULL WHERE thread_id = '{T}'`, "P4X05"},
		{"UPDATE della storia delle deroghe strutturali", `UPDATE deroga_struttura_storia SET autore_id = NULL`, "P4X02"},
		{"DELETE della storia delle deroghe strutturali", `DELETE FROM deroga_struttura_storia`, "P4X02"},
		{"TRUNCATE della storia delle deroghe strutturali", `TRUNCATE deroga_struttura_storia`, "P4X02"},
		// mutazione S08: anche il contatore dati cresce soltanto (il trigger risponde prima del CHECK)
		{"il contatore dati che torna indietro", `UPDATE working_contatore SET dati = dati - 1 WHERE thread_id = '{T}'`, "P4X05"},
		{"DELETE di un contatore", `DELETE FROM working_contatore`, "P4X05"},
		{"TRUNCATE dei contatori", `TRUNCATE working_contatore`, "P4X05"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			tx, err := p.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx) //nolint:errcheck
			if _, err := tx.Exec(ctx, "SET CONSTRAINTS ALL IMMEDIATE"); err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx, sost22.Replace(c.sql))
			if codice(err) != c.codice {
				t.Fatalf("atteso %s, ottenuto %v", c.codice, err)
			}
		})
	}
	// un componente certificato non si rimuove (FK), si archivia; un finito con una bozza nemmeno
	for _, comp := range []string{"{S}", "{P2}"} {
		_, err := p.Exec(ctx, sost22.Replace(`DELETE FROM componente WHERE componente_id = '`+comp+`'`))
		if codice(err) != "23503" {
			t.Errorf("rimozione di %s: atteso il rifiuto della FK, ottenuto %v", comp, err)
		}
	}
	// la conservazione: un esito di oltre 90 giorni si cancella (data retrodatata spegnendo il trigger della data)
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, s := range []string{
		`ALTER TABLE esito_intento DISABLE TRIGGER trg_esito_intento_ora`,
		`INSERT INTO esito_intento (chiave, thread_id, gesto, utente_id, impronta_corpo, esito, creato_il) VALUES (gen_random_uuid(), '{T}', 'riprova_copia', '{U}', repeat('a', 64), '{}', now() - interval '91 days')`,
		`ALTER TABLE esito_intento ENABLE TRIGGER trg_esito_intento_ora`,
	} {
		if _, err := tx.Exec(ctx, sost22.Replace(s)); err != nil {
			t.Fatal(err)
		}
	}
	tag, err := tx.Exec(ctx, `DELETE FROM esito_intento WHERE creato_il < now() - interval '90 days'`)
	if err != nil || tag.RowsAffected() != 1 {
		t.Errorf("cancellazione di un esito oltre la conservazione: %v (%d righe)", err, tag.RowsAffected())
	}
}

// ------------------------------------------------------------------ i limiti dei campi

func TestP4ILimitiDeiCampi(t *testing.T) {
	p := schema22ConStoria(t)
	ctx := context.Background()
	casi := []struct {
		nome, tab string
		cambi     map[string]string
		atteso    string // SQLSTATE o nome del vincolo
	}{
		{"codice certificato di 61 caratteri", "conferma_strutturale_componente", map[string]string{"codice": "repeat('A', 61)"}, "22001"},
		{"rev certificata di 11 caratteri", "conferma_strutturale_componente", map[string]string{"rev": "repeat('B', 11)"}, "22001"},
		{"descrizione certificata di 201 caratteri", "conferma_strutturale_componente", map[string]string{"descrizione": "repeat('è', 201)"}, "22001"},
		{"posizione certificata di 21 caratteri", "conferma_strutturale_arco", map[string]string{"posizione": "repeat('1', 21)"}, "22001"},
		{"nome del file corretto di 301 caratteri", "correzione_documento", map[string]string{"nome_file": "repeat('n', 301)"}, "22001"},
		{"codice letto di 61 caratteri", "correzione_documento", map[string]string{"codice_letto": "repeat('C', 61)"}, "22001"},
		{"rev attestata di 11 caratteri", "attestazione_fascicolo_documento", map[string]string{"rev": "repeat('R', 11)"}, "22001"},
		{"motivo di chiusura di 1001 caratteri", "distinta_bozza", map[string]string{"stato": "'abbandonata'", "chiusa_il": "now()", "chiusa_da": "'{U}'", "motivo_chiusura": "repeat('m', 1001)"}, "ck_distinta_bozza_chiusura"},
		{"gruppo della fonte di 301 caratteri", "distinta_bozza_versione", map[string]string{"fonte_gruppo": "repeat('g', 301)"}, "ck_dbv_fonte"},
		{"snapshot oltre 4 MiB", "distinta_bozza_versione", map[string]string{"snapshot": "jsonb_build_object('x', repeat('y', 4200000))"}, "ck_dbv_snapshot"},
		{"motivo di un evento di 1001 caratteri", "distinta_bozza_evento", map[string]string{"motivo": "repeat('m', 1001)"}, "ck_dbe_motivo"},
		{"azione di 65 caratteri", "distinta_bozza_evento", map[string]string{"azione": "repeat('a', 65)"}, "ck_dbe_azione"},
		{"chiave della bozza di 65 caratteri", "distinta_bozza_evento", map[string]string{"chiave_bozza": "repeat('k', 65)"}, "ck_dbe_chiave"},
		{"motivo mono_pezzo di 1001 caratteri", "conferma_strutturale", map[string]string{"mono_pezzo": "true", "criterio_verifica": "'mono_pezzo'", "mono_pezzo_motivo": "repeat('m', 1001)"}, "ck_conferma_mono_pezzo"},
		{"firma dell'anteprima formato 1 di 201 caratteri", "conferma_strutturale", map[string]string{"formato": "1", "senza_riepilogo_v2": "true", "chiave_idempotenza": "NULL", "firma_anteprima": "repeat('f', 201)"}, "ck_conferma_firme"},
		{"origine oltre 64 KiB", "conferma_strutturale", map[string]string{"origine": "jsonb_build_object('rotta', repeat('r', 70000))"}, "ck_conferma_json"},
		{"errore di F8 di 2001 caratteri", "f8_stato", map[string]string{"stato": "'errore'", "errore": "repeat('e', 2001)"}, "ck_f8_errore"},
		{"motivo di una correzione di 501 caratteri", "correzione_documento", map[string]string{"motivo": "repeat('m', 501)"}, "ck_correzione_motivo"},
		{"motivo della fonte di 1001 caratteri", "fonte_step_evento", map[string]string{"motivo": "repeat('m', 1001)"}, "ck_fonte_motivo"},
		{"motivo di un'associazione di 1001 caratteri", "associazione_evento", map[string]string{"motivo": "repeat('m', 1001)"}, "ck_associazione_motivo"},
		{"requisiti dell'attestazione oltre 4 MiB", "attestazione_fascicolo", map[string]string{"requisiti": "jsonb_build_array(repeat('r', 4200000))"}, "ck_attestazione_requisiti"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			tx, err := p.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx) //nolint:errcheck
			if _, err := tx.Exec(ctx, "ALTER TABLE "+c.tab+" DISABLE TRIGGER USER"); err != nil {
				t.Fatal(err)
			}
			_, err = tx.Exec(ctx, insertCon(c.tab, c.cambi))
			if got := codice(err); got != c.atteso && vincoloViolato(err) != c.atteso {
				t.Fatalf("atteso %s, ottenuto %v", c.atteso, err)
			}
		})
	}
	// i valori al limite passano
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, "ALTER TABLE conferma_strutturale_componente DISABLE TRIGGER USER"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, insertCon("conferma_strutturale_componente", map[string]string{"codice": "repeat('A', 60)", "rev": "repeat('B', 10)",
		"descrizione": "repeat('è', 200)", "chiave_bozza": "repeat('k', 64)"})); err != nil {
		t.Errorf("i valori al limite delle colonne non passano: %v", err)
	}
}
