package distintabozza

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// L1 — la riconciliazione degli snapshot con le azioni dichiarate (direttiva 06 B1: «il server confronta snapshot
// precedente e nuovo e valida le azioni[] dichiarate: nessun evento inventato dal browser deve diventare prova senza
// confronto»), in memoria, con il vocabolario 1 del contratto rev. 3 §6.6.

// Il vocabolario e' quello della tabella del contratto rev. 3 §6.6, nome per nome (RC-03).
func TestIlVocabolarioEQuelloDelContratto(t *testing.T) {
	contratto := []string{"bozza_aperta", "sorgente_vista", "sorgente_rifiutata", "sorgente_scelta", "sorgente_cambiata",
		"rebase", "nodo_aggiunto", "nodo_tolto", "nodo_modificato", "nodo_riusato", "arco_aggiunto", "arco_tolto",
		"arco_spostato", "qta_modificata", "decisione_aggiunta", "decisione_tolta", "file_preassociato", "file_svincolato",
		"annulla", "rifai"}
	if !slices.Equal(AzioniDelVocabolario, contratto) {
		t.Errorf("vocabolario %v, contratto %v", AzioniDelVocabolario, contratto)
	}
	for _, a := range AzioniDelVocabolario {
		if !reNomeCampo.MatchString(a) { // = ck_dbe_azione della 0022
			t.Errorf("l'azione %q non passa ck_dbe_azione", a)
		}
	}
	if Vocabolario != 1 {
		t.Errorf("vocabolario %d", Vocabolario)
	}
}

// La scena intera: ogni passo si riconcilia con lo snapshot costruito a mano, la prima versione parte dal
// registrato ricostruito, e il giornale rifatto da capo da' tutte le versioni (prova del rifacimento in memoria;
// quella dopo il riavvio e' nella L4).
func TestLaScenaDei35NodiSiRiconciliaPassoPerPasso(t *testing.T) {
	sc := scena35(idDi("prodotto"), idDi)
	passi := passiScena(t, sc)
	var prima *Snapshot
	var tutti []Evento
	storia := Storia{}
	attesi := map[string]int{"apertura": 6, "tre esclusioni": 9, "due aggiunte": 4, "due ricodifiche": 6, "spostamenti": 7,
		"annulla e rifai fra versioni": 2}
	for i, p := range passi {
		r, err := Riconcilia(prima, p.snap, p.azioni, Contesto{N0: p.n0, Precedente: precedentiDa(tutti)})
		if err != nil {
			t.Fatalf("%s: %v", p.nome, err)
		}
		if len(r.Eventi) != attesi[p.nome] {
			t.Errorf("%s: %d eventi, attesi %d", p.nome, len(r.Eventi), attesi[p.nome])
		}
		if i == 0 {
			if r.Partenza != PartenzaRegistrato || r.StatoDiPartenza == nil {
				t.Fatalf("prima versione senza stato di partenza: %+v", r)
			}
			if e := ControllaPartenza(r.Partenza, *r.StatoDiPartenza, registratoDi(sc)); e != nil {
				t.Fatalf("lo stato di partenza non e' il registrato: %v", e)
			}
		} else if r.StatoDiPartenza != nil {
			t.Errorf("%s: stato di partenza fuori dalla prima versione", p.nome)
		}
		imp, _ := p.snap.Impronta()
		storia.Versioni = append(storia.Versioni, VersioneLetta{Versione: i + 1, Snapshot: p.snap, Impronta: imp})
		for _, e := range r.Eventi {
			tutti = append(tutti, e)
			storia.Eventi = append(storia.Eventi, EventoLetto{N: len(tutti), Versione: i + 1, Vocabolario: Vocabolario, Azione: e.Azione,
				ChiaveBozza: e.ChiaveBozza, Prima: e.Prima, Dopo: e.Dopo, Origine: e.Origine, Motivo: e.Motivo})
		}
		s := p.snap
		prima = &s
	}
	if err := VerificaGiornale(storia); err != nil {
		t.Fatalf("il giornale non si rifa': %v", err)
	}
	ultimo := passi[len(passi)-1].snap
	if len(ultimo.Nodi) != 37 || len(ultimo.Archi) != 33 || len(ultimo.Decisioni) != 5 {
		t.Errorf("alla fine %d nodi, %d archi, %d decisioni: attesi 37, 33, 5", len(ultimo.Nodi), len(ultimo.Archi), len(ultimo.Decisioni))
	}
	// il dopo di bozza_aperta lo scrive il server: partenza, impronta e dimensioni dello stato di partenza
	var d map[string]any
	_ = json.Unmarshal(storia.Eventi[0].Dopo, &d)
	imp0, _ := sc.snap.Impronta()
	if d["partenza"] != PartenzaRegistrato || d["impronta"] != imp0 || d["nodi"].(float64) != 35 || d["archi"].(float64) != 34 {
		t.Errorf("dopo di bozza_aperta: %v", d)
	}
	// un evento guastato nel giornale salvato non si rifa' piu'
	guasto := storia
	guasto.Eventi = append([]EventoLetto(nil), storia.Eventi...)
	for i, e := range guasto.Eventi {
		if e.Azione == AzQtaModificata {
			guasto.Eventi[i].Dopo = json.RawMessage(`{"figlio":"k-p2-3","padre":"k-s2","qta":13}`)
		}
	}
	if err := VerificaGiornale(guasto); err == nil {
		t.Error("un giornale guastato (qta 13 al posto di 12) si rifa' lo stesso")
	}
	// G04: eventi di una versione che non c'e' (l'ultima versione tolta dalla storia, i suoi eventi no)
	senzaUltima := storia
	senzaUltima.Versioni = storia.Versioni[:len(storia.Versioni)-1]
	if err := VerificaGiornale(senzaUltima); err == nil || !strings.Contains(err.Error(), "che non c'e'") {
		t.Errorf("eventi di una versione che non c'e': %v", err)
	}
	// RC-16: un evento di un vocabolario che questo codice non conosce non si rifa' con le regole del vocabolario 1
	altroVoc := storia
	altroVoc.Eventi = append([]EventoLetto(nil), storia.Eventi...)
	altroVoc.Eventi[3].Vocabolario = 2
	if err := VerificaGiornale(altroVoc); err == nil || !strings.Contains(err.Error(), "vocabolario 2") {
		t.Errorf("evento del vocabolario 2: %v", err)
	}
	// numeri degli eventi con un buco
	buco := storia
	buco.Eventi = append([]EventoLetto(nil), storia.Eventi...)
	buco.Eventi[5].N = 99
	if err := VerificaGiornale(buco); err == nil {
		t.Error("un giornale con la numerazione bucata si rifa' lo stesso")
	}
}

func eRiconciliazione(t *testing.T, err error, frammento string) {
	t.Helper()
	var e *Errore
	if !errors.As(err, &e) {
		t.Fatalf("atteso un rifiuto, ottenuto %v", err)
	}
	if e.Codice != CodiceCorpoNonValido {
		t.Fatalf("codice %s: %s", e.Codice, e.Messaggio)
	}
	if !strings.Contains(e.Messaggio, frammento) && !strings.Contains(e.Campo, frammento) {
		t.Errorf("rifiuto %q, atteso che dica %q", e.Error(), frammento)
	}
}

func TestNessunEventoInventatoENessunCambiamentoTaciuto(t *testing.T) {
	sc := scena35(idDi("prodotto"), idDi)
	base := sc.snap
	conNuovoNodo := base.copia()
	n := Nodo{ChiaveBozza: "k-x", Op: OpCrea, Codice: "ACM-X", Tipo: "sciolto", Origine: "utente"}
	conNuovoNodo.Nodi = append(conNuovoNodo.Nodi, n)
	conNuovoNodo.normalizza()
	casi := []struct {
		nome   string
		dopo   Snapshot
		azioni []Azione
		dice   string
	}{
		{"snapshot cambiato senza azioni", conNuovoNodo, nil, "nessuna azione lo aggiunge"},
		{"togli un nodo che non c'e'", base, []Azione{{Azione: AzNodoTolto, Origine: "utente", Prima: grezzo(t, n)}}, "non e' quello che l'azione dice di togliere"},
		{"aggiungi un nodo che c'e' gia'", base, []Azione{{Azione: AzNodoAggiunto, Origine: "utente", Dopo: grezzo(t, nodoDi(base, "k-s1"))}}, "c'e' gia'"},
		{"qta_modificata con la qta di prima sbagliata", base, []Azione{{Azione: AzQtaModificata, Origine: "utente",
			Prima: grezzo(t, map[string]any{"padre": "k-root", "figlio": "k-s1", "qta": 7}),
			Dopo:  grezzo(t, map[string]any{"padre": "k-root", "figlio": "k-s1", "qta": 2})}}, "non ha la qta 7"},
		{"azione vera ma snapshot che non la contiene", base, []Azione{{Azione: AzNodoAggiunto, Origine: "utente", Dopo: grezzo(t, n)}}, "non c'e' piu'"},
		{"origine server dal browser", conNuovoNodo, []Azione{{Azione: AzNodoAggiunto, Origine: "server", Dopo: grezzo(t, n)}}, "origine"},
		{"origine conferma dal browser", conNuovoNodo, []Azione{{Azione: AzNodoAggiunto, Origine: "conferma", Dopo: grezzo(t, n)}}, "origine"},
		{"azione sconosciuta", base, []Azione{{Azione: "fondi_nodi", Origine: "utente"}}, "sconosciuta"},
		{"un nome della rev. 2", conNuovoNodo, []Azione{{Azione: "aggiungi_nodo", Origine: "utente", Dopo: grezzo(t, n)}}, "sconosciuta"},
		{"operazione interna dal browser", base, []Azione{{Azione: azSorgenteRipristinata, Origine: "utente"}}, "sconosciuta"},
		{"bozza_aperta su una bozza che c'e'", base, []Azione{apertura(t, PartenzaVuota)}, "bozza_aperta"},
		{"chiave dichiarata diversa", conNuovoNodo, []Azione{{Azione: AzNodoAggiunto, ChiaveBozza: "k-y", Origine: "utente", Dopo: grezzo(t, n)}}, "dichiara la chiave"},
		{"l'origine con nodo_modificato", base, []Azione{{Azione: AzNodoModificato, ChiaveBozza: "k-s1", Origine: "utente",
			Prima: grezzo(t, map[string]string{"origine": "registrato"}), Dopo: grezzo(t, map[string]string{"origine": "utente"})}}, "non si cambia cosi'"},
		{"il componente con nodo_modificato", base, []Azione{{Azione: AzNodoModificato, ChiaveBozza: "k-s1", Origine: "utente",
			Prima: grezzo(t, map[string]any{"componente_id": idDi("k-s1")}), Dopo: grezzo(t, map[string]any{"componente_id": idDi("altro")})}}, "non si cambia cosi'"},
		{"nodo_modificato con valori di prima falsi", base, []Azione{{Azione: AzNodoModificato, ChiaveBozza: "k-s1", Origine: "utente",
			Prima: grezzo(t, map[string]string{"rev": "Z"}), Dopo: grezzo(t, map[string]string{"rev": "B"})}}, "non ha i valori"},
		{"nodo_modificato che lascia un nodo malformato", base, []Azione{{Azione: AzNodoModificato, ChiaveBozza: "k-s1", Origine: "utente",
			Prima: grezzo(t, map[string]string{"rev": "A"}), Dopo: grezzo(t, map[string]string{"rev": strings.Repeat("R", 11)})}}, ".rev"},
		{"nodo_modificato con un codice nuovo di 41 caratteri", base, []Azione{{Azione: AzNodoModificato, ChiaveBozza: "k-s1", Origine: "utente",
			Prima: grezzo(t, map[string]string{"codice": "ACM-1100"}), Dopo: grezzo(t, map[string]string{"codice": strings.Repeat("C", 41)})}}, "codice nuovo"},
		{"nodo_riusato senza op riusa", base, []Azione{{Azione: AzNodoRiusato, ChiaveBozza: "k-s1", Origine: "utente",
			Prima: grezzo(t, map[string]any{"op": "riusa", "componente_id": idDi("k-s1")}), Dopo: grezzo(t, map[string]any{"op": "modifica", "componente_id": idDi("k-s1b")})}}, "nodo_riusato"},
		{"rifai di un evento che non c'e'", base, []Azione{{Azione: AzRifai, Origine: "utente", Dopo: grezzo(t, map[string]int{"n": 7})}}, "non c'e'"},
		{"annulla senza n", base, []Azione{{Azione: AzAnnulla, Origine: "utente", Dopo: grezzo(t, map[string]int{"azioni": 1})}}, "dopo"},
		{"rifiuto della sorgente senza motivo", base, []Azione{{Azione: AzSorgenteRifiutata, Origine: "utente",
			Dopo: grezzo(t, map[string]any{"allegato_id": idDi("a"), "gruppo": "cand:a:r"})}}, "motivo"},
		{"motivo di soli spazi", conNuovoNodo, []Azione{{Azione: AzNodoAggiunto, Origine: "utente", Motivo: "   ", Dopo: grezzo(t, n)}}, "motivo"},
		{"motivo con un carattere nullo", conNuovoNodo, []Azione{{Azione: AzNodoAggiunto, Origine: "utente", Motivo: "a\x00b", Dopo: grezzo(t, n)}}, "motivo"},
		{"prima e dopo oltre il limite", base, []Azione{{Azione: AzSorgenteVista, Origine: "utente",
			Dopo: json.RawMessage(`"` + strings.Repeat("x", MaxPrimaDopo) + `"`)}}, "oltre"},
		{"campo sconosciuto in dopo", conNuovoNodo, []Azione{{Azione: AzNodoAggiunto, Origine: "utente",
			Dopo: json.RawMessage(`{"chiave_bozza":"k-x","op":"crea","codice":"ACM-X","tipo":"sciolto","origine":"utente","peso":3}`)}}, "dopo"},
		{"campo con le maiuscole in dopo", conNuovoNodo, []Azione{{Azione: AzNodoAggiunto, Origine: "utente",
			Dopo: json.RawMessage(`{"chiave_bozza":"k-x","OP":"crea","codice":"ACM-X","tipo":"sciolto","origine":"utente"}`)}}, "minuscolo"},
		{"base cambiata senza rebase", func() Snapshot { s := base.copia(); s.VersioneBomBase.Contatore = 9; return s }(), nil, "rebase"},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			_, err := Riconcilia(&base, c.dopo, c.azioni, Contesto{})
			eRiconciliazione(t, err, c.dice)
		})
	}
}

// RC-02: la prima versione non si adotta com'e' arriva. Si comincia con bozza_aperta e la partenza; le azioni,
// disfatte dallo snapshot, danno lo stato di partenza; e quello deve essere quello che il server ricostruisce.
func TestLaPrimaVersioneSiConfrontaConLaPartenzaRicostruita(t *testing.T) {
	sc := scena35(idDi("prodotto"), idDi)
	reg := registratoDi(sc)
	if _, err := Riconcilia(nil, sc.snap, nil, Contesto{}); !errors.Is(err, ErrCorpoNonValido) || !strings.Contains(err.Error(), "bozza_aperta") {
		t.Errorf("prima versione senza azioni: %v", err)
	}
	k := nodoDi(sc.snap, "k-s1")
	if _, err := Riconcilia(nil, sc.snap, []Azione{{Azione: AzNodoAggiunto, Origine: "utente", Dopo: grezzo(t, k)}}, Contesto{}); !errors.Is(err, ErrCorpoNonValido) {
		t.Errorf("prima versione senza bozza_aperta: %v", err)
	}
	for _, d := range []string{`{}`, `{"partenza":"browser"}`, `{"partenza":"registrato","nodi":34}`} {
		_, err := Riconcilia(nil, sc.snap, []Azione{{Azione: AzBozzaAperta, Origine: "utente", Dopo: json.RawMessage(d)}}, Contesto{})
		if !errors.Is(err, ErrCorpoNonValido) {
			t.Errorf("bozza_aperta %s: %v", d, err)
		}
	}
	// lo snapshot del registrato, partenza registrato: si'
	r, err := Riconcilia(nil, sc.snap, []Azione{apertura(t, PartenzaRegistrato)}, Contesto{})
	if err != nil {
		t.Fatal(err)
	}
	if e := ControllaPartenza(r.Partenza, *r.StatoDiPartenza, reg); e != nil {
		t.Errorf("partenza registrato: %v", e)
	}
	// RSC-11: nella partenza registrato nodi e archi hanno origine registrato, non conferma (anche con una base che ha
	// conferme, dove lo snapshot ammetterebbe l'origine conferma)
	cid := idDi("conferma-base")
	for nome, cambia := range map[string]func(s *Snapshot){
		"nodo con origine conferma": func(s *Snapshot) { cambiaNodo(s, "k-p2-1", func(n *Nodo) { n.Origine = OrigineConferma }) },
		"arco con origine conferma": func(s *Snapshot) { s.Archi[0].Origine = OrigineConferma },
	} {
		s := sc.snap.copia()
		s.VersioneBomBase = &VersioneBomBase{ConfermaID: &cid, Numero: 1, Contatore: s.VersioneBomBase.Contatore}
		cambia(&s)
		r, err := Riconcilia(nil, s, []Azione{apertura(t, PartenzaRegistrato)}, Contesto{})
		if err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		if e := ControllaPartenza(r.Partenza, *r.StatoDiPartenza, reg); !errors.Is(e, ErrAzioniNonRiconciliate) {
			t.Errorf("partenza registrato, %s: %v", nome, e)
		}
	}
	// un nodo «step» inventato, senza l'azione che lo aggiunge: lo stato di partenza ha un nodo in piu' del registrato
	inventato := sc.snap.copia()
	inventato.Nodi = append(inventato.Nodi, Nodo{ChiaveBozza: "k-fantasma", Op: OpCrea, Codice: "ACM-F", Tipo: "sciolto", Origine: "step"})
	r, err = Riconcilia(nil, inventato, []Azione{apertura(t, PartenzaRegistrato)}, Contesto{})
	if err != nil {
		t.Fatal(err)
	}
	if e := ControllaPartenza(r.Partenza, *r.StatoDiPartenza, reg); !errors.Is(e, ErrAzioniNonRiconciliate) {
		t.Errorf("nodo inventato nella prima versione: %v", e)
	}
	// con l'azione che lo aggiunge, si'
	f := Nodo{ChiaveBozza: "k-fantasma", Op: OpCrea, Codice: "ACM-F", Tipo: "sciolto", Origine: "step"}
	r, err = Riconcilia(nil, inventato, []Azione{apertura(t, PartenzaRegistrato), {Azione: AzNodoAggiunto, Origine: "step", Dopo: grezzo(t, f)}}, Contesto{})
	if err != nil {
		t.Fatal(err)
	}
	if e := ControllaPartenza(r.Partenza, *r.StatoDiPartenza, reg); e != nil {
		t.Errorf("nodo aggiunto con la sua azione: %v", e)
	}
	// un arco con la qta diversa dal registrato, o un codice diverso, o una partenza «vuota» con tutti i nodi: no
	qta := sc.snap.copia()
	for i := range qta.Archi {
		if qta.Archi[i].Figlio == "k-p1-2" {
			qta.Archi[i].Qta = 9
		}
	}
	codice := sc.snap.copia()
	cambiaNodo(&codice, "k-p2-1", func(n *Nodo) { n.Codice = "ACM-ALTRO" })
	for nome, c := range map[string]struct {
		s        Snapshot
		partenza string
	}{"qta": {qta, PartenzaRegistrato}, "codice": {codice, PartenzaRegistrato}, "vuota con tutto": {sc.snap, PartenzaVuota}} {
		r, err := Riconcilia(nil, c.s, []Azione{apertura(t, c.partenza)}, Contesto{})
		if err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		if e := ControllaPartenza(r.Partenza, *r.StatoDiPartenza, reg); !errors.Is(e, ErrAzioniNonRiconciliate) {
			t.Errorf("%s: %v", nome, e)
		}
	}
}

// La partenza «vuota» e le chiavi della pagina: SnapshotDiPartenza scrive la sola radice; le azioni aggiungono il
// resto; ControllaPartenza riconosce la radice dal componente, non dalla chiave.
func TestLaPartenzaVuotaELeChiaviDellaPagina(t *testing.T) {
	sc := scena35(idDi("prodotto"), idDi)
	reg := registratoDi(sc)
	s0 := SnapshotDiPartenza(PartenzaVuota, reg, *sc.snap.VersioneBomBase, sc.snap.FirmaBase, func(uuid.UUID) string { return "radice-della-pagina" })
	if len(s0.Nodi) != 1 || len(s0.Archi) != 0 || s0.Nodi[0].ChiaveBozza != "radice-della-pagina" {
		t.Fatalf("partenza vuota: %+v", s0)
	}
	nuovo := s0.copia()
	x := Nodo{ChiaveBozza: "n1", Op: OpCrea, Tipo: "sciolto", Origine: "utente"} // senza codice: E2.3
	arco := Arco{Padre: "radice-della-pagina", Figlio: "n1", Qta: 3, Origine: "utente", Posizione: "7"}
	nuovo.Nodi = append(nuovo.Nodi, x)
	nuovo.Archi = append(nuovo.Archi, arco)
	r, err := Riconcilia(nil, nuovo, []Azione{apertura(t, PartenzaVuota),
		{Azione: AzNodoAggiunto, Origine: "utente", Dopo: grezzo(t, x)}, {Azione: AzArcoAggiunto, Origine: "utente", Dopo: grezzo(t, arco)}}, Contesto{})
	if err != nil {
		t.Fatal(err)
	}
	if e := ControllaPartenza(PartenzaVuota, *r.StatoDiPartenza, reg); e != nil {
		t.Errorf("partenza vuota con le chiavi della pagina: %v", e)
	}
	if e := ControllaPartenza(PartenzaStep, *r.StatoDiPartenza, reg); !errors.Is(e, ErrPartenzaNonRicostruibile) {
		t.Errorf("partenza step in B1: %v", e)
	}
	// la radice con la descrizione cambiata senza un'azione: non e' il registrato
	guasta := nuovo.copia()
	cambiaNodo(&guasta, "radice-della-pagina", func(n *Nodo) { n.Descrizione = "un'altra" })
	r, err = Riconcilia(nil, guasta, []Azione{apertura(t, PartenzaVuota),
		{Azione: AzNodoAggiunto, Origine: "utente", Dopo: grezzo(t, x)}, {Azione: AzArcoAggiunto, Origine: "utente", Dopo: grezzo(t, arco)}}, Contesto{})
	if err != nil {
		t.Fatal(err)
	}
	if e := ControllaPartenza(PartenzaVuota, *r.StatoDiPartenza, reg); !errors.Is(e, ErrAzioniNonRiconciliate) {
		t.Errorf("radice cambiata senza azione: %v", e)
	}
}

// RC-04: annulla e rifai nominano l'evento, e il server applica l'inverso o lo rifa': non bastano «k azioni dopo».
func TestAnnullaERifaiSiVerificano(t *testing.T) {
	sc := scena35(idDi("prodotto"), idDi)
	base := sc.snap
	n := Nodo{ChiaveBozza: "k-x", Op: OpCrea, Codice: "ACM-X", Tipo: "sciolto", Origine: "utente"}
	conX := base.copia()
	conX.Nodi = append(conX.Nodi, n)
	conX.normalizza()
	agg := Azione{Azione: AzNodoAggiunto, Origine: "utente", Dopo: grezzo(t, n)}
	cx := Contesto{N0: 10}
	// aggiungi (11) e annulla 11: lo snapshot resta quello di prima
	r, err := Riconcilia(&base, base, []Azione{agg, {Azione: AzAnnulla, Origine: "utente", Dopo: grezzo(t, map[string]int{"n": 11})}}, cx)
	if err != nil {
		t.Fatalf("annulla dell'evento appena fatto: %v", err)
	}
	if r.Eventi[1].ChiaveBozza != "k-x" || string(r.Eventi[1].Dopo) != `{"n":11}` || r.Strutturali != 2 {
		t.Errorf("evento dell'annulla: %+v", r.Eventi[1])
	}
	// il vecchio «annulla {azioni: 1}» seguito da un nodo mai visto (la sonda TestRevAnnullaNonVerificato): no
	if _, err := Riconcilia(&base, conX, []Azione{{Azione: AzAnnulla, Origine: "utente", Dopo: grezzo(t, map[string]int{"n": 3})}, agg}, cx); err == nil {
		t.Error("annulla di un evento che non c'e' passa")
	}
	// annulla dichiarato, ma lo snapshot ha ancora il nodo: le azioni non tornano
	if _, err := Riconcilia(&base, conX, []Azione{agg, {Azione: AzAnnulla, Origine: "utente", Dopo: grezzo(t, map[string]int{"n": 11})}}, cx); !errors.Is(err, ErrAzioniNonRiconciliate) {
		t.Errorf("annulla con lo snapshot che non lo vede: %v", err)
	}
	// annulla di se stesso o di un evento dopo: no
	if _, err := Riconcilia(&base, base, []Azione{{Azione: AzAnnulla, Origine: "utente", Dopo: grezzo(t, map[string]int{"n": 11})}}, cx); err == nil {
		t.Error("annulla di se stesso passa")
	}
	// annulla, rifai, annulla dell'evento di una versione precedente (dal Contesto)
	tolto := Evento{Azione: AzNodoTolto, ChiaveBozza: "k-p1-1", Prima: canonico(nodoDi(base, "k-p1-1")), Origine: "utente"}
	senza := base.copia()
	senza.Nodi = slices.DeleteFunc(senza.Nodi, func(x Nodo) bool { return x.ChiaveBozza == "k-p1-1" })
	senza.Archi = rimuoviArco(senza.Archi, "k-s1", "k-p1-1")
	prec := precedentiDa([]Evento{{}, {}, {}, {}, tolto})
	cx5 := Contesto{N0: 5, Precedente: prec}
	if _, err := Riconcilia(&senza, senza, []Azione{{Azione: AzAnnulla, Origine: "utente", Dopo: grezzo(t, map[string]int{"n": 5})}}, cx5); err == nil {
		t.Error("annulla di un nodo_tolto senza l'arco: il nodo torna, ma lo snapshot non lo ha")
	}
	conDiNuovo := senza.copia()
	conDiNuovo.Nodi = append(conDiNuovo.Nodi, nodoDi(base, "k-p1-1"))
	conDiNuovo.normalizza()
	if _, err := Riconcilia(&senza, conDiNuovo, []Azione{{Azione: AzAnnulla, Origine: "utente", Dopo: grezzo(t, map[string]int{"n": 5})}}, cx5); err != nil {
		t.Errorf("annulla di un evento della versione prima: %v", err)
	}
	if _, err := Riconcilia(&senza, senza, []Azione{{Azione: AzAnnulla, Origine: "utente", Dopo: grezzo(t, map[string]int{"n": 5})},
		{Azione: AzRifai, Origine: "utente", Dopo: grezzo(t, map[string]int{"n": 5})}}, cx5); err != nil {
		t.Errorf("annulla e rifai: %v", err)
	}
	// annulla di un evento che non si annulla (un cambio di fonte)
	fonte := Evento{Azione: AzSorgenteScelta, Dopo: canonico(fonteScena()), Origine: "utente"}
	cxF := Contesto{N0: 1, Precedente: precedentiDa([]Evento{fonte})}
	conF := base.copia()
	f := fonteScena()
	conF.Sorgente = &f
	if _, err := Riconcilia(&conF, base, []Azione{{Azione: AzAnnulla, Origine: "utente", Dopo: grezzo(t, map[string]int{"n": 1})}}, cxF); err == nil || !strings.Contains(err.Error(), "non si annulla") {
		t.Errorf("annulla di sorgente_scelta: %v", err)
	}
}

// RC-06: ogni stato intermedio e' una bozza possibile.
func TestGliStatiIntermediSonoPossibili(t *testing.T) {
	sc := scena35(idDi("prodotto"), idDi)
	base := sc.snap
	y := Nodo{ChiaveBozza: "k-y", Op: OpCrea, Codice: "ACM-Y", Tipo: "sciolto", Origine: "utente"}
	conY := base.copia()
	conY.Nodi = append(conY.Nodi, y)
	xy := Arco{Padre: "k-s1", Figlio: "k-y", Qta: 1, Origine: "utente"}
	conY.Archi = append(conY.Archi, xy)
	conY.normalizza()
	// l'arco prima del suo nodo (la sonda TestRevStatiIntermediImpossibili): no; nell'ordine giusto si'
	if _, err := Riconcilia(&base, conY, []Azione{{Azione: AzArcoAggiunto, Origine: "utente", Dopo: grezzo(t, xy)},
		{Azione: AzNodoAggiunto, Origine: "utente", Dopo: grezzo(t, y)}}, Contesto{}); !errors.Is(err, ErrAzioniNonRiconciliate) {
		t.Errorf("arco prima del nodo: %v", err)
	}
	if _, err := Riconcilia(&base, conY, []Azione{{Azione: AzNodoAggiunto, Origine: "utente", Dopo: grezzo(t, y)},
		{Azione: AzArcoAggiunto, Origine: "utente", Dopo: grezzo(t, xy)}}, Contesto{}); err != nil {
		t.Errorf("nodo poi arco: %v", err)
	}
	// un nodo tolto con i suoi archi ancora dentro: no
	p := nodoDi(base, "k-p1-1")
	senza := base.copia()
	senza.Nodi = slices.DeleteFunc(senza.Nodi, func(x Nodo) bool { return x.ChiaveBozza == "k-p1-1" })
	a := arcoDi(base, "k-s1", "k-p1-1")
	senza.Archi = rimuoviArco(senza.Archi, "k-s1", "k-p1-1")
	if _, err := Riconcilia(&base, senza, []Azione{{Azione: AzNodoTolto, Origine: "utente", Prima: grezzo(t, p)},
		{Azione: AzArcoTolto, Origine: "utente", Prima: grezzo(t, a)}}, Contesto{}); !errors.Is(err, ErrAzioniNonRiconciliate) {
		t.Errorf("nodo tolto prima del suo arco: %v", err)
	}
	// la radice non si toglie; un secondo nodo con il componente del prodotto no; un finito non radice no
	if _, err := Riconcilia(&base, base, []Azione{{Azione: AzNodoTolto, Origine: "utente", Prima: grezzo(t, nodoDi(base, "k-root"))}}, Contesto{}); err == nil {
		t.Error("radice tolta")
	}
	if _, err := Riconcilia(&base, base, []Azione{{Azione: AzNodoModificato, ChiaveBozza: "k-s2", Origine: "utente",
		Prima: grezzo(t, map[string]string{"tipo": "sottoassieme"}), Dopo: grezzo(t, map[string]string{"tipo": "finito"})}}, Contesto{}); !errors.Is(err, ErrAzioniNonRiconciliate) {
		t.Errorf("finito non radice in un passo intermedio: %v", err)
	}
	if _, err := Riconcilia(&base, base, []Azione{{Azione: AzNodoRiusato, ChiaveBozza: "k-s2", Origine: "utente",
		Prima: grezzo(t, map[string]any{"op": "riusa", "componente_id": idDi("k-s2")}),
		Dopo:  grezzo(t, map[string]any{"op": "riusa", "componente_id": idDi("prodotto")})}}, Contesto{}); !errors.Is(err, ErrAzioniNonRiconciliate) {
		t.Errorf("seconda radice in un passo intermedio: %v", err)
	}
	// preassociare a un nodo escluso, o escludere un nodo con un documento preassociato: no
	pre := Preassociazione2{PropostaID: idDi("proposta"), ChiaveBozza: "k-s4"}
	escl := base.copia()
	cambiaNodo(&escl, "k-s4", func(n *Nodo) { n.Op = OpEsclude })
	if _, err := Riconcilia(&escl, escl, []Azione{{Azione: AzFilePreassociato, Origine: "utente", Dopo: grezzo(t, pre)},
		{Azione: AzFileSvincolato, Origine: "utente", Prima: grezzo(t, pre)}}, Contesto{}); !errors.Is(err, ErrAzioniNonRiconciliate) {
		t.Errorf("preassociazione a un nodo escluso: %v", err)
	}
	conPre := base.copia()
	conPre.DocumentiPreassociati = []Preassociazione2{pre}
	if _, err := Riconcilia(&conPre, conPre, []Azione{
		{Azione: AzNodoModificato, ChiaveBozza: "k-s4", Origine: "utente", Prima: grezzo(t, map[string]string{"op": "riusa"}), Dopo: grezzo(t, map[string]string{"op": "esclude"})},
		{Azione: AzNodoModificato, ChiaveBozza: "k-s4", Origine: "utente", Prima: grezzo(t, map[string]string{"op": "esclude"}), Dopo: grezzo(t, map[string]string{"op": "riusa"})}}, Contesto{}); !errors.Is(err, ErrAzioniNonRiconciliate) {
		t.Errorf("esclusione di un nodo con un documento: %v", err)
	}
}

// RC-05: le fonti e le basi di passaggio si raccolgono per il controllo sul database.
func TestLeFontiELeBasiDiPassaggioSiCitano(t *testing.T) {
	sc := scena35(idDi("prodotto"), idDi)
	base := sc.snap.copia()
	f1 := fonteScena()
	base.Sorgente = &f1
	g := Sorgente{AllegatoID: pu(idDi("allegato-inventato")), Sha256: shaProva('f'), Gruppo: "cand:g:r"}
	r, err := Riconcilia(&base, base, []Azione{
		{Azione: AzSorgenteCambiata, Origine: "utente", Motivo: "prova", Prima: grezzo(t, f1), Dopo: grezzo(t, g)},
		{Azione: AzSorgenteCambiata, Origine: "utente", Motivo: "torno alla prima", Prima: grezzo(t, g), Dopo: grezzo(t, f1)}}, Contesto{})
	if err != nil {
		t.Fatal(err)
	}
	citaG := false
	for _, c := range r.FontiCitate {
		citaG = citaG || *c.AllegatoID == *g.AllegatoID
	}
	if !citaG {
		t.Errorf("la fonte di passaggio non e' citata: %+v", r.FontiCitate)
	}
	// rebase: le basi di prima e dopo sono citate, e il rebase e' restituito per il confronto con la richiesta
	nuova := base.copia()
	cid := idDi("conferma-1")
	nuova.VersioneBomBase = &VersioneBomBase{ConfermaID: &cid, Numero: 1, Contatore: 7}
	nuova.FirmaBase = shaProva('d')
	reb := Azione{Azione: AzRebase, Origine: "utente", Motivo: "nuova conferma del prodotto",
		Prima: grezzo(t, map[string]any{"base": baseDelloSnapshot(base), "sorgente": f1}),
		Dopo:  grezzo(t, map[string]any{"base": baseDelloSnapshot(nuova), "sorgente": f1, "rif_scaduti": []string{}})}
	r, err = Riconcilia(&base, nuova, []Azione{reb}, Contesto{})
	if err != nil {
		t.Fatalf("rebase: %v", err)
	}
	if len(r.BasiCitate) != 2 || r.BasiCitate[1].ConfermaID == nil || *r.BasiCitate[1].ConfermaID != cid || r.Rebase == nil || r.Rebase.Motivo != "nuova conferma del prodotto" {
		t.Errorf("basi citate %+v, rebase %+v", r.BasiCitate, r.Rebase)
	}
	reb.Motivo = ""
	if _, err := Riconcilia(&base, nuova, []Azione{reb}, Contesto{}); !errors.Is(err, ErrCorpoNonValido) {
		t.Errorf("rebase senza motivo: %v", err)
	}
	if _, err := Riconcilia(&base, nuova, nil, Contesto{}); err == nil || !strings.Contains(err.Error(), "rebase") {
		t.Errorf("base cambiata senza rebase: %v", err)
	}
}

// Le altre azioni: la fonte (scelta senza motivo, cambiata con il motivo), arco_spostato (RC-13), nodo_riusato,
// preassociazioni, decisioni, la destinazione di una fonte secondaria (E2.2), la dichiarazione mono_pezzo
// salvabile anche quando non e' applicabile (E2.12).
func TestLeAltreAzioniDelVocabolario(t *testing.T) {
	sc := scena35(idDi("prodotto"), idDi)
	base := sc.snap
	f1 := Sorgente{AllegatoID: pu(idDi("a1")), Sha256: shaProva('b'), Gruppo: "cand:a1:root"}
	f2 := Sorgente{AllegatoID: pu(idDi("a2")), Sha256: shaProva('c'), Gruppo: "cand:a2:root"}

	// fonte scelta da nessuna fonte: senza motivo; ma non con sorgente_cambiata
	v2 := base.copia()
	v2.Sorgente = &f1
	if _, err := Riconcilia(&base, v2, []Azione{{Azione: AzSorgenteScelta, Origine: "utente", Dopo: grezzo(t, f1)}}, Contesto{}); err != nil {
		t.Fatalf("prima scelta della fonte: %v", err)
	}
	if _, err := Riconcilia(&base, v2, []Azione{{Azione: AzSorgenteCambiata, Origine: "utente", Motivo: "x", Dopo: grezzo(t, f1)}}, Contesto{}); !errors.Is(err, ErrCorpoNonValido) {
		t.Errorf("sorgente_cambiata da nessuna fonte: %v", err)
	}
	// cambio di fonte: con sorgente_scelta no; sorgente_cambiata senza motivo no, con motivo si'
	v3 := v2.copia()
	v3.Sorgente = &f2
	if _, err := Riconcilia(&v2, v3, []Azione{{Azione: AzSorgenteScelta, Origine: "utente", Prima: grezzo(t, f1), Dopo: grezzo(t, f2)}}, Contesto{}); !errors.Is(err, ErrCorpoNonValido) {
		t.Errorf("sorgente_scelta su una fonte gia' scelta: %v", err)
	}
	cambio := Azione{Azione: AzSorgenteCambiata, Origine: "utente", Prima: grezzo(t, f1), Dopo: grezzo(t, f2)}
	if _, err := Riconcilia(&v2, v3, []Azione{cambio}, Contesto{}); !errors.Is(err, ErrCorpoNonValido) {
		t.Errorf("cambio di fonte senza motivo: %v", err)
	}
	cambio.Motivo = "la prima candidata era il file sbagliato"
	if _, err := Riconcilia(&v2, v3, []Azione{cambio}, Contesto{}); err != nil {
		t.Errorf("cambio di fonte con motivo: %v", err)
	}
	if _, err := Riconcilia(&v2, v3, nil, Contesto{}); err == nil || !strings.Contains(err.Error(), "sorgente_cambiata") {
		t.Errorf("fonte cambiata senza azione: %v", err)
	}

	// arco_spostato: la qta puo' cambiare, origine e posizione no (RC-13)
	a := arcoDi(base, "k-s3", "k-p3-1")
	spostato := a
	spostato.Padre, spostato.Qta = "k-s4", 40
	v4 := base.copia()
	v4.Archi = append(rimuoviArco(v4.Archi, "k-s3", "k-p3-1"), spostato)
	if _, err := Riconcilia(&base, v4, []Azione{{Azione: AzArcoSpostato, Origine: "utente", Prima: grezzo(t, a), Dopo: grezzo(t, spostato)}}, Contesto{}); err != nil {
		t.Errorf("spostamento con la qta nuova: %v", err)
	}
	altro := spostato
	altro.Origine, altro.Posizione = "utente", "99"
	v4b := base.copia()
	v4b.Archi = append(rimuoviArco(v4b.Archi, "k-s3", "k-p3-1"), altro)
	if _, err := Riconcilia(&base, v4b, []Azione{{Azione: AzArcoSpostato, Origine: "utente", Prima: grezzo(t, a), Dopo: grezzo(t, altro)}}, Contesto{}); !errors.Is(err, ErrCorpoNonValido) {
		t.Errorf("spostamento che cambia anche origine e posizione (la sonda TestRevSpostaCambiaAncheQta): %v", err)
	}

	// nodo_riusato: un nodo nuovo diventa il riuso di un componente registrato, con il suo codice; annulla lo disfa
	nuovoN := Nodo{ChiaveBozza: "k-n", Op: OpCrea, Tipo: "sciolto", Origine: "step"}
	conN := base.copia()
	conN.Nodi = append(conN.Nodi, nuovoN)
	conN.normalizza()
	riusato := conN.copia()
	cid := idDi("registrato-lungo")
	codiceLungo := "ACM-" + strings.Repeat("9", 50) // registrato, oltre i 40 dei codici nuovi (Δ31)
	cambiaNodo(&riusato, "k-n", func(n *Nodo) { n.Op, n.ComponenteID, n.Codice = OpRiusa, &cid, codiceLungo })
	riuso := Azione{Azione: AzNodoRiusato, ChiaveBozza: "k-n", Origine: "utente",
		Prima: grezzo(t, map[string]any{"op": "crea", "componente_id": nil, "codice": nil}),
		Dopo:  grezzo(t, map[string]any{"op": "riusa", "componente_id": cid, "codice": codiceLungo})}
	r, err := Riconcilia(&conN, riusato, []Azione{riuso}, Contesto{})
	if err != nil {
		t.Fatalf("nodo_riusato: %v", err)
	}
	if _, err := Riconcilia(&riusato, conN, []Azione{{Azione: AzAnnulla, Origine: "utente", Dopo: grezzo(t, map[string]int{"n": 1})}},
		Contesto{N0: 1, Precedente: precedentiDa(r.Eventi)}); err != nil {
		t.Errorf("annulla di nodo_riusato (rimette crea senza codice): %v", err)
	}

	// preassocia e svincola, decisione aggiunta e tolta; la chiave della decisione e' quella del suo nodo
	v5 := base.copia()
	d1 := map[string]any{"id": "d-1", "tipo": "da_valutare", "chiave_bozza": "k-s2"}
	pre := Preassociazione2{PropostaID: idDi("proposta"), ChiaveBozza: "k-s4"}
	az := []Azione{
		{Azione: AzFilePreassociato, Origine: "utente", Dopo: grezzo(t, pre)},
		{Azione: AzFileSvincolato, Origine: "utente", Prima: grezzo(t, pre)},
		{Azione: AzDecisioneAggiunta, Origine: "utente", Dopo: grezzo(t, d1)},
		{Azione: AzDecisioneTolta, Origine: "utente", Prima: grezzo(t, d1)},
	}
	r, err = Riconcilia(&base, v5, az, Contesto{})
	if err != nil {
		t.Fatalf("azioni miste: %v", err)
	}
	if r.Strutturali != len(az) || r.Eventi[0].ChiaveBozza != "k-s4" || r.Eventi[2].ChiaveBozza != "k-s2" || r.Eventi[3].ChiaveBozza != "k-s2" {
		t.Errorf("strutturali %d, chiavi %q %q %q", r.Strutturali, r.Eventi[0].ChiaveBozza, r.Eventi[2].ChiaveBozza, r.Eventi[3].ChiaveBozza)
	}
	v6 := base.copia()
	v6.Decisioni = append(v6.Decisioni, canonico(d1))
	if _, err := Riconcilia(&base, v6, []Azione{{Azione: AzDecisioneAggiunta, ChiaveBozza: "k-s9", Origine: "utente", Dopo: grezzo(t, d1)}}, Contesto{}); err == nil {
		t.Error("decisione dichiarata con la chiave di un altro nodo")
	}

	// E2.12 (D-P4A-03 A' [U]): la dichiarazione mono_pezzo motivata si salva in bozza anche se non e' applicabile (il
	// prodotto ha figli, lo STEP non e' autorizzato): la bozza la conserva, il riepilogo e la conferma la giudicano
	mono := map[string]any{"id": "d-mono", "tipo": "mono_pezzo", "motivo": "pezzo unico tornito dal pieno"}
	v7 := base.copia()
	v7.Decisioni = append(v7.Decisioni, canonico(mono))
	if _, err := Riconcilia(&base, v7, []Azione{{Azione: AzDecisioneAggiunta, Origine: "utente", Dopo: grezzo(t, mono)}}, Contesto{}); err != nil {
		t.Errorf("dichiarazione mono_pezzo non applicabile: %v", err)
	}

	// E2.2 (D-SHOCK-S2 B [U]): la fonte vista o rifiutata per un sottoassieme ha la destinazione; un nodo che esiste
	// solo nella bozza la ha con la sua chiave. Senza chiave e' la fonte del prodotto; la radice come destinazione no.
	sec := map[string]any{"allegato_id": idDi("step-secondario"), "gruppo": "cand:sec:ACM-1300"}
	r, err = Riconcilia(&conN, conN, []Azione{
		{Azione: AzSorgenteVista, ChiaveBozza: "k-n", Origine: "utente", Dopo: grezzo(t, sec)},
		{Azione: AzSorgenteRifiutata, ChiaveBozza: "k-n", Origine: "utente", Motivo: "geometria multicorpo, nessun arco", Dopo: grezzo(t, sec)},
		{Azione: AzSorgenteVista, Origine: "utente", Dopo: grezzo(t, sec)}}, Contesto{})
	if err != nil {
		t.Fatalf("fonte secondaria con destinazione temporanea: %v", err)
	}
	if r.Eventi[0].ChiaveBozza != "k-n" || r.Eventi[1].ChiaveBozza != "k-n" || r.Eventi[2].ChiaveBozza != "" || r.Strutturali != 0 || len(r.FontiCitate) != 3 {
		t.Errorf("destinazioni %q %q %q, strutturali %d, fonti %d", r.Eventi[0].ChiaveBozza, r.Eventi[1].ChiaveBozza, r.Eventi[2].ChiaveBozza, r.Strutturali, len(r.FontiCitate))
	}
	for _, k := range []string{"k-root", "k-nessuno"} {
		if _, err := Riconcilia(&conN, conN, []Azione{{Azione: AzSorgenteVista, ChiaveBozza: k, Origine: "utente", Dopo: grezzo(t, sec)}}, Contesto{}); err == nil {
			t.Errorf("destinazione %s accettata", k)
		}
	}

	// uno snapshot identico e nessuna azione: niente da dire, nessun errore
	if r, err := Riconcilia(&base, base, nil, Contesto{}); err != nil || len(r.Eventi) != 0 {
		t.Errorf("snapshot identico: %v, %d eventi", err, len(r.Eventi))
	}
}
