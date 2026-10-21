package distintabozza

// Le PB iniziali del server e i suggerimenti dopo l'apertura (direttiva 39 §4 [U + T-39]; EM I.8; D-PB01 A, D-PB02 A,
// D-PB03 A, D-AR11 B [U]).
//
// D-PB01 A [U] supera, solo per le proposte univoche, la regola «nessuna versione del server aggiunge PB» (MAT §4.3 righe
// 10-13; EM §6.1 P-7): alla nascita della bozza (crea con la partenza step; da-conferma-1 per i file senza AR attiva) e
// nei comandi espliciti (cambia_sorgente, riallinea_proposta) il server scrive come PB iniziali le proposte documentali
// univoche del registro d'importazione della versione (associazioni_proposte[] delle voci), con provenienza «motore» e
// il registro d'origine (importazione, l'impronta: R39-02 [T-39]). La partenza solo_prodotto non ha registro e non ne
// scrive; vuota e registrato nemmeno.
//
// La regola, per ogni allegato proposto dal registro (ClassificaProposte, pura):
//   - D-AR11 B [U]: senza una sorgente della versione confermata da S8 e valida (confermabom.SorgenteValida) niente: il
//     motore non inizializza PB e non propone file per quel prodotto;
//   - proposta da piu' voci del registro = ambigua: resta un file «da associare», ne' PB ne' suggerimento (D-PB01 A);
//   - la voce senza un nodo della bozza (la sua casella), o il nodo escluso: niente (non c'e' dove metterla);
//   - l'occorrenza fuori dalla RFQ, non attiva, di una natura che non si preassocia, o lo STEP legato come sorgente del
//     finito (D-2 (c) [U]): niente;
//   - lo STEP candidato come sorgente di un prodotto finito della RFQ (direttiva 39 §5 [U]; G39-02 [U], 20/10 sera):
//     niente, ne' PB ne' suggerimento. Candidato = uno STEP che il Motore A riconosce candidato sorgente di un finito della
//     RFQ (stepcandidati3.go: StepCandidatiDelFinito, con il calcolo del motore della forma che importa; senza il calcolo, il
//     ripiego [T-39] ListStepCandidatiDelFinito), oppure uno STEP che il registro propone sul nodo di un finito (il
//     prodotto della bozza). Lo STEP del finito segue solo S8 (fonte_step_evento,
//     documento, copia NAS), mai una AR ordinaria: una PB «motore» sulla radice diventerebbe in R5 una AR conferma_bom, e
//     da li' S8 su quello STEP sarebbe rifiutata (gia_associata_altrove). Lo STEP di un particolare o di un sottoassieme
//     (proposta cad_3d con il suo codice, su un nodo non finito) resta una PB come un PDF (D-2 (c) [U]);
//   - il file e' gia' una PB della bozza: sullo stesso nodo resta com'e' (un cambio della fonte non riscrive il suo
//     registro d'origine, D-PB03 A); su un altro nodo e' un suggerimento pb_toccata;
//   - il file ha una AR attiva: sul componente del nodo niente (e' gia' associato), altrove suggerimento ar_attiva;
//   - il file ha solo AR chiuse (una persona ha deciso), o il giornale della bozza ha un gesto dell'ingegnere sul file
//     (preassociato, svincolato o lasciato, origine utente o step), salvo una PB «motore» svincolata da questo stesso
//     comando: suggerimento pb_toccata [T-39];
//   - il file e' preassociato in una bozza aperta di un altro prodotto, ha una copia senza AR su un altro componente
//     (pre-assegnazione legacy di un altro prodotto) o e' gia' un documento confermato di un altro componente:
//     suggerimento conflitto [T-39]; un documento confermato dello stesso componente: niente;
//   - altrimenti e' applicabile: alla nascita e nei comandi diventa una PB {allegato_id, chiave_bozza, provenienza:
//     motore, importazione}; nella lettura (R1) e' un suggerimento dopo_apertura (D-PB02 A: dopo l'apertura niente
//     modifiche silenziose).
// I suggerimenti stanno in preassociazioni_proposte[] (della risposta, dell'evento del server, di R1), mai in
// da_rivedere[] (NOMI (d) [T-39 dell'orchestratore]). Si accettano con un salvataggio R2 esplicito: la PB nuova con il
// campo importazione del suggerimento (provenienzeDelServer in salva3.go la verifica: motore, o 422
// proposta_non_corrispondente).

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
)

// Le provenienze di una PB (D-PB03 A [U]; ck_dbv_preassociazioni, ck_fra_provenienza_pb). Le stesse di
// associazioni.Provenienza*, ripetute qui perche' lo snapshot non dipende dal servizio delle AR.
const (
	ProvenienzaMotore   = "motore"
	ProvenienzaCorretta = "corretta"
	ProvenienzaManuale  = "manuale"
	ProvenienzaLegacy   = "legacy"
)

var provenienzePB = map[string]bool{ProvenienzaMotore: true, ProvenienzaCorretta: true, ProvenienzaManuale: true, ProvenienzaLegacy: true}

// I motivi di preassociazioni_proposte[] (EM I.8, forma [T-39 corsia contratto], R39-27).
const (
	PropostaPBToccata    = "pb_toccata"
	PropostaARAttiva     = "ar_attiva"
	PropostaConflitto    = "conflitto"
	PropostaDopoApertura = "dopo_apertura"
)

// PreassociazioneProposta3 e' un suggerimento del server: una proposta univoca del registro che la bozza non applica
// (EM I.8, R39-27). Importazione e' l'impronta del registro che la contiene (R39-02: lo snapshot cita i registri per
// impronta; la pagina l'accetta rimandando la PB con lo stesso campo).
type PreassociazioneProposta3 struct {
	AllegatoID   uuid.UUID `json:"allegato_id"`
	ChiaveBozza  string    `json:"chiave_bozza"`
	Provenienza  string    `json:"provenienza"`
	Importazione string    `json:"importazione,omitempty"`
	Motivo       string    `json:"motivo"`
}

// FattiPB sono i fatti del database che ClassificaProposte guarda (FattiDellaRfq li legge).
type FattiPB struct {
	// SorgenteValida: la sorgente della versione e' confermata da S8 e valida (D-AR11 B [U])
	SorgenteValida bool
	// Allegati: l'occorrenza (della RFQ, attiva, preassociabile, legata come sorgente)
	Allegati map[uuid.UUID]allegatoPreassociabile
	// ARAttiva: il componente della AR attiva; ARChiusa: il file ha AR chiuse
	ARAttiva map[uuid.UUID]uuid.UUID
	ARChiusa map[uuid.UUID]bool
	// AltreBozze: il file e' preassociato nella bozza aperta di un altro prodotto
	AltreBozze map[uuid.UUID]bool
	// CopiaSenzaAR: il componente della copia di una pre-assegnazione legacy aperta
	CopiaSenzaAR map[uuid.UUID]uuid.UUID
	// Documento: il componente del documento confermato con lo stesso contenuto
	Documento map[uuid.UUID]uuid.UUID
	// Visti: il giornale della bozza ha gia' un evento dell'ingegnere sul file (file_preassociato, file_svincolato,
	// file_lasciato, origine utente o step)
	Visti map[uuid.UUID]bool
	// Ricollocabili: le PB «motore» svincolate da questo stesso comando (il nodo e' uscito): possono tornare PB
	Ricollocabili map[uuid.UUID]bool
	// StepDelFinito: gli STEP candidati sorgente di un finito della RFQ (candidati di S8; StepCandidatiDelFinito, G39-02)
	StepDelFinito map[uuid.UUID]bool
}

// eStep: l'estensione di uno STEP (D-2 (c): lo STEP si riconosce dall'estensione dell'occorrenza).
func eStep(estensione string) bool {
	e := strings.ToLower(strings.TrimSpace(estensione))
	return e == "stp" || e == "step"
}

// nodoDiUnFinito: il nodo del prodotto della bozza, o un nodo di tipo finito.
func nodoDiUnFinito(s Snapshot3, n Nodo3) bool {
	return (n.ComponenteID != nil && *n.ComponenteID == s.ProdottoID) || n.Tipo == "finito"
}

// ProposteDelRegistro: per ogni allegato proposto da associazioni_proposte[] delle voci del registro, i rif delle voci
// che lo propongono (senza ripetizioni, in ordine).
func ProposteDelRegistro(c importazione.Contenuto) map[uuid.UUID][]string {
	out := map[uuid.UUID][]string{}
	for _, v := range c.Proposta.Nodi {
		for _, a := range allegatiProposti(v) {
			gia := false
			for _, r := range out[a] {
				gia = gia || r == v.Origine.Rif
			}
			if !gia {
				out[a] = append(out[a], v.Origine.Rif)
			}
		}
	}
	for a := range out {
		sort.Strings(out[a])
	}
	return out
}

// allegatiProposti: gli allegati di associazioni_proposte[] di una voce ({allegato_id, motivo, livello}).
func allegatiProposti(v importazione.Voce) []uuid.UUID {
	if len(v.AssociazioniProposte) == 0 {
		return nil
	}
	var aa []struct {
		AllegatoID string `json:"allegato_id"`
	}
	if json.Unmarshal(v.AssociazioniProposte, &aa) != nil {
		return nil
	}
	var out []uuid.UUID
	for _, a := range aa {
		if u, err := uuid.Parse(a.AllegatoID); err == nil && u != uuid.Nil {
			out = append(out, u)
		}
	}
	return out
}

// PropostaNelRegistro dice se il registro propone l'allegato sul nodo (una sua casella sulla fonte del registro):
// la regola con cui R2 riconosce un suggerimento accettato (provenienza motore).
func PropostaNelRegistro(c importazione.Contenuto, n Nodo3, allegato uuid.UUID) bool {
	fonte := c.Proposta.Fonte.AllegatoID
	for _, x := range n.Caselle {
		if x.AllegatoID.String() != fonte {
			continue
		}
		v, ok := c.VocePerRif(x.AllegatoID, x.Rif)
		if !ok {
			continue
		}
		for _, a := range allegatiProposti(v) {
			if a == allegato {
				return true
			}
		}
	}
	return false
}

// ClassificaProposte applica la regola del commento di testa: le PB applicabili (con provenienza motore e il registro) e
// i suggerimenti, in ordine di allegato. Pura.
func ClassificaProposte(s Snapshot3, reg *importazione.Registro, f FattiPB) ([]Preassociazione3, []PreassociazioneProposta3) {
	applicabili, proposte := []Preassociazione3{}, []PreassociazioneProposta3{}
	if reg == nil || !f.SorgenteValida {
		return applicabili, proposte
	}
	c := reg.Contenuto
	fonte, err := uuid.Parse(c.Proposta.Fonte.AllegatoID)
	if err != nil {
		return applicabili, proposte
	}
	chiaveDi := map[string]Nodo3{}
	for _, n := range s.Nodi {
		for _, x := range n.Caselle {
			if x.AllegatoID == fonte {
				chiaveDi[x.Rif] = n
			}
		}
	}
	pb := map[uuid.UUID]Preassociazione3{}
	for _, p := range s.DocumentiPreassociati {
		pb[p.AllegatoID] = p
	}
	proposti := ProposteDelRegistro(c)
	allegati := make([]uuid.UUID, 0, len(proposti))
	for a := range proposti {
		allegati = append(allegati, a)
	}
	sort.Slice(allegati, func(i, j int) bool { return allegati[i].String() < allegati[j].String() })
	for _, a := range allegati {
		rif := proposti[a]
		if len(rif) != 1 {
			continue // ambigua: resta da associare
		}
		n, ok := chiaveDi[rif[0]]
		if !ok || n.Op == OpEsclude {
			continue
		}
		occ := f.Allegati[a]
		if !occ.DellaRfq || !occ.Attivo || !occ.Preassociabile || occ.LegatoComeSorgente {
			continue
		}
		if f.StepDelFinito[a] || (eStep(occ.Estensione) && nodoDiUnFinito(s, n)) {
			continue // lo STEP del finito e' un candidato di S8, non un documento da preassociare
		}
		proposta := func(motivo string) {
			proposte = append(proposte, PreassociazioneProposta3{AllegatoID: a, ChiaveBozza: n.ChiaveBozza, Provenienza: ProvenienzaMotore,
				Importazione: reg.Impronta, Motivo: motivo})
		}
		if p, ok := pb[a]; ok {
			if p.ChiaveBozza != n.ChiaveBozza {
				proposta(PropostaPBToccata)
			}
			continue
		}
		if comp, ok := f.ARAttiva[a]; ok {
			if n.ComponenteID == nil || *n.ComponenteID != comp {
				proposta(PropostaARAttiva)
			}
			continue
		}
		if f.ARChiusa[a] || (f.Visti[a] && !f.Ricollocabili[a]) {
			proposta(PropostaPBToccata)
			continue
		}
		if comp, ok := f.Documento[a]; ok {
			if n.ComponenteID == nil || *n.ComponenteID != comp {
				proposta(PropostaConflitto)
			}
			continue
		}
		if comp, ok := f.CopiaSenzaAR[a]; ok && (n.ComponenteID == nil || *n.ComponenteID != comp) {
			proposta(PropostaConflitto)
			continue
		}
		if f.AltreBozze[a] {
			proposta(PropostaConflitto)
			continue
		}
		applicabili = append(applicabili, Preassociazione3{AllegatoID: a, ChiaveBozza: n.ChiaveBozza, Provenienza: ProvenienzaMotore,
			Importazione: reg.Impronta})
	}
	return applicabili, proposte
}

// FattiDellaRfq legge i fatti di ClassificaProposte per gli allegati proposti dal registro, nella transazione di chi
// chiama: la sorgente S8 della versione (sha256 vuoto = nessuna), le occorrenze, le AR, le PB delle altre bozze, le copie
// senza AR, i documenti confermati e, con una bozza, gli allegati gia' visti dal suo giornale fino alla versione data. Gli
// STEP candidati del finito con il ripiego (senza il calcolo del motore): FattiDellaRfqCon li prende dal motore.
func FattiDellaRfq(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, sha string, reg *importazione.Registro,
	bozza uuid.UUID, versione int) (FattiPB, error) {
	return FattiDellaRfqCon(ctx, q, thread, prodotto, sha, reg, bozza, versione, nil)
}

// FattiDellaRfqCon e' FattiDellaRfq con il riconoscimento degli STEP candidati del finito del Motore A (step; nil o non
// del motore = il ripiego, G39-02).
func FattiDellaRfqCon(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, sha string, reg *importazione.Registro,
	bozza uuid.UUID, versione int, step *StepCandidati) (FattiPB, error) {
	f := FattiPB{Allegati: map[uuid.UUID]allegatoPreassociabile{}, ARAttiva: map[uuid.UUID]uuid.UUID{}, ARChiusa: map[uuid.UUID]bool{},
		AltreBozze: map[uuid.UUID]bool{}, CopiaSenzaAR: map[uuid.UUID]uuid.UUID{}, Documento: map[uuid.UUID]uuid.UUID{},
		Visti: map[uuid.UUID]bool{}, Ricollocabili: map[uuid.UUID]bool{}, StepDelFinito: map[uuid.UUID]bool{}}
	if reg == nil || strings.TrimSpace(sha) == "" {
		return f, nil
	}
	st, err := confermabom.SorgenteValida(ctx, q, thread, prodotto, sha)
	if err != nil {
		return f, err
	}
	f.SorgenteValida = st.Valida
	if !f.SorgenteValida {
		return f, nil
	}
	proposti := ProposteDelRegistro(reg.Contenuto)
	if len(proposti) == 0 {
		return f, nil
	}
	ids := make([]uuid.UUID, 0, len(proposti))
	for a := range proposti {
		ids = append(ids, a)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	rr, err := q.ListAllegatiPreassociabili(ctx, db.ListAllegatiPreassociabiliParams{ThreadID: thread, Allegati: ids})
	if err != nil {
		return f, err
	}
	for _, r := range rr {
		f.Allegati[r.AllegatoID] = allegatoPreassociabile{DellaRfq: r.DellaRfq, Attivo: r.Attivo, Preassociabile: r.Preassociabile,
			LegatoComeSorgente: r.LegatoComeSorgente, Natura: r.Natura, Estensione: r.Estensione}
	}
	stepFinito, err := StepCandidatiDelFinito(ctx, q, thread, ids, step)
	if err != nil {
		return f, err
	}
	for a := range stepFinito.Prodotti {
		f.StepDelFinito[a] = true
	}
	ar, err := q.ListAssociazioniDegliAllegati(ctx, db.ListAssociazioniDegliAllegatiParams{ThreadID: thread, Allegati: ids})
	if err != nil {
		return f, err
	}
	for _, r := range ar {
		if r.Attiva {
			f.ARAttiva[r.AllegatoID] = r.ComponenteID
		} else {
			f.ARChiusa[r.AllegatoID] = true
		}
	}
	altre, err := q.ListPreassociazioniAltreBozze(ctx, db.ListPreassociazioniAltreBozzeParams{ThreadID: thread, ProdottoID: prodotto})
	if err != nil {
		return f, err
	}
	for _, r := range altre {
		f.AltreBozze[r.AllegatoID] = true
	}
	copie, err := q.ListCopieSenzaARDellaRfq(ctx, thread)
	if err != nil {
		return f, err
	}
	for _, r := range copie {
		f.CopiaSenzaAR[r.AllegatoID] = r.ComponenteID
	}
	docs, err := q.ListDocumentiConfermatiDegliAllegati(ctx, db.ListDocumentiConfermatiDegliAllegatiParams{ThreadID: thread, Allegati: ids})
	if err != nil {
		return f, err
	}
	for _, r := range docs {
		f.Documento[r.AllegatoID] = r.ComponenteID
	}
	if bozza != uuid.Nil {
		ee, err := q.ListDistintaBozzaEventi(ctx, db.ListDistintaBozzaEventiParams{BozzaID: bozza, ThreadID: thread, VersioneMax: int32(versione),
			DopoN: 0, Limite: 1 << 20})
		if err != nil {
			return f, err
		}
		for _, e := range ee {
			// gli eventi del server (nascita, cambi della fonte, riallineamenti) non sono un gesto dell'ingegnere
			if e.Origine == "server" {
				continue
			}
			var raw *json.RawMessage
			switch e.Azione {
			case AzFilePreassociato, AzFileLasciato:
				raw = e.Dopo
			case AzFileSvincolato:
				raw = e.Prima
			default:
				continue
			}
			if raw == nil {
				continue
			}
			var x struct {
				AllegatoID uuid.UUID `json:"allegato_id"`
			}
			if json.Unmarshal(*raw, &x) == nil && x.AllegatoID != uuid.Nil {
				f.Visti[x.AllegatoID] = true
			}
		}
	}
	return f, nil
}

// aggiungiPB mette le PB applicabili nello snapshot (in ordine di allegato, come lo scrittore).
func aggiungiPB(s *Snapshot3, pb []Preassociazione3) {
	if len(pb) == 0 {
		return
	}
	s.DocumentiPreassociati = append(s.DocumentiPreassociati, pb...)
	sort.Slice(s.DocumentiPreassociati, func(i, j int) bool {
		return s.DocumentiPreassociati[i].AllegatoID.String() < s.DocumentiPreassociati[j].AllegatoID.String()
	})
}

// PBInizialiDelServer: le PB iniziali e i suggerimenti di una versione costruita dal server (commento di testa), con i
// fatti letti nella transazione di chi chiama. sha e' lo sha256 della sorgente della versione; bozza e versione quelle
// del giornale da guardare (uuid.Nil alla nascita); ricollocabili le PB «motore» svincolate dal comando stesso; step gli
// STEP candidati del finito del calcolo del motore della forma (nil = il ripiego, G39-02).
func PBInizialiDelServer(ctx context.Context, q *db.Queries, thread, prodotto uuid.UUID, s *Snapshot3, reg *importazione.Registro, sha string,
	bozza uuid.UUID, versione int, ricollocabili map[uuid.UUID]bool, step *StepCandidati) ([]Preassociazione3, []PreassociazioneProposta3, error) {
	f, err := FattiDellaRfqCon(ctx, q, thread, prodotto, sha, reg, bozza, versione, step)
	if err != nil {
		return nil, nil, err
	}
	for a := range ricollocabili {
		f.Ricollocabili[a] = true
	}
	pb, proposte := ClassificaProposte(*s, reg, f)
	aggiungiPB(s, pb)
	return pb, proposte, nil
}

// ricollocabiliDi: gli allegati delle PB «motore» della versione di prima che il comando ha svincolato (vassoio).
func ricollocabiliDi(prima Snapshot3, vassoio []string) map[uuid.UUID]bool {
	out := map[uuid.UUID]bool{}
	prov := map[string]string{}
	for _, p := range prima.DocumentiPreassociati {
		prov[p.AllegatoID.String()] = p.Provenienza
	}
	for _, a := range vassoio {
		if prov[a] == ProvenienzaMotore {
			if u, err := uuid.Parse(a); err == nil {
				out[u] = true
			}
		}
	}
	return out
}

// eventiPBDelServer: gli eventi file_preassociato (origine server) delle PB iniziali di un comando esplicito, con la
// provenienza e il registro d'origine nel dopo (D-PB03 A: il giornale tiene la provenienza di ogni PB).
func eventiPBDelServer(pb []Preassociazione3) []eventoServer {
	out := make([]eventoServer, 0, len(pb))
	for _, p := range pb {
		out = append(out, eventoServer{Azione: AzFilePreassociato, ChiaveBozza: p.ChiaveBozza, Dopo: canonico(p)})
	}
	return out
}
