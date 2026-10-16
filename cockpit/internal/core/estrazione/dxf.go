package estrazione

import (
	"fmt"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/platform/contratti/worker"
)

// La mappatura DXF (mappatura-dxf-1; K4), dai fatti decodificati con worker.DecodificaTestoDXF. Il worker
// (workers/dxf_testo.py) dà soltanto i campi del cartiglio che riconosce, etichetta → valore, con il riquadro del
// valore in unità del disegno: mai gli altri testi del disegno (ci sono nomi di persona). Qui diventano un'entità
// «disegno» e le sue unità cartiglio.<etichetta>, come il cartiglio di un PDF, senza decidere che cosa sia un
// codice: lo decide il motore, con la grammatica del cliente.
//
// La mappatura la guida la chiave dei fatti, non l'estensione: un DXF analizzato prima del K4 ha il solo esito
// del worker e resta il documento di prima, con il solo nome (il golden dxf_02 lo fissa).
//
// La sola capacità che il DXF dichiara è il cartiglio. Il testo del disegno non è una capacità del DXF, come per
// lo STEP: il worker non lo riporta di proposito, e dichiararlo «non disponibile» renderebbe parziale ogni DXF
// letto.
const (
	chiaveTestoDXF = "testo_dxf" // la parte dei fatti con il testo (RisultatoAnalisi.dettagli["testo_dxf"])

	idEntitaCartiglioDXF = "e:dxf:cartiglio"

	motivoDXFParziale = "riquadro del valore in unità del disegno (y verso l'alto), stimato dal worker dall'inserimento, dall'altezza e dall'attacco; testo già ripulito dai codici di formato: nessun offset nei byte del DXF"
)

// eDXF: un file è un DXF per l'adattatore se i fatti hanno il testo di un DXF. L'estensione da sola non basta:
// senza testo_dxf il documento resta quello di prima del K4.
func eDXF(chiavi map[string]bool) bool {
	return chiavi[chiaveTestoDXF]
}

// contenutoDXF aggiunge al documento il contenuto di un DXF (K4).
//   - testo_dxf che non si decodifica (una versione sconosciuta): dxf.non_letto, cartiglio non disponibile.
//   - Esito diverso da «letto» (un DXF binario, un file che non è un DXF ASCII, una lettura fallita):
//     dxf.non_letto con il motivo del worker. Non è un DXF «senza codici».
//   - Altrimenti: il cartiglio letto (testoDXF).
func (c *documento) contenutoDXF(f *fotorfq.Fatti) {
	t, ok := worker.DecodificaTestoDXF(f.Payload)
	if !ok {
		c.dxfNonLetto("testo_dxf di una versione che questo adattatore non legge: da rianalizzare",
			"i fatti hanno un testo_dxf che non si decodifica (versione sconosciuta): il DXF va rianalizzato, non dichiarato muto")
		return
	}
	if t.Esito != worker.DXFLetto {
		motivo := "il worker non ha letto il DXF (esito «" + t.Esito + "»"
		if t.Motivo != "" {
			motivo += ": " + t.Motivo
		}
		motivo += ")"
		c.dxfNonLetto(motivo, motivo+": non è un DXF senza codici")
		return
	}
	c.testoDXF(t)
}

// dxfNonLetto: la capacità cartiglio non disponibile con il motivo, la fonte parziale e dxf.non_letto.
func (c *documento) dxfNonLetto(motivo, messaggio string) {
	c.capacita(capCartiglio, statoNonDisponibile, motivo)
	c.peggiora(statoParziale)
	c.diagnostica(evidenze.Diagnostica{
		Codice:    CodiceDXFNonLetto,
		Gravita:   evidenze.GravitaAvviso,
		Natura:    evidenze.NaturaDati,
		Percorso:  chiaveTestoDXF,
		Messaggio: messaggio,
	})
}

// testoDXF mappa un testo letto.
//   - Troncato, o Parziale (v2: INSERT oltre la profondità, cicli, blocchi specchiati): il cartiglio è parziale,
//     dxf.troncato, la fonte è parziale.
//   - Gli INSERT annidati dentro un blocco, che il worker conta e non segue, non peggiorano niente da soli: sui
//     DXF veri ci sono anche quando il cartiglio è letto per intero (gruppi di geometria). Si dicono nei messaggi
//     di dxf.troncato e di dxf.senza_cartiglio, dove possono spiegare un campo che manca. Lo stesso per il Motivo di
//     un testo letto (gli INSERT ruotati che il worker non segue).
//   - Nessun campo riconosciuto: dxf.senza_cartiglio. Non è «nessun codice».
//   - I campi che ci sono si mappano sempre (cartiglioDXF).
func (c *documento) testoDXF(t *worker.TestoDXF) {
	c.d.Qualita.Metodo = metodoNativo
	annidati := ""
	if t.Annidati > 0 {
		annidati = fmt.Sprintf("; %d INSERT annidati dentro un blocco non si seguono", t.Annidati)
	}
	if t.Motivo != "" {
		annidati += "; " + t.Motivo
	}
	switch {
	case t.Troncato || t.Parziale:
		// GP-3: dalla v2 il worker segue gli INSERT annidati e dice `parziale` anche senza un tetto (INSERT oltre la
		// profondità massima, cicli, blocchi specchiati), con i motivi in Motivo. Un fatto v1 non ha Parziale: per lui
		// il ramo è quello di prima, byte per byte.
		motivo := "il worker si è fermato su un suo tetto (byte, testi, campi)" + annidati
		if !t.Troncato {
			motivo = "lettura parziale del worker" + annidati
		}
		c.capacita(capCartiglio, statoParziale, motivo)
		c.peggiora(statoParziale)
		c.diagnostica(evidenze.Diagnostica{
			Codice:    CodiceDXFTroncato,
			Gravita:   evidenze.GravitaNota,
			Natura:    evidenze.NaturaDati,
			Percorso:  chiaveTestoDXF + ".troncato",
			Messaggio: motivo + ": i testi dopo non ci sono",
		})
	default:
		c.capacita(capCartiglio, statoDisponibile, "")
	}
	if n := c.cartiglioDXF(t.Cartiglio); n == 0 {
		c.diagnostica(evidenze.Diagnostica{
			Codice:    CodiceDXFSenzaCartiglio,
			Gravita:   evidenze.GravitaNota,
			Natura:    evidenze.NaturaDati,
			Percorso:  chiaveTestoDXF + ".cartiglio",
			Messaggio: fmt.Sprintf("il DXF si è letto (%d testi%s), ma nessun campo del cartiglio è riconosciuto: non è un DXF senza codici", t.Testi, annidati),
		})
	}
}

// cartiglioDXF aggiunge i campi del cartiglio, come cartiglioPDF per la sottoversione di oggi: un'entità «disegno»
// del DXF e un'unità per campo legata con EntitaID, selettore cartiglio.<etichetta> (cartiglio.sconosciuto per
// un'etichetta che la foglia non ha), l'etichetta letta in CampoOriginale.Etichetta. n conta i campi con la stessa
// etichetta, nell'ordine dei fatti, anche quelli vuoti, così l'ID di un campo non dipende dagli altri valori. Un
// campo con il valore vuoto non dà unità. Pagina, zona e fonte dei fatti non si usano: un DXF non ha pagine, e il
// suo testo è sempre nativo. Restituisce quante unità ha aggiunto.
func (c *documento) cartiglioDXF(campi []worker.CampoCartiglio) int {
	_, ammessi := evidenze.CampiAmmessi(evidenze.ContestoCartiglio)
	noti := map[string]bool{}
	for _, v := range ammessi {
		noti[v] = true
	}
	perEtichetta := map[string]int{}
	n := 0
	for _, cc := range campi {
		perEtichetta[cc.Etichetta]++
		if cc.Valore == "" {
			continue
		}
		if n == 0 {
			c.entita(evidenze.EntitaLocale{ID: idEntitaCartiglioDXF, Tipo: tipoEntitaDisegno, ChiaveOriginale: "cartiglio del DXF",
				Posizione: evidenze.Localizzatore{Tipo: "dxf", DXF: &evidenze.PosDXF{}}})
		}
		n++
		campo := cc.Etichetta
		if !noti[campo] {
			campo = "sconosciuto"
		}
		c.unita(evidenze.UnitaEvidenza{
			ID:             fmt.Sprintf("u:dxf:cartiglio:%s:%d", cc.Etichetta, perEtichetta[cc.Etichetta]),
			EntitaID:       idEntitaCartiglioDXF,
			Selettore:      evidenze.Selettore{Contesto: evidenze.ContestoCartiglio, Campo: evidenze.CampoFonte{Variante: evidenze.VarianteCartiglio, Valore: campo}},
			Testo:          cc.Valore,
			CampoOriginale: evidenze.CampoOriginale{Etichetta: cc.Letta, Parser: chiaveTestoDXF + ".cartiglio." + cc.Etichetta, Mappatura: mappaturaDXF},
			Posizione:      evidenze.Localizzatore{Tipo: "dxf", DXF: &evidenze.PosDXF{RiquadroDecimi: riquadroInDecimi(cc.Riquadro)}},
			Qualita:        evidenze.QualitaUnita{Localizzazione: localizzazioneParziale, Motivo: motivoDXFParziale, Metodo: metodoNativo},
		})
	}
	return n
}
