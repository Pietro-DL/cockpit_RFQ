// Package motore_a è la Distinta nuova del motore A nel web (giro 5, A1d): i modelli, il calcolo in sola lettura,
// l'indice delle ricerche, le etichette e l'anteprima dello STEP. Le rotte, la sessione, i ruoli e il collegamento con i
// gestori legacy stanno nel package padre, in transport/web/motore_a.go (direttiva A1d dell'11/10, §1).
//
// Il package non importa transport/web né app/*: app/runtime lo importa per costruire le Regole all'avvio, e web le
// tiene nel campo Server.MotoreA. Derivato dalla PR #18 di Stefano Valo (ec06ca4…153117d), ristrutturato.
package motore_a

import (
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/valutazione"
)

// Stato: come si è chiuso il caricamento delle regole all'avvio (Q-A1d-03 = A [U]). Solo StatoPronta dà un motore che
// calcola; gli altri stati accesi sono errori espliciti, che non fermano il Cockpit.
type Stato string

const (
	// StatoSpenta: [motore_a].anteprima = false o sezione assente. Nessun file aperto, le rotte rispondono 404.
	StatoSpenta Stato = "spenta"
	// StatoPronta: indice letto e valido, almeno una grammatica attiva.
	StatoPronta Stato = "pronta"
	// StatoIndiceAssente: il file indicato da [motore_a].regole non c'è o non si legge.
	StatoIndiceAssente Stato = "indice_assente"
	// StatoIndiceNonValido: l'indice si legge ma è rifiutato (JSON, voci, limiti assenti o oltre i tetti, cliente
	// ripetuto): nessuna grammatica si attiva.
	StatoIndiceNonValido Stato = "indice_non_valido"
	// StatoNessunaGrammatica: l'indice è valido ma tutte le sue grammatiche sono scartate (o non ne elenca). Non è un
	// motore funzionante (Q-A1d-03 = A [U]): Insieme resta, per le diagnostiche degli scarti.
	StatoNessunaGrammatica Stato = "nessuna_grammatica"
)

// StatoCasi: il file dei casi dell'indice, diagnosticato a parte dalle grammatiche (direttiva §2.2). Un file dei casi
// che non si legge non toglie le regole.
type StatoCasi string

const (
	CasiNonDichiarati  StatoCasi = "non_dichiarati"  // l'indice non ha la voce casi
	CasiFuoriCartella  StatoCasi = "fuori_cartella"  // il nome esce dalla cartella dell'indice (R55)
	CasiNonLeggibili   StatoCasi = "non_leggibili"   // il file non c'è o non si apre
	CasiSha256Discorde StatoCasi = "sha256_discorde" // i byte non hanno lo sha256 che l'indice dichiara
	CasiNonValidi      StatoCasi = "non_validi"      // valutazione.LeggiIngressi li rifiuta
	CasiLetti          StatoCasi = "letti"
)

// Casi: la diagnosi del file dei casi. Impronta è lo sha256 grezzo dei byte letti (vuota se non letti); Numero i casi
// letti; Motivo il dettaglio tecnico, solo per il log.
type Casi struct {
	Stato    StatoCasi
	File     string
	Impronta string
	Numero   int
	Motivo   string
}

// Scarto: un cliente dell'indice la cui grammatica non si è attivata, con tutti i codici delle sue diagnostiche. Solo
// UUID e codici: nessun testo dei clienti.
type Scarto struct {
	Cliente uuid.UUID
	Codici  []string
}

// Regole: che cosa l'avvio ha preparato per la Distinta nuova, una volta sola (niente ricaricamento: direttiva §2.2).
// Il valore zero è la Distinta spenta.
//   - Insieme: le grammatiche compilate con i limiti dell'indice; non nil con StatoPronta e StatoNessunaGrammatica.
//   - Ingressi: gli ingressi del file dei casi, letti per la diagnosi. La pagina operativa non li passa al calcolo
//     (Q-A1d-10 = B, raccomandazione da confermare): li usa solo il banco.
//   - Limite: il tempo massimo di un calcolo della pagina, dall'indice (R43 B).
//   - Sha256Indice è lo sha256 grezzo del file indice; ImprontaIndice di Insieme è quella del canonico: sono diverse.
type Regole struct {
	Accesa         bool
	Stato          Stato
	Indice         string
	Insieme        *motorea.InsiemeRegole
	Ingressi       valutazione.Ingressi
	Casi           Casi
	Limite         time.Duration
	Sha256Indice   string
	VersioneIndice int
	VersioneLimiti string
	Attivi         []uuid.UUID
	Scartati       []Scarto
}

// Pronta: le regole ci sono e almeno una grammatica è attiva.
func (r Regole) Pronta() bool { return r.Accesa && r.Stato == StatoPronta && r.Insieme != nil }

// Errore: il testo generico per chi apre la pagina quando le regole non sono pronte; il dettaglio sta nel log
// dell'avvio, mai nella risposta (niente percorsi né dati dei clienti). Vuoto se pronte o spente.
func (r Regole) Errore() string {
	if !r.Accesa || r.Stato == StatoPronta {
		return ""
	}
	switch r.Stato {
	case StatoIndiceAssente:
		return "Le regole del motore A non sono caricate: l'indice indicato in [motore_a].regole non si legge. Il motivo è nel log dell'avvio."
	case StatoNessunaGrammatica:
		return "Le regole del motore A non sono caricate: l'indice è valido ma nessuna grammatica è attiva. Gli scarti sono nel log dell'avvio."
	default:
		return "Le regole del motore A non sono caricate: l'indice è stato rifiutato. Il motivo è nel log dell'avvio."
	}
}
