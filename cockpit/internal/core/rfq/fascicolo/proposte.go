package fascicolo

// Le proposte di struttura da uno STEP, come regola pura (Blocco 8, B8.5; addendum A1.1, A4.4).
//
// Il confronto e' fra il grafo del file, gia' classificato, e la BOM working della RFQ. Ogni nodo e ogni
// arco del file diventa una riga di proposta, e la riga dice se e' una VARIAZIONE (aperta: qualcuno deve
// decidere) o se la working la contiene gia' (duplicato: riconciliata, niente da decidere):
//
//	nodo nuovo                        nessun componente con quel codice          → aperta
//	nodo gia' nella BOM               un componente attivo con quel codice       → duplicato, agganciato
//	nodo di un componente archiviato  accettarlo lo ripristina                   → aperta, con la nota
//	arco nuovo, secondo padre          l'arco non c'e' nella working              → aperta
//	arco uguale                       stessi componenti, stessa quantita'        → duplicato
//	quantita' diversa                 solo da una lettura COMPLETA               → aperta, «qta diversa: 2 contro 1»
//	quantita' diversa, lettura parziale una lettura troncata puo' aver contato meno → duplicato, con la nota
//
// Le rimozioni non sono qui: nascono solo dallo STEP strutturale letto per intero (Rimozioni, sotto).
// Niente di questo file tocca componente o componente_relazione.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// Working e' la BOM working della RFQ come la vede il confronto.
type Working struct {
	PerCodice map[string]db.Componente // upper(codice) → componente, archiviati compresi (il codice resta loro)
	Archi     map[Arco]int32           // archi fra componenti attivi → quantita'
}

// NuovaWorking costruisce la Working da componenti e archi letti.
func NuovaWorking(comp []db.Componente, rel []db.ComponenteRelazione) Working {
	w := Working{PerCodice: map[string]db.Componente{}, Archi: map[Arco]int32{}}
	for _, c := range comp {
		w.PerCodice[strings.ToUpper(strings.TrimSpace(c.Codice))] = c
	}
	for _, r := range rel {
		w.Archi[Arco{Padre: r.PadreID, Figlio: r.FiglioID}] = r.Qta
	}
	return w
}

// conCanonici aggiunge, per un cliente con suffissi decorativi, ogni componente anche sotto il suo codice senza
// suffisso: un pezzo accettato come «X_PRT» prima che la regola ci fosse e' lo stesso pezzo che lo STEP, letto
// adesso con la regola, chiama «X». Il codice del componente non cambia; il confronto lo ritrova, e il nodo
// diventa un duplicato invece di un secondo componente.
func (w Working) conCanonici(m *classificazione.Motore) {
	if !m.HaSuffissi() {
		return
	}
	var tutti []db.Componente
	for _, c := range w.PerCodice {
		tutti = append(tutti, c)
	}
	sort.Slice(tutti, func(i, j int) bool { return tutti[i].Codice < tutti[j].Codice })
	for _, c := range tutti {
		can, _ := m.Canonico(c.Codice, "")
		if k := strings.ToUpper(strings.TrimSpace(can)); k != "" {
			if _, gia := w.PerCodice[k]; !gia {
				w.PerCodice[k] = c
			}
		}
	}
}

// Contesto e' quello che serve a leggere un file dentro una RFQ, oltre al file.
type Contesto struct {
	Working
	// Identificativi: i codici del prodotto finito della richiesta (identificativo_thread), in upper.
	// Una radice del file con uno di questi codici si propone «finito».
	Identificativi map[string]bool
	// Decisi: i nodi di questo file gia' decisi, chiave → componente. Non si ripropongono.
	Decisi map[string]uuid.UUID
	// Radice: il file e' lo STEP strutturale di questo prodotto finito. Sceglierlo e' una dichiarazione
	// («questo file e' la distinta di X», A4.4): la sua unica radice e' X anche se il nome non si
	// classifica come il codice di X.
	Radice *db.Componente
	// Completa: la lettura e' completa (struttura_motivo_parziale vuoto). Solo allora una quantita'
	// diversa e' una proposta.
	Completa bool
}

// PropostaNodo e' la riga di componente_proposta per un nodo del file.
type PropostaNodo struct {
	NodoClassificato
	Tipo         db.TipoComponente
	Stato        db.StatoProposta
	ComponenteID uuid.NullUUID
	Nota         string
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
func Pianifica(nodi []NodoClassificato, s worker.StrutturaSTEP, c Contesto) Piano {
	radici := map[string]bool{}
	for _, r := range s.Radici {
		radici[r] = true
	}
	haFigli := map[string]bool{}
	for _, r := range s.Relazioni {
		haFigli[r.Padre] = true
	}
	dichiarata := ""
	if c.Radice != nil && len(s.Radici) == 1 {
		dichiarata = s.Radici[0]
	}

	var p Piano
	attivo := map[string]uuid.UUID{} // chiave → componente attivo a cui il nodo corrisponde
	for _, n := range nodi {
		pn := PropostaNodo{NodoClassificato: n, Stato: db.StatoPropostaAperta}
		switch {
		case radici[n.Chiave] && (dichiarata == n.Chiave || c.Identificativi[strings.ToUpper(n.Codice)]):
			pn.Tipo = db.TipoComponenteFinito
		case haFigli[n.Chiave]:
			pn.Tipo = db.TipoComponenteSottoassieme
		default:
			pn.Tipo = db.TipoComponenteSciolto
		}
		comp, trovato := c.componenteDi(n, dichiarata == n.Chiave)
		switch {
		case !trovato:
		case comp.ArchiviatoIl != nil:
			pn.Nota = fmt.Sprintf("il componente %s è archiviato: accettare la proposta lo ripristina", comp.Codice)
		default:
			pn.Stato = db.StatoPropostaDuplicato
			pn.ComponenteID = uuid.NullUUID{UUID: comp.ComponenteID, Valid: true}
			attivo[n.Chiave] = comp.ComponenteID
			if dichiarata == n.Chiave && n.Codice != "" && !strings.EqualFold(n.Codice, comp.Codice) {
				pn.Nota = fmt.Sprintf("radice dello STEP strutturale di %s: il file la chiama %s", comp.Codice, n.Codice)
			}
		}
		p.Nodi = append(p.Nodi, pn)
	}

	nodoDi := map[string]bool{}
	for _, n := range nodi {
		nodoDi[n.Chiave] = true
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
		pa, okP := attivo[r.Padre]
		fi, okF := attivo[r.Figlio]
		if okP && okF {
			if pa == fi {
				pr.Stato = db.StatoPropostaScartata
				pr.Nota = "padre e figlio sono lo stesso componente: un pezzo non contiene se stesso"
			} else if q, c0 := c.Archi[Arco{Padre: pa, Figlio: fi}]; c0 {
				pr.Evidenza["qta_working"] = q
				switch {
				case q == pr.Qta:
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

// componenteDi e' il componente a cui corrisponde un nodo: quello gia' deciso, oppure la radice
// dichiarata, oppure quello con lo stesso codice (upper). Archiviati compresi: chi chiama decide.
func (c Contesto) componenteDi(n NodoClassificato, radiceDichiarata bool) (db.Componente, bool) {
	if id, ok := c.Decisi[n.Chiave]; ok {
		for _, x := range c.PerCodice {
			if x.ComponenteID == id {
				return x, true
			}
		}
	}
	if radiceDichiarata {
		return *c.Radice, true
	}
	if n.Codice == "" {
		return db.Componente{}, false
	}
	x, ok := c.PerCodice[strings.ToUpper(strings.TrimSpace(n.Codice))]
	return x, ok
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

// Rimozione e' un arco della working che lo STEP strutturale, letto per intero, non contiene piu'.
type Rimozione struct {
	Padre, Figlio uuid.UUID
	QtaWorking    int32
}

// Rimozioni confronta gli archi della working raggiungibili dal prodotto con quelli del suo STEP
// strutturale (gia' tradotti in coppie di componenti). Pura. Chi chiama garantisce che la lettura sia
// completa e che ogni nodo del file abbia un componente o sia davvero nuovo (A4.4, D27): una mancanza in
// una lettura parziale non e' un'informazione, e non diventa mai una proposta di rimozione.
func Rimozioni(prodotto uuid.UUID, archi map[Arco]int32, nelFile map[Arco]bool) []Rimozione {
	figli := map[uuid.UUID][]uuid.UUID{}
	for a := range archi {
		figli[a.Padre] = append(figli[a.Padre], a.Figlio)
	}
	visto := map[uuid.UUID]bool{prodotto: true}
	coda := []uuid.UUID{prodotto}
	var out []Rimozione
	for len(coda) > 0 {
		n := coda[0]
		coda = coda[1:]
		for _, f := range figli[n] {
			a := Arco{Padre: n, Figlio: f}
			if !nelFile[a] {
				out = append(out, Rimozione{Padre: n, Figlio: f, QtaWorking: archi[a]})
			}
			if !visto[f] {
				visto[f] = true
				coda = append(coda, f)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Padre != out[j].Padre {
			return out[i].Padre.String() < out[j].Padre.String()
		}
		return out[i].Figlio.String() < out[j].Figlio.String()
	})
	return out
}

// RadiceDiFamiglia e' la radice del file quando e' una sola e una famiglia del cliente ne riconosce il
// codice (D16): allora corregge anche la proposta del documento, perche' e' la stessa regex del cliente
// applicata al PRODUCT invece che al nome del file.
func RadiceDiFamiglia(nodi []NodoClassificato, s worker.StrutturaSTEP) (NodoClassificato, bool) {
	if len(s.Radici) != 1 {
		return NodoClassificato{}, false
	}
	for _, n := range nodi {
		if n.Chiave == s.Radici[0] && n.Origine == "famiglia" && n.Codice != "" {
			return n, true
		}
	}
	return NodoClassificato{}, false
}
