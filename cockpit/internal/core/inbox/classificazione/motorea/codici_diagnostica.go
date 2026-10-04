package motorea

// I codici che questo pacchetto produce (R41 b; piano A, par.4.4.6). Ogni pacchetto del motore A dichiara
// i suoi in un file con questo nome. Qui se ne emettono anche alcuni dichiarati da grammatica, con la sua
// costante e senza ridichiararli: limite.superato (in Riconosci, come avviso), grammatica.pattern_non_compila
// (una regex generata che non compila), capacita.non_supportata e grammatica.forma_riservata (una forma che
// non entra nel motore), regole.cliente_ripetuto e contratto.campo_obbligatorio (l'indice). Da A1b Interpreta
// emette anche limite.superato (i limiti del documento, come avviso), capacita.non_supportata (un campo ricevuto
// e non letto: come nota, «una sola nota per selettore» del 5.4.6 punto 15, non come l'avviso con cui la
// dichiara grammatica per un elemento riservato della grammatica; la natura resta capacità) e
// documento.uso_non_valido (un uso senza forma canonica), con le costanti di chi li dichiara, e passa
// avanti le diagnostiche di ValidaDocumento e ValidaUso nel suo errore di contratto. Il prefisso dice
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

// L'interpretazione (Interpreta, A1b.10; 5.4.9). Nessuno è un errore: l'errore di Interpreta è solo di
// contratto (documento o uso non validi), e un limite o un'ambiguità dei dati sta dentro l'Interpretazione.
const (
	// CodiceMotoreLettureAlternative — avviso, dati. Due letture sovrapposte sulla stessa unità, di famiglie
	// diverse o della stessa famiglia con letture diverse dello stesso tratto: restano tutte e due (5.4.6
	// punto 10).
	CodiceMotoreLettureAlternative = "motore.letture_alternative"

	// CodiceMotoreLettureAnnidate — avviso, dati. Una lettura contenuta in un'altra della stessa famiglia,
	// sulla stessa unità: resta anch'essa (R25 f).
	CodiceMotoreLettureAnnidate = "motore.letture_annidate"

	// CodiceMotoreIDNomeDiscordi — avviso, dati. L'id e il nome dello stesso nodo STEP danno letture diverse:
	// restano tutte, con la qualità da_verificare (P1 §10.5).
	CodiceMotoreIDNomeDiscordi = "motore.id_nome_discordi"

	// CodiceMotoreRipetizioniDiscordanti — avviso, dati. Una lettura con la base ripetuta che non concorda
	// (D2): la lettura resta, da verificare, senza scegliere la prima.
	CodiceMotoreRipetizioniDiscordanti = "motore.ripetizioni_discordanti"

	// CodiceMotorePertinenzaIgnota — nota, dati. Letture con funzione richiesta da un segmento di cui non si sa
	// se è pertinente (uso sconosciuto o da valutare, router-1 riga 2): la richiesta è da confermare (P1 §4.3).
	CodiceMotorePertinenzaIgnota = "motore.pertinenza_ignota"

	// CodiceRevisioneFormazioneNonConfrontabile — nota, dati. Il documento ha formazioni STEP: dati grezzi,
	// mai confrontati con le revisioni lette (E-12, D1). Una per documento, con gli ID delle formazioni e delle
	// revisioni.
	CodiceRevisioneFormazioneNonConfrontabile = "revisione.formazione_non_confrontabile"

	// CodiceRevisioneNonInterpretabile — avviso, dati. Una revisione in campo separato che la regola della
	// famiglia non legge per intero, letta come token sospeso, o con la regola riservata: l'originale si
	// conserva, nessun valore (P1 §3.4; C-34).
	CodiceRevisioneNonInterpretabile = "revisione.non_interpretabile"

	// CodiceRevisioneDiscordante — avviso, dati. La revisione in campo separato e una revisione in linea della
	// stessa entità non concordano (ConfrontaRevisioni): si conservano tutte e due (P1 §5.2).
	CodiceRevisioneDiscordante = "revisione.discordante"

	// CodiceQuantitaNonInterpretabile — nota, dati. La cella della colonna quantità non è un intero di cifre
	// decimali (da 1 a 6): il grezzo resta, nessun valore (R28 a).
	CodiceQuantitaNonInterpretabile = "quantita.non_interpretabile"
)
