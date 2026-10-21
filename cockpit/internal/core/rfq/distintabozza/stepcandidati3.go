package distintabozza

// Lo STEP candidato sorgente di un prodotto finito (G39-02 [U], 20/10 sera; direttiva 39 §5; D-2 (c) [U]): lo STEP del
// prodotto finito non e' un file ordinario. E' un candidato sorgente, che il Motore A riconosce dal codice del prodotto
// della RFQ e dal codice che legge nel file (il nome, la radice dello STEP), con il marcatore (per esempio la base
// 12345678 letta nel nome contro il prodotto 12345678Z, codici inventati; PM-01 «Z = marcatore»). Si registra solo con la conferma S8
// (fonte_step_evento); mai una PB ne' una AR ordinaria. Gli STEP di parti e sottoassiemi restano PB → R5 → AR (D-2 (c)).
//
// Chi lo usa (G39-02, conseguenze [T] lette dall'orchestratore e confermate; G39-06 [U], giro G39-ter):
//   - le PB iniziali del server e i suggerimenti (ClassificaProposte, FattiPB.StepDelFinito): ne' PB ne' suggerimento,
//     per ogni candidatura, univoca o no (il motore non propone da solo lo STEP non univoco a nessuno);
//   - R2: una PB nuova o spostata su uno STEP candidato e' 422 STEP_CANDIDATO_DEL_FINITO, anche nelle bozze manuali o
//     «solo prodotto» (preassociazioni3.go), quando BloccaPB: la candidatura e' univoca (o del ripiego), oppure la PB va
//     verso un prodotto finito (il nodo della bozza del prodotto, un nodo di tipo finito o il nodo di un componente finito
//     della RFQ: li' la strada e' S8). La PB su uno STEP candidato NON univoco verso una parte o un sottoassieme e' la
//     scelta dell'utente (G39-06 [U]: «lasciamo la scelta di conferma all'utente sul file che preferisce»; lettura [T]
//     dell'orchestratore) e si salva;
//   - R4 (e R5, che ricalcola R4): una PB su uno STEP candidato rimasta da una versione di prima, con BloccaPB, e' in
//     conflitto con il motivo step_candidato_del_finito (associazioni.MotivoStepCandidatoDelFinito), quindi un blocco
//     PREASSOCIAZIONE_NON_APPLICABILE;
//   - R1: step_candidati[] del prodotto, perche' la pagina avvisi quando si apre una bozza manuale o «solo prodotto» con uno
//     STEP candidato non confermato.
//
// Il riconoscimento con il Motore A (StepCandidatiDalMotore, pura, che chiama propostabom.CandidatureSorgenteDelFinito:
// la regola e' scritta una volta sola, nel motore; giro G39-bis: senza un'evidenza discordante, cioe' il nome del file
// che e' o comincia con il codice di un altro componente che non e' un finito, o un candidato di un componente che non e'
// un finito nell'ancoraggio, e con la rete del nome uguale al codice del finito. Contano tutte le candidature, univoche
// o no, per R2, PB iniziali, suggerimenti e R4; Univoche dice quelle univoche, le sole che S9 propone e che S8 conferma
// senza il motivo) [T-39]: lo STEP e' candidato di un
// prodotto finito valutato (ProdottoValutato con il componente) se vale una di queste, tutte calcolate dal motore:
//   - l'ancoraggio del file ha il prodotto fra i candidati a livello prodotto, senza dimensioni discordanti (namespace,
//     base e marcatore compatibili con il codice del prodotto: la regola unica dell'ancoraggio, T-B4-06, T-B4-30);
//   - fra i candidati della fonte del prodotto c'e' l'allegato con la radice compatibile, trovato dal motore (anche per il
//     nome del file, radice di riserva) o documento STEP corrente del prodotto;
//   - una struttura candidata del prodotto ha l'allegato con il prodotto come radice del file.
// Il riconoscimento non guarda documento_proposta.codice (la lettura legacy).
//
// Il ripiego senza il motore [T-39 corsia persistenza]: dove il calcolo del motore non c'e' (R1 senza
// ?suggerimenti_motore=1, R4 senza ?avvisi_motore=1; R2 e la nascita da-conferma-1 solo con le regole del motore non
// configurate: con il motore occupato, scaduto o in errore l'adattatore risponde 503/504 e non scrive) vale la query
// ListStepCandidatiDelFinito: la proposta di documento cad_3d di
// un finito (la lettura legacy, per la copia o per il codice), con il confronto del marcatore (il codice letto uguale al
// codice del prodotto intero o senza la lettera finale dopo una cifra). Il ripiego non vede quello che solo il motore legge
// (la radice dello STEP, un nome che il vecchio classificatore non ha letto): li ferma R4 con il motore e R5, che lo
// ricalcola sempre.

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/db"
)

// CodiceStepCandidatoDelFinito: R2, una PB nuova o spostata su uno STEP candidato sorgente di un prodotto finito (G39-02
// [U]): 422. Il rimedio e' la conferma della sorgente (S8) dal prodotto, oppure togliere la preassociazione.
const CodiceStepCandidatoDelFinito = "STEP_CANDIDATO_DEL_FINITO"

// StepCandidati: gli STEP candidati sorgente dei prodotti finiti della RFQ, per allegato, con i prodotti per cui lo sono.
// DalMotore dice se il riconoscimento e' del Motore A (vero) o del ripiego (ListStepCandidatiDelFinito). Prodotti ha
// tutte le candidature, univoche o no (R2, PB iniziali, suggerimenti, R4: G39-02 [U], mai PB ne' AR ordinaria);
// Univoche, solo con il motore, l'allegato delle candidature univoche con il suo prodotto (la proposta di S9, l'esenzione
// dal motivo in S8 e, con BloccaPB, il rifiuto della PB verso una parte o un sottoassieme: giro G39-bis, revisione
// indipendente; giro G39-ter, G39-06 [U]; il ripiego non la da'); Evidenze, solo per le
// univoche, le evidenze della candidatura (la traccia che S8 scrive quando corregge il codice del documento senza il
// motivo: giro G39-bis, integrazione, [T-39 da ratificare]).
type StepCandidati struct {
	DalMotore bool
	Prodotti  map[uuid.UUID][]uuid.UUID
	Univoche  map[uuid.UUID]uuid.UUID
	Evidenze  map[uuid.UUID][]propostabom.EvidenzaCandidatura
}

// Di: l'allegato e' uno STEP candidato sorgente di un finito (univoco o no).
func (s *StepCandidati) Di(a uuid.UUID) bool {
	return s != nil && len(s.Prodotti[a]) > 0
}

// BloccaPB (G39-02 [U], G39-06 [U]; giro G39-ter): la PB sull'allegato e' esclusa (R2: 422 STEP_CANDIDATO_DEL_FINITO;
// R4: in conflitto con step_candidato_del_finito). versoFinito dice se il nodo della PB e' un prodotto finito
// (NodoVersoUnFinito). Vero quando l'allegato e' uno STEP candidato e la PB va verso un prodotto finito (la strada e' S8,
// univoca o no), oppure la candidatura e' univoca, oppure il riconoscimento e' del ripiego, che non distingue le
// univoche (il lato prudente [T-39]). Falso per la PB su uno STEP candidato non univoco verso una parte o un
// sottoassieme: e' la scelta dell'utente (G39-06 [U]).
func (s *StepCandidati) BloccaPB(a uuid.UUID, versoFinito bool) bool {
	if !s.Di(a) {
		return false
	}
	if versoFinito || !s.DalMotore {
		return true
	}
	_, univoca := s.Univoche[a]
	return univoca
}

// NodoVersoUnFinito: il nodo della chiave data nello snapshot e' un prodotto finito: il nodo del prodotto della bozza, un
// nodo di tipo finito, o il nodo di un componente finito della RFQ (st, nil = non guardato). Un nodo che non c'e' conta
// come un finito (il lato prudente: lo segnalano le altre regole).
func NodoVersoUnFinito(s Snapshot3, chiave string, st *confermabom.StatoRFQ) bool {
	for _, n := range s.Nodi {
		if n.ChiaveBozza != chiave {
			continue
		}
		if nodoDiUnFinito(s, n) {
			return true
		}
		if n.ComponenteID != nil && st != nil {
			if c, ok := st.Componente(*n.ComponenteID); ok && c.Tipo == string(db.TipoComponenteFinito) {
				return true
			}
		}
		return false
	}
	return true
}

// UnivocaDel: l'allegato e' il candidato univoco del prodotto dato per il Motore A (falso con il ripiego). E' la
// condizione dell'esenzione dal motivo in S8 per lo STEP proposto dal motore.
func (s *StepCandidati) UnivocaDel(a, prodotto uuid.UUID) bool {
	if s == nil || !s.DalMotore {
		return false
	}
	p, ok := s.Univoche[a]
	return ok && p == prodotto
}

// EvidenzeUnivocaDel: le evidenze della candidatura univoca dell'allegato per il prodotto dato (nil se non e' il candidato
// univoco del prodotto per il Motore A, come UnivocaDel).
func (s *StepCandidati) EvidenzeUnivocaDel(a, prodotto uuid.UUID) []propostabom.EvidenzaCandidatura {
	if !s.UnivocaDel(a, prodotto) {
		return nil
	}
	return s.Evidenze[a]
}

// DelProdotto: gli allegati candidati sorgente del prodotto dato, in ordine.
func (s *StepCandidati) DelProdotto(prodotto uuid.UUID) []uuid.UUID {
	if s == nil {
		return nil
	}
	var out []uuid.UUID
	for a, pp := range s.Prodotti {
		for _, p := range pp {
			if p == prodotto {
				out = append(out, a)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

func (s *StepCandidati) aggiungi(a, p uuid.UUID) {
	for _, x := range s.Prodotti[a] {
		if x == p {
			return
		}
	}
	s.Prodotti[a] = append(s.Prodotti[a], p)
	sort.Slice(s.Prodotti[a], func(i, j int) bool { return s.Prodotti[a][i].String() < s.Prodotti[a][j].String() })
}

// StepCandidatiDalMotore: il riconoscimento del commento di testa sul calcolo del Motore A di un thread (nil = nessun
// calcolo) e sulla fotografia dello stesso calcolo (th: i nomi dei file e i codici dei componenti). Pura. Non filtra
// l'estensione: lo fa StepCandidatiDelFinito, sugli allegati che chiede (la rete del nome esatto di propostabom guarda
// comunque l'estensione del file).
//
// La regola e' una sola [T-39, revisione della corsia motore, giro G39; giro G39-bis, 21/10]: le coppie (allegato,
// prodotto) sono quelle di TUTTE le candidature di propostabom.CandidatureSorgenteDelFinito, univoche o no, la stessa
// funzione che S9 e /dati usano per la proposta di sorgente, qui senza il filtro dell'estensione (eStep sempre vero);
// Univoche ha quelle univoche. Una candidatura non univoca (lo STEP legato a piu' prodotti finiti, o con un altro
// candidato nell'ancoraggio, o con il nome ambiguo) resta uno STEP candidato del finito per R2, le PB iniziali e R4
// (G39-02 [U]: mai PB ne' AR ordinaria; il lato prudente, revisione indipendente del giro G39-bis): non si propone, si
// conferma con S8 verso un finito oppure, per scelta dell'utente, si preassocia a una parte o a un sottoassieme (BloccaPB,
// G39-06 [U], giro G39-ter). Solo una candidatura tolta dall'evidenza discordante (il nome del file e' o comincia con il codice
// di un altro componente che non e' un finito, o l'ancoraggio ha un candidato di un componente che non e' un finito:
// D-2 (c) [U], D-AR04 (iii) [U]) non e' uno STEP candidato: R2 accetta la sua PB, le PB iniziali e R4 la trattano come
// un file ordinario.
func StepCandidatiDalMotore(et *valutazione.EsitoThread, th *fotorfq.Thread) *StepCandidati {
	if et == nil {
		return nil
	}
	out := &StepCandidati{DalMotore: true, Prodotti: map[uuid.UUID][]uuid.UUID{}, Univoche: map[uuid.UUID]uuid.UUID{},
		Evidenze: map[uuid.UUID][]propostabom.EvidenzaCandidatura{}}
	tutti := func(uuid.UUID) bool { return true }
	for _, c := range propostabom.CandidatureSorgenteDelFinito(et, th, tutti) {
		out.aggiungi(c.AllegatoID, c.ProdottoID)
		if c.Univoca {
			out.Univoche[c.AllegatoID] = c.ProdottoID
			out.Evidenze[c.AllegatoID] = c.Evidenze
		}
	}
	return out
}

// StepCandidatiDelFinito: fra gli allegati dati, gli STEP (estensione stp o step) candidati sorgente di un finito della
// RFQ, nella transazione di chi chiama (sola lettura): con il riconoscimento del motore, se c'e' (motore.DalMotore);
// altrimenti con il ripiego ListStepCandidatiDelFinito (commento di testa). E' la funzione che le altre corsie chiamano
// per «questo allegato e' uno STEP candidato del finito».
func StepCandidatiDelFinito(ctx context.Context, q *db.Queries, thread uuid.UUID, allegati []uuid.UUID, motore *StepCandidati) (*StepCandidati, error) {
	ids := unici3(allegati)
	if motore != nil && motore.DalMotore {
		out := &StepCandidati{DalMotore: true, Prodotti: map[uuid.UUID][]uuid.UUID{}, Univoche: map[uuid.UUID]uuid.UUID{},
			Evidenze: map[uuid.UUID][]propostabom.EvidenzaCandidatura{}}
		var cercati []uuid.UUID
		for _, a := range ids {
			if motore.Di(a) {
				cercati = append(cercati, a)
			}
		}
		if len(cercati) == 0 {
			return out, nil
		}
		rr, err := q.ListAllegatiPreassociabili(ctx, db.ListAllegatiPreassociabiliParams{ThreadID: thread, Allegati: cercati})
		if err != nil {
			return nil, err
		}
		for _, r := range rr {
			if r.DellaRfq && eStep(r.Estensione) {
				for _, p := range motore.Prodotti[r.AllegatoID] {
					out.aggiungi(r.AllegatoID, p)
				}
				if p, ok := motore.Univoche[r.AllegatoID]; ok {
					out.Univoche[r.AllegatoID] = p
					out.Evidenze[r.AllegatoID] = motore.Evidenze[r.AllegatoID]
				}
			}
		}
		return out, nil
	}
	out := &StepCandidati{Prodotti: map[uuid.UUID][]uuid.UUID{}}
	if len(ids) == 0 {
		return out, nil
	}
	rr, err := q.ListStepCandidatiDelFinito(ctx, db.ListStepCandidatiDelFinitoParams{ThreadID: thread, Allegati: ids})
	if err != nil {
		return nil, err
	}
	for _, r := range rr {
		out.aggiungi(r.AllegatoID, r.ProdottoID)
	}
	return out, nil
}

// unici3: gli UUID senza ripetizioni e senza il nullo, in ordine.
func unici3(xx []uuid.UUID) []uuid.UUID {
	visti := map[uuid.UUID]bool{}
	out := make([]uuid.UUID, 0, len(xx))
	for _, x := range xx {
		if x != uuid.Nil && !visti[x] {
			visti[x] = true
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}
