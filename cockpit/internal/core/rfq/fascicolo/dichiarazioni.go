package fascicolo

// Lo STEP autorizzato per un componente (Smistamento F5, addendum A5.4, U3; decisioni dell'utente del 27/09).
//
// Tutti gli STEP si analizzano, e la loro struttura grezza resta in analisi_fatti. Quello che cambia e' che
// cosa rende utilizzabili nella BOM le proposte che ne nascono: non l'uguaglianza fra il codice di un nodo e
// quello di un componente (quella e' un indizio, e si mostra), ma una DECISIONE di una persona: «questo file
// e' autorizzato a proporre i figli diretti di C». Un file autorizzato per C ha autorita' soltanto sugli
// archi C → figli diretti e su quei figli; tutto il resto dello stesso file, e tutti i file non autorizzati,
// sono GUIDA: righe aperte, mai un componente_id, mai un duplicato per codice.
//
// L'autorita' va livello per livello e non si eredita per profondita' (decisioni del 27/09 ter, Domanda 1 =
// B): lo STEP di un sottoassieme, autorizzato per quel sottoassieme, comanda i suoi figli diretti. Il grafo
// profondo pero' non si tronca e non si nasconde: tutti i nodi e tutte le relazioni di ogni STEP restano, e
// si mostrano come guida a qualunque profondita'. Per usare una porzione profonda, una persona autorizza
// esplicitamente lo stesso file anche per il componente annidato: e' la delega (A5.4.6), una marcatura con il
// ruolo «delega» sulla riga di quel nodo, che scrive il gesto di F5b (autorizzazione.go). Una delega vale solo
// se la catena, nel file, arriva a una radice o a un raggruppamento valido dello stesso file: livello per
// livello, e mai da sola (la delega di un nipote vuole quella del figlio).
//
// L'autorizzazione vive, senza schema, nella riga di componente_proposta del nodo sorgente decisa da una
// persona come C, con la marcatura evidenza.strutturale (A5.4.2). Prima dello Smistamento c'era solo lo
// STEP strutturale di un prodotto finito (componente.step_strutturale_id): e' la forma di prima, che vale
// ancora (con le radici del suo file, criterio K1) finche' una persona non la rifa'. Una radice di quella
// forma ancora aperta RESTA aperta: e' sorgente solo qui, nel predicato, e nessuna lettura la decide (K19).
//
// Una marcatura puo' portare una SOSPENSIONE registrata (la chiave «sospesa»): la scrive il cambio del tipo
// del componente a commerciale (fase T), e la toglie solo una persona, con una riattivazione esplicita. Finche'
// c'e', la dichiarazione e' sospesa qualunque sia il tipo di oggi: tornare sottoassieme non la riattiva da
// solo (precisazione dell'utente del 27/09 sera).
//
// Il predicato e' uno solo: ListDichiarazioniRfq legge le due forme, ValutaDichiarazioni (pura) dice quali
// valgono. Pianifica, le accettazioni, le rimozioni, l'editor, la BOM visuale e il gate lo leggono da qui.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// Le origini di una dichiarazione.
const (
	OrigineSmistamento     = "smistamento"         // la marcatura evidenza.strutturale
	OrigineStepStrutturale = "step_strutturale_id" // la forma di prima dello Smistamento
)

// I ruoli di una sorgente (A5.4.2, A5.4.6).
const (
	RuoloRadice         = "radice"
	RuoloRaggruppamento = "raggruppamento"
	// RuoloDelega: il nodo annidato A nel file autorizzato per il suo padre, autorizzato esplicitamente da una
	// persona anche per A (Domanda 1 = B). Il documento e' quello del file, cioe' del padre: la delega vale
	// solo finche' lo stesso file e' autorizzato, valido, per il componente di quel documento.
	RuoloDelega = "delega"
)

// Marcatura e' evidenza.strutturale: chi ha autorizzato il file, per quale componente, con che ruolo.
type Marcatura struct {
	V            int           `json:"v"`
	Ruolo        string        `json:"ruolo"`
	ComponenteID uuid.NullUUID `json:"componente_id"`
	DocumentoID  uuid.NullUUID `json:"documento_id"`
	DichiaratoDa uuid.NullUUID `json:"dichiarato_da"`
	DichiaratoIl string        `json:"dichiarato_il,omitempty"`
	PresaDAtto   string        `json:"presa_d_atto,omitempty"`
	// Sospesa: la sospensione registrata. Presente, la dichiarazione non vale qualunque sia il tipo di oggi;
	// si toglie solo con una riattivazione esplicita di una persona.
	Sospesa *Sospensione `json:"sospesa,omitempty"`
}

// Sospensione e' evidenza.strutturale.sospesa: perche' l'autorizzazione e' ferma, per quale tipo, chi e quando.
// La forma e' questa: {"motivo": "…", "tipo": "commerciale", "da": "<utente>", "il": "2026-…"}.
type Sospensione struct {
	Motivo string        `json:"motivo"`
	Tipo   string        `json:"tipo,omitempty"` // il tipo del componente che l'ha causata (commerciale)
	Da     uuid.NullUUID `json:"da"`
	Il     string        `json:"il,omitempty"`
}

// Dichiarazione e' un file autorizzato a proporre i figli diretti di un componente (U3).
type Dichiarazione struct {
	Componente db.Componente
	Sha256     string
	NomeFile   string
	Allegato   uuid.UUID     // il portatore delle righe del file
	Documento  uuid.NullUUID // il documento del file nella RFQ
	// Sorgenti: chiave del nodo → ruolo (radice, raggruppamento, delega). Di solito una, la radice.
	Sorgenti     map[string]string
	Origine      string        // OrigineSmistamento | OrigineStepStrutturale
	DichiaratoDa uuid.NullUUID // NULL per la forma di prima: chi e quando non sono registrati
	// Sospensione: la sospensione registrata nella marcatura, se c'e'.
	Sospensione *Sospensione
	// Problema: "" = valida; altrimenti perche' non vale (conflitto, superata, incoerente, P7, sospesa).
	Problema string
	titolare uuid.NullUUID       // il componente del documento del file: per una delega, il padre
	padri    map[string][]string // chiave di una sorgente → i suoi padri nel file (la catena di una delega)
}

// Valida dice se la dichiarazione da' autorita' al file.
func (d Dichiarazione) Valida() bool { return d.Problema == "" }

// Delega dice se la dichiarazione e' una delega: lo stesso file del padre, autorizzato anche per C.
func (d Dichiarazione) Delega() bool {
	if len(d.Sorgenti) == 0 {
		return false
	}
	for _, r := range d.Sorgenti {
		if r != RuoloDelega {
			return false
		}
	}
	return true
}

// Frase dice la dichiarazione in parole, per il gate e per le risposte: «7120010 · 7120010.stp: …».
func (d Dichiarazione) Frase() string {
	s := d.Componente.Codice + " · STEP autorizzato " + d.NomeFile
	if d.Delega() {
		s += " (delega)"
	}
	if d.Problema != "" {
		s += ": " + d.Problema
	}
	return s
}

// Dichiarazioni sono le autorizzazioni della RFQ, gia' valutate.
type Dichiarazioni struct {
	PerComponente map[uuid.UUID]Dichiarazione // la sola valida di ogni componente
	PerSha        map[string][]Dichiarazione  // le valide di ogni file: un file puo' avere piu' sorgenti
	// DaSistemare: conflitto, superata, incoerente, P7, la delega con la catena rotta. Fermano il gate (L3).
	DaSistemare []Dichiarazione
	// Sospese: del componente archiviato (torna valida con il ripristino), del commerciale, con una sospensione
	// registrata (si riattiva solo con una scelta esplicita di una persona), la delega rimasta senza catena
	// perche' sopra di lei qualcuno e' sospeso. Non fermano il gate: sono un avviso.
	Sospese []Dichiarazione
	tutte   map[uuid.UUID][]Dichiarazione // di ogni componente, valide o no
}

// Sorgenti sono i nodi sorgente validi di un file: chiave → componente.
func (d Dichiarazioni) Sorgenti(sha string) map[string]uuid.UUID {
	out := map[string]uuid.UUID{}
	for _, x := range d.PerSha[sha] {
		for k := range x.Sorgenti {
			out[k] = x.Componente.ComponenteID
		}
	}
	return out
}

// Autorizzato dice se l'arco padre → figlio del file e' nell'autorita': il padre e' una sorgente valida.
func (d Dichiarazioni) Autorizzato(sha, padreChiave string) bool {
	_, ok := d.Sorgenti(sha)[padreChiave]
	return ok
}

// Di e' la dichiarazione valida del componente, se c'e'.
func (d Dichiarazioni) Di(componente uuid.UUID) (Dichiarazione, bool) {
	x, ok := d.PerComponente[componente]
	return x, ok
}

// DelComponente sono tutte le dichiarazioni del componente, valide o no.
func (d Dichiarazioni) DelComponente(componente uuid.UUID) []Dichiarazione {
	return d.tutte[componente]
}

// Valide sono le dichiarazioni valide, in ordine di codice del componente.
func (d Dichiarazioni) Valide() []Dichiarazione {
	out := make([]Dichiarazione, 0, len(d.PerComponente))
	for _, x := range d.PerComponente {
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Componente.Codice != out[j].Componente.Codice {
			return out[i].Componente.Codice < out[j].Componente.Codice
		}
		return out[i].Sha256 < out[j].Sha256
	})
	return out
}

// RimozioneValida dice se una rimozione proposta (il documento dello STEP, il padre) viene da una
// dichiarazione valida. Le rimozioni di un'autorizzazione che non vale (sospesa, in conflitto, superata)
// restano scritte ma non si decidono e non fermano il gate: il file non ha autorita' (A5.4.6, A5.4.8).
func (d Dichiarazioni) RimozioneValida(documento, padre uuid.UUID) bool {
	x, ok := d.PerComponente[padre]
	return ok && x.Documento.Valid && x.Documento.UUID == documento
}

// PerIlGate sono le chiavi che GateStrutturale vuole: le sorgenti valide (sha seguito dalla chiave del nodo:
// lo sha ha sempre 64 caratteri) e le rimozioni valide (documento seguito dal padre: due uuid di lunghezza
// fissa). Ordinate, perche' la stessa base dia la stessa query.
func (d Dichiarazioni) PerIlGate() (sorgenti, rimozioni []string) {
	sorgenti, rimozioni = []string{}, []string{}
	for _, x := range d.Valide() {
		for k := range x.Sorgenti {
			sorgenti = append(sorgenti, x.Sha256+k)
		}
		if x.Documento.Valid {
			rimozioni = append(rimozioni, x.Documento.UUID.String()+x.Componente.ComponenteID.String())
		}
	}
	sort.Strings(sorgenti)
	sort.Strings(rimozioni)
	return sorgenti, rimozioni
}

// DelDocumento dice se il documento e' quello di un file autorizzato (valido o da sistemare, non sospeso):
// un documento cosi' non lascia il suo componente finche' l'autorizzazione non si revoca. Una delega non
// conta: il documento e' del padre, e lo tiene la dichiarazione del padre.
func (d Dichiarazioni) DelDocumento(documento uuid.UUID) (Dichiarazione, bool) {
	for _, xx := range d.tutte {
		for _, x := range xx {
			if x.Documento.Valid && x.Documento.UUID == documento && !x.Sospesa() && !x.Delega() {
				return x, true
			}
		}
	}
	return Dichiarazione{}, false
}

// Sospesa dice se la dichiarazione e' ferma: il componente e' archiviato o commerciale, c'e' una sospensione
// registrata, o e' una delega rimasta senza catena perche' sopra di lei qualcuno e' sospeso.
func (d Dichiarazione) Sospesa() bool { return strings.HasPrefix(d.Problema, "sospesa") }

// problemaCatena e' il problema di una delega che non arriva, nel file, a una radice o a un raggruppamento
// valido dello stesso file: il padre non e' piu' autorizzato, o la delega del livello sopra non c'e'. La revoca
// di un'autorizzazione revoca le deleghe che restano cosi' (autorizzazione.go), e le riconosce da qui.
//
// Ha due forme (Smistamento G, A5.4.8, L3). Sospesa, quando a togliere la catena e' una sospensione sopra la
// delega (il padre o il titolare del file diventati commerciali, archiviati, con la sospensione registrata): la
// delega aspetta con loro, ed e' un avviso come loro. Rotta, in ogni altro caso (chi la tiene e' in conflitto,
// superato, incoerente, oppure la sua riga non c'e' piu' senza che un gesto abbia revocato anche la delega):
// la delega non vale e nessuno l'ha tolta, quindi e' un'autorizzazione da sistemare e ferma il gate.
const (
	problemaCatena      = "sospesa: la delega vale solo con la catena del file fino a una radice o a un raggruppamento autorizzati, che non c'è più"
	problemaCatenaRotta = "catena rotta: la delega vale solo con la catena del file fino a una radice o a un raggruppamento autorizzati, che non vale più: si sistema l'autorizzazione sopra, o si revoca la delega"
)

// problemaTitolare e' il problema di una delega il cui file non vale per il componente del suo documento (il
// titolare): la delega vale solo con lui. Le due forme sono quelle di problemaCatena.
const (
	problemaTitolare      = "sospesa: la delega vale con l'autorizzazione dello stesso file per il suo componente, che non vale"
	problemaTitolareRotto = "catena rotta: la delega vale con l'autorizzazione dello stesso file per il suo componente, che non vale: si sistema quella, o si revoca la delega"
)

// SenzaCatena dice se la dichiarazione e' una delega rimasta senza la catena che la teneva (sospesa o rotta).
func (d Dichiarazione) SenzaCatena() bool {
	return d.Problema == problemaCatena || d.Problema == problemaCatenaRotta
}

// SenzaTitolare dice se la dichiarazione e' una delega il cui file non vale per il suo titolare (sospesa o
// rotta).
func (d Dichiarazione) SenzaTitolare() bool {
	return d.Problema == problemaTitolare || d.Problema == problemaTitolareRotto
}

// sospensioneVera dice se la dichiarazione e' ferma per una sospensione sua (registrata, componente archiviato
// o commerciale), e non perche' le manca la catena.
func (d Dichiarazione) sospensioneVera() bool {
	return d.Sospesa() && !(d.Delega() && (d.SenzaCatena() || d.SenzaTitolare()))
}

// ChiaviSorgenti sono le chiavi dei nodi sorgente, in ordine.
func (d Dichiarazione) ChiaviSorgenti() []string {
	out := make([]string, 0, len(d.Sorgenti))
	for k := range d.Sorgenti {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// LeggiDichiarazioni legge e valuta le autorizzazioni della RFQ. Non scrive niente.
func LeggiDichiarazioni(ctx context.Context, q *db.Queries, thread uuid.UUID) (Dichiarazioni, error) {
	righe, err := q.ListDichiarazioniRfq(ctx, thread)
	if err != nil {
		return Dichiarazioni{}, fmt.Errorf("autorizzazioni della RFQ: %w", err)
	}
	return ValutaDichiarazioni(righe), nil
}

// ValutaDichiarazioni dice quali autorizzazioni valgono (A5.4.3). Pura. Le regole:
//   - una sospensione registrata nella marcatura: «sospesa» qualunque sia il tipo di oggi; si riattiva solo
//     con una scelta esplicita di una persona (precisazione dell'utente del 27/09 sera);
//   - il componente non e' archiviato (archiviato: «sospesa», torna valida con il ripristino);
//   - il componente non e' commerciale (decisioni del 27/09 ter, Domanda 5 = B: «sospesa», il suo STEP
//     resta guida). Non torna valida da sola quando il tipo cambia: la fase T, al passaggio a commerciale,
//     registra la sospensione nella marcatura;
//   - il documento del file esiste nella RFQ, e' corrente ed e' associato a C (per una delega: al padre,
//     con lo stesso file autorizzato e valido per il padre); sostituito: «superata»;
//   - per un finito, la marcatura e lo STEP strutturale dicono lo stesso file (I10); se no «incoerente»;
//   - la forma di prima: la radice e' decisa come C, oppure e' ancora aperta (e resta aperta); decisa come
//     un altro componente e' il conflitto P7; con piu' radici, conta quella decisa come C da una persona;
//   - una marcatura vale su una riga decisa da una persona, con il ruolo radice, raggruppamento o delega;
//   - un componente, un'autorizzazione: due file validi per C sono un conflitto, e nessuno dei due vale
//     (anche una delega e lo STEP proprio dello stesso componente: il gesto revoca l'una quando scrive
//     l'altra, e il conflitto nasce solo da scritture fuori dal gesto);
//   - una delega vale solo se lo stesso file e' autorizzato, valido, per il componente del suo documento, e
//     se il suo nodo e' figlio diretto, nel file, di una sorgente valida dello stesso file, fino a una radice o
//     a un raggruppamento (Domanda 1 = B: livello per livello, mai da sola). Senza, e' sospesa se sopra di
//     lei c'e' una sospensione (aspetta con chi la tiene), altrimenti la catena e' rotta: da sistemare, e il
//     gate si ferma (Smistamento G).
//
// La marcatura prevale sulla forma di prima per lo stesso file e lo stesso componente: e' la stessa
// dichiarazione, rifatta da una persona.
func ValutaDichiarazioni(righe []db.ListDichiarazioniRfqRow) Dichiarazioni {
	type chiave struct {
		comp    uuid.UUID
		sha     string
		origine string
	}
	gruppi := map[chiave][]db.ListDichiarazioniRfqRow{}
	var ordine []chiave
	marcati := map[[2]string]bool{} // (componente, sha) con una marcatura
	for _, r := range righe {
		k := chiave{r.Componente.ComponenteID, r.Sha256, r.Origine}
		if _, ok := gruppi[k]; !ok {
			ordine = append(ordine, k)
		}
		gruppi[k] = append(gruppi[k], r)
		if r.Origine == OrigineSmistamento {
			marcati[[2]string{r.Componente.ComponenteID.String(), r.Sha256}] = true
		}
	}
	var candidate []Dichiarazione
	for _, k := range ordine {
		if k.origine == OrigineStepStrutturale && marcati[[2]string{k.comp.String(), k.sha}] {
			continue
		}
		candidate = append(candidate, valutaGruppo(gruppi[k]))
	}
	// prima i file autorizzati per il loro componente, poi le deleghe, che dipendono da quelli: una delega
	// vale solo se lo stesso file e' autorizzato, valido, per il componente del suo documento, e se la catena
	// del file la raggiunge. Una delega in conflitto puo' togliere validita' al file proprio di un
	// sottoassieme, e con lui alle sue deleghe: si ripete finche' niente cambia (ogni giro puo' solo togliere
	// validita', quindi finisce).
	conflitti(candidate, false)
	for cambiato := true; cambiato; {
		cambiato = false
		for i := range candidate {
			x := &candidate[i]
			if !x.Delega() || !x.Valida() || titolareValido(candidate, *x) {
				continue
			}
			x.Problema = problemaTitolare
			cambiato = true
		}
		if senzaCatena(candidate) {
			cambiato = true
		}
		if conflitti(candidate, true) {
			cambiato = true
		}
	}
	// una delega senza catena e' sospesa solo se a toglierle la catena e' una sospensione sopra di lei; se no la
	// catena e' rotta, e va sistemata (Smistamento G, A5.4.8, L3). Si guarda dopo il giro e su tutte insieme: le
	// due forme risalgono allo stesso modo, quindi l'ordine non conta
	var rotte []int
	for i, x := range candidate {
		if x.Delega() && (x.SenzaCatena() || x.SenzaTitolare()) && !sospesaPerChiLaTiene(candidate, x) {
			rotte = append(rotte, i)
		}
	}
	for _, i := range rotte {
		if candidate[i].SenzaCatena() {
			candidate[i].Problema = problemaCatenaRotta
		} else {
			candidate[i].Problema = problemaTitolareRotto
		}
	}
	out := Dichiarazioni{PerComponente: map[uuid.UUID]Dichiarazione{}, PerSha: map[string][]Dichiarazione{},
		tutte: map[uuid.UUID][]Dichiarazione{}}
	for _, d := range candidate {
		c := d.Componente.ComponenteID
		out.tutte[c] = append(out.tutte[c], d)
		switch {
		case d.Valida():
			out.PerComponente[c] = d
			out.PerSha[d.Sha256] = append(out.PerSha[d.Sha256], d)
		case d.Sospesa():
			out.Sospese = append(out.Sospese, d)
		default:
			out.DaSistemare = append(out.DaSistemare, d)
		}
	}
	return out
}

// conflitti applica «un componente, un'autorizzazione» alle dichiarazioni valide (le deleghe solo con
// conDeleghe): due o piu' per lo stesso componente sono tutte un conflitto. Dice se ne ha trovato uno.
func conflitti(candidate []Dichiarazione, conDeleghe bool) bool {
	valide := map[uuid.UUID][]int{}
	var ordine []uuid.UUID
	for i, d := range candidate {
		if !d.Valida() || (!conDeleghe && d.Delega()) {
			continue
		}
		c := d.Componente.ComponenteID
		if _, ok := valide[c]; !ok {
			ordine = append(ordine, c)
		}
		valide[c] = append(valide[c], i)
	}
	trovato := false
	for _, c := range ordine {
		ii := valide[c]
		if len(ii) < 2 {
			continue
		}
		var file []string
		for _, i := range ii {
			f := candidate[i].NomeFile
			if candidate[i].Delega() {
				f += " (delega)"
			}
			file = append(file, f)
		}
		sort.Strings(file)
		for _, i := range ii {
			candidate[i].Problema = fmt.Sprintf("conflitto: %s ha %d STEP autorizzati (%s), si revoca quello che non vale",
				candidate[i].Componente.Codice, len(ii), strings.Join(file, ", "))
		}
		trovato = true
	}
	return trovato
}

// senzaCatena toglie validita' alle deleghe che la catena del loro file non raggiunge: si parte dalle sorgenti
// valide non delegate di ogni file (radici e raggruppamenti) e si scende una delega alla volta, solo verso un
// nodo che nel file e' figlio diretto di una sorgente gia' raggiunta. Una delega di un nipote senza quella del
// figlio, o due deleghe che si tengono l'una con l'altra senza arrivare a una radice, restano fuori. Dice se ne
// ha tolta una.
func senzaCatena(candidate []Dichiarazione) bool {
	raggiunti := map[string]map[string]bool{} // sha → chiavi delle sorgenti raggiunte
	aggiungi := func(x Dichiarazione) {
		if raggiunti[x.Sha256] == nil {
			raggiunti[x.Sha256] = map[string]bool{}
		}
		for k := range x.Sorgenti {
			raggiunti[x.Sha256][k] = true
		}
	}
	for _, x := range candidate {
		if x.Valida() && !x.Delega() {
			aggiungi(x)
		}
	}
	appesa := map[int]bool{}
	for progresso := true; progresso; {
		progresso = false
		for i, x := range candidate {
			if appesa[i] || !x.Valida() || !x.Delega() {
				continue
			}
			for k := range x.Sorgenti {
				for _, p := range x.padri[k] {
					if raggiunti[x.Sha256][p] {
						appesa[i] = true
					}
				}
			}
			if appesa[i] {
				aggiungi(x)
				progresso = true
			}
		}
	}
	tolta := false
	for i := range candidate {
		if candidate[i].Valida() && candidate[i].Delega() && !appesa[i] {
			candidate[i].Problema = problemaCatena
			tolta = true
		}
	}
	return tolta
}

// sospesaPerChiLaTiene dice se la delega x, senza catena, e' ferma per una sospensione di chi la tiene. Sale nel
// file dai suoi nodi sorgente, livello per livello, fino alle sorgenti delle altre dichiarazioni dello stesso
// file: una sospesa davvero (registrata, archiviata, commerciale) e' la causa; una delega a sua volta senza
// catena fa salire ancora; una da sistemare non e' una sospensione. Guarda anche il titolare del file (il
// componente del suo documento). E' la risalita di sospesaSopra (tipo.go), che pero' cerca solo le sospensioni
// registrate, per il consiglio della riattivazione. Pura.
func sospesaPerChiLaTiene(candidate []Dichiarazione, x Dichiarazione) bool {
	perChiave := map[string]Dichiarazione{}
	for _, d := range candidate {
		if d.Sha256 != x.Sha256 || d.Componente.ComponenteID == x.Componente.ComponenteID {
			continue
		}
		for k := range d.Sorgenti {
			perChiave[k] = d
		}
		if !d.Delega() && x.titolare.Valid && d.Componente.ComponenteID == x.titolare.UUID && d.sospensioneVera() {
			return true
		}
	}
	var livello []string
	for k := range x.Sorgenti {
		livello = append(livello, x.padri[k]...)
	}
	visti := map[string]bool{}
	for len(livello) > 0 {
		var sopra []string
		for _, k := range livello {
			if visti[k] {
				continue
			}
			visti[k] = true
			d, ok := perChiave[k]
			switch {
			case !ok || d.Valida():
			case d.sospensioneVera():
				return true
			case d.Delega() && (d.SenzaCatena() || d.SenzaTitolare()):
				sopra = append(sopra, d.padri[k]...)
			}
		}
		livello = sopra
	}
	return false
}

// titolareValido dice se lo stesso file della delega e' autorizzato, valido e non per delega, per il componente
// del suo documento (il padre).
func titolareValido(candidate []Dichiarazione, delega Dichiarazione) bool {
	for _, y := range candidate {
		if y.Valida() && !y.Delega() && y.Sha256 == delega.Sha256 && delega.titolare.Valid && y.Componente.ComponenteID == delega.titolare.UUID {
			return true
		}
	}
	return false
}

// valutaGruppo valuta le righe di un componente, di un file e di una forma.
func valutaGruppo(righe []db.ListDichiarazioniRfqRow) Dichiarazione {
	r0 := righe[0]
	c := r0.Componente
	d := Dichiarazione{Componente: c, Sha256: r0.Sha256, NomeFile: r0.NomeFile, Allegato: r0.AllegatoID, Origine: r0.Origine,
		Sorgenti: map[string]string{}, titolare: r0.DocumentoComponenteID, padri: map[string][]string{}}
	if r0.DocumentoID.Valid {
		d.Documento = r0.DocumentoID
	}
	// lo stesso contenuto arrivato due volte ha un portatore solo (PortatoreDelFile): le righe che contano
	// sono quelle del primo allegato, come per la lettura
	var mie []db.ListDichiarazioniRfqRow
	for _, r := range righe {
		if r.AllegatoID == r0.AllegatoID {
			mie = append(mie, r)
			d.padri[r.Chiave] = r.Padri
		}
	}
	if d.Origine == OrigineSmistamento {
		d.Problema = sorgentiMarcate(&d, mie)
	} else {
		d.Problema = sorgentiDiPrima(&d, mie)
	}
	if p := problemaComune(d, r0); p != "" {
		d.Problema = p
	}
	return d
}

// problemaComune sono le regole di tutte e due le forme: sospensione, componente, documento, coerenza del
// finito. Vengono prima dei problemi delle righe: un file superato o di un componente archiviato non si rifa'
// riga per riga.
func problemaComune(d Dichiarazione, r db.ListDichiarazioniRfqRow) string {
	c := d.Componente
	delega := d.Delega()
	switch {
	case d.Sospensione != nil:
		// precisazione dell'utente del 27/09 sera: registrata, la sospensione tiene qualunque sia il tipo di
		// oggi. Tornare sottoassieme non la riattiva: serve una scelta esplicita di una persona
		motivo := strings.TrimSpace(d.Sospensione.Motivo)
		if motivo == "" {
			motivo = "l'autorizzazione è sospesa"
		}
		return "sospesa: " + c.Codice + ": " + motivo + " (si riattiva solo con una scelta esplicita)"
	case c.ArchiviatoIl != nil:
		return "sospesa: " + c.Codice + " è archiviato (torna valida con il ripristino)"
	case c.Tipo == db.TipoComponenteCommerciale:
		// decisioni del 27/09 ter, Domanda 5 = B: lo STEP di un pezzo comprato e' guida, mai una sorgente.
		// Sospesa e non da sistemare: e' un avviso, la marcatura resta, e non ferma il gate
		return "sospesa: " + c.Codice + " è commerciale, il suo STEP resta guida (si riattiva solo con una scelta esplicita, dopo il cambio di tipo)"
	case ruoliMisti(d):
		return "incoerente: lo stesso file è autorizzato e delegato per " + c.Codice + ", l'autorizzazione è da rifare"
	case !r.DocumentoID.Valid:
		return "incoerente: il file non ha un documento nella RFQ, l'autorizzazione è da rifare"
	case delega && (!r.DocumentoComponenteID.Valid || r.DocumentoComponenteID.UUID == c.ComponenteID):
		// la delega dice il file del padre: se il file e' di C, e' un'autorizzazione, non una delega
		return "incoerente: la delega di " + c.Codice + " non dice il file di un padre, l'autorizzazione è da rifare"
	case !delega && (!r.DocumentoComponenteID.Valid || r.DocumentoComponenteID.UUID != c.ComponenteID):
		return "incoerente: il documento del file non è di " + c.Codice + ", l'autorizzazione è da rifare"
	case r.DocumentoSostituitoDa.Valid:
		return "superata: il file è stato sostituito, si risponde alla sostituzione"
	case !delega && d.Origine == OrigineSmistamento && c.Tipo == db.TipoComponenteFinito && (!r.StepSha256.Valid || r.StepSha256.String != d.Sha256):
		return "incoerente: l'autorizzazione e lo STEP strutturale di " + c.Codice + " non dicono lo stesso file, è da rifare"
	}
	return ""
}

// sorgentiMarcate legge le sorgenti della forma dello Smistamento: le righe marcate, decise da una persona.
// Legge anche la sospensione registrata, se una delle marcature la porta.
func sorgentiMarcate(d *Dichiarazione, righe []db.ListDichiarazioniRfqRow) string {
	for _, r := range righe {
		var m Marcatura
		if len(r.Marcatura) == 0 || json.Unmarshal(r.Marcatura, &m) != nil {
			return "incoerente: la marcatura del nodo " + r.Chiave + " non si legge, l'autorizzazione è da rifare"
		}
		if m.Sospesa != nil && d.Sospensione == nil {
			d.Sospensione = m.Sospesa
		}
		switch {
		case !r.DecisoDa.Valid:
			return "incoerente: il nodo " + r.Chiave + " è marcato ma nessuna persona l'ha deciso, l'autorizzazione è da rifare"
		case !r.RigaComponenteID.Valid || r.RigaComponenteID.UUID != d.Componente.ComponenteID ||
			(m.ComponenteID.Valid && m.ComponenteID.UUID != d.Componente.ComponenteID):
			return "incoerente: la marcatura del nodo " + r.Chiave + " dice un altro componente, l'autorizzazione è da rifare"
		case m.Ruolo != RuoloRadice && m.Ruolo != RuoloRaggruppamento && m.Ruolo != RuoloDelega:
			return fmt.Sprintf("incoerente: il nodo %s ha il ruolo «%s», che non si usa, l'autorizzazione è da rifare", r.Chiave, m.Ruolo)
		}
		d.Sorgenti[r.Chiave] = m.Ruolo
		if !d.DichiaratoDa.Valid {
			d.DichiaratoDa = m.DichiaratoDa
			if !d.DichiaratoDa.Valid {
				d.DichiaratoDa = r.DecisoDa
			}
		}
	}
	return ""
}

// ruoliMisti dice se le sorgenti mescolano la delega con la radice o il raggruppamento: una delega e' il file
// del padre, una radice o un raggruppamento il file di C, e insieme non dicono un file solo.
func ruoliMisti(d Dichiarazione) bool {
	deleghe := 0
	for _, ruolo := range d.Sorgenti {
		if ruolo == RuoloDelega {
			deleghe++
		}
	}
	return deleghe > 0 && deleghe < len(d.Sorgenti)
}

// sorgentiDiPrima legge le sorgenti della forma di prima: le radici del file dello STEP strutturale (K1).
// Una radice aperta vale e resta aperta; decisa come C vale; decisa come un altro componente e' P7. Con
// piu' radici nessuna e' C per una lettura: vale quella che una persona ha deciso come C.
func sorgentiDiPrima(d *Dichiarazione, radici []db.ListDichiarazioniRfqRow) string {
	c := d.Componente
	if len(radici) > 1 {
		for _, r := range radici {
			if r.DecisoDa.Valid && decisaCome(r, c.ComponenteID) {
				d.Sorgenti[r.Chiave] = RuoloRadice
			}
		}
		if len(d.Sorgenti) == 0 {
			return fmt.Sprintf("incoerente: lo STEP strutturale di %s ha %d radici e nessuna è decisa come %s, l'autorizzazione è da rifare",
				c.Codice, len(radici), c.Codice)
		}
		return ""
	}
	r := radici[0]
	switch {
	case r.Stato == db.StatoPropostaAperta, decisaCome(r, c.ComponenteID):
		d.Sorgenti[r.Chiave] = RuoloRadice
		return ""
	case r.Stato == db.StatoPropostaScartata:
		return "incoerente: la radice dello STEP strutturale di " + c.Codice + " è scartata, l'autorizzazione è da rifare"
	}
	return "conflitto P7: la radice dello STEP strutturale di " + c.Codice + " è già un altro componente: si archivia quello o si sceglie un altro file"
}

func decisaCome(r db.ListDichiarazioniRfqRow, comp uuid.UUID) bool {
	return (r.Stato == db.StatoPropostaConfermata || r.Stato == db.StatoPropostaDuplicato) &&
		r.RigaComponenteID.Valid && r.RigaComponenteID.UUID == comp
}
