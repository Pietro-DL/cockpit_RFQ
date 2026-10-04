package bancoa

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Questo è l'unico file del modulo che importa la libreria YAML (G3; R40 e). È gopkg.in/yaml.v3 v3.0.1, il
// ripiego offline di R9: go.yaml.in/yaml/v3 non è nella cache dei moduli, e le due hanno la stessa API.

// Attesi: gli attesi della consegna A, nella parte che A1a legge: la testata e i casi_contratto. Le altre
// sezioni le leggerà A1c; qui sono note, quindi non sono chiavi sconosciute, ma non si interpretano.
type Attesi struct {
	Testata Testata
	Casi    []CasoContratto // nell'ordine del file
}

// Testata: le chiavi di primo livello che descrivono il file. È l'hash del file, nel manifest, a fare da
// versione (par.3.7.2): la testata si riporta nel rapporto e non decide niente.
type Testata struct {
	VersioneAttesi                                               int
	Data, Aggiornato, Stato, Piano, BaseRepository, Riservatezza string
}

// CasoContratto: un caso di casi_contratto. ID e profilo sono dati privati: compaiono solo nei rapporti del
// banco, mai nel codice né nei nomi delle prove (R47 c). Il profilo si lega al cliente nel manifest (D-09).
type CasoContratto struct {
	ID, Profilo, Livello string
	StatoAtteso          string // definito | riservato
	Decisione, DipendeDa string // metadati: un caso definito con dipende_da si valuta, e il rapporto lo annota
	OrigineFixture       string
	AllegatoID           string // lega il caso a un allegato: serve in A1c
	Nota                 string
	Contesto, Testo      string         // l'ingresso; il contesto nella grafia degli attesi (R19)
	Precondizioni        *Precondizioni // facoltative
	Atteso               []ChiaveAttesa // in ordine di chiave
}

// Gli stati attesi di un caso.
const (
	StatoAttesoDefinito  = "definito"
	StatoAttesoRiservato = "riservato"
)

// Precondizioni: che cosa il caso presuppone. In A1a le hanno solo casi sull'attributo della revisione in
// campo separato, che si valutano in A1b (A-C02).
type Precondizioni struct {
	BaseStrutturata          string
	EntitaCondivisaConCodice bool
}

// ChiaveAttesa: una chiave di atteso con il suo valore, come è scritto.
type ChiaveAttesa struct {
	Chiave string
	Valore ValoreAtteso
}

// ValoreAtteso: il valore di una chiave degli attesi. Il testo è quello scritto nel file, senza conversioni:
// una revisione «00» resta «00» anche se il YAML la risolverebbe come un intero (D4). Tipo: stringa |
// intero | booleano | nullo | lista | mappa. Una lista ha solo elementi semplici; una mappa non si confronta
// in A1a.
type ValoreAtteso struct {
	Tipo     string
	Testo    string
	Elementi []string
}

// I tipi di un valore atteso.
const (
	TipoStringa  = "stringa"
	TipoIntero   = "intero"
	TipoBooleano = "booleano"
	TipoNullo    = "nullo"
	TipoLista    = "lista"
	TipoMappa    = "mappa"
)

// String: il valore come si scrive nel rapporto.
func (v ValoreAtteso) String() string {
	switch v.Tipo {
	case TipoNullo:
		return "null"
	case TipoLista:
		return "[" + strings.Join(v.Elementi, ", ") + "]"
	case TipoMappa:
		return "{…}"
	}
	return v.Testo
}

// attesiYAML: il file come lo legge il decoder, con KnownFields: una chiave che non è un tag è un errore. Le
// sezioni che A1a non legge sono yaml.Node: si accettano senza interpretarle.
type attesiYAML struct {
	VersioneAttesi        *int       `yaml:"versione_attesi"`
	Data                  string     `yaml:"data"`
	Aggiornato            string     `yaml:"aggiornato"`
	Stato                 string     `yaml:"stato"`
	Piano                 string     `yaml:"piano"`
	BaseRepository        string     `yaml:"base_repository"`
	Riservatezza          string     `yaml:"riservatezza"`
	CasiContratto         []casoYAML `yaml:"casi_contratto"`
	ContrattoRunner       yaml.Node  `yaml:"contratto_runner"`
	Fonti                 yaml.Node  `yaml:"fonti"`
	Profili               yaml.Node  `yaml:"profili"`
	ScenarioInoltro       yaml.Node  `yaml:"scenario_inoltro"`
	BaselineDecisioni     yaml.Node  `yaml:"baseline_decisioni"`
	RealiArchivi          yaml.Node  `yaml:"reali_archivi"`
	CasiIntegrazione      yaml.Node  `yaml:"casi_integrazione"`
	RiservatiNonBloccanti yaml.Node  `yaml:"riservati_non_bloccanti"`
	PrerequisitiGateL4    yaml.Node  `yaml:"prerequisiti_gate_l4"`
	Gate                  yaml.Node  `yaml:"gate"`
	DecisioniSuccessive   yaml.Node  `yaml:"decisioni_successive_da_rivedere"`
}

type casoYAML struct {
	ID             string             `yaml:"id"`
	Profilo        string             `yaml:"profilo"`
	Livello        string             `yaml:"livello"`
	StatoAtteso    string             `yaml:"stato_atteso"`
	Decisione      string             `yaml:"decisione"`
	DipendeDa      string             `yaml:"dipende_da"`
	OrigineFixture string             `yaml:"origine_fixture"`
	AllegatoID     string             `yaml:"allegato_id"`
	Nota           string             `yaml:"nota"`
	Input          *inputYAML         `yaml:"input"`
	Precondizioni  *precondizioniYAML `yaml:"precondizioni"`
	Atteso         yaml.Node          `yaml:"atteso"`
}

type inputYAML struct {
	Contesto *string `yaml:"contesto"`
	Testo    *string `yaml:"testo"`
}

type precondizioniYAML struct {
	BaseStrutturata          string `yaml:"base_strutturata"`
	EntitaCondivisaConCodice bool   `yaml:"entita_condivisa_con_codice"`
}

// LeggiAttesi: la decodifica degli attesi YAML. In A1a legge la testata e i casi_contratto; le altre sezioni
// le aggiunge A1c. Una chiave sconosciuta (di primo livello, in un caso o nel suo atteso) è un errore: una
// chiave nuova negli attesi non si ignora. Sono errori anche una chiave ripetuta (la rifiuta la libreria), un
// secondo documento nel file, un caso senza id, profilo, contesto o testo, un id ripetuto, uno stato atteso
// fuori elenco. Il codice conosce solo i nomi neutri delle chiavi (R47 a): nessun nome di cliente sta qui.
// L'errore dice il caso e la chiave, mai il testo dell'ingresso.
func LeggiAttesi(raw []byte) (Attesi, error) {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var f attesiYAML
	if err := dec.Decode(&f); err != nil {
		if errors.Is(err, io.EOF) {
			return Attesi{}, errors.New("attesi: il file è vuoto")
		}
		return Attesi{}, fmt.Errorf("attesi: %w", err)
	}
	var altro yaml.Node
	if err := dec.Decode(&altro); !errors.Is(err, io.EOF) {
		return Attesi{}, errors.New("attesi: il file contiene più di un documento YAML")
	}
	if f.VersioneAttesi == nil {
		return Attesi{}, errors.New("attesi: manca versione_attesi")
	}
	if f.CasiContratto == nil {
		return Attesi{}, errors.New("attesi: manca casi_contratto")
	}
	// Casi non è mai nil dopo la lettura, nemmeno con zero casi: nil vuol dire «attesi non letti» (VerificaRegole).
	a := Attesi{Casi: []CasoContratto{}, Testata: Testata{
		VersioneAttesi: *f.VersioneAttesi, Data: f.Data, Aggiornato: f.Aggiornato, Stato: f.Stato,
		Piano: f.Piano, BaseRepository: f.BaseRepository, Riservatezza: f.Riservatezza,
	}}
	visti := map[string]bool{}
	for i, c := range f.CasiContratto {
		k, err := caso(i, c)
		if err != nil {
			return Attesi{}, err
		}
		if visti[k.ID] {
			return Attesi{}, fmt.Errorf("attesi: casi_contratto[%d]: l'id %q compare due volte", i, k.ID)
		}
		visti[k.ID] = true
		a.Casi = append(a.Casi, k)
	}
	return a, nil
}

func caso(i int, c casoYAML) (CasoContratto, error) {
	p := fmt.Sprintf("attesi: casi_contratto[%d]", i)
	switch {
	case strings.TrimSpace(c.ID) == "":
		return CasoContratto{}, fmt.Errorf("%s: manca id", p)
	case strings.TrimSpace(c.Profilo) == "":
		return CasoContratto{}, fmt.Errorf("%s (%s): manca profilo", p, c.ID)
	case c.StatoAtteso != StatoAttesoDefinito && c.StatoAtteso != StatoAttesoRiservato:
		return CasoContratto{}, fmt.Errorf("%s (%s): stato_atteso %q fuori elenco (%s, %s)", p, c.ID, c.StatoAtteso, StatoAttesoDefinito, StatoAttesoRiservato)
	case c.Input == nil || c.Input.Contesto == nil || c.Input.Testo == nil:
		return CasoContratto{}, fmt.Errorf("%s (%s): input senza contesto o senza testo", p, c.ID)
	}
	k := CasoContratto{
		ID: c.ID, Profilo: c.Profilo, Livello: c.Livello, StatoAtteso: c.StatoAtteso,
		Decisione: c.Decisione, DipendeDa: c.DipendeDa, OrigineFixture: c.OrigineFixture,
		AllegatoID: c.AllegatoID, Nota: c.Nota,
		Contesto: *c.Input.Contesto, Testo: *c.Input.Testo,
	}
	if c.Precondizioni != nil {
		k.Precondizioni = &Precondizioni{
			BaseStrutturata:          c.Precondizioni.BaseStrutturata,
			EntitaCondivisaConCodice: c.Precondizioni.EntitaCondivisaConCodice,
		}
	}
	switch c.Atteso.Kind {
	case 0:
		return CasoContratto{}, fmt.Errorf("%s (%s): manca atteso", p, c.ID)
	case yaml.MappingNode:
	default:
		return CasoContratto{}, fmt.Errorf("%s (%s): atteso deve essere una mappa", p, c.ID)
	}
	viste := map[string]bool{}
	for j := 0; j+1 < len(c.Atteso.Content); j += 2 {
		chiave := c.Atteso.Content[j].Value
		if viste[chiave] {
			// sotto un yaml.Node la libreria non controlla i doppioni: lo si fa qui
			return CasoContratto{}, fmt.Errorf("%s (%s): atteso.%s: chiave ripetuta", p, c.ID, chiave)
		}
		viste[chiave] = true
		if !ChiaveNota(chiave) {
			return CasoContratto{}, fmt.Errorf("%s (%s): atteso.%s: chiave sconosciuta (una chiave nuova negli attesi non si ignora: va aggiunta alla tabella di traduzione.go)", p, c.ID, chiave)
		}
		k.Atteso = append(k.Atteso, ChiaveAttesa{Chiave: chiave, Valore: valoreDi(c.Atteso.Content[j+1])})
	}
	sort.SliceStable(k.Atteso, func(x, y int) bool { return k.Atteso[x].Chiave < k.Atteso[y].Chiave })
	return k, nil
}

// valoreDi: il valore di un nodo, con il testo scritto nel file. Un alias si segue.
func valoreDi(n *yaml.Node) ValoreAtteso {
	if n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	switch n.Kind {
	case yaml.ScalarNode:
		switch n.ShortTag() {
		case "!!null":
			return ValoreAtteso{Tipo: TipoNullo}
		case "!!int":
			return ValoreAtteso{Tipo: TipoIntero, Testo: n.Value}
		case "!!bool":
			return ValoreAtteso{Tipo: TipoBooleano, Testo: strings.ToLower(n.Value)}
		}
		// stringhe, e tutto ciò che il YAML risolverebbe in altro (numeri con decimali, date): il testo com'è
		return ValoreAtteso{Tipo: TipoStringa, Testo: n.Value}
	case yaml.SequenceNode:
		v := ValoreAtteso{Tipo: TipoLista}
		for _, e := range n.Content {
			if e.Kind == yaml.AliasNode && e.Alias != nil {
				e = e.Alias
			}
			if e.Kind != yaml.ScalarNode {
				return ValoreAtteso{Tipo: TipoMappa}
			}
			v.Elementi = append(v.Elementi, e.Value)
		}
		return v
	}
	return ValoreAtteso{Tipo: TipoMappa}
}
