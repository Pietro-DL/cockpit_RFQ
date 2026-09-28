package fascicolo

// Le proposte di struttura da uno STEP, come regola pura (Blocco 8, B8.5; addendum A1.1, A4.4; Smistamento
// F5, A5.4.4).
//
// Ogni nodo e ogni arco del file diventa una riga di proposta, e le righe restano tutte: sono i fatti del
// worker letti con le regole del cliente. Che cosa una riga VALGA lo decide l'autorita' (dichiarazioni.go):
//
//	nodo                              qualunque codice abbia                    → aperta, senza componente
//	arco dalla sorgente, figlio deciso come la sorgente                          → scartata, «stesso componente»
//	arco dalla sorgente, figlio deciso, arco uguale nella working                → duplicato (tenuta dei conti)
//	arco dalla sorgente, qta diversa   lettura COMPLETA                          → aperta, «qta diversa: 2 contro 1»
//	arco dalla sorgente, qta diversa   lettura parziale                          → duplicato, con la nota
//	ogni altro arco (guida, o verso un figlio non ancora deciso)                 → aperta
//
// Nessun nodo acquista un'identita' perche' il suo codice e' quello di un componente (decisione dell'utente
// del 27/09): il componente con lo stesso codice e' un suggerimento, calcolato in lettura, mai scritto. Un
// nodo «e'» un componente solo per una decisione di una persona (Decisi) o perche' e' la sorgente di un file
// autorizzato (Sorgenti). Le rimozioni non sono qui: nascono solo da un file autorizzato letto per intero, a
// profondita' 1 (RimozioniFigliDiretti, sotto). Niente di questo file tocca componente o componente_relazione.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// Working e' la BOM working della RFQ come la vede il confronto: gli archi fra componenti attivi. Prima
// portava anche i componenti per codice, per agganciare un nodo al componente con lo stesso codice: e'
// proprio quello che lo Smistamento toglie (F5).
type Working struct {
	Archi map[Arco]int32 // archi fra componenti attivi → quantita'
}

// NuovaWorking costruisce la Working dagli archi letti.
func NuovaWorking(rel []db.ComponenteRelazione) Working {
	w := Working{Archi: map[Arco]int32{}}
	for _, r := range rel {
		w.Archi[Arco{Padre: r.PadreID, Figlio: r.FiglioID}] = r.Qta
	}
	return w
}

// Contesto e' quello che serve a leggere un file dentro una RFQ, oltre al file.
//
// Smistamento F5 (A5.4.4): non ci sono piu' la Radice (lo STEP strutturale di un finito, che faceva della
// sua radice il prodotto per dichiarazione) e gli Identificativi (una radice con il codice della richiesta
// si proponeva «finito», E07). La sorgente la da' l'autorizzazione di una persona; il tipo della radice e'
// quello del componente per cui il file e' autorizzato.
type Contesto struct {
	Working
	// Decisi: i nodi di questo file decisi da una PERSONA (deciso_da), chiave → componente. Un duplicato
	// senza chi l'ha deciso (l'aggancio automatico di prima) non e' una decisione e non c'e' (E08).
	Decisi map[string]uuid.UUID
	// Sorgenti: i nodi del file che una persona ha autorizzato a proporre i figli diretti di un componente,
	// chiave → componente (Dichiarazioni.Sorgenti). Solo gli archi che partono da qui sono nell'autorita'.
	Sorgenti map[string]uuid.UUID
	// Completa: la lettura e' completa (struttura_motivo_parziale vuoto). Solo allora una quantita'
	// diversa e' una proposta.
	Completa bool
}

// PropostaNodo e' la riga di componente_proposta per un nodo del file. Non porta un componente: nessuna
// lettura ne da' uno (F5), e lo stato e' sempre «aperta».
type PropostaNodo struct {
	NodoClassificato
	Tipo  db.TipoComponente
	Stato db.StatoProposta
}

// PropostaRelazione e' la riga di relazione_proposta per una coppia (padre, figlio) del file.
type PropostaRelazione struct {
	Padre, Figlio string
	Qta           int32
	Evidenza      map[string]any
	Stato         db.StatoProposta
	Nota          string
}

// Piano sono le proposte di un file per una RFQ.
type Piano struct {
	Nodi      []PropostaNodo
	Relazioni []PropostaRelazione
}

// Aperte conta le righe che chiedono una decisione.
func (p Piano) Aperte() (nodi, relazioni int) {
	for _, n := range p.Nodi {
		if n.Stato == db.StatoPropostaAperta {
			nodi++
		}
	}
	for _, r := range p.Relazioni {
		if r.Stato == db.StatoPropostaAperta {
			relazioni++
		}
	}
	return nodi, relazioni
}

// Pianifica confronta il grafo del file con la working. Pura: stessi dati, stesse proposte.
//
// I nodi non dipendono dal contesto: tutti aperti, senza componente, con il tipo suggerito dal file
// (sottoassieme se nel file ha dei figli, altrimenti particolare; mai «finito»). Un arco si chiude da solo
// solo nell'autorita' (il padre e' una sorgente) e solo verso un figlio gia' deciso da una persona.
func Pianifica(nodi []NodoClassificato, s worker.StrutturaSTEP, c Contesto) Piano {
	haFigli := map[string]bool{}
	for _, r := range s.Relazioni {
		haFigli[r.Padre] = true
	}
	var p Piano
	for _, n := range nodi {
		pn := PropostaNodo{NodoClassificato: n, Stato: db.StatoPropostaAperta, Tipo: db.TipoComponenteSciolto}
		if haFigli[n.Chiave] {
			pn.Tipo = db.TipoComponenteSottoassieme
		}
		p.Nodi = append(p.Nodi, pn)
	}

	nodoDi := map[string]bool{}
	for _, n := range nodi {
		nodoDi[n.Chiave] = true
	}
	// il componente che un nodo E': la sorgente, o la decisione di una persona. Mai il codice.
	componente := func(k string) (uuid.UUID, bool) {
		if id, ok := c.Sorgenti[k]; ok {
			return id, true
		}
		id, ok := c.Decisi[k]
		return id, ok
	}
	for _, r := range s.Relazioni {
		if !nodoDi[r.Padre] || !nodoDi[r.Figlio] || r.Padre == r.Figlio || r.Qta <= 0 {
			continue // la FK di relazione_proposta vuole due nodi del file, e il CHECK una quantita'
		}
		pr := PropostaRelazione{Padre: r.Padre, Figlio: r.Figlio, Qta: int32(r.Qta), Stato: db.StatoPropostaAperta,
			Evidenza: map[string]any{}}
		for k, v := range r.Evidenza {
			pr.Evidenza[k] = v
		}
		pa, sorgente := c.Sorgenti[r.Padre]
		fi, deciso := componente(r.Figlio)
		if sorgente && deciso {
			if pa == fi {
				pr.Stato = db.StatoPropostaScartata
				pr.Nota = "padre e figlio sono lo stesso componente: un pezzo non contiene se stesso"
			} else if q, c0 := c.Archi[Arco{Padre: pa, Figlio: fi}]; c0 {
				pr.Evidenza["qta_working"] = q
				switch {
				case q == pr.Qta:
					// la tenuta dei conti: l'arco c'e' gia', con la stessa quantita', e il figlio l'ha deciso una
					// persona. Non e' un'identita' nuova
					pr.Stato = db.StatoPropostaDuplicato
				case c.Completa:
					pr.Nota = fmt.Sprintf("qta diversa: %d contro %d", pr.Qta, q)
				default:
					pr.Stato = db.StatoPropostaDuplicato
					pr.Nota = fmt.Sprintf("qta diversa (%d contro %d) in una lettura incompleta: non proposta", pr.Qta, q)
				}
			}
		}
		p.Relazioni = append(p.Relazioni, pr)
	}
	return p
}

// CreerebbeCiclo dice se aggiungere l'arco padre → figlio chiuderebbe un ciclo: succede se dal figlio
// si arriva gia' al padre. Restituisce quel cammino (figlio … padre), oppure nil. Un pezzo con due padri
// va bene (A1.1): e' un pezzo dentro se stesso che non esiste.
func CreerebbeCiclo(archi []Arco, padre, figlio uuid.UUID) []uuid.UUID {
	if padre == figlio {
		return []uuid.UUID{padre}
	}
	figli := map[uuid.UUID][]uuid.UUID{}
	for _, a := range archi {
		figli[a.Padre] = append(figli[a.Padre], a.Figlio)
	}
	da := map[uuid.UUID]uuid.UUID{}
	visto := map[uuid.UUID]bool{figlio: true}
	coda := []uuid.UUID{figlio}
	for len(coda) > 0 {
		n := coda[0]
		coda = coda[1:]
		if n == padre {
			var cammino []uuid.UUID
			for x := padre; ; x = da[x] {
				cammino = append([]uuid.UUID{x}, cammino...)
				if x == figlio {
					return cammino
				}
			}
		}
		for _, f := range figli[n] {
			if !visto[f] {
				visto[f] = true
				da[f] = n
				coda = append(coda, f)
			}
		}
	}
	return nil
}

// Rimozione e' un arco della working che un file autorizzato, letto per intero, non contiene piu'.
type Rimozione struct {
	Padre, Figlio uuid.UUID
	QtaWorking    int32
}

// RimozioniFigliDiretti confronta gli archi della working che partono da c con quelli che il suo file
// autorizzato propone a c (gia' tradotti in coppie di componenti). Pura. Profondita' 1 (Smistamento F5,
// A5.4.6, D77): il file di c ha autorita' sui figli diretti di c e basta. Prima (Rimozioni) si scendeva per
// tutto il sottoalbero, e il file del prodotto proponeva di togliere archi su cui l'autorita' e' dello STEP
// di un sottoassieme. Chi chiama garantisce che la lettura sia completa e che ogni figlio diretto sia deciso
// da una persona (A4.4, D27): una mancanza in una lettura parziale non e' un'informazione.
func RimozioniFigliDiretti(c uuid.UUID, archi map[Arco]int32, nelFile map[Arco]bool) []Rimozione {
	var out []Rimozione
	for a, q := range archi {
		if a.Padre == c && !nelFile[a] {
			out = append(out, Rimozione{Padre: a.Padre, Figlio: a.Figlio, QtaWorking: q})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Figlio.String() < out[j].Figlio.String() })
	return out
}

// RadiceDelloStep e' la radice del file quando e' una sola e il motore del cliente ne legge il codice (di
// famiglia o generico): un'EVIDENZA del codice del documento STEP (Smistamento F4, D49). Prima era la D16,
// che con una radice di famiglia correggeva la proposta del documento; adesso la radice sta accanto al nome
// del file nella valutazione del codice, e quando i due non sono d'accordo lo si dice (stato discorde) e la
// colonna tiene il nome. Con due radici non si sceglie per conto di nessuno; un codice che l'operatore ha
// scritto sul nodo e' una decisione sul nodo, non una lettura del file, e non entra.
func RadiceDelloStep(nodi []NodoClassificato, s worker.StrutturaSTEP) (classificazione.Radice, bool) {
	if len(s.Radici) != 1 {
		return classificazione.Radice{}, false
	}
	for _, n := range nodi {
		if n.Chiave != s.Radici[0] || n.Codice == "" || (n.Origine != "famiglia" && n.Origine != "generico") {
			continue
		}
		r := classificazione.Radice{Codice: n.Codice, Rev: n.Rev, DiFamiglia: n.Origine == "famiglia", Famiglia: n.Famiglia, Dove: n.Dove,
			Testo: map[string]string{DoveID: n.IDGrezzo, DoveNome: n.NomeGrezzo, DoveDescrizione: n.Descrizione}[n.Dove]}
		for _, g := range s.Nodi {
			if g.Chiave == n.Chiave && strings.TrimSpace(g.RevGrezza) != "" {
				r.RevDalFile = true
			}
		}
		return r, true
	}
	return classificazione.Radice{}, false
}
