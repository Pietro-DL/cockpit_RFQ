package motorea

import (
	"fmt"
	"sort"
	"strings"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// GP-1 (addendum «contesto partner», 16/10). Ogni piano ha una condizione: legge sempre (le forme attive del
// cliente), solo in un contesto (le forme contestuali del cliente), solo per un partner attivo (le forme di un
// emittente dichiarato), oppure solo dentro un legame. La condizione la decide la grammatica; se vale, lo decide
// l'attivazione del documento, che viene dal contesto di lavorazione (contesto.go) o dall'attivazione di un
// esempio. Senza attivazione leggono solo le forme attive del cliente: Riconosci, la verifica degli esempi senza
// attivazione e l'interpretazione senza contesto restano quelle di prima.

// condizione: quando un piano legge.
type condizione int

const (
	condSempre      condizione = iota // forma attiva del cliente (o, nel motore di un emittente da solo, sua)
	condContestuale                   // forma contestuale del cliente: solo con un'attivazione
	condPartner                       // forma di un emittente dichiarato: solo se il partner è attivo
	condSoloLegame                    // solo dentro un legame che copre tutto il testo
)

// attivazione: che cosa vale per un documento (o per un esempio). nil = nessun contesto: leggono solo le forme
// attive del cliente.
type attivazione struct {
	partner   map[string]bool   // gli emittenti attivi e caricati
	codifiche map[string]string // selettore (forma testuale) → cliente | <emittente> | misto; assente = non dichiarato
	modelli   map[string]bool   // i legami ammessi
	autore    string            // l'autore dichiarato del documento; vuoto se non dichiarato
}

// codificaDi: la codifica dichiarata del campo; "" se non dichiarata (vale come misto: alternative).
func (a *attivazione) codificaDi(sel evidenze.Selettore) string {
	if a == nil {
		return ""
	}
	return a.codifiche[sel.String()]
}

// attivo: il piano legge su questo selettore con questa attivazione, nella scansione libera.
//   - sempre: sì;
//   - contestuale: con un'attivazione, se il campo è codificato dal cliente, misto o non dichiarato;
//   - partner: se il suo emittente è attivo, e il campo è codificato da lui, misto o non dichiarato;
//   - solo legame: mai nella scansione libera.
func (p *pianoForma) attivo(sel evidenze.Selettore, a *attivazione) bool {
	switch p.condizione {
	case condSempre:
		return true
	case condContestuale:
		if a == nil {
			return false
		}
		c := a.codificaDi(sel)
		return c == "" || c == grammatica.CodificaCliente || c == grammatica.CodificaMista
	case condPartner:
		if a == nil || !a.partner[p.emittente] {
			return false
		}
		c := a.codificaDi(sel)
		return c == "" || c == p.emittente || c == grammatica.CodificaMista
	}
	return false
}

// attivoNelLegame: il piano può dare un pezzo di un legame. Dentro un legame la codifica del campo non spegne il
// partner: il legame stesso è la prova che quel tratto è suo (addendum §1, «salvo dentro un legame»).
func (p *pianoForma) attivoNelLegame(sel evidenze.Selettore, a *attivazione) bool {
	if a == nil {
		return false
	}
	switch p.condizione {
	case condSempre:
		return true
	case condContestuale:
		return true
	case condPartner, condSoloLegame:
		return p.emittente == "" || a.partner[p.emittente]
	}
	return false
}

// estensioneAmmessa: il filtro facoltativo delle estensioni sulle forme nome_file (maiuscole ASCII indifferenti).
func (p *pianoForma) estensioneAmmessa(sel evidenze.Selettore, testo string) bool {
	if len(p.estensioni) == 0 || sel.Contesto != evidenze.ContestoNomeFile {
		return true
	}
	i := strings.LastIndexByte(testo, '.')
	if i < 0 {
		return false
	}
	ext := testo[i+1:]
	for _, e := range p.estensioni {
		if strings.EqualFold(e, ext) {
			return true
		}
	}
	return false
}

// riconosciCon: come Riconosci, con i piani che l'attivazione accende (GP-1). Con attivazione nil è Riconosci.
func (m *Motore) riconosciCon(sel evidenze.Selettore, testo string, a *attivazione) ([]LetturaForma, []evidenze.Diagnostica) {
	if m == nil {
		return nil, nil
	}
	var piani []*pianoForma
	for _, p := range m.piani[sel] {
		if p.attivo(sel, a) && p.estensioneAmmessa(sel, testo) {
			piani = append(piani, p)
		}
	}
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

// ---- la composizione dei partner nel motore del cliente (CompilaVerificatoConPartner) ----

// PartnerCaricato: la grammatica di un emittente, già compilata e verificata da sola (CompilaInsieme).
type PartnerCaricato struct {
	Snapshot grammatica.SnapshotRegole
}

// partnerCompilato: un partner dichiarato dal cliente ed entrato nel suo motore.
type partnerCompilato struct {
	hash      string
	famiglie  []string
	selettori []string
}

// componiPartner aggiunge al motore del cliente le forme dei partner dichiarati, con la condizione «partner» (o
// «solo legame») e solo sui selettori della dichiarazione. Un partner assente, in deriva, con un ID di famiglia
// o un namespace in comune con il cliente non entra: il cliente resta attivo senza di lui, con la diagnosi
// (avviso). Il contesto che lo attiva lo saprà e lo dirà (contesto.partner_non_caricabile).
func (c *compilazione) componiPartner(caricati map[string]PartnerCaricato) []evidenze.Diagnostica {
	var diag []evidenze.Diagnostica
	avviso := func(p, emittente, msg string) {
		diag = append(diag, evidenze.Diagnostica{
			Codice:    CodicePartnerNonComposto,
			Gravita:   evidenze.GravitaAvviso,
			Natura:    evidenze.NaturaContratto,
			Percorso:  p,
			Messaggio: msg,
			Rif:       []string{emittente},
		})
	}
	famiglieCliente := map[string]bool{}
	namespaceCliente := map[string]bool{}
	for _, f := range c.g.Famiglie {
		famiglieCliente[f.ID] = true
		namespaceCliente[f.Namespace] = true
	}
	namespaceAltri := map[string]string{} // namespace → emittente che l'ha già preso
	famiglieAltri := map[string]string{}  // ID di famiglia → emittente che l'ha già preso
	for _, d := range c.g.Partner {
		p := "partner[" + d.Emittente + "]"
		car, ok := caricati[d.Emittente]
		if !ok {
			avviso(p, d.Emittente, "il partner dichiarato non è caricato (assente dall'indice o non valido): il cliente resta attivo senza di lui")
			continue
		}
		if !strings.EqualFold(car.Snapshot.Hash, d.HashSnapshot) {
			avviso(p+".hash_snapshot", d.Emittente, fmt.Sprintf("deriva: la grammatica caricata dell'emittente ha hash %s, la dichiarazione blocca %s; il partner non entra", car.Snapshot.Hash, d.HashSnapshot))
			continue
		}
		eg := car.Snapshot.Grammatica // già normalizzata
		var famiglie []grammatica.FamigliaCodice
		conflitto := ""
		for _, f := range eg.Famiglie {
			if len(d.Famiglie) > 0 && !contiene(d.Famiglie, f.ID) {
				continue
			}
			switch {
			case famiglieCliente[f.ID]:
				conflitto = fmt.Sprintf("la famiglia %q dell'emittente ha lo stesso ID di una famiglia del cliente", f.ID)
			case namespaceCliente[f.Namespace]:
				conflitto = fmt.Sprintf("il namespace %q della famiglia %q dell'emittente è anche del cliente: chi l'ha emesso non si distinguerebbe", f.Namespace, f.ID)
			case namespaceAltri[f.Namespace] != "" && namespaceAltri[f.Namespace] != d.Emittente:
				conflitto = fmt.Sprintf("il namespace %q è conteso con l'emittente %q", f.Namespace, namespaceAltri[f.Namespace])
			case famiglieAltri[f.ID] != "" && famiglieAltri[f.ID] != d.Emittente:
				conflitto = fmt.Sprintf("la famiglia %q ha lo stesso ID di una famiglia dell'emittente %q: le letture non si distinguerebbero", f.ID, famiglieAltri[f.ID])
			}
			if conflitto != "" {
				break
			}
			famiglie = append(famiglie, f)
		}
		if conflitto != "" {
			avviso(p, d.Emittente, conflitto+": il partner non entra")
			continue
		}
		for _, f := range d.Famiglie {
			trovata := false
			for _, x := range famiglie {
				trovata = trovata || x.ID == f
			}
			if !trovata {
				avviso(p+".famiglie", d.Emittente, fmt.Sprintf("la famiglia %q non è dell'emittente", f))
			}
		}
		pc := partnerCompilato{hash: car.Snapshot.Hash, selettori: append([]string(nil), d.Selettori...)}
		for _, f := range famiglie {
			namespaceAltri[f.Namespace] = d.Emittente
			famiglieAltri[f.ID] = d.Emittente
			pc.famiglie = append(pc.famiglie, f.ID)
			c.m.regole.ruoli[f.ID] = append([]grammatica.Ruolo(nil), f.Ruoli...)
			r := regoleFamiglia{f: f, revisioni: map[string]*regolaRevisione{}, classi: tabellaDi(f.Classi)}
			riservate := map[string]bool{}
			for _, rv := range f.Revisioni {
				if rv.Stato != grammatica.StatoAttiva {
					riservate[rv.ID] = true
					continue
				}
				if rv.Sorgente == grammatica.SorgenteCampoSeparato {
					continue // le revisioni in campo separato di un partner non entrano: il campo è del documento del cliente
				}
				rr, err := abbassaRevisione(rv)
				if err != nil {
					diag = append(diag, nonCompila(percorsoRevisione(f.ID, rv.ID), rv.ID, err))
					continue
				}
				r.revisioni[rv.ID] = rr
			}
			for _, fo := range f.Forme {
				if !grammatica.FormaCompilabile(fo.Stato) || !grammatica.ProiezioneAttiva(fo, f.Base) {
					continue
				}
				if _, ok := c.partiAttive(r, riservate, fo); !ok {
					continue
				}
				var selettori []evidenze.Selettore
				for _, s := range fo.Selettori {
					if !contiene(d.Selettori, s) || !grammatica.SelettoreAttivo(s) {
						continue
					}
					if sel, err := evidenze.LeggiSelettore(s); err == nil {
						selettori = append(selettori, sel)
					}
				}
				if len(selettori) == 0 {
					continue
				}
				pf, err := compilaForma(r, fo)
				if err != nil {
					diag = append(diag, nonCompila(percorsoForma(f.ID, fo.ID), fo.ID, err))
					continue
				}
				pf.emittente, pf.hashEmittente = d.Emittente, car.Snapshot.Hash
				pf.condizione = condPartner
				if fo.Stato == grammatica.StatoSoloLegame {
					pf.condizione = condSoloLegame
				}
				c.abbassate[chiaveForma(f.ID, fo.ID)] = true
				c.m.forme[chiaveForma(f.ID, fo.ID)] = pf
				for _, sel := range selettori {
					c.m.piani[sel] = append(c.m.piani[sel], pf)
					c.attive[chiaveSelettore(f.ID, sel.String())] = true
					c.dichiarate[chiaveSelettore(f.ID, sel.String())] = true
				}
			}
		}
		c.m.partner[d.Emittente] = pc
	}
	// I piani di ogni selettore in ordine di (famiglia, forma), anche con quelli dei partner.
	for sel := range c.m.piani {
		ps := c.m.piani[sel]
		sort.SliceStable(ps, func(i, j int) bool {
			if ps[i].famiglia != ps[j].famiglia {
				return ps[i].famiglia < ps[j].famiglia
			}
			return ps[i].forma < ps[j].forma
		})
	}
	return diag
}

// PartnerAmmessi: gli emittenti dichiarati dal cliente ed entrati nel suo motore, con il loro hash, in ordine di
// ID. Serve al banco e ai rapporti.
func (m *Motore) PartnerAmmessi() map[string]string {
	out := map[string]string{}
	for id, p := range m.partner {
		out[id] = p.hash
	}
	return out
}

// partnerDichiarato: l'emittente è nella dichiarazione del cliente, anche se non è entrato nel motore.
func (m *Motore) partnerDichiarato(id string) (grammatica.DichiarazionePartner, bool) {
	for _, d := range m.snap.Grammatica.Partner {
		if d.Emittente == id {
			return d, true
		}
	}
	return grammatica.DichiarazionePartner{}, false
}

// rangoFunzione: l'ordine delle funzioni per funzione_massima (G-7).
func rangoFunzione(f Funzione) int {
	switch f {
	case FunzMenzione:
		return 0
	case FunzAttributo, FunzRelazione:
		return 1
	case FunzStruttura, FunzIdentitaFile:
		return 2
	case FunzRichiesta:
		return 3
	}
	return 0
}
