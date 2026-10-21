//go:build integrazione

// L4 — le prove del gate della direttiva 39 (PM23 §10, P39-11) che la tabella per id del 20/10 dava NON ESEGUITE, sul
// database di prova, corsia persistenza:
//   - P35-05 (Prova 5 della direttiva 35, D-ASS-02 [U]): un sottoassieme con due padri; lo spostamento sposta solo l'arco
//     indicato (arco_spostato, prima.padre) e lascia l'altro; discendenti, archi discendenti e AR invariati (stessi
//     associazione_id, nessuna riga nuova o cambiata, nessun evento di associazione);
//   - P35-08 (Prova 8): un componente in due prodotti della stessa RFQ: la seconda bozza trova gia_applicata (R4 e R5 sul
//     database: nessuna AR e nessun evento nuovi); uno svincolo da un prodotto mostra l'effetto sull'altro in
//     condivisione[] di R4, attribuito a chi conferma; dopo R5 la AR, una sola, e' chiusa per tutti e due;
//   - legami_storici[] di R4 con la regola univoca di D-AR04 (iii) [U], la stessa di /dati.fonte.legame_storico.
// Dati sintetici (ACME, codici ACM-xxxx).

package distintabozza

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/core/rfq/associazioni"
)

// fotoAR: il contenuto di file_rfq_associazione e di associazione_evento (md5 di tutte le righe) e i loro conteggi.
func fotoAR(t *testing.T, p *pgxpool.Pool) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, tab := range []string{"file_rfq_associazione", "associazione_evento"} {
		var h *string
		var n int
		if err := p.QueryRow(context.Background(), `SELECT md5(string_agg(x::text, '|' ORDER BY x::text)), count(*) FROM `+tab+` x`).Scan(&h, &n); err != nil {
			t.Fatalf("%s: %v", tab, err)
		}
		v := "-"
		if h != nil {
			v = *h
		}
		out[tab] = v
		out[tab+"#"] = strings.Repeat("x", n)
	}
	return out
}

// nodoManuale3 e arcoUtente3: un nodo crea manuale e un arco dell'utente nella forma dello snapshot formato 3.
func nodoMan35(em any, k, codice, tipo string) map[string]any {
	return map[string]any{"chiave_bozza": k, "op": "crea", "tipo": tipo, "origine": "utente",
		"identita": map[string]any{"emittente": em, "emittente_presunto": true, "codice": codice, "modo": "manuale"}}
}

func arcoUtente3(pa, fi string, q int) map[string]any {
	return map[string]any{"padre": pa, "figlio": fi, "qta": q, "origine": "utente"}
}

// P35-05 (dir. 35 Prova 5; D-ASS-02 [U]; EM H.8): un sottoassieme SX sta sotto due padri (SA ×1, SB ×2) e ha un figlio P4
// con la sua AR; anche SX ha una AR (SA e SC hanno anche un altro figlio, perche' un assieme senza figli e' un blocco). La seconda conferma sposta solo l'arco SA → SX sotto SC (arco_spostato con
// prima.padre = SA): l'arco SB → SX resta con la sua quantita', SX → P4 resta, le identita' restano, le AR restano le
// stesse righe e non nasce nessun evento di associazione ne' nessuna AR.
func TestV1P35_05SpostamentoConPiuPadriDB(t *testing.T) {
	p, s := poolPulito(t)
	pdfSX := allegatoFileDB(t, p, s.thread, "ACM-8300.pdf", "ACM-8300", '5')
	pdfP4 := allegatoFileDB(t, p, s.thread, "ACM-6514.pdf", "ACM-6514", '6')
	c, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaVuota})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := snapshotDellaVersione(t, p, c.BozzaID, 1)
	radice := m["nodi"].([]any)[0].(map[string]any)
	em := radice["identita"].(map[string]any)["emittente"]
	r := radice["chiave_bozza"].(string)
	nn := []any{nodoMan35(em, "k-sa", "ACM-8100", "sottoassieme"), nodoMan35(em, "k-sb", "ACM-8200", "sottoassieme"),
		nodoMan35(em, "k-sc", "ACM-8400", "sottoassieme"), nodoMan35(em, "k-sx", "ACM-8300", "sottoassieme"),
		nodoMan35(em, "k-p4", "ACM-6514", "sciolto"), nodoMan35(em, "k-p5", "ACM-6515", "sciolto"), nodoMan35(em, "k-p6", "ACM-6516", "sciolto")}
	aa := []any{arcoUtente3(r, "k-sa", 1), arcoUtente3(r, "k-sb", 1), arcoUtente3(r, "k-sc", 1), arcoUtente3("k-sa", "k-sx", 1),
		arcoUtente3("k-sb", "k-sx", 2), arcoUtente3("k-sx", "k-p4", 3), arcoUtente3("k-sc", "k-p5", 1), arcoUtente3("k-sa", "k-p6", 1)}
	pbSX, pbP4 := map[string]any{"allegato_id": pdfSX, "chiave_bozza": "k-sx"}, map[string]any{"allegato_id": pdfP4, "chiave_bozza": "k-p4"}
	m["nodi"] = append(m["nodi"].([]any), nn...)
	m["archi"] = aa
	m["documenti_preassociati"] = []any{pbSX, pbP4}
	var az []Azione
	for _, x := range nn {
		az = append(az, Azione{Azione: AzNodoAggiunto, ChiaveBozza: x.(map[string]any)["chiave_bozza"].(string), Dopo: mustJSON(x), Origine: "utente"})
	}
	for _, x := range aa {
		az = append(az, Azione{Azione: AzArcoAggiunto, ChiaveBozza: x.(map[string]any)["figlio"].(string), Dopo: mustJSON(x), Origine: "utente"})
	}
	az = append(az, Azione{Azione: AzFilePreassociato, ChiaveBozza: "k-sx", Dopo: mustJSON(pbSX), Origine: "utente"},
		Azione{Azione: AzFilePreassociato, ChiaveBozza: "k-p4", Dopo: mustJSON(pbP4), Origine: "utente"})
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, c.BozzaID, s.altroFinito, 1, uuid.New(), m, az); err != nil {
		t.Fatalf("R2 della struttura con due padri: %v", err)
	}
	s8DB(t, p, s, s.altroFinito, s.allegato2, shaProva('c'), "ACM-6000", true)
	v := soloProdottoSuDB(t, p, s, s.altroFinito, c.BozzaID, 2, s.allegato2)
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, v), s.utente)
	c1, err := confermaR5DB(t, p, s, richiestaR5(t, p, c.BozzaID, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("conferma 1: %v (%v %+v)", err, codici(rp.Blocchi), rp.Blocchi)
	}
	if c1.AssociazioniApplicate != 2 {
		t.Fatalf("conferma 1, AR: %+v", c1)
	}
	sa, sb, scc, sx, p4 := c1.Mappa["k-sa"], c1.Mappa["k-sb"], c1.Mappa["k-sc"], c1.Mappa["k-sx"], c1.Mappa["k-p4"]
	if n := conta3(t, p, `SELECT count(*) FROM componente_relazione WHERE figlio_id = $1 AND padre_id IN ($2, $3)`, sx, sa, sb); n != 2 {
		t.Fatalf("SX deve avere due padri nella working: %d", n)
	}
	arSX := uuidDaSQL(t, p, `SELECT associazione_id FROM file_rfq_associazione WHERE allegato_id = '`+pdfSX.String()+`' AND stato = 'attiva'`)
	arP4 := uuidDaSQL(t, p, `SELECT associazione_id FROM file_rfq_associazione WHERE allegato_id = '`+pdfP4.String()+`' AND stato = 'attiva'`)

	// la seconda conferma (da-conferma-1): solo l'arco SA → SX passa sotto SC
	c2, err := creaDB(t, p, s, s.altroFinito, uuid.New(), map[string]any{"partenza": PartenzaConferma})
	if err != nil {
		t.Fatalf("da-conferma-1: %v", err)
	}
	m2, _ := snapshotDellaVersione(t, p, c2.BozzaID, 1)
	chiaviDi := map[string]string{}
	for _, x := range m2["nodi"].([]any) {
		n := x.(map[string]any)
		if id, ok := n["componente_id"].(string); ok {
			chiaviDi[id] = n["chiave_bozza"].(string)
		}
	}
	ksa, ksb, ksc, ksx := chiaviDi[sa.String()], chiaviDi[sb.String()], chiaviDi[scc.String()], chiaviDi[sx.String()]
	if ksa == "" || ksb == "" || ksc == "" || ksx == "" {
		t.Fatalf("i nodi della conferma 1 nella bozza: %v", chiaviDi)
	}
	var archi2 []any
	var tolto, nuovo, rimasto map[string]any
	for _, x := range m2["archi"].([]any) {
		a := x.(map[string]any)
		switch {
		case a["padre"] == ksa && a["figlio"] == ksx:
			tolto = a
			nuovo = map[string]any{}
			for k, v := range a {
				nuovo[k] = v
			}
			nuovo["padre"] = ksc
			archi2 = append(archi2, nuovo)
			continue
		case a["padre"] == ksb && a["figlio"] == ksx:
			rimasto = a
		}
		archi2 = append(archi2, a)
	}
	if tolto == nil || rimasto == nil {
		t.Fatalf("i due archi entranti di SX nella bozza da-conferma-1: %v", m2["archi"])
	}
	m2["archi"] = archi2
	prima := fotoAR(t, p)
	if _, err := sc.salva3Su(t, c2.BozzaID, s.altroFinito, 1, uuid.New(), m2, []Azione{
		{Azione: AzArcoSpostato, ChiaveBozza: ksx, Prima: mustJSON(tolto), Dopo: mustJSON(nuovo), Origine: "utente"}}); err != nil {
		t.Fatalf("R2 dello spostamento dell'arco SA → SX: %v", err)
	}
	m3, _ := snapshotDellaVersione(t, p, c2.BozzaID, 2)
	entranti := map[string]float64{}
	for _, x := range m3["archi"].([]any) {
		a := x.(map[string]any)
		if a["figlio"] == ksx {
			entranti[a["padre"].(string)] = a["qta"].(float64)
		}
	}
	if len(entranti) != 2 || entranti[ksb] != 2 || entranti[ksc] != 1 {
		t.Fatalf("nella versione salvata SX resta sotto SB (×2) e passa da SA a SC (×1): %v", entranti)
	}
	rp = mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c2.BozzaID, 2), s.utente)
	if len(rp.Blocchi) != 0 {
		t.Fatalf("R4 dello spostamento: %v %+v", codici(rp.Blocchi), rp.Blocchi)
	}
	for _, d := range rp.DocumentiPreassociati {
		if d.Esito == associazioni.EsitoApplicabile || d.Esito == associazioni.EsitoSostituzione {
			t.Fatalf("R4 non deve prevedere AR nuove per uno spostamento: %+v", rp.DocumentiPreassociati)
		}
	}
	if len(rp.EffettiDocumentali.SpostamentiAR) != 0 || len(rp.Svincoli) != 0 {
		t.Fatalf("R4 non deve prevedere spostamenti di AR: %+v %+v", rp.EffettiDocumentali.SpostamentiAR, rp.Svincoli)
	}
	out, err := confermaR5DB(t, p, s, richiestaR5(t, p, c2.BozzaID, 2, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("conferma 2: %v", err)
	}
	if out.AssociazioniApplicate != 0 || out.SvincoliApplicati != 0 || len(out.IdentitaCreate) != 0 || len(out.IdentitaArchiviate) != 0 ||
		len(out.ArchiTolti) != 1 || len(out.ArchiAggiunti) != 1 {
		t.Fatalf("risposta della conferma 2: %+v", out)
	}
	if out.ArchiTolti[0].PadreID != sa || out.ArchiTolti[0].FiglioID != sx || out.ArchiAggiunti[0].PadreID != scc || out.ArchiAggiunti[0].FiglioID != sx {
		t.Fatalf("si sposta solo l'arco indicato: tolti %+v aggiunti %+v", out.ArchiTolti, out.ArchiAggiunti)
	}
	// la working: SB → SX resta con la sua quantita', SA → SX non c'e' piu', SC → SX c'e'; SX → P4 resta
	if n := conta3(t, p, `SELECT count(*) FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2 AND qta = 2`, sb, sx); n != 1 {
		t.Fatal("l'arco SB → SX (l'altro padre) resta, con la sua quantita'")
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2`, sa, sx); n != 0 {
		t.Fatal("l'arco SA → SX e' spostato")
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2 AND qta = 1`, scc, sx); n != 1 {
		t.Fatal("l'arco SC → SX c'e', con la quantita' dell'arco spostato")
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2 AND qta = 3`, sx, p4); n != 1 {
		t.Fatal("l'arco discendente SX → P4 resta")
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente WHERE componente_id IN ($1, $2) AND archiviato_il IS NULL`, sx, p4); n != 2 {
		t.Fatal("le identita' di SX e P4 restano")
	}
	// la struttura certificata dalla conferma 2
	if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale_arco WHERE conferma_id = $1 AND figlio_id = $2`, out.ConfermaID, sx); n != 2 {
		t.Fatal("nella conferma 2 SX ha ancora due padri")
	}
	if n := conta3(t, p, `SELECT count(*) FROM conferma_strutturale_arco WHERE conferma_id = $1 AND figlio_id = $2 AND padre_id = $3`, out.ConfermaID, sx, sa); n != 0 {
		t.Fatal("nella conferma 2 l'arco SA → SX non c'e'")
	}
	// le AR: le stesse righe, attive, con la conferma d'origine; nessuna riga nuova o cambiata, nessun evento
	if d := fotoAR(t, p); d["file_rfq_associazione"] != prima["file_rfq_associazione"] || d["associazione_evento"] != prima["associazione_evento"] ||
		d["file_rfq_associazione#"] != prima["file_rfq_associazione#"] || d["associazione_evento#"] != prima["associazione_evento#"] {
		t.Fatal("lo spostamento (R2 e R5) non tocca file_rfq_associazione e non scrive eventi di associazione")
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE associazione_id IN ($1, $2) AND stato = 'attiva' AND conferma_id = $3`,
		arSX, arP4, c1.ConfermaID); n != 2 {
		t.Fatal("le due AR restano le stesse righe, attive, con la conferma 1")
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE conferma_id = $1 OR chiusa_conferma_id = $1`, out.ConfermaID); n != 0 {
		t.Fatal("nessuna AR nata o chiusa con la conferma 2")
	}
	if n := conta3(t, p, `SELECT count(*) FROM associazione_evento WHERE allegato_id IN ($1, $2)`, pdfSX, pdfP4); n != 2 {
		t.Fatalf("eventi di associazione: solo i due della conferma 1, avuti %d", n)
	}
}

// P35-08 (dir. 35 Prova 8; EM H.5 «Componente condiviso», H.7; D-TAB-03 A [T-39]): il pezzo della prima conferma di
// ACM-6000 (con la AR del suo PDF) entra anche in un altro prodotto della RFQ (ACM-6800, riusa). La bozza di ACM-6800 che
// preassocia lo stesso file allo stesso pezzo trova gia_applicata in R4 e in R5 (nessuna AR e nessun evento nuovi). Poi
// una bozza di ACM-6800 scollega il file dal pezzo: R4 mostra l'effetto su ACM-6000 in condivisione[], attribuito a chi
// conferma (la conferma di ACM-6800); R5 chiude la AR, una sola, che non vale piu' per nessuno dei due prodotti.
func TestV1P35_08CondivisoGiaApplicataESvincoloDB(t *testing.T) {
	p, s := poolPulito(t)
	_, pezzo, c1 := primaConfermaDB(t, p, s)
	pdf := uuidDaSQL(t, p, `SELECT allegato_id FROM file_rfq_associazione WHERE componente_id = '`+pezzo.String()+`' AND stato = 'attiva'`)
	ar := uuidDaSQL(t, p, `SELECT associazione_id FROM file_rfq_associazione WHERE componente_id = '`+pezzo.String()+`' AND stato = 'attiva'`)
	finitoC := uuid.New()
	esegui3(t, p, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES ($1, $2, 'ACM-6800', 'finito', 'manuale', $3)`,
		finitoC, s.thread, s.utente)
	// la seconda bozza (ACM-6800): il pezzo registrato sotto la radice, con la PB dello stesso file
	c, err := creaDB(t, p, s, finitoC, uuid.New(), map[string]any{"partenza": PartenzaVuota})
	if err != nil {
		t.Fatalf("crea vuota di ACM-6800: %v", err)
	}
	m, _ := snapshotDellaVersione(t, p, c.BozzaID, 1)
	radice := m["nodi"].([]any)[0].(map[string]any)
	em := radice["identita"].(map[string]any)["emittente"]
	k := ChiaveDelComponente(pezzo)
	nodo := map[string]any{"chiave_bozza": k, "componente_id": pezzo, "op": "riusa", "tipo": "sciolto", "origine": "registrato",
		"identita": map[string]any{"emittente": em, "emittente_presunto": true, "codice": "ACM-6500", "modo": "registrato"}}
	arco := arcoUtente3(radice["chiave_bozza"].(string), k, 1)
	pb := map[string]any{"allegato_id": pdf, "chiave_bozza": k}
	m["nodi"] = append(m["nodi"].([]any), nodo)
	m["archi"] = []any{arco}
	m["documenti_preassociati"] = []any{pb}
	sc := scena3DB{scenaDB: s, p: p}
	if _, err := sc.salva3Su(t, c.BozzaID, finitoC, 1, uuid.New(), m, []Azione{
		{Azione: AzNodoAggiunto, ChiaveBozza: k, Dopo: mustJSON(nodo), Origine: "utente"},
		{Azione: AzArcoAggiunto, ChiaveBozza: k, Dopo: mustJSON(arco), Origine: "utente"},
		{Azione: AzFilePreassociato, ChiaveBozza: k, Dopo: mustJSON(pb), Origine: "utente"}}); err != nil {
		t.Fatalf("R2 della seconda bozza: %v", err)
	}
	allC := stepDB(t, p, s.thread, "ACM-6800.stp", '8')
	s8DB(t, p, s, finitoC, allC, shaProva('8'), "ACM-6800", true)
	v := soloProdottoSuDB(t, p, s, finitoC, c.BozzaID, 2, allC)
	rp := mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c.BozzaID, v), s.utente)
	if len(rp.Blocchi) != 0 {
		t.Fatalf("R4 della seconda bozza: %v %+v", codici(rp.Blocchi), rp.Blocchi)
	}
	if len(rp.DocumentiPreassociati) != 1 || rp.DocumentiPreassociati[0].Esito != associazioni.EsitoGiaApplicata ||
		rp.DocumentiPreassociati[0].Associazione == nil || *rp.DocumentiPreassociati[0].Associazione != ar {
		t.Fatalf("la seconda bozza trova gia_applicata in R4: %+v", rp.DocumentiPreassociati)
	}
	prima := fotoAR(t, p)
	out, err := confermaR5DB(t, p, s, richiestaR5(t, p, c.BozzaID, v, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("R5 della seconda bozza: %v", err)
	}
	if out.AssociazioniGiaApplicate != 1 || out.AssociazioniApplicate != 0 || out.SvincoliApplicati != 0 {
		t.Fatalf("R5 della seconda bozza: gia_applicata, niente scritto: %+v", out)
	}
	if d := fotoAR(t, p); d["file_rfq_associazione"] != prima["file_rfq_associazione"] || d["associazione_evento"] != prima["associazione_evento"] {
		t.Fatal("gia_applicata: nessuna AR e nessun evento di associazione nuovi o cambiati")
	}
	if n := conta3(t, p, `SELECT count(*) FROM componente_relazione WHERE padre_id = $1 AND figlio_id = $2`, finitoC, pezzo); n != 1 {
		t.Fatal("il pezzo e' ora anche nella working di ACM-6800")
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE componente_id = $1 AND stato = 'attiva' AND associazione_id = $2 AND conferma_id = $3`,
		pezzo, ar, c1.ConfermaID); n != 1 {
		t.Fatal("la AR resta una sola, con la conferma d'origine di ACM-6000")
	}
	// lo svincolo da ACM-6800 (da-conferma-1): l'effetto su ACM-6000 in condivisione[]
	c2, err := creaDB(t, p, s, finitoC, uuid.New(), map[string]any{"partenza": PartenzaConferma})
	if err != nil {
		t.Fatalf("da-conferma-1 di ACM-6800: %v", err)
	}
	m2, _ := snapshotDellaVersione(t, p, c2.BozzaID, 1)
	d := map[string]any{"id": "d-sv", "tipo": DecisioneScollegaDocumento, "allegato_id": pdf, "componente_id": pezzo}
	m2["decisioni"] = append(m2["decisioni"].([]any), d)
	if _, err := sc.salva3Su(t, c2.BozzaID, finitoC, 1, uuid.New(), m2, []Azione{{Azione: AzDecisioneAggiunta, Dopo: mustJSON(d), Origine: "utente"}}); err != nil {
		t.Fatalf("R2 dello svincolo: %v", err)
	}
	rp = mustRiepilogoDB(t, p, s.thread, corpoDellaVersione(t, p, c2.BozzaID, 2), s.utente)
	if len(rp.Blocchi) != 0 || len(rp.Svincoli) != 1 || rp.Svincoli[0].Esito != associazioni.EsitoApplicabile {
		t.Fatalf("R4 dello svincolo: %v %+v %+v", codici(rp.Blocchi), rp.Blocchi, rp.Svincoli)
	}
	var effetto *CondivisioneR4
	for i := range rp.Condivisione {
		if rp.Condivisione[i].ProdottoID == s.altroFinito {
			effetto = &rp.Condivisione[i]
		}
	}
	if effetto == nil {
		t.Fatalf("lo svincolo da ACM-6800 non mostra l'effetto su ACM-6000 in condivisione[]: %+v", rp.Condivisione)
	}
	trovato := false
	for _, e := range effetto.Elementi {
		if e.Tipo == "documento" && e.ComponenteID != nil && *e.ComponenteID == pezzo && strings.Contains(e.Effetto, "ACM-6500.pdf") &&
			strings.Contains(e.Effetto, "scollegato") {
			trovato = true
		}
	}
	if !trovato || !strings.Contains(effetto.Conseguenza, "conferma di ACM-6800") || !strings.Contains(effetto.Conseguenza, "chi la conferma") {
		t.Fatalf("l'effetto dello svincolo sul pezzo condiviso, attribuito a chi conferma: %+v", *effetto)
	}
	if len(trova(rp.Avvisi, AvvisoProdottiToccati)) != 0 {
		t.Fatalf("uno svincolo non cambia la struttura dell'altro prodotto: niente PRODOTTI_TOCCATI (%v)", codici(rp.Avvisi))
	}
	out, err = confermaR5DB(t, p, s, richiestaR5(t, p, c2.BozzaID, 2, rp.FirmaAnteprima, uuid.New(), nil), OpzioniR5{})
	if err != nil {
		t.Fatalf("R5 dello svincolo: %v", err)
	}
	if out.SvincoliApplicati != 1 {
		t.Fatalf("R5 dello svincolo: %+v", out)
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE associazione_id = $1 AND stato = 'chiusa' AND chiusa_conferma_id = $2`, ar, out.ConfermaID); n != 1 {
		t.Fatal("la AR e' chiusa dalla conferma di ACM-6800")
	}
	if n := conta3(t, p, `SELECT count(*) FROM file_rfq_associazione WHERE componente_id = $1 AND stato = 'attiva'`, pezzo); n != 0 {
		t.Fatal("la AR, una sola, non vale piu' per nessuno dei due prodotti")
	}
}

// legami_storici[] di R4 con la regola univoca di D-AR04 (iii) A [U] («se esistono piu' candidati o evidenze discordanti,
// non si seleziona automaticamente una sorgente»), la stessa di /dati.fonte.legame_storico (R39-06: per i prodotti senza
// conferme di S8): uno STEP cad_3d corrente del prodotto → il legame storico; due → nessuno; una conferma di S8 su uno
// STEP del prodotto → nessuno, anche per l'altro STEP storico.
func TestV1D39R4LegamiStoriciUnivociDB(t *testing.T) {
	p, s := poolPulito(t)
	ctx := context.Background()
	corpo := func() map[string]any {
		return map[string]any{"formato": 3, "prodotto_id": s.altroFinito,
			"versione_bom_base": map[string]any{"conferma_id": nil, "numero": 0, "contatore": conta3(t, p, `SELECT COALESCE((SELECT struttura FROM working_contatore WHERE thread_id = $1), 0)`, s.thread)},
			"firma_base":        firmaDiDB(t, p, s.thread, s.altroFinito),
			"sorgente":          map[string]any{"allegato_id": s.allegato2, "sha256": shaProva('c'), "gruppo": GruppoSoloProdotto},
			"nodi": []any{map[string]any{"chiave_bozza": ChiaveDelComponente(s.altroFinito), "componente_id": s.altroFinito, "op": "riusa", "tipo": "finito",
				"origine": "registrato", "identita": map[string]any{"emittente": EmittenteCliente(idDi("cliente")), "emittente_presunto": true, "codice": "ACM-6000",
					"modo": "registrato"}}},
			"archi": []any{}, "decisioni": []any{}, "documenti_preassociati": []any{}}
	}
	stepDoc := func(nome string, sha string) uuid.UUID {
		var doc uuid.UUID
		if err := p.QueryRow(ctx, `INSERT INTO documento (thread_id, componente_id, tipo, codice, nome_file, estensione, sha256, path_relativo, confermato_da)
			VALUES ($1, $2, 'cad_3d', 'ACM-6000', $3::text, 'stp', $4, '3D/' || $3::text, $5) RETURNING documento_id`,
			s.thread, s.altroFinito, nome, sha, s.utente).Scan(&doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	docA := stepDoc("ACM-6000.stp", shaProva('c'))
	r := mustRiepilogoDB(t, p, s.thread, corpo(), s.utente)
	if len(r.LegamiStorici) != 1 || r.LegamiStorici[0].DocumentoID != docA {
		t.Fatalf("un solo STEP del prodotto, senza S8: e' il legame storico univoco: %+v", r.LegamiStorici)
	}
	docB := stepDoc("ACM-6000_alt.stp", shaProva('d'))
	r = mustRiepilogoDB(t, p, s.thread, corpo(), s.utente)
	if len(r.LegamiStorici) != 0 {
		t.Fatalf("due STEP correnti del prodotto: nessun legame storico (non univoco, D-AR04 (iii)): %+v", r.LegamiStorici)
	}
	// S8 conferma A: il prodotto ha una conferma di S8, B non e' un legame storico
	esegui3(t, p, `INSERT INTO fonte_step_evento (thread_id, prodotto_id, ruolo, azione, allegato_id, sha256, documento_id, chiave_idempotenza, autore_id)
		VALUES ($1, $2, 'prodotto', 'conferma_documento', $3, $4, $5, gen_random_uuid(), $6)`, s.thread, s.altroFinito, s.allegato2, shaProva('c'), docA, s.utente)
	r = mustRiepilogoDB(t, p, s.thread, corpo(), s.utente)
	if len(r.LegamiStorici) != 0 {
		t.Fatalf("prodotto con una conferma di S8: nessun legame storico (anche lo STEP %s): %+v", docB, r.LegamiStorici)
	}
}
