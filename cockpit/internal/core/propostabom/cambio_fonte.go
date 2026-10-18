package propostabom

import (
	"encoding/json"
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// L'effetto di un cambio della fonte su una bozza (V1: emendamento di V1 §6.6, algoritmo cambio-fonte-1, passi 1-7, e
// «L'anteprima dell'effetto»; §10.9 S6, effetto_sulla_bozza; OV-24, OT-04, OT-07, OQ-03). È l'algoritmo in sola
// lettura: dice che cosa il cambio farebbe, con gli elenchi della risposta di R2 (cambia_sorgente) e gli stessi nomi, e
// i tre elenchi della sola anteprima (nodi_che_restano, nodi_che_entrano, preassociazioni_nel_vassoio). Non costruisce
// lo snapshot nuovo: lo scrive R2, che può chiamare questa funzione per gli elenchi, così l'anteprima e la risposta danno
// gli stessi elenchi per costruzione. La bozza arriva come VistaBozza, i campi del formato 3 che servono (il DTO della
// bozza è di distintabozza, che importa questo pacchetto, mai il contrario).
//
// Limite dichiarato: P-6 (i confermati senza STEP del gruppo «bom», che vengono dalla working al momento della
// scrittura) non è qui: lo aggiunge chi conosce la working.

// I motivi di da_rivedere[] (EM §1.4, §6.5, §6.6): uno per elemento; un nodo può comparire con più motivi.
const (
	MotivoRivederePropostaCambiata   = "proposta_cambiata"
	MotivoRivedereDecisioneEsistente = "decisione_esistente"
	MotivoRivedereLetturaAssente     = "lettura_assente"
	MotivoRivedereLetturaCambiata    = "lettura_cambiata"
	MotivoRivedereAltraOccorrenza    = "altra_occorrenza"
	MotivoRivedereFonteCambiata      = "fonte_cambiata"
)

// I modi dell'identità decisa di un nodo formato 3 (EM §3.4.2).
const (
	ModoProposta   = "proposta"
	ModoDecisione  = "decisione"
	ModoRegistrato = "registrato"
	ModoManuale    = "manuale"
	ModoDaDecidere = "da_decidere"
	ModoFormato2   = "formato_2"
)

// L'origine «step» di un arco (EM §3.5).
const origineStep = "step"

// VistaBozza: della bozza formato 3, quello che serve a cambio-fonte-1.
//   - Sorgente: occorrenza, sha256, gruppo e radice della sorgente (la radice è fonte_radice del registro della
//     versione o la casella del nodo del prodotto; "" per una bozza venuta dal formato 2 senza ripiego, EM §6.4.1).
//   - NodoProdotto: la chiave_bozza del nodo del prodotto (P-2).
//   - Nodi, Archi, Decisioni, Preassociazioni: quelli dello snapshot, con i soli campi che servono.
type VistaBozza struct {
	Sorgente        SorgenteBozza
	NodoProdotto    string
	Nodi            []NodoBozza
	Archi           []ArcoBozza
	Decisioni       []DecisioneBozza
	Preassociazioni []PreassociazioneBozza
}

// CasellaBozza: la casella di un nodo STEP, {allegato_id, gruppo, rif}.
type CasellaBozza struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	Gruppo     string    `json:"gruppo"`
	Rif        string    `json:"rif"`
}

// IdentitaBozza: l'identità decisa di un nodo (EM §3.4.2), con i campi che il confronto guarda.
type IdentitaBozza struct {
	Emittente string `json:"emittente,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Codice    string `json:"codice,omitempty"`
	Rev       string `json:"rev,omitempty"`
	Modo      string `json:"modo"`
	Decisione string `json:"decisione,omitempty"`
}

// NodoBozza: un nodo della bozza.
type NodoBozza struct {
	ChiaveBozza string
	Caselle     []CasellaBozza
	Identita    IdentitaBozza
}

// PassoStep: il campo step di un arco con origine «step», {allegato_id, gruppo}.
type PassoStep struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	Gruppo     string    `json:"gruppo"`
}

// ArcoBozza: un arco della bozza, fra due chiave_bozza.
type ArcoBozza struct {
	Padre, Figlio string
	Origine       string
	Step          *PassoStep
}

// DecisioneBozza: una decisione della bozza con i campi che servono: il tipo, i nodi che cita (le chiave_bozza: una
// decisione che cita un nodo che esce cade), la casella di una non_includere, la lettura di una decisione identita
// (chiave_lettura e la copia compatta).
type DecisioneBozza struct {
	ID            string
	Tipo          string
	CitaNodi      []string
	Casella       *CasellaBozza
	ChiaveLettura string
	Lettura       *importazione.LetturaCompatta
}

// PreassociazioneBozza: una preassociazione, {proposta_id, chiave_bozza}.
type PreassociazioneBozza struct {
	PropostaID  uuid.UUID
	ChiaveBozza string
}

// I tipi di decisione che il cambio della fonte guarda.
const (
	TipoDecisioneIdentita     = "identita"
	TipoDecisioneNonIncludere = "non_includere"
)

// EffettoCambioFonte: gli elenchi di cambio-fonte-1 (EM §6.6, §9.2 la risposta di cambia_sorgente; §10.9 S6
// effetto_sulla_bozza). Per lo stesso gruppo della stessa struttura sulla stessa occorrenza c'è solo StessaSorgente
// (OV-05): il gesto è un riallineamento.
type EffettoCambioFonte struct {
	StessaSorgente            bool                 `json:"stessa_sorgente,omitempty"`
	NodiCheRestano            []string             `json:"nodi_che_restano,omitempty"`
	NodiCheEntrano            []string             `json:"nodi_che_entrano,omitempty"`
	FuoriPerCambioFonte       []string             `json:"fuori_per_cambio_fonte,omitempty"`
	NuoviNodiProposti         []string             `json:"nuovi_nodi_proposti,omitempty"`
	CaselleRiscritte          []CasellaRiscritta   `json:"caselle_riscritte,omitempty"`
	ArchiRiscritti            []ArcoRiscritto      `json:"archi_riscritti,omitempty"`
	IdentitaAggiornate        []IdentitaAggiornata `json:"identita_aggiornate,omitempty"`
	DaRivedere                []DaRivedere         `json:"da_rivedere,omitempty"`
	DecisioniCadute           []string             `json:"decisioni_cadute,omitempty"`
	Differenze                Differenze           `json:"differenze"`
	PreassociazioniNelVassoio []string             `json:"preassociazioni_nel_vassoio,omitempty"`
}

// CasellaRiscritta: una casella che passa a un'altra occorrenza (chiave_bozza o decisione, allegato prima → dopo).
type CasellaRiscritta struct {
	ChiaveBozza string    `json:"chiave_bozza,omitempty"`
	DecisioneID string    `json:"decisione_id,omitempty"`
	Rif         string    `json:"rif"`
	Prima       uuid.UUID `json:"allegato_prima"`
	Dopo        uuid.UUID `json:"allegato_dopo"`
}

// ArcoRiscritto: un arco «step» che resta, con step prima → dopo.
type ArcoRiscritto struct {
	Padre  string    `json:"padre"`
	Figlio string    `json:"figlio"`
	Prima  PassoStep `json:"step_prima"`
	Dopo   PassoStep `json:"step_dopo"`
}

// IdentitaAggiornata: un nodo «proposta» la cui identità cambia con la proposta nuova (prima → dopo).
type IdentitaAggiornata struct {
	ChiaveBozza string        `json:"chiave_bozza"`
	Prima       IdentitaBozza `json:"prima"`
	Dopo        IdentitaBozza `json:"dopo"`
}

// DaRivedere: un nodo o una decisione da rivedere, con un motivo.
type DaRivedere struct {
	ChiaveBozza string `json:"chiave_bozza,omitempty"`
	DecisioneID string `json:"decisione_id,omitempty"`
	Motivo      string `json:"motivo"`
}

// EffettoDelCambioFonte: l'effetto di cambio-fonte-1 sulla bozza, verso il registro nuovo (il gruppo dichiarato). vecchio
// è il registro della versione della bozza (nil senza registro): serve a dire se una decisione_esistente della voce è
// nuova. È puro e deterministico, e non cambia gli ingressi.
func EffettoDelCambioFonte(b VistaBozza, vecchio *importazione.Contenuto, nuovo importazione.Contenuto) EffettoCambioFonte {
	var out EffettoCambioFonte
	src := b.Sorgente
	allegatoNuovo, err := uuid.Parse(nuovo.Proposta.Fonte.AllegatoID)
	if err != nil {
		return out
	}
	shaNuovo, radiceNuova, gruppoNuovo := nuovo.Proposta.Fonte.Sha256, nuovo.Gruppo.Radice, nuovo.Gruppo.Chiave
	stessoContenuto := src.Sha256 != "" && src.Sha256 == shaNuovo
	// passo 1: la stessa struttura sulla stessa occorrenza è un riallineamento (OV-05)
	if src.Radice != "" && src.AllegatoID == allegatoNuovo && stessoContenuto && src.Radice == radiceNuova {
		return EffettoCambioFonte{StessaSorgente: true}
	}
	// fra due occorrenze della stessa struttura, o senza la radice verso lo stesso allegato con lo stesso sha256 (la
	// regola prudente, OT-07), nessuna voce entra da sola (passo 6)
	dueOccorrenze := (src.Radice != "" && stessoContenuto && src.Radice == radiceNuova && src.AllegatoID != allegatoNuovo) ||
		(src.Radice == "" && src.AllegatoID == allegatoNuovo && stessoContenuto)

	nuove := map[string]importazione.Voce{} // per rif, sull'occorrenza nuova
	for _, v := range nuovo.Proposta.Nodi {
		nuove[v.Origine.Rif] = v
	}
	fuoriNuovi := map[string]bool{}
	for _, x := range nuovo.Proposta.FuoriPerDecisione {
		if _, rif, ok := importazione.PartiDellaVoce(x.Voce); ok {
			fuoriNuovi[rif] = true
		}
	}
	nelNuovo := func(rif string) bool { _, ok := nuove[rif]; return ok || fuoriNuovi[rif] }
	vecchie := map[string]importazione.Voce{}
	if vecchio != nil {
		for _, v := range vecchio.Proposta.Nodi {
			vecchie[v.Origine.Rif] = v
		}
	}

	rivedi := map[DaRivedere]bool{}
	escono := map[string]bool{}
	casellaNuova := map[string]string{} // chiave_bozza → rif sull'occorrenza nuova (i nodi STEP che restano)
	occorrenzaCambiata := map[string]bool{}
	// passi 2 e 3: i nodi
	for _, n := range b.Nodi {
		if n.ChiaveBozza == b.NodoProdotto {
			if c := casellaDellaFonte(n, src); c != nil && c.AllegatoID != allegatoNuovo {
				out.CaselleRiscritte = append(out.CaselleRiscritte, CasellaRiscritta{ChiaveBozza: n.ChiaveBozza, Rif: radiceNuova, Prima: c.AllegatoID, Dopo: allegatoNuovo})
			}
			casellaNuova[n.ChiaveBozza] = radiceNuova
			if n.Identita.Modo == ModoDecisione {
				altra := src.AllegatoID != allegatoNuovo && stessoContenuto && src.Radice == radiceNuova
				occorrenzaCambiata[n.ChiaveBozza] = altra
				motivo := MotivoRivedereFonteCambiata
				if altra {
					motivo = MotivoRivedereAltraOccorrenza
				}
				rivedi[DaRivedere{ChiaveBozza: n.ChiaveBozza, Motivo: motivo}] = true
			}
			out.NodiCheRestano = append(out.NodiCheRestano, n.ChiaveBozza)
			continue
		}
		c := casellaDellaFonte(n, src)
		if c == nil {
			out.NodiCheRestano = append(out.NodiCheRestano, n.ChiaveBozza)
			continue
		}
		if !nelNuovo(c.Rif) {
			escono[n.ChiaveBozza] = true
			out.FuoriPerCambioFonte = append(out.FuoriPerCambioFonte, n.ChiaveBozza)
			continue
		}
		out.NodiCheRestano = append(out.NodiCheRestano, n.ChiaveBozza)
		casellaNuova[n.ChiaveBozza] = c.Rif
		altra := c.AllegatoID != allegatoNuovo
		occorrenzaCambiata[n.ChiaveBozza] = altra
		if altra {
			out.CaselleRiscritte = append(out.CaselleRiscritte, CasellaRiscritta{ChiaveBozza: n.ChiaveBozza, Rif: c.Rif, Prima: c.AllegatoID, Dopo: allegatoNuovo})
		}
		switch n.Identita.Modo {
		case ModoProposta, ModoDaDecidere:
			v, ok := nuove[c.Rif]
			if !ok {
				break // la voce nuova è fuori per decisione: l'identità resta com'era
			}
			if dopo := identitaDellaVoce(v); !stessaIdentita(n.Identita, dopo) {
				out.IdentitaAggiornate = append(out.IdentitaAggiornate, IdentitaAggiornata{ChiaveBozza: n.ChiaveBozza, Prima: n.Identita, Dopo: dopo})
			}
			if len(v.DecisioneEsistente) > 0 && !conDecisioneEsistente(vecchie, c.Rif) {
				rivedi[DaRivedere{ChiaveBozza: n.ChiaveBozza, Motivo: MotivoRivedereDecisioneEsistente}] = true
			}
		case ModoDecisione, ModoFormato2:
			if altra {
				rivedi[DaRivedere{ChiaveBozza: n.ChiaveBozza, Motivo: MotivoRivedereAltraOccorrenza}] = true
			}
		}
	}

	// passi 4 e 5: le preassociazioni dei nodi che escono vanno nel vassoio; gli archi «step» che restano si riscrivono
	for _, p := range b.Preassociazioni {
		if escono[p.ChiaveBozza] {
			out.PreassociazioniNelVassoio = append(out.PreassociazioniNelVassoio, p.PropostaID.String())
		}
	}
	nellaBozza := map[[2]string]bool{}
	for _, a := range b.Archi {
		if escono[a.Padre] || escono[a.Figlio] {
			continue
		}
		if rp, rf := casellaNuova[a.Padre], casellaNuova[a.Figlio]; rp != "" && rf != "" {
			nellaBozza[[2]string{rp, rf}] = true
		}
		if a.Origine != origineStep || a.Step == nil {
			continue
		}
		dopo := PassoStep{AllegatoID: allegatoNuovo, Gruppo: gruppoNuovo}
		if *a.Step != dopo {
			out.ArchiRiscritti = append(out.ArchiRiscritti, ArcoRiscritto{Padre: a.Padre, Figlio: a.Figlio, Prima: *a.Step, Dopo: dopo})
		}
	}

	// passo 6: le voci nuove non rappresentate entrano come nella partenza «step», salvo fra due occorrenze della stessa
	// struttura (e con la regola prudente), dove si elencano fra i nodi proposti; un arco del registro fra due nodi che
	// restano, che la bozza non ha, non entra e si elenca fra le differenze
	rappresentate := map[string]bool{}
	for _, rif := range casellaNuova {
		rappresentate[rif] = true
	}
	// una non_includere che resta rappresenta il suo nodo (EM §7.1: «lo rappresenta solo la decisione»): la sua voce non
	// rientra da sola
	for _, d := range b.Decisioni {
		if c := d.Casella; d.Tipo == TipoDecisioneNonIncludere && c != nil && c.AllegatoID == src.AllegatoID && nelNuovo(c.Rif) {
			rappresentate[c.Rif] = true
		}
	}
	entrano := map[string]bool{}
	for _, v := range nuovo.Proposta.Nodi {
		if rappresentate[v.Origine.Rif] || v.Origine.Rif == radiceNuova {
			continue
		}
		if dueOccorrenze {
			out.NuoviNodiProposti = append(out.NuoviNodiProposti, v.Voce)
			continue
		}
		entrano[v.Origine.Rif] = true
		out.NodiCheEntrano = append(out.NodiCheEntrano, v.Voce)
	}
	for _, a := range nuovo.Proposta.Archi {
		if entrano[a.Padre] || entrano[a.Figlio] || !rappresentate[a.Padre] || !rappresentate[a.Figlio] || nellaBozza[[2]string{a.Padre, a.Figlio}] {
			continue
		}
		out.Differenze.Archi = append(out.Differenze.Archi, DifferenzaArco{AllegatoID: a.AllegatoID, Padre: a.Padre, Figlio: a.Figlio, Tipo: DifferenzaAggiunto})
	}

	// passo 7: le decisioni
	for _, d := range b.Decisioni {
		cade := false
		for _, k := range d.CitaNodi {
			cade = cade || escono[k]
		}
		switch {
		case d.Tipo == TipoDecisioneNonIncludere && d.Casella != nil:
			c := d.Casella
			if c.AllegatoID == src.AllegatoID && !nelNuovo(c.Rif) {
				cade = true
			} else if c.AllegatoID == src.AllegatoID && c.AllegatoID != allegatoNuovo {
				out.CaselleRiscritte = append(out.CaselleRiscritte, CasellaRiscritta{DecisioneID: d.ID, Rif: c.Rif, Prima: c.AllegatoID, Dopo: allegatoNuovo})
				rivedi[DaRivedere{DecisioneID: d.ID, Motivo: MotivoRivedereAltraOccorrenza}] = true
			}
		case d.Tipo == TipoDecisioneIdentita && !cade:
			for _, k := range d.CitaNodi {
				if escono[k] {
					continue
				}
				motivo := MotivoRivedereFonteCambiata
				if occorrenzaCambiata[k] {
					motivo = MotivoRivedereAltraOccorrenza
				}
				rivedi[DaRivedere{ChiaveBozza: k, Motivo: motivo}] = true
				if d.ChiaveLettura != "" {
					if m := motivoDellaLettura(nuovo, d); m != "" {
						rivedi[DaRivedere{ChiaveBozza: k, Motivo: m}] = true
					}
				}
			}
		}
		if cade {
			out.DecisioniCadute = append(out.DecisioniCadute, d.ID)
		}
	}
	for x := range rivedi {
		out.DaRivedere = append(out.DaRivedere, x)
	}
	ordinaEffetto(&out)
	return out
}

// casellaDellaFonte: la casella del nodo sull'occorrenza della sorgente (con il suo gruppo); nil senza.
func casellaDellaFonte(n NodoBozza, src SorgenteBozza) *CasellaBozza {
	for i := range n.Caselle {
		c := &n.Caselle[i]
		if c.AllegatoID == src.AllegatoID && (src.Gruppo == "" || c.Gruppo == src.Gruppo) {
			return c
		}
	}
	return nil
}

// identitaDellaVoce: l'identità che una partenza «step» darebbe al nodo della voce (P-4): «proposta» con il codice
// proposto (emittente e namespace della lettura, rev proposta), «da_decidere» senza.
func identitaDellaVoce(v importazione.Voce) IdentitaBozza {
	var l struct {
		Emittente string `json:"emittente"`
		Namespace string `json:"namespace"`
	}
	if len(v.IdentitaProposta) > 0 {
		_ = json.Unmarshal(v.IdentitaProposta, &l)
	}
	if v.CodiceProposto == "" {
		return IdentitaBozza{Emittente: l.Emittente, Modo: ModoDaDecidere}
	}
	return IdentitaBozza{Emittente: l.Emittente, Namespace: l.Namespace, Codice: v.CodiceProposto, Rev: v.RevProposta, Modo: ModoProposta}
}

// stessaIdentita: modo, codice, rev e namespace uguali, e l'emittente quando le due parti lo dicono.
func stessaIdentita(a, b IdentitaBozza) bool {
	if a.Modo != b.Modo || a.Codice != b.Codice || a.Rev != b.Rev || a.Namespace != b.Namespace {
		return false
	}
	return a.Emittente == "" || b.Emittente == "" || a.Emittente == b.Emittente
}

// conDecisioneEsistente: la voce del registro vecchio con quel rif aveva già una decisione_esistente.
func conDecisioneEsistente(vecchie map[string]importazione.Voce, rif string) bool {
	v, ok := vecchie[rif]
	return ok && len(v.DecisioneEsistente) > 0
}

// motivoDellaLettura: lettura_assente se il registro nuovo non ha la lettura con la chiave della decisione (dovunque
// stia: la voce o le letture dei file), lettura_cambiata se la sua copia compatta è diversa da quella della decisione,
// "" se è la stessa (EM §6.5, §6.6 passo 7).
func motivoDellaLettura(nuovo importazione.Contenuto, d DecisioneBozza) string {
	l, ok := nuovo.TrovaLettura(d.ChiaveLettura)
	if !ok {
		return MotivoRivedereLetturaAssente
	}
	if d.Lettura == nil {
		return ""
	}
	cp, err := l.Compatta()
	if err != nil || !importazione.StessaLettura(cp, *d.Lettura) {
		return MotivoRivedereLetturaCambiata
	}
	return ""
}

// ordinaEffetto: ogni elenco in un ordine che non dipende dagli ingressi.
func ordinaEffetto(e *EffettoCambioFonte) {
	sort.Strings(e.NodiCheRestano)
	sort.Strings(e.NodiCheEntrano)
	sort.Strings(e.FuoriPerCambioFonte)
	sort.Strings(e.NuoviNodiProposti)
	sort.Strings(e.DecisioniCadute)
	sort.Strings(e.PreassociazioniNelVassoio)
	sort.Slice(e.CaselleRiscritte, func(i, j int) bool {
		a, b := e.CaselleRiscritte[i], e.CaselleRiscritte[j]
		if a.ChiaveBozza != b.ChiaveBozza {
			return a.ChiaveBozza < b.ChiaveBozza
		}
		return a.DecisioneID < b.DecisioneID
	})
	sort.Slice(e.ArchiRiscritti, func(i, j int) bool {
		a, b := e.ArchiRiscritti[i], e.ArchiRiscritti[j]
		if a.Padre != b.Padre {
			return a.Padre < b.Padre
		}
		return a.Figlio < b.Figlio
	})
	sort.Slice(e.IdentitaAggiornate, func(i, j int) bool { return e.IdentitaAggiornate[i].ChiaveBozza < e.IdentitaAggiornate[j].ChiaveBozza })
	sort.Slice(e.DaRivedere, func(i, j int) bool {
		a, b := e.DaRivedere[i], e.DaRivedere[j]
		if a.ChiaveBozza != b.ChiaveBozza {
			return a.ChiaveBozza < b.ChiaveBozza
		}
		if a.DecisioneID != b.DecisioneID {
			return a.DecisioneID < b.DecisioneID
		}
		return a.Motivo < b.Motivo
	})
}
