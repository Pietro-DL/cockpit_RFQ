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

// Validazione del documento e dell'uso dei segmenti (ValidaDocumento, ValidaUso). Erano di A1b nella bozza:
// con la foglia intera in A1a (R49 C) nascono qui.
const (
	// CodiceDocumentoRiferimentoPendente — errore, contratto. Un ID di fonte, segmento, entità o testo
	// che non si risolve nel documento.
	CodiceDocumentoRiferimentoPendente = "documento.riferimento_pendente"

	// CodiceDocumentoIntervalloNonValido — errore, contratto. Un intervallo fuori dal testo, rovesciato o
	// a metà di una runa.
	CodiceDocumentoIntervalloNonValido = "documento.intervallo_non_valido"

	// CodiceDocumentoUTF8NonValido — errore, contratto. Un testo del documento non è UTF-8 valido.
	CodiceDocumentoUTF8NonValido = "documento.utf8_non_valido"

	// CodiceDocumentoEnumIgnoto — errore, contratto. Un valore fuori da un enum del documento, o una
	// coppia non ammessa (un selettore fuori tabella, un localizzatore con la variante sbagliata, una
	// quantità su un legame che non la porta).
	CodiceDocumentoEnumIgnoto = "documento.enum_ignoto"

	// CodiceDocumentoTestoDiversoDaOriginale — errore, contratto. Localizzazione dichiarata esatta, ma il
	// testo dell'unità è diverso da originale[inizio:fine] (A-C03).
	CodiceDocumentoTestoDiversoDaOriginale = "documento.testo_diverso_da_originale"

	// CodiceDocumentoIDRipetuto — errore, contratto. Due ID locali uguali nello stesso documento.
	CodiceDocumentoIDRipetuto = "documento.id_ripetuto"

	// CodiceDocumentoUsoNonValido — errore, contratto. Un uso dei segmenti di un altro bundle, di una
	// versione ignota, con un segmento che non c'è o con un enum ignoto (parte 1 §9.3).
	CodiceDocumentoUsoNonValido = "documento.uso_non_valido"
)
