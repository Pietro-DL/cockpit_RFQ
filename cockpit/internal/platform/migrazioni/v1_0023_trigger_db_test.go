//go:build integrazione

// L4 — i trigger della 0023 (piano della 0023, PM23 §8): versioni della bozza formato 3 (M-04, M-04b), nascita,
// contenuto e controllo al COMMIT della conferma formato 3 (M-09b, M-10, M-11, M-13), registro d'importazione (M-14),
// nodi non inclusi (M-15), gesti degli intenti (M-16), fonte secondaria su un nodo formato 3 (M-17), la conferma scritta
// per ultima (M-19), la variante (M-20), la chiave del salvataggio (M-30), l'emittente della grammatica (M-34). Le righe
// di base sono quelle di v1_0023_db_test.go (schema23ConStoria).

package migrazioni_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	risorse "promatec/cockpit"
	"promatec/cockpit/internal/platform/migrazioni"
	"promatec/cockpit/internal/platform/testutil"
)

// rifiuto23 esegue le istruzioni in una transazione e vuole il rifiuto con il codice (SQLSTATE o P4Xnn) e la frase
// attesi, sull'istruzione o al COMMIT. La transazione si annulla comunque.
func rifiuto23(t *testing.T, p *pgxpool.Pool, nome, codiceAtteso, frase string, istruzioni ...string) {
	t.Helper()
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, s := range istruzioni {
		if _, err = tx.Exec(ctx, e23(s)); err != nil {
			break
		}
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err == nil || codice(err) != codiceAtteso || !strings.Contains(err.Error(), frase) {
		t.Errorf("%s: atteso %s («%s»), ottenuto %v", nome, codiceAtteso, frase, err)
	}
}

// riuscita23 esegue le istruzioni in una transazione e la conferma.
func riuscita23(t *testing.T, p *pgxpool.Pool, nome string, istruzioni ...string) {
	t.Helper()
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, s := range istruzioni {
		if _, err := tx.Exec(ctx, e23(s)); err != nil {
			t.Fatalf("%s: %v\n%s", nome, err, s)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("%s: al COMMIT %v", nome, err)
	}
}

// versione2B3: la versione 2 della bozza B3 con lo snapshot e il registro dati (registro: espressione SQL).
func versione2B3(t *testing.T, snap map[string]any, registro string) string {
	return `INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_conferma_id, base_numero, base_contatore, firma_base,
		fonte_allegato_id, fonte_sha256, fonte_gruppo, salvata_da, importazione_id, chiave_salvataggio, impronta_richiesta)
	 VALUES ('{B3}', 2, ` + jsonSQL(t, snap) + `, repeat('c', 64), '{C1}', 1, 0, repeat('1', 64), '{A}', repeat('b', 64), 'bom', '{U}', ` +
		registro + `, gen_random_uuid(), repeat('4', 64))`
}

// M-04, M-04b: la versione formato 3 ha lo snapshot del formato della testata, cita il registro dello snapshot (stessa
// RFQ, stesso prodotto, impronta, gruppo e fonte) e le decisioni citano registri dello stesso prodotto; il formato di
// una bozza aperta non cambia.
func TestV1M04LaVersioneDellaBozzaFormato3(t *testing.T) {
	p := schema23ConStoria(t)
	frase := "non e' quello dello snapshot (impronta, gruppo, fonte) o non e' del prodotto"
	conSorgente := func(campo, valore string) map[string]any {
		s := snapshot3()
		s["sorgente"].(map[string]any)[campo] = valore
		return s
	}
	conDecisione := func(importazione string) map[string]any {
		s := snapshot3()
		s["decisioni"].([]any)[0].(map[string]any)["importazione"] = importazione
		return s
	}
	rifiuto23(t, p, "testata formato 2, snapshot formato 3", "P4X04", "porta uno snapshot formato 3",
		`INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, salvata_da)
		 VALUES ('{B1}', 2, `+jsonSQL(t, snapshot3())+`, repeat('c', 64), 0, 0, repeat('1', 64), '{U}')`)
	rifiuto23(t, p, "testata formato 3, snapshot formato 2", "P4X04", "porta uno snapshot formato 2",
		`INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_conferma_id, base_numero, base_contatore, firma_base,
			fonte_allegato_id, fonte_sha256, fonte_gruppo, salvata_da) VALUES ('{B3}', 2, '{"formato": 2}', repeat('c', 64), '{C1}', 1, 0,
			repeat('1', 64), '{A}', repeat('b', 64), 'bom', '{U}')`)
	rifiuto23(t, p, "registro di un altro prodotto", "P4X04", frase, versione2B3(t, conSorgente("importazione", sha("f")), "'{R3}'"))
	rifiuto23(t, p, "impronta dello snapshot diversa dal registro", "P4X04", frase, versione2B3(t, conSorgente("importazione", sha("f")), "'{R1}'"))
	rifiuto23(t, p, "registro di un altro gruppo", "P4X04", frase, versione2B3(t, conSorgente("importazione", sha("e")), "'{R2}'"))
	rifiuto23(t, p, "registro di un'altra fonte", "P4X04", frase, versione2B3(t, conSorgente("allegato_id", u23("{A2}")), "'{R1}'"))
	rifiuto23(t, p, "snapshot con il registro, colonna NULL", "P4X04", "lo snapshot cita un registro che la versione non registra",
		versione2B3(t, snapshot3(), "NULL"))
	rifiuto23(t, p, "decisione su un registro di un altro prodotto", "P4X04", "una decisione cita il registro "+sha("f"),
		versione2B3(t, conDecisione(sha("f")), "'{R1}'"))
	rifiuto23(t, p, "decisione su un registro che non c'e'", "P4X04", "una decisione cita il registro "+sha("a"),
		versione2B3(t, conDecisione(sha("a")), "'{R1}'"))
	// M-04b, invariato: il formato di una bozza aperta non cambia sul posto, in nessuna direzione
	rifiuto23(t, p, "una bozza formato 2 aperta che diventa formato 3", "P4X04", "RFQ, prodotto, formato e creazione non cambiano",
		`UPDATE distinta_bozza SET formato = 3 WHERE bozza_id = '{B1}'`)
	rifiuto23(t, p, "una bozza formato 3 aperta che diventa formato 2", "P4X04", "RFQ, prodotto, formato e creazione non cambiano",
		`UPDATE distinta_bozza SET formato = 2 WHERE bozza_id = '{B3}'`)
	// la controprova: la versione 2 coerente passa (una decisione presa su un registro precedente dello stesso prodotto
	// compresa: R2 e' di P)
	s := snapshot3()
	s["decisioni"] = append(s["decisioni"].([]any), map[string]any{"id": "ni-2", "tipo": "non_includere",
		"casella":       map[string]any{"allegato_id": u23("{A}"), "gruppo": "bom", "rif": rifB("#32")},
		"motivo_codice": "elemento_cad", "motivo": "altro cordone", "azione_id": u23("{AZ}"), "importazione": sha("e")})
	riuscita23(t, p, "versione 2 coerente", versione2B3(t, s, "'{R1}'"))
	if v := uno23[int32](t, p, `SELECT versione FROM distinta_bozza WHERE bozza_id = '{B3}'`); v != 2 {
		t.Errorf("la testata di B3 e' alla versione %d, attesa 2", v)
	}
}

// M-09b, M-10: la nascita della conferma formato 3: bozza dello stesso formato e con lo stesso registro; registro
// dello stesso prodotto con l'impronta, il gruppo e la fonte dello snapshot; importazioni_citate uguale all'insieme dei
// registri citati dallo snapshot. Poi la conferma valida passa.
func TestV1M09bM10LaNascitaDellaConfermaFormato3(t *testing.T) {
	p := schema23ConStoria(t)
	citate := func(arr string) []string {
		c := nuovaC3()
		c.testata["importazioni_citate"] = arr
		return c.istruzioni(t)
	}
	conR2 := func() []string { // la decisione ni-1 presa sul registro R2 (stesso prodotto)
		c := nuovaC3()
		c.snap["decisioni"].([]any)[0].(map[string]any)["importazione"] = sha("e")
		c.testata["importazioni_citate"] = "ARRAY['{R1}']::uuid[]"
		return c.istruzioni(t)
	}
	frase := "importazioni_citate non e' l'insieme dei registri citati dallo snapshot"
	rifiuto23(t, p, "M-09b formato 3 senza importazioni_citate", "P4X01", frase, citate("NULL")...)
	rifiuto23(t, p, "M-09b array senza il registro della sorgente", "P4X01", frase, citate("'{}'::uuid[]")...)
	rifiuto23(t, p, "M-09b un registro in piu'", "P4X01", frase, citate("ARRAY['{R1}', '{R2}']::uuid[]")...)
	rifiuto23(t, p, "M-09b un registro in meno", "P4X01", frase, conR2()...)
	rifiuto23(t, p, "M-09b ripetizioni", "P4X01", frase, citate("ARRAY['{R1}', '{R1}']::uuid[]")...)
	altro := nuovaC3()
	altro.snap["decisioni"].([]any)[0].(map[string]any)["importazione"] = sha("f")
	altro.testata["importazioni_citate"] = "ARRAY['{R1}', '{R3}']::uuid[]"
	rifiuto23(t, p, "M-09b registro di un altro prodotto fra i citati", "P4X01", frase, altro.istruzioni(t)...)
	// formato 2 con l'array: il CHECK (la conferma n. 2 di P, formato 2, senza bozza)
	rifiuto23(t, p, "M-09b formato 2 con l'array (riga)", "23514", "ck_conferma_importazioni_citate",
		`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2,
			firma_struttura, firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati, origine, snapshot,
			impronta_snapshot, delta, mappa, criterio_verifica, chiave_idempotenza, confermato_da, importazioni_citate)
		 SELECT '{C2}', '{T}', '{P}', 2, '{C1}', 2, false, repeat('3', 64), repeat('2', 64), '{C1}', 0, repeat('1', 64),
			COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = '{T}'), 0),
			COALESCE((SELECT dati FROM working_contatore WHERE thread_id = '{T}'), 0), '{"rotta":"prova"}',
			jsonb_build_object('formato', 2, 'prodotto_id', '{P}'::text), repeat('a', 64), '{}', '{}', 'nessuno', gen_random_uuid(), '{U}',
			ARRAY['{R1}']::uuid[]`)

	// M-10: formato 3 che chiude una bozza formato 2 (B1 di P2, versione 1, impronta a…)
	rifiuto23(t, p, "M-10 formato 3 che chiude una bozza formato 2", "P4X01", "la bozza "+u23("{B1}")+" e' formato 2",
		`INSERT INTO conferma_strutturale (thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura, firma_anteprima,
			base_contatore, firma_base, contatore, contatore_dati, bozza_id, bozza_versione, origine, snapshot, impronta_snapshot, delta, mappa,
			criterio_verifica, chiave_idempotenza, confermato_da, importazioni_citate)
		 SELECT '{T}', '{P2}', 1, 3, false, repeat('3', 64), repeat('2', 64), 0, repeat('1', 64),
			COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = '{T}'), 0), COALESCE((SELECT dati FROM working_contatore WHERE thread_id = '{T}'), 0),
			'{B1}', 1, '{"rotta":"prova"}', jsonb_build_object('formato', 3, 'prodotto_id', '{P2}'::text, 'nodi', jsonb_build_array(
				jsonb_build_object('chiave_bozza', 'k-p2', 'identita', jsonb_build_object('codice', 'ACM-2000')))),
			repeat('a', 64), '{}', '{}', 'nessuno', gen_random_uuid(), '{U}', '{}'::uuid[]`)
	diverso := nuovaC3()
	diverso.snap["sorgente"].(map[string]any)["importazione"] = sha("e")
	diverso.snap["sorgente"].(map[string]any)["gruppo"] = "cand:" + u23("{A}") + ":" + rifB("#1")
	diverso.testata["importazione_id"] = "'{R2}'"
	diverso.testata["importazioni_citate"] = "ARRAY['{R1}', '{R2}']::uuid[]"
	diverso.testata["fonte_gruppo"] = "'cand:{A}:" + rifB("#1") + "'"
	rifiuto23(t, p, "M-10 registro diverso da quello della versione", "P4X01", "o la sua versione cita un altro registro", diverso.istruzioni(t)...)
	// registro di un altro prodotto: la versione 2 di B3 lo cita a trigger spento (difesa in profondita' della conferma)
	s3 := snapshot3()
	s3["sorgente"].(map[string]any)["importazione"] = sha("f")
	s3["decisioni"] = []any{}
	r3 := nuovaC3()
	r3.snap = s3
	r3.nonInclusi = nil
	r3.testata["bozza_versione"] = "2"
	r3.testata["impronta_snapshot"] = "repeat('c', 64)"
	r3.testata["importazione_id"] = "'{R3}'"
	r3.testata["importazioni_citate"] = "'{}'::uuid[]"
	rifiuto23(t, p, "M-10 registro di un altro prodotto", "P4X01", "o non e' del prodotto",
		// (il trigger resta spento fino al ROLLBACK della prova: con eventi differiti in sospeso non si riaccende)
		append([]string{`ALTER TABLE distinta_bozza_versione DISABLE TRIGGER trg_dbv_nasce`, versione2B3(t, s3, "'{R3}'")},
			r3.istruzioni(t)...)...)
	gruppo := nuovaC3()
	gruppo.testata["fonte_gruppo"] = "'cand:x'"
	rifiuto23(t, p, "M-10 registro di un altro gruppo", "P4X01", "o non e' del prodotto", gruppo.istruzioni(t)...)
	fonte := nuovaC3()
	fonte.testata["fonte_allegato_id"] = "'{A2}'"
	fonte.testata["fonte_sha256"] = "repeat('d', 64)"
	rifiuto23(t, p, "M-10 registro di un'altra fonte", "P4X01", "o non e' del prodotto", fonte.istruzioni(t)...)
	senza := nuovaC3()
	senza.testata["importazione_id"] = "NULL"
	rifiuto23(t, p, "M-10 snapshot con il registro, colonna NULL", "P4X01", "o la sua versione cita un altro registro", senza.istruzioni(t)...)

	// la conferma valida passa, chiude la bozza, e il suo gesto e' conferma_bom_v3
	riuscita23(t, p, "conferma formato 3 valida", append(nuovaC3().istruzioni(t),
		`INSERT INTO esito_intento (chiave, thread_id, gesto, utente_id, impronta_corpo, esito, conferma_id)
		 VALUES ('{K3}', '{T}', 'conferma_bom_v3', '{U}', repeat('a', 64), '{"esito": "confermata"}', '{C3}')`)...)
	if s := uno23[string](t, p, `SELECT stato FROM distinta_bozza WHERE bozza_id = '{B3}'`); s != "confermata" {
		t.Errorf("B3 dopo la conferma: %s", s)
	}
	if n := uno23[int](t, p, `SELECT count(*) FROM conferma_strutturale_non_incluso WHERE conferma_id = '{C3}'`); n != 1 {
		t.Errorf("nodi non inclusi della conferma: %d", n)
	}
	if e := uno23[string](t, p, `SELECT emittente || ' ' || emittente_presunto FROM conferma_strutturale_componente WHERE conferma_id = '{C3}' AND radice`); e != "cliente:"+u23("{C}")+" true" {
		t.Errorf("identita' certificata della radice: %s", e)
	}
}

// M-11: il contenuto della conferma: chiave della bozza e origine degli archi anche nel formato 3; emittente,
// presunzione e namespace solo nel formato 3, e li' emittente e presunzione obbligatori.
func TestV1M11IlContenutoDellaConferma(t *testing.T) {
	p := schema23ConStoria(t)
	frase := "esistono solo nel formato 3"
	c := nuovaC3()
	c.componenti = []string{strings.Replace(componenteC3P, "true, 'k-p'", "true, NULL", 1)}
	rifiuto23(t, p, "componente formato 3 senza chiave_bozza", "P4X01", "(formato 3): il componente "+u23("{P}")+" non ha la chiave della bozza", c.istruzioni(t)...)
	c = nuovaC3()
	c.componenti = []string{strings.Replace(componenteC3P, "'cliente:{C}', true", "NULL, true", 1)}
	rifiuto23(t, p, "formato 3 senza emittente", "P4X01", frase, c.istruzioni(t)...)
	c = nuovaC3()
	c.componenti = []string{strings.Replace(componenteC3P, "'cliente:{C}', true", "'cliente:{C}', NULL", 1)}
	rifiuto23(t, p, "formato 3 senza emittente_presunto", "P4X01", frase, c.istruzioni(t)...)
	c = nuovaC3()
	c.archi = []string{strings.Replace(arcoC3, "'conferma'", "NULL", 1)}
	rifiuto23(t, p, "arco formato 3 senza origine", "P4X01", "(formato 3): l'arco", c.istruzioni(t)...)
	// formato 2 (la conferma n. 2 di P, senza bozza): niente emittente, presunzione o namespace
	f2 := conferma2("0", "0", "2", "nessuno")
	rifiuto23(t, p, "formato 2 con emittente", "P4X01", frase, f2,
		`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo, emittente, emittente_presunto)
		 VALUES ('{C2}', '{T}', '{P}', true, 'k-p', 'ACM-1000', 'A', 'Gruppo base', 'finito', 'cliente:{C}', true)`)
	rifiuto23(t, p, "formato 2 con namespace", "P4X01", frase, f2,
		`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo, namespace)
		 VALUES ('{C2}', '{T}', '{P}', true, 'k-p', 'ACM-1000', 'A', 'Gruppo base', 'finito', 'acme')`)
	rifiuto23(t, p, "formato 2 senza chiave (messaggio col formato)", "P4X01", "(formato 2): il componente", f2,
		`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, radice, chiave_bozza, codice, rev, descrizione, tipo)
		 VALUES ('{C2}', '{T}', '{P}', true, NULL, 'ACM-1000', 'A', 'Gruppo base', 'finito')`)
	// la controprova formato 2: la conferma n. 2 di P senza le colonne della 23 passa come prima
	riuscita23(t, p, "conferma formato 2 come prima", f2, radiceC2, figlioC2, arcoC2)
}

// M-13: il controllo al COMMIT della conferma formato 3, per chiave_bozza: identita' decisa (codice, maiuscole, rev,
// emittente, presunzione, namespace), mappa, archi (origine, qta, step) e nodi non inclusi = decisioni non_includere.
func TestV1M13IlControlloAlCommitFormato3(t *testing.T) {
	p := schema23ConStoria(t)
	identita := "non ha il suo nodo, o non ne porta l'identita' decisa"
	arco := "non ha il suo arco nello snapshot (origine, qta) o e' step senza step"
	nonInclusi := "i nodi non inclusi non sono le decisioni non_includere dello snapshot"
	casi := []struct {
		nome, frase string
		cambia      func(c *confermaC3)
	}{
		{"codice certificato diverso dall'identita' del nodo", identita, func(c *confermaC3) { nodoDi(c.snap, "k-p")["identita"].(map[string]any)["codice"] = "ACM-1000X" }},
		{"maiuscole diverse", identita, func(c *confermaC3) { nodoDi(c.snap, "k-p")["identita"].(map[string]any)["codice"] = "acm-1000" }},
		{"rev diversa", identita, func(c *confermaC3) { nodoDi(c.snap, "k-s")["identita"].(map[string]any)["rev"] = "B" }},
		{"rev assente nel nodo", identita, func(c *confermaC3) { delete(nodoDi(c.snap, "k-s")["identita"].(map[string]any), "rev") }},
		{"emittente diverso", identita, func(c *confermaC3) { nodoDi(c.snap, "k-p")["identita"].(map[string]any)["emittente"] = "partner:beta" }},
		{"presunzione diversa", identita, func(c *confermaC3) { nodoDi(c.snap, "k-p")["identita"].(map[string]any)["emittente_presunto"] = false }},
		{"presunzione assente nel nodo (vale falso)", identita, func(c *confermaC3) {
			delete(nodoDi(c.snap, "k-p")["identita"].(map[string]any), "emittente_presunto")
		}},
		{"namespace NULL con il nodo che ne ha uno", identita, func(c *confermaC3) { nodoDi(c.snap, "k-s")["identita"].(map[string]any)["namespace"] = "acme" }},
		{"componente fuori dalla mappa (la radice non nominata)", identita, func(c *confermaC3) { delete(c.mappa, "k-p") }},
		{"mappa che assegna la chiave a un altro componente", identita, func(c *confermaC3) { c.mappa["k-p"] = u23("{S}"); c.mappa["k-s"] = u23("{S}") }},
		{"chiave_bozza che non e' un nodo", identita, func(c *confermaC3) {
			c.componenti[1] = strings.Replace(componenteC3S, "'k-s'", "'k-x'", 1)
			c.mappa["k-x"] = u23("{S}")
		}},
		{"arco step senza step nello snapshot", arco, func(c *confermaC3) {
			c.archi = []string{strings.Replace(arcoC3, "'conferma'", "'step'", 1)}
			c.snap["archi"].([]any)[0].(map[string]any)["origine"] = "step"
		}},
		{"arco certificato assente dallo snapshot", arco, func(c *confermaC3) { c.snap["archi"] = []any{} }},
		{"arco con un'altra qta nello snapshot", arco, func(c *confermaC3) { c.snap["archi"].([]any)[0].(map[string]any)["qta"] = 3 }},
		{"arco con un'altra origine nello snapshot", arco, func(c *confermaC3) { c.snap["archi"].([]any)[0].(map[string]any)["origine"] = "utente" }},
		{"decisione non_includere senza riga", nonInclusi, func(c *confermaC3) { c.nonInclusi = nil }},
		{"riga senza decisione", nonInclusi, func(c *confermaC3) { c.snap["decisioni"] = []any{} }},
		{"azione_id diverso", nonInclusi, func(c *confermaC3) {
			c.nonInclusi = []string{strings.Replace(nonInclusoC3, "'{AZ}'", "gen_random_uuid()", 1)}
		}},
		{"gruppo diverso", nonInclusi, func(c *confermaC3) {
			c.nonInclusi = []string{strings.Replace(nonInclusoC3, "'{A}', 'bom'", "'{A}', 'cand:x'", 1)}
		}},
		{"motivo diverso", nonInclusi, func(c *confermaC3) {
			c.nonInclusi = []string{strings.Replace(nonInclusoC3, "'cordone di saldatura'", "'cordone'", 1)}
		}},
		{"segnale diverso", nonInclusi, func(c *confermaC3) {
			c.nonInclusi = []string{strings.Replace(nonInclusoC3, "elemento_cad_nome_foglia_v1", "altra_regola", 1)}
		}},
		{"segnale assente nella riga", nonInclusi, func(c *confermaC3) {
			c.nonInclusi = []string{strings.Replace(nonInclusoC3, `'{"codice": "ancoraggio.possibile_elemento_cad", "regola": "elemento_cad_nome_foglia_v1", "versione_servizio": "ancoraggio-3"}'`, "NULL", 1)}
		}},
		{"chiave del nodo diversa", nonInclusi, func(c *confermaC3) {
			c.snap["decisioni"].([]any)[0].(map[string]any)["casella"].(map[string]any)["rif"] = rifB("#33")
		}},
	}
	for _, x := range casi {
		c := nuovaC3()
		x.cambia(&c)
		rifiuto23(t, p, "M-13 "+x.nome, "P4X01", x.frase, c.istruzioni(t)...)
	}
	// i controlli della 0022 restano per il formato 3: un arco della working non certificato
	c := nuovaC3()
	c.archi = nil
	c.snap["archi"] = []any{}
	rifiuto23(t, p, "M-13 perimetro aperto (controllo della 0022)", "P4X01", "della working non e' fra gli archi certificati", c.istruzioni(t)...)
	// la controprova: la conferma valida passa
	riuscita23(t, p, "conferma formato 3 valida", nuovaC3().istruzioni(t)...)
}

// M-14: il registro d'importazione: fonte e allegati citati della RFQ e attivi (P4X08), immutabile (P4X02).
func TestV1M14IlRegistroDImportazione(t *testing.T) {
	p := schema23ConStoria(t)
	registro := func(cambi map[string]string) string { return insertCon("distinta_importazione", cambi) }
	rifiuto23(t, p, "fonte con un altro sha256", "P4X08", "non e' un allegato della RFQ", registro(map[string]string{"fonte_sha256": "repeat('c', 64)"}))
	rifiuto23(t, p, "fonte di un'altra RFQ", "P4X08", "non e' un allegato della RFQ", registro(map[string]string{"fonte_allegato_id": "'{AX}'",
		"allegati_citati": "ARRAY['{AX}']::uuid[]"}))
	rifiuto23(t, p, "allegato citato di un'altra RFQ", "P4X08", "citato non e' della RFQ", registro(map[string]string{
		"allegati_citati": "ARRAY['{A}', '{AX}']::uuid[]"}))
	// un allegato citato in Altri file e uno scartato (SIC-14)
	riuscita23(t, p, "A3 in Altri file", inAltriFile("{A3}")...)
	riuscita23(t, p, "A4 scartato", scartato("{A4}")...)
	rifiuto23(t, p, "allegato citato in Altri file", "P4X08", "citato non e' attivo", registro(map[string]string{
		"allegati_citati": "ARRAY['{A}', '{A3}']::uuid[]"}))
	rifiuto23(t, p, "allegato citato scartato", "P4X08", "citato non e' attivo", registro(map[string]string{
		"allegati_citati": "ARRAY['{A}', '{A4}']::uuid[]"}))
	rifiuto23(t, p, "fonte in Altri file", "P4X08", "citato non e' attivo", registro(map[string]string{"fonte_allegato_id": "'{A3}'",
		"fonte_sha256": "repeat('3', 64)", "allegati_citati": "ARRAY['{A3}']::uuid[]"}))
	rifiuto23(t, p, "gruppo senza", "23514", "ck_dimp_gruppo", registro(map[string]string{"gruppo": "'senza'",
		"contenuto": contenutoDimp("{P}", "1", "senza", "'nodo:' || repeat('b', 64) || ':#1'", nil)}))
	rifiuto23(t, p, "stessa proposta due volte", "23505", "ux_dimp_impronta", registro(map[string]string{"impronta": "repeat('6', 64)"}))
	riuscita23(t, p, "registro valido", registro(nil))
	// data e transazione le scrive il database
	if !uno23[bool](t, p, `SELECT creata_il > now() - interval '1 hour' FROM distinta_importazione WHERE impronta = repeat('0', 64)`) {
		t.Error("la data del registro non e' quella del database")
	}
	rifiuto23(t, p, "UPDATE di un registro", "P4X02", "non si modifica", `UPDATE distinta_importazione SET gruppo = gruppo WHERE importazione_id = '{R1}'`)
	rifiuto23(t, p, "DELETE di un registro", "P4X02", "non si modifica", `DELETE FROM distinta_importazione WHERE importazione_id = '{R3}'`)
	rifiuto23(t, p, "TRUNCATE dei registri", "P4X02", "non si svuotano", `TRUNCATE distinta_importazione CASCADE`)
}

// M-15: i nodi non inclusi: solo nella transazione della conferma e prima del controllo finale, solo nel formato 3,
// con l'allegato della RFQ e lo sha256 del nodo; immutabili.
func TestV1M15INodiNonInclusi(t *testing.T) {
	p := schema23ConStoria(t)
	riga := func(conferma, allegato, shaExpr string) string {
		return `INSERT INTO conferma_strutturale_non_incluso (conferma_id, thread_id, sha256, chiave, allegato_id, gruppo, decisione_id, azione_id,
			motivo_codice, motivo) VALUES ('` + conferma + `', '{T}', ` + shaExpr + `, '#77', '` + allegato + `', 'bom', 'ni-7', gen_random_uuid(), 'altro', 'prova')`
	}
	rifiuto23(t, p, "fuori dalla transazione della conferma", "P4X01", "gia' registrata o controllata", riga("{C1}", "{A}", "repeat('b', 64)"))
	rifiuto23(t, p, "conferma formato 2", "P4X01", "esistono solo nel formato 3", conferma2("0", "0", "2", "nessuno"), riga("{C2}", "{A}", "repeat('b', 64)"))
	c := nuovaC3()
	c.nonInclusi = []string{strings.Replace(nonInclusoC3, "repeat('b', 64), '#31'", "repeat('c', 64), '#31'", 1)}
	rifiuto23(t, p, "sha256 diverso dall'allegato", "P4X01", "non e' della RFQ", c.istruzioni(t)...)
	c = nuovaC3()
	c.nonInclusi = []string{strings.Replace(nonInclusoC3, "'#31', '{A}'", "'#31', '{AX}'", 1)}
	rifiuto23(t, p, "allegato di un'altra RFQ con lo stesso sha256", "P4X01", "non e' della RFQ", c.istruzioni(t)...)
	c = nuovaC3()
	c.nonChiude = true
	c.dopo = []string{`UPDATE distinta_bozza SET stato = 'confermata', chiusa_da = '{U}' WHERE bozza_id = '{B3}'`,
		`SET CONSTRAINTS trg_conferma_strutturale_completa IMMEDIATE`, riga("{C3}", "{A}", "repeat('b', 64)")}
	rifiuto23(t, p, "dopo il controllo finale anticipato", "P4X01", "gia' registrata o controllata", c.istruzioni(t)...)
	c = nuovaC3()
	c.nonInclusi = append(c.nonInclusi, strings.Replace(nonInclusoC3, "'#31'", "'#32'", 1))
	rifiuto23(t, p, "due righe per la stessa decisione", "23505", "ux_csni_decisione", c.istruzioni(t)...)
	riuscita23(t, p, "conferma formato 3 valida", nuovaC3().istruzioni(t)...)
	rifiuto23(t, p, "UPDATE di un nodo non incluso", "P4X01", "non cambia", `UPDATE conferma_strutturale_non_incluso SET motivo = 'altro'`)
	rifiuto23(t, p, "DELETE di un nodo non incluso", "P4X01", "non cambia", `DELETE FROM conferma_strutturale_non_incluso`)
	rifiuto23(t, p, "TRUNCATE dei nodi non inclusi", "P4X01", "non si svuotano", `TRUNCATE conferma_strutturale_non_incluso`)
}

// M-16: il gesto della conferma dice il suo formato (P4X03).
func TestV1M16IGestiDegliIntenti(t *testing.T) {
	p := schema23ConStoria(t)
	riuscita23(t, p, "conferma formato 3 valida", nuovaC3().istruzioni(t)...)
	rifiuto23(t, p, "conferma_bom_v3 su una conferma formato 2", "P4X03", "non e' quello di una conferma formato 2",
		`INSERT INTO esito_intento (chiave, thread_id, gesto, utente_id, impronta_corpo, esito, conferma_id)
		 VALUES ('{K1}', '{T}', 'conferma_bom_v3', '{U}', repeat('a', 64), '{}', '{C1}')`)
	rifiuto23(t, p, "conferma_bom_v2 su una conferma formato 3", "P4X03", "non e' quello di una conferma formato 3",
		`INSERT INTO esito_intento (chiave, thread_id, gesto, utente_id, impronta_corpo, esito, conferma_id)
		 VALUES ('{K3}', '{T}', 'conferma_bom_v2', '{U}', repeat('a', 64), '{}', '{C3}')`)
	for _, g := range []string{"scarta_file", "altri_file", "riattiva_file", "associa_step"} {
		riuscita23(t, p, "gesto "+g, `INSERT INTO esito_intento (chiave, thread_id, gesto, utente_id, impronta_corpo, esito)
			VALUES (gen_random_uuid(), '{T}', '`+g+`', '{U}', repeat('a', 64), '{}')`)
	}
	riuscita23(t, p, "conferma_bom_v3 sulla conferma formato 3", `INSERT INTO esito_intento (chiave, thread_id, gesto, utente_id, impronta_corpo, esito, conferma_id)
		VALUES ('{K3}', '{T}', 'conferma_bom_v3', '{U}', repeat('a', 64), '{}', '{C3}')`)
}

// M-17: la fonte STEP secondaria con una destinazione temporanea su un nodo formato 3 senza UUID (k-n di B3): passa,
// e la conferma che certifica la chiave la risolve; se la chiave e' stata riusata per un nodo diverso, P4X07.
func TestV1M17LaFonteSecondariaSuUnNodoFormato3(t *testing.T) {
	scelta := `INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, chiave_bozza, bozza_id, azione, allegato_id, sha256, gruppo, autore_id)
		VALUES ('{T}', '{P}', 'sottoassieme', 'k-n', '{B3}', 'scelta', '{A}', repeat('b', 64), 'bom', '{U}')`
	// la conferma che crea N3 (ACM-1300) sotto P e lo certifica con la chiave k-n
	conN3 := func(versione, impronta string) confermaC3 {
		c := nuovaC3()
		c.prima = []string{
			`INSERT INTO componente (componente_id, thread_id, codice, descrizione, tipo, origine, confermato_da)
			 VALUES ('{N3}', '{T}', 'ACM-1300', NULL, 'sciolto', 'manuale', '{U}')`,
			`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ('{T}', '{P}', '{N3}', 1, 'manuale', '{U}')`,
		}
		c.snap["archi"] = append(c.snap["archi"].([]any), map[string]any{"padre": "k-p", "figlio": "k-n", "qta": 1, "origine": "utente"})
		c.mappa["k-n"] = u23("{N3}")
		c.componenti = append(c.componenti, `('{C3}', '{T}', '{N3}', false, 'k-n', 'ACM-1300', NULL, NULL, 'sciolto', 'cliente:{C}', true, NULL)`)
		c.archi = append(c.archi, `('{C3}', '{P}', '{N3}', 1, 'utente')`)
		c.testata["bozza_versione"] = versione
		c.testata["impronta_snapshot"] = impronta
		return c
	}

	t.Run("risolta", func(t *testing.T) {
		p := schema23ConStoria(t)
		riuscita23(t, p, "scelta sul nodo k-n senza UUID", scelta)
		if n := uno23[string](t, p, `SELECT destinazione -> 'nodo' ->> 'chiave_bozza' FROM fonte_step_evento WHERE chiave_bozza = 'k-n'`); n != "k-n" {
			t.Errorf("destinazione: %q", n)
		}
		riuscita23(t, p, "conferma che certifica k-n", conN3("1", "repeat('5', 64)").istruzioni(t)...)
		if r := uno23[string](t, p, `SELECT componente_id::text FROM fonte_step_evento WHERE azione = 'risoluzione' AND chiave_bozza = 'k-n'`); r != u23("{N3}") {
			t.Errorf("risoluzione: %q", r)
		}
	})
	t.Run("chiave riusata", func(t *testing.T) {
		p := schema23ConStoria(t)
		riuscita23(t, p, "scelta sul nodo k-n senza UUID", scelta)
		// versione 2: il giornale (vocabolario 2) riaggiunge k-n come un nodo diverso
		s := snapshot3()
		riuscita23(t, p, "versione 2 con k-n riusata", versione2B3(t, s, "'{R1}'"),
			`INSERT INTO distinta_bozza_evento (bozza_id, n, versione, vocabolario, azione, chiave_bozza, dopo, origine, utente_id)
			 VALUES ('{B3}', 2, 2, 2, 'nodo_aggiunto', 'k-n', '{"chiave_bozza": "k-n", "op": "crea", "tipo": "sciolto", "origine": "utente"}', 'utente', '{U}')`)
		rifiuto23(t, p, "conferma su una chiave riusata", "P4X07", "e' stata riusata per un nodo diverso",
			conN3("2", "repeat('c', 64)").istruzioni(t)...)
	})
}

// M-19: anche nel formato 3 la conferma si scrive per ultima: dopo la testata la working della RFQ non si scrive.
func TestV1M19LaConfermaFormato3SiScrivePerUltima(t *testing.T) {
	p := schema23ConStoria(t)
	c := nuovaC3()
	c.dopo = []string{`UPDATE componente SET descrizione = 'Telaio saldato' WHERE componente_id = '{S}'`}
	rifiuto23(t, p, "scrittura della working dopo la testata", "P4X01", "dopo una conferma strutturale nella stessa transazione", c.istruzioni(t)...)
}

// M-20: un database con la 23 di un'altra variante (o senza) non parte, e il migratore non tocca niente.
func TestV1M20LaVarianteDella0023(t *testing.T) {
	p := testutil.Pool(t)
	t.Cleanup(func() { testutil.SchemaPulito(t, p) })
	ctx := context.Background()
	testutil.SchemaPulito(t, p)
	m23 := migrazioneNumero(t, 23)
	if v := uno23[string](t, p, `SELECT variante FROM schema_variante WHERE versione = 23`); v != m23.Variante {
		t.Fatalf("variante registrata %q, dichiarata %q", v, m23.Variante)
	}
	for _, scena := range []struct{ nome, sql string }{
		{"un'altra variante", `UPDATE schema_variante SET variante = '0023-un-altro-testo' WHERE versione = 23`},
		{"riga della variante assente", `DELETE FROM schema_variante WHERE versione = 23`},
	} {
		testutil.SchemaPulito(t, p)
		esegui23(t, p, scena.sql)
		prima := impronta18(t, p)
		n, err := migrazioni.Applica(ctx, p, risorse.FS, testutil.LogSilenzioso())
		var vd *migrazioni.VarianteDiversa
		if !errors.As(err, &vd) || n != 0 || vd.Versione != 23 || vd.DelBinario != m23.Variante {
			t.Errorf("%s: atteso VarianteDiversa sulla 23, ottenuto %d migrazioni e %v", scena.nome, n, err)
		}
		if d := differenze(prima, impronta18(t, p)); d != "" {
			t.Errorf("%s: il migratore fermato ha cambiato lo schema:\n%s", scena.nome, d)
		}
	}
}

// M-30: la chiave del salvataggio e' unica: un reinvio non scrive una seconda versione.
func TestV1M30LaChiaveDelSalvataggioEUnica(t *testing.T) {
	p := schema23ConStoria(t)
	chiave := uno23[string](t, p, `SELECT chiave_salvataggio::text FROM distinta_bozza_versione WHERE bozza_id = '{B3}' AND versione = 1`)
	rifiuto23(t, p, "stessa chiave in due versioni", "23505", "ux_dbv_chiave_salvataggio",
		strings.Replace(versione2B3(t, snapshot3(), "'{R1}'"), "gen_random_uuid(), repeat('4', 64))", "'"+chiave+"', repeat('4', 64))", 1))
	// formato 2 senza chiave: passa come prima (B1 di P2)
	riuscita23(t, p, "versione formato 2 senza chiave", `INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero,
		base_contatore, firma_base, salvata_da) VALUES ('{B1}', 2, '{"formato": 2}', repeat('c', 64), 0, 0, repeat('1', 64), '{U}')`)
}

// M-34 (parte del database; T-14 confronta le tre regole: la grammatica e il Go in distintabozza, il CHECK in
// v1_t14_db_test.go, con gli stessi casi di distintabozza.CasiT14): l'id del partner segue la regola della
// grammatica (minuscolo, senza spazi, «:» o «/»), con un tetto di sicurezza largo; il namespace solo non vuoto.
func TestV1M34LEmittenteDellaGrammatica(t *testing.T) {
	p := schema23ConStoria(t)
	for _, e := range []struct{ emittente, namespace string }{
		{"'partner:beta+co'", "repeat('n', 70)"}, {"'partner:beta'", "NULL"}, {"'partner:è-uno'", "'Acme'"},
		{"'partner:' || repeat('a', 200)", "repeat('n', 500)"}, {"'cliente:{C}'", "NULL"},
	} {
		riuscitaSenza := insertCon("conferma_strutturale_componente", map[string]string{"emittente": e.emittente, "emittente_presunto": "false",
			"namespace": e.namespace})
		ctx := context.Background()
		tx, err := p.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, "ALTER TABLE conferma_strutturale_componente DISABLE TRIGGER USER"); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, riuscitaSenza); err != nil {
			t.Errorf("emittente %s, namespace %s: %v", e.emittente, e.namespace, err)
		}
		_ = tx.Rollback(ctx)
	}
}
