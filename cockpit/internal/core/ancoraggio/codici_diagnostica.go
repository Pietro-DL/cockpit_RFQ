package ancoraggio

// I codici che questo pacchetto produce (R41 b; piano A, 6.4.10). Ogni pacchetto del motore A dichiara i suoi in
// un file con questo nome; un codice pubblicato non cambia nome né significato, e se non serve più resta qui con il
// commento «ritirato». L'area «ancoraggio» dice di che cosa parla il codice. I codici degli ancoraggi dei file li
// aggiunge il commit che li produce (P6b: la riconciliazione).

// I prodotti della mail (ProponiProdotti, P5).
const (
	// CodiceRichiestaNonValutata — avviso, dati. Un messaggio della richiesta senza il gesto 1 dell'operatore (stato
	// da_valutare o non_valutabile) ha dei candidati: restano da confermare, con il motivo richiesta_non_valutata
	// (6.4.4, regola 1; R29 c). Il triage non lo cambia.
	CodiceRichiestaNonValutata = "ancoraggio.richiesta_non_valutata"

	// CodiceRigheStessaBase — avviso, dati. La stessa base, con gli stessi qualificatori, in più righe di tabella
	// dello stesso segmento: un candidato per riga, ognuno con la sua quantità, nessuna fusione (6.4.4, regola 3; D3).
	CodiceRigheStessaBase = "ancoraggio.righe_stessa_base"

	// CodiceQuantitaNonIntera — avviso, dati. La riga di un candidato ha la colonna della quantità dichiarata, ma la
	// cella non dà un intero (non interpretabile, o colonna ambigua): la quantità resta nil (6.4.4, regola 4; R28 a).
	CodiceQuantitaNonIntera = "ancoraggio.quantita_non_intera"

	// CodiceAlternativeConservate — avviso, dati. Due letture di prodotto, di famiglie o forme diverse, sulla stessa
	// occorrenza: restano tutte e due, ognuna con l'altra fra le alternative; nessuna si sceglie (6.4.4, regola 6).
	CodiceAlternativeConservate = "ancoraggio.alternative_conservate"
)

// Le strutture dei prodotti (ProponiStrutture, P6a). Gli ancoraggi dei file (B4) usano gli stessi codici come motivi
// della collocazione non determinabile (6.4.5, regola 3).
const (
	// CodiceTargetSenzaStruttura — avviso, dati. Un prodotto target senza nessuna struttura: nessun nodo degli STEP
	// letti ha la sua base (o il suo codice non è letto), e nessuna fonte confermata dà una radice che si trova fra le
	// strutture. Il target resta un target: non si conclude che il prodotto non esiste (6.4.5, regola 3; 6.10 n.16).
	CodiceTargetSenzaStruttura = "ancoraggio.target_senza_struttura"

	// CodiceGrafoIncompleto — avviso, dati. Una struttura di un target sta in uno STEP con il grafo non completo
	// (la capacità grafo_completo, con il motivo della 0020: R32 b): la mancanza di un arco non è un'informazione.
	// Contano solo le strutture dei target, mai i file «solo parti» dei figli (6.4.5, regola 2).
	CodiceGrafoIncompleto = "ancoraggio.grafo_incompleto"

	// CodiceRadiceDalNomeFile — nota, dati. Un target senza strutture prende come struttura candidata la radice, l'unica,
	// di uno STEP il cui nome dice il target, quando la radice non ha letture nel namespace del target (muta, o letta
	// in un altro namespace): StrutturaProdotto.Origine nome_file_step (estensione di R76 b, notte 14/10). Resta una
	// candidata: la BOM di lavoro viene solo dal gesto 3 (R59 A, R76 A).
	CodiceRadiceDalNomeFile = "ancoraggio.radice_dal_nome_file"

	// CodiceRadiceDalloSTEPAssociato — nota, dati. Come CodiceRadiceDalNomeFile, con la prova di un'associazione decisa
	// (manuale o confermata) dello STEP al componente del target: StrutturaProdotto.Origine associazione_step.
	CodiceRadiceDalloSTEPAssociato = "ancoraggio.radice_dallo_step_associato"
)

// La riconciliazione fra lo STEP e il cartiglio (ProponiAncoraggi, P6b, B4 fase 3; contratto §2.2, R41 b): una per
// codice documentale con la correzione accanto, in avviso. Niente si applica: né il componente, né l'ancoraggio, né
// l'identità del nodo cambiano (R64 A, R87).
const (
	// CodiceCompletamentoDocumentale — avviso, dati. Il cartiglio di un 2D associato a un nodo ha la base e il
	// marcatore dell'identità proposta dallo STEP e aggiunge la revisione che lo STEP non dà: completamento_proposto
	// (R87, T-B0-27), o discordante con il completamento accanto quando il file ha identità che non concordano.
	CodiceCompletamentoDocumentale = "ancoraggio.completamento_documentale"

	// CodiceCorrezioneDocumentale — avviso, dati. Il cartiglio di un 2D associato a un nodo dice un altro codice
	// (un'altra base, un altro marcatore scritto, un'altra revisione): correzione_proposta, o discordante con la
	// correzione accanto quando il file ha identità che non concordano (R64 A, T-B0-35).
	CodiceCorrezioneDocumentale = "ancoraggio.correzione_documentale"
)

// Il segnale LS-02 sui nodi degli STEP (V1; elementi_cad.go; addendum ad A1c §2.2): non è una diagnostica ma il codice
// di un SegnaleNodo, e come ogni codice che il pacchetto produce sta qui (R41 b; decisione P-1 (a) dell'ondata A di V1).
// Lo copiano il registro d'importazione (nodi[].segnali[]) e la decisione «non includere» (segnale_visto): una volta
// pubblicato non cambia nome né significato.
const (
	// CodiceSegnaleElementoCAD — segnale. Un nodo di uno STEP che ha la forma di un elemento CAD (un cordone di
	// saldatura o simile) e non di un articolo: la regola elemento_cad_nome_foglia_v1. Solo un segnale, spiegato: non
	// toglie il nodo, non lo esenta dal 2D, non decide.
	CodiceSegnaleElementoCAD = "ancoraggio.possibile_elemento_cad"
)

// Le equivalenze del cartiglio (ProponiAncoraggi, K4; equivalenze.go): un cartiglio che scrive il codice della parte e
// il documento 3D della stessa parte (cartiglio.riferimento_3d) propone il candidato sul nodo del documento 3D, con le
// prove. Niente si conferma: il candidato resta proposto (T-B0-09).
const (
	// CodiceEquivalenzaCartiglioContesa — avviso, dati. Un'equivalenza del cartiglio non si propone perché non è
	// univoca: un cartiglio con più codici della parte o più riferimenti 3D, uno stesso riferimento 3D accanto a due
	// codici della parte, o uno stesso codice accanto a due riferimenti 3D nei cartigli del thread. Le coppie coinvolte
	// cadono tutte. Più identità nel nome del file, quando il cartiglio non dice il codice, non sono una contesa: il
	// ripiego sul nome non propone e non lo dice. Rif: le identità (RifIdentita senza revisione) e gli allegati.
	CodiceEquivalenzaCartiglioContesa = "ancoraggio.equivalenza_cartiglio_contesa"
)
