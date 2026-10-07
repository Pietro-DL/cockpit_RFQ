// L1 — gli aiuti delle prove dello stato del prodotto, dei nodi, dell'impronta e del fascicolo (B6, V3): gli invarianti di
// ogni prodotto e di ogni fascicolo, che gli aiuti valuta e calcola controllano su tutti i casi delle prove (PO-35; T-E1-01,
// T-B6-09, T-B6-11), la «prova generale» della coerenza delle fonti fra valutazione e ancoraggio (T-B2-02), le letture
// dell'esito.
package valutazione_test

import (
	"sort"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/valutazione"
)

// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un dato
// dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il giorno in cui
// un cliente cambia convenzione. Il cliente è ACME (acme.example), con la famiglia acme-catena (712xxxx con il marcatore A
// o B); i nomi dei file sono di fantasia, gli UUID sono 00000000-0000-4000-8000-0000000000nn. Le prove citano i requisiti
// (R79, R88, R89, R90, R91, R96, R111, T-E1-01, T-E1-14, T-E1-19, PO-01, PO-02, PO-18, PO-19, PO-20, PO-22, PO-24, PO-25,
// PO-26, PO-31, PO-35), mai i casi degli attesi.

// sezioniDellImpronta: le sezioni della fotografia da cui dipende l'impronta del prodotto (T-12): le cinque di IM.3, come
// le dice il contratto della fase 0, più quelle della fonte, perché l'impronta comprende il riferimento della fonte (R-82
// della revisione di V3). La prova le ripete per conto suo.
var sezioniDellImpronta = []string{fotorfq.SezioneComponenti, fotorfq.SezioneRelazioni, fotorfq.SezioneDocumenti, fotorfq.SezioneProvenienze,
	fotorfq.SezioneStepProdotto, fotorfq.SezioneAllegati, fotorfq.SezioneRigheComponenteProposta, fotorfq.SezioneRigheRelazioneProposta,
	fotorfq.SezioneLavoroPendente, fotorfq.SezioneProposteDocumento}

// sezioniDelFascicoloCalcolato: le sezioni della fotografia senza le quali il fascicolo non è calcolato (T-12; dubbio
// T-B6-86): i target e la versione della BOM, e quelle dello smistamento (la nota di V2, accolta con la revisione di V3).
// La prova le ripete per conto suo.
var sezioniDelFascicoloCalcolato = []string{fotorfq.SezioneIdentificativi, fotorfq.SezioneComponenti, fotorfq.SezioneVersioneBOM,
	fotorfq.SezioneAllegati, fotorfq.SezioneMessaggi, fotorfq.SezioneProposteDocumento, fotorfq.SezioneDocumenti, fotorfq.SezioneProvenienze,
	fotorfq.SezioneRelazioni, fotorfq.SezioneRigheComponenteProposta}

// assente: la sezione è assente nella fotografia (T-12).
func assente(f fotorfq.Fotografia, k string) bool {
	s, ok := f.Sezioni[k]
	return f.Sezioni != nil && (!ok || s.Stato == fotorfq.StatoSezioneAssente)
}

// invariantiDelProdotto: gli invarianti di ogni prodotto valutato (B6, V3; R79; contratto §1.0 e §1.7):
//   - PO-35: Verificato se e solo se lo stato è pronto_fattibilita (T-E1-01), e nessun campo «fattibilità avviata» per
//     prodotto (lo controlla esito_test.go sui campi del tipo);
//   - un prodotto verificato non ha motivi; uno non verificato ne ha almeno uno (PV.Motivi dice sempre perché);
//   - lo stato è uno dei tre; un thread non valutato e uno smistamento non calcolato danno non_pronto (T-B6-09, T-B6-61);
//   - revisione_tecnica_aperta fra i motivi se e solo se c'è un percorso aperto (T-E1-13);
//   - il composto (R-84 della revisione di V3): Verificato se e solo se valgono le sei condizioni (R79); da_riesaminare
//     solo con le precondizioni fisse di T-E1-14: il target confermato, la fonte calcolata e confermata o superata, lo
//     smistamento verificato o calcolato e fermo per file non terminali, conflitti o percorsi, la completezza completa o
//     non_calcolabile per il perimetro aperto da una rimozione o dalla fonte superata, con ogni voce certa presente (R-81);
//   - l'impronta: 64 cifre esadecimali, vuota solo con T-12 sulle sue sezioni (IM.3, con quelle della fonte: R-82);
//   - i nodi solo per la BOM di lavoro (T-B6-11), con la radice per prima.
func invariantiDelProdotto(t *testing.T, f fotorfq.Fotografia, valutato bool, pv valutazione.ProdottoValutato) {
	t.Helper()
	if pv.Verificato != (pv.Stato == valutazione.ProdottoProntoFattibilita) {
		t.Errorf("prodotto %s: verificato %v con lo stato %s (PO-35)", pv.Rif, pv.Verificato, pv.Stato)
	}
	if pv.Verificato == (len(pv.Motivi) > 0) {
		t.Errorf("prodotto %s: verificato %v con i motivi %v", pv.Rif, pv.Verificato, pv.Motivi)
	}
	switch pv.Stato {
	case valutazione.ProdottoNonPronto, valutazione.ProdottoProntoFattibilita, valutazione.ProdottoDaRiesaminare:
	default:
		t.Errorf("prodotto %s: stato %q", pv.Rif, pv.Stato)
	}
	if (!valutato || !pv.Smistamento.Calcolata) && pv.Stato != valutazione.ProdottoNonPronto {
		t.Errorf("prodotto %s: stato %s con il thread valutato %v e lo smistamento calcolato %v (T-B6-09, T-B6-61)", pv.Rif, pv.Stato, valutato,
			pv.Smistamento.Calcolata)
	}
	revisione := false
	for _, m := range pv.Motivi {
		revisione = revisione || m == valutazione.MotivoProdottoRevisioneTecnicaAperta
	}
	if revisione != (len(pv.Smistamento.Percorsi) > 0) {
		t.Errorf("prodotto %s: revisione_tecnica_aperta %v con i percorsi %v (T-E1-13)", pv.Rif, revisione, pv.Smistamento.Percorsi)
	}
	fonte := pv.Fonte.Calcolata && pv.Fonte.Stato == valutazione.FonteConfermata
	superata := pv.Fonte.Calcolata && pv.Fonte.Motivo == valutazione.MotivoFonteRiferimentoSuperato
	d, s := pv.Documenti, pv.Smistamento
	sei := valutazione.TargetConfermato(pv) && fonte && pv.BOM.Nomenclatura.Stato == valutazione.StatoAsseVerificata &&
		pv.BOM.Gerarchia.Stato == valutazione.StatoAsseVerificata && s.Stato == valutazione.SmistamentoVerificato && d.Stato == valutazione.DocumentiCompleta
	if pv.Verificato != sei {
		t.Errorf("prodotto %s: verificato %v, le sei condizioni %v (R79, R-84)", pv.Rif, pv.Verificato, sei)
	}
	if pv.Stato == valutazione.ProdottoDaRiesaminare {
		voci := true
		for _, v := range d.Voci {
			voci = voci && v.Esito == valutazione.EsitoPresente
		}
		perimetro := d.Stato == valutazione.DocumentiNonCalcolabile && d.Motivo == valutazione.MotivoDocumentiPerimetroAperto &&
			(d.MotivoPerimetro == valutazione.MotivoPerimetroRimozioniAperte || (superata && d.MotivoPerimetro == valutazione.MotivoPerimetroFonteNonConfermata))
		smistamento := s.Stato == valutazione.SmistamentoVerificato || (s.Calcolata && len(s.FileNonTerminali)+len(s.Conflitti)+len(s.Percorsi) > 0)
		if !valutazione.TargetConfermato(pv) || !(fonte || superata) || !smistamento || !voci || !(d.Stato == valutazione.DocumentiCompleta || perimetro) {
			t.Errorf("prodotto %s: da_riesaminare senza le precondizioni di T-E1-14 (R-84): fonte %+v, smistamento %+v, documenti %s, voci presenti %v",
				pv.Rif, pv.Fonte, s, statoDi(d), voci)
		}
	}
	t12 := false
	for _, k := range sezioniDellImpronta {
		t12 = t12 || assente(f, k)
	}
	if (t12 && pv.Impronta != "") || (!t12 && len(pv.Impronta) != 64) {
		t.Errorf("prodotto %s: impronta %q con T-12 %v", pv.Rif, pv.Impronta, t12)
	}
	if len(pv.Nodi) > 0 && (pv.Struttura != ancoraggio.StatoBOMDiLavoroProposta || pv.Nodi[0].Nodo.Rif == "") {
		t.Errorf("prodotto %s: %d nodi con la struttura %s (T-B6-11)", pv.Rif, len(pv.Nodi), pv.Struttura)
	}
}

// coerenzaDelleFonti: la «prova generale» di T-B2-02 (README di ancoraggio; resoconto della fase 1 di B5): per la fonte
// fanno fede i candidati di valutazione (FonteProdotto.Candidati), e StrutturaProdotto.Fonti è una vista per struttura con
// lo stesso criterio (R76 b A). Su ogni prodotto di ogni caso delle prove, con la fonte calcolata (con T-12, o senza poter
// confrontare le basi, T-B1-11, la fonte non si calcola e i suoi candidati non fanno fede):
//   - ogni fonte di una struttura del prodotto è fra i candidati della fonte, con lo stesso sha256 e la stessa radice
//     compatibile, qualunque sia l'origine del candidato (uno STEP corrente del prodotto è documento_del_prodotto per
//     valutazione e motore_a per ancoraggio). La sola eccezione dichiarata è lo STEP del riferimento del gesto 3 quando la
//     fonte non è confermata (superata): ancoraggio non ha la fonte confermata e lo propone, valutazione no (dubbio
//     T-B6-89; un limite scritto nel README: quella struttura è la «versione di prima» di R111). Con la fonte confermata
//     l'eccezione non vale (R-83 della revisione di V3);
//   - ogni candidato del motore A è la fonte di una struttura del prodotto, con lo stesso sha256 e la stessa radice.
func coerenzaDelleFonti(t *testing.T, prodotti []valutazione.ProdottoValutato, strutture []ancoraggio.StrutturaProdotto) {
	t.Helper()
	for _, pv := range prodotti {
		if !pv.Fonte.Calcolata {
			continue
		}
		delleStrutture := map[string]bool{}
		for _, s := range strutture {
			if s.Target != pv.Rif {
				continue
			}
			for _, d := range s.Fonti {
				k := d.Sha256 + "\x00" + d.RadiceCompatibile
				delleStrutture[k] = true
				if r := pv.Fonte.Riferimento; r != nil && r.Sha256 == d.Sha256 && pv.Fonte.Stato != valutazione.FonteConfermata {
					continue
				}
				trovato := false
				for _, c := range pv.Fonte.Candidati {
					trovato = trovato || c.Sha256+"\x00"+c.RadiceCompatibile == k
				}
				if !trovato {
					t.Errorf("prodotto %s: la fonte %s della struttura %s non è fra i candidati della fonte %+v (T-B2-02)", pv.Rif, d.Sha256[:8], s.Radice,
						pv.Fonte.Candidati)
				}
			}
		}
		for _, c := range pv.Fonte.Candidati {
			if c.Origine == ancoraggio.CandidatoDaMotoreA && !delleStrutture[c.Sha256+"\x00"+c.RadiceCompatibile] {
				t.Errorf("prodotto %s: il candidato del motore A %s non è la fonte di nessuna struttura (T-B2-02)", pv.Rif, c.Sha256[:8])
			}
		}
	}
}

// invariantiDelFascicolo: gli invarianti del fascicolo di un thread (B6, V3; R88, R89, R96; contratto §1.0; F0-06; LD-23):
//   - i conteggi, i pronti in ordine di Rif e i bloccati vengono dai prodotti valutati;
//   - congelabile se e solo se il thread è valutato, il fascicolo calcolato, e tutti i target, almeno uno, sono verificati;
//   - un fascicolo calcolato e non congelabile ha il suo motivo, uno dei quattro;
//   - calcolato se e solo se il thread è valutato (con la grammatica e senza un errore: R-86 della controprova di V3) e
//     non c'è T-12 sulle sezioni dei target, della versione della BOM e dello smistamento (dubbio T-B6-86; la nota di V2);
//   - Calcola non ha il gesto del modello nuovo: mai congelato, gesto_non_registrato, nessun conflitto di congelamento;
//   - la fase è quella del thread (LD-10); il congelamento legacy si mostra quando la fotografia ha una versione della BOM.
func invariantiDelFascicolo(t *testing.T, f fotorfq.Fotografia, et valutazione.EsitoThread, th fotorfq.Thread) {
	t.Helper()
	fa := et.Fascicolo
	var pronti []string
	for _, pv := range et.ProdottiValutati {
		if pv.Verificato {
			pronti = append(pronti, pv.Rif)
		}
	}
	sort.Strings(pronti)
	if fa.NumeroTarget != len(et.ProdottiValutati) || fa.NumeroVerificati != len(pronti) || strings.Join(fa.Pronti, " ") != strings.Join(pronti, " ") ||
		len(fa.Bloccati) != fa.NumeroTarget-fa.NumeroVerificati {
		t.Errorf("thread %s: fascicolo %+v per i prodotti %d, pronti %v", et.ThreadID, fa, len(et.ProdottiValutati), pronti)
	}
	congelabile := et.Valutato && fa.Calcolato && fa.NumeroTarget >= 1 && fa.NumeroVerificati == fa.NumeroTarget
	if fa.Congelabile != congelabile {
		t.Errorf("thread %s: congelabile %v, la regola dice %v (R96 a A)", et.ThreadID, fa.Congelabile, congelabile)
	}
	switch {
	case fa.Congelabile && fa.MotivoNonCongelabile != "":
		t.Errorf("thread %s: congelabile con il motivo %q", et.ThreadID, fa.MotivoNonCongelabile)
	case !fa.Congelabile && fa.Calcolato && fa.MotivoNonCongelabile == "":
		t.Errorf("thread %s: non congelabile senza motivo", et.ThreadID)
	}
	switch fa.MotivoNonCongelabile {
	case "", valutazione.NonCongelabileThreadNonValutato, valutazione.NonCongelabileNessunTarget, valutazione.NonCongelabileBOMVersioneNonVerificati,
		valutazione.NonCongelabileProdottiNonPronti:
	default:
		t.Errorf("thread %s: motivo %q fuori dai quattro (T-B6-12)", et.ThreadID, fa.MotivoNonCongelabile)
	}
	if !et.Valutato && fa.MotivoNonCongelabile != valutazione.NonCongelabileThreadNonValutato {
		t.Errorf("thread %s non valutato: motivo %q (T-B6-09)", et.ThreadID, fa.MotivoNonCongelabile)
	}
	t12 := false
	for _, k := range sezioniDelFascicoloCalcolato {
		t12 = t12 || assente(f, k)
	}
	if fa.Calcolato != (!t12 && et.Valutato) {
		t.Errorf("thread %s: fascicolo calcolato %v con T-12 %v e il thread valutato %v (T-B6-86)", et.ThreadID, fa.Calcolato, t12, et.Valutato)
	}
	if fa.Congelato || fa.CongelatoDa != nil || fa.CongelatoIl != nil || fa.MotivoNonCongelato != valutazione.NonCongelatoGestoNonRegistrato ||
		len(fa.ConflittiCongelamento) != 0 {
		t.Errorf("thread %s: il gesto del modello nuovo non c'è (LD-23): %+v", et.ThreadID, fa)
	}
	if fa.FaseThread != th.Stato || (fa.Legacy != nil) != (th.VersioneBOM != nil || th.UltimaCongelata != nil) {
		t.Errorf("thread %s: fase %q (il thread %q), legacy %+v", et.ThreadID, fa.FaseThread, th.Stato, fa.Legacy)
	}
}

// nodoBOMDi: il nodo della BOM di lavoro del prodotto con quel Rif; se non c'è, la prova fallisce.
func nodoBOMDi(t *testing.T, pv valutazione.ProdottoValutato, rif string) valutazione.NodoBOM {
	t.Helper()
	var rifs []string
	for _, n := range pv.Nodi {
		if n.Nodo.Rif == rif {
			return n
		}
		rifs = append(rifs, n.Nodo.Rif)
	}
	t.Fatalf("il nodo %s non c'è fra i nodi del prodotto %s: %v", rif, pv.Rif, rifs)
	return valutazione.NodoBOM{}
}
