//go:build integrazione

// L4 — direttiva 35, blocco C-3 (R2 passo 6b), corsia persistenza: la forma crea senza il motore (vuota, registrato,
// conferma con da-conferma-1, la ripartenza) sul database di prova, dal corpo letto da LeggiRichiestaR2. Solo dati
// sintetici: la scena di persistenza_db_test.go.

package distintabozza

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// creaDB: la forma crea dal corpo di R2, con la chiave data.
func creaDB(t *testing.T, p *pgxpool.Pool, s scenaDB, prodotto uuid.UUID, chiave uuid.UUID, corpoForma map[string]any) (Creata, error) {
	t.Helper()
	r := formaR2DB(t, prodotto, nil, 0, chiave, FormaCrea, corpoForma)
	return CreaFormato3(context.Background(), p, r.CreaDaRichiesta(s.thread, s.utente, nil))
}

// confermaC3DB: una conferma formato 3 del prodotto scritta a mano (i trigger di conferma_strutturale spenti nella
// transazione: la prova e' su da-conferma-1, non sulla conferma), valida per contatore (quello della working di adesso),
// con lo snapshot, la mappa chiave → componente e la fonte dati. La bozza della conferma e' una bozza chiusa del prodotto.
func confermaC3DB(t *testing.T, p *pgxpool.Pool, s scenaDB, prodotto, bozza uuid.UUID, numero int, precedente *uuid.UUID, snap Snapshot3,
	mappa map[string]uuid.UUID, fonte *Sorgente3) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	can, err := snap.Canonico()
	if err != nil {
		t.Fatalf("snapshot della conferma: %v", err)
	}
	id := uuid.New()
	var all, sha, gruppo any
	if fonte != nil {
		all, sha, gruppo = *fonte.AllegatoID, fonte.Sha256, fonte.Gruppo
	}
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
		{`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, precedente_id, formato, senza_riepilogo_v2, firma_struttura,
				firma_anteprima, base_conferma_id, base_contatore, firma_base, contatore, contatore_dati, bozza_id, bozza_versione, fonte_allegato_id,
				fonte_sha256, fonte_gruppo, origine, snapshot, impronta_snapshot, delta, mappa, criterio_verifica, chiave_idempotenza, confermato_da,
				importazioni_citate, verifica, mono_pezzo, mono_pezzo_motivo)
			VALUES ($1, $2, $3, $4, $5, 3, false, repeat('3', 64), repeat('2', 64), $5,
				(SELECT COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = $2), 0)), repeat('1', 64),
				(SELECT COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = $2), 0)), 0, $6, 1, $7, $8, $9,
				'{"rotta":"prova"}', $10, $11, '{}', $12,
				CASE WHEN $10::jsonb -> 'decisioni' @> '[{"tipo": "mono_pezzo"}]' THEN 'mono_pezzo' ELSE 'nessuno' END, gen_random_uuid(), $13, '{}'::uuid[], '{"regola": "prima-istanza-1"}',
				$10::jsonb -> 'decisioni' @> '[{"tipo": "mono_pezzo"}]',
				CASE WHEN $10::jsonb -> 'decisioni' @> '[{"tipo": "mono_pezzo"}]' THEN 'pezzo unico' END)`,
			[]any{id, s.thread, prodotto, numero, nullUUID(precedente), bozza, all, sha, gruppo, can, jsoncanonico.Impronta(can), mustJSON(mappa), s.utente}},
		{`ALTER TABLE conferma_strutturale ENABLE TRIGGER USER`, nil},
	} {
		if _, err := tx.Exec(ctx, x.sql, x.args...); err != nil {
			t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return id
}

// Le partenze senza il motore: vuota (e la sua versione 1 si salva di nuovo con R2: l'identita' registrata e' quella che R2
// verifica), il reinvio riproposto, CHIAVE_RIUSATA, altra_bozza_aperta, la ripartenza (abbandona) con la versione sbagliata,
// con la bozza di un altro prodotto e con quella giusta, registrato sul perimetro della working, conferma senza conferme, il
// prodotto di un'altra RFQ.
func TestV1C3CreaVuotaERegistrata(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	chiave := uuid.New()
	c, err := creaDB(t, p, s, s.altroFinito, chiave, map[string]any{"partenza": "vuota"})
	if err != nil {
		t.Fatalf("crea vuota: %v", err)
	}
	if c.Versione != 1 || !c.NuovaVersione || c.Riproposto || c.Partenza != PartenzaVuota || c.Eventi != 1 {
		t.Fatalf("esito: %+v", c)
	}
	snap, err := LeggiSnapshot3(c.Snapshot)
	if err != nil || len(snap.Nodi) != 1 || *snap.Nodi[0].ComponenteID != s.altroFinito || snap.Nodi[0].Identita.Modo != ModoRegistrato ||
		snap.Nodi[0].Identita.Codice != "ACM-6000" || len(snap.DocumentiPreassociati) != 0 {
		t.Fatalf("snapshot: %v %+v", err, snap)
	}
	ee := eventiDB(t, p, c.BozzaID, 1)
	if len(ee) != 1 || ee[0].Azione != AzBozzaAperta || ee[0].Origine != "server" || campoJSON(t, ee[0].Dopo, "partenza") != `"vuota"` ||
		campoJSON(t, ee[0].Dopo, "algoritmo") != `"`+AlgoritmoVuota+`"` {
		t.Fatalf("evento di apertura: %+v", ee)
	}
	if n := conta3(t, p, `SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1 AND chiave_salvataggio = $2 AND importazione_id IS NULL`,
		c.BozzaID, chiave); n != 1 {
		t.Fatalf("versione 1 con la chiave: %d", n)
	}
	// la versione 1 si salva di nuovo: il nodo del prodotto cambiato passa la verifica dell'identita' registrata
	var m map[string]any
	if err := json.Unmarshal(c.Snapshot, &m); err != nil {
		t.Fatal(err)
	}
	nodo := m["nodi"].([]any)[0].(map[string]any)
	nodo["descrizione"] = "Gruppo di prova"
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, c.BozzaID, s.altroFinito, 1, uuid.New(), m, []Azione{{Azione: AzNodoModificato, ChiaveBozza: nodo["chiave_bozza"].(string),
		Prima: json.RawMessage(`{"descrizione":null}`), Dopo: json.RawMessage(`{"descrizione":"Gruppo di prova"}`), Origine: "utente"}}); err != nil {
		t.Fatalf("salvataggio dopo la creazione: %v", err)
	}
	// il reinvio con la stessa chiave e lo stesso corpo: riproposto, senza scritture
	versioni := conta3(t, p, `SELECT count(*) FROM distinta_bozza_versione`)
	c2, err := creaDB(t, p, s, s.altroFinito, chiave, map[string]any{"partenza": "vuota"})
	if err != nil || !c2.Riproposto || c2.BozzaID != c.BozzaID || c2.Versione != 1 || c2.Partenza != PartenzaVuota ||
		string(canonico(c2.Snapshot)) != string(canonico(c.Snapshot)) {
		t.Fatalf("reinvio: %v %+v", err, c2)
	}
	if conta3(t, p, `SELECT count(*) FROM distinta_bozza_versione`) != versioni {
		t.Fatal("il reinvio ha scritto")
	}
	_, err = creaDB(t, p, s, s.altroFinito, chiave, map[string]any{"partenza": "registrato"})
	codice3(t, err, CodiceChiaveRiusata, "")
	_, err = creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": "vuota"})
	codice3(t, err, CodiceBozzaVersione, MotivoAltraBozzaAperta)
	// la ripartenza: con la versione superata BOZZA_VERSIONE, niente scritto; con la bozza di un altro prodotto
	// BOZZA_ALTRO_PRODOTTO; con quella giusta la bozza di prima si chiude
	_, err = creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": "registrato",
		"abbandona": map[string]any{"bozza_id": c.BozzaID, "versione_attesa": 1, "motivo": "si riparte"}})
	codice3(t, err, CodiceBozzaVersione, MotivoVersioneDiversa)
	_, err = creaDB(t, p, s, s.prodotto, uuid.New(), map[string]any{"partenza": "registrato",
		"abbandona": map[string]any{"bozza_id": c.BozzaID, "versione_attesa": 2, "motivo": "si riparte"}})
	codice3(t, err, CodiceBozzaAltroProdotto, "")
	r, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": "registrato",
		"abbandona": map[string]any{"bozza_id": c.BozzaID, "versione_attesa": 2, "motivo": "si riparte da capo"}})
	if err != nil || r.BozzaAbbandonata == nil || *r.BozzaAbbandonata != c.BozzaID || r.Partenza != PartenzaRegistrato {
		t.Fatalf("ripartenza: %v %+v", err, r)
	}
	var stato, chiusura string
	if err := p.QueryRow(ctx, `SELECT stato::text, motivo_chiusura FROM distinta_bozza WHERE bozza_id = $1`, c.BozzaID).Scan(&stato, &chiusura); err != nil {
		t.Fatal(err)
	}
	if stato != StatoAbbandonata || chiusura != "ripartita (partenza registrato) nella bozza "+r.BozzaID.String() {
		t.Fatalf("la bozza di prima: %s %q", stato, chiusura)
	}
	if e := eventiDB(t, p, r.BozzaID, 1); len(e) != 1 || e[0].Motivo != "si riparte da capo" || campoJSON(t, e[0].Dopo, "da_bozza") == "null" {
		t.Fatalf("l'evento della ripartenza: %+v", e)
	}
	// registrato sul prodotto della scena: il perimetro della working, 35 nodi e 34 archi, tutti registrati
	g, err := creaDB(t, p, s, s.prodotto, uuid.New(), map[string]any{"partenza": "registrato"})
	if err != nil {
		t.Fatalf("crea registrato: %v", err)
	}
	gs, err := LeggiSnapshot3(g.Snapshot)
	if err != nil || len(gs.Nodi) != 35 || len(gs.Archi) != 34 {
		t.Fatalf("registrato: %v %d nodi %d archi", err, len(gs.Nodi), len(gs.Archi))
	}
	for _, n := range gs.Nodi {
		if n.Identita.Modo != ModoRegistrato || n.Op != OpRiusa || !n.Identita.EmittentePresunto || n.Identita.Emittente != EmittenteCliente(idDi("cliente")) {
			t.Fatalf("nodo %+v", n)
		}
	}
	// conferma senza conferme: partenza_non_ammessa (sul prodotto della seconda RFQ, che non ha bozze)
	_, err = CreaFormato3(ctx, p, Creazione3{ThreadID: s.thread2, UtenteID: s.utente, Comune: ComuneR2{ProdottoID: s.prodottoT2,
		ChiaveSalvataggio: uuid.New(), ImprontaRichiesta: strings.Repeat("5", 64)}, Crea: CreaR2{Partenza: PartenzaConferma}})
	codice3(t, err, CodiceCorpoNonValido, MotivoPartenzaNonAmmessa)
	// un prodotto di un'altra RFQ
	_, err = creaDB(t, p, s, s.prodottoT2, uuid.New(), map[string]any{"partenza": "vuota"})
	codice3(t, err, CodiceFuoriRFQ, "")
}

// da-conferma-1 e PD-02 su una conferma formato 3 valida: registrato rifiutato (partenza_non_ammessa), conferma con le chiavi,
// le caselle e le decisioni della conferma; dopo un cambio della working la conferma non vale piu': conferma rifiutata,
// registrato ammesso. Una conferma formato 2 parte con le caselle completate come nella conversione.
func TestV1C3CreaDaConferma(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	// una bozza chiusa del prodotto per la conferma
	b0, err := creaDB(t, p, s, s.prodotto, uuid.New(), map[string]any{"partenza": "vuota"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Abbandona(ctx, p, Abbandono{ThreadID: s.thread, BozzaID: b0.BozzaID, UtenteID: s.utente, VersioneAttesa: 1}); err != nil {
		t.Fatal(err)
	}
	// lo snapshot della conferma: il perimetro della working, con chiavi proprie, la casella della radice e di k-s1, una
	// conferma_categoria e un mono_pezzo (e una decisione che da-conferma-1 non riporta)
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	in, err := ingressoPartenza(ctx, db.New(tx), s.thread, s.prodotto)
	_ = tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reg, _, err := PartenzaRegistrata(PartenzaRegistrato, in)
	if err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("b", 64)
	fonte := &Sorgente3{AllegatoID: pu(s.allegato), Sha256: sha, Gruppo: "bom"}
	reg.Sorgente = fonte
	mappa := map[string]uuid.UUID{}
	rinomina := map[string]string{}
	for i, n := range reg.Nodi {
		k := "conf-" + n.ChiaveBozza[len("k-c-"):len("k-c-")+8]
		switch *n.ComponenteID {
		case s.prodotto:
			k = "k-root"
			reg.Nodi[i].Caselle = []Casella3{{AllegatoID: s.allegato, Gruppo: "bom", Rif: "nodo:" + sha + ":#1"}}
		case s.sc.componenti["k-s1"]:
			k = "k-s1"
			reg.Nodi[i].Caselle = []Casella3{{AllegatoID: s.allegato, Gruppo: "bom", Rif: "nodo:" + sha + ":#7"}}
			reg.Nodi[i].Origine = OrigineStep
		}
		rinomina[n.ChiaveBozza] = k
		reg.Nodi[i].ChiaveBozza = k
		mappa[k] = *n.ComponenteID
	}
	for i := range reg.Archi {
		reg.Archi[i].Padre, reg.Archi[i].Figlio = rinomina[reg.Archi[i].Padre], rinomina[reg.Archi[i].Figlio]
	}
	reg.Decisioni = []Decisione3{{ID: "mp-1", Tipo: DecisioneMonoPezzo, Motivo: "pezzo unico"},
		{ID: "cc-1", Tipo: DecisioneConfermaCategoria, Soggetto: &SoggettoCategoria{ChiaveBozza: "k-s1"}, Categoria: "fabbricato",
			Esito: EsitoCategoriaConferma, Motivo: "dal disegno", OrigineCategoria: OrigineCategoriaManuale, Ingresso: IngressoEditor},
		{ID: "dv-1", Tipo: DecisioneDaValutare, ChiaveBozza: "k-s1"}}
	conf := confermaC3DB(t, p, s, s.prodotto, b0.BozzaID, 1, nil, reg, mappa, fonte)
	// PD-02: registrato su un prodotto con una conferma formato 3 valida
	_, err = creaDB(t, p, s, s.prodotto, uuid.New(), map[string]any{"partenza": "registrato"})
	codice3(t, err, CodiceCorpoNonValido, MotivoPartenzaNonAmmessa)
	// direttiva 39, R39-19: anche vuota (la strada e' da-conferma-1)
	_, err = creaDB(t, p, s, s.prodotto, uuid.New(), map[string]any{"partenza": "vuota"})
	codice3(t, err, CodiceCorpoNonValido, MotivoPartenzaNonAmmessa)
	// da-conferma-1
	c, err := creaDB(t, p, s, s.prodotto, uuid.New(), map[string]any{"partenza": "conferma"})
	if err != nil {
		t.Fatalf("crea conferma: %v", err)
	}
	if c.DaConferma == nil || c.DaConferma.ConfermaID != conf || c.DaConferma.Numero != 1 || c.Partenza != PartenzaConferma {
		t.Fatalf("da_conferma: %+v", c)
	}
	snap, err := LeggiSnapshot3(c.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Nodi) != 35 || len(snap.Archi) != 34 || snap.Sorgente == nil || snap.Sorgente.Importazione != "" || snap.Sorgente.Gruppo != "bom" {
		t.Fatalf("snapshot da-conferma-1: %d %d %+v", len(snap.Nodi), len(snap.Archi), snap.Sorgente)
	}
	trovati := 0
	for _, n := range snap.Nodi {
		if n.ChiaveBozza == "k-root" || n.ChiaveBozza == "k-s1" {
			trovati++
			if len(n.Caselle) != 1 {
				t.Errorf("caselle di %s: %+v", n.ChiaveBozza, n.Caselle)
			}
		}
		if mappa[n.ChiaveBozza] != *n.ComponenteID || n.Identita.Modo != ModoRegistrato {
			t.Errorf("nodo %+v", n)
		}
	}
	if trovati != 2 || len(snap.Decisioni) != 2 {
		t.Fatalf("chiavi e decisioni della conferma: %d %+v", trovati, snap.Decisioni)
	}
	for _, d := range snap.Decisioni {
		if d.Tipo == DecisioneConfermaCategoria && (d.DaConferma == nil || d.DaConferma.ConfermaID != conf || d.DaConferma.DecisioneID != "cc-1") {
			t.Errorf("conferma_categoria senza da_conferma: %+v", d)
		}
	}
	if e := eventiDB(t, p, c.BozzaID, 1); len(e) != 1 || campoJSON(t, e[0].Dopo, "algoritmo") != `"`+AlgoritmoDaConferma+`"` ||
		!strings.Contains(campoJSON(t, e[0].Dopo, "da_conferma"), conf.String()) {
		t.Fatalf("evento: %+v", e)
	}
	// la working cambia (una quantita'): la conferma non vale piu'
	if _, err := Abbandona(ctx, p, Abbandono{ThreadID: s.thread, BozzaID: c.BozzaID, UtenteID: s.utente, VersioneAttesa: 1}); err != nil {
		t.Fatal(err)
	}
	esegui3(t, p, `UPDATE componente_relazione SET qta = qta + 1 WHERE padre_id = $1 AND figlio_id = $2`, s.prodotto, s.sc.componenti["k-s1"])
	_, err = creaDB(t, p, s, s.prodotto, uuid.New(), map[string]any{"partenza": "conferma"})
	codice3(t, err, CodiceCorpoNonValido, MotivoPartenzaNonAmmessa)
	if _, err := creaDB(t, p, s, s.prodotto, uuid.New(), map[string]any{"partenza": "registrato"}); err != nil {
		t.Fatalf("registrato dopo la conferma superata: %v", err)
	}
	// una conferma formato 2 (l'altro finito): la casella si completa con l'allegato della fonte; il nodo registrato
	sha2 := strings.Repeat("b", 64)
	s2 := soloRadice(s.altroFinito, "ACM-6000")
	s2.Sorgente = &Sorgente{AllegatoID: pu(s.allegato), Sha256: sha2, Gruppo: "bom"}
	s2.Nodi[0].Caselle = []Casella{{Gruppo: "bom", Rif: "nodo:" + sha2 + ":#1"}}
	s2.Decisioni = []json.RawMessage{json.RawMessage(`{"id":"mp-2","tipo":"mono_pezzo","motivo":"pezzo unico"}`)}
	l2, err := LeggiSnapshot(mustJSON(s2))
	if err != nil {
		t.Fatal(err)
	}
	f2, err := l2.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	b2, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": "vuota"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Abbandona(ctx, p, Abbandono{ThreadID: s.thread, BozzaID: b2.BozzaID, UtenteID: s.utente, VersioneAttesa: 1}); err != nil {
		t.Fatal(err)
	}
	tx2, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx2.Rollback(ctx) //nolint:errcheck
	for _, x := range []struct {
		sql  string
		args []any
	}{
		{`ALTER TABLE conferma_strutturale DISABLE TRIGGER USER`, nil},
		{`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura,
				firma_anteprima, base_contatore, firma_base, contatore, contatore_dati, bozza_id, bozza_versione, fonte_allegato_id, fonte_sha256,
				fonte_gruppo, origine, snapshot, impronta_snapshot, delta, mappa, criterio_verifica, chiave_idempotenza, confermato_da, mono_pezzo,
				mono_pezzo_motivo)
			VALUES (gen_random_uuid(), $1, $2, 1, 2, false, repeat('3', 64), repeat('2', 64), 0, repeat('1', 64),
				(SELECT COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = $1), 0)), 0, $3, 1, $4, $5, 'bom', '{"rotta":"prova"}',
				$6, $7, '{}', $8, 'mono_pezzo', gen_random_uuid(), $9, true, 'pezzo unico')`,
			[]any{s.thread, s.altroFinito, b2.BozzaID, s.allegato, sha2, f2, jsoncanonico.Impronta(f2), mustJSON(map[string]uuid.UUID{"r": s.altroFinito}), s.utente}},
		{`ALTER TABLE conferma_strutturale ENABLE TRIGGER USER`, nil},
	} {
		if _, err := tx2.Exec(ctx, x.sql, x.args...); err != nil {
			t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	x, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": "conferma"})
	if err != nil {
		t.Fatalf("da-conferma-1 da una conferma formato 2: %v", err)
	}
	xs, err := LeggiSnapshot3(x.Snapshot)
	if err != nil || len(xs.Nodi) != 1 || xs.Nodi[0].ChiaveBozza != "r" || len(xs.Nodi[0].Caselle) != 1 || xs.Nodi[0].Caselle[0].AllegatoID != s.allegato ||
		xs.Nodi[0].Identita.Modo != ModoRegistrato || len(xs.Decisioni) != 1 || xs.Decisioni[0].ID != "mp-2" {
		t.Fatalf("da-conferma-1 dal formato 2: %v %+v", err, xs)
	}
}
