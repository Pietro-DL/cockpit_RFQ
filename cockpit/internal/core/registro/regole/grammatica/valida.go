package grammatica

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/estrazione/evidenze"
)

// Valida controlla una grammatica già decodificata, con i limiti ricevuti dall'indice (R43 B): Valida non ha
// limiti suoi. Le regole, con il codice fra parentesi (piano A, par.4.4.4):
//  1. ID non vuoti e unici per tipo, dentro la famiglia; famiglie uniche nel file (contratto.id_vuoto,
//     contratto.id_ripetuto).
//  2. Ogni rif si risolve nella stessa famiglia e nel tipo giusto (contratto.riferimento_pendente).
//  3. Enum chiusi (contratto.enum_ignoto): «parte» e «finito» non sono ruoli, nessun adattatore legacy (R6).
//  4. ruoli non vuoti (contratto.ruoli_vuoti, R17 c); insiemi senza duplicati
//     (contratto.insieme_con_duplicati).
//  5. Ogni selettore passa evidenze.LeggiSelettore, che dà contratto.selettore_non_ammesso; quelli riservati
//     in A1 e i generici «<contesto>.*» danno capacita.non_supportata (R20 c, D-12). «storia» è attivo
//     (R48 A).
//  6. Attribuzione ⊆ riconoscimento (contratto.attribuzione_fuori_riconoscimento).
//  7. Ogni parte di una forma è dichiarata sui selettori della forma (contratto.parte_fuori_selettore).
//  8. Forme parziali come proiezioni dichiarate (contratto.forma_parziale_non_proiezione); una proiezione
//     che toglie segmenti non in coda è riservata in A1 (capacita.non_supportata, R20 c).
//  9. Min ∈ {0, 1}, Max = 1; base, separatore e ripetizione_base con Min 1; una base sola per forma
//     (contratto.cardinalita_non_valida).
//  10. Un'etichetta obbligatoria compare con Min 1 in ogni forma della famiglia
//     (contratto.etichetta_obbligatoria_assente, A-C08).
//  11. Uno stesso selettore non ha, nella stessa famiglia, sia forme sia una revisione in campo separato
//     (contratto.selettore_conteso).
//  12. Le classi *_punto_cifra solo come confine destro (contratto.confine_non_ammesso).
//  13. Pattern (VerificaPattern) e letterali: niente controlli né caratteri di formato, niente letterale
//     vuoto (grammatica.*).
//  14. I limiti di validazione dell'indice (limite.superato, errore).
//  15. Ogni famiglia ha almeno un esempio, ogni forma attiva almeno un esempio positivo
//     (contratto.campo_obbligatorio). Una famiglia con sole forme riservate è ammessa.
//
// Gli esiti sono tre (C-10): errore (lo snapshot non nasce); riservato con diagnosi (capacita.non_supportata,
// grammatica.forma_riservata, grammatica.riserva: quell'elemento non è attivo, il resto sì); attivo.
// Con limiti non validi (Limiti.Valida) non si guarda la grammatica: nessuna si attiva.
//
// Le diagnostiche seguono l'ordine del file; due chiamate danno lo stesso elenco. I percorsi usano l'ID
// degli elementi quando c'è («famiglie_codice[acme].forme[nome].parti[2]»), la posizione altrimenti.
func (g Grammatica) Valida(lim Limiti) []evidenze.Diagnostica {
	if d := lim.Valida(); len(d) > 0 {
		return d
	}
	v := &validatore{lim: lim}
	v.grammatica(g)
	return v.out
}

type validatore struct {
	lim      Limiti
	out      []evidenze.Diagnostica
	famiglie map[string]*FamigliaCodice // per ID: solo per cercare, mai per scorrere
	elenco   []FamigliaCodice           // nell'ordine del file
}

// errore aggiunge un errore di contratto; il codice è sempre una costante di codici_diagnostica.go.
func (v *validatore) errore(d evidenze.Diagnostica) {
	d.Gravita = evidenze.GravitaErrore
	if d.Natura == "" {
		d.Natura = evidenze.NaturaContratto
	}
	v.out = append(v.out, d)
}

func (v *validatore) aggiungi(d ...evidenze.Diagnostica) { v.out = append(v.out, d...) }

// riservato: una capacità nota ma non attiva in A1 (avviso).
func (v *validatore) riservato(percorso, cosa string, rif ...string) {
	v.aggiungi(evidenze.Diagnostica{
		Codice:    CodiceCapacitaNonSupportata,
		Gravita:   evidenze.GravitaAvviso,
		Natura:    evidenze.NaturaCapacita,
		Percorso:  percorso,
		Messaggio: cosa + ": riservato in A1, non attivo; il resto della grammatica sì (R20 c)",
		Rif:       rif,
	})
}

func elemento(base, id string, i int) string {
	if id != "" {
		return fmt.Sprintf("%s[%s]", base, id)
	}
	return fmt.Sprintf("%s[%d]", base, i)
}

// ---- controlli di base, riusati ovunque ----

func (v *validatore) enum(percorso, cosa, valore string, ammessi []string) bool {
	if in(valore, ammessi) {
		return true
	}
	v.errore(evidenze.Diagnostica{
		Codice:    CodiceEnumIgnoto,
		Percorso:  percorso,
		Messaggio: fmt.Sprintf("%s %q fuori elenco (ammessi: %s)", cosa, valore, strings.Join(ammessi, ", ")),
	})
	return false
}

func (v *validatore) enumFacoltativo(percorso, cosa, valore string, ammessi []string) {
	if valore != "" {
		v.enum(percorso, cosa, valore, ammessi)
	}
}

func (v *validatore) obbligatorio(percorso, cosa string) {
	v.errore(evidenze.Diagnostica{Codice: CodiceCampoObbligatorio, Percorso: percorso, Messaggio: "manca " + cosa})
}

func (v *validatore) nonAmmesso(percorso, cosa string) {
	v.errore(evidenze.Diagnostica{Codice: CodiceEnumIgnoto, Percorso: percorso, Messaggio: cosa})
}

// ids controlla ID non vuoti e unici in un elenco dello stesso tipo.
func (v *validatore) ids(base string, ids []string) {
	var visti []string
	for i, id := range ids {
		p := elemento(base, id, i)
		if id == "" {
			v.errore(evidenze.Diagnostica{Codice: CodiceIDVuoto, Percorso: p, Messaggio: "ID mancante"})
			continue
		}
		if in(id, visti) {
			v.errore(evidenze.Diagnostica{Codice: CodiceIDRipetuto, Percorso: p, Messaggio: fmt.Sprintf("l'ID %q è già usato da un elemento dello stesso tipo", id)})
			continue
		}
		visti = append(visti, id)
	}
}

// duplicati segnala i valori ripetuti di un insieme.
func (v *validatore) duplicati(base string, valori []string) {
	var visti []string
	for i, x := range valori {
		if in(x, visti) {
			v.errore(evidenze.Diagnostica{Codice: CodiceInsiemeConDuplicati, Percorso: fmt.Sprintf("%s[%d]", base, i), Messaggio: fmt.Sprintf("%q ripetuto: è un insieme", x)})
			continue
		}
		visti = append(visti, x)
	}
}

func (v *validatore) superato(percorso, cosa string, n, massimo int) {
	v.errore(evidenze.Diagnostica{
		Codice:    CodiceLimiteSuperato,
		Natura:    evidenze.NaturaLimite,
		Percorso:  percorso,
		Messaggio: fmt.Sprintf("%s: %d, il limite dell'indice è %d", cosa, n, massimo),
	})
}

// letterale: non vuoto, senza controlli né caratteri di formato (U+0020 sì), entro la lunghezza massima.
func (v *validatore) letterale(percorso, s string) {
	if s == "" {
		v.errore(evidenze.Diagnostica{Codice: CodiceLetteraleVuoto, Percorso: percorso, Messaggio: "letterale vuoto"})
		return
	}
	if strings.ContainsFunc(s, vietatoNelLetterale) {
		v.errore(evidenze.Diagnostica{Codice: CodiceCarattereDiControllo, Percorso: percorso, Messaggio: messaggioControllo("letterale", s)})
	}
	if massimo := v.lim.Grammatica.MaxLunghezzaLetterale; len(s) > massimo {
		v.superato(percorso, "letterale lungo (byte)", len(s), massimo)
	}
}

func (v *validatore) letterali(base string, ls []string) {
	for i, s := range ls {
		v.letterale(fmt.Sprintf("%s[%d]", base, i), s)
	}
	v.duplicati(base, ls)
}

func (v *validatore) pattern(percorso, p string) {
	v.aggiungi(VerificaPattern(percorso, p, v.lim)...)
}

// selettore controlla un selettore e dice se è attivo: valido e non riservato.
func (v *validatore) selettore(percorso, s string) bool {
	if selettoreGenerico(s) {
		v.riservato(percorso, fmt.Sprintf("il selettore generico %q", s))
		return false
	}
	sel, err := evidenze.LeggiSelettore(s)
	if err != nil {
		// Passa avanti le diagnostiche della foglia, con il percorso nella grammatica (R41 b).
		var ec *evidenze.ErroreContratto
		if errors.As(err, &ec) {
			for _, d := range ec.Diagnostiche {
				d.Percorso = percorso
				v.aggiungi(d)
			}
		}
		return false
	}
	if selettoreRiservato(sel) {
		v.riservato(percorso, fmt.Sprintf("il selettore %q", s))
		return false
	}
	return true
}

// selettori controlla un elenco di selettori obbligatorio; dice quanti sono attivi.
func (v *validatore) selettori(base string, ss []string) int {
	if len(ss) == 0 {
		v.obbligatorio(base, "l'elenco dei selettori")
		return 0
	}
	attivi := 0
	for i, s := range ss {
		if v.selettore(fmt.Sprintf("%s[%d]", base, i), s) {
			attivi++
		}
	}
	v.duplicati(base, ss)
	return attivi
}

func (v *validatore) confine(percorso string, c ClasseConfine, sinistro bool) {
	if !v.enum(percorso, "classe di confine", string(c), classiConfine) {
		return
	}
	if sinistro && in(string(c), classiSoloADestra) {
		v.errore(evidenze.Diagnostica{
			Codice:    CodiceConfineNonAmmesso,
			Percorso:  percorso,
			Messaggio: fmt.Sprintf("la classe %q vale solo come confine destro", c),
		})
	}
}

// ---- il file ----

func (v *validatore) grammatica(g Grammatica) {
	if g.VersioneSchema != VersioneSchema {
		v.errore(evidenze.Diagnostica{
			Codice:    CodiceVersioneSchemaIgnota,
			Percorso:  "versione_schema",
			Messaggio: fmt.Sprintf("versione_schema %d: questo codice sa leggere solo la %d; nessuna attivazione", g.VersioneSchema, VersioneSchema),
		})
	}
	if g.Cliente.ID == uuid.Nil {
		v.obbligatorio("cliente.id", "l'UUID del cliente: il legame è per UUID, mai per nome")
	}
	if strings.TrimSpace(g.Cliente.RagioneSociale) == "" {
		v.obbligatorio("cliente.ragione_sociale", "la ragione sociale, che serve da controllo contro quella del DB")
	}
	v.profilo(g.Profilo)

	v.famiglie = map[string]*FamigliaCodice{}
	v.elenco = g.Famiglie
	ids := make([]string, len(g.Famiglie))
	for i := range g.Famiglie {
		ids[i] = g.Famiglie[i].ID
		if _, gia := v.famiglie[ids[i]]; !gia && ids[i] != "" {
			v.famiglie[ids[i]] = &g.Famiglie[i]
		}
	}
	if len(g.Famiglie) == 0 {
		v.obbligatorio("famiglie_codice", "almeno una famiglia di codice")
	}
	if n, massimo := len(g.Famiglie), v.lim.Grammatica.MaxFamiglie; n > massimo {
		v.superato("famiglie_codice", "famiglie", n, massimo)
	}
	v.ids("famiglie_codice", ids)
	for i := range g.Famiglie {
		v.famiglia(g.Famiglie[i], elemento("famiglie_codice", g.Famiglie[i].ID, i))
	}

	if len(g.RiferimentiRFQ) > 0 {
		v.riservato("riferimenti_rfq", "i riferimenti RFQ")
		v.riferimenti(g.RiferimentiRFQ)
	}
	if len(g.Qualificatori) > 0 {
		v.riservato("qualificatori_testuali", "i qualificatori testuali")
		v.qualificatori(g.Qualificatori)
	}
}

func (v *validatore) profilo(p Profilo) {
	v.enum("profilo.stato", "stato del profilo", p.Stato, statiProfilo)
	ids := make([]string, len(p.Riserve))
	for i, r := range p.Riserve {
		ids[i] = r.ID
	}
	v.ids("profilo.riserve", ids)
	for i, r := range p.Riserve {
		pr := elemento("profilo.riserve", r.ID, i)
		if strings.TrimSpace(r.Motivo) == "" {
			v.obbligatorio(pr+".motivo", "il motivo della riserva")
		}
		v.duplicati(pr+".regole", r.Regole)
		v.aggiungi(evidenze.Diagnostica{
			Codice:    CodiceRiserva,
			Gravita:   evidenze.GravitaNota,
			Natura:    evidenze.NaturaCapacita,
			Percorso:  pr,
			Messaggio: "parte dichiarata e non attiva: " + r.Motivo,
			Rif:       append([]string{r.ID}, r.Regole...),
		})
	}
}

// ---- la famiglia ----

// regoleFamiglia: le regole di una famiglia per ID, per risolvere i rif delle parti.
type regoleFamiglia struct {
	etichette   map[string]*Etichetta
	affissi     map[string]*Affisso
	revisioni   map[string]*RegolaRevisione
	decorazioni map[string]*Decorazione
}

func indicizza(f FamigliaCodice) regoleFamiglia {
	r := regoleFamiglia{
		etichette:   map[string]*Etichetta{},
		affissi:     map[string]*Affisso{},
		revisioni:   map[string]*RegolaRevisione{},
		decorazioni: map[string]*Decorazione{},
	}
	// Con un ID ripetuto vale il primo: il doppione è già un errore.
	for i := range f.Etichette {
		if _, gia := r.etichette[f.Etichette[i].ID]; !gia {
			r.etichette[f.Etichette[i].ID] = &f.Etichette[i]
		}
	}
	for i := range f.Affissi {
		if _, gia := r.affissi[f.Affissi[i].ID]; !gia {
			r.affissi[f.Affissi[i].ID] = &f.Affissi[i]
		}
	}
	for i := range f.Revisioni {
		if _, gia := r.revisioni[f.Revisioni[i].ID]; !gia {
			r.revisioni[f.Revisioni[i].ID] = &f.Revisioni[i]
		}
	}
	for i := range f.Decorazioni {
		if _, gia := r.decorazioni[f.Decorazioni[i].ID]; !gia {
			r.decorazioni[f.Decorazioni[i].ID] = &f.Decorazioni[i]
		}
	}
	return r
}

func (v *validatore) famiglia(f FamigliaCodice, p string) {
	if strings.TrimSpace(f.Namespace) == "" {
		v.obbligatorio(p+".namespace", "il namespace, lo spazio di confronto degli articoli")
	}
	if len(f.Ruoli) == 0 {
		v.errore(evidenze.Diagnostica{Codice: CodiceRuoliVuoti, Percorso: p + ".ruoli", Messaggio: "una famiglia ha almeno un ruolo fra prodotto e componente (R17 c)"})
	}
	ruoliStr := make([]string, len(f.Ruoli))
	for i, r := range f.Ruoli {
		ruoliStr[i] = string(r)
		v.enum(fmt.Sprintf("%s.ruoli[%d]", p, i), "ruolo", string(r), ruoli)
	}
	v.duplicati(p+".ruoli", ruoliStr)
	catStr := make([]string, len(f.Categorie))
	for i, c := range f.Categorie {
		catStr[i] = string(c)
		v.enum(fmt.Sprintf("%s.categorie[%d]", p, i), "categoria", string(c), categorie)
	}
	v.duplicati(p+".categorie", catStr)

	v.base(f.Base, p+".base")
	r := indicizza(f)
	v.etichette(f.Etichette, p)
	v.affissi(f.Affissi, p)
	v.revisioni(f.Revisioni, p)
	v.decorazioni(f.Decorazioni, p)

	// Le forme.
	if len(f.Forme) == 0 {
		v.obbligatorio(p+".forme", "almeno una forma dichiarata, anche riservata")
	}
	if n, massimo := len(f.Forme), v.lim.Grammatica.MaxFormePerFamiglia; n > massimo {
		v.superato(p+".forme", "forme della famiglia", n, massimo)
	}
	formeIDs := make([]string, len(f.Forme))
	for i := range f.Forme {
		formeIDs[i] = f.Forme[i].ID
	}
	v.ids(p+".forme", formeIDs)
	attive := map[string]bool{}
	for i, fo := range f.Forme {
		if v.forma(fo, f, r, elemento(p+".forme", fo.ID, i)) && fo.ID != "" {
			attive[fo.ID] = true
		}
	}

	v.etichetteObbligatorie(f, p)
	v.selettoriContesi(f, p)

	// Gli esempi.
	if len(f.Esempi) == 0 {
		v.obbligatorio(p+".esempi", "almeno un esempio della famiglia")
	}
	if n, massimo := len(f.Esempi), v.lim.Grammatica.MaxEsempi; n > massimo {
		v.superato(p+".esempi", "esempi della famiglia", n, massimo)
	}
	esIDs := make([]string, len(f.Esempi))
	for i := range f.Esempi {
		esIDs[i] = f.Esempi[i].ID
	}
	v.ids(p+".esempi", esIDs)
	for i, e := range f.Esempi {
		v.esempio(e, f.ID, elemento(p+".esempi", e.ID, i))
	}
	for i, fo := range f.Forme {
		if attive[fo.ID] && !v.haEsempioPositivo(f.ID, fo.ID) {
			v.obbligatorio(elemento(p+".forme", fo.ID, i), "un esempio positivo della forma attiva")
		}
	}
}

func (v *validatore) base(b Base, p string) {
	if len(b.Segmenti) == 0 {
		v.obbligatorio(p+".segmenti", "almeno un segmento della base")
	}
	v.segmenti(b.Segmenti, p+".segmenti")
	v.enum(p+".maiuscole", "maiuscole", b.Maiuscole, maiuscole)
	v.enum(p+".normalizza", "normalizzazione", b.Normalizza, normalizzazioni)
	if b.ConfinePrima != "" {
		v.confine(p+".confine_prima", b.ConfinePrima, true)
	}
	if b.ConfineDopo != "" {
		v.confine(p+".confine_dopo", b.ConfineDopo, false)
	}
}

// segmenti: nomi unici, un letterale o un pattern, il separatore che li precede.
func (v *validatore) segmenti(ss []SegmentoBase, base string) {
	nomi := make([]string, len(ss))
	for i, s := range ss {
		nomi[i] = s.Nome
	}
	v.ids(base, nomi)
	for i, s := range ss {
		p := elemento(base, s.Nome, i)
		switch {
		case s.Letterale == "" && s.Pattern == "":
			v.obbligatorio(p, "il letterale o il pattern del segmento")
		case s.Letterale != "" && s.Pattern != "":
			v.nonAmmesso(p, "letterale e pattern sono alternativi: il segmento ne ha uno solo")
		case s.Letterale != "":
			v.letterale(p+".letterale", s.Letterale)
		default:
			v.pattern(p+".pattern", s.Pattern)
		}
		if s.Separatore != "" {
			v.letterale(p+".separatore", s.Separatore)
		}
	}
}

func (v *validatore) etichette(es []Etichetta, p string) {
	ids := make([]string, len(es))
	for i := range es {
		ids[i] = es[i].ID
	}
	v.ids(p+".etichette", ids)
	for i, e := range es {
		pe := elemento(p+".etichette", e.ID, i)
		if len(e.Letterali) == 0 {
			v.obbligatorio(pe+".letterali", "almeno un letterale dell'etichetta")
		}
		v.letterali(pe+".letterali", e.Letterali)
		if e.SpaziMax < 0 {
			v.errore(evidenze.Diagnostica{Codice: CodiceCardinalitaNonValida, Percorso: pe + ".spazi_max", Messaggio: fmt.Sprintf("spazi_max %d: non può essere negativo", e.SpaziMax)})
		}
		v.selettori(pe+".selettori", e.Selettori)
	}
}

func (v *validatore) affissi(as []Affisso, p string) {
	ids := make([]string, len(as))
	for i := range as {
		ids[i] = as[i].ID
	}
	v.ids(p+".affissi", ids)
	for i, a := range as {
		pa := elemento(p+".affissi", a.ID, i)
		if len(a.Letterali) == 0 {
			v.obbligatorio(pa+".letterali", "almeno un letterale dell'affisso")
		}
		v.letterali(pa+".letterali", a.Letterali)
		v.enum(pa+".posizione", "posizione dell'affisso", a.Posizione, posizioni)
		v.selettori(pa+".riconoscimento", a.Riconoscimento)
		for j, s := range a.Attribuzione {
			ps := fmt.Sprintf("%s.attribuzione[%d]", pa, j)
			if !in(s, a.Riconoscimento) {
				// Un selettore che sta anche nel riconoscimento è già controllato lì: qui non si ripete
				// l'avviso di un riservato.
				v.selettore(ps, s)
				v.errore(evidenze.Diagnostica{
					Codice:    CodiceAttribuzioneFuoriRiconoscimento,
					Percorso:  ps,
					Messaggio: fmt.Sprintf("%q non è fra i selettori di riconoscimento: si attribuisce solo dove si riconosce (parte 1 §5.3)", s),
				})
			}
		}
		v.duplicati(pa+".attribuzione", a.Attribuzione)
		if a.Valore != nil {
			v.enumFacoltativo(pa+".valore.fase", "fase", a.Valore.Fase, fasi)
			v.enumFacoltativo(pa+".valore.destinazione", "destinazione", a.Valore.Destinazione, destinazioni)
		}
	}
}

func (v *validatore) revisioni(rs []RegolaRevisione, p string) {
	ids := make([]string, len(rs))
	for i := range rs {
		ids[i] = rs[i].ID
	}
	v.ids(p+".revisioni", ids)
	for i, r := range rs {
		pr := elemento(p+".revisioni", r.ID, i)
		v.selettori(pr+".selettori", r.Selettori)
		if v.enum(pr+".stato", "stato della revisione", r.Stato, stati) && r.Stato == StatoRiservata {
			v.aggiungi(evidenze.Diagnostica{
				Codice:    CodiceFormaRiservata,
				Gravita:   evidenze.GravitaNota,
				Natura:    evidenze.NaturaCapacita,
				Percorso:  pr,
				Messaggio: "regola di revisione dichiarata e riservata: non attiva",
				Rif:       []string{r.ID},
			})
		}
		v.enum(pr+".sorgente", "sorgente della revisione", r.Sorgente, sorgenti)
		v.letterali(pr+".separatori", r.Separatori)
		if len(r.Segmenti) == 0 && len(r.TokenSospesi) == 0 {
			// D-06: i segmenti possono mancare solo se c'è un token sospeso.
			v.obbligatorio(pr+".segmenti", "almeno un segmento della revisione, o un token sospeso (D-06)")
		}
		nomi := make([]string, len(r.Segmenti))
		for j, s := range r.Segmenti {
			nomi[j] = s.Nome
		}
		v.ids(pr+".segmenti", nomi)
		for j, s := range r.Segmenti {
			ps := elemento(pr+".segmenti", s.Nome, j)
			v.pattern(ps+".pattern", s.Pattern)
			v.enum(ps+".significato", "significato del segmento", s.Significato, significati)
		}
		tok := make([]string, len(r.TokenSospesi))
		for j, t := range r.TokenSospesi {
			tok[j] = t.ID
		}
		v.ids(pr+".token_sospesi", tok)
		for j, t := range r.TokenSospesi {
			v.pattern(elemento(pr+".token_sospesi", t.ID, j)+".pattern", t.Pattern)
		}
		for j, eq := range r.Equivalenze {
			for k, s := range eq {
				v.letterale(fmt.Sprintf("%s.equivalenze[%d][%d]", pr, j, k), s)
			}
		}
	}
}

func (v *validatore) decorazioni(ds []Decorazione, p string) {
	ids := make([]string, len(ds))
	for i := range ds {
		ids[i] = ds[i].ID
	}
	v.ids(p+".decorazioni", ids)
	for i, d := range ds {
		pd := elemento(p+".decorazioni", d.ID, i)
		if v.enum(pd+".tipo", "tipo di decorazione", d.Tipo, tipiDecorazione) && decorazioneRiservata(d.Tipo) {
			v.riservato(pd, fmt.Sprintf("la decorazione di tipo %q", d.Tipo), d.ID)
		}
		if d.Tipo == TipoDecorazioneInvolucro {
			v.enum(pd+".sottotipo", "sottotipo dell'involucro", d.Sottotipo, sottotipiInvolucro)
		} else if d.Sottotipo != "" {
			v.nonAmmesso(pd+".sottotipo", "il sottotipo vale solo per un involucro")
		}
		if d.Tipo == TipoDecorazioneSuffissoDocumento {
			if len(d.Letterali) > 0 || d.Pattern != "" {
				v.nonAmmesso(pd, "un suffisso_documento si descrive con le sue parti, non con letterali o pattern")
			}
			if len(d.Parti) == 0 {
				v.obbligatorio(pd+".parti", "la sequenza interna del suffisso_documento")
			}
			if n, massimo := len(d.Parti), v.lim.Grammatica.MaxPartiPerForma; n > massimo {
				v.superato(pd+".parti", "parti della sequenza interna", n, massimo)
			}
			for j, pt := range d.Parti {
				v.parte(pt, fmt.Sprintf("%s.parti[%d]", pd, j), true)
			}
		} else {
			if len(d.Parti) > 0 {
				v.nonAmmesso(pd+".parti", "le parti valgono solo per un suffisso_documento (un livello, D-03)")
			}
			switch {
			case len(d.Letterali) == 0 && d.Pattern == "":
				v.obbligatorio(pd, "i letterali o il pattern della decorazione")
			case len(d.Letterali) > 0 && d.Pattern != "":
				v.nonAmmesso(pd, "letterali e pattern sono alternativi: la decorazione ne ha uno solo")
			case d.Pattern != "":
				v.pattern(pd+".pattern", d.Pattern)
			default:
				v.letterali(pd+".letterali", d.Letterali)
			}
		}
		v.selettori(pd+".selettori", d.Selettori)
	}
}

// ---- le forme ----

// forma controlla una forma e dice se è attiva: stato attiva, almeno un selettore attivo, nessuna proiezione
// riservata.
func (v *validatore) forma(fo FormaCodice, f FamigliaCodice, r regoleFamiglia, p string) bool {
	attiva := v.selettori(p+".selettori", fo.Selettori) > 0
	if v.enum(p+".stato", "stato della forma", fo.Stato, stati) {
		if fo.Stato == StatoRiservata {
			attiva = false
			v.aggiungi(evidenze.Diagnostica{
				Codice:    CodiceFormaRiservata,
				Gravita:   evidenze.GravitaNota,
				Natura:    evidenze.NaturaCapacita,
				Percorso:  p,
				Messaggio: "forma dichiarata e riservata: non attiva, i suoi esempi non si verificano",
				Rif:       []string{fo.ID},
			})
		}
	} else {
		attiva = false
	}
	if !v.proiezione(fo, f.Base, p) {
		attiva = false
	}
	if fo.ConfinePrima != nil {
		v.confine(p+".confine_prima", *fo.ConfinePrima, true)
	}
	if fo.ConfineDopo != nil {
		v.confine(p+".confine_dopo", *fo.ConfineDopo, false)
	}

	if len(fo.Parti) == 0 {
		v.obbligatorio(p+".parti", "le parti della forma")
	}
	if n, massimo := len(fo.Parti), v.lim.Grammatica.MaxPartiPerForma; n > massimo {
		v.superato(p+".parti", "parti della forma", n, massimo)
	}
	basi := 0
	for i, pt := range fo.Parti {
		pp := fmt.Sprintf("%s.parti[%d]", p, i)
		if pt.Tipo == TipoParteBase {
			basi++
		}
		if !v.parte(pt, pp, false) {
			continue
		}
		v.rif(pt, fo, r, pp)
	}
	if len(fo.Parti) > 0 && basi == 0 {
		v.obbligatorio(p+".parti", "la parte base: anche una forma parziale è una proiezione della base")
	} else if basi > 1 {
		v.errore(evidenze.Diagnostica{Codice: CodiceCardinalitaNonValida, Percorso: p + ".parti", Messaggio: fmt.Sprintf("%d parti base: una forma ne ha una sola", basi)})
	}
	return attiva
}

// proiezione controlla completa e segmenti_mancanti (regola 8). Dice false se la proiezione è riservata.
func (v *validatore) proiezione(fo FormaCodice, b Base, p string) bool {
	pm := p + ".segmenti_mancanti"
	if fo.Completa {
		if len(fo.SegmentiMancanti) > 0 {
			v.errore(evidenze.Diagnostica{Codice: CodiceFormaParzialeNonProiezione, Percorso: pm, Messaggio: "una forma completa non ha segmenti mancanti"})
		}
		return true
	}
	if len(fo.SegmentiMancanti) == 0 {
		v.errore(evidenze.Diagnostica{Codice: CodiceFormaParzialeNonProiezione, Percorso: pm, Messaggio: "una forma parziale dichiara i segmenti che mancano: è una proiezione, non una forma con pezzi facoltativi"})
		return true
	}
	v.duplicati(pm, fo.SegmentiMancanti)
	buoni := true
	for i, nome := range fo.SegmentiMancanti {
		idx := -1
		for j, s := range b.Segmenti {
			if s.Nome == nome {
				idx = j
				break
			}
		}
		switch {
		case idx < 0:
			buoni = false
			v.errore(evidenze.Diagnostica{Codice: CodiceFormaParzialeNonProiezione, Percorso: fmt.Sprintf("%s[%d]", pm, i), Messaggio: fmt.Sprintf("il segmento %q non è della base", nome)})
		case !b.Segmenti[idx].Identitario:
			buoni = false
			v.errore(evidenze.Diagnostica{Codice: CodiceFormaParzialeNonProiezione, Percorso: fmt.Sprintf("%s[%d]", pm, i), Messaggio: fmt.Sprintf("il segmento %q non è identitario: una proiezione toglie solo segmenti identitari", nome)})
		}
	}
	if !buoni {
		return true
	}
	// In coda: i mancanti sono gli ultimi segmenti della base. Altrimenti la proiezione è riservata in A1.
	n := len(fo.SegmentiMancanti)
	if n > len(b.Segmenti) {
		return true // già segnalato come duplicato
	}
	for _, s := range b.Segmenti[len(b.Segmenti)-n:] {
		if !in(s.Nome, fo.SegmentiMancanti) {
			v.riservato(pm, "una proiezione che toglie segmenti non in coda alla base", fo.ID)
			return false
		}
	}
	return true
}

// parte controlla tipo, cardinalità e campi di una parte; dice se il tipo è noto. Interna vuol dire dentro
// un suffisso_documento: lì valgono solo separatore, token e ripetizione_base (D-03).
func (v *validatore) parte(pt Parte, p string, interna bool) bool {
	ammessi := tipiParte
	if interna {
		ammessi = tipiParteInterna
	}
	if !v.enum(p+".tipo", "tipo di parte", pt.Tipo, ammessi) {
		return false
	}
	if (pt.Min != 0 && pt.Min != 1) || pt.Max != 1 {
		v.errore(evidenze.Diagnostica{Codice: CodiceCardinalitaNonValida, Percorso: p, Messaggio: fmt.Sprintf("min %d, max %d: min è 0 o 1, max è sempre 1", pt.Min, pt.Max)})
	} else if pt.Min != 1 && (pt.Tipo == TipoParteBase || pt.Tipo == TipoParteSeparatore || pt.Tipo == TipoParteRipetizioneBase) {
		v.errore(evidenze.Diagnostica{Codice: CodiceCardinalitaNonValida, Percorso: p, Messaggio: fmt.Sprintf("una parte %s non è mai facoltativa: min 1", pt.Tipo)})
	}
	conRif := pt.Tipo == TipoParteEtichetta || pt.Tipo == TipoParteAffisso || pt.Tipo == TipoParteRevisione || pt.Tipo == TipoParteDecorazione
	conLetterali := pt.Tipo == TipoParteSeparatore || pt.Tipo == TipoParteMarcatore || pt.Tipo == TipoParteToken
	if !conRif && pt.Rif != "" {
		v.nonAmmesso(p+".rif", fmt.Sprintf("una parte %s non ha rif", pt.Tipo))
	}
	if conLetterali {
		if len(pt.Letterali) == 0 {
			v.obbligatorio(p+".letterali", "i letterali della parte "+pt.Tipo)
		}
		v.letterali(p+".letterali", pt.Letterali)
	} else if len(pt.Letterali) > 0 {
		v.nonAmmesso(p+".letterali", fmt.Sprintf("una parte %s non ha letterali propri", pt.Tipo))
	}
	if pt.Tipo == TipoParteToken {
		if strings.TrimSpace(pt.Rimando) == "" {
			v.obbligatorio(p+".rimando", "il rimando del token alla domanda o alla decisione che lo spiega (Q1, D10)")
		}
	} else if pt.Rimando != "" {
		v.nonAmmesso(p+".rimando", "il rimando vale solo per un token")
	}
	return true
}

// rif risolve il rif di una parte nella famiglia (regola 2) e controlla che la regola sia dichiarata su
// ogni selettore della forma (regola 7).
func (v *validatore) rif(pt Parte, fo FormaCodice, r regoleFamiglia, p string) {
	var selettori []string
	trovata := false
	switch pt.Tipo {
	case TipoParteEtichetta:
		if e, ok := r.etichette[pt.Rif]; ok && pt.Rif != "" {
			trovata, selettori = true, e.Selettori
		}
	case TipoParteAffisso:
		if a, ok := r.affissi[pt.Rif]; ok && pt.Rif != "" {
			trovata, selettori = true, a.Riconoscimento
		}
	case TipoParteRevisione:
		if rv, ok := r.revisioni[pt.Rif]; ok && pt.Rif != "" {
			if rv.Sorgente != SorgenteInline {
				v.errore(evidenze.Diagnostica{
					Codice:    CodiceRiferimentoPendente,
					Percorso:  p + ".rif",
					Messaggio: fmt.Sprintf("la revisione %q è in campo separato: dentro una forma serve una revisione inline", pt.Rif),
				})
				return
			}
			trovata, selettori = true, rv.Selettori
		}
	case TipoParteDecorazione:
		if d, ok := r.decorazioni[pt.Rif]; ok && pt.Rif != "" {
			trovata, selettori = true, d.Selettori
		}
	default:
		return
	}
	if !trovata {
		v.errore(evidenze.Diagnostica{
			Codice:    CodiceRiferimentoPendente,
			Percorso:  p + ".rif",
			Messaggio: fmt.Sprintf("rif %q: nessuna regola %s con questo ID nella famiglia", pt.Rif, pt.Tipo),
		})
		return
	}
	for _, s := range fo.Selettori {
		if !in(s, selettori) {
			v.errore(evidenze.Diagnostica{
				Codice:    CodiceParteFuoriSelettore,
				Percorso:  p,
				Messaggio: fmt.Sprintf("la regola %s %q non è dichiarata sul selettore %q della forma", pt.Tipo, pt.Rif, s),
				Rif:       []string{pt.Rif},
			})
		}
	}
}

// etichetteObbligatorie: un'etichetta obbligatoria compare con Min 1 in ogni forma (regola 10, A-C08).
func (v *validatore) etichetteObbligatorie(f FamigliaCodice, p string) {
	for _, e := range f.Etichette {
		if !e.Obbligatoria || e.ID == "" {
			continue
		}
		for i, fo := range f.Forme {
			presente := false
			for _, pt := range fo.Parti {
				if pt.Tipo == TipoParteEtichetta && pt.Rif == e.ID && pt.Min == 1 {
					presente = true
					break
				}
			}
			if !presente {
				v.errore(evidenze.Diagnostica{
					Codice:    CodiceEtichettaObbligatoriaAssente,
					Percorso:  elemento(p+".forme", fo.ID, i),
					Messaggio: fmt.Sprintf("l'etichetta obbligatoria %q manca, o è facoltativa: senza l'etichetta nessuna lettura della famiglia (A-C08)", e.ID),
					Rif:       []string{e.ID},
				})
			}
		}
	}
}

// selettoriContesi: un selettore con forme e con una revisione in campo separato (regola 11).
func (v *validatore) selettoriContesi(f FamigliaCodice, p string) {
	var conForme []string
	for _, fo := range f.Forme {
		conForme = append(conForme, fo.Selettori...)
	}
	for i, r := range f.Revisioni {
		if r.Sorgente != SorgenteCampoSeparato {
			continue
		}
		for j, s := range r.Selettori {
			if in(s, conForme) {
				v.errore(evidenze.Diagnostica{
					Codice:    CodiceSelettoreConteso,
					Percorso:  fmt.Sprintf("%s.selettori[%d]", elemento(p+".revisioni", r.ID, i), j),
					Messaggio: fmt.Sprintf("il selettore %q ha forme e una revisione in campo separato nella stessa famiglia", s),
				})
			}
		}
	}
}

// ---- gli esempi ----

func (v *validatore) esempio(e EsempioCodice, famiglia, p string) {
	v.enum(p+".origine", "origine dell'esempio", e.Origine, origini)
	v.selettore(p+".selettore", e.Selettore)
	if e.Testo == "" {
		v.obbligatorio(p+".testo", "il testo dell'esempio")
	}
	a := e.Atteso
	switch {
	case a.Nessuna && len(a.Letture) > 0:
		v.nonAmmesso(p+".atteso", "nessuna lettura e letture attese insieme")
	case a.Nessuna && a.AltreAmmesse:
		v.nonAmmesso(p+".atteso", "nessuna lettura e altre_ammesse insieme")
	case !a.Nessuna && len(a.Letture) == 0:
		v.obbligatorio(p+".atteso", "le letture attese, o nessuna: vero")
	}
	for i, l := range a.Letture {
		pl := fmt.Sprintf("%s.atteso.letture[%d]", p, i)
		fam := famiglia
		if l.Famiglia != "" {
			fam = l.Famiglia
			if _, ok := v.famiglie[fam]; !ok {
				v.errore(evidenze.Diagnostica{Codice: CodiceRiferimentoPendente, Percorso: pl + ".famiglia", Messaggio: fmt.Sprintf("nessuna famiglia %q nel file", fam)})
				continue
			}
		}
		if l.Forma == "" {
			continue
		}
		trovata := false
		if f, ok := v.famiglie[fam]; ok {
			for _, fo := range f.Forme {
				if fo.ID == l.Forma {
					trovata = true
					break
				}
			}
		}
		if !trovata {
			v.errore(evidenze.Diagnostica{Codice: CodiceRiferimentoPendente, Percorso: pl + ".forma", Messaggio: fmt.Sprintf("nessuna forma %q nella famiglia %q", l.Forma, fam)})
		}
	}
}

// haEsempioPositivo: un esempio della grammatica attende una lettura di quella forma (regola 15). Una
// lettura senza famiglia vale per la famiglia dell'esempio.
func (v *validatore) haEsempioPositivo(famiglia, forma string) bool {
	for _, f := range v.elenco {
		for _, e := range f.Esempi {
			for _, l := range e.Atteso.Letture {
				fam := l.Famiglia
				if fam == "" {
					fam = f.ID
				}
				if fam == famiglia && l.Forma == forma {
					return true
				}
			}
		}
	}
	return false
}

// ---- riservati in A1: si controlla la forma, non si attiva niente ----

func (v *validatore) riferimenti(rs []RegolaRiferimento) {
	ids := make([]string, len(rs))
	for i := range rs {
		ids[i] = rs[i].ID
	}
	v.ids("riferimenti_rfq", ids)
	for i, r := range rs {
		p := elemento("riferimenti_rfq", r.ID, i)
		v.enum(p+".stato", "stato del riferimento", r.Stato, stati)
		if len(r.Segmenti) == 0 {
			v.obbligatorio(p+".segmenti", "almeno un segmento del riferimento")
		}
		v.segmenti(r.Segmenti, p+".segmenti")
		v.selettori(p+".selettori", r.Selettori)
		esIDs := make([]string, len(r.Esempi))
		for j := range r.Esempi {
			esIDs[j] = r.Esempi[j].ID
		}
		v.ids(p+".esempi", esIDs)
		for j, e := range r.Esempi {
			pe := elemento(p+".esempi", e.ID, j)
			v.enum(pe+".origine", "origine dell'esempio", e.Origine, origini)
			v.selettore(pe+".selettore", e.Selettore)
			if e.Testo == "" {
				v.obbligatorio(pe+".testo", "il testo dell'esempio")
			}
		}
	}
}

func (v *validatore) qualificatori(qs []QualificatoreTestuale) {
	ids := make([]string, len(qs))
	for i := range qs {
		ids[i] = qs[i].ID
	}
	v.ids("qualificatori_testuali", ids)
	for i, q := range qs {
		p := elemento("qualificatori_testuali", q.ID, i)
		v.enum(p+".stato", "stato del qualificatore", q.Stato, stati)
		v.enum(p+".ambito", "ambito del qualificatore", q.Ambito, ambiti)
		if len(q.Testi) == 0 {
			v.obbligatorio(p+".testi", "almeno un testo del qualificatore")
		}
		v.letterali(p+".testi", q.Testi)
		v.selettori(p+".selettori", q.Selettori)
		v.enumFacoltativo(p+".valore.fase", "fase", q.Valore.Fase, fasi)
		v.enumFacoltativo(p+".valore.destinazione", "destinazione", q.Valore.Destinazione, destinazioni)
	}
}
