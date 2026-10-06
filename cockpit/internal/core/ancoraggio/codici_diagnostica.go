package ancoraggio

// I codici che questo pacchetto produce (R41 b; piano A, 6.4.10). Ogni pacchetto del motore A dichiara i suoi in
// un file con questo nome; un codice pubblicato non cambia nome né significato, e se non serve più resta qui con il
// commento «ritirato». L'area «ancoraggio» dice di che cosa parla il codice. I codici degli ancoraggi dei file li
// aggiunge il commit che li produce.

// I prodotti della mail (ProponiProdotti, P5).
const (
	// CodiceRichiestaNonValutata — avviso, dati. Un messaggio della richiesta senza il gesto 1 dell'operatore (stato
	// da_valutare o non_valutabile) ha dei candidati: restano da confermare, con il motivo richiesta_non_valutata
	// (6.4.4, regola 1; R29 c). Il triage non lo cambia.
	CodiceRichiestaNonValutata = "ancoraggio.richiesta_non_valutata"

	// CodiceRigheStessaBase — avviso, dati. La stessa base, con gli stessi qualificatori, in più righe di tabella
	// dello stesso segmento: un candidato per riga, ognuno con la sua quantità, nessuna fusione (6.4.4, regola 3; D3).
	CodiceRigheStessaBase = "ancoraggio.righe_stessa_base"

	// CodiceQuantitaNonIntera — avviso, dati. La riga di un candidato ha la colonna della quantità dichiarata, ma la
	// cella non dà un intero (non interpretabile, o colonna ambigua): la quantità resta nil (6.4.4, regola 4; R28 a).
	CodiceQuantitaNonIntera = "ancoraggio.quantita_non_intera"

	// CodiceAlternativeConservate — avviso, dati. Due letture di prodotto, di famiglie o forme diverse, sulla stessa
	// occorrenza: restano tutte e due, ognuna con l'altra fra le alternative; nessuna si sceglie (6.4.4, regola 6).
	CodiceAlternativeConservate = "ancoraggio.alternative_conservate"
)

// Le strutture dei prodotti (ProponiStrutture, P6a). Gli ancoraggi dei file (B4) usano gli stessi codici come motivi
// della collocazione non determinabile (6.4.5, regola 3).
const (
	// CodiceTargetSenzaStruttura — avviso, dati. Un prodotto target senza nessuna struttura: nessun nodo degli STEP
	// letti ha la sua base (o il suo codice non è letto), e nessuna fonte confermata dà una radice che si trova fra le
	// strutture. Il target resta un target: non si conclude che il prodotto non esiste (6.4.5, regola 3; 6.10 n.16).
	CodiceTargetSenzaStruttura = "ancoraggio.target_senza_struttura"

	// CodiceGrafoIncompleto — avviso, dati. Una struttura di un target sta in uno STEP con il grafo non completo
	// (la capacità grafo_completo, con il motivo della 0020: R32 b): la mancanza di un arco non è un'informazione.
	// Contano solo le strutture dei target, mai i file «solo parti» dei figli (6.4.5, regola 2).
	CodiceGrafoIncompleto = "ancoraggio.grafo_incompleto"
)
