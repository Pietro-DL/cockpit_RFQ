package distintabozza

// riepilogo3_motore.go: gli avvisi e i conflitti del Motore A in R4 formato 3 e nella sua ricostruzione nella S di R5
// (direttiva 39, blocco «R4 completo» della corsia persistenza; EM §8.2, §9.4, §9.5 con il recepimento di U2 §6.3.5,
// RV3-25 A [U]: gli avvisi del motore entrano nella firma e R5 ricalcola il riepilogo con il motore nella seconda
// connessione). Il core non chiama il motore: lo calcola l'adattatore web (transport/web, Servizio.Calcola con il semaforo dei
// calcoli, come le altre rotte) e lo passa qui come IngressoMotoreR4; qui si compone, in modo puro.
//
// Le regole [T-39 della corsia persistenza]:
//   - R4 senza ?avvisi_motore=1 e' il riepilogo leggero (PA-C01): avvisi_motore_calcolati falso, conflitti non calcolati,
//     verifica prevista mai «si'», e confermabile falso: per confermare serve il riepilogo con il motore, che e' quello
//     che R5 ricalcola. Con il parametro e un calcolo riuscito avvisi_motore_calcolati e' vero;
//   - il motore che non risponde in R4 (occupato, scaduto, errore): 200 con avvisi_motore_calcolati falso e
//     avvisi_motore_errore {CALCOLO_OCCUPATO | CALCOLO_SCADUTO | ERRORE_CALCOLO} (EM §9.4: mai un 5xx per il parametro);
//   - in R5 il motore si calcola dentro la connessione S (la stessa fotografia del resto del riepilogo): occupato o non
//     pronto 503 CALCOLO_OCCUPATO, scaduto 504 CALCOLO_SCADUTO, un altro errore 500 ERRORE_CALCOLO, tutti non registrati;
//   - la firma comprende avvisi_motore[], avvisi_motore_calcolati e conflitti_motore[]; avvisi_motore_errore resta fuori
//     (l'errore di un calcolo non e' contenuto della conferma);
//   - i conflitti aperti del motore sul prodotto (condizione (f) di prima-istanza-1, R95 A [U]) sono quelli degli assi del
//     prodotto valutato: nomenclatura, gerarchia, smistamento (valutazione.conflittoAperto). Un prodotto che il motore non
//     valuta ha i conflitti non calcolati: la verifica non e' mai «si'»;
//   - gli avvisi calcolati: PROPOSTA_CAMBIATA (sul gruppo corrente della sorgente, solo per una bozza con un registro: motivo
//     contenuto, altra_occorrenza, gruppo_non_proposto) e NON_INCLUSO_CONDIVISO (lo STEP di un nodo non incluso e' la
//     struttura di un altro prodotto per il motore). STEP_SECONDARIO_DISPONIBILE non si calcola in V1: il registro delle
//     sorgenti secondarie e il loro gesto sono V5 (direttiva 39 §5, D-AR12 [U]); la condizione (e) la copre
//     riepilogo3_completamento.go con le dichiarazioni del Fascicolo.

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/valutazione"
)

// Gli errori del calcolo del motore che R4 e R5 riconoscono: l'adattatore li avvolge (errors.Is).
var (
	ErrMotoreOccupato  = errors.New("motore A: troppi calcoli in corso")
	ErrMotoreScaduto   = errors.New("motore A: tempo scaduto")
	ErrMotoreNonPronto = errors.New("motore A: regole non pronte")
)

// I codici dell'errore del motore (EM §8.2, §9.5) e degli avvisi del motore (EM §8.2).
const (
	MotoreErroreOccupato            = "CALCOLO_OCCUPATO"
	MotoreErroreScaduto             = "CALCOLO_SCADUTO"
	CodiceCalcoloScaduto            = "CALCOLO_SCADUTO"
	CodiceErroreCalcolo             = "ERRORE_CALCOLO"
	AvvisoMotorePropostaCambiata    = "PROPOSTA_CAMBIATA"
	AvvisoMotoreNonInclusoCondiviso = "NON_INCLUSO_CONDIVISO"
)

// I motivi di PROPOSTA_CAMBIATA (EM §8.2).
const (
	PropostaCambiataContenuto         = "contenuto"
	PropostaCambiataAltraOccorrenza   = "altra_occorrenza"
	PropostaCambiataGruppoNonProposto = "gruppo_non_proposto"
)

// IngressoMotoreR4: il calcolo del Motore A della RFQ come lo prepara l'adattatore: il prodotto valutato (nil se il motore
// non lo valuta), i suoi gruppi, le occorrenze non attive, l'importazione di un gruppo del prodotto (propostabom.ImportaGruppo
// con la fotografia del calcolo) e i gruppi degli altri prodotti finiti valutati.
type IngressoMotoreR4 struct {
	Prodotto    *valutazione.ProdottoValutato
	Gruppi      []propostabom.Gruppo
	NonAttivi   propostabom.NonAttivi
	Importa     func(gruppo string) (importazione.Registro, error)
	AltriGruppi map[uuid.UUID][]propostabom.Gruppo
	// StepCandidati: gli STEP candidati sorgente dei finiti dello stesso calcolo (StepCandidatiDalMotore, G39-02 [U]): una
	// PB su uno di loro e' in conflitto (step_candidato_del_finito). nil = il ripiego.
	StepCandidati *StepCandidati
}

// EsitoMotoreR4: il calcolo del motore per R4 (con ?avvisi_motore=1): l'ingresso, oppure l'errore.
type EsitoMotoreR4 struct {
	Ingresso *IngressoMotoreR4
	Err      error
}

// CalcoloMotoreR5: il calcolo del motore nella connessione S di R5 (tx: S, REPEATABLE READ READ ONLY, dopo il lucchetto
// della RFQ), dato dall'adattatore.
type CalcoloMotoreR5 func(ctx context.Context, tx pgx.Tx, thread, prodotto uuid.UUID) (*IngressoMotoreR4, error)

// RiassuntoDifferenze: il riassunto delle differenze fra il registro della bozza e la proposta di adesso.
type RiassuntoDifferenze struct {
	Nodi   int      `json:"nodi"`
	Archi  int      `json:"archi"`
	Gruppo []string `json:"gruppo,omitempty"`
}

// AvvisoMotoreR4: un avviso del motore (avvisi_motore[]), firmato.
type AvvisoMotoreR4 struct {
	Codice         string                      `json:"codice"`
	Motivo         string                      `json:"motivo,omitempty"`
	GruppoCorrente *propostabom.GruppoCorrente `json:"gruppo_corrente,omitempty"`
	AllegatoID     *uuid.UUID                  `json:"allegato_id,omitempty"`
	Differenze     *RiassuntoDifferenze        `json:"differenze,omitempty"`
	Nodi           []string                    `json:"nodi,omitempty"`
	Chiavi         []string                    `json:"chiavi,omitempty"`
	Prodotti       []uuid.UUID                 `json:"prodotti,omitempty"`
	Testo          string                      `json:"testo"`
}

// esitoMotore: la parte del motore del riepilogo.
type esitoMotore struct {
	avvisi     []json.RawMessage
	conflitti  []string
	calcolati  bool
	errore     *ErroreMotoreR4
	conflittiC bool // i conflitti del prodotto sono calcolati (il motore valuta il prodotto)
}

// erroreMotoreDa: il codice e il testo di un errore del calcolo.
func erroreMotoreDa(err error) *ErroreMotoreR4 {
	switch {
	case errors.Is(err, ErrMotoreOccupato), errors.Is(err, ErrMotoreNonPronto):
		return &ErroreMotoreR4{Codice: MotoreErroreOccupato, Testo: "Il motore è occupato o non pronto: gli avvisi del motore non sono calcolati. Riprova il riepilogo."}
	case errors.Is(err, ErrMotoreScaduto):
		return &ErroreMotoreR4{Codice: MotoreErroreScaduto, Testo: "Il calcolo del motore non è finito in tempo: gli avvisi del motore non sono calcolati. Riprova il riepilogo."}
	default:
		return &ErroreMotoreR4{Codice: MotoreErroreCalcolo, Testo: "Il calcolo del motore non è riuscito: gli avvisi del motore non sono calcolati. Riprova il riepilogo."}
	}
}

// motore: gli avvisi e i conflitti del motore (commento di testa).
func (a *analisiR4) motore() esitoMotore {
	out := esitoMotore{avvisi: []json.RawMessage{}, conflitti: []string{}}
	if !a.in.avvisiMotore {
		return out
	}
	em := a.in.esitoMotore
	switch {
	case em == nil:
		out.errore = &ErroreMotoreR4{Codice: MotoreErroreCalcolo, Testo: "Il motore non è stato calcolato per questo riepilogo: riprova."}
		return out
	case em.Err != nil:
		out.errore = erroreMotoreDa(em.Err)
		return out
	case em.Ingresso == nil:
		out.errore = &ErroreMotoreR4{Codice: MotoreErroreCalcolo, Testo: "Il calcolo del motore non ha dato un esito: riprova."}
		return out
	}
	ing := em.Ingresso
	var avvisi []AvvisoMotoreR4
	if pv := ing.Prodotto; pv != nil {
		out.conflittiC = true
		visti := map[string]bool{}
		for _, xx := range [][]string{pv.BOM.Nomenclatura.Conflitti, pv.BOM.Gerarchia.Conflitti, pv.Smistamento.Conflitti} {
			for _, x := range xx {
				if !visti[x] {
					visti[x] = true
					out.conflitti = append(out.conflitti, x)
				}
			}
		}
		sort.Strings(out.conflitti)
	}
	// PROPOSTA_CAMBIATA: il gruppo corrente della sorgente, solo per una bozza con un registro
	if reg, s := a.in.registro, a.s.Sorgente; reg != nil && s != nil && s.AllegatoID != nil {
		src := propostabom.SorgenteBozza{AllegatoID: *s.AllegatoID, Sha256: s.Sha256, Radice: reg.Contenuto.Gruppo.Radice, Gruppo: s.Gruppo}
		gc := propostabom.GruppoCorrenteDellaSorgente(ing.Gruppi, ing.NonAttivi, src)
		switch gc.Come {
		case propostabom.ComeAssente:
			x := gc
			avvisi = append(avvisi, AvvisoMotoreR4{Codice: AvvisoMotorePropostaCambiata, Motivo: PropostaCambiataGruppoNonProposto, GruppoCorrente: &x,
				AllegatoID: gc.AllegatoID, Testo: "Il motore non propone più la struttura da cui viene la bozza: la bozza resta tua, ma la proposta di adesso è un'altra."})
		case propostabom.ComeAltraOccorrenza:
			x := gc
			avvisi = append(avvisi, AvvisoMotoreR4{Codice: AvvisoMotorePropostaCambiata, Motivo: PropostaCambiataAltraOccorrenza, GruppoCorrente: &x,
				AllegatoID: gc.AllegatoID, Testo: "La struttura della bozza è ora la BOM di lavoro su un'altra copia dello stesso file."})
		case propostabom.ComeStessaOccorrenza:
			if ing.Importa == nil {
				out.errore = &ErroreMotoreR4{Codice: MotoreErroreCalcolo, Testo: "La proposta di adesso non si legge: gli avvisi del motore non sono calcolati."}
				return esitoMotore{avvisi: []json.RawMessage{}, conflitti: []string{}, errore: out.errore}
			}
			nuovo, err := ing.Importa(gc.Chiave)
			if err != nil {
				out.errore = &ErroreMotoreR4{Codice: MotoreErroreCalcolo, Testo: "La proposta di adesso non si importa: gli avvisi del motore non sono calcolati."}
				return esitoMotore{avvisi: []json.RawMessage{}, conflitti: []string{}, errore: out.errore}
			}
			if nuovo.Contenuto.ImprontaContenuto != reg.Contenuto.ImprontaContenuto {
				conf := propostabom.Confronta(reg.Contenuto, nuovo.Contenuto)
				x := gc
				av := AvvisoMotoreR4{Codice: AvvisoMotorePropostaCambiata, Motivo: PropostaCambiataContenuto, GruppoCorrente: &x,
					Differenze: &RiassuntoDifferenze{Nodi: len(conf.Differenze.Nodi), Archi: len(conf.Differenze.Archi), Gruppo: conf.Differenze.Gruppo},
					Testo:      "La proposta del motore è cambiata da quando la bozza l'ha importata: con «Aggiorna proposta» la rivedi, la conferma non la cambia."}
				av.Nodi = a.nodiNonToccatiCambiati(conf.Differenze.Nodi)
				avvisi = append(avvisi, av)
			}
		}
	}
	// NON_INCLUSO_CONDIVISO: lo STEP del nodo non incluso e' la struttura di un altro prodotto per il motore
	for _, d := range a.dec[DecisioneNonIncludere] {
		sha, _ := shaEChiave(d.Casella.Rif)
		if sha == "" {
			continue
		}
		var prodotti []uuid.UUID
		for p, gg := range ing.AltriGruppi {
			if p == a.s.ProdottoID {
				continue
			}
			for _, g := range gg {
				if g.Sha256 == sha {
					prodotti = append(prodotti, p)
					break
				}
			}
		}
		if len(prodotti) == 0 {
			continue
		}
		sort.Slice(prodotti, func(i, j int) bool { return prodotti[i].String() < prodotti[j].String() })
		al := d.Casella.AllegatoID
		avvisi = append(avvisi, AvvisoMotoreR4{Codice: AvvisoMotoreNonInclusoCondiviso, AllegatoID: &al, Chiavi: []string{d.Casella.Rif}, Prodotti: prodotti,
			Testo: "Lo STEP del nodo non incluso è anche la struttura di altri prodotti: la non inclusione vale solo per questa BOM."})
	}
	for _, av := range avvisi {
		b, err := json.Marshal(av)
		if err != nil {
			continue
		}
		out.avvisi = append(out.avvisi, canonico(json.RawMessage(b)))
	}
	out.calcolati = true
	return out
}

// nodiNonToccatiCambiati: le chiavi dei nodi che portano ancora l'identita' proposta (modo proposta) e la cui voce cambia
// nella proposta di adesso.
func (a *analisiR4) nodiNonToccatiCambiati(dd []propostabom.DifferenzaNodo) []string {
	cambiate := map[string]bool{}
	for _, d := range dd {
		if d.Voce != "" {
			cambiate[d.Voce] = true
		}
	}
	var out []string
	for _, k := range a.chiavi {
		n := a.nodi[k]
		if !a.incluso(k) || n.Identita.Modo != ModoProposta {
			continue
		}
		for _, c := range n.Caselle {
			if cambiate["n:"+c.AllegatoID.String()+":"+c.Rif] {
				out = append(out, k)
				break
			}
		}
	}
	return out
}
