package valutazione

import (
	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// regole_del_thread.go (V1, piano della 0023 §14 passo 6; emendamento di V1 §3.8 «regole», §3.8.2; RM-13): con quali
// regole la valutazione legge i documenti di un thread, per il costruttore del registro d'importazione. Il registro deve
// dire con quale indice, quale grammatica del cliente, quali partner e quale contesto è stata fatta la proposta, e quali
// file il contesto ha letto: l'esito non li porta tutti (il contesto risolto non è nell'esito), e l'adattatore non li deve
// ricalcolare da sé. Qui si ricalcolano con le stesse funzioni di Calcola (motoreDel, contestoDelThread, testataRegole):
// una regola sola, nessuna copia.

// RegoleDelThread: le regole con cui Calcola legge un thread.
//   - ImprontaIndice, VersioneLimiti: come Esito.ImprontaIndice ed Esito.VersioneLimiti (l'impronta del canonico
//     dell'indice, che copre anche i limiti, e la versione dei limiti); vuote senza insieme di regole.
//   - ClienteID: il cliente del thread. HashGrammatica: lo snapshot della grammatica del cliente, come
//     EsitoThread.HashSnapshot; vuoto senza motore (un thread non valutato per la grammatica).
//   - Contesto: il contesto di lavorazione che la valutazione applica ai documenti del thread, risolto come in Calcola;
//     nil senza contesto, senza motore o con un contesto che non si risolve (l'errore sta fra le diagnostiche, come
//     nell'esito). Con Contesto.Attivo() il contesto dà i partner attivi (PartnerAttivi), la copia canonica con
//     l'ereditato (CopiaCanonica) e il perimetro per occorrenza (NelPerimetro); il suo ID e la sua Impronta sono quelli
//     che le interpretazioni dei file del perimetro portano (ContestoID, ImprontaContesto).
//   - Diagnostiche: quelle della grammatica e del contesto, le stesse che l'esito del thread porta per loro, in ordine di
//     (codice, percorso, riferimenti).
type RegoleDelThread struct {
	ImprontaIndice string
	VersioneLimiti string
	ClienteID      uuid.UUID
	HashGrammatica string
	Contesto       *motorea.ContestoRisolto
	Diagnostiche   []evidenze.Diagnostica
}

// RegoleDiLettura: le regole con cui Calcola legge il thread della fotografia, con le stesse regole e la stessa
// fotografia che Calcola ha già accettato. Falso se il thread non c'è. È pura e deterministica, come Calcola: niente DB,
// file, orologio; gli ingressi di chi chiama non cambiano.
func RegoleDiLettura(f fotorfq.Fotografia, r *motorea.InsiemeRegole, thread uuid.UUID) (RegoleDelThread, bool) {
	var t *fotorfq.Thread
	for i := range f.Thread {
		if f.Thread[i].ID == thread {
			t = &f.Thread[i]
			break
		}
	}
	if t == nil {
		return RegoleDelThread{}, false
	}
	out := RegoleDelThread{ClienteID: t.ClienteID}
	out.ImprontaIndice, out.VersioneLimiti = testataRegole(r)
	m, _, diag := motoreDel(f, r, t.ClienteID)
	if m != nil {
		out.HashGrammatica = m.Snapshot().Hash
		c, d := contestoDelThread(r, m, t.ID)
		out.Contesto = c
		diag = append(diag, d...)
	}
	ordinaDiagnostiche(diag)
	out.Diagnostiche = diag
	return out, true
}

// NelPerimetro: il file del thread sta nel perimetro del contesto applicato, cioè il motore lo legge con il contesto:
// lo stesso criterio dell'attivazione (motorea.ContestoRisolto.NelPerimetro) sul documento del file. Falso senza
// contesto, con un contesto non attivo e per un file senza documento (un contenitore, un file non letto: Motivo), che il
// motore non legge affatto.
func (r RegoleDelThread) NelPerimetro(f FileInterpretato) bool {
	return r.Contesto.NelPerimetro(f.Documento)
}
