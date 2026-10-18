package propostabom

import (
	"encoding/json"
	"errors"
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/valutazione"
)

// Le aggiunte di V1 al modello della pagina (/motore-a/dati, emendamento di V1 §9.9; PE-01, PE-10, PE-15, PE-16,
// PE-28 dell'interfaccia, accolte nelle decisioni dell'ondata A) che si calcolano dall'esito, pure: i gruppi del prodotto
// con l'impronta_contenuto di un'importazione fatta adesso (con lo stesso adattatore di R2 e di S6), i nodi dei gruppi
// con la completezza e le diagnostiche correnti, l'identità registrata dei componenti decisi, lo STEP secondario, il
// motivo della mancanza di gruppi. Il resto (la fonte, F8, la bozza del prodotto) lo legge il modello dal database.

// Le compatibilità di una radice con il prodotto come le mostra la pagina (PE-01): compatibile se la base della radice è
// quella del prodotto (uguale o equivalente), incerta altrimenti (parziale, non determinabile per una radice di riserva,
// discordante). Guardare non conferma niente.
const (
	CompatibilitaCompatibile = "compatibile"
	CompatibilitaIncerta     = "incerta"
)

// CompatibilitaDellaRadice: la compatibilità della pagina dalla compatibilità della struttura.
func CompatibilitaDellaRadice(c motorea.Compatibilita) string {
	switch c {
	case motorea.CompatibilitaUguale, motorea.CompatibilitaEquivalente:
		return CompatibilitaCompatibile
	}
	return CompatibilitaIncerta
}

// IngressoModello: l'esito già calcolato, le regole di lettura, il prodotto, le occorrenze non attive e, per l'identità
// registrata dei componenti decisi, la working e le certificazioni (letture del database di chi chiama).
type IngressoModello struct {
	Esito          *valutazione.Esito
	Thread         *valutazione.EsitoThread
	Regole         valutazione.RegoleDelThread
	Prodotto       string
	NonAttivi      NonAttivi
	Componenti     map[uuid.UUID]ComponenteDellaWorking
	Certificazioni map[uuid.UUID]Certificazione
}

// ComponenteDellaWorking: il componente della working com'è nel database (codice e rev, archiviato o no).
type ComponenteDellaWorking struct {
	Codice     string
	Rev        string
	Archiviato bool
}

// Certificazione: l'identità certificata del componente dall'ultima conferma formato 3 che lo contiene (EM §5.1;
// ListIdentitaRegistrate): il codice certificato, l'emittente con la presunzione, il namespace.
type Certificazione struct {
	Codice            string
	Emittente         string
	EmittentePresunto bool
	Namespace         string
}

// IdentitaRegistrata: l'identità di un componente della working nella forma che il nodo «registrato» copia (EM §3.4.2,
// §5.1; PE-10): emittente, presunzione, namespace, codice, rev (vuota = assente, PE-11) e il modo.
type IdentitaRegistrata struct {
	Emittente         string `json:"emittente"`
	EmittentePresunto bool   `json:"emittente_presunto,omitempty"`
	Namespace         string `json:"namespace,omitempty"`
	Codice            string `json:"codice"`
	Rev               string `json:"rev,omitempty"`
	Modo              string `json:"modo"`
}

// IdentitaDelRegistrato: l'identità registrata di un componente (EM §5.1). La certificazione vale se il codice
// certificato è byte per byte il codice di oggi e il componente non è archiviato: allora emittente, presunzione e
// namespace sono i suoi; altrimenti il cliente della RFQ, presunto, senza namespace, e superata dice che una
// certificazione c'era (l'avviso EMITTENTE_CERTIFICATO_SUPERATO). Codice e rev sono quelli della working.
func IdentitaDelRegistrato(cliente uuid.UUID, k ComponenteDellaWorking, cert *Certificazione) (id IdentitaRegistrata, superata bool) {
	id = IdentitaRegistrata{Emittente: "cliente:" + cliente.String(), EmittentePresunto: true, Codice: k.Codice, Rev: k.Rev, Modo: ModoRegistrato}
	if cert == nil {
		return id, false
	}
	if cert.Codice != k.Codice || k.Archiviato {
		return id, true
	}
	id.Emittente, id.EmittentePresunto, id.Namespace = cert.Emittente, cert.EmittentePresunto, cert.Namespace
	return id, false
}

// GruppoDelModello: un gruppo del prodotto come lo dà /dati (PE-01; EM §9.9): la chiave dal core, il tipo, l'allegato,
// la radice, il nome del file, l'impronta_contenuto di un'importazione fatta adesso, il letterale della radice, la
// compatibilità, la struttura (versione dei fatti, completezza, motivo), i nodi segnalati (LS-02) e i nodi. Se
// l'adattatore rifiuta il gruppo (un file non attivo, un registro incompleto), l'impronta non c'è e Importazione dice
// il rifiuto: il gruppo non si importa.
type GruppoDelModello struct {
	Chiave            string              `json:"chiave"`
	Tipo              string              `json:"tipo"`
	AllegatoID        *uuid.UUID          `json:"allegato_id,omitempty"`
	Radice            string              `json:"radice,omitempty"`
	NomeFile          string              `json:"nome_file,omitempty"`
	ImprontaContenuto string              `json:"impronta_contenuto,omitempty"`
	RadiceLetterale   string              `json:"radice_letterale,omitempty"`
	Compatibilita     string              `json:"compatibilita,omitempty"`
	Struttura         *StrutturaDelGruppo `json:"struttura,omitempty"`
	Segnalati         int                 `json:"segnalati"`
	Importazione      *ErroreImportazione `json:"importazione,omitempty"`
	Nodi              []NodoDelModello    `json:"nodi,omitempty"`
}

// StrutturaDelGruppo: lo stato della struttura del gruppo (LS-01): la sottoversione dei fatti STEP, la completezza del
// grafo e il motivo.
type StrutturaDelGruppo struct {
	Versione int    `json:"versione"`
	Completa bool   `json:"completa"`
	Motivo   string `json:"motivo,omitempty"`
}

// ErroreImportazione: il rifiuto dell'adattatore per un gruppo, {codice, motivo?, testo}.
type ErroreImportazione struct {
	Codice string `json:"codice"`
	Motivo string `json:"motivo,omitempty"`
	Testo  string `json:"testo"`
}

// NodoDelModello: un nodo del gruppo (una voce del registro di adesso) con i campi correnti per nodo (PE-28, PE-10): la
// completezza del 2D del nodo, le diagnostiche correnti (quelle della voce del registro di adesso: le diagnostiche che
// citano più nodi stanno nella testata del registro, non in un nodo), la decisione esistente con l'identità registrata
// del componente.
type NodoDelModello struct {
	Rif                string                        `json:"rif"`
	Voce               string                        `json:"voce"`
	Completezza        *CompletezzaDelNodo           `json:"completezza,omitempty"`
	Diagnostiche       json.RawMessage               `json:"diagnostiche,omitempty"`
	DecisioneEsistente *DecisioneEsistenteDelModello `json:"decisione_esistente,omitempty"`
}

// CompletezzaDelNodo: la completezza del 2D del nodo come la calcola la valutazione adesso (PE-28; EM §9.9; MAT PN-06:
// la pagina ne fa la mappatura dei nove stati), {requisito, esito, deroga?}. Per un nodo con un componente deciso la
// voce del 2D della completezza del componente (l'esito della valutazione, la deroga), o il suo fabbisogno non bloccante
// (esito «non_bloccante»); per un nodo senza componente il fabbisogno previsto del 2D (esito «previsto»). Senza un
// requisito del 2D, niente.
type CompletezzaDelNodo struct {
	Requisito string     `json:"requisito"`
	Esito     string     `json:"esito"`
	Deroga    *uuid.UUID `json:"deroga,omitempty"`
}

// Il requisito della completezza per nodo e gli esiti che non vengono dalla voce della completezza.
const (
	RequisitoDisegno2D         = "disegno_2d"
	EsitoRequisitoPrevisto     = "previsto"
	EsitoRequisitoNonBloccante = "non_bloccante"
)

// DecisioneEsistenteDelModello: il componente della riga legacy decisa del nodo con la sua identità registrata (PE-10):
// la pagina costruisce il nodo «registrato» copiandola, e R2 la verifica sulla working (V-1).
type DecisioneEsistenteDelModello struct {
	ComponenteID           uuid.UUID          `json:"componente_id"`
	IdentitaRegistrata     IdentitaRegistrata `json:"identita_registrata"`
	CertificazioneSuperata bool               `json:"certificazione_superata,omitempty"`
}

// StepSecondario: lo STEP secondario del prodotto (PE-15; D-SHOCK-S4 A): uno STEP attivo, diverso da quello della
// struttura, il cui file ha un candidato solo, a livello componente, su un nodo non radice della BOM di lavoro del
// prodotto, senza dimensioni discordanti: la corrispondenza esatta e univoca. Solo informazione: l'accettazione degli
// archi arriva in V5. Nel JSON i soli nomi di PE-15, {allegato_id, nome_file, motivo}: il nodo resta per chi chiama.
type StepSecondario struct {
	AllegatoID uuid.UUID `json:"allegato_id"`
	NomeFile   string    `json:"nome_file"`
	Motivo     string    `json:"motivo"`
	Nodo       string    `json:"-"`
}

// MotivoStepSecondarioEsatto: il motivo di StepSecondario.
const MotivoStepSecondarioEsatto = "corrispondenza esatta e univoca"

// MotivoSenzaGruppi: perché il prodotto non ha gruppi con uno STEP (PE-16; MAT SG-13), con il codice e il testo.
type MotivoSenzaGruppi struct {
	Codice string `json:"codice"`
	Testo  string `json:"testo"`
}

// ModelloDelProdotto: le aggiunte di V1 per il prodotto.
type ModelloDelProdotto struct {
	Gruppi            []GruppoDelModello `json:"gruppi"`
	MotivoSenzaGruppi *MotivoSenzaGruppi `json:"motivo_senza_gruppi,omitempty"`
	StepSecondario    *StepSecondario    `json:"step_secondario,omitempty"`
}

// Modello: le aggiunte di V1 per il prodotto (nil per un prodotto che non c'è). I gruppi sono quelli di
// GruppiDelProdotto senza le candidate delle occorrenze non attive (EM §9.9: le occorrenze non attive non entrano fra le
// candidate); la BOM di lavoro resta anche su un portatore non attivo, con il rifiuto dell'adattatore. Per ogni gruppo
// con uno STEP l'adattatore costruisce il registro di adesso: l'impronta_contenuto è quella che R2 ricalcolerà.
func Modello(in IngressoModello) *ModelloDelProdotto {
	et := in.Thread
	if et == nil || in.Esito == nil {
		return nil
	}
	var pv *valutazione.ProdottoValutato
	for i := range et.ProdottiValutati {
		if et.ProdottiValutati[i].Rif == in.Prodotto {
			pv = &et.ProdottiValutati[i]
		}
	}
	if pv == nil {
		return nil
	}
	out := &ModelloDelProdotto{Gruppi: []GruppoDelModello{}}
	file := map[uuid.UUID]*valutazione.FileInterpretato{}
	for i := range et.File {
		file[et.File[i].AllegatoID] = &et.File[i]
	}
	conSTEP := 0
	for _, g := range GruppiDelProdotto(*pv, et.Ancoraggi.Strutture) {
		if g.Tipo == GruppoCandidata && g.AllegatoID != nil {
			if _, no := in.NonAttivi[*g.AllegatoID]; no {
				continue
			}
		}
		gm := GruppoDelModello{Chiave: g.Chiave, Tipo: g.Tipo, AllegatoID: g.AllegatoID, Radice: g.Radice}
		if g.Tipo != GruppoSenzaSTEP && g.AllegatoID != nil {
			conSTEP++
			if f := file[*g.AllegatoID]; f != nil {
				gm.NomeFile = f.Documento.Fonte.Provenienza.NomeRicevuto
				gm.Struttura = &StrutturaDelGruppo{Versione: f.Documento.Fonte.RiferimentoFatti.SottoversioneSTEP}
			}
			if st := strutturaDi(*pv, et.Ancoraggi.Strutture, g); st != nil {
				gm.Compatibilita = CompatibilitaDellaRadice(st.Compatibilita)
				if gm.Struttura == nil {
					gm.Struttura = &StrutturaDelGruppo{}
				}
				gm.Struttura.Completa, gm.Struttura.Motivo = st.GrafoCompleto, st.MotivoGrafo
			}
			reg, err := ImportaGruppo(Ingresso{Esito: in.Esito, Thread: et, Regole: in.Regole, Prodotto: in.Prodotto, Gruppo: g.Chiave, NonAttivi: in.NonAttivi})
			if err != nil {
				gm.Importazione = erroreDiImportazione(err)
			} else {
				gm.ImprontaContenuto = reg.Contenuto.ImprontaContenuto
				gm.RadiceLetterale = letteraleDellaRadice(reg.Contenuto)
				gm.Segnalati, gm.Nodi = nodiDelGruppo(in, *pv, g, reg.Contenuto)
			}
		}
		out.Gruppi = append(out.Gruppi, gm)
	}
	if conSTEP == 0 {
		out.MotivoSenzaGruppi = motivoSenzaGruppi(*pv, et)
	}
	out.StepSecondario = StepSecondarioDelProdotto(et, *pv, in.NonAttivi)
	return out
}

// strutturaDi: la struttura del prodotto del gruppo (la BOM di lavoro, o la candidata con quell'allegato e quella
// radice); nil senza.
func strutturaDi(pv valutazione.ProdottoValutato, strutture []ancoraggio.StrutturaProdotto, g Gruppo) *ancoraggio.StrutturaProdotto {
	stato := ancoraggio.StatoStrutturaCandidata
	if g.Tipo == GruppoBOM {
		stato = ancoraggio.StatoBOMDiLavoroProposta
	}
	for i := range strutture {
		s := &strutture[i]
		if s.Target == pv.Rif && s.Stato == stato && g.AllegatoID != nil && s.AllegatoID == *g.AllegatoID && s.Radice == g.Radice {
			return s
		}
	}
	return nil
}

// erroreDiImportazione: il rifiuto dell'adattatore come lo porta il modello.
func erroreDiImportazione(err error) *ErroreImportazione {
	var e *importazione.Errore
	if errors.As(err, &e) {
		return &ErroreImportazione{Codice: e.Codice, Motivo: e.Motivo, Testo: e.Messaggio}
	}
	return &ErroreImportazione{Testo: err.Error()}
}

// letteraleDellaRadice: il letterale dell'identità proposta della voce della radice; "" senza.
func letteraleDellaRadice(c importazione.Contenuto) string {
	for _, v := range c.Proposta.Nodi {
		if !c.EVoceDellaRadice(v) || len(v.IdentitaProposta) == 0 {
			continue
		}
		var l struct {
			Letterale string `json:"letterale"`
		}
		_ = json.Unmarshal(v.IdentitaProposta, &l)
		return l.Letterale
	}
	return ""
}

// nodiDelGruppo: i nodi segnalati e i nodi del gruppo, uno per voce del registro, con i campi correnti.
func nodiDelGruppo(in IngressoModello, pv valutazione.ProdottoValutato, g Gruppo, c importazione.Contenuto) (int, []NodoDelModello) {
	segnalati := 0
	var out []NodoDelModello
	for _, v := range c.Proposta.Nodi {
		if len(v.Segnali) > 0 {
			segnalati++
		}
		n := NodoDelModello{Rif: v.Origine.Rif, Voce: v.Voce, Diagnostiche: v.Diagnostiche}
		var k *uuid.UUID
		if len(v.DecisioneEsistente) > 0 {
			var de struct {
				ComponenteID uuid.UUID `json:"componente_id"`
			}
			if json.Unmarshal(v.DecisioneEsistente, &de) == nil {
				id := de.ComponenteID
				k = &id
				if w, ok := in.Componenti[id]; ok {
					var cert *Certificazione
					if x, ok := in.Certificazioni[id]; ok {
						cert = &x
					}
					reg, sup := IdentitaDelRegistrato(in.Thread.ClienteID, w, cert)
					n.DecisioneEsistente = &DecisioneEsistenteDelModello{ComponenteID: id, IdentitaRegistrata: reg, CertificazioneSuperata: sup}
				}
			}
		}
		if k == nil && g.Tipo == GruppoBOM && c.EVoceDellaRadice(v) {
			k = pv.ComponenteID
		}
		n.Completezza = completezzaDelNodo(pv.Documenti, v.Origine.Rif, k)
		out = append(out, n)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Rif < out[j].Rif })
	return segnalati, out
}

// completezzaDelNodo: la completezza del 2D del nodo (PE-28): con il componente deciso la voce del 2D o il fabbisogno non
// bloccante del componente, senza il fabbisogno previsto del nodo; nil senza requisito del 2D.
func completezzaDelNodo(d valutazione.CompletezzaDocumentale, rif string, k *uuid.UUID) *CompletezzaDelNodo {
	if k != nil {
		for _, v := range d.Voci {
			if v.ComponenteID == *k && v.TipoDocumento == RequisitoDisegno2D {
				return &CompletezzaDelNodo{Requisito: RequisitoDisegno2D, Esito: string(v.Esito), Deroga: copiaUUID(v.DerogaID)}
			}
		}
		for _, v := range d.NonBloccanti {
			if v.ComponenteID == *k && v.TipoDocumento == RequisitoDisegno2D {
				return &CompletezzaDelNodo{Requisito: RequisitoDisegno2D, Esito: EsitoRequisitoNonBloccante, Deroga: copiaUUID(v.DerogaID)}
			}
		}
		return nil
	}
	for _, p := range d.Previsti {
		if p.Nodo == rif && p.TipoDocumento == RequisitoDisegno2D {
			return &CompletezzaDelNodo{Requisito: RequisitoDisegno2D, Esito: EsitoRequisitoPrevisto}
		}
	}
	return nil
}

func copiaUUID(v *uuid.UUID) *uuid.UUID {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}

// I codici di motivo_senza_gruppi che non vengono dalla fonte del prodotto.
const MotivoSenzaGruppiTargetSenzaStruttura = "target_senza_struttura"

// motivoSenzaGruppi: perché il prodotto non ha gruppi con uno STEP, con un testo per la pagina: il motivo della fonte del
// prodotto quando dice che uno STEP non c'è o non è letto (nessuna fonte, STEP non analizzato, analisi in corso,
// estrazione fallita, …); altrimenti, se nessun nodo degli STEP letti ha la base del prodotto,
// ancoraggio.target_senza_struttura; altrimenti il motivo della fonte con un testo generico.
func motivoSenzaGruppi(pv valutazione.ProdottoValutato, et *valutazione.EsitoThread) *MotivoSenzaGruppi {
	if testo, ok := testiSenzaGruppi[pv.Fonte.Motivo]; ok {
		return &MotivoSenzaGruppi{Codice: string(pv.Fonte.Motivo), Testo: testo}
	}
	for _, d := range et.Ancoraggi.Diagnostiche {
		if d.Codice != ancoraggio.CodiceTargetSenzaStruttura {
			continue
		}
		for _, r := range d.Rif {
			if r == pv.Rif {
				return &MotivoSenzaGruppi{Codice: MotivoSenzaGruppiTargetSenzaStruttura,
					Testo: "nessun nodo degli STEP letti ha la base del prodotto: se lo STEP del prodotto è un altro file, caricalo con «+ Aggiungi file»"}
			}
		}
	}
	return &MotivoSenzaGruppi{Codice: string(pv.Fonte.Motivo), Testo: "il motore non propone una struttura per il prodotto"}
}

// testiSenzaGruppi: i testi di motivo_senza_gruppi per i motivi della fonte.
var testiSenzaGruppi = map[valutazione.MotivoFonte]string{
	valutazione.MotivoFonteNessunaFonte:              "nessuno STEP del prodotto fra i file della RFQ: caricalo con «+ Aggiungi file»",
	valutazione.MotivoFonteStepPresenteNonAnalizzato: "uno STEP della RFQ non è ancora analizzato: «Prepara i file» o «Rifai l'analisi»",
	valutazione.MotivoFonteAnalisiInCorso:            "l'analisi dello STEP è in corso: riprova fra poco",
	valutazione.MotivoFonteNonAnalizzata:             "lo STEP non è analizzato: «Rifai l'analisi»",
	valutazione.MotivoFonteEstrazioneFallita:         "il worker non ha letto lo STEP: il file va controllato o ricaricato",
	valutazione.MotivoFonteSoloAltro3D:               "c'è un 3D che non è uno STEP: la struttura viene solo da uno STEP",
	valutazione.MotivoFonteProdottoSenzaComponente:   "il prodotto non ha un componente della RFQ",
	valutazione.MotivoFonteSoloNodoInterno:           "il prodotto compare solo come nodo interno di uno STEP più grande",
}

// StepSecondarioDelProdotto: lo STEP secondario del prodotto (PE-15), il primo per allegato; nil senza. Lo dicono gli
// ancoraggi dei file, con le loro regole: nessuna regola di lettura nuova.
func StepSecondarioDelProdotto(et *valutazione.EsitoThread, pv valutazione.ProdottoValutato, nonAttivi NonAttivi) *StepSecondario {
	var bom *ancoraggio.StrutturaProdotto
	for i := range et.Ancoraggi.Strutture {
		if s := &et.Ancoraggi.Strutture[i]; s.Target == pv.Rif && s.Stato == ancoraggio.StatoBOMDiLavoroProposta {
			bom = s
			break
		}
	}
	if bom == nil {
		return nil
	}
	file := map[uuid.UUID]*valutazione.FileInterpretato{}
	for i := range et.File {
		file[et.File[i].AllegatoID] = &et.File[i]
	}
	var trovati []StepSecondario
	for _, a := range et.Ancoraggi.File {
		f := file[a.AllegatoID]
		if f == nil || a.AllegatoID == bom.AllegatoID || a.Associazione != ancoraggio.AssociazioneCandidatoUnico || len(a.Candidati) != 1 {
			continue
		}
		if _, no := nonAttivi[a.AllegatoID]; no {
			continue
		}
		if _, ok := ancoraggio.StrutturaDa(ancoraggio.FileInterpretato{AllegatoID: f.AllegatoID, Documento: f.Documento,
			Interpretazione: f.Interpretazione}); !ok {
			continue
		}
		c := a.Candidati[0]
		if c.Livello != ancoraggio.LivelloComponente || len(c.Conflitti) > 0 {
			continue
		}
		nodo := ""
		for _, p := range c.Posizioni {
			if p.Target == pv.Rif && p.AllegatoID == bom.AllegatoID && p.Radice == bom.Radice && p.Nodo != bom.Radice {
				nodo = p.Nodo
			}
		}
		if nodo == "" {
			continue
		}
		trovati = append(trovati, StepSecondario{AllegatoID: a.AllegatoID, NomeFile: f.Documento.Fonte.Provenienza.NomeRicevuto,
			Motivo: MotivoStepSecondarioEsatto, Nodo: nodo})
	}
	if len(trovati) == 0 {
		return nil
	}
	sort.Slice(trovati, func(i, j int) bool { return trovati[i].AllegatoID.String() < trovati[j].AllegatoID.String() })
	return &trovati[0]
}
