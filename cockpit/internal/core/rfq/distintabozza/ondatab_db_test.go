//go:build integrazione

// L4 — V1, ondata B, corsia persistenza (PM23 §14, passo 6, prima parte): le rotte della bozza nel formato 3 sul database
// di prova, nel package che le rotte usano:
//   - R2 forma salvataggio (SalvaFormato3): la versione nuova con il giornale del vocabolario 2, la chiave e l'impronta
//     della richiesta, il reinvio, CHIAVE_RIUSATA, BOZZA_VERSIONE, la pila d'annullamento che si ferma alla versione del
//     server (PE-05), lo svincolo per nodo escluso (PE-22) e il vassoio, i riferimenti nuovi (FUORI_RFQ, FILE_NON_ATTIVO),
//     le identita' (proposta, registrato, emittente), il segnale visto (PE-17), lo svincolo che cita l'esclusione (PE-27);
//   - R1 (LeggiR1): testata, n_eventi ed eventi (PE-02), registro, rif scaduti, base, vassoio, perimetro;
//   - R6 (LeggiIntento): non_registrato, registrato con F8, FUORI_RFQ, in_corso;
//   - R7 (LeggiStorico): i formati 1, 2 e 3 (U-06), contenuto_non_leggibile, i prerequisiti registrati (U2 §5.4) e confermato_da_nome (PE-07);
//   - la conversione formato 2 → 3 (U-05, conversione-1) e la parte di U-05b che si fa senza R4 (i nodi formato_2 in R2).
// Solo dati sintetici: la scena del database di persistenza_db_test.go e gli aiuti di formato3_aiuti_test.go.

package distintabozza

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// hashCfg3: la configurazione dell'analizzatore corrente delle prove (inventata).
const hashCfg3 = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

// fattiStep3: i fatti correnti di uno STEP con le chiavi PRODUCT date (la struttura v3 che legge chiaviDeiFatti).
func fattiStep3(t *testing.T, chiavi ...string) string {
	t.Helper()
	nodi := make([]map[string]any, 0, len(chiavi))
	for _, k := range chiavi {
		nodi = append(nodi, map[string]any{"chiave": k})
	}
	b, err := json.Marshal(map[string]any{"struttura": map[string]any{"versione": 3, "radici": []string{chiavi[0]}, "nodi": nodi,
		"relazioni": []any{}, "avvisi": []any{}, "limiti": map[string]any{"troncato": false},
		"scarti": map[string]any{"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// scena3DB: la scena del database con la bozza formato 3 del prodotto alla versione 1 (snapshotProva3 sul registro di
// componiProva3, salvata con la chiave chiave0), l'evento di apertura del server, la proposta della preassociazione (sul
// PDF della scena) e i fatti correnti dello STEP della scena.
type scena3DB struct {
	scenaDB
	p              *pgxpool.Pool
	id             idProva3
	reg            importazione.Registro
	importazioneID uuid.UUID
	bozza          uuid.UUID
	chiave0        uuid.UUID
	base           map[string]any
	proposta       uuid.UUID
}

func preparaBozza3(t *testing.T) scena3DB {
	t.Helper()
	p, s := poolPulito(t)
	ctx := context.Background()
	id := idSintetici3()
	sc := scena3DB{scenaDB: s, p: p, id: id, reg: componiProva3(t, id), chiave0: uuid.MustParse("00000000-0000-4000-8000-0000000e0f01"),
		proposta: uuid.MustParse("00000000-0000-4000-8000-0000000d0001")}
	sc.base = snapshotProva3(id, sc.reg.Impronta)
	snap := leggiProva3(t, sc.base)
	can, err := snap.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)
	riga, err := q.InsertDistintaImportazione(ctx, ParametriImportazione(sc.reg, id.utente))
	if err != nil {
		t.Fatalf("registro: %v", err)
	}
	sc.importazioneID = riga.ImportazioneID
	fonte := uuid.NullUUID{UUID: id.allegato, Valid: true}
	sha := pgtype.Text{String: id.sha, Valid: true}
	gruppo := pgtype.Text{String: gruppoProva3, Valid: true}
	b, err := q.InsertDistintaBozza(ctx, db.InsertDistintaBozzaParams{ThreadID: id.thread, ProdottoID: id.prodotto, Formato: FormatoSnapshot3,
		BaseContatore: 12, FirmaBase: firmaProva, FonteAllegatoID: fonte, FonteSha256: sha, FonteGruppo: gruppo, UtenteID: id.utente})
	if err != nil {
		t.Fatalf("bozza: %v", err)
	}
	sc.bozza = b.BozzaID
	if _, err := q.InsertDistintaBozzaVersione(ctx, db.InsertDistintaBozzaVersioneParams{BozzaID: b.BozzaID, Versione: 1, Snapshot: can,
		Impronta: jsoncanonico.Impronta(can), BaseContatore: 12, FirmaBase: firmaProva, FonteAllegatoID: fonte, FonteSha256: sha, FonteGruppo: gruppo,
		SalvataDa: id.utente, ImportazioneID: uuid.NullUUID{UUID: riga.ImportazioneID, Valid: true},
		ChiaveSalvataggio: uuid.NullUUID{UUID: sc.chiave0, Valid: true}, ImprontaRichiesta: pgtype.Text{String: strings.Repeat("4", 64), Valid: true}}); err != nil {
		t.Fatalf("versione 1: %v", err)
	}
	if err := q.InsertDistintaBozzaEvento(ctx, db.InsertDistintaBozzaEventoParams{BozzaID: b.BozzaID, N: 1, Versione: 1,
		Vocabolario: int16(VocabolarioFormato3), Azione: AzBozzaAperta, Origine: "server", UtenteID: id.utente}); err != nil {
		t.Fatalf("apertura: %v", err)
	}
	for _, x := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO documento_proposta (proposta_id, allegato_id, thread_id, tipo_proposto, codice, confidenza, fonte)
			VALUES ($1, $2, $3, 'disegno_2d', 'ACM-1100', 80, 'nome_file')`, []any{sc.proposta, id.pdf, id.thread}},
		{`INSERT INTO analizzatore_corrente (versione_analizzatore, hash_configurazione) VALUES (3, $1)
			ON CONFLICT (unico) DO UPDATE SET versione_analizzatore = 3, hash_configurazione = $1`, []any{hashCfg3}},
		{`INSERT INTO analisi_fatti (sha256, versione_analizzatore, hash_configurazione, fatti) VALUES ($1, 3, $2, $3)`,
			[]any{id.sha, hashCfg3, fattiStep3(t, "#1", "#7", "#9", "#11", "#31")}},
	} {
		if _, err := tx.Exec(ctx, x.sql, x.args...); err != nil {
			t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return sc
}

// salva3 manda la forma salvataggio di R2 per la bozza della scena, con il corpo letto da LeggiRichiestaR2 (che calcola
// l'impronta della richiesta).
func (sc scena3DB) salva3(t *testing.T, versione int, chiave uuid.UUID, snap map[string]any, azioni []Azione) (Salvato3, error) {
	t.Helper()
	return sc.salva3Su(t, sc.bozza, sc.prodotto, versione, chiave, snap, azioni)
}

func (sc scena3DB) salva3Su(t *testing.T, bozza, prodotto uuid.UUID, versione int, chiave uuid.UUID, snap map[string]any, azioni []Azione) (Salvato3, error) {
	t.Helper()
	if azioni == nil {
		azioni = []Azione{}
	}
	raw, err := json.Marshal(map[string]any{"bozza_id": bozza, "versione_attesa": versione, "prodotto_id": prodotto, "vocabolario": 2,
		"chiave_salvataggio": chiave, "snapshot": snap, "azioni": azioni})
	if err != nil {
		t.Fatal(err)
	}
	r, err := LeggiRichiestaR2(raw)
	if err != nil {
		return Salvato3{}, err
	}
	s := r.Salvataggio
	s.ThreadID, s.UtenteID = sc.thread, sc.utente
	return SalvaFormato3(context.Background(), sc.p, s)
}

// leggiR1: R1 in una transazione in sola lettura (una scrittura la farebbe fallire).
func leggiR1DB(t *testing.T, p *pgxpool.Pool, thread, prodotto uuid.UUID, op OpzioniR1) (LetturaR1, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	return LeggiR1(ctx, db.New(tx), thread, prodotto, op)
}

func (sc scena3DB) leggiR1(t *testing.T, op OpzioniR1) LetturaR1 {
	t.Helper()
	l, err := leggiR1DB(t, sc.p, sc.thread, sc.prodotto, op)
	if err != nil {
		t.Fatalf("R1: %v", err)
	}
	return l
}

// codice3: l'errore tipizzato con il codice (e il motivo, se dato).
func codice3(t *testing.T, err error, codice, motivo string) *Errore {
	t.Helper()
	e := errore3(t, err)
	if e.Codice != codice || (motivo != "" && e.Motivo != motivo) {
		t.Fatalf("atteso %s/%s, avuto %+v", codice, motivo, e)
	}
	return e
}

func conta3(t *testing.T, p *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := p.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
	return n
}

func esegui3(t *testing.T, p *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := p.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%v\n%s", err, sql)
	}
}

// inAltriFile mette un'occorrenza della sua RFQ in Altri file come il ciclo dei file della 0023 (la copia verificata,
// la riga di stato con la fotografia dell'allegato, l'evento), in una transazione.
func inAltriFile(t *testing.T, p *pgxpool.Pool, allegato, utente uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, x := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO file_rfq_conservazione (thread_id, allegato_id, sha256, bytes, percorso_relativo, radice_id)
			SELECT m.thread_id, a.allegato_id, a.sha256, 10, 'ALTRO\' || a.allegato_id || '.pdf', gen_random_uuid()
			  FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE a.allegato_id = $1`, []any{allegato}},
		{`UPDATE file_rfq_conservazione SET stato = 'copiata', sorgente_byte = 'staging' WHERE allegato_id = $1 AND stato = 'in_copia'`, []any{allegato}},
		{`INSERT INTO file_rfq (thread_id, allegato_id, stato, versione, messaggio_id, canale, contenitore_id, indice, nome_file, path_interno,
				estensione, sha256, bytes, origine, ricevuto_il, caricato_da, creato_da, aggiornato_da)
			SELECT m.thread_id, a.allegato_id, 'altri_file', 1, a.messaggio_id, m.canale, a.contenitore_id, a.indice, a.nome_file, a.path_interno,
				a.estensione, a.sha256, a.bytes, a.origine, a.ricevuto_il, a.caricato_da, $2, $2
			  FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE a.allegato_id = $1`, []any{allegato, utente}},
		{`INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, stato_prima, stato_dopo, fotografia, dipendenze, origine, autore_id)
			SELECT m.thread_id, a.allegato_id, 'altri_file', 'attivo', 'altri_file', '{}', '{}', 'ponte', $2
			  FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE a.allegato_id = $1`, []any{allegato, utente}},
	} {
		if _, err := tx.Exec(ctx, x.sql, x.args...); err != nil {
			t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Altri file: %v", err)
	}
}

// nuovoAllegato: un allegato in piu' nel messaggio della RFQ della scena (o della seconda), con lo sha dato.
func nuovoAllegato(t *testing.T, p *pgxpool.Pool, messaggio uuid.UUID, indice int, nome, sha string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	est := nome[strings.LastIndex(nome, ".")+1:]
	esegui3(t, p, `INSERT INTO allegato (allegato_id, messaggio_id, indice, nome_file, estensione, sha256, ricevuto_il)
		VALUES ($1, $2, $3, $4, $5, $6, now())`, id, messaggio, indice, nome, est, sha)
	return id
}

// nuovaProposta: una proposta di documento su un allegato (una per allegato), con il codice del componente se ne ha uno
// (fk_proposta_componente).
func nuovaProposta(t *testing.T, p *pgxpool.Pool, allegato, thread uuid.UUID, componente *uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	esegui3(t, p, `INSERT INTO documento_proposta (proposta_id, allegato_id, thread_id, tipo_proposto, codice, componente_id, confidenza, fonte)
		VALUES ($1, $2, $3, 'disegno_2d', COALESCE((SELECT c.codice FROM componente c WHERE c.componente_id = $4), 'ACM-9000'), $4, 70, 'nome_file')`,
		id, allegato, thread, nullUUID(componente))
	return id
}

// arAttivaDiProva: una associazione registrata attiva (file_rfq_associazione) allegato → componente, scritta a mano per le
// prove di R2 (lo svincolo cita una AR attiva, EM §3.6.2; direttiva 35, C-2). La scriverebbe R5 attraverso il servizio
// unico (C-1c, C-5): qui una conferma formato 1 minima del prodotto, il componente certificato e la AR, con i trigger
// delle tre tabelle spenti solo nella transazione (le FK restano: fk_fra_conferma e, al COMMIT,
// fk_fra_conferma_componente). La prova e' su R2, non sulla nascita della AR (quella e' in platform/migrazioni).
func arAttivaDiProva(t *testing.T, p *pgxpool.Pool, thread, prodotto, allegato, componente, utente uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	conferma, ar := uuid.New(), uuid.New()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, x := range []struct {
		sql  string
		args []any
	}{
		{`ALTER TABLE conferma_strutturale DISABLE TRIGGER USER`, nil},
		{`ALTER TABLE conferma_strutturale_componente DISABLE TRIGGER USER`, nil},
		{`ALTER TABLE file_rfq_associazione DISABLE TRIGGER USER`, nil},
		{`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura,
				base_contatore, contatore, contatore_dati, origine, snapshot, impronta_snapshot, delta, mappa, criterio_verifica, confermato_da)
			VALUES ($1, $2, $3, (SELECT COALESCE(max(numero), 0) + 1 FROM conferma_strutturale WHERE prodotto_id = $3), 1, true,
				repeat('3', 64), 0, 0, 0, '{"rotta":"prova"}', '{"formato":1}', repeat('9', 64), '{}', '{}', 'nessuno', $4)`,
			[]any{conferma, thread, prodotto, utente}},
		{`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, codice, tipo)
			SELECT $1, c.thread_id, c.componente_id, c.codice, c.tipo FROM componente c WHERE c.componente_id = $2`, []any{conferma, componente}},
		{`INSERT INTO file_rfq_associazione (associazione_id, thread_id, allegato_id, componente_id, origine, conferma_id, chiave_bozza,
				provenienza_pb, chiave_idempotenza, creata_da)
			VALUES ($1, $2, $3, $4, 'conferma_bom', $5, 'k-prova', 'manuale', gen_random_uuid(), $6)`, []any{ar, thread, allegato, componente, conferma, utente}},
		// le FK differite si controllano qui: con eventi in sospeso la tabella non si riaccende (55006)
		{`SET CONSTRAINTS ALL IMMEDIATE`, nil},
		{`ALTER TABLE file_rfq_associazione ENABLE TRIGGER USER`, nil},
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
	return ar
}

// eventiDB: gli eventi della bozza dal numero dato in poi, come li ha scritti il database.
func eventiDB(t *testing.T, p *pgxpool.Pool, bozza uuid.UUID, dal int) []EventoLetto {
	t.Helper()
	rr, err := db.New(p).ListDistintaBozzaEventi(context.Background(), db.ListDistintaBozzaEventiParams{BozzaID: bozza,
		ThreadID: idDi("rfq"), VersioneMax: 1 << 30, DopoN: int32(dal - 1), Limite: 1000})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]EventoLetto, 0, len(rr))
	for _, r := range rr {
		out = append(out, eventoLetto(r))
	}
	return out
}

// campoJSON: un campo di un oggetto JSON salvato, nella sua forma canonica.
func campoJSON(t *testing.T, raw json.RawMessage, k string) string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s non e' un oggetto: %v", raw, err)
	}
	return string(canonico(m[k]))
}

// ------------------------------------------------------------------ R2, la forma salvataggio

func TestV1SalvaFormato3IlGiornaleELaChiave(t *testing.T) {
	sc := preparaBozza3(t)
	ctx := context.Background()
	nuovo := nodoManuale3(sc.id, "k-u1", "ACM-2000")
	arco := map[string]any{"padre": "k-root", "figlio": "k-u1", "qta": 2, "origine": "utente"}
	v2 := copiaProfonda(t, sc.base)
	v2["nodi"] = append(v2["nodi"].([]any), nuovo)
	v2["archi"] = append(v2["archi"].([]any), arco)
	azioni := []Azione{
		azione3(t, AzNodoAggiunto, "k-u1", nil, conCampo(t, nuovo, "azione_id", azioneA)),
		azione3(t, AzArcoAggiunto, "k-u1", nil, conCampo(t, arco, "azione_id", azioneA)),
	}
	k1 := uuid.New()
	r, err := sc.salva3(t, 1, k1, v2, azioni)
	if err != nil {
		t.Fatal(err)
	}
	if r.Versione != 2 || !r.NuovaVersione || r.Riproposto || r.Eventi != 2 || r.Formato != FormatoSnapshot3 || r.BozzaID != sc.bozza {
		t.Fatalf("salvataggio: %+v", r)
	}
	var chiave, imp uuid.NullUUID
	var improntaRichiesta pgtype.Text
	if err := sc.p.QueryRow(ctx, `SELECT chiave_salvataggio, importazione_id, impronta_richiesta FROM distinta_bozza_versione
		WHERE bozza_id = $1 AND versione = 2`, sc.bozza).Scan(&chiave, &imp, &improntaRichiesta); err != nil {
		t.Fatal(err)
	}
	if chiave.UUID != k1 || imp.UUID != sc.importazioneID || !reSha256.MatchString(strings.TrimSpace(improntaRichiesta.String)) {
		t.Fatalf("versione 2: chiave %v registro %v impronta %q", chiave, imp, improntaRichiesta.String)
	}
	ee := eventiDB(t, sc.p, sc.bozza, 1)
	if len(ee) != 3 {
		t.Fatalf("eventi: %d", len(ee))
	}
	for i, atteso := range []struct{ azione, chiave, origine string }{
		{AzBozzaAperta, "", "server"}, {AzNodoAggiunto, "k-u1", OrigineUtente}, {AzArcoAggiunto, "k-u1", OrigineUtente},
	} {
		e := ee[i]
		if e.Vocabolario != VocabolarioFormato3 || e.Azione != atteso.azione || e.ChiaveBozza != atteso.chiave || e.Origine != atteso.origine || e.UtenteID != sc.utente {
			t.Errorf("evento %d: %+v", i+1, e)
		}
		if i > 0 && campoJSON(t, e.Dopo, "azione_id") != `"`+azioneA+`"` {
			t.Errorf("evento %d: l'azione_id non e' nel dopo: %s", i+1, e.Dopo)
		}
	}
	// il reinvio: la stessa chiave e lo stesso corpo trovano la versione salvata, senza scrivere
	r2, err := sc.salva3(t, 1, k1, v2, azioni)
	if err != nil || !r2.Riproposto || r2.NuovaVersione || r2.Versione != 2 || r2.Impronta != r.Impronta {
		t.Fatalf("reinvio: %+v %v", r2, err)
	}
	if n := conta3(t, sc.p, `SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1`, sc.bozza); n != 2 {
		t.Fatalf("il reinvio ha scritto: %d versioni", n)
	}
	// la stessa chiave con un altro corpo; una versione superata; niente da salvare
	_, err = sc.salva3(t, 2, k1, v2, nil)
	codice3(t, err, CodiceChiaveRiusata, "")
	_, err = sc.salva3(t, 1, uuid.New(), v2, nil)
	if !errors.Is(err, ErrConflittoVersione) {
		t.Fatalf("versione superata: %v", err)
	}
	r3, err := sc.salva3(t, 2, uuid.New(), v2, nil)
	if err != nil || r3.NuovaVersione || r3.Versione != 2 {
		t.Fatalf("niente da salvare: %+v %v", r3, err)
	}
	// la bozza che non e' della RFQ; il prodotto della richiesta diverso da quello della bozza
	_, err = sc.salva3Su(t, uuid.New(), sc.prodotto, 2, uuid.New(), v2, nil)
	codice3(t, err, CodiceFuoriRFQ, "")
	altro := copiaProfonda(t, v2)
	altro["prodotto_id"] = sc.altroFinito.String()
	_, err = sc.salva3Su(t, sc.bozza, sc.altroFinito, 2, uuid.New(), altro, nil)
	codice3(t, err, CodiceBozzaAltroProdotto, "")
	// la chiave salvata nel formato 3 e' globale: la ritrova anche una richiesta su un'altra RFQ (CHIAVE_RIUSATA)
	raw, _ := json.Marshal(map[string]any{"bozza_id": sc.bozza, "versione_attesa": 2, "prodotto_id": sc.prodottoT2, "vocabolario": 2,
		"chiave_salvataggio": k1, "snapshot": v2, "azioni": []Azione{}})
	rq, err := LeggiRichiestaR2(raw)
	if err != nil {
		t.Fatal(err)
	}
	sv := rq.Salvataggio
	sv.ThreadID, sv.UtenteID = sc.thread2, sc.utente
	_, err = SalvaFormato3(ctx, sc.p, sv)
	codice3(t, err, CodiceChiaveRiusata, "")
}

// PE-05 sul database: annulla e rifai degli eventi del gesto cumulativo, con il prima {azione_id} scritto dal server; la
// pila non attraversa l'apertura del server.
func TestV1SalvaFormato3AnnullaERifaiSulDatabase(t *testing.T) {
	sc := preparaBozza3(t)
	nuovo := nodoManuale3(sc.id, "k-u1", "ACM-2000")
	arco := map[string]any{"padre": "k-root", "figlio": "k-u1", "qta": 2, "origine": "utente"}
	v2 := copiaProfonda(t, sc.base)
	v2["nodi"] = append(v2["nodi"].([]any), nuovo)
	v2["archi"] = append(v2["archi"].([]any), arco)
	if _, err := sc.salva3(t, 1, uuid.New(), v2, []Azione{
		azione3(t, AzNodoAggiunto, "k-u1", nil, conCampo(t, nuovo, "azione_id", azioneA)),
		azione3(t, AzArcoAggiunto, "k-u1", nil, conCampo(t, arco, "azione_id", azioneA)),
	}); err != nil {
		t.Fatal(err)
	}
	r, err := sc.salva3(t, 2, uuid.New(), sc.base, []Azione{
		azione3(t, AzAnnulla, "", map[string]any{"azione_id": azioneA}, map[string]any{"n": 3}),
		azione3(t, AzAnnulla, "", nil, map[string]any{"n": 2}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Versione != 3 || r.Eventi != 2 {
		t.Fatalf("annullamenti: %+v", r)
	}
	for _, e := range eventiDB(t, sc.p, sc.bozza, 4) {
		if e.Azione != AzAnnulla || e.ChiaveBozza != "k-u1" || campoJSON(t, e.Prima, "azione_id") != `"`+azioneA+`"` {
			t.Errorf("annullamento %d: %+v", e.N, e)
		}
	}
	_, err = sc.salva3(t, 3, uuid.New(), sc.base, []Azione{azione3(t, AzAnnulla, "", nil, map[string]any{"n": 1})})
	codice3(t, err, CodiceCorpoNonValido, MotivoAnnullamentoNonAmmesso)
	conNodo := copiaProfonda(t, sc.base)
	conNodo["nodi"] = append(conNodo["nodi"].([]any), nuovo)
	r, err = sc.salva3(t, 3, uuid.New(), conNodo, []Azione{azione3(t, AzRifai, "", map[string]any{"azione_id": azioneA}, map[string]any{"n": 2})})
	if err != nil || r.Versione != 4 {
		t.Fatalf("rifai: %+v %v", r, err)
	}
	l := sc.leggiR1(t, OpzioniR1{Eventi: true})
	if l.Versione != 4 || l.NEventi == nil || *l.NEventi != 6 || len(l.Eventi) != 6 {
		t.Fatalf("R1 dopo la pila: versione %d, n_eventi %v, eventi %d", l.Versione, l.NEventi, len(l.Eventi))
	}
}

// PE-22 sul database e il vassoio di R1: lo svincolo per nodo escluso entra nel vassoio, «Lascia senza associazione» lo
// toglie (una versione nuova senza cambi dello snapshot); un allegato di un'altra RFQ nominato da «Lascia» e' FUORI_RFQ.
// Direttiva 35, C-2: gli eventi dei file e il vassoio sono per allegato_id.
func TestV1SalvaFormato3LoSvincoloEIlVassoio(t *testing.T) {
	sc := preparaBozza3(t)
	v2 := copiaProfonda(t, sc.base)
	v2["documenti_preassociati"] = []any{}
	pdf := sc.id.pdf.String()
	prima := map[string]any{"allegato_id": pdf, "chiave_bozza": "k-0007"}
	if _, err := sc.salva3(t, 1, uuid.New(), v2, []Azione{
		azione3(t, AzFileSvincolato, "k-0007", prima, map[string]any{"allegato_id": pdf, "motivo": SvincoloNodoEscluso}),
	}); err != nil {
		t.Fatal(err)
	}
	l := sc.leggiR1(t, OpzioniR1{})
	if l.Vassoio == nil || len(*l.Vassoio) != 1 || (*l.Vassoio)[0] != (VoceVassoio{AllegatoID: pdf, Motivo: SvincoloNodoEscluso, ChiaveBozza: "k-0007"}) {
		t.Fatalf("vassoio dopo lo svincolo: %+v", l.Vassoio)
	}
	r, err := sc.salva3(t, 2, uuid.New(), v2, []Azione{azione3(t, AzFileLasciato, "", nil, map[string]any{"allegato_id": pdf})})
	if err != nil || r.Versione != 3 || r.Eventi != 1 {
		t.Fatalf("Lascia: %+v %v", r, err)
	}
	if l := sc.leggiR1(t, OpzioniR1{}); l.Vassoio == nil || len(*l.Vassoio) != 0 {
		t.Fatalf("vassoio dopo Lascia: %+v", l.Vassoio)
	}
	_, err = sc.salva3(t, 3, uuid.New(), v2, []Azione{azione3(t, AzFileLasciato, "", nil, map[string]any{"allegato_id": sc.allegatoT2.String()})})
	codice3(t, err, CodiceFuoriRFQ, "")
}

// I riferimenti nuovi o cambiati di un salvataggio contro il database (EM §9.2, §10.8), le identita' (EM §3.4.2, §5.6),
// il segnale visto (PE-17) e lo svincolo che cita l'esclusione (PE-27). Ogni rifiuto lascia la bozza com'era.
func TestV1SalvaFormato3IRiferimentiELeIdentita(t *testing.T) {
	sc := preparaBozza3(t)
	versione := 1
	cur := copiaProfonda(t, sc.base)
	salva := func(snap map[string]any, azioni ...Azione) (Salvato3, error) {
		t.Helper()
		r, err := sc.salva3(t, versione, uuid.New(), snap, azioni)
		if err == nil && r.NuovaVersione {
			versione, cur = r.Versione, snap
		}
		return r, err
	}
	conPre := func(allegato uuid.UUID, chiave string) (map[string]any, Azione) {
		s := copiaProfonda(t, cur)
		x := map[string]any{"allegato_id": allegato.String(), "chiave_bozza": chiave}
		s["documenti_preassociati"] = append(s["documenti_preassociati"].([]any), x)
		return s, azione3(t, AzFilePreassociato, chiave, nil, x)
	}

	// un allegato di un'altra RFQ
	s, a := conPre(sc.allegatoT2, "k-0009")
	_, err := salva(s, a)
	codice3(t, err, CodiceFuoriRFQ, "")

	// il file in Altri file
	pdf := nuovoAllegato(t, sc.p, idDi("msg-0"), 3, "ACM-1100_c.pdf", strings.Repeat("d", 64))
	nuovaProposta(t, sc.p, pdf, sc.thread, nil)
	inAltriFile(t, sc.p, pdf, sc.utente)
	s, a = conPre(pdf, "k-0009")
	_, err = salva(s, a)
	codice3(t, err, CodiceFileNonAttivo, "")

	// l'identita' proposta che non e' quella del registro
	s = copiaProfonda(t, cur)
	idProposta := nodoIn3(cur, "k-0007")["identita"]
	cambiata := copiaProfonda(t, map[string]any{"x": idProposta})["x"].(map[string]any)
	cambiata["codice"] = "ACM-1101"
	nodoIn3(s, "k-0007")["identita"] = cambiata
	_, err = salva(s, azione3(t, AzNodoModificato, "k-0007", map[string]any{"identita": idProposta}, map[string]any{"identita": cambiata}))
	codice3(t, err, CodiceCorpoNonValido, MotivoPropostaNonCorrispondente)

	// un nodo registrato: il codice di oggi e il cliente presunto (la certificazione non c'e')
	registrato := func(codice string) (map[string]any, []Azione) {
		n := map[string]any{"chiave_bozza": "k-s1", "componente_id": idDi("k-s1").String(), "op": "riusa", "tipo": "sottoassieme", "origine": "registrato",
			"identita": map[string]any{"emittente": sc.id.clienteE(), "emittente_presunto": true, "codice": codice, "modo": "registrato"}}
		x := map[string]any{"padre": "k-root", "figlio": "k-s1", "qta": 1, "origine": "utente"}
		s := copiaProfonda(t, cur)
		s["nodi"] = append(s["nodi"].([]any), n)
		s["archi"] = append(s["archi"].([]any), x)
		return s, []Azione{azione3(t, AzNodoAggiunto, "k-s1", nil, n), azione3(t, AzArcoAggiunto, "k-s1", nil, x)}
	}
	s, aa := registrato("ACM-1199")
	_, err = salva(s, aa...)
	codice3(t, err, CodiceCorpoNonValido, MotivoRegistratoNonCorrispondente)
	s, aa = registrato("ACM-1100")
	if _, err := salva(s, aa...); err != nil {
		t.Fatalf("nodo registrato con il codice di oggi: %v", err)
	}

	// la ricodifica verso un emittente che non e' il cliente ne' un partner attivo del registro
	ricodifica := func(emittente string) (map[string]any, Azione) {
		d := map[string]any{"id": "rc-0001", "tipo": "ricodifica", "componente_id": idDi("k-p1-1").String(), "codice_prima": "ACM-1101",
			"codice_dopo": "ACM-1101X", "motivo": "il codice del fornitore", "emittente_dopo": emittente}
		s := copiaProfonda(t, cur)
		s["decisioni"] = append(s["decisioni"].([]any), d)
		return s, azione3(t, AzDecisioneAggiunta, "", nil, d)
	}
	s, a = ricodifica("partner:gamma")
	_, err = salva(s, a)
	codice3(t, err, CodiceCorpoNonValido, MotivoEmittenteNonAmmesso)
	s, a = ricodifica("partner:beta")
	if _, err := salva(s, a); err != nil {
		t.Fatalf("ricodifica verso un partner attivo del registro: %v", err)
	}

	// PE-17: il segnale visto porta la versione del servizio del registro (regole.versioni.ancoraggio)
	segnale := func(versioneServizio string) (map[string]any, []Azione) {
		vecchia := decisioneIn3(cur, "ni-0001")
		nuova := map[string]any{"id": "ni-0003", "tipo": "non_includere",
			"casella":       map[string]any{"allegato_id": sc.id.allegato.String(), "gruppo": gruppoProva3, "rif": sc.id.rif("#31")},
			"motivo_codice": "elemento_cad", "motivo": "cordone: annotazione CAD", "azione_id": azioneB, "importazione": sc.reg.Impronta,
			"segnale_visto": map[string]any{"codice": "ancoraggio.possibile_elemento_cad", "regola": "elemento_cad_nome_foglia_v1",
				"versione_servizio": versioneServizio}}
		s := copiaProfonda(t, cur)
		var dd []any
		for _, d := range s["decisioni"].([]any) {
			if d.(map[string]any)["id"] != "ni-0001" {
				dd = append(dd, d)
			}
		}
		s["decisioni"] = append(dd, nuova)
		// la non_includere non ha chiave_bozza: la chiave degli eventi e' null (RPC PD-08, raccomandata (a))
		return s, []Azione{azione3(t, AzDecisioneTolta, "", vecchia, nil), azione3(t, AzDecisioneAggiunta, "", nil, nuova)}
	}
	s, aa = segnale("ancoraggio-2")
	_, err = salva(s, aa...)
	codice3(t, err, CodiceCorpoNonValido, MotivoSegnaleNonCorrisponde)
	s, aa = segnale("ancoraggio-3")
	if _, err := salva(s, aa...); err != nil {
		t.Fatalf("segnale visto con la versione del registro: %v", err)
	}

	// PE-27: l'esclusione di un componente della RFQ e lo svincolo che la cita, nello stesso salvataggio. Lo svincolo
	// {allegato_id, componente_id} cita una AR attiva (direttiva 35, C-2; EM §3.6.2): qui scritta a mano
	comp := idDi("k-p1-1")
	pdf2 := nuovoAllegato(t, sc.p, idDi("msg-0"), 4, "ACM-1101.pdf", strings.Repeat("f", 64))
	nuovaProposta(t, sc.p, pdf2, sc.thread, &comp)
	arAttivaDiProva(t, sc.p, sc.thread, sc.prodotto, pdf2, comp, sc.utente)
	ex := map[string]any{"id": "ex-0001", "tipo": "escludi", "componente_id": comp.String(), "motivo": "non serve piu'"}
	sv := map[string]any{"id": "sv-0002", "tipo": "scollega_documento", "allegato_id": pdf2.String(), "componente_id": comp.String(),
		"da_decisione": "ex-0001"}
	s = copiaProfonda(t, cur)
	s["decisioni"] = append(s["decisioni"].([]any), ex, sv)
	if _, err := salva(s, azione3(t, AzDecisioneAggiunta, "", nil, ex), azione3(t, AzDecisioneAggiunta, "", nil, sv)); err != nil {
		t.Fatalf("esclusione e svincolo che la cita: %v", err)
	}
	l := sc.leggiR1(t, OpzioniR1{})
	if !strings.Contains(string(l.Snapshot), `"da_decisione":"ex-0001"`) {
		t.Fatal("da_decisione non e' nello snapshot riletto")
	}

	// una casella nuova su un'occorrenza in Altri file (lo STEP della scena): FILE_NON_ATTIVO prima del registro
	inAltriFile(t, sc.p, sc.id.allegato, sc.utente)
	ni := map[string]any{"id": "ni-0009", "tipo": "non_includere",
		"casella":       map[string]any{"allegato_id": sc.id.allegato.String(), "gruppo": gruppoProva3, "rif": sc.id.rif("#99")},
		"motivo_codice": "altro", "motivo": "non serve", "azione_id": azioneB}
	s = copiaProfonda(t, cur)
	s["decisioni"] = append(s["decisioni"].([]any), ni)
	_, err = salva(s, azione3(t, AzDecisioneAggiunta, "", nil, ni))
	codice3(t, err, CodiceFileNonAttivo, "")
	// i riferimenti gia' salvati restano: un salvataggio che non li tocca passa anche con lo STEP in Altri file
	s = copiaProfonda(t, cur)
	nodoIn3(s, "k-man")["descrizione"] = "Vite di prova"
	if _, err := salva(s, azione3(t, AzNodoModificato, "k-man", map[string]any{"descrizione": nil}, map[string]any{"descrizione": "Vite di prova"})); err != nil {
		t.Fatalf("salvataggio che non tocca i riferimenti salvati: %v", err)
	}
	if l := sc.leggiR1(t, OpzioniR1{}); l.RifScaduti == nil || len(*l.RifScaduti) == 0 || (*l.RifScaduti)[0].Motivo != ScadutoFileNonAttivo {
		t.Fatalf("rif scaduti con lo STEP in Altri file: %+v", l.RifScaduti)
	}
}

// ------------------------------------------------------------------ R1

func TestV1LeggiR1Formato3(t *testing.T) {
	sc := preparaBozza3(t)
	l := sc.leggiR1(t, OpzioniR1{Eventi: true, Importazione: true})
	if l.Stato != StatoAperta || l.Formato != FormatoSnapshot3 || l.Vocabolario != VocabolarioFormato3 || l.Versione != 1 || l.BozzaID == nil || *l.BozzaID != sc.bozza {
		t.Fatalf("testata: %+v", l)
	}
	if l.NEventi == nil || *l.NEventi != 1 || len(l.Eventi) != 1 || l.Eventi[0].Azione != AzBozzaAperta || l.Eventi[0].Autore != sc.utente {
		t.Fatalf("eventi: %v %+v", l.NEventi, l.Eventi)
	}
	if l.ChiaveUltimoSalvataggio == nil || *l.ChiaveUltimoSalvataggio != sc.chiave0 {
		t.Fatalf("chiave dell'ultimo salvataggio: %v", l.ChiaveUltimoSalvataggio)
	}
	if l.Importazione == nil || len(l.ImportazioniCitate) != 1 || l.ImportazioniCitate[0].Impronta != sc.reg.Impronta {
		t.Fatalf("registro: %+v %+v", l.Importazione, l.ImportazioniCitate)
	}
	b, _ := json.Marshal(l.Importazione)
	if !strings.Contains(string(b), `"contenuto"`) {
		t.Fatalf("?importazione=1 senza il contenuto: %s", b)
	}
	if l.RifScaduti == nil || len(*l.RifScaduti) != 0 || l.Vassoio == nil || len(*l.Vassoio) != 0 || l.Base == nil || l.VersioneCorrente == nil {
		t.Fatalf("stato: rif %v vassoio %v base %v", l.RifScaduti, l.Vassoio, l.Base)
	}
	out, _ := json.Marshal(l)
	for _, campo := range []string{`"rif_scaduti":[]`, `"vassoio":[]`, `"n_eventi":1`, `"vocabolario":2`, `"formato":3`} {
		if !strings.Contains(string(out), campo) {
			t.Errorf("la risposta non ha %s", campo)
		}
	}
	// senza ?eventi=1: n_eventi si', eventi no
	if l := sc.leggiR1(t, OpzioniR1{}); l.NEventi == nil || *l.NEventi != 1 || l.Eventi != nil {
		t.Fatalf("senza eventi: %v %v", l.NEventi, l.Eventi)
	}
	// ?registro: quello della bozza si legge intero; un altro e' FUORI_RFQ
	if l := sc.leggiR1(t, OpzioniR1{Registro: sc.reg.Impronta}); l.Registro == nil {
		t.Fatal("?registro della bozza senza registro")
	}
	_, err := leggiR1DB(t, sc.p, sc.thread, sc.prodotto, OpzioniR1{Registro: strings.Repeat("e", 64)})
	codice3(t, err, CodiceFuoriRFQ, "")
	// i rif scaduti: una chiave tolta dalla struttura dei fatti, poi i fatti assenti
	esegui3(t, sc.p, `UPDATE analisi_fatti SET fatti = $2 WHERE sha256 = $1`, sc.id.sha, fattiStep3(t, "#1", "#7", "#11", "#31"))
	l = sc.leggiR1(t, OpzioniR1{})
	if l.RifScaduti == nil || len(*l.RifScaduti) != 1 || (*l.RifScaduti)[0].ChiaveBozza != "k-0009" || (*l.RifScaduti)[0].Motivo != ScadutoChiaveAssente {
		t.Fatalf("chiave tolta: %+v", l.RifScaduti)
	}
	if l.Base.Stato != BaseSuperata || !strings.Contains(strings.Join(l.Base.Motivi, ","), MotivoRifScaduti) {
		t.Fatalf("base con un rif scaduto: %+v", l.Base)
	}
	esegui3(t, sc.p, `DELETE FROM analisi_fatti WHERE sha256 = $1`, sc.id.sha)
	l = sc.leggiR1(t, OpzioniR1{})
	if l.RifScaduti == nil || len(*l.RifScaduti) != 5 {
		t.Fatalf("fatti assenti: %+v", l.RifScaduti)
	}
	for _, r := range *l.RifScaduti {
		if r.Motivo != ScadutoFattiAssenti {
			t.Errorf("fatti assenti: %+v", r)
		}
	}
	// il perimetro: un prodotto senza bozza, il prodotto di un'altra RFQ, una RFQ che non c'e'
	if l, err := leggiR1DB(t, sc.p, sc.thread, sc.altroFinito, OpzioniR1{}); err != nil || l.Stato != StatoNessunaBozza || l.VersioneCorrente == nil ||
		l.RifScaduti != nil || l.Vassoio != nil {
		t.Fatalf("nessuna bozza: %+v %v", l, err)
	}
	_, err = leggiR1DB(t, sc.p, sc.thread, sc.prodottoT2, OpzioniR1{})
	codice3(t, err, CodiceFuoriRFQ, "")
	_, err = leggiR1DB(t, sc.p, uuid.New(), sc.prodotto, OpzioniR1{})
	codice3(t, err, CodiceRFQInesistente, "")
}

// ------------------------------------------------------------------ R6

func TestV1LeggiIntentoR6(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	k := uuid.New()
	if e, err := LeggiIntento(ctx, p, s.thread, k); err != nil || e.Stato != IntentoNonRegistrato {
		t.Fatalf("non registrato: %+v %v", e, err)
	}
	esegui3(t, p, `INSERT INTO esito_intento (chiave, thread_id, gesto, utente_id, impronta_corpo, esito)
		VALUES ($1, $2, 'fonte_step', $3, repeat('a', 64), '{"esito":"ok","azione":"scelta"}')`, k, s.thread, s.utente)
	esegui3(t, p, `INSERT INTO f8_stato (thread_id, chiave_intento) VALUES ($1, $2)`, s.thread, k)
	e, err := LeggiIntento(ctx, p, s.thread, k)
	if err != nil || e.Stato != IntentoRegistrato || e.Gesto != "fonte_step" || e.F8Stato != "in_attesa" || !strings.Contains(string(e.Esito), `"scelta"`) {
		t.Fatalf("registrato: %+v %v", e, err)
	}
	k2 := uuid.New()
	esegui3(t, p, `INSERT INTO esito_intento (chiave, thread_id, gesto, utente_id, impronta_corpo, esito)
		VALUES ($1, $2, 'fonte_step', $3, repeat('a', 64), '{"esito":"ok"}')`, k2, s.thread2, s.utente)
	_, err = LeggiIntento(ctx, p, s.thread, k2)
	codice3(t, err, CodiceFuoriRFQ, "")
	_, err = LeggiIntento(ctx, p, uuid.New(), k)
	codice3(t, err, CodiceRFQInesistente, "")
	// in_corso: la riga della RFQ tenuta FOR UPDATE (una R5 o un gesto dei file in corso)
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `SELECT 1 FROM thread_offerta WHERE thread_id = $1 FOR UPDATE`, s.thread); err != nil {
		t.Fatal(err)
	}
	inizio := time.Now()
	e, err = LeggiIntento(ctx, p, s.thread, k)
	durata := time.Since(inizio)
	_ = tx.Rollback(ctx)
	if err != nil || e.Stato != IntentoInCorso || durata < 2*time.Second {
		t.Fatalf("in corso: %+v %v dopo %v", e, err, durata)
	}
	// R6 non scrive
	if n := conta3(t, p, `SELECT count(*) FROM esito_intento`) + conta3(t, p, `SELECT count(*) FROM f8_stato`); n != 3 {
		t.Fatalf("righe dopo R6: %d", n)
	}
}

// ------------------------------------------------------------------ R7 (U-06, PE-07)

func TestV1StoricoR7ITreFormati(t *testing.T) {
	sc := preparaBozza3(t)
	ctx := context.Background()
	f1, err := jsoncanonico.Codifica(map[string]any{"formato": 1, "perimetro": map[string]any{"radice": "ACM-1000", "componenti": []any{}}})
	if err != nil {
		t.Fatal(err)
	}
	s2, err := LeggiSnapshot(mustJSON(soloRadice(sc.prodotto, "ACM-1000")))
	if err != nil {
		t.Fatal(err)
	}
	f2, err := s2.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	s3 := leggiProva3(t, sc.base)
	f3, err := s3.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	f3 = conProvenienzaPB(t, f3)
	c1, c2, c3, c4 := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	tx, err := sc.p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, x := range []struct {
		sql  string
		args []any
	}{
		// le conferme si scrivono a mano, con i trigger della tabella spenti nella transazione: la prova e' sul lettore
		{`ALTER TABLE conferma_strutturale DISABLE TRIGGER USER`, nil},
		{`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura,
				base_contatore, contatore, contatore_dati, origine, snapshot, impronta_snapshot, delta, mappa, criterio_verifica, confermato_da)
			VALUES ($1, $2, $3, 1, 1, true, repeat('3', 64), 0, 0, 0, '{"rotta":"distinta/albero/conferma"}', $4, $5, '{}', '{}', 'nessuno', $6)`,
			[]any{c1, sc.thread, sc.prodotto, f1, jsoncanonico.Impronta(f1), sc.utente}},
		{`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2, firma_struttura,
				firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati, origine, snapshot, impronta_snapshot, delta,
				mappa, criterio_verifica, verifica, chiave_idempotenza, confermato_da)
			VALUES ($1, $2, $3, 2, $4, 2, false, repeat('3', 64), repeat('2', 64), $4, 0, repeat('1', 64), 0, 0, '{"rotta":"prova"}', $5, $6,
				'{}', '{}', 'nessuno', '{"esito":"verificata"}', gen_random_uuid(), $7)`,
			[]any{c2, sc.thread, sc.prodotto, c1, f2, jsoncanonico.Impronta(f2), sc.utente}},
		{`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2, firma_struttura,
				firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati, bozza_id, bozza_versione, fonte_allegato_id,
				fonte_sha256, fonte_gruppo, origine, snapshot, impronta_snapshot, delta, mappa, criterio_verifica, chiave_idempotenza, confermato_da,
				importazione_id, importazioni_citate, verifica)
			VALUES ($1, $2, $3, 3, $4, 3, false, repeat('3', 64), repeat('2', 64), $4, 12, repeat('1', 64), 0, 0, $5, 1, $6, $7, 'bom',
				'{"rotta":"prova"}', $8, $9, '{}', '{}', 'nessuno', gen_random_uuid(), $10, $11, ARRAY[$11]::uuid[], '{"regola": "prima-istanza-1"}')`,
			[]any{c3, sc.thread, sc.prodotto, c2, sc.bozza, sc.id.allegato, sc.id.sha, f3, jsoncanonico.Impronta(f3), sc.utente, sc.importazioneID}},
		// una conferma con uno snapshot che il lettore del formato 2 non sa leggere (altro prodotto)
		{`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura,
				firma_anteprima, base_contatore, firma_base, contatore, contatore_dati, origine, snapshot, impronta_snapshot, delta, mappa,
				criterio_verifica, chiave_idempotenza, confermato_da)
			VALUES ($1, $2, $3, 1, 2, false, repeat('3', 64), repeat('2', 64), 0, repeat('1', 64), 0, 0, '{"rotta":"prova"}',
				$4, repeat('9', 64), '{}', '{}', 'nessuno', gen_random_uuid(), $5)`,
			[]any{c4, sc.thread, sc.altroFinito, mustJSON(map[string]any{"formato": 2, "prodotto_id": sc.altroFinito.String(), "nodi": "non una lista"}), sc.utente}},
		{`ALTER TABLE conferma_strutturale ENABLE TRIGGER USER`, nil},
	} {
		if _, err := tx.Exec(ctx, x.sql, x.args...); err != nil {
			t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	leggi := func(prodotto uuid.UUID, op OpzioniR7) (StoricoR7, error) {
		tx, err := sc.p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		return LeggiStorico(ctx, db.New(tx), sc.thread, prodotto, op)
	}
	st, err := leggi(sc.prodotto, OpzioniR7{})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Conferme) != 3 {
		t.Fatalf("conferme: %+v", st.Conferme)
	}
	// U2 §5.4 (R-17 riscritta): verifica_calcolata non c'e' piu'; la conferma formato 3 porta i prerequisiti registrati
	// (criterio, regola prima-istanza-1, firma; gerarchia e deroga dichiarate mancanti se origine non le porta), le
	// formato 1 e 2 no; nessun si' o no
	for i, atteso := range []struct {
		formato      int
		prerequisiti bool
	}{{1, false}, {2, false}, {3, true}} {
		c := st.Conferme[i]
		if c.Formato != atteso.formato || (c.Prerequisiti != nil) != atteso.prerequisiti || c.ConfermatoDaNome != "Prova bozza" || c.ConfermatoDaSigla != "DBZ" {
			t.Errorf("conferma %d: %+v", i+1, c)
		}
	}
	if p := st.Conferme[2].Prerequisiti; p == nil || p.Regola != confermabom.RegolaPrimaIstanza || p.FirmaStruttura == "" || len(p.Mancanti) != 2 {
		t.Errorf("prerequisiti della conferma formato 3: %+v", p)
	}
	if b, _ := json.Marshal(st.Conferme[2]); strings.Contains(string(b), "verifica_calcolata") || strings.Contains(string(b), `"verificata"`) {
		t.Errorf("R7 porta un esito della verifica: %s", b)
	}
	if !st.Conferme[2].Ultima || st.Conferme[0].Ultima || st.Conferme[2].ImportazioneID == nil || *st.Conferme[2].ImportazioneID != sc.importazioneID {
		t.Errorf("ultima e registro: %+v", st.Conferme[2])
	}
	if st.Attestazioni == nil || st.Fonte == nil {
		t.Error("attestazioni e fonte sono elenchi, anche vuoti")
	}
	for _, c := range []struct {
		id      uuid.UUID
		formato int
	}{{c1, 1}, {c2, 2}, {c3, 3}} {
		id := c.id
		d, err := leggi(sc.prodotto, OpzioniR7{Conferma: &id, Importazione: c.formato == 3})
		if err != nil {
			t.Fatalf("dettaglio %d: %v", c.formato, err)
		}
		if d.Dettaglio == nil || d.Dettaglio.ContenutoNonLeggibile || !strings.Contains(string(d.Dettaglio.Snapshot), `"formato":`) {
			t.Fatalf("dettaglio formato %d: %+v", c.formato, d.Dettaglio)
		}
		if c.formato == 3 && (len(d.Dettaglio.Registri) != 1 || d.Dettaglio.Registri[0].Impronta != sc.reg.Impronta) {
			t.Fatalf("registri della conferma formato 3: %+v", d.Dettaglio.Registri)
		}
	}
	d, err := leggi(sc.altroFinito, OpzioniR7{Conferma: &c4})
	if err != nil || d.Dettaglio == nil || !d.Dettaglio.ContenutoNonLeggibile || d.Dettaglio.MotivoNonLeggibile == "" || len(d.Dettaglio.Snapshot) == 0 {
		t.Fatalf("contenuto non leggibile: %+v %v", d.Dettaglio, err)
	}
	// i rifiuti: la conferma di un altro prodotto, ?importazione=1 senza conferma, il prodotto di un'altra RFQ
	_, err = leggi(sc.prodotto, OpzioniR7{Conferma: &c4})
	codice3(t, err, CodiceFuoriRFQ, "")
	_, err = leggi(sc.prodotto, OpzioniR7{Importazione: true})
	codice3(t, err, CodiceCorpoNonValido, "")
	_, err = leggi(sc.prodottoT2, OpzioniR7{})
	codice3(t, err, CodiceFuoriRFQ, "")
}

// ------------------------------------------------------------------ la conversione (U-05, U-05b in parte)

// bozza2DB scrive una bozza formato 2 aperta con il suo snapshot alla versione 1 (come l'avrebbe lasciata la pagina
// vecchia), con l'evento di apertura del vocabolario 1.
func bozza2DB(t *testing.T, p *pgxpool.Pool, thread, prodotto, utente uuid.UUID, s Snapshot) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	letto, err := LeggiSnapshot(mustJSON(s))
	if err != nil {
		t.Fatalf("snapshot formato 2: %v", err)
	}
	can, err := letto.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	all, sha, gruppo := colonneFonte(letto.Sorgente)
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)
	b, err := q.InsertDistintaBozza(ctx, db.InsertDistintaBozzaParams{ThreadID: thread, ProdottoID: prodotto, Formato: FormatoSnapshot,
		FirmaBase: firmaProva, FonteAllegatoID: all, FonteSha256: sha, FonteGruppo: gruppo, UtenteID: utente})
	if err != nil {
		t.Fatalf("bozza formato 2: %v", err)
	}
	if _, err := q.InsertDistintaBozzaVersione(ctx, db.InsertDistintaBozzaVersioneParams{BozzaID: b.BozzaID, Versione: 1, Snapshot: can,
		Impronta: jsoncanonico.Impronta(can), FirmaBase: firmaProva, FonteAllegatoID: all, FonteSha256: sha, FonteGruppo: gruppo, SalvataDa: utente}); err != nil {
		t.Fatalf("versione 1 formato 2: %v", err)
	}
	if err := q.InsertDistintaBozzaEvento(ctx, db.InsertDistintaBozzaEventoParams{BozzaID: b.BozzaID, N: 1, Versione: 1, Vocabolario: int16(Vocabolario),
		Azione: AzBozzaAperta, Origine: "utente", Dopo: rawONull(mustJSON(map[string]string{"partenza": PartenzaVuota})), UtenteID: utente}); err != nil {
		t.Fatalf("apertura formato 2: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("bozza formato 2: %v", err)
	}
	return b.BozzaID
}

func TestV1ConversioneFormato2(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	shaB := strings.Repeat("b", 64)
	riga := uuid.MustParse("00000000-0000-4000-8000-0000000c0007")
	rigaM := uuid.MustParse("00000000-0000-4000-8000-0000000c0008")
	rigaS := uuid.MustParse("00000000-0000-4000-8000-0000000c0009")
	// due preassociazioni formato 2 {proposta_id}: su un PDF (si traduce) e sullo STEP allegato2 (si traduce e si segnala);
	// direttiva 35, C-2. Direttiva 39 §5, D-2 (c) [U]: uno STEP si segnala solo se e' legato come sorgente del finito;
	// qui allegato2 lo e' come documento STEP storico del prodotto (confermato, senza conferme di S8)
	esegui3(t, p, `INSERT INTO documento (documento_id, thread_id, componente_id, tipo, codice, rev, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES (gen_random_uuid(), $1, $2, 'cad_3d', 'ACM-1000', 'A', 'ACM-1000_bis.stp', 'stp', repeat('c', 64), 'ACME/ACM-1000_bis.stp', $3)`,
		s.thread, s.prodotto, s.utente)
	pdfConv := nuovoAllegato(t, p, idDi("msg-0"), 9, "ACM-1100.pdf", strings.Repeat("a", 64))
	proposta := nuovaProposta(t, p, pdfConv, s.thread, nil)
	propostaStep := nuovaProposta(t, p, s.allegato2, s.thread, nil)
	ricodifica := json.RawMessage(`{"id":"rc-1","tipo":"ricodifica","componente_id":"` + idDi("k-p1-1").String() +
		`","codice_prima":"ACM-1101","codice_dopo":"ACM-1101X","motivo":"il codice del fornitore"}`)
	vecchio := Snapshot{Formato: FormatoSnapshot, ProdottoID: s.prodotto, VersioneBomBase: &VersioneBomBase{}, FirmaBase: firmaProva,
		Sorgente: &Sorgente{AllegatoID: pu(s.allegato), Sha256: shaB, Gruppo: "bom"},
		Nodi: []Nodo{
			{ChiaveBozza: "k-root", ComponenteID: pu(s.prodotto), Op: OpRiusa, Codice: "ACM-1000", Rev: "A", Tipo: "finito", Origine: "registrato"},
			{ChiaveBozza: "k-7", Op: OpCrea, Codice: "ACM-1100", Rev: "A", Tipo: "sottoassieme", Origine: "step",
				Caselle: []Casella{{Gruppo: "bom", Rif: "nodo:" + shaB + ":#7"}}, RigheProposta: []uuid.UUID{riga}},
			{ChiaveBozza: "k-9", Op: OpCrea, Codice: "ACM-1109", Tipo: "sciolto", Origine: "step",
				Caselle: []Casella{{Gruppo: "bom", Rif: "nodo:" + shaB + ":#9"}}},
			// un nodo dell'ingegnere con righe, e un nodo con origine step senza caselle (il formato 2 lo ammette)
			{ChiaveBozza: "k-m", Op: OpCrea, Codice: "ACM-1190", Tipo: "sciolto", Origine: "utente", RigheProposta: []uuid.UUID{rigaM}},
			{ChiaveBozza: "k-s", Op: OpCrea, Codice: "ACM-1191", Tipo: "sciolto", Origine: "step", RigheProposta: []uuid.UUID{rigaS}},
		},
		Archi: []Arco{{Padre: "k-root", Figlio: "k-7", Qta: 1, Origine: "step"}, {Padre: "k-7", Figlio: "k-9", Qta: 2, Origine: "step"},
			{Padre: "k-root", Figlio: "k-m", Qta: 1, Origine: "utente", Posizione: "P1"}, {Padre: "k-7", Figlio: "k-s", Qta: 1, Origine: "step"}},
		Decisioni:             []json.RawMessage{ricodifica, json.RawMessage(`{"id":"mp-1","tipo":"mono_pezzo","motivo":"pezzo unico"}`)},
		DocumentiPreassociati: []Preassociazione2{{PropostaID: proposta, ChiaveBozza: "k-7"}, {PropostaID: propostaStep, ChiaveBozza: "k-9"}},
	}
	b2 := bozza2DB(t, p, s.thread, s.prodotto, s.utente, vecchio)
	var primaDB string
	if err := p.QueryRow(ctx, `SELECT snapshot::text FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = 1`, b2).Scan(&primaDB); err != nil {
		t.Fatal(err)
	}
	// la fonte secondaria con la destinazione temporanea sulla bozza formato 2
	q := db.New(p)
	if _, err := q.InsertFonteStepEvento(ctx, db.InsertFonteStepEventoParams{ThreadID: s.thread, ProdottoID: s.prodotto, Ruolo: "sottoassieme",
		ChiaveBozza: testo("k-7"), Azione: "scelta", AllegatoID: s.allegato, Sha256: shaB, Gruppo: testo("bom"),
		BozzaID: uuid.NullUUID{UUID: b2, Valid: true}, AutoreID: s.utente}); err != nil {
		t.Fatalf("fonte secondaria: %v", err)
	}
	// una bozza formato 2 aperta non si salva nel formato 3: formato_da_convertire
	sc := scena3DB{scenaDB: s, p: p}
	snap3 := map[string]any{"formato": 3, "prodotto_id": s.prodotto.String(), "nodi": []any{}, "archi": []any{}, "decisioni": []any{}}
	_, err := sc.salva3Su(t, b2, s.prodotto, 1, uuid.New(), snap3, nil)
	codice3(t, err, CodiceCorpoNonValido, MotivoFormatoDaConvertire)
	// il motivo non e' piu' obbligatorio (messaggio 46 [U]); qui quello di un client di prima, che resta intero
	chiave := uuid.New()
	motivo := strings.Repeat("m", MaxMotivo)
	c, err := Converti(ctx, p, Conversione3{ThreadID: s.thread, ProdottoID: s.prodotto, UtenteID: s.utente, BozzaID: b2, VersioneAttesa: 1,
		Motivo: motivo, ChiaveSalvataggio: chiave, ImprontaRichiesta: strings.Repeat("5", 64)})
	if err != nil {
		t.Fatalf("conversione: %v", err)
	}
	// la risposta elenca i soli nodi senza casella che avevano righe (EM §6.7): k-m e k-s, non k-7
	if c.Versione != 1 || c.Formato != FormatoSnapshot3 || !c.NuovaVersione || len(c.RigheCadute) != 2 || c.RigheCadute[0].ChiaveBozza != "k-m" ||
		c.RigheCadute[0].Righe[0] != rigaM || c.RigheCadute[1].ChiaveBozza != "k-s" || c.RigheCadute[1].Righe[0] != rigaS {
		t.Fatalf("esito: %+v", c)
	}
	if len(c.NodiSenzaCasella) != 1 || c.NodiSenzaCasella[0] != "k-s" || len(c.DecisioniCompletate) != 1 ||
		c.DecisioniCompletate[0] != (DecisioneCompletata{ID: "rc-1", Campo: "emittente_dopo"}) || len(c.FontiNonRiportate) != 0 {
		t.Fatalf("tracce: %+v", c)
	}
	// direttiva 35, C-2: due preassociazioni tradotte, quella sullo STEP segnalata
	if c.PreassociazioniTradotte != 2 || len(c.PreassociazioniSegnalate) != 1 || c.PreassociazioniSegnalate[0] != (PreassociazioneSegnalata{
		PropostaID: propostaStep, AllegatoID: s.allegato2, ChiaveBozza: "k-9", Motivo: PreassociazioneLegatoComeSorgente}) {
		t.Fatalf("preassociazioni tradotte e segnalate: %d %+v", c.PreassociazioniTradotte, c.PreassociazioniSegnalate)
	}
	// la formato 2: abbandonata, con il solo prefisso nel motivo di chiusura; lo snapshot identico
	var stato, motivoChiusura, dopoDB string
	if err := p.QueryRow(ctx, `SELECT stato, motivo_chiusura FROM distinta_bozza WHERE bozza_id = $1`, b2).Scan(&stato, &motivoChiusura); err != nil {
		t.Fatal(err)
	}
	if stato != StatoAbbandonata || motivoChiusura != "convertita nel formato 3 nella bozza "+c.BozzaID.String() {
		t.Fatalf("bozza formato 2: %s %q", stato, motivoChiusura)
	}
	if err := p.QueryRow(ctx, `SELECT snapshot::text FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = 1`, b2).Scan(&dopoDB); err != nil || dopoDB != primaDB {
		t.Fatalf("lo snapshot formato 2 e' cambiato: %v", err)
	}
	// la formato 3: versione 1 con modo formato_2, il cliente presunto, la casella completata, l'arco con lo step
	n3, err := rileggiSnapshot3V(t, p, c.BozzaID, 1)
	if err != nil {
		t.Fatal(err)
	}
	cliente := EmittenteCliente(idDi("cliente"))
	for _, n := range n3.Nodi {
		if n.Identita.Modo != ModoFormato2 || n.Identita.Emittente != cliente || !n.Identita.EmittentePresunto {
			t.Errorf("nodo %s: identita' %+v", n.ChiaveBozza, n.Identita)
		}
		if n.ChiaveBozza == "k-7" && (len(n.Caselle) != 1 || n.Caselle[0].AllegatoID != s.allegato) {
			t.Errorf("caselle di k-7: %+v", n.Caselle)
		}
		if n.ChiaveBozza == "k-s" && (n.Origine != OrigineUtente || len(n.Caselle) != 0) {
			t.Errorf("il nodo step senza caselle: %+v", n)
		}
	}
	// decisioni e preassociazioni copiate; la ricodifica con l'emittente del cliente della RFQ
	if len(n3.Decisioni) != 2 || n3.Decisioni[1].ID != "rc-1" || n3.Decisioni[1].EmittenteDopo != cliente || n3.Decisioni[1].CodiceDopo != "ACM-1101X" ||
		n3.Decisioni[0].ID != "mp-1" || n3.Decisioni[0].Motivo != "pezzo unico" {
		t.Errorf("decisioni: %+v", n3.Decisioni)
	}
	// le preassociazioni tradotte 1:1 da proposta_id ad allegato_id, nell'ordine canonico per allegato (direttiva 35, C-2)
	// con la provenienza manuale (direttiva 39 §4 [T-39]: la PB del formato 2 l'ha messa l'ingegnere, senza un registro)
	attese := []Preassociazione3{{AllegatoID: pdfConv, ChiaveBozza: "k-7", Provenienza: ProvenienzaManuale},
		{AllegatoID: s.allegato2, ChiaveBozza: "k-9", Provenienza: ProvenienzaManuale}}
	sort.Slice(attese, func(i, j int) bool { return attese[i].AllegatoID.String() < attese[j].AllegatoID.String() })
	if len(n3.DocumentiPreassociati) != 2 || n3.DocumentiPreassociati[0] != attese[0] || n3.DocumentiPreassociati[1] != attese[1] {
		t.Errorf("preassociazioni: %+v", n3.DocumentiPreassociati)
	}
	if strings.Contains(string(c.Snapshot), "proposta_id") {
		t.Errorf("lo snapshot formato 3 della versione 1 porta ancora proposta_id: %s", c.Snapshot)
	}
	// l'arco fra due caselle resta dello STEP; quello dalla radice (senza caselle nel formato 2) resta dell'ingegnere
	for _, a := range n3.Archi {
		switch a.Figlio {
		case "k-9":
			if a.Origine != OrigineStep || a.Step == nil || a.Step.AllegatoID != s.allegato || a.Qta != 2 {
				t.Errorf("arco fra due caselle: %+v", a)
			}
		case "k-7":
			if a.Origine != OrigineUtente || a.Step != nil || a.Qta != 1 {
				t.Errorf("arco dalla radice: %+v", a)
			}
		}
	}
	// nell'ordine canonico degli archi (padre, figlio): «k-7» viene prima di «k-root»
	if len(c.ArchiSenzaStep) != 2 || c.ArchiSenzaStep[0] != (ArcoSenzaStep{Padre: "k-7", Figlio: "k-s"}) ||
		c.ArchiSenzaStep[1] != (ArcoSenzaStep{Padre: "k-root", Figlio: "k-7"}) {
		t.Errorf("archi senza step: %+v", c.ArchiSenzaStep)
	}
	// l'evento di apertura: origine server, il motivo intero, la partenza, l'algoritmo e le tracce (tutte le righe cadute)
	ee := eventiDB(t, p, c.BozzaID, 1)
	daBozza := ""
	if len(ee) == 1 {
		daBozza = campoJSON(t, ee[0].Dopo, "da_bozza")
	}
	if len(ee) != 1 || ee[0].Azione != AzBozzaAperta || ee[0].Origine != "server" || ee[0].Motivo != motivo ||
		campoJSON(t, ee[0].Dopo, "partenza") != `"formato_2"` || campoJSON(t, ee[0].Dopo, "algoritmo") != `"conversione-1"` ||
		!strings.Contains(daBozza, riga.String()) || !strings.Contains(daBozza, rigaM.String()) || !strings.Contains(daBozza, rigaS.String()) ||
		!strings.Contains(daBozza, `"archi_senza_step":[{"figlio":"k-s","padre":"k-7"},{"figlio":"k-7","padre":"k-root"}]`) ||
		!strings.Contains(daBozza, `"nodi_senza_casella":["k-s"]`) || !strings.Contains(daBozza, `"decisioni_completate":[{"campo":"emittente_dopo","id":"rc-1"}]`) ||
		!strings.Contains(daBozza, `"fonti_non_riportate":[]`) || !strings.Contains(daBozza, `"preassociazioni_tradotte":2`) ||
		!strings.Contains(daBozza, `"preassociazioni_segnalate":[{"allegato_id":"`+s.allegato2.String()+`","chiave_bozza":"k-9","motivo":"legato_come_sorgente","proposta_id":"`+propostaStep.String()+`"}]`) {
		t.Fatalf("evento di apertura: %+v\n%s", ee, daBozza)
	}
	// la fonte secondaria rifatta sulla bozza nuova, con la stessa chiave
	if n := conta3(t, p, `SELECT count(*) FROM fonte_step_evento WHERE bozza_id = $1 AND chiave_bozza = 'k-7' AND azione = 'scelta' AND ruolo = 'sottoassieme'`,
		c.BozzaID); n != 1 {
		t.Fatalf("fonte secondaria sulla bozza nuova: %d", n)
	}
	// il reinvio: la stessa chiave e la stessa impronta
	c2, err := Converti(ctx, p, Conversione3{ThreadID: s.thread, ProdottoID: s.prodotto, UtenteID: s.utente, BozzaID: b2, VersioneAttesa: 1,
		Motivo: motivo, ChiaveSalvataggio: chiave, ImprontaRichiesta: strings.Repeat("5", 64)})
	if err != nil || !c2.Riproposto || c2.BozzaID != c.BozzaID || len(c2.RigheCadute) != 2 || len(c2.ArchiSenzaStep) != 2 ||
		len(c2.NodiSenzaCasella) != 1 || len(c2.DecisioniCompletate) != 1 || c2.PreassociazioniTradotte != 2 || len(c2.PreassociazioniSegnalate) != 1 {
		t.Fatalf("reinvio della conversione: %+v %v", c2, err)
	}
	// U-05b (la parte senza R4): un nodo formato_2 tiene codice e rev della versione 1
	sc.bozza = c.BozzaID
	base := map[string]any{}
	if err := json.Unmarshal(mustJSON(n3), &base); err != nil {
		t.Fatal(err)
	}
	idPrima := nodoIn3(base, "k-7")["identita"]
	idDopo := copiaProfonda(t, map[string]any{"x": idPrima})["x"].(map[string]any)
	idDopo["codice"] = "ACM-1199"
	s1 := copiaProfonda(t, base)
	nodoIn3(s1, "k-7")["identita"] = idDopo
	_, err = sc.salva3(t, 1, uuid.New(), s1, []Azione{azione3(t, AzNodoModificato, "k-7", map[string]any{"identita": idPrima}, map[string]any{"identita": idDopo})})
	codice3(t, err, CodiceCorpoNonValido, MotivoFormato2NonCorrispondente)
	s2 := copiaProfonda(t, base)
	nodoIn3(s2, "k-7")["descrizione"] = "Telaio convertito"
	if _, err := sc.salva3(t, 1, uuid.New(), s2, []Azione{azione3(t, AzNodoModificato, "k-7", map[string]any{"descrizione": nil},
		map[string]any{"descrizione": "Telaio convertito"})}); err != nil {
		t.Fatalf("nodo formato_2 con codice e rev della versione 1: %v", err)
	}
}

// conversione_ambigua e conversione_piu_gruppi; un'occorrenza non attiva non conta (EM §6.7, U-05).
func TestV1ConversioneICasiCheNonSiConvertono(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	shaC := strings.Repeat("c", 64)
	conv := func(b uuid.UUID, prodotto uuid.UUID) error {
		_, err := Converti(ctx, p, Conversione3{ThreadID: s.thread, ProdottoID: prodotto, UtenteID: s.utente, BozzaID: b, VersioneAttesa: 1,
			Motivo: "si passa al formato 3", ChiaveSalvataggio: uuid.New(), ImprontaRichiesta: strings.Repeat("6", 64)})
		return err
	}
	// la sorgente senza allegato e due occorrenze attive con lo sha del rif
	ambigua := Snapshot{Formato: FormatoSnapshot, ProdottoID: s.altroFinito, VersioneBomBase: &VersioneBomBase{}, FirmaBase: firmaProva,
		Sorgente: &Sorgente{Gruppo: "bom"},
		Nodi: []Nodo{
			{ChiaveBozza: "r", ComponenteID: pu(s.altroFinito), Op: OpRiusa, Codice: "ACM-6000", Tipo: "finito", Origine: "registrato"},
			{ChiaveBozza: "k-2", Op: OpCrea, Codice: "ACM-6010", Tipo: "sciolto", Origine: "step", Caselle: []Casella{{Gruppo: "bom", Rif: "nodo:" + shaC + ":#2"}}},
		},
		Archi: []Arco{{Padre: "r", Figlio: "k-2", Qta: 1, Origine: "step"}},
	}
	b := bozza2DB(t, p, s.thread, s.altroFinito, s.utente, ambigua)
	doppio := nuovoAllegato(t, p, idDi("msg-0"), 5, "ACM-1000_ter.stp", shaC)
	codice3(t, conv(b, s.altroFinito), CodiceCorpoNonValido, MotivoConversioneAmbigua)
	if n := conta3(t, p, `SELECT count(*) FROM distinta_bozza WHERE bozza_id = $1 AND stato = 'aperta'`, b); n != 1 {
		t.Fatal("la conversione rifiutata ha chiuso la bozza formato 2")
	}
	// la seconda occorrenza in Altri file non conta
	inAltriFile(t, p, doppio, s.utente)
	if err := conv(b, s.altroFinito); err != nil {
		t.Fatalf("con una sola occorrenza attiva: %v", err)
	}
	// caselle di un gruppo diverso da quello della sorgente
	fin := idDi("finito-gruppi")
	esegui3(t, p, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES ($1, $2, 'ACM-6600', 'finito', 'manuale', $3)`,
		fin, s.thread, s.utente)
	gruppi := Snapshot{Formato: FormatoSnapshot, ProdottoID: fin, VersioneBomBase: &VersioneBomBase{}, FirmaBase: firmaProva,
		Sorgente: &Sorgente{AllegatoID: pu(s.allegato2), Sha256: shaC, Gruppo: "bom"},
		Nodi: []Nodo{
			{ChiaveBozza: "r", ComponenteID: pu(fin), Op: OpRiusa, Codice: "ACM-6600", Tipo: "finito", Origine: "registrato"},
			{ChiaveBozza: "k-3", Op: OpCrea, Codice: "ACM-6610", Tipo: "sciolto", Origine: "step",
				Caselle: []Casella{{Gruppo: "cand:" + s.allegato2.String() + ":#3", Rif: "nodo:" + shaC + ":#3"}}},
		},
		Archi: []Arco{{Padre: "r", Figlio: "k-3", Qta: 1, Origine: "step"}},
	}
	b = bozza2DB(t, p, s.thread, fin, s.utente, gruppi)
	codice3(t, conv(b, fin), CodiceCorpoNonValido, MotivoConversionePiuGruppi)
}

// rileggiSnapshot3V: lo snapshot formato 3 di una versione salvata, riletto con la sua impronta.
func rileggiSnapshot3V(t *testing.T, p *pgxpool.Pool, bozza uuid.UUID, versione int) (Snapshot3, error) {
	t.Helper()
	v, err := db.New(p).GetDistintaBozzaVersione(context.Background(), db.GetDistintaBozzaVersioneParams{BozzaID: bozza, ThreadID: idDi("rfq"), Versione: int32(versione)})
	if err != nil {
		return Snapshot3{}, err
	}
	s, _, err := rileggiSnapshot3(v.Snapshot, v.Impronta)
	return s, err
}
