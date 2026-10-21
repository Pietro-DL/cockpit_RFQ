package propostabom

// struttura_step.go: che cosa il motore sa dire della struttura di uno STEP per un prodotto (direttiva 39 §3 [U + T-39]),
// la stessa funzione per l'anteprima S6, per la fonte di /dati e per la Sorgente (S9). Quattro casi, mai confusi:
//   - con_struttura: un gruppo dello STEP per il prodotto si importa e ha figli (partenza step);
//   - sola_radice: il gruppo si importa ma il motore non ha ricostruito figli (partenza step sul vero gruppo); la pagina
//     dice «Il motore non ha estratto figli da questo STEP»: mai mono_pezzo dedotto da qui (EM I.5; D39 §1, il caso dello STEP con zero figli estratti);
//   - nessuna_struttura: l'analisi è finita e nessun gruppo dello STEP per il prodotto si importa (radice non
//     riconosciuta o nessuna candidata): la partenza offerta è solo_prodotto;
//   - non_determinabile: l'analisi è in corso, è fallita (errore o file illeggibile) o lo STEP è fuori dal calcolo: il
//     motivo è lo stato dell'analisi e l'assenza di risultato non è una prova geometrica; la partenza solo_prodotto vuole
//     la presa d'atto esplicita dello stato (presa_d_atto_richiesta).
// Lo stato dell'analisi (StatoAnalisiDelFile) ha gli stessi valori di S8 (sorgente.Analisi*: pronta, in_analisi, errore,
// non_valutato), calcolati con la stessa regola sulla disponibilità dei fatti: la rotta di S8 lo confronta con quello
// mostrato dalla pagina (409 ANTEPRIMA_SUPERATA, anteprima stato_analisi). Funzioni pure.

import (
	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/valutazione"
)

// Gli stati dell'analisi dello STEP (gli stessi valori di sorgente.Analisi* di S8) e i casi della struttura.
const (
	AnalisiPronta      = "pronta"
	AnalisiInCorso     = "in_analisi"
	AnalisiErrore      = "errore"
	AnalisiNonValutato = "non_valutato"

	StrutturaConFigli         = "con_struttura"
	StrutturaSolaRadice       = "sola_radice"
	StrutturaNessuna          = "nessuna_struttura"
	StrutturaNonDeterminabile = "non_determinabile"

	PartenzaStep         = "step"
	PartenzaSoloProdotto = "solo_prodotto"

	TestoStrutturaSolaRadice   = "Il motore non ha estratto figli da questo STEP: la bozza parte dal solo prodotto, senza dedurre un mono-pezzo"
	TestoStrutturaNessuna      = "Il motore non ha trovato una struttura importabile in questo STEP per il prodotto: puoi confermarlo come sorgente e partire dal solo prodotto"
	TestoAnalisiInCorso        = "L'analisi dello STEP è ancora in corso: l'assenza di struttura non è una prova. Puoi aspettare, o procedere dal solo prodotto prendendo atto dello stato"
	TestoAnalisiErrore         = "L'analisi dello STEP non è riuscita: l'assenza di struttura non è una prova. Puoi ricaricare lo STEP, o procedere dal solo prodotto prendendo atto dello stato"
	TestoAnalisiNonValutato    = "Lo STEP non ha un'analisi nel calcolo del motore: l'assenza di struttura non è una prova. Puoi rianalizzarlo, o procedere dal solo prodotto prendendo atto dello stato"
	TestoStrutturaPiuCandidate = "Lo STEP ha più gruppi importabili per il prodotto: scegli quale nell'anteprima"

	// riferimentoFattiAnalisi: il tipo del riferimento ai fatti di un file letto dall'analizzatore (evidenze.RiferimentoFatti)
	riferimentoFattiAnalisi = "analisi_file"
)

// StatoAnalisiDelFile: lo stato dell'analisi di un file nel calcolo, dalla disponibilità dei suoi fatti: pendenti o
// mancanti = in_analisi; errore o illeggibile = errore; un file senza i fatti di un'analisi (il motore ne legge solo il
// nome: disponibilità «parziale» senza riferimento ai fatti) = non_valutato [T-39], perché l'assenza di una struttura
// non è una prova geometrica (direttiva 39 §3); altrimenti (letti) = pronta; il file fuori dal calcolo = non_valutato.
// La rotta di S8 (statoAnalisiS8) usa questa stessa funzione: lo stato che la pagina mostra è quello che S8 ricontrolla.
func StatoAnalisiDelFile(et *valutazione.EsitoThread, allegato uuid.UUID) string {
	if et == nil {
		return AnalisiNonValutato
	}
	for _, f := range et.File {
		if f.AllegatoID != allegato {
			continue
		}
		switch f.Disponibilita {
		case ancoraggio.DisponibilitaPendente, ancoraggio.DisponibilitaMancante:
			return AnalisiInCorso
		case ancoraggio.DisponibilitaErrore, ancoraggio.DisponibilitaIlleggibile:
			return AnalisiErrore
		}
		if f.Documento.Fonte.RiferimentoFatti.Tipo != riferimentoFattiAnalisi {
			return AnalisiNonValutato
		}
		return AnalisiPronta
	}
	return AnalisiNonValutato
}

// StrutturaSTEP: il caso della struttura di uno STEP per un prodotto, con la partenza che la pagina offre per S8. Gruppo
// e ImprontaContenuto sono quelli del gruppo da dichiarare a S8 con la partenza step, quando è uno solo (la candidata in
// vista o l'unico gruppo importabile); con più gruppi importabili e nessuno in vista restano vuoti e la scelta è
// dell'anteprima S6 (gruppi[]). FigliEstratti: i nodi del gruppo che non sono la radice.
type StrutturaSTEP struct {
	StatoAnalisi        string `json:"stato_analisi"`
	Struttura           string `json:"struttura"`
	Gruppo              string `json:"gruppo,omitempty"`
	ImprontaContenuto   string `json:"impronta_contenuto,omitempty"`
	FigliEstratti       int    `json:"figli_estratti"`
	Partenza            string `json:"partenza"`
	PresaDAttoRichiesta bool   `json:"presa_d_atto_richiesta"`
	Testo               string `json:"testo,omitempty"`
}

// GruppoImportato: un gruppo dello STEP per il prodotto, con il registro se l'adattatore l'ha importato (nil se no) e se
// è la candidata in vista.
type GruppoImportato struct {
	Chiave   string
	InVista  bool
	Registro *importazione.Contenuto
}

// ClassificaStruttura: il caso della struttura (commento di testa) dallo stato dell'analisi e dai gruppi dello STEP per il
// prodotto. Con l'analisi non pronta il caso è non_determinabile anche se un gruppo si importa da fatti parziali: la
// pagina non lo presenta come la struttura dello STEP.
func ClassificaStruttura(statoAnalisi string, gruppi []GruppoImportato) StrutturaSTEP {
	out := StrutturaSTEP{StatoAnalisi: statoAnalisi, Struttura: StrutturaNonDeterminabile, Partenza: PartenzaSoloProdotto, PresaDAttoRichiesta: true}
	switch statoAnalisi {
	case AnalisiInCorso:
		out.Testo = TestoAnalisiInCorso
		return out
	case AnalisiErrore:
		out.Testo = TestoAnalisiErrore
		return out
	case AnalisiPronta:
	default:
		out.Testo = TestoAnalisiNonValutato
		return out
	}
	out.PresaDAttoRichiesta = false
	var importabili []GruppoImportato
	for _, g := range gruppi {
		if g.Registro != nil {
			importabili = append(importabili, g)
		}
	}
	if len(importabili) == 0 {
		out.Struttura, out.Testo = StrutturaNessuna, TestoStrutturaNessuna
		return out
	}
	out.Partenza = PartenzaStep
	figli := func(c *importazione.Contenuto) int {
		n := 0
		for _, v := range c.Proposta.Nodi {
			if !c.EVoceDellaRadice(v) {
				n++
			}
		}
		return n
	}
	var scelto *GruppoImportato
	for i := range importabili {
		if importabili[i].InVista {
			scelto = &importabili[i]
		}
	}
	if scelto == nil && len(importabili) == 1 {
		scelto = &importabili[0]
	}
	if scelto != nil {
		out.Gruppo, out.ImprontaContenuto, out.FigliEstratti = scelto.Chiave, scelto.Registro.ImprontaContenuto, figli(scelto.Registro)
		out.Struttura = StrutturaConFigli
		if out.FigliEstratti == 0 {
			out.Struttura, out.Testo = StrutturaSolaRadice, TestoStrutturaSolaRadice
		}
		return out
	}
	// più gruppi importabili, nessuno in vista: la struttura c'è se almeno uno ha figli; il gruppo lo sceglie l'ingegnere
	out.Struttura, out.Testo = StrutturaSolaRadice, TestoStrutturaPiuCandidate
	for _, g := range importabili {
		if n := figli(g.Registro); n > out.FigliEstratti {
			out.FigliEstratti, out.Struttura = n, StrutturaConFigli
		}
	}
	return out
}

// StrutturaDelloSTEPNelModello: il caso della struttura di uno STEP per il prodotto dal modello di /dati (i gruppi del
// prodotto con i registri importati adesso) e dallo stato dell'analisi del file.
func StrutturaDelloSTEPNelModello(et *valutazione.EsitoThread, m *ModelloDelProdotto, allegato uuid.UUID) StrutturaSTEP {
	var gg []GruppoImportato
	if m != nil {
		compatibili := 0
		for _, g := range m.Gruppi {
			if g.AllegatoID != nil && *g.AllegatoID == allegato && g.Tipo == GruppoCandidata && g.Compatibilita == CompatibilitaCompatibile {
				compatibili++
			}
		}
		for _, g := range m.Gruppi {
			if g.AllegatoID == nil || *g.AllegatoID != allegato || g.Tipo == GruppoSenzaSTEP {
				continue
			}
			gi := GruppoImportato{Chiave: g.Chiave, InVista: g.Tipo == GruppoCandidata && g.Compatibilita == CompatibilitaCompatibile && compatibili == 1}
			if c, ok := m.Registri[g.Chiave]; ok {
				c := c
				gi.Registro = &c
			}
			gg = append(gg, gi)
		}
	}
	return ClassificaStruttura(StatoAnalisiDelFile(et, allegato), gg)
}
