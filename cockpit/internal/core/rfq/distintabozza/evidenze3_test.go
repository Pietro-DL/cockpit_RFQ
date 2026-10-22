package distintabozza

// L1 — ondata C di V1, corsia persistenza: le evidenze documentali (U2 §7.7), conferma_categoria (U2 §6.4.2),
// tipo_confermato, arco_modificato, una sola cambia_tipo per componente (DEC-B PE-B06, prova OB-14). Senza database:
// la forma dello snapshot, la riconciliazione delle azioni e l'inverso di arco_modificato.

import (
	"strings"
	"testing"
)

const shaEvidenza = "abababababababababababababababababababababababababababababababab"

func evidenzaProva(id idProva3, pagine ...int) map[string]any {
	e := map[string]any{"allegato_id": id.allegato.String(), "sha256": shaEvidenza}
	if len(pagine) > 0 {
		pp := make([]any, len(pagine))
		for i, p := range pagine {
			pp[i] = p
		}
		e["pagine"] = pp
	}
	return e
}

func rifiutoDiForma(t *testing.T, s map[string]any) *Errore {
	t.Helper()
	_, err := LeggiSnapshot3(snapshotJSON(t, s))
	return errore3(t, err)
}

// EV-02 (parte L1): le evidenze dell'identita' stanno nei nodi manuali, quelle dell'arco negli archi utente; pagine in
// ordine e senza doppioni; la nota in NFC, mai normalizzata dal server; al piu' 20; nessuna ripetuta.
func TestV1EvidenzeDocumentaliForma(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)

	ok := copiaProfonda(t, base)
	nodoIn3(ok, "k-man")["evidenze_identita"] = []any{evidenzaProva(id, 1, 3), evidenzaProva(id)}
	for _, a := range ok["archi"].([]any) {
		if m := a.(map[string]any); m["figlio"] == "k-man" {
			m["evidenze_arco"] = []any{conCampo(t, evidenzaProva(id, 2), "riga", "pos. 4 dell'elenco particolari")}
		}
	}
	s := leggiProva3(t, ok)
	for _, n := range s.Nodi {
		if n.ChiaveBozza == "k-man" && (len(n.EvidenzeIdentita) != 2 || n.EvidenzeIdentita[0].Pagine != nil) {
			t.Fatalf("evidenze dell'identita' (in ordine canonico, prima quella senza pagine): %+v", n.EvidenzeIdentita)
		}
	}
	// la rilettura dei byte canonici da' gli stessi byte
	if b, err := s.Canonico(); err != nil {
		t.Fatal(err)
	} else if s2, err := LeggiSnapshot3(b); err != nil {
		t.Fatal(err)
	} else if b2, _ := s2.Canonico(); string(b) != string(b2) {
		t.Fatal("la rilettura cambia i byte")
	}

	casi := []struct {
		nome, campo string
		muta        func(m map[string]any)
	}{
		{"evidenze su un nodo proposta", "nodi[1].evidenze_identita", func(m map[string]any) {
			nodoIn3(m, "k-0007")["evidenze_identita"] = []any{evidenzaProva(id)}
		}},
		{"pagine fuori ordine", "nodi[4].evidenze_identita[0].pagine", func(m map[string]any) {
			nodoIn3(m, "k-man")["evidenze_identita"] = []any{evidenzaProva(id, 3, 1)}
		}},
		{"pagina ripetuta", "nodi[4].evidenze_identita[0].pagine", func(m map[string]any) {
			nodoIn3(m, "k-man")["evidenze_identita"] = []any{evidenzaProva(id, 2, 2)}
		}},
		{"nota non NFC", "nodi[4].evidenze_identita[0].nota", func(m map[string]any) {
			nodoIn3(m, "k-man")["evidenze_identita"] = []any{conCampo(t, evidenzaProva(id), "nota", "é la prova")}
		}},
		{"sha non valido", "nodi[4].evidenze_identita[0].sha256", func(m map[string]any) {
			nodoIn3(m, "k-man")["evidenze_identita"] = []any{conCampo(t, evidenzaProva(id), "sha256", "ABC")}
		}},
		{"evidenza ripetuta", "nodi[4].evidenze_identita[1]", func(m map[string]any) {
			nodoIn3(m, "k-man")["evidenze_identita"] = []any{evidenzaProva(id), evidenzaProva(id)}
		}},
		{"oltre 20", "nodi[4].evidenze_identita", func(m map[string]any) {
			var ee []any
			for i := 1; i <= 21; i++ {
				ee = append(ee, evidenzaProva(id, i))
			}
			nodoIn3(m, "k-man")["evidenze_identita"] = ee
		}},
		{"evidenze su un arco step", "archi[0].evidenze_arco", func(m map[string]any) {
			m["archi"].([]any)[0].(map[string]any)["evidenze_arco"] = []any{evidenzaProva(id)}
		}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			m := copiaProfonda(t, base)
			c.muta(m)
			e := rifiutoDiForma(t, m)
			if e.Codice != CodiceCorpoNonValido || e.Campo != c.campo {
				t.Fatalf("atteso CORPO_NON_VALIDO su %s, avuto %+v", c.campo, e)
			}
		})
	}
}

// L'evidenza documentale sta solo nella correzione (U2 §7.7, U2-P20).
func TestV1EvidenzaDocumentaleSoloNellaCorrezione(t *testing.T) {
	id := idSintetici3()
	corr := map[string]any{"id": "dc-1", "tipo": DecisioneIdentita, "chiave_bozza": "k-1", "modo": ModoCorrezione,
		"identita": map[string]any{"emittente": id.clienteE(), "codice": "ACM-1300"}, "motivo": "il portatag dice ACM-1300",
		"evidenza_documentale": evidenzaProva(id, 1)}
	if _, e := leggiDecisioneProva(t, corr); e != nil {
		t.Fatalf("correzione con evidenza rifiutata: %v", e)
	}
	alt := decisioniValide()[DecisioneIdentita]
	alt["evidenza_documentale"] = evidenzaProva(id)
	if _, e := leggiDecisioneProva(t, alt); e == nil || e.Campo != "decisioni[0].evidenza_documentale" {
		t.Fatalf("alternativa con evidenza: %+v", e)
	}
	corr["evidenza_documentale"] = evidenzaProva(id, 5, 4)
	if _, e := leggiDecisioneProva(t, corr); e == nil || e.Campo != "decisioni[0].evidenza_documentale.pagine" {
		t.Fatalf("pagine fuori ordine nella correzione: %+v", e)
	}
}

// CAT (parte L1): la forma chiusa di conferma_categoria (U2 §6.4.2).
func TestV1ConfermaCategoriaForma(t *testing.T) {
	cambia := func(f func(d map[string]any)) map[string]any {
		d := decisioniValide()[DecisioneConfermaCategoria]
		f(d)
		return d
	}
	casi := []struct {
		nome, campo, motivo string
		d                   map[string]any
	}{
		{"categoria fuori dal vocabolario", "decisioni[0].categoria", MotivoCategoriaNonAmmessa, cambia(func(d map[string]any) { d["categoria"] = "normalizzato" })},
		{"esito sconosciuto", "decisioni[0].esito", "", cambia(func(d map[string]any) { d["esito"] = "forse" })},
		{"ingresso mancante", "decisioni[0].ingresso", "", cambia(func(d map[string]any) { delete(d, "ingresso") })},
		{"classe senza registro", "decisioni[0].importazione", "", cambia(func(d map[string]any) { delete(d, "importazione") })},
		{"classe proposta con origine manuale", "decisioni[0].classe_proposta", "", cambia(func(d map[string]any) { d["origine_categoria"] = "manuale" })},
		{"da_conferma incompleto", "decisioni[0].da_conferma", "", cambia(func(d map[string]any) {
			d["da_conferma"] = map[string]any{"conferma_id": uuidD1, "numero": 0, "decisione_id": "cc-0"}
		})},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			_, e := leggiDecisioneProva(t, c.d)
			if e == nil || e.Campo != c.campo || (c.motivo != "" && e.Motivo != c.motivo) {
				t.Fatalf("atteso un rifiuto su %s (%s), avuto %+v", c.campo, c.motivo, e)
			}
		})
	}
	// con la classe proposta e l'esito conferma il motivo e' facoltativo
	senza := cambia(func(d map[string]any) { delete(d, "motivo") })
	if _, e := leggiDecisioneProva(t, senza); e != nil {
		t.Fatalf("classe accettata senza motivo: %v", e)
	}
	// messaggio 46 [U]: anche con l'origine manuale (o normativa) e con il rifiuto il motivo non si chiede piu'
	manuale := cambia(func(d map[string]any) {
		d["origine_categoria"] = "manuale"
		delete(d, "classe_proposta")
		delete(d, "importazione")
		delete(d, "motivo")
	})
	if _, e := leggiDecisioneProva(t, manuale); e != nil {
		t.Fatalf("categoria manuale senza motivo: %v", e)
	}
	rifiuto := cambia(func(d map[string]any) { d["esito"] = "rifiuto"; delete(d, "motivo") })
	if _, e := leggiDecisioneProva(t, rifiuto); e != nil {
		t.Fatalf("«non è minuteria» senza motivo: %v", e)
	}
}

// Nello snapshot: mai sul finito (categoria_sul_finito), una decisione viva per nodo e categoria, il soggetto e' un nodo.
func TestV1ConfermaCategoriaNelloSnapshot(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	cat := func(idd, chiave, categoria string) map[string]any {
		return map[string]any{"id": idd, "tipo": DecisioneConfermaCategoria, "soggetto": map[string]any{"chiave_bozza": chiave},
			"categoria": categoria, "esito": "conferma", "origine_categoria": "normativa", "ingresso": "wizard", "motivo": "norma di serie"}
	}
	con := func(dd ...map[string]any) map[string]any {
		s := snapshotProva3(id, imp)
		for _, d := range dd {
			s["decisioni"] = append(s["decisioni"].([]any), d)
		}
		return s
	}
	leggiProva3(t, con(cat("cc-1", "k-0009", "minuteria"), cat("cc-2", "k-0009", "commerciale")))
	if e := rifiutoDiForma(t, con(cat("cc-1", "k-root", "minuteria"))); e.Motivo != MotivoCategoriaSulFinito {
		t.Fatalf("sul finito: %+v", e)
	}
	if e := rifiutoDiForma(t, con(cat("cc-1", "k-0009", "minuteria"), cat("cc-2", "k-0009", "minuteria"))); !strings.HasSuffix(e.Campo, ".categoria") {
		t.Fatalf("due decisioni sulla stessa categoria: %+v", e)
	}
	if e := rifiutoDiForma(t, con(cat("cc-1", "k-nessuno", "minuteria"))); !strings.HasSuffix(e.Campo, ".soggetto.chiave_bozza") {
		t.Fatalf("soggetto che non c'e': %+v", e)
	}
}

// OB-14: due cambia_tipo per lo stesso componente nello snapshot → 422 sul campo della seconda.
func TestV1UnSoloCambiaTipoPerComponente(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	s := snapshotProva3(id, imp)
	ct := func(idd, tipo string) map[string]any {
		return map[string]any{"id": idd, "tipo": DecisioneCambiaTipo, "componente_id": uuidD1, "tipo_nuovo": tipo, "firma_effetto": "0123456789abcdef01234567"}
	}
	s["decisioni"] = append(s["decisioni"].([]any), ct("ct-1", "commerciale"), ct("ct-2", "sciolto"))
	e := rifiutoDiForma(t, s)
	if e.Codice != CodiceCorpoNonValido || !strings.HasSuffix(e.Campo, ".componente_id") {
		t.Fatalf("due cambia_tipo: %+v", e)
	}
	// la seconda nell'ordine dello snapshot e' ct-2 (le decisioni sono lette in ordine)
	if !strings.Contains(e.Messaggio, "una sola per componente") {
		t.Fatalf("messaggio: %s", e.Messaggio)
	}
}

// «Conferma il tipo proposto»: nodo_modificato con il solo tipo_confermato (U2 §6.4.2, [D] verificato qui: la
// riconciliazione accetta un nodo_modificato senza cambio del tipo, purche' tipo_confermato cambi); le evidenze
// dell'identita' con nodo_modificato (U2 §7.7).
func TestV1NodoModificatoTipoConfermatoEdEvidenze(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	d := copiaProfonda(t, base)
	nodoIn3(d, "k-0007")["tipo_confermato"] = true
	r, err := Riconcilia3(leggiProva3(t, base), leggiProva3(t, d),
		[]Azione{azione3(t, AzNodoModificato, "k-0007", map[string]any{"tipo_confermato": nil}, map[string]any{"tipo_confermato": true})}, Contesto3{N0: 4})
	if err != nil {
		t.Fatal(err)
	}
	if string(r.Eventi[0].Dopo) != `{"tipo_confermato":true}` {
		t.Fatalf("evento: %s", r.Eventi[0].Dopo)
	}
	// le evidenze del nodo manuale
	d2 := copiaProfonda(t, base)
	ev := []any{evidenzaProva(id, 1)}
	nodoIn3(d2, "k-man")["evidenze_identita"] = ev
	if _, err := Riconcilia3(leggiProva3(t, base), leggiProva3(t, d2),
		[]Azione{azione3(t, AzNodoModificato, "k-man", map[string]any{"evidenze_identita": nil}, map[string]any{"evidenze_identita": ev})}, Contesto3{N0: 4}); err != nil {
		t.Fatal(err)
	}
	// sul nodo proposta no: le evidenze stanno nei nodi manuali
	d3 := copiaProfonda(t, base)
	_, err = Riconcilia3(leggiProva3(t, base), leggiProva3(t, d3),
		[]Azione{azione3(t, AzNodoModificato, "k-0007", map[string]any{"evidenze_identita": nil}, map[string]any{"evidenze_identita": ev})}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Codice != CodiceCorpoNonValido {
		t.Fatalf("evidenze su un nodo proposta: %+v", e)
	}
}

// arco_modificato (U2 §7.7): le sole evidenze_arco di un arco utente; l'annulla lo disfa; su un arco step no.
func TestV1ArcoModificatoEvidenzeDellArco(t *testing.T) {
	id := idSintetici3()
	imp := componiProva3(t, id).Impronta
	base := snapshotProva3(id, imp)
	ev := []any{evidenzaProva(id, 2, 7)}
	d := copiaProfonda(t, base)
	for _, a := range d["archi"].([]any) {
		if m := a.(map[string]any); m["figlio"] == "k-man" {
			m["evidenze_arco"] = ev
		}
	}
	prima := map[string]any{"padre": "k-root", "figlio": "k-man", "evidenze_arco": nil}
	dopo := map[string]any{"padre": "k-root", "figlio": "k-man", "evidenze_arco": ev}
	sBase, sD := leggiProva3(t, base), leggiProva3(t, d)
	r, err := Riconcilia3(sBase, sD, []Azione{azione3(t, AzArcoModificato, "k-man", prima, dopo)}, Contesto3{N0: 4})
	if err != nil {
		t.Fatal(err)
	}
	if r.Eventi[0].ChiaveBozza != "k-man" || r.Strutturali != 1 {
		t.Fatalf("evento %+v, strutturali %d", r.Eventi[0], r.Strutturali)
	}
	// l'annulla dell'evento 5 riporta l'arco senza evidenze
	precedenti := map[int]EventoLetto{5: {N: 5, Azione: AzArcoModificato, ChiaveBozza: "k-man", Origine: OrigineUtente, Prima: r.Eventi[0].Prima, Dopo: r.Eventi[0].Dopo}}
	cx := Contesto3{N0: 5, Precedente: func(n int) (EventoLetto, bool, error) { e, ok := precedenti[n]; return e, ok, nil }}
	if _, err := Riconcilia3(sD, sBase, []Azione{azione3(t, AzAnnulla, "", nil, map[string]any{"n": 5})}, cx); err != nil {
		t.Fatalf("annulla di arco_modificato: %v", err)
	}
	// prima che non e' quello dell'arco: azioni_non_riconciliate
	_, err = Riconcilia3(sBase, sD, []Azione{azione3(t, AzArcoModificato, "k-man", dopo, dopo)}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Motivo != MotivoAzioniNonRiconciliate {
		t.Fatalf("prima sbagliato: %+v", e)
	}
	// un altro campo: forma
	_, err = Riconcilia3(sBase, sD, []Azione{azione3(t, AzArcoModificato, "k-man", conCampo(t, prima, "qta", 4), conCampo(t, dopo, "qta", 5))}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Codice != CodiceCorpoNonValido {
		t.Fatalf("arco_modificato con la qta: %+v", e)
	}
	// su un arco step: forma
	pStep := map[string]any{"padre": "k-root", "figlio": "k-0007", "evidenze_arco": nil}
	dStep := map[string]any{"padre": "k-root", "figlio": "k-0007", "evidenze_arco": ev}
	_, err = Riconcilia3(sBase, sBase, []Azione{azione3(t, AzArcoModificato, "k-0007", pStep, dStep)}, Contesto3{N0: 4})
	if e := errore3(t, err); e.Codice != CodiceCorpoNonValido {
		t.Fatalf("arco_modificato su un arco step: %+v", e)
	}
}
