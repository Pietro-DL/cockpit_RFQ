// Package propostabom: dalla proposta del Motore A alla bozza della Distinta unica (V1: emendamento di V1 §6.1, §6.4.1;
// piano della 0023 §14 passo 3). È puro: nessun DB, nessun file, nessun orologio; legge l'esito già calcolato della
// valutazione e non decide niente.
//
// Oggi ci sono la chiave dei gruppi di un prodotto, che prima stava nella pagina (transport/web/motore_a, indice.go), e il
// gruppo corrente della sorgente di una bozza (la regola di O-01). L'adattatore che costruisce il registro d'importazione
// e il confronto alla riapertura arrivano con il DTO del registro.
package propostabom

import (
	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/valutazione"
)

// Le chiavi e i tipi dei gruppi di un prodotto (X-02; T-A1d-09): stringhe opache e stabili, che non dipendono
// dall'ordine. «bom» è la BOM di lavoro del prodotto, «cand:<allegato_id>:<radice>» una struttura candidata (la chiave è
// ChiaveCandidata), «senza» i confermati senza STEP. Una chiave non si scompone mai per ricavarne un allegato o una
// radice: si confronta intera (contratto rev. 3 §3.2; emendamento di V1 §3.3). La chiave non contiene il prodotto: la
// stessa radice candidata per due prodotti dà la stessa stringa.
const (
	GruppoBOM       = "bom"
	GruppoCandidata = "cand"
	GruppoSenzaSTEP = "senza"
)

// ChiaveCandidata: la chiave del gruppo di una struttura candidata, «cand:<allegato_id>:<radice>», dove la radice è il
// Rif del nodo radice della struttura (ancoraggio.RifNodo).
func ChiaveCandidata(allegato uuid.UUID, radice string) string {
	return GruppoCandidata + ":" + allegato.String() + ":" + radice
}

// Gruppo: un gruppo di un prodotto, come la pagina lo mostra (gli stessi gruppi, nello stesso ordine), con i dati che
// lo legano a una struttura: l'occorrenza, il contenuto e la radice.
//   - Chiave, Tipo: GruppoBOM, ChiaveCandidata(…) con GruppoCandidata, GruppoSenzaSTEP.
//   - AllegatoID, Sha256, Radice: l'allegato e il contenuto dello STEP del gruppo e il Rif della radice della sua
//     struttura; per «bom» quelli della struttura della BOM di lavoro, se c'è; vuoti per «senza».
type Gruppo struct {
	Chiave     string
	Tipo       string
	AllegatoID *uuid.UUID
	Sha256     string
	Radice     string
}

// GruppiDelProdotto: i gruppi del prodotto, nessuno preselezionato, nell'ordine della pagina (T-A1d-09, T-A1d-27):
//   - «bom» se il prodotto ha la BOM di lavoro (ProdottoValutato.Nodi), con l'allegato, lo sha256 e la radice della prima
//     struttura del prodotto che è bom_di_lavoro_proposta (al più una per prodotto: R76 A); una struttura di un altro
//     prodotto non conta mai;
//   - una «cand:…» per ogni struttura candidata del prodotto, nell'ordine dell'esito;
//   - «senza» se il prodotto ha confermati senza STEP.
//
// È la regola con cui la pagina compone le sue schede: Indice.Gruppi la chiama e aggiunge solo i dati della pagina (il nome
// del file, la struttura candidata, i nodi del gruppo). Una sola, nel core.
func GruppiDelProdotto(pv valutazione.ProdottoValutato, strutture []ancoraggio.StrutturaProdotto) []Gruppo {
	var out []Gruppo
	if len(pv.Nodi) > 0 {
		g := Gruppo{Chiave: GruppoBOM, Tipo: GruppoBOM}
		for i := range strutture {
			if st := &strutture[i]; st.Target == pv.Rif && st.Stato == ancoraggio.StatoBOMDiLavoroProposta {
				id := st.AllegatoID
				g.AllegatoID, g.Sha256, g.Radice = &id, st.Sha256, st.Radice
				break
			}
		}
		out = append(out, g)
	}
	for _, sc := range pv.StruttureCandidate {
		id := sc.AllegatoID
		out = append(out, Gruppo{Chiave: ChiaveCandidata(sc.AllegatoID, sc.Radice), Tipo: GruppoCandidata, AllegatoID: &id,
			Sha256: sc.Sha256, Radice: sc.Radice})
	}
	if len(pv.ConfermatiSenzaSTEP) > 0 {
		out = append(out, Gruppo{Chiave: GruppoSenzaSTEP, Tipo: GruppoSenzaSTEP})
	}
	return out
}
