package distintabozza

// L1 — le 16 decisioni del formato 3 (V1, passo 2; EM §3.6, §3.6.1, §3.6.2, §7.1, §7.2): la forma chiusa di ogni tipo,
// i campi ammessi e obbligatori, i valori, l'NFC solo sui motivi, assente = null = vuoto.

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const (
	uuidD1  = "00000000-0000-4000-8000-000000002200"
	uuidD2  = "00000000-0000-4000-8000-000000002300"
	uuidD3  = "00000000-0000-4000-8000-000000002400"
	azioneV = "00000000-0000-4000-8000-0000000e0101"
)

func impProva() string { return strings.Repeat("6", 64) }

// decisioniValide: un esempio valido per ognuno dei 16 tipi.
func decisioniValide() map[string]map[string]any {
	id := idSintetici3()
	return map[string]map[string]any{
		DecisioneD1AParticolare:      {"id": "d1", "tipo": DecisioneD1AParticolare, "chiave_bozza": "k-1", "motivo": "acquistato finito"},
		DecisioneD1RestaSottoassieme: {"id": "d2", "tipo": DecisioneD1RestaSottoassieme, "chiave_bozza": "k-1"},
		DecisioneD1ASottoassieme:     {"id": "d3", "tipo": DecisioneD1ASottoassieme, "chiave_bozza": "k-1"},
		DecisioneMonoPezzo:           {"id": "d4", "tipo": DecisioneMonoPezzo, "motivo": "pezzo unico dichiarato"},
		DecisioneTogliArco:           {"id": "d5", "tipo": DecisioneTogliArco, "padre_id": uuidD1, "figlio_id": uuidD2},
		DecisioneEscludi:             {"id": "d6", "tipo": DecisioneEscludi, "componente_id": uuidD1, "motivo": "non serve piu'"},
		DecisioneSenzaPadre:          {"id": "d7", "tipo": DecisioneSenzaPadre, "chiave_bozza": "k-1"},
		DecisioneRiusaEsistente:      {"id": "d8", "tipo": DecisioneRiusaEsistente, "chiave_bozza": "k-1", "componente_id": uuidD1},
		DecisioneRiusaPerCanonico:    {"id": "d9", "tipo": DecisioneRiusaPerCanonico, "chiave_bozza": "k-1", "componente_id": uuidD1},
		DecisioneRicodifica: {"id": "d10", "tipo": DecisioneRicodifica, "componente_id": uuidD1, "codice_prima": "ACM-1110",
			"codice_dopo": "ACM-1100", "motivo": "il codice del disegno", "emittente_dopo": "partner:beta", "namespace_dopo": "beta"},
		DecisioneArcoCondiviso: {"id": "d11", "tipo": DecisioneArcoCondiviso, "padre_id": uuidD1, "figlio_id": uuidD2, "qta": 3,
			"prodotti": []any{uuidD3}},
		DecisioneDaValutare: {"id": "d12", "tipo": DecisioneDaValutare, "chiave_bozza": "k-1"},
		DecisioneCambiaTipo: {"id": "d13", "tipo": DecisioneCambiaTipo, "componente_id": uuidD1, "tipo_nuovo": "commerciale",
			"firma_effetto": "0123456789abcdef01234567"},
		DecisioneIdentita: {"id": "d14", "tipo": DecisioneIdentita, "chiave_bozza": "k-1", "modo": ModoAlternativa, "importazione": impProva(),
			"chiave_lettura": id.allegato.String() + "/" + idLetturaBeta, "lettura": letturaCompattaBeta(),
			"identita": map[string]any{"emittente": "partner:beta", "namespace": "beta", "codice": "400123"}, "motivo": "letta sul disegno"},
		DecisioneNonIncludere: {"id": "d15", "tipo": DecisioneNonIncludere,
			"casella":       map[string]any{"allegato_id": id.allegato.String(), "gruppo": "bom", "rif": id.rif("#31")},
			"motivo_codice": "elemento_cad", "motivo": "cordone", "azione_id": azioneV, "importazione": impProva(),
			"segnale_visto": map[string]any{"codice": "ancoraggio.possibile_elemento_cad", "regola": "elemento_cad_nome_foglia_v1", "versione_servizio": "ancoraggio-3"}},
		DecisioneScollegaDocumento: {"id": "d16", "tipo": DecisioneScollegaDocumento, "allegato_id": uuidD2, "componente_id": uuidD1},
		// U2 §6.4.2: il diciassettesimo tipo
		DecisioneConfermaCategoria: {"id": "d17", "tipo": DecisioneConfermaCategoria, "soggetto": map[string]any{"chiave_bozza": "k-1"},
			"categoria": "minuteria", "esito": "conferma", "origine_categoria": "classe_grammatica",
			"classe_proposta": map[string]any{"categoria": "minuteria", "fonte": "acme-norma", "valore": "2.1051"},
			"importazione":    impProva(), "ingresso": "editor", "motivo": "vite di serie"},
	}
}

func leggiDecisioneProva(t *testing.T, d map[string]any) (Decisione3, *Errore) {
	t.Helper()
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return LeggiDecisione3("decisioni[0]", b)
}

func TestV1LeDiciassetteDecisioni(t *testing.T) {
	valide := decisioniValide()
	if len(valide) != 17 || len(formeDecisione) != 17 {
		t.Fatalf("tipi: %d esempi, %d forme (attesi 17: i 16 di EM §3.6 e conferma_categoria, U2 §6.4.2)", len(valide), len(formeDecisione))
	}
	for tipo, d := range valide {
		t.Run(tipo, func(t *testing.T) {
			got, e := leggiDecisioneProva(t, d)
			if e != nil {
				t.Fatalf("decisione valida rifiutata: %v", e)
			}
			can, err := got.Canonica()
			if err != nil {
				t.Fatal(err)
			}
			// la rilettura della forma canonica da' gli stessi byte
			got2, e := LeggiDecisione3("x", can)
			if e != nil {
				t.Fatalf("rilettura: %v", e)
			}
			can2, _ := got2.Canonica()
			if !bytes.Equal(can, can2) {
				t.Fatalf("la rilettura cambia i byte:\n%s\n%s", can, can2)
			}
			// la forma canonica porta solo id, tipo e i campi del tipo
			var campi map[string]json.RawMessage
			if err := json.Unmarshal(can, &campi); err != nil {
				t.Fatal(err)
			}
			f := formeDecisione[tipo]
			ammessi := map[string]bool{"id": true, "tipo": true}
			for _, k := range append(append([]string{}, f.obbligatori...), f.facoltativi...) {
				ammessi[k] = true
			}
			for k := range campi {
				if !ammessi[k] {
					t.Errorf("la forma canonica porta %s", k)
				}
			}
			for _, k := range f.obbligatori {
				if _, ok := campi[k]; !ok {
					t.Errorf("la forma canonica non porta %s", k)
				}
			}
		})
	}
}

func TestV1LaFormaChiusaDelleDecisioni(t *testing.T) {
	cambia := func(tipo string, f func(d map[string]any)) map[string]any {
		d := decisioniValide()[tipo]
		var c map[string]any
		b, _ := json.Marshal(d)
		_ = json.Unmarshal(b, &c)
		f(c)
		return c
	}
	casi := []struct {
		nome, campo string
		d           map[string]any
	}{
		{"tipo sconosciuto", "decisioni[0].tipo", cambia(DecisioneMonoPezzo, func(d map[string]any) { d["tipo"] = "unisci" })},
		{"id non valido", "decisioni[0].id", cambia(DecisioneMonoPezzo, func(d map[string]any) { d["id"] = "d 1" })},
		{"campo di un altro tipo", "decisioni[0].chiave_bozza", cambia(DecisioneScollegaDocumento, func(d map[string]any) { d["chiave_bozza"] = "k-1" })},
		{"campo di un altro tipo in mono_pezzo", "decisioni[0].componente_id", cambia(DecisioneMonoPezzo, func(d map[string]any) { d["componente_id"] = uuidD1 })},
		{"campo sconosciuto", "decisioni[0].effetto", cambia(DecisioneDaValutare, func(d map[string]any) { d["effetto"] = "nessuno" })},
		{"escludi senza motivo", "decisioni[0].motivo", cambia(DecisioneEscludi, func(d map[string]any) { delete(d, "motivo") })},
		{"escludi con motivo vuoto", "decisioni[0].motivo", cambia(DecisioneEscludi, func(d map[string]any) { d["motivo"] = "" })},
		{"escludi con motivo null", "decisioni[0].motivo", cambia(DecisioneEscludi, func(d map[string]any) { d["motivo"] = nil })},
		{"mono_pezzo con motivo di soli spazi", "decisioni[0].motivo", cambia(DecisioneMonoPezzo, func(d map[string]any) { d["motivo"] = "   " })},
		{"motivo oltre 1000", "decisioni[0].motivo", cambia(DecisioneMonoPezzo, func(d map[string]any) { d["motivo"] = strings.Repeat("m", 1001) })},
		{"ricodifica senza emittente", "decisioni[0].emittente_dopo", cambia(DecisioneRicodifica, func(d map[string]any) { delete(d, "emittente_dopo") })},
		{"ricodifica con motivo oltre 500", "decisioni[0].motivo", cambia(DecisioneRicodifica, func(d map[string]any) { d["motivo"] = strings.Repeat("m", 501) })},
		{"ricodifica con un codice nuovo con spazio", "decisioni[0].codice_dopo", cambia(DecisioneRicodifica, func(d map[string]any) { d["codice_dopo"] = "ACM 1" })},
		{"ricodifica verso un emittente maiuscolo", "decisioni[0].emittente_dopo", cambia(DecisioneRicodifica, func(d map[string]any) { d["emittente_dopo"] = "partner:Beta" })},
		{"togli_arco su se stesso", "decisioni[0]", cambia(DecisioneTogliArco, func(d map[string]any) { d["figlio_id"] = uuidD1 })},
		{"arco_condiviso con qta zero", "decisioni[0].qta", cambia(DecisioneArcoCondiviso, func(d map[string]any) { d["qta"] = 0 })},
		{"arco_condiviso senza prodotti", "decisioni[0].prodotti", cambia(DecisioneArcoCondiviso, func(d map[string]any) { d["prodotti"] = []any{} })},
		{"arco_condiviso con un prodotto ripetuto", "decisioni[0].prodotti[1]", cambia(DecisioneArcoCondiviso, func(d map[string]any) {
			d["prodotti"] = []any{uuidD3, uuidD3}
		})},
		{"cambia_tipo con un tipo sconosciuto", "decisioni[0].tipo_nuovo", cambia(DecisioneCambiaTipo, func(d map[string]any) { d["tipo_nuovo"] = "pezzo" })},
		{"cambia_tipo con la firma in maiuscolo", "decisioni[0].firma_effetto", cambia(DecisioneCambiaTipo, func(d map[string]any) {
			d["firma_effetto"] = "0123456789ABCDEF01234567"
		})},
		{"identita con un modo sconosciuto", "decisioni[0].modo", cambia(DecisioneIdentita, func(d map[string]any) { d["modo"] = "scelta" })},
		{"alternativa senza registro", "decisioni[0].importazione", cambia(DecisioneIdentita, func(d map[string]any) { delete(d, "importazione") })},
		{"alternativa con lettura e riferimento", "decisioni[0]", cambia(DecisioneIdentita, func(d map[string]any) { d["riferimento"] = "manuale_legacy" })},
		{"alternativa senza la copia della lettura", "decisioni[0].lettura", cambia(DecisioneIdentita, func(d map[string]any) { delete(d, "lettura") })},
		{"alternativa con un emittente diverso dalla lettura", "decisioni[0].identita", cambia(DecisioneIdentita, func(d map[string]any) {
			d["identita"].(map[string]any)["emittente"] = EmittenteCliente(idDi("cliente"))
		})},
		{"copia della lettura con un campo in piu'", "decisioni[0]", cambia(DecisioneIdentita, func(d map[string]any) {
			d["lettura"].(map[string]any)["testo_unita"] = "400123"
		})},
		{"chiave di lettura senza allegato", "decisioni[0].chiave_lettura", cambia(DecisioneIdentita, func(d map[string]any) { d["chiave_lettura"] = "l:u:step:#9" })},
		{"riferimento sconosciuto", "decisioni[0].riferimento", cambia(DecisioneIdentita, func(d map[string]any) {
			delete(d, "chiave_lettura")
			delete(d, "lettura")
			d["riferimento"] = "altro"
		})},
		{"codice manuale di un partner", "decisioni[0].identita.emittente", cambia(DecisioneIdentita, func(d map[string]any) {
			delete(d, "chiave_lettura")
			delete(d, "lettura")
			d["riferimento"] = "manuale_legacy"
		})},
		{"codice manuale senza motivo", "decisioni[0].motivo", cambia(DecisioneIdentita, func(d map[string]any) {
			delete(d, "chiave_lettura")
			delete(d, "lettura")
			delete(d, "motivo")
			d["riferimento"] = "manuale_legacy"
			d["identita"] = map[string]any{"emittente": EmittenteCliente(idDi("cliente")), "codice": "ACM-1900"}
		})},
		{"correzione con una lettura", "decisioni[0]", cambia(DecisioneIdentita, func(d map[string]any) { d["modo"] = ModoCorrezione })},
		{"correzione senza motivo", "decisioni[0].motivo", cambia(DecisioneIdentita, func(d map[string]any) {
			d["modo"] = ModoCorrezione
			delete(d, "chiave_lettura")
			delete(d, "lettura")
			delete(d, "motivo")
		})},
		{"correzione verso un partner senza registro", "decisioni[0].importazione", cambia(DecisioneIdentita, func(d map[string]any) {
			d["modo"] = ModoCorrezione
			delete(d, "chiave_lettura")
			delete(d, "lettura")
			delete(d, "importazione")
		})},
		{"identita senza codice", "decisioni[0].identita.codice", cambia(DecisioneIdentita, func(d map[string]any) {
			delete(d["identita"].(map[string]any), "codice")
		})},
		// (le minuscole della rev dipendono dal nodo: le guarda lo snapshot, TestV1LaRevDaDoveViene; qui la sola forma)
		{"identita con la rev con uno spazio", "decisioni[0].identita.rev", cambia(DecisioneIdentita, func(d map[string]any) {
			d["identita"].(map[string]any)["rev"] = "B 1"
		})},
		{"identita con la rev oltre 10 byte", "decisioni[0].identita.rev", cambia(DecisioneIdentita, func(d map[string]any) {
			d["identita"].(map[string]any)["rev"] = strings.Repeat("B", 11)
		})},
		{"non_includere con un motivo_codice sconosciuto", "decisioni[0].motivo_codice", cambia(DecisioneNonIncludere, func(d map[string]any) { d["motivo_codice"] = "rumore" })},
		{"non_includere con un'azione non v4", "decisioni[0].azione_id", cambia(DecisioneNonIncludere, func(d map[string]any) {
			d["azione_id"] = "00000000-0000-1000-8000-0000000e0101"
		})},
		{"segnale visto senza registro", "decisioni[0].importazione", cambia(DecisioneNonIncludere, func(d map[string]any) { delete(d, "importazione") })},
		{"segnale visto senza codice", "decisioni[0].segnale_visto.codice", cambia(DecisioneNonIncludere, func(d map[string]any) {
			d["segnale_visto"].(map[string]any)["codice"] = ""
		})},
		{"segnale visto con un campo in piu'", "decisioni[0]", cambia(DecisioneNonIncludere, func(d map[string]any) {
			d["segnale_visto"].(map[string]any)["spiegazione"] = "x"
		})},
		{"casella con un rif malformato", "decisioni[0].casella.rif", cambia(DecisioneNonIncludere, func(d map[string]any) {
			d["casella"].(map[string]any)["rif"] = "#31"
		})},
		{"non_includere senza casella", "decisioni[0].casella", cambia(DecisioneNonIncludere, func(d map[string]any) { delete(d, "casella") })},
		{"qta con esponente", "decisioni[0]", cambia(DecisioneArcoCondiviso, func(d map[string]any) { d["qta"] = jsonNumero("3e0") })},
		{"nome con la maiuscola", "decisioni[0]", cambia(DecisioneMonoPezzo, func(d map[string]any) { d["Motivo"] = d["motivo"]; delete(d, "motivo") })},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			_, e := leggiDecisioneProva(t, c.d)
			if e == nil {
				t.Fatal("decisione non valida accettata")
			}
			if e.Codice != CodiceCorpoNonValido || !strings.HasPrefix(e.Campo, c.campo) {
				t.Fatalf("campo %q (%s), atteso %q: %v", e.Campo, e.Codice, c.campo, e)
			}
		})
	}
	// il «tipo» di cambia_tipo e' la chiave del tipo della decisione: due «tipo» nello stesso oggetto non si leggono
	doppio := []byte(`{"id":"d13","tipo":"cambia_tipo","componente_id":"` + uuidD1 + `","tipo":"commerciale","firma_effetto":"0123456789abcdef01234567"}`)
	if _, e := LeggiDecisione3("decisioni[0]", doppio); e == nil {
		t.Fatal("cambia_tipo con due «tipo» si legge")
	}
}

func TestV1NullVuotoEAssenteNelleDecisioni(t *testing.T) {
	base := decisioniValide()[DecisioneD1RestaSottoassieme]
	d0, e := leggiDecisioneProva(t, base)
	if e != nil {
		t.Fatal(e)
	}
	c0, _ := d0.Canonica()
	for _, motivo := range []any{nil, ""} {
		conMotivo := map[string]any{}
		for k, v := range base {
			conMotivo[k] = v
		}
		conMotivo["motivo"] = motivo
		conMotivo["componente_id"] = nil // un campo di un altro tipo, null: vale come assente
		d, e := leggiDecisioneProva(t, conMotivo)
		if e != nil {
			t.Fatalf("motivo %v: %v", motivo, e)
		}
		if c, _ := d.Canonica(); !bytes.Equal(c, c0) {
			t.Fatalf("motivo %v: forma canonica diversa:\n%s\n%s", motivo, c, c0)
		}
	}
	// NFC solo sul motivo: il motivo scomposto e quello composto sono la stessa decisione
	mono := decisioniValide()[DecisioneMonoPezzo]
	mono["motivo"] = "pezzo unico, verificato" + eAccento
	d1, _ := leggiDecisioneProva(t, mono)
	mono["motivo"] = "pezzo unico, verificato" + "é"
	d2, _ := leggiDecisioneProva(t, mono)
	c1, _ := d1.Canonica()
	c2, _ := d2.Canonica()
	if !bytes.Equal(c1, c2) {
		t.Fatal("il motivo non passa in NFC")
	}
	// il motivo di una ricodifica si conta dopo l'NFC: 300 «e» + accento combinante sono 300 caratteri
	ric := decisioniValide()[DecisioneRicodifica]
	ric["motivo"] = strings.Repeat(eAccento, 300)
	if _, e := leggiDecisioneProva(t, ric); e != nil {
		t.Fatalf("motivo di 300 caratteri dopo l'NFC: %v", e)
	}
	// l'identita' e il letterale non passano in NFC
	ide := decisioniValide()[DecisioneIdentita]
	ide["identita"].(map[string]any)["codice"] = "40012" + eAccento
	d3, e := leggiDecisioneProva(t, ide)
	if e != nil {
		t.Fatal(e)
	}
	if d3.Identita.Codice != "40012"+eAccento || d3.Lettura.Letterale != letteraleBeta {
		t.Fatalf("identita' o letterale normalizzati: %q %q", d3.Identita.Codice, d3.Lettura.Letterale)
	}
	var e2 *Errore
	if _, err := LeggiSnapshot3([]byte(`{"formato":3}`)); !errors.As(err, &e2) {
		t.Fatalf("lo snapshot senza prodotto: %v", err)
	}
}
