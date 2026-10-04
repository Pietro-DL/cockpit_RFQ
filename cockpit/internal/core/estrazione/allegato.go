package estrazione

import (
	"encoding/json"
	"fmt"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
)

const (
	// naturaFile: la sola natura di allegato che ha un contenuto da leggere (enum natura_allegato: file, inline,
	// elemento_outlook, collegamento). "" vuol dire «non esportata», e non toglie niente: decidono i fatti.
	naturaFile = "file"

	// capContenuto: la capacità di un documento che porta il solo nome. Il motivo dice perché il contenuto
	// manca; un file senza contenuto letto non è mai un file «senza codici» (A-C09).
	capContenuto = "contenuto"
)

// DaAllegato costruisce il documento di un file (5.4.5): il nome e, per una voce d'archivio, il percorso, più
// il contenuto secondo i fatti. In questa versione nessuna mappatura legge ancora il contenuto (STEP e PDF
// arrivano con le loro mappature): ogni file dà il solo nome, con la capacità del contenuto dichiarata e il
// motivo. esito.codice, esito.rev e product_step del worker non si usano: sono letture del worker, non fatti
// (A1.2; tipi.go:486-500).
//
// contenitore è il record dello zip da cui la voce viene, se il chiamante l'ha: deve essere quello del record
// (fonteDiAllegato). Il suo nome non entra fra le unità della voce.
//
// È un errore, mai un documento: un record incoerente (percorso senza contenitore, contenitore diverso), dei
// fatti di un altro contenuto o con un'altra impronta, un payload che non è un oggetto JSON, un documento che
// non passa ValidaDocumento.
func DaAllegato(a fotorfq.Allegato, f *fotorfq.Fatti, contenitore *fotorfq.Allegato) (evidenze.DocumentoEvidenze, error) {
	fonte, err := fonteDiAllegato(a, f, contenitore)
	if err != nil {
		return evidenze.DocumentoEvidenze{}, err
	}
	var chiavi map[string]bool
	if f != nil {
		if chiavi, err = chiaviDeiFatti(f.Payload); err != nil {
			return evidenze.DocumentoEvidenze{}, fmt.Errorf("estrazione: allegato %s: %w", a.ID, err)
		}
	}

	c := nuovoDocumento(fonte)
	c.d.Qualita.Metodo = metodoAdattatore
	c.d.Qualita.Mappatura = mappaturaVerificata
	est := c.nomeDelFile(a)

	switch {
	case a.Natura != "" && a.Natura != naturaFile:
		c.senzaContenuto(fmt.Sprintf("natura «%s»: solo un file ha un contenuto da leggere (un elemento Outlook non si espande)", a.Natura))
	case estensioniNonEstratte[est]:
		c.senzaContenuto(fmt.Sprintf("archivio .%s: l'acquisizione non lo apre, e le sue voci non sono allegati", est))
	case f == nil:
		c.senzaContenuto("nessun fatto del worker per questo contenuto")
	case len(chiavi) == 1 && chiavi[chiaveEsito]:
		c.senzaContenuto("i fatti hanno solo l'esito del worker: è una sua lettura, non un fatto, e non si usa (A1.2)")
	default:
		c.senzaContenuto("i fatti non hanno una parte che gli adattatori di questa versione leggono")
	}
	return c.chiudi()
}

// chiaveEsito: la chiave dell'esito del worker nel payload (workerapi.go, conDettagli): letture, non fatti.
const chiaveEsito = "esito"

// chiaviDeiFatti: le chiavi del primo livello del payload (analisi_fatti.fatti: le chiavi del worker più
// «esito»). Servono solo a sapere quali parti dei fatti ci sono; le parti si leggono con le funzioni del
// contratto del worker. Un payload che non è un oggetto JSON è un errore.
func chiaviDeiFatti(payload json.RawMessage) (map[string]bool, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(payload, &m); err != nil || m == nil {
		return nil, fmt.Errorf("il payload dei fatti non è un oggetto JSON")
	}
	chiavi := make(map[string]bool, len(m))
	for k := range m {
		chiavi[k] = true
	}
	return chiavi, nil
}

// senzaContenuto: il documento porta il solo nome. La capacità del contenuto non è disponibile, con il motivo,
// e la fonte è parziale: il file c'è, ma che cosa contiene non si sa.
func (c *documento) senzaContenuto(motivo string) {
	c.capacita(capContenuto, statoNonDisponibile, motivo)
	c.peggiora(statoParziale)
}
