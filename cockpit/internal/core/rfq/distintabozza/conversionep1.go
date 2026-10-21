package distintabozza

// La conversione delle pre-assegnazioni legacy aperte (P1) alla presa in carico del prodotto (direttiva 39 §4, D-AR14 A
// [T-39]; EM I.9; NOMI (f) [T-39 dell'orchestratore]). Una P1 e' una proposta di documento aperta con la copia del
// componente (documento_proposta.componente_id) e nessuna AR attiva dell'allegato: un'assegnazione fatta dalle pagine
// di prima senza il servizio unico.
//
// Alla nascita della bozza formato 3 del prodotto (R2 crea, qualunque partenza, anche da S8 passo 3 e da-conferma-1; la
// conversione converti_da dal formato 2), nella stessa transazione e per ogni P1 il cui componente e' un nodo della
// versione 1 (non escluso):
//   - la PB {allegato_id, chiave_bozza del nodo, provenienza: legacy}, senza registro: «legacy da verificare» finche'
//     l'ingegnere non la toglie, non la rifa' a mano (diventa manuale) o non la riconosce (il gesto di R39-15, non in
//     questo blocco). Se la versione 1 ha gia' una PB del file (la conversione dal formato 2) resta quella: e' la scelta
//     dell'ingegnere;
//   - l'evento conversione_legacy del giornale delle AR con componente_prima e la copia tolta, dal servizio unico
//     (associazioni.ConvertiCopiaLegacy); nessuna AR (D-AR04, niente backfill).
// Non si convertono, e restano come sono (la risposta e l'evento bozza_aperta le dicono, con il motivo):
//   - lo STEP legato come sorgente del finito (legato_come_sorgente) e lo STEP di un finito (step_del_finito: la P1s di
//     EM I.9, candidata come sorgente nell'anteprima di S8);
//   - un'occorrenza che non si preassocia (natura_non_preassociabile) o non attiva (file_non_attivo);
//   - una P1 su un componente del perimetro del prodotto che non e' un nodo della versione 1 (componente_fuori_bozza):
//     per esempio la partenza vuota, che ha la sola radice. Una P1 di un componente fuori dal perimetro e' di un altro
//     prodotto e non si nomina.
//
// La ripresa dopo l'abbandono (rilievo importante della revisione del 20/10) [T-39]. La conversione toglie la copia e
// mette l'assegnazione solo nella PB legacy della bozza nuova: se quella bozza si abbandona senza R5, l'assegnazione
// vivrebbe solo nel giornale (componente_prima) e la bozza abbandonata la conservazione la cancella dopo 90 giorni. Per
// non perdere in silenzio il lavoro legacy dell'ingegnere, alla nascita di ogni bozza successiva del prodotto (stessi
// punti: crea, converti_da) il server rilegge dal giornale delle AR le conversioni non consumate
// (ListConversioniP1DaRiprendere: l'ultimo evento dell'allegato e' la conversione, nessuna AR attiva ne' copia, nessuna
// conferma formato 3 del prodotto dopo la conversione) e, con le stesse regole della conversione (nodo della versione
// 1 non escluso, occorrenza attiva e preassociabile, ne' STEP sorgente ne' STEP di un finito), rimette la PB legacy
// «da verificare». Niente si scrive nel giornale delle AR (la copia e' gia' tolta, la conversione c'e' gia'): la
// ripresa sta in conversioni_legacy[] con ripresa: true e l'evento di conversione d'origine, nella risposta e nel dopo
// di bozza_aperta. Una conferma formato 3 del prodotto dopo la conversione l'ha decisa (AR, o PB tolta prima di R5):
// non si riprende. L'abbandono non scrive niente (nessuna copia rimessa: la copia senza AR e' proprio la P1 che il
// formato 3 non vuole piu').

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/platform/db"
)

// I motivi di una pre-assegnazione non convertita.
const (
	P1LegatoComeSorgente = "legato_come_sorgente"
	P1StepDelFinito      = "step_del_finito"
	P1NaturaNo           = "natura_non_preassociabile"
	P1FileNonAttivo      = "file_non_attivo"
	P1FuoriBozza         = "componente_fuori_bozza"
)

// ConversioneLegacy3 e' una pre-assegnazione convertita: la proposta, l'allegato, il componente della copia tolta e la
// chiave del nodo della PB legacy (PBEsistente: la versione 1 aveva gia' una PB del file, ed e' rimasta quella).
type ConversioneLegacy3 struct {
	AllegatoID      uuid.UUID `json:"allegato_id"`
	PropostaID      uuid.UUID `json:"proposta_id"`
	ComponentePrima uuid.UUID `json:"componente_prima"`
	ChiaveBozza     string    `json:"chiave_bozza"`
	PBEsistente     bool      `json:"pb_esistente,omitempty"`
	// Ripresa: la conversione e' di una bozza abbandonata, ripresa dal giornale delle AR (commento di testa); Evento e'
	// l'evento conversione_legacy d'origine
	Ripresa bool  `json:"ripresa,omitempty"`
	Evento  int64 `json:"evento_conversione,omitempty"`
}

// PreassegnazioneNonConvertita3 e' una pre-assegnazione del perimetro rimasta com'era, con il motivo.
type PreassegnazioneNonConvertita3 struct {
	AllegatoID   uuid.UUID `json:"allegato_id"`
	ComponenteID uuid.UUID `json:"componente_id"`
	Motivo       string    `json:"motivo"`
}

// convertiPreassegnazioni converte le P1 dei nodi della versione 1 (commento di testa) e mette le PB legacy nello
// snapshot. perimetro: i componenti del perimetro del prodotto (per dire quali P1 restano fuori). chiave: la chiave
// dell'atto che prende in carico il prodotto (la chiave del salvataggio della versione 1).
func convertiPreassegnazioni(ctx context.Context, q *db.Queries, thread, utente, chiave uuid.UUID, s *Snapshot3,
	perimetro map[uuid.UUID]bool) ([]ConversioneLegacy3, []PreassegnazioneNonConvertita3, error) {
	conv, non := []ConversioneLegacy3{}, []PreassegnazioneNonConvertita3{}
	copie, err := q.ListCopieSenzaARDellaRfq(ctx, thread)
	if err != nil {
		return nil, nil, err
	}
	riprese, err := q.ListConversioniP1DaRiprendere(ctx, db.ListConversioniP1DaRiprendereParams{ThreadID: thread, ProdottoID: s.ProdottoID})
	if err != nil {
		return nil, nil, err
	}
	if len(copie) == 0 && len(riprese) == 0 {
		return conv, non, nil
	}
	nodoDi := map[uuid.UUID]string{}
	for _, n := range s.Nodi {
		if n.ComponenteID != nil && n.Op != OpEsclude {
			nodoDi[*n.ComponenteID] = n.ChiaveBozza
		}
	}
	pb := map[uuid.UUID]bool{}
	for _, p := range s.DocumentiPreassociati {
		pb[p.AllegatoID] = true
	}
	var nuove []Preassociazione3
	atto := associazioni.Atto{Thread: thread, Autore: utente, Chiave: chiave}
	for _, c := range copie {
		chiaveNodo, nelNodo := nodoDi[c.ComponenteID]
		if !nelNodo && !perimetro[c.ComponenteID] {
			continue // una P1 di un altro prodotto
		}
		motivo := ""
		switch {
		case c.LegatoComeSorgente:
			motivo = P1LegatoComeSorgente
		case c.TipoComponente == "finito" && (c.Estensione == "stp" || c.Estensione == "step"):
			motivo = P1StepDelFinito
		case !c.Preassociabile:
			motivo = P1NaturaNo
		case !c.Attivo:
			motivo = P1FileNonAttivo
		case !nelNodo:
			motivo = P1FuoriBozza
		}
		if motivo != "" {
			non = append(non, PreassegnazioneNonConvertita3{AllegatoID: c.AllegatoID, ComponenteID: c.ComponenteID, Motivo: motivo})
			continue
		}
		es, err := associazioni.ConvertiCopiaLegacy(ctx, q, atto, c.AllegatoID, c.ComponenteID)
		if err != nil {
			return nil, nil, err
		}
		if es.Esito != associazioni.EsitoCopiaTolta {
			continue // la copia non c'e' piu' (letta nella stessa transazione sotto il lucchetto: non succede)
		}
		x := ConversioneLegacy3{AllegatoID: c.AllegatoID, PropostaID: c.PropostaID, ComponentePrima: c.ComponenteID, ChiaveBozza: chiaveNodo,
			PBEsistente: pb[c.AllegatoID]}
		if !x.PBEsistente {
			nuove = append(nuove, Preassociazione3{AllegatoID: c.AllegatoID, ChiaveBozza: chiaveNodo, Provenienza: ProvenienzaLegacy})
			pb[c.AllegatoID] = true
		}
		conv = append(conv, x)
	}
	// la ripresa delle conversioni di una bozza abbandonata (commento di testa)
	if len(riprese) > 0 {
		ids := make([]uuid.UUID, 0, len(riprese))
		for _, r := range riprese {
			ids = append(ids, r.AllegatoID)
		}
		occ, err := q.ListAllegatiPreassociabili(ctx, db.ListAllegatiPreassociabiliParams{ThreadID: thread, Allegati: ids})
		if err != nil {
			return nil, nil, err
		}
		perAllegato := map[uuid.UUID]db.ListAllegatiPreassociabiliRow{}
		for _, o := range occ {
			perAllegato[o.AllegatoID] = o
		}
		tipoDi := map[uuid.UUID]string{}
		for _, n := range s.Nodi {
			if n.ComponenteID != nil {
				tipoDi[*n.ComponenteID] = n.Tipo
			}
		}
		for _, r := range riprese {
			chiaveNodo, nelNodo := nodoDi[r.ComponenteID]
			o, ok := perAllegato[r.AllegatoID]
			if !nelNodo || !ok || pb[r.AllegatoID] || !o.DellaRfq || !o.Attivo || !o.Preassociabile || o.LegatoComeSorgente ||
				((tipoDi[r.ComponenteID] == "finito" || r.ComponenteID == s.ProdottoID) && eStep(o.Estensione)) {
				continue
			}
			x := ConversioneLegacy3{AllegatoID: r.AllegatoID, ComponentePrima: r.ComponenteID, ChiaveBozza: chiaveNodo, Ripresa: true, Evento: r.EventoID}
			if pp, err := q.ListProposteDiAllegatoPerCopia(ctx, r.AllegatoID); err != nil {
				return nil, nil, err
			} else if len(pp) > 0 {
				x.PropostaID = pp[0].PropostaID
			}
			nuove = append(nuove, Preassociazione3{AllegatoID: r.AllegatoID, ChiaveBozza: chiaveNodo, Provenienza: ProvenienzaLegacy})
			pb[r.AllegatoID] = true
			conv = append(conv, x)
		}
	}
	aggiungiPB(s, nuove)
	sort.Slice(non, func(i, j int) bool { return non[i].AllegatoID.String() < non[j].AllegatoID.String() })
	return conv, non, nil
}

// perimetroDi: i componenti del perimetro del prodotto nella working (per convertiPreassegnazioni).
func perimetroDi(in IngressoPartenza) (map[uuid.UUID]bool, error) {
	out := map[uuid.UUID]bool{in.Prodotto: true}
	per, err := in.Stato.Perimetro(in.Prodotto)
	if err != nil {
		return nil, err
	}
	for _, c := range per.Componenti {
		out[c.ID] = true
	}
	return out, nil
}
