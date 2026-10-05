package motorea

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// Gli attributi (5.4.6 punto 12; A-C02: mai al codice più vicino nel testo). Un attributo si lega sempre
// all'entità dell'unità che lo porta (UnitaEvidenza.EntitaID: il campo appartiene all'entità per i fatti,
// legami-1), e un'unità senza entità non dà attributi. Il legame fra un attributo e un codice letto nasce solo
// con una regola che lo giustifica (la RegolaRevisione della famiglia per la revisione), e allora lo dice
// LettureCompatibili; la quantità resta legata alla riga, e chi lega prodotto e quantità attraverso la riga è
// A1c.

// attributi calcola revisioni in campo separato, formazioni STEP, titolo, scala e materiale, quantità. La
// revisione in campo separato e la quantità dipendono dalle letture: per un'entità o una tabella con un'unità non
// letta per intero (un limite superato, punto 2) non nascono, e lo dice limite.superato con le entità in Rif. Una
// regola applicata su letture tagliate darebbe un esito che a lettura completa sarebbe un altro.
func (it *interprete) attributi() {
	perEntita := it.lettureDelleEntita()
	nonLette := it.entitaNonLette()
	var formazioni, saltate []string
	for _, u := range it.unita {
		if u.EntitaID == "" || u.Testo == "" {
			continue
		}
		switch {
		case u.Selettore.Campo.Variante == evidenze.VarianteStep && u.Selettore.Campo.Valore == "revisione":
			formazioni = append(formazioni, it.formazione(u))
		case u.Selettore.Contesto == evidenze.ContestoCartiglio && contiene([]string{AttributoTitolo, AttributoScala, AttributoMateriale}, u.Selettore.Campo.Valore):
			it.grezzo(u)
		case nonLette[u.EntitaID]:
			if len(it.m.revisioniCampo[u.Selettore]) > 0 || len(it.regole.riservate[u.Selettore]) > 0 {
				saltate = append(saltate, u.EntitaID)
			}
		default:
			it.revisioneCampo(u, perEntita[u.EntitaID])
		}
	}
	it.notaFormazioni(formazioni)
	saltate = append(saltate, it.quantita(nonLette)...)
	it.attributiSaltati(saltate)
}

// entitaNonLette: le entità con almeno un'unità del documento non letta per intero (fuori da
// max_unita_documento, dopo il taglio di max_letture_documento, o tagliata dai limiti di Riconosci).
func (it *interprete) entitaNonLette() map[string]bool {
	out := map[string]bool{}
	for _, u := range it.doc.Unita {
		if u.EntitaID != "" && !it.lette[u.ID] {
			out[u.EntitaID] = true
		}
	}
	return out
}

// attributiSaltati: una diagnostica limite.superato per le entità che non hanno avuto la revisione in campo
// separato o la quantità perché non lette per intero.
func (it *interprete) attributiSaltati(entita []string) {
	if len(entita) == 0 {
		return
	}
	sort.Strings(entita)
	var rif []string
	for _, e := range entita {
		if !contiene(rif, e) {
			rif = append(rif, e)
		}
	}
	it.limite = true
	it.diag = append(it.diag, evidenze.Diagnostica{
		Codice:    grammatica.CodiceLimiteSuperato,
		Gravita:   evidenze.GravitaAvviso,
		Natura:    evidenze.NaturaLimite,
		Percorso:  "attributi",
		Messaggio: "entità non lette per intero per un limite superato: per loro nessuna revisione in campo separato e nessuna quantità, risultato parziale",
		Rif:       rif,
	})
}

// lettureDelleEntita: per ogni entità, gli indici delle letture delle sue unità (l'unità principale della
// lettura). È la provenance «stessa entità» delle revisioni.
func (it *interprete) lettureDelleEntita() map[string][]int {
	out := map[string][]int{}
	for i, l := range it.letture {
		if e := it.unitaPer[l.UnitaID].EntitaID; e != "" {
			out[e] = append(out[e], i)
		}
	}
	return out
}

// ---- la formazione STEP ----

// formazione: il campo «revisione» di un nodo STEP è la formazione grezza, un tipo a sé, attribuito al nodo,
// senza letture compatibili e senza confronto con nessuna revisione (D1; E-12). Provenance: il nodo
// (EntitaID). Regola: il punto 12 del piano, che non ne fa mai una revisione. Più formazioni sullo stesso nodo
// sono più unità, quindi più attributi; la diagnostica step.formazioni_alternative la dà già l'adattatore,
// nella qualità della fonte.
func (it *interprete) formazione(u evidenze.UnitaEvidenza) string {
	a := AttributoLetto{
		ID:       "a:" + u.ID + ":" + AttributoFormazione,
		Tipo:     AttributoFormazione,
		Grezzo:   u.Testo,
		UnitaID:  u.ID,
		EntitaID: u.EntitaID,
		Stato:    StatoAttribuito,
	}
	it.attr = append(it.attr, a)
	return a.ID
}

// notaFormazioni: revisione.formazione_non_confrontabile, una per documento, con gli ID delle formazioni e
// delle revisioni lette nello stesso documento (in linea e in campo separato): nessun confronto fra le due.
func (it *interprete) notaFormazioni(formazioni []string) {
	if len(formazioni) == 0 {
		return
	}
	var revisioni []string
	for _, l := range it.letture {
		if l.Forma.Revisione != nil {
			revisioni = append(revisioni, l.ID)
		}
	}
	for _, a := range it.attr {
		if a.Tipo == AttributoRevisione {
			revisioni = append(revisioni, a.ID)
		}
	}
	sort.Strings(formazioni)
	sort.Strings(revisioni)
	it.diag = append(it.diag, evidenze.Diagnostica{
		Codice:    CodiceRevisioneFormazioneNonConfrontabile,
		Gravita:   evidenze.GravitaNota,
		Natura:    evidenze.NaturaDati,
		Percorso:  "attributi",
		Messaggio: fmt.Sprintf("%d formazioni STEP: dati grezzi, mai confrontati con le %d revisioni lette nel documento (D1)", len(formazioni), len(revisioni)),
		Rif:       append(formazioni, revisioni...),
	})
}

// ---- titolo, scala, materiale ----

// grezzo: titolo, scala e materiale del cartiglio, solo il grezzo, legato al disegno, senza normalizzazione.
// Provenance: il disegno (EntitaID del campo del cartiglio). Regola: la riga 13 del router e il punto 12 del piano.
// Nessun legame con un codice.
func (it *interprete) grezzo(u evidenze.UnitaEvidenza) {
	tipo := u.Selettore.Campo.Valore
	it.attr = append(it.attr, AttributoLetto{
		ID:       "a:" + u.ID + ":" + tipo,
		Tipo:     tipo,
		Grezzo:   u.Testo,
		UnitaID:  u.ID,
		EntitaID: u.EntitaID,
		Stato:    StatoAttribuito,
	})
}

// ---- la revisione in campo separato ----

// regolaCampo: una regola di revisione in campo separato candidata per un'unità: attiva, con il suo piano, o
// riservata.
type regolaCampo struct {
	famiglia, regola string
	piano            *pianoRevisione // nil per una regola riservata
}

// revisioneCampo: la revisione in campo separato (punto 12, primo trattino), per un'unità il cui selettore ha
// regole di revisione in campo separato, attive o riservate.
//   - Provenance: l'entità (EntitaID). Le famiglie che contano sono quelle lette nella STESSA entità; senza
//     letture nell'entità, vale l'unica regola attiva sul selettore, se ce n'è una sola.
//   - Regola: la RegolaRevisione con sorgente campo_separato di quella famiglia. Il valore intero deve rispettarla:
//     allora «attribuito», con il normalizzato, e LettureCompatibili sono le letture della famiglia nell'entità
//     che non discordano. Altrimenti, o con la regola riservata (Q1), «non_interpretabile», con l'originale e
//     revisione.non_interpretabile.
//   - Una revisione in linea della stessa entità e della stessa famiglia che non concorda (ConfrontaRevisioni)
//     dà revisione.discordante: si conservano tutte e due.
//   - Famiglie lette nell'entità senza una regola sul selettore: «non_attribuito», il campo c'è e nessuna regola
//     lo attribuisce. Più regole attive senza letture nell'entità: «ambiguo», nessuna si sceglie.
//   - Nessuna regola di revisione in campo separato sul selettore, né attiva né riservata, e il campo è la
//     revisione del cartiglio: il default prudente (C-34; R21 e = A; P1 §3.4 r.110). Nessun valore, l'originale
//     si conserva, stato «non_interpretabile», con revisione.non_interpretabile. Il legame è solo quello del
//     campo con la sua entità, nessun codice: è il caso della grammatica che non ha una regola per quella
//     revisione, perché nessuno gliel'ha data.
func (it *interprete) revisioneCampo(u evidenze.UnitaEvidenza, letture []int) {
	piani := it.m.revisioniCampo[u.Selettore]
	riservate := it.regole.riservate[u.Selettore]
	if len(piani) == 0 && len(riservate) == 0 {
		if u.Selettore.Contesto == evidenze.ContestoCartiglio && u.Selettore.Campo.Valore == AttributoRevisione {
			it.revisioneSenzaRegola(u)
		}
		return
	}
	var famiglie []string
	for _, i := range letture {
		if f := it.letture[i].Forma.Famiglia; !contiene(famiglie, f) {
			famiglie = append(famiglie, f)
		}
	}
	sort.Strings(famiglie)

	var candidate []regolaCampo
	if len(famiglie) > 0 {
		for _, f := range famiglie {
			attive := false
			for _, p := range piani {
				if p.famiglia == f {
					candidate = append(candidate, regolaCampo{famiglia: f, regola: p.rev.id, piano: p})
					attive = true
				}
			}
			// Le riservate della famiglia contano solo se la famiglia non ha regole attive sul selettore (Q1: «con
			// la regola riservata» vuol dire la regola che si applica): un'attiva e una riservata insieme darebbero
			// due esiti opposti sullo stesso campo.
			if attive {
				continue
			}
			for _, r := range riservate {
				if r.famiglia == f {
					candidate = append(candidate, regolaCampo{famiglia: f, regola: r.regola})
				}
			}
		}
		if len(candidate) == 0 {
			it.attr = append(it.attr, it.attributoRevisione(u, regolaCampo{}, StatoNonAttribuito, ""))
			return
		}
	} else {
		switch {
		case len(piani) == 1:
			candidate = []regolaCampo{{famiglia: piani[0].famiglia, regola: piani[0].rev.id, piano: piani[0]}}
		case len(piani) > 1:
			it.attr = append(it.attr, it.attributoRevisione(u, regolaCampo{}, StatoAmbiguo, ""))
			return
		default:
			for _, r := range riservate {
				candidate = append(candidate, regolaCampo{famiglia: r.famiglia, regola: r.regola})
			}
		}
	}

	for _, c := range candidate {
		it.applicaRevisione(u, c, letture)
	}
}

// revisioneSenzaRegola: la revisione del cartiglio quando la grammatica non ha nessuna regola di revisione in
// campo separato sul selettore (C-34, il default prudente di R21 e = A): nessuna revisione attribuita, il token
// conservato, lo stato non_interpretabile e la diagnosi.
func (it *interprete) revisioneSenzaRegola(u evidenze.UnitaEvidenza) {
	a := it.attributoRevisione(u, regolaCampo{}, StatoNonInterpretabile, "")
	it.attr = append(it.attr, a)
	it.diag = append(it.diag, evidenze.Diagnostica{
		Codice:    CodiceRevisioneNonInterpretabile,
		Gravita:   evidenze.GravitaAvviso,
		Natura:    evidenze.NaturaDati,
		Percorso:  "attributi[" + a.ID + "]",
		Messaggio: "nessuna regola di revisione in campo separato sul selettore: default prudente (C-34), l'originale si conserva, nessun valore",
		Rif:       []string{a.ID, u.ID},
	})
}

// applicaRevisione applica una regola candidata al testo intero del campo.
func (it *interprete) applicaRevisione(u evidenze.UnitaEvidenza, c regolaCampo, letture []int) {
	var rev *RevisioneLetta
	perche := ""
	switch {
	case c.piano == nil:
		perche = fmt.Sprintf("la regola %q della famiglia %q è riservata (Q1): l'originale si conserva, nessun valore", c.regola, c.famiglia)
	default:
		rev = c.piano.leggi(u.Testo)
		switch {
		case rev == nil:
			perche = fmt.Sprintf("la regola %q della famiglia %q non legge il campo per intero: l'originale si conserva, nessun valore", c.regola, c.famiglia)
		case rev.Stato != StatoRevisioneLetta:
			perche = fmt.Sprintf("la regola %q della famiglia %q legge il campo come token sospeso: conservato, nessun valore", c.regola, c.famiglia)
		}
	}
	if perche != "" {
		a := it.attributoRevisione(u, c, StatoNonInterpretabile, "")
		it.attr = append(it.attr, a)
		it.diag = append(it.diag, evidenze.Diagnostica{
			Codice:    CodiceRevisioneNonInterpretabile,
			Gravita:   evidenze.GravitaAvviso,
			Natura:    evidenze.NaturaDati,
			Percorso:  "attributi[" + a.ID + "]",
			Messaggio: perche,
			Rif:       []string{a.ID, u.ID, c.regola},
		})
		return
	}

	a := it.attributoRevisione(u, c, StatoAttribuito, rev.Normalizzata)
	for _, i := range letture {
		l := it.letture[i]
		if l.Forma.Famiglia != c.famiglia {
			continue
		}
		if l.Forma.Revisione != nil && ConfrontaRevisioni(*rev, *l.Forma.Revisione) == CompatibilitaDiscordante {
			it.diag = append(it.diag, evidenze.Diagnostica{
				Codice:    CodiceRevisioneDiscordante,
				Gravita:   evidenze.GravitaAvviso,
				Natura:    evidenze.NaturaDati,
				Percorso:  "attributi[" + a.ID + "]",
				Messaggio: "la revisione del campo separato e quella in linea della stessa entità non concordano: si conservano tutte e due",
				Rif:       []string{a.ID, l.ID},
			})
			continue
		}
		a.LettureCompatibili = append(a.LettureCompatibili, l.ID)
	}
	sort.Strings(a.LettureCompatibili)
	it.attr = append(it.attr, a)
}

// attributoRevisione: l'attributo di una revisione in campo separato. L'ID porta la famiglia e la regola che lo
// hanno dato, se ci sono («a:<unità>:revisione:<famiglia>/<regola>»): due famiglie lette nella stessa entità
// danno due attributi, e la regola che giustifica il legame resta leggibile nell'ID. Senza regola (non
// attribuito, ambiguo, non interpretabile per il default prudente) l'ID è «a:<unità>:revisione».
func (it *interprete) attributoRevisione(u evidenze.UnitaEvidenza, c regolaCampo, stato, normalizzato string) AttributoLetto {
	id := "a:" + u.ID + ":" + AttributoRevisione
	if c.famiglia != "" {
		id += ":" + c.famiglia + "/" + c.regola
	}
	return AttributoLetto{
		ID:           id,
		Tipo:         AttributoRevisione,
		Grezzo:       u.Testo,
		Normalizzato: normalizzato,
		UnitaID:      u.ID,
		EntitaID:     u.EntitaID,
		Stato:        stato,
	}
}

// ---- la quantità dalla colonna dichiarata ----

// cellaTabella: una cella di una tabella della mail, con la sua riga e la sua colonna.
type cellaTabella struct {
	u       evidenze.UnitaEvidenza
	riga    int // PosTabella.Riga, da 1
	colonna int // PosTabella.Colonna: la colonna effettiva della griglia dell'HTML
}

// quantita: la quantità dalla colonna dichiarata (punto 12, ultimo trattino; R28 a).
//   - Provenance: la stessa tabella e la stessa riga (PosTabella.Tabella, l'entità «riga» delle celle) e la stessa
//     colonna della griglia dell'HTML (PosTabella.Colonna), che dà l'adattatore dai fatti.
//   - Regola: una QuantitaTabellare attiva per il selettore delle celle, con la sua regola
//     (prima_riga_non_vuota_sopra_le_righe_con_codice) e i suoi letterali. Senza, nessun attributo.
//
// Le righe con codice sono quelle con almeno una lettura di famiglia in una loro cella. L'intestazione è la prima
// riga non vuota sopra la prima di quelle righe; la colonna è quella della cella d'intestazione che, ripulita ai
// bordi, vale uno dei letterali. Per ogni riga con codice, la cella di quella colonna dà l'attributo «quantita»
// della riga: il grezzo; il normalizzato solo per un intero di cifre decimali, da 1 a 6, altrimenti
// non_interpretabile con quantita.non_interpretabile; la cella d'intestazione in Evidenze. Se più colonne
// dell'intestazione valgono un letterale, ogni cella è «ambiguo»: nessuna colonna si sceglie.
//
// Una tabella con una riga non letta per intero (nonLette) non dà quantità: le righe con codice e l'intestazione
// si trovano solo su tutte le righe. Restituisce le entità «riga» non lette delle tabelle saltate che avevano una
// regola attiva per il loro selettore.
func (it *interprete) quantita(nonLette map[string]bool) []string {
	var saltate []string
	tabNonLetta := map[int]bool{}
	for _, u := range it.doc.Unita {
		if pt := u.Posizione.Tabella; pt != nil && nonLette[u.EntitaID] {
			tabNonLetta[pt.Tabella] = true
		}
	}
	conLetture := map[string]bool{}
	for _, l := range it.letture {
		conLetture[l.UnitaID] = true
		for _, a := range l.AltreUnita {
			conLetture[a] = true
		}
	}
	tabelle := map[int][]cellaTabella{}
	var ordine []int
	for _, u := range it.unita {
		pt := u.Posizione.Tabella
		if pt == nil || u.EntitaID == "" || u.Testo == "" {
			continue
		}
		if _, ok := tabelle[pt.Tabella]; !ok {
			ordine = append(ordine, pt.Tabella)
		}
		tabelle[pt.Tabella] = append(tabelle[pt.Tabella], cellaTabella{u: u, riga: pt.Riga, colonna: pt.Colonna})
	}
	sort.Ints(ordine)
	for _, t := range ordine {
		if !tabNonLetta[t] {
			it.quantitaDellaTabella(tabelle[t], conLetture)
			continue
		}
		if len(it.regole.quantita[tabelle[t][0].u.Selettore]) == 0 {
			continue
		}
		for _, u := range it.doc.Unita {
			if pt := u.Posizione.Tabella; pt != nil && pt.Tabella == t && nonLette[u.EntitaID] {
				saltate = append(saltate, u.EntitaID)
			}
		}
	}
	return saltate
}

func (it *interprete) quantitaDellaTabella(celle []cellaTabella, conLetture map[string]bool) {
	// Le regole attive per il selettore delle celle. Le celle di una tabella hanno tutte il selettore del segmento
	// in cui la tabella si aggancia; se non fosse così, la tabella non dà quantità.
	sel := celle[0].u.Selettore
	for _, c := range celle {
		if c.u.Selettore != sel {
			return
		}
	}
	regole := it.regole.quantita[sel]
	if len(regole) == 0 {
		return
	}

	righeConCodice := map[int]bool{}
	prima := 0
	for _, c := range celle {
		if conLetture[c.u.ID] {
			righeConCodice[c.riga] = true
			if prima == 0 || c.riga < prima {
				prima = c.riga
			}
		}
	}
	if prima == 0 {
		return
	}
	// L'intestazione: la prima riga non vuota sopra la prima riga con codice. Una riga è non vuota se ha almeno
	// una cella con testo (le celle vuote non sono unità).
	intestazione := 0
	for _, c := range celle {
		if c.riga < prima && c.riga > intestazione {
			intestazione = c.riga
		}
	}
	if intestazione == 0 {
		return
	}
	type colonnaQuantita struct {
		colonna int
		cella   string
	}
	var colonne []colonnaQuantita
	for _, c := range celle {
		if c.riga != intestazione {
			continue
		}
		testo := strings.TrimSpace(c.u.Testo)
		for _, r := range regole {
			if contiene(r.intestazioni, testo) {
				colonne = append(colonne, colonnaQuantita{colonna: c.colonna, cella: c.u.ID})
				break
			}
		}
	}
	if len(colonne) == 0 {
		return
	}
	for _, c := range celle {
		if !righeConCodice[c.riga] {
			continue
		}
		for _, col := range colonne {
			if c.colonna != col.colonna {
				continue
			}
			it.attributoQuantita(c.u, col.cella, len(colonne) > 1)
		}
	}
}

// attributoQuantita: l'attributo «quantita» della riga, dalla cella della colonna dichiarata.
func (it *interprete) attributoQuantita(u evidenze.UnitaEvidenza, intestazione string, ambigua bool) {
	a := AttributoLetto{
		ID:       "a:" + u.ID + ":" + AttributoQuantita,
		Tipo:     AttributoQuantita,
		Grezzo:   u.Testo,
		UnitaID:  u.ID,
		EntitaID: u.EntitaID,
		Evidenze: []string{intestazione},
	}
	switch {
	case ambigua:
		a.Stato = StatoAmbiguo
	case interoDecimale(u.Testo):
		n, _ := strconv.Atoi(u.Testo)
		a.Stato, a.Normalizzato = StatoAttribuito, strconv.Itoa(n)
	default:
		a.Stato = StatoNonInterpretabile
		it.diag = append(it.diag, evidenze.Diagnostica{
			Codice:    CodiceQuantitaNonInterpretabile,
			Gravita:   evidenze.GravitaNota,
			Natura:    evidenze.NaturaDati,
			Percorso:  "attributi[" + a.ID + "]",
			Messaggio: "la cella della colonna quantità non è un intero di cifre decimali (da 1 a 6): il grezzo resta, nessun valore",
			Rif:       []string{a.ID, u.ID},
		})
	}
	it.attr = append(it.attr, a)
}

// interoDecimale: da 1 a 6 cifre decimali ASCII, e niente altro.
func interoDecimale(s string) bool {
	if len(s) < 1 || len(s) > 6 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
