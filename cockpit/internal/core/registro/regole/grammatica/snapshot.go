package grammatica

import (
	"sort"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/platform/jsoncanonico"
)

// SnapshotRegole: la grammatica di un cliente decodificata, validata e resa canonica, con il suo hash
// (parte 1 §7.1). In A1 l'ID è l'hash; in A2 diventa una riga di regole_snapshot.
//
// Le versioni dei limiti e delle capacità sono metadati, fuori dall'hash (R43 B, D-08): la stessa grammatica
// validata con limiti diversi ha lo stesso hash; i limiti entrano nell'impronta dell'insieme delle regole,
// perché stanno nell'indice. La versione del router non sta qui: sta nell'identità dell'interpretazione
// (R41 c). Nello snapshot, la grammatica dovrebbe conoscere una costante di motorea, che importa la
// grammatica: sarebbe un ciclo.
type SnapshotRegole struct {
	ClienteID                  uuid.UUID
	EmittenteID                string // GP-1: l'ID dell'emittente di una grammatica partner; vuoto per un cliente
	VersioneSchema             int
	VersioneCanonicalizzazione string                 // jsoncanonico.Versione
	VersioneLimiti             string                 // la versione_limiti dell'indice con cui è stato validato
	VersioneCapacita           string                 // VersioneCapacita: l'elenco dei riservati con cui è stato validato
	Hash                       string                 // sha256 del canonico: l'identità dello snapshot
	Canonico                   []byte                 // il JSON canonico della grammatica normalizzata
	Grammatica                 Grammatica             // normalizzata
	Diagnostiche               []evidenze.Diagnostica // avvisi e note (capacità riservate, riserve); mai un errore
}

// NuovoSnapshot fa Decodifica, Valida con i limiti ricevuti, Normalizza e il JSON canonico. Con un errore lo
// snapshot non esiste: l'errore è un *evidenze.ErroreContratto con tutte le diagnostiche della validazione,
// avvisi compresi, nell'ordine del file.
func NuovoSnapshot(raw []byte, lim Limiti) (SnapshotRegole, error) {
	g, err := Decodifica(raw)
	if err != nil {
		return SnapshotRegole{}, err
	}
	diag := g.Valida(lim)
	var avvisi []evidenze.Diagnostica
	errori := false
	for _, d := range diag {
		if d.Gravita == evidenze.GravitaErrore {
			errori = true
		} else {
			avvisi = append(avvisi, d)
		}
	}
	if errori {
		return SnapshotRegole{}, &evidenze.ErroreContratto{Diagnostiche: diag}
	}
	n := Normalizza(g)
	can, err := jsoncanonico.Codifica(n)
	if err != nil {
		// Dopo Decodifica e Valida non dovrebbe succedere (niente float, UTF-8 già controllato).
		return SnapshotRegole{}, &evidenze.ErroreContratto{Diagnostiche: []evidenze.Diagnostica{{
			Codice:    CodiceJSONNonValido,
			Gravita:   evidenze.GravitaErrore,
			Natura:    evidenze.NaturaContratto,
			Messaggio: "forma canonica della grammatica: " + err.Error(),
		}}}
	}
	emittente := ""
	if n.Emittente != nil {
		emittente = n.Emittente.ID
	}
	return SnapshotRegole{
		ClienteID:                  n.Cliente.ID,
		EmittenteID:                emittente,
		VersioneSchema:             n.VersioneSchema,
		VersioneCanonicalizzazione: jsoncanonico.Versione,
		VersioneLimiti:             lim.Versione,
		VersioneCapacita:           VersioneCapacita,
		Hash:                       jsoncanonico.Impronta(can),
		Canonico:                   can,
		Grammatica:                 n,
		Diagnostiche:               avvisi,
	}, nil
}

// Normalizza dà la forma canonica della grammatica (par.3.4.2 del piano A, canonico-politica). È una copia:
// la grammatica ricevuta non cambia.
//   - Famiglie, forme, etichette, affissi, revisioni, decorazioni, esempi e riserve (e riferimenti e
//     qualificatori, che hanno un ID, e le regole quantita_tabellare) in ordine di ID: l'ordine nel file non ha
//     significato. Le voci della tabella delle classi (classi.go) in ordine di valore, per la stessa ragione; una
//     famiglia senza tabella resta senza (nil), con il canonico di prima.
//   - Gli insiemi (ruoli, categorie, selettori, letterali, separatori della revisione, testi dei
//     qualificatori, intestazioni della quantità) in ordine di byte: un duplicato è già un errore di Valida. Ordinare i letterali non cambia il riconoscimento, perché il
//     compilatore legge la più lunga.
//   - Parti, segmenti, sequenze interne, token sospesi, equivalenze, letture attese e ogni altro elenco
//     restano nell'ordine dato: lì l'ordine conta, o il contratto non dice che non conta.
//   - Una slice vuota diventa nil, così vuota e assente danno lo stesso canonico (par.3.4.3).
//   - Il confine vuoto della base diventa alnum_ascii, il valore predefinito: scritto o sottinteso, è la stessa
//     grammatica.
//
// Le descrizioni entrano nell'hash: «differenza di payload a parità di hash è errore» (parte 1 §9.2).
func Normalizza(g Grammatica) Grammatica {
	n := g
	n.Profilo.Riserve = nil
	for _, r := range g.Profilo.Riserve {
		r.Regole = copia(r.Regole)
		n.Profilo.Riserve = append(n.Profilo.Riserve, r)
	}
	sort.SliceStable(n.Profilo.Riserve, func(i, j int) bool { return n.Profilo.Riserve[i].ID < n.Profilo.Riserve[j].ID })

	n.Famiglie = nil
	for _, f := range g.Famiglie {
		n.Famiglie = append(n.Famiglie, normalizzaFamiglia(f))
	}
	sort.SliceStable(n.Famiglie, func(i, j int) bool { return n.Famiglie[i].ID < n.Famiglie[j].ID })

	n.RiferimentiRFQ = nil
	for _, r := range g.RiferimentiRFQ {
		r.Segmenti = copiaSegmenti(r.Segmenti)
		r.Selettori = insieme(r.Selettori)
		r.Esempi = normalizzaEsempi(r.Esempi)
		n.RiferimentiRFQ = append(n.RiferimentiRFQ, r)
	}
	sort.SliceStable(n.RiferimentiRFQ, func(i, j int) bool { return n.RiferimentiRFQ[i].ID < n.RiferimentiRFQ[j].ID })

	n.Qualificatori = nil
	for _, q := range g.Qualificatori {
		q.Testi = insieme(q.Testi)
		q.Selettori = insieme(q.Selettori)
		n.Qualificatori = append(n.Qualificatori, q)
	}
	sort.SliceStable(n.Qualificatori, func(i, j int) bool { return n.Qualificatori[i].ID < n.Qualificatori[j].ID })

	n.Quantita = normalizzaQuantita(g.Quantita)

	// GP-1: emittente, partner e legami. Senza, restano nil e il canonico è quello di prima.
	if g.Emittente != nil {
		e := *g.Emittente
		n.Emittente = &e
	}
	n.Partner = nil
	for _, p := range g.Partner {
		p.Selettori = insieme(p.Selettori)
		p.Famiglie = insieme(p.Famiglie)
		n.Partner = append(n.Partner, p)
	}
	sort.SliceStable(n.Partner, func(i, j int) bool { return n.Partner[i].Emittente < n.Partner[j].Emittente })
	n.Legami = nil
	for _, l := range g.Legami {
		l.Selettori = insieme(l.Selettori)
		var schema []PezzoLegame
		for _, pz := range l.Schema { // l'ordine dei pezzi conta
			pz.Famiglie = insieme(pz.Famiglie)
			pz.Letterali = insieme(pz.Letterali)
			schema = append(schema, pz)
		}
		l.Schema = schema
		var esempi []EsempioLegame
		for _, e := range l.Esempi {
			e.Attivazione = normalizzaAttivazione(e.Attivazione)
			e.Atteso.Letture = normalizzaLetture(e.Atteso.Letture)
			esempi = append(esempi, e)
		}
		sort.SliceStable(esempi, func(i, j int) bool { return esempi[i].ID < esempi[j].ID })
		l.Esempi = esempi
		n.Legami = append(n.Legami, l)
	}
	sort.SliceStable(n.Legami, func(i, j int) bool { return n.Legami[i].ID < n.Legami[j].ID })
	return n
}

func normalizzaAttivazione(a AttivazioneEsempio) AttivazioneEsempio {
	a.Partner = insieme(a.Partner)
	a.Modelli = insieme(a.Modelli)
	return a
}

func normalizzaLetture(ls []LetturaAttesa) []LetturaAttesa {
	var letture []LetturaAttesa
	for _, l := range ls {
		l.Mancanti = copia(l.Mancanti)
		l.Affissi = copia(l.Affissi)
		l.Decorazioni = copia(l.Decorazioni)
		letture = append(letture, l)
	}
	return letture
}

func normalizzaFamiglia(f FamigliaCodice) FamigliaCodice {
	n := f
	n.Ruoli = append([]Ruolo(nil), f.Ruoli...)
	sort.SliceStable(n.Ruoli, func(i, j int) bool { return n.Ruoli[i] < n.Ruoli[j] })
	n.Categorie = append([]Categoria(nil), f.Categorie...)
	sort.SliceStable(n.Categorie, func(i, j int) bool { return n.Categorie[i] < n.Categorie[j] })
	n.Classi = normalizzaClassi(f.Classi)

	n.Base.Segmenti = copiaSegmenti(f.Base.Segmenti)
	if n.Base.ConfinePrima == "" {
		n.Base.ConfinePrima = ConfineAlnumASCII
	}
	if n.Base.ConfineDopo == "" {
		n.Base.ConfineDopo = ConfineAlnumASCII
	}

	n.Forme = nil
	for _, fo := range f.Forme {
		fo.Selettori = insieme(fo.Selettori)
		fo.SegmentiMancanti = copia(fo.SegmentiMancanti)
		fo.Parti = normalizzaParti(fo.Parti)
		fo.Estensioni = insieme(fo.Estensioni)
		fo.StessaParteDi = insieme(fo.StessaParteDi)
		if fo.ConfinePrima != nil {
			c := *fo.ConfinePrima
			fo.ConfinePrima = &c
		}
		if fo.ConfineDopo != nil {
			c := *fo.ConfineDopo
			fo.ConfineDopo = &c
		}
		n.Forme = append(n.Forme, fo)
	}
	sort.SliceStable(n.Forme, func(i, j int) bool { return n.Forme[i].ID < n.Forme[j].ID })

	n.Etichette = nil
	for _, e := range f.Etichette {
		e.Letterali = insieme(e.Letterali)
		e.Selettori = insieme(e.Selettori)
		n.Etichette = append(n.Etichette, e)
	}
	sort.SliceStable(n.Etichette, func(i, j int) bool { return n.Etichette[i].ID < n.Etichette[j].ID })

	n.Affissi = nil
	for _, a := range f.Affissi {
		a.Letterali = insieme(a.Letterali)
		a.Riconoscimento = insieme(a.Riconoscimento)
		a.Attribuzione = insieme(a.Attribuzione)
		if a.Valore != nil {
			v := *a.Valore
			a.Valore = &v
		}
		n.Affissi = append(n.Affissi, a)
	}
	sort.SliceStable(n.Affissi, func(i, j int) bool { return n.Affissi[i].ID < n.Affissi[j].ID })

	n.Revisioni = nil
	for _, r := range f.Revisioni {
		r.Selettori = insieme(r.Selettori)
		r.Separatori = insieme(r.Separatori)
		r.Segmenti = append([]SegmentoRevisione(nil), r.Segmenti...)
		r.TokenSospesi = append([]TokenSospeso(nil), r.TokenSospesi...)
		r.Equivalenze = append([][2]string(nil), r.Equivalenze...)
		n.Revisioni = append(n.Revisioni, r)
	}
	sort.SliceStable(n.Revisioni, func(i, j int) bool { return n.Revisioni[i].ID < n.Revisioni[j].ID })

	n.Decorazioni = nil
	for _, d := range f.Decorazioni {
		d.Letterali = insieme(d.Letterali)
		d.Selettori = insieme(d.Selettori)
		d.Parti = normalizzaParti(d.Parti)
		n.Decorazioni = append(n.Decorazioni, d)
	}
	sort.SliceStable(n.Decorazioni, func(i, j int) bool { return n.Decorazioni[i].ID < n.Decorazioni[j].ID })

	n.Esempi = normalizzaEsempi(f.Esempi)
	return n
}

func normalizzaParti(ps []Parte) []Parte {
	var out []Parte
	for _, p := range ps {
		p.Letterali = insieme(p.Letterali)
		out = append(out, p)
	}
	return out
}

func normalizzaEsempi(es []EsempioCodice) []EsempioCodice {
	var out []EsempioCodice
	for _, e := range es {
		e.Atteso.Letture = normalizzaLetture(e.Atteso.Letture)
		if e.Attivazione != nil {
			a := normalizzaAttivazione(*e.Attivazione)
			e.Attivazione = &a
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func copiaSegmenti(ss []SegmentoBase) []SegmentoBase { return append([]SegmentoBase(nil), ss...) }

// copia: lo stesso elenco nello stesso ordine, nil se vuoto.
func copia(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return append([]string(nil), s...)
}

// insieme: l'elenco in ordine di byte, nil se vuoto.
func insieme(s []string) []string {
	out := copia(s)
	sort.Strings(out) // il confronto delle stringhe di Go è per byte
	return out
}
