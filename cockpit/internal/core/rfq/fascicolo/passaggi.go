package fascicolo

// I PASSAGGI DI FASE A MANO (Richieste nuove, cockpit/_fasi/mockup_richieste.html).
//
// Il registro delle fasi (fase_log) si scriveva in due posti soli: alla nascita della RFQ (RICEVUTA) e con la
// BOM (congelamento, revisione, abbandono: versioni.go). I passaggi della tabella `transizione` che non
// dipendono dalla BOM non avevano un gesto, e una RFQ restava in RICEVUTA per sempre: anche la BOM non si
// poteva congelare, perche' si congela da FATTIBILITA.
//
// Qui il gesto c'e', con tre regole:
//   - si passa solo lungo un arco di `transizione` che parte dalla fase aperta. Anche quelli marcati automatici
//     di RICEVUTA e ATTESA_DISEGNI, finche' l'automatismo che li fa non esiste: sono una decisione che
//     qualcuno deve poter prendere;
//   - FATTIBILITA → SCHEDA_COSTO e' il congelamento della BOM, e si fa nella Distinta: un passaggio a mano
//     lascerebbe la scheda costo senza la sua baseline;
//   - con una revisione della BOM aperta non si passa: l'abbandono (D37) torna alla fase dell'apertura
//     guardando le righe nate dopo, e un passaggio a mano in mezzo cambierebbe la sua risposta.
// Entrare in SCHEDA_COSTO da un'altra fase (quotazioni rientrate, rinegoziazione) porta la baseline
// dell'ultima scheda costo: ck_fase_log_bom_solo_scheda e v_thread_da_riesaminare la leggono da li'.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/platform/db"
)

// automaticiAMano sono gli archi automatici che, finche' nessun automatismo li fa, si fanno a mano.
var automaticiAMano = map[db.Fase]bool{db.FaseRICEVUTA: true, db.FaseATTESADISEGNI: true}

// PassaggioAMano dice se l'arco da → a si fa con il gesto di questa pagina, e se no perche'.
func PassaggioAMano(t db.Transizione) (bool, string) {
	if t.Da == db.FaseFATTIBILITA && t.A == db.FaseSCHEDACOSTO {
		return false, "si passa congelando la BOM nella Distinta"
	}
	if t.Automatica && !automaticiAMano[t.Da] {
		return false, "è automatico"
	}
	return true, ""
}

// Terminale dice se una fase chiude la RFQ (fase_catalogo.terminale: PERSA, RESPINTA, SCADUTA).
func Terminale(f db.Fase) bool {
	return f == db.FasePERSA || f == db.FaseRESPINTA || f == db.FaseSCADUTA
}

// PassaFase chiude la fase aperta della RFQ e apre `a`, a nome di `utente`. Va chiamata dentro una
// transazione: la riga della RFQ si blocca (come per il congelamento) e due passaggi non si incrociano.
func PassaFase(ctx context.Context, q *db.Queries, thread uuid.UUID, a db.Fase, utente uuid.UUID) error {
	if _, err := q.BloccaThread(ctx, thread); err != nil {
		return err
	}
	aperta, err := faseAperta(ctx, q, thread)
	if err != nil {
		return err
	}
	archi, err := q.ListTransizioniDa(ctx, aperta.NomeFase)
	if err != nil {
		return err
	}
	var arco *db.Transizione
	for i := range archi {
		if archi[i].A == a {
			arco = &archi[i]
		}
	}
	if arco == nil {
		return Rifiuto(fmt.Sprintf("da %s non si passa a %s", aperta.NomeFase, a))
	}
	if ok, motivo := PassaggioAMano(*arco); !ok {
		return Rifiuto(fmt.Sprintf("da %s a %s %s", aperta.NomeFase, a, motivo))
	}
	if _, err := q.GetBozzaAperta(ctx, thread); err == nil {
		return Rifiuto("c'è una revisione della BOM aperta: prima si congela o si abbandona, nella Distinta")
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var bom uuid.NullUUID
	if a == db.FaseSCHEDACOSTO {
		v, err := q.GetUltimaCongelata(ctx, thread)
		if errors.Is(err, pgx.ErrNoRows) {
			return Rifiuto("la scheda costo vuole una BOM congelata: si congela nella Distinta")
		}
		if err != nil {
			return err
		}
		bom = uuid.NullUUID{UUID: v.BomVersioneID, Valid: true}
	}
	esito := db.EsitoFaseOK
	switch {
	case Terminale(a):
		esito = db.EsitoFaseKO
	case a == db.FaseATTESADISEGNI || (a == db.FaseSCHEDACOSTO && aperta.NomeFase == db.FaseOFFERTAINVIATA):
		esito = db.EsitoFaseRINVIATA
	}
	adesso := time.Now()
	if err := chiudiFase(ctx, q, thread, adesso, esito, "a mano: "+string(aperta.NomeFase)+" → "+string(a)); err != nil {
		return err
	}
	_, err = q.ApriFaseConBom(ctx, db.ApriFaseConBomParams{ThreadID: thread, NomeFase: a, Inizio: adesso,
		ResponsabileID: uuid.NullUUID{UUID: utente, Valid: true}, Note: pgtype.Text{String: arco.FattoRichiesto.String, Valid: arco.FattoRichiesto.Valid},
		BomVersioneID: bom})
	return err
}
