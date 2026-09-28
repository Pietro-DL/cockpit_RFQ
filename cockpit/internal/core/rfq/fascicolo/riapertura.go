package fascicolo

// Il comando U5 (Smistamento F7, addendum A5.15; decisioni dell'utente del 27/09, U5 e Domanda 6 = A).
//
// Gli agganci automatici sono ammessi nel database solo come proposte. Prima dello Smistamento una lettura
// dello STEP agganciava da sola un nodo al componente con lo stesso codice (componente_proposta `duplicato`
// con il componente, senza una persona che l'abbia deciso: la forma F-A), e chiudeva gli archi che lo
// toccavano (la forma F-B). F5 non lo fa piu', ma le righe scritte prima restano: questo comando le riapre,
// nelle RFQ in corso, con lo stato di prima nella storia della riga. Una persona le conferma con il gesto
// solito (accettaNodo), che ritrova il componente con chi conferma.
//
// Il perimetro: le RFQ APERTE con la working non congelata, comprese quelle in revisione (una Vn congelata e
// la bozza aperta, Domanda 6 = A), che il rapporto dice a parte. Si lavora solo sulle proposte, cioe' sulle
// righe che la working e la bozza aperta leggono: nessuna bom_versione congelata, nessuna istantanea, nessun
// componente, arco della working o documento si scrive, si ricostruisce o si reinterpreta. Le marcature
// strutturali (radice, raggruppamento, delega, sospensione) non si toccano mai: le query lo tengono anche se
// chi chiama sbagliasse. Le RFQ chiuse, unite e congelate restano fuori e si contano.
//
// Le altre forme di A5.15.1 (F-C … F-L) si tengono, si contano o si segnalano: la radice di prima dello
// Smistamento (F-C) la giustifica lo STEP strutturale del finito; i conflitti (F-D, F-L) si dicono e il gate
// li ferma gia'; le chiusure automatiche per sostituzione (F-E), le radici legate a mano (F-F), le
// pre-assegnazioni (F-G), i codici riscritti dalla D16 (F-H), i documenti confermati in blocco (F-I), i
// componenti e gli archi nati da evidenze (F-J, F-K: Provenienza) restano come sono e finiscono nel rapporto.
//
// La classificazione e' pura (ClassificaFormeLegacy): la stessa per l'anteprima, in una transazione in sola
// lettura, e per l'applicazione, che la rifa' dentro la transazione della RFQ dopo BloccaThread. Nessuna rotta
// web: lo lancia solo la riga di comando (app/runtime), e sui dati veri solo con lo Smistamento F10/F11 in
// produzione e dopo il backup (decisione dell'utente del 27/09 sera).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/platform/db"
)

// Le etichette della provenienza (A5.15.4): un componente o un arco nati da un'evidenza, non da una decisione.
const (
	ProvenienzaCodiceTrovato      = "nato da un codice trovato"
	ProvenienzaNomeAllegato       = "prodotto da un codice visto solo nel nome di un allegato"
	ProvenienzaStepNonAutorizzato = "accettato da uno STEP non autorizzato"
	ProvenienzaRadiceAMano        = "radice legata a mano senza autorizzazione"
)

// notaStessoComponente e' l'inizio della nota di un arco chiuso perche' padre e figlio sono diventati lo
// stesso componente (Pianifica, accettaGrafo).
const notaStessoComponente = "padre e figlio sono lo stesso componente"

// ------------------------------------------------------------------ il perimetro

// Le ragioni per cui una RFQ resta fuori dal comando.
const (
	EsclusaChiusa    = "chiusa"
	EsclusaUnita     = "unita a un'altra RFQ"
	EsclusaCongelata = "congelata"
)

// Perimetro dice se una RFQ e' dentro il comando U5 (A5.15.1).
type Perimetro struct {
	Dentro      bool
	InRevisione bool   // una Vn congelata e la bozza aperta: dentro, e il rapporto la dice a parte
	Motivo      string // perche' e' fuori: EsclusaChiusa, EsclusaUnita, EsclusaCongelata
	Versione    string // «V2 bozza», «V1 congelata»; vuota senza versioni
}

// PerimetroDi dice il perimetro di una RFQ dalla riga di ListRfqPerLaRiapertura. Pura. E' la regola di
// bom_working_bloccata() replicata senza bloccare niente: la working e' congelata se l'ultima versione lo e'.
func PerimetroDi(r db.ListRfqPerLaRiaperturaRow) Perimetro {
	p := Perimetro{}
	if r.UltimoNumero > 0 {
		p.Versione = fmt.Sprintf("V%d %s", r.UltimoNumero, r.UltimoStato)
	}
	switch {
	case r.Stato != db.StatoThreadAPERTA:
		p.Motivo = EsclusaChiusa
	case r.UnitoIn.Valid:
		p.Motivo = EsclusaUnita
	case r.UltimoStato == string(db.StatoBomCongelata):
		p.Motivo = EsclusaCongelata
	default:
		p.Dentro = true
		p.InRevisione = r.NCongelate > 0
	}
	return p
}

// ------------------------------------------------------------------ le forme legacy (A5.15.1)

// DatiRiapertura e' quello che serve a classificare le forme legacy di una RFQ.
type DatiRiapertura struct {
	Autorita          Autorita
	Nodi              []db.ListComponenteProposteThreadRow
	Archi             []db.ListRelazioneProposteThreadRow
	Componenti        []db.Componente
	Documenti         []db.Documento
	Relazioni         []db.ComponenteRelazione
	Rimozioni         []db.RimozioneProposta
	Identificativi    []db.IdentificativoThread
	ProposteDocumento []db.ListProposteDocumentoDellaRfqRow
}

// NodoAgganciato e' una riga F-A: un nodo agganciato per codice a un componente, senza una persona.
type NodoAgganciato struct {
	PropostaID       uuid.UUID  `json:"proposta_id"`
	Allegato         uuid.UUID  `json:"allegato_id"`
	NomeFile         string     `json:"file"`
	Chiave           string     `json:"chiave"`
	CodiceNodo       string     `json:"codice_nodo"`
	ComponenteID     uuid.UUID  `json:"componente_id"`
	CodiceComponente string     `json:"componente"`
	AgganciatoIl     *time.Time `json:"agganciato_il,omitempty"`
	// NellAutorita: il nodo e' figlio diretto di un file autorizzato, e il gate lo conta da confermare;
	// altrimenti e' guida.
	NellAutorita bool   `json:"nell_autorita"`
	Dove         string `json:"dove"` // «figlio diretto di 7120001 (STEP autorizzato)» oppure «guida»
}

// ArcoAutomatico e' una riga F-B: un arco chiuso in automatico perche' toccava un nodo F-A.
type ArcoAutomatico struct {
	Allegato uuid.UUID `json:"allegato_id"`
	NomeFile string    `json:"file"`
	Padre    string    `json:"padre"`
	Figlio   string    `json:"figlio"`
	Stato    string    `json:"stato"`
	Nota     string    `json:"nota,omitempty"`
}

// RadiceLegacy e' la radice di un file legata a un finito prima dello Smistamento: tenuta (F-C, dallo STEP
// strutturale) o legata a mano senza autorizzazione (F-F).
type RadiceLegacy struct {
	Allegato   uuid.UUID `json:"allegato_id"`
	NomeFile   string    `json:"file"`
	Chiave     string    `json:"chiave"`
	CodiceNodo string    `json:"codice_nodo"`
	Componente string    `json:"componente"`
	Stato      string    `json:"stato"`
}

// ChiusureAutomatiche sono le righe chiuse da un automatismo per una sostituzione, un riferimento cambiato,
// un prodotto archiviato (F-E): non sono uguaglianza di codice, e restano.
type ChiusureAutomatiche struct {
	Nodi      int `json:"nodi"`
	Archi     int `json:"archi"`
	Rimozioni int `json:"rimozioni"`
}

// Totale conta le chiusure automatiche.
func (c ChiusureAutomatiche) Totale() int { return c.Nodi + c.Archi + c.Rimozioni }

// GruppoInBlocco sono i documenti di una RFQ confermati nello stesso istante (F-I: euristica del piano che
// confermava in blocco). Solo per il rapporto: sono decisioni, e sono sul NAS.
type GruppoInBlocco struct {
	ConfermatoIl time.Time `json:"confermato_il"`
	Documenti    []string  `json:"documenti"`
}

// DaRivedere e' un componente nato da un'evidenza (F-J), con le etichette della provenienza.
type DaRivedere struct {
	ComponenteID uuid.UUID `json:"componente_id"`
	Componente   string    `json:"componente"`
	Etichette    []string  `json:"etichette"`
}

// ArcoDaRivedere e' un arco della working accettato da uno STEP non autorizzato (F-K).
type ArcoDaRivedere struct {
	Padre  string `json:"padre"`
	Figlio string `json:"figlio"`
}

// EffettoNelGate e' quello che il gate dira' dopo la riapertura: i figli diretti dei file autorizzati tornati da
// confermare (A5.15.2).
type EffettoNelGate struct {
	FigliDaConfermare int    `json:"figli_da_confermare"`
	FileAutorizzati   int    `json:"file_autorizzati"`
	Frase             string `json:"frase"`
}

// FormeLegacy sono le forme di A5.15.1 trovate in una RFQ.
type FormeLegacy struct {
	Agganci              []NodoAgganciato    `json:"fa_nodi"`
	Archi                []ArcoAutomatico    `json:"fb_archi"`
	RadiciTenute         []RadiceLegacy      `json:"fc_radici_tenute"`
	Conflitti            []string            `json:"fd_conflitti"`
	ChiusureAutomatiche  ChiusureAutomatiche `json:"fe_chiusure_automatiche"`
	RadiciAMano          []RadiceLegacy      `json:"ff_radici_a_mano"`
	Preassegnazioni      []string            `json:"fg_preassegnazioni"`
	CodiciRiscritti      []string            `json:"fh_codici_riscritti"`
	ConfermatiInBlocco   []GruppoInBlocco    `json:"fi_confermati_in_blocco"`
	ComponentiDaRivedere []DaRivedere        `json:"fj_componenti_da_rivedere"`
	ArchiDaRivedere      []ArcoDaRivedere    `json:"fk_archi_da_rivedere"`
	Anomalie             []string            `json:"fl_anomalie"`
	Effetto              EffettoNelGate      `json:"effetto"`
}

// DaRiaprire dice se la RFQ ha righe che il comando riapre (F-A, F-B).
func (f FormeLegacy) DaRiaprire() bool { return len(f.Agganci)+len(f.Archi) > 0 }

// Vuote dice se non c'e' niente da dire della RFQ.
func (f FormeLegacy) Vuote() bool {
	return !f.DaRiaprire() && len(f.RadiciTenute)+len(f.Conflitti)+f.ChiusureAutomatiche.Totale()+len(f.RadiciAMano)+
		len(f.Preassegnazioni)+len(f.CodiciRiscritti)+len(f.ConfermatiInBlocco)+len(f.ComponentiDaRivedere)+
		len(f.ArchiDaRivedere)+len(f.Anomalie) == 0
}

// ClassificaFormeLegacy riconosce le forme di A5.15.1 nelle righe di una RFQ. Pura.
//
// La radice di prima dello Smistamento (K1) si riconosce dalla relazione, non dal codice: e' un nodo senza un
// arco entrante nel suo file, e il file e' lo STEP strutturale (step_strutturale_id) di un finito. Decisa come
// quel finito, o ancora aperta, e' tenuta (F-C); decisa come un altro componente e' il conflitto P7 (F-D), che
// si segnala e non si tocca. Una radice di un altro file con lo stesso codice del prodotto non e' niente di
// tutto questo: se una lettura l'ha agganciata al prodotto, e' un aggancio per codice (F-A) come gli altri.
func ClassificaFormeLegacy(d DatiRiapertura) FormeLegacy {
	var f FormeLegacy
	comp := map[uuid.UUID]db.Componente{}
	for _, c := range d.Componenti {
		comp[c.ComponenteID] = c
	}
	shaDoc := map[uuid.UUID]string{}
	for _, x := range d.Documenti {
		shaDoc[x.DocumentoID] = x.Sha256
	}
	// sha dello STEP strutturale → i finiti che lo dichiarano (la forma di prima)
	stepDi := map[string][]uuid.UUID{}
	stepSha := map[uuid.UUID]string{}
	for _, c := range d.Componenti {
		if c.StepStrutturaleID.Valid {
			if sha, ok := shaDoc[c.StepStrutturaleID.UUID]; ok {
				stepDi[sha] = append(stepDi[sha], c.ComponenteID)
				stepSha[c.ComponenteID] = sha
			}
		}
	}
	entrante := map[NodoFile]bool{}
	for _, r := range d.Archi {
		entrante[NodoFile{r.RelazioneProposta.AllegatoID, r.RelazioneProposta.FiglioChiave}] = true
	}
	codice := func(id uuid.UUID) string {
		if c, ok := comp[id]; ok {
			return c.Codice
		}
		return id.String()
	}
	a := d.Autorita
	agganciati := map[NodoFile]bool{}
	perComponente := map[uuid.UUID][]RigaDelComponente{}
	for _, r := range d.Nodi {
		n := r.ComponenteProposta
		nf := NodoFile{n.AllegatoID, n.Chiave}
		radice := !entrante[nf]
		if n.ComponenteID.Valid {
			perComponente[n.ComponenteID.UUID] = append(perComponente[n.ComponenteID.UUID], RigaDelComponente{Proposta: n, Radice: radice})
		}
		marcata := haChiave(n.Evidenza, "strutturale")
		switch {
		case n.Stato == db.StatoPropostaScartata && !n.DecisoDa.Valid && n.Nota.Valid:
			f.ChiusureAutomatiche.Nodi++
		case n.Stato == db.StatoPropostaConfermata && !n.DecisoDa.Valid:
			f.Anomalie = append(f.Anomalie, fmt.Sprintf("il nodo %s di %s è confermato senza chi l'ha deciso", n.Chiave, r.NomeFile))
		}
		if marcata {
			continue // una marcatura e' di una persona: non e' mai una forma legacy da toccare
		}
		// la radice di prima dello Smistamento (K1): tenuta o in conflitto, mai riaperta
		if finiti := stepDi[n.Sha256]; radice && len(finiti) > 0 {
			for _, c := range finiti {
				rl := RadiceLegacy{Allegato: n.AllegatoID, NomeFile: r.NomeFile, Chiave: n.Chiave, CodiceNodo: nomeNodo(n),
					Componente: codice(c), Stato: string(n.Stato)}
				decisa := n.Stato == db.StatoPropostaConfermata || n.Stato == db.StatoPropostaDuplicato
				switch {
				case n.Stato == db.StatoPropostaAperta, decisa && n.ComponenteID.Valid && n.ComponenteID.UUID == c:
					f.RadiciTenute = append(f.RadiciTenute, rl)
				case decisa && n.ComponenteID.Valid:
					f.Conflitti = append(f.Conflitti, fmt.Sprintf("la radice %s di %s, lo STEP strutturale di %s, è già il componente %s (P7): non si tocca",
						n.Chiave, r.NomeFile, codice(c), codice(n.ComponenteID.UUID)))
				}
			}
			continue
		}
		if AgganciatoPerCodice(n) {
			na := NodoAgganciato{PropostaID: n.PropostaID, Allegato: n.AllegatoID, NomeFile: r.NomeFile, Chiave: n.Chiave,
				CodiceNodo: nomeNodo(n), ComponenteID: n.ComponenteID.UUID, CodiceComponente: codice(n.ComponenteID.UUID),
				AgganciatoIl: n.DecisoIl, Dove: "guida"}
			if padre, ok := a.FiglioDiretto(n.AllegatoID, n.Chiave); ok {
				na.NellAutorita = true
				na.Dove = "figlio diretto di " + codice(padre) + " (STEP autorizzato)"
			}
			f.Agganci = append(f.Agganci, na)
			agganciati[nf] = true
			continue
		}
		// «È il prodotto» dall'editor: una radice legata da una persona a un finito, su un file che non e' il suo
		// STEP strutturale e senza una marcatura (F-F)
		if c, ok := comp[n.ComponenteID.UUID]; ok && radice && n.DecisoDa.Valid && c.Tipo == db.TipoComponenteFinito &&
			(n.Stato == db.StatoPropostaDuplicato || n.Stato == db.StatoPropostaConfermata) && stepSha[c.ComponenteID] != n.Sha256 {
			f.RadiciAMano = append(f.RadiciAMano, RadiceLegacy{Allegato: n.AllegatoID, NomeFile: r.NomeFile, Chiave: n.Chiave,
				CodiceNodo: nomeNodo(n), Componente: c.Codice, Stato: string(n.Stato)})
		}
	}

	for _, r := range d.Archi {
		x := r.RelazioneProposta
		automatico := !x.DecisoDa.Valid
		stessoComponente := x.Stato == db.StatoPropostaScartata && strings.HasPrefix(x.Nota.String, notaStessoComponente)
		switch {
		case automatico && (x.Stato == db.StatoPropostaDuplicato || stessoComponente) &&
			(agganciati[NodoFile{x.AllegatoID, x.PadreChiave}] || agganciati[NodoFile{x.AllegatoID, x.FiglioChiave}]):
			f.Archi = append(f.Archi, ArcoAutomatico{Allegato: x.AllegatoID, NomeFile: r.NomeFile, Padre: x.PadreChiave,
				Figlio: x.FiglioChiave, Stato: string(x.Stato), Nota: x.Nota.String})
		case automatico && x.Stato == db.StatoPropostaScartata && !stessoComponente && x.Nota.Valid:
			f.ChiusureAutomatiche.Archi++
		case automatico && x.Stato == db.StatoPropostaConfermata:
			f.Anomalie = append(f.Anomalie, fmt.Sprintf("l'arco %s → %s di %s è confermato senza chi l'ha deciso", x.PadreChiave, x.FiglioChiave, r.NomeFile))
		}
	}
	for _, x := range d.Rimozioni {
		switch {
		case x.Stato == db.StatoPropostaScartata && !x.DecisoDa.Valid:
			f.ChiusureAutomatiche.Rimozioni++
		case x.Stato == db.StatoPropostaConfermata && !x.DecisoDa.Valid:
			f.Anomalie = append(f.Anomalie, fmt.Sprintf("la rimozione %s → %s è confermata senza chi l'ha decisa", codice(x.PadreID), codice(x.FiglioID)))
		}
	}
	// le autorizzazioni da sistemare: il conflitto P7 della forma di prima e' F-D (gia' detto sopra, con la
	// radice), il resto (due file per un componente, superata, incoerente, catena rotta) e' F-L
	for _, x := range a.Dichiarazioni.DaSistemare {
		if !strings.HasPrefix(x.Problema, "conflitto P7") {
			f.Anomalie = append(f.Anomalie, x.Frase())
		}
	}

	for _, p := range d.ProposteDocumento {
		if p.Stato == db.StatoPropostaAperta && p.ComponenteID.Valid {
			f.Preassegnazioni = append(f.Preassegnazioni, p.NomeFile)
		}
		if p.Fonte == db.FontePropostaRegolaCliente && p.RadiceStep {
			f.CodiciRiscritti = append(f.CodiciRiscritti, p.NomeFile)
		}
	}
	f.ConfermatiInBlocco = confermatiInBlocco(d.Documenti)

	ident := map[string]*db.IdentificativoThread{}
	for i := range d.Identificativi {
		ident[strings.ToUpper(d.Identificativi[i].Codice)] = &d.Identificativi[i]
	}
	for _, c := range d.Componenti {
		et := Provenienza(ProvenienzaDi{Componente: c, StepSha: stepSha[c.ComponenteID], Identificativo: ident[strings.ToUpper(c.Codice)],
			Righe: perComponente[c.ComponenteID]}, a)
		if len(et) > 0 {
			f.ComponentiDaRivedere = append(f.ComponentiDaRivedere, DaRivedere{ComponenteID: c.ComponenteID, Componente: c.Codice, Etichette: et})
		}
	}
	decisi := ArchiDecisiNellAutorita(a, soloNodi(d.Nodi), soloArchi(d.Archi))
	for _, r := range d.Relazioni {
		if ArcoDaRivedereDi(r, decisi) {
			f.ArchiDaRivedere = append(f.ArchiDaRivedere, ArcoDaRivedere{Padre: codice(r.PadreID), Figlio: codice(r.FiglioID)})
		}
	}
	f.Effetto = effettoDi(f.Agganci)
	return f
}

// effettoDi e' la frase dell'anteprima sull'effetto nel gate (A5.15.2).
func effettoDi(agganci []NodoAgganciato) EffettoNelGate {
	file := map[uuid.UUID]bool{}
	e := EffettoNelGate{}
	for _, n := range agganci {
		if n.NellAutorita {
			e.FigliDaConfermare++
			file[n.Allegato] = true
		}
	}
	e.FileAutorizzati = len(file)
	if e.FigliDaConfermare == 0 {
		e.Frase = "nessun figlio diretto di un file autorizzato torna da confermare: il gate non cambia"
		return e
	}
	e.Frase = fmt.Sprintf("il gate si fermerà su %s da confermare in %s", quanti(e.FigliDaConfermare, "figlio diretto", "figli diretti"),
		quanti(e.FileAutorizzati, "file autorizzato", "file autorizzati"))
	return e
}

// confermatiInBlocco raggruppa i documenti confermati nello stesso istante (F-I, euristica).
func confermatiInBlocco(docs []db.Documento) []GruppoInBlocco {
	per := map[time.Time][]string{}
	for _, x := range docs {
		t := x.ConfermatoIl.UTC()
		per[t] = append(per[t], x.NomeFile)
	}
	var out []GruppoInBlocco
	for t, nomi := range per {
		if len(nomi) > 1 {
			sort.Strings(nomi)
			out = append(out, GruppoInBlocco{ConfermatoIl: t, Documenti: nomi})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ConfermatoIl.Before(out[j].ConfermatoIl) })
	return out
}

// haChiave dice se il documento JSON (un oggetto) ha la chiave k al primo livello.
func haChiave(ev json.RawMessage, k string) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(ev, &m) != nil {
		return false
	}
	_, ok := m[k]
	return ok
}

// ------------------------------------------------------------------ la provenienza (A5.15.4)

// RigaDelComponente e' una riga di componente_proposta che punta a un componente, con il dato che la
// provenienza vuole: se nel suo file e' una radice (nessun arco entrante, K1).
type RigaDelComponente struct {
	Proposta db.ComponenteProposta
	Radice   bool
}

// ProvenienzaDi e' quello che serve a dire da dove viene un componente.
type ProvenienzaDi struct {
	Componente db.Componente
	// StepSha e' lo sha dello STEP strutturale del componente (la forma di prima), se c'e'.
	StepSha string
	// Identificativo e' la riga di identificativo_thread con il codice del componente, se c'e'.
	Identificativo *db.IdentificativoThread
	// Righe sono le righe di componente_proposta che puntano al componente, in qualunque stato.
	Righe []RigaDelComponente
}

// Provenienza dice da dove viene un componente, quando non viene da una decisione (A5.15.4): le etichette
// «nato da un codice trovato», «prodotto da un codice visto solo nel nome di un allegato», «accettato da uno
// STEP non autorizzato», «radice legata a mano senza autorizzazione». Pura. Nessuna etichetta cancella o
// blocca qualcosa: e' un avviso («da rivedere»), e sparisce da sola quando una decisione nuova raggiunge il
// componente: una riga nell'autorita' di un file autorizzato decisa da una persona che lo punta, oppure
// un'autorizzazione valida sua.
func Provenienza(p ProvenienzaDi, a Autorita) []string {
	c := p.Componente
	if _, ok := a.Dichiarazioni.Di(c.ComponenteID); ok {
		return nil
	}
	for _, r := range p.Righe {
		if id, ok := DecisoDaUnaPersona(r.Proposta); ok && id == c.ComponenteID && a.NodoNellAutorita(r.Proposta.AllegatoID, r.Proposta.Chiave) {
			return nil
		}
	}
	var out []string
	if c.Origine == db.OrigineComponenteCodiceRilevato {
		out = append(out, ProvenienzaCodiceTrovato)
	}
	if c.Tipo == db.TipoComponenteFinito && p.Identificativo != nil && p.Identificativo.Origine == db.OrigineIdentificativoPropostaNomeFile {
		out = append(out, ProvenienzaNomeAllegato)
	}
	if c.Origine == db.OrigineComponenteStep {
		out = append(out, ProvenienzaStepNonAutorizzato)
	}
	for _, r := range p.Righe {
		n := r.Proposta
		if r.Radice && n.DecisoDa.Valid && n.ComponenteID.Valid && n.ComponenteID.UUID == c.ComponenteID && c.Tipo == db.TipoComponenteFinito &&
			(n.Stato == db.StatoPropostaDuplicato || n.Stato == db.StatoPropostaConfermata) && !haChiave(n.Evidenza, "strutturale") &&
			n.Sha256 != p.StepSha {
			out = append(out, ProvenienzaRadiceAMano)
			break
		}
	}
	return out
}

// CoppiaDiComponenti e' un arco fra due componenti: padre, figlio.
type CoppiaDiComponenti [2]uuid.UUID

// ArchiDecisiNellAutorita sono gli archi fra componenti che una persona ha deciso nell'autorita' di un file
// autorizzato: la riga di relazione_proposta decisa da una persona (confermata o duplicato) su un arco che
// parte da una sorgente valida, con il figlio che e' un componente per una decisione. Pura.
func ArchiDecisiNellAutorita(a Autorita, nodi []db.ComponenteProposta, archi []db.RelazioneProposta) map[CoppiaDiComponenti]bool {
	perNodo := map[NodoFile]db.ComponenteProposta{}
	for _, n := range nodi {
		perNodo[NodoFile{n.AllegatoID, n.Chiave}] = n
	}
	out := map[CoppiaDiComponenti]bool{}
	for _, r := range archi {
		if !r.DecisoDa.Valid || (r.Stato != db.StatoPropostaConfermata && r.Stato != db.StatoPropostaDuplicato) {
			continue
		}
		padre, ok := a.ArcoAutorizzato(ChiaveRelazione{Allegato: r.AllegatoID, Padre: r.PadreChiave, Figlio: r.FiglioChiave})
		if !ok {
			continue
		}
		if figlio, ok := a.ComponenteDi(perNodo[NodoFile{r.AllegatoID, r.FiglioChiave}]); ok {
			out[CoppiaDiComponenti{padre, figlio}] = true
		}
	}
	return out
}

// ArcoDaRivedereDi dice se un arco della working e' stato accettato da uno STEP non autorizzato (F-K): nato
// da uno STEP (origine step), e nessuna relazione_proposta decisa da una persona nell'autorita' del padre lo
// porta (decisi: ArchiDecisiNellAutorita). Pura.
func ArcoDaRivedereDi(r db.ComponenteRelazione, decisi map[CoppiaDiComponenti]bool) bool {
	return r.Origine == db.OrigineComponenteStep && !decisi[CoppiaDiComponenti{r.PadreID, r.FiglioID}]
}

// ------------------------------------------------------------------ lettura e scrittura di una RFQ

// LeggiRiapertura legge le righe che servono a classificare le forme legacy di una RFQ. Non scrive niente, e
// non blocca niente: va bene in una transazione in sola lettura.
func LeggiRiapertura(ctx context.Context, q *db.Queries, thread uuid.UUID) (DatiRiapertura, error) {
	var d DatiRiapertura
	var err error
	if d.Autorita, d.Nodi, d.Archi, err = LeggiAutorita(ctx, q, thread); err != nil {
		return d, err
	}
	if d.Componenti, err = q.ListComponentiThread(ctx, thread); err != nil {
		return d, err
	}
	if d.Documenti, err = q.ListDocumentiThread(ctx, thread); err != nil {
		return d, err
	}
	if d.Relazioni, err = q.ListRelazioniDellaRfq(ctx, thread); err != nil {
		return d, err
	}
	if d.Rimozioni, err = q.ListRimozioniDellaRfq(ctx, thread); err != nil {
		return d, err
	}
	if d.Identificativi, err = q.ListIdentificativi(ctx, thread); err != nil {
		return d, err
	}
	d.ProposteDocumento, err = q.ListProposteDocumentoDellaRfq(ctx, uuid.NullUUID{UUID: thread, Valid: true})
	return d, err
}

// Gli esiti di una RFQ nel rapporto.
const (
	EsitoAnteprima = "anteprima"
	EsitoRiaperta  = "riaperta"
	EsitoNiente    = "niente da riaprire"
	EsitoSaltata   = "saltata"
	EsitoFallita   = "fallita"
)

// RfqRiapertura e' una RFQ del perimetro nel rapporto del comando.
type RfqRiapertura struct {
	ThreadID      uuid.UUID   `json:"rfq"`
	Cartella      string      `json:"cartella"`
	Oggetto       string      `json:"oggetto"`
	Stato         string      `json:"stato"`
	Versione      string      `json:"bom"`
	InRevisione   bool        `json:"in_revisione"`
	Forme         FormeLegacy `json:"forme"`
	Esito         string      `json:"esito"`
	Motivo        string      `json:"motivo,omitempty"`
	NodiRiaperti  int         `json:"nodi_riaperti"`
	ArchiRiaperti int         `json:"archi_riaperti"`
}

// RfqEsclusa e' una RFQ fuori dal perimetro: chiusa, unita, congelata.
type RfqEsclusa struct {
	ThreadID uuid.UUID `json:"rfq"`
	Cartella string    `json:"cartella"`
	Stato    string    `json:"stato"`
	Versione string    `json:"bom"`
	Motivo   string    `json:"motivo"`
}

// ErrConteggio e' una RFQ in cui le righe scritte non sono quelle lette: la sua transazione si annulla.
var ErrConteggio = errors.New("le righe scritte non sono quelle lette")

// RiapriAgganciDellaRfq riapre le forme F-A e F-B di una RFQ, nella transazione di chi chiama (una per RFQ).
//
//  1. BloccaThread: i gesti del server sulla stessa RFQ si mettono in fila, e il comando si puo' lanciare con
//     il server acceso;
//  2. il perimetro si ricontrolla qui dentro: una RFQ chiusa, unita o congelata dopo l'anteprima si salta
//     (esito «saltata», niente scritto);
//  3. F-A e F-B si ricalcolano qui dentro, non si prendono dall'anteprima;
//  4. gli UPDATE, con le righe toccate confrontate con quelle lette: se non tornano, ErrConteggio e chi
//     chiama annulla la transazione di questa RFQ.
//
// La riga di rapporto torna anche con l'errore, per dire che cosa si era letto.
func RiapriAgganciDellaRfq(ctx context.Context, q *db.Queries, thread uuid.UUID) (RfqRiapertura, error) {
	out := RfqRiapertura{ThreadID: thread}
	t, err := q.BloccaThread(ctx, thread)
	if errors.Is(err, pgx.ErrNoRows) {
		out.Esito, out.Motivo = EsitoSaltata, "la RFQ non c'è più"
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Stato = string(t.Stato)
	out.Cartella, out.Oggetto = t.CartellaRelativa.String, t.Oggetto.String
	bloccata, err := q.WorkingBloccataNelDatabase(ctx, thread)
	if err != nil {
		return out, err
	}
	switch {
	case t.Stato != db.StatoThreadAPERTA:
		out.Esito, out.Motivo = EsitoSaltata, "chiusa dopo l'anteprima"
		return out, nil
	case t.UnitoIn.Valid:
		out.Esito, out.Motivo = EsitoSaltata, "unita a un'altra RFQ dopo l'anteprima"
		return out, nil
	case bloccata > 0:
		out.Esito, out.Motivo = EsitoSaltata, fmt.Sprintf("congelata nella V%d dopo l'anteprima: non si tocca", bloccata)
		return out, nil
	}
	dati, err := LeggiRiapertura(ctx, q, thread)
	if err != nil {
		return out, err
	}
	out.Forme = ClassificaFormeLegacy(dati)
	for _, n := range out.Forme.Agganci {
		k, err := q.RiapriAgganciAutomaticiNodo(ctx, db.RiapriAgganciAutomaticiNodoParams{CodiceComponente: n.CodiceComponente,
			PropostaID: n.PropostaID, ThreadID: thread})
		if err != nil {
			return out, err
		}
		out.NodiRiaperti += int(k)
	}
	if out.NodiRiaperti != len(out.Forme.Agganci) {
		return out, fmt.Errorf("%w: nodi riaperti %d su %d letti", ErrConteggio, out.NodiRiaperti, len(out.Forme.Agganci))
	}
	for _, x := range out.Forme.Archi {
		k, err := q.RiapriAgganciAutomaticiArco(ctx, db.RiapriAgganciAutomaticiArcoParams{ThreadID: thread, AllegatoID: x.Allegato,
			PadreChiave: x.Padre, FiglioChiave: x.Figlio})
		if err != nil {
			return out, err
		}
		out.ArchiRiaperti += int(k)
	}
	if out.ArchiRiaperti != len(out.Forme.Archi) {
		return out, fmt.Errorf("%w: archi riaperti %d su %d letti", ErrConteggio, out.ArchiRiaperti, len(out.Forme.Archi))
	}
	out.Esito = EsitoRiaperta
	if !out.Forme.DaRiaprire() {
		out.Esito = EsitoNiente
	}
	return out, nil
}

// ------------------------------------------------------------------ il comando: anteprima e applicazione

// Le modalita' del comando.
const (
	ModalitaAnteprima    = "anteprima"
	ModalitaApplicazione = "applicazione"
)

// TotaliRiapertura sono i conteggi del rapporto, per tutte le RFQ.
type TotaliRiapertura struct {
	NelPerimetro         int `json:"rfq_nel_perimetro"`
	InRevisione          int `json:"rfq_in_revisione"`
	EscluseCongelate     int `json:"rfq_escluse_congelate"`
	EscluseChiuse        int `json:"rfq_escluse_chiuse"`
	EscluseUnite         int `json:"rfq_escluse_unite"`
	Agganci              int `json:"fa_nodi"`
	AgganciNellAutorita  int `json:"fa_nodi_nell_autorita"`
	Archi                int `json:"fb_archi"`
	RadiciTenute         int `json:"fc_radici_tenute"`
	Conflitti            int `json:"fd_conflitti"`
	ChiusureAutomatiche  int `json:"fe_chiusure_automatiche"`
	RadiciAMano          int `json:"ff_radici_a_mano"`
	Preassegnazioni      int `json:"fg_preassegnazioni"`
	CodiciRiscritti      int `json:"fh_codici_riscritti"`
	ConfermatiInBlocco   int `json:"fi_gruppi_confermati_in_blocco"`
	ComponentiDaRivedere int `json:"fj_componenti_da_rivedere"`
	ArchiDaRivedere      int `json:"fk_archi_da_rivedere"`
	Anomalie             int `json:"fl_anomalie"`
	Riaperte             int `json:"rfq_riaperte"`
	Saltate              int `json:"rfq_saltate"`
	Fallite              int `json:"rfq_fallite"`
	NodiRiaperti         int `json:"nodi_riaperti"`
	ArchiRiaperti        int `json:"archi_riaperti"`
}

// RapportoRiapertura e' il rapporto del comando U5: si scrive in JSON man mano (prima di cominciare e dopo
// ogni RFQ) e si stampa alla fine.
type RapportoRiapertura struct {
	Modalita    string           `json:"modalita"`
	SolaLettura bool             `json:"sola_lettura"` // l'anteprima: una sua transazione era davvero in sola lettura
	Inizio      time.Time        `json:"inizio"`
	Fine        *time.Time       `json:"fine,omitempty"`
	Avvisi      []string         `json:"avvisi"`
	Rfq         []RfqRiapertura  `json:"rfq"`
	Escluse     []RfqEsclusa     `json:"escluse"`
	Totali      TotaliRiapertura `json:"totali"`
}

// AvvisiRiapertura sono i prerequisiti che il comando non puo' controllare da se' (A5.15.2): li dice
// l'anteprima e li scrive il rapporto.
var AvvisiRiapertura = []string{
	"sui dati veri si applica solo con lo Smistamento (F10/F11) in produzione, e dopo il backup del database con il DSN del -config verificato",
	"tutti i server che usano questo database devono girare con questo binario (F5): un Pianifica di prima rifarebbe gli agganci per codice alla prossima analisi",
	"qualche giorno dopo, una seconda anteprima deve trovare zero righe da riaprire: è la verifica che nessun server rifà gli agganci",
}

// RiapriAgganci e' il comando U5 su un database: l'anteprima (applica false) legge e classifica ogni RFQ
// del perimetro in una transazione in sola lettura; l'applicazione le riapre, una transazione per RFQ
// (RiapriAgganciDellaRfq), e una RFQ che fallisce si annulla da sola e non ferma le altre. rfq limita il
// comando a una RFQ (nil = tutte). dopo, se c'e', riceve il rapporto una prima volta prima di qualunque RFQ,
// poi dopo ogni RFQ (anche esclusa) e alla fine: chi lo scrive su file lo scrive man mano, un percorso che non
// si puo' scrivere ferma il comando prima di toccare il database, e un'interruzione lascia traccia di cio' che
// e' fatto. Un errore di dopo ferma il comando (le RFQ gia' fatte restano fatte). Torna un errore se anche una sola
// RFQ e' fallita (il codice d'uscita del comando non e' zero), con il rapporto completo.
func RiapriAgganci(ctx context.Context, pool *pgxpool.Pool, rfq *uuid.UUID, applica bool, dopo func(RapportoRiapertura) error) (RapportoRiapertura, error) {
	r := RapportoRiapertura{Modalita: ModalitaAnteprima, Inizio: time.Now(), Avvisi: AvvisiRiapertura,
		Rfq: []RfqRiapertura{}, Escluse: []RfqEsclusa{}}
	if applica {
		r.Modalita = ModalitaApplicazione
	}
	filtro := uuid.NullUUID{}
	if rfq != nil {
		filtro = uuid.NullUUID{UUID: *rfq, Valid: true}
	}
	candidate, err := db.New(pool).ListRfqPerLaRiapertura(ctx, filtro)
	if err != nil {
		return r, fmt.Errorf("le RFQ: %w", err)
	}
	if rfq != nil && len(candidate) == 0 {
		return r, fmt.Errorf("la RFQ %s non c'è in questo database", rfq)
	}
	avanti := func() error {
		if dopo == nil {
			return nil
		}
		return dopo(r)
	}
	if !applica {
		// la sola lettura si misura una volta prima di qualunque RFQ, non alla prima RFQ del perimetro: un
		// database senza RFQ nel perimetro (tutte chiuse o congelate, o -rfq di una esclusa) direbbe altrimenti
		// «sola_lettura=no» di un'anteprima che non poteva scrivere
		if r.SolaLettura, err = inSolaLettura(ctx, pool); err != nil {
			return r, err
		}
	}
	// il rapporto si scrive una prima volta prima di qualunque RFQ: un percorso di -uscita che non si puo'
	// scrivere ferma il comando qui, senza una sola scrittura nel database
	if err := avanti(); err != nil {
		return r, fmt.Errorf("nessuna RFQ toccata: %w", err)
	}
	var fallite []string
	for _, c := range candidate {
		if err := ctx.Err(); err != nil {
			// interrotto (Ctrl+C): le RFQ gia' fatte restano fatte e sono nel rapporto, le altre non si toccano
			return r, fmt.Errorf("interrotto dopo %s: %w", quanti(len(r.Rfq), "RFQ", "RFQ"), err)
		}
		p := PerimetroDi(c)
		if !p.Dentro {
			r.Escluse = append(r.Escluse, RfqEsclusa{ThreadID: c.ThreadID, Cartella: c.Cartella, Stato: string(c.Stato), Versione: p.Versione, Motivo: p.Motivo})
			switch p.Motivo {
			case EsclusaChiusa:
				r.Totali.EscluseChiuse++
			case EsclusaUnita:
				r.Totali.EscluseUnite++
			default:
				r.Totali.EscluseCongelate++
			}
			if err := avanti(); err != nil {
				return r, err
			}
			continue
		}
		var x RfqRiapertura
		if applica {
			x, err = applicaAllaRfq(ctx, pool, c.ThreadID)
		} else {
			x, err = anteprimaDellaRfq(ctx, pool, c.ThreadID)
		}
		// lo stato, la cartella e l'oggetto riletti dentro la transazione, dopo BloccaThread, valgono piu'
		// dell'elenco: una RFQ chiusa dopo l'elenco esce «saltata» con lo stato che ha adesso. Dall'elenco si
		// prende solo quello che manca (l'anteprima, un errore prima di BloccaThread); la versione e la revisione
		// restano quelle del perimetro, e il motivo della saltata dice che cosa e' cambiato.
		x.ThreadID, x.Versione, x.InRevisione = c.ThreadID, p.Versione, p.InRevisione
		if x.Stato == "" {
			x.Stato = string(c.Stato)
		}
		if x.Cartella == "" {
			x.Cartella = c.Cartella
		}
		if x.Oggetto == "" {
			x.Oggetto = c.Oggetto
		}
		if err != nil {
			if !applica {
				// l'anteprima non ha niente da annullare: un errore di lettura ferma il comando
				return r, fmt.Errorf("anteprima della RFQ %s: %w", c.ThreadID, err)
			}
			x.Esito, x.Motivo = EsitoFallita, err.Error()
			x.NodiRiaperti, x.ArchiRiaperti = 0, 0 // annullati con la transazione
			fallite = append(fallite, c.ThreadID.String())
		}
		r.aggiungi(x)
		if err := avanti(); err != nil {
			return r, err
		}
	}
	fine := time.Now()
	r.Fine = &fine
	if err := avanti(); err != nil {
		return r, err
	}
	if len(fallite) > 0 {
		return r, fmt.Errorf("%s non riaperte (annullate, le altre sono fatte): %s", quanti(len(fallite), "RFQ", "RFQ"), strings.Join(fallite, ", "))
	}
	return r, nil
}

// aggiungi mette la RFQ nel rapporto e nei totali.
func (r *RapportoRiapertura) aggiungi(x RfqRiapertura) {
	r.Rfq = append(r.Rfq, x)
	t := &r.Totali
	t.NelPerimetro++
	if x.InRevisione {
		t.InRevisione++
	}
	f := x.Forme
	t.Agganci += len(f.Agganci)
	t.AgganciNellAutorita += f.Effetto.FigliDaConfermare
	t.Archi += len(f.Archi)
	t.RadiciTenute += len(f.RadiciTenute)
	t.Conflitti += len(f.Conflitti)
	t.ChiusureAutomatiche += f.ChiusureAutomatiche.Totale()
	t.RadiciAMano += len(f.RadiciAMano)
	t.Preassegnazioni += len(f.Preassegnazioni)
	t.CodiciRiscritti += len(f.CodiciRiscritti)
	t.ConfermatiInBlocco += len(f.ConfermatiInBlocco)
	t.ComponentiDaRivedere += len(f.ComponentiDaRivedere)
	t.ArchiDaRivedere += len(f.ArchiDaRivedere)
	t.Anomalie += len(f.Anomalie)
	switch x.Esito {
	case EsitoRiaperta:
		t.Riaperte++
	case EsitoSaltata:
		t.Saltate++
	case EsitoFallita:
		t.Fallite++
	}
	t.NodiRiaperti += x.NodiRiaperti
	t.ArchiRiaperti += x.ArchiRiaperti
}

// inSolaLettura apre una transazione come quelle dell'anteprima (ReadOnly) e chiede al database se e' in
// sola lettura, prima di qualunque RFQ. Misura la transazione, non il pool: con AccessMode ReadOnly la risposta
// e' «on» anche su un pool scrivibile, quindi dice che le transazioni dell'anteprima non possono scrivere, non
// che il pool abbia il lucchetto di ApriDatabaseInLettura (default_transaction_read_only), che resta un
// secondo lucchetto del runtime. Se la risposta non e' «on» si ferma.
func inSolaLettura(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	return transazioneInSolaLettura(ctx, tx)
}

// transazioneInSolaLettura legge transaction_read_only nella transazione e torna un errore se non e' «on».
func transazioneInSolaLettura(ctx context.Context, tx pgx.Tx) (bool, error) {
	var ro string
	if err := tx.QueryRow(ctx, "SHOW transaction_read_only").Scan(&ro); err != nil {
		return false, err
	}
	if ro != "on" {
		return false, errors.New("la transazione dell'anteprima non è in sola lettura: non si va avanti")
	}
	return true, nil
}

// anteprimaDellaRfq legge e classifica una RFQ in una transazione in sola lettura (REPEATABLE READ: tutte le
// letture della RFQ vedono lo stesso istante), ricontrollata anche qui: ogni transazione dell'anteprima
// dice al database che non puo' scrivere.
func anteprimaDellaRfq(ctx context.Context, pool *pgxpool.Pool, thread uuid.UUID) (RfqRiapertura, error) {
	out := RfqRiapertura{ThreadID: thread, Esito: EsitoAnteprima}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if _, err := transazioneInSolaLettura(ctx, tx); err != nil {
		return out, err
	}
	dati, err := LeggiRiapertura(ctx, db.New(tx), thread)
	if err != nil {
		return out, err
	}
	out.Forme = ClassificaFormeLegacy(dati)
	return out, nil
}

// applicaAllaRfq riapre una RFQ nella sua transazione: COMMIT se tutto torna, ROLLBACK se no (anche per una
// RFQ saltata, che non ha scritto niente).
func applicaAllaRfq(ctx context.Context, pool *pgxpool.Pool, thread uuid.UUID) (RfqRiapertura, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return RfqRiapertura{ThreadID: thread}, err
	}
	defer tx.Rollback(ctx)
	out, err := RiapriAgganciDellaRfq(ctx, db.New(tx), thread)
	if err != nil || out.Esito == EsitoSaltata {
		return out, err
	}
	if err := tx.Commit(ctx); err != nil {
		return out, fmt.Errorf("commit: %w", err)
	}
	return out, nil
}
