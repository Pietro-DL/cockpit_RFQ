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
