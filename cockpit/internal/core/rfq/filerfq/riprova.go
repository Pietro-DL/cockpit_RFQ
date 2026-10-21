package filerfq

// riprova.go — «Riprova la rimozione dalla cartella ALTRO» (V1, direttiva 35, blocco C-6a; U2 §3.5): il gesto visibile
// sulla riga di un file scartato la cui copia in ALTRO e' ancora da togliere (di solito perche' la scrittura sul NAS era
// spenta quando lo scarto e' stato registrato). Nient'altro riprende da solo quelle copie: ne' il custode ne' l'avvio
// (come AllineaCoda). Acceso solo con la scrittura sul NAS accesa; lo stesso protocollo della rimozione (§3.7).

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// GestoRiprovaRimozione: il nome del gesto (il corpo e' quello dei gesti della Sorgente: chiave d'intento e versione
// attesa del file scartato).
const GestoRiprovaRimozione = "riprova_rimozione"

// RiprovaRimozione e' il gesto: l'occorrenza scartata della RFQ con la versione vista, almeno una copia degli Altri file
// da rilasciare per lo scarto, la scrittura accesa (altrimenti 503 NON_CONFIGURATO con il motivo); poi un passo del
// rilascio per ogni copia (RilasciaCopia, lo stesso protocollo, fuori transazione). Il gesto e' idempotente da se':
// ripeterlo rifa lo stesso passo, che su una copia gia' rilasciata non fa niente. La chiave d'intento non si registra
// in esito_intento: ck_esito_intento_gesto non ha questo gesto (richiesta alla corsia persistenza) [T]. Lo stato del
// file non cambia: resta scartato, con la sua versione.
func (s *Servizio) RiprovaRimozione(ctx context.Context, thread, allegato, utente uuid.UUID, c CorpoGesto) (*EsitoGesto, error) {
	if ok, motivo := s.disponibile(); !ok {
		return nil, conservazioneNonDisponibile(motivo)
	}
	if c.Scollega != nil || c.Perimetro != nil || c.NVoci != nil || c.ConfermaUnicaCopia != nil {
		return nil, corpoNonValido("", "Niente è cambiato: «Riprova la rimozione» porta solo la chiave e la versione del file.")
	}
	q := db.New(s.Pool)
	t, err := q.GetThread(ctx, thread)
	if err != nil {
		return nil, rfqNonTrovata()
	}
	if t.UnitoIn.Valid {
		return nil, rfqUnita()
	}
	o, err := leggiOccorrenza(ctx, q, thread, allegato)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, fuoriRFQ("")
	}
	if o.Stato != StatoScartato {
		return nil, rifiuto(http.StatusUnprocessableEntity, "RIMOZIONE_NON_DISPONIBILE",
			"Niente da riprovare: il file non è scartato in questa RFQ.", extra("allegato_id", allegato, "motivo", "non_scartato"))
	}
	if o.Versione != *c.VersioneAttesa {
		return nil, fileVersione(o.StatoCiclo(), o.Versione)
	}
	copie, err := q.ListCopieConservazioneAllegato(ctx, allegato)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, x := range copie {
		if x.ThreadID == thread && x.Stato == "da_rilasciare" && x.Causa.Valid && x.Causa.String == "scartato" {
			ids = append(ids, x.ConservazioneID)
		}
	}
	if len(ids) == 0 {
		return nil, rifiuto(http.StatusUnprocessableEntity, "RIMOZIONE_NON_DISPONIBILE",
			"Niente da riprovare: nella cartella ALTRO non c'è una copia di questo file da togliere.",
			extra("allegato_id", allegato, "motivo", "niente_da_togliere"))
	}
	for _, id := range ids {
		if _, err := s.RilasciaCopia(ctx, id, false); err != nil {
			return nil, err
		}
	}
	_, cons, err := statiCorrenti(ctx, q, thread, []uuid.UUID{allegato})
	if err != nil {
		return nil, err
	}
	e := &EsitoGesto{Esito: "riprovata", ChiaveIdempotenza: *c.ChiaveIdempotenza, AllegatoID: allegato, Stato: StatoScartato,
		Versione: o.Versione, Occorrenze: []uuid.UUID{allegato}, Conservazioni: cons, Testo: "Scarto registrato."}
	for _, x := range cons {
		if x.AllegatoID == allegato {
			e.Testo = "Scarto registrato. " + testoCopiaScartata(x)
		}
	}
	s.log().Info("ciclo dei file: «Riprova la rimozione dalla cartella ALTRO»", "allegato", allegato, "utente", utente, "copie", len(ids))
	return e, nil
}
