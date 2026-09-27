package calibrazione

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// LA POSTA (A5.16.6)
//
// Due fonti, che non si sommano:
//   - le FOTOGRAFIE: il motivo JSON che ogni decisione scrive da M3 (classificazione.Scelta). Dicono il
//     rango della RFQ scelta nell'elenco che l'operatore aveva davanti, e con quale scala di regole;
//   - la RETRO: i messaggi agganciati a mano prima di M3, con i candidati rimasti in `candidato_aggancio`
//     (le righe di un messaggio deciso non si riscrivono). Si ordinano come il pannello di oggi, con le
//     righe delle regole di prima lette verso il basso: è una stima di come avrebbe fatto la scala nuova
//     su scelte già fatte, non una fotografia. Un messaggio con la fotografia non entra nella retro.

// Posta è la parte della posta del rapporto.
type Posta struct {
	Misure []RigaPosta
	Retro  []RigaRetro
	// RetroSenzaCandidati sono i messaggi agganciati a mano, senza fotografia, che non avevano nessun
	// candidato: l'operatore ha scelto senza una lista, e non c'è un primo da misurare.
	RetroSenzaCandidati int
}

// Per dice come è raggruppata una riga.
const (
	PerTotale       = "totale"
	PerLivello      = "livello"
	PerTipo         = "tipo"
	PerFascia       = "fascia"
	PerTipoFascia   = "tipo e fascia"
	valoreQualsiasi = "tutti"
)

// perRaggruppamento traduce GROUPING(livello, tipo, fascia) di MisuraPosta: il bit è 1 per la colonna che
// NON è nel raggruppamento (livello = 4, tipo = 2, fascia = 1).
var perRaggruppamento = map[int32]string{7: PerTotale, 3: PerLivello, 5: PerTipo, 6: PerFascia, 4: PerTipoFascia}

// ordinePer è l'ordine delle righe nella stampa.
var ordinePer = map[string]int{PerTotale: 0, PerLivello: 1, PerTipo: 2, PerFascia: 3, PerTipoFascia: 4}

// RigaPosta è una cella delle fotografie: una versione delle regole, un raggruppamento. Livello, Tipo e
// Fascia sono quelli del PRIMO proposto; vuoti quando la riga non è raggruppata per quella colonna.
//
// Una decisione è un MESSAGGIO, con la sua ultima fotografia: un doppio clic, o un «Ignora» corretto poi
// in un «Aggancia», contano una volta sola, con il gesto che vale adesso (Ridecisi li conta a parte). Se
// la decisione che vale adesso è stata presa senza candidati, il messaggio non è nel campione, anche se
// una decisione di prima ne aveva.
type RigaPosta struct {
	Regole                string
	Per                   string
	Livello, Tipo, Fascia string
	Decisioni             int // tutti i gesti con una lista: aggancia, nuova RFQ, ignora
	AlPrimo               int // la prima card era la risposta giusta (solo un aggancio può esserlo)
	NeiPrimi3             int
	Agganci               int
	AgganciAlPrimo        int // la RFQ agganciata era la prima card
	NuoveRFQ              int // «Crea RFQ» con candidati: il primo non era quello giusto
	Ignorati              int
	FuoriLista            int // «Altra RFQ…» verso una RFQ che non era fra le card
	PariMerito            int
	Ridecisi              int // messaggi con una decisione prima di quella che conta
	MRR                   float64
}

// Precisione è la precision@1 della cella come la definisce r1_mail.md §7.3: sui soli agganci, quante
// volte la RFQ agganciata era la prima card. «Ignora» e «Crea RFQ» non scelgono una RFQ e non entrano.
func (r RigaPosta) Precisione() (float64, bool) { return Precisione(r.AgganciAlPrimo, r.Agganci) }

// PrimoGiusto è la misura «per livello» di §7.3: su tutte le decisioni con una lista, quante volte la
// prima card era la risposta giusta. Un «ignora» o una RFQ nuova con candidati contano come primo
// sbagliato: dicono che il primo proposto non andava proposto.
func (r RigaPosta) PrimoGiusto() (float64, bool) { return Precisione(r.AlPrimo, r.Decisioni) }

// RigaRetro è una cella della retro. Livello, Tipo e Fascia sono del primo proposto, com'è letto oggi.
type RigaRetro struct {
	Per                   string
	Livello, Tipo, Fascia string
	Messaggi              int
	AlPrimo               int // la RFQ agganciata era la prima card: è AgganciAlPrimo delle fotografie
	FuoriLista            int // la RFQ scelta non era fra i candidati
	DiPrima               int // il primo proposto era una riga delle regole di prima (A5.10)
}

// Precisione è la precision@1 della cella.
func (r RigaRetro) Precisione() (float64, bool) { return Precisione(r.AlPrimo, r.Messaggi) }

// MisuraPosta legge le fotografie e la retro.
func MisuraPosta(ctx context.Context, q *db.Queries, dal *time.Time) (Posta, error) {
	var p Posta
	righe, err := q.MisuraPosta(ctx, dal)
	if err != nil {
		return p, fmt.Errorf("calibrazione della posta: %w", err)
	}
	for _, r := range righe {
		p.Misure = append(p.Misure, RigaPosta{Regole: r.Regole, Per: perRaggruppamento[r.Raggruppamento],
			Livello: r.Livello, Tipo: r.Tipo, Fascia: r.Fascia, Decisioni: int(r.Decisioni), AlPrimo: int(r.AlPrimo),
			NeiPrimi3: int(r.NeiPrimi3), Agganci: int(r.Agganci), AgganciAlPrimo: int(r.AgganciAlPrimo), NuoveRFQ: int(r.NuoveRfq),
			Ignorati: int(r.Ignorati), FuoriLista: int(r.FuoriLista), PariMerito: int(r.PariMerito), Ridecisi: int(r.Ridecisi), MRR: r.Mrr})
	}
	// l'ordine della stampa: per versione delle regole, il totale e poi i raggruppamenti, come la retro
	sort.SliceStable(p.Misure, func(a, b int) bool {
		x, y := p.Misure[a], p.Misure[b]
		if x.Regole != y.Regole {
			return x.Regole < y.Regole
		}
		return ordinePer[x.Per] < ordinePer[y.Per]
	})
	retro, err := q.RetroPosta(ctx, dal)
	if err != nil {
		return p, fmt.Errorf("calibrazione della posta (retro): %w", err)
	}
	p.Retro, p.RetroSenzaCandidati = Retro(retro)
	return p, nil
}

// Retro ordina i candidati di ogni messaggio come il pannello di oggi (classificazione.RaggruppaEOrdina,
// con le righe di prima lette verso il basso) e conta quante volte la prima card era la RFQ che
// l'operatore ha scelto. Le righe arrivano ordinate per messaggio, con lo spareggio di ListCandidatiAggancio.
func Retro(righe []db.RetroPostaRow) (out []RigaRetro, senzaCandidati int) {
	celle := map[[4]string]*RigaRetro{}
	conta := func(per, livello, tipo, fascia string, giusto, fuori, diPrima bool) {
		k := [4]string{per, livello, tipo, fascia}
		c, ok := celle[k]
		if !ok {
			c = &RigaRetro{Per: per, Livello: livello, Tipo: tipo, Fascia: fascia}
			celle[k] = c
		}
		c.Messaggi++
		if giusto {
			c.AlPrimo++
		}
		if fuori {
			c.FuoriLista++
		}
		if diPrima {
			c.DiPrima++
		}
	}
	for i := 0; i < len(righe); {
		j := i
		for j < len(righe) && righe[j].MessaggioID == righe[i].MessaggioID {
			j++
		}
		var cand []classificazione.Candidato
		for _, r := range righe[i:j] {
			if !r.Candidato.Valid {
				continue
			}
			tipo, diPrima := classificazione.LeggiRiga(r.Regola, int(r.Punteggio), r.Evidenza)
			cand = append(cand, classificazione.Candidato{ThreadID: r.Candidato.UUID.String(), Regola: r.Regola,
				Punteggio: int(r.Punteggio), Evidenza: r.Evidenza, Chiuso: r.Chiuso, Tipo: tipo, DiPrima: diPrima})
		}
		scelto := righe[i].Scelto.UUID.String()
		i = j
		if len(cand) == 0 {
			senzaCandidati++
			continue
		}
		g := classificazione.RaggruppaEOrdina(cand)
		primo := g[0]
		giusto := primo.ThreadID == scelto
		fuori := true
		for _, x := range g {
			fuori = fuori && x.ThreadID != scelto
		}
		livello, tipo, fascia := primo.Livello.Chiave(), primo.Tipo, classificazione.FasciaScore(primo.Score)
		diPrima := primo.Evidenze[0].DiPrima
		conta(PerTotale, "", "", "", giusto, fuori, diPrima)
		conta(PerLivello, livello, "", "", giusto, fuori, diPrima)
		conta(PerTipo, "", tipo, "", giusto, fuori, diPrima)
		conta(PerFascia, "", "", fascia, giusto, fuori, diPrima)
		conta(PerTipoFascia, "", tipo, fascia, giusto, fuori, diPrima)
	}
	for _, c := range celle {
		out = append(out, *c)
	}
	sort.Slice(out, func(a, b int) bool {
		x, y := out[a], out[b]
		if ordinePer[x.Per] != ordinePer[y.Per] {
			return ordinePer[x.Per] < ordinePer[y.Per]
		}
		if x.Livello != y.Livello {
			return x.Livello < y.Livello
		}
		if x.Tipo != y.Tipo {
			return x.Tipo < y.Tipo
		}
		return x.Fascia < y.Fascia
	})
	return out, senzaCandidati
}

func oTutti(s string) string {
	if s == "" {
		return valoreQualsiasi
	}
	return s
}

// scrivi stampa le due tabelle della posta.
func (p Posta) scrivi(w io.Writer) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "POSTA · fotografie delle decisioni (messaggio_aggancio_log, gesti aggancia / nuova_rfq / ignora con almeno un candidato;")
	fmt.Fprintln(w, "una decisione per messaggio, l'ultima: «ridecisi» sono i messaggi che ne avevano già una prima)")
	fmt.Fprintln(w, "precision@1 = sui soli agganci, quante volte la RFQ agganciata era la prima proposta (r1_mail.md §7.3)")
	fmt.Fprintln(w, "primo giusto = su tutte le decisioni, quante volte la prima proposta era la risposta giusta: ignora e RFQ nuova contano come primo sbagliato")
	if len(p.Misure) == 0 {
		fmt.Fprintln(w, "nessuna decisione con la fotografia in questo periodo")
	} else {
		fmt.Fprintln(w, "regole\tper\tlivello del primo\ttipo del primo\tfascia del primo\tagganci\tagganciati al primo\tprecision@1\tdecisioni\tal primo\tprimo giusto\tnei primi 3\tMRR\tnuove RFQ\tignorati\tfuori lista\tpari merito\tridecisi\t")
		for _, r := range p.Misure {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\t%d\t%d\t%s\t%d\t%s\t%d\t%d\t%d\t%d\t%d\t\n", r.Regole, r.Per, oTutti(r.Livello), oTutti(r.Tipo),
				oTutti(r.Fascia), r.Agganci, r.AgganciAlPrimo, CellaPrecisione(r.AgganciAlPrimo, r.Agganci),
				r.Decisioni, r.AlPrimo, CellaPrecisione(r.AlPrimo, r.Decisioni), r.NeiPrimi3, CellaFrequenza(r.MRR, r.Decisioni),
				r.NuoveRFQ, r.Ignorati, r.FuoriLista, r.PariMerito, r.Ridecisi)
		}
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "POSTA · retro: messaggi agganciati a mano senza fotografia (candidato_aggancio contro messaggio.thread_id, ordinati come il pannello di oggi)")
	if len(p.Retro) == 0 {
		fmt.Fprintln(w, "nessun messaggio agganciato a mano con candidati in questo periodo")
	} else {
		fmt.Fprintln(w, "per\tlivello del primo\ttipo del primo\tfascia del primo\tmessaggi\tagganciati al primo\tprecision@1\tfuori lista\tprimo con le regole di prima\t")
		for _, r := range p.Retro {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\t%s\t%d\t%d\t\n", r.Per, oTutti(r.Livello), oTutti(r.Tipo), oTutti(r.Fascia),
				r.Messaggi, r.AlPrimo, CellaPrecisione(r.AlPrimo, r.Messaggi), r.FuoriLista, r.DiPrima)
		}
	}
	fmt.Fprintln(w, "agganciati a mano senza nessun candidato: "+strconv.Itoa(p.RetroSenzaCandidati))
}
