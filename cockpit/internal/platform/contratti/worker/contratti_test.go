// L3 — contratti a due lati (voce 5.5, test K1–K4).
//
// I contratti JSON hanno due sorgenti scritte a mano: i tipi Go di questo pacchetto e i modelli
// pydantic di workers/contratti.py. contracts/*.schema.json è generato dal secondo. Finora niente
// verificava che il primo gli corrispondesse: le due parti potevano allontanarsi per mesi, e il
// disallineamento si presentava come un campo silenziosamente vuoto in produzione — un
// `casella_id` che non arriva, un `lease_token` che il worker manda e il server non legge — non come
// un errore.
//
// Questo test è anticipato dalla fase 5 alla fine della fase 1 per un motivo preciso: la fase 2
// cambia IngestRichiesta e MessaggioIn (voce 2.1) e nei due commit precedenti sono stati aggiunti
// nove schemi senza nulla che li legasse al Go. È il momento in cui costa meno e serve di più.
//
// Che cosa prova:
//
//	K1  un campo che esiste solo in Go            → il confronto degli insiemi fallisce
//	K2  un campo che esiste solo in Python        → idem, dall'altra parte
//	K3  un valore di enum aggiunto da una parte   → il confronto degli enum fallisce
//	K4  un campo sconosciuto in arrivo            → si ignora, non si rompe (compatibilità in avanti)
//
// Che cosa NON prova: l'obbligatorietà. pydantic sa dire «questo campo non ha un default, quindi è
// obbligatorio»; in Go ogni campo ha lo zero e la differenza non esiste. Confrontare i `required`
// darebbe una lista di falsi disallineamenti, che è il modo più rapido per far ignorare un test.
package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// tipiContratto lega ogni schema pubblicato al tipo Go che deve corrispondergli. Una voce in meno qui
// è uno schema che nessuno controlla: il test verifica anche che la mappa copra tutta la cartella.
var tipiContratto = map[string]any{
	"messaggio_in":              MessaggioIn{},
	"ingest_richiesta":          IngestRichiesta{},
	"ingest_risposta":           IngestRisposta{},
	"claim_richiesta":           ClaimRichiesta{},
	"casella_servita":           CasellaServita{},
	"job":                       Job{},
	"risultato_richiesta":       RisultatoRichiesta{},
	"heartbeat_richiesta":       HeartbeatRichiesta{},
	"payload_sync_outlook":      PayloadSyncOutlook{},
	"risultato_sync":            RisultatoSync{},
	"payload_stage_allegato":    PayloadStageAllegato{},
	"risultato_stage":           RisultatoStage{},
	"risultato_elemento":        RisultatoElemento{},
	"payload_crea_bozza":        PayloadCreaBozza{},
	"risultato_bozza":           RisultatoBozza{},
	"payload_apri_elemento":     PayloadApriElemento{},
	"payload_sposta_cartella":   PayloadSpostaCartella{},
	"risultato_sposta":          RisultatoSposta{},
	"payload_segna_letto":       PayloadSegnaLetto{},
	"payload_rileggi_elemento":  PayloadRileggiElemento{},
	"payload_analizza_allegato": PayloadAnalizzaAllegato{},
	"risultato_analisi":         RisultatoAnalisi{},
	"struttura_step":            StrutturaSTEP{},
	"testo_pdf":                 TestoPDF{},
	"testo_dxf":                 TestoDXF{},
	"foglio_dati":               FoglioDati{},
	"testo_dwg":                 TestoDWG{},
}

// tipiAnnidati: i modelli che compaiono dentro gli altri come $ref. Il nome del tipo Go e il nome
// della classe pydantic devono coincidere — non per estetica: è così che un campo che punta al
// modello sbagliato si vede.
var tipiAnnidati = map[string]any{
	"MessaggioIn":         MessaggioIn{},
	"AllegatoIn":          AllegatoIn{},
	"Destinatario":        Destinatario{},
	"CursoreLotto":        CursoreLotto{},
	"ElementoSaltato":     ElementoSaltato{},
	"EsitoMessaggio":      EsitoMessaggio{},
	"CartellaCursore":     CartellaCursore{},
	"CartellaEsito":       CartellaEsito{},
	"RiferimentoElemento": RiferimentoElemento{},
	"CasellaAperta":       CasellaAperta{},
	"RisultatoElemento":   RisultatoElemento{},
	"NodoSTEP":            NodoSTEP{},
	"RelazioneSTEP":       RelazioneSTEP{},
	"LimitiSTEP":          LimitiSTEP{},
	"ScartiSTEP":          ScartiSTEP{},
	"FrammentoPDF":        FrammentoPDF{},
	"CampoCartiglio":      CampoCartiglio{},
	"OCRPDF":              OCRPDF{},
	"TentativoOCR":        TentativoOCR{},
	"MetadatiPDF":         MetadatiPDF{},
	"LimitiTestoPDF":      LimitiTestoPDF{},
	"LimitiTestoDXF":      LimitiTestoDXF{},
	"TabellaPDF":          TabellaPDF{},
	"ColonnaTabella":      ColonnaTabella{},
	"RigaTabella":         RigaTabella{},
	"CellaTabella":        CellaTabella{},
	"CompletezzaPDF":      CompletezzaPDF{},
	"FoglioInfo":          FoglioInfo{},
	"CellaFoglio":         CellaFoglio{},
	"StileCella":          StileCella{},
	"CollegamentoCella":   CollegamentoCella{},
	"IntestazioneFoglio":  IntestazioneFoglio{},
	"ColonnaFoglio":       ColonnaFoglio{},
	"LimitiFoglio":        LimitiFoglio{},
	"CampoCartiglioDWG":   CampoCartiglioDWG{},
	"SorgenteDWG":         SorgenteDWG{},
	"LimitiTestoDWG":      LimitiTestoDWG{},
	"GeometriaSTEP":       GeometriaSTEP{},
	"IstanzaSTEP":         IstanzaSTEP{},
	"ParolaOCR":           ParolaOCR{},
	"AmbiguoOCR":          AmbiguoOCR{},
}

const cartellaContratti = "../../../../contracts"

// ---------------------------------------------------------------- forma

// forma è la descrizione minima di un campo, quella che le due parti devono condividere. Non si
// confrontano i tipi nativi (string di Go e str di Python non sono la stessa cosa) ma ciò che finisce
// nel JSON, che è l'unica cosa che le due parti si scambiano davvero.
type forma struct {
	genere  string // stringa | intero | numero | booleano | elenco | oggetto | qualunque
	modello string // nome del modello annidato, se il campo è un oggetto strutturato
	voce    *forma // per gli elenchi: la forma degli elementi
	valori  []string
}

func (f forma) String() string {
	s := f.genere
	if f.modello != "" {
		s += "(" + f.modello + ")"
	}
	if f.voce != nil {
		s += " di " + f.voce.String()
	}
	return s
}

var (
	tipoRawJSON = reflect.TypeOf(json.RawMessage{})
	tipoUUID    = reflect.TypeOf(uuid.UUID{})
	tipoTempo   = reflect.TypeOf(time.Time{})
)

func formaGo(t reflect.Type) forma {
	switch t {
	case tipoRawJSON:
		return forma{genere: "oggetto"} // json.RawMessage è []byte, ma nel contratto è un oggetto
	case tipoUUID:
		return forma{genere: "stringa"} // uuid.UUID è [16]byte, ma viaggia come stringa
	case tipoTempo:
		return forma{genere: "stringa"}
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
		return formaGo(t) // il puntatore aggiunge solo «può mancare», non cambia la forma
	}
	switch t.Kind() {
	case reflect.String:
		return forma{genere: "stringa"}
	case reflect.Bool:
		return forma{genere: "booleano"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return forma{genere: "intero"}
	case reflect.Float32, reflect.Float64:
		return forma{genere: "numero"}
	case reflect.Slice, reflect.Array:
		v := formaGo(t.Elem())
		return forma{genere: "elenco", voce: &v}
	case reflect.Map:
		return forma{genere: "oggetto"}
	case reflect.Struct:
		return forma{genere: "oggetto", modello: t.Name()}
	case reflect.Interface:
		return forma{genere: "qualunque"}
	}
	return forma{genere: "sconosciuto:" + t.Kind().String()}
}

// campiGo appiattisce i campi come fa il JSON: i campi incorporati senza tag (RiferimentoElemento)
// salgono di livello, esattamente come l'ereditarietà pydantic dall'altra parte.
func campiGo(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous && f.Tag.Get("json") == "" {
			for k, v := range campiGo(f.Type) {
				out[k] = v
			}
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		nome := strings.Split(tag, ",")[0]
		if nome == "" {
			nome = f.Name
		}
		out[nome] = f.Type
	}
	return out
}

// formaSchema legge la forma dichiarata nello schema. `anyOf` con un ramo null è come pydantic scrive
// «può mancare»: si guarda il ramo che non è null, perché è quello che descrive il dato.
func formaSchema(p map[string]any) forma {
	if rami, ok := p["anyOf"].([]any); ok {
		for _, r := range rami {
			m, ok := r.(map[string]any)
			if !ok || m["type"] == "null" {
				continue
			}
			return formaSchema(m)
		}
	}
	if rif, ok := p["$ref"].(string); ok {
		return forma{genere: "oggetto", modello: strings.TrimPrefix(rif, "#/$defs/")}
	}
	f := forma{}
	if e, ok := p["enum"].([]any); ok {
		for _, v := range e {
			f.valori = append(f.valori, fmt.Sprint(v))
		}
	}
	switch p["type"] {
	case "string":
		f.genere = "stringa"
	case "integer":
		f.genere = "intero"
	case "number":
		f.genere = "numero"
	case "boolean":
		f.genere = "booleano"
	case "object":
		f.genere = "oggetto"
	case "array":
		f.genere = "elenco"
		if it, ok := p["items"].(map[string]any); ok {
			v := formaSchema(it)
			f.voce = &v
		}
	case nil:
		f.genere = "qualunque"
	default:
		f.genere = fmt.Sprint(p["type"])
	}
	return f
}

// ---------------------------------------------------------------- confronto

func leggiSchema(t *testing.T, nome string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(cartellaContratti, nome+".schema.json"))
	if err != nil {
		t.Fatalf("schema %s: %v (rigenera con `python workers/genera_contratti.py`)", nome, err)
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("schema %s non è JSON valido: %v", nome, err)
	}
	return s
}

func proprieta(s map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	p, _ := s["properties"].(map[string]any)
	for k, v := range p {
		if m, ok := v.(map[string]any); ok {
			out[k] = m
		}
	}
	return out
}

func chiavi[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// confronta un oggetto dello schema con il tipo Go corrispondente, ricorsivamente sui modelli
// annidati. `visti` impedisce il giro infinito fra modelli che si citano.
func confronta(t *testing.T, dove string, schema map[string]any, defs map[string]any, goT reflect.Type, visti map[string]bool) {
	t.Helper()
	sp := proprieta(schema)
	gp := campiGo(goT)

	for _, nome := range chiavi(sp) {
		gtipo, c := gp[nome]
		if !c {
			// K2: campo dichiarato solo in Python. Il server non lo legge: il dato arriva e si perde.
			t.Errorf("%s: il campo %q è nello schema ma non nel tipo Go %s", dove, nome, goT.Name())
			continue
		}
		fs := formaSchema(sp[nome])
		fg := formaGo(gtipo)
		if fs.genere != fg.genere {
			t.Errorf("%s.%s: schema dice %s, Go dice %s", dove, nome, fs, fg)
			continue
		}
		if fs.voce != nil && fg.voce != nil && fs.voce.genere != fg.voce.genere {
			t.Errorf("%s.%s: elementi dell'elenco, schema %s, Go %s", dove, nome, fs.voce, fg.voce)
			continue
		}
		// modello annidato: i nomi devono coincidere e il contenuto va confrontato a sua volta
		rifS, rifG := fs.modello, fg.modello
		if fs.voce != nil && fg.voce != nil {
			rifS, rifG = fs.voce.modello, fg.voce.modello
		}
		if rifS == "" {
			continue
		}
		if rifS != rifG {
			t.Errorf("%s.%s: lo schema punta al modello %s, Go al tipo %s", dove, nome, rifS, rifG)
			continue
		}
		if visti[rifS] {
			continue
		}
		visti[rifS] = true
		annidato, ok := defs[rifS].(map[string]any)
		if !ok {
			t.Errorf("%s.%s: il modello %s non è fra i $defs dello schema", dove, nome, rifS)
			continue
		}
		modello, ok := tipiAnnidati[rifS]
		if !ok {
			t.Errorf("modello %s non registrato in tipiAnnidati: nessuno lo confronta", rifS)
			continue
		}
		confronta(t, dove+"."+nome+"→"+rifS, annidato, defs, reflect.TypeOf(modello), visti)
	}

	for _, nome := range chiavi(gp) {
		if _, c := sp[nome]; !c {
			// K1: campo dichiarato solo in Go. Il worker non lo manda mai: il server lo legge sempre zero.
			t.Errorf("%s: il campo %q è nel tipo Go %s ma non nello schema", dove, nome, goT.Name())
		}
	}
}

// TestOgniSchemaCorrispondeAlTipoGo è il cuore di L3: K1 e K2 insieme, su tutti e ventuno i contratti.
func TestOgniSchemaCorrispondeAlTipoGo(t *testing.T) {
	for _, nome := range chiavi(tipiContratto) {
		t.Run(nome, func(t *testing.T) {
			s := leggiSchema(t, nome)
			defs, _ := s["$defs"].(map[string]any)
			if defs == nil {
				defs = map[string]any{}
			}
			confronta(t, nome, s, defs, reflect.TypeOf(tipiContratto[nome]), map[string]bool{})
		})
	}
}

// Uno schema che nessuno confronta è peggio di uno schema che manca: dà l'impressione che il contratto
// sia verificato. Questo test è la ragione per cui aggiungere un contratto nuovo senza registrarlo qui
// fa diventare rossa la suite.
func TestTuttiGliSchemiSonoConfrontati(t *testing.T) {
	file, err := filepath.Glob(filepath.Join(cartellaContratti, "*.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(file) == 0 {
		t.Fatal("nessuno schema trovato: la cartella contracts/ è vuota?")
	}
	for _, f := range file {
		nome := strings.TrimSuffix(filepath.Base(f), ".schema.json")
		if _, c := tipiContratto[nome]; !c {
			t.Errorf("lo schema %q non è in tipiContratto: nessun test lo confronta con il Go", nome)
		}
	}
	if len(file) != len(tipiContratto) {
		t.Errorf("schemi su disco = %d, registrati = %d", len(file), len(tipiContratto))
	}
}

// I TEMPI DEL PROTOCOLLO (blocco 2 del 3R). Non sono campi JSON: sono numeri che le due parti devono
// avere uguali per forza, perché insieme formano una regola sola — «un worker vivo si fa riconoscere
// almeno ogni attesa_claim_s; chi tace per il doppio è offline». Finché stavano in tre file senza
// niente che li legasse, il server chiamava offline chi taceva da 60 secondi mentre il battito di un
// job lungo arrivava ogni 150: nessuno dei tre numeri era sbagliato da solo.
//
// Il file lo scrive `python workers/genera_contratti.py` da workers/protocollo.py, come gli schemi:
// cambiarne uno da una parte sola fa diventare rosso questo test invece di produrre, mesi dopo, una
// testata che dice il falso.
func TestITempiDelProtocolloSonoQuelliDeiWorker(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(cartellaContratti, "tempi_protocollo.json"))
	if err != nil {
		t.Fatalf("tempi del protocollo: %v (rigenera con `python workers/genera_contratti.py`)", err)
	}
	var py map[string]int
	if err := json.Unmarshal(raw, &py); err != nil {
		t.Fatal(err)
	}
	go_ := map[string]int{
		"attesa_claim_s":          int(AttesaClaim / time.Second),
		"attesa_claim_max_s":      int(AttesaClaimMax / time.Second),
		"presenza_online_entro_s": int(PresenzaOnlineEntro / time.Second),
		"battito_max_s":           int(BattitoMax / time.Second),
	}
	for nome, atteso := range go_ {
		v, c := py[nome]
		if !c {
			t.Errorf("%s: in Go c'è (%d s) e in workers/protocollo.py no", nome, atteso)
			continue
		}
		if v != atteso {
			t.Errorf("%s: Go %d s, worker Python %d s — due processi che non sono d'accordo su quanto aspettarsi a vicenda", nome, atteso, v)
		}
	}
	for nome := range py {
		if _, c := go_[nome]; !c {
			t.Errorf("%s: i worker lo dichiarano, il server non lo conosce", nome)
		}
	}
	// La soglia non è un numero scelto: è due giri di claim. Se qualcuno la slegasse per far sparire
	// un OFFLINE — che è esattamente la scorciatoia che il blocco 2 doveva evitare — questo lo dice.
	if PresenzaOnlineEntro != 2*AttesaClaim {
		t.Errorf("PresenzaOnlineEntro = %v: doveva essere due attese di claim (%v), non un numero a sé", PresenzaOnlineEntro, 2*AttesaClaim)
	}
	if BattitoMax > AttesaClaim {
		t.Errorf("BattitoMax = %v > AttesaClaim = %v: dentro un job il worker batte più lentamente di quanto si farebbe vivo da fermo, quindi sparirebbe dalla testata mentre lavora", BattitoMax, AttesaClaim)
	}
}

// K3 — gli enum. Un valore aggiunto da una parte sola è il difetto peggiore di tutti: il messaggio
// arriva, il server lo rifiuta con un errore sull'enum, e il lotto — prima della fase 1 — si fermava.
//
// Il verso «schema → Go» vale sempre: un valore che il contratto ammette e il database non conosce
// arriverebbe fino a un errore SQL. Il verso opposto vale solo dove l'enum del database è per
// costruzione l'elenco completo di ciò che può viaggiare; `worker_tipo` per esempio ha 'server', che
// non è un worker che fa il claim via HTTP e nel contratto non deve comparire.
func TestGliEnumDelContrattoSonoQuelliDelDatabase(t *testing.T) {
	casi := []struct {
		schema, dentro, campo string
		valido                func(string) bool
		tutti                 []string // se valorizzato: il contratto deve contenerli tutti
	}{
		{"messaggio_in", "", "direzione",
			func(v string) bool { return db.Direzione(v).Valid() },
			[]string{string(db.DirezioneEntrata), string(db.DirezioneUscita)}},
		{"messaggio_in", "AllegatoIn", "natura",
			func(v string) bool { return db.NaturaAllegato(v).Valid() },
			[]string{string(db.NaturaAllegatoFile), string(db.NaturaAllegatoInline),
				string(db.NaturaAllegatoElementoOutlook), string(db.NaturaAllegatoCollegamento)}},
		{"claim_richiesta", "", "worker",
			func(v string) bool { return db.WorkerTipo(v).Valid() }, nil},
		{"payload_crea_bozza", "", "tipo",
			func(v string) bool { return db.TipoBozza(v).Valid() },
			[]string{string(db.TipoBozzaRisposta), string(db.TipoBozzaRispondiTutti),
				string(db.TipoBozzaInoltro), string(db.TipoBozzaNuovo), string(db.TipoBozzaSollecito)}},
	}
	for _, c := range casi {
		nome := c.schema + "." + c.campo
		t.Run(nome, func(t *testing.T) {
			s := leggiSchema(t, c.schema)
			props := proprieta(s)
			if c.dentro != "" {
				defs, _ := s["$defs"].(map[string]any)
				d, ok := defs[c.dentro].(map[string]any)
				if !ok {
					t.Fatalf("$defs.%s non trovato in %s", c.dentro, c.schema)
				}
				props = proprieta(d)
			}
			p, ok := props[c.campo]
			if !ok {
				t.Fatalf("campo %q non trovato", c.campo)
			}
			valori := formaSchema(p).valori
			if len(valori) == 0 {
				t.Fatalf("il campo %q non dichiara un enum: il contratto accetta qualunque stringa", c.campo)
			}
			for _, v := range valori {
				if !c.valido(v) {
					t.Errorf("il contratto ammette %q, ma il database non lo conosce", v)
				}
			}
			for _, atteso := range c.tutti {
				if !contiene(valori, atteso) {
					t.Errorf("il database ha il valore %q, il contratto no: un messaggio legittimo verrebbe rifiutato", atteso)
				}
			}
		})
	}
}

func contiene(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// K4 — compatibilità in avanti. Un worker più nuovo del server manda campi che il server non conosce:
// devono essere ignorati, non far fallire la richiesta. In Go è il comportamento predefinito di
// encoding/json e in Python è `extra="ignore"` in contratti.Base; è comunque una scelta, e una scelta
// che regge un aggiornamento sfalsato delle due parti va verificata, non data per scontata.
func TestUnCampoSconosciutoNonRompeLaLettura(t *testing.T) {
	grezzo := []byte(`{
		"message_id": "<x@acme.example>", "entry_id": "E1", "store_id": "S1",
		"cartella": "Inbox", "direzione": "entrata", "data_evento": "2026-09-15T08:00:00Z",
		"campo_del_futuro": {"annidato": [1, 2, 3]},
		"allegati": [{"indice": 1, "nome_file": "a.pdf", "natura": "file", "novita": true}]
	}`)
	var m MessaggioIn
	if err := json.Unmarshal(grezzo, &m); err != nil {
		t.Fatalf("un campo sconosciuto ha fatto fallire la lettura: %v", err)
	}
	if m.MessageID != "<x@acme.example>" || m.Direzione != "entrata" {
		t.Errorf("i campi noti non sono stati letti: %+v", m)
	}
	if len(m.Allegati) != 1 || m.Allegati[0].NomeFile != "a.pdf" {
		t.Errorf("l'allegato non è stato letto: %+v", m.Allegati)
	}
}

// ---------------------------------------------------------------- struttura STEP (B8.4)

// La struttura non porta codici, e il contratto lo dice. Il worker non conosce il cliente della
// richiesta: separare un codice da un nome è una lettura che dipende dalle famiglie di QUEL cliente,
// e la fa il server (addendum B8, A1.2). Se un giorno qualcuno rimettesse `codice` nei nodi, questo
// test diventa rosso prima che il dato entri in database.
func TestLaStrutturaNonPortaCodici(t *testing.T) {
	s := leggiSchema(t, "struttura_step")
	defs, _ := s["$defs"].(map[string]any)
	nodo, ok := defs["NodoSTEP"].(map[string]any)
	if !ok {
		t.Fatal("NodoSTEP non è fra i $defs di struttura_step")
	}
	campi := proprieta(nodo)
	for _, vietato := range []string{"codice", "rev", "tipo", "famiglia"} {
		if _, c := campi[vietato]; c {
			t.Errorf("il nodo dello STEP dichiara %q: la classificazione è del server, non del worker", vietato)
		}
	}
	// riscritta per GP-3: la v4 aggiunge `geometria` (un riassunto diagnostico, non una classificazione)
	for _, atteso := range []string{"chiave", "id_grezzo", "nome_grezzo", "descrizione_grezza", "rev_grezza", "evidenza", "geometria"} {
		if _, c := campi[atteso]; !c {
			t.Errorf("il nodo dello STEP non dichiara %q", atteso)
		}
	}
	// lo stesso, dalla parte del Go: i campi del tipo sono quelli e non altri
	var n NodoSTEP
	if reflect.TypeOf(n).NumField() != 7 {
		t.Errorf("NodoSTEP ha %d campi, attesi 7", reflect.TypeOf(n).NumField())
	}
	// GP-3: nessuna voce di «rimozione» o di giudizio nel riassunto della geometria; lo stato e' una voce chiusa
	geo := proprieta(defs["GeometriaSTEP"].(map[string]any))
	for _, vietato := range []string{"feature", "da_rimuovere", "mono_pezzo", "pezzo_unico"} {
		if _, c := geo[vietato]; c {
			t.Errorf("la geometria dichiara %q: e' diagnostica, non un giudizio", vietato)
		}
	}
	got := formaSchema(proprieta(s)["stato"]).valori
	sort.Strings(got)
	want := []string{StrutturaAssente, StrutturaNonEstraibile, StrutturaParziale, StrutturaValida}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stato della struttura: lo schema dice %v, il Go conosce %v", got, want)
	}
	// riscritta per LS-01: la forma pubblicata è quella che il server accetta (la 3, come la 0020 e la gerarchia), e la
	// diagnostica ha il suo numero. Il default dello schema per la diagnostica è 0, «nessuna»: i fatti dei worker di
	// prima di GP-3 non la portano, e non vanno scambiati per una diagnostica 1.
	if d, _ := proprieta(s)["versione"]["default"].(float64); int(d) != VersioneStruttura || VersioneStruttura != 3 {
		t.Errorf("versione della struttura: schema %v, Go %d", proprieta(s)["versione"]["default"], VersioneStruttura)
	}
	if d, ok := proprieta(s)["versione_diagnostica"]["default"].(float64); !ok || int(d) != 0 || VersioneDiagnostica != 1 {
		t.Errorf("versione della diagnostica: default dello schema %v (atteso 0), Go %d (atteso 1)",
			proprieta(s)["versione_diagnostica"]["default"], VersioneDiagnostica)
	}
	if VersioneStrutturaGP3 == VersioneStruttura {
		t.Errorf("la forma ritirata di GP-3 (%d) non può essere quella che il server accetta", VersioneStrutturaGP3)
	}
}

// LS-01 (17/10/2026; prima TestStrutturaV4, GP-3). La struttura v3 con la diagnostica 1 è quella del worker di oggi: si
// decodifica con geometria, istanze e stato (nessun campo perso, nessun rifiuto) e non si rilegge. Una v3 senza
// diagnostica (worker di prima di GP-3) si legge senza diagnostica inventata e non si rilegge: la sua completezza è
// giusta. La v4 di GP-3 si legge (i campi non si perdono) ma la sua diagnostica non è dichiarata, e si rilegge; come la
// v5 di un worker sconosciuto, la v2 e i fatti senza struttura. Una diagnostica di un'altra versione si ignora, e non fa
// rileggere: la completezza non la guarda.
func TestStrutturaV3ConDiagnostica(t *testing.T) {
	struttura := func(versione, diagnostica string) json.RawMessage {
		campo := ""
		if diagnostica != "" {
			campo = `"versione_diagnostica": ` + diagnostica + `, `
		}
		return json.RawMessage(`{"struttura": {"versione": ` + versione + `, ` + campo + `"schema": "AP214", "radici": ["#10"],
		"nodi": [{"chiave": "#10", "nome_grezzo": "ACME-ASM", "evidenza": {}, "geometria": {"solidi": 0, "rappresentazioni": 1, "vuota": true}},
		         {"chiave": "#20", "nome_grezzo": "ACME-P1", "evidenza": {}, "geometria": {"solidi": 1, "rappresentazioni": 2, "vuota": false}}],
		"relazioni": [{"padre": "#10", "figlio": "#20", "qta": 2, "evidenza": {},
			"istanze": [{"rif": "#80", "id": "NAUO1", "nome": "ACME-P1:1"}, {"rif": "#81", "id": "NAUO2", "nome": "ACME-P1:2"}]}],
		"avvisi": [], "scarti": {}, "limiti": {}, "stato": "valida", "motivo_stato": ""}}`)
	}
	letta := func(t *testing.T, nome string, d json.RawMessage) *StrutturaSTEP {
		t.Helper()
		st, ok := DecodificaStruttura(d)
		if !ok {
			t.Fatalf("%s: la struttura non si decodifica", nome)
		}
		// i campi diagnostici arrivano tutti, qualunque sia il numero: chi decide se usarli è DiagnosticaSTEP
		if st.Stato != StrutturaValida || len(st.Nodi) != 2 || st.Nodi[0].Geometria == nil || !st.Nodi[0].Geometria.Vuota {
			t.Fatalf("%s: letta male: %+v", nome, st)
		}
		if g := st.Nodi[1].Geometria; g.Solidi != 1 || g.Rappresentazioni != 2 || g.Vuota {
			t.Errorf("%s: geometria letta male: %+v", nome, g)
		}
		if r := st.Relazioni[0]; len(r.Istanze) != 2 || r.Istanze[1] != (IstanzaSTEP{Rif: "#81", ID: "NAUO2", Nome: "ACME-P1:2"}) {
			t.Errorf("%s: istanze lette male: %+v", nome, r.Istanze)
		}
		return st
	}
	casi := []struct {
		nome                 string
		versione, diagnosi   string
		versioneDiagnostica  int
		diagnosticaLeggibile bool
		daRileggere          bool
	}{
		{"v3 con la diagnostica 1: il worker di oggi", "3", "1", 1, true, false},
		{"v3 con una diagnostica futura: si ignora, non si rilegge", "3", "2", 2, false, false},
		{"v4 di GP-3: i campi ci sono, il numero no; si rilegge", "4", "", 0, false, true},
		{"v5 sconosciuta, con la diagnostica 1: si rilegge", "5", "1", 1, true, true},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			d := struttura(c.versione, c.diagnosi)
			st := letta(t, c.nome, d)
			if st.VersioneDiagnostica != c.versioneDiagnostica || DiagnosticaSTEP(st) != c.diagnosticaLeggibile {
				t.Errorf("diagnostica %d (leggibile %v), attesa %d (%v)", st.VersioneDiagnostica, DiagnosticaSTEP(st),
					c.versioneDiagnostica, c.diagnosticaLeggibile)
			}
			if StrutturaDaRileggere(d) != c.daRileggere {
				t.Errorf("da rileggere %v, atteso %v", StrutturaDaRileggere(d), c.daRileggere)
			}
		})
	}
	if VersioneStrutturaGP3 != 4 || !StrutturaDaRileggere(struttura(strconv.Itoa(VersioneStrutturaGP3), "")) {
		t.Errorf("la forma ritirata di GP-3 è la 4 e si rilegge")
	}

	// una v3 senza diagnostica (un worker di prima di GP-3): si legge senza stato né geometria inventati, e non si
	// rilegge (prima di LS-01 il predicato l'avrebbe riaccodata, con tutte le v3: la rianalisi di massa che LS-01 evita)
	v3 := json.RawMessage(`{"struttura": {"versione": 3, "radici": [], "nodi": [{"chiave": "#1", "nome_grezzo": "X"}], "relazioni": []}}`)
	st, ok := DecodificaStruttura(v3)
	if !ok || st.Stato != "" || st.Nodi[0].Geometria != nil || st.VersioneDiagnostica != 0 || DiagnosticaSTEP(st) || StrutturaDaRileggere(v3) {
		t.Errorf("una v3 senza diagnostica si legge (senza diagnostica inventata) e non si rilegge: %v %+v", ok, st)
	}
	// una v2 si legge ancora (propone le aggiunte) ma si rilegge
	if v2 := json.RawMessage(`{"struttura": {"versione": 2, "radici": [], "nodi": [], "relazioni": []}}`); !StrutturaDaRileggere(v2) {
		t.Error("una v2 si rilegge")
	}
	// fatti di uno STEP senza struttura, o che non si decodificano: un'anomalia, si rilegge
	for _, d := range []json.RawMessage{json.RawMessage(``), json.RawMessage(`{"product_step": "X"}`), json.RawMessage(`rotto`),
		json.RawMessage(`{"struttura": {"nodi": []}}`), json.RawMessage(`{"struttura": {"versione": 1, "nodi": []}}`)} {
		if !StrutturaDaRileggere(d) {
			t.Errorf("senza una struttura accettata si rilegge: %s", d)
		}
	}
	if DiagnosticaSTEP(nil) {
		t.Error("nessuna struttura, nessuna diagnostica")
	}
}

func TestDecodificaStrutturaTolleraCampiSconosciuti(t *testing.T) {
	dettagli := json.RawMessage(`{
		"product_step": "77722757",
		"struttura": {
			"versione": 2, "schema": "AP214", "radici": ["#12"],
			"nodi": [{"chiave": "#12", "id_grezzo": "77722757", "nome_grezzo": "77722757_B",
			          "descrizione_grezza": "SUPPORTO", "rev_grezza": "B",
			          "evidenza": {"entita": "PRODUCT", "riga": "#12"}, "campo_del_futuro": 3}],
			"relazioni": [{"padre": "#12", "figlio": "#40", "qta": 2, "evidenza": {}}],
			"avvisi": [], "limiti": {"nodi_max": 2000, "byte_letti": 18234211, "troncato": false},
			"sezione_nuova": {"qualcosa": true}
		}
	}`)
	st, ok := DecodificaStruttura(dettagli)
	if !ok {
		t.Fatal("struttura non decodificata")
	}
	if st.Versione != 2 || st.Schema != "AP214" || len(st.Nodi) != 1 || len(st.Relazioni) != 1 {
		t.Fatalf("struttura letta male: %+v", st)
	}
	if st.Nodi[0].NomeGrezzo != "77722757_B" || st.Nodi[0].RevGrezza != "B" {
		t.Errorf("nodo letto male: %+v", st.Nodi[0])
	}
	if st.Nodi[0].Evidenza["riga"] != "#12" {
		t.Errorf("evidenza persa: %+v", st.Nodi[0].Evidenza)
	}
	if st.Relazioni[0].Qta != 2 || st.Limiti.ByteLetti != 18234211 || st.Limiti.Troncato {
		t.Errorf("relazioni o limiti letti male: %+v %+v", st.Relazioni[0], st.Limiti)
	}
	// I limiti qui sopra sono nella forma vecchia, senza i tetti nuovi: un fatto archiviato mesi fa
	// deve continuare a leggersi, con zero dove il worker di allora non scriveva niente.
	if st.Limiti.OccorrenzeMax != 0 || st.Limiti.Motivo != "" {
		t.Errorf("limiti vecchi letti male: %+v", st.Limiti)
	}
}

// Un albero parziale senza il motivo è un albero di cui non si sa niente: chi lo guarda non può
// distinguere «il file è troppo grande» da «la lettura è andata in timeout», e sono due cose che si
// risolvono in due modi diversi. Il tetto viaggia con il fatto perché la configurazione del worker
// che l'ha prodotto, fra un anno, non sarà più quella.
func TestUnaStrutturaTroncataDiceControQualeTetto(t *testing.T) {
	dettagli := json.RawMessage(`{"struttura": {
		"versione": 2, "schema": "AP242", "radici": ["#12"],
		"nodi": [], "relazioni": [], "avvisi": ["oltre 2000 PRODUCT: letti i primi"],
		"limiti": {"nodi_max": 2000, "occorrenze_max": 50000, "tempo_max_s": 120.0,
		           "byte_letti": 9531251, "tempo_s": 7.42, "troncato": true, "motivo": "nodi"}
	}}`)
	st, ok := DecodificaStruttura(dettagli)
	if !ok {
		t.Fatal("struttura non decodificata")
	}
	l := st.Limiti
	if !l.Troncato || l.Motivo != "nodi" {
		t.Errorf("troncamento non riportato: %+v", l)
	}
	if l.NodiMax != 2000 || l.OccorrenzeMax != 50000 || l.TempoMaxS != 120.0 {
		t.Errorf("i tetti non sono arrivati con il fatto: %+v", l)
	}
	if l.TempoS != 7.42 {
		t.Errorf("tempo impiegato letto male: %+v", l)
	}
}

// I fatti della v1 portavano un codice deciso dal worker. Leggerli come se fossero v2 farebbe
// rientrare dalla finestra la classificazione che A1.2 ha tolto dalla porta: si ignorano finché una
// rianalisi non li riscrive.
func TestUnaStrutturaVecchiaNonSiLegge(t *testing.T) {
	casi := map[string]json.RawMessage{
		"versione 1":      json.RawMessage(`{"struttura": {"versione": 1, "nodi": [{"chiave": "#1", "codice": "77722757"}]}}`),
		"senza versione":  json.RawMessage(`{"struttura": {"nodi": []}}`),
		"senza struttura": json.RawMessage(`{"product_step": "77722757"}`),
		"dettagli vuoti":  json.RawMessage(``),
		"dettagli rotti":  json.RawMessage(`non sono json`),
	}
	for nome, dettagli := range casi {
		t.Run(nome, func(t *testing.T) {
			if st, ok := DecodificaStruttura(dettagli); ok {
				t.Errorf("struttura accettata: %+v", st)
			}
		})
	}
}

// La v3 (addendum B8, A4.4, D35) porta gli scarti in numeri: il server decide la completezza da li',
// non dalle frasi. Una v2 si legge ancora, per le aggiunte, e senza scarti: Scarti resta nil, e nil
// non vuol dire «zero scarti» (la funzione SQL la dice incompleta).
func TestLaStrutturaV3PortaGliScartiInNumeri(t *testing.T) {
	v3 := json.RawMessage(`{"struttura": {
		"versione": 3, "schema": "AP214", "radici": ["#12"], "nodi": [], "relazioni": [],
		"avvisi": ["2 PRODUCT senza PRODUCT_DEFINITION: ignorati"],
		"scarti": {"prodotti_senza_definizione": 2, "occorrenze_non_risolte": 1,
		           "occorrenze_su_se_stesse": 3, "testi_troncati": 4},
		"limiti": {"troncato": false}
	}}`)
	st, ok := DecodificaStruttura(v3)
	if !ok || st.Scarti == nil {
		t.Fatalf("v3 non decodificata o senza scarti: %+v", st)
	}
	if *st.Scarti != (ScartiSTEP{ProdottiSenzaDefinizione: 2, OccorrenzeNonRisolte: 1, OccorrenzeSuSeStesse: 3, TestiTroncati: 4}) {
		t.Errorf("scarti letti male: %+v", *st.Scarti)
	}
	v2 := json.RawMessage(`{"struttura": {"versione": 2, "radici": [], "nodi": [], "relazioni": [], "avvisi": []}}`)
	st, ok = DecodificaStruttura(v2)
	if !ok {
		t.Fatal("una v2 deve leggersi ancora: propone le aggiunte")
	}
	if st.Scarti != nil {
		t.Errorf("una v2 non ha scarti, e non si inventano: %+v", st.Scarti)
	}
}

// ---------------------------------------------------------------- testo dei PDF (Smistamento F9)

// Prova 174. Il testo del PDF non porta codici, e il contratto lo dice: come per i nodi dello STEP
// (TestLaStrutturaNonPortaCodici), che cosa sia un codice lo dicono le famiglie del cliente della RFQ, e il
// worker non sa per quale RFQ analizza (A5.13.7, A5.13.8). Frammenti e campi del cartiglio hanno pagina,
// riquadro e fonte (nativo, ocr); l'OCR dice il suo stato; i metadati non portano l'autore: è il nome di una
// persona, e i fatti restano archiviati per anni.
//
// Riscritta per lo Smistamento (giro 4, fase 4.6): prima fissava sei voci dei campi del cartiglio; adesso sono sette,
// con il particolare simile (worker.CampoParticolareSimile: un campo, non un codice), e la sottoversione del testo
// che lo schema dichiara per difetto e' quella che il Go conosce (VersioneTestoPDF, la 2).
//
// Riscritta per il K4: le voci sono otto, con il riferimento al documento 3D (worker.CampoRiferimento3D, un dato del
// cartiglio e mai il codice del file); la sottoversione del testo dei PDF resta la 2.
func TestIlTestoDelPdfNonPortaCodici(t *testing.T) {
	s := leggiSchema(t, "testo_pdf")
	defs, _ := s["$defs"].(map[string]any)
	vietati := []string{"codice", "codici", "rev", "tipo", "famiglia"}
	controlla := func(dove string, campi map[string]map[string]any, attesi []string) {
		t.Helper()
		for _, v := range vietati {
			if _, c := campi[v]; c {
				t.Errorf("%s dichiara %q: la classificazione è del server, non del worker", dove, v)
			}
		}
		for _, a := range attesi {
			if _, c := campi[a]; !c {
				t.Errorf("%s non dichiara %q", dove, a)
			}
		}
	}
	def := func(nome string) map[string]map[string]any {
		t.Helper()
		m, ok := defs[nome].(map[string]any)
		if !ok {
			t.Fatalf("%s non è fra i $defs di testo_pdf", nome)
		}
		return proprieta(m)
	}
	controlla("TestoPDF", proprieta(s), []string{"versione", "estraibile", "pagine", "pagine_lette", "caratteri",
		"troncato", "formato_pagina1", "frammenti", "cartiglio", "metadati", "ocr", "limiti"})
	if _, c := proprieta(s)["zone"]; c {
		t.Error("TestoPDF dichiara ancora `zone`: il testo intero per pagina non viaggia più, solo i frammenti")
	}
	controlla("FrammentoPDF", def("FrammentoPDF"), []string{"pagina", "zona", "fonte", "testo", "riquadro", "confidenza"})
	controlla("CampoCartiglio", def("CampoCartiglio"), []string{"etichetta", "letta", "valore", "pagina", "zona",
		"fonte", "riquadro", "confidenza"})
	controlla("OCRPDF", def("OCRPDF"), []string{"stato", "motivo", "motore", "tentativi"})
	controlla("TentativoOCR", def("TentativoOCR"), []string{"pagina", "zona", "esito", "motivo", "caratteri", "secondi"})
	controlla("LimitiTestoPDF", def("LimitiTestoPDF"), []string{"pagine_max", "frammenti_max", "frammento_max",
		"caratteri_max", "campi_max", "ocr_soglia_pagina", "ocr_soglia_cartiglio", "ocr_pagine_max", "ocr_tempo_max_s", "ocr_dpi"})
	meta := def("MetadatiPDF")
	controlla("MetadatiPDF", meta, []string{"titolo", "soggetto", "parole_chiave", "creatore", "produttore"})
	for _, vietato := range []string{"autore", "author"} {
		if _, c := meta[vietato]; c {
			t.Errorf("i metadati del PDF dichiarano %q: l'autore non viaggia con i fatti", vietato)
		}
	}
	// le voci chiuse dello schema sono quelle che il Go conosce: zona, fonte, stato dell'OCR, etichette
	enum := func(campi map[string]map[string]any, campo string) []string {
		t.Helper()
		return formaSchema(campi[campo]).valori
	}
	vuole := func(dove string, got []string, want ...string) {
		t.Helper()
		sort.Strings(got)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: lo schema dice %v, il Go conosce %v", dove, got, want)
		}
	}
	vuole("zona di un frammento", enum(def("FrammentoPDF"), "zona"), ZonaBassoDestra, ZonaPagina)
	vuole("fonte di un frammento", enum(def("FrammentoPDF"), "fonte"), FonteTestoNativo, FonteTestoOCR)
	vuole("stato dell'OCR", enum(def("OCRPDF"), "stato"), OCRNonNecessario, OCRSpento, OCRNonDisponibile, OCREseguito,
		OCRFallito, OCRScaduto, OCRIlleggibile)
	vuole("etichette del cartiglio", enum(def("CampoCartiglio"), "etichetta"), CampoNumeroDisegno, CampoCodice,
		CampoRevisione, CampoTitolo, CampoScala, CampoMateriale, CampoParticolareSimile, CampoRiferimento3D)
	// la sottoversione del testo: quella del worker di oggi (il default dello schema) e' quella del Go. Riscritta per
	// GP-3: la 3 porta le tabelle; il cartiglio resta quello della 2 (VersioneCartiglioPDF).
	if d, _ := proprieta(s)["versione"]["default"].(float64); int(d) != VersioneTestoPDF || VersioneTestoPDF != 3 || VersioneCartiglioPDF != 2 {
		t.Errorf("la sottoversione del testo: lo schema dice %v, il Go conosce %d/%d (GP-3 porta la 3, cartiglio della 2)", proprieta(s)["versione"]["default"], VersioneTestoPDF, VersioneCartiglioPDF)
	}
	// GP-3: le tabelle e la completezza. Le voci chiuse (ruoli, verso, stato) sono quelle delle costanti Go, e le
	// righe non portano codici: valori grezzi con le colonne.
	controlla("TabellaPDF", def("TabellaPDF"), []string{"pagina", "indice", "verso", "riquadro_intestazione", "colonne", "righe"})
	controlla("ColonnaTabella", def("ColonnaTabella"), []string{"indice", "intestazione", "ruolo", "omessa", "riquadro"})
	controlla("RigaTabella", def("RigaTabella"), []string{"indice", "riquadro", "continuazione", "celle"})
	controlla("CellaTabella", def("CellaTabella"), []string{"colonna", "intestazione", "ruolo", "valore", "riquadro", "omessa", "sorgente"})
	controlla("CompletezzaPDF", def("CompletezzaPDF"), []string{"stato", "pagine", "pagine_lette", "pagine_senza_testo", "righe_troncate", "motivo"})
	ruoli := []string{RuoloNessuno, RuoloPosizione, RuoloLivello, RuoloCodice, RuoloDescrizione, RuoloQuantita, RuoloUnita,
		RuoloMateriale, RuoloRevisione, RuoloDocumento, RuoloNota}
	vuole("ruolo di una colonna", enum(def("ColonnaTabella"), "ruolo"), ruoli...)
	vuole("ruolo di una cella", enum(def("CellaTabella"), "ruolo"), ruoli...)
	vuole("verso di una tabella", enum(def("TabellaPDF"), "verso"), VersoSotto, VersoSopra)
	vuole("stato della completezza", enum(def("CompletezzaPDF"), "stato"), CompletezzaCompleta, CompletezzaParziale, CompletezzaNonLeggibile)
	// lo stesso, dalla parte del Go: i campi sono quelli e non altri
	for nome, campi := range map[string]int{"FrammentoPDF": 8, "CampoCartiglio": 9, "MetadatiPDF": 5, "OCRPDF": 4, "TentativoOCR": 6} {
		if n := reflect.TypeOf(tipiAnnidati[nome]).NumField(); n != campi {
			t.Errorf("%s ha %d campi, attesi %d", nome, n, campi)
		}
	}
}

// Prova 175. DecodificaTestoPDF legge la lettura strutturata, tollera i campi sconosciuti, e dice «non c'e'»
// (falso) quando manca o quando la versione non e' una che conosce (0): falso non vuol dire «il PDF non ha
// testo», che e' `estraibile` falso in un testo letto. Un fatto dell'analizzatore 4 scritto da un worker
// vecchio non ha `testo_pdf`: e' un testo non letto, da rianalizzare (A5.13.8).
//
// Riscritta per lo Smistamento (giro 4, fase 4.6): prima fissava la sola versione 1 del testo; adesso anche la
// sottoversione 2, con il campo del particolare simile e l'etichetta bilingue letta intera, e la 1 si legge ancora
// (la riconosce come vecchia classificazione.TestoPDFDaRileggere, non la decodifica).
func TestDecodificaTestoPDF(t *testing.T) {
	dettagli := json.RawMessage(`{
		"cartiglio": true, "termini_trovati": ["SCALA"],
		"fonti": {"tipo": "termini_pdf", "codice": "nome_file"},
		"testo_pdf": {"versione": 1, "estraibile": true, "pagine": 3, "pagine_lette": 3, "caratteri": 2140,
			"troncato": false, "formato_pagina1": [842, 595],
			"frammenti": [
				{"pagina": 1, "zona": "basso_destra", "fonte": "nativo", "testo": "DISEGNO N. 7120010", "riquadro": [560, 530.3, 655, 542.7], "confidenza": null, "campo_del_futuro": 1},
				{"pagina": 1, "zona": "basso_destra", "fonte": "ocr", "testo": "REV. B", "riquadro": [680, 510, 720, 522], "confidenza": 64.5},
				{"pagina": 2, "zona": "pagina", "fonte": "nativo", "testo": "NOTA 7120011", "riquadro": [40, 50, 200, 62], "confidenza": null}],
			"cartiglio": [{"etichetta": "numero_disegno", "letta": "DISEGNO N.", "valore": "7120010", "pagina": 1,
				"zona": "basso_destra", "fonte": "nativo", "riquadro": [620, 530.3, 655, 542.7], "confidenza": null}],
			"metadati": {"titolo": "7120010", "soggetto": "", "parole_chiave": "", "creatore": "CAD", "produttore": "PDF"},
			"ocr": {"stato": "eseguito", "motivo": "", "motore": "tesseract (pymupdf)",
				"tentativi": [{"pagina": 1, "zona": "basso_destra", "esito": "letto", "motivo": "", "caratteri": 6, "secondi": 1.25}]},
			"limiti": {"pagine_max": 11, "frammenti_max": 200, "frammento_max": 512, "caratteri_max": 16384, "campi_max": 24,
				"ocr_soglia_pagina": 20, "ocr_soglia_cartiglio": 8, "ocr_pagine_max": 2, "ocr_tempo_max_s": 60, "ocr_dpi": 300},
			"sezione_nuova": {"qualcosa": true}}
	}`)
	tp, ok := DecodificaTestoPDF(dettagli)
	if !ok {
		t.Fatal("testo del PDF non decodificato")
	}
	if tp.Versione != 1 || !tp.Estraibile || tp.Pagine != 3 || tp.PagineLette != 3 || tp.Caratteri != 2140 || tp.Troncato ||
		!reflect.DeepEqual(tp.FormatoPagina1, []float64{842, 595}) {
		t.Errorf("testo letto male: %+v", tp)
	}
	if len(tp.Frammenti) != 3 {
		t.Fatalf("frammenti letti male: %+v", tp.Frammenti)
	}
	if f := tp.Frammenti[0]; f.Pagina != 1 || f.Zona != ZonaBassoDestra || f.Fonte != FonteTestoNativo || f.Testo != "DISEGNO N. 7120010" ||
		!reflect.DeepEqual(f.Riquadro, []float64{560, 530.3, 655, 542.7}) || f.Confidenza != nil {
		t.Errorf("frammento nativo letto male: %+v", f)
	}
	if f := tp.Frammenti[1]; f.Fonte != FonteTestoOCR || f.Confidenza == nil || *f.Confidenza != 64.5 {
		t.Errorf("frammento OCR letto male: %+v", f)
	}
	if len(tp.Cartiglio) != 1 || tp.Cartiglio[0].Etichetta != CampoNumeroDisegno || tp.Cartiglio[0].Valore != "7120010" ||
		tp.Cartiglio[0].Letta != "DISEGNO N." || tp.Cartiglio[0].Zona != ZonaBassoDestra || len(tp.Cartiglio[0].Riquadro) != 4 {
		t.Errorf("campi del cartiglio letti male: %+v", tp.Cartiglio)
	}
	if tp.Metadati.Titolo != "7120010" || tp.Metadati.Creatore != "CAD" || tp.Metadati.Produttore != "PDF" {
		t.Errorf("metadati letti male: %+v", tp.Metadati)
	}
	if tp.OCR.Stato != OCREseguito || tp.OCR.Motore != "tesseract (pymupdf)" || len(tp.OCR.Tentativi) != 1 ||
		tp.OCR.Tentativi[0] != (TentativoOCR{Pagina: 1, Zona: ZonaBassoDestra, Esito: "letto", Caratteri: 6, Secondi: 1.25}) {
		t.Errorf("OCR letto male: %+v", tp.OCR)
	}
	if tp.Limiti != (LimitiTestoPDF{PagineMax: 11, FrammentiMax: 200, FrammentoMax: 512, CaratteriMax: 16384, CampiMax: 24,
		OCRSogliaPagina: 20, OCRSogliaCartiglio: 8, OCRPagineMax: 2, OCRTempoMaxS: 60, OCRDpi: 300}) {
		t.Errorf("i limiti non sono arrivati con il fatto: %+v", tp.Limiti)
	}

	// la sottoversione 2 (fase 4.6): il campo del particolare simile e l'etichetta con la gemella inglese
	v2 := json.RawMessage(`{"testo_pdf": {"versione": 2, "estraibile": true, "pagine": 1, "pagine_lette": 1, "caratteri": 60,
		"frammenti": [{"pagina": 1, "zona": "basso_destra", "fonte": "nativo", "testo": "PART. N° 7120001A1\nPARTICOLARE SIMILE / SIMILAR PART 7120012"}],
		"cartiglio": [
			{"etichetta": "codice", "letta": "PART. N°", "valore": "7120001A1", "pagina": 1, "zona": "basso_destra", "fonte": "nativo"},
			{"etichetta": "particolare_simile", "letta": "PARTICOLARE SIMILE / SIMILAR PART", "valore": "7120012", "pagina": 1,
				"zona": "basso_destra", "fonte": "nativo", "riquadro": [650, 440, 690, 452]},
			{"etichetta": "titolo", "letta": "DENOMINAZIONE / NAME", "valore": "STAFFA ACME", "pagina": 1, "zona": "basso_destra", "fonte": "nativo"}],
		"ocr": {"stato": "non_necessario"}}}`)
	tp, ok = DecodificaTestoPDF(v2)
	if !ok || tp.Versione != VersioneCartiglioPDF || len(tp.Cartiglio) != 3 || CartiglioPDFDiPrima(tp) {
		t.Fatalf("il testo della sottoversione 2 non si decodifica, o il suo cartiglio passa per «di prima»: %v %+v", ok, tp)
	}
	if tp.Tabelle != nil || tp.Completezza != nil {
		t.Errorf("una 2 non ha tabelle ne' completezza, e non si inventano: %+v %+v", tp.Tabelle, tp.Completezza)
	}
	if c := tp.Cartiglio[1]; c.Etichetta != CampoParticolareSimile || c.Valore != "7120012" || c.Letta != "PARTICOLARE SIMILE / SIMILAR PART" ||
		len(c.Riquadro) != 4 {
		t.Errorf("il campo del particolare simile letto male: %+v", c)
	}
	if c := tp.Cartiglio[2]; c.Etichetta != CampoTitolo || c.Letta != "DENOMINAZIONE / NAME" || c.Valore != "STAFFA ACME" {
		t.Errorf("l'etichetta bilingue letta male: %+v", c)
	}

	// un PDF senza testo: si decodifica (e' una risposta), con estraibile falso e l'OCR che dice perche' no
	muto := json.RawMessage(`{"testo_letto": 1, "testo_pdf": {"versione": 1, "estraibile": false, "pagine": 1,
		"pagine_lette": 1, "caratteri": 0, "frammenti": [], "cartiglio": [], "metadati": {"titolo": "7120010"},
		"ocr": {"stato": "non_disponibile", "motivo": "Tesseract non disponibile su questa postazione"}, "limiti": {"caratteri_max": 16384}}}`)
	tp, ok = DecodificaTestoPDF(muto)
	if !ok || tp.Estraibile || len(tp.Frammenti) != 0 || tp.Metadati.Titolo != "7120010" || tp.OCR.Stato != OCRNonDisponibile {
		t.Errorf("un PDF senza testo e' una risposta da leggere, non un fatto da scartare: %v %+v", ok, tp)
	}

	casi := map[string]json.RawMessage{
		"versione 0":           json.RawMessage(`{"testo_pdf": {"versione": 0, "estraibile": true, "frammenti": []}}`),
		"senza versione":       json.RawMessage(`{"testo_pdf": {"estraibile": true}}`),
		"fatto v4 senza testo": json.RawMessage(`{"cartiglio": true, "termini_trovati": ["SCALA"]}`),
		"fatto v3 di uno STEP": json.RawMessage(`{"struttura": {"versione": 3, "nodi": []}}`),
		"PDF che non si apre":  json.RawMessage(`{"errore_pdf": "cannot open broken document"}`),
		"testo_pdf nullo":      json.RawMessage(`{"testo_pdf": null}`),
		"dettagli vuoti":       json.RawMessage(``),
		"dettagli rotti":       json.RawMessage(`non sono json`),
	}
	for nome, d := range casi {
		t.Run(nome, func(t *testing.T) {
			if tp, ok := DecodificaTestoPDF(d); ok {
				t.Errorf("testo accettato: %+v", tp)
			}
		})
	}
}

// ---------------------------------------------------------------- testo dei DXF (K4)

// K4. Il testo del DXF non porta codici, come quello del PDF: solo i campi del cartiglio riconosciuti (lo stesso
// CampoCartiglio, con le stesse voci), l'esito della lettura e i limiti in vigore. La forma che lo schema dichiara per
// difetto e' quella che il Go conosce (VersioneTestoDXF, la 1), e le voci chiuse sono quelle delle costanti Go.
func TestIlTestoDelDxfNonPortaCodici(t *testing.T) {
	s := leggiSchema(t, "testo_dxf")
	defs, _ := s["$defs"].(map[string]any)
	def := func(nome string) map[string]map[string]any {
		t.Helper()
		m, ok := defs[nome].(map[string]any)
		if !ok {
			t.Fatalf("%s non è fra i $defs di testo_dxf", nome)
		}
		return proprieta(m)
	}
	for _, v := range []string{"codice", "codici", "rev", "tipo", "famiglia", "testo", "frammenti"} {
		if _, c := proprieta(s)[v]; c {
			t.Errorf("TestoDXF dichiara %q: la classificazione è del server, e gli altri testi del disegno non viaggiano", v)
		}
	}
	for _, a := range []string{"versione", "esito", "motivo", "versione_dxf", "codifica", "testi", "annidati", "troncato",
		"cartiglio", "limiti"} {
		if _, c := proprieta(s)[a]; !c {
			t.Errorf("TestoDXF non dichiara %q", a)
		}
	}
	got := formaSchema(proprieta(s)["esito"]).valori
	sort.Strings(got)
	if want := []string{DXFLetto, DXFNonLetto}; !reflect.DeepEqual(got, want) {
		t.Errorf("esito del DXF: lo schema dice %v, il Go conosce %v", got, want)
	}
	got = formaSchema(def("CampoCartiglio")["etichetta"]).valori
	sort.Strings(got)
	want := []string{CampoNumeroDisegno, CampoCodice, CampoRevisione, CampoTitolo, CampoScala, CampoMateriale,
		CampoParticolareSimile, CampoRiferimento3D}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("etichette del cartiglio del DXF: lo schema dice %v, il Go conosce %v", got, want)
	}
	// Riscritta per GP-3: la v2 segue gli INSERT annidati e porta i conti (annidati_seguiti, oltre_profondita, cicli,
	// specchiati) e `parziale`; i limiti la profondita' e gli INSERT visitati.
	if d, _ := proprieta(s)["versione"]["default"].(float64); int(d) != VersioneTestoDXF || VersioneTestoDXF != 2 {
		t.Errorf("la versione del testo del DXF: lo schema dice %v, il Go conosce %d", proprieta(s)["versione"]["default"], VersioneTestoDXF)
	}
	for _, a := range []string{"annidati_seguiti", "oltre_profondita", "cicli", "specchiati", "parziale"} {
		if _, c := proprieta(s)[a]; !c {
			t.Errorf("TestoDXF v2 non dichiara %q", a)
		}
	}
	for nome, campi := range map[string]int{"LimitiTestoDXF": 6, "CampoCartiglio": 9} {
		if n := reflect.TypeOf(tipiAnnidati[nome]).NumField(); n != campi {
			t.Errorf("%s ha %d campi, attesi %d", nome, n, campi)
		}
	}
	if n := reflect.TypeOf(TestoDXF{}).NumField(); n != 15 {
		t.Errorf("TestoDXF ha %d campi, attesi 15", n)
	}
}

// K4. DecodificaTestoDXF legge il testo del DXF e tollera i campi sconosciuti; un DXF binario si decodifica (e' una
// risposta: esito non_letto); falso quando il testo non c'e' (un DXF analizzato prima del K4 ha i dettagli vuoti) o
// la versione e' sconosciuta. TestoDXFDaRileggere riaccoda il primo caso e non un DXF binario.
func TestDecodificaTestoDXF(t *testing.T) {
	letto := json.RawMessage(`{"esito": {"tipo_proposto": "sviluppo_dxf"}, "testo_dxf": {"versione": 1, "esito": "letto",
		"motivo": "", "versione_dxf": "AC1014", "codifica": "cp1252", "testi": 12, "annidati": 0, "troncato": false,
		"cartiglio": [
			{"etichetta": "codice", "letta": "part no./Mat. Nr.", "valore": "ACME1001", "pagina": 1, "zona": "pagina",
				"fonte": "nativo", "riquadro": [10, 20, 30, 23.5], "confidenza": null},
			{"etichetta": "riferimento_3d", "letta": "ref.3D doc./Ref.3D Dok.", "valore": "ACME3001", "pagina": 1,
				"zona": "pagina", "fonte": "nativo", "riquadro": [10, 38, 25, 40], "campo_del_futuro": true}],
		"limiti": {"byte_max": 67108864, "testi_max": 5000, "testo_max": 512, "campi_max": 24}, "sezione_nuova": 1}}`)
	td, ok := DecodificaTestoDXF(letto)
	if !ok {
		t.Fatal("testo del DXF non decodificato")
	}
	if td.Versione != 1 || td.Esito != DXFLetto || td.VersioneDXF != "AC1014" || td.Codifica != "cp1252" ||
		td.Testi != 12 || td.Troncato || len(td.Cartiglio) != 2 {
		t.Errorf("testo del DXF letto male: %+v", td)
	}
	if c := td.Cartiglio[1]; c.Etichetta != CampoRiferimento3D || c.Valore != "ACME3001" || c.Letta != "ref.3D doc./Ref.3D Dok." ||
		len(c.Riquadro) != 4 {
		t.Errorf("il riferimento al documento 3D letto male: %+v", c)
	}
	if td.Limiti != (LimitiTestoDXF{ByteMax: 67108864, TestiMax: 5000, TestoMax: 512, CampiMax: 24}) {
		t.Errorf("i limiti non sono arrivati con il fatto: %+v", td.Limiti)
	}
	// Riscritta per GP-3: un testo v1 letto si decodifica ancora, ma si rilegge (la v2 segue gli INSERT annidati)
	if !TestoDXFDaRileggere(letto) {
		t.Error("un DXF letto con la v1 si rianalizza: la v2 segue gli INSERT annidati")
	}
	v2 := json.RawMessage(`{"testo_dxf": {"versione": 2, "esito": "letto", "testi": 40, "annidati": 1, "annidati_seguiti": 3,
		"oltre_profondita": 1, "cicli": 0, "specchiati": 0, "troncato": false, "parziale": true,
		"motivo": "1 INSERT oltre 8 livelli non seguiti", "cartiglio": [],
		"limiti": {"byte_max": 67108864, "testi_max": 5000, "testo_max": 512, "campi_max": 24, "profondita_max": 8, "inserti_max": 20000}}}`)
	if td, ok := DecodificaTestoDXF(v2); !ok || !td.Parziale || td.AnnidatiSeguiti != 3 || td.OltreProfondita != 1 ||
		td.Limiti.ProfonditaMax != 8 || td.Limiti.InsertiMax != 20000 || TestoDXFDaRileggere(v2) {
		t.Errorf("la v2 letta male, o riaccodata: %v %+v", ok, td)
	}

	// un DXF binario e' una risposta, in tutte e due le versioni: la v2 non lo legge diversamente
	for _, v := range []int{1, 2} {
		binario := json.RawMessage(fmt.Sprintf(`{"testo_dxf": {"versione": %d, "esito": "non_letto", "motivo": "DXF binario", "cartiglio": []}}`, v))
		if td, ok := DecodificaTestoDXF(binario); !ok || td.Esito != DXFNonLetto || TestoDXFDaRileggere(binario) {
			t.Errorf("un DXF binario e' una risposta, non un fatto da rileggere: %v %+v", ok, td)
		}
	}

	casi := map[string]json.RawMessage{
		"DXF di prima del K4": json.RawMessage(`{"esito": {"tipo_proposto": "sviluppo_dxf"}}`),
		"versione 0":          json.RawMessage(`{"testo_dxf": {"versione": 0, "esito": "letto"}}`),
		"senza versione":      json.RawMessage(`{"testo_dxf": {"esito": "letto"}}`),
		"testo_dxf nullo":     json.RawMessage(`{"testo_dxf": null}`),
		"testo di un PDF":     json.RawMessage(`{"testo_pdf": {"versione": 2, "estraibile": true}}`),
		"dettagli vuoti":      json.RawMessage(``),
		"dettagli rotti":      json.RawMessage(`non sono json`),
	}
	for nome, d := range casi {
		t.Run(nome, func(t *testing.T) {
			if td, ok := DecodificaTestoDXF(d); ok {
				t.Errorf("testo accettato: %+v", td)
			}
			if !TestoDXFDaRileggere(d) {
				t.Error("un DXF senza il testo di oggi va rianalizzato")
			}
		})
	}
}

// GP-3 (direttiva 21 §5.1-5.2). La sottoversione 3 porta le tabelle (righe con colonne, valori grezzi, riquadri) e la
// completezza della lettura: con lo stato «parziale» il server dice «verifica incompleta», non «codice assente». Il
// cartiglio di una 3 non e' di prima, e una 2 si riaccoda per le tabelle senza che il suo cartiglio lo diventi.
func TestDecodificaTestoPDFV3Tabelle(t *testing.T) {
	v3 := json.RawMessage(`{"testo_pdf": {"versione": 3, "estraibile": true, "pagine": 2, "pagine_lette": 2, "caratteri": 900,
		"tabelle": [{"pagina": 1, "indice": 1, "verso": "sotto", "riquadro_intestazione": [48, 470, 553, 482],
			"colonne": [{"indice": 0, "intestazione": "Item", "ruolo": "posizione", "omessa": false, "riquadro": [48, 470, 64, 482]},
			            {"indice": 1, "intestazione": "Material", "ruolo": "materiale", "riquadro": [91, 470, 119, 482]},
			            {"indice": 2, "intestazione": "Qty", "ruolo": "quantita", "riquadro": [318, 470, 341, 482]}],
			"righe": [{"indice": 1, "riquadro": [48, 505, 374, 517], "continuazione": false, "celle": [
				{"colonna": 0, "intestazione": "Item", "ruolo": "posizione", "valore": "0010", "riquadro": [48, 505, 72, 517]},
				{"colonna": 1, "intestazione": "Material", "ruolo": "materiale", "valore": "ACME1001", "riquadro": [91, 505, 145, 517]},
				{"colonna": 2, "intestazione": "Qty", "ruolo": "quantita", "valore": "200 PC", "riquadro": [338, 505, 374, 517], "campo_del_futuro": 1}]},
			          {"indice": 2, "riquadro": [147, 517, 177, 529], "continuazione": true, "celle": [
				{"colonna": 1, "intestazione": "Material", "ruolo": "materiale", "valore": "LOS 40", "sorgente": []}]}]}],
		"completezza": {"stato": "parziale", "pagine": 40, "pagine_lette": 30, "pagine_senza_testo": 0, "righe_troncate": false,
			"motivo": "lette 30 pagine su 40 (tetto 30)"},
		"limiti": {"tabelle_pagine_max": 30, "tabelle_max": 20, "righe_max": 400, "cella_max": 200}}}`)
	tp, ok := DecodificaTestoPDF(v3)
	if !ok || tp.Versione != VersioneTestoPDF || CartiglioPDFDiPrima(tp) {
		t.Fatalf("la 3 non si decodifica: %v %+v", ok, tp)
	}
	if len(tp.Tabelle) != 1 || tp.Tabelle[0].Verso != VersoSotto || len(tp.Tabelle[0].Colonne) != 3 || len(tp.Tabelle[0].Righe) != 2 {
		t.Fatalf("tabella letta male: %+v", tp.Tabelle)
	}
	r := tp.Tabelle[0].Righe
	if r[0].Continuazione || len(r[0].Celle) != 3 || r[0].Celle[1].Valore != "ACME1001" || r[0].Celle[2].Ruolo != RuoloQuantita ||
		r[0].Celle[2].Valore != "200 PC" || !reflect.DeepEqual(r[0].Celle[0].Riquadro, []float64{48, 505, 72, 517}) {
		t.Errorf("riga letta male: %+v", r[0])
	}
	if !r[1].Continuazione || r[1].Celle[0].Valore != "LOS 40" {
		t.Errorf("la continuazione e' una riga a se', segnata: %+v", r[1])
	}
	if tp.Completezza == nil || tp.Completezza.Stato != CompletezzaParziale || tp.Completezza.Pagine != 40 || tp.Completezza.PagineLette != 30 {
		t.Errorf("completezza letta male: %+v", tp.Completezza)
	}
	if tp.Limiti.TabellePagineMax != 30 || tp.Limiti.RigheMax != 400 || tp.Limiti.CellaMax != 200 || tp.Limiti.TabelleMax != 20 {
		t.Errorf("i tetti delle tabelle non sono arrivati con il fatto: %+v", tp.Limiti)
	}
	// la soglia del cartiglio: una 1 e' di prima, una 2 e una 3 no
	for v, diPrima := range map[int]bool{1: true, 2: false, 3: false} {
		if CartiglioPDFDiPrima(&TestoPDF{Versione: v}) != diPrima {
			t.Errorf("sottoversione %d: cartiglio di prima = %v, atteso %v", v, !diPrima, diPrima)
		}
	}
	if CartiglioPDFDiPrima(nil) {
		t.Error("un testo che non c'e' non ha un cartiglio di prima")
	}
}

// GP-3 (G-8.6). L'OCR con la confidenza parola per parola e i caratteri ambigui (segnalati, mai corretti); un PDF letto
// senza motore, con dei passaggi saltati, aspetta l'OCR (TestoPDFAttendeOCR); uno che non ne aveva bisogno, o letto con
// l'OCR, no.
func TestOCRConConfidenzaEAmbigui(t *testing.T) {
	letto := json.RawMessage(`{"testo_pdf": {"versione": 3, "estraibile": false, "pagine": 1, "pagine_lette": 1,
		"frammenti": [{"pagina": 1, "zona": "basso_destra", "fonte": "ocr", "testo": "DWG NO. 1650B0", "confidenza": 71.5,
			"parole": [{"testo": "1650B0", "confidenza": 58.0, "riquadro": [600, 500, 640, 510]}],
			"ambigui": [{"posizione": 8, "letto": "1", "alternative": ["I", "l"]}, {"posizione": 12, "letto": "B", "alternative": ["8"]}]}],
		"cartiglio": [{"etichetta": "numero_disegno", "letta": "DWG NO.", "valore": "1650B0", "fonte": "ocr", "zona": "basso_destra",
			"ambigui": [{"posizione": 4, "letto": "B", "alternative": ["8"]}]}],
		"ocr": {"stato": "eseguito", "motore": "tesseract (tsv)"}}}`)
	tp, ok := DecodificaTestoPDF(letto)
	if !ok || len(tp.Frammenti) != 1 || len(tp.Frammenti[0].Parole) != 1 || *tp.Frammenti[0].Parole[0].Confidenza != 58.0 {
		t.Fatalf("parole OCR lette male: %v %+v", ok, tp)
	}
	if a := tp.Cartiglio[0].Ambigui; len(a) != 1 || a[0].Letto != "B" || a[0].Alternative[0] != "8" || tp.Cartiglio[0].Valore != "1650B0" {
		t.Errorf("ambigui letti male, o il valore corretto: %+v", tp.Cartiglio[0])
	}
	if TestoPDFAttendeOCR(letto) {
		t.Error("un PDF letto con l'OCR non aspetta l'OCR")
	}
	senza := json.RawMessage(`{"testo_pdf": {"versione": 3, "estraibile": false, "ocr": {"stato": "non_disponibile",
		"motivo": "Tesseract senza le lingue ita", "tentativi": [{"pagina": 1, "zona": "pagina", "esito": "saltato"}]}}}`)
	if !TestoPDFAttendeOCR(senza) {
		t.Error("un PDF letto senza motore, con dei passaggi saltati, aspetta l'OCR")
	}
	for nome, d := range map[string]json.RawMessage{
		"OCR non necessario": json.RawMessage(`{"testo_pdf": {"versione": 3, "estraibile": true, "ocr": {"stato": "non_necessario"}}}`),
		"OCR spento":         json.RawMessage(`{"testo_pdf": {"versione": 3, "ocr": {"stato": "spento", "tentativi": [{"esito": "saltato"}]}}}`),
		"senza testo":        json.RawMessage(`{"errore_pdf": "rotto"}`),
	} {
		if TestoPDFAttendeOCR(d) {
			t.Errorf("%s: non aspetta l'OCR", nome)
		}
	}
}

// GP-3 (G-8.4). Il testo di un DWG R12: gli esiti chiusi sono quelli delle costanti Go; i campi del cartiglio hanno la
// sorgente nel file e le stesse voci del DXF; le tabelle hanno la forma di quelle dei PDF; un DWG non supportato e' una
// risposta (non si rilegge), un DWG analizzato prima (dettagli vuoti) si rilegge.
func TestTestoDWG(t *testing.T) {
	s := leggiSchema(t, "testo_dwg")
	got := formaSchema(proprieta(s)["esito"]).valori
	sort.Strings(got)
	want := []string{DWGLetto, DWGNonLetto, DWGNonSupportato}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("esito del DWG: lo schema dice %v, il Go conosce %v", got, want)
	}
	defs, _ := s["$defs"].(map[string]any)
	campo, _ := defs["CampoCartiglioDWG"].(map[string]any)
	got = formaSchema(proprieta(campo)["etichetta"]).valori
	sort.Strings(got)
	want = []string{CampoNumeroDisegno, CampoCodice, CampoRevisione, CampoTitolo, CampoScala, CampoMateriale,
		CampoParticolareSimile, CampoRiferimento3D}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("etichette del cartiglio del DWG: lo schema dice %v, il Go conosce %v", got, want)
	}
	for _, v := range []string{"codice", "codici", "testo", "frammenti"} {
		if _, c := proprieta(s)[v]; c {
			t.Errorf("TestoDWG dichiara %q: gli altri testi del disegno non viaggiano", v)
		}
	}
	letto := json.RawMessage(`{"testo_dwg": {"versione": 1, "esito": "letto", "versione_dwg": "AC1009", "codifica": "cp1252",
		"entita": 300, "testi": 120, "annidati_seguiti": 2, "parziale": false,
		"cartiglio": [{"etichetta": "numero_disegno", "letta": "DWG NO.", "valore": "ACME1001", "pagina": 1, "zona": "pagina",
			"fonte": "nativo", "riquadro": [1, 2, 3, 4], "sorgente": {"offset_entita": 6284, "sezione": "ENTITIES", "blocco": "",
			"tipo_entita": "TEXT", "offset_etichetta": 6200}},
			{"etichetta": "numero_disegno", "letta": "DWG NO.", "valore": "ACME2001", "pagina": 1, "zona": "pagina", "fonte": "nativo",
			"sorgente": {"offset_entita": 6400}}],
		"tabelle": [{"pagina": 1, "indice": 1, "verso": "sopra", "colonne": [{"indice": 0, "intestazione": "POS.", "ruolo": "posizione"}],
			"righe": [{"indice": 1, "celle": [{"colonna": 0, "intestazione": "POS.", "ruolo": "posizione", "valore": "1", "sorgente": [7000]}]}]}],
		"limiti": {"entita_max": 500000, "righe_max": 400}}}`)
	d, ok := DecodificaTestoDWG(letto)
	if !ok || d.Esito != DWGLetto || d.VersioneDWG != "AC1009" || len(d.Cartiglio) != 2 || d.AnnidatiSeguiti != 2 {
		t.Fatalf("DWG letto male: %v %+v", ok, d)
	}
	if c := d.Cartiglio[0]; c.Etichetta != CampoNumeroDisegno || c.Valore != "ACME1001" || c.Sorgente.OffsetEntita != 6284 ||
		c.Sorgente.OffsetEtichetta != 6200 || c.Sorgente.TipoEntita != "TEXT" || len(c.Riquadro) != 4 {
		t.Errorf("campo del DWG letto male: %+v", c)
	}
	if d.Cartiglio[1].Valore != "ACME2001" {
		t.Errorf("il secondo numero impilato si perde: %+v", d.Cartiglio)
	}
	if len(d.Tabelle) != 1 || d.Tabelle[0].Verso != VersoSopra || d.Tabelle[0].Righe[0].Celle[0].Sorgente[0] != 7000 {
		t.Errorf("tabella del DWG letta male: %+v", d.Tabelle)
	}
	if d.Limiti.EntitaMax != 500000 || TestoDWGDaRileggere(letto) {
		t.Errorf("limiti letti male o DWG di oggi riaccodato: %+v", d.Limiti)
	}
	ns := json.RawMessage(`{"testo_dwg": {"versione": 1, "esito": "non_supportato", "versione_dwg": "AC1015", "motivo": "DWG 2000"}}`)
	if d, ok := DecodificaTestoDWG(ns); !ok || d.Esito != DWGNonSupportato || d.VersioneDWG != "AC1015" || TestoDWGDaRileggere(ns) {
		t.Errorf("un DWG non supportato e' una risposta: %v %+v", ok, d)
	}
	for nome, f := range map[string]json.RawMessage{
		"DWG di prima": json.RawMessage(`{"esito": {"tipo_proposto": "disegno_2d"}}`),
		"versione 0":   json.RawMessage(`{"testo_dwg": {"versione": 0}}`),
		"vuoti":        json.RawMessage(``),
	} {
		if _, ok := DecodificaTestoDWG(f); ok || !TestoDWGDaRileggere(f) {
			t.Errorf("%s: va riletto", nome)
		}
	}
}

// GP-3 (G-8.2). Il foglio di calcolo: celle con il testo esatto (i due spazi restano), il tipo, la formula separata dal
// valore memorizzato, lo stile per indice (il rosso e l'apostrofo che portano uno stato), il collegamento ridotto. Le
// voci chiuse sono quelle delle costanti Go; un BIFF non supportato e' una risposta, non un fatto da rileggere; un
// foglio analizzato prima (dettagli vuoti) si rilegge.
func TestFoglioDati(t *testing.T) {
	s := leggiSchema(t, "foglio_dati")
	got := formaSchema(proprieta(s)["esito"]).valori
	sort.Strings(got)
	want := []string{FoglioLetto, FoglioNonLetto, FoglioNonSupportato, FoglioParziale}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("esito del foglio: lo schema dice %v, il Go conosce %v", got, want)
	}
	if d, _ := proprieta(s)["versione"]["default"].(float64); int(d) != VersioneFoglioDati {
		t.Errorf("versione del foglio: schema %v, Go %d", proprieta(s)["versione"]["default"], VersioneFoglioDati)
	}
	defs, _ := s["$defs"].(map[string]any)
	cella, _ := defs["CellaFoglio"].(map[string]any)
	for _, vietato := range []string{"codice", "codici", "famiglia", "obsoleto", "stato_riga"} {
		if _, c := proprieta(cella)[vietato]; c {
			t.Errorf("la cella dichiara %q: l'interpretazione e' del server", vietato)
		}
	}
	letto := json.RawMessage(`{"foglio_dati": {"versione": 1, "esito": "letto", "formato": "xlsx", "sha256": "ab12",
		"fogli": [{"indice": 0, "nome": "Sheet1", "stato": "visibile", "dimensione": "B1:K15", "celle": 3, "unite": ["A1:C1"]}],
		"celle": [{"foglio": 0, "ind": "D9", "r": 9, "c": 4, "tipo": "s", "grezzo": "1  ACME117Z", "stile": null},
		          {"foglio": 0, "ind": "E9", "r": 9, "c": 5, "tipo": "s", "grezzo": "ACME.0741GT", "stile": 0},
		          {"foglio": 0, "ind": "G9", "r": 9, "c": 7, "tipo": "n", "grezzo": "42", "formula": "SUM(A1:A2)", "da_formula": true,
		           "collegamento": {"tipo": "relativo", "nome_base": "ACME.pdf", "estensione": "pdf"}}],
		"stili": [{"colore_font": "FFFF0000", "apice": true}],
		"intestazioni_candidate": [{"foglio": 0, "riga": 2, "colonne": [{"c": 4, "ind": "D2", "testo": "SUPPLIER PN", "ruolo": "codice"}]}],
		"limiti": {"celle_max": 10000}, "campo_del_futuro": 1}}`)
	f, ok := DecodificaFoglioDati(letto)
	if !ok || f.Esito != FoglioLetto || len(f.Celle) != 3 || f.Celle[0].Grezzo != "1  ACME117Z" || f.Celle[0].Stile != nil {
		t.Fatalf("foglio letto male: %v %+v", ok, f)
	}
	if f.Celle[1].Stile == nil || *f.Celle[1].Stile != 0 || f.Stili[*f.Celle[1].Stile].ColoreFont != "FFFF0000" || !f.Stili[0].Apice {
		t.Errorf("lo stile per indice letto male: %+v %+v", f.Celle[1], f.Stili)
	}
	if !f.Celle[2].DaFormula || f.Celle[2].Formula != "SUM(A1:A2)" || f.Celle[2].Grezzo != "42" || f.Celle[2].Collegamento.NomeBase != "ACME.pdf" {
		t.Errorf("formula e collegamento letti male: %+v", f.Celle[2])
	}
	if f.IntestazioniCandidate[0].Colonne[0].Ruolo != RuoloCodice || f.Fogli[0].Unite[0] != "A1:C1" || f.Limiti.CelleMax != 10000 {
		t.Errorf("intestazioni, fogli o limiti letti male: %+v", f)
	}
	if FoglioDatiDaRileggere(letto) {
		t.Error("un foglio letto con la versione di oggi non si rilegge")
	}
	biff := json.RawMessage(`{"foglio_dati": {"versione": 1, "esito": "non_supportato", "formato": "xls_biff", "motivo": "BIFF"}}`)
	if f, ok := DecodificaFoglioDati(biff); !ok || f.Esito != FoglioNonSupportato || FoglioDatiDaRileggere(biff) {
		t.Errorf("un BIFF non supportato e' una risposta: %v %+v", ok, f)
	}
	for nome, d := range map[string]json.RawMessage{
		"foglio di prima": json.RawMessage(`{"esito": {"tipo_proposto": "commerciale"}}`),
		"versione 0":      json.RawMessage(`{"foglio_dati": {"versione": 0}}`),
		"nullo":           json.RawMessage(`{"foglio_dati": null}`),
		"vuoti":           json.RawMessage(``),
		"rotti":           json.RawMessage(`non json`),
	} {
		if _, ok := DecodificaFoglioDati(d); ok || !FoglioDatiDaRileggere(d) {
			t.Errorf("%s: va riletto, non decodificato", nome)
		}
	}
}
