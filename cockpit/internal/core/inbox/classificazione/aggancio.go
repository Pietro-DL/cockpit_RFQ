package classificazione

import (
	"sort"
	"strings"
)

// Le regole di aggancio R0–R5 (fase 4.1, D9). Qui sta la parte PURA: i nomi, i punteggi, l'ordine di
// precedenza e la frase di evidenza. Le interrogazioni al database stanno in internal/core/inbox/aggancio.
//
// Perché una tabella di regole e non un punteggio unico. Un punteggio unico mescola affermazioni di
// natura diversa: «il client di posta dice che questo messaggio risponde a quello» e «questi due
// messaggi hanno lo stesso oggetto» non sono la stessa cosa detta con forza diversa, sono due cose
// diverse. Sommandole si ottiene un numero che nessuno sa più leggere, e soprattutto si ottiene che
// tre indizi deboli battono una prova. L'ordine qui sotto è una PRECEDENZA: la prima regola che parla
// decide di che tipo di messaggio si tratta, le altre aggiungono candidati da mostrare.
const (
	R0Reply         = "R0_reply"         // In-Reply-To / References verso un messaggio di una RFQ
	R1Conversazione = "R1_conversazione" // ConversationID (e ConversationIndex) di una conversazione di una RFQ
	R4Riferimento   = "R4_riferimento"   // riferimento della richiesta secondo il cliente (RDO, Anfrage, ODA)
	R3Codice        = "R3_codice"        // codice di famiglia già identificativo di una RFQ dello stesso cliente
	R2Oggetto       = "R2_oggetto"       // stesso oggetto, stesso cliente, dentro la finestra
	R5Buyer         = "R5_buyer"         // stesso buyer, di recente: da solo non basta mai
)

// RegolaMarcatore è il legame che il Cockpit stesso ha scritto sulla nostra mail: la bozza preparata per
// una RFQ (D84) o la richiesta a un fornitore (D85). Non è un valore dell'enum `regola_aggancio` e non
// finisce mai in `candidato_aggancio`: il candidato si costruisce in lettura, ogni volta, da `bozza` e da
// `richiesta_fornitore`. La mail non si aggancia da sola: la conferma è un «Aggancia» di una persona.
const RegolaMarcatore = "marcatore_cockpit"

// I TIPI DI EVIDENZA (Smistamento M1, A5.16.3, D83).
//
// Una regola non dice da sola quanto vale: «In-Reply-To punta a una mail della RFQ, e il mittente era fra
// i destinatari» e «In-Reply-To punta a una mail della RFQ, ma chi scrive è un terzo» sono la stessa
// regola R0 con due forze diverse. Il tipo è la regola con la sua variante; lo score è fisso per tipo e
// distingue la variante anche in una riga letta dal database, dove la colonna è una sola
// (`candidato_aggancio.punteggio`): (regola, punteggio) ricostruisce il tipo (LeggiRiga).
//
// Gli score NON sono percentuali e non si scrivono mai con «%» (U7, decisioni del 27/09): ordinano, e
// saranno calibrati sulle scelte vere degli operatori. Prima si guarda il livello, poi lo score dentro il
// livello: tre indizi deboli non battono una prova.
const (
	TipoR0InReplyTo     = "R0InReplyTo"     // In-Reply-To verso una mail della RFQ, verificato
	TipoR0Cockpit       = "R0Cockpit"       // In-Reply-To/References verso la nostra mail preparata dal Cockpit
	TipoMarcatore       = "Marcatore"       // il messaggio stesso è la nostra mail di una bozza o di una richiesta
	TipoR0References    = "R0References"    // References (non In-Reply-To) verso una mail della RFQ, verificato
	TipoR0NonVerificato = "R0NonVerificato" // chiave citata della RFQ, ma partecipanti diversi o un inoltro
	TipoR1Forte         = "R1Forte"         // stessa conversazione, indice che discende da una mail della RFQ, stesso cliente
	TipoR4Riferimento   = "R4Riferimento"   // riferimento del cliente uguale
	TipoR3CodiceBuyer   = "R3CodiceBuyer"   // codice di famiglia della RFQ, stesso buyer
	TipoR3Codice        = "R3Codice"        // codice di famiglia della RFQ, buyer diverso o non noto
	TipoR1Indice        = "R1Indice"        // indice che discende, cliente diverso o non noto
	TipoR1Solo          = "R1Solo"          // solo il ConversationID
	TipoR2Oggetto       = "R2Oggetto"       // solo l'oggetto uguale
	TipoR5Buyer         = "R5Buyer"         // stesso buyer, di recente
)

// Livello è la forza di un'evidenza a parole: è ciò che l'operatore legge («molto forte · score 98») e
// ciò che ordina per primo. Più alto = più forte.
type Livello int

const (
	LivelloNessuno Livello = iota
	MoltoDebole
	Debole
	Media
	MedioForte
	Forte
	MoltoForte
)

func (l Livello) String() string {
	switch l {
	case MoltoForte:
		return "molto forte"
	case Forte:
		return "forte"
	case MedioForte:
		return "medio-forte"
	case Media:
		return "media"
	case Debole:
		return "debole"
	case MoltoDebole:
		return "molto debole"
	}
	return "nessuno"
}

// Chiave è il livello senza spazi né trattini: per le classi della schermata e, domani, per il JSON della
// calibrazione (A5.16.6).
func (l Livello) Chiave() string {
	return strings.NewReplacer(" ", "_", "-", "_").Replace(l.String())
}

// TipoEvidenza è una riga della tabella di A5.16.3.
type TipoEvidenza struct {
	Nome    string
	Regola  string
	Score   int
	Livello Livello
}

// tipiEvidenza è la tabella, dal più forte al più debole. Nessuna coppia (regola, score) si ripete:
// è ciò che permette a LeggiRiga di tornare dalla riga al tipo.
var tipiEvidenza = []TipoEvidenza{
	{TipoR0InReplyTo, R0Reply, 98, MoltoForte},
	{TipoR0Cockpit, R0Reply, 97, MoltoForte},
	{TipoMarcatore, RegolaMarcatore, 97, MoltoForte},
	{TipoR0References, R0Reply, 96, MoltoForte},
	{TipoR0NonVerificato, R0Reply, 88, Forte},
	{TipoR1Forte, R1Conversazione, 86, Forte},
	{TipoR4Riferimento, R4Riferimento, 84, Forte},
	{TipoR3CodiceBuyer, R3Codice, 72, MedioForte},
	{TipoR3Codice, R3Codice, 62, Media},
	{TipoR1Indice, R1Conversazione, 45, Debole},
	{TipoR1Solo, R1Conversazione, 40, Debole},
	{TipoR2Oggetto, R2Oggetto, 20, MoltoDebole},
	{TipoR5Buyer, R5Buyer, 15, MoltoDebole},
}

// TipiEvidenza restituisce la tabella dei tipi, dal più forte al più debole.
func TipiEvidenza() []TipoEvidenza { return append([]TipoEvidenza{}, tipiEvidenza...) }

// Tipo restituisce la riga della tabella di quel tipo.
func Tipo(nome string) (TipoEvidenza, bool) {
	for _, t := range tipiEvidenza {
		if t.Nome == nome {
			return t, true
		}
	}
	return TipoEvidenza{}, false
}

// PuntiRegola è lo score con cui ogni regola si presenta quando non si sa di più: quello della sua
// variante più forte. Non è una probabilità (D9): è un ordine leggibile da una persona che deve decidere
// in due secondi quale candidato guardare per primo. La variante vera la dice il tipo.
var PuntiRegola = map[string]int{
	R0Reply:         98,
	R1Conversazione: 86,
	R4Riferimento:   84,
	R3Codice:        72,
	R2Oggetto:       20,
	R5Buyer:         15,
}

// PunteggiDellaRegola sono gli score delle varianti ATTUALI di una regola: una riga del database con un
// altro punteggio è stata scritta con le regole di prima (A5.10).
func PunteggiDellaRegola(regola string) []int16 {
	var out []int16
	for _, t := range tipiEvidenza {
		if t.Regola == regola {
			out = append(out, int16(t.Score))
		}
	}
	return out
}

// ordineRegola è la precedenza: più basso viene prima. Serve a ordinare i candidati a parità di
// punteggio e a scegliere quale regola "spiega" il messaggio.
var ordineRegola = map[string]int{RegolaMarcatore: -1, R0Reply: 0, R1Conversazione: 1, R4Riferimento: 2, R3Codice: 3, R2Oggetto: 4, R5Buyer: 5}

// SogliaEvidenza separa un candidato che è EVIDENZA di una RFQ esistente da uno che è solo un modo di
// mettere in fila la lista. Sotto la soglia un candidato si vede ma non impedisce di proporre una
// richiesta nuova: ogni RFQ nuova di un buyer noto avrebbe un candidato R5, e se bastasse quello nessuna
// richiesta nuova verrebbe mai proposta come nuova.
//
// Smistamento M1: da 50 a 60, cioè dal livello «media» in su. Sotto restano il solo ConversationID
// (Exchange mette nella stessa conversazione la posta con lo stesso oggetto, anche fuori da Outlook),
// l'indice di un altro cliente, l'oggetto uguale e il buyer: prima R2 (55) e R1 senza indice (95)
// bastavano a impedire «nuova RFQ» a una richiesta nuova che si chiamava come una vecchia.
const SogliaEvidenza = 60

// FrasePariMerito è il motivo del triage quando le prime due RFQ aperte hanno evidenze della stessa
// forza (P37): nessuna delle due si propone, e decide chi guarda le evidenze.
const FrasePariMerito = "due RFQ con evidenze della stessa forza: scegli guardando le evidenze"

// Candidato è una proposta di aggancio: un thread, la regola che l'ha indicato, e la frase che
// l'operatore legge per capire perché. Non è una decisione e non diventa mai tale da sola.
type Candidato struct {
	ThreadID  string
	Regola    string
	Punteggio int
	Evidenza  string
	Chiuso    bool // la RFQ indicata è CHIUSA: si mostra lo stesso, con l'avviso (T2)
	// Tipo è la variante (Smistamento M1). Vuoto in un candidato costruito con la sola regola: allora
	// vale quello che (regola, punteggio) dice, come per una riga letta dal database.
	Tipo string
	// DiPrima: la riga è stata scritta con le regole di prima (A5.10) e si legge con prudenza.
	DiPrima bool
}

// NuovoCandidato costruisce un candidato di quel tipo, con la regola e lo score della tabella.
func NuovoCandidato(threadID, tipo, evidenza string, chiuso bool) Candidato {
	t, _ := Tipo(tipo)
	return Candidato{ThreadID: threadID, Regola: t.Regola, Punteggio: t.Score, Evidenza: evidenza, Chiuso: chiuso, Tipo: t.Nome}
}

// TipoDi è il tipo del candidato: quello dichiarato, o quello che dicono regola e punteggio.
func (c Candidato) TipoDi() string {
	if c.Tipo != "" {
		return c.Tipo
	}
	t, _ := LeggiRiga(c.Regola, c.Punteggio, c.Evidenza)
	return t
}

// Livello è la forza del candidato secondo il suo tipo.
func (c Candidato) Livello() Livello {
	t, _ := Tipo(c.TipoDi())
	return t.Livello
}

// fraseR0DiPrima è il pezzo della frase con cui R0 si presentava prima di M1, quando non verificava i
// partecipanti. Serve solo a riconoscere quelle righe: allo stesso score (98) il tipo nuovo è verificato,
// quello di prima no.
const fraseR0DiPrima = "punta a un messaggio già agganciato a questa richiesta"

// LeggiRiga ricostruisce il tipo di una riga di `candidato_aggancio` e dice se è stata scritta con le
// regole di prima (A5.10). Le righe di prima si leggono con PRUDENZA, verso il basso, mai più in alto
// di quanto le regole nuove direbbero: R1 a 95 era il solo ConversationID (40), R0 a 98 non verificava
// chi scriveva (88), R3 a 80 non guardava il buyer (62), R2 a 55 era l'oggetto (20), R5 a 35 il buyer (15),
// R4 a 90 è il riferimento (84). Il pannello lo dice, e offre «Ricalcola».
func LeggiRiga(regola string, punti int, evidenza string) (tipo string, diPrima bool) {
	if regola == R0Reply && punti == 98 && strings.Contains(evidenza, fraseR0DiPrima) {
		return TipoR0NonVerificato, true
	}
	for _, t := range tipiEvidenza {
		if t.Regola == regola && t.Score == punti {
			return t.Nome, false
		}
	}
	switch regola {
	case R0Reply:
		return TipoR0NonVerificato, true
	case R1Conversazione:
		return TipoR1Solo, true
	case R4Riferimento:
		return TipoR4Riferimento, true
	case R3Codice:
		return TipoR3Codice, true
	case R2Oggetto:
		return TipoR2Oggetto, true
	case R5Buyer:
		return TipoR5Buyer, true
	}
	return "", true
}

// TipoDaRiga è il tipo di una riga letta dal database (A5.16.3), senza la frase.
func TipoDaRiga(regola string, punti int) string {
	t, _ := LeggiRiga(regola, punti, "")
	return t
}

// LivelloDaScore legge la forza di uno score già scritto, senza sapere la regola: è il caso della
// confidenza di `proposta_triage`, che per «aggancia» è lo score della prima RFQ. Gli score attuali
// hanno il loro livello; quelli delle regole di prima si leggono verso il basso come in LeggiRiga (95 era
// R1 senza indice, 80 era R3 senza buyer, 55 l'oggetto). Il 98 di prima non si distingue dal 98 nuovo.
func LivelloDaScore(score int) Livello {
	for _, t := range tipiEvidenza {
		if t.Score == score {
			return t.Livello
		}
	}
	switch score {
	case 95:
		return Debole
	case 90:
		return Forte
	case 80:
		return Media
	case 55, 35:
		return MoltoDebole
	}
	switch {
	case score >= 96:
		return MoltoForte
	case score >= 84:
		return Forte
	case score >= 72:
		return MedioForte
	case score >= SogliaEvidenza:
		return Media
	case score >= 40:
		return Debole
	case score > 0:
		return MoltoDebole
	}
	return LivelloNessuno
}

// OrdinaCandidati mette i più forti in cima e, a parità, rispetta la precedenza delle regole.
func OrdinaCandidati(c []Candidato) []Candidato {
	sort.SliceStable(c, func(i, j int) bool {
		if c[i].Punteggio != c[j].Punteggio {
			return c[i].Punteggio > c[j].Punteggio
		}
		return ordineRegola[c[i].Regola] < ordineRegola[c[j].Regola]
	})
	return c
}

// CandidatoRFQ è una RFQ candidata con TUTTE le sue evidenze (Smistamento M1, A5.16.3). La stessa RFQ
// indicata da tre regole è una card sola con tre righe, non tre righe che sembrano tre RFQ.
type CandidatoRFQ struct {
	ThreadID string
	Chiuso   bool
	Livello  Livello // del tipo più forte
	Score    int     // del tipo più forte
	Tipo     string  // il tipo più forte
	Evidenze []Candidato
	Rango    int  // 1 = primo nell'ordine
	DiPrima  bool // almeno una riga scritta con le regole di prima
}

// NTipi è il numero di tipi distinti almeno deboli: a pari livello e score, due evidenze indipendenti
// vengono prima di una. Gli indizi molto deboli non contano: tre di loro non fanno una prova.
func (c CandidatoRFQ) NTipi() int {
	visti := map[string]bool{}
	for _, e := range c.Evidenze {
		if e.Livello() >= Debole {
			visti[e.TipoDi()] = true
		}
	}
	return len(visti)
}

// RaggruppaEOrdina mette insieme le evidenze di ogni RFQ e ordina le RFQ a livelli: prima il livello,
// poi lo score, poi il numero di tipi distinti almeno deboli, poi la precedenza della regola più forte.
// A parità di tutto resta l'ordine d'ingresso (quello di ListCandidatiAggancio: la RFQ più recente
// prima). Nessun valore si somma: tre indizi deboli contro una prova perdono sempre.
//
// Ogni candidato vale il suo TIPO: una riga delle regole di prima entra con lo score prudente che
// LeggiRiga le dà, non con quello che porta scritto.
func RaggruppaEOrdina(c []Candidato) []CandidatoRFQ {
	indice := map[string]int{}
	var out []CandidatoRFQ
	for _, k := range c {
		tipo, diPrima := k.Tipo, k.DiPrima
		if tipo == "" {
			tipo, diPrima = LeggiRiga(k.Regola, k.Punteggio, k.Evidenza)
		}
		if t, ok := Tipo(tipo); ok {
			k.Tipo, k.Regola, k.Punteggio = t.Nome, t.Regola, t.Score
		}
		k.DiPrima = diPrima
		i, ok := indice[k.ThreadID]
		if !ok {
			i = len(out)
			indice[k.ThreadID] = i
			out = append(out, CandidatoRFQ{ThreadID: k.ThreadID})
		}
		g := &out[i]
		g.Evidenze = append(g.Evidenze, k)
		g.Chiuso = g.Chiuso || k.Chiuso
		g.DiPrima = g.DiPrima || k.DiPrima
	}
	for i := range out {
		ev := out[i].Evidenze
		sort.SliceStable(ev, func(a, b int) bool {
			if la, lb := ev[a].Livello(), ev[b].Livello(); la != lb {
				return la > lb
			}
			if ev[a].Punteggio != ev[b].Punteggio {
				return ev[a].Punteggio > ev[b].Punteggio
			}
			return ordineRegola[ev[a].Regola] < ordineRegola[ev[b].Regola]
		})
		out[i].Tipo, out[i].Score, out[i].Livello = ev[0].TipoDi(), ev[0].Punteggio, ev[0].Livello()
	}
	sort.SliceStable(out, func(a, b int) bool {
		x, y := out[a], out[b]
		if x.Livello != y.Livello {
			return x.Livello > y.Livello
		}
		if x.Score != y.Score {
			return x.Score > y.Score
		}
		if nx, ny := x.NTipi(), y.NTipi(); nx != ny {
			return nx > ny
		}
		return ordineRegola[x.Evidenze[0].Regola] < ordineRegola[y.Evidenze[0].Regola]
	})
	for i := range out {
		out[i].Rango = i + 1
	}
	return out
}

// PariMerito dice se le prime due RFQ APERTE hanno lo stesso livello, almeno «media» (P37, U7 estesa
// alla posta). Allora il sistema non propone nessuna delle due: scegliere fra due RFQ equivalenti è
// esattamente la decisione che spetta a chi guarda le evidenze. Sotto «media» non c'è niente da
// proporre comunque, e due indizi deboli pari non sono una discordanza.
func PariMerito(r []CandidatoRFQ) bool {
	var aperte []CandidatoRFQ
	for _, g := range r {
		if !g.Chiuso {
			aperte = append(aperte, g)
			if len(aperte) == 2 {
				break
			}
		}
	}
	return len(aperte) == 2 && aperte[0].Livello == aperte[1].Livello && aperte[0].Livello >= Media
}

// PariMeritoFra è PariMerito sui candidati sciolti.
func PariMeritoFra(c []Candidato) bool { return PariMerito(RaggruppaEOrdina(c)) }

// EvidenzaDiRFQEsistente dice se fra i candidati c'è qualcosa che afferma «questa richiesta esiste
// già ed è aperta». È la condizione che impedisce di valutare `nuova_rfq` (§3 del checkpoint 3R).
//
// Le RFQ CHIUSE non contano, e la differenza è voluta (T2). Una richiesta chiusa è finita: un
// messaggio che arriva dopo, con lo stesso codice o lo stesso oggetto, è più spesso una richiesta
// nuova sullo stesso pezzo — in questo mestiere lo stesso particolare viene riquotato più volte. Il
// candidato però NON sparisce: si vede, con l'avviso che quella richiesta è chiusa. Nasconderlo
// vorrebbe dire lasciare l'operatore a creare un doppione senza sapere che il precedente esiste.
func EvidenzaDiRFQEsistente(c []Candidato) bool {
	for _, k := range c {
		if k.Punteggio >= SogliaEvidenza && !k.Chiuso {
			return true
		}
	}
	return false
}

// MiglioreCandidatoAperto è la più forte fra le RFQ che sono EVIDENZA di una richiesta aperta (sopra la
// soglia, richiesta non chiusa), con la sua evidenza più forte. È il candidato verso cui si propone
// «aggancia»: il più forte in assoluto può essere una richiesta chiusa, che si mostra con l'avviso ma non
// si propone (T2). False se non ce n'è, e false anche a PARI MERITO (P37): allora l'evidenza di una RFQ
// esistente c'è, ma quale sia lo decide una persona.
func MiglioreCandidatoAperto(c []Candidato) (Candidato, bool) {
	g := RaggruppaEOrdina(c)
	if PariMerito(g) {
		return Candidato{}, false
	}
	for _, x := range g {
		if !x.Chiuso && x.Score >= SogliaEvidenza {
			return x.Evidenze[0], true
		}
	}
	return Candidato{}, false
}

// CandidatoChiuso restituisce il primo candidato forte verso una richiesta chiusa: è quello di cui la
// schermata deve avvisare.
func CandidatoChiuso(c []Candidato) (Candidato, bool) {
	for _, k := range OrdinaCandidati(append([]Candidato{}, c...)) {
		if k.Punteggio >= SogliaEvidenza && k.Chiuso {
			return k, true
		}
	}
	return Candidato{}, false
}

// MiglioreCandidato restituisce il candidato più forte, o false se non ce n'è nessuno.
func MiglioreCandidato(c []Candidato) (Candidato, bool) {
	if len(c) == 0 {
		return Candidato{}, false
	}
	return OrdinaCandidati(append([]Candidato{}, c...))[0], true
}

// pariMerito dice se il triage deve fermarsi al pari merito (P37): c'è l'evidenza di una RFQ aperta, ma
// le prime due aperte hanno la stessa forza. Restituisce la prima delle due, che dà lo score di
// «aggancia»: l'esito resta quello, nessuna delle due si propone.
func pariMerito(c []Candidato) (CandidatoRFQ, bool) {
	if !EvidenzaDiRFQEsistente(c) {
		return CandidatoRFQ{}, false
	}
	g := RaggruppaEOrdina(c)
	if !PariMerito(g) {
		return CandidatoRFQ{}, false
	}
	for _, x := range g {
		if !x.Chiuso {
			return x, true
		}
	}
	return CandidatoRFQ{}, false
}

// OrigineDaBozza dice per quale RFQ è nata la nostra mail preparata dal Cockpit (D84): la RFQ della
// bozza e quella di ADESSO della mail a cui la bozza rispondeva. Una sola se coincidono o se ce n'è una;
// due se sono diverse (bozza preparata per T1 in risposta a una mail che oggi sta in T2): si mostrano
// tutte e due e nessuna si propone; nessuna se la bozza nacque su un orfano mai agganciato.
func OrigineDaBozza(thread, threadRisposta string) []string {
	switch {
	case thread == "" && threadRisposta == "":
		return nil
	case thread == "" || thread == threadRisposta:
		return []string{threadRisposta}
	case threadRisposta == "":
		return []string{thread}
	}
	return []string{thread, threadRisposta}
}
