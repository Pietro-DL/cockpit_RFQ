package web

import "fmt"

// Il giro G39 della V1 (decisioni dell'utente del 20/10 sera, G39-02 [U]) nel banco finto della Distinta unica
// (distinta_unica_banco_test.go): lo STEP del prodotto finito è un candidato sorgente, riconosciuto con il Motore A (il
// codice del prodotto della RFQ e il codice letto dal nome del file, con il marcatore), proposto nella pagina prima di S8
// e registrato solo con S8; mai PB né AR ordinaria. Il banco dà la proposta con la forma del server della corsia motore
// (motore_a/proposta_sorgente.go): fonte.proposte_sorgente[] di S9 e di /dati, {allegato_id, nome_file, codice_prodotto,
// evidenze[], sorgente_di?, testo}; R2 rifiuta la preassociazione di quello STEP con il codice del server della corsia
// persistenza (distintabozza/stepcandidati3.go): 422 STEP_CANDIDATO_DEL_FINITO. Come il resto del banco: nessuna regola di
// dominio.
//
// Interruttore della sonda «step-proposto» (spento per difetto): acceso, il primo prodotto (ACM-1000) ha come STEP
// proposto lo STEP della BOM (ACM-1000_assieme.stp) finché non ha una sorgente S8 valida; se quello STEP è la sorgente S8
// valida del secondo prodotto (la sonda /__banco/sorgente-s8 con l'allegato), la proposta ha sorgente_di (G39-03).

// CodiceStepCandidatoDelFinito: il codice del 422 di R2 per una preassociazione dello STEP candidato sorgente di un finito
// (distintabozza.CodiceStepCandidatoDelFinito, G39-02 conseguenze [T]).
const CodiceStepCandidatoDelFinito = "STEP_CANDIDATO_DEL_FINITO"

// TestoPropostaSorgenteBanco: il testo della proposta come lo scrive il server (motore_a.TestoPropostaSorgente).
const TestoPropostaSorgenteBanco = "Il motore ha trovato che questo file è molto probabilmente lo STEP strutturale di %s: lo confermi come sorgente?"

// proposteSorgenteBanco: fonte.proposte_sorgente[] del prodotto per S9 e /dati (sempre un elenco, anche vuoto).
func (b *bancoDU) proposteSorgenteBanco(prod string) []any {
	out := []any{}
	if !b.stepProposto || prod != bdP1 || b.s8Valida(prod) != "" {
		return out
	}
	f := b.fileDi(bdA1)
	if f == nil || f.Stato != "attivo" || f.Analisi != "letto" {
		return out
	}
	x := m{"allegato_id": bdA1, "nome_file": f.Nome, "codice_prodotto": "ACM-1000", "testo": fmt.Sprintf(TestoPropostaSorgenteBanco, "ACM-1000"),
		"evidenze": []any{
			m{"lettura": "l:nome:1", "selettore": "nome_file", "codice_letto": "ACM-1000", "base_letta": "ACM-1000", "esito_base": "uguale", "esito_marcatore": "compatibile"},
			m{"lettura": "l:step:1", "selettore": "step.radice_id", "codice_letto": "ACM-1000Z", "base_letta": "ACM-1000", "marcatore_letto": "Z", "esito_base": "uguale", "esito_marcatore": "compatibile"},
			// la struttura candidata: il server dà la chiave interna del nodo come codice_letto (visto sul server vero)
			m{"selettore": "struttura_candidata", "codice_letto": "nodo:" + bdShaStep[:16] + ":#1", "esito_base": "compatibile"},
		}}
	if b.s8Valida(bdP2) == bdA1 {
		x["sorgente_di"] = bdP2
	}
	return append(out, x)
}

// stepCandidatoDelFinito: il file è lo STEP proposto come sorgente di un prodotto finito della RFQ (R2 lo rifiuta come
// documento di una PB, anche in una bozza manuale).
func (b *bancoDU) stepCandidatoDelFinito(f *fileBanco) bool {
	if b.stepCandidati && f.ID == bdA1 && b.s8Valida(bdP1) == "" {
		return true
	}
	for _, p := range []string{bdP1, bdP2} {
		for _, x := range b.proposteSorgenteBanco(p) {
			if bdStr(x.(m)["allegato_id"]) == f.ID {
				return true
			}
		}
	}
	return false
}

// suggerimentiMotoreR1: R1 con ?suggerimenti_motore=1 (distintabozza/leggi3.go, conIlCalcolo; G39-01, G39-02 [U]):
// suggerimenti_motore {calcolati} e step_candidati[] {allegato_id, dal_motore} del prodotto non ancora confermati da S8,
// anche per nessuna_bozza. Il banco dà lo STEP della BOM per il primo prodotto senza una sorgente S8 valida, con la
// proposta univoca accesa («step-proposto») o con il solo candidato («step-candidati»); la modalità «r1-motore» = errore
// dà suggerimenti_motore.errore (il motore che non calcola non è un errore di R1).
func (b *bancoDU) suggerimentiMotoreR1(o m, prod string, chiesti bool) {
	if !chiesti {
		return
	}
	if b.modo("r1-motore") == "errore" {
		o["suggerimenti_motore"] = m{"calcolati": false, "errore": "CALCOLO_OCCUPATO"}
		return
	}
	o["suggerimenti_motore"] = m{"calcolati": true}
	l := []any{}
	if prod == bdP1 && (b.stepProposto || b.stepCandidati) && b.s8Valida(prod) == "" {
		if f := b.fileDi(bdA1); f != nil && f.Stato == "attivo" {
			l = append(l, m{"allegato_id": bdA1, "dal_motore": true})
		}
	}
	o["step_candidati"] = l
}
