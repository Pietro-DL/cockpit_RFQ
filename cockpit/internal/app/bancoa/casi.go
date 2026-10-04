package bancoa

import (
	"fmt"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// Gli esiti di un caso degli attesi (par.4.7.4). Un parziale, un riservato o un rimandato non è mai un passato.
const (
	CasoPassato   = "passato"   // tutte le chiavi controllate in A1a passano, nessuna rimandata
	CasoParziale  = "parziale"  // le chiavi controllate passano, ma alcune sono rimandate ad A1b
	CasoFallito   = "fallito"   // almeno una chiave non passa, o il caso non si può eseguire
	CasoRiservato = "riservato" // stato_atteso riservato, o un predicato su una forma riservata
	CasoRimandato = "rimandato" // nessuna chiave si controlla in A1a
)

// EsitoCaso: come è andato un caso, con ciò che serve a capirlo senza rileggere gli attesi: per ogni chiave
// l'atteso e l'ottenuto, le letture con famiglia e forma, le forme riservate sul selettore del caso, le
// diagnostiche di Riconosci. L'ID e il profilo sono dati privati: stanno solo nei rapporti del banco.
type EsitoCaso struct {
	ID                    string                 `json:"id"`
	Profilo               string                 `json:"profilo"`
	Selettore             string                 `json:"selettore"`
	Esito                 string                 `json:"esito"`
	Motivo                string                 `json:"motivo,omitempty"`
	DipendeDa             string                 `json:"dipende_da,omitempty"` // un caso definito con dipende_da si valuta, e il rapporto lo annota
	Chiavi                []EsitoChiave          `json:"chiavi,omitempty"`
	Letture               []LetturaRapporto      `json:"letture,omitempty"`
	RiservateSulSelettore []string               `json:"riservate_sul_selettore,omitempty"` // «famiglia/forma»
	Diagnostiche          []evidenze.Diagnostica `json:"diagnostiche,omitempty"`
}

// EsitoChiave: una chiave dell'atteso. Stato: passata | fallita | rimandata | riservata.
type EsitoChiave struct {
	Chiave   string `json:"chiave"`
	Sessione string `json:"sessione"` // A1a | A1b | decaduta
	Stato    string `json:"stato"`
	Atteso   string `json:"atteso"`
	Ottenuto string `json:"ottenuto,omitempty"`
	Motivo   string `json:"motivo,omitempty"`
}

// LetturaRapporto: una lettura di forma come la scrive il rapporto: la regola che l'ha prodotta (famiglia e
// forma), che cosa ha letto e dove.
type LetturaRapporto struct {
	Famiglia  string `json:"famiglia"`
	Forma     string `json:"forma"`
	Originale string `json:"originale"`
	Base      string `json:"base"`
	Revisione string `json:"revisione,omitempty"`
	Stato     string `json:"stato"`
	Inizio    int    `json:"inizio"`
	Fine      int    `json:"fine"`
}

// preparato: un caso con il suo selettore, il suo motore e le sue letture, prima di valutarne le chiavi.
type preparato struct {
	caso    CasoContratto
	sel     evidenze.Selettore
	motore  *motorea.Motore
	letture []motorea.LetturaForma
	diag    []evidenze.Diagnostica
	errore  string
}

// EseguiCasiContratto: ogni caso sul motore del suo profilo. In A1a usa Riconosci e controlla solo le chiavi
// definite sulle letture di forma; le altre sono «rimandate ad A1b», con il nome (par.4.7.5). Esiti: passato,
// parziale, fallito, riservato, rimandato. Un parziale, un riservato o un rimandato non è mai un passato.
//
// Il profilo si lega al cliente con i profili del manifest (D-09), poi al motore per UUID: nel codice nessun
// nome di profilo o di cliente. Un profilo senza cliente, un cliente senza motore (scartato o assente
// dall'indice) o un contesto che non si legge fanno fallire il caso, con il motivo. Un caso con stato_atteso
// riservato non si valuta: le sue letture restano nel rapporto come informazione. I casi sull'attributo della
// revisione in campo separato (un selettore «….revisione», o con precondizioni) si valutano in A1b (A-C02,
// A-C09): tutte le loro chiavi sono rimandate. Gli esiti seguono l'ordine dei casi negli attesi.
func EseguiCasiContratto(a Attesi, r motorea.InsiemeRegole, profili map[string]uuid.UUID) []EsitoCaso {
	pp := make([]preparato, 0, len(a.Casi))
	for _, c := range a.Casi {
		pp = append(pp, prepara(c, r, profili))
	}
	out := make([]EsitoCaso, 0, len(pp))
	for i, p := range pp {
		out = append(out, valutaCaso(p, pp, i))
	}
	return out
}

func prepara(c CasoContratto, r motorea.InsiemeRegole, profili map[string]uuid.UUID) preparato {
	p := preparato{caso: c}
	sel, err := TraduciContesto(c.Contesto)
	if err != nil {
		p.errore = fmt.Sprintf("il contesto %q non si legge come selettore (R19)", c.Contesto)
		return p
	}
	p.sel = sel
	cliente, ok := profili[c.Profilo]
	if !ok {
		p.errore = "il profilo non è legato a un cliente nei profili del manifest (D-09)"
		return p
	}
	m := r.Motori[cliente]
	if m == nil {
		p.diag = append(p.diag, r.Scartati[cliente]...)
		if _, scartato := r.Scartati[cliente]; scartato {
			p.errore = "la grammatica del cliente è scartata: il caso non si può eseguire"
		} else {
			p.errore = "il cliente del profilo non ha una voce nell'indice delle regole"
		}
		return p
	}
	p.motore = m
	p.letture, p.diag = m.Riconosci(sel, c.Testo)
	return p
}

func valutaCaso(p preparato, tutti []preparato, i int) EsitoCaso {
	c := p.caso
	e := EsitoCaso{ID: c.ID, Profilo: c.Profilo, Selettore: c.Contesto, DipendeDa: c.DipendeDa, Diagnostiche: p.diag}
	if p.errore == "" {
		e.Selettore = p.sel.String()
		e.Letture = rapportoLetture(p.letture)
		e.RiservateSulSelettore = riservateSul(p.motore, p.sel)
	}
	if p.errore != "" {
		e.Esito, e.Motivo = CasoFallito, p.errore
		for _, k := range c.Atteso {
			e.Chiavi = append(e.Chiavi, EsitoChiave{Chiave: k.Chiave, Sessione: SessioneChiave(k.Chiave), Stato: ChiaveFallita,
				Atteso: k.Valore.String(), Motivo: "caso non eseguito"})
		}
		return e
	}
	if c.StatoAtteso == StatoAttesoRiservato {
		e.Esito, e.Motivo = CasoRiservato, "stato_atteso riservato negli attesi: non si valuta; le letture sono un'informazione"
		for _, k := range c.Atteso {
			e.Chiavi = append(e.Chiavi, EsitoChiave{Chiave: k.Chiave, Sessione: SessioneChiave(k.Chiave), Stato: ChiaveRiservata,
				Atteso: k.Valore.String()})
		}
		return e
	}
	attributo := p.sel.Campo.Valore == "revisione" || c.Precondizioni != nil

	sc := &scena{sel: p.sel, testo: c.Testo, letture: p.letture, motore: p.motore, atteso: map[string]ValoreAtteso{}}
	for _, k := range c.Atteso {
		sc.atteso[k.Chiave] = k.Valore
	}
	for j, q := range tutti {
		if j != i && q.errore == "" && q.caso.Profilo == c.Profilo && q.sel == p.sel {
			sc.formeAltri = append(sc.formeAltri, formeDi(q.letture))
		}
	}

	passate, fallite, rimandate, riservate := 0, 0, 0, 0
	for _, k := range c.Atteso {
		regola := tabellaChiavi[k.Chiave]
		ek := EsitoChiave{Chiave: k.Chiave, Sessione: regola.sessione, Atteso: k.Valore.String()}
		switch {
		case attributo:
			ek.Stato, ek.Motivo = ChiaveRimandata, motivoAttributo
		case regola.valuta == nil:
			ek.Stato, ek.Motivo = ChiaveRimandata, regola.motivo
		default:
			v := regola.valuta(sc, k.Valore)
			ek.Stato, ek.Ottenuto, ek.Motivo = v.stato, v.ottenuto, v.motivo
		}
		switch ek.Stato {
		case ChiavePassata:
			passate++
		case ChiaveFallita:
			fallite++
		case ChiaveRimandata:
			rimandate++
		case ChiaveRiservata:
			riservate++
		}
		e.Chiavi = append(e.Chiavi, ek)
	}
	switch {
	case fallite > 0:
		e.Esito = CasoFallito
	case riservate > 0:
		e.Esito, e.Motivo = CasoRiservato, "un predicato su una forma riservata non si decide"
	case passate == 0:
		e.Esito = CasoRimandato
		if len(c.Atteso) == 0 {
			e.Motivo = "l'atteso non ha chiavi"
		} else {
			e.Motivo = "nessuna chiave si controlla in A1a"
		}
	case rimandate > 0:
		e.Esito, e.Motivo = CasoParziale, "alcune chiavi sono rimandate ad A1b"
	default:
		e.Esito = CasoPassato
	}
	return e
}

func rapportoLetture(ls []motorea.LetturaForma) []LetturaRapporto {
	var out []LetturaRapporto
	for _, l := range ls {
		lr := LetturaRapporto{Famiglia: l.Famiglia, Forma: l.Forma, Originale: l.Originale, Base: l.Base.Normalizzata,
			Stato: l.Stato, Inizio: l.Intervallo.Inizio, Fine: l.Intervallo.Fine}
		if l.Revisione != nil {
			lr.Revisione = l.Revisione.Originale
		}
		out = append(out, lr)
	}
	return out
}

// riservateSul: le forme della grammatica del profilo dichiarate sul selettore che non entrano nel motore
// (riservate, proiezioni riservate, selettore riservato): la «riserva» che può spiegare un caso senza letture.
func riservateSul(m *motorea.Motore, sel evidenze.Selettore) []string {
	if m == nil {
		return nil
	}
	var out []string
	for _, f := range m.Snapshot().Grammatica.Famiglie {
		for _, fo := range f.Forme {
			for _, s := range fo.Selettori {
				if x, err := evidenze.LeggiSelettore(s); err == nil && x == sel && !formaAttiva(fo, f.Base) {
					out = append(out, f.ID+"/"+fo.ID)
					break
				}
			}
		}
	}
	return out
}

// ConteggiCasi: quanti casi per esito. La somma è il numero dei casi letti.
type ConteggiCasi struct {
	Totale    int `json:"totale"`
	Passati   int `json:"passati"`
	Parziali  int `json:"parziali"`
	Falliti   int `json:"falliti"`
	Riservati int `json:"riservati"`
	Rimandati int `json:"rimandati"`
}

// Conta: i conteggi di un elenco di esiti.
func Conta(es []EsitoCaso) ConteggiCasi {
	n := ConteggiCasi{Totale: len(es)}
	for _, e := range es {
		switch e.Esito {
		case CasoPassato:
			n.Passati++
		case CasoParziale:
			n.Parziali++
		case CasoFallito:
			n.Falliti++
		case CasoRiservato:
			n.Riservati++
		case CasoRimandato:
			n.Rimandati++
		}
	}
	return n
}
