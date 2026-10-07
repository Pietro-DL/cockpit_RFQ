package bancoa

import (
	"bytes"
	"path"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/confronto"
	"promatec/cockpit/internal/core/fotorfq"
	valut "promatec/cockpit/internal/core/valutazione"
)

// prodotti.go: la sezione «prodotti» del rapporto (contratto §4, riga 6.4.9; T-B0-16, R69 A; T-B6-05). È solo
// informazione: i sette assi di ogni prodotto valutato, lo stato, il fascicolo, i conflitti. Mai PASS/FAIL, mai una
// voce del gate né una NON ESEGUITA. Con -exports un asse è «non calcolato» dove una sezione della fotografia da cui
// dipende è assente (T-12): mai «assente» per difetto.

// SezioneProdotti: i prodotti valutati di ogni thread.
//   - Calcolata: nessuna sezione della fotografia da cui dipendono gli assi è assente; SezioniAssenti: quelle che lo
//     sono (gli export).
type SezioneProdotti struct {
	Calcolata      bool             `json:"calcolata"`
	SezioniAssenti []string         `json:"sezioni_assenti,omitempty"`
	Thread         []ProdottiThread `json:"thread,omitempty"`
}

// ProdottiThread: i prodotti di un thread, il fascicolo e i conflitti.
type ProdottiThread struct {
	ThreadID     uuid.UUID             `json:"thread_id"`
	ClienteID    uuid.UUID             `json:"cliente_id"`
	Valutato     bool                  `json:"valutato"`
	Prodotti     []ProdottoInformativo `json:"prodotti,omitempty"`
	Fascicolo    FascicoloInformativo  `json:"fascicolo"`
	Conflitti    []ConteggioConflitto  `json:"conflitti,omitempty"`
	Associazioni int                   `json:"associazioni"`
	DaSmistare   int                   `json:"da_smistare"`
}

// ProdottoInformativo: un prodotto valutato, asse per asse. Fonte è il confronto con il risultato atteso dagli attesi,
// solo per i prodotti dello scenario con -attesi (PO-29). R109, precisata dall'utente il 07/10, senza scegliere una
// lettera. Lettura [T]: la regola del runner (la A del testo della domanda) resta valida se la derivazione dagli attesi
// è esplicita e indipendente dal motore; la derivazione arriva in B6b, prima di Q10.
type ProdottoInformativo struct {
	Rif             string            `json:"rif"`
	CodiceRichiesto string            `json:"codice_richiesto"`
	Base            string            `json:"base"`
	Autorita        string            `json:"autorita"`
	Assi            []AsseInformativo `json:"assi"`
	Stato           string            `json:"stato"`
	Verificato      bool              `json:"prodotto_verificato"`
	Motivi          []string          `json:"motivi,omitempty"`
	Impronta        string            `json:"impronta,omitempty"`
	Fonte           *ConfrontoFonte   `json:"fonte_contro_atteso,omitempty"`
}

// AsseInformativo: uno dei sette assi (R79), con lo stato com'è nell'esito, se è calcolato e il motivo.
type AsseInformativo struct {
	Asse      string `json:"asse"`
	Stato     string `json:"stato,omitempty"`
	Calcolato bool   `json:"calcolato"`
	Motivo    string `json:"motivo,omitempty"`
}

// FascicoloInformativo: lo stato del fascicolo del thread, com'è nell'esito.
type FascicoloInformativo struct {
	Calcolato            bool   `json:"calcolato"`
	NumeroTarget         int    `json:"numero_target"`
	NumeroVerificati     int    `json:"numero_verificati"`
	Bloccati             int    `json:"bloccati"`
	Congelabile          bool   `json:"congelabile"`
	MotivoNonCongelabile string `json:"motivo_non_congelabile,omitempty"`
	Orfani               int    `json:"orfani"`
	Legacy               bool   `json:"legacy"`
	FaseThread           string `json:"fase_thread,omitempty"`
}

// ConteggioConflitto: quanti conflitti di un tipo, su un asse.
type ConteggioConflitto struct {
	Tipo string `json:"tipo"`
	Asse string `json:"asse"`
	N    int    `json:"n"`
}

// ConfrontoFonte: per un prodotto dello scenario, la fonte attesa accanto a quella calcolata (PO-29): informazione,
// fuori dal gate. Differenze dice in che cosa non coincidono, senza un esito.
type ConfrontoFonte struct {
	Atteso     RisultatoFonte `json:"atteso"`
	Calcolato  RisultatoFonte `json:"calcolato"`
	Differenze []string       `json:"differenze,omitempty"`
}

// RisultatoFonte: lo stato e il motivo della fonte, gli allegati candidati, se c'è una BOM di lavoro.
type RisultatoFonte struct {
	Stato       string      `json:"stato"`
	Motivo      string      `json:"motivo,omitempty"`
	Candidati   []uuid.UUID `json:"candidati,omitempty"`
	BOMDiLavoro bool        `json:"bom_di_lavoro"`
}

// I sette assi (R79) e, per ognuno, le sezioni della fotografia da cui dipende [T]: se una è assente (gli export),
// l'asse non è calcolato (T-12). Lo stato del prodotto dipende da tutti, il fascicolo anche dalle versioni della BOM.
var assiProdotto = []struct {
	nome    string
	sezioni []string
}{
	{"identita", []string{fotorfq.SezioneIdentificativi, fotorfq.SezioneComponenti}},
	{"fonte", []string{fotorfq.SezioneStepProdotto, fotorfq.SezioneDocumenti, fotorfq.SezioneProvenienze, fotorfq.SezioneComponenti, fotorfq.SezioneFatti}},
	{"nomenclatura", []string{fotorfq.SezioneComponenti, fotorfq.SezioneRigheComponenteProposta}},
	{"gerarchia", []string{fotorfq.SezioneComponenti, fotorfq.SezioneRelazioni, fotorfq.SezioneRigheRelazioneProposta, fotorfq.SezioneRimozioniAperte}},
	{"smistamento", []string{fotorfq.SezioneAllegati, fotorfq.SezioneProposteDocumento, fotorfq.SezioneDocumenti, fotorfq.SezioneProvenienze}},
	{"documenti", []string{fotorfq.SezioneComponenti, fotorfq.SezioneFascicolo, fotorfq.SezioneFabbisogni, fotorfq.SezioneDeroghe, fotorfq.SezioneDocumenti}},
	{"stato", nil}, // tutte quelle sopra
}

// sezioniAssenti: le sezioni della fotografia, fra quelle elencate, che sono assenti, in ordine.
func sezioniAssenti(f fotorfq.Fotografia, sezioni []string) []string {
	var out []string
	for _, k := range sezioni {
		if f.Sezioni[k].Stato == fotorfq.StatoSezioneAssente && !dentro(k, out) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// sezioneProdotti: la sezione informativa dei prodotti, dall'esito di valutazione. scenario e voci servono solo al
// confronto della fonte dei prodotti dello scenario (R109), con gli attesi.
func sezioneProdotti(f fotorfq.Fotografia, e valut.Esito, scenario *ScenarioAtteso, tr *traduzione, ix indiceFoto) *SezioneProdotti {
	var tutte []string
	for _, a := range assiProdotto {
		tutte = append(tutte, a.sezioni...)
	}
	tutte = append(tutte, fotorfq.SezioneVersioneBOM)
	s := &SezioneProdotti{SezioniAssenti: sezioniAssenti(f, tutte)}
	s.Calcolata = len(s.SezioniAssenti) == 0
	for _, et := range e.Thread {
		pt := ProdottiThread{ThreadID: et.ThreadID, ClienteID: et.ClienteID, Valutato: et.Valutato,
			Associazioni: len(et.Associazioni), DaSmistare: len(et.DaSmistare)}
		for _, pv := range et.ProdottiValutati {
			pi := prodottoInformativo(f, pv)
			if scenario != nil && tr != nil && scenario.ThreadID != nil && *scenario.ThreadID == et.ThreadID {
				pi.Fonte = fonteAttesaDelloScenario(pv, tr, ix)
			}
			pt.Prodotti = append(pt.Prodotti, pi)
		}
		fa := et.Fascicolo
		pt.Fascicolo = FascicoloInformativo{Calcolato: fa.Calcolato && len(sezioniAssenti(f, tutte)) == 0,
			NumeroTarget: fa.NumeroTarget, NumeroVerificati: fa.NumeroVerificati, Bloccati: len(fa.Bloccati),
			Congelabile: fa.Congelabile, MotivoNonCongelabile: fa.MotivoNonCongelabile, Orfani: fa.Orfani,
			Legacy: fa.Legacy != nil, FaseThread: fa.FaseThread}
		conteggi := map[[2]string]int{}
		for _, c := range et.Conflitti {
			conteggi[[2]string{string(c.Tipo), string(c.Asse)}]++
		}
		for k, n := range conteggi {
			pt.Conflitti = append(pt.Conflitti, ConteggioConflitto{Tipo: k[0], Asse: k[1], N: n})
		}
		sort.Slice(pt.Conflitti, func(i, j int) bool {
			if pt.Conflitti[i].Tipo != pt.Conflitti[j].Tipo {
				return pt.Conflitti[i].Tipo < pt.Conflitti[j].Tipo
			}
			return pt.Conflitti[i].Asse < pt.Conflitti[j].Asse
		})
		s.Thread = append(s.Thread, pt)
	}
	return s
}

// prodottoInformativo: un prodotto valutato, con i sette assi. Un asse è calcolato se l'esito lo dice (Fonte e
// Smistamento hanno il loro Calcolata) e nessuna sezione da cui dipende è assente.
func prodottoInformativo(f fotorfq.Fotografia, pv valut.ProdottoValutato) ProdottoInformativo {
	pi := ProdottoInformativo{Rif: pv.Rif, CodiceRichiesto: pv.CodiceRichiesto, Base: pv.Base.Normalizzata,
		Autorita: string(pv.Autorita), Stato: string(pv.Stato), Verificato: pv.Verificato, Impronta: pv.Impronta}
	for _, m := range pv.Motivi {
		pi.Motivi = append(pi.Motivi, string(m))
	}
	var tutte []string
	for _, a := range assiProdotto {
		tutte = append(tutte, a.sezioni...)
		sez := a.sezioni
		if a.nome == "stato" {
			sez = tutte
		}
		ax := AsseInformativo{Asse: a.nome, Calcolato: len(sezioniAssenti(f, sez)) == 0}
		switch a.nome {
		case "identita":
			ax.Stato = string(pv.Identita)
		case "fonte":
			ax.Stato, ax.Motivo = string(pv.Fonte.Stato), string(pv.Fonte.Motivo)
			ax.Calcolato = ax.Calcolato && pv.Fonte.Calcolata
		case "nomenclatura":
			ax.Stato, ax.Motivo = string(pv.BOM.Nomenclatura.Stato), pv.BOM.Nomenclatura.Motivo
		case "gerarchia":
			ax.Stato, ax.Motivo = string(pv.BOM.Gerarchia.Stato), pv.BOM.Gerarchia.Motivo
		case "smistamento":
			var motivi []string
			for _, m := range pv.Smistamento.Motivi {
				motivi = append(motivi, string(m))
			}
			ax.Stato, ax.Motivo = string(pv.Smistamento.Stato), strings.Join(motivi, ", ")
			ax.Calcolato = ax.Calcolato && pv.Smistamento.Calcolata
		case "documenti":
			ax.Stato, ax.Motivo = string(pv.Documenti.Stato), pv.Documenti.Motivo
		case "stato":
			ax.Stato = string(pv.Stato)
		}
		switch assenti := sezioniAssenti(f, sez); {
		case len(assenti) > 0:
			ax.Motivo = unisciMotivi(ax.Motivo, "non calcolato: "+strings.Join(assenti, ", ")+" assenti nella fotografia (T-12)")
		case !ax.Calcolato:
			ax.Motivo = unisciMotivi(ax.Motivo, "non calcolato dal motore")
		}
		pi.Assi = append(pi.Assi, ax)
	}
	return pi
}

// fonteAttesaDelloScenario: per un prodotto dello scenario, la fonte attesa ricavata dagli attesi accanto a quella
// calcolata (PO-29: «per ogni prodotto, il risultato atteso, ricavato dagli attesi: i file con esito radice per quel
// target»).
//
// R109, precisata dall'utente il 07/10, senza scegliere una lettera. Lettura [T]: la regola del runner (la A del testo
// della domanda) resta valida se la derivazione dagli attesi è esplicita e indipendente dal motore; la derivazione
// arriva in B6b, prima di Q10. Oggi lo fa il runner con -attesi, solo come informazione, fuori dal gate, e nessun agente
// legge gli attesi. I file attesi radice del prodotto sono le voci dello
// scenario con atteso radice e la base del target uguale alla base del prodotto. Se fra questi c'è uno STEP
// (l'estensione dichiarata, come in valutazione: T-E1-09), la fonte attesa è in_attesa_di_conferma con il motivo
// documento_candidato e quegli STEP come candidati; altrimenti assente, con il motivo che il motore dichiara. Mai
// confermata; nessuna BOM di lavoro. Il limite che B6b toglie: i prodotti scorsi e la chiave del confronto (la base
// letta) vengono ancora dal motore, e il rapporto non porta la derivazione; la chiusura lo dice fra gli informativi
// incompleti (classi.go).
func fonteAttesaDelloScenario(pv valut.ProdottoValutato, tr *traduzione, ix indiceFoto) *ConfrontoFonte {
	base := pv.Base.Normalizzata
	cf := &ConfrontoFonte{Atteso: RisultatoFonte{Stato: string(valut.FonteAssente)}}
	for _, vt := range tr.voci {
		v := vt.voce
		if v.Sezione != confronto.SezioneScenario || !vt.risolta || v.Atteso != confronto.AttesoRadice || v.TargetBase != base {
			continue
		}
		if a, ok := ix.allegati[vt.file.AllegatoID]; ok && eSTEP(a.allegato) {
			cf.Atteso.Candidati = append(cf.Atteso.Candidati, vt.file.AllegatoID)
		}
	}
	if len(cf.Atteso.Candidati) > 0 {
		cf.Atteso.Stato, cf.Atteso.Motivo = string(valut.FonteInAttesaDiConferma), string(valut.MotivoFonteDocumentoCandidato)
	}
	cf.Calcolato = RisultatoFonte{Stato: string(pv.Fonte.Stato), Motivo: string(pv.Fonte.Motivo), BOMDiLavoro: len(pv.Nodi) > 0}
	for _, c := range pv.Fonte.Candidati {
		if c.AllegatoID != nil {
			cf.Calcolato.Candidati = append(cf.Calcolato.Candidati, *c.AllegatoID)
		}
	}
	ordinaUUID(cf.Atteso.Candidati)
	ordinaUUID(cf.Calcolato.Candidati)
	if cf.Atteso.Stato != cf.Calcolato.Stato {
		cf.Differenze = append(cf.Differenze, "stato")
	}
	if cf.Atteso.Motivo != "" && cf.Atteso.Motivo != cf.Calcolato.Motivo {
		cf.Differenze = append(cf.Differenze, "motivo")
	}
	if len(cf.Atteso.Candidati) > 0 && !stessiUUID(cf.Atteso.Candidati, cf.Calcolato.Candidati) {
		cf.Differenze = append(cf.Differenze, "candidati")
	}
	if cf.Calcolato.Stato == string(valut.FonteConfermata) {
		cf.Differenze = append(cf.Differenze, "confermata: mai per lo scenario")
	}
	if cf.Calcolato.BOMDiLavoro {
		cf.Differenze = append(cf.Differenze, "bom_di_lavoro: mai per lo scenario")
	}
	return cf
}

// estensioniSTEP, naturaInline: la regola dello STEP di valutazione (fonte.go: estensioniSTEP, naturaInline,
// estensioneDi), che lì è privata e qui è ricopiata (T-B6-118). La prova di parità legge il sorgente di valutazione e
// cade se le due regole divergono (R-111); in B7 valutazione potrà esportare il predicato, e la copia sparirà.
var estensioniSTEP = map[string]bool{"stp": true, "step": true}

const naturaInline = "inline"

// eSTEP: un allegato STEP come lo riconosce valutazione (T-E1-09: solo lo STEP è fonte): un allegato inline non conta;
// l'estensione è quella dichiarata o, se manca o è vuota, quella del nome, senza maiuscole.
func eSTEP(a fotorfq.Allegato) bool {
	if a.Natura == naturaInline {
		return false
	}
	est := path.Ext(a.NomeFile)
	if a.Estensione != nil && *a.Estensione != "" {
		est = *a.Estensione
	}
	return estensioniSTEP[strings.ToLower(strings.TrimPrefix(est, "."))]
}

func ordinaUUID(u []uuid.UUID) {
	sort.Slice(u, func(i, j int) bool { return bytes.Compare(u[i][:], u[j][:]) < 0 })
}

func stessiUUID(a, b []uuid.UUID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
