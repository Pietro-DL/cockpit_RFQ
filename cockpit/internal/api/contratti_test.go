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
package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/db"
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
}

const cartellaContratti = "../../contracts"

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
