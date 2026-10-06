// L1 — la catena del codice dei nodi, l'identità, i candidati di revisione e le decisioni accanto ai nodi e agli archi
// (piano A, 6.0.6, 6.4.5; contratto §1.4, §2.2, §2.5, §2.6, §7 famiglia B4; commit P6b, fase 1): dai fatti STEP del
// worker all'adattatore, a Interpreta, a StrutturaDa e a ProponiStrutture, con i codici composti dal compositore come
// li passerà valutazione (T-08). Il grezzo invariato (I-6), la lettura, l'identità completa e parziale (R86, R87,
// T-B0-37), il codice proposto, manuale (T-B0-33) e confermato (R31 c); i candidati con la loro entità, mai propagati
// (T-E1-06, T-B0-27, T-B0-35); la provenienza della revisione registrata (T-B0-34, T-E1-22) e la decisione tracciata
// (E1R, T-E1R-08); la decisione del nodo per UUID (T-E1-04, T-E1-05; la parte B4 di PO-21); l'arco con la decisione
// accanto (R95 A); le identità parziali sulla BOM di lavoro (R85); il determinismo.
package ancoraggio_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/ancoraggio"
	"promatec/cockpit/internal/core/estrazione"
	"promatec/cockpit/internal/core/estrazione/evidenze"
	"promatec/cockpit/internal/core/fotorfq"
	"promatec/cockpit/internal/core/inbox/classificazione/motorea"
	"promatec/cockpit/internal/core/registro/regole/grammatica"
)

// I clienti di questi test sono inventati. Non è pigrizia: le famiglie di codice dei clienti veri sono un dato
// dell'azienda e questo repository è pubblico, e una prova che dipendesse da esse diventerebbe rossa il giorno in cui
// un cliente cambia convenzione. Il cliente è ACME (acme.example); i codici sono di fantasia (712xxxx con il marcatore
// A e la revisione di una cifra, «7120100A_2» nel nome del file e nei nodi, «7120100A2» nel cartiglio e nella mail),
// gli UUID sono 00000000-0000-4000-8000-0000000000nn. La famiglia acme-catena attiva un meccanismo (un nodo che legge
// base e marcatore senza revisione, mentre la forma del cartiglio la chiede: R87), mai il significato di un cliente.
// Le prove citano i requisiti (R87, T-E1-06, T-E1-22, PO-21, PO-37), mai i casi degli attesi.

// ---- la grammatica ACME della catena ----

var (
	pBaseC      = grammatica.Parte{Tipo: grammatica.TipoParteBase, Min: 1, Max: 1}
	pMarcatoreC = grammatica.Parte{Tipo: grammatica.TipoParteMarcatore, Letterali: []string{"A", "B"}, Min: 1, Max: 1}
	pSottoC     = grammatica.Parte{Tipo: grammatica.TipoParteSeparatore, Letterali: []string{"_"}, Min: 1, Max: 1}
)

func pRevC(min int) grammatica.Parte {
	return grammatica.Parte{Tipo: grammatica.TipoParteRevisione, Rif: "rev-c", Min: min, Max: 1}
}

func formaC(id string, selettori []string, parti ...grammatica.Parte) grammatica.FormaCodice {
	return grammatica.FormaCodice{ID: id, Selettori: selettori, Stato: grammatica.StatoAttiva, Completa: true, Parti: parti}
}

// famCatena: base 712 più quattro cifre, marcatore A o B (fa parte dell'identità: R86), revisione di una cifra.
//   - cartiglio [base][A][rev] su cartiglio.codice: la forma documentale, con la revisione obbligatoria (R63 B);
//   - mail [base][A][rev facoltativa] su oggetto e corpo;
//   - nome [base][A]_[rev] sul nome del file;
//   - step [base][A] sugli id delle radici e dei nodi: base e marcatore, nessuna revisione (il caso di R87);
//   - step-rev [base][A]_[rev] sugli id dei nodi: un nodo con l'identità completa.
func famCatena() grammatica.FamigliaCodice {
	step := formaC("step", []string{"radice_step.id", "nodo_step.id"}, pBaseC, pMarcatoreC)
	parola := grammatica.ConfineParolaASCII
	step.ConfineDopo = &parola
	return grammatica.FamigliaCodice{
		ID: "acme-catena", Namespace: "acme-catena",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloProdotto, grammatica.RuoloComponente},
		Base:  base(grammatica.SegmentoBase{Nome: "codice", Pattern: "712[0-9]{4}", Identitario: true}),
		Forme: []grammatica.FormaCodice{
			formaC("cartiglio", []string{"cartiglio.codice"}, pBaseC, pMarcatoreC, pRevC(1)),
			formaC("mail", []string{"oggetto", "corpo"}, pBaseC, pMarcatoreC, pRevC(0)),
			formaC("nome", []string{"nome_file"}, pBaseC, pMarcatoreC, pSottoC, pRevC(1)),
			step,
			formaC("step-rev", []string{"nodo_step.id"}, pBaseC, pMarcatoreC, pSottoC, pRevC(1)),
		},
		Revisioni: []grammatica.RegolaRevisione{{
			ID: "rev-c", Selettori: []string{"cartiglio.codice", "nome_file", "corpo", "oggetto", "nodo_step.id"},
			Stato: grammatica.StatoAttiva, Sorgente: grammatica.SorgenteInline,
			Segmenti: []grammatica.SegmentoRevisione{{Nome: "valore", Pattern: "[0-9]", Significato: grammatica.SignificatoNessuno}},
		}},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-cartiglio", "cartiglio.codice", "7120100A2", false, grammatica.LetturaAttesa{Forma: "cartiglio", Base: "7120100", Marcatore: "A", Revisione: "2"}),
			esempio("e-mail", "corpo", "7120100A2", false, grammatica.LetturaAttesa{Forma: "mail", Base: "7120100", Marcatore: "A", Revisione: "2"}),
			esempio("e-mail-senza", "corpo", "7120100A", false, grammatica.LetturaAttesa{Forma: "mail", Base: "7120100", Marcatore: "A"}),
			esempio("e-nome", "nome_file", "7120100A_2.stp", false, grammatica.LetturaAttesa{Forma: "nome", Base: "7120100", Marcatore: "A", Revisione: "2"}),
			esempio("e-step", "nodo_step.id", "7120100A", false, grammatica.LetturaAttesa{Forma: "step", Base: "7120100", Marcatore: "A"}),
			esempio("e-step-radice", "radice_step.id", "7120100A", false, grammatica.LetturaAttesa{Forma: "step", Base: "7120100", Marcatore: "A"}),
			esempio("e-step-rev", "nodo_step.id", "7120100A_3", false, grammatica.LetturaAttesa{Forma: "step-rev", Base: "7120100", Marcatore: "A", Revisione: "3"}),
		},
	}
}

func motoreCatena(t *testing.T) *motorea.Motore {
	t.Helper()
	return motore(t, famCatena())
}

// famCatenaAlt: un'altra famiglia, in un altro spazio di codici, che legge «7123xxxA» con la A dentro la base: sullo
// stesso id due letture con identità diverse (T-B4-01).
func famCatenaAlt() grammatica.FamigliaCodice {
	return grammatica.FamigliaCodice{
		ID: "acme-catena-alt", Namespace: "acme-catena-alt",
		Ruoli: []grammatica.Ruolo{grammatica.RuoloComponente},
		Base:  base(grammatica.SegmentoBase{Nome: "codice", Pattern: "7123[0-9]{3}A", Identitario: true}),
		Forme: []grammatica.FormaCodice{formaC("alt", []string{"radice_step.id", "nodo_step.id"}, pBaseC)},
		Esempi: []grammatica.EsempioCodice{
			esempio("e-alt", "nodo_step.id", "7123000A", true, grammatica.LetturaAttesa{Forma: "alt", Base: "7123000A"}),
		},
	}
}

// ---- gli STEP con le formazioni ----

// nodoR: un PRODUCT dei fatti con la formazione grezza (rev_grezza, PRODUCT_DEFINITION_FORMATION.id).
type nodoR struct{ chiave, id, nome, rev string }

// fattiR: i fatti di uno STEP completo alla terna corrente, come fattiS ma con la formazione dei nodi.
func fattiR(t *testing.T, sha string, nodi []nodoR, archi []arcoS) *fotorfq.Fatti {
	t.Helper()
	n := []map[string]any{}
	for _, x := range nodi {
		n = append(n, map[string]any{"chiave": x.chiave, "id_grezzo": x.id, "nome_grezzo": x.nome, "descrizione_grezza": "",
			"rev_grezza": x.rev, "evidenza": map[string]any{}})
	}
	r := []map[string]any{}
	for _, a := range archi {
		ev := map[string]any{}
		if len(a.righe) > 0 {
			ev["righe"] = a.righe
		}
		r = append(r, map[string]any{"padre": a.padre, "figlio": a.figlio, "qta": a.qta, "evidenza": ev})
	}
	raw, err := json.Marshal(map[string]any{"struttura": map[string]any{
		"versione": 3, "schema": "AP214", "radici": []string{nodi[0].chiave}, "avvisi": []string{}, "nodi": n, "relazioni": r,
		"limiti": map[string]any{"troncato": false},
		"scarti": map[string]any{"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return fattiGrezzi(t, sha, raw, testoS(""))
}

// stepR: lo STEP con quel nome di file, la radice è il primo nodo.
func stepR(t *testing.T, m *motorea.Motore, id uuid.UUID, nome, sha string, nodi []nodoR, archi []arcoS) ancoraggio.StrutturaFile {
	t.Helper()
	return strutturaS(t, fileS(t, m, id, nome, "stp", sha, fattiR(t, sha, nodi, archi)))
}

// ---- gli ingressi come li preparerà valutazione ----

// codiciProposti: il compositore su ogni lettura d'identità dei nodi, con la chiave del contratto (T-08).
func codiciProposti(m *motorea.Motore, strutture ...ancoraggio.StrutturaFile) map[string]motorea.CodiceComposto {
	out := map[string]motorea.CodiceComposto{}
	for _, s := range strutture {
		for _, n := range s.Nodi {
			for _, l := range n.Letture {
				out[ancoraggio.ChiaveCodiceProposto(s.BundleID, l.ID)] = m.ComponiCodiceDocumentale(l.Forma)
			}
		}
	}
	return out
}

// letturaC: la lettura completa di tutto il codice su quel selettore, che deve esserci.
func letturaC(t *testing.T, m *motorea.Motore, selettore, codice string) motorea.LetturaForma {
	t.Helper()
	sel, err := evidenze.LeggiSelettore(selettore)
	if err != nil {
		t.Fatal(err)
	}
	letture, _ := m.Riconosci(sel, codice)
	for _, l := range letture {
		if l.Intervallo.Inizio == 0 && l.Intervallo.Fine == len(codice) && l.Base.Completa {
			return l
		}
	}
	t.Fatalf("%q non si legge su %s", codice, selettore)
	return motorea.LetturaForma{}
}

// targetC: un target con il codice letto sul cartiglio (con la revisione, se c'è) o su un nodo.
func targetC(t *testing.T, m *motorea.Motore, rif string, autorita ancoraggio.Autorita, codice string, componente *uuid.UUID) ancoraggio.ProdottoRichiesto {
	t.Helper()
	sel := "nodo_step.id"
	if len(codice) == len("7120100A2") {
		sel = "cartiglio.codice"
	}
	l := letturaC(t, m, sel, codice)
	p := ancoraggio.ProdottoRichiesto{Rif: rif, Autorita: autorita, ClienteID: clienteACME, Namespace: l.Namespace, CodiceRichiesto: codice,
		Base: l.Base, ComponenteID: componente}
	if l.Marcatore != nil {
		p.Marcatore = l.Marcatore.Valore // come lo riempirà valutazione (T-B4-06 rivisto)
	}
	if l.Revisione != nil {
		r := *l.Revisione
		p.Revisione = &r
	}
	return p
}

// componenteC: un componente confermato con il codice letto (R31 c) e la revisione registrata.
func componenteC(t *testing.T, m *motorea.Motore, id uuid.UUID, codice string, rev *string) ancoraggio.ComponenteDeciso {
	t.Helper()
	l := letturaC(t, m, "nodo_step.id", codice)
	return ancoraggio.ComponenteDeciso{ComponenteID: id, Codice: codice, Autorita: ancoraggio.AutoritaConfermata, Origine: ancoraggio.OrigineConfermato,
		Rev: rev, Lettura: &l}
}

// codiciDelMessaggio: i codici letti dal motore A nel testo di un messaggio, con la posizione dell'occorrenza, come li
// preparerà valutazione dalle stesse interpretazioni di ProponiProdotti (T-E1-06).
func codiciDelMessaggio(t *testing.T, m *motorea.Motore, id uuid.UUID, testo string) []ancoraggio.CodiceDiMessaggio {
	t.Helper()
	msg := messaggioACME(id, "Richiesta di offerta ACME26-030", testo, "")
	msg.CorpoHTML = nil
	d, err := estrazione.DaMessaggio(msg)
	if err != nil {
		t.Fatalf("DaMessaggio: %v", err)
	}
	r, err := m.Interpreta(d, evidenze.UsoSconosciuto(d.BundleID))
	if err != nil {
		t.Fatalf("Interpreta: %v", err)
	}
	pos := map[string]evidenze.Localizzatore{}
	for _, u := range d.Unita {
		pos[u.ID] = u.Posizione
	}
	var out []ancoraggio.CodiceDiMessaggio
	for _, l := range r.Letture {
		p := pos[l.UnitaID]
		if l.Assoluto != nil {
			a := *l.Assoluto
			p = evidenze.Localizzatore{Tipo: "testo", Testo: &a}
		}
		out = append(out, ancoraggio.CodiceDiMessaggio{MessaggioID: id, Lettura: l, Posizione: p})
	}
	return out
}

func rigaAperta(id, allegato uuid.UUID, sha, chiave string) ancoraggio.RigaPropostaLegacy {
	return ancoraggio.RigaPropostaLegacy{ID: id, AllegatoID: allegato, Sha256: sha, Chiave: chiave, Autorita: ancoraggio.AutoritaProposta, Origine: ancoraggio.OrigineProposto}
}

func rigaDecisa(id, allegato uuid.UUID, sha, chiave, stato string, componente, da *uuid.UUID) ancoraggio.RigaDecisaLegacy {
	return ancoraggio.RigaDecisaLegacy{ID: id, AllegatoID: allegato, Sha256: sha, Chiave: chiave, Stato: stato, ComponenteID: componente, DecisoDa: da}
}

func uuidP(u uuid.UUID) *uuid.UUID { return &u }

// ---- la scena ----

var (
	idFileC  = uidS(0x601)
	shaFileC = shaS("6")
	idUtente = uidS(0x6f0)
	idC1     = uidS(0x611) // il prodotto
	idC2     = uidS(0x612)
	idC3     = uidS(0x613)
	idMailC  = uidS(0x621)
)

// nodiScenaC: il prodotto 7120100A (la radice, che legge base e marcatore senza revisione, con la formazione «1»),
// il figlio 7120101A (due volte), il figlio 7120102A_3 (identità completa, formazione «B»), una saldatura che nessuna
// famiglia legge.
func nodiScenaC() ([]nodoR, []arcoS) {
	return []nodoR{{"#1", "7120100A", "7120100A", "1"}, {"#2", "7120101A", "SUPPORTO", ""}, {"#3", "7120102A_3", "PIASTRA", "B"}, {"#4", "SALDATURA_1", "", ""}},
		[]arcoS{{"#1", "#2", 2, []string{"#21", "#22"}}, {"#1", "#3", 1, nil}, {"#1", "#4", 1, nil}}
}

func scenaC(t *testing.T, m *motorea.Motore, nomeFile string) ancoraggio.StrutturaFile {
	t.Helper()
	nodi, archi := nodiScenaC()
	return stepR(t, m, idFileC, nomeFile, shaFileC, nodi, archi)
}

// rC: il Rif di un nodo della scena.
func rC(chiave string) string { return ancoraggio.RifNodo(shaFileC, chiave) }

// ---- le chiamate, con gli invarianti della catena ----

// proponiC chiama ProponiStrutture (con gli invarianti delle strutture di proponiS) e controlla quelli della catena e
// delle decisioni:
//   - il grezzo è uno dei grezzi del nodo nel file, invariato (I-6);
//   - con la lettura c'è la forma, e nessun motivo della lettura; senza, il motivo e nessuna forma;
//   - una stringa proposta non ha motivo, una stringa assente sì (T-B0-37); Parziale vuol dire nessuna revisione e
//     nessuna stringa (R87);
//   - lo stato della revisione è uno dei tre; confermata solo con la decisione tracciata (E1R);
//   - i candidati e le fonti senza revisione hanno l'entità e una fonte del contratto; il nome del file solo su una
//     radice del file, il codice del target solo sulla radice della struttura (T-E1-06, T-B0-27); niente senza lettura;
//   - il codice manuale solo con una riga aperta che lo porta (T-B0-33); il confermato solo con la decisione, che
//     viene da una riga decisa confermata o duplicato del suo componente (E1 §4.2), oppure, sulla BOM di lavoro, sulla
//     radice scelta senza decisione, dal componente del target (T-B4-38);
//   - la decisione di un arco è una relazione confermata fra le decisioni dei suoi due nodi; solo sulla BOM di lavoro,
//     per un arco della radice senza decisione, il padre è il componente del target (T-B4-23).
func proponiC(t *testing.T, target []ancoraggio.ProdottoRichiesto, ctx ancoraggio.ContestoStrutturale) []ancoraggio.StrutturaProdotto {
	t.Helper()
	strutture, _ := proponiS(t, target, ctx)
	file := map[uuid.UUID]ancoraggio.StrutturaFile{}
	for _, s := range ctx.Strutture {
		file[s.AllegatoID] = s
	}
	perTarget := map[string]ancoraggio.ProdottoRichiesto{}
	for _, x := range target {
		perTarget[x.Rif] = x
	}
	fonti := map[string]bool{ancoraggio.FonteRevisioneNomeFileSTEP: true, ancoraggio.FonteRevisioneCartiglio: true,
		ancoraggio.FonteRevisioneCodiceTarget: true, ancoraggio.FonteRevisioneMessaggio: true}
	for _, s := range strutture {
		f := file[s.AllegatoID]
		radici := map[string]bool{}
		for _, r := range f.Radici {
			radici[r] = true
		}
		nodi := map[string]ancoraggio.NodoProposto{}
		for _, n := range s.Nodi {
			nodi[n.Rif] = n
			c := n.Codice
			var letto ancoraggio.NodoStruttura
			for _, x := range f.Nodi {
				if x.Rif == n.Rif {
					letto = x
				}
			}
			grezzo := c.Grezzo.Testo == "" && len(letto.Grezzi) == 0
			for _, g := range letto.Grezzi {
				grezzo = grezzo || reflect.DeepEqual(g, c.Grezzo)
			}
			if !grezzo {
				t.Errorf("nodo %s: il grezzo %+v non è uno dei grezzi del file %+v (I-6)", n.Rif, c.Grezzo, letto.Grezzi)
			}
			if (c.Lettura != "") != (c.Forma != nil) || (c.Lettura != "") == (c.MotivoLettura != "") {
				t.Errorf("nodo %s: lettura %q, forma %v, motivo %q", n.Rif, c.Lettura, c.Forma != nil, c.MotivoLettura)
			}
			if (c.Proposto == "") == (c.MotivoProposto == "") {
				t.Errorf("nodo %s: proposto %q con il motivo %q", n.Rif, c.Proposto, c.MotivoProposto)
			}
			if c.Identita.Parziale && (c.Identita.Revisione != nil || c.Proposto != "") {
				t.Errorf("nodo %s: identità parziale con la revisione o la stringa: %+v %q", n.Rif, c.Identita, c.Proposto)
			}
			tracciata := c.Confermato != nil && c.Confermato.RevProvenienza == ancoraggio.RevProvenienzaDecisioneTracciata
			switch c.Identita.StatoRevisione {
			case ancoraggio.StatoRevisioneAssente, ancoraggio.StatoRevisioneCandidata, ancoraggio.StatoRevisioneConfermata:
			default:
				t.Errorf("nodo %s: stato della revisione %q", n.Rif, c.Identita.StatoRevisione)
			}
			if (c.Identita.StatoRevisione == ancoraggio.StatoRevisioneConfermata) != tracciata {
				t.Errorf("nodo %s: stato %q, decisione tracciata %v (R97 B, E1R)", n.Rif, c.Identita.StatoRevisione, tracciata)
			}
			if c.Lettura == "" && (len(c.Identita.CandidatiRevisione) > 0 || len(c.Identita.FontiSenzaRevisione) > 0) {
				t.Errorf("nodo %s: indizi senza un'identità", n.Rif)
			}
			for _, k := range c.Identita.CandidatiRevisione {
				if !fonti[k.Fonte] || k.Entita == "" || k.Valore == "" {
					t.Errorf("nodo %s: candidato %+v", n.Rif, k)
				}
				if k.Fonte == ancoraggio.FonteRevisioneNomeFileSTEP && !radici[n.Rif] {
					t.Errorf("nodo %s: il nome del file è un candidato solo della radice del file (T-B0-27)", n.Rif)
				}
				if k.Fonte == ancoraggio.FonteRevisioneCodiceTarget && (n.Rif != s.Radice || k.Entita != s.Target) {
					t.Errorf("nodo %s: il codice del target è un candidato del prodotto, sulla radice (T-E1-06): %+v", n.Rif, k)
				}
			}
			for _, k := range c.Identita.FontiSenzaRevisione {
				if !fonti[k.Fonte] || k.Entita == "" || k.Motivo == "" {
					t.Errorf("nodo %s: fonte senza revisione %+v", n.Rif, k)
				}
			}
			if (c.Manuale != nil) != (n.RigaLegacy != nil && n.RigaLegacy.CodiceManuale != nil) || (c.Manuale != nil && c.Manuale.Origine != ancoraggio.OrigineManuale) {
				t.Errorf("nodo %s: codice manuale %+v con la riga %+v (T-B0-33)", n.Rif, c.Manuale, n.RigaLegacy)
			}
			// Il componente del confermato: quello della decisione; sulla radice scelta della BOM di lavoro, senza
			// decisione, quello del target, se è fra i confermati del contesto (T-B4-38).
			var atteso *uuid.UUID
			switch tg := perTarget[s.Target]; {
			case n.Decisione != nil:
				atteso = &n.Decisione.ComponenteID
			case s.Stato == ancoraggio.StatoBOMDiLavoroProposta && n.Rif == s.Radice && tg.ComponenteID != nil:
				for _, k := range ctx.Confermato {
					if k.ComponenteID == *tg.ComponenteID {
						atteso = tg.ComponenteID
					}
				}
			}
			if (c.Confermato != nil) != (atteso != nil) || (c.Confermato != nil && (c.Confermato.Origine != ancoraggio.OrigineConfermato ||
				*c.Confermato.ComponenteID != *atteso)) {
				t.Errorf("nodo %s: codice confermato %+v con la decisione %+v (T-B4-38)", n.Rif, c.Confermato, n.Decisione)
			}
			if n.Decisione != nil && (n.RigaDecisa == nil || n.RigaDecisa.ComponenteID == nil || *n.RigaDecisa.ComponenteID != n.Decisione.ComponenteID ||
				(n.RigaDecisa.Stato != ancoraggio.StatoRigaConfermata && n.RigaDecisa.Stato != ancoraggio.StatoRigaDuplicato)) {
				t.Errorf("nodo %s: la decisione %+v non viene dalla sua riga decisa %+v (E1 §4.2)", n.Rif, n.Decisione, n.RigaDecisa)
			}
			if n.DecisoDaPersona != (n.RigaDecisa != nil && n.RigaDecisa.DecisoDa != nil) {
				t.Errorf("nodo %s: deciso da persona %v con la riga %+v", n.Rif, n.DecisoDaPersona, n.RigaDecisa)
			}
		}
		for _, a := range s.Archi {
			if a.Decisione == nil {
				continue
			}
			p, f := nodi[a.Padre], nodi[a.Figlio]
			padre := ""
			switch tg := perTarget[s.Target]; {
			case p.Decisione != nil:
				padre = ancoraggio.RifComponente(p.Decisione.ComponenteID)
			case s.Stato == ancoraggio.StatoBOMDiLavoroProposta && a.Padre == s.Radice && tg.ComponenteID != nil:
				padre = ancoraggio.RifComponente(*tg.ComponenteID)
			}
			if padre == "" || f.Decisione == nil || a.Decisione.Origine != ancoraggio.OrigineArcoConfermato ||
				a.Decisione.Padre != padre || a.Decisione.Figlio != ancoraggio.RifComponente(f.Decisione.ComponenteID) {
				t.Errorf("arco %s→%s: decisione %+v fuori dalle decisioni dei nodi (T-B4-23)", a.Padre, a.Figlio, a.Decisione)
			}
		}
	}
	return strutture
}

// strutturaDi: l'unica struttura di quel target con quella radice.
func strutturaDi(t *testing.T, s []ancoraggio.StrutturaProdotto, target, radice string) ancoraggio.StrutturaProdotto {
	t.Helper()
	var trovate []ancoraggio.StrutturaProdotto
	for _, x := range s {
		if x.Target == target && x.Radice == radice {
			trovate = append(trovate, x)
		}
	}
	if len(trovate) != 1 {
		t.Fatalf("strutture di %s sotto %s: %d, attesa una", target, radice, len(trovate))
	}
	return trovate[0]
}

func nodoC(t *testing.T, s ancoraggio.StrutturaProdotto, rif string) ancoraggio.NodoProposto {
	t.Helper()
	n, ok := nodoDi(s, rif)
	if !ok {
		t.Fatalf("il nodo %s non c'è nella struttura di %s", rif, s.Target)
	}
	return n
}

func testoP(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

// ---- la catena del codice ----

// TestCatenaDelCodice (6.0.6; contratto §1.4; R86, R87, T-B0-37, T-B0-33, R31 c; PO-06 nella parte dei nodi): la
// catena di ogni nodo della scena.
//   - La radice legge base e marcatore senza revisione, e la forma del cartiglio la chiede: identità parziale, nessuna
//     revisione inventata, nessuna stringa, motivo revisione_non_determinata; il grezzo resta quello del file (I-6); i
//     candidati (il nome del file, il codice del target, il messaggio) stanno accanto e non diventano la revisione.
//   - Due letture dello stesso id con identità diverse: nessuna si sceglie, con il motivo.
//   - Il figlio 7120102A_3 ha l'identità completa e il codice proposto «7120102A3»; il figlio 7120101A, parziale, ha il
//     codice manuale dell'operatore dalla sua riga aperta; la saldatura non ha lettura, con il motivo.
//   - Il codice confermato viene dal componente legato per UUID, letto con la grammatica, con la revisione registrata.
func TestCatenaDelCodice(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "7120100A_2.stp")
	rev7, revB := "7", "B"
	manuale, revManuale := "7120101A1", "1"
	lm := letturaC(t, m, "nodo_step.id", "7120101A")
	aperta := rigaAperta(uidS(0x631), idFileC, shaFileC, "#2")
	aperta.CodiceManuale, aperta.RevManuale, aperta.LetturaManuale = &manuale, &revManuale, &lm
	ctx := ancoraggio.ContestoStrutturale{
		Strutture:      []ancoraggio.StrutturaFile{s},
		Confermato:     []ancoraggio.ComponenteDeciso{componenteC(t, m, idC1, "7120100A", nil), componenteC(t, m, idC2, "7120101A", &rev7), componenteC(t, m, idC3, "7120102A", &revB)},
		Proposto:       []ancoraggio.RigaPropostaLegacy{aperta},
		Decise:         []ancoraggio.RigaDecisaLegacy{rigaDecisa(uidS(0x632), idFileC, shaFileC, "#3", ancoraggio.StatoRigaDuplicato, uuidP(idC3), nil)},
		CodiciProposti: codiciProposti(m, s),
		CodiciMessaggi: codiciDelMessaggio(t, m, idMailC, "Buongiorno,\r\nvi chiediamo il 7120100A3 e il 7120101A1; per il 7120102A nessuna revisione.\r\nGrazie"),
	}
	p1 := targetC(t, m, ancoraggio.RifComponente(idC1), ancoraggio.AutoritaConfermata, "7120100A2", uuidP(idC1))
	x := strutturaDi(t, proponiC(t, []ancoraggio.ProdottoRichiesto{p1}, ctx), p1.Rif, rC("#1"))

	t.Run("la radice: identità parziale, nessuna stringa (R87)", func(t *testing.T) {
		c := nodoC(t, x, rC("#1")).Codice
		if c.Grezzo.Testo != "7120100A" || c.Grezzo.Campo != ancoraggio.CampoGrezzoID || c.Grezzo.UnitaID != "u:step:#1:id" ||
			c.Grezzo.Posizione.STEP == nil || c.Grezzo.Posizione.STEP.Chiave != "#1" || c.Grezzo.Posizione.STEP.Attributo != "id" {
			t.Errorf("grezzo %+v: atteso l'id del PRODUCT com'è nel file, con il suo localizzatore (I-6)", c.Grezzo)
		}
		if c.Forma == nil || c.Forma.Forma != "step" || c.Forma.Selettore.Campo.Valore != "id" || c.Forma.Originale != "7120100A" {
			t.Errorf("lettura %q %+v: attesa quella dell'id", c.Lettura, c.Forma)
		}
		id := c.Identita
		if id.Base != "7120100" || id.Marcatore != "A" || id.Revisione != nil || !id.Parziale || id.Provenienza != ancoraggio.ProvenienzaRadiceSTEP ||
			id.Motivo != motorea.MotivoComposizioneRevisioneNonDeterminata || id.Qualita != motorea.QualitaCompleta {
			t.Errorf("identità %+v (revisione %s): attese base 7120100, marcatore A, nessuna revisione, parziale, radice_step, motivo revisione_non_determinata",
				id, testoP(id.Revisione))
		}
		if c.Proposto != "" || c.MotivoProposto != motorea.MotivoComposizioneRevisioneNonDeterminata {
			t.Errorf("codice proposto %q, motivo %q: nessuna stringa canonica senza revisione (T-B0-37)", c.Proposto, c.MotivoProposto)
		}
		allegato, msg := idFileC, idMailC
		attesi := []struct {
			valore, fonte string
			allegato      *uuid.UUID
			messaggio     *uuid.UUID
		}{
			{"2", ancoraggio.FonteRevisioneCodiceTarget, nil, nil},
			{"3", ancoraggio.FonteRevisioneMessaggio, nil, &msg},
			{"2", ancoraggio.FonteRevisioneNomeFileSTEP, &allegato, nil},
		}
		if len(id.CandidatiRevisione) != len(attesi) {
			t.Fatalf("candidati %+v, attesi tre (codice del target, messaggio, nome del file)", id.CandidatiRevisione)
		}
		for i, a := range attesi {
			k := id.CandidatiRevisione[i]
			if k.Valore != a.valore || k.Fonte != a.fonte || !reflect.DeepEqual(k.AllegatoID, a.allegato) || !reflect.DeepEqual(k.MessaggioID, a.messaggio) || k.Entita != p1.Rif {
				t.Errorf("candidato %d: %+v, atteso %s da %s per il prodotto %s", i, k, a.valore, a.fonte, p1.Rif)
			}
		}
		if k := id.CandidatiRevisione[2]; k.Posizione.Tipo != "nome_file" || k.Posizione.NomeFile == nil {
			t.Errorf("il candidato del nome del file porta il localizzatore del nome: %+v", k.Posizione)
		}
		if k := id.CandidatiRevisione[1]; k.Posizione.Tipo != "testo" || k.Posizione.Testo == nil {
			t.Errorf("il candidato del messaggio porta il localizzatore dell'occorrenza: %+v", k.Posizione)
		}
		if id.StatoRevisione != ancoraggio.StatoRevisioneCandidata || len(id.FontiSenzaRevisione) != 0 {
			t.Errorf("stato %q, fonti senza revisione %+v: i candidati non decidono, la revisione resta candidata", id.StatoRevisione, id.FontiSenzaRevisione)
		}
	})

	t.Run("il figlio con l'identità completa e il codice confermato (R31 c, T-E1-22)", func(t *testing.T) {
		n := nodoC(t, x, rC("#3"))
		c := n.Codice
		if c.Grezzo.Testo != "7120102A_3" || c.Identita.Base != "7120102" || c.Identita.Marcatore != "A" || testoP(c.Identita.Revisione) != "3" ||
			c.Identita.Parziale || c.Identita.Motivo != "" || c.Identita.Provenienza != ancoraggio.ProvenienzaNodoSTEP {
			t.Errorf("identità %+v (revisione %s), grezzo %q: attesa completa, revisione 3, nodo_step", c.Identita, testoP(c.Identita.Revisione), c.Grezzo.Testo)
		}
		if c.Proposto != "7120102A3" || c.MotivoProposto != "" {
			t.Errorf("codice proposto %q (%s): atteso 7120102A3 dal compositore (T-08)", c.Proposto, c.MotivoProposto)
		}
		if c.Identita.StatoRevisione != ancoraggio.StatoRevisioneCandidata {
			t.Errorf("stato %q: la revisione dello STEP è proposta, non confermata", c.Identita.StatoRevisione)
		}
		if len(c.Identita.CandidatiRevisione) != 0 {
			t.Errorf("candidati %+v: il nome del file e il codice del target non passano ai figli (T-E1-06)", c.Identita.CandidatiRevisione)
		}
		senza := []ancoraggio.FonteSenzaRevisione{{Fonte: ancoraggio.FonteRevisioneMessaggio, Entita: ancoraggio.RifComponente(idC3),
			MessaggioID: uuidP(idMailC), Motivo: ancoraggio.MotivoSenzaRevisioneNonLetta}}
		if !reflect.DeepEqual(c.Identita.FontiSenzaRevisione, senza) {
			t.Errorf("fonti senza revisione %+v, attesa il messaggio senza revisione per il componente deciso", c.Identita.FontiSenzaRevisione)
		}
		if n.Decisione == nil || n.Decisione.ComponenteID != idC3 || n.DecisoDaPersona || n.RigaDecisa == nil || n.RigaLegacy != nil {
			t.Errorf("decisione %+v, riga decisa %+v: il componente per UUID da un aggancio senza persona", n.Decisione, n.RigaDecisa)
		}
		want := &ancoraggio.CodiceDeciso{ComponenteID: uuidP(idC3), Codice: "7120102A", Rev: &revB, Base: letturaC(t, m, "nodo_step.id", "7120102A").Base,
			Origine: ancoraggio.OrigineConfermato, RevProvenienza: ancoraggio.RevProvenienzaFormazioneSTEP}
		if canonico(t, c.Confermato) != canonico(t, want) {
			t.Errorf("codice confermato:\n%s\natteso:\n%s", canonico(t, c.Confermato), canonico(t, want))
		}
	})

	t.Run("il figlio parziale con il codice manuale (T-B0-33)", func(t *testing.T) {
		n := nodoC(t, x, rC("#2"))
		c := n.Codice
		if !c.Identita.Parziale || c.Identita.Revisione != nil || c.Proposto != "" || c.Grezzo.Testo != "7120101A" {
			t.Errorf("il figlio 7120101A è parziale come la radice: %+v", c)
		}
		if n.RigaLegacy == nil || n.RigaLegacy.ID != uidS(0x631) || n.Decisione != nil || c.Confermato != nil {
			t.Errorf("riga legacy %+v, decisione %+v: la riga aperta dello stesso nodo, nessuna decisione", n.RigaLegacy, n.Decisione)
		}
		want := &ancoraggio.CodiceDeciso{Codice: "7120101A1", Rev: &revManuale, Base: lm.Base, Origine: ancoraggio.OrigineManuale,
			RevProvenienza: ancoraggio.RevProvenienzaNonRegistrata}
		if canonico(t, c.Manuale) != canonico(t, want) {
			t.Errorf("codice manuale:\n%s\natteso:\n%s", canonico(t, c.Manuale), canonico(t, want))
		}
		k := c.Identita.CandidatiRevisione
		if len(k) != 1 || k[0].Fonte != ancoraggio.FonteRevisioneMessaggio || k[0].Valore != "1" || k[0].Entita != rC("#2") {
			t.Errorf("candidati %+v: il messaggio per il nodo stesso, che non ha decisione", k)
		}
	})

	t.Run("la saldatura: nessuna lettura, con il motivo (D10)", func(t *testing.T) {
		c := nodoC(t, x, rC("#4")).Codice
		if c.Grezzo.Testo != "SALDATURA_1" || c.Lettura != "" || c.MotivoLettura != ancoraggio.MotivoLetturaAssente || c.Proposto != "" ||
			c.MotivoProposto != ancoraggio.MotivoLetturaAssente || c.Identita.Motivo != ancoraggio.MotivoLetturaAssente || c.Identita.Base != "" ||
			c.Identita.StatoRevisione != ancoraggio.StatoRevisioneAssente {
			t.Errorf("catena della saldatura: %+v", c)
		}
	})

	t.Run("due letture dello stesso id con identità diverse: nessuna si sceglie (T-E1-07)", func(t *testing.T) {
		m2 := motore(t, famCatena(), famCatenaAlt())
		sha := shaS("8")
		d := stepR(t, m2, uidS(0x602), "7120300A_1.stp", sha, []nodoR{{"#1", "7120300A", "TELAIO", ""}, {"#2", "7123000A", "STAFFA", ""}},
			[]arcoS{{"#1", "#2", 1, nil}})
		if len(d.Nodi[1].Letture) != 2 {
			t.Fatalf("il nodo #2 deve avere due letture: %+v", d.Nodi[1].Letture)
		}
		tg := targetC(t, m2, "identificativo:7120300A", ancoraggio.AutoritaConfermata, "7120300A", nil)
		s := proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{d}, CodiciProposti: codiciProposti(m2, d)})
		c := nodoC(t, s[0], ancoraggio.RifNodo(sha, "#2")).Codice
		if c.Lettura != "" || c.MotivoLettura != ancoraggio.MotivoLettureDiscordanti || c.Identita.Base != "" || c.Grezzo.Campo != ancoraggio.CampoGrezzoID ||
			c.Grezzo.Testo != "7123000A" || c.MotivoProposto != ancoraggio.MotivoLettureDiscordanti || len(c.Identita.CandidatiRevisione) != 0 {
			t.Errorf("catena con due letture discordanti: %+v", c)
		}
	})

	t.Run("senza il codice composto: nessuna stringa, la parzialità non si dice e l'identità non sembra completa", func(t *testing.T) {
		senza := ctx
		senza.CodiciProposti = nil
		y := strutturaDi(t, proponiC(t, []ancoraggio.ProdottoRichiesto{p1}, senza), p1.Rif, rC("#1"))
		c := nodoC(t, y, rC("#1")).Codice
		if c.Proposto != "" || c.MotivoProposto != ancoraggio.MotivoPropostoNonComposto || c.Identita.Parziale || c.Identita.Revisione != nil ||
			c.Identita.Motivo != ancoraggio.MotivoPropostoNonComposto {
			t.Errorf("catena senza codice composto: %+v (T-B4-02: senza revisione il motivo dell'identità è non_composto)", c)
		}
		// con la revisione letta dallo STEP l'identità è completa anche senza il codice composto
		if c3 := nodoC(t, y, rC("#3")).Codice; c3.Identita.Motivo != "" || testoP(c3.Identita.Revisione) != "3" {
			t.Errorf("il figlio con la revisione, senza codice composto: %+v", c3.Identita)
		}
	})
}

// ---- i candidati di revisione ----

// TestCandidatiDiRevisioneConLaLoroEntita (T-E1-06, T-B0-27, T-B0-35): ogni indizio con la sua entità, mai propagato.
func TestCandidatiDiRevisioneConLaLoroEntita(t *testing.T) {
	m := motoreCatena(t)

	t.Run("il nome del file solo per la radice; un nome discordante o non letto non è un candidato (T-B0-35)", func(t *testing.T) {
		for _, c := range []struct {
			nome, motivo string
		}{
			{"7120100A_2.stp", ""},
			{"7120199A_2.stp", ancoraggio.MotivoSenzaRevisioneNomeDiscorde},
			{"telaio.stp", ancoraggio.MotivoSenzaRevisioneNomeNonLetto},
		} {
			s := scenaC(t, m, c.nome)
			tg := targetC(t, m, "scenario:caso-acme:1", ancoraggio.AutoritaScenario, "7120100A", nil)
			x := proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s)})[0]
			radice := nodoC(t, x, rC("#1")).Codice.Identita
			if c.motivo == "" {
				if len(radice.CandidatiRevisione) != 1 || radice.CandidatiRevisione[0].Fonte != ancoraggio.FonteRevisioneNomeFileSTEP ||
					radice.CandidatiRevisione[0].Valore != "2" || radice.CandidatiRevisione[0].Entita != tg.Rif || radice.Revisione != nil {
					t.Errorf("%s: candidati %+v, atteso il nome del file per il prodotto, senza revisione inventata", c.nome, radice.CandidatiRevisione)
				}
			} else if len(radice.CandidatiRevisione) != 0 || len(radice.FontiSenzaRevisione) != 1 || radice.FontiSenzaRevisione[0].Motivo != c.motivo ||
				radice.FontiSenzaRevisione[0].Fonte != ancoraggio.FonteRevisioneNomeFileSTEP {
				t.Errorf("%s: candidati %+v, fonti senza revisione %+v; atteso il motivo %s", c.nome, radice.CandidatiRevisione, radice.FontiSenzaRevisione, c.motivo)
			}
			if radice.StatoRevisione == ancoraggio.StatoRevisioneConfermata {
				t.Errorf("%s: un indizio non conferma mai", c.nome)
			}
			for _, n := range x.Nodi[1:] {
				for _, k := range n.Codice.Identita.CandidatiRevisione {
					t.Errorf("%s: il nodo %s ha il candidato %+v: nessuna propagazione ai figli", c.nome, n.Rif, k)
				}
			}
		}
	})

	t.Run("il codice del target per il prodotto; lo scenario e un target senza revisione no", func(t *testing.T) {
		s := scenaC(t, m, "telaio.stp")
		ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s)}
		for _, c := range []struct {
			nome     string
			target   ancoraggio.ProdottoRichiesto
			valore   string
			senzaRev bool
		}{
			{"confermato con la revisione", targetC(t, m, "identificativo:7120100A2", ancoraggio.AutoritaConfermata, "7120100A2", nil), "5", false},
			{"confermato senza revisione", targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil), "", true},
			{"scenario con la revisione", targetC(t, m, "scenario:caso-acme:2", ancoraggio.AutoritaScenario, "7120100A2", nil), "", false},
		} {
			tg := c.target
			if c.valore != "" {
				r := *tg.Revisione
				r.Normalizzata = c.valore
				tg.Revisione = &r
			}
			radice := nodoC(t, proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ctx)[0], rC("#1")).Codice.Identita
			var dalTarget []ancoraggio.CandidatoRevisione
			for _, k := range radice.CandidatiRevisione {
				if k.Fonte == ancoraggio.FonteRevisioneCodiceTarget {
					dalTarget = append(dalTarget, k)
				}
			}
			senza := false
			for _, k := range radice.FontiSenzaRevisione {
				senza = senza || (k.Fonte == ancoraggio.FonteRevisioneCodiceTarget && k.Motivo == ancoraggio.MotivoSenzaRevisioneNonLetta && k.Entita == tg.Rif)
			}
			switch {
			case c.valore != "" && (len(dalTarget) != 1 || dalTarget[0].Valore != c.valore || dalTarget[0].Entita != tg.Rif || dalTarget[0].AllegatoID != nil):
				t.Errorf("%s: candidati del target %+v, atteso %s per il prodotto", c.nome, dalTarget, c.valore)
			case c.valore == "" && len(dalTarget) != 0:
				t.Errorf("%s: candidati del target %+v, attesi nessuno", c.nome, dalTarget)
			case senza != c.senzaRev:
				t.Errorf("%s: fonti senza revisione %+v", c.nome, radice.FontiSenzaRevisione)
			}
		}
	})

	t.Run("il codice del target non passa a un figlio con la stessa base", func(t *testing.T) {
		sha := shaS("b")
		d := stepR(t, m, uidS(0x606), "gruppo.stp", sha, []nodoR{{"#1", "7120100A", "GRUPPO", ""}, {"#2", "7120100A", "GRUPPO INTERNO", ""}},
			[]arcoS{{"#1", "#2", 1, nil}})
		tg := targetC(t, m, "identificativo:7120100A2", ancoraggio.AutoritaConfermata, "7120100A2", nil)
		x := strutturaDi(t, proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{d},
			CodiciProposti: codiciProposti(m, d)}), tg.Rif, ancoraggio.RifNodo(sha, "#1"))
		figlio := nodoC(t, x, ancoraggio.RifNodo(sha, "#2")).Codice.Identita
		if len(figlio.CandidatiRevisione) != 0 || len(figlio.FontiSenzaRevisione) != 0 {
			t.Errorf("il figlio con la base del target: candidati %+v, fonti %+v; il target è della radice", figlio.CandidatiRevisione, figlio.FontiSenzaRevisione)
		}
	})

	t.Run("i messaggi per l'entità con la stessa identità; due entità della stessa base, due candidati", func(t *testing.T) {
		a := scenaC(t, m, "telaio.stp")
		shaB := shaS("9")
		b := stepR(t, m, uidS(0x603), "supporto.stp", shaB, []nodoR{{"#1", "7120101A", "SUPPORTO", ""}}, nil)
		p1 := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
		p2 := targetC(t, m, "identificativo:7120101A", ancoraggio.AutoritaConfermata, "7120101A", nil)
		ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{a, b}, CodiciProposti: codiciProposti(m, a, b),
			CodiciMessaggi: codiciDelMessaggio(t, m, idMailC, "Per il 7120101A1 e per il 7120101B2 vedi allegato.")}
		s := proponiC(t, []ancoraggio.ProdottoRichiesto{p1, p2}, ctx)
		entita := map[string]bool{}
		for _, x := range s {
			for _, n := range x.Nodi {
				for _, k := range n.Codice.Identita.CandidatiRevisione {
					if k.Fonte != ancoraggio.FonteRevisioneMessaggio {
						continue
					}
					if k.Valore != "1" || n.Codice.Identita.Base != "7120101" || k.MessaggioID == nil || *k.MessaggioID != idMailC {
						t.Errorf("nodo %s di %s: candidato %+v fuori dalla sua identità", n.Rif, x.Target, k)
					}
					entita[k.Entita] = true
				}
				if n.Codice.Identita.Base == "7120101" && len(n.Codice.Identita.CandidatiRevisione) != 1 {
					t.Errorf("nodo %s di %s: candidati %+v, atteso uno per entità, mai una scelta", n.Rif, x.Target, n.Codice.Identita.CandidatiRevisione)
				}
			}
		}
		if want := map[string]bool{rC("#2"): true, p2.Rif: true}; !reflect.DeepEqual(entita, want) {
			t.Errorf("entità dei candidati del messaggio %v, attese %v (il nodo dentro P1, il prodotto P2 nel suo file e come nodo interno)", entita, want)
		}
	})
}

// ---- la provenienza della revisione registrata ----

// TestRevProvenienza (T-B0-34, T-E1-22, R97 B; E1R, T-E1R-08; PO-37 nella parte B4): non_registrata,
// coincide_con_formazione_step con almeno una riga legata, decisione_tracciata con lo stato confermata; senza
// revisione nessuna provenienza.
func TestRevProvenienza(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "telaio.stp")
	shaD := shaS("a")
	d := stepR(t, m, uidS(0x604), "piastra.stp", shaD, []nodoR{{"#1", "7120102A", "PIASTRA", " C"}}, nil)
	tg := targetC(t, m, ancoraggio.RifComponente(idC1), ancoraggio.AutoritaConfermata, "7120100A", uuidP(idC1))
	confermato := func(ctx ancoraggio.ContestoStrutturale, chiave string) *ancoraggio.CodiceDeciso {
		t.Helper()
		x := strutturaDi(t, proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ctx), tg.Rif, rC("#1"))
		return nodoC(t, x, rC(chiave)).Codice.Confermato
	}
	base := func(rev2, rev3 *string) ancoraggio.ContestoStrutturale {
		return ancoraggio.ContestoStrutturale{
			Strutture:  []ancoraggio.StrutturaFile{s, d},
			Confermato: []ancoraggio.ComponenteDeciso{componenteC(t, m, idC1, "7120100A", nil), componenteC(t, m, idC2, "7120101A", rev2), componenteC(t, m, idC3, "7120102A", rev3)},
			Decise: []ancoraggio.RigaDecisaLegacy{
				rigaDecisa(uidS(0x641), idFileC, shaFileC, "#1", ancoraggio.StatoRigaConfermata, uuidP(idC1), uuidP(idUtente)),
				rigaDecisa(uidS(0x642), idFileC, shaFileC, "#2", ancoraggio.StatoRigaConfermata, uuidP(idC2), uuidP(idUtente)),
				rigaDecisa(uidS(0x643), idFileC, shaFileC, "#3", ancoraggio.StatoRigaConfermata, uuidP(idC3), uuidP(idUtente)),
				// lo stesso componente in un altro file, con un'altra formazione: «almeno uno» (T-E1-22)
				rigaDecisa(uidS(0x644), uidS(0x604), shaD, "#1", ancoraggio.StatoRigaConfermata, uuidP(idC3), uuidP(idUtente)),
			},
			CodiciProposti: codiciProposti(m, s, d),
		}
	}
	rev7, revB, revC, rev9, spazi := "7", "B", "C", "9", " B "

	for _, c := range []struct {
		nome   string
		rev3   *string
		attesa string
	}{
		{"diversa da tutte le formazioni: non registrata", &rev9, ancoraggio.RevProvenienzaNonRegistrata},
		{"uguale alla formazione del nodo nella scena", &revB, ancoraggio.RevProvenienzaFormazioneSTEP},
		{"uguale alla formazione dell'altro file legato, senza gli spazi ai bordi", &revC, ancoraggio.RevProvenienzaFormazioneSTEP},
		{"uguale senza gli spazi ai bordi, come la copia il legacy", &spazi, ancoraggio.RevProvenienzaFormazioneSTEP},
		{"senza revisione registrata: nessuna provenienza", nil, ""},
	} {
		t.Run(c.nome, func(t *testing.T) {
			k := confermato(base(&rev7, c.rev3), "#3")
			if k == nil || k.RevProvenienza != c.attesa || !reflect.DeepEqual(k.Rev, c.rev3) {
				t.Errorf("codice confermato %+v, attesa la provenienza %q", k, c.attesa)
			}
		})
	}
	t.Run("una formazione vuota non coincide con niente", func(t *testing.T) {
		if k := confermato(base(&rev7, &revB), "#2"); k.RevProvenienza != ancoraggio.RevProvenienzaNonRegistrata {
			t.Errorf("il nodo #2 non ha formazione: %+v", k)
		}
	})

	t.Run("la decisione tracciata: confermata, con R95 A (E1R)", func(t *testing.T) {
		ctx := base(&rev7, &revB)
		il := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
		ctx.DecisioniIdentita = []ancoraggio.DecisioneIdentita{
			{Oggetto: ancoraggio.OggettoDecisioneComponente, ID: idC3, Codice: "7120102A", Revisione: "1", Da: idUtente, Il: il,
				EvidenzeViste: []ancoraggio.EvidenzaVista{ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaStepEntita, "7120102A_3")}},
			{Oggetto: ancoraggio.OggettoDecisioneComponente, ID: idC2, Codice: "7120105A", Revisione: "2", Da: idUtente, Il: il},
			// una decisione su un documento non tocca i componenti: è di B5
			{Oggetto: ancoraggio.OggettoDecisioneDocumento, ID: idC1, Codice: "7120100A", Revisione: "9", Da: idUtente, Il: il},
		}
		x := strutturaDi(t, proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ctx), tg.Rif, rC("#1"))
		n3 := nodoC(t, x, rC("#3")).Codice
		if n3.Confermato == nil || n3.Confermato.Codice != "7120102A" || testoP(n3.Confermato.Rev) != "1" ||
			n3.Confermato.RevProvenienza != ancoraggio.RevProvenienzaDecisioneTracciata || n3.Confermato.Base.Normalizzata != "7120102" ||
			n3.Identita.StatoRevisione != ancoraggio.StatoRevisioneConfermata {
			t.Errorf("nodo #3: confermato %+v, stato %q; attesi la decisione tracciata e lo stato confermata", n3.Confermato, n3.Identita.StatoRevisione)
		}
		if testoP(n3.Identita.Revisione) != "3" {
			t.Errorf("l'identità del nodo resta quella dello STEP, accanto alla decisione (R95 A): %s", testoP(n3.Identita.Revisione))
		}
		n2 := nodoC(t, x, rC("#2")).Codice
		if n2.Confermato.Codice != "7120105A" || n2.Confermato.Base.Normalizzata != "" || n2.Identita.StatoRevisione != ancoraggio.StatoRevisioneConfermata {
			t.Errorf("nodo #2: con un codice deciso diverso la base del componente non vale (qui la grammatica non legge): %+v", n2.Confermato)
		}
		n1 := nodoC(t, x, rC("#1")).Codice
		if n1.Confermato.RevProvenienza == ancoraggio.RevProvenienzaDecisioneTracciata || n1.Identita.StatoRevisione == ancoraggio.StatoRevisioneConfermata {
			t.Errorf("nodo #1: una decisione su un documento non conferma il componente: %+v", n1.Confermato)
		}
	})

	t.Run("la decisione senza revisione: Rev nil, decisa (T-B4-21)", func(t *testing.T) {
		ctx := base(&rev7, &revB)
		ctx.DecisioniIdentita = []ancoraggio.DecisioneIdentita{{Oggetto: ancoraggio.OggettoDecisioneComponente, ID: idC3, Codice: "7120102A",
			Revisione: " ", Da: idUtente, Il: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)}}
		n3 := nodoC(t, strutturaDi(t, proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ctx), tg.Rif, rC("#1")), rC("#3")).Codice
		if n3.Confermato == nil || n3.Confermato.Rev != nil || n3.Confermato.Codice != "7120102A" ||
			n3.Confermato.RevProvenienza != ancoraggio.RevProvenienzaDecisioneTracciata || n3.Identita.StatoRevisione != ancoraggio.StatoRevisioneConfermata {
			t.Errorf("decisione senza revisione: %+v (rev %s), stato %q; attesi Rev nil, decisione_tracciata, confermata", n3.Confermato,
				testoP(n3.Confermato.Rev), n3.Identita.StatoRevisione)
		}
	})

	t.Run("componente.rev del legacy non conferma mai (R97 B)", func(t *testing.T) {
		x := strutturaDi(t, proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, base(&rev7, &revB)), tg.Rif, rC("#1"))
		for _, n := range x.Nodi {
			if n.Codice.Identita.StatoRevisione == ancoraggio.StatoRevisioneConfermata {
				t.Errorf("nodo %s: confermata senza una decisione del modello nuovo", n.Rif)
			}
		}
	})
}

// ---- DecisioneIdentita ----

// TestDecisioneIdentitaEvidenzaNuova (E1R §5.3, T-E1R-08, T-B4-22; PO-37, la variante del modello nuovo nella parte
// B4): le coppie si costruiscono con EvidenzaDa dal testo grezzo della fonte: il cartiglio com'è, il grezzo dell'id del
// nodo, il nome del file, «codice rev» del documento. Il cartiglio già visto non è nuovo, nemmeno con gli spazi ai
// bordi; uno con un'altra revisione sì; la stessa coppia con un'altra fonte è nuova.
func TestDecisioneIdentitaEvidenzaNuova(t *testing.T) {
	cartiglio, step := ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaCartiglio, " 7120102A1 "), ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaStepEntita, "7120102A_3")
	nome, documento := ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaNomeFile, "7120102A_1.pdf"), ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaDocumento, "7120102A", "1")
	for _, c := range []struct{ got, want ancoraggio.EvidenzaVista }{
		{cartiglio, ancoraggio.EvidenzaVista{Fonte: "cartiglio", Valore: "7120102A1"}},
		{step, ancoraggio.EvidenzaVista{Fonte: "step_entita", Valore: "7120102A_3"}},
		{nome, ancoraggio.EvidenzaVista{Fonte: "nome_file", Valore: "7120102A_1.pdf"}},
		{documento, ancoraggio.EvidenzaVista{Fonte: "documento", Valore: "7120102A 1"}},
		{ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaDocumento, "7120102A", " "), ancoraggio.EvidenzaVista{Fonte: "documento", Valore: "7120102A"}},
	} {
		if c.got != c.want {
			t.Errorf("EvidenzaDa: %+v, attesa %+v (T-B4-22)", c.got, c.want)
		}
	}
	d := ancoraggio.DecisioneIdentita{Oggetto: ancoraggio.OggettoDecisioneComponente, ID: idC3, Codice: "7120102A", Revisione: "1",
		EvidenzeViste: []ancoraggio.EvidenzaVista{cartiglio, step, documento}, Da: idUtente, Il: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)}
	for _, c := range []struct {
		e     ancoraggio.EvidenzaVista
		nuova bool
	}{
		{ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaCartiglio, "7120102A1"), false},
		{ancoraggio.EvidenzaVista{Fonte: ancoraggio.FonteEvidenzaCartiglio, Valore: "7120102A1  "}, false},
		{ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaCartiglio, "7120102A2"), true},
		{ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaStepEntita, "7120102A_3"), false},
		{ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaNomeFile, "7120102A1"), true},
		{ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaDocumento, "7120102A", "1"), false},
		{ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaDocumento, "7120102A", "2"), true},
	} {
		if got := ancoraggio.EvidenzaNuova(d, c.e); got != c.nuova {
			t.Errorf("%+v: nuova %v, attesa %v", c.e, got, c.nuova)
		}
	}
	if !ancoraggio.EvidenzaNuova(ancoraggio.DecisioneIdentita{}, ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaCartiglio, "x")) {
		t.Error("senza evidenze viste ogni evidenza è nuova")
	}
}

// ---- la decisione del nodo per UUID ----

// TestDecisioneDelNodoPerUUID (emendamento E1 §4.2; T-E1-04, T-E1-05; la parte B4 di PO-21): due fotografie.
//   - La rinomina del componente confermato: lo stesso componente, lo stesso nodo, la stessa chiave, la stessa riga
//     decisa; il codice confermato è quello nuovo.
//   - La correzione di una riga aperta dall'operatore: lo stesso nodo, la stessa riga, con il codice manuale.
//   - Lo stesso contenuto in un altro allegato: la riga di quell'allegato, non quella del primo.
func TestDecisioneDelNodoPerUUID(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "telaio.stp")
	tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	decisa := rigaDecisa(uidS(0x651), idFileC, shaFileC, "#3", ancoraggio.StatoRigaConfermata, uuidP(idC3), uuidP(idUtente))
	aperta := rigaAperta(uidS(0x652), idFileC, shaFileC, "#2")
	foto := func(codiceC3 string, aperta ancoraggio.RigaPropostaLegacy) ancoraggio.StrutturaProdotto {
		t.Helper()
		ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s),
			Confermato: []ancoraggio.ComponenteDeciso{componenteC(t, m, idC3, codiceC3, nil)},
			Proposto:   []ancoraggio.RigaPropostaLegacy{aperta}, Decise: []ancoraggio.RigaDecisaLegacy{decisa}}
		return proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ctx)[0]
	}
	manuale, lm := "7120109A", letturaC(t, m, "nodo_step.id", "7120109A")
	corretta := aperta
	corretta.CodiceManuale, corretta.LetturaManuale = &manuale, &lm

	prima, dopo := foto("7120102A", aperta), foto("7120108A", corretta)
	p3, d3 := nodoC(t, prima, rC("#3")), nodoC(t, dopo, rC("#3"))
	if p3.Rif != d3.Rif || p3.Decisione == nil || d3.Decisione == nil || p3.Decisione.ComponenteID != idC3 || d3.Decisione.ComponenteID != idC3 ||
		p3.RigaDecisa.ID != d3.RigaDecisa.ID || p3.RigaDecisa.Chiave != "#3" || !d3.DecisoDaPersona {
		t.Fatalf("dopo la rinomina il nodo non trova la sua decisione: prima %+v, dopo %+v", p3, d3)
	}
	if p3.Codice.Confermato.Codice != "7120102A" || d3.Codice.Confermato.Codice != "7120108A" || d3.Decisione.Codice != "7120108A" {
		t.Errorf("il codice confermato segue la rinomina: %q → %q", p3.Codice.Confermato.Codice, d3.Codice.Confermato.Codice)
	}
	if canonico(t, p3.Codice.Identita) != canonico(t, d3.Codice.Identita) || canonico(t, p3.Codice.Grezzo) != canonico(t, d3.Codice.Grezzo) ||
		p3.Codice.Proposto != d3.Codice.Proposto {
		t.Errorf("la proposta dello STEP non cambia con la rinomina: %+v e %+v", p3.Codice.Identita, d3.Codice.Identita)
	}

	p2, d2 := nodoC(t, prima, rC("#2")), nodoC(t, dopo, rC("#2"))
	if p2.Rif != d2.Rif || p2.RigaLegacy == nil || d2.RigaLegacy == nil || p2.RigaLegacy.ID != d2.RigaLegacy.ID || p2.Codice.Manuale != nil ||
		d2.Codice.Manuale == nil || d2.Codice.Manuale.Codice != "7120109A" || d2.Codice.Manuale.Rev != nil || d2.Codice.Manuale.RevProvenienza != "" {
		t.Errorf("la riga aperta corretta resta lo stesso nodo, con il codice manuale: prima %+v, dopo %+v", p2, d2)
	}
	if canonico(t, p2.Codice.Grezzo) != canonico(t, d2.Codice.Grezzo) || d2.Codice.Grezzo.Testo != "7120101A" {
		t.Errorf("il grezzo non cambia con la correzione (I-6): %+v", d2.Codice.Grezzo)
	}

	t.Run("lo stesso contenuto in un altro allegato: la sua riga", func(t *testing.T) {
		copia := s
		copia.AllegatoID = uidS(0x605)
		ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s, copia}, CodiciProposti: codiciProposti(m, s, copia),
			Confermato: []ancoraggio.ComponenteDeciso{componenteC(t, m, idC3, "7120102A", nil)}, Decise: []ancoraggio.RigaDecisaLegacy{decisa}}
		for _, x := range proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ctx) {
			n := nodoC(t, x, rC("#3"))
			if (x.AllegatoID == idFileC) != (n.Decisione != nil) {
				t.Errorf("allegato %s: decisione %+v; la riga decisa è solo del primo allegato", x.AllegatoID, n.Decisione)
			}
		}
	})

	t.Run("una riga scartata non è una decisione su un componente", func(t *testing.T) {
		scartata := rigaDecisa(uidS(0x653), idFileC, shaFileC, "#2", ancoraggio.StatoRigaScartata, uuidP(idC2), uuidP(idUtente))
		ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s), Decise: []ancoraggio.RigaDecisaLegacy{scartata},
			Confermato: []ancoraggio.ComponenteDeciso{componenteC(t, m, idC2, "7120101A", nil)}}
		n := nodoC(t, proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ctx)[0], rC("#2"))
		if n.Decisione != nil || n.RigaDecisa == nil || n.RigaDecisa.Stato != ancoraggio.StatoRigaScartata || !n.DecisoDaPersona {
			t.Errorf("nodo scartato: %+v", n)
		}
	})
}

// ---- l'arco con la decisione accanto ----

// TestArcoConLaDecisioneAccanto (contratto §1.4; T-14, R95 A, T-B0-24): l'arco confermato fra i componenti decisi dei
// due nodi sta accanto all'arco dei fatti, con la sua quantità; con una quantità diversa la decisione resta il valore
// corrente e QuantitaDiscorde ne dà il segnale; senza la decisione di un estremo, nessun arco accanto.
func TestArcoConLaDecisioneAccanto(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "telaio.stp")
	tg := targetC(t, m, ancoraggio.RifComponente(idC1), ancoraggio.AutoritaConfermata, "7120100A", uuidP(idC1))
	tre, uno := 3, 1
	confermate := []ancoraggio.ArcoPercorso{
		{Padre: ancoraggio.RifComponente(idC1), Figlio: ancoraggio.RifComponente(idC2), Quantita: &tre, Origine: ancoraggio.OrigineArcoConfermato},
		{Padre: ancoraggio.RifComponente(idC1), Figlio: ancoraggio.RifComponente(idC3), Quantita: &uno, Origine: ancoraggio.OrigineArcoConfermato},
	}
	ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s),
		Confermato:      []ancoraggio.ComponenteDeciso{componenteC(t, m, idC1, "7120100A", nil), componenteC(t, m, idC2, "7120101A", nil), componenteC(t, m, idC3, "7120102A", nil)},
		ArchiConfermati: confermate,
		Decise: []ancoraggio.RigaDecisaLegacy{
			rigaDecisa(uidS(0x661), idFileC, shaFileC, "#1", ancoraggio.StatoRigaConfermata, uuidP(idC1), uuidP(idUtente)),
			rigaDecisa(uidS(0x662), idFileC, shaFileC, "#2", ancoraggio.StatoRigaConfermata, uuidP(idC2), uuidP(idUtente)),
			rigaDecisa(uidS(0x663), idFileC, shaFileC, "#3", ancoraggio.StatoRigaConfermata, uuidP(idC3), uuidP(idUtente)),
		}}
	x := proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ctx)[0]
	archi := map[string]ancoraggio.ArcoProposto{}
	for _, a := range x.Archi {
		archi[a.Figlio] = a
	}
	a2, a3, a4 := archi[rC("#2")], archi[rC("#3")], archi[rC("#4")]
	if a2.Decisione == nil || *a2.Decisione.Quantita != 3 || *a2.Quantita != 2 || !ancoraggio.QuantitaDiscorde(a2) {
		t.Errorf("arco #1→#2: %+v; attesi la quantità dei fatti 2, accanto quella confermata 3, e la discordanza", a2)
	}
	if a3.Decisione == nil || *a3.Decisione.Quantita != 1 || ancoraggio.QuantitaDiscorde(a3) {
		t.Errorf("arco #1→#3: %+v; attese le due quantità uguali, nessuna discordanza", a3)
	}
	if a4.Decisione != nil || ancoraggio.QuantitaDiscorde(a4) {
		t.Errorf("arco #1→#4: %+v; la saldatura non ha decisione, niente accanto", a4)
	}
	if !reflect.DeepEqual(ctx.ArchiConfermati, confermate) {
		t.Error("gli archi confermati del contesto sono cambiati")
	}
	senzaQuantita := ancoraggio.ArcoProposto{Decisione: &ancoraggio.ArcoPercorso{Quantita: &tre}}
	if ancoraggio.QuantitaDiscorde(senzaQuantita) {
		t.Error("una quantità che i fatti non danno non discorda")
	}

	// T-B4-23: con la riga della radice aperta (com'è con «Conferma l'albero»), sulla BOM di lavoro la radice scelta con
	// il gesto 3 vale il componente del target, solo per gli archi; la radice non ha una Decisione; sulla struttura
	// candidata, o senza il componente del target, niente.
	radiceAperta := ctx
	radiceAperta.Proposto = []ancoraggio.RigaPropostaLegacy{rigaAperta(uidS(0x664), idFileC, shaFileC, "#1")}
	radiceAperta.Decise = ctx.Decise[1:]
	fonte := &ancoraggio.RiferimentoFonte{Tipo: ancoraggio.TipoRiferimentoStep, DocumentoID: uidS(0x665), Sha256: shaFileC,
		AllegatoID: uuidP(idFileC), Radice: "#1", Forma: ancoraggio.FormaRiferimentoSmistamento}
	bom := tg
	bom.FonteConfermata = fonte
	senzaComponente := bom
	senzaComponente.Rif, senzaComponente.ComponenteID = "identificativo:7120100A", nil
	for _, c := range []struct {
		nome     string
		tg       ancoraggio.ProdottoRichiesto
		decisi   bool
		statoBOM bool
	}{
		{"la BOM di lavoro con il componente del target", bom, true, true},
		{"la struttura candidata", tg, false, false},
		{"la BOM di lavoro senza il componente del target", senzaComponente, false, true},
	} {
		x := proponiC(t, []ancoraggio.ProdottoRichiesto{c.tg}, radiceAperta)[0]
		if (x.Stato == ancoraggio.StatoBOMDiLavoroProposta) != c.statoBOM {
			t.Fatalf("%s: stato %s", c.nome, x.Stato)
		}
		// Riscritta per T-B4-38: sulla BOM di lavoro con il componente del target la radice porta accanto il suo codice
		// confermato, senza una Decisione (T-B4-23); sulla struttura candidata e senza il componente niente.
		if r := nodoC(t, x, rC("#1")); r.Decisione != nil || (r.Codice.Confermato != nil) != c.decisi || r.RigaLegacy == nil ||
			(c.decisi && *r.Codice.Confermato.ComponenteID != idC1) {
			t.Errorf("%s: la radice non ha una Decisione (T-B4-23), e il confermato del target solo sulla BOM (T-B4-38): %+v", c.nome, r)
		}
		archi := map[string]ancoraggio.ArcoProposto{}
		for _, a := range x.Archi {
			archi[a.Figlio] = a
		}
		a2, a3 := archi[rC("#2")], archi[rC("#3")]
		if c.decisi {
			if a2.Decisione == nil || a2.Decisione.Padre != ancoraggio.RifComponente(idC1) || *a2.Decisione.Quantita != 3 || !ancoraggio.QuantitaDiscorde(a2) ||
				a3.Decisione == nil || *a3.Decisione.Quantita != 1 || archi[rC("#4")].Decisione != nil {
				t.Errorf("%s: archi della radice %+v %+v; attese le relazioni del componente del target accanto", c.nome, a2, a3)
			}
		} else if a2.Decisione != nil || a3.Decisione != nil {
			t.Errorf("%s: archi della radice con una decisione: %+v %+v", c.nome, a2.Decisione, a3.Decisione)
		}
	}

	// Una radice con la sua riga decisa segue la regola di sempre, anche sulla BOM di lavoro: con la radice decisa come
	// un altro componente, le relazioni del componente del target non vanno accanto.
	altra := radiceAperta
	altra.Proposto = nil
	altra.Confermato = append(append([]ancoraggio.ComponenteDeciso(nil), ctx.Confermato...), componenteC(t, m, uidS(0x666), "7120109A", nil))
	altra.Decise = append([]ancoraggio.RigaDecisaLegacy{rigaDecisa(uidS(0x667), idFileC, shaFileC, "#1", ancoraggio.StatoRigaConfermata, uuidP(uidS(0x666)), uuidP(idUtente))},
		ctx.Decise[1:]...)
	x = proponiC(t, []ancoraggio.ProdottoRichiesto{bom}, altra)[0]
	if r := nodoC(t, x, rC("#1")); r.Decisione == nil || r.Decisione.ComponenteID != uidS(0x666) {
		t.Fatalf("la radice decisa: %+v", r.Decisione)
	}
	for _, a := range x.Archi {
		if a.Decisione != nil {
			t.Errorf("radice decisa come un altro componente: l'arco %s→%s ha la decisione %+v (T-B4-23)", a.Padre, a.Figlio, a.Decisione)
		}
	}
}

// ---- la regola unica dell'identità degli indizi ----

// TestRegolaUnicaDellIdentita (T-B4-06 rivisto, T-E1-06, R86): una regola sola per il nome del file, il codice del
// target e i messaggi: lo stesso namespace, la stessa base completa e il marcatore compatibile (uguale quando c'è da
// tutte e due le parti, oppure scritto da una parte sola). Due marcatori scritti e diversi: niente candidato, la fonte
// fra quelle senza revisione con marcatore_diverso.
//
// Riscritta per B4 (fase 2, T-B4-30): con la regola unica del marcatore anche nelle radici candidate, il target
// con la B non ha come radice candidata la radice con la A, e senza fonte confermata non ha strutture. La struttura si
// ottiene con la fonte confermata sotto quella radice (la BOM di lavoro sotto la radice scelta, R76 A), con la
// compatibilità discordante: la radice non è il prodotto, e l'entità dei suoi indizi è il nodo (T-E1-06).
func TestRegolaUnicaDellIdentita(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "7120100B_2.stp")
	msg := codiciDelMessaggio(t, m, idMailC, "Il 7120100B3 e il 7120100A4; poi il 7120100A5.")
	for i := range msg {
		if msg[i].Lettura.Forma.Originale == "7120100A5" {
			msg[i].Lettura.Forma.Marcatore = nil // una forma che non scrive il marcatore: compatibile da una parte sola
		}
	}
	ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s), CodiciMessaggi: msg}
	conB := targetC(t, m, "identificativo:7120100B2", ancoraggio.AutoritaConfermata, "7120100B2", nil)
	if conB.Marcatore != "B" {
		t.Fatalf("il target deve avere il marcatore B: %+v", conB)
	}
	if strutture, _ := proponiS(t, []ancoraggio.ProdottoRichiesto{conB}, ctx); len(strutture) != 0 {
		t.Fatalf("un target con la B non ha come radice candidata un nodo con la A (T-B4-30): %+v", strutture)
	}
	conB.FonteConfermata = &ancoraggio.RiferimentoFonte{Tipo: ancoraggio.TipoRiferimentoStep, DocumentoID: uidS(0x6a1), Sha256: shaFileC,
		AllegatoID: uuidP(idFileC), Radice: "#1", Forma: ancoraggio.FormaRiferimentoSmistamento}
	bom := proponiC(t, []ancoraggio.ProdottoRichiesto{conB}, ctx)[0]
	if bom.Stato != ancoraggio.StatoBOMDiLavoroProposta || bom.Compatibilita != motorea.CompatibilitaDiscordante {
		t.Fatalf("la BOM sotto la radice scelta con un altro marcatore: %s %s", bom.Stato, bom.Compatibilita)
	}
	radice := nodoC(t, bom, rC("#1")).Codice.Identita
	valori := map[string]string{}
	for _, k := range radice.CandidatiRevisione {
		valori[k.Fonte+":"+k.Valore] = k.Entita
	}
	if want := map[string]string{"messaggio:4": rC("#1"), "messaggio:5": rC("#1")}; !reflect.DeepEqual(valori, want) {
		t.Errorf("candidati %v, attesi i due messaggi con il marcatore A o senza: %v", valori, want)
	}
	diversi := map[string]bool{}
	for _, k := range radice.FontiSenzaRevisione {
		if k.Motivo == ancoraggio.MotivoSenzaRevisioneMarcatoreDiverso {
			diversi[k.Fonte] = true
		}
	}
	if want := map[string]bool{ancoraggio.FonteRevisioneNomeFileSTEP: true, ancoraggio.FonteRevisioneCodiceTarget: true, ancoraggio.FonteRevisioneMessaggio: true}; !reflect.DeepEqual(diversi, want) {
		t.Errorf("fonti con marcatore_diverso %v, attese il nome del file, il codice del target e il messaggio con la B: %+v", diversi, radice.FontiSenzaRevisione)
	}

	t.Run("il target senza marcatore è compatibile", func(t *testing.T) {
		senza := conB
		senza.Marcatore = ""
		id := nodoC(t, proponiC(t, []ancoraggio.ProdottoRichiesto{senza}, ctx)[0], rC("#1")).Codice.Identita
		trovato := false
		for _, k := range id.CandidatiRevisione {
			trovato = trovato || (k.Fonte == ancoraggio.FonteRevisioneCodiceTarget && k.Valore == "2")
		}
		if !trovato {
			t.Errorf("il codice del target senza marcatore: candidati %+v", id.CandidatiRevisione)
		}
	})
}

// ---- altri casi della catena e delle decisioni ----

// TestCodiceManualeSoloNelSuoAllegato (T-B0-33; emendamento E1 §4.2): lo stesso contenuto in due allegati, la riga
// aperta corretta dall'operatore solo nel primo: il secondo allegato non vede il codice manuale (la riga si cerca per
// allegato e nodo).
func TestCodiceManualeSoloNelSuoAllegato(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "telaio.stp")
	copia := s
	copia.AllegatoID = uidS(0x705)
	tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	manuale := "7120109A"
	aperta := rigaAperta(uidS(0x706), idFileC, shaFileC, "#2")
	aperta.CodiceManuale = &manuale
	ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s, copia}, CodiciProposti: codiciProposti(m, s, copia),
		Proposto: []ancoraggio.RigaPropostaLegacy{aperta}}
	strutture := proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ctx)
	if len(strutture) != 2 {
		t.Fatalf("due strutture, una per allegato: %d", len(strutture))
	}
	for _, x := range strutture {
		n := nodoC(t, x, rC("#2"))
		if (x.AllegatoID == idFileC) != (n.Codice.Manuale != nil) || (x.AllegatoID == idFileC) != (n.RigaLegacy != nil) {
			t.Errorf("allegato %s: manuale %+v, riga %+v; il codice manuale è solo della riga del primo allegato", x.AllegatoID, n.Codice.Manuale, n.RigaLegacy)
		}
	}
}

// TestDecisioneSenzaComponenteNelContesto (emendamento E1 §4.2; E1R): una riga decisa confermata il cui componente non
// è fra i confermati del contesto, con una DecisioneIdentita sullo stesso componente: nessuna decisione, nessun codice
// confermato, nessuno stato confermata; la riga decisa resta visibile.
func TestDecisioneSenzaComponenteNelContesto(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "telaio.stp")
	tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s),
		Decise: []ancoraggio.RigaDecisaLegacy{rigaDecisa(uidS(0x791), idFileC, shaFileC, "#3", ancoraggio.StatoRigaConfermata, uuidP(idC3), uuidP(idUtente))},
		DecisioniIdentita: []ancoraggio.DecisioneIdentita{{Oggetto: ancoraggio.OggettoDecisioneComponente, ID: idC3, Codice: "7120102A", Revisione: "4",
			Da: idUtente, Il: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)}}}
	n := nodoC(t, proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ctx)[0], rC("#3"))
	if n.Decisione != nil || n.Codice.Confermato != nil || n.Codice.Identita.StatoRevisione == ancoraggio.StatoRevisioneConfermata ||
		n.RigaDecisa == nil || !n.DecisoDaPersona {
		t.Errorf("decisione o stato confermata senza il componente nel contesto: %+v", n)
	}
}

// TestParzialeSoloDalMotivoDelCompositore (T-B3-01, R87): l'identità è parziale solo per il motivo
// revisione_non_determinata; un'altra stringa assente (qui forma_cartiglio_assente) non la fa parziale.
func TestParzialeSoloDalMotivoDelCompositore(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "telaio.stp")
	tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	cp := codiciProposti(m, s)
	for k, v := range cp {
		if v.Testo == "" {
			v.Motivo = motorea.MotivoComposizioneFormaAssente
			cp[k] = v
		}
	}
	c := nodoC(t, proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: cp})[0], rC("#1")).Codice
	if c.Identita.Parziale || c.MotivoProposto != motorea.MotivoComposizioneFormaAssente || c.Identita.Motivo == motorea.MotivoComposizioneRevisioneNonDeterminata {
		t.Errorf("parziale %v con il motivo %q (identità: %q): la parzialità è solo revisione_non_determinata", c.Identita.Parziale, c.MotivoProposto, c.Identita.Motivo)
	}
}

// fattiConAlternative: i fatti della scena con le formazioni alternative di un nodo (evidenza.rev_alternative).
func fattiConAlternative(t *testing.T, sha string, nodi []nodoR, archi []arcoS, alternative map[string][]string) *fotorfq.Fatti {
	t.Helper()
	n := []map[string]any{}
	for _, x := range nodi {
		ev := map[string]any{}
		if a := alternative[x.chiave]; len(a) > 0 {
			ev["rev_alternative"] = a
		}
		n = append(n, map[string]any{"chiave": x.chiave, "id_grezzo": x.id, "nome_grezzo": x.nome, "descrizione_grezza": "", "rev_grezza": x.rev, "evidenza": ev})
	}
	r := []map[string]any{}
	for _, a := range archi {
		r = append(r, map[string]any{"padre": a.padre, "figlio": a.figlio, "qta": a.qta, "evidenza": map[string]any{}})
	}
	raw, err := json.Marshal(map[string]any{"struttura": map[string]any{
		"versione": 3, "schema": "AP214", "radici": []string{nodi[0].chiave}, "avvisi": []string{}, "nodi": n, "relazioni": r,
		"limiti": map[string]any{"troncato": false},
		"scarti": map[string]any{"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0, "occorrenze_su_se_stesse": 0, "testi_troncati": 0},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return fattiGrezzi(t, sha, raw, testoS(""))
}

// TestFormazioneSoloDeiNodiLegati (T-E1-22, alla lettera): la revisione registrata di un componente coincide solo con la
// formazione rev_grezza dei nodi legati a lui da una riga decisa: non con quella di un nodo legato a un altro
// componente, né con una formazione alternativa dello stesso nodo.
func TestFormazioneSoloDeiNodiLegati(t *testing.T) {
	m := motoreCatena(t)
	nodi, archi := nodiScenaC() // #3 ha la formazione «B», #2 nessuna
	s := strutturaS(t, fileS(t, m, idFileC, "telaio.stp", "stp", shaFileC, fattiConAlternative(t, shaFileC, nodi, archi, map[string][]string{"#3": {"Z"}})))
	for _, n := range s.Nodi {
		if n.Chiave == "#3" && !reflect.DeepEqual(n.Formazioni, []string{"B"}) {
			t.Fatalf("le formazioni del nodo #3 sono solo rev_grezza: %v", n.Formazioni)
		}
	}
	tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	revB, revZ := "B", "Z"
	decise := []ancoraggio.RigaDecisaLegacy{
		rigaDecisa(uidS(0x7a1), idFileC, shaFileC, "#2", ancoraggio.StatoRigaConfermata, uuidP(idC2), uuidP(idUtente)),
		rigaDecisa(uidS(0x7a2), idFileC, shaFileC, "#3", ancoraggio.StatoRigaConfermata, uuidP(idC3), uuidP(idUtente)),
	}
	for _, c := range []struct {
		nome, chiave string
		k2, k3       ancoraggio.ComponenteDeciso
		attesa       string
	}{
		{"la formazione di un nodo legato a un altro componente", "#2", componenteC(t, m, idC2, "7120101A", &revB), componenteC(t, m, idC3, "7120102A", nil),
			ancoraggio.RevProvenienzaNonRegistrata},
		{"una formazione alternativa dello stesso nodo", "#3", componenteC(t, m, idC2, "7120101A", nil), componenteC(t, m, idC3, "7120102A", &revZ),
			ancoraggio.RevProvenienzaNonRegistrata},
		{"la formazione rev_grezza del nodo legato", "#3", componenteC(t, m, idC2, "7120101A", nil), componenteC(t, m, idC3, "7120102A", &revB),
			ancoraggio.RevProvenienzaFormazioneSTEP},
	} {
		ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s),
			Confermato: []ancoraggio.ComponenteDeciso{c.k2, c.k3}, Decise: decise}
		k := nodoC(t, proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ctx)[0], rC(c.chiave)).Codice.Confermato
		if k == nil || k.RevProvenienza != c.attesa {
			t.Errorf("%s: %+v, attesa %q (T-E1-22)", c.nome, k, c.attesa)
		}
	}
}

// TestEntitaDelProdottoPrimaDelComponente (T-E1-06): l'entità dei candidati della radice della struttura è il prodotto
// anche quando la radice ha una riga decisa con il suo componente.
func TestEntitaDelProdottoPrimaDelComponente(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "7120100A_2.stp")
	tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s),
		Confermato: []ancoraggio.ComponenteDeciso{componenteC(t, m, idC1, "7120100A", nil)},
		Decise:     []ancoraggio.RigaDecisaLegacy{rigaDecisa(uidS(0x7b1), idFileC, shaFileC, "#1", ancoraggio.StatoRigaConfermata, uuidP(idC1), uuidP(idUtente))}}
	r := nodoC(t, proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ctx)[0], rC("#1"))
	if r.Decisione == nil || len(r.Codice.Identita.CandidatiRevisione) == 0 {
		t.Fatalf("la radice deve avere la decisione e almeno un candidato: %+v", r)
	}
	for _, k := range r.Codice.Identita.CandidatiRevisione {
		if k.Entita != tg.Rif {
			t.Errorf("candidato %+v con l'entità %s, atteso il prodotto %s", k, k.Entita, tg.Rif)
		}
	}
}

// TestUscitaSenzaMemoriaCondivisa (determinismo; l'uscita non condivide memoria con l'ingresso): cambiare l'uscita (il localizzatore di un candidato, la forma
// della catena, la base del codice confermato, la lettura della decisione) non cambia gli ingressi.
func TestUscitaSenzaMemoriaCondivisa(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "7120100A_2.stp")
	tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s),
		Confermato: []ancoraggio.ComponenteDeciso{componenteC(t, m, idC3, "7120102A", nil)},
		Decise:     []ancoraggio.RigaDecisaLegacy{rigaDecisa(uidS(0x7c1), idFileC, shaFileC, "#3", ancoraggio.StatoRigaConfermata, uuidP(idC3), uuidP(idUtente))}}
	prima := canonico(t, ctx)
	x := proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ctx)[0]
	r, n3 := nodoC(t, x, rC("#1")), nodoC(t, x, rC("#3"))
	for _, k := range r.Codice.Identita.CandidatiRevisione {
		if k.Posizione.NomeFile != nil && k.Posizione.NomeFile.Stem != nil {
			k.Posizione.NomeFile.Stem.Inizio = 999
		}
	}
	r.Codice.Forma.Base.Segmenti[0].Normalizzato = "MUTATO"
	r.Codice.Forma.Marcatore.Valore = "Z"
	n3.Codice.Confermato.Base.Segmenti[0].Normalizzato = "MUTATO"
	n3.Decisione.Lettura.Base.Segmenti[0].Normalizzato = "MUTATO"
	n3.Decisione.Lettura.Marcatore.Valore = "Z"
	if canonico(t, ctx) != prima {
		t.Error("cambiare l'uscita ha cambiato gli ingressi: l'uscita condivide memoria con l'ingresso")
	}
}

// ---- le identità parziali sulla BOM di lavoro ----

// TestIdentitaParzialiSullaBOMDiLavoro (R85; contratto §1.3, la riga B2 del §7): con la fonte confermata la struttura
// sotto la radice scelta è la BOM di lavoro, e nello stesso calcolo i nodi hanno la loro identità, anche parziale: la
// stessa catena della struttura candidata.
func TestIdentitaParzialiSullaBOMDiLavoro(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "7120100A_2.stp")
	ctx := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s)}
	candidato := targetC(t, m, ancoraggio.RifComponente(idC1), ancoraggio.AutoritaConfermata, "7120100A", uuidP(idC1))
	bom := candidato
	bom.FonteConfermata = &ancoraggio.RiferimentoFonte{Tipo: ancoraggio.TipoRiferimentoStep, DocumentoID: uidS(0x671), Sha256: shaFileC,
		AllegatoID: uuidP(idFileC), Radice: "#1", Forma: ancoraggio.FormaRiferimentoSmistamento}
	c := proponiC(t, []ancoraggio.ProdottoRichiesto{candidato}, ctx)[0]
	b := proponiC(t, []ancoraggio.ProdottoRichiesto{bom}, ctx)[0]
	if c.Stato != ancoraggio.StatoStrutturaCandidata || b.Stato != ancoraggio.StatoBOMDiLavoroProposta {
		t.Fatalf("stati %s e %s", c.Stato, b.Stato)
	}
	parziali := 0
	for i := range b.Nodi {
		if canonico(t, b.Nodi[i].Codice) != canonico(t, c.Nodi[i].Codice) {
			t.Errorf("nodo %s: la catena della BOM di lavoro è diversa da quella della struttura candidata", b.Nodi[i].Rif)
		}
		if b.Nodi[i].Codice.Identita.Parziale {
			parziali++
		}
	}
	if parziali != 2 {
		t.Errorf("identità parziali sulla BOM di lavoro: %d, attese 2 (la radice e il figlio 7120101A)", parziali)
	}
}

// ---- il determinismo ----

// TestCatenaDeterministica (par.3.4.2): gli stessi ingressi in un altro ordine (strutture, componenti, righe aperte e
// decise, codici dei messaggi, decisioni) danno gli stessi byte canonici; due chiamate danno lo stesso esito.
func TestCatenaDeterministica(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "7120100A_2.stp")
	shaB := shaS("9")
	b := stepR(t, m, uidS(0x603), "7120101A_4.stp", shaB, []nodoR{{"#1", "7120101A", "SUPPORTO", "4"}}, nil)
	rev := "4"
	il := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	ctx := ancoraggio.ContestoStrutturale{
		Strutture:  []ancoraggio.StrutturaFile{s, b},
		Confermato: []ancoraggio.ComponenteDeciso{componenteC(t, m, idC1, "7120100A", nil), componenteC(t, m, idC2, "7120101A", &rev), componenteC(t, m, idC3, "7120102A", nil)},
		Proposto:   []ancoraggio.RigaPropostaLegacy{rigaAperta(uidS(0x681), idFileC, shaFileC, "#4")},
		Decise: []ancoraggio.RigaDecisaLegacy{
			rigaDecisa(uidS(0x682), idFileC, shaFileC, "#1", ancoraggio.StatoRigaConfermata, uuidP(idC1), uuidP(idUtente)),
			rigaDecisa(uidS(0x683), idFileC, shaFileC, "#2", ancoraggio.StatoRigaConfermata, uuidP(idC2), uuidP(idUtente)),
			rigaDecisa(uidS(0x684), uidS(0x603), shaB, "#1", ancoraggio.StatoRigaDuplicato, uuidP(idC2), nil),
		},
		CodiciProposti: codiciProposti(m, s, b),
		CodiciMessaggi: append(codiciDelMessaggio(t, m, idMailC, "Il 7120100A3 e il 7120101A1."), codiciDelMessaggio(t, m, uidS(0x622), "Ancora il 7120101A2.")...),
		DecisioniIdentita: []ancoraggio.DecisioneIdentita{
			{Oggetto: ancoraggio.OggettoDecisioneComponente, ID: idC3, Codice: "7120102A", Revisione: "3", Da: idUtente, Il: il},
			{Oggetto: ancoraggio.OggettoDecisioneDocumento, ID: uidS(0x685), Codice: "7120102A", Revisione: "3", Da: idUtente, Il: il},
		},
	}
	target := []ancoraggio.ProdottoRichiesto{
		targetC(t, m, ancoraggio.RifComponente(idC1), ancoraggio.AutoritaConfermata, "7120100A2", uuidP(idC1)),
		targetC(t, m, "identificativo:7120101A", ancoraggio.AutoritaConfermata, "7120101A", nil),
	}
	rov := ctx
	rov.Strutture, rov.Confermato, rov.Proposto, rov.Decise = rovescia(ctx.Strutture), rovescia(ctx.Confermato), rovescia(ctx.Proposto), rovescia(ctx.Decise)
	rov.CodiciMessaggi, rov.DecisioniIdentita = rovescia(ctx.CodiciMessaggi), rovescia(ctx.DecisioniIdentita)
	a := proponiC(t, target, ctx)
	z := proponiC(t, rovescia(target), rov)
	if canonico(t, a) != canonico(t, z) {
		t.Errorf("la catena dipende dall'ordine degli ingressi:\n%s\n%s", canonico(t, a), canonico(t, z))
	}
	if canonico(t, a) != canonico(t, proponiC(t, target, ctx)) {
		t.Error("due chiamate con gli stessi ingressi danno esiti diversi")
	}
	candidati := 0
	for _, x := range a {
		for _, n := range x.Nodi {
			candidati += len(n.Codice.Identita.CandidatiRevisione)
		}
	}
	if candidati < 4 {
		t.Fatalf("la prova del determinismo ha bisogno di candidati: %d", candidati)
	}
}

// ---- gli errori di contratto ----

// TestCatenaErroriDiContratto: gli ingressi nuovi della catena che violano il contratto sono un errore di contratto,
// con il codice della foglia e il percorso, mai un esito.
func TestCatenaErroriDiContratto(t *testing.T) {
	m := motoreCatena(t)
	s := scenaC(t, m, "telaio.stp")
	tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	il := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	ok := ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}}
	if _, _, err := ancoraggio.ProponiStrutture([]ancoraggio.ProdottoRichiesto{tg}, ok); err != nil {
		t.Fatalf("l'ingresso di base non è valido: %v", err)
	}
	con := func(f func(*ancoraggio.ContestoStrutturale)) ancoraggio.ContestoStrutturale {
		x := ok
		f(&x)
		return x
	}
	decisa := rigaDecisa(uidS(0x691), idFileC, shaFileC, "#3", ancoraggio.StatoRigaConfermata, uuidP(idC3), uuidP(idUtente))
	cambia := func(f func(*ancoraggio.RigaDecisaLegacy)) []ancoraggio.RigaDecisaLegacy {
		r := decisa
		f(&r)
		return []ancoraggio.RigaDecisaLegacy{r}
	}
	rev := "1"
	decisione := ancoraggio.DecisioneIdentita{Oggetto: ancoraggio.OggettoDecisioneComponente, ID: idC3, Codice: "7120102A", Revisione: "1", Da: idUtente, Il: il}
	conDecisione := func(f func(*ancoraggio.DecisioneIdentita)) []ancoraggio.DecisioneIdentita {
		d := decisione
		f(&d)
		return []ancoraggio.DecisioneIdentita{d}
	}
	msg := codiciDelMessaggio(t, m, idMailC, "Il 7120100A3.")
	for _, k := range []struct {
		nome, codice, percorso string
		ctx                    ancoraggio.ContestoStrutturale
	}{
		{"revisione manuale senza codice manuale", evidenze.CodiceDocumentoRiferimentoPendente, "contesto.proposto[0].codice_manuale",
			con(func(x *ancoraggio.ContestoStrutturale) {
				r := rigaAperta(uidS(0x692), idFileC, shaFileC, "#2")
				r.RevManuale = &rev
				x.Proposto = []ancoraggio.RigaPropostaLegacy{r}
			})},
		{"riga decisa ripetuta", evidenze.CodiceDocumentoIDRipetuto, "contesto.decise[1].id",
			con(func(x *ancoraggio.ContestoStrutturale) { x.Decise = []ancoraggio.RigaDecisaLegacy{decisa, decisa} })},
		{"riga decisa con l'ID di una aperta", evidenze.CodiceDocumentoIDRipetuto, "contesto.decise[0].id",
			con(func(x *ancoraggio.ContestoStrutturale) {
				x.Proposto = []ancoraggio.RigaPropostaLegacy{rigaAperta(decisa.ID, idFileC, shaFileC, "#2")}
				x.Decise = []ancoraggio.RigaDecisaLegacy{decisa}
			})},
		{"riga aperta e decisa per lo stesso nodo", evidenze.CodiceDocumentoIDRipetuto, "contesto.decise[0].chiave",
			con(func(x *ancoraggio.ContestoStrutturale) {
				x.Proposto = []ancoraggio.RigaPropostaLegacy{rigaAperta(uidS(0x693), idFileC, shaFileC, "#3")}
				x.Decise = []ancoraggio.RigaDecisaLegacy{decisa}
			})},
		{"riga decisa senza chiave", evidenze.CodiceDocumentoRiferimentoPendente, "contesto.decise[0].chiave",
			con(func(x *ancoraggio.ContestoStrutturale) {
				x.Decise = cambia(func(r *ancoraggio.RigaDecisaLegacy) { r.Chiave = "" })
			})},
		{"riga decisa senza sha256", evidenze.CodiceDocumentoRiferimentoPendente, "contesto.decise[0].sha256",
			con(func(x *ancoraggio.ContestoStrutturale) {
				x.Decise = cambia(func(r *ancoraggio.RigaDecisaLegacy) { r.Sha256 = "" })
			})},
		{"riga decisa aperta", evidenze.CodiceDocumentoEnumIgnoto, "contesto.decise[0].stato",
			con(func(x *ancoraggio.ContestoStrutturale) {
				x.Decise = cambia(func(r *ancoraggio.RigaDecisaLegacy) { r.Stato = "aperta" })
			})},
		{"riga confermata senza componente", evidenze.CodiceDocumentoRiferimentoPendente, "contesto.decise[0].componente_id",
			con(func(x *ancoraggio.ContestoStrutturale) {
				x.Decise = cambia(func(r *ancoraggio.RigaDecisaLegacy) { r.ComponenteID = nil })
			})},
		{"riga decisa di un altro contenuto", evidenze.CodiceDocumentoRiferimentoPendente, "contesto.decise[0].sha256",
			con(func(x *ancoraggio.ContestoStrutturale) {
				x.Decise = cambia(func(r *ancoraggio.RigaDecisaLegacy) { r.Sha256 = shaS("e") })
			})},
		{"codice di un messaggio senza messaggio", evidenze.CodiceDocumentoRiferimentoPendente, "contesto.codici_messaggi[0].messaggio_id",
			con(func(x *ancoraggio.ContestoStrutturale) {
				c := msg[0]
				c.MessaggioID = uuid.Nil
				x.CodiciMessaggi = []ancoraggio.CodiceDiMessaggio{c}
			})},
		{"codice di un messaggio ripetuto", evidenze.CodiceDocumentoIDRipetuto, "contesto.codici_messaggi[1].lettura",
			con(func(x *ancoraggio.ContestoStrutturale) {
				x.CodiciMessaggi = []ancoraggio.CodiceDiMessaggio{msg[0], msg[0]}
			})},
		{"decisione con un oggetto ignoto", evidenze.CodiceDocumentoEnumIgnoto, "contesto.decisioni_identita[0].oggetto",
			con(func(x *ancoraggio.ContestoStrutturale) {
				x.DecisioniIdentita = conDecisione(func(d *ancoraggio.DecisioneIdentita) { d.Oggetto = "nodo" })
			})},
		{"decisione senza oggetto", evidenze.CodiceDocumentoRiferimentoPendente, "contesto.decisioni_identita[0].id",
			con(func(x *ancoraggio.ContestoStrutturale) {
				x.DecisioniIdentita = conDecisione(func(d *ancoraggio.DecisioneIdentita) { d.ID = uuid.Nil })
			})},
		{"decisione senza chi", evidenze.CodiceDocumentoRiferimentoPendente, "contesto.decisioni_identita[0].da",
			con(func(x *ancoraggio.ContestoStrutturale) {
				x.DecisioniIdentita = conDecisione(func(d *ancoraggio.DecisioneIdentita) { d.Da = uuid.Nil })
			})},
		{"decisione senza quando", evidenze.CodiceDocumentoRiferimentoPendente, "contesto.decisioni_identita[0].da",
			con(func(x *ancoraggio.ContestoStrutturale) {
				x.DecisioniIdentita = conDecisione(func(d *ancoraggio.DecisioneIdentita) { d.Il = time.Time{} })
			})},
		{"due decisioni sullo stesso componente", evidenze.CodiceDocumentoIDRipetuto, "contesto.decisioni_identita[1].id",
			con(func(x *ancoraggio.ContestoStrutturale) {
				x.DecisioniIdentita = []ancoraggio.DecisioneIdentita{decisione, decisione}
			})},
		{"decisione senza codice (T-B4-21)", evidenze.CodiceDocumentoRiferimentoPendente, "contesto.decisioni_identita[0].codice",
			con(func(x *ancoraggio.ContestoStrutturale) {
				x.DecisioniIdentita = conDecisione(func(d *ancoraggio.DecisioneIdentita) { d.Codice, d.Revisione = " ", "" })
			})},
		{"evidenza vista di una fonte fuori elenco", evidenze.CodiceDocumentoEnumIgnoto, "contesto.decisioni_identita[0].evidenze_viste[1].fonte",
			con(func(x *ancoraggio.ContestoStrutturale) {
				x.DecisioniIdentita = conDecisione(func(d *ancoraggio.DecisioneIdentita) {
					d.EvidenzeViste = []ancoraggio.EvidenzaVista{ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaCartiglio, "7120102A1"), {Fonte: "agente", Valore: "7120102A1"}}
				})
			})},
	} {
		t.Run(k.nome, func(t *testing.T) {
			s, d, err := ancoraggio.ProponiStrutture([]ancoraggio.ProdottoRichiesto{tg}, k.ctx)
			var ec *evidenze.ErroreContratto
			if !errors.As(err, &ec) {
				t.Fatalf("errore %v, atteso un errore di contratto", err)
			}
			trovato := false
			for _, x := range ec.Diagnostiche {
				trovato = trovato || (x.Codice == k.codice && x.Percorso == k.percorso)
				if x.Gravita != evidenze.GravitaErrore || x.Natura != evidenze.NaturaContratto {
					t.Errorf("diagnostica di contratto: %+v", x)
				}
			}
			if !trovato {
				t.Errorf("nessuna diagnostica %s sul percorso %s: %+v", k.codice, k.percorso, ec.Diagnostiche)
			}
			if s != nil || d != nil {
				t.Errorf("con l'errore un esito: %+v %+v", s, d)
			}
		})
	}
}

// ---- i valori del contratto ----

// TestIValoriDellaCatena (contratto §2.2, §2.5, §2.6): i valori delle costanti, la chiave dei codici proposti e i campi
// dei tipi della catena, delle decisioni e degli ingressi, con i tag JSON snake_case. Se uno cambia, questa prova si
// riscrive con «Riscritta per …».
func TestIValoriDellaCatena(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{ancoraggio.ProvenienzaNodoSTEP, "nodo_step"},
		{ancoraggio.ProvenienzaRadiceSTEP, "radice_step"},
		{ancoraggio.StatoRevisioneAssente, "assente"},
		{ancoraggio.StatoRevisioneCandidata, "candidata"},
		{ancoraggio.StatoRevisioneConfermata, "confermata"},
		{ancoraggio.FonteRevisioneNomeFileSTEP, "nome_file_step"},
		{ancoraggio.FonteRevisioneCartiglio, "cartiglio"},
		{ancoraggio.FonteRevisioneCodiceTarget, "codice_target"},
		{ancoraggio.FonteRevisioneMessaggio, "messaggio"},
		{ancoraggio.RevProvenienzaNonRegistrata, "non_registrata"},
		{ancoraggio.RevProvenienzaFormazioneSTEP, "coincide_con_formazione_step"},
		{ancoraggio.RevProvenienzaDecisioneTracciata, "decisione_tracciata"},
		{ancoraggio.OggettoDecisioneComponente, "componente"},
		{ancoraggio.OggettoDecisioneDocumento, "documento"},
		{ancoraggio.FonteEvidenzaCartiglio, "cartiglio"},
		{ancoraggio.FonteEvidenzaStepEntita, "step_entita"},
		{ancoraggio.FonteEvidenzaNomeFile, "nome_file"},
		{ancoraggio.FonteEvidenzaDocumento, "documento"},
		{string(ancoraggio.RiconciliazioneConcorda), "concorda"},
		{string(ancoraggio.RiconciliazioneCompletamentoProposto), "completamento_proposto"},
		{string(ancoraggio.RiconciliazioneCorrezioneProposta), "correzione_proposta"},
		{string(ancoraggio.RiconciliazioneDiscordante), "discordante"},
		{string(ancoraggio.RiconciliazioneNonVerificabile), "non_verificabile"},
		{ancoraggio.CampoGrezzoID, "id"},
		{ancoraggio.CampoGrezzoNome, "nome"},
		{ancoraggio.StatoRigaConfermata, "confermata"},
		{ancoraggio.StatoRigaDuplicato, "duplicato"},
		{ancoraggio.StatoRigaScartata, "scartata"},
		// le letture dell'orchestratore T-B4-01…T-B4-03
		{ancoraggio.MotivoLetturaAssente, "nessuna_lettura"},
		{ancoraggio.MotivoLettureDiscordanti, "letture_discordanti"},
		{ancoraggio.MotivoPropostoNonComposto, "non_composto"},
		{ancoraggio.MotivoSenzaRevisioneNonLetta, "revisione_non_letta"},
		{ancoraggio.MotivoSenzaRevisioneAmbigua, "revisione_ambigua"},
		{ancoraggio.MotivoSenzaRevisioneMarcatoreDiverso, "marcatore_diverso"},
		{ancoraggio.MotivoSenzaRevisioneNomeNonLetto, "nome_non_letto"},
		{ancoraggio.MotivoSenzaRevisioneNomeDiscorde, "nome_discordante"},
		{ancoraggio.ChiaveCodiceProposto(shaS("a"), "l:u:step:#1:id:f/x:0-8"), shaS("a") + ":l:u:step:#1:id:f/x:0-8"},
		{ancoraggio.EvidenzaDa(ancoraggio.FonteEvidenzaDocumento, " 7120102A ", "1").Valore, "7120102A 1"},
	} {
		if c.got != c.want {
			t.Errorf("valore %q, il contratto dice %q", c.got, c.want)
		}
	}
	for _, c := range []struct {
		tipo  any
		campi string
	}{
		{ancoraggio.CatenaCodice{}, "Grezzo:grezzo Lettura:lettura Forma:forma MotivoLettura:motivo_lettura Identita:identita Proposto:proposto " +
			"MotivoProposto:motivo_proposto Manuale:manuale Confermato:confermato Documentale:documentale"},
		{ancoraggio.ValoreGrezzo{}, "Testo:testo Campo:campo UnitaID:unita_id Posizione:posizione"},
		{ancoraggio.IdentitaNodo{}, "Base:base Marcatore:marcatore Revisione:revisione Parziale:parziale Provenienza:provenienza Qualita:qualita Motivo:motivo " +
			"CandidatiRevisione:candidati_revisione FontiSenzaRevisione:fonti_senza_revisione StatoRevisione:stato_revisione"},
		{ancoraggio.CandidatoRevisione{}, "Valore:valore Fonte:fonte AllegatoID:allegato_id MessaggioID:messaggio_id Posizione:posizione Entita:entita"},
		{ancoraggio.FonteSenzaRevisione{}, "Fonte:fonte Entita:entita AllegatoID:allegato_id MessaggioID:messaggio_id Motivo:motivo"},
		{ancoraggio.CodiceDeciso{}, "ComponenteID:componente_id Codice:codice Rev:rev Base:base Origine:origine RevProvenienza:rev_provenienza"},
		// riscritta per la fase 3: Associazione e Discordanze (T-B4-34, T-B4-39, T-B4-40)
		{ancoraggio.CodiceDocumentale{}, "AllegatoID:allegato_id DocumentoID:documento_id OrigineAssociazione:origine_associazione Associazione:associazione " +
			"Lettura:lettura Originale:originale Base:base Revisione:revisione UnitaID:unita_id Posizione:posizione Esito:esito Correzione:correzione Motivo:motivo " +
			"Discordanze:discordanze"},
		{ancoraggio.CorrezioneProposta{}, "Codice:codice Base:base Revisione:revisione AllegatoID:allegato_id UnitaID:unita_id Posizione:posizione " +
			"RevisioneInferiore:revisione_inferiore"},
		{ancoraggio.DecisioneIdentita{}, "Oggetto:oggetto ID:id Codice:codice Revisione:revisione EvidenzeViste:evidenze_viste Da:da Il:il"},
		{ancoraggio.EvidenzaVista{}, "Fonte:fonte Valore:valore"},
		{ancoraggio.RigaDecisaLegacy{}, "ID:id AllegatoID:allegato_id Sha256:sha256 Chiave:chiave Stato:stato ComponenteID:componente_id DecisoDa:deciso_da"},
		{ancoraggio.LetturaConPosizione{}, "Lettura:lettura Posizione:posizione"},
		{ancoraggio.CodiceDiMessaggio{}, "MessaggioID:messaggio_id Lettura:lettura Posizione:posizione"},
	} {
		tp := reflect.TypeOf(c.tipo)
		var campi []string
		for i := 0; i < tp.NumField(); i++ {
			f := tp.Field(i)
			tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			campi = append(campi, f.Name+":"+tag)
		}
		if got := strings.Join(campi, " "); got != c.campi {
			t.Errorf("%s: campi %s, il contratto dice %s", tp.Name(), got, c.campi)
		}
	}
}

// ---- i ritocchi finali di B4: le regole che le prove non fissavano ----

// TestLettureCompatibiliDelloStessoNodo (T-B4-01 con la regola unica del marcatore, T-B4-06 rivisto, T-B4-30): due
// letture dello stesso nodo con la stessa base, una con «A» e una senza marcatore, non sono discordanti: vale la
// precedenza di sempre (il campo id, poi l'ID minore), anche quando la lettura scelta è quella senza marcatore. Con una
// terza lettura «B» le letture sono discordanti, perché «A» e «B» lo sono, anche quando la prima è quella senza
// marcatore, compatibile con tutte e due: il confronto è a due a due.
func TestLettureCompatibiliDelloStessoNodo(t *testing.T) {
	m := motoreCatena(t)
	tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	marcatore := func(mk string) *motorea.ParteLetta {
		if mk == "" {
			return nil
		}
		return &motorea.ParteLetta{Valore: mk, Originale: mk}
	}
	// foto: il nodo 7120101A con la sua lettura (il marcatore dato) e le altre letture costruite a mano sulla stessa
	// occorrenza, con i marcatori dati, dopo di lei nell'ordine degli ID.
	foto := func(primo string, altri ...string) (ancoraggio.CatenaCodice, string) {
		t.Helper()
		s := scenaC(t, m, "telaio.stp")
		var originale string
		for i := range s.Nodi {
			if s.Nodi[i].Chiave != "#2" || len(s.Nodi[i].Letture) != 1 {
				continue
			}
			base := s.Nodi[i].Letture[0]
			originale = base.ID
			base.Forma.Marcatore = marcatore(primo)
			letture := []motorea.LetturaCodice{base}
			for j, mk := range altri {
				l := base
				l.ID = base.ID + ":" + strconv.Itoa(j)
				l.Forma.Marcatore = marcatore(mk)
				letture = append(letture, l)
			}
			s.Nodi[i].Letture = letture
		}
		x := proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s}, CodiciProposti: codiciProposti(m, s)})[0]
		return nodoC(t, x, rC("#2")).Codice, originale
	}
	for _, c := range []struct {
		primo       string
		altri       []string
		discordanti bool
		marcatore   string
	}{
		{"A", []string{""}, false, "A"},
		{"", []string{"A"}, false, ""},
		{"A", []string{"", "B"}, true, ""},
		{"", []string{"A", "B"}, true, ""},
	} {
		cat, originale := foto(c.primo, c.altri...)
		if c.discordanti {
			if cat.Lettura != "" || cat.MotivoLettura != ancoraggio.MotivoLettureDiscordanti {
				t.Errorf("%q e %v: discordanti, avuto %q %q", c.primo, c.altri, cat.Lettura, cat.MotivoLettura)
			}
			continue
		}
		if cat.Lettura != originale || cat.MotivoLettura != "" || cat.Identita.Base != "7120101" || cat.Identita.Marcatore != c.marcatore {
			t.Errorf("%q e %v: compatibili, con la precedenza dell'ID minore: %q %q %+v", c.primo, c.altri, cat.Lettura, cat.MotivoLettura, cat.Identita)
		}
	}
}

// TestLeRegoleDegliIndiziCheLeProveNonFissavano (T-E1-06, T-B4-06 rivisto, T-B0-27, D5): una prova per regola.
//   - Un codice di un messaggio letto in un altro spazio di codici, con la stessa base, non è un indizio dell'entità
//     (il namespace della regola unica).
//   - Una revisione sospesa (D5) nella lettura del nodo dà il motivo revisione_ambigua, senza revisione.
//   - Il nome del file STEP letto solo in un altro spazio di codici è «non letto», non «discordante» (T-B0-35).
//   - Lo stesso candidato due volte (due letture della stessa occorrenza con la stessa revisione) è un candidato solo.
func TestLeRegoleDegliIndiziCheLeProveNonFissavano(t *testing.T) {
	m := motoreCatena(t)
	tg := targetC(t, m, "identificativo:7120100A", ancoraggio.AutoritaConfermata, "7120100A", nil)
	proponi := func(s ancoraggio.StrutturaFile, messaggi []ancoraggio.CodiceDiMessaggio) ancoraggio.StrutturaProdotto {
		t.Helper()
		return proponiC(t, []ancoraggio.ProdottoRichiesto{tg}, ancoraggio.ContestoStrutturale{Strutture: []ancoraggio.StrutturaFile{s},
			CodiciProposti: codiciProposti(m, s), CodiciMessaggi: messaggi})[0]
	}
	delMessaggio := func(n ancoraggio.NodoProposto) (cand, senza int) {
		for _, c := range n.Codice.Identita.CandidatiRevisione {
			if c.Fonte == ancoraggio.FonteRevisioneMessaggio {
				cand++
			}
		}
		for _, f := range n.Codice.Identita.FontiSenzaRevisione {
			if f.Fonte == ancoraggio.FonteRevisioneMessaggio {
				senza++
			}
		}
		return cand, senza
	}
	messaggio := func() []ancoraggio.CodiceDiMessaggio {
		var out []ancoraggio.CodiceDiMessaggio
		for _, c := range codiciDelMessaggio(t, m, idMailC, "Vi mandiamo il supporto 7120101A2 aggiornato.") {
			if c.Lettura.Forma.Base.Normalizzata == "7120101" {
				out = append(out, c)
			}
		}
		if len(out) == 0 {
			t.Fatal("il messaggio non legge 7120101A2")
		}
		return out
	}

	t.Run("un messaggio in un altro spazio di codici", func(t *testing.T) {
		s := scenaC(t, m, "telaio.stp")
		if c, _ := delMessaggio(nodoC(t, proponi(s, messaggio()), rC("#2"))); c != 1 {
			t.Fatalf("il messaggio nello spazio del nodo è un indizio: %d", c)
		}
		altro := messaggio()
		for i := range altro {
			altro[i].Lettura.Forma.Namespace = "acme-altro" // costruita a mano: la stessa base in un altro spazio di codici
		}
		if c, f := delMessaggio(nodoC(t, proponi(s, altro), rC("#2"))); c != 0 || f != 0 {
			t.Errorf("un altro spazio di codici non è la stessa identità: %d candidati, %d fonti", c, f)
		}
	})

	t.Run("lo stesso candidato due volte è uno", func(t *testing.T) {
		s := scenaC(t, m, "telaio.stp")
		doppio := messaggio()
		copia := doppio[0]
		copia.Lettura.ID += ":bis" // costruita a mano: un'altra famiglia che legge la stessa occorrenza
		doppio = append(doppio, copia)
		if c, _ := delMessaggio(nodoC(t, proponi(s, doppio), rC("#2"))); c != 1 {
			t.Errorf("due letture della stessa occorrenza con la stessa revisione: %d candidati, atteso uno", c)
		}
	})

	t.Run("una revisione sospesa nel nodo", func(t *testing.T) {
		s := scenaC(t, m, "telaio.stp")
		for i := range s.Nodi {
			if s.Nodi[i].Chiave == "#3" {
				s.Nodi[i].Letture = append([]motorea.LetturaCodice(nil), s.Nodi[i].Letture...)
				f := &s.Nodi[i].Letture[0].Forma // costruita a mano: un token sospeso al posto della revisione (D5)
				r := *f.Revisione
				r.Stato, r.Normalizzata, r.Segmenti = motorea.StatoRevisioneNonInterpretabile, "", nil
				f.Revisione, f.Stato = &r, motorea.StatoDaVerificare
			}
		}
		id := nodoC(t, proponi(s, nil), rC("#3")).Codice.Identita
		if id.Motivo != motorea.MotivoComposizioneRevisioneAmbigua || id.Revisione != nil {
			t.Errorf("la revisione sospesa: %+v", id)
		}
	})

	t.Run("il nome del file letto solo in un altro spazio di codici", func(t *testing.T) {
		s := scenaC(t, m, "7120100A_2.stp")
		s.NomeFile = append([]ancoraggio.LetturaConPosizione(nil), s.NomeFile...)
		for i := range s.NomeFile {
			s.NomeFile[i].Lettura.Forma.Namespace = "acme-altro" // costruita a mano
		}
		radice := nodoC(t, proponi(s, nil), rC("#1")).Codice.Identita
		var motivi []string
		for _, f := range radice.FontiSenzaRevisione {
			if f.Fonte == ancoraggio.FonteRevisioneNomeFileSTEP {
				motivi = append(motivi, f.Motivo)
			}
		}
		if len(motivi) != 1 || motivi[0] != ancoraggio.MotivoSenzaRevisioneNomeNonLetto {
			t.Errorf("il nome letto in un altro spazio di codici: %v, atteso nome_non_letto", motivi)
		}
	})
}
