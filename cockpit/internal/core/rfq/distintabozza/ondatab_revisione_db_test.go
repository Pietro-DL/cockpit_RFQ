//go:build integrazione

// L4 — V1, ondata B, corsia persistenza, secondo passaggio: le prove dei rilievi della revisione (resoconto della corsia,
// §9) sul database di prova, nel package delle rotte:
//   - il giornale rifa' la versione di un nodo_modificato (T-10), e il campo del dopo che il prima non nomina si rifiuta;
//   - gli archi step nuovi o cambiati contro il registro della versione e, senza registro, contro i fatti correnti (EM §3.5);
//   - la non_includere nuova senza importazione contro il registro della versione (EM §7.1, V-3, V-5);
//   - il modo dei nodi: da_decidere su una voce con il codice proposto, l'importazione a mano con le regole P-2...P-6;
//   - la chiave degli eventi di R1 ?eventi=1 si chiama «chiave» (EM §9.1, §9.2);
//   - gli UUID e i registri che compaiono solo nelle azioni (EM §9, «ovunque sia annidato»);
//   - R-19 (componente e componente_relazione identici dopo R1, R2, R3, R6, R7), R-04 (il reinvio dopo che la versione
//     e' avanzata), R-16 (registrato_senza_esito dalla conferma, dall'evento dei file, dall'associazione; stati_correnti),
//     R-22 (il messaggio spostato a un'altra RFQ);
//   - conversione-1: le fonti secondarie per chiave e nell'ordine (scelta e rifiuto, la sostituzione, la chiave che non e'
//     piu' un nodo), le decisioni e le preassociazioni copiate, i rifiuti con i motivi propri, la traccia oltre il limite.
// Solo dati sintetici: la scena di persistenza_db_test.go e gli aiuti di formato3_aiuti_test.go e ondatab_db_test.go.

package distintabozza

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// fattiStep3Rel: i fatti correnti di uno STEP con le chiavi PRODUCT e le relazioni (padre, figlio) date.
func fattiStep3Rel(t *testing.T, chiavi []string, relazioni [][2]string) string {
	t.Helper()
	nodi := make([]map[string]any, 0, len(chiavi))
	for _, k := range chiavi {
		nodi = append(nodi, map[string]any{"chiave": k})
	}
	rel := make([]map[string]any, 0, len(relazioni))
	for _, r := range relazioni {
		rel = append(rel, map[string]any{"padre": r[0], "figlio": r[1], "qta": 1})
	}
	b, err := json.Marshal(map[string]any{"struttura": map[string]any{"versione": 3, "radici": []string{chiavi[0]}, "nodi": nodi,
		"relazioni": rel, "avvisi": []any{}, "limiti": map[string]any{"troncato": false},
		"scarti": map[string]any{"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// bozza3Diretta scrive una bozza formato 3 aperta del prodotto con lo snapshot dato alla versione 1 (senza registro se la
// sorgente non ne cita uno), con l'evento di apertura del server.
func bozza3Diretta(t *testing.T, p *pgxpool.Pool, thread, prodotto, utente uuid.UUID, snap map[string]any) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	s := leggiProva3(t, snap)
	can, err := s.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	fonte, sha, gruppo := colonneFonte3(s.Sorgente)
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	q := db.New(tx)
	b, err := q.InsertDistintaBozza(ctx, db.InsertDistintaBozzaParams{ThreadID: thread, ProdottoID: prodotto, Formato: FormatoSnapshot3,
		FirmaBase: firmaProva, FonteAllegatoID: fonte, FonteSha256: sha, FonteGruppo: gruppo, UtenteID: utente})
	if err != nil {
		t.Fatalf("bozza: %v", err)
	}
	if _, err := q.InsertDistintaBozzaVersione(ctx, db.InsertDistintaBozzaVersioneParams{BozzaID: b.BozzaID, Versione: 1, Snapshot: can,
		Impronta: jsoncanonico.Impronta(can), FirmaBase: firmaProva, FonteAllegatoID: fonte, FonteSha256: sha, FonteGruppo: gruppo, SalvataDa: utente,
		ChiaveSalvataggio: uuid.NullUUID{UUID: uuid.New(), Valid: true}, ImprontaRichiesta: pgtype.Text{String: strings.Repeat("4", 64), Valid: true}}); err != nil {
		t.Fatalf("versione 1: %v", err)
	}
	if err := q.InsertDistintaBozzaEvento(ctx, db.InsertDistintaBozzaEventoParams{BozzaID: b.BozzaID, N: 1, Versione: 1,
		Vocabolario: int16(VocabolarioFormato3), Azione: AzBozzaAperta, Origine: "server", UtenteID: utente}); err != nil {
		t.Fatalf("apertura: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return b.BozzaID
}

func senzaArco(t *testing.T, s map[string]any, padre, figlio string) map[string]any {
	t.Helper()
	c := copiaProfonda(t, s)
	var archi []any
	for _, a := range c["archi"].([]any) {
		if m := a.(map[string]any); m["padre"] != padre || m["figlio"] != figlio {
			archi = append(archi, a)
		}
	}
	c["archi"] = archi
	return c
}

func senzaDecisione(t *testing.T, s map[string]any, id string) map[string]any {
	t.Helper()
	c := copiaProfonda(t, s)
	var dd []any
	for _, d := range c["decisioni"].([]any) {
		if d.(map[string]any)["id"] != id {
			dd = append(dd, d)
		}
	}
	c["decisioni"] = dd
	return c
}

// ------------------------------------------------------------------ il giornale e nodo_modificato (rilievo 1)

func TestV1Rev2NodoModificatoEIlGiornale(t *testing.T) {
	sc := preparaBozza3(t)
	ctx := context.Background()
	v2 := copiaProfonda(t, sc.base)
	n := nodoIn3(v2, "k-man")
	n["tipo"], n["descrizione"] = "sciolto", "Vite"
	r, err := sc.salva3(t, 1, uuid.New(), v2, []Azione{azione3(t, AzNodoModificato, "k-man",
		map[string]any{"tipo": "commerciale", "descrizione": nil}, map[string]any{"tipo": "sciolto", "descrizione": "Vite"})})
	if err != nil || r.Versione != 2 {
		t.Fatalf("salvataggio: %+v %v", r, err)
	}
	es, err := VerificaGiornale3(ctx, db.New(sc.p), sc.thread, sc.bozza)
	if err != nil || len(es.Verificate) != 1 || es.Verificate[0] != 2 {
		t.Fatalf("il giornale non rifa' la versione 2: %+v %v", es, err)
	}
	// il caso del revisore: il dopo nomina la descrizione, il prima no
	v3 := copiaProfonda(t, v2)
	n = nodoIn3(v3, "k-man")
	n["tipo"], n["descrizione"] = "commerciale", "Vite e dado"
	_, err = sc.salva3(t, 2, uuid.New(), v3, []Azione{azione3(t, AzNodoModificato, "k-man", map[string]any{"tipo": "sciolto"},
		map[string]any{"tipo": "commerciale", "descrizione": "Vite e dado"})})
	codice3(t, err, CodiceCorpoNonValido, "")
	if n := conta3(t, sc.p, `SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1`, sc.bozza); n != 2 {
		t.Fatalf("versioni dopo il rifiuto: %d", n)
	}
}

// ------------------------------------------------------------------ gli archi step (rilievo 2)

func TestV1Rev2ArchiStepControIlRegistroEIFatti(t *testing.T) {
	sc := preparaBozza3(t)
	step := map[string]any{"allegato_id": sc.id.allegato.String(), "gruppo": gruppoProva3}
	// un arco step inventato: il registro ha #1→#7, #7→#9, #7→#11, #7→#31, non #9→#11
	inventato := map[string]any{"padre": "k-0009", "figlio": "k-0011", "qta": 1, "origine": "step", "step": step}
	v := copiaProfonda(t, sc.base)
	v["archi"] = append(v["archi"].([]any), inventato)
	_, err := sc.salva3(t, 1, uuid.New(), v, []Azione{azione3(t, AzArcoAggiunto, "k-0011", nil, inventato)})
	if e := codice3(t, err, CodiceCorpoNonValido, MotivoRifFuoriImportazione); !strings.HasPrefix(e.Campo, "snapshot.archi[") || !strings.HasSuffix(e.Campo, "].step") {
		t.Fatalf("campo: %+v", e)
	}
	// un arco del registro tolto e rimesso: la seconda volta e' nuovo rispetto alla versione, e passa
	arco := map[string]any{"padre": "k-0007", "figlio": "k-0011", "qta": 3, "origine": "step", "step": step}
	if _, err := sc.salva3(t, 1, uuid.New(), senzaArco(t, sc.base, "k-0007", "k-0011"), []Azione{azione3(t, AzArcoTolto, "k-0011", arco, nil)}); err != nil {
		t.Fatalf("arco tolto: %v", err)
	}
	if r, err := sc.salva3(t, 2, uuid.New(), sc.base, []Azione{azione3(t, AzArcoAggiunto, "k-0011", nil, arco)}); err != nil || r.Versione != 3 {
		t.Fatalf("arco del registro rimesso: %+v %v", r, err)
	}

	// senza registro (una bozza convertita, o partita da una conferma): l'arco step si cerca fra le relazioni dei fatti
	// correnti dello STEP
	esegui3(t, sc.p, `UPDATE analisi_fatti SET fatti = $2 WHERE sha256 = $1`, sc.id.sha,
		fattiStep3Rel(t, []string{"#1", "#7", "#9", "#11", "#31"}, [][2]string{{"#1", "#7"}, {"#7", "#9"}}))
	cliente := sc.id.clienteE()
	casella := func(k string) []any {
		return []any{map[string]any{"allegato_id": sc.id.allegato.String(), "gruppo": gruppoProva3, "rif": sc.id.rif(k)}}
	}
	daDecidere := func(chiave, k string) map[string]any {
		return map[string]any{"chiave_bozza": chiave, "op": "crea", "tipo": "sciolto", "origine": "step", "caselle": casella(k),
			"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "modo": "da_decidere"}}
	}
	senza := map[string]any{"formato": 3, "prodotto_id": sc.altroFinito.String(),
		"sorgente":          map[string]any{"allegato_id": sc.id.allegato.String(), "sha256": sc.id.sha, "gruppo": gruppoProva3},
		"versione_bom_base": map[string]any{"conferma_id": nil, "numero": 0, "contatore": 0}, "firma_base": firmaProva,
		"nodi": []any{
			map[string]any{"chiave_bozza": "k-r", "componente_id": sc.altroFinito.String(), "op": "riusa", "tipo": "finito", "origine": "registrato",
				"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": "ACM-6000", "modo": "registrato"}},
			daDecidere("k-a", "#7"), daDecidere("k-b", "#9"), daDecidere("k-c", "#11"),
		},
		"archi": []any{map[string]any{"padre": "k-r", "figlio": "k-a", "qta": 1, "origine": "utente"}}, "decisioni": []any{}}
	b := bozza3Diretta(t, sc.p, sc.thread, sc.altroFinito, sc.utente, senza)
	nei := map[string]any{"padre": "k-a", "figlio": "k-b", "qta": 1, "origine": "step", "step": step}
	v = copiaProfonda(t, senza)
	v["archi"] = append(v["archi"].([]any), nei)
	if r, err := sc.salva3Su(t, b, sc.altroFinito, 1, uuid.New(), v, []Azione{azione3(t, AzArcoAggiunto, "k-b", nil, nei)}); err != nil || r.Versione != 2 {
		t.Fatalf("arco step dei fatti senza registro: %+v %v", r, err)
	}
	fuori := map[string]any{"padre": "k-a", "figlio": "k-c", "qta": 1, "origine": "step", "step": step}
	v2 := copiaProfonda(t, v)
	v2["archi"] = append(v2["archi"].([]any), fuori)
	_, err = sc.salva3Su(t, b, sc.altroFinito, 2, uuid.New(), v2, []Azione{azione3(t, AzArcoAggiunto, "k-c", nil, fuori)})
	codice3(t, err, CodiceCorpoNonValido, MotivoRifFuoriImportazione)
}

// ------------------------------------------------------------------ la non_includere senza importazione (rilievo 3)

func TestV1Rev2NonIncludereSenzaImportazione(t *testing.T) {
	sc := preparaBozza3(t)
	ni := func(id, k string) map[string]any {
		return map[string]any{"id": id, "tipo": "non_includere",
			"casella":       map[string]any{"allegato_id": sc.id.allegato.String(), "gruppo": gruppoProva3, "rif": sc.id.rif(k)},
			"motivo_codice": "altro", "motivo": "prova della revisione", "azione_id": azioneB}
	}
	// #40 non e' nel registro della versione: senza importazione la verifica si fa lo stesso (V-3)
	v := copiaProfonda(t, sc.base)
	v["decisioni"] = append(v["decisioni"].([]any), ni("ni-r40", "#40"))
	_, err := sc.salva3(t, 1, uuid.New(), v, []Azione{azione3(t, AzDecisioneAggiunta, "", nil, ni("ni-r40", "#40"))})
	if e := codice3(t, err, CodiceCorpoNonValido, MotivoRifFuoriImportazione); !strings.HasSuffix(e.Campo, ".casella") {
		t.Fatalf("campo: %+v", e)
	}
	// #12 e' fra i fuori per decisione del registro, non segnalato: passa
	v = copiaProfonda(t, sc.base)
	v["decisioni"] = append(v["decisioni"].([]any), ni("ni-r12", "#12"))
	if r, err := sc.salva3(t, 1, uuid.New(), v, []Azione{azione3(t, AzDecisioneAggiunta, "", nil, ni("ni-r12", "#12"))}); err != nil || r.Versione != 2 {
		t.Fatalf("non_includere di #12 senza importazione: %+v %v", r, err)
	}
	// #31 il registro lo segnala: senza importazione la decisione non puo' portare il segnale visto, e V-5 contro il
	// registro della versione la rifiuta
	w := senzaDecisione(t, v, "ni-0001")
	w["decisioni"] = append(w["decisioni"].([]any), ni("ni-r31", "#31"))
	_, err = sc.salva3(t, 2, uuid.New(), w, []Azione{
		azione3(t, AzDecisioneTolta, "", decisioneIn3(v, "ni-0001"), nil),
		azione3(t, AzDecisioneAggiunta, "", nil, ni("ni-r31", "#31")),
	})
	codice3(t, err, CodiceCorpoNonValido, MotivoSegnaleNonCorrisponde)
}

// ------------------------------------------------------------------ il modo dei nodi (rilievo 6) e la chiave di R1 (rilievo 4)

func TestV1Rev2IlModoDeiNodiELaChiaveDelGiornale(t *testing.T) {
	sc := preparaBozza3(t)
	// k-0007 ha la voce #7 con il codice proposto ACM-1100: da_decidere senza la decisione identita no
	v := copiaProfonda(t, sc.base)
	prima := nodoIn3(sc.base, "k-0007")["identita"]
	dopo := map[string]any{"emittente": sc.id.clienteE(), "emittente_presunto": true, "modo": "da_decidere"}
	nodoIn3(v, "k-0007")["identita"] = dopo
	_, err := sc.salva3(t, 1, uuid.New(), v, []Azione{azione3(t, AzNodoModificato, "k-0007", map[string]any{"identita": prima}, map[string]any{"identita": dopo})})
	if e := codice3(t, err, CodiceCorpoNonValido, MotivoPropostaNonCorrispondente); !strings.HasSuffix(e.Campo, ".identita.modo") {
		t.Fatalf("campo: %+v", e)
	}
	// «Includi di nuovo» del cordone #31 (nessun codice proposto, nessuna decisione esistente): le regole P-2...P-6
	// danno da_decidere
	step := map[string]any{"allegato_id": sc.id.allegato.String(), "gruppo": gruppoProva3}
	casella := []any{map[string]any{"allegato_id": sc.id.allegato.String(), "gruppo": gruppoProva3, "rif": sc.id.rif("#31")}}
	arco := map[string]any{"padre": "k-0007", "figlio": "k-0031", "qta": 1, "origine": "step", "step": step}
	includi := func(nodo map[string]any) (map[string]any, []Azione) {
		s := senzaDecisione(t, sc.base, "ni-0001")
		s["nodi"] = append(s["nodi"].([]any), nodo)
		s["archi"] = append(s["archi"].([]any), arco)
		return s, []Azione{
			azione3(t, AzDecisioneTolta, "", decisioneIn3(sc.base, "ni-0001"), nil),
			azione3(t, AzNodoAggiunto, "k-0031", nil, nodo),
			azione3(t, AzArcoAggiunto, "k-0031", nil, arco),
		}
	}
	registrato := map[string]any{"chiave_bozza": "k-0031", "componente_id": idDi("k-p1-1").String(), "op": "riusa", "tipo": "sciolto", "origine": "step",
		"caselle": casella, "identita": map[string]any{"emittente": sc.id.clienteE(), "emittente_presunto": true, "codice": "ACM-1101", "modo": "registrato"}}
	s, aa := includi(registrato)
	_, err = sc.salva3(t, 1, uuid.New(), s, aa)
	codice3(t, err, CodiceCorpoNonValido, MotivoPropostaNonCorrispondente)
	daDecidere := map[string]any{"chiave_bozza": "k-0031", "op": "crea", "tipo": "sciolto", "origine": "step", "caselle": casella,
		"identita": map[string]any{"emittente": sc.id.clienteE(), "emittente_presunto": true, "modo": "da_decidere"}}
	s, aa = includi(daDecidere)
	if r, err := sc.salva3(t, 1, uuid.New(), s, aa); err != nil || r.Versione != 2 {
		t.Fatalf("«Includi di nuovo» da decidere: %+v %v", r, err)
	}
	// R1 ?eventi=1: la chiave dell'evento si chiama «chiave» (EM §9.1, §9.2; RIC-P6), mai «chiave_bozza» al primo livello
	l := sc.leggiR1(t, OpzioniR1{Eventi: true})
	b, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	var corpo struct {
		Eventi []map[string]json.RawMessage `json:"eventi"`
	}
	if err := json.Unmarshal(b, &corpo); err != nil || len(corpo.Eventi) != 4 {
		t.Fatalf("eventi di R1: %v %s", err, b)
	}
	chiavi := []string{}
	for _, e := range corpo.Eventi {
		if _, ok := e["chiave_bozza"]; ok {
			t.Errorf("un evento di R1 ha chiave_bozza: %v", e)
		}
		var k string
		if raw, ok := e["chiave"]; ok {
			_ = json.Unmarshal(raw, &k)
		}
		chiavi = append(chiavi, k)
		if _, ok := e["autore"]; !ok {
			t.Errorf("un evento di R1 senza autore: %v", e)
		}
	}
	if strings.Join(chiavi, ",") != ",,k-0031,k-0031" {
		t.Fatalf("chiavi degli eventi di R1: %v", chiavi)
	}
}

// ------------------------------------------------------------------ i riferimenti delle azioni (rilievo 11)

func TestV1Rev2RiferimentiDelleAzioni(t *testing.T) {
	sc := preparaBozza3(t)
	// un nodo del finito di un'altra RFQ, aggiunto e tolto nello stesso salvataggio
	n := map[string]any{"chiave_bozza": "k-t1", "componente_id": sc.prodottoT2.String(), "op": "riusa", "tipo": "sciolto", "origine": "registrato",
		"identita": map[string]any{"emittente": sc.id.clienteE(), "emittente_presunto": true, "codice": "ACM-7000", "modo": "registrato"}}
	_, err := sc.salva3(t, 1, uuid.New(), sc.base, []Azione{azione3(t, AzNodoAggiunto, "k-t1", nil, n), azione3(t, AzNodoTolto, "k-t1", n, nil)})
	if e := codice3(t, err, CodiceFuoriRFQ, ""); e.Campo != "azioni[0].dopo.componente_id" {
		t.Fatalf("campo: %+v", e)
	}
	// una non_includere su un allegato di un'altra RFQ, aggiunta e tolta
	estranea := map[string]any{"id": "ni-t2", "tipo": "non_includere",
		"casella":       map[string]any{"allegato_id": sc.allegatoT2.String(), "gruppo": gruppoProva3, "rif": "nodo:" + strings.Repeat("e", 64) + ":#1"},
		"motivo_codice": "altro", "motivo": "prova", "azione_id": azioneB}
	_, err = sc.salva3(t, 1, uuid.New(), sc.base, []Azione{azione3(t, AzDecisioneAggiunta, "", nil, estranea), azione3(t, AzDecisioneTolta, "", estranea, nil)})
	codice3(t, err, CodiceFuoriRFQ, "")
	// una decisione che cita un registro che non c'e', aggiunta e tolta
	sconosciuto := map[string]any{"id": "ni-t3", "tipo": "non_includere",
		"casella":       map[string]any{"allegato_id": sc.id.allegato.String(), "gruppo": gruppoProva3, "rif": sc.id.rif("#12")},
		"motivo_codice": "altro", "motivo": "prova", "azione_id": azioneB, "importazione": strings.Repeat("e", 64)}
	_, err = sc.salva3(t, 1, uuid.New(), sc.base, []Azione{azione3(t, AzDecisioneAggiunta, "", nil, sconosciuto), azione3(t, AzDecisioneTolta, "", sconosciuto, nil)})
	codice3(t, err, CodiceCorpoNonValido, MotivoImportazioneSconosciuta)
	if n := conta3(t, sc.p, `SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1`, sc.bozza); n != 1 {
		t.Fatalf("versioni dopo i rifiuti: %d", n)
	}
}

// ------------------------------------------------------------------ R-19, R-04

// fotoStruttura: componente e componente_relazione della scena, riga per riga.
func fotoStruttura(t *testing.T, p *pgxpool.Pool) string {
	t.Helper()
	var a, b string
	if err := p.QueryRow(context.Background(), `SELECT coalesce(string_agg(c::text, '|' ORDER BY c.componente_id), '') FROM componente c`).Scan(&a); err != nil {
		t.Fatal(err)
	}
	if err := p.QueryRow(context.Background(), `SELECT coalesce(string_agg(r::text, '|' ORDER BY r.padre_id, r.figlio_id), '') FROM componente_relazione r`).Scan(&b); err != nil {
		t.Fatal(err)
	}
	return a + "\n" + b
}

func TestV1Rev2R19NessunaModificaStrutturale(t *testing.T) {
	sc := preparaBozza3(t)
	ctx := context.Background()
	prima := fotoStruttura(t, sc.p)
	v2 := copiaProfonda(t, sc.base)
	nodoIn3(v2, "k-man")["descrizione"] = "Vite di prova"
	if _, err := sc.salva3(t, 1, uuid.New(), v2, []Azione{azione3(t, AzNodoModificato, "k-man", map[string]any{"descrizione": nil},
		map[string]any{"descrizione": "Vite di prova"})}); err != nil {
		t.Fatal(err)
	}
	sc.leggiR1(t, OpzioniR1{Eventi: true, Importazione: true})
	if _, err := LeggiIntento(ctx, sc.p, sc.thread, uuid.New()); err != nil {
		t.Fatal(err)
	}
	tx, err := sc.p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LeggiStorico(ctx, db.New(tx), sc.thread, sc.prodotto, OpzioniR7{}); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(ctx)
	a, err := LeggiRichiestaR3([]byte(`{"versione_attesa":2,"motivo":"prova di R-19"}`))
	if err != nil {
		t.Fatal(err)
	}
	a.ThreadID, a.BozzaID, a.UtenteID = sc.thread, sc.bozza, sc.utente
	if _, err := Abbandona(ctx, sc.p, a); err != nil {
		t.Fatal(err)
	}
	if dopo := fotoStruttura(t, sc.p); dopo != prima {
		t.Fatal("componente o componente_relazione sono cambiati dopo R1, R2, R3, R6, R7")
	}
}

func TestV1Rev2R04ReinvioDopoCheLaVersioneEAvanzata(t *testing.T) {
	sc := preparaBozza3(t)
	modifica := func(descr string, prima any) (map[string]any, []Azione) {
		s := copiaProfonda(t, sc.base)
		nodoIn3(s, "k-man")["descrizione"] = descr
		return s, []Azione{azione3(t, AzNodoModificato, "k-man", map[string]any{"descrizione": prima}, map[string]any{"descrizione": descr})}
	}
	k1 := uuid.New()
	s2, a2 := modifica("Vite A", nil)
	r2, err := sc.salva3(t, 1, k1, s2, a2)
	if err != nil || r2.Versione != 2 {
		t.Fatalf("versione 2: %+v %v", r2, err)
	}
	s3, a3 := modifica("Vite B", "Vite A")
	if r3, err := sc.salva3(t, 2, uuid.New(), s3, a3); err != nil || r3.Versione != 3 {
		t.Fatalf("versione 3: %+v %v", r3, err)
	}
	// la risposta del primo salvataggio si e' persa: il reinvio con la stessa chiave e lo stesso corpo, quando la bozza e'
	// gia' alla versione 3, e' riproposto con la versione 2, non 409 BOZZA_VERSIONE
	r, err := sc.salva3(t, 1, k1, s2, a2)
	if err != nil || !r.Riproposto || r.NuovaVersione || r.Versione != 2 || r.Impronta != r2.Impronta {
		t.Fatalf("reinvio: %+v %v", r, err)
	}
	if n := conta3(t, sc.p, `SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1`, sc.bozza); n != 3 {
		t.Fatalf("il reinvio ha scritto: %d versioni", n)
	}
}

// ------------------------------------------------------------------ R-16

// fileConChiave mette un'occorrenza in Altri file come inAltriFile, con la chiave d'intento sull'evento del file.
func fileConChiave(t *testing.T, p *pgxpool.Pool, allegato, utente, chiave uuid.UUID) {
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
		{`INSERT INTO file_rfq_evento (thread_id, allegato_id, azione, stato_prima, stato_dopo, fotografia, dipendenze, origine, chiave_idempotenza, autore_id)
			SELECT m.thread_id, a.allegato_id, 'altri_file', 'attivo', 'altri_file', '{}', '{}', 'ponte', $3, $2
			  FROM allegato a JOIN messaggio m ON m.messaggio_id = a.messaggio_id WHERE a.allegato_id = $1`, []any{allegato, utente, chiave}},
	} {
		if _, err := tx.Exec(ctx, x.sql, x.args...); err != nil {
			t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Altri file: %v", err)
	}
}

func TestV1Rev2R16RegistratoSenzaEsito(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	// la conferma con la chiave, l'intento oltre la conservazione: registrato_senza_esito con la conferma
	kc := uuid.New()
	s2, err := LeggiSnapshot(mustJSON(soloRadice(s.prodotto, "ACM-1000")))
	if err != nil {
		t.Fatal(err)
	}
	f2, err := s2.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	conf := uuid.New()
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []struct {
		sql  string
		args []any
	}{
		{`ALTER TABLE conferma_strutturale DISABLE TRIGGER USER`, nil},
		{`INSERT INTO conferma_strutturale (conferma_id, thread_id, prodotto_id, numero, formato, senza_riepilogo_v2, firma_struttura,
				firma_anteprima, base_contatore, firma_base, contatore, contatore_dati, origine, snapshot, impronta_snapshot, delta, mappa,
				criterio_verifica, chiave_idempotenza, confermato_da)
			VALUES ($1, $2, $3, 1, 2, false, repeat('3', 64), repeat('2', 64), 0, repeat('1', 64), 0, 0, '{"rotta":"prova"}',
				$4, $5, '{}', '{}', 'nessuno', $6, $7)`, []any{conf, s.thread, s.prodotto, f2, jsoncanonico.Impronta(f2), kc, s.utente}},
		{`ALTER TABLE conferma_strutturale ENABLE TRIGGER USER`, nil},
	} {
		if _, err := tx.Exec(ctx, x.sql, x.args...); err != nil {
			t.Fatalf("%v\n%s", err, x.sql)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	e, err := LeggiIntento(ctx, p, s.thread, kc)
	if err != nil || e.Stato != IntentoRegistratoSenzaEsito || e.ConfermaID == nil || *e.ConfermaID != conf || e.NumeroConferma != 1 || e.Formato != 2 ||
		e.Gesto != "conferma_bom_v2" {
		t.Fatalf("dalla conferma: %+v %v", e, err)
	}
	// l'evento del file con la chiave: registrato_senza_esito con gli stati correnti dell'occorrenza
	kf := uuid.New()
	fileConChiave(t, p, s.allegato2, s.utente, kf)
	e, err = LeggiIntento(ctx, p, s.thread, kf)
	if err != nil || e.Stato != IntentoRegistratoSenzaEsito || e.StatiCorrenti == nil || e.StatiCorrenti.AllegatoID != s.allegato2 ||
		e.StatiCorrenti.Stato != "altri_file" || e.StatiCorrenti.Conservazione != "copiata" {
		t.Fatalf("dall'evento del file: %+v %v", e, err)
	}
	// lo stesso gesto con l'esito ancora conservato: registrato, con gli stati correnti
	esegui3(t, p, `INSERT INTO esito_intento (chiave, thread_id, gesto, utente_id, impronta_corpo, esito)
		VALUES ($1, $2, 'altri_file', $3, repeat('a', 64), '{"esito":"ok"}')`, kf, s.thread, s.utente)
	e, err = LeggiIntento(ctx, p, s.thread, kf)
	if err != nil || e.Stato != IntentoRegistrato || e.Gesto != "altri_file" || e.StatiCorrenti == nil || e.StatiCorrenti.Stato != "altri_file" {
		t.Fatalf("registrato con gli stati correnti: %+v %v", e, err)
	}
	// l'evento del gesto associa_step con la chiave (S7: «assegna» della pagina nuova, senza riga della AR):
	// registrato_senza_esito del gesto associa_step
	ka := uuid.New()
	proposta := nuovaProposta(t, p, s.allegato, s.thread, nil)
	esegui3(t, p, `INSERT INTO associazione_evento (thread_id, proposta_id, azione, componente_dopo, origine, chiave_idempotenza, autore_id)
		VALUES ($1, $2, 'assegna', $5, 'ponte', $3, $4)`, s.thread, proposta, ka, s.utente, s.prodotto)
	e, err = LeggiIntento(ctx, p, s.thread, ka)
	if err != nil || e.Stato != IntentoRegistratoSenzaEsito || e.Gesto != "associa_step" {
		t.Fatalf("dall'associazione: %+v %v", e, err)
	}
	// direttiva 35, C-1c: un altro evento di associazione con la chiave (qui «riapri» di un gesto delle proposte) non e'
	// associa_step: la ricerca e' ristretta al gesto
	kr := uuid.New()
	esegui3(t, p, `INSERT INTO associazione_evento (thread_id, proposta_id, azione, origine, chiave_idempotenza, autore_id)
		VALUES ($1, $2, 'riapri', 'ponte', $3, $4)`, s.thread, proposta, kr, s.utente)
	e, err = LeggiIntento(ctx, p, s.thread, kr)
	if err != nil || e.Stato != IntentoNonRegistrato {
		t.Fatalf("un evento che non e' del gesto associa_step: %+v %v", e, err)
	}
	// la chiave di un evento dei file di un'altra RFQ: FUORI_RFQ, mai l'esito di quella RFQ
	kx := uuid.New()
	fileConChiave(t, p, s.allegatoT2, s.utente, kx)
	_, err = LeggiIntento(ctx, p, s.thread, kx)
	codice3(t, err, CodiceFuoriRFQ, "")
}

// ------------------------------------------------------------------ R-22

func TestV1Rev2R22IlMessaggioSpostatoAUnAltraRFQ(t *testing.T) {
	sc := preparaBozza3(t)
	// nessuna strada di V1 sposta un messaggio gia' agganciato: si simula con un UPDATE diretto (PM23 R-22)
	esegui3(t, sc.p, `UPDATE messaggio SET thread_id = $2 WHERE messaggio_id = $1`, idDi("msg-0"), sc.thread2)
	l := sc.leggiR1(t, OpzioniR1{})
	if l.RifScaduti == nil || len(*l.RifScaduti) != 5 {
		t.Fatalf("rif scaduti dopo lo spostamento: %+v", l.RifScaduti)
	}
	for _, r := range *l.RifScaduti {
		if r.Motivo != ScadutoAllegatoFuoriRFQ {
			t.Errorf("rif scaduto: %+v", r)
		}
	}
	if l.Base == nil || l.Base.Stato != BaseSuperata {
		t.Fatalf("base: %+v", l.Base)
	}
	// un salvataggio che porta le caselle gia' salvate passa
	v2 := copiaProfonda(t, sc.base)
	nodoIn3(v2, "k-man")["descrizione"] = "Vite di prova"
	if r, err := sc.salva3(t, 1, uuid.New(), v2, []Azione{azione3(t, AzNodoModificato, "k-man", map[string]any{"descrizione": nil},
		map[string]any{"descrizione": "Vite di prova"})}); err != nil || r.Versione != 2 {
		t.Fatalf("salvataggio con i riferimenti gia' salvati: %+v %v", r, err)
	}
	// una casella nuova su quell'allegato: FUORI_RFQ
	ni := map[string]any{"id": "ni-r22", "tipo": "non_includere",
		"casella":       map[string]any{"allegato_id": sc.id.allegato.String(), "gruppo": gruppoProva3, "rif": sc.id.rif("#12")},
		"motivo_codice": "altro", "motivo": "prova", "azione_id": azioneB}
	v3 := copiaProfonda(t, v2)
	v3["decisioni"] = append(v3["decisioni"].([]any), ni)
	_, err := sc.salva3(t, 2, uuid.New(), v3, []Azione{azione3(t, AzDecisioneAggiunta, "", nil, ni)})
	codice3(t, err, CodiceFuoriRFQ, "")
}

// ------------------------------------------------------------------ conversione-1: le fonti secondarie (rilievo 16)

func TestV1Rev2ConversioneFontiSecondarie(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	shaB, shaC := strings.Repeat("b", 64), strings.Repeat("c", 64)
	casella := func(k string) []Casella { return []Casella{{Gruppo: "bom", Rif: "nodo:" + shaB + ":" + k}} }
	nodoX := Nodo{ChiaveBozza: "k-x", Op: OpCrea, Codice: "ACM-1111", Tipo: "sciolto", Origine: "step", Caselle: casella("#11")}
	arcoX := Arco{Padre: "k-7", Figlio: "k-x", Qta: 1, Origine: "step"}
	vecchio := Snapshot{Formato: FormatoSnapshot, ProdottoID: s.prodotto, VersioneBomBase: &VersioneBomBase{}, FirmaBase: firmaProva,
		Sorgente: &Sorgente{AllegatoID: pu(s.allegato), Sha256: shaB, Gruppo: "bom"},
		Nodi: []Nodo{
			{ChiaveBozza: "k-root", ComponenteID: pu(s.prodotto), Op: OpRiusa, Codice: "ACM-1000", Rev: "A", Tipo: "finito", Origine: "registrato"},
			{ChiaveBozza: "k-7", Op: OpCrea, Codice: "ACM-1100", Tipo: "sottoassieme", Origine: "step", Caselle: casella("#7")},
			{ChiaveBozza: "k-9", Op: OpCrea, Codice: "ACM-1109", Tipo: "sciolto", Origine: "step", Caselle: casella("#9")},
			nodoX,
		},
		Archi: []Arco{{Padre: "k-root", Figlio: "k-7", Qta: 1, Origine: "utente"}, {Padre: "k-7", Figlio: "k-9", Qta: 2, Origine: "step"}, arcoX},
	}
	b2 := bozza2DB(t, p, s.thread, s.prodotto, s.utente, vecchio)
	q := db.New(p)
	fonte := func(chiave, azione string, allegato uuid.UUID, sha, motivo string) int64 {
		t.Helper()
		var m pgtype.Text
		if motivo != "" {
			m = testo(motivo)
		}
		r, err := q.InsertFonteStepEvento(ctx, db.InsertFonteStepEventoParams{ThreadID: s.thread, ProdottoID: s.prodotto, Ruolo: "sottoassieme",
			ChiaveBozza: testo(chiave), Azione: azione, AllegatoID: allegato, Sha256: sha, Gruppo: testo("bom"), BozzaID: uuid.NullUUID{UUID: b2, Valid: true},
			Motivo: m, AutoreID: s.utente})
		if err != nil {
			t.Fatalf("fonte %s %s: %v", azione, chiave, err)
		}
		return r.EventoID
	}
	// k-7: scelta e poi rifiuto dello stesso file (la fonte rifiutata non deve diventare quella del componente)
	fonte("k-7", "scelta", s.allegato2, shaC, "")
	fonte("k-7", "rifiuto", s.allegato2, shaC, "non e' lo STEP del telaio")
	// k-9: la sostituzione, cioe' un rifiuto piu' una scelta
	fonte("k-9", "rifiuto", s.allegato2, shaC, "revisione vecchia")
	fonte("k-9", "scelta", s.allegato, shaB, "")
	// k-x: una scelta, poi il nodo esce dalla bozza formato 2 (versione 2)
	ex := fonte("k-x", "scelta", s.allegato2, shaC, "")
	senzaX := vecchio
	senzaX.Nodi = vecchio.Nodi[:3]
	senzaX.Archi = vecchio.Archi[:2]
	if _, err := s.salva(ctx, p, s.prodotto, &b2, 1, senzaX, []Azione{
		{Azione: AzArcoTolto, Origine: "utente", Prima: mustJSON(arcoX)},
		{Azione: AzNodoTolto, Origine: "utente", Prima: mustJSON(nodoX)},
	}); err != nil {
		t.Fatalf("versione 2 della bozza formato 2: %v", err)
	}
	c, err := Converti(ctx, p, Conversione3{ThreadID: s.thread, ProdottoID: s.prodotto, UtenteID: s.utente, BozzaID: b2, VersioneAttesa: 2,
		Motivo: "si passa al formato 3", ChiaveSalvataggio: uuid.New(), ImprontaRichiesta: strings.Repeat("5", 64)})
	if err != nil {
		t.Fatalf("conversione: %v", err)
	}
	if len(c.FontiNonRiportate) != 1 || c.FontiNonRiportate[0].ChiaveBozza != "k-x" || c.FontiNonRiportate[0].Motivo != FonteNodoAssente ||
		len(c.FontiNonRiportate[0].Eventi) != 1 || c.FontiNonRiportate[0].Eventi[0] != ex {
		t.Fatalf("fonti non riportate: %+v", c.FontiNonRiportate)
	}
	rr, err := p.Query(ctx, `SELECT chiave_bozza, azione, allegato_id, coalesce(motivo, '') FROM fonte_step_evento WHERE bozza_id = $1 ORDER BY evento_id`, c.BozzaID)
	if err != nil {
		t.Fatal(err)
	}
	var righe []string
	for rr.Next() {
		var k, az, mot string
		var all uuid.UUID
		if err := rr.Scan(&k, &az, &all, &mot); err != nil {
			t.Fatal(err)
		}
		nome := "A"
		if all == s.allegato2 {
			nome = "B"
		}
		righe = append(righe, k+" "+az+" "+nome+" "+mot)
	}
	rr.Close()
	atteso := []string{"k-7 scelta B ", "k-7 rifiuto B non e' lo STEP del telaio", "k-9 rifiuto B revisione vecchia", "k-9 scelta A "}
	if strings.Join(righe, "|") != strings.Join(atteso, "|") {
		t.Fatalf("fonti sulla bozza nuova:\n%s\natteso\n%s", strings.Join(righe, "\n"), strings.Join(atteso, "\n"))
	}
	ee := eventiDB(t, p, c.BozzaID, 1)
	if len(ee) != 1 || !strings.Contains(campoJSON(t, ee[0].Dopo, "da_bozza"), `"fonti_non_riportate":[{"chiave_bozza":"k-x","eventi":[`) {
		t.Fatalf("traccia nell'evento di apertura: %+v", ee)
	}
}

// ------------------------------------------------------------------ conversione-1: i rifiuti con i motivi propri (rilievo 17)

func TestV1Rev2ConversioneIRifiutiPropri(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	shaB, shaC := strings.Repeat("b", 64), strings.Repeat("c", 64)
	conv := func(b, prodotto uuid.UUID) error {
		_, err := Converti(ctx, p, Conversione3{ThreadID: s.thread, ProdottoID: prodotto, UtenteID: s.utente, BozzaID: b, VersioneAttesa: 1,
			Motivo: "si passa al formato 3", ChiaveSalvataggio: uuid.New(), ImprontaRichiesta: strings.Repeat("6", 64)})
		return err
	}
	aperta := func(b uuid.UUID) {
		t.Helper()
		if n := conta3(t, p, `SELECT count(*) FROM distinta_bozza WHERE bozza_id = $1 AND stato = 'aperta'`, b); n != 1 {
			t.Fatal("la conversione rifiutata ha chiuso la bozza formato 2")
		}
	}
	radice := func(prodotto uuid.UUID, codice string) Nodo {
		return Nodo{ChiaveBozza: "r", ComponenteID: pu(prodotto), Op: OpRiusa, Codice: codice, Tipo: "finito", Origine: "registrato"}
	}
	// (c) una casella del gruppo della sorgente ma di un altro sha256
	fuori := Snapshot{Formato: FormatoSnapshot, ProdottoID: s.prodotto, VersioneBomBase: &VersioneBomBase{}, FirmaBase: firmaProva,
		Sorgente: &Sorgente{AllegatoID: pu(s.allegato), Sha256: shaB, Gruppo: "bom"},
		Nodi: []Nodo{radice(s.prodotto, "ACM-1000"),
			{ChiaveBozza: "k-2", Op: OpCrea, Codice: "ACM-1210", Tipo: "sciolto", Origine: "step", Caselle: []Casella{{Gruppo: "bom", Rif: "nodo:" + shaC + ":#2"}}}},
		Archi: []Arco{{Padre: "r", Figlio: "k-2", Qta: 1, Origine: "utente"}},
	}
	b := bozza2DB(t, p, s.thread, s.prodotto, s.utente, fuori)
	codice3(t, conv(b, s.prodotto), CodiceCorpoNonValido, MotivoConversioneFuoriSorgente)
	aperta(b)
	// (d) la sorgente senza allegato e uno sha256 senza nessun allegato attivo della RFQ
	senza := Snapshot{Formato: FormatoSnapshot, ProdottoID: s.altroFinito, VersioneBomBase: &VersioneBomBase{}, FirmaBase: firmaProva,
		Sorgente: &Sorgente{Gruppo: "bom"},
		Nodi: []Nodo{radice(s.altroFinito, "ACM-6000"),
			{ChiaveBozza: "k-3", Op: OpCrea, Codice: "ACM-6010", Tipo: "sciolto", Origine: "step",
				Caselle: []Casella{{Gruppo: "bom", Rif: "nodo:" + strings.Repeat("9", 64) + ":#3"}}}},
		Archi: []Arco{{Padre: "r", Figlio: "k-3", Qta: 1, Origine: "utente"}},
	}
	b = bozza2DB(t, p, s.thread, s.altroFinito, s.utente, senza)
	codice3(t, conv(b, s.altroFinito), CodiceCorpoNonValido, MotivoConversioneSenzaOccorrenza)
	aperta(b)
	// una decisione del formato 2 che non ha la forma del formato 3 (escludi senza motivo)
	fin := idDi("finito-decisioni")
	esegui3(t, p, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES ($1, $2, 'ACM-6700', 'finito', 'manuale', $3)`,
		fin, s.thread, s.utente)
	dec := Snapshot{Formato: FormatoSnapshot, ProdottoID: fin, VersioneBomBase: &VersioneBomBase{}, FirmaBase: firmaProva,
		Nodi:      []Nodo{radice(fin, "ACM-6700")},
		Decisioni: []json.RawMessage{json.RawMessage(`{"id":"ex-1","tipo":"escludi","componente_id":"` + idDi("k-p1-1").String() + `"}`)},
	}
	b = bozza2DB(t, p, s.thread, fin, s.utente, dec)
	e := codice3(t, conv(b, fin), CodiceCorpoNonValido, MotivoConversioneDecisione)
	if !strings.Contains(e.Messaggio, "ex-1") {
		t.Fatalf("messaggio: %+v", e)
	}
	aperta(b)
}

// ------------------------------------------------------------------ conversione-1: la traccia oltre il limite (rilievo 26)

func TestV1Rev2ConversioneTracciaOltreIlLimite(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	righe := make([]uuid.UUID, 4000)
	for i := range righe {
		righe[i] = uuid.New()
	}
	vecchio := Snapshot{Formato: FormatoSnapshot, ProdottoID: s.altroFinito, VersioneBomBase: &VersioneBomBase{}, FirmaBase: firmaProva,
		Nodi: []Nodo{{ChiaveBozza: "r", ComponenteID: pu(s.altroFinito), Op: OpRiusa, Codice: "ACM-6000", Tipo: "finito", Origine: "registrato"},
			{ChiaveBozza: "k-m", Op: OpCrea, Codice: "ACM-6090", Tipo: "sciolto", Origine: "utente", RigheProposta: righe}},
		Archi: []Arco{{Padre: "r", Figlio: "k-m", Qta: 1, Origine: "utente"}},
	}
	b2 := bozza2DB(t, p, s.thread, s.altroFinito, s.utente, vecchio)
	chiave := uuid.New()
	conv := func() (Convertita, error) {
		return Converti(ctx, p, Conversione3{ThreadID: s.thread, ProdottoID: s.altroFinito, UtenteID: s.utente, BozzaID: b2, VersioneAttesa: 1,
			Motivo: "si passa al formato 3", ChiaveSalvataggio: chiave, ImprontaRichiesta: strings.Repeat("7", 64)})
	}
	c, err := conv()
	if err != nil {
		t.Fatalf("conversione: %v", err)
	}
	if len(c.RigheCadute) != 1 || len(c.RigheCadute[0].Righe) != 4000 {
		t.Fatalf("righe cadute nella risposta: %d", len(c.RigheCadute))
	}
	ee := eventiDB(t, p, c.BozzaID, 1)
	if len(ee) != 1 || len(ee[0].Dopo) > MaxPrimaDopo || campoJSON(t, ee[0].Dopo, "da_bozza") == "" ||
		!strings.Contains(campoJSON(t, ee[0].Dopo, "da_bozza"), `"righe_cadute_ridotto":1`) || strings.Contains(campoJSON(t, ee[0].Dopo, "da_bozza"), `"righe_cadute":`) {
		t.Fatalf("evento di apertura oltre il limite: %d byte", len(ee[0].Dopo))
	}
	// il reinvio ricalcola l'elenco dalla bozza formato 2, che c'e' ancora
	c2, err := conv()
	if err != nil || !c2.Riproposto || len(c2.RigheCadute) != 1 || len(c2.RigheCadute[0].Righe) != 4000 {
		t.Fatalf("reinvio: %v %+v", err, c2.Salvato3)
	}
}
