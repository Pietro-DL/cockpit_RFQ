//go:build integrazione

// L4 — la lettura delle bozze di tutti i prodotti per S9 (direttiva 35, blocco C-4, parte persistenza; U2 §12 punto 6):
// una RFQ con due prodotti con la bozza formato 3 aperta e un terzo con una bozza formato 2, sul database di prova.
//   - per ogni bozza la versione corrente, le PB {allegato_id, chiave_bozza} con il nodo (codice, rev, casella dello STEP
//     sorgente se c'e'), gli svincoli pendenti con il segno della AR ancora attiva, il vassoio «Da ricollocare» uguale a
//     quello di R1 (anche per un nodo tolto: voce senza nodo);
//   - lo stesso file preassociato in due bozze compare nelle due sezioni;
//   - le AR attive dal servizio unico: la copia legacy di documento_proposta senza AR non c'e' (mai la copia come fonte);
//   - la bozza formato 2 da convertire, senza elenchi; il perimetro della RFQ; una RFQ senza bozze;
//   - tutto in una transazione REPEATABLE READ READ ONLY (nessuna scrittura).
// Solo dati sintetici: la scena di persistenza_db_test.go e gli aiuti di formato3_aiuti_test.go e ondatab_db_test.go.

package distintabozza

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

// leggiBozzeRfq: la lettura in una transazione in sola lettura (una scrittura la farebbe fallire).
func leggiBozzeRfq(t *testing.T, sc scena3DB, thread uuid.UUID) BozzeDellaRfq {
	t.Helper()
	ctx := context.Background()
	tx, err := sc.p.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	out, err := LeggiBozzeDellaRfq(ctx, db.New(tx), thread)
	if err != nil {
		t.Fatalf("lettura delle bozze della RFQ: %v", err)
	}
	return out
}

func TestV1C4LaLetturaDelleBozzeDiTuttiIProdotti(t *testing.T) {
	s := nuovoSalvaPB(t)
	sc := s.sc
	msg := idDi("msg-0")
	pdfB := nuovoAllegato(t, sc.p, msg, 3, "ACM-1109.pdf", strings.Repeat("d", 64))
	pdfM := nuovoAllegato(t, sc.p, msg, 4, "ACM-1110.pdf", strings.Repeat("e", 64))
	pdfAR := nuovoAllegato(t, sc.p, msg, 5, "ACM-1101.pdf", strings.Repeat("f", 64))
	pdfCopia := nuovoAllegato(t, sc.p, msg, 6, "ACM-1102.pdf", strings.Repeat("1", 64))
	pdfC := nuovoAllegato(t, sc.p, msg, 7, "ACM-6201.pdf", strings.Repeat("2", 64))
	comp, compCopia := idDi("k-p1-1"), idDi("k-s1")
	ar := arAttivaDiProva(t, sc.p, sc.thread, sc.prodotto, pdfAR, comp, sc.utente)
	// una pre-assegnazione legacy: la copia di documento_proposta dice un componente, senza AR
	nuovaProposta(t, sc.p, pdfCopia, sc.thread, &compCopia)

	// prodotto 1: svincolo nel vassoio (nodo_escluso), lo svincolo di una AR attiva, due PB e un nodo tolto con la sua PB
	v := copiaProfonda(t, s.cur)
	v["documenti_preassociati"] = []any{}
	if _, err := s.salva(t, v, azione3(t, AzFileSvincolato, "k-0007", map[string]any{"allegato_id": sc.id.pdf.String(), "chiave_bozza": "k-0007"},
		map[string]any{"allegato_id": sc.id.pdf.String(), "motivo": SvincoloNodoEscluso})); err != nil {
		t.Fatalf("svincolo nel vassoio: %v", err)
	}
	sv := map[string]any{"id": "sv-0010", "tipo": "scollega_documento", "allegato_id": pdfAR.String(), "componente_id": comp.String(),
		"motivo": "il disegno e' di un altro particolare"}
	v = copiaProfonda(t, s.cur)
	v["decisioni"] = append(v["decisioni"].([]any), sv)
	if _, err := s.salva(t, v, azione3(t, AzDecisioneAggiunta, "", nil, sv)); err != nil {
		t.Fatalf("svincolo della AR: %v", err)
	}
	for _, x := range []struct {
		allegato uuid.UUID
		chiave   string
	}{{pdfB, "k-0009"}, {pdfM, "k-man"}} {
		snap, a := s.conPre(t, x.allegato, x.chiave)
		if _, err := s.salva(t, snap, a); err != nil {
			t.Fatalf("PB %s: %v", x.chiave, err)
		}
	}
	man := nodoIn3(s.cur, "k-man")
	arcoMan := map[string]any{"padre": "k-root", "figlio": "k-man", "qta": 4, "origine": "utente", "posizione": "Pos." + eAccento}
	senza := senzaChiave(t, s.cur, "k-man")
	var restano []any
	for _, p := range senza["documenti_preassociati"].([]any) {
		if p.(map[string]any)["allegato_id"] != pdfM.String() {
			restano = append(restano, p)
		}
	}
	senza["documenti_preassociati"] = restano
	r, err := s.salva(t, senza,
		azione3(t, AzFileSvincolato, "k-man", map[string]any{"allegato_id": pdfM.String(), "chiave_bozza": "k-man"},
			map[string]any{"allegato_id": pdfM.String(), "motivo": SvincoloNodoTolto}),
		azione3(t, AzArcoTolto, "k-man", arcoMan, nil), azione3(t, AzNodoTolto, "k-man", man, nil))
	if err != nil || r.Versione != 6 {
		t.Fatalf("nodo tolto con la sua PB: %+v %v", r, err)
	}

	// prodotto 2: una bozza formato 3 con due PB, una sullo stesso file del prodotto 1
	cliente := sc.id.clienteE()
	b2 := bozza3Diretta(t, sc.p, sc.thread, sc.altroFinito, sc.utente, map[string]any{"formato": 3, "prodotto_id": sc.altroFinito.String(),
		"versione_bom_base": map[string]any{"conferma_id": nil, "numero": 0, "contatore": 0}, "firma_base": firmaProva,
		"nodi": []any{
			map[string]any{"chiave_bozza": "k-r", "componente_id": sc.altroFinito.String(), "op": "riusa", "tipo": "finito", "origine": "registrato",
				"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": "ACM-6000", "modo": "registrato"}},
			map[string]any{"chiave_bozza": "k-a1", "op": "crea", "tipo": "commerciale", "origine": "utente",
				"identita": map[string]any{"emittente": cliente, "emittente_presunto": true, "codice": "ACM-6200", "rev": "C", "modo": "manuale"}},
		},
		"archi":     []any{map[string]any{"padre": "k-r", "figlio": "k-a1", "qta": 1, "origine": "utente"}},
		"decisioni": []any{},
		"documenti_preassociati": []any{
			map[string]any{"allegato_id": pdfB.String(), "chiave_bozza": "k-a1"},
			map[string]any{"allegato_id": pdfC.String(), "chiave_bozza": "k-r"},
		}})

	// prodotto 3: una bozza formato 2 aperta (da convertire); e una bozza della seconda RFQ (fuori perimetro)
	terzo := uuid.New()
	esegui3(t, sc.p, `INSERT INTO componente (componente_id, thread_id, codice, tipo, origine, confermato_da) VALUES ($1, $2, 'ACM-6500', 'finito', 'manuale', $3)`,
		terzo, sc.thread, sc.utente)
	b3 := bozza2DB(t, sc.p, sc.thread, terzo, sc.utente, soloRadice(terzo, "ACM-6500"))
	bT2 := bozza2DB(t, sc.p, sc.thread2, sc.prodottoT2, sc.utente, soloRadice(sc.prodottoT2, "ACM-7000"))

	l := leggiBozzeRfq(t, sc, sc.thread)

	// le AR attive dal servizio: solo quella registrata, non la copia legacy
	if len(l.Associazioni) != 1 || l.Associazioni[0].ID != ar || l.Associazioni[0].Allegato != pdfAR || l.Associazioni[0].Componente != comp ||
		!l.Associazioni[0].Attiva() {
		t.Fatalf("AR attive: %+v", l.Associazioni)
	}
	perProdotto := map[uuid.UUID]BozzaDellaRfq{}
	for _, b := range l.Bozze {
		perProdotto[b.ProdottoID] = b
	}
	if len(l.Bozze) != 3 || len(perProdotto) != 3 {
		t.Fatalf("bozze lette: %+v", l.Bozze)
	}
	for i := 1; i < len(l.Bozze); i++ {
		if l.Bozze[i-1].ProdottoID.String() > l.Bozze[i].ProdottoID.String() {
			t.Fatalf("bozze non in ordine di prodotto: %v, %v", l.Bozze[i-1].ProdottoID, l.Bozze[i].ProdottoID)
		}
	}
	for _, b := range l.Bozze {
		if b.BozzaID == bT2 {
			t.Fatal("la bozza della seconda RFQ e' nella lettura")
		}
	}

	// prodotto 1
	p1 := perProdotto[sc.prodotto]
	r1 := sc.leggiR1(t, OpzioniR1{})
	if p1.BozzaID != sc.bozza || p1.Formato != 3 || p1.Versione != 6 || p1.Versione != r1.Versione || p1.Impronta != r1.Impronta || p1.DaConvertire ||
		p1.Sorgente == nil || p1.Sorgente.AllegatoID == nil || *p1.Sorgente.AllegatoID != sc.id.allegato {
		t.Fatalf("testata del prodotto 1: %+v (R1 versione %d)", p1, r1.Versione)
	}
	casella9 := Casella3{AllegatoID: sc.id.allegato, Gruppo: gruppoProva3, Rif: sc.id.rif("#9")}
	if len(p1.Preassociazioni) != 1 {
		t.Fatalf("PB del prodotto 1: %+v", p1.Preassociazioni)
	}
	pb := p1.Preassociazioni[0]
	if pb.AllegatoID != pdfB || pb.ChiaveBozza != "k-0009" || pb.Nodo.ChiaveBozza != "k-0009" || pb.Nodo.Codice != "400123" || pb.Nodo.Rev != "" ||
		pb.Nodo.Emittente != "partner:beta" || pb.Nodo.Modo != ModoDecisione || pb.Nodo.Casella == nil || *pb.Nodo.Casella != casella9 ||
		pb.Nodo.ComponenteID != nil {
		t.Fatalf("PB sul nodo dello STEP: %+v", pb)
	}
	svPerID := map[string]SvincoloPendente{}
	for _, x := range p1.Svincoli {
		svPerID[x.DecisioneID] = x
	}
	if len(p1.Svincoli) != 2 ||
		svPerID["sv-0010"] != (SvincoloPendente{DecisioneID: "sv-0010", AllegatoID: pdfAR, ComponenteID: comp, Motivo: "il disegno e' di un altro particolare", AssociazioneAttiva: true}) ||
		svPerID["sv-0001"].AllegatoID.String() != allegatoSvincolo3 || svPerID["sv-0001"].AssociazioneAttiva {
		t.Fatalf("svincoli del prodotto 1: %+v", p1.Svincoli)
	}
	vassoioR1 := map[string]VoceVassoio{}
	for _, x := range *r1.Vassoio {
		vassoioR1[x.AllegatoID] = x
	}
	if len(p1.Vassoio) != 2 || len(vassoioR1) != 2 {
		t.Fatalf("vassoio del prodotto 1: %+v (R1 %+v)", p1.Vassoio, *r1.Vassoio)
	}
	for _, x := range p1.Vassoio {
		if vassoioR1[x.AllegatoID.String()] != (VoceVassoio{AllegatoID: x.AllegatoID.String(), Motivo: x.Motivo, ChiaveBozza: x.ChiaveBozza}) {
			t.Fatalf("voce del vassoio diversa da R1: %+v", x)
		}
		switch x.AllegatoID {
		case sc.id.pdf:
			if x.Motivo != SvincoloNodoEscluso || x.ChiaveBozza != "k-0007" || x.Nodo == nil || x.Nodo.Codice != "ACM-1100" || x.Nodo.Rev != "A" ||
				x.Nodo.Casella == nil || x.Nodo.Casella.Rif != sc.id.rif("#7") {
				t.Fatalf("voce del nodo escluso: %+v %+v", x, x.Nodo)
			}
		case pdfM:
			if x.Motivo != SvincoloNodoTolto || x.ChiaveBozza != "k-man" || x.Nodo != nil {
				t.Fatalf("voce del nodo tolto: %+v", x)
			}
		default:
			t.Fatalf("voce inattesa: %+v", x)
		}
	}

	// prodotto 2: lo stesso file del prodotto 1 nella sua sezione, sul suo nodo
	p2 := perProdotto[sc.altroFinito]
	r2, err := leggiR1DB(t, sc.p, sc.thread, sc.altroFinito, OpzioniR1{})
	if err != nil {
		t.Fatal(err)
	}
	if p2.BozzaID != b2 || p2.Formato != 3 || p2.Versione != 1 || p2.Versione != r2.Versione || p2.Sorgente != nil ||
		len(p2.Svincoli) != 0 || p2.Svincoli == nil || len(p2.Vassoio) != 0 || p2.Vassoio == nil || len(*r2.Vassoio) != 0 {
		t.Fatalf("prodotto 2: %+v", p2)
	}
	perAllegato := map[uuid.UUID]PreassociazioneDellaBozza{}
	for _, x := range p2.Preassociazioni {
		perAllegato[x.AllegatoID] = x
	}
	a1, rr := perAllegato[pdfB], perAllegato[pdfC]
	if len(p2.Preassociazioni) != 2 || a1.ChiaveBozza != "k-a1" || a1.Nodo.Codice != "ACM-6200" || a1.Nodo.Rev != "C" || a1.Nodo.Modo != ModoManuale ||
		a1.Nodo.Casella != nil || len(a1.Nodo.Caselle) != 0 ||
		rr.ChiaveBozza != "k-r" || rr.Nodo.ComponenteID == nil || *rr.Nodo.ComponenteID != sc.altroFinito || rr.Nodo.Op != OpRiusa || rr.Nodo.Codice != "ACM-6000" {
		t.Fatalf("PB del prodotto 2: %+v", p2.Preassociazioni)
	}
	// tutti i nodi della versione (richiesta 2 della corsia motore): quelli dello snapshot, nello stesso ordine, con le PB sui loro
	// nodi; per il prodotto 1 anche i nodi senza PB ne' vassoio
	for _, x := range []struct {
		b BozzaDellaRfq
		v int
	}{{p1, p1.Versione}, {p2, p2.Versione}} {
		m, _ := snapshotDellaVersione(t, sc.p, x.b.BozzaID, x.v)
		nn := m["nodi"].([]any)
		if len(x.b.Nodi) != len(nn) {
			t.Fatalf("nodi della bozza %s: %d, nello snapshot %d", x.b.BozzaID, len(x.b.Nodi), len(nn))
		}
		for i, n := range nn {
			if k := n.(map[string]any)["chiave_bozza"]; x.b.Nodi[i].ChiaveBozza != k {
				t.Fatalf("nodo %d della bozza %s: %q, nello snapshot %v", i, x.b.BozzaID, x.b.Nodi[i].ChiaveBozza, k)
			}
		}
		for _, pb := range x.b.Preassociazioni {
			trovato := false
			for _, n := range x.b.Nodi {
				trovato = trovato || reflect.DeepEqual(n, pb.Nodo)
			}
			if !trovato {
				t.Fatalf("il nodo della PB %+v non e' fra i nodi", pb)
			}
		}
	}
	if len(p1.Nodi) <= len(p1.Preassociazioni)+len(p1.Vassoio) {
		t.Fatalf("prodotto 1: %d nodi, attesi anche quelli senza PB ne' vassoio", len(p1.Nodi))
	}

	// prodotto 3: formato 2, da convertire, senza elenchi
	p3 := perProdotto[terzo]
	if p3.BozzaID != b3 || p3.Formato != 2 || !p3.DaConvertire || p3.Versione != 1 || p3.Preassociazioni != nil || p3.Svincoli != nil || p3.Vassoio != nil || p3.Nodi != nil {
		t.Fatalf("prodotto 3: %+v", p3)
	}

	// la seconda RFQ: solo la sua bozza; una RFQ senza bozze: elenchi vuoti
	if l2 := leggiBozzeRfq(t, sc, sc.thread2); len(l2.Bozze) != 1 || l2.Bozze[0].BozzaID != bT2 || len(l2.Associazioni) != 0 {
		t.Fatalf("seconda RFQ: %+v", l2)
	}
	if l0 := leggiBozzeRfq(t, sc, uuid.New()); l0.Bozze == nil || len(l0.Bozze) != 0 || l0.Associazioni == nil || len(l0.Associazioni) != 0 {
		t.Fatalf("RFQ senza bozze: %+v", l0)
	}

	// la AR chiusa dopo lo svincolo (D-ASS-03): lo svincolo resta pendente, senza AR attiva, e la AR non e' piu' letta
	esegui3(t, sc.p, `ALTER TABLE file_rfq_associazione DISABLE TRIGGER USER`)
	esegui3(t, sc.p, `UPDATE file_rfq_associazione SET stato = 'chiusa', chiusa_il = now(), chiusa_da = $2, motivo_chiusura = 'scollega_file',
		chiusa_tx = pg_current_xact_id() WHERE allegato_id = $1`, pdfAR, sc.utente)
	esegui3(t, sc.p, `ALTER TABLE file_rfq_associazione ENABLE TRIGGER USER`)
	dopo := leggiBozzeRfq(t, sc, sc.thread)
	if len(dopo.Associazioni) != 0 {
		t.Fatalf("AR chiusa ancora letta: %+v", dopo.Associazioni)
	}
	for _, b := range dopo.Bozze {
		if b.ProdottoID != sc.prodotto {
			continue
		}
		for _, x := range b.Svincoli {
			if x.DecisioneID == "sv-0010" && x.AssociazioneAttiva {
				t.Fatalf("svincolo con la AR chiusa: %+v", x)
			}
		}
		if !reflect.DeepEqual(b.Preassociazioni, p1.Preassociazioni) || !reflect.DeepEqual(b.Vassoio, p1.Vassoio) {
			t.Fatal("la chiusura della AR ha cambiato PB o vassoio della bozza")
		}
	}
}
