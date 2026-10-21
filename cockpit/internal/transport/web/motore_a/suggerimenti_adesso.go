package motore_a

// suggerimenti_adesso.go: i suggerimenti dopo l'apertura di S9 con il calcolo di adesso (giro G39, G39-01 [U], 20/10 sera;
// D-PB02 A, D-PB03 A [U]): «un file arrivato dopo l'apertura di una bozza (prima di R5) entra in
// preassociazioni_proposte[] solo se il motore gli trova un associamento; accettato senza modifiche la provenienza resta
// motore; spostato su un altro componente è corretta».
//
// Come [T-39, corsia motore, rilievo della revisione del giro G39]: S9 dà per ogni bozza aperta formato 3 gli stessi
// suggerimenti di R1 con ?suggerimenti_motore=1, con la stessa funzione della persistenza
// (distintabozza.SuggerimentiDellaBozzaApertaCon) e lo stesso calcolo di adesso (distintabozza.CalcoloAdessoDa
// sull'ingresso del motore di questo calcolo, IngressoMotoreV1, e sulla sorgente della bozza): quelli del registro della
// versione più quelli del calcolo di adesso per i file che il registro della versione non propone, con destinazione
// univoca, motivo dopo_apertura e importazione = l'impronta del registro di adesso, che R2 riconosce («motore» sul suo
// nodo, «corretta» su un altro nodo) e scrive in distinta_importazione. L'ambiguo, o senza destinazione, non è un
// suggerimento (resta una riga proposto con proposta_ambigua, o in da_associare).
//
// La lettura del database è una transazione REPEATABLE READ READ ONLY sua, aperta dopo il calcolo (come R1, che calcola
// il motore e poi legge): la bozza o il registro possono essere cambiati nel frattempo, e allora vale quello che il
// database dice adesso (una bozza chiusa nel frattempo non dà suggerimenti). Nessuna scrittura: il registro di adesso si
// scrive solo con R2.

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// IngressoMotoreV1: il calcolo nella forma del core per il prodotto (R4 ?avvisi_motore=1, R1 ?suggerimenti_motore=1, R2,
// S9): il prodotto valutato (nil se il motore non lo valuta), i suoi gruppi, le occorrenze non attive, l'importazione di
// un gruppo, i gruppi degli altri prodotti finiti e gli STEP candidati sorgente dei finiti (G39-02 [U]).
func (r *Risultato) IngressoMotoreV1(prodotto uuid.UUID) *distintabozza.IngressoMotoreR4 {
	out := &distintabozza.IngressoMotoreR4{NonAttivi: r.V1.NonAttivi, AltriGruppi: map[uuid.UUID][]propostabom.Gruppo{}}
	for i := range r.EsitoThread.ProdottiValutati {
		pv := &r.EsitoThread.ProdottiValutati[i]
		if pv.ComponenteID == nil {
			continue
		}
		gg := propostabom.GruppiDelProdotto(*pv, r.EsitoThread.Ancoraggi.Strutture)
		if *pv.ComponenteID == prodotto {
			out.Prodotto, out.Gruppi = pv, gg
			continue
		}
		out.AltriGruppi[*pv.ComponenteID] = gg
	}
	pv := out.Prodotto
	out.Importa = func(gruppo string) (importazione.Registro, error) {
		if pv == nil {
			return importazione.Registro{}, &distintabozza.Errore{Codice: distintabozza.CodiceCorpoNonValido, Motivo: distintabozza.MotivoGruppoNonImportabile,
				Messaggio: "il prodotto non e' fra i prodotti che il motore valuta: nessun gruppo da importare"}
		}
		return propostabom.ImportaGruppo(propostabom.Ingresso{Esito: &r.Esito, Thread: r.EsitoThread, Regole: r.Regole, Prodotto: pv.Rif,
			Gruppo: gruppo, NonAttivi: r.V1.NonAttivi, Foto: &r.Foto})
	}
	out.StepCandidati = distintabozza.StepCandidatiDalMotore(r.EsitoThread, r.Thread)
	return out
}

// suggerimentiDiAdesso: per ogni bozza aperta formato 3 della lettura, i suggerimenti di R1 con il calcolo di adesso
// (commento di testa), al posto di quelli del solo registro della versione. Uno snapshot o un registro che non si
// rileggono valgono come ErroreBozze (S9 non risponde a metà); un errore del database ferma S9; un gruppo che non si
// importa lascia i soli suggerimenti del registro della versione.
func (s Servizio) suggerimentiDiAdesso(ctx context.Context, thread uuid.UUID, r *Risultato) error {
	var aperte []distintabozza.BozzaDellaRfq
	for _, b := range r.V1.LetturaBozze.Bozze {
		if !b.DaConvertire && b.Formato == distintabozza.FormatoSnapshot3 {
			aperte = append(aperte, b)
		}
	}
	if len(aperte) == 0 {
		return nil
	}
	if r.V1.Suggerimenti == nil {
		r.V1.Suggerimenti = map[uuid.UUID][]distintabozza.PreassociazioneProposta3{}
	}
	return nelloSnapshot(ctx, s.Pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		for _, b := range aperte {
			src, ok, err := distintabozza.SorgenteDellaBozza(ctx, q, thread, b.ProdottoID, b.BozzaID)
			if err != nil {
				return fmt.Errorf("%w: la sorgente della bozza non si legge: %w", ErrLettura, err)
			}
			var sorgente *propostabom.SorgenteBozza
			if ok {
				sorgente = &src
			}
			in := r.IngressoMotoreV1(b.ProdottoID)
			adesso, err := distintabozza.CalcoloAdessoDa(in, sorgente)
			if err != nil {
				// un gruppo che non si importa non ferma S9: restano i suggerimenti del registro della versione (come R1, dove
				// il motore che non calcola non è un errore)
				s.log().Warn("motore A, S9: il calcolo di adesso della bozza non si importa", "thread", thread, "prodotto", b.ProdottoID, "err", err)
				adesso = &distintabozza.CalcoloAdesso{Step: in.StepCandidati}
			}
			sg, err := distintabozza.SuggerimentiDellaBozzaApertaCon(ctx, q, thread, b.ProdottoID, adesso)
			if errors.Is(err, distintabozza.ErrSnapshotIncoerente) {
				r.V1.ErroreBozze = err
				return nil
			}
			if err != nil {
				return fmt.Errorf("%w: i suggerimenti dopo l'apertura della bozza non si leggono: %w", ErrLettura, err)
			}
			r.V1.Suggerimenti[b.ProdottoID] = sg
		}
		return nil
	})
}
