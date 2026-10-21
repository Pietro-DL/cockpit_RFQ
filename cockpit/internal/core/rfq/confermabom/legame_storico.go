package confermabom

// legame_storico.go: il legame storico univoco di D-AR04 (iii) A [U] («se esistono piu' candidati o evidenze discordanti,
// non si seleziona automaticamente una sorgente»), letto per i prodotti senza conferme di S8 (R39-06). Una regola sola,
// pura, per tutti i lettori: legami_storici[] di R4 (distintabozza) la usa; /dati.fonte.legame_storico del Motore A segue
// la stessa regola (motore_a/dati_v1.go, legameStorico) e puo' chiamare questa funzione.
//
// La regola: il prodotto non ha nessuna conferma di S8 (conferma_documento della storia della fonte, ruolo prodotto, con
// la chiave d'idempotenza del passo 2); fra i suoi documenti cad_3d c'e' uno e un solo STEP (estensione .stp/.step, o il
// nome del file che finisce cosi') corrente, cioe' non sostituito. Con zero o con piu' di uno non c'e' legame storico.

import (
	"strings"

	"github.com/google/uuid"
)

// DocumentoDelProdotto: un documento cad_3d del prodotto finito, con quel che serve alla regola del legame storico.
type DocumentoDelProdotto struct {
	DocumentoID uuid.UUID
	Sha256      string
	NomeFile    string
	Estensione  string
	Sostituito  bool
}

// LegameStoricoUnivoco: il legame storico univoco del prodotto (commento di testa), se c'e'. conS8 dice se il prodotto ha
// almeno una conferma di S8.
func LegameStoricoUnivoco(docs []DocumentoDelProdotto, conS8 bool) (DocumentoDelProdotto, bool) {
	if conS8 {
		return DocumentoDelProdotto{}, false
	}
	var out DocumentoDelProdotto
	trovati := 0
	for _, d := range docs {
		if d.Sostituito || !EstensioneOFileStep(d.Estensione, d.NomeFile) {
			continue
		}
		trovati++
		out = d
	}
	if trovati != 1 {
		return DocumentoDelProdotto{}, false
	}
	return out, true
}

// EstensioneOFileStep: il formato dichiarato del documento, o il nome del file, e' uno STEP (.stp, .step), senza
// distinguere maiuscole e senza gli spazi ai bordi (la stessa lettura di /dati).
func EstensioneOFileStep(estensione, nome string) bool {
	e := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(estensione), "."))
	if e == "stp" || e == "step" {
		return true
	}
	n := strings.ToLower(strings.TrimSpace(nome))
	return strings.HasSuffix(n, ".stp") || strings.HasSuffix(n, ".step")
}
