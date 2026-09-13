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
func RilevaPortale(corpo string) []RiferimentoPortale {
	var out []RiferimentoPortale
	for _, frase := range spezzaFrasi(corpo) {
		low := strings.ToLower(frase)
		trovato := false
		for _, f := range frasiPortale {
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
	ClienteNoto       bool   // dominio mittente censito
	BuyerNoto         bool   // indirizzo mittente censito come buyer
	ThreadTrovato     bool   // aggancio automatico già riuscito
	ConversazioneNota bool
}

type EsitoTriage struct {
	Esito      string // nuova_rfq | aggancia | ignora
	Confidenza int    // 0–100
	Motivi     []string
	Codici     []string
}

// confini di parola: «rdo» e «rfq» compaiono dentro parole comuni (ricordo, bernardo)
var reParoleRFQ = regexp.MustCompile(`\b(rfq|rdo|richiesta d'offerta|richiesta di offerta|richiesta offerta|quotazione|preventivo|quotation|request for quotation|offer request|angebot|devis)\b`)
var estensioniCAD = map[string]bool{"stp": true, "step": true, "sldprt": true, "sldasm": true, "igs": true, "iges": true,
	"dxf": true, "dwg": true, "pdf": true, "tif": true, "tiff": true, "zip": true, "7z": true, "rar": true}

// Triage propone un esito per un messaggio orfano. È solo un suggerimento: l'operatore resta l'ultimo a confermare.
func Triage(in IngressoTriage) EsitoTriage {
	var motivi []string
	punti := 0
	if in.Direzione == "uscita" {
		return EsitoTriage{Esito: "ignora", Confidenza: 60, Motivi: []string{"messaggio in uscita"}}
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
	codici := EstraiCodici(in.Oggetto, in.Corpo)
	for _, n := range in.NomiAllegati {
		base := n
		if i := strings.LastIndex(base, "."); i > 0 {
			base = base[:i]
		}
		codici = append(codici, EstraiCodici(base)...)
	}
	codici = dedup(codici)
	if len(codici) > 0 {
		punti += 10
		motivi = append(motivi, "codici rilevati: "+strings.Join(primi(codici, 4), ", "))
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
	return EsitoTriage{Esito: esito, Confidenza: punti, Motivi: motivi, Codici: codici}
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
