//go:build integrazione

// L4 — direttiva 35, blocco C-4: S9 con due prodotti sul database di prova (COCKPIT_TEST_DSN), sulla scena ACME di
// servizio_db_test.go con un secondo prodotto (9120200, il suo codice della richiesta confermato), lo STEP della scena
// con una seconda radice 9120200A che ha lo stesso figlio, il figlio 9120101 nella BOM dei due prodotti, due bozze formato
// 3 con le loro PB e uno svincolo, e due AR scritte a mano (arDiProva). La lettura delle bozze e delle AR è quella vera
// (distintabozza.LeggiBozzeDellaRfq nella transazione dello snapshot).
//   - sorgente_di: lo STEP confermato come fonte del primo prodotto è candidato del secondo con sorgente_di = il primo;
//   - il componente condiviso: la AR del PDF del figlio è registrata nelle due sezioni (nel secondo la PB della bozza la
//     conferma, con la chiave_bozza), condivisione stesso_componente;
//   - la PB salvata con la chiave_bozza vera e conti.salvati; il file conteso fra le due bozze in_conflitto nelle due
//     sezioni (file_pertinente); il file con una AR del primo prodotto che la bozza del primo scollega (da_scollegare) e
//     che la bozza del secondo preassocia (in_conflitto, gia_associata_altrove);
//   - S9 non scrive (la foto del database); con il motore occupato ErrOccupato, senza leggere né scrivere; una bozza con
//     lo snapshot che non si rilegge: /dati risponde, S9 dà ErrLettura.
//
// I clienti, i codici e gli ID sono inventati (ACME, 912xxxx), come nella scena.
package motore_a

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/jsoncanonico"
	"promatec/cockpit/internal/platform/testutil"
)

// bozza3DiProva: una bozza formato 3 aperta con la versione 1 dello snapshot dato (con la sua impronta vera: la lettura
// delle bozze la rilegge come R1) e l'evento di apertura.
func bozza3DiProva(t *testing.T, p *pgxpool.Pool, thread, prodotto, utente uuid.UUID, snap map[string]any) uuid.UUID {
	t.Helper()
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	s, err := distintabozza.LeggiSnapshot3(raw)
	if err != nil {
		t.Fatalf("lo snapshot di prova non si legge: %v", err)
	}
	can, err := s.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	b := uuid.New()
	inTransazione(t, p,
		passoSQL{`INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, formato, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
			VALUES ($1, $2, $3, 3, 0, 0, repeat('1', 64), $4, $4)`, []any{b, thread, prodotto, utente}},
		passoSQL{`INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, salvata_da,
			chiave_salvataggio, impronta_richiesta) VALUES ($1, 1, $2::jsonb, $3, 0, 0, repeat('1', 64), $4, gen_random_uuid(), repeat('4', 64))`,
			[]any{b, string(can), jsoncanonico.Impronta(can), utente}},
		passoSQL{`INSERT INTO distinta_bozza_evento (bozza_id, n, versione, vocabolario, azione, origine, utente_id) VALUES ($1, 1, 1, 2, 'bozza_aperta', 'server', $2)`,
			[]any{b, utente}})
	return b
}

// nodoDiProva: un nodo dello snapshot formato 3 (riusa con il componente, o crea senza).
func nodoDiProva(chiave string, componente *uuid.UUID, tipo, codice, rev, emittente string) map[string]any {
	id := map[string]any{"emittente": emittente, "emittente_presunto": true, "codice": codice, "modo": "registrato"}
	if rev != "" {
		id["rev"] = rev
	}
	n := map[string]any{"chiave_bozza": chiave, "op": "riusa", "tipo": tipo, "origine": "registrato", "identita": id}
	if componente == nil {
		n["op"], n["origine"], id["modo"] = "crea", "utente", "manuale"
	} else {
		n["componente_id"] = componente.String()
	}
	return n
}

// righeS9: le righe di un allegato nella sezione di un prodotto della risposta.
func righeS9(so *Sorgente, prodotto, allegato uuid.UUID) []RigaSorgente {
	var out []RigaSorgente
	for _, p := range so.Prodotti {
		if p.ProdottoID != nil && *p.ProdottoID == prodotto {
			for _, r := range p.Righe {
				if r.AllegatoID == allegato {
					out = append(out, r)
				}
			}
		}
	}
	return out
}

func TestL4V1C4LaSorgenteConDueProdotti(t *testing.T) {
	p, s, r := preparaMA(t)
	ctx := context.Background()
	riga := func(dst any, sql string, arg ...any) {
		t.Helper()
		if err := p.QueryRow(ctx, sql, arg...).Scan(dst); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	esegui := func(sql string, arg ...any) {
		t.Helper()
		if _, err := p.Exec(ctx, sql, arg...); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
	}
	// il secondo prodotto, con il codice della richiesta confermato
	var p2 uuid.UUID
	riga(&p2, `INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, '9120200', 'finito', 'manuale', $2)
		RETURNING componente_id`, s.thread, s.utente)
	esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confermato_da) VALUES ($1, '9120200', 'proposta_corpo', $2)`, s.thread, s.utente)
	// lo STEP della scena con una seconda radice 9120200A che ha lo stesso figlio 9120101A
	struttura := `{"struttura": {"versione": 3, "schema": "AP214", "radici": ["#1", "#3"], "avvisi": [], "nodi": [` +
		`{"chiave": "#1", "id_grezzo": "9120100A", "nome_grezzo": "", "evidenza": {}}, {"chiave": "#2", "id_grezzo": "9120101A", "nome_grezzo": "", "evidenza": {}}, ` +
		`{"chiave": "#3", "id_grezzo": "9120200A", "nome_grezzo": "", "evidenza": {}}], ` +
		`"relazioni": [{"padre": "#1", "figlio": "#2", "qta": 2, "evidenza": {}}, {"padre": "#3", "figlio": "#2", "qta": 1, "evidenza": {}}], ` +
		`"limiti": {"troncato": false}, ` +
		`"scarti": {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}}`
	esegui(`UPDATE analisi_fatti SET fatti = $2 WHERE sha256 = $1`, strings.Repeat("a1", 32), struttura)
	// il figlio nella BOM dei due prodotti
	esegui(`INSERT INTO componente_relazione (thread_id, padre_id, figlio_id, qta, origine, confermato_da) VALUES ($1, $2, $4, 2, 'manuale', $5),
		($1, $3, $4, 1, 'manuale', $5)`, s.thread, s.prodotto, p2, s.figlio, s.utente)
	// i file nuovi (senza fatti: nessuna proposta del motore, solo le righe delle bozze e delle AR)
	allegato := func(indice int, nome, sha string) uuid.UUID {
		var id uuid.UUID
		riga(&id, `INSERT INTO allegato (messaggio_id, indice, nome_file, estensione, natura, origine, bytes, sha256, stato, ricevuto_il)
			VALUES ($1, $2, $3, 'pdf', 'file', 'outlook', 100, $4, 'analizzato', now()) RETURNING allegato_id`, s.messaggio, indice, nome, sha)
		return id
	}
	pdfS, pdfC, pdfA := allegato(11, "9120150_1.pdf", strings.Repeat("b1", 32)), allegato(12, "9120160_1.pdf", strings.Repeat("b2", 32)),
		allegato(13, "9120170_1.pdf", strings.Repeat("b3", 32))
	// le AR: il PDF del figlio sul figlio (condiviso), il terzo file sul primo prodotto
	arDiProva(t, p, s.thread, s.prodotto, s.aPdf, s.figlio, s.utente)
	arDiProva(t, p, s.thread, p2, pdfA, s.prodotto, s.utente) // la testata di prova sul secondo prodotto: una sola conferma per prodotto
	// le due bozze
	em := distintabozza.EmittenteCliente(s.cliente)
	bozza3DiProva(t, p, s.thread, s.prodotto, s.utente, map[string]any{"formato": 3, "prodotto_id": s.prodotto.String(),
		"versione_bom_base": map[string]any{"conferma_id": nil, "numero": 0, "contatore": 0}, "firma_base": strings.Repeat("1", 64),
		"nodi": []any{nodoDiProva("k-p", &s.prodotto, "finito", "9120100", "", em), nodoDiProva("k-f", &s.figlio, "sciolto", "9120101", "2", em),
			nodoDiProva("k-n", nil, "commerciale", "9120150", "A", em)},
		"archi": []any{map[string]any{"padre": "k-p", "figlio": "k-f", "qta": 2, "origine": "utente"},
			map[string]any{"padre": "k-p", "figlio": "k-n", "qta": 1, "origine": "utente"}},
		"decisioni": []any{map[string]any{"id": "sv-1", "tipo": "scollega_documento", "allegato_id": pdfA.String(), "componente_id": s.prodotto.String(),
			"motivo": "il file è di un altro prodotto (prova)"}},
		"documenti_preassociati": []any{map[string]any{"allegato_id": pdfS.String(), "chiave_bozza": "k-n"},
			map[string]any{"allegato_id": pdfC.String(), "chiave_bozza": "k-n"}}})
	bozza3DiProva(t, p, s.thread, p2, s.utente, map[string]any{"formato": 3, "prodotto_id": p2.String(),
		"versione_bom_base": map[string]any{"conferma_id": nil, "numero": 0, "contatore": 0}, "firma_base": strings.Repeat("1", 64),
		"nodi": []any{nodoDiProva("k-q", &p2, "finito", "9120200", "", em), nodoDiProva("k-g", &s.figlio, "sciolto", "9120101", "2", em),
			nodoDiProva("k-m", nil, "commerciale", "9120160", "", em)},
		"archi": []any{map[string]any{"padre": "k-q", "figlio": "k-g", "qta": 1, "origine": "utente"},
			map[string]any{"padre": "k-q", "figlio": "k-m", "qta": 1, "origine": "utente"}},
		"decisioni": []any{},
		"documenti_preassociati": []any{map[string]any{"allegato_id": s.aPdf.String(), "chiave_bozza": "k-g"},
			map[string]any{"allegato_id": pdfC.String(), "chiave_bozza": "k-m"}, map[string]any{"allegato_id": pdfA.String(), "chiave_bozza": "k-m"}}})

	svc := Servizio{Regole: r, Pool: p}
	prima := testutil.FotoDelDatabase(t, p)
	so, err := svc.Sorgente(ctx, s.thread)
	if err != nil {
		t.Fatalf("Sorgente: %v", err)
	}
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("S9 ha cambiato il database: %v", d)
	}
	j, _ := json.MarshalIndent(so, "", " ")
	t.Logf("S9 con due prodotti: %s", j)
	if len(so.Prodotti) != 2 {
		t.Fatalf("le sezioni: %d", len(so.Prodotti))
	}

	// sorgente_di: lo STEP della fonte del primo prodotto, candidato del secondo
	var candP2 *CandidataDelloStep
	for _, st := range so.Step {
		for i, c := range st.Candidate {
			if st.AllegatoID == s.aStep && c.ProdottoID != nil && *c.ProdottoID == p2 {
				candP2 = &st.Candidate[i]
			}
		}
	}
	if candP2 == nil || candP2.SorgenteDi == nil || *candP2.SorgenteDi != s.prodotto {
		t.Errorf("lo STEP della fonte del primo prodotto come candidato del secondo: %+v", candP2)
	}

	una := func(prodotto, allegato uuid.UUID, stato, motivo, origine string) RigaSorgente {
		t.Helper()
		rr := righeS9(so, prodotto, allegato)
		if len(rr) != 1 || rr[0].Stato != stato || rr[0].Motivo != motivo || rr[0].Origine != origine {
			t.Fatalf("l'allegato %s nel prodotto %s: %+v (atteso %s/%s/%s)", allegato, prodotto, rr, stato, motivo, origine)
		}
		return rr[0]
	}
	// il componente condiviso: la AR del PDF del figlio nelle due sezioni, stesso_componente
	x := una(s.prodotto, s.aPdf, StatoRigaRegistrato, "", OrigineRigaWorking)
	if x.Destinazione.ComponenteID == nil || *x.Destinazione.ComponenteID != s.figlio || len(x.Condivisione) != 1 ||
		x.Condivisione[0] != (CondivisioneRiga{ProdottoID: p2, Motivo: CondivisioneStessoComponente}) {
		t.Errorf("la AR del figlio nel primo prodotto: %+v", x)
	}
	x = una(p2, s.aPdf, StatoRigaRegistrato, "", OrigineRigaBozza)
	if x.Destinazione.ChiaveBozza != "k-g" || x.Destinazione.ComponenteID == nil || *x.Destinazione.ComponenteID != s.figlio ||
		len(x.CondivisoCon) != 1 || x.CondivisoCon[0] != s.prodotto || x.Condivisione[0].Motivo != CondivisioneStessoComponente {
		t.Errorf("la PB che conferma la AR del figlio nel secondo prodotto: %+v", x)
	}
	// la PB salvata, con la chiave_bozza vera
	x = una(s.prodotto, pdfS, StatoRigaSalvato, "", OrigineRigaBozza)
	if x.Destinazione.ChiaveBozza != "k-n" || x.CodiceMostrato != "9120150" || x.RevMostrata != "A" || x.Destinazione.ComponenteID != nil {
		t.Errorf("la PB salvata: %+v", x)
	}
	// il file conteso fra le due bozze
	for _, pr := range []uuid.UUID{s.prodotto, p2} {
		x := una(pr, pdfC, StatoRigaInConflitto, MotivoRigaFileConteso, OrigineRigaBozza)
		if len(x.Condivisione) != 1 || x.Condivisione[0].Motivo != CondivisioneFilePertinente {
			t.Errorf("il file conteso nel prodotto %s: %+v", pr, x)
		}
	}
	// la AR del primo prodotto: la sua bozza la scollega, la bozza del secondo la preassocia
	x = una(s.prodotto, pdfA, StatoRigaDaScollegare, "", OrigineRigaBozza)
	if x.Destinazione.ComponenteID == nil || *x.Destinazione.ComponenteID != s.prodotto {
		t.Errorf("lo svincolo: %+v", x)
	}
	una(p2, pdfA, StatoRigaInConflitto, MotivoRigaGiaAssociataAltrove, OrigineRigaBozza)
	// i file delle bozze e delle AR non sono più da associare
	for _, d := range so.DaAssociare {
		if d.AllegatoID == pdfS || d.AllegatoID == pdfC || d.AllegatoID == pdfA || d.AllegatoID == s.aPdf {
			t.Errorf("un file con una PB o una AR fra i file da associare: %+v", d)
		}
	}
	// i conti, dalle righe
	for _, sez := range so.Prodotti {
		if sez.Bozza == nil {
			t.Errorf("la sezione %s senza la bozza aperta", sez.Codice)
		}
		switch *sez.ProdottoID {
		case s.prodotto:
			if c := sez.Conti; c.Salvati != 1 || c.Registrati != 1 || c.DaScollegare != 1 || c.InConflitto != 1 {
				t.Errorf("i conti del primo prodotto: %+v", c)
			}
		case p2:
			if c := sez.Conti; c.Salvati != 0 || c.Registrati != 1 || c.InConflitto != 2 || c.DaScollegare != 0 {
				t.Errorf("i conti del secondo prodotto: %+v", c)
			}
		}
	}
	// la stessa risposta a una seconda lettura
	if so2, err := svc.Sorgente(ctx, s.thread); err != nil || so2.Firma != so.Firma {
		t.Errorf("la seconda lettura: %v, firma uguale %v", err, err == nil && so2.Firma == so.Firma)
	}

	// il motore occupato: ErrOccupato, senza leggere né scrivere
	pieno := make(chan struct{}, MassimoCalcoli)
	for range MassimoCalcoli {
		pieno <- struct{}{}
	}
	prima = testutil.FotoDelDatabase(t, p)
	if _, err := (Servizio{Regole: r, Pool: p, posti: pieno}).Sorgente(ctx, s.thread); !errors.Is(err, ErrOccupato) {
		t.Errorf("con il motore occupato: %v, atteso ErrOccupato", err)
	}
	if d := testutil.Differenze(prima, testutil.FotoDelDatabase(t, p)); len(d) > 0 {
		t.Errorf("con il motore occupato il database è cambiato: %v", d)
	}

	// una bozza con lo snapshot che non si rilegge (l'impronta registrata è un'altra): /dati risponde, S9 no
	var p3 uuid.UUID
	riga(&p3, `INSERT INTO componente (thread_id, codice, tipo, origine, confermato_da) VALUES ($1, '9120300', 'finito', 'manuale', $2)
		RETURNING componente_id`, s.thread, s.utente)
	b3 := uuid.New()
	snap3, _ := json.Marshal(map[string]any{"formato": 3, "prodotto_id": p3.String(), "nodi": []any{nodoDiProva("k-z", &p3, "finito", "9120300", "", em)},
		"archi": []any{}, "decisioni": []any{}})
	inTransazione(t, p,
		passoSQL{`INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, formato, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
			VALUES ($1, $2, $3, 3, 0, 0, repeat('1', 64), $4, $4)`, []any{b3, s.thread, p3, s.utente}},
		passoSQL{`INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, salvata_da,
			chiave_salvataggio, impronta_richiesta) VALUES ($1, 1, $2::jsonb, repeat('6', 64), 0, 0, repeat('1', 64), $3, gen_random_uuid(), repeat('4', 64))`,
			[]any{b3, string(snap3), s.utente}},
		passoSQL{`INSERT INTO distinta_bozza_evento (bozza_id, n, versione, vocabolario, azione, origine, utente_id) VALUES ($1, 1, 1, 2, 'bozza_aperta', 'server', $2)`,
			[]any{b3, s.utente}})
	if _, err := svc.Pagina(ctx, RichiestaPagina{Thread: s.thread}); err != nil {
		t.Errorf("/dati con una bozza illeggibile: %v", err)
	}
	if _, err := svc.Sorgente(ctx, s.thread); !errors.Is(err, ErrLettura) || !errors.Is(err, distintabozza.ErrSnapshotIncoerente) {
		t.Errorf("S9 con una bozza illeggibile: %v, attesi ErrLettura ed ErrSnapshotIncoerente", err)
	}
}
