package distintabozza

// impronta-ingressi-1 e R1 ?ingressi=1 (U2 §6.3.3; [U] RV3-25 A, meccanismo [T]): la lettura leggera dello stato degli
// ingressi del riepilogo, solo database, che la pagina rilegge ogni 10 secondi con il wizard aperto (5 con documenti in
// analisi) per sapere se avvisare. Quattro parti, ognuna calcolata con un'istruzione sola (queries/v1_ingressi.sql):
//   - bom (per RFQ): contatori della working, ultima conferma di ogni prodotto, step_strutturale_id dei finiti, ultimo
//     evento della storia della fonte;
//   - bozza: testata e versione della bozza aperta del prodotto;
//   - proposta (per RFQ): il contenuto delle righe di proposta, i fatti degli allegati attivi (chiave e digest), file_rfq,
//     gli allegati attivi, le deroghe, i documenti, piu' l'impronta delle regole caricate all'avvio;
//   - analisi: gli allegati con un lavoro pendente.
// Le stesse query servono al controllo finale di R5 per le parti proposta e analisi (H_S/H_T, U2 §6.3.5). Niente orari ne'
// id d'interpretazione: un risultato semanticamente identico non cambia l'impronta. Nessuna scrittura, nessun lucchetto:
// chi chiama apre una transazione REPEATABLE READ READ ONLY.
//
// impronta-ingressi-2 (direttiva 35, blocco C-1c; PD35 §6.2, INC-G12): la parte proposta segue la regola unica [T] di cio'
// che R5 scrive (queries/v1_ingressi.sql, IngressiProposta): fuori le colonne di copia di documento_proposta (entra la
// lettura del codice), fuori le righe che R5 segna (il segno di R5 e l'elenco RigheDiR5), dentro le AR degli allegati
// che la bozza del prodotto cita, viste come prima della transazione corrente.

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// AlgoritmoIngressi e' il nome dell'algoritmo dell'impronta degli ingressi (direttiva 35, C-1c: impronta-ingressi-2).
const AlgoritmoIngressi = "impronta-ingressi-2"

// Gli stati di un allegato in analisi (gli stessi di S1).
const (
	AnalisiDownloadInCoda   = "download_in_coda"
	AnalisiEstrazioneInCoda = "estrazione_in_coda"
	AnalisiInAnalisi        = "in_analisi"
)

// ParteBom: la parte bom di R1 ?ingressi=1.
type ParteBom struct {
	Impronta           string     `json:"impronta"`
	ContatoreStruttura int64      `json:"contatore_struttura"`
	ContatoreDati      int64      `json:"contatore_dati"`
	UltimaConfermaID   *uuid.UUID `json:"ultima_conferma_id"`
	StepStrutturaleID  *uuid.UUID `json:"step_strutturale_id"`
	FonteEventoMax     int64      `json:"fonte_evento_max"`
}

// ParteBozza: la parte bozza (senza bozza aperta, solo l'impronta).
type ParteBozza struct {
	Impronta     string     `json:"impronta"`
	BozzaID      *uuid.UUID `json:"bozza_id,omitempty"`
	Versione     *int       `json:"versione,omitempty"`
	AggiornataDa *uuid.UUID `json:"aggiornata_da,omitempty"`
	AggiornataIl *time.Time `json:"aggiornata_il,omitempty"`
}

// ParteProposta: la parte proposta.
type ParteProposta struct {
	Impronta string `json:"impronta"`
}

// FileInAnalisi: un allegato con un lavoro pendente.
type FileInAnalisi struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	NomeFile   string    `json:"nome_file"`
	Stato      string    `json:"stato"`
}

// ParteAnalisi: la parte analisi.
type ParteAnalisi struct {
	Impronta  string          `json:"impronta"`
	InAnalisi []FileInAnalisi `json:"in_analisi"`
}

// PartiIngressi: le quattro parti.
type PartiIngressi struct {
	Bom      ParteBom      `json:"bom"`
	Bozza    ParteBozza    `json:"bozza"`
	Proposta ParteProposta `json:"proposta"`
	Analisi  ParteAnalisi  `json:"analisi"`
}

// Ingressi e' la risposta di R1 ?ingressi=1.
type Ingressi struct {
	ProdottoID       uuid.UUID     `json:"prodotto_id"`
	Algoritmo        string        `json:"algoritmo"`
	ImprontaIngressi string        `json:"impronta_ingressi"`
	Parti            PartiIngressi `json:"parti"`
}

// LeggiIngressi e' R1 ?ingressi=1: la RFQ (RFQ_INESISTENTE), il prodotto finito della RFQ (FUORI_RFQ), poi le quattro
// parti. Risponde anche senza bozza. improntaRegole: l'impronta delle regole caricate all'avvio (la da' chi chiama).
func LeggiIngressi(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, improntaRegole string) (Ingressi, error) {
	out := Ingressi{ProdottoID: prodotto, Algoritmo: AlgoritmoIngressi}
	if _, err := q.GetThread(ctx, thread); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, &Errore{Codice: CodiceRFQInesistente, Messaggio: "la RFQ non c'e'"}
		}
		return out, err
	}
	if _, err := prodottoDellaRfq(ctx, q, thread, prodotto, false); err != nil {
		return out, err
	}
	bom, err := ParteBomDi(ctx, q, thread, prodotto)
	if err != nil {
		return out, err
	}
	bozza, err := ParteBozzaDi(ctx, q, thread, prodotto)
	if err != nil {
		return out, err
	}
	prop, err := PartePropostaDi(ctx, q, thread, prodotto, improntaRegole, nil)
	if err != nil {
		return out, err
	}
	an, err := ParteAnalisiDi(ctx, q, thread)
	if err != nil {
		return out, err
	}
	out.Parti = PartiIngressi{Bom: bom, Bozza: bozza, Proposta: prop, Analisi: an}
	out.ImprontaIngressi = improntaDi("ingressi-2", bom.Impronta, bozza.Impronta, prop.Impronta, an.Impronta)
	return out, nil
}

// improntaDi: lo sha256 dei pezzi uniti da «|», con il nome dell'algoritmo davanti.
func improntaDi(pezzi ...string) string {
	return jsoncanonico.Impronta([]byte(strings.Join(pezzi, "|")))
}

// ParteBomDi: la parte bom (una istruzione).
func ParteBomDi(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID) (ParteBom, error) {
	r, err := q.IngressiBom(ctx, db.IngressiBomParams{ThreadID: thread, ProdottoID: prodotto})
	if err != nil {
		return ParteBom{}, err
	}
	p := ParteBom{Impronta: r.Impronta, ContatoreStruttura: r.ContatoreStruttura, ContatoreDati: r.ContatoreDati, FonteEventoMax: r.FonteEventoMax}
	if u, err := uuid.Parse(r.UltimaConfermaID); err == nil {
		p.UltimaConfermaID = &u
	}
	if u, err := uuid.Parse(r.StepStrutturaleID); err == nil {
		p.StepStrutturaleID = &u
	}
	return p, nil
}

// ParteBozzaDi: la parte bozza (una istruzione).
func ParteBozzaDi(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID) (ParteBozza, error) {
	rr, err := q.IngressiBozza(ctx, db.IngressiBozzaParams{ThreadID: thread, ProdottoID: prodotto})
	if err != nil {
		return ParteBozza{}, err
	}
	if len(rr) == 0 {
		return ParteBozza{Impronta: improntaDi("bozza-1", "nessuna")}, nil
	}
	r := rr[0]
	v := int(r.Versione)
	id, da, il := r.BozzaID, r.AggiornataDa, r.AggiornataIl
	return ParteBozza{Impronta: improntaDi("bozza-1", r.BozzaID.String(), strconv.Itoa(v)), BozzaID: &id, Versione: &v, AggiornataDa: &da, AggiornataIl: &il}, nil
}

// ArcoProposta: una riga di relazione_proposta (allegato, padre, figlio).
type ArcoProposta struct {
	Allegato      uuid.UUID
	Padre, Figlio string
}

// RigheDiR5: l'elenco di R5 (U2 §6.3.5 passo 4) per la parte proposta di impronta-ingressi-2: gli allegati che la bozza
// cita (PB e svincoli) e le righe di proposta che R5 segnera'. Con lo stesso elenco H_S, calcolata prima delle scritture, e
// H_T, calcolata dopo, coincidono senza cambiamenti esterni (CN-05b). nil: gli allegati citati dalla bozza aperta del
// prodotto e nessuna riga esclusa oltre a quelle col segno di R5 (R1 ?ingressi=1).
type RigheDiR5 struct {
	AllegatiCitati []uuid.UUID
	Proposte       []uuid.UUID
	Archi          []ArcoProposta
	// Ricodificati: i componenti che R5 ricodifica: il codice delle loro copie legacy non entra (IngressiProposta (2-ter))
	Ricodificati []uuid.UUID
}

// PartePropostaDi: la parte proposta (una istruzione), con l'impronta delle regole caricate (impronta-ingressi-2).
func PartePropostaDi(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, improntaRegole string, r5 *RigheDiR5) (ParteProposta, error) {
	p := db.IngressiPropostaParams{ThreadID: thread, ProdottoID: prodotto, CitatiDallaBozza: true}
	if r5 != nil {
		p.CitatiDallaBozza = false
		p.AllegatiCitati, p.EscludiProposte, p.EscludiCodiceComponenti = r5.AllegatiCitati, r5.Proposte, r5.Ricodificati
		for _, a := range r5.Archi {
			p.EscludiArchi = append(p.EscludiArchi, a.Allegato.String()+"|"+a.Padre+"|"+a.Figlio)
		}
	}
	sql, err := q.IngressiProposta(ctx, p)
	if err != nil {
		return ParteProposta{}, err
	}
	return ParteProposta{Impronta: improntaDi("proposta-2", sql, "regole:"+improntaRegole)}, nil
}

// ParteAnalisiDi: la parte analisi (una istruzione): gli allegati con un lavoro pendente, con lo stato di S1 (il download
// prima dell'estrazione, l'estrazione prima dell'analisi), in ordine di allegato.
func ParteAnalisiDi(ctx context.Context, q *db.Queries, thread uuid.UUID) (ParteAnalisi, error) {
	rr, err := q.IngressiAnalisi(ctx, thread)
	if err != nil {
		return ParteAnalisi{}, err
	}
	rango := map[string]int{"stage_allegato": 3, "estrai_archivio": 2, "analizza_allegato": 1}
	stato := map[string]string{"stage_allegato": AnalisiDownloadInCoda, "estrai_archivio": AnalisiEstrazioneInCoda, "analizza_allegato": AnalisiInAnalisi}
	per := map[uuid.UUID]FileInAnalisi{}
	miglior := map[uuid.UUID]int{}
	for _, r := range rr {
		if rango[r.Tipo] > miglior[r.AllegatoID] {
			miglior[r.AllegatoID] = rango[r.Tipo]
			per[r.AllegatoID] = FileInAnalisi{AllegatoID: r.AllegatoID, NomeFile: r.NomeFile, Stato: stato[r.Tipo]}
		}
	}
	out := ParteAnalisi{InAnalisi: []FileInAnalisi{}}
	for _, f := range per {
		out.InAnalisi = append(out.InAnalisi, f)
	}
	sort.Slice(out.InAnalisi, func(i, j int) bool {
		return out.InAnalisi[i].AllegatoID.String() < out.InAnalisi[j].AllegatoID.String()
	})
	pezzi := []string{"analisi-1"}
	for _, f := range out.InAnalisi {
		pezzi = append(pezzi, f.AllegatoID.String()+":"+f.Stato)
	}
	out.Impronta = improntaDi(pezzi...)
	return out, nil
}
