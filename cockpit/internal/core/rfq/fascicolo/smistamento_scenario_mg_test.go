package fascicolo

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/contratti/worker"
)

// Lo scenario «MG» nel flusso ancorato (F8), prova da fare del giro 4: la stessa RFQ di
// classificazione.TestScenarioMG (docs/domande/scenario_MG_28-09.md, fuori da git), con dati fittizi. Quattro
// prodotti chiesti nella tabella della mail CON la P (P7120100, P7120103, P7120101, P7120104), confermati al
// triage; cinque STEP d'assieme con la radice SENZA la P (7120100 … 7120104, piu' 7120102 che la mail non
// chiede); figli in comune fra due assiemi; un PDF e uno STEP per ogni pezzo, un DXF per le lamiere; tutti i
// nomi con il prefisso di progetto attaccato («ACME-030P<codice>…»). Nessun pezzo e' ancora deciso da una
// persona: quello che si vede e' cio' che il flusso propone quando la mail arriva.
//
// Risultati attesi (decisioni del 29/09: il cartiglio fa fede, i nomi e le mail sono alias; 27/09 «bis»: nessuna
// identita' senza una persona): ogni file ha per destinazione il SUO pezzo, trovato dal cartiglio o dalla radice
// dello STEP, attraverso l'alias P7120100 = 7120100; un figlio in comune e' una destinazione con due posizioni;
// niente si preseleziona su un pezzo sbagliato. Oggi quasi niente di questo succede: i «da fare» lo elencano.

// motoreMGFlusso e' la famiglia ACME con la forma di quella del cliente vero (vedi classificazione.motoreMG).
func motoreMGFlusso(t *testing.T) *classificazione.Motore {
	t.Helper()
	m := classificazione.Compila("ACME", regole.Regole{FamiglieCodice: []regole.FamigliaCodice{{
		Regex:        `(?i)(?:^|[^0-9A-Za-z])(?P<codice>P?712[0-9]{4})(?:-(?P<rev>rev\d{2}))?(?:[^0-9A-Za-z]|$)`,
		Descrizione:  "codici ACME: 712 + 4 cifre, eventuale P davanti, revisione -revNN nei file",
		Esempio:      "7120001-rev01",
		RevNelCodice: true,
	}}})
	if !m.HaFamiglie() {
		t.Fatal("la famiglia di prova dello scenario MG non e' entrata nel motore")
	}
	return m
}

// daFareMG: vedi classificazione.daFare.
func daFareMG(t *testing.T, soddisfatta bool, cosa, oggi string) {
	t.Helper()
	if soddisfatta {
		t.Logf("soddisfatta: %s — togliere daFareMG e lasciare l'asserzione", cosa)
		return
	}
	t.Skipf("da fare giro 4: %s (oggi: %s)", cosa, oggi)
}

// assiemeMG e' un disegno d'assieme dello scenario: il codice del cartiglio, lo speculare, i figli con la qta.
type assiemeMG struct {
	codice, speculare string
	figli             []string // "7120110" o "7120133*2"
}

var assiemiMG = []assiemeMG{
	{"7120100", "7120101", []string{"7120110", "7120111", "7120112", "7120113", "7120130", "7120131", "7120114", "7120115", "7120116", "7120117"}},
	{"7120101", "7120100", []string{"7120111", "7120112", "7120113", "7120130", "7120131", "7120114", "7120115", "7120116", "7120117", "7120118"}},
	{"7120102", "", []string{"7120121", "7120120", "7120135"}},
	{"7120103", "7120104", []string{"7120122", "7120132", "7120133*2", "7120134"}},
	{"7120104", "7120103", []string{"7120122", "7120132", "7120133*2", "7120134"}},
}

// particolariMG: i pezzi senza elenco con i loro file (STEP e PDF; DXF per le lamiere) e, se c'e', lo
// specchiato scritto sopra il cartiglio.
var particolariMG = []struct {
	codice, specchiato string
	dxf                bool
}{
	{"7120105", "", false}, {"7120106", "", true},
	{"7120110", "7120118", true}, {"7120111", "", true}, {"7120112", "", false}, {"7120113", "", true}, {"7120114", "", true},
	{"7120115", "", true}, {"7120116", "", true}, {"7120117", "", true}, {"7120120", "", true}, {"7120121", "", true},
	{"7120122", "", false}, {"7120118", "7120110", true},
}

// testoDisegnoMG e' il testo di un disegno del cliente come lo riporta il worker: nella zona in basso a destra,
// nell'ordine del file, l'elenco particolari (se c'e'), «SPECULARE/SPECCHIATO DI», la tabella delle revisioni e
// il cartiglio con il codice sotto «Part Nr:». Il campo «codice» del worker e' la prima riga dell'elenco (vedi
// classificazione.testoAssiemeMG).
func testoDisegnoMG(codice, speculare string, figli []string) json.RawMessage {
	var b strings.Builder
	if len(figli) > 0 {
		b.WriteString("Pos. Part Number Descrizione Q.ty\n")
		for i, f := range figli {
			c, q, _ := strings.Cut(f, "*")
			if q == "" {
				q = "1"
			}
			fmt.Fprintf(&b, "%d %s PEZZO %s\n", i+1, c, q)
		}
	}
	if speculare != "" {
		b.WriteString("SPECULARE DI " + speculare + "\n")
	}
	b.WriteString("Rev Mod. N. Type Modification object / Descrizione modifica\n00 ACME-P003 A Prima emissione\n")
	b.WriteString("Family: Part Nr: Descrizione: Description:\n-- " + codice + " PEZZO ACME\n")
	bd := []float64{560, 400, 830, 590}
	tp := worker.TestoPDF{Versione: 1, Estraibile: true, Pagine: 1, PagineLette: 1, FormatoPagina1: []float64{842, 595},
		Caratteri: b.Len(), OCR: worker.OCRPDF{Stato: worker.OCRNonNecessario},
		Frammenti: []worker.FrammentoPDF{{Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Testo: b.String(), Riquadro: bd}}}
	if len(figli) > 0 {
		c, _, _ := strings.Cut(figli[0], "*")
		tp.Cartiglio = append(tp.Cartiglio, worker.CampoCartiglio{Etichetta: worker.CampoCodice, Letta: "Part Number", Valore: c,
			Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Riquadro: bd})
	}
	out, _ := json.Marshal(map[string]any{"cartiglio": true, "termini_trovati": []string{"SCALA"},
		"fonti": map[string]string{"tipo": "termini_pdf"}, "testo_pdf": tp})
	return out
}

func nomeSTEPMG(c string) string { return "ACME-030P" + c + " 00 IN_WORK.stp" }
func nomePDFMG(c string) string  { return "ACME-030P" + c + ".pdf" }
func nomeDXFMG(c string) string  { return "ACME-030P" + c + ".dxf" }

// scenaMG costruisce la RFQ dello scenario com'e' quando la mail arriva: i quattro prodotti della tabella
// confermati al triage (con la P), e i 49 file con le loro letture. Nel ramo delle prove (fase 4.3, sopra la 4.2)
// il testo dei PDF arriva al flusso come evidenze normalizzate (disegnoLetto), non piu' dalla valutazione.
func scenaMG(t *testing.T) *scena {
	return scenaMGCon(t, "P7120100", "P7120103", "P7120101", "P7120104")
}

// scenaMGCon e' lo scenario con i prodotti dati: quelli della mail (con la P), o quelli che l'operatore puo'
// scrivere oggi al triage come dice il cartiglio (senza la P).
func scenaMGCon(t *testing.T, prodotti ...string) *scena {
	sc := nuovaScena(t)
	sc.s.Motore = motoreMGFlusso(t)
	for _, p := range prodotti {
		sc.prodotto(p)
	}
	for _, a := range assiemiMG {
		nodi := []string{"#1=" + a.codice}
		var archi []string
		for i, f := range a.figli {
			c, q, _ := strings.Cut(f, "*")
			k := fmt.Sprintf("#%d", i+2)
			nodi = append(nodi, k+"="+c)
			arco := "#1>" + k
			if q != "" {
				arco += "*" + q
			}
			archi = append(archi, arco)
		}
		sc.step(nomeSTEPMG(a.codice), nodi, archi)
		// il testo del PDF entra nel flusso da una porta sola, le letture normalizzate (giro 4, fase 4.2):
		// disegnoLetto le da' al file come LeggiStatoFlusso (EvidenzeContenutoPDF)
		sc.disegnoLetto(nomePDFMG(a.codice), testoDisegnoMG(a.codice, a.speculare, a.figli))
	}
	for _, p := range particolariMG {
		sc.step(nomeSTEPMG(p.codice), []string{"#1=" + p.codice}, nil)
		if p.dxf {
			sc.dalFormato(nomeDXFMG(p.codice))
		}
		nome := nomePDFMG(p.codice)
		testo := testoDisegnoMG(p.codice, "", nil)
		if p.specchiato != "" {
			var f map[string]any
			_ = json.Unmarshal(testo, &f)
			tp := f["testo_pdf"].(map[string]any)
			fr := tp["frammenti"].([]any)[0].(map[string]any)
			fr["testo"] = "12.5 SPECCHIATO DI " + p.specchiato + "\n" + fr["testo"].(string)
			testo, _ = json.Marshal(f)
		}
		sc.disegnoLetto(nome, testo)
	}
	return sc
}

// pezzoDelFile e' il pezzo giusto di un file dello scenario: il codice che il suo cartiglio (o la radice del suo
// STEP) dice. Il prodotto chiesto con la P e' lo stesso pezzo (alias).
func pezzoDelFile(nome string) string {
	s := strings.TrimPrefix(nome, "ACME-030P")
	return s[:7]
}

func stessoPezzo(codice, pezzo string) bool {
	return strings.EqualFold(strings.TrimPrefix(strings.ToUpper(codice), "P"), pezzo)
}

func TestScenarioMG(t *testing.T) {
	sc := scenaMG(t)
	if len(sc.s.File) != 49 {
		t.Fatalf("lo scenario ha %d file, non 49", len(sc.s.File))
	}
	c := sc.calcola()

	var righe []string
	for _, f := range sc.s.File {
		d := c.Destinazioni[f.AllegatoID]
		righe = append(righe, fmt.Sprintf("%s → %s [%s] presel=%v disc=%v", f.Nome, candidati(d), d.Stato, d.Preselezionabile, d.Discordanze))
	}
	t.Logf("oggi, le destinazioni:\n%s", strings.Join(righe, "\n"))
	for _, p := range []string{"P7120100", "P7120103", "P7120101", "P7120104"} {
		a := c.Ancore[p]
		t.Logf("oggi, l'ancora di %s: livello %q, %d portatori, motivi %v", p, a.Livello, len(a.Portatori), a.Motivi)
	}

	// Invariante (nessuna identita' senza una persona, U7): nessun file si preseleziona su un pezzo che non e'
	// il suo. Oggi non se ne preseleziona nessuno; domani, con le ancore giuste, solo sul pezzo del cartiglio.
	t.Run("nessun file si preseleziona su un pezzo che non e' il suo", func(t *testing.T) {
		for _, f := range sc.s.File {
			d := c.Destinazioni[f.AllegatoID]
			if d.Preselezionabile && (len(d.Candidati) == 0 || !stessoPezzo(d.Candidati[0].Codice, pezzoDelFile(f.Nome))) {
				t.Errorf("%s preselezionato su %s", f.Nome, candidati(d))
			}
		}
	})

	// Invariante: il PDF d'assieme non va mai preselezionato su un figlio del suo elenco particolari.
	t.Run("il PDF d'assieme non va su un figlio del suo elenco", func(t *testing.T) {
		for _, a := range assiemiMG {
			d := sc.dest(c, nomePDFMG(a.codice))
			if d.Preselezionabile && len(d.Candidati) > 0 && !stessoPezzo(d.Candidati[0].Codice, a.codice) {
				t.Errorf("%s preselezionato su %s", nomePDFMG(a.codice), candidati(d))
			}
		}
	})

	t.Run("da fare: lo STEP dell'assieme ancora il prodotto chiesto con la P", func(t *testing.T) {
		a := c.Ancore["P7120100"]
		daFareMG(t, a.Ancorato() && len(a.Portatori) == 1 && a.Portatori[0].Nome == nomeSTEPMG("7120100"),
			"la radice 7120100 dello STEP e il prodotto P7120100 della mail sono lo stesso pezzo (alias della regola del cliente): ancora piena",
			fmt.Sprintf("livello %q, portatori %d, motivi %v", a.Livello, len(a.Portatori), a.Motivi))
	})

	t.Run("da fare: il PDF d'assieme va al suo assieme", func(t *testing.T) {
		var male []string
		for _, a := range assiemiMG {
			d := sc.dest(c, nomePDFMG(a.codice))
			if len(d.Candidati) == 0 || !stessoPezzo(d.Candidati[0].Codice, a.codice) {
				male = append(male, nomePDFMG(a.codice)+" → "+candidati(d))
			}
		}
		daFareMG(t, len(male) == 0, "il cartiglio («Part Nr») dice l'assieme; l'elenco particolari dice i figli, non la destinazione",
			strings.Join(male, "; "))
	})

	t.Run("da fare: il PDF e il DXF di un particolare vanno al particolare", func(t *testing.T) {
		var male []string
		for _, p := range particolariMG {
			nomi := []string{nomePDFMG(p.codice)}
			if p.dxf {
				nomi = append(nomi, nomeDXFMG(p.codice))
			}
			for _, n := range nomi {
				d := sc.dest(c, n)
				if len(d.Candidati) == 0 || !stessoPezzo(d.Candidati[0].Codice, p.codice) {
					male = append(male, n+" → "+candidati(d))
				}
			}
		}
		daFareMG(t, len(male) == 0,
			"il nome «ACME-030P<codice>» (cartella del PDM + P) e il cartiglio del particolare portano al pezzo; «SPECCHIATO DI» non e' il codice",
			fmt.Sprintf("%d file su %d senza il loro pezzo, per esempio %s", len(male), 14+12, strings.Join(male[:min(3, len(male))], "; ")))
	})

	t.Run("da fare: un figlio in comune fra due assiemi e' una destinazione con due posizioni", func(t *testing.T) {
		d := sc.dest(c, nomePDFMG("7120111"))
		var pos []string
		if len(d.Candidati) > 0 {
			for _, p := range d.Candidati[0].Posizioni {
				pos = append(pos, fmt.Sprintf("%s×%d", p.Padre, p.Qta))
			}
		}
		daFareMG(t, len(d.Candidati) == 1 && d.Candidati[0].Codice == "7120111" && len(pos) == 2,
			"7120111 sta sotto 7120100 e sotto 7120101: un pezzo, una destinazione, due posizioni (come TestLoStessoPezzoSottoDuePadriEUnaDestinazione)",
			fmt.Sprintf("%s, posizioni %v", candidati(d), pos))
	})

	// Invariante: il quinto assieme fra gli allegati, che la tabella della mail non chiede, non si perde in
	// silenzio: la destinazione del suo STEP dice che la sua radice non si raggiunge da nessun prodotto ancorato.
	t.Run("7120102 (allegato, non chiesto nella mail) e' detto, non perso", func(t *testing.T) {
		d := sc.dest(c, nomeSTEPMG("7120102"))
		ev := strings.Join(d.Evidenze, " | ")
		if !strings.Contains(ev, "7120102") || !strings.Contains(ev, "non si raggiunge da nessun prodotto ancorato") {
			t.Errorf("lo STEP di 7120102: %s; evidenze %v", candidati(d), d.Evidenze)
		}
	})
}

// L'invariante del flusso e la costruzione dello scenario non dipendono dall'ordine dei file (FP7): lo stesso
// scenario costruito al contrario ha le stesse destinazioni.
func TestScenarioMGNonDipendeDallOrdine(t *testing.T) {
	a := scenaMG(t)
	b := scenaMG(t)
	for i, j := 0, len(b.s.File)-1; i < j; i, j = i+1, j-1 {
		b.s.File[i], b.s.File[j] = b.s.File[j], b.s.File[i]
	}
	ca, cb := a.calcola(), b.calcola()
	for _, f := range a.s.File {
		da, db_ := ca.Destinazioni[f.AllegatoID], cb.Destinazioni[f.AllegatoID]
		if candidati(da) != candidati(db_) || da.Preselezionabile != db_.Preselezionabile {
			t.Errorf("%s: %s contro %s", f.Nome, candidati(da), candidati(db_))
		}
	}
}

// TestScenarioMGProdottiDalCartiglio: lo stesso scenario con i prodotti scritti come dice il cartiglio (7120100
// …, senza la P), cioe' quello che un operatore puo' fare oggi al triage, e quello che l'alias dara' domani. Le
// ancore ci sono, l'indice si riempie dei figli degli STEP, e il PDF d'assieme porta al flusso le sue letture:
// qui si vede che cosa succede ai file quando l'ancora non manca.
func TestScenarioMGProdottiDalCartiglio(t *testing.T) {
	sc := scenaMGCon(t, "7120100", "7120103", "7120101", "7120104")
	c := sc.calcola()
	var righe []string
	for _, f := range sc.s.File {
		d := c.Destinazioni[f.AllegatoID]
		righe = append(righe, fmt.Sprintf("%s → %s [%s] presel=%v disc=%v", f.Nome, candidati(d), d.Stato, d.Preselezionabile, d.Discordanze))
	}
	t.Logf("oggi, le destinazioni con i prodotti dal cartiglio:\n%s", strings.Join(righe, "\n"))

	t.Run("le ancore degli STEP d'assieme ci sono", func(t *testing.T) {
		for _, p := range []string{"7120100", "7120103", "7120101", "7120104"} {
			if a := c.Ancore[p]; !a.Ancorato() || len(a.Portatori) != 1 || a.Portatori[0].Nome != nomeSTEPMG(p) {
				t.Errorf("l'ancora di %s: %+v", p, a)
			}
		}
	})

	t.Run("nessun file si preseleziona su un pezzo che non e' il suo", func(t *testing.T) {
		for _, f := range sc.s.File {
			d := c.Destinazioni[f.AllegatoID]
			if d.Preselezionabile && (len(d.Candidati) == 0 || !stessoPezzo(d.Candidati[0].Codice, pezzoDelFile(f.Nome))) {
				t.Errorf("%s preselezionato su %s", f.Nome, candidati(d))
			}
		}
	})

	t.Run("il PDF d'assieme non va su un figlio del suo elenco", func(t *testing.T) {
		for _, a := range assiemiMG {
			d := sc.dest(c, nomePDFMG(a.codice))
			if d.Preselezionabile && len(d.Candidati) > 0 && !stessoPezzo(d.Candidati[0].Codice, a.codice) {
				t.Errorf("%s preselezionato su %s", nomePDFMG(a.codice), candidati(d))
			}
		}
	})

	t.Run("da fare: il PDF d'assieme ha per primo candidato il suo assieme", func(t *testing.T) {
		var male []string
		for _, a := range assiemiMG {
			d := sc.dest(c, nomePDFMG(a.codice))
			if len(d.Candidati) == 0 || !stessoPezzo(d.Candidati[0].Codice, a.codice) {
				male = append(male, nomePDFMG(a.codice)+" → "+candidati(d))
			}
		}
		daFareMG(t, len(male) == 0,
			"le righe dell'elenco particolari entrano oggi come letture del cartiglio (pdf_testo_famiglia, le prime 8) e diventano candidati; il codice vero e' tagliato",
			strings.Join(male, "; "))
	})

	t.Run("da fare: il PDF e il DXF di un particolare vanno al particolare", func(t *testing.T) {
		var male []string
		for _, p := range particolariMG {
			nomi := []string{nomePDFMG(p.codice)}
			if p.dxf {
				nomi = append(nomi, nomeDXFMG(p.codice))
			}
			for _, n := range nomi {
				d := sc.dest(c, n)
				if len(d.Candidati) == 0 || !stessoPezzo(d.Candidati[0].Codice, p.codice) {
					male = append(male, n+" → "+candidati(d))
				}
			}
		}
		daFareMG(t, len(male) == 0,
			"il DXF ha solo il nome, e il nome «ACME-030P<codice>» non e' un codice del cliente; il PDF con «SPECCHIATO DI» ha due codici a 85",
			fmt.Sprintf("%d file senza il loro pezzo, per esempio %s", len(male), strings.Join(male[:min(4, len(male))], "; ")))
	})

	// Invariante: un figlio in comune fra due assiemi (7120111 sotto 7120100 e sotto 7120101; 7120133 ×2 sotto
	// 7120103 e sotto 7120104) e' UNA destinazione con le posizioni di tutti e due i padri, con le loro quantita'.
	t.Run("un figlio in comune fra due assiemi e' una destinazione con due posizioni", func(t *testing.T) {
		for _, x := range []struct{ codice, posizioni string }{{"7120111", "7120100×1 7120101×1"}, {"7120133", "7120103×2 7120104×2"}} {
			nome := nomeSTEPMG(x.codice)
			if x.codice == "7120133" {
				// 7120133 non ha file suoi fra gli allegati: se ne guarda la voce dell'indice
				v := c.Indice.Voci[x.codice]
				if v == nil {
					t.Errorf("7120133 non e' nell'indice")
					continue
				}
				var pos []string
				for _, p := range v.Posizioni {
					pos = append(pos, fmt.Sprintf("%s×%d", p.Padre, p.Qta))
				}
				if strings.Join(pos, " ") != x.posizioni {
					t.Errorf("la voce di 7120133: posizioni %v, attese %s", pos, x.posizioni)
				}
				continue
			}
			d := sc.dest(c, nome)
			var pos []string
			if len(d.Candidati) > 0 {
				for _, p := range d.Candidati[0].Posizioni {
					pos = append(pos, fmt.Sprintf("%s×%d", p.Padre, p.Qta))
				}
			}
			if len(d.Candidati) != 1 || d.Candidati[0].Codice != x.codice || strings.Join(pos, " ") != x.posizioni {
				t.Errorf("%s: %s, posizioni %v; attese %s", nome, candidati(d), pos, x.posizioni)
			}
		}
	})
}
