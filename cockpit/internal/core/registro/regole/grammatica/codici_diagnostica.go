package grammatica

// I codici che questo pacchetto produce (R41 b; piano A, par.4.4.6). Ogni pacchetto del motore A dichiara
// i suoi in un file con questo nome; un pacchetto che sta sopra può emettere un codice di qui con la sua
// costante (motorea con limite.superato), o passare avanti le diagnostiche che riceve, ma non lo ridichiara
// mai. Qui si passano avanti quelle di evidenze.LeggiSelettore (contratto.selettore_non_ammesso), senza
// ridichiararle. Il prefisso dice di che cosa parla il codice, non quale pacchetto lo dichiara. Un codice
// pubblicato non cambia nome né significato; se non serve più resta qui, con il commento «ritirato».
//
// Nel commento di ogni costante: gravità predefinita, natura, quando.

// La porta stretta di lettura (Decodifica, e la stessa per l'indice). Tutti errori di contratto.
const (
	// CodiceJSONNonValido — errore, contratto. JSON malformato, un valore del tipo sbagliato per il campo,
	// un annidamento troppo profondo, un UUID illeggibile.
	CodiceJSONNonValido = "contratto.json_non_valido"

	// CodiceBOM — errore, contratto. La firma UTF-8 (BOM) in testa al file.
	CodiceBOM = "contratto.bom"

	// CodiceUTF8NonValido — errore, contratto. Byte non UTF-8, o un surrogato solo scritto in escape
	// («\ud800»), che encoding/json cambierebbe in silenzio in U+FFFD.
	CodiceUTF8NonValido = "contratto.utf8_non_valido"

	// CodiceValoreDopoOggetto — errore, contratto. Testo dopo l'oggetto: il file contiene un valore solo.
	CodiceValoreDopoOggetto = "contratto.valore_dopo_oggetto"

	// CodiceChiaveSconosciuta — errore, contratto. Una chiave che non è un tag del DTO, anche un campo legacy
	// «non coinvolto» di cliente.regole (R20 c).
	CodiceChiaveSconosciuta = "contratto.chiave_sconosciuta"

	// CodiceChiaveRipetuta — errore, contratto. Due chiavi uguali nello stesso oggetto.
	CodiceChiaveRipetuta = "contratto.chiave_ripetuta"

	// CodiceChiaveMaiuscole — errore, contratto. Una chiave uguale a un tag solo ignorando le maiuscole, che
	// encoding/json accetterebbe in silenzio (T21).
	CodiceChiaveMaiuscole = "contratto.chiave_maiuscole"

	// CodiceNull — errore, contratto. Un null, in qualunque posizione.
	CodiceNull = "contratto.null"

	// CodiceNumeroNonIntero — errore, contratto. Un numero con decimali o con l'esponente.
	CodiceNumeroNonIntero = "contratto.numero_non_intero"

	// CodiceVersioneSchemaIgnota — errore, contratto. versione_schema (o versione_indice) assente o diversa
	// da quella che il codice sa leggere: diagnosi sì, attivazione no (parte 1 §7.1).
	CodiceVersioneSchemaIgnota = "contratto.versione_schema_ignota"
)

// La validazione delle grammatiche (Grammatica.Valida) e dei limiti (Limiti.Valida). Errori di contratto.
const (
	// CodiceCampoObbligatorio — errore, contratto. Manca un campo obbligatorio: anche i limiti dell'indice o
	// uno dei loro valori (R43 B), o un esempio positivo di una forma attiva.
	CodiceCampoObbligatorio = "contratto.campo_obbligatorio"

	// CodiceEnumIgnoto — errore, contratto. Un valore fuori da un enum chiuso, o una combinazione che il tipo
	// non ammette (un sottotipo fuori da un involucro, letterale e pattern insieme).
	CodiceEnumIgnoto = "contratto.enum_ignoto"

	// CodiceIDVuoto — errore, contratto. Un ID (o un nome di segmento) mancante.
	CodiceIDVuoto = "contratto.id_vuoto"

	// CodiceIDRipetuto — errore, contratto. Un ID ripetuto fra gli elementi dello stesso tipo, nella stessa
	// famiglia; una famiglia ripetuta nel file.
	CodiceIDRipetuto = "contratto.id_ripetuto"

	// CodiceRiferimentoPendente — errore, contratto. Un rif che non si risolve nella stessa famiglia e nel
	// tipo giusto; una lettura attesa con una famiglia o una forma che non c'è.
	CodiceRiferimentoPendente = "contratto.riferimento_pendente"

	// CodiceInsiemeConDuplicati — errore, contratto. Duplicati in ruoli, categorie, selettori, letterali.
	CodiceInsiemeConDuplicati = "contratto.insieme_con_duplicati"

	// CodiceRuoliVuoti — errore, contratto. Una famiglia senza ruoli (R17 c).
	CodiceRuoliVuoti = "contratto.ruoli_vuoti"

	// CodiceAttribuzioneFuoriRiconoscimento — errore, contratto. Un selettore di attribuzione che non è fra
	// quelli di riconoscimento (parte 1 §5.3).
	CodiceAttribuzioneFuoriRiconoscimento = "contratto.attribuzione_fuori_riconoscimento"

	// CodiceParteFuoriSelettore — errore, contratto. Una parte che non è dichiarata su un selettore della
	// sua forma.
	CodiceParteFuoriSelettore = "contratto.parte_fuori_selettore"

	// CodiceFormaParzialeNonProiezione — errore, contratto. Una forma parziale incoerente: completa falso
	// senza segmenti mancanti, completa vero con segmenti mancanti, un mancante che non è un segmento
	// identitario della base.
	CodiceFormaParzialeNonProiezione = "contratto.forma_parziale_non_proiezione"

	// CodiceCardinalitaNonValida — errore, contratto. Min o Max non ammessi; una forma con più di una base;
	// spazi_max negativo.
	CodiceCardinalitaNonValida = "contratto.cardinalita_non_valida"

	// CodiceEtichettaObbligatoriaAssente — errore, contratto. Una forma senza l'etichetta obbligatoria della
	// famiglia, con Min 1 (A-C08).
	CodiceEtichettaObbligatoriaAssente = "contratto.etichetta_obbligatoria_assente"

	// CodiceSelettoreConteso — errore, contratto. Uno stesso selettore, nella stessa famiglia, con forme e
	// con una revisione in campo separato.
	CodiceSelettoreConteso = "contratto.selettore_conteso"

	// CodiceConfineNonAmmesso — errore, contratto. Una classe *_punto_cifra come confine sinistro.
	CodiceConfineNonAmmesso = "contratto.confine_non_ammesso"
)

// I pattern RE2 e i letterali (VerificaPattern, Valida). Errori di contratto.
const (
	// CodicePatternNonCompila — errore, contratto. regexp/syntax rifiuta il pattern.
	CodicePatternNonCompila = "grammatica.pattern_non_compila"

	// CodicePatternOperatoreNonRE2 — errore, contratto. Lookaround, backreference, gruppi atomici,
	// ripetizioni possessive: RE2 non li ha (T1).
	CodicePatternOperatoreNonRE2 = "grammatica.pattern_operatore_non_re2"

	// CodicePatternConfine — errore, contratto. \b o \B: il confine lo controlla il motore (T3).
	CodicePatternConfine = "grammatica.pattern_confine"

	// CodicePatternAncora — errore, contratto. ^, $, \A, \z (T4).
	CodicePatternAncora = "grammatica.pattern_ancora"

	// CodicePatternGruppo — errore, contratto. Qualunque gruppo, anche (?:…) e con nome: i gruppi li genera
	// il compilatore (T12).
	CodicePatternGruppo = "grammatica.pattern_gruppo"

	// CodicePatternFlag — errore, contratto. Flag in linea, come (?i): le maiuscole si dichiarano nella base
	// (T9).
	CodicePatternFlag = "grammatica.pattern_flag"

	// CodicePatternPunto — errore, contratto. «.» o una classe negata (T11).
	CodicePatternPunto = "grammatica.pattern_punto"

	// CodicePatternClasseNonASCII — errore, contratto. Una classe con caratteri fuori dall'ASCII, come le
	// classi Unicode (T10).
	CodicePatternClasseNonASCII = "grammatica.pattern_classe_non_ascii"

	// CodicePatternAlternanza — errore, contratto. «|» nel pattern: le alternative si scrivono come letterali.
	CodicePatternAlternanza = "grammatica.pattern_alternanza"

	// CodicePatternRipetizioneIllimitata — errore, contratto. *, +, {n,}.
	CodicePatternRipetizioneIllimitata = "grammatica.pattern_ripetizione_illimitata"

	// CodicePatternRipetizioneOltreLimite — errore, contratto. {n,m} con m oltre max_ripetizione dei limiti
	// dell'indice (T8).
	CodicePatternRipetizioneOltreLimite = "grammatica.pattern_ripetizione_oltre_limite"

	// CodicePatternVuoto — errore, contratto. Il pattern può riconoscere la stringa vuota (T7).
	CodicePatternVuoto = "grammatica.pattern_vuoto"

	// CodiceCarattereDiControllo — errore, contratto. Un carattere di controllo o di formato, o uno spazio
	// che non è U+0020, in un pattern o in un letterale; nel pattern anche U+0020. Il «\b» di un JSON è un
	// backspace (T20).
	CodiceCarattereDiControllo = "grammatica.carattere_di_controllo"

	// CodiceLetteraleVuoto — errore, contratto. Un letterale vuoto.
	CodiceLetteraleVuoto = "grammatica.letterale_vuoto"
)

// Capacità, riserve e limiti.
const (
	// CodiceCapacitaNonSupportata — avviso, capacità. Un elemento noto ma riservato in A1 (R20 c; capacita.go),
	// anche un selettore generico «<contesto>.*» (D-12): non è attivo, il resto della grammatica sì (E-23).
	CodiceCapacitaNonSupportata = "capacita.non_supportata"

	// CodiceFormaRiservata — nota, capacità. Una forma, o una regola di revisione, con stato «riservata»:
	// dichiarata e non attiva.
	CodiceFormaRiservata = "grammatica.forma_riservata"

	// CodiceRiserva — nota, capacità. Una voce di profilo.riserve: la diagnosi rende visibile ciò che il file
	// non attiva.
	CodiceRiserva = "grammatica.riserva"

	// CodiceLimiteSuperato — errore in validazione, limite. Un limite dell'indice superato (R43 B). motorea lo
	// emette, come avviso, con questa stessa costante: in Riconosci il risultato è parziale, mai un successo
	// vuoto (E-23).
	CodiceLimiteSuperato = "limite.superato"

	// CodiceLimiteOltreTetto — errore, contratto. Un valore dei limiti dell'indice oltre il tetto del codice:
	// nessuna grammatica si attiva (R43 B).
	CodiceLimiteOltreTetto = "limite.oltre_tetto"
)

// L'indice delle regole (LeggiIndice, ControllaRagioneSociale).
const (
	// CodiceRegoleClienteRipetuto — errore, contratto. Un cliente due volte nell'indice: nessuna grammatica si
	// attiva.
	CodiceRegoleClienteRipetuto = "regole.cliente_ripetuto"

	// CodiceRegoleRagioneSocialeDiscorde — avviso, dati. La ragione sociale della grammatica è diversa da
	// quella del DB (spazi ai bordi e maiuscole ASCII non contano): il thread non si valuta (par.3.6.3).
	CodiceRegoleRagioneSocialeDiscorde = "regole.ragione_sociale_discorde"
)
