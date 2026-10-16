package motorea

import (
	"fmt"
	"sort"
	"strings"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// I legami (GP-1, addendum §4): un modello di accoppiamento copre per intero il testo di un'unità (un nome di
// file, l'id di una radice STEP). Il riconoscimento dentro il legame è provvisorio: ogni pezzo «lettura» prova le
// forme delle sue famiglie ancorate in quel punto (anche quelle solo_legame), i separatori e l'estensione si
// confrontano come letterali, e solo se il modello arriva alla fine del testo le letture si emettono, con il loro
// ruolo, e nascono le relazioni proposte. Se il modello si ferma prima, niente: nessuna relazione e nessun
// frammento solo_legame (direttiva 21 §4.2). Più coperture diverse dello stesso testo: ambiguo, niente si sceglie.

// Le relazioni che nascono dai legami (RelazioneSemantica.Tipo). Sono sempre proposte.
const (
	RelazioneDocumentatoDa = "documentato_da"         // l'articolo del cliente è documentato dal disegno (o assieme) del fornitore
	RelazioneVarianti      = "varianti_nel_documento" // più varianti nella stessa tavola: nessuna sostituzione, nessuna scelta
	StatoRelazioneProposta = "proposta"
)

// Tetti della ricerca di una copertura: pochi pezzi e poche letture per punto; il tetto protegge da un testo
// costruito apposta. Superarlo vale «ambiguo».
const (
	maxCopertureLegame = 8
	maxPassiLegame     = 4096
)

// legameCompilato: un modello attivo con le famiglie risolte.
type legameCompilato struct {
	id        string
	selettori map[evidenze.Selettore]bool
	pezzi     []grammatica.PezzoLegame
}

// legame: il legame compilato con quell'ID, nil se non c'è.
func (m *Motore) legame(id string) *legameCompilato {
	for _, l := range m.legami {
		if l.id == id {
			return l
		}
	}
	return nil
}

// componiLegami: i legami attivi della grammatica del cliente, dopo i partner (le famiglie dei partner entrati
// sono note). Un legame che cita una famiglia che non c'è non entra, con un avviso.
func (c *compilazione) componiLegami() []evidenze.Diagnostica {
	var diag []evidenze.Diagnostica
	note := map[string]bool{}
	for _, f := range c.g.Famiglie {
		note[f.ID] = true
	}
	for _, p := range c.m.partner {
		for _, f := range p.famiglie {
			note[f] = true
		}
	}
	for _, l := range c.g.Legami {
		if l.Stato != grammatica.StatoAttiva {
			continue
		}
		manca := ""
		for _, pz := range l.Schema {
			for _, f := range pz.Famiglie {
				if !note[f] && manca == "" {
					manca = f
				}
			}
		}
		if manca != "" {
			diag = append(diag, evidenze.Diagnostica{Codice: CodiceLegameNonComposto, Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaContratto,
				Percorso: "legami[" + l.ID + "]", Messaggio: fmt.Sprintf("il legame cita la famiglia %q, che non c'è né fra i partner entrati: non entra nel motore", manca), Rif: []string{l.ID}})
			continue
		}
		lc := &legameCompilato{id: l.ID, selettori: map[evidenze.Selettore]bool{}, pezzi: l.Schema}
		for _, s := range l.Selettori {
			if sel, err := evidenze.LeggiSelettore(s); err == nil && grammatica.SelettoreAttivo(s) {
				lc.selettori[sel] = true
			}
		}
		c.m.legami = append(c.m.legami, lc)
	}
	return diag
}

// elementoCopertura: un pezzo lettura riuscito, con la lettura provvisoria e il ruolo del pezzo.
type elementoCopertura struct {
	ruolo   string // rif_cliente | parte_fornitore | variante
	lettura LetturaForma
}

// copertura: una copertura intera del testo.
type copertura []elementoCopertura

func (cp copertura) chiave() string {
	var b strings.Builder
	for _, e := range cp {
		fmt.Fprintf(&b, "%s|%s/%s|%d-%d;", e.ruolo, e.lettura.Famiglia, e.lettura.Forma, e.lettura.Intervallo.Inizio, e.lettura.Intervallo.Fine)
	}
	return b.String()
}

// coperture: le coperture intere del testo con il legame, al più maxCopertureLegame (oltre: troncato = vero),
// e fin dove è arrivato il cursore (per la diagnostica del legame incompleto).
func (m *Motore) coperture(l *legameCompilato, sel evidenze.Selettore, testo string, a *attivazione) (out []copertura, piuLontano int, troncato bool) {
	passi := 0
	var cerca func(i, pos int, acc copertura)
	cerca = func(i, pos int, acc copertura) {
		if troncato {
			return
		}
		passi++
		if passi > maxPassiLegame || len(out) >= maxCopertureLegame {
			troncato = true
			return
		}
		// Il punto più lontano conta solo dopo almeno due pezzi riusciti: un nome che comincia con un codice del cliente
		// e poi non c'entra niente non è «un legame incompleto».
		if i >= 2 && pos > piuLontano {
			piuLontano = pos
		}
		if i == len(l.pezzi) {
			if pos == len(testo) {
				out = append(out, append(copertura(nil), acc...))
			}
			return
		}
		pz := l.pezzi[i]
		switch pz.Tipo {
		case grammatica.PezzoSeparatore:
			for _, s := range pz.Letterali {
				if strings.HasPrefix(testo[pos:], s) {
					cerca(i+1, pos+len(s), acc)
				}
			}
		case grammatica.PezzoEstensione:
			if pos < len(testo) && testo[pos] == '.' {
				for _, e := range pz.Letterali {
					if strings.EqualFold(testo[pos+1:], e) {
						cerca(i+1, len(testo), acc)
					}
				}
			}
		case grammatica.PezzoLettura:
			// Due forme della stessa famiglia che leggono lo stesso tratto con la stessa base sono la stessa lettura: ne
			// resta una, quella di una forma che legge anche fuori dal legame (così la lettura coincide con quella della
			// scansione libera), altrimenti la prima in ordine di forma.
			scelte := map[string]LetturaForma{}
			var ordine []string
			for _, p := range m.piani[sel] {
				if !contiene(pz.Famiglie, p.famiglia) || !p.attivoNelLegame(sel, a) {
					continue
				}
				lf, ok := p.alInizio(sel, testo, pos)
				if !ok {
					continue
				}
				k := fmt.Sprintf("%s\x00%d\x00%s", lf.Famiglia, lf.Intervallo.Fine, lf.Base.Normalizzata)
				prima, gia := scelte[k]
				switch {
				case !gia:
					scelte[k] = lf
					ordine = append(ordine, k)
				case m.forme[chiaveForma(prima.Famiglia, prima.Forma)].condizione == condSoloLegame && p.condizione != condSoloLegame:
					scelte[k] = lf
				}
			}
			for _, k := range ordine {
				lf := scelte[k]
				cerca(i+1, lf.Intervallo.Fine, append(acc, elementoCopertura{ruolo: pz.Ruolo, lettura: lf}))
			}
		}
	}
	cerca(0, 0, nil)
	// Coperture uguali (stesse letture con gli stessi ruoli) arrivate da piani diversi contano una volta.
	viste := map[string]bool{}
	uniche := out[:0]
	for _, cp := range out {
		k := cp.chiave()
		if !viste[k] {
			viste[k] = true
			uniche = append(uniche, cp)
		}
	}
	return uniche, piuLontano, troncato
}

// alInizio: la lettura del piano che comincia esattamente in pos, senza il controllo del confine sinistro (nel
// legame il pezzo prima è dichiarato dallo schema). Il confine destro resta quello della forma.
func (p *pianoForma) alInizio(sel evidenze.Selettore, testo string, pos int) (LetturaForma, bool) {
	if pos >= len(testo) {
		return LetturaForma{}, false
	}
	loc := p.re.FindStringSubmatchIndex(testo[pos:p.fineRicerca(testo, pos)])
	if loc == nil {
		return LetturaForma{}, false
	}
	return p.lettura(sel, testo, pos, p.allunga(testo, pos, loc)), true
}

// ruoloLettura: il ruolo della lettura nell'interpretazione, dal ruolo del pezzo (addendum §5): il rif_cliente
// resta primaria (ruolo vuoto), la parte_fornitore è riferimento, le varianti sono varianti.
func ruoloLettura(ruoloPezzo string) string {
	switch ruoloPezzo {
	case grammatica.RuoloLegameFornitore:
		return grammatica.RuoloLetturaRiferimento
	case grammatica.RuoloLegameVariante:
		return grammatica.RuoloLetturaVariante
	}
	return ""
}

// legami: il passo dei legami di Interpreta (dopo la deduplica). Per ogni unità con un selettore di un legame
// ammesso dall'attivazione del documento prova il modello; con una copertura sola emette letture, ruoli e
// relazioni; con più coperture lo dice e non sceglie.
func (it *interprete) legami() {
	a := it.att
	if a == nil || len(a.modelli) == 0 {
		return
	}
	for _, u := range it.unita {
		if !it.lette[u.ID] {
			continue // un'unità tagliata da un limite non si accoppia
		}
		for _, l := range it.m.legami {
			if !a.modelli[l.id] || !l.selettori[u.Selettore] {
				continue
			}
			cps, lontano, troncato := it.m.coperture(l, u.Selettore, u.Testo, a)
			switch {
			case len(cps) == 1 && !troncato:
				it.applicaCopertura(u, l, cps[0])
			case len(cps) > 1 || troncato:
				var vie []string
				for _, cp := range cps {
					vie = append(vie, cp.chiave())
				}
				sort.Strings(vie)
				it.diag = append(it.diag, evidenze.Diagnostica{Codice: CodiceMotoreLegameAmbiguo, Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaDati,
					Percorso: "unita[" + u.ID + "]", Messaggio: fmt.Sprintf("il legame %s copre il testo in %d modi (troncato: %v): nessun ruolo e nessuna relazione si scelgono", l.id, len(cps), troncato),
					Rif: append([]string{l.id, u.ID}, vie...)})
			case lontano > 0:
				it.diag = append(it.diag, evidenze.Diagnostica{Codice: CodiceMotoreLegameIncompleto, Gravita: evidenze.GravitaNota, Natura: evidenze.NaturaDati,
					Percorso: "unita[" + u.ID + "]", Messaggio: fmt.Sprintf("il legame %s copre il testo fino al byte %d di %d e si ferma: nessuna relazione, nessuna lettura solo_legame", l.id, lontano, len(u.Testo)),
					Rif: []string{l.id, u.ID}})
			}
		}
	}
}

// applicaCopertura: le letture della copertura entrano (o, se la scansione libera le aveva già, prendono il
// ruolo), e nascono le relazioni proposte.
func (it *interprete) applicaCopertura(u evidenze.UnitaEvidenza, l *legameCompilato, cp copertura) {
	ids := make([]string, len(cp))
	for i, e := range cp {
		lc := it.lettura(u, e.lettura)
		pos := -1
		for j := range it.letture {
			if it.letture[j].ID == lc.ID {
				pos = j
				break
			}
		}
		if pos < 0 {
			it.letture = append(it.letture, lc)
			pos = len(it.letture) - 1
		}
		x := &it.letture[pos]
		if r := ruoloLettura(e.ruolo); r != "" {
			x.Ruolo = r
		}
		x.Motivi = append(x.Motivi, "legame "+l.id+": "+e.ruolo)
		ids[i] = x.ID
	}
	sorgente := u.EntitaID
	if sorgente == "" {
		sorgente = u.ID
	}
	var cliente string
	var altri, varianti []string
	for i, e := range cp {
		switch e.ruolo {
		case grammatica.RuoloLegameCliente:
			cliente = ids[i]
		case grammatica.RuoloLegameFornitore:
			altri = append(altri, ids[i])
		case grammatica.RuoloLegameVariante:
			altri = append(altri, ids[i])
			varianti = append(varianti, ids[i])
		}
	}
	if cliente != "" && len(altri) > 0 {
		it.relazioni = append(it.relazioni, RelazioneSemantica{
			ID:               "r:" + u.ID + ":" + l.id + ":" + RelazioneDocumentatoDa,
			Tipo:             RelazioneDocumentatoDa,
			EntitaSorgente:   sorgente,
			Riconoscimento:   "legame/" + l.id,
			LettureBersaglio: altri,
			Evidenze:         []string{u.ID},
			LetturaSorgente:  cliente,
			Modello:          l.id,
			Stato:            StatoRelazioneProposta,
		})
	}
	if len(varianti) >= 2 {
		it.relazioni = append(it.relazioni, RelazioneSemantica{
			ID:               "r:" + u.ID + ":" + l.id + ":" + RelazioneVarianti,
			Tipo:             RelazioneVarianti,
			EntitaSorgente:   sorgente,
			Riconoscimento:   "legame/" + l.id,
			LettureBersaglio: varianti,
			Evidenze:         []string{u.ID},
			Modello:          l.id,
			Stato:            StatoRelazioneProposta,
		})
	}
}

// ---- la verifica degli esempi dei legami (CompilaVerificato) ----

func (c *compilazione) verificaLegami() []evidenze.Diagnostica {
	var diag []evidenze.Diagnostica
	for _, gl := range c.g.Legami {
		l := c.m.legame(gl.ID)
		if l == nil {
			continue // riservato o non composto: la diagnosi c'è già, gli esempi non si verificano
		}
		for _, e := range gl.Esempi {
			p := "legami[" + gl.ID + "].esempi[" + e.ID + "]"
			fallisce := func(msg string) {
				diag = append(diag, evidenze.Diagnostica{Codice: CodiceLegameEsempio, Gravita: evidenze.GravitaErrore, Natura: evidenze.NaturaContratto, Percorso: p, Messaggio: msg, Rif: []string{e.ID}})
			}
			sel, err := evidenze.LeggiSelettore(e.Selettore)
			if err != nil || !l.selettori[sel] {
				fallisce(fmt.Sprintf("il selettore %q non è uno di quelli del legame", e.Selettore))
				continue
			}
			for _, pid := range e.Attivazione.Partner {
				if _, ok := c.m.partner[pid]; !ok {
					fallisce(fmt.Sprintf("l'attivazione cita il partner %q, che non è entrato nel motore", pid))
				}
			}
			a := attivazioneDiEsempio(e.Attivazione, sel)
			cps, _, troncato := c.m.coperture(l, sel, e.Testo, a)
			if e.Atteso.Nessuno {
				if len(cps) > 0 {
					fallisce(fmt.Sprintf("il legame copre il testo (%d coperture), l'esempio attende nessun legame", len(cps)))
				}
				continue
			}
			if len(cps) != 1 || troncato {
				fallisce(fmt.Sprintf("coperture del testo: %d (troncato: %v); l'esempio ne attende una", len(cps), troncato))
				continue
			}
			cp := cps[0]
			if len(cp) != len(e.Atteso.Letture) {
				fallisce(fmt.Sprintf("letture del legame %d, attese %d", len(cp), len(e.Atteso.Letture)))
				continue
			}
			for i, la := range e.Atteso.Letture {
				el := cp[i]
				ruolo := ruoloLettura(el.ruolo)
				if ruolo == "" {
					ruolo = grammatica.RuoloLetturaPrimaria
				}
				switch {
				case la.Famiglia != "" && la.Famiglia != el.lettura.Famiglia:
					fallisce(fmt.Sprintf("lettura %d: famiglia %s, attesa %s", i, el.lettura.Famiglia, la.Famiglia))
				case la.Forma != "" && la.Forma != el.lettura.Forma:
					fallisce(fmt.Sprintf("lettura %d: forma %s, attesa %s", i, el.lettura.Forma, la.Forma))
				case la.Ruolo != "" && la.Ruolo != ruolo:
					fallisce(fmt.Sprintf("lettura %d: ruolo %s, atteso %s", i, ruolo, la.Ruolo))
				case !campiUguali(la, el.lettura):
					fallisce(fmt.Sprintf("lettura %d: campi diversi dall'atteso (%s)", i, campiDiversi(la, el.lettura)))
				}
			}
		}
	}
	return diag
}
