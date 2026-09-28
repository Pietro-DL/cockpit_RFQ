package fascicolo

// La differenza fra una versione congelata e la BOM working (addendum A4.7): per la schermata, per chi
// riesamina, e per l'abbandono di una bozza, che si puo' fare solo a differenza vuota.
//
// La query (DiffBomWorking) mette accanto, oggetto per oggetto, com'era nell'istantanea e com'e' nelle
// tabelle vive; il confronto lo fa Differenze, che e' pura. Conta solo quello che e' scritto: una
// deroga strutturale decaduta per una rianalisi non e' una differenza (la validita' si deriva), una
// deroga strutturale nuova si'.

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// Differenza e' un oggetto che non e' piu' com'era: aggiunto, tolto, oppure cambiato nei Campi.
type Differenza struct {
	Oggetto string // componente, arco, documento, deroga, deroga_struttura
	Chiave  string
	Tipo    string // aggiunto, tolto, cambiato
	Campi   []string
}

func (d Differenza) String() string {
	s := d.Oggetto + " " + d.Chiave + " " + d.Tipo
	if len(d.Campi) > 0 {
		s += " (" + strings.Join(d.Campi, ", ") + ")"
	}
	return s
}

// Differenze confronta le coppie della query. Una riga senza prima e' un oggetto aggiunto, una senza
// dopo un oggetto tolto; con tutte e due, i campi che non coincidono.
func Differenze(righe []db.DiffBomWorkingRow) ([]Differenza, error) {
	var out []Differenza
	for _, r := range righe {
		prima, err := oggetto(r.Prima)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", r.Oggetto, r.Chiave, err)
		}
		dopo, err := oggetto(r.Dopo)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", r.Oggetto, r.Chiave, err)
		}
		d := Differenza{Oggetto: r.Oggetto, Chiave: r.Chiave}
		switch {
		case prima == nil && dopo == nil:
			continue
		case prima == nil:
			d.Tipo = "aggiunto"
		case dopo == nil:
			d.Tipo = "tolto"
		default:
			d.Campi = campiDiversi(prima, dopo)
			if len(d.Campi) == 0 {
				continue
			}
			d.Tipo = "cambiato"
		}
		out = append(out, d)
	}
	return out, nil
}

// oggetto legge un lato della coppia: assente (NULL) oppure un oggetto JSON.
func oggetto(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func campiDiversi(a, b map[string]any) []string {
	var campi []string
	for k, v := range a {
		if w, ok := b[k]; !ok || !reflect.DeepEqual(v, w) {
			campi = append(campi, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			campi = append(campi, k)
		}
	}
	sort.Strings(campi)
	return campi
}

// DifferenzeWorking e' la differenza fra la versione (congelata) e la working della RFQ.
func DifferenzeWorking(ctx context.Context, q *db.Queries, thread, versione uuid.UUID) ([]Differenza, error) {
	righe, err := q.DiffBomWorking(ctx, db.DiffBomWorkingParams{BomVersioneID: versione, ThreadID: thread})
	if err != nil {
		return nil, err
	}
	return Differenze(righe)
}
