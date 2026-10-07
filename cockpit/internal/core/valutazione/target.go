package valutazione

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
)

// StatoIdentitaTarget: l'asse 1 di un prodotto target (R79; contratto §1.0, riga 1).
//   - confermata: il gesto 2 dell'operatore (R70 A): un identificativo con confermato_da, o un finito attivo di
//     origine «manuale»;
//   - da_confermare: un prodotto dello scenario, sempre con l'autorità «scenario» (R75 A; T-B0-23).
type StatoIdentitaTarget string

const (
	IdentitaDaConfermare StatoIdentitaTarget = "da_confermare"
	IdentitaConfermata   StatoIdentitaTarget = "confermata"
)

// ProdottoValutato: un prodotto target della RFQ con i suoi assi (contratto §2.3; fase 0 di B6, F.2: completo, nell'ordine
// dei campi del contratto). In B1 ci sono il riferimento, l'autorità, il componente, il codice richiesto, la base letta,
// l'identità e la fonte; B5 aggiunge lo stato della struttura, la verifica della BOM (gli assi 3 e 4) e la completezza
// documentale (l'asse 6); B6 aggiunge i nodi della BOM, lo smistamento (l'asse 5), il composto (T-E1-01: il solo booleano
// è Verificato, JSON «prodotto_verificato»), lo stato del prodotto (l'asse 7), i motivi e l'impronta. Finché le fasi V2 e
// V3 di B6 non li calcolano, valgono i valori prudenti di valoriPrudenti (F.3): mai «verificato» per difetto.
//   - Rif: «componente:<uuid>» per un identificativo con il suo componente e per un finito manuale;
//     «identificativo:<codice>» per un identificativo senza componente (emendamento E1 §4.2, LD-21: il legame è per
//     codice, come nel DB); «scenario:<caso>:<n>» per un prodotto dello scenario.
//   - CodiceRichiesto: il codice com'è nel DB (identificativo o componente) o come l'ha scritto la mail (scenario).
//   - Base: la base letta con la grammatica del cliente; vuota se il codice non si legge (la base non si inventa).
//   - Struttura (B5): lo stato della struttura del prodotto fra le strutture di ancoraggio (StatoStrutturaDelTarget:
//     nessuna, struttura_candidata, bom_di_lavoro_proposta; R59 A, R76 A). Una BOM di lavoro resta una proposta.
//   - BOM (B5): nomenclatura e gerarchia, gli assi 3 e 4 (R80; VerificaDellaBOM, letta in A1c dall'adattatore di
//     «Conferma l'albero»).
//   - Documenti (B5): la completezza documentale, l'asse 6 (Completezza: R72 D, R102 A; contratto §1.6).
//   - Nodi (B6, V3): i nodi della BOM di lavoro come li vedrà la UI (R91, T-B0-39; T-B6-11: solo la BOM di lavoro).
//   - Smistamento (B6, V2): l'asse 5 (R81; contratto §1.5). Verificato, Stato, Motivi (B6, V3): il composto, l'asse 7 e
//     i motivi (R79, T-E1-01, T-E1-14). Impronta (B6, V3): l'impronta dei dati decisi del prodotto (R90, T-B0-29).
type ProdottoValutato struct {
	Rif             string                    `json:"rif"`
	Autorita        ancoraggio.Autorita       `json:"autorita"`
	ComponenteID    *uuid.UUID                `json:"componente_id,omitempty"`
	CodiceRichiesto string                    `json:"codice_richiesto"`
	Base            motorea.BaseLetta         `json:"base"`
	Identita        StatoIdentitaTarget       `json:"identita"`
	Fonte           FonteProdotto             `json:"fonte"`
	Struttura       ancoraggio.StatoStruttura `json:"struttura"`
	BOM             VerificaBOM               `json:"bom"`
	Nodi            []NodoBOM                 `json:"nodi,omitempty"`
	Smistamento     VerificaSmistamento       `json:"smistamento"`
	Documenti       CompletezzaDocumentale    `json:"documenti"`
	Verificato      bool                      `json:"prodotto_verificato"`
	Stato           StatoProdotto             `json:"stato"`
	Motivi          []MotivoProdotto          `json:"motivi,omitempty"`
	Impronta        string                    `json:"impronta"`
}

// TargetConfermato: il predicato del target confermato della RFQ (R70 A, R78; T-B0-21): l'identità confermata dal
// gesto 2, con l'autorità «confermata». Un prodotto dello scenario non lo è mai (R75 A), e un candidato della mail
// non è nemmeno un target (R60 A). È la prima condizione di ProdottoVerificato (R79), che B6 calcola.
func TargetConfermato(p ProdottoValutato) bool {
	return p.Identita == IdentitaConfermata && p.Autorita == ancoraggio.AutoritaConfermata
}

// I valori del DB che i target guardano (0001: tipo_componente, origine_componente).
const (
	tipoFinito     = "finito"
	origineManuale = "manuale"
)

// I prefissi dei riferimenti dei prodotti target.
const (
	rifComponente     = "componente:"
	rifIdentificativo = "identificativo:"
	rifScenario       = "scenario:"
)

// target: un prodotto target con ciò che serve alla fonte: la lettura del codice (nil = non letto) e il componente.
type target struct {
	pv         ProdottoValutato
	lettura    *motorea.LetturaForma
	componente *fotorfq.Componente
}

// targetDelThread: i prodotti target della RFQ (R60 A: solo i confermati e lo scenario).
//   - R70 A: ogni identificativo con confermato_da, con il suo componente (stesso codice, senza maiuscole, come in
//     ProdottiDellaRfq) se c'è; un identificativo il cui componente è archiviato resta fuori (togliere il componente
//     è stata una decisione); poi ogni finito attivo di origine «manuale» che non è già il componente di un
//     identificativo. componente.confermato_da non conta mai: è sempre pieno. Due identificativi con la stessa chiave
//     del codice (maiuscole, spazi ai bordi) fanno un target solo, come in ProdottiDellaRfq.
//   - R75 A: con un caso di autorità «scenario», i prodotti letti dalla mail nei segmenti dello scenario (uso
//     pertinente, origine «scenario»), uno per (namespace, base, qualificatori); identità da_confermare. Un prodotto
//     dello scenario con lo stesso namespace e la stessa base di un target confermato non si ripete: vince il
//     confermato (6.4.6, passo 6), e lo scenario non lo trasforma in niente.
//   - T-E1-24: un identificativo confermato senza componente, con un finito attivo che non è target nello stesso
//     thread, dà target.possibile_rinomina; il target non si corregge.
//
// I candidati della mail che lo scenario non dichiara restano candidati, da confermare: mai target (R60 A).
// interpretati sono le interpretazioni dei messaggi (le stesse di ProponiProdotti): danno la lettura principale di un
// prodotto dello scenario per intero (B5).
func targetDelThread(t fotorfq.Thread, m *motorea.Motore, caso *IngressoCaso, candidati []ancoraggio.CandidatoProdotto,
	interpretati []ancoraggio.MessaggioInterpretato) ([]target, []evidenze.Diagnostica) {
	componenti := append([]fotorfq.Componente(nil), t.Componenti...)
	sort.SliceStable(componenti, func(i, j int) bool { return componenti[i].ID.String() < componenti[j].ID.String() })
	perCodice := map[string]*fotorfq.Componente{}
	for i := range componenti {
		perCodice[chiaveCodice(componenti[i].Codice)] = &componenti[i]
	}
	// In ordine di (chiave del codice, codice): fra due identificativi che differiscono solo per le maiuscole o per gli
	// spazi ai bordi vince sempre lo stesso, il primo in quest'ordine.
	identificativi := append([]fotorfq.Identificativo(nil), t.Identificativi...)
	sort.SliceStable(identificativi, func(i, j int) bool {
		a, b := identificativi[i], identificativi[j]
		if ka, kb := chiaveCodice(a.Codice), chiaveCodice(b.Codice); ka != kb {
			return ka < kb
		}
		return a.Codice < b.Codice
	})

	var confermati []target
	var diag []evidenze.Diagnostica
	componentiTarget := map[uuid.UUID]bool{}
	codiciSenzaComponente := map[string]bool{} // la chiave del codice, come in ProdottiDellaRfq
	var senzaComponente []target
	for _, id := range identificativi {
		if id.ConfermatoDa == nil || strings.TrimSpace(id.Codice) == "" {
			continue
		}
		tg := target{pv: ProdottoValutato{Autorita: ancoraggio.AutoritaConfermata, CodiceRichiesto: id.Codice, Identita: IdentitaConfermata}}
		if c := perCodice[chiaveCodice(id.Codice)]; c != nil {
			if c.ArchiviatoIl != nil {
				continue
			}
			cid := c.ID
			tg.pv.Rif, tg.pv.ComponenteID, tg.componente = rifComponente+cid.String(), &cid, c
			if componentiTarget[cid] {
				continue
			}
			componentiTarget[cid] = true
		} else {
			if codiciSenzaComponente[chiaveCodice(id.Codice)] {
				continue
			}
			codiciSenzaComponente[chiaveCodice(id.Codice)] = true
			tg.pv.Rif = rifIdentificativo + id.Codice
			senzaComponente = append(senzaComponente, tg)
		}
		confermati = append(confermati, tg)
	}
	for i := range componenti {
		c := &componenti[i]
		if c.Tipo != tipoFinito || c.ArchiviatoIl != nil || c.Origine != origineManuale || componentiTarget[c.ID] {
			continue
		}
		cid := c.ID
		componentiTarget[cid] = true
		confermati = append(confermati, target{pv: ProdottoValutato{Rif: rifComponente + cid.String(), Autorita: ancoraggio.AutoritaConfermata,
			ComponenteID: &cid, CodiceRichiesto: c.Codice, Identita: IdentitaConfermata}, componente: c})
	}
	for i := range confermati {
		if m == nil {
			continue
		}
		if l, ok := leggiCodiceRegistrato(m, confermati[i].pv.CodiceRichiesto); ok {
			confermati[i].lettura = &l
			confermati[i].pv.Base = copiaBase(l.Base)
		} else {
			diag = append(diag, evidenze.Diagnostica{
				Codice:    CodiceTargetNonLeggibile,
				Gravita:   evidenze.GravitaAvviso,
				Natura:    evidenze.NaturaDati,
				Percorso:  "prodotti[" + confermati[i].pv.Rif + "]",
				Messaggio: "il codice del target non si legge con la grammatica del cliente: la base resta vuota e la compatibilità con le radici degli STEP non si calcola",
				Rif:       []string{confermati[i].pv.Rif},
			})
		}
	}
	sort.SliceStable(confermati, func(i, j int) bool {
		a, b := confermati[i].pv, confermati[j].pv
		if ka, kb := chiaveCodice(a.CodiceRichiesto), chiaveCodice(b.CodiceRichiesto); ka != kb {
			return ka < kb
		}
		return a.Rif < b.Rif
	})

	// T-E1-24: i finiti attivi che non sono target, accanto a un identificativo confermato senza componente.
	var finitiNonTarget []string
	for _, c := range componenti {
		if c.Tipo == tipoFinito && c.ArchiviatoIl == nil && !componentiTarget[c.ID] {
			finitiNonTarget = append(finitiNonTarget, rifComponente+c.ID.String())
		}
	}
	if len(finitiNonTarget) > 0 {
		for _, tg := range senzaComponente {
			diag = append(diag, evidenze.Diagnostica{
				Codice:   CodiceTargetPossibileRinomina,
				Gravita:  evidenze.GravitaAvviso,
				Natura:   evidenze.NaturaDati,
				Percorso: "prodotti[" + tg.pv.Rif + "]",
				Messaggio: fmt.Sprintf("un identificativo confermato senza componente e %d finiti che non sono target: forse una rinomina; il target non si corregge (T-E1-24)",
					len(finitiNonTarget)),
				Rif: append([]string{tg.pv.Rif}, finitiNonTarget...),
			})
		}
	}

	out := confermati
	if caso != nil && caso.Autorita == ancoraggio.AutoritaScenario {
		out = append(out, targetDelloScenario(caso, candidati, confermati, interpretati)...)
	}
	return out, diag
}

// targetDelloScenario: i prodotti dello scenario (R75 A), in ordine di candidato, uno per (namespace, base,
// qualificatori attribuiti). La lettura del prodotto è quella principale del candidato, per intero (con il marcatore,
// la revisione e gli affissi: B5, così la fonte e le strutture usano la stessa regola del marcatore, T-B4-30); se non
// si trova fra le interpretazioni, solo namespace, base e codice.
func targetDelloScenario(caso *IngressoCaso, candidati []ancoraggio.CandidatoProdotto, confermati []target, interpretati []ancoraggio.MessaggioInterpretato) []target {
	var out []target
	visti := map[string]bool{}
	for _, c := range candidati {
		if !dalloScenario(c) {
			continue
		}
		k := c.Namespace + "\x00" + c.Base.Normalizzata + "\x00" + chiaveQualificatori(c.Qualificatori)
		if visti[k] || giaConfermato(c, confermati) {
			continue
		}
		visti[k] = true
		lettura := letturaPrincipale(c, interpretati)
		if lettura == nil {
			lettura = &motorea.LetturaForma{Namespace: c.Namespace, Base: copiaBase(c.Base), CodiceRichiesto: c.CodiceRichiesto}
		}
		out = append(out, target{
			pv: ProdottoValutato{
				Rif:             fmt.Sprintf("%s%s:%d", rifScenario, caso.ID, len(out)+1),
				Autorita:        ancoraggio.AutoritaScenario,
				CodiceRichiesto: c.CodiceRichiesto,
				Base:            copiaBase(c.Base),
				Identita:        IdentitaDaConfermare,
			},
			lettura: lettura,
		})
	}
	return out
}

// letturaPrincipale: la forma della lettura principale di un candidato (il messaggio e la lettura che il candidato
// porta), cercata nelle interpretazioni dei messaggi; nil se non c'è, o se non ha il namespace e la base del candidato.
func letturaPrincipale(c ancoraggio.CandidatoProdotto, interpretati []ancoraggio.MessaggioInterpretato) *motorea.LetturaForma {
	for _, mi := range interpretati {
		if mi.MessaggioID != c.MessaggioID {
			continue
		}
		for _, l := range mi.Interpretazione.Letture {
			if l.ID == c.Lettura && l.Forma.Namespace == c.Namespace && l.Forma.Base.Normalizzata == c.Base.Normalizzata {
				f := l.Forma
				f.Base = copiaBase(f.Base)
				return &f
			}
		}
	}
	return nil
}

// dalloScenario: il candidato ha un'evidenza di un segmento che lo scenario ha scelto pertinente (uso pertinente,
// origine «scenario»). Le altre evidenze non tolgono niente e non aggiungono niente.
func dalloScenario(c ancoraggio.CandidatoProdotto) bool {
	for _, e := range c.Evidenze {
		if e.Uso == motorea.UsoPertinente && e.OrigineUso == string(ancoraggio.AutoritaScenario) {
			return true
		}
	}
	return false
}

// giaConfermato: un target confermato letto con lo stesso namespace e la stessa base del candidato.
func giaConfermato(c ancoraggio.CandidatoProdotto, confermati []target) bool {
	for _, tg := range confermati {
		if tg.lettura != nil && tg.lettura.Namespace == c.Namespace && motorea.ConfrontaBasi(tg.lettura.Base, c.Base) == motorea.CompatibilitaUguale {
			return true
		}
	}
	return false
}

// selettoriDelCodiceRegistrato: le forme con cui si legge un codice scritto nel DB, in quest'ordine fisso (6.4.6,
// LetturaRegistrata; R31 c).
var selettoriDelCodiceRegistrato = []string{"nome_file", "cartiglio.codice", "nodo_step.id", "corpo"}

// leggiCodiceRegistrato legge un codice registrato con la grammatica del cliente (6.4.6, R31 c): vale se le letture
// complete che coprono tutto il codice, con le forme dei selettori nell'ordine fisso, danno tutte lo stesso
// namespace e la stessa base. Restituisce la prima. Nessuna lettura, o due basi diverse: non si legge, e la base non
// si inventa. Nemmeno un confronto per stringa: sarebbe un legame senza provenance (5.0); il 6.4.6, passo 6, lo
// ammetteva, B1 se ne discosta (lettura dell'orchestratore T-B1-07). Il perché di una lettura mancata lo dice
// letturaDelCodiceRegistrato, che B6 espone con LeggiCodiceRegistrato (vecchio.go).
func leggiCodiceRegistrato(m *motorea.Motore, codice string) (motorea.LetturaForma, bool) {
	l, motivo := letturaDelCodiceRegistrato(m, codice)
	return l, motivo == ""
}

// letturaDelCodiceRegistrato: la regola di leggiCodiceRegistrato, con il motivo di una lettura mancata ("" = letto; B6,
// LetturaRegistrata.Motivo): codice_vuoto, senza_grammatica, nessuna_lettura, basi_diverse.
func letturaDelCodiceRegistrato(m *motorea.Motore, codice string) (motorea.LetturaForma, string) {
	testo := strings.TrimSpace(codice)
	switch {
	case testo == "":
		return motorea.LetturaForma{}, MotivoLetturaCodiceVuoto
	case m == nil:
		return motorea.LetturaForma{}, MotivoLetturaSenzaGrammatica
	}
	var trovate []motorea.LetturaForma
	for _, s := range selettoriDelCodiceRegistrato {
		sel, err := evidenze.LeggiSelettore(s)
		if err != nil {
			continue
		}
		letture, _ := m.Riconosci(sel, testo)
		for _, l := range letture {
			if l.Intervallo.Inizio == 0 && l.Intervallo.Fine == len(testo) && l.Base.Completa {
				trovate = append(trovate, l)
			}
		}
	}
	if len(trovate) == 0 {
		return motorea.LetturaForma{}, MotivoLetturaNessunaLettura
	}
	for _, l := range trovate[1:] {
		if l.Namespace != trovate[0].Namespace || l.Base.Normalizzata != trovate[0].Base.Normalizzata {
			return motorea.LetturaForma{}, MotivoLetturaBasiDiverse
		}
	}
	return trovate[0], ""
}

// chiaveCodice: il confronto per codice del legame identificativo-componente, come lo fa il DB (upper, 0018) e
// ProdottiDellaRfq: senza maiuscole e senza gli spazi ai bordi.
func chiaveCodice(c string) string { return strings.ToUpper(strings.TrimSpace(c)) }

// chiaveQualificatori: i qualificatori attribuiti di una lettura (regola, fase, destinazione), in ordine.
func chiaveQualificatori(affissi []motorea.AffissoLetto) string {
	var parti []string
	for _, a := range affissi {
		if a.Attribuito && a.Valore != nil {
			parti = append(parti, a.Regola+"\x01"+a.Valore.Fase+"\x01"+a.Valore.Destinazione)
		}
	}
	sort.Strings(parti)
	return strings.Join(parti, "\x02")
}

// copiaBase: la base con gli elenchi copiati, così l'esito non condivide memoria con le letture.
func copiaBase(b motorea.BaseLetta) motorea.BaseLetta {
	b.Segmenti = append([]motorea.SegmentoLetto(nil), b.Segmenti...)
	b.Mancanti = append([]string(nil), b.Mancanti...)
	if len(b.Segmenti) == 0 {
		b.Segmenti = nil
	}
	if len(b.Mancanti) == 0 {
		b.Mancanti = nil
	}
	return b
}
