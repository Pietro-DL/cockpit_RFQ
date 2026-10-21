package propostabom

import (
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
