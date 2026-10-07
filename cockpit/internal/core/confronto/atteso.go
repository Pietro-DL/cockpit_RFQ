package confronto

import (
	"bytes"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// ---- l'atteso: lo riempie solo il runner ----

// Atteso: l'oracolo di un thread, come il runner del banco lo traduce dagli attesi (piano A, par.3.3.8; R2). Il motore
// non lo vede mai, l'anteprima non ce l'ha (P-14): confronto lo riceve come struttura, e non legge né file né YAML.
//   - Prodotti: i prodotti attesi dello scenario, per ConfrontaProdotti.
//   - File: una voce per file e per sezione degli attesi.
//   - AltreAmmesse: altre letture ammesse; senza, un candidato fuori dalle basi attese è una falsa associazione (R30 a).
type Atteso struct {
	ThreadID     uuid.UUID        `json:"thread_id"`
	Prodotti     []ProdottoAtteso `json:"prodotti,omitempty"`
	File         []FileAtteso     `json:"file,omitempty"`
	AltreAmmesse bool             `json:"altre_ammesse"`
}

// I valori di FileAtteso.Sezione: da quale parte degli attesi viene il file, con un nome neutro (P-10; R47 a, M-21). Il
// legame con le sezioni del file degli attesi lo fa la traduzione del runner: nel codice nessun nome di cliente. Solo
// da_rivedere cambia l'esito; le altre servono al rapporto e al gate.
const (
	SezioneScenario   = "scenario"
	SezioneBaseline   = "baseline"
	SezioneReali      = "reali"
	SezioneDaRivedere = "da_rivedere"
)

// I valori di FileAtteso.Atteso: la collocazione attesa del file.
const (
	AttesoRadice = "radice"
	AttesoFiglio = "figlio"
	AttesoFuori  = "fuori"
)

// I valori di FileAtteso.StatoAtteso (6.4.6, [agg.]): candidato_scenario vuole candidato_unico con autorità scenario;
// fuori_scenario vale come l'atteso «fuori»; riservato rende la voce riservata, come FileAtteso.Riservato.
const (
	StatoAttesoCandidatoScenario = "candidato_scenario"
	StatoAttesoFuoriScenario     = "fuori_scenario"
	StatoAttesoRiservato         = "riservato"
)

// FileAtteso: ciò che gli attesi dicono di un file, già tradotto dal runner.
//   - AllegatoID: il file; Sha256: il contenuto, per i controlli del runner (gli ID e gli sha si risolvono nella
//     fotografia). Il confronto si fa per AllegatoID.
//   - Atteso: radice | figlio | fuori; TargetBase: la base attesa del target del file (T nella tavola degli esiti; per la
//     baseline la base del componente deciso); Radici: le basi attese delle radici raggiungibili, per un figlio.
//   - Sezione: scenario | baseline | reali | da_rivedere.
//   - Riservato: il motivo per cui la voce è riservata (un caso riservato, un predicato non verificabile sui dati, una
//     domanda aperta): l'esito è riservato, mai «passato».
//   - DipendeDa: il caso è definito ma dipende da una domanda aperta: si valuta contro il default conservativo
//     dell'atteso, e l'esito lo annota; non è riservato (6.4.6).
//   - StatoAtteso, Autorita: lo stato atteso del caso e l'autorità attesa del target (R30 e).
//   - BaseLetta, Decisione, Invarianti: la base attesa delle letture del file, le righe di proposta e documento attese
//     (nella forma del vecchio), le chiavi booleane del caso. Li porta il runner per i suoi controlli della baseline e
//     della C5 (6.4.9); la tavola degli esiti non li usa, e confronto non li valuta.
type FileAtteso struct {
	AllegatoID  uuid.UUID       `json:"allegato_id"`
	Sha256      string          `json:"sha256"`
	Atteso      string          `json:"atteso"`
	TargetBase  string          `json:"target_base"`
	Radici      []string        `json:"radici,omitempty"`
	Sezione     string          `json:"sezione"`
	Riservato   string          `json:"riservato,omitempty"`
	DipendeDa   string          `json:"dipende_da,omitempty"`
	StatoAtteso string          `json:"stato_atteso,omitempty"`
	Autorita    string          `json:"autorita,omitempty"`
	BaseLetta   string          `json:"base_letta,omitempty"`
	Decisione   *Vecchio        `json:"decisione,omitempty"`
	Invarianti  map[string]bool `json:"invarianti,omitempty"`
}

// EsitoAtteso: corretto | ambiguo | errato | mancante | falsa_associazione | fuori_richiesta | non_coperto |
// da_rivedere | riservato (R30 a A, la tavola degli esiti del 6.4.6). Nel rapporto «mancante» si stampa «senza
// risposta» (R30 c), per non confonderlo con la disponibilità del file: il nome lo sceglie il runner.
type EsitoAtteso string

const (
	EsitoCorretto          EsitoAtteso = "corretto"
	EsitoAmbiguo           EsitoAtteso = "ambiguo"
	EsitoErrato            EsitoAtteso = "errato"
	EsitoMancante          EsitoAtteso = "mancante"
	EsitoFalsaAssociazione EsitoAtteso = "falsa_associazione"
	EsitoFuoriRichiesta    EsitoAtteso = "fuori_richiesta"
	EsitoNonCoperto        EsitoAtteso = "non_coperto"
	EsitoDaRivedere        EsitoAtteso = "da_rivedere"
	EsitoRiservato         EsitoAtteso = "riservato"
)

// Il peso di un esito nel gate (R30 b A): errato e falsa associazione bloccano; ambiguo e mancante sono astensioni;
// corretto e fuori richiesta sono copertura (contata per cliente dal runner); non coperto e da rivedere stanno fuori
// dal gate, contati a parte (R30 d; la decisione C5); un riservato non è mai «passato».
const (
	PesoCopertura    = "copertura"
	PesoAstensione   = "astensione"
	PesoBloccante    = "bloccante"
	PesoFuoriDalGate = "fuori_dal_gate"
	PesoMaiPassato   = "mai_passato"
)

// I motivi di EsitoFileAtteso.Motivo. Per l'esito ambiguo il motivo può portarne due, uniti da una virgola,
// nell'ordine radici_in_piu, autorita_diversa (R-49).
const (
	MotivoAttesoFileNonNelThread     = "file_non_nel_thread"           // la voce degli attesi non ha il suo file fra quelli del thread
	MotivoAttesoSenzaVoce            = "senza_voce"                    // il file non ha voci negli attesi
	MotivoAttesoStatoRiservato       = "stato_atteso_riservato"        // riservato per lo stato atteso, senza un motivo scritto
	MotivoAttesoNonValutato          = "non_valutato"                  // il motore non ha valutato il file
	MotivoAttesoNonDeterminabile     = "non_determinabile"             // collocazione non determinabile, nessun candidato
	MotivoAttesoNessunCandidato      = "nessun_candidato"              // nessun candidato
	MotivoAttesoCandidatiConFuori    = "candidati_con_atteso_fuori"    // l'atteso è «fuori», il motore ha candidati
	MotivoAttesoCandidatiSenzaTarget = "candidati_senza_target_atteso" // nessun target atteso, il motore ha candidati
	MotivoAttesoBaseFuoriAtteso      = "base_fuori_atteso"             // nessuna base dei candidati è la base attesa
	MotivoAttesoCandidatiInPiu       = "candidati_in_piu"              // una base in più, senza altre letture ammesse
	MotivoAttesoCollocazioneDiversa  = "collocazione_diversa"          // le basi giuste, la collocazione no
	MotivoAttesoAutoritaDiversa      = "autorita_diversa"              // le basi giuste, l'autorità no (R30 e)
	MotivoAttesoNonCandidatoUnico    = "non_candidato_unico"           // candidato_scenario senza candidato_unico
	MotivoAttesoRadiciInPiu          = "radici_in_piu"                 // radici raggiungibili oltre quelle attese (R30 e)
	MotivoAttesoRadiciMancanti       = "radici_mancanti"               // radici attese non raggiunte (R30 e)
)

// EsitoFileAtteso: l'esito di un file contro una voce degli attesi, con il peso nel gate e il motivo. Un file senza
// voci ha un esito non_coperto, con Sezione vuota.
type EsitoFileAtteso struct {
	AllegatoID uuid.UUID   `json:"allegato_id"`
	Sezione    string      `json:"sezione,omitempty"`
	Esito      EsitoAtteso `json:"esito"`
	Peso       string      `json:"peso"`
	Motivo     string      `json:"motivo,omitempty"`
	DipendeDa  string      `json:"dipende_da,omitempty"`
}

// ProdottoAtteso: un prodotto atteso dello scenario, con i campi che gli attesi nominano. Un campo vuoto (nil per i
// puntatori) non è nominato e non si controlla (R25 c-d); la base è la chiave.
type ProdottoAtteso struct {
	CodiceRichiesto string `json:"codice_richiesto,omitempty"`
	Base            string `json:"base"`
	Fase            string `json:"fase,omitempty"`
	Quantita        *int   `json:"quantita,omitempty"`
	QuantitaDaCella *bool  `json:"quantita_da_cella,omitempty"`
}

// EsitoProdottoAtteso: un prodotto atteso con il candidato del motore che ha la sua base, o un candidato senza voce.
//   - corretto: c'è, e ogni campo nominato coincide;
//   - errato: c'è, ma un campo nominato no (Differenze dice quali);
//   - mancante: nessun candidato ha la base attesa;
//   - non_coperto: un candidato del motore che non ha voci negli attesi; i conteggi li controlla il runner.
type EsitoProdottoAtteso struct {
	Base       string          `json:"base"`
	Atteso     *ProdottoAtteso `json:"atteso,omitempty"`
	Nuovo      *ProdottoNuovo  `json:"nuovo,omitempty"`
	Esito      EsitoAtteso     `json:"esito"`
	Differenze []string        `json:"differenze,omitempty"`
}

// PesoNelGate: il peso di un esito nel gate (R30 b A). Un esito fuori vocabolario non è mai «passato».
func PesoNelGate(e EsitoAtteso) string {
	switch e {
	case EsitoCorretto, EsitoFuoriRichiesta:
		return PesoCopertura
	case EsitoAmbiguo, EsitoMancante:
		return PesoAstensione
	case EsitoErrato, EsitoFalsaAssociazione:
		return PesoBloccante
	case EsitoNonCoperto, EsitoDaRivedere:
		return PesoFuoriDalGate
	}
	return PesoMaiPassato
}

// ConfrontaConAtteso: l'esito di ogni file del thread contro le voci degli attesi (la tavola degli esiti del 6.4.6;
// R30 a, b, d, e A). Una voce per ogni FileAtteso, anche quando il suo file non è fra quelli del thread (mancante: il
// runner lo controlla anche da sé); un esito non_coperto per ogni file senza voci. In ordine di AllegatoID, poi di
// sezione. È pura: lo stesso ingresso, comunque permutato, dà lo stesso risultato.
func ConfrontaConAtteso(file []File, atteso Atteso) []EsitoFileAtteso {
	ordinati := ordinaFile(file)
	perID := map[uuid.UUID]File{}
	for _, f := range ordinati {
		if _, ok := perID[f.AllegatoID]; !ok {
			perID[f.AllegatoID] = f
		}
	}
	voci := append([]FileAtteso(nil), atteso.File...)
	chiavi := make([]string, len(voci))
	for i, a := range voci {
		chiavi[i] = impronta(a)
	}
	indici := make([]int, len(voci))
	for i := range indici {
		indici[i] = i
	}
	sort.SliceStable(indici, func(x, y int) bool {
		a, b := voci[indici[x]], voci[indici[y]]
		if c := bytes.Compare(a.AllegatoID[:], b.AllegatoID[:]); c != 0 {
			return c < 0
		}
		if a.Sezione != b.Sezione {
			return a.Sezione < b.Sezione
		}
		return chiavi[indici[x]] < chiavi[indici[y]]
	})
	var out []EsitoFileAtteso
	coperti := map[uuid.UUID]bool{}
	for _, i := range indici {
		a := voci[i]
		f, presente := perID[a.AllegatoID]
		coperti[a.AllegatoID] = true
		esito, motivo := esitoContro(f, presente, a, atteso.AltreAmmesse)
		out = append(out, EsitoFileAtteso{AllegatoID: a.AllegatoID, Sezione: a.Sezione, Esito: esito,
			Peso: PesoNelGate(esito), Motivo: motivo, DipendeDa: a.DipendeDa})
	}
	for _, f := range ordinati {
		if coperti[f.AllegatoID] {
			continue
		}
		coperti[f.AllegatoID] = true
		out = append(out, EsitoFileAtteso{AllegatoID: f.AllegatoID, Esito: EsitoNonCoperto,
			Peso: PesoNelGate(EsitoNonCoperto), Motivo: MotivoAttesoSenzaVoce})
	}
	sort.SliceStable(out, func(x, y int) bool {
		return bytes.Compare(out[x].AllegatoID[:], out[y].AllegatoID[:]) < 0
	})
	return out
}

// esitoContro: la tavola degli esiti per un file e una voce. T è la base attesa del target; C sono i candidati del
// motore per il file valutato, con le loro basi. Un candidato la cui base non si legge (Base vuota) sta in C ma non è
// mai in T (R30 a A: «C non vuoto» vale per i candidati, non per le basi lette; R-41). L'ordine:
//  1. la sezione da_rivedere (C5), poi la voce riservata: fuori dal gate, mai «passato»;
//  2. la voce senza il suo file: mancante;
//  3. l'atteso «fuori»: falsa_associazione con candidati, fuori_richiesta se il motore lo dice fuori richiesta,
//     altrimenti mancante (nessuna risposta);
//  4. nessun target atteso: falsa_associazione con candidati; un file non valutato è nessuna risposta, mancante (R-42);
//     altrimenti corretto (C = T = ∅);
//  5. nessuna risposta (non valutato, nessun candidato, anche con la collocazione non determinabile): mancante;
//  6. nessuna base di C in T: errato;
//  7. associazione ambigua o discordante con T in C: ambiguo (un'alternativa dichiarata non è una scelta: R30 b A). Il
//     motivo dice le radici in più e l'autorità diversa del candidato con la base attesa, se ci sono: niente si perde
//     (R-49);
//  8. un candidato in più (un'altra base, o una base che non si legge) senza altre letture ammesse: falsa_associazione;
//  9. le basi giuste: la collocazione attesa (radice o figlio), l'autorità attesa (candidato_scenario vuole
//     candidato_unico con autorità scenario) e, per un figlio, le radici raggiungibili uguali alle attese (radici in
//     più: falsa_associazione; radici mancanti: mancante); altrimenti corretto.
func esitoContro(f File, presente bool, a FileAtteso, altreAmmesse bool) (EsitoAtteso, string) {
	switch {
	case a.Sezione == SezioneDaRivedere:
		return EsitoDaRivedere, ""
	case a.Riservato != "":
		return EsitoRiservato, a.Riservato
	case a.StatoAtteso == StatoAttesoRiservato:
		return EsitoRiservato, MotivoAttesoStatoRiservato
	case !presente:
		return EsitoMancante, MotivoAttesoFileNonNelThread
	}
	n := f.Nuovo
	basi := map[string]bool{}
	candidati, senzaBase := 0, 0
	if n.Valutato {
		for _, c := range n.Candidati {
			candidati++
			if c.Base == "" {
				senzaBase++
				continue
			}
			basi[c.Base] = true
		}
	}
	senzaRisposta := func() (EsitoAtteso, string) {
		switch {
		case !n.Valutato:
			return EsitoMancante, MotivoAttesoNonValutato
		case n.Collocazione == collocazioneNonDeterminabile:
			return EsitoMancante, MotivoAttesoNonDeterminabile
		}
		return EsitoMancante, MotivoAttesoNessunCandidato
	}

	if a.Atteso == AttesoFuori || a.StatoAtteso == StatoAttesoFuoriScenario {
		switch {
		case candidati > 0:
			return EsitoFalsaAssociazione, MotivoAttesoCandidatiConFuori
		case n.Valutato && n.Collocazione == collocazioneFuoriRichiesta:
			return EsitoFuoriRichiesta, ""
		}
		return senzaRisposta()
	}
	t := a.TargetBase
	if t == "" {
		switch {
		case candidati > 0:
			return EsitoFalsaAssociazione, MotivoAttesoCandidatiSenzaTarget
		case !n.Valutato:
			return senzaRisposta() // nessuna risposta non è mai copertura (R30 b)
		}
		return EsitoCorretto, ""
	}
	if candidati == 0 {
		return senzaRisposta()
	}
	if !basi[t] {
		return EsitoErrato, MotivoAttesoBaseFuoriAtteso
	}

	// I candidati con la base attesa, e le loro autorità e radici.
	var giusti []Candidato
	for _, c := range n.Candidati {
		if c.Base == t {
			giusti = append(giusti, c)
		}
	}
	autorita := autoritaAttesa(a)
	autoritaSbagliata := autoritaDiversa(giusti, autorita)
	radiciInPiu, radiciMancanti := false, false
	if a.Atteso == AttesoFiglio {
		radiciInPiu, radiciMancanti = confrontaRadici(giusti, a.Radici)
	}

	if n.Associazione == associazioneAmbiguo || n.Associazione == associazioneDiscordante {
		var motivi []string
		if radiciInPiu {
			motivi = append(motivi, MotivoAttesoRadiciInPiu)
		}
		if autoritaSbagliata {
			motivi = append(motivi, MotivoAttesoAutoritaDiversa)
		}
		return EsitoAmbiguo, strings.Join(motivi, ",")
	}
	if (len(basi) > 1 || senzaBase > 0) && !altreAmmesse {
		return EsitoFalsaAssociazione, MotivoAttesoCandidatiInPiu
	}
	if (a.Atteso == AttesoRadice && n.Collocazione != collocazioneRadice) || (a.Atteso == AttesoFiglio && n.Collocazione != collocazioneFiglio) {
		return EsitoErrato, MotivoAttesoCollocazioneDiversa
	}
	if autoritaSbagliata {
		return EsitoErrato, MotivoAttesoAutoritaDiversa
	}
	if a.StatoAtteso == StatoAttesoCandidatoScenario && n.Associazione != associazioneCandidatoUnico {
		return EsitoMancante, MotivoAttesoNonCandidatoUnico
	}
	switch {
	case radiciInPiu:
		return EsitoFalsaAssociazione, MotivoAttesoRadiciInPiu
	case radiciMancanti:
		return EsitoMancante, MotivoAttesoRadiciMancanti
	}
	return EsitoCorretto, ""
}

// autoritaAttesa: l'autorità che la voce chiede; candidato_scenario senza un'autorità scritta chiede «scenario».
func autoritaAttesa(a FileAtteso) string {
	if a.StatoAtteso == StatoAttesoCandidatoScenario && a.Autorita == "" {
		return autoritaScenario
	}
	return a.Autorita
}

// autoritaDiversa: un candidato con la base attesa ha un'autorità diversa da quella attesa (R30 e A). Nessuna autorità
// attesa: niente da controllare.
func autoritaDiversa(giusti []Candidato, autorita string) bool {
	if autorita == "" {
		return false
	}
	for _, c := range giusti {
		if c.Autorita != autorita {
			return true
		}
	}
	return false
}

// confrontaRadici: le radici raggiungibili dei candidati con la base attesa contro le radici attese (R30 e A): se ce ne
// sono in più, e se ne mancano. Il risultato non dipende dall'ordine delle mappe.
//
// Una radice che non si legge arriva come "" (valutazione ne mette una sola per una o più radici illeggibili: R-62,
// come i candidati senza base di R-41). Non coincide con nessuna base attesa, nemmeno con un "" scritto fra le attese,
// che non è una base: è sempre una radice in più, e non sparisce mai.
func confrontaRadici(giusti []Candidato, attese []string) (inPiu, mancanti bool) {
	raggiunte, attesa := map[string]bool{}, map[string]bool{}
	for _, c := range giusti {
		for _, r := range c.Radici {
			if r == "" {
				inPiu = true
				continue
			}
			raggiunte[r] = true
		}
	}
	for _, r := range attese {
		if r != "" {
			attesa[r] = true
		}
	}
	for r := range raggiunte {
		if !attesa[r] {
			inPiu = true
		}
	}
	for r := range attesa {
		if !raggiunte[r] {
			mancanti = true
		}
	}
	return inPiu, mancanti
}

// ConfrontaProdotti: i candidati prodotto della mail contro i prodotti attesi dello scenario (6.4.6, [agg.]; 6.4.9:
// originale, base, fase, quantità, evidenza in una cella). Il legame si fa in due passate (R-47), e ogni candidato si usa
// una volta:
//  1. gli attesi che nominano il codice richiesto prendono il candidato con la stessa base e lo stesso codice;
//  2. gli altri attesi, compresi quelli della prima passata rimasti senza candidato, prendono il primo candidato libero
//     con la stessa base.
//
// Così un atteso senza codice non porta via il candidato che serviva a quello con il codice. Prima gli attesi, poi i
// candidati senza voce, in un ordine che non dipende da quello degli ingressi.
func ConfrontaProdotti(nuovi []ProdottoNuovo, attesi []ProdottoAtteso) []EsitoProdottoAtteso {
	n := append([]ProdottoNuovo(nil), nuovi...)
	sort.SliceStable(n, func(x, y int) bool {
		if n[x].Base != n[y].Base {
			return n[x].Base < n[y].Base
		}
		if n[x].CodiceRichiesto != n[y].CodiceRichiesto {
			return n[x].CodiceRichiesto < n[y].CodiceRichiesto
		}
		return impronta(n[x]) < impronta(n[y])
	})
	a := append([]ProdottoAtteso(nil), attesi...)
	sort.SliceStable(a, func(x, y int) bool {
		if a[x].Base != a[y].Base {
			return a[x].Base < a[y].Base
		}
		if a[x].CodiceRichiesto != a[y].CodiceRichiesto {
			return a[x].CodiceRichiesto < a[y].CodiceRichiesto
		}
		return impronta(a[x]) < impronta(a[y])
	})
	usati := make([]bool, len(n))
	scelti := make([]int, len(a))
	for i := range scelti {
		scelti[i] = -1
	}
	for i, att := range a { // 1: il legame esatto, base e codice
		if att.Base == "" || att.CodiceRichiesto == "" {
			continue
		}
		for j := range n {
			if !usati[j] && n[j].Base == att.Base && n[j].CodiceRichiesto == att.CodiceRichiesto {
				scelti[i], usati[j] = j, true
				break
			}
		}
	}
	for i, att := range a { // 2: la sola base
		if scelti[i] >= 0 || att.Base == "" {
			continue
		}
		for j := range n {
			if !usati[j] && n[j].Base == att.Base {
				scelti[i], usati[j] = j, true
				break
			}
		}
	}
	var out []EsitoProdottoAtteso
	for i := range a {
		att := a[i]
		e := EsitoProdottoAtteso{Base: att.Base, Atteso: &att}
		if scelti[i] < 0 {
			e.Esito = EsitoMancante
			out = append(out, e)
			continue
		}
		trovato := n[scelti[i]]
		e.Nuovo = &trovato
		e.Differenze = differenzeProdotto(att, trovato)
		e.Esito = EsitoCorretto
		if len(e.Differenze) > 0 {
			e.Esito = EsitoErrato
		}
		out = append(out, e)
	}
	for j := range n {
		if usati[j] {
			continue
		}
		trovato := n[j]
		out = append(out, EsitoProdottoAtteso{Base: trovato.Base, Nuovo: &trovato, Esito: EsitoNonCoperto})
	}
	return out
}

// differenzeProdotto: i campi nominati dall'atteso che il candidato non ha uguali, in un ordine fisso.
func differenzeProdotto(a ProdottoAtteso, n ProdottoNuovo) []string {
	var out []string
	if a.CodiceRichiesto != "" && a.CodiceRichiesto != n.CodiceRichiesto {
		out = append(out, "codice_richiesto")
	}
	if a.Fase != "" && a.Fase != n.Fase {
		out = append(out, "fase")
	}
	if a.Quantita != nil && (n.Quantita == nil || *n.Quantita != *a.Quantita) {
		out = append(out, "quantita")
	}
	if a.QuantitaDaCella != nil && *a.QuantitaDaCella != n.QuantitaDaCella {
		out = append(out, "quantita_da_cella")
	}
	return out
}
