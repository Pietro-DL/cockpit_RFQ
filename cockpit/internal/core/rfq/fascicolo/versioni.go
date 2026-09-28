package fascicolo

// Congelare, aprire una revisione, abbandonarla (addendum A4.6, A4.7; decisioni D25a, D25b, D25c, D30,
// D37). Ognuna e' una transazione sola, quella di chi chiama: il primo passo prende la riga della RFQ
// FOR UPDATE, cosi' due gesti sulla stessa RFQ si mettono in fila, e il trigger della working bloccata
// (che prende la stessa riga FOR KEY SHARE) aspetta che il gesto finisca.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

// Congelamento e' quello che il congelamento ha fatto.
type Congelamento struct {
	Versione db.BomVersione
	Fase     db.Fase // la fase aperta dopo: SCHEDA_COSTO per una versione preventivo, invariata per una tecnica
	Avvisi   []string
}

// CongelaBom congela la BOM working della RFQ (A4.6): gate, versione, istantanee, congelamento e, per
// una versione preventivo, il passaggio FATTIBILITA → SCHEDA_COSTO con l'ancora della baseline.
// Qualunque rifiuto o errore lascia tutto com'era, perche' chi chiama annulla la transazione.
func CongelaBom(ctx context.Context, q *db.Queries, thread, utente uuid.UUID, motivo string) (Congelamento, error) {
	if err := bloccaThread(ctx, q, thread); err != nil {
		return Congelamento{}, err
	}
	fase, err := faseAperta(ctx, q, thread)
	if err != nil {
		return Congelamento{}, err
	}
	ultima, esiste, err := ultimaVersione(ctx, q, thread)
	if err != nil {
		return Congelamento{}, err
	}
	var contesto db.ContestoBom
	switch {
	case esiste && ultima.Stato == db.StatoBomBozza:
		contesto = ultima.Contesto
	case esiste:
		return Congelamento{}, Rifiuto(fmt.Sprintf("la V%d è già congelata e non c'è una revisione aperta: prima si apre una revisione", ultima.Numero))
	default:
		// la V1 nasce al primo congelamento (D30), e il contesto lo dice la fase
		switch fase.NomeFase {
		case db.FaseFATTIBILITA:
			contesto = db.ContestoBomPreventivo
		case db.FaseORDINE, db.FasePRODUZIONE:
			contesto = db.ContestoBomTecnica
		default:
			return Congelamento{}, Rifiuto(fmt.Sprintf("la prima baseline nasce in FATTIBILITA, oppure in ORDINE o PRODUZIONE per una RFQ arrivata lì senza baseline: la RFQ è in %s", fase.NomeFase))
		}
	}
	if !faseDaCongelamento(contesto, fase.NomeFase) {
		return Congelamento{}, Rifiuto(fmt.Sprintf("una revisione %s si congela %s: la RFQ è in %s", contesto, doveSiCongela(contesto), fase.NomeFase))
	}
	g, err := LeggiGate(ctx, q, thread)
	if err != nil {
		return Congelamento{}, err
	}
	if !g.Passa() {
		return Congelamento{}, Rifiuto("non si congela: " + g.Motivo())
	}
	v := ultima
	if !esiste {
		if strings.TrimSpace(motivo) == "" {
			motivo = "prima baseline"
		}
		if v, err = q.InsertBomVersione(ctx, db.InsertBomVersioneParams{ThreadID: thread, Numero: 1, Contesto: contesto,
			FaseAllApertura: fase.NomeFase, FaseLogIDAllApertura: fase.FaseLogID, Motivo: strings.TrimSpace(motivo), CreataDa: utente}); err != nil {
			return Congelamento{}, err
		}
	}
	arg := db.SnapshotComponentiParams{BomVersioneID: v.BomVersioneID, ThreadID: thread}
	if _, err := q.SnapshotComponenti(ctx, arg); err != nil {
		return Congelamento{}, fmt.Errorf("istantanea dei componenti: %w", err)
	}
	if _, err := q.SnapshotRelazioni(ctx, db.SnapshotRelazioniParams(arg)); err != nil {
		return Congelamento{}, fmt.Errorf("istantanea delle relazioni: %w", err)
	}
	if _, err := q.SnapshotDocumenti(ctx, db.SnapshotDocumentiParams(arg)); err != nil {
		return Congelamento{}, fmt.Errorf("istantanea dei documenti: %w", err)
	}
	if _, err := q.SnapshotDeroghe(ctx, db.SnapshotDerogheParams(arg)); err != nil {
		return Congelamento{}, fmt.Errorf("istantanea delle deroghe: %w", err)
	}
	n, err := q.CongelaBomVersione(ctx, db.CongelaBomVersioneParams{BomVersioneID: v.BomVersioneID, CongelataDa: uuid.NullUUID{UUID: utente, Valid: true}})
	if err != nil {
		return Congelamento{}, err
	}
	if n != 1 {
		return Congelamento{}, fmt.Errorf("V%d: il congelamento non ha trovato la bozza", v.Numero)
	}
	esito := Congelamento{Fase: fase.NomeFase, Avvisi: g.Avvisi}
	if contesto == db.ContestoBomPreventivo {
		adesso := time.Now()
		if err := chiudiFase(ctx, q, thread, adesso, db.EsitoFaseOK, fmt.Sprintf("BOM V%d congelata", v.Numero)); err != nil {
			return Congelamento{}, err
		}
		if _, err := q.ApriFaseConBom(ctx, db.ApriFaseConBomParams{ThreadID: thread, NomeFase: db.FaseSCHEDACOSTO, Inizio: adesso,
			BomVersioneID: uuid.NullUUID{UUID: v.BomVersioneID, Valid: true}}); err != nil {
			return Congelamento{}, err
		}
		esito.Fase = db.FaseSCHEDACOSTO
	}
	if esito.Versione, err = q.GetBomVersione(ctx, v.BomVersioneID); err != nil {
		return Congelamento{}, err
	}
	return esito, nil
}

// faseDaCongelamento: una revisione preventivo si congela da FATTIBILITA; una tecnica dalla fase in cui
// si trova la RFQ, che durante la bozza puo' essere andata avanti (da DISTINTA_ERP a ORDINE, per esempio).
func faseDaCongelamento(c db.ContestoBom, f db.Fase) bool {
	if c == db.ContestoBomPreventivo {
		return f == db.FaseFATTIBILITA
	}
	switch f {
	case db.FaseACCETTATA, db.FaseDISTINTAERP, db.FaseORDINE, db.FasePRODUZIONE:
		return true
	}
	return false
}

func doveSiCongela(c db.ContestoBom) string {
	if c == db.ContestoBomPreventivo {
		return "da FATTIBILITA"
	}
	return "da ACCETTATA, DISTINTA_ERP, ORDINE o PRODUZIONE"
}

// ApriRevisione apre la revisione V(n+1) sull'ultima baseline (A4.7). Il contesto lo decide la fase,
// tranne in ACCETTATA e DISTINTA_ERP dove lo sceglie chi apre (D25c): scelta nil = nessuna scelta.
// Una revisione preventivo porta la RFQ in FATTIBILITA; una tecnica lascia la fase dov'e'.
func ApriRevisione(ctx context.Context, q *db.Queries, thread, utente uuid.UUID, scelta *db.ContestoBom, motivo string) (db.BomVersione, error) {
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return db.BomVersione{}, Rifiuto("una revisione si apre con il suo motivo")
	}
	if err := bloccaThread(ctx, q, thread); err != nil {
		return db.BomVersione{}, err
	}
	ultima, esiste, err := ultimaVersione(ctx, q, thread)
	if err != nil {
		return db.BomVersione{}, err
	}
	if !esiste {
		return db.BomVersione{}, Rifiuto("la BOM non è mai stata congelata: non c'è niente da rivedere, si lavora sulla working")
	}
	if ultima.Stato == db.StatoBomBozza {
		return db.BomVersione{}, Rifiuto(fmt.Sprintf("la revisione V%d è già aperta", ultima.Numero))
	}
	fase, err := faseAperta(ctx, q, thread)
	if err != nil {
		return db.BomVersione{}, err
	}
	contesto, err := contestoRevisione(fase.NomeFase, scelta)
	if err != nil {
		return db.BomVersione{}, err
	}
	v, err := q.InsertBomVersione(ctx, db.InsertBomVersioneParams{ThreadID: thread, Numero: ultima.Numero + 1, Contesto: contesto,
		FaseAllApertura: fase.NomeFase, FaseLogIDAllApertura: fase.FaseLogID,
		VersionePrecedenteID: uuid.NullUUID{UUID: ultima.BomVersioneID, Valid: true},
		NumeroPrecedente:     pgtype.Int4{Int32: ultima.Numero, Valid: true},
		StatoPrecedente:      db.NullStatoBom{StatoBom: db.StatoBomCongelata, Valid: true},
		Motivo:               motivo, CreataDa: utente})
	if err != nil {
		return db.BomVersione{}, err
	}
	if contesto == db.ContestoBomPreventivo {
		adesso := time.Now()
		if err := chiudiFase(ctx, q, thread, adesso, db.EsitoFaseRINVIATA, fmt.Sprintf("revisione della BOM V%d", v.Numero)); err != nil {
			return db.BomVersione{}, err
		}
		if _, err := q.ApriFaseConBom(ctx, db.ApriFaseConBomParams{ThreadID: thread, NomeFase: db.FaseFATTIBILITA, Inizio: adesso}); err != nil {
			return db.BomVersione{}, err
		}
	}
	return v, nil
}

// contestoRevisione e' la tabella di A4.7: che revisione si apre da quale fase, e chi lo decide.
func contestoRevisione(f db.Fase, scelta *db.ContestoBom) (db.ContestoBom, error) {
	var fisso db.ContestoBom
	switch f {
	case db.FaseSCHEDACOSTO, db.FaseOFFERTEFORN, db.FaseOFFERTAINVIATA:
		fisso = db.ContestoBomPreventivo
	case db.FaseORDINE, db.FasePRODUZIONE:
		fisso = db.ContestoBomTecnica
	case db.FaseACCETTATA, db.FaseDISTINTAERP:
		if scelta == nil || !scelta.Valid() {
			return "", Rifiuto(fmt.Sprintf("in %s il tipo di revisione lo sceglie chi la apre: preventivo (il prezzo può cambiare, la RFQ torna in FATTIBILITA) oppure tecnica (il prezzo resta, la fase non cambia)", f))
		}
		return *scelta, nil
	case db.FasePERSA, db.FaseRESPINTA, db.FaseSCADUTA:
		return "", Rifiuto(fmt.Sprintf("la RFQ è chiusa (%s): nessuna revisione", f))
	default:
		return "", Rifiuto(fmt.Sprintf("la RFQ è in %s con la BOM congelata: la fase è stata spostata a mano, e va guardata prima di aprire una revisione", f))
	}
	if scelta != nil && *scelta != fisso {
		return "", Rifiuto(fmt.Sprintf("in %s la revisione è %s, non %s", f, fisso, *scelta))
	}
	return fisso, nil
}

// AbbandonaBozza elimina la revisione aperta, solo se la working e' ancora identica alla versione da
// cui e' partita (A4.7). Una revisione preventivo riporta la RFQ alla fase di apertura, con il
// responsabile della riga interrotta, se nel frattempo la fase si e' mossa solo fra FATTIBILITA e
// ATTESA_DISEGNI (D37); altrimenti si rifiuta e dice dove sta la RFQ.
func AbbandonaBozza(ctx context.Context, q *db.Queries, thread uuid.UUID) (string, error) {
	if err := bloccaThread(ctx, q, thread); err != nil {
		return "", err
	}
	bozza, err := q.GetBozzaAperta(ctx, thread)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", Rifiuto("non c'è una revisione aperta")
	}
	if err != nil {
		return "", err
	}
	if !bozza.VersionePrecedenteID.Valid {
		return "", Rifiuto(fmt.Sprintf("la V%d non ha una versione da cui ripartire", bozza.Numero))
	}
	diff, err := DifferenzeWorking(ctx, q, thread, bozza.VersionePrecedenteID.UUID)
	if err != nil {
		return "", err
	}
	if len(diff) > 0 {
		return "", Rifiuto(fmt.Sprintf("la BOM working non è più la V%d (%s): si congela la revisione oppure si riportano indietro le modifiche",
			bozza.NumeroPrecedente.Int32, elencoBreve(diff)))
	}
	if bozza.Contesto == db.ContestoBomPreventivo {
		dopo, err := q.ListFasiDopo(ctx, db.ListFasiDopoParams{ThreadID: thread, FaseLogID: bozza.FaseLogIDAllApertura})
		if err != nil {
			return "", err
		}
		for _, f := range dopo {
			if f.NomeFase != db.FaseFATTIBILITA && f.NomeFase != db.FaseATTESADISEGNI {
				return "", Rifiuto(fmt.Sprintf("dopo l'apertura della revisione la RFQ è passata per %s: l'abbandono non la riporta a %s", f.NomeFase, bozza.FaseAllApertura))
			}
		}
		aperta, err := faseAperta(ctx, q, thread)
		if err != nil {
			return "", err
		}
		if aperta.NomeFase != db.FaseFATTIBILITA && aperta.NomeFase != db.FaseATTESADISEGNI {
			return "", Rifiuto(fmt.Sprintf("la RFQ è in %s: l'abbandono la riporta a %s solo da FATTIBILITA o ATTESA_DISEGNI", aperta.NomeFase, bozza.FaseAllApertura))
		}
		riga, err := q.GetFaseLog(ctx, bozza.FaseLogIDAllApertura)
		if err != nil {
			return "", err
		}
		adesso := time.Now()
		if err := chiudiFase(ctx, q, thread, adesso, db.EsitoFaseRINVIATA, fmt.Sprintf("revisione V%d abbandonata", bozza.Numero)); err != nil {
			return "", err
		}
		// La riga interrotta non si riapre: la sua fine resta un fatto. Se ne apre una nuova, con la
		// stessa fase, lo stesso responsabile e, per SCHEDA_COSTO, la stessa baseline di riferimento.
		if _, err := q.ApriFaseConBom(ctx, db.ApriFaseConBomParams{ThreadID: thread, NomeFase: bozza.FaseAllApertura,
			ResponsabileID: riga.ResponsabileID, Inizio: adesso, BomVersioneID: riga.BomVersioneID}); err != nil {
			return "", err
		}
	}
	n, err := q.EliminaBozza(ctx, bozza.BomVersioneID)
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", fmt.Errorf("V%d: l'abbandono non ha trovato la bozza", bozza.Numero)
	}
	return fmt.Sprintf("Revisione V%d abbandonata: la BOM resta la V%d.", bozza.Numero, bozza.NumeroPrecedente.Int32), nil
}

// elencoBreve dice le prime differenze, per un messaggio che resta leggibile.
func elencoBreve(diff []Differenza) string {
	const max = 5
	parti := make([]string, 0, max)
	for i, d := range diff {
		if i == max {
			parti = append(parti, fmt.Sprintf("e altre %d", len(diff)-max))
			break
		}
		parti = append(parti, d.String())
	}
	return fmt.Sprintf("%d differenze: %s", len(diff), strings.Join(parti, "; "))
}

func bloccaThread(ctx context.Context, q *db.Queries, thread uuid.UUID) error {
	_, err := q.BloccaThread(ctx, thread)
	if errors.Is(err, pgx.ErrNoRows) {
		return Rifiuto("RFQ non trovata")
	}
	return err
}

func faseAperta(ctx context.Context, q *db.Queries, thread uuid.UUID) (db.FaseLog, error) {
	f, err := q.GetFaseApertaPerThread(ctx, thread)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.FaseLog{}, Rifiuto("la RFQ non ha una fase aperta")
	}
	return f, err
}

func ultimaVersione(ctx context.Context, q *db.Queries, thread uuid.UUID) (db.BomVersione, bool, error) {
	v, err := q.GetUltimaVersione(ctx, thread)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.BomVersione{}, false, nil
	}
	return v, err == nil, err
}

func chiudiFase(ctx context.Context, q *db.Queries, thread uuid.UUID, fine time.Time, esito db.EsitoFase, nota string) error {
	n, err := q.ChiudiFaseAperta(ctx, db.ChiudiFaseApertaParams{ThreadID: thread, Fine: &fine,
		Esito: db.NullEsitoFase{EsitoFase: esito, Valid: true}, Note: pgtype.Text{String: nota, Valid: true}})
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("fase aperta non trovata (%d righe)", n)
	}
	return nil
}
