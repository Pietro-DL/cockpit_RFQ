package fascicolo

// Il componente nuovo scritto da una persona (Smistamento, decisione U4 del 27/09): nell'editor della
// struttura, «+ Componente con codice». E' l'unica strada con cui un pezzo che nessuno STEP propone entra
// nella BOM, e il codice lo scrive l'operatore: mai precompilato da un nome di file, da un cartiglio o da un
// codice trovato (R1). Nasce con origine `manuale`.
//
// L'identita' non cambia (U4): un pezzo e' una riga di componente per (thread, upper(codice)) (0018), e un
// pezzo con piu' padri ha piu' archi in componente_relazione, mai piu' righe. Per questo un codice scritto
// che la RFQ ha gia' non crea niente: ritrova QUEL componente, e l'editor lo collega dove l'operatore lo
// mette. Un codice quasi uguale a uno della RFQ (P4) vuole la conferma esplicita che e' un pezzo diverso.

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// TipiNuovo sono i tipi con cui nasce un componente scritto nell'editor: assieme o particolare. Un prodotto
// nasce dai codici della richiesta, un commerciale non si disegna nella struttura.
var TipiNuovo = []db.TipoComponente{db.TipoComponenteSottoassieme, db.TipoComponenteSciolto}

// EsitoCodiceNuovo e' quello che la RFQ sa del codice che l'operatore sta scrivendo.
type EsitoCodiceNuovo struct {
	Codice string
	// Errore: il codice non si puo' scrivere (vuoto, troppo lungo, con spazi o controlli).
	Errore string
	// Esistente: la RFQ ha gia' un componente con questo codice (attivo o archiviato). E' quel pezzo.
	Esistente *db.Componente
	// Richiesta: e' un codice della richiesta. Diventa prodotto con una decisione del triage
	// (AssicuraProdottiDellaRichiesta) o, se confermato con la BOM congelata, aprendo la revisione
	// (AssicuraProdottiDellaRevisione); non un componente dall'editor.
	Richiesta bool
	// Vicini: i codici della RFQ quasi uguali (P4). Il componente nasce solo se l'operatore dice che e' un
	// pezzo diverso.
	Vicini []Vicino
}

// ValutaCodiceNuovo guarda il codice scritto contro i componenti della RFQ (archiviati compresi) e i codici
// della richiesta. Pura.
func ValutaCodiceNuovo(codice string, componenti []db.Componente, richiesta []string, m *classificazione.Motore) EsitoCodiceNuovo {
	e := EsitoCodiceNuovo{Codice: strings.TrimSpace(codice)}
	if !classificazione.CodiceAmmissibile(e.Codice) {
		if e.Codice == "" {
			e.Errore = "il codice si scrive: è vuoto"
		} else {
			e.Errore = fmt.Sprintf("«%s»: il codice ha più di %d caratteri o caratteri non ammessi", e.Codice, classificazione.MaxCodice)
		}
		return e
	}
	up := strings.ToUpper(e.Codice)
	ordinati := append([]db.Componente(nil), componenti...)
	sort.Slice(ordinati, func(i, j int) bool { return ordinati[i].Codice < ordinati[j].Codice })
	codici := make([]string, 0, len(ordinati)+len(richiesta))
	for i, c := range ordinati {
		if strings.ToUpper(strings.TrimSpace(c.Codice)) == up {
			e.Esistente = &ordinati[i]
			return e
		}
		codici = append(codici, c.Codice)
	}
	for _, r := range richiesta {
		if strings.ToUpper(strings.TrimSpace(r)) == up {
			e.Richiesta = true
			return e
		}
	}
	e.Vicini = Vicini(e.Codice, append(codici, richiesta...), m)
	return e
}

// ControllaCodiceNuovo e' ValutaCodiceNuovo sui dati della RFQ, per l'editor che chiede prima di mettere la
// carta. Non scrive niente: la stessa valutazione si ripete, sotto il lucchetto, alla conferma.
func ControllaCodiceNuovo(ctx context.Context, q *db.Queries, thread uuid.UUID, codice string) (EsitoCodiceNuovo, error) {
	comp, err := q.ListComponentiThread(ctx, thread)
	if err != nil {
		return EsitoCodiceNuovo{}, err
	}
	richiesta, err := codiciDellaRichiesta(ctx, q, thread)
	if err != nil {
		return EsitoCodiceNuovo{}, err
	}
	m, err := MotoreDellaRfq(ctx, q, thread)
	if err != nil {
		return EsitoCodiceNuovo{}, err
	}
	return ValutaCodiceNuovo(codice, comp, richiesta, m), nil
}

// codiciDellaRichiesta sono gli identificativi della RFQ, come stringhe.
func codiciDellaRichiesta(ctx context.Context, q *db.Queries, thread uuid.UUID) ([]string, error) {
	ids, err := q.ListIdentificativi(ctx, thread)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(ids))
	for i, x := range ids {
		out[i] = x.Codice
	}
	return out, nil
}

// codiciVicini e' l'elenco dei vicini per un rifiuto: «7120001 (lettera finale)».
func codiciVicini(v []Vicino) string {
	parti := make([]string, len(v))
	for i, x := range v {
		parti[i] = x.Codice + " (" + x.Motivo + ")"
	}
	return strings.Join(parti, ", ")
}
