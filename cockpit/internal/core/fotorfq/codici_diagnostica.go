package fotorfq

// I codici che la fotografia produce (piano A, 6.4.10; R41 b). Li dichiara questo pacchetto perché li producono
// ValidaFotografia, il caricatore e il lettore degli export del banco, che importano tutti fotorfq. Un codice
// pubblicato non cambia nome né significato; se non serve più resta qui, con il commento «ritirato».
const (
	// CodiceNessunAnalizzatore — avviso, dati. analizzatore_corrente è vuota: nessuna terna corrente, quindi
	// nessun fatto caricato (la sezione dei fatti è assente).
	CodiceNessunAnalizzatore = "fotografia.nessun_analizzatore"

	// CodiceFattiAssenti — avviso, dati. I fatti di una sorgente non ci sono (per esempio un export dei fatti che
	// manca): ciò che ne dipende non si calcola.
	CodiceFattiAssenti = "fotografia.fatti_assenti"

	// CodiceTernaDiversa — errore, contratto. Fatti a una terna diversa da quella della fotografia, o fatti senza
	// una terna corrente.
	CodiceTernaDiversa = "fotografia.terna_diversa"

	// CodiceSezioneParziale — avviso, dati. Una sezione della fotografia è parziale o filtrata (gli export).
	CodiceSezioneParziale = "fotografia.sezione_parziale"

	// CodiceRiferimentoNonRisolto — errore, contratto. Un riferimento della fotografia che non si risolve
	// (allegato → messaggio, provenienza → allegato, proposta → allegato, le righe delle viste e le deroghe →
	// componente, i documenti citati → documento confermato, aggancio → messaggio), e gli altri controlli di
	// contratto di ValidaFotografia (contratto di A1c, §3.5: un candidato dell'agente; 6.4.1: un tempo non UTC al
	// millisecondo).
	CodiceRiferimentoNonRisolto = "fotografia.riferimento_non_risolto"
)
