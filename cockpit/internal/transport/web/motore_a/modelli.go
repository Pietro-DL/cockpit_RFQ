package motore_a

// modelli.go: i dati della pagina della Distinta nuova e del JSON di /motore-a/dati (giro 5, A1d-P2; contratto A1d v1,
// §2.2 e §2.4). Derivato dalla PR #18 di Stefano Valo (c543e89: paginaMotoreA, testaMotoreA, faseRegistrata,
// componiPaginaMotoreA, testaDa, vistaMotoreA, prodottoScelto), con i tipi esportati perché il package padre li scriva e,
// in P4, il template li legga.
//
// Il JSON di /dati porta i campi del contratto §2.4, con i tag snake_case: thread_id, vista, prodotto, testa, esito
// (valutazione.EsitoThread con i suoi tag), confronto (confronto.Esito), errore, calcolato. L'indice delle ricerche e
// Scrive restano fuori dal JSON (Q-A1d-11 = A, raccomandazione della bozza: l'indice serve al rendering di P4; un DTO
// dell'editor sarà un'estensione esplicita del contratto). Da A1d-P2 un calcolo che non riesce è un codice HTTP
// dell'adattatore (404, 503, 504, 500), quindi in una risposta 200 «errore» è sempre vuoto: il campo resta per il
// contratto e per la pagina di P4.
//
// V1 (emendamento di V1 §9.9; dati_v1.go) aggiunge i campi del modello della Distinta unica, con i nomi di PE-01,
// PE-15, PE-16 (decisioni dell'ondata A): gruppi (con l'impronta_contenuto di un'importazione fatta adesso e i nodi con
// la completezza e le diagnostiche correnti), motivo_senza_gruppi, fonte (con lo STEP secondario), f8_stato e
// bozza_del_prodotto (il gruppo corrente della sorgente). L'esito resta valutazione.EsitoThread, com'è.

import (
	"context"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/propostabom"
	"promatec/cockpit/internal/core/valutazione"
)

// Le viste della pagina (contratto §2.1): schede, non pagine diverse.
const (
	VistaDistinta  = "distinta"
	VistaDocumenti = "documenti"
	VistaRFQ       = "rfq"
)

// Pagina: i dati della pagina e del JSON di /dati (contratto §2.2, §2.4).
type Pagina struct {
	ThreadID  uuid.UUID                `json:"thread_id"`
	Vista     string                   `json:"vista"`    // "distinta" | "documenti" | "rfq"
	Prodotto  string                   `json:"prodotto"` // il Rif scelto; "" senza prodotti
	Testa     Testa                    `json:"testa"`
	Esito     *valutazione.EsitoThread `json:"esito"`     // nil solo nella pagina di P4 con un errore
	Confronto *confronto.Esito         `json:"confronto"` // come sopra
	Errore    string                   `json:"errore"`    // vuoto in una risposta 200 di /dati (commento di testa)
	Scrive    bool                     `json:"-"`         // i pulsanti dei gesti sono attivi (almeno operatore)
	Indice    *Indice                  `json:"-"`         // le ricerche (contratto §2.3), per il rendering di P4
	Calcolato time.Time                `json:"calcolato"` // Fotografia.PresaIl: il now() della transazione

	// V1 (emendamento di V1 §9.9): il modello della Distinta unica per il prodotto scelto (dati_v1.go). Tutti omitempty: una
	// pagina composta senza V1 (la pagina di P4 con un errore, i casi del costruttore) resta byte per byte quella di prima, e
	// la pagina legge un elenco assente come vuoto.
	Gruppi            []propostabom.GruppoDelModello `json:"gruppi,omitempty"`
	MotivoSenzaGruppi *propostabom.MotivoSenzaGruppi `json:"motivo_senza_gruppi,omitempty"`
	Fonte             *FonteDelProdotto              `json:"fonte,omitempty"`
	F8Stato           string                         `json:"f8_stato,omitempty"`
	BozzaDelProdotto  *BozzaDelProdotto              `json:"bozza_del_prodotto,omitempty"`
}

// Testa: la testa della pagina (contratto §2.2).
type Testa struct {
	Cliente            string `json:"cliente"`
	Oggetto            string `json:"oggetto"`
	RiferimentoCliente string `json:"riferimento_cliente"`
	StatoThread        string `json:"stato_thread"` // "APERTA" | "CHIUSA" (= Esito.Fascicolo.FaseThread)
	Fase               *Fase  `json:"fase"`         // nil = nessuna fase aperta
}

// Fase: la fase di lavoro registrata, dalla vista v_thread_fase (EB7-3), letta nello snapshot e non dall'esito.
type Fase struct {
	Nome      string    `json:"nome"`
	Etichetta string    `json:"etichetta"` // fase_catalogo.descrizione
	Inizio    time.Time `json:"inizio"`
	GgInFase  int32     `json:"gg_in_fase"`
	Semaforo  string    `json:"semaforo"` // bianco | rosso | giallo | verde (migrations/0001, la vista)
	Terminale bool      `json:"terminale"`
}

// RichiestaPagina: che cosa chiede chi apre la pagina. Scrive lo decide l'adattatore dal ruolo (almeno operatore).
// SenzaV1: la pagina non vuole le aggiunte di V1 (gruppi, fonte, bozza del prodotto, …), che costano un'importazione per
// gruppo con uno STEP: la pagina HTML di /motore-a e il suo frammento non le leggono, /dati sì. Il valore zero le
// calcola, così /dati resta com'è [T].
type RichiestaPagina struct {
	Thread   uuid.UUID
	Vista    string
	Prodotto string
	Scrive   bool
	SenzaV1  bool
}

// Pagina: il calcolo (Calcola, con i suoi errori) e la pagina composta. La composizione (ComponiPagina, con NuovoIndice)
// gira nella goroutine del calcolo, sotto lo stesso limite e lo stesso recover (R-06 della revisione di A1d-P2): un panic
// nella composizione è ErrPanic, con la pila nel log, e non una connessione interrotta. Un errore torna così com'è:
// l'adattatore lo traduce nel codice HTTP.
func (s Servizio) Pagina(ctx context.Context, rp RichiestaPagina) (*Pagina, error) {
	componi := s.componi
	if componi == nil {
		componi = ComponiPagina
	}
	var p *Pagina // scritta nella goroutine, letta qui dopo la consegna sul canale (happens-before)
	if _, err := s.calcolaProtetto(ctx, rp.Thread, func(r *Risultato) { p = componi(r, rp) }); err != nil {
		return nil, err
	}
	return p, nil
}

// ComponiPagina: la pagina da un calcolo riuscito, pura (si prova in L1). Le ricerche e l'esito vengono dalla stessa
// fotografia (direttiva §3.4).
func ComponiPagina(r *Risultato, rp RichiestaPagina) *Pagina {
	p := &Pagina{ThreadID: rp.Thread, Vista: VistaDa(rp.Vista), Scrive: rp.Scrive}
	if r == nil || r.Thread == nil || r.EsitoThread == nil {
		return p
	}
	p.Testa = testaDa(r.Foto, r.Thread, r.Fase)
	p.Esito = r.EsitoThread
	c := r.Confronto
	p.Confronto = &c
	p.Indice = NuovoIndice(r.Thread, r.EsitoThread, p.Confronto, &r.Dichiarazioni)
	p.Prodotto = ProdottoScelto(r.EsitoThread, rp.Prodotto)
	p.Calcolato = r.Foto.PresaIl
	if !rp.SenzaV1 {
		componiV1(p, r)
	}
	return p
}

// VistaDa: la vista chiesta, se è una delle tre; altrimenti distinta.
func VistaDa(v string) string {
	switch v {
	case VistaDistinta, VistaDocumenti, VistaRFQ:
		return v
	}
	return VistaDistinta
}

// ProdottoScelto: il Rif chiesto, se è fra i prodotti valutati; altrimenti il primo; "" senza prodotti.
func ProdottoScelto(e *valutazione.EsitoThread, rif string) string {
	if e == nil || len(e.ProdottiValutati) == 0 {
		return ""
	}
	for _, p := range e.ProdottiValutati {
		if p.Rif == rif {
			return p.Rif
		}
	}
	return e.ProdottiValutati[0].Rif
}

// testaDa: la testa dalla fotografia, pura (contratto §2.2). Cliente: Foto.Clienti[ClienteID].RagioneSociale; Oggetto e
// RiferimentoCliente: thread_offerta (nil = ""); StatoThread: thread_offerta.stato.
func testaDa(f fotorfq.Fotografia, t *fotorfq.Thread, fase *Fase) Testa {
	out := Testa{StatoThread: t.Stato, Fase: fase}
	for _, c := range f.Clienti {
		if c.ID == t.ClienteID {
			out.Cliente = c.RagioneSociale
			break
		}
	}
	if t.Oggetto != nil {
		out.Oggetto = *t.Oggetto
	}
	if t.Riferimento != nil {
		out.RiferimentoCliente = *t.Riferimento
	}
	return out
}
