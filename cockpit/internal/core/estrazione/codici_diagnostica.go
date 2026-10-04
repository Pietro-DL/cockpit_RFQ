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

// La struttura STEP (mappatura-step-1).
const (
	// CodiceSTEPNonAnalizzato — nota, dati. Uno STEP senza fatti del worker: del file si legge solo il nome.
	CodiceSTEPNonAnalizzato = "step.non_analizzato"

	// CodiceSTEPStrutturaAssente — avviso, dati. I fatti non hanno una struttura che si legge: assente, o di
	// una versione precedente alla 2 (DecodificaStruttura falsa). Va rianalizzato, non letto.
	CodiceSTEPStrutturaAssente = "step.struttura_assente"

	// CodiceSTEPNonLetto — avviso, dati. Nessun nodo, e l'avviso del worker che il file non si è letto (non è
	// Part 21, o la lettura è fallita): lo stato della fonte è «errore».
	CodiceSTEPNonLetto = "step.non_letto"

	// CodiceSTEPTroncato — avviso, dati. limiti.troncato: il worker si è fermato su un tetto, e il grafo letto
	// è parziale.
	CodiceSTEPTroncato = "step.troncato"

	// CodiceSTEPScarti — avviso, dati. Gli scarti della v3 diversi da zero: PRODUCT senza definizione,
	// occorrenze non risolte o di un pezzo in sé stesso.
	CodiceSTEPScarti = "step.scarti"

	// CodiceSTEPCarattereSostituito — avviso, dati. Un valore contiene U+FFFD: un carattere non si è
	// decodificato (per esempio un surrogato spezzato dal troncamento del worker).
	CodiceSTEPCarattereSostituito = "step.carattere_sostituito"

	// CodiceSTEPFormazioniAlternative — avviso, dati. Il PRODUCT ha più formazioni con id diversi
	// (evidenza.rev_alternative): tutte diventano unità, nessuna si sceglie.
	CodiceSTEPFormazioniAlternative = "step.formazioni_alternative"

	// CodiceSTEPScartiNonNoti — nota, dati. Struttura v2: gli scarti in numeri non ci sono, quindi che cosa il
	// worker ha lasciato fuori non si sa.
	CodiceSTEPScartiNonNoti = "step.scarti_non_noti"

	// CodiceSTEPTestoTroncato — nota, dati. scarti.testi_troncati > 0: il worker ha tagliato dei testi a 200
	// code point; le unità che arrivano al tetto sono candidate troncate (QualitaUnita.Troncata).
	CodiceSTEPTestoTroncato = "step.testo_troncato"
)
