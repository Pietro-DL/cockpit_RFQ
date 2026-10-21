package propostabom

// candidatura_sorgente.go: lo STEP candidato del prodotto finito, letto con il Motore A (giro G39, 20/10; G39-02 [U]:
// «lo STEP del prodotto finito è un candidato sorgente, mai PB né AR ordinaria; si riconosce con il Motore A; prima di S8
// la pagina propone "il motore ha trovato che questo file è molto probabilmente lo STEP strutturale: lo accetti?"»).
//
// La regola [T-39, corsia motore] è una sola e sta qui, pura, sull'esito del calcolo e sulla fotografia dello stesso
// calcolo: la usano S9 e /dati per non offrire mai lo STEP candidato fra i file da preassociare e per esporlo nella fonte
// come proposta di sorgente; la persistenza la chiama: distintabozza.StepCandidatiDalMotore (R2, PB iniziali,
// suggerimenti, R4) prende le coppie (allegato, prodotto) di tutte le candidature di questa funzione, univoche o no, e a
// parte quelle univoche, senza riscrivere la regola (revisione della corsia motore, giro G39; giro G39-bis, 21/10).
//
// Uno STEP (estensione stp o step: lo dice chi chiama con eStep, dalla fotografia) è legato al prodotto finito P quando
// il calcolo lo lega a P in uno di questi tre modi:
//   - il suo ancoraggio (core/ancoraggio, ProponiAncoraggi, regola 3) ha un candidato a livello prodotto con il target
//     P, senza dimensioni discordanti: un'identità letta dal file (il nome del file, la voce d'archivio, l'id o il nome
//     della radice dello STEP) ha la base del codice del prodotto (uguale, equivalente o compatibile parziale) e il
//     marcatore compatibile con quello del prodotto, con la regola unica del marcatore (T-B4-30: compatibile se scritto
//     da una parte sola, come il codice letto dal nome «12345678» contro il prodotto «12345678Z», codici inventati,
//     quando la grammatica legge Z come marcatore; due marcatori scritti e diversi sono un'altra identità);
//   - una struttura candidata di P ha la radice dello STEP stesso (RadiceDelFile);
//   - un documento candidato della fonte di P è quello STEP con una radice compatibile, oppure con l'origine motore_a o
//     documento_del_prodotto.
// Il target è un prodotto valutato con il suo componente (i prodotti valutati sono i prodotti finiti della RFQ).
//
// La rete di sicurezza del nome esatto [T-39, giro G39-bis, lato prudente; rilievo della revisione del contratto su
// G39-02 [U]]: uno STEP che il calcolo non lega a nessun prodotto, il cui nome senza cartella e senza estensione, senza
// maiuscole/minuscole e spazi ai bordi, è il codice di un prodotto finito della RFQ (lo STEP «12345678Z.stp» per il
// prodotto 12345678Z, quando la grammatica non legge il nome), è legato a quel prodotto con l'evidenza
// SelettoreNomeFileCodiceProdotto: senza la rete diventerebbe un file ordinario (PB e poi AR), contro G39-02 [U].
//
// L'evidenza discordante [T-39, giro G39-bis; D-2 (c) [U]: «escludere soltanto i file effettivamente legati come
// sorgente strutturale del finito, con prova specifica», e D-AR04 (iii) [U]]: il legame dello STEP con P NON è una
// candidatura quando
//   - il nome del file, normalizzato come sopra, è il codice di un altro componente non archiviato della RFQ che non è
//     un prodotto finito valutato, oppure comincia con quel codice seguito da un separatore, cioè da un carattere che non
//     è una lettera né una cifra [T-39, revisione indipendente del giro G39-bis; giro G39-ter: ogni carattere non
//     alfanumerico]: gli STEP «12345678-100.stp», «12345678-100_rev2.stp», «12345678-100 A.stp», «12345678-100-B.stp» e
//     «12345678-100(B).stp» con il sottoassieme 12345678-100 sono STEP del sottoassieme, anche se la grammatica ne legge
//     solo la base 12345678, che è quella del finito 12345678Z. Non conta un altro componente il cui codice copre meno
//     nome di quello di P (lo STEP «12345678-100-A.stp» del finito 12345678-100-A con il sottoassieme 12345678-100 è del
//     finito). Fa eccezione lo STEP che ha una sua struttura con la radice P (la strada della struttura candidata, la
//     prova più forte che sia lo STEP del finito: [T-39] da G39-02 [U]): il nome di un altro componente rende la sua
//     candidatura non univoca, non la toglie [T-39, giro G39-ter];
//   - l'ancoraggio dello STEP ha un candidato senza dimensioni discordanti di un componente che non è un prodotto finito
//     valutato (un nodo, a qualunque livello, o un componente a livello prodotto che non è fra i valutati). Fa eccezione
//     il nodo di una struttura di P quando lo STEP ha una sua struttura con la radice P (la strada della struttura
//     candidata): il nome dello STEP del finito che somiglia a un suo figlio non è un'evidenza contro (lo STEP del
//     finito «12345678Z.stp» con il figlio 12345678-100 letto con la stessa base).
// Uno STEP che non è una candidatura resta un file ordinario (PB → R5 → AR, D-2 (c)). Un altro prodotto finito non è
// un'evidenza discordante (D-2 (c) riguarda gli STEP di parti e sottoassiemi): rende la candidatura non univoca.
//
// La candidatura è univoca quando lo STEP è legato a un prodotto solo (contando anche i legami tolti dall'evidenza
// discordante), il suo ancoraggio non ha altri candidati (né di un altro target né del prodotto con una dimensione
// discordante, salvo l'eccezione del nodo di sopra) e il nome non è ambiguo. Il nome è ambiguo (il motore non sa
// decidere) [T-39, giro G39-bis e giro G39-ter; G39-06 [U]] quando
//   - comincia, come sopra, con il codice di un altro prodotto finito, oppure con il codice di un altro componente che è
//     l'inizio del codice di P (il componente 12345678 e il finito 12345678Z: «12345678.stp» o «12345678 Z.stp» non
//     dicono quale dei due);
//   - comincia con il codice di un altro componente seguito subito da una lettera o da una cifra, oppure con quel codice
//     scritto con separatori diversi (il confronto sulle sole lettere e cifre), e il codice di P non copre più nome:
//     «12345678-100A.stp», «12345678-100rev2.stp» e «12345678_100.stp» con il sottoassieme 12345678-100 e il finito
//     12345678Z (dopo la base del finito viene il testo del codice del sottoassieme, non il marcatore del finito);
//   - è di un altro componente che non è un finito e lo STEP ha una sua struttura con la radice P (l'eccezione di sopra).
// Solo la candidatura univoca si propone come sorgente (S9, /dati) e vale per l'esenzione dal motivo in S8 (G39-02 [U]).
// Tutte le candidature restano fuori dalle PB iniziali e dai suggerimenti, e la PB verso un prodotto finito è sempre
// rifiutata (R2) o bloccata (R4). Per la candidatura non univoca sceglie l'utente (G39-06 [U]: «lasciamo la scelta di
// conferma all'utente sul file che preferisce»; lettura [T] dell'orchestratore): S8 verso un finito, con il motivo,
// oppure una PB verso una parte o un sottoassieme (distintabozza.StepCandidati.BloccaPB).
//
// Le evidenze sono le letture dell'ancoraggio che sostengono il candidato: il selettore (nome_file, radice_step.id, …), il
// testo letto, la base e il marcatore letti, e l'esito del confronto della base e del marcatore con il prodotto (le
// dimensioni del candidato); per gli altri due modi una riga con il selettore struttura_candidata o fonte.<origine> e la
// compatibilità della radice; per la rete del nome esatto una riga con il selettore nome_file.codice_del_prodotto, il
// nome letto e l'esito uguale. Il codice del prodotto è quello del componente della fotografia (CodiceRichiesto del
// prodotto valutato se la fotografia non lo dà).

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/valutazione"
)

// I selettori delle evidenze che non vengono da una lettura dell'ancoraggio (commento di testa).
const (
	SelettoreStrutturaCandidata = "struttura_candidata"
	PrefissoSelettoreFonte      = "fonte."
	// SelettoreNomeFileCodiceProdotto: la rete di sicurezza del nome esatto (giro G39-bis).
	SelettoreNomeFileCodiceProdotto = "nome_file.codice_del_prodotto"
)

// CandidaturaSorgente: uno STEP candidato sorgente di un prodotto finito (commento di testa).
type CandidaturaSorgente struct {
	AllegatoID     uuid.UUID             `json:"allegato_id"`
	ProdottoID     uuid.UUID             `json:"prodotto_id"`
	Prodotto       string                `json:"prodotto"`
	CodiceProdotto string                `json:"codice_prodotto"`
	Univoca        bool                  `json:"univoca"`
	Evidenze       []EvidenzaCandidatura `json:"evidenze"`
}

// EvidenzaCandidatura: una lettura del file che sostiene la candidatura, con il confronto con il prodotto.
type EvidenzaCandidatura struct {
	Lettura        string `json:"lettura"`
	Selettore      string `json:"selettore"`
	CodiceLetto    string `json:"codice_letto"`
	BaseLetta      string `json:"base_letta,omitempty"`
	MarcatoreLetto string `json:"marcatore_letto,omitempty"`
	EsitoBase      string `json:"esito_base,omitempty"`
	EsitoMarcatore string `json:"esito_marcatore,omitempty"`
}

// CandidatureSorgenteDelFinito: le candidature degli STEP del thread (commento di testa), in ordine di allegato e di
// prodotto. th è la fotografia dello stesso calcolo (i nomi dei file, i componenti e i loro codici; nil: niente rete del
// nome esatto e niente confronto del nome con i codici, il codice del prodotto è CodiceRichiesto); eStep dice se
// l'allegato è uno STEP (la rete del nome esatto guarda anche l'estensione del file).
func CandidatureSorgenteDelFinito(et *valutazione.EsitoThread, th *fotorfq.Thread, eStep func(uuid.UUID) bool) []CandidaturaSorgente {
	var out []CandidaturaSorgente
	if et == nil || eStep == nil {
		return out
	}
	prodotti := map[string]valutazione.ProdottoValutato{}
	for _, pv := range et.ProdottiValutati {
		if pv.ComponenteID != nil {
			prodotti[pv.Rif] = pv
		}
	}
	// dalla fotografia: i codici dei componenti (per il codice del prodotto), quelli normalizzati dei componenti non
	// archiviati (per il confronto con il nome) e gli allegati
	codici := map[uuid.UUID]string{}
	type componente struct {
		id     uuid.UUID
		codice string
	}
	var componenti []componente
	var allegati []fotorfq.Allegato
	nomi := map[uuid.UUID]string{}
	if th != nil {
		for _, c := range th.Componenti {
			codici[c.ID] = c.Codice
			if n := codiceNormalizzato(c.Codice); n != "" && c.ArchiviatoIl == nil {
				componenti = append(componenti, componente{c.ID, n})
			}
		}
		allegati = th.Allegati
		for _, a := range th.Allegati {
			nomi[a.ID] = codiceNormalizzato(nomeSenzaEstensione(a.NomeFile))
		}
	}
	letture := map[uuid.UUID]map[string]motorea.LetturaCodice{}
	for _, f := range et.File {
		m := map[string]motorea.LetturaCodice{}
		for _, l := range f.Interpretazione.Letture {
			m[l.ID] = l
		}
		letture[f.AllegatoID] = m
	}
	// per allegato e prodotto il legame, con le evidenze
	type chiave struct{ a, p uuid.UUID }
	per := map[chiave]*CandidaturaSorgente{}
	var ordine []chiave
	legame := func(a uuid.UUID, pv valutazione.ProdottoValutato) *CandidaturaSorgente {
		k := chiave{a, *pv.ComponenteID}
		if x, ok := per[k]; ok {
			return x
		}
		x := &CandidaturaSorgente{AllegatoID: a, ProdottoID: *pv.ComponenteID, Prodotto: pv.Rif, CodiceProdotto: pv.CodiceRichiesto,
			Evidenze: []EvidenzaCandidatura{}}
		if c, ok := codici[*pv.ComponenteID]; ok && c != "" {
			x.CodiceProdotto = c
		}
		per[k] = x
		ordine = append(ordine, k)
		return x
	}
	ancoraggi := map[uuid.UUID]ancoraggio.AncoraggioFile{}
	for _, af := range et.Ancoraggi.File {
		if !eStep(af.AllegatoID) {
			continue
		}
		ancoraggi[af.AllegatoID] = af
		for _, c := range af.Candidati {
			pv, ok := prodotti[c.Target]
			if c.Livello != ancoraggio.LivelloProdotto || len(c.Conflitti) > 0 || !ok {
				continue
			}
			x := legame(af.AllegatoID, pv)
			for _, id := range c.Letture {
				e := EvidenzaCandidatura{Lettura: id}
				if l, ok := letture[af.AllegatoID][id]; ok {
					e.Selettore, e.CodiceLetto, e.BaseLetta = l.Forma.Selettore.String(), l.Forma.Originale, l.Forma.Base.Normalizzata
					if l.Forma.Marcatore != nil {
						e.MarcatoreLetto = l.Forma.Marcatore.Valore
					}
				}
				for _, d := range c.Dimensioni {
					if d.Lettura != id || d.Rif != c.Target {
						continue
					}
					switch d.Dimensione {
					case ancoraggio.DimensioneBase:
						e.EsitoBase = string(d.Esito)
					case ancoraggio.DimensioneMarcatore:
						e.EsitoMarcatore = string(d.Esito)
					}
				}
				x.Evidenze = append(x.Evidenze, e)
			}
		}
	}
	// la struttura del file con la radice P (l'eccezione del nodo dell'evidenza discordante)
	strutturaPropria := map[chiave]bool{}
	for _, pv := range et.ProdottiValutati {
		if pv.ComponenteID == nil {
			continue
		}
		for _, sc := range pv.StruttureCandidate {
			if sc.RadiceDelFile && eStep(sc.AllegatoID) {
				strutturaPropria[chiave{sc.AllegatoID, *pv.ComponenteID}] = true
				x := legame(sc.AllegatoID, pv)
				x.Evidenze = append(x.Evidenze, EvidenzaCandidatura{Selettore: SelettoreStrutturaCandidata, CodiceLetto: sc.Radice,
					EsitoBase: string(sc.Compatibilita)})
			}
		}
		for _, c := range pv.Fonte.Candidati {
			if c.AllegatoID == nil || !eStep(*c.AllegatoID) {
				continue
			}
			if c.RadiceCompatibile != "" || c.Origine == ancoraggio.CandidatoDaMotoreA || c.Origine == ancoraggio.CandidatoDaDocumentoDelProdotto {
				x := legame(*c.AllegatoID, pv)
				x.Evidenze = append(x.Evidenze, EvidenzaCandidatura{Selettore: PrefissoSelettoreFonte + string(c.Origine), CodiceLetto: c.RadiceCompatibile,
					EsitoBase: string(c.Compatibilita)})
			}
		}
	}
	// la rete di sicurezza del nome esatto: solo per gli STEP che il calcolo non lega a nessun prodotto
	legati := map[uuid.UUID]bool{}
	for _, k := range ordine {
		legati[k.a] = true
	}
	for _, a := range allegati {
		n := nomi[a.ID]
		if n == "" || legati[a.ID] || !eStep(a.ID) || !nomeDiUnoStep(a) {
			continue
		}
		for _, pv := range et.ProdottiValutati {
			if pv.ComponenteID == nil {
				continue
			}
			codice := pv.CodiceRichiesto
			if c, ok := codici[*pv.ComponenteID]; ok && c != "" {
				codice = c
			}
			if codiceNormalizzato(codice) == n {
				x := legame(a.ID, pv)
				x.Evidenze = append(x.Evidenze, EvidenzaCandidatura{Selettore: SelettoreNomeFileCodiceProdotto,
					CodiceLetto: strings.TrimSpace(nomeSenzaEstensione(a.NomeFile)), EsitoBase: string(motorea.CompatibilitaUguale)})
			}
		}
	}
	// nodoProprio: il candidato è un nodo di una struttura di P e lo STEP ha la sua struttura con la radice P
	nodoProprio := func(k chiave, rif string, c ancoraggio.CandidatoAncoraggio) bool {
		if !strutturaPropria[k] || c.Livello == ancoraggio.LivelloProdotto || len(c.RadiciRaggiungibili) == 0 {
			return false
		}
		for _, r := range c.RadiciRaggiungibili {
			if r != rif {
				return false
			}
		}
		return true
	}
	// i prodotti finiti valutati (con o senza componente): un altro finito non è un'evidenza discordante, rende la
	// candidatura non univoca (commento di testa)
	valutati := map[string]bool{}
	finiti := map[uuid.UUID]bool{}
	for _, pv := range et.ProdottiValutati {
		valutati[pv.Rif] = true
		if pv.ComponenteID != nil {
			finiti[*pv.ComponenteID] = true
		}
	}
	// dalNome: il confronto del nome del file con i codici dei componenti (commento di testa). togli: il nome è di un
	// altro componente che non è un finito; ambiguo: il nome è di un altro finito, di un componente il cui codice è
	// l'inizio di quello del finito, oppure comincia con il codice di un altro componente senza un separatore dopo o con
	// separatori diversi (il nome non distingue i due; giro G39-ter). Con la struttura propria (lo STEP ha una sua
	// struttura con la radice P) il nome di un altro componente non toglie: rende ambiguo ([T-39] da G39-02 [U], G39-ter)
	dalNome := func(k chiave, codiceP string) (togli, ambiguo bool) {
		n := nomi[k.a]
		if n == "" {
			return false, false
		}
		cp := codiceNormalizzato(codiceP)
		lp := 0
		if nomeComincia(n, cp) {
			lp = len(cp)
		}
		na, cpa := soloAlfanumerici(n), soloAlfanumerici(cp)
		lpa := 0
		if cpa != "" && strings.HasPrefix(na, cpa) {
			lpa = len(cpa)
		}
		for _, c := range componenti {
			if c.id == k.p {
				continue
			}
			if !nomeComincia(n, c.codice) {
				// il codice dell'altro all'inizio del nome seguito subito da una lettera o da una cifra, o scritto con
				// separatori diversi: il motore non sa decidere, salvo quando il codice del finito copre più nome
				if ca := soloAlfanumerici(c.codice); ca != "" && strings.HasPrefix(na, ca) && lpa <= len(ca) {
					ambiguo = true
				}
				continue
			}
			switch {
			case lp > len(c.codice):
				// il codice del finito copre più nome dell'altro: il nome è del finito
			case finiti[c.id], strings.HasPrefix(cp, c.codice), strutturaPropria[k]:
				ambiguo = true
			default:
				togli = true
			}
		}
		return togli, ambiguo
	}
	// discorde: l'evidenza discordante dell'ancoraggio (commento di testa): un candidato senza dimensioni discordanti di
	// un componente che non è un prodotto finito valutato, salvo il nodo proprio
	discorde := func(k chiave, rif string) bool {
		for _, c := range ancoraggi[k.a].Candidati {
			if len(c.Conflitti) > 0 || (c.Livello == ancoraggio.LivelloProdotto && valutati[c.Target]) || nodoProprio(k, rif, c) {
				continue
			}
			return true
		}
		return false
	}
	// altri: un candidato dell'ancoraggio che non è il prodotto senza dimensioni discordanti (salvo il nodo proprio)
	altri := func(k chiave, rif string) bool {
		for _, c := range ancoraggi[k.a].Candidati {
			if (c.Livello == ancoraggio.LivelloProdotto && c.Target == rif && len(c.Conflitti) == 0) || nodoProprio(k, rif, c) {
				continue
			}
			return true
		}
		return false
	}
	prodottiDi := map[uuid.UUID]int{}
	for _, k := range ordine {
		prodottiDi[k.a]++
	}
	for _, k := range ordine {
		x := per[k]
		togli, ambiguo := dalNome(k, x.CodiceProdotto)
		if togli || discorde(k, x.Prodotto) {
			continue
		}
		x.Univoca = prodottiDi[k.a] == 1 && !ambiguo && !altri(k, x.Prodotto)
		out = append(out, *x)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AllegatoID != out[j].AllegatoID {
			return out[i].AllegatoID.String() < out[j].AllegatoID.String()
		}
		return out[i].ProdottoID.String() < out[j].ProdottoID.String()
	})
	return out
}

// StepCandidatiDelFinito: gli allegati delle candidature (univoche o no): gli STEP che S9 non mette mai fra le righe
// proposte del motore (G39-02 [U]; il lato prudente della proposta).
func StepCandidatiDelFinito(cc []CandidaturaSorgente) map[uuid.UUID]bool {
	out := map[uuid.UUID]bool{}
	for _, c := range cc {
		out[c.AllegatoID] = true
	}
	return out
}

// codiceNormalizzato: un codice o un nome senza maiuscole/minuscole e spazi ai bordi (come il ripiego
// ListStepCandidatiDelFinito: upper(btrim(...))).
func codiceNormalizzato(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// nomeComincia: il nome normalizzato è il codice normalizzato, oppure comincia con il codice seguito da un separatore,
// cioè da un carattere che non è una lettera né una cifra («12345678-100_rev2», «12345678-100 A», «12345678-100-B»,
// «12345678-100.1», «12345678-100(B)» per il codice 12345678-100; codici inventati; giro G39-ter).
func nomeComincia(nome, codice string) bool {
	if codice == "" || !strings.HasPrefix(nome, codice) {
		return false
	}
	if len(nome) == len(codice) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(nome[len(codice):])
	return !alfanumerico(r)
}

// alfanumerico: una lettera o una cifra.
func alfanumerico(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// soloAlfanumerici: il testo con le sole lettere e cifre («12345678_100» e «12345678-100» danno «12345678100»): il
// confronto del nome con un codice scritto con separatori diversi (giro G39-ter).
func soloAlfanumerici(s string) string {
	var b strings.Builder
	for _, r := range s {
		if alfanumerico(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// nomeSenzaEstensione: il nome del file senza la cartella e senza l'ultima estensione.
func nomeSenzaEstensione(nome string) string {
	n := nome
	if i := strings.LastIndexAny(n, `/\`); i >= 0 {
		n = n[i+1:]
	}
	if i := strings.LastIndex(n, "."); i > 0 {
		n = n[:i]
	}
	return n
}

// nomeDiUnoStep: l'allegato è uno STEP per l'estensione o per il nome (stp, step).
func nomeDiUnoStep(a fotorfq.Allegato) bool {
	if a.Estensione != nil {
		e := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(*a.Estensione), "."))
		if e == "stp" || e == "step" {
			return true
		}
	}
	n := strings.ToLower(strings.TrimSpace(a.NomeFile))
	return strings.HasSuffix(n, ".stp") || strings.HasSuffix(n, ".step")
}
