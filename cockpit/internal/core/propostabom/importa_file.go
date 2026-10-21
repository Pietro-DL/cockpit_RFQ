package propostabom

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
)

// Gli archi, le relazioni, i file e le diagnostiche del registro (EM §3.8).

// archi: gli archi della struttura del gruppo, dai fatti (ArcoProposto), con la quantità, le occorrenze (al più 20) e
// le altre occorrenze contate dai fatti; anche gli archi verso un nodo fuori per decisione, che la forma del registro
// ammette.
func (b *costruttore) archi() []importazione.ArcoRegistro {
	altre := map[[2]string]int{}
	f := b.file[b.st.AllegatoID]
	for _, l := range f.Documento.Legami {
		if l.Altre > 0 {
			altre[[2]string{l.Da, l.A}] = l.Altre
		}
	}
	out := make([]importazione.ArcoRegistro, 0, len(b.st.Archi))
	for _, a := range b.st.Archi {
		x := importazione.ArcoRegistro{AllegatoID: b.st.AllegatoID.String(), Padre: a.Padre, Figlio: a.Figlio, Quantita: copiaIntero(a.Quantita),
			Occorrenze: rawSeNonVuoto(a.Occorrenze)}
		if n := altre[[2]string{b.nodiSF[a.Padre].EntitaID, b.nodiSF[a.Figlio].EntitaID}]; n > 0 {
			x.Altre = raw(n)
		}
		out = append(out, x)
	}
	return out
}

func copiaIntero(v *int) *int {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}

// relazioni: le relazioni proposte del motore (EM §3.8, «proposta.relazioni[]»): quelle dello STEP del gruppo con la
// sorgente su un nodo della struttura (sorgente nodo, nel confronto) o su un'unità del file che non è un nodo (sorgente
// file), e quelle dei file attivi con un candidato fra i nodi del gruppo (sorgente file, fra le evidenze). Lo stato si
// copia dal motore, mai una costante. Ogni lettura citata deve stare nel registro: se l'interpretazione non la ha, la
// relazione non entra, con la diagnostica importazione.relazione_incompleta; altrimenti la lettura entra fra le letture
// del file, se le voci non la portano già.
func (b *costruttore) relazioni() []importazione.Relazione {
	nodoDiEntita := map[string]string{}
	for _, n := range b.st.Nodi {
		nodoDiEntita[n.EntitaID] = n.Rif
	}
	entitaSTEP := map[string]bool{}
	for _, n := range b.sf.Nodi {
		entitaSTEP[n.EntitaID] = true
	}
	var out []importazione.Relazione
	aggiungi := func(a uuid.UUID, r motorea.RelazioneSemantica, sorgente importazione.SorgenteRelazione) {
		f := b.file[a]
		letture := map[string]bool{}
		for _, l := range f.Interpretazione.Letture {
			letture[l.ID] = true
		}
		citate := append([]string(nil), r.LettureBersaglio...)
		if r.LetturaSorgente != "" {
			citate = append(citate, r.LetturaSorgente)
		}
		for _, id := range citate {
			if !letture[id] {
				b.diagTestata = append(b.diagTestata, evidenze.Diagnostica{Codice: CodiceImportazioneRelazioneIncompleta,
					Gravita: evidenze.GravitaAvviso, Natura: evidenze.NaturaDati, Percorso: "proposta.relazioni",
					Messaggio: fmt.Sprintf("la relazione %s cita la lettura %s, che l'interpretazione del file non ha: non entra nel registro", r.ID, id),
					Rif:       []string{r.ID, a.String() + "/" + id}})
				return
			}
		}
		lt := b.lettoreDi(a)
		bersagli := make([]any, 0, len(r.LettureBersaglio))
		for _, id := range r.LettureBersaglio {
			bersagli = append(bersagli, lt.chiave(id))
			b.cita(a, id)
		}
		x := importazione.Relazione{Voce: raw("r:" + a.String() + ":" + r.ID), IDMotore: raw(r.ID), Tipo: raw(r.Tipo), Stato: raw(r.Stato),
			Modello: raw(r.Modello), EntitaSorgente: raw(r.EntitaSorgente), Riconoscimento: raw(r.Riconoscimento), Sorgente: sorgente,
			LettureBersaglio: rawSeNonVuoto(bersagli), Evidenze: rawSeNonVuoto(r.Evidenze)}
		if r.LetturaSorgente != "" {
			x.LetturaSorgente = raw(lt.chiave(r.LetturaSorgente))
			b.cita(a, r.LetturaSorgente)
		}
		b.allegati[a] = true
		out = append(out, x)
	}
	step := b.st.AllegatoID
	for _, r := range b.file[step].Interpretazione.Relazioni {
		if rif, ok := nodoDiEntita[r.EntitaSorgente]; ok {
			aggiungi(step, r, importazione.SorgenteRelazione{Tipo: "nodo", Voce: raw(vocePer(step, rif))})
			continue
		}
		if entitaSTEP[r.EntitaSorgente] {
			continue // un nodo dello stesso file fuori dalla struttura del gruppo
		}
		aggiungi(step, r, importazione.SorgenteRelazione{Tipo: "file", Voce: raw(voceFile(step))})
	}
	for _, a := range b.fileCandidati() {
		if a == step {
			continue
		}
		for _, r := range b.file[a].Interpretazione.Relazioni {
			aggiungi(a, r, importazione.SorgenteRelazione{Tipo: "file", Voce: raw(voceFile(a))})
		}
	}
	return out
}

// voceFile: la chiave di un file nel registro, f:<allegato_id>.
func voceFile(a uuid.UUID) string { return "f:" + a.String() }

// fileCandidati: gli allegati attivi con un candidato che ha una posizione su un nodo della struttura del gruppo, in
// ordine di allegato; un allegato non attivo si conta fra gli esclusi.
func (b *costruttore) fileCandidati() []uuid.UUID {
	if b.candidati != nil {
		return b.candidati
	}
	nodi := map[string]bool{}
	for _, n := range b.st.Nodi {
		nodi[n.Rif] = true
	}
	var out []uuid.UUID
	for _, a := range b.et.Ancoraggi.File {
		trovato := false
		for _, c := range a.Candidati {
			for _, p := range c.Posizioni {
				trovato = trovato || (b.posizioneNelGruppo(p) && nodi[p.Nodo])
			}
		}
		if !trovato || b.file[a.AllegatoID] == nil {
			continue
		}
		if !b.attiva(a.AllegatoID) {
			b.escludi(a.AllegatoID)
			continue
		}
		out = append(out, a.AllegatoID)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	b.candidati = append([]uuid.UUID{}, out...)
	return out
}

// fileDelRegistro: i file del registro (EM §3.8, «proposta.file[]»): la fonte, i file attivi con un candidato fra i
// nodi del gruppo e quelli che un'evidenza delle voci cita (un 2D associato, la prova di un equivalente K4), mai
// un'occorrenza non attiva. Per ognuno le letture d'identità del file (AncoraggioFile.Letture) più quelle citate dal
// registro e non scritte nelle voci, gli attributi (per lo STEP solo quelli che non sono di un nodo, che stanno nelle
// voci), i candidati fra i nodi del gruppo e l'interpretazione con il contesto: contesto_id e impronta_contesto ci
// sono solo per un file nel perimetro del contesto applicato, e lì sono obbligatori (EM §3.8.2: 422
// registro_incompleto). I fatti, il documento e l'interpretazione per ID stanno in calcolo.
func (b *costruttore) fileDelRegistro() ([]importazione.FileRegistro, error) {
	insieme := map[uuid.UUID]bool{b.st.AllegatoID: true}
	for _, a := range b.fileCandidati() {
		insieme[a] = true
	}
	for a := range b.allegati {
		if b.file[a] != nil && b.attiva(a) {
			insieme[a] = true
		}
	}
	for a := range b.citate {
		if b.file[a] != nil && b.attiva(a) {
			insieme[a] = true
		}
	}
	allegati := make([]uuid.UUID, 0, len(insieme))
	for a := range insieme {
		allegati = append(allegati, a)
	}
	sort.Slice(allegati, func(i, j int) bool { return allegati[i].String() < allegati[j].String() })

	identita := map[uuid.UUID][]string{}
	for _, a := range b.et.Ancoraggi.File {
		identita[a.AllegatoID] = a.Letture
	}
	nodi := map[string]bool{}
	entitaNodi := map[string]bool{}
	for _, n := range b.st.Nodi {
		nodi[n.Rif] = true
	}
	for _, n := range b.sf.Nodi {
		entitaNodi[n.EntitaID] = true
	}
	var out []importazione.FileRegistro
	for _, a := range allegati {
		f := b.file[a]
		lt := b.lettoreDi(a)
		d := f.Documento
		pr := d.Fonte.Provenienza
		origine := pr.OrigineDichiarata
		if origine == "" {
			origine = pr.Canale
		}
		x := importazione.FileRegistro{Voce: raw(voceFile(a)), AllegatoID: a.String(), Sha256: d.Fonte.RiferimentoFatti.Sha256,
			NomeRicevuto: raw(pr.NomeRicevuto), PercorsoRicevuto: raw(pr.PercorsoRicevuto), ContenitoreID: raw(d.Fonte.ContenitoreID),
			Origine: raw(origine)}
		interp := map[string]any{"stato": f.Interpretazione.Stato, "autore_documento": f.Interpretazione.AutoreDocumento}
		if b.attivo && b.in.Regole.NelPerimetro(*f) {
			if f.Interpretazione.ContestoID == "" || f.Interpretazione.ImprontaContesto == "" {
				return nil, rifiuto(importazione.CodicePropostaNonDisponibile, importazione.MotivoRegistroIncompleto, "proposta.file",
					"il file %s sta nel perimetro del contesto, ma la sua interpretazione non ha contesto_id e impronta_contesto", a)
			}
			interp["contesto_id"], interp["impronta_contesto"] = f.Interpretazione.ContestoID, f.Interpretazione.ImprontaContesto
		}
		x.Interpretazione = raw(interp)

		volute := map[string]bool{}
		for _, id := range identita[a] {
			volute[id] = true
		}
		for id := range b.citate[a] {
			if !b.nelVoci[lt.chiave(id)] {
				volute[id] = true
			}
		}
		var letture []map[string]any
		for _, l := range f.Interpretazione.Letture {
			if volute[l.ID] {
				letture = append(letture, lt.lettura(l))
			}
		}
		ordinaLetture(letture)
		x.Letture = rawSeNonVuoto(letture)

		var attributi []any
		for _, at := range f.Interpretazione.Attributi {
			if a == b.st.AllegatoID && entitaNodi[at.EntitaID] {
				continue
			}
			attributi = append(attributi, attributo(at, lt))
		}
		x.Attributi = rawSeNonVuoto(attributi)

		candidati := map[string]bool{}
		for _, an := range b.et.Ancoraggi.File {
			if an.AllegatoID != a {
				continue
			}
			for _, c := range an.Candidati {
				for _, p := range c.Posizioni {
					if b.posizioneNelGruppo(p) && nodi[p.Nodo] {
						candidati[vocePer(b.st.AllegatoID, p.Nodo)] = true
					}
				}
			}
		}
		x.Candidati = rawSeNonVuoto(ordinate(candidati))
		b.allegati[a] = true
		out = append(out, x)
	}
	return out, nil
}

func ordinate(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---- le diagnostiche ----

var reEntita = regexp.MustCompile(`entita\[([^\]]+)\]`)

// diagnostiche: le diagnostiche dell'esito per i file del registro, divise fra le voci e la testata (EM §3.8, «Voce di
// un nodo», diagnostiche[], e «proposta.diagnostiche[]»; prova A-16):
//   - quelle del documento e dell'interpretazione dello STEP del gruppo, a una voce se citano il suo nodo (il Rif,
//     l'entità, un'unità dell'entità, una lettura di quelle unità, o l'entità nel percorso), altrimenti alla testata;
//   - quelle del documento e dell'interpretazione degli altri file del registro, alla testata;
//   - quelle dell'ancoraggio sul prodotto e sullo STEP del gruppo o sui suoi nodi, a una voce se citano il suo nodo,
//     altrimenti alla testata;
//   - quelle della valutazione del thread sul prodotto che citano un nodo di una voce, a quella voce.
//
// Una diagnostica dell'ancoraggio o della valutazione entra solo se parla di questo gruppo (diagnosticaDelGruppo): il
// prodotto, e ogni occorrenza che cita (allegato:<id>) è lo STEP del gruppo o un file attivo del registro, mai un'altra
// occorrenza dello stesso contenuto. Il rif di un nodo (nodo:<sha256>:<chiave>) è uguale su ogni copia dello STEP, e da
// solo non basta: una diagnostica della struttura di un'altra copia, o della riconciliazione di un 2D non attivo,
// cambierebbe impronta_contenuto per un caricamento o per «Altri file» (EM §3.8.1, «Perché le evidenze non entrano nel
// confronto»; §6.11). Una diagnostica lasciata fuori perché cita un'occorrenza non attiva si conta nella nota
// importazione.evidenze_file_non_attivi.
//
// Una diagnostica sta in una voce solo se è riconducibile a quel nodo solo («aggregate per nodo»); una che cita più
// voci (per esempio un avviso di capacità sulle unità di tutti i nodi) non è riconducibile a un nodo e sta una volta
// nella testata, come una che non ne cita nessuna [T]: ripeterla in ogni voce moltiplicava il registro per il numero
// dei nodi (A-17). Nessuna si perde e nessuna si inventa.
func (b *costruttore) diagnostiche(voci []importazione.Voce) {
	step := b.st.AllegatoID
	f := b.file[step]
	perChiave := map[string][]int{}
	unitaDiEntita := map[string][]string{}
	for _, u := range f.Documento.Unita {
		if u.EntitaID != "" {
			unitaDiEntita[u.EntitaID] = append(unitaDiEntita[u.EntitaID], u.ID)
		}
	}
	letturePerUnita := map[string][]string{}
	for _, l := range f.Interpretazione.Letture {
		letturePerUnita[l.UnitaID] = append(letturePerUnita[l.UnitaID], l.ID)
	}
	nodiDelleVoci := map[string]bool{}
	for i, v := range voci {
		rif := v.Origine.Rif
		nodiDelleVoci[rif] = true
		var entita string
		_ = json.Unmarshal(v.EntitaID, &entita)
		chiavi := []string{rif, entita}
		for _, u := range unitaDiEntita[entita] {
			chiavi = append(chiavi, u)
			chiavi = append(chiavi, letturePerUnita[u]...)
		}
		for _, k := range chiavi {
			if k != "" {
				perChiave[k] = append(perChiave[k], i)
			}
		}
	}
	perVoce := make([][]evidenze.Diagnostica, len(voci))
	assegna := func(d evidenze.Diagnostica) {
		indici := map[int]bool{}
		for _, r := range d.Rif {
			for _, i := range perChiave[r] {
				indici[i] = true
			}
		}
		for _, m := range reEntita.FindAllStringSubmatch(d.Percorso, -1) {
			for _, i := range perChiave[m[1]] {
				indici[i] = true
			}
		}
		if len(indici) != 1 {
			b.diagTestata = append(b.diagTestata, d)
			return
		}
		for i := range indici {
			perVoce[i] = append(perVoce[i], d)
		}
	}
	for _, d := range f.Documento.Qualita.Diagnostiche {
		assegna(d)
	}
	for _, d := range f.Interpretazione.Diagnostiche {
		assegna(d)
	}
	for a := range b.allegati {
		if a == step || b.file[a] == nil || !b.attiva(a) {
			continue
		}
		b.diagTestata = append(b.diagTestata, b.file[a].Documento.Qualita.Diagnostiche...)
		b.diagTestata = append(b.diagTestata, b.file[a].Interpretazione.Diagnostiche...)
	}
	nodiStruttura := map[string]bool{}
	for _, n := range b.st.Nodi {
		nodiStruttura[n.Rif] = true
	}
	allegatoGruppo := idAllegato(step)
	for _, d := range b.et.Ancoraggi.Diagnostiche {
		target, gruppo := false, false
		for _, r := range d.Rif {
			target = target || r == b.pv.Rif
			gruppo = gruppo || r == allegatoGruppo || nodiStruttura[r]
		}
		if target && gruppo && b.diagnosticaDelGruppo(d) {
			assegna(d)
		}
	}
	for _, d := range b.et.Diagnostiche {
		target, nodo := false, false
		for _, r := range d.Rif {
			target = target || r == b.pv.Rif
			nodo = nodo || nodiDelleVoci[r]
		}
		if target && nodo && b.diagnosticaDelGruppo(d) {
			assegna(d)
		}
	}
	for i := range voci {
		voci[i].Diagnostiche = rawDiagnostiche(perVoce[i])
	}
}

// idAllegato: il riferimento di un'occorrenza nelle diagnostiche dell'ancoraggio, «allegato:<uuid>».
func idAllegato(a uuid.UUID) string { return "allegato:" + a.String() }

// diagnosticaDelGruppo: ogni occorrenza che la diagnostica cita (allegato:<id>) è lo STEP del gruppo, oppure un file
// del registro attivo che non è un'altra occorrenza dello stesso contenuto dello STEP. Una diagnostica che cita
// un'occorrenza non attiva non entra, e la nota importazione.evidenze_file_non_attivi la conta.
func (b *costruttore) diagnosticaDelGruppo(d evidenze.Diagnostica) bool {
	ok := true
	var nonAttive []uuid.UUID
	for _, r := range d.Rif {
		s, cita := strings.CutPrefix(r, "allegato:")
		if !cita {
			continue
		}
		a, err := uuid.Parse(s)
		switch {
		case err != nil:
			ok = false
		case a == b.st.AllegatoID:
		case !b.attiva(a):
			nonAttive = append(nonAttive, a)
			ok = false
		case !b.allegati[a] || b.file[a] == nil || b.file[a].Documento.Fonte.RiferimentoFatti.Sha256 == b.st.Sha256:
			ok = false
		}
	}
	if !ok {
		for _, a := range nonAttive {
			b.escludi(a)
		}
	}
	return ok
}

// diagnosticheDellAdattatore: le diagnostiche dell'adattatore, nella testata: il thread senza grammatica del cliente,
// il contesto dichiarato e non applicato, la struttura da rileggere (LS-01), un namespace con due emittenti, le evidenze lasciate fuori perché da occorrenze non
// attive.
func (b *costruttore) diagnosticheDellAdattatore(c importazione.Contenuto) {
	if !b.et.Valutato {
		b.diagTestata = append(b.diagTestata, evidenze.Diagnostica{Codice: CodiceImportazioneSenzaGrammatica, Gravita: evidenze.GravitaAvviso,
			Natura: evidenze.NaturaDati, Percorso: "regole.grammatica_cliente",
			Messaggio: fmt.Sprintf("il thread non ha la grammatica del cliente (%s): la struttura si importa senza identità, e i codici restano da assegnare", b.et.Motivo)})
	}
	// il contesto dichiarato e non applicato (Q24 [U]; U2-P49): le diagnostiche del contesto del thread, nella testata,
	// fra le evidenze, perché il contesto della proposta resti conoscibile (Q03 [U]) e un contesto non applicato non
	// sembri applicato. contesto.non_valido (U2-P49) e, con la stessa ragione, contesto.non_attivo (un contesto
	// dichiarato ma non attivo) [T].
	for _, d := range b.in.Regole.Diagnostiche {
		if d.Codice == motorea.CodiceContestoNonValido || d.Codice == motorea.CodiceContestoNonAttivo {
			b.diagTestata = append(b.diagTestata, d)
		}
	}
	if !b.st.GrafoCompleto && reGrafoDaRileggere.MatchString(b.st.MotivoGrafo) {
		b.diagTestata = append(b.diagTestata, evidenze.Diagnostica{Codice: CodiceImportazioneGrafoDaRileggere, Gravita: evidenze.GravitaAvviso,
			Natura: evidenze.NaturaDati, Percorso: "gruppo.motivo_grafo", Messaggio: TestoGrafoDaRileggere,
			Rif: []string{"allegato:" + b.st.AllegatoID.String()}})
	}
	emittenti := map[string]map[string]bool{}
	segna := func(raw json.RawMessage) {
		var l struct {
			Emittente string `json:"emittente"`
			Namespace string `json:"namespace"`
		}
		if json.Unmarshal(raw, &l) != nil || l.Namespace == "" || l.Emittente == "" {
			return
		}
		if emittenti[l.Namespace] == nil {
			emittenti[l.Namespace] = map[string]bool{}
		}
		emittenti[l.Namespace][l.Emittente] = true
	}
	elenco := func(raw json.RawMessage, campo string) {
		var ll []map[string]json.RawMessage
		if json.Unmarshal(raw, &ll) != nil {
			return
		}
		for _, x := range ll {
			if campo == "" {
				b, _ := json.Marshal(x)
				segna(b)
			} else if v, ok := x[campo]; ok {
				segna(v)
			}
		}
	}
	for _, v := range c.Proposta.Nodi {
		if len(v.IdentitaProposta) > 0 {
			segna(v.IdentitaProposta)
		}
		elenco(v.Alternative, "")
		elenco(v.Riferimenti, "lettura")
	}
	for _, x := range c.Proposta.File {
		elenco(x.Letture, "")
	}
	for _, ns := range ordinateChiavi(emittenti) {
		if len(emittenti[ns]) < 2 {
			continue
		}
		e := ordinate(emittenti[ns])
		b.diagTestata = append(b.diagTestata, evidenze.Diagnostica{Codice: CodiceImportazioneNamespaceIncoerente, Gravita: evidenze.GravitaAvviso,
			Natura: evidenze.NaturaDati, Percorso: "proposta",
			Messaggio: fmt.Sprintf("il namespace %q compare con %d emittenti (%s): i namespace sono unici per emittente", ns, len(e), strings.Join(e, ", ")),
			Rif:       e})
	}
	if len(b.esclusi) > 0 {
		var rif []string
		n := 0
		for a, k := range b.esclusi {
			rif = append(rif, "allegato:"+a.String())
			n += k
		}
		sort.Strings(rif)
		b.diagTestata = append(b.diagTestata, evidenze.Diagnostica{Codice: CodiceImportazioneEvidenzeNonAttive, Gravita: evidenze.GravitaNota,
			Natura: evidenze.NaturaDati, Percorso: "proposta",
			Messaggio: fmt.Sprintf("%d evidenze del motore vengono da %d file non attivi (in «Altri file» o scartati): non entrano nel registro", n, len(rif)),
			Rif:       rif})
	}
}

func ordinateChiavi[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// rawDiagnostiche: le diagnostiche come le porta il registro, {codice, gravita, natura, percorso?, messaggio, rif[]},
// senza doppioni, in un ordine che non dipende da dove sono state raccolte.
func rawDiagnostiche(dd []evidenze.Diagnostica) json.RawMessage {
	visti := map[string]bool{}
	var testi []string
	for _, d := range dd {
		m := map[string]any{"codice": d.Codice, "gravita": string(d.Gravita), "natura": string(d.Natura), "percorso": d.Percorso,
			"messaggio": d.Messaggio, "rif": stringhe(d.Rif)}
		s := string(raw(m))
		if !visti[s] {
			visti[s] = true
			testi = append(testi, s)
		}
	}
	if len(testi) == 0 {
		return nil
	}
	sort.Strings(testi)
	return json.RawMessage("[" + strings.Join(testi, ",") + "]")
}
