package motorea

import (
	"fmt"
	"sort"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// La verifica degli esempi (parte 1 §5.4, §7.1). Ogni esempio passa da Riconosci sul motore intero del
// cliente, quindi anche dalle altre famiglie che hanno forme sullo stesso selettore: l'insieme delle letture
// deve coincidere con l'atteso, salvo altre_ammesse. Non si pretende di dimostrare la disgiunzione di regex
// arbitrarie (P1 §7.1): si guardano le letture che gli esempi producono davvero.
//   - Lettura attesa assente: grammatica.esempio_mancante; lettura in più: grammatica.esempio_in_eccesso;
//     campi diversi: grammatica.esempio_diverso.
//   - Due forme della stessa famiglia sulla stessa occorrenza: forma.collisione_esempi, anche con
//     altre_ammesse (R25 f: le letture annidate si evitano con confini e selettori dichiarati).
//   - Un esempio su un selettore senza forme attive né revisioni in campo separato:
//     grammatica.esempio_selettore_inattivo.
//   - Gli esempi delle forme che non entrano nel motore (riservate, o con una capacità riservata) non si
//     verificano: grammatica.esempio_non_verificato, una nota, mai un «passato».
//   - Un esempio su un selettore che ha solo revisioni in campo separato nella sua famiglia si verifica con la
//     stessa funzione che A1b userà per l'attributo: la lettura attesa ha Forma vuota e Revisione valorizzata
//     (D-07).
//
// I campi vuoti di una lettura attesa non si controllano. Il riferimento al caso degli attesi che l'esempio
// può portare non si legge (R47 b). I messaggi non riportano il testo dell'esempio.

// verificaEsempi: gli esempi di tutte le famiglie, nell'ordine della grammatica normalizzata (famiglie ed
// esempi in ordine di ID).
func (c *compilazione) verificaEsempi() []evidenze.Diagnostica {
	var diag []evidenze.Diagnostica
	for _, f := range c.g.Famiglie {
		for _, e := range f.Esempi {
			diag = append(diag, c.verificaEsempio(f, e)...)
		}
	}
	return diag
}

func (c *compilazione) verificaEsempio(f grammatica.FamigliaCodice, e grammatica.EsempioCodice) []evidenze.Diagnostica {
	p := percorsoFamiglia(f.ID) + ".esempi[" + e.ID + "]"

	// Che cosa non si verifica.
	if !grammatica.SelettoreAttivo(e.Selettore) {
		return []evidenze.Diagnostica{nonVerificato(p, e.ID, fmt.Sprintf("il selettore %q non è attivo in A1", e.Selettore))}
	}
	sel, err := evidenze.LeggiSelettore(e.Selettore)
	if err != nil {
		return []evidenze.Diagnostica{nonVerificato(p, e.ID, fmt.Sprintf("il selettore %q non si legge", e.Selettore))}
	}
	for _, la := range e.Atteso.Letture {
		fam := famigliaAttesa(la, f.ID)
		if la.Forma != "" && !c.abbassate[chiaveForma(fam, la.Forma)] {
			return []evidenze.Diagnostica{nonVerificato(p, e.ID, fmt.Sprintf("attende la forma %s/%s, che non entra nel motore", fam, la.Forma))}
		}
		// Una lettura attesa senza forma, di un'altra famiglia che sul selettore ha solo forme o revisioni fuori
		// dal motore: come per la famiglia dell'esempio (sotto), non si verifica.
		if k := chiaveSelettore(fam, e.Selettore); la.Forma == "" && fam != f.ID && !c.attive[k] && c.dichiarate[k] {
			return []evidenze.Diagnostica{nonVerificato(p, e.ID, fmt.Sprintf("attende una lettura della famiglia %q, che ha sul selettore %q solo forme o revisioni che non entrano nel motore", fam, e.Selettore))}
		}
	}
	chiave := chiaveSelettore(f.ID, e.Selettore)
	if !c.attive[chiave] && c.dichiarate[chiave] {
		return []evidenze.Diagnostica{nonVerificato(p, e.ID, fmt.Sprintf("la famiglia %q ha sul selettore %q solo forme o revisioni che non entrano nel motore", f.ID, e.Selettore))}
	}
	if len(c.m.piani[sel]) == 0 && len(c.m.revisioniCampo[sel]) == 0 {
		return []evidenze.Diagnostica{{
			Codice:    CodiceEsempioSelettoreInattivo,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  p + ".selettore",
			Messaggio: fmt.Sprintf("nessuna forma attiva e nessuna revisione in campo separato sul selettore %q: l'esempio non si può verificare", e.Selettore),
			Rif:       []string{e.ID},
		}}
	}

	// Le letture prodotte: forme e revisioni in campo separato, del motore intero.
	forme, dr := c.m.Riconosci(sel, e.Testo)
	var diag []evidenze.Diagnostica
	for _, d := range dr {
		if d.Codice == grammatica.CodiceLimiteSuperato {
			diag = append(diag, d) // le altre (revisione.da_verificare) sono il comportamento atteso dell'esempio
		}
	}
	revisioni := c.m.leggiRevisioniCampo(sel, e.Testo)

	diag = append(diag, collisioni(p, e.ID, forme)...)

	// Le letture attese, divise fra forme e revisioni in campo separato (D-07), si abbinano a quelle prodotte
	// tutte insieme (abbina), non una alla volta nell'ordine dell'array: un'attesa poco specificata non toglie
	// la lettura a una più specificata (nessuna precedenza per ordine di array).
	attese := e.Atteso.Letture
	fams := make([]string, len(attese))
	diRevisione := make([]bool, len(attese))
	var idxForme, idxRev []int
	for i, la := range attese {
		fams[i] = famigliaAttesa(la, f.ID)
		if la.Forma == "" && c.campo[chiaveSelettore(fams[i], e.Selettore)] {
			diRevisione[i] = true
			idxRev = append(idxRev, i)
		} else {
			idxForme = append(idxForme, i)
		}
	}
	perForma, usateForme := abbina(len(idxForme), len(forme), func(a, l int) bool {
		i := idxForme[a]
		return candidataForma(attese[i], fams[i], forme[l]) && campiUguali(attese[i], forme[l])
	})
	perRev, usateRev := abbina(len(idxRev), len(revisioni), func(a, l int) bool {
		i := idxRev[a]
		return revisioni[l].famiglia == fams[i] && revisioneUguale(attese[i], revisioni[l].rev)
	})
	abbinata := make([]bool, len(attese))
	for a, l := range perForma {
		abbinata[idxForme[a]] = l >= 0
	}
	for a, l := range perRev {
		abbinata[idxRev[a]] = l >= 0
	}
	// Le attese rimaste, nell'ordine dell'array: la prima candidata ancora libera è «diversa», altrimenti la
	// lettura manca. L'ordine qui sceglie solo quale errore si scrive, mai se l'esempio passa.
	for i, la := range attese {
		if abbinata[i] {
			continue
		}
		pl := fmt.Sprintf("%s.atteso.letture[%d]", p, i)
		if diRevisione[i] {
			diag = append(diag, revisioneNonAbbinata(pl, e.ID, fams[i], revisioni, usateRev)...)
			continue
		}
		diag = append(diag, formaNonAbbinata(pl, e.ID, la, fams[i], forme, usateForme)...)
	}
	if e.Atteso.AltreAmmesse {
		return diag
	}
	for i, l := range forme {
		if !usateForme[i] {
			diag = append(diag, inEccesso(p, e.ID, fmt.Sprintf("lettura %s/%s in [%d,%d) non attesa", l.Famiglia, l.Forma, l.Intervallo.Inizio, l.Intervallo.Fine)))
		}
	}
	for i, r := range revisioni {
		if !usateRev[i] {
			diag = append(diag, inEccesso(p, e.ID, fmt.Sprintf("revisione in campo separato %s/%s non attesa", r.famiglia, r.rev.Regola)))
		}
	}
	return diag
}

// famigliaAttesa: una lettura attesa senza famiglia vale per la famiglia dell'esempio.
func famigliaAttesa(la grammatica.LetturaAttesa, famiglia string) string {
	if la.Famiglia != "" {
		return la.Famiglia
	}
	return famiglia
}

// collisioni: due forme diverse della stessa famiglia con intervalli che si sovrappongono.
func collisioni(p, id string, forme []LetturaForma) []evidenze.Diagnostica {
	var diag []evidenze.Diagnostica
	for i := range forme {
		for j := i + 1; j < len(forme); j++ {
			a, b := forme[i], forme[j]
			if a.Famiglia != b.Famiglia || a.Forma == b.Forma {
				continue
			}
			if a.Intervallo.Inizio < b.Intervallo.Fine && b.Intervallo.Inizio < a.Intervallo.Fine {
				diag = append(diag, evidenze.Diagnostica{
					Codice:    CodiceFormaCollisioneEsempi,
					Gravita:   evidenze.GravitaErrore,
					Natura:    evidenze.NaturaContratto,
					Percorso:  p,
					Messaggio: fmt.Sprintf("le forme %s/%s e %s/%s leggono la stessa occorrenza: nessuna precedenza per ordine, servono confini o selettori dichiarati (R25 f)", a.Famiglia, a.Forma, b.Famiglia, b.Forma),
					Rif:       []string{id, a.Forma, b.Forma},
				})
			}
		}
	}
	return diag
}

// abbina: per ogni attesa la lettura prodotta che le corrisponde (corrisponde(a, l)), o -1, e quali letture
// sono state usate. L'abbinamento accoppia il maggior numero possibile di attese, con i cammini aumentanti
// di un abbinamento bipartito, in un ordine fisso: l'esito è deterministico, e un'attesa resta senza lettura
// solo se nessun abbinamento la poteva servire, qualunque sia l'ordine dell'array atteso. Le attese sono
// poche (i limiti della grammatica): il costo, quadratico, non conta.
func abbina(nAttese, nLette int, corrisponde func(a, l int) bool) ([]int, []bool) {
	diLettura := make([]int, nLette) // l'attesa abbinata a ogni lettura, o -1
	for l := range diLettura {
		diLettura[l] = -1
	}
	var prova func(a int, viste []bool) bool
	prova = func(a int, viste []bool) bool {
		for l := 0; l < nLette; l++ {
			if viste[l] || !corrisponde(a, l) {
				continue
			}
			viste[l] = true
			if diLettura[l] < 0 || prova(diLettura[l], viste) {
				diLettura[l] = a
				return true
			}
		}
		return false
	}
	for a := 0; a < nAttese; a++ {
		prova(a, make([]bool, nLette))
	}
	perAttesa := make([]int, nAttese)
	for a := range perAttesa {
		perAttesa[a] = -1
	}
	usate := make([]bool, nLette)
	for l, a := range diLettura {
		if a >= 0 {
			perAttesa[a] = l
			usate[l] = true
		}
	}
	return perAttesa, usate
}

// candidataForma: la lettura è della famiglia attesa, e della forma attesa se l'atteso la dice.
func candidataForma(la grammatica.LetturaAttesa, fam string, l LetturaForma) bool {
	return l.Famiglia == fam && (la.Forma == "" || l.Forma == la.Forma)
}

// formaNonAbbinata: una lettura attesa di forma che l'abbinamento non ha servito. La prima candidata ancora
// libera è «diversa» (e si usa, così non torna in eccesso); senza candidate la lettura manca.
func formaNonAbbinata(pl, id string, la grammatica.LetturaAttesa, fam string, forme []LetturaForma, usate []bool) []evidenze.Diagnostica {
	prima := -1
	for i, l := range forme {
		if !usate[i] && candidataForma(la, fam, l) {
			prima = i
			break
		}
	}
	nome := fam
	if la.Forma != "" {
		nome += "/" + la.Forma
	}
	if prima < 0 {
		return []evidenze.Diagnostica{{
			Codice:    CodiceEsempioMancante,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  pl,
			Messaggio: fmt.Sprintf("lettura attesa di %s non prodotta", nome),
			Rif:       []string{id},
		}}
	}
	usate[prima] = true
	return []evidenze.Diagnostica{diversa(pl, id, fmt.Sprintf("lettura di %s con campi diversi dall'atteso (%s)", nome, campiDiversi(la, forme[prima])))}
}

// revisioneNonAbbinata: una lettura attesa di revisione in campo separato (D-07) che l'abbinamento non ha
// servito; come formaNonAbbinata.
func revisioneNonAbbinata(pl, id string, fam string, revisioni []revisioneDiFamiglia, usate []bool) []evidenze.Diagnostica {
	prima := -1
	for i, r := range revisioni {
		if !usate[i] && r.famiglia == fam {
			prima = i
			break
		}
	}
	if prima < 0 {
		return []evidenze.Diagnostica{{
			Codice:    CodiceEsempioMancante,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Percorso:  pl,
			Messaggio: fmt.Sprintf("revisione in campo separato attesa per la famiglia %q non letta", fam),
			Rif:       []string{id},
		}}
	}
	usate[prima] = true
	return []evidenze.Diagnostica{diversa(pl, id, fmt.Sprintf("revisione in campo separato della famiglia %q diversa dall'atteso", fam))}
}

// campiUguali: ogni campo dichiarato della lettura attesa coincide con la lettura.
func campiUguali(la grammatica.LetturaAttesa, l LetturaForma) bool {
	return campiDiversi(la, l) == ""
}

// campiDiversi: i nomi dei campi dichiarati che non coincidono, in un ordine fisso; vuoto se coincidono.
//   - base: la base normalizzata;
//   - marcatore: il valore del marcatore;
//   - revisione: la revisione normalizzata, solo se letta (un token sospeso non ha valore);
//   - mancanti: come insieme;
//   - affissi e decorazioni: come insiemi del testo letto, come li scrive l'esempio del piano A (par.4.6.6:
//     le decorazioni attese sono i testi letti, non gli ID delle regole). Un solo significato: un ID uguale al
//     letterale di un'altra regola non può far passare un esempio sbagliato.
func campiDiversi(la grammatica.LetturaAttesa, l LetturaForma) string {
	var diversi []string
	if la.Base != "" && la.Base != l.Base.Normalizzata {
		diversi = append(diversi, "base")
	}
	if la.Marcatore != "" && (l.Marcatore == nil || l.Marcatore.Valore != la.Marcatore) {
		diversi = append(diversi, "marcatore")
	}
	if la.Revisione != "" && (l.Revisione == nil || l.Revisione.Stato != StatoRevisioneLetta || l.Revisione.Normalizzata != la.Revisione) {
		diversi = append(diversi, "revisione")
	}
	if len(la.Mancanti) > 0 && !stessoInsieme(la.Mancanti, l.Base.Mancanti) {
		diversi = append(diversi, "mancanti")
	}
	if len(la.Affissi) > 0 {
		letti := make([]string, len(l.Affissi))
		for i, a := range l.Affissi {
			letti[i] = a.Originale
		}
		if !stessoInsieme(la.Affissi, letti) {
			diversi = append(diversi, "affissi")
		}
	}
	if len(la.Decorazioni) > 0 {
		lette := make([]string, len(l.Decorazioni))
		for i, d := range l.Decorazioni {
			lette[i] = d.Valore
		}
		if !stessoInsieme(la.Decorazioni, lette) {
			diversi = append(diversi, "decorazioni")
		}
	}
	out := ""
	for i, d := range diversi {
		if i > 0 {
			out += ", "
		}
		out += d
	}
	return out
}

// revisioneUguale: per una revisione in campo separato conta solo la revisione attesa, normalizzata e letta;
// un campo di forma dichiarato non può coincidere.
func revisioneUguale(la grammatica.LetturaAttesa, r *RevisioneLetta) bool {
	if la.Base != "" || la.Marcatore != "" || len(la.Mancanti) > 0 || len(la.Affissi) > 0 || len(la.Decorazioni) > 0 {
		return false
	}
	return la.Revisione == "" || (r.Stato == StatoRevisioneLetta && r.Normalizzata == la.Revisione)
}

// stessoInsieme: gli stessi valori, con le stesse ripetizioni, in qualunque ordine.
func stessoInsieme(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func nonVerificato(p, id, perche string) evidenze.Diagnostica {
	return evidenze.Diagnostica{
		Codice:    CodiceEsempioNonVerificato,
		Gravita:   evidenze.GravitaNota,
		Natura:    evidenze.NaturaCapacita,
		Percorso:  p,
		Messaggio: "esempio non verificato: " + perche + "; non conta come passato",
		Rif:       []string{id},
	}
}

func inEccesso(p, id, cosa string) evidenze.Diagnostica {
	return evidenze.Diagnostica{
		Codice:    CodiceEsempioInEccesso,
		Gravita:   evidenze.GravitaErrore,
		Natura:    evidenze.NaturaContratto,
		Percorso:  p,
		Messaggio: cosa + ": l'esempio non la attende e non ammette altre letture",
		Rif:       []string{id},
	}
}

func diversa(pl, id, cosa string) evidenze.Diagnostica {
	return evidenze.Diagnostica{
		Codice:    CodiceEsempioDiverso,
		Gravita:   evidenze.GravitaErrore,
		Natura:    evidenze.NaturaContratto,
		Percorso:  pl,
		Messaggio: cosa,
		Rif:       []string{id},
	}
}
