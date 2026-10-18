// L1 — l'adattatore del registro d'importazione con i partner, i legami, le equivalenze K4 e le occorrenze dello stesso
// sha (V1: emendamento di V1 §3.8, §3.8.2, §6.1, §6.5; piano della 0023 §14 passo 3, prove A-04, A-06, A-07, A-08 e A-18
// di A3 §1.9, con le attese di PM23 §10). Come importazione_registro_scena_test.go: Calcola sulle fotografie sintetiche,
// RegoleDiLettura, ImportaGruppo, la rilettura stretta. Tutto è inventato: ACME il cliente, BETA il fornitore che
// disegna per lui (un codice di disegno di sei cifre), i codici 712xxxx e PNxxxxx, gli UUID di prova.
package valutazione_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

var (
	aStepGemello = uid(0xe8a1) // un secondo allegato con lo stesso sha dello STEP (A-08)
	aDxfK4       = uid(0xe8a2) // il DXF del cartiglio con il riferimento 3D (A-06)
	shaDxfK4     = sha("e")
)

// I nomi della scena del legame (la forma del caso dell'assieme del fornitore di D23 §8, con codici inventati): la radice dello STEP si chiama
// con il codice del cliente e il disegno del fornitore, i figli con i disegni del fornitore.
const (
	idRadiceLegame = "7120100A-654301-ASM"
	nomeStepLegame = "7120100A_1-654301-ASM.stp"
)

// legamiDiProva: i due modelli d'accoppiamento di ACME, sull'id della radice dello STEP e sul nome del file:
// [codice del cliente]-[disegno del fornitore]-ASM, con l'estensione per il nome.
func legamiDiProva() []grammatica.ModelloLegame {
	pezzo := func(ruolo, famiglia string) grammatica.PezzoLegame {
		return grammatica.PezzoLegame{Tipo: grammatica.PezzoLettura, Ruolo: ruolo, Famiglie: []string{famiglia}}
	}
	sep := func(l string) grammatica.PezzoLegame {
		return grammatica.PezzoLegame{Tipo: grammatica.PezzoSeparatore, Letterali: []string{l}}
	}
	esempio := func(sel, testo, modello string, letture ...grammatica.LetturaAttesa) grammatica.EsempioLegame {
		return grammatica.EsempioLegame{ID: "e1", Origine: grammatica.OrigineSintetico, Selettore: sel, Testo: testo,
			Attivazione: grammatica.AttivazioneEsempio{Partner: []string{"beta"}, Codifica: grammatica.CodificaMista, Modelli: []string{modello}},
			Atteso:      grammatica.AttesoLegame{Letture: letture}}
	}
	return []grammatica.ModelloLegame{
		{ID: "assieme-radice", Stato: grammatica.StatoAttiva, Selettori: []string{"radice_step.id"},
			Schema: []grammatica.PezzoLegame{pezzo(grammatica.RuoloLegameCliente, "acme-catena"), sep("-"), pezzo(grammatica.RuoloLegameFornitore, "beta-dis"), sep("-ASM")},
			Esempi: []grammatica.EsempioLegame{esempio("radice_step.id", idRadiceLegame, "assieme-radice",
				grammatica.LetturaAttesa{Ruolo: "primaria", Famiglia: "acme-catena", Base: "7120100"},
				grammatica.LetturaAttesa{Ruolo: "riferimento", Famiglia: "beta-dis", Base: "654301"})}},
		{ID: "assieme", Stato: grammatica.StatoAttiva, Selettori: []string{"nome_file"},
			Schema: []grammatica.PezzoLegame{pezzo(grammatica.RuoloLegameCliente, "acme-catena"), sep("-"), pezzo(grammatica.RuoloLegameFornitore, "beta-dis"), sep("-ASM"),
				{Tipo: grammatica.PezzoEstensione, Letterali: []string{"stp"}}},
			Esempi: []grammatica.EsempioLegame{esempio("nome_file", nomeStepLegame, "assieme",
				grammatica.LetturaAttesa{Ruolo: "primaria", Famiglia: "acme-catena", Base: "7120100"},
				grammatica.LetturaAttesa{Ruolo: "riferimento", Famiglia: "beta-dis", Base: "654301"})}},
	}
}

// insiemeConLegami: ACME con la famiglia della catena, BETA dichiarato con l'hash della sua grammatica e i due legami,
// l'emittente BETA nell'indice (la porta del prodotto, CompilaInsieme); poi i contesti dati. betaExtra aggiunge a BETA
// una famiglia che nella scena non legge niente: cambia l'hash di BETA e nient'altro (A-18). Dà anche l'hash di BETA.
func insiemeConLegami(t *testing.T, betaExtra bool, contesti ...motorea.ContestoLavorazione) (*motorea.InsiemeRegole, string) {
	t.Helper()
	lim := limitiACME()
	selPartner := []string{"nome_file", "nodo_step.id", "radice_step.id", "cartiglio.codice"}
	baseBeta := func(pattern string) grammatica.Base {
		return grammatica.Base{Segmenti: []grammatica.SegmentoBase{{Nome: "disegno", Pattern: pattern, Identitario: true}},
			Maiuscole: grammatica.MaiuscoleEsatte, Normalizza: grammatica.NormalizzaNessuna,
			ConfinePrima: grammatica.ConfineAlnumASCII, ConfineDopo: grammatica.ConfineAlnumASCII}
	}
	famiglie := []grammatica.FamigliaCodice{{
		ID: "beta-dis", Namespace: "beta-dis", Ruoli: []grammatica.Ruolo{grammatica.RuoloComponente},
		Base: baseBeta("[0-9]{6}"),
		Forme: []grammatica.FormaCodice{{ID: "ovunque", Selettori: selPartner, Stato: grammatica.StatoAttiva, Completa: true,
			Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}}},
		Esempi: []grammatica.EsempioCodice{{ID: "e-nome", Origine: grammatica.OrigineSintetico, Selettore: "nome_file", Testo: "654321.pdf",
			Atteso: grammatica.AttesoEsempio{Letture: []grammatica.LetturaAttesa{{Forma: "ovunque", Base: "654321"}}}}},
	}}
	if betaExtra {
		famiglie = append(famiglie, grammatica.FamigliaCodice{
			ID: "beta-commessa", Namespace: "beta-commessa", Ruoli: []grammatica.Ruolo{grammatica.RuoloComponente},
			Base: baseBeta("BC[0-9]{5}"),
			Forme: []grammatica.FormaCodice{{ID: "nodo", Selettori: []string{"nodo_step.id"}, Stato: grammatica.StatoAttiva, Completa: true,
				Parti: []grammatica.Parte{{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}}}},
			Esempi: []grammatica.EsempioCodice{{ID: "e-nodo", Origine: grammatica.OrigineSintetico, Selettore: "nodo_step.id", Testo: "BC12345",
				Atteso: grammatica.AttesoEsempio{Letture: []grammatica.LetturaAttesa{{Forma: "nodo", Base: "BC12345"}}}}},
		})
	}
	beta := grammatica.Grammatica{
		VersioneSchema: grammatica.VersioneSchema,
		Emittente:      &grammatica.EmittenteGrammatica{ID: "beta", Nome: "BETA (inventato)"},
		Profilo:        grammatica.Profilo{Stato: grammatica.ProfiloCompleto},
		Famiglie:       famiglie,
	}
	rawB, err := json.Marshal(beta)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := grammatica.NuovoSnapshot(rawB, lim)
	if err != nil {
		t.Fatalf("BETA: %v", err)
	}
	acme := grammatica.Grammatica{
		VersioneSchema: grammatica.VersioneSchema,
		Cliente:        grammatica.ClienteGrammatica{ID: clienteACME, RagioneSociale: "ACME S.p.A."},
		Profilo:        grammatica.Profilo{Stato: grammatica.ProfiloParziale},
		Famiglie:       []grammatica.FamigliaCodice{famigliaCatena()},
		Partner:        []grammatica.DichiarazionePartner{{Emittente: "beta", HashSnapshot: sb.Hash, Selettori: selPartner}},
		Legami:         legamiDiProva(),
	}
	rawA, err := json.Marshal(acme)
	if err != nil {
		t.Fatal(err)
	}
	ind := grammatica.IndiceRegole{VersioneIndice: grammatica.VersioneIndice, Limiti: lim,
		Grammatiche: []grammatica.VoceIndice{{ClienteID: clienteACME, File: "acme.v1.json", Sha256: shaDi(rawA)}},
		Emittenti:   []grammatica.VoceEmittente{{ID: "beta", File: "beta.v1.json", Sha256: shaDi(rawB)}}}
	r, d, err := motorea.CompilaInsieme(ind, map[string][]byte{"acme.v1.json": rawA, "beta.v1.json": rawB})
	if err != nil || r.Motori[clienteACME] == nil || r.Emittenti["beta"] == nil || len(r.Motori[clienteACME].PartnerAmmessi()) != 1 {
		t.Fatalf("insieme con i legami: %v, motori %d, emittenti %d\n%+v", err, len(r.Motori), len(r.Emittenti), d)
	}
	for _, x := range d {
		if strings.HasPrefix(x.Codice, "legame") || strings.Contains(x.Codice, ".legame") {
			t.Fatalf("un legame non entra nel motore: %+v", x)
		}
	}
	r.Contesti = contesti
	return &r, sb.Hash
}

// contestoDeiLegami: il contesto dichiarato del thread con BETA attivo, i due modelli e il perimetro dato.
func contestoDeiLegami(hash string, perimetro ...motorea.OccorrenzaContesto) motorea.ContestoLavorazione {
	c := contestoACME(threadACME, hash, perimetro...)
	c.Modelli = []string{"assieme-radice", "assieme"}
	return c
}

// scenaLegame: la BOM di lavoro del finito 7120100A da uno STEP con la radice e il nome accoppiati (codice del cliente
// e disegno del fornitore) e due figli con i disegni del fornitore.
func scenaLegame(t *testing.T) fotorfq.Thread {
	t.Helper()
	th := scenaBOM(t, []nodoF{{"#1", idRadiceLegame, ""}, {"#2", "654302", ""}, {"#3", "654303", ""}}, []arcoF{{"#1", "#2", 1}, {"#1", "#3", 2}})
	rinomina(&th, aStepB, nomeStepLegame)
	return th
}

// rinomina: il nome ricevuto di un allegato, ovunque la fotografia lo ripete (l'allegato, il documento confermato, le
// righe legacy).
func rinomina(th *fotorfq.Thread, a uuid.UUID, nome string) {
	for i := range th.Allegati {
		if th.Allegati[i].ID == a {
			th.Allegati[i].NomeFile = nome
		}
	}
	for i := range th.Documenti {
		for _, x := range th.Documenti[i].Allegati {
			if x == a {
				th.Documenti[i].NomeFile = nome
			}
		}
	}
	for i := range th.RigheComponenteProposta {
		if th.RigheComponenteProposta[i].AllegatoID == a {
			th.RigheComponenteProposta[i].NomeFile = nome
		}
	}
	for i := range th.RigheRelazioneProposta {
		if th.RigheRelazioneProposta[i].AllegatoID == a {
			th.RigheRelazioneProposta[i].NomeFile = nome
		}
	}
}

// letturaJSON: i campi di una lettura del registro che le prove guardano.
type letturaJSON struct {
	ChiaveLettura  string `json:"chiave_lettura"`
	UnitaID        string `json:"unita_id"`
	Emittente      string `json:"emittente"`
	HashGrammatica string `json:"hash_grammatica"`
	Namespace      string `json:"namespace"`
	Famiglia       string `json:"famiglia"`
	Letterale      string `json:"letterale"`
	Ruolo          string `json:"ruolo"`
	Base           struct {
		Normalizzata string `json:"normalizzata"`
	} `json:"base"`
}

func letturaDa(t *testing.T, raw json.RawMessage) letturaJSON {
	t.Helper()
	var l letturaJSON
	if err := json.Unmarshal(raw, &l); err != nil {
		t.Fatalf("non è una lettura: %s", raw)
	}
	return l
}

// riferimentiDi: i riferimenti di una voce, con la lettura (se c'è) già letta.
type riferimentoJSON struct {
	Tipo        string          `json:"tipo"`
	Ruolo       string          `json:"ruolo"`
	Fonte       string          `json:"fonte"`
	Lettura     json.RawMessage `json:"lettura"`
	Equivalente json.RawMessage `json:"equivalente"`
}

func riferimentiDi(t *testing.T, v importazione.Voce) []riferimentoJSON {
	t.Helper()
	var rr []riferimentoJSON
	if len(v.Riferimenti) == 0 {
		return nil
	}
	if err := json.Unmarshal(v.Riferimenti, &rr); err != nil {
		t.Fatalf("riferimenti di %s: %s", v.Voce, v.Riferimenti)
	}
	return rr
}

// lettureDelFile: le chiavi delle letture di un file del registro.
func lettureDelFile(t *testing.T, k importazione.Contenuto, a uuid.UUID) map[string]letturaJSON {
	t.Helper()
	out := map[string]letturaJSON{}
	for _, f := range k.Proposta.File {
		if f.AllegatoID != a.String() {
			continue
		}
		var ll []json.RawMessage
		_ = json.Unmarshal(f.Letture, &ll)
		for _, x := range ll {
			l := letturaDa(t, x)
			out[l.ChiaveLettura] = l
		}
	}
	return out
}

// TestA04A07IlLegameDellaRadice (A-04, A-07; EM §3.8, GP-1 §5; PM23 §10 A-07 con tutti i campi): sulla scena dell'assieme
// del fornitore (sintetica) con il contesto (BETA attivo, i due modelli, lo STEP nel perimetro):
//   - la radice ha l'identità del cliente (la primaria del legame) e il disegno del fornitore fra i riferimenti, con il
//     ruolo, l'emittente partner e l'hash di BETA: mai promosso a identità;
//   - i figli hanno l'identità del partner, con l'emittente «partner:beta» e l'hash, e il codice di partenza letto;
//   - le relazioni documentato_da: quella dell'id della radice agganciata alla voce della radice (sorgente nodo), quella
//     del nome del file agganciata al file (sorgente file); stato «proposta» copiato dal motore, modello, riconoscimento,
//     entità e unità della prova; i due estremi sono chiavi di letture che stanno nel registro, byte per byte.
func TestA04A07IlLegameDellaRadice(t *testing.T) {
	_, hash := insiemeConLegami(t, false)
	r, _ := insiemeConLegami(t, false, contestoDeiLegami(hash, motorea.OccorrenzaContesto{AllegatoID: aStepB.String()}))
	c := calcolaPerIlRegistro(t, scenaLegame(t), r)
	k := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	if k.Regole.Partner == nil || len(*k.Regole.Partner) != 1 || (*k.Regole.Partner)[0] != (importazione.PartnerAttivo{Emittente: "partner:beta", Hash: hash}) {
		t.Fatalf("partner attivi: %+v", k.Regole.Partner)
	}
	cliente := "cliente:" + clienteACME.String()

	// A-04: la radice
	radice := voceDelRegistro(t, k, nodoB("#1"))
	id := letturaDa(t, radice.IdentitaProposta)
	if id.Emittente != cliente || id.Famiglia != "acme-catena" || id.Letterale != "7120100A" || id.Ruolo != "" || radice.CodiceProposto != "7120100A" {
		t.Fatalf("l'identità della radice: %+v, codice %q", id, radice.CodiceProposto)
	}
	var rifBeta *letturaJSON
	for _, x := range riferimentiDi(t, radice) {
		if x.Tipo != "lettura_ruolo" {
			continue
		}
		l := letturaDa(t, x.Lettura)
		if x.Ruolo != "riferimento" || l.Ruolo != "riferimento" || l.Emittente != "partner:beta" || l.HashGrammatica != hash || l.Famiglia != "beta-dis" ||
			l.Base.Normalizzata != "654301" || x.Fonte != "id" {
			t.Errorf("il riferimento del fornitore sulla radice: %+v (%s, %s)", l, x.Ruolo, x.Fonte)
		}
		rifBeta = &l
	}
	if rifBeta == nil {
		t.Fatalf("la radice non ha il riferimento del fornitore: %s", radice.Riferimenti)
	}
	if len(radice.Alternative) != 0 {
		t.Errorf("con una lettura scelta e un riferimento, nessuna alternativa: %s", radice.Alternative)
	}
	// A-04: i figli
	for _, x := range []struct{ chiave, codice string }{{"#2", "654302"}, {"#3", "654303"}} {
		v := voceDelRegistro(t, k, nodoB(x.chiave))
		l := letturaDa(t, v.IdentitaProposta)
		if l.Emittente != "partner:beta" || l.HashGrammatica != hash || l.Namespace != "beta-dis" || l.Letterale != x.codice || v.CodiceProposto != x.codice {
			t.Errorf("%s: l'identità del partner %+v, codice %q", x.chiave, l, v.CodiceProposto)
		}
	}

	// A-07: le relazioni
	if len(k.Proposta.Relazioni) != 2 {
		t.Fatalf("relazioni %d: attese quella della radice e quella del nome\n%+v", len(k.Proposta.Relazioni), k.Proposta.Relazioni)
	}
	var entitaRadice string
	_ = json.Unmarshal(radice.EntitaID, &entitaRadice)
	letture := lettureDelFile(t, k, aStepB)
	visti := map[string]bool{}
	for _, rel := range k.Proposta.Relazioni {
		var bersagli, evidenze []string
		_ = json.Unmarshal(rel.LettureBersaglio, &bersagli)
		_ = json.Unmarshal(rel.Evidenze, &evidenze)
		if testoJSON(t, rel.Tipo) != motorea.RelazioneDocumentatoDa || testoJSON(t, rel.Stato) != motorea.StatoRelazioneProposta || len(bersagli) != 1 ||
			len(evidenze) != 1 || testoJSON(t, rel.LetturaSorgente) == "" {
			t.Errorf("una relazione senza i suoi campi: %+v", rel)
			continue
		}
		modello := testoJSON(t, rel.Modello)
		visti[modello] = true
		if testoJSON(t, rel.Riconoscimento) != "legame/"+modello || !strings.HasPrefix(testoJSON(t, rel.Voce), "r:"+aStepB.String()+":") {
			t.Errorf("riconoscimento e voce della relazione %s: %s %s", modello, rel.Riconoscimento, rel.Voce)
		}
		switch modello {
		case "assieme-radice":
			if rel.Sorgente.Tipo != "nodo" || testoJSON(t, rel.Sorgente.Voce) != radice.Voce || testoJSON(t, rel.EntitaSorgente) != entitaRadice {
				t.Errorf("la relazione dell'id della radice non è agganciata alla voce della radice: %+v", rel)
			}
			if testoJSON(t, rel.LetturaSorgente) != id.ChiaveLettura || bersagli[0] != rifBeta.ChiaveLettura || evidenze[0] != id.UnitaID {
				t.Errorf("gli estremi della relazione della radice: %s → %v (attesi %s → %s), prova %v", rel.LetturaSorgente, bersagli,
					id.ChiaveLettura, rifBeta.ChiaveLettura, evidenze)
			}
		case "assieme":
			if rel.Sorgente.Tipo != "file" || testoJSON(t, rel.Sorgente.Voce) != "f:"+aStepB.String() {
				t.Errorf("la relazione del nome non è agganciata al file: %+v", rel)
			}
			s, okS := letture[testoJSON(t, rel.LetturaSorgente)]
			b, okB := letture[bersagli[0]]
			if !okS || !okB || s.Emittente != cliente || b.Emittente != "partner:beta" || b.Ruolo != "riferimento" || s.UnitaID != evidenze[0] {
				t.Errorf("gli estremi della relazione del nome non stanno fra le letture del file: %+v %+v (%v %v)", s, b, okS, okB)
			}
		default:
			t.Errorf("un modello inatteso: %s", modello)
		}
	}
	if !visti["assieme-radice"] || !visti["assieme"] {
		t.Errorf("le relazioni dei due modelli: %v", visti)
	}
}

// TestA08LoStessoShaInDueAllegati (A-08; EM §3.8.2; PM23 §10, riga 3): due allegati con lo stesso sha, il contesto su
// uno solo. Due gruppi (la BOM di lavoro sull'allegato del perimetro, la candidata sull'altro), le stesse chiavi PRODUCT,
// gli allegati diversi nell'origine; nell'allegato fuori dal perimetro BETA non legge (i figli senza identità, la radice
// senza riferimento e senza relazioni) e il suo file non ha contesto_id: nessuna eredità dall'altra occorrenza.
func TestA08LoStessoShaInDueAllegati(t *testing.T) {
	_, hash := insiemeConLegami(t, false)
	r, _ := insiemeConLegami(t, false, contestoDeiLegami(hash, motorea.OccorrenzaContesto{AllegatoID: aStepB.String()}))
	th := scenaLegame(t)
	conAllegato(&th, allegato(aStepGemello, 6, "copia-assieme.stp", "stp", shaStepB), nil)
	c := calcolaPerIlRegistro(t, th, r)
	pv := prodottoDi(t, c.et, rifProdB)
	var copia string
	for _, g := range propostabom.GruppiDelProdotto(pv, c.et.Ancoraggi.Strutture) {
		if g.AllegatoID != nil && *g.AllegatoID == aStepGemello {
			copia = g.Chiave
		}
	}
	if copia == "" {
		t.Fatalf("la copia non ha un gruppo del prodotto: %+v", propostabom.GruppiDelProdotto(pv, c.et.Ancoraggi.Strutture))
	}
	dentro := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	fuori := c.importa(t, rifProdB, copia, nil).Contenuto
	if fuori.Gruppo.AllegatoID != aStepGemello.String() || dentro.Gruppo.AllegatoID != aStepB.String() || fuori.Gruppo.Sha256 != dentro.Gruppo.Sha256 {
		t.Fatalf("i due gruppi: %+v / %+v", dentro.Gruppo, fuori.Gruppo)
	}
	for _, ch := range []string{"#1", "#2", "#3"} {
		a, b := voceDelRegistro(t, dentro, nodoB(ch)), voceDelRegistro(t, fuori, nodoB(ch))
		if a.Origine.Chiave != b.Origine.Chiave || a.Origine.AllegatoID == b.Origine.AllegatoID || a.Voce == b.Voce {
			t.Errorf("%s: origine %+v / %+v", ch, a.Origine, b.Origine)
		}
	}
	for _, ch := range []string{"#2", "#3"} {
		if v := voceDelRegistro(t, fuori, nodoB(ch)); len(v.IdentitaProposta) != 0 || v.CodiceProposto != "" ||
			testoJSON(t, v.MotivoProposto) != ancoraggio.MotivoLetturaAssente {
			t.Errorf("%s fuori dal perimetro: identità %s, codice %q, motivo %s", ch, v.IdentitaProposta, v.CodiceProposto, v.MotivoProposto)
		}
		if v := voceDelRegistro(t, dentro, nodoB(ch)); letturaDa(t, v.IdentitaProposta).Emittente != "partner:beta" {
			t.Errorf("%s nel perimetro senza l'identità del partner: %s", ch, v.IdentitaProposta)
		}
	}
	rf := voceDelRegistro(t, fuori, nodoB("#1"))
	if strings.Contains(string(mustJSON(t, fuori.Proposta.Nodi)), "partner:beta") {
		t.Errorf("fuori dal perimetro una lettura del partner nelle voci: %s", rf.Riferimenti)
	}
	// le relazioni dello STEP del perimetro, che l'ancoraggio dà come candidato della radice della copia (stesso
	// contenuto), stanno fra le evidenze come relazioni di quel file (sorgente file), mai sui nodi o sul file della copia
	for _, rel := range fuori.Proposta.Relazioni {
		if rel.Sorgente.Tipo != "file" || testoJSON(t, rel.Sorgente.Voce) != "f:"+aStepB.String() {
			t.Errorf("una relazione nel registro della copia non è dell'altra occorrenza: %+v", rel)
		}
	}
	if letturaDa(t, rf.IdentitaProposta).Emittente != "cliente:"+clienteACME.String() {
		t.Errorf("la radice della copia senza l'identità del cliente: %s", rf.IdentitaProposta)
	}
	for _, x := range fuori.Proposta.File {
		if x.AllegatoID == aStepGemello.String() && (len(campoDi(t, x.Interpretazione, "contesto_id")) != 0 || len(campoDi(t, x.Interpretazione, "impronta_contesto")) != 0) {
			t.Errorf("la copia fuori dal perimetro ha il contesto: %s", x.Interpretazione)
		}
	}
	if fuori.ImprontaContenuto == dentro.ImprontaContenuto {
		t.Error("le due occorrenze danno lo stesso contenuto: le letture non sono diverse")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fattiDXF: i fatti di un DXF con il testo del worker (testo_dxf v1) e i campi del cartiglio dati, ognuno
// {etichetta, valore}, come li scrive workers/dxf_testo.py.
func fattiDXF(t *testing.T, sha string, campi ...[2]string) fotorfq.Fatti {
	t.Helper()
	cs := []map[string]any{}
	for _, c := range campi {
		cs = append(cs, map[string]any{"etichetta": c[0], "letta": strings.ToUpper(c[0]), "valore": c[1], "pagina": 1, "zona": "pagina",
			"fonte": "nativo", "riquadro": []float64{-145.73, 1.75, -122.63, 5.25}, "confidenza": nil})
	}
	raw, err := json.Marshal(map[string]any{"testo_dxf": map[string]any{"versione": 1, "esito": "letto", "motivo": "", "versione_dxf": "AC1014",
		"codifica": "cp1252", "testi": 40, "annidati": 0, "troncato": false, "cartiglio": cs,
		"limiti": map[string]any{"byte_max": 67108864, "testi_max": 5000, "testo_max": 512, "campi_max": 24}}})
	if err != nil {
		t.Fatal(err)
	}
	return fatti(t, sha, string(raw), testo(""))
}

// insiemeK4: ACME con la famiglia della catena più la forma del riferimento 3D (cartiglio.riferimento_3d, base e
// marcatore: lo stesso namespace dei nodi dello STEP) e una famiglia del codice della parte (PN e cinque cifre, sul
// codice del cartiglio): la coppia K4 vuole due namespace diversi.
func insiemeK4(t *testing.T) *motorea.InsiemeRegole {
	t.Helper()
	f := famigliaCatena()
	f.Forme = append(f.Forme, formaCatena("cartiglio-3d", []string{"cartiglio.riferimento_3d"}, parteC(grammatica.TipoParteBase), parteC(grammatica.TipoParteMarcatore, "A", "B")))
	f.Esempi = append(f.Esempi, grammatica.EsempioCodice{ID: "e-3d", Origine: grammatica.OrigineSintetico, Selettore: "cartiglio.riferimento_3d", Testo: "7120100A",
		Atteso: grammatica.AttesoEsempio{Letture: []grammatica.LetturaAttesa{{Forma: "cartiglio-3d", Base: "7120100", Marcatore: "A"}}}})
	pn := grammatica.FamigliaCodice{
		ID: "acme-pn", Namespace: "acme-pn", Ruoli: []grammatica.Ruolo{grammatica.RuoloComponente},
		Base: grammatica.Base{Segmenti: []grammatica.SegmentoBase{{Nome: "pn", Pattern: "PN[0-9]{5}", Identitario: true}},
			Maiuscole: grammatica.MaiuscoleEsatte, Normalizza: grammatica.NormalizzaNessuna,
			ConfinePrima: grammatica.ConfineAlnumASCII, ConfineDopo: grammatica.ConfineAlnumASCII},
		Forme: []grammatica.FormaCodice{formaCatena("cartiglio-pn", []string{"cartiglio.codice"}, parteC(grammatica.TipoParteBase))},
		Esempi: []grammatica.EsempioCodice{{ID: "e-pn", Origine: grammatica.OrigineSintetico, Selettore: "cartiglio.codice", Testo: "PN12345",
			Atteso: grammatica.AttesoEsempio{Letture: []grammatica.LetturaAttesa{{Forma: "cartiglio-pn", Base: "PN12345"}}}}},
	}
	r := insiemeDi(t, voceRegole{cliente: clienteACME, file: "acme.v1.json", byte: grammaticaDi(t, clienteACME, "ACME S.p.A.", f, pn)})
	if r.Motori[clienteACME] == nil {
		t.Fatalf("la grammatica K4 non compila: %+v", r.Scartati)
	}
	return r
}

// TestA06LEquivalenteK4 (A-06; EM §3.8, riferimenti[] equivalente_k4): un DXF il cui cartiglio scrive il codice della
// parte (PN93000) e, nella stessa entità, il documento 3D del nodo #3 (7120300A). Nella voce di #3 l'equivalente è un
// riferimento, con namespace, base, le prove e l'emittente ricavato dalla lettura del codice nelle prove; l'identità
// proposta e il codice di partenza restano quelli letti nello STEP (mai promosso). La lettura citata dalla prova sta fra
// le letture del file del DXF.
func TestA06LEquivalenteK4(t *testing.T) {
	th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}}, []arcoF{{"#1", "#2", 1}, {"#1", "#3", 1}})
	dxf := fattiDXF(t, shaDxfK4, [2]string{"riferimento_3d", "7120300A"}, [2]string{"codice", "PN93000"})
	conAllegato(&th, allegato(aDxfK4, 2, "disegno-k4.dxf", "dxf", shaDxfK4), &dxf)
	c := calcolaPerIlRegistro(t, th, insiemeK4(t))
	k := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	v := voceDelRegistro(t, k, nodoB("#3"))
	if l := letturaDa(t, v.IdentitaProposta); l.Famiglia != "acme-catena" || l.Letterale != "7120300A" || v.CodiceProposto != "7120300A" {
		t.Fatalf("l'identità di #3 non è quella dello STEP: %+v, codice %q", l, v.CodiceProposto)
	}
	var eq struct {
		Namespace string `json:"namespace"`
		Base      string `json:"base"`
		Emittente string `json:"emittente"`
		Prove     []struct {
			AllegatoID    string `json:"allegato_id"`
			LetturaCodice string `json:"lettura_codice"`
			Fonte         string `json:"fonte"`
		} `json:"prove"`
	}
	trovato := false
	for _, x := range riferimentiDi(t, v) {
		if x.Tipo != "equivalente_k4" {
			continue
		}
		trovato = true
		if err := json.Unmarshal(x.Equivalente, &eq); err != nil {
			t.Fatal(err)
		}
		if x.Fonte != ancoraggio.FonteEquivalenzaCartiglio {
			t.Errorf("la fonte dell'equivalente: %q", x.Fonte)
		}
	}
	if !trovato {
		t.Fatalf("#3 senza l'equivalente K4: %s", v.Riferimenti)
	}
	if eq.Namespace != "acme-pn" || eq.Base != "PN93000" || eq.Emittente != "cliente:"+clienteACME.String() || len(eq.Prove) != 1 ||
		eq.Prove[0].AllegatoID != aDxfK4.String() || eq.Prove[0].Fonte != ancoraggio.FonteEquivalenzaCartiglio {
		t.Fatalf("l'equivalente: %+v", eq)
	}
	if _, ok := lettureDelFile(t, k, aDxfK4)[aDxfK4.String()+"/"+eq.Prove[0].LetturaCodice]; !ok {
		t.Errorf("la lettura del codice della parte citata dalla prova non sta fra le letture del DXF")
	}
	if strings.Contains(string(v.IdentitaProposta), "PN93000") {
		t.Error("l'equivalente è diventato l'identità proposta")
	}
	for _, ch := range []string{"#1", "#2"} {
		if strings.Contains(string(voceDelRegistro(t, k, nodoB(ch)).Riferimenti), "equivalente_k4") {
			t.Errorf("%s ha un equivalente che non è suo", ch)
		}
	}
}

// TestA18IlConfrontoFraDueRegistri (A-18 esteso; EM §6.4, §6.5, §9.1; PE-18): Confronta sui registri dell'adattatore.
//   - lo stesso ingresso: uguale, differenze vuote;
//   - il contesto ridichiarato senza effetto sulle letture delle voci: regole_cambiate, differenze vuote, evidenze
//     cambiate (l'impronta del contesto dei file del perimetro);
//   - una grammatica cambiata che non cambia nessuna lettura (una famiglia nuova di BETA): oggi proposta_cambiata su ogni
//     voce letta, con il solo hash_grammatica diverso (vedi la proposta di emendamento nel resoconto della corsia);
//   - un'identità cambiata (l'id di un figlio nei fatti): proposta_cambiata, il nodo cambiato con i campi;
//   - un nodo sparito: il nodo e il suo arco tolti;
//   - un segnale cambiato (il grafo diventa non completo: il contro-indizio nei segnali dei cordoni) e i campi del gruppo;
//   - un campo che viene dallo stato della RFQ sulla voce della radice (la riga della radice decisa con il prodotto) non è
//     una differenza: uguale, solo le evidenze cambiate;
//   - (revisione della corsia, ondata B) la quantità di un arco cambiata (differenze.archi[], il campo quantita); un nodo
//     che passa fra le voci e i fuori per decisione, tolto (stato_motore.tolto) o non raggiunto (stato_motore.raggiunto);
//     una relazione con sorgente nodo tolta o cambiata (il campo «relazioni» della voce della sorgente).
func TestA18IlConfrontoFraDueRegistri(t *testing.T) {
	t.Run("uguale", func(t *testing.T) {
		c := calcolaPerIlRegistro(t, scenaCollegamento(t), insiemeACME(t))
		a := c.importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		x := propostabom.Confronta(a, a)
		if x.Esito != propostabom.EsitoUguale || !x.Differenze.Vuote() || x.EvidenzeCambiate || x.ImprontaContenutoCorrente != a.ImprontaContenuto {
			t.Errorf("lo stesso registro: %+v", x)
		}
	})
	t.Run("regole cambiate, proposta uguale", func(t *testing.T) {
		// il contesto ridichiarato (un altro giorno: cambia la sua impronta) senza effetto sulle letture delle voci
		_, h := insiemeConLegami(t, false)
		ctx := contestoDeiLegami(h, motorea.OccorrenzaContesto{AllegatoID: aStepB.String()})
		r1, _ := insiemeConLegami(t, false, ctx)
		ctx.Provenienza.Data = "2026-10-19"
		r2, _ := insiemeConLegami(t, false, ctx)
		vecchio := calcolaPerIlRegistro(t, scenaCollegamento(t), r1).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		nuovo := calcolaPerIlRegistro(t, scenaCollegamento(t), r2).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		x := propostabom.Confronta(vecchio, nuovo)
		if x.Esito != propostabom.EsitoRegoleCambiate || !x.Differenze.Vuote() || vecchio.ImprontaContenuto != nuovo.ImprontaContenuto ||
			vecchio.ImprontaRegole == nuovo.ImprontaRegole || !x.EvidenzeCambiate {
			t.Errorf("il contesto ridichiarato: %+v", x)
		}
	})
	t.Run("una grammatica cambiata cambia l'hash di ogni lettura (proposta di emendamento)", func(t *testing.T) {
		// Oggi (EM §3.8: hash_grammatica sta nella lettura, e la lettura nel contenuto) una grammatica nuova che non
		// cambia nessuna lettura cambia comunque ogni identità proposta letta con quella grammatica: qui BETA cambia, ACME
		// ne blocca l'hash (DichiarazionePartner) e quindi cambia anche l'hash di ACME. La sola differenza delle letture è
		// hash_grammatica: la prova lo fissa, finché l'emendamento proposto nel resoconto non decide altrimenti.
		_, h1 := insiemeConLegami(t, false)
		r1, _ := insiemeConLegami(t, false, contestoDeiLegami(h1, motorea.OccorrenzaContesto{AllegatoID: aStepB.String()}))
		_, h2 := insiemeConLegami(t, true)
		r2, _ := insiemeConLegami(t, true, contestoDeiLegami(h2, motorea.OccorrenzaContesto{AllegatoID: aStepB.String()}))
		if h1 == h2 {
			t.Fatal("la grammatica di BETA non è cambiata")
		}
		vecchio := calcolaPerIlRegistro(t, scenaCollegamento(t), r1).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		nuovo := calcolaPerIlRegistro(t, scenaCollegamento(t), r2).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		x := propostabom.Confronta(vecchio, nuovo)
		if x.Esito != propostabom.EsitoPropostaCambiata || len(x.Differenze.Nodi) != len(nuovo.Proposta.Nodi) {
			t.Fatalf("BETA cambiato: %+v", x)
		}
		for i := range vecchio.Proposta.Nodi {
			var a, b map[string]any
			_ = json.Unmarshal(vecchio.Proposta.Nodi[i].IdentitaProposta, &a)
			_ = json.Unmarshal(nuovo.Proposta.Nodi[i].IdentitaProposta, &b)
			if a["hash_grammatica"] == b["hash_grammatica"] {
				t.Errorf("%s: l'hash della lettura non è cambiato", vecchio.Proposta.Nodi[i].Voce)
			}
			delete(a, "hash_grammatica")
			delete(b, "hash_grammatica")
			if string(mustJSON(t, a)) != string(mustJSON(t, b)) {
				t.Errorf("%s: la lettura cambia oltre l'hash: %s / %s", vecchio.Proposta.Nodi[i].Voce, mustJSON(t, a), mustJSON(t, b))
			}
		}
	})
	t.Run("un'identità cambiata e un nodo sparito", func(t *testing.T) {
		nodi := []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}, {"#4", "7120400A", ""}}
		archi := []arcoF{{"#1", "#2", 1}, {"#1", "#3", 1}, {"#1", "#4", 1}}
		vecchio := calcolaPerIlRegistro(t, scenaBOM(t, nodi, archi), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		nodi2 := []nodoF{{"#1", "7120100A", ""}, {"#2", "7120250A", ""}, {"#3", "7120300A", ""}}
		nuovo := calcolaPerIlRegistro(t, scenaBOM(t, nodi2, archi[:2]), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		x := propostabom.Confronta(vecchio, nuovo)
		if x.Esito != propostabom.EsitoPropostaCambiata {
			t.Fatalf("esito %s", x.Esito)
		}
		perVoce := map[string]propostabom.DifferenzaNodo{}
		for _, d := range x.Differenze.Nodi {
			perVoce[d.Voce] = d
		}
		d2 := perVoce[vocePerProva(aStepB, nodoB("#2"))]
		if d2.Tipo != propostabom.DifferenzaCambiato || !contieneTesto(d2.Campi, "identita_proposta") || !contieneTesto(d2.Campi, "codice_proposto") {
			t.Errorf("l'identità cambiata di #2: %+v", d2)
		}
		if d4 := perVoce[vocePerProva(aStepB, nodoB("#4"))]; d4.Tipo != propostabom.DifferenzaTolto {
			t.Errorf("#4 sparito: %+v", d4)
		}
		if _, ok := perVoce[vocePerProva(aStepB, nodoB("#3"))]; ok {
			t.Errorf("#3 non cambia, ma è fra le differenze: %+v", perVoce)
		}
		tolto := false
		for _, a := range x.Differenze.Archi {
			tolto = tolto || (a.Figlio == nodoB("#4") && a.Tipo == propostabom.DifferenzaTolto)
		}
		if !tolto {
			t.Errorf("l'arco verso #4 non è tolto: %+v", x.Differenze.Archi)
		}
	})
	t.Run("un segnale cambiato", func(t *testing.T) {
		th := scenaCordoni(t, "Saldatura")
		vecchio := calcolaPerIlRegistro(t, th, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		f := th.Fatti[shaStepB]
		f.MotivoParziale = testo("2 radici nel file: una distinta ne ha una")
		th.Fatti[shaStepB] = f
		nuovo := calcolaPerIlRegistro(t, th, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		x := propostabom.Confronta(vecchio, nuovo)
		if x.Esito != propostabom.EsitoPropostaCambiata || !contieneTesto(x.Differenze.Gruppo, "grafo_completo") {
			t.Fatalf("il grafo non completo: %+v", x)
		}
		segnali := 0
		for _, d := range x.Differenze.Nodi {
			if contieneTesto(d.Campi, "segnali") {
				segnali++
			}
		}
		if segnali != 3 {
			t.Errorf("i cordoni con il segnale cambiato: %d (%+v)", segnali, x.Differenze.Nodi)
		}
	})
	t.Run("la radice decisa non è una differenza", func(t *testing.T) {
		th := scenaCollegamento(t)
		vecchio := calcolaPerIlRegistro(t, th, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		op := operatore
		decidi(t, &th, "#1", "duplicato", cProdotto, &op)
		nuovo := calcolaPerIlRegistro(t, th, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		x := propostabom.Confronta(vecchio, nuovo)
		if x.Esito != propostabom.EsitoUguale || !x.Differenze.Vuote() || !x.EvidenzeCambiate {
			t.Errorf("la riga della radice decisa: %+v", x)
		}
	})
	// i casi della revisione della corsia (ondata B, secondo passaggio): le differenze che EM §6.5 e PE-18 chiedono
	nodi3 := []nodoF{{"#1", "7120100A", ""}, {"#2", "7120200A", ""}, {"#3", "7120300A", ""}}
	archi3 := []arcoF{{"#1", "#2", 1}, {"#1", "#3", 1}}
	t.Run("la quantità di un arco cambiata", func(t *testing.T) {
		// EM §6.5: «arco con la quantità cambiata nei fatti: si elenca in differenze.archi[]»
		a := calcolaPerIlRegistro(t, scenaBOM(t, nodi3, archi3), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		b := calcolaPerIlRegistro(t, scenaBOM(t, nodi3, []arcoF{{"#1", "#2", 3}, {"#1", "#3", 1}}), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		x := propostabom.Confronta(a, b)
		if x.Esito != propostabom.EsitoPropostaCambiata || len(x.Differenze.Archi) != 1 || x.Differenze.Archi[0].Figlio != nodoB("#2") ||
			x.Differenze.Archi[0].Tipo != propostabom.DifferenzaCambiato || !contieneTesto(x.Differenze.Archi[0].Campi, "quantita") || len(x.Differenze.Nodi) != 0 {
			t.Errorf("la quantità cambiata: %s %+v", x.Esito, x.Differenze)
		}
	})
	t.Run("un nodo che passa fra le voci e i fuori per decisione", func(t *testing.T) {
		// DifferenzaNodo: fuori come tolto cambia stato_motore.tolto, fuori come non raggiunto stato_motore.raggiunto
		a := calcolaPerIlRegistro(t, scenaBOM(t, nodi3, archi3), insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		op := operatore
		tolto := scenaBOM(t, nodi3, archi3)
		decidi(t, &tolto, "#3", "scartata", uuid.Nil, &op)
		b := calcolaPerIlRegistro(t, tolto, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		x := propostabom.Confronta(a, b)
		if len(x.Differenze.Nodi) != 1 || x.Differenze.Nodi[0].Voce != vocePerProva(aStepB, nodoB("#3")) || x.Differenze.Nodi[0].Tipo != propostabom.DifferenzaCambiato ||
			!contieneTesto(x.Differenze.Nodi[0].Campi, "stato_motore.tolto") || x.Esito != propostabom.EsitoPropostaCambiata {
			t.Errorf("#3 tolto da una persona: %s %+v", x.Esito, x.Differenze)
		}
		staccato := scenaBOM(t, nodi3, archi3)
		r := arcoDi(t, &staccato, "#1", "#3")
		r.Stato, r.DecisoDa = "scartata", &op
		c := calcolaPerIlRegistro(t, staccato, insiemeACME(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		y := propostabom.Confronta(a, c)
		if len(y.Differenze.Nodi) != 1 || y.Differenze.Nodi[0].Voce != vocePerProva(aStepB, nodoB("#3")) ||
			!contieneTesto(y.Differenze.Nodi[0].Campi, "stato_motore.raggiunto") || y.Esito != propostabom.EsitoPropostaCambiata {
			t.Errorf("#3 non raggiunto per l'arco tolto: %s %+v", y.Esito, y.Differenze)
		}
	})
	t.Run("una relazione con sorgente nodo tolta o cambiata", func(t *testing.T) {
		// le relazioni con sorgente nodo stanno in impronta_contenuto: la differenza è il campo «relazioni» della voce della
		// sorgente. Il registro nuovo si ricompone con Componi dal contenuto vero, con la relazione tolta o con un altro stato.
		_, hash := insiemeConLegami(t, false)
		r, _ := insiemeConLegami(t, false, contestoDeiLegami(hash, motorea.OccorrenzaContesto{AllegatoID: aStepB.String()}))
		a := calcolaPerIlRegistro(t, scenaLegame(t), r).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
		var conNodo int = -1
		for i, rel := range a.Proposta.Relazioni {
			if rel.Sorgente.Tipo == "nodo" {
				conNodo = i
			}
		}
		if conNodo < 0 {
			t.Fatal("la scena del legame non ha una relazione con sorgente nodo")
		}
		voce := testoJSON(t, a.Proposta.Relazioni[conNodo].Sorgente.Voce)
		ricomponi := func(t *testing.T, cambia func([]importazione.Relazione) []importazione.Relazione) importazione.Contenuto {
			t.Helper()
			c := a
			c.Proposta.Relazioni = cambia(append([]importazione.Relazione(nil), a.Proposta.Relazioni...))
			reg, err := importazione.Componi(c)
			if err != nil {
				t.Fatal(err)
			}
			return reg.Contenuto
		}
		for nome, b := range map[string]importazione.Contenuto{
			"tolta": ricomponi(t, func(rr []importazione.Relazione) []importazione.Relazione {
				return append(rr[:conNodo:conNodo], rr[conNodo+1:]...)
			}),
			"cambiata": ricomponi(t, func(rr []importazione.Relazione) []importazione.Relazione {
				rr[conNodo].Stato = json.RawMessage(`"rifiutata"`)
				return rr
			}),
		} {
			x := propostabom.Confronta(a, b)
			if x.Esito != propostabom.EsitoPropostaCambiata || len(x.Differenze.Nodi) != 1 || x.Differenze.Nodi[0].Voce != voce ||
				!contieneTesto(x.Differenze.Nodi[0].Campi, "relazioni") {
				t.Errorf("la relazione con sorgente nodo %s: %s %+v", nome, x.Esito, x.Differenze)
			}
			if y := propostabom.Confronta(b, a); len(y.Differenze.Nodi) != 1 || !contieneTesto(y.Differenze.Nodi[0].Campi, "relazioni") {
				t.Errorf("la relazione con sorgente nodo %s, al contrario: %+v", nome, y.Differenze)
			}
		}
	})
}

func vocePerProva(a uuid.UUID, rif string) string { return "n:" + a.String() + ":" + rif }

func contieneTesto(s []string, x string) bool {
	for _, y := range s {
		if y == x {
			return true
		}
	}
	return false
}

// TestA25IlCodicePropostoDelleLetture (A-25; DECA PE-08; EM §3.8): sull'uscita vera dell'adattatore ogni lettura del
// registro con una proiezione ammissibile porta codice_proposto (e rev_proposta se c'è), uguale a quello dell'identità
// proposta di una voce con la stessa lettura: le alternative di un nodo discordante, il riferimento del fornitore sulla
// radice, le identità proposte; ogni codice rispetta CodiceAmmissibile e ogni rev è in maiuscole ASCII.
func TestA25IlCodicePropostoDelleLetture(t *testing.T) {
	// le alternative del nodo discordante (A-05): tutte e due proiettano lo stesso tratto
	th := scenaBOM(t, []nodoF{{"#1", "7120100A", ""}, {"#2", "7120300A", ""}}, []arcoF{{"#1", "#2", 1}})
	k := calcolaPerIlRegistro(t, th, insiemeConLaFamigliaAlternativa(t)).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	var alt []map[string]any
	_ = json.Unmarshal(voceDelRegistro(t, k, nodoB("#2")).Alternative, &alt)
	if len(alt) != 2 {
		t.Fatalf("le alternative: %v", alt)
	}
	for _, a := range alt {
		if a["codice_proposto"] != "7120300A" {
			t.Errorf("un'alternativa senza il codice di partenza: %v", a)
		}
	}
	// il riferimento del fornitore e le identità proposte (A-04)
	_, hash := insiemeConLegami(t, false)
	r, _ := insiemeConLegami(t, false, contestoDeiLegami(hash, motorea.OccorrenzaContesto{AllegatoID: aStepB.String()}))
	g := calcolaPerIlRegistro(t, scenaLegame(t), r).importa(t, rifProdB, propostabom.GruppoBOM, nil).Contenuto
	radice := voceDelRegistro(t, g, nodoB("#1"))
	trovato := false
	for _, x := range riferimentiDi(t, radice) {
		if x.Tipo != "lettura_ruolo" {
			continue
		}
		var l map[string]any
		_ = json.Unmarshal(x.Lettura, &l)
		trovato = true
		if l["codice_proposto"] != "654301" {
			t.Errorf("il riferimento del fornitore senza il codice di partenza: %v", l)
		}
	}
	if !trovato {
		t.Fatal("la radice senza il riferimento del fornitore")
	}
	for _, v := range g.Proposta.Nodi {
		if len(v.IdentitaProposta) == 0 {
			continue
		}
		var l map[string]any
		_ = json.Unmarshal(v.IdentitaProposta, &l)
		if l["codice_proposto"] != v.CodiceProposto || (v.RevProposta != "" && l["rev_proposta"] != v.RevProposta) {
			t.Errorf("%s: la lettura dice %v / %v, la voce %q / %q", v.Voce, l["codice_proposto"], l["rev_proposta"], v.CodiceProposto, v.RevProposta)
		}
	}
	// le regole della foglia sull'uscita vera: la rilettura stretta le controlla tutte (importa), qui i valori
	for _, x := range g.Proposta.File {
		var ll []map[string]any
		_ = json.Unmarshal(x.Letture, &ll)
		for _, l := range ll {
			if c, ok := l["codice_proposto"].(string); ok && !importazione.CodiceAmmissibile(c) {
				t.Errorf("un codice di partenza non ammissibile: %v", l)
			}
			if rv, ok := l["rev_proposta"].(string); ok && strings.ToUpper(rv) != rv {
				t.Errorf("una rev di partenza non in maiuscole: %v", l)
			}
		}
	}
}
