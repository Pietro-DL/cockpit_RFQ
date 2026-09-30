package fascicolo

// Il riepilogo dell'albero proposto (giro 4, fase 4.4a.1a; domande 27 = A, 28 = A, 29b = A, 30 seconda risposta; studio
// docs/specs/studio_albero_distinta_29-09.md § 2.5 e § 2.8, con la verifica in coda). E' la bozza dell'operatore — le correzioni
// che fa all'albero proposto prima di confermarlo — letta contro l'albero di adesso e detta riga per riga. E' una
// LETTURA: qui niente scrive. La conferma (fase 4.4a.1b, albero_conferma.go) ricalcola lo stesso riepilogo sotto il
// lucchetto della RFQ e scrive solo se la sua firma torna, e solo se e' confermabile; scrive quello che il riepilogo
// dice, con gli stessi conti (pianoConferma).
//
// La bozza ha un formato con il suo numero (FormatoBozza): e' lo stesso JSON che la pagina tiene nel browser (28 = A, il
// primo tempo), che la conferma riceve e che la tabella della fase 4.4b salvera'. I nodi si nominano con le chiavi
// dell'albero («cod:<CODICE>», «nodo:<riga>» per un nodo senza codice), i pezzi aggiunti con la loro («nuovo:<n>»). La
// bozza dice soltanto che cosa cambia: un albero accettato cosi' com'e' e' la bozza vuota. Il particolare commerciale
// scelto con la tendina per un nodo con la proposta commerciale e' il ✓ della proposta (vedi BozzaAlbero.Tipi).
//
// La cascata (29b = A): togliere un nodo toglie i legami che lo toccano; poi resta solo quello che si raggiunge ancora da
// un prodotto. Un figlio in comune resta se un altro padre lo tiene; un pezzo che nessuno tiene piu' va via con i suoi
// figli che erano solo suoi. Un componente che resterebbe senza padri si archivia se ha documenti o storia (una riga
// fuori dalla working lo tiene: ListComponentiConStoria), altrimenti si elimina: il riepilogo lo dice pezzo per pezzo.
// Un prodotto non si toglie dall'albero: viene dal triage.
//
// Le domande aperte fermano la conferma: una proposta commerciale senza ✓ o ✗ (domanda 30, seconda risposta), un pezzo
// nuovo con dei codici quasi uguali senza il «diverso» di una persona (P4), una quantita' discorde senza la scelta
// (6c), un nodo senza codice. Il riepilogo le elenca in Blocchi, e la conferma non si fara' finche' ce n'e' una.
// Le quantita' discordi fra STEP e disegno arrivano con la fase 4.12: oggi le discordi sono solo fra due file STEP.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/coda"
	"promatec/cockpit/internal/platform/db"
)

// FormatoBozza e' il numero del formato della bozza dell'albero. Un formato diverso si rifiuta: la pagina che manda
// una bozza di un altro formato deve essere aggiornata.
const FormatoBozza = 1

// MaxVociBozza e' quante voci puo' avere ogni lista della bozza: oltre, e' un errore del programma che la manda.
const MaxVociBozza = MaxArchiVoluti

// BozzaAlbero e' la bozza dell'albero: che cosa l'operatore cambia dell'albero proposto.
type BozzaAlbero struct {
	Formato int `json:"formato"`
	// Base: la firma dell'albero su cui la bozza e' stata disegnata. Se l'albero nel frattempo e' cambiato, la bozza
	// e' vecchia: il riepilogo la legge lo stesso, ma non e' confermabile finche' non la si rivede.
	Base     string          `json:"base"`
	Rinomine []RinominaBozza `json:"rinomine,omitempty"`
	Tolti    []ToltoBozza    `json:"tolti,omitempty"`
	Aggiunti []AggiuntoBozza `json:"aggiunti,omitempty"`
	// Legami: un legame in piu' fra due pezzi dell'albero («anche sotto»).
	Legami []LegameBozza `json:"legami,omitempty"`
	// Quantita: la quantita' scelta di un legame (anche di uno discorde).
	Quantita []LegameBozza `json:"quantita,omitempty"`
	// Tipi: il tipo scelto con la tendina. Scegliere «particolare commerciale» per un nodo con la proposta commerciale
	// e' il ✓ di quella proposta (fase 4.4a.1b, dalla verifica della 4.4a.1a): e' un gesto esplicito della persona, come
	// il clic sul ✓, e la conferma registra chi l'ha fatto. Un altro tipo scelto non e' il ✗: la risposta resta da dare.
	Tipi []TipoBozza `json:"tipi,omitempty"`
	// Commerciali: il ✓ o il ✗ di una persona sulle proposte commerciali (domanda 30, seconda risposta).
	Commerciali []RispostaBozza `json:"commerciali,omitempty"`
	// Diversi: i pezzi nuovi che una persona dice diversi dai loro codici quasi uguali (P4).
	Diversi []string `json:"diversi,omitempty"`
}

// RinominaBozza e' il codice (e la revisione) che l'operatore scrive per un nodo proposto.
type RinominaBozza struct {
	Nodo   string `json:"nodo"`
	Codice string `json:"codice"`
	Rev    string `json:"rev,omitempty"`
}

// ToltoBozza toglie un nodo dall'albero (Padre vuoto: da tutti i padri, con la cascata) o un legame solo («togli da
// qui»: il nodo sotto quel padre).
type ToltoBozza struct {
	Nodo  string `json:"nodo"`
	Padre string `json:"padre,omitempty"`
}

// AggiuntoBozza e' un pezzo che l'operatore aggiunge, con il codice che scrive, il tipo e il padre.
type AggiuntoBozza struct {
	ID     string            `json:"id"` // «nuovo:<n>»
	Codice string            `json:"codice"`
	Rev    string            `json:"rev,omitempty"`
	Tipo   db.TipoComponente `json:"tipo"`
	Padre  string            `json:"padre"`
	Qta    int32             `json:"qta"`
}

// LegameBozza e' un legame fra due nodi, con la quantita'.
type LegameBozza struct {
	Padre  string `json:"padre"`
	Figlio string `json:"figlio"`
	Qta    int32  `json:"qta"`
}

// TipoBozza e' il tipo che l'operatore sceglie per un nodo.
type TipoBozza struct {
	Nodo string            `json:"nodo"`
	Tipo db.TipoComponente `json:"tipo"`
}

// Le risposte a una proposta commerciale.
const (
	RispostaSi = "si" // ✓: e' un particolare commerciale
	RispostaNo = "no" // ✗: non lo e'
)

// RispostaBozza e' il ✓ o il ✗ di una persona sulla proposta commerciale di un nodo.
type RispostaBozza struct {
	Nodo     string `json:"nodo"`
	Risposta string `json:"risposta"`
}

// LeggiBozza legge il JSON di una bozza: il formato, e nient'altro che i campi del formato. I riferimenti li controlla
// il riepilogo, contro l'albero.
func LeggiBozza(raw []byte) (BozzaAlbero, error) {
	var b BozzaAlbero
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return b, Rifiuto("la bozza dell'albero non si legge: ricarica la pagina")
	}
	if dec.More() {
		return b, Rifiuto("la bozza dell'albero non si legge: ricarica la pagina")
	}
	if b.Formato != FormatoBozza {
		return b, Rifiuto(fmt.Sprintf("la bozza ha il formato %d, e qui si legge il %d: ricarica la pagina", b.Formato, FormatoBozza))
	}
	return b, nil
}

// ------------------------------------------------------------------ il riepilogo

// RiepilogoAlbero e' quello che la conferma dell'albero farebbe, detto prima: i pezzi che nascono e quelli che si
// ritrovano, i tipi, i legami nuovi e tolti con la cascata, le rimozioni, le domande aperte. Firma: la firma di questo
// riepilogo (l'albero, la working, la bozza, la storia dei componenti), che la conferma rimanda.
type RiepilogoAlbero struct {
	Formato     int    `json:"formato"`
	AlberoFirma string `json:"albero_firma"`
	Firma       string `json:"firma"`
	// Vecchia: la bozza e' stata disegnata su un albero diverso da quello di adesso.
	Vecchia          bool                   `json:"vecchia,omitempty"`
	Nuovi            []PezzoRiepilogo       `json:"nuovi"`
	Ritrovati        []RitrovatoRiepilogo   `json:"ritrovati"`
	Tipi             []TipoRiepilogo        `json:"tipi"`
	Revisioni        []RevisioneRiepilogo   `json:"revisioni"`
	LegamiNuovi      []LegameRiepilogo      `json:"legami_nuovi"`
	LegamiTolti      []LegameRiepilogo      `json:"legami_tolti"`
	Quantita         []LegameRiepilogo      `json:"quantita"`
	Cascata          []CascataRiepilogo     `json:"cascata"`
	Fuori            []FuoriRiepilogo       `json:"fuori"`
	Scartate         []ScartataRiepilogo    `json:"scartate"`
	Rimozioni        []RimozioneRiepilogo   `json:"rimozioni"`
	Commerciali      []CommercialeRiepilogo `json:"commerciali"`
	Vicini           []ViciniRiepilogo      `json:"vicini"`
	QuantitaDiscordi []DiscordeRiepilogo    `json:"quantita_discordi"`
	SenzaCodice      []string               `json:"senza_codice"`
	// Blocchi: perche' la conferma non si puo' fare (le domande aperte). Vuoto: Confermabile.
	Blocchi      []string `json:"blocchi"`
	Confermabile bool     `json:"confermabile"`
}

// PezzoRiepilogo e' un pezzo che nasce con la conferma, con il codice e la sua fonte.
type PezzoRiepilogo struct {
	Nodo   string            `json:"nodo"`
	Codice string            `json:"codice"`
	Rev    string            `json:"rev,omitempty"`
	Tipo   db.TipoComponente `json:"tipo"`
	// Fonte: da dove viene il codice (step, operatore: dalla riga dello STEP; scritto: nella bozza; richiesta).
	Fonte   string   `json:"fonte"`
	Origine string   `json:"origine,omitempty"` // come la lettura della riga l'ha trovato (famiglia, generico…)
	Padri   []string `json:"padri"`
	// Commerciale: una persona ha risposto ✓ alla proposta commerciale: il pezzo nasce particolare commerciale per la
	// sua risposta (il Tipo resta quello proposto dall'albero: il riepilogo non sceglie un tipo).
	Commerciale bool `json:"commerciale,omitempty"`
}

// RitrovatoRiepilogo e' un pezzo dell'albero legato a un componente che c'e' per lo stesso codice (P13).
type RitrovatoRiepilogo struct {
	Nodo       string    `json:"nodo"`
	Codice     string    `json:"codice"`
	Componente uuid.UUID `json:"componente"`
	Archiviato bool      `json:"archiviato,omitempty"` // la conferma lo ripristina
	Perche     string    `json:"perche"`
}

// TipoRiepilogo e' un tipo che cambia.
type TipoRiepilogo struct {
	Nodo   string            `json:"nodo"`
	Codice string            `json:"codice"`
	Da     db.TipoComponente `json:"da"`
	A      db.TipoComponente `json:"a"`
	Motivo string            `json:"motivo"`
	// Effetti e Spento: di un componente che c'e', che cosa il cambio fa oltre al tipo (le autorizzazioni che si
	// sospendono, le deleghe con loro, le rimozioni che si chiudono, lo STEP strutturale che si svuota) e perche' non si
	// puo' fare, con le regole e le parole dell'anteprima della scheda (EffettoCambioTipo), calcolati su come la
	// conferma trovera' il componente (comeDopoLaConferma). Uno Spento e' un blocco del riepilogo, e tutti e due entrano
	// nella firma (fase 4.4a.1b, dalla verifica: prima si scoprivano solo alla conferma). Li calcola LeggiRiepilogo,
	// che legge il database; Riepilogo, pura, li lascia vuoti.
	Effetti []string `json:"effetti,omitempty"`
	Spento  string   `json:"spento,omitempty"`
}

// RevisioneRiepilogo e' la revisione di un componente che c'e', che le righe ancora da decidere dei file dicono nuova:
// «7121003: rev A → rev B» (fase 4.4a.1b; risposta 28: una revisione aggiorna lo stesso pezzo, non ne crea uno nuovo).
// Con i file che dicono revisioni diverse (Discordi) non cambia niente: resta Da.
type RevisioneRiepilogo struct {
	Nodo       string    `json:"nodo"`
	Codice     string    `json:"codice"`
	Componente uuid.UUID `json:"componente"`
	Da         string    `json:"da"`
	A          string    `json:"a,omitempty"`
	Discordi   []string  `json:"discordi,omitempty"`
}

// LegameRiepilogo e' un legame, padre → figlio × quantita', con i file che lo dicono o il perche'.
type LegameRiepilogo struct {
	Padre  string   `json:"padre"`
	Figlio string   `json:"figlio"`
	Qta    int32    `json:"qta"`
	Prima  int32    `json:"prima,omitempty"` // la quantita' della working, per una quantita' che cambia
	File   []string `json:"file,omitempty"`
	Motivo string   `json:"motivo,omitempty"`
}

// CascataRiepilogo e' che cosa porta via un «togli» della bozza: i pezzi che vanno via, e quelli che restano sotto
// altri padri.
type CascataRiepilogo struct {
	Nodo    string           `json:"nodo"`
	Codice  string           `json:"codice"`
	Padre   string           `json:"padre,omitempty"` // «togli da qui»: solo il legame con questo padre
	Vanno   []string         `json:"vanno"`
	Restano []RestaRiepilogo `json:"restano"`
}

// RestaRiepilogo e' un pezzo che resta, con i padri che lo tengono.
type RestaRiepilogo struct {
	Codice string   `json:"codice"`
	Padri  []string `json:"padri"`
}

// Che cosa succede a un componente che resta senza padri.
const (
	FuoriArchivia = "si_archivia"
	FuoriElimina  = "si_elimina"
)

// FuoriRiepilogo e' un componente che c'e' e che, con la conferma, resterebbe senza padri: si archivia se una riga
// fuori dalla working lo tiene (documenti o storia), si elimina se no.
type FuoriRiepilogo struct {
	Codice     string    `json:"codice"`
	Componente uuid.UUID `json:"componente"`
	Esito      string    `json:"esito"`
	Perche     []string  `json:"perche,omitempty"`
}

// ScartataRiepilogo e' un pezzo proposto che la bozza toglie: le sue righe dello STEP si chiudono, «scartate».
type ScartataRiepilogo struct {
	Nodo   string `json:"nodo"`
	Codice string `json:"codice"`
	Righe  int    `json:"righe"`
}

// Lo stato di una rimozione proposta dallo STEP, con la bozza.
const (
	RimozioneTolta  = "tolta"  // la bozza toglie il legame: la rimozione si accetta
	RimozioneTenuta = "tenuta" // il legame resta: la proposta si chiude, «tenuto nella struttura confermata»
)

// RimozioneRiepilogo e' una rimozione proposta dallo STEP autorizzato su un legame della working.
type RimozioneRiepilogo struct {
	Padre  string    `json:"padre"`
	Figlio string    `json:"figlio"`
	Step   uuid.UUID `json:"step"`
	Stato  string    `json:"stato"`
}

// Lo stato di una proposta commerciale, con la bozza.
const (
	CommercialeSi       = "si"       // ✓
	CommercialeNo       = "no"       // ✗
	CommercialeAperta   = "aperta"   // senza risposta: la conferma non si fa
	CommercialeDecaduta = "decaduta" // il nodo non e' piu' una foglia che nasce (tolto, rinominato, con dei figli)
)

// CommercialeRiepilogo e' una proposta commerciale, una per riga, con lo stato. Tendina: il ✓ e' il particolare
// commerciale scelto con la tendina (vedi BozzaAlbero.Tipi).
type CommercialeRiepilogo struct {
	Nodo    string `json:"nodo"`
	Codice  string `json:"codice"`
	Motivo  string `json:"motivo"`
	Stato   string `json:"stato"`
	Tendina bool   `json:"tendina,omitempty"`
}

// ViciniRiepilogo e' un pezzo nuovo con dei codici quasi uguali (P4): Diverso solo se una persona l'ha detto.
type ViciniRiepilogo struct {
	Nodo    string   `json:"nodo"`
	Codice  string   `json:"codice"`
	Vicini  []Vicino `json:"vicini"`
	Diverso bool     `json:"diverso"`
}

// DiscordeRiepilogo e' un legame con le quantita' discordi: Scelta e' quella della bozza (0: nessuna, la conferma non si
// fa).
type DiscordeRiepilogo struct {
	Padre  string      `json:"padre"`
	Figlio string      `json:"figlio"`
	Fonti  []FonteArco `json:"fonti"`
	Scelta int32       `json:"scelta"`
}

// ContestoRiepilogo e' quello che il riepilogo legge oltre all'albero: le regole del cliente (per il codice canonico di
// un codice scritto), i componenti e gli archi della working, i codici della richiesta e la storia dei componenti.
type ContestoRiepilogo struct {
	Motore     *classificazione.Motore
	Componenti []db.Componente
	Relazioni  []db.ComponenteRelazione
	Richiesta  []string
	// Storia: componente → che cosa lo tiene fuori dalla working (ha dei documenti, delle proposte…). Un componente con
	// una storia non si elimina: si archivia.
	Storia map[uuid.UUID][]string
}

// LeggiRiepilogo legge l'albero proposto e quello che il riepilogo vuole sapere della working, e legge la bozza contro
// l'albero; senza bozza (nil) legge la bozza vuota sull'albero di adesso: l'albero accettato cosi' com'e'. Solo
// letture: nessuna riga cambia, nemmeno con una bozza.
func LeggiRiepilogo(ctx context.Context, q *db.Queries, thread uuid.UUID, an coda.Analizzatore, b *BozzaAlbero) (AlberoProposto, RiepilogoAlbero, error) {
	a, r, _, err := leggiRiepilogo(ctx, q, thread, an, b)
	return a, r, err
}

// leggiRiepilogo e' LeggiRiepilogo con il piano della conferma: la conferma lo chiama sotto il lucchetto della RFQ.
func leggiRiepilogo(ctx context.Context, q *db.Queries, thread uuid.UUID, an coda.Analizzatore, b *BozzaAlbero) (AlberoProposto, RiepilogoAlbero, *pianoConferma, error) {
	s, err := LeggiStatoFlusso(ctx, q, thread, an)
	if err != nil {
		return AlberoProposto{}, RiepilogoAlbero{}, nil, err
	}
	rim, err := q.ListRimozioniAperte(ctx, thread)
	if err != nil {
		return AlberoProposto{}, RiepilogoAlbero{}, nil, err
	}
	storia, err := q.ListComponentiConStoria(ctx, thread)
	if err != nil {
		return AlberoProposto{}, RiepilogoAlbero{}, nil, err
	}
	p := ProdottiDellaRfq(&s)
	anc := Ancore(p, &s)
	a := NuovoAlberoProposto(&s, p, anc, IndiceCodici(p, anc, &s), rim)
	cx := ContestoRiepilogo{Motore: s.Motore, Componenti: s.Componenti, Relazioni: s.Relazioni, Storia: map[uuid.UUID][]string{}}
	for _, i := range s.Identificativi {
		cx.Richiesta = append(cx.Richiesta, i.Codice)
	}
	for _, x := range storia {
		cx.Storia[x.ComponenteID] = fraseStoria(x)
	}
	bozza := BozzaAlbero{Formato: FormatoBozza, Base: a.Firma}
	if b != nil {
		bozza = *b
	}
	r, pc, err := riepiloga(a, bozza, cx)
	if err != nil {
		return a, r, pc, err
	}
	if err := effettiDeiTipi(ctx, q, thread, &r, pc); err != nil {
		return AlberoProposto{}, RiepilogoAlbero{}, nil, err
	}
	r.Firma = firmaRiepilogo(a.Firma, bozza, r.Fuori, r.Tipi)
	return a, r, pc, nil
}

// effettiDeiTipi dice, per ogni tipo che cambia su un componente che c'e', che cosa il cambio fara' quando la conferma
// lo scrive (fase 4.4a.1b, dalla verifica): l'anteprima della scheda (EffettoCambioTipo, solo letture) sul componente
// come la conferma lo trovera' al passo 4 — ripristinato, con i figli dell'albero confermato, con le rimozioni dei
// legami dell'albero gia' decise. Un cambio che non si puo' fare e' un blocco: la conferma non lo scoprira' dopo un
// riepilogo «confermabile».
func effettiDeiTipi(ctx context.Context, q *db.Queries, thread uuid.UUID, r *RiepilogoAlbero, pc *pianoConferma) error {
	if len(pc.cambi) == 0 {
		return nil
	}
	decise := map[[3]uuid.UUID]bool{}
	for _, l := range pc.rimozioni {
		p, okp := pc.a.Nodo(l.Padre)
		f, okf := pc.a.Nodo(l.Figlio)
		if okp && okf && p.Componente.Valid && f.Componente.Valid {
			decise[[3]uuid.UUID{l.Rimozione.Step, p.Componente.UUID, f.Componente.UUID}] = true
		}
	}
	for _, c := range pc.cambi {
		dopo := &comeDopoLaConferma{rimozioni: decise}
		for _, l := range pc.legami {
			if l.padre == c.g {
				dopo.figli = append(dopo.figli, l.figlio.codice)
			}
		}
		sort.Strings(dopo.figli)
		t := &r.Tipi[c.indice]
		e, err := effettoCambioTipo(ctx, q, thread, c.g.comp.ComponenteID, t.A, dopo)
		if err != nil {
			return err
		}
		t.Spento, t.Effetti = e.Spento, nil
		if e.Spento == "" {
			t.Effetti = e.frasi()
		} else {
			r.Blocchi = append(r.Blocchi, t.Codice+": "+e.Spento)
		}
	}
	r.Confermabile = len(r.Blocchi) == 0
	return nil
}

// fraseStoria dice che cosa tiene un componente fuori dalla working, con le parole di cheCosaLoTiene.
func fraseStoria(x db.ListComponentiConStoriaRow) []string {
	var out []string
	if x.Documenti {
		out = append(out, "ha dei documenti")
	}
	if x.Proposte {
		out = append(out, "ha delle proposte (di documento, di struttura o di rimozione)")
	}
	if x.Deroghe {
		out = append(out, "ha delle deroghe")
	}
	if x.Baseline {
		out = append(out, "è in una baseline congelata")
	}
	if x.Note {
		out = append(out, "ha delle note sui disegni")
	}
	return out
}

// ------------------------------------------------------------------ la bozza sull'albero (puro)

// pezzoBozza e' un nodo dell'albero dopo la bozza: un nodo dell'albero proposto o un pezzo aggiunto.
type pezzoBozza struct {
	chiave     string
	albero     *NodoAlbero    // nil per un pezzo aggiunto
	aggiunto   *AggiuntoBozza // nil per un nodo dell'albero
	codice     string         // il codice finale, come lo si scrive
	identita   string         // «cod:<canonico>», o la chiave per un nodo senza codice
	rev        string
	revScritta string // la revisione scritta nella rinomina
	tipo       db.TipoComponente
	tipoScelto bool
	rinominato bool
	tolto      bool
	risposta   string // ✓ o ✗ della proposta commerciale
}

// confermaCommerciale: una persona ha confermato la proposta commerciale del nodo, con il ✓ o scegliendo con la tendina
// il particolare commerciale (un gesto esplicito anche quello: vedi BozzaAlbero.Tipi).
func (p *pezzoBozza) confermaCommerciale() bool {
	if p.albero == nil || p.albero.Commerciale == nil {
		return false
	}
	return p.risposta == RispostaSi || (p.risposta == "" && p.tipoScelto && p.tipo == db.TipoComponenteCommerciale)
}

// legameBozza e' un legame dopo la bozza, fra due chiavi.
type legameBozza struct {
	padre, figlio string
	qta           int32
	scelta        bool        // la quantita' l'ha scelta la bozza
	albero        *ArcoAlbero // il legame dell'albero da cui viene, se c'e'
	scritto       bool        // lo scrive la bozza (un aggiunto, un «anche sotto»)
}

// bozzaApplicata e' l'albero con la bozza sopra: i pezzi per chiave, i legami per (padre, figlio).
type bozzaApplicata struct {
	a       *AlberoProposto
	cx      ContestoRiepilogo
	pezzi   map[string]*pezzoBozza
	ordine  []string
	legami  map[[2]string]*legameBozza
	diversi map[string]bool
}

func (x *bozzaApplicata) nome(chiave string) string {
	if p, ok := x.pezzi[chiave]; ok {
		if p.codice != "" {
			return p.codice
		}
		if p.albero != nil && p.albero.Nome != "" {
			return "«" + p.albero.Nome + "»"
		}
	}
	return chiave
}

// canonico e' il codice come lo confronta l'albero: tolto il suffisso decorativo del cliente, in maiuscolo.
func (x *bozzaApplicata) canonico(c string) string {
	c = strings.TrimSpace(c)
	if c == "" {
		return ""
	}
	k, _ := x.cx.Motore.Canonico(c, "")
	return strings.ToUpper(strings.TrimSpace(k))
}

// applica mette la bozza sull'albero e controlla che ogni voce si possa leggere: i riferimenti, i codici, i tipi, le
// quantita', i limiti. Il primo problema e' il rifiuto, con le parole per chi ha disegnato la bozza.
func applica(a *AlberoProposto, b BozzaAlbero, cx ContestoRiepilogo) (*bozzaApplicata, error) {
	x := &bozzaApplicata{a: a, cx: cx, pezzi: map[string]*pezzoBozza{}, legami: map[[2]string]*legameBozza{}, diversi: map[string]bool{}}
	if b.Formato != FormatoBozza {
		return nil, Rifiuto(fmt.Sprintf("la bozza ha il formato %d, e qui si legge il %d: ricarica la pagina", b.Formato, FormatoBozza))
	}
	for nome, n := range map[string]int{"rinomine": len(b.Rinomine), "tolti": len(b.Tolti), "aggiunti": len(b.Aggiunti),
		"legami": len(b.Legami), "quantità": len(b.Quantita), "tipi": len(b.Tipi), "risposte": len(b.Commerciali), "diversi": len(b.Diversi)} {
		if n > MaxVociBozza {
			return nil, Rifiuto(fmt.Sprintf("la bozza ha %d %s: il limite è %d", n, nome, MaxVociBozza))
		}
	}
	for i := range a.Nodi {
		n := &a.Nodi[i]
		if n.Stato == StatoAlberoScartato {
			continue
		}
		x.pezzi[n.Chiave] = &pezzoBozza{chiave: n.Chiave, albero: n, codice: n.Codice, identita: n.Chiave, rev: n.Rev, tipo: n.Tipo}
		x.ordine = append(x.ordine, n.Chiave)
	}
	for i := range a.Archi {
		r := &a.Archi[i]
		if r.Stato == StatoAlberoScartato {
			continue
		}
		x.legami[[2]string{r.Padre, r.Figlio}] = &legameBozza{padre: r.Padre, figlio: r.Figlio, qta: r.Qta, albero: r}
	}
	quantita := func(q int32, cosa string) error {
		if q < 1 || q > MaxQtaArco {
			return Rifiuto(fmt.Sprintf("%s: la quantità va da 1 a %d", cosa, MaxQtaArco))
		}
		return nil
	}
	// i pezzi aggiunti: il codice lo scrive l'operatore, il tipo e' uno di quelli di un pezzo scritto (TipiNuovo)
	for i := range b.Aggiunti {
		n := &b.Aggiunti[i]
		if !strings.HasPrefix(n.ID, "nuovo:") || len(n.ID) == len("nuovo:") {
			return nil, Rifiuto(fmt.Sprintf("un pezzo aggiunto ha la chiave «%s»: le chiavi dei pezzi aggiunti sono «nuovo:<n>»", n.ID))
		}
		if _, gia := x.pezzi[n.ID]; gia {
			return nil, Rifiuto(n.ID + " è aggiunto due volte")
		}
		codice := strings.TrimSpace(n.Codice)
		if err := codiceScritto(codice, n.Rev); err != nil {
			return nil, err
		}
		if !tipoScrivibile(n.Tipo) {
			return nil, Rifiuto(codice + ": un pezzo aggiunto è un assieme, un particolare o un particolare commerciale")
		}
		x.pezzi[n.ID] = &pezzoBozza{chiave: n.ID, aggiunto: n, codice: codice, identita: "cod:" + x.canonico(codice),
			rev: strings.ToUpper(strings.TrimSpace(n.Rev)), tipo: n.Tipo, tipoScelto: true}
		x.ordine = append(x.ordine, n.ID)
	}
	for i := range b.Aggiunti {
		n := &b.Aggiunti[i]
		if _, ok := x.pezzi[n.Padre]; !ok {
			return nil, Rifiuto(fmt.Sprintf("il padre di %s non è nell'albero: ricarica la pagina", n.Codice))
		}
		if err := quantita(n.Qta, n.Codice+" sotto "+x.nome(n.Padre)); err != nil {
			return nil, err
		}
		x.legami[[2]string{n.Padre, n.ID}] = &legameBozza{padre: n.Padre, figlio: n.ID, qta: n.Qta, scelta: true, scritto: true}
	}
	esiste := func(k, cosa string) (*pezzoBozza, error) {
		p, ok := x.pezzi[k]
		if !ok {
			return nil, Rifiuto(fmt.Sprintf("%s: il nodo «%s» non è nell'albero di adesso: ricarica la pagina", cosa, k))
		}
		return p, nil
	}
	// i tolti: un nodo intero, o un legame solo
	tolti := map[string]bool{}
	for _, t := range b.Tolti {
		p, err := esiste(t.Nodo, "togli")
		if err != nil {
			return nil, err
		}
		if p.albero != nil && p.albero.Prodotto {
			return nil, Rifiuto(p.codice + " è un prodotto della RFQ: non si toglie dall'albero, viene dal triage")
		}
		if t.Padre == "" {
			if tolti[t.Nodo] {
				return nil, Rifiuto(x.nome(t.Nodo) + " è tolto due volte")
			}
			tolti[t.Nodo], p.tolto = true, true
			for k := range x.legami {
				if k[0] == t.Nodo || k[1] == t.Nodo {
					delete(x.legami, k)
				}
			}
			continue
		}
		if _, ok := x.legami[[2]string{t.Padre, t.Nodo}]; !ok {
			return nil, Rifiuto(fmt.Sprintf("%s non sta sotto %s: ricarica la pagina", x.nome(t.Nodo), x.nome(t.Padre)))
		}
		delete(x.legami, [2]string{t.Padre, t.Nodo})
	}
	// un pezzo aggiunto sotto un padre tolto nella stessa bozza (o tolto lui stesso) si rifiuta, come un legame: senza,
	// andrebbe via con la cascata senza che il riepilogo lo dica (fase 4.4a.1b, dalla verifica della 4.4a.1a)
	for i := range b.Aggiunti {
		n := &b.Aggiunti[i]
		if tolti[n.Padre] || tolti[n.ID] {
			return nil, Rifiuto(fmt.Sprintf("%s sotto %s: uno dei due è tolto dall'albero", n.Codice, x.nome(n.Padre)))
		}
	}
	// i legami in piu' («anche sotto»)
	for _, l := range b.Legami {
		p, err := esiste(l.Padre, "anche sotto")
		if err != nil {
			return nil, err
		}
		f, err := esiste(l.Figlio, "anche sotto")
		if err != nil {
			return nil, err
		}
		if p.tolto || f.tolto {
			return nil, Rifiuto(fmt.Sprintf("%s sotto %s: uno dei due è tolto dall'albero", x.nome(l.Figlio), x.nome(l.Padre)))
		}
		if f.albero != nil && f.albero.Prodotto {
			return nil, Rifiuto(f.codice + " è un prodotto della RFQ: non va sotto un altro pezzo")
		}
		if _, gia := x.legami[[2]string{l.Padre, l.Figlio}]; gia {
			return nil, Rifiuto(fmt.Sprintf("%s sta già sotto %s: se ne cambia la quantità", x.nome(l.Figlio), x.nome(l.Padre)))
		}
		if err := quantita(l.Qta, x.nome(l.Figlio)+" sotto "+x.nome(l.Padre)); err != nil {
			return nil, err
		}
		x.legami[[2]string{l.Padre, l.Figlio}] = &legameBozza{padre: l.Padre, figlio: l.Figlio, qta: l.Qta, scelta: true, scritto: true}
	}
	for _, l := range b.Quantita {
		r, ok := x.legami[[2]string{l.Padre, l.Figlio}]
		if !ok {
			return nil, Rifiuto(fmt.Sprintf("la quantità di %s sotto %s: il legame non c'è", x.nome(l.Figlio), x.nome(l.Padre)))
		}
		if err := quantita(l.Qta, x.nome(l.Figlio)+" sotto "+x.nome(l.Padre)); err != nil {
			return nil, err
		}
		r.qta, r.scelta = l.Qta, true
	}
	// le rinomine: solo un nodo proposto, che non e' ancora un componente della distinta
	for _, r := range b.Rinomine {
		p, err := esiste(r.Nodo, "rinomina")
		if err != nil {
			return nil, err
		}
		switch {
		case p.aggiunto != nil:
			return nil, Rifiuto(p.codice + " è un pezzo aggiunto: il suo codice si scrive nell'aggiunto")
		case p.rinominato:
			return nil, Rifiuto(x.nome(r.Nodo) + " è rinominato due volte")
		case p.albero.Prodotto:
			return nil, Rifiuto(p.codice + " è un prodotto della RFQ: il suo codice viene dal triage")
		case p.albero.Stato != StatoAlberoProposto:
			return nil, Rifiuto(p.codice + " c'è già nella distinta: il codice di un componente si corregge nel Fascicolo")
		case p.albero.Ritrovato != nil && p.albero.Ritrovato.Agganciato:
			// fase 4.4a.1b: la rinomina scrive il codice solo sulle righe aperte, e l'aggancio resterebbe al componente
			// di prima
			return nil, Rifiuto(p.codice + " è agganciato per codice a un componente da prima dello Smistamento: il codice di un componente si corregge nel Fascicolo")
		}
		codice := strings.TrimSpace(r.Codice)
		if err := codiceScritto(codice, r.Rev); err != nil {
			return nil, err
		}
		p.codice, p.identita, p.rinominato = codice, "cod:"+x.canonico(codice), true
		if rev := strings.ToUpper(strings.TrimSpace(r.Rev)); rev != "" {
			p.rev, p.revScritta = rev, rev
		}
	}
	for _, t := range b.Tipi {
		p, err := esiste(t.Nodo, "tipo")
		if err != nil {
			return nil, err
		}
		if p.albero != nil && p.albero.Prodotto {
			return nil, Rifiuto(p.codice + " è un prodotto della RFQ: il tipo non cambia")
		}
		if !tipoScrivibile(t.Tipo) {
			return nil, Rifiuto(x.nome(t.Nodo) + ": il tipo è un assieme, un particolare o un particolare commerciale")
		}
		if p.tipoScelto && p.aggiunto == nil {
			return nil, Rifiuto(x.nome(t.Nodo) + ": il tipo è scelto due volte")
		}
		p.tipo, p.tipoScelto = t.Tipo, true
	}
	for _, r := range b.Commerciali {
		p, err := esiste(r.Nodo, "proposta commerciale")
		if err != nil {
			return nil, err
		}
		if p.albero == nil || p.albero.Commerciale == nil {
			return nil, Rifiuto(x.nome(r.Nodo) + " non ha una proposta commerciale: ricarica la pagina")
		}
		if r.Risposta != RispostaSi && r.Risposta != RispostaNo {
			return nil, Rifiuto(x.nome(r.Nodo) + ": la risposta alla proposta commerciale è ✓ o ✗")
		}
		if p.risposta != "" {
			return nil, Rifiuto(x.nome(r.Nodo) + ": la proposta commerciale ha due risposte")
		}
		p.risposta = r.Risposta
		// il tipo scelto con la tendina e la risposta dicono la stessa cosa
		if p.tipoScelto && (r.Risposta == RispostaSi) != (p.tipo == db.TipoComponenteCommerciale) {
			return nil, Rifiuto(x.nome(r.Nodo) + ": il tipo scelto e la risposta alla proposta commerciale non dicono la stessa cosa")
		}
	}
	for _, k := range b.Diversi {
		if _, err := esiste(k, "diverso"); err != nil {
			return nil, err
		}
		if x.diversi[k] {
			return nil, Rifiuto(x.nome(k) + " è detto diverso due volte")
		}
		x.diversi[k] = true
	}
	// un pezzo aggiunto con un codice che l'albero ha gia': e' quel pezzo, e si mette «anche sotto»
	vivi := map[string]string{}
	for _, k := range x.ordine {
		p := x.pezzi[k]
		if p.aggiunto != nil || p.tolto || p.codice == "" {
			continue
		}
		vivi[p.identita] = k
	}
	scritti := map[string]string{}
	for _, k := range x.ordine {
		p := x.pezzi[k]
		if p.aggiunto == nil {
			continue
		}
		if altro, ok := vivi[p.identita]; ok {
			return nil, Rifiuto(fmt.Sprintf("%s è già nell'albero (%s): lo si mette anche sotto %s, non lo si aggiunge di nuovo",
				p.codice, x.nome(altro), x.nome(p.aggiunto.Padre)))
		}
		if altro, ok := scritti[p.identita]; ok {
			return nil, Rifiuto(fmt.Sprintf("%s è aggiunto due volte (%s e %s): un pezzo è uno", p.codice, altro, k))
		}
		scritti[p.identita] = k
	}
	return x, nil
}

// codiceScritto controlla un codice e una revisione scritti nella bozza.
func codiceScritto(codice, rev string) error {
	if !classificazione.CodiceAmmissibile(codice) {
		if codice == "" {
			return Rifiuto("un codice scritto nella bozza è vuoto")
		}
		return Rifiuto(fmt.Sprintf("«%s»: il codice ha più di %d caratteri o caratteri non ammessi", codice, classificazione.MaxCodice))
	}
	if r := strings.ToUpper(strings.TrimSpace(rev)); r != "" && !classificazione.RevAmmissibile(r) {
		return Rifiuto(fmt.Sprintf("«%s»: la revisione ha più di %d caratteri o caratteri non ammessi", codice, classificazione.MaxRev))
	}
	return nil
}

// tipoScrivibile: i tipi che una persona sceglie per un pezzo nella Distinta (TipiNuovo).
func tipoScrivibile(t db.TipoComponente) bool {
	for _, x := range TipiNuovo {
		if t == x {
			return true
		}
	}
	return false
}

// gruppo e' un pezzo dopo la bozza: i nodi con la stessa identita' (lo stesso codice, dopo le rinomine) sono uno.
type gruppo struct {
	identita string
	membri   []*pezzoBozza
	codice   string
	tipo     db.TipoComponente
	prodotto bool
	comp     db.Componente // il componente con lo stesso codice canonico (anche archiviato)
	esiste   bool
}

// rappresentante e' il nodo che da' la chiave e il codice del gruppo: un nodo dell'albero prima di un aggiunto.
func (g *gruppo) rappresentante() *pezzoBozza { return g.membri[0] }

// commercialeConfermato: il pezzo nasce particolare commerciale perche' una persona ha confermato la sua proposta (il
// ✓, o la tendina). Solo un pezzo nuovo di un nodo solo: con due nodi (una rinomina che li unisce) la proposta decade.
func (g *gruppo) commercialeConfermato() bool {
	return !g.esiste && len(g.membri) == 1 && g.membri[0].confermaCommerciale()
}

// legameFinale e' un legame fra due gruppi dopo la bozza, con i legami della bozza (e dell'albero) che lo fanno e la
// quantita': quella scelta nella bozza vince; discordi, 0.
type legameFinale struct {
	padre, figlio *gruppo
	qta           int32
	discordi      bool
	scelta        bool
	scritto       bool
	da            []*legameBozza
}

// pianoConferma e' quello che la conferma dell'albero scrive (albero_conferma.go), calcolato con gli stessi conti del
// riepilogo: il riepilogo lo dice in parole, la conferma lo fa, e non c'e' un secondo calcolo che possa dire altro.
type pianoConferma struct {
	a *AlberoProposto
	// gruppi: i pezzi dell'albero confermato (quelli che si raggiungono da un prodotto), in ordine; legami: i legami
	// fra loro, in ordine
	gruppi []*gruppo
	legami []*legameFinale
	// usati: i legami dell'albero che un legame finale tiene; gli altri (tolti dalla bozza, o sotto un pezzo che va
	// via) si chiudono
	usati map[*ArcoAlbero]bool
	// tolti: i legami della working che vanno via; rimozioni: le rimozioni proposte dallo STEP, con la loro sorte
	tolti     []*ArcoAlbero
	rimozioni []*ArcoAlbero
	fuori     []FuoriRiepilogo
	// via: i nodi che escono dall'albero, le cui righe si chiudono
	via []*NodoAlbero
	// revisioni: la revisione nuova dei componenti che ci sono (per identita' del gruppo)
	revisioni map[string]string
	// cambi: i tipi che cambiano su un componente che c'e' (l'indice in RiepilogoAlbero.Tipi), di cui LeggiRiepilogo
	// dice l'effetto (effettiDeiTipi)
	cambi []cambioTipo
}

// cambioTipo e' un tipo che cambia su un componente che c'e': la riga del riepilogo e il gruppo.
type cambioTipo struct {
	indice int
	g      *gruppo
}

// Riepilogo legge la bozza contro l'albero proposto e dice che cosa la conferma farebbe. Pura. Un rifiuto dice che la
// bozza non si puo' leggere (un riferimento che non c'e', un codice non ammesso, un ciclo, un pezzo sotto un
// particolare); le domande ancora aperte non sono un rifiuto: stanno nei Blocchi.
func Riepilogo(a AlberoProposto, b BozzaAlbero, cx ContestoRiepilogo) (RiepilogoAlbero, error) {
	r, _, err := riepiloga(a, b, cx)
	return r, err
}

// riepiloga e' Riepilogo con il piano della conferma.
func riepiloga(a AlberoProposto, b BozzaAlbero, cx ContestoRiepilogo) (RiepilogoAlbero, *pianoConferma, error) {
	if a.per == nil {
		a.indicizza()
	}
	x, err := applica(&a, b, cx)
	if err != nil {
		return RiepilogoAlbero{}, nil, err
	}
	r := RiepilogoAlbero{Formato: FormatoBozza, AlberoFirma: a.Firma, Vecchia: b.Base != a.Firma}
	pc := &pianoConferma{a: &a, usati: map[*ArcoAlbero]bool{}, revisioni: map[string]string{}}
	perCan := map[string]db.Componente{}
	comp := append([]db.Componente(nil), cx.Componenti...)
	sort.Slice(comp, func(i, j int) bool { return comp[i].Codice < comp[j].Codice })
	for _, c := range comp {
		k := x.canonico(c.Codice)
		if prima, gia := perCan[k]; !gia || (prima.ArchiviatoIl != nil && c.ArchiviatoIl == nil) {
			perCan[k] = c
		}
	}

	// i gruppi: i pezzi non tolti, per identita'. In un gruppo di piu' nodi (una rinomina che dice «e' lo stesso
	// pezzo») vale il pezzo che c'e' gia' nella distinta, poi un nodo dell'albero: da' la chiave, il codice e il tipo,
	// salvo un tipo scelto nella bozza
	gruppi := map[string]*gruppo{}
	var ordineGruppi []string
	gruppoDi := map[string]*gruppo{}
	for _, k := range x.ordine {
		p := x.pezzi[k]
		if p.tolto {
			continue
		}
		g, ok := gruppi[p.identita]
		if !ok {
			g = &gruppo{identita: p.identita}
			if c, ok := strings.CutPrefix(p.identita, "cod:"); ok {
				g.comp, g.esiste = perCan[c]
			}
			gruppi[p.identita] = g
			ordineGruppi = append(ordineGruppi, p.identita)
		}
		g.membri = append(g.membri, p)
		g.prodotto = g.prodotto || (p.albero != nil && p.albero.Prodotto)
		gruppoDi[k] = g
	}
	rango := func(p *pezzoBozza) int {
		switch {
		case p.albero != nil && p.albero.Componente.Valid && p.albero.Stato != StatoAlberoProposto:
			return 0
		case p.albero != nil:
			return 1
		}
		return 2
	}
	for _, k := range ordineGruppi {
		g := gruppi[k]
		sort.SliceStable(g.membri, func(i, j int) bool { return rango(g.membri[i]) < rango(g.membri[j]) })
		g.codice, g.tipo = g.membri[0].codice, g.membri[0].tipo
		var scelto *pezzoBozza
		for _, m := range g.membri {
			if !m.tipoScelto {
				continue
			}
			if scelto != nil && scelto.tipo != m.tipo {
				return RiepilogoAlbero{}, nil, Rifiuto(fmt.Sprintf("%s: i nodi con questo codice hanno tipi scelti diversi", m.codice))
			}
			scelto = m
		}
		if scelto != nil {
			g.tipo = scelto.tipo
		}
		if g.prodotto {
			g.tipo = db.TipoComponenteFinito
		}
	}
	// i legami fra gruppi. Due nodi che la bozza fa diventare lo stesso pezzo (una rinomina) uniscono i loro legami: due
	// figli diversi sotto lo stesso nodo padre sono due posizioni, e le quantita' si sommano; lo stesso figlio sotto due
	// nodi padre che diventano uno sono due descrizioni dello stesso assieme, e le quantita' devono dire lo stesso, se
	// no sono discordi. Una quantita' scelta nella bozza vince.
	finali := map[[2]string]*legameFinale{}
	var ordineLegami [][2]string
	for _, k := range ordineChiavi(x.legami) {
		l := x.legami[k]
		gp, gf := gruppoDi[l.padre], gruppoDi[l.figlio]
		if gp == nil || gf == nil {
			continue
		}
		if gp == gf {
			return RiepilogoAlbero{}, nil, Rifiuto(fmt.Sprintf("%s starebbe sotto se stesso: con la rinomina %s e %s sono lo stesso pezzo",
				gp.codice, x.nome(l.padre), x.nome(l.figlio)))
		}
		fk := [2]string{gp.identita, gf.identita}
		f, ok := finali[fk]
		if !ok {
			f = &legameFinale{padre: gp, figlio: gf}
			finali[fk] = f
			ordineLegami = append(ordineLegami, fk)
		}
		f.da = append(f.da, l)
		f.scritto = f.scritto || l.scritto
	}
	for _, fk := range ordineLegami {
		f := finali[fk]
		perPadre := map[string]int32{}
		var padri []string
		for _, l := range f.da {
			if _, ok := perPadre[l.padre]; !ok {
				padri = append(padri, l.padre)
			}
			perPadre[l.padre] += l.qta
			f.discordi = f.discordi || (l.albero != nil && l.albero.QtaDiscordi && !l.scelta)
			if l.scelta {
				f.scelta = true
			}
		}
		f.qta = perPadre[padri[0]]
		for _, p := range padri[1:] {
			if perPadre[p] != f.qta {
				f.discordi = true
			}
		}
		for _, l := range f.da {
			if l.scelta {
				f.qta, f.discordi = l.qta, false
			}
		}
		if f.discordi {
			f.qta = 0
		}
	}
	figliDi := map[string][]string{}
	padriDi := map[string][]string{}
	for _, fk := range ordineLegami {
		figliDi[fk[0]] = append(figliDi[fk[0]], fk[1])
		padriDi[fk[1]] = append(padriDi[fk[1]], fk[0])
	}
	// i controlli sulla struttura voluta: un particolare non ha figli (6b), un commerciale e' una foglia (6a), un
	// prodotto non sta sotto un altro pezzo. Colpa della bozza: rifiuto; dati di prima non toccati: un blocco
	for _, k := range ordineGruppi {
		g := gruppi[k]
		if len(figliDi[k]) == 0 {
			continue
		}
		tocca := false
		for _, m := range g.membri {
			tocca = tocca || m.tipoScelto || m.rinominato || m.aggiunto != nil || m.risposta == RispostaSi
		}
		for _, f := range figliDi[k] {
			tocca = tocca || finali[[2]string{k, f}].scritto
		}
		commerciale := false
		for _, m := range g.membri {
			commerciale = commerciale || m.risposta == RispostaSi
		}
		if !Contenitore(g.tipo) || commerciale {
			frase := fraseNonContenitore(g.codice, g.tipo)
			if commerciale {
				frase = g.codice + " è confermato particolare commerciale (✓): un commerciale è una foglia, sotto non ci va niente (6a)"
			}
			if tocca {
				return RiepilogoAlbero{}, nil, Rifiuto(frase)
			}
			r.Blocchi = append(r.Blocchi, frase)
		}
	}
	for _, fk := range ordineLegami {
		if f := finali[fk]; f.figlio.prodotto {
			frase := f.figlio.codice + " è un prodotto della RFQ: non va sotto " + f.padre.codice
			if f.scritto {
				return RiepilogoAlbero{}, nil, Rifiuto(frase)
			}
			r.Blocchi = append(r.Blocchi, frase+": togli il legame")
		}
	}
	// un ciclo: se c'era gia' nell'albero e' un blocco, se lo chiude la bozza e' un rifiuto
	idDi := func(s string) uuid.UUID { return uuid.NewSHA1(uuid.NameSpaceOID, []byte(s)) }
	nomeDi := map[uuid.UUID]string{}
	var archi []Arco
	for _, fk := range ordineLegami {
		f := finali[fk]
		p, c := idDi(fk[0]), idDi(fk[1])
		nomeDi[p], nomeDi[c] = f.padre.codice, f.figlio.codice
		archi = append(archi, Arco{Padre: p, Figlio: c})
	}
	if giro := Ciclo(archi); giro != nil {
		parti := make([]string, len(giro))
		for i, g := range giro {
			parti[i] = nomeDi[g]
		}
		frase := "l'albero chiuderebbe un ciclo (" + strings.Join(parti, " → ") + ")"
		var prima []Arco
		for _, l := range a.Archi {
			if l.Stato != StatoAlberoScartato {
				prima = append(prima, Arco{Padre: idDi(l.Padre), Figlio: idDi(l.Figlio)})
			}
		}
		if Ciclo(prima) == nil {
			return RiepilogoAlbero{}, nil, Rifiuto(frase)
		}
		r.Blocchi = append(r.Blocchi, frase+": toglilo")
	}

	// quello che si raggiunge dai prodotti, dopo la bozza
	raggiunto := map[string]bool{}
	var coda []string
	for _, k := range ordineGruppi {
		if gruppi[k].prodotto {
			raggiunto[k] = true
			coda = append(coda, k)
		}
	}
	for len(coda) > 0 {
		k := coda[0]
		coda = coda[1:]
		for _, f := range figliDi[k] {
			if !raggiunto[f] {
				raggiunto[f] = true
				coda = append(coda, f)
			}
		}
	}
	codiciPadri := func(k string) []string {
		var out []string
		for _, p := range padriDi[k] {
			if raggiunto[p] {
				out = append(out, gruppi[p].codice)
			}
		}
		sort.Strings(out)
		return out
	}
	for _, k := range ordineGruppi {
		if raggiunto[k] {
			pc.gruppi = append(pc.gruppi, gruppi[k])
		}
	}
	// un pezzo aggiunto che non si raggiunge piu' da un prodotto (il suo padre va via a cascata) si rifiuta: andrebbe via
	// senza che il riepilogo lo dica
	for i := range b.Aggiunti {
		n := &b.Aggiunti[i]
		if g := gruppoDi[n.ID]; g == nil || !raggiunto[g.identita] {
			return RiepilogoAlbero{}, nil, Rifiuto(fmt.Sprintf("%s sotto %s: %s non si raggiunge più da un prodotto", n.Codice, x.nome(n.Padre), x.nome(n.Padre)))
		}
	}
	for _, fk := range ordineLegami {
		if raggiunto[fk[0]] && raggiunto[fk[1]] {
			f := finali[fk]
			pc.legami = append(pc.legami, f)
			for _, l := range f.da {
				if l.albero != nil {
					pc.usati[l.albero] = true
				}
			}
		}
	}

	// i pezzi nuovi, i ritrovati, i tipi, le revisioni, i vicini, i nodi senza codice, le proposte commerciali
	var codiciFinali []string
	for _, c := range cx.Componenti {
		codiciFinali = append(codiciFinali, c.Codice)
	}
	codiciFinali = append(codiciFinali, cx.Richiesta...)
	for _, k := range ordineGruppi {
		if raggiunto[k] && gruppi[k].codice != "" {
			codiciFinali = append(codiciFinali, gruppi[k].codice)
		}
	}
	sort.Strings(codiciFinali)
	for _, k := range ordineGruppi {
		g := gruppi[k]
		if !raggiunto[k] {
			continue
		}
		rp := g.rappresentante()
		if g.codice == "" {
			r.SenzaCodice = append(r.SenzaCodice, x.nome(rp.chiave))
			continue
		}
		if !g.esiste {
			pz := PezzoRiepilogo{Nodo: rp.chiave, Codice: g.codice, Rev: rp.rev, Tipo: g.tipo, Padri: codiciPadri(k)}
			switch {
			case rp.aggiunto != nil || rp.rinominato:
				pz.Fonte = "scritto"
			case rp.albero != nil:
				pz.Fonte, pz.Origine = rp.albero.FonteCodice, rp.albero.OrigineCodice
			}
			pz.Commerciale = g.commercialeConfermato()
			r.Nuovi = append(r.Nuovi, pz)
			// i vicini (P4): contro i componenti, la richiesta e gli altri pezzi dell'albero
			var altri []string
			for _, c := range codiciFinali {
				if !strings.EqualFold(c, g.codice) {
					altri = append(altri, c)
				}
			}
			if v := Vicini(g.codice, altri, cx.Motore); len(v) > 0 {
				diverso := false
				for _, m := range g.membri {
					diverso = diverso || x.diversi[m.chiave]
				}
				r.Vicini = append(r.Vicini, ViciniRiepilogo{Nodo: rp.chiave, Codice: g.codice, Vicini: v, Diverso: diverso})
			}
		} else {
			// legato a un componente che c'e' per lo stesso codice (P13): ogni nodo con righe aperte, rinominato o scritto,
			// una riga per nodo
			for _, m := range g.membri {
				perche := ""
				switch {
				case m.aggiunto != nil:
					perche = "scritto nella bozza: è il componente che c'è con questo codice"
				case m.rinominato:
					perche = "rinominato nella bozza: è il componente che c'è con questo codice"
				case m.albero.Ritrovato != nil:
					perche = "le righe dello STEP hanno il suo codice"
					if m.albero.Stato != StatoAlberoProposto {
						perche += " (è già nella distinta)"
					}
					if m.albero.Ritrovato.Agganciato {
						// fase 4.4a.1b, dalla verifica (P13, U5): la conferma lo fa diventare la decisione di chi conferma
						perche += "; agganciato per codice prima dello Smistamento, senza una persona: con la conferma lo decidi tu"
					}
				}
				if perche != "" {
					r.Ritrovati = append(r.Ritrovati, RitrovatoRiepilogo{Nodo: m.chiave, Codice: g.codice, Componente: g.comp.ComponenteID,
						Archiviato: g.comp.ArchiviatoIl != nil, Perche: perche})
				}
			}
			// la revisione (fase 4.4a.1b; risposta 28: una revisione aggiorna lo stesso pezzo, non ne fa uno nuovo): le
			// righe ancora da decidere dicono una revisione diversa da quella del componente. Due revisioni diverse fra
			// i file non decidono: resta quella che c'e', e il riepilogo le dice
			if !g.prodotto {
				if rv := revisioneDaiFile(g); len(rv) > 0 {
					da := strings.ToUpper(strings.TrimSpace(g.comp.Rev.String))
					switch {
					case len(rv) > 1:
						r.Revisioni = append(r.Revisioni, RevisioneRiepilogo{Nodo: rp.chiave, Codice: g.codice, Componente: g.comp.ComponenteID,
							Da: da, Discordi: rv})
					case rv[0] != da:
						r.Revisioni = append(r.Revisioni, RevisioneRiepilogo{Nodo: rp.chiave, Codice: g.codice, Componente: g.comp.ComponenteID,
							Da: da, A: rv[0]})
						pc.revisioni[g.identita] = rv[0]
					}
				}
			}
		}
		// i tipi che cambiano: per un componente che c'e' (anche archiviato: la conferma lo ripristina, e segue 6a e
		// 6b come un attivo) rispetto al suo tipo, per un pezzo nuovo rispetto alla proposta
		scelto := false
		for _, m := range g.membri {
			scelto = scelto || (m.tipoScelto && m.aggiunto == nil)
		}
		da, motivo := db.TipoComponente(""), ""
		switch {
		case g.esiste && !g.prodotto:
			da = g.comp.Tipo
			motivo = "scelto nella bozza"
			if rp.albero != nil && !scelto {
				motivo = rp.albero.TipoMotivo
			}
		case !g.esiste && rp.albero != nil && scelto:
			da, motivo = rp.albero.Tipo, "scelto nella bozza"
		}
		if da != "" && da != g.tipo {
			r.Tipi = append(r.Tipi, TipoRiepilogo{Nodo: rp.chiave, Codice: g.codice, Da: da, A: g.tipo, Motivo: motivo})
			if g.esiste {
				pc.cambi = append(pc.cambi, cambioTipo{indice: len(r.Tipi) - 1, g: g})
			}
		}
	}
	// le proposte commerciali, una per riga. Quella di un nodo senza codice conta da quando nella bozza prende un
	// codice: prima il nodo non nasce (lo ferma il codice che manca). Il particolare commerciale scelto con la tendina
	// e' il ✓ della proposta: e' un gesto esplicito della persona (vedi BozzaAlbero)
	for _, k := range x.ordine {
		p := x.pezzi[k]
		if p.albero == nil || p.albero.Commerciale == nil {
			continue
		}
		g := gruppoDi[k]
		if (g == nil || g.codice == "") && p.albero.Codice == "" {
			continue
		}
		cr := CommercialeRiepilogo{Nodo: k, Codice: p.albero.Codice, Motivo: p.albero.Commerciale.Motivo}
		if g != nil {
			cr.Codice = g.codice
		}
		switch {
		case g == nil || !raggiunto[g.identita] || g.esiste || len(figliDi[g.identita]) > 0 || len(g.membri) > 1:
			cr.Stato = CommercialeDecaduta
		case p.risposta == RispostaSi:
			cr.Stato = CommercialeSi
		case p.risposta == RispostaNo:
			cr.Stato = CommercialeNo
		case p.confermaCommerciale():
			cr.Stato, cr.Tendina = CommercialeSi, true
		default:
			cr.Stato = CommercialeAperta
		}
		r.Commerciali = append(r.Commerciali, cr)
	}

	// i legami: nuovi, tolti, con la quantita' cambiata; le rimozioni; le quantita' discordi
	nellaWorking := map[[2]string]*ArcoAlbero{}
	for i := range a.Archi {
		l := &a.Archi[i]
		if l.QtaWorking > 0 {
			nellaWorking[[2]string{l.Padre, l.Figlio}] = l
		}
	}
	toltiEspliciti := map[[2]string]bool{}
	toltiInteri := map[string]bool{}
	for _, t := range b.Tolti {
		if t.Padre != "" {
			toltiEspliciti[[2]string{t.Padre, t.Nodo}] = true
		} else {
			toltiInteri[t.Nodo] = true
		}
	}
	for _, fk := range ordineLegami {
		f := finali[fk]
		if !raggiunto[fk[0]] || !raggiunto[fk[1]] {
			continue
		}
		inWorking := false
		var file []string
		for _, l := range f.da {
			if l.albero != nil {
				if l.albero.QtaWorking > 0 && !f.padre.rappresentante().rinominato && !f.figlio.rappresentante().rinominato {
					inWorking = true
					if f.qta != l.albero.QtaWorking {
						r.Quantita = append(r.Quantita, LegameRiepilogo{Padre: f.padre.codice, Figlio: f.figlio.codice, Qta: f.qta,
							Prima: l.albero.QtaWorking, Motivo: "scelta nella bozza"})
					}
				}
				for _, fo := range l.albero.Fonti {
					if !fo.Scartata && !contiene(file, fo.File) {
						file = append(file, fo.File)
					}
				}
				if l.albero.QtaDiscordi && !contieneDiscorde(r.QuantitaDiscordi, f.padre.codice, f.figlio.codice) {
					d := DiscordeRiepilogo{Padre: f.padre.codice, Figlio: f.figlio.codice, Fonti: l.albero.Fonti}
					if f.scelta {
						d.Scelta = f.qta
					}
					r.QuantitaDiscordi = append(r.QuantitaDiscordi, d)
				}
			}
		}
		if f.discordi && !f.scelta && !contieneDiscorde(r.QuantitaDiscordi, f.padre.codice, f.figlio.codice) {
			r.QuantitaDiscordi = append(r.QuantitaDiscordi, DiscordeRiepilogo{Padre: f.padre.codice, Figlio: f.figlio.codice})
		}
		if !inWorking {
			l := LegameRiepilogo{Padre: f.padre.codice, Figlio: f.figlio.codice, Qta: f.qta, File: file}
			if f.scritto {
				l.Motivo = "scritto nella bozza"
			}
			r.LegamiNuovi = append(r.LegamiNuovi, l)
		}
	}
	for _, k := range ordineChiavi(nellaWorking) {
		l := nellaWorking[k]
		gp, gf := gruppoDi[k[0]], gruppoDi[k[1]]
		resta := gp != nil && gf != nil && raggiunto[gp.identita] && raggiunto[gf.identita] && finali[[2]string{gp.identita, gf.identita}] != nil
		if l.Rimozione != nil {
			st := RimozioneTenuta
			if !resta {
				st = RimozioneTolta
			}
			r.Rimozioni = append(r.Rimozioni, RimozioneRiepilogo{Padre: x.nome(k[0]), Figlio: x.nome(k[1]), Step: l.Rimozione.Step, Stato: st})
			pc.rimozioni = append(pc.rimozioni, l)
		}
		if resta {
			continue
		}
		pc.tolti = append(pc.tolti, l)
		motivo := ""
		switch {
		case toltiEspliciti[k] && l.Rimozione != nil:
			motivo = "tolto nella bozza: la rimozione proposta dallo STEP è accettata"
		case toltiEspliciti[k]:
			motivo = "tolto nella bozza"
		case toltiInteri[k[1]]:
			motivo = x.nome(k[1]) + " è tolto dall'albero"
		case toltiInteri[k[0]]:
			motivo = x.nome(k[0]) + " è tolto dall'albero"
		default:
			motivo = "a cascata: " + x.nome(k[0]) + " non si raggiunge più da un prodotto"
		}
		r.LegamiTolti = append(r.LegamiTolti, LegameRiepilogo{Padre: x.nome(k[0]), Figlio: x.nome(k[1]), Qta: l.QtaWorking, Motivo: motivo})
	}

	// la cascata: che cosa porta via ogni «togli», e che cosa resta sotto altri padri
	figliAlbero := map[string][]string{}
	for _, l := range a.Archi {
		if l.Stato != StatoAlberoScartato {
			figliAlbero[l.Padre] = append(figliAlbero[l.Padre], l.Figlio)
		}
	}
	for _, t := range b.Tolti {
		c := CascataRiepilogo{Nodo: t.Nodo, Codice: x.nome(t.Nodo), Padre: t.Padre}
		if t.Padre != "" {
			c.Padre = x.nome(t.Padre)
		}
		visti := map[string]bool{t.Nodo: true}
		coda := []string{t.Nodo}
		for len(coda) > 0 {
			k := coda[0]
			coda = coda[1:]
			g := gruppoDi[k]
			if g != nil && raggiunto[g.identita] {
				c.Restano = append(c.Restano, RestaRiepilogo{Codice: g.codice, Padri: codiciPadri(g.identita)})
				continue
			}
			c.Vanno = append(c.Vanno, x.nome(k))
			for _, f := range figliAlbero[k] {
				if !visti[f] {
					visti[f] = true
					coda = append(coda, f)
				}
			}
		}
		sort.Strings(c.Vanno)
		sort.Slice(c.Restano, func(i, j int) bool { return c.Restano[i].Codice < c.Restano[j].Codice })
		r.Cascata = append(r.Cascata, c)
	}

	// i componenti che restano senza padri, e i nodi che vanno via (le loro righe si chiudono)
	padriWorking := map[uuid.UUID][]uuid.UUID{}
	for _, l := range cx.Relazioni {
		padriWorking[l.FiglioID] = append(padriWorking[l.FiglioID], l.PadreID)
	}
	chiaveComp := map[uuid.UUID]string{}
	for _, n := range a.Nodi {
		if n.Componente.Valid && n.Stato != StatoAlberoProposto && n.Stato != StatoAlberoScartato {
			chiaveComp[n.Componente.UUID] = n.Chiave
		}
	}
	for i := range a.Nodi {
		n := &a.Nodi[i]
		g := gruppoDi[n.Chiave]
		va := g == nil || !raggiunto[g.identita]
		if n.Stato == StatoAlberoScartato || n.Prodotto || !va {
			continue
		}
		pc.via = append(pc.via, n)
		if n.Stato == StatoAlberoProposto {
			vive := 0
			for _, rr := range n.Righe {
				if rr.daDecidere() {
					vive++
				}
			}
			r.Scartate = append(r.Scartate, ScartataRiepilogo{Nodo: n.Chiave, Codice: x.nome(n.Chiave), Righe: vive})
			continue
		}
		// un componente della distinta che l'albero non tiene piu': ha ancora un padre fuori dall'albero?
		altri := false
		for _, p := range padriWorking[n.Componente.UUID] {
			if _, nellAlbero := chiaveComp[p]; !nellAlbero {
				altri = true
			}
		}
		if altri {
			continue
		}
		fu := FuoriRiepilogo{Codice: n.Codice, Componente: n.Componente.UUID, Esito: FuoriElimina, Perche: cx.Storia[n.Componente.UUID]}
		if len(fu.Perche) > 0 {
			fu.Esito = FuoriArchivia
		} else {
			fu.Perche = []string{"non ha documenti né storia"}
		}
		r.Fuori = append(r.Fuori, fu)
	}
	pc.fuori = r.Fuori

	// le domande aperte
	if r.Vecchia {
		r.Blocchi = append(r.Blocchi, "l'albero è cambiato da quando la bozza è stata disegnata: rivedila sull'albero di adesso")
	}
	aperte := 0
	for _, c := range r.Commerciali {
		if c.Stato == CommercialeAperta {
			aperte++
		}
	}
	if aperte > 0 {
		r.Blocchi = append(r.Blocchi, quanti(aperte, "proposta commerciale senza ✓ o ✗", "proposte commerciali senza ✓ o ✗"))
	}
	for _, v := range r.Vicini {
		if !v.Diverso {
			r.Blocchi = append(r.Blocchi, fmt.Sprintf("%s è quasi uguale a %s: è lo stesso pezzo (rinominalo) o un pezzo diverso?", v.Codice, codiciVicini(v.Vicini)))
		}
	}
	for _, d := range r.QuantitaDiscordi {
		if d.Scelta == 0 {
			r.Blocchi = append(r.Blocchi, fmt.Sprintf("%s sotto %s: i file dicono quantità diverse, scegline una", d.Figlio, d.Padre))
		}
	}
	for _, s := range r.SenzaCodice {
		r.Blocchi = append(r.Blocchi, s+" non ha un codice: scrivilo")
	}
	r.Confermabile = len(r.Blocchi) == 0
	r.vuoteNonNulle()
	r.Firma = firmaRiepilogo(a.Firma, b, r.Fuori, r.Tipi)
	return r, pc, nil
}

// revisioneDaiFile sono le revisioni (diverse, in ordine) che le righe ancora da decidere dei nodi del gruppo dicono,
// piu' quella scritta nella bozza per un nodo rinominato. Le righe gia' decise non contano: una persona le ha gia'
// viste, e la revisione del componente l'ha decisa (o corretta) lei.
func revisioneDaiFile(g *gruppo) []string {
	var out []string
	for _, m := range g.membri {
		if m.revScritta != "" && !contiene(out, m.revScritta) {
			out = append(out, m.revScritta)
		}
		if m.albero == nil {
			continue
		}
		for _, r := range m.albero.Righe {
			if r.daDecidere() && r.Rev != "" && !contiene(out, r.Rev) {
				out = append(out, r.Rev)
			}
		}
	}
	sort.Strings(out)
	return out
}

func contieneDiscorde(dd []DiscordeRiepilogo, padre, figlio string) bool {
	for _, d := range dd {
		if d.Padre == padre && d.Figlio == figlio {
			return true
		}
	}
	return false
}

func ordineChiavi[V any](m map[[2]string]V) [][2]string {
	out := make([][2]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}

// vuoteNonNulle: le liste vuote sono [] nel JSON, non null (la pagina le scorre senza guardare).
func (r *RiepilogoAlbero) vuoteNonNulle() {
	if r.Nuovi == nil {
		r.Nuovi = []PezzoRiepilogo{}
	}
	if r.Ritrovati == nil {
		r.Ritrovati = []RitrovatoRiepilogo{}
	}
	if r.Tipi == nil {
		r.Tipi = []TipoRiepilogo{}
	}
	if r.Revisioni == nil {
		r.Revisioni = []RevisioneRiepilogo{}
	}
	for _, l := range []*[]LegameRiepilogo{&r.LegamiNuovi, &r.LegamiTolti, &r.Quantita} {
		if *l == nil {
			*l = []LegameRiepilogo{}
		}
	}
	if r.Cascata == nil {
		r.Cascata = []CascataRiepilogo{}
	}
	if r.Fuori == nil {
		r.Fuori = []FuoriRiepilogo{}
	}
	if r.Scartate == nil {
		r.Scartate = []ScartataRiepilogo{}
	}
	if r.Rimozioni == nil {
		r.Rimozioni = []RimozioneRiepilogo{}
	}
	if r.Commerciali == nil {
		r.Commerciali = []CommercialeRiepilogo{}
	}
	if r.Vicini == nil {
		r.Vicini = []ViciniRiepilogo{}
	}
	if r.QuantitaDiscordi == nil {
		r.QuantitaDiscordi = []DiscordeRiepilogo{}
	}
	if r.SenzaCodice == nil {
		r.SenzaCodice = []string{}
	}
	if r.Blocchi == nil {
		r.Blocchi = []string{}
	}
}

// firmaRiepilogo e' lo sha256 della firma dell'albero (che porta la working), della bozza e della sorte dei componenti
// che restano senza padri (archiviati o eliminati: dipende dalla loro storia). La bozza entra con il suo JSON: la
// stessa bozza sullo stesso albero, la stessa firma. La storia degli altri componenti non ci entra: un documento
// confermato nel frattempo per un pezzo che resta non cambia niente di quello che la conferma farebbe. Dalla fase
// 4.4a.1b anche l'effetto dei cambi di tipo sui componenti che ci sono (TipoRiepilogo.Effetti e Spento): dipende dalle
// autorizzazioni e dalle rimozioni aperte, che la firma dell'albero non porta tutte (una sospensione, una delega).
func firmaRiepilogo(albero string, b BozzaAlbero, fuori []FuoriRiepilogo, tipi []TipoRiepilogo) string {
	var storia, effetti []string
	for _, f := range fuori {
		storia = append(storia, f.Componente.String()+":"+f.Esito)
	}
	sort.Strings(storia)
	for _, t := range tipi {
		if t.Spento != "" || len(t.Effetti) > 0 {
			effetti = append(effetti, t.Nodo+":"+t.Spento+":"+strings.Join(t.Effetti, "|"))
		}
	}
	raw, _ := json.Marshal(struct {
		Albero  string
		Bozza   BozzaAlbero
		Fuori   []string
		Effetti []string
	}{albero, b, storia, effetti})
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
