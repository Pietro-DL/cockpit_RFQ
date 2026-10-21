//go:build integrazione

// L4 — V1, ondata C, corsia persistenza: R15, il rifiuto della proposta aggiornata (U2 §5.7; PM23 R-49, CN-09 lato
// server, M-35 nella parte che R15 usa). La proposta di adesso la da' la prova (PropostaDiAdesso): il calcolo del motore
// e' di chi chiama. Solo dati sintetici: la scena di ondatab_db_test.go, con una conferma formato 3 scritta a mano (i
// trigger della tabella spenti nella transazione: la prova e' sul gesto) e la bozza chiusa da quella conferma.

package distintabozza

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/jsoncanonico"
)

// confermaFormato3DB scrive a mano una conferma formato 3 valida del prodotto della scena, con il registro della scena, e
// chiude la bozza aperta come confermata da lei (R15 vale senza bozza).
func confermaFormato3DB(t *testing.T, sc scena3DB) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	s3 := leggiProva3(t, sc.base)
	f3, err := s3.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	f3 = conProvenienzaPB(t, f3)
	c := uuid.New()
	tx, err := sc.p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, x := range []struct {
		sql  string
		args []any
	}{
		{`ALTER TABLE conferma_strutturale DISABLE TRIGGER USER`, nil},
		{`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura,
				firma_anteprima, base_contatore, firma_base, contatore, contatore_dati, bozza_id, bozza_versione, fonte_allegato_id,
				fonte_sha256, fonte_gruppo, origine, snapshot, impronta_snapshot, delta, mappa, criterio_verifica, chiave_idempotenza, confermato_da,
				importazione_id, importazioni_citate, verifica)
			VALUES ($1, $2, $3, 1, 3, false, repeat('3', 64), repeat('2', 64), 12, repeat('1', 64),
				COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = $2), 0), 0, $4, 1, $5, $6, 'bom',
				'{"rotta":"prova"}', $7, $8, '{}', '{}', 'step', gen_random_uuid(), $9, $10, ARRAY[$10]::uuid[], '{"regola": "prima-istanza-1"}')`,
			[]any{c, sc.thread, sc.prodotto, sc.bozza, sc.id.allegato, sc.id.sha, f3, jsoncanonico.Impronta(f3), sc.utente, sc.importazioneID}},
		{`ALTER TABLE conferma_strutturale ENABLE TRIGGER USER`, nil},
		// la bozza si chiude «confermata» dalla conferma che la cita, come in R5
		{`UPDATE distinta_bozza SET stato = 'confermata', chiusa_da = $2, chiusa_il = now() WHERE bozza_id = $1 AND stato = 'aperta'`,
			[]any{sc.bozza, sc.utente}},
	} {
		if _, err := tx.Exec(ctx, x.sql, x.args...); err != nil {
			t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return c
}

// conProvenienzaPB: lo snapshot formato 3 di una conferma scritta a mano nella prova, con la provenienza «manuale» su
// ogni PB (direttiva 39 §4, D-PB03 A: ck_conferma_preassociazioni della variante 20261019c la vuole nella conferma; il
// tipo Go della PB non la porta ancora: la scrive R2 nel blocco della provenienza delle PB).
func conProvenienzaPB(t *testing.T, raw []byte) []byte {
	t.Helper()
	var m map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	if pp, ok := m["documenti_preassociati"].([]any); ok {
		for _, p := range pp {
			if x, ok := p.(map[string]any); ok {
				if _, c := x["provenienza"]; !c {
					x["provenienza"] = "manuale"
				}
			}
		}
	}
	out, err := jsoncanonico.Codifica(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func corpoR15(t *testing.T, chiave, prodotto, conferma uuid.UUID, impronta, motivo string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"chiave_idempotenza": chiave, "prodotto_id": prodotto, "conferma_id": conferma, "gruppo": "bom",
		"impronta_contenuto": impronta, "motivo": motivo})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestV1R15RifiutoDellaPropostaAggiornata(t *testing.T) {
	sc := preparaBozza3(t)
	conferma := confermaFormato3DB(t, sc)
	ctx := context.Background()
	nuova := strings.Repeat("7", 64)
	adesso := PropostaDiAdesso{Disponibile: true, ImprontaContenuto: nuova, Differenze: json.RawMessage(`{"nodi":[{"voce":"n:x","tipo":"cambiato","campi":["codice_proposto"]}]}`)}
	rifiuta := func(chiave uuid.UUID, impronta, motivo string, a PropostaDiAdesso) (EsitoR15, error) {
		t.Helper()
		r, imp, err := LeggiRichiestaR15(corpoR15(t, chiave, sc.prodotto, conferma, impronta, motivo))
		if err != nil {
			return EsitoR15{}, err
		}
		return RifiutaPropostaAggiornata(ctx, sc.p, sc.thread, sc.utente, r, imp, a)
	}
	foto := func() string {
		t.Helper()
		var x string
		if err := sc.p.QueryRow(ctx, `SELECT concat_ws('|', (SELECT count(*) FROM componente), (SELECT count(*) FROM conferma_strutturale),
			(SELECT count(*) FROM fonte_step_evento), (SELECT count(*) FROM documento_proposta WHERE stato <> 'aperta'),
			(SELECT count(*) FROM componente_relazione), (SELECT count(*) FROM distinta_bozza WHERE stato = 'aperta'))`).Scan(&x); err != nil {
			t.Fatal(err)
		}
		return x
	}
	prima := foto()

	// l'impronta dichiarata non e' quella di adesso: 409 ANTEPRIMA_SUPERATA, proposta_step, niente scritto
	_, err := rifiuta(uuid.New(), strings.Repeat("8", 64), "non serve", adesso)
	codice3(t, err, CodiceAnteprimaSuperata, MotivoPropostaStep)
	// la proposta di adesso e' quella della conferma: niente da rifiutare (422)
	_, err = rifiuta(uuid.New(), sc.reg.Contenuto.ImprontaContenuto, "non serve",
		PropostaDiAdesso{Disponibile: true, ImprontaContenuto: sc.reg.Contenuto.ImprontaContenuto})
	codice3(t, err, CodiceCorpoNonValido, MotivoNessunaPropostaAggiornata)
	// senza motivo: MOTIVO_OBBLIGATORIO
	_, err = rifiuta(uuid.New(), nuova, "  ", adesso)
	codice3(t, err, CodiceMotivoObbligatorio, "")
	if conta3(t, sc.p, `SELECT count(*) FROM proposta_aggiornata_rifiuto`) != 0 || conta3(t, sc.p, `SELECT count(*) FROM esito_intento`) != 0 {
		t.Fatal("un rifiuto non riuscito ha scritto")
	}

	// il rifiuto: una riga del rifiuto e l'esito dell'intento, nient'altro
	chiave := uuid.New()
	e, err := rifiuta(chiave, nuova, "manteniamo la BOM confermata", adesso)
	if err != nil {
		t.Fatalf("rifiuto: %v", err)
	}
	if e.Esito != "registrato" || e.Riproposto || e.RifiutoID == uuid.Nil || e.ConfermaID != conferma || e.ImprontaContenuto != nuova {
		t.Fatalf("esito: %+v", e)
	}
	if conta3(t, sc.p, `SELECT count(*) FROM proposta_aggiornata_rifiuto WHERE conferma_id = $1 AND impronta_contenuto = $2
		AND differenze ? 'nodi' AND motivo = 'manteniamo la BOM confermata' AND autore_id = $3`, conferma, nuova, sc.utente) != 1 {
		t.Fatal("la riga del rifiuto non c'e' o non e' quella attesa")
	}
	if conta3(t, sc.p, `SELECT count(*) FROM esito_intento WHERE chiave = $1 AND gesto = 'rifiuta_proposta' AND conferma_id IS NULL`, chiave) != 1 {
		t.Fatal("l'esito dell'intento non c'e'")
	}
	if dopo := foto(); dopo != prima {
		t.Fatalf("il rifiuto ha toccato altro: %s → %s", prima, dopo)
	}
	// lo stesso reinvio: riproposto, nessuna riga in piu'; la stessa chiave con un altro corpo: CHIAVE_RIUSATA
	e2, err := rifiuta(chiave, nuova, "manteniamo la BOM confermata", adesso)
	if err != nil || !e2.Riproposto || e2.RifiutoID != e.RifiutoID {
		t.Fatalf("reinvio: %+v %v", e2, err)
	}
	_, err = rifiuta(chiave, nuova, "un altro motivo", adesso)
	codice3(t, err, CodiceChiaveRiusata, "")
	if conta3(t, sc.p, `SELECT count(*) FROM proposta_aggiornata_rifiuto`) != 1 {
		t.Fatal("il reinvio ha scritto un'altra riga")
	}
	// la riga del rifiuto non si cambia ne' si cancella (P4X02, M-35)
	if _, err := sc.p.Exec(ctx, `UPDATE proposta_aggiornata_rifiuto SET motivo = 'altro'`); err == nil || !strings.Contains(err.Error(), "P4X02") {
		t.Fatalf("UPDATE del rifiuto: %v", err)
	}
	// una conferma che non e' l'ultima valida: ANTEPRIMA_SUPERATA conferma_non_corrente
	r, imp, err := LeggiRichiestaR15(corpoR15(t, uuid.New(), sc.prodotto, uuid.New(), nuova, "motivo"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = RifiutaPropostaAggiornata(ctx, sc.p, sc.thread, sc.utente, r, imp, adesso)
	codice3(t, err, CodiceAnteprimaSuperata, MotivoConfermaNonCorrente)
}
