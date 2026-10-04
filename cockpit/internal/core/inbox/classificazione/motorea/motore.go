// Package motorea è il motore A. In A1a fa due cose:
//   - compila la grammatica v1 di un cliente in un Motore immutabile e ne verifica tutti gli esempi
//     (CompilaVerificato; CompilaInsieme per tutti i clienti dell'indice);
//   - riconosce le forme sui testi, con una regex per ogni famiglia e forma, mai tutte in alternanza (v3
//     §4.3), e restituisce le letture di forma (Riconosci).
//
// L'interpretazione di un DocumentoEvidenze, con il router, la funzione e i ruoli, arriva in A1b (R23).
//
// È puro e deterministico: niente DB, file, orologio, rete, goroutine o LLM; nessun ordine dipende da una
// mappa. Non usa il motore legacy di classificazione (quindi nemmeno il riconoscitore della minuteria, R7) e
// non lo può usare: sta nella sua cartella ma non lo importa. I limiti con cui lavora vengono dall'indice
// delle regole: non ne ha di suoi (R43 B). Nessun tipo, costante o ramo per un cliente: famiglie, forme e
// indice sono dati, e il runtime è privato e minimo (par.12).
package motorea

import (
	"fmt"
	"sort"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// VersioneAlgoritmo: riconoscimento (A1a) e interpretazione (A1b). VersioneRouter e VersioneRisultato
// nascono con il router in A1b (R23). Le versioni degli algoritmi sono costanti del codice; quella dei limiti
// no: la dichiara l'indice (R43 B).
const VersioneAlgoritmo = "motorea-1"

// Motore: le regole compilate di un cliente. Lo costruisce solo CompilaVerificato; verifica degli esempi,
// banco e anteprima lo usano nello stesso modo: non c'è un secondo motore.
//
// È privato e minimo, non una copia dei tipi della grammatica (par.12): piani indicizzati per selettore,
// regex RE2 compilate, gruppi catturati con la loro operazione. Una capacità riservata non ha piani. Dopo
// CompilaVerificato non cambia più: si può usare da più goroutine del chiamante.
type Motore struct {
	snap           grammatica.SnapshotRegole
	lim            grammatica.LimitiRiconoscimento
	piani          map[evidenze.Selettore][]*pianoForma     // in ordine di (famiglia, forma)
	revisioniCampo map[evidenze.Selettore][]*pianoRevisione // le revisioni in campo separato (D-07)
}

// CompilaVerificato valida lo snapshot con i limiti ricevuti, compila una volta ogni coppia (famiglia, forma)
// attiva, verifica tutti gli esempi con Riconosci sul motore intero del cliente e restituisce un motore
// immutabile (parte 1 §7.1). Con un errore la diagnosi c'è, il motore no e l'attivazione è vietata: l'errore
// è un *evidenze.ErroreContratto con tutte le diagnostiche. Le collisioni osservate sugli esempi sono errori:
// nessuna precedenza per ordine di array. Gli esempi delle forme che non entrano nel motore non si verificano:
// restano «non verificati», mai «passati». I limiti sono quelli dell'indice (R43 B): il motore non ne ha di
// suoi. Il riferimento al caso degli attesi che un esempio può portare è un metadato opaco: qui non si legge
// mai (R47 b).
//
// Le diagnostiche, in un ordine fisso: quelle della validazione (avvisi e note: capacità riservate, forme
// riservate, riserve), poi quelle della compilazione, poi quelle degli esempi, famiglia per famiglia.
func CompilaVerificato(s grammatica.SnapshotRegole, lim grammatica.Limiti) (*Motore, []evidenze.Diagnostica, error) {
	// La forma canonica anche qui: famiglie, forme ed esempi in ordine di ID, qualunque sia lo snapshot
	// ricevuto (A1a-DET).
	g := grammatica.Normalizza(s.Grammatica)
	diag := g.Valida(lim)
	if conErrori(diag) {
		return nil, diag, &evidenze.ErroreContratto{Diagnostiche: diag}
	}
	c := &compilazione{
		g: g,
		m: &Motore{
			snap:           s,
			lim:            lim.Riconoscimento,
			piani:          map[evidenze.Selettore][]*pianoForma{},
			revisioniCampo: map[evidenze.Selettore][]*pianoRevisione{},
		},
		abbassate:  map[string]bool{},
		dichiarate: map[string]bool{},
		attive:     map[string]bool{},
		campo:      map[string]bool{},
	}
	diag = append(diag, c.abbassa()...)
	if conErrori(diag) {
		return nil, diag, &evidenze.ErroreContratto{Diagnostiche: diag}
	}
	diag = append(diag, c.verificaEsempi()...)
	if conErrori(diag) {
		return nil, diag, &evidenze.ErroreContratto{Diagnostiche: diag}
	}
	return c.m, diag, nil
}

// Snapshot: lo snapshot da cui il motore è stato compilato, così come è arrivato. Le sue slice sono quelle
// ricevute: chi lo legge non le modifica. I piani non ne dipendono (vengono dalla copia di Normalizza), e
// MotoreDi ne legge solo il cliente, che è un valore.
func (m *Motore) Snapshot() grammatica.SnapshotRegole { return m.snap }

// Riconosci applica a un testo i piani attivi per quel selettore, ognuno da solo (v3 §4.3), e restituisce
// tutte le letture con gli intervalli in byte sul testo dato, in ordine di (inizio, fine, famiglia, forma).
// Non assegna funzione né ruoli: quelli li dà Interpreta in A1b. È la sola funzione di riconoscimento: la
// usano la verifica degli esempi, il banco, l'anteprima e Interpreta.
//
// Un selettore senza piani dà zero letture e nessuna diagnostica: che cosa voglia dire lo dice chi chiama.
// Le diagnostiche: limite.superato (avviso) se il testo o le letture superano i limiti dell'indice, con un
// risultato parziale; revisione.da_verificare (avviso) per ogni lettura con un token di revisione sospeso.
func (m *Motore) Riconosci(sel evidenze.Selettore, testo string) ([]LetturaForma, []evidenze.Diagnostica) {
	if m == nil {
		return nil, nil
	}
	piani := m.piani[sel]
	if len(piani) == 0 {
		return nil, nil
	}
	var diag []evidenze.Diagnostica
	if _, superato := fineInizi(testo, m.lim); superato {
		diag = append(diag, diagnosticaByte(len(testo), m.lim.MaxByteUnita))
	}
	var letture []LetturaForma
	troppe := false
	for _, p := range piani {
		l, d := scandisci(p, sel, testo, m.lim)
		letture = append(letture, l...)
		if len(d) > 0 {
			troppe = true
		}
	}
	sort.SliceStable(letture, func(i, j int) bool { return primaDi(letture[i], letture[j]) })
	// Ogni piano si ferma alle sue prime max_letture_per_unita letture, in ordine di inizio: le prime nell'ordine
	// complessivo sono quindi tutte qui, e il taglio è esatto.
	letture, tagliate := tagliaLetture(letture, m.lim)
	if troppe || tagliate {
		diag = append(diag, diagnosticaLetture(m.lim.MaxLetturePerUnita))
	}
	for _, l := range letture {
		if l.Revisione != nil && l.Revisione.Stato == StatoRevisioneNonInterpretabile {
			diag = append(diag, evidenze.Diagnostica{
				Codice:    CodiceRevisioneDaVerificare,
				Gravita:   evidenze.GravitaAvviso,
				Natura:    evidenze.NaturaDati,
				Messaggio: fmt.Sprintf("lettura %s/%s in [%d,%d): token di revisione sospeso, conservato senza valore; da verificare (D5)", l.Famiglia, l.Forma, l.Intervallo.Inizio, l.Intervallo.Fine),
				Rif:       []string{l.Revisione.Regola},
			})
		}
	}
	return letture, diag
}

// primaDi: l'ordine delle letture, (inizio, fine, famiglia, forma).
func primaDi(a, b LetturaForma) bool {
	switch {
	case a.Intervallo.Inizio != b.Intervallo.Inizio:
		return a.Intervallo.Inizio < b.Intervallo.Inizio
	case a.Intervallo.Fine != b.Intervallo.Fine:
		return a.Intervallo.Fine < b.Intervallo.Fine
	case a.Famiglia != b.Famiglia:
		return a.Famiglia < b.Famiglia
	}
	return a.Forma < b.Forma
}

// leggiRevisioniCampo: le revisioni in campo separato su un selettore, applicate a tutto il testo del campo
// (D-07), in ordine di (famiglia, regola). In A1a le usa solo la verifica degli esempi.
func (m *Motore) leggiRevisioniCampo(sel evidenze.Selettore, testo string) []revisioneDiFamiglia {
	var out []revisioneDiFamiglia
	for _, p := range m.revisioniCampo[sel] {
		if r := p.leggi(testo); r != nil {
			out = append(out, revisioneDiFamiglia{famiglia: p.famiglia, rev: r})
		}
	}
	return out
}

// revisioneDiFamiglia: una revisione in campo separato letta, con la famiglia della sua regola.
type revisioneDiFamiglia struct {
	famiglia string
	rev      *RevisioneLetta
}

func conErrori(d []evidenze.Diagnostica) bool {
	for _, x := range d {
		if x.Gravita == evidenze.GravitaErrore {
			return true
		}
	}
	return false
}

// ---- l'abbassamento della grammatica nel motore ----

// compilazione: lo stato di CompilaVerificato mentre costruisce il motore. Le mappe servono solo a cercare,
// mai a scorrere: ogni elenco segue l'ordine della grammatica normalizzata.
type compilazione struct {
	g          grammatica.Grammatica
	m          *Motore
	abbassate  map[string]bool // famiglia/forma entrate nel motore
	dichiarate map[string]bool // famiglia|selettore con una forma o una revisione in campo separato dichiarata, attiva o no
	attive     map[string]bool // famiglia|selettore con un piano o una revisione in campo separato nel motore
	campo      map[string]bool // famiglia|selettore con una revisione in campo separato nel motore
}

func chiaveForma(famiglia, forma string) string { return famiglia + "/" + forma }
func percorsoFamiglia(famiglia string) string   { return "famiglie_codice[" + famiglia + "]" }

// chiaveSelettore: famiglia e selettore, con il selettore nella forma canonica di evidenze se si legge (come
// le chiavi dei piani), altrimenti com'è scritto. Così dichiarate, attive e campo usano una sola forma.
func chiaveSelettore(famiglia, s string) string {
	if sel, err := evidenze.LeggiSelettore(s); err == nil {
		s = sel.String()
	}
	return famiglia + "|" + s
}
func percorsoForma(famiglia, forma string) string {
	return percorsoFamiglia(famiglia) + ".forme[" + forma + "]"
}
func percorsoRevisione(famiglia, id string) string {
	return percorsoFamiglia(famiglia) + ".revisioni[" + id + "]"
}

// abbassa compila le forme e le revisioni in campo separato attive. Una forma entra nel motore se è attiva,
// ha almeno un selettore attivo, non è una proiezione riservata e non usa una decorazione riservata o una
// revisione riservata: un elemento riservato non entra, il resto sì (R20 c). Entra solo sui suoi selettori
// attivi.
func (c *compilazione) abbassa() []evidenze.Diagnostica {
	var diag []evidenze.Diagnostica
	for _, f := range c.g.Famiglie {
		r := regoleFamiglia{f: f, revisioni: map[string]*regolaRevisione{}}
		riservate := map[string]bool{} // revisioni riservate, per ID
		for _, rv := range f.Revisioni {
			if rv.Stato != grammatica.StatoAttiva {
				riservate[rv.ID] = true
				if rv.Sorgente == grammatica.SorgenteCampoSeparato {
					for _, s := range rv.Selettori {
						c.dichiarate[chiaveSelettore(f.ID, s)] = true
					}
				}
				continue
			}
			rr, err := abbassaRevisione(rv)
			if err != nil {
				diag = append(diag, nonCompila(percorsoRevisione(f.ID, rv.ID), rv.ID, err))
				continue
			}
			r.revisioni[rv.ID] = rr
			if rv.Sorgente != grammatica.SorgenteCampoSeparato {
				continue
			}
			for _, s := range rv.Selettori {
				c.dichiarate[chiaveSelettore(f.ID, s)] = true
			}
			pr, err := compilaRevisioneCampo(f.ID, rr)
			if err != nil {
				diag = append(diag, nonCompila(percorsoRevisione(f.ID, rv.ID), rv.ID, err))
				continue
			}
			for _, s := range rv.Selettori {
				if !grammatica.SelettoreAttivo(s) {
					continue
				}
				sel, err := evidenze.LeggiSelettore(s)
				if err != nil {
					continue
				}
				c.m.revisioniCampo[sel] = append(c.m.revisioniCampo[sel], pr)
				c.attive[chiaveSelettore(f.ID, s)] = true
				c.campo[chiaveSelettore(f.ID, s)] = true
			}
		}

		for _, fo := range f.Forme {
			for _, s := range fo.Selettori {
				c.dichiarate[chiaveSelettore(f.ID, s)] = true
			}
			if fo.Stato != grammatica.StatoAttiva || !grammatica.ProiezioneAttiva(fo, f.Base) {
				continue // la diagnosi l'ha già data Valida
			}
			if d, ok := c.partiAttive(r, riservate, fo); !ok {
				diag = append(diag, d)
				continue
			}
			var selettori []evidenze.Selettore
			for _, s := range fo.Selettori {
				if !grammatica.SelettoreAttivo(s) {
					continue
				}
				if sel, err := evidenze.LeggiSelettore(s); err == nil {
					selettori = append(selettori, sel)
				}
			}
			if len(selettori) == 0 {
				continue // nessun selettore attivo: lo dice Valida
			}
			p, err := compilaForma(r, fo)
			if err != nil {
				diag = append(diag, nonCompila(percorsoForma(f.ID, fo.ID), fo.ID, err))
				continue
			}
			c.abbassate[chiaveForma(f.ID, fo.ID)] = true
			for _, sel := range selettori {
				c.m.piani[sel] = append(c.m.piani[sel], p)
				c.attive[chiaveSelettore(f.ID, sel.String())] = true
			}
		}
	}
	// I piani di ogni selettore sono già in ordine di (famiglia, forma): la grammatica normalizzata ha famiglie
	// e forme in ordine di ID, e qui si aggiungono in quell'ordine.
	return diag
}

// partiAttive: una forma attiva che usa una decorazione riservata in A1 o una revisione riservata non entra
// nel motore, perché senza quella parte leggerebbe un'altra cosa; i suoi esempi restano non verificati. La
// diagnostica dice quale parte la tiene fuori.
func (c *compilazione) partiAttive(r regoleFamiglia, riservate map[string]bool, fo grammatica.FormaCodice) (evidenze.Diagnostica, bool) {
	for _, pt := range fo.Parti {
		switch pt.Tipo {
		case grammatica.TipoParteDecorazione:
			if d := r.decorazione(pt.Rif); d != nil && !grammatica.DecorazioneAttiva(d.Tipo) {
				return evidenze.Diagnostica{
					Codice:    grammatica.CodiceCapacitaNonSupportata,
					Gravita:   evidenze.GravitaAvviso,
					Natura:    evidenze.NaturaCapacita,
					Percorso:  percorsoForma(r.f.ID, fo.ID),
					Messaggio: fmt.Sprintf("la forma usa la decorazione %q, di un tipo riservato in A1: non entra nel motore, i suoi esempi non si verificano; il resto della grammatica sì (R20 c)", d.ID),
					Rif:       []string{fo.ID, d.ID},
				}, false
			}
		case grammatica.TipoParteRevisione:
			if riservate[pt.Rif] {
				return evidenze.Diagnostica{
					Codice:    grammatica.CodiceFormaRiservata,
					Gravita:   evidenze.GravitaNota,
					Natura:    evidenze.NaturaCapacita,
					Percorso:  percorsoForma(r.f.ID, fo.ID),
					Messaggio: fmt.Sprintf("la forma usa la revisione riservata %q: non attiva, i suoi esempi non si verificano", pt.Rif),
					Rif:       []string{fo.ID, pt.Rif},
				}, false
			}
		}
	}
	// LetturaForma ha un posto solo per l'etichetta, il marcatore, il token e la revisione (par.3.3.4). Una forma
	// che ne dichiara due dello stesso tipo, contando la sequenza interna di un suffisso, ne perderebbe uno in
	// silenzio, e l'originale non si perde mai: non entra nel motore, con la diagnosi, e il resto sì (R20 c).
	// Valida non lo vieta; nessuna grammatica prevista lo fa.
	conti := map[string]int{}
	conta := func(parti []grammatica.Parte) {
		for _, pt := range parti {
			switch pt.Tipo {
			case grammatica.TipoParteEtichetta, grammatica.TipoParteMarcatore, grammatica.TipoParteToken,
				grammatica.TipoParteRevisione:
				conti[pt.Tipo]++
			}
		}
	}
	conta(fo.Parti)
	for _, pt := range fo.Parti {
		if pt.Tipo == grammatica.TipoParteDecorazione {
			if d := r.decorazione(pt.Rif); d != nil {
				conta(d.Parti)
			}
		}
	}
	for _, tipo := range []string{grammatica.TipoParteEtichetta, grammatica.TipoParteMarcatore,
		grammatica.TipoParteToken, grammatica.TipoParteRevisione} {
		if conti[tipo] > 1 {
			return evidenze.Diagnostica{
				Codice:    grammatica.CodiceCapacitaNonSupportata,
				Gravita:   evidenze.GravitaAvviso,
				Natura:    evidenze.NaturaCapacita,
				Percorso:  percorsoForma(r.f.ID, fo.ID),
				Messaggio: fmt.Sprintf("la forma dichiara %d parti %q: la lettura ne conserva una sola, quindi non entra nel motore e i suoi esempi non si verificano; il resto della grammatica sì (R20 c)", conti[tipo], tipo),
				Rif:       []string{fo.ID},
			}, false
		}
	}
	return evidenze.Diagnostica{}, true
}

// nonCompila: una regex generata che non compila. Dopo Valida succede solo in casi limite (per esempio uno
// spazi_max oltre la ripetizione massima di RE2); se succede, la grammatica non si attiva.
func nonCompila(percorso, rif string, err error) evidenze.Diagnostica {
	return evidenze.Diagnostica{
		Codice:    grammatica.CodicePatternNonCompila,
		Gravita:   evidenze.GravitaErrore,
		Natura:    evidenze.NaturaContratto,
		Percorso:  percorso,
		Messaggio: "la regex generata dal compilatore non compila: " + err.Error(),
		Rif:       []string{rif},
	}
}
