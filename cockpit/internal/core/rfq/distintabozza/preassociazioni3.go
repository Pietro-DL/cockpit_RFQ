package distintabozza

// Le preassociazioni del formato 3 per allegato_id (direttiva 35 §1.1 [U], DIR35-PB; blocco C-2; EM §3.6.2, §3.7): le
// verifiche di R2 contro il database sulle preassociazioni (PB) e sugli svincoli (scollega_documento) nuovi o cambiati
// nel salvataggio. Una PB e' {allegato_id, chiave_bozza} e non ha bisogno di una riga di documento_proposta (Prova 2
// della direttiva): conta l'occorrenza, non la classificazione.
//
// Per ogni PB nuova o spostata, nell'ordine:
//   - l'allegato e' della RFQ del percorso (allegato → messaggio → thread): 422 FUORI_RFQ;
//   - si preassocia: natura file o elemento_outlook, ne' contenitori ne' archivi (la regola del punto 1 di trg_fra_nasce,
//     scritta uguale in ListAllegatiPreassociabili): 422 CORPO_NON_VALIDO, motivo natura_non_preassociabile (INC-G6; il
//     nome del motivo e' una scelta [T] della corsia, A-07 della corsia contratto);
//   - non e' lo STEP legato come sorgente strutturale del finito (direttiva 39 §5, D-2 (c) [U]: gli altri STEP si
//     preassociano come un PDF; il predicato e' quello del database, allegato_sorgente_del_finito): 422
//     CORPO_NON_VALIDO, motivo legato_come_sorgente (EM I.10);
//   - non e' uno STEP candidato sorgente di un prodotto finito (G39-02 [U], 20/10 sera: lo STEP del finito non e' mai una PB,
//     neanche in una bozza manuale o «solo prodotto»; stepcandidati3.go, con il calcolo del motore di adesso se
//     l'adattatore lo prepara, altrimenti il ripiego [T-39]): 422 STEP_CANDIDATO_DEL_FINITO, con il rimedio S8. Con il
//     motore, la PB su uno STEP candidato NON univoco verso una parte o un sottoassieme si salva: e' la scelta
//     dell'utente (G39-06 [U]; StepCandidati.BloccaPB, giro G39-ter); verso un prodotto finito resta il 422;
//   - e' attivo: ne' in Altri file ne' scartato, anche col vecchio «Scarta il file» (il punto 4 di trg_fra_nasce):
//     422 FILE_NON_ATTIVO.
// Per ogni svincolo nuovo o cambiato: l'allegato e' della RFQ (FUORI_RFQ) e la AR attiva di quella coppia (allegato,
// componente) c'e' in file_rfq_associazione (EM §3.6.2): altrimenti 422 CORPO_NON_VALIDO, motivo
// svincolo_senza_associazione, sul campo dello svincolo. Una copia di documento_proposta senza AR (pre-assegnazione
// legacy) non si svincola da qui (sospeso, INC-S2 e INC-S3). Una PB o uno svincolo gia' salvati e scaduti dopo passano:
// li segnalano R1 e R4 (EM §6.4, una regola sola per R2).

import (
	"fmt"
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/platform/db"
)

// I motivi di CORPO_NON_VALIDO delle preassociazioni e degli svincoli (direttiva 35, C-2).
const (
	// MotivoNaturaNonPreassociabile: l'occorrenza di una PB nuova o spostata non e' un file ne' un elemento di Outlook, o
	// e' un contenitore o un archivio.
	MotivoNaturaNonPreassociabile = "natura_non_preassociabile"
	// MotivoLegatoComeSorgente: l'occorrenza di una PB nuova o spostata e' lo STEP legato come sorgente strutturale di un
	// prodotto finito (direttiva 39 §5 [U]; EM I.10): lo segue S8, non una PB.
	MotivoLegatoComeSorgente = "legato_come_sorgente"
	// MotivoProvenienzaDelServer: la pagina mette o cambia la provenienza (o il registro d'origine) di una PB: li decide
	// il server (direttiva 39 §4, R39-24; nome [T-39] della corsia).
	MotivoProvenienzaDelServer = "provenienza_dal_server"
	// MotivoSvincoloSenzaAssociazione: lo svincolo nuovo o cambiato non cita una AR attiva (EM §3.6.2).
	MotivoSvincoloSenzaAssociazione = "svincolo_senza_associazione"
)

// preassociazioniESvincoli: le verifiche del commento di testa, sulle PB (pre, per posto nello snapshot nuovo) e sugli
// svincoli fra le decisioni nuove o cambiate.
func (vf *verifica3) preassociazioniESvincoli(pre map[int]Preassociazione3, decisioni map[int]Decisione3) error {
	type svincoloNuovo struct {
		i     int
		d     Decisione3
		campo string
	}
	var svincoli []svincoloNuovo
	ids := make([]uuid.UUID, 0, len(pre))
	for _, p := range pre {
		ids = append(ids, p.AllegatoID)
	}
	for i, d := range decisioni {
		if d.Tipo == DecisioneScollegaDocumento && d.AllegatoID != nil {
			svincoli = append(svincoli, svincoloNuovo{i, d, fmt.Sprintf("snapshot.decisioni[%d]", i)})
			ids = append(ids, *d.AllegatoID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	allegati, err := leggiAllegatiPreassociabili(vf, ids)
	if err != nil {
		return err
	}
	pbIDs := make([]uuid.UUID, 0, len(pre))
	for _, p := range pre {
		pbIDs = append(pbIDs, p.AllegatoID)
	}
	candidati, err := StepCandidatiDelFinito(vf.ctx, vf.q, vf.thread, pbIDs, vf.adesso.step())
	if err != nil {
		return err
	}
	posti := make([]int, 0, len(pre))
	for i := range pre {
		posti = append(posti, i)
	}
	sort.Ints(posti)
	for _, i := range posti {
		p := pre[i]
		campo := fmt.Sprintf("snapshot.documenti_preassociati[%d].allegato_id", i)
		a := allegati[p.AllegatoID]
		switch {
		case !a.DellaRfq:
			return fuori(campo, "l'allegato %s non e' un file di questa RFQ", p.AllegatoID)
		case !a.Preassociabile:
			natura := a.Natura
			if natura == "" {
				natura = "-"
			}
			return corpoMotivo(campo, MotivoNaturaNonPreassociabile,
				"l'allegato %s (natura %s, estensione %s) non si preassocia: solo file ed elementi di Outlook, esclusi i contenitori",
				p.AllegatoID, natura, estensioneODash(a.Estensione))
		case a.LegatoComeSorgente:
			return corpoMotivo(campo, MotivoLegatoComeSorgente,
				"l'allegato %s e' lo STEP legato come sorgente strutturale di un prodotto finito: si cambia con la conferma della sorgente (S8), non con una preassociazione",
				p.AllegatoID)
		case candidati.BloccaPB(p.AllegatoID, NodoVersoUnFinito(vf.nuovo, p.ChiaveBozza, vf.st)):
			id := p.AllegatoID
			return &Errore{Codice: CodiceStepCandidatoDelFinito, Campo: campo, AllegatoID: &id,
				Messaggio: fmt.Sprintf("l'allegato %s e' lo STEP candidato sorgente di un prodotto finito: non si preassocia come documento; "+
					"si conferma come sorgente del prodotto (S8)", p.AllegatoID)}
		case !a.Attivo:
			return &Errore{Codice: CodiceFileNonAttivo, Campo: campo,
				Messaggio: fmt.Sprintf("l'allegato %s e' in Altri file o scartato: prima si riattiva", p.AllegatoID)}
		}
	}
	if len(svincoli) == 0 {
		return nil
	}
	sort.Slice(svincoli, func(a, b int) bool { return svincoli[a].i < svincoli[b].i })
	cercati := make([]uuid.UUID, 0, len(svincoli))
	for _, s := range svincoli {
		if !allegati[*s.d.AllegatoID].DellaRfq {
			return fuori(s.campo+".allegato_id", "l'allegato %s non e' un file di questa RFQ", *s.d.AllegatoID)
		}
		cercati = append(cercati, *s.d.AllegatoID)
	}
	attive, err := vf.q.ListAssociazioniAttiveDegliAllegati(vf.ctx, db.ListAssociazioniAttiveDegliAllegatiParams{ThreadID: vf.thread, Allegati: cercati})
	if err != nil {
		return err
	}
	componenteDi := map[uuid.UUID]uuid.UUID{}
	for _, r := range attive {
		componenteDi[r.AllegatoID] = r.ComponenteID
	}
	for _, s := range svincoli {
		c, ok := componenteDi[*s.d.AllegatoID]
		if !ok || s.d.ComponenteID == nil || c != *s.d.ComponenteID {
			return corpoMotivo(s.campo, MotivoSvincoloSenzaAssociazione,
				"lo svincolo dell'allegato %s dal componente %s non cita un'associazione registrata attiva: si svincola solo una AR attiva (EM §3.6.2)",
				*s.d.AllegatoID, uuidODash(s.d.ComponenteID))
		}
	}
	return nil
}

// allegatoPreassociabile: una riga di ListAllegatiPreassociabili.
type allegatoPreassociabile struct {
	DellaRfq, Attivo, Preassociabile, LegatoComeSorgente bool
	Natura, Estensione                                   string
}

// leggiAllegatiPreassociabili: le occorrenze date, per allegato (senza ripetizioni nella query).
func leggiAllegatiPreassociabili(vf *verifica3, ids []uuid.UUID) (map[uuid.UUID]allegatoPreassociabile, error) {
	visti := map[uuid.UUID]bool{}
	unici := make([]uuid.UUID, 0, len(ids))
	for _, u := range ids {
		if !visti[u] {
			visti[u] = true
			unici = append(unici, u)
		}
	}
	rr, err := vf.q.ListAllegatiPreassociabili(vf.ctx, db.ListAllegatiPreassociabiliParams{ThreadID: vf.thread, Allegati: unici})
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]allegatoPreassociabile, len(rr))
	for _, r := range rr {
		out[r.AllegatoID] = allegatoPreassociabile{DellaRfq: r.DellaRfq, Attivo: r.Attivo, Preassociabile: r.Preassociabile,
			LegatoComeSorgente: r.LegatoComeSorgente, Natura: r.Natura, Estensione: r.Estensione}
	}
	return out, nil
}

func estensioneODash(e string) string {
	if e == "" {
		return "-"
	}
	return e
}

func uuidODash(u *uuid.UUID) string {
	if u == nil {
		return "-"
	}
	return u.String()
}
