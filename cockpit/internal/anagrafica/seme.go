// Package anagrafica semina clienti, domini e buyer da un file, una volta sola e senza
// sovrascrivere niente (voce 6.6, blocco 3).
//
// # Perché un file e non un seed dentro il binario
//
// I clienti di Promatec, i loro domini, i nomi dei buyer e le forme dei loro codici sono dati
// dell'azienda. Il repository è pubblico: nel binario non ci vanno. Il file lo tiene chi
// amministra il Cockpit, accanto a `cockpit.toml`, e questo pacchetto sa soltanto come si legge.
//
// # Il cancello
//
// Prima di scrivere una riga, TUTTE le regole di TUTTI i clienti passano da `domain.ValidaRegole`.
// Se una sola regola non rispetta lo schema, o ha un esempio che non corrisponde alla propria
// regex, il seme non parte affatto: non «quel cliente viene saltato», proprio non parte.
//
// È la stessa regola di D17 applicata all'altra porta. La schermata Anagrafica rifiuta una regola
// rotta scritta a mano; senza questo controllo, lo stesso identico errore scritto in un file
// entrerebbe in database senza che nessuno lo guardi, e ci resterebbe — riconoscendo niente, in
// silenzio, per mesi.
package anagrafica

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"promatec/cockpit/internal/domain"
	"promatec/cockpit/internal/platform/db"
)

// Seme è il file. Un oggetto con una sola chiave, così ci si può aggiungere altro (articoli, nel
// blocco 5) senza cambiare la forma di quello che c'è.
type Seme struct {
	Clienti []ClienteSeme `json:"clienti"`
}

type ClienteSeme struct {
	RagioneSociale string          `json:"ragione_sociale"`
	CartellaNas    string          `json:"cartella_nas"`
	Lingua         string          `json:"lingua,omitempty"`
	Peso           int             `json:"peso,omitempty"`
	PortaleUrl     string          `json:"portale_url,omitempty"`
	PortaleNote    string          `json:"portale_note,omitempty"`
	Note           string          `json:"note,omitempty"`
	Domini         []string        `json:"domini,omitempty"`
	Buyer          []BuyerSeme     `json:"buyer,omitempty"`
	Regole         json.RawMessage `json:"regole,omitempty"`
}

type BuyerSeme struct {
	Cognome  string `json:"cognome"`
	Nome     string `json:"nome,omitempty"`
	Email    string `json:"email,omitempty"`
	Telefono string `json:"telefono,omitempty"`
	Ruolo    string `json:"ruolo,omitempty"`
	Tipo     string `json:"tipo,omitempty"`
	Lingua   string `json:"lingua,omitempty"`
	Note     string `json:"note,omitempty"`
}

// Esito è il resoconto: si legge per sapere che cosa è successo davvero, non per sapere che è
// andato bene.
type Esito struct {
	ClientiCreati   []string
	ClientiPresenti []string // c'erano già: lasciati com'erano, nemmeno un campo toccato
	DominiAggiunti  int
	BuyerCreati     int
	Avvisi          []string
	// Come per i fornitori (7B.5): i domini e le email dei buyer scritti adesso sono le chiavi con
	// cui la posta già arrivata va riguardata. Senza il ricalcolo, seminare l'anagrafica non sposta
	// di un messaggio il quadrante «Da validare».
	DominiScritti    []string
	IndirizziScritti []string
}

// Leggi legge e CONVALIDA il file. Ogni errore qui è un errore del file, e il messaggio dice la
// riga (cioè il cliente) in cui sta.
func Leggi(r io.Reader) (Seme, error) {
	var s Seme
	d := json.NewDecoder(senzaBOM(r))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return s, fmt.Errorf("seme: %w", err)
	}
	if len(s.Clienti) == 0 {
		return s, errors.New("seme: non contiene nessun cliente")
	}
	viste := map[string]bool{}
	domini := map[string]string{}
	for i, c := range s.Clienti {
		dove := fmt.Sprintf("cliente %d (%s)", i+1, primo(c.RagioneSociale, c.CartellaNas))
		if strings.TrimSpace(c.RagioneSociale) == "" || strings.TrimSpace(c.CartellaNas) == "" {
			return s, fmt.Errorf("seme: %s: servono ragione_sociale e cartella_nas", dove)
		}
		if viste[c.CartellaNas] {
			return s, fmt.Errorf("seme: la cartella NAS %q compare due volte: due clienti nella stessa cartella si sovrascriverebbero i disegni", c.CartellaNas)
		}
		viste[c.CartellaNas] = true
		if c.Peso < 0 || c.Peso > 15 {
			return s, fmt.Errorf("seme: %s: peso %d fuori da 0–15", dove, c.Peso)
		}
		for _, dom := range c.Domini {
			dom = strings.ToLower(strings.TrimSpace(dom))
			if strings.Contains(dom, "@") {
				return s, fmt.Errorf("seme: %s: %q è un indirizzo, non un dominio", dove, dom)
			}
			if altro, doppio := domini[dom]; doppio {
				return s, fmt.Errorf("seme: il dominio %s è assegnato sia a %q sia a %q: appartiene a un cliente solo", dom, altro, c.RagioneSociale)
			}
			domini[dom] = c.RagioneSociale
		}
		// Il cancello di D17: l'esempio deve corrispondere, anche quando la regola arriva da un file.
		if len(c.Regole) > 0 {
			if _, err := domain.ValidaRegole(c.Regole); err != nil {
				return s, fmt.Errorf("seme: %s: %w", dove, err)
			}
		}
	}
	return s, nil
}

// LeggiFile è `Leggi` su un percorso.
func LeggiFile(percorso string) (Seme, error) {
	f, err := os.Open(percorso)
	if err != nil {
		return Seme{}, err
	}
	defer f.Close()
	return Leggi(f)
}

// Semina scrive ciò che manca e non tocca ciò che c'è.
//
// Un cliente già presente viene SALTATO per intero, non aggiornato: il file è una fotografia di
// un foglio Excel, e il database è dove qualcuno ha già corretto a mano quello che il foglio
// sbagliava. Un seme che «aggiorna» cancellerebbe quelle correzioni ogni volta che lo si rilancia,
// ed è esattamente il genere di cosa che si rilancia per abitudine.
func Semina(ctx context.Context, pool *pgxpool.Pool, s Seme) (Esito, error) {
	var e Esito
	tx, err := pool.Begin(ctx)
	if err != nil {
		return e, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)

	for _, c := range s.Clienti {
		esistente, err := q.GetClientePerCartella(ctx, c.CartellaNas)
		switch {
		case err == nil:
			e.ClientiPresenti = append(e.ClientiPresenti, c.RagioneSociale)
			// I domini sì: aggiungerne uno nuovo a un cliente che c'è già non sovrascrive niente,
			// e un dominio mancante è il motivo più comune per cui la posta di un cliente censito
			// non viene riconosciuta.
			scritti, avvisi := seminaDomini(ctx, q, c, esistente.ClienteID)
			e.DominiAggiunti += len(scritti)
			e.DominiScritti = append(e.DominiScritti, scritti...)
			e.Avvisi = append(e.Avvisi, avvisi...)
			continue
		case !errors.Is(err, pgx.ErrNoRows):
			return e, err
		}

		regole := c.Regole
		if len(regole) == 0 {
			regole = json.RawMessage("{}")
		}
		nuovo, err := creaCliente(ctx, q, c, regole)
		if err != nil {
			return e, fmt.Errorf("cliente %s: %w", c.RagioneSociale, err)
		}
		e.ClientiCreati = append(e.ClientiCreati, c.RagioneSociale)

		scritti, avvisi := seminaDomini(ctx, q, c, nuovo.ClienteID)
		e.DominiAggiunti += len(scritti)
		e.DominiScritti = append(e.DominiScritti, scritti...)
		e.Avvisi = append(e.Avvisi, avvisi...)

		for _, b := range c.Buyer {
			if strings.TrimSpace(b.Cognome) == "" {
				continue
			}
			tipo := db.TipoBuyer(primo(b.Tipo, string(db.TipoBuyerBuyer)))
			if !tipo.Valid() {
				return e, fmt.Errorf("cliente %s, buyer %s: tipo %q non valido", c.RagioneSociale, b.Cognome, b.Tipo)
			}
			if _, err := q.InsertBuyer(ctx, db.InsertBuyerParams{
				ClienteID: nuovo.ClienteID, Cognome: b.Cognome, Nome: ptxt(b.Nome),
				Email: ptxt(strings.ToLower(b.Email)), Telefono: ptxt(b.Telefono), Ruolo: ptxt(b.Ruolo),
				Tipo: tipo, Lingua: ptxt(b.Lingua), Origine: db.OrigineAnagraficaExcel, Note: ptxt(b.Note),
			}); err != nil {
				return e, fmt.Errorf("cliente %s, buyer %s: %w", c.RagioneSociale, b.Cognome, err)
			}
			e.BuyerCreati++
			if em := strings.ToLower(strings.TrimSpace(b.Email)); em != "" {
				e.IndirizziScritti = append(e.IndirizziScritti, em)
			}
		}
	}
	return e, tx.Commit(ctx)
}

func creaCliente(ctx context.Context, q *db.Queries, c ClienteSeme, regole json.RawMessage) (db.Cliente, error) {
	return q.InsertCliente(ctx, db.InsertClienteParams{
		CartellaNas: c.CartellaNas, RagioneSociale: c.RagioneSociale, Lingua: ptxt(c.Lingua),
		PortaleUrl: ptxt(c.PortaleUrl), PortaleNote: ptxt(c.PortaleNote), Peso: int16(c.Peso), Regole: regole,
	})
}

// seminaDomini non sposta mai un dominio già assegnato: lo segnala e prosegue. Un seme che si
// ferma a metà per un dominio è un seme che lascia il database a metà; uno che sposta il dominio
// cambia il cliente di tutta la posta già arrivata (T8).
func seminaDomini(ctx context.Context, q *db.Queries, c ClienteSeme, id uuid.UUID) (scritti []string, avvisi []string) {
	for _, dom := range c.Domini {
		dom = strings.ToLower(strings.TrimSpace(dom))
		if dom == "" {
			continue
		}
		if altro, err := q.GetClientePerDominio(ctx, dom); err == nil {
			if altro.ClienteID != id {
				avvisi = append(avvisi, fmt.Sprintf("il dominio %s è già di %s: lasciato dov'era (nel seme era di %s)", dom, altro.RagioneSociale, c.RagioneSociale))
			}
			continue
		}
		if err := q.InsertDominioCliente(ctx, db.InsertDominioClienteParams{Lower: dom, ClienteID: id}); err != nil {
			avvisi = append(avvisi, fmt.Sprintf("dominio %s: %v", dom, err))
			continue
		}
		scritti = append(scritti, dom)
	}
	return scritti, avvisi
}

func ptxt(v string) pgtype.Text {
	v = strings.TrimSpace(v)
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func primo(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// senzaBOM toglie la firma UTF-8 che Windows mette in testa a un file salvato con Blocco note o con
// `Out-File`. Non è un dettaglio da puristi: senza, il file viene rifiutato con «invalid character
// '\ufeff' looking for beginning of value», che non dice a nessuno che cosa fare. Il contenuto è
// giusto, il problema sono tre byte invisibili.
func senzaBOM(r io.Reader) io.Reader {
	b := bufio.NewReader(r)
	if primi, err := b.Peek(3); err == nil && primi[0] == 0xEF && primi[1] == 0xBB && primi[2] == 0xBF {
		_, _ = b.Discard(3)
	}
	return b
}
