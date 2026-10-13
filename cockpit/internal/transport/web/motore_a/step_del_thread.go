package motore_a

// step_del_thread.go: gli STEP della RFQ per un prodotto senza strutture candidate, e gli elementi della mail (giro 5,
// A1d, notte 14/10, WP-U, U3 e U4; diagnosi UI del 14/10, G1 e §3.3).
//
// U3 (G1, il prodotto senza strutture candidate). Quando un prodotto ha struttura «nessuna» la pagina era un vicolo
// cieco: «nessuna struttura candidata», e nessun gesto portava uno STEP a fonte. Il motore invece sa costruire la BOM
// di lavoro da una fonte confermata con la radice scelta da una persona anche se la radice non porta il codice del
// prodotto (core/ancoraggio/struttura_prodotto.go, R76 A). StepDelThread elenca sotto il prodotto ogni STEP della RFQ
// che ha una struttura (le entità nodo_step del documento delle evidenze), con il testo delle sue radici, e dice quale
// gesto esistente porta da lì all'autorizzazione dello STEP strutturale:
//   - GestoAutorizza: il file è già un documento corrente del componente del prodotto (lo stesso documento che vuole
//     l'autorizzazione, documentoDelComponente): si apre l'anteprima dell'autorizzazione del server (il dialogo che la
//     pagina ha già, con data-ma-documento e senza sorgente: l'anteprima elenca le radici e nessuna è scelta);
//   - GestoUsa: il file ha una proposta aperta di 3D: la catena del Fascicolo legacy, «assegna» al componente del
//     prodotto (con correggi_codice se il codice letto dal file è scritto e diverso da quello del componente) e poi
//     «Conferma il documento» di quella proposta, due gesti esistenti del ponte, ognuno con il suo esito; dopo la
//     rilettura il file è un documento del prodotto e il gesto diventa GestoAutorizza. Decisione autonoma
//     dell'orchestratore della notte 14/10 sulla domanda aperta Q-P3-03 [T, da ratificare]: correggi_codice dalla pagina
//     nuova solo da questa azione esplicita, con la finestra che elenca che cosa si scrive (motore_a.mjs, apriUsaStep;
//     la guardia del ponte, motore_a_gesti.mjs, lo ammette solo con correzione_confermata);
//   - nessun gesto (Motivo): il prodotto senza componente, il file documento di un altro componente, nessuna proposta
//     aperta di 3D, il componente che ha già un 3D corrente (revisione della notte 14/10: la conferma del file singolo
//     chiederebbe se si aggiunge o si sostituisce, e la catena si fermerebbe a metà, con l'associazione e la correzione
//     del codice già scritte): la pagina lo dice e porta al Fascicolo sul componente del prodotto.
// Se il gesto riesce lo dice il server, non questa ricerca: le verifiche restano quelle delle rotte (FK, codice del
// componente, scelta della revisione, anteprima e firma dell'autorizzazione).
//
// U4: gli elementi della mail (FileInterpretato.Motivo = natura_non_file: inline, elementi di Outlook, collegamenti)
// non sono file e affollano gli elenchi dei file; ElementoDellaMail li riconosce dal motivo del server, la pagina li
// nasconde finché chi guarda non spunta «mostra elementi della mail».
//
// Sono ricerche come quelle di indice.go: leggono la fotografia e l'esito dello stesso calcolo, nessuna query, nessuna
// regola di classificazione nuova. La lettura delle radici è quella di valutazione (fonte.go: «una entità nodo_step è
// radice se le sue unità hanno il contesto radice_step»), ripetuta qui perché l'esito non espone le radici dei file.

import (
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/valutazione"
	"promatec/cockpit/internal/platform/db"
)

// I gesti di uno STEP della RFQ per il prodotto (StepDelThread.Gesto) [T].
const (
	GestoAutorizza = "autorizza" // l'anteprima dell'autorizzazione dello STEP strutturale, con il documento
	GestoUsa       = "usa"       // assegna (con correggi_codice se serve) + Conferma il documento, poi GestoAutorizza
)

// tipoEntitaNodoSTEP, contestoRadiceSTEP, campoIDSTEP, campoNomeSTEP: l'entità di un nodo dello STEP, il contesto di
// un'unità di una radice e i campi del testo di una radice nel documento delle evidenze (evidenze.EntitaLocale.Tipo,
// evidenze.ContestoRadiceSTEP, evidenze.CampoFonte.Valore della variante step). Valori del vocabolario chiuso di
// core/estrazione/evidenze ripetuti come testo: il sottopackage non importa evidenze (la tabella delle dipendenze di
// core/estrazione/evidenze/dipendenze_test.go); le prove li confrontano con le costanti.
const (
	tipoEntitaNodoSTEP = "nodo_step"
	contestoRadiceSTEP = "radice_step"
	campoIDSTEP        = "id"
	campoNomeSTEP      = "nome"
)

// RadiceSTEP: una radice di uno STEP: la chiave del nodo nel file e il testo letto (PRODUCT.id, altrimenti il nome).
type RadiceSTEP struct {
	Chiave string
	Testo  string
}

// PropostaSTEP: la proposta aperta del file, i campi che la finestra di «Usa come STEP» mostra.
type PropostaSTEP struct {
	ID           uuid.UUID
	Tipo         string
	Codice       string     // il codice della proposta; "" se non è scritto
	ComponenteID *uuid.UUID // il componente a cui è già associata; nil se nessuno
}

// StepDelThread: uno STEP della RFQ con una struttura, per un prodotto (U3).
type StepDelThread struct {
	AllegatoID       uuid.UUID
	NomeFile         string
	Radici           []RadiceSTEP
	Nodi             int           // le entità nodo_step del documento
	Motivi           []Spiegazione // i motivi dell'ancoraggio del file
	Gesto            string        // GestoAutorizza | GestoUsa | "" (nessun gesto: Motivo)
	Motivo           string        // perché nessun gesto, in parole
	Documento        *uuid.UUID    // con GestoAutorizza: il documento corrente del componente del prodotto
	Proposta         *PropostaSTEP // la proposta aperta del file, se c'è
	CodiceComponente string        // il codice del componente del prodotto
	Correzione       bool          // con GestoUsa: il codice della proposta è scritto e diverso da quello del componente
}

// StepDelThread: gli STEP della RFQ con una struttura (almeno un'entità nodo_step nel documento delle evidenze),
// nell'ordine dell'esito (per allegato), con il gesto che li porta all'autorizzazione per il prodotto; nil senza
// prodotto o senza STEP. Gli elementi della mail non ci sono mai (non hanno un documento).
func (x *Indice) StepDelThread(prodotto string) []StepDelThread {
	p := x.Prodotto(prodotto)
	if p == nil {
		return nil
	}
	codice := ""
	if p.ComponenteID != nil {
		if c := x.componenti[*p.ComponenteID]; c != nil {
			codice = c.Codice
		}
	}
	var out []StepDelThread
	for i := range x.e.File {
		f := &x.e.File[i]
		radici, nodi := radiciDelFile(f)
		if nodi == 0 {
			continue
		}
		s := StepDelThread{AllegatoID: f.AllegatoID, NomeFile: x.NomeFile(f.AllegatoID), Radici: radici, Nodi: nodi,
			Motivi: x.MotiviAncoraggio(f.AllegatoID), CodiceComponente: codice, Proposta: x.propostaSTEP(f.AllegatoID)}
		x.gestoDelloStep(&s, p.ComponenteID)
		out = append(out, s)
	}
	return out
}

// gestoDelloStep: il gesto (o il motivo dell'assenza) di uno STEP per il componente del prodotto (commento di testa).
func (x *Indice) gestoDelloStep(s *StepDelThread, componente *uuid.UUID) {
	switch {
	case componente == nil:
		s.Motivo = "il prodotto non ha ancora un componente: lo STEP non si può autorizzare per lui"
		return
	case x.t == nil:
		s.Motivo = "la fotografia del thread non c'è: i documenti e le proposte non si leggono"
		return
	}
	if doc := x.documentoDelComponente(*componente, s.AllegatoID); doc != nil {
		s.Gesto, s.Documento = GestoAutorizza, doc
		return
	}
	if altro := x.documentoDiAltri(*componente, s.AllegatoID); altro != "" {
		s.Motivo = altro
		return
	}
	pr := s.Proposta
	switch {
	case pr == nil:
		s.Motivo = "il file non ha una proposta aperta: si decide dalla pagina del Fascicolo"
	case pr.Tipo != string(db.TipoDocumentoCad3d):
		s.Motivo = "la proposta aperta del file non è un 3D: si corregge dalla pagina del Fascicolo"
	case x.ha3DCorrente(*componente):
		chi := s.CodiceComponente
		if chi == "" {
			chi = "il componente del prodotto"
		}
		s.Motivo = chi + " ha già un 3D corrente: si aggiunge o si sostituisce dalla pagina del Fascicolo"
	default:
		s.Gesto = GestoUsa
		s.Correzione = pr.Codice != "" && !stessoCodiceVista(pr.Codice, s.CodiceComponente)
	}
}

// ha3DCorrente: il componente ha già un documento corrente (non sostituito) di tipo cad_3d. Con un 3D corrente la
// conferma del file singolo («Conferma il documento») chiede se il file si aggiunge o sostituisce quale
// (DomandaSostituzione del piano della conferma) e non passa: «Usa come STEP» non si offre (revisione della notte 14/10).
func (x *Indice) ha3DCorrente(componente uuid.UUID) bool {
	for i := range x.t.Documenti {
		d := &x.t.Documenti[i]
		if d.SostituitoDa == nil && d.ComponenteID != nil && *d.ComponenteID == componente && d.Tipo == string(db.TipoDocumentoCad3d) {
			return true
		}
	}
	return false
}

// documentoDiAltri: il motivo, se il file è già un documento corrente di un altro componente o della RFQ senza
// componente; "" altrimenti.
func (x *Indice) documentoDiAltri(componente, a uuid.UUID) string {
	for i := range x.t.Documenti {
		d := &x.t.Documenti[i]
		if d.SostituitoDa != nil {
			continue
		}
		for _, al := range d.Allegati {
			if al != a {
				continue
			}
			if d.ComponenteID == nil {
				return "il file è già un documento della RFQ senza componente: si sposta dalla pagina del Fascicolo"
			}
			if *d.ComponenteID != componente {
				if c := x.componenti[*d.ComponenteID]; c != nil {
					return "il file è già un documento confermato di " + c.Codice + ": si sposta dalla pagina del Fascicolo"
				}
				return "il file è già un documento confermato di un altro componente: si sposta dalla pagina del Fascicolo"
			}
		}
	}
	return ""
}

// propostaSTEP: la proposta aperta dell'allegato, una copia; nil senza.
func (x *Indice) propostaSTEP(a uuid.UUID) *PropostaSTEP {
	if x.t == nil {
		return nil
	}
	for i := range x.t.Proposte {
		pr := &x.t.Proposte[i]
		if pr.AllegatoID != a || pr.Stato != string(db.StatoPropostaAperta) {
			continue
		}
		out := &PropostaSTEP{ID: pr.ID, Tipo: pr.Tipo, ComponenteID: copiaUUID(pr.ComponenteID)}
		if pr.Codice != nil {
			out.Codice = strings.TrimSpace(*pr.Codice)
		}
		return out
	}
	return nil
}

// stessoCodiceVista: il confronto dei codici come lo fa l'identità del componente, upper(codice) senza spazi ai bordi
// (stessoCodice di transport/web, fascicolo.go, che il sottopackage non importa): dice solo se la finestra deve
// annunciare la correzione; la decide il server (codiceDaComponente).
func stessoCodiceVista(a, b string) bool {
	return strings.ToUpper(strings.TrimSpace(a)) == strings.ToUpper(strings.TrimSpace(b))
}

// radiciDelFile: le radici di uno STEP nel documento delle evidenze del file (le entità nodo_step con un'unità nel
// contesto radice_step) con il loro testo, nell'ordine delle entità del documento, e il numero delle entità nodo_step.
// Il testo è quello dell'unità del campo id, altrimenti del campo nome, altrimenti la chiave del nodo.
func radiciDelFile(f *valutazione.FileInterpretato) ([]RadiceSTEP, int) {
	doc := &f.Documento
	radice := map[string]bool{}
	id, nome := map[string]string{}, map[string]string{}
	for _, u := range doc.Unita {
		if u.EntitaID == "" || string(u.Selettore.Contesto) != contestoRadiceSTEP {
			continue
		}
		radice[u.EntitaID] = true
		switch u.Selettore.Campo.Valore {
		case campoIDSTEP:
			if _, ok := id[u.EntitaID]; !ok {
				id[u.EntitaID] = strings.TrimSpace(u.Testo)
			}
		case campoNomeSTEP:
			if _, ok := nome[u.EntitaID]; !ok {
				nome[u.EntitaID] = strings.TrimSpace(u.Testo)
			}
		}
	}
	var out []RadiceSTEP
	nodi := 0
	for _, e := range doc.Entita {
		if e.Tipo != tipoEntitaNodoSTEP {
			continue
		}
		nodi++
		if !radice[e.ID] {
			continue
		}
		testo := id[e.ID]
		if testo == "" {
			testo = nome[e.ID]
		}
		if testo == "" {
			testo = e.ChiaveOriginale
		}
		out = append(out, RadiceSTEP{Chiave: e.ChiaveOriginale, Testo: testo})
	}
	return out, nodi
}

// ElementoDellaMail: l'allegato è un elemento della mail, non un file (FileInterpretato.Motivo = natura_non_file, U4).
func (x *Indice) ElementoDellaMail(a uuid.UUID) bool {
	f := x.File(a)
	return f != nil && f.Motivo == MotivoFileNaturaNonFile
}

// ElementiDellaMail: quanti allegati dell'esito sono elementi della mail (U4: il numero accanto all'interruttore).
func (x *Indice) ElementiDellaMail() int {
	if x.vuoto() {
		return 0
	}
	n := 0
	for i := range x.e.File {
		if x.e.File[i].Motivo == MotivoFileNaturaNonFile {
			n++
		}
	}
	return n
}

// IndirizzoFascicolo: la pagina del Fascicolo della RFQ sul componente del prodotto (?nodo=, come le schede della
// pagina Richieste), o sulla RFQ se il prodotto non ha un componente; "" senza la fotografia del thread.
func (x *Indice) IndirizzoFascicolo(prodotto string) string {
	if x.vuoto() || x.t == nil {
		return ""
	}
	u := "/thread/" + x.t.ID.String() + "/fascicolo"
	if p := x.Prodotto(prodotto); p != nil && p.ComponenteID != nil {
		u += "?nodo=" + p.ComponenteID.String()
	}
	return u
}
