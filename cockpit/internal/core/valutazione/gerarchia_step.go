package valutazione

// gerarchia_step: che cosa lo STEP della fonte dice della gerarchia del prodotto (P4, passo B3.4 del piano esecutivo;
// direttiva 11 D-SHOCK-S1 B [U]; contratto rev. 3, Emendamento 1 E1.1 ed Emendamento 2 E2.1; lente struttura §5).
// È un campo derivato, mai persistito: si calcola dai numeri della struttura letta dall'analisi corrente (versione,
// troncamento, scarti, radici, relazioni), non dal testo del motivo parziale, che il worker stesso giudica fragile.
// Il motivo parziale non si tocca e non si svuota (vincolo V2 della verifica avversaria dell'interrogativo 1): la
// sospensione delle rimozioni di una lettura «solo parti» dipende da lui, e questa funzione lo lascia com'è.

// GerarchiaStep: i quattro valori (piano esecutivo B3.4; lente struttura §5, il quarto valore parziale):
//   - riconosciuta: la lettura è completa e ha relazioni di assieme;
//   - non_riconosciuta: la lettura è completa, una radice e nessuna relazione (uno STEP «solo parti»): lo STEP non
//     dimostra che i figli non esistano, dà la geometria ma non la gerarchia (direttiva 11 D-SHOCK-S1 B);
//   - parziale: la lettura è troncata, ha scarti, ha più radici, oppure ha relazioni ma un motivo parziale;
//   - non_analizzabile: nessuna analisi corrente leggibile, una versione della struttura che la regola non conosce,
//     oppure nessuna radice. Comprende i due casi della 0020: lo STEP senza un'analisi corrente (v_step_prodotto,
//     presente_non_analizzato, «nessuna analisi corrente dello STEP») e lo STEP con un'analisi corrente la cui
//     struttura non si legge (presente_parziale, struttura_motivo_parziale).
//
// Nessun valore blocca da solo la verifica: con il criterio step e la fonte confermata, non_riconosciuta, parziale e
// non_analizzabile vogliono una deroga_struttura applicabile (direttiva 12 §2.1 [U], che supera T-2, il blocco di
// non_analizzabile anche con la deroga; addendum ad A1c AD-4). Il valore resta esposto in VerificaBOM.GerarchiaStep
// anche con la verifica positiva: il fatto che lo STEP non determini la gerarchia deve continuare a vedersi.
type GerarchiaStep string

const (
	GerarchiaStepRiconosciuta    GerarchiaStep = "riconosciuta"
	GerarchiaStepNonRiconosciuta GerarchiaStep = "non_riconosciuta"
	GerarchiaStepParziale        GerarchiaStep = "parziale"
	GerarchiaStepNonAnalizzabile GerarchiaStep = "non_analizzabile"
)

// VersioneStrutturaStep: la versione della struttura STEP che la regola sa leggere (la stessa che vuole
// struttura_motivo_parziale nella 0020).
const VersioneStrutturaStep = 3

// AnalisiStruttura: gli esiti dell'analisi corrente dello STEP della fonte, già letti dal chiamante (l'ingresso di
// GerarchiaStepDi; tipo [T] di B3.4). Nessuna lettura dal DB qui.
//   - Letta: c'è un'analisi corrente della fonte e la sua struttura si legge (fatti presenti, nessun errore);
//   - Versione: la versione della struttura (VersioneStrutturaStep);
//   - Troncato: la lettura si è fermata a un limite (limiti.troncato);
//   - Scarti: quante entità la lettura ha scartato (la somma dei suoi contatori);
//   - Radici, Relazioni: quante radici e quante relazioni di assieme la lettura ha trovato;
//   - MotivoParziale: struttura_motivo_parziale com'è; la regola lo guarda solo per dire parziale quando ci sono
//     relazioni, e lo restituisce intatto.
type AnalisiStruttura struct {
	Letta          bool   `json:"letta"`
	Versione       int    `json:"versione,omitempty"`
	Troncato       bool   `json:"troncato,omitempty"`
	Scarti         int    `json:"scarti,omitempty"`
	Radici         int    `json:"radici,omitempty"`
	Relazioni      int    `json:"relazioni,omitempty"`
	MotivoParziale string `json:"motivo_parziale,omitempty"`
}

// GerarchiaStepDi: il valore di gerarchia_step, funzione pura dei numeri dell'analisi. Vale il primo che si applica:
//  1. non_analizzabile senza una lettura (nessuna analisi corrente, o una struttura che non si legge), con una versione
//     diversa da VersioneStrutturaStep, o senza radici: non blocca, vuole la deroga (direttiva 12 §2.1);
//  2. parziale con una lettura troncata, con scarti o con più radici: «una radice e nessuna relazione» da sola
//     comprende anche una lettura troncata prima delle occorrenze, che non va detta senza gerarchia (lente §5);
//  3. non_riconosciuta con una radice e nessuna relazione;
//  4. parziale con relazioni e un motivo parziale (occorrenze non risolte, PRODUCT orfani: la lettura non è completa);
//  5. altrimenti riconosciuta.
func GerarchiaStepDi(a AnalisiStruttura) GerarchiaStep {
	switch {
	case !a.Letta || a.Versione != VersioneStrutturaStep || a.Radici <= 0:
		return GerarchiaStepNonAnalizzabile
	case a.Troncato || a.Scarti > 0 || a.Radici > 1:
		return GerarchiaStepParziale
	case a.Relazioni <= 0:
		return GerarchiaStepNonRiconosciuta
	case a.MotivoParziale != "":
		return GerarchiaStepParziale
	default:
		return GerarchiaStepRiconosciuta
	}
}
