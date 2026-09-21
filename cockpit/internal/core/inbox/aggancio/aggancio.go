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
	"errors"
	"fmt"
	"strings"
	"time"

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
	Oggetto         string   // già ripulito da RE:/R:/FW: (domain.OggettoPulito)
	DataEvento      time.Time
	// Codici sono i codici su cui vale R3. Solo quelli di FAMIGLIA: un codice pescato
	// dall'estrattore generico è evidenza che esiste un numero, non che esiste quella richiesta.
	Codici []string
	// Riferimento è il numero con cui il cliente chiama la richiesta (RDO, Anfrage, ODA): R4.
	Riferimento string
	// FinestraGG viene da `cliente.regole.finestra_aggancio_gg`; 0 = usa FinestraDefault.
	FinestraGG int
}

func (in Ingresso) finestra() time.Time {
	g := in.FinestraGG
	if g <= 0 {
		g = FinestraDefault
	}
	return in.DataEvento.AddDate(0, 0, -g)
}

// Calcola interroga il database e restituisce i candidati ordinati, i più forti in cima. Non scrive niente.
func Calcola(ctx context.Context, q *db.Queries, in Ingresso) ([]domain.Candidato, error) {
	var out []domain.Candidato
	visto := map[string]bool{} // (thread, regola): la stessa regola non parla due volte dello stesso thread

	agg := func(threadID uuid.UUID, regola, evidenza string, chiuso bool) {
		k := threadID.String() + "|" + regola
		if visto[k] {
			return
		}
		visto[k] = true
		out = append(out, domain.Candidato{
			ThreadID: threadID.String(), Regola: regola, Punteggio: domain.PuntiRegola[regola],
			Evidenza: evidenza, Chiuso: chiuso,
		})
	}

	// ---- R0: In-Reply-To e References. È l'unico legame che scrive il programma di posta e non una
	// persona: se c'è, quel messaggio risponde davvero a quell'altro. Non è una somiglianza.
	if chiavi := ChiaviCitate(in.InReplyTo, in.Riferimenti); len(chiavi) > 0 {
		righe, err := q.ThreadPerChiaviCitate(ctx, chiavi)
		if err != nil {
			return nil, fmt.Errorf("R0: %w", err)
		}
		for _, r := range righe {
			if !r.ThreadID.Valid {
				continue
			}
			campo := "References"
			if strings.Contains(in.InReplyTo, strings.Trim(r.ChiaveEsterna, "<>")) {
				campo = "In-Reply-To"
			}
			agg(r.ThreadID.UUID, domain.R0Reply,
				fmt.Sprintf("%s punta a un messaggio già agganciato a questa richiesta (%s)", campo, r.ChiaveEsterna),
				r.Stato == db.StatoThreadCHIUSA)
		}
	}

	// ---- R1: la conversazione, se un OPERATORE l'ha collegata. `conversazione.thread_id` non viene più
	// scritto dall'ingest: è una decisione, e per questo può fare da evidenza.
	if riga, err := q.ThreadDellaConversazioneConStato(ctx, in.ConversazioneID); err == nil && riga.ThreadID.Valid {
		agg(riga.ThreadID.UUID, domain.R1Conversazione,
			"la conversazione di Outlook è già stata collegata a questa richiesta da un operatore",
			riga.Stato == db.StatoThreadCHIUSA)
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("R1: %w", err)
	}

	if in.ClienteID.Valid {
		// ---- R4: il riferimento del cliente. «RDO 490020618» identifica la richiesta nel sistema del
		// cliente: se combacia, è la stessa richiesta.
		if in.Riferimento != "" {
			righe, err := q.ThreadPerRiferimentoCliente(ctx, db.ThreadPerRiferimentoClienteParams{
				ClienteID: in.ClienteID.UUID, Riferimento: in.Riferimento})
			if err != nil {
				return nil, fmt.Errorf("R4: %w", err)
			}
			for _, r := range righe {
				agg(r.ThreadID, domain.R4Riferimento,
					"il riferimento "+in.Riferimento+" è quello di questa richiesta",
					r.Stato == db.StatoThreadCHIUSA)
			}
		}
		// ---- R3: un codice di famiglia già identificativo di una richiesta dello STESSO cliente.
		if len(in.Codici) > 0 {
			righe, err := q.ThreadPerCodiciCliente(ctx, db.ThreadPerCodiciClienteParams{
				ClienteID: in.ClienteID.UUID, Codici: maiuscole(in.Codici), Dal: in.finestra()})
			if err != nil {
				return nil, fmt.Errorf("R3: %w", err)
			}
			for _, r := range righe {
				agg(r.ThreadID, domain.R3Codice,
					"il codice "+r.Codice+" è già un identificativo di questa richiesta",
					r.Stato == db.StatoThreadCHIUSA)
			}
		}
		// ---- R2: stesso oggetto, stesso cliente, dentro la finestra. L'oggetto è quello ripulito dai
		// prefissi di risposta, altrimenti «R: X» e «X» sarebbero due oggetti diversi.
		if len(strings.TrimSpace(in.Oggetto)) >= 8 {
			righe, err := q.ThreadPerOggettoCliente(ctx, db.ThreadPerOggettoClienteParams{
				ClienteID: in.ClienteID.UUID, Oggetto: strings.TrimSpace(in.Oggetto), Dal: in.finestra()})
			if err != nil {
				return nil, fmt.Errorf("R2: %w", err)
			}
			for _, r := range righe {
				agg(r.ThreadID, domain.R2Oggetto,
					"stesso oggetto di questa richiesta, entro i giorni della finestra",
					r.Stato == db.StatoThreadCHIUSA)
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
		for _, r := range righe {
			agg(r.ThreadID, domain.R5Buyer,
				"stesso buyer, richiesta aperta negli ultimi giorni", r.Stato == db.StatoThreadCHIUSA)
		}
	}
	return domain.OrdinaCandidati(out), nil
}

// Salva sostituisce i candidati di un messaggio. Sostituisce e non aggiunge: i candidati sono una
// fotografia dello stato di adesso, e un candidato vecchio verso una richiesta che nel frattempo è stata
// unita ad un'altra è peggio di nessun candidato.
func Salva(ctx context.Context, q *db.Queries, messaggioID uuid.UUID, c []domain.Candidato) error {
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
func CalcolaESalva(ctx context.Context, q *db.Queries, in Ingresso) ([]domain.Candidato, error) {
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
// non può stare due volte con due ruoli, e il riferimento viene inserito per primo.
func SalvaCandidatiCodice(ctx context.Context, q *db.Queries, messaggioID uuid.UUID, e domain.Estrazione) error {
	if _, err := q.EliminaCandidatiCodice(ctx, messaggioID); err != nil {
		return err
	}
	ins := func(codice, rev, ruolo, origine, famiglia string, punti int, dove string) error {
		if codice = strings.TrimSpace(codice); codice == "" {
			return nil
		}
		if len(codice) > 60 {
			codice = codice[:60]
		}
		return q.InsertCandidatoCodice(ctx, db.InsertCandidatoCodiceParams{
			MessaggioID: messaggioID, Codice: codice, Ruolo: db.RuoloCodice(ruolo), Rev: tronca(rev, 10),
			Origine: db.OrigineCodice(origine), Famiglia: tronca(famiglia, 120), Punteggio: int16(punti),
			Evidenza: dove,
		})
	}
	if e.Riferimento != "" {
		if err := ins(e.Riferimento, "", domain.RuoloRiferimento, "riferimento", e.RiferimentoNome,
			domain.PuntiRiferimen, e.RiferimentoDove); err != nil {
			return err
		}
	}
	for _, c := range e.Codici {
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

func tronca(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ChiaviCitate normalizza In-Reply-To e References in un elenco di Message-ID confrontabili con
// `messaggio.chiave_esterna`. Le parentesi angolari ci sono negli header e possono esserci o non esserci
// nella proprietà MAPI: si provano tutte e due le forme invece di scommettere su una.
func ChiaviCitate(inReplyTo string, riferimenti []string) []string {
	visti := map[string]bool{}
	var out []string
	agg := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		nudo := strings.Trim(s, "<>")
		if nudo == "" {
			return
		}
		for _, f := range []string{"<" + nudo + ">", nudo} {
			if !visti[f] {
				visti[f] = true
				out = append(out, f)
			}
		}
	}
	for _, s := range strings.Fields(inReplyTo) {
		agg(s)
	}
	for _, s := range riferimenti {
		agg(s)
	}
	// Un thread di posta lungo porta decine di References: le più recenti sono in fondo, e sono quelle
	// che contano. Il tetto evita di costruire un ANY con centinaia di elementi su un forward di forward.
	const max = 40
	if len(out) > max {
		out = out[len(out)-max:]
	}
	return out
}

func maiuscole(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, strings.ToUpper(s))
	}
	return out
}
