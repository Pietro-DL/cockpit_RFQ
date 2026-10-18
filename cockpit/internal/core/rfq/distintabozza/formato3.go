package distintabozza

// Il formato 3 della bozza (V1; emendamento di V1 al contratto rev. 3, EM §3): lo snapshot con i nodi formato 3
// (caselle con l'occorrenza, identita' decisa), gli archi con lo step, le 16 decisioni con una forma chiusa per tipo
// (decisioni3.go), la sorgente con il registro d'importazione (il pacchetto importazione). Il formato 2 resta com'era
// (snapshot.go): il suo lettore serve solo a rileggere versioni e conferme gia' salvate (lettori.go).
//
// La regola dell'NFC (EM §3.10, prova N-01): descrizioni, motivi e posizioni (testi umani) in NFC, all'ingresso e
// prima dei limiti di lunghezza; l'identita' (codice, rev, emittente, namespace, anche dentro le decisioni identita e
// ricodifica) e tutto quello che viene dal registro (letture, letterali, segnali) byte per byte, mai NFC: un codice
// accettato dalla proposta resta uguale alla proposta. Gruppi, rif e UUID sono ASCII e restano come sono.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// FormatoSnapshot3 e' il formato della bozza e della conferma di V1 (EM §2.5): l'unico che si scrive.
const FormatoSnapshot3 = 3

// VocabolarioFormato3 e' il vocabolario delle azioni del giornale di una bozza formato 3 (EM §9.2): i verbi del
// vocabolario 1 con gli stessi nomi, piu' proposta_riallineata e file_lasciato.
const VocabolarioFormato3 = 2

// I limiti del formato 3 che il formato 2 non ha (EM §3.4.2, §3.6, §3.10; PM23 §4.6). Gli altri sono quelli del
// formato 2 (MaxSnapshot, MaxVoci, MaxQta, MaxDescrizione, MaxPosizione, MaxGruppo, MaxRif, MaxMotivo, MaxCifre).
const (
	// MaxIDPartner: caratteri dell'id di un partner (partner:<id>). La grammatica non ha un tetto; questo e' quello di
	// sicurezza del CHECK ck_csc_identita (char_length(emittente) fino a 208, cioe' 200 dopo «partner:»): un valore
	// oltre e' un 422 del salvataggio, mai un 23514 al COMMIT della conferma.
	MaxIDPartner = 200
	// MaxNamespace: caratteri di un namespace (tetto di sicurezza del CHECK ck_csc_identita).
	MaxNamespace = 500
	// MaxMotivoRicodifica: caratteri del motivo di una ricodifica, dopo l'NFC: R5 lo copia in correzione_documento,
	// che ha ck_correzione_motivo <= 500 (EM §3.6, riga 10).
	MaxMotivoRicodifica = 500
	// MaxCodiceSegnale: caratteri di codice, regola e versione_servizio di un segnale visto (tetto di forma [T]).
	MaxCodiceSegnale = 200
	// MaxChiaveLettura: caratteri di una chiave di lettura <allegato_id>/<id della lettura> (tetto di forma [T]).
	MaxChiaveLettura = 400
)

// I modi dell'identita' decisa di un nodo (EM §3.4.2).
const (
	ModoProposta   = "proposta"
	ModoDecisione  = "decisione"
	ModoRegistrato = "registrato"
	ModoManuale    = "manuale"
	ModoDaDecidere = "da_decidere"
	ModoFormato2   = "formato_2"
)

var modiIdentita = map[string]bool{ModoProposta: true, ModoDecisione: true, ModoRegistrato: true, ModoManuale: true,
	ModoDaDecidere: true, ModoFormato2: true}

// I motivi di CORPO_NON_VALIDO del formato 3 che la forma sa dare senza il database (EM §9.2).
const (
	MotivoRifFuoriImportazione     = "rif_fuori_importazione"
	MotivoEmittentePresuntoDiverso = "emittente_presunto_diverso"
	MotivoArcoSenzaQuantita        = "arco_senza_quantita"
)

var (
	reUUIDMinuscolo = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	reChiaveLettura = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/.+$`)
)

func corpoMotivo(campo, motivo, formato string, a ...any) *Errore {
	return &Errore{Codice: CodiceCorpoNonValido, Motivo: motivo, Campo: campo, Messaggio: fmt.Sprintf(formato, a...)}
}

// Snapshot3 e' la bozza formato 3 di un prodotto, come si salva e come si rilegge (EM §3.2): dodici campi di primo
// livello come nel formato 2, tre dei quali (bozza_id, bozza_versione, chiave_idempotenza) sono della richiesta e non
// stanno nello snapshot salvato. La forma canonica e' quella di jsoncanonico, senza NFC sull'identita'; gli elenchi in
// ordine: nodi per chiave, archi per (padre, figlio), decisioni per id, preassociazioni per proposta, caselle per
// (gruppo, rif, allegato_id). I facoltativi vuoti non si scrivono; i quattro elenchi di testa si scrivono sempre.
type Snapshot3 struct {
	Formato               int               `json:"formato"`
	ProdottoID            uuid.UUID         `json:"prodotto_id"`
	Sorgente              *Sorgente3        `json:"sorgente,omitempty"`
	VersioneBomBase       *VersioneBomBase  `json:"versione_bom_base,omitempty"`
	FirmaBase             string            `json:"firma_base,omitempty"`
	Nodi                  []Nodo3           `json:"nodi"`
	Archi                 []Arco3           `json:"archi"`
	Decisioni             []Decisione3      `json:"decisioni"`
	DocumentiPreassociati []Preassociazione `json:"documenti_preassociati"`
}

// Sorgente3 e' la fonte STEP della bozza con il registro d'importazione (EM §3.3): allegato e sha256 insieme o nessuno
// dei due; il gruppo opaco di /dati, mai scomposto; l'impronta del registro della versione, se la bozza ne ha uno.
type Sorgente3 struct {
	AllegatoID   *uuid.UUID `json:"allegato_id,omitempty"`
	Sha256       string     `json:"sha256,omitempty"`
	Gruppo       string     `json:"gruppo"`
	Importazione string     `json:"importazione,omitempty"`
}

// Nodo3 e' un nodo formato 3 (EM §3.4): codice e rev stanno nell'identita' decisa, le righe di proposta non ci sono piu'
// (R4 e R5 le ritrovano per (thread, sha256, chiave)).
type Nodo3 struct {
	ChiaveBozza  string     `json:"chiave_bozza"`
	ComponenteID *uuid.UUID `json:"componente_id,omitempty"`
	Op           string     `json:"op"`
	Tipo         string     `json:"tipo"`
	Descrizione  string     `json:"descrizione,omitempty"`
	Origine      string     `json:"origine"`
	Caselle      []Casella3 `json:"caselle,omitempty"`
	Identita     Identita   `json:"identita"`
}

// Casella3 e' la chiave d'origine di un nodo STEP (EM §3.4.1): l'occorrenza, il gruppo, il rif nodo:<sha256>:<chiave>.
type Casella3 struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	Gruppo     string    `json:"gruppo"`
	Rif        string    `json:"rif"`
}

// Identita e' l'identita' decisa di un nodo (EM §3.4.2): emittente sempre esplicito, la presunzione che calcola il server
// (assente = falso), namespace e codice byte per byte, la rev della working, il modo e, con modo decisione, la decisione.
type Identita struct {
	Emittente         string `json:"emittente"`
	EmittentePresunto bool   `json:"emittente_presunto,omitempty"`
	Namespace         string `json:"namespace,omitempty"`
	Codice            string `json:"codice,omitempty"`
	Rev               string `json:"rev,omitempty"`
	Modo              string `json:"modo"`
	Decisione         string `json:"decisione,omitempty"`
}

// Arco3 e' un arco formato 3 (EM §3.5): qta sempre esplicita, origine esplicita, step se e solo se origine = step.
type Arco3 struct {
	Padre     string     `json:"padre"`
	Figlio    string     `json:"figlio"`
	Qta       int        `json:"qta"`
	Origine   string     `json:"origine"`
	Posizione string     `json:"posizione,omitempty"`
	Step      *StepArco3 `json:"step,omitempty"`
}

// StepArco3 dice da quale occorrenza e gruppo viene un arco STEP accettato (EM §3.5).
type StepArco3 struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	Gruppo     string    `json:"gruppo"`
}

// ------------------------------------------------------------------ la lettura stretta

type snapshot3In struct {
	Formato               *int               `json:"formato"`
	ProdottoID            *uuid.UUID         `json:"prodotto_id"`
	BozzaID               json.RawMessage    `json:"bozza_id"`
	BozzaVersione         json.RawMessage    `json:"bozza_versione"`
	ChiaveIdempotenza     json.RawMessage    `json:"chiave_idempotenza"`
	Sorgente              *sorgente3In       `json:"sorgente"`
	VersioneBomBase       *versioneBomBaseIn `json:"versione_bom_base"`
	FirmaBase             *string            `json:"firma_base"`
	Nodi                  []nodo3In          `json:"nodi"`
	Archi                 []arco3In          `json:"archi"`
	Decisioni             []json.RawMessage  `json:"decisioni"`
	DocumentiPreassociati []Preassociazione  `json:"documenti_preassociati"`
}

type sorgente3In struct {
	AllegatoID   *uuid.UUID `json:"allegato_id"`
	Sha256       *string    `json:"sha256"`
	Gruppo       *string    `json:"gruppo"`
	Importazione *string    `json:"importazione"`
}

type nodo3In struct {
	ChiaveBozza  *string      `json:"chiave_bozza"`
	ComponenteID *uuid.UUID   `json:"componente_id"`
	Op           *string      `json:"op"`
	Tipo         *string      `json:"tipo"`
	Descrizione  *string      `json:"descrizione"`
	Origine      *string      `json:"origine"`
	Caselle      []casella3In `json:"caselle"`
	Identita     *identitaIn  `json:"identita"`
}

type casella3In struct {
	AllegatoID *uuid.UUID `json:"allegato_id"`
	Gruppo     *string    `json:"gruppo"`
	Rif        *string    `json:"rif"`
}

type identitaIn struct {
	Emittente         *string `json:"emittente"`
	EmittentePresunto *bool   `json:"emittente_presunto"`
	Namespace         *string `json:"namespace"`
	Codice            *string `json:"codice"`
	Rev               *string `json:"rev"`
	Modo              *string `json:"modo"`
	Decisione         *string `json:"decisione"`
}

type arco3In struct {
	Padre     *string      `json:"padre"`
	Figlio    *string      `json:"figlio"`
	Qta       *int         `json:"qta"`
	Origine   *string      `json:"origine"`
	Posizione *string      `json:"posizione"`
	Step      *stepArco3In `json:"step"`
}

type stepArco3In struct {
	AllegatoID *uuid.UUID `json:"allegato_id"`
	Gruppo     *string    `json:"gruppo"`
}

// LeggiSnapshot3 legge e valida la forma di uno snapshot formato 3 (EM §3.2-§3.7, §7.2; senza leggere il database) e
// lo restituisce normalizzato. L'appartenenza alla RFQ, i registri citati (V-1...V-3, V-5), l'identita' registrata, la
// regola unica dell'emittente e le regole di dominio le controlla chi chiama.
func LeggiSnapshot3(raw []byte) (Snapshot3, error) {
	s, e := leggiSnapshot3(raw, MaxSnapshot)
	if e != nil {
		return Snapshot3{}, e
	}
	return s, nil
}

// leggiSnapshot3 e' LeggiSnapshot3 con un limite di peso a scelta: 0 = nessuno (la rilettura di uno snapshot salvato:
// il testo che restituisce PostgreSQL per un jsonb pesa piu' dei byte canonici salvati).
func leggiSnapshot3(raw []byte, limite int) (Snapshot3, *Errore) {
	if limite > 0 && len(raw) > limite {
		return Snapshot3{}, corpo("snapshot", "lo snapshot pesa %d byte, oltre il limite di %d", len(raw), limite)
	}
	if e := valoriSenzaNFC("snapshot", raw); e != nil {
		return Snapshot3{}, e
	}
	var in snapshot3In
	if e := decodificaStretta("snapshot", raw, &in); e != nil {
		return Snapshot3{}, e
	}
	s, e := in.converti()
	if e != nil {
		return Snapshot3{}, e
	}
	if e := s.valida(); e != nil {
		return Snapshot3{}, e
	}
	s.normalizza()
	return s, nil
}

func (in snapshot3In) converti() (Snapshot3, *Errore) {
	var s Snapshot3
	if in.Formato == nil {
		return s, corpo("formato", "manca il formato")
	}
	if *in.Formato != FormatoSnapshot3 {
		return s, corpo("formato", "formato %d: qui si legge solo il formato %d (il formato 2 si rilegge con il suo lettore)", *in.Formato, FormatoSnapshot3)
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
	if so := in.Sorgente; so != nil {
		s.Sorgente = &Sorgente3{AllegatoID: so.AllegatoID, Sha256: valore(so.Sha256), Gruppo: valore(so.Gruppo),
			Importazione: valore(so.Importazione)}
	}
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
		dec, e := LeggiDecisione3(fmt.Sprintf("decisioni[%d]", i), d)
		if e != nil {
			return s, e
		}
		s.Decisioni = append(s.Decisioni, dec)
	}
	if len(in.DocumentiPreassociati) > MaxVoci {
		return s, corpo("documenti_preassociati", "%d preassociazioni, oltre il limite di %d", len(in.DocumentiPreassociati), MaxVoci)
	}
	s.DocumentiPreassociati = in.DocumentiPreassociati
	return s, nil
}

func valore(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (n nodo3In) converti(campo string) (Nodo3, *Errore) {
	var o Nodo3
	for _, c := range []struct {
		nome string
		v    *string
	}{{"chiave_bozza", n.ChiaveBozza}, {"op", n.Op}, {"tipo", n.Tipo}, {"origine", n.Origine}} {
		if c.v == nil {
			return o, corpo(campo+"."+c.nome, "manca %s", c.nome)
		}
	}
	if n.Identita == nil {
		return o, corpo(campo+".identita", "un nodo formato 3 ha sempre la sua identita' decisa")
	}
	o = Nodo3{ChiaveBozza: *n.ChiaveBozza, ComponenteID: n.ComponenteID, Op: *n.Op, Tipo: *n.Tipo, Origine: *n.Origine}
	if n.Descrizione != nil {
		o.Descrizione = nfc(*n.Descrizione) // testo umano: NFC prima dei limiti
	}
	for j, c := range n.Caselle {
		if c.AllegatoID == nil || c.Gruppo == nil || c.Rif == nil {
			return o, corpo(fmt.Sprintf("%s.caselle[%d]", campo, j), "una casella vuole allegato_id, gruppo e rif")
		}
		o.Caselle = append(o.Caselle, Casella3{AllegatoID: *c.AllegatoID, Gruppo: *c.Gruppo, Rif: *c.Rif})
	}
	id := n.Identita
	if id.Emittente == nil || id.Modo == nil {
		return o, corpo(campo+".identita", "l'identita' vuole emittente e modo")
	}
	// l'identita': byte per byte, mai NFC (EM §3.10)
	o.Identita = Identita{Emittente: *id.Emittente, Namespace: valore(id.Namespace), Codice: valore(id.Codice), Rev: valore(id.Rev),
		Modo: *id.Modo, Decisione: valore(id.Decisione)}
	if id.EmittentePresunto != nil {
		o.Identita.EmittentePresunto = *id.EmittentePresunto
	}
	return o, nil
}

func (a arco3In) converti(campo string) (Arco3, *Errore) {
	var o Arco3
	if a.Padre == nil || a.Figlio == nil {
		return o, corpo(campo, "un arco vuole padre e figlio")
	}
	if a.Qta == nil {
		return o, corpoMotivo(campo+".qta", MotivoArcoSenzaQuantita, "l'arco %s → %s non ha la quantita': non si mette 1 da soli", *a.Padre, *a.Figlio)
	}
	if a.Origine == nil {
		return o, corpo(campo+".origine", "manca l'origine dell'arco")
	}
	o = Arco3{Padre: *a.Padre, Figlio: *a.Figlio, Qta: *a.Qta, Origine: *a.Origine}
	if a.Posizione != nil {
		o.Posizione = nfc(*a.Posizione)
	}
	if a.Step != nil {
		if a.Step.AllegatoID == nil || a.Step.Gruppo == nil {
			return o, corpo(campo+".step", "lo step di un arco vuole allegato_id e gruppo")
		}
		o.Step = &StepArco3{AllegatoID: *a.Step.AllegatoID, Gruppo: *a.Step.Gruppo}
	}
	return o, nil
}

// ------------------------------------------------------------------ la regola dell'emittente

// EmittenteAmmissibile e' la regola unica della forma dell'emittente (EM §3.4.2, §5.6; PM23 §4.6, T-14):
// «cliente:<uuid>» con l'UUID in forma canonica minuscola, oppure «partner:<id>» con l'id come lo vuole la grammatica
// delle regole (non vuoto, minuscolo, senza spazi, «:» o «/»: grammatica/partner.go) e al piu' MaxIDPartner caratteri, il
// tetto di sicurezza del CHECK ck_csc_identita. Quale emittente sia ammesso per una bozza (il cliente della RFQ, un
// partner attivo nel registro citato, la certificazione valida del componente) lo decide chi chiama.
func EmittenteAmmissibile(e string) bool {
	if resto, ok := strings.CutPrefix(e, "cliente:"); ok {
		return reUUIDMinuscolo.MatchString(resto)
	}
	id, ok := strings.CutPrefix(e, "partner:")
	if !ok {
		return false
	}
	return strings.TrimSpace(id) != "" && id == strings.ToLower(id) && !strings.ContainsAny(id, " :/") &&
		utf8.RuneCountInString(id) <= MaxIDPartner
}

// NamespaceAmmissibile: la regola della grammatica (un testo non vuoto: grammatica/valida.go) con il tetto di sicurezza
// del CHECK ck_csc_identita.
func NamespaceAmmissibile(n string) bool {
	return strings.TrimSpace(n) != "" && utf8.RuneCountInString(n) <= MaxNamespace
}

// CasoT14 e' un caso della prova T-14 (PM23 §11): gli stessi casi danno lo stesso esito al validatore delle grammatiche,
// alle funzioni Go e al CHECK ck_csc_identita. Sono qui, e non in un file di prova, perche' la parte L4 della prova
// (platform/migrazioni) usi gli stessi.
type CasoT14 struct {
	Nome      string
	Partner   string // l'id dopo «partner:»; vuoto se il caso e' solo sul namespace
	Namespace string // vuoto se il caso e' solo sull'emittente
	// Atteso: l'esito di EmittenteAmmissibile o NamespaceAmmissibile, e del CHECK.
	Atteso bool
	// OltreIlTetto: oltre il tetto di sicurezza della 0023, che la grammatica non ha: la grammatica accetta, il Go e il
	// CHECK no (la sola differenza dichiarata, EM §3.4.2).
	OltreIlTetto bool
	// MaiuscolaNonASCII: il CHECK confronta con lower() del database, che conosce le maiuscole non ASCII solo con una
	// LC_CTYPE che le conosce (non con C): su un database con LC_CTYPE C il CHECK accetta quello che il Go rifiuta.
	MaiuscolaNonASCII bool
}

// CasiT14 restituisce i casi di T-14: quelli elencati dal piano (partner:beta, partner:beta+co, partner:Beta, partner:be
// ta, partner:a/b, un id lungo, un namespace di 70 caratteri, lettere non ASCII) e i bordi dei tetti.
func CasiT14() []CasoT14 {
	return []CasoT14{
		{Nome: "beta", Partner: "beta", Atteso: true},
		{Nome: "beta+co", Partner: "beta+co", Atteso: true},
		{Nome: "Beta", Partner: "Beta"},
		{Nome: "be ta", Partner: "be ta"},
		{Nome: "a/b", Partner: "a/b"},
		{Nome: "a:b", Partner: "a:b"},
		{Nome: "id di 200 caratteri", Partner: strings.Repeat("b", MaxIDPartner), Atteso: true},
		{Nome: "id di 201 caratteri", Partner: strings.Repeat("b", MaxIDPartner+1), OltreIlTetto: true},
		{Nome: "lettere non ASCII minuscole", Partner: "è-uno", Atteso: true},
		{Nome: "lettere non ASCII maiuscole", Partner: "È-uno", MaiuscolaNonASCII: true},
		{Nome: "namespace di 70 caratteri", Namespace: strings.Repeat("n", 70), Atteso: true},
		{Nome: "namespace di 500 caratteri", Namespace: strings.Repeat("n", MaxNamespace), Atteso: true},
		{Nome: "namespace di 501 caratteri", Namespace: strings.Repeat("n", MaxNamespace+1), OltreIlTetto: true},
		{Nome: "namespace di soli spazi", Namespace: "   "},
		{Nome: "namespace non ASCII", Namespace: "acmè", Atteso: true},
	}
}

// EmittenteCliente: l'emittente del cliente della RFQ, esplicito (EM §3.4.2).
func EmittenteCliente(cliente uuid.UUID) string { return "cliente:" + cliente.String() }

// PresunzioneAttesa: la presunzione dell'emittente che il server calcola per i modi in cui non dipende dal database
// (EM §3.4.2): falsa per proposta, vera per manuale e formato_2. Per decisione dipende dalla decisione (falsa, salvo il
// codice manuale adottato con riferimento manuale_legacy: «emittente il cliente, presunto», §3.6.1); per registrato e
// da_decidere decidono la certificazione del componente o la regola del cliente (ok = false).
func PresunzioneAttesa(modo string) (presunto, ok bool) {
	switch modo {
	case ModoProposta:
		return false, true
	case ModoManuale, ModoFormato2:
		return true, true
	}
	return false, false
}

// ------------------------------------------------------------------ la validazione della forma

func (s *Sorgente3) valida(campo string) *Errore {
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
	if s.Importazione != "" {
		if !reSha256.MatchString(s.Importazione) {
			return corpo(campo+".importazione", "l'impronta del registro e' di 64 cifre esadecimali minuscole")
		}
		if s.Gruppo == "senza" {
			return corpo(campo+".gruppo", "il gruppo «senza» non si importa")
		}
		if s.AllegatoID == nil {
			return corpo(campo+".allegato_id", "un registro ha la sua fonte: la sorgente vuole allegato_id e sha256")
		}
	}
	return nil
}

// validaIdentita: la forma dell'identita' decisa di un nodo, senza il database e senza le decisioni (EM §3.4.2).
func validaIdentita(campo string, n Nodo3) *Errore {
	id := n.Identita
	c := campo + ".identita"
	if !EmittenteAmmissibile(id.Emittente) {
		return corpo(c+".emittente", "emittente %q: cliente:<uuid> oppure partner:<id> minuscolo, senza spazi, «:» o «/», al piu' %d caratteri",
			id.Emittente, MaxIDPartner)
	}
	if id.Namespace != "" && !NamespaceAmmissibile(id.Namespace) {
		return corpo(c+".namespace", "il namespace e' un testo non vuoto di al piu' %d caratteri", MaxNamespace)
	}
	if !modiIdentita[id.Modo] {
		return corpo(c+".modo", "modo sconosciuto %q (proposta, decisione, registrato, manuale, da_decidere, formato_2)", id.Modo)
	}
	if (id.Modo == ModoDecisione) != (id.Decisione != "") {
		return corpo(c+".decisione", "la decisione c'e' se e solo se il modo e' decisione")
	}
	if id.Decisione != "" && !reChiave.MatchString(id.Decisione) {
		return corpo(c+".decisione", "l'id di una decisione e' 1-64 caratteri fra lettere, cifre e . _ : -")
	}
	if p, ok := PresunzioneAttesa(id.Modo); ok && p != id.EmittentePresunto {
		return corpoMotivo(c+".emittente_presunto", MotivoEmittentePresuntoDiverso,
			"con il modo %s la presunzione dell'emittente e' %v: la calcola il server", id.Modo, p)
	}
	switch id.Modo {
	case ModoProposta:
		if len(n.Caselle) == 0 {
			return corpo(c+".modo", "il modo proposta e' di un nodo con la sua casella")
		}
	case ModoDaDecidere:
		if len(n.Caselle) == 0 {
			return corpo(c+".modo", "il modo da_decidere e' di un nodo con la sua casella")
		}
		if n.Op != OpCrea || id.Codice != "" {
			return corpo(c+".codice", "un nodo da decidere e' un nodo da creare, ancora senza codice")
		}
	case ModoManuale:
		if len(n.Caselle) > 0 || n.Op != OpCrea {
			return corpo(c+".modo", "il modo manuale e' di un nodo da creare senza casella")
		}
		if !strings.HasPrefix(id.Emittente, "cliente:") || id.Namespace != "" {
			return corpo(c+".emittente", "un nodo scritto a mano ha l'emittente del cliente, presunto, senza namespace (EM §5.6)")
		}
	case ModoRegistrato:
		if n.Op == OpCrea {
			return corpo(c+".modo", "il modo registrato e' di un componente che c'e' (riusa, modifica, esclude)")
		}
	case ModoFormato2:
		if !strings.HasPrefix(id.Emittente, "cliente:") {
			return corpo(c+".emittente", "un nodo venuto dal formato 2 ha l'emittente del cliente, presunto")
		}
	}
	switch {
	case id.Codice == "":
		if n.Op != OpCrea {
			return corpo(c+".codice", "op %s vuole il codice del componente", n.Op)
		}
	case n.Op == OpCrea:
		if !classificazione.CodiceAmmissibile(id.Codice) {
			return corpo(c+".codice", "un codice nuovo e' senza spazi e al piu' %d caratteri", classificazione.MaxCodice)
		}
	default:
		if !codiceRegistrato(id.Codice) {
			return corpo(c+".codice", "op %s vuole il codice del componente, al piu' %d caratteri", n.Op, MaxCodiceRegistrato)
		}
	}
	if id.Rev != "" && !revDellaWorking(id.Rev) {
		return corpo(c+".rev", "revisione con spazi, minuscole o oltre %d caratteri: e' la rev della working", classificazione.MaxRev)
	}
	return nil
}

func validaNodo3(campo string, n Nodo3, sorgente *Sorgente3) *Errore {
	if !reChiave.MatchString(n.ChiaveBozza) {
		return corpo(campo+".chiave_bozza", "la chiave e' 1-64 caratteri fra lettere, cifre e . _ : -")
	}
	switch n.Op {
	case OpCrea:
		if n.ComponenteID != nil {
			return corpo(campo+".componente_id", "un nodo da creare non ha componente_id: l'UUID lo da' la conferma")
		}
	case OpRiusa, OpModifica, OpEsclude:
		if n.ComponenteID == nil || *n.ComponenteID == uuid.Nil {
			return corpo(campo+".componente_id", "op %s vuole il componente_id esistente", n.Op)
		}
	default:
		return corpo(campo+".op", "op sconosciuta %q (crea, riusa, modifica, esclude)", n.Op)
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
	if n.Origine == OrigineStep && len(n.Caselle) == 0 {
		return corpo(campo+".caselle", "un nodo con origine step ha la sua casella")
	}
	if len(n.Caselle) > MaxVoci {
		return corpo(campo, "troppe caselle (al piu' %d)", MaxVoci)
	}
	viste := map[Casella3]bool{}
	for j, c := range n.Caselle {
		cc := fmt.Sprintf("%s.caselle[%d]", campo, j)
		if e := validaCasella3(cc, c, sorgente); e != nil {
			return e
		}
		if viste[c] {
			return corpo(cc, "casella ripetuta nello stesso nodo")
		}
		viste[c] = true
	}
	return validaIdentita(campo, n)
}

// validaCasella3: la forma di una casella (EM §3.4.1) e il legame con la sorgente: nel formato 3 di V1 ogni casella ha
// l'occorrenza e il gruppo della sorgente, e il rif e' dello sha256 della sorgente (EM §3.3).
func validaCasella3(campo string, c Casella3, sorgente *Sorgente3) *Errore {
	if e := validaCasellaForma(campo, c); e != nil {
		return e
	}
	if sorgente == nil {
		return corpo("sorgente", "una casella STEP vuole la sorgente (allegato, sha256, gruppo)")
	}
	if sorgente.AllegatoID == nil || *sorgente.AllegatoID != c.AllegatoID || sorgente.Gruppo != c.Gruppo {
		return corpoMotivo(campo, MotivoRifFuoriImportazione, "la casella non ha l'occorrenza e il gruppo della sorgente (%s)", sorgente.Gruppo)
	}
	if c.Rif[5:69] != sorgente.Sha256 {
		return corpoMotivo(campo+".rif", MotivoRifFuoriImportazione, "il rif non e' dello sha256 della sorgente")
	}
	return nil
}

func validaArco3(campo string, a Arco3, sorgente *Sorgente3) *Errore {
	if !reChiave.MatchString(a.Padre) || !reChiave.MatchString(a.Figlio) {
		return corpo(campo, "padre e figlio sono chiavi della bozza")
	}
	if a.Padre == a.Figlio {
		return corpo(campo, "un arco da %s a se stesso", a.Padre)
	}
	if a.Qta < 1 || a.Qta > MaxQta {
		return corpoMotivo(campo+".qta", MotivoArcoSenzaQuantita, "quantita' %d fuori da 1…%d", a.Qta, MaxQta)
	}
	if !origini[a.Origine] {
		return corpo(campo+".origine", "origine sconosciuta %q (step, utente, conferma, registrato)", a.Origine)
	}
	if runeOltre(a.Posizione, MaxPosizione) || haControlli(a.Posizione) {
		return corpo(campo+".posizione", "posizione oltre %d caratteri", MaxPosizione)
	}
	if (a.Origine == OrigineStep) != (a.Step != nil) {
		return corpo(campo+".step", "lo step c'e' se e solo se l'origine dell'arco e' step")
	}
	if a.Step != nil {
		if a.Step.AllegatoID == uuid.Nil || !testoLibero(a.Step.Gruppo, MaxGruppo) {
			return corpo(campo+".step", "lo step vuole l'allegato e il gruppo")
		}
		if sorgente == nil || sorgente.AllegatoID == nil || *sorgente.AllegatoID != a.Step.AllegatoID || sorgente.Gruppo != a.Step.Gruppo {
			return corpoMotivo(campo+".step", MotivoRifFuoriImportazione, "lo step dell'arco non ha l'occorrenza e il gruppo della sorgente")
		}
	}
	return nil
}

func (s *Snapshot3) valida() *Errore {
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
	nodi := map[string]Nodo3{}
	rifDi := map[string]string{}
	radici, proposte := 0, 0
	var radice Nodo3
	for i, n := range s.Nodi {
		campo := fmt.Sprintf("nodi[%d]", i)
		if e := validaNodo3(campo, n, s.Sorgente); e != nil {
			return e
		}
		if _, dup := nodi[n.ChiaveBozza]; dup {
			return corpo(campo+".chiave_bozza", "chiave %q ripetuta", n.ChiaveBozza)
		}
		nodi[n.ChiaveBozza] = n
		if n.ComponenteID != nil && *n.ComponenteID == s.ProdottoID {
			radici++
			radice = n
		} else if n.Tipo == string(db.TipoComponenteFinito) {
			return corpo(campo+".tipo", "finito solo sulla radice (il prodotto %s)", s.ProdottoID)
		}
		if n.Identita.Modo == ModoProposta {
			proposte++
		}
		// nel formato 3 tutte le caselle hanno il gruppo della sorgente: un rif sta in un nodo solo
		for _, c := range n.Caselle {
			if altro, ok := rifDi[c.Rif]; ok {
				return corpo(campo+".caselle", "il rif %s sta gia' nel nodo %s", c.Rif, altro)
			}
			rifDi[c.Rif] = n.ChiaveBozza
		}
	}
	if radici != 1 {
		return corpo("nodi", "%d nodi con il componente del prodotto %s: ne serve esattamente uno (la radice)", radici, s.ProdottoID)
	}
	if s.Sorgente != nil && s.Sorgente.Importazione != "" && len(radice.Caselle) == 0 {
		return corpo("nodi", "la radice di una bozza con un registro ha la casella della radice dello STEP (P-2)")
	}
	if proposte > 0 && (s.Sorgente == nil || s.Sorgente.Importazione == "") {
		return corpo("sorgente.importazione", "un nodo con l'identita' proposta vuole il registro della sorgente")
	}
	coppie := map[[2]string]bool{}
	for i, a := range s.Archi {
		campo := fmt.Sprintf("archi[%d]", i)
		if e := validaArco3(campo, a, s.Sorgente); e != nil {
			return e
		}
		p, ok := nodi[a.Padre]
		if !ok {
			return corpo(campo+".padre", "il padre %q non e' un nodo della bozza", a.Padre)
		}
		f, ok := nodi[a.Figlio]
		if !ok {
			return corpo(campo+".figlio", "il figlio %q non e' un nodo della bozza", a.Figlio)
		}
		if a.Step != nil && (len(p.Caselle) == 0 || len(f.Caselle) == 0) {
			return corpo(campo+".step", "un arco STEP unisce due nodi con la loro casella: e' (allegato, rif del padre, rif del figlio)")
		}
		k := [2]string{a.Padre, a.Figlio}
		if coppie[k] {
			return corpo(campo, "l'arco %s → %s e' ripetuto", a.Padre, a.Figlio)
		}
		coppie[k] = true
	}
	if e := s.validaDecisioni(nodi, rifDi); e != nil {
		return e
	}
	proposteViste := map[uuid.UUID]bool{}
	for i, p := range s.DocumentiPreassociati {
		campo := fmt.Sprintf("documenti_preassociati[%d]", i)
		if p.PropostaID == uuid.Nil {
			return corpo(campo+".proposta_id", "proposta nulla")
		}
		if proposteViste[p.PropostaID] {
			return corpo(campo+".proposta_id", "la proposta %s e' preassociata due volte", p.PropostaID)
		}
		proposteViste[p.PropostaID] = true
		n, ok := nodi[p.ChiaveBozza]
		if !ok {
			return corpo(campo+".chiave_bozza", "%q non e' un nodo della bozza", p.ChiaveBozza)
		}
		if n.Op == OpEsclude {
			return corpo(campo+".chiave_bozza", "un documento non si preassocia a un nodo escluso")
		}
	}
	return s.originiConferma()
}

// validaDecisioni: id unici, le chiavi della bozza citate, le non_includere (una per rif, mai sul rif di una casella
// della bozza: EM §7.2) e il legame fra un nodo in modo decisione e la sua decisione identita o ricodifica, con la
// stessa identita' e la presunzione che ne viene (EM §3.4.2, §3.6.1).
func (s *Snapshot3) validaDecisioni(nodi map[string]Nodo3, rifDi map[string]string) *Errore {
	perID := map[string]Decisione3{}
	rifNonInclusi := map[string]bool{}
	for i, d := range s.Decisioni {
		campo := fmt.Sprintf("decisioni[%d]", i)
		if _, dup := perID[d.ID]; dup {
			return corpo(campo+".id", "id %q ripetuto", d.ID)
		}
		perID[d.ID] = d
		if d.ChiaveBozza != "" {
			if _, ok := nodi[d.ChiaveBozza]; !ok {
				return corpo(campo+".chiave_bozza", "%q non e' un nodo della bozza", d.ChiaveBozza)
			}
		}
		switch d.Tipo {
		case DecisioneNonIncludere:
			rif := d.Casella.Rif
			if rifNonInclusi[rif] {
				return corpo(campo+".casella.rif", "il rif %s ha gia' una decisione non_includere", rif)
			}
			rifNonInclusi[rif] = true
			if altro, ok := rifDi[rif]; ok {
				return corpo(campo+".casella.rif", "il rif %s e' anche nelle caselle del nodo %s: non incluso e incluso insieme", rif, altro)
			}
			if e := validaCasella3(campo+".casella", *d.Casella, s.Sorgente); e != nil {
				return e
			}
		case DecisioneIdentita:
			n := nodi[d.ChiaveBozza]
			if n.Identita.Modo != ModoDecisione || n.Identita.Decisione != d.ID {
				return corpo(campo+".chiave_bozza", "il nodo %s non ha l'identita' decisa da %s (modo decisione)", d.ChiaveBozza, d.ID)
			}
		}
	}
	for i, n := range s.Nodi {
		if n.Identita.Modo != ModoDecisione {
			continue
		}
		campo := fmt.Sprintf("nodi[%d].identita", i)
		d, ok := perID[n.Identita.Decisione]
		if !ok || (d.Tipo != DecisioneIdentita && d.Tipo != DecisioneRicodifica) {
			return corpo(campo+".decisione", "%q non e' una decisione identita o ricodifica della bozza", n.Identita.Decisione)
		}
		id := n.Identita
		presunto := false
		switch d.Tipo {
		case DecisioneIdentita:
			x := d.Identita
			if id.Emittente != x.Emittente || id.Namespace != x.Namespace || id.Codice != x.Codice || id.Rev != x.Rev {
				return corpo(campo, "il nodo %s non porta l'identita' della decisione %s", n.ChiaveBozza, d.ID)
			}
			presunto = d.Riferimento == RiferimentoManualeLegacy
		case DecisioneRicodifica:
			if n.ComponenteID == nil || *n.ComponenteID != *d.ComponenteID {
				return corpo(campo+".decisione", "la ricodifica %s e' di un altro componente", d.ID)
			}
			if id.Codice != d.CodiceDopo || id.Emittente != d.EmittenteDopo || id.Namespace != d.NamespaceDopo {
				return corpo(campo, "il nodo %s non porta l'identita' della ricodifica %s (codice, emittente e namespace dopo)", n.ChiaveBozza, d.ID)
			}
		}
		if id.EmittentePresunto != presunto {
			return corpoMotivo(campo+".emittente_presunto", MotivoEmittentePresuntoDiverso,
				"con la decisione %s la presunzione dell'emittente e' %v: la calcola il server", d.ID, presunto)
		}
	}
	return nil
}

// originiConferma: un nodo o un arco con origine «conferma» viene da una conferma, e la base allora ha numero almeno 1.
func (s *Snapshot3) originiConferma() *Errore {
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
func (s *Snapshot3) normalizza() {
	if s.Nodi == nil {
		s.Nodi = []Nodo3{}
	}
	if s.Archi == nil {
		s.Archi = []Arco3{}
	}
	if s.Decisioni == nil {
		s.Decisioni = []Decisione3{}
	}
	if s.DocumentiPreassociati == nil {
		s.DocumentiPreassociati = []Preassociazione{}
	}
	for i := range s.Nodi {
		n := &s.Nodi[i]
		if len(n.Caselle) == 0 {
			n.Caselle = nil
		}
		sort.Slice(n.Caselle, func(a, b int) bool {
			x, y := n.Caselle[a], n.Caselle[b]
			if x.Gruppo != y.Gruppo {
				return x.Gruppo < y.Gruppo
			}
			if x.Rif != y.Rif {
				return x.Rif < y.Rif
			}
			return x.AllegatoID.String() < y.AllegatoID.String()
		})
	}
	sort.Slice(s.Nodi, func(i, j int) bool { return s.Nodi[i].ChiaveBozza < s.Nodi[j].ChiaveBozza })
	sort.Slice(s.Archi, func(i, j int) bool {
		if s.Archi[i].Padre != s.Archi[j].Padre {
			return s.Archi[i].Padre < s.Archi[j].Padre
		}
		return s.Archi[i].Figlio < s.Archi[j].Figlio
	})
	sort.Slice(s.Decisioni, func(i, j int) bool { return s.Decisioni[i].ID < s.Decisioni[j].ID })
	sort.Slice(s.DocumentiPreassociati, func(i, j int) bool {
		return s.DocumentiPreassociati[i].PropostaID.String() < s.DocumentiPreassociati[j].PropostaID.String()
	})
}

// Canonico restituisce i byte canonici dello snapshot (normalizzato prima): quelli che si salvano, che R1 restituisce e
// di cui si calcola l'impronta.
func (s Snapshot3) Canonico() ([]byte, error) {
	c := s.copia()
	c.normalizza()
	return jsoncanonico.Codifica(c)
}

// Impronta e' lo sha256 dei byte canonici (distinta_bozza_versione.impronta, conferma_strutturale.impronta_snapshot).
func (s Snapshot3) Impronta() (string, error) {
	b, err := s.Canonico()
	if err != nil {
		return "", err
	}
	return jsoncanonico.Impronta(b), nil
}

func (s Snapshot3) copia() Snapshot3 {
	c := s
	c.Nodi = make([]Nodo3, len(s.Nodi))
	for i, n := range s.Nodi {
		n.Caselle = append([]Casella3(nil), n.Caselle...)
		c.Nodi[i] = n
	}
	c.Archi = append([]Arco3(nil), s.Archi...)
	c.Decisioni = append([]Decisione3(nil), s.Decisioni...)
	c.DocumentiPreassociati = append([]Preassociazione(nil), s.DocumentiPreassociati...)
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

// RegistriCitati: le impronte dei registri citati dallo snapshot, la sorgente e le decisioni, senza ripetizioni e in
// ordine: e' l'insieme che conferma_strutturale.importazioni_citate deve avere (trigger della 0023, EM §3.9).
func (s Snapshot3) RegistriCitati() []string {
	visti := map[string]bool{}
	if s.Sorgente != nil && s.Sorgente.Importazione != "" {
		visti[s.Sorgente.Importazione] = true
	}
	for _, d := range s.Decisioni {
		if d.Importazione != "" {
			visti[d.Importazione] = true
		}
	}
	out := make([]string, 0, len(visti))
	for k := range visti {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
