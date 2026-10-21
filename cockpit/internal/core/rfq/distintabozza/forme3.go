package distintabozza

// Le forme di R2 costruite dal server (EM §9.2; direttiva 35, blocco C-3, R2 passo 6b): crea, cambia_sorgente,
// riallinea_proposta, converti_da. Qui i codici e i motivi che le quattro forme condividono; i servizi sono in crea3.go
// (crea), cambiafonte3.go (cambia_sorgente), riallinea3.go (riallinea_proposta) e conversione.go (converti_da), i
// costruttori della versione in partenza3.go.

// I codici nuovi delle forme che importano (EM §6.1, §6.3, §9.2; U2 §2.8).
const (
	// CodiceFonteNonConfermata: crea (partenza step) e cambia_sorgente vogliono che la sorgente dichiarata sia lo STEP
	// confermato, e non sostituito, del prodotto (U2 §2.8, base [T] U2-P38; 422).
	CodiceFonteNonConfermata = "FONTE_NON_CONFERMATA"
	// CodicePropostaNonDisponibile: il thread non e' valutato per un errore, o il registro sarebbe incompleto (EM §6.1; 422).
	CodicePropostaNonDisponibile = "PROPOSTA_NON_DISPONIBILE"
	// CodiceImportazioneTroppoGrande: il registro supera il limite della foglia (EM §6.1; 422, mai una troncatura).
	CodiceImportazioneTroppoGrande = "IMPORTAZIONE_TROPPO_GRANDE"
)

// I motivi di CORPO_NON_VALIDO delle forme costruite dal server (EM §6.1, §6.5, §6.6, §6.12, §9.2) e il motivo del 409
// ANTEPRIMA_SUPERATA delle forme che importano (MotivoPropostaStep, rifiuto.go).
const (
	MotivoGruppoNonImportabile     = "gruppo_non_importabile"
	MotivoGruppoSuAltraOccorrenza  = "gruppo_su_altra_occorrenza"
	MotivoStessaSorgente           = "stessa_sorgente"
	MotivoPartenzaNonAmmessa       = "partenza_non_ammessa"
	MotivoRiallineamentoIncompleto = "riallineamento_incompleto"
	MotivoErroreValutazione        = "errore_valutazione"
	MotivoRegistroIncompleto       = "registro_incompleto"
)

// I verbi del server delle forme costruite dal server (vocabolario 2, EM §9.2) sono AzBozzaAperta, AzSorgenteScelta,
// AzSorgenteCambiata (azioni.go) e AzPropostaRiallineata (vocabolario2.go).

// Gli algoritmi delle versioni costruite dal server (EM §2.5, §6.1, §6.5, §6.6, §6.12). conversione-1 e' in
// conversione.go.
const (
	AlgoritmoImportazione = "importazione-1"
	AlgoritmoRiancoraggio = "riancoraggio-1"
	AlgoritmoCambioFonte  = "cambio-fonte-1"
	AlgoritmoDaConferma   = "da-conferma-1"
	AlgoritmoRegistrato   = "registrato-1"
	AlgoritmoVuota        = "vuota-1"
)

// La partenza solo_prodotto (direttiva 39 §3 [U + T-39]; NOMI (b)): uno STEP confermato da S8 senza struttura
// importabile. La versione 1 e' il solo prodotto (come la partenza vuota) con la sorgente STEP reale (allegato_id,
// sha256) e il gruppo riservato «solo_prodotto», senza registro d'importazione: niente gruppo estratto ne' registro
// fittizio. In fonte_step_evento la scelta usa lo stesso gruppo riservato (mai «senza», che e' un gruppo del motore).
// Nasce solo con S8 (la rotta di R2 la rifiuta: partenza_non_ammessa).
const (
	PartenzaSoloProdotto  = "solo_prodotto"
	GruppoSoloProdotto    = "solo_prodotto"
	AlgoritmoSoloProdotto = "solo-prodotto-1"
)

// partenze3: le partenze che il servizio di crea accetta (quelle di R2 piu' solo_prodotto, che arriva solo da S8).
var partenze3 = map[string]bool{PartenzaConferma: true, PartenzaRegistrato: true, PartenzaStep: true, PartenzaVuota: true,
	PartenzaSoloProdotto: true}

// chiaveDelServer: la chiave del salvataggio di una forma costruita dal server. La rotta di R2 vuole un UUID v4 (lo
// controlla LeggiRichiestaR2); S8 passa le sue chiavi dei passi, UUID v5 derivati dalla chiave d'intento (U2 §2.2).
func chiaveDelServer(c ComuneR2) bool {
	v := c.ChiaveSalvataggio.Version()
	return (v == 4 || v == 5) && reSha256.MatchString(c.ImprontaRichiesta)
}
