package propostabom

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// L'identità proposta di un nodo, con le due proiezioni e il motivo della rev (EM §3.8; U2 §4.2 punto 2), e la traccia
// dei nodi fuori per decisione (U2 §4.4, Q13 [U] e [UR]; U2-P22). Tutto dalla stessa fotografia del calcolo: la
// traccia viene dalle righe legacy del thread (componente_proposta, relazione_proposta), che il caricatore legge già e
// che l'adattatore riceve nel suo ingresso (Ingresso.Foto), senza toccare il caricatore congelato.

// I motivi di una rev proposta vuota (motivo_rev; U2 §4.2 punto 2, prova A-32): con rev_proposta vuota il motivo c'è
// sempre.
//   - revisione_assente: la lettura scelta non ha una revisione, e nessun riferimento del nodo ne porta una (o non c'è
//     una lettura scelta);
//   - revisione_non_letta, revisione_non_ammissibile: MotivoRevNonLetta, MotivoRevNonAmmissibile (la revisione della
//     lettura scelta c'è, ma non si propone);
//   - revisione_fra_i_riferimenti: la lettura scelta non ha una revisione, ma un riferimento del nodo ne porta una (un
//     candidato di revisione, la correzione del cartiglio di un 2D attivo, una lettura con il ruolo riferimento). Con la
//     base B di V1-Q26 la rev si propone solo dalla lettura dello STEP, quindi resta vuota e il motivo dice dove sta.
const (
	MotivoRevAssente         = "revisione_assente"
	MotivoRevFraIRiferimenti = "revisione_fra_i_riferimenti"
	OrigineLegacyScartaNodo  = "scarta_nodo"
	OrigineLegacyConfermaAlb = "conferma_albero"
	OrigineLegacyArcoTolto   = "arco_tolto"
	MancanteQuando           = "quando"
	MancanteMotivo           = "motivo"
	MancanteArchiTolti       = "archi_tolti"
	statoRigaLegacyScartata  = "scartata"
	formatoQuandoDelRegistro = time.RFC3339Nano
)

// identitaDelNodo: l'identità proposta di un nodo e le sue proiezioni, come le scrivono la voce e il nodo fuori per
// decisione.
type identitaDelNodo struct {
	scelta         *motorea.LetturaCodice
	identita       json.RawMessage
	codice, rev    string
	motivoProposto json.RawMessage
	motivoRev      json.RawMessage
}

// identitaProposta: la lettura scelta dalla catena del codice (CatenaCodice.Forma, mai IdentitaNodo), le due
// proiezioni di importazione-1 e il motivo della rev, sempre presente con la rev vuota.
func (b *costruttore) identitaProposta(n ancoraggio.NodoProposto, ns ancoraggio.NodoStruttura, lt *lettore) identitaDelNodo {
	var out identitaDelNodo
	if n.Codice.Lettura != "" {
		for i := range ns.Letture {
			if ns.Letture[i].ID == n.Codice.Lettura {
				out.scelta = &ns.Letture[i]
				break
			}
		}
	}
	mr := ""
	if s := out.scelta; s != nil {
		out.identita = raw(lt.lettura(*s))
		b.nelVoci[lt.chiave(s.ID)] = true
		cod, motivo := ProiezioneCodice(s.Forma, lt.unita[s.UnitaID].Testo)
		out.codice, out.motivoProposto = cod, raw(motivo)
		out.rev, mr = ProiezioneRev(s.Forma)
	} else {
		motivo := n.Codice.MotivoLettura
		if motivo == "" {
			motivo = ancoraggio.MotivoLetturaAssente
		}
		out.motivoProposto = raw(motivo)
	}
	if out.rev == "" && mr == "" {
		mr = MotivoRevAssente
		if b.revisioneFraIRiferimenti(n, ns) {
			mr = MotivoRevFraIRiferimenti
		}
	}
	if out.rev == "" {
		out.motivoRev = raw(mr)
	}
	return out
}

// revisioneFraIRiferimenti: un riferimento del nodo porta una revisione (un candidato di revisione da un'occorrenza
// attiva o da una mail, la correzione del cartiglio di un 2D attivo con la revisione, una lettura con il ruolo
// riferimento con una revisione). Non conta le evidenze lasciate fuori: quelle le conta già la nota
// importazione.evidenze_file_non_attivi.
func (b *costruttore) revisioneFraIRiferimenti(n ancoraggio.NodoProposto, ns ancoraggio.NodoStruttura) bool {
	for _, cr := range n.Codice.Identita.CandidatiRevisione {
		if cr.AllegatoID == nil || b.attiva(*cr.AllegatoID) {
			return true
		}
	}
	for _, cd := range n.Codice.Documentale {
		if cd.Correzione != nil && cd.Correzione.Revisione != nil && b.attiva(cd.AllegatoID) {
			return true
		}
	}
	_, rr := ancoraggio.DividiRiferimenti(ns.Letture)
	for _, l := range rr {
		if l.Forma.Revisione != nil {
			return true
		}
	}
	return false
}

// ---- la traccia dei nodi fuori per decisione ----

// fotoDelThread: le righe legacy del thread nella fotografia del calcolo, indicizzate.
type fotoDelThread struct {
	righe      map[uuid.UUID]*fotorfq.RigaComponenteProposta
	archiTolti map[[2]string][]*fotorfq.RigaRelazioneProposta // per (padre, figlio) rif, le righe scartate da una persona
	tenuti     map[[2]string]bool                             // per (padre, figlio) rif, un'altra riga che tiene l'arco
}

// indicizzaFoto: il thread dell'esito nella fotografia; senza (Ingresso.Foto nil, o il thread non c'è) un difetto
// dell'adattatore, perché la traccia di Q13 non si ricostruisce.
func (b *costruttore) indicizzaFoto() error {
	if b.in.Foto == nil {
		return difetto("ingresso", "l'adattatore vuole la fotografia del calcolo (Ingresso.Foto): la traccia dei nodi fuori per decisione viene da lì (U2 §4.4)")
	}
	var t *fotorfq.Thread
	for i := range b.in.Foto.Thread {
		if b.in.Foto.Thread[i].ID == b.et.ThreadID {
			t = &b.in.Foto.Thread[i]
			break
		}
	}
	if t == nil {
		return difetto("ingresso", "la fotografia non ha il thread dell'esito")
	}
	f := &fotoDelThread{righe: map[uuid.UUID]*fotorfq.RigaComponenteProposta{}, archiTolti: map[[2]string][]*fotorfq.RigaRelazioneProposta{},
		tenuti: map[[2]string]bool{}}
	for i := range t.RigheComponenteProposta {
		r := &t.RigheComponenteProposta[i]
		f.righe[r.ID] = r
	}
	sha := map[uuid.UUID]string{}
	for _, a := range t.Allegati {
		if a.Sha256 != nil {
			sha[a.ID] = *a.Sha256
		}
	}
	for i := range t.RigheRelazioneProposta {
		r := &t.RigheRelazioneProposta[i]
		s := sha[r.AllegatoID]
		if s == "" || r.PadreChiave == "" || r.FiglioChiave == "" {
			continue
		}
		k := [2]string{ancoraggio.RifNodo(s, r.PadreChiave), ancoraggio.RifNodo(s, r.FiglioChiave)}
		if r.Stato == statoRigaLegacyScartata && r.DecisoDa != nil {
			f.archiTolti[k] = append(f.archiTolti[k], r)
		} else {
			f.tenuti[k] = true
		}
	}
	b.foto = f
	return nil
}

// traccia: la traccia di un nodo fuori per decisione (U2 §4.4). Per un nodo tolto la riga legacy decisa del nodo: chi,
// quando, la nota come motivo, il gesto d'origine (conferma_albero con il segno di «Conferma l'albero», altrimenti
// scarta_nodo); per un nodo non raggiunto gli archi tolti da una persona che lo staccano dalla radice. I campi che lo
// storico non ha vanno in mancanti, mai ricostruiti ([UR] Q13).
func (b *costruttore) traccia(n ancoraggio.NodoProposto, x *importazione.FuoriPerDecisione) {
	var mancanti []string
	if x.Segno == importazione.SegnoTolto {
		x.OrigineLegacy = raw(OrigineLegacyScartaNodo)
		var r *fotorfq.RigaComponenteProposta
		if n.RigaDecisa != nil {
			r = b.foto.righe[n.RigaDecisa.ID]
			if n.RigaDecisa.DecisoDa != nil {
				x.Chi = raw(n.RigaDecisa.DecisoDa.String())
			}
		}
		var quando *time.Time
		var nota *string
		if r != nil {
			quando, nota = r.DecisoIl, r.Nota
			if a := r.Albero; a != nil && a.Nodo != "" {
				x.OrigineLegacy = raw(OrigineLegacyConfermaAlb)
				x.SegnoAlbero = raw(map[string]any{"da": a.Da.String(), "il": a.Il, "firma": a.Firma, "nodo": a.Nodo})
			}
		}
		mancanti = append(mancanti, b.quandoEMotivo(func(k string, v json.RawMessage) {
			switch k {
			case MancanteQuando:
				x.Quando = v
			case MancanteMotivo:
				x.Motivo = v
			}
		}, quando, nota)...)
	} else {
		x.OrigineLegacy = raw(OrigineLegacyArcoTolto)
		archi := b.archiCheStaccano(n.Rif)
		if len(archi) == 0 {
			mancanti = append(mancanti, MancanteArchiTolti)
		} else {
			x.ArchiTolti = raw(archi)
		}
	}
	if len(mancanti) > 0 {
		x.Mancanti = raw(mancanti)
	}
}

// quandoEMotivo scrive quando e motivo che lo storico ha e restituisce i nomi di quelli che non ha.
func (b *costruttore) quandoEMotivo(scrivi func(string, json.RawMessage), quando *time.Time, nota *string) []string {
	var mancanti []string
	if quando != nil {
		scrivi(MancanteQuando, raw(quando.UTC().Format(formatoQuandoDelRegistro)))
	} else {
		mancanti = append(mancanti, MancanteQuando)
	}
	if nota != nil && strings.TrimSpace(*nota) != "" {
		scrivi(MancanteMotivo, raw(*nota))
	} else {
		mancanti = append(mancanti, MancanteMotivo)
	}
	return mancanti
}

// archiCheStaccano: gli archi della struttura del gruppo tolti da una persona (righe legacy scartate con chi le ha
// decise, nessun'altra riga che tiene l'arco: la regola della valutazione, VA/bom.go) che entrano nel nodo o in un suo
// antenato del grafo del file, cioè quelli che lo staccano dalla radice; in ordine di (padre, figlio). Per ognuno chi,
// quando, motivo della riga (quella dell'occorrenza del gruppo, se c'è, altrimenti la prima per allegato) e i mancanti.
func (b *costruttore) archiCheStaccano(rif string) []any {
	padri := map[string][]string{}
	for _, a := range b.st.Archi {
		padri[a.Figlio] = append(padri[a.Figlio], a.Padre)
	}
	antenati := map[string]bool{rif: true}
	coda := []string{rif}
	for len(coda) > 0 {
		x := coda[0]
		coda = coda[1:]
		for _, p := range padri[x] {
			if !antenati[p] {
				antenati[p] = true
				coda = append(coda, p)
			}
		}
	}
	var chiavi [][2]string
	for _, a := range b.st.Archi {
		k := [2]string{a.Padre, a.Figlio}
		if !antenati[a.Figlio] || b.foto.tenuti[k] || len(b.foto.archiTolti[k]) == 0 {
			continue
		}
		chiavi = append(chiavi, k)
	}
	sort.Slice(chiavi, func(i, j int) bool {
		if chiavi[i][0] != chiavi[j][0] {
			return chiavi[i][0] < chiavi[j][0]
		}
		return chiavi[i][1] < chiavi[j][1]
	})
	var out []any
	visti := map[[2]string]bool{}
	for _, k := range chiavi {
		if visti[k] {
			continue
		}
		visti[k] = true
		righe := append([]*fotorfq.RigaRelazioneProposta(nil), b.foto.archiTolti[k]...)
		sort.SliceStable(righe, func(i, j int) bool {
			ai, aj := righe[i].AllegatoID == b.st.AllegatoID, righe[j].AllegatoID == b.st.AllegatoID
			if ai != aj {
				return ai
			}
			return righe[i].AllegatoID.String() < righe[j].AllegatoID.String()
		})
		r := righe[0]
		m := map[string]any{"padre": k[0], "figlio": k[1], "chi": r.DecisoDa.String()}
		mancanti := b.quandoEMotivo(func(c string, v json.RawMessage) { m[c] = v }, r.DecisoIl, r.Nota)
		if a := r.Albero; a != nil {
			m["segno_albero"] = map[string]any{"da": a.Da.String(), "il": a.Il, "firma": a.Firma}
		}
		if len(mancanti) > 0 {
			m["mancanti"] = mancanti
		}
		out = append(out, m)
	}
	return out
}
