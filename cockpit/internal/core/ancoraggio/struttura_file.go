package ancoraggio

import (
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// La struttura di un file STEP (piano A, 6.4.5; contratto §1.3, §1.4, §2.2, §2.5; commit P6a, B2): il grafo del
// file com'è nei fatti del worker, letto dal documento dell'adattatore di A1b e dalla sua interpretazione. È un
// ingresso di ProponiStrutture (ContestoStrutturale.Strutture), che valutazione riempie con StrutturaDa. Solo lo STEP
// dà una struttura (T-E1-09, R68 A): un PDF, un IGS, un DXF mai.

// FileInterpretato: un allegato con il suo documento e la sua interpretazione (par.3.3.7), come MessaggioInterpretato
// per un messaggio. Lo prepara valutazione dalla fotografia: estrazione.DaAllegato con i fatti alla terna corrente,
// poi Interpreta con l'uso sconosciuto (un file non ha segmenti). La disponibilità del file (Disponibilita, par.3.3.7,
// con senza_testo di T-B0-31) serve agli ancoraggi dei file e arriva con loro (B4): la struttura non la guarda.
type FileInterpretato struct {
	AllegatoID      uuid.UUID
	Documento       evidenze.DocumentoEvidenze
	Interpretazione motorea.Interpretazione
}

// I valori del documento che la struttura guarda, come li scrive l'adattatore STEP di A1b (mappatura-step-1): il
// tipo dell'entità di un nodo, il tipo del legame padre-figlio, le capacità «struttura» e «grafo_completo» con i
// loro stati, lo stato «errore» della qualità, il tipo del riferimento ai fatti di un file, i campi d'identità di un
// nodo. Sono ripetuti qui perché estrazione non li esporta, e ancoraggio non la importa; se cambiano, lo dicono le
// prove del pacchetto, che passano dall'adattatore.
const (
	entitaNodoSTEP           = "nodo_step"
	legamePadreFiglioSTEP    = "padre_figlio_step"
	capacitaStruttura        = "struttura"
	capacitaGrafoCompleto    = "grafo_completo"
	statoCapacitaDisponibile = "disponibile"
	statoCapacitaParziale    = "parziale"
	statoQualitaErrore       = "errore"
	riferimentoAnalisiFile   = "analisi_file"
	campoID                  = "id"
	campoNome                = "nome"
)

// motivoGrafoNonDichiarato: il motivo di un grafo non completo quando il documento non dice perché (la capacità
// grafo_completo senza motivo). Il grafo non si dichiara mai completo per assenza di un motivo (R32 b).
const motivoGrafoNonDichiarato = "completezza del grafo non dichiarata dal documento"

// PrefissoRifNodo e RifNodo: il riferimento di un nodo di uno STEP, «nodo:<sha256>:<chiave>» (T-E1-04; emendamento
// E1 §4.2): lo sha256 del contenuto del file più la chiave del PRODUCT nel file («#12»), mai il codice. Una correzione
// del codice lascia lo stesso nodo; due file con lo stesso codice danno due nodi; lo stesso contenuto in due allegati
// dà gli stessi riferimenti, e le strutture restano due, una per allegato (nessuna fusione: 6.4.5, regola 7). La
// chiave da sola, fuori dal suo file, non significa niente (il contratto del worker): per questo c'è lo sha256. La
// stessa forma la usano le righe legacy dello stesso nodo, che hanno sha256 e chiave (contratto §1.4).
const PrefissoRifNodo = "nodo:"

// RifNodo: il riferimento del nodo con quella chiave nel contenuto con quello sha256.
func RifNodo(sha256, chiave string) string { return PrefissoRifNodo + sha256 + ":" + chiave }

// ArcoPercorso: un arco con quantità e occorrenze, e da dove viene (6.4.5; T-14): i fatti dello STEP
// (padre_figlio_step), una relazione confermata della RFQ (componente_relazione) o una riga aperta di
// relazione_proposta. Conserva archi, quantità e percorrenze anche quando lo stesso figlio sta sotto più padri
// (FIGLIO-CONDIVISO; v3 §2 r.162). L'arco dell'albero proposto di una struttura del prodotto è ArcoProposto (M2):
// ArcoPercorso è l'arco con la sua origine, nel grafo di un file e nel contesto.
//   - Padre, Figlio: i riferimenti dei due estremi: RifNodo per i fatti e per le righe proposte, RifComponente per le
//     relazioni confermate.
//   - Quantita: per i fatti il numero di occorrenze della coppia (senza unità di misura, che i fatti non hanno); per
//     il contesto la quantità della relazione o della riga; nil = non data.
//   - Occorrenze: i riferimenti alle occorrenze che i fatti elencano (i NAUO, al più 20: il totale lo dice Quantita).
//   - Origine: fatti | confermato | proposto.
type ArcoPercorso struct {
	Padre      string   `json:"padre"`
	Figlio     string   `json:"figlio"`
	Quantita   *int     `json:"quantita,omitempty"`
	Occorrenze []string `json:"occorrenze,omitempty"`
	Origine    string   `json:"origine"`
}

// I valori di ArcoPercorso.Origine (6.4.5).
const (
	OrigineArcoFatti      = "fatti"
	OrigineArcoConfermato = "confermato"
	OrigineArcoProposto   = "proposto"
)

// NodoStruttura: un nodo del grafo di un file, con le sue letture d'identità (6.4.5: «nodi con le letture
// d'identità»).
//   - Rif: RifNodo(sha256, chiave). EntitaID: l'entità nodo_step del documento («e:step:#12»), locale al documento.
//     Chiave: la chiave del PRODUCT nel file.
//   - Letture: le letture d'identità del nodo, cioè quelle dei suoi campi id e nome (funzione identita_file per una
//     radice, struttura per gli altri nodi: router-2, righe 9 e 10), in ordine di ID. Vuote: nessuna famiglia lo legge
//     (le saldature, D10). Il nodo si mostra lo stesso, ma non è mai una radice candidata né un candidato (6.4.5).
type NodoStruttura struct {
	Rif      string                  `json:"rif"`
	EntitaID string                  `json:"entita_id"`
	Chiave   string                  `json:"chiave"`
	Letture  []motorea.LetturaCodice `json:"letture,omitempty"`
}

// StrutturaFile: il grafo STEP di un file, ricavato dal suo documento e dalla sua interpretazione (6.4.5, E-20):
// radici, nodi con le letture d'identità, archi padre_figlio_step con quantità e occorrenze, completezza del grafo.
//   - AllegatoID, BundleID, Sha256: l'allegato, il documento e il contenuto da cui viene. Due allegati con lo stesso
//     contenuto danno due strutture, con gli stessi riferimenti dei nodi.
//   - Radici: i Rif delle radici del file, quelle dei fatti (struttura.radici: le unità con il contesto radice_step),
//     in ordine.
//   - Nodi: in ordine di Rif. Archi: i fatti, con l'origine «fatti», in ordine di (padre, figlio).
//   - GrafoCompleto e MotivoGrafo: la capacità grafo_completo del documento, che viene dal motivo della 0020
//     (struttura_motivo_parziale, R32 b): una qualità della fonte, mai «completa» da sola (contratto T-10). Falso con
//     il motivo, anche per un file «solo parti» o con più radici: la completezza che conta per un target è quella
//     delle sue strutture (6.4.5, regola 2).
type StrutturaFile struct {
	AllegatoID    uuid.UUID       `json:"allegato_id"`
	BundleID      string          `json:"bundle_id"`
	Sha256        string          `json:"sha256"`
	Radici        []string        `json:"radici,omitempty"`
	Nodi          []NodoStruttura `json:"nodi,omitempty"`
	Archi         []ArcoPercorso  `json:"archi,omitempty"`
	GrafoCompleto bool            `json:"grafo_completo"`
	MotivoGrafo   string          `json:"motivo_grafo,omitempty"`
}

// StrutturaDa ricava la struttura di un file STEP (6.4.5). È pura: valutazione la usa per riempire
// ContestoStrutturale.Strutture (R42 B). Falso, senza struttura, quando:
//   - il documento non viene dai fatti di un file (riferimento ai fatti diverso da «analisi_file», o senza sha256);
//   - il worker non ha letto il file (qualità «errore»), o la capacità «struttura» non è disponibile né parziale (un
//     PDF, un IGS, un DXF, uno STEP senza fatti o con una struttura di una versione che va rianalizzata): solo lo STEP
//     letto dà una struttura (T-E1-09, R68 A);
//   - non c'è nessun nodo;
//   - l'interpretazione non è quella del documento (un altro BundleID): le letture non si possono attribuire, e una
//     struttura con le letture sbagliate sarebbe peggio di nessuna.
//
// Altrimenti la struttura c'è anche con il grafo non completo (GrafoCompleto falso, con il motivo), anche con la
// lettura troncata (capacità «struttura» parziale), e anche con nodi che nessuna famiglia legge.
func StrutturaDa(f FileInterpretato) (StrutturaFile, bool) {
	d := f.Documento
	rf := d.Fonte.RiferimentoFatti
	if rf.Tipo != riferimentoAnalisiFile || rf.Sha256 == "" || d.Qualita.Stato == statoQualitaErrore {
		return StrutturaFile{}, false
	}
	if s := capacitaDi(d, capacitaStruttura).Stato; s != statoCapacitaDisponibile && s != statoCapacitaParziale {
		return StrutturaFile{}, false
	}
	if f.Interpretazione.BundleID != d.BundleID {
		return StrutturaFile{}, false
	}

	nodi := map[string]*NodoStruttura{} // per EntitaID: solo per cercare, mai per scorrere
	var ordine []*NodoStruttura
	for _, e := range d.Entita {
		if e.Tipo != entitaNodoSTEP || nodi[e.ID] != nil {
			continue
		}
		n := &NodoStruttura{Rif: RifNodo(rf.Sha256, e.ChiaveOriginale), EntitaID: e.ID, Chiave: e.ChiaveOriginale}
		nodi[e.ID] = n
		ordine = append(ordine, n)
	}
	if len(ordine) == 0 {
		return StrutturaFile{}, false
	}

	// Le radici: un nodo è radice se una sua unità ha il contesto radice_step (le radici dei fatti, A1b). Le unità
	// d'identità: i campi id e nome di un nodo.
	radice := map[string]bool{}
	identita := map[string]string{} // unità → entità del nodo
	for _, u := range d.Unita {
		n := nodi[u.EntitaID]
		if n == nil {
			continue
		}
		if u.Selettore.Contesto == evidenze.ContestoRadiceSTEP {
			radice[n.Rif] = true
		}
		if c := u.Selettore.Campo.Valore; c == campoID || c == campoNome {
			identita[u.ID] = u.EntitaID
		}
	}
	for _, l := range f.Interpretazione.Letture {
		if e, ok := identita[l.UnitaID]; ok {
			nodi[e].Letture = append(nodi[e].Letture, l)
		}
	}

	s := StrutturaFile{AllegatoID: f.AllegatoID, BundleID: d.BundleID, Sha256: rf.Sha256}
	for _, n := range ordine {
		sort.SliceStable(n.Letture, func(i, j int) bool { return n.Letture[i].ID < n.Letture[j].ID })
		if radice[n.Rif] {
			s.Radici = append(s.Radici, n.Rif)
		}
		s.Nodi = append(s.Nodi, *n)
	}
	for _, g := range d.Legami {
		p, c := nodi[g.Da], nodi[g.A]
		if g.Tipo != legamePadreFiglioSTEP || p == nil || c == nil {
			continue
		}
		s.Archi = append(s.Archi, ArcoPercorso{Padre: p.Rif, Figlio: c.Rif, Quantita: copiaIntero(g.Quantita),
			Occorrenze: copiaStringhe(g.Evidenze), Origine: OrigineArcoFatti})
	}
	sort.Strings(s.Radici)
	sort.SliceStable(s.Nodi, func(i, j int) bool { return s.Nodi[i].Rif < s.Nodi[j].Rif })
	ordinaArchi(s.Archi)

	g := capacitaDi(d, capacitaGrafoCompleto)
	s.GrafoCompleto = g.Stato == statoCapacitaDisponibile
	if !s.GrafoCompleto {
		s.MotivoGrafo = g.Motivo
		if s.MotivoGrafo == "" {
			s.MotivoGrafo = motivoGrafoNonDichiarato
		}
	}
	return s, true
}

// capacitaDi: la capacità del documento con quel nome; vuota se non c'è.
func capacitaDi(d evidenze.DocumentoEvidenze, nome string) evidenze.Capacita {
	for _, c := range d.Qualita.Capacita {
		if c.Nome == nome {
			return c
		}
	}
	return evidenze.Capacita{}
}

// ordinaArchi: l'ordine canonico degli archi, (origine, padre, figlio), poi quantità e occorrenze: un ordine totale,
// perché nel contesto lo stesso arco può arrivare con quantità diverse.
func ordinaArchi(a []ArcoPercorso) {
	sort.SliceStable(a, func(i, j int) bool { return chiaveArco(a[i]) < chiaveArco(a[j]) })
}

// chiaveArco: la chiave dell'ordine e dei doppioni di un arco, con tutti i suoi campi (una quantità nil e una data
// hanno chiavi diverse).
func chiaveArco(a ArcoPercorso) string {
	q := ""
	if a.Quantita != nil {
		q = "q" + strconv.Itoa(*a.Quantita)
	}
	return a.Origine + "\x00" + a.Padre + "\x00" + a.Figlio + "\x00" + q + "\x00" + strings.Join(a.Occorrenze, "\x01")
}

// copiaIntero: un nuovo puntatore allo stesso valore, così l'uscita non condivide memoria con il documento.
func copiaIntero(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// copiaStringhe: una copia dell'elenco; nil se vuoto.
func copiaStringhe(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return append([]string(nil), s...)
}
