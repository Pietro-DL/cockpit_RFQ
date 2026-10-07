package confronto

import (
	"time"

	"github.com/google/uuid"
)

// I DTO propri e piatti di confronto (piano A, par.3.3.8 e 6.4.6, «Il passaggio»; R53 B). Li riempie chi chiama,
// copiando campo per campo i record piatti di valutazione (FileConfrontabile, VecchioPiatto, NuovoPiatto,
// CandidatoPiatto, ProdottoConfrontabile), senza scelte: le scelte (quali basi, quale lettura del vecchio, se le
// revisioni sono confrontabili) le fa valutazione, una volta sola.
//
// Nomi, ordine, tipi e tag JSON sono quelli dei gemelli di valutazione, campo per campo (elenco congelato della fase 0
// di B6, CP.2, con l'emendamento F0-19, il campo CodiceLettoMarcatore di R114 e il campo RevisioneDa di R113: 19 campi
// in Vecchio). Solo tipi delle foglie: string, bool, int, uuid.UUID, time.Time, i loro puntatori, gli slice e le struct
// gemelle; nessun tipo con nome del motore, nemmeno per gli enumerati (per esempio Collocazione è una string). I due
// pacchetti non si importano:
// che i campi coincidano lo controlla una prova nel chiamante (A1c-L1-31), con un confronto strutturale, perché i nomi
// dei tipi annidati sono diversi. Un campo cambiato da una parte si annuncia all'altra prima del commit.

// File: il vecchio e il nuovo di un allegato del thread, come confronto li riceve. Uno per allegato, anche per i
// contenitori, le nature non «file» e i file di un thread non valutato (Nuovo.Valutato falso).
type File struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	Vecchio    Vecchio   `json:"vecchio"`
	Nuovo      Nuovo     `json:"nuovo"`
}

// Vecchio: ciò che c'è adesso nel DB per un file, con il codice già letto dalla grammatica del cliente (R31 c). La base
// vecchia è sempre Base, mai la stringa: «7120100A» e la base 7120100 sono la stessa base.
//   - Stato, Fonte: della proposta attuale (documento_proposta); Stato "" = nessuna proposta.
//   - Codice, Rev: della proposta, o del documento confermato se il file è deciso (registro §10.2).
//   - Base, Marcatore, Leggibile, MotivoLettura: la lettura del codice con la grammatica; Base "" se il codice non si
//     legge (Leggibile falso, con il motivo). Revisione: la revisione vecchia interpretata da valutazione, con la sua
//     provenienza in RevisioneDa (R113 B ratificata; E2 §2.6): «codice» se letta nel codice con la grammatica, «colonna»
//     se letta nella colonna Rev con la regola in campo separato della famiglia, "" se non ce n'è una (allora Revisione
//     è vuota). L'originale resta in Codice e in Rev.
//   - CodiceLetto: la lettura del vecchio motore registrata nella proposta (fonte del contratto, T-B0-14); "" negli
//     export, che non hanno i dettagli. CodiceLettoBase: la base di CodiceLetto letta con la grammatica del cliente
//     (LetturaRegistrata.Base); "" se CodiceLetto è vuoto o non si legge (emendamento F0-19 di CP.2, R-44).
//     CodiceLettoMarcatore: il marcatore di CodiceLetto, dalla stessa lettura (LetturaRegistrata.Marcatore); "" se il
//     marcatore non c'è o CodiceLetto non si legge. Serve alla misura delle correzioni (R114, precisata dall'utente il
//     07/10), che conta a parte la stessa base con due marcatori diversi.
//   - Componente, Documento: il componente e il documento della decisione, dal documento (documento_provenienza →
//     documento), mai dalla proposta. ComponenteProposta: il componente della proposta («assegna»), per i controlli di
//     baseline e C5. SostituitoDa: il documento che sostituisce quello deciso. DecisoIl: UTC, al millisecondo.
//   - Destinazione: le chiavi dei candidati della proposta F8 (componente:<uuid>, nodo:<id>, identificativo:<CODICE>);
//     nil se assente o negli export.
type Vecchio struct {
	Stato                string     `json:"stato"`
	Fonte                string     `json:"fonte"`
	Codice               string     `json:"codice"`
	Rev                  string     `json:"rev"`
	Base                 string     `json:"base"`
	Marcatore            string     `json:"marcatore"`
	Revisione            string     `json:"revisione"`
	Leggibile            bool       `json:"leggibile"`
	MotivoLettura        string     `json:"motivo_lettura"`
	CodiceLetto          string     `json:"codice_letto"`
	CodiceLettoBase      string     `json:"codice_letto_base"`
	CodiceLettoMarcatore string     `json:"codice_letto_marcatore"`
	RevisioneDa          string     `json:"revisione_da"`
	Componente           *uuid.UUID `json:"componente,omitempty"`
	Documento            *uuid.UUID `json:"documento,omitempty"`
	ComponenteProposta   *uuid.UUID `json:"componente_proposta,omitempty"`
	SostituitoDa         *uuid.UUID `json:"sostituito_da,omitempty"`
	DecisoIl             *time.Time `json:"deciso_il,omitempty"`
	Destinazione         []string   `json:"destinazione,omitempty"`
}

// Nuovo: le letture d'identità del file (R25 a) e l'ancoraggio del motore A, piatti.
//   - Valutato falso, con il Motivo: thread non valutato, contenitore, natura non «file», documento non leggibile.
//   - Basi: le basi delle letture d'identità, in ordine, senza doppioni.
//   - Candidati: i candidati di ancoraggio, nell'ordine del sostegno (mai un punteggio né una scelta).
//   - Collocazione, Associazione, Disponibilita: i valori dell'ancoraggio, come stringhe.
//   - Revisione: la revisione letta dal motore. Revisioni: uguali | diverse | non_confrontabili, con il motivo: lo
//     decide valutazione con la grammatica, perché confronto non ha il motore (6.4.6, passo 10).
type Nuovo struct {
	Valutato        bool        `json:"valutato"`
	Motivo          string      `json:"motivo"`
	Basi            []string    `json:"basi,omitempty"`
	Candidati       []Candidato `json:"candidati,omitempty"`
	Collocazione    string      `json:"collocazione"`
	Associazione    string      `json:"associazione"`
	Disponibilita   string      `json:"disponibilita"`
	Revisione       string      `json:"revisione"`
	Revisioni       string      `json:"revisioni"`
	MotivoRevisioni string      `json:"motivo_revisioni"`
}

// Candidato: un candidato di ancoraggio, piatto: il target (per il livello prodotto il Rif del target, per il livello
// componente il componente deciso, il nodo o l'identità), il livello (prodotto | componente), la base del target o del
// nodo, l'autorità e le basi dei target delle radici raggiungibili.
type Candidato struct {
	Target   string   `json:"target"`
	Livello  string   `json:"livello"`
	Base     string   `json:"base"`
	Autorita string   `json:"autorita"`
	Radici   []string `json:"radici,omitempty"`
}

// ProdottoNuovo: un candidato prodotto della mail, piatto, per ConfrontaProdotti: il codice richiesto com'è scritto, la
// base, il qualificatore fase attribuito, la quantità e se viene da una cella sotto l'intestazione dichiarata (R28).
type ProdottoNuovo struct {
	CodiceRichiesto string `json:"codice_richiesto"`
	Base            string `json:"base"`
	Fase            string `json:"fase"`
	Quantita        *int   `json:"quantita,omitempty"`
	QuantitaDaCella bool   `json:"quantita_da_cella"`
}

// I valori dei campi stringa che le regole di questo pacchetto leggono. Sono i valori dei tipi del DB e del motore
// (stato_proposta, fonte_proposta; ancoraggio.Collocazione, Associazione, Autorita, la forma «componente:<uuid>» di
// ancoraggio.RifComponente; motorea.ConfrontaRevisioni e un motivo di valutazione), riscritti qui come stringhe perché
// confronto non importa il motore (grafo del par.3.2.1). Un valore che non è fra questi non vale come nessuno di loro.
const (
	statoScartata = "scartata"

	fonteOperatore = "operatore"

	collocazioneRadice           = "radice"
	collocazioneFiglio           = "figlio"
	collocazioneFuoriRichiesta   = "fuori_richiesta"
	collocazioneNonDeterminabile = "non_determinabile"

	associazioneCandidatoUnico = "candidato_unico"
	associazioneAmbiguo        = "ambiguo"
	associazioneDiscordante    = "discordante"

	autoritaScenario = "scenario"

	prefissoRifComponente = "componente:"

	revisioniUguali           = "uguali"
	revisioniDiverse          = "diverse"
	revisioniNonConfrontabili = "non_confrontabili"

	motivoRevisioniNuoveDiscordi = "revisioni_nuove_discordi" // valutazione.MotivoRevisioniNuoveDiscordi
)
