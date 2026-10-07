package bancoa

import (
	"bytes"
	"fmt"
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
//   - FontiScenario: con -attesi, la fonte attesa dei prodotti attesi dello scenario accanto a quella calcolata (PO-29,
//     con la regola di R109: fontiDelloScenario).
type SezioneProdotti struct {
	Calcolata      bool                `json:"calcolata"`
	SezioniAssenti []string            `json:"sezioni_assenti,omitempty"`
	Thread         []ProdottiThread    `json:"thread,omitempty"`
	FontiScenario  *FontiDelloScenario `json:"fonti_scenario,omitempty"`
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

// ProdottoInformativo: un prodotto valutato, asse per asse. Il confronto con la fonte attesa non sta qui: si scorrono i
// prodotti attesi, non quelli del motore (R109; SezioneProdotti.FontiScenario).
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

// FontiDelloScenario: PO-29 nel rapporto, con la regola di R109 (fontiDelloScenario). Informazione, fuori dal gate.
//   - ThreadID: il thread dello scenario, come lo dicono gli attesi;
//   - NonVerificabile: perché la corsa non può verificare la fonte calcolata dello scenario (il thread non è fra quelli
//     scelti, o non si valuta per un limite degli ingressi: R-104, T-B6-104); i prodotti attesi ci sono lo stesso, con
//     la loro derivazione, e la parte calcolata è fra le parti non verificate;
//   - Prodotti: un confronto per prodotto atteso, nell'ordine delle basi;
//   - CalcolatiSenzaAtteso: i prodotti calcolati del thread dello scenario che gli attesi non attendono («base (rif)»).
type FontiDelloScenario struct {
	ThreadID             *uuid.UUID       `json:"thread_id,omitempty"`
	NonVerificabile      string           `json:"non_verificabile,omitempty"`
	Prodotti             []ConfrontoFonte `json:"prodotti,omitempty"`
	CalcolatiSenzaAtteso []string         `json:"calcolati_senza_atteso,omitempty"`
}

// ConfrontoFonte: per un prodotto atteso dello scenario, la fonte attesa accanto a quella calcolata (PO-29):
// informazione, fuori dal gate.
//   - Base: la base del prodotto atteso, com'è scritta negli attesi (la chiave del legame con il prodotto calcolato);
//   - Derivazione: le voci degli attesi usate e l'identificativo della regola;
//   - Atteso: la fonte attesa, derivata dagli attesi; Stato vuoto se non si deriva (una voce radice non risolta);
//   - Calcolato: la fonte del prodotto calcolato con la stessa base; nil se non c'è (SenzaCalcolato, una differenza), se
//     più prodotti calcolati hanno quella base (una differenza: il confronto non sceglie) o se la corsa non lo può
//     verificare;
//   - Livelli: presente, estrazione riuscita, struttura, associazione, autorizzazione come fonte, verifica operativa,
//     distinti (R109; E1 §PO-29; R-144 per la struttura);
//   - Differenze: in che cosa non coincidono, senza un esito; NonVerificate: le parti che non si confrontano, con il
//     motivo (la regola delle chiavi accettate: verificata, oppure dichiarata non verificata).
type ConfrontoFonte struct {
	Base           string           `json:"base"`
	Derivazione    DerivazioneFonte `json:"derivazione"`
	Atteso         RisultatoFonte   `json:"atteso"`
	Calcolato      *RisultatoFonte  `json:"calcolato,omitempty"`
	SenzaCalcolato bool             `json:"senza_calcolato"`
	Livelli        []LivelloFonte   `json:"livelli"`
	Differenze     []string         `json:"differenze,omitempty"`
	NonVerificate  []string         `json:"non_verificate,omitempty"`
}

// DerivazioneFonte: come la fonte attesa si deriva dagli attesi, prima del confronto e senza l'uscita del motore (R109:
// «Gli attesi devono essere esplicitati e motivati prima del confronto»).
//   - Regola: l'identificativo della regola (RegolaFonteAttesa);
//   - Prodotti: i percorsi dei prodotti attesi dello scenario con la base;
//   - Voci: i percorsi delle voci dello scenario con atteso radice e la base del target uguale, risolte nella fotografia
//     (solo quelle: le voci figlio e fuori non entrano);
//   - NonRisolte: le voci radice del target che non si risolvono, con il motivo: senza il file non si sa se sono STEP.
type DerivazioneFonte struct {
	Regola     string   `json:"regola"`
	Prodotti   []string `json:"prodotti,omitempty"`
	Voci       []string `json:"voci,omitempty"`
	NonRisolte []string `json:"non_risolte,omitempty"`
}

// LivelloFonte: un livello della fonte di un prodotto atteso, con quello che fissano gli attesi e quello che dice la
// corsa (i fatti della fotografia o l'esito del motore, detto nel testo). Mai un esito.
type LivelloFonte struct {
	Livello   string `json:"livello"`
	Atteso    string `json:"atteso"`
	Calcolato string `json:"calcolato"`
}

// I livelli della fonte, distinti (R109, precisata dall'utente il 07/10: «Mantieni però distinti estrazione riuscita,
// struttura corretta, associazione al prodotto, autorizzazione come fonte e verifica operativa»; E1 §PO-29: file
// presente, analizzato, proposto, confermato mai). La struttura corretta la misura l'asse della gerarchia del prodotto
// calcolato: il livello «struttura» lo riporta, «calcolato, non confrontato» (T-B6-224, precisata dall'orchestratore con
// R-144), perché gli attesi di PO-29 non fissano la gerarchia.
const (
	LivelloPresente       = "presente"
	LivelloEstrazione     = "estrazione_riuscita"
	LivelloStruttura      = "struttura"
	LivelloAssociazione   = "associazione"
	LivelloAutorizzazione = "autorizzazione_come_fonte"
	LivelloVerifica       = "verifica_operativa"
)

// RegolaFonteAttesa: l'identificativo della regola con cui il runner deriva la fonte attesa di un prodotto dello
// scenario dagli attesi (PO-29; R109 [T]; fontiDelloScenario). Cambiare la regola vuol dire cambiare l'identificativo.
const RegolaFonteAttesa = "po29-radici-step-1"

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

// sezioneProdotti: la sezione informativa dei prodotti, dall'esito di valutazione. scenario, tr e nonVerificabile
// servono solo al confronto della fonte dei prodotti attesi dello scenario (PO-29, R109), con gli attesi:
// nonVerificabile è perché la corsa non può verificare lo scenario (scenarioNonVerificabile).
func sezioneProdotti(f fotorfq.Fotografia, e valut.Esito, scenario *ScenarioAtteso, tr *traduzione, ix indiceFoto, nonVerificabile string) *SezioneProdotti {
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
			pt.Prodotti = append(pt.Prodotti, prodottoInformativo(f, pv))
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
	s.FontiScenario = fontiDelloScenario(f, e, scenario, tr, ix, nonVerificabile)
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

// fontiDelloScenario: PO-29 («per ogni prodotto, il risultato atteso, ricavato dagli attesi: i file con esito radice
// per quel target»), con la derivazione esplicita e indipendente dal motore che R109 chiede.
//
// R109, precisata dall'utente il 07/10, senza scegliere una lettera; E2 §2.3. Lettura [T]: la regola del runner (la A
// del testo della domanda) resta valida, con la derivazione dagli attesi esplicita e indipendente dal motore:
//  1. si scorrono i prodotti attesi, non quelli del motore: le basi dei prodotti attesi dello scenario e le basi del
//     target delle sue voci radice, distinte, in ordine di byte (T-B6-222);
//  2. la fonte attesa di un prodotto, con la regola RegolaFonteAttesa (fonteAttesa): le voci dello scenario con atteso
//     radice e la base del target uguale; se fra quelle risolte c'è uno STEP (l'estensione dichiarata o del nome, come in
//     valutazione: T-E1-09; un fatto della fotografia, non un'uscita del motore), in_attesa_di_conferma con il motivo
//     documento_candidato e quegli STEP come candidati; altrimenti assente, e il motivo gli attesi non lo fissano: il
//     rapporto lo scrive, e il motivo calcolato non si confronta. Mai confermata, nessuna BOM di lavoro;
//  3. il prodotto calcolato è quello del thread dello scenario con la stessa base, scritta com'è (nessuna
//     normalizzazione): un prodotto atteso senza calcolato è una differenza esplicita, mai un «assente» che coincide per
//     caso (fonteCalcolata);
//  4. i livelli restano distinti (livelliDellaFonte): la struttura è l'asse della gerarchia del prodotto calcolato,
//     calcolato e non confrontato; la verifica operativa non è mai di A1c.
//
// Solo informazione, fuori dal gate, con -attesi: gli attesi li legge il runner, nessun agente.
func fontiDelloScenario(f fotorfq.Fotografia, e valut.Esito, s *ScenarioAtteso, tr *traduzione, ix indiceFoto, nonVerificabile string) *FontiDelloScenario {
	if s == nil || tr == nil {
		return nil
	}
	out := &FontiDelloScenario{NonVerificabile: nonVerificabile}
	if s.ThreadID != nil {
		id := *s.ThreadID
		out.ThreadID = &id
	} else if out.NonVerificabile == "" {
		out.NonVerificabile = "lo scenario degli attesi non dice il thread"
	}
	perBase := map[string]*ConfrontoFonte{}
	var basi []string
	prodotto := func(base string) *ConfrontoFonte {
		c := perBase[base]
		if c == nil {
			c = &ConfrontoFonte{Base: base, Derivazione: DerivazioneFonte{Regola: RegolaFonteAttesa}}
			perBase[base] = c
			basi = append(basi, base)
		}
		return c
	}
	for _, p := range s.Prodotti {
		if p.Base == "" { // un prodotto atteso senza base è già una differenza della traduzione (prodottoAttesoDi)
			continue
		}
		c := prodotto(p.Base)
		c.Derivazione.Prodotti = append(c.Derivazione.Prodotti, p.Percorso)
	}
	for _, vt := range tr.voci {
		v := vt.voce
		if v.Sezione != confronto.SezioneScenario || v.Atteso != confronto.AttesoRadice || v.TargetBase == "" {
			continue // una radice senza la base del target è già una differenza della traduzione (D8)
		}
		c := prodotto(v.TargetBase)
		if !vt.risolta {
			c.Derivazione.NonRisolte = append(c.Derivazione.NonRisolte, v.Percorso+": "+vt.motivoNR)
			continue
		}
		c.Derivazione.Voci = append(c.Derivazione.Voci, v.Percorso)
		if a, ok := ix.allegati[vt.file.AllegatoID]; ok && eSTEP(a.allegato) {
			c.Atteso.Candidati = append(c.Atteso.Candidati, vt.file.AllegatoID)
		}
	}
	sort.Strings(basi)

	var et *valut.EsitoThread
	var th *fotorfq.Thread
	if s.ThreadID != nil {
		for i := range e.Thread {
			if e.Thread[i].ThreadID == *s.ThreadID {
				et = &e.Thread[i]
			}
		}
		if i, ok := ix.thread[*s.ThreadID]; ok {
			th = &f.Thread[i]
		}
	}
	for _, base := range basi {
		c := perBase[base]
		confrontaCandidati := fonteAttesa(c)
		var gerarchia *AsseInformativo
		if pv := fonteCalcolata(c, et, out.NonVerificabile, confrontaCandidati); pv != nil {
			// l'asse della gerarchia com'è nella sezione dei prodotti, con il «non calcolato» delle sezioni assenti (T-12)
			for _, a := range prodottoInformativo(f, *pv).Assi {
				if a.Asse == "gerarchia" {
					x := a
					gerarchia = &x
				}
			}
		}
		c.Livelli = livelliDellaFonte(*c, gerarchia, th, ix, out.NonVerificabile)
		out.Prodotti = append(out.Prodotti, *c)
	}
	if out.NonVerificabile == "" && et != nil && et.Valutato {
		for _, pv := range et.ProdottiValutati {
			if perBase[pv.Base.Normalizzata] == nil {
				out.CalcolatiSenzaAtteso = append(out.CalcolatiSenzaAtteso, pv.Base.Normalizzata+" ("+pv.Rif+")")
			}
		}
		sort.Strings(out.CalcolatiSenzaAtteso)
	}
	return out
}

// fonteAttesa: la fonte attesa di un prodotto dalle sue voci radice (RegolaFonteAttesa), con le parti che non si
// derivano fra le non verificate. Restituisce se i candidati attesi sono tutti noti, cioè se si possono confrontare.
//   - Uno STEP fra le radici risolte: in_attesa_di_conferma, documento_candidato, quegli STEP come candidati. Con una
//     radice non risolta gli STEP attesi possono essere di più: i candidati non si confrontano.
//   - Nessuno STEP e una radice non risolta: senza il file non si sa se è uno STEP, e la fonte attesa non si deriva
//     (stato vuoto): né lo stato né il motivo si confrontano.
//   - Nessuno STEP e nessuna radice non risolta: assente. Gli attesi non fissano il motivo: il rapporto lo scrive fra le
//     parti non verificate, e il motivo calcolato non si confronta (R109; E2 §2.3).
func fonteAttesa(c *ConfrontoFonte) bool {
	ordinaUUID(c.Atteso.Candidati)
	nr := len(c.Derivazione.NonRisolte)
	switch {
	case len(c.Atteso.Candidati) > 0:
		c.Atteso.Stato, c.Atteso.Motivo = string(valut.FonteInAttesaDiConferma), string(valut.MotivoFonteDocumentoCandidato)
		if nr > 0 {
			c.NonVerificate = append(c.NonVerificate, fmt.Sprintf("candidati: %d voci radice del target non risolte, e gli STEP attesi possono essere di più", nr))
			return false
		}
	case nr > 0:
		c.NonVerificate = append(c.NonVerificate, fmt.Sprintf("stato e motivo: %d voci radice del target non risolte: senza i file non si sa se fra le radici c'è uno STEP, e la fonte attesa non si deriva", nr))
	default:
		c.Atteso.Stato = string(valut.FonteAssente)
		c.NonVerificate = append(c.NonVerificate, "motivo: non fissato dagli attesi (la fonte attesa è assente: nessuno STEP fra le radici attese); il motivo calcolato non si confronta")
	}
	return true
}

// fonteCalcolata: il prodotto calcolato con la base attesa, e le differenze; restituisce il prodotto calcolato, nil se
// non c'è o non si sceglie. Un prodotto atteso senza calcolato è una differenza; due prodotti calcolati con la stessa
// base sono una differenza, e il confronto non sceglie; se la corsa non può verificare lo scenario, la parte calcolata
// è fra le non verificate, mai una differenza.
func fonteCalcolata(c *ConfrontoFonte, et *valut.EsitoThread, nonVerificabile string, confrontaCandidati bool) *valut.ProdottoValutato {
	senza := func(perche string) {
		c.SenzaCalcolato = true
		c.Differenze = append(c.Differenze, "prodotto atteso senza calcolato: "+perche)
	}
	switch {
	case nonVerificabile != "":
		c.NonVerificate = append(c.NonVerificate, "calcolato: "+nonVerificabile)
		return nil
	case et == nil:
		senza("il thread dello scenario non è nell'esito")
		return nil
	case !et.Valutato:
		senza("il thread dello scenario non è valutato (" + string(et.Motivo) + ")")
		return nil
	}
	var trovati []valut.ProdottoValutato
	for _, pv := range et.ProdottiValutati {
		if pv.Base.Normalizzata == c.Base {
			trovati = append(trovati, pv)
		}
	}
	switch len(trovati) {
	case 0:
		senza("nessun prodotto del thread dello scenario ha la base attesa")
		return nil
	case 1:
	default:
		c.Differenze = append(c.Differenze, fmt.Sprintf("%d prodotti calcolati con la base attesa: il confronto non sceglie", len(trovati)))
		return nil
	}
	pv := trovati[0]
	k := &RisultatoFonte{Stato: string(pv.Fonte.Stato), Motivo: string(pv.Fonte.Motivo), BOMDiLavoro: len(pv.Nodi) > 0}
	for _, x := range pv.Fonte.Candidati {
		if x.AllegatoID != nil {
			k.Candidati = append(k.Candidati, *x.AllegatoID)
		}
	}
	ordinaUUID(k.Candidati)
	c.Calcolato = k
	a := c.Atteso
	if a.Stato != "" && a.Stato != k.Stato {
		c.Differenze = append(c.Differenze, "stato")
	}
	if a.Motivo != "" && a.Motivo != k.Motivo {
		c.Differenze = append(c.Differenze, "motivo")
	}
	if confrontaCandidati && len(a.Candidati) > 0 && !stessiUUID(a.Candidati, k.Candidati) {
		c.Differenze = append(c.Differenze, "candidati")
	}
	if k.Stato == string(valut.FonteConfermata) {
		c.Differenze = append(c.Differenze, "confermata: mai per lo scenario")
	}
	if k.BOMDiLavoro {
		c.Differenze = append(c.Differenze, "bom_di_lavoro: mai per lo scenario")
	}
	return &pv
}

// livelliDellaFonte: i sei livelli di un prodotto atteso, distinti (R109; E1 §PO-29), ognuno con la sua sorgente:
//   - presente: le voci radice degli attesi e i loro file nella fotografia;
//   - estrazione riuscita: i fatti della fotografia degli STEP attesi (alla terna, il grafo dichiarato dal worker con
//     motivo_parziale, il lavoro pendente): dati d'ingresso, non l'uscita del motore che si verifica;
//   - struttura: l'asse della gerarchia del prodotto calcolato (gerarchia, com'è nella sezione dei prodotti), che misura
//     la «struttura corretta» della risposta; «calcolato, non confrontato»: gli attesi di PO-29 non la fissano (R-144);
//   - associazione: gli STEP attesi fra i candidati del motore per il prodotto;
//   - autorizzazione come fonte: lo stato e il motivo della fonte, attesi e calcolati (mai confermata per lo scenario);
//   - verifica operativa: mai in A1c (E2 §2.3).
func livelliDellaFonte(c ConfrontoFonte, gerarchia *AsseInformativo, th *fotorfq.Thread, ix indiceFoto, nonVerificabile string) []LivelloFonte {
	n, nr := len(c.Atteso.Candidati), len(c.Derivazione.NonRisolte)
	senzaCalcolato := "nessun prodotto calcolato con la base attesa"
	switch {
	case nonVerificabile != "":
		senzaCalcolato = "non verificato: " + nonVerificabile
	case c.Calcolato == nil && !c.SenzaCalcolato:
		senzaCalcolato = "più prodotti calcolati con la base attesa: nessuna scelta"
	}

	estrazione := "nessuno STEP fra le radici attese"
	if n > 0 {
		conFatti, completo, parziale, nonDeterminabile, pendenti := 0, 0, 0, 0, 0
		for _, id := range c.Atteso.Candidati {
			if th == nil {
				break
			}
			if dentroUUID(th.InAttesa, id) {
				pendenti++
			}
			a := ix.allegati[id].allegato
			if a.Sha256 == nil {
				continue
			}
			ft, ok := th.Fatti[*a.Sha256]
			if !ok {
				continue
			}
			conFatti++
			switch {
			case ft.MotivoParziale == nil:
				nonDeterminabile++
			case *ft.MotivoParziale == "":
				completo++
			default:
				parziale++
			}
		}
		estrazione = fmt.Sprintf("fatti della fotografia (dati d'ingresso, non l'uscita del motore): %d STEP attesi su %d con i fatti alla terna "+
			"(grafo completo %d, parziale %d, non determinabile %d), %d con il lavoro pendente", conFatti, n, completo, parziale, nonDeterminabile, pendenti)
		if th == nil {
			estrazione = "il thread dello scenario non è nella fotografia"
		}
	}

	struttura := senzaCalcolato
	if gerarchia != nil {
		struttura = "asse della gerarchia del prodotto calcolato: " + vuotoONo(gerarchia.Stato)
		if gerarchia.Motivo != "" {
			struttura += " (" + gerarchia.Motivo + ")"
		}
		struttura += "; calcolato, non confrontato"
	}
	associazione, autorizzazione := senzaCalcolato, senzaCalcolato
	if k := c.Calcolato; k != nil {
		comuni := 0
		for _, id := range c.Atteso.Candidati {
			if dentroUUID(k.Candidati, id) {
				comuni++
			}
		}
		associazione = fmt.Sprintf("motore: %d candidati per il prodotto, %d fra gli STEP attesi", len(k.Candidati), comuni)
		autorizzazione = "motore: " + vuotoONo(k.Stato) + ", motivo " + vuotoONo(k.Motivo)
	}
	autorizzazioneAttesa := "non derivata: voci radice del target non risolte"
	if c.Atteso.Stato != "" {
		motivo := c.Atteso.Motivo
		if motivo == "" {
			motivo = "non fissato dagli attesi"
		}
		autorizzazioneAttesa = c.Atteso.Stato + ", motivo " + motivo + "; mai confermata"
	}
	return []LivelloFonte{
		{Livello: LivelloPresente, Atteso: fmt.Sprintf("%d voci radice del target, %d STEP fra quelle risolte", len(c.Derivazione.Voci)+nr, n),
			Calcolato: fmt.Sprintf("fotografia: %d voci radice con il file, %d senza (non risolte)", len(c.Derivazione.Voci), nr)},
		{Livello: LivelloEstrazione, Atteso: "non fissata dagli attesi", Calcolato: estrazione},
		{Livello: LivelloStruttura, Atteso: "non fissata dagli attesi: la «struttura corretta» la misura l'asse della gerarchia del prodotto (R-144)",
			Calcolato: struttura},
		{Livello: LivelloAssociazione, Atteso: fmt.Sprintf("%d STEP attesi come candidati", n), Calcolato: associazione},
		{Livello: LivelloAutorizzazione, Atteso: autorizzazioneAttesa, Calcolato: autorizzazione},
		{Livello: LivelloVerifica, Atteso: "mai in A1c", Calcolato: "non eseguita: mai in A1c (E2 §2.3)"},
	}
}

func dentroUUID(s []uuid.UUID, x uuid.UUID) bool {
	for _, y := range s {
		if y == x {
			return true
		}
	}
	return false
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
