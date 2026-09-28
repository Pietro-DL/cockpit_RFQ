// Package aggancio calcola i CANDIDATI di aggancio di un messaggio: le regole R0–R5 con punteggio ed
// evidenza (fase 4.1 del piano, D9).
//
// Che cosa è cambiato con il checkpoint 3R. Prima queste stesse informazioni venivano usate dall'ingest
// per SCRIVERE `messaggio.thread_id`: un ConversationID uguale, o un codice pescato dall'estrattore
// generico, e il messaggio finiva agganciato a una RFQ senza che nessuno avesse deciso niente. Sul banco
// reale questo produce due danni che si vedono solo con la posta vera:
//
//   - «Rispondi» a una mail vecchia per parlare d'altro conserva il ConversationID. Il messaggio nuovo
//     entrava nella RFQ sbagliata, e ci restava, perché un aggancio non si annulla da solo;
//   - l'estrattore generico chiamava «codice» qualunque numero con tre cifre. Un numero d'ordine, un CAP,
//     una data compatta bastavano a far combaciare due richieste che non c'entravano niente.
//
// Adesso ogni regola produce una RIGA in `candidato_aggancio`, con il punteggio e la frase che l'operatore
// legge. `messaggio.thread_id` lo scrive solo un bottone premuto da una persona.
package aggancio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// FinestraDefault è entro quanti giorni due messaggi possono ancora essere la stessa richiesta quando il
// cliente non lo dichiara (`regole.finestra_aggancio_gg`).
//
// Sessanta giorni, e non «sempre», perché in questo mestiere lo stesso pezzo viene riquotato: stesso
// codice, stesso oggetto, stesso buyer, due anni dopo, ed è una richiesta nuova. Una regola senza finestra
// aggancerebbe la richiesta del 2026 a quella del 2024 e chiamerebbe «evidenza» la coincidenza (T3).
const FinestraDefault = 60

// FinestraBuyer è la memoria corta di R5: «questo buyer ci ha scritto la settimana scorsa» è un modo di
// mettere in fila i candidati, non un'affermazione su niente.
const FinestraBuyer = 14

// Ingresso è tutto ciò che serve a calcolare i candidati. Sono fatti già estratti: questo pacchetto non
// legge il testo del messaggio e non interpreta niente.
type Ingresso struct {
	MessaggioID     uuid.UUID
	ConversazioneID uuid.UUID
	ClienteID       uuid.NullUUID
	BuyerID         uuid.NullUUID
	InReplyTo       string
	Riferimenti     []string // header References
	Oggetto         string   // già ripulito da RE:/R:/FW: (classificazione.OggettoPulito)
	DataEvento      time.Time
	// Codici sono i codici su cui vale R3. Solo quelli di FAMIGLIA: un codice pescato
	// dall'estrattore generico è evidenza che esiste un numero, non che esiste quella richiesta.
	Codici []string
	// Riferimento è il numero con cui il cliente chiama la richiesta (RDO, Anfrage, ODA): R4.
	Riferimento string
	// FinestraGG viene da `cliente.regole.finestra_aggancio_gg`; 0 = usa FinestraDefault.
	FinestraGG int
	// Smistamento M1: ciò che serve a VERIFICARE R0 e a far salire R1. Il mittente (era fra i
	// partecipanti della mail citata?), se il messaggio è un inoltro (classificazione.EInoltro: chi
	// inoltra si porta dietro le References di un'altra conversazione) e il ConversationIndex.
	Mittente            string
	Inoltro             bool
	IndiceConversazione string
}

func (in Ingresso) finestra() time.Time {
	g := in.FinestraGG
	if g <= 0 {
		g = FinestraDefault
	}
	return in.DataEvento.AddDate(0, 0, -g)
}

// raccolta tiene i candidati uno per (RFQ, regola), come la chiave primaria di `candidato_aggancio`.
// Due varianti della stessa regola verso la stessa RFQ (In-Reply-To e References, R3 con e senza buyer)
// finiscono nella stessa riga: vince la PIÙ FORTE (K25). Prima vinceva la prima che si incontrava.
type raccolta struct {
	out []classificazione.Candidato
	pos map[string]int
}

func (r *raccolta) agg(threadID uuid.UUID, tipo, evidenza string, chiuso bool) {
	k := classificazione.NuovoCandidato(threadID.String(), tipo, evidenza, chiuso)
	chiave := k.ThreadID + "|" + k.Regola
	if r.pos == nil {
		r.pos = map[string]int{}
	}
	if i, ok := r.pos[chiave]; ok {
		if k.Punteggio > r.out[i].Punteggio {
			r.out[i] = k
		}
		return
	}
	r.pos[chiave] = len(r.out)
	r.out = append(r.out, k)
}

// Calcola interroga il database e restituisce i candidati ordinati, i più forti in cima. Non scrive niente.
func Calcola(ctx context.Context, q *db.Queries, in Ingresso) ([]classificazione.Candidato, error) {
	var r raccolta

	// ---- R0: In-Reply-To e References. È l'unico legame che scrive il programma di posta e non una
	// persona: se c'è, quel messaggio risponde davvero a quell'altro. Non è una somiglianza. Ma vale come
	// prova solo VERIFICATO (M1, P36): chi scrive adesso era fra chi si scriveva allora, e non è un
	// inoltro. Il messaggio citato sta nella RFQ se è agganciato, oppure se è la nostra mail preparata
	// dal Cockpit per quella RFQ (D84): la nostra mail inviata resta orfana finché qualcuno non la
	// aggancia, e prima la risposta del cliente non trovava niente.
	if chiavi := ChiaviCitate(in.InReplyTo, in.Riferimenti); len(chiavi) > 0 {
		righe, err := q.CitatiPerChiavi(ctx, chiavi)
		if err != nil {
			return nil, fmt.Errorf("R0: %w", err)
		}
		for _, c := range righe {
			inReplyTo := strings.Contains(in.InReplyTo, strings.Trim(c.ChiaveEsterna, "<>"))
			campo := "References"
			if inReplyTo {
				campo = "In-Reply-To"
			}
			partecipanti := append([]string{c.MittenteIndirizzo.String}, indirizziDestinatari(c.Destinatari)...)
			verificato, manca := classificazione.R0Verificato(in.Mittente, partecipanti, in.Inoltro)
			quando := c.DataEvento.Local().Format("02/01")
			completa := func(tipo, frase string) (string, string) {
				if !verificato {
					return classificazione.TipoR0NonVerificato, frase + ", ma " + manca
				}
				return tipo, frase + " (" + strings.ToLower(strings.TrimSpace(in.Mittente)) + " era fra i partecipanti)"
			}
			if c.ThreadID.Valid {
				tipo := classificazione.TipoR0References
				if inReplyTo {
					tipo = classificazione.TipoR0InReplyTo
				}
				tipo, frase := completa(tipo, fmt.Sprintf("%s punta alla mail del %s di questa richiesta", campo, quando))
				r.agg(c.ThreadID.UUID, tipo, frase, c.StatoThread.StatoThread == db.StatoThreadCHIUSA)
			}
			// la nostra mail preparata dal Cockpit: la RFQ della bozza e quella di adesso della mail a cui
			// rispondeva. Due diverse sono due candidati della stessa forza: nessuno si propone.
			for _, t := range classificazione.OrigineDaBozza(uuidTesto(c.ThreadBozza), uuidTesto(c.ThreadRisposta)) {
				tid, _ := uuid.Parse(t)
				perche := "preparata dal Cockpit per questa richiesta"
				chiusa := c.StatoBozza.StatoThread == db.StatoThreadCHIUSA
				if !c.ThreadBozza.Valid || t != c.ThreadBozza.UUID.String() {
					perche = "preparata dal Cockpit in risposta a una mail che oggi sta in questa richiesta"
					chiusa = c.StatoRisposta.StatoThread == db.StatoThreadCHIUSA
				}
				tipo, frase := completa(classificazione.TipoR0Cockpit, fmt.Sprintf("%s punta alla nostra mail del %s, %s", campo, quando, perche))
				r.agg(tid, tipo, frase, chiusa)
			}
		}
	}

	// ---- R1: la conversazione. Forte solo se questa mail è una RISPOSTA nella catena di un messaggio
	// della RFQ (il ConversationIndex discende) e il cliente è lo stesso (M1, P36). Il solo
	// ConversationID è debole: Exchange mette nella stessa conversazione la posta con lo stesso oggetto.
	r1, err := candidatiConversazione(ctx, q, in.ConversazioneID, in.MessaggioID, in.IndiceConversazione, in.ClienteID)
	if err != nil {
		return nil, err
	}
	for _, k := range r1 {
		tid, _ := uuid.Parse(k.ThreadID)
		r.agg(tid, k.Tipo, k.Evidenza, k.Chiuso)
	}

	if in.ClienteID.Valid {
		// ---- R4: il riferimento del cliente. «RDO 400012345» identifica la richiesta nel sistema del
		// cliente: se combacia, è la stessa richiesta.
		if in.Riferimento != "" {
			righe, err := q.ThreadPerRiferimentoCliente(ctx, db.ThreadPerRiferimentoClienteParams{
				ClienteID: in.ClienteID.UUID, Riferimento: in.Riferimento})
			if err != nil {
				return nil, fmt.Errorf("R4: %w", err)
			}
			for _, x := range righe {
				r.agg(x.ThreadID, classificazione.TipoR4Riferimento,
					"il riferimento "+in.Riferimento+" è quello di questa richiesta",
					x.Stato == db.StatoThreadCHIUSA)
			}
		}
		// ---- R3: un codice di famiglia già identificativo di una richiesta dello STESSO cliente. Con lo
		// stesso buyer della RFQ sale di un livello (M1): è la stessa persona che parla dello stesso pezzo.
		if len(in.Codici) > 0 {
			righe, err := q.ThreadPerCodiciCliente(ctx, db.ThreadPerCodiciClienteParams{
				ClienteID: in.ClienteID.UUID, Codici: maiuscole(in.Codici), Dal: in.finestra()})
			if err != nil {
				return nil, fmt.Errorf("R3: %w", err)
			}
			for _, x := range righe {
				tipo, frase := classificazione.TipoR3Codice, "il codice "+x.Codice+" è già un identificativo di questa richiesta, ma il buyer è un altro o non è noto"
				if in.BuyerID.Valid && x.BuyerID.Valid && x.BuyerID.UUID == in.BuyerID.UUID {
					tipo, frase = classificazione.TipoR3CodiceBuyer, "il codice "+x.Codice+" è già un identificativo di questa richiesta, dello stesso buyer"
				}
				r.agg(x.ThreadID, tipo, frase, x.Stato == db.StatoThreadCHIUSA)
			}
		}
		// ---- R2: stesso oggetto, stesso cliente, dentro la finestra. L'oggetto è quello ripulito dai
		// prefissi di risposta, altrimenti «R: X» e «X» sarebbero due oggetti diversi. Molto debole: da
		// solo non impedisce più di proporre una richiesta nuova (M1).
		if len(strings.TrimSpace(in.Oggetto)) >= 8 {
			righe, err := q.ThreadPerOggettoCliente(ctx, db.ThreadPerOggettoClienteParams{
				ClienteID: in.ClienteID.UUID, Oggetto: strings.TrimSpace(in.Oggetto), Dal: in.finestra()})
			if err != nil {
				return nil, fmt.Errorf("R2: %w", err)
			}
			for _, x := range righe {
				r.agg(x.ThreadID, classificazione.TipoR2Oggetto,
					"stesso oggetto di questa richiesta, entro i giorni della finestra",
					x.Stato == db.StatoThreadCHIUSA)
			}
		}
	}

	// ---- R5: lo stesso buyer, di recente. Sotto la soglia di evidenza apposta: da solo non deve
	// impedire di proporre una richiesta nuova, altrimenti nessuna richiesta di un buyer conosciuto
	// verrebbe mai proposta come nuova.
	if in.BuyerID.Valid {
		righe, err := q.ThreadRecentiBuyer(ctx, db.ThreadRecentiBuyerParams{
			BuyerID: in.BuyerID, Dal: in.DataEvento.AddDate(0, 0, -FinestraBuyer)})
		if err != nil {
			return nil, fmt.Errorf("R5: %w", err)
		}
		for _, x := range righe {
			r.agg(x.ThreadID, classificazione.TipoR5Buyer,
				"stesso buyer, richiesta aperta negli ultimi giorni", x.Stato == db.StatoThreadCHIUSA)
		}
	}
	return classificazione.OrdinaCandidati(r.out), nil
}

// candidatiConversazione sono i candidati R1 di un messaggio: una RFQ per ogni RFQ che ha messaggi nella
// stessa conversazione (o a cui un operatore l'ha collegata), con la variante che indice e cliente danno.
// È la stessa regola per l'ingest e per il giro degli orfani dopo un aggancio (CandidatoConversazione).
func candidatiConversazione(ctx context.Context, q *db.Queries, conv, messaggio uuid.UUID, indice string, cliente uuid.NullUUID) ([]classificazione.Candidato, error) {
	type rfq struct {
		stato     db.StatoThread
		cliente   uuid.UUID
		indici    []string
		collegata bool
	}
	per := map[uuid.UUID]*rfq{}
	var ordine []uuid.UUID
	prendi := func(t uuid.UUID, stato db.StatoThread, cl uuid.UUID) *rfq {
		x, ok := per[t]
		if !ok {
			x = &rfq{stato: stato, cliente: cl}
			per[t] = x
			ordine = append(ordine, t)
		}
		return x
	}
	// la conversazione collegata da un operatore viene prima: è una decisione, e il suo candidato è quello
	// che l'operatore si aspetta di vedere in cima a parità di forza
	if riga, err := q.ThreadDellaConversazioneConStato(ctx, conv); err == nil && riga.ThreadID.Valid {
		prendi(riga.ThreadID.UUID, riga.Stato, riga.ClienteID).collegata = true
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("R1: %w", err)
	}
	righe, err := q.ConversazioneConIndici(ctx, db.ConversazioneConIndiciParams{ConversazioneID: conv, Escluso: messaggio})
	if err != nil {
		return nil, fmt.Errorf("R1: %w", err)
	}
	for _, x := range righe {
		t := prendi(x.ThreadID, x.Stato, x.ClienteID)
		if x.Indice != "" {
			t.indici = append(t.indici, x.Indice)
		}
	}
	var out []classificazione.Candidato
	for _, t := range ordine {
		x := per[t]
		stesso := cliente.Valid && cliente.UUID == x.cliente
		tipo := classificazione.TipoR1(indice, x.indici, stesso)
		var frase string
		switch tipo {
		case classificazione.TipoR1Forte:
			frase = "stessa conversazione di Outlook, e questa mail è una risposta nella catena di una mail di questa richiesta, dello stesso cliente"
		case classificazione.TipoR1Indice:
			frase = "questa mail è una risposta nella catena di una mail di questa richiesta, ma il cliente è un altro o non è noto: con lo stesso cliente sarebbe forte"
		default:
			frase = "stessa conversazione di Outlook, ma questa mail non risulta una risposta della catena: Exchange può averla messa lì per l'oggetto"
			if x.collegata {
				frase = "la conversazione di Outlook è stata collegata a questa richiesta da un operatore, ma questa mail non risulta una risposta della catena"
			}
		}
		out = append(out, classificazione.NuovoCandidato(t.String(), tipo, frase, x.stato == db.StatoThreadCHIUSA))
	}
	return out, nil
}

// CandidatoConversazione è il candidato R1 di un orfano verso la RFQ a cui un operatore ha appena
// agganciato un altro messaggio della stessa conversazione (il giro degli orfani, T21). Prima era un R1
// fisso a 95; adesso è la variante che l'indice e il cliente dell'orfano danno (M1): la mail messa nella
// conversazione per l'oggetto resta debole e non cambia proposta.
func CandidatoConversazione(ctx context.Context, q *db.Queries, messaggioID, threadID uuid.UUID) (classificazione.Candidato, error) {
	m, err := q.GetMessaggio(ctx, messaggioID)
	if err != nil {
		return classificazione.Candidato{}, err
	}
	indice := ""
	if mo, err := q.GetMessaggioOutlook(ctx, messaggioID); err == nil {
		indice = mo.ConversationIndex.String
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return classificazione.Candidato{}, err
	}
	var cliente uuid.NullUUID
	if riga, err := q.GetInboxRiga(ctx, messaggioID); err == nil {
		cliente = riga.ClienteID
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return classificazione.Candidato{}, err
	}
	tutti, err := candidatiConversazione(ctx, q, m.ConversazioneID, messaggioID, indice, cliente)
	if err != nil {
		return classificazione.Candidato{}, err
	}
	for _, k := range tutti {
		if k.ThreadID == threadID.String() {
			return k, nil
		}
	}
	// la RFQ non ha messaggi nella conversazione (è stata collegata a un'altra prima): resta il solo
	// ConversationID, debole
	return classificazione.NuovoCandidato(threadID.String(), classificazione.TipoR1Solo,
		"stessa conversazione di Outlook del messaggio appena agganciato, ma questa mail non risulta una risposta della catena", false), nil
}

// SalvaPiuForte scrive UN candidato senza abbassare una riga già scritta per (messaggio, RFQ, regola):
// se l'ingest ha già trovato una variante più forte, resta quella (K25). Una riga delle regole di prima
// si sostituisce sempre.
func SalvaPiuForte(ctx context.Context, q *db.Queries, messaggioID uuid.UUID, k classificazione.Candidato) error {
	tid, err := uuid.Parse(k.ThreadID)
	if err != nil {
		return err
	}
	stato := db.StatoThreadAPERTA
	if k.Chiuso {
		stato = db.StatoThreadCHIUSA
	}
	return q.UpsertCandidatoAggancioPiuForte(ctx, db.UpsertCandidatoAggancioPiuForteParams{
		MessaggioID: messaggioID, ThreadID: tid, Regola: db.RegolaAggancio(k.Regola),
		Punteggio: int16(k.Punteggio), Evidenza: k.Evidenza, ThreadStato: stato,
		Attuali: classificazione.PunteggiDellaRegola(k.Regola),
	})
}

// indirizziDestinatari legge gli indirizzi di `messaggio.destinatari` ([{nome, indirizzo, tipo}]).
func indirizziDestinatari(raw json.RawMessage) []string {
	var d []struct {
		Indirizzo string `json:"indirizzo"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &d) != nil {
		return nil
	}
	out := make([]string, 0, len(d))
	for _, x := range d {
		out = append(out, x.Indirizzo)
	}
	return out
}

func uuidTesto(u uuid.NullUUID) string {
	if !u.Valid {
		return ""
	}
	return u.UUID.String()
}

// Salva sostituisce i candidati di un messaggio. Sostituisce e non aggiunge: i candidati sono una
// fotografia dello stato di adesso, e un candidato vecchio verso una richiesta che nel frattempo è stata
// unita ad un'altra è peggio di nessun candidato.
func Salva(ctx context.Context, q *db.Queries, messaggioID uuid.UUID, c []classificazione.Candidato) error {
	if _, err := q.EliminaCandidatiAggancio(ctx, messaggioID); err != nil {
		return err
	}
	for _, k := range c {
		tid, err := uuid.Parse(k.ThreadID)
		if err != nil {
			return err
		}
		stato := db.StatoThreadAPERTA
		if k.Chiuso {
			stato = db.StatoThreadCHIUSA
		}
		if err := q.InsertCandidatoAggancio(ctx, db.InsertCandidatoAggancioParams{
			MessaggioID: messaggioID, ThreadID: tid, Regola: db.RegolaAggancio(k.Regola),
			Punteggio: int16(k.Punteggio), Evidenza: k.Evidenza, ThreadStato: stato,
		}); err != nil {
			return err
		}
	}
	return nil
}

// CalcolaESalva è la coppia, che è quasi sempre come si usa.
func CalcolaESalva(ctx context.Context, q *db.Queries, in Ingresso) ([]classificazione.Candidato, error) {
	c, err := Calcola(ctx, q, in)
	if err != nil {
		return nil, err
	}
	return c, Salva(ctx, q, in.MessaggioID, c)
}

// SalvaCandidatiCodice sostituisce i candidati di codice di un messaggio.
//
// Qui è dove un numero smette di essere «un codice» e diventa «un numero con un ruolo». Il riferimento
// della richiesta entra con ruolo `riferimento_rfq`, le famiglie del cliente con `prodotto`, l'estrattore
// generico con `non_classificato`. La chiave primaria (messaggio, codice) fa il resto: lo stesso numero
// non può stare due volte con due ruoli. Sul conflitto vince l'ULTIMA riga scritta: il riferimento va
// per primo (l'estrazione lo toglie già dai codici), i codici della storia citata prima degli altri
// (perSalvare), così il testo di adesso ha l'ultima parola.
func SalvaCandidatiCodice(ctx context.Context, q *db.Queries, messaggioID uuid.UUID, e classificazione.Estrazione) error {
	if _, err := q.EliminaCandidatiCodice(ctx, messaggioID); err != nil {
		return err
	}
	ins := func(codice, rev, ruolo, origine, famiglia string, punti int, dove string) error {
		if codice = strings.TrimSpace(codice); codice == "" {
			return nil
		}
		codice = tronca(codice, 60)
		return q.InsertCandidatoCodice(ctx, db.InsertCandidatoCodiceParams{
			MessaggioID: messaggioID, Codice: codice, Ruolo: db.RuoloCodice(ruolo), Rev: tronca(rev, 10),
			Origine: db.OrigineCodice(origine), Famiglia: tronca(famiglia, 120), Punteggio: int16(punti),
			Evidenza: dove,
		})
	}
	if e.Riferimento != "" {
		if err := ins(e.Riferimento, "", classificazione.RuoloRiferimento, "riferimento", e.RiferimentoNome,
			classificazione.PuntiRiferimen, e.RiferimentoDove); err != nil {
			return err
		}
	}
	for _, c := range perSalvare(e.Codici) {
		origine := "generico"
		if c.Origine == "famiglia" {
			origine = "famiglia"
		}
		if err := ins(c.Codice, c.Rev, c.Ruolo, origine, c.Famiglia, c.Punteggio, c.Dove); err != nil {
			return err
		}
	}
	return nil
}

// perSalvare mette in testa i codici trovati nella storia citata e lascia gli altri nel loro ordine.
//
// La riga di candidato_codice è una per (messaggio, codice) e l'inserimento tiene l'ULTIMA scritta.
// L'estrazione invece tiene lo stesso codice due volte se le revisioni sono diverse, nell'ordine
// oggetto, corpo, storia, allegati: «rev C» scritta adesso e «rev B» citata sotto finivano in
// database come rev B con evidenza «storia citata». La revisione vecchia della catena cancellava
// quella nuova, e il conflitto di revisioni che il Fascicolo deve mostrare (B8.6) spariva alla fonte.
func perSalvare(cc []classificazione.CodiceTrovato) []classificazione.CodiceTrovato {
	out := make([]classificazione.CodiceTrovato, 0, len(cc))
	for _, c := range cc {
		if c.Dove == classificazione.DoveStoria {
			out = append(out, c)
		}
	}
	for _, c := range cc {
		if c.Dove != classificazione.DoveStoria {
			out = append(out, c)
		}
	}
	return out
}

// tronca accorcia a n CARATTERI, non a n byte: le colonne sono varchar(n), che PostgreSQL conta in
// caratteri, e un taglio a byte dentro una lettera accentata lascia UTF-8 non valido — che il
// database rifiuta, facendo finire in scarto un messaggio intero per la descrizione di una famiglia.
func tronca(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// maxChiaviCitate è il tetto delle chiavi confrontate da R0: un ANY con centinaia di elementi su un
// forward di forward non serve a niente.
const maxChiaviCitate = 40

// ChiaviCitate normalizza In-Reply-To e References in un elenco di Message-ID confrontabili con
// `messaggio.chiave_esterna`. Le parentesi angolari ci sono negli header e possono esserci o non esserci
// nella proprietà MAPI: si provano tutte e due le forme invece di scommettere su una.
func ChiaviCitate(inReplyTo string, riferimenti []string) []string {
	visti := map[string]bool{}
	agg := func(out []string, s string) []string {
		s = strings.TrimSpace(s)
		if s == "" {
			return out
		}
		nudo := strings.Trim(s, "<>")
		if nudo == "" {
			return out
		}
		for _, f := range []string{"<" + nudo + ">", nudo} {
			if !visti[f] {
				visti[f] = true
				out = append(out, f)
			}
		}
		return out
	}
	var irt, rif []string
	for _, s := range strings.Fields(inReplyTo) {
		irt = agg(irt, s)
	}
	for _, s := range riferimenti {
		rif = agg(rif, s)
	}
	// Un thread di posta lungo porta decine di References: le più recenti sono in fondo, e sono quelle
	// che contano. Il tetto però si paga sulle References e mai sull'In-Reply-To: prima si tagliava
	// l'elenco intero dal fondo, e l'In-Reply-To — che sta in testa, ed è il messaggio a cui si
	// risponde davvero — era la prima cosa a sparire proprio nelle catene lunghe.
	if len(irt) > maxChiaviCitate {
		irt = irt[:maxChiaviCitate] // un In-Reply-To di venti identificativi non è un header, è rumore
	}
	if spazio := maxChiaviCitate - len(irt); len(rif) > spazio {
		rif = rif[len(rif)-spazio:]
	}
	return append(irt, rif...)
}

func maiuscole(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, strings.ToUpper(s))
	}
	return out
}
