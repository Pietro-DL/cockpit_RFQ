package estrazione

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// Gli stati della fonte e delle capacità (QualitaFonte.Stato, Capacita.Stato), e le localizzazioni e i metodi
// delle unità: i valori chiusi della foglia (parte 1 §3.4), che la foglia non esporta.
const (
	statoDisponibile    = "disponibile"
	statoParziale       = "parziale"
	statoNonDisponibile = "non_disponibile"
	statoErrore         = "errore"

	localizzazioneEsatta   = "esatta"
	localizzazioneParziale = "parziale"

	metodoAdattatore = "adattatore"
	metodoParser     = "parser"

	mappaturaVerificata = "verificata"
)

// gravitaDelloStato: l'ordine degli stati della fonte. Una parte del documento può solo peggiorare lo stato
// che le altre hanno dato, mai migliorarlo: un errore resta un errore anche se il nome del file si legge.
var gravitaDelloStato = map[string]int{statoDisponibile: 0, statoParziale: 1, statoNonDisponibile: 2, statoErrore: 3}

// documento: il costruttore comune degli adattatori (5.4.5, «Regole comuni»). Raccoglie testi, segmenti,
// entità, unità, legami e qualità nell'ordine in cui l'adattatore li trova; chiudi li mette in ordine canonico,
// controlla il documento con la porta della foglia e calcola il BundleID. Non legge niente da fuori: riceve
// solo ciò che l'adattatore gli passa.
type documento struct {
	d evidenze.DocumentoEvidenze
}

// nuovoDocumento apre un documento sulla fonte data, con le versioni di oggi e lo stato «disponibile», che
// le parti dell'adattatore possono solo peggiorare. I tempi della fonte si portano in UTC, al millisecondo,
// prima che entrino nell'impronta (par.3.4.3, regola 4).
func nuovoDocumento(f evidenze.Fonte) *documento {
	f.Provenienza.Data = alMillisecondo(f.Provenienza.Data)
	f.RiferimentoFatti.CalcolatoIl = alMillisecondo(f.RiferimentoFatti.CalcolatoIl)
	return &documento{d: evidenze.DocumentoEvidenze{
		VersioneSchema:     evidenze.VersioneSchemaDocumento,
		VersioneAdattatore: VersioneAdattatore,
		Fonte:              f,
		Qualita:            evidenze.QualitaFonte{Stato: statoDisponibile},
	}}
}

// alMillisecondo: UTC, troncato al millisecondo. Lo zero resta zero (omitzero lo toglie dal JSON).
func alMillisecondo(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	return t.UTC().Truncate(time.Millisecond)
}

func (c *documento) testo(id, testo, origine string) {
	c.d.Testi = append(c.d.Testi, evidenze.TestoOriginale{ID: id, Testo: testo, Origine: origine})
}

func (c *documento) segmento(s evidenze.Segmento) { c.d.Segmenti = append(c.d.Segmenti, s) }

func (c *documento) entita(e evidenze.EntitaLocale) { c.d.Entita = append(c.d.Entita, e) }

// unita aggiunge un'unità della fonte del documento: FonteID lo mette il costruttore, mai l'adattatore.
func (c *documento) unita(u evidenze.UnitaEvidenza) {
	u.FonteID = c.d.Fonte.ID
	c.d.Unita = append(c.d.Unita, u)
}

func (c *documento) legame(l evidenze.LegameFonte) { c.d.Legami = append(c.d.Legami, l) }

// capacita dichiara una capacità del documento. Una capacità si dichiara una volta sola: chiudi rifiuta un
// nome ripetuto, perché due stati per la stessa capacità sarebbero una contraddizione dell'adattatore.
func (c *documento) capacita(nome, stato, motivo string) {
	c.d.Qualita.Capacita = append(c.d.Qualita.Capacita, evidenze.Capacita{Nome: nome, Stato: stato, Motivo: motivo})
}

func (c *documento) diagnostica(d evidenze.Diagnostica) {
	c.d.Qualita.Diagnostiche = append(c.d.Qualita.Diagnostiche, d)
}

// peggiora porta lo stato della fonte a quello dato, se è più grave di quello che ha.
func (c *documento) peggiora(stato string) {
	if gravitaDelloStato[stato] > gravitaDelloStato[c.d.Qualita.Stato] {
		c.d.Qualita.Stato = stato
	}
}

// chiudi finisce il documento:
//   - ordine canonico: testi, segmenti, entità, unità e legami per ID; capacità per nome; diagnostiche per
//     codice, riferimenti e messaggio. L'ordine dei fatti in ingresso non conta (par.3.4.3, regola 1);
//   - gli elenchi facoltativi vuoti diventano nil, quelli obbligatori (testi, unità) un elenco vuoto: una slice
//     vuota e una nil danno lo stesso canonico (regola 2);
//   - la porta della foglia, ValidaDocumento: un documento che non la passa è un errore dell'adattatore, e
//     torna come *evidenze.ErroreContratto con le diagnostiche documento.*, mai come documento;
//   - il BundleID: lo sha256 del canonico del documento con BundleID vuoto e senza Provenienza.Bytes
//     (par.3.4.2; R51 A).
func (c *documento) chiudi() (evidenze.DocumentoEvidenze, error) {
	d := c.d
	sort.SliceStable(d.Testi, func(i, j int) bool { return d.Testi[i].ID < d.Testi[j].ID })
	sort.SliceStable(d.Segmenti, func(i, j int) bool { return d.Segmenti[i].ID < d.Segmenti[j].ID })
	sort.SliceStable(d.Entita, func(i, j int) bool { return d.Entita[i].ID < d.Entita[j].ID })
	sort.SliceStable(d.Unita, func(i, j int) bool { return d.Unita[i].ID < d.Unita[j].ID })
	sort.SliceStable(d.Legami, func(i, j int) bool { return d.Legami[i].ID < d.Legami[j].ID })
	sort.SliceStable(d.Qualita.Capacita, func(i, j int) bool { return d.Qualita.Capacita[i].Nome < d.Qualita.Capacita[j].Nome })
	sort.SliceStable(d.Qualita.Diagnostiche, func(i, j int) bool {
		return chiaveDiagnostica(d.Qualita.Diagnostiche[i]) < chiaveDiagnostica(d.Qualita.Diagnostiche[j])
	})
	sort.Strings(d.Qualita.Limiti)
	sort.Strings(d.Fonte.Provenienza.Ignoti)

	for i := 1; i < len(d.Qualita.Capacita); i++ {
		if d.Qualita.Capacita[i].Nome == d.Qualita.Capacita[i-1].Nome {
			return evidenze.DocumentoEvidenze{}, fmt.Errorf("estrazione: la capacità %q è dichiarata due volte", d.Qualita.Capacita[i].Nome)
		}
	}

	if d.Testi == nil {
		d.Testi = []evidenze.TestoOriginale{}
	}
	if d.Unita == nil {
		d.Unita = []evidenze.UnitaEvidenza{}
	}
	if len(d.Segmenti) == 0 {
		d.Segmenti = nil
	}
	if len(d.Entita) == 0 {
		d.Entita = nil
	}
	if len(d.Legami) == 0 {
		d.Legami = nil
	}
	if len(d.Qualita.Capacita) == 0 {
		d.Qualita.Capacita = nil
	}
	if len(d.Qualita.Limiti) == 0 {
		d.Qualita.Limiti = nil
	}
	if len(d.Qualita.Diagnostiche) == 0 {
		d.Qualita.Diagnostiche = nil
	}
	if len(d.Fonte.Provenienza.Ignoti) == 0 {
		d.Fonte.Provenienza.Ignoti = nil
	}

	if diag := evidenze.ValidaDocumento(d); len(diag) > 0 {
		return evidenze.DocumentoEvidenze{}, &evidenze.ErroreContratto{Diagnostiche: diag}
	}

	impronta := d
	impronta.BundleID = ""
	impronta.Fonte.Provenienza.Bytes = nil
	h, err := jsoncanonico.ImprontaDi(impronta)
	if err != nil {
		return evidenze.DocumentoEvidenze{}, errors.Join(errors.New("estrazione: BundleID non calcolabile"), err)
	}
	d.BundleID = h
	return d, nil
}

// chiaveDiagnostica: la chiave dell'ordine canonico delle diagnostiche. I campi si separano con il byte 0 e i
// riferimenti con il byte 1: gli ID locali non contengono caratteri di controllo, quindi due diagnostiche
// diverse non hanno la stessa chiave.
func chiaveDiagnostica(d evidenze.Diagnostica) string {
	return d.Codice + "\x00" + strings.Join(d.Rif, "\x01") + "\x00" + d.Percorso + "\x00" + d.Messaggio
}
