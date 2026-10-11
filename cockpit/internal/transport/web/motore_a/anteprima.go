package motore_a

// anteprima.go: l'anteprima dell'autorizzazione dello STEP strutturale in JSON, per la Distinta nuova (giro 5, A1d-P2;
// direttiva A1d §3.3; contratto A1d §3.4 A; v1.1 §4.8; T-A1d-03, T-A1d-28). Derivato dalla PR #18 di Stefano Valo
// (c543e89: motore_a_anteprima.go, senza il gestore HTTP, che sta nell'adattatore transport/web/motore_a.go), con le
// correzioni della revisione: una RFQ inesistente è ErrRFQInesistente, quindi 404 (F-10; la PR rispondeva 200 con
// «il componente non è di questa RFQ»); i figli sono un DTO di questo package e non fascicolo.NodoEffetto; rimozioni,
// revoche, guida e avvisi escono anche come campi, non solo come frasi; la sorgente scelta torna indietro.
//
// Il calcolo è quello del Fascicolo, riusato e non copiato: la richiesta la costruisce l'adattatore con
// richiestaAutorizzazione (fascicolo_rotte.go, privata di transport/web), l'effetto lo calcola
// fascicolo.EffettoAutorizzazione, le frasi sono le righe del modulo legacy fasc_autorizzazione (fascicolo.html),
// nello stesso ordine. Nessuna regola di autorizzazione nuova. La sola differenza dal gestore legacy
// (anteprimaAutorizzazione, che legge sul pool) è la transazione REPEATABLE READ READ ONLY, sempre chiusa con ROLLBACK.
//
// DUE MOTORI (Q-A1d-05, raccomandazione della bozza, A transitoria; F-16): l'anteprima usa il motore del Fascicolo
// legacy, cioè fascicolo.EffettoAutorizzazione con le regole del cliente lette da MotoreDellaRfq (cliente.regole), mentre
// la pagina e /dati usano il motore A, valutazione-3, con le grammatiche dell'indice. Codici e tipi della struttura
// possono quindi differire fra la pagina e l'anteprima: il campo «motore» lo dichiara, e la resa della discordanza è di
// P4. L'anteprima è anche una seconda lettura, in un istante successivo a quello della pagina: la sua transazione è sua.

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// MotoreAnteprima: il valore del campo «motore» dell'anteprima: il Fascicolo legacy (Q-A1d-05).
const MotoreAnteprima = "fascicolo_legacy"

// AnteprimaSTEP: l'anteprima dell'autorizzazione dello STEP strutturale in JSON (contratto §3.4 A; v1.1 §4.8). Gli
// elenchi ci sono sempre ([]), mai null. Le condizioni: Spento (perché non si autorizza), Gia (l'autorizzazione c'è già
// così), Autorizzabile (fascicolo.Effetto.Autorizzabile), Inviabile (il modulo legacy avrebbe il bottone), le deleghe
// con Spenta e Revoca.
//
// Di che cosa parla (IX-04 della revisione di A1d-P2; Q-A1d-05: la discordanza fra i nodi firmati dal Fascicolo legacy e
// quelli del motore A si calcola solo se il JSON dice il file e il componente): Componente (id e codice), Delega e
// Titolare, Allegato, Sha256 e NomeFile del file (in modo delega quello del padre), DocumentoID (il «documento» che il
// gesto rimanda). Il Rif del motore A di un nodo è ancoraggio.RifNodo(Sha256, Chiave). Ci sono anche con il riquadro
// spento; in un rifiuto della richiesta (AnteprimaConErrore) componente, allegato e documento_id sono null.
type AnteprimaSTEP struct {
	Motore           string          `json:"motore"` // MotoreAnteprima: il motore che classifica i nodi qui
	Componente       *ComponenteSTEP `json:"componente"`
	Delega           bool            `json:"delega"`
	Titolare         string          `json:"titolare,omitempty"` // in modo delega: il codice del componente del file
	Allegato         *uuid.UUID      `json:"allegato"`           // il portatore delle righe del file; null se le righe non ci sono
	Sha256           string          `json:"sha256"`
	NomeFile         string          `json:"nome_file"`
	DocumentoID      *uuid.UUID      `json:"documento_id"` // il documento del file (in modo delega, quello del padre)
	Spento           string          `json:"spento,omitempty"`
	Errore           string          `json:"errore,omitempty"` // un rifiuto della richiesta o del Fascicolo (spiegaErrore)
	Gia              string          `json:"gia,omitempty"`
	Autorizzabile    bool            `json:"autorizzabile"`
	Inviabile        bool            `json:"inviabile"`
	Frasi            []string        `json:"frasi"`
	Sorgente         string          `json:"sorgente,omitempty"` // la chiave del nodo sorgente, se indicata o unica
	Figli            []NodoSTEP      `json:"figli"`              // i figli diretti della sorgente: entrano nell'autorità
	Sorgenti         []Scelta        `json:"sorgenti"`           // più radici e nessuna indicata: le radici fra cui scegliere
	Raggruppamenti   []Scelta        `json:"raggruppamenti"`
	Deleghe          []Delega        `json:"deleghe"`
	Guida            int             `json:"guida"`       // i nodi del file che restano guida
	GuidaSotto       []string        `json:"guida_sotto"` // «sotto 7120010: 2»
	Rimozioni        []string        `json:"rimozioni"`   // gli archi della working che il file non ha
	RimozioniSospese string          `json:"rimozioni_sospese,omitempty"`
	Revoche          []string        `json:"revoche"` // le autorizzazioni che si revocano, nominate
	Avvisi           []string        `json:"avvisi"`
	PresaDAtto       string          `json:"presa_d_atto,omitempty"`
	Bottone          string          `json:"bottone,omitempty"`
	Firma            string          `json:"firma,omitempty"` // solo dal ruolo operatore in su
}

// ComponenteSTEP: il componente dell'anteprima, piatto (nessun db.Componente nel DTO).
type ComponenteSTEP struct {
	ID     uuid.UUID `json:"id"`
	Codice string    `json:"codice"`
}

// NodoSTEP: un nodo del file come lo dice l'anteprima (i campi di fascicolo.NodoEffetto, con gli stessi nomi JSON, e lo
// stato come testo).
type NodoSTEP struct {
	Chiave   string `json:"chiave"`
	Nome     string `json:"nome"`
	Codice   string `json:"codice"`
	Qta      int32  `json:"qta"`
	Figli    int    `json:"figli"` // quanti figli ha nel file
	Deciso   string `json:"deciso"`
	Scartato bool   `json:"scartato"`
	Stato    string `json:"stato"`
}

// Scelta: una scelta della finestra (una radice da indicare, un raggruppamento da dichiarare): la chiave del nodo, che
// il gesto rimanda, e il testo del modulo legacy.
type Scelta struct {
	Chiave    string `json:"chiave"`
	Etichetta string `json:"etichetta"`
}

// Delega: una delega possibile (fascicolo.DelegaPossibile) con il testo del modulo legacy, il motivo per cui è spenta e
// le autorizzazioni che revocherebbe, nominate. Revoca è [] e mai null.
type Delega struct {
	Chiave    string   `json:"chiave"`
	Etichetta string   `json:"etichetta"`
	Spenta    string   `json:"spenta,omitempty"`
	Revoca    []string `json:"revoca"`
}

// AnteprimaConErrore: l'anteprima di un rifiuto (la richiesta non valida, «il componente non è di questa RFQ»), con la
// stessa forma e gli elenchi vuoti.
func AnteprimaConErrore(testo string) AnteprimaSTEP {
	return AnteprimaSTEP{Errore: testo}.conElenchi()
}

// conElenchi: gli elenchi vuoti diventano [] (mai null nel JSON), anche la revoca di ogni delega; Motore c'è sempre.
func (a AnteprimaSTEP) conElenchi() AnteprimaSTEP {
	a.Motore = MotoreAnteprima
	if a.Frasi == nil {
		a.Frasi = []string{}
	}
	if a.Figli == nil {
		a.Figli = []NodoSTEP{}
	}
	if a.Sorgenti == nil {
		a.Sorgenti = []Scelta{}
	}
	if a.Raggruppamenti == nil {
		a.Raggruppamenti = []Scelta{}
	}
	if a.Deleghe == nil {
		a.Deleghe = []Delega{}
	}
	for i := range a.Deleghe {
		if a.Deleghe[i].Revoca == nil {
			a.Deleghe[i].Revoca = []string{}
		}
	}
	for _, l := range []*[]string{&a.GuidaSotto, &a.Rimozioni, &a.Revoche, &a.Avvisi} {
		if *l == nil {
			*l = []string{}
		}
	}
	return a
}

// AnteprimaSTEP: l'anteprima per il thread, in una transazione REPEATABLE READ READ ONLY sua, con il limite delle
// regole (se c'è). firma: chi chiede arriva almeno al ruolo operatore (lo decide l'adattatore).
//   - la RFQ non esiste → ErrRFQInesistente (GetThread nella stessa transazione, prima dell'effetto);
//   - un rifiuto del Fascicolo (fascicolo.Rifiuto: il componente non è della RFQ, il nodo non è una radice, …) torna
//     così com'è: l'adattatore lo dice con spiegaErrore, in una risposta 200 con «errore», come il modulo legacy;
//   - una lettura non finita entro il limite (quello delle regole, o LimiteDiRipiego: l'anteprima ha sempre un limite)
//     → ErrTempo (HTTP-01 della revisione di A1d-P2: 504, non 500);
//   - un errore del database torna avvolto in ErrLettura; un SQLSTATE 25006 (una lettura che scriverebbe o
//     prenderebbe un lucchetto) va anche nel log.
//
// Le regole del motore A non servono qui: l'anteprima non dipende da Regole.Pronta().
func (s Servizio) AnteprimaSTEP(ctx context.Context, thread uuid.UUID, req fascicolo.RichiestaAutorizzazione, firma bool) (AnteprimaSTEP, error) {
	ctx, cancel := s.conLimite(ctx)
	defer cancel()
	var e fascicolo.Effetto
	err := nelloSnapshot(ctx, s.Pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if err := esisteIn(ctx, q, thread); err != nil {
			return err
		}
		var err error
		e, err = fascicolo.EffettoAutorizzazione(ctx, q, thread, req)
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "25006" {
			s.log().Error("motore A: l'anteprima dello STEP ha chiesto di scrivere in una transazione in sola lettura",
				"thread", thread, "componente", req.Componente, "sqlstate", pg.Code, "err", err)
		}
		var rf fascicolo.Rifiuto
		if err != nil && !errors.As(err, &rf) {
			return fmt.Errorf("%w: l'anteprima dello STEP: %w", ErrLettura, err)
		}
		return err
	})
	var rf fascicolo.Rifiuto
	if err != nil && !errors.Is(err, ErrRFQInesistente) && !errors.As(err, &rf) && scaduto(ctx) {
		return AnteprimaSTEP{}, s.tempoScaduto(ctx, err)
	}
	if err != nil {
		return AnteprimaSTEP{}, err
	}
	return AnteprimaDa(e, firma), nil
}

// AnteprimaDa: dal risultato del dominio al JSON, puro (si prova in L1; T-A1d-18). Le frasi sono le righe che scrive il
// modulo legacy fasc_autorizzazione, nello stesso ordine (T-A1d-28): il titolo; con il riquadro spento nient'altro (il
// motivo sta in Spento); poi «già», la domanda delle radici, e con la sorgente i figli diretti, la guida, le rimozioni,
// le revoche e gli avvisi. Senza firma (sotto il ruolo operatore) la firma non c'è e niente è inviabile, come nel
// modulo legacy, che senza Scrive non ha il form.
func AnteprimaDa(e fascicolo.Effetto, firma bool) AnteprimaSTEP {
	codice := e.Componente.Codice
	titolo := "Autorizzazione: "
	if e.Delega {
		titolo = "Delega: "
	}
	titolo += cmp.Or(e.NomeFile, "lo STEP") + " per " + codice
	if e.Titolare != "" {
		titolo += " (nel file, sotto " + e.Titolare + ")"
	}
	out := AnteprimaSTEP{Autorizzabile: e.Autorizzabile(), Frasi: []string{titolo},
		Componente: &ComponenteSTEP{ID: e.Componente.ComponenteID, Codice: codice}, Delega: e.Delega, Titolare: e.Titolare,
		Sha256: e.Sha256, NomeFile: e.NomeFile}
	if e.Allegato != uuid.Nil {
		a := e.Allegato
		out.Allegato = &a
	}
	if e.Documento.DocumentoID != uuid.Nil {
		d := e.Documento.DocumentoID
		out.DocumentoID = &d
	}
	if e.Spento != "" {
		// il riquadro spento dice solo perché: niente scelte, niente bottone, niente firma, niente da inviare
		out.Spento = e.Spento
		return out.conElenchi()
	}
	if e.Gia != "" {
		out.Gia = e.Gia
		out.Frasi = append(out.Frasi, e.Gia+".")
	}
	if e.Sorgente == nil && len(e.Radici) > 0 {
		// più radici e nessuna indicata: la persona sceglie, e nessuna è scelta in partenza (U7)
		out.Frasi = append(out.Frasi, fmt.Sprintf("Il file ha %d radici: quale è %s? Nessuna è scelta.", len(e.Radici), codice))
		for _, n := range e.Radici {
			out.Sorgenti = append(out.Sorgenti, Scelta{Chiave: n.Chiave, Etichetta: fmt.Sprintf("%s (%d figli nel file)", n.Nome, n.Figli)})
		}
	}
	if e.Sorgente != nil {
		out.Sorgente = e.Sorgente.Chiave
		out.Frasi = append(out.Frasi, fmt.Sprintf("%d %s nell'autorità di %s:", len(e.Figli),
			plurale(len(e.Figli), "figlio diretto entra", "figli diretti entrano"), codice))
		for _, n := range e.Figli {
			out.Figli = append(out.Figli, nodoSTEP(n))
		}
		sotto := ""
		for _, x := range e.GuidaSotto {
			sotto += " · " + x
		}
		out.Guida, out.GuidaSotto = e.Guida, append([]string(nil), e.GuidaSotto...)
		out.Frasi = append(out.Frasi, fmt.Sprintf("%d %s%s: si vedono, e non entrano nella BOM senza un'altra autorizzazione.",
			e.Guida, plurale(e.Guida, "nodo del file resta guida", "nodi del file restano guida"), sotto))
		out.Rimozioni = append([]string(nil), e.Rimozioni...)
		for _, x := range e.Rimozioni {
			out.Frasi = append(out.Frasi, "Rimozione: "+x)
		}
		out.RimozioniSospese = e.RimozioniSospese
		if e.RimozioniSospese != "" {
			out.Frasi = append(out.Frasi, "Rimozioni: "+e.RimozioniSospese+".")
		}
		out.Revoche = append([]string(nil), e.Revoche...)
		for _, x := range e.Revoche {
			out.Frasi = append(out.Frasi, "Un componente, un'autorizzazione: "+x+" si revoca.")
		}
		out.Avvisi = append([]string(nil), e.Avvisi...)
		for _, x := range e.Avvisi {
			out.Frasi = append(out.Frasi, x+".")
		}
		out.PresaDAtto = e.PresaDAtto
		for _, g := range e.Raggruppamenti {
			out.Raggruppamenti = append(out.Raggruppamenti, Scelta{Chiave: g.Nodo.Chiave,
				Etichetta: fmt.Sprintf("%s è %s (un contenitore): i suoi %d figli diventano figli diretti", g.Nodo.Nome, codice, len(g.Figli))})
		}
		for _, d := range e.Deleghe {
			out.Deleghe = append(out.Deleghe, Delega{Chiave: d.Nodo.Chiave,
				Etichetta: fmt.Sprintf("Autorizza questo STEP anche per %s (%s, %d figli nel file)",
					cmp.Or(d.Codice, "il componente di "+d.Nodo.Nome+", quando sarà deciso"), d.Nodo.Nome, d.Nodo.Figli),
				Spenta: d.Spenta, Revoca: append([]string(nil), d.Revoca...)})
		}
		switch {
		case e.Gia == "":
			out.Bottone = e.Bottone()
		case len(e.Deleghe) > 0 || len(e.Raggruppamenti) > 0:
			out.Bottone = "Autorizza le scelte"
		}
	}
	if firma {
		out.Firma = e.Firma
	}
	// la condizione del bottone di fasc_autorizzazione: il form c'è solo con Scrive, il bottone solo senza «già» oppure
	// con deleghe o raggruppamenti da scegliere
	out.Inviabile = firma && e.Spento == "" && e.Sorgente != nil && (e.Gia == "" || len(e.Deleghe) > 0 || len(e.Raggruppamenti) > 0)
	return out.conElenchi()
}

// nodoSTEP: la copia di un fascicolo.NodoEffetto nel DTO dell'anteprima.
func nodoSTEP(n fascicolo.NodoEffetto) NodoSTEP {
	return NodoSTEP{Chiave: n.Chiave, Nome: n.Nome, Codice: n.Codice, Qta: n.Qta, Figli: n.Figli, Deciso: n.Deciso,
		Scartato: n.Scartato, Stato: string(n.Stato)}
}

// plurale: la copia di plurale di transport/web (thread.go), che il sottopackage non importa.
func plurale(n int, uno, molti string) string {
	if n == 1 {
		return uno
	}
	return molti
}
