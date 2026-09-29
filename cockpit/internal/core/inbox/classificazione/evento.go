package classificazione

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"promatec/cockpit/internal/core/registro/regole"
)

// L'EVENTO DELLA MAIL (Smistamento M2, A5.16.2)
//
// Due domande, due risposte, due posti. «Che cosa sta succedendo con questa mail?» — una richiesta
// nuova, l'arrivo dei CAD, una revisione, un sollecito — è una proprietà del MESSAGGIO, e si legge
// soltanto dal messaggio: chi scrive, in che direzione, il testo scritto adesso, gli allegati. «A quale
// RFQ appartiene?» è un elenco di candidati con le loro evidenze, e lo decide una persona. Fino al 7C
// le due cose erano la stessa informazione: l'atto dei clienti si ricavava dall'esito (nuova_rfq →
// richiesta_offerta, tutto il resto «incerto»), e una risposta con uno STEP nuovo dentro una RFQ
// esistente non poteva essere altro che «incerto».
//
// Qui l'evento non guarda i candidati — la funzione non li riceve nemmeno — e non ne cambia l'ordine,
// l'esito, il thread proposto o thread_id. Una risposta a una mail vecchia che nel testo nuovo chiede
// un'offerta con uno STEP è NUOVA_RFQ anche con un R0 a 98: evento e candidato si vedono insieme, e
// decide l'utente.
//
// Non c'è una colonna nuova. L'evento si salva come ATTO (`proposta_triage.atto`, la tabella
// `atto_business` della 0016) e si rilegge con EventoDa: ogni atto porta a un solo evento, quindi anche
// le proposte già decise conservano il loro. Le evidenze vanno fra i motivi con il prefisso «evento ·».
const (
	EventoNuovaRFQ            = "NUOVA_RFQ"
	EventoArrivoCAD           = "ARRIVO_CAD"
	EventoRevisioneCAD        = "REVISIONE_CAD"
	EventoRispostaCommerciale = "RISPOSTA_COMMERCIALE"
	EventoOffertaFornitore    = "OFFERTA_FORNITORE"
	EventoOrdine              = "ORDINE"
	EventoSollecito           = "SOLLECITO"
	EventoAltro               = "ALTRO"
)

// Eventi è il vocabolario, nell'ordine della tabella di A5.16.2.
var Eventi = []string{EventoNuovaRFQ, EventoArrivoCAD, EventoRevisioneCAD, EventoRispostaCommerciale,
	EventoOffertaFornitore, EventoOrdine, EventoSollecito, EventoAltro}

// La forza dell'evento è una parola e non un numero: l'evento è un'evidenza da leggere, non uno score da
// confrontare, e le liste di parole su cui si regge non sono calibrate.
const (
	ForzaChiaro    = "chiaro"
	ForzaProbabile = "probabile"
	ForzaIncerto   = "incerto"
)

// PrefissoEvento apre le righe dei motivi che parlano dell'evento: la schermata le separa da quelle
// della proposta di aggancio, che rispondono all'altra domanda.
const PrefissoEvento = "evento · "

var etichetteEvento = map[string]string{
	EventoNuovaRFQ: "Nuova RFQ", EventoArrivoCAD: "Arrivo CAD", EventoRevisioneCAD: "Revisione CAD",
	EventoRispostaCommerciale: "Risposta commerciale", EventoOffertaFornitore: "Offerta del fornitore",
	EventoOrdine: "Ordine", EventoSollecito: "Sollecito", EventoAltro: "Altro",
}

// EtichettaEvento è l'evento con le parole della schermata («Revisione CAD»).
func EtichettaEvento(evento string) string {
	if e, ok := etichetteEvento[evento]; ok {
		return e
	}
	return etichetteEvento[EventoAltro]
}

// EventoDa ricava l'evento da ciò che è salvato: controparte, direzione, atto e legame. È la tabella di
// A5.16.2. Gli eventi dei clienti valgono per un cliente in entrata e per un inoltro di un collega (il
// legame `inoltro` di un interno: l'atto è quello del contenuto girato); l'offerta vale solo per un
// fornitore in entrata. Tutto il resto è ALTRO: le nostre mail in uscita, gli atti che non dicono niente
// (`incerto`, `notifica`, …) e il vecchio atto `inoltro`, che dal M2 non si scrive più.
func EventoDa(controparte, direzione, atto, legame string) string {
	if controparte == ControparteFornitore && direzione == "entrata" {
		if atto == AttoOfferta {
			return EventoOffertaFornitore
		}
		return EventoAltro
	}
	delCliente := controparte == ControparteCliente && direzione == "entrata" ||
		controparte == ControparteInterno && legame == LegameInoltro
	if !delCliente {
		return EventoAltro
	}
	switch atto {
	case AttoRichiestaOfferta:
		return EventoNuovaRFQ
	case AttoDocumentiAggiuntivi:
		return EventoArrivoCAD
	case AttoRevisioneDocumenti:
		return EventoRevisioneCAD
	case AttoDomandaChiarimento, AttoRispostaChiarimento, AttoAccettazione, AttoRifiuto:
		return EventoRispostaCommerciale
	case AttoOrdine:
		return EventoOrdine
	case AttoSollecito:
		return EventoSollecito
	}
	return EventoAltro
}

// IngressoEvento è soltanto il messaggio. Non ci sono i candidati, e non ci devono essere.
type IngressoEvento struct {
	Controparte string // uno dei Controparte*
	Direzione   string // entrata | uscita
	Interno     bool   // un collega che gira una mail
	Oggetto     string
	Corpo       string // intero: il taglio della catena lo fa Evento
	InReplyTo   string
	// NomiAllegati: i nomi degli allegati di primo livello, con l'estensione.
	NomiAllegati []string
	Mittente     string
	// DalCockpit: la nostra mail preparata dal Cockpit (la bozza, D84). Lo sa solo chi legge `bozza`, cioè
	// la schermata: l'ingest non interpreta le nostre mail ai clienti.
	DalCockpit bool
	// Motore sono le regole del cliente (Smistamento 4.13b): i mittenti di sistema, il numero d'ordine
	// anticipato o da verificare, la forma del numero d'ordine. Sono cose che il cliente dichiara di sé,
	// non candidati: l'evento resta una proprietà del messaggio. Nil = le regole di sempre.
	Motore *Motore
}

// EsitoEvento: l'evento, l'atto con cui si salva, la forza a parole e le evidenze da leggere.
type EsitoEvento struct {
	Evento   string
	Atto     string
	Forza    string
	Evidenze []string
}

// Evento legge il messaggio e dice che evento è. Le regole sono in ordine e la prima che parla decide
// (A5.16.2, r1_mail §3.2):
//
//	E1  nostra mail in uscita (non interna)            → ALTRO, con l'atto di sempre (triageUscita)
//	E1b mittente di sistema del cliente (4.13b)          → l'evento dichiarato (ALTRO / notifica, ORDINE)
//	E2  posta che non è di lavoro (NonBusiness)          → ALTRO / non_business
//	E3  fornitore in entrata                             → AttoFornitore: OFFERTA_FORNITORE se è un'offerta
//	E4  parole d'ordine, numero d'ordine del cliente     → ORDINE
//	    «commande» da sola                               → ORDINE (probabile)
//	    con numero_ordine_anticipato e una richiesta     → non decide: si va avanti, l'ordine resta un'evidenza (4.13b)
//	    con ordine_da_verificare                         → ORDINE (probabile) (4.13b)
//	E5  parole di sollecito, senza allegati tecnici      → SOLLECITO
//	    «in attesa di Vs. riscontro», senza richiesta    → SOLLECITO
//	E6  allegati tecnici e segni di revisione            → REVISIONE_CAD
//	E7  allegati tecnici in una risposta senza richiesta → ARRIVO_CAD
//	E8  parole di richiesta                              → NUOVA_RFQ
//	    («SI RFQ», «SI CTE» valgono come richiesta solo accanto all'ordine, con numero_ordine_anticipato)
//	E9  allegati tecnici in una mail che non risponde    → NUOVA_RFQ (probabile)
//	E10 risposta con parole commerciali                  → RISPOSTA_COMMERCIALE
//	E11 il resto                                         → ALTRO
//
// Le parole si cercano solo nel testo scritto adesso, e per le risposte non nell'oggetto: è la lezione
// del checkpoint 3R («R: RICHIESTA OFFERTA COD …» era la risposta a una richiesta nostra, non una nuova).
func Evento(in IngressoEvento) EsitoEvento { return evento(in, true) }

// evento è Evento con E2 facoltativa. Senza E2 è la «seconda lettura» che Triage usa in un caso solo:
// un cliente con l'evidenza di una RFQ aperta, dove le parole di posta non di lavoro (una firma con
// «newsletter», un «fuori ufficio» di cortesia) non decidono niente (evidenza_test.go, revisione 25/09).
func evento(in IngressoEvento, conNonBusiness bool) EsitoEvento {
	legame := ""
	if in.Controparte == ControparteInterno {
		legame = LegameInoltro
	}
	// ordineAnticipato è l'evidenza dell'ordine che E4 non ha deciso (4.13b): si aggiunge all'evidenza di
	// qualunque regola decida dopo, perché l'operatore la veda accanto al sollecito o alla revisione
	ordineAnticipato := ""
	fine := func(atto, forza string, evidenze ...string) EsitoEvento {
		if ordineAnticipato != "" {
			evidenze = append(evidenze[:len(evidenze):len(evidenze)], ordineAnticipato)
		}
		return EsitoEvento{Evento: EventoDa(in.Controparte, in.Direzione, atto, legame), Atto: atto, Forza: forza, Evidenze: evidenze}
	}
	// E1
	if in.Direzione == "uscita" && !in.Interno {
		ev := []string{"nostra mail in uscita"}
		if in.DalCockpit {
			ev = append(ev, "preparata dal Cockpit")
		}
		return fine(attoUscita(in.Controparte), ForzaChiaro, ev...)
	}
	// E1b (Smistamento 4.13b): un mittente di sistema del cliente. Lo ha dichiarato una persona, nelle regole
	// del cliente, e si legge prima delle parole: un avviso del gestionale con «RFQ» nell'oggetto non è una
	// richiesta d'offerta. Viene anche prima di E2, che legge parole generiche («noreply», «newsletter»):
	// la voce del cliente è più precisa, e un ordine generato da un sistema resta un ordine. La condizione è
	// quella del triage (postaDelCliente): i motivi e l'evento dicono la stessa cosa.
	if postaDelCliente(in.Controparte, in.Direzione, in.Interno) {
		if ms, ok := in.Motore.MittenteDiSistema(in.Mittente); ok {
			return fine(attoMittenteSistema(ms.Evento), ForzaChiaro, "mittente di sistema del cliente: "+descriviMittente(ms))
		}
	}
	// E2
	if conNonBusiness {
		if si, m := NonBusiness(in.Mittente, in.Oggetto, in.Corpo); si {
			return fine(AttoNonBusiness, ForzaChiaro, m)
		}
	}
	// E3: l'atto del fornitore resta quello di sempre, con la sua frase
	if in.Controparte == ControparteFornitore {
		atto, perche := AttoFornitore(in.Mittente, in.Oggetto, in.Corpo, in.NomiAllegati)
		return fine(atto, ForzaProbabile, perche)
	}
	// Un mittente da censire (o censito due volte): prima si decide CHI è, poi che cosa vuole (D34). Le
	// parole di un fornitore e quelle di un cliente dicono cose diverse, e leggerle con le regole dei
	// clienti vorrebbe dire scegliere la controparte senza dirlo.
	if in.Controparte == ControparteSconosciuto || in.Controparte == ControparteAmbiguo {
		return fine(AttoIncerto, ForzaIncerto, "mittente da censire: prima si decide chi è, poi che cosa vuole")
	}
	c := leggiContenuto(in)
	// Le parole di richiesta si leggono qui e non più in fondo, perché E4 (4.13b) ed E5 (4.13) le guardano.
	// Il valore è lo stesso: testo e oggetto non cambiano fra E4 ed E8.
	richiesta, doveRichiesta := c.trova(reParoleRFQ)
	// E4. Le parole d'ordine chiare, poi il numero d'ordine nella forma del cliente (4.13b: «ODA_0001234»
	// è una parola d'ordine per chi lo dichiara), poi la lista a parte. «commande» da sola non dice «ordine»
	// con certezza: «une commande de 20 pièces» sta anche dentro una richiesta di prezzo in francese, che
	// fino alla 4.13 diventava un ORDINE chiaro. Resta un ordine, ma probabile, e sceglie l'operatore. Che una
	// parola di richiesta accanto a «commande» debba far decidere E8 non lo dice il piano, salvo per il
	// cliente con il numero d'ordine anticipato (qui sotto): per gli altri è una domanda aperta.
	ordine, forzaOrdine := "", ""
	if m, dove := c.trova(reOrdine); m != "" {
		ordine, forzaOrdine = dove+" dice «"+m+"»", ForzaChiaro
	} else if m, dove := c.trovaNumeroOrdine(in.Motore); m != "" {
		ordine, forzaOrdine = dove+" dice «"+m+"», un numero d'ordine del cliente ("+in.Motore.NomeNumeroOrdine()+")", ForzaChiaro
	} else if m, dove := c.trova(reOrdineProbabile); m != "" {
		ordine, forzaOrdine = dove+" dice «"+m+"», che da sola non basta per un ordine chiaro", ForzaProbabile
	}
	if ordine != "" {
		// Il numero d'ordine anticipato (4.13b): questo cliente emette il numero d'ordine prima di ricevere
		// l'offerta, e un ordine accanto a una parola di richiesta non dice «ordine». E4 non decide: la mail si
		// legge come se l'ordine non ci fosse, e l'ordine resta fra le evidenze. Così un «Sollecito RDO …» resta
		// un sollecito (E5) e una revisione dei disegni resta una revisione (E6), come senza il numero d'ordine;
		// la richiesta vince solo se nessuna delle due parla (E8). Senza la parola di richiesta l'ordine resta
		// un ordine: la chiave non dice che il cliente non mandi mai ordini veri.
		if in.Motore.OrdineAnticipato() {
			if richiesta == "" {
				// L'eccezione: «SI RFQ» e «SI CTE», l'oggetto delle mail del suo portale, valgono come richiesta
				// SOLO accanto all'ordine, e solo per il cliente con la chiave: si cercano qui e in nessun altro
				// posto. Da sole non fanno una richiesta: una CTE può essere un cambio tecnico o un phase out (la
				// legge la 4.16). Trovate qui, E5 (la formula di chiusura), E7 ed E8 le contano come le altre
				// parole di richiesta. «SI RFQ» contiene già «rfq», una parola di richiesta per tutti
				// (reParoleRFQ), e qui non arriva mai: in pratica l'eccezione cambia solo «SI CTE».
				richiesta, doveRichiesta = c.trova(reSiPortale)
			}
			if richiesta != "" {
				ordineAnticipato = ordine + ": il cliente emette il numero d'ordine prima dell'offerta (numero_ordine_anticipato)"
			}
		}
		if ordineAnticipato == "" {
			// La voce più debole (4.13b, domanda 22a = A): per questo cliente un ordine va verificato, e scende
			// a probabile. La parola di richiesta, se c'è, si dice: è ciò che l'operatore deve guardare.
			if forzaOrdine == ForzaChiaro && in.Motore.OrdineDaVerificare() {
				ev := []string{ordine, "per questo cliente una mail con «ordine» può essere una richiesta (ordine_da_verificare): sceglie l'operatore"}
				if richiesta != "" {
					ev = append(ev, doveRichiesta+" dice anche «"+richiesta+"»")
				}
				return fine(AttoOrdine, ForzaProbabile, ev...)
			}
			return fine(AttoOrdine, forzaOrdine, ordine)
		}
	}
	// E5. Le parole di sollecito vere («sollecito», «reminder», «relance») decidono anche accanto a una
	// richiesta: «Sollecito RDO …» è un sollecito. La formula di chiusura «Resto in attesa di Vs.
	// riscontro» no: chiude quasi ogni richiesta di preventivo scritta in italiano, e con una parola di
	// richiesta nel testo non fa un SOLLECITO (Smistamento 4.13, risposta 12 del 29/09).
	if len(c.tecnici) == 0 {
		if m, dove := c.trova(reSollecito); m != "" {
			return fine(AttoSollecito, ForzaProbabile, "chiede notizie: "+dove+" dice «"+m+"»")
		}
		if m, dove := c.trova(reAttesaRiscontro); m != "" && richiesta == "" {
			return fine(AttoSollecito, ForzaProbabile, "chiede notizie: "+dove+" dice «"+m+"»")
		}
	}
	// E6: la parola e la revisione nel nome insieme sono chiare; una sola delle due è probabile
	if len(c.tecnici) > 0 {
		parola, dove := c.trova(reRevisione)
		if parola != "" || len(c.revisioni) > 0 {
			var ev []string
			ev = append(ev, c.revisioni...)
			if parola != "" {
				ev = append(ev, dove+" dice «"+parola+"»")
			}
			forza := ForzaProbabile
			if parola != "" && len(c.revisioni) > 0 {
				forza = ForzaChiaro
			}
			return fine(AttoRevisioneDocumenti, forza, strings.Join(ev, "; "))
		}
	}
	// E7
	if len(c.tecnici) > 0 && c.risposta && richiesta == "" {
		return fine(AttoDocumentiAggiuntivi, ForzaProbabile, fmt.Sprintf("risposta con %s (%s)", fileTecnici(len(c.tecnici)), elenco(c.tecnici)))
	}
	// E8
	if richiesta != "" {
		ev := doveRichiesta + " dice «" + richiesta + "»"
		forza := ForzaProbabile
		if len(c.tecnici) > 0 || len(c.pdf) > 0 {
			forza = ForzaChiaro
			ev += "; " + c.allegati()
		}
		return fine(AttoRichiestaOfferta, forza, ev)
	}
	// E9
	if len(c.tecnici) > 0 && !c.risposta {
		return fine(AttoRichiestaOfferta, ForzaProbabile, fmt.Sprintf("prima mail con %s (%s), senza parole di richiesta", fileTecnici(len(c.tecnici)), elenco(c.tecnici)))
	}
	// E10
	if c.risposta {
		if m, _ := c.trova(reCommerciale); m != "" {
			return fine(attoCommerciale(c), ForzaProbabile, "risponde sull'offerta: «"+m+"»")
		}
	}
	// E11
	atto := AttoIncerto
	if in.Controparte == ControparteAltro {
		atto = AttoComunicazioneGenerica
	}
	return fine(atto, ForzaIncerto, "nessun segno riconosciuto")
}

// postaDelCliente dice se un messaggio è posta in entrata di un cliente, per le voci che il cliente dichiara
// di sé (4.13b, i mittenti di sistema). Vale anche con la controparte non dichiarata: il banco di prova
// dell'Anagrafica, e i messaggi di prima della 0014, hanno comunque le regole di un cliente, e le parole si
// leggono come per lui (E4–E11). Mai per un collega che gira una mail, né per un fornitore. È la stessa
// condizione per l'evento (E1b) e per il triage (mittenteDiSistema): i motivi e l'evento dicono la stessa cosa.
func postaDelCliente(controparte, direzione string, interno bool) bool {
	return direzione == "entrata" && !interno && (controparte == ControparteCliente || controparte == "")
}

// attoMittenteSistema è l'atto con cui si salva l'evento dichiarato per un mittente di sistema: l'ordine è
// un ordine, un avviso è una notifica (che EventoDa rilegge ALTRO).
func attoMittenteSistema(evento string) string {
	if evento == EventoOrdine {
		return AttoOrdine
	}
	return AttoNotifica
}

// descriviMittente è la riga del mittente di sistema come la legge l'operatore: il mittente, e la
// descrizione che gli ha dato chi l'ha scritto.
func descriviMittente(ms regole.MittenteSistema) string {
	s := regole.NormaIndirizzo(ms.Mittente)
	if d := strings.TrimSpace(ms.Descrizione); d != "" {
		s += " («" + d + "»)"
	}
	return s
}

// attoUscita è l'atto di una nostra mail in uscita, lo stesso di triageUscita: a un fornitore una
// richiesta d'offerta (o una risposta a lui), a un cliente la nostra offerta. Per gli altri non si dice.
func attoUscita(controparte string) string {
	switch controparte {
	case ControparteFornitore:
		return AttoRichiestaOfferta
	case ControparteCliente:
		return AttoOfferta
	}
	return ""
}

// Le liste di parole di E4, E5, E6 e E10. Si cercano sul testo in minuscolo. Sono corte e multilingua di
// proposito, e non sono calibrate: un errore qui costa una lettura sbagliata dell'evento, mai una
// scrittura (l'evento non aggancia).
var (
	// «conferma d'ordine» non c'è: la scrive un fornitore, e il fornitore passa da E3.
	//
	// Il confine ammette il trattino basso (Smistamento 4.13). Per `\b` il «_» è una lettera, e dopo
	// «bestellung» in «Bestellung_4500099999» — la parola attaccata al numero, come la scrive un
	// gestionale — il confine non c'era: non era un ordine. Il «_» preso dal confine lo toglie trova.
	reOrdine = regexp.MustCompile(`(?:\b|_)(ordine d['’]acquisto|ordine di acquisto|ordine n[°r.]*\s*\d+|purchase order|p\.?o\.?\s*(?:n[°or.]*|#|:)\s*\d+|bestellung|bon de commande)(?:\b|_)`)
	// la lista a parte di E4: parole che dicono «ordine» solo come probabile
	reOrdineProbabile = regexp.MustCompile(`(?:\b|_)(commande)(?:\b|_)`)
	// «SI RFQ», «SI CTE»: l'oggetto delle mail del portale di un cliente che emette il numero d'ordine prima
	// dell'offerta (4.13b). Vale come una parola di richiesta SOLO accanto a un ordine di quel cliente, con
	// numero_ordine_anticipato: una CTE da sola può essere un cambio tecnico, un phase out, e non una richiesta
	// (la legge la 4.16)
	reSiPortale = regexp.MustCompile(`\bsi\s+(?:rfq|cte)\b`)
	// sollecit* prende sollecito, sollecitiamo, sollecitare
	reSollecito = regexp.MustCompile(`(\bsollecit\w*|\breminder\b|\bany updates?\b|\brelance\w*|\bnachfrage\b)`)
	// la formula di chiusura: fa un sollecito solo in un testo senza parole di richiesta (E5)
	reAttesaRiscontro = regexp.MustCompile(`in attesa (?:di )?(?:un )?(?:vostro|vs\.?) riscontro`)
	// segni di revisione nel testo: la parola intera, per l'evidenza. Davanti a «änderung» niente \b: per
	// le regex di Go «ä» non è una lettera, e il confine non ci sarebbe mai
	reRevisione = regexp.MustCompile(`(\brevision\w*|\brev\b\.?|\baggiornat\w*|\bnuova versione\b|\bmodific\w*|\bsostitui\w*|\bupdated\b|\brevised\b|änderung\w*)`)
	// parole di chi risponde su un'offerta
	reCommerciale  = regexp.MustCompile(`(\bprezz\w*|\bscont[oi]\b|\bquantit\w*|\blott[oi]\b|\bconsegn\w*|\bvs\.? offerta\b|\bvostra offerta\b|\baccettiamo\b|\bconfermiamo (?:la vostra |l['’])offerta\b|\bnon competitiv\w*|\bnon procediamo\b|\btarget\b|\bprice\b|\bdiscount\b|\bdelivery\b|\blead ?time\b)`)
	reAccettazione = regexp.MustCompile(`(\baccettiamo\b|\bconfermiamo (?:la vostra |l['’])offerta\b|\bprocediamo con l['’]ordine\b|\bwe accept\b)`)
	reRifiuto      = regexp.MustCompile(`(\bnon competitiv\w*|\bnon procediamo\b|\bnon siamo interessati\b|\babbiamo scelto (?:un )?altr\w*|\bnot competitive\b|\bdeclin\w*)`)
	// il prefisso di risposta, che dice «risposta» anche senza In-Reply-To (la posta fuori da Outlook)
	reRispostaOggetto = regexp.MustCompile(`(?i)^\s*(re|r|aw|antw|sv)\s*:`)
)

// attoCommerciale: accettazione, rifiuto, domanda o risposta di chiarimento, in quest'ordine.
func attoCommerciale(c contenuto) string {
	switch {
	case reAccettazione.MatchString(c.testo):
		return AttoAccettazione
	case reRifiuto.MatchString(c.testo):
		return AttoRifiuto
	case reDomandaFornitore.MatchString(c.testo):
		return AttoDomandaChiarimento
	}
	return AttoRispostaChiarimento
}

// contenuto è ciò che le regole E4–E11 leggono del messaggio.
type contenuto struct {
	testo     string // lo scritto adesso (e il contenuto girato di un inoltro), in minuscolo
	oggetto   string // l'oggetto in minuscolo, "" per le risposte: lì le parole non contano
	risposta  bool
	tecnici   []string // nomi degli allegati tecnici (estensioniTecniche)
	pdf       []string // PDF, TIF: forse tecnici
	revisioni []string // «7120001A_2.stp porta la rev 2»

	// testo e oggetto come sono scritti, per le regex del cliente (4.13b: il numero d'ordine), che sono
	// sue e distinguono le maiuscole. Le stesse porzioni: l'oggetto di una risposta resta vuoto.
	testoGrezzo, oggettoGrezzo string
}

// trova cerca le parole prima nel testo e poi, se il messaggio non è una risposta, nell'oggetto. Il
// secondo valore dice dove, con le parole dell'evidenza. Un «_» ai bordi è il confine delle parole
// d'ordine (reOrdine), non una parola: l'evidenza dice «bestellung», non «bestellung_».
func (c contenuto) trova(re *regexp.Regexp) (string, string) {
	if m := re.FindString(c.testo); m != "" {
		return strings.Trim(strings.TrimSpace(m), "_"), "il testo nuovo"
	}
	if m := re.FindString(c.oggetto); m != "" {
		return strings.Trim(strings.TrimSpace(m), "_"), "l'oggetto"
	}
	return "", ""
}

// trovaNumeroOrdine è trova per il numero d'ordine nella forma del cliente (4.13b): lo stesso ordine (prima il
// testo, poi l'oggetto se non è una risposta), sul testo com'è scritto. Senza la voce, niente.
func (c contenuto) trovaNumeroOrdine(m *Motore) (string, string) {
	if n := m.NumeriOrdine(c.testoGrezzo); len(n) > 0 {
		return n[0], "il testo nuovo"
	}
	if n := m.NumeriOrdine(c.oggettoGrezzo); len(n) > 0 {
		return n[0], "l'oggetto"
	}
	return "", ""
}

func (c contenuto) allegati() string {
	var p []string
	if len(c.tecnici) > 0 {
		p = append(p, fileTecnici(len(c.tecnici)))
	}
	if len(c.pdf) > 0 {
		p = append(p, fmt.Sprintf("%d PDF", len(c.pdf)))
	}
	return "con " + strings.Join(p, " e ")
}

func fileTecnici(n int) string {
	if n == 1 {
		return "1 file tecnico"
	}
	return fmt.Sprintf("%d file tecnici", n)
}

func elenco(nomi []string) string {
	if len(nomi) <= 3 {
		return strings.Join(nomi, ", ")
	}
	return strings.Join(nomi[:3], ", ") + ", …"
}

// leggiContenuto prepara il testo su cui cercare le parole, dice se il messaggio è una risposta e
// divide gli allegati.
//
// «Risposta» vuol dire In-Reply-To, oppure un prefisso di risposta nell'oggetto, oppure una storia
// citata. Un inoltro è un caso a parte: la storia citata è il contenuto girato e non dice niente, e ciò
// che conta è se la mail GIRATA era una risposta (il suo oggetto, la sua storia). Per un inoltro e per
// una mail interna le parole si cercano anche nella prima porzione della storia, cioè nel contenuto
// girato (A5.16.2: il legame è `inoltro`, l'atto è del contenuto); l'origine citata è del M4.
func leggiContenuto(in IngressoEvento) contenuto {
	utile, storia := TagliaCatena(in.Corpo)
	oggetto := in.Oggetto
	testo := utile
	var risposta bool
	if EInoltro(in.Oggetto, in.Corpo) {
		oggetto = reInoltroOggetto.ReplaceAllString(oggetto, "")
		commento, girato := utile, storia
		if storia == "" {
			// inoltro senza commento: il taglio ha lasciato il corpo intero, che è tutto contenuto girato
			commento, girato = "", utile
		}
		g, storiaGirato := contenutoGirato(girato)
		testo = commento + "\n" + g
		risposta = reRispostaOggetto.MatchString(oggetto) || storiaGirato != ""
	} else {
		risposta = strings.TrimSpace(in.InReplyTo) != "" || reRispostaOggetto.MatchString(oggetto) || storia != ""
		if in.Interno && storia != "" {
			g, _ := contenutoGirato(storia)
			testo += "\n" + g
		}
	}
	c := contenuto{testo: strings.ToLower(testo), testoGrezzo: testo, risposta: risposta}
	if !risposta {
		c.oggetto, c.oggettoGrezzo = strings.ToLower(oggetto), oggetto
	}
	for _, n := range in.NomiAllegati {
		i := strings.LastIndex(n, ".")
		if i < 0 {
			continue
		}
		ext := strings.ToLower(n[i+1:])
		switch {
		case estensioniTecniche[ext]:
			c.tecnici = append(c.tecnici, n)
		case estensioniDaDeterminare[ext]:
			c.pdf = append(c.pdf, n)
		default:
			continue
		}
		if rev := revDalNome(n[:i]); rev != "" {
			c.revisioni = append(c.revisioni, n+" porta la rev "+rev)
		}
	}
	return c
}

// contenutoGirato è la prima porzione di una storia citata: si saltano il separatore, l'apertura di
// citazione e il blocco delle intestazioni in testa, e si tiene lo scritto del messaggio girato fino alla
// sua storia. Il secondo valore è quella storia: se c'è, anche la mail girata era una risposta.
func contenutoGirato(s string) (girato, storia string) {
	righe := strings.Split(normalizza(s), "\n")
	i := 0
	for i < len(righe) {
		t := pulisciCitazione(righe[i])
		if t == "" || rigaDiSeparazione(t) || separatoreEsplicito(t) || chiaveIntestazione(t) != "" || aperturaDiCitazione(righe, i) {
			i++
			continue
		}
		break
	}
	return TagliaCatena(strings.Join(righe[i:], "\n"))
}

// revDalNome: la revisione che il nome di un file porta, se è una revisione successiva alla prima.
// «7120001A_2» porta la rev 2 ed è un segno di revisione; «7120001A_1» è la prima emissione (0, 1 e A lo
// sono per convenzione) e non dice che qualcosa è cambiato: è l'arrivo dei CAD, non la loro revisione.
// Il codice deve avere la forma di un codice, perché «foto_2» non è la seconda revisione di niente.
func revDalNome(base string) string {
	codice, rev := CodiceRev(base)
	if rev == "" || !sembraCodice(codice) {
		return ""
	}
	if n, err := strconv.Atoi(rev); err == nil {
		if n >= 2 {
			return rev
		}
		return ""
	}
	if len(rev) == 1 && rev <= "A" {
		return ""
	}
	return rev
}

// MotiviEvento sono le righe dei motivi che raccontano l'evento: la prima con l'evento, la forza e la
// prima evidenza («evento · REVISIONE_CAD (probabile): 7120001A_2.stp porta la rev 2»), poi una riga per
// ogni altra evidenza. Restano stringhe perché `proposta_triage.motivi` è un array di stringhe.
func MotiviEvento(e EsitoEvento) []string {
	if e.Evento == "" {
		return nil
	}
	testa := PrefissoEvento + e.Evento
	if e.Forza != "" {
		testa += " (" + e.Forza + ")"
	}
	if len(e.Evidenze) == 0 {
		return []string{testa}
	}
	out := []string{testa + ": " + e.Evidenze[0]}
	for _, x := range e.Evidenze[1:] {
		out = append(out, PrefissoEvento+x)
	}
	return out
}

// LeggiMotiviEvento è il contrario di MotiviEvento, per la schermata: separa dai motivi salvati le righe
// dell'evento (evento, forza, evidenze) da quelle della proposta. Un evento letto qui vale solo se
// coincide con quello che l'atto salvato dice: chi lo usa lo confronta con EventoDa.
func LeggiMotiviEvento(motivi []string) (e EsitoEvento, altri []string) {
	for _, m := range motivi {
		resto, ok := strings.CutPrefix(m, PrefissoEvento)
		if !ok {
			altri = append(altri, m)
			continue
		}
		if e.Evento == "" {
			if codice, dopo, ok := testaEvento(resto); ok {
				e.Evento = codice
				if f, ev, ok := strings.Cut(dopo, ":"); ok {
					e.Forza = strings.Trim(strings.TrimSpace(f), "()")
					if ev = strings.TrimSpace(ev); ev != "" {
						e.Evidenze = append(e.Evidenze, ev)
					}
				} else {
					e.Forza = strings.Trim(strings.TrimSpace(dopo), "()")
				}
				continue
			}
		}
		e.Evidenze = append(e.Evidenze, resto)
	}
	return e, altri
}

// testaEvento riconosce la prima riga: comincia con uno degli eventi.
func testaEvento(s string) (codice, dopo string, ok bool) {
	for _, ev := range Eventi {
		if r, ok := strings.CutPrefix(s, ev); ok && (r == "" || r[0] == ' ' || r[0] == ':') {
			return ev, r, true
		}
	}
	return "", "", false
}
