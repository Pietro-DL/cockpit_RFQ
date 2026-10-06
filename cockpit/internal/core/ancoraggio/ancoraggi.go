package ancoraggio

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// Gli ancoraggi dei file e la pre-associazione (piano A, 6.4.5, regole 3-8; par.3.3.7; contratto §1.2, §1.3, §1.5,
// §2.2; commit P6b, B4, fase 2): a quali prodotti e a quali nodi delle strutture dei prodotti si candida ogni file,
// con l'associazione e la collocazione, i motivi, le impronte. Sopra le strutture di ProponiStrutture, chiamata qui
// dentro (T-B2-01), con le identità dei nodi della fase 1: prima della conferma della fonte sulle strutture candidate
// (R59 A), dopo anche sulla BOM di lavoro, nello stesso calcolo (pre-associazione, R85). Le proposte non aspettano la
// nomenclatura e portano accanto l'incertezza dell'identità (T-E1-03). Un candidato non è mai una decisione: la sua
// origine è sempre «proposto» (T-B0-09), e la pertinenza, il perimetro e «da smistare» sono di valutazione (R93, B6).
// Dopo gli ancoraggi, nello stesso calcolo, la riconciliazione fra lo STEP e il cartiglio dei 2D associati ai nodi
// (riconciliazione.go, fase 3).

// ---- i due assi della parte 1 e la collocazione ----

// Disponibilita: se il contenuto di un allegato che c'è dà letture (par.3.3.7; parte 1 §7.4). La dà valutazione nel
// FileInterpretato (R42 B; 6.4.6 passo 8); qui si porta nell'ancoraggio del file, accanto agli altri assi, e non
// cambia le regole: un asse distinto, mai un'«assenza verificata» (A-C09).
//   - disponibile: i fatti alla terna corrente;
//   - mancante: un allegato senza contenuto (sha256 nullo), non un componente senza CAD (P-20);
//   - pendente: un lavoro di analisi in attesa;
//   - parziale: il documento dice che la lettura è parziale;
//   - senza_testo: una scansione, un PDF senza testo nativo e senza OCR (T-B0-31: non «illeggibile»);
//   - illeggibile: un contenuto che non si apre (LD-05);
//   - errore: i fatti in errore, come li chiama il 6.4.6 passo 8. Quale dei due valga per un PDF che non si apre lo
//     decide valutazione, che legge i fatti (lettura dell'orchestratore T-B4-09).
type Disponibilita string

const (
	DisponibilitaDisponibile Disponibilita = "disponibile"
	DisponibilitaMancante    Disponibilita = "mancante"
	DisponibilitaPendente    Disponibilita = "pendente"
	DisponibilitaParziale    Disponibilita = "parziale"
	DisponibilitaSenzaTesto  Disponibilita = "senza_testo"
	DisponibilitaIlleggibile Disponibilita = "illeggibile"
	DisponibilitaErrore      Disponibilita = "errore"
)

// Associazione: l'asse dell'associazione di un file (par.3.3.7; contratto §1.5; T-B0-11: un campo distinto dalla
// riconciliazione).
//   - nessun_candidato: nessun prodotto e nessun nodo raggiungibile ha un'identità del file;
//   - candidato_unico: un solo candidato. Non è una conferma: la conferma è un gesto (R29 f);
//   - ambiguo: più candidati, nessuna scelta (T-E1-07: a parità c'è ambiguo; A-C07; v3 §2 r.163);
//   - discordante: il file ha più identità che non concordano (R64 A, T-B0-35): restano tutte, con i loro candidati;
//   - non_valutata: un contenitore o una natura che non è un file; non arriva qui, lo segna valutazione (6.4.5 regola
//     5): ProponiAncoraggi non lo produce mai.
type Associazione string

const (
	AssociazioneNonValutata     Associazione = "non_valutata"
	AssociazioneNessunCandidato Associazione = "nessun_candidato"
	AssociazioneCandidatoUnico  Associazione = "candidato_unico"
	AssociazioneAmbiguo         Associazione = "ambiguo"
	AssociazioneDiscordante     Associazione = "discordante"
)

// Collocazione: dove sta il file nella richiesta (par.3.3.7; v3 §2 r.166-167; contratto §1.5).
//   - radice: i candidati sono tutti a livello prodotto;
//   - figlio: i candidati sono tutti a livello componente;
//   - fuori_richiesta: nessun candidato, con l'identità letta, i target noti e lo scenario completo (ogni target con
//     almeno una struttura, tutte con il grafo completo). È una proposta, mai uno scarto (R105);
//   - non_determinabile: altrimenti, con i motivi (Motivo*).
type Collocazione string

const (
	CollocazioneRadice           Collocazione = "radice"
	CollocazioneFiglio           Collocazione = "figlio"
	CollocazioneFuoriRichiesta   Collocazione = "fuori_richiesta"
	CollocazioneNonDeterminabile Collocazione = "non_determinabile"
)

// I livelli di un candidato (CandidatoAncoraggio.Livello; par.3.3.7): il prodotto target (collocazione radice) o un
// nodo raggiungibile nelle strutture dei target (collocazione figlio).
const (
	LivelloProdotto   = "prodotto"
	LivelloComponente = "componente"
)

// I motivi di un ancoraggio (AncoraggioFile.Motivi), in ordine di valore.
//   - Per la collocazione non determinabile (6.4.5 regola 3): target_ignoti (nessun target); ancoraggio.grafo_incompleto
//     (una struttura di un target con il grafo non completo: la mancanza di un arco non è un'informazione);
//     ancoraggio.target_senza_struttura (un target senza strutture: un file può essere un suo figlio);
//     nessuna_lettura_identita (il file non ha letture d'identità). I due motivi con il nome di un codice sono gli
//     stessi valori delle diagnostiche delle strutture (6.4.5: «con il motivo»); sui file nessuna diagnostica in più.
//   - livelli_diversi: un file sia fra i target sia fra i figli di un altro target (6.4.5 regola 3): la collocazione
//     non si sceglie, ed è non determinabile.
//   - identita_discordanti: l'associazione è discordante (R64 A, T-B0-35).
//   - per l'ambiguità: revisioni_discordanti (la stessa base raggiungibile con revisioni lette diverse: v3 §2 r.163),
//     completamenti_multipli (una forma parziale compatibile con più completamenti: A-C07), candidati_multipli
//     (gli altri casi).
const (
	MotivoAncoraggioTargetIgnoti          = "target_ignoti"
	MotivoAncoraggioGrafoIncompleto       = CodiceGrafoIncompleto
	MotivoAncoraggioTargetSenzaStruttura  = CodiceTargetSenzaStruttura
	MotivoAncoraggioNessunaLettura        = "nessuna_lettura_identita"
	MotivoAncoraggioLivelliDiversi        = "livelli_diversi"
	MotivoAncoraggioIdentitaDiscordanti   = "identita_discordanti"
	MotivoAncoraggioRevisioniDiscordanti  = "revisioni_discordanti"
	MotivoAncoraggioCompletamentiMultipli = "completamenti_multipli"
	MotivoAncoraggioCandidatiMultipli     = "candidati_multipli"
)

// I motivi di un candidato (CandidatoAncoraggio.Motivo): la base di un'identità del file è quella del target, o
// quella di un nodo raggiungibile (6.4.5 regola 3); radice_scelta: l'identità del file è quella della radice scelta
// dall'operatore per la BOM di lavoro, che non ha la base del prodotto (R76 A, T-B4-25).
const (
	MotivoCandidatoBaseDelTarget = "base_del_target"
	MotivoCandidatoBaseDelNodo   = "base_di_un_nodo_raggiungibile"
	MotivoCandidatoRadiceScelta  = "radice_scelta"
)

// Le dimensioni di un candidato (DimensioneCompatibilita.Dimensione; 6.4.5 regola 4), più le due della compatibilità
// con le decisioni del nodo, che si ricalcola a ogni fotografia (T-E1-05; PO-21, la parte di B4). Le dimensioni
// «radici» e «tipo documento» del 6.4.5 non si calcolano (T-B4-29): le radici sono già campi del candidato
// (RadiciDelFile, RadiciRaggiungibili, Posizioni), e il tipo documentale in ancoraggio non è noto (è di valutazione).
// Il marcatore segue la regola unica (T-B4-06 rivisto, T-B4-30): uguale, compatibile_parziale se è scritto da una parte sola, discordante se
// sono scritti tutti e due e diversi.
const (
	DimensioneNamespace        = "namespace"
	DimensioneBase             = "base"
	DimensioneMarcatore        = "marcatore"
	DimensioneRevisione        = "revisione"
	DimensioneQualificatori    = "qualificatori"
	DimensioneCodiceConfermato = "codice_confermato"
	DimensioneCodiceManuale    = "codice_manuale"
)

// PrefissoRifIdentita e RifIdentita: il riferimento di un'identità raggiungibile, «identita:<namespace>:<base>:
// <marcatore>:<revisione>» (le ultime due anche vuote). È il Target di un candidato a livello componente che sta su
// nodi diversi senza una decisione comune: un componente in due STEP diversi è un candidato solo
// (FIGLIO-CONDIVISO), e due STEP diversi non hanno un nodo comune (T-E1-04), quindi si abbinano per l'identità, come
// proposta (emendamento E1 §4.2; lettura dell'orchestratore T-B4-08). Non è il Rif di un nodo («nodo:<sha256>:
// <chiave>»).
const PrefissoRifIdentita = "identita:"

// RifIdentita: il riferimento dell'identità con quel namespace, quella base normalizzata, quel marcatore e quella
// revisione letta.
func RifIdentita(namespace, base, marcatore, revisione string) string {
	return PrefissoRifIdentita + namespace + ":" + base + ":" + marcatore + ":" + revisione
}

// ---- le uscite ----

// EsitoAncoraggi: gli ancoraggi dei file di un thread (par.3.3.7; contratto §2.2).
//   - HashIngresso: la versione del servizio e, per ogni file, l'allegato, il bundle, l'ID dell'interpretazione, la
//     disponibilità (6.4.5 regola 8) e, se è un 2D, il segno del 2D (fase 3). HashTarget: i target e il contesto, in
//     ordine canonico (parte 1 §9.4): con le stesse interpretazioni e target diversi cambia solo lui (A-C11).
//   - File: un ancoraggio per allegato, in ordine di allegato; due allegati con lo stesso sha256 sono due ancoraggi,
//     mai fusi (HASH-CONFLITTO, 6.4.5 regola 7).
//   - Strutture: le strutture di ProponiStrutture (era Scenario: T-B0-02), con SenzaFile ricalcolato dai file ancorati
//     e, sui nodi con un 2D associato, la riconciliazione (CatenaCodice.Documentale, i candidati di revisione del
//     cartiglio, lo stato della revisione: fase 3).
//   - Diagnostiche: quelle delle strutture e quelle della riconciliazione (ancoraggio.completamento_documentale,
//     ancoraggio.correzione_documentale), in ordine di (codice, percorso, riferimenti).
//   - Impronta: sha256 del canonico dell'esito con Impronta vuota.
type EsitoAncoraggi struct {
	VersioneServizio string                 `json:"versione_servizio"`
	HashIngresso     string                 `json:"hash_ingresso"`
	HashTarget       string                 `json:"hash_target"`
	File             []AncoraggioFile       `json:"file,omitempty"`
	Strutture        []StrutturaProdotto    `json:"strutture,omitempty"`
	Diagnostiche     []evidenze.Diagnostica `json:"diagnostiche,omitempty"`
	Impronta         string                 `json:"impronta"`
}

// AncoraggioFile: l'ancoraggio di un file (par.3.3.7): i due assi, la collocazione, i candidati, i motivi.
//   - Letture: gli ID delle letture d'identità del file (funzione identita_file: nome, voce d'archivio, codice o
//     numero di disegno del cartiglio, id o nome di una radice STEP), in ordine (campo in più, lettura
//     dell'orchestratore T-B4-10). Le menzioni del testo e le relazioni non sono identità (P1 §4.3; C-39).
//   - Candidati: nell'ordine del sostegno (meno dimensioni discordanti prima, poi il livello prodotto, poi il Target):
//     un ordine euristico, mai un punteggio né una scelta (T-E1-07).
type AncoraggioFile struct {
	AllegatoID    uuid.UUID             `json:"allegato_id"`
	Disponibilita Disponibilita         `json:"disponibilita"`
	Associazione  Associazione          `json:"associazione"`
	Collocazione  Collocazione          `json:"collocazione"`
	Letture       []string              `json:"letture,omitempty"`
	Candidati     []CandidatoAncoraggio `json:"candidati,omitempty"`
	Motivi        []string              `json:"motivi,omitempty"`
}

// CandidatoAncoraggio: un target per un file (par.3.3.7, 6.4.5). Un figlio condiviso è UN candidato con più radici e
// percorsi (FIGLIO-CONDIVISO); le radici del file e le radici raggiungibili sono due cose distinte (CP-24).
//   - Target: per il livello prodotto il Rif del target, lo stesso dell'entità dei candidati di revisione (T-E1-06; il
//     6.4.5 scriveva «prodotto:<Rif>», e Livello lo dice già); per il livello componente il componente deciso comune a
//     tutti i nodi (RifComponente), altrimenti il nodo, se è uno solo (RifNodo), altrimenti l'identità (RifIdentita).
//     La chiave di un candidato è (Livello, Target), mai il solo Target (T-B4-28): un target «componente:<uuid>» e un
//     componente deciso con lo stesso UUID, a due livelli, sono due candidati.
//   - Autorita: la più debole fra quelle dei target da cui il candidato si raggiunge (scenario < proposta <
//     confermata); gli archi dei fatti non la abbassano. Non è mai una conferma del file (R29 f).
//   - Origine: sempre «proposto» (T-B0-09): un'associazione proposta non diventa mai confermata (contratto §1.5).
//   - RadiciDelFile: le radici dello STEP stesso, se il file ha una struttura nel contesto (Rif dei nodi); vuoto per
//     gli altri file. RadiciRaggiungibili: i target da cui il nodo si raggiunge (C-21).
//   - Padri: i padri immediati dei nodi nelle strutture, che non sono per forza le radici. Percorsi: per ogni
//     struttura, il target e poi i nodi dalla radice al nodo, tutti i percorsi. Archi: gli archi dei percorsi, con
//     quantità, occorrenze e origine fatti, e accanto l'arco confermato quando c'è (T-14).
//   - Posizioni: dove sta il candidato, struttura per struttura, con lo stato della struttura: la pre-associazione sulla
//     BOM di lavoro si vede qui (R85; campo in più, T-B4-10). Per il livello prodotto le radici delle strutture del
//     target che lo rappresentano. Una posizione su un nodo scartato porta il motivo nodo_scartato (T-B4-27).
//   - Letture: le letture d'identità del file che lo sostengono. Dimensioni: namespace, base, marcatore, revisione,
//     qualificatori, per ogni lettura e ogni nodo o target (per la radice scelta anche con la radice: T-B4-25); per un
//     nodo deciso anche la compatibilità con il codice confermato o manuale (T-E1-05); «radici» e «tipo documento» no
//     (T-B4-29). Conflitti: le dimensioni discordanti, che restano visibili (6.4.5 regola 4).
//   - IdentitaParziale, MotiviIdentita: l'incertezza dell'identità dei nodi su cui il candidato sta (per il livello
//     prodotto le radici delle strutture del target): la proposta non aspetta la nomenclatura e la porta accanto
//     (T-E1-03, PO-20; campi in più, T-B4-10).
//   - Motivo: MotivoCandidato*.
type CandidatoAncoraggio struct {
	Target              string                    `json:"target"`
	Livello             string                    `json:"livello"`
	Autorita            Autorita                  `json:"autorita"`
	Origine             OrigineDato               `json:"origine"`
	RadiciDelFile       []string                  `json:"radici_del_file,omitempty"`
	RadiciRaggiungibili []string                  `json:"radici_raggiungibili,omitempty"`
	Padri               []string                  `json:"padri,omitempty"`
	Percorsi            [][]string                `json:"percorsi,omitempty"`
	Archi               []ArcoPercorso            `json:"archi,omitempty"`
	Posizioni           []PosizioneCandidato      `json:"posizioni,omitempty"`
	Letture             []string                  `json:"letture,omitempty"`
	Dimensioni          []DimensioneCompatibilita `json:"dimensioni,omitempty"`
	Conflitti           []string                  `json:"conflitti,omitempty"`
	IdentitaParziale    bool                      `json:"identita_parziale"`
	MotiviIdentita      []string                  `json:"motivi_identita,omitempty"`
	Motivo              string                    `json:"motivo"`
}

// PosizioneCandidato: un nodo di una struttura su cui sta un candidato (T-B4-10): il target, l'allegato e la radice
// della struttura, il nodo, lo stato della struttura (candidata o BOM di lavoro proposta), il motivo (nodo_scartato:
// la riga decisa del nodo è scartata, T-B4-27; "" altrimenti). Il candidato resta: lo scarto del nodo non è uno scarto
// del file (R105), e chi compone lo smistamento lo vede.
type PosizioneCandidato struct {
	Target     string         `json:"target"`
	AllegatoID uuid.UUID      `json:"allegato_id"`
	Radice     string         `json:"radice"`
	Nodo       string         `json:"nodo"`
	Stato      StatoStruttura `json:"stato"`
	Motivo     string         `json:"motivo,omitempty"`
}

// DimensioneCompatibilita: una dimensione di un candidato (6.4.5 regola 4): il confronto di una lettura del file con
// il target, con il nodo o con la decisione del nodo. Esito: uguale, equivalente, compatibile_parziale, discordante o
// non_determinabile (motorea.Compatibilita). Le revisioni con ConfrontaRevisioni, senza ordinarle; i qualificatori
// discordano solo se espliciti da tutte e due le parti (P1 §7.4 r.300). Rif: il target, il nodo, il componente o la
// riga con cui si confronta.
type DimensioneCompatibilita struct {
	Dimensione string                `json:"dimensione"`
	Esito      motorea.Compatibilita `json:"esito"`
	Lettura    string                `json:"lettura"`
	Rif        string                `json:"rif"`
}

// ---- ProponiAncoraggi ----

// ProponiAncoraggi: gli ancoraggi dei file (6.4.5, regole 3-8; par.3.3.7). Prima le strutture dei target, con
// ProponiStrutture (regole 1 e 2; T-B2-01); poi, per ogni file:
//
//  1. Le letture d'identità: quelle con la funzione identita_file. Le menzioni del testo del PDF («SPECCHIATO DI …»),
//     le relazioni e i nodi interni di uno STEP non sono identità.
//  2. Candidati a livello prodotto: un target con la base di un'identità del file (stesso namespace, ConfrontaBasi
//     uguale, equivalente o compatibile parziale) e il marcatore compatibile con ProdottoRichiesto.Marcatore (la regola
//     unica, T-B4-06 rivisto, T-B4-30: due marcatori scritti e diversi sono un'altra identità), con «prodotto» fra i ruoli
//     candidati della lettura. Con la bom_di_lavoro_proposta sotto una radice scelta senza la base del prodotto, anche
//     un'identità del file che è quella della radice (motivo radice_scelta; la base discordante con il target resta
//     visibile nelle dimensioni: R76 A, T-B4-25).
//  3. Candidati a livello componente: le identità raggiungibili, cioè i nodi delle strutture dei target (la radice
//     esclusa quando è il prodotto) raggruppati per namespace, base, marcatore e revisione letta (un nodo senza
//     marcatore sta con l'unico marcatore scritto della sua base). Un candidato per identità compatibile (stesso
//     namespace, base compatibile, marcatore compatibile: R86, T-B4-30), con «componente» fra i ruoli candidati della
//     lettura, con tutte le radici raggiungibili, i padri, i percorsi e gli archi (FIGLIO-CONDIVISO).
//     La stessa base con revisioni lette diverse, senza equivalenza dichiarata, dà identità distinte; una formazione
//     STEP non è una revisione e non divide (D1). Un nodo senza revisione letta sta con l'unica identità della sua base
//     che ne ha una, altrimenti a sé.
//  4. Nessun candidato debole (E-18): nessuna somiglianza di stringhe, nessun involucro nuovo.
//  5. L'associazione: discordante se il file ha identità che non concordano (R64 A, T-B0-35: due letture dello stesso
//     soggetto con basi, marcatori scritti o revisioni lette diverse; il nome di uno STEP che non ha base e marcatore
//     compatibili con nessuna radice; una base ripetuta che non concorda, D2), anche con un candidato solo; altrimenti nessun_candidato,
//     candidato_unico o ambiguo (A-C07: una forma parziale con più completamenti; un file sia fra i target sia fra i
//     figli di un altro target). Nessuna scelta a parità (T-E1-07); il cartiglio non sposta da solo l'ancoraggio (R64
//     A): i candidati di tutte le identità restano.
//  6. La collocazione: radice o figlio dai livelli dei candidati (non determinabile se sono tutti e due); senza
//     candidati, fuori_richiesta solo con l'identità letta, i target noti e lo scenario completo, altrimenti non
//     determinabile, con i motivi. I file «solo parti» dei figli non contano: contano le strutture dei target.
//  7. Le dimensioni di ogni candidato, e la compatibilità con il codice confermato o manuale del nodo, che si ricalcola
//     a ogni fotografia: dopo una correzione del codice il nodo e il componente restano gli stessi (T-E1-04), e un file
//     che non è più compatibile lo dice con il conflitto della dimensione (T-E1-05; il conflitto composto è di B6).
//
// La pre-associazione (R85) viene da sé: le strutture sono candidate finché la fonte non è confermata, e dopo quella
// sotto la radice scelta è la BOM di lavoro, nello stesso calcolo (Posizioni.Stato). SenzaFile dei nodi si ricalcola
// dai candidati: un nodo con almeno un file candidato, o la radice di una struttura che è la radice del suo STEP, non è
// senza file. HASH-CONFLITTO: un ancoraggio per allegato, mai fusi. Le impronte con gli ingressi in ordine canonico.
//
// Poi la riconciliazione (fase 3; riconcilia): per ogni nodo con un 2D associato (FileInterpretato.Disegno: un
// candidato con una posizione sul nodo, o un'associazione decisa sul componente del nodo), il codice documentale del
// cartiglio con l'esito, la correzione accanto e la discordanza con la decisione; il cartiglio come candidato di
// revisione per l'entità del nodo; le diagnostiche dei completamenti e delle correzioni. Niente si applica da solo: la
// catena, le decisioni e gli ancoraggi restano quelli di prima (R64 A, R87).
//
// È pura e deterministica: l'ordine degli ingressi non conta, e gli ingressi di chi chiama non cambiano. L'errore è
// solo di contratto (*evidenze.ErroreContratto): quelli di ProponiStrutture e, per i file, un allegato ripetuto o
// senza ID, un documento che non è quello dell'allegato, un'interpretazione di un altro documento, una lettura su
// un'unità che il documento non ha, una disponibilità fuori elenco, la struttura del contesto di un altro contenuto
// dello stesso allegato; per le associazioni decise un'origine che non è manuale né confermato, un allegato o un
// componente assente, un allegato che non è fra i file, una confermata senza documento, un'associazione ripetuta.
func ProponiAncoraggi(file []FileInterpretato, target []ProdottoRichiesto, ctx ContestoStrutturale) (EsitoAncoraggi, error) {
	d := controllaFile(file, ctx)
	d = append(d, controllaContesto(target, ctx)...)
	if len(d) > 0 {
		return EsitoAncoraggi{}, &evidenze.ErroreContratto{Diagnostiche: d}
	}
	strutture, diag, err := ProponiStrutture(target, ctx)
	if err != nil {
		return EsitoAncoraggi{}, err
	}
	files := append([]FileInterpretato(nil), file...)
	sort.SliceStable(files, func(i, j int) bool { return files[i].AllegatoID.String() < files[j].AllegatoID.String() })

	a := nuovoAncoratore(target, ctx, strutture)
	esito := EsitoAncoraggi{VersioneServizio: VersioneServizio, Strutture: strutture, Diagnostiche: diag}
	for _, f := range files {
		esito.File = append(esito.File, a.ancora(f))
	}
	ricalcolaSenzaFile(esito.Strutture, esito.File)
	esito.Diagnostiche = append(esito.Diagnostiche, riconcilia(esito.Strutture, esito.File, files, target, ctx)...)
	sort.SliceStable(esito.Diagnostiche, func(i, j int) bool {
		return chiaveDiagnostica(esito.Diagnostiche[i]) < chiaveDiagnostica(esito.Diagnostiche[j])
	})
	return chiudiAncoraggi(esito, files, target, ctx)
}

// ---- il calcolo ----

// ancoratore: i target, le strutture dei target con i loro indici, le identità raggiungibili, i motivi dello scenario.
// Le mappe servono solo a cercare, mai a scorrere.
type ancoratore struct {
	target     []ProdottoRichiesto
	autorita   map[string]Autorita
	strutture  []StrutturaProdotto
	file       map[uuid.UUID]StrutturaFile
	letture    map[uuid.UUID]map[string][]motorea.LetturaCodice // per allegato e nodo
	padri      []map[string][]ArcoProposto                      // per struttura: figlio → archi entranti
	identita   []*identitaRaggiungibile
	motiviNonD []string // i motivi dello scenario che impediscono fuori_richiesta
}

// identitaRaggiungibile: un'identità dei nodi delle strutture dei target (regola 3): namespace, base, marcatore e
// revisione letta (nil = nessuna), con i nodi che la portano.
type identitaRaggiungibile struct {
	namespace, base, marcatore string
	baseLetta                  motorea.BaseLetta
	revisione                  *motorea.RevisioneLetta
	nodi                       []nodoRaggiungibile
}

// nodoRaggiungibile: un nodo di una struttura con la sua lettura d'identità.
type nodoRaggiungibile struct {
	s int // indice della struttura
	n int // indice del nodo nella struttura
	l motorea.LetturaCodice
}

func nuovoAncoratore(target []ProdottoRichiesto, ctx ContestoStrutturale, strutture []StrutturaProdotto) *ancoratore {
	a := &ancoratore{autorita: map[string]Autorita{}, strutture: strutture, file: map[uuid.UUID]StrutturaFile{},
		letture: map[uuid.UUID]map[string][]motorea.LetturaCodice{}}
	a.target = append([]ProdottoRichiesto(nil), target...)
	sort.SliceStable(a.target, func(i, j int) bool { return a.target[i].Rif < a.target[j].Rif })
	for _, t := range a.target {
		a.autorita[t.Rif] = t.Autorita
	}
	for _, s := range ctx.Strutture {
		a.file[s.AllegatoID] = s
		perNodo := map[string][]motorea.LetturaCodice{}
		for _, n := range s.Nodi {
			l := append([]motorea.LetturaCodice(nil), n.Letture...)
			sort.SliceStable(l, func(i, j int) bool { return l[i].ID < l[j].ID })
			perNodo[n.Rif] = l
		}
		a.letture[s.AllegatoID] = perNodo
	}
	for i := range strutture {
		p := map[string][]ArcoProposto{}
		for _, arco := range strutture[i].Archi {
			p[arco.Figlio] = append(p[arco.Figlio], arco)
		}
		a.padri = append(a.padri, p)
	}
	a.identitaRaggiungibili()

	// I motivi dello scenario (regola 3): senza target non si sa niente; un target senza strutture o con una struttura
	// incompleta non permette di dire che un file è fuori richiesta.
	if len(a.target) == 0 {
		a.motiviNonD = append(a.motiviNonD, MotivoAncoraggioTargetIgnoti)
	}
	for _, t := range a.target {
		proprie := 0
		for _, s := range strutture {
			if s.Target != t.Rif {
				continue
			}
			proprie++
			if !s.GrafoCompleto {
				a.motiviNonD = append(a.motiviNonD, MotivoAncoraggioGrafoIncompleto)
			}
		}
		if proprie == 0 {
			a.motiviNonD = append(a.motiviNonD, MotivoAncoraggioTargetSenzaStruttura)
		}
	}
	a.motiviNonD = unici(a.motiviNonD)
	return a
}

// radiceProdotto: la radice della struttura rappresenta il prodotto: ha la sua base (regola 1), o è la radice scelta
// dall'operatore per la BOM di lavoro (R76 A). Una radice così non è un nodo da componente.
func radiceProdotto(s StrutturaProdotto) bool {
	return compatibile(s.Compatibilita) || s.Stato == StatoBOMDiLavoroProposta
}

// identitaRaggiungibili raggruppa le letture d'identità dei nodi delle strutture dei target (regola 3), per namespace e
// base, poi per marcatore con la regola unica del marcatore (T-B4-06 rivisto, T-B4-30: un nodo senza marcatore sta con
// l'unico marcatore scritto della sua base, altrimenti a sé), poi per revisione letta (un nodo senza revisione sta con
// l'unica revisione letta del suo gruppo, altrimenti a sé). Due marcatori scritti e diversi, o due revisioni lette
// diverse senza equivalenza dichiarata, sono identità distinte; una formazione STEP non divide (D1). Un file con un
// altro marcatore scritto si abbina lo stesso ai nodi senza marcatore del gruppo (senzaMarcatore).
func (a *ancoratore) identitaRaggiungibili() {
	type secchio struct {
		nodi []nodoRaggiungibile
	}
	var secchi []*secchio
	perChiave := map[string]*secchio{}
	for si, s := range a.strutture {
		for ni, n := range s.Nodi {
			if n.Rif == s.Radice && radiceProdotto(s) {
				continue
			}
			for _, l := range a.letture[s.AllegatoID][n.Rif] {
				k := l.Forma.Namespace + "\x00" + l.Forma.Base.Normalizzata
				b, ok := perChiave[k]
				if !ok {
					b = &secchio{}
					perChiave[k] = b
					secchi = append(secchi, b)
				}
				b.nodi = append(b.nodi, nodoRaggiungibile{s: si, n: ni, l: l})
			}
		}
	}
	for _, b := range secchi {
		var marcatori []string
		perMarcatore := map[string][]nodoRaggiungibile{}
		var senzaMarcatore []nodoRaggiungibile
		for _, x := range b.nodi {
			m := marcatoreDi(x.l.Forma)
			if m == "" {
				senzaMarcatore = append(senzaMarcatore, x)
				continue
			}
			if _, ok := perMarcatore[m]; !ok {
				marcatori = append(marcatori, m)
			}
			perMarcatore[m] = append(perMarcatore[m], x)
		}
		sort.Strings(marcatori)
		switch {
		case len(senzaMarcatore) == 0:
		case len(marcatori) == 1:
			perMarcatore[marcatori[0]] = append(perMarcatore[marcatori[0]], senzaMarcatore...)
		default:
			marcatori = append(marcatori, "")
			perMarcatore[""] = senzaMarcatore
		}
		for _, m := range marcatori {
			a.identita = append(a.identita, classiDiRevisione(perMarcatore[m], m)...)
		}
	}
	sort.SliceStable(a.identita, func(i, j int) bool { return a.identita[i].rif() < a.identita[j].rif() })
}

// classiDiRevisione divide i nodi di un namespace, una base e un marcatore per revisione letta.
func classiDiRevisione(nodi []nodoRaggiungibile, marcatore string) []*identitaRaggiungibile {
	var classi []*identitaRaggiungibile
	var senza []nodoRaggiungibile
	for _, x := range nodi {
		r := revisioneDi(x.l.Forma)
		if r == nil {
			senza = append(senza, x)
			continue
		}
		var trovata *identitaRaggiungibile
		for _, c := range classi {
			if v := motorea.ConfrontaRevisioni(*c.revisione, *r); v == motorea.CompatibilitaUguale || v == motorea.CompatibilitaEquivalente {
				trovata = c
				break
			}
		}
		if trovata == nil {
			trovata = nuovaIdentita(x, marcatore, r)
			classi = append(classi, trovata)
		}
		trovata.nodi = append(trovata.nodi, x)
	}
	switch {
	case len(senza) == 0:
	case len(classi) == 1:
		classi[0].nodi = append(classi[0].nodi, senza...)
	default:
		c := nuovaIdentita(senza[0], marcatore, nil)
		c.nodi = senza
		classi = append(classi, c)
	}
	return classi
}

// senzaMarcatore: la parte del gruppo fatta dei nodi che non scrivono il marcatore, quando il gruppo ha il marcatore
// scritto dagli altri nodi (identitaRaggiungibili li ha uniti con la regola unica); nil se non ce ne sono, o se il gruppo
// non ha marcatore. Un file con un altro marcatore scritto è compatibile solo con loro.
func (g *identitaRaggiungibile) senzaMarcatore() *identitaRaggiungibile {
	if g.marcatore == "" {
		return nil
	}
	var nodi []nodoRaggiungibile
	for _, x := range g.nodi {
		if marcatoreDi(x.l.Forma) == "" {
			nodi = append(nodi, x)
		}
	}
	if len(nodi) == 0 {
		return nil
	}
	return &identitaRaggiungibile{namespace: g.namespace, base: g.base, baseLetta: g.baseLetta, revisione: g.revisione, nodi: nodi}
}

func nuovaIdentita(x nodoRaggiungibile, marcatore string, r *motorea.RevisioneLetta) *identitaRaggiungibile {
	return &identitaRaggiungibile{namespace: x.l.Forma.Namespace, base: x.l.Forma.Base.Normalizzata, marcatore: marcatore,
		baseLetta: x.l.Forma.Base, revisione: r}
}

// rif: il riferimento dell'identità (RifIdentita).
func (g *identitaRaggiungibile) rif() string {
	rev := ""
	if g.revisione != nil {
		rev = g.revisione.Normalizzata
	}
	return RifIdentita(g.namespace, g.base, g.marcatore, rev)
}

// sostegno: una lettura del file che sostiene un candidato, con la base che si è confrontata (la base, o una sua
// ripetizione che non concorda: D2). Per il candidato sulla radice scelta (T-B4-25) anche la radice e la sua lettura.
type sostegno struct {
	l      motorea.LetturaCodice
	base   motorea.BaseLetta
	radice string
	lr     motorea.LetturaForma
}

// sostegnoRadiceScelta (T-B4-25; R76 A): con la bom_di_lavoro_proposta di un target sotto una radice scelta che non ha
// la base del prodotto, la lettura del file che ha l'identità della radice (lo stesso namespace, una base compatibile,
// il marcatore compatibile: T-B4-30) sostiene il candidato a livello prodotto: la radice l'ha scelta l'operatore con il
// gesto 3, e il suo file non è fuori richiesta. Con la radice compatibile vale la base del target, come sempre.
func (a *ancoratore) sostegnoRadiceScelta(t ProdottoRichiesto, l motorea.LetturaCodice) (sostegno, bool) {
	for _, s := range a.strutture {
		if s.Target != t.Rif || s.Stato != StatoBOMDiLavoroProposta || compatibile(s.Compatibilita) {
			continue
		}
		for _, r := range a.letture[s.AllegatoID][s.Radice] {
			if r.Forma.Namespace != l.Forma.Namespace || !marcatoriCompatibili(marcatoreDi(r.Forma), marcatoreDi(l.Forma)) {
				continue
			}
			for _, b := range basiDi(l.Forma) {
				if compatibile(motorea.ConfrontaBasi(r.Forma.Base, b)) {
					return sostegno{l: l, base: b, radice: s.Radice, lr: r.Forma}, true
				}
			}
		}
	}
	return sostegno{}, false
}

// ancora calcola l'ancoraggio di un file.
func (a *ancoratore) ancora(f FileInterpretato) AncoraggioFile {
	af := AncoraggioFile{AllegatoID: f.AllegatoID, Disponibilita: f.Disponibilita}
	ident := lettureIdentita(f.Interpretazione)
	for _, l := range ident {
		af.Letture = append(af.Letture, l.ID)
	}
	var radiciDelFile []string
	if s, ok := a.file[f.AllegatoID]; ok {
		radiciDelFile = append(radiciDelFile, s.Radici...)
		sort.Strings(radiciDelFile)
	}

	var candidati []CandidatoAncoraggio
	var identita []identitaCandidata
	for _, t := range a.target {
		var sost, scelta []sostegno
		for _, l := range ident {
			if !haRuolo(l.RuoliCandidati, grammatica.RuoloProdotto) {
				continue
			}
			// La regola unica dell'identità (T-B4-06 rivisto, T-B4-30): un marcatore scritto dal file e uno diverso scritto
			// dal target sono un'altra identità, anche con la stessa base.
			if l.Forma.Namespace == t.Namespace && marcatoriCompatibili(t.Marcatore, marcatoreDi(l.Forma)) {
				for _, b := range basiDi(l.Forma) {
					if compatibile(motorea.ConfrontaBasi(t.Base, b)) {
						sost = append(sost, sostegno{l: l, base: b})
						break
					}
				}
			}
			// La radice scelta dall'operatore per la BOM di lavoro, anche senza la base del prodotto (R76 A, T-B4-25):
			// un'identità del file che è quella della radice è il prodotto.
			if x, ok := a.sostegnoRadiceScelta(t, l); ok {
				scelta = append(scelta, x)
			}
		}
		if len(sost) > 0 || len(scelta) > 0 {
			candidati = append(candidati, a.candidatoProdotto(t, sost, scelta))
			identita = append(identita, identitaCandidata{livello: LivelloProdotto, namespace: t.Namespace, base: t.Base.Normalizzata})
		}
	}
	for _, g := range a.identita {
		// Una lettura con il marcatore del gruppo (o senza) sostiene il gruppo intero; una con un altro marcatore scritto
		// sostiene solo i nodi del gruppo che non scrivono il marcatore, compatibili con lei da una parte sola (la regola
		// unica, T-B4-06 rivisto, T-B4-30): il file non perde il nodo e non finisce fuori richiesta.
		var sost, sostSenza []sostegno
		for _, l := range ident {
			if !haRuolo(l.RuoliCandidati, grammatica.RuoloComponente) || l.Forma.Namespace != g.namespace {
				continue
			}
			for _, b := range basiDi(l.Forma) {
				if !compatibile(motorea.ConfrontaBasi(g.baseLetta, b)) {
					continue
				}
				if marcatoriCompatibili(marcatoreDi(l.Forma), g.marcatore) {
					sost = append(sost, sostegno{l: l, base: b})
				} else {
					sostSenza = append(sostSenza, sostegno{l: l, base: b})
				}
				break
			}
		}
		for _, x := range []struct {
			g    *identitaRaggiungibile
			sost []sostegno
		}{{g, sost}, {g.senzaMarcatore(), sostSenza}} {
			if x.g == nil || len(x.sost) == 0 {
				continue
			}
			candidati = append(candidati, a.candidatoComponente(x.g, x.sost))
			rev := ""
			if x.g.revisione != nil {
				rev = x.g.revisione.Normalizzata
			}
			identita = append(identita, identitaCandidata{livello: LivelloComponente, namespace: x.g.namespace, base: x.g.base, marcatore: x.g.marcatore, revisione: rev})
		}
	}
	for i := range candidati {
		candidati[i].RadiciDelFile = copiaStringhe(radiciDelFile)
	}
	ordinaCandidatiAncoraggio(candidati)
	af.Candidati = candidati

	discordante := identitaDiscordanti(f.Documento, ident)
	livelli := map[string]bool{}
	for _, c := range candidati {
		livelli[c.Livello] = true
	}
	switch {
	case discordante:
		af.Associazione = AssociazioneDiscordante
		af.Motivi = append(af.Motivi, MotivoAncoraggioIdentitaDiscordanti)
	case len(candidati) == 0:
		af.Associazione = AssociazioneNessunCandidato
	case len(candidati) == 1:
		af.Associazione = AssociazioneCandidatoUnico
	default:
		af.Associazione = AssociazioneAmbiguo
		af.Motivi = append(af.Motivi, motiviAmbiguita(identita, ident)...)
	}
	switch {
	case len(candidati) > 0 && len(livelli) > 1:
		af.Collocazione = CollocazioneNonDeterminabile
		af.Motivi = append(af.Motivi, MotivoAncoraggioLivelliDiversi)
	case livelli[LivelloProdotto]:
		af.Collocazione = CollocazioneRadice
	case livelli[LivelloComponente]:
		af.Collocazione = CollocazioneFiglio
	case len(ident) == 0:
		af.Collocazione = CollocazioneNonDeterminabile
		af.Motivi = append(af.Motivi, MotivoAncoraggioNessunaLettura)
		af.Motivi = append(af.Motivi, a.motiviNonD...)
	case len(a.motiviNonD) > 0:
		af.Collocazione = CollocazioneNonDeterminabile
		af.Motivi = append(af.Motivi, a.motiviNonD...)
	default:
		af.Collocazione = CollocazioneFuoriRichiesta
	}
	af.Motivi = unici(af.Motivi)
	return af
}

// candidatoProdotto: il candidato a livello prodotto, con le radici delle strutture del target che rappresentano il
// prodotto e la loro incertezza (T-E1-03). I sostegni sono quelli della base del target (motivo base_del_target) e
// quelli della radice scelta senza la base del prodotto (T-B4-25: motivo radice_scelta solo se mancano i primi); per
// questi le dimensioni dicono il confronto con il target, la base discordante compresa, e con la radice.
func (a *ancoratore) candidatoProdotto(t ProdottoRichiesto, sost, scelta []sostegno) CandidatoAncoraggio {
	c := CandidatoAncoraggio{Target: t.Rif, Livello: LivelloProdotto, Autorita: t.Autorita, Origine: OrigineProposto,
		RadiciRaggiungibili: []string{t.Rif}, Motivo: MotivoCandidatoBaseDelTarget}
	if len(sost) == 0 {
		c.Motivo = MotivoCandidatoRadiceScelta
	}
	var motivi []string
	for _, s := range a.strutture {
		if s.Target != t.Rif || !radiceProdotto(s) {
			continue
		}
		p := PosizioneCandidato{Target: t.Rif, AllegatoID: s.AllegatoID, Radice: s.Radice, Nodo: s.Radice, Stato: s.Stato}
		if r, ok := nodoDiStruttura(s, s.Radice); ok {
			c.IdentitaParziale = c.IdentitaParziale || r.Codice.Identita.Parziale
			if r.Codice.Identita.Motivo != "" {
				motivi = append(motivi, r.Codice.Identita.Motivo)
			}
			p.Motivo = motivoPosizione(r)
		}
		c.Posizioni = append(c.Posizioni, p)
	}
	c.MotiviIdentita = unici(motivi)
	for _, x := range append(append([]sostegno(nil), sost...), scelta...) {
		f := x.l.Forma
		c.Letture = append(c.Letture, x.l.ID)
		base, marcatore := motorea.ConfrontaBasi(t.Base, x.base), compatibilitaMarcatori(t.Marcatore, marcatoreDi(f))
		if t.Namespace != f.Namespace {
			base, marcatore = motorea.CompatibilitaNonDeterminabile, motorea.CompatibilitaNonDeterminabile
		}
		c.Dimensioni = append(c.Dimensioni,
			DimensioneCompatibilita{Dimensione: DimensioneNamespace, Esito: confrontaNamespace(t.Namespace, f.Namespace), Lettura: x.l.ID, Rif: t.Rif},
			DimensioneCompatibilita{Dimensione: DimensioneBase, Esito: base, Lettura: x.l.ID, Rif: t.Rif},
			DimensioneCompatibilita{Dimensione: DimensioneMarcatore, Esito: marcatore, Lettura: x.l.ID, Rif: t.Rif},
			DimensioneCompatibilita{Dimensione: DimensioneRevisione, Esito: confrontaRevisioni(t.Revisione, f.Revisione), Lettura: x.l.ID, Rif: t.Rif},
			DimensioneCompatibilita{Dimensione: DimensioneQualificatori, Esito: confrontaQualificatori(t.Qualificatori, f.Affissi), Lettura: x.l.ID, Rif: t.Rif})
		if x.radice != "" {
			c.Dimensioni = append(c.Dimensioni,
				DimensioneCompatibilita{Dimensione: DimensioneBase, Esito: motorea.ConfrontaBasi(x.lr.Base, x.base), Lettura: x.l.ID, Rif: x.radice},
				DimensioneCompatibilita{Dimensione: DimensioneMarcatore, Esito: compatibilitaMarcatori(marcatoreDi(x.lr), marcatoreDi(f)), Lettura: x.l.ID, Rif: x.radice},
				DimensioneCompatibilita{Dimensione: DimensioneRevisione, Esito: confrontaRevisioni(x.lr.Revisione, f.Revisione), Lettura: x.l.ID, Rif: x.radice})
		}
	}
	chiudiCandidato(&c)
	return c
}

// confrontaNamespace: lo stesso namespace è uguale; un altro (per la radice scelta, letta da un'altra famiglia) non è
// determinabile, come in ConfrontaBasi.
func confrontaNamespace(a, b string) motorea.Compatibilita {
	if a == b {
		return motorea.CompatibilitaUguale
	}
	return motorea.CompatibilitaNonDeterminabile
}

// motivoPosizione: il motivo di una posizione di un candidato: nodo_scartato quando la riga decisa del nodo è scartata
// (T-B4-27); "" altrimenti.
func motivoPosizione(n NodoProposto) string {
	if n.RigaDecisa != nil && n.RigaDecisa.Stato == StatoRigaScartata {
		return MotivoNodoScartato
	}
	return ""
}

// candidatoComponente: il candidato a livello componente su un'identità raggiungibile: tutte le sue posizioni, le
// radici raggiungibili, i padri, i percorsi e gli archi (FIGLIO-CONDIVISO), le dimensioni con i nodi e con le loro
// decisioni.
func (a *ancoratore) candidatoComponente(g *identitaRaggiungibile, sost []sostegno) CandidatoAncoraggio {
	c := CandidatoAncoraggio{Livello: LivelloComponente, Origine: OrigineProposto, Motivo: MotivoCandidatoBaseDelNodo}
	var motivi []string
	nodi := map[string]bool{}
	componenti := map[string]bool{}
	tuttiDecisi := true
	visti := map[string]bool{}  // posizioni
	conDim := map[string]bool{} // nodi già confrontati
	var perDim []nodoRaggiungibile
	for _, x := range g.nodi {
		s := a.strutture[x.s]
		n := s.Nodi[x.n]
		k := s.Target + "\x00" + s.AllegatoID.String() + "\x00" + s.Radice + "\x00" + n.Rif
		if !visti[k] {
			visti[k] = true
			c.Posizioni = append(c.Posizioni, PosizioneCandidato{Target: s.Target, AllegatoID: s.AllegatoID, Radice: s.Radice, Nodo: n.Rif, Stato: s.Stato,
				Motivo: motivoPosizione(n)})
			c.RadiciRaggiungibili = append(c.RadiciRaggiungibili, s.Target)
			c.Padri = append(c.Padri, n.Padri...)
			for _, p := range a.percorsi(x.s, n.Rif) {
				c.Percorsi = append(c.Percorsi, append([]string{s.Target}, p...))
				c.Archi = append(c.Archi, a.archiDelPercorso(x.s, p)...)
			}
			c.IdentitaParziale = c.IdentitaParziale || n.Codice.Identita.Parziale
			if n.Codice.Identita.Motivo != "" {
				motivi = append(motivi, n.Codice.Identita.Motivo)
			}
			nodi[n.Rif] = true
			if n.Decisione != nil {
				componenti[RifComponente(n.Decisione.ComponenteID)] = true
			} else {
				tuttiDecisi = false
			}
		}
		if !conDim[n.Rif+"\x00"+x.l.ID] {
			conDim[n.Rif+"\x00"+x.l.ID] = true
			perDim = append(perDim, x)
		}
	}
	c.MotiviIdentita = unici(motivi)
	switch {
	case tuttiDecisi && len(componenti) == 1:
		for k := range componenti {
			c.Target = k
		}
	case len(nodi) == 1:
		for k := range nodi {
			c.Target = k
		}
	default:
		c.Target = g.rif()
	}
	c.Autorita = AutoritaConfermata
	for _, r := range c.RadiciRaggiungibili {
		if rangoAutorita(a.autorita[r]) < rangoAutorita(c.Autorita) {
			c.Autorita = a.autorita[r]
		}
	}

	for _, x := range sost {
		f := x.l.Forma
		c.Letture = append(c.Letture, x.l.ID)
		for _, nr := range perDim {
			n := a.strutture[nr.s].Nodi[nr.n]
			nf := nr.l.Forma
			c.Dimensioni = append(c.Dimensioni,
				DimensioneCompatibilita{Dimensione: DimensioneNamespace, Esito: motorea.CompatibilitaUguale, Lettura: x.l.ID, Rif: n.Rif},
				DimensioneCompatibilita{Dimensione: DimensioneBase, Esito: motorea.ConfrontaBasi(nf.Base, x.base), Lettura: x.l.ID, Rif: n.Rif},
				DimensioneCompatibilita{Dimensione: DimensioneMarcatore, Esito: compatibilitaMarcatori(marcatoreDi(nf), marcatoreDi(f)), Lettura: x.l.ID, Rif: n.Rif},
				DimensioneCompatibilita{Dimensione: DimensioneRevisione, Esito: confrontaRevisioni(nf.Revisione, f.Revisione), Lettura: x.l.ID, Rif: n.Rif},
				DimensioneCompatibilita{Dimensione: DimensioneQualificatori, Esito: confrontaQualificatori(nf.Affissi, f.Affissi), Lettura: x.l.ID, Rif: n.Rif})
			if n.Decisione != nil {
				c.Dimensioni = append(c.Dimensioni, DimensioneCompatibilita{Dimensione: DimensioneCodiceConfermato,
					Esito: compatibilitaConDecisione(f, x.base, n.Decisione.Lettura, n.Codice.Confermato), Lettura: x.l.ID, Rif: RifComponente(n.Decisione.ComponenteID)})
			}
			if n.RigaLegacy != nil && n.RigaLegacy.CodiceManuale != nil {
				c.Dimensioni = append(c.Dimensioni, DimensioneCompatibilita{Dimensione: DimensioneCodiceManuale,
					Esito: compatibilitaConDecisione(f, x.base, n.RigaLegacy.LetturaManuale, n.Codice.Manuale), Lettura: x.l.ID, Rif: RifRigaProposta(n.RigaLegacy.ID)})
			}
		}
	}
	chiudiCandidato(&c)
	return c
}

// chiudiCandidato mette in ordine canonico gli elenchi del candidato, senza doppioni, e ne ricava i conflitti.
func chiudiCandidato(c *CandidatoAncoraggio) {
	c.RadiciRaggiungibili = unici(c.RadiciRaggiungibili)
	c.Padri = unici(c.Padri)
	c.Letture = unici(c.Letture)
	sort.SliceStable(c.Percorsi, func(i, j int) bool {
		return strings.Join(c.Percorsi[i], "\x00") < strings.Join(c.Percorsi[j], "\x00")
	})
	var percorsi [][]string
	for i, p := range c.Percorsi {
		if i > 0 && strings.Join(p, "\x00") == strings.Join(c.Percorsi[i-1], "\x00") {
			continue
		}
		percorsi = append(percorsi, p)
	}
	c.Percorsi = percorsi
	c.Archi = archiInOrdine(c.Archi)
	if len(c.Archi) == 0 {
		c.Archi = nil
	}
	sort.SliceStable(c.Posizioni, func(i, j int) bool {
		return chiavePosizioneCandidato(c.Posizioni[i]) < chiavePosizioneCandidato(c.Posizioni[j])
	})
	sort.SliceStable(c.Dimensioni, func(i, j int) bool { return chiaveDimensione(c.Dimensioni[i]) < chiaveDimensione(c.Dimensioni[j]) })
	var dim []DimensioneCompatibilita
	var conflitti []string
	for i, x := range c.Dimensioni {
		if i > 0 && chiaveDimensione(x) == chiaveDimensione(c.Dimensioni[i-1]) {
			continue
		}
		dim = append(dim, x)
		if x.Esito == motorea.CompatibilitaDiscordante {
			conflitti = append(conflitti, x.Dimensione)
		}
	}
	c.Dimensioni = dim
	c.Conflitti = unici(conflitti)
}

func chiavePosizioneCandidato(p PosizioneCandidato) string {
	return p.Target + "\x00" + p.AllegatoID.String() + "\x00" + p.Radice + "\x00" + p.Nodo
}

func chiaveDimensione(d DimensioneCompatibilita) string {
	return d.Dimensione + "\x00" + d.Lettura + "\x00" + d.Rif + "\x00" + string(d.Esito)
}

// ordinaCandidatiAncoraggio: l'ordine del sostegno (T-E1-07): meno conflitti prima, poi il livello prodotto, poi il
// Target e le letture. Un ordine totale e deterministico, mai una scelta: l'associazione lo dice.
func ordinaCandidatiAncoraggio(c []CandidatoAncoraggio) {
	sort.SliceStable(c, func(i, j int) bool {
		a, b := c[i], c[j]
		if len(a.Conflitti) != len(b.Conflitti) {
			return len(a.Conflitti) < len(b.Conflitti)
		}
		if (a.Livello == LivelloProdotto) != (b.Livello == LivelloProdotto) {
			return a.Livello == LivelloProdotto
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		return strings.Join(a.Letture, "\x00") < strings.Join(b.Letture, "\x00")
	})
}

// percorsi: i percorsi dalla radice della struttura al nodo negli archi dei fatti, ognuno dalla radice al nodo. In un
// grafo con un ciclo un nodo non si ripete in un percorso. Un limite noto: i percorsi sono quante le occorrenze del
// nodo nella distinta esplosa, e in un DAG con molti figli condivisi possono crescere molto; da guardare in B8, con il
// profilo dei limiti.
func (a *ancoratore) percorsi(si int, nodo string) [][]string {
	radice := a.strutture[si].Radice
	var out [][]string
	var visita func(r string, dietro []string)
	visita = func(r string, dietro []string) {
		dietro = append(dietro, r)
		if r == radice {
			p := make([]string, len(dietro))
			for i, x := range dietro {
				p[len(dietro)-1-i] = x
			}
			out = append(out, p)
			return
		}
		for _, arco := range a.padri[si][r] {
			if contiene(dietro, arco.Padre) {
				continue
			}
			visita(arco.Padre, dietro)
		}
	}
	visita(nodo, nil)
	return out
}

// archiDelPercorso: gli archi dei fatti fra i nodi consecutivi del percorso, con l'origine fatti, e accanto l'arco
// confermato quando c'è (T-14).
func (a *ancoratore) archiDelPercorso(si int, p []string) []ArcoPercorso {
	var out []ArcoPercorso
	for i := 1; i < len(p); i++ {
		for _, arco := range a.padri[si][p[i]] {
			if arco.Padre != p[i-1] {
				continue
			}
			out = append(out, ArcoPercorso{Padre: arco.Padre, Figlio: arco.Figlio, Quantita: copiaIntero(arco.Quantita),
				Occorrenze: copiaStringhe(arco.Occorrenze), Origine: OrigineArcoFatti})
			if arco.Decisione != nil {
				out = append(out, copiaArco(*arco.Decisione))
			}
		}
	}
	return out
}

// identitaCandidata: l'identità su cui sta un candidato, per i motivi dell'ambiguità: il namespace, la base, il
// marcatore e la revisione dell'identità raggiungibile; per il livello prodotto il namespace e la base del target.
type identitaCandidata struct {
	livello, namespace, base, marcatore, revisione string
}

// motiviAmbiguita: perché più candidati (regola 3): la stessa base con revisioni lette diverse (v3 §2 r.163), una
// forma parziale del file compatibile con più basi (A-C07), altrimenti candidati multipli. I livelli diversi li dice
// la collocazione.
func motiviAmbiguita(c []identitaCandidata, ident []motorea.LetturaCodice) []string {
	var motivi []string
	basi := map[string]bool{}
	revisioni := map[string]map[string]bool{} // namespace, base, marcatore → revisioni
	livelli := map[string]bool{}
	for _, x := range c {
		livelli[x.livello] = true
		basi[x.namespace+"\x00"+x.base] = true
		if x.livello != LivelloComponente {
			continue
		}
		k := x.namespace + "\x00" + x.base + "\x00" + x.marcatore
		if revisioni[k] == nil {
			revisioni[k] = map[string]bool{}
		}
		revisioni[k][x.revisione] = true
	}
	for _, r := range revisioni {
		if len(r) > 1 {
			motivi = append(motivi, MotivoAncoraggioRevisioniDiscordanti)
			break
		}
	}
	parziale := false
	for _, l := range ident {
		parziale = parziale || !l.Forma.Base.Completa || len(l.Forma.Base.Mancanti) > 0
	}
	if parziale && len(basi) > 1 {
		motivi = append(motivi, MotivoAncoraggioCompletamentiMultipli)
	}
	if len(motivi) == 0 && len(livelli) == 1 {
		motivi = append(motivi, MotivoAncoraggioCandidatiMultipli)
	}
	return motivi
}

// ---- le identità del file ----

// lettureIdentita: le letture d'identità di un file, quelle con la funzione identita_file, in ordine di ID.
func lettureIdentita(r motorea.Interpretazione) []motorea.LetturaCodice {
	var out []motorea.LetturaCodice
	for _, l := range r.Letture {
		if l.Funzione == motorea.FunzIdentitaFile {
			out = append(out, l)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// identitaDiscordanti (R64 A, T-B0-35): il file ha identità che non concordano, nello stesso namespace (letture di
// namespace diversi sono alternative, non identità discordanti):
//   - una lettura con una ripetizione della base che non concorda (D2);
//   - due letture dello stesso soggetto (il file: nome, cartiglio, voce d'archivio; oppure una stessa radice STEP)
//     con basi discordanti, due marcatori scritti e diversi (T-B4-30) o revisioni lette discordanti, anche solo la
//     revisione (T-B0-35). Due letture alternative della stessa occorrenza non sono due identità;
//   - il nome di uno STEP che non ha base e marcatore compatibile (T-B4-30) con nessuna delle sue radici: fra la radice e il
//     nome la discordanza si guarda solo su base e marcatore (T-B0-35); un marcatore scritto da una parte sola non
//     discorda. Radici diverse dello stesso STEP non sono discordanti: danno candidati distinti (C-21).
func identitaDiscordanti(d evidenze.DocumentoEvidenze, ident []motorea.LetturaCodice) bool {
	unita := make(map[string]evidenze.UnitaEvidenza, len(d.Unita))
	for _, u := range d.Unita {
		unita[u.ID] = u
	}
	type soggetti struct {
		file   []motorea.LetturaCodice
		radici map[string][]motorea.LetturaCodice
		ordine []string
	}
	perNS := map[string]*soggetti{}
	var namespace []string
	for _, l := range ident {
		for _, r := range l.Forma.Ripetizioni {
			if !r.Concorda {
				return true
			}
		}
		ns := l.Forma.Namespace
		s, ok := perNS[ns]
		if !ok {
			s = &soggetti{radici: map[string][]motorea.LetturaCodice{}}
			perNS[ns] = s
			namespace = append(namespace, ns)
		}
		u := unita[l.UnitaID]
		if u.Selettore.Contesto == evidenze.ContestoRadiceSTEP && u.EntitaID != "" {
			if _, ok := s.radici[u.EntitaID]; !ok {
				s.ordine = append(s.ordine, u.EntitaID)
			}
			s.radici[u.EntitaID] = append(s.radici[u.EntitaID], l)
			continue
		}
		s.file = append(s.file, l)
	}
	for _, ns := range namespace {
		s := perNS[ns]
		if discordantiFra(s.file) {
			return true
		}
		for _, e := range s.ordine {
			if discordantiFra(s.radici[e]) {
				return true
			}
		}
		if len(s.radici) == 0 {
			continue
		}
		for _, l := range s.file {
			concorda := false
			for _, e := range s.ordine {
				for _, r := range s.radici[e] {
					b := motorea.ConfrontaBasi(l.Forma.Base, r.Forma.Base)
					concorda = concorda || (b != motorea.CompatibilitaDiscordante && b != motorea.CompatibilitaNonDeterminabile &&
						marcatoriCompatibili(marcatoreDi(l.Forma), marcatoreDi(r.Forma)))
				}
			}
			if !concorda {
				return true
			}
		}
	}
	return false
}

// discordantiFra: due letture dello stesso soggetto con basi discordanti, due marcatori scritti e diversi (T-B4-30) o
// revisioni lette discordanti, anche solo la revisione (T-B0-35); due letture alternative della stessa occorrenza
// (stessa unità, intervalli che si toccano) non contano.
func discordantiFra(l []motorea.LetturaCodice) bool {
	for i := range l {
		for j := i + 1; j < len(l); j++ {
			x, y := l[i], l[j]
			if x.UnitaID == y.UnitaID && x.Occorrenza.Inizio < y.Occorrenza.Fine && y.Occorrenza.Inizio < x.Occorrenza.Fine {
				continue
			}
			if motorea.ConfrontaBasi(x.Forma.Base, y.Forma.Base) == motorea.CompatibilitaDiscordante || !marcatoriCompatibili(marcatoreDi(x.Forma), marcatoreDi(y.Forma)) ||
				confrontaRevisioni(x.Forma.Revisione, y.Forma.Revisione) == motorea.CompatibilitaDiscordante {
				return true
			}
		}
	}
	return false
}

// basiDi: le basi di una lettura: la base e, quando una ripetizione non concorda, anche la sua (D2: nessuna scelta
// della prima).
func basiDi(f motorea.LetturaForma) []motorea.BaseLetta {
	out := []motorea.BaseLetta{f.Base}
	for _, r := range f.Ripetizioni {
		if !r.Concorda {
			out = append(out, r.Base)
		}
	}
	return out
}

// marcatoreDi: il valore del marcatore di una lettura; "" senza marcatore.
func marcatoreDi(f motorea.LetturaForma) string {
	if f.Marcatore == nil {
		return ""
	}
	return f.Marcatore.Valore
}

// revisioneDi: la revisione di una lettura, se è «letta»; nil altrimenti.
func revisioneDi(f motorea.LetturaForma) *motorea.RevisioneLetta {
	if f.Revisione == nil || f.Revisione.Stato != motorea.StatoRevisioneLetta {
		return nil
	}
	return f.Revisione
}

// confrontaRevisioni: ConfrontaRevisioni quando ci sono tutte e due; non determinabile se ne manca una.
func confrontaRevisioni(a, b *motorea.RevisioneLetta) motorea.Compatibilita {
	if a == nil || b == nil {
		return motorea.CompatibilitaNonDeterminabile
	}
	return motorea.ConfrontaRevisioni(*a, *b)
}

// confrontaQualificatori: fase e destinazione attribuite (P1 §7.4 r.300): discordanti solo se esplicite da tutte e due
// le parti e diverse; uguali se almeno una è esplicita da tutte e due e nessuna discorda; altrimenti non determinabile.
// La fase ignota non diventa serie né prototipo.
func confrontaQualificatori(a, b []motorea.AffissoLetto) motorea.Compatibilita {
	fa, da := qualificatoriEspliciti(a)
	fb, db := qualificatoriEspliciti(b)
	esito := motorea.CompatibilitaNonDeterminabile
	for _, x := range [][2]string{{fa, fb}, {da, db}} {
		if x[0] == "" || x[1] == "" {
			continue
		}
		if x[0] != x[1] {
			return motorea.CompatibilitaDiscordante
		}
		esito = motorea.CompatibilitaUguale
	}
	return esito
}

// qualificatoriEspliciti: la fase e la destinazione attribuite di una lettura ("" se non attribuite). Con più affissi
// che dicono valori diversi della stessa cosa, il valore è «?»: esplicito, ma non uguale a nessun altro.
func qualificatoriEspliciti(affissi []motorea.AffissoLetto) (fase, destinazione string) {
	for _, x := range affissi {
		if !x.Attribuito || x.Valore == nil {
			continue
		}
		if v := x.Valore.Fase; v != "" {
			if fase != "" && fase != v {
				v = "?"
			}
			fase = v
		}
		if v := x.Valore.Destinazione; v != "" {
			if destinazione != "" && destinazione != v {
				v = "?"
			}
			destinazione = v
		}
	}
	return fase, destinazione
}

// compatibilitaConDecisione: la compatibilità di una lettura del file con un codice deciso del nodo (confermato o
// manuale; T-E1-05): lo stesso namespace (non determinabile se no), la base (ConfrontaBasi) e il marcatore compatibile
// (la regola unica, T-B4-30: due marcatori scritti e diversi sono discordanti). Senza la lettura del codice deciso, o con
// una decisione tracciata su un codice che la grammatica non ha letto (la base vuota), non determinabile.
func compatibilitaConDecisione(f motorea.LetturaForma, base motorea.BaseLetta, lettura *motorea.LetturaForma, deciso *CodiceDeciso) motorea.Compatibilita {
	if lettura == nil || deciso == nil || len(deciso.Base.Segmenti) == 0 || lettura.Namespace != f.Namespace {
		return motorea.CompatibilitaNonDeterminabile
	}
	return conMarcatore(motorea.ConfrontaBasi(deciso.Base, base), marcatoreDi(*lettura), marcatoreDi(f))
}

// rangoAutorita: l'ordine delle autorità, dalla più debole (par.3.3.7): scenario, proposta, confermata.
func rangoAutorita(a Autorita) int {
	switch a {
	case AutoritaScenario:
		return 0
	case AutoritaProposta:
		return 1
	case AutoritaConfermata:
		return 2
	}
	return -1
}

// nodoDiStruttura: il nodo di una struttura con quel Rif.
func nodoDiStruttura(s StrutturaProdotto, rif string) (NodoProposto, bool) {
	for _, n := range s.Nodi {
		if n.Rif == rif {
			return n, true
		}
	}
	return NodoProposto{}, false
}

// unici: una copia ordinata dell'elenco, senza doppioni; nil se vuoto.
func unici(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	out := append([]string(nil), s...)
	sort.Strings(out)
	k := out[:1]
	for _, x := range out[1:] {
		if x != k[len(k)-1] {
			k = append(k, x)
		}
	}
	return k
}

// ---- SenzaFile ----

// ricalcolaSenzaFile: un nodo è senza file quando nessun file è candidato su di lui (3.3.7, P-20: l'assenza è ammessa
// e senza diagnostica). Conta ogni candidato, anche di un file ambiguo o discordante: un nodo con un file proposto non
// è un «figlio senza file» (lettura dell'orchestratore T-B4-11). La radice di una struttura che è la radice del suo
// STEP ha il suo file: non è senza file.
func ricalcolaSenzaFile(strutture []StrutturaProdotto, file []AncoraggioFile) {
	ancorati := map[string]bool{}
	for _, f := range file {
		for _, c := range f.Candidati {
			for _, p := range c.Posizioni {
				ancorati[chiavePosizioneCandidato(p)] = true
			}
		}
	}
	for si := range strutture {
		s := &strutture[si]
		for ni := range s.Nodi {
			n := &s.Nodi[ni]
			k := chiavePosizioneCandidato(PosizioneCandidato{Target: s.Target, AllegatoID: s.AllegatoID, Radice: s.Radice, Nodo: n.Rif})
			n.SenzaFile = !ancorati[k] && !(n.Rif == s.Radice && s.RadiceDelFile)
		}
	}
}

// ---- i controlli di contratto dei file ----

// fonteAllegato: il tipo della fonte di un allegato, come lo scrive l'adattatore (evidenze, Fonte.Tipo). Ripetuto qui
// perché la foglia non lo esporta, come fonteMessaggio.
const fonteAllegato = "allegato"

// disponibilitaValide: l'elenco chiuso della disponibilità.
var disponibilitaValide = map[Disponibilita]bool{DisponibilitaDisponibile: true, DisponibilitaMancante: true, DisponibilitaPendente: true,
	DisponibilitaParziale: true, DisponibilitaSenzaTesto: true, DisponibilitaIlleggibile: true, DisponibilitaErrore: true}

// controllaFile: i controlli di contratto dei file, tutti insieme, nell'ordine degli ingressi, con i codici della
// foglia come ProponiProdotti e ProponiStrutture.
func controllaFile(file []FileInterpretato, ctx ContestoStrutturale) []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	// d porta solo il codice, scritto da chi chiama con la sua costante (A1a-CAT, regola 5); qui il resto.
	errore := func(d evidenze.Diagnostica, percorso, messaggio string, rif ...string) {
		d.Gravita, d.Natura, d.Percorso, d.Messaggio, d.Rif = evidenze.GravitaErrore, evidenze.NaturaContratto, percorso, messaggio, rif
		out = append(out, d)
	}
	strutture := map[uuid.UUID]string{}
	for _, s := range ctx.Strutture {
		strutture[s.AllegatoID] = s.Sha256
	}
	visti := map[uuid.UUID]bool{}
	for i, f := range file {
		p := fmt.Sprintf("file[%d]", i)
		if f.AllegatoID == uuid.Nil {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".allegato_id", "un file senza allegato")
		} else if visti[f.AllegatoID] {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoIDRipetuto}, p+".allegato_id", fmt.Sprintf("l'allegato %s compare due volte fra i file: un ancoraggio per allegato", f.AllegatoID))
		}
		visti[f.AllegatoID] = true
		if fo := f.Documento.Fonte; fo.Tipo != fonteAllegato || fo.OrigineID != f.AllegatoID {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".documento.fonte", fmt.Sprintf("il documento è della fonte %q, non dell'allegato %s", fo.ID, f.AllegatoID))
		}
		if f.Interpretazione.BundleID != f.Documento.BundleID {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".interpretazione.bundle_id",
				fmt.Sprintf("l'interpretazione è del bundle %q, il documento è il bundle %q", f.Interpretazione.BundleID, f.Documento.BundleID))
		}
		unita := make(map[string]bool, len(f.Documento.Unita))
		for _, u := range f.Documento.Unita {
			unita[u.ID] = true
		}
		for _, l := range f.Interpretazione.Letture {
			for _, uid := range append([]string{l.UnitaID}, l.AltreUnita...) {
				if !unita[uid] {
					errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".interpretazione.letture["+l.ID+"]",
						fmt.Sprintf("la lettura sta sull'unità %q, che il documento non ha", uid), l.ID, uid)
				}
			}
		}
		if !disponibilitaValide[f.Disponibilita] {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoEnumIgnoto}, p+".disponibilita",
				fmt.Sprintf("disponibilità %q: la dà valutazione, con uno dei valori del contratto (par.3.3.7, T-B0-31)", f.Disponibilita))
		}
		sha := f.Documento.Fonte.RiferimentoFatti.Sha256
		if s, ok := strutture[f.AllegatoID]; ok && sha != "" && s != sha {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".documento.fonte.riferimento_fatti.sha256",
				"la struttura del contesto dello stesso allegato è di un altro contenuto", idFonteAllegato(f.AllegatoID))
		}
	}

	// Le associazioni decise (fase 3): manuale o confermato, di un allegato fra i file e di un componente; la confermata
	// con il suo documento (contratto §1.5); nessuna ripetuta.
	associazioni := map[string]bool{}
	for i, a := range ctx.AssociazioniDecise {
		p := fmt.Sprintf("%s.associazioni_decise[%d]", percorsoContesto, i)
		if a.Origine != OrigineManuale && a.Origine != OrigineConfermato {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoEnumIgnoto}, p+".origine",
				fmt.Sprintf("origine %q: un'associazione decisa è manuale o confermata; un candidato è una proposta, che si calcola (contratto §1.5)", a.Origine))
		}
		switch {
		case a.AllegatoID == uuid.Nil:
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".allegato_id", "un'associazione decisa senza allegato")
		case !visti[a.AllegatoID]:
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".allegato_id",
				fmt.Sprintf("l'allegato %s di un'associazione decisa non è fra i file: il suo cartiglio non si legge", a.AllegatoID), idFonteAllegato(a.AllegatoID))
		}
		if a.ComponenteID == uuid.Nil {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".componente_id", "un'associazione decisa senza componente")
		}
		if a.Origine == OrigineConfermato && a.DocumentoID == nil {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoRiferimentoPendente}, p+".documento_id",
				"un'associazione confermata senza il documento: la conferma è documento.componente_id (contratto §1.5)", idFonteAllegato(a.AllegatoID))
		}
		if k := chiaveAssociazioneDecisa(a); associazioni[k] {
			errore(evidenze.Diagnostica{Codice: evidenze.CodiceDocumentoIDRipetuto}, p, "la stessa associazione decisa due volte", idFonteAllegato(a.AllegatoID), RifComponente(a.ComponenteID))
		} else {
			associazioni[k] = true
		}
	}
	return out
}

// ---- la chiusura dell'esito ----

// ingressoAncoraggi: ciò che copre HashIngresso (6.4.5 regola 8).
type ingressoAncoraggi struct {
	VersioneServizio string         `json:"versione_servizio"`
	File             []ingressoFile `json:"file,omitempty"`
}

type ingressoFile struct {
	AllegatoID      uuid.UUID     `json:"allegato_id"`
	BundleID        string        `json:"bundle_id"`
	Interpretazione string        `json:"interpretazione"`
	Disponibilita   Disponibilita `json:"disponibilita"`
	Disegno         bool          `json:"disegno,omitempty"`
}

// ingressoTarget: ciò che copre HashTarget (parte 1 §9.4): i target e il contesto, in ordine canonico.
type ingressoTarget struct {
	VersioneServizio string              `json:"versione_servizio"`
	Target           []ProdottoRichiesto `json:"target,omitempty"`
	Contesto         ContestoStrutturale `json:"contesto"`
}

// chiudiAncoraggi calcola HashIngresso, HashTarget e Impronta (par.3.4.2, 3.4.3). File e strutture sono già in ordine.
func chiudiAncoraggi(e EsitoAncoraggi, files []FileInterpretato, target []ProdottoRichiesto, ctx ContestoStrutturale) (EsitoAncoraggi, error) {
	if len(e.File) == 0 {
		e.File = nil
	}
	if len(e.Strutture) == 0 {
		e.Strutture = nil
	}
	if len(e.Diagnostiche) == 0 {
		e.Diagnostiche = nil
	}
	in := ingressoAncoraggi{VersioneServizio: VersioneServizio}
	for _, f := range files {
		in.File = append(in.File, ingressoFile{AllegatoID: f.AllegatoID, BundleID: f.Documento.BundleID, Interpretazione: f.Interpretazione.ID, Disponibilita: f.Disponibilita,
			Disegno: f.Disegno})
	}
	h, err := jsoncanonico.ImprontaDi(in)
	if err != nil {
		return EsitoAncoraggi{}, fmt.Errorf("ancoraggio: impronta degli ingressi degli ancoraggi: %w", err)
	}
	e.HashIngresso = h
	tg := append([]ProdottoRichiesto(nil), target...)
	sort.SliceStable(tg, func(i, j int) bool { return tg[i].Rif < tg[j].Rif })
	if len(tg) == 0 {
		tg = nil
	}
	h, err = jsoncanonico.ImprontaDi(ingressoTarget{VersioneServizio: VersioneServizio, Target: tg, Contesto: contestoCanonico(ctx)})
	if err != nil {
		return EsitoAncoraggi{}, fmt.Errorf("ancoraggio: impronta dei target e del contesto: %w", err)
	}
	e.HashTarget = h
	e.Impronta = ""
	imp, err := jsoncanonico.ImprontaDi(e)
	if err != nil {
		return EsitoAncoraggi{}, fmt.Errorf("ancoraggio: impronta dell'esito degli ancoraggi: %w", err)
	}
	e.Impronta = imp
	return e, nil
}

// contestoCanonico: una copia del contesto con ogni elenco in ordine canonico e i tempi in UTC al millisecondo
// (par.3.4.3), per l'impronta. Non cambia il contesto di chi chiama.
func contestoCanonico(ctx ContestoStrutturale) ContestoStrutturale {
	c := ContestoStrutturale{CodiciProposti: ctx.CodiciProposti, LettureDecise: ctx.LettureDecise}
	for _, s := range ctx.Strutture {
		x := s
		x.Radici = unici(s.Radici)
		x.Nodi = nil
		for _, n := range s.Nodi {
			y := n
			y.Letture = append([]motorea.LetturaCodice(nil), n.Letture...)
			sort.SliceStable(y.Letture, func(i, j int) bool { return y.Letture[i].ID < y.Letture[j].ID })
			y.Grezzi = append([]ValoreGrezzo(nil), n.Grezzi...)
			sort.SliceStable(y.Grezzi, func(i, j int) bool {
				if (y.Grezzi[i].Campo == campoID) != (y.Grezzi[j].Campo == campoID) {
					return y.Grezzi[i].Campo == campoID
				}
				return y.Grezzi[i].UnitaID < y.Grezzi[j].UnitaID
			})
			y.Formazioni = unici(n.Formazioni)
			x.Nodi = append(x.Nodi, y)
		}
		sort.SliceStable(x.Nodi, func(i, j int) bool { return x.Nodi[i].Rif < x.Nodi[j].Rif })
		x.Archi = archiInOrdine(s.Archi)
		x.NomeFile = append([]LetturaConPosizione(nil), s.NomeFile...)
		sort.SliceStable(x.NomeFile, func(i, j int) bool { return x.NomeFile[i].Lettura.ID < x.NomeFile[j].Lettura.ID })
		c.Strutture = append(c.Strutture, x)
	}
	sort.SliceStable(c.Strutture, func(i, j int) bool {
		if c.Strutture[i].Sha256 != c.Strutture[j].Sha256 {
			return c.Strutture[i].Sha256 < c.Strutture[j].Sha256
		}
		return c.Strutture[i].AllegatoID.String() < c.Strutture[j].AllegatoID.String()
	})
	c.Confermato = append([]ComponenteDeciso(nil), ctx.Confermato...)
	sort.SliceStable(c.Confermato, func(i, j int) bool {
		return c.Confermato[i].ComponenteID.String() < c.Confermato[j].ComponenteID.String()
	})
	c.ArchiConfermati = archiInOrdine(ctx.ArchiConfermati)
	c.Proposto = append([]RigaPropostaLegacy(nil), ctx.Proposto...)
	sort.SliceStable(c.Proposto, func(i, j int) bool { return c.Proposto[i].ID.String() < c.Proposto[j].ID.String() })
	c.ArchiProposti = archiInOrdine(ctx.ArchiProposti)
	c.Decise = append([]RigaDecisaLegacy(nil), ctx.Decise...)
	sort.SliceStable(c.Decise, func(i, j int) bool { return c.Decise[i].ID.String() < c.Decise[j].ID.String() })
	c.CodiciMessaggi = append([]CodiceDiMessaggio(nil), ctx.CodiciMessaggi...)
	sort.SliceStable(c.CodiciMessaggi, func(i, j int) bool {
		if c.CodiciMessaggi[i].MessaggioID != c.CodiciMessaggi[j].MessaggioID {
			return c.CodiciMessaggi[i].MessaggioID.String() < c.CodiciMessaggi[j].MessaggioID.String()
		}
		return c.CodiciMessaggi[i].Lettura.ID < c.CodiciMessaggi[j].Lettura.ID
	})
	for _, d := range ctx.DecisioniIdentita {
		x := d
		x.Il = d.Il.UTC().Truncate(time.Millisecond)
		x.EvidenzeViste = append([]EvidenzaVista(nil), d.EvidenzeViste...)
		sort.SliceStable(x.EvidenzeViste, func(i, j int) bool {
			if x.EvidenzeViste[i].Fonte != x.EvidenzeViste[j].Fonte {
				return x.EvidenzeViste[i].Fonte < x.EvidenzeViste[j].Fonte
			}
			return x.EvidenzeViste[i].Valore < x.EvidenzeViste[j].Valore
		})
		if len(x.EvidenzeViste) == 0 {
			x.EvidenzeViste = nil
		}
		c.DecisioniIdentita = append(c.DecisioniIdentita, x)
	}
	sort.SliceStable(c.DecisioniIdentita, func(i, j int) bool {
		a, b := c.DecisioniIdentita[i], c.DecisioniIdentita[j]
		if a.Oggetto != b.Oggetto {
			return a.Oggetto < b.Oggetto
		}
		return a.ID.String() < b.ID.String()
	})
	if len(c.Confermato) == 0 {
		c.Confermato = nil
	}
	if len(c.ArchiConfermati) == 0 {
		c.ArchiConfermati = nil
	}
	if len(c.Proposto) == 0 {
		c.Proposto = nil
	}
	if len(c.ArchiProposti) == 0 {
		c.ArchiProposti = nil
	}
	if len(c.Decise) == 0 {
		c.Decise = nil
	}
	if len(c.CodiciMessaggi) == 0 {
		c.CodiciMessaggi = nil
	}
	c.AssociazioniDecise = append([]AssociazioneDecisa(nil), ctx.AssociazioniDecise...)
	sort.SliceStable(c.AssociazioniDecise, func(i, j int) bool {
		return chiaveAssociazioneDecisa(c.AssociazioniDecise[i]) < chiaveAssociazioneDecisa(c.AssociazioniDecise[j])
	})
	if len(c.AssociazioniDecise) == 0 {
		c.AssociazioniDecise = nil
	}
	return c
}
