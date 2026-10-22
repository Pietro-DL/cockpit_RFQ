package distintabozza

// conferma3_piano.go: il piano di R5 formato 3 (conferma3.go), calcolato due volte dalla stessa funzione: nella connessione
// S dopo il lucchetto della RFQ e in T dopo i lucchetti delle righe. I due piani devono coincidere (confronto): con H_S e
// H_T e' il «contenuto letto in T diverso da quello di S» di EM §9.5 passo 7. Il piano dice che cosa R5 scrivera' e quali
// righe tocchera': i nodi inclusi e la mappa verso i componenti esistenti, i nodi da creare, gli archi voluti, i componenti
// esclusi da archiviare (o condivisi con un altro prodotto), la promozione della sorgente e la sostituzione documentale,
// gli elenchi delle righe di proposta da bloccare, segnare e togliere da H, le ricodifiche, i ripristini, gli alias dei
// pezzi nuovi con lo stesso codice, le voci assenti, e la difesa R5_NON_APPLICABILE (R4 blocca prima ogni suo caso).

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/fascicolo"
	"promatec/cockpit/internal/platform/db"
)

// arcoR5: un arco voluto della bozza (fra nodi inclusi), per chiave.
type arcoR5 struct {
	padre, figlio string
	qta           int
	posizione     string
	origine       string
	step          *StepArco3
}

// pianoR5: il piano (commento di testa).
type pianoR5 struct {
	prodotto  uuid.UUID
	inclusi   []string             // chiavi dei nodi inclusi, in ordine
	esistenti map[string]uuid.UUID // chiave → componente esistente (registrato o riusato)
	riusati   map[string]bool      // chiavi di un crea risolto su un componente esistente (riusa_esistente, riusa_per_canonico)
	daCreare  []string             // chiavi dei nodi da creare, in ordine (la prima di ogni codice della working)
	// alias: le altre chiavi dei nodi da creare con lo stesso codice della working (EM §5.2 W1, P4-WZ-02 [U]: un componente
	// con piu' archi) → la chiave che lo crea
	alias map[string]string
	// ripristinare: i componenti archiviati che la bozza riusa (EM §5.2 W3): R5 li rimette nella working
	ripristinare []uuid.UUID
	tipo         map[string]string  // tipo effettivo dopo la conferma
	voluti       []arcoR5           // in ordine di (padre, figlio)
	escluso      map[uuid.UUID]bool // componenti esclusi (op esclude o decisione escludi)
	togli        map[[2]uuid.UUID]bool
	archiviare   []uuid.UUID // esclusi che restano senza padri: archiviati
	condivisi    []uuid.UUID // esclusi che un altro prodotto tiene: escono da questo, restano nella working

	// la fonte della working (App. I.6 passi 1 e 2)
	stepA        uuid.NullUUID
	shaA         string
	stepB        uuid.UUID
	shaB         string
	promozione   bool
	sostituzione bool

	// le righe: i lucchetti, i segni, gli elenchi di H (IngressiProposta (2), (2-bis))
	documentiDaBloccare []uuid.UUID
	allegatiCitati      []uuid.UUID
	proposte            []uuid.UUID
	archi               []ArcoProposta
	documentiEsclusi    []uuid.UUID
	rimozioniEscluse    []uuid.UUID
	righe               map[string][]db.ListRighePerShaChiaveRow // «sha|chiave» → righe (nodi tenuti e non inclusi)
	archiEntranti       map[string][]db.ListArchiPerShaChiaveRow // «sha|chiave» → archi che entrano (non inclusi)
	proposteA           []db.ListProposteDelloShaR5Row
	archiA              []db.ListRelazioniDelloShaR5Row

	// ricodifiche: le decisioni ricodifica dei componenti registrati, per componente (D-ASS-01 [U]; EM §9.5 passo 5): R5
	// cambia il codice nella working, allinea le copie e scrive la traccia sulle AR attive (correzione_documento)
	ricodifiche []Decisione3
	// ricodificati: i componenti delle ricodifiche (H: il codice delle loro copie legacy non entra, IngressiProposta (2-ter))
	ricodificati []uuid.UUID

	// assenti: le caselle delle voci della proposta assenti dalla bozza (esclusi[] con origine assenza di R4): R5 ne chiude
	// le righe come per una non_includere, con la nota fissa dell'assenza (EM §7.5, §7.8 con U2 §6.1)
	assenti []Casella3

	nonApplicabili []Rilievo
}

// confronto: le parti del piano che S e T devono vedere uguali.
func (p *pianoR5) confronto() any {
	return map[string]any{"inclusi": p.inclusi, "esistenti": p.esistenti, "creare": p.daCreare, "tipo": p.tipo, "archiviare": p.archiviare,
		"condivisi": p.condivisi, "stepA": p.stepA, "stepB": p.stepB, "promozione": p.promozione, "sostituzione": p.sostituzione,
		"documenti": p.documentiDaBloccare, "allegati": p.allegatiCitati, "proposte": p.proposte, "archi": p.archi,
		"esclusiH": p.documentiEsclusi, "rimozioniH": p.rimozioniEscluse, "assenti": p.assenti, "ricodificati": p.ricodificati,
		"alias": p.alias, "ripristinare": p.ripristinare,
		"nonApplicabili": len(p.nonApplicabili)}
}

// tipoEffettivo3: il tipo che il nodo avrebbe dopo la conferma: prima cambia_tipo, poi d1_a_particolare, poi
// d1_a_sottoassieme (l'ultima vince). E' la regola unica di R4 (analisiR4.tipoEffettivo) e di R5.
func tipoEffettivo3(s Snapshot3, n Nodo3) string {
	t := n.Tipo
	if n.ComponenteID != nil {
		for _, d := range s.Decisioni {
			if d.Tipo == DecisioneCambiaTipo && d.ComponenteID != nil && *d.ComponenteID == *n.ComponenteID {
				t = d.TipoNuovo
			}
		}
	}
	for _, d := range s.Decisioni {
		if d.Tipo == DecisioneD1AParticolare && d.ChiaveBozza == n.ChiaveBozza {
			t = string(db.TipoComponenteSciolto)
		}
	}
	for _, d := range s.Decisioni {
		if d.Tipo == DecisioneD1ASottoassieme && d.ChiaveBozza == n.ChiaveBozza {
			t = string(db.TipoComponenteSottoassieme)
		}
	}
	return t
}

// codiceAtteso: il codice che il nodo di un registrato porta: quello della working, o quello dopo la sua ricodifica.
func codiceAtteso(c confermabom.Componente, ricodificaDi map[uuid.UUID]Decisione3) string {
	if d, ok := ricodificaDi[c.ID]; ok {
		return d.CodiceDopo
	}
	return c.CodiceLetterale
}

func nonApplicabile(motivo, testo string, k string, c *uuid.UUID) Rilievo {
	return Rilievo{Codice: BloccoR5NonApplicabile, Motivo: motivo, ChiaveBozza: k, ComponenteID: c, Testo: testo,
		Rimedi: []string{"la conferma di questa parte arriva in un blocco successivo: per ora toglila dalla bozza o confermala dalla pagina di prima"}}
}

// pianificaR5: il piano (commento di testa), nella transazione di chi chiama (S o T), solo SELECT.
func pianificaR5(ctx context.Context, q *db.Queries, thread uuid.UUID, s Snapshot3, rp RiepilogoR4, st *confermabom.StatoRFQ) (*pianoR5, error) {
	p := &pianoR5{prodotto: s.ProdottoID, esistenti: map[string]uuid.UUID{}, riusati: map[string]bool{}, tipo: map[string]string{}, alias: map[string]string{},
		escluso: map[uuid.UUID]bool{}, togli: map[[2]uuid.UUID]bool{}, righe: map[string][]db.ListRighePerShaChiaveRow{},
		archiEntranti: map[string][]db.ListArchiPerShaChiaveRow{}}
	nodi := map[string]Nodo3{}
	incluso := map[string]bool{}
	for _, n := range s.Nodi {
		nodi[n.ChiaveBozza] = n
		if n.Op == OpEsclude {
			if n.ComponenteID != nil {
				p.escluso[*n.ComponenteID] = true
			}
			continue
		}
		incluso[n.ChiaveBozza] = true
		p.inclusi = append(p.inclusi, n.ChiaveBozza)
	}
	sort.Strings(p.inclusi)
	riusa := map[string]uuid.UUID{}
	var nonInclusi []Decisione3
	for _, d := range s.Decisioni {
		switch d.Tipo {
		case DecisioneEscludi:
			p.escluso[*d.ComponenteID] = true
		case DecisioneRiusaEsistente, DecisioneRiusaPerCanonico:
			riusa[d.ChiaveBozza] = *d.ComponenteID
		case DecisioneTogliArco:
			p.togli[[2]uuid.UUID{*d.PadreID, *d.FiglioID}] = true
		case DecisioneRicodifica:
			p.ricodifiche = append(p.ricodifiche, d)
		case DecisioneNonIncludere:
			nonInclusi = append(nonInclusi, d)
		}
	}
	sort.Slice(nonInclusi, func(i, j int) bool { return nonInclusi[i].ID < nonInclusi[j].ID })
	sort.Slice(p.ricodifiche, func(i, j int) bool {
		return p.ricodifiche[i].ComponenteID.String() < p.ricodifiche[j].ComponenteID.String()
	})
	ricodificaDi := map[uuid.UUID]Decisione3{}
	for _, d := range p.ricodifiche {
		ricodificaDi[*d.ComponenteID] = d
		p.ricodificati = append(p.ricodificati, *d.ComponenteID)
	}
	perComponente := map[uuid.UUID][]string{}
	for _, k := range p.inclusi {
		n := nodi[k]
		p.tipo[k] = tipoEffettivo3(s, n)
		var id uuid.UUID
		switch {
		case n.ComponenteID != nil:
			id = *n.ComponenteID
		case riusa[k] != uuid.Nil:
			id = riusa[k]
			p.riusati[k] = true
		case n.Op == OpCrea:
			p.daCreare = append(p.daCreare, k)
			continue
		default:
			p.nonApplicabili = append(p.nonApplicabili, nonApplicabile("nodo_senza_componente", "Il nodo non è un pezzo nuovo e non cita un componente.", k, nil))
			continue
		}
		p.esistenti[k] = id
		perComponente[id] = append(perComponente[id], k)
		c, ok := st.Componente(id)
		if ok && c.Archiviato && !contieneUUID(p.ripristinare, id) {
			p.ripristinare = append(p.ripristinare, id)
		}
		switch {
		case !ok:
			p.nonApplicabili = append(p.nonApplicabili, nonApplicabile("componente_assente", "Il componente del nodo non c'è nella RFQ.", k, uid(id)))
		case n.Identita.Codice != codiceAtteso(c, ricodificaDi):
			// difesa: R4 lo blocca (IDENTITA_INCOMPATIBILE, RIUSO_INCOMPATIBILE «codice»)
			p.nonApplicabili = append(p.nonApplicabili, nonApplicabile("identita_diversa_dal_registrato",
				"Il codice deciso per il nodo ("+n.Identita.Codice+") non è quello di "+c.CodiceLetterale+" nella working.", k, uid(id)))
		}
	}
	sort.Strings(p.daCreare)
	// il tipo di un componente esistente lo decide il nodo del componente (cambia_tipo, D1: tipoEffettivo3); un crea risolto
	// su di lui (riusa_esistente, riusa_per_canonico) ne prende il tipo, o quello registrato. Due nodi del componente con
	// tipi diversi: difesa (R4: RIUSO_INCOMPATIBILE «stesso_componente»)
	tipoDi := map[uuid.UUID]string{}
	for _, k := range p.inclusi {
		id, ok := p.esistenti[k]
		if !ok || nodi[k].ComponenteID == nil {
			continue
		}
		if t, gia := tipoDi[id]; gia && t != p.tipo[k] {
			r := nonApplicabile("nodi_sullo_stesso_componente", "Due nodi dello stesso componente gli danno tipi diversi.", k, uid(id))
			p.nonApplicabili = append(p.nonApplicabili, r)
			continue
		}
		tipoDi[id] = p.tipo[k]
	}
	for _, k := range p.inclusi {
		id, ok := p.esistenti[k]
		if !ok {
			continue
		}
		if t, ok := tipoDi[id]; ok {
			p.tipo[k] = t
		} else if c, ok := st.Componente(id); ok {
			p.tipo[k] = c.Tipo
		}
	}
	sort.Slice(p.ripristinare, func(i, j int) bool { return p.ripristinare[i].String() < p.ripristinare[j].String() })
	// i nodi da creare con lo stesso codice della working sono un componente solo (W1): lo crea la prima chiave, le altre
	// sono alias; i nodi dello stesso componente (registrato o riusato) si scrivono una volta. Difesa: identita', revisione,
	// tipo e descrizione uguali (R4 lo blocca altrimenti: COLLISIONE_EMITTENTI, IDENTITA_INCOMPATIBILE,
	// DUE_CREA_STESSO_CODICE, RIUSO_INCOMPATIBILE «stesso_componente»)
	descrizione := func(k string, reg string) string {
		if d := nodi[k].Descrizione; d != "" {
			return d
		}
		return nfc(reg)
	}
	uguali := func(x, y string, reg string) bool {
		nx, ny := nodi[x], nodi[y]
		return identicaR4(nx.Identita, ny.Identita) && nx.Identita.Rev == ny.Identita.Rev && p.tipo[x] == p.tipo[y] &&
			descrizione(x, reg) == descrizione(y, reg)
	}
	var creare []string
	primaPer := map[string]string{}
	for _, k := range p.daCreare {
		w := strings.ToUpper(strings.TrimSpace(nodi[k].Identita.Codice))
		if prima, ok := primaPer[w]; ok && w != "" {
			p.alias[k] = prima
			if !uguali(prima, k, "") {
				r := nonApplicabile("nodi_sullo_stesso_componente", "Due pezzi nuovi con lo stesso codice hanno dati diversi.", "", nil)
				r.Chiavi = []string{prima, k}
				p.nonApplicabili = append(p.nonApplicabili, r)
			}
			continue
		}
		primaPer[w] = k
		creare = append(creare, k)
	}
	p.daCreare = creare
	var ripetuti []uuid.UUID
	for id, kk := range perComponente {
		if len(kk) > 1 {
			ripetuti = append(ripetuti, id)
		}
	}
	sort.Slice(ripetuti, func(i, j int) bool { return ripetuti[i].String() < ripetuti[j].String() })
	for _, id := range ripetuti {
		kk := perComponente[id]
		sort.Strings(kk)
		c, _ := st.Componente(id)
		for _, k := range kk[1:] {
			if !uguali(kk[0], k, c.Descrizione) {
				r := nonApplicabile("nodi_sullo_stesso_componente", "Più nodi dello stesso componente hanno dati diversi.", "", uid(id))
				r.Chiavi = kk
				p.nonApplicabili = append(p.nonApplicabili, r)
				break
			}
		}
	}
	// gli archi voluti
	for _, x := range s.Archi {
		if !incluso[x.Padre] || !incluso[x.Figlio] {
			continue
		}
		p.voluti = append(p.voluti, arcoR5{padre: x.Padre, figlio: x.Figlio, qta: x.Qta, posizione: x.Posizione, origine: x.Origine, step: x.Step})
	}
	sort.Slice(p.voluti, func(i, j int) bool {
		if p.voluti[i].padre != p.voluti[j].padre {
			return p.voluti[i].padre < p.voluti[j].padre
		}
		return p.voluti[i].figlio < p.voluti[j].figlio
	})
	// gli esclusi: archiviati se, tolti gli archi di questo prodotto (e quelli dei componenti che R5 archivia), restano
	// senza padri; altrimenti condivisi con un altro prodotto: escono da questo e restano nella working ([T-39])
	mappati := map[uuid.UUID]bool{}
	for _, id := range p.esistenti {
		mappati[id] = true
	}
	per, err := st.Perimetro(s.ProdottoID)
	if err != nil {
		return nil, err
	}
	nelPerimetro := map[uuid.UUID]bool{}
	for _, c := range per.Componenti {
		nelPerimetro[c.ID] = true
	}
	var esclusi []uuid.UUID
	for id := range p.escluso {
		if c, ok := st.Componente(id); ok && !c.Archiviato && nelPerimetro[id] && id != s.ProdottoID {
			esclusi = append(esclusi, id)
		}
	}
	sort.Slice(esclusi, func(i, j int) bool { return esclusi[i].String() < esclusi[j].String() })
	archivio := map[uuid.UUID]bool{}
	for _, id := range esclusi {
		archivio[id] = true
	}
	for cambiato := true; cambiato; {
		cambiato = false
		for _, id := range esclusi {
			if !archivio[id] {
				continue
			}
			for _, a := range st.Archi {
				if a.Figlio == id && !mappati[a.Padre] && !archivio[a.Padre] {
					if c, ok := st.Componente(a.Padre); ok && !c.Archiviato {
						archivio[id] = false
						cambiato = true
						break
					}
				}
			}
		}
	}
	for _, id := range esclusi {
		if archivio[id] {
			p.archiviare = append(p.archiviare, id)
		} else {
			p.condivisi = append(p.condivisi, id)
		}
	}
	// la fonte: la sorgente valida B (senza blocchi c'e') e lo STEP A della working
	if rp.Sorgente.DocumentoID != nil {
		p.stepB, p.shaB = *rp.Sorgente.DocumentoID, rp.Sorgente.Sha256
		p.documentiDaBloccare = append(p.documentiDaBloccare, p.stepB)
	}
	if c, ok := st.Componente(s.ProdottoID); ok && c.StepStrutturale.Valid {
		p.stepA = c.StepStrutturale
		if p.stepB != uuid.Nil && p.stepA.UUID != p.stepB {
			p.documentiDaBloccare = append(p.documentiDaBloccare, p.stepA.UUID)
		}
	}
	if p.stepB != uuid.Nil {
		p.promozione = !p.stepA.Valid || p.stepA.UUID != p.stepB
		if p.stepA.Valid && p.stepA.UUID != p.stepB {
			a, err := q.GetDocumento(ctx, p.stepA.UUID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			if err == nil && !a.SostituitoDa.Valid {
				p.sostituzione = true
				p.shaA = strings.TrimSpace(a.Sha256)
				p.documentiEsclusi = append(p.documentiEsclusi, p.stepA.UUID)
				p.rimozioniEscluse = append(p.rimozioniEscluse, p.stepA.UUID)
			}
		}
	}
	if len(p.archiviare) > 0 {
		dich, err := fascicolo.LeggiDichiarazioni(ctx, q, thread)
		if err != nil {
			return nil, err
		}
		for _, id := range p.archiviare {
			for _, d := range dich.DelComponente(id) {
				if d.Documento.Valid {
					p.rimozioniEscluse = append(p.rimozioniEscluse, d.Documento.UUID)
				}
			}
		}
	}
	p.rimozioniEscluse = uuidUnici(p.rimozioniEscluse)
	// gli allegati citati (PB e svincoli)
	var allegati []uuid.UUID
	for _, x := range s.DocumentiPreassociati {
		allegati = append(allegati, x.AllegatoID)
	}
	for _, d := range s.Decisioni {
		if d.Tipo == DecisioneScollegaDocumento && d.AllegatoID != nil {
			allegati = append(allegati, *d.AllegatoID)
		}
	}
	p.allegatiCitati = uuidUnici(allegati)
	// le righe dei nodi tenuti (caselle dei nodi inclusi) e dei non inclusi
	var shas, chiavi []string
	visti := map[string]bool{}
	aggiungi := func(rif string) {
		sha, ch := shaEChiave(rif)
		if sha == "" || visti[sha+"|"+ch] {
			return
		}
		visti[sha+"|"+ch] = true
		shas, chiavi = append(shas, sha), append(chiavi, ch)
	}
	for _, k := range p.inclusi {
		for _, c := range nodi[k].Caselle {
			aggiungi(c.Rif)
		}
	}
	var shasNI, chiaviNI []string
	for _, d := range nonInclusi {
		aggiungi(d.Casella.Rif)
		sha, ch := shaEChiave(d.Casella.Rif)
		if sha != "" {
			shasNI, chiaviNI = append(shasNI, sha), append(chiaviNI, ch)
		}
	}
	for _, e := range rp.Esclusi {
		if e.Origine != OrigineEsclusoAssenza || e.Casella == nil {
			continue
		}
		p.assenti = append(p.assenti, *e.Casella)
		aggiungi(e.Casella.Rif)
		if sha, ch := shaEChiave(e.Casella.Rif); sha != "" {
			shasNI, chiaviNI = append(shasNI, sha), append(chiaviNI, ch)
		}
	}
	var proposte []uuid.UUID
	if len(shas) > 0 {
		rr, err := q.ListRighePerShaChiave(ctx, db.ListRighePerShaChiaveParams{ThreadID: thread, Shas: shas, Chiavi: chiavi})
		if err != nil {
			return nil, err
		}
		for _, r := range rr {
			k := strings.TrimSpace(r.Sha256) + "|" + r.Chiave
			p.righe[k] = append(p.righe[k], r)
			proposte = append(proposte, r.PropostaID)
		}
	}
	archi := map[ArcoProposta]bool{}
	if len(shasNI) > 0 {
		aa, err := q.ListArchiPerShaChiave(ctx, db.ListArchiPerShaChiaveParams{ThreadID: thread, Shas: shasNI, Chiavi: chiaviNI})
		if err != nil {
			return nil, err
		}
		for _, a := range aa {
			k := strings.TrimSpace(a.Sha256) + "|" + a.FiglioChiave
			p.archiEntranti[k] = append(p.archiEntranti[k], a)
			archi[ArcoProposta{Allegato: a.AllegatoID, Padre: a.PadreChiave, Figlio: a.FiglioChiave}] = true
		}
	}
	for _, x := range p.voluti {
		if a, ok := p.archoStep(nodi, x); ok {
			archi[a] = true
		}
	}
	if p.sostituzione && p.shaA != "" && p.shaA != p.shaB {
		pa, err := q.ListProposteDelloShaR5(ctx, db.ListProposteDelloShaR5Params{ThreadID: thread, Sha256: p.shaA})
		if err != nil {
			return nil, err
		}
		p.proposteA = pa
		for _, r := range pa {
			proposte = append(proposte, r.PropostaID)
		}
		ra, err := q.ListRelazioniDelloShaR5(ctx, db.ListRelazioniDelloShaR5Params{ThreadID: thread, Sha256: pgtype.Text{String: p.shaA, Valid: true}})
		if err != nil {
			return nil, err
		}
		p.archiA = ra
		for _, r := range ra {
			archi[ArcoProposta{Allegato: r.AllegatoID, Padre: r.PadreChiave, Figlio: r.FiglioChiave}] = true
		}
	}
	p.proposte = uuidUnici(proposte)
	for a := range archi {
		p.archi = append(p.archi, a)
	}
	sort.Slice(p.archi, func(i, j int) bool {
		a, b := p.archi[i], p.archi[j]
		if a.Allegato != b.Allegato {
			return a.Allegato.String() < b.Allegato.String()
		}
		if a.Padre != b.Padre {
			return a.Padre < b.Padre
		}
		return a.Figlio < b.Figlio
	})
	p.documentiDaBloccare = uuidUnici(p.documentiDaBloccare)
	return p, nil
}

// archoStep: la riga di relazione_proposta di un arco STEP accettato (origine step con archi[].step): l'occorrenza dello
// step e le chiavi dei nodi padre e figlio nelle loro caselle su quella occorrenza.
func (p *pianoR5) archoStep(nodi map[string]Nodo3, x arcoR5) (ArcoProposta, bool) {
	if x.origine != "step" || x.step == nil {
		return ArcoProposta{}, false
	}
	chiave := func(k string) string {
		for _, c := range nodi[k].Caselle {
			if c.AllegatoID == x.step.AllegatoID {
				if _, ch := shaEChiave(c.Rif); ch != "" {
					return ch
				}
			}
		}
		return ""
	}
	pk, fk := chiave(x.padre), chiave(x.figlio)
	if pk == "" || fk == "" {
		return ArcoProposta{}, false
	}
	return ArcoProposta{Allegato: x.step.AllegatoID, Padre: pk, Figlio: fk}, true
}

// contieneUUID: l'UUID e' nell'elenco.
func contieneUUID(xx []uuid.UUID, x uuid.UUID) bool {
	for _, y := range xx {
		if y == x {
			return true
		}
	}
	return false
}

// uuidUnici: gli UUID senza ripetizioni, in ordine.
func uuidUnici(xx []uuid.UUID) []uuid.UUID {
	visti := map[uuid.UUID]bool{}
	out := []uuid.UUID{}
	for _, x := range xx {
		if x != uuid.Nil && !visti[x] {
			visti[x] = true
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}
