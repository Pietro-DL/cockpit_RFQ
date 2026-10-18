//go:build integrazione

// L4 — la conservazione del formato 3 sul database (V1, passo 2 di PM23 §14): il registro d'importazione e una bozza
// formato 3 alla versione 1 si scrivono con le query sqlc della 0023, si rileggono, e ogni riga della tabella di
// conservazione (EM §4) si ritrova byte per byte nel registro e nello snapshot (TestV1ConservazioneFormato3, l'ossatura
// per i campi che esistono al passo 2; conferma e storico arrivano con il passo 6). In piu' la parte del database di
// N-01 (T-02): la «e» con l'accento combinante del codice e del letterale passa da jsonb senza cambiare. Dati
// sintetici: la scena del database di persistenza_db_test.go.

package distintabozza

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

func TestV1ConservazioneFormato3(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	id := idSintetici3()
	if id.thread != s.thread || id.prodotto != s.prodotto || id.allegato != s.allegato || id.pdf != s.allegato2 || id.utente != s.utente {
		t.Fatal("gli identificativi della prova non sono quelli della scena del database")
	}
	reg := componiProva3(t, id)
	snap := leggiProva3(t, snapshotProva3(id, reg.Impronta))
	can, err := snap.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	imp := jsoncanonico.Impronta(can)

	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)
	riga, err := q.InsertDistintaImportazione(ctx, ParametriImportazione(reg, id.utente))
	if err != nil {
		t.Fatalf("il registro non si scrive: %v", err)
	}
	fonte := uuid.NullUUID{UUID: id.allegato, Valid: true}
	sha := pgtype.Text{String: id.sha, Valid: true}
	gruppo := pgtype.Text{String: gruppoProva3, Valid: true}
	b, err := q.InsertDistintaBozza(ctx, db.InsertDistintaBozzaParams{ThreadID: id.thread, ProdottoID: id.prodotto, Formato: FormatoSnapshot3,
		BaseContatore: 12, FirmaBase: firmaProva, FonteAllegatoID: fonte, FonteSha256: sha, FonteGruppo: gruppo, UtenteID: id.utente})
	if err != nil {
		t.Fatalf("la bozza formato 3 non si scrive: %v", err)
	}
	if _, err := q.InsertDistintaBozzaVersione(ctx, db.InsertDistintaBozzaVersioneParams{BozzaID: b.BozzaID, Versione: 1, Snapshot: can,
		Impronta: imp, BaseContatore: 12, FirmaBase: firmaProva, FonteAllegatoID: fonte, FonteSha256: sha, FonteGruppo: gruppo,
		SalvataDa: id.utente, ImportazioneID: uuid.NullUUID{UUID: riga.ImportazioneID, Valid: true},
		ChiaveSalvataggio: uuid.NullUUID{UUID: uuid.MustParse("00000000-0000-4000-8000-0000000e0f01"), Valid: true},
		ImprontaRichiesta: pgtype.Text{String: strings.Repeat("4", 64), Valid: true}}); err != nil {
		t.Fatalf("la versione 1 formato 3 non si scrive: %v", err)
	}
	if err := q.InsertDistintaBozzaEvento(ctx, db.InsertDistintaBozzaEventoParams{BozzaID: b.BozzaID, N: 1, Versione: 1,
		Vocabolario: int16(VocabolarioFormato3), Azione: "bozza_aperta", Origine: "server", UtenteID: id.utente}); err != nil {
		t.Fatalf("l'evento di apertura non si scrive: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("COMMIT: %v", err)
	}

	// la rilettura, fuori dalla transazione che ha scritto
	q = db.New(p)
	r, err := q.GetDistintaImportazione(ctx, db.GetDistintaImportazioneParams{ImportazioneID: riga.ImportazioneID, ThreadID: id.thread})
	if err != nil {
		t.Fatal(err)
	}
	riletto, testata, err := RileggiImportazione(r)
	if err != nil {
		t.Fatalf("il registro non si rilegge: %v", err)
	}
	if !bytes.Equal(riletto.Canonico, reg.Canonico) || riletto.Impronta != reg.Impronta {
		t.Fatal("il registro riletto dal database non ha gli stessi byte canonici")
	}
	per, err := q.GetDistintaImportazionePerImpronta(ctx, db.GetDistintaImportazionePerImprontaParams{ThreadID: id.thread,
		ProdottoID: id.prodotto, Impronta: reg.Impronta})
	if err != nil || per.ImportazioneID != riga.ImportazioneID {
		t.Fatalf("il registro non si ritrova per impronta: %v", err)
	}
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: b.BozzaID, ThreadID: id.thread, Versione: 1})
	if err != nil {
		t.Fatal(err)
	}
	letto, err := RileggiVersione(FormatoSnapshot3, v.Snapshot, v.Impronta)
	if err != nil {
		t.Fatalf("la versione formato 3 non si rilegge: %v", err)
	}
	if !bytes.Equal(letto.Canonico, can) || letto.Impronta != imp {
		t.Fatal("lo snapshot riletto dal database non ha gli stessi byte canonici")
	}
	if !v.ImportazioneID.Valid || v.ImportazioneID.UUID != riga.ImportazioneID || !v.ChiaveSalvataggio.Valid {
		t.Fatalf("registro o chiave della versione: %+v", v)
	}

	// N-01 nel database: il testo di jsonb porta ancora la «e» seguita dall'accento combinante
	var codice, letterale, descrizione string
	if err := p.QueryRow(ctx, `SELECT n -> 'identita' ->> 'codice' FROM distinta_bozza_versione v, jsonb_array_elements(v.snapshot -> 'nodi') n
		WHERE v.bozza_id = $1 AND n ->> 'chiave_bozza' = 'k-man'`, b.BozzaID).Scan(&codice); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `SELECT d -> 'lettura' ->> 'letterale' FROM distinta_bozza_versione v, jsonb_array_elements(v.snapshot -> 'decisioni') d
		WHERE v.bozza_id = $1 AND d ->> 'id' = 'di-0001'`, b.BozzaID).Scan(&letterale); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(ctx, `SELECT n ->> 'descrizione' FROM distinta_bozza_versione v, jsonb_array_elements(v.snapshot -> 'nodi') n
		WHERE v.bozza_id = $1 AND n ->> 'chiave_bozza' = 'k-0007'`, b.BozzaID).Scan(&descrizione); err != nil {
		t.Fatal(err)
	}
	if codice != codiceAccentato || letterale != letteraleBeta || descrizione != "Télaio" {
		t.Fatalf("N-01 nel database: codice %q, letterale %q, descrizione %q", codice, letterale, descrizione)
	}
	var letteraleRegistro string
	if err := p.QueryRow(ctx, `SELECT a ->> 'letterale' FROM distinta_importazione x, jsonb_array_elements(x.contenuto -> 'proposta' -> 'nodi') n,
		jsonb_array_elements(n -> 'alternative') a WHERE x.importazione_id = $1`, riga.ImportazioneID).Scan(&letteraleRegistro); err != nil {
		t.Fatal(err)
	}
	if letteraleRegistro != letteraleBeta {
		t.Fatalf("il letterale del registro nel database e' %q", letteraleRegistro)
	}

	eseguiCasiConservazione(t, datiConservazione{id: id, registro: riletto, testata: testata, snapshot: *letto.Formato3, canonico: letto.Canonico})
}
