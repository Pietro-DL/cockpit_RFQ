package lettura

import "strings"

// IL FALLBACK TAB
//
// Senza HTML (o quando l'HTML non si ancora) una tabella incollata resta visibile nel testo come
// righe con le celle separate da TAB. Si mostra come tabella solo un tratto che lo è senza dubbi:
// almeno due righe di fila, ognuna con almeno due celle non vuote, e lo stesso numero di colonne a
// meno di una (Excel toglie le celle vuote in fondo). Una riga indentata con un TAB, o un elenco
// «voce<TAB>valore» isolato, restano testo. Mai intestazione: dal testo non si sa quale riga lo sia.

// celleTab sono le celle di una riga candidata, senza le vuote in fondo; nil se la riga non lo è.
func celleTab(riga string) []string {
	if !strings.Contains(riga, "\t") {
		return nil
	}
	parti := strings.Split(strings.TrimRight(riga, " \t"), "\t")
	piene := 0
	for i := range parti {
		parti[i] = strings.TrimSpace(perVista(parti[i]))
		if parti[i] != "" {
			piene++
		}
	}
	if piene < 2 {
		return nil
	}
	for len(parti) > 0 && parti[len(parti)-1] == "" {
		parti = parti[:len(parti)-1]
	}
	return parti
}

// tabelleTab cerca i tratti di righe candidate fra le righe ancora libere. Una riga vuota sola in
// mezzo è ammessa se la riga dopo è candidata: alcuni client lasciano una riga fra le righe.
func (s *sezione) tabelleTab() {
	n := len(s.grezze)
	for i := 0; i < n; {
		if !s.libera(i) || celleTab(s.grezze[i]) == nil {
			i++
			continue
		}
		var righe [][]string
		ultima := i
		for j := i; j < n; {
			if s.libera(j) {
				if c := celleTab(s.grezze[j]); c != nil {
					righe = append(righe, c)
					ultima = j
					j++
					continue
				}
				if s.vuota(j) && j+1 < n && s.libera(j+1) && celleTab(s.grezze[j+1]) != nil {
					j++
					continue
				}
			}
			break
		}
		if t := tabellaTab(righe); t != nil {
			s.occupa(regione{tipo: BloccoTabella, da: i, a: ultima + 1, tabella: t})
		}
		i = ultima + 1
	}
}

func tabellaTab(righe [][]string) *Tabella {
	if len(righe) < 2 || len(righe) > maxRigheTabella {
		return nil
	}
	minimo, massimo := len(righe[0]), len(righe[0])
	for _, r := range righe {
		minimo, massimo = min(minimo, len(r)), max(massimo, len(r))
	}
	if massimo > maxColonne || massimo-minimo > 1 {
		return nil
	}
	t := &Tabella{Colonne: massimo, Origine: OrigineTab}
	for _, r := range righe {
		rt := RigaTabella{Celle: make([]Cella, massimo)}
		for k := range rt.Celle {
			c := Cella{Colspan: 1, Rowspan: 1}
			if k < len(r) {
				c.Testo, c.Troncata = tronca(r[k], maxRuneCella)
				c.Numero = numerico(r[k])
			}
			rt.Celle[k] = c
		}
		t.Righe = append(t.Righe, rt)
	}
	return t
}
