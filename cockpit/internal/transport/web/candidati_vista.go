package web

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/inbox/aggancio"
	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// I CANDIDATI DI AGGANCIO SULLA SCHERMATA (Smistamento M1, A5.16.4)
//
// Una card per RFQ, non una riga per (RFQ, regola): la stessa RFQ indicata da In-Reply-To e dall'oggetto
// è una card con due evidenze. Le card sono in ordine a livelli («molto forte · score 98» prima di
// «debole · score 40»), e ognuna ha il suo bottone «Aggancia a questa RFQ». NESSUN radio e nessun
// default: la prima card ha solo un bordo più marcato, che non vuol dire selezionata, e a pari merito
// nessuna card è marcata (P37). Gli score non sono percentuali e non si scrivono con «%» (U7).

// Modi della lista: nel pannello ogni card ha il suo form; dentro «Aggancia a…» le card sono bottoni del
// form (porta con sé buyer e allegati); nell'avviso del form «Nuova RFQ» le card si leggono e basta.
const (
	modoPannello = "pannello"
	modoForm     = "form"
	modoAvviso   = "avviso"
)

type candidatiVista struct {
	MessaggioID uuid.UUID
	Modo        string
	Carte       []cartaCandidato
	PariMerito  bool
	DiPrima     bool
	// Proposta: nel pannello, la RFQ che ha gia' il suo bottone nella carta «Che cosa fare». La sua card non lo
	// ripete: un bottone solo per RFQ.
	Proposta string
}

type cartaCandidato struct {
	classificazione.CandidatoRFQ
	aggancio.RFQ
	// Primo: la prima card, se non c'è pari merito. Solo un bordo più marcato: non è una scelta.
	Primo bool
}

// Forza è la forza della RFQ con le parole della schermata: «molto forte · score 98».
func (c cartaCandidato) Forza() string { return fmt.Sprintf("%s · score %d", c.Livello, c.Score) }

func vistaCandidati(messaggioID uuid.UUID, l aggancio.Lettura, modo string) candidatiVista {
	v := candidatiVista{MessaggioID: messaggioID, Modo: modo, PariMerito: l.PariMerito, DiPrima: l.DiPrima}
	for i, g := range l.Candidati {
		v.Carte = append(v.Carte, cartaCandidato{CandidatoRFQ: g, RFQ: l.RFQ[g.ThreadID], Primo: i == 0 && !l.PariMerito && !g.Chiuso})
	}
	return v
}

// N è il numero delle RFQ candidate, per le frasi che le contano.
func (v candidatiVista) N() int { return len(v.Carte) }

// fraseOrigine è il box del pannello sulla nostra mail preparata dal Cockpit (D84) o sulla richiesta a
// un fornitore legata dal marcatore (D85): da dove nasce, letto adesso.
func fraseOrigine(o aggancio.Origine) string {
	quando := o.Quando.Local().Format("02/01")
	rfq := func(i int) string {
		if i < len(o.Oggetti) && strings.TrimSpace(o.Oggetti[i]) != "" {
			return "«" + o.Oggetti[i] + "»"
		}
		return "della richiesta"
	}
	if o.Via == "richiesta" {
		return fmt.Sprintf("Richiesta a %s creata dal Cockpit da %s il %s per la RFQ %s. La mail non si aggancia da sola: confermala qui sotto.",
			o.Fornitore, o.Sigla, quando, rfq(0))
	}
	risposta := ""
	if o.OggettoRisposta != "" {
		risposta = " (risposta a «" + o.OggettoRisposta + "»"
		if o.DataRisposta != nil {
			risposta += " del " + o.DataRisposta.Local().Format("02/01")
		}
		risposta += ")"
	}
	switch len(o.RFQ) {
	case 0:
		return fmt.Sprintf("Preparata dal Cockpit da %s il %s%s, ma la mail a cui rispondeva non è in nessuna RFQ.", o.Sigla, quando, risposta)
	case 1:
		if o.DallaRisposta {
			return fmt.Sprintf("Preparata dal Cockpit da %s il %s in risposta a una mail che oggi sta nella RFQ %s%s. La mail non si aggancia da sola: confermala qui sotto.", o.Sigla, quando, rfq(0), risposta)
		}
		return fmt.Sprintf("Preparata dal Cockpit da %s il %s per la RFQ %s%s. La mail non si aggancia da sola: confermala qui sotto.", o.Sigla, quando, rfq(0), risposta)
	}
	return fmt.Sprintf("Preparata dal Cockpit da %s il %s per la RFQ %s, in risposta a una mail che oggi sta nella RFQ %s: le due origini non coincidono, e nessuna è proposta.",
		o.Sigla, quando, rfq(0), rfq(1))
}

// chipOrigine è il chip della lista per la nostra mail preparata dal Cockpit: «dal Cockpit · RFQ …».
func chipOrigine(oo []aggancio.Origine) string {
	for _, o := range oo {
		if o.Via != "bozza" {
			continue
		}
		switch {
		case len(o.RFQ) > 1:
			return "dal Cockpit · due RFQ"
		case len(o.RFQ) == 1 && len(o.Oggetti) == 1 && o.Oggetti[0] != "":
			return "dal Cockpit · " + o.Oggetti[0]
		case len(o.RFQ) == 1:
			return "dal Cockpit · RFQ"
		}
		return "dal Cockpit"
	}
	return ""
}

// origineDellaPagina legge le origini Cockpit delle righe della lista con due query per pagina, non una
// per riga. Non scrive niente.
func origineDellaPagina(ctx context.Context, q *db.Queries, righe []db.VInbox) map[uuid.UUID]string {
	var ids []uuid.UUID
	for _, r := range righe {
		if r.Direzione == db.DirezioneUscita {
			ids = append(ids, r.MessaggioID)
		}
	}
	out := map[uuid.UUID]string{}
	origini, err := aggancio.OriginiCockpit(ctx, q, ids)
	if err != nil {
		return out
	}
	for id, oo := range origini {
		if c := chipOrigine(oo); c != "" {
			out[id] = c
		}
	}
	return out
}

// chipTriage è la proposta del triage con le parole della schermata, senza percentuali (U7): per
// «aggancia» la forza della prima RFQ («RFQ: forte»), per «nuova_rfq» il punteggio del contenuto
// («nuova RFQ? score 60»). A pari merito nessuna RFQ è proposta e il chip lo dice. La posta di un
// fornitore che «aggancia» a una nostra richiesta (non a pari merito fra due RFQ) dice «risposta a una
// nostra richiesta»: i punteggi delle richieste sono un'altra scala, e il chip non la traduce in un
// livello.
func chipTriage(r db.VInbox) string {
	return testoTriage(r.TriageEsito, r.TriageConfidenza, r.ThreadProposto, r.ControparteTipo, pariMeritoNeiMotivi(r.TriageMotivi))
}

// pariMeritoNeiMotivi dice se il triage ha trovato due RFQ della stessa forza (P37). v_inbox non porta
// `richiesta_proposta`, e «aggancia» senza `thread_proposto` ha due origini che la riga, da sola, non
// separa: la risposta a una nostra richiesta (ramo fornitore) e il pari merito. Il secondo il triage lo
// scrive fra i motivi con una frase fissa, ed è quella che si cerca qui.
func pariMeritoNeiMotivi(m *json.RawMessage) bool {
	if m == nil {
		return false
	}
	var motivi []string
	if json.Unmarshal(*m, &motivi) != nil {
		return false
	}
	for _, x := range motivi {
		if x == classificazione.FrasePariMerito {
			return true
		}
	}
	return false
}

// rispostaARichiesta: «aggancia» senza una RFQ proposta, dalla posta di un fornitore, e non a pari
// merito. È la risposta a una nostra richiesta (richiesta_proposta), che il triage ha proposto.
func rispostaARichiesta(proposto uuid.NullUUID, controparte string, pari bool) bool {
	return !proposto.Valid && !pari && controparte == string(db.TipoControparteFornitore)
}

func testoTriage(esito pgtype.Text, conf pgtype.Int2, proposto uuid.NullUUID, controparte string, pari bool) string {
	if !esito.Valid {
		return ""
	}
	switch esito.String {
	case "aggancia":
		if rispostaARichiesta(proposto, controparte, pari) {
			return "risposta a una nostra richiesta"
		}
		s := "RFQ: " + classificazione.LivelloDaScore(int(conf.Int16)).String()
		if !proposto.Valid {
			s += " · da scegliere"
		}
		return s
	case "nuova_rfq":
		return fmt.Sprintf("nuova RFQ? score %d", conf.Int16)
	}
	return esito.String
}

// coloreTriage è il colore della riga dell'Inbox. «aggancia» è verde solo quando il triage propone
// qualcosa (una RFQ, o la richiesta a cui risponde il fornitore): con «da scegliere» (pari merito, o
// nessuna RFQ proposta) la riga è gialla come il chip, e non dice «deciso» dove il chip dice il
// contrario. Per «nuova_rfq» resta la soglia 75 di prima, che è della fase F3/M2.
func coloreTriage(r db.VInbox) string {
	return coloreDelTriage(r.TriageEsito, r.TriageConfidenza, r.ThreadProposto, r.ControparteTipo, pariMeritoNeiMotivi(r.TriageMotivi))
}

func coloreDelTriage(esito pgtype.Text, conf pgtype.Int2, proposto uuid.NullUUID, controparte string, pari bool) string {
	if !esito.Valid {
		return ""
	}
	switch esito.String {
	case "nuova_rfq":
		if conf.Int16 >= 75 {
			return "verde"
		}
		return "giallo"
	case "aggancia":
		if proposto.Valid || rispostaARichiesta(proposto, controparte, pari) {
			return "verde"
		}
		return "giallo"
	}
	return "grigio"
}
