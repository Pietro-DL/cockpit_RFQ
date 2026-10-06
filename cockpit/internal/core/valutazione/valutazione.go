// Package valutazione è il percorso puro comune del banco e dell'anteprima (piano A, par.3.3.8, 6.4.6; R42 B): dalla
// fotografia già chiusa produce il «nuovo» del motore A e, per ogni prodotto target della RFQ, i suoi assi (R79).
// Non confronta e non vede mai l'atteso: non importa confronto né una libreria YAML (R2). È puro: nessun DB, file,
// orologio o rete; la fotografia la legge il caricatore, prima e altrove.
//
// In B1 (commit P7a) ci sono gli ingressi del file dei casi (LeggiIngressi), la richiesta del thread
// (RichiestaDelThread), i prodotti target con l'identità di R70 A e R75 A (TargetConfermato) e la fonte
// strutturale di R65 A e R76 b A (FonteProdotto), con il punto d'ingresso ValutaProdotti. Gli altri assi, lo stato
// del prodotto, il fascicolo, l'impronta e Calcola arrivano con B5 e B6.
package valutazione

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// ValutazioneProdotti: la parte di B1 della valutazione di un thread. La richiesta (il gesto 1 per messaggio), i
// prodotti candidati della mail (sempre da confermare: R60 A), i prodotti target con l'identità e la fonte, le
// diagnostiche in ordine di (codice, percorso, riferimenti). B6 la porta dentro l'esito del thread.
type ValutazioneProdotti struct {
	ThreadID         uuid.UUID                    `json:"thread_id"`
	Richiesta        ancoraggio.RichiestaValutata `json:"richiesta"`
	Prodotti         ancoraggio.EsitoProdotti     `json:"prodotti"`
	ProdottiValutati []ProdottoValutato           `json:"prodotti_valutati,omitempty"`
	Diagnostiche     []evidenze.Diagnostica       `json:"diagnostiche,omitempty"`
}

// ValutaProdotti: il percorso di B1 per un thread della fotografia, quello che Calcola (B6) userà per ogni thread
// (6.4.6, passi 3-6 e 6a, nella parte che riguarda i messaggi, la richiesta, i target e la fonte).
//  1. I documenti dei messaggi (estrazione.DaMessaggio).
//  2. RichiestaDelThread, con il caso del thread se il file dei casi ne ha uno.
//  3. L'interpretazione di ogni messaggio con il suo uso, poi ancoraggio.ProponiProdotti: i candidati della mail.
//  4. I prodotti target (targetDelThread: R70 A, R75 A, T-E1-24).
//  5. La fonte di ogni target (contesto.fonte: R65 A, R76 b A, T-B0-07, T-B0-08, T-E1-09, T-12).
//
// m è il motore della grammatica del cliente; nil = nessuna grammatica A: niente interpretazioni né candidati, i
// target confermati senza la base letta, e la fonte senza il confronto delle radici (non_determinabile dove servirebbe).
// caso è il caso del file dei casi per questo thread (Ingressi.CasoDelThread), o nil.
//
// È pura e deterministica: l'ordine degli elenchi della fotografia non conta, e la fotografia di chi chiama non
// cambia. L'errore è di contratto o di un adattatore: un caso di un altro thread o di un altro cliente, un messaggio
// che l'adattatore non legge, un uso dei segmenti che non torna con il documento (un segmento del caso che non c'è),
// un errore di ProponiProdotti.
func ValutaProdotti(f fotorfq.Fotografia, t fotorfq.Thread, m *motorea.Motore, caso *IngressoCaso) (ValutazioneProdotti, error) {
	if caso != nil && (caso.ThreadID == nil || *caso.ThreadID != t.ID || caso.ClienteID != t.ClienteID) {
		return ValutazioneProdotti{}, fmt.Errorf("valutazione: il caso %q non è del thread %s e del suo cliente", caso.ID, t.ID)
	}
	out := ValutazioneProdotti{ThreadID: t.ID}

	messaggi := messaggiInOrdine(t)
	docs := make(map[uuid.UUID]evidenze.DocumentoEvidenze, len(messaggi))
	for _, msg := range messaggi {
		d, err := estrazione.DaMessaggio(msg)
		if err != nil {
			return ValutazioneProdotti{}, fmt.Errorf("valutazione: thread %s: %w", t.ID, err)
		}
		docs[msg.ID] = d
	}
	richiesta, usi := RichiestaDelThread(t, caso, docs)
	out.Richiesta = richiesta

	var interpretati []ancoraggio.MessaggioInterpretato
	if m != nil {
		for _, msg := range messaggi {
			d := docs[msg.ID]
			uso, ok := usi[d.BundleID]
			if !ok {
				uso = evidenze.UsoSconosciuto(d.BundleID)
			}
			r, err := m.Interpreta(d, uso)
			if err != nil {
				return ValutazioneProdotti{}, fmt.Errorf("valutazione: thread %s, messaggio %s: %w", t.ID, msg.ID, err)
			}
			interpretati = append(interpretati, ancoraggio.MessaggioInterpretato{MessaggioID: msg.ID, Documento: d, Interpretazione: r})
		}
	}
	prodotti, err := ancoraggio.ProponiProdotti(interpretati, richiesta)
	if err != nil {
		return ValutazioneProdotti{}, fmt.Errorf("valutazione: thread %s: %w", t.ID, err)
	}
	out.Prodotti = prodotti

	targets, diag := targetDelThread(t, m, caso, prodotti.Candidati)
	c := nuovoContesto(f, t, m)
	for _, tg := range targets {
		fonte, d := c.fonte(tg)
		tg.pv.Fonte = fonte
		out.ProdottiValutati = append(out.ProdottiValutati, tg.pv)
		diag = append(diag, d...)
	}
	sort.SliceStable(diag, func(i, j int) bool { return chiaveDiagnostica(diag[i]) < chiaveDiagnostica(diag[j]) })
	out.Diagnostiche = diag
	return out, nil
}

// chiaveDiagnostica: l'ordine canonico delle diagnostiche, (codice, percorso, riferimenti) e poi il messaggio, come
// in motorea e in ancoraggio.
func chiaveDiagnostica(d evidenze.Diagnostica) string {
	return d.Codice + "\x00" + d.Percorso + "\x00" + strings.Join(d.Rif, "\x01") + "\x00" + d.Messaggio
}
