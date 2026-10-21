package distintabozza

// La provenienza delle PB in R2, forma salvataggio (direttiva 39 §4, D-PB03 A [U]; EM I.8, R39-24 [T-39 corsia
// contratto]): la decide il server, mai la pagina da sola.
//
// La riconciliazione delle azioni (riconcilia3.go) guarda solo l'occorrenza e il nodo di una PB ({allegato_id,
// chiave_bozza}); la provenienza e il registro d'origine (importazione) che la pagina rimanda si controllano prima
// (controllaDichiarate) e si decidono dopo le verifiche (provenienzeDecise), contro la versione precedente:
//   - una PB rimasta sullo stesso nodo tiene provenienza e registro (D-PB03 A: un cambio della fonte non riscrive il
//     registro d'origine di una PB rimasta); la pagina li rimanda come li ha letti o li omette; una PB salvata prima di
//     questo blocco senza provenienza e' dell'ingegnere: manuale;
//   - una PB spostata su un altro nodo: motore o corretta → corretta, con il registro di prima (la correzione
//     dell'ingegnere conserva il riferimento originale, e il giornale ha i suoi file_svincolato e file_preassociato);
//     manuale → manuale; legacy → manuale [T-39: spostarla e' rifarla a mano, il rimedio di R39-15];
//   - una PB nuova: manuale;
//   - una PB nuova o spostata con il campo importazione (un suggerimento accettato) e' motore con quel registro, se il
//     registro e' del prodotto e propone quell'allegato su quel nodo e solo li' (PropostaNelRegistro, univoca: una proposta
//     ambigua non e' mai un suggerimento, D-PB01 A; G39-01 [T-39]); se non lo propone ma e' il
//     registro che la PB spostata aveva gia' (la pagina rimanda i campi letti), vale la regola di sopra; se il registro
//     propone l'allegato, in modo univoco, su un altro nodo, e' corretta con quel registro (G39-01 [U], 20/10 sera: la
//     proposta del motore spostata dall'ingegnere su un altro componente); altrimenti 422 CORPO_NON_VALIDO
//     proposta_non_corrispondente (registro sconosciuto: importazione_sconosciuta);
//   - il registro del calcolo di adesso (G39-01, adesso3.go): un registro che il database non ha e' quello del calcolo di
//     adesso se l'adattatore l'ha preparato con la stessa impronta, oppure, se il calcolo e' cambiato fra la lettura e il
//     salvataggio, se il registro di adesso propone l'allegato in modo univoco; la PB prende allora l'impronta del registro
//     di adesso, che il salvataggio scrive in distinta_importazione (D-PB03 A: il riferimento stabile al calcolo);
//   - la pagina non mette ne' cambia una provenienza (422 CORPO_NON_VALIDO provenienza_dal_server), salvo «motore» con il
//     registro del suggerimento.
// Gli eventi file_preassociato e file_svincolato del salvataggio portano la provenienza e il registro della PB che
// nominano (D-PB03 A: «lo snapshot e il giornale della bozza»), scritti dal server.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// senzaProvenienza: lo snapshot con le sole occorrenze e nodi delle PB (quello che la riconciliazione delle azioni
// confronta).
func senzaProvenienza(s Snapshot3) Snapshot3 {
	c := s.copia()
	for i := range c.DocumentiPreassociati {
		c.DocumentiPreassociati[i] = Preassociazione3{AllegatoID: c.DocumentiPreassociati[i].AllegatoID, ChiaveBozza: c.DocumentiPreassociati[i].ChiaveBozza}
	}
	return c
}

// controllaDichiarate: la provenienza e il registro che la pagina rimanda, PB per PB, contro la versione precedente
// (commento di testa). Pura.
func controllaDichiarate(prima, nuovo Snapshot3) error {
	pp := map[uuid.UUID]Preassociazione3{}
	for _, p := range prima.DocumentiPreassociati {
		pp[p.AllegatoID] = p
	}
	for i, p := range nuovo.DocumentiPreassociati {
		campo := fmt.Sprintf("snapshot.documenti_preassociati[%d].provenienza", i)
		rifiuta := func() error {
			return corpoMotivo(campo, MotivoProvenienzaDelServer,
				"la provenienza della preassociazione dell'allegato %s la decide il server, non la pagina", p.AllegatoID)
		}
		vuota := p.Provenienza == "" && p.Importazione == ""
		v, cera := pp[p.AllegatoID]
		letta := cera && p.Provenienza == v.Provenienza && p.Importazione == v.Importazione
		suggerimento := p.Importazione != "" && (p.Provenienza == "" || p.Provenienza == ProvenienzaMotore)
		switch {
		case vuota || letta:
		case cera && v.ChiaveBozza == p.ChiaveBozza:
			return rifiuta()
		case suggerimento:
		default:
			return rifiuta()
		}
	}
	return nil
}

// decisa: la provenienza di una PB {allegato, chiave} contro la versione precedente, senza il registro dichiarato dalla
// pagina (quello lo guarda provenienzeDecise): rimasta → com'era (manuale se era senza); spostata da motore o corretta →
// corretta con il registro di prima; il resto → manuale.
func decisa(prima map[uuid.UUID]Preassociazione3, a uuid.UUID, chiave string) Preassociazione3 {
	p := Preassociazione3{AllegatoID: a, ChiaveBozza: chiave, Provenienza: ProvenienzaManuale}
	v, cera := prima[a]
	switch {
	case cera && v.ChiaveBozza == chiave && v.Provenienza != "":
		p.Provenienza, p.Importazione = v.Provenienza, v.Importazione
		// R39-15: il riconoscimento di una PB legacy resta finche' la PB resta sul suo nodo
		p.Verificata = v.Provenienza == ProvenienzaLegacy && v.Verificata
	case cera && v.ChiaveBozza != chiave && (v.Provenienza == ProvenienzaMotore || v.Provenienza == ProvenienzaCorretta):
		p.Provenienza, p.Importazione = ProvenienzaCorretta, v.Importazione
	}
	return p
}

// rimasta: la PB e' sullo stesso nodo della versione precedente.
func rimasta(prima map[uuid.UUID]Preassociazione3, a uuid.UUID, chiave string) bool {
	v, ok := prima[a]
	return ok && v.ChiaveBozza == chiave
}

// provenienzeDecise: le PB dello snapshot nuovo con la provenienza decisa dal server (commento di testa), dopo le altre
// verifiche. Una PB nuova o spostata con il registro dichiarato (un suggerimento accettato) e' motore con quel registro
// se il registro, del prodotto nella RFQ, propone il file su quel nodo; altrimenti, se e' il registro che la PB spostata
// aveva gia' (la pagina rimanda i campi letti), vale la regola di decisa; altrimenti 422 importazione_sconosciuta o
// proposta_non_corrispondente.
//
// R39-15: una PB legacy rimasta sul suo nodo e nominata da un'azione file_riconosciuto del salvataggio passa a verificata;
// un riconoscimento di una PB che non e' legacy, o che non e' piu' sul nodo riconosciuto, e' 422 CORPO_NON_VALIDO
// riconoscimento_non_applicabile; una verificata dichiarata dalla pagina senza il riconoscimento e' provenienza_dal_server.
func (vf *verifica3) provenienzeDecise(riconosciuti []Preassociazione3) ([]Preassociazione3, error) {
	ric := map[uuid.UUID]string{}
	for _, r := range riconosciuti {
		ric[r.AllegatoID] = r.ChiaveBozza
	}
	trovati := map[uuid.UUID]bool{}
	prima := map[uuid.UUID]Preassociazione3{}
	for _, p := range vf.prima.DocumentiPreassociati {
		prima[p.AllegatoID] = p
	}
	nodi := map[string]Nodo3{}
	for _, n := range vf.nuovo.Nodi {
		nodi[n.ChiaveBozza] = n
	}
	out := make([]Preassociazione3, 0, len(vf.nuovo.DocumentiPreassociati))
	registri := map[string]*importazione.Registro{}
	for i, x := range vf.nuovo.DocumentiPreassociati {
		p := decisa(prima, x.AllegatoID, x.ChiaveBozza)
		if x.Importazione != "" && !rimasta(prima, x.AllegatoID, x.ChiaveBozza) {
			campo := fmt.Sprintf("snapshot.documenti_preassociati[%d].importazione", i)
			reg, ok := registri[x.Importazione]
			if !ok {
				r, err := vf.registroDelProdotto(x.Importazione)
				if err != nil {
					return nil, err
				}
				reg, registri[x.Importazione] = r, r
			}
			v, cera := prima[x.AllegatoID]
			gia := cera && v.Importazione == x.Importazione
			impronta := x.Importazione
			if reg == nil {
				r, superato := vf.registroDiAdesso(x.Importazione, x.AllegatoID, nodi[x.ChiaveBozza])
				if superato {
					return nil, corpoMotivo(campo, MotivoPropostaNonCorrispondente,
						"il calcolo del motore e' cambiato dopo la lettura: ora propone l'allegato %s su un altro nodo, non su %s; si rileggono i suggerimenti (R1)",
						x.AllegatoID, x.ChiaveBozza)
				}
				if r != nil {
					reg, impronta, vf.daScrivere = r, r.Impronta, r
				}
			}
			switch {
			case reg != nil && PropostaNelRegistro(reg.Contenuto, nodi[x.ChiaveBozza], x.AllegatoID) && destinazioneUnica(reg.Contenuto, x.AllegatoID) != "":
				p.Provenienza, p.Importazione = ProvenienzaMotore, impronta
			case gia:
			case reg != nil && destinazioneUnica(reg.Contenuto, x.AllegatoID) != "":
				p.Provenienza, p.Importazione = ProvenienzaCorretta, impronta
			case reg == nil:
				return nil, corpoMotivo(campo, MotivoImportazioneSconosciuta, "il registro %s non c'e' fra quelli del prodotto in questa RFQ", x.Importazione)
			default:
				return nil, corpoMotivo(campo, MotivoPropostaNonCorrispondente,
					"il registro %s non propone l'allegato %s sul nodo %s: senza la proposta la preassociazione e' manuale (non si dichiara il registro)",
					x.Importazione, x.AllegatoID, x.ChiaveBozza)
			}
		}
		if ch, ok := ric[x.AllegatoID]; ok {
			trovati[x.AllegatoID] = true
			if ch != x.ChiaveBozza || p.Provenienza != ProvenienzaLegacy {
				return nil, corpoMotivo(fmt.Sprintf("snapshot.documenti_preassociati[%d]", i), MotivoRiconoscimentoNonApplicabile,
					"si riconosce solo una preassociazione legacy che resta sul suo nodo (allegato %s)", x.AllegatoID)
			}
			p.Verificata = true
		}
		if x.Verificata && !p.Verificata {
			return nil, corpoMotivo(fmt.Sprintf("snapshot.documenti_preassociati[%d].verificata", i), MotivoProvenienzaDelServer,
				"la verifica della preassociazione dell'allegato %s la decide il server con il riconoscimento esplicito (file_riconosciuto)", x.AllegatoID)
		}
		out = append(out, p)
	}
	for _, r := range riconosciuti {
		if !trovati[r.AllegatoID] {
			return nil, corpoMotivo("azioni", MotivoRiconoscimentoNonApplicabile,
				"il riconoscimento dell'allegato %s nomina una preassociazione che lo snapshot non ha", r.AllegatoID)
		}
	}
	return out, nil
}

// registroDiAdesso (G39-01, adesso3.go): il registro del calcolo di adesso per una PB che cita un registro che il database
// non ha: con la stessa impronta (la PB puo' essere stata spostata: «corretta»); con un'altra impronta (il calcolo e'
// cambiato dopo la lettura) solo se il registro di adesso propone l'allegato, univoco, proprio sul nodo della PB: il
// suggerimento accettato senza modifiche resta «motore» con il riferimento al calcolo che il server ha davanti. Se invece
// il registro di adesso lo propone, univoco, su un altro nodo, il suggerimento letto e' superato (superato vero): non e'
// l'ingegnere che l'ha spostato, quindi non e' «corretta» (rilievo minore 7 della revisione G39 parte A). nil altrimenti o
// senza il calcolo.
func (vf *verifica3) registroDiAdesso(impronta string, a uuid.UUID, nodo Nodo3) (r *importazione.Registro, superato bool) {
	return registroDiAdessoPer(vf.adesso.registro(), vf.thread, vf.prodotto.ComponenteID, impronta, a, nodo)
}

// registroDiAdessoPer: la regola di registroDiAdesso sul registro del calcolo di adesso r (nil = nessun calcolo), per il
// prodotto della RFQ; la usano R2 (verifica3) e R4 (provenienzeR4, con il calcolo di ?avvisi_motore=1).
func registroDiAdessoPer(r *importazione.Registro, thread, prodotto uuid.UUID, impronta string, a uuid.UUID, nodo Nodo3) (*importazione.Registro, bool) {
	if r == nil || r.Contenuto.ThreadID != thread.String() || r.Contenuto.ProdottoID != prodotto.String() {
		return nil, false
	}
	if r.Impronta == impronta {
		return r, false
	}
	if destinazioneUnica(r.Contenuto, a) == "" {
		return nil, false
	}
	if PropostaNelRegistro(r.Contenuto, nodo, a) {
		return r, false
	}
	return nil, true
}

// MotivoRiconoscimentoNonApplicabile: file_riconosciuto su una PB che non e' legacy o non resta sul suo nodo (R39-15).
const MotivoRiconoscimentoNonApplicabile = "riconoscimento_non_applicabile"

// registroDelProdotto: il registro con quell'impronta, del prodotto della bozza nella RFQ (nil se non c'e').
func (vf *verifica3) registroDelProdotto(impronta string) (*importazione.Registro, error) {
	if r, ok := vf.registri[impronta]; ok {
		return r, nil
	}
	if !reSha256.MatchString(impronta) {
		return nil, nil
	}
	rr, err := vf.q.ListImportazioniPerImpronte(vf.ctx, db.ListImportazioniPerImpronteParams{ThreadID: vf.thread,
		ProdottoID: vf.prodotto.ComponenteID, Impronte: []string{impronta}})
	if err != nil || len(rr) == 0 {
		return nil, err
	}
	reg, _, err := RileggiImportazione(db.GetDistintaImportazioneRow(rr[0]))
	if err != nil {
		return nil, err
	}
	return &reg, nil
}

// conProvenienzaNegliEventi: gli eventi file_preassociato (il dopo) e file_svincolato (il prima) del salvataggio con la
// provenienza e il registro della PB che nominano: la PB finale se e' sullo stesso nodo, altrimenti quella della
// versione precedente sullo stesso nodo, altrimenti la regola di una PB nuova senza registro dichiarato.
func conProvenienzaNegliEventi(eventi []Evento, prima Snapshot3, finali []Preassociazione3) {
	pp := map[uuid.UUID]Preassociazione3{}
	for _, p := range prima.DocumentiPreassociati {
		pp[p.AllegatoID] = p
	}
	ff := map[uuid.UUID]Preassociazione3{}
	for _, p := range finali {
		ff[p.AllegatoID] = p
	}
	della := func(a uuid.UUID, chiave string) Preassociazione3 {
		if f, ok := ff[a]; ok && f.ChiaveBozza == chiave {
			return f
		}
		return decisa(pp, a, chiave)
	}
	riscrivi := func(raw json.RawMessage) json.RawMessage {
		var m map[string]json.RawMessage
		if nullo(raw) || json.Unmarshal(raw, &m) != nil {
			return raw
		}
		var x struct {
			AllegatoID  uuid.UUID `json:"allegato_id"`
			ChiaveBozza string    `json:"chiave_bozza"`
		}
		if json.Unmarshal(raw, &x) != nil || x.AllegatoID == uuid.Nil {
			return raw
		}
		p := della(x.AllegatoID, x.ChiaveBozza)
		delete(m, "provenienza")
		delete(m, "importazione")
		if p.Provenienza != "" {
			b, _ := json.Marshal(p.Provenienza)
			m["provenienza"] = b
		}
		if p.Importazione != "" {
			b, _ := json.Marshal(p.Importazione)
			m["importazione"] = b
		}
		return canonico(m)
	}
	for i := range eventi {
		switch eventi[i].Azione {
		case AzFilePreassociato:
			eventi[i].Dopo = riscrivi(eventi[i].Dopo)
		case AzFileSvincolato:
			eventi[i].Prima = riscrivi(eventi[i].Prima)
		}
	}
}

// validaProvenienza: la forma della provenienza di una PB (ck_dbv_preassociazioni nel Go, per un 422 leggibile invece
// di un 23514): uno dei quattro valori o assente; il registro solo con motore o corretta, un'impronta di 64 cifre.
func validaProvenienza(campo string, p Preassociazione3) *Errore {
	if p.Provenienza != "" && !provenienzePB[p.Provenienza] {
		return corpo(campo+".provenienza", "la provenienza e' una fra motore, corretta, manuale e legacy")
	}
	if p.Verificata && p.Provenienza != ProvenienzaLegacy {
		return corpo(campo+".verificata", "verificata vale solo per una preassociazione legacy (R39-15)")
	}
	if p.Importazione != "" {
		if p.Provenienza != "" && p.Provenienza != ProvenienzaMotore && p.Provenienza != ProvenienzaCorretta {
			return corpo(campo+".importazione", "il registro d'origine c'e' solo con la provenienza motore o corretta")
		}
		if !reSha256.MatchString(strings.TrimSpace(p.Importazione)) || p.Importazione != strings.TrimSpace(p.Importazione) {
			return corpo(campo+".importazione", "l'impronta del registro e' di 64 cifre esadecimali minuscole")
		}
	}
	return nil
}
