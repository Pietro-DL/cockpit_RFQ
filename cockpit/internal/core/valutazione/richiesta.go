package valutazione

import (
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// I valori del DB che la richiesta guarda (0001: enum aggancio e direzione; 0014 e 0016: tipo_controparte).
const (
	aggancioOperatore    = "operatore"
	direzioneEntrata     = "entrata"
	direzioneUscita      = "uscita"
	controparteCliente   = "cliente"
	controparteFornitore = "fornitore"
)

// segmentoCorrente: il segmento della parte corrente di una mail, come lo scrive l'adattatore (estrazione,
// DaMessaggio). Il riconoscimento tocca solo questo, mai la storia (R48 A).
const segmentoCorrente = "s:corrente"

// I motivi del gesto 1 di un messaggio (GestoMessaggio.Motivo) e della scelta di riconoscimento.
const (
	motivoGestoOperatore     = "gesto 1 dell'operatore, messaggio in entrata dal cliente del thread"
	motivoUscita             = "messaggio in uscita"
	motivoFornitore          = "controparte fornitore"
	motivoAltroCliente       = "controparte un altro cliente"
	motivoSenzaGesto         = "nessun gesto dell'operatore (aggancio «%s»)"
	motivoControparte        = "controparte «%s», non il cliente del thread"
	motivoDirezioneIgnota    = "direzione non nota"
	motivoRiconoscimento     = "riconoscimento: la parte corrente di un messaggio con il gesto 1 è pertinente, un candidato da confermare (E2)"
	motivoTriageComeEvidenza = "; triage «%s» solo come evidenza (R29 c)"
)

// RichiestaDelThread compone la richiesta valutata di un thread (6.4.4, 6.4.6; T-B0-06: una per thread, i gesti per
// messaggio) e l'uso dei segmenti di ogni messaggio, indicizzato per BundleID del suo documento.
//
// Lo stato di un messaggio (6.0.5; contratto §1.1, nota; R29 c):
//   - valutata: il gesto 1 dell'operatore (aggancio = «operatore», con agganciato_da e agganciato_il), il messaggio
//     in entrata, la controparte il cliente del thread;
//   - non_valutabile: un messaggio in uscita, la controparte un fornitore o un altro cliente;
//   - da_valutare: tutto il resto, qualunque cosa dica il triage. Il triage deterministico sta in Evento, solo come
//     evidenza: «ignora» non rende non valutabile, «nuova_rfq» non rende valutata.
//
// Lo stato del thread: valutata se un messaggio della richiesta lo è; non_valutabile se lo sono tutti; altrimenti
// da_valutare. Evento del thread: quello del primo messaggio valutato, o del primo con un triage. Decisioni:
// l'impronta dei gesti dei messaggi valutati, (messaggio_id, agganciato_da, agganciato_il) in ordine di messaggio
// (T-16); "" senza gesti.
//
// L'uso dei segmenti:
//   - con un caso per il thread, i segmenti dichiarati, con la loro origine «scenario», nei messaggi del caso (R29
//     b C; R75 A); gli altri messaggi hanno l'uso sconosciuto;
//   - senza caso, il riconoscimento tracciato: la parte corrente di un messaggio valutato è pertinente, con origine
//     «riconoscimento» e il triage nel motivo. È un candidato, non una conferma (E2: il router lo legge come da
//     valutare); la storia non si tocca mai, nemmeno quando non è separabile (R48 A);
//   - per gli altri messaggi, e per i messaggi senza documento, l'uso sconosciuto.
//
// I messaggi della richiesta sono quelli del caso, se il caso li elenca, altrimenti tutti i messaggi del thread, in
// ordine di (data, ID). docs è indicizzato per messaggio; un messaggio senza documento resta nella richiesta, con il
// suo gesto, ma senza uso.
func RichiestaDelThread(t fotorfq.Thread, caso *IngressoCaso, docs map[uuid.UUID]evidenze.DocumentoEvidenze) (ancoraggio.RichiestaValutata, map[string]evidenze.UsoSegmenti) {
	messaggi := messaggiInOrdine(t)
	if caso != nil && len(caso.Messaggi) > 0 {
		delCaso := map[uuid.UUID]bool{}
		for _, id := range caso.Messaggi {
			delCaso[id] = true
		}
		filtrati := messaggi[:0:0]
		for _, m := range messaggi {
			if delCaso[m.ID] {
				filtrati = append(filtrati, m)
			}
		}
		messaggi = filtrati
	}
	agganci := map[uuid.UUID]fotorfq.AggancioMessaggio{}
	for _, a := range t.Agganci {
		agganci[a.MessaggioID] = a
	}
	eventi := eventiDelTriage(t)

	r := ancoraggio.RichiestaValutata{ClienteID: t.ClienteID}
	usi := map[string]evidenze.UsoSegmenti{}
	var gestiUsati []gestoUsato
	valutati, nonValutabili := 0, 0
	for _, m := range messaggi {
		g := gestoDelMessaggio(t, m, agganci[m.ID], eventi[m.ID])
		r.Messaggi = append(r.Messaggi, m.ID)
		r.Gesti = append(r.Gesti, g)
		switch g.Stato {
		case ancoraggio.StatoRichiestaValutata:
			valutati++
			u := gestoUsato{MessaggioID: m.ID, AgganciatoDa: g.AgganciatoDa}
			if g.AgganciatoIl != nil {
				il := g.AgganciatoIl.Format(time.RFC3339Nano)
				u.AgganciatoIl = &il
			}
			gestiUsati = append(gestiUsati, u)
			if r.Evento == "" {
				r.Evento = g.Evento
			}
		case ancoraggio.StatoRichiestaNonValutabile:
			nonValutabili++
		}

		d, ok := docs[m.ID]
		if !ok {
			continue
		}
		uso := evidenze.UsoSconosciuto(d.BundleID)
		switch {
		case caso != nil:
			for _, s := range caso.Segmenti {
				if s.MessaggioID != m.ID {
					continue
				}
				uso.Stato = "valutato"
				uso.Selezioni = append(uso.Selezioni, evidenze.SelezioneSegmento{SegmentoID: s.SegmentoID, Uso: s.Uso, Origine: s.Origine,
					Motivo: s.Motivo, Rif: "caso:" + caso.ID})
				if s.Uso == "pertinente" {
					r.Segmenti = append(r.Segmenti, ancoraggio.SegmentoPertinente{MessaggioID: m.ID, SegmentoID: s.SegmentoID, Origine: s.Origine, Motivo: s.Motivo})
				}
			}
		case g.Stato == ancoraggio.StatoRichiestaValutata && haSegmento(d, segmentoCorrente):
			motivo := motivoRiconoscimento
			if g.Evento != "" {
				motivo += fmt.Sprintf(motivoTriageComeEvidenza, g.Evento)
			}
			uso.Stato = "valutato"
			uso.Selezioni = []evidenze.SelezioneSegmento{{SegmentoID: segmentoCorrente, Uso: "pertinente", Origine: "riconoscimento", Motivo: motivo}}
			r.Segmenti = append(r.Segmenti, ancoraggio.SegmentoPertinente{MessaggioID: m.ID, SegmentoID: segmentoCorrente, Origine: "riconoscimento", Motivo: motivo})
		}
		sort.SliceStable(uso.Selezioni, func(i, j int) bool { return uso.Selezioni[i].SegmentoID < uso.Selezioni[j].SegmentoID })
		usi[d.BundleID] = uso
	}
	if r.Evento == "" {
		for _, g := range r.Gesti {
			if g.Evento != "" {
				r.Evento = g.Evento
				break
			}
		}
	}
	switch {
	case valutati > 0:
		r.Stato = ancoraggio.StatoRichiestaValutata
	case len(messaggi) > 0 && nonValutabili == len(messaggi):
		r.Stato = ancoraggio.StatoRichiestaNonValutabile
	default:
		r.Stato = ancoraggio.StatoRichiestaDaValutare
	}
	if len(gestiUsati) > 0 {
		// T-16: l'impronta del canonico dei gesti usati. gestoUsato ha solo tipi chiusi, UUID e testi ASCII (il tempo è
		// già scritto, in UTC al millisecondo): jsoncanonico rifiuta solo i decimali, l'UTF-8 non valido e i tipi fuori
		// elenco, quindi qui non può fallire, e l'errore non c'è da riportare.
		r.Decisioni, _ = jsoncanonico.ImprontaDi(gestiUsati)
	}
	return r, usi
}

// gestoUsato: ciò che entra nell'impronta delle decisioni (T-16). Il tempo è scritto in RFC 3339 con i nanosecondi
// (in UTC al millisecondo), lo stesso testo che darebbe time.Time con MarshalJSON, ma senza il suo errore per gli
// anni fuori da 0-9999.
type gestoUsato struct {
	MessaggioID  uuid.UUID  `json:"messaggio_id"`
	AgganciatoDa *uuid.UUID `json:"agganciato_da"`
	AgganciatoIl *string    `json:"agganciato_il"`
}

// gestoDelMessaggio: il gesto 1 del messaggio con il suo stato e il motivo. Il triage sta in Evento, e non conta.
func gestoDelMessaggio(t fotorfq.Thread, m fotorfq.Messaggio, a fotorfq.AggancioMessaggio, evento string) ancoraggio.GestoMessaggio {
	g := ancoraggio.GestoMessaggio{MessaggioID: m.ID, Aggancio: a.Aggancio, AgganciatoDa: a.AgganciatoDa, Evento: evento}
	if a.AgganciatoIl != nil {
		il := a.AgganciatoIl.UTC().Truncate(time.Millisecond)
		g.AgganciatoIl = &il
	}
	altroCliente := m.ControparteTipo == controparteCliente && m.ControparteClienteID != nil && *m.ControparteClienteID != t.ClienteID
	clienteDelThread := m.ControparteTipo == controparteCliente && m.ControparteClienteID != nil && *m.ControparteClienteID == t.ClienteID
	switch {
	case m.Direzione == direzioneUscita:
		g.Stato, g.Motivo = ancoraggio.StatoRichiestaNonValutabile, motivoUscita
	case m.ControparteTipo == controparteFornitore:
		g.Stato, g.Motivo = ancoraggio.StatoRichiestaNonValutabile, motivoFornitore
	case altroCliente:
		g.Stato, g.Motivo = ancoraggio.StatoRichiestaNonValutabile, motivoAltroCliente
	case a.Aggancio != aggancioOperatore || a.AgganciatoDa == nil || a.AgganciatoIl == nil:
		aggancio := a.Aggancio
		if aggancio == "" {
			aggancio = "nessuno"
		}
		g.Stato, g.Motivo = ancoraggio.StatoRichiestaDaValutare, fmt.Sprintf(motivoSenzaGesto, aggancio)
	case m.Direzione != direzioneEntrata:
		g.Stato, g.Motivo = ancoraggio.StatoRichiestaDaValutare, motivoDirezioneIgnota
	case !clienteDelThread:
		tipo := m.ControparteTipo
		if tipo == "" {
			tipo = "non nota"
		}
		g.Stato, g.Motivo = ancoraggio.StatoRichiestaDaValutare, fmt.Sprintf(motivoControparte, tipo)
	default:
		g.Stato, g.Motivo = ancoraggio.StatoRichiestaValutata, motivoGestoOperatore
	}
	return g
}

// eventiDelTriage: per messaggio, «<esito>/<atto>» del triage deterministico più recente (per data, poi ID); solo
// l'esito se l'atto manca. La fotografia porta solo il triage deterministico (6.4.1).
func eventiDelTriage(t fotorfq.Thread) map[uuid.UUID]string {
	triage := append([]fotorfq.Triage(nil), t.Triage...)
	sort.SliceStable(triage, func(i, j int) bool {
		a, b := triage[i], triage[j]
		if !a.CreatoIl.Equal(b.CreatoIl) {
			return a.CreatoIl.Before(b.CreatoIl)
		}
		return a.ID.String() < b.ID.String()
	})
	out := map[uuid.UUID]string{}
	for _, x := range triage {
		e := x.Esito
		if x.Atto != "" {
			e += "/" + x.Atto
		}
		out[x.MessaggioID] = e // l'ultimo, il più recente, vince
	}
	return out
}

// messaggiInOrdine: i messaggi del thread per (data, ID), senza toccare la fotografia di chi chiama.
func messaggiInOrdine(t fotorfq.Thread) []fotorfq.Messaggio {
	m := append([]fotorfq.Messaggio(nil), t.Messaggi...)
	sort.SliceStable(m, func(i, j int) bool {
		if !m[i].DataEvento.Equal(m[j].DataEvento) {
			return m[i].DataEvento.Before(m[j].DataEvento)
		}
		return m[i].ID.String() < m[j].ID.String()
	})
	return m
}

// haSegmento: il documento ha il segmento con quell'ID.
func haSegmento(d evidenze.DocumentoEvidenze, id string) bool {
	for _, s := range d.Segmenti {
		if s.ID == id {
			return true
		}
	}
	return false
}
