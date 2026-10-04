package motorea

// I codici che questo pacchetto produce (R41 b; piano A, par.4.4.6). Ogni pacchetto del motore A dichiara
// i suoi in un file con questo nome. Qui se ne emettono anche alcuni dichiarati da grammatica, con la sua
// costante e senza ridichiararli: limite.superato (in Riconosci, come avviso), grammatica.pattern_non_compila
// (una regex generata che non compila), capacita.non_supportata e grammatica.forma_riservata (una forma che
// non entra nel motore), regole.cliente_ripetuto e contratto.campo_obbligatorio (l'indice). Il prefisso dice
// di che cosa parla il codice, non quale pacchetto lo dichiara: grammatica.esempio_* li dichiara motorea,
// che verifica gli esempi. Un codice pubblicato non cambia nome né significato; se non serve più resta qui,
// con il commento «ritirato».
//
// Nel commento di ogni costante: gravità predefinita, natura, quando.

// La verifica degli esempi (CompilaVerificato). Un errore vieta l'attivazione della grammatica.
const (
	// CodiceEsempioMancante — errore, contratto. Una lettura attesa da un esempio che Riconosci, sul motore
	// intero del cliente, non produce.
	CodiceEsempioMancante = "grammatica.esempio_mancante"

	// CodiceEsempioInEccesso — errore, contratto. Una lettura prodotta su un esempio e non attesa, senza
	// altre_ammesse; anche una lettura qualunque su un esempio negativo.
	CodiceEsempioInEccesso = "grammatica.esempio_in_eccesso"

	// CodiceEsempioDiverso — errore, contratto. Una lettura della famiglia e della forma attese, ma con
	// campi diversi da quelli dell'esempio.
	CodiceEsempioDiverso = "grammatica.esempio_diverso"

	// CodiceEsempioSelettoreInattivo — errore, contratto. Un esempio su un selettore senza forme attive né
	// revisioni in campo separato: non c'è niente con cui verificarlo.
	CodiceEsempioSelettoreInattivo = "grammatica.esempio_selettore_inattivo"

	// CodiceFormaCollisioneEsempi — errore, contratto. Due forme della stessa famiglia leggono la stessa
	// occorrenza (intervalli che si sovrappongono) su un esempio: nessuna precedenza per ordine di array
	// (E-23; R25 f).
	CodiceFormaCollisioneEsempi = "forma.collisione_esempi"

	// CodiceEsempioNonVerificato — nota, capacità. Un esempio di una forma che non entra nel motore
	// (riservata, o con una capacità riservata in A1): resta «non verificato», mai «passato», e si conta a
	// parte.
	CodiceEsempioNonVerificato = "grammatica.esempio_non_verificato"
)

// L'insieme delle regole (CompilaInsieme, MotoreDi).
const (
	// CodiceRegoleAssenti — nota, dati. Un cliente senza voce nell'indice: non si valuta, nessuna regola A
	// (E-23).
	CodiceRegoleAssenti = "regole.assenti"

	// CodiceRegoleNonValide — errore, contratto. La grammatica di un cliente è scartata: le diagnostiche che
	// seguono dicono perché. Gli altri clienti restano attivi (E-23).
	CodiceRegoleNonValide = "regole.non_valide"

	// CodiceRegoleFileAssente — errore, contratto. Il file che l'indice indica per un cliente non è fra i
	// contenuti ricevuti.
	CodiceRegoleFileAssente = "regole.file_assente"

	// CodiceRegoleSha256Discorde — errore, contratto. Lo sha256 del file è diverso da quello dell'indice:
	// un file cambiato non passa in silenzio (R29 d).
	CodiceRegoleSha256Discorde = "regole.sha256_discorde"

	// CodiceRegoleClienteDiscorde — errore, contratto. Il cliente.id del file è diverso dalla voce
	// dell'indice (par.3.6.3).
	CodiceRegoleClienteDiscorde = "regole.cliente_discorde"
)

// Il riconoscimento (Riconosci).
const (
	// CodiceRevisioneDaVerificare — avviso, dati. Una lettura con un token di revisione sospeso, come
	// «/xx» o «_nn»: il token si conserva, nessun valore di revisione, da verificare (E-11, D5, R16).
	CodiceRevisioneDaVerificare = "revisione.da_verificare"
)
