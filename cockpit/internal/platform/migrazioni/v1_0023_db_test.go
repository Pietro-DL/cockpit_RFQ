//go:build integrazione

// L4 — la 0023 di V1 (piano della 0023, PM23 §8-§9): le righe di prova comuni, le righe valide e i casi del catalogo
// dei vincoli (M-32: la prova del catalogo di p4_0022_db_test.go copre le tabelle della 22 e della 23). Le prove dei
// trigger stanno in v1_0023_trigger_db_test.go, quelle del ciclo dei file in v1_0023_file_db_test.go, la
// compatibilita' 22 → 23 → 22 in v1_0023_compat_db_test.go. Solo dati sintetici (ACME, BETA, example.com).

package migrazioni_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/migrazioni"
)

// Le sei tabelle nuove della 0023 (PM23 §3).
var tabelle23 = []string{"distinta_importazione", "conferma_strutturale_non_incluso", "file_rfq", "file_rfq_evento",
	"file_rfq_rimozione", "file_rfq_conservazione"}

// Le otto colonne nuove delle tabelle della 22 (PM23 §0, punto 3).
var colonne23 = []struct{ tab, col string }{
	{"distinta_bozza_versione", "importazione_id"}, {"distinta_bozza_versione", "chiave_salvataggio"},
	{"distinta_bozza_versione", "impronta_richiesta"}, {"conferma_strutturale", "importazione_id"},
	{"conferma_strutturale", "importazioni_citate"}, {"conferma_strutturale_componente", "emittente"},
	{"conferma_strutturale_componente", "emittente_presunto"}, {"conferma_strutturale_componente", "namespace"},
}

var sost23 = strings.NewReplacer(
	"{R1}", "00000000-0000-4000-8000-0000002301a1",
	"{R2}", "00000000-0000-4000-8000-0000002301a2",
	"{R3}", "00000000-0000-4000-8000-0000002301a3",
	"{R4}", "00000000-0000-4000-8000-0000002301a4",
	"{RX}", "00000000-0000-4000-8000-0000002301af",
	"{B3}", "00000000-0000-4000-8000-0000002301b3",
	"{B4}", "00000000-0000-4000-8000-0000002301b4",
	"{C3}", "00000000-0000-4000-8000-0000002301c3",
	"{C4}", "00000000-0000-4000-8000-0000002301c4",
	"{K3}", "00000000-0000-4000-8000-0000002301d3",
	"{K4}", "00000000-0000-4000-8000-0000002301d4",
	"{AZ}", "00000000-0000-4000-8000-0000002301e1",
	"{A3}", "00000000-0000-4000-8000-0000002301f3",
	"{A4}", "00000000-0000-4000-8000-0000002301f4",
	"{A5}", "00000000-0000-4000-8000-0000002301f5",
	"{AI}", "00000000-0000-4000-8000-0000002301f6",
	"{AX}", "00000000-0000-4000-8000-0000002301f7",
	"{M2}", "00000000-0000-4000-8000-0000002301e7",
	"{CV2}", "00000000-0000-4000-8000-0000002301e8",
	"{N3}", "00000000-0000-4000-8000-000000230300",
	"{DP3}", "00000000-0000-4000-8000-000000230103",
	"{DP4}", "00000000-0000-4000-8000-000000230104",
	"{DP5}", "00000000-0000-4000-8000-000000230105",
	"{CP1}", "00000000-0000-4000-8000-000000230111",
	"{CP2}", "00000000-0000-4000-8000-000000230112",
	"{F1}", "00000000-0000-4000-8000-000000230121",
	"{V1}", "00000000-0000-4000-8000-000000230122",
	"{TF}", "00000000-0000-4000-8000-0000002302f1",
	"{AF}", "00000000-0000-4000-8000-0000002302f2",
	"{PF}", "00000000-0000-4000-8000-0000002302f3",
)

// e23 sostituisce i segnaposto della 0023, della 0022 e della direttiva 11.
func e23(sql string) string { return sost22.Replace(sost11.Replace(sost23.Replace(sql))) }

func esegui23(t *testing.T, p *pgxpool.Pool, sql string) {
	t.Helper()
	if _, err := p.Exec(context.Background(), e23(sql)); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
}

func uno23[T any](t *testing.T, p *pgxpool.Pool, sql string) T {
	t.Helper()
	var v T
	if err := p.QueryRow(context.Background(), e23(sql)).Scan(&v); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return v
}

// migrazioneNumero: la migrazione incorporata con il numero dato (non l'ultima: dopo la 0023 la 0022 non lo e' piu').
func migrazioneNumero(t *testing.T, n int) migrazioni.Migrazione {
	t.Helper()
	migs, err := migrazioni.Elenca(risorse.FS)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migs {
		if m.Versione == n {
			return m
		}
	}
	t.Fatalf("la migrazione %04d non c'e'", n)
	return migrazioni.Migrazione{}
}

// fsFinoA: un filesystem con le sole migrazioni fino alla n inclusa, cioe' un «binario» che conosce solo quelle.
func fsFinoA(t *testing.T, n int) fstest.MapFS {
	t.Helper()
	migs, err := migrazioni.Elenca(risorse.FS)
	if err != nil {
		t.Fatal(err)
	}
	out := fstest.MapFS{}
	for _, m := range migs {
		if m.Versione <= n {
			out["migrations/"+m.Nome] = &fstest.MapFile{Data: []byte(m.SQL)}
		}
	}
	return out
}

// u23: l'UUID di un segnaposto, come stringa.
func u23(nome string) string { return e23(nome) }

func sha(c string) string { return strings.Repeat(c, 64) }

// rifB: il rif di un nodo dello STEP di A (sha256 b…).
func rifB(chiave string) string { return "nodo:" + sha("b") + ":" + chiave }

// segnaleCordone: il segnale LS-02 di un nodo nel registro (forma di EM §7.1).
func segnaleCordone() map[string]any {
	return map[string]any{"codice": "ancoraggio.possibile_elemento_cad", "regola": "elemento_cad_nome_foglia_v1",
		"versione_servizio": "ancoraggio-3"}
}

// snapshot3 e' lo snapshot formato 3 della bozza B3 di P (e della conferma che la chiude): sorgente A nel gruppo bom
// con il registro R1 (impronta 6…), i nodi del prodotto P (k-p) e del telaio S (k-s) registrati e presunti del
// cliente, un nodo nuovo senza UUID (k-n), l'arco P → S qta 2, e una decisione non_includere sul cordone #31 presa sul
// registro R1.
func snapshot3() map[string]any {
	cliente := "cliente:" + u23("{C}")
	return map[string]any{
		"formato": 3, "prodotto_id": u23("{P}"),
		"sorgente":          map[string]any{"allegato_id": u23("{A}"), "sha256": sha("b"), "gruppo": "bom", "importazione": sha("6")},
		"versione_bom_base": map[string]any{"conferma_id": u23("{C1}"), "numero": 1, "contatore": 0},
		"firma_base":        sha("1"),
		"nodi": []any{
			map[string]any{"chiave_bozza": "k-n", "op": "crea", "tipo": "sciolto", "origine": "utente",
				"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": "ACM-1300", "modo": "manuale"}},
			map[string]any{"chiave_bozza": "k-p", "componente_id": u23("{P}"), "op": "riusa", "tipo": "finito", "descrizione": "Gruppo base",
				"origine": "registrato", "caselle": []any{map[string]any{"allegato_id": u23("{A}"), "gruppo": "bom", "rif": rifB("#1")}},
				"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": "ACM-1000", "rev": "A", "modo": "registrato"}},
			map[string]any{"chiave_bozza": "k-s", "componente_id": u23("{S}"), "op": "riusa", "tipo": "sottoassieme", "descrizione": "Telaio",
				"origine":  "registrato",
				"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": "ACM-1100", "rev": "A", "modo": "registrato"}},
		},
		"archi": []any{map[string]any{"padre": "k-p", "figlio": "k-s", "qta": 2, "origine": "conferma"}},
		"decisioni": []any{map[string]any{"id": "ni-1", "tipo": "non_includere",
			"casella":       map[string]any{"allegato_id": u23("{A}"), "gruppo": "bom", "rif": rifB("#31")},
			"motivo_codice": "elemento_cad", "motivo": "cordone di saldatura", "azione_id": u23("{AZ}"), "importazione": sha("6"),
			"segnale_visto": segnaleCordone()}},
		"documenti_preassociati": []any{},
	}
}

// jsonSQL: un valore Go come letterale jsonb SQL.
func jsonSQL(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return "'" + strings.ReplaceAll(string(b), "'", "''") + "'::jsonb"
}

// nodoDi: il nodo dello snapshot con la chiave data (per cambiarlo in una prova).
func nodoDi(s map[string]any, chiave string) map[string]any {
	for _, n := range s["nodi"].([]any) {
		if m := n.(map[string]any); m["chiave_bozza"] == chiave {
			return m
		}
	}
	return nil
}

// contenutoDimp: il contenuto minimo di un registro coerente con le sue colonne (CHECK ck_dimp_contenuto), come
// espressione SQL; cambi sostituisce i valori (testo SQL) delle chiavi di primo livello o di gruppo.chiave/radice.
func contenutoDimp(prodotto, impronte, gruppo, radice string, cambi map[string]string) string {
	v := map[string]string{
		"formato_registro": "1", "versione_importazione": "'importazione-1'", "thread_id": "'{T}'", "prodotto_id": "'" + prodotto + "'",
		"gruppo.chiave": "'" + gruppo + "'", "gruppo.radice": radice,
		"impronta_contenuto": "repeat('" + impronte + "', 64)", "impronta_evidenze": "repeat('" + impronte + "', 64)",
		"impronta_regole": "repeat('" + impronte + "', 64)",
	}
	for k, x := range cambi {
		v[k] = x
	}
	return fmt.Sprintf(`jsonb_strip_nulls(jsonb_build_object('formato_registro', %s, 'versione_importazione', %s, 'thread_id', %s, 'prodotto_id', %s,
		'gruppo', jsonb_strip_nulls(jsonb_build_object('chiave', %s, 'radice', %s)), 'impronta_contenuto', %s, 'impronta_evidenze', %s,
		'impronta_regole', %s, 'proposta', jsonb_build_object('nodi', '[]'::jsonb)))`,
		v["formato_registro"], v["versione_importazione"], v["thread_id"], v["prodotto_id"], v["gruppo.chiave"], v["gruppo.radice"],
		v["impronta_contenuto"], v["impronta_evidenze"], v["impronta_regole"])
}

// allegati23: gli allegati della 0023, che stanno anche nello schema 22: A3, A4, A5 della RFQ T (2D, STEP, 2D); un
// allegato inline AI; un messaggio M2 della RFQ T2 con un allegato AX che ha lo stesso sha256 di A.
const allegati23 = `INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il) VALUES
   ('{A3}', '{M}', 3, 'ACM-1100_b.pdf', 'pdf', repeat('3', 64), now()),
   ('{A4}', '{M}', 4, 'ACM-2000.stp', 'stp', repeat('4', 64), now()),
   ('{A5}', '{M}', 5, 'ACM-3000.pdf', 'pdf', repeat('5', 64), now());
 INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, natura, ricevuto_il) VALUES
   ('{AI}', '{M}', 6, 'logo.png', 'png', 'inline', now());
 INSERT INTO conversazione (conversazione_id, canale, chiave_esterna, thread_id, primo_messaggio_il)
   VALUES ('{CV2}', 'outlook', 'conv-0023@example.com', '{T2}', now());
 INSERT INTO messaggio (messaggio_id, canale, chiave_esterna, conversazione_id, thread_id, direzione, data_evento, oggetto)
   VALUES ('{M2}', 'outlook', 'msg-0023@example.com', '{CV2}', '{T2}', 'entrata', now(), 'RFQ ACME-9000');
 INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il) VALUES
   ('{AX}', '{M2}', 1, 'ACM-1000.stp', 'stp', repeat('b', 64), now());`

// righe23: gli allegati della 0023 (A3, A4, A5 della RFQ T; un allegato inline AI; un messaggio M2 della RFQ T2 con un
// allegato AX che ha lo stesso sha256 di A), i quattro registri (R1 e R2 di P; R3 di P2 senza fonte; R4 di P2 con la
// stessa fonte, la stessa radice e lo stesso gruppo di R1, cosi' che da R1 lo distingua solo il prodotto: due prodotti
// sullo stesso STEP) e la bozza formato 3 B3 di P alla versione 1 con il registro R1.
func righe23(t *testing.T) []string {
	radice := "'" + rifB("#1") + "'"
	registro := func(id, prodotto, impronta, impronte, gruppo string, conFonte bool) string {
		fonte, shaF, rad, citati := "NULL", "NULL", "NULL", "'{}'::uuid[]"
		radCont := "NULL"
		if conFonte {
			fonte, shaF, rad, citati, radCont = "'{A}'", "repeat('b', 64)", radice, "ARRAY['{A}']::uuid[]", radice
		}
		return `INSERT INTO distinta_importazione (importazione_id, thread_id, prodotto_id, impronta, impronta_contenuto, impronta_evidenze,
			impronta_regole, formato_registro, versione_importazione, gruppo, fonte_allegato_id, fonte_sha256, fonte_radice, allegati_citati, contenuto, creata_da)
			VALUES ('` + id + `', '{T}', '` + prodotto + `', repeat('` + impronta + `', 64), repeat('` + impronte + `', 64), repeat('` + impronte + `', 64),
			repeat('` + impronte + `', 64), 1, 'importazione-1', '` + gruppo + `', ` + fonte + `, ` + shaF + `, ` + rad + `, ` + citati + `, ` +
			contenutoDimp(prodotto, impronte, gruppo, radCont, nil) + `, '{U}')`
	}
	return []string{
		allegati23,
		`INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, origine, esito)
		   VALUES ('{T}', '{A2}', 'conservazione_errore', 'server', '{"causa": "prova del catalogo"}');`,
		registro("{R1}", "{P}", "6", "7", "bom", true) + ";" +
			registro("{R2}", "{P}", "e", "e", "cand:{A}:"+rifB("#1"), true) + ";" +
			registro("{R3}", "{P2}", "f", "f", "bom", false) + ";" +
			registro("{R4}", "{P2}", "9", "9", "bom", true),
		`INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, formato, base_conferma_id, base_numero, base_contatore, firma_base,
			fonte_allegato_id, fonte_sha256, fonte_gruppo, creata_da, aggiornata_da)
		 VALUES ('{B3}', '{T}', '{P}', 3, '{C1}', 1, 0, repeat('1', 64), '{A}', repeat('b', 64), 'bom', '{U}', '{U}');
		 INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_conferma_id, base_numero, base_contatore, firma_base,
			fonte_allegato_id, fonte_sha256, fonte_gruppo, salvata_da, importazione_id, chiave_salvataggio, impronta_richiesta)
		 VALUES ('{B3}', 1, ` + jsonSQL(t, snapshot3()) + `, repeat('5', 64), '{C1}', 1, 0, repeat('1', 64), '{A}', repeat('b', 64), 'bom', '{U}',
			'{R1}', gen_random_uuid(), repeat('4', 64));
		 INSERT INTO distinta_bozza_evento (bozza_id, n, versione, vocabolario, azione, origine, utente_id)
		 VALUES ('{B3}', 1, 1, 2, 'bozza_aperta', 'server', '{U}');`,
	}
}

// schema23ConStoria: le righe della 22 (schema22ConStoria, che con SchemaPulito arriva alla 23) piu' righe23.
func schema23ConStoria(t *testing.T) *pgxpool.Pool {
	t.Helper()
	p := schema22ConStoria(t)
	for _, s := range righe23(t) {
		esegui23(t, p, s)
	}
	return p
}

// ------------------------------------------------------------------ la conferma formato 3 C3 di P

// confermaC3 descrive la conferma formato 3 n. 2 di P che chiude B3 alla versione 1. Ogni campo ha il valore valido;
// le prove ne cambiano qualcuno.
type confermaC3 struct {
	snap       map[string]any
	mappa      map[string]any
	testata    map[string]string // colonna → espressione SQL che sostituisce quella valida
	componenti []string          // righe VALUES di conferma_strutturale_componente
	archi      []string          // righe VALUES di conferma_strutturale_arco
	nonInclusi []string          // righe VALUES di conferma_strutturale_non_incluso
	prima      []string          // istruzioni prima della testata (scritture della working)
	dopo       []string          // istruzioni dopo il contenuto, prima della chiusura della bozza
	nonChiude  bool              // senza la chiusura della bozza
}

const (
	componenteC3P = `('{C3}', '{T}', '{P}', true, 'k-p', 'ACM-1000', 'A', 'Gruppo base', 'finito', 'cliente:{C}', true, NULL)`
	componenteC3S = `('{C3}', '{T}', '{S}', false, 'k-s', 'ACM-1100', 'A', 'Telaio', 'sottoassieme', 'cliente:{C}', true, NULL)`
	arcoC3        = `('{C3}', '{P}', '{S}', 2, 'conferma')`
	nonInclusoC3  = `('{C3}', '{T}', repeat('b', 64), '#31', '{A}', 'bom', 'ni-1', '{AZ}', 'elemento_cad', 'cordone di saldatura',
		'{"codice": "ancoraggio.possibile_elemento_cad", "regola": "elemento_cad_nome_foglia_v1", "versione_servizio": "ancoraggio-3"}')`
)

func nuovaC3() confermaC3 {
	return confermaC3{
		snap:       snapshot3(),
		mappa:      map[string]any{"k-p": u23("{P}"), "k-s": u23("{S}")},
		testata:    map[string]string{},
		componenti: []string{componenteC3P, componenteC3S},
		archi:      []string{arcoC3},
		nonInclusi: []string{nonInclusoC3},
	}
}

// istruzioni: la transazione della conferma, nell'ordine di R5 (working, testata, componenti, archi, nodi non
// inclusi, chiusura della bozza).
func (c confermaC3) istruzioni(t *testing.T) []string {
	t.Helper()
	col := []struct{ nome, expr string }{
		{"conferma_id", "'{C3}'"}, {"thread_id", "'{T}'"}, {"prodotto_id", "'{P}'"}, {"numero", "2"}, {"precedente_id", "'{C1}'"},
		{"formato", "3"}, {"senza_riepilogo_v2", "false"}, {"firma_struttura", "repeat('3', 64)"}, {"firma_anteprima", "repeat('2', 64)"},
		{"base_conferma_id", "'{C1}'"}, {"base_contatore", "0"}, {"firma_base", "repeat('1', 64)"},
		{"contatore", "COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = '{T}'), 0)"},
		{"contatore_dati", "COALESCE((SELECT dati FROM working_contatore WHERE thread_id = '{T}'), 0)"},
		{"bozza_id", "'{B3}'"}, {"bozza_versione", "1"}, {"fonte_allegato_id", "'{A}'"}, {"fonte_sha256", "repeat('b', 64)"},
		{"fonte_gruppo", "'bom'"}, {"origine", `'{"rotta": "motore-a/bom/conferma"}'`}, {"snapshot", jsonSQL(t, c.snap)},
		{"impronta_snapshot", "repeat('5', 64)"}, {"delta", `'{"formato": 3}'`}, {"mappa", jsonSQL(t, c.mappa)}, {"decisioni", "'[]'"},
		{"criterio_verifica", "'nessuno'"}, {"chiave_idempotenza", "'{K3}'"}, {"confermato_da", "'{U}'"},
		{"importazione_id", "'{R1}'"}, {"importazioni_citate", "ARRAY['{R1}']::uuid[]"},
	}
	var cols, vals []string
	visti := map[string]bool{}
	for _, x := range col {
		v := x.expr
		if s, ok := c.testata[x.nome]; ok {
			v = s
		}
		cols, vals = append(cols, x.nome), append(vals, v)
		visti[x.nome] = true
	}
	for k, v := range c.testata {
		if !visti[k] {
			cols, vals = append(cols, k), append(vals, v)
		}
	}
	out := append([]string(nil), c.prima...)
	out = append(out, "INSERT INTO conferma_strutturale ("+strings.Join(cols, ", ")+") SELECT "+strings.Join(vals, ", "))
	if len(c.componenti) > 0 {
		out = append(out, `INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev,
			descrizione, tipo, emittente, emittente_presunto, namespace) VALUES `+strings.Join(c.componenti, ", "))
	}
	if len(c.archi) > 0 {
		out = append(out, `INSERT INTO conferma_strutturale_arco (conferma_id, padre_id, figlio_id, qta, origine) VALUES `+strings.Join(c.archi, ", "))
	}
	if len(c.nonInclusi) > 0 {
		out = append(out, `INSERT INTO conferma_strutturale_non_incluso (conferma_id, thread_id, sha256, chiave, allegato_id, gruppo, decisione_id,
			azione_id, motivo_codice, motivo, segnale) VALUES `+strings.Join(c.nonInclusi, ", "))
	}
	out = append(out, c.dopo...)
	if !c.nonChiude {
		out = append(out, `UPDATE distinta_bozza SET stato = 'confermata', chiusa_da = '{U}' WHERE bozza_id = '{B3}'`)
	}
	return out
}

// ------------------------------------------------------------------ il ciclo dei file: le transizioni complete

// fotografiaFile: la riga di file_rfq di un'occorrenza, con la fotografia letta dall'allegato.
func fotografiaFile(allegato, stato string) string {
	return `INSERT INTO file_rfq (thread_id, allegato_id, stato, versione, messaggio_id, canale, contenitore_id, indice, nome_file, path_interno,
		estensione, sha256, bytes, origine, ricevuto_il, caricato_da, creato_da, aggiornato_da)
	 SELECT m.thread_id, a.allegato_id, '` + stato + `', 1, a.messaggio_id, m.canale, a.contenitore_id, a.indice, a.nome_file, a.path_interno,
		a.estensione, a.sha256, a.bytes, a.origine, a.ricevuto_il, a.caricato_da, '{U}', '{U}'
	   FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE a.allegato_id = '` + allegato + `'`
}

// inAltriFile: le istruzioni di «Metti in Altri file» per un'occorrenza della RFQ T (fase A e B in una transazione):
// la copia verificata, la riga di stato, l'evento.
func inAltriFile(allegato string) []string {
	return []string{
		`INSERT INTO file_rfq_conservazione (thread_id, allegato_id, sha256, bytes, percorso_relativo, deposito_id)
		 SELECT '{T}', a.allegato_id, a.sha256, 10, 'segnaposto', gen_random_uuid() FROM allegato a WHERE a.allegato_id = '` + allegato + `'`,
		`UPDATE file_rfq_conservazione SET stato = 'copiata', sorgente_byte = 'staging'
		  WHERE thread_id = '{T}' AND allegato_id = '` + allegato + `' AND stato = 'in_copia'`,
		fotografiaFile(allegato, "altri_file"),
		`INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, stato_prima, stato_dopo, fotografia, dipendenze, origine, autore_id)
		 VALUES ('{T}', '` + allegato + `', 'altri_file', 'attivo', 'altri_file', '{}', '{}', 'ponte', '{U}')`,
	}
}

// scartato: le istruzioni di «Scarta» (tempo 1) per un'occorrenza della RFQ T: la riga di stato, l'evento, la
// rimozione da eseguire.
func scartato(allegato string) []string {
	return []string{
		fotografiaFile(allegato, "scartato"),
		`INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, stato_prima, stato_dopo, motivo, fotografia, dipendenze, origine, autore_id)
		 VALUES ('{T}', '` + allegato + `', 'scarta', 'attivo', 'scartato', 'non serve alla RFQ', '{}', '{}', 'ponte', '{U}')`,
		`INSERT INTO file_rfq_rimozione (thread_id, allegato_id, evento_id, sha256)
		 SELECT e.thread_id, e.allegato_id, e.evento_id, a.sha256 FROM file_rfq_evento e JOIN allegato a ON a.allegato_id = e.allegato_id
		  WHERE e.allegato_id = '` + allegato + `' AND e.azione = 'scarta'`,
	}
}

// ------------------------------------------------------------------ il catalogo dei vincoli (M-32)

// La riga formato 3 completa di conferma_strutturale per i casi del catalogo (trigger spenti): passa ogni vincolo.
var cambiF3 = map[string]string{
	"formato": "3", "bozza_id": "'{B3}'", "bozza_versione": "1", "importazione_id": "'{R1}'",
	"importazioni_citate": "ARRAY['{R1}']::uuid[]", "fonte_allegato_id": "'{A}'", "fonte_sha256": "repeat('b', 64)", "fonte_gruppo": "'bom'",
	"snapshot": snap3Compatto,
}

// snap3Compatto: lo snapshot formato 3 piu' piccolo che passa i CHECK (nodi, archi e decisioni in array, nodi oggetto
// con l'identita' oggetto).
const snap3Compatto = `jsonb_build_object('formato', 3, 'prodotto_id', '{P}'::text, 'nodi', jsonb_build_array(jsonb_build_object('chiave_bozza', 'k-p',
	'identita', jsonb_build_object('codice', 'ACM-1000'))), 'archi', '[]'::jsonb, 'decisioni', '[]'::jsonb)`

// snap3Con: lo snapshot formato 3 della versione per i casi del catalogo, con i nodi dati (testo JSON) e, se non vuoti,
// archi e decisioni dati; vuoti = assenti.
func snap3Con(nodi, archi, decisioni string) string {
	s := `'{"formato": 3, "nodi": ` + nodi
	if archi != "" {
		s += `, "archi": ` + archi
	}
	if decisioni != "" {
		s += `, "decisioni": ` + decisioni
	}
	return s + `}'`
}

// confSnap3Con: come snap3Con, per lo snapshot di una conferma (con il prodotto P).
func confSnap3Con(nodi, archi, decisioni string) string {
	return strings.Replace(snap3Con(nodi, archi, decisioni), `{"formato": 3, `, `{"formato": 3, "prodotto_id": "{P}", `, 1)
}

// La versione formato 3 con la chiave del salvataggio, che passa ck_dbv_chiave_salvataggio (i casi del catalogo su
// ck_dbv_snapshot_formato la portano tutti, cosi' il solo vincolo violato e' quello).
func versioneF3(snapshot string) map[string]string {
	return map[string]string{"snapshot": snapshot, "chiave_salvataggio": "gen_random_uuid()", "impronta_richiesta": "repeat('4', 64)"}
}

func con(base map[string]string, cambi map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range cambi {
		out[k] = v
	}
	return out
}

// righeValide23: una riga valida per ogni tabella nuova (le prove ne cambiano qualche colonna).
var righeValide23 = map[string]riga{
	"distinta_importazione": {{"thread_id", "'{T}'"}, {"prodotto_id", "'{P}'"}, {"impronta", "repeat('0', 64)"},
		{"impronta_contenuto", "repeat('1', 64)"}, {"impronta_evidenze", "repeat('1', 64)"}, {"impronta_regole", "repeat('1', 64)"},
		{"formato_registro", "1"}, {"versione_importazione", "'importazione-1'"}, {"gruppo", "'bom'"}, {"fonte_allegato_id", "'{A}'"},
		{"fonte_sha256", "repeat('b', 64)"}, {"fonte_radice", "'nodo:' || repeat('b', 64) || ':#1'"}, {"allegati_citati", "ARRAY['{A}']::uuid[]"},
		{"contenuto", contenutoDimp("{P}", "1", "bom", "'nodo:' || repeat('b', 64) || ':#1'", nil)}, {"creata_da", "'{U}'"}},
	"conferma_strutturale_non_incluso": {{"conferma_id", "'{C1}'"}, {"thread_id", "'{T}'"}, {"sha256", "repeat('b', 64)"}, {"chiave", "'#99'"},
		{"allegato_id", "'{A}'"}, {"gruppo", "'bom'"}, {"decisione_id", "'ni-9'"}, {"azione_id", "gen_random_uuid()"},
		{"motivo_codice", "'altro'"}, {"motivo", "'prova del catalogo'"}, {"segnale", "NULL"}, {"righe_chiuse", "'{}'"}, {"archi_chiusi", "'[]'"}},
	"file_rfq": {{"thread_id", "'{T}'"}, {"allegato_id", "'{A2}'"}, {"stato", "'altri_file'"}, {"versione", "1"}, {"messaggio_id", "'{M}'"},
		{"canale", "'outlook'"}, {"contenitore_id", "NULL"}, {"indice", "2"}, {"nome_file", "'ACM-1100.pdf'"}, {"path_interno", "NULL"},
		{"estensione", "'pdf'"}, {"sha256", "repeat('d', 64)"}, {"bytes", "NULL"}, {"origine", "'outlook'"}, {"ricevuto_il", "now()"},
		{"caricato_da", "NULL"}, {"creato_da", "'{U}'"}, {"aggiornato_da", "'{U}'"}},
	"file_rfq_evento": {{"thread_id", "'{T}'"}, {"allegato_id", "'{A2}'"}, {"azione", "'altri_file'"}, {"stato_prima", "'attivo'"},
		{"stato_dopo", "'altri_file'"}, {"motivo", "NULL"}, {"fotografia", "'{}'"}, {"dipendenze", "'{}'"}, {"esito", "NULL"},
		{"altro_allegato_id", "NULL"}, {"origine", "'ponte'"}, {"autore_id", "'{U}'"}, {"chiave_idempotenza", "gen_random_uuid()"}},
	"file_rfq_rimozione": {{"thread_id", "'{T}'"}, {"allegato_id", "'{A2}'"}, {"evento_id", "(SELECT min(evento_id) FROM file_rfq_evento)"},
		{"sha256", "repeat('d', 64)"}, {"percorso_trovato", "NULL"}, {"stato", "'in_attesa'"}, {"motivo", "'da_eseguire'"},
		{"ritentabile", "true"}, {"errore", "NULL"}, {"tentativi", "0"}, {"concluso_il", "NULL"}},
	// conservazione_id esplicito (OVERRIDING SYSTEM VALUE, insertCon): il percorso lo nomina, e con i trigger spenti
	// nessuno lo calcola
	"file_rfq_conservazione": {{"conservazione_id", "99999"}, {"thread_id", "'{T}'"}, {"allegato_id", "'{A2}'"}, {"sha256", "repeat('d', 64)"},
		{"bytes", "10"}, {"percorso_relativo", `'{T}\{A2}.99999.pdf'`}, {"deposito_id", "gen_random_uuid()"}, {"sorgente_byte", "NULL"},
		{"stato", "'in_copia'"}, {"causa", "NULL"}, {"errore", "NULL"}, {"tentativi", "0"}, {"verificato_il", "NULL"}, {"concluso_il", "NULL"}},
}

// rigaValida: la riga valida di una tabella della 22 o della 23.
func rigaValida(tab string) riga {
	if r, ok := righeValide22[tab]; ok {
		return r
	}
	return righeValide23[tab]
}

// La riga del catalogo che non esiste: un UUID fisso (serve dove un CHECK lo ripete, come il percorso della copia).
const uuidNuovo23 = "'00000000-0000-4000-8000-0000002301ff'"

var casiVincoli23 = []casoVincolo{
	// M-01: la testata accetta 2 e 3, non 1 (caso della 22) ne' 4
	{"distinta_bozza", map[string]string{"formato": "4"}, "ck_distinta_bozza_formato", nil, false},

	// M-02, M-03, M-30: versioni della bozza. Ogni caso formato 3 viola una sola condizione di ck_dbv_snapshot_formato:
	// gli altri campi passano (nodi, archi e decisioni in array, nodi oggetto con l'identita' oggetto)
	{"distinta_bozza_versione", map[string]string{"snapshot": `'{"formato": 4}'`}, "ck_dbv_snapshot_formato", nil, false},
	{"distinta_bozza_versione", map[string]string{"importazione_id": "'{R1}'"}, "ck_dbv_snapshot_formato", nil, false},
	{"distinta_bozza_versione", versioneF3(snap3Con(`[{"chiave_bozza": "k-p"}]`, "[]", "[]")), "ck_dbv_snapshot_formato", nil, false},
	{"distinta_bozza_versione", versioneF3(snap3Con(`{"k-p": {"identita": {}}}`, "[]", "[]")), "ck_dbv_snapshot_formato", nil, false},
	// (proposte di emendamento del resoconto della corsia persistenza: nodi, archi o decisioni assenti o non array; un
	// nodo che non e' un oggetto, anche un array che ne avvolge uno; un'identita' che non e' un oggetto)
	{"distinta_bozza_versione", versioneF3(`'{"formato": 3, "archi": [], "decisioni": []}'`), "ck_dbv_snapshot_formato", nil, false},
	{"distinta_bozza_versione", versioneF3(snap3Con(`[{"identita": {}}]`, "", "[]")), "ck_dbv_snapshot_formato", nil, false},
	{"distinta_bozza_versione", versioneF3(snap3Con(`[{"identita": {}}]`, "{}", "[]")), "ck_dbv_snapshot_formato", nil, false},
	{"distinta_bozza_versione", versioneF3(snap3Con(`[{"identita": {}}]`, "[]", "")), "ck_dbv_snapshot_formato", nil, false},
	{"distinta_bozza_versione", versioneF3(snap3Con(`[{"identita": {}}]`, "[]", "{}")), "ck_dbv_snapshot_formato", nil, false},
	{"distinta_bozza_versione", versioneF3(snap3Con(`[[{"identita": {}}]]`, "[]", "[]")), "ck_dbv_snapshot_formato", nil, false},
	{"distinta_bozza_versione", versioneF3(snap3Con(`[{"identita": {}}, []]`, "[]", "[]")), "ck_dbv_snapshot_formato", nil, false},
	{"distinta_bozza_versione", versioneF3(snap3Con(`["k-p"]`, "[]", "[]")), "ck_dbv_snapshot_formato", nil, false},
	{"distinta_bozza_versione", versioneF3(snap3Con(`[{"identita": null}]`, "[]", "[]")), "ck_dbv_snapshot_formato", nil, false},
	{"distinta_bozza_versione", versioneF3(snap3Con(`[{"identita": "x"}]`, "[]", "[]")), "ck_dbv_snapshot_formato", nil, false},
	{"distinta_bozza_versione", versioneF3(snap3Con(`[{"identita": [{}]}]`, "[]", "[]")), "ck_dbv_snapshot_formato", nil, false},
	{"distinta_bozza_versione", con(versioneF3(snap3Con(`[{"chiave_bozza": "k-p", "identita": {}}]`, "[]", "[]")),
		map[string]string{"importazione_id": uuidNuovo23}), "fk_dbv_importazione", nil, false},
	{"distinta_bozza_versione", map[string]string{"snapshot": snap3Con(`[{"chiave_bozza": "k-p", "identita": {}}]`, "[]", "[]")},
		"ck_dbv_chiave_salvataggio", nil, false},
	{"distinta_bozza_versione", map[string]string{"chiave_salvataggio": "gen_random_uuid()"}, "ck_dbv_chiave_salvataggio", nil, false},
	{"distinta_bozza_versione", map[string]string{"chiave_salvataggio": "gen_random_uuid()", "impronta_richiesta": "'x'"}, "ck_dbv_chiave_salvataggio", nil, false},

	// M-05, M-06, M-06b, M-07, M-08, M-09, M-09b, M-18: la conferma
	{"conferma_strutturale", map[string]string{"formato": "4", "chiave_idempotenza": "NULL"}, "ck_conferma_formato", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"senza_riepilogo_v2": "true"}), "ck_conferma_formato", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"firma_anteprima": "NULL"}), "ck_conferma_firme", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"bozza_id": "NULL", "bozza_versione": "NULL"}), "ck_conferma_bozza", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"bozza_versione": "NULL"}), "ck_conferma_bozza", nil, false},
	{"conferma_strutturale", map[string]string{"formato": "1", "senza_riepilogo_v2": "true", "chiave_idempotenza": "NULL", "bozza_id": "'{B1}'",
		"bozza_versione": "1"}, "ck_conferma_bozza", nil, false},
	{"conferma_strutturale", map[string]string{"snapshot": snap3Compatto}, "ck_conferma_snapshot", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"snapshot": "jsonb_build_object('formato', 2, 'prodotto_id', '{P}'::text)"}),
		"ck_conferma_snapshot", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"snapshot": strings.Replace(snap3Compatto, "'{P}'::text", "'{P2}'::text", 1)}),
		"ck_conferma_snapshot", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"snapshot": confSnap3Con(`[{"chiave_bozza": "k-p"}]`, "[]", "[]")}),
		"ck_conferma_snapshot", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"snapshot": `'{"formato": 3, "prodotto_id": "{P}", "archi": [], "decisioni": []}'`}),
		"ck_conferma_snapshot", nil, false},
	// (le stesse proposte di emendamento della versione: archi e decisioni in array, nodi e identita' oggetti)
	{"conferma_strutturale", con(cambiF3, map[string]string{"snapshot": confSnap3Con(`[{"identita": {}}]`, "", "[]")}), "ck_conferma_snapshot", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"snapshot": confSnap3Con(`[{"identita": {}}]`, "[]", "")}), "ck_conferma_snapshot", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"snapshot": confSnap3Con(`[{"identita": {}}]`, "[]", "{}")}), "ck_conferma_snapshot", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"snapshot": confSnap3Con(`[[{"identita": {}}]]`, "[]", "[]")}), "ck_conferma_snapshot", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"snapshot": confSnap3Con(`[{"identita": null}]`, "[]", "[]")}), "ck_conferma_snapshot", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"snapshot": confSnap3Con(`[{"identita": "x"}]`, "[]", "[]")}), "ck_conferma_snapshot", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"chiave_idempotenza": "NULL"}), "ck_conferma_chiave", nil, false},
	{"conferma_strutturale", map[string]string{"formato": "1", "senza_riepilogo_v2": "true"}, "ck_conferma_chiave", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"importazione_id": uuidNuovo23, "importazioni_citate": "ARRAY[" + uuidNuovo23 + "]::uuid[]"}),
		"fk_conferma_importazione", nil, false},
	{"conferma_strutturale", map[string]string{"importazione_id": "'{R1}'"}, "ck_conferma_importazione", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"importazioni_citate": "NULL"}), "ck_conferma_importazioni_citate", nil, false},
	{"conferma_strutturale", map[string]string{"importazioni_citate": "ARRAY['{R1}']::uuid[]"}, "ck_conferma_importazioni_citate", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"importazioni_citate": "ARRAY['{R2}']::uuid[]"}), "ck_conferma_importazioni_citate", nil, false},
	{"conferma_strutturale", con(cambiF3, map[string]string{"mono_pezzo": "true", "criterio_verifica": "'mono_pezzo'",
		"mono_pezzo_motivo": "'pezzo unico'"}), "ck_conferma_mono_pezzo_snapshot", nil, false},

	// M-12, M-34: l'identita' certificata con la regola della grammatica
	{"conferma_strutturale_componente", map[string]string{"emittente": "'boh'", "emittente_presunto": "true"}, "ck_csc_identita", nil, false},
	{"conferma_strutturale_componente", map[string]string{"emittente": "'partner:Beta'", "emittente_presunto": "false"}, "ck_csc_identita", nil, false},
	{"conferma_strutturale_componente", map[string]string{"emittente": "'partner:be ta'", "emittente_presunto": "false"}, "ck_csc_identita", nil, false},
	{"conferma_strutturale_componente", map[string]string{"emittente": "'partner:a/b'", "emittente_presunto": "false"}, "ck_csc_identita", nil, false},
	{"conferma_strutturale_componente", map[string]string{"emittente": "'partner:a:b'", "emittente_presunto": "false"}, "ck_csc_identita", nil, false},
	{"conferma_strutturale_componente", map[string]string{"emittente": "'partner:'", "emittente_presunto": "false"}, "ck_csc_identita", nil, false},
	{"conferma_strutturale_componente", map[string]string{"emittente": "'cliente:ACME'", "emittente_presunto": "true"}, "ck_csc_identita", nil, false},
	{"conferma_strutturale_componente", map[string]string{"namespace": "'acme'"}, "ck_csc_identita", nil, false},
	{"conferma_strutturale_componente", map[string]string{"emittente_presunto": "true"}, "ck_csc_identita", nil, false},
	{"conferma_strutturale_componente", map[string]string{"emittente": "'cliente:{C}'", "emittente_presunto": "true", "namespace": "' '"},
		"ck_csc_identita", nil, false},
	{"conferma_strutturale_componente", map[string]string{"emittente": "'partner:' || repeat('a', 201)", "emittente_presunto": "false"},
		"ck_csc_identita", nil, false},
	{"conferma_strutturale_componente", map[string]string{"emittente": "'partner:beta'", "emittente_presunto": "false",
		"namespace": "repeat('n', 501)"}, "ck_csc_identita", nil, false},
	// un id o un namespace di soli bianchi Unicode (tab, NBSP): vuoti per strings.TrimSpace della grammatica e del Go
	{"conferma_strutturale_componente", map[string]string{"emittente": `E'partner:\t'`, "emittente_presunto": "false"}, "ck_csc_identita", nil, false},
	{"conferma_strutturale_componente", map[string]string{"emittente": "'cliente:{C}'", "emittente_presunto": "true", "namespace": `E'\u00a0\t'`},
		"ck_csc_identita", nil, false},

	// M-16: i gesti
	{"esito_intento", map[string]string{"gesto": "'conferma_bom_v3'"}, "ck_esito_intento_conferma", nil, false},
	{"esito_intento", map[string]string{"gesto": "'scarta_file'", "conferma_id": "'{C1}'"}, "ck_esito_intento_conferma", nil, false},

	// M-14, M-29: il registro d'importazione
	{"distinta_importazione", map[string]string{"impronta": "repeat('X', 64)"}, "ck_dimp_impronte", nil, false},
	{"distinta_importazione", map[string]string{"formato_registro": "2",
		"contenuto": contenutoDimp("{P}", "1", "bom", "'nodo:' || repeat('b', 64) || ':#1'", map[string]string{"formato_registro": "2"})},
		"ck_dimp_formato", nil, false},
	{"distinta_importazione", map[string]string{"versione_importazione": "'importazione-0'",
		"contenuto": contenutoDimp("{P}", "1", "bom", "'nodo:' || repeat('b', 64) || ':#1'", map[string]string{"versione_importazione": "'importazione-0'"})},
		"ck_dimp_formato", nil, false},
	{"distinta_importazione", map[string]string{"gruppo": "'senza'",
		"contenuto": contenutoDimp("{P}", "1", "senza", "'nodo:' || repeat('b', 64) || ':#1'", nil)}, "ck_dimp_gruppo", nil, false},
	{"distinta_importazione", map[string]string{"gruppo": "' '",
		"contenuto": contenutoDimp("{P}", "1", " ", "'nodo:' || repeat('b', 64) || ':#1'", nil)}, "ck_dimp_gruppo", nil, false},
	{"distinta_importazione", map[string]string{"fonte_allegato_id": "NULL", "fonte_sha256": "NULL", "allegati_citati": "'{}'"}, "ck_dimp_fonte", nil, false},
	{"distinta_importazione", map[string]string{"fonte_radice": "NULL", "contenuto": contenutoDimp("{P}", "1", "bom", "NULL", nil)},
		"ck_dimp_fonte", nil, false},
	{"distinta_importazione", map[string]string{"fonte_radice": "'radice:1'", "contenuto": contenutoDimp("{P}", "1", "bom", "'radice:1'", nil)},
		"ck_dimp_fonte", nil, false},
	{"distinta_importazione", map[string]string{"fonte_sha256": "'x'"}, "ck_dimp_fonte", nil, false},
	{"distinta_importazione", map[string]string{"allegati_citati": "'{}'"}, "ck_dimp_citati", nil, false},
	{"distinta_importazione", map[string]string{"contenuto": contenutoDimp("{P}", "1", "bom", "'nodo:' || repeat('b', 64) || ':#1'",
		map[string]string{"impronta_contenuto": "repeat('2', 64)"})}, "ck_dimp_contenuto", nil, false},
	{"distinta_importazione", map[string]string{"contenuto": contenutoDimp("{P}", "1", "cand:x", "'nodo:' || repeat('b', 64) || ':#1'", nil)},
		"ck_dimp_contenuto", nil, false},
	{"distinta_importazione", map[string]string{"contenuto": contenutoDimp("{P}", "1", "bom", "'nodo:' || repeat('b', 64) || ':#2'", nil)},
		"ck_dimp_contenuto", nil, false},
	{"distinta_importazione", map[string]string{"contenuto": "'[]'"}, "ck_dimp_contenuto", nil, false},
	{"distinta_importazione", map[string]string{"thread_id": "'{TF}'", "contenuto": contenutoDimp("{P}", "1", "bom",
		"'nodo:' || repeat('b', 64) || ':#1'", map[string]string{"thread_id": "'{TF}'"})}, "distinta_importazione_thread_id_fkey",
		[]string{"distinta_importazione.fk_dimp_prodotto"}, false},
	{"distinta_importazione", map[string]string{"fonte_allegato_id": uuidNuovo23, "allegati_citati": "ARRAY[" + uuidNuovo23 + "]::uuid[]"},
		"distinta_importazione_fonte_allegato_id_fkey", nil, false},
	{"distinta_importazione", map[string]string{"creata_da": nuovo}, "distinta_importazione_creata_da_fkey", nil, false},
	{"distinta_importazione", map[string]string{"prodotto_id": "'{PF}'", "contenuto": contenutoDimp("{PF}", "1", "bom",
		"'nodo:' || repeat('b', 64) || ':#1'", nil)}, "fk_dimp_prodotto", nil, false},
	{"distinta_importazione", map[string]string{"impronta": "repeat('6', 64)"}, "ux_dimp_impronta", nil, false},

	// M-15: i nodi non inclusi
	{"conferma_strutturale_non_incluso", map[string]string{"sha256": "'x'"}, "ck_csni_sha", nil, false},
	{"conferma_strutturale_non_incluso", map[string]string{"chiave": "' '"}, "ck_csni_chiave", nil, false},
	{"conferma_strutturale_non_incluso", map[string]string{"chiave": "repeat('c', 331)"}, "ck_csni_chiave", nil, false},
	{"conferma_strutturale_non_incluso", map[string]string{"gruppo": "' '"}, "ck_csni_gruppo", nil, false},
	{"conferma_strutturale_non_incluso", map[string]string{"decisione_id": "'ni 9'"}, "ck_csni_decisione", nil, false},
	{"conferma_strutturale_non_incluso", map[string]string{"motivo_codice": "'boh'"}, "ck_csni_motivo", nil, false},
	{"conferma_strutturale_non_incluso", map[string]string{"motivo": "' '"}, "ck_csni_motivo", nil, false},
	{"conferma_strutturale_non_incluso", map[string]string{"segnale": "'[]'"}, "ck_csni_json", nil, false},
	{"conferma_strutturale_non_incluso", map[string]string{"archi_chiusi": "'{}'"}, "ck_csni_json", nil, false},
	{"conferma_strutturale_non_incluso", map[string]string{"conferma_id": nuovo}, "fk_csni_conferma", nil, false},
	{"conferma_strutturale_non_incluso", map[string]string{"allegato_id": nuovo}, "conferma_strutturale_non_incluso_allegato_id_fkey", nil, false},

	// M-21: lo stato dei file
	{"file_rfq", map[string]string{"stato": "'boh'"}, "ck_file_rfq_stato", nil, false},
	{"file_rfq", map[string]string{"versione": "0"}, "ck_file_rfq_versione", nil, false},
	{"file_rfq", map[string]string{"sha256": "'x'"}, "ck_file_rfq_sha", nil, false},
	{"file_rfq", map[string]string{"bytes": "-1"}, "ck_file_rfq_bytes", nil, false},
	{"file_rfq", map[string]string{"thread_id": nuovo}, "file_rfq_thread_id_fkey", nil, false},
	{"file_rfq", map[string]string{"allegato_id": nuovo}, "file_rfq_allegato_id_fkey", nil, false},
	{"file_rfq", map[string]string{"messaggio_id": nuovo}, "file_rfq_messaggio_id_fkey", nil, false},
	{"file_rfq", map[string]string{"contenitore_id": nuovo}, "file_rfq_contenitore_id_fkey", nil, false},
	{"file_rfq", map[string]string{"caricato_da": nuovo}, "file_rfq_caricato_da_fkey", nil, false},
	{"file_rfq", map[string]string{"creato_da": nuovo}, "file_rfq_creato_da_fkey", nil, false},
	{"file_rfq", map[string]string{"aggiornato_da": nuovo}, "file_rfq_aggiornato_da_fkey", nil, false},

	// M-25: il registro dei file (ck_fre_coerente viene prima nell'ordine dei controlli: dove serve si toglie)
	{"file_rfq_evento", map[string]string{"azione": "'boh'"}, "ck_fre_azione", nil, false},
	{"file_rfq_evento", map[string]string{"stato_prima": "'boh'"}, "ck_fre_stati", []string{"file_rfq_evento.ck_fre_coerente"}, false},
	{"file_rfq_evento", map[string]string{"origine": "'boh'"}, "ck_fre_origine", []string{"file_rfq_evento.ck_fre_coerente"}, false},
	{"file_rfq_evento", map[string]string{"dipendenze": "NULL"}, "ck_fre_coerente", nil, false},
	{"file_rfq_evento", map[string]string{"stato_dopo": "NULL"}, "ck_fre_coerente", nil, false},
	{"file_rfq_evento", map[string]string{"azione": "'scarta'", "stato_dopo": "'scartato'"}, "ck_fre_coerente", nil, false},
	{"file_rfq_evento", map[string]string{"azione": "'riattiva'", "stato_prima": "'scartato'", "stato_dopo": "'attivo'"}, "ck_fre_coerente", nil, false},
	{"file_rfq_evento", map[string]string{"azione": "'rimozione_eseguita'", "stato_prima": "NULL", "stato_dopo": "NULL", "origine": "'server'",
		"autore_id": "NULL"}, "ck_fre_coerente", nil, false},
	{"file_rfq_evento", map[string]string{"autore_id": "NULL"}, "ck_fre_coerente", nil, false},
	{"file_rfq_evento", map[string]string{"azione": "'ricaricato_dopo_scarto'", "stato_prima": "NULL", "stato_dopo": "NULL", "origine": "'server'",
		"autore_id": "NULL", "altro_allegato_id": "'{A}'"}, "ck_fre_coerente", nil, false},
	{"file_rfq_evento", map[string]string{"motivo": "' '"}, "ck_fre_motivo", nil, false},
	{"file_rfq_evento", map[string]string{"fotografia": "'[]'"}, "ck_fre_json", nil, false},
	{"file_rfq_evento", map[string]string{"thread_id": nuovo}, "file_rfq_evento_thread_id_fkey", nil, false},
	{"file_rfq_evento", map[string]string{"allegato_id": nuovo}, "file_rfq_evento_allegato_id_fkey", nil, false},
	{"file_rfq_evento", map[string]string{"altro_allegato_id": nuovo}, "file_rfq_evento_altro_allegato_id_fkey", nil, false},
	{"file_rfq_evento", map[string]string{"autore_id": nuovo}, "file_rfq_evento_autore_id_fkey", nil, false},

	// M-26: la rimozione
	{"file_rfq_rimozione", map[string]string{"stato": "'boh'"}, "ck_rim_stato", []string{"file_rfq_rimozione.ck_rim_motivo"}, false},
	{"file_rfq_rimozione", map[string]string{"stato": "'eseguita'", "motivo": "'lavoro_in_corso'", "concluso_il": "now()"}, "ck_rim_motivo", nil, false},
	{"file_rfq_rimozione", map[string]string{"stato": "'non_necessaria_condiviso'", "motivo": "'pin'", "concluso_il": "now()"}, "ck_rim_motivo", nil, false},
	{"file_rfq_rimozione", map[string]string{"ritentabile": "false"}, "ck_rim_ritentabile", nil, false},
	{"file_rfq_rimozione", map[string]string{"stato": "'errore'", "motivo": "'fuori_cache'", "ritentabile": "true", "errore": "'x'"},
		"ck_rim_ritentabile", nil, false},
	{"file_rfq_rimozione", map[string]string{"stato": "'errore'", "motivo": "'errore_io'"}, "ck_rim_errore", nil, false},
	{"file_rfq_rimozione", map[string]string{"errore": "'x'"}, "ck_rim_errore", nil, false},
	{"file_rfq_rimozione", map[string]string{"tentativi": "-1"}, "ck_rim_tentativi", nil, false},
	{"file_rfq_rimozione", map[string]string{"sha256": "'x'"}, "ck_rim_sha", nil, false},
	{"file_rfq_rimozione", map[string]string{"concluso_il": "now()"}, "ck_rim_concluso", nil, false},
	{"file_rfq_rimozione", map[string]string{"evento_id": "999999999"}, "file_rfq_rimozione_evento_id_fkey", nil, false},
	{"file_rfq_rimozione", map[string]string{}, "fk_rimozione_file", nil, true},

	// M-27: la conservazione
	{"file_rfq_conservazione", map[string]string{"stato": "'boh'"}, "ck_cons_stato", nil, false},
	{"file_rfq_conservazione", map[string]string{"sha256": "'x'"}, "ck_cons_sha", nil, false},
	{"file_rfq_conservazione", map[string]string{"percorso_relativo": `'..\{A2}.99999.pdf'`}, "ck_cons_percorso", nil, false},
	{"file_rfq_conservazione", map[string]string{"percorso_relativo": `'C:\{T}\{A2}.99999.pdf'`}, "ck_cons_percorso", nil, false},
	{"file_rfq_conservazione", map[string]string{"percorso_relativo": `'\{T}\{A2}.99999.pdf'`}, "ck_cons_percorso", nil, false},
	{"file_rfq_conservazione", map[string]string{"percorso_relativo": `'{T}\{A}.99999.pdf'`}, "ck_cons_percorso", nil, false},
	{"file_rfq_conservazione", map[string]string{"percorso_relativo": `'{T}\{A2}.99998.pdf'`}, "ck_cons_percorso", nil, false},
	{"file_rfq_conservazione", map[string]string{"percorso_relativo": `'{T}\{A2}.pdf'`}, "ck_cons_percorso", nil, false},
	{"file_rfq_conservazione", map[string]string{"percorso_relativo": `'{T}\{A2}.99999.pdf\..\x'`}, "ck_cons_percorso", nil, false},
	{"file_rfq_conservazione", map[string]string{"sorgente_byte": "'boh'"}, "ck_cons_sorgente", nil, false},
	{"file_rfq_conservazione", map[string]string{"stato": "'errore_copia'", "concluso_il": "now()"}, "ck_cons_causa", nil, false},
	{"file_rfq_conservazione", map[string]string{"stato": "'copiata'", "verificato_il": "now()", "sorgente_byte": "'staging'", "causa": "'spazio'"},
		"ck_cons_causa", nil, false},
	{"file_rfq_conservazione", map[string]string{"causa": "'boh'", "stato": "'da_rilasciare'"}, "ck_cons_causa", nil, false},
	{"file_rfq_conservazione", map[string]string{"stato": "'copiata'", "sorgente_byte": "'staging'"}, "ck_cons_copiata", nil, false},
	{"file_rfq_conservazione", map[string]string{"stato": "'copiata'", "verificato_il": "now()"}, "ck_cons_copiata", nil, false},
	{"file_rfq_conservazione", map[string]string{"errore": "repeat('e', 2001)"}, "ck_cons_errore", nil, false},
	{"file_rfq_conservazione", map[string]string{"tentativi": "-1"}, "ck_cons_tentativi", nil, false},
	{"file_rfq_conservazione", map[string]string{"concluso_il": "now()"}, "ck_cons_concluso", nil, false},
	{"file_rfq_conservazione", map[string]string{"thread_id": "'{TF}'", "percorso_relativo": `'{TF}\{A2}.99999.pdf'`},
		"file_rfq_conservazione_thread_id_fkey", nil, false},
	{"file_rfq_conservazione", map[string]string{"allegato_id": "'{AF}'", "percorso_relativo": `'{T}\{AF}.99999.pdf'`},
		"file_rfq_conservazione_allegato_id_fkey", nil, false},
}
