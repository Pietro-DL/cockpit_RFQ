package estrazione

import (
	"encoding/json"
	"fmt"
	"math"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/platform/contratti/worker"
)

// La mappatura PDF (mappatura-pdf-1; 5.4.5), dai fatti decodificati con worker.DecodificaTestoPDF
// (tipi.go:767-781). Il worker dà i campi del probabile cartiglio della pagina 1 (etichetta → valore, con il
// riquadro), i frammenti di testo con pagina e riquadro, i metadati e l'esito dell'OCR; qui diventano entità e
// unità, senza decidere che cosa sia un codice o una revisione: lo decide il motore, con la grammatica del
// cliente.
//
// Lo stato del testo si ricava con la regola di testoDelPDF (classificazione/testo_pdf.go:74-88), replicata e non
// importata, perché non è esportata e G4 non la ammette: il testo c'è (estraibile o no) se DecodificaTestoPDF lo
// legge; altrimenti un errore_pdf non vuoto vuol dire PDF illeggibile; altrimenti il testo non è stato letto.
const (
	chiaveTestoPDF  = "testo_pdf"  // la parte dei fatti con il testo (RisultatoAnalisi.dettagli["testo_pdf"])
	chiaveErrorePDF = "errore_pdf" // il PDF non si apre

	capTesto     = "testo"
	capCartiglio = "cartiglio"
	capElencoPDF = "elenco_pdf"

	idEntitaPagina1 = "e:pdf:p1"

	tipoEntitaDisegno = "disegno"
	tipoEntitaIsolata = "unita_isolata"

	localizzazioneAssente = "assente"
	metodoNativo          = "nativo"
	metodoOCR             = "ocr"
	mappaturaSconosciuta  = "sconosciuta"

	motivoPDFParziale   = "riquadro del blocco, non del codice; testo già normalizzato dal worker: nessun offset nei byte del PDF"
	motivoPDFMetadato   = "metadato del documento: nessuna pagina né riquadro"
	motivoElencoPDF     = "il worker non legge le righe dell'elenco particolari: nessun fatto tabellare"
	motivoTestoNonLetto = "testo non letto: da rianalizzare"
)

// estensioniPDF: i file che il worker legge come PDF.
var estensioniPDF = map[string]bool{"pdf": true}

// chiaviMetadati: i metadati del worker (tipi.go:736-742), nell'ordine della struttura. Si leggono per nome, mai
// in ordine di mappa; l'autore non c'è, di proposito (è il nome di una persona).
var chiaviMetadati = []string{"titolo", "soggetto", "parole_chiave", "creatore", "produttore"}

// ePDF: un file è un PDF per l'adattatore se l'estensione è «pdf», o se i fatti hanno il testo o l'errore di un
// PDF.
func ePDF(est string, chiavi map[string]bool) bool {
	return estensioniPDF[est] || chiavi[chiaveTestoPDF] || chiavi[chiaveErrorePDF]
}

// contenutoPDF aggiunge al documento il contenuto di un PDF (5.4.5, «Mappatura PDF»).
//   - Sempre: la capacità elenco_pdf non disponibile, perché il worker non legge le righe dell'elenco.
//   - Nessun fatto: pdf.non_analizzato; il testo non è disponibile.
//   - Nessun testo_pdf che si legge: con errore_pdf, pdf.illeggibile e lo stato «errore»; senza, pdf.non_letto
//     («da rianalizzare»). Né l'uno né l'altro è un PDF «senza codici».
//   - Altrimenti: il testo letto (testoPDF).
func (c *documento) contenutoPDF(f *fotorfq.Fatti) {
	c.capacita(capElencoPDF, statoNonDisponibile, motivoElencoPDF)
	if f == nil {
		c.capacita(capTesto, statoNonDisponibile, "nessun fatto del worker: il PDF non è stato analizzato")
		c.peggiora(statoParziale)
		c.diagnostica(evidenze.Diagnostica{
			Codice:    CodicePDFNonAnalizzato,
			Gravita:   evidenze.GravitaNota,
			Natura:    evidenze.NaturaDati,
			Messaggio: "il PDF non ha fatti del worker: del file si legge solo il nome",
		})
		return
	}
	t, ok := worker.DecodificaTestoPDF(f.Payload)
	if !ok {
		if e := errorePDF(f.Payload); e != "" {
			c.capacita(capTesto, statoNonDisponibile, "il PDF non si apre: «"+e+"»")
			c.peggiora(statoErrore)
			c.diagnostica(evidenze.Diagnostica{
				Codice:    CodicePDFIlleggibile,
				Gravita:   evidenze.GravitaAvviso,
				Natura:    evidenze.NaturaDati,
				Percorso:  chiaveErrorePDF,
				Messaggio: "il worker non ha aperto il PDF: «" + e + "»",
			})
			return
		}
		c.capacita(capTesto, statoNonDisponibile, motivoTestoNonLetto)
		c.peggiora(statoParziale)
		c.diagnostica(evidenze.Diagnostica{
			Codice:    CodicePDFNonLetto,
			Gravita:   evidenze.GravitaAvviso,
			Natura:    evidenze.NaturaDati,
			Percorso:  chiaveTestoPDF,
			Messaggio: "i fatti non portano il testo del PDF (analizzatore di prima, o un worker non aggiornato): il testo non è stato letto, e il PDF va rianalizzato, non dichiarato muto",
		})
		return
	}
	c.testoPDF(t)
}

// errorePDF: il testo di errore_pdf, se c'è e non è vuoto, con la stessa lettura di testoDelPDF.
func errorePDF(payload json.RawMessage) string {
	var f struct {
		ErrorePdf string `json:"errore_pdf"`
	}
	if len(payload) > 0 && json.Unmarshal(payload, &f) == nil {
		return f.ErrorePdf
	}
	return ""
}

// testoPDF mappa un testo decodificato.
//   - Estraibile falso: nelle pagine lette non c'è testo nativo. Le capacità testo e cartiglio non sono
//     disponibili, con lo stato dell'OCR; pdf.senza_testo. Non è «nessun codice» (A-C09).
//   - Troncato: il worker si è fermato su un suo tetto; pdf.troncato, la fonte è parziale.
//   - Sottoversione di prima (< worker.VersioneTestoPDF, come testoDiPrima di testo_pdf.go:114-116): il
//     cartiglio di quel worker non è contenuto, è un indizio (giro 4, fase 4.6r). I suoi campi diventano unità
//     testo_pdf isolate, con l'etichetta conservata, la mappatura «sconosciuta» e pdf.testo_di_prima.
//   - I fatti che ci sono si mappano sempre (campi, frammenti, metadati): un fatto non si scarta.
func (c *documento) testoPDF(t *worker.TestoPDF) {
	c.d.Fonte.RiferimentoFatti.SottoversionePDF = t.Versione
	diPrima := t.Versione < worker.VersioneTestoPDF

	switch {
	case t.Estraibile:
		c.d.Qualita.Metodo = metodoNativo
	case t.OCR.Stato == worker.OCREseguito:
		c.d.Qualita.Metodo = metodoOCR
	}

	if !t.Estraibile {
		motivo := "nessun testo nativo nelle pagine lette (curve o scansione); OCR «" + t.OCR.Stato + "»"
		if t.OCR.Motivo != "" {
			motivo += ": " + t.OCR.Motivo
		}
		c.capacita(capTesto, statoNonDisponibile, motivo)
		c.capacita(capCartiglio, statoNonDisponibile, motivo)
		c.peggiora(statoParziale)
		c.diagnostica(evidenze.Diagnostica{
			Codice:    CodicePDFSenzaTesto,
			Gravita:   evidenze.GravitaAvviso,
			Natura:    evidenze.NaturaDati,
			Percorso:  chiaveTestoPDF + ".estraibile",
			Messaggio: "il PDF non ha testo nel file; OCR «" + t.OCR.Stato + "»: non è un PDF senza codici",
		})
	} else {
		testo, motivo := statoDisponibile, ""
		if t.Troncato {
			testo, motivo = statoParziale, "il worker si è fermato sui suoi tetti (pagine, frammenti, caratteri)"
		}
		c.capacita(capTesto, testo, motivo)
		if diPrima {
			c.capacita(capCartiglio, statoNonDisponibile, fmt.Sprintf("testo letto con il worker di prima (sottoversione %d): il cartiglio è un indizio, da rianalizzare", t.Versione))
		} else {
			c.capacita(capCartiglio, statoDisponibile, "")
		}
	}
	if t.Troncato {
		c.peggiora(statoParziale)
		c.diagnostica(evidenze.Diagnostica{
			Codice:    CodicePDFTroncato,
			Gravita:   evidenze.GravitaNota,
			Natura:    evidenze.NaturaDati,
			Percorso:  chiaveTestoPDF + ".troncato",
			Messaggio: fmt.Sprintf("il worker ha letto %d pagine su %d e si è fermato su un suo tetto: il testo dopo non c'è", t.PagineLette, t.Pagine),
		})
	}

	diPrimaIDs := c.cartiglioPDF(t.Cartiglio, diPrima)
	if diPrima {
		c.d.Qualita.Mappatura = mappaturaSconosciuta
		c.peggiora(statoParziale)
		c.diagnostica(evidenze.Diagnostica{
			Codice:    CodicePDFTestoDiPrima,
			Gravita:   evidenze.GravitaAvviso,
			Natura:    evidenze.NaturaDati,
			Percorso:  chiaveTestoPDF + ".versione",
			Messaggio: fmt.Sprintf("testo letto con il worker di prima (sottoversione %d, oggi %d): i campi del cartiglio sono testo_pdf, un indizio e mai il codice del file, finché il PDF non si rianalizza", t.Versione, worker.VersioneTestoPDF),
			Rif:       diPrimaIDs,
		})
	}
	for i, fr := range t.Frammenti {
		c.frammentoPDF(i+1, fr)
	}
	c.metadatiPDF(t.Metadati)
}

// cartiglioPDF aggiunge i campi del cartiglio (CampoCartiglio, tipi.go:703-712). n conta i campi con la stessa
// etichetta, nell'ordine dei fatti, anche quelli vuoti, così l'ID di un campo non dipende dagli altri valori.
// Un campo con il valore vuoto non dà unità.
//   - Sottoversione di oggi: un'entità «disegno» della pagina 1, e un'unità per campo legata con EntitaID,
//     selettore cartiglio.<etichetta> (cartiglio.sconosciuto per un'etichetta che la foglia non ha), l'etichetta
//     letta in CampoOriginale.Etichetta.
//   - Sottoversione di prima: ogni campo è un'unità testo_pdf con un'entità «unita_isolata» sua: nessun legame
//     con il disegno, perché quel cartiglio non è contenuto. Restituisce i loro ID.
func (c *documento) cartiglioPDF(campi []worker.CampoCartiglio, diPrima bool) []string {
	_, ammessi := evidenze.CampiAmmessi(evidenze.ContestoCartiglio)
	noti := map[string]bool{}
	for _, v := range ammessi {
		noti[v] = true
	}
	perEtichetta := map[string]int{}
	disegno := false
	var diPrimaIDs []string
	for _, cc := range campi {
		perEtichetta[cc.Etichetta]++
		if cc.Valore == "" {
			continue
		}
		chiave := fmt.Sprintf("cartiglio:%s:%d", cc.Etichetta, perEtichetta[cc.Etichetta])
		pos := posPDF(cc.Pagina, cc.Riquadro, cc.Zona, cc.Fonte)
		u := evidenze.UnitaEvidenza{
			ID:             "u:pdf:" + chiave,
			Testo:          cc.Valore,
			CampoOriginale: evidenze.CampoOriginale{Etichetta: cc.Letta, Parser: chiaveTestoPDF + ".cartiglio." + cc.Etichetta, Mappatura: mappaturaPDF},
			Posizione:      pos,
			Qualita:        evidenze.QualitaUnita{Localizzazione: localizzazioneParziale, Motivo: motivoPDFParziale, Metodo: metodoDellaFonte(cc.Fonte)},
		}
		if diPrima {
			idEntita := "e:pdf:" + chiave
			c.entita(evidenze.EntitaLocale{ID: idEntita, Tipo: tipoEntitaIsolata, ChiaveOriginale: "campo " + cc.Letta, Posizione: pos})
			u.EntitaID = idEntita
			u.Selettore = selettoreDi(evidenze.ContestoTestoPDF)
			c.unita(u)
			diPrimaIDs = append(diPrimaIDs, u.ID)
			continue
		}
		if !disegno {
			disegno = true
			c.entita(evidenze.EntitaLocale{ID: idEntitaPagina1, Tipo: tipoEntitaDisegno, ChiaveOriginale: "pagina 1",
				Posizione: evidenze.Localizzatore{Tipo: "pdf", PDF: &evidenze.PosPDF{Pagina: 1}}})
		}
		campo := cc.Etichetta
		if !noti[campo] {
			campo = "sconosciuto"
		}
		u.EntitaID = idEntitaPagina1
		u.Selettore = evidenze.Selettore{Contesto: evidenze.ContestoCartiglio, Campo: evidenze.CampoFonte{Variante: evidenze.VarianteCartiglio, Valore: campo}}
		c.unita(u)
	}
	return diPrimaIDs
}

// frammentoPDF aggiunge un frammento (FrammentoPDF, tipi.go:690-697): un'unità testo_pdf con un'entità
// «unita_isolata» sua. Un frammento non si lega al disegno né a un altro frammento: la «Rev» di un frammento non
// è la revisione del cartiglio (A-C02). n è la posizione nei fatti, contando anche i frammenti vuoti.
func (c *documento) frammentoPDF(n int, fr worker.FrammentoPDF) {
	if fr.Testo == "" {
		return
	}
	idEntita := fmt.Sprintf("e:pdf:frammento:%d", n)
	pos := posPDF(fr.Pagina, fr.Riquadro, fr.Zona, fr.Fonte)
	c.entita(evidenze.EntitaLocale{ID: idEntita, Tipo: tipoEntitaIsolata, ChiaveOriginale: fmt.Sprintf("frammento %d", n), Posizione: pos})
	c.unita(evidenze.UnitaEvidenza{
		ID:             fmt.Sprintf("u:pdf:frammento:%d", n),
		EntitaID:       idEntita,
		Selettore:      selettoreDi(evidenze.ContestoTestoPDF),
		Testo:          fr.Testo,
		CampoOriginale: evidenze.CampoOriginale{Parser: chiaveTestoPDF + ".frammenti", Mappatura: mappaturaPDF},
		Posizione:      pos,
		Qualita:        evidenze.QualitaUnita{Localizzazione: localizzazioneParziale, Motivo: motivoPDFParziale, Metodo: metodoDellaFonte(fr.Fonte)},
	})
}

// metadatiPDF aggiunge un'unità metadati_pdf per ogni metadato non vuoto, con un'entità «unita_isolata» sua e la
// chiave in CampoOriginale. Un metadato non ha pagina né riquadro: la localizzazione è «assente».
func (c *documento) metadatiPDF(m worker.MetadatiPDF) {
	valori := map[string]string{"titolo": m.Titolo, "soggetto": m.Soggetto, "parole_chiave": m.ParoleChiave, "creatore": m.Creatore, "produttore": m.Produttore}
	for _, k := range chiaviMetadati {
		v := valori[k]
		if v == "" {
			continue
		}
		idEntita := "e:pdf:metadati:" + k
		pos := evidenze.Localizzatore{Tipo: "pdf", PDF: &evidenze.PosPDF{}}
		c.entita(evidenze.EntitaLocale{ID: idEntita, Tipo: tipoEntitaIsolata, ChiaveOriginale: "metadato " + k, Posizione: pos})
		c.unita(evidenze.UnitaEvidenza{
			ID:             "u:pdf:metadati:" + k,
			EntitaID:       idEntita,
			Selettore:      selettoreDi(evidenze.ContestoMetadatiPDF),
			Testo:          v,
			CampoOriginale: evidenze.CampoOriginale{Etichetta: k, Parser: chiaveTestoPDF + ".metadati." + k, Mappatura: mappaturaPDF},
			Posizione:      pos,
			Qualita:        evidenze.QualitaUnita{Localizzazione: localizzazioneAssente, Motivo: motivoPDFMetadato, Metodo: metodoNativo},
		})
	}
}

// posPDF: il localizzatore di un campo o di un frammento, come i fatti lo danno. Zona e fonte passano come sono:
// un valore che la foglia non conosce è un cambio del contratto del worker, e ValidaDocumento lo rifiuta.
func posPDF(pagina int, riquadro []float64, zona, fonte string) evidenze.Localizzatore {
	return evidenze.Localizzatore{Tipo: "pdf", PDF: &evidenze.PosPDF{
		Pagina:         pagina,
		RiquadroDecimi: riquadroInDecimi(riquadro),
		Zona:           zona,
		Fonte:          fonte,
	}}
}

// riquadroInDecimi: il riquadro del worker ([x0, y0, x1, y1] in punti, arrotondati a 0,1 pt) in decimi di punto
// interi, con math.Round(x*10): il JSON canonico non ha numeri decimali (5.4.5, «Regole comuni»). Un riquadro che
// non ha quattro numeri finiti non si scrive, e non si inventa.
func riquadroInDecimi(r []float64) *[4]int {
	if len(r) != 4 {
		return nil
	}
	var out [4]int
	for i, x := range r {
		if math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x*10) > math.MaxInt32 {
			return nil
		}
		out[i] = int(math.Round(x * 10))
	}
	return &out
}

// metodoDellaFonte: il metodo di lettura di un'unità dalla fonte del testo (nativo | ocr). Una fonte che il
// contratto non ha non dà un metodo.
func metodoDellaFonte(fonte string) string {
	switch fonte {
	case worker.FonteTestoNativo:
		return metodoNativo
	case worker.FonteTestoOCR:
		return metodoOCR
	}
	return ""
}
