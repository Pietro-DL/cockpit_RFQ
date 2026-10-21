package distintabozza

// riassegnazione3.go: l'evento riassegnazione_sorgente del giornale (vocabolario 2, solo del server; giro G39-ter).
// G39-03 [U] (20/10): prima di R5, «la bozza appena creata deve essere annullata siccome la sorgente step cambia»;
// messaggio 43, caso 1 [U] (21/10): «i registri delle bozze devono annotare la revoca ed il successivo riassegnamento».
// Quando S8 conferma per un prodotto uno STEP diverso da quello da cui e' nata la sua bozza aperta (nata da una sorgente
// S8, partenza step, prima di qualunque R5 del prodotto), la bozza vecchia si abbandona e la nuova nasce dalla proposta
// (CreaFormato3 con Abbandona); nella stessa transazione, sulla versione 1 della bozza nuova, questo evento lega le due
// bozze, le due sorgenti e la revoca della sorgente vecchia in fonte_step_evento. La forma e' [T-39] della corsia
// sorgente, senza DDL: distinta_bozza_evento accetta un'azione nuova (ck_dbe_azione ne controlla solo la forma; il
// vocabolario e' versionato nel Go) e il trigger distinta_bozza_evento_nasce vuole la bozza aperta alla versione corrente,
// nella transazione che l'ha salvata: per questo l'evento lo scrive chi crea, prima del COMMIT. Non cambia lo snapshot e
// non si annulla (AzioniSoloDelServer2).

import (
	"context"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// SorgenteRiassegnata: una sorgente STEP nell'evento (l'occorrenza, il contenuto, il gruppo).
type SorgenteRiassegnata struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	Sha256     string    `json:"sha256"`
	Gruppo     string    `json:"gruppo,omitempty"`
}

// Riassegnazione: il contenuto dell'evento: la bozza abbandonata con la sua versione e la sua sorgente (prima), la
// sorgente nuova con la conferma S8 che l'ha registrata e la revoca della vecchia (dopo), il motivo della revoca.
type Riassegnazione struct {
	BozzaAbbandonata    uuid.UUID
	VersioneAbbandonata int
	Prima, Dopo         SorgenteRiassegnata
	RevocaEvento        int64
	Motivo              string
}

// ScriviRiassegnazione aggiunge l'evento riassegnazione_sorgente (origine server) alla bozza appena creata, alla sua
// versione corrente, dopo gli eventi che la creazione ha gia' scritto. Va chiamata nella transazione della creazione
// (Creazione3.PrimaDelCommit): fuori il trigger della 0022 la rifiuta (P4X04).
func ScriviRiassegnazione(ctx context.Context, q *db.Queries, thread, bozza uuid.UUID, versione int, utente uuid.UUID, r Riassegnazione) error {
	n, err := q.UltimoEventoDistintaBozza(ctx, db.UltimoEventoDistintaBozzaParams{BozzaID: bozza, ThreadID: thread})
	if err != nil {
		return err
	}
	prima := canonico(map[string]any{"bozza_id": r.BozzaAbbandonata, "versione": r.VersioneAbbandonata, "sorgente": r.Prima})
	dopo := canonico(map[string]any{"sorgente": r.Dopo, "revoca_evento": r.RevocaEvento})
	if err := q.InsertDistintaBozzaEvento(ctx, db.InsertDistintaBozzaEventoParams{BozzaID: bozza, N: n + 1, Versione: int32(versione),
		Vocabolario: VocabolarioFormato3, Azione: AzRiassegnazioneSorgente, Prima: rawONull(prima), Dopo: rawONull(dopo), Origine: "server",
		Motivo: testo(troncaMotivo(r.Motivo, MaxMotivo)), UtenteID: utente}); err != nil {
		return erroreDiScrittura(err, "crea")
	}
	return nil
}

// troncaMotivo: il testo in al piu' max caratteri (con «...» in fondo se tagliato).
func troncaMotivo(s string, max int) string {
	r := []rune(s)
	if max < 4 || len(r) <= max {
		if max >= 0 && len(r) > max {
			return string(r[:max])
		}
		return s
	}
	return string(r[:max-3]) + "..."
}
