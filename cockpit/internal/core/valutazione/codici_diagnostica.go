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
