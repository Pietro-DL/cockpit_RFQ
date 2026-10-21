//go:build integrazione

// L4 — direttiva 39 §1 e §3 [U + T-39], §8 punti 1 e 2, corsia persistenza: la bozza manuale senza STEP e la
// riconciliazione con lo STEP (cambio-fonte-2) sul database di prova, dal corpo letto da LeggiRichiestaR2, con un registro
// di prova al posto del calcolo del motore. Che cosa si prova:
//   - crea vuota: nessun allegato, sha256, gruppo, registro o fonte_step_evento inventati; la versione 1 senza sorgente;
//   - R2 salva nodi e archi manuali e PB manuali create, spostate e svincolate; R1 rilegge; tre versioni; nessuna scrittura
//     su AR, documento_proposta, documento, componente, componente_relazione;
//   - il cambio della fonte verso uno STEP: la stessa bozza (nessuna seconda bozza attiva, versione n+1), la versione
//     manuale immutabile; il nodo con l'identita' sostenuta tiene la chiave e le PB e acquisisce la casella, niente
//     doppioni; ambiguo, incompatibile ed evidenze discordanti in abbinamenti_proposti[] (fuori da da_rivedere[]) con le
//     voci trattenute; la quantita' diversa e gli archi verso le voci trattenute fra le differenze; fonte_step_evento
//     scelta; l'evento sorgente_cambiata con prima null e l'algoritmo cambio-fonte-2;
//   - la risoluzione esplicita con R2: «Accetta» (nodo_modificato con la casella e l'identita' della proposta), «Rifiuta»
//     (niente: il nodo resta manuale) e la voce come nodo nuovo («Importa dalla proposta…»); la regola dello stesso nodo
//     (DEC-B PB-34 estesa): il nodo abbinato tolto e rimesso sulla stessa casella passa, un nodo manuale senza casella con
//     quella chiave no.
// Solo dati sintetici: la scena di persistenza_db_test.go, gli aiuti di c3_*_db_test.go e c39_riconcilia_test.go.

package distintabozza

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

func manuale39(id idProva3, chiave, codice, rev, tipo string) map[string]any {
	idn := map[string]any{"emittente": id.clienteE(), "emittente_presunto": true, "codice": codice, "modo": "manuale"}
	if rev != "" {
		idn["rev"] = rev
	}
	return map[string]any{"chiave_bozza": chiave, "op": "crea", "tipo": tipo, "origine": "utente", "identita": idn}
}

func arco39(padre, figlio string, qta int) map[string]any {
	return map[string]any{"padre": padre, "figlio": figlio, "qta": qta, "origine": "utente"}
}

func pb39(allegato uuid.UUID, chiave string) map[string]any {
	return map[string]any{"allegato_id": allegato.String(), "chiave_bozza": chiave}
}

// registro39: il registro di prova con le identita' proposte della prova (codici fuori dalla working della scena).
func registro39(t *testing.T, id idProva3) importazione.Registro {
	t.Helper()
	c := registroProva3(t, id)
	vocePropone(t, id, &c, "#7", "ACM-1950", "A", false)
	vocePropone(t, id, &c, "#9", "ACM-1959", "", true) // con le alternative: evidenze discordanti
	vocePropone(t, id, &c, "#11", "ACM-1961", "", false)
	vocePropone(t, id, &c, "#31", "ACM-1971", "A", false)
	return componi(t, c)
}

func snapshotDB(t *testing.T, p *pgxpool.Pool, bozza uuid.UUID, versione int) (string, string) {
	t.Helper()
	var snap, impronta string
	if err := p.QueryRow(context.Background(), `SELECT snapshot::text, impronta FROM distinta_bozza_versione WHERE bozza_id = $1 AND versione = $2`,
		bozza, versione).Scan(&snap, &impronta); err != nil {
		t.Fatal(err)
	}
	return snap, impronta
}

func TestV1D39BozzaManualeERiconciliazione(t *testing.T) {
	p, s := poolPulito(t)
	id := idSintetici3()
	sc := scena3DB{scenaDB: s, p: p, id: id}
	root := ChiaveDelComponente(s.prodotto)
	vietate := contiTabelleVietate(t, p)

	// 1. crea vuota: nessuna sorgente, nessun registro, nessuna riga della storia della fonte
	c, err := creaDB(t, p, s, s.prodotto, uuid.New(), map[string]any{"partenza": "vuota"})
	if err != nil {
		t.Fatalf("crea vuota: %v", err)
	}
	s1, err := LeggiSnapshot3(c.Snapshot)
	if err != nil || s1.Sorgente != nil || len(s1.Nodi) != 1 || c.Importazione != nil || strings.Contains(string(c.Snapshot), `"sorgente"`) {
		t.Fatalf("vuota: %v %+v %s", err, s1.Sorgente, c.Snapshot)
	}
	if n := conta3(t, p, `SELECT count(*) FROM distinta_bozza WHERE bozza_id = $1 AND fonte_allegato_id IS NULL AND fonte_sha256 IS NULL AND fonte_gruppo IS NULL`,
		c.BozzaID); n != 1 {
		t.Fatalf("la testata con una fonte inventata: %d", n)
	}
	if n := conta3(t, p, `SELECT count(*) FROM distinta_bozza_versione WHERE bozza_id = $1 AND (importazione_id IS NOT NULL OR fonte_allegato_id IS NOT NULL)`,
		c.BozzaID); n != 0 {
		t.Fatalf("la versione 1 con un registro o una fonte: %d", n)
	}
	if n := conta3(t, p, `SELECT (SELECT count(*) FROM distinta_importazione) + (SELECT count(*) FROM fonte_step_evento)`); n != 0 {
		t.Fatalf("registro o storia della fonte inventati: %d", n)
	}

	// 2. R2 salva i nodi manuali, gli archi e le PB
	pdf1 := nuovoAllegato(t, p, idDi("msg-0"), 21, "ACM-1950.pdf", strings.Repeat("1", 64))
	pdf2 := nuovoAllegato(t, p, idDi("msg-0"), 22, "ACM-1959.pdf", strings.Repeat("2", 64))
	pdf3 := nuovoAllegato(t, p, idDi("msg-0"), 23, "ACM-1999.pdf", strings.Repeat("3", 64))
	var cur map[string]any
	if err := json.Unmarshal(c.Snapshot, &cur); err != nil {
		t.Fatal(err)
	}
	nodi := []map[string]any{manuale39(id, "m-telaio", "ACM-1950", "A", "sottoassieme"), manuale39(id, "m-pezzo", "ACM-1959", "", "sciolto"),
		manuale39(id, "m-d1", "ACM-1961", "", "sciolto"), manuale39(id, "m-d2", "ACM-1961", "", "sciolto"),
		manuale39(id, "m-cordone", "ACM-1971", "B", "sciolto"), manuale39(id, "m-libero", "ACM-1999", "", "commerciale")}
	archi := []map[string]any{arco39(root, "m-telaio", 2), arco39("m-telaio", "m-pezzo", 1), arco39("m-telaio", "m-d1", 1), arco39(root, "m-d2", 1),
		arco39("m-telaio", "m-cordone", 1), arco39(root, "m-libero", 4)}
	pbs := []map[string]any{pb39(pdf1, "m-telaio"), pb39(pdf2, "m-pezzo"), pb39(pdf3, "m-libero")}
	v2 := copiaProfonda(t, cur)
	var aa []Azione
	for i, n := range nodi {
		v2["nodi"] = append(v2["nodi"].([]any), n)
		v2["archi"] = append(v2["archi"].([]any), archi[i])
		aa = append(aa, azione3(t, AzNodoAggiunto, n["chiave_bozza"].(string), nil, n), azione3(t, AzArcoAggiunto, n["chiave_bozza"].(string), nil, archi[i]))
	}
	for _, x := range pbs {
		v2["documenti_preassociati"] = append(v2["documenti_preassociati"].([]any), x)
		aa = append(aa, azione3(t, AzFilePreassociato, x["chiave_bozza"].(string), nil, x))
	}
	r, err := sc.salva3Su(t, c.BozzaID, s.prodotto, 1, uuid.New(), v2, aa)
	if err != nil || r.Versione != 2 {
		t.Fatalf("i nodi manuali e le PB: %+v %v", r, err)
	}
	// la PB spostata (svincolo e preassociazione) e quella svincolata
	v3 := copiaProfonda(t, v2)
	v3["documenti_preassociati"] = []any{pb39(pdf1, "m-telaio"), pb39(pdf2, "m-telaio")}
	r, err = sc.salva3Su(t, c.BozzaID, s.prodotto, 2, uuid.New(), v3, []Azione{
		azione3(t, AzFileSvincolato, "m-pezzo", pb39(pdf2, "m-pezzo"), map[string]any{"allegato_id": pdf2.String(), "motivo": SvincoloUtente}),
		azione3(t, AzFilePreassociato, "m-telaio", nil, pb39(pdf2, "m-telaio")),
		azione3(t, AzFileSvincolato, "m-libero", pb39(pdf3, "m-libero"), map[string]any{"allegato_id": pdf3.String(), "motivo": SvincoloUtente})})
	if err != nil || r.Versione != 3 {
		t.Fatalf("PB spostata e svincolata: %+v %v", r, err)
	}
	l := sc.leggiR1(t, OpzioniR1{Eventi: true})
	sl, err := LeggiSnapshot3(l.Snapshot)
	if err != nil || l.Versione != 3 || len(sl.Nodi) != 7 || len(sl.Archi) != 6 || len(sl.DocumentiPreassociati) != 2 || sl.Sorgente != nil {
		t.Fatalf("R1 della bozza manuale: v%d %v %+v", l.Versione, err, sl.DocumentiPreassociati)
	}
	if dopo := contiTabelleVietate(t, p); !mappeUguali(vietate, dopo) {
		t.Fatalf("la bozza manuale ha scritto fuori dalla bozza: %v → %v", vietate, dopo)
	}
	manualeSnap, manualeImpronta := snapshotDB(t, p, c.BozzaID, 3)

	// 3. arriva lo STEP: la fonte confermata del prodotto, poi il cambio della fonte sulla stessa bozza (cambio-fonte-2)
	esegui3(t, p, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
		VALUES ($1, $2, 'cad_3d', 'ACM-1000', 'ACM-1000.stp', 'stp', repeat('b', 64), '3D/ACM-1000.stp', $3)`, s.thread, s.prodotto, s.utente)
	vietate = contiTabelleVietate(t, p)
	reg := registro39(t, id)
	chiave := uuid.New()
	v, err := cambiaDB(t, p, s, c.BozzaID, 3, chiave, s.allegato, reg, nil)
	if err != nil {
		t.Fatalf("la riconciliazione: %v", err)
	}
	if v.BozzaID != c.BozzaID || v.Versione != 4 {
		t.Fatalf("la versione della stessa bozza: %+v", v.Salvato3)
	}
	if n := conta3(t, p, `SELECT count(*) FROM distinta_bozza WHERE prodotto_id = $1`, s.prodotto); n != 1 {
		t.Fatalf("bozze del prodotto: %d", n)
	}
	if snap, imp := snapshotDB(t, p, c.BozzaID, 3); snap != manualeSnap || imp != manualeImpronta {
		t.Fatal("la versione manuale e' cambiata")
	}
	s4, err := LeggiSnapshot3(v.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	tel, ok := nodoC3(s4, "m-telaio")
	if !ok || len(tel.Caselle) != 1 || tel.Caselle[0].Rif != id.rif("#7") || tel.Identita.Modo != ModoProposta || tel.Identita.Codice != "ACM-1950" {
		t.Fatalf("il telaio abbinato: %+v", tel)
	}
	if _, doppio := nodoC3(s4, ChiaveDellaVoce(s.allegato, id.rif("#7"))); doppio || len(s4.Nodi) != 7 {
		t.Fatalf("doppioni o voci trattenute entrate: %d nodi", len(s4.Nodi))
	}
	if len(s4.DocumentiPreassociati) != 2 || len(s4.Archi) != 6 {
		t.Fatalf("PB o archi dell'ingegnere toccati: %+v %+v", s4.DocumentiPreassociati, s4.Archi)
	}
	imp := v.Importazione
	if imp == nil || len(imp.AbbinamentiProposti) != 4 || len(imp.CaselleRiscritte) < 2 {
		t.Fatalf("elenchi: %+v", imp)
	}
	motivi := map[string]string{}
	for _, x := range imp.AbbinamentiProposti {
		motivi[x.ChiaveBozza] = x.Motivo
	}
	if motivi["m-pezzo"] != AbbinamentoEvidenzeDiscordanti || motivi["m-d1"] != AbbinamentoAmbiguo || motivi["m-d2"] != AbbinamentoAmbiguo ||
		motivi["m-cordone"] != AbbinamentoIncompatibile {
		t.Fatalf("abbinamenti proposti: %+v", imp.AbbinamentiProposti)
	}
	for _, x := range imp.DaRivedere {
		if x.DecisioneID == "" && strings.HasPrefix(x.ChiaveBozza, "m-") {
			t.Fatalf("un abbinamento in da_rivedere: %+v", x)
		}
	}
	acquisita := false
	for _, x := range imp.CaselleRiscritte {
		acquisita = acquisita || (x.ChiaveBozza == "m-telaio" && x.Prima == nil && x.Dopo == s.allegato)
	}
	var diff propostabom.Differenze
	if err := json.Unmarshal(imp.Differenze, &diff); err != nil || !acquisita {
		t.Fatalf("casella acquisita %v, differenze %v", acquisita, err)
	}
	if !diffArco(diff, id.rif("#1"), id.rif("#7"), propostabom.DifferenzaCambiato) || !diffArco(diff, id.rif("#7"), id.rif("#11"), propostabom.DifferenzaAggiunto) {
		t.Fatalf("differenze: %+v", diff)
	}
	ee := eventiDB(t, p, c.BozzaID, 1)
	ultimo := ee[len(ee)-1]
	if ultimo.Azione != AzSorgenteCambiata || ultimo.Origine != "server" || ultimo.Prima != nil || campoJSON(t, ultimo.Dopo, "algoritmo") != `"`+AlgoritmoCambioFonte2+`"` ||
		!strings.Contains(string(ultimo.Dopo), `"abbinamenti_proposti"`) {
		t.Fatalf("sorgente_cambiata: %+v %s", ultimo, ultimo.Dopo)
	}
	if n := conta3(t, p, `SELECT count(*) FROM fonte_step_evento WHERE azione = 'scelta' AND bozza_id = $1 AND allegato_id = $2 AND precedente_allegato_id IS NULL`,
		c.BozzaID, s.allegato); n != 1 {
		t.Fatalf("la prima fonte della bozza e' una scelta: %d", n)
	}
	if dopo := contiTabelleVietate(t, p); !mappeUguali(vietate, dopo) {
		t.Fatalf("la riconciliazione ha scritto fuori dalla bozza: %v → %v", vietate, dopo)
	}
	// il reinvio: riproposto, niente scritto
	if v2, err := cambiaDB(t, p, s, c.BozzaID, 3, chiave, s.allegato, reg, nil); err != nil || !v2.Riproposto || v2.Versione != 4 {
		t.Fatalf("reinvio: %v %+v", err, v2)
	}

	// 4. la risoluzione esplicita con R2. «Accetta» il pezzo (evidenze discordanti): la casella e l'identita' della proposta
	var m4 map[string]any
	if err := json.Unmarshal(v.Snapshot, &m4); err != nil {
		t.Fatal(err)
	}
	v5 := copiaProfonda(t, m4)
	pezzo := nodoIn3(v5, "m-pezzo")
	primaPezzo := map[string]any{"caselle": nil, "identita": pezzo["identita"]}
	casella := map[string]any{"allegato_id": s.allegato.String(), "gruppo": "bom", "rif": id.rif("#9")}
	idProposta := map[string]any{"emittente": id.clienteE(), "namespace": "acme", "codice": "ACM-1959", "modo": "proposta"}
	pezzo["caselle"] = []any{casella}
	pezzo["identita"] = idProposta
	r, err = sc.salva3Su(t, c.BozzaID, s.prodotto, 4, uuid.New(), v5, []Azione{azione3(t, AzNodoModificato, "m-pezzo", primaPezzo,
		map[string]any{"caselle": []any{casella}, "identita": idProposta})})
	if err != nil || r.Versione != 5 {
		t.Fatalf("Accetta: %+v %v", r, err)
	}
	// «Rifiuta» il cordone (incompatibile): niente; la voce #31 entra come nodo nuovo con «Importa dalla proposta…»
	k31 := ChiaveDellaVoce(s.allegato, id.rif("#31"))
	nuovo31 := map[string]any{"chiave_bozza": k31, "op": "crea", "tipo": "sciolto", "origine": "step",
		"caselle":  []any{map[string]any{"allegato_id": s.allegato.String(), "gruppo": "bom", "rif": id.rif("#31")}},
		"identita": map[string]any{"emittente": id.clienteE(), "namespace": "acme", "codice": "ACM-1971", "rev": "A", "modo": "proposta"}}
	v6 := copiaProfonda(t, v5)
	v6["nodi"] = append(v6["nodi"].([]any), nuovo31)
	r, err = sc.salva3Su(t, c.BozzaID, s.prodotto, 5, uuid.New(), v6, []Azione{azione3(t, AzNodoAggiunto, k31, nil, nuovo31)})
	if err != nil || r.Versione != 6 {
		t.Fatalf("la voce come nodo nuovo: %+v %v", r, err)
	}
	s6 := sc.leggiR1(t, OpzioniR1{})
	sn6, err := LeggiSnapshot3(s6.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if cord, _ := nodoC3(sn6, "m-cordone"); cord.Identita.Modo != ModoManuale || len(cord.Caselle) != 0 {
		t.Fatalf("il cordone rifiutato: %+v", cord)
	}
	// 5. lo stesso nodo (DEC-B PB-34 estesa): il telaio abbinato tolto e rimesso sulla stessa casella passa; con la sua
	// chiave un nodo manuale senza casella no
	telM := nodoIn3(v6, "m-telaio")
	senza := senzaChiave(t, v6, "m-telaio")
	pbTolte := []Azione{}
	for _, x := range []uuid.UUID{pdf1, pdf2} {
		pbTolte = append(pbTolte, azione3(t, AzFileSvincolato, "m-telaio", pb39(x, "m-telaio"), map[string]any{"allegato_id": x.String(), "motivo": SvincoloNodoTolto}))
	}
	var tolte []Azione
	for _, a := range v6["archi"].([]any) {
		am := a.(map[string]any)
		if am["padre"] == "m-telaio" || am["figlio"] == "m-telaio" {
			tolte = append(tolte, azione3(t, AzArcoTolto, am["figlio"].(string), am, nil))
		}
	}
	senza["documenti_preassociati"] = []any{}
	tolte = append(append(tolte, pbTolte...), azione3(t, AzNodoTolto, "m-telaio", telM, nil))
	r, err = sc.salva3Su(t, c.BozzaID, s.prodotto, 6, uuid.New(), senza, tolte)
	if err != nil {
		t.Fatalf("togliere il telaio abbinato: %v", err)
	}
	diverso := manuale39(id, "m-telaio", "ACM-1950", "A", "sottoassieme")
	con := copiaProfonda(t, senza)
	con["nodi"] = append(con["nodi"].([]any), diverso)
	_, err = sc.salva3Su(t, c.BozzaID, s.prodotto, r.Versione, uuid.New(), con, []Azione{azione3(t, AzNodoAggiunto, "m-telaio", nil, diverso)})
	if e := codice3(t, err, CodiceCorpoNonValido, ""); e.Campo != "azioni[0].chiave_bozza" {
		t.Fatalf("la chiave del nodo abbinato per un nodo manuale: %+v", e)
	}
	stesso := copiaProfonda(t, telM)
	con = copiaProfonda(t, senza)
	con["nodi"] = append(con["nodi"].([]any), stesso)
	if _, err := sc.salva3Su(t, c.BozzaID, s.prodotto, r.Versione, uuid.New(), con, []Azione{azione3(t, AzNodoAggiunto, "m-telaio", nil, stesso)}); err != nil {
		t.Fatalf("il nodo abbinato rimesso sulla stessa casella: %v", err)
	}
	if dopo := contiTabelleVietate(t, p); !mappeUguali(vietate, dopo) {
		t.Fatalf("i salvataggi hanno scritto fuori dalla bozza: %v → %v", vietate, dopo)
	}
}
