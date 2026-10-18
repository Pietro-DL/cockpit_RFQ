package distintabozza

// Le prove L1 della riconciliazione del formato 3 (V1, ondata B; R2 forma salvataggio, EM §9.2, vocabolario 2): le azioni
// rifatte sullo snapshot salvato, la pila d'annullamento che si ferma all'ultima versione costruita dal server e il prima
// {azione_id} dell'annullamento (PE-05), lo svincolo per nodo escluso (PE-22), il rebase che cambia solo la base, le
// contraddizioni. Dati inventati (gli aiuti di formato3_aiuti_test.go).

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// azione3 costruisce un'azione del vocabolario 2 con prima e dopo dati come valori Go (nil = assente), come li manda la
// pagina (JSON qualsiasi: la forma canonica la scrive il server).
func azione3(t *testing.T, verbo, chiave string, prima, dopo any) Azione {
	t.Helper()
	a := Azione{Azione: verbo, ChiaveBozza: chiave, Origine: OrigineUtente}
	for _, x := range []struct {
		v   any
		dst *json.RawMessage
	}{{prima, &a.Prima}, {dopo, &a.Dopo}} {
		if x.v == nil {
			continue
		}
		b, err := json.Marshal(x.v)
		if err != nil {
			t.Fatal(err)
		}
		*x.dst = b
	}
	return a
}

// errore3: l'errore tipizzato, o la prova si ferma.
func errore3(t *testing.T, err error) *Errore {
	t.Helper()
	var e *Errore
	if err == nil || !errorsAs(err, &e) {
		t.Fatalf("atteso un *Errore, avuto %v", err)
	}
	return e
}

const (
	azioneA = "00000000-0000-4000-8000-0000000a0001"
	azioneB = "00000000-0000-4000-8000-0000000a0002"
)

func nodoManuale3(id idProva3, chiave, codice string) map[string]any {
	return map[string]any{"chiave_bozza": chiave, "op": "crea", "tipo": "commerciale", "origine": "utente",
		"identita": map[string]any{"emittente": id.clienteE(), "emittente_presunto": true, "codice": codice, "modo": "manuale"}}
}

func conCampo(t *testing.T, m map[string]any, k string, v any) map[string]any {
	t.Helper()
	c := copiaProfonda(t, m)
	c[k] = v
	return c
}

func TestV1Riconcilia3UnNodoEUnArcoNuovi(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	nuovo := nodoManuale3(id, "k-u1", "ACM-2000")
	arco := map[string]any{"padre": "k-root", "figlio": "k-u1", "qta": 2, "origine": "utente"}
	d := copiaProfonda(t, base)
	d["nodi"] = append(d["nodi"].([]any), nuovo)
	d["archi"] = append(d["archi"].([]any), arco)
	r, err := Riconcilia3(leggiProva3(t, base), leggiProva3(t, d), []Azione{
		azione3(t, AzNodoAggiunto, "k-u1", nil, conCampo(t, nuovo, "azione_id", azioneA)),
		azione3(t, AzArcoAggiunto, "k-u1", nil, conCampo(t, arco, "azione_id", azioneA)),
	}, Contesto3{N0: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Eventi) != 2 || r.Strutturali != 2 {
		t.Fatalf("eventi %d, strutturali %d", len(r.Eventi), r.Strutturali)
	}
	for i, ev := range r.Eventi {
		if ev.ChiaveBozza != "k-u1" || !strings.Contains(string(ev.Dopo), azioneA) || ev.Prima != nil {
			t.Errorf("evento %d: %+v (la chiave dedotta, l'azione_id nel dopo, niente prima)", i, ev)
		}
	}
	// lo snapshot salvato non porta l'azione_id
	s := leggiProva3(t, d)
	can, _ := s.Canonico()
	if strings.Contains(string(can), azioneA) {
		t.Error("l'azione_id e' finita nello snapshot")
	}
}

func TestV1Riconcilia3LoSnapshotDeveVenireDalleAzioni(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	d := copiaProfonda(t, base)
	d["nodi"] = append(d["nodi"].([]any), nodoManuale3(id, "k-u1", "ACM-2000"))
	_, err := Riconcilia3(leggiProva3(t, base), leggiProva3(t, d), nil, Contesto3{N0: 4})
	e := errore3(t, err)
	if e.Codice != CodiceCorpoNonValido || e.Motivo != MotivoAzioniNonRiconciliate || e.Campo != "snapshot.nodi" {
		t.Fatalf("errore %+v", e)
	}
	// la sorgente cambiata senza cambia_sorgente: rebase_cambia_sorgente (EM §9.2)
	d = copiaProfonda(t, base)
	d["sorgente"].(map[string]any)["importazione"] = strings.Repeat("f", 64)
	_, err = Riconcilia3(leggiProva3(t, base), leggiProva3(t, d), nil, Contesto3{N0: 4})
	if e := errore3(t, err); e.Motivo != MotivoRebaseCambiaSorgente || e.Campo != "snapshot.sorgente" {
		t.Fatalf("errore %+v", e)
	}
}

func TestV1Riconcilia3IVerbiDelServerNonVengonoDalBrowser(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	s := leggiProva3(t, snapshotProva3(id, imp))
	for _, verbo := range AzioniSoloDelServer2 {
		_, err := Riconcilia3(s, s, []Azione{azione3(t, verbo, "", nil, map[string]any{})}, Contesto3{N0: 4})
		if e := errore3(t, err); e.Codice != CodiceCorpoNonValido || e.Campo != "azioni[0].azione" {
			t.Errorf("%s: %+v", verbo, e)
		}
	}
	a := azione3(t, AzFileLasciato, "", nil, map[string]any{"proposta_id": "00000000-0000-4000-8000-0000000d0001"})
	a.Origine = "server"
	_, err := Riconcilia3(s, s, []Azione{a}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Campo != "azioni[0].origine" {
		t.Errorf("origine server dal browser: %+v", e)
	}
	_, err = Riconcilia3(s, s, []Azione{azione3(t, "nodo_cancellato", "", nil, nil)}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Campo != "azioni[0].azione" {
		t.Errorf("verbo sconosciuto: %+v", e)
	}
}

// la pila d'annullamento (EM §9.2, PE-05): annulla e rifai dell'evento 5, un nodo_aggiunto dell'ingegnere con l'azione_id
// del gesto; l'evento 1 apre la versione costruita dal server.
func TestV1Riconcilia3AnnullaERifaiConLAzioneDelGesto(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	nuovo := nodoManuale3(id, "k-u1", "ACM-2000")
	conNodo := copiaProfonda(t, base)
	conNodo["nodi"] = append(conNodo["nodi"].([]any), nuovo)
	sNodo, sBase := leggiProva3(t, conNodo), leggiProva3(t, base)
	precedenti := map[int]EventoLetto{
		1: {N: 1, Azione: AzBozzaAperta, Origine: "server", Dopo: rawDi(t, map[string]any{"partenza": "nuova"})},
		5: {N: 5, Azione: AzNodoAggiunto, ChiaveBozza: "k-u1", Origine: OrigineUtente, Dopo: rawDi(t, conCampo(t, nuovo, "azione_id", azioneA))},
	}
	cx := Contesto3{N0: 5, LimiteServer: 1, Precedente: func(n int) (EventoLetto, bool, error) {
		e, ok := precedenti[n]
		return e, ok, nil
	}}

	// annulla con il prima {azione_id}: il nodo esce, l'evento porta il prima scritto dal server e il dopo {n}
	r, err := Riconcilia3(sNodo, sBase, []Azione{azione3(t, AzAnnulla, "", map[string]any{"azione_id": azioneA}, map[string]any{"n": 5})}, cx)
	if err != nil {
		t.Fatal(err)
	}
	ev := r.Eventi[0]
	if ev.ChiaveBozza != "k-u1" || string(ev.Dopo) != `{"n":5}` || string(ev.Prima) != `{"azione_id":"`+azioneA+`"}` {
		t.Fatalf("evento dell'annullamento: chiave %q prima %s dopo %s", ev.ChiaveBozza, ev.Prima, ev.Dopo)
	}
	// annulla con il prima null: il server scrive comunque l'azione_id dell'evento
	r, err = Riconcilia3(sNodo, sBase, []Azione{azione3(t, AzAnnulla, "", nil, map[string]any{"n": 5})}, cx)
	if err != nil {
		t.Fatal(err)
	}
	if string(r.Eventi[0].Prima) != `{"azione_id":"`+azioneA+`"}` {
		t.Fatalf("prima dell'annullamento senza azione dichiarata: %s", r.Eventi[0].Prima)
	}
	// un'altra azione_id: azioni_non_riconciliate sul campo del prima
	_, err = Riconcilia3(sNodo, sBase, []Azione{azione3(t, AzAnnulla, "", map[string]any{"azione_id": azioneB}, map[string]any{"n": 5})}, cx)
	if e := errore3(t, err); e.Motivo != MotivoAzioniNonRiconciliate || e.Campo != "azioni[0].prima.azione_id" {
		t.Fatalf("azione_id diversa: %+v", e)
	}
	// il prima con altro oltre l'azione_id: 422 di forma
	_, err = Riconcilia3(sNodo, sBase, []Azione{azione3(t, AzAnnulla, "", map[string]any{"azione_id": azioneA, "n": 5}, map[string]any{"n": 5})}, cx)
	if e := errore3(t, err); e.Codice != CodiceCorpoNonValido || !strings.HasPrefix(e.Campo, "azioni[0].prima") {
		t.Fatalf("prima con altri campi: %+v", e)
	}
	// annulla e poi rifai dello stesso evento: lo snapshot torna quello di prima
	r, err = Riconcilia3(sNodo, sNodo, []Azione{
		azione3(t, AzAnnulla, "", nil, map[string]any{"n": 5}),
		azione3(t, AzRifai, "", map[string]any{"azione_id": azioneA}, map[string]any{"n": 5}),
	}, cx)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Eventi) != 2 || r.Eventi[1].Azione != AzRifai || string(r.Eventi[1].Dopo) != `{"n":5}` {
		t.Fatalf("annulla e rifai: %+v", r.Eventi)
	}
}

func TestV1Riconcilia3LaPilaSiFermaAllaVersioneDelServer(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	s := leggiProva3(t, snapshotProva3(id, imp))
	nuovo := nodoManuale3(id, "k-u1", "ACM-2000")
	precedenti := map[int]EventoLetto{
		2: {N: 2, Azione: AzNodoAggiunto, ChiaveBozza: "k-u1", Origine: OrigineUtente, Dopo: rawDi(t, nuovo)},
		3: {N: 3, Azione: AzSorgenteCambiata, Origine: "server", Dopo: rawDi(t, map[string]any{})},
		4: {N: 4, Azione: AzNodoTolto, ChiaveBozza: "k-u1", Origine: "server", Prima: rawDi(t, nuovo)},
	}
	cx := Contesto3{N0: 4, LimiteServer: 3, Precedente: func(n int) (EventoLetto, bool, error) {
		e, ok := precedenti[n]
		return e, ok, nil
	}}
	for _, n := range []int{2, 3, 4} {
		for _, verbo := range []string{AzAnnulla, AzRifai} {
			_, err := Riconcilia3(s, s, []Azione{azione3(t, verbo, "", nil, map[string]any{"n": n})}, cx)
			e := errore3(t, err)
			if e.Codice != CodiceCorpoNonValido || e.Motivo != MotivoAnnullamentoNonAmmesso {
				t.Errorf("%s dell'evento %d: %+v (annullamento_non_ammesso)", verbo, n, e)
			}
		}
	}
	// un evento che non c'e'
	_, err := Riconcilia3(s, s, []Azione{azione3(t, AzAnnulla, "", nil, map[string]any{"n": 9})}, cx)
	if e := errore3(t, err); e.Motivo != MotivoAzioniNonRiconciliate {
		t.Errorf("evento assente: %+v", e)
	}
}

// PE-22: lo svincolo di una preassociazione con il motivo nodo_escluso; un motivo fuori dall'elenco chiuso e' 422.
func TestV1Riconcilia3LoSvincoloPerNodoEscluso(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	d := copiaProfonda(t, base)
	d["documenti_preassociati"] = []any{}
	p := "00000000-0000-4000-8000-0000000d0001"
	prima := map[string]any{"proposta_id": p, "chiave_bozza": "k-0007"}
	r, err := Riconcilia3(leggiProva3(t, base), leggiProva3(t, d), []Azione{
		azione3(t, AzFileSvincolato, "k-0007", prima, map[string]any{"proposta_id": p, "motivo": SvincoloNodoEscluso}),
	}, Contesto3{N0: 4})
	if err != nil {
		t.Fatal(err)
	}
	var dopo DopoSvincolo
	if err := json.Unmarshal(r.Eventi[0].Dopo, &dopo); err != nil || dopo.Motivo != SvincoloNodoEscluso || r.Eventi[0].ChiaveBozza != "k-0007" {
		t.Fatalf("evento: %+v", r.Eventi[0])
	}
	if len(r.ProposteCitate) != 1 || r.ProposteCitate[0] != uuid.MustParse(p) {
		t.Fatalf("proposte citate: %v", r.ProposteCitate)
	}
	_, err = Riconcilia3(leggiProva3(t, base), leggiProva3(t, d), []Azione{
		azione3(t, AzFileSvincolato, "k-0007", prima, map[string]any{"proposta_id": p, "motivo": "escluso"}),
	}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Campo != "azioni[0].dopo.motivo" {
		t.Fatalf("motivo fuori elenco: %+v", e)
	}
	// «Lascia senza associazione» non cambia lo snapshot e non conta fra le azioni strutturali
	r, err = Riconcilia3(leggiProva3(t, d), leggiProva3(t, d), []Azione{
		azione3(t, AzFileLasciato, "", nil, map[string]any{"proposta_id": p}),
	}, Contesto3{N0: 5})
	if err != nil || r.Strutturali != 0 || len(r.Eventi) != 1 {
		t.Fatalf("file_lasciato: %v %+v", err, r)
	}
}

// «Non includere» nell'ordine della pagina (arco_tolto, nodo_tolto, poi la decisione), con la stessa azione_id; la
// decisione non_includere non ha chiave_bozza, e la chiave del suo evento e' null (RPC PD-08, raccomandata (a), come il
// vocabolario 1): una chiave dichiarata e' azioni_non_riconciliate; la decisione non_includere sul rif di una casella
// della bozza e' una contraddizione (EM §7.2).
func TestV1Riconcilia3NonIncludereNellOrdineDellaPagina(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	s := leggiProva3(t, base)
	step := map[string]any{"allegato_id": id.allegato.String(), "gruppo": gruppoProva3}
	arco := map[string]any{"padre": "k-0007", "figlio": "k-0011", "qta": 3, "origine": "step", "step": step}
	nodo := nodoIn3(base, "k-0011")
	dec := decisioneIn3(base, "di-0002")
	ni := map[string]any{"id": "ni-0002", "tipo": "non_includere",
		"casella":       map[string]any{"allegato_id": id.allegato.String(), "gruppo": gruppoProva3, "rif": id.rif("#11")},
		"motivo_codice": "duplicato_cad", "motivo": "doppione del CAD", "azione_id": azioneA}
	d := copiaProfonda(t, base)
	var nodi, archi, decisioni []any
	for _, n := range d["nodi"].([]any) {
		if n.(map[string]any)["chiave_bozza"] != "k-0011" {
			nodi = append(nodi, n)
		}
	}
	for _, a := range d["archi"].([]any) {
		if a.(map[string]any)["figlio"] != "k-0011" {
			archi = append(archi, a)
		}
	}
	for _, x := range d["decisioni"].([]any) {
		if x.(map[string]any)["id"] != "di-0002" {
			decisioni = append(decisioni, x)
		}
	}
	d["nodi"], d["archi"], d["decisioni"] = nodi, archi, append(decisioni, ni)
	gesto := func(chiaveDecisione string) []Azione {
		return []Azione{
			azione3(t, AzArcoTolto, "k-0011", arco, map[string]any{"azione_id": azioneA}),
			azione3(t, AzDecisioneTolta, "k-0011", dec, nil),
			azione3(t, AzNodoTolto, "k-0011", nodo, map[string]any{"azione_id": azioneA}),
			azione3(t, AzDecisioneAggiunta, chiaveDecisione, nil, ni),
		}
	}
	r, err := Riconcilia3(s, leggiProva3(t, d), gesto(""), Contesto3{N0: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Eventi) != 4 || r.Strutturali != 4 {
		t.Fatalf("eventi %d strutturali %d", len(r.Eventi), r.Strutturali)
	}
	for i, ev := range r.Eventi {
		atteso := "k-0011" // la chiave dedotta: il figlio dell'arco, il nodo, la chiave_bozza della decisione identita
		if i == 3 {
			atteso = "" // la non_includere non ha chiave_bozza: null
		}
		if ev.ChiaveBozza != atteso {
			t.Errorf("evento %d (%s): chiave %q, attesa %q", i, ev.Azione, ev.ChiaveBozza, atteso)
		}
	}
	// PD-08 (a): la chiave del nodo tolto dichiarata sulla non_includere non entra nel giornale
	_, err = Riconcilia3(s, leggiProva3(t, d), gesto("k-0011"), Contesto3{N0: 4})
	if e := errore3(t, err); e.Motivo != MotivoAzioniNonRiconciliate || e.Campo != "azioni[3].chiave_bozza" {
		t.Fatalf("chiave dichiarata sulla non_includere: %+v", e)
	}
	// la decisione prima del nodo tolto: il rif sta ancora nelle caselle
	_, err = Riconcilia3(s, s, []Azione{azione3(t, AzDecisioneAggiunta, "", nil, ni)}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Codice != CodiceCorpoNonValido || e.Campo != "azioni[0].dopo.casella.rif" {
		t.Fatalf("non incluso e incluso insieme: %+v", e)
	}
	// il nodo tolto con l'arco ancora dentro
	_, err = Riconcilia3(s, s, []Azione{azione3(t, AzNodoTolto, "k-0011", nodo, nil)}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Motivo != MotivoAzioniNonRiconciliate {
		t.Fatalf("nodo tolto con l'arco: %+v", e)
	}
	// la radice non si toglie
	_, err = Riconcilia3(s, s, []Azione{azione3(t, AzNodoTolto, "k-root", nodoIn3(base, "k-root"), nil)}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Motivo != MotivoAzioniNonRiconciliate {
		t.Fatalf("radice tolta: %+v", e)
	}
}

// il rebase del formato 3 cambia solo la base (EM §9.2): la base di prima e' quella della bozza; una sorgente diversa
// nel prima o nel dopo e' rebase_cambia_sorgente.
func TestV1Riconcilia3IlRebaseCambiaSoloLaBase(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	firma2 := strings.Repeat("2", 64)
	d := copiaProfonda(t, base)
	d["versione_bom_base"] = map[string]any{"conferma_id": nil, "numero": 0, "contatore": 13}
	d["firma_base"] = firma2
	vecchia := map[string]any{"conferma_id": nil, "numero": 0, "contatore": 12, "firma_struttura": firmaProva}
	nuova := map[string]any{"conferma_id": nil, "numero": 0, "contatore": 13, "firma_struttura": firma2}
	a := azione3(t, AzRebase, "", map[string]any{"base": vecchia}, map[string]any{"base": nuova, "rif_scaduti": []any{}})
	a.Motivo = "la struttura del prodotto e' cambiata"
	r, err := Riconcilia3(leggiProva3(t, base), leggiProva3(t, d), []Azione{a}, Contesto3{N0: 4})
	if err != nil {
		t.Fatal(err)
	}
	if r.Rebase == nil || r.Rebase.Base.Contatore != 13 || r.Rebase.Base.FirmaStruttura != firma2 || r.Rebase.Motivo != a.Motivo {
		t.Fatalf("rebase: %+v", r.Rebase)
	}
	// la base cambiata senza il rebase
	_, err = Riconcilia3(leggiProva3(t, base), leggiProva3(t, d), nil, Contesto3{N0: 4})
	if e := errore3(t, err); e.Campo != "rebase" || e.Motivo != MotivoAzioniNonRiconciliate {
		t.Fatalf("base senza rebase: %+v", e)
	}
	// una sorgente diversa nel prima del rebase
	altra := map[string]any{"allegato_id": id.pdf.String(), "sha256": id.shaPdf, "gruppo": gruppoProva3, "importazione": imp}
	b := azione3(t, AzRebase, "", map[string]any{"base": vecchia, "sorgente": altra}, map[string]any{"base": nuova, "rif_scaduti": []any{}})
	b.Motivo = a.Motivo
	_, err = Riconcilia3(leggiProva3(t, base), leggiProva3(t, d), []Azione{b}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Motivo != MotivoRebaseCambiaSorgente {
		t.Fatalf("rebase con un'altra sorgente: %+v", e)
	}
	// i rif scaduti del dopo li scrive il server: l'evento ha l'elenco vuoto, e un elenco dichiarato non vuoto e' un rebase
	// che non si salva (422 rif_scaduti)
	if campoJSON3(t, r.Eventi[0].Dopo, "rif_scaduti") != `[]` {
		t.Fatalf("dopo del rebase: %s", r.Eventi[0].Dopo)
	}
	scaduto := map[string]any{"chiave_bozza": "k-0009", "casella": map[string]any{"allegato_id": id.allegato.String(), "gruppo": gruppoProva3,
		"rif": id.rif("#9")}, "motivo": ScadutoChiaveAssente}
	conScaduti := azione3(t, AzRebase, "", map[string]any{"base": vecchia}, map[string]any{"base": nuova, "rif_scaduti": []any{scaduto}})
	conScaduti.Motivo = a.Motivo
	_, err = Riconcilia3(leggiProva3(t, base), leggiProva3(t, d), []Azione{conScaduti}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Motivo != MotivoRifScaduti || e.Campo != "azioni[0].dopo.rif_scaduti" {
		t.Fatalf("rebase con rif scaduti dichiarati: %+v", e)
	}
	// il rebase senza motivo
	c := a
	c.Motivo = ""
	_, err = Riconcilia3(leggiProva3(t, base), leggiProva3(t, d), []Azione{c}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Campo != "azioni[0].motivo" {
		t.Fatalf("rebase senza motivo: %+v", e)
	}
	// il campo rebase della richiesta e l'azione devono dire la stessa cosa
	if e := rebaseCoerente3(&RebaseRichiesta3{Base: r.Rebase.Base, Motivo: a.Motivo}, r.Rebase); e != nil {
		t.Fatalf("rebase coerente: %+v", e)
	}
	if e := rebaseCoerente3(nil, r.Rebase); e == nil || e.Campo != "rebase" {
		t.Fatalf("rebase senza il campo: %+v", e)
	}
	altro := r.Rebase.Base
	altro.Contatore = 14
	if e := rebaseCoerente3(&RebaseRichiesta3{Base: altro, Motivo: a.Motivo}, r.Rebase); e == nil {
		t.Fatal("campo rebase diverso dall'azione accettato")
	}
}

// campoJSON3: un campo di un oggetto JSON, nella sua forma canonica.
func campoJSON3(t *testing.T, raw json.RawMessage, k string) string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s non e' un oggetto: %v", raw, err)
	}
	return string(canonico(m[k]))
}

// nodo_modificato e nodo_riusato: prima e dopo nominano gli stessi campi, almeno uno, fra quelli del verbo (EM §9.2, come
// il vocabolario 1); la chiave e l'origine non si cambiano mai (EM §3.4). Rilievo bloccante della revisione dell'ondata B:
// un campo del dopo che il prima non nomina entrava nello snapshot ma non nell'evento, e il giornale non rifaceva la
// versione (T-10).
func TestV1Riconcilia3NodoModificatoNominaGliStessiCampi(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	conTipo := func(extra map[string]any) map[string]any {
		d := copiaProfonda(t, base)
		n := nodoIn3(d, "k-man")
		n["tipo"] = "sciolto"
		for k, v := range extra {
			n[k] = v
		}
		return d
	}
	casi := []struct {
		nome        string
		prima, dopo map[string]any
		snap        map[string]any
	}{
		{"un campo del dopo che il prima non nomina", map[string]any{"tipo": "commerciale"},
			map[string]any{"tipo": "sciolto", "descrizione": "aggiunta senza dirlo"}, conTipo(map[string]any{"descrizione": "aggiunta senza dirlo"})},
		{"l'origine, nominata solo nel dopo", map[string]any{"tipo": "commerciale"},
			map[string]any{"tipo": "sciolto", "origine": "registrato"}, conTipo(map[string]any{"origine": "registrato"})},
		{"l'origine, nominata nel prima e nel dopo", map[string]any{"tipo": "commerciale", "origine": "utente"},
			map[string]any{"tipo": "sciolto", "origine": "registrato"}, conTipo(map[string]any{"origine": "registrato"})},
		{"la chiave, nominata solo nel dopo", map[string]any{"tipo": "commerciale"},
			map[string]any{"tipo": "sciolto", "chiave_bozza": "k-man2"}, base},
		{"la chiave, nominata nel prima e nel dopo", map[string]any{"tipo": "commerciale", "chiave_bozza": "k-man"},
			map[string]any{"tipo": "sciolto", "chiave_bozza": "k-man2"}, base},
		{"prima e dopo vuoti", map[string]any{}, map[string]any{}, base},
	}
	for _, c := range casi {
		_, err := Riconcilia3(leggiProva3(t, base), leggiProva3(t, c.snap), []Azione{azione3(t, AzNodoModificato, "k-man", c.prima, c.dopo)}, Contesto3{N0: 4})
		if e := errore3(t, err); e.Codice != CodiceCorpoNonValido || e.Campo != "azioni[0]" {
			t.Errorf("%s: %+v (atteso 422 CORPO_NON_VALIDO sull'azione)", c.nome, e)
		}
	}
	// nodo_riusato nomina op e componente_id
	_, err := Riconcilia3(leggiProva3(t, base), leggiProva3(t, base), []Azione{azione3(t, AzNodoRiusato, "k-man",
		map[string]any{"op": "crea"}, map[string]any{"op": "riusa"})}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Codice != CodiceCorpoNonValido {
		t.Errorf("nodo_riusato senza componente_id: %+v", e)
	}
	// la chiave cambiata con nodo_modificato su un nodo appena aggiunto: rifiutata (EM §3.4: una chiave non cambia)
	nuovo := nodoManuale3(id, "k-u1", "ACM-2000")
	rinominato := copiaProfonda(t, nuovo)
	rinominato["chiave_bozza"], rinominato["tipo"] = "k-u2", "sciolto"
	d := copiaProfonda(t, base)
	d["nodi"] = append(d["nodi"].([]any), rinominato)
	_, err = Riconcilia3(leggiProva3(t, base), leggiProva3(t, d), []Azione{
		azione3(t, AzNodoAggiunto, "k-u1", nil, nuovo),
		azione3(t, AzNodoModificato, "k-u1", map[string]any{"tipo": "commerciale"}, map[string]any{"tipo": "sciolto", "chiave_bozza": "k-u2"}),
	}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Codice != CodiceCorpoNonValido || e.Campo != "azioni[1]" {
		t.Errorf("la chiave cambiata da nodo_modificato: %+v", e)
	}
	// il caso buono: l'evento scritto dal server, rifatto come azione, rifa' lo snapshot salvato (la proprieta' di T-10)
	buono := conTipo(map[string]any{"descrizione": "Vite"})
	az := []Azione{azione3(t, AzNodoModificato, "k-man", map[string]any{"tipo": "commerciale", "descrizione": nil},
		map[string]any{"tipo": "sciolto", "descrizione": "Vite"})}
	r, err := Riconcilia3(leggiProva3(t, base), leggiProva3(t, buono), az, Contesto3{N0: 4})
	if err != nil {
		t.Fatal(err)
	}
	ev := r.Eventi[0]
	if string(ev.Prima) != `{"descrizione":null,"tipo":"commerciale"}` || string(ev.Dopo) != `{"descrizione":"Vite","tipo":"sciolto"}` {
		t.Fatalf("evento: prima %s dopo %s", ev.Prima, ev.Dopo)
	}
	giornale := []Azione{{Azione: ev.Azione, ChiaveBozza: ev.ChiaveBozza, Prima: ev.Prima, Dopo: ev.Dopo, Origine: ev.Origine}}
	if _, err := Riconcilia3(leggiProva3(t, base), leggiProva3(t, buono), giornale, Contesto3{N0: 4}); err != nil {
		t.Fatalf("l'evento salvato non rifa' lo snapshot: %v", err)
	}
}

// I riferimenti che compaiono solo nelle azioni (EM §9, «ogni UUID del corpo, ovunque sia annidato»): un nodo del
// componente aggiunto e tolto nello stesso salvataggio lascia lo snapshot com'era, ma il suo componente e' fra i
// riferimenti che chi salva controlla.
func TestV1Riconcilia3IRiferimentiDelleAzioni(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	s := leggiProva3(t, base)
	estraneo := idDi("componente-di-un-altra-rfq")
	n := map[string]any{"chiave_bozza": "k-t1", "componente_id": estraneo.String(), "op": "riusa", "tipo": "sciolto", "origine": "registrato",
		"identita": map[string]any{"emittente": id.clienteE(), "emittente_presunto": true, "codice": "ACM-7001", "modo": "registrato"}}
	ni := map[string]any{"id": "ni-t1", "tipo": "non_includere",
		"casella":       map[string]any{"allegato_id": id.allegato.String(), "gruppo": gruppoProva3, "rif": id.rif("#12")},
		"motivo_codice": "altro", "motivo": "prova", "azione_id": azioneB, "importazione": strings.Repeat("e", 64)}
	r, err := Riconcilia3(s, s, []Azione{
		azione3(t, AzNodoAggiunto, "k-t1", nil, n),
		azione3(t, AzNodoTolto, "k-t1", n, nil),
		azione3(t, AzDecisioneAggiunta, "", nil, ni),
		azione3(t, AzDecisioneTolta, "", ni, nil),
	}, Contesto3{N0: 4})
	if err != nil {
		t.Fatal(err)
	}
	if r.Riferimenti.Componenti[estraneo] != "azioni[0].dopo.componente_id" {
		t.Errorf("componenti delle azioni: %v", r.Riferimenti.Componenti)
	}
	if _, ok := r.Riferimenti.Allegati[id.allegato]; !ok {
		t.Errorf("allegati delle azioni: %v", r.Riferimenti.Allegati)
	}
	if _, ok := r.Riferimenti.Registri[strings.Repeat("e", 64)]; !ok {
		t.Errorf("registri delle azioni: %v", r.Riferimenti.Registri)
	}
	if len(r.NodiAggiunti) != 1 || r.NodiAggiunti[0] != "k-t1" {
		t.Errorf("nodi aggiunti: %v", r.NodiAggiunti)
	}
}

// arco_spostato: l'arco spostato ha origine utente e niente step (EM §6.6).
func TestV1Riconcilia3LArcoSpostatoEDellIngegnere(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	vecchio := map[string]any{"padre": "k-root", "figlio": "k-man", "qta": 4, "origine": "utente", "posizione": "Pos." + eAccento}
	nuovo := map[string]any{"padre": "k-0007", "figlio": "k-man", "qta": 4, "origine": "utente", "posizione": "Pos." + eAccento}
	d := copiaProfonda(t, base)
	for _, a := range d["archi"].([]any) {
		if m := a.(map[string]any); m["figlio"] == "k-man" {
			m["padre"] = "k-0007"
		}
	}
	r, err := Riconcilia3(leggiProva3(t, base), leggiProva3(t, d), []Azione{azione3(t, AzArcoSpostato, "k-man", vecchio, nuovo)}, Contesto3{N0: 4})
	if err != nil {
		t.Fatal(err)
	}
	if r.Eventi[0].ChiaveBozza != "k-man" {
		t.Fatalf("evento %+v", r.Eventi[0])
	}
	step := map[string]any{"allegato_id": id.allegato.String(), "gruppo": gruppoProva3}
	conStep := conCampo(t, nuovo, "origine", "step")
	conStep["step"] = step
	_, err = Riconcilia3(leggiProva3(t, base), leggiProva3(t, d), []Azione{azione3(t, AzArcoSpostato, "k-man", vecchio, conStep)}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Codice != CodiceCorpoNonValido {
		t.Fatalf("arco spostato con lo step: %+v", e)
	}
}

// T-16, i due casi in piu' di PM23 oltre al file condiviso: lo svincolo con il motivo nodo_escluso entra nel vassoio
// (PE-22), e l'annulla di un evento di un gesto cumulativo con prima {azione_id} lo toglie (PE-05).
func TestV1T16NodoEsclusoEAnnullaConAzione(t *testing.T) {
	ev := func(n int, verbo, prima, dopo string) EventoGiornale {
		e := EventoGiornale{N: n, Azione: verbo, Dopo: json.RawMessage(dopo)}
		if prima != "" {
			e.Prima = json.RawMessage(prima)
		}
		return e
	}
	svincolo := ev(1, AzFileSvincolato, `{"proposta_id":"p-e","chiave_bozza":"k-r"}`, `{"proposta_id":"p-e","motivo":"nodo_escluso","azione_id":"`+azioneA+`"}`)
	got := VassoioDa([]EventoGiornale{svincolo})
	if len(got) != 1 || got[0].Motivo != SvincoloNodoEscluso || got[0].ChiaveBozza != "k-r" {
		t.Fatalf("nodo_escluso: %+v", got)
	}
	got = VassoioDa([]EventoGiornale{svincolo, ev(2, AzAnnulla, `{"azione_id":"`+azioneA+`"}`, `{"n":1}`)})
	if len(got) != 0 {
		t.Fatalf("annullato con l'azione_id: %+v", got)
	}
}
