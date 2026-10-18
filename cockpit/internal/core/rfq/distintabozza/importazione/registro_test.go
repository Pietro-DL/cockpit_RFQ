package importazione

// L1 — il registro d'importazione (V1, passo 2 di PM23 §14; EM §3.8, §3.8.1, §3.8.2, §3.10): la lettura stretta,
// l'elenco chiuso delle impronte, il limite di peso, la completezza del contesto, la copia compatta di una lettura byte
// per byte (N-01). Dati sintetici: ACME e BETA sono inventati, gli UUID sono di prova.

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/jsoncanonico"
)

const (
	tProva   = "00000000-0000-4000-8000-0000000000a1"
	pProva   = "00000000-0000-4000-8000-000000001000"
	cProva   = "00000000-0000-4000-8000-0000000c1001"
	aProva   = "00000000-0000-4000-8000-0000000a0001"
	fProva   = "00000000-0000-4000-8000-0000000a0002"
	uProva   = "00000000-0000-4000-8000-0000000000e1"
	letteraE = "é" // «e» + accento combinante: N-01
)

var shaA = strings.Repeat("a", 64)

func rifA(chiave string) string  { return "nodo:" + shaA + ":" + chiave }
func voceA(chiave string) string { return "n:" + aProva + ":" + rifA(chiave) }
func raw(s string) json.RawMessage {
	return json.RawMessage(s)
}

// lettura: una lettura sintetica del registro, con il letterale e il testo dell'unita' byte per byte.
func lettura(allegato, id, emittente, namespace, letterale string) string {
	m := map[string]any{
		"chiave_lettura": allegato + "/" + id, "unita_id": "u:" + id, "testo_unita": letterale,
		"intervallo": map[string]any{"inizio": 0, "fine": len(letterale)}, "letterale": letterale, "emittente": emittente,
		"hash_grammatica": strings.Repeat("7", 64), "namespace": namespace, "famiglia": "num", "forma": "sei",
		"base":          map[string]any{"originale": letterale, "normalizzata": letterale, "segmenti": []any{}, "mancanti": []any{}, "completa": true},
		"qualificatori": map[string]any{}, "revisione": nil, "stato_forma": "letta", "ruolo": "",
	}
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// contenutoDiProva: un registro del prodotto P sul gruppo bom dello STEP A: la radice #1, il telaio #7 con l'identita'
// proposta del cliente, il pezzo #9 con un letterale con l'accento combinante e un'alternativa di BETA, il cordone #31
// con il segnale LS-02, un PDF con una lettura, due relazioni (una dal nodo, una dal file), un nodo tolto.
func contenutoDiProva() Contenuto {
	cliente := "cliente:" + cProva
	partner := []PartnerAttivo{{Emittente: "partner:beta", Hash: strings.Repeat("8", 64)}}
	_ = partner
	voce := func(chiave string) Voce {
		vero, falso := true, false
		return Voce{Voce: voceA(chiave), Origine: OrigineVoce{AllegatoID: aProva, Sha256: shaA, Chiave: chiave, Rif: rifA(chiave)},
			EntitaID: raw(`"e` + chiave + `"`), Foglia: raw("false"), Leggibile: raw("true"),
			StatoMotore: StatoMotore{Raggiunto: &vero, Tolto: &falso, Deciso: &falso},
			Grezzi:      raw(`{"id":"` + chiave + `","nome":"pezzo ` + chiave + `","formazioni":[]}`)}
	}
	radice := voce("#1")
	radice.Padri = raw(`[]`)
	radice.IdentitaProposta = raw(lettura(aProva, "l:u:step:#1:id:acme-num/sei:0-8", cliente, "acme", "ACM-1000"))
	radice.CodiceProposto, radice.RevProposta = "ACM-1000", "B"
	radice.DecisioneEsistente = raw(`{"componente_id":"` + pProva + `","codice":"ACM-1000","rev":"B"}`)
	radice.TipoProposto = raw(`{"valore":"finito","origine":"confermato"}`)
	radice.Riferimenti = raw(`[{"tipo":"documentale","valore":"ACM-1000","esito":"concorde","correzione":{"codice":"ACM-1000","revisione_inferiore":false},` +
		`"discordanze":[{"decisione":"ACM-1000","evidenza":"ACM-1000","revisione_inferiore":true}],"fonte":"pdf"}]`)
	telaio := voce("#7")
	telaio.Padri = raw(`["` + rifA("#1") + `"]`)
	telaio.IdentitaProposta = raw(lettura(aProva, "l:u:step:#7:id:acme-num/sei:0-8", cliente, "acme", "ACM-1100"))
	telaio.CodiceProposto, telaio.RevProposta = "ACM-1100", "A"
	telaio.TipoProposto = raw(`{"valore":"sottoassieme","origine":"regola"}`)
	telaio.AssociazioniProposte = raw(`[{"allegato_id":"` + fProva + `","motivo":"nome","livello":"alto"}]`)
	pezzo := voce("#9")
	pezzo.Padri = raw(`["` + rifA("#7") + `"]`)
	pezzo.Foglia = raw("true")
	pezzo.IdentitaProposta = raw(lettura(aProva, "l:u:step:#9:id:acme-num/sei:0-7", cliente, "acme", "P"+letteraE+"ZZO-9"))
	pezzo.MotivoProposto = raw(`"codice_non_ammissibile"`)
	pezzo.Alternative = raw(`[` + lettura(aProva, "l:u:step:#9:id:beta-num/sei:0-6", "partner:beta", "beta", "400123") + `]`)
	pezzo.CodiceManualeLegacy = raw(`{"codice":"ACM-1900","rev":"A","riga_legacy":"00000000-0000-4000-8000-0000000c0009"}`)
	cordone := voce("#31")
	cordone.Padri = raw(`["` + rifA("#7") + `"]`)
	cordone.Foglia, cordone.Leggibile = raw("true"), raw("false")
	cordone.Segnali = raw(`[{"codice":"ancoraggio.possibile_elemento_cad","regola":"elemento_cad_nome_foglia_v1",` +
		`"riscontri":[{"campo":"nome","unita_id":"u:#31","parole":["cordone"],"intervalli":[{"inizio":0,"fine":7}]}],"spiegazione":"possibile elemento CAD"}]`)
	cordone.FabbisogniPrevisti = raw(`[{"tipo":"disegno_2d","bloccante":true}]`)

	quantita := 2
	return Contenuto{
		FormatoRegistro: FormatoRegistro, VersioneImportazione: VersioneImportazione1, ThreadID: tProva, ProdottoID: pProva,
		Gruppo: Gruppo{Chiave: "bom", Tipo: raw(`"bom"`), AllegatoID: aProva, Sha256: shaA, Radice: rifA("#1"),
			NomeFile: raw(`"ACM-1000.stp"`), StatoStruttura: raw(`"bom_di_lavoro_proposta"`), RadiceDelFile: raw("true"),
			Compatibilita: raw(`"uguale"`), GrafoCompleto: raw("true")},
		Regole: Regole{Versioni: Versioni{Algoritmo: raw(`"a1-1"`), Ancoraggio: "ancoraggio-3", Importazione: VersioneImportazione1,
			Canonico: jsoncanonico.Versione, Valutazione: raw(`"v-4"`)},
			GrammaticaCliente: &GrammaticaCliente{ClienteID: cProva, Hash: strings.Repeat("7", 64)}},
		Calcolo: raw(`{"voci":{"` + voceA("#7") + `":{"calcolato_il":"2026-10-17T10:00:00Z","digest_payload":"d1"}},"impronta_indice":"i1"}`),
		Proposta: Proposta{
			Fonte: FonteProposta{AllegatoID: aProva, Sha256: shaA, NomeRicevuto: raw(`"ACM-1000.stp"`)},
			Nodi:  []Voce{radice, telaio, pezzo, cordone},
			Archi: []ArcoRegistro{{AllegatoID: aProva, Padre: rifA("#1"), Figlio: rifA("#7"), Quantita: &quantita},
				{AllegatoID: aProva, Padre: rifA("#7"), Figlio: rifA("#9"), Quantita: &quantita}},
			File: []FileRegistro{{Voce: raw(`"f:` + fProva + `"`), AllegatoID: fProva, Sha256: strings.Repeat("c", 64),
				NomeRicevuto: raw(`"ACM-1100.pdf"`), Origine: raw(`"allegato"`), Interpretazione: raw(`{"stato":"letta"}`),
				Letture:   raw(`[` + lettura(fProva, "l:u:pdf:1:id:beta-num/sei:0-6", "partner:beta", "beta", "400123") + `]`),
				Candidati: raw(`["` + voceA("#7") + `"]`)}},
			Relazioni: []Relazione{
				{Voce: raw(`"r:1"`), IDMotore: raw(`"rel-1"`), Tipo: raw(`"documentato_da"`), Stato: raw(`"proposta"`),
					Sorgente:        SorgenteRelazione{Tipo: "nodo", Voce: raw(`"` + voceA("#7") + `"`)},
					LetturaSorgente: raw(`"` + aProva + `/l:u:step:#7:id:acme-num/sei:0-8"`), LettureBersaglio: raw(`["` + fProva + `/l:u:pdf:1:id:beta-num/sei:0-6"]`),
					Evidenze: raw(`["u:1"]`)},
				{Voce: raw(`"r:2"`), IDMotore: raw(`"rel-2"`), Tipo: raw(`"varianti_nel_documento"`), Stato: raw(`"proposta"`),
					Sorgente: SorgenteRelazione{Tipo: "file", Voce: raw(`"f:` + fProva + `"`)}},
			},
			FuoriPerDecisione: []FuoriPerDecisione{{Voce: voceA("#12"), Segno: SegnoTolto, Chi: raw(`"` + uProva + `"`),
				Quando: raw(`"2026-10-16T09:00:00Z"`), Motivo: raw(`"duplicato"`)}},
			Diagnostiche: raw(`[{"codice":"importazione.prova","gravita":"avviso"}]`),
		},
		AllegatiCitati: []string{aProva, fProva},
	}
}

func componi(t *testing.T, c Contenuto) Registro {
	t.Helper()
	r, err := Componi(c)
	if err != nil {
		t.Fatalf("Componi: %v", err)
	}
	return r
}

// cambiato: il contenuto con un cambio fatto sul JSON (percorso libero), riletto in modo largo.
func cambiato(t *testing.T, c Contenuto, f func(m map[string]any)) Contenuto {
	t.Helper()
	m := mappaDi(t, c)
	f(m)
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var out Contenuto
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func mappaDi(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := jsoncanonico.Codifica(v)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func ogg(v any) map[string]any { return v.(map[string]any) }
func ele(v any) []any          { return v.([]any) }

// voceIn: la voce della mappa del contenuto con la chiave PRODUCT data.
func voceIn(m map[string]any, chiave string) map[string]any {
	for _, v := range ele(ogg(m["proposta"])["nodi"]) {
		if ogg(v)["voce"] == voceA(chiave) {
			return ogg(v)
		}
	}
	panic("voce " + chiave)
}

func TestComponiELeggiIlRegistro(t *testing.T) {
	r := componi(t, contenutoDiProva())
	if r.Impronta != jsoncanonico.Impronta(r.Canonico) {
		t.Fatal("l'impronta non e' lo sha256 dei byte canonici")
	}
	// la rilettura del testo come lo restituisce PostgreSQL (spazi, altro ordine delle chiavi) da' gli stessi byte
	var generico any
	if err := json.Unmarshal(r.Canonico, &generico); err != nil {
		t.Fatal(err)
	}
	conSpazi, err := json.MarshalIndent(generico, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Leggi(conSpazi)
	if err != nil {
		t.Fatalf("rilettura: %v", err)
	}
	if !bytes.Equal(r2.Canonico, r.Canonico) || r2.Impronta != r.Impronta {
		t.Fatal("la rilettura cambia i byte canonici")
	}
	// Componi e' deterministico, e ignora impronte scritte a mano
	c := contenutoDiProva()
	c.ImprontaContenuto = strings.Repeat("0", 64)
	if r3 := componi(t, c); r3.Impronta != r.Impronta {
		t.Fatal("Componi non e' deterministico o tiene un'impronta scritta da fuori")
	}
	col := r.Colonne()
	if col.Gruppo != "bom" || col.FonteRadice != rifA("#1") || col.FonteAllegatoID == nil || col.FonteAllegatoID.String() != aProva ||
		col.FonteSha256 != shaA || len(col.AllegatiCitati) != 2 || col.ImprontaContenuto != r.Contenuto.ImprontaContenuto {
		t.Fatalf("colonne: %+v", col)
	}
	if err := VerificaColonne(col, r); err != nil {
		t.Fatal(err)
	}
	for nome, guasta := range map[string]func(*Testata){
		"thread":    func(x *Testata) { x.ThreadID = uuid.MustParse(uProva) },
		"impronta":  func(x *Testata) { x.Impronta = strings.Repeat("1", 64) },
		"contenuto": func(x *Testata) { x.ImprontaContenuto = strings.Repeat("1", 64) },
		"gruppo":    func(x *Testata) { x.Gruppo = "cand:x" },
		"fonte":     func(x *Testata) { x.FonteAllegatoID = nil },
		"radice":    func(x *Testata) { x.FonteRadice = rifA("#2") },
		"citati":    func(x *Testata) { x.AllegatiCitati = x.AllegatiCitati[:1] },
	} {
		x := r.Colonne()
		guasta(&x)
		if err := VerificaColonne(x, r); !errors.Is(err, ErrRegistroIncoerente) {
			t.Errorf("%s: atteso ErrRegistroIncoerente, %v", nome, err)
		}
	}
	if string(r.Grezzo("regole")) == "" || !strings.Contains(string(r.Grezzo("regole")), `"importazione":"importazione-1"`) {
		t.Fatalf("regole grezze: %s", r.Grezzo("regole"))
	}
}

func TestIlRegistroSiLeggeStretto(t *testing.T) {
	r := componi(t, contenutoDiProva())
	base := mappaDi(t, json.RawMessage(r.Canonico))
	casi := []struct {
		nome, campo string
		cambia      func(m map[string]any)
	}{
		{"campo sconosciuto in testa", "contenuto.inventato", func(m map[string]any) { m["inventato"] = "x" }},
		// (le voci stanno in ordine di rif: #1, #31, #7, #9)
		{"campo sconosciuto nella voce", "proposta.nodi[2].orario", func(m map[string]any) { voceIn(m, "#7")["orario"] = "2026-10-17" }},
		{"campo sconosciuto nel gruppo", "gruppo.bundle_id", func(m map[string]any) { ogg(m["gruppo"])["bundle_id"] = "b" }},
		{"nome con la maiuscola", "contenuto.Proposta", func(m map[string]any) { m["Proposta"] = m["proposta"]; delete(m, "proposta") }},
		{"stato_motore con un campo in piu'", "stato_motore.calcolato_il", func(m map[string]any) {
			ogg(voceIn(m, "#7")["stato_motore"])["calcolato_il"] = "x"
		}},
		{"numero decimale", "non e' un intero", func(m map[string]any) {
			ogg(m["proposta"])["archi"].([]any)[0].(map[string]any)["quantita"] = json.Number("2.5")
		}},
		{"numero con esponente", "non e' un intero", func(m map[string]any) { m["formato_registro"] = json.Number("1e0") }},
		{"U+0000", "U+0000", func(m map[string]any) { voceIn(m, "#7")["codice_proposto"] = "A\x00B" }},
		{"formato del registro", "formato_registro", func(m map[string]any) { m["formato_registro"] = 2 }},
		{"gruppo senza", "gruppo.chiave", func(m map[string]any) { ogg(m["gruppo"])["chiave"] = "senza" }},
		{"radice di un altro sha", "gruppo.radice", func(m map[string]any) { ogg(m["gruppo"])["radice"] = "nodo:" + strings.Repeat("d", 64) + ":#1" }},
		{"voce che non e' la sua origine", "proposta.nodi[2].voce", func(m map[string]any) { voceIn(m, "#7")["voce"] = voceA("#8") }},
		{"origine con un'altra chiave", "proposta.nodi[2].origine", func(m map[string]any) { ogg(voceIn(m, "#7")["origine"])["chiave"] = "#8" }},
		{"codice proposto con uno spazio", "proposta.nodi[2].codice_proposto", func(m map[string]any) { voceIn(m, "#7")["codice_proposto"] = "ACM 1100" }},
		{"codice proposto con lo spazio unificatore", "codice_proposto", func(m map[string]any) { voceIn(m, "#7")["codice_proposto"] = "ACM\u00a01100" }},
		{"codice proposto oltre 40 byte", "codice_proposto", func(m map[string]any) { voceIn(m, "#7")["codice_proposto"] = strings.Repeat("A", 41) }},
		{"rev proposta minuscola", "proposta.nodi[2].rev_proposta", func(m map[string]any) { voceIn(m, "#7")["rev_proposta"] = "a" }},
		{"rev proposta con uno spazio", "rev_proposta", func(m map[string]any) { voceIn(m, "#7")["rev_proposta"] = "A 1" }},
		{"radice senza la sua voce", "gruppo.radice", func(m map[string]any) {
			p := ogg(m["proposta"])
			var resto []any
			for _, v := range ele(p["nodi"]) {
				if ogg(v)["voce"] != voceA("#1") {
					resto = append(resto, v)
				}
			}
			p["nodi"] = resto
		}},
		{"arco verso un nodo che non e' una voce", "figlio", func(m map[string]any) {
			p := ogg(m["proposta"])
			p["archi"] = append(ele(p["archi"]), map[string]any{"allegato_id": aProva, "padre": rifA("#7"), "figlio": rifA("#404"), "quantita": 1})
		}},
		{"chiave di lettura con due letture diverse", "chiave_lettura", func(m map[string]any) {
			v := voceIn(m, "#9")
			altra := mappaDi(t, json.RawMessage(lettura(aProva, "l:u:step:#9:id:beta-num/sei:0-6", "partner:beta", "beta", "400124")))
			v["alternative"] = append(ele(v["alternative"]), altra)
		}},
		{"chiave di lettura di un altro allegato", "chiave_lettura", func(m map[string]any) {
			ogg(ele(voceIn(m, "#9")["alternative"])[0])["chiave_lettura"] = fProva + "/l:u:step:#9:id:beta-num/sei:0-6"
		}},
		{"lettura di un file con la chiave della voce", "chiave_lettura", func(m map[string]any) {
			ogg(ele(ogg(ele(ogg(m["proposta"])["file"])[0])["letture"])[0])["chiave_lettura"] = aProva + "/l:u:pdf:1:id:beta-num/sei:0-6"
		}},
		{"lettura senza chiave", "chiave_lettura", func(m map[string]any) { delete(ogg(voceIn(m, "#7")["identita_proposta"]), "chiave_lettura") }},
		// null dove il §3.8.2 vuole assente (senza contesto: difetti di forma)
		{"contesto null", "contesto", func(m map[string]any) { m["contesto"] = nil }},
		{"partner null", "regole.partner", func(m map[string]any) { ogg(m["regole"])["partner"] = nil }},
		{"contesto delle regole null", "regole.contesto", func(m map[string]any) { ogg(m["regole"])["contesto"] = nil }},
		{"grammatica del cliente null", "regole.grammatica_cliente", func(m map[string]any) { ogg(m["regole"])["grammatica_cliente"] = nil }},
		{"allegato non citato", "allegati_citati", func(m map[string]any) { m["allegati_citati"] = []any{fProva} }},
		{"citati fuori ordine", "allegati_citati[1]", func(m map[string]any) { m["allegati_citati"] = []any{fProva, aProva} }},
		{"relazione di un'altra sorgente", "sorgente.tipo", func(m map[string]any) {
			ogg(ele(ogg(m["proposta"])["relazioni"])[0])["sorgente"] = map[string]any{"tipo": "mail"}
		}},
		{"segno sconosciuto", "segno", func(m map[string]any) { ogg(ele(ogg(m["proposta"])["fuori_per_decisione"])[0])["segno"] = "perso" }},
		{"radice fuori per decisione", "voce", func(m map[string]any) {
			ogg(ele(ogg(m["proposta"])["fuori_per_decisione"])[0])["voce"] = voceA("#1")
		}},
		{"impronta del contenuto ritoccata", "impronta_contenuto", func(m map[string]any) { m["impronta_contenuto"] = strings.Repeat("0", 64) }},
		{"un campo del confronto cambiato senza ricalcolo", "impronta_contenuto", func(m map[string]any) { voceIn(m, "#7")["codice_proposto"] = "ACM-1101" }},
		{"versione dell'adattatore diversa", "regole.versioni.importazione", func(m map[string]any) {
			ogg(ogg(m["regole"])["versioni"])["importazione"] = "importazione-2"
		}},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			m := mappaDi(t, base)
			c.cambia(m)
			b, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Leggi(b)
			if err == nil || !strings.Contains(err.Error(), c.campo) {
				t.Fatalf("atteso un rifiuto su %q, %v", c.campo, err)
			}
		})
	}
	// la chiave ripetuta non si legge (rilettura di jsoncanonico)
	doppia := bytes.Replace(r.Canonico, []byte(`{"allegati_citati":`), []byte(`{"allegati_citati":[],"allegati_citati":`), 1)
	if _, err := Leggi(doppia); err == nil {
		t.Fatal("una chiave ripetuta passa")
	}
}

// TestElencoChiusoDelRegistro (la parte strutturale di T-13, PM23 §11): ogni campo tipizzato del contenuto ha la sua
// classe nell'elenco chiuso, o la eredita da un livello classificato per intero; nessuna classe nomina un campo che non
// c'e'. Un campo nuovo del registro senza una riga fa fallire la prova.
func TestElencoChiusoDelRegistro(t *testing.T) {
	tipizzati := map[string]bool{}
	var scorri func(tp reflect.Type, prefisso string)
	scorri = func(tp reflect.Type, prefisso string) {
		for tp.Kind() == reflect.Pointer {
			tp = tp.Elem()
		}
		if tp.Kind() != reflect.Struct || tp == tipoUUID {
			return
		}
		for i := 0; i < tp.NumField(); i++ {
			f := tp.Field(i)
			nome, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			p := prefisso + nome
			tipizzati[p] = true
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			switch {
			case ft == tipoRaw:
			case ft.Kind() == reflect.Slice && ft.Elem().Kind() == reflect.Struct:
				scorri(ft.Elem(), p+"[].")
			case ft.Kind() == reflect.Struct:
				scorri(ft, p+".")
			}
		}
	}
	scorri(reflect.TypeFor[Contenuto](), "")
	cl := Classificazione()
	for p := range tipizzati {
		if _, ok := cl[p]; ok {
			continue
		}
		// eredita la classe dall'antenato classificato piu' vicino, purche' non sia «parti»
		antenato, classe := p, ""
		for classe == "" {
			i := strings.LastIndex(antenato, ".")
			if i < 0 {
				break
			}
			antenato = strings.TrimSuffix(antenato[:i], "[]")
			classe = cl[antenato]
		}
		if classe == "" || classe == Parti {
			t.Errorf("il campo %s non ha una classe nell'elenco chiuso (EM §3.8.1)", p)
		}
	}
	for p, c := range cl {
		if !tipizzati[p] {
			t.Errorf("l'elenco chiuso classifica %s (%s), che il contenuto non ha", p, c)
		}
		switch c {
		case InContenuto, InEvidenze, InRegole, SoloImprontaIntera, PerSorgente, Parti:
		default:
			t.Errorf("classe sconosciuta %q per %s", c, p)
		}
	}
	// i campi con un orario o un ID di calcolo non entrano mai nel confronto
	for _, p := range []string{"calcolo", "contesto", "proposta.fuori_per_decisione[].quando", "proposta.fuori_per_decisione[].chi",
		"proposta.nodi[].riga_legacy", "proposta.nodi[].stato_motore.deciso", "gruppo.chiave"} {
		if cl[p] == InContenuto {
			t.Errorf("%s non deve stare nel confronto", p)
		}
	}
}

// TestLeImpronteSeguonoLElencoChiuso: che cosa cambia quale impronta (EM §3.8.1; C-15, C-19; O-01).
func TestLeImpronteSeguonoLElencoChiuso(t *testing.T) {
	r0 := componi(t, contenutoDiProva())
	confronta := func(t *testing.T, nome string, f func(m map[string]any), contenuto, evidenze, regole bool) {
		t.Helper()
		r := componi(t, cambiato(t, contenutoDiProva(), f))
		if r.Impronta == r0.Impronta {
			t.Errorf("%s: l'impronta intera non cambia", nome)
		}
		for _, x := range []struct {
			cosa   string
			prima  string
			dopo   string
			attesa bool
		}{{"impronta_contenuto", r0.Contenuto.ImprontaContenuto, r.Contenuto.ImprontaContenuto, contenuto},
			{"impronta_evidenze", r0.Contenuto.ImprontaEvidenze, r.Contenuto.ImprontaEvidenze, evidenze},
			{"impronta_regole", r0.Contenuto.ImprontaRegole, r.Contenuto.ImprontaRegole, regole}} {
			if (x.prima != x.dopo) != x.attesa {
				t.Errorf("%s: %s cambia = %v, atteso %v", nome, x.cosa, x.prima != x.dopo, x.attesa)
			}
		}
	}
	confronta(t, "un orario nel calcolo (C-15)", func(m map[string]any) {
		m["calcolo"] = map[string]any{"voci": map[string]any{voceA("#7"): map[string]any{"calcolato_il": "2026-10-18T08:00:00Z"}}}
	}, false, false, false)
	confronta(t, "il codice proposto di una voce", func(m map[string]any) { voceIn(m, "#7")["codice_proposto"] = "ACM-1101" }, true, false, false)
	confronta(t, "decisione_esistente di una voce sotto la radice", func(m map[string]any) {
		voceIn(m, "#7")["decisione_esistente"] = map[string]any{"componente_id": uProva, "codice": "ACM-1100"}
	}, true, false, false)
	confronta(t, "decisione_esistente della radice (O-01)", func(m map[string]any) {
		voceIn(m, "#1")["decisione_esistente"] = map[string]any{"componente_id": uProva, "codice": "ACM-1000"}
	}, false, true, false)
	confronta(t, "tipo proposto della radice", func(m map[string]any) { delete(voceIn(m, "#1"), "tipo_proposto") }, false, true, false)
	confronta(t, "raggiunto della radice", func(m map[string]any) { ogg(voceIn(m, "#1")["stato_motore"])["raggiunto"] = false }, false, true, false)
	confronta(t, "raggiunto di una voce", func(m map[string]any) { ogg(voceIn(m, "#7")["stato_motore"])["raggiunto"] = false }, true, false, false)
	confronta(t, "deciso di una voce (riga legacy)", func(m map[string]any) { ogg(voceIn(m, "#7")["stato_motore"])["deciso"] = true }, false, true, false)
	confronta(t, "le discordanze del documentale della radice", func(m map[string]any) {
		ogg(ele(voceIn(m, "#1")["riferimenti"])[0])["discordanze"] = []any{}
	}, false, true, false)
	confronta(t, "la revisione_inferiore della correzione della radice", func(m map[string]any) {
		ogg(ogg(ele(voceIn(m, "#1")["riferimenti"])[0])["correzione"])["revisione_inferiore"] = true
	}, false, true, false)
	confronta(t, "l'esito del documentale della radice", func(m map[string]any) {
		ogg(ele(voceIn(m, "#1")["riferimenti"])[0])["esito"] = "discorde"
	}, true, false, false)
	confronta(t, "la chiave del gruppo, da bom a cand (O-01)", func(m map[string]any) {
		ogg(m["gruppo"])["chiave"] = "cand:" + aProva + ":" + rifA("#1")
		ogg(m["gruppo"])["tipo"] = "cand"
	}, false, true, false)
	confronta(t, "la completezza del grafo", func(m map[string]any) { ogg(m["gruppo"])["grafo_completo"] = false }, true, false, false)
	confronta(t, "una lettura di un file", func(m map[string]any) {
		ogg(ele(ogg(m["proposta"])["file"])[0])["nome_ricevuto"] = "ACM-1100_rev.pdf"
	}, false, true, false)
	confronta(t, "una relazione dal nodo", func(m map[string]any) {
		ogg(ele(ogg(m["proposta"])["relazioni"])[0])["stato"] = "scartata"
	}, true, false, false)
	confronta(t, "una relazione dal file", func(m map[string]any) {
		ogg(ele(ogg(m["proposta"])["relazioni"])[1])["stato"] = "scartata"
	}, false, true, false)
	confronta(t, "chi ha tolto un nodo", func(m map[string]any) {
		ogg(ele(ogg(m["proposta"])["fuori_per_decisione"])[0])["chi"] = cProva
	}, false, true, false)
	confronta(t, "il segno di un nodo fuori", func(m map[string]any) {
		ogg(ele(ogg(m["proposta"])["fuori_per_decisione"])[0])["segno"] = SegnoNonRaggiunto
	}, true, false, false)
	confronta(t, "la grammatica del cliente", func(m map[string]any) {
		ogg(ogg(m["regole"])["grammatica_cliente"])["hash"] = strings.Repeat("9", 64)
	}, false, false, true)
	confronta(t, "il nome ricevuto della fonte", func(m map[string]any) {
		ogg(ogg(m["proposta"])["fonte"])["nome_ricevuto"] = "altro.stp"
	}, false, true, false)
	// null, "", [] e {} valgono come assenti per le impronte parziali, anche annidati e nella parte confrontata, e l'ordine
	// di voci e archi non conta (la forma canonica del registro)
	for nome, f := range map[string]func(m map[string]any){
		"un campo null":                  func(m map[string]any) { voceIn(m, "#7")["motivo_lettura"] = nil },
		"un elenco vuoto nel confronto":  func(m map[string]any) { voceIn(m, "#7")["alternative"] = []any{} },
		"un null annidato nel confronto": func(m map[string]any) { ogg(voceIn(m, "#7")["identita_proposta"])["revisione"] = nil },
		"un oggetto vuoto fra le evidenze": func(m map[string]any) {
			ogg(ele(ogg(m["proposta"])["file"])[0])["interpretazione"] = map[string]any{"stato": "letta", "contesto_id": nil}
		},
		"gli archi al rovescio": func(m map[string]any) {
			l := ele(ogg(m["proposta"])["archi"])
			l[0], l[1] = l[1], l[0]
		},
	} {
		x := cambiato(t, contenutoDiProva(), f)
		imp, err := ImpronteParziali(mustCodifica(t, x))
		if err != nil {
			t.Fatal(err)
		}
		if imp.Contenuto != r0.Contenuto.ImprontaContenuto || imp.Evidenze != r0.Contenuto.ImprontaEvidenze || imp.Regole != r0.Contenuto.ImprontaRegole {
			t.Errorf("%s cambia le impronte parziali", nome)
		}
		if r := componi(t, x); r.Impronta != r0.Impronta {
			t.Errorf("%s cambia l'impronta intera", nome)
		}
	}
}

func mustCodifica(t *testing.T, v any) []byte {
	t.Helper()
	b, err := jsoncanonico.Codifica(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestIlRegistroOltreIlLimite(t *testing.T) {
	c := contenutoDiProva()
	c.Calcolo = raw(`{"zavorra":"` + strings.Repeat("x", MaxContenuto) + `"}`)
	_, err := Componi(c)
	var e *Errore
	if !errors.As(err, &e) || e.Codice != CodiceTroppoGrande {
		t.Fatalf("atteso %s, %v", CodiceTroppoGrande, err)
	}
	// appena sotto il limite passa
	c = contenutoDiProva()
	r := componi(t, c)
	spazio := MaxContenuto - len(r.Canonico) - len(`,"zavorra":""`) - 100
	c.Calcolo = raw(`{"zavorra":"` + strings.Repeat("x", spazio) + `"}`)
	if _, err := Componi(c); err != nil {
		t.Fatalf("sotto il limite: %v", err)
	}
}

func TestLaCompletezzaDelContesto(t *testing.T) {
	conContesto := func() Contenuto {
		c := contenutoDiProva()
		c.Regole.Contesto = &ContestoRegole{ID: raw(`"ctx-acme"`), Impronta: strings.Repeat("5", 64)}
		p := []PartnerAttivo{{Emittente: "partner:beta", Hash: strings.Repeat("8", 64)}}
		c.Regole.Partner = &p
		c.Contesto = raw(`{"id":"ctx-acme","eredita_da":null,"partner":["beta"]}`)
		return c
	}
	r := componi(t, conContesto())
	if got := r.Contenuto.PartnerAttivi(); len(got) != 1 || got[0] != "partner:beta" {
		t.Fatalf("partner attivi: %v", got)
	}
	nessuno := conContesto()
	vuoto := []PartnerAttivo{}
	nessuno.Regole.Partner = &vuoto
	// con il contesto e nessun partner attivo: un elenco vuoto, ammesso, e scritto (la forma canonica non lo toglie)
	if r := componi(t, nessuno); !bytes.Contains(r.Canonico, []byte(`"partner":[]`)) {
		t.Fatal("l'elenco vuoto dei partner con il contesto attivo non si scrive")
	}
	for nome, guasta := range map[string]func(*Contenuto){
		"senza partner":         func(c *Contenuto) { c.Regole.Partner = nil },
		"senza la copia":        func(c *Contenuto) { c.Contesto = nil },
		"con la copia null":     func(c *Contenuto) { c.Contesto = raw("null") },
		"con la copia vuota":    func(c *Contenuto) { c.Contesto = raw("{}") },
		"senza impronta":        func(c *Contenuto) { c.Regole.Contesto.Impronta = "" },
		"senza id del contesto": func(c *Contenuto) { c.Regole.Contesto.ID = nil },
		"senza impronta_indice": func(c *Contenuto) { c.Calcolo = raw(`{"voci":{},"bundle_id":"b-1"}`) },
		"impronta_indice vuota": func(c *Contenuto) { c.Calcolo = raw(`{"impronta_indice":""}`) },
		"senza calcolo":         func(c *Contenuto) { c.Calcolo = nil },
	} {
		c := conContesto()
		guasta(&c)
		_, err := Componi(c)
		var e *Errore
		if !errors.As(err, &e) || e.Codice != CodicePropostaNonDisponibile || e.Motivo != MotivoRegistroIncompleto {
			t.Errorf("%s: atteso %s/%s, %v", nome, CodicePropostaNonDisponibile, MotivoRegistroIncompleto, err)
		}
	}
	// senza contesto, partner e copia del contesto sono assenti
	c := contenutoDiProva()
	p := []PartnerAttivo{{Emittente: "partner:beta", Hash: strings.Repeat("8", 64)}}
	c.Regole.Partner = &p
	if _, err := Componi(c); err == nil {
		t.Fatal("partner attivi senza contesto passano")
	}
	c = contenutoDiProva()
	c.Contesto = raw(`{"id":"x"}`)
	if _, err := Componi(c); err == nil {
		t.Fatal("una copia del contesto senza contesto nelle regole passa")
	}
	// un null al posto della copia del contesto: con il contesto attivo e' un registro incompleto (sopra), senza e' un
	// difetto di forma, mai un «assente» (EM §3.8.2; PM23 §10 righe 2-3)
	c = contenutoDiProva()
	c.Contesto = raw("null")
	_, err := Componi(c)
	var e *Errore
	if !errors.As(err, &e) || e.Codice != "" || e.Campo != "contesto" {
		t.Fatalf("contesto null senza contesto: atteso un difetto di forma, %v", err)
	}
	// Leggi distingue allo stesso modo: il testo con un null nei campi del §3.8.2 non si legge
	m := mappaDi(t, json.RawMessage(componi(t, conContesto()).Canonico))
	ogg(m["regole"])["partner"] = nil
	b, _ := json.Marshal(m)
	if _, err := Leggi(b); !errors.As(err, &e) || e.Codice != CodicePropostaNonDisponibile || e.Motivo != MotivoRegistroIncompleto {
		t.Fatalf("partner null con il contesto attivo: atteso %s/%s, %v", CodicePropostaNonDisponibile, MotivoRegistroIncompleto, err)
	}
}

// TestLaFormaCanonicaDelRegistro (EM §3.10, §3.8; P-12 e P-19 del resoconto della corsia persistenza): lo stesso
// contenuto da' gli stessi byte e le stesse impronte comunque sia scritto: i membri null, "", [] e {} a ogni livello
// valgono come assenti e non si scrivono (salvo regole.partner, provato sopra), le voci e gli archi si mettono in
// ordine. Leggi e Componi danno gli stessi byte.
func TestLaFormaCanonicaDelRegistro(t *testing.T) {
	r0 := componi(t, contenutoDiProva())
	// lo stesso contenuto con dei vuoti annidati, anche nella parte confrontata, e gli elenchi al rovescio
	m := mappaDi(t, json.RawMessage(r0.Canonico))
	voceIn(m, "#7")["motivo_lettura"] = nil
	voceIn(m, "#7")["alternative"] = []any{}
	voceIn(m, "#7")["codice_composto"] = map[string]any{}
	voceIn(m, "#9")["motivo_rev"] = ""
	ogg(voceIn(m, "#1")["identita_proposta"])["qualificatori"] = map[string]any{"affissi": []any{}, "decorazioni": []any{}}
	ogg(m["gruppo"])["motivo_grafo"] = nil
	ogg(m["proposta"])["relazioni"].([]any)[0].(map[string]any)["modello"] = ""
	p := ogg(m["proposta"])
	for _, k := range []string{"nodi", "archi"} {
		l := ele(p[k])
		for i, j := 0, len(l)-1; i < j; i, j = i+1, j-1 {
			l[i], l[j] = l[j], l[i]
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	r1, err := Leggi(b)
	if err != nil {
		t.Fatalf("lo stesso contenuto con i vuoti e gli elenchi al rovescio non si legge: %v", err)
	}
	if !bytes.Equal(r1.Canonico, r0.Canonico) || r1.Impronta != r0.Impronta {
		t.Fatalf("vuoti e ordine cambiano i byte o l'impronta:\n%s\n%s", r1.Canonico, r0.Canonico)
	}
	// Componi sullo stesso contenuto scritto cosi' da' lo stesso registro
	r2 := componi(t, cambiato(t, contenutoDiProva(), func(m map[string]any) {
		voceIn(m, "#7")["alternative"] = []any{}
		voceIn(m, "#9")["motivo_rev"] = ""
		p := ogg(m["proposta"])
		l := ele(p["nodi"])
		l[0], l[len(l)-1] = l[len(l)-1], l[0]
	}))
	if !bytes.Equal(r2.Canonico, r0.Canonico) {
		t.Fatalf("Componi dipende dai vuoti o dall'ordine delle voci:\n%s\n%s", r2.Canonico, r0.Canonico)
	}
	// Leggi e Componi: Componi(Leggi(x).Contenuto) ha gli stessi byte di Leggi(x)
	for nome, reg := range map[string]Registro{"senza contesto": r0, "con il contesto": componi(t, func() Contenuto {
		c := contenutoDiProva()
		c.Regole.Contesto = &ContestoRegole{ID: raw(`"ctx-acme"`), Impronta: strings.Repeat("5", 64)}
		p := []PartnerAttivo{}
		c.Regole.Partner = &p
		c.Contesto = raw(`{"id":"ctx-acme","eredita_da":null,"partner":[]}`)
		return c
	}())} {
		letto, err := Leggi(reg.Canonico)
		if err != nil {
			t.Fatalf("%s: %v", nome, err)
		}
		ricomposto := componi(t, letto.Contenuto)
		if !bytes.Equal(ricomposto.Canonico, letto.Canonico) || ricomposto.Impronta != letto.Impronta {
			t.Fatalf("%s: Componi(Leggi(x).Contenuto) cambia i byte:\n%s\n%s", nome, ricomposto.Canonico, letto.Canonico)
		}
	}
	// nei byte canonici non resta un vuoto, salvo l'elenco dei partner
	for _, vuoto := range []string{`:null`, `:""`, `:{}`} {
		if bytes.Contains(r0.Canonico, []byte(vuoto)) {
			t.Errorf("i byte canonici portano %s", vuoto)
		}
	}
	if bytes.Contains(r0.Canonico, []byte(`:[]`)) {
		t.Error("i byte canonici portano un elenco vuoto")
	}
}

// TestLaLetturaCompattaByteperByte (N-01, la parte del registro): il letterale con «e» + accento combinante resta tale
// nel registro, nella lettura ritrovata e nella sua copia compatta; la copia normalizzata in NFC non e' piu' la stessa.
func TestLaLetturaCompattaByteperByte(t *testing.T) {
	r := componi(t, contenutoDiProva())
	if !bytes.Contains(r.Canonico, []byte("P"+letteraE+"ZZO-9")) {
		t.Fatal("il registro canonico ha perso la «e» con l'accento combinante")
	}
	chiave := aProva + "/l:u:step:#9:id:acme-num/sei:0-7"
	l, ok := r.Contenuto.TrovaLettura(chiave)
	if !ok || l.Letterale != "P"+letteraE+"ZZO-9" {
		t.Fatalf("lettura %s: %+v %v", chiave, l, ok)
	}
	cp, err := l.Compatta()
	if err != nil {
		t.Fatal(err)
	}
	if cp.Letterale != "P"+letteraE+"ZZO-9" || cp.Base.Originale != "P"+letteraE+"ZZO-9" || cp.Qualificatori != nil || cp.Revisione != nil {
		t.Fatalf("copia compatta: %+v", cp)
	}
	can, err := cp.Canonica()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(can, []byte(letteraE)) {
		t.Fatal("la copia compatta ha perso l'accento combinante")
	}
	// la stessa copia come la manda il browser, con revisione null e qualificatori vuoti: uguale
	var dalBrowser LetturaCompatta
	if err := json.Unmarshal([]byte(`{"emittente":"cliente:`+cProva+`","hash_grammatica":"`+strings.Repeat("7", 64)+`","namespace":"acme",`+
		`"letterale":"P`+"e\\u0301"+`ZZO-9","base":{"originale":"P`+"e\\u0301"+`ZZO-9","normalizzata":"P`+"e\\u0301"+`ZZO-9"},"qualificatori":{},"revisione":null}`), &dalBrowser); err != nil {
		t.Fatal(err)
	}
	if !StessaLettura(cp, dalBrowser) {
		t.Fatal("la copia del browser con null e {} non e' la stessa lettura")
	}
	composta := dalBrowser
	composta.Letterale = "PÉZZO-9"
	if StessaLettura(cp, composta) {
		t.Fatal("una copia normalizzata in NFC vale come la lettura del registro")
	}
	// le letture si trovano anche fra le alternative e nei file
	if _, ok := r.Contenuto.TrovaLettura(aProva + "/l:u:step:#9:id:beta-num/sei:0-6"); !ok {
		t.Fatal("l'alternativa non si trova")
	}
	if _, ok := r.Contenuto.TrovaLettura(fProva + "/l:u:pdf:1:id:beta-num/sei:0-6"); !ok {
		t.Fatal("la lettura del file non si trova")
	}
	if _, ok := r.Contenuto.TrovaLettura(aProva + "/nessuna"); ok {
		t.Fatal("una lettura che non c'e' si trova")
	}
}

func TestISegnaliDellaVoce(t *testing.T) {
	r := componi(t, contenutoDiProva())
	ss, err := r.Contenuto.SegnaliDellaVoce(voceA("#31"))
	if err != nil || len(ss) != 1 {
		t.Fatalf("segnali: %v %v", ss, err)
	}
	if ss[0] != (SegnaleVisto{Codice: "ancoraggio.possibile_elemento_cad", Regola: "elemento_cad_nome_foglia_v1", VersioneServizio: "ancoraggio-3"}) {
		t.Fatalf("segnale visto: %+v", ss[0])
	}
	if ss, _ := r.Contenuto.SegnaliDellaVoce(voceA("#7")); len(ss) != 0 {
		t.Fatalf("una voce senza segnali ne ha: %v", ss)
	}
}

func TestLeVociDelRegistro(t *testing.T) {
	r := componi(t, contenutoDiProva())
	a := uuid.MustParse(aProva)
	if v, ok := r.Contenuto.VocePerRif(a, rifA("#7")); !ok || v.CodiceProposto != "ACM-1100" {
		t.Fatalf("voce #7: %+v %v", v, ok)
	}
	if _, ok := r.Contenuto.VocePerRif(uuid.MustParse(fProva), rifA("#7")); ok {
		t.Fatal("lo stesso rif su un'altra occorrenza vale come la voce (V-3)")
	}
	if x, ok := r.Contenuto.FuoriPerRif(a, rifA("#12")); !ok || x.Segno != SegnoTolto {
		t.Fatalf("fuori per decisione: %+v %v", x, ok)
	}
	if v, _ := r.Contenuto.VocePerRif(a, rifA("#1")); !r.Contenuto.EVoceDellaRadice(v) {
		t.Fatal("la radice non si riconosce")
	}
	all, rif, ok := PartiDellaVoce(voceA("#9"))
	if !ok || all != aProva || rif != rifA("#9") {
		t.Fatalf("parti della voce: %s %s %v", all, rif, ok)
	}
	for _, v := range []string{"", "n:", "n:" + aProva, "x:" + aProva + ":" + rifA("#1"), "n:" + aProva + ":nodo:corto:#1"} {
		if _, _, ok := PartiDellaVoce(v); ok {
			t.Errorf("voce %q accettata", v)
		}
	}
}
