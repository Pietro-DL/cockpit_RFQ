package valutazione

// I codici che questo pacchetto produce (R41 b; piano A, 6.4.10; contratto §2.3 e §2.5). Ogni pacchetto del motore A
// dichiara i suoi in un file con questo nome; un codice pubblicato non cambia nome né significato, e se non serve più
// resta qui con il commento «ritirato». Il prefisso dice di che cosa parla il codice, non quale pacchetto lo
// dichiara. Gli altri codici di valutazione li aggiunge il commit che li produce (B5, B6). Per la porta stretta del
// file dei casi si usano i codici «contratto.*» che la grammatica dichiara, senza ridichiararli.

// I target (P7a).
const (
	// CodiceTargetNonLeggibile — avviso, dati. Il codice di un target confermato (identificativo o finito manuale)
	// non si legge con la grammatica del cliente (6.4.6, LetturaRegistrata; R31 c): la base resta vuota e la
	// compatibilità con le radici degli STEP non si calcola. Nessun confronto per stringa: la base non si inventa, e
	// una fusione per stringa sarebbe un legame senza provenance (5.0). Il 6.4.6, passo 6, la ammetteva; B1 se ne
	// discosta (lettura dell'orchestratore T-B1-07).
	CodiceTargetNonLeggibile = "ancoraggio.target_non_leggibile"

	// CodiceTargetPossibileRinomina — avviso, dati. Un identificativo confermato senza componente e, nello stesso
	// thread, un finito attivo che non è target: forse il finito è stato rinominato. Il target non si corregge e la
	// rinomina non si deduce (T-E1-24; emendamento E1 §4.2).
	CodiceTargetPossibileRinomina = "target.possibile_rinomina"
)

// La fonte strutturale (P7a).
const (
	// CodiceRiferimentoIncoerente — avviso, dati. Il gesto 3 non torna con sé stesso: lo STEP strutturale fuori dai
	// documenti del thread o diverso da quello della vista, una marcatura di un altro componente o di un altro
	// contenuto, più marcature valide per lo stesso STEP, una marcatura valida senza step_strutturale_id, una delega
	// la cui radice non ha la base del prodotto. Nessuna conferma si deduce (contratto §1.2; T-B0-07, T-B0-08).
	CodiceRiferimentoIncoerente = "fonte_strutturale.riferimento_incoerente"

	// CodiceRadiceNonRegistrata — avviso, dati. Lo STEP strutturale non dice la radice: nessuna marcatura, e le righe
	// del file non hanno una sola radice senza arco entrante. La radice resta «non registrata», mai scelta dal
	// sistema (T-B0-08).
	CodiceRadiceNonRegistrata = "fonte_strutturale.radice_non_registrata"
)

// La verifica della BOM (P7b).
const (
	// CodiceBOMFonteNonRegistrata — avviso, dati. La BOM del prodotto è letta dal gesto legacy, «Conferma l'albero»,
	// che conferma la BOM ma non registra da quale STEP, sha256 o radice venga l'albero (R61 A; contratto §1.3, §2.3):
	// VerificaBOM.FonteRegistrata è falso, e la fonte resta quella dell'asse 2. Si dice per ogni prodotto il cui
	// perimetro ha il segno della conferma.
	CodiceBOMFonteNonRegistrata = "bom.fonte_non_registrata"
)

// La completezza documentale (P7b).
const (
	// CodiceDocumentiDerogaNonSostituisce2D — avviso, dati. Una deroga sul disegno 2D di un componente del perimetro
	// (deroga_fabbisogno con il tipo disegno_2d): la deroga non sostituisce il 2D (R62 D.4; contratto §1.6, «La
	// deroga»), quindi la voce del 2D resta con il suo esito, e senza un 2D valido è «manca» con il motivo
	// derogato_non_sostituisce_2d. Sugli altri tipi la deroga vale come nella vista (S3). Si dice per ogni voce del 2D
	// con una deroga, per prodotto.
	CodiceDocumentiDerogaNonSostituisce2D = "documenti.deroga_non_sostituisce_2d"
)

// Calcola e il vecchio (P7c; B6, V1).
const (
	// CodiceErroreValutazione — errore, dati. La valutazione di un thread, o di un messaggio fuori RFQ, dà un errore che
	// non porta già le diagnostiche di un errore di contratto: un caso del file dei casi di un altro thread o di un altro
	// cliente, un messaggio che l'adattatore non legge, un messaggio di un caso di censimento che la fotografia non ha
	// (F0-05; T-B6-09). Il thread non è valutato, con il motivo errore_valutazione se la grammatica c'è, e gli altri
	// thread proseguono. Con un errore di contratto se ne copiano le diagnostiche, e questo codice non c'è. Come avviso,
	// l'errore dell'adattatore o dell'interpretazione su un file solo, che resta fuori (documento_non_leggibile) mentre
	// il thread prosegue (R-61 della revisione di V1).
	CodiceErroreValutazione = "valutazione.errore_valutazione"

	// CodiceCodiceRegistratoNonLeggibile — avviso, dati. Il codice registrato di un file (della proposta, o del documento
	// confermato se il file è deciso) non si legge con la grammatica del cliente (6.4.6, LetturaRegistrata; R31 c): la
	// base vecchia resta vuota, e il confronto non la sostituisce con la stringa (T-B1-07). Il prefisso dice di che cosa
	// parla, il confronto del vecchio con il nuovo; lo dichiara valutazione, che lo produce (6.4.10; T-B6-06).
	CodiceCodiceRegistratoNonLeggibile = "confronto.codice_registrato_non_leggibile"

	// CodiceMessaggioFuoriRFQSenzaCaso — avviso, dati. Un messaggio fuori RFQ della fotografia che nessun caso senza thread
	// del file dei casi elenca (R34; T-B6-04; T-B6-26 con R-63 della revisione di V1): non si valuta. È un'incoerenza fra
	// chi chiama e il file dei casi: fra i fuori RFQ (caricatore.Richiesta.Messaggi) vanno solo i messaggi dei casi senza
	// thread. Il messaggio di un caso che la fotografia non ha è invece un record non valutato, con
	// valutazione.errore_valutazione (F0-05).
	CodiceMessaggioFuoriRFQSenzaCaso = "valutazione.messaggio_fuori_rfq_senza_caso"
)

// Lo smistamento (P7c; B6, le correzioni dopo le risposte dell'utente a R106–R117 del 07/10).
const (
	// CodiceContestoDiscorde — avviso, dati. Il messaggio a cui il file è allegato nomina prodotti target che non hanno
	// niente in comune con quelli a cui il file è già collegato: per le evidenze di pertinenza (un candidato del motore A,
	// la collocazione non determinabile di un target senza struttura, la destinazione F8, l'«assegna», un documento
	// confermato, lo STEP candidato della fonte) o, per un file terminale, per la decisione (i prodotti con il componente
	// del documento confermato). Il contesto non aggiunge destinazioni e non sostituisce la decisione: «una
	// contraddizione deve restare visibile» (R106 B, precisata dall'utente il 07/10). Non blocca niente, non cambia la
	// pertinenza e non entra nell'impronta del prodotto. Porta l'allegato, i prodotti nominati e quelli collegati.
	CodiceContestoDiscorde = "valutazione.contesto_discorde"
)
