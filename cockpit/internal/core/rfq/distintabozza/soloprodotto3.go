package distintabozza

// La bozza manuale aperta e uno STEP confermato da S8 senza struttura importabile (direttiva 39 §3 [U + T-39]: niente
// seconda bozza attiva; scelta [T-39] della corsia persistenza): una forma del server, solo da S8 (passo 3), che la rotta
// di R2 non espone. La versione n+1 della stessa bozza e' la versione n con la sorgente reale dello STEP e il gruppo
// riservato solo_prodotto, senza registro: niente nodo esce o entra, nessuna casella, nessuna PB, decisione o arco
// cambia (non c'e' una struttura da confrontare). Eventi: sorgente_cambiata (origine server, prima null, dopo con la
// sorgente, l'algoritmo solo-prodotto-1 e la presa d'atto dello stato dell'analisi; il motivo), fonte_step_evento scelta
// con il gruppo riservato. Quando l'analisi dello STEP trova una struttura, S8 con la partenza step fa il cambio della
// fonte (cambio-fonte-2: la sorgente solo_prodotto vale come assenza).
//
// Solo per una bozza formato 3 senza sorgente: con una sorgente (STEP o solo_prodotto) la strada non e' questa (S8
// risponde 409 RICONCILIAZIONE_NON_DISPONIBILE solo_prodotto_su_bozza).

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

// SoloProdottoSuBozza3: la richiesta del server (commento di testa).
type SoloProdottoSuBozza3 struct {
	ThreadID, UtenteID uuid.UUID
	Comune             ComuneR2
	Sorgente           SorgenteDichiarata
	Motivo             string
	// PresaDAttoAnalisi: lo stato dell'analisi dello STEP di cui l'ingegnere ha preso atto (S8)
	PresaDAttoAnalisi string
	PrimaDelCommit    func(ctx context.Context, q *db.Queries, bozza uuid.UUID, versione int, imp *ImportazioneR2) error
}

// SoloProdottoSuBozzaFormato3: la versione n+1 della bozza manuale con la sorgente solo_prodotto (commento di testa).
// L'ordine: BloccaThread → la chiave → il prodotto → la bozza (aperta, alla versione attesa, formato 3, senza sorgente) →
// lo STEP (sorgenteSoloProdotto: della RFQ, scaricato, attivo, fonte confermata, motivo) → la versione e gli eventi.
func SoloProdottoSuBozzaFormato3(ctx context.Context, pool Avviatore, r SoloProdottoSuBozza3) (VersioneDelServer, error) {
	var out VersioneDelServer
	if !chiaveDelServer(r.Comune) {
		return out, corpo("chiave_salvataggio", "la chiave del salvataggio e' un UUID v4, con l'impronta della richiesta")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // dopo il Commit non fa niente
	q := db.New(tx)
	if _, err := q.BloccaThread(ctx, r.ThreadID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, &Errore{Codice: CodiceRFQInesistente, Messaggio: "la RFQ non c'e'"}
		}
		return out, err
	}
	if gia, trovata, err := chiaveGiaUsata(ctx, q, r.ThreadID, r.Comune); err != nil || trovata {
		if err != nil {
			return out, err
		}
		return riproponiVersioneDelServer(ctx, q, r.ThreadID, gia)
	}
	if _, err := prodottoDellaRfq(ctx, q, r.ThreadID, r.Comune.ProdottoID, true); err != nil {
		return out, err
	}
	b, prima, _, err := bozzaFormato3Bloccata(ctx, q, r.ThreadID, r.Comune)
	if err != nil {
		return out, err
	}
	if prima.Sorgente != nil {
		return out, corpoMotivo("sorgente", MotivoPartenzaNonAmmessa, "la bozza ha gia' una sorgente: il passaggio a uno STEP senza struttura non e' questa forma")
	}
	src, err := sorgenteSoloProdotto(ctx, q, r.ThreadID, r.Comune.ProdottoID, r.Sorgente, r.Motivo, "sorgente")
	if err != nil {
		return out, err
	}
	s := prima.copia()
	s.Sorgente = &src
	canon, err := s.Canonico()
	if err != nil {
		return out, erroreDellaVersione(err, "sorgente")
	}
	dopo := map[string]any{"sorgente": s.Sorgente, "algoritmo": AlgoritmoSoloProdotto,
		"conteggi": map[string]int{"nodi": len(s.Nodi), "archi": len(s.Archi)}}
	if r.PresaDAttoAnalisi != "" {
		dopo["presa_d_atto_analisi"] = r.PresaDAttoAnalisi
	}
	finale := eventoServer{Azione: AzSorgenteCambiata, Dopo: canonico(dopo), Motivo: r.Motivo}
	salvato, err := scriviVersioneDelServer(ctx, q, r.ThreadID, r.UtenteID, b, r.Comune, canon, s, uuid.Nil, nil, finale)
	if err != nil {
		return out, err
	}
	if _, err := q.InsertFonteStepEvento(ctx, db.InsertFonteStepEventoParams{ThreadID: r.ThreadID, ProdottoID: r.Comune.ProdottoID, Ruolo: "prodotto",
		Azione: "scelta", AllegatoID: *src.AllegatoID, Sha256: src.Sha256, Gruppo: testo(GruppoSoloProdotto),
		BozzaID: uuid.NullUUID{UUID: b.BozzaID, Valid: true}, Motivo: testo(r.Motivo), AutoreID: r.UtenteID}); err != nil {
		return out, erroreDiScrittura(err, "sorgente")
	}
	imp := ImportazioneR2{}
	if r.PrimaDelCommit != nil {
		if err := r.PrimaDelCommit(ctx, q, salvato.BozzaID, salvato.Versione, &imp); err != nil {
			return out, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return out, erroreDelCommit(err, "sorgente")
	}
	return VersioneDelServer{Salvato3: salvato, Snapshot: canon}, nil
}
