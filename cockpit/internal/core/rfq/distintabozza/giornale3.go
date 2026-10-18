package distintabozza

// VerificaGiornale3: il giornale di una bozza formato 3 rifatto versione per versione (PM23 T-10, la parte che non
// vuole l'adattatore del Motore A; EM §9.2):
//   - una versione costruita dalla pagina e' il risultato dei suoi eventi applicati alla versione prima: Riconcilia3
//     con gli eventi salvati come azioni deve dare lo snapshot salvato e gli stessi eventi, byte per byte nella forma
//     canonica;
//   - la versione 1 di una bozza convertita (bozza_aperta con partenza formato_2, algoritmo conversione-1) si ricostruisce
//     dalla versione citata della bozza formato 2, se c'e' ancora (una bozza abbandonata si cancella dopo 90 giorni), con
//     la sua impronta ricontrollata;
//   - le versioni costruite con l'adattatore (importazione-1, riancoraggio-1, cambio-fonte-1, da-conferma-1: crea,
//     riallinea_proposta, cambia_sorgente e la partenza dalla conferma) arrivano con l'ondata C, e qui restano fra le
//     non verificate con il motivo.
// Solo letture: chi chiama sceglie la transazione.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

// ErrGiornaleIncoerente: il giornale non rifa' le versioni salvate (un guasto dei dati, non un rifiuto).
var ErrGiornaleIncoerente = errors.New("il giornale della bozza non rifa' le versioni salvate")

// VersioneNonVerificata: una versione che VerificaGiornale3 non sa rifare, con il motivo.
type VersioneNonVerificata struct {
	Versione int    `json:"versione"`
	Motivo   string `json:"motivo"`
}

// EsitoGiornale3 e' l'esito di VerificaGiornale3.
type EsitoGiornale3 struct {
	Verificate    []int                   `json:"verificate"`
	NonVerificate []VersioneNonVerificata `json:"non_verificate"`
}

// VerificaGiornale3 rifa' il giornale della bozza formato 3 (commento di testa).
func VerificaGiornale3(ctx context.Context, q *db.Queries, thread, bozza uuid.UUID) (EsitoGiornale3, error) {
	out := EsitoGiornale3{Verificate: []int{}, NonVerificate: []VersioneNonVerificata{}}
	b, err := q.GetDistintaBozza(ctx, db.GetDistintaBozzaParams{BozzaID: bozza, ThreadID: thread})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, fuori("bozza_id", "la bozza %s non e' di questa RFQ", bozza)
	}
	if err != nil {
		return out, err
	}
	if int(b.Formato) != FormatoSnapshot3 {
		return out, corpo("bozza_id", "la bozza e' nel formato %d: il giornale del formato 2 lo rifa' VerificaGiornale", b.Formato)
	}
	vv, err := q.ListDistintaBozzaVersioni(ctx, db.ListDistintaBozzaVersioniParams{BozzaID: bozza, ThreadID: thread})
	if err != nil {
		return out, err
	}
	ee, err := q.ListDistintaBozzaEventi(ctx, db.ListDistintaBozzaEventiParams{BozzaID: bozza, ThreadID: thread, VersioneMax: math.MaxInt32,
		DopoN: 0, Limite: math.MaxInt32})
	if err != nil {
		return out, err
	}
	eventi := make([]EventoLetto, 0, len(ee))
	perNumero := map[int]EventoLetto{}
	perVersione := map[int][]EventoLetto{}
	for i, r := range ee {
		e := eventoLetto(r)
		if e.N != i+1 {
			return out, fmt.Errorf("%w: alla posizione %d l'evento %d", ErrGiornaleIncoerente, i, e.N)
		}
		if e.Vocabolario != VocabolarioFormato3 {
			return out, fmt.Errorf("%w: l'evento %d e' del vocabolario %d", ErrGiornaleIncoerente, e.N, e.Vocabolario)
		}
		eventi = append(eventi, e)
		perNumero[e.N] = e
		perVersione[e.Versione] = append(perVersione[e.Versione], e)
	}
	precedente := func(n int) (EventoLetto, bool, error) {
		e, ok := perNumero[n]
		return e, ok, nil
	}
	var prima *Snapshot3
	n0 := 0
	for i, v := range vv {
		versione := int(v.Versione)
		if versione != i+1 {
			return out, fmt.Errorf("%w: alla posizione %d la versione %d", ErrGiornaleIncoerente, i, versione)
		}
		snap, _, err := rileggiSnapshot3(v.Snapshot, v.Impronta)
		if err != nil {
			return out, err
		}
		dellaVersione := perVersione[versione]
		if len(dellaVersione) == 0 {
			return out, fmt.Errorf("%w: la versione %d non ha eventi", ErrGiornaleIncoerente, versione)
		}
		testa := dellaVersione[0]
		switch {
		case versione == 1:
			motivo, err := verificaPrimaVersione3(ctx, q, thread, testa, snap)
			if err != nil {
				return out, err
			}
			if motivo != "" {
				out.NonVerificate = append(out.NonVerificate, VersioneNonVerificata{Versione: 1, Motivo: motivo})
			} else {
				out.Verificate = append(out.Verificate, 1)
			}
		case azioniDelServer2[testa.Azione]:
			out.NonVerificate = append(out.NonVerificate, VersioneNonVerificata{Versione: versione,
				Motivo: fmt.Sprintf("versione costruita dal server (%s): la rifa' l'adattatore del Motore A, con l'ondata C", testa.Azione)})
		default:
			if prima == nil {
				return out, fmt.Errorf("%w: la versione %d non ha una versione prima", ErrGiornaleIncoerente, versione)
			}
			limite := 0
			for _, e := range eventi[:n0] {
				if azioniDelServer2[e.Azione] && e.N > limite {
					limite = e.N
				}
			}
			azioni := make([]Azione, 0, len(dellaVersione))
			for _, e := range dellaVersione {
				azioni = append(azioni, Azione{Azione: e.Azione, ChiaveBozza: e.ChiaveBozza, Prima: e.Prima, Dopo: e.Dopo, Origine: e.Origine, Motivo: e.Motivo})
			}
			r, err := Riconcilia3(*prima, snap, azioni, Contesto3{N0: n0, Precedente: precedente, LimiteServer: limite})
			if err != nil {
				return out, fmt.Errorf("%w: la versione %d: %v", ErrGiornaleIncoerente, versione, err)
			}
			if len(r.Eventi) != len(dellaVersione) {
				return out, fmt.Errorf("%w: la versione %d rifa' %d eventi, salvati %d", ErrGiornaleIncoerente, versione, len(r.Eventi), len(dellaVersione))
			}
			for j, ev := range r.Eventi {
				s := dellaVersione[j]
				if ev.Azione != s.Azione || ev.ChiaveBozza != s.ChiaveBozza || ev.Origine != s.Origine || ev.Motivo != s.Motivo ||
					forma3(ev.Prima) != forma3(s.Prima) || forma3(ev.Dopo) != forma3(s.Dopo) {
					return out, fmt.Errorf("%w: l'evento %d (%s) non e' quello che il server scriverebbe", ErrGiornaleIncoerente, s.N, s.Azione)
				}
			}
			out.Verificate = append(out.Verificate, versione)
		}
		n0 += len(dellaVersione)
		s := snap
		prima = &s
	}
	if n0 != len(eventi) {
		return out, fmt.Errorf("%w: %d eventi oltre l'ultima versione", ErrGiornaleIncoerente, len(eventi)-n0)
	}
	return out, nil
}

// verificaPrimaVersione3: la versione 1. Una conversione si rifa' con conversione-1; le altre partenze vogliono
// l'adattatore (motivo non vuoto = non verificata).
func verificaPrimaVersione3(ctx context.Context, q *db.Queries, thread uuid.UUID, testa EventoLetto, snap Snapshot3) (string, error) {
	if testa.Azione != AzBozzaAperta || testa.Origine != "server" {
		return "", fmt.Errorf("%w: la versione 1 non comincia con bozza_aperta del server", ErrGiornaleIncoerente)
	}
	var d struct {
		Partenza  string `json:"partenza"`
		Algoritmo string `json:"algoritmo"`
		Impronta  string `json:"impronta"`
		DaBozza   *struct {
			BozzaID  uuid.UUID `json:"bozza_id"`
			Versione int32     `json:"versione"`
			Impronta string    `json:"impronta"`
		} `json:"da_bozza"`
	}
	if len(testa.Dopo) == 0 || json.Unmarshal(testa.Dopo, &d) != nil {
		return "versione 1 senza la partenza nell'evento di apertura: la rifa' l'adattatore, con l'ondata C", nil
	}
	if d.Partenza != PartenzaFormato2 {
		return fmt.Sprintf("versione 1 con la partenza %q: la rifa' l'adattatore del Motore A, con l'ondata C", d.Partenza), nil
	}
	if d.Algoritmo != AlgoritmoConversione || d.DaBozza == nil {
		return "", fmt.Errorf("%w: la conversione non dice l'algoritmo o la bozza di partenza", ErrGiornaleIncoerente)
	}
	v, err := q.GetDistintaBozzaVersione(ctx, db.GetDistintaBozzaVersioneParams{BozzaID: d.DaBozza.BozzaID, ThreadID: thread, Versione: d.DaBozza.Versione})
	if errors.Is(err, pgx.ErrNoRows) {
		return "la bozza formato 2 della conversione non c'e' piu' (si cancella 90 giorni dopo l'abbandono)", nil
	}
	if err != nil {
		return "", err
	}
	vecchio, improntaVecchia, err := rileggiSnapshot(v.Snapshot, v.Impronta)
	if err != nil {
		return "", err
	}
	if improntaVecchia != d.DaBozza.Impronta {
		return "", fmt.Errorf("%w: la bozza formato 2 della conversione non ha piu' l'impronta registrata", ErrGiornaleIncoerente)
	}
	cliente, err := q.GetClienteDellaRfq(ctx, thread)
	if err != nil {
		return "", err
	}
	rifatto, _, err := convertiSnapshot(ctx, q, thread, vecchio, EmittenteCliente(cliente))
	if err != nil {
		return fmt.Sprintf("conversione-1 oggi non si rifa' (%v): le occorrenze dei file sono cambiate dopo la conversione", err), nil
	}
	a, err := rifatto.Canonico()
	if err != nil {
		return "", err
	}
	b, err := snap.Canonico()
	if err != nil {
		return "", err
	}
	if jsonImpronta(a) != jsonImpronta(b) {
		// con la sorgente senza allegato le caselle si completano con l'occorrenza attiva di oggi, che puo' essere
		// cambiata (EM §6.7): allora la versione non si verifica; con l'allegato della sorgente il rifacimento e'
		// deterministico, e una differenza e' un guasto
		if vecchio.Sorgente == nil || vecchio.Sorgente.AllegatoID == nil {
			return "conversione-1 oggi completa le caselle con un'altra occorrenza attiva: la versione 1 non si rifa' con lo stato di oggi", nil
		}
		return "", fmt.Errorf("%w: la versione 1 non e' quella che conversione-1 da' dalla bozza formato 2", ErrGiornaleIncoerente)
	}
	return "", nil
}

// forma3: un prima o un dopo nella forma canonica, "" per assente o null.
func forma3(raw json.RawMessage) string {
	if nullo(raw) {
		return ""
	}
	return string(canonico(raw))
}
