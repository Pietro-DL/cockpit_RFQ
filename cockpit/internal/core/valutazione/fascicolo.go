package valutazione

import (
	"sort"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
)

// fascicolo.go: lo stato del fascicolo della RFQ (R88, R89, R96 (a) A, (b) B, (c) A con il gesto che resta [R]; T-B0-28,
// T-E1-01, T-E1-16; LD-09, LD-10, LD-23; contratto §0.3, §1.0, §1.7, §2.3, §2.5; fase 0 di B6, F.2, F0-06, F0-07, F0-18;
// T-B6-09, T-B6-12). La regola (Fascicolo) è una funzione pura sui prodotti valutati del thread e sul gesto di
// congelamento del modello nuovo, che è un ingresso astratto senza adattatore (LD-23): Calcola passa nil. Il congelamento
// legacy (la bom_versione) si legge dalla fotografia e si mostra a parte (congelamentoLegacy).

// I valori di StatoFascicolo.MotivoNonCongelabile (T-B6-12), con la precedenza di F0-06: la prima che vale; gli altri
// motivi che valgono insieme vanno in Avvisi.
const (
	NonCongelabileThreadNonValutato        = "thread_non_valutato"
	NonCongelabileNessunTarget             = "nessun_target"
	NonCongelabileBOMVersioneNonVerificati = "bom_versione_con_prodotti_non_verificati"
	NonCongelabileProdottiNonPronti        = "prodotti_non_pronti"
)

// NonCongelatoGestoNonRegistrato: StatoFascicolo.MotivoNonCongelato senza il gesto del modello nuovo (contratto §2.5;
// LD-23: in A1c nessun adattatore).
const NonCongelatoGestoNonRegistrato = "gesto_non_registrato"

// I valori di StatoFascicolo.ConflittiCongelamento (contratto §2.5; R96 [R]): non_piu_congelabile, oppure il prefisso con
// il Rif del target, impronta_cambiata:<rif> e target_nuovo:<rif>.
const (
	CongelamentoNonPiuCongelabile        = "non_piu_congelabile"
	PrefissoCongelamentoImprontaCambiata = "impronta_cambiata:"
	PrefissoCongelamentoTargetNuovo      = "target_nuovo:"
)

// prefissoAvvisoNonCongelabile: il formato degli avvisi dei motivi di non congelabilità che valgono insieme al primo
// (F0-06): «non_congelabile:<motivo>», accanto agli avvisi degli orfani (F0-18: «orfano:<allegato_id>…»). Formato [T]
// (dubbio T-B6-87).
const prefissoAvvisoNonCongelabile = "non_congelabile:"

// statoVersioneCongelata: lo stato di una bom_versione congelata (0020, bom_versione.stato: bozza | congelata), ripetuto
// qui perché il legacy non lo esporta.
const statoVersioneCongelata = "congelata"

// StatoFascicolo: il fascicolo della RFQ (contratto §2.3, §2.5).
//   - Pronti: i Rif dei prodotti verificati; Bloccati: gli altri, con i motivi.
//   - Congelato, CongelatoDa, CongelatoIl: solo dal gesto di congelamento del modello nuovo (in A1c nessuno: LD-23).
//   - Calcolato: falso con T-12 sulle sezioni dei target, della versione della BOM e dello smistamento, per un thread
//     con un errore della valutazione e per un thread senza grammatica (dubbio T-B6-86; R-86 della controprova di V3).
//   - ConflittiCongelamento: non_piu_congelabile | impronta_cambiata:<rif> | target_nuovo:<rif> (F0-07).
//   - Legacy: il congelamento legacy, da Thread.VersioneBOM e UltimaCongelata (R96 b B, T-B0-28).
//   - Orfani, Avvisi: i file orfani (formato degli avvisi in F0-18: «orfano:<allegato_id>», con il contesto
//     «orfano:<allegato_id>:prodotti_contesto:<rif>|<rif>») e, dopo, i motivi di non congelabilità che valgono insieme al
//     primo («non_congelabile:<motivo>»). FaseThread: lo stato del thread, APERTA o CHIUSA (fotorfq.Thread.Stato, cioè
//     thread_offerta.stato; LD-10). Non è la fase di lavoro: la fase registrata la legge il gestore di A1d dalla vista
//     v_thread_fase, fuori dall'esito (EB7-3 A, E2 §6.4); fase, stato del thread e prontezza del prodotto sono tre dati.
type StatoFascicolo struct {
	NumeroTarget          int                 `json:"numero_target"`
	NumeroVerificati      int                 `json:"numero_verificati"`
	Pronti                []string            `json:"pronti,omitempty"`
	Bloccati              []ProdottoBloccato  `json:"bloccati,omitempty"`
	Congelabile           bool                `json:"congelabile"`
	MotivoNonCongelabile  string              `json:"motivo_non_congelabile,omitempty"`
	Congelato             bool                `json:"congelato"`
	CongelatoDa           *uuid.UUID          `json:"congelato_da,omitempty"`
	CongelatoIl           *time.Time          `json:"congelato_il,omitempty"`
	Calcolato             bool                `json:"calcolato"`
	MotivoNonCongelato    string              `json:"motivo_non_congelato,omitempty"`
	ConflittiCongelamento []string            `json:"conflitti_congelamento,omitempty"`
	Legacy                *CongelamentoLegacy `json:"legacy,omitempty"`
	Orfani                int                 `json:"orfani"`
	Avvisi                []string            `json:"avvisi,omitempty"`
	FaseThread            string              `json:"fase_thread"`
}

// ProdottoBloccato: un prodotto non verificato del fascicolo, con i motivi (contratto §2.3).
type ProdottoBloccato struct {
	Rif    string           `json:"rif"`
	Motivi []MotivoProdotto `json:"motivi,omitempty"`
}

// CongelamentoLegacy: il congelamento della BOM del legacy, che si mostra com'è e non vale come fascicolo congelato
// (contratto §2.5; R96 b B, c A; T-B0-28).
type CongelamentoLegacy struct {
	VersioneCorrente int32      `json:"versione_corrente"`
	StatoCorrente    string     `json:"stato_corrente"`
	UltimaCongelata  *int32     `json:"ultima_congelata,omitempty"`
	CongelataDa      *uuid.UUID `json:"congelata_da,omitempty"`
	CongelataIl      *time.Time `json:"congelata_il,omitempty"`
	Riaperta         bool       `json:"riaperta"`
}

// GestoCongelamento: il gesto di congelamento del modello nuovo, l'ingresso astratto della regola (contratto §2.5; R89,
// R96 [R]): chi, quando e i target al momento del gesto, ognuno con la sua impronta. In A1c nessun adattatore lo produce
// (LD-23): Calcola passa nil, e la regola si prova con gesti sintetici (PO-31). Le Impronte si scorrono in ordine di
// chiave (determinismo).
type GestoCongelamento struct {
	Da       uuid.UUID         `json:"da"`
	Il       time.Time         `json:"il"`
	Impronte map[string]string `json:"impronte"`
}

// IngressoFascicolo: l'ingresso della regola del fascicolo (F.4), che l'esito del thread prepara (esitoDelThread).
//   - Valutato: il thread è valutato (T-B6-09: con un thread senza grammatica o con un errore il fascicolo non si congela).
//   - Calcolato: le sezioni dei target, della versione della BOM e dello smistamento ci sono (T-12), la valutazione non
//     ha dato un errore e il thread ha la grammatica (lo stesso criterio della pertinenza: R-86 della controprova di V3).
//   - Prodotti: i prodotti target valutati (ProdottiValutati, con Verificato, Motivi e Impronta).
//   - Legacy: il congelamento legacy (congelamentoLegacy). FaseThread: lo stato del thread, APERTA o CHIUSA (LD-10), non
//     la fase di lavoro, che il gestore di A1d legge da v_thread_fase (EB7-3 A).
//   - DaSmistare: «da smistare» del thread, per gli orfani e i loro avvisi (avvisiDegliOrfani, V2).
type IngressoFascicolo struct {
	Valutato   bool                `json:"valutato"`
	Calcolato  bool                `json:"calcolato"`
	Prodotti   []ProdottoValutato  `json:"prodotti,omitempty"`
	Legacy     *CongelamentoLegacy `json:"legacy,omitempty"`
	FaseThread string              `json:"fase_thread"`
	DaSmistare []FileDaSmistare    `json:"da_smistare,omitempty"`
}

// Fascicolo: la regola del fascicolo della RFQ (R88, R89; contratto §0.3, §1.0; T-B0-28, T-E1-16; F0-06, F0-07; T-B6-12).
// È pura e deterministica: l'ordine dei prodotti non conta, e le impronte del gesto si scorrono in ordine di chiave.
//
//		FascicoloCongelabile = NumeroTarget >= 1 AND NumeroVerificati = NumeroTarget   (R96 a A [R])
//		                       (e il thread valutato, T-B6-09, e il fascicolo calcolato, T-12)
//		FascicoloCongelato   = c'è il gesto del modello nuovo: un fatto, mai dedotto da Congelabile (R89)
//
//	  - NumeroTarget, NumeroVerificati; Pronti (i Rif dei verificati) e Bloccati (gli altri, con i loro motivi), in ordine
//	    di Rif: ognuno dei pronti può passare alla fattibilità da solo (R88), e pronto non vuol dire partito (FaseThread).
//	  - MotivoNonCongelabile, il primo che vale (F0-06): thread_non_valutato; nessun_target; bom_versione_con_prodotti_non_
//	    verificati (la versione corrente della BOM legacy è congelata, e un prodotto non è verificato: si mostra com'è, con
//	    il motivo, T-B0-28); prodotti_non_pronti. I motivi dei target valgono solo con il fascicolo calcolato. Gli altri che
//	    valgono insieme vanno in Avvisi («non_congelabile:<motivo>»), dopo gli avvisi degli orfani. Gli orfani sono un
//	    avviso, mai una condizione di Congelabile (R93, T-E1-16).
//	  - Senza il gesto: Congelato falso, con gesto_non_registrato (LD-23), anche con una bom_versione congelata (R96 b B), che
//	    si mostra in Legacy.
//	  - Con il gesto: Congelato vero, con chi e quando; il gesto resta, e il fascicolo mostra i conflitti, mai in silenzio
//	    (R96 [R]): non_piu_congelabile se oggi non è congelabile; impronta_cambiata:<rif> per ogni target del gesto la cui
//	    impronta di oggi è diversa, anche un target che oggi non c'è più (F0-07: l'impronta di oggi, vuota, è diversa);
//	    target_nuovo:<rif> per ogni target di oggi che il gesto non conosceva. In quest'ordine, poi per Rif. Lo stato del
//	    prodotto non cambia: segue i suoi assi (T-E1-14).
func Fascicolo(in IngressoFascicolo, gesto *GestoCongelamento) StatoFascicolo {
	out := StatoFascicolo{Calcolato: in.Calcolato, FaseThread: in.FaseThread, Legacy: copiaLegacy(in.Legacy)}
	out.Orfani, out.Avvisi = avvisiDegliOrfani(in.DaSmistare)
	prodotti := append([]ProdottoValutato(nil), in.Prodotti...)
	sort.SliceStable(prodotti, func(i, j int) bool { return prodotti[i].Rif < prodotti[j].Rif })
	attuali := map[string]string{}
	for _, pv := range prodotti {
		attuali[pv.Rif] = pv.Impronta
		if pv.Verificato {
			out.NumeroVerificati++
			out.Pronti = append(out.Pronti, pv.Rif)
			continue
		}
		out.Bloccati = append(out.Bloccati, ProdottoBloccato{Rif: pv.Rif, Motivi: append([]MotivoProdotto(nil), pv.Motivi...)})
	}
	out.NumeroTarget = len(prodotti)
	var motivi []string
	if !in.Valutato {
		motivi = append(motivi, NonCongelabileThreadNonValutato)
	}
	if in.Calcolato {
		nonVerificati := out.NumeroVerificati < out.NumeroTarget
		if out.NumeroTarget == 0 {
			motivi = append(motivi, NonCongelabileNessunTarget)
		}
		if nonVerificati && in.Legacy != nil && in.Legacy.StatoCorrente == statoVersioneCongelata {
			motivi = append(motivi, NonCongelabileBOMVersioneNonVerificati)
		}
		if nonVerificati {
			motivi = append(motivi, NonCongelabileProdottiNonPronti)
		}
	}
	out.Congelabile = in.Calcolato && len(motivi) == 0
	if len(motivi) > 0 {
		out.MotivoNonCongelabile = motivi[0]
		for _, m := range motivi[1:] {
			out.Avvisi = append(out.Avvisi, prefissoAvvisoNonCongelabile+m)
		}
	}

	if gesto == nil {
		out.MotivoNonCongelato = NonCongelatoGestoNonRegistrato
		return out
	}
	da, il := gesto.Da, gesto.Il
	out.Congelato, out.CongelatoDa, out.CongelatoIl = true, &da, &il
	if !out.Congelabile {
		out.ConflittiCongelamento = append(out.ConflittiCongelamento, CongelamentoNonPiuCongelabile)
	}
	chiavi := make([]string, 0, len(gesto.Impronte))
	for k := range gesto.Impronte {
		chiavi = append(chiavi, k)
	}
	sort.Strings(chiavi)
	for _, k := range chiavi {
		if oggi, ok := attuali[k]; !ok || oggi != gesto.Impronte[k] {
			out.ConflittiCongelamento = append(out.ConflittiCongelamento, PrefissoCongelamentoImprontaCambiata+k)
		}
	}
	for _, pv := range prodotti {
		if _, ok := gesto.Impronte[pv.Rif]; !ok {
			out.ConflittiCongelamento = append(out.ConflittiCongelamento, PrefissoCongelamentoTargetNuovo+pv.Rif)
		}
	}
	return out
}

// sezioniDelFascicolo: le sezioni della fotografia da cui il fascicolo dipende (T-12): i target (gli identificativi e i
// componenti: R60, R70 A) e la versione della BOM (il congelamento legacy). Senza una di loro il fascicolo non è
// calcolato (dubbio T-B6-86).
var sezioniDelFascicolo = []string{fotorfq.SezioneIdentificativi, fotorfq.SezioneComponenti, fotorfq.SezioneVersioneBOM}

// fascicoloNonDeterminabile: una sezione del fascicolo è assente (T-12): dei target e della versione della BOM
// (sezioniDelFascicolo), oppure dello smistamento (sezioniDelloSmistamento). Senza una sezione dello smistamento la
// pertinenza non si calcola, gli orfani e i loro avvisi non ci sono (R-75 della revisione di V2), e lo smistamento dei
// prodotti non è calcolato: un fascicolo che li mostrasse come calcolati direbbe «nessun orfano» senza saperlo (la nota di
// V2 per V3, accolta con la revisione di V3; dubbio T-B6-86, allargato).
func fascicoloNonDeterminabile(sezioni map[string]fotorfq.StatoSezione) bool {
	return unaSezioneAssente(sezioni, sezioniDelFascicolo, sezioniDelloSmistamento)
}

// congelamentoLegacy: il congelamento legacy del thread (R96 b B, c A [R]; T-B0-28), dalla fotografia: la versione
// corrente della BOM (Thread.VersioneBOM: l'ultima, bozza o congelata) e l'ultima congelata (Thread.UltimaCongelata), con
// chi e quando. Riaperta: c'è una versione congelata, ma la corrente non lo è (una bozza dopo una congelata: il fascicolo
// legacy è riaperto, e la congelata si mostra). nil senza versioni. Non vale mai come fascicolo congelato.
func congelamentoLegacy(t fotorfq.Thread) *CongelamentoLegacy {
	corrente, congelata := t.VersioneBOM, t.UltimaCongelata
	if corrente == nil {
		corrente = congelata
	}
	if corrente == nil {
		return nil
	}
	l := &CongelamentoLegacy{VersioneCorrente: corrente.Numero, StatoCorrente: corrente.Stato}
	if congelata != nil {
		n := congelata.Numero
		l.UltimaCongelata, l.CongelataDa, l.CongelataIl = &n, copiaUUID(congelata.CongelataDa), copiaTempo(congelata.CongelataIl)
		l.Riaperta = corrente.Stato != statoVersioneCongelata
	}
	return l
}

// copiaLegacy: il congelamento legacy che non condivide memoria con quello di partenza.
func copiaLegacy(l *CongelamentoLegacy) *CongelamentoLegacy {
	if l == nil {
		return nil
	}
	c := *l
	if l.UltimaCongelata != nil {
		n := *l.UltimaCongelata
		c.UltimaCongelata = &n
	}
	c.CongelataDa, c.CongelataIl = copiaUUID(l.CongelataDa), copiaTempo(l.CongelataIl)
	return &c
}
