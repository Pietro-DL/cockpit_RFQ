// Package importazione e' il registro d'importazione del Motore A (V1; emendamento di V1 al contratto rev. 3, EM §3.8):
// la proposta del Motore A per (RFQ, prodotto, gruppo), immutabile, come la conserva distinta_importazione (0023). Qui
// stanno il DTO del contenuto in nomi minuscoli, la lettura stretta, la forma canonica del registro (FormaCanonica:
// jsoncanonico, mai NFC — intervalli in byte, letterale = testo[inizio:fine] —, i vuoti non si scrivono, voci e archi in
// ordine), il limite di peso, le quattro impronte con l'elenco chiuso del §3.8.1, la coerenza con le colonne della
// testata e la copia compatta di una lettura (§3.6.1).
//
// E' una foglia: libreria standard, uuid e jsoncanonico; niente database, niente core/rfq, niente orologio. Cosi' lo
// stesso DTO serve all'adattatore del Motore A (propostabom, puro: EM §6.1), che il registro lo scrive, e alla bozza
// (distintabozza), che lo conserva e lo cita. La lista chiusa delle dipendenze dei pacchetti puri vieta oggi tutto
// core/rfq: perche' propostabom importi questa foglia serve l'eccezione per nome (proposta di emendamento nel resoconto
// della corsia persistenza di V1).
package importazione

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/jsoncanonico"
)

// Le versioni e i limiti del registro (EM §2.5, §3.10; PM23 §4.3).
const (
	// FormatoRegistro: la forma del contenuto (distinta_importazione.formato_registro, CHECK ck_dimp_formato).
	FormatoRegistro = 1
	// VersioneImportazione1: le regole dell'adattatore e dell'elenco chiuso delle impronte di questo file. Un campo
	// nuovo del registro, o un cambio dell'elenco chiuso, vuole importazione-2 (EM §3.8.1).
	VersioneImportazione1 = "importazione-1"
	// MaxContenuto: 12 MiB di testo canonico; il database ha 16 MiB come jsonb (ck_dimp_contenuto). Oltre: 422
	// IMPORTAZIONE_TROPPO_GRANDE, niente scritto, mai una troncatura (EM §3.10).
	MaxContenuto = 12 << 20
	// MaxGruppo e MaxRif: i tetti delle colonne gruppo e fonte_radice (ck_dimp_gruppo, ck_dimp_fonte).
	MaxGruppo = 300
	MaxRif    = 400
	// MaxAllegatiCitati: il tetto di ck_dimp_citati.
	MaxAllegatiCitati = 10000
	// MaxCifre: cifre di un numero intero del contenuto (come il formato 2: jsonb rifiuta oltre 131 072 cifre).
	MaxCifre = 1000
	// MaxCodiceProposto e MaxRevProposta: i tetti di classificazione.MaxCodice e MaxRev (byte), le regole del codice e
	// della rev di partenza per la working (EM §3.8, §6.1; CodiceAmmissibile, RevAmmissibile qui sotto).
	MaxCodiceProposto = 40
	MaxRevProposta    = 10
)

// I codici che il registro restituisce a chi lo scrive o lo legge (EM §6.1, §3.8.2). Un difetto di forma senza codice e'
// un difetto di chi ha scritto il registro (l'adattatore), non della richiesta: chi chiama lo tratta come errore interno.
const (
	CodiceTroppoGrande           = "IMPORTAZIONE_TROPPO_GRANDE"
	CodicePropostaNonDisponibile = "PROPOSTA_NON_DISPONIBILE"
	MotivoRegistroIncompleto     = "registro_incompleto"
)

// Errore e' un rifiuto del registro: Codice vuoto = forma.
type Errore struct {
	Codice    string
	Motivo    string
	Campo     string
	Messaggio string
}

func (e *Errore) Error() string {
	testa := "registro d'importazione"
	if e.Codice != "" {
		testa = e.Codice
		if e.Motivo != "" {
			testa += "/" + e.Motivo
		}
	}
	if e.Campo != "" {
		return fmt.Sprintf("%s (%s): %s", testa, e.Campo, e.Messaggio)
	}
	return testa + ": " + e.Messaggio
}

func forma(campo, formato string, a ...any) *Errore {
	return &Errore{Campo: campo, Messaggio: fmt.Sprintf(formato, a...)}
}

func incompleto(campo, formato string, a ...any) *Errore {
	return &Errore{Codice: CodicePropostaNonDisponibile, Motivo: MotivoRegistroIncompleto, Campo: campo, Messaggio: fmt.Sprintf(formato, a...)}
}

// ErrRegistroIncoerente: un registro salvato non si rilegge, o non ha piu' le impronte o le colonne con cui e' stato
// scritto. E' un guasto dei dati salvati, non un rifiuto.
var ErrRegistroIncoerente = errors.New("registro d'importazione salvato non coerente")

var (
	reUUID          = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	reSha256        = regexp.MustCompile(`^[0-9a-f]{64}$`)
	reRif           = regexp.MustCompile(`^nodo:[0-9a-f]{64}:.+$`)
	reVersion       = regexp.MustCompile(`^importazione-[1-9][0-9]*$`)
	reIntero        = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
	reChiaveLettura = regexp.MustCompile(`^([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})/.+$`)
)

// CodiceAmmissibile e RevAmmissibile: le regole di classificazione.CodiceAmmissibile e RevAmmissibile (non vuoto, al
// piu' MaxCodiceProposto o MaxRevProposta byte, senza spazi, controlli o caratteri di formato), ripetute qui perche' la
// foglia non importa classificazione; la prova L1 di distintabozza TestV1LeRegoleDelCodiceUgualiAClassificazione le
// tiene uguali. Il registro le applica a codice_proposto e a rev_proposta, che diventano il codice e la rev di un nodo
// proposta (EM §3.4.2, §3.8, §6.1).
func CodiceAmmissibile(s string) bool {
	return s != "" && len(s) <= MaxCodiceProposto && !strings.ContainsFunc(s, spazioOControllo)
}

// RevAmmissibile: vedi CodiceAmmissibile.
func RevAmmissibile(s string) bool {
	return s != "" && len(s) <= MaxRevProposta && !strings.ContainsFunc(s, spazioOControllo)
}

func spazioOControllo(r rune) bool {
	return r <= ' ' || unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
}

func conMinuscoleASCII(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r >= 'a' && r <= 'z' })
}

// ------------------------------------------------------------------ il contenuto

// Contenuto e' il contenuto del registro (EM §3.8, «Contenuto»). I campi che servono fuori dal motore sono tipizzati; il
// resto e' json.RawMessage, copiato byte per byte. I nomi dei campi di ogni livello tipizzato sono un elenco chiuso:
// un campo che non sta qui fa fallire la lettura (e la prova dell'elenco chiuso, EM §3.8.1).
type Contenuto struct {
	FormatoRegistro      int             `json:"formato_registro"`
	VersioneImportazione string          `json:"versione_importazione"`
	ThreadID             string          `json:"thread_id"`
	ProdottoID           string          `json:"prodotto_id"`
	Gruppo               Gruppo          `json:"gruppo"`
	Regole               Regole          `json:"regole"`
	Calcolo              json.RawMessage `json:"calcolo,omitempty"`
	Contesto             json.RawMessage `json:"contesto,omitempty"`
	Proposta             Proposta        `json:"proposta"`
	AllegatiCitati       []string        `json:"allegati_citati"`
	ImprontaContenuto    string          `json:"impronta_contenuto"`
	ImprontaEvidenze     string          `json:"impronta_evidenze"`
	ImprontaRegole       string          `json:"impronta_regole"`
}

// Gruppo: il gruppo importato (StrutturaProdotto). Nel confronto entrano solo radice, grafo_completo e motivo_grafo.
type Gruppo struct {
	Chiave         string          `json:"chiave"`
	Tipo           json.RawMessage `json:"tipo,omitempty"`
	AllegatoID     string          `json:"allegato_id,omitempty"`
	Sha256         string          `json:"sha256,omitempty"`
	Radice         string          `json:"radice,omitempty"`
	NomeFile       json.RawMessage `json:"nome_file,omitempty"`
	StatoStruttura json.RawMessage `json:"stato_struttura,omitempty"`
	RadiceDelFile  json.RawMessage `json:"radice_del_file,omitempty"`
	Compatibilita  json.RawMessage `json:"compatibilita,omitempty"`
	Origine        json.RawMessage `json:"origine,omitempty"`
	GrafoCompleto  json.RawMessage `json:"grafo_completo,omitempty"`
	MotivoGrafo    json.RawMessage `json:"motivo_grafo,omitempty"`
}

// Regole: versioni, grammatica del cliente, partner attivi, contesto (EM §3.8; impronta_regole).
type Regole struct {
	Versioni          Versioni           `json:"versioni"`
	GrammaticaCliente *GrammaticaCliente `json:"grammatica_cliente,omitempty"`
	Partner           *[]PartnerAttivo   `json:"partner,omitempty"`
	Contesto          *ContestoRegole    `json:"contesto,omitempty"`
}

// Versioni: le versioni del motore e dell'adattatore. Le tre che servono fuori dal motore sono testi; le altre restano
// com'erano.
type Versioni struct {
	Algoritmo    json.RawMessage `json:"algoritmo,omitempty"`
	Router       json.RawMessage `json:"router,omitempty"`
	Risultato    json.RawMessage `json:"risultato,omitempty"`
	Ancoraggio   string          `json:"ancoraggio,omitempty"`
	Valutazione  json.RawMessage `json:"valutazione,omitempty"`
	Composizione json.RawMessage `json:"composizione,omitempty"`
	Importazione string          `json:"importazione,omitempty"`
	Canonico     string          `json:"canonico,omitempty"`
	Limiti       json.RawMessage `json:"limiti,omitempty"`
}

// GrammaticaCliente: la grammatica del cliente con il suo hash; assente per un thread senza grammatica (EM §6.1).
type GrammaticaCliente struct {
	ClienteID string `json:"cliente_id"`
	Hash      string `json:"hash"`
}

// PartnerAttivo: un partner attivo nel contesto, con l'hash della sua grammatica (regola unica dell'emittente, EM §5.6).
type PartnerAttivo struct {
	Emittente string `json:"emittente"`
	Hash      string `json:"hash"`
}

// ContestoRegole: il contesto di lavorazione con l'impronta che copre anche i contesti ereditati.
type ContestoRegole struct {
	ID       json.RawMessage `json:"id"`
	Impronta string          `json:"impronta"`
}

// Proposta: la proposta del Motore A per il gruppo.
type Proposta struct {
	Fonte             FonteProposta       `json:"fonte"`
	Nodi              []Voce              `json:"nodi,omitempty"`
	Archi             []ArcoRegistro      `json:"archi,omitempty"`
	File              []FileRegistro      `json:"file,omitempty"`
	Relazioni         []Relazione         `json:"relazioni,omitempty"`
	FuoriPerDecisione []FuoriPerDecisione `json:"fuori_per_decisione,omitempty"`
	Diagnostiche      json.RawMessage     `json:"diagnostiche,omitempty"`
}

// FonteProposta: l'occorrenza della fonte, con il nome ricevuto.
type FonteProposta struct {
	AllegatoID       string          `json:"allegato_id,omitempty"`
	Sha256           string          `json:"sha256,omitempty"`
	NomeRicevuto     json.RawMessage `json:"nome_ricevuto,omitempty"`
	ContenitoreID    json.RawMessage `json:"contenitore_id,omitempty"`
	PercorsoRicevuto json.RawMessage `json:"percorso_ricevuto,omitempty"`
}

// Voce e' la voce di un nodo (EM §3.8, «Voce di un nodo»): la chiave n:<allegato_id>:<rif>.
type Voce struct {
	Voce                 string          `json:"voce"`
	Origine              OrigineVoce     `json:"origine"`
	EntitaID             json.RawMessage `json:"entita_id,omitempty"`
	Padri                json.RawMessage `json:"padri,omitempty"`
	Foglia               json.RawMessage `json:"foglia,omitempty"`
	RadiceDelFile        json.RawMessage `json:"radice_del_file,omitempty"`
	Leggibile            json.RawMessage `json:"leggibile,omitempty"`
	StatoMotore          StatoMotore     `json:"stato_motore"`
	Grezzi               json.RawMessage `json:"grezzi,omitempty"`
	IdentitaProposta     json.RawMessage `json:"identita_proposta,omitempty"`
	CodiceProposto       string          `json:"codice_proposto,omitempty"`
	MotivoProposto       json.RawMessage `json:"motivo_proposto,omitempty"`
	RevProposta          string          `json:"rev_proposta,omitempty"`
	MotivoRev            json.RawMessage `json:"motivo_rev,omitempty"`
	CodiceComposto       json.RawMessage `json:"codice_composto,omitempty"`
	MotivoLettura        json.RawMessage `json:"motivo_lettura,omitempty"`
	Alternative          json.RawMessage `json:"alternative,omitempty"`
	Riferimenti          json.RawMessage `json:"riferimenti,omitempty"`
	CodiceManualeLegacy  json.RawMessage `json:"codice_manuale_legacy,omitempty"`
	Attributi            json.RawMessage `json:"attributi,omitempty"`
	TipoProposto         json.RawMessage `json:"tipo_proposto,omitempty"`
	Segnali              json.RawMessage `json:"segnali,omitempty"`
	Diagnostiche         json.RawMessage `json:"diagnostiche,omitempty"`
	DecisioneEsistente   json.RawMessage `json:"decisione_esistente,omitempty"`
	AbbinamentoProposto  json.RawMessage `json:"abbinamento_proposto,omitempty"`
	AssociazioniProposte json.RawMessage `json:"associazioni_proposte,omitempty"`
	FabbisogniPrevisti   json.RawMessage `json:"fabbisogni_previsti_all_importazione,omitempty"`
	RigaLegacy           json.RawMessage `json:"riga_legacy,omitempty"`
}

// OrigineVoce: la chiave d'origine del nodo (allegato, sha256, chiave PRODUCT, rif).
type OrigineVoce struct {
	AllegatoID string `json:"allegato_id"`
	Sha256     string `json:"sha256"`
	Chiave     string `json:"chiave"`
	Rif        string `json:"rif"`
}

// StatoMotore: raggiunto e tolto sono struttura (nel confronto), deciso e' un'evidenza delle righe legacy.
type StatoMotore struct {
	Raggiunto *bool `json:"raggiunto,omitempty"`
	Tolto     *bool `json:"tolto,omitempty"`
	Deciso    *bool `json:"deciso,omitempty"`
}

// ArcoRegistro: un arco della struttura del motore, fra due rif dell'occorrenza.
type ArcoRegistro struct {
	AllegatoID string          `json:"allegato_id"`
	Padre      string          `json:"padre"`
	Figlio     string          `json:"figlio"`
	Quantita   *int            `json:"quantita,omitempty"`
	Occorrenze json.RawMessage `json:"occorrenze,omitempty"`
	Altre      json.RawMessage `json:"altre,omitempty"`
}

// FileRegistro: un file della fonte o con un candidato fra i nodi del gruppo (mai un'occorrenza non attiva).
type FileRegistro struct {
	Voce             json.RawMessage `json:"voce,omitempty"`
	AllegatoID       string          `json:"allegato_id"`
	Sha256           string          `json:"sha256,omitempty"`
	NomeRicevuto     json.RawMessage `json:"nome_ricevuto,omitempty"`
	PercorsoRicevuto json.RawMessage `json:"percorso_ricevuto,omitempty"`
	ContenitoreID    json.RawMessage `json:"contenitore_id,omitempty"`
	Origine          json.RawMessage `json:"origine,omitempty"`
	Interpretazione  json.RawMessage `json:"interpretazione,omitempty"`
	Letture          json.RawMessage `json:"letture,omitempty"`
	Attributi        json.RawMessage `json:"attributi,omitempty"`
	Candidati        json.RawMessage `json:"candidati,omitempty"`
}

// Relazione: una relazione semantica del motore; la sorgente dice se va nel confronto (nodo) o fra le evidenze (file).
type Relazione struct {
	Voce             json.RawMessage   `json:"voce,omitempty"`
	IDMotore         json.RawMessage   `json:"id_motore,omitempty"`
	Tipo             json.RawMessage   `json:"tipo,omitempty"`
	Stato            json.RawMessage   `json:"stato,omitempty"`
	Modello          json.RawMessage   `json:"modello,omitempty"`
	EntitaSorgente   json.RawMessage   `json:"entita_sorgente,omitempty"`
	Riconoscimento   json.RawMessage   `json:"riconoscimento,omitempty"`
	Sorgente         SorgenteRelazione `json:"sorgente"`
	LetturaSorgente  json.RawMessage   `json:"lettura_sorgente,omitempty"`
	LettureBersaglio json.RawMessage   `json:"letture_bersaglio,omitempty"`
	Evidenze         json.RawMessage   `json:"evidenze,omitempty"`
}

// SorgenteRelazione: {tipo: nodo | file, voce}.
type SorgenteRelazione struct {
	Tipo string          `json:"tipo"`
	Voce json.RawMessage `json:"voce,omitempty"`
}

// FuoriPerDecisione: un nodo tolto da una persona o non raggiunto per archi tolti, con la traccia.
type FuoriPerDecisione struct {
	Voce   string          `json:"voce"`
	Segno  string          `json:"segno"`
	Chi    json.RawMessage `json:"chi,omitempty"`
	Quando json.RawMessage `json:"quando,omitempty"`
	Motivo json.RawMessage `json:"motivo,omitempty"`
}

// I segni di fuori_per_decisione[].
const (
	SegnoTolto        = "tolto"
	SegnoNonRaggiunto = "non_raggiunto"
)

// ------------------------------------------------------------------ la testata

// Testata: le colonne di distinta_importazione (PM23 §4.3).
type Testata struct {
	ImportazioneID       uuid.UUID
	ThreadID             uuid.UUID
	ProdottoID           uuid.UUID
	Impronta             string
	ImprontaContenuto    string
	ImprontaEvidenze     string
	ImprontaRegole       string
	FormatoRegistro      int
	VersioneImportazione string
	Gruppo               string
	FonteAllegatoID      *uuid.UUID
	FonteSha256          string
	FonteRadice          string
	AllegatiCitati       []uuid.UUID
	CreataDa             uuid.UUID
	CreataIl             time.Time
}

// Registro e' un registro letto o composto: il contenuto, i suoi byte canonici (quelli che si salvano) e l'impronta.
type Registro struct {
	Contenuto Contenuto
	Canonico  []byte
	Impronta  string
}

// Grezzo restituisce un campo di primo livello del contenuto con i suoi byte canonici (per esempio «regole», che R1
// restituisce com'e'), o nil se non c'e'.
func (r Registro) Grezzo(campo string) json.RawMessage {
	var m map[string]json.RawMessage
	if json.Unmarshal(r.Canonico, &m) != nil {
		return nil
	}
	return m[campo]
}

// Colonne restituisce i valori delle colonne che vengono dal contenuto (tutte salvo id, autore e data).
func (r Registro) Colonne() Testata {
	c := r.Contenuto
	t := Testata{
		ThreadID: uuid.MustParse(c.ThreadID), ProdottoID: uuid.MustParse(c.ProdottoID), Impronta: r.Impronta,
		ImprontaContenuto: c.ImprontaContenuto, ImprontaEvidenze: c.ImprontaEvidenze, ImprontaRegole: c.ImprontaRegole,
		FormatoRegistro: c.FormatoRegistro, VersioneImportazione: c.VersioneImportazione, Gruppo: c.Gruppo.Chiave,
		FonteSha256: c.Proposta.Fonte.Sha256, FonteRadice: c.Gruppo.Radice,
	}
	if c.Proposta.Fonte.AllegatoID != "" {
		u := uuid.MustParse(c.Proposta.Fonte.AllegatoID)
		t.FonteAllegatoID = &u
	}
	for _, a := range c.AllegatiCitati {
		t.AllegatiCitati = append(t.AllegatiCitati, uuid.MustParse(a))
	}
	return t
}

// VerificaColonne confronta le colonne salvate con il contenuto: le stesse regole di ck_dimp_contenuto, piu' fonte e
// allegati citati, che il CHECK non confronta. Una differenza e' ErrRegistroIncoerente.
func VerificaColonne(t Testata, r Registro) error {
	a := r.Colonne()
	diverso := func(cosa string, x, y any) error {
		return fmt.Errorf("%w: %s %v nella colonna, %v nel contenuto", ErrRegistroIncoerente, cosa, x, y)
	}
	switch {
	case t.ThreadID != a.ThreadID:
		return diverso("thread_id", t.ThreadID, a.ThreadID)
	case t.ProdottoID != a.ProdottoID:
		return diverso("prodotto_id", t.ProdottoID, a.ProdottoID)
	case strings.TrimSpace(t.Impronta) != a.Impronta:
		return diverso("impronta", t.Impronta, a.Impronta)
	case strings.TrimSpace(t.ImprontaContenuto) != a.ImprontaContenuto:
		return diverso("impronta_contenuto", t.ImprontaContenuto, a.ImprontaContenuto)
	case strings.TrimSpace(t.ImprontaEvidenze) != a.ImprontaEvidenze:
		return diverso("impronta_evidenze", t.ImprontaEvidenze, a.ImprontaEvidenze)
	case strings.TrimSpace(t.ImprontaRegole) != a.ImprontaRegole:
		return diverso("impronta_regole", t.ImprontaRegole, a.ImprontaRegole)
	case t.FormatoRegistro != a.FormatoRegistro:
		return diverso("formato_registro", t.FormatoRegistro, a.FormatoRegistro)
	case t.VersioneImportazione != a.VersioneImportazione:
		return diverso("versione_importazione", t.VersioneImportazione, a.VersioneImportazione)
	case t.Gruppo != a.Gruppo:
		return diverso("gruppo", t.Gruppo, a.Gruppo)
	case (t.FonteAllegatoID == nil) != (a.FonteAllegatoID == nil) || (t.FonteAllegatoID != nil && *t.FonteAllegatoID != *a.FonteAllegatoID):
		return diverso("fonte_allegato_id", t.FonteAllegatoID, a.FonteAllegatoID)
	case strings.TrimSpace(t.FonteSha256) != a.FonteSha256:
		return diverso("fonte_sha256", t.FonteSha256, a.FonteSha256)
	case t.FonteRadice != a.FonteRadice:
		return diverso("fonte_radice", t.FonteRadice, a.FonteRadice)
	}
	if !stessoInsieme(t.AllegatiCitati, a.AllegatiCitati) {
		return diverso("allegati_citati", t.AllegatiCitati, a.AllegatiCitati)
	}
	return nil
}

func stessoInsieme(x, y []uuid.UUID) bool {
	mx := map[uuid.UUID]bool{}
	for _, u := range x {
		mx[u] = true
	}
	my := map[uuid.UUID]bool{}
	for _, u := range y {
		my[u] = true
	}
	if len(mx) != len(my) {
		return false
	}
	for u := range mx {
		if !my[u] {
			return false
		}
	}
	return true
}

// ------------------------------------------------------------------ scrivere e leggere

// Componi chiude un contenuto scritto dall'adattatore: lo porta nella forma canonica del registro (FormaCanonica: i
// vuoti non si scrivono, nodi e archi in ordine), calcola le tre impronte parziali (§3.8.1), le scrive nel contenuto,
// ne fa i byte canonici e l'impronta intera, e controlla forma e peso come Leggi. Le impronte che il contenuto portava
// gia' si ignorano: le calcola sempre questa funzione. Per ogni contenuto c che Componi accetta, Leggi(Componi(c))
// restituisce gli stessi byte, e Componi(Leggi(x).Contenuto) gli stessi byte di Leggi(x).
func Componi(c Contenuto) (Registro, error) {
	if c.VersioneImportazione != VersioneImportazione1 {
		return Registro{}, forma("versione_importazione", "questo codice compone solo registri %s (l'elenco chiuso delle impronte e' il suo)", VersioneImportazione1)
	}
	c.ImprontaContenuto, c.ImprontaEvidenze, c.ImprontaRegole = "", "", ""
	grezzo, err := jsoncanonico.Codifica(c)
	if err != nil {
		return Registro{}, forma("contenuto", "il contenuto non ha una forma canonica: %v", err)
	}
	if e := nullVietati(grezzo); e != nil {
		return Registro{}, e
	}
	imp, err := ImpronteParziali(grezzo)
	if err != nil {
		return Registro{}, err
	}
	c.ImprontaContenuto, c.ImprontaEvidenze, c.ImprontaRegole = imp.Contenuto, imp.Evidenze, imp.Regole
	grezzo, err = jsoncanonico.Codifica(c)
	if err != nil {
		return Registro{}, forma("contenuto", "il contenuto non ha una forma canonica: %v", err)
	}
	can, err := FormaCanonica(grezzo)
	if err != nil {
		return Registro{}, forma("contenuto", "il contenuto non ha una forma canonica: %v", err)
	}
	return leggi(can)
}

// Leggi rilegge un registro (dal database o da chi lo ha appena composto): rilettura stretta, numeri interi, nessun
// U+0000, i nomi dei campi tipizzati esatti e in un elenco chiuso, nessun null dove il §3.8.2 lo vieta, la forma
// canonica del registro (FormaCanonica), le regole di forma, la completezza del §3.8.2, il peso del testo canonico, le
// impronte parziali ricalcolate (per importazione-1). Restituisce i byte canonici e l'impronta intera.
func Leggi(raw []byte) (Registro, error) {
	can, err := jsoncanonico.Codifica(json.RawMessage(raw))
	if err != nil {
		return Registro{}, forma("contenuto", "il JSON non si legge: %v", err)
	}
	if e := nullVietati(can); e != nil {
		return Registro{}, e
	}
	if can, err = FormaCanonica(can); err != nil {
		return Registro{}, forma("contenuto", "il JSON non si legge: %v", err)
	}
	return leggi(can)
}

// FormaCanonica e' la forma canonica del registro (EM §3.10; P-12 e P-19 del resoconto della corsia persistenza): un
// membro assente, null, "", [] o {} e' la stessa cosa e non si scrive, a ogni livello (gli elementi degli elenchi
// restano: la loro posizione conta), salvo regole.partner, che con un contesto attivo e nessun partner e' l'elenco vuoto
// che dice «nessuno» (§3.8.2); proposta.nodi in ordine di (origine.rif, voce) e proposta.archi in ordine di (padre,
// figlio, allegato_id), come li vuole il §3.8. Cosi' lo stesso contenuto da' sempre gli stessi byte e le stesse impronte,
// comunque l'adattatore abbia ordinato gli elenchi o scritto i vuoti: niente PROPOSTA_CAMBIATA e niente 409 di V-7 falsi.
func FormaCanonica(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	v = senzaVuoti(v, "", map[string]bool{"regole.partner": true})
	if m, ok := v.(map[string]any); ok {
		if p, ok := m["proposta"].(map[string]any); ok {
			ordinaElenco(p["nodi"], func(x map[string]any) []string {
				return []string{testoDi(oggettoDi(x["origine"])["rif"]), testoDi(x["voce"])}
			})
			ordinaElenco(p["archi"], func(x map[string]any) []string {
				return []string{testoDi(x["padre"]), testoDi(x["figlio"]), testoDi(x["allegato_id"])}
			})
		}
	}
	return jsoncanonico.Codifica(v)
}

// SenzaVuoti: un valore JSON senza i membri vuoti (null, "", [], {}), a ogni livello, come FormaCanonica senza
// eccezioni e senza ordinamenti: la regola con cui la copia compatta di una lettura (qualificatori, revisione) si
// confronta con quella del registro (V-1).
func SenzaVuoti(raw json.RawMessage) (json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return jsoncanonico.Codifica(senzaVuoti(v, "", nil))
}

func senzaVuoti(v any, percorso string, tieni map[string]bool) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, el := range x {
			p := k
			if percorso != "" {
				p = percorso + "." + k
			}
			c := senzaVuoti(el, p, tieni)
			if vuotoJSON(c) && !tieni[p] {
				continue
			}
			out[k] = c
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, el := range x {
			out[i] = senzaVuoti(el, percorso+"[]", tieni)
		}
		return out
	}
	return v
}

func vuotoJSON(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

func testoDi(v any) string {
	s, _ := v.(string)
	return s
}

// ordinaElenco ordina (stabile) un elenco di oggetti per le chiavi date, in ordine di byte.
func ordinaElenco(v any, chiavi func(map[string]any) []string) {
	l, ok := v.([]any)
	if !ok {
		return
	}
	sort.SliceStable(l, func(i, j int) bool {
		a, b := chiavi(oggettoDi(l[i])), chiavi(oggettoDi(l[j]))
		for k := range a {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return false
	})
}

// nullVietati: contesto, regole.contesto, regole.partner e regole.grammatica_cliente sono assenti o valorizzati, mai
// null (EM §3.8.2: «assenti, non null»; PM23 §10 righe 2-3 e RM-13: un null «con il motivo» vale FALLITA). Con un
// contesto attivo un null al posto dei partner o della copia del contesto e' un registro incompleto (422
// PROPOSTA_NON_DISPONIBILE, registro_incompleto); negli altri casi e' un difetto di forma di chi scrive il registro.
// Si controlla prima della forma canonica, che un null lo toglierebbe.
func nullVietati(can []byte) *Errore {
	var testa, regole map[string]json.RawMessage
	if json.Unmarshal(can, &testa) != nil {
		return nil // la forma la dice la lettura che segue
	}
	_ = json.Unmarshal(testa["regole"], &regole)
	null := func(r json.RawMessage) bool { return string(r) == "null" }
	attivo := len(regole["contesto"]) > 0 && !null(regole["contesto"])
	for _, x := range []struct {
		campo string
		r     json.RawMessage
		conIl bool // con il contesto attivo il campo e' obbligatorio
	}{{"regole.contesto", regole["contesto"], false}, {"regole.grammatica_cliente", regole["grammatica_cliente"], false},
		{"regole.partner", regole["partner"], true}, {"contesto", testa["contesto"], true}} {
		if !null(x.r) {
			continue
		}
		if x.conIl && attivo {
			return incompleto(x.campo, "con un contesto attivo %s e' obbligatorio: un null non e' ammesso (EM §3.8.2)", x.campo)
		}
		return forma(x.campo, "%s e' assente o valorizzato, mai null (EM §3.8.2)", x.campo)
	}
	return nil
}

func leggi(can []byte) (Registro, error) {
	if len(can) > MaxContenuto {
		return Registro{}, &Errore{Codice: CodiceTroppoGrande, Campo: "contenuto",
			Messaggio: fmt.Sprintf("il registro pesa %d byte di testo canonico, oltre il limite di %d: non si tronca", len(can), MaxContenuto)}
	}
	if e := valoriAmmessi(can); e != nil {
		return Registro{}, e
	}
	if e := chiaviEsatte(json.RawMessage(can), reflect.TypeFor[Contenuto](), "contenuto"); e != nil {
		return Registro{}, e
	}
	var c Contenuto
	if err := json.Unmarshal(can, &c); err != nil {
		return Registro{}, forma("contenuto", "il contenuto non ha la forma attesa: %v", err)
	}
	if e := c.valida(can); e != nil {
		return Registro{}, e
	}
	if c.VersioneImportazione == VersioneImportazione1 {
		imp, err := ImpronteParziali(can)
		if err != nil {
			return Registro{}, err
		}
		for _, x := range []struct{ nome, scritta, calcolata string }{
			{"impronta_contenuto", c.ImprontaContenuto, imp.Contenuto},
			{"impronta_evidenze", c.ImprontaEvidenze, imp.Evidenze},
			{"impronta_regole", c.ImprontaRegole, imp.Regole},
		} {
			if x.scritta != x.calcolata {
				return Registro{}, forma(x.nome, "l'impronta scritta %s non e' quella del contenuto (%s)", x.scritta, x.calcolata)
			}
		}
	}
	return Registro{Contenuto: c, Canonico: can, Impronta: jsoncanonico.Impronta(can)}, nil
}

// Rileggi rilegge un registro salvato e lo confronta con le sue colonne: un difetto e' ErrRegistroIncoerente.
func Rileggi(t Testata, contenuto []byte) (Registro, error) {
	r, err := Leggi(contenuto)
	if err != nil {
		return Registro{}, fmt.Errorf("%w: %v", ErrRegistroIncoerente, err)
	}
	if err := VerificaColonne(t, r); err != nil {
		return Registro{}, err
	}
	return r, nil
}

// valoriAmmessi scorre il JSON: numeri solo interi, senza esponente, diversi da -0 e al piu' MaxCifre cifre (jsonb
// riscriverebbe 1e2 in 100 e l'impronta alla rilettura cambierebbe); nessuna stringa con U+0000, che jsonb rifiuta.
func valoriAmmessi(can []byte) *Errore {
	dec := json.NewDecoder(bytes.NewReader(can))
	dec.UseNumber()
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return forma("contenuto", "il JSON non si legge: %v", err)
		}
		switch x := tok.(type) {
		case json.Number:
			s := string(x)
			if len(strings.TrimPrefix(s, "-")) > MaxCifre {
				return forma("contenuto", "un numero ha piu' di %d cifre", MaxCifre)
			}
			if !reIntero.MatchString(s) || s == "-0" {
				return forma("contenuto", "il numero %s non e' un intero scritto senza esponente (la forma canonica ha solo interi)", s)
			}
		case string:
			if strings.ContainsRune(x, 0) {
				return forma("contenuto", "una stringa contiene U+0000, che il database non accetta")
			}
		}
	}
}

var (
	tipoRaw  = reflect.TypeFor[json.RawMessage]()
	tipoUUID = reflect.TypeFor[uuid.UUID]()
)

// chiaviEsatte controlla i nomi dei campi di ogni livello tipizzato: scritti esattamente come i tag (encoding/json li
// confronta senza guardare le maiuscole: «Voce» passerebbe per «voce») e tutti nell'elenco del tipo. Un json.RawMessage
// e' opaco: i suoi campi sono classificati con lui (§3.8.1).
func chiaviEsatte(raw json.RawMessage, t reflect.Type, percorso string) *Errore {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == tipoRaw || t == tipoUUID || len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	switch t.Kind() {
	case reflect.Struct:
		var campi map[string]json.RawMessage
		if err := json.Unmarshal(raw, &campi); err != nil {
			return forma(percorso, "atteso un oggetto: %v", err)
		}
		tag := map[string]reflect.StructField{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			nome, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if nome != "" && nome != "-" {
				tag[nome] = f
			}
		}
		nomi := make([]string, 0, len(campi))
		for k := range campi {
			nomi = append(nomi, k)
		}
		sort.Strings(nomi)
		for _, k := range nomi {
			f, ok := tag[k]
			if !ok {
				return forma(percorso+"."+k, "campo sconosciuto: il registro ha un elenco chiuso di campi (EM §3.8.1)")
			}
			if e := chiaviEsatte(campi[k], f.Type, percorso+"."+k); e != nil {
				return e
			}
		}
	case reflect.Slice, reflect.Array:
		var elementi []json.RawMessage
		if err := json.Unmarshal(raw, &elementi); err != nil {
			return forma(percorso, "atteso un elenco: %v", err)
		}
		for i, el := range elementi {
			if e := chiaviEsatte(el, t.Elem(), fmt.Sprintf("%s[%d]", percorso, i)); e != nil {
				return e
			}
		}
	}
	return nil
}

// ------------------------------------------------------------------ le regole di forma

func (c *Contenuto) valida(can []byte) *Errore {
	if c.FormatoRegistro != FormatoRegistro {
		return forma("formato_registro", "formato del registro %d: si legge solo il %d", c.FormatoRegistro, FormatoRegistro)
	}
	if !reVersion.MatchString(c.VersioneImportazione) {
		return forma("versione_importazione", "versione %q: importazione-<n>", c.VersioneImportazione)
	}
	if !reUUID.MatchString(c.ThreadID) || !reUUID.MatchString(c.ProdottoID) {
		return forma("thread_id", "thread_id e prodotto_id sono UUID in forma canonica minuscola")
	}
	g := c.Gruppo
	if strings.TrimSpace(g.Chiave) == "" || len([]rune(g.Chiave)) > MaxGruppo || g.Chiave == "senza" {
		return forma("gruppo.chiave", "il gruppo e' obbligatorio, al piu' %d caratteri, mai «senza»", MaxGruppo)
	}
	f := c.Proposta.Fonte
	if (f.AllegatoID == "") != (f.Sha256 == "") {
		return forma("proposta.fonte", "allegato_id e sha256 della fonte vanno insieme")
	}
	if f.AllegatoID != "" && (!reUUID.MatchString(f.AllegatoID) || !reSha256.MatchString(f.Sha256)) {
		return forma("proposta.fonte", "allegato_id UUID canonico e sha256 di 64 cifre esadecimali minuscole")
	}
	if (f.AllegatoID == "") != (g.Radice == "") {
		return forma("gruppo.radice", "la radice del gruppo c'e' se e solo se c'e' la fonte (ck_dimp_fonte)")
	}
	if g.Radice != "" && (!reRif.MatchString(g.Radice) || len([]rune(g.Radice)) > MaxRif || g.Radice[5:69] != f.Sha256) {
		return forma("gruppo.radice", "la radice e' il rif nodo:<sha256 della fonte>:<chiave>, al piu' %d caratteri", MaxRif)
	}
	if (g.AllegatoID != "" && g.AllegatoID != f.AllegatoID) || (g.Sha256 != "" && g.Sha256 != f.Sha256) {
		return forma("gruppo", "allegato e sha256 del gruppo ripetono quelli di proposta.fonte")
	}
	if len(c.AllegatiCitati) > MaxAllegatiCitati {
		return forma("allegati_citati", "%d allegati citati, oltre il limite di %d", len(c.AllegatiCitati), MaxAllegatiCitati)
	}
	citati := map[string]bool{}
	for i, a := range c.AllegatiCitati {
		if !reUUID.MatchString(a) {
			return forma(fmt.Sprintf("allegati_citati[%d]", i), "UUID in forma canonica minuscola")
		}
		if i > 0 && a <= c.AllegatiCitati[i-1] {
			return forma(fmt.Sprintf("allegati_citati[%d]", i), "gli allegati citati sono in ordine e senza ripetizioni")
		}
		citati[a] = true
	}
	if f.AllegatoID != "" && !citati[f.AllegatoID] {
		return forma("allegati_citati", "la fonte %s non e' fra gli allegati citati", f.AllegatoID)
	}
	for _, x := range []struct{ nome, valore string }{{"impronta_contenuto", c.ImprontaContenuto},
		{"impronta_evidenze", c.ImprontaEvidenze}, {"impronta_regole", c.ImprontaRegole}} {
		if !reSha256.MatchString(x.valore) {
			return forma(x.nome, "64 cifre esadecimali minuscole")
		}
	}
	if e := c.validaRegole(); e != nil {
		return e
	}
	voci := map[string]bool{}
	for i, v := range c.Proposta.Nodi {
		campo := fmt.Sprintf("proposta.nodi[%d]", i)
		o := v.Origine
		if !reUUID.MatchString(o.AllegatoID) || !reSha256.MatchString(o.Sha256) || !reRif.MatchString(o.Rif) {
			return forma(campo+".origine", "origine {allegato_id, sha256, chiave, rif nodo:<sha256>:<chiave>}")
		}
		if o.Rif != "nodo:"+o.Sha256+":"+o.Chiave {
			return forma(campo+".origine", "il rif dell'origine e' nodo:<sha256>:<chiave> della stessa origine")
		}
		if v.Voce != "n:"+o.AllegatoID+":"+o.Rif {
			return forma(campo+".voce", "la voce e' n:<allegato_id>:<rif> dell'origine")
		}
		if voci[v.Voce] {
			return forma(campo+".voce", "voce %s ripetuta", v.Voce)
		}
		voci[v.Voce] = true
		if !citati[o.AllegatoID] {
			return forma(campo+".origine.allegato_id", "l'occorrenza %s non e' fra gli allegati citati", o.AllegatoID)
		}
		// codice e rev di partenza della working: le regole del codice e della rev, la rev in maiuscole ASCII (EM §3.8, §6.1)
		if v.CodiceProposto != "" && !CodiceAmmissibile(v.CodiceProposto) {
			return forma(campo+".codice_proposto", "il codice proposto e' senza spazi, controlli o caratteri di formato, al piu' %d byte", MaxCodiceProposto)
		}
		if v.RevProposta != "" && (!RevAmmissibile(v.RevProposta) || conMinuscoleASCII(v.RevProposta)) {
			return forma(campo+".rev_proposta", "la rev proposta e' in maiuscole ASCII, senza spazi o controlli, al piu' %d byte", MaxRevProposta)
		}
	}
	// la radice del gruppo e' una voce della proposta: P-2 la porta al prodotto (EM §6.1)
	if g.Radice != "" && !voci["n:"+f.AllegatoID+":"+g.Radice] {
		return forma("gruppo.radice", "la radice del gruppo %s non e' fra le voci della proposta", g.Radice)
	}
	fuori := map[string]bool{}
	for _, x := range c.Proposta.FuoriPerDecisione {
		fuori[x.Voce] = true
	}
	for i, a := range c.Proposta.Archi {
		campo := fmt.Sprintf("proposta.archi[%d]", i)
		if !reUUID.MatchString(a.AllegatoID) || !reRif.MatchString(a.Padre) || !reRif.MatchString(a.Figlio) {
			return forma(campo, "un arco ha l'allegato e due rif")
		}
		if !citati[a.AllegatoID] {
			return forma(campo+".allegato_id", "l'occorrenza %s non e' fra gli allegati citati", a.AllegatoID)
		}
		// gli estremi sono voci della proposta, o voci fuori per decisione (un arco verso un nodo tolto)
		for _, x := range []struct{ nome, rif string }{{"padre", a.Padre}, {"figlio", a.Figlio}} {
			if k := "n:" + a.AllegatoID + ":" + x.rif; !voci[k] && !fuori[k] {
				return forma(campo+"."+x.nome, "l'estremo %s non e' una voce della proposta ne' una voce fuori per decisione", x.rif)
			}
		}
	}
	for i, x := range c.Proposta.File {
		if !reUUID.MatchString(x.AllegatoID) || !citati[x.AllegatoID] {
			return forma(fmt.Sprintf("proposta.file[%d].allegato_id", i), "un file cita un'occorrenza fra gli allegati citati")
		}
	}
	if e := c.validaLetture(citati); e != nil {
		return e
	}
	for i, r := range c.Proposta.Relazioni {
		if r.Sorgente.Tipo != "nodo" && r.Sorgente.Tipo != "file" {
			return forma(fmt.Sprintf("proposta.relazioni[%d].sorgente.tipo", i), "la sorgente di una relazione e' nodo o file")
		}
	}
	for i, x := range c.Proposta.FuoriPerDecisione {
		campo := fmt.Sprintf("proposta.fuori_per_decisione[%d]", i)
		if x.Segno != SegnoTolto && x.Segno != SegnoNonRaggiunto {
			return forma(campo+".segno", "segno %q: tolto o non_raggiunto", x.Segno)
		}
		allegato, rif, ok := PartiDellaVoce(x.Voce)
		if !ok || !citati[allegato] {
			return forma(campo+".voce", "la voce e' n:<allegato_id fra gli allegati citati>:<rif>")
		}
		if g.Radice != "" && rif == g.Radice {
			return forma(campo+".voce", "la radice del gruppo non e' mai fuori per decisione (P-2)")
		}
		if voci[x.Voce] {
			return forma(campo+".voce", "la voce %s e' anche fra i nodi della proposta", x.Voce)
		}
	}
	return nil
}

// validaLetture: ogni lettura del registro (identita_proposta, alternative[] e riferimenti[].lettura delle voci,
// letture[] dei file) ha la sua chiave_lettura <allegato_id>/<id>, unica dentro il registro (EM §3.8, «Una lettura»):
// la stessa chiave in due punti porta la stessa lettura, byte per byte, altrimenti V-1 (la copia della decisione) e
// TrovaLettura sarebbero ambigui. L'allegato della chiave e' quello dell'unita' letta: della voce per l'identita'
// proposta e le alternative, del file per le sue letture, un allegato citato per un riferimento. I due campi facoltativi
// codice_proposto e rev_proposta di una lettura (DECA PE-08) hanno le regole di quelli della voce: il codice
// ammissibile, la rev in maiuscole ASCII.
func (c *Contenuto) validaLetture(citati map[string]bool) *Errore {
	viste := map[string]string{}
	una := func(campo string, raw json.RawMessage, allegato string) *Errore {
		var l struct {
			ChiaveLettura  string  `json:"chiave_lettura"`
			CodiceProposto *string `json:"codice_proposto"`
			RevProposta    *string `json:"rev_proposta"`
		}
		if err := json.Unmarshal(raw, &l); err != nil {
			return forma(campo, "una lettura e' un oggetto: %v", err)
		}
		if l.CodiceProposto != nil && !CodiceAmmissibile(*l.CodiceProposto) {
			return forma(campo+".codice_proposto", "il codice proposto di una lettura e' senza spazi, controlli o caratteri di formato, al piu' %d byte", MaxCodiceProposto)
		}
		if l.RevProposta != nil && (!RevAmmissibile(*l.RevProposta) || conMinuscoleASCII(*l.RevProposta)) {
			return forma(campo+".rev_proposta", "la rev proposta di una lettura e' in maiuscole ASCII, senza spazi o controlli, al piu' %d byte", MaxRevProposta)
		}
		m := reChiaveLettura.FindStringSubmatch(l.ChiaveLettura)
		if m == nil || haControlli(l.ChiaveLettura) {
			return forma(campo+".chiave_lettura", "la chiave di una lettura e' <allegato_id>/<id della lettura>")
		}
		if (allegato != "" && m[1] != allegato) || (allegato == "" && !citati[m[1]]) {
			return forma(campo+".chiave_lettura", "la chiave %s non e' dell'allegato dell'unita' letta", l.ChiaveLettura)
		}
		if prima, ok := viste[l.ChiaveLettura]; ok && prima != string(raw) {
			return forma(campo+".chiave_lettura", "la chiave %s ha due letture diverse nel registro", l.ChiaveLettura)
		}
		viste[l.ChiaveLettura] = string(raw)
		return nil
	}
	elenco := func(campo string, raw json.RawMessage) ([]json.RawMessage, *Errore) {
		var ll []json.RawMessage
		if len(raw) > 0 && json.Unmarshal(raw, &ll) != nil {
			return nil, forma(campo, "atteso un elenco")
		}
		return ll, nil
	}
	for i, v := range c.Proposta.Nodi {
		campo := fmt.Sprintf("proposta.nodi[%d]", i)
		if len(v.IdentitaProposta) > 0 {
			if e := una(campo+".identita_proposta", v.IdentitaProposta, v.Origine.AllegatoID); e != nil {
				return e
			}
		}
		alt, e := elenco(campo+".alternative", v.Alternative)
		if e != nil {
			return e
		}
		for j, l := range alt {
			if e := una(fmt.Sprintf("%s.alternative[%d]", campo, j), l, v.Origine.AllegatoID); e != nil {
				return e
			}
		}
		rr, e := elenco(campo+".riferimenti", v.Riferimenti)
		if e != nil {
			return e
		}
		for j, r := range rr {
			var x struct {
				Lettura json.RawMessage `json:"lettura"`
			}
			if json.Unmarshal(r, &x) != nil {
				return forma(fmt.Sprintf("%s.riferimenti[%d]", campo, j), "un riferimento e' un oggetto")
			}
			if len(x.Lettura) > 0 {
				if e := una(fmt.Sprintf("%s.riferimenti[%d].lettura", campo, j), x.Lettura, ""); e != nil {
					return e
				}
			}
		}
	}
	for i, f := range c.Proposta.File {
		ll, e := elenco(fmt.Sprintf("proposta.file[%d].letture", i), f.Letture)
		if e != nil {
			return e
		}
		for j, l := range ll {
			if e := una(fmt.Sprintf("proposta.file[%d].letture[%d]", i, j), l, f.AllegatoID); e != nil {
				return e
			}
		}
	}
	return nil
}

// PartiDellaVoce scompone la chiave di una voce, n:<allegato_id>:<rif>, nell'allegato e nel rif.
func PartiDellaVoce(voce string) (allegato, rif string, ok bool) {
	if len(voce) < 2+36+1 || !strings.HasPrefix(voce, "n:") || voce[38] != ':' {
		return "", "", false
	}
	allegato, rif = voce[2:38], voce[39:]
	return allegato, rif, reUUID.MatchString(allegato) && reRif.MatchString(rif)
}

// validaRegole: le versioni che servono fuori dal motore e la completezza del §3.8.2: con un contesto attivo sono
// obbligatori i partner (anche nessuno: un elenco vuoto), il contesto delle regole, la copia del contesto risolto e,
// in calcolo, impronta_indice (il nome minuscolo di ImprontaIndice: proposta di emendamento P-20); senza contesto quei
// campi sono assenti, non null (nullVietati). contesto_id e impronta_contesto dei file nel perimetro li controlla
// l'adattatore, che dal motore sa quali occorrenze stanno nel perimetro (EM §3.8.2): il registro non lo porta.
func (c *Contenuto) validaRegole() *Errore {
	r := c.Regole
	if r.Versioni.Importazione != c.VersioneImportazione {
		return forma("regole.versioni.importazione", "la versione dell'adattatore %q non e' quella del registro %q", r.Versioni.Importazione, c.VersioneImportazione)
	}
	if r.Versioni.Canonico != jsoncanonico.Versione {
		return forma("regole.versioni.canonico", "canonicalizzazione %q, attesa %q", r.Versioni.Canonico, jsoncanonico.Versione)
	}
	if g := r.GrammaticaCliente; g != nil && (!reUUID.MatchString(g.ClienteID) || strings.TrimSpace(g.Hash) == "") {
		return forma("regole.grammatica_cliente", "cliente_id UUID canonico e hash non vuoto")
	}
	if r.Partner != nil {
		visti := map[string]bool{}
		for i, p := range *r.Partner {
			if !strings.HasPrefix(p.Emittente, "partner:") || strings.TrimSpace(p.Hash) == "" || visti[p.Emittente] {
				return forma(fmt.Sprintf("regole.partner[%d]", i), "un partner attivo e' partner:<id>, con l'hash, una volta sola")
			}
			visti[p.Emittente] = true
		}
	}
	contesto := len(c.Contesto) > 0 && string(c.Contesto) != "null"
	if r.Contesto == nil {
		if r.Partner != nil || contesto {
			return forma("regole.contesto", "senza contesto partner e contesto risolto sono assenti (EM §3.8.2)")
		}
		return nil
	}
	if strings.TrimSpace(r.Contesto.Impronta) == "" || len(r.Contesto.ID) == 0 || string(r.Contesto.ID) == "null" {
		return incompleto("regole.contesto", "il contesto delle regole vuole id e impronta")
	}
	if r.Partner == nil {
		return incompleto("regole.partner", "con un contesto attivo i partner attivi sono obbligatori (anche nessuno)")
	}
	if !contesto {
		return incompleto("contesto", "con un contesto attivo la copia del contesto risolto e' obbligatoria")
	}
	var calcolo map[string]json.RawMessage
	var indice string
	if json.Unmarshal(c.Calcolo, &calcolo) != nil || json.Unmarshal(calcolo["impronta_indice"], &indice) != nil || strings.TrimSpace(indice) == "" {
		return incompleto("calcolo.impronta_indice", "con un contesto attivo l'impronta dell'indice (ImprontaIndice) e' obbligatoria")
	}
	return nil
}

func haControlli(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return true
		}
	}
	return false
}

// PartnerAttivi: gli emittenti partner attivi nel registro (regola unica dell'emittente, EM §5.6).
func (c *Contenuto) PartnerAttivi() []string {
	if c.Regole.Partner == nil {
		return nil
	}
	out := make([]string, 0, len(*c.Regole.Partner))
	for _, p := range *c.Regole.Partner {
		out = append(out, p.Emittente)
	}
	return out
}

// VocePerRif: la voce del registro con quell'occorrenza e quel rif (V-3: una voce intera, mai il solo rif).
func (c *Contenuto) VocePerRif(allegato uuid.UUID, rif string) (Voce, bool) {
	chiave := "n:" + allegato.String() + ":" + rif
	for _, v := range c.Proposta.Nodi {
		if v.Voce == chiave {
			return v, true
		}
	}
	return Voce{}, false
}

// FuoriPerRif: la voce fuori per decisione con quell'occorrenza e quel rif (V-3).
func (c *Contenuto) FuoriPerRif(allegato uuid.UUID, rif string) (FuoriPerDecisione, bool) {
	chiave := "n:" + allegato.String() + ":" + rif
	for _, x := range c.Proposta.FuoriPerDecisione {
		if x.Voce == chiave {
			return x, true
		}
	}
	return FuoriPerDecisione{}, false
}

// EVoceDellaRadice: la voce della radice del gruppo, riconosciuta dalla struttura (origine.rif = gruppo.radice), mai
// dal valore di un campo (EM §3.8.1).
func (c *Contenuto) EVoceDellaRadice(v Voce) bool {
	return c.Gruppo.Radice != "" && v.Origine.Rif == c.Gruppo.Radice
}
