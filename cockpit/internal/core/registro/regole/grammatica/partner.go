package grammatica

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// GP-1 (addendum «contesto partner», 16/10): emittenti partner, dichiarazione dei partner ammissibili, modelli di
// accoppiamento sul testo intero (legami), stati di forma contestuale e solo_legame, attivazione degli esempi.
//
// Tutti i campi sono facoltativi: una grammatica che non li usa ha il canonico e l'hash di prima. La grammatica
// dichiara che cosa è AMMISSIBILE; l'attivazione la dà il contesto di lavorazione (motorea, contesto.go), mai la
// grammatica da sola: un partner non si eredita da tutte le RFQ del cliente.
//
// Nessun cliente è nominato qui: gli esempi dei commenti sono inventati (ACME, BETA).

// EmittenteGrammatica: il soggetto che scrive i codici di una grammatica partner (per esempio il fornitore BETA
// che disegna per il cliente ACME). Una grammatica ha o il cliente o l'emittente, mai tutti e due.
type EmittenteGrammatica struct {
	ID   string `json:"id"`   // stabile, minuscolo: lo cita la dichiarazione del cliente e il contesto
	Nome string `json:"nome"` // solo descrittivo
}

// DichiarazionePartner: un emittente che il cliente ammette, con la versione bloccata e i soli campi su cui può
// leggere. Non attiva niente: lo fa il contesto. Non fissa un ruolo: il ruolo nasce dalla forma e dal legame.
type DichiarazionePartner struct {
	Emittente    string   `json:"emittente"`
	HashSnapshot string   `json:"hash_snapshot"`      // l'hash della grammatica dell'emittente: un altro hash è una deriva
	Selettori    []string `json:"selettori"`          // il perimetro dei campi; mai oggetto, corpo o storia
	Famiglie     []string `json:"famiglie,omitempty"` // facoltativo: solo queste famiglie dell'emittente
}

// ModelloLegame: un modello di accoppiamento che copre per intero il testo di un'unità (un nome di file, l'id di
// una radice STEP): per esempio «<ACME>__<BETA>_ASM.STEP». Le letture e le relazioni nascono solo se il modello
// copre tutto il testo; altrimenti nessuna relazione e nessun frammento solo_legame (direttiva 21 §4.2). La
// relazione si deriva dai ruoli dei pezzi: documentato_da (rif_cliente ← parte_fornitore o variante),
// varianti_nel_documento (fra le varianti).
type ModelloLegame struct {
	ID        string          `json:"id"`
	Stato     string          `json:"stato"`     // attiva | riservata
	Selettori []string        `json:"selettori"` // dove si prova; mai oggetto, corpo o storia
	Schema    []PezzoLegame   `json:"schema"`    // in ordine; copre tutto il testo
	Esempi    []EsempioLegame `json:"esempi"`
}

// PezzoLegame: un pezzo dello schema.
//   - lettura: una lettura di una delle famiglie (del cliente o di un partner dichiarato), ancorata in quel punto,
//     con il ruolo che il pezzo le dà;
//   - separatore: uno dei letterali, esatto;
//   - estensione: uno dei letterali (senza il punto), maiuscole ASCII indifferenti, preceduto da «.» e in fondo.
type PezzoLegame struct {
	Tipo      string   `json:"tipo"`
	Ruolo     string   `json:"ruolo,omitempty"`     // solo lettura: rif_cliente | parte_fornitore | variante
	Famiglie  []string `json:"famiglie,omitempty"`  // solo lettura
	Letterali []string `json:"letterali,omitempty"` // separatore, estensione
}

// EsempioLegame: una fixture del modello, verificata con l'attivazione data.
type EsempioLegame struct {
	ID          string             `json:"id"`
	Origine     string             `json:"origine"` // sintetico | caso_reale
	RifCaso     string             `json:"rif_caso,omitempty"`
	Selettore   string             `json:"selettore"`
	Testo       string             `json:"testo"`
	Attivazione AttivazioneEsempio `json:"attivazione"`
	Atteso      AttesoLegame       `json:"atteso"`
}

// AttesoLegame: le letture del legame riuscito, con il loro ruolo, oppure nessun legame.
type AttesoLegame struct {
	Nessuno bool            `json:"nessuno,omitempty"`
	Letture []LetturaAttesa `json:"letture,omitempty"`
}

// AttivazioneEsempio: come in un contesto dichiarato, per un esempio. Codifica vuota = campo non dichiarato (le
// forme contestuali del cliente e quelle dei partner attivi leggono tutte e due: alternative).
type AttivazioneEsempio struct {
	Partner  []string `json:"partner,omitempty"`
	Codifica string   `json:"codifica,omitempty"` // cliente | <id emittente> | misto
	Modelli  []string `json:"modelli,omitempty"`
}

// VoceEmittente: il file della grammatica di un emittente nell'indice, con il suo sha256.
type VoceEmittente struct {
	ID     string `json:"id"`
	File   string `json:"file"`
	Sha256 string `json:"sha256"`
}

// Stati di forma nuovi (GP-1) e valori dei legami.
const (
	// StatoContestuale: la forma è compilata ma legge solo in un contesto che dichiara il campo codificato dal
	// cliente, misto o non dichiarato (per esempio i 7 cifre di ACME, reali ma ambigui per sola forma).
	StatoContestuale = "contestuale"
	// StatoSoloLegame: la forma legge solo dentro un legame che copre tutto il testo.
	StatoSoloLegame = "solo_legame"

	PezzoLettura    = "lettura"
	PezzoSeparatore = "separatore"
	PezzoEstensione = "estensione"

	RuoloLegameCliente   = "rif_cliente"
	RuoloLegameFornitore = "parte_fornitore"
	RuoloLegameVariante  = "variante"

	// Le codifiche di un campo nel contesto e nell'attivazione degli esempi.
	CodificaCliente = "cliente"
	CodificaMista   = "misto"

	// I ruoli di una lettura nell'interpretazione (LetturaAttesa.Ruolo negli esempi dei legami).
	RuoloLetturaPrimaria    = "primaria"
	RuoloLetturaRiferimento = "riferimento"
	RuoloLetturaVariante    = "variante"
)

var (
	statiForma          = []string{StatoAttiva, StatoRiservata, StatoContestuale, StatoSoloLegame}
	statiFormaEmittente = []string{StatoAttiva, StatoRiservata, StatoSoloLegame}
	tipiPezzo           = []string{PezzoLettura, PezzoSeparatore, PezzoEstensione}
	ruoliLegame         = []string{RuoloLegameCliente, RuoloLegameFornitore, RuoloLegameVariante}
	ruoliLettura        = []string{RuoloLetturaPrimaria, RuoloLetturaRiferimento, RuoloLetturaVariante}
	funzioniMassime     = []string{"menzione", "attributo", "relazione", "struttura", "identita_file", "richiesta"}
)

// FormaCompilabile: la forma entra nel motore (con la sua condizione): attiva, contestuale o solo_legame.
func FormaCompilabile(stato string) bool {
	return stato == StatoAttiva || stato == StatoContestuale || stato == StatoSoloLegame
}

// selettoreDiMessaggio: oggetto, corpo, storia. Un partner non legge mai lì (direttiva 21 §3.1: un partner non
// eredita l'autorità di estrarre target dalla mail).
func selettoreDiMessaggio(s string) bool {
	ctx, _, _ := strings.Cut(s, ".")
	switch evidenze.Contesto(ctx) {
	case evidenze.ContestoOggetto, evidenze.ContestoCorpo, evidenze.ContestoStoria:
		return true
	}
	return false
}

// ---- validazione (chiamata da validatore.grammatica) ----

// soggetto: cliente o emittente, esattamente uno (GP-1 §2).
func (v *validatore) soggetto(g Grammatica) {
	if g.Emittente == nil {
		if g.Cliente.ID == uuid.Nil {
			v.obbligatorio("cliente.id", "l'UUID del cliente: il legame è per UUID, mai per nome")
		}
		if strings.TrimSpace(g.Cliente.RagioneSociale) == "" {
			v.obbligatorio("cliente.ragione_sociale", "la ragione sociale, che serve da controllo contro quella del DB")
		}
		return
	}
	if g.Cliente.ID != uuid.Nil || g.Cliente.RagioneSociale != "" {
		v.nonAmmesso("cliente", "una grammatica ha il cliente o l'emittente, mai tutti e due")
	}
	if strings.TrimSpace(g.Emittente.ID) == "" {
		v.obbligatorio("emittente.id", "l'ID dell'emittente")
	} else if g.Emittente.ID != strings.ToLower(g.Emittente.ID) || strings.ContainsAny(g.Emittente.ID, " :/") {
		v.nonAmmesso("emittente.id", fmt.Sprintf("l'ID dell'emittente %q va scritto minuscolo, senza spazi, «:» o «/»", g.Emittente.ID))
	}
	if strings.TrimSpace(g.Emittente.Nome) == "" {
		v.obbligatorio("emittente.nome", "il nome dell'emittente")
	}
	if len(g.Partner) > 0 {
		v.nonAmmesso("partner", "un emittente non dichiara partner: la composizione ha profondità 1")
	}
	if len(g.Legami) > 0 {
		v.nonAmmesso("legami", "i legami sono convenzioni del cliente: stanno nella sua grammatica")
	}
	for _, f := range g.Famiglie {
		for _, fo := range f.Forme {
			for j, s := range fo.Selettori {
				if selettoreDiMessaggio(s) {
					v.nonAmmesso(fmt.Sprintf("%s.selettori[%d]", percorsoFormaG(f.ID, fo.ID), j),
						fmt.Sprintf("un emittente partner non legge %q: mai target dalla mail", s))
				}
			}
		}
	}
}

func percorsoFormaG(famiglia, forma string) string {
	return "famiglie_codice[" + famiglia + "].forme[" + forma + "]"
}

// partner: le dichiarazioni dei partner ammissibili.
func (v *validatore) partner(ps []DichiarazionePartner) {
	var visti []string
	for i, p := range ps {
		pp := elemento("partner", p.Emittente, i)
		if strings.TrimSpace(p.Emittente) == "" {
			v.obbligatorio(pp+".emittente", "l'ID dell'emittente")
		} else if in(p.Emittente, visti) {
			v.errore(evidenze.Diagnostica{Codice: CodiceIDRipetuto, Percorso: pp, Messaggio: fmt.Sprintf("il partner %q è dichiarato due volte", p.Emittente)})
		} else {
			visti = append(visti, p.Emittente)
		}
		if strings.TrimSpace(p.HashSnapshot) == "" {
			v.obbligatorio(pp+".hash_snapshot", "l'hash della grammatica dell'emittente: la versione è bloccata")
		}
		v.selettori(pp+".selettori", p.Selettori)
		for j, s := range p.Selettori {
			if selettoreDiMessaggio(s) {
				v.nonAmmesso(fmt.Sprintf("%s.selettori[%d]", pp, j), fmt.Sprintf("un partner non legge %q: mai target dalla mail", s))
			}
		}
		v.duplicati(pp+".famiglie", p.Famiglie)
	}
}

// legami: i modelli di accoppiamento. Le famiglie dei partner non sono note qui: le controlla motorea quando
// compone (un legame che cita una famiglia sconosciuta non entra, con la diagnosi).
func (v *validatore) legami(ls []ModelloLegame) {
	ids := make([]string, len(ls))
	for i := range ls {
		ids[i] = ls[i].ID
	}
	v.ids("legami", ids)
	for i, l := range ls {
		pl := elemento("legami", l.ID, i)
		v.enum(pl+".stato", "stato del legame", l.Stato, stati)
		v.selettori(pl+".selettori", l.Selettori)
		for j, s := range l.Selettori {
			if selettoreDiMessaggio(s) {
				v.nonAmmesso(fmt.Sprintf("%s.selettori[%d]", pl, j), fmt.Sprintf("un legame non si prova su %q", s))
			}
		}
		if len(l.Schema) == 0 {
			v.obbligatorio(pl+".schema", "lo schema del legame")
		}
		letture, clienti, fornitori, varianti := 0, 0, 0, 0
		for j, pz := range l.Schema {
			pp := fmt.Sprintf("%s.schema[%d]", pl, j)
			if !v.enum(pp+".tipo", "tipo di pezzo", pz.Tipo, tipiPezzo) {
				continue
			}
			switch pz.Tipo {
			case PezzoLettura:
				letture++
				if v.enum(pp+".ruolo", "ruolo del pezzo", pz.Ruolo, ruoliLegame) {
					switch pz.Ruolo {
					case RuoloLegameCliente:
						clienti++
					case RuoloLegameFornitore:
						fornitori++
					case RuoloLegameVariante:
						varianti++
					}
				}
				if len(pz.Famiglie) == 0 {
					v.obbligatorio(pp+".famiglie", "le famiglie del pezzo")
				}
				v.duplicati(pp+".famiglie", pz.Famiglie)
				if len(pz.Letterali) > 0 {
					v.nonAmmesso(pp+".letterali", "un pezzo lettura non ha letterali")
				}
			default:
				if len(pz.Letterali) == 0 {
					v.obbligatorio(pp+".letterali", "i letterali del pezzo "+pz.Tipo)
				}
				v.letterali(pp+".letterali", pz.Letterali)
				if pz.Ruolo != "" || len(pz.Famiglie) > 0 {
					v.nonAmmesso(pp, "ruolo e famiglie valgono solo per un pezzo lettura")
				}
				if pz.Tipo == PezzoEstensione && j != len(l.Schema)-1 {
					v.nonAmmesso(pp, "l'estensione è l'ultimo pezzo dello schema")
				}
			}
		}
		if len(l.Schema) > 0 && letture == 0 {
			v.obbligatorio(pl+".schema", "almeno un pezzo lettura")
		}
		if fornitori > 0 && clienti == 0 {
			v.nonAmmesso(pl+".schema", "una parte_fornitore senza rif_cliente: il riferimento non diventa mai identità da solo")
		}
		if varianti == 1 {
			v.nonAmmesso(pl+".schema", "una variante sola non è un documento a più varianti")
		}
		if clienti > 1 {
			v.nonAmmesso(pl+".schema", "più di un rif_cliente nello stesso legame")
		}
		eids := make([]string, len(l.Esempi))
		for k := range l.Esempi {
			eids[k] = l.Esempi[k].ID
		}
		v.ids(pl+".esempi", eids)
		positivo := false
		for k, e := range l.Esempi {
			pe := elemento(pl+".esempi", e.ID, k)
			v.enum(pe+".origine", "origine dell'esempio", e.Origine, origini)
			v.selettore(pe+".selettore", e.Selettore)
			if e.Testo == "" {
				v.obbligatorio(pe+".testo", "il testo dell'esempio")
			}
			v.attivazione(pe+".attivazione", e.Attivazione)
			switch {
			case e.Atteso.Nessuno && len(e.Atteso.Letture) > 0:
				v.nonAmmesso(pe+".atteso", "nessun legame e letture attese insieme")
			case !e.Atteso.Nessuno && len(e.Atteso.Letture) == 0:
				v.obbligatorio(pe+".atteso", "le letture attese del legame, o nessuno: vero")
			case !e.Atteso.Nessuno:
				positivo = true
			}
			for m, la := range e.Atteso.Letture {
				v.enumFacoltativo(fmt.Sprintf("%s.atteso.letture[%d].ruolo", pe, m), "ruolo della lettura", la.Ruolo, ruoliLettura)
			}
		}
		if l.Stato == StatoAttiva && !positivo {
			v.obbligatorio(pl+".esempi", "un esempio positivo del legame attivo")
		}
	}
}

// attivazione: i valori dell'attivazione di un esempio. Partner e modelli si controllano in motorea, che li conosce.
func (v *validatore) attivazione(p string, a AttivazioneEsempio) {
	v.duplicati(p+".partner", a.Partner)
	v.duplicati(p+".modelli", a.Modelli)
	if a.Codifica != "" && a.Codifica != CodificaCliente && a.Codifica != CodificaMista && !in(a.Codifica, a.Partner) {
		v.nonAmmesso(p+".codifica", fmt.Sprintf("codifica %q: vale cliente, misto o uno dei partner dell'attivazione", a.Codifica))
	}
}

// estensioniEFunzione: i due campi facoltativi di una forma (GP-1).
func (v *validatore) estensioniEFunzione(fo FormaCodice, p string) {
	if len(fo.Estensioni) > 0 {
		for _, s := range fo.Selettori {
			if s != string(evidenze.ContestoNomeFile) {
				v.nonAmmesso(p+".estensioni", fmt.Sprintf("le estensioni valgono solo su nome_file, non su %q", s))
				break
			}
		}
		for i, e := range fo.Estensioni {
			pe := fmt.Sprintf("%s.estensioni[%d]", p, i)
			v.letterale(pe, e)
			if strings.HasPrefix(e, ".") {
				v.nonAmmesso(pe, "l'estensione si scrive senza il punto")
			}
		}
		v.duplicati(p+".estensioni", fo.Estensioni)
	}
	v.enumFacoltativo(p+".funzione_massima", "funzione massima", fo.FunzioneMassima, funzioniMassime)
	v.enumFacoltativo(p+".suffissi_ignoti", "trattamento dei suffissi ignoti", fo.SuffissiIgnoti, []string{SuffissiIgnotiSegnala})
}

// SuffissiIgnotiSegnala (G-5): dopo la lettura il testo continua con «-» (o «.») e un carattere alfanumerico che la
// forma non legge: la lettura resta, non è completa, e il suffisso si dice (motore.suffisso_non_letto).
const SuffissiIgnotiSegnala = "segnala"

// esempioPositivoDiLegame: un esempio di un legame attende una lettura di quella forma (regola 15, GP-1: le forme
// solo_legame hanno gli esempi nei legami).
func esempioPositivoDiLegame(ls []ModelloLegame, famiglia, forma string) bool {
	for _, l := range ls {
		for _, e := range l.Esempi {
			for _, la := range e.Atteso.Letture {
				if la.Famiglia == famiglia && la.Forma == forma {
					return true
				}
			}
		}
	}
	return false
}
