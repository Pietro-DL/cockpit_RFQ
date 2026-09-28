package fascicolo

// Il flusso ancorato al prodotto, passi 3 e 4 (Smistamento F8, addendum A5.13.4): i nodi degli STEP come
// evidenze, la raggiungibilita' dei sottoassiemi e l'indice di tutti i codici rilevati.
//
// Nessuno scrittore nuovo: i nodi e gli archi li scrive ApplicaStruttura (A5.4.4), e qui si LEGGONO, con le
// correzioni dell'operatore (il codice scritto da una persona; i nodi e gli archi che una persona ha scartato
// restano fuori). Ogni voce dell'indice dice con che AUTORITA' il codice c'e' (U3, Domanda 1 = B):
//   - decisa: un prodotto, o un componente attivo raggiungibile da un prodotto ancorato per un cammino che una
//     persona ha deciso tutto: il componente e ogni arco (lo stesso criterio della Struttura BOM, F7);
//   - diretta: un figlio diretto del pezzo nel file che ne porta la struttura. Nell'autorita' di un file
//     autorizzato e' una proposta vera; in un file non ancora autorizzato e' un candidato bloccato
//     («autorizza prima lo STEP»);
//   - profonda: un nodo piu' in basso. La guida si vede, anche profonda, e spiega dove vanno i disegni, ma
//     non comanda niente: la struttura sotto 7120010 la propone lo STEP di 7120010, o lo stesso file
//     autorizzato anche per 7120010. Nessun «accetta guida» (ter). Sono guida anche i discendenti di un
//     commerciale, e i nodi dello STEP di un commerciale: non e' una sorgente strutturale finche' una persona
//     non ne cambia il tipo (Domanda 5 = B), ne' lo STEP di un suo discendente; e i nodi di uno STEP che e'
//     un'alternativa al file che ancora il pezzo (A5.13.3: le alternative si mostrano, non sono autorita');
//   - da_rivedere: un componente della working sotto un prodotto ancorato, ma nato da un'evidenza (un codice
//     trovato, un nodo accettato da uno STEP non autorizzato) o portato li' da un arco nato da un'evidenza: e'
//     un candidato con il motivo, mai preselezionabile, finche' una persona non lo decide. Lo stato passa
//     lungo il cammino: i nodi dello STEP di un pezzo da rivedere (o raggiunto solo attraverso uno) si
//     mostrano, ma non sostengono una destinazione;
//   - scollegata: un nodo di uno STEP la cui radice non si raggiunge da nessun prodotto ancorato. Non da'
//     destinazioni.
//
// Lo stesso pezzo e' una voce sola, con tutte le sue posizioni: l'identita' della RFQ e' (thread, codice), e
// la destinazione e' il pezzo, non il percorso. Tutto qui e' puro.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// L'autorita' di una voce dell'indice (A5.13.4).
const (
	AutoritaDecisa     = "decisa"
	AutoritaDiretta    = "diretta"
	AutoritaProfonda   = "profonda"
	AutoritaDaRivedere = "da_rivedere"
	AutoritaScollegata = "scollegata"
)

var rangoAutorita = map[string]int{AutoritaDecisa: 5, AutoritaDiretta: 4, AutoritaProfonda: 3, AutoritaDaRivedere: 2, AutoritaScollegata: 1}

var rangoLivello = map[string]int{LivelloAutorizzata: 3, LivelloPiena: 2, LivelloDaConfermare: 1}

// Da dove viene una voce.
const (
	FonteVoceProdotto   = "prodotto"
	FonteVoceComponente = "componente"
	FonteVoceNodo       = "nodo_step"
)

// Voce e' un codice dell'indice: il pezzo, con tutte le sue posizioni.
type Voce struct {
	Codice   string   `json:"codice"`
	Fonti    []string `json:"fonti"`
	Autorita string   `json:"autorita"`
	// Livello: il livello dell'ancora migliore da cui la voce si raggiunge (autorizzata, piena, da confermare).
	Livello     string   `json:"livello,omitempty"`
	Concorrenti bool     `json:"concorrenti,omitempty"` // raggiunta solo da ancore concorrenti
	Prodotti    []string `json:"prodotti,omitempty"`
	// ProdottiWorking: i prodotti sotto cui il componente sta nella working (decisiSotto); ProdottiDecisi:
	// quelli da cui lo si raggiunge per un cammino che una persona ha deciso tutto. Prodotti dice anche i
	// prodotti dei cammini negli STEP, che non fanno del pezzo un componente di quel prodotto.
	ProdottiWorking []string      `json:"prodotti_working,omitempty"`
	ProdottiDecisi  []string      `json:"prodotti_decisi,omitempty"`
	Posizioni       []Posizione   `json:"posizioni,omitempty"`
	ComponenteID    uuid.NullUUID `json:"componente_id"`
	// Identificativo: un prodotto confermato che non ha ancora il suo componente («Crea il prodotto»).
	Identificativo bool      `json:"identificativo,omitempty"`
	Nodi           []RifNodo `json:"nodi,omitempty"`
	Discordanze    []string  `json:"discordanze,omitempty"`
	// DaRivedere: perche' il pezzo sta sotto un prodotto senza una decisione (il suo componente, o un arco del
	// cammino, nati da un'evidenza). Vuota per una voce decisa. Un candidato verso di lei non si preseleziona.
	DaRivedere []string `json:"da_rivedere,omitempty"`
	// Commerciale: il pezzo e' un commerciale; il suo STEP e i suoi discendenti sono solo guida.
	Commerciale bool `json:"commerciale,omitempty"`
	// SottoCommerciale: il pezzo c'e' solo come guida sotto questo commerciale; anche il suo STEP e' solo guida.
	SottoCommerciale string `json:"sotto_commerciale,omitempty"`
}

// Prodotto dice se la voce e' un prodotto della RFQ.
func (v *Voce) Prodotto() bool { return contiene(v.Fonti, FonteVoceProdotto) }

// Posizione e' dove il pezzo sta: sotto quale padre, quante volte, secondo quale file.
type Posizione struct {
	Padre  string `json:"padre"`
	Qta    int    `json:"qta"`
	File   string `json:"file,omitempty"`
	Chiave string `json:"chiave,omitempty"`
}

// RifNodo e' un nodo di uno STEP che porta il codice della voce.
type RifNodo struct {
	PropostaID   uuid.UUID     `json:"proposta_id"`
	AllegatoID   uuid.UUID     `json:"allegato_id"`
	File         string        `json:"file"`
	Chiave       string        `json:"chiave"`
	Autorita     string        `json:"autorita"`
	NellAutorita bool          `json:"nell_autorita,omitempty"` // figlio diretto di una sorgente autorizzata valida
	Accettato    uuid.NullUUID `json:"accettato"`               // il componente, se una persona l'ha accettato
	Ancora       string        `json:"ancora"`                  // il pezzo la cui ancora porta il nodo
	Livello      string        `json:"livello"`                 // il livello di quell'ancora (il piu' debole del cammino)
	Concorrenti  bool          `json:"concorrenti,omitempty"`   // quell'ancora ha due file concorrenti
	// TramiteDaRivedere: il pezzo da rivedere attraverso cui soltanto l'ancora si raggiunge dal prodotto (il
	// pezzo dell'ancora stesso, o uno sopra di lui): nessuna persona ha deciso quell'anello del cammino.
	TramiteDaRivedere string `json:"tramite_da_rivedere,omitempty"`
	Padre             string `json:"padre"`
	Qta               int    `json:"qta"`
	// SottoCommerciale: il commerciale di cui il nodo e' un discendente (o nel cui STEP sta): solo guida.
	SottoCommerciale string `json:"sotto_commerciale,omitempty"`
	// Alternativa: il nodo sta in uno STEP che e' un'alternativa al file in uso per il pezzo AlternativaDi.
	Alternativa   string `json:"alternativa,omitempty"`
	AlternativaDi string `json:"alternativa_di,omitempty"`
}

// guida dice se il nodo e' solo guida per una ragione che non e' la profondita': il discendente di un
// commerciale, o il nodo di uno STEP alternativo. Non sostiene una destinazione (scelta 2).
func (n RifNodo) guida() bool { return n.SottoCommerciale != "" || n.Alternativa != "" }

// affidabile dice se il nodo sta in uno STEP che ancora un pezzo con un'ancora affidabile (autorizzata o piena,
// senza un file concorrente e senza un pezzo da rivedere, per tutto il cammino dal prodotto), fuori dalla sola
// guida: solo allora e' un'evidenza strutturale che sostiene una destinazione (scelta 2). Il nodo di un'ancora
// da confermare o concorrente, o raggiunta solo attraverso un pezzo che nessuno ha deciso, si mostra, ma non
// sostiene niente (U7; «bis»: un'uguaglianza di codice non diventa una struttura).
func (n RifNodo) affidabile() bool {
	return n.Ancora != "" && !n.guida() && !n.Concorrenti && n.TramiteDaRivedere == "" &&
		(n.Livello == LivelloAutorizzata || n.Livello == LivelloPiena)
}

// Indice e' l'indice dei codici della RFQ (A5.13.4).
type Indice struct {
	Voci     map[string]*Voce
	Prodotti []Prodotto
	Ancore   map[string]Ancora // dei prodotti
	// Sottoassiemi: l'ancora di ogni voce che ha uno STEP suo (raggiungibilita').
	Sottoassiemi map[string]Ancora
	// UsatoDa: sha di uno STEP → il pezzo di cui e' l'ancora. Scollegati: gli STEP che non si raggiungono.
	// Alternative: sha di uno STEP non usato che pure ancorerebbe un pezzo ancorato da un altro file → quel
	// pezzo (le alternative mostrate, A5.13.3).
	UsatoDa     map[string]string
	Scollegati  map[string]bool
	Alternative map[string]string
	Firma       string
}

// IndiceCodici costruisce l'indice (A5.13.4): i prodotti ancorati e i componenti decisi raggiungibili da loro
// (decisa); i nodi degli STEP che li ancorano (diretta, profonda); per ogni voce, lo STEP della RFQ non ancora
// usato la cui radice e' quella voce (A-S1, A-S2, A-S3), o quello autorizzato per il suo componente, che ne
// propone i figli diretti: e' la raggiungibilita' dei sottoassiemi, un punto fisso sugli sha visitati (lo STEP
// di un commerciale si cammina come sola guida, e anche quello di un discendente di un commerciale, che non e'
// una sorgente ma si raggiunge: Domanda 5 = B); poi gli STEP non usati che pure ancorerebbero un pezzo
// ancorato da un altro file (le alternative: guida, non autorita'); infine i nodi degli STEP che non si
// raggiungono (scollegata). Due STEP che dicono diversamente i figli dello stesso pezzo portano
// struttura_diversa. Pura.
func IndiceCodici(prodotti []Prodotto, ancore map[string]Ancora, s *StatoFlusso) Indice {
	d := s.derivati()
	ix := Indice{Voci: map[string]*Voce{}, Prodotti: prodotti, Ancore: ancore, Sottoassiemi: map[string]Ancora{},
		UsatoDa: map[string]string{}, Scollegati: map[string]bool{}, Alternative: map[string]string{}}
	visite := map[string]visita{} // le voci di cui si e' gia' cercato lo STEP, con i valori di allora
	usati := map[string]bool{}

	// i prodotti ancorati e i componenti decisi sotto di loro
	for _, p := range prodotti {
		a := ancore[p.Codice]
		if !a.Ancorato() {
			continue
		}
		v := ix.voce(p.Codice)
		v.fonte(FonteVoceProdotto)
		v.alza(AutoritaDecisa)
		v.prodotto(p.Codice)
		v.livello(a.Livello, a.concorrenti())
		if p.ComponenteID.Valid {
			v.ComponenteID = p.ComponenteID
			d.decisiSotto(&ix, p.ComponenteID.UUID, p.Codice, a)
		} else {
			v.Identificativo = true
		}
		visite[p.Codice] = visita{prodotto: true}
	}
	// i file che ancorano i prodotti
	for _, p := range prodotti {
		a := ancore[p.Codice]
		for _, x := range portatoriUsati(a) {
			if st := d.strutture[x.Sha]; st != nil && !usati[x.Sha] {
				usati[x.Sha], ix.UsatoDa[x.Sha] = true, p.Codice
				d.cammina(&ix, st, x.Sorgenti, p.Codice, []string{p.Codice}, valoreCammino{livello: a.Livello, concorrenti: a.concorrenti()}, "")
			}
		}
	}
	// i sottoassiemi: ogni voce che ha uno STEP suo ne riceve i figli diretti, fino al punto fisso
	d.raggiungi(&ix, visite, usati)
	d.strutturaDiversa(&ix)
	for _, sha := range d.shaStep {
		if usati[sha] {
			continue
		}
		st := d.strutture[sha]
		f, _ := d.primoFile(sha)
		// un'alternativa al file che ancora un pezzo (un secondo STEP con la radice del prodotto autorizzato, una
		// radice vicina accanto a un'ancora piena): i suoi nodi si vedono come guida, con il file in uso, e non
		// sono «scollegati» (la radice si raggiunge)
		if pezzo, uso, ok := d.alternativaDi(&ix, st); ok {
			ix.Alternative[sha] = pezzo
			for _, k := range st.chiavi() {
				n := st.Nodi[k]
				c := d.codiceNodo(n)
				if c == "" || c == pezzo {
					continue
				}
				v := ix.voce(c)
				v.fonte(FonteVoceNodo)
				v.alza(AutoritaProfonda)
				padre := pezzo
				if pp := st.Padri[k]; len(pp) > 0 {
					if pc := d.codiceNodo(st.Nodi[pp[0]]); pc != "" {
						padre = pc
					}
				}
				v.Nodi = append(v.Nodi, RifNodo{PropostaID: n.PropostaID, AllegatoID: f.AllegatoID, File: f.Nome, Chiave: k,
					Autorita: AutoritaProfonda, Accettato: nullo(DecisoDaUnaPersona(n)), Padre: padre, Alternativa: uso, AlternativaDi: pezzo})
			}
			continue
		}
		// gli STEP che non si raggiungono da nessun prodotto ancorato: i loro nodi sono scollegati
		ix.Scollegati[sha] = true
		for _, k := range st.chiavi() {
			n := st.Nodi[k]
			c := d.codiceNodo(n)
			if c == "" {
				continue
			}
			v := ix.voce(c)
			v.fonte(FonteVoceNodo)
			v.alza(AutoritaScollegata)
			v.Nodi = append(v.Nodi, RifNodo{PropostaID: n.PropostaID, AllegatoID: f.AllegatoID, File: f.Nome, Chiave: k,
				Autorita: AutoritaScollegata, Accettato: nullo(DecisoDaUnaPersona(n))})
		}
	}
	ix.chiudi(d)
	return ix
}

// visita e' come il punto fisso ha visitato una voce: i valori con cui ne ha cercato lo STEP e camminato i
// nodi. Una voce si rivisita quando cambiano. prodotto: un prodotto ancorato, che non si visita (il suo file
// l'ha gia' camminato l'ancora del prodotto).
type visita struct {
	prodotto    bool
	guida       bool   // la voce c'e' solo come guida di un commerciale
	vicine      bool   // lo STEP si cerca anche fra le radici vicine (A-S3)
	livello     string // Voce.Livello
	concorrenti bool   // Voce.Concorrenti
	tramite     string // tramiteDaRivedere
	sopra       string // il commerciale sopra una voce di sola guida
	componente  bool
	prodotti    int
}

// valoreCammino sono i valori con cui si camminano i nodi di un'ancora: il livello dell'anello piu' debole, se
// un anello ha due file concorrenti, e il pezzo da rivedere attraverso cui soltanto l'ancora si raggiunge.
type valoreCammino struct {
	livello     string
	concorrenti bool
	tramite     string
}

// raggiungi e' il punto fisso della raggiungibilita' dei sottoassiemi: ogni voce che ha uno STEP suo ne
// riceve i figli. Tre fasi, e si torna alla prima appena una visita cambia qualcosa:
//   - 0: le voci fuori dalla guida, con lo STEP la cui radice e' la voce (A-S1, A-S2) o quello autorizzato per
//     il suo componente;
//   - 1: quando queste sono ferme, anche con una radice vicina (A-S3), che pero' non prende lo STEP la cui
//     radice e' esattamente un altro pezzo della RFQ, una voce dell'indice o un nodo di un altro STEP, anche
//     non ancora camminato: quello e' lo STEP di quel pezzo (radiceDiUnAltraVoce);
//   - 2: quando anche queste sono ferme, le voci che ci sono solo come guida di un commerciale. I discendenti
//     di un commerciale non entrano nella working da soli (ter): il loro STEP non e' una sorgente, ma si
//     raggiunge, e si cammina tutto come guida con il commerciale sopra; non e' «scollegato» (Domanda 5 = B).
//
// Una voce gia' visitata si rivisita quando cambiano i valori con cui la si raggiunge (il livello, un file
// concorrente, un pezzo da rivedere, la guida, i prodotti, il componente): i nodi del suo STEP si camminano di
// nuovo con i valori nuovi. Cosi' il risultato non dipende dall'ordine delle visite, cioe' dai nomi dei
// codici (FP7), salvo due scelte a pari merito fatte in ordine: il commerciale sopra una voce di sola guida
// sotto piu' commerciali (commercialeSopra), e lo STEP con una radice vicina a due voci (lo prende la prima).
// Il cammino e' affidabile quanto il suo anello piu' debole (U7).
func (d *derivati) raggiungi(ix *Indice, visite map[string]visita, usati map[string]bool) {
	// ogni rivisita migliora i valori di una voce, che sono finiti: il limite e' solo una difesa
	for giro, fase := 0, 0; giro < 8*len(ix.Voci)+64; giro++ {
		if d.visitaLeVoci(ix, visite, usati, fase) {
			fase = 0
			continue
		}
		if fase == 2 {
			return
		}
		fase++
	}
}

// visitaLeVoci visita, in ordine, le voci della fase che non sono ancora state visitate con i valori di adesso.
// Dice se ne ha visitata qualcuna.
func (d *derivati) visitaLeVoci(ix *Indice, visite map[string]visita, usati map[string]bool, fase int) bool {
	visitato := false
	for _, k := range ix.codici() {
		v := ix.Voci[k]
		prima, gia := visite[k]
		guida := soloGuidaCommerciale(v)
		if prima.prodotto || v.Autorita == AutoritaScollegata || guida != (fase == 2) {
			continue
		}
		ora := visita{guida: guida, vicine: prima.vicine || fase >= 1, livello: v.Livello, concorrenti: v.Concorrenti,
			tramite: tramiteDaRivedere(v), componente: v.ComponenteID.Valid, prodotti: len(v.Prodotti)}
		if guida {
			ora.sopra = commercialeSopra(v)
		}
		if gia && ora == prima {
			continue
		}
		visite[k], visitato = ora, true
		d.visitaVoce(ix, k, v, usati, ora)
	}
	return visitato
}

// visitaVoce cerca lo STEP della voce k e ne cammina i nodi con i valori x. Rivisitata, la voce tiene i file
// della sua ancora (lo STEP che ha preso resta suo) e i nodi che ne aveva camminato si tolgono, per camminarli
// di nuovo; cambia solo la natura dell'ancora: uscita dalla guida, il suo STEP non e' piu' solo guida. Lo STEP
// di un commerciale, e quello di una voce di sola guida sotto un commerciale, non sono una sorgente
// strutturale (Domanda 5 = B): l'ancora e' SoloGuida, e i nodi si camminano come guida con il commerciale sopra.
func (d *derivati) visitaVoce(ix *Indice, k string, v *Voce, usati map[string]bool, x visita) {
	a, ok := ix.Sottoassiemi[k]
	if ok {
		ix.togliNodiDi(k)
		a.SoloGuida, a.Motivi = false, nil
	} else if a, ok = d.ancoraDelSottoassieme(ix, v, usati, x.vicine); !ok {
		return
	}
	sopra := ""
	switch {
	case d.commerciale(v):
		sopra, a.SoloGuida = k, true
		a.Motivi = []string{fmt.Sprintf("%s è un commerciale: il suo STEP è solo guida; per usarlo come struttura cambia prima il tipo", k)}
	case x.guida:
		sopra, a.SoloGuida = x.sopra, true
		a.Motivi = []string{fmt.Sprintf("%s è sotto %s, un commerciale: il suo STEP è solo guida; per usarlo come struttura cambia prima il tipo di %s", k, x.sopra, x.sopra)}
	}
	ix.Sottoassiemi[k] = a
	livello, concorrenti := livelloDelCammino(v, a)
	val := valoreCammino{livello: livello, concorrenti: concorrenti, tramite: x.tramite}
	for _, p := range portatoriUsati(a) {
		usati[p.Sha], ix.UsatoDa[p.Sha] = true, k
		d.cammina(ix, d.strutture[p.Sha], p.Sorgenti, k, append([]string(nil), v.Prodotti...), val, sopra)
	}
}

// togliNodiDi toglie dalle voci i nodi camminati dall'ancora del pezzo k, per camminarli di nuovo. Quello che
// i nodi avevano gia' alzato (l'autorita', il livello, le posizioni) resta: una rivisita cammina con valori
// migliori, e li alza di nuovo almeno altrettanto.
func (ix *Indice) togliNodiDi(k string) {
	for _, v := range ix.Voci {
		tieni := v.Nodi[:0]
		for _, n := range v.Nodi {
			if n.Ancora != k {
				tieni = append(tieni, n)
			}
		}
		v.Nodi = tieni
	}
}

// tramiteDaRivedere e' il pezzo da rivedere attraverso cui soltanto la voce si raggiunge dal prodotto: vuoto se
// una persona ha deciso tutto un cammino fino a lei (decisa) o se e' un nodo affidabile di uno STEP; la voce
// stessa se e' da rivedere; altrimenti quello dei suoi nodi (il primo in ordine). I nodi dello STEP di un
// sottoassieme raggiunto solo attraverso un pezzo che nessuno ha deciso non sostengono una destinazione: sui
// dati di prima quel pezzo puo' essere un'identita' nata da un'uguaglianza di codice («bis»).
func tramiteDaRivedere(v *Voce) string {
	if v.Autorita == AutoritaDecisa {
		return ""
	}
	for _, n := range v.Nodi {
		if n.affidabile() {
			return ""
		}
	}
	if len(v.DaRivedere) > 0 {
		return v.Codice
	}
	out := ""
	for _, n := range v.Nodi {
		if t := n.TramiteDaRivedere; t != "" && !n.guida() && (out == "" || t < out) {
			out = t
		}
	}
	return out
}

// livelloDelCammino e' il livello con cui si camminano i nodi dell'ancora a del sottoassieme v: il minore fra
// quello con cui la voce si raggiunge e quello dell'ancora; concorrenti se l'uno o l'altra lo sono. Un
// sottoassieme raggiunto solo da un'ancora da confermare non ha figli piu' affidabili di lui.
func livelloDelCammino(v *Voce, a Ancora) (string, bool) {
	l := a.Livello
	if v.Livello != "" && rangoLivello[v.Livello] < rangoLivello[l] {
		l = v.Livello
	}
	return l, v.Concorrenti || a.concorrenti()
}

// commercialeSopra e' il commerciale sotto cui la voce sta come guida (il primo in ordine, se sono piu').
func commercialeSopra(v *Voce) string {
	out := ""
	for _, n := range v.Nodi {
		if n.SottoCommerciale != "" && (out == "" || n.SottoCommerciale < out) {
			out = n.SottoCommerciale
		}
	}
	return out
}

// portatoriUsati sono i file di un'ancora che l'indice usa: l'autorizzato da solo; tutti i pieni (con due,
// ancore_concorrenti); tutti i da confermare.
func portatoriUsati(a Ancora) []Portatore {
	switch a.Livello {
	case LivelloAutorizzata:
		if len(a.Portatori) > 0 {
			return a.Portatori[:1]
		}
	case LivelloPiena, LivelloDaConfermare:
		return conLivello(a.Portatori, a.Livello)
	}
	return nil
}

func (a Ancora) concorrenti() bool { return contiene(a.Discordanze, DiscAncoreConcorrenti) }

// decisiSotto aggiunge i componenti attivi raggiungibili dal componente del prodotto attraverso gli archi
// della working. «decisa» solo per un cammino che una persona ha deciso tutto: ogni componente e ogni arco,
// con lo stesso criterio della Struttura BOM (Provenienza e ArcoDaRivedereDi, riapertura.go: F7). Un componente
// nato da un'evidenza (un codice trovato, un nodo accettato da uno STEP non autorizzato) o un arco accettato da
// uno STEP non autorizzato entrano come evidenza, da_rivedere con i motivi: il pezzo c'e', la decisione no.
// Prima i cammini decisi, poi tutto il raggiungibile: i motivi dipendono solo dagli archi, non dall'ordine.
func (d *derivati) decisiSotto(ix *Indice, radice uuid.UUID, prodotto string, a Ancora) {
	figli := map[uuid.UUID][]db.ComponenteRelazione{}
	for _, r := range d.s.Relazioni {
		figli[r.PadreID] = append(figli[r.PadreID], r)
	}
	attivo := func(id uuid.UUID) (db.Componente, bool) {
		c, ok := d.compPerID[id]
		return c, ok && c.ArchiviatoIl == nil
	}
	deciso := map[uuid.UUID]bool{radice: true}
	coda := []uuid.UUID{radice}
	for len(coda) > 0 {
		padre := coda[0]
		coda = coda[1:]
		for _, r := range figli[padre] {
			if c, ok := attivo(r.FiglioID); ok && !deciso[c.ComponenteID] && d.arcoDeciso(r) && len(d.daRivedere(c)) == 0 {
				deciso[c.ComponenteID] = true
				coda = append(coda, c.ComponenteID)
			}
		}
	}
	visti := map[uuid.UUID]bool{radice: true}
	coda = []uuid.UUID{radice}
	for len(coda) > 0 {
		padre := coda[0]
		coda = coda[1:]
		pc, _ := d.compPerID[padre]
		for _, r := range figli[padre] {
			c, ok := attivo(r.FiglioID)
			if !ok {
				continue
			}
			v := ix.voce(d.canonico(c.Codice))
			v.fonte(FonteVoceComponente)
			v.ProdottiWorking = aggiungi(v.ProdottiWorking, prodotto)
			if deciso[c.ComponenteID] {
				v.alza(AutoritaDecisa)
				v.ProdottiDecisi = aggiungi(v.ProdottiDecisi, prodotto)
			} else {
				v.alza(AutoritaDaRivedere)
				if !deciso[padre] {
					v.rivedere(fmt.Sprintf("sta sotto %s, a sua volta da rivedere", pc.Codice))
				}
				if !d.arcoDeciso(r) {
					v.rivedere(fmt.Sprintf("l'arco %s → %s è %s", pc.Codice, c.Codice, ProvenienzaStepNonAutorizzato))
				}
				for _, e := range d.daRivedere(c) {
					v.rivedere(c.Codice + ": " + e)
				}
			}
			v.prodotto(prodotto)
			v.livello(a.Livello, a.concorrenti())
			v.ComponenteID = uuid.NullUUID{UUID: c.ComponenteID, Valid: true}
			v.posizione(Posizione{Padre: d.canonico(pc.Codice), Qta: int(r.Qta)})
			if !visti[c.ComponenteID] {
				visti[c.ComponenteID] = true
				coda = append(coda, c.ComponenteID)
			}
		}
	}
}

// cammina porta nell'indice i nodi di un file che ancora il pezzo: da ogni sorgente (la radice, o i nodi
// autorizzati), i figli diretti (diretta) e piu' in basso (profonda). Un nodo nell'autorita' di un file
// autorizzato (anche per una delega) e' diretto del suo componente a qualunque profondita' stia nel file.
// Sotto un commerciale tutto e' guida (Domanda 5 = B): i suoi discendenti sono profondi, con il commerciale;
// guida non vuota e' il commerciale di cui si cammina lo STEP stesso, che allora e' tutto guida. val sono i
// valori del cammino dal prodotto fino al pezzo (livelloDelCammino, tramiteDaRivedere).
func (d *derivati) cammina(ix *Indice, st *strutturaStep, sorgenti []string, pezzo string, prodotti []string, val valoreCammino, guida string) {
	f, _ := d.primoFile(st.Sha)
	type passo struct {
		chiave string
		prof   int
		padre  string
		guida  string // il commerciale sopra il nodo, se c'e'
	}
	visti := map[string]bool{}
	var coda []passo
	for _, s := range sorgenti {
		visti[s] = true
		coda = append(coda, passo{s, 0, pezzo, guida})
	}
	for len(coda) > 0 {
		x := coda[0]
		coda = coda[1:]
		for _, a := range st.Figli[x.chiave] {
			n, ok := st.Nodi[a.Figlio]
			if !ok {
				continue
			}
			codice := d.codiceNodo(n)
			autorita, nell := AutoritaProfonda, false
			padre := x.padre
			if c, ok := d.aut.FiglioDiretto(st.Portatore, a.Figlio); ok {
				autorita, nell = AutoritaDiretta, true
				if pc, ok := d.compPerID[c]; ok {
					padre = d.canonico(pc.Codice)
				}
			} else if x.prof == 0 && x.guida == "" {
				autorita = AutoritaDiretta
			}
			acc := nullo(DecisoDaUnaPersona(n))
			if codice != "" {
				v := ix.voce(codice)
				v.fonte(FonteVoceNodo)
				v.alza(autorita)
				for _, p := range prodotti {
					v.prodotto(p)
				}
				rif := RifNodo{PropostaID: n.PropostaID, AllegatoID: f.AllegatoID, File: f.Nome, Chiave: a.Figlio,
					Autorita: autorita, NellAutorita: nell, Accettato: acc, Ancora: pezzo, Livello: val.livello, Concorrenti: val.concorrenti,
					TramiteDaRivedere: val.tramite, Padre: padre, Qta: a.Qta}
				if x.guida != "" && !nell {
					rif.SottoCommerciale, rif.Livello = x.guida, ""
				} else {
					v.livello(val.livello, val.concorrenti)
					v.posizione(Posizione{Padre: padre, Qta: a.Qta, File: f.Nome, Chiave: a.Figlio})
				}
				if acc.Valid && !v.ComponenteID.Valid {
					if c, ok := d.compPerID[acc.UUID]; ok && c.ArchiviatoIl == nil {
						v.ComponenteID = acc
						// il nodo l'ha accettato una persona, ma il componente puo' essere nato da un'evidenza
						// (accettato da uno STEP non autorizzato): per la Struttura BOM e' da rivedere, e qui pure
						for _, e := range d.daRivedere(c) {
							v.rivedere(c.Codice + ": " + e)
						}
					}
				}
				v.Nodi = append(v.Nodi, rif)
			}
			if !visti[a.Figlio] {
				visti[a.Figlio] = true
				prossimo := codice
				if prossimo == "" {
					prossimo = strings.TrimSpace(n.NomeGrezzo)
				}
				sotto := x.guida
				if sotto == "" && d.nodoCommerciale(codice, acc) {
					sotto = codice
				}
				coda = append(coda, passo{a.Figlio, x.prof + 1, prossimo, sotto})
			}
		}
	}
}

// soloGuidaCommerciale: la voce c'e' solo come discendente di un commerciale (nessun componente, nessun'altra
// posizione).
func soloGuidaCommerciale(v *Voce) bool {
	if v.ComponenteID.Valid || len(v.Nodi) == 0 {
		return false
	}
	for _, n := range v.Nodi {
		if n.SottoCommerciale == "" {
			return false
		}
	}
	return true
}

// nodoCommerciale: il nodo e' un commerciale (il componente che una persona gli ha dato, o quello con il suo
// codice).
func (d *derivati) nodoCommerciale(codice string, acc uuid.NullUUID) bool {
	if acc.Valid {
		if c, ok := d.compPerID[acc.UUID]; ok {
			return c.ArchiviatoIl == nil && c.Tipo == db.TipoComponenteCommerciale
		}
	}
	if codice == "" {
		return false
	}
	c, ok := d.componentePerCodice(codice)
	return ok && c.ArchiviatoIl == nil && c.Tipo == db.TipoComponenteCommerciale
}

// commerciale: la voce e' un commerciale, con lo stesso criterio dei nodi (nodoCommerciale): il suo
// componente, o, se la voce non ne ha uno, il componente con il suo codice. Un criterio solo: lo stesso pezzo
// e' commerciale per i suoi discendenti (guida), per il suo STEP (solo guida) e per le sue destinazioni.
func (d *derivati) commerciale(v *Voce) bool {
	return d.nodoCommerciale(v.Codice, v.ComponenteID)
}

// alternativaDi dice se lo STEP non usato st ancorerebbe (A-S1…A-S5) un pezzo che un altro file gia' ancora:
// allora e' un'alternativa a quel file, e restituisce il pezzo e il file in uso.
func (d *derivati) alternativaDi(ix *Indice, st *strutturaStep) (string, string, bool) {
	prova := func(m map[string]Ancora) (string, string, bool) {
		for _, k := range chiaviOrdinate(m) {
			a := m[k]
			usati := portatoriUsati(a)
			if len(usati) == 0 {
				continue
			}
			if _, _, ok := d.ancoraDelFile(st, k); ok {
				return k, usati[0].Nome, true
			}
		}
		return "", "", false
	}
	if k, uso, ok := prova(ix.Ancore); ok {
		return k, uso, true
	}
	return prova(ix.Sottoassiemi)
}

// ancoraDelSottoassieme cerca lo STEP della voce v: quello autorizzato per il suo componente (non una delega,
// che vive nel file del padre e l'indice ha gia' camminato), o uno STEP della RFQ non ancora usato la cui
// radice e' la voce (A-S1, A-S2: piena; con vicine, A-S3: da confermare). Per un commerciale solo lo STEP con
// la sua radice: non e' una sorgente strutturale (Domanda 5 = B), e visitaVoce ne fa un'ancora SoloGuida.
func (d *derivati) ancoraDelSottoassieme(ix *Indice, v *Voce, usati map[string]bool, vicine bool) (Ancora, bool) {
	a := Ancora{Prodotto: v.Codice}
	if v.ComponenteID.Valid && !d.commerciale(v) {
		if x, ok := d.s.Dichiarazioni.Di(v.ComponenteID.UUID); ok && !x.Delega() && !usati[x.Sha256] && d.strutture[x.Sha256] != nil {
			f, _ := d.primoFile(x.Sha256)
			a.Livello = LivelloAutorizzata
			a.Portatori = []Portatore{{AllegatoID: f.AllegatoID, Sha: x.Sha256, Nome: f.Nome, Livello: LivelloAutorizzata,
				Regole: []string{"autorizzata"}, Sorgenti: x.ChiaviSorgenti()}}
			return a, true
		}
	}
	return d.ancoraPerContenuto(ix, v, usati, vicine)
}

// ancoraPerContenuto e' l'ancora della voce da uno STEP della RFQ non ancora usato la cui radice e' la voce
// (A-S1, A-S2: piena; con vicine, A-S3: da confermare). Una radice vicina che e' esattamente un altro pezzo
// della RFQ non e' un'ancora della voce: e' lo STEP di quell'altro pezzo («7120020A.stp» non e' di 7120020
// quando 7120020A e' un pezzo della RFQ). Vedi radiceDiUnAltraVoce.
func (d *derivati) ancoraPerContenuto(ix *Indice, v *Voce, usati map[string]bool, vicine bool) (Ancora, bool) {
	a := Ancora{Prodotto: v.Codice}
	candidati, _ := d.ancorePerContenuto(v.Codice, usati)
	var utili []Portatore
	for _, c := range candidati {
		switch {
		case contiene(c.Regole, "A-S1") || contiene(c.Regole, "A-S2"):
			utili = append(utili, c)
		case contiene(c.Regole, "A-S3") && vicine && !d.radiceDiUnAltraVoce(ix, c.Sha, v.Codice):
			utili = append(utili, c)
		}
	}
	if p := conLivello(utili, LivelloPiena); len(p) > 0 {
		a.Livello, a.Portatori = LivelloPiena, p
		if shaDiversi(p) > 1 {
			a.Discordanze = []string{DiscAncoreConcorrenti}
		}
		return a, true
	}
	if p := conLivello(utili, LivelloDaConfermare); len(p) > 0 {
		a.Livello, a.Portatori, a.Discordanze = LivelloDaConfermare, p, []string{DiscAncoraDaConfermare}
		return a, true
	}
	return a, false
}

// radiceDiUnAltraVoce: una radice dello STEP sha e' esattamente il codice di un pezzo della RFQ diverso da k:
// una voce dell'indice, o un nodo (non la radice) di un altro STEP della RFQ (radiceAltrove), anche se quello
// STEP non e' ancora stato camminato. Le voci che il punto fisso aggiunge vengono tutte dai nodi degli STEP:
// guardando tutti gli STEP, e non solo quelli gia' camminati, la risposta non dipende da quando la voce
// nasce, cioe' dall'ordine delle visite e dai nomi dei codici (FP7). Prima una radice vicina visitata presto
// prendeva lo STEP di un pezzo che un altro sottoassieme avrebbe raggiunto dopo, e lo teneva.
func (d *derivati) radiceDiUnAltraVoce(ix *Indice, sha, k string) bool {
	st := d.strutture[sha]
	if st == nil {
		return false
	}
	for _, r := range st.Radici {
		if c := d.codiceNodo(st.Nodi[r]); c != "" && c != k && (ix.Voci[c] != nil || d.radiceAltrove(st, c)) {
			return true
		}
	}
	return false
}

// strutturaDiversa: lo STEP di un sottoassieme e lo STEP del padre che elenca anche i figli del sottoassieme
// li dicono diversamente. Due STEP non comandano lo stesso pezzo d'albero: decide la persona che autorizza
// (A5.13.4). La discordanza va sul sottoassieme e sui figli coinvolti. Non c'e' fra due guide: lo STEP di un
// commerciale (o di un pezzo sotto un commerciale) e i figli che un altro STEP elenca sotto un commerciale
// non comandano niente, e non c'e' niente da autorizzare finche' una persona non cambia il tipo (Domanda 5 = B).
func (d *derivati) strutturaDiversa(ix *Indice) {
	for _, k := range ix.codici() {
		sa, ok := ix.Sottoassiemi[k]
		if !ok || sa.SoloGuida {
			continue
		}
		propri := map[string]bool{}
		shaPropri := map[string]bool{}
		for _, x := range portatoriUsati(sa) {
			shaPropri[x.Sha] = true
			st := d.strutture[x.Sha]
			for _, s := range x.Sorgenti {
				for _, a := range st.Figli[s] {
					if c := d.codiceNodo(st.Nodi[a.Figlio]); c != "" {
						propri[c] = true
					}
				}
			}
		}
		altrove := map[string]bool{}
		for sha := range ix.UsatoDa {
			if shaPropri[sha] {
				continue
			}
			st := d.strutture[sha]
			f, _ := d.primoFile(sha)
			for _, chiave := range st.chiavi() {
				if d.codiceNodo(st.Nodi[chiave]) != k || nodoDiGuida(ix.Voci[k], f.AllegatoID, chiave) {
					continue
				}
				for _, a := range st.Figli[chiave] {
					if c := d.codiceNodo(st.Nodi[a.Figlio]); c != "" {
						altrove[c] = true
					}
				}
			}
		}
		if len(propri) == 0 || len(altrove) == 0 || stessiInsiemi(propri, altrove) {
			continue
		}
		ix.Voci[k].discordanza(DiscStrutturaDiversa)
		for c := range propri {
			if v, ok := ix.Voci[c]; ok {
				v.discordanza(DiscStrutturaDiversa)
			}
		}
		for c := range altrove {
			if v, ok := ix.Voci[c]; ok {
				v.discordanza(DiscStrutturaDiversa)
			}
		}
	}
}

// nodoDiGuida dice se il nodo chiave del file allegato, che porta il codice della voce v, e' solo guida (sotto
// un commerciale): allora i figli che il file gli mette sotto sono guida anche loro.
func nodoDiGuida(v *Voce, allegato uuid.UUID, chiave string) bool {
	if v == nil {
		return false
	}
	for _, n := range v.Nodi {
		if n.AllegatoID == allegato && n.Chiave == chiave {
			return n.guida()
		}
	}
	return false
}

func stessiInsiemi(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// ------------------------------------------------------------------ le voci

func (ix *Indice) voce(codice string) *Voce {
	v, ok := ix.Voci[codice]
	if !ok {
		v = &Voce{Codice: codice}
		ix.Voci[codice] = v
	}
	return v
}

// codici sono i codici dell'indice in ordine: il punto fisso li visita sempre nello stesso ordine (FP7).
func (ix *Indice) codici() []string {
	out := make([]string, 0, len(ix.Voci))
	for k := range ix.Voci {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (v *Voce) fonte(f string) {
	if !contiene(v.Fonti, f) {
		v.Fonti = append(v.Fonti, f)
	}
}

func (v *Voce) alza(a string) {
	if rangoAutorita[a] > rangoAutorita[v.Autorita] {
		v.Autorita = a
	}
}

func (v *Voce) prodotto(p string) {
	if !contiene(v.Prodotti, p) {
		v.Prodotti = append(v.Prodotti, p)
	}
}

// livello tiene il livello migliore; una voce raggiunta anche da un'ancora non concorrente non e' concorrente.
func (v *Voce) livello(l string, concorrenti bool) {
	switch {
	case rangoLivello[l] > rangoLivello[v.Livello]:
		v.Livello, v.Concorrenti = l, concorrenti
	case rangoLivello[l] == rangoLivello[v.Livello] && !concorrenti:
		v.Concorrenti = false
	}
}

func (v *Voce) posizione(p Posizione) {
	for _, x := range v.Posizioni {
		if x == p {
			return
		}
	}
	v.Posizioni = append(v.Posizioni, p)
}

func (v *Voce) discordanza(k string) {
	if !contiene(v.Discordanze, k) {
		v.Discordanze = append(v.Discordanze, k)
	}
}

func (v *Voce) rivedere(motivo string) {
	if !contiene(v.DaRivedere, motivo) {
		v.DaRivedere = append(v.DaRivedere, motivo)
	}
}

// chiudi ordina tutto e calcola la firma dell'indice: lo sha256 del JSON canonico di prodotti, ancore, voci,
// regole del cliente e versione dell'algoritmo (A5.13.4). I motivi in parole non ci entrano: cambiano con le
// frasi, non con lo stato.
func (ix *Indice) chiudi(d *derivati) {
	for _, v := range ix.Voci {
		if v.Autorita == AutoritaDecisa {
			// raggiunta anche per un cammino deciso: i motivi di un altro cammino non la fanno da rivedere
			v.DaRivedere = nil
		}
		v.Commerciale = d.commerciale(v)
		if soloGuidaCommerciale(v) {
			v.SottoCommerciale = commercialeSopra(v)
		}
		sort.Strings(v.Fonti)
		sort.Strings(v.Prodotti)
		sort.Strings(v.ProdottiWorking)
		sort.Strings(v.ProdottiDecisi)
		sort.Strings(v.Discordanze)
		sort.Strings(v.DaRivedere)
		sort.Slice(v.Posizioni, func(i, j int) bool {
			a, b := v.Posizioni[i], v.Posizioni[j]
			if a.Padre != b.Padre {
				return a.Padre < b.Padre
			}
			if a.File != b.File {
				return a.File < b.File
			}
			return a.Chiave < b.Chiave
		})
		sort.Slice(v.Nodi, func(i, j int) bool {
			a, b := v.Nodi[i], v.Nodi[j]
			if rangoAutorita[a.Autorita] != rangoAutorita[b.Autorita] {
				return rangoAutorita[a.Autorita] > rangoAutorita[b.Autorita]
			}
			if a.NellAutorita != b.NellAutorita {
				return a.NellAutorita
			}
			if a.File != b.File {
				return a.File < b.File
			}
			return a.Chiave < b.Chiave
		})
	}
	type portatoreFirma struct {
		Sha, Livello        string
		Regole, Discordanze []string
	}
	type ancoraFirma struct {
		Pezzo, Livello string
		Portatori      []portatoreFirma
		Discordanze    []string
		SoloGuida      bool
	}
	ancore := func(m map[string]Ancora) []ancoraFirma {
		var out []ancoraFirma
		for _, k := range chiaviOrdinate(m) {
			a := m[k]
			af := ancoraFirma{Pezzo: k, Livello: a.Livello, Discordanze: a.Discordanze, SoloGuida: a.SoloGuida}
			for _, p := range a.Portatori {
				af.Portatori = append(af.Portatori, portatoreFirma{p.Sha, p.Livello, p.Regole, p.Discordanze})
			}
			out = append(out, af)
		}
		return out
	}
	type voceFirma struct {
		Voce
		DaRivedere bool `json:"da_rivedere"` // lo stato, non le parole dei motivi
	}
	voci := make([]voceFirma, 0, len(ix.Voci))
	for _, k := range ix.codici() {
		v := voceFirma{Voce: *ix.Voci[k], DaRivedere: len(ix.Voci[k].DaRivedere) > 0}
		v.Voce.DaRivedere = nil
		voci = append(voci, v)
	}
	scollegati := chiaviOrdinate(ix.Scollegati)
	raw, _ := json.Marshal(struct {
		Algoritmo, Regole string
		Prodotti          []Prodotto
		Ancore, Sotto     []ancoraFirma
		Voci              []voceFirma
		Scollegati        []string
		Alternative       map[string]string
	}{AlgoritmoFlusso, d.s.ImprontaRegole, ix.Prodotti, ancore(ix.Ancore), ancore(ix.Sottoassiemi), voci, scollegati, ix.Alternative})
	h := sha256.Sum256(raw)
	ix.Firma = hex.EncodeToString(h[:])
}

func chiaviOrdinate[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func nullo(id uuid.UUID, ok bool) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: ok} }

// ------------------------------------------------------------------ lo stato, derivato una volta

// derivati e' cio' che il flusso ricava dallo StatoFlusso e usa a ogni passo: il Motore, le strutture degli
// STEP per contenuto, i file per contenuto, i componenti per codice, l'autorita' (A5.4).
type derivati struct {
	s         *StatoFlusso
	m         *classificazione.Motore
	strutture map[string]*strutturaStep
	shaStep   []string                // gli STEP nel flusso con una struttura letta, nell'ordine di arrivo
	fileStep  map[string][]FileFlusso // sha → i file STEP nel flusso con quel contenuto
	perSha    map[string][]FileFlusso // sha → tutti i file con quel contenuto, nell'ordine di arrivo
	compPer   map[string]db.Componente
	compPerID map[uuid.UUID]db.Componente
	aut       Autorita
	// codiciDeiFile: i codici che i nomi dei file della RFQ dicono (la copertura di un'ancora).
	codiciDeiFile map[string]bool
	// archiDecisi: gli archi della working che una persona ha deciso nell'autorita' di un file autorizzato;
	// provenienze: le etichette «da rivedere» di ogni componente, calcolate alla prima domanda.
	archiDecisi map[CoppiaDiComponenti]bool
	provenienze map[uuid.UUID][]string
}

func (s *StatoFlusso) derivati() *derivati {
	if s.der != nil {
		return s.der
	}
	d := &derivati{s: s, m: s.Motore, strutture: map[string]*strutturaStep{}, fileStep: map[string][]FileFlusso{},
		perSha: map[string][]FileFlusso{}, compPer: map[string]db.Componente{}, compPerID: map[uuid.UUID]db.Componente{},
		codiciDeiFile: map[string]bool{}}
	for _, c := range s.Componenti {
		d.compPer[strings.ToUpper(strings.TrimSpace(c.Codice))] = c
		d.compPerID[c.ComponenteID] = c
	}
	file := append([]FileFlusso(nil), s.File...)
	sort.SliceStable(file, func(i, j int) bool {
		if !file[i].RicevutoIl.Equal(file[j].RicevutoIl) {
			return file[i].RicevutoIl.Before(file[j].RicevutoIl)
		}
		return file[i].AllegatoID.String() < file[j].AllegatoID.String()
	})
	for _, f := range file {
		if f.Sha != "" {
			d.perSha[f.Sha] = append(d.perSha[f.Sha], f)
		}
		if f.Sha != "" && f.step() && f.nelFlusso() {
			d.fileStep[f.Sha] = append(d.fileStep[f.Sha], f)
		}
		if f.nelFlusso() {
			if c, _ := codiciDelNome(f.Nome, s.Motore); c != "" {
				d.codiciDeiFile[c] = true
			}
		}
	}
	d.aut = CalcolaAutorita(s.Dichiarazioni, s.Nodi, s.Archi)
	d.archiDecisi = ArchiDecisiNellAutorita(d.aut, s.Nodi, s.Archi)
	for sha, st := range costruisciStrutture(s.Nodi, s.Archi) {
		if len(d.fileStep[sha]) == 0 {
			continue // le righe di un file escluso, o di una fonte che non e' del cliente, non contano
		}
		d.strutture[sha] = st
	}
	for _, f := range file {
		if st := d.strutture[f.Sha]; st != nil && !contiene(d.shaStep, f.Sha) {
			d.shaStep = append(d.shaStep, f.Sha)
		}
	}
	s.der = d
	return d
}

// arcoDeciso: l'arco della working l'ha deciso una persona. Lo stesso criterio della Struttura BOM (F7): non
// lo e' un arco accettato da uno STEP non autorizzato (ArcoDaRivedereDi).
func (d *derivati) arcoDeciso(r db.ComponenteRelazione) bool {
	return !ArcoDaRivedereDi(r, d.archiDecisi)
}

// daRivedere sono le etichette della provenienza di un componente nato da un'evidenza (Provenienza, A5.15.4:
// le stesse che la Struttura BOM mostra «da rivedere»); vuote per un componente deciso da una persona. Il
// flusso non conosce lo STEP strutturale di prima di un finito: per un finito sotto un altro prodotto la
// «radice legata a mano» e' detta anche quando quel file lo era (un avviso in piu', mai una preselezione).
func (d *derivati) daRivedere(c db.Componente) []string {
	if d.provenienze == nil {
		d.provenienze = map[uuid.UUID][]string{}
		entrante := map[NodoFile]bool{}
		for _, r := range d.s.Archi {
			entrante[NodoFile{r.AllegatoID, r.FiglioChiave}] = true
		}
		righe := map[uuid.UUID][]RigaDelComponente{}
		for _, n := range d.s.Nodi {
			if n.ComponenteID.Valid {
				righe[n.ComponenteID.UUID] = append(righe[n.ComponenteID.UUID],
					RigaDelComponente{Proposta: n, Radice: !entrante[NodoFile{n.AllegatoID, n.Chiave}]})
			}
		}
		ident := map[string]*db.IdentificativoThread{}
		for i := range d.s.Identificativi {
			ident[strings.ToUpper(strings.TrimSpace(d.s.Identificativi[i].Codice))] = &d.s.Identificativi[i]
		}
		for _, x := range d.s.Componenti {
			d.provenienze[x.ComponenteID] = Provenienza(ProvenienzaDi{Componente: x,
				Identificativo: ident[strings.ToUpper(strings.TrimSpace(x.Codice))], Righe: righe[x.ComponenteID]}, d.aut)
		}
	}
	return d.provenienze[c.ComponenteID]
}

// canonico e' il codice come lo confronta il flusso: tolto il suffisso decorativo del cliente, in maiuscolo.
func (d *derivati) canonico(c string) string {
	c = strings.TrimSpace(c)
	if c == "" {
		return ""
	}
	k, _ := d.m.Canonico(c, "")
	return strings.ToUpper(strings.TrimSpace(k))
}

func (d *derivati) componentePerCodice(c string) (db.Componente, bool) {
	x, ok := d.compPer[strings.ToUpper(strings.TrimSpace(c))]
	return x, ok
}

// primoFile e' il primo file arrivato con quel contenuto: quello che il flusso nomina come portatore.
func (d *derivati) primoFile(sha string) (FileFlusso, bool) {
	if ff := d.fileStep[sha]; len(ff) > 0 {
		return ff[0], true
	}
	if ff := d.perSha[sha]; len(ff) > 0 {
		return ff[0], true
	}
	return FileFlusso{}, false
}

// codiceNodo e' il codice di un nodo: quello della riga (classificato con le regole del cliente, o scritto
// dall'operatore), canonico.
func (d *derivati) codiceNodo(n db.ComponenteProposta) string {
	if !n.Codice.Valid {
		return ""
	}
	return d.canonico(n.Codice.String)
}

func (d *derivati) codiciRadici(st *strutturaStep) []string {
	var out []string
	for _, r := range st.Radici {
		c := d.codiceNodo(st.Nodi[r])
		if c == "" {
			c = strings.TrimSpace(st.Nodi[r].NomeGrezzo)
		}
		out = append(out, c)
	}
	return out
}

// strutturaStep e' la struttura di un contenuto STEP come la dicono le righe di proposta della RFQ: i nodi,
// gli archi e le radici (i nodi senza un arco entrante, K1). I nodi e gli archi scartati da una persona non ci
// sono: una persona ha detto che non fanno parte del pezzo.
type strutturaStep struct {
	Sha       string
	Portatore uuid.UUID
	Nodi      map[string]db.ComponenteProposta
	Figli     map[string][]arcoStep
	Padri     map[string][]string
	Radici    []string
}

type arcoStep struct {
	Figlio string
	Qta    int
}

func (st *strutturaStep) chiavi() []string { return chiaviOrdinate(st.Nodi) }

func scartatoDaUnaPersona(stato db.StatoProposta, da uuid.NullUUID) bool {
	return stato == db.StatoPropostaScartata && da.Valid
}

func costruisciStrutture(nodi []db.ComponenteProposta, archi []db.RelazioneProposta) map[string]*strutturaStep {
	out := map[string]*strutturaStep{}
	perAllegato := map[uuid.UUID]*strutturaStep{}
	for _, n := range nodi {
		if scartatoDaUnaPersona(n.Stato, n.DecisoDa) {
			continue
		}
		st, ok := out[n.Sha256]
		if !ok {
			st = &strutturaStep{Sha: n.Sha256, Portatore: n.AllegatoID, Nodi: map[string]db.ComponenteProposta{},
				Figli: map[string][]arcoStep{}, Padri: map[string][]string{}}
			out[n.Sha256] = st
		}
		st.Nodi[n.Chiave] = n
		perAllegato[n.AllegatoID] = st
	}
	ord := append([]db.RelazioneProposta(nil), archi...)
	sort.Slice(ord, func(i, j int) bool {
		if ord[i].PadreChiave != ord[j].PadreChiave {
			return ord[i].PadreChiave < ord[j].PadreChiave
		}
		return ord[i].FiglioChiave < ord[j].FiglioChiave
	})
	for _, r := range ord {
		st, ok := perAllegato[r.AllegatoID]
		if !ok || scartatoDaUnaPersona(r.Stato, r.DecisoDa) {
			continue
		}
		if _, ok := st.Nodi[r.PadreChiave]; !ok {
			continue
		}
		if _, ok := st.Nodi[r.FiglioChiave]; !ok {
			continue
		}
		st.Figli[r.PadreChiave] = append(st.Figli[r.PadreChiave], arcoStep{Figlio: r.FiglioChiave, Qta: int(r.Qta)})
		st.Padri[r.FiglioChiave] = append(st.Padri[r.FiglioChiave], r.PadreChiave)
	}
	for _, st := range out {
		for _, k := range st.chiavi() {
			if len(st.Padri[k]) == 0 {
				st.Radici = append(st.Radici, k)
			}
		}
	}
	return out
}
