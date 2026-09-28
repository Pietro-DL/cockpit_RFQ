package classificazione

import (
	"bytes"
	"encoding/json"
	"strings"
)

// LA FOTOGRAFIA DI UNA DECISIONE SULLA POSTA (Smistamento M3, A5.16.6, K15)
//
// Gli score dei candidati non sono percentuali (U7): ordinano, e diventeranno una frequenza vera solo
// quando si saprà quante volte il primo proposto era quello giusto. Per saperlo, a ogni decisione il
// motivo di `messaggio_aggancio_log` smette di essere la frase fissa «decisione dell'operatore» e
// diventa un JSON compatto e versionato: che cosa era in cima, in che posizione stava la RFQ scelta, su
// quante, a pari merito o no, con quale evento e con quale versione delle regole. La frase resta
// dentro, per chi legge il log.
//
// Solo alla decisione (P39), mai alla GET: registrare «mostrato» vorrebbe dire una GET che scrive (R8).
// Il pannello mostra sempre i candidati salvati, quindi rileggerli nella transazione della decisione dà
// lo stesso elenco che l'operatore aveva davanti. Nessuna colonna nuova: il motivo è già `text`, e le
// righe di prima restano frasi (le misure le saltano, `motivo LIKE '{%'`).

// VersioneMotivo è la versione del JSON: un campo in più o in meno cambia il numero.
const VersioneMotivo = 1

// VersioneRegoleAggancio è la versione della scala dei candidati con cui è stata presa la decisione: la
// gerarchia a livelli di M1 (A5.16.3) è la seconda, dopo R0–R5 a punteggio fisso del 3R. Una misura non
// mescola due scale: lo stesso «score 86» voleva dire un'altra cosa prima.
const VersioneRegoleAggancio = "aggancio-2"

// I gesti che lasciano una fotografia. `nuova_rfq` e `marcatore` non sono valori di
// `messaggio_aggancio_log.azione` (il CHECK della 0003 ammette aggancia, sgancia, ignora, propaga,
// candidato): la RFQ nuova si registra come `aggancia` verso la RFQ appena nata, come sempre, e il
// gesto nel JSON dice che cosa è successo davvero.
const (
	GestoAggancia  = "aggancia"  // «Aggancia a questa RFQ» su una card, o su una riga di «Altra RFQ…»
	GestoNuovaRFQ  = "nuova_rfq" // «Crea RFQ»: nessun candidato era quello giusto (rango 0)
	GestoIgnora    = "ignora"    // il messaggio non è di nessuna RFQ
	GestoCandidato = "candidato" // il giro degli orfani: una proposta, non una scelta di una persona
	// GestoMarcatore è l'aggancio scritto dal marcatore della richiesta ai fornitori fino a M1 (I5a). Da
	// M1 (D85) il marcatore non aggancia più e nessuno scrive questo gesto: la conferma di una persona è
	// un `aggancia` con `Marcatore` fra i tipi. Resta nel vocabolario perché le misure lo escludano se
	// mai ricomparisse, come il `candidato`: non c'è una scelta umana da misurare.
	GestoMarcatore = "marcatore"
)

// GestiMisurati sono i gesti di una persona davanti ai candidati: gli unici che dicono se il primo
// proposto era quello giusto. `candidato` e `marcatore` sono automatici e restano fuori (P39).
var GestiMisurati = []string{GestoAggancia, GestoNuovaRFQ, GestoIgnora}

// frasiGesto sono le frasi di sempre del log, dentro il JSON.
var frasiGesto = map[string]string{
	GestoAggancia: "decisione dell'operatore",
	GestoNuovaRFQ: "decisione dell'operatore",
	GestoIgnora:   "chiuso senza RFQ",
}

// SceltoRFQ è la RFQ scelta com'era nell'elenco: rango 1 = la prima card. Rango 0 = non era fra i
// candidati (una RFQ nuova, o una RFQ trovata con «Altra RFQ…»): allora livello «nessuno», score 0 e
// nessun tipo.
type SceltoRFQ struct {
	Rango   int      `json:"rango"`
	Livello string   `json:"livello"`
	Score   int      `json:"score"`
	Tipi    []string `json:"tipi"`
}

// PrimoRFQ è la prima card dell'elenco, quella che il sistema metteva in cima.
type PrimoRFQ struct {
	Thread  string   `json:"thread"`
	Livello string   `json:"livello"`
	Score   int      `json:"score"`
	Tipi    []string `json:"tipi"`
}

// Scelta è la fotografia di una decisione. L'ordine dei campi è quello del JSON e non cambia (249).
type Scelta struct {
	V     int    `json:"v"`
	Frase string `json:"frase"`
	Gesto string `json:"gesto"`
	// Scelto è nil per «ignora»: nessuna RFQ è stata scelta.
	Scelto *SceltoRFQ `json:"scelto"`
	// Primo è nil se non c'erano candidati.
	Primo *PrimoRFQ `json:"primo"`
	// Su è il numero delle RFQ candidate (le card), marcatore compreso.
	Su         int  `json:"su"`
	PariMerito bool `json:"pari_merito"`
	// FuoriLista: «Aggancia» verso una RFQ che non era fra i candidati («Altra RFQ…»).
	FuoriLista bool   `json:"fuori_lista"`
	Evento     string `json:"evento"`
	Atto       string `json:"atto"`
	Regole     string `json:"regole"`
}

// SceltaDa fotografa la decisione: i candidati come li mostra il pannello (RaggruppaEOrdina, con il
// marcatore), il gesto, la RFQ scelta (vuota per «ignora»), e l'evento e l'atto del messaggio. Il rango è
// la posizione della RFQ scelta nell'elenco; una RFQ nuova ha rango 0 anche se c'erano candidati: vuol
// dire che nessuno di loro era quello giusto.
func SceltaDa(candidati []CandidatoRFQ, gesto, scelto, evento, atto string) Scelta {
	s := Scelta{V: VersioneMotivo, Frase: frasiGesto[gesto], Gesto: gesto, Su: len(candidati),
		PariMerito: PariMerito(candidati), Evento: evento, Atto: atto, Regole: VersioneRegoleAggancio}
	if len(candidati) > 0 {
		p := candidati[0]
		s.Primo = &PrimoRFQ{Thread: p.ThreadID, Livello: p.Livello.Chiave(), Score: p.Score, Tipi: tipiDi(p)}
	}
	if gesto == GestoIgnora {
		return s
	}
	s.Scelto = &SceltoRFQ{Livello: LivelloNessuno.Chiave(), Tipi: []string{}}
	if gesto == GestoNuovaRFQ {
		return s
	}
	for _, g := range candidati {
		if g.ThreadID == scelto {
			s.Scelto = &SceltoRFQ{Rango: g.Rango, Livello: g.Livello.Chiave(), Score: g.Score, Tipi: tipiDi(g)}
			return s
		}
	}
	s.FuoriLista = true
	return s
}

// tipiDi sono i tipi distinti delle evidenze di una card, dal più forte: sono le evidenze che
// l'operatore aveva sotto gli occhi, e dicono per esempio se la card era un R3 da solo o un R3 con R2.
func tipiDi(g CandidatoRFQ) []string {
	out := []string{}
	visti := map[string]bool{}
	for _, e := range g.Evidenze {
		if t := e.TipoDi(); t != "" && !visti[t] {
			visti[t] = true
			out = append(out, t)
		}
	}
	return out
}

// Motivo è il JSON che va nel log: compatto, con i campi sempre nello stesso ordine, e con i caratteri
// della frase così come sono («·», «<»), perché chi legge il log in DBeaver lo legge senza decodificarlo.
func (s Scelta) Motivo() string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return s.Frase // non succede: la struttura è fatta solo di stringhe, numeri e booleani
	}
	return strings.TrimRight(b.String(), "\n")
}

// LeggiScelta rilegge il motivo di una riga del log. False per le righe di prima, che sono frasi, e per
// un JSON che non è una fotografia (senza la versione).
func LeggiScelta(motivo string) (Scelta, bool) {
	if !strings.HasPrefix(motivo, "{") {
		return Scelta{}, false
	}
	var s Scelta
	if err := json.Unmarshal([]byte(motivo), &s); err != nil || s.V == 0 {
		return Scelta{}, false
	}
	return s, true
}

// FasciaScore è la fascia di uno score della posta nelle misure. È la stessa del CASE di
// `queries/calibrazione.sql` (MisuraPosta): la query «retro», che si calcola qui, e quella della
// fotografia devono dare righe confrontabili.
func FasciaScore(score int) string {
	switch {
	case score >= 90:
		return "90-100"
	case score >= 70:
		return "70-89"
	case score >= 50:
		return "50-69"
	case score >= 30:
		return "30-49"
	}
	return "0-29"
}
