package distintabozza

// Il calcolo del Motore A di adesso per una bozza aperta (G39-01 e G39-02 [U], 20/10 sera; D-PB02 A, D-PB03 A [U]).
//
// G39-01 [U]: un file arrivato dopo l'apertura della bozza, prima di R5, entra in preassociazioni_proposte[] solo se il
// Motore A gli trova una destinazione univoca (D-PB01 A); accettato senza modifiche diventa una PB «motore» con il
// riferimento stabile al calcolo che l'ha proposto (D-PB03 A), spostato dall'ingegnere su un altro componente una PB
// «corretta». La provenienza la decide sempre il server. Un file arrivato dopo R5 va nella revisione successiva: la bozza
// confermata e' chiusa, e la bozza nuova del prodotto (da-conferma-1) lo vede fra i suoi suggerimenti.
//
// Come [T-39 corsia persistenza]:
//   - il calcolo di adesso e' il registro del gruppo corrente della sorgente della bozza, importato dal calcolo del motore
//     di adesso (la stessa strada di riallinea_proposta: GruppoDelRiallineamento, poi Importa), senza salvarlo; lo prepara
//     l'adattatore (CalcoloAdessoDa) con il semaforo dei calcoli, prima di ogni transazione, come le forme che importano;
//   - R1 con ?suggerimenti_motore=1 (OpzioniR1.Adesso) aggiunge ai suggerimenti dal registro della versione quelli del
//     calcolo di adesso per i file che il registro della versione non propone (arrivati dopo l'apertura, o proposti solo
//     adesso), con la stessa regola delle PB iniziali (ClassificaProposte: univoca, sorgente S8 valida, niente STEP
//     candidato del finito); alla nascita sarebbero PB: motivo dopo_apertura, con importazione = l'impronta del registro di
//     adesso. Senza il parametro R1 resta com'era (D-18: R1 non ricalcola il motore);
//   - R2 (salvataggio) riconosce l'impronta del registro di adesso (Salvataggio3.Adesso, preparato dall'adattatore quando
//     una PB nuova o spostata dichiara un registro che il database non ha): la PB e' «motore» se il registro la propone sul
//     suo nodo, «corretta» se la propone, univoca, su un altro nodo; il registro si scrive allora in distinta_importazione
//     nella stessa transazione (registroDellaVersione: immutabile, citato per impronta), cosi' il riferimento resta stabile.
//     Se fra la lettura e il salvataggio il calcolo e' cambiato (un altro file arrivato), vale il registro di adesso purche'
//     proponga lo stesso file in modo univoco: il riferimento e' quello del calcolo che il server ha davanti.

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// CalcoloAdesso: il calcolo del Motore A di adesso per una bozza, come lo prepara l'adattatore: il registro del gruppo
// corrente della sorgente (nil = la bozza non ha una sorgente STEP, o il motore non propone piu' quel gruppo) e gli STEP
// candidati sorgente dei finiti (G39-02).
type CalcoloAdesso struct {
	Registro *importazione.Registro
	Step     *StepCandidati
}

func (c *CalcoloAdesso) step() *StepCandidati {
	if c == nil {
		return nil
	}
	return c.Step
}

func (c *CalcoloAdesso) registro() *importazione.Registro {
	if c == nil {
		return nil
	}
	return c.Registro
}

// CalcoloAdessoDa: il calcolo di adesso dall'ingresso del motore (ingressoMotoreV1 dell'adattatore) e dalla sorgente
// della bozza (nil = nessuna sorgente STEP: niente registro). Un gruppo che il motore non propone piu', o che sta su
// un'altra copia dello stesso file, non e' un errore: niente registro (la bozza resta valida; si segue con
// riallinea_proposta o cambia_sorgente). Gli altri errori dell'importazione tornano a chi chiama.
func CalcoloAdessoDa(in *IngressoMotoreR4, src *propostabom.SorgenteBozza) (*CalcoloAdesso, error) {
	if in == nil {
		return nil, nil
	}
	out := &CalcoloAdesso{Step: in.StepCandidati}
	if src == nil || in.Importa == nil {
		return out, nil
	}
	chiave, err := GruppoDelRiallineamento(in.Gruppi, in.NonAttivi, *src)
	if err != nil {
		var e *Errore
		if errors.As(err, &e) {
			return out, nil
		}
		return nil, err
	}
	reg, err := in.Importa(chiave)
	if err != nil {
		return nil, err
	}
	out.Registro = &reg
	return out, nil
}

// MotoreDelSalvataggio dice, in sola lettura e prima della transazione di R2, che cosa del calcolo di adesso serve al
// salvataggio (l'adattatore lo calcola solo allora, con il semaforo): registro, una PB nuova o spostata dichiara un
// registro che il database non ha per il prodotto (un suggerimento del calcolo di adesso accettato: G39-01); step, una PB
// nuova o spostata e' su uno STEP (il riconoscimento dello STEP candidato del finito con il motore: G39-02). Un corpo che
// non si legge, una bozza che non c'e' o non e' aperta: tutti e due falsi (il servizio rispondera' con il suo rifiuto).
func MotoreDelSalvataggio(ctx context.Context, q *db.Queries, s Salvataggio3) (registro, step bool, err error) {
	nuovo, err := LeggiSnapshot3(s.Snapshot)
	if err != nil || len(nuovo.DocumentiPreassociati) == 0 {
		return false, false, nil
	}
	b, err := q.GetDistintaBozza(ctx, db.GetDistintaBozzaParams{BozzaID: s.BozzaID, ThreadID: s.ThreadID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if b.Stato != StatoAperta || int(b.Formato) != FormatoSnapshot3 {
		return false, false, nil
	}
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: b.BozzaID, ThreadID: s.ThreadID, Versione: b.Versione})
	if err != nil {
		return false, false, err
	}
	prima, _, err := rileggiSnapshot3(v.Snapshot, v.Impronta)
	if err != nil {
		return false, false, nil
	}
	pp := map[uuid.UUID]Preassociazione3{}
	for _, p := range prima.DocumentiPreassociati {
		pp[p.AllegatoID] = p
	}
	var cambiate []uuid.UUID
	impronte := map[string]bool{}
	for _, p := range nuovo.DocumentiPreassociati {
		if x, ok := pp[p.AllegatoID]; ok && x.ChiaveBozza == p.ChiaveBozza {
			continue
		}
		cambiate = append(cambiate, p.AllegatoID)
		if imp := strings.TrimSpace(p.Importazione); reSha256.MatchString(imp) {
			impronte[imp] = true
		}
	}
	if len(cambiate) == 0 {
		return false, false, nil
	}
	if len(impronte) > 0 {
		ii := make([]string, 0, len(impronte))
		for k := range impronte {
			ii = append(ii, k)
		}
		rr, err := q.ListImportazioniPerImpronte(ctx, db.ListImportazioniPerImpronteParams{ThreadID: s.ThreadID, ProdottoID: s.ProdottoID, Impronte: ii})
		if err != nil {
			return false, false, err
		}
		registro = len(rr) < len(ii)
	}
	rr, err := q.ListAllegatiPreassociabili(ctx, db.ListAllegatiPreassociabiliParams{ThreadID: s.ThreadID, Allegati: unici3(cambiate)})
	if err != nil {
		return false, false, err
	}
	for _, r := range rr {
		step = step || (r.DellaRfq && eStep(r.Estensione))
	}
	return registro, step, nil
}

// SuggerimentiDellaBozzaApertaCon (G39-01 [U], per S9 della corsia motore): i suggerimenti della bozza aperta formato 3
// del prodotto come R1 con ?suggerimenti_motore=1: quelli del registro della versione (con gli STEP candidati del calcolo)
// piu' quelli del calcolo di adesso per i file che il registro della versione non propone. Vuoti senza bozza aperta, per
// una bozza formato 2, senza la sorgente S8 valida. adesso nil = i soli suggerimenti del registro della versione (come
// SuggerimentiDellaBozzaAperta). Il calcolo lo compone chi chiama: CalcoloAdessoDa(&IngressoMotoreR4{Gruppi, NonAttivi,
// Importa, StepCandidati: StepCandidatiDalMotore(esito del thread)}, sorgente della bozza). Solo SELECT.
func SuggerimentiDellaBozzaApertaCon(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, adesso *CalcoloAdesso) ([]PreassociazioneProposta3, error) {
	out := []PreassociazioneProposta3{}
	b, err := q.GetDistintaBozzaAperta(ctx, db.GetDistintaBozzaApertaParams{ThreadID: thread, ProdottoID: prodotto})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: b.BozzaID, ThreadID: thread, Versione: b.Versione})
	if err != nil {
		return nil, err
	}
	letto, err := RileggiVersione(int(b.Formato), v.Snapshot, v.Impronta)
	if err != nil {
		return nil, err
	}
	if letto.Formato3 == nil {
		return out, nil
	}
	s := *letto.Formato3
	var regVersione *importazione.Registro
	if s.Sorgente != nil && s.Sorgente.Importazione != "" {
		rr, err := q.ListImportazioniPerImpronte(ctx, db.ListImportazioniPerImpronteParams{ThreadID: thread, ProdottoID: prodotto,
			Impronte: []string{s.Sorgente.Importazione}})
		if err != nil {
			return nil, err
		}
		for _, r := range rr {
			if strings.TrimSpace(r.Impronta) == s.Sorgente.Importazione {
				reg, _, err := RileggiImportazione(rigaRegistro(r))
				if err != nil {
					return nil, err
				}
				regVersione = &reg
			}
		}
		if regVersione == nil {
			return nil, ErrSnapshotIncoerente
		}
		if out, err = suggerimentiDopoAperturaCon(ctx, q, thread, prodotto, s, regVersione, b.BozzaID, int(b.Versione), adesso.step()); err != nil {
			return nil, err
		}
	}
	if adesso == nil {
		return out, nil
	}
	dalCalcolo, err := suggerimentiDelCalcolo(ctx, q, thread, prodotto, s, regVersione, adesso, b.BozzaID, int(b.Versione))
	if err != nil {
		return nil, err
	}
	return unisciSuggerimenti(out, dalCalcolo), nil
}

// destinazioneUnica: il rif dell'unica voce del registro che propone l'allegato ("" se nessuna o piu' d'una: D-PB01 A).
func destinazioneUnica(c importazione.Contenuto, a uuid.UUID) string {
	if rif := ProposteDelRegistro(c)[a]; len(rif) == 1 {
		return rif[0]
	}
	return ""
}
