package propostabom

import (
	"encoding/json"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/valutazione"
)

// Il contenuto di S6 (V1: emendamento di V1 §10.9, S6 «che cosa propone il motore se scelgo questo STEP»; §6.6
// «L'anteprima dell'effetto»; piano della 0023 §14 passo 3), come funzione pura che la rotta S6 usa: lo stesso adattatore
// della partenza «step» (quindi la stessa impronta_contenuto che R2 ricalcolerà), i gruppi dello STEP per il prodotto,
// la proposta del gruppo chiesto o della candidata in vista, le associazioni previste, lo STEP secondario e, con una
// bozza, l'effetto di cambio-fonte-1 con gli elenchi e i nomi di R2. Non scrive niente e non legge il database.
//
// Restano della rotta: l'appartenenza e i 4xx (FUORI_RFQ, NON_STEP, FILE_NON_ATTIVO), il calcolo con i suoi 503/504, lo
// stato del file che l'esito non ha (in_analisi, non_scaricato, dallo stato dei file e delle analisi), lo stato di
// partenza della proposta (P-1…P-8, scritto dalla corsia persistenza accanto a SnapshotDiPartenza) e la lettura della
// bozza e del suo registro.

// Gli stati di S6 che l'esito sa dire (EM §10.9): pronta (lo STEP ha una struttura per il prodotto), errore (il file non si
// legge), struttura_non_determinabile (nessuna struttura per il prodotto, o una struttura letta da un worker non
// aggiornato: LS-01, mai una struttura inventata).
const (
	StatoAnteprimaPronta                     = "pronta"
	StatoAnteprimaErrore                     = "errore"
	StatoAnteprimaStrutturaNonDeterminabile  = "struttura_non_determinabile"
	ConfrontoCodiceCoincide                  = "coincide"
	ConfrontoCodiceDiverso                   = "diverso"
	ConfrontoCodiceNonDeterminabile          = "non_determinabile"
	motivoFileDocumentoNonLeggibileAnteprima = valutazione.MotivoFileDocumentoNonLeggibile
)

// IngressoAnteprima: l'esito già calcolato, le regole di lettura, il prodotto, il codice del suo componente nella
// working (componente.codice: lo legge chi chiama, dalla fotografia o dal database, come lo leggerà R2), lo STEP, il
// gruppo chiesto ("" = la candidata in vista), le occorrenze non attive e, per l'effetto sulla bozza, la vista della
// bozza aperta del prodotto con il registro della sua versione (nil senza).
type IngressoAnteprima struct {
	Esito          *valutazione.Esito
	Thread         *valutazione.EsitoThread
	Regole         valutazione.RegoleDelThread
	Prodotto       string
	CodiceProdotto string
	Allegato       uuid.UUID
	Gruppo         string
	NonAttivi      NonAttivi
	Bozza          *VistaBozza
	RegistroBozza  *importazione.Contenuto
}

// GruppoAnteprima: un gruppo dello STEP per il prodotto (EM §10.9): la chiave, l'identità proposta della radice, la
// compatibilità, se è la candidata in vista, i conti di nodi, archi e segnalati, l'impronta_contenuto; o il rifiuto
// dell'adattatore.
type GruppoAnteprima struct {
	Gruppo            string              `json:"gruppo"`
	Radice            *RadiceAnteprima    `json:"radice,omitempty"`
	Compatibilita     string              `json:"compatibilita"`
	InVista           bool                `json:"in_vista"`
	Nodi              int                 `json:"nodi"`
	Archi             int                 `json:"archi"`
	Segnalati         int                 `json:"segnalati"`
	ImprontaContenuto string              `json:"impronta_contenuto,omitempty"`
	Importazione      *ErroreImportazione `json:"importazione,omitempty"`
}

// RadiceAnteprima: l'identità proposta della radice (la lettura del registro).
type RadiceAnteprima struct {
	IdentitaProposta json.RawMessage `json:"identita_proposta,omitempty"`
}

// PropostaAnteprima: la proposta di un gruppo, con il contenuto del registro (i byte canonici). Lo stato di partenza lo
// aggiunge la rotta.
type PropostaAnteprima struct {
	Gruppo            string          `json:"gruppo"`
	ImprontaContenuto string          `json:"impronta_contenuto"`
	Contenuto         json.RawMessage `json:"contenuto"`
}

// AssociazionePrevista: una proposta d'associazione del motore per una voce della proposta.
type AssociazionePrevista struct {
	Voce       string `json:"voce"`
	AllegatoID string `json:"allegato_id"`
	Motivo     string `json:"motivo,omitempty"`
	Livello    string `json:"livello,omitempty"`
}

// AvvisoAnteprima: un avviso di S6 (il gruppo, il codice, il testo).
type AvvisoAnteprima struct {
	Gruppo string `json:"gruppo,omitempty"`
	Codice string `json:"codice"`
	Testo  string `json:"testo"`
}

// Anteprima: il contenuto di S6 che si calcola dall'esito.
type Anteprima struct {
	AllegatoID        uuid.UUID              `json:"allegato_id"`
	NomeFile          string                 `json:"nome_file"`
	Sha256            string                 `json:"sha256"`
	ProdottoID        string                 `json:"prodotto_id"`
	Stato             string                 `json:"stato"`
	Struttura         StrutturaDelGruppo     `json:"struttura"`
	Gruppi            []GruppoAnteprima      `json:"gruppi"`
	CodiceProdotto    string                 `json:"codice_prodotto"`
	Confronto         string                 `json:"confronto"`
	Proposta          *PropostaAnteprima     `json:"proposta,omitempty"`
	Associazioni      []AssociazionePrevista `json:"associazioni_previste"`
	StepSecondario    *StepSecondario        `json:"step_secondario"`
	EffettoSullaBozza *EffettoCambioFonte    `json:"effetto_sulla_bozza,omitempty"`
	Versioni          json.RawMessage        `json:"versioni,omitempty"`
	Avvisi            []AvvisoAnteprima      `json:"avvisi"`
}

// AnteprimaDelloSTEP: il contenuto di S6 per lo STEP e il prodotto. I gruppi sono le candidate del prodotto su quello
// STEP e, se la BOM di lavoro del prodotto sta su quell'allegato, il gruppo «bom»; in vista c'è solo la candidata
// compatibile, quando è una sola, mai un'incerta. La proposta è quella del gruppo chiesto, altrimenti della candidata in
// vista; il confronto del codice (ConfrontoDelCodice) è fra il codice proposto della radice di quella proposta e il
// codice del componente del prodotto nella working, codice_prodotto (il motivo della scelta della fonte lo dice
// MotivoFonteObbligatorio, la stessa funzione di R2, EM §6.3 passo 3). Con la bozza, l'effetto di cambio-fonte-1 verso
// quel gruppo.
func AnteprimaDelloSTEP(in IngressoAnteprima) Anteprima {
	out := Anteprima{AllegatoID: in.Allegato, Gruppi: []GruppoAnteprima{}, Associazioni: []AssociazionePrevista{}, Avvisi: []AvvisoAnteprima{},
		Confronto: ConfrontoCodiceNonDeterminabile, Stato: StatoAnteprimaStrutturaNonDeterminabile}
	et := in.Thread
	if et == nil || in.Esito == nil {
		return out
	}
	var pv *valutazione.ProdottoValutato
	for i := range et.ProdottiValutati {
		if et.ProdottiValutati[i].Rif == in.Prodotto {
			pv = &et.ProdottiValutati[i]
		}
	}
	var f *valutazione.FileInterpretato
	for i := range et.File {
		if et.File[i].AllegatoID == in.Allegato {
			f = &et.File[i]
		}
	}
	if f != nil {
		out.NomeFile = f.Documento.Fonte.Provenienza.NomeRicevuto
		out.Sha256 = f.Documento.Fonte.RiferimentoFatti.Sha256
		out.Struttura.Versione = f.Documento.Fonte.RiferimentoFatti.SottoversioneSTEP
		if f.Motivo == motivoFileDocumentoNonLeggibileAnteprima {
			out.Stato = StatoAnteprimaErrore
		}
	}
	if pv == nil {
		return out
	}
	if pv.ComponenteID != nil {
		out.ProdottoID = pv.ComponenteID.String()
	}
	out.CodiceProdotto = in.CodiceProdotto
	out.StepSecondario = StepSecondarioDelProdotto(et, *pv, in.NonAttivi)

	var gruppi []Gruppo
	for _, g := range GruppiDelProdotto(*pv, et.Ancoraggi.Strutture) {
		if g.AllegatoID != nil && *g.AllegatoID == in.Allegato && g.Tipo != GruppoSenzaSTEP {
			gruppi = append(gruppi, g)
		}
	}
	if len(gruppi) == 0 {
		return out
	}
	compatibili := 0
	strutture := map[string]*ancoraggio.StrutturaProdotto{}
	for _, g := range gruppi {
		st := strutturaDi(*pv, et.Ancoraggi.Strutture, g)
		strutture[g.Chiave] = st
		if st != nil && g.Tipo == GruppoCandidata && CompatibilitaDellaRadice(st.Compatibilita) == CompatibilitaCompatibile {
			compatibili++
		}
		if st != nil && !st.GrafoCompleto && reGrafoDaRileggere.MatchString(st.MotivoGrafo) {
			out.Struttura.Completa, out.Struttura.Motivo = false, st.MotivoGrafo
			out.Avvisi = append(out.Avvisi, AvvisoAnteprima{Gruppo: g.Chiave, Codice: CodiceImportazioneGrafoDaRileggere, Testo: TestoGrafoDaRileggere})
			return out // LS-01: una struttura letta da un worker non aggiornato non si mostra come una struttura determinata
		}
	}
	registri := map[string]importazione.Registro{}
	scelto := in.Gruppo
	out.Stato = StatoAnteprimaPronta
	for _, g := range gruppi {
		st := strutture[g.Chiave]
		ga := GruppoAnteprima{Gruppo: g.Chiave}
		if st != nil {
			ga.Compatibilita = CompatibilitaDellaRadice(st.Compatibilita)
			ga.InVista = g.Tipo == GruppoCandidata && ga.Compatibilita == CompatibilitaCompatibile && compatibili == 1
			out.Struttura.Completa, out.Struttura.Motivo = st.GrafoCompleto, st.MotivoGrafo
		}
		if ga.InVista && scelto == "" {
			scelto = g.Chiave
		}
		reg, err := importaProtetto(Ingresso{Esito: in.Esito, Thread: et, Regole: in.Regole, Prodotto: in.Prodotto, Gruppo: g.Chiave, NonAttivi: in.NonAttivi})
		if err != nil {
			ga.Importazione = erroreDiImportazione(err)
			out.Avvisi = append(out.Avvisi, AvvisoAnteprima{Gruppo: g.Chiave, Codice: ga.Importazione.Codice + motivoConBarra(ga.Importazione.Motivo), Testo: ga.Importazione.Testo})
		} else {
			registri[g.Chiave] = reg
			c := reg.Contenuto
			ga.ImprontaContenuto, ga.Nodi, ga.Archi = c.ImprontaContenuto, len(c.Proposta.Nodi), len(c.Proposta.Archi)
			for _, v := range c.Proposta.Nodi {
				if len(v.Segnali) > 0 {
					ga.Segnalati++
				}
				if c.EVoceDellaRadice(v) && len(v.IdentitaProposta) > 0 {
					ga.Radice = &RadiceAnteprima{IdentitaProposta: v.IdentitaProposta}
				}
			}
		}
		out.Gruppi = append(out.Gruppi, ga)
	}
	reg, ok := registri[scelto]
	if !ok {
		return out
	}
	c := reg.Contenuto
	out.Proposta = &PropostaAnteprima{Gruppo: scelto, ImprontaContenuto: c.ImprontaContenuto, Contenuto: reg.Canonico}
	out.Versioni = raw(c.Regole.Versioni)
	out.Confronto = ConfrontoDelCodice(c, in.CodiceProdotto)
	for _, v := range c.Proposta.Nodi {
		var aa []struct {
			AllegatoID string `json:"allegato_id"`
			Motivo     string `json:"motivo"`
			Livello    string `json:"livello"`
		}
		if len(v.AssociazioniProposte) == 0 || json.Unmarshal(v.AssociazioniProposte, &aa) != nil {
			continue
		}
		for _, a := range aa {
			out.Associazioni = append(out.Associazioni, AssociazionePrevista{Voce: v.Voce, AllegatoID: a.AllegatoID, Motivo: a.Motivo, Livello: a.Livello})
		}
	}
	if in.Bozza != nil {
		e := EffettoDelCambioFonte(*in.Bozza, in.RegistroBozza, c)
		out.EffettoSullaBozza = &e
	}
	return out
}

func motivoConBarra(m string) string {
	if m == "" {
		return ""
	}
	return "/" + m
}

// ConfrontoDelCodice: il codice proposto della radice del registro contro il codice del componente del prodotto nella
// working (componente.codice), byte per byte: coincide, diverso, oppure non determinabile senza un codice proposto della
// radice o senza il codice del prodotto. È il confronto di S6 (confronto) e quello di R2 per il motivo della scelta della
// fonte (MotivoFonteObbligatorio): una funzione sola, così S6 e R2 non danno risposte diverse.
func ConfrontoDelCodice(c importazione.Contenuto, codiceProdotto string) string {
	for _, v := range c.Proposta.Nodi {
		if !c.EVoceDellaRadice(v) {
			continue
		}
		switch {
		case v.CodiceProposto == "" || codiceProdotto == "":
			return ConfrontoCodiceNonDeterminabile
		case v.CodiceProposto == codiceProdotto:
			return ConfrontoCodiceCoincide
		default:
			return ConfrontoCodiceDiverso
		}
	}
	return ConfrontoCodiceNonDeterminabile
}

// MotivoFonteObbligatorio: la scelta della fonte (crea con partenza «step», cambia_sorgente, la ripartenza) vuole il
// motivo (EM §6.3 passo 3: «se la radice letta dallo STEP ha un codice diverso da quello del prodotto»; 422
// MOTIVO_OBBLIGATORIO) quando il confronto non è «coincide»: con «diverso» per il contratto, e con «non determinabile»
// per prudenza [T], perché senza un codice proposto della radice non si sa che il codice sia lo stesso. S6 lo mostra
// con confronto, R2 lo applica con la stessa funzione.
func MotivoFonteObbligatorio(c importazione.Contenuto, codiceProdotto string) bool {
	return ConfrontoDelCodice(c, codiceProdotto) != ConfrontoCodiceCoincide
}
