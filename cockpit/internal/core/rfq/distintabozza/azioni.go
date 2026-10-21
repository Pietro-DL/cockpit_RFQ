package distintabozza

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// Vocabolario e' la versione del vocabolario delle azioni (distinta_bozza_evento.vocabolario). Un'azione nuova, o
// un'azione che cambia significato, vuole il vocabolario 2: gli eventi gia' scritti restano leggibili con la loro
// versione (matrice 07 §1: «vocabolario versionato»; contratto rev. 3 §6.6).
const Vocabolario = 1

// I limiti delle azioni di un salvataggio. MaxPrimaDopo: quanti byte di testo puo' pesare prima o dopo di
// un'azione (il CHECK della 0022 ferma a 256 KiB di jsonb; se jsonb pesa di piu' del testo, Salva traduce il CHECK in
// CORPO_NON_VALIDO, RC-07). MaxPesoAzioni: la somma di prima, dopo e motivo di tutte le azioni; il corpo intero di
// R2, in B2, e' comunque al piu' 2 MiB + 4 KiB (RC-18).
const (
	MaxAzioni     = 2 * MaxVoci
	MaxPrimaDopo  = 128 << 10
	MaxPesoAzioni = MaxSnapshot
)

// Le azioni del vocabolario 1, con i nomi del contratto rev. 3 §6.6. Ognuna dice che cosa trova (prima) e che cosa
// lascia (dopo); il server le rifa' in ordine sullo snapshot precedente e il risultato deve essere esattamente lo
// snapshot nuovo (Riconcilia). sorgente_vista e sorgente_rifiutata sono informative: si registrano, non cambiano
// lo snapshot e non provano niente sulla struttura.
const (
	AzBozzaAperta       = "bozza_aperta"       // prima azione della prima versione: dopo = {partenza}
	AzSorgenteVista     = "sorgente_vista"     // informativa: dopo = {allegato_id, gruppo, sha256?}; chiave = nodo di destinazione (E2.2)
	AzSorgenteRifiutata = "sorgente_rifiutata" // informativa, con motivo: come sorgente_vista
	AzSorgenteScelta    = "sorgente_scelta"    // prima = fonte senza allegato o null, dopo = la fonte nuova
	AzSorgenteCambiata  = "sorgente_cambiata"  // prima = la fonte con allegato, dopo = la nuova o null; motivo
	AzRebase            = "rebase"             // prima = {base, sorgente}, dopo = {base, sorgente, rif_scaduti}; motivo
	AzNodoAggiunto      = "nodo_aggiunto"      // dopo = il nodo intero
	AzNodoTolto         = "nodo_tolto"         // prima = il nodo intero
	AzNodoModificato    = "nodo_modificato"    // prima/dopo = gli stessi campi fra codice, rev, descrizione, tipo, op, caselle, righe_proposta
	AzNodoRiusato       = "nodo_riusato"       // prima/dopo = {op, componente_id[, codice]}, dopo.op = riusa
	AzArcoAggiunto      = "arco_aggiunto"      // dopo = l'arco intero
	AzArcoTolto         = "arco_tolto"         // prima = l'arco intero
	AzArcoSpostato      = "arco_spostato"      // prima/dopo = l'arco intero: stesso figlio, padre diverso, qta anche diversa
	AzQtaModificata     = "qta_modificata"     // prima/dopo = {padre, figlio, qta}
	AzDecisioneAggiunta = "decisione_aggiunta" // dopo = la decisione
	AzDecisioneTolta    = "decisione_tolta"    // prima = la decisione
	AzFilePreassociato  = "file_preassociato"  // dopo = {proposta_id, chiave_bozza}
	AzFileSvincolato    = "file_svincolato"    // prima = {proposta_id, chiave_bozza}
	AzAnnulla           = "annulla"            // dopo = {n}: il server applica l'inverso dell'evento n
	AzRifai             = "rifai"              // dopo = {n}: il server riapplica l'evento n
)

// Le due operazioni interne con cui si applica l'inverso di nodo_riusato e dei cambi di fonte (annulla, e lo stato
// di partenza della prima versione). Non arrivano mai dal browser e non si scrivono: «_» non passa ck_dbe_azione.
const (
	azIdentitaRipristinata = "_identita_ripristinata"
	azSorgenteRipristinata = "_sorgente_ripristinata"
)

// Le partenze della prima versione (contratto rev. 3 §6.6, bozza_aperta.dopo.partenza).
const (
	PartenzaConferma   = "conferma"
	PartenzaRegistrato = "registrato"
	PartenzaStep       = "step"
	PartenzaVuota      = "vuota"
)

var partenze = map[string]bool{PartenzaConferma: true, PartenzaRegistrato: true, PartenzaStep: true, PartenzaVuota: true}

// AzioniDelVocabolario: le azioni che il browser puo' dichiarare, nell'ordine della tabella del contratto §6.6.
var AzioniDelVocabolario = []string{
	AzBozzaAperta, AzSorgenteVista, AzSorgenteRifiutata, AzSorgenteScelta, AzSorgenteCambiata, AzRebase,
	AzNodoAggiunto, AzNodoTolto, AzNodoModificato, AzNodoRiusato, AzArcoAggiunto, AzArcoTolto, AzArcoSpostato,
	AzQtaModificata, AzDecisioneAggiunta, AzDecisioneTolta, AzFilePreassociato, AzFileSvincolato, AzAnnulla, AzRifai,
}

var azioniNote = func() map[string]bool {
	m := map[string]bool{}
	for _, a := range AzioniDelVocabolario {
		m[a] = true
	}
	return m
}()

var azioniInformative = map[string]bool{AzSorgenteVista: true, AzSorgenteRifiutata: true}

// Le azioni che annulla e rifai possono nominare: quelle sulla struttura, che hanno un inverso esatto. Le fonti e il
// rebase no: hanno un motivo e un effetto fuori dalla bozza (la storia della fonte), e si cambiano con le loro azioni.
var azioniInvertibili = map[string]bool{
	AzNodoAggiunto: true, AzNodoTolto: true, AzNodoModificato: true, AzNodoRiusato: true, AzArcoAggiunto: true,
	AzArcoTolto: true, AzArcoSpostato: true, AzQtaModificata: true, AzDecisioneAggiunta: true, AzDecisioneTolta: true,
	AzFilePreassociato: true, AzFileSvincolato: true,
}

// Le origini che il browser puo' dichiarare: «conferma» e «server» le scrive solo il server (0022, ck_dbe_origine).
var originiDelBrowser = map[string]bool{"step": true, "utente": true}
var originiTutte = map[string]bool{"step": true, "utente": true, "conferma": true, "server": true}

// Azione e' un'azione dichiarata dalla pagina in un salvataggio (contratto rev. 3 §6.2, azioni[]).
type Azione struct {
	Azione      string          `json:"azione"`
	ChiaveBozza string          `json:"chiave_bozza,omitempty"`
	Prima       json.RawMessage `json:"prima,omitempty"`
	Dopo        json.RawMessage `json:"dopo,omitempty"`
	Origine     string          `json:"origine"`
	Motivo      string          `json:"motivo,omitempty"`
	// interna: un'applicazione calcolata dal server (l'inverso di un'azione, annulla e rifai), non dichiarata dalla
	// pagina: non ripete i controlli dei valori nuovi (un annulla rimette il codice registrato, anche lungo).
	interna bool
}

// Evento e' un'azione riconciliata, pronta per distinta_bozza_evento: prima e dopo in forma canonica, scritti dal
// server (non copiati dal corpo), la chiave della bozza dedotta dall'azione.
type Evento struct {
	Azione      string
	ChiaveBozza string
	Prima       json.RawMessage
	Dopo        json.RawMessage
	Origine     string
	Motivo      string
}

// Precedente restituisce un evento gia' salvato della bozza, per numero (annulla e rifai di una versione precedente).
// ok falso: l'evento non c'e'.
type Precedente func(n int) (ev Evento, ok bool, err error)

// Contesto: quello che Riconcilia deve sapere oltre agli snapshot. N0 e' il numero dell'ultimo evento gia' salvato
// della bozza (0 per la prima versione): gli eventi di questo salvataggio sono N0+1, N0+2, …
type Contesto struct {
	N0         int
	Precedente Precedente
}

// RebaseDichiarato e' l'effetto dell'azione rebase: la base e la fonte nuove, con il motivo. La richiesta di R2 deve
// portare lo stesso rebase nel suo campo «rebase» (contratto rev. 3 §6.7).
type RebaseDichiarato struct {
	Base     Base
	Sorgente *Sorgente
	Motivo   string
}

// Riconciliazione e' l'esito di Riconcilia: gli eventi da scrivere, e quello che chi salva controlla sul database.
type Riconciliazione struct {
	Eventi []Evento
	// FontiCitate: ogni allegato che le azioni nominano, anche di passaggio (prima e dopo dei cambi di fonte e del
	// rebase, le fonti viste o rifiutate): chi salva controlla che sia un allegato della RFQ, con lo sha256 quando
	// c'e' (RC-05).
	FontiCitate []Sorgente
	// BasiCitate: le basi nominate dal rebase, prima e dopo (RC-05).
	BasiCitate []Base
	// Strutturali: quante azioni hanno cambiato lo snapshot.
	Strutturali int
	// Partenza e StatoDiPartenza, solo per la prima versione: la partenza dichiarata da bozza_aperta e lo stato da cui
	// le azioni portano allo snapshot. Chi salva lo confronta con quello che ricostruisce dal database
	// (ControllaPartenza): Riconcilia da sola non lo puo' fare, perche' non legge il database (RC-02).
	Partenza        string
	StatoDiPartenza *Snapshot
	// Rebase: il rebase dichiarato, se c'e' (al piu' uno per salvataggio).
	Rebase *RebaseDichiarato
}

// Riconcilia confronta lo snapshot precedente (nil per la prima versione) con quello nuovo e valida le azioni
// dichiarate dal browser: le rifa' in ordine, controllando che ognuna trovi quello che dice (prima) e che ogni stato
// intermedio sia una bozza possibile, e il risultato deve essere esattamente lo snapshot nuovo. Cosi' nessun evento
// inventato diventa prova (un'azione che non trova il suo «prima» si rifiuta) e nessun cambiamento passa senza
// un'azione che lo dica (il risultato non coinciderebbe).
//
// Prima versione: la prima azione e' bozza_aperta con la partenza. Lo stato di partenza si ottiene disfacendo le
// azioni dallo snapshot nuovo (con le chiavi della pagina), e si restituisce in StatoDiPartenza: chi salva DEVE
// confrontarlo con quello ricostruito dal server (ControllaPartenza), altrimenti la prima versione non sarebbe
// confrontata con niente.
//
// Pura: niente database (gli eventi gia' salvati li da' Contesto.Precedente), niente orologio.
func Riconcilia(prima *Snapshot, dopo Snapshot, azioni []Azione, cx Contesto) (Riconciliazione, error) {
	return riconcilia(prima, dopo, azioni, cx, originiDelBrowser)
}

func riconcilia(prima *Snapshot, dopo Snapshot, azioni []Azione, cx Contesto, origini map[string]bool) (Riconciliazione, error) {
	var r Riconciliazione
	if len(azioni) > MaxAzioni {
		return r, corpo("azioni", "%d azioni, oltre il limite di %d", len(azioni), MaxAzioni)
	}
	peso := 0
	for _, a := range azioni {
		peso += len(a.Prima) + len(a.Dopo) + len(a.Motivo)
	}
	if peso > MaxPesoAzioni {
		return r, corpo("azioni", "le azioni pesano %d byte, oltre il limite di %d", peso, MaxPesoAzioni)
	}
	nuovo := dopo.copia()
	nuovo.normalizza()
	if prima != nil && prima.ProdottoID != nuovo.ProdottoID {
		return r, fuori("prodotto_id", "la bozza e' del prodotto %s, lo snapshot del %s", prima.ProdottoID, nuovo.ProdottoID)
	}
	for i, a := range azioni {
		if e := formaAzione(fmt.Sprintf("azioni[%d]", i), a, origini); e != nil {
			return r, e
		}
		if a.Azione == AzBozzaAperta && (prima != nil || i != 0) {
			return r, corpo(fmt.Sprintf("azioni[%d]", i), "bozza_aperta e' solo la prima azione della prima versione")
		}
	}

	var st *stato
	if prima == nil {
		if len(azioni) == 0 || azioni[0].Azione != AzBozzaAperta {
			return r, corpo("azioni", "la prima versione comincia con bozza_aperta")
		}
		partenza, e := leggiBozzaAperta("azioni[0]", azioni[0])
		if e != nil {
			return r, e
		}
		s0, err := statoDiPartenza(nuovo, azioni, cx)
		if err != nil {
			return r, err
		}
		r.Partenza, r.StatoDiPartenza = partenza.Partenza, &s0
		st = statoDa(s0)
	} else {
		st = statoDa(*prima)
	}

	for i, a := range azioni {
		campo := fmt.Sprintf("azioni[%d]", i)
		if a.Azione == AzBozzaAperta {
			ev, e := eventoBozzaAperta(campo, a, *r.StatoDiPartenza)
			if e != nil {
				return r, e
			}
			r.Eventi = append(r.Eventi, ev)
			continue
		}
		ev, err := st.applica(campo, a, &r, cx, prima == nil)
		if err != nil {
			return r, err
		}
		if !azioniInformative[a.Azione] {
			r.Strutturali++
		}
		r.Eventi = append(r.Eventi, ev)
	}
	if e := st.uguale(nuovo); e != nil {
		return r, e
	}
	return r, nil
}

// formaAzione: i controlli che non dipendono dallo stato.
func formaAzione(campo string, a Azione, origini map[string]bool) *Errore {
	if !azioniNote[a.Azione] {
		return corpo(campo+".azione", "azione sconosciuta nel vocabolario %d: %q", Vocabolario, a.Azione)
	}
	if !origini[a.Origine] {
		return corpo(campo+".origine", "origine %q non ammessa qui", a.Origine)
	}
	if a.ChiaveBozza != "" && !reChiave.MatchString(a.ChiaveBozza) {
		return corpo(campo+".chiave_bozza", "chiave della bozza non valida")
	}
	if a.Motivo != "" && !testoLibero(nfc(a.Motivo), MaxMotivo) {
		return corpo(campo+".motivo", "il motivo non e' vuoto, sta in %d caratteri e non ha caratteri di controllo", MaxMotivo)
	}
	if len(a.Prima) > MaxPrimaDopo || len(a.Dopo) > MaxPrimaDopo {
		return corpo(campo, "prima o dopo oltre %d byte", MaxPrimaDopo)
	}
	return nil
}

func nullo(r json.RawMessage) bool {
	t := bytes.TrimSpace(r)
	return len(t) == 0 || bytes.Equal(t, []byte("null"))
}

func canonico(v any) json.RawMessage {
	b, err := jsoncanonico.Codifica(v)
	if err != nil {
		return nil
	}
	return b
}

func uguali(a, b any) bool {
	ca, cb := canonico(a), canonico(b)
	return ca != nil && bytes.Equal(ca, cb)
}

// ------------------------------------------------------------------ la prima versione: bozza_aperta

type bozzaApertaDopo struct {
	Partenza string `json:"partenza"`
	Impronta string `json:"impronta"`
	Nodi     *int   `json:"nodi"`
	Archi    *int   `json:"archi"`
}

// leggiBozzaAperta: prima e chiave assenti, dopo con la partenza (gli altri campi li scrive il server; se la pagina
// o il giornale li portano, devono coincidere con lo stato di partenza).
func leggiBozzaAperta(campo string, a Azione) (bozzaApertaDopo, *Errore) {
	var d bozzaApertaDopo
	if !nullo(a.Prima) || a.ChiaveBozza != "" {
		return d, corpo(campo, "bozza_aperta non porta prima ne' chiave_bozza")
	}
	if nullo(a.Dopo) {
		return d, corpo(campo+".dopo.partenza", "bozza_aperta dice da dove parte: conferma, registrato, step o vuota")
	}
	if e := decodificaStretta(campo+".dopo", a.Dopo, &d); e != nil {
		return d, e
	}
	if !partenze[d.Partenza] {
		return d, corpo(campo+".dopo.partenza", "partenza %q sconosciuta (conferma, registrato, step, vuota)", d.Partenza)
	}
	return d, nil
}

// eventoBozzaAperta: il «dopo» lo scrive il server (partenza, impronta e dimensioni dello stato di partenza).
func eventoBozzaAperta(campo string, a Azione, s0 Snapshot) (Evento, *Errore) {
	d, e := leggiBozzaAperta(campo, a)
	if e != nil {
		return Evento{}, e
	}
	imp, err := s0.Impronta()
	if err != nil {
		return Evento{}, corpo("snapshot", "lo stato di partenza non ha una forma canonica: %v", err)
	}
	if (d.Impronta != "" && d.Impronta != imp) || (d.Nodi != nil && *d.Nodi != len(s0.Nodi)) || (d.Archi != nil && *d.Archi != len(s0.Archi)) {
		return Evento{}, nonRiconciliate(campo, "bozza_aperta descrive uno stato di partenza diverso da quello che le azioni disfano")
	}
	return Evento{Azione: AzBozzaAperta, Origine: a.Origine, Motivo: strings.TrimSpace(nfc(a.Motivo)),
		Dopo: canonico(map[string]any{"partenza": d.Partenza, "impronta": imp, "nodi": len(s0.Nodi), "archi": len(s0.Archi)})}, nil
}

// statoDiPartenza disfa le azioni della prima versione, dall'ultima alla seconda, partendo dallo snapshot nuovo:
// cio' che resta e' lo stato da cui la pagina e' partita, con le sue chiavi. Ogni passo all'indietro ha gli stessi
// controlli di quello in avanti, e lo stato che ne esce deve essere una bozza valida.
func statoDiPartenza(nuovo Snapshot, azioni []Azione, cx Contesto) (Snapshot, error) {
	st := statoDa(nuovo)
	var scarto Riconciliazione
	for i := len(azioni) - 1; i >= 1; i-- {
		a := azioni[i]
		campo := fmt.Sprintf("azioni[%d]", i)
		if azioniInformative[a.Azione] {
			continue
		}
		var inv Azione
		switch a.Azione {
		case AzRebase:
			return Snapshot{}, corpo(campo, "il rebase non serve nella prima versione: la base e' quella dello snapshot")
		case AzAnnulla, AzRifai:
			n, e := leggiNumeroEvento(campo, a)
			if e != nil {
				return Snapshot{}, e
			}
			if n < cx.N0+1 || n >= cx.N0+1+i {
				return Snapshot{}, nonRiconciliate(campo+".dopo.n", "%s nomina l'evento %d, che non e' prima di lui nella prima versione", a.Azione, n)
			}
			bersaglio := azioni[n-cx.N0-1]
			if !azioniInvertibili[bersaglio.Azione] {
				return Snapshot{}, nonRiconciliate(campo+".dopo.n", "%s nomina l'evento %d (%s), che non si annulla ne' si rifa'", a.Azione, n, bersaglio.Azione)
			}
			if a.Azione == AzAnnulla { // all'indietro, un annulla si disfa riapplicando il bersaglio
				inv = bersaglio
			} else {
				var ok bool
				if inv, ok = inversa(bersaglio); !ok {
					return Snapshot{}, nonRiconciliate(campo, "l'evento %d non ha un inverso", n)
				}
			}
		default:
			var ok bool
			if inv, ok = inversa(a); !ok {
				return Snapshot{}, nonRiconciliate(campo, "%s non si disfa", a.Azione)
			}
		}
		inv.Origine, inv.Motivo, inv.interna = "server", "", true
		if _, err := st.applica(campo, inv, &scarto, cx, true); err != nil {
			if e, ok := err.(*Errore); ok {
				return Snapshot{}, nonRiconciliate(campo, "le azioni, disfatte dallo snapshot salvato, non tornano allo stato di partenza: %s", e.Messaggio)
			}
			return Snapshot{}, err
		}
	}
	s0 := st.snapshot()
	if e := s0.valida(); e != nil {
		return Snapshot{}, nonRiconciliate("azioni", "lo stato di partenza che le azioni descrivono non e' una bozza valida: %s (%s)", e.Messaggio, e.Campo)
	}
	return s0, nil
}

// inversa: l'azione che disfa un'azione strutturale (o un cambio di fonte). Le forme di prima e dopo sono quelle
// dell'azione stessa: basta scambiarle e, per le coppie, cambiare verbo.
func inversa(a Azione) (Azione, bool) {
	inv := Azione{ChiaveBozza: a.ChiaveBozza, Prima: a.Dopo, Dopo: a.Prima, Origine: a.Origine, interna: true}
	switch a.Azione {
	case AzNodoAggiunto:
		inv.Azione = AzNodoTolto
	case AzNodoTolto:
		inv.Azione = AzNodoAggiunto
	case AzNodoModificato, AzArcoSpostato, AzQtaModificata:
		inv.Azione = a.Azione
	case AzNodoRiusato:
		inv.Azione = azIdentitaRipristinata
	case AzArcoAggiunto:
		inv.Azione = AzArcoTolto
	case AzArcoTolto:
		inv.Azione = AzArcoAggiunto
	case AzDecisioneAggiunta:
		inv.Azione = AzDecisioneTolta
	case AzDecisioneTolta:
		inv.Azione = AzDecisioneAggiunta
	case AzFilePreassociato:
		inv.Azione = AzFileSvincolato
	case AzFileSvincolato:
		inv.Azione = AzFilePreassociato
	case AzSorgenteScelta, AzSorgenteCambiata:
		inv.Azione = azSorgenteRipristinata
	default:
		return Azione{}, false
	}
	return inv, true
}

// ------------------------------------------------------------------ lo stato che le azioni cambiano

type stato struct {
	formato   int
	prodotto  uuid.UUID
	sorgente  *Sorgente
	base      *VersioneBomBase
	firma     string
	nodi      map[string]Nodo
	archi     map[[2]string]Arco
	decisioni map[string]json.RawMessage
	pre       map[uuid.UUID]string
	// gli indici dei controlli locali (RC-06), tenuti a ogni azione: archi che toccano un nodo, preassociazioni di un
	// nodo, quanti nodi hanno il componente del prodotto
	grado  map[string]int
	preDi  map[string]int
	radici int
}

func statoDa(s Snapshot) *stato {
	c := s.copia()
	c.normalizza()
	st := &stato{formato: c.Formato, prodotto: c.ProdottoID, sorgente: c.Sorgente, base: c.VersioneBomBase, firma: c.FirmaBase,
		nodi: map[string]Nodo{}, archi: map[[2]string]Arco{}, decisioni: map[string]json.RawMessage{}, pre: map[uuid.UUID]string{},
		grado: map[string]int{}, preDi: map[string]int{}}
	for _, n := range c.Nodi {
		st.nodi[n.ChiaveBozza] = n
		if st.eRadice(n) {
			st.radici++
		}
	}
	for _, a := range c.Archi {
		st.mettiArco(a)
	}
	for _, d := range c.Decisioni {
		st.decisioni[idDecisione(d)] = d
	}
	for _, p := range c.DocumentiPreassociati {
		st.pre[p.PropostaID] = p.ChiaveBozza
		st.preDi[p.ChiaveBozza]++
	}
	return st
}

func (st *stato) eRadice(n Nodo) bool { return n.ComponenteID != nil && *n.ComponenteID == st.prodotto }

func (st *stato) mettiArco(a Arco) {
	st.archi[[2]string{a.Padre, a.Figlio}] = a
	st.grado[a.Padre]++
	st.grado[a.Figlio]++
}

func (st *stato) togliArco(k [2]string) {
	delete(st.archi, k)
	st.grado[k[0]]--
	st.grado[k[1]]--
}

func (st *stato) snapshot() Snapshot {
	s := Snapshot{Formato: st.formato, ProdottoID: st.prodotto, Sorgente: st.sorgente, VersioneBomBase: st.base, FirmaBase: st.firma}
	for _, n := range st.nodi {
		s.Nodi = append(s.Nodi, n)
	}
	for _, a := range st.archi {
		s.Archi = append(s.Archi, a)
	}
	for _, d := range st.decisioni {
		s.Decisioni = append(s.Decisioni, d)
	}
	for p, k := range st.pre {
		s.DocumentiPreassociati = append(s.DocumentiPreassociati, Preassociazione2{PropostaID: p, ChiaveBozza: k})
	}
	s.normalizza()
	return s
}

// uguale dice se lo stato rifatto e' lo snapshot nuovo; se no, la prima differenza, detta per nome.
func (st *stato) uguale(nuovo Snapshot) *Errore {
	rifatto := st.snapshot()
	if uguali(rifatto, nuovo) {
		return nil
	}
	atteso := statoDa(nuovo)
	if !uguali(st.base, atteso.base) || st.firma != atteso.firma {
		// contratto rev. 3 §6.7: senza rebase, uno snapshot che cambia la base e' un CORPO_NON_VALIDO sul campo rebase
		return &Errore{Codice: CodiceCorpoNonValido, Motivo: MotivoAzioniNonRiconciliate, Campo: "rebase",
			Messaggio: "la base (versione_bom_base, firma_base) cambia, ma nessuna azione rebase lo dice"}
	}
	if !uguali(st.sorgente, atteso.sorgente) {
		return nonRiconciliate("snapshot.sorgente", "la fonte cambia, ma nessuna azione sorgente_scelta, sorgente_cambiata o rebase lo dice")
	}
	for _, k := range chiaviOrdinate(st.nodi, atteso.nodi) {
		a, okA := st.nodi[k]
		b, okB := atteso.nodi[k]
		switch {
		case okA && !okB:
			return nonRiconciliate("snapshot.nodi", "il nodo %s non c'e' piu', ma nessuna azione lo toglie", k)
		case !okA && okB:
			return nonRiconciliate("snapshot.nodi", "il nodo %s e' nuovo, ma nessuna azione lo aggiunge", k)
		case !uguali(a, b):
			return nonRiconciliate("snapshot.nodi", "il nodo %s e' cambiato, ma nessuna azione lo dice", k)
		}
	}
	for _, k := range coppieOrdinate(st.archi, atteso.archi) {
		a, okA := st.archi[k]
		b, okB := atteso.archi[k]
		switch {
		case okA && !okB:
			return nonRiconciliate("snapshot.archi", "l'arco %s → %s non c'e' piu', ma nessuna azione lo toglie", k[0], k[1])
		case !okA && okB:
			return nonRiconciliate("snapshot.archi", "l'arco %s → %s e' nuovo, ma nessuna azione lo aggiunge", k[0], k[1])
		case !uguali(a, b):
			return nonRiconciliate("snapshot.archi", "l'arco %s → %s e' cambiato, ma nessuna azione lo dice", k[0], k[1])
		}
	}
	if !uguali(rifatto.Decisioni, nuovo.Decisioni) {
		return nonRiconciliate("snapshot.decisioni", "le decisioni cambiano, ma le azioni non lo dicono")
	}
	if !uguali(rifatto.DocumentiPreassociati, nuovo.DocumentiPreassociati) {
		return nonRiconciliate("snapshot.documenti_preassociati", "le preassociazioni cambiano, ma le azioni non lo dicono")
	}
	return nonRiconciliate("snapshot", "le azioni non danno lo snapshot salvato")
}

func chiaviOrdinate(a, b map[string]Nodo) []string {
	m := map[string]bool{}
	for k := range a {
		m[k] = true
	}
	for k := range b {
		m[k] = true
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func coppieOrdinate(a, b map[[2]string]Arco) [][2]string {
	m := map[[2]string]bool{}
	for k := range a {
		m[k] = true
	}
	for k := range b {
		m[k] = true
	}
	out := make([][2]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}

// ------------------------------------------------------------------ le azioni, una per una

// applica rifa' un'azione sullo stato e restituisce il suo evento. Raccoglie in r le fonti e le basi citate e il
// rebase. primaVersione: siamo nella prima versione (nessun evento salvato prima).
func (st *stato) applica(campo string, a Azione, r *Riconciliazione, cx Contesto, primaVersione bool) (Evento, error) {
	ev := Evento{Azione: a.Azione, Origine: a.Origine, Motivo: strings.TrimSpace(nfc(a.Motivo))}
	chiave := func(dedotta string) *Errore {
		if a.ChiaveBozza != "" && a.ChiaveBozza != dedotta {
			return nonRiconciliate(campo+".chiave_bozza", "l'azione dichiara la chiave %s ma riguarda %s", a.ChiaveBozza, dedotta)
		}
		ev.ChiaveBozza = dedotta
		return nil
	}
	vuoto := func(raw json.RawMessage, nome string) *Errore {
		if !nullo(raw) {
			return corpo(campo+"."+nome, "%s non porta %s", a.Azione, nome)
		}
		return nil
	}
	var e *Errore
	switch a.Azione {
	case AzNodoAggiunto:
		if e = vuoto(a.Prima, "prima"); e != nil {
			return ev, e
		}
		var n Nodo
		if n, e = leggiNodo(campo+".dopo", a.Dopo); e != nil {
			return ev, e
		}
		if e = chiave(n.ChiaveBozza); e != nil {
			return ev, e
		}
		if _, c := st.nodi[n.ChiaveBozza]; c {
			return ev, nonRiconciliate(campo, "nodo_aggiunto: il nodo %s c'e' gia'", n.ChiaveBozza)
		}
		if e = st.nodoAmmesso(campo, n); e != nil {
			return ev, e
		}
		if st.eRadice(n) {
			if st.radici > 0 {
				return ev, nonRiconciliate(campo, "nodo_aggiunto: il nodo %s avrebbe il componente del prodotto, che ha gia' la sua radice", n.ChiaveBozza)
			}
			st.radici++
		}
		st.nodi[n.ChiaveBozza] = n
		ev.Dopo = canonico(n)
	case AzNodoTolto:
		if e = vuoto(a.Dopo, "dopo"); e != nil {
			return ev, e
		}
		var n Nodo
		if n, e = leggiNodo(campo+".prima", a.Prima); e != nil {
			return ev, e
		}
		if e = chiave(n.ChiaveBozza); e != nil {
			return ev, e
		}
		cur, c := st.nodi[n.ChiaveBozza]
		if !c || !uguali(cur, n) {
			return ev, nonRiconciliate(campo, "nodo_tolto: il nodo %s non e' quello che l'azione dice di togliere", n.ChiaveBozza)
		}
		if st.eRadice(cur) {
			return ev, nonRiconciliate(campo, "nodo_tolto: il nodo %s e' la radice del prodotto e non si toglie", n.ChiaveBozza)
		}
		if st.grado[n.ChiaveBozza] > 0 || st.preDi[n.ChiaveBozza] > 0 {
			return ev, nonRiconciliate(campo, "nodo_tolto: il nodo %s ha ancora archi o documenti preassociati (prima si tolgono quelli)", n.ChiaveBozza)
		}
		delete(st.nodi, n.ChiaveBozza)
		ev.Prima = canonico(cur)
	case AzNodoModificato, AzNodoRiusato, azIdentitaRipristinata:
		if a.ChiaveBozza == "" {
			return ev, corpo(campo+".chiave_bozza", "%s vuole la chiave del nodo", a.Azione)
		}
		if e = chiave(a.ChiaveBozza); e != nil {
			return ev, e
		}
		p, d, e := st.modificaNodo(campo, a)
		if e != nil {
			return ev, e
		}
		ev.Prima, ev.Dopo = p, d
	case AzArcoAggiunto:
		if e = vuoto(a.Prima, "prima"); e != nil {
			return ev, e
		}
		var x Arco
		if x, e = leggiArco(campo+".dopo", a.Dopo); e != nil {
			return ev, e
		}
		if e = chiave(x.Figlio); e != nil {
			return ev, e
		}
		k := [2]string{x.Padre, x.Figlio}
		if _, c := st.archi[k]; c {
			return ev, nonRiconciliate(campo, "arco_aggiunto: l'arco %s → %s c'e' gia'", x.Padre, x.Figlio)
		}
		if e = st.estremiPresenti(campo, x); e != nil {
			return ev, e
		}
		st.mettiArco(x)
		ev.Dopo = canonico(x)
	case AzArcoTolto:
		if e = vuoto(a.Dopo, "dopo"); e != nil {
			return ev, e
		}
		var x Arco
		if x, e = leggiArco(campo+".prima", a.Prima); e != nil {
			return ev, e
		}
		if e = chiave(x.Figlio); e != nil {
			return ev, e
		}
		k := [2]string{x.Padre, x.Figlio}
		cur, c := st.archi[k]
		if !c || !uguali(cur, x) {
			return ev, nonRiconciliate(campo, "arco_tolto: l'arco %s → %s non e' quello che l'azione dice di togliere", x.Padre, x.Figlio)
		}
		st.togliArco(k)
		ev.Prima = canonico(cur)
	case AzQtaModificata:
		var p, d struct {
			Padre  string `json:"padre"`
			Figlio string `json:"figlio"`
			Qta    *int   `json:"qta"`
		}
		if e = decodificaStretta(campo+".prima", a.Prima, &p); e != nil {
			return ev, e
		}
		if e = decodificaStretta(campo+".dopo", a.Dopo, &d); e != nil {
			return ev, e
		}
		if p.Qta == nil || d.Qta == nil || p.Padre != d.Padre || p.Figlio != d.Figlio {
			return ev, corpo(campo, "qta_modificata vuole prima e dopo con lo stesso padre e figlio e la qta")
		}
		if e = chiave(p.Figlio); e != nil {
			return ev, e
		}
		k := [2]string{p.Padre, p.Figlio}
		cur, c := st.archi[k]
		if !c || cur.Qta != *p.Qta {
			return ev, nonRiconciliate(campo, "qta_modificata: l'arco %s → %s non ha la qta %d che l'azione dice di trovare", p.Padre, p.Figlio, *p.Qta)
		}
		if *d.Qta == cur.Qta {
			return ev, nonRiconciliate(campo, "qta_modificata: la qta non cambia")
		}
		nuovoArco := cur
		nuovoArco.Qta = *d.Qta
		if e = validaArco(campo+".dopo", nuovoArco); e != nil {
			return ev, e
		}
		st.archi[k] = nuovoArco
		ev.Prima = canonico(map[string]any{"padre": p.Padre, "figlio": p.Figlio, "qta": cur.Qta})
		ev.Dopo = canonico(map[string]any{"padre": p.Padre, "figlio": p.Figlio, "qta": *d.Qta})
	case AzArcoSpostato:
		var x, y Arco
		if x, e = leggiArco(campo+".prima", a.Prima); e != nil {
			return ev, e
		}
		if y, e = leggiArco(campo+".dopo", a.Dopo); e != nil {
			return ev, e
		}
		// contratto rev. 3 §6.6: «prima.padre → dopo.padre, figlio, qta»; origine e posizione restano (RC-13)
		if x.Figlio != y.Figlio || x.Padre == y.Padre || x.Origine != y.Origine || x.Posizione != y.Posizione {
			return ev, corpo(campo, "arco_spostato cambia il padre dello stesso figlio (e al piu' la qta): origine e posizione restano")
		}
		if e = chiave(x.Figlio); e != nil {
			return ev, e
		}
		kx, ky := [2]string{x.Padre, x.Figlio}, [2]string{y.Padre, y.Figlio}
		cur, c := st.archi[kx]
		if !c || !uguali(cur, x) {
			return ev, nonRiconciliate(campo, "arco_spostato: l'arco %s → %s non e' quello che l'azione dice di spostare", x.Padre, x.Figlio)
		}
		if _, c := st.archi[ky]; c {
			return ev, nonRiconciliate(campo, "arco_spostato: l'arco %s → %s c'e' gia'", y.Padre, y.Figlio)
		}
		if e = st.estremiPresenti(campo, y); e != nil {
			return ev, e
		}
		st.togliArco(kx)
		st.mettiArco(y)
		ev.Prima, ev.Dopo = canonico(cur), canonico(y)
	case AzDecisioneAggiunta, AzDecisioneTolta:
		p, d, e := st.decisione(campo, a)
		if e != nil {
			return ev, e
		}
		// la chiave e' quella del nodo a cui la decisione si riferisce, se ne nomina uno (d1, senza_padre, …)
		src := d
		if src == nil {
			src = p
		}
		if e := chiave(chiaveDecisione(src)); e != nil {
			return ev, e
		}
		ev.Prima, ev.Dopo = p, d
	case AzFilePreassociato, AzFileSvincolato:
		var x Preassociazione2
		src, altro, nome := a.Dopo, a.Prima, "dopo"
		if a.Azione == AzFileSvincolato {
			src, altro, nome = a.Prima, a.Dopo, "prima"
		}
		if !nullo(altro) {
			return ev, corpo(campo, "%s porta solo %s", a.Azione, nome)
		}
		if e = decodificaStretta(campo+"."+nome, src, &x); e != nil {
			return ev, e
		}
		if x.PropostaID == uuid.Nil || !reChiave.MatchString(x.ChiaveBozza) {
			return ev, corpo(campo+"."+nome, "proposta_id e chiave_bozza")
		}
		if e = chiave(x.ChiaveBozza); e != nil {
			return ev, e
		}
		cur, c := st.pre[x.PropostaID]
		if a.Azione == AzFilePreassociato {
			if c {
				return ev, nonRiconciliate(campo, "file_preassociato: la proposta %s e' gia' preassociata", x.PropostaID)
			}
			n, ok := st.nodi[x.ChiaveBozza]
			if !ok || n.Op == OpEsclude {
				return ev, nonRiconciliate(campo, "file_preassociato: il nodo %s non c'e' o e' escluso", x.ChiaveBozza)
			}
			st.pre[x.PropostaID] = x.ChiaveBozza
			st.preDi[x.ChiaveBozza]++
			ev.Dopo = canonico(x)
		} else {
			if !c || cur != x.ChiaveBozza {
				return ev, nonRiconciliate(campo, "file_svincolato: la proposta %s non e' preassociata a %s", x.PropostaID, x.ChiaveBozza)
			}
			delete(st.pre, x.PropostaID)
			st.preDi[x.ChiaveBozza]--
			ev.Prima = canonico(x)
		}
	case AzSorgenteScelta, AzSorgenteCambiata, azSorgenteRipristinata:
		var p, d *Sorgente
		if p, e = leggiSorgenteONull(campo+".prima", a.Prima); e != nil {
			return ev, e
		}
		if d, e = leggiSorgenteONull(campo+".dopo", a.Dopo); e != nil {
			return ev, e
		}
		if e = chiave(""); e != nil {
			return ev, e
		}
		if !uguali(p, st.sorgente) {
			return ev, nonRiconciliate(campo, "%s: la fonte di prima non e' quella della bozza", a.Azione)
		}
		if uguali(p, d) {
			return ev, nonRiconciliate(campo, "%s: la fonte non cambia", a.Azione)
		}
		switch a.Azione {
		case AzSorgenteScelta: // dalla nessuna fonte (o da una senza allegato) a una fonte
			if (p != nil && p.AllegatoID != nil) || d == nil {
				return ev, corpo(campo, "sorgente_scelta va da nessuna fonte (o da una senza allegato) a una fonte: per cambiarla c'e' sorgente_cambiata")
			}
		case AzSorgenteCambiata: // il trigger della 0022 vuole un evento motivato quando cambia una fonte con allegato
			if p == nil || p.AllegatoID == nil {
				return ev, corpo(campo, "sorgente_cambiata parte da una fonte con allegato: la prima scelta e' sorgente_scelta")
			}
			if ev.Motivo == "" {
				return ev, corpo(campo+".motivo", "cambiare una fonte gia' scelta vuole il motivo")
			}
		}
		r.citaFonte(p)
		r.citaFonte(d)
		st.sorgente = d
		ev.Prima, ev.Dopo = canonicoONull(p), canonicoONull(d)
	case AzRebase:
		if primaVersione {
			return ev, corpo(campo, "il rebase non serve nella prima versione: la base e' quella dello snapshot")
		}
		if r.Rebase != nil {
			return ev, corpo(campo, "un salvataggio porta al piu' un rebase")
		}
		if ev.Motivo == "" {
			return ev, corpo(campo+".motivo", "il rebase vuole il motivo (contratto §6.6)")
		}
		if e = chiave(""); e != nil {
			return ev, e
		}
		p, d, e := leggiRebase(campo, a)
		if e != nil {
			return ev, e
		}
		if !uguali(p.Base.versione(), st.base) || p.Base.Firma != st.firma || !uguali(p.Sorgente, st.sorgente) {
			return ev, nonRiconciliate(campo, "rebase: la base o la fonte di prima non sono quelle della bozza")
		}
		if uguali(p.Base, d.Base) && uguali(p.Sorgente, d.Sorgente) {
			return ev, nonRiconciliate(campo, "rebase: base e fonte non cambiano")
		}
		r.BasiCitate = append(r.BasiCitate, p.Base, d.Base)
		r.citaFonte(p.Sorgente)
		r.citaFonte(d.Sorgente)
		st.base, st.firma, st.sorgente = d.Base.versione(), d.Base.Firma, d.Sorgente
		r.Rebase = &RebaseDichiarato{Base: d.Base, Sorgente: d.Sorgente, Motivo: ev.Motivo}
		ev.Prima = canonico(map[string]any{"base": p.Base, "sorgente": p.Sorgente})
		ev.Dopo = canonico(map[string]any{"base": d.Base, "sorgente": d.Sorgente, "rif_scaduti": d.RifScaduti})
	case AzSorgenteVista, AzSorgenteRifiutata:
		if e = vuoto(a.Prima, "prima"); e != nil {
			return ev, e
		}
		s, e := leggiCitazione(campo+".dopo", a.Dopo)
		if e != nil {
			return ev, e
		}
		if a.Azione == AzSorgenteRifiutata && ev.Motivo == "" {
			return ev, corpo(campo+".motivo", "un rifiuto della sorgente vuole il motivo")
		}
		// E2.2 (D-SHOCK-S2 B [U]): la fonte di un sottoassieme ha la sua destinazione; se il nodo esiste solo nella
		// bozza, la destinazione temporanea e' la sua chiave. Senza chiave: la fonte del prodotto.
		if a.ChiaveBozza != "" {
			n, ok := st.nodi[a.ChiaveBozza]
			if !ok || st.eRadice(n) {
				return ev, nonRiconciliate(campo+".chiave_bozza", "%s: la destinazione %s non e' un nodo della bozza diverso dalla radice", a.Azione, a.ChiaveBozza)
			}
		}
		ev.ChiaveBozza = a.ChiaveBozza
		r.citaFonte(&s)
		ev.Dopo = canonico(s)
	case AzAnnulla, AzRifai:
		if e = vuoto(a.Prima, "prima"); e != nil {
			return ev, e
		}
		n, e := leggiNumeroEvento(campo, a)
		if e != nil {
			return ev, e
		}
		bersaglio, err := r.evento(n, cx)
		if err != nil {
			return ev, err
		}
		if bersaglio == nil {
			return ev, nonRiconciliate(campo+".dopo.n", "%s nomina l'evento %d, che non c'e' prima di lui", a.Azione, n)
		}
		if !azioniInvertibili[bersaglio.Azione] {
			return ev, nonRiconciliate(campo+".dopo.n", "%s nomina l'evento %d (%s), che non si annulla ne' si rifa'", a.Azione, n, bersaglio.Azione)
		}
		if e := chiave(bersaglio.ChiaveBozza); e != nil {
			return ev, e
		}
		eff := Azione{Azione: bersaglio.Azione, ChiaveBozza: bersaglio.ChiaveBozza, Prima: bersaglio.Prima, Dopo: bersaglio.Dopo, Origine: "server"}
		if a.Azione == AzAnnulla {
			eff, _ = inversa(eff)
		}
		eff.interna = true
		var scarto Riconciliazione
		if _, err := st.applica(campo, eff, &scarto, cx, primaVersione); err != nil {
			if x, ok := err.(*Errore); ok {
				return ev, nonRiconciliate(campo, "%s dell'evento %d: %s", a.Azione, n, x.Messaggio)
			}
			return ev, err
		}
		ev.Dopo = canonico(map[string]int{"n": n})
	default:
		return ev, corpo(campo+".azione", "azione %q non applicabile", a.Azione)
	}
	return ev, nil
}

// evento: l'evento numero n, gia' salvato (Contesto.Precedente) o di questo salvataggio (gia' riconciliato).
func (r *Riconciliazione) evento(n int, cx Contesto) (*Evento, error) {
	if n >= 1 && n <= cx.N0 {
		if cx.Precedente == nil {
			return nil, nil
		}
		ev, ok, err := cx.Precedente(n)
		if err != nil || !ok {
			return nil, err
		}
		return &ev, nil
	}
	if i := n - cx.N0 - 1; i >= 0 && i < len(r.Eventi) {
		ev := r.Eventi[i]
		return &ev, nil
	}
	return nil, nil
}

func (r *Riconciliazione) citaFonte(s *Sorgente) {
	if s != nil && s.AllegatoID != nil {
		r.FontiCitate = append(r.FontiCitate, *s)
	}
}

func leggiNumeroEvento(campo string, a Azione) (int, *Errore) {
	var d struct {
		N *int `json:"n"`
	}
	if e := decodificaStretta(campo+".dopo", a.Dopo, &d); e != nil {
		return 0, e
	}
	if d.N == nil || *d.N < 1 {
		return 0, corpo(campo+".dopo.n", "%s nomina un evento con il suo numero (n >= 1)", a.Azione)
	}
	return *d.N, nil
}

// nodoAmmesso: i controlli locali su un nodo che entra o cambia (RC-06): finito solo sulla radice.
func (st *stato) nodoAmmesso(campo string, n Nodo) *Errore {
	if n.Tipo == "finito" && !st.eRadice(n) {
		return nonRiconciliate(campo, "il nodo %s sarebbe finito senza essere la radice del prodotto", n.ChiaveBozza)
	}
	return nil
}

// estremiPresenti: un arco tocca solo nodi che ci sono gia' (RC-06).
func (st *stato) estremiPresenti(campo string, a Arco) *Errore {
	for _, k := range []string{a.Padre, a.Figlio} {
		if _, ok := st.nodi[k]; !ok {
			return nonRiconciliate(campo, "l'arco %s → %s tocca il nodo %s, che non c'e' (si aggiunge prima il nodo)", a.Padre, a.Figlio, k)
		}
	}
	return nil
}

// leggiNodo legge un nodo intero da prima o dopo, con le regole di forma di un nodo dello snapshot.
func leggiNodo(campo string, raw json.RawMessage) (Nodo, *Errore) {
	if nullo(raw) {
		return Nodo{}, corpo(campo, "manca il nodo")
	}
	var in nodoIn
	if e := decodificaStretta(campo, raw, &in); e != nil {
		return Nodo{}, e
	}
	n, e := in.converti(campo)
	if e != nil {
		return n, e
	}
	if e := validaNodo(campo, n); e != nil {
		return n, e
	}
	normalizzaNodo(&n)
	return n, nil
}

func leggiArco(campo string, raw json.RawMessage) (Arco, *Errore) {
	if nullo(raw) {
		return Arco{}, corpo(campo, "manca l'arco")
	}
	var in arcoIn
	if e := decodificaStretta(campo, raw, &in); e != nil {
		return Arco{}, e
	}
	x, e := in.converti(campo)
	if e != nil {
		return x, e
	}
	return x, validaArco(campo, x)
}

func leggiSorgenteONull(campo string, raw json.RawMessage) (*Sorgente, *Errore) {
	if nullo(raw) {
		return nil, nil
	}
	var s Sorgente
	if e := decodificaStretta(campo, raw, &s); e != nil {
		return nil, e
	}
	sn := sorgenteNFC(&s)
	if e := sn.valida(campo); e != nil {
		return nil, e
	}
	return sn, nil
}

// leggiCitazione: la fonte nominata da sorgente_vista e sorgente_rifiutata (contratto §6.6: {allegato_id, gruppo});
// lo sha256 e' facoltativo e, se c'e', chi salva lo confronta con quello dell'allegato.
func leggiCitazione(campo string, raw json.RawMessage) (Sorgente, *Errore) {
	var s Sorgente
	if nullo(raw) {
		return s, corpo(campo, "manca la fonte: allegato_id e gruppo")
	}
	if e := decodificaStretta(campo, raw, &s); e != nil {
		return s, e
	}
	s.Gruppo = nfc(s.Gruppo)
	if s.AllegatoID == nil || *s.AllegatoID == uuid.Nil {
		return s, corpo(campo+".allegato_id", "la fonte citata e' un allegato")
	}
	if !testoLibero(s.Gruppo, MaxGruppo) {
		return s, corpo(campo+".gruppo", "il gruppo e' obbligatorio, al piu' %d caratteri, senza caratteri di controllo", MaxGruppo)
	}
	if s.Sha256 != "" && !reSha256.MatchString(s.Sha256) {
		return s, corpo(campo+".sha256", "lo sha256 e' di 64 cifre esadecimali minuscole")
	}
	return s, nil
}

func canonicoONull(s *Sorgente) json.RawMessage {
	if s == nil {
		return json.RawMessage("null")
	}
	return canonico(s)
}

type rebasePrima struct {
	Base     Base      `json:"base"`
	Sorgente *Sorgente `json:"sorgente"`
}

type rebaseDopo struct {
	Base       Base      `json:"base"`
	Sorgente   *Sorgente `json:"sorgente"`
	RifScaduti []string  `json:"rif_scaduti"`
}

func leggiRebase(campo string, a Azione) (rebasePrima, rebaseDopo, *Errore) {
	var p rebasePrima
	var d rebaseDopo
	if e := decodificaStretta(campo+".prima", a.Prima, &p); e != nil {
		return p, d, e
	}
	if e := decodificaStretta(campo+".dopo", a.Dopo, &d); e != nil {
		return p, d, e
	}
	for _, b := range []struct {
		nome string
		v    Base
	}{{"prima", p.Base}, {"dopo", d.Base}} {
		if e := b.v.valida(campo + "." + b.nome + ".base"); e != nil {
			return p, d, e
		}
	}
	p.Sorgente, d.Sorgente = sorgenteNFC(p.Sorgente), sorgenteNFC(d.Sorgente)
	if e := p.Sorgente.valida(campo + ".prima.sorgente"); e != nil {
		return p, d, e
	}
	if e := d.Sorgente.valida(campo + ".dopo.sorgente"); e != nil {
		return p, d, e
	}
	if len(d.RifScaduti) > MaxVoci {
		return p, d, corpo(campo+".dopo.rif_scaduti", "troppi rif scaduti (al piu' %d)", MaxVoci)
	}
	for i, x := range d.RifScaduti {
		if !reRif.MatchString(x) || runeOltre(x, MaxRif) || haControlli(x) {
			return p, d, corpo(fmt.Sprintf("%s.dopo.rif_scaduti[%d]", campo, i), "rif nella forma nodo:<sha256>:<chiave>")
		}
	}
	sort.Strings(d.RifScaduti)
	if d.RifScaduti == nil {
		d.RifScaduti = []string{}
	}
	return p, d, nil
}

// I campi di un nodo che ogni azione di modifica puo' toccare (la chiave mai: e' l'identita' del nodo nella bozza;
// l'origine resta quella con cui il nodo e' entrato).
var (
	campiNodoModificato = []string{"caselle", "codice", "descrizione", "op", "rev", "righe_proposta", "tipo"}
	campiNodoRiusato    = []string{"codice", "componente_id", "op"}
)

// modificaNodo applica nodo_modificato, nodo_riusato e il suo inverso: prima deve coincidere con il nodo corrente
// sui campi che nomina, dopo da' i valori nuovi, e il nodo che ne esce deve avere la forma di un nodo e passare i
// controlli locali.
func (st *stato) modificaNodo(campo string, a Azione) (json.RawMessage, json.RawMessage, *Errore) {
	cur, c := st.nodi[a.ChiaveBozza]
	if !c {
		return nil, nil, nonRiconciliate(campo, "%s: il nodo %s non c'e'", a.Azione, a.ChiaveBozza)
	}
	var p, d map[string]json.RawMessage
	if e := decodificaStretta(campo+".prima", a.Prima, &p); e != nil {
		return nil, nil, e
	}
	if e := decodificaStretta(campo+".dopo", a.Dopo, &d); e != nil {
		return nil, nil, e
	}
	chiaviP, chiaviD := chiaviDi(p), chiaviDi(d)
	if len(chiaviP) == 0 || !slices.Equal(chiaviP, chiaviD) {
		return nil, nil, corpo(campo, "%s: prima e dopo nominano gli stessi campi, almeno uno", a.Azione)
	}
	ammessi := campiNodoModificato
	if a.Azione != AzNodoModificato {
		ammessi = campiNodoRiusato
	}
	for _, k := range chiaviP {
		if !slices.Contains(ammessi, k) {
			return nil, nil, corpo(campo, "%s: il campo %q del nodo non si cambia cosi' (ammessi: %s)", a.Azione, k, strings.Join(ammessi, ", "))
		}
	}
	primaNodo, e := sovrapponi(campo+".prima", cur, p)
	if e != nil {
		return nil, nil, e
	}
	if !uguali(primaNodo, cur) {
		return nil, nil, nonRiconciliate(campo, "%s: il nodo %s non ha i valori che l'azione dice di trovare", a.Azione, a.ChiaveBozza)
	}
	dopoNodo, e := sovrapponi(campo+".dopo", cur, d)
	if e != nil {
		return nil, nil, e
	}
	if uguali(dopoNodo, cur) {
		return nil, nil, nonRiconciliate(campo, "%s: il nodo %s non cambia", a.Azione, a.ChiaveBozza)
	}
	if a.Azione == AzNodoRiusato {
		if !slices.Contains(chiaviP, "op") || !slices.Contains(chiaviP, "componente_id") || dopoNodo.Op != OpRiusa || dopoNodo.ComponenteID == nil {
			return nil, nil, corpo(campo, "nodo_riusato porta op e componente_id (e al piu' il codice registrato): dopo, op riusa con il componente")
		}
	}
	// Δ31: un codice che cambia e' un codice nuovo, con la regola dei codici nuovi
	if dopoNodo.Codice != cur.Codice && dopoNodo.Codice != "" && a.Azione == AzNodoModificato && !a.interna && !classificazione.CodiceAmmissibile(dopoNodo.Codice) {
		return nil, nil, corpo(campo+".dopo.codice", "un codice nuovo e' senza spazi e al piu' %d caratteri", classificazione.MaxCodice)
	}
	if e := st.nodoAmmesso(campo, dopoNodo); e != nil {
		return nil, nil, e
	}
	if st.eRadice(cur) != st.eRadice(dopoNodo) {
		return nil, nil, nonRiconciliate(campo, "%s: il nodo %s diventerebbe (o smetterebbe di essere) la radice del prodotto", a.Azione, a.ChiaveBozza)
	}
	if dopoNodo.Op == OpEsclude && st.preDi[a.ChiaveBozza] > 0 {
		return nil, nil, nonRiconciliate(campo, "%s: il nodo %s ha documenti preassociati e non si esclude (prima si svincolano)", a.Azione, a.ChiaveBozza)
	}
	st.nodi[a.ChiaveBozza] = dopoNodo
	return sottoinsieme(cur, chiaviP), sottoinsieme(dopoNodo, chiaviP), nil
}

func chiaviDi(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// sovrapponi: il nodo corrente con i campi dati al posto dei suoi, riletto con le regole di un nodo.
func sovrapponi(campo string, cur Nodo, campi map[string]json.RawMessage) (Nodo, *Errore) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(canonico(cur), &m); err != nil {
		return Nodo{}, corpo(campo, "nodo illeggibile: %v", err)
	}
	for k, v := range campi {
		if nullo(v) {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
	return leggiNodo(campo, canonico(m))
}

// sottoinsieme: i campi nominati del nodo, in forma canonica (null per un campo assente).
func sottoinsieme(n Nodo, chiavi []string) json.RawMessage {
	var m map[string]json.RawMessage
	_ = json.Unmarshal(canonico(n), &m)
	out := map[string]json.RawMessage{}
	for _, k := range chiavi {
		if v, ok := m[k]; ok {
			out[k] = v
		} else {
			out[k] = json.RawMessage("null")
		}
	}
	return canonico(out)
}

// chiaveDecisione: il campo chiave_bozza di una decisione, se c'e' ed e' una chiave valida; altrimenti "".
func chiaveDecisione(c json.RawMessage) string {
	var x struct {
		ChiaveBozza string `json:"chiave_bozza"`
	}
	if err := json.Unmarshal(c, &x); err != nil || !reChiave.MatchString(x.ChiaveBozza) {
		return ""
	}
	return x.ChiaveBozza
}

// decisione: decisione_aggiunta e decisione_tolta (il contratto non ha una modifica: si toglie e si aggiunge).
func (st *stato) decisione(campo string, a Azione) (json.RawMessage, json.RawMessage, *Errore) {
	var p, d json.RawMessage
	var idP, idD string
	var e *Errore
	if !nullo(a.Prima) {
		if p, idP, e = leggiDecisione(campo+".prima", a.Prima); e != nil {
			return nil, nil, e
		}
	}
	if !nullo(a.Dopo) {
		if d, idD, e = leggiDecisione(campo+".dopo", a.Dopo); e != nil {
			return nil, nil, e
		}
	}
	switch a.Azione {
	case AzDecisioneAggiunta:
		if p != nil || d == nil {
			return nil, nil, corpo(campo, "decisione_aggiunta porta solo dopo")
		}
		if _, c := st.decisioni[idD]; c {
			return nil, nil, nonRiconciliate(campo, "decisione_aggiunta: la decisione %s c'e' gia'", idD)
		}
		st.decisioni[idD] = d
	case AzDecisioneTolta:
		if p == nil || d != nil {
			return nil, nil, corpo(campo, "decisione_tolta porta solo prima")
		}
		if cur, c := st.decisioni[idP]; !c || !bytes.Equal(cur, p) {
			return nil, nil, nonRiconciliate(campo, "decisione_tolta: la decisione %s non e' quella che l'azione dice di togliere", idP)
		}
		delete(st.decisioni, idP)
	}
	return p, d, nil
}
