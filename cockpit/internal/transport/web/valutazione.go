package web

import (
	"fmt"
	"html"
	"html/template"
	"strings"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// La resa degli score (Smistamento F3, A5.14.5). Uno score e' di REGOLA: ordina le proposte, non e' una
// probabilita', e finche' la calibrazione non l'ha misurato non si scrive mai «85%» (decisioni del 27/09).
// Si scrive «score 85», sempre accanto alla dimensione di cui parla (tipo, codice, rev) e alla regola in
// parole: «CAD 3D = 90» non deve piu' leggersi «associazione = 90». Una decisione non ha score: ha un autore.

// titoloScore e titoloScoreS1 sono il tooltip di ogni score (U-C1): lo stesso ovunque, perche' la stessa
// cifra non si legga in due modi.
const (
	titoloScore   = "score di regola, non calibrato: ordina, non è una probabilità"
	titoloScoreS1 = "score di regola (tabella S1), non calibrato: ordina, non è una probabilità"
)

// vistaDimensione e' una dimensione della lettura di un file, con quello che la schermata le mette intorno.
type vistaDimensione struct {
	Nome        string // «Tipo rilevato», «Codice letto», «Rev»: vuoto quando la etichetta ce l'ha gia' la schermata
	Quale       string // tipo | codice | rev
	D           classificazione.Dimensione
	Ricostruita bool // riga di prima, riletta dalle colonne (U-C3)
	Decisa      bool // l'ha scritta una persona: nessuno score (U-C4)
}

// vistaValutazione sono le tre dimensioni di una proposta, pronte per il frammento «dimensione».
type vistaValutazione struct {
	Tipo, Codice, Rev   vistaDimensione
	Ricostruita, Decisa bool
}

// valutazioneDi legge la valutazione di una proposta: `dettagli.valutazione` quando c'e', altrimenti
// ricostruita dalle colonne con ValutazioneDaRiga (le righe di oggi). Una riga con fonte `operatore` e' una
// decisione e si legge dalle colonne anche se porta la lettura della macchina su cui si e' deciso. Non
// scrive niente: e' una lettura, e si fa anche in GET (R8).
func valutazioneDi(p *db.DocumentoProposta, nomeFile string) vistaValutazione {
	if p == nil {
		return vistaValutazione{}
	}
	v := classificazione.ValutazioneDellaRiga(string(p.TipoProposto), p.Codice.String, p.Rev.String, string(p.Fonte),
		int(p.Confidenza), p.Dettagli, nomeFile, "")
	decisa := v.Decisa() || p.Fonte == db.FontePropostaOperatore
	dim := func(quale string, d classificazione.Dimensione) vistaDimensione {
		return vistaDimensione{Quale: quale, D: d, Ricostruita: v.Ricostruita && !decisa, Decisa: decisa}
	}
	return vistaValutazione{Tipo: dim("tipo", v.Tipo), Codice: dim("codice", v.Codice), Rev: dim("rev", v.Rev),
		Ricostruita: v.Ricostruita && !decisa, Decisa: decisa}
}

// Valore e' il valore della dimensione con le parole della schermata (il tipo con la sua etichetta).
func (d vistaDimensione) Valore() string {
	if d.Quale == "tipo" && d.D.Valore != "" {
		return etichettaTipoDoc(d.D.Valore)
	}
	return d.D.Valore
}

// ConValore dice se la dimensione ha un valore da mostrare con il suo score.
func (d vistaDimensione) ConValore() bool {
	return !d.Decisa && d.D.Stato != classificazione.StatoNessuna && d.D.Valore != ""
}

// Regola e' la regola vincente in parole, con il frammento letto quando dice qualcosa («estensione .stp»,
// «convenzione generica _1»).
func (d vistaDimensione) Regola() string {
	if !d.ConValore() {
		// «nessuna evidenza (PDF non ancora letto)»: il perche', senza il frammento
		return paroleRegola(d.D.Regola, "")
	}
	for _, e := range d.D.Evidenze {
		if e.Regola == d.D.Regola {
			return paroleEvidenza(e)
		}
	}
	return paroleRegola(d.D.Regola, "")
}

// Fonti conta le fonti indipendenti che dicono il valore vincente (U-C3: «2 fonti»).
func (d vistaDimensione) Fonti() int {
	n := 0
	for _, e := range d.D.Evidenze {
		if e.Valore != "" && e.DipendeDa == "" && strings.EqualFold(e.Valore, d.D.Valore) {
			n++
		}
	}
	return n
}

// ConEvidenze dice se vale la pena aprire l'elenco delle evidenze: piu' di una, o una discordanza.
func (d vistaDimensione) ConEvidenze() bool {
	return !d.Decisa && (len(d.D.Evidenze) > 1 || d.D.Stato == classificazione.StatoDiscorde)
}

// Testo e' la dimensione in una riga di testo semplice, per i `title`: «Tipo · Disegno 2D · score 75 ·
// termini da cartiglio nel testo».
func (d vistaDimensione) Testo() string {
	var b strings.Builder
	if d.Nome != "" {
		b.WriteString(d.Nome + " · ")
	}
	switch {
	case d.Decisa:
		if d.D.Valore != "" {
			b.WriteString(d.Valore() + " · ")
		}
		b.WriteString("deciso da una persona")
	case !d.ConValore():
		b.WriteString("nessuna evidenza")
		if r := d.Regola(); r != "" {
			b.WriteString(" (" + r + ")")
		}
	default:
		fmt.Fprintf(&b, "%s · score %d · %s", d.Valore(), d.D.Score, d.Regola())
		switch d.D.Stato {
		case classificazione.StatoConcorde:
			fmt.Fprintf(&b, " · %d fonti", d.Fonti())
		case classificazione.StatoDiscorde:
			b.WriteString(" · fonti discordi")
		}
		b.WriteString(" (" + titoloScore + ")")
	}
	if d.Ricostruita {
		b.WriteString(" · lettura precedente, ricostruita")
	}
	return b.String()
}

// paroleRegola e' la regola come la legge l'operatore; una regola che la tabella non conosce si dice col suo
// nome, mai con un numero da solo.
func paroleRegola(regola, testo string) string {
	r, ok := classificazione.Punteggi[regola]
	if !ok {
		return strings.ReplaceAll(regola, "_", " ")
	}
	if r.ConTesto && testo != "" {
		return r.Parole + " " + testo
	}
	return r.Parole
}

// paroleEvidenza e' una evidenza in parole, per l'elenco delle evidenze di una dimensione.
func paroleEvidenza(e classificazione.Evidenza) string {
	s := paroleRegola(e.Regola, e.Testo)
	r := classificazione.Punteggi[e.Regola]
	if !r.ConTesto && e.Testo != "" && e.Regola != "nome_codice_generico" {
		s += " («" + e.Testo + "»)"
	}
	if e.Famiglia != "" {
		s += ", famiglia «" + e.Famiglia + "»"
	}
	return s
}

// dimVista da' il nome alla dimensione per il frammento: `{{template "dimensione" (dim "Tipo" $v.Tipo)}}`.
func dimVista(nome string, d vistaDimensione) vistaDimensione {
	d.Nome = nome
	return d
}

// scoreHTML e' la funzione `score` dei template (U-C1): «score N» dentro `<span class="punteggio">` con il
// tooltip che dice che cosa e' e che cosa non e'. Per una dimensione senza evidenza, e per una decisione,
// niente: «nessuna evidenza» e «deciso da una persona» li scrive chi la mostra, senza numero.
func scoreHTML(x any) template.HTML {
	titolo, n := titoloScore, 0
	switch v := x.(type) {
	case vistaDimensione:
		if !v.ConValore() {
			return ""
		}
		titolo, n = titoloScoreS1, v.D.Score
	case classificazione.Dimensione:
		if v.Stato == classificazione.StatoNessuna || v.Regola == classificazione.RegolaOperatore {
			return ""
		}
		titolo, n = titoloScoreS1, v.Score
	case classificazione.Evidenza:
		if v.Valore == "" || v.Regola == classificazione.RegolaOperatore {
			return ""
		}
		titolo, n = titoloScoreS1, v.Score
	case int, int16, int32, int64:
		n = intero(v)
	default:
		return template.HTML(fmt.Sprintf(`<span class="punteggio" title="%s">score %s</span>`, html.EscapeString(titolo), html.EscapeString(fmt.Sprint(x))))
	}
	return template.HTML(fmt.Sprintf(`<span class="punteggio" title="%s">score %d</span>`, html.EscapeString(titolo), n))
}
