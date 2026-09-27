package web

import (
	"context"
	"encoding/json"
	"strings"

	"promatec/cockpit/internal/core/inbox/aggancio"
	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// L'EVENTO SULLA SCHERMATA (Smistamento M2, A5.16.4)
//
// Il chip dell'atto grezzo («incerto», «documenti_aggiuntivi») diventa il chip dell'evento («Revisione
// CAD»), nella lista e nel pannello. L'evento si rilegge dall'atto salvato (EventoDa) e le sue evidenze
// dai motivi con il prefisso «evento ·»; per un messaggio che il triage non interpreta (una nostra mail a
// un cliente) si calcola adesso, dal messaggio, senza scrivere niente. L'evento non aggancia: nessun
// bottone parte da qui, e la RFQ si sceglie fra i candidati.

type vistaEvento struct {
	Codice    string // uno degli Evento*
	Etichetta string // «Revisione CAD»
	Forza     string // chiaro | probabile | incerto; vuota sulle righe scritte prima del M2
	Evidenze  []string
	Atto      string
	Legame    string
	// InLettura: calcolato adesso dal messaggio, perché nessun triage l'ha salvato.
	InLettura bool
}

// Chip è il testo del chip. ALTRO dice anche l'atto quando l'atto dice qualcosa («Altro · non
// business», «Altro · domanda chiarimento» di un fornitore): è l'informazione che il chip grezzo dava.
func (v vistaEvento) Chip() string {
	if v.Codice == classificazione.EventoAltro && v.Atto != "" && v.Atto != classificazione.AttoIncerto {
		return v.Etichetta + " · " + strings.ReplaceAll(v.Atto, "_", " ")
	}
	return v.Etichetta
}

// Titolo è il suggerimento del chip: le evidenze, l'atto e il legame, e che l'evento non aggancia.
func (v vistaEvento) Titolo() string {
	var p []string
	switch {
	case len(v.Evidenze) > 0:
		p = append(p, "evento: "+strings.Join(v.Evidenze, "; "))
	case v.InLettura:
		p = append(p, "evento letto adesso dal messaggio")
	default:
		p = append(p, "evento letto dall'atto salvato, senza evidenze (triage di prima)")
	}
	if v.Atto != "" {
		p = append(p, "atto: "+v.Atto)
	}
	if v.Legame != "" {
		p = append(p, "legame: "+v.Legame)
	}
	p = append(p, "l'evento non aggancia")
	return strings.Join(p, " · ")
}

// eventoDellaRiga è l'evento di una riga di v_inbox, dall'atto salvato. Nil se il triage non ha scritto
// un atto. Le evidenze si prendono dai motivi solo se dicono lo stesso evento dell'atto: una riga
// ritoccata a mano non deve far leggere evidenze di un altro evento.
func eventoDellaRiga(r db.VInbox) *vistaEvento {
	if r.TriageAtto == "" {
		return nil
	}
	codice := classificazione.EventoDa(r.ControparteTipo, string(r.Direzione), r.TriageAtto, r.TriageLegame)
	v := &vistaEvento{Codice: codice, Etichetta: classificazione.EtichettaEvento(codice), Atto: r.TriageAtto, Legame: r.TriageLegame}
	if letto, _ := classificazione.LeggiMotiviEvento(leggiMotivi(r.TriageMotivi)); letto.Evento == codice {
		v.Forza, v.Evidenze = letto.Forza, letto.Evidenze
	}
	return v
}

func leggiMotivi(m *json.RawMessage) []string {
	if m == nil {
		return nil
	}
	var out []string
	_ = json.Unmarshal(*m, &out)
	return out
}

// motiviProposta sono i motivi della proposta di aggancio, senza le righe dell'evento: rispondono a
// un'altra domanda, e il pannello le mostra a parte.
func motiviProposta(m *json.RawMessage) []string {
	_, altri := classificazione.LeggiMotiviEvento(leggiMotivi(m))
	return altri
}

// eventoInLettura calcola l'evento di un messaggio che il triage non ha interpretato (le nostre mail a un
// cliente, E1; i messaggi di prima della 0014), con la stessa funzione pura dell'ingest. Legge e basta.
func eventoInLettura(ctx context.Context, q *db.Queries, m db.Messaggio, origini []aggancio.Origine) *vistaEvento {
	var nomi []string
	if allegati, err := q.ListAllegatiMessaggio(ctx, m.MessaggioID); err == nil {
		for _, a := range allegati {
			if !a.ContenitoreID.Valid {
				nomi = append(nomi, a.NomeFile)
			}
		}
	}
	var inReplyTo string
	if mo, err := q.GetMessaggioOutlook(ctx, m.MessaggioID); err == nil {
		inReplyTo = mo.InReplyTo.String
	}
	dalCockpit := false
	for _, o := range origini {
		dalCockpit = dalCockpit || o.Via == "bozza"
	}
	e := classificazione.Evento(classificazione.IngressoEvento{
		Controparte: string(m.ControparteTipo), Direzione: string(m.Direzione), Interno: m.Interno,
		Oggetto: m.Oggetto.String, Corpo: m.CorpoTesto.String, InReplyTo: inReplyTo, NomiAllegati: nomi,
		Mittente: m.MittenteIndirizzo.String, DalCockpit: dalCockpit,
	})
	return &vistaEvento{Codice: e.Evento, Etichetta: classificazione.EtichettaEvento(e.Evento), Forza: e.Forza,
		Evidenze: e.Evidenze, Atto: e.Atto, InLettura: true}
}
