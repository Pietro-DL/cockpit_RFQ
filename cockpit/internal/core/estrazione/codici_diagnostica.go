package estrazione

// I codici che gli adattatori producono (R41 b; piano A, 5.4.9). Il prefisso dice di che cosa parla il codice,
// non quale pacchetto lo dichiara. Ogni codice ha natura e gravità predefinita, scritte nel commento; un codice
// pubblicato non cambia nome né significato, e se non serve più resta qui con il commento «ritirato». I codici
// documento.* li dichiara la foglia, che li produce con ValidaDocumento: un documento che non la passa torna
// come errore, con quelle diagnostiche (documento.go, chiudi).

// Il nome del file e la voce d'archivio (mappatura-nome-1).
const (
	// CodiceNomeEstensioneDiscorde — nota, dati. L'estensione della regola dello stem (dividiNome), in
	// minuscolo, è diversa da allegato.estensione (NULL vale ""): le quattro regole di oggi non sono una.
	CodiceNomeEstensioneDiscorde = "nome.estensione_discorde"

	// CodiceNomeForseTroncato — nota, dati. Il nome di un allegato diretto ha 300 rune: l'acquisizione tronca
	// a 300 (ingest.go:895), quindi la fine può mancare. Le voci d'archivio non si troncano.
	CodiceNomeForseTroncato = "nome.forse_troncato"

	// CodiceArchivioNonEstraibile — avviso, capacità. Un archivio .7z o .rar: l'acquisizione non ne estrae le
	// voci (zip.go:1-2), e A lo dice (REG §5.10, «Conseguenze»).
	CodiceArchivioNonEstraibile = "archivio.non_estraibile"
)
