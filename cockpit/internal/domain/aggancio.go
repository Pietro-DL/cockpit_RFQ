package domain

import "sort"

// Le regole di aggancio R0–R5 (fase 4.1, D9). Qui sta la parte PURA: i nomi, i punteggi, l'ordine di
// precedenza e la frase di evidenza. Le interrogazioni al database stanno in internal/aggancio.
//
// Perché una tabella di regole e non un punteggio unico. Un punteggio unico mescola affermazioni di
// natura diversa: «il client di posta dice che questo messaggio risponde a quello» e «questi due
// messaggi hanno lo stesso oggetto» non sono la stessa cosa detta con forza diversa, sono due cose
// diverse. Sommandole si ottiene un numero che nessuno sa più leggere, e soprattutto si ottiene che
// tre indizi deboli battono una prova. L'ordine qui sotto è una PRECEDENZA: la prima regola che parla
// decide di che tipo di messaggio si tratta, le altre aggiungono candidati da mostrare.
const (
	R0Reply         = "R0_reply"         // In-Reply-To / References verso un messaggio già agganciato
	R1Conversazione = "R1_conversazione" // ConversationID di una conversazione collegata da un operatore
	R4Riferimento   = "R4_riferimento"   // riferimento della richiesta secondo il cliente (RDO, Anfrage, ODA)
	R3Codice        = "R3_codice"        // codice di famiglia già identificativo di una RFQ dello stesso cliente
	R2Oggetto       = "R2_oggetto"       // stesso oggetto, stesso cliente, dentro la finestra
	R5Buyer         = "R5_buyer"         // stesso buyer, di recente: da solo non basta mai
)

// PuntiRegola è il punteggio con cui ogni regola si presenta. Non è una probabilità (D9): è un ordine
// leggibile da una persona che deve decidere in due secondi quale candidato guardare per primo.
var PuntiRegola = map[string]int{
	R0Reply:         98,
	R1Conversazione: 95,
	R4Riferimento:   90,
	R3Codice:        80,
	R2Oggetto:       55,
	R5Buyer:         35,
}

// ordineRegola è la precedenza: più basso viene prima. Serve a ordinare i candidati a parità di
// punteggio e a scegliere quale regola "spiega" il messaggio.
var ordineRegola = map[string]int{R0Reply: 0, R1Conversazione: 1, R4Riferimento: 2, R3Codice: 3, R2Oggetto: 4, R5Buyer: 5}

// SogliaEvidenza separa un candidato che è EVIDENZA di una RFQ esistente da uno che è solo un modo di
// mettere in fila la lista. Sotto la soglia (oggi solo R5) un candidato si vede ma non impedisce di
// proporre una richiesta nuova: ogni RFQ nuova di un buyer noto avrebbe un candidato R5, e se bastasse
// quello nessuna richiesta nuova verrebbe mai proposta come nuova.
const SogliaEvidenza = 50

// Candidato è una proposta di aggancio: un thread, la regola che l'ha indicato, e la frase che
// l'operatore legge per capire perché. Non è una decisione e non diventa mai tale da sola.
type Candidato struct {
	ThreadID  string
	Regola    string
	Punteggio int
	Evidenza  string
	Chiuso    bool // la RFQ indicata è CHIUSA: si mostra lo stesso, con l'avviso (T2)
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
