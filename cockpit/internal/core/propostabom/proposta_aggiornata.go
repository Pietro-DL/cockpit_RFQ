package propostabom

import (
	"encoding/json"
	"strings"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// La proposta aggiornata di un prodotto confermato (V1, emendamento U2 §5.3; C-04 C [U]): la proposta di adesso del gruppo
// corrente confrontata con il registro della conferma, nelle sei classi di C-04 (identità, revisione, metadati dei
// nodi, quantità, relazioni, gerarchia). Il segno «Proposta aggiornata» nasce solo dal confronto fra proposte (lettura
// [T] di U2 §5.3): le evidenze sono informazione (differenze.evidenze[]), le regole cambiate si dicono a parte.

// PerTipo: quante differenze del contenuto per ognuna delle sei classi di C-04. Una differenza di un nodo con campi di
// più classi conta in ognuna; un nodo aggiunto o tolto e un arco aggiunto o tolto sono gerarchia; un arco cambiato è
// quantità; un campo del gruppo (la radice, la completezza del grafo, la fonte) è gerarchia.
type PerTipo struct {
	Identita  int `json:"identita"`
	Revisione int `json:"revisione"`
	Metadati  int `json:"metadati"`
	Quantita  int `json:"quantita"`
	Relazioni int `json:"relazioni"`
	Gerarchia int `json:"gerarchia"`
}

// classiDeiCampi: la classe di C-04 di ogni campo della voce che entra in impronta_contenuto [T]. Un campo che non sta
// qui (un campo nuovo della foglia) conta fra i metadati: nessuna differenza si perde.
var classiDeiCampi = map[string]string{
	"identita_proposta": "identita", "codice_proposto": "identita", "motivo_proposto": "identita", "codice_composto": "identita",
	"motivo_lettura": "identita", "alternative": "identita", "riferimenti": "identita", "grezzi": "identita",
	"decisione_esistente": "identita", "abbinamento_proposto": "identita", "codice_manuale_legacy": "identita",
	"rev_proposta": "revisione", "motivo_rev": "revisione",
	"tipo_proposto": "metadati", "attributi": "metadati", "segnali": "metadati", "diagnostiche": "metadati", "foglia": "metadati",
	"leggibile": "metadati", "entita_id": "metadati", "radice_del_file": "metadati",
	"relazioni": "relazioni",
	"padri":     "gerarchia",
}

// ClassiDelleDifferenze: le differenze del contenuto contate nelle sei classi di C-04 (PerTipo). Le evidenze non
// contano.
func ClassiDelleDifferenze(d Differenze) PerTipo {
	var p PerTipo
	conta := func(classe string) {
		switch classe {
		case "identita":
			p.Identita++
		case "revisione":
			p.Revisione++
		case "relazioni":
			p.Relazioni++
		case "gerarchia":
			p.Gerarchia++
		case "quantita":
			p.Quantita++
		default:
			p.Metadati++
		}
	}
	for _, n := range d.Nodi {
		if n.Tipo != DifferenzaCambiato {
			conta("gerarchia")
			continue
		}
		viste := map[string]bool{}
		for _, c := range n.Campi {
			classe := classiDeiCampi[c]
			if strings.HasPrefix(c, "stato_motore.") {
				classe = "gerarchia"
			}
			if classe == "" {
				classe = "metadati"
			}
			if !viste[classe] {
				viste[classe] = true
				conta(classe)
			}
		}
	}
	for _, a := range d.Archi {
		if a.Tipo == DifferenzaCambiato {
			conta("quantita")
		} else {
			conta("gerarchia")
		}
	}
	if len(d.Gruppo) > 0 {
		conta("gerarchia")
	}
	return p
}

// PropostaAggiornata: la proposta di adesso del gruppo corrente rispetto al registro della conferma, quando il
// contenuto cambia (U2 §5.3): l'impronta_contenuto di adesso, il gruppo, le differenze (con le evidenze, informative) e le
// classi. nil quando il contenuto è lo stesso: niente segno, anche con le regole o le evidenze cambiate.
type PropostaAggiornata struct {
	ImprontaContenuto string     `json:"impronta_contenuto"`
	Gruppo            string     `json:"gruppo"`
	Differenze        Differenze `json:"differenze"`
	PerTipo           PerTipo    `json:"per_tipo"`
	RegoleCambiate    bool       `json:"regole_cambiate,omitempty"`
}

// PropostaAggiornataDi: il confronto della proposta di adesso (adesso) con il registro della conferma (conferma), sullo
// stesso prodotto; nil se il contenuto non cambia.
func PropostaAggiornataDi(conferma, adesso importazione.Contenuto) *PropostaAggiornata {
	x := Confronta(conferma, adesso)
	if x.Esito != EsitoPropostaCambiata {
		return nil
	}
	return &PropostaAggiornata{ImprontaContenuto: adesso.ImprontaContenuto, Gruppo: adesso.Gruppo.Chiave, Differenze: x.Differenze,
		PerTipo: ClassiDelleDifferenze(x.Differenze), RegoleCambiate: conferma.ImprontaRegole != adesso.ImprontaRegole}
}

// campiDellaConferma: i campi di una voce che la conferma stessa cambia quando crea o lega il componente del pezzo (EM §6.1
// P-3: dopo R5 il motore legge il componente della working come decisione_esistente, e il tipo e l'abbinamento proposti
// che ne seguono).
var campiDellaConferma = map[string]bool{"decisione_esistente": true, "tipo_proposto": true, "abbinamento_proposto": true}

// SenzaGliEffettiDellaConferma: la proposta aggiornata senza le differenze che nascono solo dalla conferma (messaggio 46,
// collaudo UI46, rilievo B2 [T]). Subito dopo «Conferma BOM» i pezzi creati o legati dalla conferma sono componenti della
// working, e la proposta di adesso li dice con decisione_esistente o con abbinamento_proposto (con il tipo che ne segue)
// sulle stesse voci: senza questo, la pagina diceva «Proposta aggiornata: 6 differenze da valutare» per una BOM appena
// confermata identica alla proposta. Una voce cambiata solo in campiDellaConferma, con la decisione_esistente di adesso
// su un componente della mappa della conferma (componenti), o con l'abbinamento proposto ai soli componenti della mappa,
// non è una differenza; le altre restano com'erano. nil quando
// non resta nessuna differenza del contenuto: niente segno.
func SenzaGliEffettiDellaConferma(pa *PropostaAggiornata, adesso importazione.Contenuto, componenti map[string]bool) *PropostaAggiornata {
	if pa == nil || len(componenti) == 0 {
		return pa
	}
	dellaConferma := map[string]bool{}
	for _, v := range adesso.Proposta.Nodi {
		if len(v.DecisioneEsistente) > 0 {
			var de struct {
				ComponenteID string `json:"componente_id"`
			}
			if json.Unmarshal(v.DecisioneEsistente, &de) == nil && componenti[strings.ToLower(de.ComponenteID)] {
				dellaConferma[v.Voce] = true
			}
		}
		// l'abbinamento proposto ai soli componenti della conferma (abbinamento_per_base sui pezzi appena creati)
		if len(v.AbbinamentoProposto) > 0 {
			var ab struct {
				Componenti []string `json:"componenti"`
			}
			if json.Unmarshal(v.AbbinamentoProposto, &ab) == nil && len(ab.Componenti) > 0 {
				tutti := true
				for _, c := range ab.Componenti {
					tutti = tutti && componenti[strings.ToLower(c)]
				}
				if tutti {
					dellaConferma[v.Voce] = true
				}
			}
		}
	}
	out := *pa
	out.Differenze.Nodi = nil
	tolte := false
	for _, n := range pa.Differenze.Nodi {
		if n.Tipo != DifferenzaCambiato || !dellaConferma[n.Voce] {
			out.Differenze.Nodi = append(out.Differenze.Nodi, n)
			continue
		}
		var resto []string
		for _, c := range n.Campi {
			if !campiDellaConferma[c] {
				resto = append(resto, c)
			}
		}
		if len(resto) == len(n.Campi) {
			out.Differenze.Nodi = append(out.Differenze.Nodi, n)
			continue
		}
		tolte = true
		if len(resto) > 0 {
			n.Campi = resto
			out.Differenze.Nodi = append(out.Differenze.Nodi, n)
		}
	}
	if !tolte {
		return pa
	}
	if out.Differenze.Vuote() {
		return nil
	}
	out.PerTipo = ClassiDelleDifferenze(out.Differenze)
	return &out
}

// ComponentiDellaMappa: i componenti della mappa chiave_bozza → componente di una conferma (in minuscolo); vuoto se la
// mappa non si legge.
func ComponentiDellaMappa(mappa []byte) map[string]bool {
	m := map[string]string{}
	if len(mappa) == 0 || json.Unmarshal(mappa, &m) != nil {
		return nil
	}
	out := map[string]bool{}
	for _, c := range m {
		if c != "" {
			out[strings.ToLower(c)] = true
		}
	}
	return out
}
