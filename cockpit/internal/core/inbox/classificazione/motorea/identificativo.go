package motorea

import (
	"sort"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// L'identificativo verso il cliente (V1, emendamento U2 §4.2 punto 4; C-01 [U], D31 §8 [U]): la stringa che il cliente
// usa per un componente, composta da codice e revisione DECISI con la forma attiva su cartiglio.codice della sua
// grammatica. È un valore calcolato alla lettura, mai salvato, mai un'identità: codice e revisione restano due campi.
// Mai la concatenazione come regola universale: la forma la dà la grammatica del cliente; senza una forma che si possa
// comporre dai due campi l'identificativo è «non determinato», con il motivo, mai una stringa inventata.
//
// ComponiCodiceDocumentale compone da una lettura; qui i due campi decisi non sono una lettura: il codice si rilegge con
// le forme della grammatica (le letture intere, complete e senza revisione del namespace dato), la revisione decisa
// prende il posto della revisione della forma del cartiglio, e la composizione e la rilettura sono quelle di
// ComponiCodiceDocumentale, con le stesse regole parte per parte. Nessuna regola per cliente nel codice.

// I motivi dell'identificativo non determinato, oltre a quelli di ComponiCodiceDocumentale (MotivoComposizione*): non
// sono codici di diagnostica, come quelli.
const (
	// Nessuna forma della grammatica legge il codice deciso per intero, completo e senza revisione (nel namespace dato).
	MotivoIdentificativoCodiceNonLetto = "codice_non_letto"
	// Il codice deciso si legge con forme di famiglie diverse: il compositore non sceglie.
	MotivoIdentificativoFamiglieMultiple = "famiglie_multiple"
)

// ComponiIdentificativo: l'identificativo verso il cliente da codice e revisione decisi (rev "" = senza revisione) e
// dal namespace dell'identità ("" = ogni namespace). Le regole:
//   - il codice si rilegge con Riconosci su ogni selettore con forme attive (in ordine di selettore): valgono le letture
//     che coprono tutto il codice, complete, con la base completa, senza revisione e, se dato, nel namespace;
//   - le letture devono essere di una famiglia sola, che deve avere una sola forma attiva su cartiglio.codice
//     (MotivoComposizioneFormaAssente, MotivoComposizioneFormeMultiple);
//   - con la revisione decisa la lettura riceve la revisione della regola della forma del cartiglio, letta, con il
//     valore deciso com'è; una forma senza revisione non le dà un posto (qualificatore_non_trasferibile);
//   - per ogni lettura la composizione e la rilettura di ComponiCodiceDocumentale; tutte devono dare lo stesso testo,
//     altrimenti l'identificativo non è determinato in modo univoco (parte_non_determinata).
//
// È deterministica e pura come ComponiCodiceDocumentale. Un motore nullo non compone niente.
func (m *Motore) ComponiIdentificativo(codice, rev, namespace string) CodiceComposto {
	var c CodiceComposto
	if m == nil || codice == "" {
		c.Motivo = MotivoIdentificativoCodiceNonLetto
		return c
	}
	selettori := make([]evidenze.Selettore, 0, len(m.piani))
	for s := range m.piani {
		selettori = append(selettori, s)
	}
	sort.Slice(selettori, func(i, j int) bool { return selettori[i].String() < selettori[j].String() })
	var letture []LetturaForma
	famiglie := map[string]bool{}
	for _, s := range selettori {
		ll, _ := m.Riconosci(s, codice)
		for _, l := range ll {
			if l.Intervallo.Inizio != 0 || l.Intervallo.Fine != len(codice) || l.Stato != StatoCompleta || !l.Base.Completa ||
				len(l.Base.Mancanti) > 0 || l.Revisione != nil || (namespace != "" && l.Namespace != namespace) {
				continue
			}
			letture = append(letture, l)
			famiglie[l.Famiglia] = true
		}
	}
	switch {
	case len(letture) == 0:
		c.Motivo = MotivoIdentificativoCodiceNonLetto
		return c
	case len(famiglie) > 1:
		c.Motivo = MotivoIdentificativoFamiglieMultiple
		return c
	}
	c.Famiglia = letture[0].Famiglia
	forme := m.formeDocumentali(c.Famiglia)
	switch {
	case len(forme) == 0:
		c.Motivo = MotivoComposizioneFormaAssente
		return c
	case len(forme) > 1:
		c.Motivo = MotivoComposizioneFormeMultiple
		return c
	}
	p := forme[0]
	c.Forma = p.forma
	regolaRev := ""
	for _, pt := range p.scrittura {
		if pt.tipo == grammatica.TipoParteRevisione {
			regolaRev = pt.rif
			break
		}
	}
	testo := ""
	for _, l := range letture {
		if rev != "" {
			l.Revisione = &RevisioneLetta{Regola: regolaRev, Originale: rev, Normalizzata: rev, Stato: StatoRevisioneLetta}
		}
		t, motivo := componi(p, m.formaDellaLettura(l), l)
		if motivo == "" && !m.rilegge(p, t, l) {
			motivo = MotivoComposizioneParteNonDeterminata
		}
		if motivo == "" && testo != "" && t != testo {
			motivo = MotivoComposizioneParteNonDeterminata
		}
		if motivo != "" {
			c.Motivo = motivo
			return c
		}
		testo = t
	}
	c.Testo = testo
	return c
}
