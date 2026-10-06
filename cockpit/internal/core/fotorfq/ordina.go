package fotorfq

// ordina.go: l'ordine totale degli elenchi della fotografia (piano A, A1c, 6.4.1). Diverse query riusate non hanno
// ORDER BY (le proposte, le provenienze), e quelle che lo hanno ordinano con la collazione del database: qui si
// ordina in Go, per chiavi stabili e con il confronto dei byte, così due letture della stessa copia danno gli
// stessi elenchi qualunque sia il database o la locale.

import (
	"bytes"
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// Ordina mette ogni elenco in un ordine totale per chiave stabile:
//   - messaggi per (data, ID);
//   - allegati per (data del messaggio, messaggio, contenitore prima, indice, ID), come ListAllegatiFascicolo;
//   - il resto per chiave primaria: clienti, utenti, thread, proposte, documenti (con i loro allegati), componenti,
//     deroghe, righe proposte per ID; identificativi per codice; relazioni per (padre, figlio); righe di arco per
//     (allegato, padre, figlio); rimozioni per (documento, padre, figlio); righe delle viste per componente (e tipo
//     di documento); fabbisogni per (tipo di componente, tipo di documento); triage e candidati per messaggio;
//     agganci e allegati in attesa per ID; i messaggi fuori RFQ per ID; le diagnostiche per codice e percorso.
func (f *Fotografia) Ordina() {
	slices.SortStableFunc(f.Clienti, func(a, b Cliente) int { return cmpUUID(a.ID, b.ID) })
	slices.SortStableFunc(f.Utenti, func(a, b Utente) int { return cmpUUID(a.ID, b.ID) })
	slices.SortStableFunc(f.Thread, func(a, b Thread) int { return cmpUUID(a.ID, b.ID) })
	for i := range f.Thread {
		f.Thread[i].ordina()
	}
	slices.SortStableFunc(f.FuoriRFQ, func(a, b MessaggioFuoriRFQ) int { return cmpUUID(a.Messaggio.ID, b.Messaggio.ID) })
	for i := range f.FuoriRFQ {
		m := &f.FuoriRFQ[i]
		ordinaAllegati(m.Allegati, map[uuid.UUID]time.Time{m.Messaggio.ID: m.Messaggio.DataEvento})
	}
	ordinaDiagnostiche(f.Diagnostiche)
}

func (t *Thread) ordina() {
	slices.SortStableFunc(t.Messaggi, func(a, b Messaggio) int {
		return cmp.Or(a.DataEvento.Compare(b.DataEvento), cmpUUID(a.ID, b.ID))
	})
	slices.SortStableFunc(t.Agganci, func(a, b AggancioMessaggio) int { return cmpUUID(a.MessaggioID, b.MessaggioID) })
	date := make(map[uuid.UUID]time.Time, len(t.Messaggi))
	for _, m := range t.Messaggi {
		date[m.ID] = m.DataEvento
	}
	ordinaAllegati(t.Allegati, date)
	slices.SortStableFunc(t.Proposte, func(a, b PropostaAttuale) int { return cmpUUID(a.ID, b.ID) })
	slices.SortStableFunc(t.Documenti, func(a, b DocumentoConfermato) int { return cmpUUID(a.ID, b.ID) })
	for i := range t.Documenti {
		slices.SortStableFunc(t.Documenti[i].Allegati, cmpUUID)
	}
	slices.SortStableFunc(t.Identificativi, func(a, b Identificativo) int { return strings.Compare(a.Codice, b.Codice) })
	slices.SortStableFunc(t.Componenti, func(a, b Componente) int { return cmpUUID(a.ID, b.ID) })
	slices.SortStableFunc(t.Relazioni, func(a, b Relazione) int {
		return cmp.Or(cmpUUID(a.PadreID, b.PadreID), cmpUUID(a.FiglioID, b.FiglioID))
	})
	slices.SortStableFunc(t.RigheComponenteProposta, func(a, b RigaComponenteProposta) int { return cmpUUID(a.ID, b.ID) })
	slices.SortStableFunc(t.RigheRelazioneProposta, func(a, b RigaRelazioneProposta) int {
		return cmp.Or(cmpUUID(a.AllegatoID, b.AllegatoID), strings.Compare(a.PadreChiave, b.PadreChiave),
			strings.Compare(a.FiglioChiave, b.FiglioChiave))
	})
	slices.SortStableFunc(t.RimozioniAperte, func(a, b RimozioneAperta) int {
		return cmp.Or(cmpUUID(a.StepDocumentoID, b.StepDocumentoID), cmpUUID(a.PadreID, b.PadreID), cmpUUID(a.FiglioID, b.FiglioID))
	})
	slices.SortStableFunc(t.StepProdotto, func(a, b RigaStepProdotto) int {
		return cmp.Or(cmpUUID(a.ComponenteID, b.ComponenteID), strings.Compare(a.Codice, b.Codice))
	})
	slices.SortStableFunc(t.Fascicolo, func(a, b RigaFascicolo) int {
		return cmp.Or(cmpUUID(a.ComponenteID, b.ComponenteID), strings.Compare(a.TipoDocumento, b.TipoDocumento))
	})
	slices.SortStableFunc(t.Fabbisogni, func(a, b RigaFabbisogno) int {
		return cmp.Or(strings.Compare(a.TipoComponente, b.TipoComponente), strings.Compare(a.TipoDocumento, b.TipoDocumento))
	})
	slices.SortStableFunc(t.Deroghe, func(a, b DerogaFabbisogno) int { return cmpUUID(a.ID, b.ID) })
	slices.SortStableFunc(t.Triage, func(a, b Triage) int {
		return cmp.Or(cmpUUID(a.MessaggioID, b.MessaggioID), cmpUUID(a.ID, b.ID))
	})
	slices.SortStableFunc(t.CandidatiCodice, func(a, b CandidatoCodice) int {
		return cmp.Or(cmpUUID(a.MessaggioID, b.MessaggioID), strings.Compare(a.Codice, b.Codice))
	})
	slices.SortStableFunc(t.InAttesa, cmpUUID)
}

// ordinaAllegati: (data del messaggio, messaggio, contenitore prima, indice, ID). Le voci di un contenitore
// vengono dopo i file del messaggio, ordinate per contenitore.
func ordinaAllegati(allegati []Allegato, date map[uuid.UUID]time.Time) {
	slices.SortStableFunc(allegati, func(a, b Allegato) int {
		return cmp.Or(
			date[a.MessaggioID].Compare(date[b.MessaggioID]),
			cmpUUID(a.MessaggioID, b.MessaggioID),
			cmpUUIDPrimaNil(a.ContenitoreID, b.ContenitoreID),
			cmp.Compare(a.Indice, b.Indice),
			cmpUUID(a.ID, b.ID),
		)
	})
}

func ordinaDiagnostiche(d []evidenze.Diagnostica) {
	slices.SortStableFunc(d, func(a, b evidenze.Diagnostica) int {
		return cmp.Or(
			strings.Compare(a.Codice, b.Codice),
			strings.Compare(a.Percorso, b.Percorso),
			strings.Compare(a.Messaggio, b.Messaggio),
			strings.Compare(strings.Join(a.Rif, "\x00"), strings.Join(b.Rif, "\x00")),
			strings.Compare(string(a.Gravita), string(b.Gravita)),
			strings.Compare(string(a.Natura), string(b.Natura)),
		)
	})
}

// cmpUUID confronta i byte: è anche l'ordine del testo canonico (esadecimale minuscolo).
func cmpUUID(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) }

// cmpUUIDPrimaNil: nil prima di ogni valore, poi per byte.
func cmpUUIDPrimaNil(a, b *uuid.UUID) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	return cmpUUID(*a, *b)
}
