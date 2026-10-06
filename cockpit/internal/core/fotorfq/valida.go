package fotorfq

// valida.go: i controlli di contratto della fotografia (piano A, A1c, 6.4.1; contratto di A1c, §3.5). Servono a
// valutazione.Calcola per dire «errore di contratto» invece di calcolare su dati che non tornano. Non correggono
// niente: dicono che cosa non va, con il riferimento.

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// origineAgente: l'origine dei candidati di codice scritti dall'agente LLM, che una fotografia non porta mai
// (MOTORE-SENZA-LLM, par.3.5).
const origineAgente = "agente"

// ValidaFotografia controlla la fotografia e restituisce le diagnostiche, in ordine (codice, percorso):
//   - ogni riferimento si risolve: allegato → messaggio del thread; provenienza di un documento → allegato del
//     thread; proposta → allegato del thread; RigaStepProdotto, RigaFascicolo e DerogaFabbisogno → componente del
//     thread; RigaFascicolo.DocumentoID, Componente.StepStrutturaleID, RigaStepProdotto.StepStrutturaleID,
//     MarcaturaStrutturale.DocumentoID, RimozioneAperta.StepDocumentoID → documento confermato del thread; ogni
//     AggancioMessaggio ha il suo messaggio (§3.5);
//   - nessun CandidatoCodice di origine «agente» (§3.5, 6.4.1);
//   - i fatti sono alla terna della fotografia, e la chiave è il loro sha256;
//   - i tempi sono UTC al millisecondo (6.4.1).
//
// Il triage di fonte diversa da «deterministico» non si può vedere qui: Triage non porta la fonte (6.4.1), e la
// query del caricatore lo esclude (ListTriageThread). I codici sono quelli del contratto: §3.5 assegna
// fotografia.riferimento_non_risolto ai suoi controlli; terna_diversa ai fatti.
func ValidaFotografia(f Fotografia) []evidenze.Diagnostica {
	v := &validatore{analizzatore: f.Analizzatore}
	v.tempo("presa_il", f.PresaIl)
	for _, t := range f.Thread {
		v.thread(t)
	}
	for _, m := range f.FuoriRFQ {
		p := fmt.Sprintf("fuori_rfq[%s]", m.Messaggio.ID)
		v.tempo(p+".messaggio.data_evento", m.Messaggio.DataEvento)
		if m.Aggancio.MessaggioID != m.Messaggio.ID {
			v.riferimento(p+".aggancio.messaggio_id", "l'aggancio non è del suo messaggio", m.Aggancio.MessaggioID)
		}
		v.tempoPtr(p+".aggancio.agganciato_il", m.Aggancio.AgganciatoIl)
		for _, a := range m.Allegati {
			pa := fmt.Sprintf("%s.allegati[%s]", p, a.ID)
			if a.MessaggioID != m.Messaggio.ID {
				v.riferimento(pa+".messaggio_id", "l'allegato non è del suo messaggio", a.MessaggioID)
			}
			v.tempo(pa+".ricevuto_il", a.RicevutoIl)
		}
		v.fatti(p+".fatti", m.Fatti)
	}
	ordinaDiagnostiche(v.out)
	return v.out
}

type validatore struct {
	analizzatore *Terna
	out          []evidenze.Diagnostica
}

// nonRisolto: un errore di contratto con il codice di §3.5.
func (v *validatore) nonRisolto(percorso, messaggio string, rif ...string) {
	v.out = append(v.out, evidenze.Diagnostica{
		Codice: CodiceRiferimentoNonRisolto, Gravita: evidenze.GravitaErrore, Natura: evidenze.NaturaContratto,
		Percorso: percorso, Messaggio: messaggio, Rif: rif,
	})
}

func (v *validatore) riferimento(percorso, messaggio string, id uuid.UUID) {
	v.nonRisolto(percorso, messaggio, id.String())
}

// tempo: UTC e al millisecondo; lo zero passa (vuol dire «non c'è», per esempio PresaIl negli export).
func (v *validatore) tempo(percorso string, t time.Time) {
	if t.IsZero() {
		return
	}
	if t.Location() != time.UTC || t.Nanosecond()%int(time.Millisecond) != 0 {
		v.nonRisolto(percorso, "tempo non UTC al millisecondo: export e DB si confrontano così")
	}
}

func (v *validatore) tempoPtr(percorso string, t *time.Time) {
	if t != nil {
		v.tempo(percorso, *t)
	}
}

func (v *validatore) fatti(percorso string, fatti map[string]Fatti) {
	for sha, x := range fatti {
		p := fmt.Sprintf("%s[%s]", percorso, sha)
		if x.Sha256 != sha {
			v.nonRisolto(p+".sha256", "i fatti stanno sotto lo sha256 di un altro contenuto", sha)
		}
		if v.analizzatore == nil || x.Terna != *v.analizzatore {
			v.out = append(v.out, evidenze.Diagnostica{
				Codice: CodiceTernaDiversa, Gravita: evidenze.GravitaErrore, Natura: evidenze.NaturaContratto,
				Percorso: p + ".terna", Messaggio: "fatti a una terna diversa da quella della fotografia", Rif: []string{sha},
			})
		}
		v.tempo(p+".calcolato_il", x.CalcolatoIl)
	}
}

func (v *validatore) thread(t Thread) {
	pt := fmt.Sprintf("thread[%s]", t.ID)
	v.tempo(pt+".creato_il", t.CreatoIl)

	messaggi := map[uuid.UUID]bool{}
	for _, m := range t.Messaggi {
		messaggi[m.ID] = true
		v.tempo(fmt.Sprintf("%s.messaggi[%s].data_evento", pt, m.ID), m.DataEvento)
	}
	for _, a := range t.Agganci {
		p := fmt.Sprintf("%s.agganci[%s]", pt, a.MessaggioID)
		if !messaggi[a.MessaggioID] {
			v.riferimento(p+".messaggio_id", "l'aggancio non ha il suo messaggio nel thread", a.MessaggioID)
		}
		v.tempoPtr(p+".agganciato_il", a.AgganciatoIl)
	}
	allegati := map[uuid.UUID]bool{}
	for _, a := range t.Allegati {
		allegati[a.ID] = true
		p := fmt.Sprintf("%s.allegati[%s]", pt, a.ID)
		if !messaggi[a.MessaggioID] {
			v.riferimento(p+".messaggio_id", "l'allegato non ha il suo messaggio nel thread", a.MessaggioID)
		}
		v.tempo(p+".ricevuto_il", a.RicevutoIl)
	}
	v.fatti(pt+".fatti", t.Fatti)

	for _, x := range t.Proposte {
		p := fmt.Sprintf("%s.proposte[%s]", pt, x.ID)
		if !allegati[x.AllegatoID] {
			v.riferimento(p+".allegato_id", "la proposta non ha il suo allegato nel thread", x.AllegatoID)
		}
		v.tempoPtr(p+".deciso_il", x.DecisoIl)
	}
	documenti := map[uuid.UUID]bool{}
	for _, d := range t.Documenti {
		documenti[d.ID] = true
	}
	for _, d := range t.Documenti {
		p := fmt.Sprintf("%s.documenti[%s]", pt, d.ID)
		for _, a := range d.Allegati {
			if !allegati[a] {
				v.riferimento(p+".allegati", "la provenienza del documento non è un allegato del thread", a)
			}
		}
		v.tempo(p+".confermato_il", d.ConfermatoIl)
	}
	for _, x := range t.Identificativi {
		v.tempo(fmt.Sprintf("%s.identificativi[%s].creato_il", pt, x.Codice), x.CreatoIl)
	}
	componenti := map[uuid.UUID]bool{}
	for _, c := range t.Componenti {
		componenti[c.ID] = true
		p := fmt.Sprintf("%s.componenti[%s]", pt, c.ID)
		if c.StepStrutturaleID != nil && !documenti[*c.StepStrutturaleID] {
			v.riferimento(p+".step_strutturale_id", "lo STEP strutturale non è un documento confermato del thread", *c.StepStrutturaleID)
		}
		v.tempo(p+".creato_il", c.CreatoIl)
		v.tempoPtr(p+".archiviato_il", c.ArchiviatoIl)
	}
	for _, r := range t.Relazioni {
		v.tempo(fmt.Sprintf("%s.relazioni[%s,%s].creato_il", pt, r.PadreID, r.FiglioID), r.CreatoIl)
	}
	for _, r := range t.RigheComponenteProposta {
		p := fmt.Sprintf("%s.righe_componente_proposta[%s]", pt, r.ID)
		if r.Marcatura != nil && r.Marcatura.DocumentoID != nil && !documenti[*r.Marcatura.DocumentoID] {
			v.riferimento(p+".marcatura.documento_id", "la marcatura strutturale cita un documento che non è del thread", *r.Marcatura.DocumentoID)
		}
		v.tempoPtr(p+".deciso_il", r.DecisoIl)
	}
	for _, r := range t.RigheRelazioneProposta {
		v.tempoPtr(fmt.Sprintf("%s.righe_relazione_proposta[%s,%s,%s].deciso_il", pt, r.AllegatoID, r.PadreChiave, r.FiglioChiave), r.DecisoIl)
	}
	for _, r := range t.RimozioniAperte {
		if !documenti[r.StepDocumentoID] {
			v.riferimento(fmt.Sprintf("%s.rimozioni_aperte[%s,%s].step_documento_id", pt, r.PadreID, r.FiglioID),
				"la rimozione cita uno STEP che non è un documento confermato del thread", r.StepDocumentoID)
		}
	}
	for _, r := range t.StepProdotto {
		p := fmt.Sprintf("%s.step_prodotto[%s]", pt, r.ComponenteID)
		if !componenti[r.ComponenteID] {
			v.riferimento(p+".componente_id", "la riga di v_step_prodotto non ha il suo componente nel thread", r.ComponenteID)
		}
		if r.StepStrutturaleID != nil && !documenti[*r.StepStrutturaleID] {
			v.riferimento(p+".step_strutturale_id", "lo STEP strutturale della vista non è un documento confermato del thread", *r.StepStrutturaleID)
		}
	}
	for _, r := range t.Fascicolo {
		p := fmt.Sprintf("%s.fascicolo[%s,%s]", pt, r.ComponenteID, r.TipoDocumento)
		if !componenti[r.ComponenteID] {
			v.riferimento(p+".componente_id", "la riga di v_fascicolo non ha il suo componente nel thread", r.ComponenteID)
		}
		if r.DocumentoID != nil && !documenti[*r.DocumentoID] {
			v.riferimento(p+".documento_id", "il documento della riga di v_fascicolo non è un documento confermato del thread", *r.DocumentoID)
		}
	}
	for _, d := range t.Deroghe {
		p := fmt.Sprintf("%s.deroghe[%s]", pt, d.ID)
		if !componenti[d.ComponenteID] {
			v.riferimento(p+".componente_id", "la deroga non ha il suo componente nel thread", d.ComponenteID)
		}
		v.tempo(p+".creata_il", d.CreataIl)
	}
	for _, x := range t.Triage {
		p := fmt.Sprintf("%s.triage[%s]", pt, x.ID)
		v.tempo(p+".creato_il", x.CreatoIl)
		v.tempoPtr(p+".deciso_il", x.DecisoIl)
	}
	for _, c := range t.CandidatiCodice {
		if c.Origine == origineAgente {
			v.nonRisolto(fmt.Sprintf("%s.candidati_codice[%s,%s].origine", pt, c.MessaggioID, c.Codice),
				"un candidato di codice dell'agente: la fotografia non porta mai l'agente (MOTORE-SENZA-LLM)", c.MessaggioID.String())
		}
	}
	for _, b := range []*VersioneBOM{t.VersioneBOM, t.UltimaCongelata} {
		if b != nil {
			v.tempoPtr(fmt.Sprintf("%s.versione_bom[%s].congelata_il", pt, b.ID), b.CongelataIl)
		}
	}
}
