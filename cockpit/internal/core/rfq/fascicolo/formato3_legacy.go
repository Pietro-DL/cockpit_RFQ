package fascicolo

// I gesti legacy che scrivono componente.step_strutturale_id in una RFQ formato 3 (direttiva 39 §2 [U + T-39]: «R5 e'
// l'unico scrittore di componente.step_strutturale_id nel percorso formato 3, tanto in prima istanza quanto nelle
// revisioni»; D-TAB-02 B rivista [T-39]; compito C-6b: «nelle RFQ formato 3 "Autorizza" non scrive
// step_strutturale_id: rifiuta e rimanda a S8; nelle RFQ legacy invariato»).
//
// Che cosa si ferma, nelle RFQ formato 3 (associazioni.RFQFormato3, l'unica definizione: almeno una bozza o una conferma
// formato 3 nella storia della RFQ):
//   - «Autorizza come STEP» di un prodotto finito (DichiaraStrutturale, anche quando la chiama la sostituzione di un
//     documento con «il nuovo prende il suo posto»): la materializzazione dello STEP strutturale;
//   - il cambio di tipo che riporta un componente a prodotto finito e rimetterebbe lo STEP strutturale sospeso
//     (cambiaTipo con stepDoc) [T-39 corsia file: e' la stessa scrittura di «Autorizza»].
//
//   - la «Revoca» legacy dello STEP strutturale di un prodotto finito (RevocaStrutturale, e per prudenza anche lo
//     svuotamento in revocaDichiarazione) e il cambio di tipo che svuoterebbe lo STEP strutturale di un finito (cambiaTipo
//     con SvuotaStep): sono scritture di step_strutturale_id fuori da R5. La revoca della direttiva 39 §2 («Revoca:
//     conserva il gesto esplicito per correggere uno STEP scelto per errore ... la revoca non muta silenziosamente la
//     working») e' «Revoca come sorgente» della Sorgente, che non tocca la working; lo STEP della working cambia con S8
//     e una nuova R5 [T-39 corsia file, rilievi della revisione: prima la revoca legacy restava];
//   - «Sostituisci» e «Annulla sostituzione» dei documenti quando uno dei due e' uno STEP del prodotto finito
//     (DocumentoStepDelFinito: lo STEP strutturale della working, la sorgente di una bozza aperta, lo STEP legato come
//     sorgente secondo allegato_sorgente_del_finito): §2, A resta la working fino a R5 e la sostituzione di A con B la
//     scrive R5; §5 [U], «Cambiarlo = S8, non un normale gesto documentale». Il blocco sta nel core (Sostituisci,
//     AnnullaSostituzione), quindi copre la rotta, la conferma legacy e «Assegna» con «sostituisce» (ApplicaScelta);
//   - «Assegna» di un documento confermato verso un altro componente (o senza componente) quando il documento e' uno STEP
//     del prodotto finito o quando un allegato con il suo contenuto ha una AR attiva su un componente diverso dalla
//     destinazione (GuardiaSpostamentoDocumento: la guardia di INC-G9 e D-AR07 [T-39], niente divergenza documento-AR).
//
// Che cosa resta: le autorizzazioni e le revoche dei componenti che non sono finiti e le deleghe, che non scrivono
// step_strutturale_id; le sostituzioni e gli spostamenti dei documenti che non sono STEP del finito e non hanno una AR
// altrove. Nelle RFQ interamente legacy niente cambia.

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/platform/db"
)

// TestoStepNelFormato3: il rimando a S8 dei gesti legacy che scriverebbero lo STEP strutturale in una RFQ formato 3.
const TestoStepNelFormato3 = "la RFQ è gestita con la Distinta nuova (formato 3): lo STEP strutturale del prodotto non si autorizza da qui. " +
	"Si conferma come sorgente dalla Sorgente («Conferma la sorgente», S8) e diventa lo STEP strutturale con la conferma della BOM. Niente è cambiato"

// rifiutoStepNelFormato3: il rifiuto se la RFQ e' formato 3, altrimenti nil. Chi chiama tiene la RFQ.
func rifiutoStepNelFormato3(ctx context.Context, q *db.Queries, thread uuid.UUID) error {
	f3, err := associazioni.RFQFormato3(ctx, q, thread)
	if err != nil {
		return err
	}
	if f3 {
		return Rifiuto(TestoStepNelFormato3)
	}
	return nil
}

// I rimandi dei gesti documentali e della revoca legacy nelle RFQ formato 3 (commento di testa).
const (
	// TestoSostituzioneNelFormato3 (con il nome del file): «Sostituisci» su uno STEP del prodotto finito.
	TestoSostituzioneNelFormato3 = "la RFQ è gestita con la Distinta nuova (formato 3): %s è uno STEP del prodotto finito " +
		"(lo STEP strutturale, la sorgente confermata o la sorgente di una bozza) e non si sostituisce da qui. " +
		"Il file nuovo si conferma come sorgente dalla Sorgente («Conferma la sorgente», S8) e la sostituzione la scrive la conferma della BOM. Niente è cambiato"
	// TestoAnnullaNelFormato3 (con il nome del file): «Annulla sostituzione» che tocca uno STEP del prodotto finito.
	TestoAnnullaNelFormato3 = "la RFQ è gestita con la Distinta nuova (formato 3): la sostituzione tocca %s, uno STEP del prodotto finito, " +
		"e non si annulla da qui: la sostituzione dello STEP la scrive la conferma della BOM. Uno STEP scelto per errore si corregge dalla Sorgente " +
		"(«Revoca come sorgente», poi «Conferma la sorgente» con il file giusto) e con una nuova conferma della BOM. Niente è cambiato"
	// TestoRevocaNelFormato3 (con il codice del prodotto): la «Revoca» legacy dello STEP strutturale di un finito.
	TestoRevocaNelFormato3 = "la RFQ è gestita con la Distinta nuova (formato 3): lo STEP strutturale di %s non si revoca da qui. " +
		"Uno STEP scelto per errore si corregge dalla Sorgente («Revoca come sorgente», poi «Conferma la sorgente» con il file giusto) " +
		"e cambia con la conferma della BOM. Niente è cambiato"
	// TestoTipoNelFormato3 (con il codice del prodotto): il cambio di tipo che svuoterebbe lo STEP strutturale di un finito.
	TestoTipoNelFormato3 = "la RFQ è gestita con la Distinta nuova (formato 3): %s ha lo STEP strutturale, che cambia solo con la conferma della BOM, " +
		"e il cambio di tipo lo svuoterebbe: non si fa da qui. Niente è cambiato"
	// TestoSpostaStepNelFormato3 (con il nome del file): «Assegna» di uno STEP del prodotto finito.
	TestoSpostaStepNelFormato3 = "la RFQ è gestita con la Distinta nuova (formato 3): %s è uno STEP del prodotto finito " +
		"(lo STEP strutturale, la sorgente confermata o la sorgente di una bozza) e non si sposta con «Assegna»: la sorgente si cambia dalla Sorgente (S8). Niente è cambiato"
	// TestoSpostaARNelFormato3 (con il nome del file e il codice del componente della AR): «Assegna» di un documento via
	// dal componente della sua AR.
	TestoSpostaARNelFormato3 = "la RFQ è gestita con la Distinta nuova (formato 3): %s ha un'associazione registrata con %s e il documento " +
		"non lascia quel componente da qui: per un altro componente si cambia la preassociazione nella bozza e si conferma la BOM. Niente è cambiato"
)

// rifiutoTestoNelFormato3: il rifiuto con il testo se la RFQ e' formato 3, altrimenti nil. Chi chiama tiene la RFQ.
func rifiutoTestoNelFormato3(ctx context.Context, q *db.Queries, thread uuid.UUID, testo string) error {
	f3, err := associazioni.RFQFormato3(ctx, q, thread)
	if err != nil {
		return err
	}
	if f3 {
		return Rifiuto(testo)
	}
	return nil
}

// stepDelFinitoFra: il primo dei documenti che e' uno STEP del prodotto finito (DocumentoStepDelFinito), o nil.
func stepDelFinitoFra(ctx context.Context, q *db.Queries, thread uuid.UUID, docs ...db.Documento) (*db.Documento, error) {
	for i := range docs {
		si, err := q.DocumentoStepDelFinito(ctx, db.DocumentoStepDelFinitoParams{DocumentoID: docs[i].DocumentoID, ThreadID: thread})
		if err != nil {
			return nil, err
		}
		if si {
			return &docs[i], nil
		}
	}
	return nil, nil
}

// rifiutoDocumentiNelFormato3: nelle RFQ formato 3, il rifiuto (testo con il nome del file) se uno dei documenti e' uno
// STEP del prodotto finito; altrimenti nil. Chi chiama tiene la RFQ e i documenti.
func rifiutoDocumentiNelFormato3(ctx context.Context, q *db.Queries, thread uuid.UUID, testo string, docs ...db.Documento) error {
	f3, err := associazioni.RFQFormato3(ctx, q, thread)
	if err != nil || !f3 {
		return err
	}
	d, err := stepDelFinitoFra(ctx, q, thread, docs...)
	if err != nil {
		return err
	}
	if d != nil {
		return Rifiuto(fmt.Sprintf(testo, d.NomeFile))
	}
	return nil
}

// GuardiaSpostamentoDocumento: nelle RFQ formato 3, «Assegna» di un documento confermato che cambia componente (dest non
// valido = senza componente) si rifiuta se il documento e' uno STEP del prodotto finito o se un allegato con il suo
// contenuto ha una AR attiva su un componente diverso da dest (commento di testa). Nelle RFQ interamente legacy nil. Chi
// chiama tiene la RFQ e il documento, e la chiama solo quando il componente cambia.
func GuardiaSpostamentoDocumento(ctx context.Context, q *db.Queries, thread uuid.UUID, d db.Documento, dest uuid.NullUUID) error {
	f3, err := associazioni.RFQFormato3(ctx, q, thread)
	if err != nil || !f3 {
		return err
	}
	if x, err := stepDelFinitoFra(ctx, q, thread, d); err != nil {
		return err
	} else if x != nil {
		return Rifiuto(fmt.Sprintf(TestoSpostaStepNelFormato3, d.NomeFile))
	}
	ar, err := q.ListAssociazioniAttiveDelDocumento(ctx, db.ListAssociazioniAttiveDelDocumentoParams{DocumentoID: d.DocumentoID, ThreadID: thread})
	if err != nil {
		return err
	}
	for _, a := range ar {
		if !dest.Valid || a.ComponenteID != dest.UUID {
			return Rifiuto(fmt.Sprintf(TestoSpostaARNelFormato3, d.NomeFile, a.Codice))
		}
	}
	return nil
}
