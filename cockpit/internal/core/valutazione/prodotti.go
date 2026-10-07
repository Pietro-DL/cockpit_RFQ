package valutazione

import (
	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
)

// prodotti.go: lo stato del prodotto, l'asse 7, e il composto (R78, R79, R94 A precisata [R]; T-B0-21, T-B0-23, T-E1-01,
// T-E1-13, T-E1-14, T-E1R-04; contratto §0.3, §1.0 riga 7, §1.7, §2.3, §2.5; fase 0 di B6, F.2, F.3, F.4). La regola
// (StatoDelProdotto) è una funzione pura sul prodotto con i suoi assi e su un ingresso astratto (CondizioniNuove: T-B0-22);
// l'adattatore (condizioniNuoveDellaBOM) lo prepara dai gesti e dalla struttura della verifica della BOM, con R111 A,
// precisata dall'utente il 07/10, dopo una fonte superata (vociNuoveDopoLaFonteSuperataR111). Mai «verificato» per
// difetto.

// StatoProdotto: l'asse 7 (contratto §2.3). pronto_fattibilita se e solo se il prodotto è verificato (T-B0-23); il
// confine fra da_riesaminare e non_pronto è T-E1-14.
type StatoProdotto string

const (
	ProdottoNonPronto         StatoProdotto = "non_pronto"
	ProdottoProntoFattibilita StatoProdotto = "pronto_fattibilita"
	ProdottoDaRiesaminare     StatoProdotto = "da_riesaminare"
)

// MotivoProdotto: perché un prodotto non è verificato (contratto §2.3, nove valori; §2.5, +1).
type MotivoProdotto string

const (
	MotivoProdottoTargetNonConfermato      MotivoProdotto = "target_non_confermato"
	MotivoProdottoFonte                    MotivoProdotto = "fonte_strutturale_step_mancante_o_non_confermata"
	MotivoProdottoFonteSuperata            MotivoProdotto = "fonte_superata"
	MotivoProdottoNomenclatura             MotivoProdotto = "nomenclatura_non_verificata"
	MotivoProdottoGerarchia                MotivoProdotto = "gerarchia_non_verificata"
	MotivoProdottoSmistamento              MotivoProdotto = "smistamento_non_verificato"
	MotivoProdottoDocumentazioneIncompleta MotivoProdotto = "documentazione_incompleta"
	MotivoProdottoDocumentazioneNonCalc    MotivoProdotto = "documentazione_non_calcolabile"
	MotivoProdottoConflitto                MotivoProdotto = "conflitto"
	MotivoProdottoRevisioneTecnicaAperta   MotivoProdotto = "revisione_tecnica_aperta"
)

// CondizioniNuove: l'ingresso di StatoDelProdotto che il prodotto da solo non porta (T-E1-14; F.4): se la nomenclatura e
// la gerarchia, quando non sono verificate, sono ferme solo per condizioni «nuove» nella fotografia. Per saperlo servono i
// gesti e la struttura della verifica: un asse in conflitto nasconde il suo stato di base, e le voci da decidere si
// classificano solo con le strutture (R111). Gli altri assi (fonte, smistamento, completezza) si leggono dal prodotto.
// In A1c lo prepara l'adattatore (condizioniNuoveDellaBOM); le prove della regola lo scrivono a mano.
type CondizioniNuove struct {
	Nomenclatura bool `json:"nomenclatura"`
	Gerarchia    bool `json:"gerarchia"`
}

// StatoDelProdotto: la regola del composto e dello stato del prodotto (R79; contratto §0.3 e §1.0; T-E1-01, T-E1-14).
// È pura e deterministica.
//
//	ProdottoVerificato = TargetRFQConfermato AND FonteSTEPConfermata AND NomenclaturaBOMVerificata
//	                 AND GerarchiaBOMVerificata AND SmistamentoVerificato AND DocumentazioneCompleta
//	Stato = pronto_fattibilita  se e solo se ProdottoVerificato (un solo predicato: T-E1-01)
//	      = da_riesaminare      se il prodotto non è verificato e mancano solo condizioni nuove (T-E1-14):
//	                            l'identità è confermata; la fonte è confermata o superata; la completezza è completa, o
//	                            non_calcolabile solo per il perimetro aperto da una condizione nuova, con ogni voce certa
//	                            presente (a perimetro chiuso sarebbe completa); nomenclatura, gerarchia e smistamento sono
//	                            verificati, o fermi solo per condizioni nuove
//	      = non_pronto          altrimenti
//
// Sono nuove (T-E1-14): un conflitto aperto, un file pertinente non terminale, la fonte superata, una rimozione proposta
// aperta o un arco in conflitto sul perimetro, un percorso di revisione aperto; con la lettura A di K-02 (T-E1R-04) la
// gerarchia ferma solo per fonte_non_confermata dopo una fonte superata; con R111 A, precisata il 07/10, le voci da
// decidere che vengono solo dal nuovo STEP (nell'ingresso nuove). Lo smistamento con Calcolata falso non è mai fermo solo
// per condizioni nuove: un thread senza grammatica, un target senza componente, T-12 danno non_pronto (T-B6-09, T-B6-61).
//
// I motivi, uno per ogni condizione che manca, nell'ordine dei valori (motiviDelProdotto); vuoti per un prodotto verificato.
func StatoDelProdotto(pv ProdottoValutato, nuove CondizioniNuove) (bool, StatoProdotto, []MotivoProdotto) {
	target := TargetConfermato(pv)
	fonte := pv.Fonte.Calcolata && pv.Fonte.Stato == FonteConfermata
	nomenclatura := pv.BOM.Nomenclatura.Stato == StatoAsseVerificata
	gerarchia := pv.BOM.Gerarchia.Stato == StatoAsseVerificata
	smistamento := pv.Smistamento.Stato == SmistamentoVerificato
	documenti := pv.Documenti.Stato == DocumentiCompleta
	if target && fonte && nomenclatura && gerarchia && smistamento && documenti {
		return true, ProdottoProntoFattibilita, nil
	}
	superata := fonteSuperata(pv.Fonte)
	motivi := motiviDelProdotto(pv, target, fonte, superata)
	if target && (fonte || superata) &&
		(nomenclatura || nuove.Nomenclatura) && (gerarchia || nuove.Gerarchia) &&
		(smistamento || smistamentoFermoPerCondizioniNuove(pv.Smistamento)) && (documenti || documentiFermiPerCondizioniNuove(pv.Documenti, superata)) {
		return false, ProdottoDaRiesaminare, motivi
	}
	return false, ProdottoNonPronto, motivi
}

// motiviDelProdotto: i motivi di un prodotto non verificato, nell'ordine dei valori del contratto (§2.3, §2.5):
//   - target_non_confermato senza TargetConfermato;
//   - fonte_superata con la fonte superata (lo STEP confermato sostituito: R65 A, riferimento_superato), altrimenti
//     fonte_strutturale_step_mancante_o_non_confermata senza la fonte confermata (anche non calcolata: T-12);
//   - nomenclatura_non_verificata, gerarchia_non_verificata, smistamento_non_verificato per un asse non verificato;
//   - documentazione_incompleta, oppure documentazione_non_calcolabile per ogni altro stato che non è completa;
//   - conflitto con un conflitto aperto su un asse (nomenclatura, gerarchia, smistamento: R95 A);
//   - revisione_tecnica_aperta con un percorso di revisione aperto: sta sempre fra i motivi (T-E1-13).
func motiviDelProdotto(pv ProdottoValutato, target, fonte, superata bool) []MotivoProdotto {
	var out []MotivoProdotto
	if !target {
		out = append(out, MotivoProdottoTargetNonConfermato)
	}
	switch {
	case superata:
		out = append(out, MotivoProdottoFonteSuperata)
	case !fonte:
		out = append(out, MotivoProdottoFonte)
	}
	if pv.BOM.Nomenclatura.Stato != StatoAsseVerificata {
		out = append(out, MotivoProdottoNomenclatura)
	}
	if pv.BOM.Gerarchia.Stato != StatoAsseVerificata {
		out = append(out, MotivoProdottoGerarchia)
	}
	if pv.Smistamento.Stato != SmistamentoVerificato {
		out = append(out, MotivoProdottoSmistamento)
	}
	switch pv.Documenti.Stato {
	case DocumentiCompleta:
	case DocumentiIncompleta:
		out = append(out, MotivoProdottoDocumentazioneIncompleta)
	default:
		out = append(out, MotivoProdottoDocumentazioneNonCalc)
	}
	if conflittoAperto(pv) {
		out = append(out, MotivoProdottoConflitto)
	}
	if len(pv.Smistamento.Percorsi) > 0 {
		out = append(out, MotivoProdottoRevisioneTecnicaAperta)
	}
	return out
}

// conflittoAperto: un asse del prodotto ha un conflitto aperto (R95 A; T-B0-24, T-B0-25, T-E1R-08).
func conflittoAperto(pv ProdottoValutato) bool {
	b, s := pv.BOM, pv.Smistamento
	return b.Nomenclatura.Stato == StatoAsseConflitto || len(b.Nomenclatura.Conflitti) > 0 ||
		b.Gerarchia.Stato == StatoAsseConflitto || len(b.Gerarchia.Conflitti) > 0 ||
		s.Stato == SmistamentoConflitto || len(s.Conflitti) > 0
}

// fonteSuperata: la fonte è superata, cioè lo STEP confermato è stato sostituito (R65 A: riferimento_superato, in attesa
// di conferma; T-E1-14 la conta fra le condizioni nuove).
func fonteSuperata(f FonteProdotto) bool {
	return f.Calcolata && f.Motivo == MotivoFonteRiferimentoSuperato
}

// smistamentoFermoPerCondizioniNuove: lo smistamento, calcolato e non verificato, è fermo solo per condizioni nuove
// (T-E1-14): un percorso aperto (in_revisione), un conflitto aperto, i file pertinenti non terminali. L'altra ragione
// della regola (Smistamento), le voci certe con un documento presente non confermato (T-B6-08), qui non si guarda: una
// voce certa non presente, di qualunque motivo e con qualunque file, tiene il prodotto non_pronto per la completezza
// (documentiFermiPerCondizioniNuove), con il perimetro chiuso o aperto. Così il giudizio sulle voci sta in un posto solo,
// uguale per tutti i motivi (R-81 della revisione di V3, che toglie l'asimmetria di T-B6-82). Con Calcolata falso mai
// (T-B6-07, T-B6-61, T-12).
func smistamentoFermoPerCondizioniNuove(s VerificaSmistamento) bool {
	return s.Calcolata && len(s.FileNonTerminali)+len(s.Conflitti)+len(s.Percorsi) > 0
}

// documentiFermiPerCondizioniNuove: la completezza non_calcolabile solo perché il perimetro è aperto per una condizione
// nuova (T-E1-14): una rimozione proposta aperta sul perimetro, oppure la fonte non confermata dopo una fonte superata
// (le note di B5: MotivoPerimetro dice quale condizione); e senza mancanze certe, cioè con ogni voce certa presente: a
// perimetro chiuso sarebbe completa (R94 A: «tutto il resto ci sarebbe»; R-81 della revisione di V3). Una voce certa non
// presente per una ragione vecchia (il 2D confermato e mai analizzato, il documento non confermato, il formato non
// configurato) tiene il prodotto non_pronto anche dietro il perimetro aperto, come a perimetro chiuso (incompleta); una
// voce che manca dà già incompleta (StatoDellaCompletezza). Un perimetro aperto per altro (nodi o archi da decidere, la
// BOM di lavoro assente, il target senza componente) o non determinabile (T-12) non lo è.
func documentiFermiPerCondizioniNuove(d CompletezzaDocumentale, superata bool) bool {
	if d.Stato != DocumentiNonCalcolabile || d.Motivo != MotivoDocumentiPerimetroAperto {
		return false
	}
	for _, v := range d.Voci {
		if v.Esito != EsitoPresente {
			return false
		}
	}
	switch d.MotivoPerimetro {
	case MotivoPerimetroRimozioniAperte:
		return true
	case MotivoPerimetroFonteNonConfermata:
		return superata
	}
	return false
}

// ---- l'adattatore delle condizioni nuove ----

// condizioniNuoveDellaBOM: l'ingresso CondizioniNuove di un prodotto, dai gesti e dalla struttura della verifica della BOM
// (B5, fase 1), con lo stato di base degli assi senza i conflitti (un conflitto aperto è una condizione nuova, e lo stato
// conflitto nasconde quello che resta sotto). Un asse non verificato è fermo solo per condizioni nuove quando il suo stato
// di base:
//   - è verificata: lo fermano solo i conflitti;
//   - è da_verificare con qualcosa da decidere (da_decidere), e tutte le voci da decidere vengono solo dal nuovo STEP dopo
//     una fonte superata (vociNuoveDopoLaFonteSuperataR111: R111 A, precisata dall'utente il 07/10);
//   - per la gerarchia, è da_verificare per fonte_non_confermata dopo una fonte superata (K-02, T-E1R-04, T-E1-14);
//   - per la gerarchia del modello nuovo, è da_verificare per nomenclatura_radice_non_verificata e la nomenclatura è
//     ferma solo per condizioni nuove (R80: la gerarchia vuota aspetta la nomenclatura della radice).
//
// Ogni altro stato di base (nessun gesto, non_verificabile, nessuna struttura, la BOM di lavoro assente) non è una
// condizione nuova.
func condizioniNuoveDellaBOM(pv ProdottoValutato, g GestiVerificaBOM, s StrutturaDaVerificare, strutture []ancoraggio.StrutturaProdotto, t fotorfq.Thread) CondizioniNuove {
	senza := s
	senza.Conflitti = nil
	base := VerificaDellaBOM(g, senza)
	vociNom, vociGer := vociDaDecidere(g, s)
	nuove := func(a VerificaAsse, voci []string) bool {
		switch {
		case a.Stato == StatoAsseVerificata:
			return true
		case a.Stato == StatoAsseDaVerificare && a.Motivo == MotivoAsseDaDecidere:
			return vociNuoveDopoLaFonteSuperataR111(voci, pv, strutture, t)
		}
		return false
	}
	var out CondizioniNuove
	out.Nomenclatura = nuove(base.Nomenclatura, vociNom)
	switch {
	case base.Gerarchia.Stato == StatoAsseDaVerificare && base.Gerarchia.Motivo == MotivoAsseFonteNonConfermata:
		out.Gerarchia = fonteSuperata(pv.Fonte)
	case base.Gerarchia.Stato == StatoAsseDaVerificare && base.Gerarchia.Motivo == MotivoAsseNomenclaturaRadice:
		out.Gerarchia = out.Nomenclatura
	default:
		out.Gerarchia = nuove(base.Gerarchia, vociGer)
	}
	return out
}

// vociNuoveDopoLaFonteSuperataR111: R111 A, precisata dall'utente il 07/10 (domande-a1c.md). Lo stato non cambia in questo
// giro; cambia solo come la regola si dichiara.
//
// Che cosa calcola: le voci della nuova fonte, cioè gli effetti della fonte superata, che T-E1-14 conta come condizioni
// nuove («gli effetti della stessa fonte superata possono motivare un riesame; le mancanze indipendenti già presenti non
// devono essere nascoste»). Non calcola le voci semanticamente nuove: fra lo STEP di prima e il nuovo STEP oggi non c'è
// nessuna corrispondenza, perché le chiavi dei nodi e degli archi contengono lo sha256 del documento (T-E1-04, una chiave
// valida solo dentro un file). Il confronto semantico che l'utente chiede (stessi padri e figli, occorrenze e quantità;
// gli affissi e le revisioni prima dei codici nuovi; un criterio di corrispondenza esplicito, con i casi ambigui lasciati
// irrisolti) esce da A1c come limite dichiarato (README): con la decisione dell'orchestratore del 07/10 sera (E2 §3.4,
// un contrasto isolato con R111, dichiarato lì) è il primo pezzo dopo A1c, prerequisito del riesame nello spazio di
// verifica, con una funzione a parte.
//
// Vero quando tutte le voci (i Rif di vociDaDecidere) sono della nuova fonte:
//  1. la fonte del prodotto è superata, con il riferimento del gesto 3 (lo STEP confermato prima, il suo sha256);
//  2. il nuovo STEP è il documento corrente in fondo alla catena delle sostituzioni del documento del riferimento
//     (DocumentoConfermato.SostituitoDa, nella fotografia: il caricatore legge tutti i documenti del thread, anche quelli
//     sostituiti, e i fatti dei loro contenuti alla terna corrente); una catena rotta o un ciclo senza un documento
//     corrente non hanno un nuovo STEP; un riferimento non sostituito è il nuovo STEP di sé stesso, e allora le due
//     versioni coincidono e nessuna voce è nuova;
//  3. la fotografia porta tutte e due le versioni: il prodotto ha una struttura dello STEP di prima e una del nuovo STEP
//     (ancoraggio le costruisce dai file del thread con i fatti letti; senza BOM di lavoro sono candidate). Se non le
//     porta, le voci da decidere non sono mai condizioni nuove, un limite dichiarato (README): il prodotto è
//     da_riesaminare solo se gli assi sono già verificati o fermi per le condizioni nuove che T-E1-14 elenca;
//  4. ogni voce è un nodo o un arco del nuovo STEP che lo STEP di prima non aveva: una riga di componente_proposta vale
//     per il suo nodo (sha256, chiave), una riga di relazione_proposta per il suo arco (lo sha256 del suo allegato, le
//     due chiavi), un RifNodo e un RifArco per sé stessi. I nodi e gli archi si confrontano con le chiavi del contratto
//     (lo sha256 e la chiave: T-E1-04), mai con il codice. Una relazione confermata che il segno non copre, una riga di
//     un'altra struttura, una voce che non si riconosce non sono nuove.
//
// La regola effettiva (R-85 della revisione di V3). Con le chiavi che comprendono lo sha256, ogni nodo e ogni arco del
// nuovo STEP è della nuova fonte, anche con lo stesso codice, la stessa chiave o la stessa struttura dello STEP di prima:
// con le due strutture leggibili, dopo una fonte superata, sono nuove tutte e sole le voci del nuovo STEP. Lo STEP di
// prima serve solo a tre cose: la sua struttura deve esserci (altrimenti vale il limite del punto 3); le sue voci, e
// quelle di un terzo STEP, restano vecchie; con lo stesso sha256 (lo stesso contenuto) nessuna voce è nuova.
//
// La conseguenza per l'utente (D-2 della revisione di V3): la finestra è stretta. Con il gesto 3 sul nuovo STEP la fonte
// torna confermata, R111 non vale più, e le voci ancora da decidere tornano condizioni vecchie: il prodotto passa da
// da_riesaminare a non_pronto, e resta non_pronto finché le voci nuove non sono decise, dopo un gesto che fa avanzare. Il
// riesame calcolato non apre da solo un percorso di revisione tecnica (in A1c i percorsi non ci sono: LD-18).
func vociNuoveDopoLaFonteSuperataR111(voci []string, pv ProdottoValutato, strutture []ancoraggio.StrutturaProdotto, t fotorfq.Thread) bool {
	rif := pv.Fonte.Riferimento
	if !fonteSuperata(pv.Fonte) || rif == nil {
		return false
	}
	documenti := map[uuid.UUID]*fotorfq.DocumentoConfermato{}
	for i := range t.Documenti {
		documenti[t.Documenti[i].ID] = &t.Documenti[i]
	}
	nuovo, visti := documenti[rif.DocumentoID], map[uuid.UUID]bool{}
	for nuovo != nil && nuovo.SostituitoDa != nil && !visti[nuovo.ID] {
		visti[nuovo.ID] = true
		nuovo = documenti[*nuovo.SostituitoDa]
	}
	if nuovo == nil || nuovo.SostituitoDa != nil {
		return false
	}
	elementi := func(sha string) map[string]bool {
		out := map[string]bool{}
		for _, st := range strutture {
			if st.Target != pv.Rif || st.Sha256 != sha {
				continue
			}
			for _, n := range st.Nodi {
				out[n.Rif] = true
			}
			for _, a := range st.Archi {
				out[RifArco(a.Padre, a.Figlio)] = true
			}
		}
		return out
	}
	prima, dopo := elementi(rif.Sha256), elementi(nuovo.Sha256)
	if len(prima) == 0 || len(dopo) == 0 {
		return false
	}
	sha := map[uuid.UUID]string{}
	for _, a := range t.Allegati {
		if a.Sha256 != nil {
			sha[a.ID] = *a.Sha256
		}
	}
	elementoDi := map[string]string{}
	for _, r := range t.RigheComponenteProposta {
		elementoDi[ancoraggio.RifRigaProposta(r.ID)] = ancoraggio.RifNodo(r.Sha256, r.Chiave)
	}
	for _, r := range t.RigheRelazioneProposta {
		s := sha[r.AllegatoID]
		elementoDi[rigaArco(r.AllegatoID, r.PadreChiave, r.FiglioChiave)] = RifArco(ancoraggio.RifNodo(s, r.PadreChiave), ancoraggio.RifNodo(s, r.FiglioChiave))
	}
	for _, v := range voci {
		e, ok := elementoDi[v]
		if !ok {
			e = v // un RifNodo o un RifArco vale per sé; ogni altra voce non è un elemento del nuovo STEP
		}
		if !dopo[e] || prima[e] {
			return false
		}
	}
	return true
}
