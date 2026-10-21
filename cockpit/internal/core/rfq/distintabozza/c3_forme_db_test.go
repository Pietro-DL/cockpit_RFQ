//go:build integrazione

// L4 — direttiva 35, blocco C-3 (R2 passo 6b), corsia persistenza: le forme di R2 costruite dal server sul database di
// prova, dal corpo letto da LeggiRichiestaR2 al servizio, con la chiave del salvataggio (riproposto, CHIAVE_RIUSATA).
// Solo dati sintetici: la scena di persistenza_db_test.go e gli aiuti di formato3_aiuti_test.go.

package distintabozza

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// formaR2DB: un corpo di R2 di una forma del server, letto come lo legge il transport.
func formaR2DB(t *testing.T, prodotto uuid.UUID, bozza *uuid.UUID, versione int, chiave uuid.UUID, forma string, corpoForma map[string]any) RichiestaR2 {
	t.Helper()
	m := map[string]any{"versione_attesa": versione, "prodotto_id": prodotto, "vocabolario": 2, "chiave_salvataggio": chiave, forma: corpoForma}
	if bozza != nil {
		m["bozza_id"] = *bozza
	} else {
		m["bozza_id"] = nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	r, err := LeggiRichiestaR2(raw)
	if err != nil {
		t.Fatalf("corpo di R2 (%s): %v", forma, err)
	}
	return r
}

// converti_da dal corpo di R2: la conversione, il reinvio con lo stesso corpo (riproposto, nessuna scrittura), lo stesso
// corpo con un'altra chiave dopo la conversione (la bozza formato 2 e' chiusa: BOZZA_VERSIONE), la stessa chiave con un
// altro corpo (CHIAVE_RIUSATA).
func TestV1C3ConvertiDalCorpoDiR2(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	b2 := bozza2DB(t, p, s.thread, s.altroFinito, s.utente, soloRadice(s.altroFinito, "ACM-6000"))
	chiave := uuid.New()
	corpo := map[string]any{"bozza_id": b2, "versione_attesa": 1, "motivo": "si lavora nel formato nuovo"}
	r := formaR2DB(t, s.altroFinito, nil, 0, chiave, FormaConvertiDa, corpo)
	c, err := Converti(ctx, p, r.Conversione3Da(s.thread, s.utente))
	if err != nil {
		t.Fatalf("conversione: %v", err)
	}
	if c.Versione != 1 || c.Formato != FormatoSnapshot3 || !c.NuovaVersione || c.Riproposto || len(c.Snapshot) == 0 {
		t.Fatalf("esito: %+v", c.Salvato3)
	}
	versioni := conta3(t, p, `SELECT count(*) FROM distinta_bozza_versione`)
	eventi := conta3(t, p, `SELECT count(*) FROM distinta_bozza_evento`)
	// lo stesso corpo con la stessa chiave: riproposto, con la stessa bozza e lo stesso snapshot
	r2 := formaR2DB(t, s.altroFinito, nil, 0, chiave, FormaConvertiDa, corpo)
	if r2.Comune.ImprontaRichiesta != r.Comune.ImprontaRichiesta {
		t.Fatal("lo stesso corpo con un'altra impronta")
	}
	c2, err := Converti(ctx, p, r2.Conversione3Da(s.thread, s.utente))
	if err != nil || !c2.Riproposto || c2.BozzaID != c.BozzaID || string(canonico(c2.Snapshot)) != string(canonico(c.Snapshot)) {
		t.Fatalf("reinvio: %v %+v", err, c2.Salvato3)
	}
	if conta3(t, p, `SELECT count(*) FROM distinta_bozza_versione`) != versioni || conta3(t, p, `SELECT count(*) FROM distinta_bozza_evento`) != eventi {
		t.Fatal("il reinvio ha scritto")
	}
	// lo stesso corpo con un'altra chiave: la bozza formato 2 non e' piu' aperta
	_, err = Converti(ctx, p, formaR2DB(t, s.altroFinito, nil, 0, uuid.New(), FormaConvertiDa, corpo).Conversione3Da(s.thread, s.utente))
	codice3(t, err, CodiceBozzaVersione, "")
	// la stessa chiave con un altro corpo
	corpo["motivo"] = "un altro motivo"
	_, err = Converti(ctx, p, formaR2DB(t, s.altroFinito, nil, 0, chiave, FormaConvertiDa, corpo).Conversione3Da(s.thread, s.utente))
	codice3(t, err, CodiceChiaveRiusata, "")
}
