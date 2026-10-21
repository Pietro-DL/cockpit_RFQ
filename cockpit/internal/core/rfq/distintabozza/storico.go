package distintabozza

// R7 (EM §9.7; PE-07, decisione [T] dell'integratore del 18/10): lo storico delle conferme di un prodotto nei formati 1, 2
// e 3, senza scrivere e senza convertire niente. Per ogni conferma: la testata, ultima e valida derivate alla lettura (C3
// §5.7), confermato_da_nome (PE-07) e, per il formato 3, i prerequisiti registrati (U2 §5.4: criterio, regola, fonte,
// firma, mono_pezzo, gerarchia e deroga; nessun si' o no per una conferma passata, e verifica_calcolata non c'e' piu': la
// verifica corrente la dice /dati.conferma_corrente, DEC-B RQ-8 superata da U2 §5.4); poi le
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
	Prerequisiti       *PrerequisitiR7 `json:"prerequisiti,omitempty"`
	F8Stato            string          `json:"f8_stato,omitempty"`
	// CorrezioneSorgente e Revisione: la conferma e' una revisione interna, correzione della sorgente (origine della
	// conferma scritta da R5: correzione_sorgente, revisione «interna»; G39-03 [U]: verso il cliente non e' una sua
	// revisione; forma [T-39])
	CorrezioneSorgente bool   `json:"correzione_sorgente,omitempty"`
	Revisione          string `json:"revisione,omitempty"`
}

// correzioneDiOrigine: la marcatura della correzione della sorgente in origine di una conferma (conferma3_scritture.go).
func correzioneDiOrigine(origine json.RawMessage) (bool, string) {
	var o struct {
		Correzione bool   `json:"correzione_sorgente"`
		Revisione  string `json:"revisione"`
	}
	if json.Unmarshal(origine, &o) != nil {
		return false, ""
	}
	return o.Correzione, o.Revisione
}

// PrerequisitiR7: i prerequisiti registrati di una conferma formato 3 (U2 §5.4, riga R7; [U] D-SHOCK-S1 B: criterio,
// origine_gerarchia, gerarchia_step, stato e motivo della deroga). Sono dati registrati, mai un esito della verifica: la
// regola e' prima-istanza-1, la fonte e' quella della conferma (sha256 e gruppo), la gerarchia e la deroga quelle che R5
// scrive in origine (assenti se la conferma non le porta: si dichiarano mancanti, non si ricostruiscono).
type PrerequisitiR7 struct {
	Criterio         string                       `json:"criterio"`
	Regola           string                       `json:"regola"`
	FonteSha256      string                       `json:"fonte_sha256,omitempty"`
	FonteGruppo      string                       `json:"fonte_gruppo,omitempty"`
	FirmaStruttura   string                       `json:"firma_struttura"`
	MonoPezzo        bool                         `json:"mono_pezzo"`
	OrigineGerarchia string                       `json:"origine_gerarchia,omitempty"`
	GerarchiaStep    *bool                        `json:"gerarchia_step,omitempty"`
	Deroga           *confermabom.DerogaStruttura `json:"deroga,omitempty"`
	Mancanti         []string                     `json:"mancanti,omitempty"`
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
//
// Origine (U2 §6.1, 0023 riga 7): non_includere (la decisione, con azione e motivo) oppure assenza, legacy, ereditata,
// escludi (la traccia di un'esclusione senza decisione propria: decisione, azione e motivo possono mancare, e il motivo
// che manca si dichiara, non si inventa).
type NonInclusoR7 struct {
	Origine        string          `json:"origine"`
	DecisioneID    string          `json:"decisione_id,omitempty"`
	Sha256         string          `json:"sha256"`
	Chiave         string          `json:"chiave"`
	AllegatoID     uuid.UUID       `json:"allegato_id"`
	Gruppo         string          `json:"gruppo"`
	AzioneID       *uuid.UUID      `json:"azione_id,omitempty"`
	MotivoCodice   string          `json:"motivo_codice,omitempty"`
	Motivo         string          `json:"motivo,omitempty"`
	MotivoMancante bool            `json:"motivo_mancante,omitempty"`
	Segnale        json.RawMessage `json:"segnale,omitempty"`
	RigheChiuse    []uuid.UUID     `json:"righe_chiuse"`
	ArchiChiusi    json.RawMessage `json:"archi_chiusi"`
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

// prerequisitiRegistrati: i prerequisiti di una conferma formato 3 (PrerequisitiR7), dalle colonne della conferma e da
// origine (origine_gerarchia, gerarchia_step, deroga, se R5 li ha scritti).
func prerequisitiRegistrati(c ConfermaR7, origine json.RawMessage) *PrerequisitiR7 {
	p := &PrerequisitiR7{Criterio: c.CriterioVerifica, Regola: confermabom.RegolaPrimaIstanza, FirmaStruttura: c.FirmaStruttura, MonoPezzo: c.MonoPezzo}
	if c.Fonte != nil {
		p.FonteSha256, p.FonteGruppo = c.Fonte.Sha256, c.Fonte.Gruppo
	}
	var o struct {
		OrigineGerarchia string                       `json:"origine_gerarchia"`
		GerarchiaStep    *bool                        `json:"gerarchia_step"`
		Deroga           *confermabom.DerogaStruttura `json:"deroga"`
	}
	_ = json.Unmarshal(origine, &o)
	p.OrigineGerarchia, p.GerarchiaStep, p.Deroga = o.OrigineGerarchia, o.GerarchiaStep, o.Deroga
	if p.OrigineGerarchia == "" {
		p.Mancanti = append(p.Mancanti, "origine_gerarchia")
	}
	if p.GerarchiaStep == nil {
		p.Mancanti = append(p.Mancanti, "gerarchia_step")
	}
	return p
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
			MonoPezzo: r.MonoPezzo, Origine: canonico(r.Origine), F8Stato: r.F8Stato.String, ImportazioniCitate: r.ImportazioniCitate}
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
		if r.Formato == FormatoSnapshot3 {
			c.Prerequisiti = prerequisitiRegistrati(c, r.Origine)
			c.CorrezioneSorgente, c.Revisione = correzioneDiOrigine(r.Origine)
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
		x := NonInclusoR7{Origine: n.Origine, DecisioneID: n.DecisioneID.String, Sha256: strings.TrimSpace(n.Sha256), Chiave: n.Chiave,
			AllegatoID: n.AllegatoID, Gruppo: n.Gruppo, MotivoCodice: n.MotivoCodice.String, Motivo: n.Motivo.String,
			MotivoMancante: !n.Motivo.Valid, RigheChiuse: n.RigheChiuse, ArchiChiusi: grezzoCanonico(n.ArchiChiusi)}
		if n.AzioneID.Valid {
			a := n.AzioneID.UUID
			x.AzioneID = &a
		}
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
