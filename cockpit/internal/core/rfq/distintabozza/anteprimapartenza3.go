package distintabozza

// Lo stato di partenza della partenza step per l'anteprima di S6 (messaggio 46 [U], 21/10: «le anteprime di quando si
// conferma uno step non compaiono»; collaudo UI46, rilievo B1 [T]). S6 dava la proposta del gruppo con il registro, ma non
// lo stato di partenza: la pagina disegna l'albero proposto dallo stato di partenza (i nodi con le chiavi della bozza,
// le identità e gli archi) e, senza, non mostrava niente, né nell'anteprima né nel dialogo di S8. Qui lo stato di
// partenza è quello che R2 costruirebbe con la partenza step dallo stesso registro: VersioneDaRegistro (importazione-1,
// P-1…P-8) con lo stato della RFQ, il cliente e le identità certificate letti come li legge R2 (ingressoPartenza). Solo
// lettura: nessuna bozza, nessun registro, nessun evento.

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// StatoDiPartenzaDellaProposta: la versione 1 che la partenza step costruirebbe dal contenuto c del registro della
// proposta per il prodotto (commento di testa), letta con q (la transazione in sola lettura di S6). nil, nil quando la
// partenza non si costruisce da quel registro (un registro che non si compone, o che R2 rifiuterebbe con un suo codice,
// per esempio un arco senza quantità): l'anteprima resta senza albero e la pagina lo dice; mai un albero inventato.
func StatoDiPartenzaDellaProposta(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, c importazione.Contenuto) (*Snapshot3, error) {
	in, err := ingressoPartenza(ctx, q, thread, prodotto)
	if err != nil {
		return nil, err
	}
	reg, err := importazione.Componi(c)
	if err != nil {
		return nil, nil
	}
	s, _, err := VersioneDaRegistro(reg, in)
	if err != nil {
		var e *Errore
		if errors.As(err, &e) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}
