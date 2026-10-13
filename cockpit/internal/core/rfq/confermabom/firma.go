// Package confermabom tiene le regole delle conferme strutturali della BOM che f.1 e f.2 condividono (giro 5,
// A1d, P4; piano esecutivo B2-B5 §1.1, decisione D-T-09 a): la firma del perimetro di un prodotto e la validita'
// dell'ultima conferma. Una sola definizione, usata dalla conferma formato 2 (R5, via distintabozza) e dalla
// conferma formato 1 (core/rfq/fascicolo): il package non importa nessuno dei due, e nessun ciclo nasce.
//
// Qui solo funzioni pure (B2.3, parte pura): niente query, niente sqlc, niente database. Chi chiama legge le righe
// (componente, componente_relazione, conferma_strutturale, working_contatore) e le passa con i tipi di questo file;
// la lettura dello stato della RFQ e la scrittura di conferma_strutturale sono di B2.3 (parte con il database) e
// B2.7.
package confermabom

import (
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/jsoncanonico"
)

// VersioneFirma e' la versione della forma canonica del perimetro. Entra nella forma, come jsoncanonico.Versione:
// cambiarla cambia tutte le firme, e una conferma registrata con l'altra versione resta valida solo per contatore.
// Si cambia solo con un commit che lo dichiara.
//
// firma-perimetro-2 (13/10/2026): D-FIRMA-01 = b [U] (direttiva 12 §5). La firma porta il codice LETTERALE e non piu'
// il codice canonico, che dipendeva dalle regole del cliente: un cambio delle convenzioni di codifica non e' un cambio
// della struttura meccanica, e non deve far decadere una conferma strutturale immutata. firma-perimetro-1 (con il
// canonico) non e' mai stata registrata: nessuna firma da migrare.
const VersioneFirma = "firma-perimetro-2"

// I tipi di componente (enum tipo_componente della 0018/0020). Ripetuti qui per non importare platform/db: il
// package resta senza sqlc.
var tipiComponente = map[string]bool{"finito": true, "sottoassieme": true, "sciolto": true, "commerciale": true}

// Componente e' una riga di componente della RFQ, come la legge chi chiama. Rev e Descrizione ci sono perche'
// chi certifica le righe (conferma_strutturale_componente) le vuole, ma NON entrano nella firma: una revisione
// solo CAD lascia la struttura com'e' (contratto rev. 3 §5.1, [U] direttiva 01 §4; asse «dati» del contatore).
type Componente struct {
	ID uuid.UUID
	// CodiceLetterale e' componente.codice com'e' scritto, senza toccarlo: «7120001_prt» e «7120001_PRT» sono due
	// firme diverse, come per il contatore (0022, working_conta: codice). E' il solo codice della firma: il codice
	// canonico (le regole del cliente) non entra, ne' come ingresso ne' calcolato qui (D-FIRMA-01 = b [U]). Collisioni
	// di identita' e incoerenze di nomenclatura si controllano altrove (ux_componente_thread_codice, il riepilogo
	// della conferma dell'albero), non con la firma.
	CodiceLetterale string
	Tipo            string // tipo_componente: finito, sottoassieme, sciolto, commerciale
	Archiviato      bool   // archiviato_il non nullo
	// StepStrutturale e' componente.step_strutturale_id (solo un finito lo ha: ck_componente_step_solo_finito).
	StepStrutturale uuid.NullUUID
	Rev             string // non entra nella firma
	Descrizione     string // non entra nella firma
}

// Arco e' una riga di componente_relazione della RFQ. Posizione non entra nella firma (asse «dati», Δ28).
type Arco struct {
	Padre, Figlio uuid.UUID
	Qta           int32
	Posizione     string // non entra nella firma
}

// Perimetro e' il perimetro di un prodotto: il prodotto e i componenti raggiungibili da lui per gli archi verso
// figli non archiviati, con gli archi percorsi. Componenti in ordine di ID (il prodotto fra gli altri), archi in
// ordine di padre e figlio: lo stesso perimetro da' sempre lo stesso valore.
type Perimetro struct {
	Prodotto   uuid.UUID
	Componenti []Componente
	Archi      []Arco
}

// ErrIngresso e' un ingresso che non descrive una working possibile (un arco verso un componente che non c'e', due
// righe con lo stesso ID, una quantita' fuori misura): un errore di chi chiama, non una firma diversa.
var ErrIngresso = errors.New("confermabom: ingresso non valido")

func ingresso(formato string, arg ...any) error {
	return fmt.Errorf("%w: %s", ErrIngresso, fmt.Sprintf(formato, arg...))
}

// CalcolaPerimetro calcola il perimetro del prodotto sui componenti e sugli archi della RFQ. Pura. Ogni componente
// nominato da un arco passato deve essere fra i componenti passati (altrimenti ErrIngresso): il modo semplice e'
// passare tutti i componenti e tutti gli archi della RFQ; passare i soli raggiungibili va bene solo con i soli
// archi fra loro.
//
// La regola e' quella del controllo al COMMIT della 0022 (conferma_strutturale_completa, (c) «perimetro chiuso»): dal
// prodotto si scende per ogni arco verso un figlio non archiviato; un figlio archiviato resta fuori con il suo arco.
// Il prodotto c'e' sempre, anche archiviato, con il suo segno. Per un prodotto attivo coincide con il perimetro del
// Motore A (valutazione/bom.go, perimetro: i componenti attivi raggiungibili per le relazioni fra componenti attivi).
//
// Errori (ErrIngresso): prodotto assente; due componenti con lo stesso ID; componente senza codice o con un tipo
// sconosciuto; arco verso o da un componente assente, da un componente a se stesso, ripetuto, con qta < 1.
func CalcolaPerimetro(prodotto uuid.UUID, componenti []Componente, archi []Arco) (Perimetro, error) {
	perID := make(map[uuid.UUID]Componente, len(componenti))
	for _, c := range componenti {
		if _, gia := perID[c.ID]; gia {
			return Perimetro{}, ingresso("due componenti con l'ID %s", c.ID)
		}
		if c.CodiceLetterale == "" {
			return Perimetro{}, ingresso("il componente %s non ha un codice", c.ID)
		}
		if !tipiComponente[c.Tipo] {
			return Perimetro{}, ingresso("il componente %s ha il tipo sconosciuto %q", c.ID, c.Tipo)
		}
		perID[c.ID] = c
	}
	if _, ok := perID[prodotto]; !ok {
		return Perimetro{}, ingresso("il prodotto %s non e' fra i componenti", prodotto)
	}
	figli := map[uuid.UUID][]Arco{}
	visti := map[[2]uuid.UUID]bool{}
	for _, a := range archi {
		if _, ok := perID[a.Padre]; !ok {
			return Perimetro{}, ingresso("l'arco %s → %s parte da un componente che non c'e'", a.Padre, a.Figlio)
		}
		if _, ok := perID[a.Figlio]; !ok {
			return Perimetro{}, ingresso("l'arco %s → %s arriva a un componente che non c'e'", a.Padre, a.Figlio)
		}
		if a.Padre == a.Figlio {
			return Perimetro{}, ingresso("l'arco %s → %s va da un componente a se stesso", a.Padre, a.Figlio)
		}
		if a.Qta < 1 {
			return Perimetro{}, ingresso("l'arco %s → %s ha qta %d", a.Padre, a.Figlio, a.Qta)
		}
		k := [2]uuid.UUID{a.Padre, a.Figlio}
		if visti[k] {
			return Perimetro{}, ingresso("l'arco %s → %s c'e' due volte", a.Padre, a.Figlio)
		}
		visti[k] = true
		figli[a.Padre] = append(figli[a.Padre], a)
	}
	p := Perimetro{Prodotto: prodotto}
	dentro := map[uuid.UUID]bool{prodotto: true}
	coda := []uuid.UUID{prodotto}
	for len(coda) > 0 {
		n := coda[0]
		coda = coda[1:]
		for _, a := range figli[n] {
			if perID[a.Figlio].Archiviato {
				continue
			}
			p.Archi = append(p.Archi, a)
			if !dentro[a.Figlio] {
				dentro[a.Figlio] = true
				coda = append(coda, a.Figlio)
			}
		}
	}
	for id := range dentro {
		p.Componenti = append(p.Componenti, perID[id])
	}
	sort.Slice(p.Componenti, func(i, j int) bool { return p.Componenti[i].ID.String() < p.Componenti[j].ID.String() })
	sort.Slice(p.Archi, func(i, j int) bool {
		if p.Archi[i].Padre != p.Archi[j].Padre {
			return p.Archi[i].Padre.String() < p.Archi[j].Padre.String()
		}
		return p.Archi[i].Figlio.String() < p.Archi[j].Figlio.String()
	})
	return p, nil
}

// La forma canonica del perimetro (firma-perimetro-2): e' questa struttura, scritta da jsoncanonico (chiavi in ordine
// di byte, nessuno spazio, UTF-8 grezzo, interi). codice e' il codice letterale, com'e' nella working. Esempio con un
// prodotto e un figlio (gli UUID abbreviati):
//
//	{"archi":[{"figlio":"…02","padre":"…01","qta":2}],"canonicalizzazione":"canonico-1",
//	 "componenti":[{"archiviato":false,"codice":"7120001","id":"…01","step_strutturale_id":"…aa",
//	 "tipo":"finito"},{"archiviato":false,"codice":"7120010_prt","id":"…02","step_strutturale_id":null,
//	 "tipo":"sottoassieme"}],"prodotto":"…01","versione":"firma-perimetro-2"}
//
// Componenti in ordine di id (la forma testuale minuscola dell'UUID), archi in ordine di padre e poi di figlio.
// Rev, descrizione, posizione e codice canonico non ci sono. «canonicalizzazione» e' la versione della scrittura
// JSON canonica (jsoncanonico.Versione), non delle regole dei codici del cliente.
type formaPerimetro struct {
	Versione           string            `json:"versione"`
	Canonicalizzazione string            `json:"canonicalizzazione"`
	Prodotto           uuid.UUID         `json:"prodotto"`
	Componenti         []formaComponente `json:"componenti"`
	Archi              []formaArco       `json:"archi"`
}

type formaComponente struct {
	ID              uuid.UUID  `json:"id"`
	Codice          string     `json:"codice"`
	Tipo            string     `json:"tipo"`
	Archiviato      bool       `json:"archiviato"`
	StepStrutturale *uuid.UUID `json:"step_strutturale_id"`
}

type formaArco struct {
	Padre  uuid.UUID `json:"padre"`
	Figlio uuid.UUID `json:"figlio"`
	Qta    int32     `json:"qta"`
}

// FormaCanonica e' la forma canonica del perimetro: i byte da cui si calcola la firma. Pura e deterministica: non
// dipende dall'ordine in cui il perimetro elenca componenti e archi, anche per un Perimetro scritto a mano, ne' dalle
// regole del cliente (D-FIRMA-01 = b [U]). Un componente senza codice e' ErrIngresso.
func FormaCanonica(p Perimetro) ([]byte, error) {
	f := formaPerimetro{Versione: VersioneFirma, Canonicalizzazione: jsoncanonico.Versione, Prodotto: p.Prodotto,
		Componenti: make([]formaComponente, 0, len(p.Componenti)), Archi: make([]formaArco, 0, len(p.Archi))}
	for _, c := range p.Componenti {
		if c.CodiceLetterale == "" {
			return nil, ingresso("il componente %s non ha un codice", c.ID)
		}
		x := formaComponente{ID: c.ID, Codice: c.CodiceLetterale, Tipo: c.Tipo, Archiviato: c.Archiviato}
		if c.StepStrutturale.Valid {
			s := c.StepStrutturale.UUID
			x.StepStrutturale = &s
		}
		f.Componenti = append(f.Componenti, x)
	}
	for _, a := range p.Archi {
		f.Archi = append(f.Archi, formaArco{Padre: a.Padre, Figlio: a.Figlio, Qta: a.Qta})
	}
	sort.Slice(f.Componenti, func(i, j int) bool { return f.Componenti[i].ID.String() < f.Componenti[j].ID.String() })
	sort.Slice(f.Archi, func(i, j int) bool {
		if f.Archi[i].Padre != f.Archi[j].Padre {
			return f.Archi[i].Padre.String() < f.Archi[j].Padre.String()
		}
		return f.Archi[i].Figlio.String() < f.Archi[j].Figlio.String()
	})
	return jsoncanonico.Codifica(f)
}

// Firma e' la firma del perimetro: lo sha256 della forma canonica, 64 cifre esadecimali minuscole (le colonne
// firma_struttura e firma_base della 0022, char(64) con il CHECK '^[0-9a-f]{64}$').
func (p Perimetro) Firma() (string, error) {
	b, err := FormaCanonica(p)
	if err != nil {
		return "", fmt.Errorf("confermabom: forma canonica del perimetro di %s: %w", p.Prodotto, err)
	}
	return jsoncanonico.Impronta(b), nil
}

// FirmaPerimetro e' la firma del perimetro del prodotto sui componenti e sugli archi della RFQ (contratto rev. 3
// §5.1, emendato da D-FIRMA-01 = b [U]: firma_struttura / firma_base). Entrano, per ogni componente del perimetro:
// id, codice letterale, tipo, archiviato, step_strutturale_id; per ogni arco del perimetro: padre, figlio, qta. Non
// entrano rev, descrizione, posizione e il codice canonico. L'ordine degli ingressi non conta. Pura.
//
// Le regole del cliente non sono un ingresso: un cambio delle regole di canonizzazione lascia la firma com'e', e la
// conferma che la porta resta valida anche dopo che il contatore si e' mosso per un altro prodotto della RFQ (lo
// Scenario F della direttiva 12). Un cambio vero di un componente o di un arco del perimetro cambia la firma, e la
// validita' la decide ConfermaValida (ultima e, contatore uguale oppure firma uguale).
func FirmaPerimetro(prodotto uuid.UUID, componenti []Componente, archi []Arco) (string, error) {
	p, err := CalcolaPerimetro(prodotto, componenti, archi)
	if err != nil {
		return "", err
	}
	return p.Firma()
}
