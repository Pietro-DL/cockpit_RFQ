package estrazione

import (
	"fmt"
	"strings"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/inbox/lettura"
)

// La mappatura della mail (mappatura-email-1; 5.4.5). Di classificazione e lettura si usano solo i nomi
// dell'elenco chiuso di G4: il taglio con le posizioni e i livelli della storia, EInoltro, le tabelle con la
// loro origine e TestoDaHTML. I confini, le righe e le celle vengono da lì; qui diventano segmenti, entità e
// unità, senza nessuna regola nuova e senza decidere quali segmenti valgono come richiesta: quello è
// UsoSegmenti, un ingresso del motore.
const (
	testoOggetto  = "messaggio.oggetto"
	testoCorpo    = "messaggio.corpo_testo"   // anche il testo su cui la foglia misura PosTabella.Esatto
	testoDaHTML   = "messaggio.testo_da_html" // il testo ricavato dall'HTML, quando il corpo di testo manca
	origineDaHTML = "derivato_da_html"
	campoHTML     = "messaggio.corpo_html"

	idUnitaOggetto  = "u:oggetto"
	origineMittente = "mittente_del_messaggio"

	capFirma          = "firma"
	capSezioneTecnica = "sezione_tecnica"
	capStoriaAnnidata = "storia_annidata"
	capTabelle        = "tabelle"
	capSegmentazione  = "segmentazione"

	tipoEntitaRiga         = "riga"
	tipoLegameIntestazione = "intestazione_di_cella"

	// limiteTestoMail e limiteHTMLMail: i limiti della vista (lettura.LimiteTesto e lettura.LimiteHTML,
	// lettura.go:35-36), oltre i quali TabelleConOrigine e TestoDaHTML non leggono niente. G4 non ammette le
	// due costanti, quindi il valore è ripetuto qui, e una prova lo confronta con quello di lettura: servono
	// solo a dire PERCHÉ le tabelle mancano, non a decidere che cosa si legge.
	limiteTestoMail = 1 << 20
	limiteHTMLMail  = 2 << 20

	// motivoCopertura: il limite dichiarato dei riconoscitori dei confini (REG §5.9, «Lingue»; grafo §B.5). Non
	// è «nessuna storia» verificata: un confine in una lingua non coperta lascia il testo nel segmento in cui
	// sta (R48 A).
	motivoCopertura = "i riconoscitori dei confini coprono italiano, inglese e tedesco, il francese in parte, lo spagnolo no: " +
		"un confine in una lingua non coperta lascia il testo nel segmento in cui sta"
	motivoNonSeparabileConfine = "un confine riconosciuto e la parte corrente vuota: tutto ciò che segue il confine resta storia"
	motivoNonSeparabileInoltro = "indizio d'inoltro nell'oggetto, nessun confine nel corpo"
	motivoDaHTML               = "testo ricavato dall'HTML (TestoDaHTML): nessun offset sul corpo_testo"
	motivoCellaParziale        = "testo della cella com'è nell'HTML: le sue parole non coincidono, da un capo all'altro, con le sue righe del testo"
)

// segmentoMail: un segmento della mail con il contesto che dà alle sue unità e le righe del Taglio che copre.
// Serve a dire in quale segmento sta una tabella: le righe della tabella devono stare tutte in uno.
type segmentoMail struct {
	id    string
	ctx   evidenze.Contesto
	righe [2]int
}

// lettoreMail: lo stato dell'adattatore della mail mentre costruisce il documento.
type lettoreMail struct {
	c        *documento
	corpo    string // il testo su cui si misura tutto: corpo_testo, o il testo ricavato dall'HTML
	idCorpo  string // testoCorpo o testoDaHTML
	derivato bool   // il corpo viene dall'HTML: nessuna localizzazione esatta
	taglio   classificazione.Taglio
	segmenti []segmentoMail
}

// DaMessaggio costruisce il documento di un messaggio (5.4.5, «Mappatura mail»): oggetto, segmenti con gli
// offset sul corpo_testo come sta nel DB, livelli della storia, tabelle HTML agganciate. Non decide quali
// segmenti sono pertinenti: quello è UsoSegmenti, un ingresso di Interpreta.
//   - Il corpo assente con l'HTML presente: il testo è quello di TestoDaHTML (origine «derivato_da_html»), con la
//     localizzazione parziale e email.testo_da_html.
//   - L'oggetto è l'unità u:oggetto del segmento s:corrente, che esiste sempre, anche vuoto.
//   - La storia: un segmento per livello (LivelliDellaStoria), s:storia:<k>, «inoltro» o «citazione» secondo la
//     riga che lo apre (R27 c), con PadreID il livello sopra; al più maxLivelliStoria livelli.
//   - Un oggetto con il prefisso d'inoltro e un corpo senza nessun confine danno la storia non separabile: tutto
//     il corpo è s:storia:1, tipo «inoltro», contesto storia, e nessuna parte diventa corrente da sola (R48 A).
//     La richiesta, se c'è, la dice il file dei casi o l'operatore, mai l'adattatore.
//   - Le tabelle: un'entità «riga» per riga e un'unità per cella non vuota, solo per le tabelle che si agganciano
//     al testo dentro un solo segmento.
//
// È un errore, mai un documento: dei livelli della storia che non tornano con il taglio, un documento che non
// passa ValidaDocumento.
func DaMessaggio(m fotorfq.Messaggio) (evidenze.DocumentoEvidenze, error) {
	fonte, err := fonteDiMessaggio(m)
	if err != nil {
		return evidenze.DocumentoEvidenze{}, err
	}
	c := nuovoDocumento(fonte)
	c.d.Qualita.Metodo = metodoAdattatore
	c.d.Qualita.Mappatura = mappaturaVerificata

	oggetto, html := valoreDi(m.Oggetto), valoreDi(m.CorpoHTML)
	l := &lettoreMail{c: c, corpo: valoreDi(m.CorpoTesto), idCorpo: testoCorpo}
	if l.corpo == "" {
		if t := lettura.TestoDaHTML(html); t != "" {
			l.corpo, l.idCorpo, l.derivato = t, testoDaHTML, true
		}
	}

	if m.Oggetto != nil {
		c.testo(testoOggetto, oggetto, testoOggetto)
	}
	if l.derivato {
		c.testo(testoDaHTML, l.corpo, origineDaHTML)
		c.diagnostica(evidenze.Diagnostica{
			Codice:    CodiceEmailTestoDaHTML,
			Gravita:   evidenze.GravitaNota,
			Natura:    evidenze.NaturaDati,
			Percorso:  campoHTML,
			Messaggio: "il corpo di testo manca: il testo viene dall'HTML, con le regole della vista, e le unità non hanno un offset sul corpo_testo",
		})
	} else {
		c.testo(testoCorpo, l.corpo, testoCorpo)
	}

	l.taglio = classificazione.TagliaCatenaConPosizioni(l.corpo)
	// R48 A: il taglio non ha trovato nessun confine, ma l'oggetto dice «inoltro» (EInoltro, con nessuna storia,
	// è vero solo per il prefisso dell'oggetto). Un corpo vuoto non ha una storia da dichiarare.
	inoltroSenzaConfine := l.taglio.Stato == classificazione.StatoNessunaStoria &&
		l.taglio.CorrenteUtile[0] < l.taglio.CorrenteUtile[1] && classificazione.EInoltro(oggetto, l.corpo)

	l.corrente(oggetto, m.Oggetto != nil, inoltroSenzaConfine)
	if err := l.storia(inoltroSenzaConfine); err != nil {
		return evidenze.DocumentoEvidenze{}, fmt.Errorf("estrazione: messaggio %s: %w", m.ID, err)
	}
	l.capacitaDellaMail(inoltroSenzaConfine)
	l.tabelle(html)
	return c.chiudi()
}

// valoreDi: il testo di una colonna, "" se assente. L'assenza resta visibile nell'impronta dei testi
// (fonteDiMessaggio), non nel testo.
func valoreDi(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// posCorpo: il localizzatore di un intervallo del corpo.
func (l *lettoreMail) posCorpo(iv evidenze.Intervallo) evidenze.Localizzatore {
	return evidenze.Localizzatore{Tipo: "testo", Testo: &evidenze.PosTesto{TestoID: l.idCorpo, Intervallo: iv}}
}

// unitaDelCorpo aggiunge l'unità di un segmento, con il testo dell'intervallo dato (già senza gli spazi ai
// bordi). Un intervallo vuoto non dà unità: un segmento senza testo resta senza unità.
func (l *lettoreMail) unitaDelCorpo(id, segmento string, ctx evidenze.Contesto, iv evidenze.Intervallo) {
	if iv.Inizio >= iv.Fine {
		return
	}
	q := evidenze.QualitaUnita{Localizzazione: localizzazioneEsatta, Metodo: metodoAdattatore}
	if l.derivato {
		q = evidenze.QualitaUnita{Localizzazione: localizzazioneParziale, Motivo: motivoDaHTML, Metodo: metodoAdattatore}
	}
	l.c.unita(evidenze.UnitaEvidenza{
		ID:             id,
		SegmentoID:     segmento,
		Selettore:      selettoreDi(ctx),
		Testo:          l.corpo[iv.Inizio:iv.Fine],
		CampoOriginale: evidenze.CampoOriginale{Parser: l.idCorpo, Mappatura: mappaturaEmail},
		Posizione:      l.posCorpo(iv),
		Qualita:        q,
	})
}

// corrente aggiunge il segmento s:corrente (messaggio logico m0, origine il mittente), l'oggetto e la parte
// corrente. Il segmento esiste sempre, anche vuoto, perché l'oggetto vi si appoggia; con la storia non
// separabile per l'indizio d'inoltro è vuoto all'inizio del corpo, e tutto il corpo va nella storia.
func (l *lettoreMail) corrente(oggetto string, conOggetto, inoltroSenzaConfine bool) {
	t := l.taglio
	iv, righe := t.Corrente, [2]int{0, len(t.Righe)}
	switch {
	case inoltroSenzaConfine:
		iv, righe = [2]int{0, 0}, [2]int{0, 0}
	case t.RigaTaglio >= 0:
		righe = [2]int{0, t.RigaTaglio}
	}
	l.c.segmento(evidenze.Segmento{ID: idSegCorrente, Tipo: evidenze.SegmentoCorrente, MessaggioLogicoID: messaggioCorrente,
		Origine: origineMittente, Posizione: l.posCorpo(daCoppia(iv))})
	l.segmenti = append(l.segmenti, segmentoMail{id: idSegCorrente, ctx: evidenze.ContestoCorpo, righe: righe})

	if conOggetto && oggetto != "" {
		l.c.unita(evidenze.UnitaEvidenza{
			ID:             idUnitaOggetto,
			SegmentoID:     idSegCorrente,
			Selettore:      selettoreDi(evidenze.ContestoOggetto),
			Testo:          oggetto,
			CampoOriginale: evidenze.CampoOriginale{Parser: testoOggetto, Mappatura: mappaturaEmail},
			Posizione:      evidenze.Localizzatore{Tipo: "testo", Testo: &evidenze.PosTesto{TestoID: testoOggetto, Intervallo: intero(oggetto)}},
			Qualita:        evidenze.QualitaUnita{Localizzazione: localizzazioneEsatta, Metodo: metodoAdattatore},
		})
	}
	if !inoltroSenzaConfine {
		l.unitaDelCorpo("u:corpo:"+idSegCorrente, idSegCorrente, evidenze.ContestoCorpo, daCoppia(t.CorrenteUtile))
	}
}

// storia aggiunge i segmenti della storia.
//   - Storia non separabile per l'indizio d'inoltro (R48 A): un solo segmento s:storia:1 su tutto il corpo, tipo
//     «inoltro», messaggio logico m1. LivelliDellaStoria non si applica: nel corpo non c'è nessun confine.
//   - Con un confine: i livelli di LivelliDellaStoria, al più maxLivelliStoria. Oltre, il resto resta
//     nell'ultimo livello, con email.livelli_oltre_limite (5.4.3 punto 6). Il tipo viene solo dalla riga che apre
//     il livello: «inoltro» per una frase d'inoltro, «citazione» per ogni altro confine (R27 c).
func (l *lettoreMail) storia(inoltroSenzaConfine bool) error {
	t := l.taglio
	var livelli []classificazione.LivelloStoria
	switch {
	case inoltroSenzaConfine:
		livelli = []classificazione.LivelloStoria{{Righe: [2]int{0, len(t.Righe)}, Byte: [2]int{0, len(l.corpo)}, Inoltro: true}}
	case t.Stato == classificazione.StatoNessunaStoria:
		l.c.capacita(capStoriaAnnidata, statoDisponibile, fmt.Sprintf("livelli con le regole del taglio di oggi (R27 b), al più %d", maxLivelliStoria))
		return nil
	default:
		livelli = classificazione.LivelliDellaStoria(l.corpo, t, maxLivelliStoria+1)
		if len(livelli) == 0 {
			return fmt.Errorf("il taglio ha una storia, ma i suoi livelli non si leggono")
		}
	}

	annidata, motivo := statoDisponibile, fmt.Sprintf("livelli con le regole del taglio di oggi (R27 b), al più %d", maxLivelliStoria)
	if len(livelli) > maxLivelliStoria {
		livelli = livelli[:maxLivelliStoria]
		ultimo := &livelli[maxLivelliStoria-1]
		ultimo.Righe[1], ultimo.Byte[1] = len(t.Righe), len(l.corpo)
		annidata, motivo = statoParziale, fmt.Sprintf("più di %d livelli: il resto resta nell'ultimo", maxLivelliStoria)
		l.c.peggiora(statoParziale)
		l.c.diagnostica(evidenze.Diagnostica{
			Codice:    CodiceEmailLivelliOltreLimite,
			Gravita:   evidenze.GravitaAvviso,
			Natura:    evidenze.NaturaLimite,
			Percorso:  l.idCorpo,
			Messaggio: fmt.Sprintf("la storia ha più di %d livelli: dal livello %d in poi resta tutto nell'ultimo", maxLivelliStoria, maxLivelliStoria),
			Rif:       []string{idStoria(maxLivelliStoria)},
		})
	}
	l.c.capacita(capStoriaAnnidata, annidata, motivo)

	for i, lv := range livelli {
		k := i + 1
		s := evidenze.Segmento{
			ID:                idStoria(k),
			Tipo:              evidenze.SegmentoCitazione,
			MessaggioLogicoID: fmt.Sprintf("m%d", k),
			Origine:           origineIgnota,
			Posizione:         l.posCorpo(daCoppia(lv.Byte)),
		}
		if lv.Inoltro {
			s.Tipo = evidenze.SegmentoInoltro
		}
		if k > 1 {
			s.PadreID = idStoria(k - 1)
		}
		l.c.segmento(s)
		l.segmenti = append(l.segmenti, segmentoMail{id: s.ID, ctx: evidenze.ContestoStoria, righe: lv.Righe})
		l.unitaDelCorpo("u:storia:"+s.ID, s.ID, evidenze.ContestoStoria, ripulito(l.corpo, lv.Byte))
	}
	return nil
}

// idStoria: l'ID del livello k della storia, numerato anche il primo (5.4.5 [+3]).
func idStoria(k int) string { return fmt.Sprintf("s:storia:%d", k) }

// capacitaDellaMail: le capacità che la mail dichiara sempre (5.4.5, «Qualità della fonte»). Firma e sezione
// tecnica non hanno un riconoscitore (R27 a): un codice nella firma resta nel segmento in cui sta. La
// segmentazione è disponibile con la copertura dei riconoscitori dichiarata, parziale con la storia non
// separabile, nei due casi che il motivo distingue (E-22; R48 A).
func (l *lettoreMail) capacitaDellaMail(inoltroSenzaConfine bool) {
	l.c.capacita(capFirma, statoNonDisponibile, "nessun riconoscitore della firma (R27 a): un codice nella firma resta nel segmento in cui sta")
	l.c.capacita(capSezioneTecnica, statoNonDisponibile, "nessun riconoscitore delle sezioni tecniche: il testo resta nel segmento in cui sta")

	motivo := ""
	switch {
	case inoltroSenzaConfine:
		motivo = motivoNonSeparabileInoltro
	case l.taglio.Stato == classificazione.StatoStoriaNonSeparabile:
		motivo = motivoNonSeparabileConfine
	default:
		l.c.capacita(capSegmentazione, statoDisponibile, motivoCopertura)
		return
	}
	l.c.capacita(capSegmentazione, statoParziale, motivo)
	l.c.peggiora(statoParziale)
	l.c.diagnostica(evidenze.Diagnostica{
		Codice:    CodiceEmailStoriaNonSeparabile,
		Gravita:   evidenze.GravitaAvviso,
		Natura:    evidenze.NaturaDati,
		Percorso:  l.idCorpo,
		Messaggio: "storia non separabile: " + motivo + "; nessuna parte diventa corrente da sola",
		Rif:       []string{idSegCorrente, idStoria(1)},
	})
}

// tabelle aggiunge le tabelle dell'HTML (TabelleConOrigine) e dichiara la capacità «tabelle»: disponibile,
// parziale se una tabella non si aggancia, non disponibile senza HTML o oltre i limiti della vista.
func (l *lettoreMail) tabelle(html string) {
	switch {
	case html == "":
		l.c.capacita(capTabelle, statoNonDisponibile, "il messaggio non ha corpo HTML")
	case len(html) > limiteHTMLMail:
		l.c.capacita(capTabelle, statoNonDisponibile, fmt.Sprintf("l'HTML supera il limite di lettura della vista (%d byte)", limiteHTMLMail))
		l.c.peggiora(statoParziale)
		l.c.diagnostica(evidenze.Diagnostica{
			Codice:    CodiceEmailHTMLOltreLimite,
			Gravita:   evidenze.GravitaAvviso,
			Natura:    evidenze.NaturaLimite,
			Percorso:  campoHTML,
			Messaggio: fmt.Sprintf("l'HTML ha %d byte, oltre il limite di %d: le tabelle non si leggono", len(html), limiteHTMLMail),
		})
	case len(l.corpo) > limiteTestoMail:
		l.c.capacita(capTabelle, statoNonDisponibile, fmt.Sprintf("il testo supera il limite di lettura della vista (%d byte): le tabelle non si agganciano", limiteTestoMail))
	default:
		nonAgganciate := 0
		for i, tab := range lettura.TabelleConOrigine(l.corpo, html, l.taglio) {
			if !l.tabella(i+1, tab) {
				nonAgganciate++
			}
		}
		if nonAgganciate > 0 {
			l.c.capacita(capTabelle, statoParziale, fmt.Sprintf("tabelle dell'HTML che non si agganciano al testo in un segmento solo, e le cui celle non sono unità: %d", nonAgganciate))
		} else {
			l.c.capacita(capTabelle, statoDisponibile, "")
		}
	}
}

// diagnosticaTabella: una tabella senza unità. d porta solo il codice, scritto dal chiamante con la sua costante
// (A1a-CAT); qui si completano gravità, natura, percorso e messaggio, uguali per i due codici.
func (l *lettoreMail) diagnosticaTabella(d evidenze.Diagnostica, n int, tab lettura.TabellaOrigine, perche string) {
	html := 0
	if len(tab.Celle) > 0 {
		html = tab.Celle[0].TabellaHTML + 1
	}
	d.Gravita, d.Natura = evidenze.GravitaAvviso, evidenze.NaturaDati
	d.Percorso = fmt.Sprintf("%s.tabella[%d]", campoHTML, n)
	d.Messaggio = fmt.Sprintf("la tabella %d (<table> n. %d del documento) %s: nessuna unità, e quantità e ambito delle sue celle restano irrisolti", n, html, perche)
	l.c.peggiora(statoParziale)
	l.c.diagnostica(d)
}

// rigaTabella: le celle di una riga della tabella, raccolte prima di scrivere l'entità.
type rigaTabella struct {
	prima evidenze.PosTabella // la posizione della prima cella, per gli indici della riga
	righe *[2]int             // l'unione delle righe del testo delle sue celle
}

// intestazioneTabella: una cella con la sua unità e le colonne che copre (per i legami dell'intestazione).
type intestazioneTabella struct {
	id    string
	riga  int
	cella int
	da, a int // colonne [da, a) della griglia
}

// tabella aggiunge una tabella agganciata e restituisce vero; per una tabella non agganciata, o a cavallo di due
// segmenti, scrive la diagnostica e restituisce falso.
//   - Un'entità «riga» per riga, e:tab:<t>:r<r>, nel segmento che contiene le righe della tabella.
//   - Un'unità per cella non vuota, u:tab:<t>:r<r>:c<c>, con il selettore del segmento e l'EntitaID della riga:
//     la cella appartiene alla riga, e la riga al segmento, ognuna in un campo solo (legami-1). Con Coincide, e
//     sul corpo_testo, il testo è il tratto originale delle sue righe senza gli spazi ai bordi, esatto, con
//     PosTabella.Esatto; altrimenti il testo della cella com'è nell'HTML, parziale. Una cella è una sola
//     evidenza (R28 b).
//   - Con la riga d'intestazione che l'euristica di lettura riconosce (TabellaOrigine.Intestazione), un legame
//     intestazione_di_cella da ogni cella d'intestazione alle celle delle righe sotto che stanno nelle sue
//     colonne: lo dice la griglia dell'HTML, non la somiglianza dei testi (legami-1).
//
// Gli indici di lettura sono da 0; quelli di PosTabella e degli ID da 1. RigheTesto sono le righe del Taglio,
// [da, a), da 0.
func (l *lettoreMail) tabella(n int, tab lettura.TabellaOrigine) bool {
	if tab.ACavallo {
		l.diagnosticaTabella(evidenze.Diagnostica{Codice: CodiceEmailTabellaACavallo}, n, tab, "si ritrova nel testo solo a cavallo fra la parte corrente e la storia")
		return false
	}
	if !tab.Agganciata || tab.Righe == nil {
		l.diagnosticaTabella(evidenze.Diagnostica{Codice: CodiceEmailTabellaNonAgganciata}, n, tab, "non si ritrova nel testo, parola per parola")
		return false
	}
	seg, ok := l.segmentoDelleRighe(*tab.Righe)
	if !ok {
		l.diagnosticaTabella(evidenze.Diagnostica{Codice: CodiceEmailTabellaACavallo}, n, tab, "si ritrova nel testo a cavallo di due livelli della storia")
		return false
	}

	righe := map[int]*rigaTabella{}
	var ordine []int
	for _, ce := range tab.Celle {
		r, ok := righe[ce.Riga]
		if !ok {
			r = &rigaTabella{prima: evidenze.PosTabella{
				Tabella:     n,
				Riga:        ce.Riga + 1,
				TabellaHTML: ce.TabellaHTML + 1,
				RigaHTML:    ce.RigaHTML + 1,
				Percorso:    percorsoDellaRiga(ce.Percorso),
			}}
			righe[ce.Riga] = r
			ordine = append(ordine, ce.Riga)
		}
		if ce.Righe != nil {
			if r.righe == nil {
				r.righe = &[2]int{ce.Righe[0], ce.Righe[1]}
			} else {
				r.righe[0], r.righe[1] = min(r.righe[0], ce.Righe[0]), max(r.righe[1], ce.Righe[1])
			}
		}
	}
	for _, k := range ordine {
		r := righe[k]
		pos := r.prima
		pos.RigheTesto = r.righe
		l.c.entita(evidenze.EntitaLocale{
			ID:              idRiga(n, k),
			Tipo:            tipoEntitaRiga,
			SegmentoID:      seg.id,
			ChiaveOriginale: fmt.Sprintf("tabella %d riga %d", n, k+1),
			Posizione:       evidenze.Localizzatore{Tipo: "tabella", Tabella: &pos},
		})
	}

	var intestazioni []intestazioneTabella
	var dati []intestazioneTabella
	for _, ce := range tab.Celle {
		if ce.Testo == "" {
			continue
		}
		id := fmt.Sprintf("u:tab:%d:r%d:c%d", n, ce.Riga+1, ce.Cella+1)
		l.cella(id, idRiga(n, ce.Riga), seg, n, ce)
		col := intestazioneTabella{id: id, riga: ce.Riga, cella: ce.Cella, da: ce.Colonna, a: ce.Colonna + max(1, ce.Colspan)}
		if tab.Intestazione && ce.Riga == 0 {
			intestazioni = append(intestazioni, col)
		} else {
			dati = append(dati, col)
		}
	}
	for _, h := range intestazioni {
		for _, d := range dati {
			if d.da >= h.da && d.a <= h.a {
				l.c.legame(evidenze.LegameFonte{
					ID:   fmt.Sprintf("g:tab:%d:r%d:c%d>r%d:c%d", n, h.riga+1, h.cella+1, d.riga+1, d.cella+1),
					Tipo: tipoLegameIntestazione,
					Da:   h.id,
					A:    d.id,
				})
			}
		}
	}
	return true
}

// idRiga: l'ID dell'entità di una riga, con gli indici da 1.
func idRiga(n, riga int) string { return fmt.Sprintf("e:tab:%d:r%d", n, riga+1) }

// cella aggiunge l'unità di una cella non vuota.
func (l *lettoreMail) cella(id, riga string, seg segmentoMail, n int, ce lettura.CellaOrigine) {
	pos := evidenze.PosTabella{
		Tabella:     n,
		Riga:        ce.Riga + 1,
		Cella:       ce.Cella + 1,
		TabellaHTML: ce.TabellaHTML + 1,
		RigaHTML:    ce.RigaHTML + 1,
		Colonna:     ce.Colonna + 1,
		Percorso:    ce.Percorso,
	}
	if ce.Righe != nil {
		pos.RigheTesto = &[2]int{ce.Righe[0], ce.Righe[1]}
	}
	u := evidenze.UnitaEvidenza{
		ID:             id,
		EntitaID:       riga,
		Selettore:      selettoreDi(seg.ctx),
		Testo:          ce.Testo,
		CampoOriginale: evidenze.CampoOriginale{Parser: campoHTML, Mappatura: mappaturaEmail},
		Qualita:        evidenze.QualitaUnita{Localizzazione: localizzazioneParziale, Motivo: motivoCellaParziale, Metodo: metodoParser},
	}
	switch {
	case l.derivato:
		u.Qualita.Motivo = motivoDaHTML
	case ce.Coincide && ce.Righe != nil:
		if iv, ok := l.trattoDelleRighe(*ce.Righe); ok {
			pos.Esatto = &iv
			u.Testo = l.corpo[iv.Inizio:iv.Fine]
			u.Qualita = evidenze.QualitaUnita{Localizzazione: localizzazioneEsatta, Metodo: metodoParser}
		}
	}
	u.Posizione = evidenze.Localizzatore{Tipo: "tabella", Tabella: &pos}
	l.c.unita(u)
}

// trattoDelleRighe: il tratto del corpo dalle righe [da, a) del Taglio, dall'inizio della prima alla fine
// dell'ultima (senza il terminatore), senza gli spazi ai bordi. ok è falso se le righe non sono del taglio o il
// tratto è vuoto.
func (l *lettoreMail) trattoDelleRighe(r [2]int) (evidenze.Intervallo, bool) {
	righe := l.taglio.Righe
	if r[0] < 0 || r[1] > len(righe) || r[0] >= r[1] {
		return evidenze.Intervallo{}, false
	}
	iv := ripulito(l.corpo, [2]int{righe[r[0]].Inizio, righe[r[1]-1].Fine})
	return iv, iv.Inizio < iv.Fine
}

// segmentoDelleRighe: il segmento che contiene tutte le righe [da, a). Nessuno, se le righe stanno a cavallo di
// due segmenti.
func (l *lettoreMail) segmentoDelleRighe(r [2]int) (segmentoMail, bool) {
	for _, s := range l.segmenti {
		if s.righe[0] < s.righe[1] && r[0] >= s.righe[0] && r[1] <= s.righe[1] {
			return s, true
		}
	}
	return segmentoMail{}, false
}

// percorsoDellaRiga: il percorso nel DOM della <tr> di una cella, cioè quello della cella senza l'ultimo pezzo.
func percorsoDellaRiga(p string) string {
	if i := strings.LastIndexByte(p, '/'); i > 0 {
		return p[:i]
	}
	return ""
}
