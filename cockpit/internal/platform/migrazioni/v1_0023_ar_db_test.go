//go:build integrazione

// L4 — le associazioni registrate della 0023 (direttiva 35 §2, blocco C-1b; variante 20261019a): file_rfq_associazione
// e il suo giornale in associazione_evento, provati sul database con le righe di v1_0023_db_test.go (schema23ConStoria)
// e la conferma formato 3 C3 di P (nuovaC3). Le sei verifiche della direttiva §2.1 (appartenenza al thread, componente
// della stessa RFQ e della working, occorrenza attiva, nessuna AR attiva incompatibile, conferma coerente), l'unicita'
// della AR attiva, la sola chiusura, il giornale in sola aggiunta, il controllo al COMMIT della conferma e due conferme
// successive (Prova 10 a livello di vincoli). Solo dati sintetici (ACME, example.com).
//
// Direttiva 39 (variante 20261019c): D-AR01 A [U] (la AR nasce con la conferma), D-2 (c) [U] (uno STEP documentale si
// associa, lo STEP legato come sorgente del finito no), D-AR07 A, D-AR08 A, D-4 A, D-ASS-04 A [T-39], la provenienza
// della PB sulla AR (D-PB03 A [U]). Le prove nuove della direttiva 39 stanno in v1_0023_dir39_db_test.go.

package migrazioni_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

var sostAR = strings.NewReplacer(
	"{AR1}", "00000000-0000-4000-8000-0000002304a1",
	"{AR2}", "00000000-0000-4000-8000-0000002304a2",
	"{AR3}", "00000000-0000-4000-8000-0000002304a3",
	"{AR5}", "00000000-0000-4000-8000-0000002304a5",
	"{ARX}", "00000000-0000-4000-8000-0000002304af",
	"{AE}", "00000000-0000-4000-8000-0000002304e1",
	"{AY}", "00000000-0000-4000-8000-0000002304e2",
	"{B5}", "00000000-0000-4000-8000-0000002304b5",
	"{C5}", "00000000-0000-4000-8000-0000002304c5",
	"{K5}", "00000000-0000-4000-8000-0000002304d5",
	"{FL}", "00000000-0000-4000-8000-0000002304f1",
	"{BV}", "00000000-0000-4000-8000-0000002304f2",
)

// ar sostituisce i segnaposto di questo file (gli altri li sostituiscono rifiuto23, riuscita23, esegui23 e uno23).
func ar(istruzioni ...string) []string {
	out := make([]string, len(istruzioni))
	for i, s := range istruzioni {
		out[i] = sostAR.Replace(s)
	}
	return out
}

func uuidAR(nome string) string { return sostAR.Replace(u23(nome)) }

// congelaBomT: la BOM della RFQ T congelata a mano nella transazione della prova (la V1 congelata, senza le istantanee:
// bom_working_bloccata guarda solo lo stato dell'ultima versione), con i trigger delle due tabelle spenti solo per
// scrivere quelle righe.
var congelaBomT = []string{
	`ALTER TABLE fase_log DISABLE TRIGGER USER`,
	`INSERT INTO fase_log (fase_log_id, thread_id, nome_fase, inizio) VALUES ('{FL}', '{T}', 'FATTIBILITA', now() - interval '1 hour')`,
	`ALTER TABLE bom_versione DISABLE TRIGGER USER`,
	`INSERT INTO bom_versione (bom_versione_id, thread_id, numero, stato, contesto, fase_all_apertura, fase_log_id_all_apertura, motivo,
		creata_da, congelata_da, congelata_il)
	 VALUES ('{BV}', '{T}', 1, 'congelata', 'preventivo', 'FATTIBILITA', '{FL}', 'prova AR', '{U}', '{U}', now())`,
	`ALTER TABLE bom_versione ENABLE TRIGGER USER`,
	`ALTER TABLE fase_log ENABLE TRIGGER USER`,
}

// passaAiControlli esegue le istruzioni, anticipa i controlli differiti (SET CONSTRAINTS ALL IMMEDIATE) e annulla la
// transazione: la prova vuole che tutto passi senza lasciare righe.
func passaAiControlli(t *testing.T, p *pgxpool.Pool, nome string, istruzioni ...string) {
	t.Helper()
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, s := range append(istruzioni, "SET CONSTRAINTS ALL IMMEDIATE") {
		if _, err := tx.Exec(ctx, e23(s)); err != nil {
			t.Fatalf("%s: %v\n%s", nome, err, s)
		}
	}
}

// nascita: la AR id di R5 (allegato → componente, PB sul nodo chiave della conferma conferma, chiave dell'atto) e il
// suo evento preassociazione nella stessa transazione.
func nascita(id, allegato, componente, chiave, conferma, k string) []string {
	return []string{
		`INSERT INTO file_rfq_associazione (associazione_id, thread_id, allegato_id, componente_id, origine, conferma_id, chiave_bozza,
			provenienza_pb, chiave_idempotenza, creata_da)
		 VALUES ('` + id + `', '{T}', '` + allegato + `', '` + componente + `', 'conferma_bom', '` + conferma + `', '` + chiave + `', 'manuale', '` + k + `', '{U}')`,
		`INSERT INTO associazione_evento (thread_id, allegato_id, associazione_id, azione, componente_dopo, conferma_id, origine,
			chiave_idempotenza, autore_id)
		 VALUES ('{T}', '` + allegato + `', '` + id + `', 'preassociazione', '` + componente + `', '` + conferma + `', 'conferma_bom', '` + k + `', '{U}')`,
	}
}

// pb: una preassociazione formato 3 {allegato_id, chiave_bozza, provenienza} (DIR35-PB [U]; D-PB03 A [U]), manuale.
func pb(allegato, chiave string) map[string]any {
	return map[string]any{"allegato_id": uuidAR(allegato), "chiave_bozza": chiave, "provenienza": "manuale"}
}

// c3ConAR: la conferma C3 con le PB date nello snapshot e le istruzioni dopo il contenuto (le AR e i loro eventi).
func c3ConAR(pbs []any, dopo ...string) confermaC3 {
	c := nuovaC3()
	c.snap["documenti_preassociati"] = pbs
	c.dopo = ar(dopo...)
	return c
}

// copiaDP: la copia di compatibilita' della proposta DP (allegato A2): il componente e il suo codice (FK a tre campi).
func copiaDP(componente, codice string) string {
	if componente == "" {
		return `UPDATE documento_proposta SET componente_id = NULL WHERE proposta_id = '{DP}'`
	}
	return `UPDATE documento_proposta SET componente_id = '` + componente + `', codice = '` + codice + `' WHERE proposta_id = '{DP}'`
}

// copiaLegacyDP: la stessa copia scritta come dato legacy (pre-assegnazione P1 del vecchio «Assegna», senza AR ne' evento),
// con trg_proposta_copia_derivata spento per la sola transazione della preparazione: dalla variante 20261019d il
// trigger e' acceso e rifiuterebbe al COMMIT un cambio della copia senza evento (come testutil.CopiaLegacy).
func copiaLegacyDP(componente, codice string) []string {
	return []string{`ALTER TABLE documento_proposta DISABLE TRIGGER trg_proposta_copia_derivata`, copiaDP(componente, codice),
		`ALTER TABLE documento_proposta ENABLE TRIGGER trg_proposta_copia_derivata`}
}

// confermaC3ConTreAR: la conferma C3 valida con tre AR: A3 → S (allegato senza documento_proposta, Prova 2), A2 → P
// (con la copia della proposta DP), A5 → S.
func confermaC3ConTreAR() confermaC3 {
	dopo := append(append(append(nascita("{AR1}", "{A3}", "{S}", "k-s", "{C3}", "{K3}"),
		nascita("{AR2}", "{A2}", "{P}", "k-p", "{C3}", "{K3}")...),
		copiaDP("{P}", "ACM-1000")),
		nascita("{AR5}", "{A5}", "{S}", "k-s", "{C3}", "{K3}")...)
	return c3ConAR([]any{pb("{A3}", "k-s"), pb("{A2}", "k-p"), pb("{A5}", "k-s")}, dopo...)
}

// La nascita: le sei verifiche della direttiva §2.1 nel database, la conferma coerente, la BOM congelata.
func TestV1ARLeVerificheAllaNascita(t *testing.T) {
	p := schema23ConStoria(t)
	k := func(dopo ...string) []string { return c3ConAR([]any{pb("{A3}", "k-s")}, dopo...).istruzioni(t) }
	n := nascita("{AR1}", "{A3}", "{S}", "k-s", "{C3}", "{K3}")

	// (1) l'allegato e' della RFQ: AX e' di T2
	rifiuto23(t, p, "allegato di un'altra RFQ", "P4X10", "non e' della RFQ",
		k(nascita("{AR1}", "{AX}", "{S}", "k-s", "{C3}", "{K3}")...)...)
	// (1) D-2 (c) [U]: lo STEP documentale A4 di una parte si associa come un PDF (M-42 riscritta); lo STEP A, sorgente
	//     della bozza aperta B3 di P, no
	passaAiControlli(t, p, "uno STEP documentale si associa",
		c3ConAR([]any{pb("{A4}", "k-s")}, nascita("{AR1}", "{A4}", "{S}", "k-s", "{C3}", "{K3}")...).istruzioni(t)...)
	rifiuto23(t, p, "lo STEP sorgente del finito non si associa", "P4X10", "legato come sorgente strutturale",
		c3ConAR([]any{pb("{A}", "k-s")}, nascita("{AR1}", "{A}", "{S}", "k-s", "{C3}", "{K3}")...).istruzioni(t)...)
	// (2) il componente e' della stessa RFQ: Q e' di T2
	rifiuto23(t, p, "componente di un'altra RFQ", "23503", "fk_fra_componente",
		k(nascita("{AR1}", "{A3}", "{Q}", "k-s", "{C3}", "{K3}")...)...)
	// (3) il componente della working non e' archiviato (P3, archiviato prima della testata)
	arch := c3ConAR([]any{pb("{A3}", "k-s")}, nascita("{AR1}", "{A3}", "{P3}", "k-s", "{C3}", "{K3}")...)
	arch.prima = []string{`UPDATE componente SET archiviato_il = now(), archiviato_da = '{U}', motivo_archiviazione = 'prova AR'
		WHERE componente_id = '{P3}'`}
	rifiuto23(t, p, "componente archiviato", "P4X10", "e' archiviato", arch.istruzioni(t)...)
	// (3) il componente di una AR di R5 e' certificato dalla sua conferma (P2 non lo e' in C3): al COMMIT
	rifiuto23(t, p, "componente non certificato dalla conferma", "23503", "fk_fra_conferma_componente",
		k(append([]string{"SET CONSTRAINTS fk_fra_conferma_componente IMMEDIATE"}, nascita("{AR1}", "{A3}", "{P2}", "k-s", "{C3}", "{K3}")...)...)...)
	// (4) l'occorrenza e' attiva: A5 in Altri file
	riuscita23(t, p, "A5 in Altri file", inAltriFile("{A5}")...)
	rifiuto23(t, p, "occorrenza in Altri file", "P4X10", "non e' attiva",
		k(nascita("{AR1}", "{A5}", "{S}", "k-s", "{C3}", "{K3}")...)...)
	// (4) ... e non scartata col vecchio «Scarta il file» (la proposta DP di A2 scartata da una persona)
	rifiuto23(t, p, "occorrenza scartata col vecchio gesto", "P4X10", "non e' attiva",
		k(append([]string{`UPDATE documento_proposta SET stato = 'scartata', deciso_da = '{U}', deciso_il = now() WHERE proposta_id = '{DP}'`},
			nascita("{AR2}", "{A2}", "{S}", "k-s", "{C3}", "{K3}")...)...)...)
	// (5) nessun legame fuori dalle AR: la copia della proposta DP dice gia' P
	rifiuto23(t, p, "legame nascosto nella copia", "P4X10", "fuori dalle associazioni registrate",
		k(append([]string{copiaDP("{P}", "ACM-1000")}, nascita("{AR1}", "{A2}", "{S}", "k-s", "{C3}", "{K3}")...)...)...)
	// (5) provvisorio, base [T] D-AR07 A: lo stesso contenuto e' gia' il documento confermato D2 di S
	stessoSha := `INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il)
		VALUES ('{AE}', '{M}', 9, 'ACM-1100_copia.pdf', 'pdf', repeat('e', 64), now())`
	rifiuto23(t, p, "documento confermato dello stesso contenuto su un altro componente", "P4X10", "documento confermato di un altro componente",
		k(append([]string{stessoSha}, nascita("{AE}", "{AE}", "{P}", "k-p", "{C3}", "{K3}")...)...)...)
	// (5) una sola AR attiva per allegato (provvisorio, base [T] D-ASS-04 A)
	rifiuto23(t, p, "seconda AR attiva dello stesso allegato", "23505", "ux_fra_attiva_allegato",
		k(append(append([]string{}, n...), nascita("{ARX}", "{A3}", "{P}", "k-p", "{C3}", "{K3}")...)...)...)
	// (6) la conferma coerente: la chiave dell'atto e' quella della conferma
	rifiuto23(t, p, "chiave diversa da quella della conferma", "P4X10", "chiave e autore non sono i suoi",
		k(nascita("{AR1}", "{A3}", "{S}", "k-s", "{C3}", "{K5}")...)...)
	// (6) una conferma che non e' di questa transazione (C1, formato 2)
	rifiuto23(t, p, "conferma di un'altra transazione", "P4X10", "non e' una formato 3 di questa transazione",
		k(nascita("{AR1}", "{A3}", "{S}", "k-s", "{C1}", "{K3}")...)...)
	// nasce attiva
	rifiuto23(t, p, "nasce chiusa", "P4X10", "nasce attiva", k(`INSERT INTO file_rfq_associazione (thread_id, allegato_id, componente_id,
		stato, origine, conferma_id, chiave_bozza, chiave_idempotenza, creata_da, chiusa_il, chiusa_da, motivo_chiusura, chiusa_tx)
		VALUES ('{T}', '{A3}', '{S}', 'chiusa', 'conferma_bom', '{C3}', 'k-s', '{K3}', '{U}', now(), '{U}', 'scollega_file', pg_current_xact_id())`)...)
	// al COMMIT: la nascita ha il suo evento; la PB e' nello snapshot della conferma; ogni PB applicabile ha la sua AR
	rifiuto23(t, p, "AR senza evento", "P4X10", "la nascita non ha il suo evento", k(n[0])...)
	rifiuto23(t, p, "AR senza la sua PB nello snapshot", "P4X10", "non ha la preassociazione",
		c3ConAR([]any{}, n...).istruzioni(t)...)
	rifiuto23(t, p, "PB senza la sua AR", "P4X10", "non ha la sua associazione registrata", k()...)
	// (e) la copia della proposta aperta DP segue la AR attiva
	rifiuto23(t, p, "copia non allineata alla AR", "P4X10", "copia di compatibilita'",
		c3ConAR([]any{pb("{A2}", "k-p")}, nascita("{AR2}", "{A2}", "{P}", "k-p", "{C3}", "{K3}")...).istruzioni(t)...)
	// l'idempotenza: con la stessa chiave l'atto non fa rinascere la stessa coppia (anche dopo averla chiusa)
	rifiuto23(t, p, "stessa chiave, stessa coppia", "23505", "ux_fra_chiave", k(append(append(append([]string{}, n...),
		`UPDATE file_rfq_associazione SET stato = 'chiusa', chiusa_da = '{U}', motivo_chiusura = 'scollega_file' WHERE associazione_id = '{AR1}'`,
		`INSERT INTO associazione_evento (thread_id, allegato_id, associazione_id, azione, componente_prima, origine, autore_id)
		 VALUES ('{T}', '{A3}', '{AR1}', 'sgancia', '{S}', 'ponte', '{U}')`),
		nascita("{ARX}", "{A3}", "{S}", "k-s", "{C3}", "{K3}")...)...)...)
	// provvisorio, base [T] D-AR08 A: con la BOM congelata un file non si associa
	rifiuto23(t, p, "BOM congelata", "BOM01", "un file si associa solo aprendo una revisione", append(ar(congelaBomT...), k(n...)...)...)
	// la conferma valida con le tre AR (A3 senza documento_proposta: Prova 2; A2 con la copia) e' in TestV1ARDueConfermeSuccessive
}

// Due conferme successive (Prova 10 a livello di vincoli): C3 registra A3 → S, A2 → P, A5 → S; fra le due, le righe si
// chiudono sole e non si cambiano; C5 sostituisce A3 → P, svincola A2 da P e conferma di nuovo A5 → S senza toccarla. La
// storia si ricostruisce dal database: chi e' nato e chi e' stato chiuso da quale conferma.
func TestV1ARDueConfermeSuccessive(t *testing.T) {
	p := schema23ConStoria(t)
	riuscita23(t, p, "conferma C3 con tre AR", confermaC3ConTreAR().istruzioni(t)...)
	if s := uno23[string](t, p, sostAR.Replace(`SELECT string_agg(allegato_id || '>' || componente_id || ':' || stato || ':' || conferma_id, ' ' ORDER BY associazione_id)
		FROM file_rfq_associazione`)); s != uuidAR("{A3}")+">"+u23("{S}")+":attiva:"+u23("{C3}")+" "+uuidAR("{A2}")+">"+u23("{P}")+":attiva:"+u23("{C3}")+" "+
		uuidAR("{A5}")+">"+u23("{S}")+":attiva:"+u23("{C3}") {
		t.Errorf("AR dopo C3: %s", s)
	}
	if n := uno23[int](t, p, `SELECT count(*) FROM associazione_evento WHERE conferma_id = '{C3}' AND azione = 'preassociazione' AND tx IS NOT NULL
		AND associazione_id IS NOT NULL AND allegato_id IS NOT NULL`); n != 3 {
		t.Errorf("eventi di nascita di C3: %d", n)
	}

	// fra le due conferme: niente cambia una AR se non la chiusura; non si cancella; una chiusa non si riapre
	rifiuto23(t, p, "spostare la AR su un altro componente", "P4X10", "identita', atto e nascita non cambiano",
		ar(`UPDATE file_rfq_associazione SET componente_id = '{P}' WHERE associazione_id = '{AR1}'`)...)
	rifiuto23(t, p, "aggiornare senza chiudere", "P4X10", "l'unico cambiamento ammesso e' la chiusura",
		ar(`UPDATE file_rfq_associazione SET stato = 'attiva' WHERE associazione_id = '{AR1}'`)...)
	rifiuto23(t, p, "cancellare una AR", "P4X10", "si chiude", ar(`DELETE FROM file_rfq_associazione WHERE associazione_id = '{AR1}'`)...)
	rifiuto23(t, p, "svuotare le AR", "P4X10", "non si svuotano", `TRUNCATE file_rfq_associazione CASCADE`)
	rifiuto23(t, p, "chiudere senza evento", "P4X10", "la chiusura non ha il suo evento",
		ar(`UPDATE file_rfq_associazione SET stato = 'chiusa', chiusa_da = '{U}', motivo_chiusura = 'scollega_file' WHERE associazione_id = '{AR5}'`)...)
	rifiuto23(t, p, "svincolo di R5 fuori dalla sua conferma", "P4X10", "la conferma che chiude non e' una formato 3 di questa transazione",
		ar(`UPDATE file_rfq_associazione SET stato = 'chiusa', chiusa_da = '{U}', motivo_chiusura = 'svincolo_conferma', chiusa_conferma_id = '{C3}',
			chiusa_chiave = '{K3}' WHERE associazione_id = '{AR5}'`)...)
	rifiuto23(t, p, "chiudere la AR senza togliere la copia", "P4X10", "copia di compatibilita'", ar(
		`UPDATE file_rfq_associazione SET stato = 'chiusa', chiusa_da = '{U}', motivo_chiusura = 'file_generale' WHERE associazione_id = '{AR2}'`,
		`INSERT INTO associazione_evento (thread_id, proposta_id, associazione_id, azione, componente_prima, origine, autore_id)
		 VALUES ('{T}', '{DP}', '{AR2}', 'sgancia', '{P}', 'legacy', '{U}')`)...)
	// §2.1 punto 4 visto dall'occorrenza: Altri file con una AR attiva no (D-ASS-03: prima lo svincolo)
	rifiuto23(t, p, "Altri file con la AR attiva", "P4X10", "associazione registrata ancora attiva", inAltriFile("{A5}")...)
	// provvisorio, base [T] D-AR08 A: con la BOM congelata scollegare resta libero
	passaAiControlli(t, p, "chiusura con la BOM congelata", append(ar(congelaBomT...), ar(
		`UPDATE file_rfq_associazione SET stato = 'chiusa', chiusa_da = '{U}', motivo_chiusura = 'scollega_file' WHERE associazione_id = '{AR5}'`,
		`INSERT INTO associazione_evento (thread_id, allegato_id, associazione_id, azione, componente_prima, origine, autore_id)
		 VALUES ('{T}', '{A5}', '{AR5}', 'sgancia', '{S}', 'ponte', '{U}')`)...)...)
	// M-47 (variante 20261019d, direttiva 39 dopo C-6b): il trigger della copia derivata e' acceso
	if g := uno23[string](t, p, `SELECT tgenabled::text FROM pg_trigger WHERE tgname = 'trg_proposta_copia_derivata'`); g != "O" {
		t.Errorf("trg_proposta_copia_derivata: tgenabled %s, atteso O (acceso)", g)
	}
	// il giornale: l'evento ripete la riga; l'allegato viene dalla proposta; la stessa chiave non scrive due volte; sola aggiunta
	rifiuto23(t, p, "evento che non ripete la riga", "P4X10", "non ripete allegato, componente o conferma", ar(
		`INSERT INTO associazione_evento (thread_id, allegato_id, associazione_id, azione, componente_prima, origine, autore_id)
		 VALUES ('{T}', '{A3}', '{AR1}', 'sgancia', '{P}', 'ponte', '{U}')`)...)
	riuscita23(t, p, "evento della proposta senza allegato", `INSERT INTO associazione_evento (thread_id, proposta_id, azione, origine,
		chiave_idempotenza, autore_id) VALUES ('{T}', '{DP}', 'riapri', 'ponte', '{K1}', '{U}')`)
	if a := uno23[string](t, p, `SELECT allegato_id::text FROM associazione_evento WHERE chiave_idempotenza = '{K1}' AND azione = 'riapri'`); a != u23("{A2}") {
		t.Errorf("allegato dell'evento della proposta: %s", a)
	}
	rifiuto23(t, p, "stesso evento con la stessa chiave", "23505", "ux_associazione_evento_chiave",
		`INSERT INTO associazione_evento (thread_id, proposta_id, azione, origine, chiave_idempotenza, autore_id)
		 VALUES ('{T}', '{DP}', 'riapri', 'ponte', '{K1}', '{U}')`)
	rifiuto23(t, p, "modificare un evento", "P4X02", "non si modifica", `UPDATE associazione_evento SET motivo = 'altro' WHERE conferma_id = '{C3}'`)
	rifiuto23(t, p, "cancellare un evento", "P4X02", "non si modifica", `DELETE FROM associazione_evento WHERE conferma_id = '{C3}'`)

	// la bozza B5 di P sulla conferma C3, e la conferma C5 che la chiude
	snap5 := snapshot3()
	snap5["versione_bom_base"] = map[string]any{"conferma_id": u23("{C3}"), "numero": 2, "contatore": 0}
	snap5["documenti_preassociati"] = []any{pb("{A3}", "k-p"), pb("{A5}", "k-s")}
	snap5["decisioni"] = append(snap5["decisioni"].([]any), map[string]any{"id": "sd-1", "tipo": "scollega_documento",
		"allegato_id": u23("{A2}"), "componente_id": u23("{P}")})
	riuscita23(t, p, "bozza B5", ar(`INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, formato, base_conferma_id, base_numero, base_contatore,
			firma_base, fonte_allegato_id, fonte_sha256, fonte_gruppo, creata_da, aggiornata_da)
		 VALUES ('{B5}', '{T}', '{P}', 3, '{C3}', 2, 0, repeat('1', 64), '{A}', repeat('b', 64), 'bom', '{U}', '{U}')`,
		`INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_conferma_id, base_numero, base_contatore, firma_base,
			fonte_allegato_id, fonte_sha256, fonte_gruppo, salvata_da, importazione_id, chiave_salvataggio, impronta_richiesta)
		 VALUES ('{B5}', 1, `+jsonSQL(t, snap5)+`, repeat('8', 64), '{C3}', 2, 0, repeat('1', 64), '{A}', repeat('b', 64), 'bom', '{U}',
			'{R1}', gen_random_uuid(), repeat('4', 64))`,
		`INSERT INTO distinta_bozza_evento (bozza_id, n, versione, vocabolario, azione, origine, utente_id)
		 VALUES ('{B5}', 1, 1, 2, 'bozza_aperta', 'server', '{U}')`)...)
	c5 := func(snap map[string]any, dopo ...string) []string {
		c := nuovaC3()
		c.snap = snap
		c.testata = map[string]string{"conferma_id": "'{C5}'", "numero": "3", "precedente_id": "'{C3}'", "base_conferma_id": "'{C3}'",
			"bozza_id": "'{B5}'", "chiave_idempotenza": "'{K5}'", "impronta_snapshot": "repeat('8', 64)"}
		in5 := strings.NewReplacer("'{C3}'", "'{C5}'")
		for _, l := range []*[]string{&c.componenti, &c.archi, &c.nonInclusi} {
			for i := range *l {
				(*l)[i] = in5.Replace((*l)[i])
			}
		}
		c.dopo = ar(dopo...)
		out := ar(c.istruzioni(t)...)
		for i := range out {
			out[i] = strings.Replace(out[i], "WHERE bozza_id = '{B3}'", "WHERE bozza_id = '"+uuidAR("{B5}")+"'", 1)
		}
		return out
	}
	sostituzione := []string{
		`UPDATE file_rfq_associazione SET stato = 'chiusa', chiusa_da = '{U}', motivo_chiusura = 'sostituita', chiusa_conferma_id = '{C5}',
			chiusa_chiave = '{K5}' WHERE associazione_id = '{AR1}'`,
		`INSERT INTO file_rfq_associazione (associazione_id, thread_id, allegato_id, componente_id, origine, conferma_id, chiave_bozza, sostituisce_id,
			provenienza_pb, chiave_idempotenza, creata_da)
		 VALUES ('{AR3}', '{T}', '{A3}', '{P}', 'conferma_bom', '{C5}', 'k-p', '{AR1}', 'manuale', '{K5}', '{U}')`,
		`INSERT INTO associazione_evento (thread_id, allegato_id, associazione_id, associazione_precedente_id, azione, componente_prima, componente_dopo,
			conferma_id, origine, chiave_idempotenza, autore_id)
		 VALUES ('{T}', '{A3}', '{AR3}', '{AR1}', 'sostituisci', '{S}', '{P}', '{C5}', 'conferma_bom', '{K5}', '{U}')`,
	}
	svincolo := []string{
		`UPDATE file_rfq_associazione SET stato = 'chiusa', chiusa_da = '{U}', motivo_chiusura = 'svincolo_conferma', chiusa_conferma_id = '{C5}',
			chiusa_chiave = '{K5}' WHERE associazione_id = '{AR2}'`,
		`INSERT INTO associazione_evento (thread_id, allegato_id, associazione_id, azione, componente_prima, conferma_id, origine, chiave_idempotenza,
			autore_id)
		 VALUES ('{T}', '{A2}', '{AR2}', 'sgancia', '{P}', '{C5}', 'conferma_bom', '{K5}', '{U}')`,
		copiaDP("", ""),
	}
	// C5 senza applicare lo svincolo della bozza: il controllo al COMMIT della conferma lo vede
	rifiuto23(t, p, "svincolo della bozza non applicato", "P4X10", "non e' stato applicato", c5(snap5, sostituzione...)...)
	// la sostituzione senza chiudere la riga di prima: la AR attiva dello stesso allegato la ferma
	rifiuto23(t, p, "sostituzione silenziosa", "23505", "ux_fra_attiva_allegato", c5(snap5, append(append([]string{}, sostituzione[1:]...), svincolo...)...)...)
	// (c) una chiusura svincolo_conferma vuole il suo scollega_documento nello snapshot (A5 non lo ha; ne' la sua PB)
	snapC := snapshot3()
	snapC["documenti_preassociati"] = []any{pb("{A3}", "k-p")}
	snapC["decisioni"] = snap5["decisioni"]
	rifiuto23(t, p, "svincolo di R5 senza la sua decisione", "P4X10", "non ha lo svincolo (scollega_documento)", c5(snapC, append(append(append([]string{},
		sostituzione...), svincolo...),
		`UPDATE file_rfq_associazione SET stato = 'chiusa', chiusa_da = '{U}', motivo_chiusura = 'svincolo_conferma', chiusa_conferma_id = '{C5}',
			chiusa_chiave = '{K5}' WHERE associazione_id = '{AR5}'`,
		`INSERT INTO associazione_evento (thread_id, allegato_id, associazione_id, azione, componente_prima, conferma_id, origine, chiave_idempotenza,
			autore_id)
		 VALUES ('{T}', '{A5}', '{AR5}', 'sgancia', '{S}', '{C5}', 'conferma_bom', '{K5}', '{U}')`)...)...)
	// (d) la sostituzione cita una riga chiusa per un altro motivo
	rifiuto23(t, p, "sostituzione di una riga non chiusa come sostituita", "P4X10", "la riga sostituita", c5(snap5, append(append([]string{
		`UPDATE file_rfq_associazione SET stato = 'chiusa', chiusa_da = '{U}', motivo_chiusura = 'scollega_file', chiusa_chiave = '{K5}'
		  WHERE associazione_id = '{AR1}'`}, sostituzione[1:]...), svincolo...)...)...)
	riuscita23(t, p, "conferma C5: sostituzione, svincolo, A5 confermata di nuovo", c5(snap5, append(append([]string{}, sostituzione...), svincolo...)...)...)

	// la storia, solo dal database
	storia := uno23[string](t, p, sostAR.Replace(`SELECT string_agg(r.associazione_id || ':' || r.stato || ':' || r.conferma_id || ':' || COALESCE(r.motivo_chiusura, '-')
			|| ':' || COALESCE(r.chiusa_conferma_id::text, '-') || ':' || COALESCE(r.sostituisce_id::text, '-'), ' ' ORDER BY r.associazione_id)
		FROM file_rfq_associazione r`))
	attesa := strings.Join([]string{
		uuidAR("{AR1}") + ":chiusa:" + u23("{C3}") + ":sostituita:" + uuidAR("{C5}") + ":-",
		uuidAR("{AR2}") + ":chiusa:" + u23("{C3}") + ":svincolo_conferma:" + uuidAR("{C5}") + ":-",
		uuidAR("{AR3}") + ":attiva:" + uuidAR("{C5}") + ":-:-:" + uuidAR("{AR1}"),
		uuidAR("{AR5}") + ":attiva:" + u23("{C3}") + ":-:-:-",
	}, " ")
	if storia != attesa {
		t.Errorf("storia delle AR:\n%s\nattesa:\n%s", storia, attesa)
	}
	// la AR confermata di nuovo da C5 senza cambiamenti e' la stessa riga, nata con C3 (nessun doppione, nessuna riscrittura)
	if n := uno23[int](t, p, sostAR.Replace(`SELECT count(*) FROM file_rfq_associazione WHERE allegato_id = '{A5}'`)); n != 1 {
		t.Errorf("righe di A5: %d", n)
	}
	// le AR attive al momento di ciascuna conferma, ricostruite dalle date della conferma
	perConferma := func(c string) string {
		return uno23[string](t, p, sostAR.Replace(`SELECT string_agg(r.allegato_id || '>' || r.componente_id, ' ' ORDER BY r.allegato_id)
			FROM file_rfq_associazione r JOIN conferma_strutturale c ON c.conferma_id = '`+c+`'
		   WHERE r.creata_il <= c.confermato_il AND (r.chiusa_il IS NULL OR r.chiusa_il > c.confermato_il
		         OR (r.chiusa_il = c.confermato_il AND r.chiusa_conferma_id IS DISTINCT FROM c.conferma_id))`))
	}
	if s, atteso := perConferma("{C3}"), strings.Join([]string{u23("{A2}") + ">" + u23("{P}"), uuidAR("{A3}") + ">" + u23("{S}"),
		uuidAR("{A5}") + ">" + u23("{S}")}, " "); s != atteso {
		t.Errorf("AR applicate a C3: %s, attese %s", s, atteso)
	}
	if s, atteso := perConferma("{C5}"), strings.Join([]string{uuidAR("{A3}") + ">" + u23("{P}"), uuidAR("{A5}") + ">" + u23("{S}")}, " "); s != atteso {
		t.Errorf("AR applicate a C5: %s, attese %s", s, atteso)
	}
	if n := uno23[int](t, p, sostAR.Replace(`SELECT count(*) FROM associazione_evento WHERE conferma_id = '{C5}'`)); n != 2 {
		t.Errorf("eventi di C5: %d (una sostituzione e uno svincolo)", n)
	}
	if c := uno23[string](t, p, `SELECT COALESCE(componente_id::text, 'nessuna') FROM documento_proposta WHERE proposta_id = '{DP}'`); c != "nessuna" {
		t.Errorf("copia di A2 dopo lo svincolo: %s", c)
	}
	rifiuto23(t, p, "una AR chiusa non si riapre", "P4X10", "non si riapre",
		ar(`UPDATE file_rfq_associazione SET stato = 'attiva', chiusa_il = NULL, chiusa_da = NULL, motivo_chiusura = NULL, chiusa_conferma_id = NULL,
			chiusa_chiave = NULL, chiusa_tx = NULL WHERE associazione_id = '{AR1}'`)...)

	// D-ASS-03 / V1-Q42 [U]: Scollega e metti in Altri file, nello stesso gesto (prima lo svincolo, poi la conservazione); poi
	// «Riattiva» non riapre la AR chiusa
	riuscita23(t, p, "Scollega e metti in Altri file", append(ar(
		`UPDATE file_rfq_associazione SET stato = 'chiusa', chiusa_da = '{U}', motivo_chiusura = 'scollega_file' WHERE associazione_id = '{AR5}'`,
		`INSERT INTO associazione_evento (thread_id, allegato_id, associazione_id, azione, componente_prima, origine, autore_id)
		 VALUES ('{T}', '{A5}', '{AR5}', 'sgancia', '{S}', 'ponte', '{U}')`), inAltriFile("{A5}")...)...)
	riuscita23(t, p, "Riattiva", `UPDATE file_rfq SET stato = 'attivo', versione = versione + 1, aggiornato_da = '{U}'
		WHERE thread_id = '{T}' AND allegato_id = '{A5}'`, `INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, stato_prima, stato_dopo,
		fotografia, origine, autore_id) VALUES ('{T}', '{A5}', 'riattiva', 'altri_file', 'attivo', '{}', 'ponte', '{U}')`)
	if s := uno23[string](t, p, sostAR.Replace(`SELECT stato FROM file_rfq_associazione WHERE associazione_id = '{AR5}'`)); s != "chiusa" {
		t.Errorf("AR di A5 dopo Riattiva: %s", s)
	}
	// la vista diagnostica del backfill non conta le AR come pre-assegnazioni: A2 non ha piu' la copia
	if n := uno23[int](t, p, `SELECT count(*) FROM v_ar_backfill_diagnostica WHERE classe LIKE 'P%'`); n != 0 {
		t.Errorf("pre-assegnazioni nella diagnostica: %d", n)
	}
}

// Lo svincolo gia' chiuso (piano §3.6, esito svincolo_gia_chiuso): la corsia file chiude la AR dopo lo svincolo R2
// (D-ASS-03), la bozza conserva lo scollega_documento, e la conferma successiva lo accetta senza una chiusura nuova.
func TestV1ARSvincoloGiaChiuso(t *testing.T) {
	p := schema23ConStoria(t)
	riuscita23(t, p, "conferma C3 con tre AR", confermaC3ConTreAR().istruzioni(t)...)
	riuscita23(t, p, "Scollega e scarta: la chiusura della corsia file", ar(
		`UPDATE file_rfq_associazione SET stato = 'chiusa', chiusa_da = '{U}', motivo_chiusura = 'scollega_file', chiusa_chiave = '{K1}'
		  WHERE associazione_id = '{AR2}'`,
		`INSERT INTO associazione_evento (thread_id, proposta_id, associazione_id, azione, componente_prima, origine, chiave_idempotenza, autore_id)
		 VALUES ('{T}', '{DP}', '{AR2}', 'sgancia', '{P}', 'ponte', '{K1}', '{U}')`,
		copiaDP("", ""))...)
	if a := uno23[string](t, p, sostAR.Replace(`SELECT allegato_id::text FROM associazione_evento WHERE associazione_id = '{AR2}' AND azione = 'sgancia'`)); a != u23("{A2}") {
		t.Errorf("allegato dello sgancio della proposta: %s", a)
	}
	snap5 := snapshot3()
	snap5["documenti_preassociati"] = []any{pb("{A3}", "k-s"), pb("{A5}", "k-s")}
	snap5["decisioni"] = append(snap5["decisioni"].([]any), map[string]any{"id": "sd-1", "tipo": "scollega_documento",
		"allegato_id": u23("{A2}"), "componente_id": u23("{P}")})
	riuscita23(t, p, "bozza B5", ar(`INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, formato, base_conferma_id, base_numero, base_contatore,
			firma_base, fonte_allegato_id, fonte_sha256, fonte_gruppo, creata_da, aggiornata_da)
		 VALUES ('{B5}', '{T}', '{P}', 3, '{C3}', 2, 0, repeat('1', 64), '{A}', repeat('b', 64), 'bom', '{U}', '{U}')`,
		`INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_conferma_id, base_numero, base_contatore, firma_base,
			fonte_allegato_id, fonte_sha256, fonte_gruppo, salvata_da, importazione_id, chiave_salvataggio, impronta_richiesta)
		 VALUES ('{B5}', 1, `+jsonSQL(t, snap5)+`, repeat('8', 64), '{C3}', 2, 0, repeat('1', 64), '{A}', repeat('b', 64), 'bom', '{U}',
			'{R1}', gen_random_uuid(), repeat('4', 64))`,
		`INSERT INTO distinta_bozza_evento (bozza_id, n, versione, vocabolario, azione, origine, utente_id)
		 VALUES ('{B5}', 1, 1, 2, 'bozza_aperta', 'server', '{U}')`)...)
	c := nuovaC3()
	c.snap = snap5
	c.testata = map[string]string{"conferma_id": "'{C5}'", "numero": "3", "precedente_id": "'{C3}'", "base_conferma_id": "'{C3}'",
		"bozza_id": "'{B5}'", "chiave_idempotenza": "'{K5}'", "impronta_snapshot": "repeat('8', 64)"}
	in5 := strings.NewReplacer("'{C3}'", "'{C5}'")
	for _, l := range []*[]string{&c.componenti, &c.archi, &c.nonInclusi} {
		for i := range *l {
			(*l)[i] = in5.Replace((*l)[i])
		}
	}
	out := ar(c.istruzioni(t)...)
	for i := range out {
		out[i] = strings.Replace(out[i], "WHERE bozza_id = '{B3}'", "WHERE bozza_id = '"+uuidAR("{B5}")+"'", 1)
	}
	riuscita23(t, p, "conferma C5 con lo svincolo gia' chiuso", out...)
	if n := uno23[int](t, p, sostAR.Replace(`SELECT count(*) FROM file_rfq_associazione WHERE chiusa_conferma_id = '{C5}' OR conferma_id = '{C5}'`)); n != 0 {
		t.Errorf("righe toccate da C5: %d (lo svincolo era gia' soddisfatto, le altre AR confermate di nuovo)", n)
	}
	// M-47 (variante 20261019d): con trg_proposta_copia_derivata acceso un cambio della copia senza evento e' rifiutato al
	// COMMIT, anche verso nessun componente; l'evento di un altro allegato non basta; con l'evento dello stesso allegato
	// nella stessa transazione passa (cosi' scrive il servizio unico)
	rifiuto23(t, p, "copia cambiata senza evento", "P4X10", "copia derivata", copiaDP("{S}", "ACM-1100"))
	rifiuto23(t, p, "copia cambiata con l'evento di un altro allegato", "P4X10", "copia derivata", ar(
		`INSERT INTO associazione_evento (thread_id, allegato_id, azione, componente_prima, origine, autore_id)
		 VALUES ('{T}', '{A3}', 'sgancia', '{S}', 'legacy', '{U}')`, copiaDP("{S}", "ACM-1100"))...)
	riuscita23(t, p, "copia cambiata con l'evento dello stesso allegato", ar(copiaDP("{S}", "ACM-1100"),
		`INSERT INTO associazione_evento (thread_id, proposta_id, allegato_id, azione, componente_dopo, origine, autore_id)
		 VALUES ('{T}', '{DP}', '{A2}', 'assegna', '{S}', 'ponte', '{U}')`)...)
	rifiuto23(t, p, "copia tolta senza evento", "P4X10", "copia derivata", copiaDP("", ""))
	// un cambio che lascia lo stesso componente (solo il codice nella stessa istruzione) non e' un cambio della copia
	riuscita23(t, p, "stesso componente", copiaDP("{S}", "ACM-1100"))
	// la pre-assegnazione senza AR che la diagnostica conta come P1 resta un dato legacy (scritto prima del trigger)
	riuscita23(t, p, "copia tolta con l'evento", ar(copiaDP("", ""),
		`INSERT INTO associazione_evento (thread_id, proposta_id, allegato_id, azione, componente_prima, origine, autore_id)
		 VALUES ('{T}', '{DP}', '{A2}', 'sgancia', '{S}', 'legacy', '{U}')`)...)
	riuscita23(t, p, "pre-assegnazione legacy", copiaLegacyDP("{S}", "ACM-1100")...)
	if g := uno23[string](t, p, `SELECT tgenabled::text FROM pg_trigger WHERE tgname = 'trg_proposta_copia_derivata'`); g != "O" {
		t.Errorf("trg_proposta_copia_derivata dopo il dato legacy: tgenabled %s, atteso O", g)
	}
	if n := uno23[int](t, p, `SELECT count(*) FROM v_ar_backfill_diagnostica WHERE classe = 'P1' AND allegato_id = '{A2}' AND NOT ha_ar_attiva`); n != 1 {
		t.Errorf("A2 come pre-assegnazione senza AR nella diagnostica: %d", n)
	}
}

// La vista diagnostica del backfill (piano §5.3; PM23 M-48): una riga per classe su dati sintetici, in sola lettura;
// nessuna AR nasce (INC-S2).
func TestV1ARLaDiagnosticaDelBackfill(t *testing.T) {
	p := schema23ConStoria(t)
	// P1: pre-assegnazione del vecchio «Assegna» sulla proposta aperta DP (A2), occorrenza attiva, senza autore: un dato
	// legacy, scritto con il trigger della copia spento (variante 20261019d)
	riuscita23(t, p, "la pre-assegnazione P1", ar(copiaLegacyDP("{S}", "ACM-1100")...)...)
	riuscita23(t, p, "le classi", ar(
		// P1s: lo STEP A del finito P, proposta aperta con il componente
		`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte)
		 VALUES ('{A}', '{T}', 'cad_3d', 'ACM-1000', '{P}', 50, 'nome_file')`,
		// P3: la proposta di A3 scartata da una persona, con il componente
		`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte, stato, deciso_da, deciso_il)
		 VALUES ('{A3}', '{T}', 'disegno_2d', 'ACM-1100', '{S}', 50, 'nome_file', 'scartata', '{U}', now())`,
		// P2: la proposta aperta di A5 con il componente, l'occorrenza in Altri file (sotto)
		`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte)
		 VALUES ('{A5}', '{T}', 'disegno_2d', 'ACM-1100', '{S}', 50, 'nome_file')`,
		// D1: il documento confermato D2 di S arrivato da A4, senza proposta
		`INSERT INTO documento_provenienza (documento_id, allegato_id, ricevuto_il) VALUES ('{D2}', '{A4}', now())`,
		// C1: il documento D di P arrivato da AE, con la proposta sullo stesso componente
		`INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il) VALUES
		   ('{AE}', '{M}', 9, 'ACM-1000_c.pdf', 'pdf', repeat('7', 64), now()), ('{AY}', '{M}', 10, 'ACM-1000_d.pdf', 'pdf', repeat('8', 64), now())`,
		`INSERT INTO documento_provenienza (documento_id, allegato_id, ricevuto_il) VALUES ('{D}', '{AE}', now() - interval '2 minutes'),
		   ('{D}', '{AY}', now() - interval '1 minute')`,
		`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte, stato, deciso_da, deciso_il)
		 VALUES ('{AE}', '{T}', 'disegno_2d', 'ACM-1000', '{P}', 50, 'nome_file', 'confermata', '{U}', now())`,
		// X: il documento D di P arrivato da AY, con la copia su S
		`INSERT INTO documento_proposta (allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte)
		 VALUES ('{AY}', '{T}', 'disegno_2d', 'ACM-1100', '{S}', 50, 'nome_file')`)...)
	riuscita23(t, p, "A5 in Altri file", inAltriFile("{A5}")...)
	if s := uno23[string](t, p, `SELECT string_agg(classe, ',' ORDER BY classe) FROM v_ar_backfill_diagnostica`); s != "C1,D1,P1,P1s,P2,P3,X" {
		t.Errorf("classi della diagnostica: %s", s)
	}
	if s := uno23[string](t, p, sostAR.Replace(`SELECT string_agg(classe || ':' || autore_registrato || ':' || occorrenza_attiva || ':' || step_su_finito || ':' || ha_ar_attiva,
			' ' ORDER BY classe) FROM v_ar_backfill_diagnostica WHERE allegato_id IN ('{A2}', '{A}', '{A3}', '{A5}')`)); s !=
		"P1:false:true:false:false P1s:false:true:true:false P2:false:false:false:false P3:true:false:false:false" {
		t.Errorf("colonne della diagnostica: %s", s)
	}
	if n := contaRighe(t, p, "file_rfq_associazione"); n != 0 {
		t.Errorf("la diagnostica ha fatto nascere %d AR", n)
	}
}

// contaRighe: le righe di una tabella.
func contaRighe(t *testing.T, p *pgxpool.Pool, tab string) int {
	return uno23[int](t, p, "SELECT count(*) FROM "+tab)
}
