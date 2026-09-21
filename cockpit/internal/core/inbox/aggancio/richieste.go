package aggancio

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// I CANDIDATI VERSO UNA RICHIESTA AI FORNITORI (blocco 7B.2)
//
// La posta di un fornitore non si aggancia a una RFQ per somiglianza con la RFQ: si aggancia alla
// RICHIESTA che gli abbiamo mandato, e da quella alla RFQ cliente. Le regole, nell'ordine di
// fiducia: R0 (In-Reply-To/References verso la nostra mail di richiesta), R1 (la conversazione di
// Outlook della nostra mail), R3f (un codice cliente citato che appartiene a una RFQ con una
// richiesta a QUESTO fornitore). Ogni regola scrive una riga in `candidato_richiesta`; chi decide
// e' l'operatore («e' la risposta a X per la RFQ Y»).

// IngressoRichieste e' cio' che serve: fatti gia' estratti, niente testo.
type IngressoRichieste struct {
	MessaggioID     uuid.UUID
	FornitoreID     uuid.UUID
	ConversazioneID uuid.UUID
	InReplyTo       string
	Riferimenti     []string
	// Codici sono i codici DI FAMIGLIA dei clienti che hanno richieste aperte a questo fornitore.
	// Un numero pescato dall'estrattore generico non ci entra: S235JR e ISO 2768 non sono codici
	// di nessuno (IB8).
	Codici []string
}

// CalcolaRichieste interroga il database e restituisce i candidati ordinati. Non scrive niente.
func CalcolaRichieste(ctx context.Context, q *db.Queries, in IngressoRichieste) ([]domain.CandidatoRichiesta, error) {
	var out []domain.CandidatoRichiesta
	visto := map[string]bool{}
	agg := func(r db.RichiestaFornitore, regola, evidenza string) {
		k := r.RichiestaID.String() + "|" + regola
		if visto[k] {
			return
		}
		visto[k] = true
		out = append(out, domain.CandidatoRichiesta{RichiestaID: r.RichiestaID.String(), Regola: regola,
			Punteggio: domain.PuntiRichiesta[regola], Evidenza: evidenza})
	}
	if chiavi := ChiaviCitate(in.InReplyTo, in.Riferimenti); len(chiavi) > 0 {
		righe, err := q.RichiestePerChiaviCitate(ctx, db.RichiestePerChiaviCitateParams{Chiavi: chiavi, FornitoreID: in.FornitoreID})
		if err != nil {
			return nil, fmt.Errorf("R0 richieste: %w", err)
		}
		for _, r := range righe {
			campo := "References"
			if strings.Contains(in.InReplyTo, strings.Trim(r.ChiaveEsterna, "<>")) {
				campo = "In-Reply-To"
			}
			agg(rigaRichiesta(r), domain.RichiestaR0Reply, fmt.Sprintf("%s punta alla nostra richiesta (%s)", campo, r.ChiaveEsterna))
		}
	}
	righe, err := q.RichiestePerConversazione(ctx, db.RichiestePerConversazioneParams{ConversazioneID: in.ConversazioneID, FornitoreID: in.FornitoreID})
	if err != nil {
		return nil, fmt.Errorf("R1 richieste: %w", err)
	}
	for _, r := range righe {
		agg(r, domain.RichiestaR1Conversazione, "stessa conversazione di Outlook della nostra richiesta")
	}
	if len(in.Codici) > 0 {
		righe, err := q.RichiestePerCodiciFornitore(ctx, db.RichiestePerCodiciFornitoreParams{FornitoreID: in.FornitoreID, Codici: maiuscole(in.Codici)})
		if err != nil {
			return nil, fmt.Errorf("R3f richieste: %w", err)
		}
		for _, r := range righe {
			agg(rigaRichiestaCodice(r), domain.RichiestaR3fCodice, "il codice "+r.Codice+" e' della RFQ per cui abbiamo chiesto l'offerta a questo fornitore")
		}
	}
	// i piu' forti in cima; a parita' l'ordine di lettura
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Punteggio > out[j-1].Punteggio; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// SalvaCandidatiRichiesta sostituisce i candidati di un messaggio: sono una fotografia di adesso.
func SalvaCandidatiRichiesta(ctx context.Context, q *db.Queries, messaggioID uuid.UUID, c []domain.CandidatoRichiesta) error {
	if err := q.CancellaCandidatiRichiesta(ctx, messaggioID); err != nil {
		return err
	}
	for _, k := range c {
		rid, err := uuid.Parse(k.RichiestaID)
		if err != nil {
			continue
		}
		if err := q.InsertCandidatoRichiesta(ctx, db.InsertCandidatoRichiestaParams{
			MessaggioID: messaggioID, RichiestaID: rid, Regola: db.RegolaRichiesta(k.Regola),
			Punteggio: int16(k.Punteggio), Evidenza: tronca(k.Evidenza, 500)}); err != nil {
			return err
		}
	}
	return nil
}

// RichiesteManuali e' la regola RF_oggetto (7B.2, «mandata a mano»): una NOSTRA mail a un fornitore
// che cita un codice identificativo di una RFQ aperta, di qualunque cliente. Il risultato e' un
// candidato per RFQ, con l'evidenza; la proposta e' «richiesta a X per la RFQ Y», e la conferma
// crea la richiesta. Un thread nuovo non nasce mai da qui (IB3).
func RichiesteManuali(ctx context.Context, q *db.Queries, codici []string) ([]domain.Candidato, error) {
	if len(codici) == 0 {
		return nil, nil
	}
	righe, err := q.ThreadApertePerCodici(ctx, maiuscole(codici))
	if err != nil {
		return nil, fmt.Errorf("RF_oggetto: %w", err)
	}
	var out []domain.Candidato
	visto := map[uuid.UUID]bool{}
	for _, r := range righe {
		if visto[r.ThreadID] {
			continue
		}
		visto[r.ThreadID] = true
		out = append(out, domain.Candidato{ThreadID: r.ThreadID.String(), Regola: domain.RichiestaRFOggetto,
			Punteggio: domain.PuntiRichiesta[domain.RichiestaRFOggetto],
			Evidenza:  fmt.Sprintf("il codice %s e' un identificativo della RFQ «%s» di %s", r.Codice, r.Oggetto.String, r.Cliente)})
	}
	return out, nil
}

func rigaRichiesta(r db.RichiestePerChiaviCitateRow) db.RichiestaFornitore {
	return db.RichiestaFornitore{RichiestaID: r.RichiestaID, ThreadID: r.ThreadID, FornitoreID: r.FornitoreID, Lavorazione: r.Lavorazione,
		Codici: r.Codici, MessaggioID: r.MessaggioID, Stato: r.Stato, InviataIl: r.InviataIl, OffertaRicevutaIl: r.OffertaRicevutaIl, DeclinataIl: r.DeclinataIl, Note: r.Note, CreataDa: r.CreataDa, CreataIl: r.CreataIl}
}

func rigaRichiestaCodice(r db.RichiestePerCodiciFornitoreRow) db.RichiestaFornitore {
	return db.RichiestaFornitore{RichiestaID: r.RichiestaID, ThreadID: r.ThreadID, FornitoreID: r.FornitoreID, Lavorazione: r.Lavorazione,
		Codici: r.Codici, MessaggioID: r.MessaggioID, Stato: r.Stato, InviataIl: r.InviataIl, OffertaRicevutaIl: r.OffertaRicevutaIl, DeclinataIl: r.DeclinataIl, Note: r.Note, CreataDa: r.CreataDa, CreataIl: r.CreataIl}
}
