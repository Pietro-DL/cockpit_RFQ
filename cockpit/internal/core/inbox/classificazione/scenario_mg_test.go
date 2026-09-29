package classificazione

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"promatec/cockpit/internal/core/registro/regole"
	"promatec/cockpit/internal/platform/contratti/worker"
)

// Lo scenario «MG» (29/09, prova da fare del giro 4): una mail vera di un cliente, difficile da smistare, rifatta
// con dati fittizi ACME e con la stessa forma. La mail, i 49 allegati e le cause con file:riga stanno in
// docs/domande/scenario_MG_28-09.md (fuori da git, dati reali ammessi solo li'). La forma:
//   - una mail girata dal buyer («I: Elenco codici ACME») che chiede «vostra miglior offerta» con
//     una tabella di QUATTRO prodotti (P7120100, P7120103, P7120101, P7120104, 5 pezzi ciascuno);
//   - 49 allegati: per ogni pezzo lo STEP «ACME-030\P<codice> 00 IN_WORK.stp», il PDF «ACME-030\P<codice>.pdf»
//     e per le lamiere il DXF. Il «\» e' un separatore di cartella del PDM del cliente: il nome che arriva al
//     server e' senza («ACME-030P7120100.pdf»), e la P si attacca al prefisso di progetto;
//   - cinque assiemi saldati (7120100 DX e 7120101 SX speculari, 7120102, 7120103 e 7120104 specchiati), molti
//     figli in comune (7120111…7120117 stanno sotto 7120100 e sotto 7120101; 7120122 sotto 7120103 e 7120104);
//   - i PDF d'assieme hanno l'ELENCO PARTICOLARI subito sopra il cartiglio, dentro la zona in basso a destra, e
//     il codice del disegno nel campo «Part Nr:» del cartiglio, con l'etichetta su una riga e il valore sotto;
//   - la tabella della mail dice P7120100 (con la P), il cartiglio e lo STEP 7120100 (senza).
//
// Della mail vera resta solo la forma: l'oggetto, le frasi del corpo, le descrizioni dei pezzi e le note dei
// disegni sono inventati.
//
// Le decisioni che fissano i risultati attesi: il cartiglio fa fede per l'identita' (29/09, principio
// generale); i nomi dei file, i PRODUCT e i testi della mail sono alias o evidenze piu' deboli (29/09, 1 e 6);
// nessuna identita' tecnica senza una persona (27/09 «bis» e «ter»); le destinazioni del flusso (F8) sono
// proposte, mai scritture.
//
// Ogni aspettativa non ancora soddisfatta sta in un sottotest suo che finisce con t.Skip("da fare giro 4: …"):
// la prova resta verde e `go test -run TestScenarioMG -v` elenca i «da fare». Quando il comportamento arriva, il
// sottotest passa da solo (e il suo daFare va tolto, con l'asserzione vera al suo posto). I sottotest senza
// daFare sono invarianti di oggi che devono restare vere.

// motoreMG e' il motore ACME con la forma della famiglia del cliente vero: 712 + 4 cifre, eventuale P davanti,
// revisione «-revNN» nei file. La P sta DENTRO il gruppo del codice, come nella regola vera: per il motore
// P7120100 e 7120100 sono due codici diversi.
func motoreMG(t *testing.T) *Motore {
	t.Helper()
	m := Compila("ACME", regole.Regole{FamiglieCodice: []regole.FamigliaCodice{{
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

// daFare chiude un sottotest «da fare»: se l'aspettativa e' soddisfatta passa (e lo dice), altrimenti salta con
// quello che succede oggi.
func daFare(t *testing.T, soddisfatta bool, cosa, oggi string) {
	t.Helper()
	if soddisfatta {
		t.Logf("soddisfatta: %s — togliere daFare e lasciare l'asserzione", cosa)
		return
	}
	t.Skipf("da fare giro 4: %s (oggi: %s)", cosa, oggi)
}

// rigaMG e' una riga dell'elenco particolari di un disegno d'assieme.
type rigaMG struct {
	codice, descrizione string
	qta                 int
}

// testoAssiemeMG e' il testo di un disegno d'assieme del cliente come lo riporta il worker (analizzatore 4):
// nella zona in basso a destra della pagina 1, nell'ordine del file, l'elenco particolari («Pos. Part Number
// Descrizione Q.ty»), la frase «SPECULARE DI <altro>» (se c'e'), la tabella delle revisioni e il cartiglio con
// «Family: Part Nr: Descrizione: Description:» su una riga e i valori sulla riga sotto. Il campo che il worker
// riconosce come «codice» e' quello dell'intestazione dell'elenco («PART NUMBER», worker_analisi.py,
// ETICHETTE_CARTIGLIO): il suo valore e' la riga sotto, cioe' la PRIMA riga dell'elenco. «Part Nr» non e' fra
// le etichette, e il codice vero del disegno non ha un campo. E' la lettura del testo dei PDF veri fatta con il
// connettore (docs/domande/scenario_MG_28-09.md), rimessa nella forma dei fatti del worker.
func testoAssiemeMG(codice, descrizione, speculare string, righe []rigaMG) worker.TestoPDF {
	var b strings.Builder
	if len(righe) > 0 {
		b.WriteString("Pos. Part Number Descrizione Q.ty\n")
		for i, r := range righe {
			fmt.Fprintf(&b, "%d %s %s %d\n", i+1, r.codice, r.descrizione, r.qta)
		}
	}
	if speculare != "" {
		b.WriteString("SPECULARE DI " + speculare + "\n")
	}
	b.WriteString("Rev Mod. N. Type Modification object / Descrizione modifica Drawn by Approved by\n")
	b.WriteString("00 ACME-P003 A Prima emissione progettista --- ---\n")
	b.WriteString("Family: Part Nr: Descrizione: Description:\n")
	b.WriteString("-- " + codice + " " + descrizione + "\n")
	b.WriteString("ACME S.p.A. - DRAFT -")
	tp := testoPDF(b.String(), "Isometric view Section view A-A", worker.MetadatiPDF{})
	if len(righe) > 0 {
		tp.Cartiglio = append(tp.Cartiglio, worker.CampoCartiglio{Etichetta: worker.CampoCodice, Letta: "Part Number",
			Valore: righe[0].codice, Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo, Riquadro: riquadroBD})
	}
	tp.Cartiglio = append(tp.Cartiglio, worker.CampoCartiglio{Etichetta: worker.CampoRevisione, Letta: "Rev",
		Valore: "Mod. N. Type Modification object /", Pagina: 1, Zona: worker.ZonaBassoDestra, Fonte: worker.FonteTestoNativo,
		Riquadro: riquadroBD})
	return tp
}

// I disegni dello scenario (i numeri veri sono nel documento).
var (
	// 7120100: TELAIO SALDATO DX, 10 righe, speculare di 7120101 (nel caso vero il codice del file diventava il
	// figlio in posizione 1)
	righe7120100 = []rigaMG{{"7120110", "PIASTRA A DX", 1}, {"7120111", "LAMIERA B", 1}, {"7120112", "TONDO C", 1},
		{"7120113", "PIASTRINA D", 1}, {"7120130", "BUSSOLA E", 1}, {"7120131", "BLOCCHETTO F", 1},
		{"7120114", "LAMIERA G", 1}, {"7120115", "COSTOLA H", 1}, {"7120116", "SQUADRETTA K", 1}, {"7120117", "COPERCHIO L", 1}}
	// 7120102: TELAIO CENTRALE, 3 righe (nel caso vero colonna e score non combaciavano)
	righe7120102 = []rigaMG{{"7120121", "MONTANTE M", 1}, {"7120120", "SQUADRA N", 1}, {"7120135", "SPINA R", 1}}
)

func valutaPDFMG(t *testing.T, m *Motore, nome string, tp worker.TestoPDF) Valutazione {
	t.Helper()
	return Valuta(IngressoFile{Da: DaAnalisi, NomeFile: nome, Direzione: "entrata", Motore: m,
		Esito: &Esito{"disegno_2d", "cartiglio"}, Fatti: fattiDisegno(t, tp)})
}

func valoriEvidenze(d Dimensione) string {
	var p []string
	for _, e := range d.Evidenze {
		v := e.Valore
		if v == "" {
			v = "«" + e.Testo + "»"
		}
		p = append(p, fmt.Sprintf("%s %s %d", e.Regola, v, e.Score))
	}
	return strings.Join(p, " | ")
}

func TestScenarioMG(t *testing.T) {
	m := motoreMG(t)

	// ---------------------------------------------------------------- (c) i nomi dei file

	// Invariante: con il separatore di cartella al suo posto la famiglia del cliente trova il codice nel nome;
	// e' il motivo per cui il «\» tolto prima del server (outlook_com.py, Attachment.FileName) conta.
	t.Run("nomi: con il separatore di cartella la famiglia trova il codice", func(t *testing.T) {
		for _, nome := range []string{`ACME-030\P7120100.pdf`, `ACME-030\P7120100 00 IN_WORK.stp`, `ACME-030\P7120110.dxf`} {
			var trovati []string
			for _, c := range m.Estrai(Testo{Dove: "nome", Corpo: nome}).Codici {
				if c.Origine == "famiglia" {
					trovati = append(trovati, c.Codice)
				}
			}
			if strings.Join(trovati, " ") != "P7120100" && strings.Join(trovati, " ") != "P7120110" {
				t.Errorf("%s: codici di famiglia %v, atteso il codice dopo il separatore", nome, trovati)
			}
		}
	})

	t.Run("da fare: il nome senza separatore «ACME-030P7120100.pdf» legge il codice di famiglia", func(t *testing.T) {
		v := Valuta(IngressoFile{Da: DaAnalisi, NomeFile: "ACME-030P7120100.pdf", Direzione: "entrata", Motore: m})
		_, ok := evidenzaDi(v.Codice, "nome_codice_famiglia")
		daFare(t, ok && (v.Codice.Valore == "7120100" || v.Codice.Valore == "P7120100"),
			"il prefisso di progetto «ACME-030» (la cartella del PDM) non fa parte del codice: il nome dice P7120100 (alias di 7120100)",
			valoriEvidenze(v.Codice))
	})

	t.Run("da fare: il nome dello STEP «… 00 IN_WORK.stp» legge codice e revisione 00", func(t *testing.T) {
		v := Valuta(IngressoFile{Da: DaAnalisi, NomeFile: "ACME-030P7120100 00 IN_WORK.stp", Direzione: "entrata", Motore: m,
			Esito: &Esito{"cad_3d", "step"}})
		daFare(t, (v.Codice.Valore == "7120100" || v.Codice.Valore == "P7120100") && v.Rev.Valore == "00",
			"«<prefisso>P<codice> <rev> <stato PDM>»: « 00» e' la revisione, «IN_WORK» lo stato del PDM, non parte del codice",
			fmt.Sprintf("codice %q rev %q; evidenze: %s", v.Codice.Valore, v.Rev.Valore, valoriEvidenze(v.Codice)))
	})

	t.Run("da fare: la revisione della radice dello STEP «00.00» e' la 00 del cartiglio", func(t *testing.T) {
		v := Valuta(IngressoFile{Da: DaAnalisi, NomeFile: "ACME-030P7120100 00 IN_WORK.stp", Direzione: "entrata", Motore: m,
			Esito:  &Esito{"cad_3d", "step"},
			Radice: &Radice{Codice: "7120100", Rev: "00.00", DiFamiglia: true, Famiglia: "codici ACME", Dove: "id", Testo: "7120100", RevDalFile: true}})
		if v.Codice.Valore != "7120100" {
			t.Errorf("la radice di famiglia dello STEP e' il codice del file: %q (%s)", v.Codice.Valore, valoriEvidenze(v.Codice))
		}
		daFare(t, v.Rev.Valore == "00",
			"la rev del PDM «NN.MM» (PRODUCT_DEFINITION_FORMATION) si normalizza nella forma del cartiglio «NN» con la regola del cliente",
			fmt.Sprintf("rev %q (%s)", v.Rev.Valore, v.Rev.Regola))
	})

	// ---------------------------------------------------------------- (d) prodotto con la P e disegni senza

	t.Run("da fare: P7120100 della mail e 7120100 del cartiglio sono lo stesso pezzo (alias)", func(t *testing.T) {
		c1, _ := m.Canonico("P7120100", "")
		c2, _ := m.Canonico("7120100", "")
		daFare(t, strings.EqualFold(c1, c2),
			"la P davanti e' una lettura del cliente (da confermare con l'utente: prototipo?): identita' = codice del cartiglio 7120100, P7120100 alias",
			fmt.Sprintf("Canonico(P7120100) = %s, Canonico(7120100) = %s", c1, c2))
	})

	// ---------------------------------------------------------------- (a) il PDF d'assieme

	v100 := valutaPDFMG(t, m, "ACME-030P7120100.pdf", testoAssiemeMG("7120100", "TELAIO SALDATO DX", "7120101", righe7120100))
	r100 := v100.Riepilogo()
	t.Logf("oggi ACME-030P7120100.pdf: colonna %q score %d (%s); dimensione %q %s %s; evidenze: %s",
		r100.Codice, r100.Confidenza, r100.Fonte, v100.Codice.Valore, v100.Codice.Regola, v100.Codice.Stato, valoriEvidenze(v100.Codice))

	// Invariante: un codice sbagliato non si presenta mai come sicuro. Se la colonna non dice il codice del
	// cartiglio, la dimensione del codice e' discorde (niente preselezionato, U7).
	t.Run("PDF d'assieme: un codice che non e' quello del cartiglio non e' mai sicuro", func(t *testing.T) {
		if r100.Codice != "7120100" && v100.Codice.Stato != StatoDiscorde {
			t.Errorf("colonna %q con la dimensione %s: un figlio dell'elenco particolari presentato come il codice del file", r100.Codice, v100.Codice.Stato)
		}
	})

	t.Run("da fare: il codice del PDF d'assieme e' quello del cartiglio («Part Nr»), non la prima riga dell'elenco particolari", func(t *testing.T) {
		daFare(t, r100.Codice == "7120100" && v100.Codice.Valore == "7120100",
			"cartiglio fa fede: il valore sotto «Part Nr:» e' il codice del disegno; le righe dell'elenco particolari sono i suoi figli",
			fmt.Sprintf("colonna %q, dimensione %q %s", r100.Codice, v100.Codice.Valore, v100.Codice.Stato))
	})

	t.Run("da fare: le righe dell'elenco particolari non sono letture del codice del file", func(t *testing.T) {
		var figli []string
		for _, e := range v100.Codice.Evidenze {
			if e.Regola == "pdf_testo_famiglia" && e.Valore != "7120100" {
				figli = append(figli, e.Valore)
			}
		}
		daFare(t, len(figli) == 0,
			"l'elenco particolari (e «SPECULARE DI») si legge come distinta del disegno e come relazione, non come pdf_testo_famiglia",
			"pdf_testo_famiglia per "+strings.Join(figli, ", "))
	})

	t.Run("da fare: la lettura del nome del file non esce dalle evidenze (MaxEvidenze)", func(t *testing.T) {
		_, gen := evidenzaDi(v100.Codice, "nome_codice_generico")
		_, fam := evidenzaDi(v100.Codice, "nome_codice_famiglia")
		daFare(t, gen || fam,
			"con piu' di 8 letture a 85 il taglio a MaxEvidenze toglie quella del nome (45), e con lei il ripiego della colonna sul nome quando le fonti discordano",
			fmt.Sprintf("%d evidenze, nessuna del nome", len(v100.Codice.Evidenze)))
	})

	t.Run("da fare: il codice vero del disegno resta fra le evidenze", func(t *testing.T) {
		_, c := evidenzaConValore(v100.Codice, "7120100")
		daFare(t, c,
			"7120100 e' l'undicesima lettura a 85 nell'ordine del file, e il taglio a 8 la toglie: il flusso (F8) non la vede piu'",
			"7120100 assente dalle evidenze")
	})

	// Gli stessi fatti prodotti dal worker VERO (worker_analisi.analizza_file) su un PDF generato con la forma di
	// quelli del cliente: testdata/scenario_mg/assieme_7120100_dettagli.json, rigenerato da
	// workers/tests/test_scenario_mg.py (disegno_assieme_mg). Il worker prende «Part Number» dell'elenco come
	// campo del codice (valore: la prima riga) e non conosce «Part Nr».
	vVero := func(t *testing.T) Valutazione {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("testdata", "scenario_mg", "assieme_7120100_dettagli.json"))
		if err != nil {
			t.Fatal(err)
		}
		return Valuta(IngressoFile{Da: DaAnalisi, NomeFile: "ACME-030P7120100.pdf", Direzione: "entrata", Motore: m, Fatti: b})
	}
	t.Run("PDF d'assieme letto dal worker vero: un codice che non e' quello del cartiglio non e' mai sicuro", func(t *testing.T) {
		v := vVero(t)
		r := v.Riepilogo()
		t.Logf("oggi, dal worker vero: colonna %q, dimensione %q %s; %s", r.Codice, v.Codice.Valore, v.Codice.Stato, valoriEvidenze(v.Codice))
		if r.Codice != "7120100" && v.Codice.Stato != StatoDiscorde {
			t.Errorf("colonna %q con la dimensione %s", r.Codice, v.Codice.Stato)
		}
	})
	t.Run("da fare: il PDF d'assieme letto dal worker vero ha il codice del cartiglio", func(t *testing.T) {
		v := vVero(t)
		r := v.Riepilogo()
		daFare(t, r.Codice == "7120100", "il cartiglio fa fede (vedi sopra, con i fatti del worker vero)",
			fmt.Sprintf("colonna %q, dimensione %q %s", r.Codice, v.Codice.Valore, v.Codice.Stato))
	})

	// ---------------------------------------------------------------- (b) colonna e score della riga Inbox

	v102 := valutaPDFMG(t, m, "ACME-030P7120102.pdf", testoAssiemeMG("7120102", "TELAIO CENTRALE", "", righe7120102))
	r102 := v102.Riepilogo()
	t.Logf("oggi ACME-030P7120102.pdf: colonna %q score %d (%s); dimensione %q score %d %s %s",
		r102.Codice, r102.Confidenza, r102.Fonte, v102.Codice.Valore, v102.Codice.Score, v102.Codice.Regola, v102.Codice.Stato)

	// Invariante: con le fonti discordi la colonna tiene la lettura del nome (A5.14.3, U7), con il SUO score.
	t.Run("PDF d'assieme corto: con le fonti discordi la colonna e' il nome con il suo score", func(t *testing.T) {
		if v102.Codice.Stato == StatoDiscorde {
			if e, ok := letturaDelNome(v102.Codice, prefissiNomeCodice); ok && (r102.Codice != e.Valore || r102.Confidenza != e.Score) {
				t.Errorf("colonna %q score %d, lettura del nome %q score %d", r102.Codice, r102.Confidenza, e.Valore, e.Score)
			}
		}
	})

	t.Run("da fare: la riga dell'Inbox mostra lo score della lettura in colonna, non quello della dimensione", func(t *testing.T) {
		// frammenti.html (allegato_riga) scrive la colonna (.Codice, dal Riepilogo) e accanto «score» e regola della
		// DIMENSIONE ($v.Codice): con le fonti discordi sono due letture diverse. Qui si vede la differenza; il
		// template si prova in web (TestScenarioMGRigaInbox).
		daFare(t, r102.Codice == v102.Codice.Valore || v102.Codice.Stato != StatoDiscorde,
			"con il codice discorde la riga dice «fonti discordi» e lo score della lettura del nome, o il codice del cartiglio con il suo",
			fmt.Sprintf("colonna %q (score %d), accanto «score %d · %s» di %q", r102.Codice, r102.Confidenza, v102.Codice.Score, v102.Codice.Regola, v102.Codice.Valore))
	})

	// ---------------------------------------------------------------- un particolare con «SPECCHIATO DI»

	t.Run("da fare: il PDF di un particolare con «SPECCHIATO DI» ha il codice del suo cartiglio", func(t *testing.T) {
		tp := testoAssiemeMG("7120110", "PIASTRA A DX", "", nil)
		tp.Frammenti[0].Testo = "12.5 SPECCHIATO DI 7120118\n" + tp.Frammenti[0].Testo
		v := valutaPDFMG(t, m, "ACME-030P7120110.pdf", tp)
		r := v.Riepilogo()
		daFare(t, r.Codice == "7120110",
			"«SPECCHIATO DI 7120118» e' una relazione fra pezzi (DX/SX), non il codice del disegno: il 2D di 7120110 deve trovare il suo pezzo",
			fmt.Sprintf("colonna %q, dimensione %q %s; %s", r.Codice, v.Codice.Valore, v.Codice.Stato, valoriEvidenze(v.Codice)))
	})

	// ---------------------------------------------------------------- la tabella della mail e il riferimento

	// Invariante: la tabella della mail propone i quattro prodotti chiesti (con o senza la P, secondo l'alias
	// che la regola del cliente dara'), e nient'altro come prodotto.
	t.Run("la tabella della mail propone i quattro prodotti", func(t *testing.T) {
		var prop []string
		for _, c := range m.Estrai(Testo{Dove: "corpo", Corpo: ingressoMG().Corpo}).Proponibili() {
			prop = append(prop, strings.TrimPrefix(strings.ToUpper(c.Codice), "P"))
		}
		if strings.Join(prop, " ") != "7120100 7120103 7120101 7120104" {
			t.Errorf("proponibili dalla mail: %v", prop)
		}
	})

	t.Run("da fare: «ACME26-030» e' il riferimento della richiesta (e «ACME-030» nei nomi dei file lo stesso)", func(t *testing.T) {
		// le regole vere del cliente non hanno un riferimento_rfq: il numero di progetto della mail resta un
		// «codice generico», e i file («ACME-030\…») non si legano alla richiesta. Con un riferimento dichiarato
		// nelle regole (qui la forma che servirebbe) la mail lo riconosce; il prefisso dei file, senza l'anno, no.
		e := m.Estrai(Testo{Dove: "corpo", Corpo: ingressoMG().Corpo})
		conRif := Compila("ACME", regole.Regole{FamiglieCodice: m.Regole.FamiglieCodice,
			RiferimentoRFQ: &regole.Riferimento{Regex: `(?i)\b(?P<rif>ACME\d{2}-\d{3})\b`, Descrizione: "progetto ACME", Esempio: "ACME26-030"}})
		dalCorpo := conRif.Estrai(Testo{Dove: "corpo", Corpo: ingressoMG().Corpo}).Riferimento
		dalNome := conRif.Estrai(Testo{Dove: "nome", Corpo: `ACME-030\P7120100.pdf`}).Riferimento
		daFare(t, e.Riferimento == "ACME26-030" && dalNome != "",
			"il riferimento del cliente (progetto «ACME26-030» nella mail, «ACME-030» come cartella nei nomi) nelle regole, perche' la RFQ si segua nel tempo",
			fmt.Sprintf("con le regole di oggi riferimento %q; con un riferimento dichiarato: dal corpo %q, dal nome del file %q", e.Riferimento, dalCorpo, dalNome))
	})

	// ---------------------------------------------------------------- (e) l'evento

	t.Run("evento: la mail girata con 30 file tecnici e' una richiesta d'offerta", func(t *testing.T) {
		e := Evento(ingressoMG())
		if e.Atto != AttoRichiestaOfferta {
			t.Fatalf("atto %q (%s): la mail chiede un'offerta", e.Atto, strings.Join(e.Evidenze, "; "))
		}
		if !strings.Contains(strings.Join(e.Evidenze, " "), "30 file tecnici") {
			t.Errorf("i file tecnici sono 30 (19 STEP e 11 DXF; i 19 PDF sono da determinare): %v", e.Evidenze)
		}
	})

	t.Run("da fare: «vostra miglior offerta» e' una parola di richiesta", func(t *testing.T) {
		e := Evento(ingressoMG())
		ev := strings.Join(e.Evidenze, "; ")
		daFare(t, e.Forza == ForzaChiaro && strings.Contains(ev, "offerta"),
			"«attendo vostra miglior offerta per questi codici» e' una richiesta (reParoleRFQ non conosce «offerta» da sola)",
			fmt.Sprintf("%s (%s): %s", e.Atto, e.Forza, ev))
	})
}

// evidenzaConValore dice se una dimensione ha un'evidenza con quel valore.
func evidenzaConValore(d Dimensione, valore string) (Evidenza, bool) {
	for _, e := range d.Evidenze {
		if strings.EqualFold(e.Valore, valore) {
			return e, true
		}
	}
	return Evidenza{}, false
}

// ingressoMG e' la mail dello scenario: girata dal buyer («I:»), senza storia sotto, con la tabella dei quattro
// prodotti e i 49 allegati.
func ingressoMG() IngressoEvento {
	corpo := "Buongiorno\n\nvi mando l'elenco qui sotto e attendo vostra miglior offerta per questi codici\n\n" +
		"Serve una consegna rapida, entro fine mese\n\nResto in attesa\n\nGrazie\n\n" +
		"REPARTO ACME\nACME26-030 PZ.\nP7120100 TELAIO SALDATO DX 5\nP7120103 LEVA SALDATA DX 5\n" +
		"P7120101 TELAIO SALDATO SX 5\nP7120104 LEVA SALDATA SX 5\n\nMario Rossi\nBuyer\nACME S.p.A.\n"
	return IngressoEvento{Controparte: ControparteCliente, Direzione: "entrata", Oggetto: "I: Elenco codici ACME ",
		Corpo: corpo, Mittente: "buyer@acme.example", NomiAllegati: allegatiMG()}
}

// allegatiMG sono i nomi dei 49 allegati come arrivano al server (senza il «\»): 19 STEP, 19 PDF, 11 DXF.
func allegatiMG() []string {
	conDXF := map[string]bool{"7120106": true, "7120110": true, "7120111": true, "7120113": true, "7120114": true, "7120115": true,
		"7120116": true, "7120117": true, "7120120": true, "7120121": true, "7120118": true}
	var out []string
	for _, c := range codiciMG {
		out = append(out, "ACME-030P"+c+" 00 IN_WORK.stp")
		if conDXF[c] {
			out = append(out, "ACME-030P"+c+".dxf")
		}
		out = append(out, "ACME-030P"+c+".pdf")
	}
	return out
}

// codiciMG sono i 19 pezzi con uno STEP e un PDF fra gli allegati, nell'ordine della mail: il perno sciolto, i
// cinque assiemi, l'asta sciolta, i particolari.
var codiciMG = []string{"7120105", "7120100", "7120101", "7120102", "7120103", "7120104", "7120106",
	"7120110", "7120111", "7120112", "7120113", "7120114", "7120115", "7120116", "7120117", "7120120", "7120121", "7120122", "7120118"}
