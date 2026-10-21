package distintabozza

// La lettura delle bozze di tutti i prodotti della RFQ per S9 (direttiva 35 §8 C-4 [U]; piano delle dipendenze della
// direttiva 35, §7.1 e §7.2 C-4, parte persistenza [T]; U2 §2.3 e §12 punto 6; EM §10.9 «S9»). Per ogni bozza aperta:
//   - le preassociazioni (PB) {allegato_id, chiave_bozza} con il nodo a cui stanno: codice e rev dell'identita' decisa,
//     il componente di un nodo riusato e la casella del nodo dello STEP, se c'e' (il riferimento stabile della riga di
//     S9: con la bozza la chiave_bozza e, per un nodo dello STEP, la casella; mai il codice);
//   - gli svincoli pendenti (le decisioni scollega_documento {allegato_id, componente_id} dello snapshot: si applicano
//     alla R5 della bozza), con il segno della AR ancora attiva o gia' chiusa (svincolo_gia_chiuso, D-ASS-03 [U]);
//   - il vassoio «Da ricollocare», con la stessa regola di R1 (VassoioDa sul giornale fino alla versione corrente,
//     EM §3.7), e il nodo d'uscita se la bozza lo ha ancora;
//   - la versione della bozza (e la sua impronta).
// E le associazioni registrate (AR) attive della RFQ, lette dal servizio unico (associazioni.AttiveDellaRfq).
//
// Mai documento_proposta.componente_id come fonte: le PB vengono dallo snapshot, le AR da file_rfq_associazione (righe
// attive), e le query di questa lettura (v1_lettura_bozze.sql) non leggono documento_proposta. Il ripiego del Motore A
// per le copie senza AR (pre-assegnazioni legacy, piano §7.2 C-4) e' del motore, non di questa lettura.
//
// Chi chiama apre la transazione (REPEATABLE READ READ ONLY, la stessa della fotografia del motore) e la chiude: qui
// solo SELECT, nessun lucchetto, nessuna scrittura (D-18). Le letture del giornale si fermano alla versione corrente
// di ogni bozza, come R1.

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/platform/db"
)

// BozzeDellaRfq e' la lettura delle bozze di tutti i prodotti della RFQ: le bozze aperte, in ordine di prodotto, e le
// AR attive della RFQ, in ordine di allegato e componente (le letture della working usano solo le righe attive).
type BozzeDellaRfq struct {
	Bozze        []BozzaDellaRfq
	Associazioni []associazioni.Associazione
}

// BozzaDellaRfq e' la bozza aperta di un prodotto, alla versione corrente. Per una bozza formato 3 gli elenchi ci sono
// sempre, anche vuoti. Una bozza di un formato precedente (2) si dice con DaConvertire e senza elenchi: le sue PB
// citano proposte di documento e si traducono solo con la conversione (converti_da, piano §6.3); S9 non le legge.
type BozzaDellaRfq struct {
	ProdottoID      uuid.UUID
	BozzaID         uuid.UUID
	Formato         int
	Versione        int
	Impronta        string
	Sorgente        *Sorgente3
	DaConvertire    bool
	Preassociazioni []PreassociazioneDellaBozza
	Svincoli        []SvincoloPendente
	Vassoio         []VoceVassoioDellaBozza
	// Nodi: tutti i nodi della versione corrente, nell'ordine dello snapshot (richiesta 2 della corsia motore, direttiva 39: la
	// chiave_bozza dei suggerimenti di S9 anche per i nodi senza PB ne' voci del vassoio); solo formato 3.
	Nodi []NodoDellaBozza
}

// NodoDellaBozza e' il nodo della bozza a cui sta una PB (o da cui e' uscita una voce del vassoio): la chiave, il
// componente di un nodo riusato, op, tipo e origine, l'identita' decisa (emittente, codice e rev, il modo) e le caselle.
// Casella e' la casella del nodo dello STEP sorgente della bozza (stesso allegato e gruppo), altrimenti la prima delle
// caselle in ordine canonico; nil per un nodo senza caselle (un nodo manuale).
type NodoDellaBozza struct {
	ChiaveBozza  string
	ComponenteID *uuid.UUID
	Op           string
	Tipo         string
	Origine      string
	Emittente    string
	Codice       string
	Rev          string
	Modo         string
	Casella      *Casella3
	Caselle      []Casella3
}

// PreassociazioneDellaBozza e' una PB {allegato_id, chiave_bozza} con il suo nodo, la provenienza e il registro
// d'origine (direttiva 39 §4, D-PB03 A [U]: vuoti per una versione salvata prima che il server li scrivesse).
type PreassociazioneDellaBozza struct {
	AllegatoID   uuid.UUID
	ChiaveBozza  string
	Nodo         NodoDellaBozza
	Provenienza  string
	Importazione string
}

// SvincoloPendente e' uno svincolo salvato nella bozza (scollega_documento {allegato_id, componente_id}), in attesa della
// R5. AssociazioneAttiva dice se la AR di quella coppia e' ancora attiva nella stessa lettura: falso quando la corsia
// file l'ha gia' chiusa dopo lo svincolo (D-ASS-03 [U]; alla R5 l'esito sara' svincolo_gia_chiuso).
type SvincoloPendente struct {
	DecisioneID        string
	AllegatoID         uuid.UUID
	ComponenteID       uuid.UUID
	Motivo             string
	DaDecisione        string
	AssociazioneAttiva bool
}

// VoceVassoioDellaBozza e' un file nel vassoio «Da ricollocare» (VoceVassoio) con il nodo d'uscita, se la bozza lo ha
// ancora (nil se il nodo e' stato tolto).
type VoceVassoioDellaBozza struct {
	AllegatoID  uuid.UUID
	Motivo      string
	ChiaveBozza string
	Nodo        *NodoDellaBozza
}

// LeggiBozzeDellaRfq legge le bozze aperte di tutti i prodotti della RFQ e le AR attive (commento di testa), sulla
// transazione di chi chiama. Una RFQ senza bozze da' elenchi vuoti; il perimetro della RFQ (che esista, che il
// chiamante possa leggerla) lo controlla chi chiama. Uno snapshot formato 3 illeggibile o con un'impronta diversa e'
// ErrSnapshotIncoerente, come in R1.
func LeggiBozzeDellaRfq(ctx context.Context, q *db.Queries, thread uuid.UUID) (BozzeDellaRfq, error) {
	out := BozzeDellaRfq{Bozze: []BozzaDellaRfq{}}
	bb, err := q.ListBozzeAperteDellaRfq(ctx, thread)
	if err != nil {
		return out, fmt.Errorf("bozze aperte della RFQ: %w", err)
	}
	ar, err := associazioni.AttiveDellaRfq(ctx, q, thread)
	if err != nil {
		return out, fmt.Errorf("associazioni attive della RFQ: %w", err)
	}
	out.Associazioni = ar
	attiva := make(map[[2]uuid.UUID]bool, len(ar))
	for _, a := range ar {
		attiva[[2]uuid.UUID{a.Allegato, a.Componente}] = true
	}
	var giornale map[uuid.UUID][]EventoGiornale
	for _, b := range bb {
		bz := BozzaDellaRfq{ProdottoID: b.ProdottoID, BozzaID: b.BozzaID, Formato: int(b.Formato), Versione: int(b.Versione), Impronta: b.Impronta}
		if bz.Formato != FormatoSnapshot3 {
			bz.DaConvertire = true
			out.Bozze = append(out.Bozze, bz)
			continue
		}
		letto, err := RileggiVersione(bz.Formato, b.Snapshot, b.Impronta)
		if err != nil {
			return out, fmt.Errorf("la bozza %s del prodotto %s: %w", b.BozzaID, b.ProdottoID, err)
		}
		if letto.Formato3 == nil {
			return out, fmt.Errorf("%w: la bozza %s non e' del formato 3", ErrSnapshotIncoerente, b.BozzaID)
		}
		s := *letto.Formato3
		bz.Impronta, bz.Sorgente = letto.Impronta, s.Sorgente
		bz.Nodi = make([]NodoDellaBozza, 0, len(s.Nodi))
		nodi := make(map[string]NodoDellaBozza, len(s.Nodi))
		for _, n := range s.Nodi {
			nodi[n.ChiaveBozza] = nodoDellaBozza(n, s.Sorgente)
			bz.Nodi = append(bz.Nodi, nodi[n.ChiaveBozza])
		}
		bz.Preassociazioni = make([]PreassociazioneDellaBozza, 0, len(s.DocumentiPreassociati))
		for _, p := range s.DocumentiPreassociati {
			n, ok := nodi[p.ChiaveBozza]
			if !ok { // la forma lo esclude (la verifica delle PB in formato3.go): un guasto dei dati salvati
				return out, fmt.Errorf("%w: la PB dell'allegato %s cita il nodo %q che la bozza %s non ha", ErrSnapshotIncoerente,
					p.AllegatoID, p.ChiaveBozza, b.BozzaID)
			}
			bz.Preassociazioni = append(bz.Preassociazioni, PreassociazioneDellaBozza{AllegatoID: p.AllegatoID, ChiaveBozza: p.ChiaveBozza, Nodo: n,
				Provenienza: p.Provenienza, Importazione: p.Importazione})
		}
		bz.Svincoli = []SvincoloPendente{}
		for _, d := range s.Decisioni {
			if d.Tipo != DecisioneScollegaDocumento || d.AllegatoID == nil || d.ComponenteID == nil {
				continue
			}
			bz.Svincoli = append(bz.Svincoli, SvincoloPendente{DecisioneID: d.ID, AllegatoID: *d.AllegatoID, ComponenteID: *d.ComponenteID,
				Motivo: d.Motivo, DaDecisione: d.DaDecisione, AssociazioneAttiva: attiva[[2]uuid.UUID{*d.AllegatoID, *d.ComponenteID}]})
		}
		if giornale == nil {
			if giornale, err = giornaliDelVassoio(ctx, q, thread); err != nil {
				return out, err
			}
		}
		bz.Vassoio = []VoceVassoioDellaBozza{}
		for _, v := range VassoioDa(giornale[b.BozzaID]) {
			id, err := uuid.Parse(v.AllegatoID)
			if err != nil {
				return out, fmt.Errorf("%w: il giornale della bozza %s cita l'allegato %q", ErrSnapshotIncoerente, b.BozzaID, v.AllegatoID)
			}
			voce := VoceVassoioDellaBozza{AllegatoID: id, Motivo: v.Motivo, ChiaveBozza: v.ChiaveBozza}
			if n, ok := nodi[v.ChiaveBozza]; ok {
				voce.Nodo = &n
			}
			bz.Vassoio = append(bz.Vassoio, voce)
		}
		out.Bozze = append(out.Bozze, bz)
	}
	return out, nil
}

// giornaliDelVassoio: gli eventi che la regola del vassoio guarda, per bozza aperta della RFQ, fino alla versione
// corrente di ognuna (una query per tutta la RFQ).
func giornaliDelVassoio(ctx context.Context, q *db.Queries, thread uuid.UUID) (map[uuid.UUID][]EventoGiornale, error) {
	ee, err := q.ListEventiFileBozzeAperteDellaRfq(ctx, db.ListEventiFileBozzeAperteDellaRfqParams{ThreadID: thread, Azioni: AzioniDelVassoio})
	if err != nil {
		return nil, fmt.Errorf("giornale delle bozze aperte della RFQ: %w", err)
	}
	out := map[uuid.UUID][]EventoGiornale{}
	for _, e := range ee {
		l := eventoLetto(e)
		out[e.BozzaID] = append(out[e.BozzaID], EventoGiornale{N: l.N, Azione: l.Azione, ChiaveBozza: l.ChiaveBozza, Prima: l.Prima, Dopo: l.Dopo})
	}
	return out, nil
}

// nodoDellaBozza: il nodo nella forma della lettura, con la casella dello STEP sorgente se c'e'.
func nodoDellaBozza(n Nodo3, sorgente *Sorgente3) NodoDellaBozza {
	out := NodoDellaBozza{ChiaveBozza: n.ChiaveBozza, ComponenteID: n.ComponenteID, Op: n.Op, Tipo: n.Tipo, Origine: n.Origine,
		Emittente: n.Identita.Emittente, Codice: n.Identita.Codice, Rev: n.Identita.Rev, Modo: n.Identita.Modo,
		Caselle: append([]Casella3{}, n.Caselle...)}
	for i := range out.Caselle {
		c := out.Caselle[i]
		if sorgente != nil && sorgente.AllegatoID != nil && c.AllegatoID == *sorgente.AllegatoID && c.Gruppo == sorgente.Gruppo {
			out.Casella = &c
			break
		}
	}
	if out.Casella == nil && len(out.Caselle) > 0 {
		c := out.Caselle[0]
		out.Casella = &c
	}
	return out
}
