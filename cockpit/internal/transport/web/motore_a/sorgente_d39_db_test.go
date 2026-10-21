//go:build integrazione

// L4 — direttiva 39, corsia motore: S9 con e senza S8 sul database di prova, con il calcolo vero del Motore A (scena ACME
// di servizio_db_test.go: lo STEP 9120100A con il figlio 9120101A, il PDF 9120101A_1.pdf letto con il codice del figlio):
//   - senza S8 (D-AR11 B [U]): nessuna riga del motore per il prodotto e nessun suggerimento, anche con la bozza aperta; il
//     PDF che il motore abbina resta visibile nella chip «attende la sorgente» con il motivo sorgente_non_confermata;
//   - con la conferma di S8 (conferma_documento con la chiave del passo 2): la proposta del motore per il PDF c'è
//     (provenienza motore); la bozza manuale non ha un registro, quindi nessun suggerimento (una sola sorgente, R1);
//   - TestL4V1D39S9ELeProposteDiR1: con la bozza citata dal registro vero dell'adattatore, S9 e R1 (LeggiR1) danno gli
//     stessi preassociazioni_proposte[] sulla stessa scena, vuoti senza S8 e con il suggerimento dopo_apertura del PDF
//     (chiave_bozza del nodo e importazione = impronta del registro) con S8;
//   - l'impronta degli ancoraggi del calcolo non cambia con S8 e con la bozza (le PB e la sorgente non entrano nel motore:
//     senza AR il calcolo è quello di prima);
//   - nessuna scrittura.
//
// I clienti, i codici e gli ID sono inventati (ACME, 912xxxx).
package motore_a

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
	"promatec/cockpit/internal/platform/testutil"
)

func TestL4V1D39S9ConESenzaS8(t *testing.T) {
	p, s, r := preparaMA(t)
	ctx := context.Background()
	svc := Servizio{Regole: r, Pool: p}
	sezione := func() (ProdottoSorgente, string) {
		t.Helper()
		prima := testutil.FotoDelDatabase(t, p)
		so, err := svc.Sorgente(ctx, s.thread)
		if err != nil {
			t.Fatalf("S9: %v", err)
		}
		if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
			t.Errorf("S9 ha scritto: %v", d)
		}
		ris, err := svc.Calcola(ctx, s.thread)
		if err != nil {
			t.Fatal(err)
		}
		impronta := ris.EsitoThread.Ancoraggi.Impronta
		for _, x := range so.Prodotti {
			if x.ProdottoID != nil && *x.ProdottoID == s.prodotto {
				return x, impronta
			}
		}
		t.Fatalf("nessuna sezione del prodotto: %+v", so.Prodotti)
		return ProdottoSorgente{}, ""
	}
	// la bozza aperta del prodotto con il solo prodotto (manuale, senza STEP)
	em := distintabozza.EmittenteCliente(s.cliente)
	bozza3DiProva(t, p, s.thread, s.prodotto, s.utente, map[string]any{"formato": 3, "prodotto_id": s.prodotto.String(),
		"nodi": []any{nodoDiProva("k-p", &s.prodotto, "finito", "9120100", "", em)}, "archi": []any{}, "decisioni": []any{}})

	// senza S8
	x, imp0 := sezione()
	t.Logf("senza S8: conti %+v, chip %+v", x.Conti, x.InAttesaSorgente)
	for _, rr := range x.Righe {
		if rr.Origine == OrigineRigaMotore {
			t.Errorf("senza S8, una riga del motore: %+v", rr)
		}
	}
	if len(x.PreassociazioniProposte) != 0 {
		t.Errorf("senza S8, suggerimenti: %+v", x.PreassociazioniProposte)
	}
	inAttesa := false
	for _, c := range x.InAttesaSorgente {
		if c.AllegatoID == s.aPdf && c.Motivo == MotivoAttesaNonConfermata {
			inAttesa = true
		}
	}
	if !inAttesa {
		t.Errorf("senza S8 il PDF che il motore abbina non è nella chip «attende la sorgente»: %+v", x.InAttesaSorgente)
	}

	// la conferma di S8 (passo 2) sullo STEP della scena
	inTransazione(t, p, passoSQL{`INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, gruppo, documento_id,
		chiave_idempotenza, autore_id) SELECT $1, $2, 'prodotto', 'conferma_documento', $3, a.sha256, 'bom', $4, $5, $6 FROM allegato a WHERE a.allegato_id = $3`,
		[]any{s.thread, s.prodotto, s.aStep, s.docStep, uuid.New(), s.utente}})
	x, imp1 := sezione()
	t.Logf("con S8: conti %+v, suggerimenti %+v", x.Conti, x.PreassociazioniProposte)
	var proposte []RigaSorgente
	for _, rr := range x.Righe {
		if rr.AllegatoID == s.aPdf && rr.Origine == OrigineRigaMotore {
			proposte = append(proposte, rr)
		}
	}
	if len(proposte) == 0 || proposte[0].Provenienza != ProvenienzaMotore {
		t.Errorf("con S8 la proposta del motore per il PDF: %+v", proposte)
	}
	// la bozza manuale non ha un registro: nessun suggerimento dopo l'apertura (sono quelli di R1, una sola sorgente)
	if len(x.PreassociazioniProposte) != 0 {
		t.Errorf("con S8 e la bozza manuale, suggerimenti senza registro: %+v", x.PreassociazioniProposte)
	}
	// l'impronta degli ancoraggi: la conferma di S8 e la bozza non entrano nel calcolo
	if imp0 == "" || imp0 != imp1 {
		t.Errorf("l'impronta degli ancoraggi è cambiata con S8: %q → %q", imp0, imp1)
	}
}

func TestL4V1D39S9ELeProposteDiR1(t *testing.T) {
	p, s, r := preparaMA(t)
	ctx := context.Background()
	svc := Servizio{Regole: r, Pool: p}
	rifProdotto := "componente:" + s.prodotto.String()
	ris, err := svc.Calcola(ctx, s.thread)
	if err != nil {
		t.Fatal(err)
	}
	// il registro vero dell'adattatore per il gruppo bom (la forma di R2: la versione 1 lo cita)
	reg, err := propostabom.ImportaGruppo(propostabom.Ingresso{Esito: &ris.Esito, Thread: ris.EsitoThread, Regole: ris.Regole, Prodotto: rifProdotto,
		Gruppo: propostabom.GruppoBOM, NonAttivi: ris.V1.NonAttivi, Foto: &ris.Foto})
	if err != nil {
		t.Fatalf("il registro dell'adattatore: %v", err)
	}
	proposti := distintabozza.ProposteDelRegistro(reg.Contenuto)
	if len(proposti[s.aPdf]) != 1 {
		t.Fatalf("il registro non propone il PDF su una sola voce: %v", proposti)
	}
	rifPdf := proposti[s.aPdf][0]
	col := reg.Colonne()
	var imp uuid.UUID
	if err := p.QueryRow(ctx, `INSERT INTO distinta_importazione (thread_id, prodotto_id, impronta, impronta_contenuto, impronta_evidenze, impronta_regole,
		formato_registro, versione_importazione, gruppo, fonte_allegato_id, fonte_sha256, fonte_radice, allegati_citati, contenuto, creata_da)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::jsonb, $15) RETURNING importazione_id`,
		col.ThreadID, col.ProdottoID, col.Impronta, col.ImprontaContenuto, col.ImprontaEvidenze, col.ImprontaRegole, col.FormatoRegistro,
		col.VersioneImportazione, col.Gruppo, col.FonteAllegatoID, col.FonteSha256, col.FonteRadice, col.AllegatiCitati, string(reg.Canonico), s.utente).Scan(&imp); err != nil {
		t.Fatalf("il registro: %v", err)
	}
	em := distintabozza.EmittenteCliente(s.cliente)
	casella := func(rif string) []any {
		return []any{map[string]any{"allegato_id": s.aStep.String(), "gruppo": propostabom.GruppoBOM, "rif": rif}}
	}
	np, nf := nodoDiProva("k-p", &s.prodotto, "finito", "9120100", "", em), nodoDiProva("k-f", &s.figlio, "sciolto", "9120101", "", em)
	np["caselle"], nf["caselle"] = casella(col.FonteRadice), casella(rifPdf)
	raw, _ := json.Marshal(map[string]any{"formato": 3, "prodotto_id": s.prodotto.String(),
		"sorgente": map[string]any{"allegato_id": s.aStep.String(), "sha256": col.FonteSha256, "gruppo": propostabom.GruppoBOM, "importazione": reg.Impronta},
		"nodi":     []any{np, nf},
		"archi": []any{map[string]any{"padre": "k-p", "figlio": "k-f", "qta": 2, "origine": "step",
			"step": map[string]any{"allegato_id": s.aStep.String(), "gruppo": propostabom.GruppoBOM}}},
		"decisioni": []any{}})
	snap, err := distintabozza.LeggiSnapshot3(raw)
	if err != nil {
		t.Fatalf("lo snapshot di prova non si legge: %v", err)
	}
	can, err := snap.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	b := uuid.New()
	inTransazione(t, p,
		passoSQL{`INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, formato, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
			VALUES ($1, $2, $3, 3, 0, 0, repeat('1', 64), $4, $4)`, []any{b, s.thread, s.prodotto, s.utente}},
		passoSQL{`INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base,
			fonte_allegato_id, fonte_sha256, fonte_gruppo, salvata_da, importazione_id, chiave_salvataggio, impronta_richiesta)
			VALUES ($1, 1, $2::jsonb, $3, 0, 0, repeat('1', 64), $4, $5, 'bom', $6, $7, gen_random_uuid(), repeat('4', 64))`,
			[]any{b, string(can), jsoncanonico.Impronta(can), s.aStep, col.FonteSha256, s.utente, imp}},
		passoSQL{`INSERT INTO distinta_bozza_evento (bozza_id, n, versione, vocabolario, azione, origine, utente_id) VALUES ($1, 1, 1, 2, 'bozza_aperta', 'server', $2)`,
			[]any{b, s.utente}})

	// S9 e R1 sulla stessa scena, nello stesso stato
	confronta := func(fase string) []distintabozza.PreassociazioneProposta3 {
		t.Helper()
		prima := testutil.FotoDelDatabase(t, p)
		so, err := svc.Sorgente(ctx, s.thread)
		if err != nil {
			t.Fatalf("%s, S9: %v", fase, err)
		}
		var s9 []distintabozza.PreassociazioneProposta3
		for _, x := range so.Prodotti {
			if x.ProdottoID != nil && *x.ProdottoID == s.prodotto {
				s9 = x.PreassociazioniProposte
			}
		}
		tx, err := p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		l, err := distintabozza.LeggiR1(ctx, db.New(tx), s.thread, s.prodotto, distintabozza.OpzioniR1{})
		if err != nil || l.PreassociazioniProposte == nil {
			t.Fatalf("%s, R1: %+v %v", fase, l.PreassociazioniProposte, err)
		}
		if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
			t.Errorf("%s: S9 o R1 hanno scritto: %v", fase, d)
		}
		if !reflect.DeepEqual(s9, *l.PreassociazioniProposte) {
			t.Errorf("%s: S9 %+v, R1 %+v", fase, s9, *l.PreassociazioniProposte)
		}
		return s9
	}
	// senza S8: nessun suggerimento (D-AR11 B [U]), né in S9 né in R1
	if x := confronta("senza S8"); len(x) != 0 {
		t.Errorf("senza S8: %+v", x)
	}
	// con la conferma di S8 della sorgente della versione: il PDF è un suggerimento dopo l'apertura sul nodo del figlio
	inTransazione(t, p, passoSQL{`INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, gruppo, documento_id,
		chiave_idempotenza, autore_id) SELECT $1, $2, 'prodotto', 'conferma_documento', $3, a.sha256, 'bom', $4, $5, $6 FROM allegato a WHERE a.allegato_id = $3`,
		[]any{s.thread, s.prodotto, s.aStep, s.docStep, uuid.New(), s.utente}})
	x := confronta("con S8")
	atteso := distintabozza.PreassociazioneProposta3{AllegatoID: s.aPdf, ChiaveBozza: "k-f", Provenienza: distintabozza.ProvenienzaMotore,
		Importazione: reg.Impronta, Motivo: distintabozza.PropostaDopoApertura}
	trovato := false
	for _, y := range x {
		trovato = trovato || y == atteso
	}
	if !trovato {
		t.Errorf("con S8 il suggerimento del PDF: %+v (atteso %+v)", x, atteso)
	}
}
