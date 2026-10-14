package motore_a

// spiegazioni.go: il perché dell'ancoraggio, detto all'operatore (giro 5, A1d, notte 14/10, WP-U, U2; diagnosi UI del
// 14/10, G4: «ancoraggi.diagnostiche e ancoraggi.file[].motivi non sono resi»). Le diagnostiche dell'ancoraggio
// (EsitoAncoraggi.Diagnostiche) e i motivi di un file (AncoraggioFile.Motivi) arrivano dal server come codici; qui ogni
// codice ha un titolo breve e una frase in italiano piano, che la pagina mette accanto al prodotto o al file che
// riguardano, così chi guarda capisce perché un prodotto non ha un albero o un file non si colloca.
//
// Non sono etichette (etichette.go): il dominio codice_diagnostica dà il nome del codice, qui c'è la spiegazione, e
// la tabella delle etichette ha una prova a mondo chiuso (TestEtichettaCopreOgniCostante: nessuna voce senza la sua
// costante) che non ammette i codici che il motore non produce ancora. La tabella delle spiegazioni invece porta già
// i codici che WP-A (ancoraggio, notte 14/10) introduce: la diagnostica radice_dal_nome_file e il motivo di file
// namespace_non_confrontabile; finché il motore non li produce non compaiono mai, e la pagina non si rompe quando
// arrivano. La prova TestSpiegazioniCopronoICodiciDiAncoraggio legge le costanti dai sorgenti di core/ancoraggio:
// ogni motivo e ogni codice delle strutture ha la sua spiegazione; una voce senza costante deve stare fra quelle attese
// da WP-A (codiciAttesiWPA). Da K4 anche il codice delle equivalenze del cartiglio contese, che la pagina mostra fra le
// diagnostiche della RFQ; il motivo del candidato (equivalenza_cartiglio) non ha spiegazione, perché la pagina non
// mostra i motivi dei candidati.
//
// Tolleranza [T]: un codice che la tabella non conosce non torna mai fra quadre (la pagina non le ammette, T-A1d-29):
// il titolo è l'etichetta del dominio codice_diagnostica se il codice ce l'ha (le altre diagnostiche dell'ancoraggio:
// richiesta non valutata, righe con la stessa base, completamento e correzione documentale…), altrimenti il codice senza
// il prefisso «ancoraggio.» e con gli spazi al posto dei trattini bassi; la frase è vuota; il codice grezzo resta nel
// title per l'admin. Nessuna regola nuova: si legge il codice e si sceglie il testo.

import (
	"reflect"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
)

// prefissoAncoraggio: l'area dei codici delle diagnostiche dell'ancoraggio («ancoraggio.<caso>»). I motivi dei file
// usano gli stessi valori per i due codici delle strutture e il solo caso per gli altri (ancoraggi.go, Motivo*): la
// tabella è per caso, senza l'area.
const prefissoAncoraggio = "ancoraggio."

// Le origini di una struttura candidata (StrutturaProdotto.Origine) che la pagina mostra con un badge (U2, WP-A, notte
// 14/10): la radice proposta dal nome dello STEP, o dallo STEP associato al prodotto da una persona. Le altre origini
// (la base del target, la radice scelta) e l'origine assente non hanno badge: sono il caso di sempre.
const (
	OrigineStrutturaNomeFile     = "nome_file_step"
	OrigineStrutturaAssociazione = "associazione_step"
)

// dominioCodiceDiagnostica: il dominio delle etichette dei codici delle diagnostiche (etichette.go).
const dominioCodiceDiagnostica = "codice_diagnostica"

// MotivoFileNaturaNonFile: il motivo di un file che non è un file (inline, elemento di Outlook, collegamento), lo stesso
// valore di valutazione.MotivoFileNaturaNonFile (U4: nascosti dagli elenchi dei file finché chi guarda non li chiede).
const MotivoFileNaturaNonFile = "natura_non_file"

// Spiegazione: un codice dell'ancoraggio detto all'operatore. Codice è il valore grezzo così com'è (per il title);
// Titolo non è mai vuoto; Frase può esserlo (un codice che la tabella non conosce). Messaggio e Gravita sono quelli
// della diagnostica del server, così come sono ("" per un motivo di file); la gravità è un testo (errore | avviso |
// nota) perché il sottopackage non importa evidenze (la tabella delle dipendenze di
// core/estrazione/evidenze/dipendenze_test.go).
type Spiegazione struct {
	Codice    string
	Titolo    string
	Frase     string
	Messaggio string
	Gravita   string
}

// voceSpiegazione: titolo e frase di un caso.
type voceSpiegazione struct{ titolo, frase string }

// spiegazioni: caso (senza «ancoraggio.») → titolo e frase. I testi sono [T] di questa corsia: la proposta del
// frontend, da far firmare come le etichette (contratto A1d §2.3).
var spiegazioni = map[string]voceSpiegazione{
	// le strutture dei prodotti (codici_diagnostica.go), anche motivi dei file
	"target_senza_struttura": {"prodotto senza struttura",
		"Nessun nodo degli STEP letti porta il codice del prodotto, e nessuno STEP è stato autorizzato per lui: il prodotto non ha un albero finché qualcuno non sceglie il suo STEP."},
	"grafo_incompleto": {"struttura dello STEP incompleta",
		"Lo STEP non dichiara di avere tutti i legami padre-figlio: un legame che manca non vuol dire che non ci sia."},
	// WP-A (notte 14/10): la radice proposta dal nome del file
	"radice_dal_nome_file": {"radice dal nome del file",
		"I nodi dello STEP non portano il codice del prodotto, ma il nome del file sì: la radice del file è proposta come struttura candidata. Va guardata prima di autorizzarla."},
	"radice_dallo_step_associato": {"radice dallo STEP associato",
		"Lo STEP è stato associato al prodotto da una persona, ma i suoi nodi non portano il codice del prodotto: la sua radice è proposta come struttura candidata. Va guardata prima di autorizzarla."},
	// K4: l'equivalenza del cartiglio fra il codice della parte e il documento 3D che non si propone
	"equivalenza_cartiglio_contesa": {"equivalenza del cartiglio non univoca",
		"Un cartiglio scrive il codice della parte accanto al documento 3D, ma non in modo univoco (più codici, più riferimenti, o lo stesso riferimento con codici diversi in altri disegni): il motore non propone il disegno sul nodo del 3D, va guardato a mano."},
	// i motivi della collocazione di un file (ancoraggi.go, MotivoAncoraggio*)
	"target_ignoti": {"nessun prodotto della RFQ",
		"La RFQ non ha prodotti: un file non si può mettere sotto nessuno."},
	"nessuna_lettura_identita": {"nessun codice letto dal file",
		"La grammatica del cliente non legge un codice da questo file, né dal nome né dal contenuto: il motore non sa a quale prodotto appartiene."},
	"livelli_diversi": {"posizione contraddittoria",
		"Il file risulta sia un prodotto della RFQ sia un figlio di un altro prodotto: la posizione non si sceglie da sola."},
	"identita_discordanti": {"codici del file discordanti",
		"Il file porta codici che non concordano fra loro (per esempio il nome e il cartiglio): va deciso a mano."},
	"revisioni_discordanti": {"revisioni discordanti",
		"Lo stesso codice si raggiunge con revisioni lette diverse: quale vale si decide a mano."},
	"completamenti_multipli": {"codice parziale ambiguo",
		"Il codice letto è parziale e si completa in più modi: il file resta da decidere."},
	"candidati_multipli": {"più destinazioni possibili",
		"Il file può stare sotto più prodotti o componenti: il motore non sceglie al posto tuo."},
	// WP-A (notte 14/10): il motivo della guardia su fuori_richiesta
	"namespace_non_confrontabile": {"codice di un'altra famiglia",
		"Il codice letto dal file è di una famiglia di codici che non compare fra i prodotti della RFQ (per esempio un numero di disegno contro un numero di materiale): il motore non può dire che il file è fuori richiesta."},
}

// Spiega: la spiegazione di un codice dell'ancoraggio (una diagnostica «ancoraggio.<caso>» o un motivo di file, con o
// senza l'area). Un codice che la tabella non conosce ha il titolo dall'etichetta della diagnostica o dal codice e la
// frase vuota, mai le quadre.
func Spiega(codice string) Spiegazione {
	caso := strings.TrimPrefix(strings.TrimSpace(codice), prefissoAncoraggio)
	if v, ok := spiegazioni[caso]; ok {
		return Spiegazione{Codice: codice, Titolo: v.titolo, Frase: v.frase}
	}
	if t, ok := voce(dominioCodiceDiagnostica, prefissoAncoraggio+caso); ok {
		return Spiegazione{Codice: codice, Titolo: t}
	}
	titolo := strings.TrimSpace(strings.ReplaceAll(caso, "_", " "))
	if titolo == "" {
		titolo = "motivo senza codice"
	}
	return Spiegazione{Codice: codice, Titolo: titolo}
}

// spiegaDiagnostica: la spiegazione di una diagnostica (codice, messaggio e gravità del server).
func spiegaDiagnostica(codice, messaggio, gravita string) Spiegazione {
	s := Spiega(codice)
	s.Messaggio, s.Gravita = messaggio, gravita
	return s
}

// DiagnosticheAncoraggio: le diagnostiche dell'ancoraggio che riguardano il prodotto (il suo Rif è fra i Rif della
// diagnostica: ProponiStrutture lo mette per primo), spiegate, nell'ordine del server; nil senza.
func (x *Indice) DiagnosticheAncoraggio(prodotto string) []Spiegazione {
	if x.vuoto() || prodotto == "" {
		return nil
	}
	var out []Spiegazione
	for _, d := range x.e.Ancoraggi.Diagnostiche {
		for _, r := range d.Rif {
			if r == prodotto {
				out = append(out, spiegaDiagnostica(d.Codice, d.Messaggio, string(d.Gravita)))
				break
			}
		}
	}
	return out
}

// DiagnosticheAncoraggioDellaRFQ: le diagnostiche dell'ancoraggio che non riguardano nessun prodotto valutato (nessuno
// dei loro Rif è il Rif di un prodotto), spiegate, nell'ordine del server; nil senza.
func (x *Indice) DiagnosticheAncoraggioDellaRFQ() []Spiegazione {
	if x.vuoto() {
		return nil
	}
	var out []Spiegazione
	for _, d := range x.e.Ancoraggi.Diagnostiche {
		di := false
		for _, r := range d.Rif {
			if _, ok := x.prodotti[r]; ok {
				di = true
				break
			}
		}
		if !di {
			out = append(out, spiegaDiagnostica(d.Codice, d.Messaggio, string(d.Gravita)))
		}
	}
	return out
}

// ancoraggioFile: l'ancoraggio dell'allegato (EsitoAncoraggi.File), il primo con quell'allegato; nil senza.
func (x *Indice) ancoraggioFile(a uuid.UUID) *ancoraggio.AncoraggioFile {
	if x.vuoto() {
		return nil
	}
	for i := range x.e.Ancoraggi.File {
		if x.e.Ancoraggi.File[i].AllegatoID == a {
			return &x.e.Ancoraggi.File[i]
		}
	}
	return nil
}

// MotiviAncoraggio: i motivi dell'ancoraggio dell'allegato (AncoraggioFile.Motivi), spiegati, nell'ordine del server;
// nil senza.
func (x *Indice) MotiviAncoraggio(a uuid.UUID) []Spiegazione {
	af := x.ancoraggioFile(a)
	if af == nil {
		return nil
	}
	var out []Spiegazione
	for _, m := range af.Motivi {
		out = append(out, Spiega(m))
	}
	return out
}

// BadgeOrigine: il badge dell'origine di una struttura candidata (U2, WP-A): il testo e il title.
type BadgeOrigine struct {
	Testo, Titolo string
}

// OrigineStruttura: il badge dell'origine della struttura del prodotto in quell'allegato con quella radice; nil per le
// origini senza badge o assenti. Il campo Origine si legge per nome (reflect) dalla struttura di ancoraggio
// (StrutturaProdotto) e, se non c'è, dalla struttura candidata dell'esito (StrutturaCandidata): WP-A lo aggiunge al
// tipo di ancoraggio nella stessa notte, in un altro ramo, e la pagina deve compilare e rendersi prima e dopo l'unione
// [T]. Quando il campo c'è in tutti e due i rami la lettura per nome si può sostituire con il campo.
func (x *Indice) OrigineStruttura(prodotto string, a uuid.UUID, radice string) *BadgeOrigine {
	if x.vuoto() {
		return nil
	}
	origine := ""
	for _, st := range x.struttureNellAllegato(prodotto, a) {
		if st.Radice == radice {
			origine = campoTesto(*st, "Origine")
			break
		}
	}
	if origine == "" {
		if p := x.Prodotto(prodotto); p != nil {
			for i := range p.StruttureCandidate {
				if sc := &p.StruttureCandidate[i]; sc.AllegatoID == a && sc.Radice == radice {
					origine = campoTesto(*sc, "Origine")
					break
				}
			}
		}
	}
	return badgeOrigine(origine)
}

// badgeOrigine: il badge di un valore dell'origine di una struttura; nil per le origini senza badge.
func badgeOrigine(origine string) *BadgeOrigine {
	switch origine {
	case OrigineStrutturaNomeFile:
		return &BadgeOrigine{Testo: "radice dal nome del file", Titolo: "origine = " + origine +
			": i nodi dello STEP non portano il codice del prodotto, il nome del file sì. Va guardata prima di autorizzarla."}
	case OrigineStrutturaAssociazione:
		return &BadgeOrigine{Testo: "radice dallo STEP associato", Titolo: "origine = " + origine +
			": lo STEP è associato al prodotto da una persona; la sua radice è proposta anche se non porta il codice del prodotto."}
	}
	return nil
}

// campoTesto: il valore del campo stringa (anche di un tipo con nome) «nome» della struct v; "" se il campo non c'è o
// non è una stringa.
func campoTesto(v any, nome string) string {
	r := reflect.ValueOf(v)
	if r.Kind() != reflect.Struct {
		return ""
	}
	f := r.FieldByName(nome)
	if !f.IsValid() || f.Kind() != reflect.String {
		return ""
	}
	return f.String()
}
