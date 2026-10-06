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
