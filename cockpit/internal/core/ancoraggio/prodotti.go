// Package ancoraggio contiene i servizi di proposta del motore A: quali prodotti chiede la mail
// (ProponiProdotti) e, dai commit che seguono, a quali target si ancorano i file (piano A, par.3.3.7; parte 1
// §7.4, con le correzioni del v3 §2 e §10.4). La classificazione dice che cosa significa un'evidenza; qui si
// decide a quali prodotti candidarla, conservando ambiguità, alternative, esclusioni e motivi.
//
// È puro: nessuna scrittura, nessun DB, file, orologio o rete; non importa la fotografia (T-B0-04). Un candidato
// non è mai una decisione: un prodotto letto dalla mail resta «da confermare» (R60 A), e il target lo decidono il
// gesto 2 dell'operatore o lo scenario, che legge valutazione (R70 A, R75 A).
package ancoraggio

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// VersioneServizio: le regole di proposta (par.3.4.1). È la versione_servizio della parte 1 §9.4 ed entra nell'ID
// degli esiti (HashIngresso). Cambia con un commit che lo dichiara, e la prova che la fissa si riscrive.
const VersioneServizio = "ancoraggio-1"

// ---- il vocabolario comune (contratto §2.2) ----

// Autorita: l'autorità di un target o di un candidato di ancoraggio (par.3.3.7; R29 f). Per i target valgono solo
// confermata e scenario (R60 A): un prodotto letto dalla mail non è mai un target. Lo scenario è l'ingresso del
// banco e dell'anteprima che simula il caso (il file dei casi): non nasce una conferma (R75 A, v3 §4.4).
type Autorita string

const (
	AutoritaConfermata Autorita = "confermata"
	AutoritaProposta   Autorita = "proposta"
	AutoritaScenario   Autorita = "scenario"
)

// OrigineDato: il vocabolario unico delle origini di un dato (workflow, §1; T-B0-09). È un campo distinto
// dall'autorità:
//   - grezzo: il valore com'è nella fonte, mai cambiato;
//   - proposto: inferito dal sistema (motore A, F8, proposte di struttura);
//   - manuale: un'indicazione o una correzione dell'operatore non confermata (T-B0-33);
//   - confermato: un gesto di conferma salvato;
//   - scenario: l'ingresso del caso, che simula una scelta.
type OrigineDato string

const (
	OrigineGrezzo     OrigineDato = "grezzo"
	OrigineProposto   OrigineDato = "proposto"
	OrigineManuale    OrigineDato = "manuale"
	OrigineConfermato OrigineDato = "confermato"
	OrigineScenario   OrigineDato = "scenario"
)

// StatoRichiesta: lo stato della richiesta, per messaggio e per thread (6.0.5; R29 c). La decisione operativa viene
// dal gesto 1 salvato dell'operatore; il triage deterministico è un'evidenza e non diventa mai una conferma.
//   - valutata: il messaggio è entrato nella RFQ con il gesto dell'operatore (messaggio.aggancio = «operatore», con
//     agganciato_da e agganciato_il), è in entrata e la controparte è il cliente del thread;
//   - non_valutabile: un messaggio in uscita, o con la controparte fornitore o un altro cliente;
//   - da_valutare: tutto il resto, anche con un triage «nuova_rfq» o «aggancia»: un aggancio automatico di righe
//     vecchie o nessun gesto; la controparte interna, altra, sconosciuta o ambigua.
type StatoRichiesta string

const (
	StatoRichiestaValutata      StatoRichiesta = "valutata"
	StatoRichiestaDaValutare    StatoRichiesta = "da_valutare"
	StatoRichiestaNonValutabile StatoRichiesta = "non_valutabile"
)

// GestoMessaggio: il gesto 1 di un messaggio (T-B0-06; contratto §1.1), come lo legge valutazione dal record
// AggancioMessaggio della fotografia (T-B0-01). Aggancio è il valore di messaggio.aggancio; AgganciatoDa nil vuol
// dire un aggancio senza persona. Evento è il triage deterministico del messaggio («<esito>/<atto>»), solo come
// evidenza; Motivo dice perché lo stato è quello (per esempio la direzione o la controparte).
type GestoMessaggio struct {
	MessaggioID  uuid.UUID      `json:"messaggio_id"`
	Stato        StatoRichiesta `json:"stato"`
	Aggancio     string         `json:"aggancio,omitempty"`
	AgganciatoDa *uuid.UUID     `json:"agganciato_da,omitempty"`
	AgganciatoIl *time.Time     `json:"agganciato_il,omitempty"`
	Evento       string         `json:"evento,omitempty"`
	Motivo       string         `json:"motivo,omitempty"`
}

// RichiestaValutata: la richiesta del cliente come la attestano i gesti salvati (parte 1 §7.4; 6.0.5), una per
// thread con i gesti per messaggio (T-B0-06). Non è un booleano dato al parser. La compone valutazione dalla
// fotografia (gesti, direzione, controparte, triage deterministico come evidenza) e dagli ingressi del caso
// (segmenti, scenario). Nessun esito LLM.
//   - Messaggi: i messaggi della richiesta; ProponiProdotti considera solo le loro interpretazioni (regola 1).
//   - Segmenti: i segmenti dichiarati pertinenti, con l'origine (operatore | riconoscimento | scenario).
//   - Evento: il triage deterministico, solo evidenza; "" = nessun triage.
//   - Stato: lo stato del thread; per un messaggio con il suo gesto in Gesti vale lo stato del gesto.
//   - Decisioni: l'impronta dei gesti usati, sha256 del canonico di (messaggio_id, agganciato_da, agganciato_il) in
//     ordine di messaggio (T-16): la calcola chi compone la richiesta.
type RichiestaValutata struct {
	ClienteID uuid.UUID            `json:"cliente_id"`
	Messaggi  []uuid.UUID          `json:"messaggi,omitempty"`
	Segmenti  []SegmentoPertinente `json:"segmenti,omitempty"`
	Evento    string               `json:"evento,omitempty"`
	Stato     StatoRichiesta       `json:"stato"`
	Gesti     []GestoMessaggio     `json:"gesti,omitempty"`
	Decisioni string               `json:"decisioni,omitempty"`
}

// SegmentoPertinente: un segmento di un messaggio dichiarato pertinente, con l'origine della scelta (operatore |
// riconoscimento | scenario) e il motivo. È la stessa scelta che arriva a Interpreta come UsoSegmenti: qui resta
// visibile nella richiesta.
type SegmentoPertinente struct {
	MessaggioID uuid.UUID `json:"messaggio_id"`
	SegmentoID  string    `json:"segmento_id"`
	Origine     string    `json:"origine"`
	Motivo      string    `json:"motivo,omitempty"`
}

// MessaggioInterpretato: un messaggio con il suo documento e la sua interpretazione, come FileInterpretato per un
// allegato (par.3.3.7). Il documento serve a ProponiProdotti per le provenance che l'interpretazione non porta: il
// segmento di un'unità e l'entità «riga» di una cella (legami-1), e il messaggio stesso (Fonte.OrigineID). Gli ID
// delle letture, delle unità e delle entità sono locali al documento (par.3.4.2: l'identità completa è (bundle, ID
// locale)): per questo ogni candidato e ogni esclusione porta il suo MessaggioID.
type MessaggioInterpretato struct {
	MessaggioID     uuid.UUID
	Documento       evidenze.DocumentoEvidenze
	Interpretazione motorea.Interpretazione
}

// EsitoProdotti: i prodotti candidati della mail, le letture escluse con il motivo, le diagnostiche.
//   - HashIngresso: la versione del servizio, le interpretazioni (messaggio, bundle, ID) e la richiesta usata.
//     EsitoProdotti non ha un HashTarget: per i prodotti la richiesta è il «target» della parte 1 §9.4, e sta qui.
//   - Impronta: sha256 del canonico dell'esito con Impronta vuota.
type EsitoProdotti struct {
	VersioneServizio string                 `json:"versione_servizio"`
	HashIngresso     string                 `json:"hash_ingresso"`
	Candidati        []CandidatoProdotto    `json:"candidati,omitempty"`
	Esclusioni       []Esclusione           `json:"esclusioni,omitempty"`
	Diagnostiche     []evidenze.Diagnostica `json:"diagnostiche,omitempty"`
	Impronta         string                 `json:"impronta"`
}

// CandidatoProdotto: un prodotto letto dalla mail (6.4.4). È sempre «da confermare»: non è un target, nemmeno con
// la richiesta valutata e il segmento scelto dall'operatore (R60 A); Origine vale sempre «proposto».
//   - CodiceRichiesto è l'originale della richiesta (affissi e base, senza etichetta né revisione: «P7120100»),
//     Base la lettura tecnica («7120100»): due campi distinti (parte 1 §7.4). Namespace: lo spazio di codici della
//     famiglia, parte della chiave.
//   - Qualificatori: fase e destinazione, solo se attribuite sul selettore (D2, D3).
//   - Lettura, MessaggioID, Segmento, Uso, OrigineUso: la lettura principale (regola 3: la cella della riga, o la
//     lettura in testo libero con la scelta più forte) e da dove viene, con l'uso del segmento e l'origine della scelta
//     (operatore | riconoscimento | scenario | "" senza selezione), conservata com'è (v3 §10.2).
//   - Riga: l'EntitaID della riga di tabella, nel messaggio della lettura principale; "" = testo libero.
//   - Quantita viene dalla riga: l'attributo «quantita» della stessa entità riga, cioè la cella sotto
//     l'intestazione che la grammatica dichiara (R28 a; E-16); nil = non provata. EvidenzaQuantita: l'attributo, la
//     cella e la cella d'intestazione, del messaggio della riga.
//   - Evidenze: tutte le letture che lo sostengono, ognuna con il suo messaggio, segmento, uso e origine (la deduplica
//     cita l'evidenza: principio della provenance, 5.0); testo e cella della stessa occorrenza sono già una lettura
//     sola (R28 b). Alternative: le letture di altre famiglie o forme sulla stessa occorrenza (regola 6).
//   - Motivo: da_confermare, oppure richiesta_non_valutata quando nessuna evidenza viene da un messaggio con il
//     gesto 1 (regola 1).
type CandidatoProdotto struct {
	MessaggioID      uuid.UUID              `json:"messaggio_id"`
	Segmento         string                 `json:"segmento"`
	Uso              string                 `json:"uso"`
	OrigineUso       string                 `json:"origine_uso,omitempty"`
	Origine          OrigineDato            `json:"origine"`
	Namespace        string                 `json:"namespace"`
	CodiceRichiesto  string                 `json:"codice_richiesto"`
	Lettura          string                 `json:"lettura"`
	Base             motorea.BaseLetta      `json:"base"`
	Qualificatori    []motorea.AffissoLetto `json:"qualificatori,omitempty"`
	Quantita         *int                   `json:"quantita,omitempty"`
	EvidenzaQuantita []string               `json:"evidenza_quantita,omitempty"`
	Riga             string                 `json:"riga,omitempty"`
	Evidenze         []EvidenzaProdotto     `json:"evidenze,omitempty"`
	Alternative      []EvidenzaProdotto     `json:"alternative,omitempty"`
	Motivo           string                 `json:"motivo"`
}

// EvidenzaProdotto: una lettura di un messaggio della richiesta, con il segmento da cui viene, l'uso del segmento e
// l'origine della scelta. Il messaggio c'è perché gli ID delle letture sono locali al documento (par.3.4.2).
type EvidenzaProdotto struct {
	MessaggioID uuid.UUID `json:"messaggio_id"`
	Lettura     string    `json:"lettura"`
	Segmento    string    `json:"segmento"`
	Uso         string    `json:"uso"`
	OrigineUso  string    `json:"origine_uso,omitempty"`
}

// I motivi di un candidato. Tutti e due dicono «da confermare»: il secondo dice anche che nessuna evidenza viene da
// un messaggio con il gesto 1 (6.4.4, regola 1; R29 c). Nessuno dei due fa del candidato un target (R60 A).
const (
	MotivoCandidatoDaConfermare         = "da_confermare"
	MotivoCandidatoRichiestaNonValutata = "richiesta_non_valutata"
)

// Esclusione: una lettura di un messaggio della richiesta che non diventa un prodotto, con il motivo (6.4.4).
type Esclusione struct {
	MessaggioID uuid.UUID `json:"messaggio_id"`
	Lettura     string    `json:"lettura"`
	Motivo      string    `json:"motivo"`
}

// I motivi di un'esclusione.
//   - menzione: una lettura fuori dai segmenti pertinenti (la storia non selezionata, anche tutta la storia non
//     separabile di un inoltro senza confine finché nessuno la seleziona, R48 A; un segmento escluso);
//   - funzione_non_richiesta: un'altra funzione del router che non è una richiesta (in una mail non succede);
//   - famiglia_senza_ruolo_prodotto: una richiesta di una famiglia senza il ruolo «prodotto» (R17 b);
//   - attributo_di_riga: la lettura sta nella cella che dà l'attributo «quantita» della riga: una quantità non è mai
//     un prodotto (parte 1 §7.4).
//
// Il riferimento della RFQ non ha un motivo suo: in A1 non nasce nessuna lettura di riferimento (riferimenti_rfq
// riservato, R20 c), quindi non diventa mai un prodotto (C-31).
const (
	MotivoEsclusioneMenzione             = "menzione"
	MotivoEsclusioneFunzioneNonRichiesta = "funzione_non_richiesta"
	MotivoEsclusioneSenzaRuoloProdotto   = "famiglia_senza_ruolo_prodotto"
	MotivoEsclusioneAttributoDiRiga      = "attributo_di_riga"
)

// fonteMessaggio: il tipo della fonte di un messaggio, come lo scrive l'adattatore (evidenze, Fonte.Tipo). Il
// valore è ripetuto qui perché la foglia non lo esporta; se cambia, lo dicono le prove del pacchetto.
const fonteMessaggio = "messaggio"

// ---- ProponiProdotti ----

// ProponiProdotti: i prodotti candidati della mail, con alternative, esclusioni e motivi (6.4.4; parte 1 §7.4).
// Prende più messaggi, perché una richiesta può averne più d'uno. È puro e deterministico: l'ordine degli ingressi
// non conta.
//
// Le regole (6.4.4, con R60 A del 05/10):
//  1. Si considerano solo i messaggi di r.Messaggi. Lo stato di un messaggio è quello del suo gesto, o r.Stato se
//     non ha un gesto. Un candidato senza nessuna evidenza da un messaggio valutato ha il motivo
//     richiesta_non_valutata, con ancoraggio.richiesta_non_valutata. Con il gesto o senza, nessun candidato è un
//     target (R60 A).
//  2. Entrano, una per una, le letture con funzione «richiesta» e «prodotto» fra i ruoli candidati. La funzione la dà
//     il router di motorea dall'uso del segmento: la storia non scelta dall'operatore o dallo scenario resta
//     menzione, e va fra le esclusioni (R48 A; E2). L'ammissione è per evidenza.
//  3. Chiave del candidato: (namespace, base normalizzata, qualificatori attribuiti, riga di tabella); la riga è
//     (messaggio, entità riga), perché gli ID sono locali al documento. La stessa base in due righe dà due
//     candidati, ognuno con la sua quantità, con ancoraggio.righe_stessa_base. Le letture in testo libero con la
//     chiave di una sola riga diventano evidenze di quella riga, anche da segmenti e messaggi diversi della richiesta;
//     senza righe (o con più righe) le letture in testo libero con la stessa chiave fanno un candidato solo. La
//     lettura principale: per una riga la sua cella (l'ID minore, se più celle); per il testo libero quella con la
//     scelta più forte (pertinente con origine operatore o scenario, poi pertinente da riconoscimento, poi da
//     valutare, poi sconosciuto), a parità il messaggio e poi la lettura con l'ID minore.
//  4. Quantità: l'attributo «quantita» della stessa entità riga, con un intero; altrimenti nil e
//     ancoraggio.quantita_non_intera.
//  5. Attributi e relazioni non producono mai un candidato; la cella della quantità è un'esclusione.
//  6. Letture di più famiglie o forme sulla stessa occorrenza: restano tutte, ognuna nella sua chiave, con le altre
//     in Alternative e ancoraggio.alternative_conservate; nessuna scelta per ordine.
//  7. Ordine dei candidati: il testo libero prima delle righe, le righe per (messaggio, tabella, riga), poi base,
//     codice richiesto, namespace e lettura principale; HashIngresso e Impronta (par.3.4.2).
//
// L'errore è di contratto (*evidenze.ErroreContratto): un messaggio ripetuto, un documento che non è quello del
// messaggio, un'interpretazione di un altro documento, una lettura o un attributo su un'unità che il documento non
// ha, un gesto ripetuto.
func ProponiProdotti(messaggi []MessaggioInterpretato, r RichiestaValutata) (EsitoProdotti, error) {
	if d := controllaIngressi(messaggi, r); len(d) > 0 {
		return EsitoProdotti{}, &evidenze.ErroreContratto{Diagnostiche: d}
	}

	richiesti := make(map[uuid.UUID]bool, len(r.Messaggi))
	for _, id := range r.Messaggi {
		richiesti[id] = true
	}
	var considerati []MessaggioInterpretato
	for _, m := range messaggi {
		if richiesti[m.MessaggioID] {
			considerati = append(considerati, m)
		}
	}
	sort.SliceStable(considerati, func(i, j int) bool {
		return considerati[i].MessaggioID.String() < considerati[j].MessaggioID.String()
	})

	esito := EsitoProdotti{VersioneServizio: VersioneServizio}
	var ammesse []letturaAmmessa
	quantita := map[string][]motorea.AttributoLetto{} // per riga (messaggio, entità)
	alternative := map[string][]EvidenzaProdotto{}    // per lettura (messaggio, ID)
	valutati := map[uuid.UUID]bool{}
	for _, m := range considerati {
		p := nuovaProposta(m, statoDelMessaggio(r, m.MessaggioID))
		valutati[m.MessaggioID] = p.stato == StatoRichiestaValutata
		a, e, d := p.ammissione(quantita, alternative)
		ammesse = append(ammesse, a...)
		esito.Esclusioni = append(esito.Esclusioni, e...)
		esito.Diagnostiche = append(esito.Diagnostiche, d...)
	}
	gruppi, d := raggruppa(ammesse)
	esito.Diagnostiche = append(esito.Diagnostiche, d...)
	var candidati []candidatoOrdinabile
	for _, g := range gruppi {
		c, d := candidato(g, quantita, alternative, valutati)
		candidati = append(candidati, c)
		esito.Diagnostiche = append(esito.Diagnostiche, d...)
	}
	esito.Diagnostiche = append(esito.Diagnostiche, diagnosiNonValutate(considerati, candidati, valutati)...)
	ordinaCandidati(candidati)
	for _, c := range candidati {
		esito.Candidati = append(esito.Candidati, c.c)
	}
	return chiudiEsito(esito, considerati, r)
}

// controllaIngressi: i controlli di contratto, tutti insieme, nell'ordine degli ingressi.
func controllaIngressi(messaggi []MessaggioInterpretato, r RichiestaValutata) []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	visti := map[uuid.UUID]bool{}
	for i, m := range messaggi {
		percorso := fmt.Sprintf("messaggi[%d]", i)
		if visti[m.MessaggioID] {
			out = append(out, evidenze.Diagnostica{
				Codice:    evidenze.CodiceDocumentoIDRipetuto,
				Gravita:   evidenze.GravitaErrore,
				Natura:    evidenze.NaturaContratto,
				Percorso:  percorso + ".messaggio_id",
				Messaggio: fmt.Sprintf("il messaggio %s compare due volte fra gli ingressi", m.MessaggioID),
			})
		}
		visti[m.MessaggioID] = true
		f := m.Documento.Fonte
		if f.Tipo != fonteMessaggio || f.OrigineID != m.MessaggioID {
			out = append(out, evidenze.Diagnostica{
				Codice:    evidenze.CodiceDocumentoRiferimentoPendente,
				Gravita:   evidenze.GravitaErrore,
				Natura:    evidenze.NaturaContratto,
				Percorso:  percorso + ".documento.fonte",
				Messaggio: fmt.Sprintf("il documento è della fonte %q, non del messaggio %s", f.ID, m.MessaggioID),
			})
		}
		if m.Interpretazione.BundleID != m.Documento.BundleID {
			out = append(out, evidenze.Diagnostica{
				Codice:    evidenze.CodiceDocumentoRiferimentoPendente,
				Gravita:   evidenze.GravitaErrore,
				Natura:    evidenze.NaturaContratto,
				Percorso:  percorso + ".interpretazione.bundle_id",
				Messaggio: fmt.Sprintf("l'interpretazione è del bundle %q, il documento è il bundle %q", m.Interpretazione.BundleID, m.Documento.BundleID),
			})
		}
		unita := make(map[string]bool, len(m.Documento.Unita))
		for _, u := range m.Documento.Unita {
			unita[u.ID] = true
		}
		for _, l := range m.Interpretazione.Letture {
			for _, uid := range append([]string{l.UnitaID}, l.AltreUnita...) {
				if !unita[uid] {
					out = append(out, evidenze.Diagnostica{
						Codice:    evidenze.CodiceDocumentoRiferimentoPendente,
						Gravita:   evidenze.GravitaErrore,
						Natura:    evidenze.NaturaContratto,
						Percorso:  percorso + ".interpretazione.letture[" + l.ID + "]",
						Messaggio: fmt.Sprintf("la lettura sta sull'unità %q, che il documento non ha", uid),
						Rif:       []string{l.ID, uid},
					})
				}
			}
		}
		for _, a := range m.Interpretazione.Attributi {
			if !unita[a.UnitaID] {
				out = append(out, evidenze.Diagnostica{
					Codice:    evidenze.CodiceDocumentoRiferimentoPendente,
					Gravita:   evidenze.GravitaErrore,
					Natura:    evidenze.NaturaContratto,
					Percorso:  percorso + ".interpretazione.attributi[" + a.ID + "]",
					Messaggio: fmt.Sprintf("l'attributo sta sull'unità %q, che il documento non ha", a.UnitaID),
					Rif:       []string{a.ID, a.UnitaID},
				})
			}
		}
	}
	gesti := map[uuid.UUID]bool{}
	for i, g := range r.Gesti {
		if gesti[g.MessaggioID] {
			out = append(out, evidenze.Diagnostica{
				Codice:    evidenze.CodiceDocumentoIDRipetuto,
				Gravita:   evidenze.GravitaErrore,
				Natura:    evidenze.NaturaContratto,
				Percorso:  fmt.Sprintf("richiesta.gesti[%d].messaggio_id", i),
				Messaggio: fmt.Sprintf("il messaggio %s ha due gesti: il gesto 1 è uno per messaggio (T-B0-06)", g.MessaggioID),
			})
		}
		gesti[g.MessaggioID] = true
	}
	return out
}

// statoDelMessaggio: lo stato del gesto del messaggio, se la richiesta ne ha uno (T-B0-06); altrimenti quello del
// thread. Un valore fuori elenco vale come «non valutata»: in dubbio nessuna promozione.
func statoDelMessaggio(r RichiestaValutata, id uuid.UUID) StatoRichiesta {
	for _, g := range r.Gesti {
		if g.MessaggioID == id {
			return g.Stato
		}
	}
	return r.Stato
}

// ---- l'ammissione delle letture di un messaggio ----

// letturaAmmessa: una lettura che può diventare un candidato, con le provenance che servono alla chiave.
type letturaAmmessa struct {
	l         motorea.LetturaCodice
	ev        EvidenzaProdotto
	riga      string // (messaggio, entità riga), per una cella di tabella; "" = testo libero
	entita    string // l'EntitaID della riga
	tabella   int    // PosTabella.Tabella e PosTabella.Riga della cella, per l'ordine
	nRiga     int
	chiave    string // namespace, base normalizzata, qualificatori attribuiti
	cellaRiga bool   // la lettura sta in una cella della riga del suo gruppo
}

// proposta: lo stato dell'ammissione per un messaggio. Le mappe servono solo a cercare per ID, mai a scorrere.
type proposta struct {
	m      MessaggioInterpretato
	stato  StatoRichiesta
	unita  map[string]evidenze.UnitaEvidenza
	entita map[string]evidenze.EntitaLocale
}

func nuovaProposta(m MessaggioInterpretato, stato StatoRichiesta) *proposta {
	p := &proposta{
		m:      m,
		stato:  stato,
		unita:  make(map[string]evidenze.UnitaEvidenza, len(m.Documento.Unita)),
		entita: make(map[string]evidenze.EntitaLocale, len(m.Documento.Entita)),
	}
	for _, u := range m.Documento.Unita {
		p.unita[u.ID] = u
	}
	for _, e := range m.Documento.Entita {
		p.entita[e.ID] = e
	}
	return p
}

// segmentoDi: il segmento dell'unità o, per una cella, quello della sua entità «riga» (legami-1), come lo legge
// Interpreta per l'uso.
func (p *proposta) segmentoDi(u evidenze.UnitaEvidenza) string {
	if u.SegmentoID != "" {
		return u.SegmentoID
	}
	if u.EntitaID != "" {
		return p.entita[u.EntitaID].SegmentoID
	}
	return ""
}

// chiaveRiga: l'identità di una riga di tabella nella richiesta, (messaggio, entità riga).
func chiaveRiga(messaggio uuid.UUID, entita string) string {
	return messaggio.String() + "\x00" + entita
}

// chiaveLettura: l'identità di una lettura nella richiesta, (messaggio, ID della lettura).
func chiaveLettura(messaggio uuid.UUID, lettura string) string {
	return messaggio.String() + "\x00" + lettura
}

// ammissione applica le regole 2, 5 e 6 alle letture del messaggio, in ordine di ID: le esclusioni con il motivo, le
// letture ammesse con la loro chiave, gli attributi «quantita» per riga e le alternative per lettura.
func (p *proposta) ammissione(quantita map[string][]motorea.AttributoLetto, alternative map[string][]EvidenzaProdotto) ([]letturaAmmessa, []Esclusione, []evidenze.Diagnostica) {
	id := p.m.MessaggioID
	interp := p.m.Interpretazione
	letture := append([]motorea.LetturaCodice(nil), interp.Letture...)
	sort.SliceStable(letture, func(i, j int) bool { return letture[i].ID < letture[j].ID })

	// Le celle che danno l'attributo «quantita» della loro riga (regola 5). Provenance: la stessa unità; regola: la
	// quantita_tabellare della grammatica, che motorea ha già applicato.
	celleQuantita := map[string]bool{}
	for _, a := range interp.Attributi {
		if a.Tipo != motorea.AttributoQuantita {
			continue
		}
		celleQuantita[a.UnitaID] = true
		k := chiaveRiga(id, a.EntitaID)
		quantita[k] = append(quantita[k], a)
	}

	var esclusioni []Esclusione
	escludi := func(l motorea.LetturaCodice, motivo string) {
		esclusioni = append(esclusioni, Esclusione{MessaggioID: id, Lettura: l.ID, Motivo: motivo})
	}
	var ammesse []letturaAmmessa
	for _, l := range letture {
		switch {
		case l.Funzione == motorea.FunzMenzione:
			escludi(l, MotivoEsclusioneMenzione)
		case l.Funzione != motorea.FunzRichiesta:
			escludi(l, MotivoEsclusioneFunzioneNonRichiesta)
		case !haRuolo(l.RuoliCandidati, grammatica.RuoloProdotto):
			escludi(l, MotivoEsclusioneSenzaRuoloProdotto)
		case suCellaQuantita(l, celleQuantita):
			escludi(l, MotivoEsclusioneAttributoDiRiga)
		default:
			ammesse = append(ammesse, p.ammessa(l))
		}
	}
	return ammesse, esclusioni, alternativeDelleLetture(ammesse, id, alternative)
}

// ammessa: la lettura con l'evidenza, la riga e la chiave. La riga è l'entità della cella solo per un'unità di
// tabella: è la provenance «stessa riga di tabella» del principio del 5.0.
func (p *proposta) ammessa(l motorea.LetturaCodice) letturaAmmessa {
	u := p.unita[l.UnitaID]
	a := letturaAmmessa{l: l, ev: EvidenzaProdotto{MessaggioID: p.m.MessaggioID, Lettura: l.ID, Segmento: p.segmentoDi(u), Uso: l.Uso, OrigineUso: l.OrigineUso}}
	if pt := u.Posizione.Tabella; pt != nil && u.EntitaID != "" {
		a.riga, a.entita, a.tabella, a.nRiga = chiaveRiga(p.m.MessaggioID, u.EntitaID), u.EntitaID, pt.Tabella, pt.Riga
	}
	a.chiave = strings.Join([]string{l.Forma.Namespace, l.Forma.Base.Normalizzata, chiaveQualificatori(l.Forma.Affissi)}, "\x00")
	return a
}

// alternativeDelleLetture applica la regola 6: due letture ammesse della stessa unità che si sovrappongono sono
// alternative. Provenance: la stessa unità e intervalli che si toccano, come motore.letture_alternative; nessun
// legame nasce, e nessuna lettura si sceglie. Scrive, per ogni lettura, le altre della stessa occorrenza.
func alternativeDelleLetture(ammesse []letturaAmmessa, messaggio uuid.UUID, out map[string][]EvidenzaProdotto) []evidenze.Diagnostica {
	var diag []evidenze.Diagnostica
	for i := range ammesse {
		for j := i + 1; j < len(ammesse); j++ {
			x, y := ammesse[i], ammesse[j]
			ox, oy := x.l.Occorrenza, y.l.Occorrenza
			if x.l.UnitaID != y.l.UnitaID || !(ox.Inizio < oy.Fine && oy.Inizio < ox.Fine) {
				continue
			}
			kx, ky := chiaveLettura(messaggio, x.l.ID), chiaveLettura(messaggio, y.l.ID)
			out[kx] = append(out[kx], y.ev)
			out[ky] = append(out[ky], x.ev)
			diag = append(diag, evidenze.Diagnostica{
				Codice:    CodiceAlternativeConservate,
				Gravita:   evidenze.GravitaAvviso,
				Natura:    evidenze.NaturaDati,
				Percorso:  "messaggi[" + messaggio.String() + "].letture",
				Messaggio: "due letture di prodotto sulla stessa occorrenza: restano tutte e due, ognuna con l'altra fra le alternative, nessuna si sceglie",
				Rif:       []string{x.l.ID, y.l.ID},
			})
		}
	}
	return diag
}

// ---- i candidati della richiesta ----

// gruppo: le letture di un candidato, con la riga (vuota per il testo libero) e la lettura principale.
type gruppo struct {
	riga       string
	letture    []letturaAmmessa
	principale int
}

// rangoScelta: la forza della scelta del segmento di una lettura, per la principale del testo libero: pertinente con
// origine operatore o scenario (una scelta esplicita), poi pertinente da riconoscimento (un candidato, E2), poi da
// valutare, poi tutto il resto (uso sconosciuto).
func rangoScelta(ev EvidenzaProdotto) int {
	switch {
	case ev.Uso == motorea.UsoPertinente && (ev.OrigineUso == "operatore" || ev.OrigineUso == "scenario"):
		return 3
	case ev.Uso == motorea.UsoPertinente:
		return 2
	case ev.Uso == motorea.UsoDaValutare:
		return 1
	}
	return 0
}

// precede: a viene prima di b, a parità di tutto il resto: il messaggio, poi l'ID della lettura.
func precede(a, b EvidenzaProdotto) bool {
	if a.MessaggioID != b.MessaggioID {
		return a.MessaggioID.String() < b.MessaggioID.String()
	}
	return a.Lettura < b.Lettura
}

// raggruppa applica la regola 3 su tutta la richiesta. Le righe di una chiave si contano prima; le letture in testo
// libero si uniscono alla riga solo se la chiave ha una riga sola (provenance: la stessa richiesta e la stessa
// identità della grammatica; regola: 6.4.4 n.3); con più righe restano un candidato a sé, perché scegliere una riga
// sarebbe una scelta per ordine.
func raggruppa(ammesse []letturaAmmessa) ([]*gruppo, []evidenze.Diagnostica) {
	righe := map[string][]letturaAmmessa{} // chiave → una lettura per riga distinta
	for _, a := range ammesse {
		if a.riga == "" {
			continue
		}
		nuova := true
		for _, x := range righe[a.chiave] {
			nuova = nuova && x.riga != a.riga
		}
		if nuova {
			righe[a.chiave] = append(righe[a.chiave], a)
		}
	}
	perChiave := map[string]*gruppo{}
	var ordine []*gruppo
	for _, a := range ammesse {
		riga := a.riga
		if riga == "" && len(righe[a.chiave]) == 1 {
			riga = righe[a.chiave][0].riga
		}
		a.cellaRiga = a.riga != "" && a.riga == riga
		k := a.chiave + "\x00" + riga
		g, ok := perChiave[k]
		if !ok {
			g = &gruppo{riga: riga}
			perChiave[k] = g
			ordine = append(ordine, g)
		}
		g.letture = append(g.letture, a)
	}
	for _, g := range ordine {
		g.principale = -1
		for i, a := range g.letture {
			if g.principale < 0 {
				g.principale = i
				continue
			}
			b := g.letture[g.principale]
			switch {
			case g.riga != "":
				// Per una riga la sua cella; fra più celle della stessa riga, l'ID minore.
				if a.cellaRiga && (!b.cellaRiga || precede(a.ev, b.ev)) {
					g.principale = i
				}
			case rangoScelta(a.ev) != rangoScelta(b.ev):
				if rangoScelta(a.ev) > rangoScelta(b.ev) {
					g.principale = i
				}
			case precede(a.ev, b.ev):
				g.principale = i
			}
		}
	}

	var diag []evidenze.Diagnostica
	chiavi := make([]string, 0, len(righe))
	for k, rr := range righe {
		if len(rr) > 1 {
			chiavi = append(chiavi, k)
		}
	}
	sort.Strings(chiavi)
	for _, k := range chiavi {
		rr := append([]letturaAmmessa(nil), righe[k]...)
		sort.SliceStable(rr, func(i, j int) bool { return rr[i].riga < rr[j].riga })
		var rif []string
		for _, x := range rr {
			rif = append(rif, idFonteMessaggio(x.ev.MessaggioID), x.entita)
		}
		diag = append(diag, evidenze.Diagnostica{
			Codice:    CodiceRigheStessaBase,
			Gravita:   evidenze.GravitaAvviso,
			Natura:    evidenze.NaturaDati,
			Percorso:  "righe",
			Messaggio: fmt.Sprintf("la stessa base in %d righe di tabella: un candidato per riga, ognuno con la sua quantità, nessuna fusione (D3)", len(rr)),
			Rif:       rif,
		})
	}
	return ordine, diag
}

// idFonteMessaggio: l'ID della fonte di un messaggio, come lo scrive l'adattatore («messaggio:<uuid>»).
func idFonteMessaggio(id uuid.UUID) string { return fonteMessaggio + ":" + id.String() }

// candidatoOrdinabile: un candidato con le chiavi del suo ordine (regola 7).
type candidatoOrdinabile struct {
	c              CandidatoProdotto
	tabella, nRiga int
}

// candidato costruisce il candidato di un gruppo, con la quantità della riga (regola 4) e il motivo (regola 1).
func candidato(g *gruppo, quantita map[string][]motorea.AttributoLetto, alternative map[string][]EvidenzaProdotto, valutati map[uuid.UUID]bool) (candidatoOrdinabile, []evidenze.Diagnostica) {
	pr := g.letture[g.principale]
	f := pr.l.Forma
	c := CandidatoProdotto{
		MessaggioID:     pr.ev.MessaggioID,
		Segmento:        pr.ev.Segmento,
		Uso:             pr.ev.Uso,
		OrigineUso:      pr.ev.OrigineUso,
		Origine:         OrigineProposto,
		Namespace:       f.Namespace,
		CodiceRichiesto: f.CodiceRichiesto,
		Lettura:         pr.l.ID,
		Base:            copiaBase(f.Base),
		Qualificatori:   qualificatoriAttribuiti(f.Affissi),
		Motivo:          MotivoCandidatoRichiestaNonValutata,
	}
	if g.riga != "" {
		c.Riga = pr.entita
	}
	alt := map[string]EvidenzaProdotto{}
	for _, a := range g.letture {
		c.Evidenze = append(c.Evidenze, a.ev)
		if valutati[a.ev.MessaggioID] {
			c.Motivo = MotivoCandidatoDaConfermare
		}
		for _, x := range alternative[chiaveLettura(a.ev.MessaggioID, a.l.ID)] {
			alt[chiaveLettura(x.MessaggioID, x.Lettura)] = x
		}
	}
	sort.SliceStable(c.Evidenze, func(i, j int) bool { return precede(c.Evidenze[i], c.Evidenze[j]) })
	delete(alt, chiaveLettura(pr.ev.MessaggioID, pr.l.ID))
	for _, x := range alt {
		c.Alternative = append(c.Alternative, x)
	}
	sort.SliceStable(c.Alternative, func(i, j int) bool { return precede(c.Alternative[i], c.Alternative[j]) })
	out := candidatoOrdinabile{c: c, tabella: pr.tabella, nRiga: pr.nRiga}

	if g.riga == "" {
		return out, nil
	}
	attr := quantita[g.riga]
	if len(attr) == 1 && attr[0].Stato == motorea.StatoAttribuito {
		if n, err := strconv.Atoi(attr[0].Normalizzato); err == nil {
			out.c.Quantita = &n
			out.c.EvidenzaQuantita = append([]string{attr[0].ID, attr[0].UnitaID}, attr[0].Evidenze...)
			return out, nil
		}
	}
	if len(attr) == 0 {
		return out, nil
	}
	rif := []string{pr.l.ID}
	for _, a := range attr {
		rif = append(rif, a.ID)
	}
	return out, []evidenze.Diagnostica{{
		Codice:    CodiceQuantitaNonIntera,
		Gravita:   evidenze.GravitaAvviso,
		Natura:    evidenze.NaturaDati,
		Percorso:  "messaggi[" + pr.ev.MessaggioID.String() + "].candidati[" + pr.l.ID + "].quantita",
		Messaggio: "la riga ha la colonna della quantità, ma non un intero (cella non intera o colonna ambigua): la quantità resta non provata",
		Rif:       rif,
	}}
}

// diagnosiNonValutate: ancoraggio.richiesta_non_valutata per ogni messaggio senza il gesto 1 con evidenze in un
// candidato che ha il motivo richiesta_non_valutata (regola 1), con le letture del messaggio.
func diagnosiNonValutate(considerati []MessaggioInterpretato, candidati []candidatoOrdinabile, valutati map[uuid.UUID]bool) []evidenze.Diagnostica {
	var out []evidenze.Diagnostica
	for _, m := range considerati {
		if valutati[m.MessaggioID] {
			continue
		}
		var letture []string
		for _, c := range candidati {
			if c.c.Motivo != MotivoCandidatoRichiestaNonValutata {
				continue
			}
			for _, e := range c.c.Evidenze {
				if e.MessaggioID == m.MessaggioID {
					letture = append(letture, e.Lettura)
				}
			}
		}
		if len(letture) == 0 {
			continue
		}
		sort.Strings(letture)
		out = append(out, evidenze.Diagnostica{
			Codice:   CodiceRichiestaNonValutata,
			Gravita:  evidenze.GravitaAvviso,
			Natura:   evidenze.NaturaDati,
			Percorso: "messaggi[" + m.MessaggioID.String() + "]",
			Messaggio: fmt.Sprintf("il messaggio non ha il gesto 1: le sue %d letture sostengono candidati che restano da confermare, con il motivo richiesta_non_valutata",
				len(letture)),
			Rif: append([]string{m.Documento.Fonte.ID}, letture...),
		})
	}
	return out
}

// ordinaCandidati: il testo libero prima delle righe; le righe per (messaggio, tabella, riga); poi base, codice
// richiesto, namespace e lettura principale (6.4.4, regola 7): un ordine totale.
func ordinaCandidati(c []candidatoOrdinabile) {
	sort.SliceStable(c, func(i, j int) bool {
		a, b := c[i], c[j]
		if (a.c.Riga == "") != (b.c.Riga == "") {
			return a.c.Riga == ""
		}
		if a.c.Riga != "" {
			if a.c.MessaggioID != b.c.MessaggioID {
				return a.c.MessaggioID.String() < b.c.MessaggioID.String()
			}
			if a.tabella != b.tabella {
				return a.tabella < b.tabella
			}
			if a.nRiga != b.nRiga {
				return a.nRiga < b.nRiga
			}
		}
		for _, x := range [][2]string{
			{a.c.Base.Normalizzata, b.c.Base.Normalizzata}, {a.c.CodiceRichiesto, b.c.CodiceRichiesto},
			{a.c.Namespace, b.c.Namespace}, {a.c.MessaggioID.String(), b.c.MessaggioID.String()}, {a.c.Lettura, b.c.Lettura},
		} {
			if x[0] != x[1] {
				return x[0] < x[1]
			}
		}
		return false
	})
}

// ---- la chiusura dell'esito ----

// ingressoProdotti: ciò che copre HashIngresso.
type ingressoProdotti struct {
	VersioneServizio string              `json:"versione_servizio"`
	Messaggi         []ingressoMessaggio `json:"messaggi,omitempty"`
	Richiesta        RichiestaValutata   `json:"richiesta"`
}

type ingressoMessaggio struct {
	MessaggioID     uuid.UUID `json:"messaggio_id"`
	BundleID        string    `json:"bundle_id"`
	Interpretazione string    `json:"interpretazione"`
}

// chiudiEsito mette in ordine canonico esclusioni e diagnostiche (i candidati sono già in ordine) e calcola
// HashIngresso e Impronta (par.3.4.2, 3.4.3).
func chiudiEsito(e EsitoProdotti, considerati []MessaggioInterpretato, r RichiestaValutata) (EsitoProdotti, error) {
	sort.SliceStable(e.Esclusioni, func(i, j int) bool {
		a, b := e.Esclusioni[i], e.Esclusioni[j]
		if a.MessaggioID != b.MessaggioID {
			return a.MessaggioID.String() < b.MessaggioID.String()
		}
		return a.Lettura < b.Lettura
	})
	sort.SliceStable(e.Diagnostiche, func(i, j int) bool {
		return chiaveDiagnostica(e.Diagnostiche[i]) < chiaveDiagnostica(e.Diagnostiche[j])
	})
	if len(e.Candidati) == 0 {
		e.Candidati = nil
	}
	if len(e.Esclusioni) == 0 {
		e.Esclusioni = nil
	}
	if len(e.Diagnostiche) == 0 {
		e.Diagnostiche = nil
	}

	in := ingressoProdotti{VersioneServizio: VersioneServizio, Richiesta: richiestaCanonica(r)}
	for _, m := range considerati {
		in.Messaggi = append(in.Messaggi, ingressoMessaggio{MessaggioID: m.MessaggioID, BundleID: m.Documento.BundleID, Interpretazione: m.Interpretazione.ID})
	}
	h, err := jsoncanonico.ImprontaDi(in)
	if err != nil {
		return EsitoProdotti{}, fmt.Errorf("ancoraggio: impronta degli ingressi dei prodotti: %w", err)
	}
	e.HashIngresso = h
	e.Impronta = ""
	imp, err := jsoncanonico.ImprontaDi(e)
	if err != nil {
		return EsitoProdotti{}, fmt.Errorf("ancoraggio: impronta dell'esito dei prodotti: %w", err)
	}
	e.Impronta = imp
	return e, nil
}

// richiestaCanonica: la richiesta con gli elenchi in ordine e i tempi in UTC al millisecondo (par.3.4.3), per
// l'impronta. Non cambia la richiesta di chi chiama.
func richiestaCanonica(r RichiestaValutata) RichiestaValutata {
	c := r
	c.Messaggi = append([]uuid.UUID(nil), r.Messaggi...)
	sort.Slice(c.Messaggi, func(i, j int) bool { return c.Messaggi[i].String() < c.Messaggi[j].String() })
	c.Segmenti = append([]SegmentoPertinente(nil), r.Segmenti...)
	sort.SliceStable(c.Segmenti, func(i, j int) bool {
		a, b := c.Segmenti[i], c.Segmenti[j]
		if a.MessaggioID != b.MessaggioID {
			return a.MessaggioID.String() < b.MessaggioID.String()
		}
		return a.SegmentoID < b.SegmentoID
	})
	c.Gesti = make([]GestoMessaggio, 0, len(r.Gesti))
	for _, g := range r.Gesti {
		if g.AgganciatoIl != nil {
			t := g.AgganciatoIl.UTC().Truncate(time.Millisecond)
			g.AgganciatoIl = &t
		}
		c.Gesti = append(c.Gesti, g)
	}
	sort.SliceStable(c.Gesti, func(i, j int) bool { return c.Gesti[i].MessaggioID.String() < c.Gesti[j].MessaggioID.String() })
	if len(c.Messaggi) == 0 {
		c.Messaggi = nil
	}
	if len(c.Segmenti) == 0 {
		c.Segmenti = nil
	}
	if len(c.Gesti) == 0 {
		c.Gesti = nil
	}
	return c
}

// chiaveDiagnostica: l'ordine canonico delle diagnostiche, (codice, percorso, riferimenti) e poi il messaggio, come
// in motorea.
func chiaveDiagnostica(d evidenze.Diagnostica) string {
	return d.Codice + "\x00" + d.Percorso + "\x00" + strings.Join(d.Rif, "\x01") + "\x00" + d.Messaggio
}

// ---- aiuti ----

// chiaveQualificatori: i qualificatori attribuiti di una lettura (regola, fase, destinazione), in ordine. Un affisso
// riconosciuto ma non attribuito non distingue due prodotti (D5).
func chiaveQualificatori(affissi []motorea.AffissoLetto) string {
	var parti []string
	for _, a := range affissi {
		if a.Attribuito && a.Valore != nil {
			parti = append(parti, a.Regola+"\x01"+a.Valore.Fase+"\x01"+a.Valore.Destinazione)
		}
	}
	sort.Strings(parti)
	return strings.Join(parti, "\x02")
}

// qualificatoriAttribuiti: copia degli affissi attribuiti, nell'ordine del testo; nil se nessuno.
func qualificatoriAttribuiti(affissi []motorea.AffissoLetto) []motorea.AffissoLetto {
	var out []motorea.AffissoLetto
	for _, a := range affissi {
		if !a.Attribuito || a.Valore == nil {
			continue
		}
		v := *a.Valore
		a.Valore = &v
		out = append(out, a)
	}
	return out
}

// copiaBase: la base con gli elenchi copiati, così l'esito non condivide memoria con l'interpretazione.
func copiaBase(b motorea.BaseLetta) motorea.BaseLetta {
	b.Segmenti = append([]motorea.SegmentoLetto(nil), b.Segmenti...)
	b.Mancanti = append([]string(nil), b.Mancanti...)
	if len(b.Segmenti) == 0 {
		b.Segmenti = nil
	}
	if len(b.Mancanti) == 0 {
		b.Mancanti = nil
	}
	return b
}

// suCellaQuantita: la lettura sta (anche come stessa occorrenza) nella cella di un attributo «quantita».
func suCellaQuantita(l motorea.LetturaCodice, celle map[string]bool) bool {
	if celle[l.UnitaID] {
		return true
	}
	for _, u := range l.AltreUnita {
		if celle[u] {
			return true
		}
	}
	return false
}

func haRuolo(ruoli []grammatica.Ruolo, r grammatica.Ruolo) bool {
	for _, x := range ruoli {
		if x == r {
			return true
		}
	}
	return false
}

func contiene(elenco []string, s string) bool {
	for _, x := range elenco {
		if x == s {
			return true
		}
	}
	return false
}
