package distintabozza

// R7 (EM §9.7; PE-07, decisione [T] dell'integratore del 18/10): lo storico delle conferme di un prodotto nei formati 1, 2
// e 3, senza scrivere e senza convertire niente. Per ogni conferma: la testata, ultima e valida derivate alla lettura (C3
// §5.7), verifica_calcolata e confermato_da_nome (PE-07: la pagina non legge piu' il formato per dire «Verificata»); poi le
// attestazioni (con da_riverificare derivato) e la storia della fonte (fonte_step_evento del prodotto, compresi gli atti
// esistenti che in V1 la scrivono). Con &conferma= il dettaglio: snapshot, delta, decisioni, mappa, componenti certificati
// (con emittente, presunzione e namespace nel formato 3), archi, non_inclusi[] e, con &importazione=1, tutti i registri
// citati. Ogni formato ha il suo lettore (lettori.go): un contenuto che il lettore non sa leggere si mostra con
// contenuto_non_leggibile e i campi grezzi, mai nascosto.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/platform/db"
)

// OpzioniR7 sono i parametri di R7: &conferma=<uuid>, &importazione=1.
type OpzioniR7 struct {
	Conferma     *uuid.UUID
	Importazione bool
}

// FonteConferma e' la fonte di una conferma (fonte_allegato_id, fonte_sha256, fonte_gruppo).
type FonteConferma struct {
	AllegatoID *uuid.UUID `json:"allegato_id,omitempty"`
	Sha256     string     `json:"sha256,omitempty"`
	Gruppo     string     `json:"gruppo,omitempty"`
}

// ConfermaR7 e' una conferma dell'elenco di R7.
type ConfermaR7 struct {
	ConfermaID         uuid.UUID       `json:"conferma_id"`
	Numero             int             `json:"numero"`
	Formato            int             `json:"formato"`
	SenzaRiepilogoV2   bool            `json:"senza_riepilogo_v2"`
	Ultima             bool            `json:"ultima"`
	Valida             bool            `json:"valida"`
	ConfermatoDa       uuid.UUID       `json:"confermato_da"`
	ConfermatoDaNome   string          `json:"confermato_da_nome"`
	ConfermatoDaSigla  string          `json:"confermato_da_sigla"`
	ConfermatoIl       time.Time       `json:"confermato_il"`
	FirmaStruttura     string          `json:"firma_struttura"`
	CriterioVerifica   string          `json:"criterio_verifica"`
	MonoPezzo          bool            `json:"mono_pezzo"`
	Origine            json.RawMessage `json:"origine"`
	BozzaID            *uuid.UUID      `json:"bozza_id,omitempty"`
	Fonte              *FonteConferma  `json:"fonte,omitempty"`
	ImportazioneID     *uuid.UUID      `json:"importazione_id,omitempty"`
	ImportazioniCitate []uuid.UUID     `json:"importazioni_citate,omitempty"`
	VerificaCalcolata  bool            `json:"verifica_calcolata"`
	F8Stato            string          `json:"f8_stato,omitempty"`
}

// AttestazioneR7 e' un'attestazione del fascicolo del prodotto, con da_riverificare derivato (C3 §5.7).
type AttestazioneR7 struct {
	AttestazioneID        uuid.UUID  `json:"attestazione_id"`
	Numero                int        `json:"numero"`
	ConfermaStrutturaleID *uuid.UUID `json:"conferma_strutturale_id,omitempty"`
	AttestatoDa           uuid.UUID  `json:"attestato_da"`
	AttestatoIl           time.Time  `json:"attestato_il"`
	DaRiverificare        bool       `json:"da_riverificare"`
}

// FonteR7 e' un evento della storia della fonte STEP del prodotto.
type FonteR7 struct {
	EventoID     int64          `json:"evento_id"`
	Ruolo        string         `json:"ruolo"`
	Azione       string         `json:"azione"`
	AllegatoID   uuid.UUID      `json:"allegato_id"`
	Sha256       string         `json:"sha256"`
	Gruppo       string         `json:"gruppo,omitempty"`
	DocumentoID  *uuid.UUID     `json:"documento_id,omitempty"`
	BozzaID      *uuid.UUID     `json:"bozza_id,omitempty"`
	ChiaveBozza  string         `json:"chiave_bozza,omitempty"`
	ComponenteID *uuid.UUID     `json:"componente_id,omitempty"`
	Precedente   *FonteConferma `json:"precedente,omitempty"`
	Motivo       string         `json:"motivo,omitempty"`
	AutoreID     uuid.UUID      `json:"autore_id"`
	Quando       time.Time      `json:"quando"`
}

// ComponenteCertificato e' una riga di conferma_strutturale_componente nel dettaglio di R7.
type ComponenteCertificato struct {
	ComponenteID      uuid.UUID `json:"componente_id"`
	Radice            bool      `json:"radice"`
	ChiaveBozza       string    `json:"chiave_bozza,omitempty"`
	Codice            string    `json:"codice"`
	Rev               string    `json:"rev,omitempty"`
	Descrizione       string    `json:"descrizione,omitempty"`
	Tipo              string    `json:"tipo"`
	Emittente         string    `json:"emittente,omitempty"`
	EmittentePresunto *bool     `json:"emittente_presunto,omitempty"`
	Namespace         string    `json:"namespace,omitempty"`
}

// ArcoCertificato e' una riga di conferma_strutturale_arco.
type ArcoCertificato struct {
	PadreID   uuid.UUID `json:"padre_id"`
	FiglioID  uuid.UUID `json:"figlio_id"`
	Qta       int       `json:"qta"`
	Posizione string    `json:"posizione,omitempty"`
	Origine   string    `json:"origine,omitempty"`
}

// NonInclusoR7 e' una riga di conferma_strutturale_non_incluso.
type NonInclusoR7 struct {
	DecisioneID  string          `json:"decisione_id"`
	Sha256       string          `json:"sha256"`
	Chiave       string          `json:"chiave"`
	AllegatoID   uuid.UUID       `json:"allegato_id"`
	Gruppo       string          `json:"gruppo"`
	AzioneID     uuid.UUID       `json:"azione_id"`
	MotivoCodice string          `json:"motivo_codice"`
	Motivo       string          `json:"motivo"`
	Segnale      json.RawMessage `json:"segnale,omitempty"`
	RigheChiuse  []uuid.UUID     `json:"righe_chiuse"`
	ArchiChiusi  json.RawMessage `json:"archi_chiusi"`
}

// DettaglioR7 e' il dettaglio di una conferma (&conferma=).
type DettaglioR7 struct {
	ConfermaR7
	Snapshot              json.RawMessage         `json:"snapshot"`
	ContenutoNonLeggibile bool                    `json:"contenuto_non_leggibile,omitempty"`
	MotivoNonLeggibile    string                  `json:"motivo_non_leggibile,omitempty"`
	Delta                 json.RawMessage         `json:"delta"`
	Decisioni             json.RawMessage         `json:"decisioni"`
	Mappa                 json.RawMessage         `json:"mappa"`
	Verifica              json.RawMessage         `json:"verifica"`
	Componenti            []ComponenteCertificato `json:"componenti"`
	Archi                 []ArcoCertificato       `json:"archi"`
	NonInclusi            []NonInclusoR7          `json:"non_inclusi"`
	Registri              []ImportazioneLetta     `json:"registri,omitempty"`
}

// StoricoR7 e' la risposta di R7.
type StoricoR7 struct {
	ProdottoID   uuid.UUID        `json:"prodotto_id"`
	Conferme     []ConfermaR7     `json:"conferme"`
	Attestazioni []AttestazioneR7 `json:"attestazioni"`
	Fonte        []FonteR7        `json:"fonte"`
	Dettaglio    *DettaglioR7     `json:"dettaglio,omitempty"`
}

// verificaCalcolata dice se la conferma porta una verifica calcolata: nel formato 3 di V1 la verifica registrata e' quella
// provvisoria «mai positiva» (EM §3.9, K-1 [U]; base di lavoro RV3-36 A), quindi falsa; nei formati 1 e 2 vero se la
// colonna verifica ne porta una (un oggetto non vuoto con un esito).
func verificaCalcolata(formato int16, verifica json.RawMessage) bool {
	if formato == FormatoSnapshot3 {
		return false
	}
	var v map[string]json.RawMessage
	if json.Unmarshal(verifica, &v) != nil || len(v) == 0 {
		return false
	}
	_, esito := v["esito"]
	_, verificata := v["verificata"]
	return esito || verificata
}

// LeggiStorico e' R7 (commento di testa). Chi chiama apre la transazione in sola lettura.
func LeggiStorico(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, op OpzioniR7) (StoricoR7, error) {
	if _, err := q.GetThread(ctx, thread); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StoricoR7{}, &Errore{Codice: CodiceRFQInesistente, Messaggio: "la RFQ non c'e'"}
		}
		return StoricoR7{}, err
	}
	p, err := q.GetProdottoDellaRfq(ctx, db.GetProdottoDellaRfqParams{ThreadID: thread, ComponenteID: prodotto})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && p.Tipo != db.TipoComponenteFinito) {
		return StoricoR7{}, fuori("prodotto", "il prodotto %s non e' un finito di questa RFQ", prodotto)
	}
	if err != nil {
		return StoricoR7{}, err
	}
	st, err := confermabom.LeggiStatoRFQ(ctx, q, thread)
	if err != nil {
		return StoricoR7{}, err
	}
	u, val, haUltima, err := st.ValiditaUltima(prodotto)
	if err != nil {
		return StoricoR7{}, err
	}
	righe, err := q.ListConfermeStoricoProdotto(ctx, db.ListConfermeStoricoProdottoParams{ThreadID: thread, ProdottoID: prodotto})
	if err != nil {
		return StoricoR7{}, err
	}
	out := StoricoR7{ProdottoID: prodotto, Conferme: []ConfermaR7{}, Attestazioni: []AttestazioneR7{}, Fonte: []FonteR7{}}
	perID := map[uuid.UUID]ConfermaR7{}
	for _, r := range righe {
		c := ConfermaR7{ConfermaID: r.ConfermaID, Numero: int(r.Numero), Formato: int(r.Formato), SenzaRiepilogoV2: r.SenzaRiepilogoV2,
			ConfermatoDa: r.ConfermatoDa, ConfermatoDaNome: r.ConfermatoDaNome, ConfermatoDaSigla: r.ConfermatoDaSigla,
			ConfermatoIl: r.ConfermatoIl, FirmaStruttura: strings.TrimSpace(r.FirmaStruttura), CriterioVerifica: r.CriterioVerifica,
			MonoPezzo: r.MonoPezzo, Origine: canonico(r.Origine), VerificaCalcolata: verificaCalcolata(r.Formato, r.Verifica),
			F8Stato: r.F8Stato.String, ImportazioniCitate: r.ImportazioniCitate}
		c.Ultima = haUltima && u.ID == r.ConfermaID
		c.Valida = c.Ultima && val.Valida
		if r.BozzaID.Valid {
			id := r.BozzaID.UUID
			c.BozzaID = &id
		}
		if r.FonteAllegatoID.Valid || r.FonteGruppo.Valid {
			f := &FonteConferma{Sha256: strings.TrimSpace(r.FonteSha256.String), Gruppo: r.FonteGruppo.String}
			if r.FonteAllegatoID.Valid {
				a := r.FonteAllegatoID.UUID
				f.AllegatoID = &a
			}
			c.Fonte = f
		}
		if r.ImportazioneID.Valid {
			id := r.ImportazioneID.UUID
			c.ImportazioneID = &id
		}
		out.Conferme = append(out.Conferme, c)
		perID[c.ConfermaID] = c
	}
	att, err := q.ListAttestazioniProdotto(ctx, db.ListAttestazioniProdottoParams{ThreadID: thread, ProdottoID: prodotto})
	if err != nil {
		return StoricoR7{}, err
	}
	for _, a := range att {
		x := AttestazioneR7{AttestazioneID: a.AttestazioneID, Numero: int(a.Numero), AttestatoDa: a.AttestatoDa, AttestatoIl: a.AttestatoIl}
		id := a.ConfermaStrutturaleID
		x.ConfermaStrutturaleID = &id
		// da riverificare: l'attestazione poggia su una conferma che non e' piu' l'ultima valida (C3 §5.7)
		x.DaRiverificare = true
		if c, ok := perID[id]; ok && c.Valida {
			x.DaRiverificare = false
		}
		out.Attestazioni = append(out.Attestazioni, x)
	}
	ee, err := q.ListFonteStepProdotto(ctx, db.ListFonteStepProdottoParams{ThreadID: thread, ProdottoID: prodotto})
	if err != nil {
		return StoricoR7{}, err
	}
	for _, e := range ee {
		f := FonteR7{EventoID: e.EventoID, Ruolo: e.Ruolo, Azione: e.Azione, AllegatoID: e.AllegatoID, Sha256: strings.TrimSpace(e.Sha256),
			Gruppo: e.Gruppo.String, ChiaveBozza: e.ChiaveBozza.String, Motivo: e.Motivo.String, AutoreID: e.AutoreID, Quando: e.Quando}
		if e.DocumentoID.Valid {
			id := e.DocumentoID.UUID
			f.DocumentoID = &id
		}
		if e.BozzaID.Valid {
			id := e.BozzaID.UUID
			f.BozzaID = &id
		}
		if e.ComponenteID.Valid {
			id := e.ComponenteID.UUID
			f.ComponenteID = &id
		}
		if e.PrecedenteAllegatoID.Valid {
			a := e.PrecedenteAllegatoID.UUID
			f.Precedente = &FonteConferma{AllegatoID: &a, Sha256: strings.TrimSpace(e.PrecedenteSha256.String), Gruppo: e.PrecedenteGruppo.String}
		}
		out.Fonte = append(out.Fonte, f)
	}
	if op.Conferma == nil {
		if op.Importazione {
			return StoricoR7{}, corpo("importazione", "?importazione=1 vale con il dettaglio di una conferma (&conferma=)")
		}
		return out, nil
	}
	c, ok := perID[*op.Conferma]
	if !ok {
		return StoricoR7{}, fuori("conferma", "la conferma %s non e' di questo prodotto della RFQ", *op.Conferma)
	}
	d, err := dettaglioConferma(ctx, q, thread, c, op.Importazione)
	if err != nil {
		return StoricoR7{}, err
	}
	out.Dettaglio = &d
	return out, nil
}

func dettaglioConferma(ctx context.Context, q *db.Queries, thread uuid.UUID, c ConfermaR7, conRegistri bool) (DettaglioR7, error) {
	r, err := q.GetConfermaStrutturale(ctx, db.GetConfermaStrutturaleParams{ThreadID: thread, ConfermaID: c.ConfermaID})
	if err != nil {
		return DettaglioR7{}, err
	}
	letto := LeggiSnapshotConferma(int(r.Formato), r.Snapshot, r.ImprontaSnapshot)
	d := DettaglioR7{ConfermaR7: c, Snapshot: letto.Canonico, ContenutoNonLeggibile: letto.NonLeggibile, MotivoNonLeggibile: letto.Motivo,
		Delta: grezzoCanonico(r.Delta), Decisioni: grezzoCanonico(r.Decisioni), Mappa: grezzoCanonico(r.Mappa), Verifica: grezzoCanonico(r.Verifica),
		Componenti: []ComponenteCertificato{}, Archi: []ArcoCertificato{}, NonInclusi: []NonInclusoR7{}}
	kk, err := q.ListConfermaStrutturaleComponenti(ctx, db.ListConfermaStrutturaleComponentiParams{ConfermaID: c.ConfermaID, ThreadID: thread})
	if err != nil {
		return DettaglioR7{}, err
	}
	for _, k := range kk {
		x := ComponenteCertificato{ComponenteID: k.ComponenteID, Radice: k.Radice, ChiaveBozza: k.ChiaveBozza.String, Codice: k.Codice,
			Rev: k.Rev.String, Descrizione: k.Descrizione.String, Tipo: string(k.Tipo), Emittente: k.Emittente.String, Namespace: k.Namespace.String}
		if k.EmittentePresunto.Valid {
			b := k.EmittentePresunto.Bool
			x.EmittentePresunto = &b
		}
		d.Componenti = append(d.Componenti, x)
	}
	aa, err := q.ListConfermaStrutturaleArchi(ctx, db.ListConfermaStrutturaleArchiParams{ConfermaID: c.ConfermaID, ThreadID: thread})
	if err != nil {
		return DettaglioR7{}, err
	}
	for _, a := range aa {
		d.Archi = append(d.Archi, ArcoCertificato{PadreID: a.PadreID, FiglioID: a.FiglioID, Qta: int(a.Qta), Posizione: a.Posizione.String,
			Origine: a.Origine.String})
	}
	nn, err := q.ListNonInclusiConferma(ctx, db.ListNonInclusiConfermaParams{ConfermaID: c.ConfermaID, ThreadID: thread})
	if err != nil {
		return DettaglioR7{}, err
	}
	for _, n := range nn {
		x := NonInclusoR7{DecisioneID: n.DecisioneID, Sha256: strings.TrimSpace(n.Sha256), Chiave: n.Chiave, AllegatoID: n.AllegatoID, Gruppo: n.Gruppo,
			AzioneID: n.AzioneID, MotivoCodice: n.MotivoCodice, Motivo: n.Motivo, RigheChiuse: n.RigheChiuse, ArchiChiusi: grezzoCanonico(n.ArchiChiusi)}
		if n.Segnale != nil {
			x.Segnale = grezzoCanonico(*n.Segnale)
		}
		if x.RigheChiuse == nil {
			x.RigheChiuse = []uuid.UUID{}
		}
		d.NonInclusi = append(d.NonInclusi, x)
	}
	if conRegistri && len(r.ImportazioniCitate) > 0 {
		rr, err := q.ListImportazioniPerID(ctx, db.ListImportazioniPerIDParams{ThreadID: thread, Importazioni: r.ImportazioniCitate})
		if err != nil {
			return DettaglioR7{}, err
		}
		for _, x := range rr {
			il, err := importazioneLettaDaRiga(db.GetDistintaImportazioneRow(x), true)
			if err != nil {
				return DettaglioR7{}, err
			}
			d.Registri = append(d.Registri, il)
		}
	}
	return d, nil
}

// grezzoCanonico: un JSON salvato nella sua forma canonica (o com'e', se non ne ha una).
func grezzoCanonico(r json.RawMessage) json.RawMessage {
	if len(r) == 0 {
		return nil
	}
	if c := canonico(r); c != nil {
		return c
	}
	return r
}
