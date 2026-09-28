package fascicolo

// La classificazione dei nodi di uno STEP (Blocco 8, B8.5; addendum A1.2, D16).
//
// Il worker manda i grezzi di ogni PRODUCT e non decide niente: che cosa sia un codice dipende dalle
// famiglie del cliente della RFQ, e quelle le conosce solo il server. Qui si passano id, nome e
// descrizione del PRODUCT allo stesso Motore che legge oggetto, corpo e nomi dei file, con le regole
// del cliente di QUESTA RFQ: gli stessi fatti danno codici diversi in RFQ di clienti diversi, e cambiare
// le regole cambia la lettura delle proposte ancora aperte. I fatti non cambiano: la classificazione
// non e' un fatto.

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/contratti/worker"
	"promatec/cockpit/internal/platform/db"
)

// Dove sta, nel PRODUCT, il testo da cui viene un codice. L'ordine e' la precedenza: se id e nome
// danno due codici diversi vince l'id, e l'altro resta nell'evidenza (A1.7, «SolidWorks mette il
// file in name e il codice in id, altri il contrario»).
const (
	DoveID          = "id"
	DoveNome        = "nome"
	DoveDescrizione = "descrizione"
)

// Misure delle colonne di componente_proposta: i grezzi arrivano da un worker, cioe' da fuori, e
// prima di una colonna passano da qui.
const (
	maxGrezzo   = 200
	maxChiave   = 120
	maxFamiglia = 120
)

// NodoClassificato e' un nodo del file con il codice che il server gli riconosce.
type NodoClassificato struct {
	Chiave      string
	IDGrezzo    string
	NomeGrezzo  string
	Descrizione string
	Codice      string // "" = il server non ci vede un codice: lo scrivera' l'operatore
	Rev         string
	Origine     string // "famiglia" | "generico" | "" (e "operatore" se l'ha scritto una persona)
	Famiglia    string // la famiglia del cliente che l'ha riconosciuto
	Confidenza  int
	Dove        string // DoveID, DoveNome, DoveDescrizione
	Alternativo string // un codice DIVERSO letto nell'altro fra id e nome
	Evidenza    map[string]any
}

// ClassificaNodi da' a ogni nodo il codice che il Motore del cliente gli riconosce. Pura.
//
// Per ogni nodo: il primo codice di famiglia, cercato nell'id, poi nel nome, poi nella descrizione;
// altrimenti il primo generico, nello stesso ordine; altrimenti nessuno. La revisione e' quella del
// file (rev_grezza) se c'e', altrimenti quella che la famiglia separa dal codice. Un codice o una
// revisione fuori misura non entrano nel campo: restano nell'evidenza, con il loro nome.
func ClassificaNodi(m *classificazione.Motore, s worker.StrutturaSTEP) []NodoClassificato {
	out := make([]NodoClassificato, 0, len(s.Nodi))
	for _, n := range s.Nodi {
		c := NodoClassificato{
			Chiave: taglia(n.Chiave, maxChiave), IDGrezzo: taglia(n.IDGrezzo, maxGrezzo),
			NomeGrezzo: taglia(n.NomeGrezzo, maxGrezzo), Descrizione: taglia(n.DescrizioneGrezza, maxGrezzo),
			Evidenza: map[string]any{},
		}
		for k, v := range n.Evidenza {
			c.Evidenza[k] = v
		}
		c.Evidenza["testo"] = c.NomeGrezzo
		e := m.Estrai(
			classificazione.Testo{Dove: DoveID, Corpo: n.IDGrezzo},
			classificazione.Testo{Dove: DoveNome, Corpo: n.NomeGrezzo},
			classificazione.Testo{Dove: DoveDescrizione, Corpo: n.DescrizioneGrezza},
		)
		scelto, ok := primo(e.Codici)
		if ok {
			c.Codice, c.Rev, c.Origine, c.Famiglia, c.Confidenza, c.Dove =
				scelto.Codice, scelto.Rev, scelto.Origine, taglia(scelto.Famiglia, maxFamiglia), scelto.Punteggio, scelto.Dove
			c.Evidenza["dove"] = scelto.Dove
			if scelto.Famiglia != "" {
				c.Evidenza["regola"] = scelto.Famiglia
			}
			if alt := alternativo(e.Codici, scelto); alt != "" {
				c.Alternativo = alt
				c.Evidenza["alternativo"] = alt
			}
		}
		if r := strings.TrimSpace(n.RevGrezza); r != "" {
			c.Rev = r
		}
		if c.Codice != "" && !classificazione.CodiceAmmissibile(c.Codice) {
			c.Evidenza["codice_scartato"] = c.Codice
			c.Codice, c.Origine, c.Famiglia, c.Confidenza, c.Dove = "", "", "", 0, ""
		}
		if c.Rev != "" && !classificazione.RevAmmissibile(c.Rev) {
			c.Evidenza["rev_scartata"] = c.Rev
			c.Rev = ""
		}
		out = append(out, c)
	}
	return out
}

// primo sceglie il codice del nodo: famiglia prima di generico, e dentro ciascuno id, nome, descrizione.
func primo(codici []classificazione.CodiceTrovato) (classificazione.CodiceTrovato, bool) {
	for _, origine := range []string{"famiglia", "generico"} {
		for _, dove := range []string{DoveID, DoveNome, DoveDescrizione} {
			for _, c := range codici {
				if c.Origine == origine && c.Dove == dove && c.Codice != "" {
					return c, true
				}
			}
		}
	}
	return classificazione.CodiceTrovato{}, false
}

// alternativo e' un codice diverso da quello scelto, letto nell'altro dei due attributi che nominano il
// pezzo (id e nome). La descrizione non conta: e' una frase, e ci si trovano misure e norme.
func alternativo(codici []classificazione.CodiceTrovato, scelto classificazione.CodiceTrovato) string {
	altro := DoveNome
	if scelto.Dove == DoveNome {
		altro = DoveID
	} else if scelto.Dove != DoveID {
		return ""
	}
	for _, c := range codici {
		if c.Dove == altro && c.Codice != "" && !strings.EqualFold(c.Codice, scelto.Codice) &&
			(c.Origine == scelto.Origine || c.Origine == "famiglia") {
			return c.Codice
		}
	}
	return ""
}

// taglia porta un testo alla misura della colonna, contando i caratteri e non i byte.
func taglia(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max])
}

// MotoreDellaRfq compila le regole del cliente della RFQ. Un cliente senza regole da' un motore vuoto
// ma funzionante: l'estrattore generico lavora lo stesso, ed e' il caso normale finche' l'anagrafica
// non e' compilata. Le regole marcate come non valide restano fuori (classificazione.Compila).
func MotoreDellaRfq(ctx context.Context, q *db.Queries, thread uuid.UUID) (*classificazione.Motore, error) {
	t, err := q.GetThread(ctx, thread)
	if err != nil {
		return nil, err
	}
	c, err := q.GetCliente(ctx, t.ClienteID)
	if err != nil {
		return nil, err
	}
	lette, _ := regole.LeggiRegole(c.Regole)
	return classificazione.Compila(c.RagioneSociale, lette), nil
}
