// Package domain contiene le regole pure del Cockpit: nessun HTTP, nessun DB.
// Prende struct in ingresso e restituisce struct/errori; è qui che si concentrano i test unitari.
package domain

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// reCodice riconosce i codici prodotto tipici dei clienti: almeno 5 caratteri, almeno 3 cifre,
// lettere/cifre con eventuali separatori interni (6674611A, 6674611A_4, 12-34567, TS9 8712-4).
var reCodice = regexp.MustCompile(`\b[A-Z0-9]{2,}(?:[-_/][A-Z0-9]+)*\b`)

// parole che compaiono nelle mail RFQ e che non sono codici
var stopCodici = map[string]bool{
	"RFQ": true, "RDO": true, "REV": true, "PDF": true, "STEP": true, "STP": true, "DXF": true, "DWG": true, "ZIP": true,
	"ISO": true, "UNI": true, "EN": true, "RE": true, "FW": true, "FWD": true, "TR": true, "CAD": true, "MM": true, "KG": true,
	"S235": false, "S355": false,
}

// EstraiCodici restituisce i possibili codici prodotto presenti in un testo, deduplicati, in ordine di apparizione.
func EstraiCodici(testi ...string) []string {
	visti := map[string]bool{}
	var out []string
	for _, t := range testi {
		// FIX: Rimuoviamo gli URL prima di cercare i codici per evitare falsi positivi
		// generati dai link di tracciamento (es. newsletter, Teams, ecc.)
		tSenzaURL := reURL.ReplaceAllString(t, " ")

		// Ora facciamo la ricerca sul testo pulito
		for _, m := range reCodice.FindAllString(strings.ToUpper(tSenzaURL), -1) {
			if !sembraCodice(m) || visti[m] {
				continue
			}
			visti[m] = true
			out = append(out, m)
		}
	}
	return out
}

// nomi di file generati da telefoni e strumenti di cattura: non sono codici prodotto
var rePrefissiNonCodice = regexp.MustCompile(`^(SCREENSHOT|IMG|IMAGE|WHATSAPP|DSC|PHOTO|FOTO|SCAN|DOC|PXL|VID)[_\-]?\d`)

// I LIMITI DI UN CODICE (7C.1, P0).
//
// Un codice prodotto e' lungo al massimo MaxCodice caratteri e non contiene spazi; una revisione al
// massimo MaxRev. Sono regole del dominio, non del database: le colonne `codice varchar(60)` e
// `rev varchar(10)` sono piu' larghe apposta, e un test (migrazioni) verifica che restino almeno
// cosi' larghe. Cosi' un valore che passa di qui non puo' rompere una INSERT, e un valore che non
// passa si scarta con il suo nome — non si tronca in silenzio.
//
// Il difetto che ha fatto nascere questa regola: `CodiceRev` restituiva l'intero nome del file
// quando non riconosceva un suffisso di revisione, e `PropostaDaNome` lo scriveva come codice se
// dentro c'era ANCHE UN SOLO token che sembrava un codice. «Offerta 12345678 per fornitura staffe
// zincate rev finale allegato tecnico completo.pdf» diventava un codice di 82 caratteri, e il
// result dello stage — con il file gia' caricato e verificato — veniva rifiutato dal database con
// «value too long for type character varying(60)».
const (
	MaxCodice = 40
	MaxRev    = 10
)

// CodiceAmmissibile dice se una stringa puo' stare nella colonna `codice` di una proposta o di un
// documento: non vuota, entro MaxCodice, senza spazi ne' controlli. Non dice che SIA un codice —
// per quello c'e' sembraCodice — dice che non rompe niente. E' la guardia che il server applica
// a cio' che arriva da fuori (il risultato del worker analisi) prima di scriverlo.
func CodiceAmmissibile(s string) bool {
	if s == "" || len(s) > MaxCodice {
		return false
	}
	return !strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' })
}

// RevAmmissibile: come CodiceAmmissibile, per la revisione (MaxRev).
func RevAmmissibile(s string) bool {
	if s == "" || len(s) > MaxRev {
		return false
	}
	return !strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' })
}

func sembraCodice(s string) bool {
	if len(s) < 5 || len(s) > MaxCodice || stopCodici[s] || rePrefissiNonCodice.MatchString(s) {
		return false
	}
	// un codice non ha spazi dentro: «AB 12345» e' due token, non uno (la stessa regola del
	// worker analisi, `sembra_codice`)
	if strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' }) {
		return false
	}
	cifre, lettere := 0, 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			cifre++
		case r >= 'A' && r <= 'Z':
			lettere++
		}
	}
	if cifre < 3 {
		return false
	}
	// date (2026-09-08, 08/09/2026), orari, numeri di telefono, CAP
	if lettere == 0 && (strings.Count(s, "/") >= 2 || strings.Count(s, "-") >= 2 || strings.Count(s, ".") >= 2) {
		return false
	}
	if lettere == 0 && strings.HasPrefix(s, "+") {
		return false
	}
	return true
}

// CodiceRev separa "6674611A_4" in codice "6674611A" e rev "4" secondo la convenzione più diffusa
// (suffisso _n, -n, _REVn, _Rn). Se non riconosce nulla restituisce il codice intero e rev vuota.
var reRev = regexp.MustCompile(`^(.+?)[_\-](?:REV|R)?([0-9]{1,2}|[A-Z])$`)

func CodiceRev(s string) (codice, rev string) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if m := reRev.FindStringSubmatch(s); m != nil {
		return m[1], m[2]
	}
	return s, ""
}

// ---------------------------------------------------------------- portale

var frasiPortale = []string{
	"sul portale", "nel portale", "on the portal", "caricato sul", "caricati sul", "uploaded to", "uploaded on",
	"disponibili sul", "disponibile sul", "scaricare dal", "scaricabili dal", "download from", "in piattaforma",
}

// RiferimentoPortale è la frase della mail che dice "i file sono sul portale", con i codici citati vicino.
type RiferimentoPortale struct {
	TestoCitato string
	Codici      []string
	URL         string
}

var reURL = regexp.MustCompile(`https?://[^\s<>"']+`)

// RilevaPortale cerca nel corpo le frasi tipo "vi abbiamo caricato i CAD sul portale" e restituisce
// al più un riferimento per frase trovata, con i codici presenti nella stessa frase.
//
// `extra` sono le frasi del cliente (`regole.frasi_portale`), che si AGGIUNGONO a quelle generiche
// e non le sostituiscono: «caricato sul portale» resta vero anche per un cliente che ha frasi sue,
// e un cliente con una frase sbagliata non deve smettere di riconoscere quelle di tutti.
func RilevaPortale(corpo string, extra ...string) []RiferimentoPortale {
	frasi := frasiPortale
	if len(extra) > 0 {
		frasi = append(append([]string{}, frasiPortale...), extra...)
	}
	var out []RiferimentoPortale
	for _, frase := range spezzaFrasi(corpo) {
		low := strings.ToLower(frase)
		trovato := false
		for _, f := range frasi {
			if strings.Contains(low, f) {
				trovato = true
				break
			}
		}
		if !trovato {
			continue
		}
		r := RiferimentoPortale{TestoCitato: strings.TrimSpace(frase), Codici: EstraiCodici(frase)}
		if u := reURL.FindString(frase); u != "" {
			r.URL = u
		}
		out = append(out, r)
	}
	return out
}

func spezzaFrasi(t string) []string {
	t = strings.ReplaceAll(t, "\r\n", "\n")
	var out []string
	for _, riga := range strings.Split(t, "\n") {
		for _, f := range regexp.MustCompile(`[.;!?]\s+`).Split(riga, -1) {
			if strings.TrimSpace(f) != "" {
				out = append(out, f)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- scadenza

var reData = regexp.MustCompile(`\b(\d{1,2})[/.\-](\d{1,2})[/.\-](\d{2,4})\b`)
var paroleScadenza = []string{"entro", "scadenza", "deadline", "by ", "before", "termine", "due date", "risposta entro"}

// RilevaScadenza cerca una data preceduta da parole tipo "entro"/"scadenza" nelle vicinanze.
func RilevaScadenza(corpo string, riferimento time.Time) (time.Time, bool) {
	low := strings.ToLower(corpo)
	for _, m := range reData.FindAllStringSubmatchIndex(low, -1) {
		inizio := m[0] - 40
		if inizio < 0 {
			inizio = 0
		}
		ctx := low[inizio:m[0]]
		ok := false
		for _, p := range paroleScadenza {
			if strings.Contains(ctx, p) {
				ok = true
				break
			}
		}
		if !ok {
			continue
		}
		d, err := parseData(low[m[2]:m[3]], low[m[4]:m[5]], low[m[6]:m[7]])
		if err != nil || d.Before(riferimento.AddDate(0, 0, -1)) || d.After(riferimento.AddDate(1, 0, 0)) {
			continue
		}
		return d, true
	}
	return time.Time{}, false
}

func parseData(g, m, a string) (time.Time, error) {
	if len(a) == 2 {
		a = "20" + a
	}
	return time.Parse("2/1/2006", g+"/"+m+"/"+a)
}

// ---------------------------------------------------------------- triage deterministico (SPEC §6.3)

type IngressoTriage struct {
	Oggetto      string
	Corpo        string
	NomiAllegati []string
	Direzione    string // entrata | uscita
	Interno      bool   // mittente e destinatari tutti nostri: il collega che gira una mail
	ClienteNoto  bool   // dominio mittente censito
	BuyerNoto    bool   // indirizzo mittente censito come buyer
	// Controparte è chi c'è dall'altra parte, risolta dall'anagrafica (blocco 7A, D33): uno dei
	// Controparte* di controparte.go. Vuota = non risolta (messaggi di prima della 0014, o prove
	// che non la dichiarano): vale come «sconosciuto», cioè il comportamento di sempre.
	//
	// Un `fornitore` in entrata NON può produrre `nuova_rfq`: non è un punteggio da superare, è
	// un ramo che non arriva mai a contare i punti. `ambiguo` non produce nessuna proposta: decide
	// una persona.
	Controparte string
	// Motore sono le regole del cliente riconosciuto, già compilate (voce 6.11). Nil = cliente
	// sconosciuto, o cliente senza regole: il triage continua a funzionare con il solo
	// estrattore generico, perché un cliente non censito è il caso NORMALE del primo giorno.
	Motore *Motore
	// Candidati sono le proposte di aggancio già calcolate (R0–R5, internal/core/inbox/aggancio). La loro
	// presenza è ciò che impedisce di valutare `nuova_rfq`: vedi la precedenza in Triage.
	Candidati []Candidato
	// Blocco 7B: il mittente (per riconoscere gli indirizzi automatici), i candidati verso una
	// richiesta ai fornitori (per la posta IN ENTRATA di un fornitore) e le RFQ aperte che una
	// NOSTRA mail a un fornitore cita (RF_oggetto: la richiesta mandata a mano).
	Mittente           string
	CandidatiRichiesta []CandidatoRichiesta
	RichiesteManuali   []Candidato
}

// Testi sono i pezzi di messaggio in cui si cercano numeri, ognuno con l'etichetta di dove sta.
//
// È un metodo e non tre righe dentro Triage perché lo usano in due: il triage, per proporre un esito,
// e l'ingest, per calcolare i candidati di aggancio PRIMA di chiamare il triage. Se fossero due elenchi
// scritti in due posti, il giorno in cui uno dei due impara a leggere un campo in più l'altro no, e i
// candidati verrebbero calcolati su un testo diverso da quello su cui viene calcolato l'esito.
func (in IngressoTriage) Testi() []Testo {
	// Il corpo arriva TAGLIATO (blocco 6): quello che e' stato scritto adesso sta in «corpo», la
	// catena di risposta precedente sta in «storia citata». Non si perde niente — i codici della
	// storia sono l'evidenza migliore per agganciare una risposta alla sua richiesta — ma i due
	// pezzi non pesano uguale: vedi Proponibili.
	//
	// Il taglio NON si applica ai nomi degli allegati: li' non c'e' nessuna catena di risposta, e un
	// nome di file che comincia per «Da» o contiene «On» non e' una citazione di niente.
	utile, storia := TagliaCatena(in.Corpo)
	testi := []Testo{{Dove: "oggetto", Corpo: in.Oggetto}, {Dove: "corpo", Corpo: utile}}
	if storia != "" {
		testi = append(testi, Testo{Dove: DoveStoria, Corpo: storia})
	}
	for _, n := range in.NomiAllegati {
		base := n
		if i := strings.LastIndex(base, "."); i > 0 {
			base = base[:i]
		}
		testi = append(testi, Testo{Dove: "allegato " + n, Corpo: base})
	}
	return testi
}

type EsitoTriage struct {
	Esito      string // nuova_rfq | aggancia | ignora
	Confidenza int    // 0–100
	Motivi     []string
	Codici     []string
	// Trovati sono gli stessi codici con la loro provenienza (famiglia del cliente o generico):
	// è l'evidenza che la schermata mostra accanto alla proposta. `Codici` resta l'elenco piatto
	// perché è ciò che finisce in `proposta_triage.identificativi`.
	Trovati []CodiceTrovato
	// Riferimento è il numero della richiesta secondo il cliente (RDO, Anfrage, ODA), con il
	// nome della regola che l'ha riconosciuto. Non è un codice prodotto.
	Riferimento     string
	RiferimentoNome string
	// Estrazione è tutto ciò che è stato trovato, già diviso per ruolo: è quello che la schermata
	// mostra come «proponibili» e «altri numeri trovati».
	Estrazione Estrazione
	// Candidato è il candidato di aggancio più forte, quando ce n'è uno. Non è una scelta: è il
	// primo della lista che l'operatore vede per intero.
	Candidato *Candidato
	// Atto è che cosa sta facendo il mittente (7C.0): uno degli Atto* di atto.go, vuoto se la
	// controparte non è stata dichiarata (prove che non la conoscono, messaggi di prima della 0014).
	// Legame è che cosa la mail è rispetto a ciò che il Cockpit conosce già: uno dei Legame*.
	// Sono proposte, come l'esito; il deterministico dice quello che sa e «incerto» dove non sa.
	Atto   string
	Legame string
	// CandidatoRichiesta è la richiesta ai fornitori più forte a cui questa posta risponde.
	CandidatoRichiesta *CandidatoRichiesta
}

// confini di parola: «rdo» e «rfq» compaiono dentro parole comuni (ricordo, bernardo)
var reParoleRFQ = regexp.MustCompile(`\b(rfq|rdo|richiesta d'offerta|richiesta di offerta|richiesta offerta|quotazione|preventivo|quotation|request for quotation|offer request|angebot|devis)\b`)

// estensioniTecniche sono i formati che DA SOLI dicono «qui dentro c'è del disegno»: un file STEP o un
// DXF non è nient'altro. Gli archivi ci stanno perché un allegato compresso in una richiesta d'offerta
// è quasi sempre un pacco di disegni, e comunque l'affermazione riguarda il messaggio, non il file.
//
// PDF, TIF e TIFF NON ci sono più, ed è il punto §5 del checkpoint. Un PDF può essere un disegno, un
// capitolato, un'offerta del fornitore, una conferma d'ordine o la firma di qualcuno: dire «allegato
// tecnico» prima di averlo aperto è un'affermazione su un file che nessuno ha letto. Contano lo stesso,
// ma per quello che sono: allegati di tipo ancora da determinare.
var estensioniTecniche = map[string]bool{"stp": true, "step": true, "sldprt": true, "sldasm": true,
	"igs": true, "iges": true, "x_t": true, "x_b": true, "dxf": true, "dwg": true,
	"zip": true, "7z": true, "rar": true}

// estensioniDaDeterminare sono documenti che potrebbero essere tecnici e potrebbero non esserlo.
var estensioniDaDeterminare = map[string]bool{"pdf": true, "tif": true, "tiff": true}

// Triage propone un esito per un messaggio orfano. È solo un suggerimento: l'operatore resta l'ultimo a confermare.
//
// PRECEDENZA (checkpoint 3R §3). L'esito non è più il risultato di una somma che supera una soglia. Un
// punteggio di contenuto misura quanto un messaggio SEMBRA una richiesta nuova; non può retrocedere una
// risposta di cui esiste la prova. L'ordine è:
//
//  1. c'è evidenza di una RFQ esistente (un candidato R0–R3, o R2, sopra la soglia) → «aggancia», e
//     `nuova_rfq` non viene nemmeno valutata;
//  2. non c'è nessuna evidenza → si contano i punti del contenuto, e sopra 50 si propone `nuova_rfq`;
//  3. sotto → «ignora», cioè «non ho niente da dire»: resta in Inbox, senza proposta.
//
// Il caso che ha aperto il checkpoint: «R: RICHIESTA OFFERTA COD 0.056.8238.3» con un PDF allegato
// faceva 35 (parola «offerta») + 25 (allegato «tecnico») = 60, e diventava una richiesta NUOVA mentre
// era la risposta a una richiesta nostra.
func Triage(in IngressoTriage) EsitoTriage {
	var motivi []string
	punti := 0
	// Una mail in uscita è roba nostra già vista: non c'è niente da smistare. Una mail INTERNA è in
	// uscita anch'essa — parte da un nostro indirizzo — ma «ti giro questa richiesta» è uno dei modi
	// in cui una RFQ arriva davvero sul tavolo, e ignorarla per il mittente significherebbe non
	// proporre niente proprio sui messaggi che un collega ha inoltrato apposta perché qualcuno li
	// guardasse (voce 2.1, D10).
	if in.Direzione == "uscita" && !in.Interno {
		return triageUscita(in)
	}
	if in.Interno {
		motivi = append(motivi, "mail interna: inoltrata da un collega")
	}
	// Blocco 7A: la controparte viene PRIMA dei punti, e non è un punto. Un fornitore che scrive
	// «richiesta d'offerta» con un PDF allegato sta rispondendo a una richiesta nostra, o ne sta
	// facendo una a noi che non è una RFQ cliente: in nessuno dei due casi si apre una RFQ. Se c'è
	// l'evidenza di una nostra richiesta esistente si propone l'aggancio (è la sua offerta),
	// altrimenti non si propone niente. I codici si estraggono lo stesso: la schermata li mostra.
	switch in.Controparte {
	case ControparteFornitore:
		return triageFornitore(in, motivi)
	case ControparteAmbiguo:
		motivi = append(motivi, "controparte ambigua: l'indirizzo o il dominio sono censiti sia come cliente sia come fornitore. Decide una persona")
		return EsitoTriage{Esito: "ignora", Confidenza: 0, Motivi: motivi, Codici: []string{}, Atto: AttoIncerto, Legame: LegameIncerto}
	case ControparteSconosciuto:
		// Blocco 7B (D34): da un mittente non censito non nasce una proposta di RFQ. Prima si decide
		// CHI è («Censisci come cliente / fornitore», che ricalcola), poi che cosa vuole.
		return triageSconosciuto(in, motivi)
	case ControparteCliente:
		if si, m := NonBusiness(in.Mittente, in.Oggetto, in.Corpo); si {
			motivi = append(motivi, m, "posta di un cliente che non è di lavoro: se l'indirizzo è automatico, censiscilo come Altro")
			return EsitoTriage{Esito: "ignora", Confidenza: 0, Motivi: motivi, Codici: []string{}, Atto: AttoNonBusiness, Legame: LegameNessuno}
		}
	}
	// Blocco 6: le parole «richiesta d'offerta» si cercano in quello che e' stato scritto ADESSO.
	// In una catena di risposta quelle parole ci sono sempre — stanno nel primo messaggio — e
	// contarle vorrebbe dire dare trentacinque punti di «sembra una richiesta nuova» a ogni
	// «ricevuto, grazie».
	testo := strings.ToLower(in.Oggetto + "\n" + CorpoUtilePerInterpretazione(in.Corpo))
	if m := reParoleRFQ.FindString(testo); m != "" {
		punti += 35
		motivi = append(motivi, "testo contiene «"+m+"»")
	}
	nTecnici, nIgnoti := 0, 0
	for _, n := range in.NomiAllegati {
		i := strings.LastIndex(n, ".")
		if i < 0 {
			continue
		}
		switch ext := strings.ToLower(n[i+1:]); {
		case estensioniTecniche[ext]:
			nTecnici++
		case estensioniDaDeterminare[ext]:
			nIgnoti++
		}
	}
	if nTecnici > 0 {
		punti += 25
		motivi = append(motivi, "allegati tecnici presenti")
	}
	if nIgnoti > 0 {
		punti += 10
		motivi = append(motivi, "allegati di tipo da determinare (l'analisi dirà che cosa sono)")
	}
	if in.BuyerNoto {
		punti += 25
		motivi = append(motivi, "mittente è un buyer censito")
	} else if in.ClienteNoto {
		punti += 15
		motivi = append(motivi, "dominio mittente è un cliente censito")
	}
	e := in.Motore.Estrai(in.Testi()...)
	// `Codici` è ciò che può diventare identificativo della RFQ se l'operatore lo spunta. Con un
	// cliente che ha famiglie dichiarate, l'estrattore generico non ci entra (Proponibili).
	codici := dedup(SoloCodici(e.Proponibili()))
	if len(codici) > 0 {
		punti += 10
		motivi = append(motivi, "codici rilevati: "+strings.Join(primi(codici, 4), ", "))
	}
	// Un codice riconosciuto da una FAMIGLIA del cliente vale più di uno pescato dall'estrattore
	// generico: il primo dice «questo è un codice DI QUESTO CLIENTE», il secondo dice «questo ha la forma di
	// un codice». Sono due affermazioni diverse e non devono pesare uguale.
	if f := primaFamiglia(e.Codici); f != "" {
		punti += 10
		motivi = append(motivi, "codici della famiglia «"+f+"» del cliente")
	}
	// Un riferimento trovato nella STORIA citata non dice che questo messaggio e' una richiesta
	// nuova: dice a quale richiesta risponde, ed e' l'aggancio a occuparsene. Contarlo qui
	// significherebbe far salire il punteggio di «nuova RFQ» proprio sulle risposte.
	if e.Riferimento != "" && e.RiferimentoDove != DoveStoria {
		punti += 15
		motivi = append(motivi, "riferimento "+e.Riferimento+" ("+e.RiferimentoNome+")")
	}
	if punti > 100 {
		punti = 100
	}

	out := EsitoTriage{Confidenza: punti, Codici: codici, Trovati: e.Codici,
		Riferimento: e.Riferimento, RiferimentoNome: e.RiferimentoNome, Estrazione: e}

	// ---- la precedenza. Prima si guarda se la richiesta esiste già, POI se ne sembra una nuova.
	if k, ok := MiglioreCandidato(in.Candidati); ok {
		out.Candidato = &k
		motivi = append(motivi, k.Evidenza)
		if EvidenzaDiRFQEsistente(in.Candidati) {
			out.Esito, out.Confidenza = "aggancia", k.Punteggio
			out.Motivi = motivi
			out.Atto, out.Legame = attoELegameCliente(in, out.Esito)
			return out
		}
		// Nessuna evidenza verso una richiesta APERTA. Restano due casi, e in tutti e due il
		// candidato si vede ma non decide: una richiesta chiusa (che è finita), oppure un indizio
		// debole come R5, «stesso buyer di recente» — che ogni richiesta nuova di un buyer noto
		// avrebbe, e che quindi non può impedire di proporne una nuova.
		if c, chiuso := CandidatoChiuso(in.Candidati); chiuso {
			motivi = append(motivi, "attenzione: esiste già una richiesta simile, ma è CHIUSA ("+c.Evidenza+")")
		} else {
			motivi = append(motivi, "l'indizio è debole: non basta a dire che la richiesta esiste già")
		}
	}
	out.Esito = "ignora"
	if punti >= 50 {
		out.Esito = "nuova_rfq"
	}
	if len(motivi) == 0 {
		motivi = []string{"nessun indizio RFQ"}
	}
	out.Motivi = motivi
	out.Atto, out.Legame = attoELegameCliente(in, out.Esito)
	return out
}

// attoELegameCliente è l'atto e il legame del ramo «cliente» e del ramo «interno» (7C.0).
//
// Il deterministico sa poche cose e le dice: una mail che SEMBRA una richiesta nuova è una
// richiesta d'offerta (legame: nuovo); una mail con l'evidenza di una RFQ esistente è una
// risposta a quella RFQ, ma di che ATTO sia (una revisione, una domanda, un sollecito) dal
// punteggio non si capisce, e l'atto resta «incerto»: prima della 0016 la chiamava rfq_cliente,
// che era falso. Un collega che gira una mail fa un inoltro. Atto vuoto se la controparte non è
// dichiarata: è il comportamento di prima della 0014, e le prove che non la conoscono lo tengono.
func attoELegameCliente(in IngressoTriage, esito string) (atto, legame string) {
	switch esito {
	case "nuova_rfq":
		legame = LegameNuovo
	case "aggancia":
		legame = LegameRisposta
	default:
		legame = LegameNessuno
		if len(in.Candidati) > 0 {
			legame = LegameIncerto
		}
	}
	switch {
	case in.Controparte == ControparteInterno:
		return AttoInoltro, legame
	case in.Controparte != ControparteCliente:
		return "", ""
	case esito == "nuova_rfq":
		return AttoRichiestaOfferta, legame
	}
	return AttoIncerto, legame
}

// triageUscita: una nostra mail. A un fornitore è una richiesta d'offerta (o una risposta a lui):
// se cita un codice di una RFQ aperta, si propone «richiesta a questo fornitore per quella RFQ»
// (RF_oggetto, la richiesta mandata a mano: 7B.2) e la conferma crea la richiesta, mai un thread.
// A un cliente è la nostra offerta o una risposta.
func triageUscita(in IngressoTriage) EsitoTriage {
	out := EsitoTriage{Esito: "ignora", Confidenza: 60, Motivi: []string{"messaggio in uscita"}, Codici: []string{}}
	switch in.Controparte {
	case ControparteFornitore:
		out.Atto, out.Legame = AttoRichiestaOfferta, LegameNessuno
		out.Motivi = []string{"nostra mail a un fornitore censito: una richiesta d'offerta a lui, o una risposta"}
		e := in.Motore.Estrai(in.Testi()...)
		out.Trovati, out.Estrazione = e.Codici, e
		// i codici citati nella nostra mail: sono quelli che la richiesta porta con se' alla conferma
		if codici := dedup(SoloCodici(e.Proponibili())); len(codici) > 0 {
			out.Codici = codici
		}
		if k, ok := MiglioreCandidato(in.RichiesteManuali); ok {
			out.Candidato = &k
			// nasce una richiesta (un oggetto nuovo) dentro una RFQ esistente: il legame è «nuovo»
			out.Esito, out.Confidenza, out.Legame = "aggancia", k.Punteggio, LegameNuovo
			out.Motivi = append(out.Motivi, k.Evidenza,
				"se è la richiesta a questo fornitore per quella RFQ, confermala: nasce la richiesta, non una RFQ nuova")
		}
	case ControparteCliente:
		// la regola di sempre, rinominata: una nostra mail a un cliente si presume la nostra
		// offerta. Il legame non si calcola sulla posta in uscita ai clienti: incerto.
		out.Atto, out.Legame = AttoOfferta, LegameIncerto
		out.Motivi = []string{"nostra mail a un cliente: la nostra offerta, o una risposta"}
	}
	return out
}

// triageFornitore: la posta in entrata di un fornitore censito. Mai `nuova_rfq`, per costruzione.
// L'atto (offerta, domanda, conferma, non di lavoro) viene dal testo; l'esito è «aggancia» se
// c'è una richiesta nostra a cui risponde (candidati richiesta) o una RFQ già collegata (R0/R1),
// altrimenti «ignora» a zero: si vede, e decide una persona.
func triageFornitore(in IngressoTriage, motivi []string) EsitoTriage {
	e := in.Motore.Estrai(in.Testi()...)
	// IB8: nella posta di un fornitore contano SOLO i codici di famiglia dei clienti che hanno
	// richieste aperte a lui (è così che l'ingest costruisce il motore). Un numero pescato
	// dall'estrattore generico non è un codice di nessuno: S235JR, ISO 2768, DIN 933 restano parole.
	e.Codici = DiFamiglia(e.Codici)
	out := EsitoTriage{Codici: dedup(SoloCodici(e.Proponibili())), Trovati: e.Codici,
		Riferimento: e.Riferimento, RiferimentoNome: e.RiferimentoNome, Estrazione: e}
	if out.Codici == nil {
		out.Codici = []string{}
	}
	atto, perche := AttoFornitore(in.Mittente, in.Oggetto, in.Corpo, in.NomiAllegati)
	out.Atto, out.Legame = atto, LegameNessuno
	motivi = append(motivi, "mittente censito come fornitore: non è una richiesta di un cliente, quindi mai una RFQ nuova", perche)
	if k, ok := MiglioreCandidatoRichiesta(in.CandidatiRichiesta); ok {
		out.CandidatoRichiesta = &k
		motivi = append(motivi, k.Evidenza)
		out.Esito, out.Confidenza, out.Motivi, out.Legame = "aggancia", k.Punteggio, motivi, LegameRisposta
		return out
	}
	if k, ok := MiglioreCandidato(in.Candidati); ok {
		out.Candidato = &k
		motivi = append(motivi, k.Evidenza)
		out.Legame = LegameIncerto
		if EvidenzaDiRFQEsistente(in.Candidati) {
			out.Esito, out.Confidenza, out.Motivi, out.Legame = "aggancia", k.Punteggio, motivi, LegameRisposta
			return out
		}
	}
	out.Esito, out.Confidenza, out.Motivi = "ignora", 0, motivi
	return out
}

// triageSconosciuto: un mittente che l'anagrafica non conosce. Si estraggono i codici (la schermata
// li mostra) e si riconosce la posta che non è di lavoro; una RFQ nuova non si propone: chi è, lo
// decide una persona con «Censisci», e il ricalcolo fa il resto. Resta l'aggancio se una regola
// forte lo dice (una risposta dentro una catena già agganciata).
func triageSconosciuto(in IngressoTriage, motivi []string) EsitoTriage {
	e := in.Motore.Estrai(in.Testi()...)
	out := EsitoTriage{Esito: "ignora", Confidenza: 0, Codici: dedup(SoloCodici(e.Proponibili())), Trovati: e.Codici,
		Riferimento: e.Riferimento, RiferimentoNome: e.RiferimentoNome, Estrazione: e, Atto: AttoIncerto, Legame: LegameNessuno}
	if out.Codici == nil {
		out.Codici = []string{}
	}
	if si, m := NonBusiness(in.Mittente, in.Oggetto, in.Corpo); si {
		out.Atto = AttoNonBusiness
		motivi = append(motivi, m)
	}
	motivi = append(motivi, "mittente non censito: prima si decide chi è (Censisci come cliente o fornitore), poi che cosa vuole. Nessuna RFQ nuova da qui")
	if k, ok := MiglioreCandidato(in.Candidati); ok {
		out.Candidato = &k
		motivi = append(motivi, k.Evidenza)
		out.Legame = LegameIncerto
		if EvidenzaDiRFQEsistente(in.Candidati) {
			out.Esito, out.Confidenza, out.Legame = "aggancia", k.Punteggio, LegameRisposta
		}
	}
	out.Motivi = motivi
	return out
}

// primaFamiglia restituisce la descrizione della prima famiglia del cliente che ha riconosciuto
// qualcosa, o "" se hanno parlato solo le regole generiche.
func primaFamiglia(in []CodiceTrovato) string {
	for _, c := range in {
		if c.Origine == "famiglia" {
			return c.Famiglia
		}
	}
	return ""
}

func dedup(in []string) []string {
	visti := map[string]bool{}
	var out []string
	for _, s := range in {
		if !visti[s] {
			visti[s] = true
			out = append(out, s)
		}
	}
	return out
}

func primi(in []string, n int) []string {
	if len(in) <= n {
		return in
	}
	return in[:n]
}

// Ordina è un helper per i test: ordina in place e restituisce lo slice.
func Ordina(s []string) []string { sort.Strings(s); return s }
