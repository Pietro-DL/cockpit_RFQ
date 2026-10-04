package motorea

import (
	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// VersioneRisultato: lo schema del JSON di Interpretazione (5.6.1). Cambia con un commit che lo dichiara, e la
// prova del pacchetto che lo fissa si riscrive.
const VersioneRisultato = 1

// Interpretazione: il risultato di Interpreta (parte 1 §7.3; par.3.3.4). È tutta in memoria e non si salva in
// A1. Ogni elenco è in ordine di ID, le diagnostiche per (codice, percorso, riferimenti): due esecuzioni sugli
// stessi ingressi danno gli stessi byte canonici e la stessa Impronta.
//
// Non ci sono i campi Riferimenti e Menzioni del par.3.3.4: RiferimentoLetto nasce solo quando riferimenti_rfq
// diventa attivo (R20 c; par.12), MenzioneGenerica solo con R25 (e) = B. Con R25 (e) = A un token che nessuna
// famiglia riconosce non dà letture.
type Interpretazione struct {
	ID                string                 `json:"id"` // impronta degli ingressi: bundle, snapshot, versioni, limiti, uso (par.3.4.2)
	BundleID          string                 `json:"bundle_id"`
	ClienteID         uuid.UUID              `json:"cliente_id"`
	HashSnapshot      string                 `json:"hash_snapshot"`
	VersioneAlgoritmo string                 `json:"versione_algoritmo"`
	VersioneRouter    string                 `json:"versione_router"` // qui, non nello snapshot delle regole (R41 c)
	VersioneLimiti    string                 `json:"versione_limiti"` // la versione_limiti dell'indice (R43 B)
	ImprontaLimiti    string                 `json:"impronta_limiti"` // sha256 del canonico dei limiti usati, valori e versione (R43 B)
	VersioneRisultato int                    `json:"versione_risultato"`
	ImprontaUso       string                 `json:"impronta_uso"`
	Stato             string                 `json:"stato"` // completa | parziale | non_disponibile; «completa» con zero letture non è un PDF illeggibile
	Letture           []LetturaCodice        `json:"letture,omitempty"`
	Attributi         []AttributoLetto       `json:"attributi,omitempty"`
	Relazioni         []RelazioneSemantica   `json:"relazioni,omitempty"`
	Diagnostiche      []evidenze.Diagnostica `json:"diagnostiche,omitempty"`
	Impronta          string                 `json:"impronta"` // sha256 del canonico del risultato con Impronta vuota
}

// Gli stati dell'interpretazione (5.4.6 punto 16). Contano solo le capacità di lettura del documento (testo,
// cartiglio, struttura, contenuto), i limiti superati e i troncamenti del worker; le capacità che dicono che cosa
// il documento non sa distinguere (firma, segmentazione, grafo completo, …) restano nella qualità della fonte.
const (
	StatoInterpretazioneCompleta       = "completa"
	StatoInterpretazioneParziale       = "parziale"
	StatoInterpretazioneNonDisponibile = "non_disponibile"
)

// LetturaCodice: una lettura del codice in un'occorrenza precisa, con funzione e ruoli (parte 1 §7.3). Si
// deduplicano solo le letture equivalenti della stessa occorrenza (A-C10): mai per sola stringa.
//
// In più rispetto al par.3.3.4 [+3, 5.4.6]:
//   - AltreUnita: le altre unità che vedono la STESSA occorrenza (stesso testo originale e stesso intervallo, o
//     una cella agganciata per righe e il segmento che la contiene): la lettura è una sola e le evidenze di
//     supporto restano (parte 1 §7.3; R28 b);
//   - Uso, OrigineUso: l'uso del segmento da cui viene la lettura e la sua origine (operatore | riconoscimento |
//     scenario | "" senza selezione). L'incertezza sulla pertinenza accompagna la lettura (P1 §4.3) e l'origine
//     resta visibile (v3 §10.2). Per le unità fuori da un messaggio l'uso è «non_applicabile».
type LetturaCodice struct {
	ID             string                 `json:"id"` // «l:<unità>:<famiglia>/<forma>:<inizio>-<fine>»: stabile a parità di ingresso
	UnitaID        string                 `json:"unita_id"`
	AltreUnita     []string               `json:"altre_unita,omitempty"`
	Occorrenza     evidenze.Intervallo    `json:"occorrenza"`         // sul testo dell'unità
	Assoluto       *evidenze.PosTesto     `json:"assoluto,omitempty"` // sul testo originale, quando l'unità è localizzata in modo esatto (A-C03)
	Forma          LetturaForma           `json:"forma"`
	Funzione       Funzione               `json:"funzione"`
	Uso            string                 `json:"uso"`
	OrigineUso     string                 `json:"origine_uso,omitempty"`
	RuoliCandidati []grammatica.Ruolo     `json:"ruoli_candidati,omitempty"` // ruoli della famiglia ∩ ruoli ammessi dalla funzione (v3 §2; P1 §6)
	MotivoRuoli    string                 `json:"motivo_ruoli"`
	Categorie      []grammatica.Categoria `json:"categorie,omitempty"` // annotazione dalla famiglia, fuori dall'intersezione (R7)
	Trasformazioni []Trasformazione       `json:"trasformazioni,omitempty"`
	Qualita        string                 `json:"qualita"` // completa | parziale | da_verificare
	Motivi         []string               `json:"motivi,omitempty"`
}

// Le qualità di una lettura (5.4.6 punto 8).
const (
	QualitaCompleta     = "completa"
	QualitaParziale     = "parziale"
	QualitaDaVerificare = "da_verificare"
)

// AttributoLetto: un attributo legato all'ENTITÀ, mai al codice più vicino nel testo (A-C02). Un'unità senza
// entità non dà attributi. La formazione STEP è un tipo a sé, mai una revisione confrontabile (v3 D1).
//
// In più rispetto al par.3.3.4 [+3, 5.4.6]: Evidenze, le unità che sostengono l'attributo oltre alla sua (per
// esempio la cella d'intestazione della colonna quantità). LettureCompatibili sono le letture della stessa
// entità a cui la regola che ha dato l'attributo si applica: è la provenance del legame fra attributo e codice.
type AttributoLetto struct {
	ID                 string   `json:"id"`   // «a:<unità>:<tipo>»; per una revisione data da una regola, più «:<famiglia>/<regola>»
	Tipo               string   `json:"tipo"` // revisione | formazione | materiale | scala | titolo | quantita
	Grezzo             string   `json:"grezzo"`
	Normalizzato       string   `json:"normalizzato,omitempty"`
	UnitaID            string   `json:"unita_id"`
	EntitaID           string   `json:"entita_id"`
	Stato              string   `json:"stato"` // attribuito | ambiguo | non_attribuito | non_interpretabile
	LettureCompatibili []string `json:"letture_compatibili,omitempty"`
	Evidenze           []string `json:"evidenze,omitempty"`
}

// I tipi e gli stati di un attributo.
const (
	AttributoRevisione  = "revisione"
	AttributoFormazione = "formazione"
	AttributoMateriale  = "materiale"
	AttributoScala      = "scala"
	AttributoTitolo     = "titolo"
	AttributoQuantita   = "quantita"

	StatoAttribuito        = "attribuito"
	StatoAmbiguo           = "ambiguo"
	StatoNonAttribuito     = "non_attribuito"
	StatoNonInterpretabile = "non_interpretabile"
)

// RelazioneSemantica: simile | speculare, dall'entità sorgente verso le letture bersaglio. Nessuna fusione. Nasce
// solo da una lettura con funzione relazione (router-1 riga 12), cioè da una forma attiva sul campo del
// particolare simile: il testo libero resta menzione (parte 1 §10.1). In A1 quel campo è riservato, quindi
// nessuna grammatica la produce.
type RelazioneSemantica struct {
	ID               string   `json:"id"`
	Tipo             string   `json:"tipo"`
	EntitaSorgente   string   `json:"entita_sorgente"`
	Riconoscimento   string   `json:"riconoscimento"` // famiglia/forma che l'ha riconosciuta
	LettureBersaglio []string `json:"letture_bersaglio"`
	Evidenze         []string `json:"evidenze,omitempty"`
}

// RelazioneSimile: il tipo della relazione che dà la riga 12 del router.
const RelazioneSimile = "simile"

// Trasformazione: la regola, l'operazione, gli intervalli originali sul testo dell'unità, il valore prima e dopo,
// il motivo. Una per ogni parte che resta fuori dalla base, più la normalizzazione delle maiuscole: l'originale
// non si perde mai (testo[Intervalli[0]] == Prima).
type Trasformazione struct {
	Regola     string                `json:"regola,omitempty"`
	Operazione string                `json:"operazione"`
	Prima      string                `json:"prima"`
	Dopo       string                `json:"dopo"`
	Motivo     string                `json:"motivo"`
	Intervalli []evidenze.Intervallo `json:"intervalli"`
}

// Le operazioni delle trasformazioni.
const (
	OpSeparaEtichetta     = "separa_etichetta"
	OpSeparaAffisso       = "separa_affisso"
	OpSeparaMarcatore     = "separa_marcatore"
	OpSeparaToken         = "separa_token"
	OpSeparaDecorazione   = "separa_decorazione"
	OpSeparaRevisione     = "separa_revisione"
	OpSeparaRipetizione   = "separa_ripetizione"
	OpNormalizzaMaiuscolo = "normalizza_maiuscolo"
)
