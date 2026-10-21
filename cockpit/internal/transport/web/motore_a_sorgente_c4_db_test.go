//go:build integrazione

// L4 — direttiva 35, blocco C-4 (corsia motore, aggancio dichiarato: un file di prova nuovo in transport/web): S9 sulla
// rotta HTTP, GET /thread/{id}/motore-a/dati?sorgente=1, con le sessioni vere (EM §10.9 «S9»; prove R-36…R-40 di PM23),
// sulla scena B87 (mwScena) con un secondo prodotto che ha il particolare nella sua BOM, le due bozze formato 3 e una AR:
//   - R-36: S9 non scrive (la foto del database prima e dopo), la firma è la stessa a due letture, no-store e nosniff;
//     a flag spento 404 a tutti; a flag acceso solo l'amministratore (operatore e consultazione 403, l'anonimo 302);
//   - R-37: una sezione per prodotto, con la bozza aperta, lo stato del calcolo e i conti;
//   - R-38: i riferimenti stabili: la PB con la chiave_bozza del suo nodo, la AR con il componente, mai il codice;
//   - R-39: il file condiviso (la AR sul particolare, che sta nella BOM dei due prodotti: stesso_componente) e il file in
//     conflitto (preassociato dalle due bozze a due nodi diversi: in_conflitto con file_conteso_fra_prodotti);
//   - R-40: le regole non pronte 503, la RFQ che non c'è 404, una bozza con lo snapshot che non si rilegge: /dati 200 e
//     S9 500, senza scrivere. (Il motore occupato è nella L4 del servizio, sorgente_c4_db_test.go di motore_a: qui il
//     semaforo del processo non si raggiunge; la traduzione in 503 è quella di /dati, erroreMotoreA.)
//
// I codici 777xxxxx sono quelli della scena B87 (vedi motore_a_db_test.go); il resto è inventato (ACME, UUID sintetici).
package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/platform/jsoncanonico"
	motorea_web "promatec/cockpit/internal/transport/web/motore_a"
)

// s9Bozza3: una bozza formato 3 aperta, con la versione 1 dello snapshot dato e la sua impronta vera (impronta = "" ) o
// quella data (una bozza che non si rilegge), e l'evento di apertura.
func (s *mwScena) s9Bozza3(t *testing.T, prodotto uuid.UUID, snap map[string]any, impronta string) {
	t.Helper()
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	x, err := distintabozza.LeggiSnapshot3(raw)
	if err != nil {
		t.Fatalf("lo snapshot di prova non si legge: %v", err)
	}
	can, err := x.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	if impronta == "" {
		impronta = jsoncanonico.Impronta(can)
	}
	b := uuid.New()
	// in una transazione: la testata e la versione corrente si citano a vicenda (FK differita)
	tx, err := s.b.pool.Begin(s.b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(s.b.ctx) }()
	for _, x := range []struct {
		sql string
		arg []any
	}{
		{`INSERT INTO distinta_bozza (bozza_id, thread_id, prodotto_id, formato, base_numero, base_contatore, firma_base, creata_da, aggiornata_da)
			VALUES ($1, $2, $3, 3, 0, 0, repeat('1', 64), $4, $4)`, []any{b, s.thread, prodotto, s.utente}},
		{`INSERT INTO distinta_bozza_versione (bozza_id, versione, snapshot, impronta, base_numero, base_contatore, firma_base, salvata_da,
			chiave_salvataggio, impronta_richiesta) VALUES ($1, 1, $2::jsonb, $3, 0, 0, repeat('1', 64), $4, gen_random_uuid(), repeat('4', 64))`,
			[]any{b, string(can), impronta, s.utente}},
		{`INSERT INTO distinta_bozza_evento (bozza_id, n, versione, vocabolario, azione, origine, utente_id) VALUES ($1, 1, 1, 2, 'bozza_aperta', 'server', $2)`,
			[]any{b, s.utente}},
	} {
		if _, err := tx.Exec(s.b.ctx, x.sql, x.arg...); err != nil {
			t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	if err := tx.Commit(s.b.ctx); err != nil {
		t.Fatal(err)
	}
}

// s9AR: una AR attiva scritta a mano, con la sua testata di conferma, con i trigger delle due tabelle spenti solo nella
// sua transazione (come le prove della persistenza): la scena non ha una conferma formato 3 vera.
func (s *mwScena) s9AR(t *testing.T, prodotto, allegato, componente uuid.UUID) {
	t.Helper()
	tx, err := s.b.pool.Begin(s.b.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(s.b.ctx) }()
	conferma := uuid.New()
	for _, x := range []struct {
		sql string
		arg []any
	}{
		{`ALTER TABLE conferma_strutturale DISABLE TRIGGER USER`, nil},
		{`ALTER TABLE conferma_strutturale_componente DISABLE TRIGGER USER`, nil},
		{`ALTER TABLE file_rfq_associazione DISABLE TRIGGER USER`, nil},
		{`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura,
				base_contatore, contatore, contatore_dati, origine, snapshot, impronta_snapshot, delta, mappa, criterio_verifica, confermato_da)
			VALUES ($1, $2, $3, 1, 1, true, repeat('3', 64), 0, 0, 0, '{"rotta":"prova"}', '{"formato":1}', repeat('9', 64), '{}', '{}', 'nessuno', $4)`,
			[]any{conferma, s.thread, prodotto, s.utente}},
		{`INSERT INTO conferma_strutturale_componente (conferma_id, thread_id, componente_id, codice, tipo)
			SELECT $1, c.thread_id, c.componente_id, c.codice, c.tipo FROM componente c WHERE c.componente_id = $2`, []any{conferma, componente}},
		{`INSERT INTO file_rfq_associazione (thread_id, allegato_id, componente_id, origine, conferma_id, chiave_bozza, chiave_idempotenza, creata_da, provenienza_pb)
			VALUES ($1, $2, $3, 'conferma_bom', $4, 'k-prova', gen_random_uuid(), $5, 'manuale')`, []any{s.thread, allegato, componente, conferma, s.utente}},
		{`SET CONSTRAINTS ALL IMMEDIATE`, nil},
		{`ALTER TABLE file_rfq_associazione ENABLE TRIGGER USER`, nil},
		{`ALTER TABLE conferma_strutturale_componente ENABLE TRIGGER USER`, nil},
		{`ALTER TABLE conferma_strutturale ENABLE TRIGGER USER`, nil},
	} {
		if _, err := tx.Exec(s.b.ctx, x.sql, x.arg...); err != nil {
			t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	if err := tx.Commit(s.b.ctx); err != nil {
		t.Fatal(err)
	}
}

// s9Nodo: un nodo dello snapshot formato 3 (riusa con il componente, o crea senza).
func s9Nodo(chiave string, componente *uuid.UUID, tipo, codice, emittente string) map[string]any {
	id := map[string]any{"emittente": emittente, "emittente_presunto": true, "codice": codice, "modo": "registrato"}
	n := map[string]any{"chiave_bozza": chiave, "op": "riusa", "tipo": tipo, "origine": "registrato", "identita": id}
	if componente == nil {
		n["op"], n["origine"], id["modo"] = "crea", "utente", "manuale"
	} else {
		n["componente_id"] = componente.String()
	}
	return n
}

func TestMotoreAL4S9SullaRotta(t *testing.T) {
	b := preparaBancoWeb(t)
	s := b.mwScena(t, "MA9S")
	sorgente := s.dati() + "?sorgente=1"

	// R-36, a flag spento: 404 a tutti
	spente := s.regole
	spente.Accesa, spente.Stato = false, motorea_web.StatoSpenta
	b.ws.MotoreA = spente
	for nome, w := range map[string]*browser{"anonimo": s.anonimo, "operatore": s.op, "consultazione": s.co, "amministratore": s.ad} {
		if resp, _ := w.fai(http.MethodGet, sorgente, nil, false); resp.StatusCode != http.StatusNotFound {
			t.Errorf("flag spento, %s: %d", nome, resp.StatusCode)
		}
	}
	mwAccendi(t, b, s.regole)

	// la scena: i due prodotti con il codice della richiesta confermato, il particolare anche nella BOM del secondo, i
	// file nuovi, la AR del PDF del particolare, le due bozze
	p2 := s.componenteTipo("77799001", "finito")
	s.arco(p2, s.particolare, 1)
	s.esegui(`INSERT INTO identificativo_thread (thread_id, codice, origine, confermato_da) VALUES ($1, '77722757', 'manuale', $2),
		($1, '77799001', 'manuale', $2) ON CONFLICT DO NOTHING`, s.thread, s.utente)
	fileS, _ := s.allegato("77799150.pdf")
	fileC, _ := s.allegato("77799160.pdf")
	s.s9AR(t, s.prodotto, s.allLibero, s.particolare)
	em := distintabozza.EmittenteCliente(uuidSQL(t, b, `SELECT cliente_id FROM thread_offerta WHERE thread_id = $1`, s.thread))
	s.s9Bozza3(t, s.prodotto, map[string]any{"formato": 3, "prodotto_id": s.prodotto.String(),
		"nodi":  []any{s9Nodo("k-p", &s.prodotto, "finito", "77722757", em), s9Nodo("k-n", nil, "commerciale", "77799150", em)},
		"archi": []any{map[string]any{"padre": "k-p", "figlio": "k-n", "qta": 1, "origine": "utente"}}, "decisioni": []any{},
		"documenti_preassociati": []any{map[string]any{"allegato_id": fileS.String(), "chiave_bozza": "k-n"},
			map[string]any{"allegato_id": fileC.String(), "chiave_bozza": "k-n"}}}, "")
	s.s9Bozza3(t, p2, map[string]any{"formato": 3, "prodotto_id": p2.String(),
		"nodi":  []any{s9Nodo("k-q", &p2, "finito", "77799001", em), s9Nodo("k-m", nil, "commerciale", "77799160", em)},
		"archi": []any{map[string]any{"padre": "k-q", "figlio": "k-m", "qta": 1, "origine": "utente"}}, "decisioni": []any{},
		"documenti_preassociati": []any{map[string]any{"allegato_id": fileC.String(), "chiave_bozza": "k-m"}}}, "")

	// R-36, a flag acceso: solo l'amministratore; nessuna scrittura; la firma stabile
	for nome, w := range map[string]*browser{"operatore": s.op, "consultazione": s.co} {
		if resp, _ := w.fai(http.MethodGet, sorgente, nil, false); resp.StatusCode != http.StatusForbidden || resp.Header.Get("Cache-Control") != "no-store" {
			t.Errorf("%s: %d %q, attesi 403 e no-store", nome, resp.StatusCode, resp.Header.Get("Cache-Control"))
		}
	}
	if resp, _ := s.anonimo.fai(http.MethodGet, sorgente, nil, false); resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/login" {
		t.Errorf("anonimo: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	prima := fotoDelDatabase(t, b)
	resp, corpo := s.ad.fai(http.MethodGet, sorgente, nil, false)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/json; charset=utf-8" || resp.Header.Get("Cache-Control") != "no-store" ||
		resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("amministratore: %d %q %q: %.400s", resp.StatusCode, resp.Header.Get("Content-Type"), resp.Header.Get("Cache-Control"), corpo)
	}
	_, corpo2 := s.ad.fai(http.MethodGet, sorgente, nil, false)
	if d := differenzeFoto(prima, fotoDelDatabase(t, b)); len(d) > 0 {
		t.Errorf("S9 ha scritto: %v", d)
	}
	var so, so2 motorea_web.Sorgente
	if err := json.Unmarshal([]byte(corpo), &so); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(corpo2), &so2); err != nil {
		t.Fatal(err)
	}
	if len(so.Firma) != 64 || so.Firma != so2.Firma {
		t.Errorf("la firma: %q / %q", so.Firma, so2.Firma)
	}
	t.Logf("S9 sulla rotta: %.3000s", corpo)

	// R-37: una sezione per prodotto, con la bozza aperta, il calcolo e i conti
	sezione := map[uuid.UUID]motorea_web.ProdottoSorgente{}
	for _, p := range so.Prodotti {
		if p.ProdottoID != nil {
			sezione[*p.ProdottoID] = p
		}
	}
	p1s, ok1 := sezione[s.prodotto]
	p2s, ok2 := sezione[p2]
	if !ok1 || !ok2 || p1s.Bozza == nil || p2s.Bozza == nil || p1s.Codice != "77722757" || p2s.Codice != "77799001" {
		t.Fatalf("le sezioni dei due prodotti: %+v", so.Prodotti)
	}
	for _, p := range []motorea_web.ProdottoSorgente{p1s, p2s} {
		if p.Calcolo.Stato != motorea_web.CalcoloPronto && p.Calcolo.Stato != motorea_web.CalcoloInAttesaAnalisi && p.Calcolo.Stato != motorea_web.CalcoloErrore {
			t.Errorf("lo stato del calcolo di %s: %+v", p.Codice, p.Calcolo)
		}
	}
	una := func(p motorea_web.ProdottoSorgente, allegato uuid.UUID) motorea_web.RigaSorgente {
		t.Helper()
		var out []motorea_web.RigaSorgente
		for _, r := range p.Righe {
			if r.AllegatoID == allegato {
				out = append(out, r)
			}
		}
		if len(out) != 1 {
			t.Fatalf("le righe dell'allegato %s in %s: %+v", allegato, p.Codice, out)
		}
		return out[0]
	}
	// R-38: la PB con la chiave_bozza, la AR con il componente
	x := una(p1s, fileS)
	if x.Stato != motorea_web.StatoRigaSalvato || x.Destinazione != (motorea_web.DestinazioneRiga{ChiaveBozza: "k-n"}) || x.CodiceMostrato != "77799150" ||
		p1s.Conti.Salvati != 1 {
		t.Errorf("la PB salvata: %+v, conti %+v", x, p1s.Conti)
	}
	if strings.Contains(corpo, `"destinazione":{"codice`) {
		t.Error("una destinazione con il codice")
	}
	// R-39: il file condiviso sul componente condiviso, nelle due sezioni
	for _, p := range []motorea_web.ProdottoSorgente{p1s, p2s} {
		x := una(p, s.allLibero)
		if x.Stato != motorea_web.StatoRigaRegistrato || x.Destinazione.ComponenteID == nil || *x.Destinazione.ComponenteID != s.particolare ||
			len(x.Condivisione) != 1 || x.Condivisione[0].Motivo != motorea_web.CondivisioneStessoComponente || p.Conti.Registrati < 1 {
			t.Errorf("il file condiviso in %s: %+v", p.Codice, x)
		}
	}
	// R-39: il file in conflitto fra le due bozze
	for _, p := range []motorea_web.ProdottoSorgente{p1s, p2s} {
		x := una(p, fileC)
		if x.Stato != motorea_web.StatoRigaInConflitto || x.Motivo != motorea_web.MotivoRigaFileConteso || len(x.Condivisione) != 1 ||
			x.Condivisione[0].Motivo != motorea_web.CondivisioneFilePertinente {
			t.Errorf("il file conteso in %s: %+v", p.Codice, x)
		}
	}

	// R-40: le regole non pronte, la RFQ che non c'è
	r := s.regole
	r.Stato, r.Insieme = motorea_web.StatoNessunaGrammatica, nil
	b.ws.MotoreA = r
	if resp, corpo := s.ad.fai(http.MethodGet, sorgente, nil, false); resp.StatusCode != http.StatusServiceUnavailable || resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("regole non pronte: %d %s", resp.StatusCode, corpo)
	}
	b.ws.MotoreA = s.regole
	nessuna := "/thread/00000000-0000-4000-8000-00000000a1d9/motore-a/dati?sorgente=1"
	if resp, corpo := s.ad.fai(http.MethodGet, nessuna, nil, false); resp.StatusCode != http.StatusNotFound || !strings.Contains(corpo, `"errore":"RFQ non trovata"`) {
		t.Errorf("RFQ inesistente: %d %s", resp.StatusCode, corpo)
	}
	// R-40: una bozza che non si rilegge: /dati risponde, S9 no, e niente si scrive
	p3 := s.componenteTipo("77799003", "finito")
	s.s9Bozza3(t, p3, map[string]any{"formato": 3, "prodotto_id": p3.String(), "nodi": []any{s9Nodo("k-z", &p3, "finito", "77799003", em)},
		"archi": []any{}, "decisioni": []any{}}, strings.Repeat("6", 64))
	prima = fotoDelDatabase(t, b)
	if resp, corpo := s.ad.fai(http.MethodGet, s.dati(), nil, false); resp.StatusCode != 200 {
		t.Errorf("/dati con una bozza che non si rilegge: %d %.300s", resp.StatusCode, corpo)
	}
	if resp, corpo := s.ad.fai(http.MethodGet, sorgente, nil, false); resp.StatusCode != http.StatusInternalServerError || resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("S9 con una bozza che non si rilegge: %d %.300s", resp.StatusCode, corpo)
	}
	if d := differenzeFoto(prima, fotoDelDatabase(t, b)); len(d) > 0 {
		t.Errorf("le letture hanno scritto: %v", d)
	}
}
