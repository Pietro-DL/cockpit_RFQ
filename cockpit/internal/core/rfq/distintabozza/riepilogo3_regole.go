package distintabozza

// riepilogo3_regole.go: la parte pura di R4 formato 3 (riepilogo3.go): dagli ingressi letti (ingressiR4) a RiepilogoR4,
// con la firma anteprima-3. Nessun accesso al database, nessun orologio: gli stessi ingressi danno sempre la stessa
// risposta e la stessa firma, cosi' R5 la ricalcola nella connessione S e confronta la firma.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/rfq/associazioni"
	"promatec/cockpit/internal/core/rfq/confermabom"
	"promatec/cockpit/internal/core/rfq/distintabozza/importazione"
	"promatec/cockpit/internal/platform/db"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// analisiR4: lo stato del calcolo.
type analisiR4 struct {
	in      *ingressiR4
	s       Snapshot3
	nodi    map[string]Nodo3
	chiavi  []string // in ordine
	radice  string
	figli   map[string][]Arco3 // per padre, solo fra nodi inclusi
	padri   map[string][]Arco3 // per figlio
	dec     map[string][]Decisione3
	riusa   map[string]uuid.UUID // chiave di un crea → componente riusato (riusa_esistente, riusa_per_canonico)
	risolto map[string]string    // chiave → nodo del grafo («c:<uuid>», «n:<CODICE>», «k:<chiave>»)
	per     confermabom.Perimetro
	perOK   bool
	nodoDi  map[uuid.UUID]string // componente → chiave del nodo registrato
	escluso map[uuid.UUID]bool   // componenti esclusi dalla bozza (op esclude o decisione escludi)
	ricod   map[uuid.UUID]Decisione3
	mono    *Decisione3
	blocchi []Rilievo
	avvisi  []Rilievo
	mot     esitoMotore
}

func calcolaR4(in *ingressiR4) (RiepilogoR4, error) {
	a := nuovaAnalisi(in)
	s := a.s
	out := RiepilogoR4{Formato: FormatoSnapshot3, ProdottoID: s.ProdottoID, BozzaID: in.richiesta.BozzaID, BozzaVersione: in.versione,
		VersioneCorrente: in.vc, VersioneBomBase: s.VersioneBomBase, FirmaBase: s.FirmaBase, Base: in.base, RifScaduti: in.scaduti,
		Sorgente: in.sorgente, DaRicollocare: []VoceVassoio{}, AvvisiMotore: []json.RawMessage{}, FileInAnalisi: in.inAnalisi,
		LegamiStorici: []LegameStoricoR4{}, Ingressi: in.ingressi}
	if out.RifScaduti == nil {
		out.RifScaduti = []RifScaduto{}
	}
	if out.FileInAnalisi == nil {
		out.FileInAnalisi = []FileInAnalisi{}
	}
	if in.registro != nil {
		out.Importazione = &ImportazioneR4{Impronta: in.registro.Impronta, ImprontaContenuto: in.registro.Contenuto.ImprontaContenuto}
	}
	// la base e la sorgente (gate)
	if in.base.Stato == BaseSuperata {
		a.blocca(Rilievo{Codice: BloccoBaseSuperata, Motivi: append([]string(nil), in.base.Motivi...),
			Testo:  "La base della bozza non e' piu' quella di adesso: riallineala con un rebase esplicito (la bozza non si perde).",
			Rimedi: []string{"rebase della base (R2)"}})
	}
	a.gate()
	out.SorgenteLegata, out.LegamiStorici = a.sorgenteLegata()
	out.EffettoSullaFonte = a.effettoSullaFonte()
	// struttura e identita'
	a.prodottoProtetto()
	a.struttura()
	a.assenze()
	a.ciclo()
	a.identitaCollisioni()
	a.canonici()
	a.decisioniConIlDatabase()
	a.registratiDaApplicare()
	a.nonInclusi()
	a.origineTroppoGrande()
	if in.congelata {
		a.blocca(Rilievo{Codice: BloccoCongelata, Testo: "La BOM di lavoro e' congelata: la conferma non la cambia (D-AR08: niente associazioni nuove).",
			Rimedi: []string{"apri una revisione della BOM"}})
	}
	// i documenti (Piano)
	piano, pre := a.documenti()
	out.DocumentiPreassociati = pre
	out.Svincoli = piano.Svincoli
	out.EffettiDocumentali = a.effettiDocumentali(piano, pre)
	out.esitiPiano = piano
	out.risoluzione = a.risolto
	// condivisione e avvisi
	out.Condivisione = a.condivisione(piano, pre)
	out.Identita = a.identita(piano)
	out.Archi = a.archi()
	out.Fuori = a.fuori()
	out.Esclusi = a.esclusi()
	a.avvisiDelRegistro()
	a.avvisiFinali(out)
	out.RegistroDecisioni = a.registro()
	a.mot = a.motore()
	out.AvvisiMotore, out.AvvisiMotoreCalcolati, out.AvvisiMotoreErrore, out.ConflittiMotore = a.mot.avvisi, a.mot.calcolati, a.mot.errore, a.mot.conflitti
	out.CriterioPrevisto = a.criterio()
	out.Blocchi, out.Avvisi = nonNilRilievi(a.blocchi), nonNilRilievi(a.avvisi)
	out.VerificaPrevista = a.verifica(out.CriterioPrevisto, len(out.Blocchi) > 0)
	out.UgualeAllaBase, out.DifferenzeDallaBase = a.ugualeAllaBase(out.CriterioPrevisto, piano)
	if out.DifferenzeDallaBase == nil {
		out.DifferenzeDallaBase = []DifferenzaAtto{}
	}
	// confermabile vuole anche il motore (PA-C01, riepilogo3_motore.go): il riepilogo leggero non e' confermabile
	out.Confermabile = len(out.Blocchi) == 0 && in.base.Stato == BaseValida && out.AvvisiMotoreCalcolati
	f, err := firmaR4(out, in.canonico)
	if err != nil {
		return RiepilogoR4{}, err
	}
	out.FirmaAnteprima = f
	return out, nil
}

// firmaR4: sha256 del JSON canonico della risposta con lo snapshot canonico e il nome della firma, senza le parti fuori
// firma (EM §8.3 con U2 §6.3.5: l'errore del motore, file_in_analisi, ingressi; la firma stessa). Gli avvisi e i conflitti
// del motore sono dentro: il riepilogo leggero e quello con il motore hanno firme diverse.
func firmaR4(out RiepilogoR4, snapshot []byte) (string, error) {
	raw, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	for _, k := range []string{"avvisi_motore_errore", "file_in_analisi", "ingressi", "firma_anteprima"} {
		delete(m, k)
	}
	m["versione"] = json.RawMessage(`"` + VersioneAnteprima3 + `"`)
	m["snapshot"] = json.RawMessage(snapshot)
	can, err := jsoncanonico.Codifica(m)
	if err != nil {
		return "", err
	}
	return jsoncanonico.Impronta(can), nil
}

func nonNilRilievi(rr []Rilievo) []Rilievo {
	if rr == nil {
		return []Rilievo{}
	}
	return rr
}

func nuovaAnalisi(in *ingressiR4) *analisiR4 {
	s := in.snap
	a := &analisiR4{in: in, s: s, nodi: map[string]Nodo3{}, figli: map[string][]Arco3{}, padri: map[string][]Arco3{},
		dec: map[string][]Decisione3{}, riusa: map[string]uuid.UUID{}, risolto: map[string]string{}, nodoDi: map[uuid.UUID]string{},
		escluso: map[uuid.UUID]bool{}, ricod: map[uuid.UUID]Decisione3{}}
	for _, n := range s.Nodi {
		a.nodi[n.ChiaveBozza] = n
		a.chiavi = append(a.chiavi, n.ChiaveBozza)
		if n.ComponenteID != nil {
			a.nodoDi[*n.ComponenteID] = n.ChiaveBozza
			if *n.ComponenteID == s.ProdottoID {
				a.radice = n.ChiaveBozza
			}
			if n.Op == OpEsclude {
				a.escluso[*n.ComponenteID] = true
			}
		}
	}
	sort.Strings(a.chiavi)
	for _, d := range s.Decisioni {
		d := d
		a.dec[d.Tipo] = append(a.dec[d.Tipo], d)
		switch d.Tipo {
		case DecisioneEscludi:
			a.escluso[*d.ComponenteID] = true
		case DecisioneRiusaEsistente, DecisioneRiusaPerCanonico:
			a.riusa[d.ChiaveBozza] = *d.ComponenteID
		case DecisioneRicodifica:
			a.ricod[*d.ComponenteID] = d
		case DecisioneMonoPezzo:
			a.mono = &d
		}
	}
	for _, x := range s.Archi {
		if !a.incluso(x.Padre) || !a.incluso(x.Figlio) {
			continue
		}
		a.figli[x.Padre] = append(a.figli[x.Padre], x)
		a.padri[x.Figlio] = append(a.padri[x.Figlio], x)
	}
	for _, k := range a.chiavi {
		n := a.nodi[k]
		switch {
		case n.Op == OpEsclude:
		case n.ComponenteID != nil:
			a.risolto[k] = "c:" + n.ComponenteID.String()
		case a.riusa[k] != uuid.Nil:
			a.risolto[k] = "c:" + a.riusa[k].String()
		case strings.TrimSpace(n.Identita.Codice) != "":
			a.risolto[k] = "n:" + strings.ToUpper(n.Identita.Codice)
		default:
			a.risolto[k] = "k:" + k
		}
	}
	if per, err := in.stato.Perimetro(s.ProdottoID); err == nil {
		a.per, a.perOK = per, true
	}
	return a
}

// incluso: il nodo c'e' e non e' escluso.
func (a *analisiR4) incluso(k string) bool {
	n, ok := a.nodi[k]
	return ok && n.Op != OpEsclude
}

func (a *analisiR4) blocca(r Rilievo) { a.blocchi = append(a.blocchi, r) }
func (a *analisiR4) avvisa(r Rilievo) { a.avvisi = append(a.avvisi, r) }

// nome: il codice di un nodo per i testi (o la chiave, se non ha codice).
func (a *analisiR4) nome(k string) string {
	if n, ok := a.nodi[k]; ok && n.Identita.Codice != "" {
		return n.Identita.Codice
	}
	return k
}

func (a *analisiR4) codiceDi(id uuid.UUID) string {
	if c, ok := a.in.stato.Componente(id); ok {
		return c.CodiceLetterale
	}
	return id.String()
}

func uid(u uuid.UUID) *uuid.UUID { return &u }

// ------------------------------------------------------------------ la sorgente (direttiva 39 §1, §2, §5)

// testiGate: il testo del blocco per motivo.
var testiGate = map[string]string{
	SorgenteMotivoSenzaSorgente: "La bozza non ha uno STEP sorgente confermato: la conferma lo vuole. Conferma lo STEP del prodotto come sorgente (S8).",
	SorgenteMotivoNonConfermata: "Lo STEP della bozza non e' confermato come sorgente da S8: confermalo come sorgente del prodotto prima della conferma.",
	SorgenteMotivoRevocata:      "La conferma dello STEP come sorgente e' stata revocata: conferma di nuovo una sorgente valida (S8).",
	SorgenteMotivoRifiutata:     "Lo STEP della bozza e' stato rifiutato come fonte: conferma una sorgente valida (S8).",
	SorgenteMotivoSuperata:      "La conferma dello STEP della bozza e' superata (un altro STEP confermato, o il documento sostituito): conferma la sorgente giusta (S8).",
	SorgenteMotivoNonCoerente:   "Lo STEP della bozza non corrisponde al documento confermato o non e' un'occorrenza attiva della RFQ: conferma di nuovo la sorgente (S8).",
}

// gate: il blocco firmato STEP_SORGENTE_NON_CONFERMATA (EM §8.1 riga 31, App. I.3; forma [T-39 corsia contratto]). Porta
// mono_pezzo: true quando la bozza ha la dichiarazione: MONO_PEZZO_SENZA_STEP nel formato 3 non si somma al gate (R39-20).
func (a *analisiR4) gate() {
	sg := a.in.sorgente
	if sg.Valida {
		return
	}
	r := Rilievo{Codice: BloccoStepSorgenteNonConfermata, Motivo: sg.Motivo, DocumentoID: sg.DocumentoID, LegameStorico: sg.LegameStorico,
		MonoPezzo: a.mono != nil, Testo: testiGate[sg.Motivo],
		Rimedi: []string{"«Conferma questo STEP come sorgente» del prodotto (S8)"}}
	if sg.AllegatoID != nil {
		r.Sorgente = &RiferimentoFonte{AllegatoID: *sg.AllegatoID, Sha256: sg.Sha256}
	}
	if r.Testo == "" {
		r.Testo = testiGate[SorgenteMotivoNonConfermata]
	}
	if sg.Motivo == SorgenteMotivoNonConfermata && sg.LegameStorico {
		r.Testo = "Lo STEP della bozza e' collegato al prodotto solo dalle pagine di prima (legame storico): riconfermalo come sorgente da S8."
	}
	a.blocca(r)
}

// sorgenteLegata: la riga «legato come sorgente» (in sola lettura, fuori dalle PB) e il legame storico di D-AR04 (iii):
// solo quello univoco, per un prodotto senza conferme di S8 (D-AR04 (iii) A [U]: con piu' candidati non si seleziona
// nessuna sorgente; R39-06), con la stessa regola di /dati.fonte.legame_storico (confermabom.LegameStoricoUnivoco).
// L'elenco resta, con zero o una voce.
func (a *analisiR4) sorgenteLegata() (*SorgenteLegataR4, []LegameStoricoR4) {
	sg := a.in.sorgente
	storici := []LegameStoricoR4{}
	docs := make([]confermabom.DocumentoDelProdotto, 0, len(a.in.stepProd))
	for _, d := range a.in.stepProd {
		docs = append(docs, confermabom.DocumentoDelProdotto{DocumentoID: d.DocumentoID, Sha256: strings.TrimSpace(d.Sha256), NomeFile: d.NomeFile,
			Estensione: d.Estensione, Sostituito: d.Sostituito})
	}
	if d, ok := confermabom.LegameStoricoUnivoco(docs, len(a.in.shaS8) > 0); ok {
		storici = append(storici, LegameStoricoR4{DocumentoID: d.DocumentoID, Sha256: d.Sha256, NomeFile: d.NomeFile,
			Testo: "STEP del prodotto confermato dalle pagine di prima: e' un legame storico e non vale come sorgente della conferma finche' non lo riconfermi da S8."})
	}
	if sg.AllegatoID == nil {
		return nil, storici
	}
	l := &SorgenteLegataR4{AllegatoID: *sg.AllegatoID, Sha256: sg.Sha256, NomeFile: a.in.nomeFonte, DocumentoID: sg.DocumentoID, Confermata: sg.Valida,
		Motivo: sg.Motivo}
	if sg.Valida {
		l.Testo = "Legato come sorgente del prodotto (confermato da S8): non e' un documento da associare, e la conferma non ne registra l'associazione."
	} else {
		l.Testo = "Sorgente della bozza, non ancora confermata validamente da S8: non e' un documento da associare."
	}
	return l, storici
}

// effettoSullaFonte: la promozione che R5 farebbe (EM App. I.6, passi 1 e 2).
func (a *analisiR4) effettoSullaFonte() EffettoFonteR4 {
	sg := a.in.sorgente
	fa := a.in.vc.FonteAutorizzata
	e := EffettoFonteR4{}
	if fa != nil {
		d := fa.DocumentoID
		e.StepPrima, e.ShaPrima = &d, fa.Sha256
	}
	if !sg.Valida || sg.DocumentoID == nil {
		e.Testo = "Nessuna promozione: la bozza non ha una sorgente confermata validamente da S8."
		if fa != nil {
			e.Testo += " La fonte della working resta quella di adesso."
		}
		return e
	}
	e.StepDopo, e.ShaDopo = sg.DocumentoID, sg.Sha256
	switch {
	case fa != nil && fa.DocumentoID == *sg.DocumentoID:
		e.Testo = "Lo STEP sorgente della bozza e' gia' lo STEP strutturale del prodotto."
	case fa != nil:
		e.Promozione = true
		d := fa.DocumentoID
		e.SostituzioneDi = &d
		e.Testo = "Con la conferma lo STEP sorgente della bozza diventa lo STEP strutturale del prodotto al posto di quello di prima, che resta nella storia come sostituito."
	default:
		e.Promozione = true
		e.Testo = "Con la conferma lo STEP sorgente della bozza diventa lo STEP strutturale del prodotto."
	}
	// la correzione della sorgente: una revisione interna, non del cliente (G39-03 [U]; forma [T-39])
	if sg.Correzione && a.s.VersioneBomBase != nil && a.s.VersioneBomBase.ConfermaID != nil {
		e.CorrezioneSorgente = true
		e.Testo += " È una correzione della sorgente: una revisione interna, non una revisione del cliente."
	}
	return e
}

// ------------------------------------------------------------------ la struttura

// prodottoProtetto: la radice non cambia codice, tipo o fonte fuori dalle regole, non e' esclusa ne' figlia, e nessun altro
// nodo e' un finito (C3 §4.4 n. 9).
func (a *analisiR4) prodottoProtetto() {
	p := a.s.ProdottoID
	r := func(testo string) {
		a.blocca(Rilievo{Codice: BloccoProdottoProtetto, ComponenteID: uid(p), ChiaveBozza: a.radice, Testo: testo})
	}
	if a.radice == "" {
		r("La bozza non ha il nodo del prodotto finito.")
		return
	}
	n := a.nodi[a.radice]
	c, _ := a.in.stato.Componente(p)
	switch {
	case n.Op == OpEsclude || a.escluso[p]:
		r("Il prodotto finito non si esclude dalla sua BOM.")
	case n.Tipo != string(db.TipoComponenteFinito):
		r("Il prodotto resta un finito: il suo tipo non cambia.")
	case n.Identita.Codice != c.CodiceLetterale:
		r("Il codice del prodotto finito non cambia dalla BOM.")
	}
	if _, ok := a.ricod[p]; ok {
		r("Il prodotto finito non si ricodifica dalla BOM.")
	}
	for _, d := range a.dec[DecisioneCambiaTipo] {
		if *d.ComponenteID == p {
			r("Il tipo del prodotto finito non cambia.")
		}
	}
	if len(a.padri[a.radice]) > 0 {
		r("Il prodotto finito non e' figlio di un altro nodo.")
	}
	for _, k := range a.chiavi {
		if k != a.radice && a.incluso(k) && a.nodi[k].Tipo == string(db.TipoComponenteFinito) {
			a.blocca(Rilievo{Codice: BloccoProdottoProtetto, ChiaveBozza: k, Testo: a.nome(k) + " e' un altro finito: nella BOM di un prodotto c'e' un finito solo."})
		}
	}
}

// tipoEffettivo: il tipo che il nodo avrebbe dopo la conferma (cambia_tipo e le decisioni D1).
func (a *analisiR4) tipoEffettivo(k string) string { return tipoEffettivo3(a.s, a.nodi[k]) }

// tipoDelComponente: il tipo dopo la conferma del componente del nodo: per un crea risolto su un registrato (riusa_esistente,
// riusa_per_canonico) quello del nodo del registrato nella bozza, o quello registrato; altrimenti tipoEffettivo (la stessa
// regola del piano di R5).
func (a *analisiR4) tipoDelComponente(k string) string {
	n := a.nodi[k]
	if n.ComponenteID == nil {
		if c, ok := a.riusa[k]; ok && c != uuid.Nil {
			if kr, nella := a.nodoDi[c]; nella && a.incluso(kr) {
				return a.tipoEffettivo(kr)
			}
			if reg, ok := a.in.stato.Componente(c); ok {
				return reg.Tipo
			}
		}
	}
	return a.tipoEffettivo(k)
}

// struttura: FIGLIO_SENZA_PADRE, D1, E37, CODICE_DA_ASSEGNARE, MONO_PEZZO_CON_FIGLI, ARCO_DUPLICATO_DOPO_RISOLUZIONE. I figli
// di un nodo per D1 ed E37 sono quelli del suo componente: con due nodi dello stesso componente (EM §5.2 W1) i figli stanno
// sotto uno solo, e l'altro non e' un assieme vuoto ([T-39] del blocco «R5 completo»).
func (a *analisiR4) struttura() {
	figliPer := map[string]int{}
	for k, ff := range a.figli {
		figliPer[a.risolto[k]] += len(ff)
	}
	for _, k := range a.chiavi {
		n := a.nodi[k]
		if n.Op == OpEsclude {
			continue
		}
		if k != a.radice && len(a.padri[k]) == 0 {
			r := Rilievo{Codice: BloccoFiglioSenzaPadre, ChiaveBozza: k, ComponenteID: n.ComponenteID,
				Testo:  a.nome(k) + " non ha un padre nella bozza: ricollocalo, escludilo o toglilo.",
				Rimedi: []string{"«Sposta sotto…» un padre", "escludi il componente (registrato)", "togli il nodo"}}
			a.blocca(r)
		}
		t := a.tipoDelComponente(k)
		nf := figliPer[a.risolto[k]]
		switch {
		case k != a.radice && t == string(db.TipoComponenteSottoassieme) && nf == 0:
			a.blocca(Rilievo{Codice: BloccoD1UltimoFiglio, ChiaveBozza: k, ComponenteID: n.ComponenteID,
				Testo:  a.nome(k) + " e' un assieme senza figli: diventa un particolare, oppure dagli un figlio.",
				Rimedi: []string{"«Diventa particolare» (d1_a_particolare)", "aggiungi un figlio"}})
		case t == string(db.TipoComponenteSciolto) && nf > 0:
			a.blocca(Rilievo{Codice: BloccoD1ParticolareConFigli, ChiaveBozza: k, ComponenteID: n.ComponenteID,
				Testo: a.nome(k) + " e' un particolare con figli: diventa un assieme, oppure togli i figli.", Rimedi: []string{"«Diventa assieme» (d1_a_sottoassieme)"}})
		case db.TipoComponente(t) == db.TipoComponenteCommerciale && nf > 0:
			a.blocca(Rilievo{Codice: BloccoE37NonContenitore, ChiaveBozza: k, ComponenteID: n.ComponenteID,
				Testo: a.nome(k) + " e' un commerciale: non ha figli.", Rimedi: []string{"togli i figli", "cambia il tipo, con l'effetto mostrato"}})
		}
		if n.Op == OpCrea && a.riusa[k] == uuid.Nil && (strings.TrimSpace(n.Identita.Codice) == "" || n.Identita.Modo == ModoDaDecidere) {
			a.blocca(Rilievo{Codice: BloccoCodiceDaAssegnare, ChiaveBozza: k,
				Testo:  "Il pezzo «" + strings.TrimSpace(n.Descrizione+" "+k) + "» non ha un codice: assegnalo prima di confermare.",
				Rimedi: []string{"correggi il codice del pezzo, con il motivo", "non includerlo, se non e' un articolo"}})
		}
	}
	if a.mono != nil && a.radice != "" && len(a.figli[a.radice]) > 0 {
		a.blocca(Rilievo{Codice: BloccoMonoPezzoConFigli, ChiaveBozza: a.radice, MonoPezzo: true,
			Testo: "Il prodotto e' dichiarato pezzo unico ma ha figli nella bozza: togli la dichiarazione o i figli."})
	}
	coppie := map[[2]string][]string{}
	var ordine [][2]string
	for _, x := range a.s.Archi {
		if !a.incluso(x.Padre) || !a.incluso(x.Figlio) {
			continue
		}
		c := [2]string{a.risolto[x.Padre], a.risolto[x.Figlio]}
		if _, ok := coppie[c]; !ok {
			ordine = append(ordine, c)
		}
		coppie[c] = append(coppie[c], x.Padre+">"+x.Figlio)
	}
	for _, c := range ordine {
		if len(coppie[c]) > 1 {
			a.blocca(Rilievo{Codice: BloccoArcoDuplicato, Chiavi: coppie[c],
				Testo: fmt.Sprintf("%d archi diventano lo stesso legame fra gli stessi due componenti: uniscili in uno.", len(coppie[c]))})
		}
	}
}

// assenze: ASSENZA_NON_DECISA (C3 §4.4 n. 2; U2: per i componenti gia' registrati): un componente o un arco della working del
// prodotto che la bozza non porta, senza la decisione (escludi, togli_arco). Un arco spostato (stesso figlio, padre nuovo:
// arco_spostato, MAT BZ-07) non e' un'assenza.
func (a *analisiR4) assenze() {
	if !a.perOK {
		return
	}
	p := a.s.ProdottoID
	presenti := map[uuid.UUID]bool{}
	for k, r := range a.risolto {
		_ = k
		if strings.HasPrefix(r, "c:") {
			presenti[uuid.MustParse(r[2:])] = true
		}
	}
	for _, c := range a.per.Componenti {
		if c.ID == p || presenti[c.ID] || a.escluso[c.ID] {
			continue
		}
		a.blocca(Rilievo{Codice: BloccoAssenzaNonDecisa, ComponenteID: uid(c.ID), Motivo: "componente",
			Testo:  c.CodiceLetterale + " e' nella BOM registrata ma non nella bozza: decidi se escluderlo o rimetterlo (un'assenza non e' una cancellazione).",
			Rimedi: []string{"«Escludi» con il motivo", "rimettilo nella bozza"}})
	}
	togli := map[[2]uuid.UUID]bool{}
	for _, d := range a.dec[DecisioneTogliArco] {
		togli[[2]uuid.UUID{*d.PadreID, *d.FiglioID}] = true
	}
	nella := map[[2]string]bool{}
	nuoviPerFiglio := map[string]int{}
	working := map[[2]string]bool{}
	for _, x := range a.per.Archi {
		working[[2]string{"c:" + x.Padre.String(), "c:" + x.Figlio.String()}] = true
	}
	for _, x := range a.s.Archi {
		if !a.incluso(x.Padre) || !a.incluso(x.Figlio) {
			continue
		}
		c := [2]string{a.risolto[x.Padre], a.risolto[x.Figlio]}
		nella[c] = true
		if !working[c] {
			nuoviPerFiglio[c[1]]++
		}
	}
	mancanti := map[string][]confermabom.Arco{}
	var figli []string
	for _, x := range a.per.Archi {
		c := [2]string{"c:" + x.Padre.String(), "c:" + x.Figlio.String()}
		if nella[c] || togli[[2]uuid.UUID{x.Padre, x.Figlio}] || a.escluso[x.Padre] || a.escluso[x.Figlio] {
			continue
		}
		if !presenti[x.Figlio] || !presenti[x.Padre] {
			continue // gia' detto per il componente
		}
		if _, ok := mancanti[c[1]]; !ok {
			figli = append(figli, c[1])
		}
		mancanti[c[1]] = append(mancanti[c[1]], x)
	}
	sort.Strings(figli)
	for _, f := range figli {
		mm := mancanti[f]
		if len(mm) <= nuoviPerFiglio[f] {
			continue // spostati sotto un altro padre
		}
		for _, x := range mm {
			a.blocca(Rilievo{Codice: BloccoAssenzaNonDecisa, Motivo: "arco", ComponenteID: uid(x.Figlio),
				Testo:  fmt.Sprintf("L'arco %s → %s della BOM registrata non c'e' nella bozza: scollegalo esplicitamente o rimettilo.", a.codiceDi(x.Padre), a.codiceDi(x.Figlio)),
				Rimedi: []string{"«Scollega da un padre…» (togli_arco)", "rimetti l'arco"}})
		}
	}
}

// ciclo: CICLO sul grafo della bozza unito agli archi della working che restano fuori dal perimetro del prodotto (C3 Δ33;
// PD35 INC-G13; D-ASS-02 [U]: impedire cicli nel grafo). Gli archi dei componenti esclusi escono con loro.
func (a *analisiR4) ciclo() {
	adj := map[string][]string{}
	add := func(p, f string) { adj[p] = append(adj[p], f) }
	for _, x := range a.s.Archi {
		if a.incluso(x.Padre) && a.incluso(x.Figlio) {
			add(a.risolto[x.Padre], a.risolto[x.Figlio])
		}
	}
	dentro := map[[2]uuid.UUID]bool{}
	if a.perOK {
		for _, x := range a.per.Archi {
			dentro[[2]uuid.UUID{x.Padre, x.Figlio}] = true
		}
	}
	for _, x := range a.in.stato.Archi {
		if dentro[[2]uuid.UUID{x.Padre, x.Figlio}] || a.escluso[x.Padre] || a.escluso[x.Figlio] {
			continue
		}
		add("c:"+x.Padre.String(), "c:"+x.Figlio.String())
	}
	var nodi []string
	for k := range adj {
		nodi = append(nodi, k)
	}
	sort.Strings(nodi)
	for k := range adj {
		sort.Strings(adj[k])
	}
	colore := map[string]int{}
	var pila []string
	var trovato []string
	var visita func(n string) bool
	visita = func(n string) bool {
		colore[n] = 1
		pila = append(pila, n)
		for _, f := range adj[n] {
			switch colore[f] {
			case 1:
				for i := len(pila) - 1; i >= 0; i-- {
					if pila[i] == f {
						trovato = append([]string(nil), pila[i:]...)
						return true
					}
				}
			case 0:
				if visita(f) {
					return true
				}
			}
		}
		pila = pila[:len(pila)-1]
		colore[n] = 2
		return false
	}
	for _, n := range nodi {
		if colore[n] == 0 && visita(n) {
			break
		}
	}
	if trovato == nil {
		return
	}
	var nomi []string
	for _, id := range trovato {
		nomi = append(nomi, a.etichetta(id))
	}
	a.blocca(Rilievo{Codice: BloccoCiclo, Chiavi: nomi,
		Testo:  "La struttura avrebbe un ciclo (" + strings.Join(append(nomi, nomi[0]), " → ") + "): un componente non puo' contenere se stesso.",
		Rimedi: []string{"sposta o scollega uno degli archi del ciclo"}})
}

// etichetta: un nodo del grafo per i testi (la chiave della bozza, se c'e', o il codice del componente).
func (a *analisiR4) etichetta(id string) string {
	for _, k := range a.chiavi {
		if a.risolto[k] == id {
			return a.nome(k)
		}
	}
	if strings.HasPrefix(id, "c:") {
		return a.codiceDi(uuid.MustParse(id[2:]))
	}
	return strings.TrimPrefix(strings.TrimPrefix(id, "n:"), "k:")
}

// ------------------------------------------------------------------ identita' (EM §5.2, §5.3)

func identicaR4(x, y Identita) bool {
	return x.Codice == y.Codice && x.Emittente == y.Emittente && x.Namespace == y.Namespace && x.EmittentePresunto == y.EmittentePresunto
}

func differenzeIdentita(x, y Identita) []string {
	var out []string
	if x.Codice != y.Codice {
		out = append(out, "codice")
	}
	if x.Namespace != y.Namespace {
		out = append(out, "namespace")
	}
	if x.EmittentePresunto != y.EmittentePresunto {
		out = append(out, "emittente_presunto")
	}
	return out
}

func parteNodo(k string, n Nodo3) ParteIdentita {
	return ParteIdentita{ChiaveBozza: k, ComponenteID: n.ComponenteID, Codice: n.Identita.Codice, Emittente: n.Identita.Emittente,
		EmittentePresunto: n.Identita.EmittentePresunto, Namespace: n.Identita.Namespace, Origine: "bozza"}
}

func parteRegistrata(c confermabom.Componente, id Identita) ParteIdentita {
	return ParteIdentita{ComponenteID: uid(c.ID), Codice: c.CodiceLetterale, Emittente: id.Emittente, EmittentePresunto: id.EmittentePresunto,
		Namespace: id.Namespace, Origine: "registrato"}
}

// chiaveWorking: upper(codice) che il nodo avrebbe nella working dopo la conferma (con la ricodifica, per un registrato).
func (a *analisiR4) chiaveWorking(k string) string {
	n := a.nodi[k]
	if n.ComponenteID != nil {
		if d, ok := a.ricod[*n.ComponenteID]; ok {
			return strings.ToUpper(d.CodiceDopo)
		}
		if c, ok := a.in.stato.Componente(*n.ComponenteID); ok {
			return strings.ToUpper(c.CodiceLetterale)
		}
	}
	return strings.ToUpper(strings.TrimSpace(n.Identita.Codice))
}

// haDecisioneTipo: il nodo ha una decisione che ne cambia il tipo (cambia_tipo, D1).
func (a *analisiR4) haDecisioneTipo(k string) bool {
	return a.tipoEffettivo(k) != a.nodi[k].Tipo || a.cambiaTipoPer(a.nodi[k].ComponenteID) ||
		a.haDecisione(DecisioneD1AParticolare, k) || a.haDecisione(DecisioneD1ASottoassieme, k) || a.haDecisione(DecisioneD1RestaSottoassieme, k)
}

func (a *analisiR4) cambiaTipoPer(id *uuid.UUID) bool {
	if id == nil {
		return false
	}
	for _, d := range a.dec[DecisioneCambiaTipo] {
		if *d.ComponenteID == *id {
			return true
		}
	}
	return false
}

func (a *analisiR4) haDecisione(tipo, k string) bool {
	for _, d := range a.dec[tipo] {
		if d.ChiaveBozza == k {
			return true
		}
	}
	return false
}

// identitaCollisioni: W1-W4, CREA_SU_ESISTENTE, DUE_CREA_STESSO_CODICE, RIUSO_INCOMPATIBILE, l'avviso
// EMITTENTE_CERTIFICATO_SUPERATO. Nessuna fusione, nessun suffisso: i rimedi li sceglie l'ingegnere.
func (a *analisiR4) identitaCollisioni() {
	gruppi := map[string][]string{}
	var chiaviW []string
	for _, k := range a.chiavi {
		if !a.incluso(k) {
			continue
		}
		w := a.chiaveWorking(k)
		if w == "" {
			continue
		}
		if _, ok := gruppi[w]; !ok {
			chiaviW = append(chiaviW, w)
		}
		gruppi[w] = append(gruppi[w], k)
	}
	sort.Strings(chiaviW)
	registratiPer := map[string][]confermabom.Componente{}
	for _, c := range a.in.stato.Componenti {
		registratiPer[strings.ToUpper(c.CodiceLetterale)] = append(registratiPer[strings.ToUpper(c.CodiceLetterale)], c)
	}
	for _, w := range chiaviW {
		kk := gruppi[w]
		collisione, incompatibile, dueCrea := false, false, false
		for i := 0; i < len(kk); i++ {
			for j := i + 1; j < len(kk); j++ {
				x, y := a.nodi[kk[i]], a.nodi[kk[j]]
				cx, cy := a.componenteDi(kk[i]), a.componenteDi(kk[j])
				if cx != nil && cy != nil {
					if *cx != *cy {
						continue // due registrati diversi: una sola chiave per componente nella working (RICODIFICA_COLLIDE)
					}
					// due nodi dello stesso componente (W1 con P4-WZ-02 [U]: un componente con piu' archi): la stessa identita'
					// e gli stessi dati, altrimenti R5 non saprebbe quale scrivere
					a.stessoComponente(kk[i], kk[j], *cx, w)
					continue
				}
				switch {
				case x.Identita.Emittente != y.Identita.Emittente:
					if !collisione {
						collisione = true
						a.blocca(Rilievo{Codice: BloccoCollisioneEmittenti, ChiaveWorking: w, Parti: []ParteIdentita{parteNodo(kk[i], x), parteNodo(kk[j], y)},
							Testo: "Il codice " + w + " di due emittenti diversi finirebbe nello stesso componente: la distinta non puo' tenerli separati.",
							Rimedi: []string{"decidi un'altra identita' per uno dei due nodi (decisione «identita», con motivo)", "togli il nodo o non includerlo",
								"se sono lo stesso pezzo, usa lo stesso emittente per tutti e due (decisione «identita», con motivo)"}})
					}
				case !identicaR4(x.Identita, y.Identita):
					if !incompatibile {
						incompatibile = true
						a.blocca(Rilievo{Codice: BloccoIdentitaIncompatibile, ChiaveWorking: w, Parti: []ParteIdentita{parteNodo(kk[i], x), parteNodo(kk[j], y)},
							Differenze: differenzeIdentita(x.Identita, y.Identita),
							Testo:      "I due nodi finirebbero nello stesso componente con due identita' diverse: uniformale prima di confermare.",
							Rimedi:     []string{"dai a un nodo l'identita' dell'altro (decisione «identita», con motivo)", "togli uno dei due nodi o non includerlo"}})
					}
				case cx == nil && cy == nil:
					if !dueCrea && (x.Identita.Rev != y.Identita.Rev || a.tipoEffettivo(kk[i]) != a.tipoEffettivo(kk[j]) || x.Descrizione != y.Descrizione) {
						dueCrea = true
						a.blocca(Rilievo{Codice: BloccoDueCreaStessoCodice, ChiaveWorking: w, Chiavi: []string{kk[i], kk[j]},
							Testo: "Due pezzi nuovi con il codice " + w + " hanno dati diversi (revisione, tipo o descrizione): uniscili o cambia il codice di uno."})
					}
				default:
					crea, reg := kk[i], cy
					if cx != nil {
						crea, reg = kk[j], cx
					}
					if a.riusa[crea] != *reg {
						a.creaSuEsistente(crea, *reg)
					}
				}
			}
		}
		// un nodo da creare contro un componente registrato che la bozza non porta (W3, W4)
		for _, k := range kk {
			n := a.nodi[k]
			if n.ComponenteID != nil {
				continue
			}
			for _, c := range registratiPer[w] {
				if _, nella := a.nodoDi[c.ID]; nella {
					continue
				}
				reg := IdentitaRegistrataDi(c, a.in.cert, a.in.cliente)
				switch {
				case reg.Emittente != n.Identita.Emittente:
					a.blocca(Rilievo{Codice: BloccoCollisioneEmittenti, ChiaveWorking: w, Parti: []ParteIdentita{parteNodo(k, n), parteRegistrata(c, reg)},
						Testo: "Il codice " + w + " del nodo e il componente registrato con lo stesso codice hanno emittenti diversi: la distinta non puo' tenerli separati.",
						Rimedi: []string{"decidi un'altra identita' per il nodo (decisione «identita», con motivo)", "togli il nodo o non includerlo",
							"se il registrato e' presunto ed e' un codice del partner: «Fissa l'emittente»"}})
				case a.riusa[k] == c.ID:
					if !identicaR4(n.Identita, reg) && n.Identita.Modo != ModoDecisione {
						a.blocca(Rilievo{Codice: BloccoIdentitaIncompatibile, ChiaveWorking: w, Parti: []ParteIdentita{parteNodo(k, n), parteRegistrata(c, reg)},
							Differenze: differenzeIdentita(n.Identita, reg),
							Testo:      "Il nodo riusa il componente registrato con un'identita' diversa: uniformala prima di confermare."})
					}
				default:
					a.creaSuEsistente(k, c.ID)
				}
			}
		}
	}
	// i registrati: riuso compatibile con la working (C3 §4.4 n. 14), identita' registrata (W3), certificazione superata
	for _, k := range a.chiavi {
		n := a.nodi[k]
		if n.ComponenteID == nil || n.Op == OpEsclude {
			continue
		}
		c, ok := a.in.stato.Componente(*n.ComponenteID)
		if !ok {
			continue
		}
		if x, ok := a.in.cert[c.ID]; ok && !x.Valida && x.Emittente != "" {
			a.avvisa(Rilievo{Codice: AvvisoEmittenteCertificatoSuperat, ChiaveBozza: k, ComponenteID: uid(c.ID),
				Testo: c.CodiceLetterale + " e' stato ricodificato dopo l'ultima conferma: l'emittente certificato era di un altro codice, e il nodo vale come codice del cliente presunto."})
		}
		atteso := c.CodiceLetterale
		if d, ric := a.ricod[c.ID]; ric {
			atteso = d.CodiceDopo
		}
		if n.Op != OpRiusa {
			// un nodo modifica del componente porta il codice della working (o quello della ricodifica, U2-P35): un altro
			// codice non lo scrive nessuno (R5 certifica il codice del componente), e il controllo al COMMIT lo rifiuterebbe
			if n.Identita.Codice != atteso {
				reg := IdentitaRegistrataDi(c, a.in.cert, a.in.cliente)
				a.blocca(Rilievo{Codice: BloccoIdentitaIncompatibile, ChiaveBozza: k, ComponenteID: uid(c.ID), ChiaveWorking: strings.ToUpper(atteso),
					Parti: []ParteIdentita{parteNodo(k, n), parteRegistrata(c, reg)}, Differenze: []string{"codice"},
					Testo:  "Il codice deciso per il nodo (" + n.Identita.Codice + ") non e' quello di " + c.CodiceLetterale + ": per cambiarlo serve una ricodifica.",
					Rimedi: []string{"rimetti il codice del componente", "«Correggi codice» con la ricodifica, con il motivo"}})
			}
			continue
		}
		var diff []string
		if n.Identita.Codice != atteso {
			diff = append(diff, "codice")
		}
		if n.Identita.Rev != c.Rev {
			diff = append(diff, "rev")
		}
		if n.Tipo != c.Tipo && !a.haDecisioneTipo(k) {
			diff = append(diff, "tipo")
		}
		if n.Descrizione != "" && n.Descrizione != nfc(c.Descrizione) {
			diff = append(diff, "descrizione")
		}
		if len(diff) > 0 {
			a.blocca(Rilievo{Codice: BloccoRiusoIncompatibile, ChiaveBozza: k, ComponenteID: uid(c.ID), Differenze: diff,
				Testo: c.CodiceLetterale + " e' riusato con dati diversi da quelli registrati (" + strings.Join(diff, ", ") + "): modificalo esplicitamente o rimetti i dati registrati."})
			continue
		}
		if n.Identita.Modo == ModoDecisione || n.Identita.Modo == ModoFormato2 {
			continue
		}
		reg := IdentitaRegistrataDi(c, a.in.cert, a.in.cliente)
		if _, ric := a.ricod[c.ID]; !ric && !identicaR4(n.Identita, reg) {
			a.blocca(Rilievo{Codice: BloccoIdentitaIncompatibile, ChiaveWorking: strings.ToUpper(c.CodiceLetterale),
				Parti:      []ParteIdentita{parteNodo(k, n), parteRegistrata(c, reg)},
				Differenze: differenzeIdentita(n.Identita, reg),
				Testo:      c.CodiceLetterale + " e' riusato con un'identita' diversa da quella registrata: uniformala (decisione «identita», con motivo)."})
		}
	}
}

// stessoComponente: due nodi della bozza che vanno nello stesso componente registrato (EM §5.2 W1, W3): emittenti diversi
// COLLISIONE_EMITTENTI, identita' non identiche IDENTITA_INCOMPATIBILE, revisione, tipo dopo la conferma (fra due nodi del
// componente) o descrizione diversi RIUSO_INCOMPATIBILE motivo stesso_componente ([T-39] del blocco «R5 completo»).
func (a *analisiR4) stessoComponente(x, y string, c uuid.UUID, w string) {
	nx, ny := a.nodi[x], a.nodi[y]
	switch {
	case nx.Identita.Emittente != ny.Identita.Emittente:
		a.blocca(Rilievo{Codice: BloccoCollisioneEmittenti, ChiaveWorking: w, ComponenteID: uid(c), Parti: []ParteIdentita{parteNodo(x, nx), parteNodo(y, ny)},
			Testo:  "Due nodi dello stesso componente " + a.codiceDi(c) + " hanno emittenti diversi: la distinta ne certifica uno solo.",
			Rimedi: []string{"usa lo stesso emittente per tutti e due (decisione «identita», con motivo)", "togli uno dei due nodi"}})
	case !identicaR4(nx.Identita, ny.Identita):
		a.blocca(Rilievo{Codice: BloccoIdentitaIncompatibile, ChiaveWorking: w, ComponenteID: uid(c), Parti: []ParteIdentita{parteNodo(x, nx), parteNodo(y, ny)},
			Differenze: differenzeIdentita(nx.Identita, ny.Identita),
			Testo:      "Due nodi dello stesso componente " + a.codiceDi(c) + " hanno identita' diverse: uniformale prima di confermare."})
	default:
		reg, _ := a.in.stato.Componente(c)
		descr := func(n Nodo3) string {
			if n.Descrizione == "" {
				return nfc(reg.Descrizione)
			}
			return n.Descrizione
		}
		var diff []string
		if nx.Identita.Rev != ny.Identita.Rev {
			diff = append(diff, "rev")
		}
		// il tipo lo decide solo il nodo del componente (cambia_tipo, D1): un crea risolto su di lui ne prende il tipo
		if nx.ComponenteID != nil && ny.ComponenteID != nil && a.tipoEffettivo(x) != a.tipoEffettivo(y) {
			diff = append(diff, "tipo")
		}
		if descr(nx) != descr(ny) {
			diff = append(diff, "descrizione")
		}
		if len(diff) > 0 {
			a.blocca(Rilievo{Codice: BloccoRiusoIncompatibile, Motivo: MotivoStessoComponente, ComponenteID: uid(c), Chiavi: []string{x, y}, Differenze: diff,
				Testo: "Due nodi dello stesso componente " + a.codiceDi(c) + " hanno dati diversi (" + strings.Join(diff, ", ") + "): la working ne tiene uno solo."})
		}
	}
}

// componenteDi: il componente registrato a cui va il nodo (il suo, o quello che una decisione di riuso indica), o nil.
func (a *analisiR4) componenteDi(k string) *uuid.UUID {
	n := a.nodi[k]
	if n.ComponenteID != nil {
		return n.ComponenteID
	}
	if c, ok := a.riusa[k]; ok {
		return &c
	}
	return nil
}

func (a *analisiR4) creaSuEsistente(k string, c uuid.UUID) {
	a.blocca(Rilievo{Codice: BloccoCreaSuEsistente, ChiaveBozza: k, ComponenteID: uid(c), ChiaveWorking: a.chiaveWorking(k),
		Testo:  "Il codice di " + a.nome(k) + " e' gia' di un componente registrato (" + a.codiceDi(c) + "): riusalo, oppure cambia il codice.",
		Rimedi: []string{"«Riusa i componenti con lo stesso codice» (riusa_esistente)", "cambia il codice del nodo, con il motivo"}})
}

// I motivi dei blocchi del blocco «R5 completo» ([T-39]): le parti dei registrati che R5 non applica senza toccare le
// autorizzazioni STEP del Fascicolo.
const (
	MotivoTipoEffettoNonApplicabile   = "effetto_non_applicabile"
	MotivoArchiviatoConAutorizzazione = "archiviato_con_autorizzazione"
	MotivoStessoComponente            = "stesso_componente"
)

// registratiDaApplicare: le parti dei componenti registrati che R5 applica alla working (il tipo dopo cambia_tipo o D1, il
// ripristino di un archiviato riusato) e i casi che non applica, come blocchi del catalogo ([T-39], blocco «R5 completo»):
//   - TIPO_EFFETTO_CAMBIATO motivo effetto_non_applicabile: il cambio di tipo da o verso commerciale di un componente con
//     un'autorizzazione STEP del Fascicolo (il gesto della scheda sospende o registra l'autorizzazione e ricalcola le
//     rimozioni: R5 non lo fa), oppure verso finito (un prodotto nasce da «Aggiungi un codice della richiesta»);
//   - RIUSO_INCOMPATIBILE motivo archiviato_con_autorizzazione: il riuso di un archiviato con un'autorizzazione STEP (il
//     ripristino la riaccende e ricalcola le rimozioni: si ripristina dalla scheda).
func (a *analisiR4) registratiDaApplicare() {
	for _, k := range a.chiavi {
		if !a.incluso(k) {
			continue
		}
		id := a.componenteDi(k)
		if id == nil {
			continue
		}
		c, ok := a.in.stato.Componente(*id)
		if !ok {
			continue
		}
		t := a.tipoEffettivo(k)
		if a.nodi[k].ComponenteID == nil {
			t = c.Tipo // un crea risolto su un registrato (riuso) prende il tipo registrato (RIUSO_INCOMPATIBILE altrimenti)
		}
		switch {
		case t == c.Tipo:
		case k != a.radice && t == string(db.TipoComponenteFinito):
			a.blocca(Rilievo{Codice: BloccoTipoEffettoCambiato, Motivo: MotivoTipoEffettoNonApplicabile, ChiaveBozza: k, ComponenteID: id,
				Testo:  c.CodiceLetterale + " non diventa un prodotto finito dalla BOM: un prodotto nasce da «Aggiungi un codice della richiesta».",
				Rimedi: []string{"togli il cambio di tipo"}})
		case (db.TipoComponente(c.Tipo) == db.TipoComponenteCommerciale || db.TipoComponente(t) == db.TipoComponenteCommerciale) && a.in.dichiarati[c.ID]:
			a.blocca(Rilievo{Codice: BloccoTipoEffettoCambiato, Motivo: MotivoTipoEffettoNonApplicabile, ChiaveBozza: k, ComponenteID: id,
				Testo: "Il cambio di tipo di " + c.CodiceLetterale + " (" + c.Tipo + " → " + t + ") tocca la sua autorizzazione STEP " +
					"(sospensione e rimozioni): la conferma della BOM non lo applica.",
				Rimedi: []string{"cambia il tipo dalla scheda del componente, con l'anteprima, poi riallinea la bozza", "togli il cambio di tipo"}})
		}
		if c.Archiviato && a.in.dichiarati[c.ID] {
			a.blocca(Rilievo{Codice: BloccoRiusoIncompatibile, Motivo: MotivoArchiviatoConAutorizzazione, ChiaveBozza: k, ComponenteID: id,
				Testo:  c.CodiceLetterale + " e' archiviato e ha un'autorizzazione STEP: il ripristino la riaccende, e la conferma della BOM non lo fa.",
				Rimedi: []string{"ripristina il componente dalla scheda, poi riallinea la bozza", "usa un pezzo nuovo con un altro codice"}})
		}
	}
}

// I motivi di RICODIFICA_FILE_NAS (D-ASS-01 [U]; [T-39] del blocco «R5 completo»).
const (
	MotivoRicodificaScrittoSulNAS       = "scritto_sul_nas"
	MotivoRicodificaDocumentoConfermato = "documento_confermato"
)

// decisioniConIlDatabase: RICODIFICA_COLLIDE, RICODIFICA_FILE_NAS, TIPO_EFFETTO_CAMBIATO, ARCO_CONDIVISO_QTA, DA_VALUTARE_APERTO.
func (a *analisiR4) decisioniConIlDatabase() {
	for _, d := range a.dec[DecisioneRicodifica] {
		w := strings.ToUpper(d.CodiceDopo)
		for _, c := range a.in.stato.Componenti {
			if c.ID != *d.ComponenteID && strings.ToUpper(c.CodiceLetterale) == w {
				a.blocca(Rilievo{Codice: BloccoRicodificaCollide, ComponenteID: d.ComponenteID, ChiaveWorking: w,
					Testo: "La ricodifica di " + a.codiceDi(*d.ComponenteID) + " in " + d.CodiceDopo + " va su un codice gia' usato da un altro componente della RFQ."})
			}
		}
		for _, k := range a.chiavi {
			n := a.nodi[k]
			if a.incluso(k) && n.ComponenteID == nil && strings.ToUpper(strings.TrimSpace(n.Identita.Codice)) == w {
				a.blocca(Rilievo{Codice: BloccoRicodificaCollide, ComponenteID: d.ComponenteID, ChiaveBozza: k, ChiaveWorking: w,
					Testo: "La ricodifica di " + a.codiceDi(*d.ComponenteID) + " in " + d.CodiceDopo + " va sul codice di un pezzo nuovo della bozza."})
			}
		}
		// D-ASS-01 [U]: per i documenti gia' confermati resta RICODIFICA_FILE_NAS. Il blocco vale per ogni documento confermato
		// del componente, anche non ancora scritto sul NAS: la ricodifica ne cambierebbe codice e cartella, e R5 non scrive
		// documento (direttiva 35 §7) [T-39]
		switch {
		case a.in.scrittiNAS[*d.ComponenteID] > 0:
			a.blocca(Rilievo{Codice: BloccoRicodificaFileNAS, Motivo: MotivoRicodificaScrittoSulNAS, ComponenteID: d.ComponenteID,
				Testo: fmt.Sprintf("%s ha %d documenti gia' scritti sul NAS: la ricodifica cambierebbe la loro cartella (lo spostamento non e' in V1).",
					a.codiceDi(*d.ComponenteID), a.in.scrittiNAS[*d.ComponenteID])})
		case a.in.confermati[*d.ComponenteID] > 0:
			a.blocca(Rilievo{Codice: BloccoRicodificaFileNAS, Motivo: MotivoRicodificaDocumentoConfermato, ComponenteID: d.ComponenteID,
				Testo: fmt.Sprintf("%s ha %d documenti gia' confermati: la ricodifica ne cambierebbe il codice e la cartella (non e' in V1).",
					a.codiceDi(*d.ComponenteID), a.in.confermati[*d.ComponenteID]),
				Rimedi: []string{"togli la ricodifica dalla bozza", "correggi il codice dalla scheda del componente, che porta anche i documenti"}})
		}
	}
	for _, d := range a.dec[DecisioneCambiaTipo] {
		f, ok := a.in.effettiTipo[d.ComponenteID.String()+"|"+d.TipoNuovo]
		if !ok || f == "" || f != d.FirmaEffetto {
			a.blocca(Rilievo{Codice: BloccoTipoEffettoCambiato, ComponenteID: d.ComponenteID,
				Testo: "L'effetto del cambio di tipo di " + a.codiceDi(*d.ComponenteID) + " non e' piu' quello visto: rileggilo e decidi di nuovo."})
		}
	}
	// ARCO_CONDIVISO_QTA: una quantita' cambiata su un arco che un altro prodotto con una conferma valida raggiunge
	qtaW := map[[2]uuid.UUID]int64{}
	for _, x := range a.in.stato.Archi {
		qtaW[[2]uuid.UUID{x.Padre, x.Figlio}] = int64(x.Qta)
	}
	condiviso := map[[2]uuid.UUID]int{}
	for _, d := range a.dec[DecisioneArcoCondiviso] {
		condiviso[[2]uuid.UUID{*d.PadreID, *d.FiglioID}] = d.Qta
	}
	altri := a.altriProdottiValidi()
	for _, x := range a.s.Archi {
		cp, cf := a.componenteDi(x.Padre), a.componenteDi(x.Figlio)
		if cp == nil || cf == nil || !a.incluso(x.Padre) || !a.incluso(x.Figlio) {
			continue
		}
		q, ok := qtaW[[2]uuid.UUID{*cp, *cf}]
		if !ok || int(q) == x.Qta {
			continue
		}
		var toccati []uuid.UUID
		for _, p2 := range altri {
			if per, err := a.in.stato.Perimetro(p2); err == nil && contieneComponente(per, *cp) {
				toccati = append(toccati, p2)
			}
		}
		if len(toccati) == 0 {
			continue
		}
		if qc, ok := condiviso[[2]uuid.UUID{*cp, *cf}]; ok && qc == x.Qta {
			continue
		}
		a.blocca(Rilievo{Codice: BloccoArcoCondivisoQta, Chiavi: []string{x.Padre, x.Figlio}, Prodotti: toccati,
			Testo:  fmt.Sprintf("La quantita' di %s sotto %s cambia (%d → %d) su un arco che anche altri prodotti confermati usano: la working ha una quantita' sola.", a.nome(x.Figlio), a.nome(x.Padre), q, x.Qta),
			Rimedi: []string{"«Arco condiviso»: la stessa quantita' per tutti i prodotti (arco_condiviso)", "rimetti la quantita' di prima"}})
	}
	var dv []string
	for _, d := range a.dec[DecisioneDaValutare] {
		dv = append(dv, d.ChiaveBozza)
	}
	if len(dv) > 0 {
		sort.Strings(dv)
		a.avvisa(Rilievo{Codice: AvvisoDaValutareAperto, Chiavi: dv, Testo: fmt.Sprintf("%d nodi restano «da valutare»: il prodotto resta non verificato.", len(dv))})
	}
}

// altriProdottiValidi: gli altri prodotti finiti della RFQ con l'ultima conferma valida.
func (a *analisiR4) altriProdottiValidi() []uuid.UUID {
	var out []uuid.UUID
	for _, p := range a.in.finiti() {
		if p == a.s.ProdottoID {
			continue
		}
		if _, v, ok, err := a.in.stato.ValiditaUltima(p); err == nil && ok && v.Valida {
			out = append(out, p)
		}
	}
	return out
}

func contieneComponente(per confermabom.Perimetro, id uuid.UUID) bool {
	for _, c := range per.Componenti {
		if c.ID == id {
			return true
		}
	}
	return false
}

// nonInclusi: NON_INCLUSO_HA_COMPONENTE, NON_INCLUSO_CON_FIGLI (con il registro della versione), NON_INCLUSO_NON_APPLICABILE
// (con le righe legacy per sha256 e chiave): EM §7.3.
func (a *analisiR4) nonInclusi() {
	nodoPerCasella := map[string]string{}
	for _, k := range a.chiavi {
		for _, c := range a.nodi[k].Caselle {
			nodoPerCasella[c.AllegatoID.String()+"|"+c.Rif] = k
		}
	}
	nonInclusiRif := map[string]bool{}
	for _, d := range a.dec[DecisioneNonIncludere] {
		nonInclusiRif[d.Casella.AllegatoID.String()+"|"+d.Casella.Rif] = true
	}
	for _, d := range a.dec[DecisioneNonIncludere] {
		c := *d.Casella
		if reg := a.in.registro; reg != nil {
			if v, ok := reg.Contenuto.VocePerRif(c.AllegatoID, c.Rif); ok && conComponente(v.DecisioneEsistente) {
				a.blocca(Rilievo{Codice: BloccoNonInclusoHaComponente, AllegatoID: uid(c.AllegatoID), Chiavi: []string{c.Rif},
					Testo:  "Il nodo non incluso e' gia' un componente registrato: escludi il componente o scollegalo, non «non includere».",
					Rimedi: []string{"«Escludi» il componente", "«Scollega da un padre…»"}})
			}
			var orfani []string
			for _, x := range reg.Contenuto.Proposta.Archi {
				if x.AllegatoID != c.AllegatoID.String() || x.Padre != c.Rif {
					continue
				}
				fk := c.AllegatoID.String() + "|" + x.Figlio
				if nonInclusiRif[fk] {
					continue
				}
				if k, ok := nodoPerCasella[fk]; ok && a.incluso(k) && len(a.padri[k]) == 0 {
					orfani = append(orfani, k)
				}
			}
			if len(orfani) > 0 {
				sort.Strings(orfani)
				a.blocca(Rilievo{Codice: BloccoNonInclusoConFigli, AllegatoID: uid(c.AllegatoID), Chiavi: orfani,
					Testo:  fmt.Sprintf("Il nodo non incluso ha %d figli che resterebbero senza padre: ricollocali o non includi anche loro (nessuna cascata).", len(orfani)),
					Rimedi: []string{"ricolloca i figli", "non includere anche i figli"}})
			}
		}
		sha, ch := shaEChiave(c.Rif)
		righe := a.in.righeNI[sha+"|"+ch]
		motivo := ""
		if len(righe) == 0 {
			motivo = "senza_riga"
		} else {
			automatiche := true
			for _, r := range righe {
				if r.Stato == db.StatoPropostaAperta || r.DecisoDa.Valid {
					automatiche = false
				}
			}
			if automatiche {
				motivo = "riga_automatica"
			}
		}
		if motivo != "" {
			a.blocca(Rilievo{Codice: BloccoNonInclusoNonApplicabile, Motivo: motivo, AllegatoID: uid(c.AllegatoID), Chiavi: []string{c.Rif},
				Testo: "La conferma non saprebbe applicare la non inclusione di questo nodo (nessuna riga della proposta, o righe chiuse da un automatismo)."})
		}
	}
}

// conComponente: decisione_esistente di una voce del registro indica un componente.
func conComponente(raw json.RawMessage) bool {
	if nullo(raw) {
		return false
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return false
	}
	v, ok := m["componente_id"]
	return ok && v != nil && v != ""
}

// origineTroppoGrande: l'unione delle evidenze degli archi dell'ingegnere oltre i 64 KiB di origine (U2 §7.7).
func (a *analisiR4) origineTroppoGrande() {
	type ev struct {
		Padre    string                `json:"padre"`
		Figlio   string                `json:"figlio"`
		Evidenze []EvidenzaDocumentale `json:"evidenze_arco"`
	}
	var tutte []ev
	for _, x := range a.s.Archi {
		if len(x.EvidenzeArco) > 0 {
			tutte = append(tutte, ev{Padre: x.Padre, Figlio: x.Figlio, Evidenze: x.EvidenzeArco})
		}
	}
	if len(tutte) == 0 {
		return
	}
	b, err := jsoncanonico.Codifica(tutte)
	if err == nil && len(b) > MaxOrigine {
		a.blocca(Rilievo{Codice: BloccoOrigineTroppoGrande,
			Testo: fmt.Sprintf("Le evidenze degli archi pesano %d byte, oltre i %d che la conferma puo' tenere: togline qualcuna.", len(b), MaxOrigine)})
	}
}

// ------------------------------------------------------------------ i documenti

// documenti: il Piano del servizio unico sulle PB, sugli svincoli e sulle AR dei registrati che escono, e i blocchi.
func (a *analisiR4) documenti() (associazioni.EsitiPiano, []PreassociazioneR4) {
	svincolate := map[[2]uuid.UUID]bool{}
	var sv []associazioni.VoceSvincolo
	for _, d := range a.dec[DecisioneScollegaDocumento] {
		svincolate[[2]uuid.UUID{*d.AllegatoID, *d.ComponenteID}] = true
		sv = append(sv, associazioni.VoceSvincolo{Allegato: *d.AllegatoID, Componente: *d.ComponenteID, DaDecisione: d.DaDecisione})
	}
	var voci []associazioni.VocePB
	for _, p := range a.s.DocumentiPreassociati {
		n := a.nodi[p.ChiaveBozza]
		v := associazioni.VocePB{Allegato: p.AllegatoID, ChiaveBozza: p.ChiaveBozza, Provenienza: p.Provenienza, Verificata: p.Verificata,
			Componente: a.componenteDi(p.ChiaveBozza),
			Codice:     strings.TrimSpace(n.Identita.Codice), Rev: n.Identita.Rev,
			NonRichiesta: n.Op == OpEsclude || db.TipoComponente(a.tipoEffettivo(p.ChiaveBozza)) == db.TipoComponenteCommerciale}
		if v.Componente != nil {
			if c, ok := a.in.stato.Componente(*v.Componente); ok && n.Identita.Codice == "" {
				v.Codice = c.CodiceLetterale
			}
			// D-ASS-01 [U]: la PB si confronta con l'identita' nuova (la ricodifica); una discordanza resta un conflitto
			if d, ok := a.ricod[*v.Componente]; ok {
				v.Codice = d.CodiceDopo
			}
		}
		if ar, ok := a.in.fatti.Attive[p.AllegatoID]; ok {
			v.Svincolata = svincolate[[2]uuid.UUID{p.AllegatoID, ar.Componente}]
		}
		voci = append(voci, v)
	}
	var uscite []associazioni.Uscita
	if a.perOK {
		presenti := map[uuid.UUID]bool{}
		for _, r := range a.risolto {
			if strings.HasPrefix(r, "c:") {
				presenti[uuid.MustParse(r[2:])] = true
			}
		}
		for _, c := range a.per.Componenti {
			switch {
			case c.ID == a.s.ProdottoID:
			case a.escluso[c.ID]:
				uscite = append(uscite, associazioni.Uscita{Componente: c.ID, Escluso: true})
			case !presenti[c.ID]:
				uscite = append(uscite, associazioni.Uscita{Componente: c.ID})
			}
		}
	}
	piano := associazioni.Piano(voci, sv, uscite, a.in.fatti)
	esiti := map[uuid.UUID]associazioni.EsitoPB{}
	for _, e := range piano.Preassociazioni {
		esiti[e.Allegato] = e
	}
	pre := make([]PreassociazioneR4, 0, len(a.s.DocumentiPreassociati))
	for _, p := range a.s.DocumentiPreassociati {
		e := esiti[p.AllegatoID]
		x := PreassociazioneR4{EsitoPB: e, Provenienza: p.Provenienza, Importazione: p.Importazione, Verificata: p.Verificata, Gruppo: gruppoDi(p.Provenienza, p.Verificata),
			NomeFile: a.in.fatti.Occorrenze[p.AllegatoID].NomeFile, ComponenteID: a.componenteDi(p.ChiaveBozza), Effetto: effettoPB(e)}
		if x.Provenienza == "" {
			x.Provenienza = ProvenienzaManuale
		}
		pre = append(pre, x)
		if e.Blocca() && e.Esito == associazioni.EsitoInConflitto {
			r := Rilievo{Codice: BloccoPreassociazioneNonApplicab, Motivo: e.Motivo, Variante: e.Variante, ChiaveBozza: p.ChiaveBozza,
				AllegatoID: uid(p.AllegatoID), ComponenteID: e.ARComponente, Prodotti: e.ARProdotti, DocumentoID: e.Documento,
				Testo: testoPB(e, x.NomeFile), Rimedi: rimediPB(e)}
			if e.CopiaComponente != nil {
				r.ComponenteID = e.CopiaComponente
			}
			a.blocca(r)
		}
	}
	for _, e := range piano.Svincoli {
		if e.Blocca() {
			a.blocca(Rilievo{Codice: BloccoPreassociazioneNonApplicab, Motivo: e.Motivo, AllegatoID: uid(e.Allegato), ComponenteID: uid(e.Componente),
				DocumentoID: e.Documento, Testo: testoSvincolo(e)})
		}
	}
	for _, e := range piano.Uscite {
		if e.Blocca() {
			a.blocca(Rilievo{Codice: BloccoPreassociazioneNonApplicab, Motivo: e.Motivo, AllegatoID: uid(e.Allegato), ComponenteID: uid(e.Componente),
				Testo:  a.codiceDi(e.Componente) + " esce dalla BOM con un file associato e senza una decisione: escludilo (con la scelta sulle associazioni) o scollega il file.",
				Rimedi: []string{"«Escludi» con «Mantieni le associazioni» o «Scollega alla conferma»", "scollega il file"}})
		}
	}
	return piano, pre
}

func gruppoDi(provenienza string, verificata bool) string {
	switch provenienza {
	case ProvenienzaMotore:
		return GruppoMotoreIntatta
	case ProvenienzaCorretta:
		return GruppoCorretta
	case ProvenienzaLegacy:
		if verificata {
			return GruppoLegacyVerificata
		}
		return GruppoLegacyDaVerificare
	default:
		return GruppoManuale
	}
}

func effettoPB(e associazioni.EsitoPB) string {
	switch e.Esito {
	case associazioni.EsitoApplicabile:
		return "associazione registrata con la conferma (non e' la conferma del documento)"
	case associazioni.EsitoGiaApplicata:
		return "gia' registrata: nessuna riga nuova"
	case associazioni.EsitoSostituzione:
		return "l'associazione di prima si chiude e la nuova si registra con la conferma"
	case associazioni.EsitoNonRichiesta:
		return "non si applica: il documento non e' richiesto per questo nodo"
	case associazioni.EsitoInAttesaCodice:
		return "in attesa del codice del nodo"
	default:
		return "non si applica finche' il conflitto non e' risolto"
	}
}

var testiPB = map[string]string{
	associazioni.MotivoNaturaNonPreassociabile:   "non e' un file che si associa (contenitore o archivio)",
	associazioni.MotivoLegatoComeSorgente:        "e' lo STEP legato come sorgente di un prodotto finito: non e' un documento da associare",
	associazioni.MotivoStepCandidatoDelFinito:    "e' lo STEP candidato sorgente di un prodotto finito: si conferma come sorgente (S8), non si associa come documento",
	associazioni.MotivoFileNonAttivo:             "non e' piu' un file attivo della RFQ (Altri file o scartato)",
	associazioni.MotivoLegacyDaVerificare:        "viene da una pre-assegnazione delle pagine di prima: va verificata (riconoscila, toglila o rifalla a mano)",
	associazioni.MotivoGiaAssociataAltrove:       "e' gia' associato a un altro componente",
	associazioni.MotivoDocumentoSenzaComponente:  "e' gia' un documento confermato come documentazione generale",
	associazioni.MotivoDocumentoConfermatoAltrui: "e' gia' un documento confermato di un altro componente",
	associazioni.MotivoCodiceNonLetto:            "non ha un codice letto",
	associazioni.MotivoCodiceDiverso:             "ha un codice o una revisione diversi da quelli del pezzo",
	associazioni.MotivoFuoriRFQ:                  "non e' un file di questa RFQ",
}

func testoPB(e associazioni.EsitoPB, nome string) string {
	if nome == "" {
		nome = e.Allegato.String()
	}
	t := testiPB[e.Motivo]
	if t == "" {
		t = "non si applica"
	}
	return "Il file «" + nome + "» " + t + "."
}

func rimediPB(e associazioni.EsitoPB) []string {
	switch e.Motivo {
	case associazioni.MotivoGiaAssociataAltrove:
		switch e.Variante {
		case associazioni.VarianteStessoProdotto:
			return []string{"togli la preassociazione", "nella stessa bozza scollega il file dal componente di prima e preassocialo qui (sostituzione)"}
		case associazioni.VarianteAltroProdotto:
			return []string{"togli la preassociazione", "scollega il file nella bozza dell'altro prodotto e conferma quella, poi questa"}
		default:
			return []string{"togli la preassociazione", "scollega la pre-assegnazione delle pagine di prima"}
		}
	case associazioni.MotivoCodiceNonLetto:
		return []string{"togli la preassociazione", "riclassifica il file da «Da verificare»"}
	case associazioni.MotivoCodiceDiverso:
		return []string{"togli la preassociazione", "correggi il codice del pezzo o del file"}
	case associazioni.MotivoLegacyDaVerificare:
		return []string{"riconoscila: resta legacy e diventa verificata (file_riconosciuto, R2)", "togli la preassociazione",
			"rifalla a mano (diventa manuale)"}
	case associazioni.MotivoStepCandidatoDelFinito:
		return []string{"togli la preassociazione", "conferma lo STEP come sorgente del prodotto (S8)"}
	default:
		return []string{"togli la preassociazione"}
	}
}

func testoSvincolo(e associazioni.EsitoSvincolo) string {
	switch e.Motivo {
	case associazioni.MotivoSvincoloSuDocumento:
		return "Il file e' un documento confermato di quel componente: non si scollega da qui (serve una revisione documentale, non in V1)."
	case associazioni.MotivoSvincoloComponenteDiverso:
		return "Il file e' associato a un altro componente: lo svincolo non corrisponde all'associazione registrata."
	default:
		return "Il file non ha un'associazione registrata con quel componente: lo svincolo non si applica."
	}
}

// effettiDocumentali: i gruppi D-PB-04 A [U].
func (a *analisiR4) effettiDocumentali(piano associazioni.EsitiPiano, pre []PreassociazioneR4) EffettiDocumentaliR4 {
	out := EffettiDocumentaliR4{MotoreIntatta: []VoceGruppoR4{}, Corretta: []VoceGruppoR4{}, Manuale: []VoceGruppoR4{},
		LegacyDaVerificare: []VoceGruppoR4{}, LegacyVerificate: []VoceGruppoR4{}, ProposteRimosse: []PropostaRimossaR4{}, SpostamentiAR: []SpostamentoARR4{}, ARInvariate: []ARInvariataR4{}}
	presenti := map[uuid.UUID]bool{}
	for _, p := range pre {
		presenti[p.Allegato] = true
		v := VoceGruppoR4{AllegatoID: p.Allegato, ChiaveBozza: p.ChiaveBozza, Importazione: p.Importazione, Esito: p.Esito, Motivo: p.Motivo}
		switch p.Gruppo {
		case GruppoMotoreIntatta:
			out.MotoreIntatta = append(out.MotoreIntatta, v)
		case GruppoCorretta:
			out.Corretta = append(out.Corretta, v)
		case GruppoLegacyDaVerificare:
			out.LegacyDaVerificare = append(out.LegacyDaVerificare, v)
		case GruppoLegacyVerificata:
			out.LegacyVerificate = append(out.LegacyVerificate, v)
		default:
			out.Manuale = append(out.Manuale, v)
		}
		if p.Esito == associazioni.EsitoSostituzione {
			out.SpostamentiAR = append(out.SpostamentiAR, SpostamentoARR4{Tipo: associazioni.EsitoSostituzione, AllegatoID: p.Allegato,
				Associazione: p.Associazione, DaComponente: p.ARComponente, AChiaveBozza: p.ChiaveBozza, AComponente: p.ComponenteID, Esito: p.Esito})
		}
	}
	for _, e := range piano.Svincoli {
		out.SpostamentiAR = append(out.SpostamentiAR, SpostamentoARR4{Tipo: "svincolo", AllegatoID: e.Allegato, Associazione: e.Associazione,
			DaComponente: uid(e.Componente), Esito: e.Esito, Motivo: e.Motivo, DaDecisione: e.DaDecisione})
	}
	// le proposte del motore entrate nella bozza (alla nascita, nei comandi espliciti) e poi tolte dall'ingegnere
	entrate := map[uuid.UUID]Preassociazione3{}
	var ordine []uuid.UUID
	segna := func(p Preassociazione3) {
		if p.Provenienza != ProvenienzaMotore || p.AllegatoID == uuid.Nil {
			return
		}
		if _, ok := entrate[p.AllegatoID]; !ok {
			ordine = append(ordine, p.AllegatoID)
		}
		entrate[p.AllegatoID] = p
	}
	for _, e := range a.in.eventi {
		switch e.Azione {
		case AzFilePreassociato:
			var p Preassociazione3
			if json.Unmarshal(e.Dopo, &p) == nil {
				segna(p)
			}
		case AzBozzaAperta, AzSorgenteCambiata, AzPropostaRiallineata:
			var d struct {
				Iniziali []Preassociazione3 `json:"preassociazioni_iniziali"`
			}
			if json.Unmarshal(e.Dopo, &d) == nil {
				for _, p := range d.Iniziali {
					segna(p)
				}
			}
		}
	}
	sort.Slice(ordine, func(i, j int) bool { return ordine[i].String() < ordine[j].String() })
	for _, al := range ordine {
		if presenti[al] {
			continue
		}
		p := entrate[al]
		out.ProposteRimosse = append(out.ProposteRimosse, PropostaRimossaR4{AllegatoID: al, ChiaveBozza: p.ChiaveBozza, Importazione: p.Importazione})
	}
	// le AR che restano: escluso con «Mantieni», ricodifica (D-ASS-01 «mantenere»)
	for _, u := range piano.Uscite {
		if u.Esito == associazioni.EsitoMantenuta {
			out.ARInvariate = append(out.ARInvariate, ARInvariataR4{AllegatoID: u.Allegato, ComponenteID: u.Componente, Associazione: u.Associazione,
				Motivo: "escluso_mantieni", Testo: "Il componente esce dalla BOM e l'associazione resta registrata (storica) finche' non la scolleghi."})
		}
	}
	svincolate := map[[2]uuid.UUID]bool{}
	for _, e := range piano.Svincoli {
		svincolate[[2]uuid.UUID{e.Allegato, e.Componente}] = true
	}
	var allegati []uuid.UUID
	for al := range a.in.fatti.Attive {
		allegati = append(allegati, al)
	}
	sort.Slice(allegati, func(i, j int) bool { return allegati[i].String() < allegati[j].String() })
	for _, al := range allegati {
		ar := a.in.fatti.Attive[al]
		d, ok := a.ricod[ar.Componente]
		if !ok || svincolate[[2]uuid.UUID{al, ar.Componente}] {
			continue
		}
		out.ARInvariate = append(out.ARInvariate, ARInvariataR4{AllegatoID: al, ComponenteID: ar.Componente, Associazione: ar.ID, Motivo: "ricodifica",
			Testo: "Il componente cambia codice in " + d.CodiceDopo + ": l'associazione resta sul componente (mantenuta) e la copia si allinea con la conferma."})
	}
	return out
}

// ------------------------------------------------------------------ condivisione e avvisi

// toccati: i componenti registrati che l'atto cambia, con gli effetti (per condivisione[] e PRODOTTI_TOCCATI).
func (a *analisiR4) toccati() (map[uuid.UUID][]string, []uuid.UUID) {
	eff := map[uuid.UUID][]string{}
	add := func(id uuid.UUID, e string) { eff[id] = append(eff[id], e) }
	for _, k := range a.chiavi {
		n := a.nodi[k]
		if n.ComponenteID == nil {
			continue
		}
		switch {
		case n.Op == OpModifica:
			add(*n.ComponenteID, "modificato (codice, revisione, descrizione o tipo)")
		case n.Op == OpEsclude || a.escluso[*n.ComponenteID]:
			add(*n.ComponenteID, "escluso dalla BOM")
		}
	}
	for _, d := range a.dec[DecisioneRicodifica] {
		add(*d.ComponenteID, "ricodificato in "+d.CodiceDopo)
	}
	for _, d := range a.dec[DecisioneCambiaTipo] {
		add(*d.ComponenteID, "tipo cambiato in "+d.TipoNuovo)
	}
	working := map[[2]uuid.UUID]int64{}
	if a.perOK {
		for _, x := range a.per.Archi {
			working[[2]uuid.UUID{x.Padre, x.Figlio}] = int64(x.Qta)
		}
	}
	nella := map[[2]uuid.UUID]bool{}
	for _, x := range a.s.Archi {
		cp, cf := a.componenteDi(x.Padre), a.componenteDi(x.Figlio)
		if cp == nil || !a.incluso(x.Padre) || !a.incluso(x.Figlio) {
			continue
		}
		if cf == nil {
			add(*cp, "nuovo figlio "+a.nome(x.Figlio))
			continue
		}
		k := [2]uuid.UUID{*cp, *cf}
		nella[k] = true
		q, ok := working[k]
		switch {
		case !ok:
			add(*cp, "nuovo figlio "+a.nome(x.Figlio))
		case int(q) != x.Qta:
			add(*cp, fmt.Sprintf("quantita' di %s da %d a %d", a.nome(x.Figlio), q, x.Qta))
		}
	}
	for k := range working {
		if !nella[k] && !a.escluso[k[0]] {
			add(k[0], "figlio "+a.codiceDi(k[1])+" tolto o spostato")
		}
	}
	ids := make([]uuid.UUID, 0, len(eff))
	for id := range eff {
		sort.Strings(eff[id])
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	return eff, ids
}

// toccatiDocumenti: i componenti registrati di cui l'atto cambia le associazioni (EM H.5 «Componente condiviso»: la AR e'
// del componente e vale per tutti i prodotti che lo contengono; dir. 35 Prova 8): uno svincolo che R5 applicherebbe
// (scollega_documento, anche quello di «Scollega alla conferma» di un escluso) e una PB che R5 registrerebbe su un
// componente che c'e' gia' (applicabile o sostituzione). Gli esiti che non scrivono (gia_applicata, svincolo_gia_chiuso) e
// quelli che bloccano non cambiano niente e restano fuori.
func (a *analisiR4) toccatiDocumenti(piano associazioni.EsitiPiano, pre []PreassociazioneR4) map[uuid.UUID][]string {
	eff := map[uuid.UUID][]string{}
	nome := func(al uuid.UUID) string {
		if n := a.in.fatti.Occorrenze[al].NomeFile; n != "" {
			return "«" + n + "»"
		}
		return al.String()
	}
	for _, e := range piano.Svincoli {
		if e.Esito != associazioni.EsitoApplicabile {
			continue
		}
		eff[e.Componente] = append(eff[e.Componente], "file "+nome(e.Allegato)+" scollegato")
	}
	for _, x := range pre {
		if !x.Applica() || x.ComponenteID == nil {
			continue
		}
		eff[*x.ComponenteID] = append(eff[*x.ComponenteID], "file "+nome(x.Allegato)+" associato")
	}
	for id := range eff {
		sort.Strings(eff[id])
	}
	return eff
}

// condivisione: gli altri prodotti della RFQ che l'atto tocca (EM §7.7; D-TAB-03 A [T-39]): un componente condiviso nel loro
// perimetro, o lo stesso STEP sorgente; e un componente condiviso di cui l'atto cambia le associazioni (elemento di tipo
// documento, EM H.5; dir. 35 Prova 8), che non tocca la struttura dell'altro prodotto. La conseguenza e' attribuita a chi
// conferma; confermare questo prodotto non e' mai una conferma implicita dell'altro.
func (a *analisiR4) condivisione(piano associazioni.EsitiPiano, pre []PreassociazioneR4) []CondivisioneR4 {
	eff, ids := a.toccati()
	effDoc := a.toccatiDocumenti(piano, pre)
	idsDoc := make([]uuid.UUID, 0, len(effDoc))
	for id := range effDoc {
		idsDoc = append(idsDoc, id)
	}
	sort.Slice(idsDoc, func(i, j int) bool { return idsDoc[i].String() < idsDoc[j].String() })
	out := []CondivisioneR4{}
	nomeProd := a.codiceDi(a.s.ProdottoID)
	stesso := map[uuid.UUID][]string{}
	if sg := a.in.sorgente; sg.Sha256 != "" {
		for _, p := range a.in.stessoStep[sg.Sha256] {
			stesso[p.ProdottoID] = append(stesso[p.ProdottoID], p.Come)
		}
	}
	for _, p2 := range a.in.finiti() {
		if p2 == a.s.ProdottoID {
			continue
		}
		per, err := a.in.stato.Perimetro(p2)
		if err != nil {
			continue
		}
		var el []ElementoCondiviso
		for _, id := range ids {
			if id == p2 || !contieneComponente(per, id) {
				continue
			}
			el = append(el, ElementoCondiviso{Tipo: "componente", ComponenteID: uid(id), Codice: a.codiceDi(id), Effetto: strings.Join(eff[id], "; ")})
		}
		if come, ok := stesso[p2]; ok {
			sort.Strings(come)
			el = append(el, ElementoCondiviso{Tipo: "step", Sha256: a.in.sorgente.Sha256, Effetto: "stesso STEP (" + strings.Join(come, ", ") + ")"})
		}
		strutturali := len(el)
		for _, id := range idsDoc {
			if id == p2 || !contieneComponente(per, id) {
				continue
			}
			el = append(el, ElementoCondiviso{Tipo: "documento", ComponenteID: uid(id), Codice: a.codiceDi(id),
				Effetto: strings.Join(effDoc[id], "; ") + " (l'associazione e' del componente e vale anche per " + a.codiceDi(p2) + ")"})
		}
		if len(el) == 0 {
			continue
		}
		_, v, ok, _ := a.in.stato.ValiditaUltima(p2)
		c := CondivisioneR4{ProdottoID: p2, Codice: a.codiceDi(p2), Elementi: el, ConfermaValida: ok && v.Valida,
			Conseguenza: "Le variazioni vengono dalla conferma di " + nomeProd + " e sono attribuite a chi la conferma; " + a.codiceDi(p2) +
				" non e' confermato per implicazione."}
		if ok && v.Valida && strutturali > 0 {
			c.Conseguenza += " La sua ultima conferma puo' non valere piu' per la working cambiata."
		}
		out = append(out, c)
	}
	return out
}

// avvisiFinali: VERIFICA_DOPO_COMMIT, PRODOTTO_SENZA_FIGLI, ESCLUSO_CON_DOCUMENTI, FILE_CONTESO_FRA_PRODOTTI,
// SORGENTE_NON_AUTORIZZATA (informativo, mai con il gate), PRODOTTI_TOCCATI, ESCLUSI_SENZA_MOTIVO, SEGNALATI_INCLUSI.
func (a *analisiR4) avvisiFinali(out RiepilogoR4) {
	a.avvisa(Rilievo{Codice: AvvisoVerificaDopoCommit, Testo: "La verifica della BOM si calcola sulla struttura confermata, dopo il commit."})
	if a.radice != "" && len(a.figli[a.radice]) == 0 {
		t := "Il prodotto non ha figli nella bozza: una BOM vuota non e' verificata per implicazione; dichiara mono_pezzo con il motivo o costruisci la BOM."
		if a.mono != nil {
			t = "Il prodotto e' dichiarato pezzo unico: la dichiarazione vale solo con lo STEP del prodotto confermato come sorgente."
		}
		a.avvisa(Rilievo{Codice: AvvisoProdottoSenzaFigli, ChiaveBozza: a.radice, MonoPezzo: a.mono != nil, Testo: t})
	}
	for _, u := range out.esitiPiano.Uscite {
		if u.Esito == associazioni.EsitoMantenuta {
			a.avvisa(Rilievo{Codice: AvvisoEsclusoConDocumenti, ComponenteID: uid(u.Componente), AllegatoID: uid(u.Allegato),
				Testo: a.codiceDi(u.Componente) + " esce dalla BOM con un file associato: l'associazione resta registrata (storica)."})
		}
	}
	// FILE_CONTESO_FRA_PRODOTTI: lo stesso file preassociato dalla bozza di un altro prodotto a un altro componente, finche'
	// nessuna delle due e' confermata (dopo la prima R5 c'e' la AR e il blocco gia_associata_altrove)
	for _, p := range out.DocumentiPreassociati {
		if p.Esito == associazioni.EsitoInConflitto && p.Motivo == associazioni.MotivoGiaAssociataAltrove {
			continue
		}
		var prodotti []uuid.UUID
		for _, b := range a.in.altreBozze {
			for _, q := range b.Preassociazioni {
				if q.AllegatoID != p.Allegato {
					continue
				}
				if p.ComponenteID != nil && q.Nodo.ComponenteID != nil && *q.Nodo.ComponenteID == *p.ComponenteID {
					continue
				}
				prodotti = append(prodotti, b.ProdottoID)
			}
		}
		if len(prodotti) > 0 {
			a.avvisa(Rilievo{Codice: AvvisoFileContesoFraProdotti, AllegatoID: uid(p.Allegato), ChiaveBozza: p.ChiaveBozza, Prodotti: prodotti,
				Testo: "Il file «" + p.NomeFile + "» e' preassociato anche nella bozza di un altro prodotto a un altro componente: la prima conferma lo registra, la seconda trovera' il conflitto."})
		}
	}
	if sg := a.in.sorgente; sg.Valida {
		fa := a.in.vc.FonteAutorizzata
		if fa == nil || fa.Sha256 != sg.Sha256 || fa.Superata {
			t := "Con la conferma, lo STEP della bozza diventa lo STEP strutturale di " + a.codiceDi(a.s.ProdottoID)
			if fa != nil {
				t += " al posto di quello di prima"
			}
			a.avvisa(Rilievo{Codice: AvvisoSorgenteNonAutorizzata, Sorgente: &RiferimentoFonte{AllegatoID: *sg.AllegatoID, Sha256: sg.Sha256}, Testo: t + "."})
		}
	}
	var toccati []uuid.UUID
	for _, c := range out.Condivisione {
		if c.ConfermaValida {
			for _, e := range c.Elementi {
				if e.Tipo == "componente" {
					toccati = append(toccati, c.ProdottoID)
					break
				}
			}
		}
	}
	if len(toccati) > 0 {
		a.avvisa(Rilievo{Codice: AvvisoProdottiToccati, Prodotti: toccati,
			Testo: "La conferma cambia componenti condivisi con altri prodotti confermati: la loro ultima conferma puo' non valere piu'."})
	}
	n := 0
	for _, e := range out.Esclusi {
		if e.MotivoMancante {
			n++
		}
	}
	if n > 0 {
		a.avvisa(Rilievo{Codice: AvvisoEsclusiSenzaMotivo, Testo: fmt.Sprintf("%d voci della proposta escono dalla BOM senza un motivo scritto.", n)})
	}
	if reg := a.in.registro; reg != nil {
		var seg []string
		for _, k := range a.chiavi {
			n := a.nodi[k]
			if n.Op != OpCrea {
				continue
			}
			for _, c := range n.Caselle {
				if v, ok := reg.Contenuto.VocePerRif(c.AllegatoID, c.Rif); ok && segnalata(v.Segnali) {
					seg = append(seg, k)
					break
				}
			}
		}
		if len(seg) > 0 {
			a.avvisa(Rilievo{Codice: AvvisoSegnalatiInclusi, Chiavi: seg,
				Testo: fmt.Sprintf("%d nodi segnalati come possibili elementi CAD stanno per diventare componenti.", len(seg))})
		}
	}
}

func segnalata(raw json.RawMessage) bool {
	var xx []json.RawMessage
	return !nullo(raw) && json.Unmarshal(raw, &xx) == nil && len(xx) > 0
}

// ------------------------------------------------------------------ le sezioni della risposta

var esitiIdentita = map[string]string{ModoProposta: "accettata", ModoRegistrato: "registrata", ModoManuale: "manuale", ModoDaDecidere: "da_assegnare",
	ModoDecisione: "corretta", ModoFormato2: "formato_2"}

func strp(s string) *string { return &s }

// identita: identita[] (C3 §4.3 slide 1).
func (a *analisiR4) identita(piano associazioni.EsitiPiano) []IdentitaR4 {
	pb := map[string]int{}
	for _, p := range a.s.DocumentiPreassociati {
		pb[p.ChiaveBozza]++
	}
	ar := map[uuid.UUID]int{}
	for _, x := range a.in.fatti.Attive {
		ar[x.Componente]++
	}
	coll := map[string][]string{}
	for _, b := range a.blocchi {
		for _, p := range b.Parti {
			if p.ChiaveBozza != "" {
				coll[p.ChiaveBozza] = append(coll[p.ChiaveBozza], b.Codice)
			}
		}
		if b.Codice == BloccoCreaSuEsistente || b.Codice == BloccoRiusoIncompatibile {
			coll[b.ChiaveBozza] = append(coll[b.ChiaveBozza], b.Codice)
		}
	}
	out := []IdentitaR4{}
	for _, k := range a.chiavi {
		n := a.nodi[k]
		x := IdentitaR4{ChiaveBozza: k, ComponenteID: a.componenteDi(k), Azione: n.Op, Origine: n.Origine, Decisa: n.Identita,
			Codice: PrimaDopo{Dopo: n.Identita.Codice}, Rev: PrimaDopo{Dopo: n.Identita.Rev}, Descrizione: PrimaDopo{Dopo: n.Descrizione},
			Tipo: PrimaDopo{Dopo: a.tipoEffettivo(k)}, EsitoIdentita: esitiIdentita[n.Identita.Modo], Collisioni: coll[k],
			Documenti: DocumentiNodoR4{Preassociati: pb[k]}, Padri: len(a.padri[k]), DifferenzeDallaWork: []string{}}
		if x.Collisioni == nil {
			x.Collisioni = []string{}
		}
		if id := x.ComponenteID; id != nil {
			if c, ok := a.in.stato.Componente(*id); ok {
				x.Codice.Prima, x.Rev.Prima, x.Descrizione.Prima, x.Tipo.Prima = strp(c.CodiceLetterale), strp(c.Rev), strp(c.Descrizione), strp(c.Tipo)
				if d, ok := a.ricod[c.ID]; ok {
					x.Codice.Dopo = d.CodiceDopo
				}
				x.Documenti.ARAttive = ar[c.ID]
				if x.Codice.Dopo != c.CodiceLetterale {
					x.DifferenzeDallaWork = append(x.DifferenzeDallaWork, "codice")
				}
				if x.Tipo.Dopo != c.Tipo {
					x.DifferenzeDallaWork = append(x.DifferenzeDallaWork, "tipo")
				}
				if n.Op == OpEsclude || a.escluso[c.ID] {
					x.DifferenzeDallaWork = append(x.DifferenzeDallaWork, "escluso")
				}
			}
		} else {
			x.NonAncoraConfermato = true
			x.DifferenzeDallaWork = append(x.DifferenzeDallaWork, "nuovo")
		}
		if reg := a.in.registro; reg != nil {
			for _, c := range n.Caselle {
				if v, ok := reg.Contenuto.VocePerRif(c.AllegatoID, c.Rif); ok && !nullo(v.IdentitaProposta) {
					x.Proposta = canonico(v.IdentitaProposta)
					break
				}
			}
		}
		out = append(out, x)
	}
	return out
}

func (a *analisiR4) estremo(k string) EstremoArcoR4 {
	return EstremoArcoR4{ChiaveBozza: k, ComponenteID: a.componenteDi(k), Codice: a.nome(k), Tipo: a.tipoEffettivo(k)}
}

func (a *analisiR4) estremoComponente(id uuid.UUID) EstremoArcoR4 {
	e := EstremoArcoR4{ComponenteID: uid(id), Codice: a.codiceDi(id)}
	if k, ok := a.nodoDi[id]; ok {
		e.ChiaveBozza = k
	}
	if c, ok := a.in.stato.Componente(id); ok {
		e.Tipo = c.Tipo
	}
	return e
}

// archi: archi[] (C3 §4.3 slide 2): gli archi voluti con l'azione, e quelli della working che escono.
func (a *analisiR4) archi() []ArcoR4 {
	working := map[[2]uuid.UUID]int64{}
	var wOrd [][2]uuid.UUID
	if a.perOK {
		for _, x := range a.per.Archi {
			k := [2]uuid.UUID{x.Padre, x.Figlio}
			working[k] = int64(x.Qta)
			wOrd = append(wOrd, k)
		}
	}
	togli := map[[2]uuid.UUID]bool{}
	for _, d := range a.dec[DecisioneTogliArco] {
		togli[[2]uuid.UUID{*d.PadreID, *d.FiglioID}] = true
	}
	out := []ArcoR4{}
	nella := map[[2]uuid.UUID]bool{}
	padrePrimaDi := map[uuid.UUID]uuid.UUID{}
	spostatiDa := map[[2]uuid.UUID]bool{} // l'arco di prima di uno spostamento: si mostra come padre_prima, non come tolto
	for _, k := range wOrd {
		if _, ok := padrePrimaDi[k[1]]; !ok {
			padrePrimaDi[k[1]] = k[0]
		}
	}
	for _, x := range a.s.Archi {
		if !a.incluso(x.Padre) || !a.incluso(x.Figlio) {
			continue
		}
		q := x.Qta
		r := ArcoR4{Padre: a.estremo(x.Padre), Figlio: a.estremo(x.Figlio), QtaVoluta: &q, Origine: x.Origine, Azione: "aggiunto"}
		cp, cf := a.componenteDi(x.Padre), a.componenteDi(x.Figlio)
		if cp != nil && cf != nil {
			k := [2]uuid.UUID{*cp, *cf}
			if w, ok := working[k]; ok {
				nella[k] = true
				wq := int(w)
				r.QtaPrecedente = &wq
				r.Azione = "invariato"
				if wq != q {
					r.Azione = "modificato"
				}
			} else if pp, ok := padrePrimaDi[*cf]; ok && pp != *cp && !spostatiDa[[2]uuid.UUID{pp, *cf}] {
				if _, resta := a.archiNellaBozza()[[2]uuid.UUID{pp, *cf}]; !resta {
					r.Azione = "spostato"
					e := a.estremoComponente(pp)
					r.PadrePrima = &e
					spostatiDa[[2]uuid.UUID{pp, *cf}] = true
				}
			}
		}
		out = append(out, r)
	}
	for _, k := range wOrd {
		if nella[k] || spostatiDa[k] {
			continue
		}
		q := int(working[k])
		r := ArcoR4{Padre: a.estremoComponente(k[0]), Figlio: a.estremoComponente(k[1]), QtaPrecedente: &q, Azione: "tolto"}
		switch {
		case togli[k]:
			r.Decisione = DecisioneTogliArco
		case a.escluso[k[0]] || a.escluso[k[1]]:
			r.Decisione = DecisioneEscludi
		}
		out = append(out, r)
	}
	return out
}

// archiNellaBozza: le coppie di registrati della bozza (padre, figlio).
func (a *analisiR4) archiNellaBozza() map[[2]uuid.UUID]bool {
	out := map[[2]uuid.UUID]bool{}
	for _, x := range a.s.Archi {
		cp, cf := a.componenteDi(x.Padre), a.componenteDi(x.Figlio)
		if cp != nil && cf != nil && a.incluso(x.Padre) && a.incluso(x.Figlio) {
			out[[2]uuid.UUID{*cp, *cf}] = true
		}
	}
	return out
}

// fuori: fuori[]: i componenti della working del prodotto esclusi dalla bozza o che la bozza non porta.
func (a *analisiR4) fuori() []FuoriR4 {
	out := []FuoriR4{}
	if !a.perOK {
		return out
	}
	presenti := map[uuid.UUID]bool{}
	for _, r := range a.risolto {
		if strings.HasPrefix(r, "c:") {
			presenti[uuid.MustParse(r[2:])] = true
		}
	}
	for _, c := range a.per.Componenti {
		switch {
		case c.ID == a.s.ProdottoID:
		case a.escluso[c.ID]:
			out = append(out, FuoriR4{ComponenteID: c.ID, Codice: c.CodiceLetterale, Tipo: c.Tipo, Motivo: "escluso", Decisione: DecisioneEscludi})
		case !presenti[c.ID]:
			out = append(out, FuoriR4{ComponenteID: c.ID, Codice: c.CodiceLetterale, Tipo: c.Tipo, Motivo: "assente"})
		}
	}
	return out
}

// esclusi: esclusi[] (EM §7.4 con U2 §6.1): le non_includere, gli escludi e le voci della proposta assenti dalla bozza.
func (a *analisiR4) esclusi() []EsclusoR4 {
	out := []EsclusoR4{}
	presenti := map[string]bool{}
	for _, k := range a.chiavi {
		for _, c := range a.nodi[k].Caselle {
			presenti[c.AllegatoID.String()+"|"+c.Rif] = true
		}
	}
	for _, d := range a.dec[DecisioneNonIncludere] {
		c := *d.Casella
		presenti[c.AllegatoID.String()+"|"+c.Rif] = true
		out = append(out, EsclusoR4{Origine: DecisioneNonIncludere, ID: d.ID, Casella: &c, MotivoCodice: d.MotivoCodice, Motivo: d.Motivo,
			AzioneID: d.AzioneID, SegnaleVisto: d.SegnaleVisto, SegnaleRegistro: a.segnaleRegistro(c), Effetti: a.effettiNonIncluso(c)})
	}
	for _, d := range a.dec[DecisioneEscludi] {
		out = append(out, EsclusoR4{Origine: DecisioneEscludi, ID: d.ID, ComponenteID: d.ComponenteID, Codice: a.codiceDi(*d.ComponenteID), Motivo: d.Motivo})
	}
	// le voci della proposta assenti dalla bozza (EM §7.8 con U2 §6.1): R5 ne chiude le righe come per una non_includere, con
	// la nota fissa dell'assenza, e ne scrive la traccia; gli effetti li dichiara R4 (nessuna cascata nascosta)
	for _, c := range caselleAssenti(a.s, a.in.registro) {
		c := c
		presenti[c.AllegatoID.String()+"|"+c.Rif] = true
		out = append(out, EsclusoR4{Origine: OrigineEsclusoAssenza, Casella: &c, MotivoMancante: true, SegnaleRegistro: a.segnaleRegistro(c),
			Effetti: a.effettiNonIncluso(c)})
	}
	return append(out, a.esclusiLegacyEdEreditati(presenti)...)
}

// caselleAssenti: le voci raggiunte e non tolte del registro della versione che la bozza non rappresenta (ne' una casella di
// un nodo, ne' una non_includere) e che non stanno fra i fuori per decisione: l'origine «assenza» di esclusi[] (EM §7.8, U2
// §6.1). La stessa funzione per R4 (le letture delle righe da chiudere e la parte pura) e per R5 (il piano). Senza registro o
// senza sorgente: nessuna.
func caselleAssenti(s Snapshot3, reg *importazione.Registro) []Casella3 {
	if reg == nil || s.Sorgente == nil {
		return nil
	}
	presenti := map[string]bool{}
	for _, n := range s.Nodi {
		for _, c := range n.Caselle {
			presenti[c.AllegatoID.String()+"|"+c.Rif] = true
		}
	}
	for _, d := range s.Decisioni {
		if d.Tipo == DecisioneNonIncludere && d.Casella != nil {
			presenti[d.Casella.AllegatoID.String()+"|"+d.Casella.Rif] = true
		}
	}
	var out []Casella3
	for _, v := range reg.Contenuto.Proposta.Nodi {
		if v.StatoMotore.Tolto != nil && *v.StatoMotore.Tolto || v.StatoMotore.Raggiunto != nil && !*v.StatoMotore.Raggiunto {
			continue
		}
		k := v.Origine.AllegatoID + "|" + v.Origine.Rif
		if presenti[k] {
			continue
		}
		al, err := uuid.Parse(v.Origine.AllegatoID)
		if err != nil {
			continue
		}
		if _, fuori := reg.Contenuto.FuoriPerRif(al, v.Origine.Rif); fuori {
			continue
		}
		presenti[k] = true
		out = append(out, Casella3{AllegatoID: al, Gruppo: s.Sorgente.Gruppo, Rif: v.Origine.Rif})
	}
	return out
}

// registro: registro_decisioni[] (C3 §4.3; EM §8.3): le decisioni con autore, ora e motivo dall'evento che le ha
// introdotte (senza evento: chi chiede il riepilogo), le identita' create o modificate, le accettazioni della proposta
// (autore: chi conferma) e gli archi spostati.
func (a *analisiR4) registro() []VoceRegistroR4 {
	out := []VoceRegistroR4{}
	autore := func(id string) (*uuid.UUID, *time.Time) {
		for i := len(a.in.eventi) - 1; i >= 0; i-- {
			e := a.in.eventi[i]
			if e.Azione != AzDecisioneAggiunta {
				continue
			}
			var d struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(e.Dopo, &d) == nil && d.ID == id {
				u, q := e.UtenteID, e.Quando
				return &u, &q
			}
		}
		if a.in.utente == uuid.Nil {
			return nil, nil
		}
		u := a.in.utente
		return &u, nil
	}
	chi := func() *uuid.UUID {
		if a.in.utente == uuid.Nil {
			return nil
		}
		u := a.in.utente
		return &u
	}
	decs := append([]Decisione3(nil), a.s.Decisioni...)
	sort.Slice(decs, func(i, j int) bool { return decs[i].ID < decs[j].ID })
	for _, d := range decs {
		rim := map[string]any{}
		if d.ChiaveBozza != "" {
			rim["chiave_bozza"] = d.ChiaveBozza
		}
		if d.ComponenteID != nil {
			rim["componente_id"] = d.ComponenteID
		}
		if d.Casella != nil {
			rim["casella"] = d.Casella
		}
		if d.AllegatoID != nil {
			rim["allegato_id"] = d.AllegatoID
		}
		if d.PadreID != nil {
			rim["padre_id"], rim["figlio_id"] = d.PadreID, d.FiglioID
		}
		u, q := autore(d.ID)
		out = append(out, VoceRegistroR4{Voce: "d:" + d.ID, OrigineVoce: "decisione", Tipo: d.Tipo, Rimando: canonico(rim), AutoreID: u, Quando: q,
			Motivo: d.Motivo, Importazione: d.Importazione, Effetto: "decisione " + d.Tipo})
	}
	for _, k := range a.chiavi {
		n := a.nodi[k]
		rim := canonico(map[string]any{"chiave_bozza": k})
		switch n.Op {
		case OpCrea:
			out = append(out, VoceRegistroR4{Voce: "i:" + k, OrigineVoce: "identita", Tipo: OpCrea, Rimando: rim, AutoreID: chi(),
				Effetto: "nuova identita' " + n.Identita.Codice + " (" + a.tipoEffettivo(k) + ")"})
		case OpModifica:
			out = append(out, VoceRegistroR4{Voce: "i:" + k, OrigineVoce: "identita", Tipo: OpModifica, Rimando: rim, AutoreID: chi(),
				Effetto: "identita' " + n.Identita.Codice + " modificata"})
		}
		if n.Identita.Modo == ModoProposta {
			r := map[string]any{"chiave_bozza": k}
			if len(n.Caselle) > 0 {
				r["casella"] = n.Caselle[0]
			}
			out = append(out, VoceRegistroR4{Voce: "p:" + k, OrigineVoce: "identita", Tipo: "accetta_proposta", Rimando: canonico(r), AutoreID: chi(),
				Importazione: a.importazioneDellaVersione(), Effetto: "identita' proposta " + n.Identita.Codice + " accettata con la conferma"})
		}
	}
	for _, e := range a.esclusiLegacyEdEreditati(map[string]bool{}) {
		if e.Origine != OrigineEsclusoLegacy {
			continue
		}
		out = append(out, VoceRegistroR4{Voce: "l:" + e.Casella.Rif, OrigineVoce: "esclusione", Tipo: VoceEsclusioneLegacy,
			Rimando: canonico(map[string]any{"casella": e.Casella}), AutoreID: e.AutoreID, Motivo: e.Motivo,
			Effetto: "nodo escluso nelle pagine di prima: resta fuori dalla BOM"})
	}
	for _, x := range a.archi() {
		if x.Azione == "spostato" {
			out = append(out, VoceRegistroR4{Voce: "a:" + x.Padre.ChiaveBozza + ">" + x.Figlio.ChiaveBozza, OrigineVoce: "arco", Tipo: "spostato",
				Rimando: canonico(map[string]any{"padre": x.Padre.ChiaveBozza, "figlio": x.Figlio.ChiaveBozza}), AutoreID: chi(),
				Effetto: x.Figlio.Codice + " spostato sotto " + x.Padre.Codice})
		}
	}
	return out
}

func (a *analisiR4) importazioneDellaVersione() string {
	if a.s.Sorgente != nil {
		return a.s.Sorgente.Importazione
	}
	return ""
}

// criterio: il criterio che la conferma registrerebbe (C3 §4.3), mai dedotto: con zero figli serve la dichiarazione
// mono_pezzo (caso a zero figli, direttiva 39 §1, §3).
func (a *analisiR4) criterio() CriterioR4 {
	figli := 0
	if a.radice != "" {
		figli = len(a.figli[a.radice])
	}
	valida := a.in.sorgente.Valida
	switch {
	case a.mono != nil && figli == 0 && valida:
		return CriterioR4{Criterio: confermabom.CriterioMonoPezzo, Motivo: "dichiarazione mono_pezzo con il motivo, sullo STEP sorgente confermato"}
	case a.mono != nil && figli == 0:
		return CriterioR4{Criterio: "nessuno", Motivo: "la dichiarazione mono_pezzo vale solo con lo STEP del prodotto confermato come sorgente (S8)"}
	case a.mono != nil:
		return CriterioR4{Criterio: "nessuno", Motivo: "la dichiarazione mono_pezzo non vale con i figli nella bozza"}
	case valida && figli > 0:
		return CriterioR4{Criterio: confermabom.CriterioStep, Motivo: "la radice con lo STEP sorgente confermato e almeno un figlio"}
	case valida:
		return CriterioR4{Criterio: "nessuno", Motivo: "lo STEP sorgente non ha figli nella bozza: senza figli serve la dichiarazione mono_pezzo con il motivo (mai dedotta)"}
	default:
		return CriterioR4{Criterio: "nessuno", Motivo: "la bozza non ha uno STEP sorgente confermato da S8: la conferma lo vuole"}
	}
}

// verifica: verifica_prevista (prima-istanza-1, U2 §5.4), con la condizione (b) letta sullo stato dopo R5 (R39-22). Senza i
// conflitti del motore non e' mai «si'».
func (a *analisiR4) verifica(c CriterioR4, blocchi bool) confermabom.VerificaPrimaIstanza {
	figli := 0
	if a.radice != "" {
		figli = len(a.figli[a.radice])
	}
	in := confermabom.IngressiPrimaIstanza{FigliDellaRadice: figli, BlocchiAperti: blocchi}
	in.RigheDaDecidere, in.RimozioniAperte, in.RigheSecondarieAperte = a.contiPrimaIstanza()
	in.ConflittiCalcolati, in.ConflittiAperti = a.mot.calcolati && a.mot.conflittiC, len(a.mot.conflitti)
	if c.Criterio == confermabom.CriterioStep || c.Criterio == confermabom.CriterioMonoPezzo {
		in.Criterio = c.Criterio
	}
	if a.mono != nil {
		in.MotivoMonoPezzo = a.mono.Motivo
	}
	valida := a.in.sorgente.Valida
	in.FonteConfermata, in.FonteAutorizzata, in.SorgenteUgualeAllaFonte = valida, valida, valida
	in.MotivoParzialeStruttura = "nessuna sorgente confermata"
	if s := a.in.struttura; valida && s != nil {
		in.MotivoParzialeStruttura = s.MotivoParziale
		if s.ConDeroga {
			in.Deroga = &confermabom.DerogaStruttura{Stato: "valida", Motivo: s.MotivoDeroga}
		}
	}
	return confermabom.PrimaIstanza(in)
}

// ugualeAllaBase: contenuto-atto-1 (U2 §6.2; contenutoatto.go) fra la conferma di base formato 3 e la bozza come R5 la
// certificherebbe adesso.
func (a *analisiR4) ugualeAllaBase(c CriterioR4, piano associazioni.EsitiPiano) (bool, []DifferenzaAtto) {
	cb := a.in.confBase
	if cb == nil || cb.formato != FormatoSnapshot3 || cb.snap == nil {
		return false, []DifferenzaAtto{}
	}
	base := ContenutoDaSnapshot(*cb.snap, cb.mappa)
	base.Criterio = cb.criterio
	for i := range base.Componenti {
		if base.Componenti[i].Radice {
			base.Componenti[i].RadiceStepSha = cb.radiceSha
		}
	}
	atto := ContenutoDaSnapshot(a.s, nil)
	atto.Criterio = c.Criterio
	sha := ""
	if a.in.sorgente.Valida {
		sha = a.in.sorgente.Sha256
	} else if fa := a.in.vc.FonteAutorizzata; fa != nil {
		sha = fa.Sha256
	}
	for i := range atto.Componenti {
		if atto.Componenti[i].Radice {
			atto.Componenti[i].RadiceStepSha = sha
		}
	}
	for _, e := range piano.Preassociazioni {
		if e.Applica() {
			atto.Preassociazioni = append(atto.Preassociazioni, VocePreassociazione(Preassociazione3{AllegatoID: e.Allegato, ChiaveBozza: e.ChiaveBozza}))
		}
	}
	for _, e := range piano.Svincoli {
		if e.Esito == associazioni.EsitoApplicabile {
			atto.Preassociazioni = append(atto.Preassociazioni, VoceSvincolo(e.Allegato, e.Componente))
		}
	}
	return ContenutoAtto1(base, true, atto)
}
