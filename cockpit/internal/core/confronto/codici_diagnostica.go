package confronto

// I codici che questo pacchetto produce (R41 b; piano A, 6.4.10). Ogni pacchetto del motore A dichiara i suoi in un
// file con questo nome; un codice pubblicato non cambia nome né significato, e se non serve più resta qui con il
// commento «ritirato». Il prefisso dice di che cosa parla il codice, non quale pacchetto lo dichiara:
// confronto.codice_registrato_non_leggibile parla del confronto ma lo dichiara valutazione, che legge i codici
// registrati con la grammatica (R31 c).

// L'indicatore di revisione (P8).
const (
	// CodiceRevisioneNonConfrontabile — nota, dati. Le revisioni del vecchio e del nuovo di un file non si confrontano:
	// valutazione, che ha la grammatica, le dice non_confrontabili (per esempio una formazione STEP «00.00» del legacy,
	// con il token non attribuito), oppure il valore che riceve è fuori vocabolario. L'indicatore è non_determinabile,
	// e la sola revisione non è mai un badge né una regressione (R31 b; E-19). Una per file, con l'ID dell'allegato.
	CodiceRevisioneNonConfrontabile = "confronto.revisione_non_confrontabile"
)
