package grammatica

import (
	"strings"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// VersioneCapacita: la versione dell'elenco delle capacità riservate in A1 (R20 c, D-08). Entra nello
// snapshot come metadato, fuori dall'hash, come la versione dei limiti. È una costante del codice: le
// versioni degli algoritmi non si configurano (R43). Cambiare l'elenco qui sotto vuol dire cambiare questa
// versione, con un commit che lo dichiara.
const VersioneCapacita = "capacita-a1-1"

// capacitaRiservate: selettori, campi e tipi che una grammatica può nominare ma che in A1 non sono attivi,
// perché non hanno un caso negli attesi (v3 §2; R20 c). Se una grammatica li usa, la validazione dà
// capacita.non_supportata (avviso) e l'elemento non entra nel motore: nessun piano e nessun codice runtime
// finché non diventa attivo. Il resto resta attivo. L'elenco completo:
//   - forme o regole su: voce_archivio, elenco_pdf.*, metadati_pdf, cartiglio.{numero_disegno, titolo,
//     scala, materiale, particolare_simile, sconosciuto}, radice_step.{nome, descrizione, revisione},
//     nodo_step.{nome, descrizione, revisione} (contesti e campi qui sotto);
//   - selettori generici («cartiglio.*»): Valida li riconosce dalla forma «<contesto>.*», prima di
//     evidenze.LeggiSelettore (D-12; selettoreGenerico);
//   - qualificatori_testuali e riferimenti_rfq non vuoti (Valida, sul file);
//   - decorazioni foglio, formato, sigla_interna, copia, annotazione (qui sotto);
//   - una proiezione parziale che toglie segmenti non in coda alla base (Valida, sulla forma).
//
// Non è riservato il selettore «storia»: una grammatica può dichiararvi forme, affissi ed esempi, come fa la
// forma della mail di uno scenario d'inoltro (R48 A; P-16). Che un segmento di storia valga come richiesta
// lo dice UsoSegmenti, non la grammatica.
var capacitaRiservate = struct {
	contesti    []evidenze.Contesto            // riservati con qualunque campo
	campi       map[evidenze.Contesto][]string // riservati solo con questi campi
	decorazioni []string
}{
	contesti: []evidenze.Contesto{evidenze.ContestoVoceArchivio, evidenze.ContestoElencoPDF, evidenze.ContestoMetadatiPDF},
	campi: map[evidenze.Contesto][]string{
		evidenze.ContestoCartiglio:  {"numero_disegno", "titolo", "scala", "materiale", "particolare_simile", "sconosciuto"},
		evidenze.ContestoRadiceSTEP: {"nome", "descrizione", "revisione"},
		evidenze.ContestoNodoSTEP:   {"nome", "descrizione", "revisione"},
	},
	decorazioni: []string{TipoDecorazioneFoglio, TipoDecorazioneFormato, TipoDecorazioneSiglaInterna, TipoDecorazioneCopia, TipoDecorazioneAnnotazione},
}

// selettoreRiservato dice se un selettore valido è fra quelli riservati in A1.
func selettoreRiservato(s evidenze.Selettore) bool {
	for _, c := range capacitaRiservate.contesti {
		if s.Contesto == c {
			return true
		}
	}
	return in(s.Campo.Valore, capacitaRiservate.campi[s.Contesto])
}

// selettoreGenerico riconosce la forma «<contesto>.*» con un contesto dell'elenco chiuso: non è una coppia,
// e in A1 è riservato (D-12). Un contesto fuori elenco non è un generico: lo rifiuta LeggiSelettore.
func selettoreGenerico(s string) bool {
	ctx, campo, ok := strings.Cut(s, ".")
	if !ok || campo != "*" {
		return false
	}
	v, _ := evidenze.CampiAmmessi(evidenze.Contesto(ctx))
	return v != ""
}

// decorazioneRiservata dice se un tipo di decorazione è fra quelli riservati in A1.
func decorazioneRiservata(tipo string) bool { return in(tipo, capacitaRiservate.decorazioni) }

// proiezioneInCoda: i segmenti mancanti sono gli ultimi della base. Una proiezione che ne toglie altri è
// riservata in A1 (R20 c).
func proiezioneInCoda(mancanti []string, b Base) bool {
	n := len(mancanti)
	if n > len(b.Segmenti) {
		return false
	}
	for _, s := range b.Segmenti[len(b.Segmenti)-n:] {
		if !in(s.Nome, mancanti) {
			return false
		}
	}
	return true
}

// SelettoreAttivo, DecorazioneAttiva e ProiezioneAttiva espongono la stessa politica che Valida applica a chi
// abbassa la grammatica nel motore (motorea): un elemento riservato in A1 non entra nel motore, e il resto sì
// (R20 c; par.12 del piano A, nessuna infrastruttura per ciò che è solo riservato). L'elenco dei riservati
// resta uno solo, qui, con la sua VersioneCapacita: chi compila non ne tiene una copia. Non danno
// diagnostiche: le dà Valida.

// SelettoreAttivo: il selettore, nella forma testuale della grammatica, è una coppia ammessa, non generica e
// fuori dall'elenco dei riservati in A1. «storia» è attivo (R48 A).
func SelettoreAttivo(s string) bool {
	if selettoreGenerico(s) {
		return false
	}
	sel, err := evidenze.LeggiSelettore(s)
	return err == nil && !selettoreRiservato(sel)
}

// DecorazioneAttiva: il tipo di decorazione è nell'enum e non è fra quelli della parte 1 riservati in A1.
func DecorazioneAttiva(tipo string) bool {
	return in(tipo, tipiDecorazione) && !decorazioneRiservata(tipo)
}

// ProiezioneAttiva: la forma è completa, o è una proiezione che toglie segmenti in coda alla base.
func ProiezioneAttiva(fo FormaCodice, b Base) bool {
	return fo.Completa || proiezioneInCoda(fo.SegmentiMancanti, b)
}
