package propostabom

// I codici che questo pacchetto produce (R41 b; piano A, 6.4.10). Ogni pacchetto del motore A dichiara i suoi in un file
// con questo nome; un codice pubblicato non cambia nome né significato, e se non serve più resta qui con il commento
// «ritirato». L'area «importazione» dice di che cosa parla il codice: il registro d'importazione che l'adattatore
// costruisce (emendamento di V1 §3.8, §6.1, §11.2). Stanno fra le diagnostiche del registro (proposta.diagnostiche),
// mai nell'esito della valutazione.

// Le diagnostiche dell'adattatore (ImportaGruppo).
const (
	// CodiceImportazioneSenzaGrammatica — avviso, dati. Il thread non ha la grammatica del cliente (non valutato per la
	// grammatica assente, scartata o con la ragione sociale discorde): l'importazione è ammessa, con la struttura senza
	// identità, e i codici restano da assegnare (emendamento di V1 §6.1, «Errori dell'importazione»).
	CodiceImportazioneSenzaGrammatica = "importazione.senza_grammatica"

	// CodiceImportazioneNamespaceIncoerente — avviso, dati. Lo stesso namespace compare nel registro con due emittenti
	// diversi: i namespace sono unici per emittente, e il registro lo dice senza scegliere (emendamento di V1 §6.1).
	CodiceImportazioneNamespaceIncoerente = "importazione.namespace_incoerente"

	// CodiceImportazioneRelazioneIncompleta — avviso, dati. Una relazione del motore cita una lettura che
	// l'interpretazione del suo file non ha: la relazione non entra nel registro, perché ogni lettura citata da una
	// relazione deve starci (emendamento di V1 §3.8, «proposta.relazioni[]»).
	CodiceImportazioneRelazioneIncompleta = "importazione.relazione_incompleta"

	// CodiceImportazioneGrafoDaRileggere — avviso, dati. La struttura del gruppo è stata letta da un worker non aggiornato
	// (il motivo della regressione v4 della 0020): il registro tiene grafo_completo falso e il motivo come sono, e dopo
	// l'aggiornamento serve «Rifai l'analisi» (emendamento di V1 §11.2; LS-01).
	CodiceImportazioneGrafoDaRileggere = "importazione.grafo_da_rileggere"

	// CodiceImportazioneEvidenzeNonAttive — nota, dati. Delle evidenze del motore (proposte d'associazione, prove di un
	// equivalente K4, codici documentali, candidati di revisione, relazioni) venivano da un'occorrenza in «Altri file» o
	// scartata: non entrano nel registro, e la nota le conta, con gli allegati (emendamento di V1 §3.8, «proposta.file[]»;
	// A3 §1.4; direttiva 23 §5.2).
	CodiceImportazioneEvidenzeNonAttive = "importazione.evidenze_file_non_attivi"
)
