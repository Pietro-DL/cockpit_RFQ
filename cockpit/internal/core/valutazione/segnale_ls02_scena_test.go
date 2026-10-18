// L1 — Q-06 portata al livello L1 (V1, LS-02: emendamento di V1 §11.1, «Che cosa il segnale NON fa», punti 1-3;
// direttiva 23 §4.2, D23:175; piano della 0023 §10, Q-06, che resta anche una L4 della corsia QA). Con Calcola, su una RFQ
// con la fonte confermata sulla radice e le righe legacy aperte, i cordoni di saldatura segnalati dall'ancoraggio restano
// nodi della BOM di lavoro, con il loro segnale, e niente della valutazione cambia rispetto alla scena gemella, uguale
// salvo la parola nel nome dei cordoni: i 2D previsti, bloccanti, con il motivo nodo_proposto; il perimetro aperto con
// nodi_da_decidere; la verifica della BOM; lo stato del prodotto; le righe da decidere. Il segnale non toglie il nodo e
// non lo esenta dal 2D: oggi è vero per costruzione (la valutazione non legge i segnali), e questa prova lo tiene vero.
// I nomi e i codici sono sintetici, con la forma dei casi veri.
package valutazione_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

// fattiConNomi: i fatti di uno STEP come fattiStepF (struttura v3 completa, la radice nel primo nodo), con il nome del
// PRODUCT uguale al suo id, come nei cordoni veri.
func fattiConNomi(t *testing.T, sha string, nodi []nodoF, archi []arcoF) fotorfq.Fatti {
	t.Helper()
	n := []map[string]any{}
	for _, x := range nodi {
		n = append(n, map[string]any{"chiave": x.chiave, "id_grezzo": x.id, "nome_grezzo": x.id, "descrizione_grezza": "",
			"rev_grezza": x.rev, "evidenza": map[string]any{}})
	}
	r := []map[string]any{}
	for _, a := range archi {
		r = append(r, map[string]any{"padre": a.padre, "figlio": a.figlio, "qta": a.qta, "evidenza": map[string]any{}})
	}
	raw, err := json.Marshal(map[string]any{"struttura": map[string]any{
		"versione": 3, "schema": "AP214", "radici": []string{nodi[0].chiave}, "avvisi": []string{}, "nodi": n, "relazioni": r,
		"limiti": map[string]any{"troncato": false},
		"scarti": map[string]any{"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return fatti(t, sha, string(raw), testo(""))
}

// scenaCordoni: la scena della BOM (il finito 7120100A con il suo STEP autorizzato, la marcatura della radice, una riga
// aperta per nodo, i fabbisogni di default) con due pezzi e tre cordoni foglia sotto la radice. Il nome dei cordoni ha la
// parola data: con «Saldatura» l'ancoraggio li segnala, con «Piastrina» (stessa lunghezza) no. Nessuna famiglia legge i
// cordoni, come nei casi veri.
func scenaCordoni(t *testing.T, parola string) fotorfq.Thread {
	t.Helper()
	nodi := []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}}
	archi := []arcoF{{"#1", "#2", 2}, {"#1", "#3", 1}}
	for i := 0; i < 3; i++ {
		chiave := fmt.Sprintf("#%d", 10+i)
		nodi = append(nodi, nodoF{chiave, fmt.Sprintf("ACME_111_2.000_%s d'angolo %d_oa_%d", parola, i+1, i), ""})
		archi = append(archi, arcoF{"#1", chiave, 1})
	}
	th := scenaBOM(t, nodi, archi)
	th.Fatti[shaStepB] = fattiConNomi(t, shaStepB, nodi, archi)
	th.Fabbisogni = fabbisogniDefault()
	return th
}

// bomDellEsito: la struttura del prodotto che è la BOM di lavoro, nell'esito del thread.
func bomDellEsito(t *testing.T, et valutazione.EsitoThread, rif string) ancoraggio.StrutturaProdotto {
	t.Helper()
	for _, s := range et.Ancoraggi.Strutture {
		if s.Target == rif && s.Stato == ancoraggio.StatoBOMDiLavoroProposta {
			return s
		}
	}
	t.Fatalf("il prodotto %s non ha la BOM di lavoro", rif)
	return ancoraggio.StrutturaProdotto{}
}

// TestQ06IlSegnaleNonCambiaLaValutazione (Q-06, L1): i tre punti di EM §11.1 sul nodo segnalato, con Calcola.
//  1. Non toglie il nodo: i cordoni sono nodi della BOM di lavoro (PV.Nodi), ognuno con il segnale, uguali ai nodi della
//     gemella salvo il segnale e il testo grezzo; nella struttura restano con i loro archi e le loro righe da decidere.
//  2. Non esenta dal 2D: ogni cordone ha il 2D previsto con il motivo nodo_proposto (i previsti sono i fabbisogni
//     bloccanti), e i previsti sono quelli della gemella.
//  3. Non cambia tipo, perimetro, righe da decidere: la completezza (voci, previsti, stato, perimetro aperto con
//     nodi_da_decidere), la verifica della BOM, lo stato e i motivi del prodotto sono quelli della gemella, campo per campo.
func TestQ06IlSegnaleNonCambiaLaValutazione(t *testing.T) {
	r := insiemeACME(t)
	con := esitoSmistamentoCon(t, scenaCordoni(t, "Saldatura"), r)
	senza := esitoSmistamentoCon(t, scenaCordoni(t, "Piastrina"), r)
	pc, ps := prodottoDi(t, con, rifProdB), prodottoDi(t, senza, rifProdB)
	cordoni := map[string]bool{nodoB("#10"): true, nodoB("#11"): true, nodoB("#12"): true}

	// 1. i nodi
	if len(pc.Nodi) != 6 || len(ps.Nodi) != len(pc.Nodi) {
		t.Fatalf("nodi della BOM di lavoro: %d con la parola, %d senza; attesi 6 e 6 (la radice, due pezzi, tre cordoni)", len(pc.Nodi), len(ps.Nodi))
	}
	segnalati := map[string]bool{}
	for i := range pc.Nodi {
		a, b := pc.Nodi[i], ps.Nodi[i]
		if len(a.Nodo.Segnali) > 0 {
			segnalati[a.Nodo.Rif] = true
			if s := a.Nodo.Segnali[0]; len(a.Nodo.Segnali) != 1 || s.Codice != ancoraggio.CodiceSegnaleElementoCAD || s.Regola != ancoraggio.RegolaElementoCADNomeFogliaV1 {
				t.Errorf("%s: segnali %+v", a.Nodo.Rif, a.Nodo.Segnali)
			}
		}
		if len(b.Nodo.Segnali) > 0 {
			t.Errorf("gemella, %s: segnali %+v senza la parola", b.Nodo.Rif, b.Nodo.Segnali)
		}
		a.Nodo.Segnali, a.Nodo.Codice.Grezzo, b.Nodo.Codice.Grezzo = nil, ancoraggio.ValoreGrezzo{}, ancoraggio.ValoreGrezzo{}
		if x, y := canonicoDi(t, a), canonicoDi(t, b); x != y {
			t.Errorf("il nodo %s cambia con il segnale:\n%s\n%s", a.Nodo.Rif, x, y)
		}
	}
	if !reflect.DeepEqual(segnalati, cordoni) {
		t.Errorf("nodi segnalati %v, attesi i tre cordoni %v", segnalati, cordoni)
	}

	// 2. il 2D dei cordoni
	for rif := range cordoni {
		if p := previstoDi(t, pc.Documenti, rif, "disegno_2d"); p.Motivo != valutazione.MotivoPrevistoNodoProposto {
			t.Errorf("il 2D del cordone %s: %+v", rif, p)
		}
	}

	// 3. la completezza, la verifica della BOM, il prodotto
	if s := statoDi(pc.Documenti); s != statoDi(ps.Documenti) || !strings.HasSuffix(s, " aperto/"+valutazione.MotivoPerimetroNodiDaDecidere) {
		t.Errorf("completezza %q, nella gemella %q: il perimetro resta aperto con nodi_da_decidere", s, statoDi(ps.Documenti))
	}
	if len(pc.Documenti.Previsti) != 5 || len(ps.Documenti.Previsti) != 5 {
		t.Errorf("previsti %d e %d, attesi 5 e 5 (i 2D dei due pezzi e dei tre cordoni): %+v", len(pc.Documenti.Previsti), len(ps.Documenti.Previsti), pc.Documenti.Previsti)
	}
	if x, y := canonicoDi(t, pc.Documenti), canonicoDi(t, ps.Documenti); x != y {
		t.Errorf("la completezza cambia con il segnale:\n%s\n%s", x, y)
	}
	if x, y := canonicoDi(t, pc.BOM), canonicoDi(t, ps.BOM); x != y {
		t.Errorf("la verifica della BOM cambia con il segnale:\n%s\n%s", x, y)
	}
	if pc.Stato != ps.Stato || pc.Verificato || ps.Verificato || canonicoDi(t, pc.Motivi) != canonicoDi(t, ps.Motivi) {
		t.Errorf("il prodotto: stato %s / %s, verificato %v / %v, motivi %+v / %+v", pc.Stato, ps.Stato, pc.Verificato, ps.Verificato, pc.Motivi, ps.Motivi)
	}

	// la struttura di ancoraggio: gli stessi archi e le stesse righe da decidere, cinque (le righe dei due pezzi e dei tre
	// cordoni; quella della radice sta a parte)
	sc, ss := bomDellEsito(t, con, rifProdB), bomDellEsito(t, senza, rifProdB)
	if len(sc.RigheDaDecidere) != 5 || !reflect.DeepEqual(sc.RigheDaDecidere, ss.RigheDaDecidere) || !reflect.DeepEqual(sc.Archi, ss.Archi) ||
		len(sc.Nodi) != len(ss.Nodi) {
		t.Errorf("la struttura: righe da decidere %v / %v, archi %d / %d, nodi %d / %d", sc.RigheDaDecidere, ss.RigheDaDecidere, len(sc.Archi), len(ss.Archi),
			len(sc.Nodi), len(ss.Nodi))
	}
}
