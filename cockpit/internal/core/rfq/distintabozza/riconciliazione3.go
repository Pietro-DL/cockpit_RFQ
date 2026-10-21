package distintabozza

// La riconciliazione di una bozza senza struttura con uno STEP nuovo: cambio-fonte-2 (direttiva 39 §3 [U + T-39]; EM §6.6
// e App. I.7, passo 3-bis proposto dalla corsia contratto, rilievo R39-17; scelte [T-39] della corsia persistenza).
//
// Quando vale. Il cambio della fonte di una versione che non ha una sorgente STEP con struttura: nessuna sorgente (la
// partenza vuota, una bozza da-conferma-1 di una conferma senza fonte) o la sorgente riservata solo_prodotto (STEP reale,
// nessun registro). E' la bozza manuale a cui S8 porta uno STEP (passo 3), senza una seconda bozza attiva. Fra due
// sorgenti STEP vale cambio-fonte-1, invariato byte per byte: cambio-fonte-2 e' un algoritmo a parte perche' cambia il
// comportamento del caso «sorgente assente» che cambio-fonte-1 gia' trattava (le voci entravano tutte, anche quelle che la
// bozza aveva gia' a mano: doppioni).
//
// cambio-fonte-2 = cambio-fonte-1 (passi 1-9) piu':
//
//	3-bis l'abbinamento dei nodi senza casella (il nodo del prodotto segue il passo 2) alle voci che il passo 6 farebbe
//	      entrare. Una voce e un nodo si abbinano solo se l'identita' e' sostenuta e il legame e' univoco nei due versi:
//	        - nodo registrato (componente esistente): la decisione_esistente della voce e' quel componente, oppure il codice
//	          proposto, la rev proposta e l'emittente della lettura sono quelli del componente (namespace uguale se il nodo
//	          ne ha uno);
//	        - nodo da creare (manuale, o deciso da una persona): codice proposto (byte per byte), rev proposta ed emittente
//	          uguali a quelli del nodo, la voce senza decisione_esistente verso un altro componente, e le evidenze coerenti:
//	          la voce senza alternative ne' motivo_lettura (letture discordanti), il nodo senza evidenze documentali proprie
//	          (evidenze_identita, che il modo proposta non porta).
//	      Mai per la sola somiglianza del nome: un codice uguale senza maiuscole/minuscole uguali, una rev o un emittente
//	      diversi, una decisione_esistente verso un altro componente fanno un candidato «incompatibile», non un abbinamento.
//	      Il nodo abbinato tiene la chiave_bozza, le PB, le decisioni, la descrizione, il tipo e l'origine, e acquisisce la
//	      casella della voce (caselle_riscritte[] con allegato_prima null). Un nodo manuale passa al modo proposta con
//	      l'identita' della voce, che ha lo stesso codice, la stessa rev e lo stesso emittente (cambiano solo la presunzione e
//	      il namespace, che calcola il server): e' elencato in identita_aggiornate[]. Un nodo registrato o deciso tiene la
//	      sua identita'. La voce abbinata non entra (niente doppioni);
//	3-ter i casi non certi (piu' candidati da una parte o dall'altra: ambiguo; un solo candidato con l'identita' diversa:
//	      incompatibile; un solo candidato con identita' uguale ma evidenze non coerenti: evidenze_discordanti) vanno in
//	      abbinamenti_proposti[] {chiave_bozza, voci[], motivo}, fuori dal vocabolario chiuso di da_rivedere[]; le voci
//	      coinvolte non entrano da sole. L'ingegnere li risolve con un salvataggio R2 esplicito: «Accetta» = nodo_modificato
//	      che aggiunge la casella al nodo (con l'identita' della proposta, o una decisione identita se tiene la sua);
//	      «Rifiuta» = niente, oppure la voce come nodo nuovo («Importa dalla proposta…», nodo_aggiunto con la casella);
//	6     le altre voci entrano come in cambio-fonte-1; gli archi del registro: entra un arco verso una voce che entra (P-5),
//	      e uno da una voce che entra verso un nodo abbinato che nella bozza non ha un padre; tutti gli altri archi del
//	      registro che la bozza non ha (fra nodi che restano, o verso un nodo abbinato che ha gia' un padre, o che toccano
//	      una voce trattenuta in 3-ter) si elencano in differenze.archi[] (aggiunto). Un arco della bozza che c'e' anche nel
//	      registro con un'altra quantita' resta com'e' (la quantita' dell'ingegnere) e si elenca (cambiato, quantita); un
//	      arco della bozza fra nodi che ora hanno la casella, che il registro non ha, resta e si elenca (tolto). Il tipo di un
//	      nodo abbinato diverso dal tipo proposto della voce resta quello della bozza e si elenca in differenze.nodi[]
//	      (cambiato, tipo). «Recepisci» e' un gesto normale di R2, «mantieni» e' il default: nessuna sovrascrittura;
//	      P-6 non vale (i confermati senza STEP che la bozza non ha restano assenze da decidere: la bozza era manuale);
//	7     le decisioni restano tutte; diventano da rivedere (da_rivedere[] {decisione_id, fonte_cambiata}) quelle che citano
//	      un nodo abbinato (salvo conferma_categoria con il tipo uguale a quello proposto) e mono_pezzo se lo STEP porta dei
//	      figli; i blocchi di R4 fanno il resto. PB, nodi manuali non abbinati, motivi, esclusioni, vassoio ed eventi restano.
//
// La regola dello «stesso nodo» (DEC-B PB-34) estesa [T-39]: un nodo senza casella che 3-bis (o un «Accetta» di un
// abbinamento proposto) collega a una voce resta lo stesso nodo: tiene la chiave_bozza, senza nodo_tolto ne'
// nodo_aggiunto, e da quella versione «lo stesso nodo» e' la casella acquisita (o il componente, per un registrato). Una
// chiave non viene mai riusata per un nodo diverso: un nodo manuale tolto prima dello STEP resta senza casella nel
// giornale, e la sua chiave non torna per un nodo con casella (ChiaveRiusabile).
//
// RiconciliaEffetto e' pura: la usa Cambia (R2 cambia_sorgente) e la puo' usare l'anteprima di S6 `?bozza=` dopo
// propostabom.EffettoDelCambioFonte, perche' gli elenchi siano gli stessi.

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// AlgoritmoCambioFonte2: cambio-fonte-1 con la riconciliazione di una bozza senza struttura (commento di testa).
const AlgoritmoCambioFonte2 = "cambio-fonte-2"

// I motivi di abbinamenti_proposti[] (EM App. I.7, forma proposta dalla corsia contratto, adottata).
const (
	AbbinamentoAmbiguo             = "ambiguo"
	AbbinamentoIncompatibile       = "incompatibile"
	AbbinamentoEvidenzeDiscordanti = "evidenze_discordanti"
)

// AbbinamentoProposto3: un nodo senza casella con le voci candidate che il server non ha unito (3-ter).
type AbbinamentoProposto3 struct {
	ChiaveBozza string   `json:"chiave_bozza"`
	Voci        []string `json:"voci"`
	Motivo      string   `json:"motivo"`
}

// Abbinamento3: un nodo senza casella abbinato a una voce del registro nuovo (3-bis).
type Abbinamento3 struct {
	ChiaveBozza string `json:"chiave_bozza"`
	Voce        string `json:"voce"`
	Rif         string `json:"rif"`
	// identita: l'identita' della proposta, per il nodo manuale che passa al modo proposta
	identita *Identita
}

// CasellaRiscritta3: la riscrittura di una casella (EM §6.6 passo 3), con allegato_prima null per la casella acquisita da
// un nodo senza casella (3-bis).
type CasellaRiscritta3 struct {
	ChiaveBozza string     `json:"chiave_bozza,omitempty"`
	DecisioneID string     `json:"decisione_id,omitempty"`
	Rif         string     `json:"rif"`
	Prima       *uuid.UUID `json:"allegato_prima"`
	Dopo        uuid.UUID  `json:"allegato_dopo"`
}

// caselleRiscritte3: le riscritture di propostabom nella forma della risposta.
func caselleRiscritte3(xx []propostabom.CasellaRiscritta) []CasellaRiscritta3 {
	var out []CasellaRiscritta3
	for _, x := range xx {
		p := x.Prima
		out = append(out, CasellaRiscritta3{ChiaveBozza: x.ChiaveBozza, DecisioneID: x.DecisioneID, Rif: x.Rif, Prima: &p, Dopo: x.Dopo})
	}
	return out
}

// arcoRif: un arco del registro che entra nella versione, per rif.
type arcoRif struct {
	padre, figlio string
	qta           *int
}

// RiconciliazioneFonte3: l'esito di 3-bis/3-ter sull'effetto di cambio-fonte-1.
type RiconciliazioneFonte3 struct {
	// Effetto: l'effetto di cambio-fonte-1 con le voci abbinate e trattenute tolte da quelle che entrano, le differenze
	// e i da_rivedere in piu'
	Effetto  propostabom.EffettoCambioFonte
	Abbinati []Abbinamento3
	Proposti []AbbinamentoProposto3
	// Acquisite: le caselle acquisite dai nodi abbinati (allegato_prima null)
	Acquisite []CasellaRiscritta3
	archi     []arcoRif
}

// La revisione con un secondo STEP (direttiva 39 §2-§3 [U + T-39]; rilievo bloccante della revisione indipendente del
// 20/10 sul passaggio A→B di una bozza da-conferma-1) [T-39]. Le caselle sono legate allo sha256 dello STEP (il rif e'
// nodo:<sha256>:<chiave>): fra due STEP con contenuti diversi nessuna voce del registro nuovo ha il rif di una casella
// della fonte di prima, e cambio-fonte-1 faceva uscire tutti i nodi con la casella di A, compresi i componenti della
// working confermata, per far rientrare le voci di B come pezzi da creare con gli stessi codici (R4: ASSENZA_NON_DECISA,
// CREA_SU_ESISTENTE, PREASSOCIAZIONE_NON_APPLICABILE nodo_uscente_con_ar), o come doppioni registrati (P-6). La versione
// con una base confermata (numero almeno 1) che passa a un'altra sorgente STEP tratta allora i suoi nodi registrati
// (componenti della working, non il prodotto) le cui caselle il registro nuovo non ha come nodi senza casella e li
// riconcilia con le regole di cambio-fonte-2 (3-bis, 3-ter, archi, decisioni, niente P-6): con l'identita' sostenuta e
// univoca il nodo resta lo stesso (chiave, PB, decisioni) e prende la casella di B, con la sua origine di prima; i casi
// non certi restano nodi senza casella (origine conferma, archi step verso di loro con origine conferma) con le voci
// trattenute in abbinamenti_proposti[]: mai l'uscita silenziosa di un componente confermato, mai un doppione. Un nodo
// registrato senza candidati esce come prima (la parte tolta dallo STEP nuovo: R4 chiede la decisione sugli archi della
// working). Gli archi step verso i nodi abbinati tornano step su B se il registro nuovo li ha, altrimenti restano con
// origine conferma e si elencano fra le differenze (tolto). Gli altri nodi della fonte di prima (proposte non ancora
// confermate) seguono cambio-fonte-1 com'e'. Fra due occorrenze dello stesso contenuto (stessi rif) niente cambia.
// L'etichetta dell'evento e' cambio-fonte-2, con il conteggio registrati_della_fonte_precedente.

// revisioneFonte3: la versione di partenza preparata per la riconciliazione della revisione (commento sopra).
type revisioneFonte3 struct {
	snapshot Snapshot3
	// origini: chiave -> origine di prima dei nodi registrati che hanno perso la casella della fonte di prima
	origini map[string]string
	// archi: gli archi step verso o da quei nodi, portati a origine conferma
	archi map[[2]string]bool
	// allegatoPrima: l'occorrenza della fonte di prima
	allegatoPrima uuid.UUID
}

// PreparaRevisione: la vista della versione di partenza che il cambio della fonte usa nella revisione con un secondo
// STEP (commento sopra), e se la riconciliazione vale. La usa Cambia, e la puo' usare l'anteprima di S6 prima di
// propostabom.EffettoDelCambioFonte e di RiconciliaEffetto, perche' gli elenchi siano gli stessi.
func PreparaRevisione(prima Snapshot3, nuovo importazione.Contenuto) (Snapshot3, bool) {
	r, ok := preparaRevisione(prima, nuovo)
	if !ok {
		return prima, false
	}
	return r.snapshot, true
}

func preparaRevisione(prima Snapshot3, nuovo importazione.Contenuto) (revisioneFonte3, bool) {
	out := revisioneFonte3{origini: map[string]string{}, archi: map[[2]string]bool{}}
	if SenzaStruttura(prima) || prima.VersioneBomBase == nil || prima.VersioneBomBase.Numero < 1 {
		return out, false
	}
	src := *prima.Sorgente.AllegatoID
	nel := map[string]bool{}
	for _, v := range nuovo.Proposta.Nodi {
		nel[v.Origine.Rif] = true
	}
	for _, x := range nuovo.Proposta.FuoriPerDecisione {
		if _, rif, ok := importazione.PartiDellaVoce(x.Voce); ok {
			nel[rif] = true
		}
	}
	s := prima.copia()
	for i := range s.Nodi {
		n := &s.Nodi[i]
		if n.ComponenteID == nil || *n.ComponenteID == prima.ProdottoID || len(n.Caselle) == 0 {
			continue
		}
		dellaFonte, resta := false, false
		for _, c := range n.Caselle {
			if c.AllegatoID == src && c.Gruppo == prima.Sorgente.Gruppo {
				dellaFonte = true
				resta = resta || nel[c.Rif]
			}
		}
		if !dellaFonte || resta {
			continue
		}
		out.origini[n.ChiaveBozza] = n.Origine
		n.Caselle = nil
		if n.Origine == OrigineStep {
			n.Origine = OrigineConferma
		}
	}
	if len(out.origini) == 0 {
		return out, false
	}
	for i := range s.Archi {
		a := &s.Archi[i]
		_, p := out.origini[a.Padre]
		_, f := out.origini[a.Figlio]
		if a.Origine == OrigineStep && (p || f) {
			out.archi[[2]string{a.Padre, a.Figlio}] = true
			a.Origine, a.Step = OrigineConferma, nil
		}
	}
	out.snapshot, out.allegatoPrima = s, src
	return out, true
}

// dopoLaRevisione: nella versione costruita da Cambia, i nodi abbinati riprendono la loro origine di prima, le caselle
// acquisite dicono l'occorrenza di prima, gli archi step portati a conferma tornano step sulla fonte nuova se il registro
// nuovo li ha (fra due nodi che ora hanno la casella).
func (r revisioneFonte3) dopoLaRevisione(s *Snapshot3, ric *RiconciliazioneFonte3, nuovo importazione.Contenuto, allegato uuid.UUID, gruppo string) {
	abbinati := map[string]bool{}
	for _, a := range ric.Abbinati {
		abbinati[a.ChiaveBozza] = true
	}
	rif := map[string]string{}
	for i := range s.Nodi {
		n := &s.Nodi[i]
		if o, ok := r.origini[n.ChiaveBozza]; ok && abbinati[n.ChiaveBozza] {
			n.Origine = o
		}
		if len(n.Caselle) > 0 {
			rif[n.ChiaveBozza] = n.Caselle[0].Rif
		}
	}
	for i := range ric.Acquisite {
		if _, ok := r.origini[ric.Acquisite[i].ChiaveBozza]; ok {
			p := r.allegatoPrima
			ric.Acquisite[i].Prima = &p
		}
	}
	nelRegistro := map[[2]string]bool{}
	for _, a := range nuovo.Proposta.Archi {
		if a.AllegatoID == allegato.String() {
			nelRegistro[[2]string{a.Padre, a.Figlio}] = true
		}
	}
	for i := range s.Archi {
		a := &s.Archi[i]
		if !r.archi[[2]string{a.Padre, a.Figlio}] || a.Origine != OrigineConferma {
			continue
		}
		rp, rf := rif[a.Padre], rif[a.Figlio]
		if rp != "" && rf != "" && nelRegistro[[2]string{rp, rf}] {
			a.Origine, a.Step = OrigineStep, &StepArco3{AllegatoID: allegato, Gruppo: gruppo}
		}
	}
}

// SenzaStruttura dice se la versione non ha una sorgente STEP con struttura (nessuna sorgente, o la riservata
// solo_prodotto): il suo cambio della fonte segue cambio-fonte-2.
func SenzaStruttura(s Snapshot3) bool {
	return s.Sorgente == nil || s.Sorgente.AllegatoID == nil || s.Sorgente.Gruppo == GruppoSoloProdotto
}

func pieno(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s != "" && s != "null" && s != "[]" && s != "{}" && s != `""`
}

// identitaSostenuta: stesso codice (byte per byte), stessa rev, stesso emittente; il namespace conta se il nodo ne ha uno.
func identitaSostenuta(n, v Identita) bool {
	return n.Codice != "" && n.Codice == v.Codice && n.Rev == v.Rev && n.Emittente == v.Emittente && (n.Namespace == "" || n.Namespace == v.Namespace)
}

const (
	relNessuna = iota
	relSostenuta
	relDebole
)

type candidato struct {
	chiave, voce string
	rel          int
	discordanti  bool
}

// relazioneNodoVoce: il legame fra un nodo senza casella e una voce (commento di testa, 3-bis).
func relazioneNodoVoce(n Nodo3, idv Identita, okv bool, de uuid.UUID, conDE bool) int {
	codiceSimile := okv && n.Identita.Codice != "" && strings.EqualFold(n.Identita.Codice, idv.Codice)
	if n.ComponenteID != nil {
		if conDE {
			if de == *n.ComponenteID {
				return relSostenuta
			}
			if codiceSimile {
				return relDebole
			}
			return relNessuna
		}
		if !codiceSimile {
			return relNessuna
		}
		if identitaSostenuta(n.Identita, idv) {
			return relSostenuta
		}
		return relDebole
	}
	if !codiceSimile {
		return relNessuna
	}
	if conDE || !identitaSostenuta(n.Identita, idv) {
		return relDebole
	}
	return relSostenuta
}

// candidabile: un nodo senza casella che 3-bis puo' abbinare: non il prodotto; registrato, manuale o deciso.
func candidabile(n Nodo3, prodotto uuid.UUID) bool {
	if len(n.Caselle) > 0 || (n.ComponenteID != nil && *n.ComponenteID == prodotto) {
		return false
	}
	switch n.Identita.Modo {
	case ModoRegistrato, ModoManuale, ModoDecisione:
		return true
	}
	return false
}

// RiconciliaEffetto: 3-bis, 3-ter e le regole degli archi, delle differenze e delle decisioni di cambio-fonte-2
// (commento di testa) sull'effetto di cambio-fonte-1 di una versione senza struttura verso il registro nuovo.
func RiconciliaEffetto(prima Snapshot3, nuovo importazione.Contenuto, eff propostabom.EffettoCambioFonte) (RiconciliazioneFonte3, error) {
	out := RiconciliazioneFonte3{Effetto: eff}
	allegato, err := uuid.Parse(nuovo.Proposta.Fonte.AllegatoID)
	if err != nil {
		return out, corpoMotivo("cambia_sorgente", MotivoGruppoNonImportabile, "il registro nuovo non ha la fonte")
	}
	voci := map[string]importazione.Voce{}
	for _, v := range nuovo.Proposta.Nodi {
		voci[v.Voce] = v
	}
	nodi := map[string]Nodo3{}
	var candidati []Nodo3
	for _, n := range prima.Nodi {
		nodi[n.ChiaveBozza] = n
		if candidabile(n, prima.ProdottoID) {
			candidati = append(candidati, n)
		}
	}
	// i candidati, voce per voce e nodo per nodo
	perVoce := map[string][]candidato{}
	perNodo := map[string][]candidato{}
	identitaVoce := map[string]Identita{}
	for _, vid := range eff.NodiCheEntrano {
		v, ok := voci[vid]
		if !ok {
			continue
		}
		idv, okv, err := IdentitaDellaProposta(v)
		if err != nil {
			return out, err
		}
		if okv {
			identitaVoce[vid] = idv
		}
		de, conDE := decisioneEsistente(v)
		if conDE && de == prima.ProdottoID {
			conDE = false // il raggruppamento (P-3): non e' un componente da riusare
		}
		discordanteVoce := pieno(v.Alternative) || pieno(v.MotivoLettura)
		for _, n := range candidati {
			rel := relazioneNodoVoce(n, idv, okv, de, conDE)
			if rel == relNessuna {
				continue
			}
			c := candidato{chiave: n.ChiaveBozza, voce: vid, rel: rel}
			if rel == relSostenuta && n.ComponenteID == nil {
				c.discordanti = discordanteVoce || len(n.EvidenzeIdentita) > 0
			}
			perVoce[vid] = append(perVoce[vid], c)
			perNodo[n.ChiaveBozza] = append(perNodo[n.ChiaveBozza], c)
		}
	}
	trattenute := map[string]bool{}
	abbinati := map[string]string{} // voce → chiave
	proposti := map[string]*AbbinamentoProposto3{}
	for _, vid := range eff.NodiCheEntrano {
		cc := perVoce[vid]
		if len(cc) == 0 {
			continue
		}
		c := cc[0]
		if len(cc) == 1 && len(perNodo[c.chiave]) == 1 && c.rel == relSostenuta && !c.discordanti {
			abbinati[vid] = c.chiave
			continue
		}
		trattenute[vid] = true
		for _, x := range cc {
			p := proposti[x.chiave]
			if p == nil {
				p = &AbbinamentoProposto3{ChiaveBozza: x.chiave}
				proposti[x.chiave] = p
			}
			p.Voci = append(p.Voci, vid)
		}
	}
	for k, p := range proposti {
		sort.Strings(p.Voci)
		ambiguo := len(perNodo[k]) > 1
		for _, vid := range p.Voci {
			ambiguo = ambiguo || len(perVoce[vid]) > 1
		}
		switch c := perNodo[k][0]; {
		case ambiguo:
			p.Motivo = AbbinamentoAmbiguo
		case c.rel == relDebole:
			p.Motivo = AbbinamentoIncompatibile
		default:
			p.Motivo = AbbinamentoEvidenzeDiscordanti
		}
		out.Proposti = append(out.Proposti, *p)
	}
	sort.Slice(out.Proposti, func(i, j int) bool { return out.Proposti[i].ChiaveBozza < out.Proposti[j].ChiaveBozza })

	// le voci che entrano davvero; i nodi abbinati
	rifDi := map[string]string{} // chiave_bozza → rif sull'occorrenza nuova
	presente := map[string]bool{}
	entrano := map[string]bool{} // rif
	var restano []string
	for _, vid := range eff.NodiCheEntrano {
		v := voci[vid]
		switch {
		case abbinati[vid] != "":
			k := abbinati[vid]
			a := Abbinamento3{ChiaveBozza: k, Voce: vid, Rif: v.Origine.Rif}
			if n := nodi[k]; n.Identita.Modo == ModoManuale {
				if idv, ok := identitaVoce[vid]; ok {
					x := idv
					a.identita = &x
				}
			}
			out.Abbinati = append(out.Abbinati, a)
			out.Acquisite = append(out.Acquisite, CasellaRiscritta3{ChiaveBozza: k, Rif: v.Origine.Rif, Dopo: allegato})
			rifDi[k] = v.Origine.Rif
			presente[v.Origine.Rif] = true
		case trattenute[vid]:
		default:
			restano = append(restano, vid)
			entrano[v.Origine.Rif] = true
			presente[v.Origine.Rif] = true
		}
	}
	sort.Slice(out.Abbinati, func(i, j int) bool { return out.Abbinati[i].ChiaveBozza < out.Abbinati[j].ChiaveBozza })
	sort.Slice(out.Acquisite, func(i, j int) bool { return out.Acquisite[i].ChiaveBozza < out.Acquisite[j].ChiaveBozza })
	out.Effetto.NodiCheEntrano = restano
	for _, n := range prima.Nodi {
		if n.ComponenteID != nil && *n.ComponenteID == prima.ProdottoID && nuovo.Gruppo.Radice != "" {
			rifDi[n.ChiaveBozza] = nuovo.Gruppo.Radice
			presente[nuovo.Gruppo.Radice] = true
			if len(n.Caselle) == 0 {
				// il nodo del prodotto prende la casella della radice (passo 2): anche questa e' una casella acquisita
				out.Acquisite = append(out.Acquisite, CasellaRiscritta3{ChiaveBozza: n.ChiaveBozza, Rif: nuovo.Gruppo.Radice, Dopo: allegato})
			}
		}
	}
	sort.Slice(out.Acquisite, func(i, j int) bool { return out.Acquisite[i].ChiaveBozza < out.Acquisite[j].ChiaveBozza })
	trattenutaRif := map[string]bool{}
	for vid := range trattenute {
		trattenutaRif[voci[vid].Origine.Rif] = true
	}
	abbinatoChiave := map[string]bool{}
	for _, a := range out.Abbinati {
		abbinatoChiave[a.ChiaveBozza] = true
	}

	// gli archi
	inBozza := map[[2]string]int{}
	genitori := map[string]int{}
	for _, a := range prima.Archi {
		genitori[a.Figlio]++
		if rp, rf := rifDi[a.Padre], rifDi[a.Figlio]; rp != "" && rf != "" {
			inBozza[[2]string{rp, rf}] = a.Qta
		}
	}
	chiavePerRif := map[string]string{}
	for k, r := range rifDi {
		chiavePerRif[r] = k
	}
	nelRegistro := map[[2]string]bool{}
	diffArchi := out.Effetto.Differenze.Archi
	for _, a := range nuovo.Proposta.Archi {
		if a.AllegatoID != allegato.String() {
			continue
		}
		nelRegistro[[2]string{a.Padre, a.Figlio}] = true
		aggiunto := propostabom.DifferenzaArco{AllegatoID: a.AllegatoID, Padre: a.Padre, Figlio: a.Figlio, Tipo: propostabom.DifferenzaAggiunto}
		if !presente[a.Padre] || !presente[a.Figlio] {
			if trattenutaRif[a.Padre] || trattenutaRif[a.Figlio] {
				diffArchi = append(diffArchi, aggiunto)
			}
			continue
		}
		if q, ok := inBozza[[2]string{a.Padre, a.Figlio}]; ok {
			if a.Quantita != nil && *a.Quantita != q {
				diffArchi = append(diffArchi, propostabom.DifferenzaArco{AllegatoID: a.AllegatoID, Padre: a.Padre, Figlio: a.Figlio,
					Tipo: propostabom.DifferenzaCambiato, Campi: []string{"quantita"}})
			}
			continue
		}
		figlioOrfano := !entrano[a.Figlio] && genitori[chiavePerRif[a.Figlio]] == 0
		if entrano[a.Figlio] || (entrano[a.Padre] && figlioOrfano) {
			out.archi = append(out.archi, arcoRif{padre: a.Padre, figlio: a.Figlio, qta: a.Quantita})
			continue
		}
		if entrano[a.Padre] || abbinatoChiave[chiavePerRif[a.Padre]] || abbinatoChiave[chiavePerRif[a.Figlio]] {
			diffArchi = append(diffArchi, aggiunto)
		}
	}
	for _, a := range prima.Archi {
		rp, rf := rifDi[a.Padre], rifDi[a.Figlio]
		if rp == "" || rf == "" || (!abbinatoChiave[a.Padre] && !abbinatoChiave[a.Figlio]) || nelRegistro[[2]string{rp, rf}] {
			continue
		}
		diffArchi = append(diffArchi, propostabom.DifferenzaArco{AllegatoID: allegato.String(), Padre: rp, Figlio: rf, Tipo: propostabom.DifferenzaTolto})
	}
	sort.SliceStable(diffArchi, func(i, j int) bool {
		a, b := diffArchi[i], diffArchi[j]
		if a.Padre != b.Padre {
			return a.Padre < b.Padre
		}
		if a.Figlio != b.Figlio {
			return a.Figlio < b.Figlio
		}
		return a.Tipo < b.Tipo
	})
	out.Effetto.Differenze.Archi = diffArchi

	// il tipo dei nodi abbinati
	tipoDiverso := map[string]bool{}
	for _, a := range out.Abbinati {
		n, v := nodi[a.ChiaveBozza], voci[a.Voce]
		if tipoDellaVoce(v) != n.Tipo {
			tipoDiverso[a.ChiaveBozza] = true
			out.Effetto.Differenze.Nodi = append(out.Effetto.Differenze.Nodi, propostabom.DifferenzaNodo{ChiaveBozza: a.ChiaveBozza, Voce: a.Voce,
				Tipo: propostabom.DifferenzaCambiato, Campi: []string{"tipo"}})
		}
	}

	// le decisioni che non sono piu' certe
	figli := len(out.Abbinati) > 0 || len(restano) > 0
	rivedi := map[propostabom.DaRivedere]bool{}
	for _, x := range out.Effetto.DaRivedere {
		rivedi[x] = true
	}
	for _, d := range prima.Decisioni {
		if d.Tipo == DecisioneMonoPezzo {
			if figli {
				rivedi[propostabom.DaRivedere{DecisioneID: d.ID, Motivo: RivedereFonteCambiata}] = true
			}
			continue
		}
		for _, a := range out.Abbinati {
			if !citaNodo(d, a.ChiaveBozza) || (d.Tipo == DecisioneConfermaCategoria && !tipoDiverso[a.ChiaveBozza]) {
				continue
			}
			rivedi[propostabom.DaRivedere{DecisioneID: d.ID, Motivo: RivedereFonteCambiata}] = true
		}
	}
	out.Effetto.DaRivedere = out.Effetto.DaRivedere[:0:0]
	for x := range rivedi {
		out.Effetto.DaRivedere = append(out.Effetto.DaRivedere, x)
	}
	sort.Slice(out.Effetto.DaRivedere, func(i, j int) bool {
		a, b := out.Effetto.DaRivedere[i], out.Effetto.DaRivedere[j]
		if a.ChiaveBozza != b.ChiaveBozza {
			return a.ChiaveBozza < b.ChiaveBozza
		}
		if a.DecisioneID != b.DecisioneID {
			return a.DecisioneID < b.DecisioneID
		}
		return a.Motivo < b.Motivo
	})
	return out, nil
}
