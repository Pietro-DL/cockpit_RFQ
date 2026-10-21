package distintabozza

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// FormatoSnapshot e' il formato della bozza salvata: il formato 2 del contratto (rev. 3 §3). Il formato 1 del giro 4
// resta sulle sue rotte e non si salva qui.
const FormatoSnapshot = 2

// I limiti della forma (contratto rev. 3 §3.2, §6.2). Le colonne del database sono uguali o piu' larghe: un valore che
// passa di qui non rompe una INSERT, e uno che non passa si rifiuta con il suo campo, non si tronca. I CHECK sul peso
// del jsonb (4 MiB lo snapshot, 256 KiB prima e dopo di un evento) li misura il database: se scattano, Salva li
// traduce in CORPO_NON_VALIDO con il campo (RC-07).
const (
	// MaxSnapshot: lo snapshot e' al piu' 2 MiB di testo; il corpo intero del salvataggio, in B2, al piu' 2 MiB + 4 KiB.
	MaxSnapshot = 2 << 20
	// MaxVoci: quante voci puo' avere ogni elenco dello snapshot (nodi, archi, decisioni, preassociazioni, caselle e
	// righe di un nodo). E' MaxVociBozza del formato 1 (core/rfq/fascicolo; la prova L1 lo confronta).
	MaxVoci = 5000
	// MaxQta: la quantita' di un arco, da 1 a MaxQtaArco di core/rfq/fascicolo (la prova L1 lo confronta).
	MaxQta = 100000
	// MaxCodiceRegistrato: caratteri del codice di un componente registrato (colonna componente.codice varchar(60)).
	// I codici nuovi (crea, o un cambio di codice) seguono invece classificazione.CodiceAmmissibile, al piu' 40 (Δ31).
	MaxCodiceRegistrato = 60
	// MaxDescrizione: caratteri della descrizione (colonna componente.descrizione varchar(200)).
	MaxDescrizione = 200
	// MaxPosizione: caratteri della posizione di un arco (colonna componente_relazione.posizione varchar(20)).
	MaxPosizione = 20
	// MaxGruppo: caratteri del gruppo opaco di una fonte o di una casella (CHECK di distinta_bozza.fonte_gruppo).
	MaxGruppo = 300
	// MaxRif: caratteri di un riferimento opaco a un nodo STEP (nodo:<sha256>:<chiave PRODUCT>) [T].
	MaxRif = 400
	// MaxMotivo: caratteri di un motivo (CHECK di distinta_bozza_evento.motivo e distinta_bozza.motivo_chiusura).
	MaxMotivo = 1000
	// MaxCifre: cifre di un numero intero dentro lo snapshot o una decisione [T]. jsonb rifiuta un numero oltre 131 072
	// cifre (22003), e nessun valore del contratto ne ha bisogno di piu' di qualche decina: oltre, CORPO_NON_VALIDO con
	// il campo, non un errore del database (revisione «scrittori» della notte, RSC-10).
	MaxCifre = 1000
)

var (
	reChiave = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`) // = CHECK ck_dbe_chiave e ck_csc_chiave della 0022
	reSha256 = regexp.MustCompile(`^[0-9a-f]{64}$`)
	reRif    = regexp.MustCompile(`^nodo:[0-9a-f]{64}:.+$`)
	// reNomeCampo: i nomi dei campi di ogni oggetto del contratto sono in minuscolo (snake_case). encoding/json associa
	// le chiavi ai campi senza guardare le maiuscole: «NODI» o «Formato» passerebbero (RC-12).
	reNomeCampo = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	// reIntero: i numeri della forma canonica sono interi senza esponente (contratto rev. 3 §6.3). jsonb riscrive
	// 1e2 in 100 e -0 in 0: un numero non intero cambierebbe l'impronta alla rilettura (RC-01).
	reIntero = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
)

// Le operazioni su un nodo (contratto rev. 3 §3.2, nodi[].op).
const (
	OpCrea     = "crea"
	OpRiusa    = "riusa"
	OpModifica = "modifica"
	OpEsclude  = "esclude"
)

// Le provenienze di un nodo o di un arco (contratto rev. 3 §3.2, origine; R-21).
const (
	OrigineStep       = "step"
	OrigineUtente     = "utente"
	OrigineConferma   = "conferma"
	OrigineRegistrato = "registrato"
)

var origini = map[string]bool{OrigineStep: true, OrigineUtente: true, OrigineConferma: true, OrigineRegistrato: true}

// I 13 tipi letterali delle decisioni (contratto rev. 3 §3.3; matrice 07 §2). La forma dei loro campi la controlla B2
// (Emendamento 1, E1.5); qui id, tipo, numeri interi, nessun U+0000, chiavi in minuscolo.
var tipiDecisione = map[string]bool{
	"d1_a_particolare": true, "d1_resta_sottoassieme": true, "d1_a_sottoassieme": true, "mono_pezzo": true,
	"togli_arco": true, "escludi": true, "senza_padre": true, "riusa_esistente": true, "riusa_per_canonico": true,
	"ricodifica": true, "arco_condiviso": true, "da_valutare": true, "cambia_tipo": true,
}

// Snapshot e' la bozza formato 2 di un prodotto, come si salva e come si rilegge: lo stato desiderato completo
// del perimetro (nodi e archi), le decisioni, le preassociazioni, la fonte e la base.
//
// La forma canonica (contratto rev. 3 §6.3) e' quella di jsoncanonico: chiavi ordinate, niente spazi, numeri interi,
// stringhe UTF-8 in NFC; elenchi ordinati (nodi per chiave, archi per padre e figlio, decisioni per id,
// preassociazioni per proposta, caselle per gruppo e rif, righe in ordine lessicografico); i campi facoltativi di un
// nodo o di un arco vuoti, assenti o null sono la stessa cosa e non si scrivono (componente_id, codice, rev,
// descrizione, caselle, righe_proposta, posizione). I quattro elenchi di testa (nodi, archi, decisioni,
// documenti_preassociati) si scrivono sempre, anche vuoti: sono la forma del documento [T].
type Snapshot struct {
	Formato               int                `json:"formato"`
	ProdottoID            uuid.UUID          `json:"prodotto_id"`
	Sorgente              *Sorgente          `json:"sorgente,omitempty"`
	VersioneBomBase       *VersioneBomBase   `json:"versione_bom_base,omitempty"`
	FirmaBase             string             `json:"firma_base,omitempty"`
	Nodi                  []Nodo             `json:"nodi"`
	Archi                 []Arco             `json:"archi"`
	Decisioni             []json.RawMessage  `json:"decisioni"`
	DocumentiPreassociati []Preassociazione2 `json:"documenti_preassociati"`
}

// Sorgente e' la fonte STEP a cui la bozza e' ancorata (contratto rev. 3 §3.2): allegato e sha256 insieme o nessuno
// dei due, il gruppo opaco di /dati sempre (bom, senza, cand:<allegato>:<radice>).
type Sorgente struct {
	AllegatoID *uuid.UUID `json:"allegato_id,omitempty"`
	Sha256     string     `json:"sha256,omitempty"`
	Gruppo     string     `json:"gruppo"`
}

// VersioneBomBase e' la base della bozza: l'ultima conferma letta (nessuna: numero 0) e il contatore struttura.
type VersioneBomBase struct {
	ConfermaID *uuid.UUID `json:"conferma_id"`
	Numero     int        `json:"numero"`
	Contatore  int64      `json:"contatore"`
}

// Nodo e' un nodo della bozza: un'identita' desiderata (op) con i suoi dati. Un nodo da creare puo' non avere
// ancora il codice (Emendamento 2, E2.3: D-SHOCK-S3 C [U]): lo identifica la chiave, e il riepilogo dira'
// CODICE_DA_ASSEGNARE. La posizione del cartiglio sta sull'arco e non diventa un codice.
type Nodo struct {
	ChiaveBozza   string      `json:"chiave_bozza"`
	ComponenteID  *uuid.UUID  `json:"componente_id,omitempty"`
	Op            string      `json:"op"`
	Codice        string      `json:"codice,omitempty"`
	Rev           string      `json:"rev,omitempty"`
	Descrizione   string      `json:"descrizione,omitempty"`
	Tipo          string      `json:"tipo"`
	Caselle       []Casella   `json:"caselle,omitempty"`
	RigheProposta []uuid.UUID `json:"righe_proposta,omitempty"`
	Origine       string      `json:"origine"`
}

// Casella e' un nodo STEP del Motore A che il nodo della bozza rappresenta, nel suo gruppo.
type Casella struct {
	Gruppo string `json:"gruppo"`
	Rif    string `json:"rif"`
}

// Arco e' una relazione desiderata fra due nodi della bozza, con la quantita' sempre esplicita.
type Arco struct {
	Padre     string `json:"padre"`
	Figlio    string `json:"figlio"`
	Qta       int    `json:"qta"`
	Origine   string `json:"origine"`
	Posizione string `json:"posizione,omitempty"`
}

// Preassociazione2 lega una proposta di documento a un nodo della bozza formato 2 (contratto rev. 3 §12.3): e' la forma
// del formato 2 e del vocabolario 1, che si rilegge com'era (direttiva 35, blocco C-2; PD35 §6.3, INC-G1). Il formato 3
// ha la sua, Preassociazione3 {allegato_id, chiave_bozza}; l'unica traduzione dall'una all'altra sta nella conversione
// (conversione.go, traduciPreassociazioni).
type Preassociazione2 struct {
	PropostaID  uuid.UUID `json:"proposta_id"`
	ChiaveBozza string    `json:"chiave_bozza"`
}

// ------------------------------------------------------------------ la lettura stretta

// Le forme di ingresso: i puntatori dicono se un campo obbligatorio c'era. I tre campi della busta (bozza_id,
// bozza_versione, chiave_idempotenza) si leggono solo per rifiutarli: nello snapshot salvato non ci stanno.
type snapshotIn struct {
	Formato               *int               `json:"formato"`
	ProdottoID            *uuid.UUID         `json:"prodotto_id"`
	BozzaID               json.RawMessage    `json:"bozza_id"`
	BozzaVersione         json.RawMessage    `json:"bozza_versione"`
	ChiaveIdempotenza     json.RawMessage    `json:"chiave_idempotenza"`
	Sorgente              *Sorgente          `json:"sorgente"`
	VersioneBomBase       *versioneBomBaseIn `json:"versione_bom_base"`
	FirmaBase             *string            `json:"firma_base"`
	Nodi                  []nodoIn           `json:"nodi"`
	Archi                 []arcoIn           `json:"archi"`
	Decisioni             []json.RawMessage  `json:"decisioni"`
	DocumentiPreassociati []Preassociazione2 `json:"documenti_preassociati"`
}

type versioneBomBaseIn struct {
	ConfermaID *uuid.UUID `json:"conferma_id"`
	Numero     *int       `json:"numero"`
	Contatore  *int64     `json:"contatore"`
}

type nodoIn struct {
	ChiaveBozza   *string     `json:"chiave_bozza"`
	ComponenteID  *uuid.UUID  `json:"componente_id"`
	Op            *string     `json:"op"`
	Codice        *string     `json:"codice"`
	Rev           *string     `json:"rev"`
	Descrizione   *string     `json:"descrizione"`
	Tipo          *string     `json:"tipo"`
	Caselle       []Casella   `json:"caselle"`
	RigheProposta []uuid.UUID `json:"righe_proposta"`
	Origine       *string     `json:"origine"`
}

type arcoIn struct {
	Padre     *string `json:"padre"`
	Figlio    *string `json:"figlio"`
	Qta       *int    `json:"qta"`
	Origine   *string `json:"origine"`
	Posizione *string `json:"posizione"`
}

// decodificaStretta legge un JSON senza tolleranze: prima la rilettura stretta di jsoncanonico (UTF-8 valido,
// nessuna chiave ripetuta, niente dopo il valore: encoding/json li lascerebbe passare), poi i nomi dei campi in
// minuscolo (RC-12), poi encoding/json con i soli campi previsti.
func decodificaStretta(campo string, raw []byte, v any) *Errore {
	if _, err := jsoncanonico.Codifica(json.RawMessage(raw)); err != nil {
		return corpo(campo, "il JSON non si legge: %v", err)
	}
	if err := nomiInMinuscolo(raw); err != nil {
		return corpo(campo, "%v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return corpo(campo, "il JSON non ha la forma attesa: %v", err)
	}
	if dec.More() {
		return corpo(campo, "dopo il JSON c'e' altro")
	}
	return nil
}

// nomiInMinuscolo scorre il JSON (gia' riletto da jsoncanonico) e rifiuta un nome di campo che non sia in minuscolo:
// tutti i campi del contratto lo sono, e cosi' «nodi» e «NODI» non possono stare nello stesso oggetto.
func nomiInMinuscolo(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	type livello struct{ oggetto, attesaChiave bool }
	var pila []livello
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("il JSON non si legge: %v", err)
		}
		if d, ok := tok.(json.Delim); ok {
			switch d {
			case '{':
				pila = append(pila, livello{oggetto: true, attesaChiave: true})
				continue
			case '[':
				pila = append(pila, livello{})
				continue
			default: // '}' o ']': il valore del livello di sopra e' finito
				pila = pila[:len(pila)-1]
			}
		} else if n := len(pila); n > 0 && pila[n-1].oggetto && pila[n-1].attesaChiave {
			if k, _ := tok.(string); !reNomeCampo.MatchString(k) {
				return fmt.Errorf("il nome di campo %q non e' in minuscolo (lettere a-z, cifre, _)", k)
			}
			pila[n-1].attesaChiave = false
			continue
		}
		if n := len(pila); n > 0 && pila[n-1].oggetto {
			pila[n-1].attesaChiave = true
		}
	}
}

// nfc: le stringhe della forma canonica sono in NFC (contratto rev. 3 §6.3): «é» composta e «e» + accento combinante
// sono la stessa stringa e danno la stessa impronta. Si normalizza all'ingresso, prima dei limiti di lunghezza.
func nfc(s string) string { return norm.NFC.String(s) }

// LeggiSnapshot legge e valida la forma di uno snapshot formato 2 (contratto rev. 3 §3.2 «Regole di forma», §6.2) e
// lo restituisce normalizzato (elenchi ordinati, stringhe NFC). Non legge il database: appartenenza alla RFQ,
// esistenza dei componenti, delle proposte e degli allegati, e le regole di dominio (§4.4) le controlla chi chiama.
func LeggiSnapshot(raw []byte) (Snapshot, error) {
	return leggiSnapshot(raw, MaxSnapshot)
}

// leggiSnapshot e' LeggiSnapshot con un limite di peso a scelta: 0 = nessuno. La rilettura di uno snapshot salvato
// non ha limite, perche' il testo che restituisce PostgreSQL per un jsonb (con gli spazi dopo «:» e «,») pesa piu'
// dei byte canonici che si sono salvati.
func leggiSnapshot(raw []byte, limite int) (Snapshot, error) {
	if limite > 0 && len(raw) > limite {
		return Snapshot{}, corpo("snapshot", "lo snapshot pesa %d byte, oltre il limite di %d", len(raw), limite)
	}
	var in snapshotIn
	if e := decodificaStretta("snapshot", raw, &in); e != nil {
		return Snapshot{}, e
	}
	s, e := in.converti()
	if e != nil {
		return Snapshot{}, e
	}
	if e := s.valida(); e != nil {
		return Snapshot{}, e
	}
	s.normalizza()
	return s, nil
}

func presente(r json.RawMessage) bool { return len(r) > 0 }

func (in snapshotIn) converti() (Snapshot, *Errore) {
	var s Snapshot
	if in.Formato == nil {
		return s, corpo("formato", "manca il formato")
	}
	if *in.Formato != FormatoSnapshot {
		return s, corpo("formato", "formato %d: qui si salva solo il formato %d (il formato 1 resta su distinta/albero)", *in.Formato, FormatoSnapshot)
	}
	s.Formato = *in.Formato
	if in.ProdottoID == nil || *in.ProdottoID == uuid.Nil {
		return s, corpo("prodotto_id", "manca il prodotto")
	}
	s.ProdottoID = *in.ProdottoID
	for _, c := range []struct {
		nome string
		r    json.RawMessage
	}{{"bozza_id", in.BozzaID}, {"bozza_versione", in.BozzaVersione}, {"chiave_idempotenza", in.ChiaveIdempotenza}} {
		if presente(c.r) {
			return s, corpo(c.nome, "lo snapshot salvato non porta %s: e' un campo della richiesta, non della bozza", c.nome)
		}
	}
	s.Sorgente = sorgenteNFC(in.Sorgente)
	if b := in.VersioneBomBase; b != nil {
		if b.Numero == nil || b.Contatore == nil {
			return s, corpo("versione_bom_base", "la base vuole numero e contatore")
		}
		s.VersioneBomBase = &VersioneBomBase{ConfermaID: b.ConfermaID, Numero: *b.Numero, Contatore: *b.Contatore}
	}
	if in.FirmaBase != nil {
		if *in.FirmaBase == "" {
			return s, corpo("firma_base", "la firma della base e' vuota")
		}
		s.FirmaBase = *in.FirmaBase
	}
	if len(in.Nodi) > MaxVoci {
		return s, corpo("nodi", "%d nodi, oltre il limite di %d", len(in.Nodi), MaxVoci)
	}
	for i, n := range in.Nodi {
		nodo, e := n.converti(fmt.Sprintf("nodi[%d]", i))
		if e != nil {
			return s, e
		}
		s.Nodi = append(s.Nodi, nodo)
	}
	if len(in.Archi) > MaxVoci {
		return s, corpo("archi", "%d archi, oltre il limite di %d", len(in.Archi), MaxVoci)
	}
	for i, a := range in.Archi {
		arco, e := a.converti(fmt.Sprintf("archi[%d]", i))
		if e != nil {
			return s, e
		}
		s.Archi = append(s.Archi, arco)
	}
	if len(in.Decisioni) > MaxVoci {
		return s, corpo("decisioni", "%d decisioni, oltre il limite di %d", len(in.Decisioni), MaxVoci)
	}
	for i, d := range in.Decisioni {
		c, _, e := leggiDecisione(fmt.Sprintf("decisioni[%d]", i), d)
		if e != nil {
			return s, e
		}
		s.Decisioni = append(s.Decisioni, c)
	}
	if len(in.DocumentiPreassociati) > MaxVoci {
		return s, corpo("documenti_preassociati", "%d preassociazioni, oltre il limite di %d", len(in.DocumentiPreassociati), MaxVoci)
	}
	s.DocumentiPreassociati = in.DocumentiPreassociati
	return s, nil
}

func sorgenteNFC(s *Sorgente) *Sorgente {
	if s == nil {
		return nil
	}
	c := *s
	c.Gruppo = nfc(c.Gruppo)
	return &c
}

// converti: i campi obbligatori di un nodo sono chiave_bozza, op, tipo, origine; il codice e' obbligatorio salvo per
// op = crea (E2.3) e lo controlla validaNodo, che sa quale op ha il nodo.
func (n nodoIn) converti(campo string) (Nodo, *Errore) {
	var o Nodo
	obbligatori := []struct {
		nome string
		v    *string
	}{{"chiave_bozza", n.ChiaveBozza}, {"op", n.Op}, {"tipo", n.Tipo}, {"origine", n.Origine}}
	for _, c := range obbligatori {
		if c.v == nil {
			return o, corpo(campo+"."+c.nome, "manca %s", c.nome)
		}
	}
	o = Nodo{ChiaveBozza: *n.ChiaveBozza, ComponenteID: n.ComponenteID, Op: *n.Op, Tipo: *n.Tipo,
		RigheProposta: n.RigheProposta, Origine: *n.Origine}
	if n.Codice != nil {
		o.Codice = nfc(*n.Codice)
	}
	if n.Rev != nil {
		o.Rev = nfc(*n.Rev)
	}
	if n.Descrizione != nil {
		o.Descrizione = nfc(*n.Descrizione)
	}
	for _, c := range n.Caselle {
		o.Caselle = append(o.Caselle, Casella{Gruppo: nfc(c.Gruppo), Rif: nfc(c.Rif)})
	}
	return o, nil
}

func (a arcoIn) converti(campo string) (Arco, *Errore) {
	var o Arco
	if a.Padre == nil || a.Figlio == nil {
		return o, corpo(campo, "un arco vuole padre e figlio")
	}
	if a.Qta == nil {
		return o, corpo(campo+".qta", "l'arco %s → %s non ha la quantita': non si mette 1 da soli", *a.Padre, *a.Figlio)
	}
	if a.Origine == nil {
		return o, corpo(campo+".origine", "manca l'origine dell'arco")
	}
	o = Arco{Padre: *a.Padre, Figlio: *a.Figlio, Qta: *a.Qta, Origine: *a.Origine}
	if a.Posizione != nil {
		o.Posizione = nfc(*a.Posizione)
	}
	return o, nil
}

// decisioneTesta: i due campi di ogni decisione che la forma controlla gia' in B1; gli altri sono di B2.
type decisioneTesta struct {
	ID   *string `json:"id"`
	Tipo *string `json:"tipo"`
}

// leggiDecisione rilegge una decisione in modo stretto e ne restituisce la forma canonica e l'id. La decisione e'
// ancora opaca (B2 ne controllera' i campi per tipo, E1.5), ma la sua forma canonica deve tornare identica dopo
// jsonb: numeri interi senza esponente, nessun U+0000, stringhe in NFC, nomi dei campi in minuscolo (RC-01, RC-07,
// RC-12).
func leggiDecisione(campo string, raw json.RawMessage) (json.RawMessage, string, *Errore) {
	if _, err := jsoncanonico.Codifica(raw); err != nil {
		return nil, "", corpo(campo, "la decisione non si legge: %v", err)
	}
	if err := nomiInMinuscolo(raw); err != nil {
		return nil, "", corpo(campo, "%v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, "", corpo(campo, "la decisione non si legge: %v", err)
	}
	if _, ok := v.(map[string]any); !ok {
		return nil, "", corpo(campo, "una decisione e' un oggetto")
	}
	pulita, err := valoreCanonico(v)
	if err != nil {
		return nil, "", corpo(campo, "%v", err)
	}
	c, err := jsoncanonico.Codifica(pulita)
	if err != nil {
		return nil, "", corpo(campo, "la decisione non ha una forma canonica: %v", err)
	}
	var t decisioneTesta
	if err := json.Unmarshal(c, &t); err != nil {
		return nil, "", corpo(campo, "la decisione non ha la forma attesa: %v", err)
	}
	if t.ID == nil || !reChiave.MatchString(*t.ID) {
		return nil, "", corpo(campo+".id", "l'id di una decisione e' 1-64 caratteri fra lettere, cifre e . _ : -")
	}
	if t.Tipo == nil || !tipiDecisione[*t.Tipo] {
		return nil, "", corpo(campo+".tipo", "tipo di decisione sconosciuto")
	}
	return json.RawMessage(c), *t.ID, nil
}

// valoreCanonico scorre un valore JSON letto con UseNumber: i numeri devono essere interi senza esponente e diversi
// da -0, le stringhe senza U+0000 (PostgreSQL non lo accetta in jsonb) e in NFC.
func valoreCanonico(v any) (any, error) {
	switch x := v.(type) {
	case string:
		if strings.ContainsRune(x, 0) {
			return nil, fmt.Errorf("una stringa contiene U+0000, che il database non accetta")
		}
		return nfc(x), nil
	case json.Number:
		if len(strings.TrimPrefix(string(x), "-")) > MaxCifre {
			return nil, fmt.Errorf("un numero ha piu' di %d cifre", MaxCifre)
		}
		if !reIntero.MatchString(string(x)) || string(x) == "-0" {
			return nil, fmt.Errorf("il numero %s non e' un intero scritto senza esponente (la forma canonica ha solo interi)", x)
		}
		return x, nil
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			p, err := valoreCanonico(e)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			out[k] = p
		}
		return out, nil
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			p, err := valoreCanonico(e)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			out[i] = p
		}
		return out, nil
	default: // bool, nil
		return x, nil
	}
}

func idDecisione(c json.RawMessage) string {
	var t decisioneTesta
	_ = json.Unmarshal(c, &t)
	if t.ID == nil {
		return ""
	}
	return *t.ID
}

// ------------------------------------------------------------------ la validazione della forma

func runeOltre(s string, max int) bool { return utf8.RuneCountInString(s) > max }

func vuotaOSpazi(s string) bool { return strings.TrimSpace(s) == "" }

func haControlli(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return unicode.IsControl(r) })
}

// testoLibero: un motivo o un gruppo, non vuoto, entro il limite, senza caratteri di controllo (U+0000 compreso:
// il database lo rifiuterebbe con un errore grezzo, RC-07).
func testoLibero(s string, max int) bool {
	return !vuotaOSpazi(s) && !runeOltre(s, max) && !haControlli(s)
}

func (s *Sorgente) valida(campo string) *Errore {
	if s == nil {
		return nil
	}
	if !testoLibero(s.Gruppo, MaxGruppo) {
		return corpo(campo+".gruppo", "il gruppo della fonte e' obbligatorio, al piu' %d caratteri, senza caratteri di controllo", MaxGruppo)
	}
	if (s.AllegatoID == nil) != (s.Sha256 == "") {
		return corpo(campo, "allegato_id e sha256 della fonte vanno insieme")
	}
	if s.AllegatoID != nil && *s.AllegatoID == uuid.Nil {
		return corpo(campo+".allegato_id", "allegato nullo")
	}
	if s.Sha256 != "" && !reSha256.MatchString(s.Sha256) {
		return corpo(campo+".sha256", "lo sha256 e' di 64 cifre esadecimali minuscole")
	}
	return nil
}

func (b *VersioneBomBase) valida(campo string) *Errore {
	if b == nil {
		return nil
	}
	if b.Numero < 0 || b.Contatore < 0 {
		return corpo(campo, "numero e contatore della base non sono negativi")
	}
	if (b.Numero == 0) != (b.ConfermaID == nil) {
		return corpo(campo, "numero 0 vuol dire nessuna conferma, e solo allora conferma_id e' null")
	}
	return nil
}

// codiceRegistrato: il codice di un componente gia' registrato (riusa, modifica, esclude) e' quello della colonna,
// fino a 60 caratteri; che sia proprio quello registrato lo controlla B2, quando risolve le identita' (Δ31, RC-08).
func codiceRegistrato(s string) bool {
	return s != "" && strings.TrimSpace(s) == s && !runeOltre(s, MaxCodiceRegistrato) && !haControlli(s)
}

func validaNodo(campo string, n Nodo) *Errore {
	if !reChiave.MatchString(n.ChiaveBozza) {
		return corpo(campo+".chiave_bozza", "la chiave e' 1-64 caratteri fra lettere, cifre e . _ : -")
	}
	switch n.Op {
	case OpCrea:
		if n.ComponenteID != nil {
			return corpo(campo+".componente_id", "un nodo da creare non ha componente_id: l'UUID lo da' la conferma")
		}
		// E2.3: un nodo nuovo si salva anche senza codice; se c'e', e' un codice nuovo (Δ31)
		if n.Codice != "" && !classificazione.CodiceAmmissibile(n.Codice) {
			return corpo(campo+".codice", "un codice nuovo e' senza spazi e al piu' %d caratteri", classificazione.MaxCodice)
		}
	case OpRiusa, OpModifica, OpEsclude:
		if n.ComponenteID == nil || *n.ComponenteID == uuid.Nil {
			return corpo(campo+".componente_id", "op %s vuole il componente_id esistente", n.Op)
		}
		if !codiceRegistrato(n.Codice) {
			return corpo(campo+".codice", "op %s vuole il codice del componente, al piu' %d caratteri", n.Op, MaxCodiceRegistrato)
		}
	default:
		return corpo(campo+".op", "op sconosciuta %q (crea, riusa, modifica, esclude)", n.Op)
	}
	if n.Rev != "" && !classificazione.RevAmmissibile(n.Rev) {
		return corpo(campo+".rev", "revisione con spazi o oltre %d caratteri", classificazione.MaxRev)
	}
	if runeOltre(n.Descrizione, MaxDescrizione) || haControlli(n.Descrizione) {
		return corpo(campo+".descrizione", "descrizione oltre %d caratteri o con caratteri di controllo", MaxDescrizione)
	}
	if !db.TipoComponente(n.Tipo).Valid() {
		return corpo(campo+".tipo", "tipo sconosciuto %q", n.Tipo)
	}
	if !origini[n.Origine] {
		return corpo(campo+".origine", "origine sconosciuta %q (step, utente, conferma, registrato)", n.Origine)
	}
	if len(n.Caselle) > MaxVoci || len(n.RigheProposta) > MaxVoci {
		return corpo(campo, "troppe caselle o righe di proposta (al piu' %d)", MaxVoci)
	}
	viste := map[Casella]bool{}
	for j, c := range n.Caselle {
		cc := fmt.Sprintf("%s.caselle[%d]", campo, j)
		if !testoLibero(c.Gruppo, MaxGruppo) {
			return corpo(cc+".gruppo", "il gruppo e' obbligatorio, al piu' %d caratteri, senza caratteri di controllo", MaxGruppo)
		}
		if !reRif.MatchString(c.Rif) || runeOltre(c.Rif, MaxRif) || haControlli(c.Rif) {
			return corpo(cc+".rif", "rif nella forma nodo:<sha256>:<chiave>, al piu' %d caratteri", MaxRif)
		}
		if viste[c] {
			return corpo(cc, "casella ripetuta nello stesso nodo")
		}
		viste[c] = true
	}
	for j, r := range n.RigheProposta {
		if r == uuid.Nil {
			return corpo(fmt.Sprintf("%s.righe_proposta[%d]", campo, j), "riga di proposta nulla")
		}
	}
	return nil
}

func validaArco(campo string, a Arco) *Errore {
	if !reChiave.MatchString(a.Padre) || !reChiave.MatchString(a.Figlio) {
		return corpo(campo, "padre e figlio sono chiavi della bozza")
	}
	if a.Padre == a.Figlio {
		return corpo(campo, "un arco da %s a se stesso", a.Padre)
	}
	if a.Qta < 1 || a.Qta > MaxQta {
		return corpo(campo+".qta", "quantita' %d fuori da 1…%d", a.Qta, MaxQta)
	}
	if !origini[a.Origine] {
		return corpo(campo+".origine", "origine sconosciuta %q (step, utente, conferma, registrato)", a.Origine)
	}
	if runeOltre(a.Posizione, MaxPosizione) || haControlli(a.Posizione) {
		return corpo(campo+".posizione", "posizione oltre %d caratteri", MaxPosizione)
	}
	return nil
}

func (s *Snapshot) valida() *Errore {
	if e := s.Sorgente.valida("sorgente"); e != nil {
		return e
	}
	if e := s.VersioneBomBase.valida("versione_bom_base"); e != nil {
		return e
	}
	if s.FirmaBase != "" && !reSha256.MatchString(s.FirmaBase) {
		return corpo("firma_base", "la firma della base e' di 64 cifre esadecimali minuscole")
	}
	if len(s.Nodi) == 0 {
		return corpo("nodi", "la bozza ha almeno il nodo del prodotto")
	}
	chiavi := map[string]Nodo{}
	casellaDi := map[Casella]string{}
	rigaDi := map[uuid.UUID]string{}
	radici := 0
	for i, n := range s.Nodi {
		campo := fmt.Sprintf("nodi[%d]", i)
		if e := validaNodo(campo, n); e != nil {
			return e
		}
		if _, dup := chiavi[n.ChiaveBozza]; dup {
			return corpo(campo+".chiave_bozza", "chiave %q ripetuta", n.ChiaveBozza)
		}
		chiavi[n.ChiaveBozza] = n
		radice := n.ComponenteID != nil && *n.ComponenteID == s.ProdottoID
		if radice {
			radici++
		}
		if n.Tipo == string(db.TipoComponenteFinito) && !radice {
			return corpo(campo+".tipo", "finito solo sulla radice (il prodotto %s)", s.ProdottoID)
		}
		// Δ43: una casella e' un nodo di una fonte STEP, e la fonte si dichiara
		if len(n.Caselle) > 0 && s.Sorgente == nil {
			return corpo("sorgente", "il nodo %s ha caselle STEP: la bozza vuole la sorgente (allegato, sha256, gruppo)", n.ChiaveBozza)
		}
		for _, c := range n.Caselle {
			if altro, ok := casellaDi[c]; ok {
				return corpo(campo+".caselle", "il rif %s del gruppo %s sta gia' nel nodo %s: lo stesso rif in due nodi solo con gruppi diversi", c.Rif, c.Gruppo, altro)
			}
			casellaDi[c] = n.ChiaveBozza
		}
		for _, r := range n.RigheProposta {
			if altro, ok := rigaDi[r]; ok {
				return corpo(campo+".righe_proposta", "la riga di proposta %s sta gia' nel nodo %s", r, altro)
			}
			rigaDi[r] = n.ChiaveBozza
		}
	}
	if radici != 1 {
		return corpo("nodi", "%d nodi con il componente del prodotto %s: ne serve esattamente uno (la radice)", radici, s.ProdottoID)
	}
	coppie := map[[2]string]bool{}
	for i, a := range s.Archi {
		campo := fmt.Sprintf("archi[%d]", i)
		if e := validaArco(campo, a); e != nil {
			return e
		}
		if _, ok := chiavi[a.Padre]; !ok {
			return corpo(campo+".padre", "il padre %q non e' un nodo della bozza", a.Padre)
		}
		if _, ok := chiavi[a.Figlio]; !ok {
			return corpo(campo+".figlio", "il figlio %q non e' un nodo della bozza", a.Figlio)
		}
		k := [2]string{a.Padre, a.Figlio}
		if coppie[k] {
			return corpo(campo, "l'arco %s → %s e' ripetuto", a.Padre, a.Figlio)
		}
		coppie[k] = true
	}
	ids := map[string]bool{}
	for i, d := range s.Decisioni {
		id := idDecisione(d)
		if ids[id] {
			return corpo(fmt.Sprintf("decisioni[%d].id", i), "id %q ripetuto", id)
		}
		ids[id] = true
	}
	proposte := map[uuid.UUID]bool{}
	for i, p := range s.DocumentiPreassociati {
		campo := fmt.Sprintf("documenti_preassociati[%d]", i)
		if p.PropostaID == uuid.Nil {
			return corpo(campo+".proposta_id", "proposta nulla")
		}
		if proposte[p.PropostaID] {
			return corpo(campo+".proposta_id", "la proposta %s e' preassociata due volte", p.PropostaID)
		}
		proposte[p.PropostaID] = true
		n, ok := chiavi[p.ChiaveBozza]
		if !ok {
			return corpo(campo+".chiave_bozza", "%q non e' un nodo della bozza", p.ChiaveBozza)
		}
		if n.Op == OpEsclude {
			return corpo(campo+".chiave_bozza", "un documento non si preassocia a un nodo escluso")
		}
	}
	return s.originiConferma()
}

// originiConferma (Δ32): un nodo o un arco con origine «conferma» viene da una conferma, e la base allora ha numero
// almeno 1. Se lo snapshot non porta la base la si ricontrolla dopo, quando Salva la conosce.
func (s *Snapshot) originiConferma() *Errore {
	if s.VersioneBomBase == nil || s.VersioneBomBase.Numero >= 1 {
		return nil
	}
	for i, n := range s.Nodi {
		if n.Origine == OrigineConferma {
			return corpo(fmt.Sprintf("nodi[%d].origine", i), "il nodo %s ha origine conferma, ma la base non ha conferme (numero 0)", n.ChiaveBozza)
		}
	}
	for i, a := range s.Archi {
		if a.Origine == OrigineConferma {
			return corpo(fmt.Sprintf("archi[%d].origine", i), "l'arco %s → %s ha origine conferma, ma la base non ha conferme (numero 0)", a.Padre, a.Figlio)
		}
	}
	return nil
}

// ------------------------------------------------------------------ la forma canonica

// normalizza ordina gli elenchi senza significato d'ordine e mette gli elenchi di testa vuoti al posto dei nil.
func (s *Snapshot) normalizza() {
	if s.Nodi == nil {
		s.Nodi = []Nodo{}
	}
	if s.Archi == nil {
		s.Archi = []Arco{}
	}
	if s.Decisioni == nil {
		s.Decisioni = []json.RawMessage{}
	}
	if s.DocumentiPreassociati == nil {
		s.DocumentiPreassociati = []Preassociazione2{}
	}
	for i := range s.Nodi {
		normalizzaNodo(&s.Nodi[i])
	}
	sort.Slice(s.Nodi, func(i, j int) bool { return s.Nodi[i].ChiaveBozza < s.Nodi[j].ChiaveBozza })
	sort.Slice(s.Archi, func(i, j int) bool {
		if s.Archi[i].Padre != s.Archi[j].Padre {
			return s.Archi[i].Padre < s.Archi[j].Padre
		}
		return s.Archi[i].Figlio < s.Archi[j].Figlio
	})
	sort.Slice(s.Decisioni, func(i, j int) bool { return idDecisione(s.Decisioni[i]) < idDecisione(s.Decisioni[j]) })
	sort.Slice(s.DocumentiPreassociati, func(i, j int) bool {
		return s.DocumentiPreassociati[i].PropostaID.String() < s.DocumentiPreassociati[j].PropostaID.String()
	})
}

// normalizzaNodo ordina caselle e righe; un elenco vuoto diventa nil (nella forma canonica non si scrive).
func normalizzaNodo(n *Nodo) {
	if len(n.Caselle) == 0 {
		n.Caselle = nil
	}
	if len(n.RigheProposta) == 0 {
		n.RigheProposta = nil
	}
	sort.Slice(n.Caselle, func(i, j int) bool {
		if n.Caselle[i].Gruppo != n.Caselle[j].Gruppo {
			return n.Caselle[i].Gruppo < n.Caselle[j].Gruppo
		}
		return n.Caselle[i].Rif < n.Caselle[j].Rif
	})
	sort.Slice(n.RigheProposta, func(i, j int) bool { return n.RigheProposta[i].String() < n.RigheProposta[j].String() })
}

// Canonico restituisce i byte canonici dello snapshot (normalizzato prima): sono quelli che si salvano, che R1
// restituisce e di cui si calcola l'impronta.
func (s Snapshot) Canonico() ([]byte, error) {
	c := s.copia()
	c.normalizza()
	return jsoncanonico.Codifica(c)
}

// Impronta e' lo sha256 dei byte canonici (distinta_bozza_versione.impronta, conferma_strutturale.impronta_snapshot).
func (s Snapshot) Impronta() (string, error) {
	b, err := s.Canonico()
	if err != nil {
		return "", err
	}
	return jsoncanonico.Impronta(b), nil
}

// copia: una copia profonda abbastanza da poter ordinare senza toccare l'originale.
func (s Snapshot) copia() Snapshot {
	c := s
	c.Nodi = make([]Nodo, len(s.Nodi))
	for i, n := range s.Nodi {
		n.Caselle = append([]Casella(nil), n.Caselle...)
		n.RigheProposta = append([]uuid.UUID(nil), n.RigheProposta...)
		c.Nodi[i] = n
	}
	c.Archi = append([]Arco(nil), s.Archi...)
	c.Decisioni = append([]json.RawMessage(nil), s.Decisioni...)
	c.DocumentiPreassociati = append([]Preassociazione2(nil), s.DocumentiPreassociati...)
	if s.Sorgente != nil {
		sc := *s.Sorgente
		c.Sorgente = &sc
	}
	if s.VersioneBomBase != nil {
		bc := *s.VersioneBomBase
		c.VersioneBomBase = &bc
	}
	return c
}
