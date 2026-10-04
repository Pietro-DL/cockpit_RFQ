package evidenze

// I codici che questo pacchetto produce (R41 b; piano A, par.4.4.6). Ogni pacchetto del motore A dichiara
// i suoi in un file con questo nome; un pacchetto che sta sopra può emettere un codice di qui con la sua
// costante, o passare avanti le diagnostiche che riceve, ma non lo ridichiara mai. Un codice pubblicato non
// cambia nome né significato; se non serve più resta qui, con il commento «ritirato».

// Validazione dei selettori (LeggiSelettore, Selettore.Valida).
const (
	// CodiceSelettoreNonAmmesso — errore, contratto. Una coppia contesto/campo fuori dalla tabella della
	// parte 1 §4.2, «nessuno» usato come jolly, un contesto fuori elenco (anche «figlio_step», che solo il
	// runner degli attesi traduce). La grammatica la passa avanti, non la ridichiara.
	CodiceSelettoreNonAmmesso = "contratto.selettore_non_ammesso"

	// CodiceSelettoreGenerico — errore, contratto. «<contesto>.*» dato a LeggiSelettore, che vuole una
	// coppia (D-12). In una grammatica non arriva mai: la sua validazione riconosce prima il generico e dà
	// la diagnosi di capacità.
	CodiceSelettoreGenerico = "contratto.selettore_generico"
)
