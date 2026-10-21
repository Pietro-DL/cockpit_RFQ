package web

// lucchetti_legacy.go: l'ordine dei lucchetti dei gesti delle pagine legacy rispetto a R5 formato 3 (direttiva 39 §7
// [U + T-39]; piano della direttiva 35 INC-G25 [T]; C-6b, corsia file).
//
// R5 prende per prima la riga della RFQ FOR UPDATE (BloccaThread), poi la bozza, i documenti dello STEP per documento_id
// crescente, il componente del prodotto, le proposte di documento, di componente e di relazione (README di
// core/rfq/distintabozza). Un gesto che blocca prima una di quelle righe e chiede la RFQ dopo (con BloccaThread, con la
// chiave esterna di una riga nuova verso thread_offerta, o con il trigger della working bloccata, che la prende FOR KEY
// SHARE) puo' chiudere un ciclo con R5: 40P01. Percio' ogni transazione di un gesto legacy prende la RFQ prima di ogni
// altra riga [T-39]:
//   - FOR UPDATE nei gesti che passano da gesto/gestoPoi/gestoConDomanda/gestoConEsito, nella preparazione dei file
//     (preparaFile), in «Assegna» e nella correzione del codice (preparaGesto) e nella conferma del Fascicolo: e' il modo
//     piu' forte, quindi la transazione non lo deve mai alzare dopo (un FOR KEY SHARE alzato a FOR UPDATE mentre R5 aspetta
//     la riga e' a sua volta un ciclo);
//   - FOR KEY SHARE solo dove la transazione non lo alza mai e vuole lasciar correre le altre conferme: il vecchio
//     «Scarta il file» (allegati.go) e la conferma della proposta (allegati.go, corsia persistenza). FOR KEY SHARE
//     aspetta chi tiene la RFQ FOR UPDATE, quindi anche R5.
// Dopo la RFQ, chi tocca i documenti di uno STEP li blocca per documento_id crescente (bloccaDocumenti di
// core/rfq/fascicolo, AnnullaSostituzione compresa), poi le proposte; il servizio unico delle AR non prende lucchetti.
// I caricamenti (E1′ e il ramo HTML di POST /thread/{id}/carica, il caricamento del Fascicolo, l'importazione dal NAS)
// prendono anche loro la RFQ FOR UPDATE per prima, dentro fascicolo.NotaPortale / NotaInterna, poi il messaggio della
// nota, il contenuto e le righe nuove (giro G39-bis, corsia file). [superato: fino al giro G39 «restavano come sono»,
// messaggio della nota → RFQ: con l'estrazione di un archivio della stessa nota, che tiene la RFQ FOR UPDATE e poi
// riscrive il messaggio con il trigger aggiorna_n_allegati, era un ciclo (40P01 nella prova d1, F2 «+ Aggiungi file»).
// Il messaggio della nota non e' una riga dell'ingresso della posta, quindi l'ordine RFQ → nota non rovescia niente.]
// I risultati del worker seguono lo stesso ordine (transport/workerapi/lucchetti.go). Il ritento su 40P01 / 40001 c'e'
// solo in E1′ (filerfq.RegistraFile, il ramo JSON) e nel result del worker; il ramo HTML, il caricamento del Fascicolo
// e l'importazione dal NAS hanno l'ordine dei lucchetti, senza ritento.

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/platform/db"
)

// bloccaRfqDelGesto prende la RFQ FOR UPDATE come primo lucchetto della transazione del gesto (commento di testa). Una
// RFQ che non c'e' non e' un errore qui: non c'e' niente da mettere in fila, e il gesto risponde con il suo rifiuto di
// sempre («RFQ non trovata», «il file non è di questa RFQ», …).
func bloccaRfqDelGesto(ctx context.Context, q *db.Queries, thread uuid.UUID) error {
	if _, err := q.BloccaThread(ctx, thread); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return nil
}
