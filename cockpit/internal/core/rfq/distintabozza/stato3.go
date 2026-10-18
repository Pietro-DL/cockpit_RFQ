package distintabozza

// Lo stato di una bozza formato 3 rispetto al database (EM §6.4): i rif scaduti, la base valida o superata, la versione
// corrente del prodotto. Solo letture: R1 le fa in una transazione in sola lettura, R2 sotto BloccaThread.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// I motivi di un rif scaduto (EM §6.4, condizioni 1-5; i nomi sono [T] della corsia persistenza, ondata B).
const (
	ScadutoAllegatoFuoriRFQ = "allegato_fuori_rfq" // 1: l'allegato non e' piu' della RFQ
	ScadutoFattiAssenti     = "fatti_assenti"      // 2: l'allegato non ha i fatti correnti per (sha256, analizzatore, configurazione)
	ScadutoChiaveAssente    = "chiave_assente"     // 3: la chiave PRODUCT non c'e' piu' nella struttura di quei fatti
	ScadutoShaDiverso       = "sha_diverso"        // 4: lo sha256 del rif non e' quello dell'allegato
	ScadutoFileNonAttivo    = "file_non_attivo"    // 5: l'occorrenza non e' attiva (Altri file o scartata, EM §10.8)
)

// RifScaduto e' una casella scaduta, di un nodo o di una decisione non_includere (EM §6.4: {chiave_bozza | decisione,
// casella, motivo}).
type RifScaduto struct {
	ChiaveBozza string   `json:"chiave_bozza,omitempty"`
	Decisione   string   `json:"decisione,omitempty"`
	Casella     Casella3 `json:"casella"`
	Motivo      string   `json:"motivo"`
}

// shaEChiave scompone un rif nodo:<sha256>:<chiave PRODUCT> (la forma l'ha gia' controllata il lettore).
func shaEChiave(rif string) (string, string) {
	if len(rif) < 70 || !strings.HasPrefix(rif, "nodo:") {
		return "", ""
	}
	return rif[5:69], rif[70:]
}

// AllegatoCitato e' un allegato come lo vedono i controlli delle caselle: lo sha256, se e' della RFQ e se e' attivo.
type AllegatoCitato struct {
	Sha256   string
	DellaRfq bool
	Attivo   bool
}

// LeggiAllegatiCitati legge gli allegati dati (uno per riga anche se non esistono).
func LeggiAllegatiCitati(ctx context.Context, q *db.Queries, thread uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]AllegatoCitato, error) {
	out := map[uuid.UUID]AllegatoCitato{}
	if len(ids) == 0 {
		return out, nil
	}
	visti := map[uuid.UUID]bool{}
	var unici []uuid.UUID
	for _, id := range ids {
		if !visti[id] {
			visti[id] = true
			unici = append(unici, id)
		}
	}
	rr, err := q.ListAllegatiCitati(ctx, db.ListAllegatiCitatiParams{ThreadID: thread, Allegati: unici})
	if err != nil {
		return nil, err
	}
	for _, r := range rr {
		out[r.AllegatoID] = AllegatoCitato{Sha256: strings.TrimSpace(r.Sha256.String), DellaRfq: r.DellaRfq, Attivo: r.Attivo}
	}
	return out, nil
}

// chiaviDeiFatti: le chiavi PRODUCT della struttura dei fatti correnti di un contenuto; ok falso senza fatti correnti
// o senza una struttura leggibile.
func chiaviDeiFatti(ctx context.Context, q *db.Queries, sha string) (map[string]bool, bool, error) {
	af, err := q.GetAnalisiCorrente(ctx, sha)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	st, ok := worker.DecodificaStruttura(af.Fatti)
	if !ok {
		return nil, false, nil
	}
	out := make(map[string]bool, len(st.Nodi))
	for _, n := range st.Nodi {
		out[n.Chiave] = true
	}
	return out, true, nil
}

// RifScaduti calcola i rif scaduti di uno snapshot formato 3 con letture del database (EM §6.4): le caselle dei nodi e
// delle decisioni non_includere, con il primo motivo che vale nell'ordine delle condizioni. La sola sparizione del gruppo
// dalle candidate di /dati non fa scadere una casella (Δ-V1-19).
func RifScaduti(ctx context.Context, q *db.Queries, thread uuid.UUID, s Snapshot3) ([]RifScaduto, error) {
	type rif struct {
		chiave, decisione string
		c                 Casella3
	}
	var tutti []rif
	var ids []uuid.UUID
	for _, n := range s.Nodi {
		for _, c := range n.Caselle {
			tutti = append(tutti, rif{chiave: n.ChiaveBozza, c: c})
			ids = append(ids, c.AllegatoID)
		}
	}
	for _, d := range s.Decisioni {
		if d.Tipo == DecisioneNonIncludere && d.Casella != nil {
			tutti = append(tutti, rif{decisione: d.ID, c: *d.Casella})
			ids = append(ids, d.Casella.AllegatoID)
		}
	}
	out := []RifScaduto{}
	if len(tutti) == 0 {
		return out, nil
	}
	allegati, err := LeggiAllegatiCitati(ctx, q, thread, ids)
	if err != nil {
		return nil, err
	}
	fatti := map[string]map[string]bool{}
	conFatti := map[string]bool{}
	for _, r := range tutti {
		sha, chiave := shaEChiave(r.c.Rif)
		a := allegati[r.c.AllegatoID]
		motivo := ""
		switch {
		case !a.DellaRfq:
			motivo = ScadutoAllegatoFuoriRFQ
		case a.Sha256 != sha:
			motivo = ScadutoShaDiverso
		case !a.Attivo:
			motivo = ScadutoFileNonAttivo
		default:
			kk, letto := fatti[sha]
			if _, fatto := conFatti[sha]; !fatto {
				var ok bool
				if kk, ok, err = chiaviDeiFatti(ctx, q, sha); err != nil {
					return nil, err
				}
				fatti[sha], conFatti[sha] = kk, ok
				letto = ok
			} else {
				letto = conFatti[sha]
			}
			switch {
			case !letto:
				motivo = ScadutoFattiAssenti
			case !kk[chiave]:
				motivo = ScadutoChiaveAssente
			}
		}
		if motivo != "" {
			out = append(out, RifScaduto{ChiaveBozza: r.chiave, Decisione: r.decisione, Casella: r.c, Motivo: motivo})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ChiaveBozza != out[j].ChiaveBozza {
			return out[i].ChiaveBozza < out[j].ChiaveBozza
		}
		if out[i].Decisione != out[j].Decisione {
			return out[i].Decisione < out[j].Decisione
		}
		return out[i].Casella.Rif < out[j].Casella.Rif
	})
	return out, nil
}

// StatoBase e' lo stato della base di una bozza (C3 §4.2; EM §6.4): valida, oppure superata con i motivi.
type StatoBase struct {
	Stato  string   `json:"stato"`
	Motivi []string `json:"motivi"`
}

// Gli stati della base.
const (
	BaseValida   = "valida"
	BaseSuperata = "superata"
)

// MotivoFonteRifiutata e MotivoRifScaduti: i due motivi di BASE_SUPERATA che dipendono dalla fonte (C3 §4.4 n. 1).
const (
	MotivoFonteRifiutata = "fonte_rifiutata"
	MotivoRifScaduti     = "rif_scaduti"
)

// VersioneCorrenteDi: la versione corrente del prodotto (ultima conferma, contatore, firma del perimetro ricalcolata
// adesso, fonte autorizzata), che R1 restituisce e che la base di un rebase deve dire.
func VersioneCorrenteDi(st *confermabom.StatoRFQ, prodotto uuid.UUID) (confermabom.VersioneCorrente, error) {
	return st.VersioneCorrente(prodotto)
}

// BaseDellaBozza calcola lo stato della base di una bozza formato 3 (EM §6.4; V0-D54): la base vale se l'ultima conferma
// e' quella della bozza e il contatore oppure la firma del perimetro coincidono; superata anche con una fonte rifiutata
// (un rifiuto della sorgente in fonte_step_evento senza una scelta dopo) o con rif scaduti.
func BaseDellaBozza(ctx context.Context, q *db.Queries, thread uuid.UUID, s Snapshot3, st *confermabom.StatoRFQ, scaduti []RifScaduto) (StatoBase, error) {
	out := StatoBase{Stato: BaseValida, Motivi: []string{}}
	if s.VersioneBomBase != nil && s.FirmaBase != "" {
		b := confermabom.Base{ConfermaID: s.VersioneBomBase.ConfermaID, Numero: int32(s.VersioneBomBase.Numero),
			Contatore: s.VersioneBomBase.Contatore, Firma: s.FirmaBase}
		m, err := st.ControllaBase(s.ProdottoID, b)
		if err != nil {
			return out, fmt.Errorf("base della bozza: %w", err)
		}
		out.Motivi = append(out.Motivi, m...)
	}
	if s.Sorgente != nil && s.Sorgente.AllegatoID != nil {
		ee, err := q.ListFonteStepProdotto(ctx, db.ListFonteStepProdottoParams{ThreadID: thread, ProdottoID: s.ProdottoID})
		if err != nil {
			return out, err
		}
		rifiutata := false
		for _, e := range ee {
			if e.Ruolo != "prodotto" || e.AllegatoID != *s.Sorgente.AllegatoID || e.Gruppo.String != s.Sorgente.Gruppo {
				continue
			}
			switch e.Azione {
			case "rifiuto":
				rifiutata = true
			case "scelta", "sostituzione":
				rifiutata = false
			}
		}
		if rifiutata {
			out.Motivi = append(out.Motivi, MotivoFonteRifiutata)
		}
	}
	if len(scaduti) > 0 {
		out.Motivi = append(out.Motivi, MotivoRifScaduti)
	}
	if len(out.Motivi) > 0 {
		out.Stato = BaseSuperata
	}
	return out, nil
}
