package confronto

// CorrezioniManuali: la misura «prima e dopo» delle correzioni manuali sui file decisi (parte 1 §1.1, C-01; R30 f A),
// con la definizione di R114, precisata dall'utente il 07/10 (domande-a1c.md). È un indicatore ricostruito delle
// correzioni necessarie, non una misura del tempo risparmiato. Distingue la proposta del vecchio motore (la lettura
// registrata, Vecchio.CodiceLetto), la decisione di riferimento (il documento confermato: Codice, Base, Marcatore,
// Componente) e la proposta del nuovo (Nuovo), e conta «prima» e «dopo» sullo stesso campione.
//   - Decisi: i file decisi (un documento confermato li porta: deciso).
//   - Valutabili: il denominatore, cioè i decisi con «prima» e «dopo» tutti e due determinabili. La copertura è
//     Valutabili su Decisi. Ogni deciso è valutabile oppure escluso, con un motivo solo: Decisi = Valutabili +
//     Esclusi.Totale(). Un'informazione mancante non vale zero: esce dal denominatore e si conta.
//   - Esclusi: i decisi non valutabili, per motivo (EsclusiCorrezioni).
//   - Prima: sui valutabili, la base della lettura del vecchio motore non è la base del codice deciso (correzionePrima).
//   - PrimaMarcatore: sui valutabili, la stessa base con due marcatori scritti e diversi (per la grammatica, un'altra
//     identità: T-B4-30). Non si somma a Prima: l'utente non ha scelto se il titolo della misura sia la sola base o la
//     base con il marcatore (D-R114, aperta in domande-a1c.md). I due numeri restano separati e visibili.
//   - MarcatoreSoloDaUnLato: sui valutabili, la stessa base con il marcatore scritto da un lato solo. Per la regola di
//     nomina è compatibile, quindi non è una correzione; si conta perché resti visibile, e Prima non cambia.
//   - Dopo: sui valutabili, le correzioni ancora necessarie con la proposta del nuovo motore: FalseAssociazioni +
//     Ambiguita + Astensioni.
//   - FalseAssociazioni: il nuovo propone candidati, ma non il componente deciso con la sua base (un altro target o
//     un'altra base): il badge regressione_su_confermata con almeno un candidato.
//   - Ambiguita: il componente deciso con la sua base sta fra più candidati, o l'associazione è ambigua o discordante:
//     il badge resta uguale, ma un'alternativa dichiarata non è una scelta (R30 b A), e l'operatore deve ancora
//     scegliere.
//   - Astensioni: il nuovo non risponde sul file: nessun candidato (anche fuori richiesta o non determinabile), oppure
//     il file non valutato dentro un thread valutato (un documento che non si legge, un contenitore): il badge
//     regressione_su_confermata senza candidati.
//   - SoloRevisione: sui valutabili, la revisione contata a parte, cioè il badge uguale con l'indicatore di revisione
//     «diversa»: per la sola revisione non c'è mai una regressione (R31 b), e l'indicatore resta separato dalla
//     correttezza dell'associazione (R113).
//
// Con gli export la lettura del vecchio motore non c'è: i decisi escono tutti con senza_lettura_vecchia, e la misura
// dice che non si può calcolare invece di dare zero.
type CorrezioniManuali struct {
	Decisi                int               `json:"decisi"`
	Valutabili            int               `json:"valutabili"`
	Esclusi               EsclusiCorrezioni `json:"esclusi"`
	Prima                 int               `json:"prima"`
	PrimaMarcatore        int               `json:"prima_marcatore"`
	MarcatoreSoloDaUnLato int               `json:"marcatore_solo_da_un_lato"`
	Dopo                  int               `json:"dopo"`
	FalseAssociazioni     int               `json:"false_associazioni"`
	Ambiguita             int               `json:"ambiguita"`
	Astensioni            int               `json:"astensioni"`
	SoloRevisione         int               `json:"solo_revisione"`
}

// EsclusiCorrezioni: i decisi che non entrano nel denominatore, per motivo. Un file ha un motivo solo, il primo che
// vale: prima quelli di «prima» (correzionePrima: la proposta del vecchio motore), poi quelli di «dopo» (esclusoDopo:
// la decisione di riferimento e la proposta del nuovo). Tutti i campi escono nel JSON, anche a zero.
//   - SoloOrigineManuale: la proposta ha la sola fonte «operatore», senza la lettura del vecchio motore. La sola
//     origine manuale non dimostra un errore corretto (R114).
//   - SenzaLetturaVecchia: la lettura del vecchio motore non c'è (CodiceLetto vuoto: gli export, una versione dei
//     dettagli che non si conosce). Un'informazione storica mancante non vale zero (R114).
//   - CodiceNonLeggibile: la grammatica non legge la base da un lato, e le due stringhe sono diverse; oppure il nuovo
//     propone il componente deciso, ma la base del codice deciso non si legge e il badge non si può dire (il motivo
//     codice_registrato_non_leggibile). Mai un confronto per stringa (R31 c A): né un'uguaglianza inventata né un
//     conflitto artificiale (R113).
//   - DocumentoSenzaComponente: il documento deciso non ha componente (un documento commerciale del thread): il nuovo
//     ancora i file a prodotti e componenti, e con la decisione non c'è niente da mettere a fianco (T-B6-183).
//   - NuovoNonValutato: il motore A non ha valutato il thread del file (Nuovo.Motivo thread_non_valutato): il file è
//     fuori dalla copertura del nuovo. Un file non valutato dentro un thread valutato è invece un'astensione.
type EsclusiCorrezioni struct {
	SoloOrigineManuale       int `json:"solo_origine_manuale"`
	SenzaLetturaVecchia      int `json:"senza_lettura_vecchia"`
	CodiceNonLeggibile       int `json:"codice_non_leggibile"`
	DocumentoSenzaComponente int `json:"documento_senza_componente"`
	NuovoNonValutato         int `json:"nuovo_non_valutato"`
}

// Totale: i decisi esclusi, per tutti i motivi.
func (e EsclusiCorrezioni) Totale() int {
	return e.SoloOrigineManuale + e.SenzaLetturaVecchia + e.CodiceNonLeggibile + e.DocumentoSenzaComponente + e.NuovoNonValutato
}

// Aggiungi somma alla misura un'altra misura, campo per campo (per esempio le misure dei thread di un cliente nel
// rapporto del banco). Sta qui, accanto ai campi, perché un campo nuovo non resti fuori dalla somma (T-B6-186).
func (c *CorrezioniManuali) Aggiungi(altra CorrezioniManuali) {
	c.Decisi += altra.Decisi
	c.Valutabili += altra.Valutabili
	c.Esclusi.SoloOrigineManuale += altra.Esclusi.SoloOrigineManuale
	c.Esclusi.SenzaLetturaVecchia += altra.Esclusi.SenzaLetturaVecchia
	c.Esclusi.CodiceNonLeggibile += altra.Esclusi.CodiceNonLeggibile
	c.Esclusi.DocumentoSenzaComponente += altra.Esclusi.DocumentoSenzaComponente
	c.Esclusi.NuovoNonValutato += altra.Esclusi.NuovoNonValutato
	c.Prima += altra.Prima
	c.PrimaMarcatore += altra.PrimaMarcatore
	c.MarcatoreSoloDaUnLato += altra.MarcatoreSoloDaUnLato
	c.Dopo += altra.Dopo
	c.FalseAssociazioni += altra.FalseAssociazioni
	c.Ambiguita += altra.Ambiguita
	c.Astensioni += altra.Astensioni
	c.SoloRevisione += altra.SoloRevisione
}

// I motivi di esclusione, come li restituiscono correzionePrima ed esclusoDopo: sono i tag di EsclusiCorrezioni.
const (
	esclusoSoloOrigineManuale       = "solo_origine_manuale"
	esclusoSenzaLetturaVecchia      = "senza_lettura_vecchia"
	esclusoCodiceNonLeggibile       = "codice_non_leggibile"
	esclusoDocumentoSenzaComponente = "documento_senza_componente"
	esclusoNuovoNonValutato         = "nuovo_non_valutato"
)

// motivoFileThreadNonValutato: il motivo di valutazione per i file di un thread che il motore A non valuta
// (valutazione.MotivoFileThreadNonValutato), riscritto qui come stringa perché confronto non importa valutazione.
const motivoFileThreadNonValutato = "thread_non_valutato"

// esitoPrima: i tre esiti di «prima» per un file deciso (R114).
type esitoPrima int

const (
	primaNessunaCorrezione esitoPrima = iota
	primaCorrezione
	primaNonDeterminabile
)

// correzioniDi: la misura sui file già confrontati. Per ogni deciso, nell'ordine, «prima» (correzionePrima) e «dopo»
// (esclusoDopo): il primo motivo che vale lo esclude. Sui valutabili si contano prima, i marcatori, dopo con le sue
// tre parti e la sola revisione.
func correzioniDi(righe []EsitoFile) CorrezioniManuali {
	var c CorrezioniManuali
	for _, r := range righe {
		v, n := r.File.Vecchio, r.File.Nuovo
		if !deciso(v) {
			continue
		}
		c.Decisi++
		prima, motivo := correzionePrima(v)
		if prima != primaNonDeterminabile {
			motivo = esclusoDopo(r)
		}
		if motivo != "" {
			c.Esclusi.aggiungi(motivo)
			continue
		}
		c.Valutabili++
		switch diversi, soloDaUnLato := marcatori(v); {
		case prima == primaCorrezione:
			c.Prima++
		case diversi:
			c.PrimaMarcatore++
		case soloDaUnLato:
			c.MarcatoreSoloDaUnLato++
		}
		switch {
		case r.Badge == BadgeRegressioneSuConfermata && (!n.Valutato || len(n.Candidati) == 0):
			c.Astensioni++
		case r.Badge == BadgeRegressioneSuConfermata:
			c.FalseAssociazioni++
		case r.Badge == BadgeUguale && ambiguo(n):
			c.Ambiguita++
		}
		if r.Badge == BadgeUguale && r.Revisione.Valore == RevisioneDiversa {
			c.SoloRevisione++
		}
	}
	c.Dopo = c.FalseAssociazioni + c.Ambiguita + c.Astensioni
	return c
}

// aggiungi: un escluso in più per il motivo dato. I motivi sono solo quelli di correzionePrima ed esclusoDopo.
func (e *EsclusiCorrezioni) aggiungi(motivo string) {
	switch motivo {
	case esclusoSoloOrigineManuale:
		e.SoloOrigineManuale++
	case esclusoSenzaLetturaVecchia:
		e.SenzaLetturaVecchia++
	case esclusoCodiceNonLeggibile:
		e.CodiceNonLeggibile++
	case esclusoDocumentoSenzaComponente:
		e.DocumentoSenzaComponente++
	case esclusoNuovoNonValutato:
		e.NuovoNonValutato++
	}
}

// correzionePrima: la decisione corregge la proposta del vecchio motore? Tre esiti (R114; R30 f A, R31 c A; F0-19):
//   - senza la lettura del vecchio motore (CodiceLetto vuoto) non si può dire: solo_origine_manuale se la proposta ha la
//     fonte «operatore» (l'ha scritta una persona, e la sola origine manuale non dimostra un errore corretto), altrimenti
//     senza_lettura_vecchia. La fonte da sola non è mai una correzione;
//   - con le due basi lette dalla grammatica del cliente (CodiceLettoBase e Base), è una correzione se sono diverse.
//     «7120100A» e «7120100» hanno la stessa base, e non sono una correzione: il marcatore si conta a parte
//     (marcatori);
//   - se una delle due basi non si legge, due stringhe identiche restano «nessuna correzione»; altrimenti non si può
//     dire (codice_non_leggibile). Il ripiego sul confronto per stringa non c'è più: una base che non c'è non è uguale
//     a niente e non è diversa da niente (R113: né un'uguaglianza inventata né un conflitto artificiale).
//
// Il motivo è "" se l'esito si può dire.
func correzionePrima(v Vecchio) (esitoPrima, string) {
	switch {
	case v.CodiceLetto == "" && v.Fonte == fonteOperatore:
		return primaNonDeterminabile, esclusoSoloOrigineManuale
	case v.CodiceLetto == "":
		return primaNonDeterminabile, esclusoSenzaLetturaVecchia
	case v.CodiceLettoBase != "" && v.Base != "":
		if v.CodiceLettoBase != v.Base {
			return primaCorrezione, ""
		}
		return primaNessunaCorrezione, ""
	case v.CodiceLetto == v.Codice:
		return primaNessunaCorrezione, ""
	}
	return primaNonDeterminabile, esclusoCodiceNonLeggibile
}

// marcatori: con la stessa base letta da tutte e due le parti, i marcatori della lettura del vecchio motore
// (CodiceLettoMarcatore) e del codice deciso (Marcatore), come li dà la grammatica: diversi se sono scritti tutti e due
// e non coincidono (T-B4-30: un'altra identità), soloDaUnLato se ce n'è uno solo (compatibile, come nella regola di
// nomina). Il confronto è quello della regola, sui valori letti, senza normalizzare niente. Senza le due basi lette e
// uguali i marcatori non si guardano.
func marcatori(v Vecchio) (diversi, soloDaUnLato bool) {
	if v.CodiceLettoBase == "" || v.CodiceLettoBase != v.Base {
		return false, false
	}
	a, b := v.CodiceLettoMarcatore, v.Marcatore
	switch {
	case a != "" && b != "":
		return a != b, false
	case a != "" || b != "":
		return false, true
	}
	return false, false
}

// esclusoDopo: il motivo per cui «dopo» non si può dire su un file deciso, "" se si può:
//   - il documento deciso non ha componente: documento_senza_componente (T-B6-183);
//   - il motore A non ha valutato il thread: nuovo_non_valutato;
//   - il nuovo propone il componente deciso, ma la base del codice deciso non si legge (il badge dice regressione con
//     il motivo codice_registrato_non_leggibile, perché una base vuota non è uguale a niente): codice_non_leggibile.
//
// Un file non valutato per un altro motivo, in un thread valutato, si può dire: è un'astensione.
func esclusoDopo(r EsitoFile) string {
	v, n := r.File.Vecchio, r.File.Nuovo
	switch {
	case v.Componente == nil:
		return esclusoDocumentoSenzaComponente
	case !n.Valutato && n.Motivo == motivoFileThreadNonValutato:
		return esclusoNuovoNonValutato
	case r.Badge == BadgeRegressioneSuConfermata && r.Motivo == MotivoCodiceRegistratoNonLeggibile:
		return esclusoCodiceNonLeggibile
	}
	return ""
}

// ambiguo: il nuovo non sceglie fra più destinazioni: più di un candidato, o l'associazione ambigua o discordante
// (ancoraggio: «più candidati, nessuna scelta»; «più identità che non concordano»).
func ambiguo(n Nuovo) bool {
	return len(n.Candidati) > 1 || n.Associazione == associazioneAmbiguo || n.Associazione == associazioneDiscordante
}
