//go:build integrazione

// L4 — la compatibilita' 22 → 23 → 22 (piano della 0023, PM23 §9.1): l'upgrade con righe formato 1 e 2 che restano
// identiche e leggibili (U-01), le cinque viste uguali senza righe di stato dei file (U-02), il formato 2 stretto e il
// formato 3 nuovo (U-03), la bozza formato 2 che non cambia formato (U-04), la conversione formato 2 → 3 nella sua parte
// di database (U-05), il binario della 22 fermato da un database alla 23 (U-07) e il ritorno manuale con
// scripts/0023_indietro.sql (U-09). U-08 e' TestP4La22DaZeroEDueAvviiIdentici (p4_0022_db_test.go), riscritta per V1.

package migrazioni_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/migrazioni"
	"promatec/cockpit/internal/platform/testutil"
)

func applica23(p *pgxpool.Pool) error {
	_, err := migrazioni.ApplicaFinoA(context.Background(), p, risorse.FS, 23, testutil.LogSilenzioso())
	return err
}

// righeF1F2: le righe sintetiche alla 22 di U-01 e U-09 (base21 compresa): la conferma formato 1 n. 1 di P con i suoi
// componenti e il suo arco; una bozza formato 2 aperta di P3 con due versioni, gli eventi e una fonte secondaria con la
// destinazione temporanea sul nodo senza UUID; una bozza formato 2 di P2 confermata dalla conferma formato 2 n. 1 di P2,
// con l'esito conferma_bom_v2 e lo stato di F8; l'attestazione sulla conferma di P, una correzione, un'associazione, la
// fonte del prodotto e una deroga strutturale.
func righeF1F2() []string {
	conta := `COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = '{T}'), 0), COALESCE((SELECT dati FROM working_contatore WHERE thread_id = '{T}'), 0)`
	return []string{
		base21,
		`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2, firma_struttura,
			firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati, origine, snapshot, impronta_snapshot, delta,
			mappa, decisioni, criterio_verifica, confermato_da)
		 SELECT '{C1}', '{T}', '{P}', 1, NULL, 1, true, repeat('3', 64), NULL, NULL, 0, NULL, ` + conta + `,
			'{"rotta": "distinta/albero/conferma"}', '{"busta": "formato 1", "voci": 2}', repeat('a', 64), '{}', '{}',
			'[{"tipo": "d1_a_particolare", "codice": "ACM-1100"}]', 'nessuno', '{U}';
		 INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo)
		 VALUES ('{C1}', '{T}', '{P}', true, NULL, 'ACM-1000', 'A', 'Gruppo base', 'finito'),
		        ('{C1}', '{T}', '{S}', false, NULL, 'ACM-1100', 'A', 'Telaio', 'sottoassieme');
		 INSERT INTO conferma_strutturale_arco (conferma_id, padre_id, figlio_id, qta, origine) VALUES ('{C1}', '{P}', '{S}', 2, NULL);`,
		`INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
		 VALUES ('{BR}', '{T}', '{P3}', 0, 0, repeat('1', 64), '{U}', '{U}');
		 INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, salvata_da)
		 VALUES ('{BR}', 1, jsonb_build_object('formato', 2, 'prodotto_id', '{P3}'::text, 'nodi', jsonb_build_array(
			jsonb_build_object('chiave_bozza', 'k-p3', 'componente_id', '{P3}'::text, 'op', 'riusa', 'codice', 'ACM-3000', 'tipo', 'finito', 'origine', 'registrato'),
			jsonb_build_object('chiave_bozza', 'k-n', 'op', 'crea', 'codice', 'ACM-3100', 'tipo', 'sciolto', 'origine', 'utente'))),
			repeat('a', 64), 0, 0, repeat('1', 64), '{U}');
		 INSERT INTO distinta_bozza_evento (bozza_id, n, versione, azione, origine, utente_id) VALUES ('{BR}', 1, 1, 'bozza_aperta', 'server', '{U}');`,
		`INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, salvata_da)
		 SELECT '{BR}', 2, snapshot, repeat('b', 64), 0, 0, repeat('1', 64), '{U}' FROM distinta_bozza_versione WHERE bozza_id = '{BR}' AND versione = 1;
		 INSERT INTO distinta_bozza_evento (bozza_id, n, versione, azione, chiave_bozza, origine, utente_id)
		 VALUES ('{BR}', 2, 2, 'nodo_modificato', 'k-n', 'utente', '{U}');`,
		`INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, chiave_bozza, bozza_id, azione, allegato_id, sha256, gruppo, autore_id)
		 VALUES ('{T}', '{P3}', 'sottoassieme', 'k-n', '{BR}', 'scelta', '{A}', repeat('b', 64), 'bom', '{U}');`,
		`INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
		 VALUES ('{B1}', '{T}', '{P2}', 0, 0, repeat('1', 64), '{U}', '{U}');
		 INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, salvata_da)
		 VALUES ('{B1}', 1, jsonb_build_object('formato', 2, 'prodotto_id', '{P2}'::text), repeat('a', 64), 0, 0, repeat('1', 64), '{U}');`,
		`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura, firma_anteprima,
			base_contatore, firma_base, contatore, contatore_dati, bozza_id, bozza_versione, origine, snapshot, impronta_snapshot, delta, mappa,
			criterio_verifica, chiave_idempotenza, confermato_da)
		 SELECT '{C2}', '{T}', '{P2}', 1, 2, false, repeat('3', 64), repeat('2', 64), 0, repeat('1', 64), ` + conta + `,
			'{B1}', 1, '{"rotta": "motore-a/bom/conferma"}', jsonb_build_object('formato', 2, 'prodotto_id', '{P2}'::text), repeat('a', 64),
			'{}', '{"k-p2": "{P2}"}', 'nessuno', '{K1}', '{U}';
		 INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, descrizione, tipo)
		 VALUES ('{C2}', '{T}', '{P2}', true, 'k-p2', 'ACM-2000', 'Piastra', 'finito');
		 UPDATE distinta_bozza SET stato = 'confermata', chiusa_da = '{U}' WHERE bozza_id = '{B1}';
		 INSERT INTO esito_intento (chiave, thread_id, gesto, utente_id, impronta_corpo, esito, conferma_id)
		 VALUES ('{K1}', '{T}', 'conferma_bom_v2', '{U}', repeat('a', 64), '{"esito": "confermata"}', '{C2}');
		 INSERT INTO f8_stato (thread_id, chiave_intento, conferma_id) VALUES ('{T}', '{K1}', '{C2}');`,
		storia22[2], storia22[3], storia22[4],
	}
}

// colonneDi: le colonne di una tabella (in ordine), lette dal catalogo.
func colonneDi(t *testing.T, p *pgxpool.Pool, tab string) string {
	return uno23[string](t, p, fmt.Sprintf(`SELECT string_agg(quote_ident(attname), ', ' ORDER BY attnum) FROM pg_attribute
		WHERE attrelid = '%s'::regclass AND attnum > 0 AND NOT attisdropped`, tab))
}

// improntaColonne: l'md5 delle righe di una tabella sulle sole colonne date.
func improntaColonne(t *testing.T, p *pgxpool.Pool, tab, colonne, filtro string) string {
	return uno23[string](t, p, fmt.Sprintf(`SELECT md5(coalesce(string_agg(x::text, '|' ORDER BY x::text), '')) FROM (SELECT %s FROM %s %s) x`,
		colonne, tab, filtro))
}

// funzioni e viste che la 0023 sostituisce o aggiunge, per la lista ammessa del catalogo (U-01 c).
var (
	funzioniNuove23 = []string{"distinta_importazione_nasce", "conferma_strutturale_non_incluso_nasce", "esito_intento_formato",
		"file_rfq_nasce", "file_rfq_cambia", "file_rfq_rimozione_nasce", "file_rfq_rimozione_cambia", "file_rfq_conservazione_nasce",
		"file_rfq_conservazione_cambia", "file_rfq_invarianti", "file_rfq_conservazione_invarianti", "file_rfq_attivo",
		"file_rfq_scartato_legacy", "analisi_fatti_digest",
		// direttiva 35 (variante 20261019a): le associazioni registrate e il loro giornale
		"file_rfq_associazione_nasce", "file_rfq_associazione_bom", "file_rfq_associazione_cambia", "file_rfq_associazione_invarianti",
		"file_rfq_senza_ar_attive", "conferma_strutturale_associazioni", "documento_proposta_copia_derivata", "associazione_evento_della_riga",
		// direttiva 39 (variante 20261019c): il segno della RFQ formato 3, lo STEP legato come sorgente del finito
		"rfq_formato3_segna", "rfq_e_formato3", "allegato_sorgente_del_finito"}
	vincoliRicreati23 = []string{"ck_distinta_bozza_formato", "ck_conferma_formato", "ck_conferma_bozza", "ck_conferma_snapshot",
		"ck_conferma_chiave", "ck_esito_intento_gesto", "ck_esito_intento_conferma",
		"ck_associazione_oggetto", "ck_associazione_azione", "ck_associazione_coerente", "ck_correzione_oggetto",
		// direttiva 39: le origini del giornale e le azioni della fonte; il NOT NULL di autore_id che la 0023 toglie
		"ck_associazione_origine", "ck_fonte_azione", "associazione_evento_autore_id_not_null"}
	vincoliNuoviSu22 = []string{"fk_dbv_importazione", "ck_dbv_snapshot_formato", "ck_dbv_chiave_salvataggio", "fk_conferma_importazione",
		"ck_conferma_importazione", "ck_conferma_importazioni_citate", "ck_csc_identita", "ck_conferma_verifica", "ck_analisi_fatti_digest",
		"analisi_fatti_digest_not_null",
		"associazione_evento_allegato_id_fkey", "fk_associazione_ar", "fk_associazione_ar_precedente", "fk_correzione_associazione",
		"trg_conferma_associazioni", "trg_proposta_copia_derivata", // i due trigger di vincolo su tabelle della 22 stanno anche in pg_constraint
		"ck_associazione_autore", "ck_fonte_revoca_sorgente", "ck_dbv_preassociazioni", "ck_conferma_preassociazioni"} // direttiva 39
	indiciNuoviSu22 = []string{"ix_dbv_importazione", "ux_dbv_chiave_salvataggio", "ix_conferma_importazione", "ix_conferma_importazioni_citate", "ix_csc_emittente",
		"ux_associazione_evento_chiave", "ix_associazione_evento_allegato", "ix_associazione_evento_ar", "ix_correzione_associazione"}
	commentiCambiati = []string{"commento table public.distinta_bozza_versione ", "commento table public.conferma_strutturale ",
		"commento table column public.conferma_strutturale_arco.origine ", "commento table column public.conferma_strutturale.verifica ",
		"commento table column public.analisi_fatti.digest ",
		"commento table public.associazione_evento ", "commento table column public.documento_proposta.componente_id "}
	// la colonna della 0023 su una tabella di prima che si riempie (il digest dei fatti, U2 §3.14 riga 10) e il suo trigger
	colonneRiempite23 = []struct{ tab, col string }{{"analisi_fatti", "digest"}}
	triggerNuoviSu22  = []string{"trg_esito_intento_formato", "trg_analisi_fatti_digest",
		"trg_associazione_della_riga", "trg_conferma_associazioni", "trg_proposta_copia_derivata", "trg_distinta_bozza_formato3"}
	// le colonne della 22 che la 0023 cambia senza riempirle (direttiva 39: autore_id nullo solo con conversione_legacy)
	colonneCambiate23 = []struct{ tab, col string }{{"associazione_evento", "autore_id"}}
)

// rigaAmmessa23: una riga del catalogo che la 0023 puo' cambiare o aggiungere.
func rigaAmmessa23(r string) bool {
	for _, f := range append(append([]string{}, funzioniSostituite23()...), funzioniNuove23...) {
		if strings.HasPrefix(r, "funzione "+f+"(") {
			return true
		}
	}
	for _, v := range []string{"v_fascicolo", "v_step_prodotto", "v_thread_da_riesaminare", "v_cruscotto", "v_codici_candidati_thread",
		"v_ar_backfill_diagnostica"} {
		if strings.HasPrefix(r, "vista "+v+" ") {
			return true
		}
	}
	for _, c := range commentiCambiati {
		if strings.HasPrefix(r, c) {
			return true
		}
	}
	tab, nome := oggettoDelCatalogo(r)
	for _, t23 := range append(append([]string{}, tabelle23...), "v_ar_backfill_diagnostica") { // la vista diagnostica della direttiva 35
		if tab == t23 || strings.Contains(r, " "+t23+" ") || strings.Contains(r, " "+t23+".") || strings.Contains(r, "public."+t23) ||
			strings.HasPrefix(r, "relazione "+t23+"_") || strings.HasPrefix(r, "sequenza "+t23+"_") {
			return true
		}
	}
	for _, c := range colonne23 {
		if (tab == c.tab && nome == c.col) || strings.HasPrefix(r, "commento table column public."+c.tab+"."+c.col+" ") {
			return true
		}
	}
	for _, v := range append(append([]string{}, vincoliRicreati23...), vincoliNuoviSu22...) {
		if strings.HasPrefix(r, "vincolo ") && nome == v {
			return true
		}
	}
	for _, i := range indiciNuoviSu22 {
		if strings.HasPrefix(r, "indice "+i+" ") {
			return true
		}
	}
	for _, c := range append(append([]struct{ tab, col string }{}, colonneRiempite23...), colonneCambiate23...) {
		if tab == c.tab && nome == c.col {
			return true
		}
	}
	for _, g := range triggerNuoviSu22 {
		if strings.HasPrefix(r, "trigger ") && nome == g {
			return true
		}
	}
	for _, f := range []string{"file_rfq_attivo", "file_rfq_scartato_legacy", "rfq_e_formato3", "allegato_sorgente_del_finito"} {
		if strings.HasPrefix(r, "commento function public."+f+"(") {
			return true
		}
	}
	return false
}

func funzioniSostituite23() []string {
	return []string{"distinta_bozza_versione_nasce", "conferma_strutturale_nasce", "conferma_strutturale_contenuto", "conferma_strutturale_completa",
		"fonte_step_risolvi"}
}

// U-01: 22 → 23 con righe formato 1 e 2: (a) per ogni tabella della 22, l'impronta sulle sole colonne della 22 e'
// identica, e i dati delle tabelle storiche non cambiano; (b) le otto colonne nuove sono NULL sulle righe vecchie; (c) il
// catalogo prima e dopo differisce solo per la lista ammessa; (d) lo schema dopo 22 → 23 e' quello da zero. Con la
// direttiva 35 le colonne nuove sono tredici (colonne23), e le righe di prima di associazione_evento restano senza
// allegato, riga della AR e transazione (nessun riempimento).
func TestV1Upgrade22a23ConRigheF1F2(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	testutil.SchemaFinoA(t, p, 22)
	for _, s := range righeF1F2() {
		esegui23(t, p, s)
	}
	storiche := tabelleDelloSchema(t, p)
	colonne := map[string]string{}
	impronte := map[string]string{}
	for _, tab := range storiche {
		colonne[tab] = colonneDi(t, p, tab)
		filtro := ""
		if tab == "schema_variante" || tab == "schema_versione" {
			filtro = "WHERE versione <= 22"
		}
		impronte[tab] = improntaColonne(t, p, tab, colonne[tab], filtro)
	}
	catPrima := impronta18(t, p)
	if err := applica23(p); err != nil {
		t.Fatalf("upgrade 22 → 23: %v", err)
	}
	if !applicata(t, p, 23) {
		t.Fatal("la 23 non risulta applicata")
	}
	// (a)
	for _, tab := range storiche {
		filtro := ""
		if tab == "schema_variante" || tab == "schema_versione" {
			filtro = "WHERE versione <= 22"
		}
		if got := improntaColonne(t, p, tab, colonne[tab], filtro); got != impronte[tab] {
			t.Errorf("l'upgrade ha cambiato le righe di %s (sulle colonne della 22)", tab)
		}
	}
	// (b)
	for _, c := range colonne23 {
		if n := uno23[int](t, p, fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s IS NOT NULL`, c.tab, c.col)); n != 0 {
			t.Errorf("%s.%s ha %d valori sulle righe di prima", c.tab, c.col, n)
		}
	}
	for _, tab := range tabelle23 {
		if n := testutil.Conta(t, p, tab); n != 0 {
			t.Errorf("l'upgrade ha scritto %d righe in %s", n, tab)
		}
	}
	if n := uno23[int](t, p, `SELECT count(*) FROM schema_variante WHERE versione = 23`); n != 1 {
		t.Errorf("righe della variante della 23: %d", n)
	}
	// le righe di prima si leggono ancora con i vincoli di oggi: nessun vincolo della 23 lascia una riga «non valida»
	// (ALTER TABLE ... VALIDATE e' implicito nell'ADD CONSTRAINT), e la conferma formato 1 resta senza bozza ne' chiave
	if n := uno23[int](t, p, `SELECT count(*) FROM conferma_strutturale WHERE formato IN (1, 2)`); n != 2 {
		t.Errorf("conferme formato 1 e 2 dopo l'upgrade: %d", n)
	}
	// (c)
	catDopo := impronta18(t, p)
	inDopo, inPrima := map[string]bool{}, map[string]bool{}
	for _, r := range catDopo {
		inDopo[r] = true
	}
	for _, r := range catPrima {
		inPrima[r] = true
		if !inDopo[r] && !rigaAmmessa23(r) {
			t.Errorf("l'upgrade ha cambiato o tolto: %s", r)
		}
	}
	for _, r := range catDopo {
		if !inPrima[r] && !rigaAmmessa23(r) {
			t.Errorf("riga nuova del catalogo che non e' della 0023: %s", r)
		}
	}
	// (d)
	testutil.SchemaPulito(t, p)
	if d := differenze(catDopo, impronta18(t, p)); d != "" {
		t.Errorf("lo schema dopo 22 → 23 non e' quello da zero:\n%s", d)
	}
}

// U-02: con allegati, proposte (aperte, scartate, confermate), righe strutturali e documenti alla 22, dopo la 23 le
// cinque viste danno le stesse righe, colonna per colonna (nessuna riga in file_rfq).
func TestV1LeVisteUgualiDopoLa23(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	testutil.SchemaFinoA(t, p, 22)
	for _, s := range append([]string{base21, allegati23}, righeViste...) {
		esegui23(t, p, s)
	}
	// una proposta scartata da una persona e una confermata, che non contano gia' prima
	esegui23(t, p, `UPDATE documento_proposta SET stato = 'scartata', deciso_da = '{U}', deciso_il = now() WHERE proposta_id = '{DP5}';
		UPDATE componente_proposta SET stato = 'confermata', componente_id = '{P2}', deciso_da = '{U}', deciso_il = now() WHERE proposta_id = '{CP1}';`)
	prima := righeDelleViste(t, p)
	for v, righe := range prima {
		if righe == "" {
			t.Errorf("%s non ha righe prima della 23: la prova non direbbe niente", v)
		}
	}
	if err := applica23(p); err != nil {
		t.Fatalf("upgrade 22 → 23: %v", err)
	}
	dopo := righeDelleViste(t, p)
	for v := range prima {
		if prima[v] != dopo[v] {
			t.Errorf("%s ha righe diverse dopo la 23:\nprima: %s\ndopo:  %s", v, prima[v], dopo[v])
		}
	}
}

// U-03, U-04: dopo la 23 il formato 2 resta stretto (una conferma formato 2 nuova passa con le sue regole, uno snapshot
// «3» sotto una formato 2 no), la conferma formato 3 con la chiave passa e senza no, e la bozza formato 2 aperta nata
// alla 22 non cambia formato.
func TestV1IlFormato2RestaStrettoEIlFormato3Nuovo(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	testutil.SchemaFinoA(t, p, 22)
	esegui23(t, p, base21)
	esegui23(t, p, conferma1)
	for _, s := range storia22 {
		esegui23(t, p, s)
	}
	if err := applica23(p); err != nil {
		t.Fatalf("upgrade 22 → 23: %v", err)
	}
	// U-04
	rifiuto23(t, p, "U-04 la bozza formato 2 nata alla 22 non diventa formato 3", "P4X04", "RFQ, prodotto, formato e creazione non cambiano",
		`UPDATE distinta_bozza SET formato = 3 WHERE bozza_id = '{B1}'`)
	// U-03, formato 2
	rifiuto23(t, p, "U-03 formato 2 con snapshot «3»", "23514", "ck_conferma_snapshot",
		strings.Replace(conferma2("0", "0", "2", "nessuno"), "jsonb_build_object('formato', 2, 'prodotto_id', '{P}'::text)",
			"jsonb_build_object('formato', 3, 'prodotto_id', '{P}'::text, 'nodi', '[]'::jsonb)", 1))
	rifiuto23(t, p, "U-03 formato 2 senza chiave", "23514", "ck_conferma_chiave",
		strings.Replace(conferma2("0", "0", "2", "nessuno"), "gen_random_uuid(), '{U}'", "NULL, '{U}'", 1))
	riuscita23(t, p, "U-03 conferma formato 2 nuova con le regole formato 2", conferma2("0", "0", "2", "nessuno"), radiceC2, figlioC2, arcoC2)
	// U-03, formato 3: le righe della 23 sopra il database migrato, poi la conferma n. 3 di P che chiude B3
	for _, s := range righe23(t) {
		esegui23(t, p, s)
	}
	c := nuovaC3()
	c.testata["numero"] = "3"
	c.testata["precedente_id"] = "'{C2}'"
	c.testata["base_conferma_id"] = "'{C2}'"
	senza := nuovaC3()
	senza.testata = map[string]string{"numero": "3", "precedente_id": "'{C2}'", "base_conferma_id": "'{C2}'", "chiave_idempotenza": "NULL"}
	rifiuto23(t, p, "U-03 formato 3 senza chiave", "23514", "ck_conferma_chiave", senza.istruzioni(t)...)
	riuscita23(t, p, "U-03 formato 3 con la chiave", c.istruzioni(t)...)
}

// U-05 (parte del database; l'algoritmo conversione-1 e' di R2, passo 6): la conversione esplicita di una bozza formato
// 2 aperta, nata alla 22, in una bozza formato 3 nuova, in una transazione: la formato 2 e' abbandonata con il motivo e
// resta nel registro delle bozze, la formato 3 nasce alla versione 1 con i nodi formato_2 presunti e l'evento
// bozza_aperta motivato; gli snapshot formato 2 restano identici byte per byte; la fonte secondaria temporanea si
// riprende sulla bozza nuova con la stessa chiave.
func TestV1LaConversioneFormato2InFormato3(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	testutil.SchemaFinoA(t, p, 22)
	for _, s := range righeF1F2() {
		esegui23(t, p, s)
	}
	if err := applica23(p); err != nil {
		t.Fatalf("upgrade 22 → 23: %v", err)
	}
	snapPrima := uno23[string](t, p, `SELECT string_agg(snapshot::text || impronta, '|' ORDER BY versione) FROM distinta_bozza_versione WHERE bozza_id = '{BR}'`)
	cliente := "cliente:" + u23("{C}")
	nuova := map[string]any{"formato": 3, "prodotto_id": u23("{P3}"),
		"versione_bom_base": map[string]any{"conferma_id": nil, "numero": 0, "contatore": 0}, "firma_base": sha("1"),
		"nodi": []any{
			map[string]any{"chiave_bozza": "k-n", "op": "crea", "tipo": "sciolto", "origine": "utente",
				"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": "ACM-3100", "modo": "formato_2"}},
			map[string]any{"chiave_bozza": "k-p3", "componente_id": u23("{P3}"), "op": "riusa", "tipo": "finito", "origine": "registrato",
				"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": "ACM-3000", "modo": "formato_2"}}},
		"archi": []any{}, "decisioni": []any{}, "documenti_preassociati": []any{}}
	// il motivo dell'ingegnere al limite di 1000 caratteri: intero e senza prefissi nell'evento bozza_aperta della bozza
	// nuova; la formato 2 si chiude con il solo prefisso, che sta sempre nei 1000 caratteri (EM §6.7 passi 1 e 3; MIG-12)
	motivo := strings.Repeat("m", 1000)
	chiusura := "convertita nel formato 3 nella bozza " + u23("{B4}")
	riuscita23(t, p, "U-05 conversione in una transazione",
		`UPDATE distinta_bozza SET stato = 'abbandonata', chiusa_da = '{U}', motivo_chiusura = '`+chiusura+`' WHERE bozza_id = '{BR}'`,
		`INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, formato, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
		 VALUES ('{B4}', '{T}', '{P3}', 3, 0, 0, repeat('1', 64), '{U}', '{U}')`,
		`INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, salvata_da,
			chiave_salvataggio, impronta_richiesta)
		 VALUES ('{B4}', 1, `+jsonSQL(t, nuova)+`, repeat('d', 64), 0, 0, repeat('1', 64), '{U}', gen_random_uuid(), repeat('4', 64))`,
		`INSERT INTO distinta_bozza_evento (bozza_id, n, versione, vocabolario, azione, dopo, origine, motivo, utente_id)
		 VALUES ('{B4}', 1, 1, 2, 'bozza_aperta', '{"algoritmo": "conversione-1", "converti_da": "{BR}", "righe_cadute": []}', 'server', '`+motivo+`', '{U}')`,
		`INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, chiave_bozza, bozza_id, azione, allegato_id, sha256, gruppo, autore_id)
		 VALUES ('{T}', '{P3}', 'sottoassieme', 'k-n', '{B4}', 'scelta', '{A}', repeat('b', 64), 'bom', '{U}')`)
	if s := uno23[string](t, p, `SELECT string_agg(snapshot::text || impronta, '|' ORDER BY versione) FROM distinta_bozza_versione WHERE bozza_id = '{BR}'`); s != snapPrima {
		t.Error("gli snapshot formato 2 sono cambiati con la conversione")
	}
	if s := uno23[string](t, p, `SELECT string_agg(passo, ' ' ORDER BY registro_id) FROM distinta_bozza_registro WHERE bozza_id = '{BR}'`); s != "aperta abbandonata" {
		t.Errorf("registro della bozza formato 2: %s", s)
	}
	if s := uno23[string](t, p, `SELECT formato || ' ' || stato || ' ' || versione FROM distinta_bozza WHERE bozza_id = '{B4}'`); s != "3 aperta 1" {
		t.Errorf("bozza formato 3 nuova: %s", s)
	}
	if m := uno23[string](t, p, `SELECT motivo FROM distinta_bozza_evento WHERE bozza_id = '{B4}' AND azione = 'bozza_aperta'`); m != motivo {
		t.Errorf("l'evento bozza_aperta non porta il motivo intero (%d caratteri invece di %d)", len(m), len(motivo))
	}
	if m := uno23[string](t, p, `SELECT motivo_chiusura FROM distinta_bozza WHERE bozza_id = '{BR}'`); m != chiusura {
		t.Errorf("motivo di chiusura della formato 2: %q, atteso il solo prefisso %q", m, chiusura)
	}
	if n := uno23[int](t, p, `SELECT count(*) FROM fonte_step_evento WHERE chiave_bozza = 'k-n' AND azione = 'scelta'`); n != 2 {
		t.Errorf("scelte della fonte secondaria su k-n (vecchia e nuova bozza): %d", n)
	}
	// una conversione sul posto resta vietata (U-04): la bozza nuova e' l'unica strada
	rifiuto23(t, p, "U-05 conversione sul posto", "P4X04", "una bozza chiusa non cambia", `UPDATE distinta_bozza SET formato = 3 WHERE bozza_id = '{BR}'`)
}

// U-07: un binario che conosce solo la 22 contro un database alla 23 si ferma prima di toccare qualunque cosa.
func TestV1UnBinarioDella22SiFermaSullaVentitre(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	ctx := context.Background()
	testutil.SchemaPulito(t, p)
	migs, err := migrazioni.Elenca(fsFinoA(t, 22))
	if err != nil {
		t.Fatal(err)
	}
	prima := impronta18(t, p)
	n, err := migrazioni.ApplicaElenco(ctx, p, migs, 22, testutil.LogSilenzioso())
	if err == nil || n != 0 || !strings.Contains(err.Error(), "il DB è alla versione 23 ma il binario conosce solo la 22") {
		t.Errorf("binario della 22 su un database alla 23: %d migrazioni, %v", n, err)
	}
	if d := differenze(prima, impronta18(t, p)); d != "" {
		t.Errorf("il binario fermato ha cambiato lo schema:\n%s", d)
	}
}

// indietro23 legge lo script di ritorno com'e' sul disco, fine riga compresi: lo stesso testo che l'utente lancerebbe
// con psql. I fine riga possono non essere quelli della 0022 che il binario ha incorporato (core.autocrlf scrive CRLF
// nei file presi dal repository e lascia LF in quelli nati sul posto): per questo il catalogo si confronta a meno dei
// CR (senzaCR).
func indietro23(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "0023_indietro.sql"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// senzaCR: le righe del catalogo (impronta18) senza i CR, che stanno solo nei corpi delle funzioni e nei commenti, in
// ordine: la garanzia del ritorno manuale e' «uguale alla 22 a meno dei fine riga».
func senzaCR(righe []string) []string {
	out := make([]string, len(righe))
	for i, r := range righe {
		out[i] = strings.ReplaceAll(r, "\r", "")
	}
	sort.Strings(out)
	return out
}

// U-09: 22 → 23 → 22 con scripts/0023_indietro.sql eseguito com'e' sul disco: il catalogo torna quello della 22 a meno
// dei CR (le sole righe diverse, se ci sono, differiscono per i soli CR), le righe della 22 restano intatte,
// schema_versione torna a 22 e la 0023 si riapplica sopra il ritorno.
func TestV1IlRitornoManualeDalla23Alla22(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	ctx := context.Background()
	testutil.SchemaFinoA(t, p, 22)
	catalogo22 := impronta18(t, p)
	for _, s := range righeF1F2() {
		esegui23(t, p, s)
	}
	tabelle := tabelleDelloSchema(t, p)
	dati := impronteDati(t, p, tabelle)
	if err := applica23(p); err != nil {
		t.Fatalf("upgrade 22 → 23: %v", err)
	}
	if _, err := p.Exec(ctx, indietro23(t)); err != nil {
		t.Fatalf("0023_indietro.sql: %v", err)
	}
	dopo := impronta18(t, p)
	if d := differenze(senzaCR(catalogo22), senzaCR(dopo)); d != "" {
		t.Errorf("lo schema dopo 22 → 23 → 22 non e' quello della 22, nemmeno a meno dei fine riga:\n%s", d)
	}
	if d := differenze(catalogo22, dopo); d != "" {
		t.Logf("ritorno uguale alla 22 a meno dei CR: le righe con fine riga diversi (checkout dello script e binario della 0022) sono\n%s", d)
	}
	if v := uno23[int32](t, p, `SELECT max(versione) FROM schema_versione`); v != 22 {
		t.Errorf("schema_versione e' a %d dopo il ritorno: doveva tornare a 22", v)
	}
	for tab, imp := range impronteDati(t, p, tabelle) {
		if dati[tab] != imp {
			t.Errorf("il ritorno ha cambiato le righe di %s", tab)
		}
	}
	if err := applica23(p); err != nil {
		t.Fatalf("la 0023 non si riapplica dopo il ritorno: %v", err)
	}
	if !applicata(t, p, 23) {
		t.Error("la 0023 riapplicata non risulta registrata")
	}
}

// U-09: il ritorno si rifiuta, senza cambiare niente, con lo schema non alla 23, con una riga formato 3, con una riga in
// una tabella nuova, con un valore in una colonna nuova e con un gesto della 23.
func TestV1IlRitornoDalla23SiRifiuta(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	ctx := context.Background()
	rifiutato := func(nome, frase string) {
		t.Helper()
		prima := impronta18(t, p)
		_, err := p.Exec(ctx, indietro23(t))
		if err == nil || !strings.Contains(err.Error(), frase) {
			t.Errorf("%s: atteso il rifiuto «%s», ottenuto %v", nome, frase, err)
		}
		// la connessione dello script puo' essere rimasta in una transazione fallita: si controlla da una nuova
		if d := differenze(prima, impronta18(t, p)); d != "" {
			t.Errorf("%s: il ritorno fermato ha cambiato lo schema:\n%s", nome, d)
		}
	}
	testutil.SchemaFinoA(t, p, 22)
	rifiutato("schema alla 22", "non alla 23")

	nuovoSchema := func() {
		testutil.SchemaPulito(t, p)
		for _, s := range righeF1F2() {
			esegui23(t, p, s)
		}
	}
	nuovoSchema()
	esegui23(t, p, `INSERT INTO esito_intento (chiave, thread_id, gesto, utente_id, impronta_corpo, esito)
		VALUES (gen_random_uuid(), '{T}', 'associa_step', '{U}', repeat('a', 64), '{}')`)
	rifiutato("un gesto della 23", "gesto della 23")

	nuovoSchema()
	esegui23(t, p, `INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, salvata_da,
		chiave_salvataggio, impronta_richiesta) SELECT '{BR}', 3, snapshot, repeat('c', 64), 0, 0, repeat('1', 64), '{U}', gen_random_uuid(), repeat('4', 64)
		FROM distinta_bozza_versione WHERE bozza_id = '{BR}' AND versione = 2`)
	rifiutato("un valore in una colonna della 23", "colonne della 23")

	nuovoSchema()
	esegui23(t, p, `INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, origine, esito) VALUES ('{T}', '{A2}', 'conservazione_errore', 'server', '{}')`)
	rifiutato("una riga in una tabella della 23", "le tabelle della 23")

	nuovoSchema()
	esegui23(t, p, `INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, formato, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
		VALUES ('{B4}', '{T2}', '{Q}', 3, 0, 0, repeat('1', 64), '{U}', '{U}')`+";"+`
		INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, salvata_da,
			chiave_salvataggio, impronta_richiesta)
		VALUES ('{B4}', 1, '{"formato": 3, "prodotto_id": "{Q}", "nodi": [{"chiave_bozza": "k-q", "identita": {"codice": "ACM-9000"}}], "archi": [], "decisioni": []}',
			repeat('d', 64), 0, 0, repeat('1', 64), '{U}', gen_random_uuid(), repeat('4', 64))`)
	rifiutato("una bozza formato 3", "righe formato 3")
}
