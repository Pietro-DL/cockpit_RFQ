package distintabozza

// riepilogo3_completamento.go: il completamento di R4 formato 3 (direttiva 39, blocco «R4 completo» della corsia
// persistenza; EM §7.3, §7.4, §8.2, §9.4; U2 §5.4, §6.1), nella parte pura del riepilogo, sopra letture fatte da
// leggiIngressiR4 (riepilogo3_lettura.go) nella stessa transazione. Tutto entra nella firma (anteprima-3), salvo ingressi.
//
//  1. Gli avvisi del registro della versione, senza il motore (R5 li rifa' dal registro):
//     - EQUIVALENTE_NON_ADOTTATO (EM §8.2; [U] Q12, RV3-31): un nodo incluso va nella working con un codice diverso dal
//       P/N equivalente K4 che il registro propone per la sua casella, e la bozza non ha una decisione identita con modo
//       alternativa e motivo sul nodo (che lo adotta o lo rifiuta). Non scatta quando nella working va il P/N;
//     - CORREZIONE_CARTIGLIO_NON_ADOTTATA (U2-P21): un riferimento documentale con esito correzione_proposta (il cartiglio
//       di un 2D associato dice un altro codice) sul nodo incluso, senza nessuna decisione identita sul nodo, con il codice
//       del nodo diverso da quello del cartiglio;
//     - PREASSOCIAZIONE_EMITTENTE_DIVERSO (EM §3.7; [T] A1 §1.8): la lettura primaria del file della PB nel registro ha un
//       emittente diverso da quello deciso per il nodo;
//     - NON_INCLUSO_SEGNALE_CAMBIATO (EM §7.3): il segnale della voce nel registro della versione (segnale_registro) non e'
//       il segnale visto della decisione non_includere; la decisione resta valida.
//  2. Gli effetti dei non inclusi (EM §7.4): le righe e gli archi legacy che R5 chiudera' (la stessa regola di R5:
//     daChiudereNI), la padre_nella_bozza degli archi, i figli (nessuna cascata: sempre vuoto), i fabbisogni tolti dopo la
//     conferma (i fabbisogni previsti della voce nel registro).
//  3. Le esclusioni con origine legacy ([U] Q13: le righe dello STEP sorgente scartate da una persona nelle pagine di
//     prima, con la voce esclusione_legacy del registro delle decisioni) ed ereditata (le non_includere della conferma di
//     base che la bozza non ripete e non reinclude).
//  4. Le condizioni (c) ed (e) di prima-istanza-1 sullo stato dopo R5 (come la (b), R39-22): (c) le righe dello STEP
//     sorgente ancora da decidere (aperte o decise da un automatismo) che R5 non segna, sui nodi raggiunti della proposta,
//     radice esclusa, e le rimozioni proposte aperte fra due componenti della bozza che R5 non chiude; (e) le righe aperte
//     degli STEP secondari autorizzati (dichiarazioni valide del Fascicolo di componenti non finiti della bozza).

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// I codici degli avvisi del completamento (EM §8.2).
const (
	AvvisoEquivalenteNonAdottato        = "EQUIVALENTE_NON_ADOTTATO"
	AvvisoCorrezioneCartiglioNonAdottat = "CORREZIONE_CARTIGLIO_NON_ADOTTATA"
	AvvisoPreassociazioneEmittenteDiv   = "PREASSOCIAZIONE_EMITTENTE_DIVERSO"
	AvvisoNonInclusoSegnaleCambiato     = "NON_INCLUSO_SEGNALE_CAMBIATO"
)

// Le origini di esclusi[] (EM §7.4, U2 §6.1).
const (
	OrigineEsclusoLegacy    = "legacy"
	OrigineEsclusoEreditata = "ereditata"
	OrigineEsclusoAssenza   = "assenza"
)

// VoceEsclusioneLegacy: il tipo della voce del registro delle decisioni per un nodo escluso nelle pagine di prima (EM §7.4).
const VoceEsclusioneLegacy = "esclusione_legacy"

// stepSecondario: uno STEP secondario autorizzato nel perimetro (la condizione (e)): il componente e il contenuto.
type stepSecondario struct {
	componente uuid.UUID
	sha256     string
}

// RigaDaChiudere: una riga legacy che R5 chiudera' per un non incluso.
type RigaDaChiudere struct {
	PropostaID uuid.UUID `json:"proposta_id"`
	AllegatoID uuid.UUID `json:"allegato_id"`
}

// ArcoDaChiudere: un arco legacy che R5 chiudera' per un non incluso.
type ArcoDaChiudere struct {
	AllegatoID      uuid.UUID `json:"allegato_id"`
	PadreChiave     string    `json:"padre_chiave"`
	FiglioChiave    string    `json:"figlio_chiave"`
	PadreNellaBozza bool      `json:"padre_nella_bozza"`
}

// ------------------------------------------------------------------ 1. gli avvisi del registro

// rifRegistro: un riferimento di una voce del registro (EM §3.8), con i soli campi che il completamento guarda.
type rifRegistro struct {
	Tipo        string `json:"tipo"`
	AllegatoID  string `json:"allegato_id"`
	Esito       string `json:"esito"`
	Equivalente *struct {
		Namespace string `json:"namespace"`
		Base      string `json:"base"`
		Marcatore string `json:"marcatore"`
	} `json:"equivalente"`
	Correzione *struct {
		Codice string `json:"codice"`
	} `json:"correzione"`
}

func riferimentiDi(v importazione.Voce) []rifRegistro {
	if nullo(v.Riferimenti) {
		return nil
	}
	var out []rifRegistro
	if json.Unmarshal(v.Riferimenti, &out) != nil {
		return nil
	}
	return out
}

var reNonAlfanumerico = regexp.MustCompile(`[^0-9A-Z]+`)

// stessoCodice: due codici uguali senza maiuscole, spazi ai lati e separatori (il P/N K4 e' una base normalizzata).
func stessoCodice(a, b string) bool {
	a, b = strings.ToUpper(strings.TrimSpace(a)), strings.ToUpper(strings.TrimSpace(b))
	if a == "" || b == "" {
		return false
	}
	return a == b || reNonAlfanumerico.ReplaceAllString(a, "") == reNonAlfanumerico.ReplaceAllString(b, "")
}

// decisioniIdentita: le decisioni identita della bozza per nodo.
func (a *analisiR4) decisioniIdentita() map[string][]Decisione3 {
	out := map[string][]Decisione3{}
	for _, d := range a.dec[DecisioneIdentita] {
		out[d.ChiaveBozza] = append(out[d.ChiaveBozza], d)
	}
	return out
}

// avvisiDelRegistro: EQUIVALENTE_NON_ADOTTATO, CORREZIONE_CARTIGLIO_NON_ADOTTATA, PREASSOCIAZIONE_EMITTENTE_DIVERSO,
// NON_INCLUSO_SEGNALE_CAMBIATO (commento di testa, punto 1). Senza un registro della versione non nasce niente.
func (a *analisiR4) avvisiDelRegistro() {
	reg := a.in.registro
	if reg == nil {
		return
	}
	idd := a.decisioniIdentita()
	for _, k := range a.chiavi {
		n := a.nodi[k]
		if !a.incluso(k) {
			continue
		}
		visti := map[string]bool{}
		for _, c := range n.Caselle {
			v, ok := reg.Contenuto.VocePerRif(c.AllegatoID, c.Rif)
			if !ok {
				continue
			}
			for _, r := range riferimentiDi(v) {
				switch {
				case r.Tipo == "equivalente_k4" && r.Equivalente != nil && strings.TrimSpace(r.Equivalente.Base) != "":
					pn := strings.TrimSpace(r.Equivalente.Base)
					if visti["eq|"+pn] || stessoCodice(a.chiaveWorking(k), pn) {
						continue
					}
					deciso := false
					for _, d := range idd[k] {
						if d.Modo == ModoAlternativa && strings.TrimSpace(d.Motivo) != "" {
							deciso = true
						}
					}
					if deciso {
						continue
					}
					visti["eq|"+pn] = true
					a.avvisa(Rilievo{Codice: AvvisoEquivalenteNonAdottato, ChiaveBozza: k, ComponenteID: n.ComponenteID, Valore: pn,
						Testo: a.nome(k) + " va nella BOM con il suo codice, ma i cartigli dicono che il codice della parte e' " + pn +
							": adottalo come codice, oppure rifiuta l'equivalente con il motivo.",
						Rimedi: []string{"«Adotta come codice» il P/N (decisione identita, alternativa sulla lettura del P/N, con motivo)",
							"rifiuta l'equivalente (decisione identita, alternativa sulla lettura primaria, con motivo)"}})
				case r.Tipo == "documentale" && r.Esito == "correzione_proposta" && r.Correzione != nil && strings.TrimSpace(r.Correzione.Codice) != "":
					cod := strings.TrimSpace(r.Correzione.Codice)
					if len(idd[k]) > 0 || visti["cc|"+r.AllegatoID+"|"+cod] || strings.EqualFold(strings.TrimSpace(n.Identita.Codice), cod) {
						continue
					}
					visti["cc|"+r.AllegatoID+"|"+cod] = true
					rl := Rilievo{Codice: AvvisoCorrezioneCartiglioNonAdottat, ChiaveBozza: k, ComponenteID: n.ComponenteID, Valore: cod,
						Testo: "Il cartiglio di un disegno associato a " + a.nome(k) + " dice " + cod + ": la correzione non e' adottata.",
						Rimedi: []string{"correggi il codice del nodo con il cartiglio (decisione identita, correzione, con motivo)",
							"conferma l'identita' di adesso con una decisione"}}
					if id, err := uuid.Parse(r.AllegatoID); err == nil {
						rl.AllegatoID = uid(id)
					}
					a.avvisa(rl)
				}
			}
		}
	}
	// la lettura primaria del file della PB ha un altro emittente (EM §3.7)
	for _, p := range a.s.DocumentiPreassociati {
		n, ok := a.nodi[p.ChiaveBozza]
		if !ok || !a.incluso(p.ChiaveBozza) {
			continue
		}
		em := emittentePrimarioDelFile(reg.Contenuto, p.AllegatoID, n.Identita.Codice)
		if em == "" || em == n.Identita.Emittente {
			continue
		}
		a.avvisa(Rilievo{Codice: AvvisoPreassociazioneEmittenteDiv, ChiaveBozza: p.ChiaveBozza, AllegatoID: uid(p.AllegatoID), Valore: em,
			Testo: "Il file preassociato a " + a.nome(p.ChiaveBozza) + " e' letto con un altro emittente (" + em + ") rispetto a quello deciso per il nodo."})
	}
	// il segnale del registro della versione non e' quello visto dalla non_includere (EM §7.3)
	for _, d := range a.dec[DecisioneNonIncludere] {
		if d.Casella == nil || VerificaSegnaleVisto("", &reg.Contenuto, d) == nil {
			continue
		}
		a.avvisa(Rilievo{Codice: AvvisoNonInclusoSegnaleCambiato, AllegatoID: uid(d.Casella.AllegatoID), Chiavi: []string{d.Casella.Rif},
			Testo: "Il segnale del nodo non incluso e' cambiato rispetto a quello visto quando l'hai deciso: la decisione resta tua e resta valida."})
	}
}

// emittentePrimarioDelFile: l'emittente della lettura primaria del file nel registro [T-39]: la prima lettura (nell'ordine del
// registro) il cui codice proposto e' il codice del nodo; senza, la prima lettura con un codice proposto; "" senza letture.
func emittentePrimarioDelFile(c importazione.Contenuto, allegato uuid.UUID, codice string) string {
	for _, f := range c.Proposta.File {
		if f.AllegatoID != allegato.String() || nullo(f.Letture) {
			continue
		}
		var ll []struct {
			Emittente      string `json:"emittente"`
			CodiceProposto string `json:"codice_proposto"`
		}
		if json.Unmarshal(f.Letture, &ll) != nil {
			return ""
		}
		primo := ""
		for _, l := range ll {
			if strings.TrimSpace(l.CodiceProposto) == "" {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(l.CodiceProposto), strings.TrimSpace(codice)) {
				return l.Emittente
			}
			if primo == "" {
				primo = l.Emittente
			}
		}
		return primo
	}
	return ""
}

// ------------------------------------------------------------------ 2. e 3. gli effetti e le origini degli esclusi

// effettiNonIncluso: gli effetti di una non_includere che R4 sa dal database e dal registro (EM §7.4).
func (a *analisiR4) effettiNonIncluso(c Casella3) *EffettiEsclusoR4 {
	sha, ch := shaEChiave(c.Rif)
	pp := a.in.stessoStep[sha]
	if pp == nil {
		pp = []ProdottoStessoStep{}
	}
	e := &EffettiEsclusoR4{OccorrenzeToccate: []uuid.UUID{c.AllegatoID}, ProdottiConLoStessoStep: pp, RigheDaChiudere: []RigaDaChiudere{},
		ArchiDaChiudere: []ArcoDaChiudere{}, Figli: []string{}, FabbisogniTolti: []json.RawMessage{}}
	for _, r := range a.in.righeNI[sha+"|"+ch] {
		if decidibile(r) {
			e.RigheDaChiudere = append(e.RigheDaChiudere, RigaDaChiudere{PropostaID: r.PropostaID, AllegatoID: r.AllegatoID})
		}
	}
	nellaBozza := a.caselleIncluse()
	for _, x := range a.in.archiNI[sha+"|"+ch] {
		if x.Stato != db.StatoPropostaAperta {
			continue
		}
		e.ArchiDaChiudere = append(e.ArchiDaChiudere, ArcoDaChiudere{AllegatoID: x.AllegatoID, PadreChiave: x.PadreChiave, FiglioChiave: x.FiglioChiave,
			PadreNellaBozza: nellaBozza[sha+"|"+x.PadreChiave]})
	}
	if reg := a.in.registro; reg != nil {
		if v, ok := reg.Contenuto.VocePerRif(c.AllegatoID, c.Rif); ok && !nullo(v.FabbisogniPrevisti) {
			var ff []json.RawMessage
			if json.Unmarshal(v.FabbisogniPrevisti, &ff) == nil {
				for _, f := range ff {
					e.FabbisogniTolti = append(e.FabbisogniTolti, canonico(f))
				}
			}
		}
	}
	return e
}

// caselleIncluse: «sha|chiave» delle caselle dei nodi inclusi.
func (a *analisiR4) caselleIncluse() map[string]bool {
	out := map[string]bool{}
	for _, k := range a.chiavi {
		if !a.incluso(k) {
			continue
		}
		for _, c := range a.nodi[k].Caselle {
			if sha, ch := shaEChiave(c.Rif); sha != "" {
				out[sha+"|"+ch] = true
			}
		}
	}
	return out
}

// segnaleRegistro: il segnale della voce nel registro della versione, nella forma del segnale visto (nil: non segnalata).
func (a *analisiR4) segnaleRegistro(c Casella3) *importazione.SegnaleVisto {
	if a.in.registro == nil {
		return nil
	}
	ss, err := SegnaliAttesi(&a.in.registro.Contenuto, c)
	if err != nil || len(ss) == 0 {
		return nil
	}
	return &ss[0]
}

// radiceDellaSorgente: la chiave della radice dello STEP sorgente (la casella del nodo del prodotto sullo sha della sorgente,
// o la radice del registro), per escluderla dalle righe da decidere (R80).
func (a *analisiR4) radiceDellaSorgente(sha string) string {
	if a.radice != "" {
		for _, c := range a.nodi[a.radice].Caselle {
			if s, ch := shaEChiave(c.Rif); s == sha {
				return ch
			}
		}
	}
	if reg := a.in.registro; reg != nil {
		if s, ch := shaEChiave(reg.Contenuto.Gruppo.Radice); s == sha {
			return ch
		}
	}
	return ""
}

// decisoDaPersona: la riga e' chiusa da una persona (non un automatismo e non la nota di R5).
func decisoDaPersona(r db.ListRigheStrutturaR4Row) bool {
	return r.DecisoDa.Valid
}

// esclusiLegacyEdEreditati: le voci di esclusi[] con origine legacy ed ereditata (commento di testa, punto 3), dopo quelle
// gia' calcolate (presenti: «allegato|rif» gia' mostrati).
func (a *analisiR4) esclusiLegacyEdEreditati(presenti map[string]bool) []EsclusoR4 {
	var out []EsclusoR4
	sg := a.in.sorgente
	inclusi := a.caselleIncluse()
	nonIncl := map[string]bool{}
	for _, d := range a.dec[DecisioneNonIncludere] {
		if sha, ch := shaEChiave(d.Casella.Rif); sha != "" {
			nonIncl[sha+"|"+ch] = true
		}
	}
	gruppo := ""
	if a.s.Sorgente != nil {
		gruppo = a.s.Sorgente.Gruppo
	}
	if sg.Sha256 != "" && sg.AllegatoID != nil {
		radice := a.radiceDellaSorgente(sg.Sha256)
		visti := map[string]bool{}
		for _, r := range a.in.righeSha[sg.Sha256] {
			k := sg.Sha256 + "|" + r.Chiave
			if r.Stato != db.StatoPropostaScartata || !decisoDaPersona(r) || r.Chiave == radice || inclusi[k] || nonIncl[k] || visti[k] ||
				strings.TrimSpace(r.Nota.String) == NotaNonInclusoR5 || strings.HasPrefix(strings.TrimSpace(r.Nota.String), PrefissoNotaAssenzaR5) {
				continue
			}
			visti[k] = true
			c := Casella3{AllegatoID: *sg.AllegatoID, Gruppo: gruppo, Rif: "nodo:" + sg.Sha256 + ":" + r.Chiave}
			if presenti[c.AllegatoID.String()+"|"+c.Rif] {
				continue
			}
			x := EsclusoR4{Origine: OrigineEsclusoLegacy, Casella: &c, Motivo: strings.TrimSpace(r.Nota.String)}
			x.MotivoMancante = x.Motivo == ""
			if r.DecisoDa.Valid {
				u := r.DecisoDa.UUID
				x.AutoreID = &u
			}
			out = append(out, x)
		}
	}
	for _, n := range a.in.nonInclusiBase {
		k := strings.TrimSpace(n.Sha256) + "|" + n.Chiave
		if n.Origine != DecisioneNonIncludere || inclusi[k] || nonIncl[k] {
			continue
		}
		c := Casella3{AllegatoID: n.AllegatoID, Gruppo: n.Gruppo, Rif: "nodo:" + strings.TrimSpace(n.Sha256) + ":" + n.Chiave}
		if presenti[c.AllegatoID.String()+"|"+c.Rif] {
			continue
		}
		x := EsclusoR4{Origine: OrigineEsclusoEreditata, ID: n.DecisioneID.String, Casella: &c, MotivoCodice: n.MotivoCodice.String, Motivo: n.Motivo.String}
		x.MotivoMancante = strings.TrimSpace(x.Motivo) == ""
		out = append(out, x)
	}
	return out
}

// ------------------------------------------------------------------ 4. le condizioni (c) ed (e) di prima-istanza-1

// contiPrimaIstanza: le righe da decidere dello STEP sorgente e le rimozioni aperte che restano dopo R5 (c), le righe aperte
// degli STEP secondari autorizzati (e). Solo con una sorgente valida (senza, la verifica e' gia' «non calcolata»).
func (a *analisiR4) contiPrimaIstanza() (righe, rimozioni, secondarie int) {
	sg := a.in.sorgente
	if !sg.Valida || sg.Sha256 == "" {
		return 0, 0, 0
	}
	segnate := a.caselleIncluse()
	for _, d := range a.dec[DecisioneNonIncludere] {
		if sha, ch := shaEChiave(d.Casella.Rif); sha != "" {
			segnate[sha+"|"+ch] = true
		}
	}
	// le voci assenti: R5 ne chiude le righe con la nota dell'assenza (EM §7.8 con U2 §6.1)
	for _, c := range caselleAssenti(a.s, a.in.registro) {
		if sha, ch := shaEChiave(c.Rif); sha != "" {
			segnate[sha+"|"+ch] = true
		}
	}
	raggiunti := map[string]bool{}
	conRegistro := a.in.registro != nil
	if conRegistro {
		for _, v := range a.in.registro.Contenuto.Proposta.Nodi {
			if v.StatoMotore.Tolto != nil && *v.StatoMotore.Tolto || v.StatoMotore.Raggiunto != nil && !*v.StatoMotore.Raggiunto {
				continue
			}
			if sha, ch := shaEChiave(v.Origine.Rif); sha == sg.Sha256 {
				raggiunti[ch] = true
			}
		}
	}
	radice := a.radiceDellaSorgente(sg.Sha256)
	viste := map[string]bool{}
	for _, r := range a.in.righeSha[sg.Sha256] {
		if r.Chiave == radice || segnate[sg.Sha256+"|"+r.Chiave] || viste[r.Chiave] || (conRegistro && !raggiunti[r.Chiave]) {
			continue
		}
		if r.Stato == db.StatoPropostaAperta || !decisoDaPersona(r) {
			viste[r.Chiave] = true
			righe++
		}
	}
	// le rimozioni aperte fra due componenti della bozza; quelle dello STEP A sostituito le chiude R5
	presenti := map[uuid.UUID]bool{}
	for k, r := range a.risolto {
		if a.incluso(k) && strings.HasPrefix(r, "c:") {
			presenti[uuid.MustParse(r[2:])] = true
		}
	}
	var stepA uuid.UUID
	if fa := a.in.vc.FonteAutorizzata; fa != nil && sg.DocumentoID != nil && fa.DocumentoID != *sg.DocumentoID {
		stepA = fa.DocumentoID
	}
	for _, x := range a.in.rimozioni {
		if x.Stato == db.StatoPropostaAperta && presenti[x.PadreID] && presenti[x.FiglioID] && x.StepDocumentoID != stepA {
			rimozioni++
		}
	}
	// gli STEP secondari autorizzati dei componenti della bozza
	vistiSha := map[string]bool{}
	for _, s := range a.in.secondari {
		if !presenti[s.componente] || s.sha256 == sg.Sha256 || vistiSha[s.sha256] {
			continue
		}
		vistiSha[s.sha256] = true
		for _, r := range a.in.righeSha[s.sha256] {
			if r.Stato == db.StatoPropostaAperta {
				secondarie++
			}
		}
	}
	return righe, rimozioni, secondarie
}

// shaOrdinati: gli sha dati senza ripetizioni, in ordine.
func shaOrdinati(ss []string) []string {
	visti := map[string]bool{}
	var out []string
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s != "" && !visti[s] {
			visti[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
