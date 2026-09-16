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

func sembraCodice(s string) bool {
	if len(s) < 5 || len(s) > 40 || stopCodici[s] || rePrefissiNonCodice.MatchString(s) {
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
	Oggetto           string
	Corpo             string
	NomiAllegati      []string
	Direzione         string // entrata | uscita
	Interno           bool   // mittente e destinatari tutti nostri: il collega che gira una mail
	ClienteNoto       bool   // dominio mittente censito
	BuyerNoto         bool   // indirizzo mittente censito come buyer
	ThreadTrovato     bool   // aggancio automatico già riuscito
	ConversazioneNota bool
	// Motore sono le regole del cliente riconosciuto, già compilate (voce 6.11). Nil = cliente
	// sconosciuto, o cliente senza regole: il triage continua a funzionare con il solo
	// estrattore generico, perché un cliente non censito è il caso NORMALE del primo giorno.
	Motore *Motore
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
}

// confini di parola: «rdo» e «rfq» compaiono dentro parole comuni (ricordo, bernardo)
var reParoleRFQ = regexp.MustCompile(`\b(rfq|rdo|richiesta d'offerta|richiesta di offerta|richiesta offerta|quotazione|preventivo|quotation|request for quotation|offer request|angebot|devis)\b`)
var estensioniCAD = map[string]bool{"stp": true, "step": true, "sldprt": true, "sldasm": true, "igs": true, "iges": true,
	"dxf": true, "dwg": true, "pdf": true, "tif": true, "tiff": true, "zip": true, "7z": true, "rar": true}

// Triage propone un esito per un messaggio orfano. È solo un suggerimento: l'operatore resta l'ultimo a confermare.
func Triage(in IngressoTriage) EsitoTriage {
	var motivi []string
	punti := 0
	// Una mail in uscita è roba nostra già vista: non c'è niente da smistare. Una mail INTERNA è in
	// uscita anch'essa — parte da un nostro indirizzo — ma «ti giro questa richiesta» è uno dei modi
	// in cui una RFQ arriva davvero sul tavolo, e ignorarla per il mittente significherebbe non
	// proporre niente proprio sui messaggi che un collega ha inoltrato apposta perché qualcuno li
	// guardasse (voce 2.1, D10).
	if in.Direzione == "uscita" && !in.Interno {
		return EsitoTriage{Esito: "ignora", Confidenza: 60, Motivi: []string{"messaggio in uscita"}}
	}
	if in.Interno {
		motivi = append(motivi, "mail interna: inoltrata da un collega")
	}
	if in.ThreadTrovato {
		return EsitoTriage{Esito: "aggancia", Confidenza: 95, Motivi: []string{"agganciato automaticamente"}}
	}
	testo := strings.ToLower(in.Oggetto + "\n" + in.Corpo)
	if m := reParoleRFQ.FindString(testo); m != "" {
		punti += 35
		motivi = append(motivi, "testo contiene «"+m+"»")
	}
	nCAD := 0
	for _, n := range in.NomiAllegati {
		if i := strings.LastIndex(n, "."); i >= 0 && estensioniCAD[strings.ToLower(n[i+1:])] {
			nCAD++
		}
	}
	if nCAD > 0 {
		punti += 25
		motivi = append(motivi, "allegati tecnici presenti")
	}
	if in.BuyerNoto {
		punti += 25
		motivi = append(motivi, "mittente è un buyer censito")
	} else if in.ClienteNoto {
		punti += 15
		motivi = append(motivi, "dominio mittente è un cliente censito")
	}
	testi := []string{in.Oggetto, in.Corpo}
	for _, n := range in.NomiAllegati {
		base := n
		if i := strings.LastIndex(base, "."); i > 0 {
			base = base[:i]
		}
		testi = append(testi, base)
	}
	trovati := in.Motore.Codici(testi...)
	codici := dedup(SoloCodici(trovati))
	if len(codici) > 0 {
		punti += 10
		motivi = append(motivi, "codici rilevati: "+strings.Join(primi(codici, 4), ", "))
	}
	// Un codice riconosciuto da una FAMIGLIA del cliente vale più di uno pescato dall'estrattore
	// generico: il primo dice «questo è un codice DI QUESTO CLIENTE», il secondo dice «questo ha la forma di
	// un codice». Sono due affermazioni diverse e non devono pesare uguale.
	if f := primaFamiglia(trovati); f != "" {
		punti += 10
		motivi = append(motivi, "codici della famiglia «"+f+"» del cliente")
	}
	rif, rifNome := in.Motore.Riferimento(in.Oggetto, in.Corpo)
	if rif != "" {
		punti += 15
		motivi = append(motivi, "riferimento "+rif+" ("+rifNome+")")
	}
	if punti > 100 {
		punti = 100
	}
	esito := "ignora"
	if in.ConversazioneNota && !in.ThreadTrovato {
		esito = "ignora"
	}
	if punti >= 50 {
		esito = "nuova_rfq"
	}
	if len(motivi) == 0 {
		motivi = []string{"nessun indizio RFQ"}
	}
	return EsitoTriage{Esito: esito, Confidenza: punti, Motivi: motivi, Codici: codici,
		Trovati: trovati, Riferimento: rif, RiferimentoNome: rifNome}
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
