package aggancio

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// LA LETTURA DEI CANDIDATI (Smistamento M1, A5.16.4, D84, D85)
//
// Quello che l'operatore vede di un messaggio orfano: una card per RFQ, ordinate a livelli, con tutte
// le evidenze. Le righe vengono da `candidato_aggancio` (scritte dall'ingest, dal ricalcolo e dal giro
// degli orfani); il candidato del MARCATORE si aggiunge qui, ogni volta, leggendo `bozza` e
// `richiesta_fornitore`: la nostra mail preparata dal Cockpit dice da quale RFQ nasce, e la sua RFQ è
// un candidato molto forte. Niente di tutto questo scrive: è la lettura di una GET (R8), e la mail non
// si aggancia da sola (I14).

// Origine è da dove viene la nostra mail: una bozza preparata dal Cockpit (D84) o una richiesta a un
// fornitore legata dal marcatore (D85).
type Origine struct {
	Via    string // "bozza" | "richiesta"
	Sigla  string // chi l'ha preparata
	Quando time.Time
	// RFQ sono le RFQ per cui è nata: una, o due discordanti (bozza preparata per T1 in risposta a una
	// mail che oggi sta in T2), o nessuna (bozza nata su un orfano mai agganciato).
	RFQ []uuid.UUID
	// Oggetti delle RFQ, nello stesso ordine; OggettoRisposta e DataRisposta sono della mail a cui la
	// bozza rispondeva. Fornitore è quello della richiesta.
	Oggetti         []string
	OggettoRisposta string
	DataRisposta    *time.Time
	Fornitore       string
	Discordi        bool
	// DallaRisposta: la sola RFQ è quella della mail a cui la bozza rispondeva (la bozza nacque su un
	// orfano, agganciato dopo).
	DallaRisposta bool
}

// OriginiCockpit legge le origini di più messaggi con due query, non una per messaggio: serve alla lista
// dell'Inbox (il chip «dal Cockpit») come al pannello.
func OriginiCockpit(ctx context.Context, q *db.Queries, messaggi []uuid.UUID) (map[uuid.UUID][]Origine, error) {
	out := map[uuid.UUID][]Origine{}
	if len(messaggi) == 0 {
		return out, nil
	}
	bozze, err := q.OrigineCockpit(ctx, messaggi)
	if err != nil {
		return nil, fmt.Errorf("origine delle bozze: %w", err)
	}
	for _, b := range bozze {
		o := Origine{Via: "bozza", Sigla: b.Sigla, Quando: b.CreataIl, OggettoRisposta: b.OggettoRisposta.String, DataRisposta: b.DataRisposta}
		oggetti := map[string]string{uuidTesto(b.ThreadID): b.OggettoThread.String, uuidTesto(b.ThreadRisposta): b.OggettoThreadRisposta.String}
		for _, t := range classificazione.OrigineDaBozza(uuidTesto(b.ThreadID), uuidTesto(b.ThreadRisposta)) {
			id, _ := uuid.Parse(t)
			o.RFQ = append(o.RFQ, id)
			o.Oggetti = append(o.Oggetti, oggetti[t])
		}
		o.Discordi = len(o.RFQ) > 1
		o.DallaRisposta = len(o.RFQ) == 1 && !b.ThreadID.Valid
		out[b.MessaggioID] = append(out[b.MessaggioID], o)
	}
	richieste, err := q.OrigineRichiesta(ctx, messaggi)
	if err != nil {
		return nil, fmt.Errorf("origine delle richieste: %w", err)
	}
	for _, r := range richieste {
		out[r.MessaggioID] = append(out[r.MessaggioID], Origine{Via: "richiesta", Sigla: r.Sigla.String, Quando: r.CreataIl,
			RFQ: []uuid.UUID{r.ThreadID}, Fornitore: r.Fornitore})
	}
	return out, nil
}

// RFQ sono i dati di una RFQ candidata che la card mostra.
type RFQ struct {
	Cliente, Oggetto, Cartella, Riferimento string
	DataInizio                              time.Time
}

// Lettura è ciò che il pannello e i form mostrano dei candidati di un messaggio.
type Lettura struct {
	Candidati  []classificazione.CandidatoRFQ
	RFQ        map[string]RFQ
	Origini    []Origine
	PariMerito bool
	// DiPrima: almeno una riga è stata calcolata con le regole di prima (A5.10): il pannello lo dice e
	// offre «Ricalcola».
	DiPrima bool
}

// InLettura legge i candidati di un messaggio e aggiunge quello del marcatore. Non scrive niente.
func InLettura(ctx context.Context, q *db.Queries, messaggioID uuid.UUID) (Lettura, error) {
	out := Lettura{RFQ: map[string]RFQ{}}
	righe, err := q.ListCandidatiAggancio(ctx, messaggioID)
	if err != nil {
		return out, err
	}
	var cand []classificazione.Candidato
	for _, r := range righe {
		tipo, diPrima := classificazione.LeggiRiga(string(r.Regola), int(r.Punteggio), r.Evidenza)
		cand = append(cand, classificazione.Candidato{ThreadID: r.ThreadID.String(), Regola: string(r.Regola),
			Punteggio: int(r.Punteggio), Evidenza: r.Evidenza, Chiuso: r.ThreadStato == db.StatoThreadCHIUSA,
			Tipo: tipo, DiPrima: diPrima})
		out.RFQ[r.ThreadID.String()] = RFQ{Cliente: r.Cliente, Oggetto: r.Oggetto.String, Cartella: r.CartellaRelativa.String,
			Riferimento: r.RiferimentoCliente.String, DataInizio: r.DataInizio}
	}
	origini, err := OriginiCockpit(ctx, q, []uuid.UUID{messaggioID})
	if err != nil {
		return out, err
	}
	out.Origini = origini[messaggioID]
	for i := range out.Origini {
		o := &out.Origini[i]
		for j, t := range o.RFQ {
			th, err := q.GetThread(ctx, t)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return out, err
			}
			if _, ok := out.RFQ[t.String()]; !ok {
				cliente := ""
				if c, err := q.GetCliente(ctx, th.ClienteID); err == nil {
					cliente = c.RagioneSociale
				}
				out.RFQ[t.String()] = RFQ{Cliente: cliente, Oggetto: th.Oggetto.String, Cartella: th.CartellaRelativa.String,
					Riferimento: th.RiferimentoCliente.String, DataInizio: th.DataInizio}
			}
			if j >= len(o.Oggetti) {
				o.Oggetti = append(o.Oggetti, th.Oggetto.String) // la richiesta non porta l'oggetto della sua RFQ
			}
			cand = append(cand, classificazione.NuovoCandidato(t.String(), classificazione.TipoMarcatore, o.frase(t), th.Stato == db.StatoThreadCHIUSA))
		}
	}
	out.Candidati = classificazione.RaggruppaEOrdina(cand)
	out.PariMerito = classificazione.PariMerito(out.Candidati)
	for _, g := range out.Candidati {
		out.DiPrima = out.DiPrima || g.DiPrima
	}
	return out, nil
}

// frase è l'evidenza del marcatore verso una delle RFQ dell'origine.
func (o Origine) frase(t uuid.UUID) string {
	quando := o.Quando.Local().Format("02/01")
	if o.Via == "richiesta" {
		return fmt.Sprintf("è la nostra richiesta a %s, creata dal Cockpit da %s il %s per questa richiesta", o.Fornitore, o.Sigla, quando)
	}
	if len(o.RFQ) > 1 && t == o.RFQ[1] || o.DallaRisposta {
		return fmt.Sprintf("è la nostra mail preparata dal Cockpit da %s il %s in risposta a una mail che oggi sta in questa richiesta (la bozza era per un'altra)", o.Sigla, quando)
	}
	if len(o.RFQ) > 1 {
		return fmt.Sprintf("è la nostra mail preparata dal Cockpit da %s il %s per questa richiesta (ma rispondeva a una mail che oggi sta in un'altra)", o.Sigla, quando)
	}
	return fmt.Sprintf("è la nostra mail preparata dal Cockpit da %s il %s per questa richiesta", o.Sigla, quando)
}
