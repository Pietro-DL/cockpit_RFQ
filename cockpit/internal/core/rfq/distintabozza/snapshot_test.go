package distintabozza

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/rfq/fascicolo"
)

// L1 — la forma dello snapshot formato 2 (contratto rev. 3 §3.2, §6.3): i limiti delle colonne, le regole di forma,
// la forma canonica. Nessun database.

// I limiti del package sono quelli del formato 1 e delle colonne, non numeri scelti a parte.
func TestILimitiSonoQuelliDelFascicoloEDelleColonne(t *testing.T) {
	if MaxVoci != fascicolo.MaxVociBozza {
		t.Errorf("MaxVoci %d, MaxVociBozza del formato 1 %d", MaxVoci, fascicolo.MaxVociBozza)
	}
	if MaxQta != fascicolo.MaxQtaArco {
		t.Errorf("MaxQta %d, MaxQtaArco %d", MaxQta, fascicolo.MaxQtaArco)
	}
	if classificazione.MaxCodice > MaxCodiceRegistrato || classificazione.MaxRev > 10 || MaxCodiceRegistrato != 60 {
		t.Errorf("codice %d / %d, rev %d oltre le colonne varchar(60) / varchar(10)", classificazione.MaxCodice, MaxCodiceRegistrato, classificazione.MaxRev)
	}
	if MaxDescrizione != 200 || MaxPosizione != 20 || MaxGruppo != 300 || MaxMotivo != 1000 {
		t.Error("i limiti di descrizione, posizione, gruppo e motivo non sono piu' quelli delle colonne e dei CHECK della 0022")
	}
}

// mappaDi restituisce lo snapshot come mappa, da guastare una regola alla volta.
func mappaDi(t *testing.T, s Snapshot) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(grezzo(t, s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func nodi(m map[string]any) []any                 { return m["nodi"].([]any) }
func archi(m map[string]any) []any                { return m["archi"].([]any) }
func nodo(m map[string]any, i int) map[string]any { return nodi(m)[i].(map[string]any) }
func arco(m map[string]any, i int) map[string]any { return archi(m)[i].(map[string]any) }

// indice del nodo con quella chiave nella forma normalizzata
func iNodo(m map[string]any, k string) int {
	for i, n := range nodi(m) {
		if n.(map[string]any)["chiave_bozza"] == k {
			return i
		}
	}
	panic(k)
}

// conFonte: la scena con la fonte e una casella STEP su k-s1 (per i casi delle caselle).
func conFonte(t *testing.T, sc scena) map[string]any {
	m := mappaDi(t, sc.snap)
	f := fonteScena()
	m["sorgente"] = map[string]any{"allegato_id": f.AllegatoID.String(), "sha256": f.Sha256, "gruppo": f.Gruppo}
	nodo(m, iNodo(m, "k-s1"))["caselle"] = []any{map[string]any{"gruppo": "bom", "rif": "nodo:" + shaProva('a') + ":P#1"}}
	return m
}

func TestLoSnapshotRifiutaOgniViolazioneDiForma(t *testing.T) {
	sc := scena35(idDi("prodotto"), idDi)
	if _, err := LeggiSnapshot(grezzo(t, sc.snap)); err != nil {
		t.Fatalf("la scena di base non passa: %v", err)
	}
	if _, err := LeggiSnapshot(grezzo(t, conFonte(t, sc))); err != nil {
		t.Fatalf("la scena con la fonte e una casella non passa: %v", err)
	}
	casi := []struct {
		nome   string
		campo  string
		fonte  bool // parte dalla scena con la fonte e una casella
		guasta func(m map[string]any)
	}{
		{"formato 1", "formato", false, func(m map[string]any) { m["formato"] = 1 }},
		{"formato assente", "formato", false, func(m map[string]any) { delete(m, "formato") }},
		{"prodotto assente", "prodotto_id", false, func(m map[string]any) { delete(m, "prodotto_id") }},
		{"campo sconosciuto", "snapshot", false, func(m map[string]any) { m["uuid_inventato"] = "x" }},
		{"campo sconosciuto in un nodo", "snapshot", false, func(m map[string]any) { nodo(m, 0)["colore"] = "rosso" }},
		{"campo con le maiuscole (RC-12)", "snapshot", false, func(m map[string]any) { m["NODI"] = m["nodi"]; delete(m, "nodi") }},
		{"campo con l'iniziale maiuscola (RC-12)", "snapshot", false, func(m map[string]any) { m["Formato"] = 2; delete(m, "formato") }},
		{"due campi uguali salvo le maiuscole (RC-12)", "snapshot", false, func(m map[string]any) { nodo(m, 1)["Codice"] = "ACM-ALTRO" }},
		{"bozza_id nello snapshot", "bozza_id", false, func(m map[string]any) { m["bozza_id"] = idDi("b").String() }},
		{"bozza_versione nello snapshot", "bozza_versione", false, func(m map[string]any) { m["bozza_versione"] = 3 }},
		{"chiave d'idempotenza nello snapshot", "chiave_idempotenza", false, func(m map[string]any) { m["chiave_idempotenza"] = idDi("c").String() }},
		{"nessun nodo", "nodi", false, func(m map[string]any) { m["nodi"] = []any{}; m["archi"] = []any{} }},
		{"nessuna radice", "nodi", false, func(m map[string]any) {
			nodo(m, iNodo(m, "k-root"))["componente_id"] = idDi("altro").String()
			nodo(m, iNodo(m, "k-root"))["tipo"] = "sottoassieme"
		}},
		{"due radici", "nodi", false, func(m map[string]any) { nodo(m, iNodo(m, "k-s1"))["componente_id"] = idDi("prodotto").String() }},
		{"chiave ripetuta", "nodi[", false, func(m map[string]any) { nodo(m, 1)["chiave_bozza"] = nodo(m, 2)["chiave_bozza"] }},
		{"chiave con spazio", ".chiave_bozza", false, func(m map[string]any) { nodo(m, 1)["chiave_bozza"] = "k 1" }},
		{"chiave di 65 caratteri", ".chiave_bozza", false, func(m map[string]any) { nodo(m, 1)["chiave_bozza"] = strings.Repeat("k", 65) }},
		{"op sconosciuta", ".op", false, func(m map[string]any) { nodo(m, 1)["op"] = "fondi" }},
		{"crea con componente_id", ".componente_id", false, func(m map[string]any) { nodo(m, iNodo(m, "k-s1"))["op"] = "crea" }},
		{"riusa senza componente_id", ".componente_id", false, func(m map[string]any) { delete(nodo(m, iNodo(m, "k-s1")), "componente_id") }},
		{"riusa senza codice", ".codice", false, func(m map[string]any) { delete(nodo(m, iNodo(m, "k-s1")), "codice") }},
		{"riusa con codice vuoto", ".codice", false, func(m map[string]any) { nodo(m, iNodo(m, "k-s1"))["codice"] = "" }},
		{"codice registrato di 61 caratteri", ".codice", false, func(m map[string]any) { nodo(m, iNodo(m, "k-s1"))["codice"] = strings.Repeat("A", 61) }},
		{"codice registrato con uno spazio in testa", ".codice", false, func(m map[string]any) { nodo(m, iNodo(m, "k-s1"))["codice"] = " ACM-1100" }},
		{"codice nuovo di 41 caratteri", ".codice", false, func(m map[string]any) {
			nodi := m["nodi"].([]any)
			m["nodi"] = append(nodi, map[string]any{"chiave_bozza": "k-n", "op": "crea", "codice": strings.Repeat("A", 41), "tipo": "sciolto", "origine": "utente"})
		}},
		{"codice nuovo con spazio unificatore", ".codice", false, func(m map[string]any) {
			nodi := m["nodi"].([]any)
			m["nodi"] = append(nodi, map[string]any{"chiave_bozza": "k-n", "op": "crea", "codice": "ACM 1", "tipo": "sciolto", "origine": "utente"})
		}},
		{"rev di 11 caratteri", ".rev", false, func(m map[string]any) { nodo(m, 1)["rev"] = strings.Repeat("B", 11) }},
		{"descrizione di 201 caratteri", ".descrizione", false, func(m map[string]any) { nodo(m, 1)["descrizione"] = strings.Repeat("è", 201) }},
		{"descrizione con un a capo", ".descrizione", false, func(m map[string]any) { nodo(m, 1)["descrizione"] = "riga\nriga" }},
		{"tipo sconosciuto", ".tipo", false, func(m map[string]any) { nodo(m, 1)["tipo"] = "assieme" }},
		{"finito non radice", ".tipo", false, func(m map[string]any) { nodo(m, iNodo(m, "k-s1"))["tipo"] = "finito" }},
		{"origine del nodo sconosciuta", ".origine", false, func(m map[string]any) { nodo(m, 1)["origine"] = "browser" }},
		{"caselle senza sorgente (Δ43, RC-08)", "sorgente", true, func(m map[string]any) { delete(m, "sorgente") }},
		{"origine conferma con base numero 0 (Δ32, RC-08)", ".origine", false, func(m map[string]any) { nodo(m, 1)["origine"] = "conferma" }},
		{"arco con origine conferma e base numero 0 (Δ32)", ".origine", false, func(m map[string]any) { arco(m, 0)["origine"] = "conferma" }},
		{"rif malformato", ".rif", true, func(m map[string]any) {
			nodo(m, iNodo(m, "k-s1"))["caselle"] = []any{map[string]any{"gruppo": "bom", "rif": "PRODUCT#1"}}
		}},
		{"casella senza gruppo", ".gruppo", true, func(m map[string]any) {
			nodo(m, iNodo(m, "k-s1"))["caselle"] = []any{map[string]any{"gruppo": " ", "rif": "nodo:" + shaProva('a') + ":P#1"}}
		}},
		{"gruppo di una casella con U+0000 (RC-07)", ".gruppo", true, func(m map[string]any) {
			nodo(m, iNodo(m, "k-s1"))["caselle"] = []any{map[string]any{"gruppo": "bo\x00m", "rif": "nodo:" + shaProva('a') + ":P#1"}}
		}},
		{"casella ripetuta nel nodo", ".caselle[1]", true, func(m map[string]any) {
			c := map[string]any{"gruppo": "bom", "rif": "nodo:" + shaProva('a') + ":P#9"}
			nodo(m, iNodo(m, "k-s1"))["caselle"] = []any{c, c}
		}},
		{"stesso rif e gruppo in due nodi", ".caselle", true, func(m map[string]any) {
			nodo(m, iNodo(m, "k-s2"))["caselle"] = nodo(m, iNodo(m, "k-s1"))["caselle"]
		}},
		{"riga di proposta in due nodi", ".righe_proposta", false, func(m map[string]any) {
			r := []any{idDi("riga").String()}
			nodo(m, iNodo(m, "k-s1"))["righe_proposta"] = r
			nodo(m, iNodo(m, "k-s2"))["righe_proposta"] = r
		}},
		{"riga di proposta non UUID", "snapshot", false, func(m map[string]any) { nodo(m, 1)["righe_proposta"] = []any{"nodo:abc"} }},
		{"arco verso chiave inesistente", ".figlio", false, func(m map[string]any) { arco(m, 0)["figlio"] = "k-nessuno" }},
		{"arco da se stesso", "archi[0]", false, func(m map[string]any) { arco(m, 0)["figlio"] = arco(m, 0)["padre"] }},
		{"arco ripetuto", "archi[1]", false, func(m map[string]any) { archi(m)[1] = archi(m)[0] }},
		{"qta assente", ".qta", false, func(m map[string]any) { delete(arco(m, 0), "qta") }},
		{"qta zero", ".qta", false, func(m map[string]any) { arco(m, 0)["qta"] = 0 }},
		{"qta negativa", ".qta", false, func(m map[string]any) { arco(m, 0)["qta"] = -2 }},
		{"qta oltre il massimo", ".qta", false, func(m map[string]any) { arco(m, 0)["qta"] = MaxQta + 1 }},
		{"qta non intera", "snapshot", false, func(m map[string]any) { arco(m, 0)["qta"] = 1.5 }},
		{"qta come testo", "snapshot", false, func(m map[string]any) { arco(m, 0)["qta"] = "2" }},
		{"origine dell'arco assente", ".origine", false, func(m map[string]any) { delete(arco(m, 0), "origine") }},
		{"posizione di 21 caratteri", ".posizione", false, func(m map[string]any) { arco(m, 0)["posizione"] = strings.Repeat("1", 21) }},
		{"decisione senza id", ".id", false, func(m map[string]any) { m["decisioni"] = []any{map[string]any{"tipo": "mono_pezzo"}} }},
		{"decisione di tipo sconosciuto", ".tipo", false, func(m map[string]any) { m["decisioni"] = []any{map[string]any{"id": "d1", "tipo": "fondi"}} }},
		{"decisione non oggetto", "decisioni[0]", false, func(m map[string]any) { m["decisioni"] = []any{"d1"} }},
		{"decisioni con id ripetuto", "decisioni[1].id", false, func(m map[string]any) {
			d := map[string]any{"id": "d1", "tipo": "da_valutare", "chiave_bozza": "k-s1"}
			m["decisioni"] = []any{d, d}
		}},
		{"decisione con U+0000 (RC-07)", "decisioni[0]", false, func(m map[string]any) {
			m["decisioni"] = []any{map[string]any{"id": "d1", "tipo": "mono_pezzo", "motivo": "pezzo\x00unico"}}
		}},
		{"decisione con un numero decimale (RC-01)", "decisioni[0]", false, func(m map[string]any) {
			m["decisioni"] = []any{map[string]any{"id": "d1", "tipo": "arco_condiviso", "qta": 1.5}}
		}},
		{"decisione con un campo in maiuscolo (RC-12)", "snapshot", false, func(m map[string]any) {
			m["decisioni"] = []any{map[string]any{"id": "d1", "tipo": "mono_pezzo", "MOTIVO": "x"}}
		}},
		{"preassociazione doppia", ".proposta_id", false, func(m map[string]any) {
			p := map[string]any{"proposta_id": idDi("prop").String(), "chiave_bozza": "k-s1"}
			m["documenti_preassociati"] = []any{p, p}
		}},
		{"preassociazione a chiave inesistente", ".chiave_bozza", false, func(m map[string]any) {
			m["documenti_preassociati"] = []any{map[string]any{"proposta_id": idDi("prop").String(), "chiave_bozza": "k-x"}}
		}},
		{"preassociazione a nodo escluso", ".chiave_bozza", false, func(m map[string]any) {
			nodo(m, iNodo(m, "k-s1"))["op"] = "esclude"
			m["documenti_preassociati"] = []any{map[string]any{"proposta_id": idDi("prop").String(), "chiave_bozza": "k-s1"}}
		}},
		{"sorgente con allegato senza sha", "sorgente", false, func(m map[string]any) {
			m["sorgente"] = map[string]any{"allegato_id": idDi("all").String(), "gruppo": "bom"}
		}},
		{"sorgente con sha maiuscolo", "sorgente.sha256", false, func(m map[string]any) {
			m["sorgente"] = map[string]any{"allegato_id": idDi("all").String(), "sha256": strings.Repeat("A", 64), "gruppo": "bom"}
		}},
		{"sorgente con gruppo di 301 caratteri", "sorgente.gruppo", false, func(m map[string]any) {
			m["sorgente"] = map[string]any{"gruppo": strings.Repeat("g", 301)}
		}},
		{"sorgente con un carattere di controllo nel gruppo (RC-07)", "sorgente.gruppo", false, func(m map[string]any) {
			m["sorgente"] = map[string]any{"gruppo": "bom\x00"}
		}},
		{"base numero 0 con conferma", "versione_bom_base", false, func(m map[string]any) {
			m["versione_bom_base"] = map[string]any{"conferma_id": idDi("conf").String(), "numero": 0, "contatore": 1}
		}},
		{"base senza contatore", "versione_bom_base", false, func(m map[string]any) {
			m["versione_bom_base"] = map[string]any{"conferma_id": nil, "numero": 0}
		}},
		{"firma della base non esadecimale", "firma_base", false, func(m map[string]any) { m["firma_base"] = strings.Repeat("z", 64) }},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			m := mappaDi(t, sc.snap)
			if c.fonte {
				m = conFonte(t, sc)
			}
			c.guasta(m)
			_, err := LeggiSnapshot(grezzo(t, m))
			var e *Errore
			if !errors.As(err, &e) || e.Codice != CodiceCorpoNonValido {
				t.Fatalf("atteso CORPO_NON_VALIDO, ottenuto %v", err)
			}
			if !strings.Contains(e.Campo, c.campo) {
				t.Errorf("campo %q, atteso che contenga %q (%s)", e.Campo, c.campo, e.Messaggio)
			}
		})
	}
}

// Tre rifiuti che encoding/json da solo lascerebbe passare.
func TestLoSnapshotRifiutaChiaviRipetuteUTF8NonValidoEAltroDopo(t *testing.T) {
	sc := scena35(idDi("prodotto"), idDi)
	base := string(grezzo(t, sc.snap))
	casi := map[string][]byte{
		"chiave ripetuta":    []byte(strings.Replace(base, `"formato":2`, `"formato":2,"formato":2`, 1)),
		"UTF-8 non valido":   bytes.Replace([]byte(base), []byte("Gruppo base"), []byte("Gruppo \xff base"), 1),
		"testo dopo il JSON": []byte(base + ` {}`),
	}
	for nome, raw := range casi {
		if _, err := LeggiSnapshot(raw); !errors.Is(err, ErrCorpoNonValido) {
			t.Errorf("%s: atteso CORPO_NON_VALIDO, ottenuto %v", nome, err)
		}
	}
}

// I limiti al bordo: 40 caratteri di un codice nuovo, 60 di un codice registrato, 10 di rev, 200 di descrizione
// (anche accentati), qta 100000, 20 di posizione passano; 5000 nodi passano e 5001 no; oltre 2 MiB no.
func TestILimitiAlBordo(t *testing.T) {
	sc := scena35(idDi("prodotto"), idDi)
	m := mappaDi(t, sc.snap)
	nodo(m, iNodo(m, "k-s1"))["codice"] = strings.Repeat("A", 60)
	nodo(m, 1)["rev"] = strings.Repeat("B", 10)
	nodo(m, 1)["descrizione"] = strings.Repeat("è", 200)
	arco(m, 0)["qta"] = MaxQta
	arco(m, 0)["posizione"] = strings.Repeat("9", 20)
	m["nodi"] = append(m["nodi"].([]any), map[string]any{"chiave_bozza": "k-n", "op": "crea", "codice": strings.Repeat("C", 40), "tipo": "sciolto", "origine": "utente"})
	if _, err := LeggiSnapshot(grezzo(t, m)); err != nil {
		t.Fatalf("i valori al bordo non passano: %v", err)
	}

	molti := func(n int) Snapshot {
		s := Snapshot{Formato: 2, ProdottoID: idDi("p"), Nodi: []Nodo{{ChiaveBozza: "r", ComponenteID: pu(idDi("p")), Op: OpRiusa,
			Codice: "ACM-R", Tipo: "finito", Origine: "registrato"}}}
		for i := 1; i < n; i++ {
			s.Nodi = append(s.Nodi, Nodo{ChiaveBozza: fmt.Sprintf("n%d", i), Op: OpCrea, Codice: fmt.Sprintf("ACM-%d", i), Tipo: "sciolto", Origine: "utente"})
		}
		return s
	}
	if _, err := LeggiSnapshot(grezzo(t, molti(MaxVoci))); err != nil {
		t.Errorf("%d nodi devono passare: %v", MaxVoci, err)
	}
	if _, err := LeggiSnapshot(grezzo(t, molti(MaxVoci+1))); !errors.Is(err, ErrCorpoNonValido) {
		t.Errorf("%d nodi devono essere rifiutati: %v", MaxVoci+1, err)
	}
	grande := molti(10)
	raw := grezzo(t, grande)
	raw = append(raw[:len(raw)-1], []byte(`,"pad":"`+strings.Repeat("x", MaxSnapshot)+`"}`)...)
	_, err := LeggiSnapshot(raw)
	var e *Errore
	if !errors.As(err, &e) || !strings.Contains(e.Messaggio, "oltre il limite") {
		t.Errorf("uno snapshot oltre %d byte va rifiutato per il peso: %v", MaxSnapshot, err)
	}
}

// La forma canonica non dipende dall'ordine degli elenchi e si rilegge identica (R1 = dati identici).
func TestLaFormaCanonicaNonDipendeDallOrdineESiRileggeIdentica(t *testing.T) {
	sc := scena35(idDi("prodotto"), idDi)
	attesa, err := sc.snap.Canonico()
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewPCG(1, 2))
	for giro := 0; giro < 5; giro++ {
		s := sc.snap.copia()
		r.Shuffle(len(s.Nodi), func(i, j int) { s.Nodi[i], s.Nodi[j] = s.Nodi[j], s.Nodi[i] })
		r.Shuffle(len(s.Archi), func(i, j int) { s.Archi[i], s.Archi[j] = s.Archi[j], s.Archi[i] })
		letta, err := LeggiSnapshot(grezzo(t, s))
		if err != nil {
			t.Fatal(err)
		}
		c, _ := letta.Canonico()
		if !bytes.Equal(c, attesa) {
			t.Fatalf("giro %d: l'ordine degli elenchi cambia la forma canonica", giro)
		}
	}
	riletta, err := LeggiSnapshot(attesa)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := riletta.Canonico()
	if !bytes.Equal(c, attesa) {
		t.Fatal("la forma canonica riletta non e' identica")
	}
}

// RC-03, contratto rev. 3 §6.3: i campi facoltativi assenti, null o vuoti sono la stessa cosa e non si scrivono;
// le stringhe sono in NFC; i numeri delle decisioni sono interi senza esponente (RC-01).
func TestLaFormaCanonicaDelContratto(t *testing.T) {
	sc := scena35(idDi("prodotto"), idDi)
	attesa, _ := sc.snap.Canonico()
	if bytes.Contains(attesa, []byte(`"rev":""`)) || bytes.Contains(attesa, []byte(`"caselle":[]`)) ||
		bytes.Contains(attesa, []byte(`"righe_proposta":[]`)) || bytes.Contains(attesa, []byte(`"descrizione":""`)) {
		t.Errorf("la forma canonica scrive campi facoltativi vuoti: %.300s", attesa)
	}
	if !bytes.Contains(attesa, []byte(`"decisioni":[]`)) || !bytes.Contains(attesa, []byte(`"documenti_preassociati":[]`)) {
		t.Error("i quattro elenchi di testa si scrivono sempre")
	}
	// rev assente, null e vuota; caselle assenti, null e vuote: stessa forma canonica
	for nome, f := range map[string]func(n map[string]any){
		"rev null":      func(n map[string]any) { n["rev"] = nil },
		"rev vuota":     func(n map[string]any) { n["rev"] = "" },
		"caselle null":  func(n map[string]any) { n["caselle"] = nil },
		"caselle vuote": func(n map[string]any) { n["caselle"] = []any{} },
		"righe vuote":   func(n map[string]any) { n["righe_proposta"] = []any{} },
	} {
		m := mappaDi(t, sc.snap)
		f(nodo(m, iNodo(m, "k-p1-1")))
		letta, err := LeggiSnapshot(grezzo(t, m))
		if err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		if c, _ := letta.Canonico(); !bytes.Equal(c, attesa) {
			t.Errorf("%s: la forma canonica cambia", nome)
		}
	}
	// NFC: «è» composta e «e» + accento grave combinante sono la stessa descrizione e la stessa impronta
	composta, scomposta := scena35(idDi("prodotto"), idDi), scena35(idDi("prodotto"), idDi)
	cambiaNodo(&composta.snap, "k-s1", func(n *Nodo) { n.Descrizione = "Telaio in acciaio è zincato" })
	cambiaNodo(&scomposta.snap, "k-s1", func(n *Nodo) { n.Descrizione = "Telaio in acciaio è zincato" })
	a, err := LeggiSnapshot(grezzo(t, composta.snap))
	if err != nil {
		t.Fatal(err)
	}
	b, err := LeggiSnapshot(grezzo(t, scomposta.snap))
	if err != nil {
		t.Fatal(err)
	}
	ia, _ := a.Impronta()
	ib, _ := b.Impronta()
	if ia != ib {
		t.Error("una stringa in NFD e la stessa in NFC danno impronte diverse")
	}
	// RC-01: 1e2, -0, 1E+2, 0.0e5, 1.0 in una decisione si rifiutano; un intero grande si conserva com'e'
	for _, num := range []string{"1e2", "-0", "1E+2", "0.0e5", "1.0", "-1.5"} {
		raw := json.RawMessage(`{"id":"d1","tipo":"arco_condiviso","qta":` + num + `}`)
		if _, _, e := leggiDecisione("decisioni[0]", raw); e == nil {
			t.Errorf("il numero %s in una decisione passa (jsonb lo riscriverebbe)", num)
		}
	}
	c, _, e := leggiDecisione("decisioni[0]", json.RawMessage(`{"tipo":"arco_condiviso","id":"d1","qta":12345678901234567890123,"neg":-7,"zero":0}`))
	if e != nil || string(c) != `{"id":"d1","neg":-7,"qta":12345678901234567890123,"tipo":"arco_condiviso","zero":0}` {
		t.Errorf("decisione con interi: %s %v", c, e)
	}
	// RSC-10: un intero oltre MaxCifre cifre si rifiuta qui (CORPO_NON_VALIDO), non arriva al database (22003)
	if _, _, e := leggiDecisione("decisioni[0]", json.RawMessage(`{"tipo":"arco_condiviso","id":"d1","qta":1`+strings.Repeat("0", MaxCifre)+`}`)); e == nil {
		t.Errorf("un intero di %d cifre in una decisione passa", MaxCifre+1)
	}
	if _, _, e := leggiDecisione("decisioni[0]", json.RawMessage(`{"tipo":"arco_condiviso","id":"d1","qta":-`+strings.Repeat("9", MaxCifre)+`}`)); e != nil {
		t.Errorf("un intero di %d cifre (negativo) si rifiuta: %v", MaxCifre, e)
	}
}

// E2.3 (D-SHOCK-S3 C [U]): un nodo da creare si salva anche senza codice; la posizione del cartiglio resta
// sull'arco e non diventa un codice. La forma canonica non scrive il codice, e si rilegge identica.
func TestUnNodoNuovoSenzaCodiceSiLeggeESiRileggeIdentico(t *testing.T) {
	sc := scena35(idDi("prodotto"), idDi)
	s := sc.snap.copia()
	s.Nodi = append(s.Nodi, Nodo{ChiaveBozza: "k-senza-codice", Op: OpCrea, Descrizione: "Staffa del cartiglio", Tipo: "sciolto", Origine: "utente"})
	s.Archi = append(s.Archi, Arco{Padre: "k-s1", Figlio: "k-senza-codice", Qta: 2, Origine: "utente", Posizione: "12"})
	letta, err := LeggiSnapshot(grezzo(t, s))
	if err != nil {
		t.Fatalf("un nodo crea senza codice non passa: %v", err)
	}
	n := nodoDi(letta, "k-senza-codice")
	if n.Codice != "" || arcoDi(letta, "k-s1", "k-senza-codice").Posizione != "12" {
		t.Errorf("la posizione e' diventata un codice, o il codice e' stato inventato: %+v", n)
	}
	c, _ := letta.Canonico()
	if bytes.Contains(c, []byte(`"chiave_bozza":"k-senza-codice","codice"`)) {
		t.Error("la forma canonica scrive un codice per il nodo che non ce l'ha")
	}
	riletta, err := LeggiSnapshot(c)
	if err != nil {
		t.Fatal(err)
	}
	if c2, _ := riletta.Canonico(); !bytes.Equal(c, c2) {
		t.Error("la bozza con il nodo senza codice non si rilegge identica")
	}
	// solo «crea»: un riuso vuole il codice registrato
	cambiaNodo(&s, "k-s2", func(n *Nodo) { n.Codice = "" })
	if _, err := LeggiSnapshot(grezzo(t, s)); !errors.Is(err, ErrCorpoNonValido) {
		t.Errorf("un riuso senza codice passa: %v", err)
	}
}
