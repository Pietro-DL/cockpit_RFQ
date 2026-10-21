package associazioni

// Il legame dello STEP sorgente → prodotto (S7, passo 1 di S8, «Usa come STEP»), distinto dalle AR (direttiva 35 §3 [U]:
// «distinguere il legame della sorgente dalle AR di R5»). PROVVISORIO, base [T] D-AR02 A (piano §4), e SOSPESO
// (INC-S1): LegaSorgente e' scritta qui perche' il servizio unico resti l'unico scrittore della copia, ma NON e' cablata
// in nessuna rotta, e S7, il passo 1 di S8 e «Usa come STEP» non la chiamano finche' l'utente non risponde a D-AR02.
//
// Che cosa scrive, nella transazione del chiamante:
//   - fonte_step_evento, ruolo prodotto, azione caricamento: la fonte autorevole proposta per il legame;
//   - associazione_evento «assegna» sulla proposta, senza riga di file_rfq_associazione (il legame non e' una AR). La
//     variante corrente della 0023 non ha l'origine sorgente_step (sospesa, INC-S1): l'evento usa l'origine ponte, quella
//     che il contratto di S7 gia' dice (EM §10.9);
//   - la copia della proposta aperta sul prodotto (per una proposta la copia e' la AR attiva oppure il legame, mai tutte e
//     due: con un legame il punto 5 di trg_fra_nasce rifiuta una AR su un altro componente).

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

// LegameSorgente: lo STEP (allegato della RFQ) e il prodotto finito di cui e' la sorgente.
type LegameSorgente struct {
	Allegato uuid.UUID
	Prodotto uuid.UUID
}

// EsitoSorgente e' il risultato di LegaSorgente.
type EsitoSorgente struct {
	Ripetuto     bool
	FonteEvento  int64
	EventoID     int64
	Copia        uuid.NullUUID
	ProdottoTipo db.TipoComponente
}

// LegaSorgente lega lo STEP al prodotto finito (provvisorio, base [T] D-AR02 A; NON cablato: INC-S1). Un allegato con una
// AR attiva, o con la copia su un altro componente, non si lega (gia_associata_altrove). La stessa chiave non scrive due
// volte (Ripetuto).
func LegaSorgente(ctx context.Context, q *db.Queries, a Atto, l LegameSorgente) (EsitoSorgente, error) {
	if err := a.valida(); err != nil {
		return EsitoSorgente{}, err
	}
	if a.Conferma.Valid {
		return EsitoSorgente{}, rifiuto(CodiceAttoNonValido, "il legame della sorgente non e' una conferma della BOM")
	}
	if l.Allegato == uuid.Nil || l.Prodotto == uuid.Nil {
		return EsitoSorgente{}, rifiuto(CodiceAttoNonValido, "mancano l'allegato o il prodotto")
	}
	al, err := q.GetAllegatoPerSorgente(ctx, db.GetAllegatoPerSorgenteParams{AllegatoID: l.Allegato, ThreadID: nuid(a.Thread)})
	if errors.Is(err, pgx.ErrNoRows) {
		return EsitoSorgente{}, rifiuto(CodiceAttoNonValido, "l'allegato %s non e' della RFQ %s", l.Allegato, a.Thread)
	}
	if err != nil {
		return EsitoSorgente{}, err
	}
	if !al.Sha256.Valid {
		return EsitoSorgente{}, rifiuto(CodiceAttoNonValido, "l'allegato %s non e' ancora scaricato (manca il contenuto)", l.Allegato)
	}
	pr, err := q.GetComponenteDellaRfqPerAR(ctx, db.GetComponenteDellaRfqPerARParams{ComponenteID: l.Prodotto, ThreadID: a.Thread})
	if errors.Is(err, pgx.ErrNoRows) {
		return EsitoSorgente{}, rifiuto(CodiceComponenteNonDellaRfq, "il prodotto %s non e' della RFQ %s", l.Prodotto, a.Thread)
	}
	if err != nil {
		return EsitoSorgente{}, err
	}
	if pr.Tipo != db.TipoComponenteFinito {
		return EsitoSorgente{}, rifiuto(CodiceAttoNonValido, "%s non e' un prodotto finito: lo STEP sorgente e' di un finito", pr.Codice)
	}
	if pr.ArchiviatoIl != nil {
		return EsitoSorgente{}, rifiuto(CodiceComponenteArchiviato, "il prodotto %s e' archiviato", pr.Codice)
	}
	ee, err := q.ListEventiAssociazionePerChiave(ctx, db.ListEventiAssociazionePerChiaveParams{ThreadID: a.Thread, ChiaveIdempotenza: nuid(a.Chiave),
		AllegatoID: nuid(l.Allegato)})
	if err != nil {
		return EsitoSorgente{}, err
	}
	for _, e := range ee {
		if e.Azione == AzioneAssegna && !e.AssociazioneID.Valid && e.ComponenteDopo.UUID == l.Prodotto {
			return EsitoSorgente{Ripetuto: true, EventoID: e.EventoID, Copia: e.PropostaID, ProdottoTipo: pr.Tipo}, nil
		}
	}
	att, err := Attiva(ctx, q, a.Thread, l.Allegato)
	if err != nil {
		return EsitoSorgente{}, err
	}
	if att != nil {
		return EsitoSorgente{}, altrove(l.Allegato, att.Componente, "una AR attiva: un file e' AR o sorgente, mai tutte e due")
	}
	prop, err := propostaDi(ctx, q, l.Allegato)
	if err != nil {
		return EsitoSorgente{}, err
	}
	if prop != nil && prop.Stato == db.StatoPropostaAperta && prop.ComponenteID.Valid && prop.ComponenteID.UUID != l.Prodotto {
		return EsitoSorgente{}, altrove(l.Allegato, prop.ComponenteID.UUID, "la copia della proposta")
	}
	out := EsitoSorgente{ProdottoTipo: pr.Tipo}
	fp := db.InsertFonteStepCaricamentoParams{ThreadID: a.Thread, ProdottoID: l.Prodotto, AllegatoID: l.Allegato, Sha256: al.Sha256.String,
		Motivo: testo(a.Nota), ChiaveIdempotenza: nuid(a.Chiave), AutoreID: a.Autore}
	ev := db.InsertEventoAssociazioneParams{ThreadID: a.Thread, AllegatoID: nuid(l.Allegato), Azione: AzioneAssegna, ComponenteDopo: nuid(l.Prodotto),
		Motivo: testo(a.Nota), Origine: OrigineEventoPonte, ChiaveIdempotenza: nuid(a.Chiave), AutoreID: a.Autore}
	if prop != nil {
		fp.PropostaID, ev.PropostaID = nuid(prop.PropostaID), nuid(prop.PropostaID)
		if prop.ComponenteID.Valid {
			ev.ComponentePrima = prop.ComponenteID
		}
	}
	if out.FonteEvento, err = q.InsertFonteStepCaricamento(ctx, fp); err != nil {
		return EsitoSorgente{}, traduci(err)
	}
	if out.EventoID, err = q.InsertEventoAssociazione(ctx, ev); err != nil {
		return EsitoSorgente{}, traduci(err)
	}
	ids, err := q.ScriviCopiaAssociazione(ctx, db.ScriviCopiaAssociazioneParams{AllegatoID: l.Allegato, ComponenteID: l.Prodotto, ThreadID: a.Thread})
	if err != nil {
		return EsitoSorgente{}, traduci(err)
	}
	if len(ids) > 0 {
		out.Copia = nuid(ids[0])
	}
	return out, nil
}
