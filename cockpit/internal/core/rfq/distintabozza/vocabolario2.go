package distintabozza

// Il vocabolario 2 del giornale di una bozza formato 3 (EM §9.2; decisioni [T] dell'integratore sulle proposte
// dell'ondata A, 18/10: PE-03, PE-04, PE-05, PE-22). Sono i verbi del vocabolario 1 con gli stessi nomi (nodo_aggiunto
// resta per il trigger fonte_step_risolvi), piu' proposta_riallineata, che scrive solo il server, e file_lasciato.
//
// Le forme di prima e dopo, verbo per verbo (gli esempi sono i casi condivisi della pagina,
// transport/web/e2e/distinta_unica_casi.json, che il server rifa' nella sua meta' di T-16):
//
//	verbo                 chiave_bozza          prima                               dopo
//	nodo_aggiunto         il nodo               null                                il nodo formato 3 (+ azione_id)
//	nodo_tolto            il nodo               il nodo formato 3                   null | {azione_id}
//	nodo_modificato       il nodo               {campi}                             {campi}: identita, descrizione, tipo, op,
//	                                                                                caselle, componente_id
//	nodo_riusato          il nodo               {op, componente_id, identita?}      lo stesso, con op riusa
//	arco_aggiunto         il figlio             null                                l'arco formato 3 (+ azione_id)
//	arco_tolto            il figlio             l'arco                              null | {azione_id}
//	arco_spostato         il figlio             l'arco                              l'arco con il padre nuovo, origine utente
//	qta_modificata        il figlio             {padre, figlio, qta}                {padre, figlio, qta}
//	decisione_aggiunta    quella della decis.   null                                la decisione formato 3
//	decisione_tolta       quella della decis.   la decisione                        null
//	file_preassociato     il nodo               null                                {proposta_id, chiave_bozza} (+ azione_id)
//	file_svincolato       il nodo               {proposta_id, chiave_bozza}         {proposta_id, motivo} (+ azione_id)
//	file_lasciato         —                     null                                {proposta_id, motivo?}
//	rebase                —                     {base}                              {base, rif_scaduti}
//	sorgente_vista,       la destinazione?      null                                {allegato_id, gruppo, sha256?}
//	sorgente_rifiutata
//	annulla, rifai        quella dell'evento    null | {azione_id}                  {n}
//
// base = {conferma_id, numero, contatore, firma_struttura}: la versione corrente che R1 restituisce. azione_id: l'UUID v4
// della selezione multipla («Non includere»), su tutti gli eventi del gesto (EM §7.1): non entra nello snapshot, sta
// nell'evento, e l'annullamento lo porta nel suo prima (PE-05). Il codice dello svincolo sta in dopo.motivo (PE-04), in un
// elenco chiuso (PE-22: nodo_escluso per le preassociazioni di un registrato escluso).
//
// bozza_aperta, sorgente_scelta, sorgente_cambiata e proposta_riallineata li scrive solo il server (origine server), e
// non si annullano; nemmeno i nodo_tolto, arco_tolto e file_svincolato che il server scrive dentro cambia_sorgente e
// riallinea_proposta. La pila d'annullamento non attraversa una versione costruita dal server (annullamento_non_ammesso).

import (
	"encoding/json"

	"github.com/google/uuid"
)

// I verbi nuovi del vocabolario 2 (EM §9.2).
const (
	AzPropostaRiallineata = "proposta_riallineata" // solo del server: il riallineamento della proposta (§6.5)
	AzFileLasciato        = "file_lasciato"        // «Lascia senza associazione» dal vassoio (§3.7): non cambia lo snapshot
)

// I motivi dello svincolo, file_svincolato.dopo.motivo (EM §3.7, §9.2; PE-04, PE-22): un elenco chiuso.
const (
	SvincoloUtente        = "utente"         // «Togli la preassociazione»: non va nel vassoio
	SvincoloNodoTolto     = "nodo_tolto"     // il nodo e' uscito dalla bozza
	SvincoloNodoEscluso   = "nodo_escluso"   // il registrato e' escluso (PE-22): non nodo_tolto
	SvincoloNonIncluso    = "non_incluso"    // il nodo non e' incluso nella BOM (LS-02)
	SvincoloFonteCambiata = "fonte_cambiata" // il nodo e' uscito con il cambio della fonte (§6.6)
	SvincoloRifScaduto    = "rif_scaduto"    // la casella del nodo e' scaduta (§6.4)
)

// MotiviSvincolo: l'elenco chiuso dei motivi dello svincolo, nell'ordine della tabella di EM §9.2.
var MotiviSvincolo = []string{SvincoloUtente, SvincoloNodoTolto, SvincoloNodoEscluso, SvincoloNonIncluso, SvincoloFonteCambiata,
	SvincoloRifScaduto}

var motiviSvincolo = func() map[string]bool {
	m := map[string]bool{}
	for _, x := range MotiviSvincolo {
		m[x] = true
	}
	return m
}()

// MotivoSvincoloAmmesso dice se il motivo e' nell'elenco chiuso.
func MotivoSvincoloAmmesso(m string) bool { return motiviSvincolo[m] }

// SvincoloNelVassoio: uno svincolo con questo motivo mette la proposta nel vassoio «Da ricollocare» (EM §3.7): tutti i
// motivi dell'elenco tranne utente.
func SvincoloNelVassoio(m string) bool { return motiviSvincolo[m] && m != SvincoloUtente }

// AzioniDelVocabolario2: le azioni che il browser puo' dichiarare in un salvataggio del formato 3 (EM §9.2). La pagina di
// V1 non emette sorgente_vista (guardare non scrive), ma il verbo resta nel vocabolario.
var AzioniDelVocabolario2 = []string{
	AzSorgenteVista, AzSorgenteRifiutata, AzRebase, AzNodoAggiunto, AzNodoTolto, AzNodoModificato, AzNodoRiusato,
	AzArcoAggiunto, AzArcoTolto, AzArcoSpostato, AzQtaModificata, AzDecisioneAggiunta, AzDecisioneTolta,
	AzFilePreassociato, AzFileSvincolato, AzFileLasciato, AzAnnulla, AzRifai,
}

// AzioniSoloDelServer2: le azioni del vocabolario 2 che scrive solo il server, con origine server (EM §9.2): le versioni
// costruite dal server cominciano con una di queste, e la pila d'annullamento si ferma a loro.
var AzioniSoloDelServer2 = []string{AzBozzaAperta, AzSorgenteScelta, AzSorgenteCambiata, AzPropostaRiallineata}

var (
	azioniNote2        = insiemeDi(AzioniDelVocabolario2)
	azioniDelServer2   = insiemeDi(AzioniSoloDelServer2)
	azioniInvertibili2 = insiemeDi([]string{AzNodoAggiunto, AzNodoTolto, AzNodoModificato, AzNodoRiusato, AzArcoAggiunto,
		AzArcoTolto, AzArcoSpostato, AzQtaModificata, AzDecisioneAggiunta, AzDecisioneTolta, AzFilePreassociato,
		AzFileSvincolato, AzFileLasciato})
)

func insiemeDi(xx []string) map[string]bool {
	m := make(map[string]bool, len(xx))
	for _, x := range xx {
		m[x] = true
	}
	return m
}

// MotivoAnnullamentoNonAmmesso: un annulla o un rifai di un evento scritto prima dell'ultima versione costruita dal server,
// o di un evento del server (EM §9.2, 422 CORPO_NON_VALIDO).
const MotivoAnnullamentoNonAmmesso = "annullamento_non_ammesso"

// leggiAzioneID: l'azione_id di un prima o di un dopo, se c'e' (un UUID v4: la selezione multipla, EM §7.1); il resto
// dei campi resta a chi chiama.
func leggiAzioneID(campo string, raw json.RawMessage) (*uuid.UUID, *Errore) {
	if nullo(raw) {
		return nil, nil
	}
	var x struct {
		AzioneID *uuid.UUID `json:"azione_id"`
	}
	if err := json.Unmarshal(raw, &x); err != nil {
		return nil, corpo(campo, "non si legge: %v", err)
	}
	if x.AzioneID != nil && x.AzioneID.Version() != 4 {
		return nil, corpo(campo+".azione_id", "l'azione e' un UUID v4, lo stesso per gli eventi di un gesto")
	}
	return x.AzioneID, nil
}

// leggiSoloAzioneID: un prima o un dopo che e' null oppure {azione_id} e nient'altro (il dopo di nodo_tolto e arco_tolto,
// il prima di annulla e rifai: PE-05).
func leggiSoloAzioneID(campo string, raw json.RawMessage) (*uuid.UUID, *Errore) {
	if nullo(raw) {
		return nil, nil
	}
	var x struct {
		AzioneID *uuid.UUID `json:"azione_id"`
	}
	if e := decodificaStretta(campo, raw, &x); e != nil {
		return nil, e
	}
	if x.AzioneID == nil || x.AzioneID.Version() != 4 {
		return nil, corpo(campo+".azione_id", "qui c'e' solo l'azione_id del gesto, un UUID v4, oppure niente")
	}
	return x.AzioneID, nil
}

// senzaAzioneID toglie azione_id da un oggetto JSON (il nodo o l'arco del dopo di un gesto cumulativo, che lo snapshot non
// porta).
func senzaAzioneID(raw json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if nullo(raw) || json.Unmarshal(raw, &m) != nil {
		return raw
	}
	if _, ok := m["azione_id"]; !ok {
		return raw
	}
	delete(m, "azione_id")
	b, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return b
}

// conAzioneID aggiunge azione_id a un oggetto canonico (il dopo scritto dal server), se c'e'.
func conAzioneID(raw json.RawMessage, a *uuid.UUID) json.RawMessage {
	if a == nil {
		return raw
	}
	var m map[string]json.RawMessage
	if nullo(raw) {
		m = map[string]json.RawMessage{}
	} else if json.Unmarshal(raw, &m) != nil {
		return raw
	}
	b, _ := json.Marshal(a.String())
	m["azione_id"] = b
	return canonico(m)
}

// svincolo e' il prima di file_svincolato e il dopo di file_preassociato: la proposta e il nodo.
type svincolo struct {
	PropostaID  uuid.UUID `json:"proposta_id"`
	ChiaveBozza string    `json:"chiave_bozza"`
}

// DopoSvincolo e' il dopo di file_svincolato (PE-04: il codice dello svincolo sta qui).
type DopoSvincolo struct {
	PropostaID uuid.UUID  `json:"proposta_id"`
	Motivo     string     `json:"motivo"`
	AzioneID   *uuid.UUID `json:"azione_id,omitempty"`
}

// LeggiDopoSvincolo legge il dopo di file_svincolato: {proposta_id, motivo, azione_id?}, con il motivo dell'elenco chiuso
// (PE-22).
func LeggiDopoSvincolo(campo string, raw json.RawMessage) (DopoSvincolo, *Errore) {
	var d DopoSvincolo
	if nullo(raw) {
		return d, corpo(campo, "file_svincolato dice nel dopo la proposta e il motivo dello svincolo")
	}
	if e := decodificaStretta(campo, raw, &d); e != nil {
		return d, e
	}
	if d.PropostaID == uuid.Nil {
		return d, corpo(campo+".proposta_id", "manca la proposta")
	}
	if !MotivoSvincoloAmmesso(d.Motivo) {
		return d, corpo(campo+".motivo", "motivo dello svincolo %q: uno di utente, nodo_tolto, nodo_escluso, non_incluso, fonte_cambiata, rif_scaduto", d.Motivo)
	}
	if d.AzioneID != nil && d.AzioneID.Version() != 4 {
		return d, corpo(campo+".azione_id", "l'azione e' un UUID v4, lo stesso per gli eventi di un gesto")
	}
	return d, nil
}

// DopoLasciato e' il dopo di file_lasciato: la proposta che esce dal vassoio, con un motivo facoltativo.
type DopoLasciato struct {
	PropostaID uuid.UUID `json:"proposta_id"`
	Motivo     string    `json:"motivo,omitempty"`
}

// LeggiDopoLasciato legge il dopo di file_lasciato: {proposta_id, motivo?}; il motivo e' un testo umano (NFC, al piu'
// MaxMotivo caratteri).
func LeggiDopoLasciato(campo string, raw json.RawMessage) (DopoLasciato, *Errore) {
	var d DopoLasciato
	if nullo(raw) {
		return d, corpo(campo, "file_lasciato dice nel dopo la proposta")
	}
	if e := decodificaStretta(campo, raw, &d); e != nil {
		return d, e
	}
	if d.PropostaID == uuid.Nil {
		return d, corpo(campo+".proposta_id", "manca la proposta")
	}
	d.Motivo = nfc(d.Motivo)
	if d.Motivo != "" && !testoLibero(d.Motivo, MaxMotivo) {
		return d, corpo(campo+".motivo", "il motivo sta in %d caratteri, senza caratteri di controllo", MaxMotivo)
	}
	return d, nil
}

// LeggiPrimaAnnullamento legge il prima di annulla e rifai: null, oppure {azione_id} quando l'evento annullato o ripetuto
// e' di un gesto cumulativo (PE-05). Che sia l'azione_id dell'evento lo controlla la riconciliazione.
func LeggiPrimaAnnullamento(campo string, raw json.RawMessage) (*uuid.UUID, *Errore) {
	return leggiSoloAzioneID(campo, raw)
}
