// Package agente è il TERZO strato di interpretazione: dopo i fatti deterministici, prima
// dell'operatore (checkpoint 3R §9).
//
//	Outlook → ingest dei fatti → candidati deterministici → analisi semantica → PROPOSTE → operatore
//
// Che cosa può fare e che cosa no. L'agente legge un messaggio già in database e produce una
// proposta strutturata: che intento ha il messaggio, a quale richiesta somiglia, che cosa sono i
// numeri che contiene, che cosa manca, e — quando serve — una bozza di risposta. Non scrive
// `thread_id`, non conferma articoli né distinte, non tocca il NAS, non invia posta, non cambia
// fase. Tutto ciò che produce è una riga in `analisi_messaggio` e, di lì, un riquadro in Inbox.
//
// # Il grounding sta qui, non nel prompt
//
// «L'agente non può inventare un codice» non è un'istruzione da scrivere in un prompt: è un
// CONTROLLO. Ogni codice, ogni riferimento e ogni richiesta citati nella risposta vengono
// ricontrollati dal server contro il testo del messaggio, i nomi degli allegati e i candidati che il
// motore deterministico aveva già calcolato. Ciò che non trova riscontro viene scartato e registrato
// in `analisi_messaggio.scartato` con il motivo: non raggiunge la schermata, ma resta leggibile,
// perché è la misura di quanto il modello sta inventando.
//
// Un modello che sbaglia di rado è più pericoloso di uno che sbaglia spesso, perché smette di essere
// controllato. Il grezzo si conserva apposta.
//
// # Riservatezza
//
// Il testo delle mail dei clienti esce verso un servizio esterno. È una decisione che va messa per
// iscritto con l'IT e con chi segue la ISO 27001, per casella, con la possibilità di spegnerla.
// Perciò: spento se non lo si accende nel file di configurazione, spento se manca la chiave, e
// acceso solo sulle caselle elencate. Nella prima versione escono oggetto, corpo, nomi degli
// allegati e i fatti già estratti: MAI il contenuto dei PDF, mai gli allegati.
package agente

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// VersioneAnalizzatore cambia quando cambia il CODICE di questo pacchetto in modo da rendere diverso
// il risultato. Entra nella chiave di `analisi_messaggio` insieme all'hash dell'input, alla versione
// del prompt e al modello: a parità di tutti e quattro, rifare l'analisi darebbe lo stesso risultato
// e costerebbe soldi.
const VersioneAnalizzatore = "1"

// VersionePrompt cambia a ogni modifica del testo delle istruzioni.
const VersionePrompt = "p1"

// Intenti ammessi. Un valore fuori da questo elenco fa rifiutare l'intera risposta: un intento che il
// resto del sistema non sa leggere non è una proposta, è un campo che qualcuno interpreterà a caso.
const (
	IntentoNuovaRFQ    = "nuova_rfq"
	IntentoRispostaRFQ = "risposta_rfq"
	IntentoDocumenti   = "documenti_aggiuntivi"
	IntentoNonRFQ      = "non_rfq"
	IntentoIncerto     = "incerto"
)

var intentiValidi = map[string]bool{
	IntentoNuovaRFQ: true, IntentoRispostaRFQ: true, IntentoDocumenti: true,
	IntentoNonRFQ: true, IntentoIncerto: true,
}

// ruoliValidi sono gli stessi dell'enum `ruolo_codice`: l'agente non ne inventa di nuovi.
var ruoliValidi = map[string]bool{
	"riferimento_rfq": true, "prodotto": true, "parte": true, "non_classificato": true,
}

// tipiAllegatoValidi sono i valori di `tipo_documento` che l'agente può proporre dal solo nome del
// file. Non c'è `disegno_2d` per un PDF: quello lo dice il worker-analisi leggendo il cartiglio, e
// lasciarlo dire a un modello che ha visto solo il nome rifarebbe il difetto di §5 con un'altra mano.
var tipiAllegatoValidi = map[string]bool{
	"da_determinare": true, "cad_3d": true, "sviluppo_dxf": true, "distinta_cliente": true,
	"capitolato": true, "commerciale": true, "offerta_fornitore": true, "ordine_cliente": true,
	"corrispondenza": true, "rumore": true, "altro": true,
}

// Proposta è l'output dell'agente. I tag JSON sono anche lo schema che il modello deve rispettare.
type Proposta struct {
	Intento string `json:"intento"`
	// Riferimento è il numero con cui il cliente chiama la richiesta, se il messaggio lo cita.
	Riferimento string `json:"riferimento_rfq,omitempty"`
	// Candidati sono le richieste esistenti a cui il messaggio potrebbe appartenere, in ordine. Gli
	// id devono venire dall'elenco passato nel contesto: l'agente sceglie, non cerca.
	Candidati []CandidatoAI `json:"candidati,omitempty"`
	Codici    []CodiceAI    `json:"codici,omitempty"`
	Allegati  []AllegatoAI  `json:"allegati,omitempty"`
	// Evidenze sono le frasi del messaggio su cui l'agente si basa. Servono all'operatore per
	// credergli o non credergli in due secondi.
	Evidenze []string `json:"evidenze"`
	// CosaManca è la lettura utile che il deterministico non sa dare: «mancano i 3D», «i CAD sono
	// sul portale», «non è dichiarato il materiale».
	CosaManca []string `json:"cosa_manca,omitempty"`
	// Bozza è una proposta di risposta. Resta una bozza: l'invio è manuale, in Outlook.
	Bozza string `json:"bozza_risposta,omitempty"`
}

type CandidatoAI struct {
	ThreadID  string `json:"thread_id"`
	Punteggio int    `json:"punteggio"`
	Perche    string `json:"perche"`
}

type CodiceAI struct {
	Codice string `json:"codice"`
	Rev    string `json:"rev,omitempty"`
	Ruolo  string `json:"ruolo"`
	Dove   string `json:"dove,omitempty"`
}

type AllegatoAI struct {
	Nome string `json:"nome"`
	Tipo string `json:"tipo"`
}

// Scarto è una cosa che l'agente ha detto e il server non ha confermato.
type Scarto struct {
	Campo  string `json:"campo"`
	Valore string `json:"valore"`
	Motivo string `json:"motivo"`
}

// Contesto è tutto ciò che il server sa del messaggio e che serve a controllare la risposta. È anche
// tutto ciò che viene MANDATO: non esiste un secondo canale con cui l'agente possa vedere altro.
type Contesto struct {
	Oggetto      string
	Corpo        string
	Mittente     string
	Cliente      string
	NomiAllegati []string
	// Candidati sono le richieste che il motore deterministico ha già proposto: id, oggetto, e
	// perché. L'agente può solo sceglierne una, o nessuna.
	Candidati []RichiestaNota
	// CodiciNoti sono i codici che il deterministico ha già trovato, più quelli già presenti in
	// database per questo cliente: un codice citato dall'agente vale anche se compare qui.
	CodiciNoti []string
}

type RichiestaNota struct {
	ThreadID string
	Oggetto  string
	Perche   string
}

// ErrSchema: la risposta non rispetta lo schema. Chi la riceve NON applica niente e registra
// l'analisi come `rifiutata`: un output non conforme non ha effetti parziali.
var ErrSchema = errors.New("risposta dell'agente non conforme allo schema")

// Valida legge la risposta con lo schema stretto. Un campo che non esiste fa fallire tutto: di solito
// è un nome scritto male, e senza questo controllo verrebbe ignorato in silenzio — cioè l'agente
// direbbe una cosa e il sistema ne leggerebbe un'altra.
func Valida(raw []byte) (Proposta, error) {
	var p Proposta
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return p, fmt.Errorf("%w: %v", ErrSchema, err)
	}
	if d.More() {
		return p, fmt.Errorf("%w: dopo l'oggetto JSON c'è dell'altro testo", ErrSchema)
	}
	if !intentiValidi[p.Intento] {
		return p, fmt.Errorf("%w: intento %q non previsto", ErrSchema, p.Intento)
	}
	for _, c := range p.Codici {
		if !ruoliValidi[c.Ruolo] {
			return p, fmt.Errorf("%w: ruolo %q non previsto per il codice %q", ErrSchema, c.Ruolo, c.Codice)
		}
	}
	for _, a := range p.Allegati {
		if !tipiAllegatoValidi[a.Tipo] {
			return p, fmt.Errorf("%w: tipo %q non previsto per l'allegato %q", ErrSchema, a.Tipo, a.Nome)
		}
	}
	for _, k := range p.Candidati {
		if k.Punteggio < 0 || k.Punteggio > 100 {
			return p, fmt.Errorf("%w: punteggio %d fuori scala per il candidato %q", ErrSchema, k.Punteggio, k.ThreadID)
		}
	}
	return p, nil
}

// reTokenCodice riconosce, nel testo di una bozza, i gruppi che hanno la FORMA di un codice. Serve
// solo al controllo della bozza: non è un estrattore, è una rete.
var reTokenCodice = regexp.MustCompile(`\b[A-Z0-9][A-Z0-9._/\-]{4,}\b`)

// Verifica è il grounding. Toglie dalla proposta tutto ciò che non ha riscontro nel messaggio o nei
// dati già in database, e restituisce che cosa ha tolto e perché.
//
// Le regole, una per campo:
//
//   - un CODICE deve comparire nel testo del messaggio, nei nomi degli allegati, oppure fra i codici
//     che il deterministico e il database già conoscono per questo cliente;
//   - il RIFERIMENTO deve comparire nel testo;
//   - un CANDIDATO deve essere uno di quelli passati nel contesto: l'agente sceglie fra richieste
//     esistenti, non ne nomina di sue;
//   - un ALLEGATO deve essere uno degli allegati veri, con il suo nome esatto;
//   - la BOZZA cade interamente se cita un numero con la forma di un codice che non esiste nel
//     messaggio né in database. Una risposta al cliente che contiene un part number sbagliato è un
//     danno vero, e non è il caso di correggerla a metà.
func Verifica(p Proposta, c Contesto) (Proposta, []Scarto) {
	var scarti []Scarto
	testo := normalizza(c.Oggetto + "\n" + c.Corpo + "\n" + strings.Join(c.NomiAllegati, "\n"))
	noti := map[string]bool{}
	for _, k := range c.CodiciNoti {
		noti[normalizza(k)] = true
	}
	nelMessaggio := func(v string) bool {
		n := normalizza(v)
		return n != "" && (strings.Contains(testo, n) || noti[n])
	}

	if p.Riferimento != "" && !nelMessaggio(p.Riferimento) {
		scarti = append(scarti, Scarto{"riferimento_rfq", p.Riferimento, "non compare nel messaggio"})
		p.Riferimento = ""
	}

	var codici []CodiceAI
	for _, k := range p.Codici {
		if !nelMessaggio(k.Codice) {
			scarti = append(scarti, Scarto{"codici", k.Codice, "non compare nel messaggio né fra i codici noti del cliente"})
			continue
		}
		// un riferimento non diventa un codice prodotto nemmeno se lo dice l'agente (§4)
		if p.Riferimento != "" && strings.Contains(normalizza(p.Riferimento), normalizza(k.Codice)) && k.Ruolo != "riferimento_rfq" {
			scarti = append(scarti, Scarto{"codici", k.Codice, "è il riferimento della richiesta, non un codice prodotto"})
			continue
		}
		codici = append(codici, k)
	}
	p.Codici = codici

	ammessi := map[string]bool{}
	for _, r := range c.Candidati {
		ammessi[r.ThreadID] = true
	}
	var cand []CandidatoAI
	for _, k := range p.Candidati {
		if !ammessi[k.ThreadID] {
			scarti = append(scarti, Scarto{"candidati", k.ThreadID, "non è fra le richieste passate nel contesto"})
			continue
		}
		cand = append(cand, k)
	}
	sort.SliceStable(cand, func(i, j int) bool { return cand[i].Punteggio > cand[j].Punteggio })
	p.Candidati = cand

	veri := map[string]bool{}
	for _, n := range c.NomiAllegati {
		veri[n] = true
	}
	var all []AllegatoAI
	for _, a := range p.Allegati {
		if !veri[a.Nome] {
			scarti = append(scarti, Scarto{"allegati", a.Nome, "questo messaggio non ha un allegato con questo nome"})
			continue
		}
		all = append(all, a)
	}
	p.Allegati = all

	if p.Bozza != "" {
		for _, tok := range reTokenCodice.FindAllString(strings.ToUpper(p.Bozza), -1) {
			if contaCifre(tok) < 3 || nelMessaggio(tok) {
				continue
			}
			scarti = append(scarti, Scarto{"bozza_risposta", tok,
				"la bozza cita un numero che nel messaggio non c'è: una risposta con un codice sbagliato è un danno, e non si corregge a metà"})
			p.Bozza = ""
			break
		}
	}
	return p, scarti
}

func normalizza(s string) string {
	return strings.ToUpper(strings.Join(strings.Fields(s), " "))
}

func contaCifre(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}

// Impronta è l'hash di ciò che viene mandato. Con versione, prompt e modello forma la chiave di
// `analisi_messaggio`: la stessa domanda non si paga due volte, e la rianalisi è una scelta.
func (c Contesto) Impronta() string {
	b, _ := json.Marshal(c)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Modello è chi risponde davvero. È un'interfaccia perché tutto il resto di questo pacchetto — lo
// schema, il grounding, la chiave di idempotenza — si prova senza rete e senza chiave d'API, e
// perché il giorno in cui si cambia fornitore non si riscrive il grounding.
type Modello interface {
	// Chiedi manda il contesto e restituisce il JSON grezzo della risposta, più i token consumati.
	Chiedi(ctx context.Context, sistema, utente string) (grezzo []byte, tokenIn, tokenOut int, err error)
	Nome() string
}

// Analizza è il giro completo: costruisce il prompt, chiede, convalida, verifica.
//
// Restituisce SEMPRE il grezzo e gli scarti, anche in caso di rifiuto: sono la misura di quanto il
// modello sta inventando, e senza quella misura «l'agente funziona bene» è un'impressione.
type Esito struct {
	Proposta Proposta
	Grezzo   []byte
	Scarti   []Scarto
	TokenIn  int
	TokenOut int
	Errore   error // non nil = schema rifiutato o errore di rete: nessun effetto
}

func Analizza(ctx context.Context, m Modello, c Contesto) Esito {
	sistema, utente := Prompt(c)
	grezzo, in, out, err := m.Chiedi(ctx, sistema, utente)
	e := Esito{Grezzo: grezzo, TokenIn: in, TokenOut: out}
	if err != nil {
		e.Errore = err
		return e
	}
	p, err := Valida(ripulisci(grezzo))
	if err != nil {
		e.Errore = err
		return e
	}
	e.Proposta, e.Scarti = Verifica(p, c)
	return e
}

// ripulisci toglie il recinto markdown che i modelli mettono intorno al JSON anche quando gli si
// chiede di non farlo. Non è indulgenza verso l'output: è che rifiutare una risposta giusta per tre
// backtick significherebbe rifiutarne parecchie, e lo schema resta comunque il giudice.
func ripulisci(raw []byte) []byte {
	s := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(s, "```") {
		return []byte(s)
	}
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	if i := strings.LastIndex(s, "```"); i >= 0 {
		s = s[:i]
	}
	return []byte(strings.TrimSpace(s))
}
