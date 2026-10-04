package estrazione

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/platform/contratti/worker"
)

// La mappatura STEP (mappatura-step-1; 5.4.5), dai fatti decodificati con worker.DecodificaStruttura
// (tipi.go:591-605). Il worker dà i quattro attributi grezzi di ogni PRODUCT, le relazioni per coppia con la
// quantità e le radici; qui diventano entità, unità e legami, senza decidere che cosa sia un codice o una
// revisione: lo decide il motore, con la grammatica del cliente.
const (
	chiaveStruttura = "struttura" // la parte dei fatti con il grafo (RisultatoAnalisi.dettagli["struttura"])

	capStruttura     = "struttura"
	capGrafoCompleto = "grafo_completo"

	// maxTestoSTEP: il tetto del worker per una stringa del file, in code point Python (step_struttura.py:60).
	maxTestoSTEP = 200
	// maxRiferimentiNAUO: quante occorrenze di una coppia il worker elenca (step_struttura.py:285-286); le
	// altre le conta in «altre».
	maxRiferimentiNAUO = 20

	motivoSTEPParziale = "valore decodificato dal worker; nessun offset nel file STEP"

	tipoEntitaNodo = "nodo_step"
	tipoLegameSTEP = "padre_figlio_step"
)

// estensioniSTEP: i file che il worker legge come STEP (worker_analisi.py:1071).
var estensioniSTEP = map[string]bool{"stp": true, "step": true}

// Gli avvisi con cui il worker dice che il file non si è letto per niente (step_struttura.py:149, :162). Sono
// frasi del worker, non dati del cliente: contano solo se non c'è nessun nodo.
var avvisiDiLetturaFallita = []string{"non e' un file STEP Part 21", "struttura non letta:"}

// eSTEP: un file è uno STEP per l'adattatore se l'estensione è quella che il worker legge come STEP, o se i
// fatti hanno la parte «struttura» (anche di una versione che non si legge più).
func eSTEP(est string, chiavi map[string]bool) bool {
	return estensioniSTEP[est] || chiavi[chiaveStruttura]
}

// attributoSTEP: un attributo grezzo di un PRODUCT e come diventa un'unità.
type attributoSTEP struct {
	suffisso  string // dell'ID dell'unità: u:step:#n:<suffisso>
	campo     string // il campo del selettore (variante step)
	attributo string // PosSTEP.Attributo
	etichetta string // l'attributo nel file STEP (CampoOriginale.Etichetta)
	parser    string // il campo nei fatti del worker (CampoOriginale.Parser)
}

var (
	attributoID          = attributoSTEP{"id", "id", "id", "PRODUCT.id", "id_grezzo"}
	attributoNome        = attributoSTEP{"nome", "nome", "nome", "PRODUCT.name", "nome_grezzo"}
	attributoDescrizione = attributoSTEP{"descrizione", "descrizione", "descrizione", "PRODUCT.description", "descrizione_grezza"}
	// La formazione è un dato grezzo, mai confrontato con una revisione (D1, E-12): il selettore ha il campo
	// «revisione» (parte 1 §4.2), il localizzatore dice «formazione».
	attributoFormazione = attributoSTEP{"revisione", "revisione", "formazione", "PRODUCT_DEFINITION_FORMATION.id", "rev_grezza"}
)

// contenutoSTEP aggiunge al documento il contenuto di uno STEP.
//   - Nessun fatto: step.non_analizzato; la struttura non è disponibile.
//   - Struttura assente o di una versione che non si legge (DecodificaStruttura falsa, per esempio la v1, che
//     portava i codici decisi dal worker): step.struttura_assente; la struttura non è disponibile.
//   - Altrimenti: i nodi, le relazioni, la qualità della lettura e la completezza del grafo.
func (c *documento) contenutoSTEP(f *fotorfq.Fatti) {
	if f == nil {
		c.capacita(capStruttura, statoNonDisponibile, "nessun fatto del worker: lo STEP non è stato analizzato")
		c.peggiora(statoParziale)
		c.diagnostica(evidenze.Diagnostica{
			Codice:    CodiceSTEPNonAnalizzato,
			Gravita:   evidenze.GravitaNota,
			Natura:    evidenze.NaturaDati,
			Messaggio: "lo STEP non ha fatti del worker: del file si legge solo il nome",
		})
		return
	}
	s, ok := worker.DecodificaStruttura(f.Payload)
	if !ok {
		c.capacita(capStruttura, statoNonDisponibile, "i fatti non hanno una struttura che si legge (assente, o di una versione che portava le letture del worker)")
		c.peggiora(statoParziale)
		c.diagnostica(evidenze.Diagnostica{
			Codice:    CodiceSTEPStrutturaAssente,
			Gravita:   evidenze.GravitaAvviso,
			Natura:    evidenze.NaturaDati,
			Percorso:  chiaveStruttura,
			Messaggio: "struttura STEP assente o di una versione precedente alla 2: va rianalizzata, non letta",
		})
		return
	}
	c.d.Qualita.Metodo = metodoParser
	c.d.Fonte.RiferimentoFatti.SottoversioneSTEP = s.Versione

	if len(s.Nodi) == 0 {
		if avviso := letturaFallita(s.Avvisi); avviso != "" {
			c.capacita(capStruttura, statoNonDisponibile, "il worker non ha letto il file: «"+avviso+"»")
			c.peggiora(statoErrore)
			c.diagnostica(evidenze.Diagnostica{
				Codice:    CodiceSTEPNonLetto,
				Gravita:   evidenze.GravitaAvviso,
				Natura:    evidenze.NaturaDati,
				Percorso:  chiaveStruttura + ".avvisi",
				Messaggio: "nessun nodo, e il worker dice che il file non si è letto: «" + avviso + "»",
			})
			return
		}
	}

	radici := make(map[string]bool, len(s.Radici))
	for _, r := range s.Radici {
		radici[r] = true
	}
	testiTroncati := s.Scarti != nil && s.Scarti.TestiTroncati > 0
	var candidate []string
	for _, n := range s.Nodi {
		candidate = append(candidate, c.nodoSTEP(n, radici[n.Chiave], testiTroncati)...)
	}
	for _, r := range s.Relazioni {
		c.relazioneSTEP(r)
	}
	c.qualitaSTEP(s, candidate)
	c.grafoCompleto(f.MotivoParziale)
}

// letturaFallita: il primo avviso del worker che dice che il file non si è letto, o "".
func letturaFallita(avvisi []string) string {
	for _, a := range avvisi {
		for _, p := range avvisiDiLetturaFallita {
			if strings.HasPrefix(a, p) {
				return a
			}
		}
	}
	return ""
}

// nodoSTEP aggiunge un nodo: l'entità «nodo_step» e un'unità per ogni attributo non vuoto, legata al nodo con
// EntitaID (legami-1). Il selettore è radice_step.* se la chiave sta fra le radici dei fatti, altrimenti
// nodo_step.*: le radici vengono da struttura.radici, mai da product_step (par.2.5). Restituisce gli ID delle
// unità candidate troncate.
func (c *documento) nodoSTEP(n worker.NodoSTEP, radice, testiTroncati bool) []string {
	idEntita := "e:step:" + n.Chiave
	c.entita(evidenze.EntitaLocale{
		ID:              idEntita,
		Tipo:            tipoEntitaNodo,
		ChiaveOriginale: n.Chiave,
		// La foglia vuole un attributo anche per l'entità: il nodo è il PRODUCT, e il suo identificatore nel
		// file è PRODUCT.id.
		Posizione: evidenze.Localizzatore{Tipo: "step", STEP: &evidenze.PosSTEP{Chiave: n.Chiave, Attributo: attributoID.attributo}},
	})
	contesto := evidenze.ContestoNodoSTEP
	if radice {
		contesto = evidenze.ContestoRadiceSTEP
	}
	riferimenti := riferimentiDelNodo(n.Evidenza)

	var candidate []string
	aggiungi := func(a attributoSTEP, id, valore, parser string, rif []string) {
		if valore == "" {
			return
		}
		troncata := testiTroncati && lunghezzaPython(valore) >= maxTestoSTEP
		c.unita(evidenze.UnitaEvidenza{
			ID:             id,
			EntitaID:       idEntita,
			Selettore:      evidenze.Selettore{Contesto: contesto, Campo: evidenze.CampoFonte{Variante: evidenze.VarianteStep, Valore: a.campo}},
			Testo:          valore,
			CampoOriginale: evidenze.CampoOriginale{Etichetta: a.etichetta, Parser: parser, Mappatura: mappaturaSTEP},
			Posizione:      evidenze.Localizzatore{Tipo: "step", STEP: &evidenze.PosSTEP{Chiave: n.Chiave, Attributo: a.attributo, Riferimenti: rif}},
			Qualita:        evidenze.QualitaUnita{Localizzazione: localizzazioneParziale, Motivo: motivoSTEPParziale, Metodo: metodoParser, Troncata: troncata},
		})
		if troncata {
			candidate = append(candidate, id)
		}
		if strings.ContainsRune(valore, utf8.RuneError) {
			c.diagnostica(evidenze.Diagnostica{
				Codice:    CodiceSTEPCarattereSostituito,
				Gravita:   evidenze.GravitaAvviso,
				Natura:    evidenze.NaturaDati,
				Messaggio: "il valore contiene il carattere sostitutivo U+FFFD: un carattere non si è decodificato (per esempio un surrogato spezzato dal troncamento del worker)",
				Rif:       []string{id},
			})
		}
	}
	base := "u:step:" + n.Chiave + ":"
	aggiungi(attributoID, base+attributoID.suffisso, n.IDGrezzo, attributoID.parser, riferimenti)
	aggiungi(attributoNome, base+attributoNome.suffisso, n.NomeGrezzo, attributoNome.parser, riferimenti)
	aggiungi(attributoDescrizione, base+attributoDescrizione.suffisso, n.DescrizioneGrezza, attributoDescrizione.parser, riferimenti)
	aggiungi(attributoFormazione, base+attributoFormazione.suffisso, n.RevGrezza, attributoFormazione.parser, riferimenti)

	// Le formazioni alternative (step_struttura.py:261-264): un'unità in più per ognuna, senza sovrascrivere la
	// prima. Sceglierne una sarebbe inventare la revisione. Non portano i riferimenti dell'evidenza: «formation» e
	// «definition» sono quelli della prima formazione (step_struttura.py:256-260), e il worker non dice da quale
	// entità del file venga ciascuna delle altre. Dargliene uno sarebbe una provenienza inventata (5.0).
	alternative := stringheDi(n.Evidenza["rev_alternative"])
	if len(alternative) > 0 {
		rif := []string{idEntita}
		for i, v := range alternative {
			id := fmt.Sprintf("%s%s:%d", base, attributoFormazione.suffisso, i+2)
			aggiungi(attributoFormazione, id, v, "evidenza.rev_alternative", nil)
			if v != "" {
				rif = append(rif, id)
			}
		}
		c.diagnostica(evidenze.Diagnostica{
			Codice:    CodiceSTEPFormazioniAlternative,
			Gravita:   evidenze.GravitaAvviso,
			Natura:    evidenze.NaturaDati,
			Messaggio: fmt.Sprintf("il PRODUCT ha %d formazioni con id diversi: le altre si conservano accanto alla prima, nessuna si sceglie", len(alternative)+1),
			Rif:       rif,
		})
	}
	return candidate
}

// riferimentiDelNodo: le entità del file da cui viene il nodo, come le dà l'evidenza: la formazione e la
// definizione (step_struttura.py:256-260). Le chiavi della mappa si leggono per nome, mai in ordine di mappa.
func riferimentiDelNodo(ev map[string]any) []string {
	var out []string
	for _, k := range []string{"formation", "definition"} {
		if s, ok := ev[k].(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// relazioneSTEP aggiunge un legame padre_figlio_step, l'unico legame della mappatura (legami-1): da un nodo
// all'altro, con la quantità della coppia e i riferimenti NAUO che il worker elenca (al più 20), più il numero
// delle altre occorrenze. Non nasce nessun legame occorrenza_step: i fatti aggregano per coppia.
func (c *documento) relazioneSTEP(r worker.RelazioneSTEP) {
	righe := stringheDi(r.Evidenza["righe"])
	altre := interoDi(r.Evidenza["altre"])
	if len(righe) > maxRiferimentiNAUO {
		altre += len(righe) - maxRiferimentiNAUO
		righe = righe[:maxRiferimentiNAUO]
	}
	qta := r.Qta
	c.legame(evidenze.LegameFonte{
		ID:       "g:step:" + r.Padre + ">" + r.Figlio,
		Tipo:     tipoLegameSTEP,
		Da:       "e:step:" + r.Padre,
		A:        "e:step:" + r.Figlio,
		Quantita: &qta,
		Evidenze: righe,
		Altre:    altre,
	})
}

// qualitaSTEP: la qualità della lettura dai limiti e dagli scarti dei fatti (5.4.5, «Qualità della fonte»).
// Lo stato di lettura non dice niente della completezza del grafo: quella è grafo_completo.
func (c *documento) qualitaSTEP(s *worker.StrutturaSTEP, candidate []string) {
	struttura := statoDisponibile
	motivo := ""
	if s.Limiti.Troncato {
		struttura, motivo = statoParziale, "lettura fermata dal tetto «"+s.Limiti.Motivo+"» del worker"
		c.peggiora(statoParziale)
		c.diagnostica(evidenze.Diagnostica{
			Codice:    CodiceSTEPTroncato,
			Gravita:   evidenze.GravitaAvviso,
			Natura:    evidenze.NaturaDati,
			Percorso:  chiaveStruttura + ".limiti",
			Messaggio: fmt.Sprintf("il worker ha fermato la lettura sul tetto «%s»: i nodi e le relazioni dopo non ci sono", s.Limiti.Motivo),
		})
	}
	c.capacita(capStruttura, struttura, motivo)

	if s.Scarti == nil {
		c.peggiora(statoParziale)
		c.diagnostica(evidenze.Diagnostica{
			Codice:    CodiceSTEPScartiNonNoti,
			Gravita:   evidenze.GravitaNota,
			Natura:    evidenze.NaturaDati,
			Percorso:  chiaveStruttura + ".scarti",
			Messaggio: fmt.Sprintf("struttura v%d: gli scarti in numeri arrivano dalla v3, quindi che cosa il worker ha lasciato fuori non si sa", s.Versione),
		})
		return
	}
	sc := s.Scarti
	if sc.ProdottiSenzaDefinizione+sc.OccorrenzeNonRisolte+sc.OccorrenzeSuSeStesse > 0 {
		c.peggiora(statoParziale)
		c.diagnostica(evidenze.Diagnostica{
			Codice:   CodiceSTEPScarti,
			Gravita:  evidenze.GravitaAvviso,
			Natura:   evidenze.NaturaDati,
			Percorso: chiaveStruttura + ".scarti",
			Messaggio: fmt.Sprintf("il worker ha lasciato fuori %d PRODUCT senza definizione, %d occorrenze non risolte, %d occorrenze di un pezzo in sé stesso",
				sc.ProdottiSenzaDefinizione, sc.OccorrenzeNonRisolte, sc.OccorrenzeSuSeStesse),
		})
	}
	if sc.TestiTroncati > 0 {
		c.peggiora(statoParziale)
		c.diagnostica(evidenze.Diagnostica{
			Codice:   CodiceSTEPTestoTroncato,
			Gravita:  evidenze.GravitaNota,
			Natura:   evidenze.NaturaDati,
			Percorso: chiaveStruttura + ".scarti.testi_troncati",
			Messaggio: fmt.Sprintf("il worker ha troncato %d testi a %d code point; le unità che arrivano al tetto sono candidate troncate: %d",
				sc.TestiTroncati, maxTestoSTEP, len(candidate)),
			Rif: candidate,
		})
	}
}

// grafoCompleto: la completezza del grafo è una qualità della fonte, e viene dal motivo che il caricatore legge
// con struttura_motivo_parziale (0020_bom_versioni.sql:654-683), la sola definizione (E-20; R32 b = A):
//   - "" vuol dire completo: disponibile;
//   - un testo vuol dire parziale, con quel motivo;
//   - nil vuol dire non determinabile (un export, una prova): la capacità di dirlo non è disponibile.
//
// Il grafo mostrato come risultato è quello interpretato a valle, non questo (5.4.6 punto 16).
func (c *documento) grafoCompleto(motivo *string) {
	switch {
	case motivo == nil:
		c.capacita(capGrafoCompleto, statoNonDisponibile, "completezza non determinabile: nessun motivo dal caricatore")
	case *motivo == "":
		c.capacita(capGrafoCompleto, statoDisponibile, "")
	default:
		c.capacita(capGrafoCompleto, statoParziale, *motivo)
	}
}

// stringheDi: un elenco di stringhe dall'evidenza del worker (decodificata come []any). Un valore che non è
// una stringa non si legge, e non si inventa.
func stringheDi(v any) []string {
	elenco, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, x := range elenco {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// interoDi: un numero intero non negativo dall'evidenza del worker (decodificato come float64); 0 se il valore
// non c'è o non è un intero.
func interoDi(v any) int {
	f, ok := v.(float64)
	if !ok || f < 0 || f != math.Trunc(f) || f > math.MaxInt32 {
		return 0
	}
	return int(f)
}
