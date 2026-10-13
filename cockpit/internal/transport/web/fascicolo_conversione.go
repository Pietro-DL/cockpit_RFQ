package web

// La conferma contestuale del cambio di tipo nel Fascicolo legacy (giro 5, A1d P4; D-K6-01 = A [U] con la
// precisazione della direttiva 12 §6). Un gesto che, per entrare, cambierebbe un particolare in un assieme non lo fa
// da solo: il core risponde con fascicolo.RichiestaConversione e annulla la transazione, cioe' niente e' cambiato.
// Qui quella risposta diventa una domanda nel posto dell'avviso, con il componente e il cambiamento, e un form che
// rimanda lo STESSO gesto con l'id di QUEL componente nel campo `assieme`. Il core rifa' tutto, controlli compresi
// (ciclo, archiviati, autorita'), e conversione e arco entrano nella stessa transazione. «No» chiude la domanda nel
// browser e non manda niente.
//
// Le rotte che sanno farlo: relazione/accetta e file/{aid}/accetta (anche con `chiave`, il sottoalbero), solo dalla
// schermata del Fascicolo di questa RFQ (HX-Current-URL), dove i loro pulsanti stanno. Da ogni altra pagina, e per i
// gesti che non hanno un form da rimandare (l'editor della BOM, il lotto di «Conferma Fascicolo»), resta il rifiuto
// che nomina solo le strade che ci sono gia'.
//
// Non dalla vista Documenti, la predefinita (revisione D-E del 13/10, rilievo 1 [V]): li' fascicolo.mjs (clic, in fase
// di cattura sul document) svuota #fasc-avviso al primo clic che cade dentro, e il form del «Sì» uscirebbe dal
// documento prima dell'invio: il pulsante non funzionerebbe. Dalla vista Documenti resta quindi il rifiuto, con in
// piu' la strada della vista Albero, dove la domanda c'e' (stradaDallaVistaAlbero). La domanda anche nella vista
// Documenti vuole fascicolo.mjs, fuori dai file di questa corsia: e' per B5/web.

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/fascicolo"
)

// campoAssieme e' il campo del form, ripetuto, con gli id dei componenti che la persona ha confermato assiemi.
const campoAssieme = "assieme"

// confermaTipo e' la domanda che la schermata mostra (fasc_avviso), con il form che rimanda il gesto.
type confermaTipo struct {
	Domanda    string
	Codice     string // il particolare che diventerebbe assieme, per il testo del pulsante
	Componente string // il suo id, per le prove e per chi legge la pagina
	Azione     string // la rotta del gesto da rimandare, costruita dal server (mai copiata dall'indirizzo)
	Campi      []campoNascosto
}

type campoNascosto struct{ Nome, Valore string }

// decisioneTipoDal legge il campo `assieme` del form: gli id dei componenti che la persona ha confermato assiemi. Un
// id non valido e' un rifiuto, non un 400: la pagina deve tornare.
func decisioneTipoDal(r *http.Request) (fascicolo.DecisioneTipo, error) {
	ids, err := uuidDalForm(r.Form[campoAssieme])
	if err != nil {
		return fascicolo.DecisioneTipo{}, rifiuto("componente da cambiare in assieme non valido")
	}
	return fascicolo.DecisioneTipo{Assiemi: ids}, nil
}

// domandaConversione costruisce la domanda per il gesto della richiesta r: percorso e' la parte della rotta dopo
// /thread/{id}/fascicolo (gia' verificata da chi chiama), campi i campi del form che il gesto rimanda com'erano.
// Agli id gia' confermati aggiunge quello nuovo: un file con piu' sorgenti particolari chiede una conferma per volta,
// e ogni conferma vale per il suo componente soltanto. Nil (niente domanda, resta il rifiuto) se percorso e' vuoto o
// se il componente era gia' confermato: il core lo avrebbe convertito, e chiedere di nuovo girerebbe in tondo.
func domandaConversione(r *http.Request, percorso string, campi ...string) func(uuid.UUID, fascicolo.RichiestaConversione) *confermaTipo {
	return func(thread uuid.UUID, rc fascicolo.RichiestaConversione) *confermaTipo {
		if percorso == "" {
			return nil
		}
		c := &confermaTipo{Domanda: rc.Domanda(), Codice: rc.Codice, Componente: rc.Componente.String(),
			Azione: "/thread/" + thread.String() + "/fascicolo" + percorso}
		for _, n := range campi {
			if v := strings.TrimSpace(r.FormValue(n)); v != "" {
				c.Campi = append(c.Campi, campoNascosto{n, v})
			}
		}
		gia, err := uuidDalForm(r.Form[campoAssieme])
		if err != nil {
			return nil
		}
		for _, id := range gia {
			if id == rc.Componente {
				return nil
			}
			c.Campi = append(c.Campi, campoNascosto{campoAssieme, id.String()})
		}
		c.Campi = append(c.Campi, campoNascosto{campoAssieme, rc.Componente.String()})
		return c
	}
}

// stradaDallaVistaAlbero si aggiunge al rifiuto della conversione quando il gesto viene dalla vista Documenti, dove
// la domanda non si mostra: la vista Albero ha gli stessi gesti e la domanda (rilievo 1 della revisione D-E).
const stradaDallaVistaAlbero = "; oppure rifai il gesto dalla vista Albero del Fascicolo, che chiede di confermare il cambio"

// soloDalFascicolo dice se la risposta del gesto andra' alla schermata del Fascicolo di questa RFQ (threadFrammento:
// non dal Motore A, non dalla Distinta), con il suo stato, in una vista dove la domanda funziona: non la vista
// Documenti (vedi in testa al file).
func (s *Server) soloDalFascicolo(r *http.Request, thread uuid.UUID) (statoFascicolo, bool) {
	st, ok := s.dalloSchermoDelFascicolo(r, thread)
	if !ok || st.Vista == "" {
		return statoFascicolo{}, false
	}
	return st, true
}

// dallaVistaDocumenti dice se il gesto viene dalla vista Documenti del Fascicolo di questa RFQ.
func (s *Server) dallaVistaDocumenti(r *http.Request, thread uuid.UUID) bool {
	st, ok := s.dalloSchermoDelFascicolo(r, thread)
	return ok && st.Vista == ""
}

// dalloSchermoDelFascicolo dice se la risposta del gesto andra' alla schermata del Fascicolo di questa RFQ, in
// qualunque vista, con il suo stato.
func (s *Server) dalloSchermoDelFascicolo(r *http.Request, thread uuid.UUID) (statoFascicolo, bool) {
	if s.MotoreA.Accesa && dalMotoreA(r, thread) {
		return statoFascicolo{}, false
	}
	if _, ok := dallaDistinta(r.Header, thread); ok {
		return statoFascicolo{}, false
	}
	return dalFascicolo(r.Header, thread)
}

// stradaInPiuDellaConversione e' cio' che si aggiunge a «Niente è cambiato: …» di un gesto che non e' entrato.
// conversione dice che il gesto ha risposto con fascicolo.RichiestaConversione e che la domanda non si e' mostrata:
// dalla vista Documenti la frase nomina anche la vista Albero, la strada con la domanda; altrimenti niente.
func (s *Server) stradaInPiuDellaConversione(r *http.Request, thread uuid.UUID, conversione bool) string {
	if conversione && s.dallaVistaDocumenti(r, thread) {
		return stradaDallaVistaAlbero
	}
	return ""
}
