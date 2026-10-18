package distintabozza

// L'applicazione delle azioni del vocabolario 2 a uno snapshot, oggetto per oggetto (EM §9.2; PE-03): la stessa regola
// che applica la pagina (web/static/distinta_unica.mjs, applica) e che il server rifa' nella riconciliazione di R2
// (riconcilia3.go). Lavora sul JSON: un nodo, un arco, una decisione e una preassociazione sono oggetti, confrontati nella
// loro forma canonica. La forma dei campi la controlla prima chi chiama: cosi' i casi condivisi della pagina
// (transport/web/e2e/distinta_unica_casi.json, «applica»), che hanno snapshot parziali, si rifanno qui uguali.
//
// Che cosa fa ogni verbo allo snapshot:
//   - nodo_aggiunto aggiunge il nodo del dopo (senza azione_id), che non deve esserci; nodo_tolto toglie il nodo, che deve
//     essere uguale al prima; nodo_modificato e nodo_riusato trovano nel nodo i campi del prima (null = assente) e ci
//     scrivono quelli del dopo (null = toglie il campo);
//   - arco_aggiunto, arco_tolto, arco_spostato, qta_modificata fanno lo stesso con gli archi, per (padre, figlio);
//   - decisione_aggiunta e decisione_tolta, per id; file_preassociato e file_svincolato, per proposta_id;
//   - rebase porta versione_bom_base e firma_base dal prima al dopo: base = {conferma_id, numero, contatore,
//     firma_struttura};
//   - file_lasciato, sorgente_vista e sorgente_rifiutata non cambiano lo snapshot.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// oggettoV2 e' un oggetto JSON per campo.
type oggettoV2 = map[string]json.RawMessage

// statoV2 e' uno snapshot come oggetti per chiave: i quattro elenchi per la loro chiave, il resto (formato, prodotto,
// sorgente, base, firma) com'e'. grado e preDi sono gli indici dei controlli locali: archi che toccano un nodo,
// preassociazioni di un nodo.
type statoV2 struct {
	testa     oggettoV2
	nodi      map[string]oggettoV2
	archi     map[[2]string]oggettoV2
	decisioni map[string]oggettoV2
	pre       map[string]oggettoV2
	grado     map[string]int
	preDi     map[string]int
}

func testoJSON(r json.RawMessage) (string, bool) {
	var s string
	if json.Unmarshal(r, &s) != nil {
		return "", false
	}
	return s, true
}

func campoTesto(o oggettoV2, k string) string {
	s, _ := testoJSON(o[k])
	return s
}

// statoV2Da legge uno snapshot (anche parziale) negli oggetti per chiave.
func statoV2Da(raw []byte) (*statoV2, error) {
	var top oggettoV2
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("lo snapshot non e' un oggetto: %v", err)
	}
	s := &statoV2{testa: oggettoV2{}, nodi: map[string]oggettoV2{}, archi: map[[2]string]oggettoV2{},
		decisioni: map[string]oggettoV2{}, pre: map[string]oggettoV2{}, grado: map[string]int{}, preDi: map[string]int{}}
	elenco := func(k string) ([]oggettoV2, error) {
		var xx []oggettoV2
		if r, ok := top[k]; ok && !nullo(r) {
			if err := json.Unmarshal(r, &xx); err != nil {
				return nil, fmt.Errorf("%s non e' un elenco di oggetti: %v", k, err)
			}
		}
		return xx, nil
	}
	nn, err := elenco("nodi")
	if err != nil {
		return nil, err
	}
	for _, n := range nn {
		s.nodi[campoTesto(n, "chiave_bozza")] = n
	}
	aa, err := elenco("archi")
	if err != nil {
		return nil, err
	}
	for _, a := range aa {
		s.mettiArco(a)
	}
	dd, err := elenco("decisioni")
	if err != nil {
		return nil, err
	}
	for _, d := range dd {
		s.decisioni[campoTesto(d, "id")] = d
	}
	pp, err := elenco("documenti_preassociati")
	if err != nil {
		return nil, err
	}
	for _, p := range pp {
		s.mettiPre(p)
	}
	for k, v := range top {
		switch k {
		case "nodi", "archi", "decisioni", "documenti_preassociati":
		default:
			s.testa[k] = v
		}
	}
	return s, nil
}

func chiaveArco(a oggettoV2) [2]string {
	return [2]string{campoTesto(a, "padre"), campoTesto(a, "figlio")}
}

func (s *statoV2) mettiArco(a oggettoV2) {
	k := chiaveArco(a)
	s.archi[k] = a
	s.grado[k[0]]++
	s.grado[k[1]]++
}

func (s *statoV2) togliArco(k [2]string) {
	delete(s.archi, k)
	s.grado[k[0]]--
	s.grado[k[1]]--
}

func (s *statoV2) mettiPre(p oggettoV2) {
	s.pre[campoTesto(p, "proposta_id")] = p
	s.preDi[campoTesto(p, "chiave_bozza")]++
}

func (s *statoV2) togliPre(id string) {
	if p, ok := s.pre[id]; ok {
		s.preDi[campoTesto(p, "chiave_bozza")]--
		delete(s.pre, id)
	}
}

// snapshotJSON: lo snapshot nella forma canonica, con gli elenchi in ordine (nodi per chiave, archi per padre e figlio,
// decisioni per id, preassociazioni per proposta: lo stesso ordine dello scrittore del formato 3).
func (s *statoV2) snapshotJSON() ([]byte, error) {
	top := map[string]any{}
	for k, v := range s.testa {
		top[k] = v
	}
	nodi := make([]string, 0, len(s.nodi))
	for k := range s.nodi {
		nodi = append(nodi, k)
	}
	sort.Strings(nodi)
	ln := make([]oggettoV2, 0, len(nodi))
	for _, k := range nodi {
		ln = append(ln, s.nodi[k])
	}
	archi := make([][2]string, 0, len(s.archi))
	for k := range s.archi {
		archi = append(archi, k)
	}
	sort.Slice(archi, func(i, j int) bool {
		if archi[i][0] != archi[j][0] {
			return archi[i][0] < archi[j][0]
		}
		return archi[i][1] < archi[j][1]
	})
	la := make([]oggettoV2, 0, len(archi))
	for _, k := range archi {
		la = append(la, s.archi[k])
	}
	dec := make([]string, 0, len(s.decisioni))
	for k := range s.decisioni {
		dec = append(dec, k)
	}
	sort.Strings(dec)
	ld := make([]oggettoV2, 0, len(dec))
	for _, k := range dec {
		ld = append(ld, s.decisioni[k])
	}
	pre := make([]string, 0, len(s.pre))
	for k := range s.pre {
		pre = append(pre, k)
	}
	sort.Strings(pre)
	lp := make([]oggettoV2, 0, len(pre))
	for _, k := range pre {
		lp = append(lp, s.pre[k])
	}
	top["nodi"], top["archi"], top["decisioni"], top["documenti_preassociati"] = ln, la, ld, lp
	return canonicoErr(top)
}

func canonicoErr(v any) ([]byte, error) {
	b := canonico(v)
	if b == nil {
		return nil, fmt.Errorf("non ha una forma canonica")
	}
	return b, nil
}

// stessoOggetto: due oggetti JSON con la stessa forma canonica (null e assente sono la stessa cosa nei campi).
func stessoOggetto(a, b oggettoV2) bool {
	return bytes.Equal(canonico(senzaNull(a)), canonico(senzaNull(b)))
}

func senzaNull(o oggettoV2) oggettoV2 {
	out := make(oggettoV2, len(o))
	for k, v := range o {
		if !nullo(v) {
			out[k] = v
		}
	}
	return out
}

func oggettoDa(campo string, raw json.RawMessage) (oggettoV2, *Errore) {
	var o oggettoV2
	if nullo(raw) {
		return nil, corpo(campo, "manca l'oggetto")
	}
	if err := json.Unmarshal(raw, &o); err != nil || o == nil {
		return nil, corpo(campo, "non e' un oggetto JSON")
	}
	return o, nil
}

// applicaV2 applica un'azione del vocabolario 2 allo stato (le regole in testa al file). Controlla soltanto che l'azione
// trovi quello che dice: un'azione che non torna e' azioni_non_riconciliate. annulla e rifai, che vogliono il giornale, li
// risolve chi chiama in un'applicazione del verbo dell'evento (o del suo inverso).
func (s *statoV2) applicaV2(campo, verbo string, prima, dopo json.RawMessage) *Errore {
	switch verbo {
	case AzNodoAggiunto:
		n, e := oggettoDa(campo+".dopo", senzaAzioneID(dopo))
		if e != nil {
			return e
		}
		k := campoTesto(n, "chiave_bozza")
		if _, c := s.nodi[k]; c {
			return nonRiconciliate(campo, "nodo_aggiunto: il nodo %s c'e' gia'", k)
		}
		s.nodi[k] = n
	case AzNodoTolto:
		n, e := oggettoDa(campo+".prima", prima)
		if e != nil {
			return e
		}
		k := campoTesto(n, "chiave_bozza")
		cur, c := s.nodi[k]
		if !c || !stessoOggetto(cur, n) {
			return nonRiconciliate(campo, "nodo_tolto: il nodo %s non e' quello che l'azione dice di togliere", k)
		}
		delete(s.nodi, k)
	case AzNodoModificato, AzNodoRiusato:
		return nonRiconciliate(campo, "%s vuole la chiave del nodo: lo applica modificaV2", verbo)
	case AzArcoAggiunto:
		a, e := oggettoDa(campo+".dopo", senzaAzioneID(dopo))
		if e != nil {
			return e
		}
		k := chiaveArco(a)
		if _, c := s.archi[k]; c {
			return nonRiconciliate(campo, "arco_aggiunto: l'arco %s → %s c'e' gia'", k[0], k[1])
		}
		s.mettiArco(a)
	case AzArcoTolto:
		a, e := oggettoDa(campo+".prima", prima)
		if e != nil {
			return e
		}
		k := chiaveArco(a)
		cur, c := s.archi[k]
		if !c || !stessoOggetto(cur, a) {
			return nonRiconciliate(campo, "arco_tolto: l'arco %s → %s non e' quello che l'azione dice di togliere", k[0], k[1])
		}
		s.togliArco(k)
	case AzArcoSpostato:
		x, e := oggettoDa(campo+".prima", prima)
		if e != nil {
			return e
		}
		y, e := oggettoDa(campo+".dopo", senzaAzioneID(dopo))
		if e != nil {
			return e
		}
		kx, ky := chiaveArco(x), chiaveArco(y)
		cur, c := s.archi[kx]
		if !c || !stessoOggetto(cur, x) {
			return nonRiconciliate(campo, "arco_spostato: l'arco %s → %s non e' quello che l'azione dice di spostare", kx[0], kx[1])
		}
		if kx[1] != ky[1] || kx[0] == ky[0] {
			return corpo(campo, "arco_spostato cambia il padre dello stesso figlio")
		}
		if _, c := s.archi[ky]; c {
			return nonRiconciliate(campo, "arco_spostato: l'arco %s → %s c'e' gia'", ky[0], ky[1])
		}
		s.togliArco(kx)
		s.mettiArco(y)
	case AzQtaModificata:
		p, e := oggettoDa(campo+".prima", prima)
		if e != nil {
			return e
		}
		d, e := oggettoDa(campo+".dopo", dopo)
		if e != nil {
			return e
		}
		k := chiaveArco(p)
		if chiaveArco(d) != k {
			return corpo(campo, "qta_modificata vuole prima e dopo con lo stesso padre e figlio")
		}
		cur, c := s.archi[k]
		if !c || !bytes.Equal(canonico(cur["qta"]), canonico(p["qta"])) {
			return nonRiconciliate(campo, "qta_modificata: l'arco %s → %s non ha la qta che l'azione dice di trovare", k[0], k[1])
		}
		nuovo := oggettoV2{}
		for kk, v := range cur {
			nuovo[kk] = v
		}
		nuovo["qta"] = d["qta"]
		s.archi[k] = nuovo
	case AzDecisioneAggiunta:
		d, e := oggettoDa(campo+".dopo", dopo)
		if e != nil {
			return e
		}
		id := campoTesto(d, "id")
		if _, c := s.decisioni[id]; c {
			return nonRiconciliate(campo, "decisione_aggiunta: la decisione %s c'e' gia'", id)
		}
		s.decisioni[id] = d
	case AzDecisioneTolta:
		d, e := oggettoDa(campo+".prima", prima)
		if e != nil {
			return e
		}
		id := campoTesto(d, "id")
		cur, c := s.decisioni[id]
		if !c || !stessoOggetto(cur, d) {
			return nonRiconciliate(campo, "decisione_tolta: la decisione %s non e' quella che l'azione dice di togliere", id)
		}
		delete(s.decisioni, id)
	case AzFilePreassociato:
		p, e := oggettoDa(campo+".dopo", senzaAzioneID(dopo))
		if e != nil {
			return e
		}
		id := campoTesto(p, "proposta_id")
		if _, c := s.pre[id]; c {
			return nonRiconciliate(campo, "file_preassociato: la proposta %s e' gia' preassociata", id)
		}
		s.mettiPre(oggettoV2{"proposta_id": p["proposta_id"], "chiave_bozza": p["chiave_bozza"]})
	case AzFileSvincolato:
		p, e := oggettoDa(campo+".prima", prima)
		if e != nil {
			return e
		}
		id := campoTesto(p, "proposta_id")
		cur, c := s.pre[id]
		if !c || campoTesto(cur, "chiave_bozza") != campoTesto(p, "chiave_bozza") {
			return nonRiconciliate(campo, "file_svincolato: la proposta %s non e' preassociata a %s", id, campoTesto(p, "chiave_bozza"))
		}
		s.togliPre(id)
	case AzRebase:
		return s.rebaseV2(campo, prima, dopo)
	case AzFileLasciato, AzSorgenteVista, AzSorgenteRifiutata:
		// non cambiano lo snapshot
	default:
		return corpo(campo+".azione", "azione %q non applicabile allo snapshot", verbo)
	}
	return nil
}

// modificaV2 applica nodo_modificato e nodo_riusato al nodo chiave: il prima deve coincidere con il nodo sui campi che
// nomina (null = assente), il dopo da' i valori nuovi (null = toglie il campo). Restituisce il nodo di prima e quello di
// dopo; i campi ammessi e la forma del nodo che ne esce li controlla chi chiama.
func (s *statoV2) modificaV2(campo, verbo, chiave string, prima, dopo json.RawMessage) (oggettoV2, oggettoV2, *Errore) {
	cur, c := s.nodi[chiave]
	if !c {
		return nil, nil, nonRiconciliate(campo, "%s: il nodo %s non c'e'", verbo, chiave)
	}
	p, e := oggettoDa(campo+".prima", prima)
	if e != nil {
		return nil, nil, e
	}
	d, e := oggettoDa(campo+".dopo", dopo)
	if e != nil {
		return nil, nil, e
	}
	if len(p) == 0 || len(p) != len(d) {
		return nil, nil, corpo(campo, "%s: prima e dopo nominano gli stessi campi, almeno uno", verbo)
	}
	for k := range p {
		if _, ok := d[k]; !ok {
			return nil, nil, corpo(campo, "%s: prima e dopo nominano gli stessi campi, almeno uno", verbo)
		}
		if k == "chiave_bozza" {
			return nil, nil, corpo(campo, "%s: la chiave del nodo non cambia", verbo)
		}
		atteso, ha := cur[k]
		if nullo(p[k]) {
			if ha && !nullo(atteso) {
				return nil, nil, nonRiconciliate(campo, "%s: il nodo %s ha il campo %s, che l'azione dice assente", verbo, chiave, k)
			}
			continue
		}
		if !ha || !bytes.Equal(canonico(atteso), canonico(p[k])) {
			return nil, nil, nonRiconciliate(campo, "%s: il nodo %s non ha i valori che l'azione dice di trovare (%s)", verbo, chiave, k)
		}
	}
	nuovo := oggettoV2{}
	for k, v := range cur {
		nuovo[k] = v
	}
	for k, v := range d {
		if nullo(v) {
			delete(nuovo, k)
		} else {
			nuovo[k] = v
		}
	}
	if stessoOggetto(nuovo, cur) {
		return nil, nil, nonRiconciliate(campo, "%s: il nodo %s non cambia", verbo, chiave)
	}
	s.nodi[chiave] = nuovo
	return cur, nuovo, nil
}

// baseV2 e' la base del vocabolario 2 (EM §9.2, il rebase della sola base): la versione corrente che R1 restituisce.
type baseV2 struct {
	ConfermaID     json.RawMessage `json:"conferma_id"`
	Numero         json.RawMessage `json:"numero"`
	Contatore      json.RawMessage `json:"contatore"`
	FirmaStruttura json.RawMessage `json:"firma_struttura"`
}

func (b baseV2) versioneBom() oggettoV2 {
	id := b.ConfermaID
	if nullo(id) {
		id = json.RawMessage("null")
	}
	return oggettoV2{"conferma_id": id, "numero": b.Numero, "contatore": b.Contatore}
}

func leggiBaseV2(campo string, raw json.RawMessage) (baseV2, *Errore) {
	var x struct {
		Base *baseV2 `json:"base"`
	}
	if err := json.Unmarshal(raw, &x); err != nil || x.Base == nil {
		return baseV2{}, corpo(campo+".base", "il rebase dice la base: {conferma_id, numero, contatore, firma_struttura}")
	}
	return *x.Base, nil
}

func (s *statoV2) rebaseV2(campo string, prima, dopo json.RawMessage) *Errore {
	p, e := leggiBaseV2(campo+".prima", prima)
	if e != nil {
		return e
	}
	d, e := leggiBaseV2(campo+".dopo", dopo)
	if e != nil {
		return e
	}
	vb := oggettoV2{}
	if r, ok := s.testa["versione_bom_base"]; ok && !nullo(r) {
		if err := json.Unmarshal(r, &vb); err != nil {
			return corpo("snapshot.versione_bom_base", "non e' un oggetto")
		}
	}
	if !stessoOggetto(vb, p.versioneBom()) || !bytes.Equal(canonico(s.testa["firma_base"]), canonico(p.FirmaStruttura)) {
		return nonRiconciliate(campo, "rebase: la base di prima non e' quella della bozza")
	}
	if stessoOggetto(p.versioneBom(), d.versioneBom()) && bytes.Equal(canonico(p.FirmaStruttura), canonico(d.FirmaStruttura)) {
		return nonRiconciliate(campo, "rebase: la base non cambia")
	}
	s.testa["versione_bom_base"] = canonico(d.versioneBom())
	s.testa["firma_base"] = d.FirmaStruttura
	return nil
}

// inversaV2: l'azione che disfa un evento del vocabolario 2 (annulla), con prima e dopo dell'evento. file_lasciato non
// cambia lo snapshot e il suo inverso nemmeno.
func inversaV2(verbo string, prima, dopo json.RawMessage) (string, json.RawMessage, json.RawMessage, bool) {
	switch verbo {
	case AzNodoAggiunto:
		return AzNodoTolto, senzaAzioneID(dopo), nil, true
	case AzNodoTolto:
		return AzNodoAggiunto, nil, prima, true
	case AzArcoAggiunto:
		return AzArcoTolto, senzaAzioneID(dopo), nil, true
	case AzArcoTolto:
		return AzArcoAggiunto, nil, prima, true
	case AzArcoSpostato:
		return AzArcoSpostato, senzaAzioneID(dopo), prima, true
	case AzNodoModificato, AzNodoRiusato, AzQtaModificata:
		return verbo, dopo, prima, true
	case AzDecisioneAggiunta:
		return AzDecisioneTolta, dopo, nil, true
	case AzDecisioneTolta:
		return AzDecisioneAggiunta, nil, prima, true
	case AzFilePreassociato:
		return AzFileSvincolato, senzaAzioneID(dopo), nil, true
	case AzFileSvincolato:
		return AzFilePreassociato, nil, prima, true
	case AzFileLasciato:
		return AzFileLasciato, nil, dopo, true
	}
	return "", nil, nil, false
}
