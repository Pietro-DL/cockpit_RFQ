package valutazione

import (
	"bytes"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// VersioneCasi: la versione del file dei casi che LeggiIngressi sa leggere (6.6.3, «versione_casi»). Cambia con un
// commit che lo dichiara, e la prova che la fissa si riscrive.
const VersioneCasi = 1

// Ingressi: ciò che un caso dichiara e la fotografia non contiene (par.3.3.8; 6.6.3): i segmenti pertinenti dello
// scenario, la sua autorità, i messaggi fuori RFQ. Vengono dal file dei casi, mai dagli attesi (R2). Il banco lo
// trova nel manifest, l'anteprima attraverso l'indice delle regole, e i due sono lo stesso file (R29 a A, b C).
type Ingressi struct {
	Versione int            `json:"versione_casi"`
	Casi     []IngressoCaso `json:"casi"`
}

// IngressoCaso: un caso del file dei casi.
//   - ThreadID: la RFQ del caso; nil per i messaggi fuori RFQ (R34).
//   - Messaggi: i messaggi fuori RFQ del caso, o i messaggi della richiesta del thread; vuoto = tutti i messaggi
//     del thread.
//   - Segmenti: i segmenti dichiarati pertinenti, sempre con l'origine «scenario» (R29 b C; R48 A).
//   - Autorita: «scenario», o vuota. Con «scenario» i prodotti letti dalla mail nei segmenti del caso sono target
//     di scenario, mai conferme dell'operatore (R75 A).
type IngressoCaso struct {
	ID        string               `json:"id"`
	ThreadID  *uuid.UUID           `json:"thread_id,omitempty"`
	ClienteID uuid.UUID            `json:"cliente_id"`
	Messaggi  []uuid.UUID          `json:"messaggi,omitempty"`
	Segmenti  []SegmentoDichiarato `json:"segmenti,omitempty"`
	Autorita  ancoraggio.Autorita  `json:"autorita,omitempty"`
}

// SegmentoDichiarato: un segmento di un messaggio, con l'uso (pertinente | escluso | da_valutare), l'origine
// (sempre «scenario») e il motivo.
type SegmentoDichiarato struct {
	MessaggioID uuid.UUID `json:"messaggio_id"`
	SegmentoID  string    `json:"segmento_id"`
	Uso         string    `json:"uso"`
	Origine     string    `json:"origine"`
	Motivo      string    `json:"motivo,omitempty"`
}

// CasoDelThread: il caso del file per quel thread, se c'è (al più uno: LeggiIngressi rifiuta i doppioni).
func (in Ingressi) CasoDelThread(thread uuid.UUID) *IngressoCaso {
	for i := range in.Casi {
		if c := &in.Casi[i]; c.ThreadID != nil && *c.ThreadID == thread {
			return c
		}
	}
	return nil
}

// Gli usi e l'origine ammessi in un segmento dichiarato. L'origine è solo «scenario»: il file dei casi simula una
// scelta, e non può fingere quella dell'operatore né un riconoscimento (R75 A: «mai trasformati in conferme
// dell'operatore»).
var (
	usiDichiarati      = []string{"pertinente", "escluso", "da_valutare"}
	origineDichiarata  = "scenario"
	chiaveVersioneCasi = "versione_casi"
)

// LeggiIngressi: la decodifica stretta del file dei casi (6.6.3; par.3.4.3, regola 5; A1c-L1-21). Riceve i byte da
// chi chiama: niente file. Rifiuta:
//   - la BOM, l'UTF-8 non valido e i surrogati soli scritti in escape, il JSON malformato, il testo dopo l'oggetto;
//   - versione_casi assente o diversa da VersioneCasi: con una versione ignota non si guarda altro;
//   - le chiavi sconosciute, ripetute o con le maiuscole diverse dal tag, i null, i numeri non interi;
//   - un caso senza ID o con un ID ripetuto, senza cliente, con un thread ripetuto; un segmento senza messaggio o
//     senza ID, ripetuto, con un uso fuori elenco o un'origine diversa da «scenario»; un'autorità diversa da
//     «scenario».
//
// L'errore è un *evidenze.ErroreContratto con tutte le diagnostiche trovate, con i codici «contratto.*» che la
// grammatica dichiara per la stessa porta stretta; con un errore gli ingressi restituiti sono vuoti.
func LeggiIngressi(raw []byte) (Ingressi, error) {
	if d := decodificaStretta(raw); len(d) > 0 {
		return Ingressi{}, &evidenze.ErroreContratto{Diagnostiche: d}
	}
	var in Ingressi
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(&in); err != nil {
		return Ingressi{}, &evidenze.ErroreContratto{Diagnostiche: []evidenze.Diagnostica{{
			Codice:    grammatica.CodiceJSONNonValido,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Messaggio: "il file dei casi non si decodifica: " + err.Error(),
		}}}
	}
	if d := in.valida(); len(d) > 0 {
		return Ingressi{}, &evidenze.ErroreContratto{Diagnostiche: d}
	}
	return in, nil
}

// valida: i controlli del contenuto, dopo la decodifica stretta.
func (in Ingressi) valida() []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	// d porta solo il codice, scritto dal chiamante con la sua costante (A1a-CAT); qui il resto.
	errore := func(d evidenze.Diagnostica, percorso, messaggio string) {
		d.Gravita, d.Natura, d.Percorso, d.Messaggio = evidenze.GravitaErrore, evidenze.NaturaContratto, percorso, messaggio
		out = append(out, d)
	}
	ids := map[string]bool{}
	thread := map[uuid.UUID]bool{}
	for i, c := range in.Casi {
		p := fmt.Sprintf("casi[%d]", i)
		switch {
		case strings.TrimSpace(c.ID) == "":
			errore(evidenze.Diagnostica{Codice: grammatica.CodiceIDVuoto}, p+".id", "un caso senza ID")
		case ids[c.ID]:
			errore(evidenze.Diagnostica{Codice: grammatica.CodiceIDRipetuto}, p+".id", fmt.Sprintf("il caso %q compare due volte", c.ID))
		}
		ids[c.ID] = true
		if c.ClienteID == uuid.Nil {
			errore(evidenze.Diagnostica{Codice: grammatica.CodiceCampoObbligatorio}, p+".cliente_id", "manca l'UUID del cliente del caso")
		}
		if c.ThreadID != nil {
			if thread[*c.ThreadID] {
				errore(evidenze.Diagnostica{Codice: grammatica.CodiceIDRipetuto}, p+".thread_id", fmt.Sprintf("il thread %s ha due casi", *c.ThreadID))
			}
			thread[*c.ThreadID] = true
		}
		if c.Autorita != "" && c.Autorita != ancoraggio.AutoritaScenario {
			errore(evidenze.Diagnostica{Codice: grammatica.CodiceEnumIgnoto}, p+".autorita", fmt.Sprintf("autorità %q: il file dei casi dichiara solo «scenario» (R75 A)", c.Autorita))
		}
		visti := map[string]bool{}
		for j, s := range c.Segmenti {
			ps := fmt.Sprintf("%s.segmenti[%d]", p, j)
			if s.MessaggioID == uuid.Nil {
				errore(evidenze.Diagnostica{Codice: grammatica.CodiceCampoObbligatorio}, ps+".messaggio_id", "un segmento senza messaggio")
			}
			if strings.TrimSpace(s.SegmentoID) == "" {
				errore(evidenze.Diagnostica{Codice: grammatica.CodiceIDVuoto}, ps+".segmento_id", "un segmento senza ID")
			}
			k := s.MessaggioID.String() + "\x00" + s.SegmentoID
			if visti[k] {
				errore(evidenze.Diagnostica{Codice: grammatica.CodiceIDRipetuto}, ps, fmt.Sprintf("il segmento %q del messaggio %s è dichiarato due volte", s.SegmentoID, s.MessaggioID))
			}
			visti[k] = true
			if !contiene(usiDichiarati, s.Uso) {
				errore(evidenze.Diagnostica{Codice: grammatica.CodiceEnumIgnoto}, ps+".uso", fmt.Sprintf("uso %q fuori elenco (pertinente | escluso | da_valutare)", s.Uso))
			}
			if s.Origine != origineDichiarata {
				errore(evidenze.Diagnostica{Codice: grammatica.CodiceEnumIgnoto}, ps+".origine", fmt.Sprintf("origine %q: il file dei casi dichiara solo «scenario»", s.Origine))
			}
		}
	}
	return out
}

// ---- la porta stretta ----

// profonditaMassima: gli oggetti e gli array annidati che la lettura accetta; il file dei casi ne ha quattro.
const profonditaMassima = 16

// decodificaStretta controlla i byte prima di json.Decoder, che accetterebbe in silenzio una chiave con le maiuscole
// sbagliate, lascerebbe vincere un doppione, cambierebbe un surrogato solo in U+FFFD e leggerebbe un null come un
// valore assente (par.3.4.3, T21). Le chiavi ammesse le dice il tipo Ingressi, per riflessione. nil = letto.
func decodificaStretta(raw []byte) []evidenze.Diagnostica {
	// d porta solo il codice, scritto dal chiamante con la sua costante (A1a-CAT); qui il resto.
	contratto := func(d evidenze.Diagnostica, percorso, messaggio string) []evidenze.Diagnostica {
		d.Gravita, d.Natura, d.Percorso, d.Messaggio = evidenze.GravitaErrore, evidenze.NaturaContratto, percorso, messaggio
		return []evidenze.Diagnostica{d}
	}
	if bytes.HasPrefix(raw, []byte("\xef\xbb\xbf")) {
		return contratto(evidenze.Diagnostica{Codice: grammatica.CodiceBOM}, "", "il file comincia con la firma UTF-8 (BOM): va salvato in UTF-8 senza BOM")
	}
	if !utf8.Valid(raw) {
		return contratto(evidenze.Diagnostica{Codice: grammatica.CodiceUTF8NonValido}, "", "il file non è UTF-8 valido")
	}
	if surrogatoSolo(raw) {
		return contratto(evidenze.Diagnostica{Codice: grammatica.CodiceUTF8NonValido}, "", "un surrogato solo scritto in escape, che la decodifica cambierebbe in U+FFFD")
	}
	// Il primo valore soltanto: il testo dopo l'oggetto lo dice il passaggio a token, con il suo codice.
	var radice map[string]json.RawMessage
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&radice); err != nil || radice == nil {
		return contratto(evidenze.Diagnostica{Codice: grammatica.CodiceJSONNonValido}, "", "il file dei casi non è un oggetto JSON")
	}
	if v, ok := radice[chiaveVersioneCasi]; !ok || string(bytes.TrimSpace(v)) != strconv.Itoa(VersioneCasi) {
		return contratto(evidenze.Diagnostica{Codice: grammatica.CodiceVersioneSchemaIgnota}, chiaveVersioneCasi,
			fmt.Sprintf("versione_casi assente o diversa da %d: il file non si legge", VersioneCasi))
	}

	l := &lettoreStretto{dec: json.NewDecoder(bytes.NewReader(raw))}
	l.dec.UseNumber()
	if err := l.valore(reflect.TypeOf(Ingressi{}), "", 0); err != nil {
		return append(l.out, contratto(evidenze.Diagnostica{Codice: grammatica.CodiceJSONNonValido}, "", "JSON malformato: "+err.Error())...)
	}
	if _, err := l.dec.Token(); err != io.EOF {
		l.out = append(l.out, contratto(evidenze.Diagnostica{Codice: grammatica.CodiceValoreDopoOggetto}, "", "testo dopo l'oggetto: il file contiene un valore solo")...)
	}
	return l.out
}

// lettoreStretto: il passaggio a token, con le diagnostiche raccolte.
type lettoreStretto struct {
	dec *json.Decoder
	out []evidenze.Diagnostica
}

var tipoTextUnmarshaler = reflect.TypeFor[encoding.TextUnmarshaler]()

// diagnosi: d porta solo il codice, scritto dal chiamante con la sua costante (A1a-CAT); qui il resto.
func (l *lettoreStretto) diagnosi(d evidenze.Diagnostica, percorso, messaggio string) {
	d.Gravita, d.Natura, d.Percorso, d.Messaggio = evidenze.GravitaErrore, evidenze.NaturaContratto, percorso, messaggio
	l.out = append(l.out, d)
}

// valore legge un valore e controlla le sue chiavi contro t (nil = un valore sconosciuto, di cui si controllano
// solo i null e le chiavi ripetute).
func (l *lettoreStretto) valore(t reflect.Type, percorso string, prof int) error {
	if prof > profonditaMassima {
		return errors.New("annidamento troppo profondo")
	}
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	tok, err := l.dec.Token()
	if err != nil {
		return err
	}
	switch x := tok.(type) {
	case nil:
		l.diagnosi(evidenze.Diagnostica{Codice: grammatica.CodiceNull}, percorso, "un null: un campo assente si omette")
	case json.Number:
		if t != nil && t.Kind() >= reflect.Int && t.Kind() <= reflect.Uint64 && strings.ContainsAny(x.String(), ".eE") {
			l.diagnosi(evidenze.Diagnostica{Codice: grammatica.CodiceNumeroNonIntero}, percorso, fmt.Sprintf("il numero %s non è un intero", x))
		}
	case json.Delim:
		switch x {
		case '{':
			return l.oggetto(t, percorso, prof)
		case '[':
			var elem reflect.Type
			if t != nil && (t.Kind() == reflect.Slice || t.Kind() == reflect.Array) && !t.Implements(tipoTextUnmarshaler) &&
				!reflect.PointerTo(t).Implements(tipoTextUnmarshaler) {
				elem = t.Elem()
			}
			for i := 0; l.dec.More(); i++ {
				if err := l.valore(elem, fmt.Sprintf("%s[%d]", percorso, i), prof+1); err != nil {
					return err
				}
			}
			_, err := l.dec.Token()
			return err
		}
	}
	return nil
}

// oggetto legge le chiavi di un oggetto: ripetute, con le maiuscole diverse dal tag, sconosciute.
func (l *lettoreStretto) oggetto(t reflect.Type, percorso string, prof int) error {
	campi := map[string]reflect.Type{}
	struttura := t != nil && t.Kind() == reflect.Struct
	if struttura {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			nome, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if nome != "" && nome != "-" {
				campi[nome] = f.Type
			}
		}
	}
	visti := map[string]bool{}
	for l.dec.More() {
		tok, err := l.dec.Token()
		if err != nil {
			return err
		}
		chiave, _ := tok.(string)
		p := chiave
		if percorso != "" {
			p = percorso + "." + chiave
		}
		if visti[chiave] {
			l.diagnosi(evidenze.Diagnostica{Codice: grammatica.CodiceChiaveRipetuta}, p, fmt.Sprintf("la chiave %q compare due volte", chiave))
		}
		visti[chiave] = true
		var ft reflect.Type
		if struttura {
			var ok bool
			if ft, ok = campi[chiave]; !ok {
				simile := ""
				for nome := range campi {
					if strings.EqualFold(nome, chiave) {
						simile = nome
					}
				}
				if simile != "" {
					l.diagnosi(evidenze.Diagnostica{Codice: grammatica.CodiceChiaveMaiuscole}, p, fmt.Sprintf("la chiave %q va scritta %q", chiave, simile))
				} else {
					l.diagnosi(evidenze.Diagnostica{Codice: grammatica.CodiceChiaveSconosciuta}, p, fmt.Sprintf("la chiave %q non è del file dei casi", chiave))
				}
			}
		}
		if err := l.valore(ft, p, prof+1); err != nil {
			return err
		}
	}
	_, err := l.dec.Token()
	return err
}

// surrogatoSolo: c'è, dentro una stringa JSON, un «\uD800»-«\uDFFF» senza la sua metà. Si scorrono le stringhe con i
// loro escape, così «\\ud800» (una barra scritta, poi testo) non conta.
func surrogatoSolo(raw []byte) bool {
	dentro := false
	alto := false // l'ultimo escape era una metà alta, che aspetta la bassa subito dopo
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if !dentro {
			if c == '"' {
				dentro, alto = true, false
			}
			continue
		}
		switch {
		case c == '"':
			if alto {
				return true
			}
			dentro = false
		case c == '\\' && i+1 < len(raw):
			i++
			if raw[i] != 'u' {
				if alto {
					return true
				}
				continue
			}
			if i+4 >= len(raw) {
				return false
			}
			v, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 32)
			i += 4
			if err != nil {
				return false
			}
			switch {
			case v >= 0xD800 && v <= 0xDBFF:
				if alto {
					return true
				}
				alto = true
			case v >= 0xDC00 && v <= 0xDFFF:
				if !alto {
					return true
				}
				alto = false
			default:
				if alto {
					return true
				}
			}
		default:
			if alto {
				return true
			}
		}
	}
	return false
}

func contiene(elenco []string, s string) bool {
	for _, x := range elenco {
		if x == s {
			return true
		}
	}
	return false
}
