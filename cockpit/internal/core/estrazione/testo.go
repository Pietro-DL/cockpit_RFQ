package estrazione

import (
	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// Gli ID e i testi del documento di DaTesto.
const (
	idTestoIsolato    = "testo"      // il testo originale di un testo isolato, e la sua origine
	idUnitaTesto      = "u:testo"    // l'unica unità
	idEntitaTesto     = "e:testo"    // l'entità a cui si lega, per i selettori del cartiglio e dello STEP
	idSegCorrente     = "s:corrente" // il segmento della parte corrente (oggetto e corpo)
	idPrimaStoria     = "s:storia:1" // il primo livello della storia (5.4.5: numerato anche il primo)
	messaggioCorrente = "m0"
	messaggioStoria   = "m1"
	origineIgnota     = "ignota"
)

// DaTesto costruisce un documento con una sola unità, per gli esempi e per i casi degli attesi (il banco,
// A1b.11; 5.4.5). La fonte è «testo»: senza origine nel DB, con il riferimento ai fatti «nessuno», e un
// documento così non si salva mai (5.4.1).
//
// L'unità ha il selettore dato, il testo com'è (nessuna normalizzazione) e la localizzazione esatta su tutto il
// testo. Intorno all'unità nasce solo ciò che il selettore richiede, perché il motore lega gli attributi
// all'entità e l'uso dei segmenti al segmento:
//   - oggetto e corpo: il segmento «corrente» s:corrente (messaggio logico m0); storia: il segmento s:storia:1
//     (m1), di tipo «citazione», perché senza la riga che apre il livello non c'è niente che dica «inoltro»
//     (R27 c: il tipo viene dalla riga d'apertura). L'origine dei due è «ignota»: un testo isolato non ha
//     mittente;
//   - cartiglio: l'entità «disegno»; radice_step e nodo_step: l'entità «nodo_step». L'unità vi si lega con
//     EntitaID;
//   - nome_file e voce_archivio: il localizzatore del nome, con stem ed estensione della regola dichiarata
//     (dividiNome), sul testo che ha per ID il campo del nome (la foglia lo chiede per PosNomeFile);
//   - testo_pdf, metadati_pdf ed elenco_pdf: la sola unità.
//
// Gli ID sono fissi («u:testo», «e:testo», «s:corrente», «s:storia:1»): il documento ha una sola unità, e il
// suo BundleID cambia con il selettore e con il testo. Un selettore non valido è un errore, e così un testo
// che non è UTF-8: nessun ripiego.
func DaTesto(sel evidenze.Selettore, testo string) (evidenze.DocumentoEvidenze, error) {
	if err := sel.Valida(); err != nil {
		return evidenze.DocumentoEvidenze{}, err
	}
	c := nuovoDocumento(fonteDiTesto())
	c.d.Qualita.Metodo = metodoAdattatore

	sulTesto := evidenze.Localizzatore{Tipo: "testo", Testo: &evidenze.PosTesto{TestoID: idTestoIsolato, Intervallo: intero(testo)}}
	u := evidenze.UnitaEvidenza{
		ID:        idUnitaTesto,
		Selettore: sel,
		Testo:     testo,
		Posizione: sulTesto,
		Qualita:   evidenze.QualitaUnita{Localizzazione: localizzazioneEsatta, Metodo: metodoAdattatore},
	}

	switch sel.Contesto {
	case evidenze.ContestoOggetto, evidenze.ContestoCorpo:
		c.testo(idTestoIsolato, testo, idTestoIsolato)
		c.segmento(evidenze.Segmento{ID: idSegCorrente, Tipo: evidenze.SegmentoCorrente, MessaggioLogicoID: messaggioCorrente,
			Origine: origineIgnota, Posizione: sulTesto})
		u.SegmentoID = idSegCorrente
	case evidenze.ContestoStoria:
		c.testo(idTestoIsolato, testo, idTestoIsolato)
		c.segmento(evidenze.Segmento{ID: idPrimaStoria, Tipo: evidenze.SegmentoCitazione, MessaggioLogicoID: messaggioStoria,
			Origine: origineIgnota, Posizione: sulTesto})
		u.SegmentoID = idPrimaStoria
	case evidenze.ContestoCartiglio, evidenze.ContestoRadiceSTEP, evidenze.ContestoNodoSTEP:
		c.testo(idTestoIsolato, testo, idTestoIsolato)
		tipo := "disegno"
		if sel.Contesto != evidenze.ContestoCartiglio {
			tipo = "nodo_step"
		}
		c.entita(evidenze.EntitaLocale{ID: idEntitaTesto, Tipo: tipo, ChiaveOriginale: idTestoIsolato, Posizione: sulTesto})
		u.EntitaID = idEntitaTesto
	case evidenze.ContestoNomeFile, evidenze.ContestoVoceArchivio:
		campo := campoNomeFile
		if sel.Contesto == evidenze.ContestoVoceArchivio {
			campo = campoPercorso
		}
		c.testo(campo, testo, idTestoIsolato)
		u.Posizione = posNome(campo, testo, "")
	default:
		c.testo(idTestoIsolato, testo, idTestoIsolato)
	}
	c.unita(u)
	return c.chiudi()
}
