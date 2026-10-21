package propostabom

import (
	"encoding/json"
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/core/valutazione"
)

// Le voci del registro (EM §3.8, «Voce di un nodo») e i nodi fuori per decisione.
//
// Un nodo della struttura del gruppo è una voce di proposta.nodi[] se è la radice del gruppo, oppure se il motore lo
// raggiunge senza gli archi tolti da una persona (è fra i nodi della BOM di lavoro o della struttura candidata) e non è
// tolto da una persona (la sua riga legacy decisa senza un componente, con chi l'ha decisa: VA/completezza_thread.go,
// il predicato del 2D previsto). Altrimenti sta in proposta.fuori_per_decisione[], con il segno «tolto» o «non_raggiunto»
// e la traccia che l'esito ha (chi ha deciso la riga). Le due liste sono disgiunte: è la regola di forma del DTO del
// registro (importazione, P-24 della corsia persistenza). La radice non è mai fuori per decisione: P-2 la porta sempre
// al prodotto (EM §6.1).

// voci: le voci e i nodi fuori per decisione, nell'ordine della struttura (la forma canonica del registro le riordina).
func (b *costruttore) voci() ([]importazione.Voce, []importazione.FuoriPerDecisione, error) {
	var voci []importazione.Voce
	var fuori []importazione.FuoriPerDecisione
	for _, n := range b.st.Nodi {
		ns, ok := b.nodiSF[n.Rif]
		if !ok {
			return nil, nil, difetto("proposta.nodi", "il nodo %s della struttura non è nel grafo del file", n.Rif)
		}
		nb := b.nodiBOM[n.Rif]
		tolto := toltoDaPersona(n)
		radice := n.Rif == b.st.Radice
		if !radice && (nb == nil || tolto) {
			x := importazione.FuoriPerDecisione{Voce: vocePer(b.st.AllegatoID, n.Rif), Segno: importazione.SegnoNonRaggiunto}
			if tolto {
				x.Segno = importazione.SegnoTolto
			}
			b.traccia(n, &x)
			id := b.identitaProposta(n, ns, b.lettoreDi(b.st.AllegatoID))
			x.IdentitaProposta, x.CodiceProposto, x.MotivoProposto = id.identita, id.codice, id.motivoProposto
			x.RevProposta, x.MotivoRev = id.rev, id.motivoRev
			fuori = append(fuori, x)
			continue
		}
		voci = append(voci, b.voce(n, ns, nb, tolto))
	}
	return voci, fuori, nil
}

// toltoDaPersona: il nodo ha una riga decisa da una persona senza un componente deciso (lo stesso predicato della
// valutazione per il nodo tolto: VA/completezza_thread.go:465-467).
func toltoDaPersona(n ancoraggio.NodoProposto) bool {
	return n.Decisione == nil && n.RigaDecisa != nil && n.DecisoDaPersona
}

// vocePer: la chiave di una voce, n:<allegato_id>:<rif>.
func vocePer(allegato uuid.UUID, rif string) string { return "n:" + allegato.String() + ":" + rif }

// voce: la voce di un nodo nel registro.
func (b *costruttore) voce(n ancoraggio.NodoProposto, ns ancoraggio.NodoStruttura, nb *valutazione.NodoBOM, tolto bool) importazione.Voce {
	st := b.st
	lt := b.lettoreDi(st.AllegatoID)
	raggiunto, deciso := nb != nil, n.Decisione != nil
	v := importazione.Voce{
		Voce:          vocePer(st.AllegatoID, n.Rif),
		Origine:       importazione.OrigineVoce{AllegatoID: st.AllegatoID.String(), Sha256: st.Sha256, Chiave: ns.Chiave, Rif: n.Rif},
		EntitaID:      raw(n.EntitaID),
		Padri:         raw(stringhe(n.Padri)),
		Foglia:        raw(b.foglia(n.Rif)),
		RadiceDelFile: raw(b.radiceDelFile(n.Rif)),
		Leggibile:     raw(n.Leggibile),
		StatoMotore:   importazione.StatoMotore{Raggiunto: &raggiunto, Tolto: &tolto, Deciso: &deciso},
		Grezzi:        raw(b.grezzi(n, ns)),
		MotivoLettura: raw(n.Codice.MotivoLettura),
		Segnali:       rawSeNonVuoto(n.Segnali),
	}

	// l'identità proposta: la lettura scelta dalla catena del codice (CatenaCodice.Forma, mai IdentitaNodo), le due
	// proiezioni e il motivo della rev, sempre con la rev vuota (U2 §4.2 punto 2)
	id := b.identitaProposta(n, ns, lt)
	scelta := id.scelta
	v.IdentitaProposta, v.CodiceProposto, v.MotivoProposto = id.identita, id.codice, id.motivoProposto
	v.RevProposta, v.MotivoRev = id.rev, id.motivoRev
	if n.Codice.Proposto != "" || n.Codice.MotivoProposto != "" {
		v.CodiceComposto = raw(map[string]any{"testo": n.Codice.Proposto, "motivo": n.Codice.MotivoProposto})
	}
	v.Alternative = rawSeNonVuoto(b.alternative(n, ns, scelta, lt))
	v.Riferimenti = rawSeNonVuoto(b.riferimenti(n, ns, scelta, lt))
	if m := n.Codice.Manuale; m != nil {
		cm := map[string]any{"codice": m.Codice}
		if m.Rev != nil {
			cm["rev"] = *m.Rev
		}
		if n.RigaLegacy != nil {
			cm["riga_legacy"] = n.RigaLegacy.ID.String()
		}
		v.CodiceManualeLegacy = raw(cm)
	}
	v.Attributi = rawSeNonVuoto(b.attributi(n, lt))
	if nb != nil && nb.Tipo != "" {
		tp := map[string]any{"valore": nb.Tipo, "origine": string(nb.OrigineTipo)}
		if c := nb.Classe; c != nil {
			tp["classe"] = map[string]any{"categoria": c.Categoria, "descrizione": c.Descrizione, "fonte": c.Fonte, "valore": c.Valore}
		}
		v.TipoProposto = raw(tp)
	}
	if d := n.Decisione; d != nil {
		de := map[string]any{"componente_id": d.ComponenteID.String(), "codice": d.Codice}
		if d.Rev != nil {
			de["rev"] = *d.Rev
		}
		v.DecisioneEsistente = raw(de)
	}
	if n.AbbinamentoPerBase != nil {
		v.AbbinamentoProposto = raw(n.AbbinamentoPerBase)
	}
	v.AssociazioniProposte = rawSeNonVuoto(b.associazioniProposte(n.Rif))
	v.FabbisogniPrevisti = rawSeNonVuoto(b.fabbisogni(n.Rif))
	switch {
	case n.RigaDecisa != nil:
		v.RigaLegacy = raw(map[string]any{"id": n.RigaDecisa.ID.String(), "stato": n.RigaDecisa.Stato, "decisa_da_persona": n.DecisoDaPersona})
	case n.RigaLegacy != nil:
		v.RigaLegacy = raw(map[string]any{"id": n.RigaLegacy.ID.String(), "stato": "aperta", "decisa_da_persona": false})
	}
	return v
}

// foglia: nessun arco dei fatti del file parte dal nodo (lo stesso criterio del segnale LS-02: la foglia nel file).
func (b *costruttore) foglia(rif string) bool {
	for _, a := range b.sf.Archi {
		if a.Padre == rif {
			return false
		}
	}
	return true
}

func (b *costruttore) radiceDelFile(rif string) bool {
	for _, r := range b.sf.Radici {
		if r == rif {
			return true
		}
	}
	return false
}

// grezzi: i campi del PRODUCT byte per byte: l'id e il nome (i primi, come nella catena del codice), la descrizione
// (l'unità della descrizione dell'entità nel documento) e le formazioni (rev_grezza), che non sono revisioni.
func (b *costruttore) grezzi(n ancoraggio.NodoProposto, ns ancoraggio.NodoStruttura) map[string]any {
	g := map[string]any{"formazioni": stringhe(ns.Formazioni)}
	for _, x := range ns.Grezzi {
		switch x.Campo {
		case ancoraggio.CampoGrezzoID:
			if _, ok := g["id"]; !ok {
				g["id"] = x.Testo
			}
		case ancoraggio.CampoGrezzoNome:
			if _, ok := g["nome"]; !ok {
				g["nome"] = x.Testo
			}
		}
	}
	if f := b.file[b.st.AllegatoID]; f != nil {
		for _, u := range f.Documento.Unita {
			if u.EntitaID == n.EntitaID && u.Selettore.Campo.Variante == evidenze.VarianteStep && u.Selettore.Campo.Valore == campoDescrizione {
				g["descrizione"] = u.Testo
				break
			}
		}
	}
	return g
}

// campoDescrizione: il campo dell'unità della descrizione di un PRODUCT nel documento di uno STEP (mappatura-step-1),
// ripetuto qui come in valutazione, perché estrazione non lo esporta.
const campoDescrizione = "descrizione"

// alternative: le altre letture d'identità delle unità id e nome (EM §3.8, «alternative[]»): con una lettura scelta le
// letture compatibili non scelte, senza la lettura scelta, i riferimenti e le letture con la funzione «relazione»; con
// letture discordanti tutte le letture d'identità, perché nessuna si sceglie. Il motivo di ogni alternativa
// (concorde_non_scelta, discordante) si ricava da motivo_lettura della voce: dentro la lettura non si scrive, perché la
// stessa lettura deve avere gli stessi byte in ogni punto del registro.
func (b *costruttore) alternative(n ancoraggio.NodoProposto, ns ancoraggio.NodoStruttura, scelta *motorea.LetturaCodice, lt *lettore) []any {
	if scelta == nil && n.Codice.MotivoLettura != ancoraggio.MotivoLettureDiscordanti {
		return nil
	}
	identita, _ := ancoraggio.DividiRiferimenti(ns.Letture)
	var out []any
	for _, l := range identita {
		if (scelta != nil && l.ID == scelta.ID) || (scelta != nil && l.Funzione == motorea.FunzRelazione) {
			continue
		}
		out = append(out, lt.lettura(l))
		b.nelVoci[lt.chiave(l.ID)] = true
	}
	return out
}

// riferimenti: i riferimenti della voce (EM §3.8, «riferimenti[]»), mai identità per implicazione (GP-1 §5, regola 4),
// in quest'ordine: le letture con il ruolo riferimento (lettura_ruolo), le letture con la funzione relazione non scelte
// (funzione_relazione), l'equivalente K4 (equivalente_k4), i codici del cartiglio dei 2D associati (documentale), i
// candidati di revisione (candidato_revisione). Il codice corretto a mano nella pagina vecchia non è un riferimento: sta
// solo in codice_manuale_legacy, così la voce della radice lo porta fra le evidenze con un campo solo. Un'evidenza che
// viene da un'occorrenza non attiva non entra (la conta la nota importazione.evidenze_file_non_attivi).
func (b *costruttore) riferimenti(n ancoraggio.NodoProposto, ns ancoraggio.NodoStruttura, scelta *motorea.LetturaCodice, lt *lettore) []any {
	var out []any
	_, rr := ancoraggio.DividiRiferimenti(ns.Letture)
	for _, l := range rr {
		out = append(out, map[string]any{"tipo": "lettura_ruolo", "ruolo": l.Ruolo, "lettura": lt.lettura(l), "fonte": l.Forma.Selettore.Campo.Valore})
		b.nelVoci[lt.chiave(l.ID)] = true
	}
	if scelta != nil {
		identita, _ := ancoraggio.DividiRiferimenti(ns.Letture)
		for _, l := range identita {
			if l.ID == scelta.ID || l.Funzione != motorea.FunzRelazione {
				continue
			}
			out = append(out, map[string]any{"tipo": "funzione_relazione", "lettura": lt.lettura(l), "fonte": l.Forma.Selettore.Campo.Valore})
			b.nelVoci[lt.chiave(l.ID)] = true
		}
	}
	if eq := b.equivalente(n.Codice.Equivalente); eq != nil {
		out = append(out, eq)
	}
	for _, cd := range n.Codice.Documentale {
		if !b.attiva(cd.AllegatoID) {
			b.escludi(cd.AllegatoID)
			continue
		}
		out = append(out, b.documentale(cd))
	}
	for _, cr := range n.Codice.Identita.CandidatiRevisione {
		if cr.AllegatoID != nil && !b.attiva(*cr.AllegatoID) {
			b.escludi(*cr.AllegatoID)
			continue
		}
		m := map[string]any{"tipo": "candidato_revisione", "valore": cr.Valore, "fonte": cr.Fonte}
		if cr.AllegatoID != nil {
			m["allegato_id"] = cr.AllegatoID.String()
			b.allegati[*cr.AllegatoID] = true
		}
		if cr.MessaggioID != nil {
			m["messaggio_id"] = cr.MessaggioID.String()
		}
		out = append(out, m)
	}
	return out
}

// equivalente: il riferimento equivalente_k4 (EM §3.8): namespace, base, marcatore e le prove dell'equivalenza, con
// l'emittente della lettura del codice della parte nelle prove (nel file della prova), oppure assente se le prove non
// concordano o la lettura non c'è; mai l'emittente del nodo. Le prove da un'occorrenza non attiva non entrano; senza
// prove non entra l'equivalente. Le letture del codice della parte entrano fra le letture del file della prova.
func (b *costruttore) equivalente(eq *ancoraggio.CodiceEquivalente) map[string]any {
	if eq == nil {
		return nil
	}
	var prove []any
	emittente, concordano := "", true
	for _, p := range eq.Prove {
		if !b.attiva(p.AllegatoID) {
			b.escludi(p.AllegatoID)
			continue
		}
		prove = append(prove, map[string]any{"allegato_id": p.AllegatoID.String(), "entita": p.Entita, "lettura_codice": p.LetturaCodice,
			"lettura_riferimento": p.LetturaRiferimento, "fonte": p.Fonte})
		b.allegati[p.AllegatoID] = true
		e := ""
		if l, lt := b.letturaDi(p.AllegatoID, p.LetturaCodice); l != nil {
			e, _ = lt.emittente(l.Forma)
			b.cita(p.AllegatoID, p.LetturaCodice)
		}
		switch {
		case e == "":
			concordano = false
		case emittente == "":
			emittente = e
		case emittente != e:
			concordano = false
		}
	}
	if len(prove) == 0 {
		return nil
	}
	m := map[string]any{"namespace": eq.Namespace, "base": eq.Base, "marcatore": eq.Marcatore, "prove": prove}
	if concordano && emittente != "" {
		m["emittente"] = emittente
	}
	return map[string]any{"tipo": "equivalente_k4", "equivalente": m, "fonte": eq.Fonte()}
}

// documentale: il codice del cartiglio di un 2D associato al nodo, con la riconciliazione con l'identità proposta dallo
// STEP (esito, motivo, correzione) e le discordanze con i codici decisi del nodo (EM §3.8). La lettura del cartiglio
// entra anche fra le letture del file del 2D. Lo stato dell'associazione del file non entra: cambia con gli altri file
// (un caricamento, uno scarto) senza cambiare l'identità proposta, e sta fra le evidenze del file (EM §3.8.1).
func (b *costruttore) documentale(cd ancoraggio.CodiceDocumentale) map[string]any {
	m := map[string]any{"tipo": "documentale", "allegato_id": cd.AllegatoID.String(), "valore": cd.Originale, "esito": string(cd.Esito),
		"motivo": cd.Motivo, "fonte": "cartiglio"}
	b.allegati[cd.AllegatoID] = true
	if cd.Lettura != "" {
		if l, lt := b.letturaDi(cd.AllegatoID, cd.Lettura); l != nil {
			m["lettura"] = lt.lettura(*l)
			b.cita(cd.AllegatoID, cd.Lettura)
		}
	}
	if c := cd.Correzione; c != nil {
		corr := map[string]any{"codice": c.Codice, "base": c.Base.Normalizzata, "allegato_id": c.AllegatoID.String(), "unita_id": c.UnitaID,
			"revisione_inferiore": c.RevisioneInferiore}
		if c.Revisione != nil {
			corr["revisione"] = *c.Revisione
		}
		m["correzione"] = corr
	}
	var dd []any
	for _, d := range cd.Discordanze {
		dd = append(dd, map[string]any{"parte": d.Parte, "decisione": d.Decisione.Codice, "evidenza": d.Evidenza.Valore,
			"revisione_inferiore": d.RevisioneInferiore})
	}
	if len(dd) > 0 {
		m["discordanze"] = dd
	}
	return m
}

// letturaDi: la lettura con quell'ID nell'interpretazione dell'allegato, con il lettore dell'allegato; nil se non c'è.
func (b *costruttore) letturaDi(a uuid.UUID, id string) (*motorea.LetturaCodice, *lettore) {
	f := b.file[a]
	if f == nil {
		return nil, nil
	}
	for i := range f.Interpretazione.Letture {
		if f.Interpretazione.Letture[i].ID == id {
			return &f.Interpretazione.Letture[i], b.lettoreDi(a)
		}
	}
	return nil, nil
}

// attributi: gli attributi dell'entità del nodo nell'interpretazione dello STEP (EM §3.8, «attributi[]»), in ordine
// di ID: la formazione non è mai una revisione; letture_compatibili con le chiavi del registro.
func (b *costruttore) attributi(n ancoraggio.NodoProposto, lt *lettore) []any {
	f := b.file[b.st.AllegatoID]
	var out []any
	for _, a := range f.Interpretazione.Attributi {
		if a.EntitaID != n.EntitaID {
			continue
		}
		out = append(out, attributo(a, lt))
	}
	return out
}

func attributo(a motorea.AttributoLetto, lt *lettore) map[string]any {
	compatibili := make([]any, 0, len(a.LettureCompatibili))
	for _, id := range a.LettureCompatibili {
		compatibili = append(compatibili, lt.chiave(id))
	}
	return map[string]any{"id": a.ID, "tipo": a.Tipo, "grezzo": a.Grezzo, "normalizzato": a.Normalizzato, "stato": a.Stato,
		"unita_id": a.UnitaID, "entita_id": a.EntitaID, "letture_compatibili": compatibili, "evidenze": stringhe(a.Evidenze)}
}

// associazioniProposte: le proposte d'associazione del motore per il nodo (EM §3.8): i file attivi con un candidato
// che ha una posizione sul nodo nel prodotto, uno per allegato (il primo candidato nell'ordine del sostegno), con il
// motivo e il livello del candidato, in ordine di allegato. Mai una preassociazione.
func (b *costruttore) associazioniProposte(rif string) []any {
	type ass struct {
		allegato        uuid.UUID
		motivo, livello string
	}
	var aa []ass
	for _, a := range b.et.Ancoraggi.File {
		trovato := false
		for _, c := range a.Candidati {
			for _, p := range c.Posizioni {
				if !trovato && b.posizioneNelGruppo(p) && p.Nodo == rif {
					trovato = true
					if !b.attiva(a.AllegatoID) {
						b.escludi(a.AllegatoID)
						continue
					}
					aa = append(aa, ass{a.AllegatoID, c.Motivo, c.Livello})
				}
			}
		}
	}
	sort.SliceStable(aa, func(i, j int) bool { return aa[i].allegato.String() < aa[j].allegato.String() })
	var out []any
	for _, x := range aa {
		out = append(out, map[string]any{"allegato_id": x.allegato.String(), "motivo": x.motivo, "livello": x.livello})
	}
	return out
}

// posizioneNelGruppo: la posizione di un candidato sta nella struttura del gruppo (il prodotto, lo STEP e la radice).
func (b *costruttore) posizioneNelGruppo(p ancoraggio.PosizioneCandidato) bool {
	return p.Target == b.pv.Rif && p.AllegatoID == b.st.AllegatoID && p.Radice == b.st.Radice
}

// fabbisogni: i 2D (e gli altri documenti) previsti del nodo al momento dell'importazione (EM §3.8): i fabbisogni
// previsti del prodotto con quel nodo, bloccanti per costruzione, con il motivo.
func (b *costruttore) fabbisogni(rif string) []any {
	var out []any
	for _, p := range b.pv.Documenti.Previsti {
		if p.Nodo == rif {
			out = append(out, map[string]any{"tipo_documento": p.TipoDocumento, "bloccante": true, "motivo": p.Motivo})
		}
	}
	return out
}

// rawSeNonVuoto: il JSON di un elenco, nil per un elenco vuoto.
func rawSeNonVuoto[T any](v []T) json.RawMessage {
	if len(v) == 0 {
		return nil
	}
	return raw(v)
}
